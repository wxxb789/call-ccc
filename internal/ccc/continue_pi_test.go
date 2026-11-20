package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/config"
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
	if result.Verification.Structure.ReadBack.ToolCalls != 1 {
		t.Errorf("read-back tool calls = %d, want the fixture's own 1",
			result.Verification.Structure.ReadBack.ToolCalls)
	}
	if result.Verification.Structure.ReadBack.ToolResults != 1 {
		t.Errorf("read-back tool results = %d, want the fixture's own 1",
			result.Verification.Structure.ReadBack.ToolResults)
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

// TestContinueIntoPiPrintsTheInvocationItVerifies pins the printed command against the check that runs
// it, word by word and variable by variable. The exec form must be print mode: measured on 1.5.2, `pi
// --session <id> -- <prompt>` with terminal stdin and stdout opens the TUI and runs no turn, while
// VerifyPiResume independently passes `-p` -- so without this a pasted "non-interactive" line would never
// do what the report says. Both lines must also name the sessions tree the check runs with
// (PI_CODING_AGENT_SESSION_DIR, the project directory the installer filed the session under), because Pi
// reads that variable ahead of the agent directory's own default: a caller's leftover setting would
// otherwise send the pasted line looking where the session is not.
func TestContinueIntoPiPrintsTheInvocationItVerifies(t *testing.T) {
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
	printed := result.TargetSession.ExecResumeArguments
	if len(printed) < 5 || printed[1] != "-p" {
		t.Fatalf("exec resume arguments = %v, want print mode first: the reported non-interactive line "+
			"must not open the TUI", printed)
	}
	if printed[len(printed)-1] != resumePromptPlaceholder {
		t.Errorf("exec resume arguments = %v, want the placeholder last", printed)
	}
	projectDir := filepath.Base(filepath.Dir(result.TargetSession.Path))
	wantSessionDir := piSessionTree(home, projectDir)
	if got := result.TargetSession.ResumeEnvironment[piSessionDirEnv]; got != wantSessionDir {
		t.Errorf("the resume environment names %s = %q, want the sessions tree the session was installed "+
			"into (%q)", piSessionDirEnv, got, wantSessionDir)
	}
	for _, command := range []string{result.TargetSession.ResumeCommand, result.TargetSession.ExecResumeCommand} {
		if !strings.Contains(command, piSessionDirEnv+"="+shellQuote(wantSessionDir)) {
			t.Errorf("rendered command = %q, want the sessions tree named so a shell reads it as one word",
				command)
		}
	}

	// The same question the other way round: the check that can verify a turn runs the printed argument
	// vector (the prompt excepted, which is the caller's own turn), in the printed environment's tree.
	turn := &fakePiTurn{growPath: result.TargetSession.Path, answer: "It is in the config file."}
	withFakePiHarness(t, nil, turn)
	verification := VerifyPiResume(context.Background(), PiResumeOptions{
		Home: home, Binary: missingTargetBinary, SessionID: result.TargetSession.SessionID,
		ProjectDir: projectDir, Cwd: result.TargetSession.Cwd,
		SessionPath: result.TargetSession.Path, Prompt: "Where is the retry cap configured?",
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("resume = %+v, want the fake turn verified through the printed invocation", verification)
	}
	if len(turn.calls) != 1 {
		t.Fatalf("the check made %d turn(s), want one", len(turn.calls))
	}
	ran := turn.calls[0].arguments
	if len(ran)+1 != len(printed) {
		t.Fatalf("the check ran %v, want the printed %v (the binary excepted)", ran, printed)
	}
	if !slices.Equal(ran[:len(ran)-1], printed[1:len(printed)-1]) {
		t.Errorf("the check ran %v, want the printed flags and session %v (printed %v)",
			ran, printed[1:len(printed)-1], printed)
	}
	if ran[len(ran)-1] != "Where is the retry cap configured?" {
		t.Errorf("the check's last word = %q, want the caller's own prompt", ran[len(ran)-1])
	}
	if turn.calls[0].sessionDir != wantSessionDir {
		t.Errorf("the check named the sessions tree %q, want the printed one %q",
			turn.calls[0].sessionDir, wantSessionDir)
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
// source uses, with no Pi-specific branch in the pipeline. The claim is made testable by the direct
// writer call below: the pipeline's conversion is compared with a direct conversion of the same document
// by the target's own writer, so a Pi-specific shortcut that bypassed the shared model -- an
// `if provider == "pi"` branch -- is red. The conversion's own report names the inherited Pi reader as
// what read the source.
func TestContinueFromPiThroughTheCommonModel(t *testing.T) {
	conversation := readFixtureConversation(t, piFixtureRef)
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
			directWriterAgreesWithThePipeline(t, target, conversation, result)
		})
	}
}

// directWriterAgreesWithThePipeline drives the target's own writer directly over the same document the
// pipeline read, installs and reads back its own conversion, and requires the pipeline's result to agree
// on what was carried and read back. Whatever route the pipeline took, the answer must be the shared
// model's: a shortcut specific to one source harness would produce a different carried report or a
// different read-back here.
func directWriterAgreesWithThePipeline(
	t *testing.T, target string, conversation Conversation, result ContinueResult,
) {
	t.Helper()
	sessionID := result.TargetSession.SessionID
	ctx := context.Background()
	switch target {
	case TargetClaude:
		direct, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{SessionID: sessionID})
		if err != nil {
			t.Fatalf("converting the same document with the target's own writer: %v", err)
		}
		if got, want := carriedWithoutBytes(result.Conversion.Carried), carriedWithoutBytes(direct.Carried); got != want {
			t.Errorf("the pipeline carried %+v, while the target's own writer over the same source carried %+v",
				got, want)
		}
		directHome := t.TempDir()
		if _, _, err := InstallClaudeTranscript(direct, directHome); err != nil {
			t.Fatalf("installing the direct conversion: %v", err)
		}
		directVerify, err := VerifyClaudeTranscriptStructure(ctx,
			filepath.Join(directHome, claudeProjectsSubdir), direct, fixtureConfig(t))
		if err != nil {
			t.Fatalf("reading the direct conversion back: %v", err)
		}
		if directVerify.ReadBack != result.Verification.Structure.ReadBack {
			t.Errorf("the pipeline read back %+v for %s, while the target's own writer over the same source "+
				"read back %+v: the answer must be the shared model's, whatever the source harness",
				result.Verification.Structure.ReadBack, target, directVerify.ReadBack)
		}
	case TargetCodex:
		direct, err := ConvertToCodexRollout(conversation, CodexTargetOptions{SessionID: sessionID})
		if err != nil {
			t.Fatalf("converting the same document with the target's own writer: %v", err)
		}
		if got, want := carriedWithoutBytes(result.Conversion.Carried), carriedWithoutBytes(direct.Carried); got != want {
			t.Errorf("the pipeline carried %+v, while the target's own writer over the same source carried %+v",
				got, want)
		}
		directHome := t.TempDir()
		if _, _, err := InstallCodexRollout(direct, directHome); err != nil {
			t.Fatalf("installing the direct conversion: %v", err)
		}
		directVerify, err := VerifyCodexRolloutStructure(ctx,
			filepath.Join(directHome, codexSessionsSubdir), direct, fixtureConfig(t))
		if err != nil {
			t.Fatalf("reading the direct conversion back: %v", err)
		}
		if directVerify.ReadBack != result.Verification.Structure.ReadBack {
			t.Errorf("the pipeline read back %+v for %s, while the target's own writer over the same source "+
				"read back %+v: the answer must be the shared model's, whatever the source harness",
				result.Verification.Structure.ReadBack, target, directVerify.ReadBack)
		}
	case TargetPi:
		direct, err := ConvertToPiSession(conversation, PiTargetOptions{SessionID: sessionID})
		if err != nil {
			t.Fatalf("converting the same document with the target's own writer: %v", err)
		}
		if got, want := carriedWithoutBytes(result.Conversion.Carried), carriedWithoutBytes(direct.Carried); got != want {
			t.Errorf("the pipeline carried %+v, while the target's own writer over the same source carried %+v",
				got, want)
		}
		directHome := t.TempDir()
		if _, _, err := InstallPiSession(direct, directHome); err != nil {
			t.Fatalf("installing the direct conversion: %v", err)
		}
		directVerify, err := VerifyPiSessionStructure(ctx,
			filepath.Join(directHome, piSessionsSubdir), direct, fixtureConfig(t))
		if err != nil {
			t.Fatalf("reading the direct conversion back: %v", err)
		}
		if directVerify.ReadBack != result.Verification.Structure.ReadBack {
			t.Errorf("the pipeline read back %+v for %s, while the target's own writer over the same source "+
				"read back %+v: the answer must be the shared model's, whatever the source harness",
				result.Verification.Structure.ReadBack, target, directVerify.ReadBack)
		}
	default:
		t.Fatalf("no direct writer is pinned for target %q", target)
	}
}

// carriedWithoutBytes is one carried report without its byte count, which is the one field two runs of
// the same conversion need not share: the timestamps' fractional digits depend on the clock, so the
// byte length can differ between two conversions of the same document.
func carriedWithoutBytes(carried Carried) Carried {
	carried.Bytes = 0
	return carried
}

