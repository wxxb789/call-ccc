package ccc

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
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

// harnessOutputMax and harnessDiagnosticsMax bound what one harness *turn* may add to this process's
// memory, and the diagnostics an RPC arm keeps. The answer a report quotes is cut to boundedText's own
// limit anyway (8192 bytes by default), and a harness flooding either stream is answered by its deadline
// and an excerpt rather than by this process growing with every byte it writes. The diagnostics bound is
// the size the Codex app server keeps of its own, 4096 bytes.
//
// An RPC arm's *protocol answers* are not bounded here: the Pi load check reads its harness's answers
// incrementally, retaining only the fields it asks about (readPiResponses), because a `get_messages`
// answer carries a whole conversation's contents and an excerpt of one is not an answer to the check.
const (
	harnessOutputMax      = 1 << 20
	harnessDiagnosticsMax = 4096
)

// entryClock hands out timestamps that never go backwards: each target orders its records by the
// timestamp it records, and a source whose messages are unordered (a DAG, a replayed fork) would
// otherwise write a file whose order contradicts its own times. The layout is the route's own: the Codex
// and Claude routes write `time.RFC3339Nano`, and Pi writes milliseconds, always three digits, always
// UTC (`2026-11-10T02:07:01.784Z`, piEntryTimestampLayout).
//
// A record whose own *recorded* instant precedes the instant already written is stamped with that
// instant instead, and counted (`substituted`): the document then carries an instant the read did not
// answer for that record, and the report names the substitution (`substituted_record_instants`) rather
// than letting the written record read as the source's own answer. A record the read answered no
// instant for at all is not counted as a substitution: there is no recorded instant to replace, and the
// conversion instant the options document ("used for identities and for a source that recorded no
// times") is what such a record is stamped with.
type entryClock struct {
	last   time.Time
	layout string
	// substituted counts the written timestamps whose recorded instant the document does not carry: each
	// call with a record's own earlier instant replaces one written timestamp, so a record the writer
	// stamps as several entries (Codex writes a turn context, a response item and a view event for one
	// message) is counted on each of its entries rather than once for the record.
	substituted int
}

func newEntryClock(start time.Time, layout string) *entryClock {
	return &entryClock{last: start.UTC(), layout: layout}
}

func (c *entryClock) next(at time.Time) string {
	at = at.UTC()
	if at.IsZero() {
		at = c.last
	} else if at.Before(c.last) {
		c.substituted++
		at = c.last
	}
	c.last = at
	return at.Format(c.layout)
}

// substitutedRecordInstantsDetail words the caveat the clock-bearing writers report for a record whose
// own recorded instant the document does not carry. The instant written in the recorded one's place is
// always another instant the read recorded -- the previous written record's, the highest the document
// already holds -- never one this build invented, and the entries and their order are unaffected; only
// the one instant is. The text is the same for every writer that shares the clock, and keeping one copy
// is what stops the routes from drifting apart.
func substitutedRecordInstantsDetail() string {
	return "the read recorded an instant for each of these records, and the document does not hold " +
		"it: the target orders its records by the timestamp it writes, so a record whose own instant " +
		"precedes the instant already written is stamped with the previous written record's instant " +
		"instead -- an instant the read recorded for a record, never one this build invented -- and " +
		"the record's own instant is in the source session alone. The count is per written timestamp, " +
		"not per source record: a record the writer stamps as several entries (a turn context, a " +
		"response item and a view event for one message) carries the substituted instant on each of " +
		"them, and each of those written timestamps is counted"
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

// resultStatesAnOutcome answers whether one recorded result occurrence states an outcome, as opposed to
// the progress a read streams while a call is still executing. The words are the inherited readers' own:
// internal/parser/termination.go treats any status other than `running` as the call being resolved (a
// Codex subagent's pending notification stream), the RooCode/Kilo chunk readers record streamed chunks as
// `running` and flip the last one to the aggregate's own final status, and Copilot's and OpenCode's own
// call events spell the same state `started`/`pending`/`streaming` (internal/parser/copilot.go,
// internal/parser/opencode_v2.go). Anything else -- `completed`, `errored`, a provider's own word, or no
// status at all -- is a recorded outcome. The selection consults it to decide whether a call was left
// undischarged, and the OpenCode writer to decide whether one event is the call's outcome; either way a
// target's own status rule (OpenCode's toolPart) then decides what state, if any, it writes.
func resultStatesAnOutcome(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "pending", "streaming", "started", "in_progress":
		return false
	default:
		return true
	}
}

// toolResultCarrierRecord answers the source record a record hanging off this message's outcome would
// read as its parent, where the read may answer a projection, so that a later record naming that source
// record is the place the outcome's entry stands for.
//
// The answer is decided by whether the message itself carries a call at all: an outcome the read folded
// onto the call's own message -- the archive's projection attaches a result-only row to the message that
// made the call and drops the row -- sits on a message that carries calls, and a record the source's file
// placed under that outcome reads that message's own identity as its parent; a result record of its own
// carries no call (the reader pairs its result with a call of another record), and the reader answers a
// result's descendants with the result's visible ancestor, which is the parent the result record was
// given. The answer registers the outcome's entry only for a read that may project (sourceTree.wrote); an
// exact read's answers name the record they mean.
func toolResultCarrierRecord(message Message) string {
	if len(message.ToolCalls) > 0 {
		return strings.TrimSpace(message.SourceUUID)
	}
	return strings.TrimSpace(message.SourceParentUUID)
}

