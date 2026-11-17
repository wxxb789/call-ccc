package ccc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The three routes -- Codex, Claude Code and Pi -- carry the same install/verify/probe/turn machinery
// three times over, differing only in a handful of route-specific facts (their subdirectory, their mode
// constant, the inherited reader each verifies through, the environment variable that selects their data
// directory, their default executable and the shape of a version line). This file holds that machinery
// once, with each difference passed in, so a later change lands in one place instead of three. The route
// files keep their own option types, method constants, agent constant, target-path join, round-trip
// comparator and blocked-environment names as their readable face.

// harnessOutputMax and harnessDiagnosticsMax bound what one harness run may add to this process's
// memory, for both the RPC arm and the turn arm of the checks. The answer a report quotes is cut to
// boundedText's own limit anyway (8192 bytes by default), and a harness flooding either stream is
// answered by its deadline and an excerpt rather than by this process growing with every byte it writes.
// The diagnostics bound is the size the Codex app server keeps of its own, 4096 bytes.
const (
	harnessOutputMax      = 1 << 20
	harnessDiagnosticsMax = 4096
)

// entryClock hands out timestamps that never go backwards: each target orders its records by the
// timestamp it records, and a source whose messages are unordered (a DAG, a replayed fork) would
// otherwise write a file whose order contradicts its own times. The layout is the route's own: the Codex
// and Claude routes write `time.RFC3339Nano`, and Pi writes milliseconds, always three digits, always
// UTC (`2026-11-10T02:07:01.784Z`, piEntryTimestampLayout).
type entryClock struct {
	last   time.Time
	layout string
}

func newEntryClock(start time.Time, layout string) *entryClock {
	return &entryClock{last: start.UTC(), layout: layout}
}

func (c *entryClock) next(at time.Time) string {
	at = at.UTC()
	if at.IsZero() || at.Before(c.last) {
		at = c.last
	}
	c.last = at
	return at.Format(c.layout)
}

// ledger accumulates the bookkeeping the three writers share while they walk a document: the calls
// carried with no result yet, and every tool name seen so far in the order it was first written.
type ledger struct {
	// pendingCalls holds the carried calls no result has answered yet. It is what an id-less result may
	// be paired with, and only while exactly one call is outstanding: with two or more, the result could
	// belong to any of them, and that ambiguity must not disappear just because a further call arrived.
	pendingCalls     map[string]struct{}
	foreignToolNames map[string]struct{}
	toolNameOrder    []string
}

// solePendingCall answers the one call an id-less result may belong to: exactly one carried call with
// no result yet.
func (l *ledger) solePendingCall() (string, bool) {
	if len(l.pendingCalls) != 1 {
		return "", false
	}
	for callID := range l.pendingCalls {
		return callID, true
	}
	return "", false
}

// noteToolName records one tool name in the order it is first written.
func (l *ledger) noteToolName(name string) {
	if l.foreignToolNames == nil {
		l.foreignToolNames = map[string]struct{}{}
	}
	if _, ok := l.foreignToolNames[name]; ok {
		return
	}
	l.foreignToolNames[name] = struct{}{}
	l.toolNameOrder = append(l.toolNameOrder, name)
}

// distinctCalls answers one message's calls with the calls whose id the message already carried dropped,
// in the message's own order: such a call is another projection of a call already taken (the inherited
// Codex reader expands one apply_patch into one call per file it names, every projection sharing the id
// and the input) and not a second invocation, so only the first is kept and every dropped projection
// charges the caller's own loss counter. A call with no recorded id is never treated as a repeat: its id
// is generated from its own place in the message, so two of them are two calls.
func distinctCalls[C any](message Message, project func(index int, call ToolCall) C, coalesced *int) []C {
	calls := make([]C, 0, len(message.ToolCalls))
	seen := make(map[string]struct{}, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		id := strings.TrimSpace(call.ToolUseID)
		if id != "" {
			if _, ok := seen[id]; ok {
				*coalesced++
				continue
			}
			seen[id] = struct{}{}
		}
		calls = append(calls, project(index, call))
	}
	return calls
}