// TestContinueFromPiKeepsACompactedAndBranchedHistoryCoherent pins the goal's own clause for the shape
// that is easiest to get wrong: a Pi session compacted at a boundary, whose file also holds a branch the
// conversation left. The conversion carries the boundary as the target's own boundary record, keeps the
// pre-boundary turns and the abandoned branch in the file as recorded history, and reports how much of
// the conversation the boundary stands in front of. What the case measures is the *context* the target's
// loader builds -- the walk from the installed transcript's last record back through its own parent
// links, compared with the walk the source file's last record answers -- rather than a file-wide role
// count, because a file can hold every record and still hand a resumed turn an invented conversation.
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
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed transcript read back as written",
			result.Verification.Structure)
	}
	if result.Verification.Structure.ReadBack.CompactionBoundaries != 1 {
		t.Errorf("read-back boundaries = %d, want the boundary the conversion wrote",
			result.Verification.Structure.ReadBack.CompactionBoundaries)
	}

	// The abandoned branch is taken from the fixture's own bytes rather than restated: the message texts
	// the walk from the file's last record never reaches are the branch the conversation left, and the
	// target transcript must hold every one of them -- recorded history a conversion has no business
	// dropping -- while no record on the walk may hand off to it.
	fixtureBytes, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the history fixture's bytes: %v", err)
	}
	type historyEntry struct {
		Type    string `json:"type"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	var prompts []string
	var fixtureTexts []string
	for _, line := range strings.Split(strings.TrimSpace(string(fixtureBytes)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded := historyEntry{}
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decoding a fixture line: %v", err)
		}
		if decoded.Type != "message" {
			continue
		}
		text := transcriptContentText(decoded.Message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		fixtureTexts = append(fixtureTexts, text)
		if decoded.Message.Role == "user" {
			prompts = append(prompts, text)
		}
	}
	if len(prompts) != 2 {
		t.Fatalf("the fixture holds %d user prompt(s), want its two", len(prompts))
	}
	sourceWalk := transcriptWalkTexts(t, fixtureBytes)
	if len(sourceWalk) != 3 {
		t.Fatalf("the source fixture's own walk = %q, want the three turns of the last record's path",
			sourceWalk)
	}
	var abandoned []string
	for _, text := range fixtureTexts {
		if !slices.Contains(sourceWalk, text) {
			abandoned = append(abandoned, text)
		}
	}
	if len(abandoned) == 0 {
		t.Fatal("the fixture holds no message off the walk from its last record: the fixture cannot " +
			"show a branch the conversation left")
	}

	installedBytes, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed target transcript: %v", err)
	}
	// The context the target's own loader builds: Claude Code walks the installed transcript from its
	// last record back through parentUuid, exactly as Pi's loader walks a Pi session's parentId chain,
	// and that walk must be the one the source's own file answers. What makes the branch recorded history
	// is that it is in the file and no record on the walk hands off to it -- not that it is absent.
	targetWalk := transcriptWalkTexts(t, installedBytes)
	if !slices.Equal(targetWalk, sourceWalk) {
		t.Errorf("the installed transcript's model-visible history = %q, while the source file's own walk "+
			"is %q: the target's loader would read a different conversation than the source holds",
			targetWalk, sourceWalk)
	}
	for _, text := range abandoned {
		if slices.Contains(targetWalk, text) {
			t.Errorf("the abandoned branch's %q is on the walk the target's loader makes, so a turn the "+
				"conversation left was presented as an active one", text)
		}
	}
	type targetRecord struct {
		Type             string `json:"type"`
		IsCompactSummary bool   `json:"isCompactSummary"`
		Message          struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	promptsSeen := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(installedBytes)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded := targetRecord{}
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decoding an installed record: %v", err)
		}
		// A prompt record is the target's own user record that is not the compaction summary and whose
		// content is a plain string: the shape the writer gives a written prompt, as opposed to a
		// tool_result block or the summary record.
		if decoded.Type == "user" && !decoded.IsCompactSummary && isPlainTextContent(decoded.Message.Content) {
			promptsSeen[transcriptContentText(decoded.Message.Content)]++
		}
	}
	for _, text := range abandoned {
		if !strings.Contains(string(installedBytes), text) {
			t.Errorf("the abandoned branch's %q is not in the converted file at all: the branch is recorded "+
				"history and must travel", text)
		}
	}
	for _, prompt := range prompts {
		if promptsSeen[prompt] != 1 {
			t.Errorf("prompt %q appears %d time(s) among the installed prompt records, want once",
				prompt, promptsSeen[prompt])
		}
	}
	if len(promptsSeen) != len(prompts) {
		t.Errorf("the installed transcript holds %d distinct prompt(s) %v, want the fixture's two %v "+
			"(a conversion that invents a third prompt is red)", len(promptsSeen), promptsSeen, prompts)
	}
}

// TestContinueFromTheArchiveKeepsAPiSessionsBranch pins the archive half of the source-relation rule: a
// Pi session read out of the inherited SQLite projection must answer the same record identities and
// parents the native read does, so an archive-backed continuation preserves the source's own tree --
// leaving a branch the conversation left off the walk the target's loader makes -- instead of flattening
// it into the active path, and counts no relation as unresolved that the read answered. The archive
// adapter dropped both columns while the inherited messages table held them, which is what this case
// keeps out: the fallback is exercised through the pipeline, with the source file gone and the archive
// holding the session.
func TestContinueFromTheArchiveKeepsAPiSessionsBranch(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	fixtureBytes, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, fixtureBytes)
	if len(sourceWalk) != 3 {
		t.Fatalf("the source fixture's own walk = %q, want the three turns of the last record's path",
			sourceWalk)
	}

	// A throw-away provider root: the archived copy is the one removed, never the committed fixture.
	root := t.TempDir()
	relative := filepath.Join("-home-user-code-history", piHistoryFixtureRef.ID+".jsonl")
	copyPath := filepath.Join(root, "pi", relative)
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	if err := os.WriteFile(copyPath, fixtureBytes, 0o600); err != nil {
		t.Fatalf("writing the fixture copy: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {filepath.Join(root, "pi")}}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, copyPath); err != nil {
		t.Fatalf("ingesting the fixture copy: %v", err)
	}
	if err := os.Remove(copyPath); err != nil {
		t.Fatalf("removing the source copy: %v", err)
	}

	home := t.TempDir()
	result, err := Continue(ctx, ContinueOptions{
		Ref:          piHistoryFixtureRef,
		Target:       TargetPi,
		Config:       cfg,
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		ArchivePath:  archivePath,
	})
	if err != nil {
		t.Fatalf("continuing the archived session: %v", err)
	}
	if result.Conversion.ReadFrom != "archive" {
		t.Fatalf("read from %q, want the archive fallback for a source file that is gone",
			result.Conversion.ReadFrom)
	}
	installed, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if got := transcriptWalkTexts(t, installed); !slices.Equal(got, sourceWalk) {
		t.Errorf("the archive-backed conversion's model-visible history = %q, while the source file's "+
			"own walk is %q: the archive read lost the relations the native read answers", got, sourceWalk)
	}
	// An archive read has no source file open, so the entry the source's own loader selects as the
	// conversation's leaf is not established from the source's bytes: the report says so, rather than
	// presenting the read's own last record as the leaf the source's file answered.
	leafStated := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "conversation's leaf was not established") &&
			strings.Contains(warning, "rather than from a source file of its own") {
			leafStated = true
		}
	}
	if !leafStated {
		t.Errorf("warnings = %v, want one saying the source's leaf was not established because the read did "+
			"not come from a source file of its own", result.Warnings)
	}
	if loss := lossCount(t, result.Conversion.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %d, want 0: the archive read answers the same relations "+
			"the source read does", loss.Count)
	}
}

// TestContinueFromTheArchiveKeepsAFoldedOutcomeOffAnAbandonedBranch pins the archive half of the
// call-outcome relation on the one shape the native fixture read does not have: a source session whose
// abandoned branch holds a tool call *and* its result. The archive's projection attaches the result-only
// row to the call's own message and drops the row, so the read that reaches the conversion holds the
// outcome on the message that made the call; registering that outcome under the record the call's
// message hangs off chained the sibling turn through the abandoned call's result and revived the call
// branch as the target's active history. The source file is written here rather than carried as a
// fixture: what this case adds is the combination, and the conversion is measured against the source
// bytes' own walk.
func TestContinueFromTheArchiveKeepsAFoldedOutcomeOffAnAbandonedBranch(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	const sessionID = "01a1d100-2222-7333-8444-555566667777"
	source := []byte(strings.Join([]string{
		`{"cwd":"/home/user/code-history","id":"` + sessionID + `","timestamp":"2026-05-05T12:00:00.000Z","type":"session","version":3}`,
		`{"id":"u1","message":{"content":"start the build check","role":"user","timestamp":1795028401000},"parentId":null,"timestamp":"2026-05-05T12:00:01.000Z","type":"message"}`,
		`{"id":"a1","message":{"content":[{"type":"text","text":"running the check"},{"type":"toolCall","id":"call_1","name":"grep","arguments":{"pattern":"maxBackoff"}}],"role":"assistant","timestamp":1795028402000},"parentId":"u1","timestamp":"2026-05-05T12:00:02.000Z","type":"message"}`,
		`{"id":"r1","message":{"content":[{"type":"text","text":"internal/retry/config.go:41"}],"role":"toolResult","timestamp":1795028403000,"toolCallId":"call_1"},"parentId":"a1","timestamp":"2026-05-05T12:00:03.000Z","type":"message"}`,
		// The sibling: a turn recorded after the result that names the call's own parent, so the source's
		// own line runs start -> sibling with the call and its outcome off it.
		`{"id":"b1","message":{"content":[{"type":"text","text":"the cap is 30 seconds"}],"role":"assistant","timestamp":1795028404000},"parentId":"u1","timestamp":"2026-05-05T12:00:04.000Z","type":"message"}`,
	}, "\n") + "\n")
	sourceWalk := transcriptWalkTexts(t, source)
	if len(sourceWalk) != 2 || sourceWalk[0] != "start the build check" ||
		sourceWalk[1] != "the cap is 30 seconds" {
		t.Fatalf("the source file's own walk = %q, want the sibling turn's line and not the abandoned one",
			sourceWalk)
	}

	// A throw-away provider root: the copy is the file the archive ingests, and it is removed before the
	// continuation, so the archive is the only read left.
	root := t.TempDir()
	copyPath := filepath.Join(root, "pi", "-home-user-code-history", sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	if err := os.WriteFile(copyPath, source, 0o600); err != nil {
		t.Fatalf("writing the source file: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {filepath.Join(root, "pi")}}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, copyPath); err != nil {
		t.Fatalf("ingesting the source file: %v", err)
	}
	if err := os.Remove(copyPath); err != nil {
		t.Fatalf("removing the source copy: %v", err)
	}

	home := t.TempDir()
	result, err := Continue(ctx, ContinueOptions{
		Ref:          Ref{Provider: "pi", ID: sessionID},
		Target:       TargetPi,
		Config:       cfg,
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		ArchivePath:  archivePath,
	})
	if err != nil {
		t.Fatalf("continuing the archived session: %v", err)
	}
	if result.Conversion.ReadFrom != "archive" {
		t.Fatalf("read from %q, want the archive fallback for a source file that is gone",
			result.Conversion.ReadFrom)
	}
	installed, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if got := transcriptWalkTexts(t, installed); !slices.Equal(got, sourceWalk) {
		t.Errorf("the archive-backed conversion's model-visible history = %q, while the source file's "+
			"own walk is %q: the read's folded outcome was chained in as the sibling's ancestor", got,
			sourceWalk)
	}
	if !strings.Contains(string(installed), "internal/retry/config.go:41") {
		t.Error("the abandoned call's outcome is not in the converted file at all: it is recorded " +
			"history and must travel, off the walk rather than dropped")
	}
	if loss := lossCount(t, result.Conversion.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %d, want 0: the archive read answers the sibling's parent "+
			"exactly, and no result of that parent stands anywhere for it to have projected through",
			loss.Count)
	}
}

// TestContinueFromPiIntoCodexOmitsAnAbandonedBranch pins the same clause for the route whose target
// cannot hold a branch at all: the target's log is one sequence of records with no parent relation to
// hang a record off, and a response item written into it becomes part of the conversation a resumed turn
// reads. The conversion therefore writes the source's own active line, omits the records the source's
// relations place off it, counts them (inactive_branch_records), and the target's own reader reads back
// exactly the conversation the source file's last record answers -- with no boundary record, because the
// boundary the source recorded stands on the branch and must not cut the line it was not on.
func TestContinueFromPiIntoCodexOmitsAnAbandonedBranch(t *testing.T) {
	fixtureBytes, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the history fixture's bytes: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, fixtureBytes)
	if len(sourceWalk) != 3 {
		t.Fatalf("the source fixture's own walk = %q, want the three turns of the last record's path",
			sourceWalk)
	}

	home := t.TempDir()
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          piHistoryFixtureRef,
		Target:       TargetCodex,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing the history fixture into codex: %v", err)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q, want the source unchanged", result.Installation.SourceCheckState)
	}
	omitted := lossCount(t, result.Conversion.Losses, "inactive_branch_records")
	if omitted.Count != 3 || omitted.Disposition != DispositionOmitted {
		t.Errorf("inactive_branch_records = %+v, want the fixture's three off-line records omitted",
			omitted)
	}
	if result.Conversion.Carried.CompactionBoundaries != 0 {
		t.Errorf("carried boundaries = %d, want none: the fixture's boundary stands on the branch the "+
			"conversion omits, and the target must not cut the line it was not on",
			result.Conversion.Carried.CompactionBoundaries)
	}
	installedBytes, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed rollout: %v", err)
	}
	if strings.Contains(string(installedBytes), `"compacted"`) {
		t.Error("the installed rollout holds a compacted record, so the target would replace the " +
			"conversation's model-visible history with the summary of a boundary that stood off it")
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed rollout read back as written",
			result.Verification.Structure)
	}
	// The context the target's own reader builds from the installed rollout: the inherited Codex reader
	// reads its response items in order, and that must be the source's own line -- neither shortened nor
	// holding a turn the conversation left.
	if got := result.Verification.Structure.ReadBack; got.Messages != 3 || got.UserMessages != 1 ||
		got.AssistantMessages != 2 {
		t.Errorf("read-back = %+v, want the source's own line (three messages: one user, two assistant)",
			got)
	}
	for _, text := range sourceWalk {
		if !strings.Contains(string(installedBytes), text) {
			t.Errorf("the source's own turn %q is not in the installed rollout", text)
		}
	}
	for _, abandoned := range []string{"And the retry cap?", "Thirty seconds, in internal/retry/config.go."} {
		if strings.Contains(string(installedBytes), abandoned) {
			t.Errorf("the abandoned branch's %q is in the installed rollout: a record written into this "+
				"target's log becomes model-visible history, so the branch was revived as an active turn",
				abandoned)
		}
	}
}

// transcriptContentText reads a message record's content either as a plain string -- the shape a Pi user
// record and a written Claude prompt or summary record carry -- or as an array of text blocks, the shape
// an assistant record carries, for the walks these cases make over a file's own bytes.
func transcriptContentText(content json.RawMessage) string {
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var builder strings.Builder
	for _, block := range blocks {
		if block.Type != "text" || block.Text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(block.Text)
	}
	return builder.String()
}

// transcriptWalkTexts reads one transcript's own loader walk: the path from its last record back
// through the record each one names as its parent (`parentUuid` in a Claude transcript, `parentId` in a
// Pi session), reversed into file order, with each message record's own dialogue text. Both loaders walk
// exactly this path, and both formats spell the relation with one of those two fields, so one reader
// measures either side of a conversion -- which is what lets a case compare the conversation a target's
// loader would build with the one the source's own file answers, rather than comparing counts. A record
// the walk reaches that carries no message (a session header, a compaction record) contributes no text:
// the walk is read for its dialogue, and a test that needs to assert a boundary's placement reads the
// records themselves.
func transcriptWalkTexts(t *testing.T, data []byte) []string {
	t.Helper()
	type record struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		UUID       string          `json:"uuid"`
		ParentID   *string         `json:"parentId"`
		ParentUUID *string         `json:"parentUuid"`
		Message    json.RawMessage `json:"message"`
	}
	var records []record
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded := record{}
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decoding a transcript record: %v", err)
		}
		if decoded.Type == "session" {
			continue
		}
		records = append(records, decoded)
	}
	if len(records) == 0 {
		t.Fatal("the transcript holds no record")
	}
	identity := func(entry record) string {
		if entry.UUID != "" {
			return entry.UUID
		}
		return entry.ID
	}
	parentOf := func(entry record) string {
		if entry.ParentUUID != nil {
			return *entry.ParentUUID
		}
		if entry.ParentID != nil {
			return *entry.ParentID
		}
		return ""
	}
	byIdentity := map[string]int{}
	for index, entry := range records {
		if id := identity(entry); id != "" {
			byIdentity[id] = index
		}
	}
	onPath := map[int]bool{}
	var reversed []int
	for cursor := len(records) - 1; cursor >= 0 && !onPath[cursor]; {
		onPath[cursor] = true
		reversed = append(reversed, cursor)
		parent := parentOf(records[cursor])
		if parent == "" {
			break
		}
		next, ok := byIdentity[parent]
		if !ok {
			break
		}
		cursor = next
	}
	walk := make([]string, 0, len(reversed))
	for index := len(reversed) - 1; index >= 0; index-- {
		entry := records[reversed[index]]
		if len(entry.Message) == 0 {
			continue
		}
		var message struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &message); err != nil {
			continue
		}
		if text := transcriptContentText(message.Content); strings.TrimSpace(text) != "" {
			walk = append(walk, text)
		}
	}
	return walk
}

// isPlainTextContent reports whether a record's content is a JSON string rather than a block array: the
// shape the Claude writer gives a written prompt.
func isPlainTextContent(content json.RawMessage) bool {
	var text string
	return json.Unmarshal(content, &text) == nil
}

// TestPiHistoryFixtureBranchIsNotOnTheActivePath reads the committed fixture's own tree and pins the
// property the goal names: the branch the conversation left hangs off an entry the active path does not
// continue from, so a conversion cannot present it as an active turn by following the file's own parent
// links. The walk is over the fixture's bytes -- the shape a real Pi file has -- rather than over any
// product of this build, and the walk's own answer is what the conversion is measured against in the
// case below this one.
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
		Message  struct {
			Role string `json:"role"`
		} `json:"message"`
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
	offPathMessages, offPathUserRecords := 0, 0
	for _, item := range entries {
		if onPath[item.ID] {
			continue
		}
		offPath = append(offPath, item.ID)
		if item.Type == "message" {
			offPathMessages++
			if item.Message.Role == "user" {
				offPathUserRecords++
			}
		}
	}
	if len(offPath) == 0 {
		t.Fatal("every entry is on the active path: the fixture holds no branch the conversation left")
	}
	if offPathMessages == 0 {
		t.Fatal("no message entry is off the active path: the fixture cannot show a turn the " +
			"conversation left")
	}
	// The prompt matters and is checked by name: a conversion that flattened the branch presents an
	// abandoned *user prompt* as an active turn, which is the shape this fixture exists to catch.
	if offPathUserRecords == 0 {
		t.Fatal("no user record is off the active path: the fixture cannot show a conversion turning an " +
			"abandoned prompt into an active turn")
	}
	// Every off-path entry hangs off the active line: walking its own parents reaches an entry that is on
	// the path. An off-path entry that reached none would be a line of its own rather than a branch this
	// conversation left, and the walk above would still report it as off-path.
	for _, id := range offPath {
		reaches := false
		for cursor, steps := id, 0; cursor != "" && steps < len(entries); steps++ {
			item, ok := byID[cursor]
			if !ok {
				break
			}
			if onPath[cursor] {
				reaches = true
				break
			}
			if item.ParentID == nil {
				break
			}
			cursor = *item.ParentID
		}
		if !reaches {
			t.Errorf("the off-path entry %s does not reach the active path through its own parents, so "+
				"it is a line of its own rather than a branch the conversation left", id)
		}
	}
}

// piSourceCopy writes a copy of one committed Pi fixture into a throw-away provider root, with the given
// lines appended, and answers the configuration that reads it, the reference that names it and its path.
// A copy is how a case bends a committed fixture -- growing it, or hanging one more record off an entry --
// without the fixture's own bytes moving; a case that appended to the fixture itself would edit a
// committed artifact, and the archive-backed cases already copy for the same reason.
func piSourceCopy(t *testing.T, fixtureID string, extra ...string) (config.Config, Ref, string) {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", fixtureID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	content := string(data)
	if len(extra) > 0 {
		content += strings.Join(extra, "\n") + "\n"
	}
	root := t.TempDir()
	dir := filepath.Join(root, "pi", "-home-user-code-copy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	path := filepath.Join(dir, fixtureID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the fixture copy: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {filepath.Join(root, "pi")}}
	return cfg, Ref{Provider: "pi", ID: fixtureID}, path
}

// piSourceCopyWithInsertion writes a copy of one committed Pi fixture with one line inserted after the
// first line holding marker, and answers the configuration that reads it, the reference that names it and
// its path. It is piSourceCopy's sibling for a case whose extra record has to stand *between* the
// fixture's own entries: appending it would put it after the fixture's last entry, where the harness's own
// leaf walk runs through it.
func piSourceCopyWithInsertion(t *testing.T, fixtureID, marker, line string) (config.Config, Ref, string) {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", fixtureID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	inserted := false
	out := []string{}
	for _, existing := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		out = append(out, existing)
		if !inserted && strings.Contains(existing, marker) {
			out = append(out, line)
			inserted = true
		}
	}
	if !inserted {
		t.Fatalf("the fixture holds no line matching %s, so the insertion point is not in it", marker)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "pi", "-home-user-code-copy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	path := filepath.Join(dir, fixtureID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture copy: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {filepath.Join(root, "pi")}}
	return cfg, Ref{Provider: "pi", ID: fixtureID}, path
}

// TestPiBranchSwitchFixtureEndsOffTheReadsLastRecord pins what the branch-switch fixture holds and what
// the read answers for it, both read from the fixture's own bytes rather than restated: the file's last
// entry is the switch's `branch_summary`, hanging off the first branch's tip, and the read's last record
// is the abandoned branch's own last turn -- which is the shape the conversion below refuses. Pi's own
// loader answers this file with the first branch plus the summary (measured natively; see
// docs/evidence/0005-pi-continuation.md), so a conversion written from the read's last record would
// resume the branch the summary switched away from.
func TestPiBranchSwitchFixtureEndsOffTheReadsLastRecord(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piBranchSwitchFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	type entry struct {
		Type     string  `json:"type"`
		ID       string  `json:"id"`
		ParentID *string `json:"parentId"`
		FromID   string  `json:"fromId"`
		Summary  string  `json:"summary"`
		Message  struct {
			Role string `json:"role"`
		} `json:"message"`
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
	if len(entries) == 0 {
		t.Fatal("the fixture holds no entry")
	}
	last := entries[len(entries)-1]
	if last.Type != "branch_summary" || last.Summary == "" || last.FromID == "" {
		t.Fatalf("the fixture's last entry = %+v, want the branch switch's own summary record", last)
	}
	if last.ParentID == nil {
		t.Fatal("the fixture's branch_summary names no parent, so the fixture cannot show a switch that " +
			"continues another branch")
	}
	// The entry the switch continues from is a record the read holds, and it is not the read's last record:
	// the summary hangs off the branch the conversation returned to while the abandoned branch's own turns
	// are still in the file behind it.
	continued := *last.ParentID
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piBranchSwitchFixtureRef)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	if len(conversation.Messages) == 0 {
		t.Fatal("the read holds no message")
	}
	lastRead := conversation.Messages[len(conversation.Messages)-1]
	if lastRead.SourceUUID != last.FromID {
		t.Errorf("the read's last record = %s, want the leaf the switch left behind (%s): the fixture "+
			"cannot show a read whose last record is off the branch the file continues",
			lastRead.SourceUUID, last.FromID)
	}
	if lastRead.SourceUUID == continued {
		t.Fatalf("the read's last record is the entry the switch continues from (%s), so the fixture "+
			"does not hold the shape under test", continued)
	}
	held := false
	for _, message := range conversation.Messages {
		if message.SourceUUID == continued {
			held = true
		}
	}
	if !held {
		t.Errorf("the read does not hold the entry the switch continues from (%s): the refusal's own "+
			"answer would be a record outside the read", continued)
	}
}

// TestContinueRefusesABranchSwitchTheReadDoesNotReach pins the source-side leaf rule end to end: a Pi
// session whose file switched back to an earlier branch is refused for every target, because the record
// the target's loader would walk from -- the read's own last record -- is not on the branch the source's
// own loader selects. A target that wrote from the read's last record would resume the branch the
// conversation left, and a target that writes the source's own line (Codex) would omit the branch the
// file continues while keeping the abandoned one; both are the same wrong conversation, so the refusal is
// the source's own answer rather than one target writer's.
func TestContinueRefusesABranchSwitchTheReadDoesNotReach(t *testing.T) {
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piBranchSwitchFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("decoding the fixture's last entry: %v", err)
	}
	path := fixturePath(t, filepath.Join("pi", piBranchSwitchFixtureRef.ID+".jsonl"))
	for _, target := range []string{TargetPi, TargetClaude, TargetCodex} {
		t.Run(target, func(t *testing.T) {
			_, err := Continue(context.Background(), ContinueOptions{
				Ref:          piBranchSwitchFixtureRef,
				Target:       target,
				Config:       fixtureConfig(t),
				TargetHome:   t.TempDir(),
				TargetBinary: missingTargetBinary,
			})
			if !errors.Is(err, ErrUnrepresentableLeaf) {
				t.Fatalf("continuing to %s = %v, want ErrUnrepresentableLeaf: the read's last record is "+
					"not the record the source file's own loader walks from", target, err)
			}
			for _, want := range []string{path, last.Type, last.ID} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal %q does not name %q", err, want)
				}
			}
		})
	}
}

// TestContinueKeepsARecordOutsideTheContextOutOfTheLeafRefusal pins the other half of the leaf rule: an
// entry the read does not carry may stand on the file's own leaf path when the harness's own loader keeps
// its kind out of the conversation. The copy's last line is the metadata record Pi writes when a session
// is renamed, hanging off the last message; the file's leaf is therefore not the read's last record, and
// the conversion must still go through -- the record adds no message to the source's conversation, so the
// record the read's last message stands for is still the entry the loader walks from.
func TestContinueKeepsARecordOutsideTheContextOutOfTheLeafRefusal(t *testing.T) {
	cfg, ref, path := piSourceCopy(t, piFixtureRef.ID,
		`{"type":"session_info","id":"f1e2d3c4","parentId":"1d4e7a90","timestamp":"2026-05-04T09:17:00.000Z","name":"retry cap notes"}`)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          ref,
		Target:       TargetPi,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing a session whose file ends with a rename record: %v", err)
	}
	if result.Conversion.State != ConversionStateConverted {
		t.Errorf("conversion state = %q, want the copy converted", result.Conversion.State)
	}
	if result.Source == nil || result.Source.Boundary == nil || result.Source.Boundary.Path != path {
		t.Errorf("the source report names %+v, want the copy's own file %s", result.Source, path)
	}
	// The check's own loss category is a count of the records the *file* holds: a source whose file holds no
	// system record must not report one, so the category's presence in a report is its own evidence.
	for _, loss := range result.Conversion.Losses {
		if loss.Category == "source_system_records" {
			t.Errorf("the conversion reports %+v for a source file that holds no system record: the category "+
				"is the source-side check's own count, and a file that holds none must not name one", loss)
		}
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "not established") {
			t.Errorf("warning %q says the leaf was not established, while the file's own leaf path was "+
				"read and supported the read's last record", warning)
		}
	}
	// The installed session's own leaf is the record the read's last message stands for, which is what the
	// target's loader walks from: the walk the converted file answers is the copy's own walk.
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	installed, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, source)
	if got := transcriptWalkTexts(t, installed); !slices.Equal(got, sourceWalk) {
		t.Errorf("the installed session's model-visible history = %q, want the copy's own %q", got, sourceWalk)
	}
}

// TestContinueRefusesADroppedContextRecordOnTheLeafsPath pins the leaf rule's second half on its own: an
// entry this read does not carry must not stand on the file's own leaf path unless the harness's own loader
// keeps its kind out of the conversation. The copy's last line is a branch switch whose summary hangs off
// the record the read's last message stands for -- the shape a `/tree` switch to the branch the session is
// already on leaves -- so the record the target's loader would walk from is the right one, and what the
// conversion cannot give the target is the message the source's own loader builds from the summary.
// Refusing is the rule: a converted session whose conversation is silently missing a context record the
// source's own loader reads would be claiming the source's conversation.
func TestContinueRefusesADroppedContextRecordOnTheLeafsPath(t *testing.T) {
	cfg, ref, _ := piSourceCopy(t, piBranchSwitchFixtureRef.ID,
		`{"type":"branch_summary","id":"f1e2d3c4","parentId":"df607182","timestamp":"2026-05-07T08:34:00.000Z","fromId":"e0718293","summary":"The user compared the archive lock with the retry backoff."}`)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	if last := conversation.Messages[len(conversation.Messages)-1].SourceUUID; last != "df607182" {
		t.Fatalf("the read's last record = %s, want the copy's abandoned-branch leaf: the case is about a "+
			"leaf path whose deepest record the read *does* hold", last)
	}
	_, err = Continue(context.Background(), ContinueOptions{
		Ref:          ref,
		Target:       TargetPi,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("continuing the copy = %v, want ErrUnrepresentableLeaf: the file's leaf path holds a "+
			"branch_summary record whose words the harness's own loader puts in the conversation and "+
			"this read does not carry", err)
	}
	if !strings.Contains(err.Error(), "branch_summary") {
		t.Errorf("the refusal %q does not name the record's kind", err)
	}
}

// TestContinueCarriesASessionPiItselfHasExtended pins the source direction on the shape a real Pi run
// leaves behind, which the committed fixture (a hand-written linear session) does not have: when Pi
// answers a turn it appends its own records to the session file -- a system record holding the prompt
// sections and tool loadout it rebuilt the prompt from, then the user turn and the answer
// (docs/evidence/0005-pi-continuation.md, section 2, measured on 1.5.2). The system record stands on the
// file's own leaf path, the inherited Pi reader does not carry its role, and before this case's fix the
// leaf check refused every such session with ErrUnrepresentableLeaf -- so the source direction worked for
// the hand-written fixture and failed for every session Pi had actually extended.
//
// The fix is an accounted omission, and this case pins all three of its parts: the conversion proceeds for
// every target, the omission is *reported* as its own loss category with the count the file's own entries
// answer, and what the target carries is the conversation's turns rather than the harness's prompt state.
// The copy's own least-record read is checked first, so a case that broke the parser's role handling would
// fail where it broke rather than in the counts below.
func TestContinueCarriesASessionPiItselfHasExtended(t *testing.T) {
	// The three records the harness appended, in the shape and order the evidence record measured: the
	// system record hangs off the record the conversation stood on, and the turn's two messages hang off
	// one another. The system record's `content` is empty because its payload is the prompt state
	// (`sections`, `toolsAdded`), which is what Pi's own format document says a system record carries.
	const (
		systemRecord = `{"type":"message","id":"11aa22bb","parentId":"1d4e7a90","timestamp":"2026-05-04T09:17:00.000Z","message":{"role":"system","content":"","sections":{"preamble":"You are an expert coding assistant.","cwd":"/home/user/code/example-api"},"toolsAdded":[{"name":"read","description":"Read a file","parameters":{}}],"timestamp":1777886220000}}`
		userTurn     = `{"type":"message","id":"22bb33cc","parentId":"11aa22bb","timestamp":"2026-05-04T09:17:01.000Z","message":{"role":"user","content":"And the retry budget?","timestamp":1777886221000}}`
		answerTurn   = `{"type":"message","id":"33cc44dd","parentId":"22bb33cc","timestamp":"2026-05-04T09:17:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"The cap is configured in internal/retry/config.go."}],"model":"claude-sonnet-4-5","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0,"totalFromProvider":false}},"stopReason":"stop","timestamp":1777886224000}}`
	)
	cfg, ref, path := piSourceCopy(t, piFixtureRef.ID, systemRecord, userTurn, answerTurn)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	lastRead := conversation.Messages[len(conversation.Messages)-1].SourceUUID
	if lastRead != "33cc44dd" {
		t.Fatalf("the read's last record = %q, want the harness's own answer record: the case is about a "+
			"session whose leaf is the turn the harness appended", lastRead)
	}
	// The read does not hold the system record: the carried parser keeps the message roles user,
	// assistant and toolResult, and the case's premise is that the record is on the file's leaf path
	// without being in the conversation the read answers.
	for _, message := range conversation.Messages {
		if message.SourceUUID == "11aa22bb" {
			t.Fatalf("the read answered a record for the system entry, which this case assumes the carried "+
				"parser drops: %+v", message)
		}
	}
	omitted, err := piSourceLeafRefusal(conversation, path)
	if err != nil {
		t.Fatalf("the leaf check on a session Pi itself has extended = %v, want none: the harness's own "+
			"system record is a record whose omission the conversion accounts for, not a refusal", err)
	}
	if omitted != 1 {
		t.Errorf("the leaf check counted %d system record(s), want 1: the check's count is what the "+
			"conversion's loss category reports", omitted)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	if !strings.Contains(string(source), `"role":"system"`) {
		t.Fatalf("the copy holds no system record, so the case would pass vacuously")
	}
	sourceWalk := transcriptWalkTexts(t, source)
	for _, target := range []string{TargetPi, TargetClaude, TargetCodex} {
		t.Run(target, func(t *testing.T) {
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          ref,
				Target:       target,
				Config:       cfg,
				TargetHome:   t.TempDir(),
				TargetBinary: missingTargetBinary,
			})
			if err != nil {
				t.Fatalf("continuing a session Pi itself has extended to %s: %v", target, err)
			}
			if result.Conversion.State != ConversionStateConverted {
				t.Fatalf("conversion state = %q, want the copy converted", result.Conversion.State)
			}
			var counted *Loss
			for index := range result.Conversion.Losses {
				if result.Conversion.Losses[index].Category == "source_system_records" {
					counted = &result.Conversion.Losses[index]
				}
			}
			if counted == nil {
				t.Fatalf("the conversion's losses %+v do not name source_system_records, so the record the "+
					"source's own file holds and the read drops travels unreported", result.Conversion.Losses)
			}
			if counted.Disposition != DispositionOmitted || counted.Count != 1 {
				t.Errorf("the reported omission = %+v, want one omitted record: the count comes from the "+
					"source file's own entries", *counted)
			}
			if !strings.Contains(counted.Detail, "prompt and tool loadout") {
				t.Errorf("the loss detail %q does not say what a Pi system record is", counted.Detail)
			}
			// The read holds the fixture's own five records plus the two the harness's turn appended; the
			// fixture's tool result is counted as a result rather than as a message, so those seven records
			// travel as six messages and one result, and the harness's prompt record is in neither count.
			if got := len(conversation.Messages); got != 7 {
				t.Fatalf("the read holds %d record(s), want the literal 7 the copy's bytes answer", got)
			}
			if got := result.Conversion.Carried.Messages; got != 6 {
				t.Errorf("carried %d message(s), want the literal 6: the harness's prompt record is not a "+
					"turn and must not become one", got)
			}
			if got := result.Conversion.Carried.ToolResults; got != 1 {
				t.Errorf("carried %d tool result(s), want the literal 1", got)
			}
			installed, err := os.ReadFile(result.TargetSession.Path)
			if err != nil {
				t.Fatalf("reading the installed session: %v", err)
			}
			if strings.Contains(string(installed), "expert coding assistant") {
				t.Errorf("the installed %s session holds the source's prompt text, which the conversion "+
					"reports as omitted:\n%s", target, installed)
			}
			if !strings.Contains(string(installed), "And the retry budget?") {
				t.Errorf("the installed %s session does not hold the turn the harness appended after its "+
					"system record:\n%s", target, installed)
			}
			if target == TargetPi {
				// The Pi target reproduces the source's own tree: the harness's own loader, walked over
				// the installed file, reads the same conversation the copy's own walk holds.
				if got := transcriptWalkTexts(t, installed); !slices.Equal(got, sourceWalk) {
					t.Errorf("the installed session's model-visible history = %q, want the copy's own %q",
						got, sourceWalk)
				}
				if strings.Contains(string(installed), `"role":"system"`) {
					t.Errorf("the installed Pi session records a system message, which this conversion "+
						"reports as omitted rather than writing:\n%s", installed)
				}
			}
		})
	}
}

// TestContinueRefusesAMessageRoleItCannotAccountFor pins the boundary of the accounting: the system
// record's omission is one named shape (piAccountedSourceOmission), and a message record of any other role
// this build does not know remains context it cannot say it may drop. The copy's last line is such a record
// hanging off the record the read's last message stands for, so the leaf path is otherwise sound and the
// refusal is the role's own -- and it answers zero counted records, because a refused conversion reports no
// conversion.
func TestContinueRefusesAMessageRoleItCannotAccountFor(t *testing.T) {
	cfg, ref, path := piSourceCopy(t, piFixtureRef.ID,
		`{"type":"message","id":"55ee66ff","parentId":"1d4e7a90","timestamp":"2026-05-04T09:17:00.000Z","message":{"role":"developer","content":"an instruction record this build does not know"}}`)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	omitted, err := piSourceLeafRefusal(conversation, path)
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: a message record of a role this build "+
			"does not name is not a record whose omission it can account for", err)
	}
	if omitted != 0 {
		t.Errorf("the refusal answered %d counted record(s), want 0: a refused conversion reports nothing",
			omitted)
	}
	for _, want := range []string{"55ee66ff", "developer", "cannot say the harness's own conversation " +
		"context leaves out"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
	// The same record through the conversion surface: the refusal is what `ccc continue` reports, not an
	// error the pipeline swallows.
	_, err = Continue(context.Background(), ContinueOptions{
		Ref:          ref,
		Target:       TargetClaude,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("continuing the copy = %v, want ErrUnrepresentableLeaf", err)
	}
}

// TestContinueRefusesALeafPathThatReachesAnotherBranch pins the leaf rule's identity comparison on the
// shape the summary itself is not part of: the copy's last line is another rename record, but this one
// hangs off the branch the switch continued from rather than off the read's last record. The file's leaf
// path therefore reaches a record the read holds -- and that record is not the read's last record, so the
// conversion is refused while the earlier refusal (an entry whose kind this build cannot place) does not
// apply here.
func TestContinueRefusesALeafPathThatReachesAnotherBranch(t *testing.T) {
	cfg, ref, path := piSourceCopy(t, piBranchSwitchFixtureRef.ID,
		`{"type":"session_info","id":"f1e2d3c4","parentId":"bd4e5f60","timestamp":"2026-05-07T08:33:00.000Z","name":"archive lock notes"}`)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	lastRead := conversation.Messages[len(conversation.Messages)-1].SourceUUID
	_, err = piSourceLeafRefusal(conversation, path)
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the file's leaf path reaches the "+
			"branch the switch continued from, not the read's last record (%s)", err, lastRead)
	}
	for _, want := range []string{"bd4e5f60", lastRead, "branch the source's conversation left"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// TestContinueRefusesALeafPathWithNoRecordTheReadHolds pins the third answer the walk gives: a file whose
// leaf path holds no record this read holds at all. The copy's last line is a usage record hanging off the
// session's own root, so the walk from the leaf passes only records outside the conversation -- and the
// read's messages, including its last record, are on a branch the file's leaf does not reach.
func TestContinueRefusesALeafPathWithNoRecordTheReadHolds(t *testing.T) {
	cfg, ref, path := piSourceCopy(t, piFixtureRef.ID,
		`{"type":"usage","id":"aabbccdd","parentId":"9e7a5bd5","timestamp":"2026-05-04T09:18:00.000Z","kind":"cache_warm","usage":{"input":0,"output":0,"cacheRead":1200,"cacheWrite":0,"totalTokens":1200}}`)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	lastRead := conversation.Messages[len(conversation.Messages)-1].SourceUUID
	_, err = piSourceLeafRefusal(conversation, path)
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the file's own leaf path reaches no "+
			"record this read holds, while the read's last record is %s", err, lastRead)
	}
	for _, want := range []string{"reaches no record this read holds", lastRead} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// TestPiSourceLeafRefusalAnswersAnIdentityFreeFileByOrder pins the file that records no tree: a Pi v1
// session writes no entry identities, so its own order is its answer -- it cannot hold a branch, because a
// branch is a `parentId` link -- and the entry its loader selects is the file's last entry. A metadata
// record after the last message is outside the context and asks nothing of the read; a record kind the
// harness's own loader builds context from is refused.
func TestPiSourceLeafRefusalAnswersAnIdentityFreeFileByOrder(t *testing.T) {
	const header = `{"type":"session","version":1,"timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
	const prompt = `{"type":"message","timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"user","content":"Where is the retry cap configured?"}}`
	const answer = `{"type":"message","timestamp":"2026-05-04T09:15:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"It is set in internal/retry/config.go."}],"usage":{"input":412,"output":38,"totalTokens":450}}}`
	write := func(t *testing.T, lines ...string) (string, Conversation) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "v1-session.jsonl")
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatalf("writing the session: %v", err)
		}
		cfg := fixtureConfig(t)
		cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {dir}}
		conversation, err := NewSourceReader(cfg).Read(context.Background(), Ref{Provider: "pi", ID: path})
		if err != nil {
			t.Fatalf("reading the v1 session: %v", err)
		}
		if len(conversation.Messages) != 2 || conversation.Messages[1].SourceUUID != "" {
			t.Fatalf("the read = %+v, want the file's two messages with no identities answered: a v1 "+
				"session records none, and the check below is about a file that answers no tree",
				conversation.Messages)
		}
		return path, conversation
	}

	t.Run("a record outside the context after the last message", func(t *testing.T) {
		path, conversation := write(t, header, prompt, answer,
			`{"type":"session_info","timestamp":"2026-05-04T09:16:00.000Z","name":"retry cap notes"}`)
		if _, err := piSourceLeafRefusal(conversation, path); err != nil {
			t.Errorf("the leaf check = %v, want none: a session title adds no message to the conversation", err)
		}
	})

	t.Run("a record the loader builds context from", func(t *testing.T) {
		path, conversation := write(t, header, prompt, answer,
			`{"type":"branch_summary","timestamp":"2026-05-04T09:16:00.000Z","fromId":"","summary":"another branch"}`)
		_, err := piSourceLeafRefusal(conversation, path)
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the file's last record is one the "+
				"harness's own loader builds a message from and this read does not carry", err)
		}
		if !strings.Contains(err.Error(), "branch_summary") {
			t.Errorf("the refusal %q does not name the record's kind", err)
		}
	})

	t.Run("the harness's own system record after the last message", func(t *testing.T) {
		path, conversation := write(t, header, prompt, answer,
			`{"type":"message","timestamp":"2026-05-04T09:16:00.000Z","message":{"role":"system","content":"","sections":{"preamble":"You are an expert coding assistant."},"toolsAdded":[{"name":"read","parameters":{}}]}}`)
		omitted, err := piSourceLeafRefusal(conversation, path)
		if err != nil {
			t.Fatalf("the leaf check = %v, want none: the harness's own system record is a record whose "+
				"omission the conversion accounts for", err)
		}
		if omitted != 1 {
			t.Errorf("the leaf check counted %d system record(s), want 1", omitted)
		}
	})
}