// sourceRecordOrigin is what one written entry stands for in the source's own record stream: the
// record's identity, the record *it* hangs off as the read answered it, the entry this conversion
// chained the record's first entry to, and -- for an entry written as a tool result -- the source record
// a record hanging off that result would read as its parent (toolResultCarrierRecord), empty for every
// other entry. The entry the next record hangs off is decided from it, where the read may answer a
// projection: a record that follows a tool result in the source's file hangs off that result while the
// read answers it with the message that made the call, and an exact read answers where the record hangs
// off itself.
type sourceRecordOrigin struct {
	sourceID          string
	sourceParent      string
	parentID          *string
	toolResultCarrier string
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
	// omittedAncestorEntry maps a source record the target left out -- a record that produced no entry
	// of its own -- to the entry its first entry would have been chained to: its nearest written
	// ancestor as the conversion *resolved* it when the record was walked, which for a record standing
	// under a written tool result is the result's own entry and not the nearest ancestor of the record's
	// raw relation (endRecord registers it; writtenAncestor answers with it). An omitted record has no
	// entry to chain a child to, and re-resolving the child's relation from the raw one would drop the
	// place the omitted record was resolved to -- measured: with a Pi chain call → result → empty
	// assistant → next turn, the reader answers the empty record as hanging off the call, and a writer
	// that re-resolved the turn's raw relation chained it straight to the call, leaving the written
	// result off the walk the target's loader makes.
	omittedAncestorEntry map[string]string
	// toolResultEntry maps the source record a record hanging off a written tool result would read as
	// its parent (toolResultCarrierRecord) to the entry of the most recent such result, and is filled
	// only when projectedParents holds. The read answers a record under a tool result as hanging off the
	// message that made the call, so a later record naming that source record may in the source's own
	// file have hung off the result; the result's own entry is where such a record is chained, and the
	// relation is counted (unresolved_source_parents) because which of the two the record named is not
	// answerable from the read. Keying this by the carrier -- and not by the parent of the record the
	// result entry was written while walking -- is what keeps a call's outcome from being registered
	// under the record the call's message hangs off: that reading chained a sibling of the call through
	// the abandoned call's result, reviving the call branch as the target's active history.
	toolResultEntry map[string]string
	// projectedParents says the read may answer a projection: projectedParentProvider of the source's own
	// provider. A read whose answers are exact keeps the narrower reading below, which invents nothing.
	projectedParents bool
	// preambleEntryID is the entry a family writes before the conversation: a record of its own format
	// that is not a source record and that the source's own first record hangs off. OMP's session model
	// binding is one (measured: a session whose binding entry nothing chains to is not restored, so the
	// entry has to stand on the walk the target's loader makes from its leaf), and Pi has none. It is a
	// place, not a record: nothing is registered for it, no source relation is counted through it, and
	// lastEntryID therefore stays nil until a source record is written -- which is what keeps every
	// per-record count the same on a target whose format has no preamble.
	preambleEntryID *string
	// lastEntryID is the entry written last and lastOrigin what it stands for; pending is what the
	// entries written next stand for.
	lastEntryID *string
	lastOrigin  sourceRecordOrigin
	pending     sourceRecordOrigin
	// unresolved counts the source relations this conversion could not place: the entries written for
	// their records follow the entry before them, which is a relation this build invented.
	unresolved int
	// entryByRecord maps each record of the document this conversion walks (its own position in
	// Messages) to the final entry written for it, and omittedEntryByRecord the place resolved for a
	// record the target left out (the entry its first entry would have been chained to, as resolved when
	// its walk started). They are the index-keyed counterparts of writtenEntryID and omittedAncestorEntry,
	// and where the kept-ancestor answer is consulted they are the ones read: that answer is an index
	// (Conversation.sourceKeptAncestor), because the read answers no identity for some records -- the
	// inherited Claude reader preserves `parentUuid` and `uuid` independently -- and a record without one
	// must keep its answered ancestor all the same.
	entryByRecord        map[int]string
	omittedEntryByRecord map[int]string
	// inactive carries, per record of the document this conversion walks, whether the read's own line
	// leaves it off it (sourceLineFlags: the read's answer kept through a selection, or the read's own
	// walk membership for a document a selection produced -- the flags alone never mark an identity-less
	// record, and the membership is the only answer such a record's place has). It is keyed by the
	// record's own walk position for the same reason entryByRecord is: the fallback's revival check and
	// the off-line-entry registration ask about a record that may carry no identity.
	inactive []bool
	// record is the position in Messages of the record being walked -- -1 before the first record, which
	// is what a family's own preamble or header entry is written under -- and records counts the records
	// started, so each startRecord stamps the record's own position.
	record  int
	records int
	// keptAncestor carries the selection's own ancestry answer, per record of the document this conversion
	// walks: the position of the nearest ancestor the read holds that the selection also carried (or -1),
	// beside whether resolving it crossed the ambiguous tool-result projection. It is empty for a read that
	// was not reduced, and reduced says whether the document is one a selection produced -- which is also
	// what gates the revived-branch check below, so an unflagged conversion keeps the writers'
	// long-standing answer for a relation the read cannot place.
	keptAncestor []keptAncestorAnswer
	reduced      bool
	// offLineEntry maps each entry written for a record the read's line leaves off it to that record's own
	// position in the document this conversion walks, and revivedChild/revivedParent name the first
	// relation the fallback resolved from a record on the line onto such an entry: the child whose entry
	// would hang there (its own position in Messages) and the off-line record whose entry it is (its own
	// position too, because the read answers no identity for some records and the off-line answer the
	// check reads is per position: a record the read answered no identity for must be refused like any
	// other). A negative revivedChild means no such relation was resolved; a non-negative one is what the
	// leaf guard refuses (selectedLeafRefusal), because following the entry written before the child would
	// put an abandoned record on the walk a resumed session makes.
	offLineEntry  map[string]int
	revivedChild  int
	revivedParent int
	// entryPlaces is where each written entry stands: the entry it hangs off -- the place its record's
	// first entry resolved, else the entry written before it, entryParent's own answer -- and the position
	// of the record it was written for (-1 for an entry a family writes before the conversation). It is
	// what lets a writer show a recorded tool result's paired call on the result's written ancestry
	// (recordOnWrittenPath): the chain of parents an entry's place names is the walk the target's loader
	// makes from it.
	entryPlaces map[string]writtenEntryPlace
	// recordPlace is the place each walked record hangs off, keyed by the record's own position
	// (startRecord's own resolution): pending.parentID is cleared by the record's first written entry, so
	// a check asked later in the record's own walk -- recordOnWrittenPath, from a result written after the
	// record's dialogue entry -- still needs the place it started from.
	recordPlace map[int]string
}

