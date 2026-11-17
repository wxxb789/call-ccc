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
// model's: a Pi-specific branch would produce a different carried report or a different read-back here.
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
			t.Errorf("the pipeline read back %+v, while the target's own writer over the same source read "+
				"back %+v: a Pi-specific shortcut is not the shared model's answer",
				result.Verification.Structure.ReadBack, directVerify.ReadBack)
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
			t.Errorf("the pipeline read back %+v, while the target's own writer over the same source read "+
				"back %+v: a Pi-specific shortcut is not the shared model's answer",
				result.Verification.Structure.ReadBack, directVerify.ReadBack)
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
