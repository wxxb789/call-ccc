package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codexFixtureRefRelative is the committed Codex fixture as a reference relative to that provider's
// configured source root, which is how the CLI resolves a session the caller names by path.
const codexFixtureRefRelative = "2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl"

// claudeShellUnsafeHome answers a Claude Code data directory whose path a POSIX shell would act on --
// it holds a space -- so the printed resume line must quote it on every host. The rule is the one
// main_test.go's shellUnsafeHome pins for the Codex route: an assertion written against the host's own
// temp path would pin whichever arm that host happens to spell (bare where nothing in it is shell
// syntax, quoted where it holds backslashes) instead of the quoting rule itself.
func claudeShellUnsafeHome(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "claude home")
}

// TestContinueToClaudeConvertsInstallsAndReportsTheRoute pins the second route through the CLI the way
// the Codex route's own cases pin theirs: one documented non-interactive invocation converts a Codex
// session into a Claude Code session, the report distinguishes conversion, installation and verification,
// and the resume invocation it prints carries both the data directory and the working directory the
// target's loader finds a session through.
func TestContinueToClaudeConvertsInstallsAndReportsTheRoute(t *testing.T) {
	isolatedEnvironment(t)
	home := claudeShellUnsafeHome(t)
	sourcePath := filepath.Join(fixtureRoot(t), "codex", filepath.FromSlash(codexFixtureRefRelative))
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source fixture: %v", err)
	}

	code, stdout, stderr := runCLI(t, "continue", "codex:"+codexFixtureRefRelative, "--to", "claude",
		"--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Target != "claude" || document.TargetSession.Provider != "claude" {
		t.Errorf("target = %q / provider = %q, want the Claude Code route", document.Target, document.TargetSession.Provider)
	}
	if document.Conversion.State != "converted" || document.Conversion.Format != "claude-code-transcript-jsonl" {
		t.Errorf("conversion = %+v, want a converted Claude Code transcript", document.Conversion)
	}
	if document.Conversion.Carried.ToolCalls != 1 || document.Conversion.Carried.ToolResults != 1 ||
		document.Conversion.Carried.CompactionBoundaries != 1 {
		t.Errorf("carried = %+v, want the fixture's tool pair and boundary", document.Conversion.Carried)
	}
	// The three disclosures this route owes: the model attribution it does not write, the turns before the
	// boundary, and the source's tool vocabulary.
	for _, want := range []string{"message_model_attribution", "pre_compaction_history", "tool_name_vocabulary"} {
		found := false
		for _, loss := range document.Conversion.Losses {
			if loss.Category == want && loss.Count > 0 && loss.Disposition != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("the loss report does not name %q with a count and a disposition: %+v", want, document.Conversion.Losses)
		}
	}
	if len(document.TargetSession.FormatLimits) == 0 {
		t.Error("the report states no format limits, so a consumer cannot tell what the pair cannot express")
	}

	if document.Installation.State != "installed" || document.Installation.Mechanism == "" {
		t.Errorf("installation = %+v, want an installed session", document.Installation)
	}
	if document.Installation.Path != document.TargetSession.Path ||
		!strings.HasPrefix(document.TargetSession.Path, filepath.Join(home, "projects")) {
		t.Errorf("installed at %q (report says %q), want it below %q",
			document.TargetSession.Path, document.Installation.Path, filepath.Join(home, "projects"))
	}
	if document.TargetSession.SessionID == "" || document.TargetSession.Cwd != "/home/user/code/example-api" {
		t.Errorf("target session = %+v, want a fresh identity and the source's working directory", document.TargetSession)
	}
	if document.TargetSession.CLIVersion == "" || document.TargetSession.CLIVersionSource == "" {
		t.Errorf("the report does not say which format version it wrote or where that came from: %+v", document.TargetSession)
	}
	if document.TargetSession.BinaryFound {
		t.Errorf("the report claims to have found %q on PATH", missingTargetBinary)
	}
	// The invocation is reported three ways, and all three have to name the session; the printed line
	// additionally names the working directory and the data directory, without which the loader cannot
	// find the session at all. The home holds a space, so a shell would act on the bare word: the line
	// has to quote the data directory, exactly as a POSIX shell reads it back.
	wantArguments := []string{missingTargetBinary, "--resume", document.TargetSession.SessionID}
	if len(document.TargetSession.ResumeArguments) != len(wantArguments) {
		t.Errorf("resume arguments = %v, want %v", document.TargetSession.ResumeArguments, wantArguments)
	} else {
		for index, want := range wantArguments {
			if document.TargetSession.ResumeArguments[index] != want {
				t.Errorf("resume argument %d = %q, want %q", index, document.TargetSession.ResumeArguments[index], want)
			}
		}
	}
	if document.TargetSession.ResumeEnvironment["CLAUDE_CONFIG_DIR"] != home {
		t.Errorf("resume environment = %v, want the data directory %q", document.TargetSession.ResumeEnvironment, home)
	}
	if !strings.HasPrefix(document.TargetSession.ResumeCommand, "cd /home/user/code/example-api && ") {
		t.Errorf("resume command = %q, want it to start in the session's working directory", document.TargetSession.ResumeCommand)
	}
	for _, want := range []string{"CLAUDE_CONFIG_DIR='" + home + "'", missingTargetBinary, "--resume " + document.TargetSession.SessionID} {
		if !strings.Contains(document.TargetSession.ResumeCommand, want) {
			t.Errorf("resume command %q does not carry %q", document.TargetSession.ResumeCommand, want)
		}
	}
	if !strings.Contains(document.TargetSession.ExecResumeCommand, "-p --output-format json --resume "+document.TargetSession.SessionID) {
		t.Errorf("exec resume command = %q, want the non-interactive form", document.TargetSession.ExecResumeCommand)
	}

	if document.Verification.Structure.State != "verified" {
		t.Fatalf("structure = %+v, want the installed session read back as what was written", document.Verification.Structure)
	}
	if got := document.Verification.Structure.ReadBack; got.ToolCalls != 1 || got.ToolResults != 1 || got.CompactionBoundaries != 1 {
		t.Errorf("read-back = %+v, want the carried tool evidence and boundary", got)
	}
	if document.Verification.Load.State != "not-verified" || !strings.Contains(document.Verification.Load.Detail, missingTargetBinary) {
		t.Errorf("load = %+v, want not-verified naming the absent harness", document.Verification.Load)
	}
	if document.Verification.Resume.State != "not-requested" {
		t.Errorf("resume = %+v, want not-requested at the default level", document.Verification.Resume)
	}

	sourceAfter, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("re-reading the source fixture: %v", err)
	}
	if string(sourceAfter) != string(sourceBefore) {
		t.Error("the conversion changed the source session")
	}
}

