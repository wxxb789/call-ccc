package ccc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/stringutil"
)

// codexSessionsSubdir is the subdirectory of a Codex data directory that holds its rollout sessions.
// The target's own discovery walks it as <home>/sessions/<YYYY>/<MM>/<DD>/rollout-*.jsonl.
const codexSessionsSubdir = "sessions"

// ErrTargetHomeMissing reports a target data directory that does not exist. call/ccc never invents one:
// a conversion that wrote sessions into a directory the target harness does not use would look
// installed while nothing could resume it.
var ErrTargetHomeMissing = errors.New("the target harness has no data directory")

// ErrTargetExists reports that the target path a conversion chose is already taken. A conversion always
// creates a new identity, and a taken path is a refusal, never an overwrite.
var ErrTargetExists = errors.New("a target session already exists at that path")

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
// final name is taken either with a hard link (atomic on a filesystem that has them, and exclusive, so
// a taken name fails instead of being overwritten) or, on a filesystem without hard links, by checking
// that the name is free and renaming. A reader therefore never sees a half-written session, and a
// conversion cannot replace a session another agent may already have continued.
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
		os.Remove(tempName)
	}
	if err := temp.Chmod(0o600); err != nil {
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
		os.Remove(tempName)
		return "", "", fmt.Errorf("closing %s: %w", tempName, err)
	}

	if err := os.Link(tempName, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			os.Remove(tempName)
			return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
		}
		// A filesystem without hard links (some Windows volumes, some network mounts) cannot take the
		// exclusive link. The name is still never taken twice: the check above and this one refuse a
		// path that holds anything, and only then does the rename move the finished file into place.
		if _, statErr := os.Lstat(target); statErr == nil {
			os.Remove(tempName)
			return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			os.Remove(tempName)
			return "", "", fmt.Errorf("checking %s: %w", target, statErr)
		}
		if renameErr := os.Rename(tempName, target); renameErr != nil {
			os.Remove(tempName)
			return "", "", fmt.Errorf("installing %s: %w", target, renameErr)
		}
		return target, "rename-after-existence-check", nil
	}
	if err := os.Remove(tempName); err != nil {
		// The session is installed; a temporary file left behind is not a failure of the conversion,
		// but it is not silent either.
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
		Method:     "the inherited AgentsView Codex reader over the installed session",
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
	differences := compareCodexRoundTrip(rollout, conversation)
	if len(differences) > 0 {
		verification.Detail = "the installed session does not hold the evidence in the target document: " +
			strings.Join(differences, "; ")
		return verification, nil
	}
	verification.State = "verified"
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

// compareCodexRoundTrip names every way the file's read-back differs from the target document.
//
// The comparison is by evidence, not by record shape: the target's reader pairs a call with its output
// onto one message, trims a result's text and drops an empty argument object, so those are the reader's
// own normalisations and not differences in what the file holds. What it cannot normalise is a missing
// or altered message, call, result or boundary, which is what this reports.
func compareCodexRoundTrip(rollout CodexRollout, readBack Conversation) []string {
	var differences []string
	intendedDialogue := []string{}
	intendedCalls := []string{}
	intendedResults := []string{}
	intendedBoundaries := []string{}
	for _, item := range rollout.Items {
		switch item.Kind {
		case ItemUser, ItemAssistant:
			intendedDialogue = append(intendedDialogue, string(item.Kind)+": "+item.Text)
		case ItemToolCall:
			intendedCalls = append(intendedCalls, roundTripCallKey(item.CallID, item.ToolName, item.InputJSON))
		case ItemToolResult:
			intendedResults = append(intendedResults, roundTripResultKey(item.CallID, item.Text))
		case ItemCompactionBoundary:
			intendedBoundaries = append(intendedBoundaries, item.Text)
		}
	}

	readDialogue := []string{}
	readCalls := []string{}
	readResults := []string{}
	readBoundaries := []string{}
	for _, message := range readBack.Messages {
		if message.IsCompactBoundary {
			readBoundaries = append(readBoundaries, message.Content)
			continue
		}
		if text := dialogueText(message); strings.TrimSpace(text) != "" {
			kind := ItemAssistant
			if parser.RoleType(message.Role) == parser.RoleUser {
				kind = ItemUser
			} else if parser.RoleType(message.Role) == parser.RoleTool {
				continue
			}
			readDialogue = append(readDialogue, string(kind)+": "+text)
		}
		for _, call := range message.ToolCalls {
			readCalls = append(readCalls, roundTripCallKey(call.ToolUseID, call.Name, call.InputJSON))
		}
		for _, result := range message.ToolResults {
			readResults = append(readResults, roundTripResultKey(result.ToolUseID, decodeResultText(result)))
		}
	}

	differences = append(differences, compareSequences("dialogue", compareInOrder, intendedDialogue, readDialogue)...)
	differences = append(differences, compareSequences("tool call", compareAsMultiset, intendedCalls, readCalls)...)
	differences = append(differences, compareSequences("tool result", compareAsMultiset, intendedResults, readResults)...)
	differences = append(differences, compareSequences("compaction boundary", compareInOrder, intendedBoundaries, readBoundaries)...)
	return differences
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

// sequenceComparison says how a comparison treats item order: dialogue and compaction boundaries are
// compared in the conversation's order, while calls and results are compared as multisets, because the
// target's reader attaches a call's output to the call's own message, so their position in the read-back
// is the reader's arrangement rather than the file's.
type sequenceComparison int

const (
	compareInOrder    sequenceComparison = iota // the two sides must hold the items in the same order
	compareAsMultiset                           // the two sides must hold the same items, order free
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
				category, index+1, summariseItem(intended[index])))
		case index >= len(intended):
			differences = append(differences, fmt.Sprintf("%s %d is in the file but was not written: %s",
				category, index+1, summariseItem(read[index])))
		case intended[index] != read[index]:
			differences = append(differences, fmt.Sprintf("%s %d differs (written %s, read back %s)",
				category, index+1, summariseItem(intended[index]), summariseItem(read[index])))
		}
	}
	return differences
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

// summariseItem bounds an item's text so a difference report cannot carry a whole tool output.
func summariseItem(item string) string {
	const limit = 120
	parts := strings.SplitN(item, "\x00", 3)
	joined := strings.Join(parts, " ")
	joined = strings.ReplaceAll(joined, "\n", " ")
	if len(joined) <= limit {
		return fmt.Sprintf("%q", joined)
	}
	return fmt.Sprintf("%q…", stringutil.SafeTruncate(joined, limit))
}
