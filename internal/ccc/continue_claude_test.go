package ccc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// claudeTargetHome prepares an isolated Claude Code data directory holding one unrelated session, and
// answers the home and that session's path: a conversion must leave it alone.
func claudeTargetHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	unrelated := filepath.Join(home, claudeProjectsSubdir, "-home-user-other-project",
		"00000000-0000-4000-8000-0000000000ff.jsonl")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatalf("creating the unrelated session's directory: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("{\"type\":\"user\",\"uuid\":\"x\",\"sessionId\":\"unrelated\"}\n"), 0o600); err != nil {
		t.Fatalf("writing the unrelated session: %v", err)
	}
	return home, unrelated
}

// TestContinueIntoClaudeConvertsInstallsAndReadsBack is the route's central property test: one call turns a
// real Codex session into a new Claude Code session, installs it under the projects directory the target's
// discovery walks, leaves the source and an unrelated session untouched, and reads the installed file back
// through the inherited Claude reader as what the writer placed in it.
func TestContinueIntoClaudeConvertsInstallsAndReadsBack(t *testing.T) {
	sourcePath := fixturePath(t, filepath.Join("codex", "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)
	home, unrelated := claudeTargetHome(t)
	unrelatedBefore := hashFile(t, unrelated)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          codexFixtureRef,
		Target:       TargetClaude,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}

	if result.Target != TargetClaude {
		t.Errorf("target = %q, want %q", result.Target, TargetClaude)
	}
	if result.Conversion.State != ConversionStateConverted || result.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", result.Conversion)
	}
	if result.Conversion.Format != ClaudeTargetFormat {
		t.Errorf("format = %q, want %q", result.Conversion.Format, ClaudeTargetFormat)
	}
	carried := result.Conversion.Carried
	if carried.Messages == 0 || carried.ToolCalls != 1 || carried.ToolResults != 1 || carried.CompactionBoundaries != 1 {
		t.Errorf("carried = %+v, want the fixture's messages, tool pair and boundary", carried)
	}
	// The three disclosures this route owes a reader: the source's per-message model is not written, the
	// turns before the boundary are recorded but not continued from, and the tool names are the source's
	// own vocabulary.
	for _, testCase := range []struct {
		category string
		want     int
	}{
		{"message_model_attribution", 5},
		{"pre_compaction_history", 2},
		{"tool_name_vocabulary", 1},
	} {
		if got := lossCounts(result.Conversion.Losses)[testCase.category]; got != testCase.want {
			t.Errorf("%s = %d, want %d", testCase.category, got, testCase.want)
		}
	}
	if len(result.TargetSession.FormatLimits) == 0 {
		t.Error("the report states no format limits, so a consumer cannot tell what the pair cannot express")
	}

	if result.Installation.State != InstallationStateInstalled || !result.Installation.Created {
		t.Errorf("installation = %+v, want an installed session", result.Installation)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("installation = %+v, want the source reported unchanged", result.Installation)
	}
	installed := result.Installation.Path
	if installed != result.TargetSession.Path {
		t.Errorf("installed path %q and target session path %q disagree", installed, result.TargetSession.Path)
	}
	want := filepath.Join(home, claudeProjectsSubdir, ClaudeProjectDir("/home/user/code/example-api"),
		result.TargetSession.SessionID+".jsonl")
	if installed != want {
		t.Errorf("installed at %q, want %q", installed, want)
	}
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("the installed transcript is not there: %v", err)
	}

	if result.TargetSession.Provider != TargetClaude {
		t.Errorf("target provider = %q, want %q", result.TargetSession.Provider, TargetClaude)
	}
	if result.TargetSession.SessionID == codexFixtureRef.ID {
		t.Error("the target identity is the source session's id")
	}
	if result.TargetSession.Cwd != "/home/user/code/example-api" {
		t.Errorf("target working directory = %q, want the source session's", result.TargetSession.Cwd)
	}
	if result.TargetSession.CLIVersion != ClaudeFormatVersion || result.TargetSession.CLIVersionSource != "writer-pin" {
		t.Errorf("cli version = %q (%s), want the writer's pin when no harness answered",
			result.TargetSession.CLIVersion, result.TargetSession.CLIVersionSource)
	}
	if result.TargetSession.BinaryFound {
		t.Errorf("the report claims to have found %q on PATH", missingTargetBinary)
	}
	// Measured: the loader finds a session through the working directory it is run from, so the printed
	// command carries both the directory and the data directory. A command missing either resumes the
	// caller's own home, or nothing at all.
	if !strings.HasPrefix(result.TargetSession.ResumeCommand, "cd "+result.TargetSession.Cwd+" && ") {
		t.Errorf("resume command %q does not start by the session's working directory", result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, claudeHomeEnv+"="+home) {
		t.Errorf("resume command %q does not select the data directory it installed into", result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ExecResumeCommand, "-p --output-format json --resume "+result.TargetSession.SessionID) {
		t.Errorf("exec resume command %q does not name the non-interactive resume", result.TargetSession.ExecResumeCommand)
	}
	if got := result.TargetSession.ResumeEnvironment[claudeHomeEnv]; got != home {
		t.Errorf("resume environment = %q, want the data directory %q", got, home)
	}

	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed session to read back as what was written",
			result.Verification.Structure)
	}
	if readBack := result.Verification.Structure.ReadBack; readBack.ToolCalls != 1 || readBack.ToolResults != 1 ||
		readBack.CompactionBoundaries != 1 {
		t.Errorf("read-back = %+v, want the carried tool evidence and boundary", result.Verification.Structure.ReadBack)
	}
	if result.Verification.Load.State != VerificationStateNotVerified {
		t.Errorf("load = %+v, want not-verified: no harness is installed here", result.Verification.Load)
	}
	if !strings.Contains(result.Verification.Load.Detail, missingTargetBinary) {
		t.Errorf("load detail = %q, want it to name the missing harness", result.Verification.Load.Detail)
	}
	if result.Verification.Resume.State != VerificationStateNotRequested {
		t.Errorf("resume = %+v, want not-requested at the load level", result.Verification.Resume)
	}

	if digest := hashFile(t, sourcePath); digest != sourceBefore {
		t.Error("the conversion changed the source session")
	}
	if digest := hashFile(t, unrelated); digest != unrelatedBefore {
		t.Error("the conversion changed an unrelated session in the target home")
	}
}