// writtenEntryPlace is one written entry's place in the target's own tree: the entry it hangs off (empty
// for an entry nothing chains to) and the position of the source record it was written for.
type writtenEntryPlace struct {
	parent string
	record int
}

func newSourceTree(conversation Conversation) sourceTree {
	return sourceTree{
		sourceRecordParents:  map[string]string{},
		writtenEntryID:       map[string]string{},
		omittedAncestorEntry: map[string]string{},
		toolResultEntry:      map[string]string{},
		entryByRecord:        map[int]string{},
		omittedEntryByRecord: map[int]string{},
		offLineEntry:         map[string]int{},
		entryPlaces:          map[string]writtenEntryPlace{},
		recordPlace:          map[int]string{},
		inactive:             sourceLineFlags(conversation),
		keptAncestor:         conversation.sourceKeptAncestor,
		projectedParents:     projectedParentProvider(conversation.Source.Provider),
		reduced:              conversation.sourceKeptAncestor != nil,
		record:               -1,
		revivedChild:         -1,
		revivedParent:        -1,
	}
}

// startRecord names the source record the entries written next stand for and resolves the entry its
// first entry hangs off from the source's own relation. The record's own position in the document is
// stamped here, because the places a selection answered are keyed by position: a record the read answered
// no identity for is a record like any other for that answer.
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
	t.record = t.records
	t.records++
	parentID, counted := t.parentPlace(id, parent)
	if counted {
		t.unresolved++
	}
	if parentID != nil {
		t.recordPlace[t.record] = *parentID
	}
	t.pending = sourceRecordOrigin{sourceID: id, sourceParent: parent, parentID: parentID}
}

// endRecord ends the record's walk, so a record that produced no entry cannot leak its relation onto the
// next record's first entry.
//
// A record the target leaves out produced no entry of its own, but it still had a place resolved for it
// (startRecord): the entry its first entry would have been chained to. That place is kept under the
// record's own identity and under its own position, because a later record naming it must hang off the
// same place -- the omitted record has no entry of its own for the child to follow, and re-resolving the
// child's raw relation can only reach a *different* entry where the place was resolved through a written
// tool result. The registration is exact rather than conditional: parentID is non-nil only when nothing
// was written for the record (wrote clears it), so a record with entries of its own keeps its entry as
// the place its children hang off, through writtenEntryID and entryByRecord. The position-keyed
// registration is what a selection's ancestry answer reads when the omitted record is the ancestor it
// named and that record carries no identity of its own.
func (t *sourceTree) endRecord() {
	if t.pending.parentID != nil {
		if t.pending.sourceID != "" {
			t.omittedAncestorEntry[t.pending.sourceID] = *t.pending.parentID
		}
		if t.record >= 0 {
			t.omittedEntryByRecord[t.record] = *t.pending.parentID
		}
	}
	t.pending = sourceRecordOrigin{}
}

// markToolResult says the entry about to be written is a tool result, carrying the source record a
// record hanging off that result would read as its parent (toolResultCarrierRecord). Where the read may
// answer a projection, a later record naming that source record then hangs off the result rather than
// off the message that made the call, because that is the order the source's own file holds
// (call → result → what follows) and the other reading would put the result off the walk the target's
// loader makes.
func (t *sourceTree) markToolResult(carrierRecord string) {
	t.pending.toolResultCarrier = strings.TrimSpace(carrierRecord)
}

// entryParent answers the parent the entry about to be written carries: the entry the source's relation
// resolved for the record's first entry, else the entry written last, because a record written as
// several entries is one place in the tree and its later entries follow each other.
func (t *sourceTree) entryParent() *string {
	if t.pending.parentID != nil {
		return t.pending.parentID
	}
	return t.priorEntry()
}

// preamble registers the entry a family wrote before the conversation (preambleEntryID) as the place
// the source's own first record hangs off, so the record's entry is reachable on the walk the target's
// loader makes from the file's last entry. The entry itself is written by the family, not here: this
// records only where it stands, and nothing about it is counted as a source relation.
func (t *sourceTree) preamble(entryID string) {
	id := strings.TrimSpace(entryID)
	if id == "" {
		return
	}
	t.preambleEntryID = &id
}

// priorEntry answers the entry a record's first entry follows when the source's own relation resolves
// none: the entry written last, else the family's own preamble entry. The second case is a document
// whose first record has no relation of its own -- on a family whose format carries a record before the
// conversation, the first record's entry follows that record, and the alternative (a null parent) would
// leave the preamble off the walk the target's own loader makes, which on OMP is a session whose model
// binding is never restored (measured).
func (t *sourceTree) priorEntry() *string {
	if t.lastEntryID != nil {
		return t.lastEntryID
	}
	return t.preambleEntryID
}

