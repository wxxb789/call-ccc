package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// piTargetHome prepares an isolated Pi data directory holding one unrelated session, and answers the
// home and that session's path: a conversion must leave it alone. The home holds a space, so a rendered
// resume line has to quote the data directory on every host.
func piTargetHome(t *testing.T) (string, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "pi home")
	unrelated := filepath.Join(home, piSessionsSubdir, "--home-user-other-project",
		"2026-01-02T00-00-00-000Z_00000000-0000-4000-8000-0000000000ff.jsonl")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatalf("creating the unrelated session's directory: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte(
		`{"type":"session","version":3,"id":"00000000-0000-4000-8000-0000000000ff","timestamp":"2026-01-02T00:00:00.000Z","cwd":"/home/user/other-project"}`+"\n"),
		0o600); err != nil {
		t.Fatalf("writing the unrelated session: %v", err)
	}
	return home, unrelated
}

// TestContinueIntoPiConvertsInstallsAndReadsBack is the route's central property test: one call turns a
// real Claude Code session into a new Pi session, installs it under the project directory Pi's own
// discovery derives from the session's working directory, leaves the source and an unrelated session
// untouched, and reads the installed file back through the inherited Pi reader as what the writer placed
// in it.
func TestContinueIntoPiConvertsInstallsAndReadsBack(t *testing.T) {
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)
	home, unrelated := piTargetHome(t)
	unrelatedBefore := hashFile(t, unrelated)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetPi,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}

	if result.Target != TargetPi {
		t.Errorf("target = %q, want %q", result.Target, TargetPi)
	}
	if result.Conversion.State != ConversionStateConverted || result.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", result.Conversion)
	}
	if result.Conversion.Format != PiTargetFormat {
		t.Errorf("format = %q, want %q", result.Conversion.Format, PiTargetFormat)
	}
	if result.Conversion.Carried.Messages == 0 {
		t.Error("the conversion carried no message")
	}
	if result.Installation.State != InstallationStateInstalled {
		t.Fatalf("installation = %+v, want the session installed", result.Installation)
	}
	wantDir := filepath.Join(home, piSessionsSubdir, PiProjectDir("/home/user/code/example-api"))
	if filepath.Dir(result.TargetSession.Path) != wantDir {
		t.Errorf("installed under %s, want the project directory the working directory spells to (%s)",
			filepath.Dir(result.TargetSession.Path), wantDir)
	}
	if filepath.Base(result.TargetSession.Path) != result.TargetSession.FileName {
		t.Errorf("installed file %s does not carry the reported name %s",
			result.TargetSession.Path, result.TargetSession.FileName)
	}
	if !strings.HasSuffix(result.TargetSession.FileName, "_"+result.TargetSession.SessionID+".jsonl") {
		t.Errorf("file name = %q, want <timestamp>_<session id>.jsonl", result.TargetSession.FileName)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file read back as what was written",
			result.Verification.Structure)
	}
	if result.Verification.Structure.ReadBack.ToolCalls != result.Conversion.Carried.ToolCalls {
		t.Errorf("read-back tool calls = %d, want the %d the conversion carried",
			result.Verification.Structure.ReadBack.ToolCalls, result.Conversion.Carried.ToolCalls)
	}
	if result.Verification.Structure.ReadBack.ToolResults != result.Conversion.Carried.ToolResults {
		t.Errorf("read-back tool results = %d, want the %d the conversion carried",
			result.Verification.Structure.ReadBack.ToolResults, result.Conversion.Carried.ToolResults)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q, want the source unchanged", result.Installation.SourceCheckState)
	}
	if hashFile(t, sourcePath) != sourceBefore {
		t.Error("the conversion changed the source session")
	}
	if hashFile(t, unrelated) != unrelatedBefore {
		t.Error("the conversion changed an unrelated session in the target home")
	}
	// The carried byte count is the file's own size, which is what the report's "N bytes" line states.
	// The count is a convenience for a human reading it, so it has to be computed from the bytes rather
	// than from a second length over the same buffer.
	info, err := os.Stat(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("measuring the installed session: %v", err)
	}
	if int64(result.Conversion.Carried.Bytes) != info.Size() {
		t.Errorf("carried bytes = %d, want the installed file's own size (%d)",
			result.Conversion.Carried.Bytes, info.Size())
	}

	// The installed file is read back once more, through the inherited Pi reader over the file the
	// conversion published: the check compares the file's own bytes with what the writer placed in it, so
	// a conversion whose file disagrees with its own report is caught here.
	// The comparison is rerun over the same document the route converted, so the check's own verdict in
	// the report is not the only evidence that the installed file holds what was written: this is the
	// same walk, driven from the document rather than from the report's summary of it.
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the source fixture: %v", err)
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{SessionID: result.TargetSession.SessionID})
	if err != nil {
		t.Fatalf("converting the same document again: %v", err)
	}
	readBack, err := VerifyPiSessionStructure(context.Background(),
		filepath.Join(home, piSessionsSubdir), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if readBack.State != VerificationStateVerified {
		t.Errorf("read-back = %+v, want the installed file read as the document's own evidence", readBack)
	}
}

