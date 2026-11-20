package ccc

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// A Pi session is a tree of entries linked by `id`/`parentId`, and the entry the harness's own loader
// selects as the conversation's leaf is the *file's* last entry: the loader's `buildContextEntries()`
// walks from the current leaf back to the root through `parentId`, and a session written by appending
// entries leaves the leaf last (Pi 1.5.2, `docs/session-format.md`, "Tree Structure" and "Context
// Building"). The load check re-derives that walk from a converted file's own bytes the same way
// (piModelVisibleRoles), so a conversion's own file has to leave the source's leaf as its last entry.
//
// The carried Pi parser does not answer that leaf. It drops the entry kinds that are not messages or
// compaction boundaries, and two of the kinds it drops are ones the harness's own loader *does* build
// context from: a `branch_summary` becomes a `branchSummary` message and a `custom_message` becomes a
// `CustomMessage` (the same document's `buildSessionContext()`). So a read's last record is the last
// record the *read* holds, which is not necessarily the record the source's file selects: a session the
// user switched back to an earlier branch with `/tree` ends with a `branch_summary` entry whose
// `parentId` is the entry the continued branch hangs off, while the abandoned branch's own last message
// is still the read's last record. A conversion written from there would resume a branch the source's
// conversation left -- and every per-record count in the report would stay silent about it, because the
// records on that branch are records the read carried and the writer wrote.
//
// The check below therefore reads the *source file's own entries* (the same entry shape and reader the
// load check reads a converted file with) and refuses the conversion when the file's own answer does not
// support the record the read's last message stands for. It is the source-side half of the leaf rule
// (selectedLeafRefusal, ErrUnrepresentableLeaf); the writer's half is unchanged.
//
// An entry on that path the read does not carry is refused unless the harness's own loader keeps its kind
// out of the conversation, and one record shape is a third case: Pi's own system record, which the
// harness writes into every session it has run a turn in and its loader builds a message from, and which
// the inherited reader does not carry. It is the harness's own prompt and tool loadout rather than a turn,
// so the conversion accounts for it as an omitted record (piAccountedSourceOmission, piSystemRecordLoss)
// instead of refusing every session that a real Pi run has extended.

// piSourceLeafProviders names the providers whose reader is the carried Pi parser, which is the reader
// that drops the tree entries the harness's own loader walks. The list is spelled here because the
// inherited registry exposes no format-family predicate (it has no Pi counterpart to IsCodexFormatAgent),
// and the carried parser is not edited here: it is the family projectedParentProviders names beside the
// Pi-derived readers, without `tau` -- the Tau reader replays the ancestry its own `index.jsonl` leaf
// selects (internal/parser/tau.go), so its read already answers the record the source's loader would walk
// from.
var piSourceLeafProviders = map[string]bool{
	"pi":          true,
	"omp":         true,
	"omo":         true,
	"stepcode":    true,
	"prime-agent": true,
	"direct-api":  true,
}

// piSourceLeafProvider reports whether a source read through this provider is read with the carried Pi
// parser.
func piSourceLeafProvider(provider string) bool {
	return piSourceLeafProviders[strings.ToLower(strings.TrimSpace(provider))]
}

// piEntryKindsOutsideTheContext names the entry kinds the harness's own loader keeps out of the
// conversation it builds, so a dropped entry of one of them standing on the leaf's path asks nothing of
// the read. Each is the carried parser's own or a kind the pinned session-format document places outside
// the model context: `model_change` and `thinking_level_change` (settings, no context message), `usage`
// ("does not participate in LLM context"), `custom` ("Does NOT participate in LLM context"), `label` (a
// bookmark), `session_info` (session metadata), and the two entries the OMP parser reads by name
// (`title`, the rewritable title slot line, and `child_usage_attributed`, a usage attribution).
//
// A kind this map does not name is not assumed to be outside the context: `branch_summary` and
// `custom_message` are in it -- the harness's own loader builds a message from either -- and so is
// `context_edit`, which produces no message of its own but changes what the context holds for an entry
// the read did carry. An entry of a kind this build does not know is refused for the same reason: nothing
// here can say the source's own conversation does not hold it.
var piEntryKindsOutsideTheContext = map[string]bool{
	"model_change":           true,
	"thinking_level_change":  true,
	"usage":                  true,
	"custom":                 true,
	"label":                  true,
	"session_info":           true,
	"title":                  true,
	"child_usage_attributed": true,
}

