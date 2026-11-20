package ccc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// codexSessionsSubdir is the subdirectory of a Codex data directory that holds its rollout sessions.
// The target's own discovery walks it as <home>/sessions/<YYYY>/<MM>/<DD>/rollout-*.jsonl.
const codexSessionsSubdir = "sessions"

// codexSessionFileMode is the mode the installer asks the OS to give a session file it writes:
// readable and writable by its owner, by nobody else. It is named because the *request* is the
// checkable half of that rule on a host without POSIX mode bits — Windows answers every os.Stat with
// -rw-rw-rw- for a file this installer asked to be 0600 — so the test that pins the rule asserts this
// constant there and the installed file's own bits where the platform reports them.
const codexSessionFileMode os.FileMode = 0o600

// ErrTargetHomeMissing reports a target data directory that does not exist. call/ccc never invents one:
// a conversion that wrote sessions into a directory the target harness does not use would look
// installed while nothing could resume it.
var ErrTargetHomeMissing = errors.New("the target harness has no data directory")

// ErrTargetExists reports that the target path a conversion chose is already taken. A conversion always
// creates a new identity, and a taken path is a refusal, never an overwrite.
var ErrTargetExists = errors.New("a target session already exists at that path")

// ErrNoExclusiveInstall reports that the target filesystem has no exclusive publication mechanism. The
// installer's only one is a hard link, which takes the session's name atomically and fails when the name
// is taken; a filesystem without hard links (some Windows volumes, some network mounts) is refused
// instead of being published with a rename, because a rename replaces whatever is at the destination and
// would let one conversion replace a session another agent had already continued.
var ErrNoExclusiveInstall = errors.New("the target filesystem has no exclusive install mechanism")

// linkSession is os.Link, named so the installer's exclusive-publication rule can be exercised on a host
// whose filesystems all have hard links: a test replaces it to fail the way a filesystem without them
// does, and the installer must then refuse rather than publish by rename.
var linkSession = os.Link

// removeFile is os.Remove, named so the installer's answer to a temporary file it cannot remove after the
// session was published (the path and the failure are returned together) can be exercised on any host:
// making a removal fail through the directory's own permissions would be a POSIX-only test of a rule that
// holds wherever this installer runs.
var removeFile = os.Remove

// ResolveCodexHome answers the Codex data directory a conversion installs into: the caller's explicit
// directory, else $CODEX_HOME, else ~/.codex. The boolean reports whether the answer came from the
// caller, which is what decides whether a missing directory may be created.
func ResolveCodexHome(explicit string) (string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		home, err := filepath.Abs(explicit)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s: %w", explicit, err)
		}
		return home, true, nil
	}
	if fromEnv := strings.TrimSpace(os.Getenv("CODEX_HOME")); fromEnv != "" {
		home, err := filepath.Abs(fromEnv)
		if err != nil {
			return "", false, fmt.Errorf("resolving $CODEX_HOME: %w", err)
		}
		return home, false, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolving the home directory: %w", err)
	}
	return filepath.Join(dir, ".codex"), false, nil
}

// InstallCodexRollout writes a converted rollout into a Codex data directory and returns the path it
// occupies, plus the mechanism that put it there.
//
// Nothing existing is ever replaced: the bytes are written to a sibling temporary file first and the
// final name is taken with a hard link, which is atomic on a filesystem that has them and exclusive, so
// a taken name fails instead of being overwritten. A filesystem without hard links (some Windows
// volumes, some network mounts) has no exclusive way to publish the finished file here, so the
// installation is refused (ErrNoExclusiveInstall) rather than published by rename: a rename replaces
// whatever is at the destination, which is exactly the overwrite this rule forbids. A reader therefore
// never sees a half-written session, and a conversion cannot replace a session another agent may already
// have continued.
//
// The returned path and the error are independent answers. A non-empty path means the session is
// published, and a non-nil error beside it is about something other than the publication itself -- a
// temporary file that could not be removed, reported rather than left behind in silence. A call that
// published nothing answers with an empty path, so the caller reads the two apart instead of inferring
// publication from the error.
func InstallCodexRollout(rollout CodexRollout, home string) (string, string, error) {
	return installFile(home, codexSessionsSubdir, rollout.Directory, rollout.FileName, rollout.Data, codexSessionFileMode)
}

// StructureVerification reports whether the installed (or, for a dry run, a temporary copy) target
// session holds every piece of evidence the writer placed in it, read back through the inherited
// AgentsView Codex reader. The reader is upstream's implementation of the same format, so this compares
// the file with an independent read of it rather than with the writer's own idea of it.
type StructureVerification struct {
	State      string         `json:"state"`
	Method     string         `json:"method"`
	Detail     string         `json:"detail,omitempty"`
	TargetPath string         `json:"target_path,omitempty"`
	ReadBack   ReadBackCounts `json:"read_back,omitempty"`
	// NotReadBackVerbatim counts the written dialogue messages the read-back does not show with their own
	// text and the installed file's own dialogue record holds; one read-back message shows one written
	// message, so a text the document writes twice needs the read-back to hold it twice. The inherited
	// reader reads some text a file holds differently: a user text block that opens with `# AGENTS.md` (or
	// another injected-context envelope) is read as that context rather than as a turn (internal/parser's
	// preprocessCodexUserTextBlocks), so the reader reads no message for the whole prompt, or reads only
	// the text the envelope preceded. A Claude transcript may hold a genuine prompt with that prefix, and
	// such a message is in the file, so the run counts it here and warns instead of reporting a loss the
	// conversion did not make; the file's record is what establishes that, so a message the file does not
	// hold stays a failure.
	NotReadBackVerbatim int `json:"not_read_back_verbatim,omitempty"`
}

// ReadBackCounts is what the inherited reader made of the target file: the session it found and the
// evidence it read there. Every target's structural check reports these, so a consumer reads the same
// block whichever target a conversion wrote.
type ReadBackCounts struct {
	SessionID            string `json:"session_id,omitempty"`
	Messages             int    `json:"messages"`
	UserMessages         int    `json:"user_messages"`
	AssistantMessages    int    `json:"assistant_messages"`
	ToolCalls            int    `json:"tool_calls"`
	ToolResults          int    `json:"tool_results"`
	CompactionBoundaries int    `json:"compaction_boundaries"`
}

// VerifyCodexRolloutStructure reads the installed rollout back through the inherited Codex reader and
// compares it, item by item, with what the writer intended to carry. sessionsDir is the target's
// sessions directory; cfg supplies the provider configuration (its Codex roots are replaced, never
// mutated: the map is copied).
//
// The read-back is the file this call names (TargetPath: the path the conversion published, or the dry
// run's copy of the same bytes), read through the inherited reader's own path reference. The check never
// resolves the session's id through the target's sessions tree: that lookup enumerates every dated
// directory under the root and prefers the lexically first copy of the id, so an unrelated copy of the
// id -- or simply a home with a long history -- would decide what is compared and how much of the home
// is walked, for a file whose path the check was handed (review round 12's finding 2).
func VerifyCodexRolloutStructure(
	ctx context.Context, sessionsDir string, rollout CodexRollout, cfg config.Config,
) (StructureVerification, error) {
	return verifyStructure(ctx, sessionsDir, parser.AgentCodex, structureMethodCodex,
		structureReaderCodex, structureReaderCodex,
		filepath.Join(sessionsDir, filepath.FromSlash(rollout.Directory), rollout.FileName),
		os.ReadFile,
		func(readBack Conversation, installed []byte) roundTripComparison {
			return compareCodexRoundTrip(rollout, readBack, installed)
		}, cfg)
}

