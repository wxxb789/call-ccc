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

// missingTargetBinary is a name no host has: a test that left the target binary unnamed would probe
// whatever the machine running it happens to have installed, so every test here names one and the
// verification paths are exercised deterministically.
const missingTargetBinary = "ccc-no-such-target-harness"

// hashFile is one file's content digest, for the before/after comparisons a conversion has to satisfy.
func hashFile(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
	digest, err := fileDigest(path)
	if err != nil {
		t.Fatalf("digesting %s: %v", path, err)
	}
	return digest
}

// TestContinueConvertsInstallsAndReadsBackTheFixture is the increment's central property test: one call
// converts a real session into a new Codex session, installs it where that harness keeps its sessions,
// leaves the source and everything else in the target home untouched, and reads the installed file back
// through the inherited reader as exactly what it wrote.
func TestContinueConvertsInstallsAndReadsBackTheFixture(t *testing.T) {
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)

	targetHome := t.TempDir()
	// An unrelated session the target home already holds: a conversion must not touch it.
	unrelated := filepath.Join(targetHome, codexSessionsSubdir, "2026", "01", "02", "rollout-2026-01-02T00-00-00-unrelated.jsonl")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatalf("creating the unrelated session's directory: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"unrelated\"}}\n"), 0o600); err != nil {
		t.Fatalf("writing the unrelated session: %v", err)
	}
	unrelatedBefore := hashFile(t, unrelated)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   targetHome,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}

	if result.Conversion.State != ConversionStateConverted || result.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", result.Conversion)
	}
	if want := (Carried{Messages: 5, UserMessages: 2, AssistantMessages: 3, ToolCalls: 1, ToolResults: 1, CompactionBoundaries: 1, Turns: 2}); result.Conversion.Carried.Messages != want.Messages ||
		result.Conversion.Carried.UserMessages != want.UserMessages ||
		result.Conversion.Carried.AssistantMessages != want.AssistantMessages ||
		result.Conversion.Carried.ToolCalls != want.ToolCalls ||
		result.Conversion.Carried.ToolResults != want.ToolResults ||
		result.Conversion.Carried.CompactionBoundaries != want.CompactionBoundaries ||
		result.Conversion.Carried.Turns != want.Turns {
		t.Errorf("carried = %+v, want %+v", result.Conversion.Carried, want)
	}
	if result.Conversion.Carried.Bytes == 0 {
		t.Error("the conversion reports no bytes")
	}

	if result.Installation.State != InstallationStateInstalled || !result.Installation.Created {
		t.Errorf("installation = %+v, want an installed session", result.Installation)
	}
	if !result.Installation.SourceUnchanged {
		t.Errorf("installation = %+v, want the source reported unchanged", result.Installation)
	}
	installed := result.Installation.Path
	if installed != result.TargetSession.Path {
		t.Errorf("installed path %q and target session path %q disagree", installed, result.TargetSession.Path)
	}
	if want := filepath.Join(targetHome, codexSessionsSubdir, "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+result.TargetSession.SessionID+".jsonl"); installed != want {
		t.Errorf("installed at %q, want %q", installed, want)
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("the installed session is not there: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the installed session's permissions are %v, want 0600", info.Mode().Perm())
	}
	if len(result.TargetSession.FormatLimits) == 0 {
		t.Error("the report states no format limits, so a consumer cannot tell what the pair cannot express")
	}

	if result.TargetSession.SessionID == claudeFixtureRef.ID {
		t.Error("the target identity is the source session's id")
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, result.TargetSession.SessionID) {
		t.Errorf("resume command %q does not name the target session", result.TargetSession.ResumeCommand)
	}
	if result.TargetSession.Cwd != "/home/user/code/example-api" {
		t.Errorf("target working directory = %q, want the source session's", result.TargetSession.Cwd)
	}
	if result.TargetSession.CLIVersion != CodexFormatVersion {
		t.Errorf("cli_version = %q, want the writer's verified format version", result.TargetSession.CLIVersion)
	}
	if result.TargetSession.CLIVersionSource != "writer-pin" {
		t.Errorf("cli_version source = %q, want the writer pin when no harness answered", result.TargetSession.CLIVersionSource)
	}
	if result.TargetSession.BinaryFound {
		t.Errorf("the report claims to have found %q on PATH", missingTargetBinary)
	}
	if result.TargetSession.ModelProvider != "" || result.TargetSession.ProviderSource != "unrecorded" {
		t.Errorf("provider report = %q/%q, want none recorded when no harness answered",
			result.TargetSession.ModelProvider, result.TargetSession.ProviderSource)
	}

	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed session to read back as what was written",
			result.Verification.Structure)
	}
	readBack := result.Verification.Structure.ReadBack
	if readBack.SessionID != "codex:"+result.TargetSession.SessionID {
		t.Errorf("read-back session id = %q, want the target session's", readBack.SessionID)
	}
	if readBack.ToolCalls != 1 || readBack.ToolResults != 1 || readBack.CompactionBoundaries != 1 {
		t.Errorf("read-back = %+v, want the carried tool call, result and boundary", readBack)
	}
	if result.Verification.Load.State != VerificationStateNotVerified {
		t.Errorf("load = %+v, want an explicit not-verified when the harness is absent", result.Verification.Load)
	}
	if !strings.Contains(result.Verification.Load.Detail, missingTargetBinary) {
		t.Errorf("the not-verified detail does not name the missing harness: %q", result.Verification.Load.Detail)
	}
	if len(result.Warnings) == 0 {
		t.Error("no warning reports that the native half could not be verified")
	}

	// The two properties the goal names: the source is unchanged, and nothing else in the target home
	// moved.
	if after := hashFile(t, sourcePath); after != sourceBefore {
		t.Errorf("the source session changed: %s before, %s after", sourceBefore, after)
	}
	if after := hashFile(t, unrelated); after != unrelatedBefore {
		t.Errorf("the unrelated session in the target home changed: %s before, %s after", unrelatedBefore, after)
	}
}

// TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures pins the property for a second
// transcript shape as well: a conversion of a session with thinking, a sidechain turn and an errored
// tool result must leave the whole fixture tree byte-for-byte as it found it.
func TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures(t *testing.T) {
	cfg := fixtureConfig(t)
	before := hashTree(t, fixtureRoot(t))
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          Ref{Provider: "claude", ID: "b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef"},
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v", result.Verification.Structure)
	}
	after := hashTree(t, fixtureRoot(t))
	if len(before) != len(after) {
		t.Fatalf("the fixture tree gained or lost files: %d before, %d after", len(before), len(after))
	}
	for path, digest := range before {
		if after[path] != digest {
			t.Errorf("%s changed during the conversion", path)
		}
	}
	// The shapes this transcript carries are reported rather than silently carried or dropped.
	if loss := lossCount(t, result.Conversion.Losses, "reasoning_text"); loss.Count != 1 {
		t.Errorf("reasoning_text = %+v, want the persisted reasoning trace counted", loss)
	}
	if loss := lossCount(t, result.Conversion.Losses, "sidechain_messages"); loss.Count != 1 {
		t.Errorf("sidechain_messages = %+v, want the subagent turn counted", loss)
	}
	if loss := lossCount(t, result.Conversion.Losses, "tool_result_outcomes"); loss.Count != 1 {
		t.Errorf("tool_result_outcomes = %+v, want the errored result counted", loss)
	}
	if result.Conversion.Carried.ToolCalls != 2 || result.Conversion.Carried.ToolResults != 2 {
		t.Errorf("carried = %+v, want both calls and both results", result.Conversion.Carried)
	}
}

// TestContinueDryRunWritesNothing pins the dry run: the whole conversion and its structural check
// happen, and the target home is not touched at all — neither a home that exists, nor a directory the
// caller named that is not there yet (which is not created, and is reported as still absent).
func TestContinueDryRunWritesNothing(t *testing.T) {
	cfg := fixtureConfig(t)
	targetHome := t.TempDir()
	before := hashTree(t, targetHome)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   targetHome,
		TargetBinary: missingTargetBinary,
		DryRun:       true,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Installation.State != InstallationStateNotInstalled {
		t.Errorf("installation = %+v, want not-installed", result.Installation)
	}
	if result.TargetSession.Path != "" {
		t.Errorf("a dry run reported a target path: %q", result.TargetSession.Path)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the throw-away copy verified", result.Verification.Structure)
	}
	if got := hashTree(t, targetHome); len(got) != len(before) {
		t.Errorf("a dry run wrote into the target home: %d entries before, %d after", len(before), len(got))
	}

	// The one input the promise is checkable on: a home the caller named that is not there yet. The dry
	// run resolves and reports it, and leaves the filesystem alone -- no directory, and no target harness
	// started against it either (a harness writes its own state into the home it runs with).
	missingHome := filepath.Join(t.TempDir(), "fresh-home")
	result, err = Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   missingHome,
		TargetBinary: missingTargetBinary,
		DryRun:       true,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing into a home that is not there yet: %v", err)
	}
	if _, err := os.Stat(missingHome); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run created the target home %s", missingHome)
	}
	if result.TargetSession.Home != missingHome {
		t.Errorf("home = %q, want the directory the caller named (%q)", result.TargetSession.Home, missingHome)
	}
	if result.Installation.Created {
		t.Errorf("installation = %+v, want created false: a dry run installs nothing", result.Installation)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, missingHome) || !strings.Contains(warnings, "does not exist") {
		t.Errorf("the dry run does not report the home as not there yet: %v", result.Warnings)
	}
}