// wrote records one written entry: it is the entry the record being walked stands for -- the last of
// them, overwriting the maps so the record's *final* entry is the place its children hang off -- and the
// parent the next entry written for that record follows. The record's own position is stamped beside its
// identity, so the place a selection answered for a record the read gave no identity resolves all the
// same; an entry a family writes before the conversation (its preamble or header) has no position of its
// own and registers under none.
func (t *sourceTree) wrote(id string) {
	// The entry's place is recorded before the walk advances: the entry it hangs off -- the place the
	// record's first entry resolved, else the entry written before it, which is entryParent's own answer,
	// taken here before lastEntryID moves -- and the position of the record it was written for.
	parent := ""
	if t.pending.parentID != nil {
		parent = *t.pending.parentID
	} else if prior := t.priorEntry(); prior != nil {
		parent = *prior
	}
	t.entryPlaces[id] = writtenEntryPlace{parent: parent, record: t.record}
	t.lastEntryID = &id
	if t.record >= 0 {
		t.entryByRecord[t.record] = id
	}
	if t.pending.sourceID != "" {
		t.writtenEntryID[t.pending.sourceID] = id
	}
	if t.record >= 0 && t.record < len(t.inactive) && t.inactive[t.record] {
		// The entry stands for a record the read's line leaves off it. It is registered by the record's
		// own position, not its identity: the read answers no identity for some records, and the
		// revival check has to refuse a record the line holds chained under such an entry exactly as
		// under an identified one.
		t.offLineEntry[id] = t.record
	}
	if t.projectedParents && t.pending.toolResultCarrier != "" {
		t.toolResultEntry[t.pending.toolResultCarrier] = id
	}
	t.lastOrigin = t.pending
	t.pending.parentID = nil
	t.pending.toolResultCarrier = ""
}

// parentPlace answers the entry the record's first entry is chained to, and whether that place is a
// relation this conversion invented -- which startRecord counts (unresolved). The source's own relation
// decides, not the order the file happens to hold its records in: a target format that is a tree is
// written as the source's own tree, so an entry follows the record it hangs off and a branch the
// conversation left stays off the path the target's own loader walks from the file's last entry. The
// places, in order:
//
//   - the record the source named, resolved through the records the document holds (writtenAncestor): the
//     entry this conversion wrote for it, or -- for a record the target left out -- the place resolved for
//     it when it was walked (omittedAncestorEntry: for a record standing under a written tool result that
//     is the result's own entry, which the raw walk alone cannot reach), else the nearest written
//     ancestor walked through the document's own relations. Not counted.
//   - the place a written tool result stands for (toolResultEntry), where the read may project: the
//     source's own file runs call → result → what follows while the read answers the next record as
//     hanging off the call. Always counted: the read answers a record the source placed under the call
//     and one it placed under the call's result with the same parent, so which of the two it names is not
//     answerable from the read; the entry is chained through the result -- the reading the source's own
//     file order supports -- with the count naming the relation as one the read did not establish. Only a
//     read that may project can reach this place (wrote, projectedParents), so an exact read's answers
//     name the records they mean. A call's outcome the read folded onto the call's own message is what the
//     carrier of a written result is answered from, so a sibling of the call names the call's message's
//     *parent*, not the message, and is chained to that parent's own entry instead of through the call's
//     result: only a record naming the call's own message can be a record the file might have placed
//     under the outcome.
//   - the nearest retained ancestor a selection answered for the record (sourceKeptAncestor): the
//     record's own relation steps through records the window left out to an ancestor the window carries,
//     and the entry hangs off that ancestor's place. Not counted where the answer rests on the read's own
//     relations alone -- the ancestors and the exclusion are the read's own answers, disclosed by the
//     selection report -- but counted where resolving the answer crossed the ambiguous tool-result
//     projection (keptAncestorAnswer.projected), exactly as the place above is counted: the read answers a
//     record under a call and one under that call's result the same way, so the chain the writer hangs
//     there is a relation this conversion resolved rather than one the read named. This is the place a
//     reduced tree hangs a record whose own parent the selection left out -- without it the entry would
//     follow the entry before it and could put a branch the conversation left on the walk. An inactive
//     ancestor answers only for a record the read's own line also leaves off it: keeping a branch off the
//     active walk is recorded history the writer reproduces, but chaining an *on-line* record through an
//     off-line ancestor is the revival the fallback's check refuses.
//   - the entry written before it (priorEntry), counted when a relation was recorded but could not be
//     placed -- a record outside the read, a relation no walk can complete -- or when the record named no
//     parent and is not the conversation's first. A record that names no parent at all is read as the
//     read's own order: this conversion cannot tell a record the source placed at the head of a line from
//     one whose identity or parent the provider did not write, and chaining it to the entry before it
//     keeps a record the read *did* carry visible, where a written null could cut the conversation into
//     pieces the target's loader never walks. A reduced document whose selection left a record's whole
//     ancestry out takes this place too -- a record the read answered no identity for included, because
//     the guard asks by the record's own position -- and where that entry stands for an off-line record
//     the relation is noted as the revived branch the guard refuses (fallbackPlace).
func (t *sourceTree) parentPlace(id, parent string) (*string, bool) {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		// The record named no parent: the entry written before it is the read's own order, and the
		// fallback's own check decides whether that place is usable at all -- for a record the read
		// answered no identity for exactly as for any other, because on a reduced document a record whose
		// whole retained ancestry the selection left out would otherwise be chained to the entry of an
		// off-line record and put that branch back on the walk a resumed session makes. Only the count
		// differs: a record the read gave an identity but no parent is a relation this conversion invented
		// (counted where an entry was already written), while an identity-less record's place is the
		// read's own order, which invents nothing.
		return t.fallbackPlace(), id != "" && t.lastEntryID != nil
	}
	if entry, ok := t.toolResultEntry[parent]; ok {
		return &entry, true
	}
	if entry, ok := t.writtenAncestor(parent); ok {
		return &entry, false
	}
	if entry, projected, ok := t.keptAncestorEntry(); ok {
		return &entry, projected
	}
	return t.fallbackPlace(), true
}