// TestContinueIntoPiInvocationNamesTheHomeAndTheWorkingDirectory pins the printed invocation: Pi is told
// which data directory to read by one environment variable and resolves a session id through the project
// directory of the process's own working directory, so a resume line that names neither would open the
// caller's own home and look for a session of another project.
func TestContinueIntoPiInvocationNamesTheHomeAndTheWorkingDirectory(t *testing.T) {
	home, _ := piTargetHome(t)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetPi,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if got := result.TargetSession.ResumeEnvironment[piHomeEnv]; got != home {
		t.Errorf("resume environment %s = %q, want the home the session was installed into (%q)",
			piHomeEnv, got, home)
	}
	if len(result.TargetSession.ResumeArguments) < 3 || result.TargetSession.ResumeArguments[1] != "--session" ||
		result.TargetSession.ResumeArguments[2] != result.TargetSession.SessionID {
		t.Errorf("resume arguments = %v, want --session <id>", result.TargetSession.ResumeArguments)
	}
	// The rendered line changes into the session's working directory, sets the data directory (with its
	// space preserved as one word), and names the session. The assertions are on the words a POSIX shell
	// reads rather than on the exact spacing, because the renderer quotes what a shell would act on.
	for _, command := range []string{result.TargetSession.ResumeCommand, result.TargetSession.ExecResumeCommand} {
		if !strings.HasPrefix(command, "cd "+shellQuote("/home/user/code/example-api")+" && ") {
			t.Errorf("rendered command = %q, want it to change into the session's working directory", command)
		}
		if !strings.Contains(command, piHomeEnv+"="+shellQuote(home)) {
			t.Errorf("rendered command = %q, want the data directory named so a shell reads it as one word",
				command)
		}
		if !strings.Contains(command, "--session "+result.TargetSession.SessionID) {
			t.Errorf("rendered command = %q, want the session named by id", command)
		}
	}
	if !strings.Contains(result.TargetSession.ExecResumeCommand, promptTerminator+" "+shellQuote(resumePromptPlaceholder)) {
		t.Errorf("exec resume command = %q, want the prompt placeholder behind the terminator",
			result.TargetSession.ExecResumeCommand)
	}
}

// TestContinueIntoPiDryRunWritesNothing pins the dry run: the conversion is verified against a throw-away
// copy of its own bytes and the target home is not created.
func TestContinueIntoPiDryRunWritesNothing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "pi home")
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetPi,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if result.Conversion.State != ConversionStateConverted || !result.Conversion.DryRun {
		t.Errorf("conversion = %+v, want a dry-run conversion", result.Conversion)
	}
	if result.Installation.State != InstallationStateNotInstalled {
		t.Errorf("installation = %+v, want nothing installed", result.Installation)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the dry run's own copy verified", result.Verification.Structure)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the dry run created the target home %s (%v)", home, err)
	}
}