// TestPiSourceLeafRefusalCountsACycledSystemRecordOnce pins the omission count's own idempotence, the one
// answer of the walk that is not stateless. The file's last entry is the record the read's last message
// stands for, and the two system records above it name each other, so the walk passes them and is turned
// back through them until its bound is spent: each record is counted once, and the report names the two
// records the file actually holds rather than the walk's iteration count.
func TestPiSourceLeafRefusalCountsACycledSystemRecordOnce(t *testing.T) {
	const header = `{"type":"session","version":3,"id":"01a1c33a-0000-4000-8000-000000000000","timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
	dir := t.TempDir()
	path := filepath.Join(dir, "cycled-system-records.jsonl")
	// File order puts the read's record last, which is what makes it the file's leaf; the system records
	// above it close a loop (aa11bb22 names cc33dd44, cc33dd44 names aa11bb22).
	content := strings.Join([]string{
		header,
		`{"type":"message","id":"cc33dd44","parentId":"aa11bb22","timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"system","content":"","sections":{"preamble":"You are an expert coding assistant."}}}`,
		`{"type":"message","id":"aa11bb22","parentId":"cc33dd44","timestamp":"2026-05-04T09:15:02.000Z","message":{"role":"system","content":"","sections":{"skills":"<skills>…</skills>"}}}`,
		`{"type":"message","id":"bb22cc33","parentId":"aa11bb22","timestamp":"2026-05-04T09:15:03.000Z","message":{"role":"user","content":"hello"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the session: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {dir}}
	conversation, err := NewSourceReader(cfg).Read(context.Background(), Ref{Provider: "pi", ID: path})
	if err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	if last := conversation.Messages[len(conversation.Messages)-1].SourceUUID; last != "bb22cc33" {
		t.Fatalf("the read's last record = %q, want the file's own last entry: the case is about the leaf "+
			"walking into the cycle above it", last)
	}
	omitted, err := piSourceLeafRefusal(conversation, path)
	if err != nil {
		t.Fatalf("the leaf check = %v, want none: the leaf is the record the read holds and the records "+
			"above it are ones the conversion accounts for", err)
	}
	if omitted != 2 {
		t.Errorf("the check counted %d system record(s), want the literal 2: the file holds two, and the "+
			"walk re-reaching them through the cycle must not count them again", omitted)
	}
}

// TestPiSourceLeafCheckStatesTheLeafItCouldNotEstablish pins the two answers that say a leaf was *not*
// established, at the seam where each is produced: a read whose source file was not open (an archive row,
// or a conversation with no file behind it) and a file that moved while it was being read. Both are the
// report's own channel -- the same state the boundary reports for the same situations -- rather than a
// refusal: the read is real, and what the report owes its caller is that the record the source's own
// loader selects was not established from the source's bytes.
func TestPiSourceLeafCheckStatesTheLeafItCouldNotEstablish(t *testing.T) {
	conversation := Conversation{
		Source:   SourceIdentity{Provider: "pi", SourcePath: "/home/user/code/example-api/session.jsonl"},
		Messages: []Message{{Ordinal: 0, Role: string(parser.RoleUser), Content: "hello", SourceUUID: "aa11bb22"}},
	}
	t.Run("a read with no source file open", func(t *testing.T) {
		for _, readFrom := range []string{"archive", "source"} {
			boundary := SourceBoundary{State: SourceBoundaryNotTaken, Detail: "no source file was open at all"}
			answer, err := piSourceLeafCheck(conversation, readFrom, boundary)
			if err != nil {
				t.Fatalf("the leaf check (%s) = %v, want a stated answer rather than a refusal", readFrom, err)
			}
			if !strings.Contains(answer.Warning, "was not established") {
				t.Errorf("the warning %q does not say the leaf was not established", answer.Warning)
			}
			if len(answer.Losses) != 0 {
				t.Errorf("the leaf check (%s) answered losses %+v, want none: no source file was read to "+
					"count records from", readFrom, answer.Losses)
			}
		}
		// The boundary pass's own reason travels: a read that came from the source but whose conversation no
		// file holds (a container member, a read that reported no path) is answered with the pass's sentence
		// for it, and a conversation no boundary pass answered for gets the check's own.
		stated := SourceBoundary{State: SourceBoundaryNotTaken, Detail: "the source read resolved member x inside the container y"}
		answer, err := piSourceLeafCheck(conversation, "source", stated)
		if err != nil || !strings.Contains(answer.Warning, "inside the container y") {
			t.Errorf("the leaf check = %q, %v; want the boundary's own reason quoted", answer.Warning, err)
		}
		answer, err = piSourceLeafCheck(conversation, "source", SourceBoundary{})
		if err != nil || !strings.Contains(answer.Warning, "no source file of its own was measured") {
			t.Errorf("the leaf check = %q, %v; want its own sentence for a conversation no boundary answered",
				answer.Warning, err)
		}
		// A boundary that was taken but names no file is the same answer: the gate asks for a path.
		answer, err = piSourceLeafCheck(conversation, "source", SourceBoundary{State: SourceBoundaryTaken})
		if err != nil || !strings.Contains(answer.Warning, "was not established") {
			t.Errorf("the leaf check = %q, %v; want a stated answer for a taken boundary with no file",
				answer.Warning, err)
		}
	})
	t.Run("a source that is not a Pi-family read", func(t *testing.T) {
		claude := conversation
		claude.Source.Provider = "claude"
		answer, err := piSourceLeafCheck(claude, "source", SourceBoundary{State: SourceBoundaryNotTaken})
		if answer.Warning != "" || len(answer.Losses) != 0 || err != nil {
			t.Errorf("the leaf check for a Claude read = %+v, %v; want no answer at all: the rule is about "+
				"the carried Pi parser, and another reader answers its own source's leaf", answer, err)
		}
	})
	t.Run("a file that moved while it was read", func(t *testing.T) {
		conversation := conversation
		conversation.Source.FileSize = 8
		boundary := SourceBoundary{State: SourceBoundaryTaken, Path: "/tmp/session.jsonl", Bytes: 16}
		answer, err := piSourceLeafCheck(conversation, "source", boundary)
		if err != nil {
			t.Fatalf("the leaf check = %v, want a stated answer rather than a refusal", err)
		}
		for _, want := range []string{"16 byte(s)", "recorded 8", "was not established"} {
			if !strings.Contains(answer.Warning, want) {
				t.Errorf("the warning %q does not name %q", answer.Warning, want)
			}
		}
	})
}

// TestPiScanConversationEntriesStopsAtItsLineBound pins the scan's own rule: its line bound is what one
// line may be, and a line beyond it stops the scan with the read error rather than leaving the caller to
// judge a file by the entries before the line it could not read. The bound's one value here is the leaf
// check's source scan (piSourceEntryLineMax, internal/parser's maxLineSize); the load check's byte-backed
// read of a converted file takes no bound at all (piConversationEntries), because the writer imposes none
// on an entry and a bounded read there would judge the file by a prefix. A bufio.Scanner's maximum token
// size is the larger of the bound and the initial buffer, so the line this case uses is beyond both.
func TestPiScanConversationEntriesStopsAtItsLineBound(t *testing.T) {
	long := `{"type":"message","id":"aa11bb22","parentId":null,"message":{"role":"user","content":"` +
		strings.Repeat("x", 70<<10) + `"}}`
	content := long + "\n" + `{"type":"usage","id":"cc33dd44","parentId":"aa11bb22"}` + "\n"
	var seen []string
	err := piScanConversationEntries(strings.NewReader(content), 64<<10, func(entry piConversationEntry) {
		seen = append(seen, entry.ID)
	})
	if err == nil {
		t.Fatalf("the scan answered no error for a line beyond its bound (it read %v): a caller would "+
			"judge the file by the entries before the line it could not read", seen)
	}
	if len(seen) != 0 {
		t.Errorf("the scan answered %v before failing: the first line is already beyond the bound", seen)
	}
	seen = nil
	if err := piScanConversationEntries(strings.NewReader(content), len(content), func(entry piConversationEntry) {
		seen = append(seen, entry.ID)
	}); err != nil {
		t.Fatalf("the scan failed within its bound: %v", err)
	}
	if len(seen) != 2 || seen[0] != "aa11bb22" || seen[1] != "cc33dd44" {
		t.Errorf("the scan read %v, want both entries in the file's order", seen)
	}
}

// TestPiSourceLeafRefusalEndsAWalkThroughARecordedCycle pins the walk's termination rule on a malformed
// file: a record naming itself and two records naming each other are walked until the file's own entry
// count is spent, and the answer is what the walk reached -- here no record of the read at all, because the
// loop stands beside the conversation. The bound is the file's own size in entries rather than a depth
// constant, so a linear history longer than any fixed bound is still walked to its end; without it the walk
// would never finish, which is why the case runs the check under a deadline of its own rather than under
// the suite's.
func TestPiSourceLeafRefusalEndsAWalkThroughARecordedCycle(t *testing.T) {
	const header = `{"type":"session","version":3,"id":"01a1c339-0000-4000-8000-000000000000","timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
	dir := t.TempDir()
	path := filepath.Join(dir, "cycle.jsonl")
	content := strings.Join([]string{
		header,
		`{"type":"message","id":"aa11bb22","parentId":null,"timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"user","content":"hello"}}`,
		// A record naming itself, then two records naming each other, and the file's last entry is the one
		// the cycle reaches: the walk from it visits the model_change records and never the message.
		`{"type":"model_change","id":"bb22cc33","parentId":"bb22cc33","timestamp":"2026-05-04T09:15:02.000Z","provider":"p","modelId":"m"}`,
		`{"type":"model_change","id":"cc33dd44","parentId":"dd44ee55","timestamp":"2026-05-04T09:16:00.000Z","provider":"p","modelId":"m"}`,
		`{"type":"model_change","id":"dd44ee55","parentId":"cc33dd44","timestamp":"2026-05-04T09:16:01.000Z","provider":"p","modelId":"m"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the session: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {dir}}
	conversation, err := NewSourceReader(cfg).Read(context.Background(), Ref{Provider: "pi", ID: path})
	if err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := piSourceLeafRefusal(conversation, path); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the file's leaf path is a cycle of "+
				"records outside the read, so it reaches none of them", err)
		}
		if !strings.Contains(err.Error(), "reaches no record this read holds") {
			t.Errorf("the refusal %q does not say the leaf path reaches no record of the read", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the leaf check did not answer for a file whose leaf path is a cycle: the walk's bound must " +
			"be the file's own entries rather than an unbounded search")
	}
}

// TestContinueWalksALeafPathLongerThanOneStep pins the walk's reach: the bound is the file's own entry
// count, not a depth constant, so a leaf path longer than any fixed bound is still walked to the record the
// read holds at its top. The copy's last lines are five records the harness's own loader keeps out of the
// conversation, each hanging off the one before, and the first of them hangs off the read's last message:
// the file's leaf path is six entries long, and the conversion goes through.
func TestContinueWalksALeafPathLongerThanOneStep(t *testing.T) {
	chain := make([]string, 0, 5)
	parent := "1d4e7a90"
	for index := 1; index <= 5; index++ {
		id := fmt.Sprintf("f1e2d3%02d", index)
		chain = append(chain, fmt.Sprintf(
			`{"type":"label","id":%q,"parentId":%q,"timestamp":"2026-05-04T09:17:00.000Z","targetId":"1d4e7a90","label":"checkpoint-%d"}`,
			id, parent, index))
		parent = id
	}
	cfg, ref, _ := piSourceCopy(t, piFixtureRef.ID, chain...)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          ref,
		Target:       TargetPi,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing a session whose leaf path holds %d records outside the conversation: %v",
			len(chain), err)
	}
	if result.Conversion.State != ConversionStateConverted {
		t.Errorf("conversion state = %q, want the copy converted", result.Conversion.State)
	}
}

// TestPiSourceLeafRefusalRefusesAReadWithNoIdentityForItsLastRecord pins the answer the walk gives when
// the file records a tree but the read answered no identity for its last record: the copy's last two lines
// are a message the file records without an identity (which the read carries as a record it cannot name)
// and a rename record after it, so the walk from the file's leaf reaches an identity-bearing record while
// the read's own leaf stands for no identity at all. Which entry the file's leaf stands for is then not
// established, and a conversion claiming a leaf would be claiming a relation the read never answered.
func TestPiSourceLeafRefusalRefusesAReadWithNoIdentityForItsLastRecord(t *testing.T) {
	cfg, ref, path := piSourceCopy(t, piFixtureRef.ID,
		`{"type":"message","parentId":"1d4e7a90","timestamp":"2026-05-04T09:16:30.000Z","message":{"role":"assistant","content":[{"type":"text","text":"a record the file never named"}]}}`,
		`{"type":"session_info","id":"f1e2d3c4","parentId":"1d4e7a90","timestamp":"2026-05-04T09:17:00.000Z","name":"retry cap notes"}`)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the copy: %v", err)
	}
	if last := conversation.Messages[len(conversation.Messages)-1]; last.SourceUUID != "" {
		t.Fatalf("the read's last record answered the identity %q, want none: the copy's last message "+
			"entry records no id, and the check's answer for a read that answered no leaf is what this "+
			"case pins", last.SourceUUID)
	}
	_, err = piSourceLeafRefusal(conversation, path)
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the read answered no identity for its "+
			"last record", err)
	}
	if !strings.Contains(err.Error(), "answered no identity") {
		t.Errorf("the refusal %q does not say the read answered no identity for its last record", err)
	}
}

// TestPiSourceLeafRefusalAnswersAReadWithNoRecordsAndAFileWithNoEntries pins the two shapes the check
// answers without walking a path: a read that holds no record at all against a file whose own entries are
// all outside the conversation's context (there is no leaf to resume wrongly, and the loader would build
// no conversation from the file either) and a file that holds nothing beyond its session header (there is
// no entry the loader could select). Neither is a refusal: an empty conversation converts to an empty
// session, and no branch can be revived by it. The empty read is asked of a file that *does* hold an
// entry, so the answer is the guard's own rather than the file's; a file entry the harness's own loader
// builds context from is the case beside this one
// (TestPiSourceLeafRefusalRefusesADroppedRecordBehindAnEmptyRead).
func TestPiSourceLeafRefusalAnswersAReadWithNoRecordsAndAFileWithNoEntries(t *testing.T) {
	dir := t.TempDir()
	header := `{"type":"session","version":3,"id":"01a1c338-0000-4000-8000-000000000000","timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
	headerOnly := filepath.Join(dir, "header-only.jsonl")
	if err := os.WriteFile(headerOnly, []byte(header+"\n"), 0o600); err != nil {
		t.Fatalf("writing the header-only session: %v", err)
	}
	metadataOnly := filepath.Join(dir, "metadata-only.jsonl")
	if err := os.WriteFile(metadataOnly, []byte(header+"\n"+
		`{"type":"usage","id":"aabbccdd","parentId":null,"timestamp":"2026-05-04T09:16:00.000Z","kind":"cache_warm","usage":{"input":0,"output":0,"cacheRead":1200,"cacheWrite":0,"totalTokens":1200}}`+"\n"),
		0o600); err != nil {
		t.Fatalf("writing the metadata-only session: %v", err)
	}
	recorded := Conversation{
		Source:   SourceIdentity{Provider: "pi", SourcePath: headerOnly},
		Messages: []Message{{Ordinal: 0, Role: string(parser.RoleUser), Content: "hello", SourceUUID: "aa11bb22"}},
	}
	if _, err := piSourceLeafRefusal(recorded, headerOnly); err != nil {
		t.Errorf("the leaf check on a file holding no entry = %v, want none: the file selects nothing", err)
	}
	empty := Conversation{Source: SourceIdentity{Provider: "pi", SourcePath: metadataOnly}}
	if _, err := piSourceLeafRefusal(empty, metadataOnly); err != nil {
		t.Errorf("the leaf check on a read holding no record = %v, want none: the file's own entries are "+
			"all outside the conversation's context, so there is no leaf to resume wrongly and no context "+
			"the loader would build", err)
	}
	// A file whose only entry is the harness's own system record is the one empty-read shape with an
	// accounted omission: the record is Pi's prompt and tool-loadout state rather than a turn, the read
	// holds no record because the inherited reader drops the role, and the conversion reports the record
	// it could not carry instead of installing an unaccounted empty session.
	systemOnly := filepath.Join(dir, "system-record-only.jsonl")
	if err := os.WriteFile(systemOnly, []byte(header+"\n"+
		`{"type":"message","id":"aa11bb22","parentId":null,"timestamp":"2026-05-04T09:16:00.000Z","message":{"role":"system","content":"","sections":{"preamble":"You are an expert coding assistant."}}}`+"\n"),
		0o600); err != nil {
		t.Fatalf("writing the system-record-only session: %v", err)
	}
	omitted, err := piSourceLeafRefusal(Conversation{Source: SourceIdentity{Provider: "pi", SourcePath: systemOnly}}, systemOnly)
	if err != nil {
		t.Errorf("the leaf check on a file holding the harness's own system record = %v, want none: the "+
			"record's omission is accounted for and counted", err)
	}
	if omitted != 1 {
		t.Errorf("the leaf check counted %d system record(s), want 1", omitted)
	}
	// Off the file's own leaf path, a record the read did not carry is recorded history rather than
	// context (the same reading the walk makes for a read that holds records): the file's last entry walks
	// to a metadata record, and the `custom_message` beside it is not on that walk.
	offPath := filepath.Join(dir, "dropped-record-off-the-path.jsonl")
	if err := os.WriteFile(offPath, []byte(header+"\n"+
		`{"type":"custom_message","id":"aa11bb22","parentId":null,"timestamp":"2026-05-04T09:16:00.000Z","customType":"note","content":"the archive lock is held"}`+"\n"+
		`{"type":"session_info","id":"bb22cc33","parentId":null,"timestamp":"2026-05-04T09:16:30.000Z","name":"retry cap notes"}`+"\n"),
		0o600); err != nil {
		t.Fatalf("writing the off-path session: %v", err)
	}
	if _, err := piSourceLeafRefusal(empty, offPath); err != nil {
		t.Errorf("the leaf check on a file whose leaf path reaches no dropped record = %v, want none: the "+
			"custom_message beside the leaf is not on the walk the loader builds its context from", err)
	}
}

// TestPiSourceLeafRefusalRefusesADroppedRecordBehindAnEmptyRead pins the empty read's other answer: a read
// that holds no record at all while the file's own entries hold one the harness's own loader builds a
// message from -- a `custom_message`, which the carried Pi parser skips -- is refused rather than
// installed as an empty session, because the read's own empty answer cannot count the dropped record as a
// loss. The file shape is the review's own: a session header and one such entry, on the path its loader
// walks from.
func TestPiSourceLeafRefusalRefusesADroppedRecordBehindAnEmptyRead(t *testing.T) {
	dir := t.TempDir()
	header := `{"type":"session","version":3,"id":"01a1c338-0000-4000-8000-000000000000","timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
	custom := `{"type":"custom_message","id":"f1e2d3c4","parentId":null,"timestamp":"2026-05-04T09:16:00.000Z","customType":"note","content":"the archive lock is held"}`
	t.Run("an entry the loader builds a message from", func(t *testing.T) {
		path := filepath.Join(dir, "custom-message.jsonl")
		if err := os.WriteFile(path, []byte(header+"\n"+custom+"\n"), 0o600); err != nil {
			t.Fatalf("writing the session: %v", err)
		}
		_, err := piSourceLeafRefusal(Conversation{Source: SourceIdentity{Provider: "pi", SourcePath: path}}, path)
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf: the file's own loader builds a "+
				"message from the entry the empty read dropped", err)
		}
		for _, want := range []string{"holds no record at all", "custom_message", "selects the leaf"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %q does not name %q", err, want)
			}
		}
	})
	t.Run("an identity-free file, by its own order", func(t *testing.T) {
		const v1Header = `{"type":"session","version":1,"timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
		const v1Custom = `{"type":"custom_message","timestamp":"2026-05-04T09:16:00.000Z","customType":"note","content":"the archive lock is held"}`
		path := filepath.Join(dir, "custom-message-v1.jsonl")
		if err := os.WriteFile(path, []byte(v1Header+"\n"+v1Custom+"\n"), 0o600); err != nil {
			t.Fatalf("writing the v1 session: %v", err)
		}
		_, err := piSourceLeafRefusal(Conversation{Source: SourceIdentity{Provider: "pi", SourcePath: path}}, path)
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Fatalf("the leaf check = %v, want ErrUnrepresentableLeaf for an identity-free file", err)
		}
		for _, want := range []string{"holds no record at all", "records no entry identities", "custom_message"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %q does not name %q", err, want)
			}
		}
	})
}

// TestContinueIntoPiKeepsABranchedSourcesOwnConversation pins the goal's branch clause for the Pi
// *target*: converting a branched and compacted Pi session into a Pi session must leave the target's own
// loader with the source's conversation, not a flattened one in which an abandoned branch's prompt has
// become an active user turn.
//
// The measurement is the file's own walk: piModelVisibleRoles answers the roles the harness's own loader
// builds from a session file (its `get_state`/`get_messages` answer, re-derived by
// scripts/pi-native-check.py against the installed binary), so requiring the converted file's walk to
// equal the source file's walk is requiring a loader to see the same conversation in both. The writer
// this route shipped before its review -- chaining every entry to the one before it -- answered the
// source with [user assistant assistant] and its own conversion with
// [compactionSummary user assistant assistant]: the abandoned prompt "And the retry cap?" presented as
// live, and the source's own line hidden behind a compaction record that stood off it.
func TestContinueIntoPiKeepsABranchedSourcesOwnConversation(t *testing.T) {
	fixture, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	home := t.TempDir()
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          piHistoryFixtureRef,
		Target:       TargetPi,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing the history fixture into Pi: %v", err)
	}
	installed, err := os.ReadFile(result.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	sourceWalk := piModelVisibleRoles(fixture)
	targetWalk := piModelVisibleRoles(installed)
	if want := []string{"user", "assistant", "assistant"}; !slices.Equal(sourceWalk, want) {
		t.Fatalf("the source fixture's own model-visible history = %v, want the measured %v", sourceWalk, want)
	}
	if !slices.Equal(targetWalk, sourceWalk) {
		t.Errorf("the converted session's model-visible history = %v, while the source file's is %v: the "+
			"target's own loader would read a different conversation than the source holds",
			targetWalk, sourceWalk)
	}
	// The abandoned line travels as recorded history and stays off the walk; the source's own last turn is
	// on it. A conversion that dropped the branch fails the first half and one that flattened it the
	// second.
	lines := decodePiSession(t, installed)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	walkTexts := map[string]bool{}
	for id := range onPath {
		walkTexts[texts[id]] = true
	}
	for _, abandoned := range []string{"And the retry cap?", "Thirty seconds, in internal/retry/config.go."} {
		if !strings.Contains(string(installed), abandoned) {
			t.Errorf("the abandoned branch's %q is not in the converted file at all: the branch is "+
				"recorded history and must travel", abandoned)
		}
		if walkTexts[abandoned] {
			t.Errorf("the abandoned branch's %q is on the walk the target's loader makes, so a turn the "+
				"conversation left was presented as an active one", abandoned)
		}
	}
	if !walkTexts["I should not have answered with only the height."] {
		t.Error("the source's own last turn is not on the walk the target's loader makes")
	}
	// The compaction entry the source recorded *off* its own path stays off the walk in the target too:
	// recorded history, which is what the source's own file holds (its loader rebuilds nothing from it).
	compactions := 0
	for _, line := range lines {
		if line.Type != "compaction" {
			continue
		}
		compactions++
		if onPath[line.ID] {
			t.Error("a compaction entry stands on the walk the target's loader makes, while the source " +
				"recorded it off its own path: the target would rebuild its context from a boundary the " +
				"source's conversation does not stand behind")
		}
	}
	if compactions != 1 {
		t.Errorf("the converted session holds %d compaction entr(ies), want the source's one", compactions)
	}
}

// TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere pins the load check's own boundary: the
// target resolves a session id through the process's own working directory, so a session whose recorded
// directory does not exist on this machine cannot be loaded here -- and the check says that instead of
// reading the target's fork offer as a pass.
//
// The working directory is one this case owns and never creates, and the harness's executable lookup is
// answered by the test's own seam, so the refusal is exercised on every host: the case decides when it
// runs, not whether the host happens to have Pi installed or happens to have the fixture's own recorded
// directory (/home/user/code/example-api) absent.
func TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere(t *testing.T) {
	withFakePiHarness(t, nil, nil)
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
	missing := filepath.Join(t.TempDir(), "a-directory-this-host-does-not-have")
	if _, err := os.Stat(missing); err == nil {
		t.Fatalf("%s exists, so this case cannot exercise the refusal it is about", missing)
	}
	load := VerifyPiLoad(context.Background(), PiLoadOptions{
		SessionID: result.TargetSession.SessionID,
		Cwd:       missing,
		Directory: PiProjectDir(missing),
		FileName:  result.TargetSession.FileName,
		Data:      []byte{},
		Installed: result.TargetSession.Path,
		Stderr:    os.Stderr,
	})
	if load.State != VerificationStateNotVerified {
		t.Fatalf("load = %+v, want not-verified for a working directory this host does not have", load)
	}
	if !strings.Contains(load.Detail, "working directory") || !strings.Contains(load.Detail, missing) {
		t.Errorf("load detail = %q, want it to name the working directory the harness needs (%s)",
			load.Detail, missing)
	}
}

// TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath pins the eleventh round's second finding.
// The copy's appended turn hangs off the *call's* entry, not off the tool result's, so the file's own leaf
// path is turn → call → …, and the result entry stands beside the turn off that path -- the shape a
// `/tree` switch back to the call's entry leaves. The inherited reader answers the turn's parent as the
// call (a result's descendants resolve to their visible ancestor), so the read alone cannot tell that
// shape from a file whose turn hangs off the result, and the read's own reconstruction chains through the
// result and marks it on the read's line. A window ending there would therefore pass the tree writers'
// leaf guard and write a target session resumed at a branch the source's file left. The source file's own
// leaf path settles it for every record the read answered an identity for (sourceFileLine): a tree target
// refuses the window by name, a line target omits the record and counts it, and a window ending on the
// file's own path is written -- while the writers follow the file's own relation for a record the file
// placed at the call, so the resumed context is the source's own walk. A run without a selection is
// untouched: its line is the reconstruction's, projection included (goal 0009's D4).
func TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath(t *testing.T) {
	// The appended turn: an ordinary assistant record whose parentId is the call's entry b3c4d5e6.
	const laterTurn = `{"id":"aabbccdd","message":{"api":"openai-completions","content":[{"text":"And the retry budget?","type":"text"}],"model":"example-model-2","provider":"example-proxy","role":"assistant","stopReason":"stop","timestamp":1794938120000,"usage":{"cacheRead":0,"cacheWrite":0,"cost":{"cacheRead":0,"cacheWrite":0,"input":0.0000412,"output":0.0000038,"total":0.000045,"totalFromProvider":false},"input":412,"output":38,"reasoning":0,"totalTokens":450}},"parentId":"b3c4d5e6","timestamp":"2026-05-04T09:15:20.000Z","type":"message"}`
	const (
		resultText  = "internal/retry/config.go:41: maxBackoffSeconds = 30"
		resultEntry = "c7d8e9fa"
	)
	run := func(t *testing.T, target, spec string, supplied bool) (ContinueResult, error) {
		t.Helper()
		cfg, ref, _ := piSourceCopy(t, piFixtureRef.ID, laterTurn)
		return Continue(context.Background(), ContinueOptions{
			Ref: ref, Target: target, Config: cfg,
			TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
			Selection: spec, SelectionSupplied: supplied,
		})
	}

	// A tree target refuses the finding's window: its last record is the result, which the file's own
	// leaf path does not hold.
	for _, target := range []string{TargetPi, TargetClaude} {
		result, err := run(t, target, "0-2", true)
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Fatalf("%s: --select 0-2 = %v, want ErrUnrepresentableLeaf: the window ends on the tool "+
				"result the source's file ran beside the branch the conversation continued on", target, err)
		}
		for _, want := range []string{resultEntry, "ordinal 2", "the record the entry written last stands for"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: the refusal %q does not name %q", target, err, want)
			}
		}
		if result.TargetSession.Path != "" {
			t.Errorf("%s: a refused conversion reported an installed session at %s", target,
				result.TargetSession.Path)
		}
	}

	// A line target omits the off-line record and counts it, exactly as it does for a branch the read
	// itself answers: the log holds no result text, and the leaf check did not refuse the run (the read's
	// own last record *is* the file's leaf).
	converted, err := run(t, TargetCodex, "0-2", true)
	if err != nil {
		t.Fatalf("codex: --select 0-2: %v", err)
	}
	if got := lossCount(t, converted.Conversion.Losses, "inactive_branch_records").Count; got != 1 {
		t.Errorf("codex: inactive_branch_records = %d, want the one record the file's leaf path leaves off", got)
	}
	for _, warning := range converted.Warnings {
		if strings.Contains(warning, "not established") {
			t.Errorf("codex: warning %q says the leaf was not established, while the file's own leaf path "+
				"was read and supports the read's last record", warning)
		}
	}
	installed, err := os.ReadFile(converted.TargetSession.Path)
	if err != nil {
		t.Fatalf("codex: reading the installed rollout: %v", err)
	}
	if strings.Contains(string(installed), resultText) {
		t.Error("codex: the installed rollout holds the result the file's leaf path leaves off")
	}

	// A window ending on the file's own path is written: the root record's own ordinal carries nothing
	// else, so the window's last record is one the file's walk reaches.
	for _, target := range []string{TargetPi, TargetCodex} {
		if _, err := run(t, target, "0", true); err != nil {
			t.Errorf("%s: --select 0 = %v, want the window written: its last record stands on the "+
				"file's own leaf path", target, err)
		}
	}

	// The whole read, written as the *source's own walk*: the result stays off the walk the target's
	// loader makes from the last entry (the exact tree the file records), while the read's reconstruction
	// would chain the last record through the result. The default path is the other arm: unselected, the
	// projection is taken and the result is on the walk -- the bytes a run without a selection has always
	// written (goal 0009's D4).
	selectedRun, err := run(t, TargetPi, "0-", true)
	if err != nil {
		t.Fatalf("pi: --select 0-: %v", err)
	}
	cfg, ref, sourcePath := piSourceCopy(t, piFixtureRef.ID, laterTurn)
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source copy: %v", err)
	}
	installedBytes, err := os.ReadFile(selectedRun.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, sourceBytes)
	installedWalk := transcriptWalkTexts(t, installedBytes)
	if !slices.Equal(installedWalk, sourceWalk) {
		t.Errorf("pi: the selected session's model-visible history = %q, want the source file's own %q: a "+
			"resumed session must continue the source's own line, not the reconstruction's projection",
			installedWalk, sourceWalk)
	}
	for _, text := range installedWalk {
		if text == resultText {
			t.Error("pi: the selected session's walk holds the result the file's leaf path leaves off")
		}
	}
	if got := lossCount(t, selectedRun.Conversion.Losses, "unresolved_source_parents").Count; got != 0 {
		t.Errorf("pi: --select 0- unresolved_source_parents = %d, want none: the file's own bytes answer "+
			"every carried record's parent (the off-line reply's included -- the file names the result it "+
			"stands under), so no place is a relation this conversion resolved", got)
	}
	unflaggedRun, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("pi: unflagged: %v", err)
	}
	unflaggedBytes, err := os.ReadFile(unflaggedRun.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the unflagged session: %v", err)
	}
	unflaggedWalk := transcriptWalkTexts(t, unflaggedBytes)
	if slices.Equal(unflaggedWalk, sourceWalk) {
		t.Error("pi: the unflagged session's walk equals the source's own: the default path's reconstruction " +
			"is what a run without a selection has always written and must stay untouched")
	}
	if got := lossCount(t, unflaggedRun.Conversion.Losses, "unresolved_source_parents").Count; got != 2 {
		t.Errorf("pi: unflagged unresolved_source_parents = %d, want the two projected relations a run "+
			"without a selection has always counted", got)
	}
}

