package ccc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestResolveClaudeHome pins where a conversion installs: the caller's own directory, else
// $CLAUDE_CONFIG_DIR, else ~/.claude, with the boolean saying whether the caller named it (which is what
// decides whether a missing directory may be created).
func TestResolveClaudeHome(t *testing.T) {
	explicit := t.TempDir()
	home, named, err := ResolveClaudeHome(explicit)
	if err != nil || !named {
		t.Fatalf("ResolveClaudeHome(%q) = %q, %v, %v; want it used and reported as the caller's", explicit, home, named, err)
	}
	if want, _ := filepath.Abs(explicit); home != want {
		t.Errorf("explicit home = %q, want %q", home, want)
	}

	fromEnv := t.TempDir()
	t.Setenv(claudeHomeEnv, fromEnv)
	home, named, err = ResolveClaudeHome("")
	if err != nil || named {
		t.Fatalf("ResolveClaudeHome(\"\") with %s set = %q, %v, %v; want the environment's answer", claudeHomeEnv, home, named, err)
	}
	if want, _ := filepath.Abs(fromEnv); home != want {
		t.Errorf("environment home = %q, want %q", home, want)
	}

	// The environment's value is resolved as it was written, the way the explicit branch resolves the
	// caller's string: a directory name may begin or end with a space, and trimming it here would install
	// into a differently-named directory than the one the environment selected. Nothing has to exist for
	// this: the answer is a spelling, and whether a missing directory may be created is the boolean's
	// answer, not this function's business.
	spaced := filepath.Join(t.TempDir(), " claude home ")
	t.Setenv(claudeHomeEnv, spaced)
	home, named, err = ResolveClaudeHome("")
	if err != nil || named {
		t.Fatalf("ResolveClaudeHome(\"\") with a whitespace-bearing %s = %q, %v, %v; want it used", claudeHomeEnv, home, named, err)
	}
	if want, _ := filepath.Abs(spaced); home != want {
		t.Errorf("environment home = %q, want the value as written, %q", home, want)
	}

	// A value that is only whitespace named no directory at all, so it falls through to the default: the
	// presence test's own trimming is the half that trimming is for.
	t.Setenv(claudeHomeEnv, "   ")
	t.Setenv("HOME", t.TempDir())
	home, named, err = ResolveClaudeHome("")
	if err != nil || named || filepath.Base(home) != ".claude" {
		t.Fatalf("ResolveClaudeHome(\"\") with a whitespace-only %s = %q, %v, %v; want the default", claudeHomeEnv, home, named, err)
	}

	t.Setenv(claudeHomeEnv, "")
	t.Setenv("HOME", t.TempDir())
	home, named, err = ResolveClaudeHome("")
	if err != nil || named {
		t.Fatalf("ResolveClaudeHome(\"\") with nothing set = %q, %v, %v", home, named, err)
	}
	if filepath.Base(home) != ".claude" {
		t.Errorf("default home = %q, want a .claude directory", home)
	}
}