// piCarriedMessageRoles names the message roles the carried Pi parser keeps as read records. A message
// entry of any other role is an entry the read does not hold, and a message is a record the harness's own
// loader builds context from whatever its role.
var piCarriedMessageRoles = map[string]bool{
	piRoleUser:       true,
	piRoleAssistant:  true,
	piRoleToolResult: true,
}

// piCarriedSourceEntry reports whether one entry is a record kind the read carries: a message of a role
// the carried parser keeps, or the boundary record a compaction is read as.
func piCarriedSourceEntry(kind, role string) bool {
	if kind == piEntryCompaction {
		return true
	}
	return kind == piEntryMessage && piCarriedMessageRoles[strings.TrimSpace(role)]
}

// piSystemMessageRole is the role of the record in which Pi replays its own prompt and tool loadout: the
// first request of a session persists one holding every prompt section and tool declaration, and later
// changes persist more that patch `sections` by name and list `toolsAdded`/`toolsRemoved` (Pi 1.5.2,
// `docs/session-format.md`, "message"). Pi itself writes these records into sessions it has run a turn
// in, and its own loader builds a message from them, so an ordinary native session holds one on its
// leaf path.
const piSystemMessageRole = "system"

// piAccountedSourceOmission reports whether one entry is a record the conversion accounts for as omitted
// rather than refuses: a `message` entry whose role the carried Pi parser drops by design and whose shape
// this build can name (the harness's own system record, `role: "system"`).
//
// The record is the harness's own prompt state rather than a turn of the conversation: its `content` is
// empty and its payload is the prompt sections and tool declarations Pi replays to rebuild the system
// prompt, which no target this pipeline writes has a place for and no other harness has a use for -- a
// target harness resumes under its own prompt, and Pi's own loader rebuilds a session's prompt from its
// configuration when a session records no system message at all. The inherited Pi parser does not carry
// the role into the read (internal/parser/pi.go keeps the message roles user, assistant and toolResult),
// so what the conversion writes is the conversation the source recorded without that state, and the
// omission is reported as its own loss category (piSystemRecordLoss) rather than silently dropped. It is
// not refused: refusing would make every session Pi itself has run a turn in unconvertible, and what is
// omitted is the harness's own bookkeeping, not a record whose words the source's conversation holds.
//
// A role this predicate does not name is not accounted for: a message record of any other role on the
// leaf path is context this build cannot say it may drop, and is refused exactly as before.
func piAccountedSourceOmission(kind, role string) bool {
	return kind == piEntryMessage && strings.TrimSpace(role) == piSystemMessageRole
}

// piSystemRecordLoss is the loss entry that reports the system records this build's read cannot carry
// (piAccountedSourceOmission): the count of the records the source file's own leaf path holds that the
// read does not, and the sentence that says what they are and why no target entry holds them. It is built
// here because the file's own entries are where the count comes from -- the read's own answer never held
// the record, so no target writer can count it -- and it travels on every conversion whose source file
// held one.
func piSystemRecordLoss(count int) Loss {
	return Loss{
		Category:    "source_system_records",
		Disposition: DispositionOmitted,
		Count:       count,
		Detail: "a Pi system record is where the harness replays its own prompt and tool loadout: the " +
			"first request of a session persists one holding every prompt section and tool declaration, " +
			"and later changes persist more that patch `sections` and list `toolsAdded`/`toolsRemoved` " +
			"(Pi 1.5.2, docs/session-format.md, \"message\"). The inherited Pi reader does not carry the " +
			"role into the read and no target of this conversion has that state to hold -- a target " +
			"harness resumes under its own prompt, and Pi rebuilds a session's prompt from its " +
			"configuration when the session records no system message -- so the count is the number of " +
			"such records the source file's own leaf path holds that this read does not carry. The " +
			"conversation's own turns are unaffected: the record is the harness's prompt bookkeeping, " +
			"not a turn",
	}
}