// projectedParentProviders are the agents whose reader answers a record under a tool result as hanging
// off the message that made the call (the parser resolves a result's descendants to their visible
// ancestor): the Pi parser and the Pi-derived readers, which internal/parser/provider.go builds with
// newPiProviderFactory and newTauProviderFactory. The list is spelled here because the inherited registry
// exposes no format-family predicate (it has IsCodexFormatAgent and no Pi counterpart) and the carried
// parser is not edited here. A provider the list does not name is read with exact parent answers --
// a Claude transcript's parentUuid names a record, not a projection -- so the writers keep the narrower
// reading there, which invents no relation for a read whose answers are its own.
var projectedParentProviders = map[string]bool{
	"pi":          true,
	"tau":         true,
	"omp":         true,
	"omo":         true,
	"stepcode":    true,
	"prime-agent": true,
	"direct-api":  true,
}

// projectedParentProvider reports whether a source read through this provider answers projections: a
// parent it names may be the message that made a call rather than the record the source's file named.
func projectedParentProvider(provider string) bool {
	return projectedParentProviders[strings.ToLower(strings.TrimSpace(provider))]
}

// sourceRecordOrigin is what one written entry stands for in the source's own record stream: the
// record's identity, the record *it* hangs off as the read answered it, the entry this conversion
// chained the record's first entry to, and whether the entry was written as a tool result. The entry the
// next record hangs off is decided from it, where the read may answer a projection: a record that
// follows a tool result in the source's file hangs off that result while the read answers it with the
// message that made the call, and an exact read answers where the record hangs off itself.
type sourceRecordOrigin struct {
	sourceID     string
	sourceParent string
	parentID     *string
	toolResult   bool
}

// sourceTree maps the source's own record relations onto the entries a conversion writes.
//
// A target format that is itself a tree is written as the *source's* own tree: each entry hangs off the
// entry of the record the source named as its parent, so the target's own loader walks the source's
// active path and a branch the conversation left stays off it as recorded history rather than being
// chained in as an active turn. One record is one place in that tree even where the target writes it as
// several entries (a prompt and a tool result it also carries): the record that follows hangs off the
// *final* entry written for it, which is what keeps the tool result on the walk the target's loader
// makes.
type sourceTree struct {
	// sourceRecordParents is every source record the document holds, keyed by its own identity: the
	// record it hangs off, as the read answered it. It is filled for the records the target writes and
	// for the ones it leaves out, so an entry whose own parent has no entry of its own can still be
	// chained to the nearest ancestor that was written.
	sourceRecordParents map[string]string
	// writtenEntryID maps a source record's identity to the final entry written for it.
	writtenEntryID map[string]string
	// toolResultEntry maps a call's source identity to the entry of the most recent tool result written
	// for it, and is consulted only when projectedParents holds. The inherited Pi reader answers a record
	// under a tool result as hanging off the message that made the call, so a later record naming that
	// message may in the source's own file have hung off a result of it standing between them; that
	// result is where such a record is chained, and the relation is counted
	// (unresolved_source_parents) because which of the two the record named is not answerable from the
	// read.
	toolResultEntry map[string]string
	// projectedParents says the read may answer a projection: projectedParentProvider of the source's own
	// provider. A read whose answers are exact keeps the narrower reading below, which invents nothing.
	projectedParents bool
	// lastEntryID is the entry written last and lastOrigin what it stands for; pending is what the
	// entries written next stand for.
	lastEntryID *string
	lastOrigin  sourceRecordOrigin
	pending     sourceRecordOrigin
	// unresolved counts the source relations this conversion could not place: the entries written for
	// their records follow the entry before them, which is a relation this build invented.
	unresolved int
}

func newSourceTree(provider string) sourceTree {
	return sourceTree{
		sourceRecordParents: map[string]string{},
		writtenEntryID:      map[string]string{},
		toolResultEntry:     map[string]string{},
		projectedParents:    projectedParentProvider(provider),
	}
}