// TestContinueDryRunReportsWhatItVerified pins the dry run's verification block: the structure check
// runs against the throw-away copy, a level the caller asked for comes back not verified with the
// dry-run reason (a dry run installs nothing for the harness to load, so "not requested" would be
// false), a level nobody asked for stays not requested, and the source comparison an installed run
// makes is made here too.
func TestContinueDryRunReportsWhatItVerified(t *testing.T) {
	cfg := fixtureConfig(t)
	for _, testCase := range []struct {
		level       string
		loadState   string
		resumeState string
	}{
		{VerifyNone, VerificationStateNotRequested, VerificationStateNotRequested},
		{VerifyLoad, VerificationStateNotVerified, VerificationStateNotRequested},
		{VerifyResume, VerificationStateNotVerified, VerificationStateNotVerified},
	} {
		t.Run(testCase.level, func(t *testing.T) {
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          claudeFixtureRef,
				Target:       TargetCodex,
				Config:       cfg,
				TargetHome:   t.TempDir(),
				TargetBinary: missingTargetBinary,
				DryRun:       true,
				Verify:       testCase.level,
				ResumePrompt: "a dry run must not run this turn",
			})
			if err != nil {
				t.Fatalf("continuing: %v", err)
			}
			if result.Verification.Structure.State != VerificationStateVerified {
				t.Errorf("structure = %+v, want the throw-away copy verified", result.Verification.Structure)
			}
			if result.Verification.Load.State != testCase.loadState {
				t.Errorf("load state = %q, want %q (detail %q)",
					result.Verification.Load.State, testCase.loadState, result.Verification.Load.Detail)
			}
			if result.Verification.Resume.State != testCase.resumeState {
				t.Errorf("resume state = %q, want %q (detail %q)",
					result.Verification.Resume.State, testCase.resumeState, result.Verification.Resume.Detail)
			}
			for _, block := range []struct {
				name   string
				state  string
				detail string
			}{
				{"load", result.Verification.Load.State, result.Verification.Load.Detail},
				{"resume", result.Verification.Resume.State, result.Verification.Resume.Detail},
			} {
				if strings.TrimSpace(block.detail) == "" {
					t.Errorf("%s state %q is reported without a reason", block.name, block.state)
				}
				if block.state == VerificationStateNotVerified && !strings.Contains(block.detail, "dry run") {
					t.Errorf("%s not-verified detail does not say the dry run is why: %q", block.name, block.detail)
				}
			}
			if !result.Installation.SourceUnchanged {
				t.Errorf("installation = %+v, want the dry run's comparison to report the source unchanged",
					result.Installation)
			}
			if !strings.Contains(result.Installation.Detail, "sha256") {
				t.Errorf("the dry run does not name the digest it compared: %q", result.Installation.Detail)
			}
		})
	}
}

// TestInstallCodexRolloutNeverReplacesASession pins the exclusive creation: the second install at the
// same identity is refused, and the file still holds the first conversion's bytes.
func TestInstallCodexRolloutNeverReplacesASession(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	rollout, err := ConvertToCodexRollout(conversation, CodexTargetOptions{SessionID: "01a1e000-1111-7000-8000-000000000001"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := t.TempDir()
	path, mechanism, err := InstallCodexRollout(rollout, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	if mechanism == "" {
		t.Error("the installation reports no mechanism")
	}
	original := hashFile(t, path)

	second := rollout
	second.Data = []byte("{\"timestamp\":\"2026-05-04T09:15:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"replaced\"}}\n")
	if _, _, err := InstallCodexRollout(second, home); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("installing over an existing session returned %v, want ErrTargetExists", err)
	}
	if after := hashFile(t, path); after != original {
		t.Errorf("the refused install changed the existing session: %s before, %s after", original, after)
	}
}

// TestContinueRequiresAPromptBeforeItRunsATurn pins the rule the CLI also enforces: a resume
// verification writes a real turn into the session, so its text comes from the caller.
func TestContinueRequiresAPromptBeforeItRunsATurn(t *testing.T) {
	cfg := fixtureConfig(t)
	home := t.TempDir()
	_, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyResume,
	})
	if err == nil || !strings.Contains(err.Error(), "resume-prompt") {
		t.Fatalf("error = %v, want a refusal that names the missing prompt", err)
	}
	if entries := hashTree(t, home); len(entries) != 0 {
		t.Errorf("the refused run wrote into the target home: %v", entries)
	}
}

