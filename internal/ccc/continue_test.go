package ccc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
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
	// The fixture's last line is not JSON, and the inherited reader skips it and counts it: the report
	// carries that count as the read's own omission, so a caller reading the loss list learns that the
	// conversation the target holds is not the whole of what the source file records.
	if loss := lossCount(t, result.Conversion.Losses, "malformed_source_lines"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("malformed_source_lines = %+v, want the skipped fixture line counted", loss)
	}
	if result.Conversion.SourceFidelity.MalformedLines != 1 {
		t.Errorf("source fidelity = %+v, want the read's own malformed-line count",
			result.Conversion.SourceFidelity)
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
	if want := filepath.Join(targetHome, codexSessionsSubdir, "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+result.TargetSession.SessionID+".jsonl"); installed != want {
		t.Errorf("installed at %q, want %q", installed, want)
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("the installed session is not there: %v", err)
	}
	// The rule is that the installed session is readable and writable by its owner and by nobody else.
	// Where the filesystem reports POSIX mode bits, that is the installed file's own mode, and it must
	// be exactly what the installer asked for. Windows reports none — every stat answers -rw-rw-rw-
	// there, whatever mode the file was opened with — so what is checkable on such a host is the
	// request itself, which is why the installer names it (codexSessionFileMode); a conversion that
	// widened the mode must still fail on the platform where the answer is invisible.
	if runtime.GOOS == "windows" {
		if codexSessionFileMode.Perm() != 0o600 {
			t.Errorf("the installer asks for mode %v, want 0600 (Windows reports no POSIX mode bits, "+
				"so the request is the half of the rule that is checkable here)", codexSessionFileMode)
		}
	} else if perm := info.Mode().Perm(); perm != codexSessionFileMode.Perm() || perm&0o077 != 0 {
		t.Errorf("the installed session's permissions are %v, want %o — the mode the installer asked "+
			"for, with nothing for group or other", perm, codexSessionFileMode.Perm())
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
	// The fixture's dialogue is written and read back as itself, so no message needs the projection
	// allowance. The count is what a run reports for a message the reader does not read with its own text,
	// so a conversion that counted ordinary messages would be telling its caller the read-back shows
	// fewer messages than the file holds.
	if result.Verification.Structure.NotReadBackVerbatim != 0 {
		t.Errorf("not_read_back_verbatim = %d, want 0 for a fixture the reader reads as written",
			result.Verification.Structure.NotReadBackVerbatim)
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

// TestContinueReportCarriesNoSourceContent pins the content rule for a conversion's own report: the
// core design forbids logging conversation bodies, tool arguments or full prompts by default, and the
// opening prompt's preview the inspection summary carries would put up to 240 bytes of source text — the
// whole prompt, when it is short — into every continuation report, machine-readable ones included. The
// preview stays the inspection surface's own field (an inspection is the command that asks to see the
// session); a conversion report carries identity, shape and losses, never message text.
func TestContinueReportCarriesNoSourceContent(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	result, err := Continue(ctx, ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Source == nil {
		t.Fatal("the report carries no source block")
	}
	if result.Source.FirstUserMessage != "" {
		t.Errorf("the report carries the opening prompt's preview %q", result.Source.FirstUserMessage)
	}
	// The rule is about the serialized report: the source's own words are taken from the read, so the
	// assertion does not repeat fixture content, and it covers every message text the report could have
	// picked up rather than the preview field alone.
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshalling the report: %v", err)
	}
	if strings.Contains(string(encoded), "first_user_message") {
		t.Errorf("the JSON report carries a first_user_message field:\n%s", encoded)
	}
	for _, message := range conversation.Messages {
		text := strings.TrimSpace(message.Content)
		if text == "" {
			continue
		}
		if strings.Contains(string(encoded), text) {
			t.Errorf("the JSON report carries source message text %q:\n%s", text, encoded)
		}
	}
}

// TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures pins the property for a second
// transcript shape as well: a conversion of a session with thinking, a sidechain turn and an errored
// tool result must leave the whole fixture tree byte-for-byte as it found it.
func TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures(t *testing.T) {
	cfg := fixtureConfig(t)
	before := hashTree(t, fixtureRoot(t))
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeToolingFixtureRef,
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
	if loss := lossCount(t, result.Conversion.Losses, "reasoning_text"); loss.Count != 2 {
		t.Errorf("reasoning_text = %+v, want both persisted reasoning traces counted (the one beside its "+
			"dialogue and the reasoning-only turn)", loss)
	}
	if loss := lossCount(t, result.Conversion.Losses, "sidechain_messages"); loss.Count != 1 {
		t.Errorf("sidechain_messages = %+v, want the subagent turn counted", loss)
	}
	if loss := lossCount(t, result.Conversion.Losses, "tool_result_outcomes"); loss.Count != 1 {
		t.Errorf("tool_result_outcomes = %+v, want the errored result counted", loss)
	}
	if result.Conversion.Carried.ToolCalls != 3 || result.Conversion.Carried.ToolResults != 3 {
		t.Errorf("carried = %+v, want all three calls and all three results", result.Conversion.Carried)
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
			if result.Installation.SourceCheckState != SourceCheckUnchanged {
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

// TestInstallCodexRolloutPublishesByTheExclusiveLink pins the publication's *mechanism*, not only its
// guard: the final name is taken by the exclusive link the installer names (`linkSession`, `os.Link`'s
// own seam), so the installer cannot be published by a rename behind an intact guard -- the reversal the
// contract refuses, which the round's mutation showed a guard-only case would stay green under.
func TestInstallCodexRolloutPublishesByTheExclusiveLink(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	rollout, err := ConvertToCodexRollout(conversation,
		CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000001"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}

	link := linkSession
	var linked [][2]string
	linkSession = func(oldname, newname string) error {
		linked = append(linked, [2]string{oldname, newname})
		return link(oldname, newname)
	}
	defer func() { linkSession = link }()

	path, mechanism, err := InstallCodexRollout(rollout, t.TempDir())
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	if mechanism != "hard-link" {
		t.Errorf("mechanism = %q, want the exclusive link", mechanism)
	}
	if len(linked) != 1 || linked[0][1] != path {
		t.Fatalf("the link seam took %v, want exactly one link to the reported path %s", linked, path)
	}
	if from := filepath.Base(linked[0][0]); !strings.Contains(from, ".tmp-") {
		t.Errorf("the link took %q, want the temporary sibling the bytes were written through", linked[0][0])
	}
	if _, err := os.Stat(linked[0][0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temporary source of the link survives at %s: %v", linked[0][0], err)
	}
}

// TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks pins what happens when the exclusive link is
// unavailable: the conversion is refused, nothing is published under the session's name, and no temporary
// file is left behind. A rename would publish it -- and a rename replaces whatever is at the destination,
// which is the overwrite the install rule forbids -- so the fallback is a refusal rather than a rename.
func TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	rollout, err := ConvertToCodexRollout(conversation, CodexTargetOptions{SessionID: "01a1e000-3333-7000-8000-000000000001"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}

	link := linkSession
	linkSession = func(_, _ string) error { return errors.New("link: the filesystem does not support it") }
	defer func() { linkSession = link }()

	home := t.TempDir()
	path, _, err := InstallCodexRollout(rollout, home)
	if !errors.Is(err, ErrNoExclusiveInstall) {
		t.Fatalf("installing onto a filesystem without hard links returned %v, want ErrNoExclusiveInstall", err)
	}
	if !strings.Contains(err.Error(), "hard links") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
	if path != "" {
		t.Errorf("the refused install reported the path %q", path)
	}
	// The directory the installer created is allowed to exist; a session file or a leftover temporary
	// file in it is not, because a reader of that home would see a session that cannot be resumed.
	if entries := hashTree(t, home); len(entries) != 0 {
		t.Errorf("the refused install left files in the target home: %v", entries)
	}
}

// TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome pins the invocation a caller runs: the
// argument vector and the environment beside it, and the shell line rendered from both. Codex takes its
// data directory from CODEX_HOME alone (there is no flag for it), so a command without it opens the
// caller's own default home rather than the one the session was installed into -- and a home or a binary
// path holding a space has to be quoted, or the pasted line splits into two arguments.
func TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome(t *testing.T) {
	cfg := fixtureConfig(t)
	home := filepath.Join(t.TempDir(), "home with a space")
	binary := filepath.Join(t.TempDir(), "my tools", "codex")
	result, err := Continue(context.Background(), ContinueOptions{
		Ref: claudeFixtureRef, Target: TargetCodex, Config: cfg,
		TargetHome: home, TargetBinary: binary, Verify: VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	sessionID := result.TargetSession.SessionID
	report := result.TargetSession
	if !slices.Equal(report.ResumeArguments, []string{binary, "resume", sessionID}) {
		t.Errorf("resume arguments = %q, want the binary the caller named and the session id",
			report.ResumeArguments)
	}
	if !slices.Equal(report.ExecResumeArguments, []string{binary, "exec", "resume", sessionID, "<prompt>"}) {
		t.Errorf("exec resume arguments = %q", report.ExecResumeArguments)
	}
	if len(report.ResumeEnvironment) != 1 || report.ResumeEnvironment["CODEX_HOME"] != home {
		t.Errorf("resume environment = %v, want CODEX_HOME pointing at the installation directory",
			report.ResumeEnvironment)
	}
	want := "CODEX_HOME='" + home + "' '" + binary + "' resume " + sessionID
	if report.ResumeCommand != want {
		t.Errorf("resume command = %q, want %q", report.ResumeCommand, want)
	}
	if wantExec := "CODEX_HOME='" + home + "' '" + binary + "' exec resume " + sessionID + " '<prompt>'"; report.ExecResumeCommand != wantExec {
		t.Errorf("exec resume command = %q, want %q", report.ExecResumeCommand, wantExec)
	}
	// Resolving the home and installing are unchanged by the rendering: the session really is there.
	if result.Installation.State != InstallationStateInstalled ||
		!strings.HasPrefix(result.Installation.Path, home+string(filepath.Separator)) {
		t.Errorf("installation = %+v, want a session below %s", result.Installation, home)
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

// archiveFixtureCopy writes a copy of one committed fixture under a provider root of its own, ingests
// that copy into the configuration's archive and removes the copy again, so the archive is the only place
// the session exists afterwards. It answers the configuration, whose Claude root now points at the copy's
// root: the committed fixture never moves.
func archiveFixtureCopy(t *testing.T, cfg config.Config, fixture, destination string) config.Config {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	root := t.TempDir()
	copyPath := filepath.Join(root, destination)
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
		t.Fatalf("creating the copied project directory: %v", err)
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
	return cfg
}

// sourceFixtureCopy writes a copy of one committed fixture under a provider root of its own and answers
// the configuration, whose Claude root now points at that root, plus the copy's path. A test that has to
// change or remove the source session's file works on the copy: the committed fixture never moves.
func sourceFixtureCopy(t *testing.T, cfg config.Config, fixture, destination string) (config.Config, string) {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	root := t.TempDir()
	copyPath := filepath.Join(root, destination)
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
		t.Fatalf("creating the copied project directory: %v", err)
	}
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatalf("writing the copy: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {root}}
	return cfg, copyPath
}

// TestContinueFallsBackToTheArchiveWhenTheSourceIsGone pins the source-side rule: a session whose file
// the source reader cannot answer is read from the archive instead, and the report says so rather than
// presenting the projection as the file.
func TestContinueFallsBackToTheArchiveWhenTheSourceIsGone(t *testing.T) {
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	cfg = archiveFixtureCopy(t, cfg, sourcePath,
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

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
	if result.Installation.SourceCheckState != SourceCheckNotVerified {
		t.Errorf("installation = %+v, want an archive read reported as a comparison that did not run",
			result.Installation)
	}
	if !strings.Contains(result.Installation.Detail, "archive") {
		t.Errorf("installation = %+v, want the reason no file was compared", result.Installation)
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

// TestContinueFallsBackToTheArchiveWhenTheSourceCannotBeRead pins the other source-side failures the
// fallback has to survive: the provider never answered which source the reference names, or it
// *resolved* the source and then could not read its bytes -- a fingerprint or a parse that failed at the
// I/O level. Either way the source's identity was asked for and the provider could not produce a session
// from it, so this is not "this machine holds no such session": it is a provider failure, and an archive
// that holds the session must still be consulted. Every shape is pinned because all of them used to
// escape readForConversion as bare errors, and the report has to name where the answer came from and
// why, so a reader cannot take the projection for the file.
func TestContinueFallsBackToTheArchiveWhenTheSourceCannotBeRead(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		fail   func(*fixedSourceProvider)
		reason string
	}{
		{
			name:   "the provider could not resolve the reference",
			fail:   func(p *fixedSourceProvider) { p.findErr = errors.New("the provider store could not be listed") },
			reason: "the provider store could not be listed",
		},
		{
			name:   "the fingerprint could not read the file",
			fail:   func(p *fixedSourceProvider) { p.fingerprintErr = errors.New("input/output error") },
			reason: "input/output error",
		},
		{
			name:   "the parse could not read the file",
			fail:   func(p *fixedSourceProvider) { p.parseErr = errors.New("permission denied") },
			reason: "permission denied",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := fixtureConfig(t)
			sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
				claudeFixtureRef.ID+".jsonl"))
			cfg = archiveFixtureCopy(t, cfg, sourcePath,
				filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

			// The source side resolves the reference to the session's own file and then fails to read
			// it, which is the shape no committed fixture produces on its own.
			reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
			provider.source = parser.SourceRef{
				Provider:       parser.AgentClaude,
				Key:            sourcePath,
				DisplayPath:    sourcePath,
				FingerprintKey: sourcePath,
			}
			testCase.fail(provider)

			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          claudeFixtureRef,
				Target:       TargetCodex,
				Config:       cfg,
				Reader:       reader,
				ArchivePath:  cfg.DBPath,
				TargetHome:   t.TempDir(),
				TargetBinary: missingTargetBinary,
				Verify:       VerifyNone,
			})
			if err != nil {
				t.Fatalf("continuing after the source read failed: %v", err)
			}
			if result.Conversion.ReadFrom != "archive" {
				t.Errorf("read_from = %q, want the archive", result.Conversion.ReadFrom)
			}
			warnings := strings.Join(result.Warnings, " ")
			if !strings.Contains(warnings, "archive") {
				t.Errorf("warnings = %v, want one naming the archive the session was read from",
					result.Warnings)
			}
			if !strings.Contains(warnings, testCase.reason) {
				t.Errorf("warnings = %v, want the source's own failure (%q) named beside the archive",
					result.Warnings, testCase.reason)
			}
			if result.Installation.SourceCheckState != SourceCheckNotVerified {
				t.Errorf("installation = %+v, want an archive read reported as a comparison that did not run",
					result.Installation)
			}
			if result.Verification.Structure.State != VerificationStateVerified {
				t.Errorf("structure = %+v, want the archived conversion to read back as what was written",
					result.Verification.Structure)
			}
		})
	}

	// The path half of the same rule: a reference that names a file is offered to the provider as a
	// stored path (reader.go's resolvePathReference), so a provider that fails *there* has to be
	// classified the same way -- the sentinel belongs to the provider's resolution whichever question it
	// was asked, and the archive must still be consulted.
	t.Run("a path reference whose provider fails still reaches the archive", func(t *testing.T) {
		ctx := context.Background()
		cfg := fixtureConfig(t)
		root := t.TempDir()
		copyPath := filepath.Join(root, "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
		data, err := os.ReadFile(fixturePath(t, filepath.Join("claude",
			"-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")))
		if err != nil {
			t.Fatalf("reading the fixture: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
			t.Fatalf("creating the copied project directory: %v", err)
		}
		if err := os.WriteFile(copyPath, data, 0o600); err != nil {
			t.Fatalf("writing the copy: %v", err)
		}
		cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {root}}
		ingestor, err := NewIngestor(cfg, cfg.DBPath)
		if err != nil {
			t.Fatalf("building the ingestor: %v", err)
		}
		if err := ingestor.IngestPaths(ctx, copyPath); err != nil {
			t.Fatalf("ingesting the copy: %v", err)
		}

		reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
		provider.findErr = errors.New("the stored-path lookup could not run")
		result, err := Continue(ctx, ContinueOptions{
			Ref:          Ref{Provider: "claude", ID: copyPath},
			Target:       TargetCodex,
			Config:       cfg,
			Reader:       reader,
			ArchivePath:  cfg.DBPath,
			TargetHome:   t.TempDir(),
			TargetBinary: missingTargetBinary,
			Verify:       VerifyNone,
		})
		if err != nil {
			t.Fatalf("continuing after the path lookup failed: %v", err)
		}
		if result.Conversion.ReadFrom != "archive" {
			t.Errorf("read_from = %q, want the archive", result.Conversion.ReadFrom)
		}
		warnings := strings.Join(result.Warnings, " ")
		for _, want := range []string{"archive", "the stored-path lookup could not run"} {
			if !strings.Contains(warnings, want) {
				t.Errorf("warnings = %v, want %q named beside the archive", result.Warnings, want)
			}
		}
	})

	// The other half of the obligation: a source that failed at the I/O level does not get to pre-empt the
	// archive's own answer either. A reference two archive rows answer is the archive's ambiguity, with
	// both candidate identities in the error, exactly as it is when the source side answered "no such
	// session" -- the failure a caller is told about is the one the archive gave, because that is the list
	// of spellings a caller can retry with.
	t.Run("the archive's own refusal is the reported failure", func(t *testing.T) {
		ctx := context.Background()
		cfg := fixtureConfig(t)
		sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
			claudeFixtureRef.ID+".jsonl"))
		reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
		provider.source = parser.SourceRef{
			Provider:       parser.AgentClaude,
			Key:            sourcePath,
			DisplayPath:    sourcePath,
			FingerprintKey: sourcePath,
		}
		provider.fingerprintErr = errors.New("input/output error")

		dir := dbtest.MkdirTempWithCleanup(t)
		archivePath := filepath.Join(dir, "sessions.db")
		database := dbtest.OpenTestDBAt(t, archivePath)
		for _, machine := range []string{"host-a", "host-b"} {
			dbtest.SeedSession(t, database, machine+"~"+claudeFixtureRef.ID, "example-api",
				func(session *db.Session) {
					session.Agent = "claude"
					session.Machine = machine
				})
		}
		if err := database.Close(); err != nil {
			t.Fatalf("closing the seeded archive: %v", err)
		}

		_, err := Continue(ctx, ContinueOptions{
			Ref:          claudeFixtureRef,
			Target:       TargetCodex,
			Config:       cfg,
			Reader:       reader,
			ArchivePath:  archivePath,
			TargetHome:   t.TempDir(),
			TargetBinary: missingTargetBinary,
			DryRun:       true,
			Verify:       VerifyNone,
		})
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("a source that failed to read and an archive two rows answer returned %v, "+
				"want the archive's ambiguity report", err)
		}
		if len(ambiguous.Candidates) != 2 {
			t.Errorf("candidates = %v, want both stored copies of the requested session",
				ambiguous.Candidates)
		}
		if errors.Is(err, ErrNotFound) {
			t.Errorf("the archive's ambiguity is also reported as absence: %v", err)
		}
	})
}

// TestContinueReportsTheArchivesAmbiguityInsteadOfAbsence pins which failure a conversion reports when
// the source side cannot answer the reference and the archive refuses it as well.
//
// An archive keeps one row per remote copy of a session an AgentsView sync has seen (`<host>~<id>`), so
// a reference can name several of them: `archive.Read` answers with their identities, and that list is
// the only spelling a caller can retry with. Returning the source side's error instead replaced it with
// "the source session was not found" -- a conversion that reported a session absent from a machine
// whose archive holds it twice, and no candidate to name. The other half is pinned here too: a reference
// neither side holds still answers with the source's own error, because then the two sides agree and
// there is no candidate list to offer.
func TestContinueReportsTheArchivesAmbiguityInsteadOfAbsence(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	// The source side reads no session: an empty provider root is what "this machine has no file for
	// that reference" looks like, and the archive is the only side that can answer.
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {t.TempDir()}}

	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	seedCopy := func(id, machine string) {
		t.Helper()
		dbtest.SeedSession(t, database, id, "example-api", func(session *db.Session) {
			session.Agent = "claude"
			session.Machine = machine
		})
	}
	// Two remote copies of the requested Claude session: the archive can name both, but not choose.
	seedCopy("host-a~"+claudeFixtureRef.ID, "host-a")
	seedCopy("host-b~"+claudeFixtureRef.ID, "host-b")
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}
	options := func(ref Ref) ContinueOptions {
		return ContinueOptions{
			Ref:          ref,
			Target:       TargetCodex,
			Config:       cfg,
			ArchivePath:  archivePath,
			TargetHome:   t.TempDir(),
			TargetBinary: missingTargetBinary,
			DryRun:       true,
			Verify:       VerifyNone,
		}
	}

	_, err := Continue(ctx, options(claudeFixtureRef))
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("a reference two archive rows answer returned %v, want an ambiguity report", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("candidates = %v, want both stored copies of the requested session", ambiguous.Candidates)
	}

	// A reference no row answers keeps the source's own answer: both sides say this machine has no such
	// session, and there is no candidate list an archive could have offered instead.
	_, err = Continue(ctx, options(Ref{Provider: "claude", ID: "3c1f4b6e-0d2a-4c8f-9e21-77b0a5d43f12"}))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("a reference neither side holds returned %v, want %v", err, ErrNotFound)
	}
	var absent *AmbiguousError
	if errors.As(err, &absent) {
		t.Errorf("a reference neither side holds was reported as ambiguous: %v", err)
	}
}

// TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce pins the archive half of the duplicate rule
// with payloads the default configuration does not block. An archive read exposes a call's outcome
// twice -- as the events the archive kept on the call and as the summary internal/db restored from
// them -- and one recorded outcome has to reach the target as one function_call_output record. Three of
// the tooling fixture's results exercise it: its Read result is in a category the default configuration
// blocks (the archive keeps its length and no text), and its two Bash results are retained -- one an
// error, one the text of a JSON string -- so each of those would otherwise be written twice.
func TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce(t *testing.T) {
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-tooling",
		claudeToolingFixtureRef.ID+".jsonl"))
	cfg = archiveFixtureCopy(t, cfg, sourcePath,
		filepath.Join("-home-user-code-tooling", claudeToolingFixtureRef.ID+".jsonl"))

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeToolingFixtureRef,
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
	if result.Conversion.Carried.ToolResults != 2 {
		t.Errorf("carried = %+v, want the two results the archive retains", result.Conversion.Carried)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the archived conversion to read back as what was written",
			result.Verification.Structure)
	}
	if got := result.Verification.Structure.ReadBack.ToolResults; got != 2 {
		t.Errorf("read-back tool results = %d, want two", got)
	}
	installed, err := os.ReadFile(result.Installation.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	outputs := 0
	jsonLiteral := ""
	for _, line := range strings.Split(strings.TrimSpace(string(installed)), "\n") {
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				Type   string `json:"type"`
				Output string `json:"output"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decoding a rollout line: %v", err)
		}
		if record.Type != codexRecordResponseItem || record.Payload.Type != codexItemFunctionOutput {
			continue
		}
		outputs++
		switch {
		case strings.Contains(record.Payload.Output, "no such file or directory"):
		case record.Payload.Output == `"maxBackoffSeconds"`:
			// The archive stores the result's text, not the provider's content field, so the
			// conversion must carry those quotation marks unchanged.
			jsonLiteral = record.Payload.Output
		default:
			t.Errorf("an output record holds %q, want one of the archive's retained results",
				record.Payload.Output)
		}
	}
	if jsonLiteral == "" {
		t.Error("the JSON-string result lost its quotation marks on the way through the archive")
	}
	if outputs != 2 {
		t.Errorf("the installed session holds %d function_call_output record(s), want two: a summary that "+
			"repeats the call's own event is the same outcome, not a second one", outputs)
	}
	// The blocked category is named rather than silently gone, and the outcome that arrived as both an
	// event and a summary is counted once.
	if loss := lossCount(t, result.Conversion.Losses, "empty_tool_results"); loss.Count != 1 {
		t.Errorf("empty_tool_results = %+v, want the blocked result counted once", loss)
	}
}

// TestResumeVerificationCommandNamesTheHomeItRanAgainst pins the command a resume verification reports:
// it is a line a reader may paste, so it selects the data directory the turn ran against. Codex has no
// flag for that directory (its loader takes it from CODEX_HOME alone), so a line without the environment
// resumes the reader's own default home -- and a home or a binary path holding a space has to be quoted
// or the pasted line splits into two arguments.
func TestResumeVerificationCommandNamesTheHomeItRanAgainst(t *testing.T) {
	arguments := []string{"exec", "resume", "01a1e000-4444-7000-8000-000000000001", "a turn's prompt",
		"--skip-git-repo-check", "-c", `sandbox_mode="read-only"`}
	got := resumeCommandLine("/opt/my tools/codex", arguments,
		"/tmp/a home with spaces")
	want := "CODEX_HOME='/tmp/a home with spaces' '/opt/my tools/codex' exec resume " +
		"01a1e000-4444-7000-8000-000000000001 " + `'a turn'\''s prompt'` + " " +
		`--skip-git-repo-check -c 'sandbox_mode="read-only"'`
	if got != want {
		t.Errorf("resume command line =\n%s\nwant\n%s", got, want)
	}
	// A caller that named no home reports no environment rather than an empty assignment.
	if got := resumeCommandLine("codex", []string{"resume", "id"}, "  "); got != "codex resume id" {
		t.Errorf("resume command line without a home = %q, want it to carry no environment", got)
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
	// The difference report is the failure path a caller reads (it is serialized into the JSON document
	// and printed beside the failure), so it identifies the item rather than quoting it: the text of the
	// evidence the file does not hold is never in the report, and what names it is its size and digest.
	missing := "a message the file does not hold"
	if strings.Contains(verification.Detail, missing) {
		t.Errorf("the difference report carries the evidence it could not match: %q", verification.Detail)
	}
	if !strings.Contains(verification.Detail, describeText(missing)) {
		t.Errorf("the difference report does not identify the missing evidence: %q", verification.Detail)
	}
}

// TestStructureVerificationReadsTheInstalledPathItWasGiven pins which file the structural check reads:
// the path it was handed (TargetPath), never whichever copy of the session's id the target's own
// sessions tree would resolve to. The inherited reader's id lookup walks every dated directory under
// the root and prefers the lexically first copy of a UUID, so a target home that already holds another
// copy of the id -- or merely a long history of unrelated sessions -- would otherwise decide what the
// check compares, and how much of the home it walks, although the check knows the installed file's path
// (review round 12's finding 2).
//
// The decoy is planted where that lookup prefers it -- an earlier dated directory under the same
// sessions root, holding the same identity and one different model-visible message -- and the premise is
// asserted first: asked for the id, the same reader the check builds answers with the decoy. A check that
// resolved the id would read the decoy and report the installed file as missing the message the document
// writes; the mutation recorded for this rule showed exactly that.
func TestStructureVerificationReadsTheInstalledPathItWasGiven(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	const sessionID = "01a1e000-2222-7000-8000-000000000006"
	rollout, err := ConvertToCodexRollout(conversation, CodexTargetOptions{SessionID: sessionID})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := t.TempDir()
	installed, _, err := InstallCodexRollout(rollout, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	sessions := filepath.Join(home, codexSessionsSubdir)

	// The decoy's conversation is the installed one with one model-visible text replaced, so it is
	// certainly a readable session of the same id whose read-back cannot equal the document. The text is
	// taken from the document and required to be in the file's own bytes, so the replacement is the one
	// this case means to make.
	marker := "a decoy session another agent wrote"
	replaced := ""
	for _, item := range rollout.Items {
		if item.Kind == ItemAssistant && strings.TrimSpace(item.Text) != "" &&
			bytes.Contains(rollout.Data, []byte(item.Text)) {
			replaced = item.Text
			break
		}
	}
	if replaced == "" {
		t.Fatalf("no assistant text the conversion wrote is present in the file's own bytes, so this "+
			"case cannot plant a different conversation (%d item(s))", len(rollout.Items))
	}
	decoyBytes := bytes.ReplaceAll(rollout.Data, []byte(replaced), []byte(marker))
	if bytes.Equal(decoyBytes, rollout.Data) {
		t.Fatal("the decoy holds the installed bytes: it would not tell an id-based read-back from a path-based one")
	}
	decoy := filepath.Join(sessions, "2001", "01", "01", "rollout-2001-01-01T00-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(decoy), 0o700); err != nil {
		t.Fatalf("creating the decoy's directory: %v", err)
	}
	if err := os.WriteFile(decoy, decoyBytes, 0o600); err != nil {
		t.Fatalf("writing the decoy: %v", err)
	}

	// The premise: the reader this check builds, asked for the session's id -- the resolution the check
	// must not make -- answers with the decoy's own conversation rather than with the installed file's. A
	// reader that stopped preferring the decoy would leave this case pinning nothing, so the premise is
	// asserted, not assumed.
	premise := cfg
	premiseDirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		premiseDirs[agent] = roots
	}
	premiseDirs[parser.AgentCodex] = []string{sessions}
	premise.AgentDirs = premiseDirs
	premise.DisabledAgents = nil
	byID, err := NewSourceReader(premise).Read(ctx, Ref{Provider: string(parser.AgentCodex), ID: sessionID})
	if err != nil {
		t.Fatalf("reading the decoy by id: %v", err)
	}
	decoyRead := false
	for _, message := range byID.Messages {
		if strings.Contains(message.Content, marker) || strings.Contains(message.Dialogue, marker) {
			decoyRead = true
			break
		}
	}
	if !decoyRead {
		t.Fatalf("the reader's id lookup did not answer with the decoy's own conversation: this case " +
			"pins the read path only while that lookup prefers the decoy")
	}

	verification, err := VerifyCodexRolloutStructure(ctx, sessions, rollout, cfg)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.TargetPath != installed {
		t.Errorf("the verification names %s, want the installed file %s", verification.TargetPath, installed)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("the check read something other than the installed file: %+v", verification)
	}
}

// TestStructureVerificationAsksTheFileForTheMessageRecord pins the question the structural check asks the
// installed file about a written dialogue message, by mutating the file the way a review's finding did.
//
// The target writes every message twice: a response_item, which is what its model history is built from,
// and an event_msg holding the same text, which is what its own view of the conversation is built from --
// and the inherited reader reads no message from the event. A file whose response_item was dropped while
// its event stayed is therefore a session the target can load with the message missing from its model
// history, and its own reader agrees there is no message; a check that asked the file whether it held the
// text *somewhere* found the words in the event and reported that session verified.
func TestStructureVerificationAsksTheFileForTheMessageRecord(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	rollout, err := ConvertToCodexRollout(conversation,
		CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000002"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := t.TempDir()
	path, _, err := InstallCodexRollout(rollout, home)
	if err != nil {
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

	// The message to drop: the first user message the conversion wrote. Its record and its event both
	// hold the text, so the file still holds every word after the mutation.
	dropped := ""
	for _, item := range rollout.Items {
		if item.Kind == ItemUser {
			dropped = item.Text
			break
		}
	}
	if dropped == "" {
		t.Fatalf("the fixture holds no user message to drop: %d item(s)", len(rollout.Items))
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(installed), "\n"), "\n")
	mutated := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the installed session holds a line that is not a record: %v\n%s", err, line)
		}
		if !removed && record.Type == "response_item" && record.Payload.Role == "user" &&
			len(record.Payload.Content) > 0 && record.Payload.Content[0].Text == dropped {
			removed = true
			continue
		}
		mutated = append(mutated, line)
	}
	if !removed {
		t.Fatalf("the installed session holds no user response_item for %q", dropped)
	}
	if err := os.WriteFile(path, []byte(strings.Join(mutated, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the mutated session: %v", err)
	}
	// The file really does still hold the text: the event the mutation kept is the only thing that does.
	if !strings.Contains(strings.Join(mutated, "\n"), dropped) {
		t.Fatalf("the mutated session no longer holds %q, so it does not pin this rule", dropped)
	}

	verification, err = VerifyCodexRolloutStructure(context.Background(), sessions, rollout, cfg)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Fatal("the structure check accepted a session whose message record is gone")
	}
	if !strings.Contains(verification.Detail, "dialogue") {
		t.Errorf("the difference report does not name the category: %q", verification.Detail)
	}
	if !strings.Contains(verification.Detail, describeText(dropped)) {
		t.Errorf("the difference report does not identify the missing message: %q", verification.Detail)
	}
	if strings.Contains(verification.Detail, dropped) {
		t.Errorf("the difference report carries the evidence it could not match: %q", verification.Detail)
	}
}

// TestStructureVerificationReportsDialogueRecordsOutOfOrder pins the file's own dialogue order at the
// level the finding named: the installed file's records, read back through the inherited reader. Two
// messages whose records are swapped leave every text in the file, so a check that asked the file
// whether it holds each message answers the same as for the file the conversion wrote -- while the
// conversation the file holds is not the one the document writes.
//
// Both shapes of the reader's projection are exercised, because the read-back answers differently for
// each. A document whose two prompts open with an injected-context envelope comes back as the same
// assistant message whichever order its records stand in, so the swap is invisible to the read-back
// alone. A real fixture's first two records are a user turn and the assistant turn after it, so moving
// them also puts the moved message where the document does not write it -- further evidence beside the
// file's own records.
func TestStructureVerificationReportsDialogueRecordsOutOfOrder(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)

	// swapFirstDialogueRecords moves the installed file's first two dialogue records past each other and
	// answers the texts that were in them, after checking that the file still holds both: the mutation
	// moved the records rather than dropping anything.
	swapFirstDialogueRecords := func(t *testing.T, path string) (string, string) {
		t.Helper()
		installed, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the installed session: %v", err)
		}
		lines := strings.Split(strings.TrimSuffix(string(installed), "\n"), "\n")
		// The dialogue records, in the file's own order: the response_items whose payload is a message
		// the inherited reader reads a turn from.
		var dialogue []int
		var texts []string
		for index, line := range lines {
			if !json.Valid([]byte(line)) {
				t.Fatalf("the installed session holds a line that is not a record: %s", line)
			}
			text, ok := dialogueRecordText(line)
			if !ok {
				continue
			}
			dialogue = append(dialogue, index)
			texts = append(texts, text)
		}
		if len(dialogue) < 2 {
			t.Fatalf("the installed session holds %d dialogue record(s), so this case pins nothing",
				len(dialogue))
		}
		lines[dialogue[0]], lines[dialogue[1]] = lines[dialogue[1]], lines[dialogue[0]]
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatalf("writing the mutated session: %v", err)
		}
		// The mutation moved the two records rather than dropping anything: the records the mutated file
		// holds still decode to both texts.
		held := map[string]bool{}
		for _, line := range lines {
			if text, ok := dialogueRecordText(line); ok {
				held[text] = true
			}
		}
		for _, text := range texts[:2] {
			if !held[text] {
				t.Fatalf("the mutated session no longer holds %q, so it does not pin this rule", text)
			}
		}
		return texts[0], texts[1]
	}

	t.Run("a document whose prompts the reader projects away", func(t *testing.T) {
		const first = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
		const second = "# AGENTS.md\nThe retry budget is a constant in internal/retry/budget.go."
		const answer = "I read both notes."
		conversation := testConversation(
			Message{Ordinal: 0, Role: string(parser.RoleUser), Content: first, Dialogue: first},
			Message{Ordinal: 1, Role: string(parser.RoleUser), Content: second, Dialogue: second},
			Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: answer, Dialogue: answer},
		)
		rollout, err := ConvertToCodexRollout(conversation,
			CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000004"})
		if err != nil {
			t.Fatalf("converting: %v", err)
		}
		home := t.TempDir()
		path, _, err := InstallCodexRollout(rollout, home)
		if err != nil {
			t.Fatalf("installing: %v", err)
		}
		sessions := filepath.Join(home, codexSessionsSubdir)
		verification, err := VerifyCodexRolloutStructure(ctx, sessions, rollout, cfg)
		if err != nil {
			t.Fatalf("verifying: %v", err)
		}
		// The premise: the inherited reader reads no message for either prompt, so the read-back is the
		// assistant's message alone and the file's own records are what carry the two prompts.
		if verification.State != VerificationStateVerified || verification.NotReadBackVerbatim != 2 {
			t.Fatalf("the file the conversion wrote did not verify with both prompts counted: %+v",
				verification)
		}
		swapFirstDialogueRecords(t, path)
		verification, err = VerifyCodexRolloutStructure(ctx, sessions, rollout, cfg)
		if err != nil {
			t.Fatalf("verifying: %v", err)
		}
		if verification.State == VerificationStateVerified {
			t.Fatal("the check accepted a file whose projected dialogue records stand in another order")
		}
		if !strings.Contains(verification.Detail, "out of order") ||
			!strings.Contains(verification.Detail, describeText(first)) {
			t.Errorf("the difference report does not name the out-of-place message by its size and digest: %q",
				verification.Detail)
		}
		if strings.Contains(verification.Detail, first) {
			t.Errorf("the difference report carries the evidence it compared: %q", verification.Detail)
		}
	})

	t.Run("a fixture whose first two records are swapped", func(t *testing.T) {
		conversation, err := NewSourceReader(cfg).Read(ctx, claudeFixtureRef)
		if err != nil {
			t.Fatalf("reading the fixture: %v", err)
		}
		rollout, err := ConvertToCodexRollout(conversation,
			CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000003"})
		if err != nil {
			t.Fatalf("converting: %v", err)
		}
		home := t.TempDir()
		path, _, err := InstallCodexRollout(rollout, home)
		if err != nil {
			t.Fatalf("installing: %v", err)
		}
		sessions := filepath.Join(home, codexSessionsSubdir)
		verification, err := VerifyCodexRolloutStructure(ctx, sessions, rollout, cfg)
		if err != nil {
			t.Fatalf("verifying: %v", err)
		}
		if verification.State != VerificationStateVerified {
			t.Fatalf("an unchanged rollout did not verify: %+v", verification)
		}
		first, _ := swapFirstDialogueRecords(t, path)
		verification, err = VerifyCodexRolloutStructure(ctx, sessions, rollout, cfg)
		if err != nil {
			t.Fatalf("verifying: %v", err)
		}
		if verification.State == VerificationStateVerified {
			t.Fatal("the structure check accepted a session whose dialogue records stand in another order")
		}
		if !strings.Contains(verification.Detail, "out of order") ||
			!strings.Contains(verification.Detail, describeText(first)) {
			t.Errorf("the difference report does not name the out-of-place message by its size and digest: %q",
				verification.Detail)
		}
		if strings.Contains(verification.Detail, first) {
			t.Errorf("the difference report carries the evidence it compared: %q", verification.Detail)
		}
	})
}

// TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce pins the count a read-back
// answers when its text overlaps two written messages. The inherited reader drops a user prompt that
// opens with `# AGENTS.md` (preprocessCodexUserTextBlocks), and the prompt it drops can be followed by a
// turn whose own text the read-back shows: `# AGENTS.md\nhello` and then `hello` is exactly that pair.
// The dropped prompt's own text is not shown, so it is counted; the later turn's own text is shown, so it
// is not -- and the read-back holds one message for that turn, not one per written message a pairing
// could read it as (review round 11's finding 2, whose count was read off that pairing).
//
// The second shape is the same class one step further: two later turns carrying the same text, with the
// reader returning one message for each. A count that read the pairing answers two there as well, while
// the read-back shows both turns' own text.
func TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	const answer = "I read the policy."
	for _, testCase := range []struct {
		name    string
		prompts []string
		shown   int
	}{
		{
			name:    "the omitted prompt is followed by one turn carrying its text",
			prompts: []string{"# AGENTS.md\nhello", "hello"},
			shown:   1,
		},
		{
			name:    "the omitted prompt is followed by two turns carrying the same text",
			prompts: []string{"# AGENTS.md\nhello", "hello", "hello"},
			shown:   2,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			messages := make([]Message, 0, len(testCase.prompts)+1)
			for ordinal, prompt := range testCase.prompts {
				messages = append(messages, Message{
					Ordinal: ordinal, Role: string(parser.RoleUser), Content: prompt, Dialogue: prompt,
				})
			}
			messages = append(messages, Message{
				Ordinal: len(messages), Role: string(parser.RoleAssistant), Content: answer, Dialogue: answer,
			})
			rollout, err := ConvertToCodexRollout(testConversation(messages...),
				CodexTargetOptions{SessionID: "01a1e000-2222-7000-8000-000000000005"})
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			// Half the premise: the conversion carries every prompt, so the file holds a record for each
			// of them and the count answers the reader's projection rather than a loss.
			written := 0
			for _, item := range rollout.Items {
				if item.Kind == ItemUser {
					written++
				}
			}
			if written != len(testCase.prompts) {
				t.Fatalf("the conversion wrote %d user message(s), want %d", written, len(testCase.prompts))
			}
			home := t.TempDir()
			if _, _, err := InstallCodexRollout(rollout, home); err != nil {
				t.Fatalf("installing: %v", err)
			}
			verification, err := VerifyCodexRolloutStructure(ctx,
				filepath.Join(home, codexSessionsSubdir), rollout, cfg)
			if err != nil {
				t.Fatalf("verifying: %v", err)
			}
			// The other half: the reader drops the envelope prompt and reads every later turn, which is
			// what makes an overlapping count possible at all.
			if verification.ReadBack.UserMessages != testCase.shown {
				t.Fatalf("the read-back shows %d user message(s), want %d: the reader's own projection is "+
					"the shape this case pins", verification.ReadBack.UserMessages, testCase.shown)
			}
			if verification.State != VerificationStateVerified {
				t.Fatalf("the file the conversion wrote did not verify: %+v", verification)
			}
			if verification.NotReadBackVerbatim != 1 {
				t.Errorf("not_read_back_verbatim = %d, want 1: the omitted prompt's own text is the one "+
					"the read-back does not show", verification.NotReadBackVerbatim)
			}
			if !strings.Contains(verification.Detail, "1 written message(s)") {
				t.Errorf("the verification detail = %q, want the count named", verification.Detail)
			}
		})
	}
}

// dialogueRecordText answers the text one installed line carries, when the line is a dialogue record the
// inherited reader reads a turn from: a response_item whose payload is a message, whose role is the
// user's or the assistant's, and whose content holds a text block. A line that is not that shape answers
// false, so the walk applies to the file's own bytes rather than to a hand-written expectation of them.
func dialogueRecordText(line string) (string, bool) {
	var record struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return "", false
	}
	if record.Type != "response_item" || record.Payload.Type != "message" ||
		(record.Payload.Role != "user" && record.Payload.Role != "assistant") {
		return "", false
	}
	for _, block := range record.Payload.Content {
		if block.Text != "" {
			return block.Text, true
		}
	}
	return "", false
}

