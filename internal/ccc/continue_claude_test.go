package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// claudeTargetHome prepares an isolated Claude Code data directory holding one unrelated session, and
// answers the home and that session's path: a conversion must leave it alone. The home holds a space,
// so the resume line the route prints has to quote the data directory on every host; an assertion
// against a host temp path would instead pin the separator that host happens to spell.
func claudeTargetHome(t *testing.T) (string, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "claude home")
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
		if got := lossCount(t, result.Conversion.Losses, testCase.category).Count; got != testCase.want {
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
	// caller's own home, or nothing at all. The home holds a space, so a shell would act on the bare
	// word: the line must quote the directory exactly as a POSIX shell reads it back.
	if !strings.HasPrefix(result.TargetSession.ResumeCommand, "cd "+result.TargetSession.Cwd+" && ") {
		t.Errorf("resume command %q does not start by the session's working directory", result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, claudeHomeEnv+"='"+home+"'") {
		t.Errorf("resume command %q does not select the data directory it installed into", result.TargetSession.ResumeCommand)
	}
	if want := []string{missingTargetBinary, "-p", "--output-format", "json", "--resume",
		result.TargetSession.SessionID, promptTerminator, resumePromptPlaceholder}; !slices.Equal(result.TargetSession.ExecResumeArguments, want) {
		// The exec form is the check's own command plus the placeholder, terminator included: the resume
		// verification runs its prompt behind one (claude_verify.go), and without it a prompt whose own
		// first word is an option would be read as one by the harness.
		t.Errorf("exec resume arguments = %q, want %q", result.TargetSession.ExecResumeArguments, want)
	}
	if !strings.Contains(result.TargetSession.ExecResumeCommand,
		"-p --output-format json --resume "+result.TargetSession.SessionID+" "+promptTerminator) {
		t.Errorf("exec resume command %q does not name the non-interactive resume behind the prompt terminator",
			result.TargetSession.ExecResumeCommand)
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

// TestContinueIntoClaudeReportsACompactionBoundaryWhoseSummaryHoldsNoText reproduces the one boundary
// shape whose summary cannot travel as text: a source `compacted` record whose message is whitespace
// alone. The inherited Codex reader trims it (internal/parser's handleCompacted), so the conversion
// carries it with no summary text; the target's boundary pair is still written, because the pair is what
// makes the target apply the boundary, and the report counts the empty summary instead of presenting it
// as a summary that traveled.
func TestContinueIntoClaudeReportsACompactionBoundaryWhoseSummaryHoldsNoText(t *testing.T) {
	sourceRoot := t.TempDir()
	const sessionID = "0c8e5f21-0000-4000-8000-00000000beef"
	relative := filepath.Join("2026", "05", "04", "rollout-2026-05-04T09-15-00-"+sessionID+".jsonl")
	path := filepath.Join(sourceRoot, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating the source directory: %v", err)
	}
	lines := []string{
		`{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"cwd":"/home/user/code/example-api","id":"` + sessionID + `","originator":"codex_cli_rs"}}`,
		`{"timestamp":"2026-05-04T09:15:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"timestamp":"2026-05-04T09:16:00Z","type":"compacted","payload":{"message":"   "}}`,
		`{"timestamp":"2026-05-04T09:16:10Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"after the boundary"}]}}`,
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
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: Ref{Provider: "codex", ID: sessionID}, Target: TargetClaude, Config: cfg,
		TargetHome: home, TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Conversion.Carried.CompactionBoundaries != 1 {
		t.Fatalf("carried = %+v, want the boundary written", result.Conversion.Carried)
	}
	if loss := lossCount(t, result.Conversion.Losses, "empty_compaction_summary"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("empty_compaction_summary = %+v, want the boundary's missing summary text counted as omitted", loss)
	}
	installed, err := os.ReadFile(result.Installation.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if !strings.Contains(string(installed), `"subtype":"compact_boundary"`) ||
		!strings.Contains(string(installed), `"isCompactSummary":true`) {
		t.Errorf("the installed session does not hold the boundary pair:\n%s", installed)
	}
	if strings.Contains(string(installed), "   ") {
		t.Errorf("the installed session holds the whitespace-only summary text:\n%s", installed)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
}

// TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation pins the word this route's
// reported invocation carries when the caller names the harness by a relative path. The invocation runs
// from the session's own working directory (measured: the loader finds a session through the directory it
// is run from), so `tools/claude` would be evaluated there -- beside the session, not beside the caller
// whose directory the conversion and the native verification resolved it from -- and the printed line
// would name a program that is not the one the run verified. The caller's own spelling is therefore
// resolved before the argument vector and the line are rendered, while a bare name is left alone: it is a
// PATH lookup, which the directory change does not affect.
func TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation(t *testing.T) {
	cfg := fixtureConfig(t)
	// The caller's directory holds a space -- a word a POSIX shell acts on -- so the printed line has to
	// quote the resolved executable on every host. An assertion against the bare spelling would pin
	// whichever arm the host happens to spell instead of the quoting rule: bare where nothing in the
	// temp path is shell syntax (POSIX), quoted where it holds backslashes (every Windows one).
	caller := filepath.Join(t.TempDir(), "caller with a space")
	binary := filepath.Join(caller, "tools", "claude")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatalf("creating the caller's tools directory: %v", err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("writing the caller's harness: %v", err)
	}
	t.Chdir(caller)
	relative := filepath.Join("tools", "claude")

	home, _ := claudeTargetHome(t)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: codexFixtureRef, Target: TargetClaude, Config: cfg,
		TargetHome: home, TargetBinary: relative,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	report := result.TargetSession
	// The case is only about a relative spelling while the directory the invocation changes into is a
	// different one: with the caller's own directory there, both spellings would resolve alike.
	if cwd := report.Cwd; cwd == "" || cwd == caller {
		t.Fatalf("the session's working directory is %q, want one that is not the caller's %q", cwd, caller)
	}
	if args := report.ResumeArguments; len(args) != 3 || args[0] != binary || args[2] != report.SessionID {
		t.Errorf("resume arguments = %q, want the executable resolved to %q beside the session id",
			args, binary)
	}
	if args := report.ExecResumeArguments; len(args) != 8 || args[0] != binary ||
		args[6] != promptTerminator || args[7] != resumePromptPlaceholder {
		t.Errorf("exec resume arguments = %q, want the same resolved executable and the prompt behind %q",
			args, promptTerminator)
	}
	// The word the line spells for the executable is the one a POSIX shell reads back as the resolved
	// path, which is the quoted form here: the caller's directory holds a space.
	quoted := "'" + binary + "'"
	if !strings.Contains(report.ResumeCommand, "cd "+report.Cwd+" && ") ||
		!strings.Contains(report.ResumeCommand, quoted+" --resume "+report.SessionID) {
		t.Errorf("resume command %q does not change into %q and name the executable there",
			report.ResumeCommand, report.Cwd)
	}
	if !strings.Contains(report.ExecResumeCommand, quoted+" -p ") ||
		!strings.HasSuffix(report.ExecResumeCommand, " "+promptTerminator+" '"+resumePromptPlaceholder+"'") {
		t.Errorf("exec resume command %q does not name the executable in the caller's directory with the "+
			"prompt behind %q", report.ExecResumeCommand, promptTerminator)
	}

	// A bare name the probe could not resolve is a PATH lookup, which the directory change does not
	// affect: it stays the word the caller wrote, and this is what keeps the two cases apart rather than
	// resolving every spelling.
	otherHome, _ := claudeTargetHome(t)
	other, err := Continue(context.Background(), ContinueOptions{
		Ref: codexFixtureRef, Target: TargetClaude, Config: cfg,
		TargetHome: otherHome, TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing with a bare name: %v", err)
	}
	if args := other.TargetSession.ResumeArguments; len(args) != 3 || args[0] != missingTargetBinary {
		t.Errorf("resume arguments = %q, want the bare name %q it was given", args, missingTargetBinary)
	}
}

// TestClaudeInvocationNamesTheExecutableItsProbeFound pins the other half of the same rule: when the probe
// found the executable, the printed invocation names the path it found -- the program the conversion asked
// and the native checks resolved and ran -- rather than the word the caller typed. A shell that reads the
// line after its `cd` would otherwise search PATH again from the session's directory and can run a
// different program there, which is what a bare `claude` passed to a route whose invocation changes
// directory leaves open. The probe's answer is what the report's own binary fields already carry, so the
// line and those fields cannot disagree.
func TestClaudeInvocationNamesTheExecutableItsProbeFound(t *testing.T) {
	route, err := targetRouteFor(TargetClaude)
	if err != nil {
		t.Fatalf("the Claude route is not in this build: %v", err)
	}
	// A probed path distinct from the spelling, with no host dependency: the answer comes from the probe
	// the route is handed, exactly as a real run's does. It holds a space, so the line has to quote it.
	const probed = "/opt/ccc tools/claude"
	invocation := route.Invocation(
		convertedTarget{ID: "8f1a1d2c-0000-4000-8000-000000000001", Cwd: "/home/user/code/project"},
		targetInvocationInput{
			Binary: "claude",
			Probe:  harnessProbe{Binary: probed, Found: true, Version: "2.1.326"},
			Home:   "/tmp/ccc home",
		},
	)
	if invocation.Arguments[0] != probed || invocation.ExecArguments[0] != probed {
		t.Errorf("arguments name %q/%q, want the probed executable %q",
			invocation.Arguments[0], invocation.ExecArguments[0], probed)
	}
	line := renderInvocation(invocation.ExecArguments, invocation.Environment, invocation.WorkingDir)
	if !strings.HasPrefix(line, "cd /home/user/code/project && ") ||
		!strings.Contains(line, "'"+probed+"' -p --output-format json --resume "+
			"8f1a1d2c-0000-4000-8000-000000000001 "+promptTerminator+" '"+resumePromptPlaceholder+"'") {
		t.Errorf("the printed exec line %q does not name the probed executable in the session's directory", line)
	}

	// A probe that found nothing leaves the caller's spelling alone: there is no program to name, and the
	// report's own binary fields say so beside it.
	unresolved := route.Invocation(
		convertedTarget{ID: "8f1a1d2c-0000-4000-8000-000000000002", Cwd: "/home/user/code/project"},
		targetInvocationInput{
			Binary: "claude",
			Probe:  harnessProbe{Binary: "claude", Detail: "claude is not on PATH"},
			Home:   "/tmp/ccc home",
		},
	)
	if unresolved.Arguments[0] != "claude" || unresolved.ExecArguments[0] != "claude" {
		t.Errorf("a probe that found nothing changed the word: %q/%q",
			unresolved.Arguments[0], unresolved.ExecArguments[0])
	}
}

// TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace pins the recorded working directory end
// to end: the project directory the session is filed under, the `cwd` every record carries and the
// printed `cd` all spell the string the source recorded, so a directory whose last component ends in
// whitespace is filed under the project directory *that* string spells to rather than under another
// one -- and the printed line quotes it the way a shell reads it back.
func TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace(t *testing.T) {
	sourceRoot := t.TempDir()
	const sessionID = "0c8e5f21-0000-4000-8000-00000000cafe"
	// A directory name whose last component ends in whitespace: valid on the platforms this build runs on,
	// and a string the inherited Codex reader carries from session_meta verbatim.
	const cwd = "/home/user/code/project "
	relative := filepath.Join("2026", "05", "04", "rollout-2026-05-04T09-15-00-"+sessionID+".jsonl")
	path := filepath.Join(sourceRoot, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating the source directory: %v", err)
	}
	lines := []string{
		`{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"cwd":"` + cwd + `","id":"` + sessionID + `","originator":"codex_cli_rs"}}`,
		`{"timestamp":"2026-05-04T09:15:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
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
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: Ref{Provider: "codex", ID: sessionID}, Target: TargetClaude, Config: cfg,
		TargetHome: home, TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if got := result.TargetSession.Cwd; got != cwd {
		t.Errorf("target working directory = %q, want the source's own %q", got, cwd)
	}
	// The directory the target's own discovery spells for that string: every character outside
	// [A-Za-z0-9] one dash, the trailing space included.
	wantInstalled := filepath.Join(home, claudeProjectsSubdir, ClaudeProjectDir(cwd),
		result.TargetSession.SessionID+".jsonl")
	if result.Installation.Path != wantInstalled {
		t.Errorf("installed at %q, want %q", result.Installation.Path, wantInstalled)
	}
	// The home and the working directory both hold a space, so the printed line quotes both.
	if !strings.HasPrefix(result.TargetSession.ResumeCommand, "cd '"+cwd+"' && ") {
		t.Errorf("resume command %q does not start by the source's own working directory %q",
			result.TargetSession.ResumeCommand, cwd)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
}

// TestContinueIntoClaudeCoalescesOneCallsPerFileProjections reproduces the source shape a multi-file
// patch produces: the inherited Codex reader projects one `apply_patch` call into one call per file it
// names (internal/parser's handleFunctionCall), every projection sharing the call's own id and input,
// which is what the archive's file-keyed views group by. The inherited Claude reader puts the records
// that share a message id back into one message and keeps one tool block per id, so the writer has to
// write the projections as the single call they are projections of: a record per projection would leave
// the installed file holding fewer calls than the report claims to have carried, and the mandatory
// structure check would refuse the conversion after installing it. The projections it did not write as
// their own record are named in the report rather than dropped in silence.
func TestContinueIntoClaudeCoalescesOneCallsPerFileProjections(t *testing.T) {
	sourcePath := fixturePath(t, filepath.Join("codex", "2026", "05", "06",
		"rollout-2026-05-06T10-30-00-"+codexPatchFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)
	home, _ := claudeTargetHome(t)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref: codexPatchFixtureRef, Target: TargetClaude, Config: fixtureConfig(t),
		TargetHome: home, TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	carried := result.Conversion.Carried
	if carried.ToolCalls != 1 || carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the patch's one call and the one result that answered it", carried)
	}
	loss := lossCount(t, result.Conversion.Losses, "coalesced_tool_call_projections")
	if loss.Count != 1 || loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("coalesced_tool_call_projections = %+v, want the one projection written as the call it "+
			"is a projection of counted as carried with a caveat", loss)
	}
	if !strings.Contains(loss.Detail, "per file") {
		t.Errorf("the category's reason does not name the shape it counts: %q", loss.Detail)
	}

	installed, err := os.ReadFile(result.Installation.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	var calls, results int
	for _, line := range strings.Split(strings.TrimRight(string(installed), "\n"), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the installed session holds a line that is not one JSON object: %v\n%s", err, line)
		}
		for _, block := range recordBlocks(record) {
			switch block["type"] {
			case claudeBlockToolUse:
				if block["id"] == "call_02Example000000000000000000" {
					calls++
				}
			case claudeBlockToolResult:
				if block["tool_use_id"] == "call_02Example000000000000000000" {
					results++
				}
			}
		}
	}
	if calls != 1 || results != 1 {
		t.Errorf("the installed session holds %d tool_use and %d tool_result block(s) for the patch's "+
			"call, want one of each: the projections are one call", calls, results)
	}
	if readBack := result.Verification.Structure.ReadBack; readBack.ToolCalls != 1 || readBack.ToolResults != 1 {
		t.Errorf("read-back = %+v, want the inherited reader to show the one call and its result", readBack)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
	if digest := hashFile(t, sourcePath); digest != sourceBefore {
		t.Error("the conversion changed the source session")
	}
}