// TestInstallClaudeTranscriptIsExclusiveAndPrivate pins the two rules an installation has to hold: nothing
// at the target name is ever replaced, and the file it publishes is its owner's alone.
func TestInstallClaudeTranscriptIsExclusiveAndPrivate(t *testing.T) {
	home := t.TempDir()
	transcript := ClaudeTranscript{
		SessionID: "11111111-1111-4111-8111-111111111111",
		FileName:  "11111111-1111-4111-8111-111111111111.jsonl",
		Directory: "-home-user-project",
		Data:      []byte("{\"type\":\"user\"}\n"),
	}
	path, mechanism, err := InstallClaudeTranscript(transcript, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	want := filepath.Join(home, claudeProjectsSubdir, "-home-user-project", transcript.FileName)
	if path != want {
		t.Errorf("installed at %q, want %q", path, want)
	}
	if mechanism != "hard-link" {
		t.Errorf("mechanism = %q, want the exclusive hard link", mechanism)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the installed transcript is not there: %v", err)
	}
	if runtime.GOOS == "windows" {
		if claudeSessionFileMode.Perm() != 0o600 {
			t.Errorf("the installer asks for mode %v, want 0600 (Windows reports no POSIX mode bits, "+
				"so the request is the half of the rule that is checkable here)", claudeSessionFileMode)
		}
	} else if perm := info.Mode().Perm(); perm != claudeSessionFileMode.Perm() || perm&0o077 != 0 {
		t.Errorf("the installed transcript's permissions are %v, want %o", perm, claudeSessionFileMode.Perm())
	}

	// A second installation at the same name is refused, and the file's bytes are the first installation's.
	if _, _, err := InstallClaudeTranscript(transcript, home); err == nil {
		t.Fatal("a taken name was installed over")
	} else if !strings.Contains(err.Error(), ErrTargetExists.Error()) {
		t.Errorf("error = %v, want it to report the taken name", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(transcript.Data) {
		t.Errorf("the installed bytes changed: %q (%v)", data, err)
	}
	// No temporary file is left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the target directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the target directory holds %d entries, want the one transcript: %v", len(entries), entries)
	}
}

// TestInstallClaudeTranscriptRefusesAFileSystemWithoutHardLinks pins the refusal: this installer's only
// mechanism that takes a name without replacing what is there is a hard link, so a filesystem without
// them must be answered with an error rather than published by a rename.
func TestInstallClaudeTranscriptRefusesAFileSystemWithoutHardLinks(t *testing.T) {
	original := linkSession
	linkSession = func(string, string) error { return os.ErrInvalid }
	defer func() { linkSession = original }()

	transcript := ClaudeTranscript{
		SessionID: "22222222-2222-4222-8222-222222222222",
		FileName:  "22222222-2222-4222-8222-222222222222.jsonl",
		Directory: "-home-user-project",
		Data:      []byte("{}\n"),
	}
	home := t.TempDir()
	if _, _, err := InstallClaudeTranscript(transcript, home); err == nil {
		t.Fatal("the installation was accepted on a filesystem without hard links")
	} else if !strings.Contains(err.Error(), ErrNoExclusiveInstall.Error()) {
		t.Errorf("error = %v, want it to name the missing exclusive mechanism", err)
	}
	if _, err := os.Stat(filepath.Join(home, claudeProjectsSubdir, "-home-user-project", transcript.FileName)); !os.IsNotExist(err) {
		t.Errorf("a refused installation published something: %v", err)
	}
}

// TestVerifyClaudeTranscriptStructureRoundTripsAConversion pins the structural half of the evidence: the
// installed transcript reads back through the inherited Claude reader as the evidence the writer placed in
// it, which is the only independent statement that the file holds what the conversion says it carried.
func TestVerifyClaudeTranscriptStructureRoundTripsAConversion(t *testing.T) {
	conversation := readFixtureConversation(t, codexFixtureRef)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	home := t.TempDir()
	path, _, err := InstallClaudeTranscript(transcript, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	verification, err := VerifyClaudeTranscriptStructure(context.Background(),
		filepath.Join(home, claudeProjectsSubdir), transcript, fixtureConfig(t))
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want a verified round trip", verification)
	}
	if verification.TargetPath != path {
		t.Errorf("the check read %q, want the installed path %q", verification.TargetPath, path)
	}
	carried := transcript.Carried
	if verification.ReadBack.ToolCalls != carried.ToolCalls ||
		verification.ReadBack.ToolResults != carried.ToolResults ||
		verification.ReadBack.CompactionBoundaries != carried.CompactionBoundaries {
		t.Errorf("read-back = %+v, want the carried tool evidence and boundary", verification.ReadBack)
	}
	if !strings.Contains(verification.ReadBack.SessionID, transcript.SessionID) {
		t.Errorf("read-back session id = %q, want the installed session's", verification.ReadBack.SessionID)
	}
	// Nothing in this conversion is projected away: every written message is one the read-back shows with
	// its own text, so a check that reports a count or a detail here would be reporting something that did
	// not happen -- and an empty detail is the state the disclosure rule leaves for that.
	if verification.NotReadBackVerbatim != 0 || verification.Detail != "" {
		t.Errorf("the round trip projects nothing away, so the check has nothing to disclose, but it "+
			"reports not_read_back_verbatim = %d and detail %q",
			verification.NotReadBackVerbatim, verification.Detail)
	}
}

// TestVerifyClaudeTranscriptStructureNamesTheClaudeReaderWhenItProjectsAway pins the disclosure a
// verified round trip carries when the reader projects a written prompt away: the file holds the prompt's
// own record, the reader reads no message for it (a record it classifies as an injected command
// envelope), and the detail names the reader that read this file -- the Claude reader -- never the Codex
// route's, whose reader never saw it.
func TestVerifyClaudeTranscriptStructureNamesTheClaudeReaderWhenItProjectsAway(t *testing.T) {
	command := "<command-name>/usage</command-name>"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: command, Dialogue: command, DialogueProjected: true,
			Timestamp: time.Unix(1000, 0)},
		Message{Ordinal: 1, Role: "assistant", Content: "The usage report.", Dialogue: "The usage report.",
			DialogueProjected: true, Timestamp: time.Unix(1001, 0)},
		Message{Ordinal: 2, Role: "user", Content: "Where is the retry cap configured?",
			Dialogue: "Where is the retry cap configured?", DialogueProjected: true,
			Timestamp: time.Unix(1002, 0)},
	)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the document: %v", err)
	}
	home := t.TempDir()
	if _, _, err := InstallClaudeTranscript(transcript, home); err != nil {
		t.Fatalf("installing: %v", err)
	}
	verification, err := VerifyClaudeTranscriptStructure(context.Background(),
		filepath.Join(home, claudeProjectsSubdir), transcript, fixtureConfig(t))
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the file to hold every written message", verification)
	}
	if verification.NotReadBackVerbatim != 1 {
		t.Fatalf("not_read_back_verbatim = %d, want the one written prompt the reader reads as injected "+
			"context (detail: %q)", verification.NotReadBackVerbatim, verification.Detail)
	}
	if !strings.Contains(verification.Detail, structureReaderClaude) {
		t.Errorf("detail = %q, want it to name the %s reader that read this file",
			verification.Detail, structureReaderClaude)
	}
	if strings.Contains(verification.Detail, structureReaderCodex) {
		t.Errorf("detail = %q, want it not to name the %s reader, which did not read this file",
			verification.Detail, structureReaderCodex)
	}
}

// TestVerifyClaudeTranscriptStructureReportsEvidenceTheFileDoesNotHold pins the other half: a document
// whose item list claims evidence the installed bytes do not hold is reported, naming the item by what
// identifies it, because "verified" must never follow from a successful parse of anything.
func TestVerifyClaudeTranscriptStructureReportsEvidenceTheFileDoesNotHold(t *testing.T) {
	conversation := readFixtureConversation(t, codexFixtureRef)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	// The bytes stay as written; the claim grows by one message the file does not hold.
	transcript.Items = append(transcript.Items,
		TargetItem{Kind: ItemAssistant, Text: "a message this file does not hold"})
	home := t.TempDir()
	if _, _, err := InstallClaudeTranscript(transcript, home); err != nil {
		t.Fatalf("installing: %v", err)
	}
	verification, err := VerifyClaudeTranscriptStructure(context.Background(),
		filepath.Join(home, claudeProjectsSubdir), transcript, fixtureConfig(t))
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Fatal("the check verified a document whose evidence the file does not hold")
	}
	if !strings.Contains(verification.Detail, "not in the file") {
		t.Errorf("detail = %q, want it to name the missing evidence", verification.Detail)
	}
}