// startRecord names the source record the entries written next stand for and resolves the entry its
// first entry hangs off from the source's own relation.
//
// A record the read answered an identity for but no parent is counted when an entry was already written:
// the record is not the conversation's first, so the entry written for it follows the entry before it --
// a relation this conversion invented. A document that records no identity at all is the other case: the
// read's own order is linear, following it invents nothing, and nothing is counted.
func (t *sourceTree) startRecord(id, parent string) {
	id = strings.TrimSpace(id)
	parent = strings.TrimSpace(parent)
	if id != "" {
		t.sourceRecordParents[id] = parent
	}
	if id != "" && parent == "" && t.lastEntryID != nil {
		t.unresolved++
	}
	t.pending = sourceRecordOrigin{sourceID: id, sourceParent: parent, parentID: t.parentFor(parent)}
}

// endRecord ends the record's walk, so a record that produced no entry cannot leak its relation onto the
// next record's first entry.
func (t *sourceTree) endRecord() { t.pending = sourceRecordOrigin{} }

// markToolResult says the entry about to be written is a tool result. Where the read may answer a
// projection, the record after it then hangs off the result rather than off the message that made the
// call, because that is the order the source's own file holds (call → result → what follows) and the
// other reading would put the result off the walk the target's loader makes.
func (t *sourceTree) markToolResult() { t.pending.toolResult = true }

// entryParent answers the parent the entry about to be written carries: the entry the source's relation
// resolved for the record's first entry, else the entry written last, because a record written as
// several entries is one place in the tree and its later entries follow each other.
func (t *sourceTree) entryParent() *string {
	if t.pending.parentID != nil {
		return t.pending.parentID
	}
	return t.lastEntryID
}

// wrote records one written entry: it is the entry the record being walked stands for -- the last of
// them, overwriting the map so the record's *final* entry is the place its children hang off -- and the
// parent the next entry written for that record follows.
func (t *sourceTree) wrote(id string) {
	t.lastEntryID = &id
	if t.pending.sourceID != "" {
		t.writtenEntryID[t.pending.sourceID] = id
	}
	if t.projectedParents && t.pending.toolResult && t.pending.sourceParent != "" {
		t.toolResultEntry[t.pending.sourceParent] = id
	}
	t.lastOrigin = t.pending
	t.pending.parentID = nil
	t.pending.toolResult = false
}

// parentFor answers the entry a source record's first written entry is chained to: the entry this
// conversion wrote for the record the source named as its parent.
//
// The source's own relation decides, not the order the file happens to hold its records in. A target
// format that is a tree is written as the source's own tree, so an entry follows the record it hangs
// off and a branch the conversation left stays off the path the target's own loader walks from the
// file's last entry. Three answers are possible for a record that *does* name a parent:
//
//   - the named record was written: its final entry is the parent;
//   - the named record was left out of the target (an omitted record, or one with no entry of its own):
//     the nearest ancestor that *was* written, walked through the document's own relations;
//   - the record that follows a tool result, where the read may answer a projection: the tool result
//     itself, because the source's own file runs call → result → what follows while the read answers
//     the next record as hanging off the call.
//
// The third answer is the projection undone, so it is taken only where the read may project
// (projectedParents); an exact read's answer names the record it means (a Claude transcript's
// parentUuid), and chaining a record through a result that merely stands beside the record it named
// would chain it to a sibling the source placed elsewhere -- reviving that sibling as an ancestor. An
// exact read resolves every named parent above and below by the read's own answer.
//
// A record that names no parent at all is read as the read's own order: this conversion cannot tell a
// record the source placed at the head of a line from one whose identity or parent the provider did not
// write, and chaining it to the entry before it keeps a record the read *did* carry visible, where a
// written null could cut the conversation into pieces the target's loader never walks. A parent the
// document cannot resolve -- a record outside the read -- follows the entry before it and is counted
// (unresolved): that is a relation this conversion invented, and the report says so rather than
// presenting the entry as one the source recorded hanging where it does.
//
// The record that names a call the read answered for a descendant of a tool result is the other counted
// answer: the read projects such a record onto the call's own message, so whether it hung off the call or
// off a result of it is not answerable, and the entry is chained through the result standing between them
// -- the reading the source's file order supports -- with the count naming the relation as one the read
// did not establish.
func (t *sourceTree) parentFor(parent string) *string {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return t.lastEntryID
	}
	if t.projectedParents && t.lastOrigin.toolResult && t.lastOrigin.sourceParent == parent {
		return t.lastEntryID
	}
	if id, ok := t.toolResultEntry[parent]; ok && t.projectedParents {
		t.unresolved++
		return &id
	}
	if id, ok := t.writtenAncestor(parent); ok {
		return &id
	}
	t.unresolved++
	return t.lastEntryID
}

