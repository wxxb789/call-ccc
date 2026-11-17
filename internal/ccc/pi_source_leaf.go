package ccc

import (
	"bufio"
	"bytes"
	"encoding/json"
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

// piSourceEntryLineMax bounds one line of a source session the entry scan reads. It is the bound the
// carried Pi parser reads a line with (internal/parser's maxLineSize): a longer line is a line the read
// could not have read either, so the scan stops and answers the read error rather than skipping it and
// judging the file by the entries before it.
const piSourceEntryLineMax = 64 << 20

// piSourceLeafCheck answers what one conversion could establish about the record the source's own loader
// selects as the conversation's leaf. Two answers travel: the warning the report carries when the source's
// own bytes were not what the check could read, and the refusal when they were and do not support the
// record the read's last message stands for.
//
// The check runs where the source's own file stands behind the read: the read came from the file itself
// (not from an archive row, which is a projection with no file open, built by the archive's own ingest)
// and the file's bytes are the size the read's own snapshot recorded -- a size match, which is what that
// record can show, exactly as the boundary's own use of the same number states (fillReadCoverage).
// Anywhere else the leaf is not established, and the report says exactly that instead of presenting the
// read's own last record as the source's leaf.
func piSourceLeafCheck(conversation Conversation, readFrom string, boundary SourceBoundary) (string, error) {
	if !piSourceLeafProvider(conversation.Source.Provider) {
		return "", nil
	}
	if readFrom != "source" || boundary.State != SourceBoundaryTaken || strings.TrimSpace(boundary.Path) == "" {
		return fmt.Sprintf(
			"the entry the source's own loader selects as the conversation's leaf was not established: %s, "+
				"so the conversion writes the line of the read's own last record and no source file answers "+
				"which entry the harness's own loader would walk from",
			sourceLeafNotMeasuredDetail(readFrom, boundary)), nil
	}
	if size := conversation.Source.FileSize; size > 0 && boundary.Bytes != size {
		return fmt.Sprintf(
			"the entry the source's own loader selects as the conversation's leaf was not established: the "+
				"source file holds %d byte(s) while the read's own snapshot of this file recorded %d, so the "+
				"file moved while it was being read and the entry its own bytes end with is not shown to be "+
				"the entry this read's last record stands for", boundary.Bytes, size), nil
	}
	return "", piSourceLeafRefusal(conversation, boundary.Path)
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

// piSourceLeafRefusal answers whether the source file's own answer supports the record this read's last
// message stands for: nil when it does, and a refusal naming the difference when it does not.
//
// The file's own leaf path is walked from the file's last entry back through `parentId` -- the harness's
// own walk -- and every entry on it is asked about:
//
//   - a record this read answered an identity for is a record the conversion can place, and the *first*
//     one the walk reaches (the deepest, since the walk runs leaf to root) must be the record this read's
//     last message stands for, because that is the record the target's loader would walk from;
//   - an entry this read did not answer an identity for is a record the conversion cannot place, and it
//     may stand on the path only when the harness's own loader keeps its kind out of the conversation's
//     context (piEntryKindsOutsideTheContext).
//
// A file that records no entry identities at all -- a Pi v1 session, which the inherited reader accepts and
// derives an identity for from the filename -- answers its own order rather than a tree: it cannot hold a
// branch, because a branch is a `parentId` link, so the entry the harness's loader selects is the file's
// last entry and the conversation is the file's own order. What the read's own last record stands for is
// therefore asked by order: the last entry whose kind is one the read carries is the record its last
// message stands for, and every entry after that one must be an entry the harness's loader keeps out of
// the context. A walk that ends where the file does (a root, a parent the file does not hold, a record
// naming itself, a loop) answers what it reached. A read with no records at all has no leaf to resume
// wrongly, and a file with no entry beyond its header selects nothing: both answer nil.
func piSourceLeafRefusal(conversation Conversation, path string) error {
	messages := conversation.Messages
	if len(messages) == 0 {
		return nil
	}
	carried := make(map[string]bool, len(messages))
	for _, message := range messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			carried[id] = true
		}
	}
	lastID := strings.TrimSpace(messages[len(messages)-1].SourceUUID)

	entries, byID, err := piSourceEntries(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		// Nothing but a session header (and possibly a title slot line): the file selects no entry at all,
		// so no branch of its own can be resumed in place of this read's last record.
		return nil
	}
	leaf := entries[len(entries)-1]
	if len(byID) == 0 {
		for index := len(entries) - 1; index >= 0; index-- {
			entry := entries[index]
			if piCarriedSourceEntry(entry.Type, entry.Message.Role) {
				return nil
			}
			if !piEntryKindsOutsideTheContext[entry.Type] {
				return fmt.Errorf("%w: the source's own file %s records no entry identities, and the %s "+
					"record it holds after its last message-like record (entry %d of %d, role %s) is one "+
					"this read does not carry and whose kind this build cannot say the harness's own "+
					"conversation context leaves out, so the conversation a resumed target would read is "+
					"not the one the source's loader builds",
					ErrUnrepresentableLeaf, path, entryKind(entry.Type), index+1, len(entries),
					entryRole(entry.Message.Role))
			}
		}
		return fmt.Errorf("%w: the source's own file %s records no entry identities and holds no record "+
			"kind this read carries, so the entry the source's own loader selects as the conversation's leaf "+
			"cannot be represented", ErrUnrepresentableLeaf, path)
	}

	// The walk: from the file's last entry back through parentId, the way the harness's own loader walks. A
	// parent the file does not hold, a record naming itself and a loop each end where the entry count runs
	// out, which is the only bound the walk needs and the only one it has: a cycle the source recorded
	// (a record naming itself, or a longer loop) is re-walked until the file's own entry count is spent and
	// then answered as what it reached. No visited set is kept beside the bound: each entry's answer is
	// idempotent, so removing one would leave every case green while stating only the loop's iteration
	// count, which is what this walk's own bound already states (the writers' walk, writtenAncestor, makes
	// the same choice for the same reason).
	current := leaf
	deepest := ""
	for steps := 0; steps <= len(entries); steps++ {
		currentID := strings.TrimSpace(current.ID)
		if carried[currentID] {
			if deepest == "" {
				deepest = currentID
			}
		} else if !piEntryKindsOutsideTheContext[current.Type] {
			// The entry is not one this read answered an identity for -- so the conversion cannot place it
			// -- and nothing here knows the harness's own loader keeps its kind out of the context either.
			return fmt.Errorf("%w: the source's own file %s selects the leaf %s, and that leaf's own path "+
				"holds a %s record (id %s, role %s) which this read does not carry and whose kind this "+
				"build cannot say the harness's own conversation context leaves out, so the conversation a "+
				"resumed target would read is not the one the source's loader builds",
				ErrUnrepresentableLeaf, path, sourceLeafName(leaf), entryKind(current.Type),
				entryIdentity(currentID), entryRole(current.Message.Role))
		}
		parent := parentOfPiEntry(current)
		if parent == "" {
			break
		}
		next, ok := byID[parent]
		if !ok {
			break
		}
		current = entries[next]
	}
	switch {
	case deepest == "":
		return fmt.Errorf("%w: the source's own file %s selects the leaf %s, and that leaf's own path "+
			"reaches no record this read holds, while the read's last record is %s: a resumed session "+
			"would continue another branch than the one the source's conversation ends on",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), entryIdentity(lastID))
	case lastID == "":
		return fmt.Errorf("%w: the source's own file %s selects the leaf %s, whose own path reaches the "+
			"record %s, while this read answered no identity for its last record, so which entry of the "+
			"source's own file that record stands for is not established",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), deepest)
	case deepest != lastID:
		return fmt.Errorf("%w: the source's own file %s selects the leaf %s, whose own path reaches the "+
			"record %s, while this read's last record is %s: a resumed session would continue a branch "+
			"the source's conversation left",
			ErrUnrepresentableLeaf, path, sourceLeafName(leaf), deepest, lastID)
	}
	return nil
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
// each readable entry. The session header and lines that are not JSON objects are skipped, exactly as the
// load check's own entry read (piConversationEntries) does with the bytes of a converted file: the two
// callers read the same format family, and its entry shape has one reader here.
//
// maxLine bounds one line. A line beyond it and a read that fails stop the scan and are answered as the
// error they are: a caller judging a file by the entries before such a line would be judging a file it did
// not read.
func piScanConversationEntries(r io.Reader, maxLine int, visit func(piConversationEntry)) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry piConversationEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "" || entry.Type == piEntrySession {
			continue
		}
		visit(entry)
	}
	return scanner.Err()
}