// piSourceEntryLineMax bounds one line of a source session the entry scan reads. It is the bound the
// carried Pi parser reads a line with (internal/parser's maxLineSize): a longer line is a line the read
// could not have read either, so the scan stops and answers the read error rather than skipping it and
// judging the file by the entries before it.
const piSourceEntryLineMax = 64 << 20

// piSourceLeafAnswer is what the source-side leaf check answers, in the two channels a conversion's report
// has for it.
type piSourceLeafAnswer struct {
	// Warning states the record the source's own loader selects as the conversation's leaf was not
	// established from the source's own bytes, and why. Empty when the file answered the question.
	Warning string
	// Losses names the records the source's own leaf path holds that this read does not carry and the
	// conversion accounts for as omitted (piSystemRecordLoss): the count comes from the file's own
	// entries, which the read's own answer does not hold, so the check is where it is measured. Empty for
	// a source whose file held none.
	Losses []Loss
}

// piSourceFileAnswer is one read of a source session's own entries for a Pi-family conversion, and the
// two answers the conversion takes from that read: the file's own leaf-path membership (lineMembership),
// which a selection's line flags are taken from, and the leaf check (check). One read serves both -- the
// conversion parses its source file once -- and the read happens before the selection runs, while the
// answers are only returned where the conversion's own order says they are (check is asked after the
// selection, exactly as piSourceLeafCheck was: a specification the read's own ordinals refuse is a
// statement about the caller's command line and must not be reported as something about the source file).
type piSourceFileAnswer struct {
	// piFamily says the source's reader is the carried Pi parser, so a file is the question's subject at
	// all; a provider outside the family answers nothing, silently, because its read is not a
	// reconstruction of a file the harness's own loader walks.
	piFamily bool
	// readable says the file stood behind the read and its entries were read: readFrom is the source,
	// the boundary was taken, the path is known, the file is the size the read's own snapshot recorded,
	// and piSourceEntries could read it.
	readable bool
	// unestablished names why no file answered (the report's warning detail), empty when one did.
	unestablished string
	// path, entries and byID are the file as it was read: its path, its entries in the file's order, and
	// the index from an identity to its entry.
	path    string
	entries []piConversationEntry
	byID    map[string]int
	// err is the file read's own failure, answered by check where piSourceLeafCheck answered it.
	err error
}

// readPiSourceFile reads the session behind one Pi-family read, once. The gates are the leaf check's own
// (and stated there): the read came from the file itself (not from an archive row, which is a projection
// with no file open, built by the archive's own ingest) and the file's bytes are the size the read's own
// snapshot recorded -- a size match, which is what that record can show, exactly as the boundary's own
// use of the same number states (fillReadCoverage). Anywhere else no file answers, and the report says
// exactly that instead of presenting the read's own last record as the source's leaf.
func readPiSourceFile(conversation Conversation, readFrom string, boundary SourceBoundary) piSourceFileAnswer {
	if !piSourceLeafProvider(conversation.Source.Provider) {
		return piSourceFileAnswer{}
	}
	answer := piSourceFileAnswer{piFamily: true}
	if readFrom != "source" || boundary.State != SourceBoundaryTaken || strings.TrimSpace(boundary.Path) == "" {
		answer.unestablished = sourceLeafNotMeasuredDetail(readFrom, boundary)
		return answer
	}
	if size := conversation.Source.FileSize; size > 0 && boundary.Bytes != size {
		answer.unestablished = fmt.Sprintf(
			"the source file holds %d byte(s) while the read's own snapshot of this file recorded %d, so the "+
				"file moved while it was being read and the entry its own bytes end with is not shown to be "+
				"the entry this read's last record stands for", boundary.Bytes, size)
		return answer
	}
	entries, byID, err := piSourceEntries(boundary.Path)
	if err != nil {
		answer.err = err
		return answer
	}
	answer.readable = true
	answer.path = boundary.Path
	answer.entries = entries
	answer.byID = byID
	return answer
}