// writtenAncestor answers the entry this conversion wrote for the nearest record a walk from sourceID
// reaches, and whether it found one at all. The walk is bounded by the document's own size rather than by
// a depth constant: it can step through each record the read holds at most once, so a linear history
// longer than any fixed bound is still walked to its end, while a cycle the source recorded (a record
// naming itself or a longer loop) cannot keep it going past the records the read answered.
func (t *sourceTree) writtenAncestor(sourceID string) (string, bool) {
	for steps := 0; sourceID != "" && steps <= len(t.sourceRecordParents); steps++ {
		if id, ok := t.writtenEntryID[sourceID]; ok {
			return id, true
		}
		next, ok := t.sourceRecordParents[sourceID]
		if !ok || next == sourceID {
			return "", false
		}
		sourceID = next
	}
	return "", false
}

// inactiveSourceRecords answers, for every message of a read, whether the source's own relations place
// it off the line the source's loader builds context from: the line the walk from the read's *last*
// record reaches. It is the record-level counterpart of the relation the tree writers resolve, for the
// targets whose format is a single line of records -- one where a record written becomes part of the
// conversation a resumed turn reads, so a branch the conversation left cannot travel there as recorded
// history without being revived as an active turn.
//
// The second answer is the count of places the walk took for a relation the read did not establish
// (unresolved), the same category the tree writers report (unresolved_source_parents): this target has
// no entry to chain, but a record's place on the line is likewise this conversion's answer where the
// read's own answer could not give one -- a record the read left parentless or gave a parent this read
// does not hold, and a record chained through a tool result standing between it and the call it named.
// The detail the report carries names each of the three.
//
// A record the read answered no identity for is never marked inactive: the read recorded no place for it
// in the source's tree, and omitting evidence on a relation the read did not give is exactly what the
// report exists to prevent. The walk uses the writers' own tool-result rule, so a record the reader
// answers as hanging off the message that made a call -- while the source's file runs call → result →
// what follows -- keeps the result on the line.
func inactiveSourceRecords(messages []Message, provider string) ([]bool, int) {
	inactive := make([]bool, len(messages))
	if len(messages) == 0 {
		return inactive, 0
	}
	byIdentity := make(map[string]int, len(messages))
	for index, message := range messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			if _, seen := byIdentity[id]; !seen {
				byIdentity[id] = index
			}
		}
	}
	onLine := make([]bool, len(messages))
	unresolved := 0
	// The walk needs no depth bound: each step moves to a record before the one it is at, so a record is
	// visited at most once and a cycle cannot be entered -- a source's linear history of any length is
	// walked to its end rather than cut off at a bound no real conversation was measured against.
	for cursor := len(messages) - 1; cursor >= 0; {
		if onLine[cursor] {
			break
		}
		onLine[cursor] = true
		next, counted := sourceLineParent(messages, byIdentity, cursor,
			projectedParentProvider(provider))
		if counted {
			unresolved++
		}
		if next >= cursor {
			break
		}
		cursor = next
	}
	for index, message := range messages {
		inactive[index] = strings.TrimSpace(message.SourceUUID) != "" && !onLine[index]
	}
	return inactive, unresolved
}

