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
	directory := filepath.Join(home, codexSessionsSubdir, filepath.FromSlash(rollout.Directory))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", directory, err)
	}
	target := filepath.Join(directory, rollout.FileName)
	if _, err := os.Lstat(target); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("checking %s: %w", target, err)
	}

	temp, err := os.CreateTemp(directory, "."+rollout.FileName+".tmp-")
	if err != nil {
		return "", "", fmt.Errorf("creating a temporary session file in %s: %w", directory, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		removeFile(tempName)
	}
	if err := temp.Chmod(codexSessionFileMode); err != nil {
		cleanup()
		return "", "", fmt.Errorf("restricting %s: %w", tempName, err)
	}
	if _, err := temp.Write(rollout.Data); err != nil {
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
		// A filesystem without hard links cannot take the exclusive link, and this installer has no
		// other mechanism that takes a name without replacing what is there. The conversion is refused
		// rather than published by a rename, which would be able to replace a session another agent had
		// already continued.
		return "", "", fmt.Errorf("%w: %s: %v; install into a filesystem that supports hard links",
			ErrNoExclusiveInstall, directory, err)
	}
	if err := removeFile(tempName); err != nil {
		// The session is installed -- the path returned below is the one it occupies -- and a temporary
		// file left behind is not a failure of the conversion, but it is not silent either, so the
		// caller gets both answers.
		return target, "hard-link", fmt.Errorf("removing the temporary file %s: %w", tempName, err)
	}
	return target, "hard-link", nil
}

// StructureVerification reports whether the installed (or, for a dry run, a temporary copy) target
// session holds every piece of evidence the writer placed in it, read back through the inherited
// AgentsView Codex reader. The reader is upstream's implementation of the same format, so this compares
// the file with an independent read of it rather than with the writer's own idea of it.
type StructureVerification struct {
	State      string        `json:"state"`
	Method     string        `json:"method"`
	Detail     string        `json:"detail,omitempty"`
	TargetPath string        `json:"target_path,omitempty"`
	ReadBack   CodexReadBack `json:"read_back,omitempty"`
	// NotReadBackVerbatim counts the written dialogue messages the read-back does not show with their own
	// text and the installed file's own dialogue record holds. The inherited reader reads some text a file
	// holds differently: a user text block that opens with `# AGENTS.md` (or another injected-context
	// envelope) is read as that context rather than as a turn (internal/parser's
	// preprocessCodexUserTextBlocks), so the reader reads no message for the whole prompt, or reads only
	// the text the envelope preceded. A Claude transcript may hold a genuine prompt with that prefix, and
	// such a message is in the file, so the run counts it here and warns instead of reporting a loss the
	// conversion did not make; the file's record is what establishes that, so a message the file does not
	// hold stays a failure.
	NotReadBackVerbatim int `json:"not_read_back_verbatim,omitempty"`
}

// CodexReadBack is what the inherited reader made of the target file.
type CodexReadBack struct {
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
func VerifyCodexRolloutStructure(
	ctx context.Context, sessionsDir string, rollout CodexRollout, cfg config.Config,
) (StructureVerification, error) {
	verification := StructureVerification{
		State:      "not-verified",
		Method:     structureMethod,
		TargetPath: filepath.Join(sessionsDir, filepath.FromSlash(rollout.Directory), rollout.FileName),
	}
	readerConfig := cfg
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		dirs[agent] = roots
	}
	dirs[parser.AgentCodex] = []string{sessionsDir}
	readerConfig.AgentDirs = dirs
	readerConfig.DisabledAgents = nil

	conversation, err := NewSourceReader(readerConfig).Read(ctx, Ref{Provider: string(parser.AgentCodex), ID: rollout.SessionID})
	if err != nil {
		verification.Detail = "the inherited Codex reader could not read the installed session back: " + err.Error()
		return verification, nil
	}
	verification.ReadBack = codexReadBack(conversation)
	// The comparison asks the file's own bytes about any written dialogue the read-back does not show:
	// that is the one question the inherited reader's projection cannot answer for such a message, and
	// it is asked of the bytes on disk rather than of the document the writer kept in memory.
	installed, err := os.ReadFile(verification.TargetPath)
	if err != nil {
		verification.Detail = "the installed session could not be read back for comparison: " + err.Error()
		return verification, nil
	}
	roundTrip := compareCodexRoundTrip(rollout, conversation, installed)
	verification.NotReadBackVerbatim = roundTrip.notReadBackVerbatim
	if len(roundTrip.differences) > 0 {
		verification.Detail = "the installed session does not hold the evidence in the target document: " +
			strings.Join(roundTrip.differences, "; ")
		return verification, nil
	}
	verification.State = "verified"
	verification.Detail = roundTrip.detail()
	return verification, nil
}

func codexReadBack(conversation Conversation) CodexReadBack {
	counts := Summarize(conversation.Messages)
	return CodexReadBack{
		SessionID:            conversation.Source.SessionID,
		Messages:             counts.Messages,
		UserMessages:         counts.UserMessages,
		AssistantMessages:    counts.AssistantMessages,
		ToolCalls:            counts.ToolCalls,
		ToolResults:          counts.ToolResults,
		CompactionBoundaries: counts.CompactBoundaries,
	}
}