// lineMembership answers, per record of the read, the source file's own answer about the read's line:
// whether the walk the harness's own loader makes -- from the file's last entry back through `parentId`,
// the walk its context is built from -- reaches the record, for the records the read answered an identity
// for. nil where the file answered nothing (no file stands behind the read, a file that holds no
// entries), and every record the read gave no identity for is left unanswered whoever else answered: the
// read's own order (inactiveSourceRecords) is the only answer such a record has, because nothing can name
// an identity-less entry as a parent -- only a file's own last entry could hold one, and a carried record
// there is the source-side leaf check's own refusal.
func (answer piSourceFileAnswer) lineMembership(conversation Conversation) []sourceFileLineAnswer {
	if !answer.readable || len(answer.entries) == 0 {
		return nil
	}
	onPath := make(map[string]bool, len(answer.entries))
	current := len(answer.entries) - 1
	// The walk is bounded by the entry count, the leaf check's own bound: a parent the file does not
	// hold, a record naming itself and a loop each end where the count runs out -- the only bound the
	// walk needs and the only one it has.
	for steps := 0; steps <= len(answer.entries); steps++ {
		if id := strings.TrimSpace(answer.entries[current].ID); id != "" {
			onPath[id] = true
		}
		parent := parentOfPiEntry(answer.entries[current])
		if parent == "" {
			break
		}
		next, ok := answer.byID[parent]
		if !ok {
			break
		}
		current = next
	}
	held := make(map[string]bool, len(conversation.Messages))
	for _, message := range conversation.Messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			held[id] = true
		}
	}
	line := make([]sourceFileLineAnswer, len(conversation.Messages))
	for index, message := range conversation.Messages {
		id := strings.TrimSpace(message.SourceUUID)
		if id == "" {
			continue
		}
		recordAnswer := sourceFileLineAnswer{holds: onPath[id], answered: true}
		// The record's own place in the file's tree travels beside its membership: the record the file's
		// own chain names as its nearest ancestor the read holds (piFileParent, which steps through the
		// file's metadata entries the way the harness's own loader does). The read may have answered that
		// relation as a projection (a record the file placed under a tool result reads as hanging off the
		// message that made the call), and a tree writer holding this answer chains the entry to the
		// record the file named.
		if at, ok := answer.byID[id]; ok {
			recordAnswer.parent = piFileParent(answer.entries, answer.byID, held, answer.entries[at])
		}
		line[index] = recordAnswer
	}
	return line
}

// piFileParent answers the nearest ancestor record one entry's own file chain names that the read holds
// an identity for, stepping through the entries the harness's own loader keeps out of the conversation
// (piEntryKindsOutsideTheContext) and the records the conversion accounts for as omitted
// (piAccountedSourceOmission). Pi writes metadata rows into the tree -- a `model_change` may stand
// between a tool result and the reply that names it as its parent, and the loader's own walk passes
// through it -- and a raw `parentId` answer for such a record names an entry this read never holds. A
// consumer left with the read's own answer instead takes the projection (the reply reads as hanging off
// the message that made the call), and the tree writer's tool-result step reaches the *last* result of
// that call -- a later sibling the file's own leaf path leaves off -- so the reply is chained past the
// result the file recorded. The file's own chain settles it: the walk steps through the entries the
// source's conversation context does not hold and answers the first ancestor the read does, which is the
// record the source's own loader would walk through. "" where the chain reaches an entry this build
// cannot step through (a kind nothing here can say the conversation leaves out) or runs out (a root, a
// parent the file does not hold, a loop): the caller's other places decide then, exactly as they did
// before the file answered anything.
func piFileParent(
	entries []piConversationEntry, byID map[string]int, held map[string]bool, entry piConversationEntry,
) string {
	parent := parentOfPiEntry(entry)
	for steps := 0; parent != "" && steps <= len(entries); steps++ {
		at, ok := byID[parent]
		if !ok {
			return ""
		}
		if held[parent] {
			return parent
		}
		ancestor := entries[at]
		if !piEntryKindsOutsideTheContext[ancestor.Type] &&
			!piAccountedSourceOmission(ancestor.Type, ancestor.Message.Role) {
			return ""
		}
		parent = parentOfPiEntry(ancestor)
	}
	return ""
}