// TestContinueKeepsTheFilesOwnResultOnASelectedWalk pins the twelfth round's first finding. The fixture
// copy's call carries two results -- the second inserted between the first result and the reply, hanging
// off the call itself, while the reply keeps the file's own `parentId` (the first result). The source
// file's leaf path is therefore reply → first result → call, with the second result beside the walk. The
// writer stores the most recent result entry for a call's carrier record, and the round-11 guard refuses
// that off-path entry while the record being written is one the line holds -- falling back to the call and
// chaining the reply *past* the first result, so the resumed context lost an output the file's own walk
// holds although the report counted it as carried (round 12's first finding). The file's own relation
// settles both places: the reply hangs off the first result's entry, exactly as the file records it, and
// the second result stands beside the walk as recorded history. A run without a selection is untouched
// (goal 0009's D4): its reconstruction takes the projection, so the reply follows the *later* result and
// every result stands on the walk -- the bytes that path has always written.
func TestContinueKeepsTheFilesOwnResultOnASelectedWalk(t *testing.T) {
	const (
		secondResultText = "the second result for the same call"
		firstResultText  = "internal/retry/config.go:41: maxBackoffSeconds = 30"
		replyText        = "It is set in internal/retry/config.go."
		firstResultEntry = "c7d8e9fa"
	)
	// The second result: an ordinary toolResult record for the call the fixture's assistant message makes.
	// Its own parent is that message's entry (b3c4d5e6), so it is a sibling of the first result.
	const secondResult = `{"id":"f00dbeef","message":{"content":[{"text":"the second result for the same call","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938106500,"toolCallId":"call_01Example000000000000000000","toolName":"grep"},"parentId":"b3c4d5e6","timestamp":"2026-05-04T09:15:06.500Z","type":"message"}`
	cfg, ref, sourcePath := piSourceCopyWithInsertion(t, piFixtureRef.ID, `"id":"`+firstResultEntry+`"`, secondResult)
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source copy: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, sourceBytes)
	if !slices.Contains(sourceWalk, firstResultText) || slices.Contains(sourceWalk, secondResultText) {
		t.Fatalf("the source file's own walk = %q, want the first result in it and the second one beside "+
			"it: the fixture copy does not hold the shape under test", sourceWalk)
	}
	selected, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0-", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("pi: --select 0-: %v", err)
	}
	installedBytes, err := os.ReadFile(selected.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	installedWalk := transcriptWalkTexts(t, installedBytes)
	if !slices.Equal(installedWalk, sourceWalk) {
		t.Errorf("pi: the selected session's model-visible history = %q, want the source file's own %q: "+
			"a resumed session must continue the source's own line, with the result the file holds between "+
			"the call and the reply", installedWalk, sourceWalk)
	}
	if !strings.Contains(string(installedBytes), secondResultText) {
		t.Error("pi: the installed session does not hold the second result's text: the file records it " +
			"beside the walk, and a conversion writes that record as the history it is")
	}
	// The reply's own entry names the entry the file names: the native parent relation, not the call the
	// read's projection answers and not the later result the projection step reaches. A conversion that
	// chained the reply elsewhere could still pass a walk comparison if it dropped a result, so the
	// relation itself is read off the written entries.
	lines := decodePiSession(t, installedBytes)
	texts := piEntryTexts(t, lines)
	entryOf := map[string]string{}
	for _, line := range lines {
		if text := texts[line.ID]; text != "" {
			entryOf[text] = line.ID
		}
	}
	replyParent := ""
	for _, line := range lines {
		if texts[line.ID] == replyText && line.ParentID != nil {
			replyParent = *line.ParentID
		}
	}
	if want := entryOf[firstResultText]; replyParent != want {
		t.Errorf("pi: the reply's entry hangs off %q, want the first result's entry %q: the file's own "+
			"relation is the record the reply stands under", replyParent, want)
	}
	if got := lossCount(t, selected.Conversion.Losses, "unresolved_source_parents").Count; got != 0 {
		t.Errorf("pi: --select 0- unresolved_source_parents = %d, want none: the file's own bytes answer "+
			"every carried record's parent, the reply's included", got)
	}
	// The default path keeps the reconstruction's own answer: unselected, the reply follows the later
	// result (the projection step's last write for the call) and both results stand on the walk -- the
	// bytes a run without a selection has always written (goal 0009's D4).
	unflagged, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("pi: unflagged: %v", err)
	}
	unflaggedBytes, err := os.ReadFile(unflagged.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the unflagged session: %v", err)
	}
	unflaggedWalk := transcriptWalkTexts(t, unflaggedBytes)
	if slices.Equal(unflaggedWalk, sourceWalk) {
		t.Error("pi: the unflagged session's walk equals the source's own: the default path's " +
			"reconstruction is what a run without a selection has always written and must stay untouched")
	}
	if !slices.Contains(unflaggedWalk, secondResultText) {
		t.Errorf("pi: the unflagged session's walk = %q, want the later result on it: the reconstruction "+
			"chains the reply through the last result written for the call", unflaggedWalk)
	}
}