func readBackCounts(conversation Conversation) ReadBackCounts {
	counts := Summarize(conversation.Messages)
	return ReadBackCounts{
		SessionID:            conversation.Source.SessionID,
		Messages:             counts.Messages,
		UserMessages:         counts.UserMessages,
		AssistantMessages:    counts.AssistantMessages,
		ToolCalls:            counts.ToolCalls,
		ToolResults:          counts.ToolResults,
		CompactionBoundaries: counts.CompactBoundaries,
	}
}

// roundTripComparison is what comparing a target document with a read-back of the installed file found.
// Both targets' structural checks compare the same way, so this is the type they both answer with.
type roundTripComparison struct {
	// differences names every item one side holds and the other does not, each by what identifies it
	// rather than by its text.
	differences []string
	// notReadBackVerbatim counts the written dialogue messages the read-back does not show with their own
	// text and the installed file's records hold: the reader's own projection of the target format, not a
	// loss.
	notReadBackVerbatim int
}

// detail is the verification detail of a comparison the file satisfied: empty when the read-back showed
// every written item verbatim, and otherwise the one thing a caller has to be told before reading
// "verified" as "the read-back shows every message with the text the file holds". The reader is named by
// the caller, because the sentence is about the reader that read *this* file back: the name is the
// route's own (structureReaderCodex/structureReaderClaude), so a Claude conversion can never report a
// Codex reader.
func (roundTrip roundTripComparison) detail(reader string) string {
	if roundTrip.notReadBackVerbatim == 0 {
		return ""
	}
	return fmt.Sprintf(
		"%d written message(s) are in the installed session and the inherited %s reader does not read "+
			"their own text from it: the file's dialogue records hold them, so the conversion carried them "+
			"and the reader's own projection of the file is what the read-back shows instead",
		roundTrip.notReadBackVerbatim, reader)
}

// compareRoundTrip names every way a read-back of the installed file differs from a target document,
// item by item. Every target's document is the same item list and the comparison is the same, so the walk
// lives here and each target answers with its own reader's rules (roundTripReader) and its own file's
// records.
//
// The comparison is by evidence, not by record shape: a target reader's own normalisations -- the records
// it puts back into one message, the text it trims, the boundary record it classifies as a system row --
// are not differences in what the file holds. What it cannot normalise is a missing or altered message,
// call, result or boundary, which is what this reports.
//
// A message the reader reads as no message at all (its text is blank after trimming) is not one the
// writer may count as carried: the writer omits such text and names it, and a document that claims one is
// reported here as the missing message it is.
//
// One reading is neither a normalisation nor a loss: a written message can come back as no message at all
// or with the text an injected-context envelope preceded, because the reader classifies a block that
// opens with such an envelope as that context rather than a turn. Dialogue is therefore walked against
// the installed file's own dialogue records, in the file's own order, rather than through a set
// membership test: what tells a projection from a loss is the record -- and where the document writes it
// -- not the read-back's silence, which is what lets both projection shapes resolve to the same count
// (compareDialogue) while a file whose records hold the right texts in another order is reported. The
// file is asked about dialogue only, because that is where the reader's projection applies: a call, a
// result and a boundary are compared through keys the reader derives from the record itself, and a
// difference there is a difference in what the file holds.
func compareRoundTrip(
	items []TargetItem, readBack Conversation, reader roundTripReader, installedDialogue dialogueRecords,
	installed []byte,
) roundTripComparison {
	intended := intendedRoundTripEvidence(items)
	read := readRoundTripEvidence(readBack, reader, roundTripResultOwnersStated(items), installed)
	dialogue := compareDialogue(intended.dialogue, read.dialogue, installedDialogue)
	var roundTrip roundTripComparison
	roundTrip.differences = append(roundTrip.differences, dialogue.differences...)
	roundTrip.notReadBackVerbatim = dialogue.notReadBackVerbatim
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryToolCall, compareAsMultiset, intended.calls, read.calls)...)
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryToolResult, compareAsMultiset, intended.results, read.results)...)
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryCompactionBoundary, compareInOrder, intended.boundaries, read.boundaries)...)
	return roundTrip
}

// roundTripEvidence is one side of a round-trip comparison, by kind: the identities the comparison asks
// about, in the order that side holds them (dialogue in reading order, calls and results as multisets,
// boundaries in order).
type roundTripEvidence struct {
	dialogue   []string
	calls      []string
	results    []string
	boundaries []string
}

// intendedRoundTripEvidence collects the evidence a target document says it wrote.
//
// A tool result is identified by the invocation it was written for as well as by its own call id and
// text where the writer states that invocation (TargetItem.OwnerCall, which the reduced writers know
// from the read's pairing or their disclosed inference): an id two written calls share cannot say which
// occurrence an output answers, and the target's reader attaches the output to whichever occurrence its
// own order reaches (the last call stated under the id). A document that states no owner -- a run without
// a selection, whose format pairs by the id alone -- keeps the id-and-text key it has always had, so its
// comparison and whole report are what they were (goal 0009's D4).
func intendedRoundTripEvidence(items []TargetItem) roundTripEvidence {
	var evidence roundTripEvidence
	callKeys := map[int]string{}
	// The occurrence count is per call id, in the document's own order: an id two written call items
	// share cannot distinguish two invocations whose id, tool and arguments are identical, so the
	// occurrence of the invocation is the only evidence left that says which of them a result was
	// written for, and a comparison keyed by the call's fields alone would accept a result moved from
	// one occurrence to the other.
	occurrences := map[string]int{}
	for itemIndex, item := range items {
		switch item.Kind {
		case ItemUser, ItemAssistant:
			evidence.dialogue = append(evidence.dialogue, dialogueKey(string(item.Kind), item.Text))
		case ItemToolCall:
			key := roundTripCallKey(item.CallID, item.ToolName, item.InputJSON)
			occurrences[item.CallID]++
			callKeys[itemIndex] = roundTripOwnerCallKey(key, occurrences[item.CallID])
			evidence.calls = append(evidence.calls, key)
		case ItemToolResult:
			ownerKey, known := "", false
			if item.OwnerCall != nil {
				ownerKey, known = callKeys[*item.OwnerCall], true
			}
			evidence.results = append(evidence.results,
				roundTripOwnerResultKey(ownerKey, known, item.CallID, item.Text))
		case ItemCompactionBoundary:
			evidence.boundaries = append(evidence.boundaries, roundTripBoundaryKey(item.Text))
		}
	}
	return evidence
}

