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
	// NotReadBackAsDialogue counts the written dialogue messages the installed file holds verbatim and
	// the read-back shows no message for. The inherited reader reads no message for some text a file
	// holds: a user text block that opens with `# AGENTS.md` (or another injected-context envelope) is
	// read as that context rather than as a turn (internal/parser's preprocessCodexUserTextBlocks), and
	// a Claude transcript may hold a genuine prompt with that prefix. Such a message is in the file, so
	// the run counts it here and warns, instead of reporting a loss the conversion did not make; the
	// check establishes the file really holds it, so a message the file does not hold stays a failure.
	NotReadBackAsDialogue int `json:"not_read_back_as_dialogue,omitempty"`
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
	verification.NotReadBackAsDialogue = roundTrip.notReadBackAsDialogue
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
	// notReadBackAsDialogue counts the written dialogue messages the file holds verbatim and the
	// read-back shows no message for: the reader's own projection of the target format, not a loss.
	notReadBackAsDialogue int
}

// detail is the verification detail of a comparison the file satisfied: empty when the read-back showed
// every written item, and otherwise the one thing a caller has to be told before reading "verified" as
// "the read-back shows every message".
func (roundTrip codexRoundTrip) detail() string {
	if roundTrip.notReadBackAsDialogue == 0 {
		return ""
	}
	return fmt.Sprintf(
		"%d written message(s) are in the installed session and the inherited Codex reader reads no "+
			"message for their text: the file holds them verbatim, so the conversion carried them and "+
			"the reader's own projection of the file is what does not show them",
		roundTrip.notReadBackAsDialogue)
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
// One absence is neither: the reader reads no message for some text a file holds, because a user text
// block that opens with an injected-context envelope is read as that context rather than as a turn
// (internal/parser's preprocessCodexUserTextBlocks). A written dialogue message the read-back does not
// show is therefore not reported until the installed file has been asked, and `installed` is that file's
// own bytes: text the file holds is the reader's projection and is counted in notReadBackAsDialogue,
// while text the file does not hold is the missing message it looks like. The file is asked about
// dialogue only, because that is where the reader's projection applies: a call, a result and a boundary
// are compared through keys the reader derives from the record itself, and a difference there is a
// difference in what the file holds.
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

	// A written dialogue message the read-back shows no message for is the one item whose absence is not
	// yet a difference: where the file holds its text, its absence is the reader's own projection (a user
	// text block that opens with an injected-context envelope is read as that context, not as a turn),
	// and it is counted rather than reported; where the file does not hold the text, the item stays in
	// the comparison and is named there as the difference it is. The items the read-back does show are
	// compared in order as before.
	shown := make(map[string]struct{}, len(readDialogue))
	for _, key := range readDialogue {
		shown[key] = struct{}{}
	}
	compared := make([]string, 0, len(intendedDialogue))
	for _, key := range intendedDialogue {
		if _, ok := shown[key]; !ok {
			_, text, _ := strings.Cut(key, "\x00")
			if fileHoldsText(installed, text) {
				roundTrip.notReadBackAsDialogue++
				continue
			}
		}
		compared = append(compared, key)
	}

	roundTrip.differences = append(roundTrip.differences,
		compareSequences(categoryDialogue, compareInOrder, compared, readDialogue)...)
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

// fileHoldsText reports whether the installed file's own bytes hold text verbatim, as the JSON string
// one of its records carries it in.
//
// This is the question a read-back through the inherited reader cannot answer for a message that
// reader's projection reads no message for, and it is asked of the bytes read from the installed file --
// not of the document the writer kept in memory -- so the answer is about what is on disk. The text is
// spelled with the writer's own encoder settings (html escaping off), so the literal searched for is the
// literal that was written; and because an unescaped quote in a well-formed JSON line can only open a
// string, a match is a string token whose value is exactly this text. An empty text is answered as not
// held: the writer never writes one, so a document claiming an empty message is a difference whatever
// the file holds.
func fileHoldsText(installed []byte, text string) bool {
	if text == "" {
		return false
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		return false
	}
	return bytes.Contains(installed, bytes.TrimSuffix(encoded.Bytes(), []byte("\n")))
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