// check answers what one conversion could establish about the record the source's own loader selects as
// the conversation's leaf, from the entries readPiSourceFile read. Three answers travel: the warning the
// report carries when the source's own bytes were not what the check could read, the omitted records the
// conversion accounts for (piAccountedSourceOmission), and the refusal when the file's own answer does
// not support the record the read's last message stands for. An archive read is also where a system
// record cannot be counted: the row holds the read's own answer, which never carried the record.
func (answer piSourceFileAnswer) check(conversation Conversation) (piSourceLeafAnswer, error) {
	if answer.err != nil {
		return piSourceLeafAnswer{}, answer.err
	}
	if !answer.readable {
		if !answer.piFamily {
			return piSourceLeafAnswer{}, nil
		}
		return piSourceLeafAnswer{Warning: fmt.Sprintf(
			"the entry the source's own loader selects as the conversation's leaf was not established: %s, "+
				"so the conversion writes the line of the read's own last record and no source file answers "+
				"which entry the harness's own loader would walk from", answer.unestablished)}, nil
	}
	omitted, err := piSourceLeafRefusalFrom(conversation, answer.path, answer.entries, answer.byID)
	if err != nil {
		return piSourceLeafAnswer{}, err
	}
	if omitted > 0 {
		return piSourceLeafAnswer{Losses: []Loss{piSystemRecordLoss(omitted)}}, nil
	}
	return piSourceLeafAnswer{}, nil
}

// piSourceLeafCheck answers what one conversion could establish about the record the source's own loader
// selects as the conversation's leaf: one read of the file's entries (readPiSourceFile), then the check
// asked of it. The conversion itself asks the same two steps separately, because a selection consults
// the same read's line membership before the check runs.
func piSourceLeafCheck(conversation Conversation, readFrom string, boundary SourceBoundary) (piSourceLeafAnswer, error) {
	return readPiSourceFile(conversation, readFrom, boundary).check(conversation)
}

// sourceLeafNotMeasuredDetail names why no source file stood behind this read: the read's own answer when
// it did not come from a source file at all, and otherwise the boundary pass's own reason for not measuring
// one (which is where "no file holds this conversation's bytes" and "the read reported no path" are
// already stated), else a sentence for a conversation no boundary pass answered for.
func sourceLeafNotMeasuredDetail(readFrom string, boundary SourceBoundary) string {
	if readFrom != "source" {
		return fmt.Sprintf("the conversion read from %s rather than from a source file of its own", readFrom)
	}
	if detail := strings.TrimSpace(boundary.Detail); detail != "" {
		return detail
	}
	return "no source file of its own was measured for this conversation"
}

// piSourceLeafRefusal reads one source session's own entries and answers whether they support the record
// this read's last message stands for (piSourceLeafRefusalFrom, which holds the rule and every sentence
// of the refusals). It reads the file itself, which the conversion does not need twice: continueWith hands
// the same entries to piSourceLeafRefusalFrom through its own read (readPiSourceFile).
func piSourceLeafRefusal(conversation Conversation, path string) (int, error) {
	entries, byID, err := piSourceEntries(path)
	if err != nil {
		return 0, err
	}
	return piSourceLeafRefusalFrom(conversation, path, entries, byID)
}