// roundTripResultOwnersStated answers whether a target document states the owning call of its results
// (TargetItem.OwnerCall). A document a selection produced does -- the read's pairing or the writer's
// disclosed inference names the occurrence -- and a run without a selection does not; the comparison
// asserts the attachment on the documents that claim one and keeps the id-and-text comparison the
// default path has always had, whose whole report is byte-identical to the same run before selections
// existed (goal 0009's D4).
func roundTripResultOwnersStated(items []TargetItem) bool {
	for _, item := range items {
		if item.Kind == ItemToolResult && item.OwnerCall != nil {
			return true
		}
	}
	return false
}

// roundTripOwnerResultKey identifies one written result by the invocation it was written for and by the
// result's own key: the owning call's evidence key (its id, tool, arguments and occurrence, so an
// occurrence sharing an id with another is still told apart) where the document states one, and the plain
// result key where it does not. The read side answers with the same shape, read under the same gate.
func roundTripOwnerResultKey(ownerKey string, known bool, callID, text string) string {
	result := roundTripResultKey(callID, text)
	if !known || ownerKey == "" {
		return result
	}
	return "paired\x00" + ownerKey + "\x00" + result
}

// roundTripOwnerCallKey identifies one invocation an output may have been written for: the call's own
// evidence key (id, tool, arguments) beside the occurrence of the invocation among the calls the same id
// records, in the order the side holds them, 1-based. An id two call occurrences share cannot tell two
// invocations apart when their id, tool and arguments are equal -- so those three fields alone would
// accept a result moved from one occurrence to the other, and the occurrence is the only evidence the
// comparison has left. Both sides count the occurrence the same way: the document counts its call items
// under the id in item order, and the read-back counts the invocations its rows stand for -- the file's
// own invocation units where the fold applies (callOccurrenceOrdinals), the rows themselves otherwise.
func roundTripOwnerCallKey(callKey string, occurrence int) string {
	return callKey + "\x00#" + strconv.Itoa(occurrence)
}

// readResultOwnerKeys answers, per result occurrence of a read-back, the call occurrence the target's own
// reader attaches it to, keyed by the result's place in the read-back.
//
// reader.ownersByAncestry says the read-back's reader answers its records' parent relations (the Pi family
// and the Claude transcript): the owner is then the call occurrence the result's own written ancestry
// reaches (readResultOwnerKeysByAncestry), so a result whose recorded relation reaches a sibling's
// message is reported rather than accepted because an earlier call in file order shares its id (round
// 9's finding 4). Otherwise the rule these formats' flat readers take stands: the last call the read-back
// states under the result's id at or before the result's own place -- an output names its invocation by
// id, and a call stated later under the same id is what such a reader resolves it to (the inherited
// Codex reader keeps the last call registered for an id) -- so a result a conversion wrote under an id
// two occurrences share answers a different invocation than the writer intended, and comparing the owner
// is what shows it. An occurrence the reader's rows only stand for (the fold: one written record the
// reader projects into several rows) answers with the occurrence of the invocation the row stands for,
// which is the occurrence the document states for the record it wrote.
func readResultOwnerKeys(readBack Conversation, reader roundTripReader, installed []byte) map[resultOccurrence]string {
	if reader.ownersByAncestry {
		return readResultOwnerKeysByAncestry(readBack, reader, installed)
	}
	ordinals := callOccurrenceOrdinals(readBack, reader, installed)
	owners := map[resultOccurrence]string{}
	lastUnderID := map[string]string{}
	for index, message := range readBack.Messages {
		for callIndex, call := range message.ToolCalls {
			lastUnderID[call.ToolUseID] = roundTripOwnerCallKey(
				roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON),
				ordinals[callOccurrence{message: index, call: callIndex}])
		}
		for resultIndex, result := range message.ToolResults {
			key, ok := lastUnderID[result.ToolUseID]
			if !ok {
				continue
			}
			owners[resultOccurrence{message: index, result: resultIndex}] = key
		}
	}
	return owners
}

// callOccurrenceOrdinals answers, per call row of a read-back, the occurrence of the invocation that row
// stands for among the same id's invocations, 1-based, in the read-back's own order. Where a fold stands
// between the installed file's call records and the reader's rows (roundTripReader.callEvidence), the
// count is over the *invocation units* the file's own records state: the reader's per-file rows are the
// projection of one written record, so they all stand at that record's own occurrence -- which is the
// occurrence the document states for the invocation it wrote -- and a row the file's records do not
// account for (an id whose rows are not foldable, codexFoldableRows) is an occurrence of its own, exactly
// as the calls comparison reads it. Without a fold every row is an occurrence of its own, in the
// read-back's own order.
func callOccurrenceOrdinals(readBack Conversation, reader roundTripReader, installed []byte) map[callOccurrence]int {
	type callRow struct {
		place callOccurrence
		key   string
	}
	rowsByID := map[string][]callRow{}
	for index, message := range readBack.Messages {
		for callIndex, call := range message.ToolCalls {
			rowsByID[call.ToolUseID] = append(rowsByID[call.ToolUseID], callRow{
				place: callOccurrence{message: index, call: callIndex},
				key:   roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON),
			})
		}
	}
	unitsByID := map[string][]codexInstalledCallUnit{}
	if reader.callEvidence != nil && installed != nil {
		for _, unit := range codexInstalledCallUnits(installed) {
			unitsByID[unit.id] = append(unitsByID[unit.id], unit)
		}
	}
	ordinals := make(map[callOccurrence]int)
	for id, rows := range rowsByID {
		units := unitsByID[id]
		foldable := false
		if len(units) > 0 {
			keys := make([]readBackCall, 0, len(rows))
			for _, row := range rows {
				keys = append(keys, readBackCall{key: row.key})
			}
			foldable = codexFoldableRows(units, keys)
		}
		if foldable {
			// The rows are the units' projection in the same order, so a row belongs to the unit
			// whose key it states: the cursor advances only when the row no longer states the current
			// unit's key but does state the next unit's, which is what one written record's rows (all
			// stating that record's key) followed by the next record's produce.
			unit := 0
			for _, row := range rows {
				if unit+1 < len(units) && row.key != units[unit].key && row.key == units[unit+1].key {
					unit++
				}
				ordinals[row.place] = unit + 1
			}
			continue
		}
		for index, row := range rows {
			ordinals[row.place] = index + 1
		}
	}
	return ordinals
}