// TestContinueIntoPiRefusesASourceWithoutAWorkingDirectory pins the refusal through the pipeline: Pi's
// lookup is project-scoped, so a session installed without a working directory could never be resumed
// while a report called it installed. Nothing is written into the target home.
func TestContinueIntoPiRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	sourceRoot := t.TempDir()
	const sessionID = "01a1c335-0000-4000-8000-00000000dead"
	path := filepath.Join(sourceRoot, sessionID+".jsonl")
	lines := []string{
		`{"type":"session","version":3,"id":"` + sessionID + `","timestamp":"2026-05-04T09:15:00.000Z"}`,
		`{"type":"message","id":"52fd5d0f","parentId":null,"timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"user","content":"hello","timestamp":1777886101000}}`,
		`{"type":"message","id":"b3c4d5e6","parentId":"52fd5d0f","timestamp":"2026-05-04T09:15:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0},"stopReason":"stop","timestamp":1777886102000}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the source session: %v", err)
	}
	cfg := fixtureConfig(t)
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		dirs[agent] = roots
	}
	dirs[parser.AgentPi] = []string{sourceRoot}
	cfg.AgentDirs = dirs

	home, _ := piTargetHome(t)
	before := hashTree(t, home)
	_, err := Continue(context.Background(), ContinueOptions{
		Ref: Ref{Provider: "pi", ID: sessionID}, Target: TargetPi, Config: cfg,
		TargetHome: home, TargetBinary: missingTargetBinary,
	})
	if !errors.Is(err, ErrNoTargetCwd) {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
	if after := hashTree(t, home); !sameTree(before, after) {
		t.Errorf("the refused conversion wrote into the target home: %v -> %v", before, after)
	}
}

// TestContinueFromPiThroughTheCommonModel pins the source direction: a Pi session continues in a
// previously supported target harness through the same route table and the same reader every other
// source uses, with no Pi-specific branch in the pipeline. The conversion's own report names the
// inherited Pi reader as what read the source.
func TestContinueFromPiThroughTheCommonModel(t *testing.T) {
	for _, target := range []string{TargetClaude, TargetCodex} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          piFixtureRef,
				Target:       target,
				Config:       fixtureConfig(t),
				TargetHome:   home,
				TargetBinary: missingTargetBinary,
			})
			if err != nil {
				t.Fatalf("continuing pi:%s into %s: %v", piFixtureRef.ID, target, err)
			}
			if result.Source == nil {
				t.Fatal("the result carries no source report")
			}
			if result.Source.Identity.Provider != "pi" {
				t.Errorf("source provider = %q, want the reference's own provider",
					result.Source.Identity.Provider)
			}
			if result.Source.Identity.SourcePath == "" {
				t.Error("the source report names no file: the read came through the inherited Pi reader")
			}
			if result.Conversion.State != ConversionStateConverted {
				t.Fatalf("conversion = %+v, want the conversation converted", result.Conversion)
			}
			if result.Conversion.Carried.ToolCalls == 0 || result.Conversion.Carried.ToolResults == 0 {
				t.Errorf("carried = %+v, want the fixture's call and result", result.Conversion.Carried)
			}
			// The installed target document is read back by the *target's* own reader: the common model
			// carried the call and its result across, which is what a route-specific shortcut would skip.
			if result.Verification.Structure.State != VerificationStateVerified {
				t.Fatalf("structure = %+v, want the installed session read back as written",
					result.Verification.Structure)
			}
			if result.Verification.Structure.ReadBack.ToolCalls != result.Conversion.Carried.ToolCalls {
				t.Errorf("read-back calls = %d, want the %d carried",
					result.Verification.Structure.ReadBack.ToolCalls, result.Conversion.Carried.ToolCalls)
			}
		})
	}
}

// TestContinueFromPiKeepsACompactedAndBranchedHistoryCoherent pins the goal's own clause for the shape
// that is easiest to get wrong: a Pi session compacted at a boundary, whose file also holds a branch the
// conversation left. The conversion carries the boundary as the target's own boundary record (so the
// target rebuilds its context from the summary, as the source's own compaction would), keeps the
// pre-boundary turns in the file as recorded history, reports how much of the conversation the boundary
// stands in front of, and never invents the branch as another active turn.
func TestContinueFromPiKeepsACompactedAndBranchedHistoryCoherent(t *testing.T) {
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piHistoryFixtureRef)
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	boundaries := 0
	for _, message := range conversation.Messages {
		if message.IsCompactBoundary {
			boundaries++
			if strings.TrimSpace(message.Content) == "" {
				t.Error("the boundary carries no summary text")
			}
		}
	}
	if boundaries != 1 {
		t.Fatalf("the source read reports %d boundary(ies), want the fixture's one", boundaries)
	}

	home := t.TempDir()
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          piHistoryFixtureRef,
		Target:       TargetClaude,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing the history fixture: %v", err)
	}
	if result.Conversion.Carried.CompactionBoundaries != 1 {
		t.Errorf("carried boundaries = %d, want the source's one",
			result.Conversion.Carried.CompactionBoundaries)
	}
	preBoundary := lossCount(t, result.Conversion.Losses, "pre_compaction_history")
	if preBoundary.Count == 0 {
		t.Error("no pre_compaction_history count: the turns the boundary stands in front of are unaccounted for")
	}
	if result.Verification.Structure.ReadBack.CompactionBoundaries != 1 {
		t.Errorf("read-back boundaries = %d, want the boundary the conversion wrote",
			result.Verification.Structure.ReadBack.CompactionBoundaries)
	}
}

// TestPiHistoryFixtureBranchIsNotOnTheActivePath reads the committed fixture's own tree and pins the
// property the goal names: the branch the conversation left hangs off an entry the active path does not
// continue from, so a conversion cannot present it as an active turn by following the file's own parent
// links. The walk is over the fixture's bytes -- the shape a real Pi file has -- rather than over any
// product of this build.
func TestPiHistoryFixtureBranchIsNotOnTheActivePath(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	type entry struct {
		Type     string  `json:"type"`
		ID       string  `json:"id"`
		ParentID *string `json:"parentId"`
		Summary  string  `json:"summary"`
	}
	var entries []entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded := entry{}
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decoding a fixture line: %v", err)
		}
		if decoded.Type == "session" {
			continue
		}
		entries = append(entries, decoded)
	}
	byID := map[string]entry{}
	for _, item := range entries {
		byID[item.ID] = item
	}
	if len(entries) == 0 {
		t.Fatal("the fixture holds no entry")
	}
	// The active path is what the harness's own loader walks: from the file's last entry back through
	// parentId. The branch's own turn is in the file and is not on it.
	onPath := map[string]bool{}
	cursor := entries[len(entries)-1].ID
	for cursor != "" {
		onPath[cursor] = true
		next := byID[cursor]
		if next.ParentID == nil {
			break
		}
		cursor = *next.ParentID
	}
	if !onPath[entries[len(entries)-1].ID] {
		t.Fatal("the last entry is not on its own path")
	}
	offPath := []string{}
	for _, item := range entries {
		if !onPath[item.ID] {
			offPath = append(offPath, item.ID)
		}
	}
	if len(offPath) == 0 {
		t.Fatal("every entry is on the active path: the fixture holds no branch the conversation left")
	}
	// The branch hangs off an entry the active path does not continue from, which is what makes it a
	// branch rather than another link in one line.
	branchParent := byID[offPath[0]].ParentID
	if branchParent == nil || !onPath[*branchParent] {
		t.Fatalf("the off-path entry's parent %v is not an entry on the active path", branchParent)
	}
}

// TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere pins the load check's own boundary: the
// target resolves a session id through the process's own working directory, so a session whose recorded
// directory does not exist on this machine cannot be loaded here -- and the check says that instead of
// reading the target's fork offer as a pass.
func TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere(t *testing.T) {
	if _, err := exec.LookPath(PiBinaryDefault); err != nil {
		t.Skip("pi is not installed on this host")
	}
	home := t.TempDir()
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetPi,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	// The Claude fixture records /home/user/code/example-api, which this host does not have.
	if _, err := os.Stat(result.TargetSession.Cwd); err == nil {
		t.Skipf("this host has %s, so the refusal cannot be exercised here", result.TargetSession.Cwd)
	}
	load := VerifyPiLoad(context.Background(), PiLoadOptions{
		Home:      home,
		SessionID: result.TargetSession.SessionID,
		Cwd:       result.TargetSession.Cwd,
		Directory: PiProjectDir(result.TargetSession.Cwd),
		FileName:  result.TargetSession.FileName,
		Data:      []byte{},
		Installed: result.TargetSession.Path,
		Stderr:    os.Stderr,
	})
	if load.State != VerificationStateNotVerified {
		t.Fatalf("load = %+v, want not-verified for a working directory this host does not have", load)
	}
	if !strings.Contains(load.Detail, "working directory") {
		t.Errorf("load detail = %q, want it to name the working directory the harness needs", load.Detail)
	}
}