// TestContinueOmitsOutputTheInheritedReaderCannotReadBack pins the rule that made an LF-only tool result
// a false failure: a conversion may only write evidence its own round-trip check can read back, because
// that check — not the source — is the run's verdict about the installed file. The inherited Codex reader
// trims a function_call_output's text and records no result at all for an output that holds only
// whitespace, so such an output is omitted under its own loss category instead of being written and then
// reported as missing from the file the conversion had just written (which failed the run even with
// --verify none, since the structural check always runs).
//
// The transcript is the native shape: the source reader really parses the LF-only result, so this test
// fails if either the writer writes it or the reader's normalisation reads as a difference.
func TestContinueOmitsOutputTheInheritedReaderCannotReadBack(t *testing.T) {
	cfg, _ := claudeBashTranscript(t, "\n", false)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Conversion.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want no tool result: the recorded output holds only whitespace",
			result.Conversion.Carried)
	}
	if loss := lossCount(t, result.Conversion.Losses, "whitespace_only_tool_results"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("whitespace_only_tool_results = %+v, want the LF-only output counted as omitted", loss)
	}
	if loss := lossCount(t, result.Conversion.Losses, "empty_tool_results"); loss.Count != 0 {
		t.Errorf("empty_tool_results = %+v, want 0: the read held text, it was only whitespace", loss)
	}
	installed, err := os.ReadFile(result.Installation.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if strings.Contains(string(installed), codexItemFunctionOutput) {
		t.Errorf("the installed session holds a %s record the inherited reader cannot read back:\n%s",
			codexItemFunctionOutput, installed)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
	if got := result.Verification.Structure.ReadBack.ToolResults; got != 0 {
		t.Errorf("read-back tool results = %d, want none: no result was written", got)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "does not hold every piece of evidence") {
			t.Errorf("the report warns that the installed session lacks written evidence: %q", warning)
		}
	}
}