// codexRoundTrip is what comparing the target document with a read-back of the installed file found.
type codexRoundTrip struct {
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
// "verified" as "the read-back shows every message with the text the file holds".
func (roundTrip codexRoundTrip) detail() string {
	if roundTrip.notReadBackVerbatim == 0 {
		return ""
	}
	return fmt.Sprintf(
		"%d written message(s) are in the installed session and the inherited Codex reader does not read "+
			"their own text from it: the file's dialogue records hold them, so the conversion carried them "+
			"and the reader's own projection of the file is what the read-back shows instead",
		roundTrip.notReadBackVerbatim)
}

// compareCodexRoundTrip names every way the file's read-back differs from the target document.
//
// The comparison is by evidence, not by record shape: the target's reader pairs a call with its output
// onto one message, trims a result's output, trims a compacted record's summary and drops an empty
// argument object, so those are the reader's own normalisations and not differences in what the file
// holds. A message the reader reads as no message at all (its text is blank after trimming) is not one
// the writer may count as carried: the writer omits such text and names it, and a document that claims
// one is reported here as the missing message it is. What the comparison cannot normalise is a missing
// or altered message, call, result or boundary, which is what this reports.
//
// One reading is neither: the reader reads user text blocks through the injected-context classification,
// because a block that opens with an envelope is that context rather than a turn (internal/parser's
// preprocessCodexUserTextBlocks), so a written message can come back as no message at all or with the
// text the envelope preceded. Dialogue is therefore walked against the installed file's own dialogue
// records, in the file's own order, rather than through a set membership test: what tells a projection
// from a loss is the record -- and where the document writes it -- not the read-back's silence, which is
// what lets both projection shapes resolve to the same count (compareDialogue) while a file whose
// records hold the right texts in another order is reported. The file is asked about dialogue only,
// because that is where the reader's projection applies: a call, a result and a boundary are compared
// through keys the reader derives from the record itself, and a difference there is a difference in what
// the file holds.
func compareCodexRoundTrip(rollout CodexRollout, readBack Conversation, installed []byte) codexRoundTrip {
	var roundTrip codexRoundTrip
	intendedDialogue := []string{}
	intendedCalls := []string{}
	intendedResults := []string{}
	intendedBoundaries := []string{}
	for _, item := range rollout.Items {
		switch item.Kind {
		case ItemUser, ItemAssistant:
			intendedDialogue = append(intendedDialogue, dialogueKey(string(item.Kind), item.Text))
		case ItemToolCall:
			intendedCalls = append(intendedCalls, roundTripCallKey(item.CallID, item.ToolName, item.InputJSON))
		case ItemToolResult:
			intendedResults = append(intendedResults, roundTripResultKey(item.CallID, item.Text))
		case ItemCompactionBoundary:
			intendedBoundaries = append(intendedBoundaries, roundTripBoundaryKey(item.Text))
		}
	}

	readDialogue := []string{}
	readCalls := []string{}
	readResults := []string{}
	readBoundaries := []string{}
	for _, message := range readBack.Messages {
		if message.IsCompactBoundary {
			readBoundaries = append(readBoundaries, roundTripBoundaryKey(message.Content))
			continue
		}
		if text := dialogueText(message); strings.TrimSpace(text) != "" {
			kind := ItemAssistant
			if parser.RoleType(message.Role) == parser.RoleUser {
				kind = ItemUser
			} else if parser.RoleType(message.Role) == parser.RoleTool {
				continue
			}
			readDialogue = append(readDialogue, dialogueKey(string(kind), text))
		}
		for _, call := range message.ToolCalls {
			readCalls = append(readCalls, roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON))
		}
		for _, result := range message.ToolResults {
			text, _ := resultText(result)
			readResults = append(readResults, roundTripResultKey(result.ToolUseID, text))
		}
	}

	dialogue := compareDialogue(intendedDialogue, readDialogue, installedDialogueRecords(installed))
	roundTrip.differences = append(roundTrip.differences, dialogue.differences...)
	roundTrip.notReadBackVerbatim = dialogue.notReadBackVerbatim
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryToolCall, compareAsMultiset, intendedCalls, readCalls)...)
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryToolResult, compareAsMultiset, intendedResults, readResults)...)
	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryCompactionBoundary, compareInOrder, intendedBoundaries, readBoundaries)...)
	return roundTrip
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
	// the installed file's records hold in their own place.
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
// record the reader does not show) from a message the file does not hold.
//
// The file's records are read in their own order, and a message is resolved against the record that
// stands where the document writes it: a record the file holds anywhere else is a file whose dialogue
// stands in another order, which the read-back cannot report for the messages the reader projects away,
// and a record no written message resolves is dialogue the file holds that the document does not write.
// `records` is consumed as the walk resolves messages, so one record answers for one written message:
// the same prompt recorded twice in the source needs two records in the file.
func compareDialogue(intended, read []string, records dialogueRecords) dialogueWalk {
	var walk dialogueWalk
	// account resolves one written message against the file's own records, names the difference where
	// the file's answer is one, and answers whether the message's record was the file's next unconsumed
	// record -- the shape the reader's projection leaves behind, which is what notReadBackVerbatim counts.
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
	resolve := func(ordinal int) {
		if account(ordinal) {
			walk.notReadBackVerbatim++
		}
	}
	cursor := 0
	for index, readKey := range read {
		paired, projected := pairReadBackMessage(intended, cursor, readKey, &records)
		switch {
		case paired >= 0:
			for skipped := cursor; skipped < paired; skipped++ {
				resolve(skipped)
			}
			// The record is what the count is about: this is the message's own record being consumed, in
			// the place the document writes it, and a reading is only counted where it was.
			if account(paired) && projected {
				walk.notReadBackVerbatim++
			}
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
// has). The boolean reports whether the pairing is the reader's reading rather than the message's own
// text; a message neither relation answers for has -1.
func pairReadBackMessage(intended []string, from int, readKey string, records *dialogueRecords) (int, bool) {
	for index := from; index < len(intended); index++ {
		key := intended[index]
		if key == readKey {
			return index, false
		}
		if !records.holds(key) {
			continue
		}
		kind, text, _ := strings.Cut(key, "\x00")
		if readsAsDialogueMessage(kind, text, readKey) {
			return index, true
		}
	}
	return -1, false
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