// readResultOwnerKeysByAncestry answers the same question through the read-back's own recorded relations,
// for a format that is a tree: the result occurrence's owner is the call occurrence the nearest message
// on the result's own recorded parent chain states under the result's id -- the last call that message
// states under it, which is the reader's own attachment within one message. A chain that reaches no such
// call leaves the occurrence ownerless, and the comparison reports it against the document's stated
// owner instead of finding some other occurrence by id.
//
// The walk starts at the result's own message's parent, never at the message itself: the format records
// the invocation on an ancestor, and a message holding both a call and its own output is the flat
// format's shape, which keeps the file-order rule above. A chain the file's records cannot answer (an
// unknown parent id, a record without one) ends the walk; the cycle bound keeps a file whose relations
// loop from walking forever.
//
// A parent the read-back does not hold as a message of its own is not the end of the walk: the reader
// merges some records into others (the Claude reader puts the assistant entries of one message id back
// into the run's last entry, which keeps that entry's parent -- a uuid held by the entry that was merged
// away), and the chain the *file* records steps through that entry to the ancestor the merged message
// really hangs off. Where the route answers the installed file's own entry relations
// (roundTripReader.entryParents), the walk follows that chain past such an entry rather than stopping at
// it, so a result whose invocation stands above a merged entry is not reported missing although the
// installed ancestry reaches it (round 10's finding 3).
func readResultOwnerKeysByAncestry(
	readBack Conversation, reader roundTripReader, installed []byte,
) map[resultOccurrence]string {
	position := map[string]int{}
	for index, message := range readBack.Messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			position[id] = index
		}
	}
	fileParents := map[string]string{}
	if reader.entryParents != nil {
		fileParents = reader.entryParents(installed)
	}
	ordinals := callOccurrenceOrdinals(readBack, reader, installed)
	lastUnderID := func(messageIndex int, message Message, callID string) (string, bool) {
		owner := ""
		found := false
		for callIndex, call := range message.ToolCalls {
			if call.ToolUseID != callID {
				continue
			}
			owner = roundTripOwnerCallKey(
				roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON),
				ordinals[callOccurrence{message: messageIndex, call: callIndex}])
			found = true
		}
		return owner, found
	}
	owners := map[resultOccurrence]string{}
	for index, message := range readBack.Messages {
		for resultIndex, result := range message.ToolResults {
			parent := strings.TrimSpace(message.SourceParentUUID)
			for steps := 0; parent != "" && steps <= len(readBack.Messages)+len(fileParents); steps++ {
				next, ok := position[parent]
				if !ok {
					// The entry produced no message of its own (the reader merged it into another): the
					// file's own relation is what says where the message continues.
					parent = strings.TrimSpace(fileParents[parent])
					continue
				}
				if owner, ok := lastUnderID(next, readBack.Messages[next], result.ToolUseID); ok {
					owners[resultOccurrence{message: index, result: resultIndex}] = owner
					break
				}
				parent = strings.TrimSpace(readBack.Messages[next].SourceParentUUID)
			}
		}
	}
	return owners
}

// readRoundTripEvidence collects what a read-back of the installed file shows, minus the records the
// target's own reader classifies away. ownersStated says the target document states the owning call of
// its results (roundTripResultOwnersStated): the read-back's own attachment is then compared with it,
// and where the document states none the results keep the id-and-text keys the comparison has always
// used. installed lets a route whose reader projects one call record into several rows fold them back
// into the file's own invocation units (roundTripReader.callEvidence); a nil installed leaves the
// read-back's rows as the call evidence.
func readRoundTripEvidence(
	readBack Conversation, reader roundTripReader, ownersStated bool, installed []byte,
) roundTripEvidence {
	var evidence roundTripEvidence
	evidence.dialogue = readBackDialogueKeys(readBack, reader)
	owners := map[resultOccurrence]string{}
	if ownersStated {
		owners = readResultOwnerKeys(readBack, reader, installed)
	}
	var calls []readBackCall
	for index, message := range readBack.Messages {
		if message.IsCompactBoundary {
			evidence.boundaries = append(evidence.boundaries, roundTripBoundaryKey(message.Content))
			continue
		}
		for _, call := range message.ToolCalls {
			calls = append(calls, readBackCall{
				id:  call.ToolUseID,
				key: roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON),
			})
		}
		for resultIndex, result := range message.ToolResults {
			text, _ := resultText(result)
			owner, known := owners[resultOccurrence{message: index, result: resultIndex}]
			evidence.results = append(evidence.results,
				roundTripOwnerResultKey(owner, known, result.ToolUseID, text))
		}
	}
	if reader.callEvidence != nil {
		evidence.calls = reader.callEvidence(calls, installed)
		return evidence
	}
	for _, call := range calls {
		evidence.calls = append(evidence.calls, call.key)
	}
	return evidence
}

// roundTripReader is what one target's own reader decides about a read-back, so the walk over the
// read-back is shared while each target keeps its own reader's normalisations: which records the reader
// classifies away before the walk, and which item kind one message's dialogue stands for (false when the
// reader has no message for that role).
type roundTripReader struct {
	classifiesAway func(Message) bool
	kindOf         func(Message) (TargetItemKind, bool)
	// readText is the route's own rule for the dialogue a read-back message shows, for a reader whose
	// own projection renders a message differently from the way the shared rule reads one (OpenCode's
	// reader joins a message's dialogue and its persisted reasoning into one text). Nil -- every route
	// whose reader answers dialogue or plain text -- leaves the shared rule in place.
	readText func(Message) string
	// callEvidence, where non-nil, answers the read-back's calls as the invocation units the installed
	// file's own records state (codexCallEvidence for the Codex route). A reader that projects one call
	// record into several call rows -- the inherited Codex reader reads one `apply_patch` as one call per
	// file it names -- makes the read-back's rows the reader's view of the file's records rather than
	// additional invocations, and a comparison that took them for calls would report one invocation as
	// many (round 9's finding 2). Nil leaves the read-back's own rows as the call evidence, which is what
	// every other route's format holds.
	callEvidence func(rows []readBackCall, installed []byte) []string
	// ownersByAncestry says the read-back's format is a tree whose records the reader answers parent
	// relations for (the Pi family and the Claude transcript): a result's owning call is then resolved
	// through the result's own recorded ancestry rather than by file order
	// (readResultOwnerKeysByAncestry). False is the flat format (the Codex rollout), whose read-back
	// answers no such relations.
	ownersByAncestry bool
	// entryParents answers the installed file's own entry relations -- each record's identity and the
	// identity it hangs off -- for a format whose reader merges some records into others: the Claude
	// reader puts the assistant entries of one message id back into the run's last entry, which keeps
	// that entry's parent, so the read-back holds no message for the entry that parent names and the
	// ancestry walk follows the file's own chain past it (readResultOwnerKeysByAncestry). Nil leaves the
	// walk bounded by the read-back's own messages, which is every format whose records the reader
	// answers one message for.
	entryParents func(installed []byte) map[string]string
}

// readBackCall is one call row of a read-back: the id its file's own record is keyed by, and the
// comparison's key for the call.
type readBackCall struct {
	id  string
	key string
}

// readDialogue is one read-back message's dialogue as this route's reader shows it.
func (reader roundTripReader) readDialogue(message Message) string {
	if reader.readText != nil {
		return reader.readText(message)
	}
	return dialogueOf(message).Text
}

// readBackDialogueKeys answers one read-back's dialogue keys, in the read-back's own order, under one
// reader's rules: a boundary is compared as its own kind rather than as dialogue, a record the reader
// classifies away carries no dialogue, and what one message's dialogue is comes from the route's own
// reader rule. Two callers ask this question -- the walk's read side, and a target whose artifact only
// its harness can write, whose installed dialogue records therefore are the read-back's own -- so the rule
// lives here and both answer it the same way.
func readBackDialogueKeys(readBack Conversation, reader roundTripReader) []string {
	var keys []string
	for _, message := range readBack.Messages {
		if message.IsCompactBoundary || reader.classifiesAway(message) {
			continue
		}
		if text := reader.readDialogue(message); strings.TrimSpace(text) != "" {
			if kind, ok := reader.kindOf(message); ok {
				keys = append(keys, dialogueKey(string(kind), text))
			}
		}
	}
	return keys
}