// piSourceLeafRefusalFrom answers whether the source file's own answer supports the record this read's
// last message stands for: nil when it does, and a refusal naming the difference when it does not.
//
// The file's own leaf path is walked from the file's last entry back through `parentId` -- the harness's
// own walk -- and every entry on it is asked about:
//
//   - a record this read answered an identity for is a record the conversion can place, and the *first*
//     one the walk reaches (the deepest, since the walk runs leaf to root) must be the record this read's
//     last message stands for, because that is the record the target's loader would walk from;
//   - an entry this read did not answer an identity for is a record the conversion cannot place, and it
//     may stand on the path only when the harness's own loader keeps its kind out of the conversation's
//     context (piEntryKindsOutsideTheContext) or when it is a record the conversion accounts for as
//     omitted (piAccountedSourceOmission), which is counted and reported rather than refused.
//
// The answer is the number of records the walk counted as omissions, so the report can name them; every
// refusal answers zero, because a refused conversion reports no conversion.
//
// A file that records no entry identities at all -- a Pi v1 session, which the inherited reader accepts and
// derives an identity for from the filename -- answers its own order rather than a tree: it cannot hold a
// branch, because a branch is a `parentId` link, so the entry the harness's loader selects is the file's
// last entry and the conversation is the file's own order. What the read's own last record stands for is
// therefore asked by order: the last entry whose kind is one the read carries is the record its last
// message stands for, and every entry after that one must be an entry the harness's loader keeps out of
// the context or one the conversion accounts for as omitted. A walk that ends where the file does (a root,
// a parent the file does not hold, a record naming itself, a loop) answers what it reached.
//
// A read that holds no record at all is asked the same question of the same file: there is no leaf of its
// own that could be resumed wrongly, but the file's entries are still the source's, and the harness's own
// loader would build a context from them (piEmptyReadRefusal). A file with no entry beyond its header
// selects nothing, and both of these answers are nil.
func piSourceLeafRefusalFrom(conversation Conversation, path string, entries []piConversationEntry, byID map[string]int) (int, error) {
	messages := conversation.Messages
	if len(entries) == 0 {
		// Nothing but a session header (and possibly a title slot line): the file selects no entry at all,
		// so no branch of its own can be resumed in place of this read's last record.
		return 0, nil
	}
	if len(messages) == 0 {
		return piEmptyReadRefusal(path, entries, byID)
	}
	carried := make(map[string]bool, len(messages))
	for _, message := range messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			carried[id] = true
		}
	}
	lastID := strings.TrimSpace(messages[len(messages)-1].SourceUUID)

	leaf := entries[len(entries)-1]
	if len(byID) == 0 {
		omitted := 0
		for index := len(entries) - 1; index >= 0; index-- {
			entry := entries[index]
			if piCarriedSourceEntry(entry.Type, entry.Message.Role) {
				return omitted, nil
			}
			if piAccountedSourceOmission(entry.Type, entry.Message.Role) {
				omitted++
				continue
			}
			if !piEntryKindsOutsideTheContext[entry.Type] {
				return 0, fmt.Errorf("%w: the source's own file %s records no entry identities, and the %s "+
					"record it holds after its last message-like record (entry %d of %d, role %s) is one "+
					"this read does not carry and whose kind this build cannot say the harness's own "+
					"conversation context leaves out, so the conversation a resumed target would read is "+
					"not the one the source's loader builds",
					ErrUnrepresentableLeaf, path, entryKind(entry.Type), index+1, len(entries),
					entryRole(entry.Message.Role))
			}
		}
		return 0, fmt.Errorf("%w: the source's own file %s records no entry identities and holds no record "+
			"kind this read carries, so the entry the source's own loader selects as the conversation's leaf "+
			"cannot be represented", ErrUnrepresentableLeaf, path)
	}

	// The walk: from the file's last entry back through parentId, the way the harness's own loader walks. A
	// parent the file does not hold, a record naming itself and a loop each end where the entry count runs
	// out, which is the only bound the walk needs and the only one it has: a cycle the source recorded
	// (a record naming itself, or a longer loop) is re-walked until the file's own entry count is spent and
	// then answered as what it reached. The walk's own answers are idempotent, which is what lets it
	// re-reach an entry of a cycle without a visited set -- except for the omission count, which the
	// visited set below keeps idempotent too: a record counted once is counted once, however often the
	// walk reaches it (the writers' walk, writtenAncestor, makes the same choice for the same reason).
	current := len(entries) - 1
	deepest := ""
	omitted := 0
	counted := make(map[int]bool, 1)
	for steps := 0; steps <= len(entries); steps++ {
		entry := entries[current]
		currentID := strings.TrimSpace(entry.ID)
		switch {
		case carried[currentID]:
			if deepest == "" {
				deepest = currentID
			}
		case piAccountedSourceOmission(entry.Type, entry.Message.Role):
			if !counted[current] {
				counted[current] = true
				omitted++
			}
		case !piEntryKindsOutsideTheContext[entry.Type]:
			// The entry is not one this read answered an identity for -- so the conversion cannot place it
			// -- and nothing here knows the harness's own loader keeps its kind out of the context either.
			return 0, fmt.Errorf("%w: the source's own file %s selects the leaf %s, and that leaf's own "+
				"path holds a %s record (id %s, role %s) which this read does not carry and whose kind "+
				"this build cannot say the harness's own conversation context leaves out, so the "+
				"conversation a resumed target would read is not the one the source's loader builds",
				ErrUnrepresentableLeaf, path, sourceLeafName(leaf), entryKind(entry.Type),
				entryIdentity(currentID), entryRole(entry.Message.Role))
		}
		parent := parentOfPiEntry(entry)
		if parent == "" {
			break
		}
		next, ok := byID[parent]
		if !ok {
			break
		}
		current = next
	}
	switch {
	case deepest == "":
		return 0, fmt.Errorf("%w: the source's own file %s selects the leaf %s, and that leaf's own path "+
			"reaches no record this read holds, while the read's last record is %s: a resumed session "+
			"would continue another branch than the one the source's conversation ends on",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), entryIdentity(lastID))
	case lastID == "":
		return 0, fmt.Errorf("%w: the source's own file %s selects the leaf %s, whose own path reaches the "+
			"record %s, while this read answered no identity for its last record, so which entry of the "+
			"source's own file that record stands for is not established",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), deepest)
	case deepest != lastID:
		return 0, fmt.Errorf("%w: the source's own file %s selects the leaf %s, whose own path reaches the "+
			"record %s, while this read's last record is %s: a resumed session would continue a branch "+
			"the source's conversation left",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), deepest, lastID)
	}
	return omitted, nil
}