// keptAncestorEntry answers the written place of the nearest retained ancestor a selection answered for
// the record being walked (Conversation.sourceKeptAncestor, keyed by the record's own position), whether
// that place is a relation this conversion resolved rather than one the read named (the walk crossed the
// ambiguous tool-result projection), and whether it answers at all. It answers nothing for a read that
// was not reduced (the answers are not carried then), for a record the read answered no ancestor for, and
// for an ancestor the read's own line leaves off it where the record being walked is *on* that line --
// hanging the record there would put an off-line record on the walk a resumed session makes, and the
// fallback's own check then answers whether the entry before it is usable at all. A record the line
// leaves off the walk may keep such an ancestor: the branch it hangs off is the read's own recorded
// history, and rewriting it to another entry would change a relation the source did record.
func (t *sourceTree) keptAncestorEntry() (string, bool, bool) {
	if t.record < 0 || t.record >= len(t.keptAncestor) {
		return "", false, false
	}
	answer := t.keptAncestor[t.record]
	if answer.index < 0 || answer.index >= len(t.keptAncestor) {
		return "", false, false
	}
	if t.inactiveAt(answer.index) && !t.inactiveAt(t.record) {
		return "", false, false
	}
	if entry, ok := t.entryByRecord[answer.index]; ok {
		return entry, answer.projected, true
	}
	if entry, ok := t.omittedEntryByRecord[answer.index]; ok {
		return entry, answer.projected, true
	}
	return "", false, false
}

// inactiveAt answers whether the read's own line leaves the record at one position of the document this
// conversion walks off it (sourceLineFlags, the read's answer kept through a selection). A position
// outside the document answers false: there is no record there for the line to place.
func (t *sourceTree) inactiveAt(index int) bool {
	return index >= 0 && index < len(t.inactive) && t.inactive[index]
}

// fallbackPlace takes the entry written before the record and, on a reduced document, notes the first
// relation the fallback resolved from a record the read's line holds onto an entry written for one the
// line leaves off it (a record whose whole retained ancestry the selection left out): following it would
// put the abandoned record on the walk a resumed target session makes, which selectedLeafRefusal refuses.
// The check asks by the record's own position, so a record the read answered no identity for is checked
// like any other -- the record's place on the read's own line is answered by position (sourceLineFlags),
// not by an identity the read may never have given. A read that was not reduced is never checked here:
// the writers' long-standing answer for a relation the read cannot place is the entry before it, counted
// and disclosed, and this conversion does not change that answer.
func (t *sourceTree) fallbackPlace() *string {
	entry := t.priorEntry()
	if !t.reduced || entry == nil || t.record < 0 || t.revivedChild >= 0 {
		return entry
	}
	if t.record < len(t.inactive) && t.inactive[t.record] {
		return entry
	}
	if offLineRecord, ok := t.offLineEntry[*entry]; ok {
		t.revivedChild, t.revivedParent = t.record, offLineRecord
	}
	return entry
}

// writtenAncestor answers the place the nearest record a walk from sourceID stands for, and whether it
// found one at all: the entry this conversion wrote for the record, or -- for a record the target left
// out -- the place resolved for it when its own walk started (omittedAncestorEntry), which is the entry
// its first entry would have been chained to. That place is consulted before the raw relation is stepped
// through because it may be one the raw walk alone cannot reach: a record the read answers as hanging off
// a call may in the source's own file have stood under a result of that call, and the place's resolution
// took the result's entry. The walk runs bounded by the records the read answered rather than by a depth
// constant, so a linear history longer than any fixed bound is still walked to its end while a cycle the
// source recorded (a record naming itself or a longer loop) can only walk the relations the read holds
// before the bound ends it -- it cannot keep the conversion going, and the relation is answered as one
// this conversion could not place. No early exit for a record naming itself is kept beside the bound:
// removing one leaves every test green, because the same relation is answered one step later, so an exit
// would state only the loop's iteration count.
func (t *sourceTree) writtenAncestor(sourceID string) (string, bool) {
	for steps := 0; sourceID != "" && steps <= len(t.sourceRecordParents); steps++ {
		if id, ok := t.writtenEntryID[sourceID]; ok {
			return id, true
		}
		if id, ok := t.omittedAncestorEntry[sourceID]; ok {
			return id, true
		}
		next, ok := t.sourceRecordParents[sourceID]
		if !ok {
			return "", false
		}
		sourceID = next
	}
	return "", false
}

// recordOnWrittenPath answers whether the record at one position of the document being written stands on
// the written ancestry of the record now being walked: the walk starts at the place the current record's
// first entry hangs off (startRecord's own resolution, pending.parentID) and steps up the entries this
// conversion wrote (entryPlaces), asking at each entry whether it was written for that record. A call and
// a result on one record are one place, so the record being walked answers for itself; a record the
// target wrote no entry for -- a subagent turn, a record whose kind has no message here, a window the
// target left out -- is on no walk at all, and a call it recorded answers false exactly as the flat
// targets' occurrence check answers a call their line left off. This is the check the tree writers ask
// before accepting a recorded result under the id of the call the seam paired it with: a result whose
// paired call stands on another branch would otherwise be written as that call's outcome while the walk a
// resumed session makes holds only the output, never the invocation.
func (t *sourceTree) recordOnWrittenPath(record int) bool {
	if record < 0 || t.record < 0 {
		return false
	}
	if record == t.record {
		return true
	}
	place := t.pending.parentID
	if place == nil {
		if resolved, ok := t.recordPlace[t.record]; ok {
			place = &resolved
		}
	}
	for steps := 0; place != nil && steps <= len(t.entryPlaces); steps++ {
		at, ok := t.entryPlaces[*place]
		if !ok {
			break
		}
		if at.record == record {
			return true
		}
		if at.parent == "" {
			break
		}
		parent := at.parent
		place = &parent
	}
	return false
}