// compareCodexRoundTrip names every way the file's read-back differs from the target document, through
// the shared walk with the Codex reader's own rules.
//
// The Codex reader pairs a call with its output onto one message, trims a result's output, trims a
// compacted record's summary and drops an empty argument object, so those are the reader's own
// normalisations and not differences in what the file holds. It reads user text blocks through the
// injected-context classification, because a block that opens with an envelope is that context rather
// than a turn (internal/parser's preprocessCodexUserTextBlocks).
func compareCodexRoundTrip(rollout CodexRollout, readBack Conversation, installed []byte) roundTripComparison {
	return compareRoundTrip(rollout.Items, readBack, codexRoundTripReader(rollout.SelectionReduced),
		installedDialogueRecords(installed), installed)
}

// codexRoundTripReader is the Codex reader's rules for the read-back walk: the shared plain reader, plus
// the invocation-unit fold (codexCallEvidence) the reader's per-file projection of one `apply_patch`
// call requires.
//
// reduced says the document being compared is one a selection produced (CodexRollout.SelectionReduced).
// The fold is gated on it because the shape it answers exists only there: the writer coalesces the
// reader's per-file rows into one `function_call` exactly on the reduced path, so a reduced document
// needs the fold, while a run without a selection writes the rows it has always written and keeps the
// comparison -- and the verdict, the warning and the CLI exit status that follow from it -- its earlier
// revisions had (goal 0009's D4; round 10's finding 2).
func codexRoundTripReader(reduced bool) roundTripReader {
	reader := plainRoundTripReader()
	if reduced {
		reader.callEvidence = codexCallEvidence
	}
	return reader
}

// codexInstalledCallUnit is one invocation unit the installed file's own records state: the id its record
// registers the call under and, where this build can state it, the call key the comparison uses. The
// writer writes every call as a `function_call` record, so a unit of that type is keyed; a record of
// another type the inherited reader reads as a call (`custom_tool_call`, `image_generation_call`) is a
// unit all the same -- the file states an invocation there -- but its key is the reader's own reading of
// the record's fields, which this build does not restate. An unkeyed unit is never folded
// (codexFoldableRows), so the read-back's rows stand for that id rather than disappearing behind a fold
// the comparison cannot attribute (round 10's finding 4).
type codexInstalledCallUnit struct {
	id    string
	key   string
	keyed bool
}

// codexInstalledCallUnits answers the invocation units the installed file's own records state, in the
// file's own order: one unit per record the inherited reader reads as a call, keyed the way both sides
// spell a call (roundTripCallKey) where the record is the writer's own `function_call` envelope. The
// inherited reader projects one `apply_patch` record into one call row per file it names, so the
// read-back's rows are the reader's view of these records; the records themselves are what the file
// holds, and a file holding two call records under one id states two units (round 9's finding 2).
func codexInstalledCallUnits(installed []byte) []codexInstalledCallUnit {
	var units []codexInstalledCallUnit
	for _, line := range bytes.Split(installed, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record codexInstalledCallRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != codexRecordResponseItem {
			continue
		}
		switch record.Payload.Type {
		case codexItemFunctionCall:
			units = append(units, codexInstalledCallUnit{
				id:    record.Payload.CallID,
				key:   roundTripCallKey(record.Payload.CallID, record.Payload.Name, record.Payload.Arguments),
				keyed: true,
			})
		case codexItemCustomToolCall:
			units = append(units, codexInstalledCallUnit{id: record.Payload.CallID})
		case codexItemImageGenerationCall:
			units = append(units, codexInstalledCallUnit{id: record.Payload.ID})
		}
	}
	return units
}

// codexCallEvidence answers the read-back's call evidence as the installed file's own invocation units:
// for every id the file's call records and the read-back's rows agree about, the file's units -- as many
// as it holds, and no more (two actual call envelopes are two units) -- and, for every other id, the
// read-back's own rows, exactly as the comparison has always read them.
//
// The file's records stand for the read-back's rows under a stated id because one record is one written
// invocation and the reader's rows are its per-file projection of it: a document that wrote one
// invocation matches the one envelope however many rows the reader shows, while a second envelope is
// never folded into the first (round 9's finding 2). The fold is taken only where the rows really are
// that projection (codexFoldableRows): a row the file's call records do not account for -- a call the
// reader read from another record under the same id, whose key the units do not state -- is evidence of
// an invocation the document did not write, and the id's rows then stand for the comparison instead of
// being discarded in favor of the units (round 10's finding 4). Where the read-back holds *fewer* rows
// under a stated id than the file holds records, some record's rows are not in the read-back at all, and
// the read-back's own rows stand for the comparison so the document's unmatched call is reported.
func codexCallEvidence(rows []readBackCall, installed []byte) []string {
	units := codexInstalledCallUnits(installed)
	if len(units) == 0 {
		keys := make([]string, 0, len(rows))
		for _, row := range rows {
			keys = append(keys, row.key)
		}
		return keys
	}
	unitsUnderID := map[string][]codexInstalledCallUnit{}
	for _, unit := range units {
		unitsUnderID[unit.id] = append(unitsUnderID[unit.id], unit)
	}
	rowsUnderID := map[string][]readBackCall{}
	for _, row := range rows {
		rowsUnderID[row.id] = append(rowsUnderID[row.id], row)
	}
	keys := make([]string, 0, len(units)+len(rows))
	handled := map[string]bool{}
	for _, unit := range units {
		if handled[unit.id] {
			continue
		}
		handled[unit.id] = true
		idUnits, idRows := unitsUnderID[unit.id], rowsUnderID[unit.id]
		if codexFoldableRows(idUnits, idRows) {
			for _, folded := range idUnits {
				keys = append(keys, folded.key)
			}
			continue
		}
		for _, row := range idRows {
			keys = append(keys, row.key)
		}
	}
	for _, row := range rows {
		if !handled[row.id] {
			keys = append(keys, row.key)
		}
	}
	return keys
}

// codexFoldableRows answers whether the read-back's rows under one stated id are the reader's projection
// of the file's call records under it: every unit states a key this build can compare, the units state
// *distinct* keys, the read-back holds at least as many rows as the file holds records, and every row's
// key is one a unit states. A row whose key no unit states is a call the reader read from a record these
// units do not account for, so folding the id's rows into the units would hide it; an unkeyed unit (a
// record type whose call key this build does not restate) leaves the id's rows standing for the same
// reason; and two units stating one key leave every row with that key ambiguous -- the fold attributes a
// row by the key it states, so it cannot say which of the two records a row projects, and folding them
// would let a result the reader attached to one occurrence be read as the other's (round 15's finding 2).
// The writer's own documents state no such pair (a repeated recorded id is emitted under an id generated
// from the occurrence's own place, colliding_call_ids), but this walk reads arbitrary installed bytes,
// and the fold must not collapse records the file's own bytes cannot tell apart.
func codexFoldableRows(units []codexInstalledCallUnit, rows []readBackCall) bool {
	if len(rows) < len(units) {
		return false
	}
	seenKeys := make(map[string]bool, len(units))
	for _, unit := range units {
		if !unit.keyed {
			return false
		}
		if seenKeys[unit.key] {
			return false
		}
		seenKeys[unit.key] = true
	}
	for _, row := range rows {
		attributed := false
		for _, unit := range units {
			if unit.key == row.key {
				attributed = true
				break
			}
		}
		if !attributed {
			return false
		}
	}
	return true
}