// TestContinueOmitsDialogueTheInheritedReaderReadsAsNothing pins the same rule for dialogue text: the
// target's own reader reads a message whose text is blank after trimming as no message at all
// (internal/parser's handleResponseItem returns before appending one), so a whitespace-only dialogue
// written as a message would be in the file and absent from every read of it — the round-trip check
// would report the file as missing evidence it holds, and the run would fail on a source whose recorded
// text was only whitespace. The transcript is the native shape this really arrives in: a tool-result
// record that also carries a whitespace-only text block (the inherited parser's blank-message guard
// drops a blank record only while it carries no tool evidence), so the parser keeps that text as the
// record's dialogue. The text is omitted and named, and the tool evidence on the same record still
// travels.
func TestContinueOmitsDialogueTheInheritedReaderReadsAsNothing(t *testing.T) {
	cfg, _ := writeClaudeTranscript(t, claudeTranscriptShape{resultOutput: "PASS\n", blankDialogue: true})
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if loss := lossCount(t, result.Conversion.Losses, "whitespace_only_dialogue"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("whitespace_only_dialogue = %+v, want the blank dialogue counted as omitted", loss)
	}
	if result.Conversion.Carried.Messages != 1 {
		t.Errorf("carried = %+v, want only the conversation's own first turn written",
			result.Conversion.Carried)
	}
	// The record's tool evidence is not lost with the text it carried: the call and its output are
	// written, paired, and read back.
	if result.Conversion.Carried.ToolCalls != 1 || result.Conversion.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the call and its result written", result.Conversion.Carried)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
}