// TestContinueRefusesAnUnsupportedTargetOrLevel pins the two names a caller can get wrong.
func TestContinueRefusesAnUnsupportedTargetOrLevel(t *testing.T) {
	cfg := fixtureConfig(t)
	for _, testCase := range []struct {
		name   string
		opts   ContinueOptions
		expect error
	}{
		{"no target", ContinueOptions{Ref: claudeFixtureRef, Config: cfg, TargetHome: t.TempDir()},
			ErrUnsupportedTarget},
		{"unknown target", ContinueOptions{Ref: claudeFixtureRef, Target: "claude", Config: cfg,
			TargetHome: t.TempDir()}, ErrUnsupportedTarget},
		{"unknown level", ContinueOptions{Ref: claudeFixtureRef, Target: TargetCodex, Config: cfg,
			TargetHome: t.TempDir(), Verify: "perhaps", TargetBinary: missingTargetBinary}, nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			opts := testCase.opts
			opts.TargetBinary = missingTargetBinary
			_, err := Continue(context.Background(), opts)
			if err == nil {
				t.Fatalf("the conversion was accepted")
			}
			if testCase.expect != nil && !errors.Is(err, testCase.expect) {
				t.Errorf("error = %v, want %v", err, testCase.expect)
			}
		})
	}
}

