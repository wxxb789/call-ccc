package ccc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// piSessionsSubdir is the subdirectory of a Pi data directory that holds its sessions; Pi's own
// discovery walks `<agent dir>/sessions/<project dir>/<file>.jsonl`.
const piSessionsSubdir = "sessions"

// piSessionFileMode is the mode the installer asks the OS to give a session file it writes: readable
// and writable by its owner, by nobody else. It is named for the same reason the other installers'
// modes are: the *request* is the checkable half of the rule on a host without POSIX mode bits.
const piSessionFileMode os.FileMode = 0o600

// ResolvePiHome answers the Pi data directory a conversion installs into: the caller's explicit
// directory, else $PI_CODING_AGENT_DIR, else ~/.pi/agent. The boolean reports whether the answer came
// from the caller, which is what decides whether a missing directory may be created.
//
// A value is read to answer two things, and only the first of them trims: trimming decides whether a
// directory was named at all, and the value is then resolved exactly as it was written, because a
// directory name may begin or end with a space.
func ResolvePiHome(explicit string) (string, bool, error) {
	return resolveRouteHome(explicit, piHomeEnv, ".pi", "agent")
}

// InstallPiSession writes a converted session into a Pi data directory and returns the path it
// occupies, plus the mechanism that put it there.
//
// The rules are the other installers', because the reason for them is: nothing existing is ever
// replaced, so the bytes go to a sibling temporary file first and the final name is taken with a hard
// link (atomic and exclusive where the filesystem has them); a filesystem without hard links is
// refused (ErrNoExclusiveInstall) rather than published by a rename, which would replace whatever is
// at the destination. The returned path and the error are independent answers: a non-empty path means
// the session is published.
func InstallPiSession(session PiSession, home string) (string, string, error) {
	return installFile(home, piSessionsSubdir, session.Directory, session.FileName, session.Data, piSessionFileMode)
}

// VerifyPiSessionStructure reads the installed session back through the inherited AgentsView Pi reader
// and compares it, item by item, with what the writer intended to carry. sessionsDir is the target's
// sessions directory; cfg supplies the provider configuration (its Pi roots are replaced, never
// mutated: the map is copied).
//
// The read-back is the file this call names, read through the inherited reader's own path reference, so
// the check compares the file the conversion published (or the dry run's copy of the same bytes) rather
// than resolving an id through the target's session tree, which could answer with another copy of it.
func VerifyPiSessionStructure(
	ctx context.Context, sessionsDir string, session PiSession, cfg config.Config,
) (StructureVerification, error) {
	return verifyStructure(ctx, sessionsDir, parser.AgentPi, structureMethodPi,
		structureReaderPi, structureReaderPi,
		filepath.Join(sessionsDir, session.Directory, session.FileName),
		os.ReadFile,
		func(readBack Conversation, installed []byte) roundTripComparison {
			return comparePiRoundTrip(session, readBack, installed)
		}, cfg)
}

// comparePiRoundTrip names every way the file's read-back differs from the target document, through the
// shared walk with the Pi reader's own rules.
//
// The Pi reader projects a compaction record as an assistant-role message flagged as a boundary, reads a
// toolResult record's content as the result's raw content, and recognises no injected-context envelope
// (there is none in this format), so those are the reader's own normalisations and not differences in
// what the file holds. The session is a tree whose entries the reader answers parent relations for, so a
// stated owner is resolved through the result's own ancestry rather than by id order (round 9's finding
// 4); Oh My Pi is the same format family and uses this comparison.
func comparePiRoundTrip(session PiSession, readBack Conversation, installed []byte) roundTripComparison {
	return compareRoundTrip(session.Items, readBack, piRoundTripReader(), piInstalledDialogue(installed), installed)
}

// piRoundTripReader is the Pi reader's rules for the read-back walk: the shared plain reader, plus the
// ancestry rule the family's tree format answers for its results' owners.
func piRoundTripReader() roundTripReader {
	reader := plainRoundTripReader()
	reader.ownersByAncestry = true
	return reader
}

// piInstalledMessage is one installed message payload as the dialogue walk reads it: its role and its
// content, which is a plain string for a user record and an array of blocks for an assistant or
// toolResult record. It declares no member for a block's own fields beyond the text a message's dialogue
// was written as, so reading a large session does not copy the evidence the walk does not ask about.
type piInstalledMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// dialogueMessage answers the dialogue record one installed line is, for the walk.
func (r piInstalledMessage) dialogueMessage() (TargetItemKind, string, bool) {
	switch r.Role {
	case piRoleUser:
		var text string
		if err := json.Unmarshal(r.Content, &text); err != nil {
			return "", "", false
		}
		if strings.TrimSpace(text) == "" {
			return "", "", false
		}
		return ItemUser, text, true
	case piRoleAssistant:
		var blocks []piContentBlock
		if err := json.Unmarshal(r.Content, &blocks); err != nil {
			return "", "", false
		}
		var text strings.Builder
		for _, block := range blocks {
			if block.Type != piBlockText || block.Text == "" {
				continue
			}
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(block.Text)
		}
		if strings.TrimSpace(text.String()) == "" {
			return "", "", false
		}
		return ItemAssistant, text.String(), true
	}
	return "", "", false
}

// piInstalledDialogue reads the dialogue the installed file's own records hold, in the file's own order:
// for every entry that is a message written the way this writer writes one -- a user record whose
// content is the prompt's text, an assistant record whose content holds the text block the message's
// own dialogue was written as -- the identity the comparison uses.
//
// The question is this narrow on purpose: the same text can appear inside a tool result, and asking the
// file whether it holds a text *somewhere* would answer for records that are not the message. A line
// that is not one readable record is skipped: the failure this walk answers for is an absent record,
// not a malformed one.
func piInstalledDialogue(installed []byte) dialogueRecords {
	var keys []string
	for _, line := range bytes.Split(installed, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry struct {
			Type    string             `json:"type"`
			Message piInstalledMessage `json:"message"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != piEntryMessage {
			continue
		}
		kind, text, ok := entry.Message.dialogueMessage()
		if !ok {
			continue
		}
		keys = append(keys, dialogueKey(string(kind), text))
	}
	return newDialogueRecords(keys)
}