// TestContinueRoundTripsACompactionSummaryTheReaderTrims pins the last of the target reader's own
// normalisations on a conversion's evidence: it trims a compacted record's summary (internal/parser's
// handleCompacted), so a source summary that ends in whitespace is the same evidence to that read as
// one that does not. The comparison trims both sides, exactly as it does for a result's output, because
// comparing the untrimmed strings would report the reader's normalisation as a difference the file does
// not have — a false structural failure for a summary the provider wrote verbatim (the inherited Claude
// parser joins a compact summary's text blocks without trimming). The summary is carried as recorded;
// the file is not rewritten, and the check does not fail.
func TestContinueRoundTripsACompactionSummaryTheReaderTrims(t *testing.T) {
	const summary = "Summary of the work so far.\n"
	cfg, _ := writeClaudeTranscript(t, claudeTranscriptShape{
		resultOutput:   "PASS\n",
		compactSummary: summary,
	})
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Conversion.Carried.CompactionBoundaries != 1 {
		t.Fatalf("carried = %+v, want the compaction boundary written", result.Conversion.Carried)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed file to read back as what was written",
			result.Verification.Structure)
	}
	installed, err := os.ReadFile(result.Installation.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if !strings.Contains(string(installed), `Summary of the work so far.\n`) {
		t.Errorf("the installed session does not hold the summary exactly as the source recorded it:\n%s",
			installed)
	}
}