// TestContinueToClaudeHumanOutputCarriesTheResumeLine pins the human block, which is what a caller pastes:
// the route, the installed identity and the exact command that resumes it.
func TestContinueToClaudeHumanOutputCarriesTheResumeLine(t *testing.T) {
	isolatedEnvironment(t)
	home := claudeShellUnsafeHome(t)
	code, stdout, stderr := runCLI(t, "continue", "codex:"+codexFixtureRefRelative, "--to", "claude",
		"--target-home", home, "--target-binary", missingTargetBinary)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	for _, want := range []string{
		"→ claude",
		"converted:    claude-code-transcript-jsonl",
		"installed:",
		"target id:",
		"cd /home/user/code/example-api && ",
		"CLAUDE_CONFIG_DIR='" + home + "'",
		missingTargetBinary + " --resume ",
		"structure:    verified",
		"cli_version:",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human block does not carry %q:\n%s", want, stdout)
		}
	}
}

// TestContinueRefusesATargetThisBuildDoesNotWrite pins the refusal's wording: it names every target this
// build writes, so a caller learns the alternatives instead of only the one that failed.
func TestContinueRefusesATargetThisBuildDoesNotWrite(t *testing.T) {
	isolatedEnvironment(t)
	code, _, stderr := runCLI(t, "continue", "codex:"+codexFixtureRefRelative, "--to", "gemini")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, `this build writes codex, claude, not "gemini"`) {
		t.Errorf("stderr = %q, want it to name every target this build writes", stderr)
	}
}