// inactiveSourceRecords answers, for every message of a read, whether the source's own relations place
// it off the line the source's loader builds context from: the line the walk from the read's *last*
// record reaches. It is the record-level counterpart of the relation the tree writers resolve, for the
// targets whose format is a single line of records -- one where a record written becomes part of the
// conversation a resumed turn reads, so a branch the conversation left cannot travel there as recorded
// history without being revived as an active turn.
//
// The flags answer *off* the line only for the records the read holds an identity for; the second
// answer is the walk's own membership, per record position -- whether the walk reached the record --
// and it holds for identity-less records too, exactly when the read's own relations place them on the
// line (a record the walk steps to by the read's own order included). A caller asking whether a record
// the read answered no identity for stands on the line reads it there, because the flag cannot answer
// for it and a leaf that is a continuation of an abandoned branch must not pass as unknown.
//
// The third answer is the count of places the walk took for a relation the read did not establish
// (unresolved), the same category the tree writers report (unresolved_source_parents): this target has
// no entry to chain, but a record's place on the line is likewise this conversion's answer where the
// read's own answer could not give one -- a record the read left parentless or gave a parent this read
// does not hold, and a record chained through a tool result standing between it and the call it named.
// The detail the report carries names each of the three.
//
// A record the read answered no identity for is never marked inactive *in the flags*: the read recorded
// no place for it in the source's tree, and omitting evidence on a relation the read did not give is
// exactly what the report exists to prevent. Its place is answered by membership instead, and a document
// a selection produced -- whose line answer travels per position (sourceLineFlags) -- reads it there: a
// window that carries such a record as its leaf, or chains one onto the line, is refused or omitted by
// the position-keyed answer exactly as an identified record is. A read that was not reduced keeps the
// flags' long-standing answer: the writers write every record the flags do not name, because the read
// recorded no off-line place for it, and goal 0009's D4 keeps those bytes and that report as they were.
// The walk uses the writers' own tool-result rule, so a record the reader answers as hanging off the
// message that made a call -- while the source's file runs call → result → what follows -- keeps the
// result on the line.
func inactiveSourceRecords(messages []Message, provider string) ([]bool, []bool, int) {
	inactive := make([]bool, len(messages))
	if len(messages) == 0 {
		return inactive, make([]bool, 0), 0
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
	return inactive, onLine, unresolved
}

// sourceLineParent answers the record a walk from index hangs off, by the writers' own rule, and whether
// that place is a relation the read did not establish: the record the source named -- or the record a
// written tool result stands for, where the read may answer a projection -- and the record before it
// where the read answered no relation or named one it does not hold.
//
// The tool-result step is the writers' own projection rule, taken only where the read may project, and
// it is always counted: the read answers a record the source placed under the call and one it placed
// under the call's result with the same parent, so whether the record hung off the call or off a result
// of it is not answerable -- adjacency does not settle it, because the two source files are the same
// answer in the read. The step is the reading the source's file order supports (call → result → what
// follows), and the count names the relation as this conversion's rather than the read's.
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
			return at, true
		}
	}
	if at, ok := byIdentity[parent]; ok && at < index {
		return at, false
	}
	return previous, true
}