// TestCodexRoundTripFoldsAnEmptyArgumentObjectAsNoArguments pins the call-side normalisation of the
// round-trip comparison: the writer fills a call whose source recorded no input object with "{}" (the
// target's call record needs an arguments string, and the loss report says the object is an empty one),
// and the inherited reader drops an empty argument object while reading, so the two spellings are the
// same evidence to that read and the comparison folds them onto one key. Without the folding, a
// converted call with an empty recorded input would be reported as a difference the file does not have:
// the structural check would fail on a conversion that is exactly what the format holds.
func TestCodexRoundTripFoldsAnEmptyArgumentObjectAsNoArguments(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read"}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: "the read's output"}}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "empty_tool_inputs"); loss.Count != 1 {
		t.Fatalf("empty_tool_inputs = %+v, want the call with no recorded input counted", loss)
	}
	home := t.TempDir()
	if _, _, err := InstallCodexRollout(rollout, home); err != nil {
		t.Fatalf("installing: %v", err)
	}
	verification, err := VerifyCodexRolloutStructure(context.Background(),
		filepath.Join(home, codexSessionsSubdir), rollout, fixtureConfig(t))
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the empty-input call to read back as the call that was written",
			verification)
	}
}

// TestSourceCheckComparesTheFileBeforeAndAfter pins the property the installation reports: the source
// file's own bytes are the same before and after the conversion, and a file that changed while it was
// converted is reported as changed rather than assumed intact.
func TestSourceCheckComparesTheFileBeforeAndAfter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"user\"}\n"), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: path}}

	before, err := sourceFileDigest(conversation, "source")
	if err != nil {
		t.Fatalf("digesting the source: %v", err)
	}
	state, detail, err := sourceCheckAfter(conversation, "source", before)
	if err != nil {
		t.Fatalf("comparing the source: %v", err)
	}
	if state != SourceCheckUnchanged {
		t.Errorf("an untouched source was reported as %q: %s", state, detail)
	}
	if !strings.Contains(detail, before) {
		t.Errorf("the report does not name the digest it compared: %q", detail)
	}

	if err := os.WriteFile(path, []byte("{\"type\":\"user\",\"changed\":true}\n"), 0o600); err != nil {
		t.Fatalf("changing the source: %v", err)
	}
	state, detail, err = sourceCheckAfter(conversation, "source", before)
	if err != nil {
		t.Fatalf("comparing the changed source: %v", err)
	}
	if state != SourceCheckChanged {
		t.Fatalf("a source that changed during the conversion was reported as %q, want %q",
			state, SourceCheckChanged)
	}
	if !strings.Contains(detail, "changed while it was converted") {
		t.Errorf("the report does not say what happened: %q", detail)
	}
	// Both digests are in the report, so a caller can tell which two versions of the file it read: the
	// digest taken before the conversion, and the file's digest now.
	after, err := fileDigest(path)
	if err != nil {
		t.Fatalf("digesting the changed source: %v", err)
	}
	if after == before {
		t.Fatalf("the changed file digests the same as before (%s): the case's premise does not hold", after)
	}
	for _, want := range []string{before, after} {
		if !strings.Contains(detail, want) {
			t.Errorf("the changed report %q does not carry the digest %s", detail, want)
		}
	}
}