// TestContinueTargetHomeResolution pins where a conversion is allowed to write: an explicit directory is
// created and used, a missing directory nobody named is refused rather than invented, and $CODEX_HOME is
// the answer when the caller names nothing.
func TestContinueTargetHomeResolution(t *testing.T) {
	cfg := fixtureConfig(t)
	explicit := filepath.Join(t.TempDir(), "fresh")
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: claudeFixtureRef, Target: TargetCodex, Config: cfg,
		TargetHome: explicit, TargetBinary: missingTargetBinary, Verify: VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing into an explicit directory: %v", err)
	}
	if !strings.HasPrefix(result.Installation.Path, explicit) {
		t.Errorf("installed at %q, want it below %q", result.Installation.Path, explicit)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("CODEX_HOME", missing)
	if _, err := Continue(context.Background(), ContinueOptions{
		Ref: claudeFixtureRef, Target: TargetCodex, Config: cfg,
		TargetBinary: missingTargetBinary, Verify: VerifyNone,
	}); !errors.Is(err, ErrTargetHomeMissing) {
		t.Fatalf("error = %v, want ErrTargetHomeMissing for a data directory nobody named", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the refused run created %s", missing)
	}

	fromEnv := t.TempDir()
	t.Setenv("CODEX_HOME", fromEnv)
	result, err = Continue(context.Background(), ContinueOptions{
		Ref: claudeFixtureRef, Target: TargetCodex, Config: cfg,
		TargetBinary: missingTargetBinary, Verify: VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing into $CODEX_HOME: %v", err)
	}
	if result.TargetSession.Home != fromEnv {
		t.Errorf("home = %q, want $CODEX_HOME (%q)", result.TargetSession.Home, fromEnv)
	}
}

// TestContinueFallsBackToTheArchiveWhenTheSourceIsGone pins the source-side rule: a session whose file
// the source reader cannot answer is read from the archive instead, and the report says so rather than
// presenting the projection as the file.
func TestContinueFallsBackToTheArchiveWhenTheSourceIsGone(t *testing.T) {
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))

	// A copy of the session, in its own provider root, so the archive can be written from it and the
	// committed fixture never moves.
	root := t.TempDir()
	copyPath := filepath.Join(root, "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
		t.Fatalf("creating the copied project directory: %v", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatalf("writing the copy: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {root}}
	ingestor, err := NewIngestor(cfg, cfg.DBPath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(context.Background(), copyPath); err != nil {
		t.Fatalf("ingesting the copy: %v", err)
	}
	if err := os.Remove(copyPath); err != nil {
		t.Fatalf("removing the copy: %v", err)
	}

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		ArchivePath:  cfg.DBPath,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing from the archive: %v", err)
	}
	if result.Conversion.ReadFrom != "archive" {
		t.Fatalf("read_from = %q, want the archive", result.Conversion.ReadFrom)
	}
	if !result.Installation.SourceUnchanged {
		t.Errorf("installation = %+v, want no source file reported as changed", result.Installation)
	}
	if result.Conversion.SourceFidelity.ReadFrom != "archive" {
		t.Errorf("source fidelity = %+v, want it to say the read came from the archive",
			result.Conversion.SourceFidelity)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "archive") {
		t.Errorf("warnings = %v, want one naming the archive the session was read from", result.Warnings)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the archived conversion to read back as what was written",
			result.Verification.Structure)
	}
}

// TestStructureVerificationNamesWhatTheFileDoesNotHold pins that the round-trip check compares rather
// than agrees: a rollout whose report claims evidence the file does not hold must come back unverified
// with the difference named.
func TestStructureVerificationNamesWhatTheFileDoesNotHold(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	rollout, err := ConvertToCodexRollout(conversation, CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000001"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := t.TempDir()
	if _, _, err := InstallCodexRollout(rollout, home); err != nil {
		t.Fatalf("installing: %v", err)
	}
	sessions := filepath.Join(home, codexSessionsSubdir)

	verification, err := VerifyCodexRolloutStructure(context.Background(), sessions, rollout, cfg)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("an unchanged rollout did not verify: %+v", verification)
	}

	// The same report, with one piece of evidence it did not write: the check has to see it.
	overstated := rollout
	overstated.Items = append(append([]TargetItem(nil), rollout.Items...),
		TargetItem{Kind: ItemAssistant, Text: "a message the file does not hold"})
	verification, err = VerifyCodexRolloutStructure(context.Background(), sessions, overstated, cfg)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Fatal("the structure check accepted a report the file does not support")
	}
	if !strings.Contains(verification.Detail, "dialogue") {
		t.Errorf("the difference report does not name the category: %q", verification.Detail)
	}
}

// TestSourceUnchangedComparesTheFileBeforeAndAfter pins the property the installation reports: the
// source file's own bytes are the same before and after the conversion, and a file that changed while
// it was converted is reported as changed rather than assumed intact.
func TestSourceUnchangedComparesTheFileBeforeAndAfter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"user\"}\n"), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: path}}

	before, err := sourceFileDigest(conversation, "source")
	if err != nil {
		t.Fatalf("digesting the source: %v", err)
	}
	state, detail, err := sourceUnchangedAfter(conversation, "source", before)
	if err != nil {
		t.Fatalf("comparing the source: %v", err)
	}
	if !state {
		t.Errorf("an untouched source was reported as changed: %s", detail)
	}
	if !strings.Contains(detail, before) {
		t.Errorf("the report does not name the digest it compared: %q", detail)
	}

	if err := os.WriteFile(path, []byte("{\"type\":\"user\",\"changed\":true}\n"), 0o600); err != nil {
		t.Fatalf("changing the source: %v", err)
	}
	state, detail, err = sourceUnchangedAfter(conversation, "source", before)
	if err != nil {
		t.Fatalf("comparing the changed source: %v", err)
	}
	if state {
		t.Fatal("a source that changed during the conversion was reported as unchanged")
	}
	if !strings.Contains(detail, "changed while it was converted") {
		t.Errorf("the report does not say what happened: %q", detail)
	}
}

// TestSourceUnchangedSaysWhenThereWasNoFileToCompare pins the archive read's answer: a projection has no
// file of its own, so the report says the check did not run instead of claiming one.
func TestSourceUnchangedSaysWhenThereWasNoFileToCompare(t *testing.T) {
	conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: "/absent/file.jsonl"}}
	digest, err := sourceFileDigest(conversation, "archive")
	if err != nil || digest != "" {
		t.Fatalf("digest = %q, err = %v; an archive read has no source file to digest", digest, err)
	}
	state, detail, err := sourceUnchangedAfter(conversation, "archive", "")
	if err != nil || !state {
		t.Fatalf("state = %v, err = %v", state, err)
	}
	if !strings.Contains(detail, "archive") {
		t.Errorf("the report does not say why no file was compared: %q", detail)
	}
}

// TestSourceChangedWarningsNameTheMove pins the report's warning half: a source that moved while it was
// converted is warned about, not only described in a detail field a reader may never open.
func TestSourceChangedWarningsNameTheMove(t *testing.T) {
	if warnings := sourceChangedWarnings(true); len(warnings) != 0 {
		t.Errorf("an unchanged source produced warnings: %v", warnings)
	}
	warnings := sourceChangedWarnings(false)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "changed while it was converted") {
		t.Errorf("warnings = %v, want one naming the source that moved", warnings)
	}
}