// piEmptyReadRefusal answers whether a file can stand behind a read that holds no record at all: nil when
// the harness's own loader would build no conversation from the file either, and a refusal naming the
// entry where it would. It also answers the number of records it counted as omissions the conversion
// accounts for (piAccountedSourceOmission), which is zero for every file it does not refuse.
//
// An empty read is not by itself a defect -- a conversation that recorded no message and a file that
// holds nothing beyond its header are both real, and neither can be resumed wrongly -- but the file's
// entries are still the source's, and an empty read is an answer the conversion would write as an empty
// session: a *dropped* record is what this case may not pass over, because the read's own empty answer
// cannot count it as a loss. The carried Pi parser skips `custom_message` and `branch_summary` (and any
// kind this build does not know), while the harness's own loader builds a message from the first two, so
// a file holding one on the path its loader walks from is context the conversion would install an empty
// session in place of -- the same dropped-context shape the walk above refuses for a read that holds
// records, with nothing carried to reconcile it against. A system record of the same path is the one shape
// whose omission *is* accounted for (piAccountedSourceOmission): it is the harness's own prompt state and
// the count travels on the conversion like any other reported omission.
//
// The file is asked the question its own shape supports: a file that records entry identities is asked
// about the path its loader walks from (the file's last entry and its ancestors), exactly as the walk
// above asks it, because an entry off that path is recorded history the loader does not build context
// from; a file that records none is asked by order, because that is the shape its answer takes (above).
func piEmptyReadRefusal(path string, entries []piConversationEntry, byID map[string]int) (int, error) {
	if len(byID) == 0 {
		omitted := 0
		for index, entry := range entries {
			if piEntryKindsOutsideTheContext[entry.Type] {
				continue
			}
			if piAccountedSourceOmission(entry.Type, entry.Message.Role) {
				omitted++
				continue
			}
			return 0, fmt.Errorf("%w: this read holds no record at all, while the source's own file %s "+
				"records no entry identities and holds a %s record (entry %d of %d, role %s) whose kind "+
				"this build cannot say the harness's own conversation context leaves out, so the "+
				"conversation a resumed target would read is not the one the source's loader builds",
				ErrUnrepresentableLeaf, path, entryKind(entry.Type), index+1, len(entries),
				entryRole(entry.Message.Role))
		}
		return omitted, nil
	}
	leaf := entries[len(entries)-1]
	current := len(entries) - 1
	omitted := 0
	counted := make(map[int]bool, 1)
	for steps := 0; steps <= len(entries); steps++ {
		entry := entries[current]
		switch {
		case piAccountedSourceOmission(entry.Type, entry.Message.Role):
			if !counted[current] {
				counted[current] = true
				omitted++
			}
		case !piEntryKindsOutsideTheContext[entry.Type]:
			return 0, fmt.Errorf("%w: this read holds no record at all, while the source's own file %s "+
				"selects the leaf %s, whose own path holds a %s record (id %s, role %s) whose kind this "+
				"build cannot say the harness's own conversation context leaves out: an empty session "+
				"would be installed in place of the conversation the source's own loader builds",
				ErrUnrepresentableLeaf, path, sourceLeafName(leaf), entryKind(entry.Type),
				entryIdentity(strings.TrimSpace(entry.ID)), entryRole(entry.Message.Role))
		}
		parent := parentOfPiEntry(entry)
		if parent == "" {
			break
		}
		next, ok := byID[parent]
		if !ok {
			break
		}
		current = next
	}
	return omitted, nil
}