// TestContinueReportsASourceThatMovedDuringTheConversion pins the warning half of the same rule end to
// end: a source whose bytes change while it is converted -- the publication is the step between the
// digest taken before the conversion and the comparison after it -- is answered "changed" with both
// digests, and the report's own warning channel names the move. The conversion and the installation are
// real; the file simply no longer holds the bytes they read, and a caller that reads only the warnings
// must not miss it.
func TestContinueReportsASourceThatMovedDuringTheConversion(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg, sourcePath := sourceFixtureCopy(t, cfg,
		fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")),
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	before, err := fileDigest(sourcePath)
	if err != nil {
		t.Fatalf("digesting the source: %v", err)
	}
	moved := "{\"type\":\"user\",\"moved\":true}\n"
	link := linkSession
	linkSession = func(oldname, newname string) error {
		if err := link(oldname, newname); err != nil {
			return err
		}
		return os.WriteFile(sourcePath, []byte(moved), 0o600)
	}
	defer func() { linkSession = link }()

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing after the source moved: %v", err)
	}
	if result.Conversion.State != ConversionStateConverted ||
		result.Installation.State != InstallationStateInstalled {
		t.Errorf("conversion = %+v, installation = %+v, want the session carried and installed",
			result.Conversion, result.Installation)
	}
	if result.Installation.SourceCheckState != SourceCheckChanged {
		t.Fatalf("source_check_state = %q, want %q with the file's own bytes changed",
			result.Installation.SourceCheckState, SourceCheckChanged)
	}
	after, err := fileDigest(sourcePath)
	if err != nil {
		t.Fatalf("digesting the moved source: %v", err)
	}
	for _, want := range []string{before, after} {
		if !strings.Contains(result.Installation.Detail, want) {
			t.Errorf("the source check detail %q does not carry the digest %s",
				result.Installation.Detail, want)
		}
	}
	if warnings := strings.Join(result.Warnings, " "); !strings.Contains(warnings, "changed while it was converted") {
		t.Errorf("warnings = %v, want the report's own channel to name the source that moved", result.Warnings)
	}
}

// TestContinueFromAContainerMemberReportsNoFileToCompare pins the read whose identity is not a file: a
// member of a multi-session container is read through `parser.VirtualSourcePath`'s spelling
// (`<container>#<member>`), which the inherited provider records as the session's own path and this
// reader carries unchanged. The source check cannot digest that spelling -- it names a row, not a file,
// and the container's bytes hold every member the container carries -- so it must answer not-verified
// with the member it was about instead of stopping a conversion whose source was read (review round 11's
// finding 1). The container's own bytes are compared here by hand, because the run reports no comparison
// of them: the property the report would otherwise claim is the caller's to check for this shape.
func TestContinueFromAContainerMemberReportsNoFileToCompare(t *testing.T) {
	ctx := context.Background()
	root, containerPath := openClawContainer(t, map[string][]string{
		"good": openClawContainerEvents("good"),
	})
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentOpenClaw: {root}}
	before, err := fileDigest(containerPath)
	if err != nil {
		t.Fatalf("digesting the container: %v", err)
	}

	// The premise: the member's own path is the virtual identity, not a file the check could digest.
	ref := Ref{Provider: "openclaw", ID: "openclaw:main:good"}
	conversation, err := NewSourceReader(cfg).Read(ctx, ref)
	if err != nil {
		t.Fatalf("reading the container member: %v", err)
	}
	if want := containerPath + "#main:good"; conversation.Source.SourcePath != want {
		t.Fatalf("the member's own path = %q, want the container's member identity %q, which is what "+
			"this case is about", conversation.Source.SourcePath, want)
	}

	result, err := Continue(ctx, ContinueOptions{
		Ref:          ref,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing the container member: %v", err)
	}
	if result.Conversion.State != ConversionStateConverted ||
		result.Installation.State != InstallationStateInstalled {
		t.Errorf("conversion = %+v, installation = %+v, want the session carried and installed",
			result.Conversion, result.Installation)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the converted session to read back", result.Verification.Structure)
	}
	if result.Installation.SourceCheckState != SourceCheckNotVerified {
		t.Errorf("source_check_state = %q, want %q: no file holds the member's own bytes",
			result.Installation.SourceCheckState, SourceCheckNotVerified)
	}
	for _, want := range []string{"main:good", containerPath} {
		if !strings.Contains(result.Installation.Detail, want) {
			t.Errorf("the source check detail %q does not name %q", result.Installation.Detail, want)
		}
	}
	if warnings := sourceChangedWarnings(result.Installation.SourceCheckState); len(warnings) != 0 {
		t.Errorf("a comparison that did not run warned about a moved source: %v", warnings)
	}
	after, err := fileDigest(containerPath)
	if err != nil {
		t.Fatalf("digesting the container after the conversion: %v", err)
	}
	if after != before {
		t.Errorf("the container's bytes changed while it was converted (sha256 %s before, %s after)",
			before, after)
	}
}

