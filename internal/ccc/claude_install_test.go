package ccc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if verification.Detail != "" && !strings.Contains(verification.Detail, "projection") {
		t.Errorf("a verified check with a detail should say what the reader projected away: %q", verification.Detail)
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