// codexInstalledCallRecord is the part of one installed line the invocation-unit walk reads: the
// record's own type, its function call's id, name and arguments, and the `id` an image generation call
// registers its call under. It declares no field for anything else, so reading a large session copies no
// payload the walk does not ask about.
type codexInstalledCallRecord struct {
	Type    string `json:"type"`
	Payload struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		CallID    string `json:"call_id"`
		ID        string `json:"id"`
	} `json:"payload"`
}

// dialogueKey identifies one piece of dialogue: which role recorded it and the text itself. The two are
// joined by a NUL rather than a printable separator so that a text holding the separator cannot read as
// a different kind, and so that a difference report can take the text back out of the key it describes.
func dialogueKey(kind, text string) string {
	return kind + "\x00" + text
}

// installedDialogueRecords reads the dialogue the installed file's own records hold, in the file's own
// order: for every rollout record that is a dialogue message written the way this writer writes one -- a
// response_item whose payload is a message, whose role is the user's or the assistant's, and whose
// content holds a block of the type this writer writes for that role carrying the message's own text --
// the identity the comparison uses (which role recorded it and the text itself).
//
// The question is this narrow on purpose. The same text is also written into the target's own event for
// the turn, because that is what the harness's view of the conversation is built from, and the text of
// one message can appear in another message's tool output; asking the file whether it holds a text
// *somewhere* would answer for records that are not the message, and a session whose model-visible
// record is gone would verify against the copy its own view keeps. The walk asks what the target's model
// history is built from.
//
// The order is kept because it is a claim of its own: the file's dialogue records are the document's
// dialogue in the document's own order, and a walk that only asked whether the file holds each text
// would verify two prompts the reader projects away against one another swapped -- a different
// conversation from the one the document writes.
//
// A line that is not one readable record is skipped: the file's own bytes are being read, and the
// failure this walk answers for is an absent record, not a malformed one.
func installedDialogueRecords(installed []byte) dialogueRecords {
	var keys []string
	for _, line := range bytes.Split(installed, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record codexInstalledRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		kind, text, ok := record.dialogueMessage()
		if !ok {
			continue
		}
		keys = append(keys, dialogueKey(string(kind), text))
	}
	return newDialogueRecords(keys)
}

// dialoguePlace says what the installed file's own records answered for one written message.
type dialoguePlace int

const (
	// dialogueRecordInPlace: the file's next unconsumed record is the message's own, and it was consumed.
	dialogueRecordInPlace dialoguePlace = iota
	// dialogueRecordOutOfPlace: the file holds the message's own record and consumed it, but not where
	// the document writes the message: the file's dialogue stands in another order.
	dialogueRecordOutOfPlace
	// dialogueRecordAbsent: the file holds no dialogue record for the message.
	dialogueRecordAbsent
)

// dialogueRecords is the installed file's own dialogue records, in the file's own order, and what the
// walk has resolved against them so far.
//
// The order is the point. The walk resolves a written message the read-back does not show by the file's
// own record -- the reader projects some records away, so its silence is not evidence about them -- and
// a record the file holds *anywhere* answers a question the walk must not ask: two prompts the reader
// projects away would verify against one another swapped, because the file holds both texts and neither
// one's place was ever read. Each written message is therefore resolved against the file's dialogue in
// the file's own order: the message's record is the file's next unconsumed record, a record the file
// holds anywhere else is reported as the difference it is, and a record no written message resolves is
// dialogue the file holds that the document does not write.
type dialogueRecords struct {
	// keys is the file's dialogue records, in the file's own order.
	keys []string
	// pending is where each identity's records still stand, in the file's own order: consuming one takes
	// the first entry, so one record answers for one written message and no record answers twice.
	pending map[string][]int
	// taken marks the records the walk has consumed, wherever they stood.
	taken []bool
	// next is the position the walk stands on, advanced past every record consumed so far.
	next int
}

// newDialogueRecords indexes one file's dialogue records for the walk.
func newDialogueRecords(keys []string) dialogueRecords {
	records := dialogueRecords{
		keys:    keys,
		pending: make(map[string][]int, len(keys)),
		taken:   make([]bool, len(keys)),
	}
	for index, key := range keys {
		records.pending[key] = append(records.pending[key], index)
	}
	return records
}

// holds reports whether the file still holds a record for one written message. The pairing asks it
// before it treats a read-back message as this message's own reading: a reading is evidence about a
// message the file has.
func (records *dialogueRecords) holds(key string) bool {
	return len(records.pending[key]) > 0
}

// hold resolves one written message against the file's own records, consuming the record it answers
// with, if it answers with one.
func (records *dialogueRecords) hold(key string) dialoguePlace {
	if next, ok := records.nextKey(); ok && next == key {
		records.consume(records.next)
		records.next++
		return dialogueRecordInPlace
	}
	if records.holds(key) {
		// The file holds the message's record, but not where the document writes the message. It is
		// consumed where it stands so that one record answers for one written message and the record that
		// does stand here is left for the message that follows.
		records.consume(records.pending[key][0])
		return dialogueRecordOutOfPlace
	}
	return dialogueRecordAbsent
}

// nextKey answers the file's next unconsumed record, advancing the walk's position past the records the
// walk already consumed out of their place.
func (records *dialogueRecords) nextKey() (string, bool) {
	for records.next < len(records.keys) && records.taken[records.next] {
		records.next++
	}
	if records.next >= len(records.keys) {
		return "", false
	}
	return records.keys[records.next], true
}

// consume takes the record at one position: it is marked taken and leaves its identity's queue. The
// queue's front is therefore the first record of that identity the file still holds, which is what makes
// a position in the queue the record's own place.
func (records *dialogueRecords) consume(index int) {
	key := records.keys[index]
	records.taken[index] = true
	records.pending[key] = records.pending[key][1:]
}