// piSourceEntries reads one source session's own entries in the file's order and answers them, plus the
// index from an identity to its entry. A file that cannot be read is an error rather than an answer, the
// way the boundary pass treats the same file (takeSourceBoundary); a line the scan cannot read is skipped,
// which is what both the inherited reader and the load check's own entry read do with a malformed line.
func piSourceEntries(path string) ([]piConversationEntry, map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the source session's own entries: %w", err)
	}
	defer file.Close()
	entries := make([]piConversationEntry, 0, 64)
	if err := piScanConversationEntries(file, piSourceEntryLineMax, func(entry piConversationEntry) {
		entries = append(entries, entry)
	}); err != nil {
		return nil, nil, fmt.Errorf("reading the source session's own entries: %w", err)
	}
	byID := make(map[string]int, len(entries))
	for index, entry := range entries {
		if id := strings.TrimSpace(entry.ID); id != "" {
			if _, seen := byID[id]; !seen {
				byID[id] = index
			}
		}
	}
	return entries, byID, nil
}

// parentOfPiEntry answers the entry a record names as its parent, or "" when it names none.
func parentOfPiEntry(entry piConversationEntry) string {
	if entry.ParentID == nil {
		return ""
	}
	return strings.TrimSpace(*entry.ParentID)
}

// sourceLeafName names one entry for a refusal: its kind and its identity.
func sourceLeafName(entry piConversationEntry) string {
	return fmt.Sprintf("%s entry %s", entryKind(entry.Type), entryIdentity(strings.TrimSpace(entry.ID)))
}

func entryKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "unrecorded-kind"
	}
	return kind
}

func entryIdentity(id string) string {
	if id == "" {
		return "(no identity)"
	}
	return id
}

func entryRole(role string) string {
	if strings.TrimSpace(role) == "" {
		return "-"
	}
	return role
}

// piScanConversationEntries reads one Pi session file's entries in the file's own order, calling visit for
// each readable entry. The line-to-entry shape is the family's one reader (piEntryFromLine), the same one
// the load check's byte-backed read uses on a converted file: the two callers read the same format family,
// and the shape it holds is decided once.
//
// maxLine bounds one line. A line beyond it and a read that fails stop the scan and are answered as the
// error they are: a caller judging a file by the entries before such a line would be judging a file it did
// not read.
func piScanConversationEntries(r io.Reader, maxLine int, visit func(piConversationEntry)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	for scanner.Scan() {
		if entry, ok := piEntryFromLine(scanner.Bytes()); ok {
			visit(entry)
		}
	}
	return scanner.Err()
}