// TestContinueIntoClaudeDryRunWritesNothing pins a dry run's promise: the conversion is verified against a
// throw-away copy, the target home stays exactly as it was, and the run still reports the identity it would
// have written.
func TestContinueIntoClaudeDryRunWritesNothing(t *testing.T) {
	home, _ := claudeTargetHome(t)
	before := hashTree(t, home)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          codexFixtureRef,
		Target:       TargetClaude,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if !result.Conversion.DryRun || result.Installation.State == InstallationStateInstalled {
		t.Errorf("installation = %+v, want a dry run that installed nothing", result.Installation)
	}
	if result.TargetSession.SessionID == "" {
		t.Error("the dry run reports no identity")
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the copy read back as what was written", result.Verification.Structure)
	}
	if result.Verification.Structure.TargetPath != "" {
		t.Errorf("the dry run names an installed path (%q) it never wrote", result.Verification.Structure.TargetPath)
	}
	if result.Verification.Load.State != VerificationStateNotVerified ||
		!strings.Contains(result.Verification.Load.Detail, "dry run") {
		t.Errorf("load = %+v, want a dry run answered as installing nothing", result.Verification.Load)
	}
	if after := hashTree(t, home); !sameTree(before, after) {
		t.Errorf("the dry run changed the target home: %v -> %v", before, after)
	}
}

// sameTree answers whether two directory listings hold the same files with the same digests.
func sameTree(before, after map[string]string) bool {
	if len(before) != len(after) {
		return false
	}
	for path, digest := range before {
		if after[path] != digest {
			return false
		}
	}
	return true
}

// TestContinueIntoClaudeHomeResolution pins where a conversion is allowed to write: an explicit directory
// is created and used, and a missing directory nobody named is refused rather than invented.
func TestContinueIntoClaudeHomeResolution(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv(claudeHomeEnv, missing)
	if _, err := Continue(context.Background(), ContinueOptions{
		Ref: codexFixtureRef, Target: TargetClaude, Config: fixtureConfig(t),
		TargetBinary: missingTargetBinary, Verify: VerifyNone,
	}); !errors.Is(err, ErrTargetHomeMissing) {
		t.Fatalf("error = %v, want ErrTargetHomeMissing for a data directory nobody named", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("the refused run created %s", missing)
	}

	explicit := filepath.Join(t.TempDir(), "fresh")
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: codexFixtureRef, Target: TargetClaude, Config: fixtureConfig(t),
		TargetHome: explicit, TargetBinary: missingTargetBinary, Verify: VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing into an explicit directory: %v", err)
	}
	if !strings.HasPrefix(result.Installation.Path, explicit) {
		t.Errorf("installed at %q, want it below %q", result.Installation.Path, explicit)
	}
}

// TestContinueIntoClaudeRefusesASourceWithoutAWorkingDirectory pins the refusal at the route level: the
// target's loader finds a session through the working directory it is run from, so a source session that
// records none cannot be installed anywhere resumable. Nothing may be written in that case, and the run
// must fail rather than report an installation nobody can resume.
func TestContinueIntoClaudeRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	sourceRoot := t.TempDir()
	const sessionID = "0c8e5f21-0000-4000-8000-00000000dead"
	relative := filepath.Join("2026", "05", "04", "rollout-2026-05-04T09-15-00-"+sessionID+".jsonl")
	path := filepath.Join(sourceRoot, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating the source directory: %v", err)
	}
	// A rollout with no cwd at all: session metadata, a user turn and an assistant turn, and no record
	// that names a working directory.
	lines := []string{
		`{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","originator":"codex_cli_rs"}}`,
		`{"timestamp":"2026-05-04T09:15:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"timestamp":"2026-05-04T09:15:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the source session: %v", err)
	}
	cfg := fixtureConfig(t)
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		dirs[agent] = roots
	}
	dirs[parser.AgentCodex] = []string{sourceRoot}
	cfg.AgentDirs = dirs

	home, _ := claudeTargetHome(t)
	before := hashTree(t, home)
	_, err := Continue(context.Background(), ContinueOptions{
		Ref: Ref{Provider: "codex", ID: sessionID}, Target: TargetClaude, Config: cfg,
		TargetHome: home, TargetBinary: missingTargetBinary,
	})
	if !errors.Is(err, ErrNoTargetCwd) {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
	if after := hashTree(t, home); !sameTree(before, after) {
		t.Errorf("the refused conversion wrote into the target home: %v -> %v", before, after)
	}
}