// TestContinueKeepsTheFilesOwnResultAcrossAMetadataEntry pins the fifteenth round's first finding. The
// round-12 fixture copy is extended with a `model_change` entry Pi wrote between the first result and the
// reply: the reply's own `parentId` names the metadata entry, the metadata entry names the first result,
// and the file's leaf path therefore runs reply → metadata → first result → call. The inherited reader
// projects the metadata and the result onto the call (a record's parent resolves through the entries the
// read does not carry), so the reply's *read* answer names the call while the file names the metadata
// entry -- an identity the read's message index cannot resolve. Left with the read's answer, the ancestry
// walk's tool-result step reaches the *last* result of that call (the sibling the file ran beside its
// line), the writers' guard refuses that off-line entry, and the reply is chained straight to the call:
// the first result stays in the target file but falls off the walk a resumed session makes. The file's
// own chain settles it: the walk steps through the metadata entry and the reply hangs off the first
// result's entry, with the second result recorded history beside the walk.
func TestContinueKeepsTheFilesOwnResultAcrossAMetadataEntry(t *testing.T) {
	const (
		secondResultText = "the second result for the same call"
		firstResultText  = "internal/retry/config.go:41: maxBackoffSeconds = 30"
		replyText        = "It is set in internal/retry/config.go."
		firstResultEntry = "c7d8e9fa"
		replyEntry       = "2a3b4c5d"
		metadataEntry    = "ab12cd34"
	)
	const secondResult = `{"id":"f00dbeef","message":{"content":[{"text":"the second result for the same call","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938106500,"toolCallId":"call_01Example000000000000000000","toolName":"grep"},"parentId":"b3c4d5e6","timestamp":"2026-05-04T09:15:06.500Z","type":"message"}`
	// The metadata entry Pi writes into the tree: a settings change, carried by no read record, that the
	// reply in the file hangs off.
	const metadataLine = `{"id":"ab12cd34","modelId":"example-model-3","parentId":"c7d8e9fa","provider":"example-proxy","timestamp":"2026-05-04T09:15:06.500Z","type":"model_change"}`
	cfg, ref, sourcePath := piSourceCopyWithInsertion(t, piFixtureRef.ID, `"id":"`+firstResultEntry+`"`, secondResult)
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the fixture copy: %v", err)
	}
	out := []string{}
	inserted, reparented := false, false
	for _, existing := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(existing, `"id":"`+replyEntry+`"`) {
			// The reply names the metadata entry as its parent in the file; the read's projection is what
			// the metadata-bridged shape is about.
			reparented = strings.Contains(existing, `"parentId":"`+firstResultEntry+`"`)
			existing = strings.Replace(existing,
				`"parentId":"`+firstResultEntry+`"`, `"parentId":"`+metadataEntry+`"`, 1)
		}
		out = append(out, existing)
		if !inserted && strings.Contains(existing, `"id":"`+firstResultEntry+`"`) {
			out = append(out, metadataLine)
			inserted = true
		}
	}
	if !inserted || !reparented {
		t.Fatalf("the fixture copy does not hold the shape under test (metadata inserted %v, reply "+
			"reparented %v)", inserted, reparented)
	}
	if err := os.WriteFile(sourcePath, []byte(strings.Join(out, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture copy: %v", err)
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source copy: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, sourceBytes)
	if !slices.Contains(sourceWalk, firstResultText) || slices.Contains(sourceWalk, secondResultText) {
		t.Fatalf("the source file's own walk = %q, want the first result in it and the second one beside "+
			"it: the fixture copy does not hold the shape under test", sourceWalk)
	}
	selected, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0-", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("pi: --select 0-: %v", err)
	}
	installedBytes, err := os.ReadFile(selected.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	installedWalk := transcriptWalkTexts(t, installedBytes)
	if !slices.Equal(installedWalk, sourceWalk) {
		t.Errorf("pi: the selected session's model-visible history = %q, want the source file's own %q: "+
			"a resumed session must continue the source's own line, with the result the file holds "+
			"between the call and the reply, across the metadata entry the file wrote between them",
			installedWalk, sourceWalk)
	}
	if !strings.Contains(string(installedBytes), secondResultText) {
		t.Error("pi: the installed session does not hold the second result's text: the file records it " +
			"beside the walk, and a conversion writes that record as the history it is")
	}
	// The reply's own entry names the first result's entry: the file's own chain, not the call the read's
	// projection answers and not the later result the projection step reaches.
	lines := decodePiSession(t, installedBytes)
	texts := piEntryTexts(t, lines)
	entryOf := map[string]string{}
	for _, line := range lines {
		if text := texts[line.ID]; text != "" {
			entryOf[text] = line.ID
		}
	}
	replyParent := ""
	for _, line := range lines {
		if texts[line.ID] == replyText && line.ParentID != nil {
			replyParent = *line.ParentID
		}
	}
	if want := entryOf[firstResultText]; replyParent != want {
		t.Errorf("pi: the reply's entry hangs off %q, want the first result's entry %q: the file's own "+
			"chain runs through the metadata entry to the result it recorded", replyParent, want)
	}
	if got := lossCount(t, selected.Conversion.Losses, "unresolved_source_parents").Count; got != 0 {
		t.Errorf("pi: --select 0- unresolved_source_parents = %d, want none: the file's own bytes answer "+
			"every carried record's parent, the reply's included", got)
	}
}

// TestContinueKeepsASparseWindowsFileAncestryThroughAnExcludedReply pins the thirteenth round's first
// finding. The round-12 case selects the whole read (0-), so every record the file names is carried and
// the ancestry walk never steps through an excluded record. This case narrows the window to 0,1,5: the
// call at 1 pulls both of its results in as necessities, the intermediate reply (ordinal 4) is excluded,
// and the final reply (ordinal 5) names that excluded reply as its parent in the source file. The
// excluded record's *read* parent is the projection (the message that made the call), so a retained-
// ancestor walk that used the read's answers alone steps from the projection to the later sibling result
// the file ran beside its line -- index 3 in this copy -- and the writers' guard refuses a window the
// file's own chain represents. The file's own parent answers every record whose walk passes through it,
// so the walk follows the file: the final reply hangs off the first result's entry, the second result
// stands beside the walk as recorded history, and no place is counted as a relation this conversion
// resolved. A line target writes the same conversation: the off-line sibling is omitted and counted, and
// the records the file's own walk holds are written in its order.
func TestContinueKeepsASparseWindowsFileAncestryThroughAnExcludedReply(t *testing.T) {
	const (
		secondResultText = "the second result for the same call"
		firstResultText  = "internal/retry/config.go:41: maxBackoffSeconds = 30"
		replyText        = "It is set in internal/retry/config.go."
		finalReplyText   = "The cap is 30 seconds of backoff between attempts."
		firstResultEntry = "c7d8e9fa"
	)
	// The second result: an ordinary toolResult record for the call the fixture's assistant message makes,
	// inserted between the first result and the reply so it stands beside the file's own leaf path.
	const secondResult = `{"id":"f00dbeef","message":{"content":[{"text":"the second result for the same call","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938106500,"toolCallId":"call_01Example000000000000000000","toolName":"grep"},"parentId":"b3c4d5e6","timestamp":"2026-05-04T09:15:06.500Z","type":"message"}`
	cfg, ref, sourcePath := piSourceCopyWithInsertion(t, piFixtureRef.ID, `"id":"`+firstResultEntry+`"`, secondResult)
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source copy: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, sourceBytes)
	if !slices.Contains(sourceWalk, replyText) || !slices.Contains(sourceWalk, firstResultText) ||
		slices.Contains(sourceWalk, secondResultText) {
		t.Fatalf("the source file's own walk = %q, want the reply and the first result in it and the "+
			"second result beside it: the fixture copy does not hold the shape under test", sourceWalk)
	}
	// What the target's own loader must walk: the source's own walk, minus the one record the window
	// excludes (the intermediate reply).
	wantWalk := make([]string, 0, len(sourceWalk)-1)
	for _, text := range sourceWalk {
		if text != replyText {
			wantWalk = append(wantWalk, text)
		}
	}
	if !slices.Contains(wantWalk, finalReplyText) {
		t.Fatalf("the source file's own walk = %q, want the final reply on it", sourceWalk)
	}
	selected, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0,1,5", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("pi: --select 0,1,5: %v", err)
	}
	installedBytes, err := os.ReadFile(selected.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	installedWalk := transcriptWalkTexts(t, installedBytes)
	if !slices.Equal(installedWalk, wantWalk) {
		t.Errorf("pi: the selected session's model-visible history = %q, want the source file's own "+
			"walk without the excluded record %q: the window must keep the file's own ancestry, not the "+
			"projection's", installedWalk, wantWalk)
	}
	// The relation itself, off the written entries: the final reply hangs off the first result's entry
	// (the record the file named through the excluded reply), not off the call and not off the later
	// sibling result.
	lines := decodePiSession(t, installedBytes)
	texts := piEntryTexts(t, lines)
	entryOf := map[string]string{}
	for _, line := range lines {
		if text := texts[line.ID]; text != "" {
			entryOf[text] = line.ID
		}
	}
	finalParent := ""
	for _, line := range lines {
		if texts[line.ID] == finalReplyText && line.ParentID != nil {
			finalParent = *line.ParentID
		}
	}
	if want := entryOf[firstResultText]; finalParent != want {
		t.Errorf("pi: the final reply's entry hangs off %q, want the first result's entry %q: the "+
			"excluded reply's file parent is the record the walk through it must follow", finalParent, want)
	}
	if !strings.Contains(string(installedBytes), secondResultText) {
		t.Error("pi: the installed session does not hold the second result's text: the file records it " +
			"beside the walk, and a conversion writes that record as the history it is")
	}
	if slices.Contains(installedWalk, secondResultText) {
		t.Error("pi: the second result stands on the resumed walk: the file's own leaf path leaves it " +
			"off, and a written record on the walk would be an active turn the source never recorded")
	}
	if got := lossCount(t, selected.Conversion.Losses, "unresolved_source_parents").Count; got != 0 {
		t.Errorf("pi: --select 0,1,5 unresolved_source_parents = %d, want none: the file's own bytes "+
			"answer every record the walk steps through, the excluded reply's included", got)
	}

	// The line target's arm: the same window is one line of records, so the sibling the file's leaf
	// path leaves off cannot travel there -- it is omitted and counted -- and the records the file's
	// own walk holds are written in the file's own order.
	flat, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetCodex, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0,1,5", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("codex: --select 0,1,5: %v", err)
	}
	flatBytes, err := os.ReadFile(flat.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed rollout: %v", err)
	}
	if strings.Contains(string(flatBytes), secondResultText) {
		t.Error("codex: the written rollout holds the sibling the file's leaf path leaves off: a line " +
			"target must omit and count it rather than making it model-visible history")
	}
	if got := lossCount(t, flat.Conversion.Losses, "inactive_branch_records").Count; got != 1 {
		t.Errorf("codex: inactive_branch_records = %d, want 1: the sibling is the one record the read's "+
			"own line leaves off the window", got)
	}
	for _, text := range []string{firstResultText, finalReplyText} {
		if !strings.Contains(string(flatBytes), text) {
			t.Errorf("codex: the written rollout does not hold %q: the records the file's own walk holds "+
				"travel", text)
		}
	}
}

