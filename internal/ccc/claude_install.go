package ccc

import (
	"context"
	"encoding/json"
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
//
// A value is read to answer two things, and only the first of them trims: trimming decides whether a
// directory was named at all, and the value is then resolved exactly as it was written. A directory
// name may begin or end with a space, and the flag and the environment have to name the same directory
// they were given -- the explicit branch has never rewritten the caller's own string, and the
// environment is a caller's string too.
func ResolveClaudeHome(explicit string) (string, bool, error) {
	return resolveRouteHome(explicit, claudeHomeEnv, ".claude")
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
	return installFile(home, claudeProjectsSubdir, transcript.Directory, transcript.FileName, transcript.Data, claudeSessionFileMode)
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
	return verifyStructure(ctx, projectsDir, parser.AgentClaude, structureMethodClaude,
		"Claude", structureReaderClaude,
		filepath.Join(projectsDir, filepath.FromSlash(transcript.Directory), transcript.FileName),
		func(readBack Conversation, installed []byte) roundTripComparison {
			return compareClaudeRoundTrip(transcript, readBack, installed)
		}, cfg)
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