// sourceToolResultBefore answers the record a tool-result step stands for: the nearest record before
// index whose tool result a record hanging off it would read as naming parent --
// toolResultCarrierRecord, so a call's outcome the read folded onto the call's own message is matched
// by that message's identity and a result record of its own by the parent it was given. It is the
// writers' own projection rule, so it is consulted only where the read may answer projections: the
// inherited Pi reader resolves a result's descendants to their visible ancestor, so a record the read
// answers as a child of a call may in the file have hung off a result of it, and the file runs
// call → result → what follows. An exact read's answer names the record it means (a Claude transcript's
// parentUuid), so no step is taken there: a result standing beside a record the read chains to the same
// message is a *sibling* of it, and chaining through the sibling would place the record under a branch
// the source did not record.
func sourceToolResultBefore(messages []Message, index int, parent string) (int, bool) {
	for at := index - 1; at >= 0; at-- {
		if len(messages[at].ToolResults) > 0 && toolResultCarrierRecord(messages[at]) == parent {
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
		"record as hanging off the call (a result's descendants resolve to their visible ancestor), so " +
		"a record the source placed directly under the call and one it placed under the call's result " +
		"read as the same answer and which of the two it named is not answerable: the entry is chained " +
		"through the result, the reading the source's own file order holds (call → result → what " +
		"follows). These entries follow the entry before them, or the call's own tool result, which is " +
		"a relation this conversion invented. The entries and their order are unaffected; what the " +
		"count names is that the source's own place in the tree is not the one written"
}

// ErrUnrepresentableLeaf reports a source whose selected leaf the target cannot be given: the target's own
// loader walks from the last entry a conversion wrote and the source's from the read's last record, so
// when the entry written last stands for a record off the line the read's last record answers, a resumed
// target session would continue a branch the source's conversation left -- a context substitution that no
// per-record count discloses. The conversion refuses rather than writing it.
var ErrUnrepresentableLeaf = errors.New(
	"the source's own loader selects a leaf this conversion cannot write as the target's")

// sourceLineAnswers answers the two facts a route's line rule needs about a conversation: for every
// message, whether the source's own relations place it off the line the read's last record answers, and
// how many of the places a walk over the records this conversion writes were ones the read did not
// establish (the same category the tree writers count).
//
// A conversation the reader returned is the read itself, and the walk is taken over it: the flags answer
// off the line only where the read holds an identity for the record. A conversation Select produced
// carries the read's own *walk membership* per record position (Conversation.sourceLineHolds): the line
// belongs to the read and not to the window, so a selection that ends off it must not redefine it -- a
// target session written from such a window would resume a branch the source's conversation left -- and
// this target's log is one line of records, so a record the read's own walk does not reach is omitted and
// counted whether or not the read held an identity for it: writing the identity-less record into it would
// make the abandoned branch model-visible history, and the membership is the only answer such a record's
// place has. The unresolved count is the carried line's own: a walk over the records this conversion
// writes, so it names the places of the entries actually written rather than of a read the caller did not
// carry.
func sourceLineAnswers(conversation Conversation) ([]bool, int) {
	if conversation.sourceLineHolds == nil {
		inactive, _, unresolved := inactiveSourceRecords(conversation.Messages, conversation.Source.Provider)
		return inactive, unresolved
	}
	inactive := make([]bool, len(conversation.sourceLineHolds))
	kept := make([]Message, 0, len(conversation.Messages))
	for index, message := range conversation.Messages {
		inactive[index] = !conversation.sourceLineHolds[index]
		if !inactive[index] {
			kept = append(kept, message)
		}
	}
	_, _, unresolved := inactiveSourceRecords(kept, conversation.Source.Provider)
	return inactive, unresolved
}

// sourceLineFlags answers only the per-record flags, for a caller that asks about one record's place
// (the leaf guard) rather than about the counts. A document a selection produced answers by the read's
// walk membership (Conversation.sourceLineHolds), which holds for records the read gave no identity for
// exactly as for any other; a document the read returned keeps the identity-gated flags, whose
// long-standing answer for an identity-less record is that the read recorded no place to mark.
func sourceLineFlags(conversation Conversation) []bool {
	if conversation.sourceLineHolds != nil {
		inactive := make([]bool, len(conversation.sourceLineHolds))
		for index, holds := range conversation.sourceLineHolds {
			inactive[index] = !holds
		}
		return inactive
	}
	inactive, _, _ := inactiveSourceRecords(conversation.Messages, conversation.Source.Provider)
	return inactive
}

// sourceLineHoldsRecord answers whether the read's own line holds the record at one position of the
// document a conversion writes. A document a selection produced carries the read's own per-position
// answer (Conversation.sourceLineHolds), which is the only answer an identity-less record has; a
// document the read returned is not asked this way -- the writers' long-standing leaf rule reads the
// identity the read holds (sourceLineFlags) and the read's last record is the line's own end by
// construction.
func sourceLineHoldsRecord(conversation Conversation, record int) (bool, bool) {
	if conversation.sourceLineHolds == nil || record < 0 || record >= len(conversation.sourceLineHolds) {
		return false, false
	}
	return conversation.sourceLineHolds[record], true
}

// selectedLeafRefusal answers whether the walk a tree-reproducing conversion wrote is the read's own
// line. Two things are refused, because a resumed target session would otherwise continue a branch the
// source's conversation left: the record the entry written last stands for is one the read's own line
// leaves off it -- a window a selection reduced must not move that answer, or a selection could revive
// the branch by making its own last record the line's end -- and a record the line holds was chained to
// an entry written for one the line leaves off it, which is the place the fallback takes when the
// document no longer holds the record's own relation (a selection that left the record's ancestry out, or
// a parent the read does not hold). Both halves ask by the record's own position, so a record the read
// answered no identity for is refused like any other: the read's answer about the line's membership
// travels with the reduced document per position (Conversation.sourceLineHolds), and the revival check
// (fallbackPlace/the revived pair) is keyed by position too. The first half reads the membership through
// the *written* ancestry of the entry written last, so an identity-less record chained off the line
// through an explicit parent -- the place the direct ancestor lookup takes, ahead of the guarded
// retained-ancestor and fallback places -- is refused as well.
func selectedLeafRefusal(conversation Conversation, tree *sourceTree) error {
	if tree == nil {
		return nil
	}
	if err := lastEntryLineRefusal(conversation, tree); err != nil {
		return err
	}
	if tree.revivedChild < 0 {
		return nil
	}
	return fmt.Errorf("%w: %s, a record on the line the read's last record answers, would be chained to "+
		"the entry of %s, which that line leaves off it: the relation is not in the document this "+
		"conversion writes (the source named a record the read does not hold, or a selection left the "+
		"record's ancestry out), so following the entry written before it would put an abandoned record on "+
		"the walk a resumed session makes", ErrUnrepresentableLeaf,
		recordDescription(conversation, tree.revivedChild),
		recordDescription(conversation, tree.revivedParent))
}

// lastEntryLineRefusal answers the guard's own long-standing half: whether the record the entry written
// last stands for is one the read's line holds. The check is the read's own line -- the walk the writers
// use, with the read's answer kept through a selection -- and the refusal is about the line, not about
// the count: the walk's own unresolved answer names the relations the conversion placed, and this check
// only asks whether the entry written last stands on the line that walk reached.
//
// A document a selection produced answers by the record's own *position* and its actual written
// ancestry (Conversation.sourceLineHolds, tree.entryPlaces): the record may carry no identity, and the
// walk's answer about the line is per position, so the identity lookup cannot answer for it -- an
// identity-less record a window leaves as its leaf, chained off the line through an explicit parent,
// would pass unexamined and let the target's loader resume the branch the source's conversation left.
// A document the read returned keeps the identity-keyed check it has always had: the read's own last
// record is the line's end by construction, and a run without a selection keeps its bytes as they were
// (goal 0009's D4).
func lastEntryLineRefusal(conversation Conversation, tree *sourceTree) error {
	if tree == nil || tree.lastEntryID == nil {
		return nil
	}
	if conversation.sourceLineHolds != nil {
		// Walk from the entry written last up its written ancestry (entryPlaces) to the first place that
		// stands for a record of the document: a family writes its preamble before the conversation, and
		// the place a family-owned entry hangs off answers for it. The membership of that record's own
		// position is the read's answer about the line the target's loader would resume from.
		place, ok := tree.entryPlaces[*tree.lastEntryID]
		for steps := 0; ok && place.record < 0 && steps <= len(tree.entryPlaces); steps++ {
			place, ok = tree.entryPlaces[place.parent]
		}
		if holds, answered := sourceLineHoldsRecord(conversation, place.record); answered && !holds {
			return fmt.Errorf("%w: %s, the record the entry written last stands for, is off the line the "+
				"read's last record answers, so a resumed session would continue a branch the source's "+
				"conversation left", ErrUnrepresentableLeaf, recordDescription(conversation, place.record))
		}
		return nil
	}
	lastWrittenSourceID := strings.TrimSpace(tree.lastOrigin.sourceID)
	if lastWrittenSourceID == "" || len(conversation.Messages) == 0 {
		return nil
	}
	inactive := sourceLineFlags(conversation)
	for index, message := range conversation.Messages {
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

// recordDescription names one record of the document by its own position, for a refusal about a record
// that may carry no identity: its identity and ordinal value where the read holds one, and its ordinal
// alone -- with the read's own answer about the identity said out loud -- where it does not. The
// position is the one the document being written holds, which is the document the refusal is about.
func recordDescription(conversation Conversation, index int) string {
	if index < 0 || index >= len(conversation.Messages) {
		return "a record outside the document"
	}
	message := conversation.Messages[index]
	if id := strings.TrimSpace(message.SourceUUID); id != "" {
		return fmt.Sprintf("%s (ordinal %d)", id, message.Ordinal)
	}
	return fmt.Sprintf("the record at ordinal %d (the read answered no identity for it)", message.Ordinal)
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
// same bytes), or -- for a target whose artifact is a row inside a database rather than a file -- the
// reader's own reference to it; installedBytes answers the bytes of the artifact that was published
// (the file at targetPath for every file-backed route, and the document the importer was handed for the
// database-backed one); cfg supplies the provider configuration (its roots for the route's agent are
// replaced, never mutated: the map is copied); compare walks the read-back against those bytes with the
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
	installedBytes func(targetPath string) ([]byte, error),
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
	installed, err := installedBytes(verification.TargetPath)
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
	answer := newBoundedBuffer(harnessOutputMax)
	diagnostics, err := runHarnessInvocation(ctx, executable, cwd, args, environment, timeout, stderr, answer)
	return answer.String(), diagnostics, err
}

// runRouteDocument runs one non-interactive harness invocation whose stdout is a *protocol document*
// rather than a turn's answer, and returns it whole. The command, its working directory, its environment,
// its deadline and its bounded diagnostics are a turn's; what is not applied is the turn-output bound:
// a session export is a document, and an excerpt of one is not an answer to a check that must read the
// session it names (the same rule the Pi-family load reader applies to its own protocol answers, which it
// reads incrementally). The bytes are the harness's own, so a caller that has to report the harness's
// words for a failed run -- a refusal naming the id it could not resolve is the expected answer to the
// load check's control arm -- still holds them; the caller walks them and keeps only what it asks about.
func runRouteDocument(
	ctx context.Context, executable, cwd string, args []string, environment []string,
	timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	answer := new(bytes.Buffer)
	diagnostics, err := runHarnessInvocation(ctx, executable, cwd, args, environment, timeout, stderr, answer)
	return answer.String(), diagnostics, err
}

// runHarnessInvocation is the process plumbing a route turn and a route document share: the deadline, the
// working directory the harness's own lookup resolves through, the environment, the bounded diagnostics
// channel, and stdout into the caller's own buffer -- the one thing the two callers differ in, because a
// turn's answer is bounded and a protocol document is not.
func runHarnessInvocation(
	ctx context.Context, executable, cwd string, args []string, environment []string,
	timeout time.Duration, stderr io.Writer, answer io.Writer,
) (string, error) {
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
	diagnostics := newBoundedBuffer(harnessDiagnosticsMax)
	command.Stdout = answer
	if stderr != nil {
		command.Stderr = io.MultiWriter(diagnostics, stderr)
	} else {
		command.Stderr = diagnostics
	}
	err := command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return diagnostics.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return diagnostics.String(), err
}

// skipJSONValue consumes one JSON value token by token, holding no more than the token in hand: a
// container is walked by depth and every scalar is read and dropped. It is what keeps an unnamed member's
// value -- a message's content, an event's payload -- from being held as one value, which is what both
// encoding/json's Decoder.Decode and jsontext's own SkipValue do with it (SkipValue validates the skipped
// bytes and keeps them in the decoder's buffer until the value ends: measured at twice the value's size
// over a 16 MiB member). The Pi-family load reader and the OpenCode export reader both walk their
// harness's own protocol answers with it, for the same reason: an answer carrying a whole conversation is
// read for the fields the check asks about, not held whole.
func skipJSONValue(dec *jsontext.Decoder) error {
	token, err := dec.ReadToken()
	if err != nil {
		return err
	}
	switch token.Kind() {
	case '{', '[':
	default:
		return nil
	}
	depth := 1
	for depth > 0 {
		token, err := dec.ReadToken()
		if err != nil {
			return err
		}
		switch token.Kind() {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return nil
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