// sourceLineParent answers the record a walk from index hangs off, by the writers' own rule, and whether
// that place is a relation the read did not establish: the record the source named -- or the tool result
// standing between this record and the call it named, where the read may answer a projection -- and the
// record before it where the read answered no relation or named one it does not hold.
//
// The tool-result step is the writers' own projection rule, taken only where the read may project, and
// it is counted only when the result stands between the record and the call it named: there the read's
// answer cannot settle whether the record hung off the call or off a result of it. The result
// immediately before the record is the other reading -- the file's own continuation, call → result →
// what follows -- which the source's file order supports and the writers take without counting it.
//
// The other two places are counted too, exactly as the tree writers count them: a record the read gave
// an identity but no parent is not the conversation's first, so following the record before it is this
// conversion's relation and not the read's answer (a read that records no identity at all is the other
// case -- its order is linear, and nothing is counted); and a record naming a parent the read does not
// hold follows the record before it, which is likewise this conversion's place.
func sourceLineParent(messages []Message, byIdentity map[string]int, index int, projected bool) (int, bool) {
	parent := strings.TrimSpace(messages[index].SourceParentUUID)
	previous := index - 1
	if parent == "" {
		return previous, strings.TrimSpace(messages[index].SourceUUID) != "" && index > 0
	}
	if projected {
		if at, ok := sourceToolResultBefore(messages, index, parent); ok {
			return at, at < previous
		}
	}
	if at, ok := byIdentity[parent]; ok && at < index {
		return at, false
	}
	return previous, true
}

// sourceToolResultBefore answers the record a tool-result step stands for: the nearest record before
// index that is a tool result answering the record parent names. It is the writers' own projection rule,
// so it is consulted only where the read may answer projections: the inherited Pi reader resolves a
// result's descendants to their visible ancestor, so a record the read answers as a child of a call may
// in the file have hung off a result of it, and the file runs call → result → what follows. An exact
// read's answer names the record it means (a Claude transcript's parentUuid), so no step is taken there:
// a result standing beside a record the read chains to the same message is a *sibling* of it, and
// chaining through the sibling would place the record under a branch the source did not record.
func sourceToolResultBefore(messages []Message, index int, parent string) (int, bool) {
	for at := index - 1; at >= 0; at-- {
		if len(messages[at].ToolResults) > 0 &&
			strings.TrimSpace(messages[at].SourceParentUUID) == parent {
			return at, true
		}
	}
	return 0, false
}

// unresolvedSourceParentsDetail words the caveat both tree-reproducing writers report for a relation the
// read could not answer. The entries and their order are unaffected by it -- only the source's own place
// in the tree is not the one written -- so the text is the same for either writer, and keeping one copy
// is what stops the two from drifting apart.
func unresolvedSourceParentsDetail() string {
	return "a relation the read could not place exactly: the source named a record this read does not " +
		"hold (a record with no identity of its own, or a parent outside the read); recorded no parent " +
		"for a record that is not the conversation's first; or named the message that made a call where " +
		"a result of that call stands between the two records in the file -- the read answers such a " +
		"record as hanging off the call (a result's descendants resolve to their visible ancestor), " +
		"while the source's own file runs call → result → what follows, so which of the two the record " +
		"hung off is not answerable and the entry is chained through the result. These entries follow " +
		"the entry before them, or the call's own tool result, which is a relation this conversion " +
		"invented. The entries and their order are unaffected; what the count names is that the " +
		"source's own place in the tree is not the one written"
}

// ErrUnrepresentableLeaf reports a source whose selected leaf the target cannot be given: the target's own
// loader walks from the last entry a conversion wrote and the source's from the read's last record, so
// when the entry written last stands for a record off the line the read's last record answers, a resumed
// target session would continue a branch the source's conversation left -- a context substitution that no
// per-record count discloses. The conversion refuses rather than writing it.
var ErrUnrepresentableLeaf = errors.New(
	"the source's own loader selects a leaf this conversion cannot write as the target's")

// selectedLeafRefusal answers whether the entry a tree-reproducing conversion wrote last is one the
// source's own loader reaches. The check is the read's own line, recorded by the same walk the writers
// use (provider selects the same reading the tree does): the record the last entry stands for is on it,
// or the conversion wrote an off-line branch's record as the file's last entry and the target would
// resume from there. An empty identity (a record the read answered none for) or a record the read does
// not hold is not this case: nothing was answered about its place, and the writers already follow the
// read's own order there.
func selectedLeafRefusal(messages []Message, lastWrittenSourceID, provider string) error {
	lastWrittenSourceID = strings.TrimSpace(lastWrittenSourceID)
	if lastWrittenSourceID == "" || len(messages) == 0 {
		return nil
	}
	// The refusal is about the line, not about the count: the walk's own unresolved answer names the
	// relations the conversion placed, and the leaf check only asks whether the entry written last is on
	// the line that walk reached.
	inactive, _ := inactiveSourceRecords(messages, provider)
	for index, message := range messages {
		if strings.TrimSpace(message.SourceUUID) != lastWrittenSourceID {
			continue
		}
		if inactive[index] {
			return fmt.Errorf("%w: %s, the record its last entry stands for, is off the line the read's "+
				"last record answers, so a resumed session would continue a branch the source's "+
				"conversation left", ErrUnrepresentableLeaf, lastWrittenSourceID)
		}
		return nil
	}
	return nil
}

