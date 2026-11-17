package ccc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// claudeProjectsSubdir is the subdirectory of a Claude Code data directory that holds its session
// transcripts: the target's own discovery walks <config>/projects/<project-dir>/<session-id>.jsonl.
const claudeProjectsSubdir = "projects"

// claudeSessionFileMode is the mode the installer asks the OS to give a transcript it writes: readable
// and writable by its owner, by nobody else. It is named for the same reason the Codex installer's is:
// the *request* is the checkable half of the rule on a host without POSIX mode bits.
const claudeSessionFileMode os.FileMode = 0o600

// ResolveClaudeHome answers the Claude Code data directory a conversion installs into: the caller's
// explicit directory, else $CLAUDE_CONFIG_DIR, else ~/.claude. The boolean reports whether the answer
// came from the caller, which is what decides whether a missing directory may be created.
func ResolveClaudeHome(explicit string) (string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		home, err := filepath.Abs(explicit)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s: %w", explicit, err)
		}
		return home, true, nil
	}
	if fromEnv := strings.TrimSpace(os.Getenv(claudeHomeEnv)); fromEnv != "" {
		home, err := filepath.Abs(fromEnv)
		if err != nil {
			return "", false, fmt.Errorf("resolving $%s: %w", claudeHomeEnv, err)
		}
		return home, false, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolving the home directory: %w", err)
	}
	return filepath.Join(dir, ".claude"), false, nil
}