// codexInstalledRecord is the part of one installed line this walk reads: the record's own type and, for
// a dialogue message, its payload's type, role and content blocks. It is the writer's shape read from
// the file's bytes rather than from the document the writer kept in memory, and it declares no field for
// the payloads the walk does not ask about (a tool output, a call's arguments), so reading a large
// session does not copy them.
type codexInstalledRecord struct {
	Type    string `json:"type"`
	Payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"payload"`
}

// dialogueMessage answers the identity of the message this record holds, when it holds one the way this
// writer writes it: the record type, the payload's message type, the role and the content block's own
// type are each asked, because a record is the message's model-visible evidence only where every one of
// them is the shape the writer writes. A record whose content holds no such block (an empty text, or a
// different block type) is not the message.
func (record codexInstalledRecord) dialogueMessage() (TargetItemKind, string, bool) {
	if record.Type != codexRecordResponseItem || record.Payload.Type != codexItemMessage {
		return "", "", false
	}
	var kind TargetItemKind
	var contentType string
	switch record.Payload.Role {
	case codexRoleUser:
		kind, contentType = ItemUser, codexContentInputText
	case codexRoleAssistant:
		kind, contentType = ItemAssistant, codexContentOutputText
	default:
		return "", "", false
	}
	for _, block := range record.Payload.Content {
		if block.Type == contentType && block.Text != "" {
			return kind, block.Text, true
		}
	}
	return "", "", false
}

// dialogueWalk is what walking the written dialogue, the read-back and the file's own records together
// found.
type dialogueWalk struct {
	// differences names every written message the file does not hold where the document writes it (one it
	// does not hold at all, or one whose record stands elsewhere in the file), every record the file
	// holds that no written message resolves, and every read-back message no written message accounts
	// for, each by what identifies it rather than by its text.
	differences []string
	// notReadBackVerbatim counts the written messages the read-back does not show with their own text and
	// the installed file's records hold in their own place. It is counted from the read-back's texts, one
	// claim per read-back message (readBackTexts), not from what the pairing made of them.
	notReadBackVerbatim int
}

// compareDialogue walks the written dialogue, the read-back and the installed file's own records
// together, in the document's own order, and names what they differ by.
//
// A written message is accounted for by the read-back's message that is its own text, or by the reader's
// reading of it: the reader deletes the segments it classifies away from a text block (a leading
// injected-context envelope; a plugin envelope in the middle of the block) and never invents characters,
// so what it reads for a written text is a run of that text's own characters in their own order. The
// pairing settles which written message a read-back message is about; it is not what answers for the
// message -- the file's own record is, for the paired message as much as for the ones the walk passes
// over, because the reader reads a block of any of the three types it accepts (`input_text`,
// `output_text`, `text`), so a record this writer would not write can put the message's own words in the
// read-back while the message's own dialogue record is absent from the file.
// The walk pairs the read-back's messages with the written messages they account for, in order, and
// every written message -- the ones left between them, and the paired one itself -- is resolved by the
// file: its own record is what tells the reader's projection (not a loss: nothing else can explain a
// record the reader does not show) from a message the file does not hold. What the count is read from is
// the read-back's texts rather than the pairing's answers: a message the file holds where the document
// writes it is counted when the read-back holds no message with its own text for it
// (readBackTexts), so the two relations the pairing chooses between cannot make one read-back message
// stand for two written ones.
//
// The file's records are read in their own order, and a message is resolved against the record that
// stands where the document writes it: a record the file holds anywhere else is a file whose dialogue
// stands in another order, which the read-back cannot report for the messages the reader projects away,
// and a record no written message resolves is dialogue the file holds that the document does not write.
// `records` is consumed as the walk resolves messages, so one record answers for one written message:
// the same prompt recorded twice in the source needs two records in the file.
func compareDialogue(intended, read []string, records dialogueRecords) dialogueWalk {
	var walk dialogueWalk
	// The read-back's own texts, one claim each: the count asks whether the read-back shows this message's
	// own text, which is a question about the texts rather than about the pairing -- see readBackTexts.
	shown := newReadBackTexts(read)
	// account resolves one written message against the file's own records, names the difference where
	// the file's answer is one, and answers whether the message's record was the file's next unconsumed
	// record -- the shape the reader's projection leaves behind, which is what gates notReadBackVerbatim.
	account := func(ordinal int) bool {
		key := intended[ordinal]
		switch records.hold(key) {
		case dialogueRecordInPlace:
			return true
		case dialogueRecordOutOfPlace:
			walk.differences = append(walk.differences, fmt.Sprintf(
				"%s %d is out of order in the file: %s", categoryDialogue, ordinal+1,
				describeItem(categoryDialogue, key)))
		default:
			walk.differences = append(walk.differences, fmt.Sprintf(
				"%s %d is in the target document but not in the file: %s",
				categoryDialogue, ordinal+1, describeItem(categoryDialogue, key)))
		}
		return false
	}
	// Every written message is resolved exactly once, in the document's own order, whatever the pairing
	// did with it: the message's own record being consumed where the document writes it is what the count
	// is gated on, and the read-back's texts are claimed in the same order.
	resolve := func(ordinal int) {
		if account(ordinal) && !shown.claim(intended[ordinal]) {
			walk.notReadBackVerbatim++
		}
	}
	cursor := 0
	for index, readKey := range read {
		paired := pairReadBackMessage(intended, cursor, readKey, &records)
		switch {
		case paired >= 0:
			for skipped := cursor; skipped < paired; skipped++ {
				resolve(skipped)
			}
			resolve(paired)
			cursor = paired + 1
		case cursor < len(intended) && !records.holds(intended[cursor]):
			walk.differences = append(walk.differences, fmt.Sprintf(
				"%s %d differs (written %s, read back %s)", categoryDialogue, cursor+1,
				describeItem(categoryDialogue, intended[cursor]), describeItem(categoryDialogue, readKey)))
			cursor++
		default:
			walk.differences = append(walk.differences, fmt.Sprintf(
				"%s %d is in the file but was not written: %s",
				categoryDialogue, index+1, describeItem(categoryDialogue, readKey)))
		}
	}
	for ordinal := cursor; ordinal < len(intended); ordinal++ {
		resolve(ordinal)
	}
	// A record the walk never resolved is dialogue the file holds that the document does not write. The
	// read-back is not asked: a record the reader projects away reaches no read-back message at all, so
	// the file's own records are the only side that can report it.
	for index, key := range records.keys {
		if records.taken[index] {
			continue
		}
		walk.differences = append(walk.differences, fmt.Sprintf(
			"%s record %d is in the file but was not written: %s",
			categoryDialogue, index+1, describeItem(categoryDialogue, key)))
	}
	return walk
}

// pairReadBackMessage answers the written message the read-back's message accounts for: the first written
// message at or after `from` whose own text it is, or whose reading it is -- a run of that message's
// characters in their own order, where the installed file still holds a record for it (the reader reads
// a written message only as far as the file holds it, so a reading is evidence about a message the file
// has). A message neither relation answers for has -1.
func pairReadBackMessage(intended []string, from int, readKey string, records *dialogueRecords) int {
	for index := from; index < len(intended); index++ {
		key := intended[index]
		if key == readKey {
			return index
		}
		if !records.holds(key) {
			continue
		}
		kind, text, _ := strings.Cut(key, "\x00")
		if readsAsDialogueMessage(kind, text, readKey) {
			return index
		}
	}
	return -1
}

// readBackTexts is the read-back's own messages as the count asks about them: one claim per read-back
// message, so a written message is shown with its own text exactly as often as the read-back holds a
// message with that text.
//
// The count asks a question about texts -- "does the read-back show this message's own text?" -- while
// the pairing answers a different one: which written message a read-back message is *about*. The two
// relations the pairing chooses between can overlap, and a read-back message can be both a reading of an
// earlier written message and the own text of a later one; an omitted `# AGENTS.md` envelope in front of a
// later turn carrying the same text is exactly that shape. Reading the count off the pairing made the
// pairing's choice the count's, so that shape reported the envelope's message as projected *and* the
// later turn as never read back -- two messages where the read-back does not show one (review round 11's
// finding 2). Claims are what the count is read from instead, and no choice of pairing moves them: the
// walk resolves the written messages in the document's own order and claims the read-back's texts in it,
// so a text the document writes twice needs the read-back to hold it twice, and which of two identical
// written messages the one read-back message is said to show cannot change either answer.
type readBackTexts struct {
	claims map[string]int
}

// newReadBackTexts counts one read-back's dialogue keys.
func newReadBackTexts(read []string) readBackTexts {
	claims := make(map[string]int, len(read))
	for _, key := range read {
		claims[key]++
	}
	return readBackTexts{claims: claims}
}

// claim takes the read-back message showing this key, when the read-back still holds one, and answers
// whether it did.
func (texts readBackTexts) claim(key string) bool {
	if texts.claims[key] == 0 {
		return false
	}
	texts.claims[key]--
	return true
}

// readsAsDialogueMessage reports whether the read-back's message is the reader's own reading of a written
// message: a run of the written message's characters in their own order, recorded under the message's own
// kind. A read-back message the reader recorded under another role is not this message's reading, and a
// text equal to the message's own is the message itself rather than a reading of it (the walk pairs that
// case first).
func readsAsDialogueMessage(kind, written, readKey string) bool {
	readKind, readText, _ := strings.Cut(readKey, "\x00")
	if readKind != kind || readText == "" || readText == written {
		return false
	}
	return holdsTextInOrder(written, readText)
}

// holdsTextInOrder reports whether every rune of read occurs in written, in read's own order. A run with
// gaps in it is what a deletion of segments leaves behind, which is how the reader reads a block it
// classifies part of away; characters the written text does not hold, or the same characters out of
// their order, are not a run of it.
func holdsTextInOrder(written, read string) bool {
	position := 0
	for _, character := range read {
		found := false
		for position < len(written) {
			next, size := utf8.DecodeRuneInString(written[position:])
			position += size
			if next == character {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// roundTripCallKey identifies a call the way both sides spell it. An empty argument object and no
// argument object are the same evidence: the inherited reader drops "{}" while reading, so a call whose
// input the source did not record would otherwise be reported as a difference the file does not have.
func roundTripCallKey(callID, name, inputJSON string) string {
	input := strings.TrimSpace(inputJSON)
	if input == "{}" || input == "[]" {
		input = ""
	}
	return callID + "\x00" + name + "\x00" + input
}

// roundTripResultKey identifies a result by its call and the text it holds, trimmed the way the
// inherited reader trims a result's output.
func roundTripResultKey(callID, text string) string {
	return callID + "\x00" + strings.TrimSpace(text)
}

// roundTripBoundaryKey identifies a compaction boundary by its summary text, trimmed the way the
// inherited reader trims a compacted record's message: a summary that ends in whitespace, or holds
// nothing but whitespace, is the same evidence once read as one that does not, and comparing the
// untrimmed strings would report the reader's own normalisation as a difference the file does not have.
func roundTripBoundaryKey(text string) string {
	return strings.TrimSpace(text)
}

// sequenceComparison says how a comparison treats item order: dialogue and compaction boundaries are
// compared in the conversation's order, while calls and results are compared as multisets, because the
// target's reader attaches a call's output to the call's own message, so their position in the read-back
// is the reader's arrangement rather than the file's.
type sequenceComparison int

const (
	compareInOrder    sequenceComparison = iota // the two sides must hold the items in the same order
	compareAsMultiset                           // the two sides must hold the same items, order free
)

// The categories a difference report names, spelled once: describeItem takes the shape of the item's
// own key from the category, because a dialogue key and a tool result key both hold two fields.
const (
	categoryDialogue           = "dialogue"
	categoryToolCall           = "tool call"
	categoryToolResult         = "tool result"
	categoryCompactionBoundary = "compaction boundary"
)

// compareSequences names the items one side holds and the other does not, in the order the comparison
// asks for.
func compareSequences(category string, order sequenceComparison, intended, read []string) []string {
	if len(intended) > 0 && order == compareAsMultiset {
		intended = sortedCopy(intended)
		read = sortedCopy(read)
	}
	var differences []string
	for index := 0; index < len(intended) || index < len(read); index++ {
		switch {
		case index >= len(read):
			differences = append(differences, fmt.Sprintf("%s %d is in the target document but not in the file: %s",
				category, index+1, describeItem(category, intended[index])))
		case index >= len(intended):
			differences = append(differences, fmt.Sprintf("%s %d is in the file but was not written: %s",
				category, index+1, describeItem(category, read[index])))
		case intended[index] != read[index]:
			differences = append(differences, fmt.Sprintf("%s %d differs (written %s, read back %s)",
				category, index+1, describeItem(category, intended[index]), describeItem(category, read[index])))
		}
	}
	return differences
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

// describeItem names one compared item without carrying its text. A difference report reaches the JSON
// document and the warnings a caller reads, and the items compared here are dialogue, tool arguments and
// tool outputs -- so the report holds what identifies an item (its kind, the call it belongs to and the
// tool it names) and, for the text itself, its size and digest: two texts are told apart, and a caller
// can match a digest against the text outside this program without the report ever holding the words.
func describeItem(category, item string) string {
	identity, rest, _ := strings.Cut(item, "\x00")
	switch category {
	case categoryDialogue:
		return fmt.Sprintf("%s (%s)", identity, describeText(rest))
	case categoryToolCall:
		tool, arguments, _ := strings.Cut(rest, "\x00")
		return fmt.Sprintf("call %s to %s (%s)", identity, tool, describeText(arguments))
	case categoryToolResult:
		if parts := strings.SplitN(item, "\x00", 7); len(parts) == 7 && parts[0] == "paired" {
			// An owner-annotated result: "paired", the owning call's id, tool and arguments, the
			// occurrence of that invocation under the call's id, the result's own call id and its text.
			// A tool name, argument object or output containing a NUL byte can shift the fields, exactly
			// as it can in the call key above; the occurrence is named only when the field really is one
			// (a `#<number>`), and nothing here ever carries the text, which is reported by size and
			// digest only. The occurrence itself is spelled only when it is not the id's first: an
			// invocation the id records once has nothing to disambiguate, and every such report keeps
			// the text it has always had.
			described := fmt.Sprintf("call %s's result (%s) written for the invocation of call %s to %s",
				parts[5], describeText(parts[6]), parts[1], parts[2])
			if occurrence, err := strconv.Atoi(strings.TrimPrefix(parts[4], "#")); err == nil && occurrence > 1 {
				described += fmt.Sprintf(" (occurrence %d)", occurrence)
			}
			return described
		}
		return fmt.Sprintf("call %s's result (%s)", identity, describeText(rest))
	default:
		return fmt.Sprintf("summary (%s)", describeText(item))
	}
}

// describeText says what can be said about one piece of compared text without quoting any of it: its
// size in bytes and its SHA-256, both taken over the text as the comparison holds it, so that a reader
// told two items differ can tell which texts they were and check them against the source.
func describeText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%d bytes, sha256 %s", len(text), hex.EncodeToString(sum[:]))
}