// TestSourceCheckStopsAReadThatNamesNoFileAtAll pins the line between the two answers a read that names
// no file can earn: an identity that names a *member* (a row inside a container) is a fact about the
// source, and the check answers not-verified with it; a read that reported no path at all is a read the
// report cannot describe, and the caller stops the run rather than reporting a state of a file that was
// never named. sourceFileOf answers the two apart, and each half is pinned by the test beside this one.
func TestSourceCheckStopsAReadThatNamesNoFileAtAll(t *testing.T) {
	conversation := Conversation{Source: SourceIdentity{Provider: "claude"}}
	digest, err := sourceFileDigest(conversation, "source")
	if err != nil || digest != "" {
		t.Fatalf("digest = %q, err = %v; a read that names no path has no file to digest", digest, err)
	}
	if _, _, err := sourceCheckAfter(conversation, "source", ""); err == nil ||
		!strings.Contains(err.Error(), "no path") {
		t.Fatalf("err = %v, want a stopped run naming the read that reported no path", err)
	}
}

// TestSourceCheckStopsAComparisonWhoseDigestWasNeverTaken pins the third stop condition: with a file to
// compare and no digest taken before the conversion, the comparison cannot run, and the run stops naming
// the missing digest rather than answering a state nothing observed -- "unchanged" would claim a
// comparison the run never made, and "not-verified" is for a comparison the source's own shape kept from
// running (an archive read, a member identity), not for a caller that handed over no digest to compare
// against.
func TestSourceCheckStopsAComparisonWhoseDigestWasNeverTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"user\"}\n"), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: path}}
	state, _, err := sourceCheckAfter(conversation, "source", "")
	if err == nil || !strings.Contains(err.Error(), "was not taken") {
		t.Fatalf("state = %q, err = %v; want a stopped run naming the digest that was never taken",
			state, err)
	}
}

// TestSourceCheckSaysWhenThereWasNoFileToCompare pins the archive read's answer: a projection has no file
// of its own, so the report says the comparison did not run rather than claiming one -- "unchanged" would
// be a claim about a file that was never open.
func TestSourceCheckSaysWhenThereWasNoFileToCompare(t *testing.T) {
	conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: "/absent/file.jsonl"}}
	digest, err := sourceFileDigest(conversation, "archive")
	if err != nil || digest != "" {
		t.Fatalf("digest = %q, err = %v; an archive read has no source file to digest", digest, err)
	}
	state, detail, err := sourceCheckAfter(conversation, "archive", "")
	if err != nil || state != SourceCheckNotVerified {
		t.Fatalf("state = %q, err = %v; want a comparison that did not run", state, err)
	}
	if !strings.Contains(detail, "archive") {
		t.Errorf("the report does not say why no file was compared: %q", detail)
	}
}

// TestSourceChangedWarningsNameTheMove pins the report's warning half: a source that moved while it was
// converted is warned about, not only described in a detail field a reader may never open -- and a
// comparison that did not run is not warned about as a move, because nothing observed one.
func TestSourceChangedWarningsNameTheMove(t *testing.T) {
	for _, state := range []string{SourceCheckUnchanged, SourceCheckNotVerified} {
		if warnings := sourceChangedWarnings(state); len(warnings) != 0 {
			t.Errorf("a %q source check produced warnings: %v", state, warnings)
		}
	}
	warnings := sourceChangedWarnings(SourceCheckChanged)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "changed while it was converted") {
		t.Errorf("warnings = %v, want one naming the source that moved", warnings)
	}
}

// stoppedRunChecks pins what a report says about the checks a stopped run never reached: the checks the
// run's level asked for are not-verified and name the failure, and a check nobody asked for stays
// not-requested -- "could not run" and "nobody asked" stay different answers.
func stoppedRunChecks(t *testing.T, result ContinueResult) {
	t.Helper()
	const reason = "the run stopped before this check ran"
	for _, block := range []struct{ name, state, detail string }{
		{"structure", result.Verification.Structure.State, result.Verification.Structure.Detail},
		{"load", result.Verification.Load.State, result.Verification.Load.Detail},
	} {
		if block.state != VerificationStateNotVerified {
			t.Errorf("%s = %q, want %q", block.name, block.state, VerificationStateNotVerified)
		}
		if !strings.Contains(block.detail, reason) {
			t.Errorf("%s detail = %q, want it to name the stopped run", block.name, block.detail)
		}
	}
	if result.Verification.Resume.State != VerificationStateNotRequested {
		t.Errorf("resume = %q, want %q: the default level asks for the load check only",
			result.Verification.Resume.State, VerificationStateNotRequested)
	}
	// The report's own channel names it as well, so a caller that reads only the warnings does not have
	// to infer from a state that the run did not finish.
	if !strings.Contains(strings.Join(result.Warnings, " "), "the run stopped after the conversion") {
		t.Errorf("warnings = %v, want one naming the stopped run", result.Warnings)
	}
}

// TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall is the post-publication failure the
// installation has to survive: the session is installed, and the comparison of the source file's own
// bytes cannot run because the file is gone. The caller still gets the conversion, the installation and
// the identity the target harness resumes by; what it must not get is a report thrown away, which would
// leave an installed session with no identity reported for it.
func TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg, sourcePath := sourceFixtureCopy(t, cfg,
		fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")),
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

	// The source disappears exactly at publication: the exclusive link is the step before the report's
	// comparison of the source file, so a link that also removes the source is the failure that
	// comparison has to answer rather than replace.
	link := linkSession
	linkSession = func(oldname, newname string) error {
		if err := link(oldname, newname); err != nil {
			return err
		}
		return os.Remove(sourcePath)
	}
	defer func() { linkSession = link }()

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err == nil {
		t.Fatal("a run whose source vanished after the installation reported success")
	}
	if !strings.Contains(err.Error(), "re-reading the source session") {
		t.Errorf("the error does not name the comparison that could not run: %v", err)
	}

	if result.Conversion.State != ConversionStateConverted || result.Conversion.Carried.Messages == 0 {
		t.Errorf("conversion = %+v, want the conversion the run completed, with its counts",
			result.Conversion)
	}
	if result.Installation.State != InstallationStateInstalled || result.Installation.Path == "" ||
		!result.Installation.Created {
		t.Fatalf("installation = %+v, want the published session reported", result.Installation)
	}
	if result.TargetSession.SessionID == "" || result.TargetSession.Path != result.Installation.Path {
		t.Errorf("target session = %+v, want the identity of the session that is installed",
			result.TargetSession)
	}
	if _, err := os.Stat(result.Installation.Path); err != nil {
		t.Errorf("the report names an installed session that is not there: %v", err)
	}
	// The comparison did not run, so the report says that; it must not read as "the file moved".
	if result.Installation.SourceCheckState != SourceCheckNotVerified {
		t.Errorf("installation = %+v, want the source check reported as %q",
			result.Installation, SourceCheckNotVerified)
	}
	if !strings.Contains(result.Installation.Detail, "re-reading the source session") {
		t.Errorf("the source check's detail does not name what stopped it: %q", result.Installation.Detail)
	}
	if warnings := strings.Join(result.Warnings, " "); strings.Contains(warnings, "changed while it was converted") {
		t.Errorf("warnings claim the source moved, which nothing observed: %v", result.Warnings)
	}
	stoppedRunChecks(t, result)
}

// TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved pins the installer's own answer when it
// publishes the session and then cannot remove the temporary file it wrote through: the path and the
// failure are both returned, and the caller keeps the identity of a session that really is there rather
// than trading the publication for the cleanup.
func TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved(t *testing.T) {
	cfg := fixtureConfig(t)
	remove := removeFile
	removeFile = func(string) error { return errors.New("remove: the file is in use") }
	defer func() { removeFile = remove }()

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err == nil {
		t.Fatal("a run whose temporary file could not be removed reported success")
	}
	if !strings.Contains(err.Error(), "removing the temporary file") {
		t.Errorf("the error does not name the cleanup that failed: %v", err)
	}
	if result.Installation.State != InstallationStateInstalled || result.Installation.Path == "" {
		t.Fatalf("installation = %+v, want the published session reported beside the failure",
			result.Installation)
	}
	if result.TargetSession.SessionID == "" || result.TargetSession.Path != result.Installation.Path {
		t.Errorf("target session = %+v, want the identity of the session that is installed",
			result.TargetSession)
	}
	data, readErr := os.ReadFile(result.Installation.Path)
	if readErr != nil {
		t.Fatalf("the published session is not readable: %v", readErr)
	}
	if len(data) != result.Conversion.Carried.Bytes {
		t.Errorf("the published session holds %d bytes, want the conversion's %d",
			len(data), result.Conversion.Carried.Bytes)
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "removing the temporary file") {
		t.Errorf("warnings = %v, want one naming the temporary file left behind", result.Warnings)
	}
	stoppedRunChecks(t, result)
}

// TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated pins the same rule one step earlier: a
// failure that stops the run before anything is installed still hands back the conversion's own report --
// its state, its counts, its losses -- with the installation saying it is not installed and the checks
// saying why they did not run.
func TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated(t *testing.T) {
	cfg := fixtureConfig(t)
	// A file where the home's parent chain should be: nothing below it can be created.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   filepath.Join(blocker, "home"),
		TargetBinary: missingTargetBinary,
	})
	if err == nil {
		t.Fatal("a run that could not create its target home reported success")
	}
	if result.Conversion.State != ConversionStateConverted || result.Conversion.Carried.Messages == 0 {
		t.Errorf("conversion = %+v, want the conversion the run completed, with its counts",
			result.Conversion)
	}
	if result.Installation.State != InstallationStateNotInstalled || result.Installation.Path != "" {
		t.Errorf("installation = %+v, want not-installed", result.Installation)
	}
	if result.Installation.SourceCheckState != SourceCheckNotVerified ||
		!strings.Contains(result.Installation.Detail, "the run stopped before this check ran") {
		t.Errorf("installation = %+v, want the source check reported as not run, naming what stopped it",
			result.Installation)
	}
	stoppedRunChecks(t, result)
}

// TestContinueKeepsTheDryRunReportWhenItsSourceCheckCannotRun pins the same rule for the dry run: it
// installs nothing, so there is no identity to lose, but the conversion it produced is still what the
// caller asked for, and a failure after it comes back beside that report rather than instead of it. The
// structural read-back runs before the source comparison and has already answered, so the report keeps
// its verdict too: a failure later in the run must not downgrade a check that did verify.
func TestContinueKeepsTheDryRunReportWhenItsSourceCheckCannotRun(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg, sourcePath := sourceFixtureCopy(t, cfg,
		fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")),
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

	link := linkSession
	linkSession = func(oldname, newname string) error {
		if err := link(oldname, newname); err != nil {
			return err
		}
		return os.Remove(sourcePath)
	}
	defer func() { linkSession = link }()

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		Reader:       NewSourceReader(cfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		DryRun:       true,
	})
	if err == nil {
		t.Fatal("a dry run whose source vanished after its throw-away copy reported success")
	}
	if result.Conversion.State != ConversionStateConverted || !result.Conversion.DryRun {
		t.Errorf("conversion = %+v, want the dry run's conversion reported", result.Conversion)
	}
	if result.Installation.State != InstallationStateNotInstalled {
		t.Errorf("installation = %+v, want not-installed: a dry run writes nothing", result.Installation)
	}
	if result.Installation.SourceCheckState != SourceCheckNotVerified ||
		!strings.Contains(result.Installation.Detail, "the source") {
		t.Errorf("installation = %+v, want the source check reported as not run", result.Installation)
	}
	// The check that did run: the throw-away copy was read back before the comparison failed.
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the read-back the dry run completed to keep its verdict",
			result.Verification.Structure)
	}
	if result.Verification.Load.State != VerificationStateNotVerified ||
		!strings.Contains(result.Verification.Load.Detail, "a dry run installs nothing") {
		t.Errorf("load = %+v, want the check the dry run could not run, with its own reason",
			result.Verification.Load)
	}
	if result.Verification.Resume.State != VerificationStateNotRequested {
		t.Errorf("resume = %q, want %q: the default level asks for the load check only",
			result.Verification.Resume.State, VerificationStateNotRequested)
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "the run stopped after the conversion") {
		t.Errorf("warnings = %v, want one naming the stopped run", result.Warnings)
	}
}

// TestContinueDisclosesASummaryTranscriptRead pins the continuation's own disclosure of the provider's
// classification of the read. The conversion's fidelity block carries the classification as a field, and
// a caller that reads only the report's warnings and its loss list has to be told the same thing: a
// summary fallback is a degraded read standing in for a transcript the provider could not produce, not
// the session's recorded transcript, and a run that converted, installed and verified says nothing about
// that by itself.
//
// The fixture is the finding's own shape: a Grok session whose chat history is missing reads as one
// synthesized fallback message (internal/parser's summary classification). The control is a transcript
// read of the same provider family's route: no summary warning, and the category is present with a zero
// count, so a reader can tell "not a summary" from "the report does not say".
func TestContinueDisclosesASummaryTranscriptRead(t *testing.T) {
	ctx := context.Background()
	summaryCfg, _, summaryRef := grokSummarySession(t)
	summaryResult, err := Continue(ctx, ContinueOptions{
		Ref:          summaryRef,
		Target:       TargetCodex,
		Config:       summaryCfg,
		Reader:       NewSourceReader(summaryCfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing the summary-only session: %v", err)
	}
	if got := summaryResult.Conversion.SourceFidelity.TranscriptFidelity; got != parser.TranscriptFidelitySummary {
		t.Errorf("source fidelity = %q, want %q", got, parser.TranscriptFidelitySummary)
	}
	if warnings := strings.Join(summaryResult.Warnings, "\n"); !strings.Contains(warnings, "summary transcript") {
		t.Errorf("the classification is in the fidelity block but not in the report's warnings: %v",
			summaryResult.Warnings)
	}
	loss := lossCount(t, summaryResult.Conversion.Losses, "summary_transcript")
	if loss.Count != 1 || loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("summary_transcript = %+v, want the read's classification counted once, carried with a caveat",
			loss)
	}
	if !strings.Contains(loss.Detail, "summary") || !strings.Contains(loss.Detail, "recorded transcript") {
		t.Errorf("summary_transcript detail = %q, want it to say what the read is not", loss.Detail)
	}

	// The control: a read from a transcript, which the provider classifies as full.
	fullCfg := fixtureConfig(t)
	fullResult, err := Continue(ctx, ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       fullCfg,
		Reader:       NewSourceReader(fullCfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing the transcript fixture: %v", err)
	}
	if warnings := strings.Join(fullResult.Warnings, "\n"); strings.Contains(warnings, "summary") {
		t.Errorf("a transcript read is disclosed as a summary: %v", fullResult.Warnings)
	}
	if loss := lossCount(t, fullResult.Conversion.Losses, "summary_transcript"); loss.Count != 0 ||
		strings.TrimSpace(loss.Detail) == "" {
		t.Errorf("summary_transcript = %+v, want a zero count that still says what the category is", loss)
	}
}

// TestContinueDisclosesARetryableRead pins the other classification the provider can return beside a
// usable conversation: a result that is not current for its own data version. A Codex fork whose copied
// prefix comes from a page whose parent head is unreadable parses to the copied parent messages *and*
// that state (internal/parser's DataVersionNeedsRetry), so the conversion carries messages whose session
// boundary is unresolved -- which the report has to say in its warnings and in its loss list, not only
// in the fidelity block.
//
// The control is the same reference once the parent's head is readable: no retry warning, and the
// category is present with a zero count.
func TestContinueDisclosesARetryableRead(t *testing.T) {
	ctx := context.Background()
	forkCfg, _ := codexForkRollouts(t, false)
	forkResult, err := Continue(ctx, ContinueOptions{
		Ref:          Ref{Provider: "codex", ID: codexForkID},
		Target:       TargetCodex,
		Config:       forkCfg,
		Reader:       NewSourceReader(forkCfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing the fork whose parent head is unreadable: %v", err)
	}
	if !forkResult.Conversion.SourceFidelity.NeedsRetry {
		t.Errorf("source fidelity = %+v, want the provider's own retry state",
			forkResult.Conversion.SourceFidelity)
	}
	warnings := strings.Join(forkResult.Warnings, "\n")
	if !strings.Contains(warnings, "needing a retry") || !strings.Contains(warnings, "forked from") {
		t.Errorf("the retry and the provider's own reason are not in the warnings: %v", forkResult.Warnings)
	}
	loss := lossCount(t, forkResult.Conversion.Losses, "source_needs_retry")
	if loss.Count != 1 || loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("source_needs_retry = %+v, want the read's state counted once, carried with a caveat", loss)
	}
	if !strings.Contains(loss.Detail, "retry") || !strings.Contains(loss.Detail, "forked from") {
		t.Errorf("source_needs_retry detail = %q, want the provider's own reason in it", loss.Detail)
	}

	// The control: the same reference once the parent's head is readable.
	resolvedCfg, _ := codexForkRollouts(t, true)
	resolvedResult, err := Continue(ctx, ContinueOptions{
		Ref:          Ref{Provider: "codex", ID: codexForkID},
		Target:       TargetCodex,
		Config:       resolvedCfg,
		Reader:       NewSourceReader(resolvedCfg),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing the resolved fork: %v", err)
	}
	if warnings := strings.Join(resolvedResult.Warnings, "\n"); strings.Contains(warnings, "needing a retry") {
		t.Errorf("a resolved read is disclosed as needing a retry: %v", resolvedResult.Warnings)
	}
	if loss := lossCount(t, resolvedResult.Conversion.Losses, "source_needs_retry"); loss.Count != 0 ||
		strings.TrimSpace(loss.Detail) == "" {
		t.Errorf("source_needs_retry = %+v, want a zero count that still says what the category is", loss)
	}
}