// preBoundaryHistoryDetail words the caveat both tree-reproducing writers report for the turns carried
// before a compaction boundary. Whether the target rebuilds its model-visible history from the boundary
// depends on where the source recorded it: on the source's own line the target applies the boundary (a
// resumed turn cannot answer from the pre-boundary turns), while a boundary the source recorded off that
// line -- standing beside a branch the conversation left -- stands off the target's walk exactly as the
// source's own file holds it, and the turns on the line remain the target's history.
func preBoundaryHistoryDetail() string {
	return "the entries before a compaction boundary stay in the file, which is what the source's own " +
		"compaction leaves behind. Where the source recorded the boundary on its own line, the target " +
		"rebuilds its model-visible history from the boundary's summary (measured: a resumed turn could " +
		"not answer from the pre-boundary turns), so those turns are recorded history the target does " +
		"not continue from; where the source recorded it off that line, the boundary stands off the " +
		"target's walk beside the branch it belongs to, as the source's own file holds it, and the turns " +
		"on the line remain the target's history. The count names how many turns were carried before the " +
		"boundary"
}

// installFile writes one converted session into a target data directory and returns the path it
// occupies, plus the mechanism that put it there. The route names only what differs: the subdirectory it
// installs under, the project directory inside it, the file name, the bytes and the mode.
//
// Nothing existing is ever replaced: the bytes go to a sibling temporary file first and the final name is
// taken with a hard link, which is atomic where the filesystem has hard links and exclusive everywhere,
// so a taken name fails instead of being overwritten. A filesystem without hard links (some Windows
// volumes, some network mounts) has no exclusive way to publish the finished file, so the installation is
// refused (ErrNoExclusiveInstall) rather than published by rename: a rename replaces whatever is at the
// destination, which is exactly the overwrite this rule forbids. A reader therefore never sees a
// half-written session, and a conversion cannot replace a session another agent may already have
// continued.
//
// The returned path and the error are independent answers. A non-empty path means the session is
// published, and a non-nil error beside it is about something other than the publication itself -- a
// temporary file that could not be removed, reported rather than left behind in silence. A call that
// published nothing answers with an empty path, so the caller reads the two apart instead of inferring
// publication from the error.
func installFile(home, subdir, projectDir, fileName string, data []byte, mode os.FileMode) (string, string, error) {
	directory := filepath.Join(home, subdir, filepath.FromSlash(projectDir))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", directory, err)
	}
	target := filepath.Join(directory, fileName)
	if _, err := os.Lstat(target); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("checking %s: %w", target, err)
	}

	temp, err := os.CreateTemp(directory, "."+fileName+".tmp-")
	if err != nil {
		return "", "", fmt.Errorf("creating a temporary session file in %s: %w", directory, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		removeFile(tempName)
	}
	if err := temp.Chmod(mode); err != nil {
		cleanup()
		return "", "", fmt.Errorf("restricting %s: %w", tempName, err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return "", "", fmt.Errorf("writing %s: %w", tempName, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", "", fmt.Errorf("flushing %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		removeFile(tempName)
		return "", "", fmt.Errorf("closing %s: %w", tempName, err)
	}

	if err := linkSession(tempName, target); err != nil {
		removeFile(tempName)
		if errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
		}
		return "", "", fmt.Errorf("%w: %s: %v; install into a filesystem that supports hard links",
			ErrNoExclusiveInstall, directory, err)
	}
	if err := removeFile(tempName); err != nil {
		return target, "hard-link", fmt.Errorf("removing the temporary file %s: %w", tempName, err)
	}
	return target, "hard-link", nil
}

// verifyStructure reads an installed session back through the inherited AgentsView reader the route
// names and compares it, item by item, with what the writer intended to carry. sessionsDir is the
// target's own directory; targetPath is the file the conversion published (or the dry run's copy of the
// same bytes); cfg supplies the provider configuration (its roots for the route's agent are replaced,
// never mutated: the map is copied); compare walks the read-back against the file's own bytes with the
// route's reader rules.
//
// The read-back is the file this call names, read through the inherited reader's own path reference, so
// the check compares the file the conversion published rather than resolving an id through the target's
// session tree, which could answer with another copy of it (an id lookup enumerates every dated
// directory under the root and prefers the lexically first copy of the id, so an unrelated copy -- or
// simply a home with a long history -- would decide what is compared and how much of the home is walked).
func verifyStructure(
	ctx context.Context,
	sessionsDir string,
	agent parser.AgentType,
	method, readerMessage, readerLabel, targetPath string,
	compare func(readBack Conversation, installed []byte) roundTripComparison,
	cfg config.Config,
) (StructureVerification, error) {
	verification := StructureVerification{
		State:      VerificationStateNotVerified,
		Method:     method,
		TargetPath: targetPath,
	}
	readerConfig := cfg
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for name, roots := range cfg.AgentDirs {
		dirs[name] = roots
	}
	dirs[agent] = []string{sessionsDir}
	readerConfig.AgentDirs = dirs
	readerConfig.DisabledAgents = nil

	conversation, err := NewSourceReader(readerConfig).Read(ctx,
		Ref{Provider: string(agent), ID: verification.TargetPath})
	if err != nil {
		verification.Detail = "the inherited " + readerMessage + " reader could not read the installed session back: " + err.Error()
		return verification, nil
	}
	verification.ReadBack = readBackCounts(conversation)
	installed, err := os.ReadFile(verification.TargetPath)
	if err != nil {
		verification.Detail = "the installed session could not be read back for comparison: " + err.Error()
		return verification, nil
	}
	roundTrip := compare(conversation, installed)
	verification.NotReadBackVerbatim = roundTrip.notReadBackVerbatim
	if len(roundTrip.differences) > 0 {
		verification.Detail = "the installed session does not hold the evidence in the target document: " +
			strings.Join(roundTrip.differences, "; ")
		return verification, nil
	}
	verification.State = VerificationStateVerified
	verification.Detail = roundTrip.detail(readerLabel)
	return verification, nil
}

// plainRoundTripReader is the inherited reader's own rules for the read-back walk as the Codex and Pi
// readers share them: it marks no record as injected context (neither reader has such a classification),
// a tool record is no message of its own (the reader pairs the tool's output onto the call's message),
// and any other role the reader reads dialogue for stands for the assistant's turn -- the reader's own
// answer, which the check reports rather than second-guesses. The Claude reader's rules differ (it
// classifies injected context away and knows only the user and assistant roles) and are spelled per
// route.
func plainRoundTripReader() roundTripReader {
	return roundTripReader{
		classifiesAway: func(Message) bool { return false },
		kindOf: func(message Message) (TargetItemKind, bool) {
			switch parser.RoleType(message.Role) {
			case parser.RoleUser:
				return ItemUser, true
			case parser.RoleTool:
				return "", false
			default:
				return ItemAssistant, true
			}
		},
	}
}

// probeBinary finds a route's harness executable and asks it for its version: the shape the three routes'
// probes share. A missing binary is an answer, not an error: a conversion can still be written (and
// verified structurally), but the report says the native half could not run. versionOf reads the version
// out of the `--version` answer, and noVersion words the sentence the probe reports when that answer
// names none -- both the route's own, so each route's report reads as it always did.
func probeBinary(
	ctx context.Context, binary, defaultBinary string,
	versionOf func(output string) string,
	noVersion func(path, output string) string,
) harnessProbe {
	if strings.TrimSpace(binary) == "" {
		binary = defaultBinary
	}
	probe := harnessProbe{Binary: binary}
	path, err := exec.LookPath(binary)
	if err != nil {
		probe.Detail = fmt.Sprintf("%s is not on PATH", binary)
		return probe
	}
	probe.Found = true
	probe.Binary = path
	output, err := runBounded(ctx, path, 30*time.Second, "--version")
	if err != nil {
		probe.Detail = fmt.Sprintf("%s --version failed: %v", path, err)
		return probe
	}
	if version := versionOf(output); version != "" {
		probe.Version = version
		return probe
	}
	probe.Detail = noVersion(path, output)
	return probe
}

// childEnvironment builds a child environment for one harness run: the caller's own environment with the
// variable that selects the route's data directory (homeEnv) set to home, every name in set set to its
// own value, the named variables removed, and any further names in alsoDeny removed too. Nothing else is
// changed, so a host's PATH, locale and tool configuration still reach the harness.
//
// Every name is read in its own upper case, the way the denylist and os/exec itself read one: Windows'
// environment is case-insensitive and os/exec keeps the last value among the names it is handed, so a
// leftover lower- or mixed-case spelling of a set variable would, after the sort below, sit after the
// appended override and win there -- the check would then read the caller's own value instead of the one
// it means to run with.
func childEnvironment(home, homeEnv string, denied []string, set map[string]string, alsoDeny ...string) []string {
	blocked := make(map[string]struct{}, len(denied)+len(alsoDeny)+len(set))
	for _, name := range denied {
		blocked[strings.ToUpper(name)] = struct{}{}
	}
	for _, name := range alsoDeny {
		blocked[strings.ToUpper(name)] = struct{}{}
	}
	for name := range set {
		blocked[strings.ToUpper(name)] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+1+len(set))
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		folded := strings.ToUpper(name)
		if _, deny := blocked[folded]; deny {
			continue
		}
		if folded == homeEnv {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, homeEnv+"="+home)
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		environment = append(environment, name+"="+set[name])
	}
	sort.Strings(environment)
	return environment
}