// InstallClaudeTranscript writes a converted transcript into a Claude Code data directory and returns
// the path it occupies, plus the mechanism that put it there.
//
// The rules are the Codex installer's, because the reason for them is: nothing existing is ever
// replaced, so the bytes go to a sibling temporary file first and the final name is taken with a hard
// link (atomic and exclusive where the filesystem has them); a filesystem without hard links is refused
// (ErrNoExclusiveInstall) rather than published by a rename, which would replace whatever is at the
// destination. A reader therefore never sees a half-written session, and a conversion cannot replace a
// session another agent may already have continued. The returned path and the error are independent
// answers, as they are there: a non-empty path means the session is published.
func InstallClaudeTranscript(transcript ClaudeTranscript, home string) (string, string, error) {
	directory := filepath.Join(home, claudeProjectsSubdir, filepath.FromSlash(transcript.Directory))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", directory, err)
	}
	target := filepath.Join(directory, transcript.FileName)
	if _, err := os.Lstat(target); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("checking %s: %w", target, err)
	}

	temp, err := os.CreateTemp(directory, "."+transcript.FileName+".tmp-")
	if err != nil {
		return "", "", fmt.Errorf("creating a temporary session file in %s: %w", directory, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		removeFile(tempName)
	}
	if err := temp.Chmod(claudeSessionFileMode); err != nil {
		cleanup()
		return "", "", fmt.Errorf("restricting %s: %w", tempName, err)
	}
	if _, err := temp.Write(transcript.Data); err != nil {
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
		if os.IsExist(err) {
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

// VerifyClaudeTranscriptStructure reads the installed transcript back through the inherited AgentsView
// Claude reader and compares it, item by item, with what the writer intended to carry. projectsDir is
// the target's projects directory; cfg supplies the provider configuration (its Claude roots are
// replaced, never mutated: the map is copied).
//
// The read-back is the file this call names, read through the inherited reader's own path reference, so
// the check compares the file the conversion published (or the dry run's copy of the same bytes) rather
// than resolving an id through the target's session tree, which could answer with another copy of it.
func VerifyClaudeTranscriptStructure(
	ctx context.Context, projectsDir string, transcript ClaudeTranscript, cfg config.Config,
) (StructureVerification, error) {
	verification := StructureVerification{
		State:      VerificationStateNotVerified,
		Method:     structureMethodClaude,
		TargetPath: filepath.Join(projectsDir, filepath.FromSlash(transcript.Directory), transcript.FileName),
	}
	readerConfig := cfg
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		dirs[agent] = roots
	}
	dirs[parser.AgentClaude] = []string{projectsDir}
	readerConfig.AgentDirs = dirs
	readerConfig.DisabledAgents = nil

	conversation, err := NewSourceReader(readerConfig).Read(ctx,
		Ref{Provider: string(parser.AgentClaude), ID: verification.TargetPath})
	if err != nil {
		verification.Detail = "the inherited Claude reader could not read the installed session back: " + err.Error()
		return verification, nil
	}
	verification.ReadBack = readBackCounts(conversation)
	installed, err := os.ReadFile(verification.TargetPath)
	if err != nil {
		verification.Detail = "the installed session could not be read back for comparison: " + err.Error()
		return verification, nil
	}
	roundTrip := compareClaudeRoundTrip(transcript, conversation, installed)
	verification.NotReadBackVerbatim = roundTrip.notReadBackVerbatim
	if len(roundTrip.differences) > 0 {
		verification.Detail = "the installed session does not hold the evidence in the target document: " +
			strings.Join(roundTrip.differences, "; ")
		return verification, nil
	}
	verification.State = VerificationStateVerified
	verification.Detail = roundTrip.detail(structureReaderClaude)
	return verification, nil
}

// compareClaudeRoundTrip names every way the file's read-back differs from the target document, through
// the shared walk with the Claude reader's own rules.
//
// The Claude reader puts records that share a message id back into one message, trims a tool result's
// content through its own text channel, and classifies a boundary record as a system row, so those are
// the reader's own normalisations and not differences in what the file holds. It also projects some user
// records away -- a pure command envelope, a record it classifies as system-injected -- and that silence
// is not evidence about the file, which is why the dialogue walk asks the file's own records.
func compareClaudeRoundTrip(transcript ClaudeTranscript, readBack Conversation, installed []byte) roundTripComparison {
	return compareRoundTrip(transcript.Items, readBack, claudeRoundTripReader(),
		newDialogueRecords(claudeInstalledDialogue(installed)))
}

// claudeRoundTripReader is the inherited Claude reader's own rules for the read-back walk: a record it
// classifies as injected context carries no message, and a role that is neither the user's nor the
// assistant's has no Claude message to stand for.
func claudeRoundTripReader() roundTripReader {
	return roundTripReader{
		classifiesAway: func(message Message) bool { return message.IsSystem },
		kindOf: func(message Message) (TargetItemKind, bool) {
			switch parser.RoleType(message.Role) {
			case parser.RoleUser:
				return ItemUser, true
			case parser.RoleAssistant:
				return ItemAssistant, true
			default:
				return "", false
			}
		},
	}
}

// claudeInstalledRecord is the part of one installed line the dialogue walk reads: the record's own type,
// its boundary flag, and its message's content. It declares no field for a tool block's input or a
// result's output, so reading a large transcript does not copy evidence the walk does not ask about.
type claudeInstalledRecord struct {
	Type             string `json:"type"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Message          struct {
		// Content is a plain string, an array of blocks, or absent.
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// claudeInstalledDialogue reads the dialogue the installed file's own records hold, in the file's own
// order: for every record this writer writes a message's text on -- a user or assistant record that is
// not the compaction summary -- each text block's own non-empty text, with the role the record's type
// names.
func claudeInstalledDialogue(installed []byte) []string {
	var keys []string
	for _, line := range strings.Split(string(installed), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record claudeInstalledRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		var kind TargetItemKind
		switch record.Type {
		case claudeTypeUser:
			kind = ItemUser
		case claudeTypeAssistant:
			kind = ItemAssistant
		default:
			continue
		}
		if record.IsCompactSummary {
			// The summary record is the boundary the comparison checks as a boundary, not dialogue.
			continue
		}
		var text string
		if err := json.Unmarshal(record.Message.Content, &text); err == nil {
			if text != "" {
				keys = append(keys, dialogueKey(string(kind), text))
			}
			continue
		}
		var blocks []claudeContentBlock
		if err := json.Unmarshal(record.Message.Content, &blocks); err != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == claudeBlockText && block.Text != "" {
				keys = append(keys, dialogueKey(string(kind), block.Text))
			}
		}
	}
	return keys
}