// TestContinueKeepsASparseWindowsFileAncestryThroughAnExcludedResult pins the round-16 first finding:
// the retained file-backed ancestry answer is preferred over the read's projected parent when the direct
// file parent was excluded. The fixture copy appends a second exchange whose file runs call B → call A →
// result R1 for A → result R_B for B → the reply, with a later sibling result R2 for A standing beside
// that path, and a window that selects call A's record and the reply leaves B and R_B out. The read
// answers the reply as hanging off call A's message (a result's descendants resolve to their visible
// ancestor: R_B's own read parent is A), and the writer's tool-result step for A holds the *later*
// sibling result R2 -- off the file's line, so the eleventh round's guard refuses that place. Before the
// fix the writer then took the read's projected parent (A's own entry), which left R1 -- the result the
// file's chain runs through, carried as the call's counterpart -- beside the reply instead of under it:
// present in the file but off the walk a resumed session makes. The file's own chain through the
// excluded R_B reaches R1, and that answer is now read before the projected parent: the reply hangs off
// R1's entry, the sibling stays recorded history beside the walk, and no place is counted as a relation
// this conversion resolved. A run without a selection keeps the reconstruction, whose projection chains
// every result under the call and the reply under the last of them (goal 0009's D4).
func TestContinueKeepsASparseWindowsFileAncestryThroughAnExcludedResult(t *testing.T) {
	const (
		promptText = "Where is the retry cap configured?"
		callBText  = "B's call is on its way."
		callAText  = "A's call is on its way."
		r1Text     = "the result the file's chain runs through"
		r2Text     = "the sibling result the file left beside the walk"
		rbText     = "B's own recorded output"
		replyText  = "The cap is 30 seconds of backoff between attempts, and B's output says so."
	)
	// The appended records: call B hangs off the fixture's prompt, call A off call B, the first result
	// off call A, the sibling result off call A too (later in the file), B's result off the first
	// result, and the reply off B's result -- the file's own leaf path is reply → R_B → R1 → call A →
	// call B → prompt, with the sibling beside it. The prompt's own record is carried (its read parent
	// is empty, so its place is the read's own order and no relation is counted), which makes the
	// window's every other relation resolvable through the file.
	const (
		callB = `{"id":"bbbb0001","message":{"api":"openai-completions","content":[{"text":"B's call is on its way.","type":"text"},{"arguments":{"pattern":"callB"},"id":"call_B_sparse","name":"grep","type":"toolCall"}],"model":"example-model-2","provider":"example-proxy","role":"assistant","stopReason":"toolUse","timestamp":1794938110000},"parentId":"52fd5d0f","timestamp":"2026-05-04T09:16:00.000Z","type":"message"}`
		callA = `{"id":"aaaa0001","message":{"api":"openai-completions","content":[{"text":"A's call is on its way.","type":"text"},{"arguments":{"pattern":"callA"},"id":"call_A_sparse","name":"grep","type":"toolCall"}],"model":"example-model-2","provider":"example-proxy","role":"assistant","stopReason":"toolUse","timestamp":1794938111000},"parentId":"bbbb0001","timestamp":"2026-05-04T09:16:01.000Z","type":"message"}`
		r1    = `{"id":"r1r1r1r1","message":{"content":[{"text":"the result the file's chain runs through","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938112000,"toolCallId":"call_A_sparse","toolName":"grep"},"parentId":"aaaa0001","timestamp":"2026-05-04T09:16:02.000Z","type":"message"}`
		r2    = `{"id":"r2r2r2r2","message":{"content":[{"text":"the sibling result the file left beside the walk","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938112500,"toolCallId":"call_A_sparse","toolName":"grep"},"parentId":"aaaa0001","timestamp":"2026-05-04T09:16:02.500Z","type":"message"}`
		rb    = `{"id":"rbrbrbrb","message":{"content":[{"text":"B's own recorded output","type":"text"}],"isError":false,"role":"toolResult","timestamp":1794938113000,"toolCallId":"call_B_sparse","toolName":"grep"},"parentId":"r1r1r1r1","timestamp":"2026-05-04T09:16:03.000Z","type":"message"}`
		reply = `{"id":"rrrr0001","message":{"api":"openai-completions","content":[{"text":"The cap is 30 seconds of backoff between attempts, and B's output says so.","type":"text"}],"model":"example-model-2","provider":"example-proxy","role":"assistant","stopReason":"stop","timestamp":1794938114000},"parentId":"rbrbrbrb","timestamp":"2026-05-04T09:16:04.000Z","type":"message"}`
	)
	cfg, ref, sourcePath := piSourceCopy(t, piFixtureRef.ID, callB, callA, r1, r2, rb, reply)
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source copy: %v", err)
	}
	sourceWalk := transcriptWalkTexts(t, sourceBytes)
	if !slices.Contains(sourceWalk, rbText) || slices.Contains(sourceWalk, r2Text) {
		t.Fatalf("the source file's own walk = %q, want B's result on it and the sibling beside it: the "+
			"fixture copy does not hold the shape under test", sourceWalk)
	}
	// The window the finding names: call A's record and the reply. Its ordinals are asserted from the
	// read rather than assumed, so a fixture whose ordinals moved fails here rather than converting a
	// different window.
	read, err := NewSourceReader(cfg).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading the appended fixture: %v", err)
	}
	window, report, err := Select(read, "0,6,10", false)
	if err != nil {
		t.Fatalf("selecting 0,6,10: %v", err)
	}
	if got := selectionOrdinals(window); !slices.Equal(got, []int{0, 6, 7, 8, 10}) {
		t.Fatalf("the window carries ordinals %v, want the prompt, call A's record and the reply "+
			"(0, 6, 10) plus the call's two results pulled in as necessities (7, 8): the appended fixture "+
			"does not hold the shape under test", got)
	}
	if report.NecessitiesAdded.Count != 2 {
		t.Fatalf("necessities = %+v, want the call's two recorded results carried with it",
			report.NecessitiesAdded)
	}

	// The tree target: the reply hangs off R1's entry -- the result the file's own chain runs through --
	// and R1's text stands on the resumed walk while the sibling stays beside it as recorded history.
	selectedRun, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0,6,10", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("pi: --select 0,6,10: %v", err)
	}
	if selectedRun.Verification.Structure.State != VerificationStateVerified {
		t.Fatalf("pi: structure = %+v, want the installed session read back as what was written",
			selectedRun.Verification.Structure)
	}
	installedBytes, err := os.ReadFile(selectedRun.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	installedWalk := transcriptWalkTexts(t, installedBytes)
	wantWalk := []string{promptText, callAText, r1Text, replyText}
	if !slices.Equal(installedWalk, wantWalk) {
		t.Errorf("pi: the selected session's model-visible history = %q, want %q: the reply must hang "+
			"off the result the file's chain ran through, not off the projected call", installedWalk, wantWalk)
	}
	lines := decodePiSession(t, installedBytes)
	texts := piEntryTexts(t, lines)
	entryOf := map[string]string{}
	for _, line := range lines {
		if text := texts[line.ID]; text != "" {
			entryOf[text] = line.ID
		}
	}
	replyParent := ""
	for _, line := range lines {
		if texts[line.ID] == replyText && line.ParentID != nil {
			replyParent = *line.ParentID
		}
	}
	if want := entryOf[r1Text]; replyParent != want {
		t.Errorf("pi: the reply's entry hangs off %q, want the first result's entry %q: the file's own "+
			"chain through the excluded result reaches the result the window carries", replyParent, want)
	}
	if !strings.Contains(string(installedBytes), r2Text) {
		t.Error("pi: the installed session does not hold the sibling result's text: the file records it " +
			"beside the walk, and a conversion writes that record as the history it is")
	}
	if slices.Contains(installedWalk, r2Text) {
		t.Error("pi: the sibling result stands on the resumed walk: the file's own leaf path leaves it " +
			"off, and a written record on the walk would be an active turn the source never recorded")
	}
	if got := lossCount(t, selectedRun.Conversion.Losses, "unresolved_source_parents").Count; got != 0 {
		t.Errorf("pi: --select 0,6,10 unresolved_source_parents = %d, want none: the file's own bytes "+
			"answer every relation the window's walk steps through, the excluded result's included, and "+
			"the retained-ancestor answer the reply takes is the file's own, not a relation this "+
			"conversion resolved", got)
	}

	// The line target omits the sibling -- its one line cannot hold a record the file's leaf path leaves
	// off -- and counts it, while the records the file's own walk holds travel.
	flat, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetCodex, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
		Selection: "0,6,10", SelectionSupplied: true,
	})
	if err != nil {
		t.Fatalf("codex: --select 0,6,10: %v", err)
	}
	flatBytes, err := os.ReadFile(flat.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed rollout: %v", err)
	}
	if strings.Contains(string(flatBytes), r2Text) {
		t.Error("codex: the written rollout holds the sibling the file's leaf path leaves off: a line " +
			"target must omit and count it rather than making it model-visible history")
	}
	if got := lossCount(t, flat.Conversion.Losses, "inactive_branch_records").Count; got != 1 {
		t.Errorf("codex: inactive_branch_records = %d, want 1: the sibling is the one record the read's "+
			"own line leaves off the window", got)
	}
	for _, text := range []string{promptText, callAText, r1Text, replyText} {
		if !strings.Contains(string(flatBytes), text) {
			t.Errorf("codex: the written rollout does not hold %q: the records the file's own walk holds "+
				"travel", text)
		}
	}

	// The default path keeps the reconstruction, which is what a run without a selection has always
	// written (goal 0009's D4): its projection chains every result under the call's message and the reply
	// under the last result written, so the sibling stands on the reconstruction's walk and the
	// projection steps are counted as the relations they are.
	unflagged, err := Continue(context.Background(), ContinueOptions{
		Ref: ref, Target: TargetPi, Config: cfg,
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("pi: unflagged: %v", err)
	}
	unflaggedBytes, err := os.ReadFile(unflagged.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the unflagged session: %v", err)
	}
	unflaggedWalk := transcriptWalkTexts(t, unflaggedBytes)
	if !slices.Contains(unflaggedWalk, r2Text) || !slices.Contains(unflaggedWalk, rbText) {
		t.Errorf("pi: the unflagged session's walk = %q, want the reconstruction's projection: every "+
			"result chained under the call and the reply under the last of them", unflaggedWalk)
	}
	if got := lossCount(t, unflagged.Conversion.Losses, "unresolved_source_parents").Count; got != 4 {
		t.Errorf("pi: unflagged unresolved_source_parents = %d, want the four projected relations the "+
			"reconstruction has always counted (the fixture's intermediate reply, the sibling result, "+
			"B's result and the reply -- each chained through the result the projection step reaches)", got)
	}
}

// TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities pins the boundary of the source file's
// own line answer: the membership is per *identity*, and a file that records none answers no tree at all
// -- a Pi v1 session's own order is its conversation, and the read's order is that order -- so every
// record the read gave no identity for keeps the read's own order answer. The selection's window ending
// on the file's second message is therefore written for a tree target and a line target alike, exactly as
// before the file's leaf path was consulted; answering an identity-less record as off the file's path
// (the mutation this case exists to refuse) would refuse the window and omit the record from the log.
func TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities(t *testing.T) {
	const (
		header = `{"type":"session","version":1,"timestamp":"2026-05-04T09:15:00.000Z","cwd":"/home/user/code/example-api"}`
		prompt = `{"type":"message","timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"user","content":"Where is the retry cap configured?"}}`
		answer = `{"type":"message","timestamp":"2026-05-04T09:15:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"It is set in internal/retry/config.go."}],"usage":{"input":412,"output":38,"totalTokens":450}}}`
	)
	dir := t.TempDir()
	path := filepath.Join(dir, "v1-session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join([]string{header, prompt, answer}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the v1 session: %v", err)
	}
	for _, arm := range []struct {
		target string
		spec   string
		lost   int
	}{
		{TargetPi, "1", 0},
		{TargetCodex, "1", 0},
	} {
		cfg := fixtureConfig(t)
		cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentPi: {dir}}
		result, err := Continue(context.Background(), ContinueOptions{
			Ref: Ref{Provider: "pi", ID: path}, Target: arm.target, Config: cfg,
			TargetHome: t.TempDir(), TargetBinary: missingTargetBinary,
			Selection: arm.spec, SelectionSupplied: true,
		})
		if err != nil {
			t.Fatalf("%s: --select %s: %v", arm.target, arm.spec, err)
		}
		if result.Conversion.State != ConversionStateConverted {
			t.Errorf("%s: conversion state = %q, want the window converted", arm.target, result.Conversion.State)
		}
		for _, loss := range result.Conversion.Losses {
			if loss.Category == "inactive_branch_records" || loss.Category == "unresolved_source_parents" {
				if loss.Count != arm.lost {
					t.Errorf("%s: %s = %d, want %d: an identity-less record's place is the read's own "+
						"order, and the file records no tree to answer another", arm.target, loss.Category,
						loss.Count, arm.lost)
				}
			}
		}
		installed, err := os.ReadFile(result.TargetSession.Path)
		if err != nil {
			t.Fatalf("%s: reading the installed session: %v", arm.target, err)
		}
		if !strings.Contains(string(installed), "It is set in internal/retry/config.go.") {
			t.Errorf("%s: the written session does not hold the record the window selected: the file "+
				"records no identities, so the read's own order is the line", arm.target)
		}
	}
}