// runRouteTurn runs one non-interactive harness invocation and returns its stdout and stderr separately,
// both bounded. The command runs in cwd when it exists, because the harness's own lookup resolves a
// session through the directory it is started in, and runs with the environment the caller passes (the
// caller's own with the route's data-directory variable set and the model-reaching names removed).
func runRouteTurn(
	ctx context.Context, executable, cwd string, args []string, environment []string,
	timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, executable, args...)
	// The directory is the source's own recorded value: trimming decides only whether one was recorded,
	// because a directory name may end in whitespace.
	if dir := cwd; strings.TrimSpace(dir) != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			command.Dir = dir
		}
	}
	command.Env = environment
	answer := newBoundedBuffer(harnessOutputMax)
	diagnostics := newBoundedBuffer(harnessDiagnosticsMax)
	command.Stdout = answer
	if stderr != nil {
		command.Stderr = io.MultiWriter(diagnostics, stderr)
	} else {
		command.Stderr = diagnostics
	}
	err := command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return answer.String(), diagnostics.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return answer.String(), diagnostics.String(), err
}

// resolveRouteHome answers the data directory a route's conversion installs into: the caller's explicit
// directory, else the route's environment variable, else its default under the user's home. The boolean
// reports whether the answer came from the caller, which is what decides whether a missing directory may
// be created.
//
// A value is read to answer two things, and only the first of them trims: trimming decides whether a
// directory was named at all, and the value is then resolved exactly as it was written, because a
// directory name may begin or end with a space.
func resolveRouteHome(explicit, homeEnv string, defaultDir ...string) (string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		home, err := filepath.Abs(explicit)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s: %w", explicit, err)
		}
		return home, true, nil
	}
	if fromEnv := os.Getenv(homeEnv); strings.TrimSpace(fromEnv) != "" {
		home, err := filepath.Abs(fromEnv)
		if err != nil {
			return "", false, fmt.Errorf("resolving $%s: %w", homeEnv, err)
		}
		return home, false, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolving the home directory: %w", err)
	}
	return filepath.Join(append([]string{dir}, defaultDir...)...), false, nil
}
