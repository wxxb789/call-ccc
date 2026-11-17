package ccc

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestClaudeVersionFrom pins the version line the probe reads: the harness answers `2.1.326 (Claude
// Code)`, and the report records the version itself, not the product name beside it.
func TestClaudeVersionFrom(t *testing.T) {
	for _, testCase := range []struct{ output, want string }{
		{"2.1.326 (Claude Code)\n", "2.1.326"},
		{"1.0.0", "1.0.0"},
		{"Claude Code 2.1.326\n", "2.1.326"},
		{"no version here", ""},
		{"", ""},
	} {
		if got := claudeVersionFrom(testCase.output); got != testCase.want {
			t.Errorf("claudeVersionFrom(%q) = %q, want %q", testCase.output, got, testCase.want)
		}
	}
}

// TestProbeClaudeBinaryReportsAMissingBinary pins the answer a host without the harness gets: an answer,
// not an error, with the reason named, so a conversion can still be written and verified structurally.
func TestProbeClaudeBinaryReportsAMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	probe := ProbeClaudeBinary(context.Background(), "claude")
	if probe.Found {
		t.Fatalf("probe = %+v, want it not found on an empty PATH", probe)
	}
	if !strings.Contains(probe.Detail, "PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", probe.Detail)
	}
}

// TestClaudeLoadEnvironmentRemovesEveryModelPath pins what the offline load check hands its harness: the
// data directory it must read, and none of the variables that could reach a model. The documented set is
// written out here as literals: a case that looped over the check's own list could not fail for a name
// the list is missing, and that is the failure this contract has to catch. One name is also placed under
// a second spelling, because the environment it is removed from is case-insensitive on Windows: the
// scan below reads every name the built environment carries in its own upper case, so a spelling the
// check forgot to fold is red here rather than silently handed to the harness there.
func TestClaudeLoadEnvironmentRemovesEveryModelPath(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-secret")
	t.Setenv("ANTHROPIC_BASE_URL", "https://example.invalid")
	t.Setenv("CLAUDE_CONFIG_DIR", "/somewhere/else")
	t.Setenv("UNRELATED_SETTING", "kept")
	documented := []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_SMALL_FAST_MODEL",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
	}
	if !slices.Equal(claudeLoadEnvDenied, documented) {
		t.Errorf("claudeLoadEnvDenied = %v, want the harness's documented model paths %v",
			claudeLoadEnvDenied, documented)
	}
	t.Setenv(strings.ToLower(documented[0]), "sk-also-secret")
	environment := claudeEnvironment("/scratch/home", claudeLoadEnvDenied)
	joined := strings.Join(environment, "\n")
	denied := make(map[string]struct{}, len(documented))
	for _, name := range documented {
		denied[strings.ToUpper(name)] = struct{}{}
	}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, still := denied[strings.ToUpper(name)]; still {
			t.Errorf("the environment still carries %s, whatever its spelling", entry)
		}
	}
	if !strings.Contains(joined, claudeHomeEnv+"=/scratch/home") {
		t.Errorf("the environment does not select the check's own data directory: %v", environment)
	}
	if !strings.Contains(joined, "UNRELATED_SETTING=kept") {
		t.Error("the check dropped a variable it does not own")
	}
}

// TestClaudeLoadEnvironmentReplacesEverySpellingOfTheHomeVariable pins the other half of the same rule,
// and the failure it prevents is the opposite one: the home variable must be *replaced*, not merely
// left alone. Windows' environment is case-insensitive and os/exec keeps the last value among the names
// it is handed, and the built list is sorted -- so a lower- or mixed-case spelling of the variable left
// in place sorts after the appended override and wins there, and the harness reads the caller's own home
// instead of the directory the check installed into. Exactly one spelling may survive, carrying the
// check's own home; the alternate spelling is set here rather than assumed to exist in the caller's
// environment.
func TestClaudeLoadEnvironmentReplacesEverySpellingOfTheHomeVariable(t *testing.T) {
	t.Setenv(strings.ToLower(claudeHomeEnv), "/the-callers-own-home")
	environment := claudeEnvironment("/scratch/home", claudeLoadEnvDenied)
	var selected []string
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found && strings.ToUpper(name) == claudeHomeEnv {
			selected = append(selected, value)
		}
	}
	if len(selected) != 1 || selected[0] != "/scratch/home" {
		t.Errorf("the environment selects %d spelling(s) of %s (%v), want exactly the check's own "+
			"data directory", len(selected), claudeHomeEnv, selected)
	}
}

// fakeHarness stands in for the target harness in the two verification tests below: it answers the load
// check's two arms and the resume turn the way the installed harness was measured to, and it records what
// it was asked, so the test can assert both the check's verdict and what the check really ran.
type fakeHarness struct {
	// answerSessionID is the id the first load arm (and the resume turn) reports; a different value
	// exercises a loader answering about something other than the installed session.
	answerSessionID string
	// controlAnswersJSON makes the control arm answer with a result document instead of the not-found
	// error, which must not verify.
	controlAnswersJSON bool
	// resumeResult and resumeIsError shape the resume turn's answer document.
	resumeResult  string
	resumeIsError bool
	// growSession appends a record to the session file the turn was told to run in, the way the harness
	// writes its own bookkeeping records into a session it resumes.
	growSession bool
	// resolvedExecutable is the path the checks' executable lookup answers with while this fake is
	// installed, so both checks can be driven on a host that has no harness on PATH; each call records
	// the path it was really handed.
	resolvedExecutable string
	// calls records each arm: the arguments and the environment it was given.
	calls []fakeHarnessCall
}

type fakeHarnessCall struct {
	executable  string
	arguments   []string
	environment []string
	home        string
	cwd         string
}

func (h *fakeHarness) run(
	_ context.Context, executable string, opts ClaudeResumeOptions, _ time.Duration,
	args []string, environment []string, _ io.Writer,
) (string, string, error) {
	home := ""
	for _, entry := range environment {
		if strings.HasPrefix(entry, claudeHomeEnv+"=") {
			home = strings.TrimPrefix(entry, claudeHomeEnv+"=")
		}
	}
	h.calls = append(h.calls, fakeHarnessCall{
		executable: executable, arguments: args, environment: environment, home: home, cwd: opts.Cwd,
	})
	sessionID := ""
	if index := slices.Index(args, "--resume"); index >= 0 && index+1 < len(args) {
		sessionID = args[index+1]
	}
	if sessionID != h.answerSessionID {
		if h.controlAnswersJSON {
			// The shape a loader answering a session that was never installed has.
			return harnessDocument(sessionID, false, "an answer for a session nobody installed"), "", nil
		}
		// The measured shape of the not-found arm: no result document at all, and the loader's own words
		// on stderr. The arm exits non-zero, which the check treats as part of that answer.
		return "", "No conversation found with session ID: " + sessionID + "\n", os.ErrProcessDone
	}
	if h.growSession && opts.SessionPath != "" {
		if file, err := os.OpenFile(opts.SessionPath, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			file.WriteString("{\"type\":\"cost-state\",\"sessionId\":\"" + sessionID + "\"}\n")
			file.Close()
		}
	}
	if opts.Prompt == "" {
		// The load probe: measured, the harness resolves the session and then reports the model call it
		// could not make, with a non-zero exit status.
		return harnessDocument(sessionID, true, "Not logged in · Please run /login"), "", os.ErrProcessDone
	}
	return harnessDocument(sessionID, h.resumeIsError, h.resumeResult), "", nil
}

// harnessDocument is one result document in the shape the harness was measured to write.
func harnessDocument(sessionID string, isError bool, result string) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "session_id": sessionID,
		"is_error": isError, "result": result, "terminal_reason": "api_error", "num_turns": 1,
	})
	return string(encoded) + "\n"
}

// withFakeHarness installs one fake harness for the duration of a test: the turn seam both checks drive a
// harness through is answered by the fake, and the executable lookup they find the harness with is
// answered by a path of the test's own -- what these tests pin is what each check makes of the harness's
// answer, not whether this host has Claude Code installed. The missing-binary answers keep their own
// coverage where no fake is installed and the real lookup answers: TestProbeClaudeBinaryReportsAMissingBinary
// and TestVerifyClaudeResumeRefusesWithoutABinary.
func withFakeHarness(t *testing.T, harness *fakeHarness) {
	t.Helper()
	originalTurn := runClaudeHarnessTurn
	runClaudeHarnessTurn = func(
		ctx context.Context, executable string, opts ClaudeResumeOptions, timeout time.Duration,
		args []string, environment []string, stderr io.Writer,
	) (string, string, error) {
		return harness.run(ctx, executable, opts, timeout, args, environment, stderr)
	}
	t.Cleanup(func() { runClaudeHarnessTurn = originalTurn })
	harness.resolvedExecutable = filepath.Join(t.TempDir(), ClaudeBinaryDefault)
	originalLookup := resolveClaudeExecutable
	resolveClaudeExecutable = func(string) (string, error) { return harness.resolvedExecutable, nil }
	t.Cleanup(func() { resolveClaudeExecutable = originalLookup })
}

// installedClaudeSession writes a converted transcript into a fresh home and answers its path.
func installedClaudeSession(t *testing.T) (string, ClaudeTranscript, string) {
	t.Helper()
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
	return home, transcript, path
}

// TestVerifyClaudeLoadResolvesTheInstalledSessionAndItsControl pins the load check's rule: the harness's
// own loader must name the installed session, the same command for an unknown id must report no such
// conversation, the loader must be driven against a throw-away copy (never the installed file, which the
// harness writes into), and the installed file must be untouched afterwards.
func TestVerifyClaudeLoadResolvesTheInstalledSessionAndItsControl(t *testing.T) {
	home, transcript, installed := installedClaudeSession(t)
	before := hashFile(t, installed)
	harness := &fakeHarness{answerSessionID: transcript.SessionID}
	withFakeHarness(t, harness)

	verification := VerifyClaudeLoad(context.Background(), ClaudeLoadOptions{
		Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		Directory: transcript.Directory, FileName: transcript.FileName, Data: transcript.Data,
		Installed: installed, Timeout: time.Minute,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("load = %+v, want a verified load", verification)
	}
	if len(harness.calls) != 2 {
		t.Fatalf("the check made %d call(s), want the probe and its control", len(harness.calls))
	}
	for index, call := range harness.calls {
		if slices.Index(call.arguments, "--bare") < 0 {
			t.Errorf("arm %d did not run with --bare, so it cannot be offline: %v", index, call.arguments)
		}
		if call.home == home {
			t.Errorf("arm %d drove the loader against the installed data directory rather than a copy (%s)", index, call.home)
		}
		if call.executable != harness.resolvedExecutable {
			t.Errorf("arm %d ran %q, want the path the check resolved (%q)",
				index, call.executable, harness.resolvedExecutable)
		}
		if call.cwd != transcript.Cwd {
			t.Errorf("arm %d ran in %q, want the session's own working directory %q", index, call.cwd, transcript.Cwd)
		}
		for _, entry := range call.environment {
			if strings.HasPrefix(entry, "ANTHROPIC_API_KEY=") || strings.HasPrefix(entry, "ANTHROPIC_AUTH_TOKEN=") {
				t.Errorf("arm %d carried model credentials: %s", index, entry)
			}
		}
	}
	if harness.calls[0].arguments[slices.Index(harness.calls[0].arguments, "--resume")+1] != transcript.SessionID {
		t.Error("the first arm did not ask the loader for the installed session")
	}
	if harness.calls[1].arguments[slices.Index(harness.calls[1].arguments, "--resume")+1] == transcript.SessionID {
		t.Error("the control arm asked for the installed session, so it is not a control")
	}
	if !strings.Contains(verification.Detail, "No conversation found") {
		t.Errorf("detail = %q, want the control arm's own answer quoted", verification.Detail)
	}
	if digest := hashFile(t, installed); digest != before {
		t.Error("the load check altered the installed session")
	}
}

// TestVerifyClaudeLoadRefusesAnAnswerAboutAnotherSession pins what makes the first arm evidence: a loader
// that answers about a different session -- or answers at all for an id nothing installed -- has not shown
// that this session resolves.
func TestVerifyClaudeLoadRefusesAnAnswerAboutAnotherSession(t *testing.T) {
	_, transcript, installed := installedClaudeSession(t)
	other := &fakeHarness{answerSessionID: transcript.SessionID, controlAnswersJSON: true}
	withFakeHarness(t, other)
	verification := VerifyClaudeLoad(context.Background(), ClaudeLoadOptions{
		Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		Directory: transcript.Directory, FileName: transcript.FileName, Data: transcript.Data,
		Installed: installed,
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("load = %+v, want it refused: the control arm answered a session that was never installed", verification)
	}
	if !strings.Contains(verification.Detail, "control arm") {
		t.Errorf("detail = %q, want it to name the control arm", verification.Detail)
	}

	// A loader answering about a session nobody installed: its answer is not evidence about this one.
	answeringAnotherSession := &fakeHarness{answerSessionID: "99999999-9999-4999-8999-999999999999"}
	withFakeHarness(t, answeringAnotherSession)
	verification = VerifyClaudeLoad(context.Background(), ClaudeLoadOptions{
		Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		Directory: transcript.Directory, FileName: transcript.FileName, Data: transcript.Data,
		Installed: installed,
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("load = %+v, want it refused: the loader answered about another session", verification)
	}
}

// TestVerifyClaudeResumeReportsTheTurnAndTheGrowth pins the resume level's report: the harness's own
// answer, the exit path, and the session file's growth, with "verified" claimed only when a turn really
// produced an answer.
func TestVerifyClaudeResumeReportsTheTurnAndTheGrowth(t *testing.T) {
	home, transcript, installed := installedClaudeSession(t)
	harness := &fakeHarness{answerSessionID: transcript.SessionID, resumeResult: "Four attempts, in internal/retry/policy.go.", growSession: true}
	withFakeHarness(t, harness)

	verification := VerifyClaudeResume(context.Background(), ClaudeResumeOptions{
		Home: home, Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		SessionPath: installed, Prompt: "How many attempts?",
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("resume = %+v, want a verified turn", verification)
	}
	if !strings.Contains(verification.Output, "Four attempts") {
		t.Errorf("output = %q, want the harness's own answer", verification.Output)
	}
	if verification.SessionBytesAfter <= verification.SessionBytesBefore {
		t.Errorf("the report does not show the session growing (%d -> %d)",
			verification.SessionBytesBefore, verification.SessionBytesAfter)
	}
	if !strings.Contains(verification.Command, "cd "+transcript.Cwd) {
		t.Errorf("command = %q, want the session's working directory in it", verification.Command)
	}
	if len(harness.calls) != 1 || slices.Index(harness.calls[0].arguments, "--bare") >= 0 {
		t.Errorf("the resume turn ran %+v, want the caller's own environment and no --bare", harness.calls)
	}
	if harness.calls[0].executable != harness.resolvedExecutable {
		t.Errorf("the resume turn ran %q, want the path the check resolved (%q)",
			harness.calls[0].executable, harness.resolvedExecutable)
	}
	if !strings.Contains(strings.Join(harness.calls[0].arguments, " "), "How many attempts?") {
		t.Error("the resume turn did not carry the caller's prompt")
	}

	failing := &fakeHarness{answerSessionID: transcript.SessionID, resumeResult: "Not logged in · Please run /login", resumeIsError: true}
	withFakeHarness(t, failing)
	verification = VerifyClaudeResume(context.Background(), ClaudeResumeOptions{
		Home: home, Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		SessionPath: installed, Prompt: "How many attempts?",
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it refused: the harness answered that it did not run", verification)
	}
	if !strings.Contains(verification.Detail, "Not logged in") {
		t.Errorf("detail = %q, want the harness's own reason", verification.Detail)
	}

	// A harness that answers in one turn but appends nothing to the installed session: an answer alone
	// is not evidence that the turn continues the installed session, so the state is refused and the
	// detail names the sizes that show nothing was written.
	nonGrowing := &fakeHarness{answerSessionID: transcript.SessionID, resumeResult: "Four attempts, in internal/retry/policy.go."}
	withFakeHarness(t, nonGrowing)
	verification = VerifyClaudeResume(context.Background(), ClaudeResumeOptions{
		Home: home, Binary: "claude", SessionID: transcript.SessionID, Cwd: transcript.Cwd,
		SessionPath: installed, Prompt: "How many attempts?",
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it refused: the session file did not grow", verification)
	}
	if !strings.Contains(verification.Detail, "did not grow") {
		t.Errorf("detail = %q, want it to name the session file that did not grow", verification.Detail)
	}
	if verification.SessionBytesAfter != verification.SessionBytesBefore {
		t.Errorf("the report shows the session growing (%d -> %d) although the turn wrote nothing",
			verification.SessionBytesBefore, verification.SessionBytesAfter)
	}
}

// TestVerifyClaudeResumeRefusesAPromptItWouldHaveToInvent pins the rule the resume level carries: the
// turn becomes part of the caller's session, so call/ccc never writes a prompt of its own.
func TestVerifyClaudeResumeRefusesAPromptItWouldHaveToInvent(t *testing.T) {
	verification := VerifyClaudeResume(context.Background(), ClaudeResumeOptions{
		Home: t.TempDir(), Binary: "claude", SessionID: "11111111-1111-4111-8111-111111111111",
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it refused", verification)
	}
	if !strings.Contains(verification.Detail, "does not invent one") {
		t.Errorf("detail = %q, want it to say no prompt was given", verification.Detail)
	}
}

// TestVerifyClaudeResumeRefusesWithoutABinary pins the missing-binary answer: it is reported as a reason,
// never as a pass and never as a panic.
func TestVerifyClaudeResumeRefusesWithoutABinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	verification := VerifyClaudeResume(context.Background(), ClaudeResumeOptions{
		Home: t.TempDir(), Binary: "claude", SessionID: "11111111-1111-4111-8111-111111111111",
		SessionPath: filepath.Join(t.TempDir(), "session.jsonl"), Prompt: "how many attempts?",
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it not verified", verification)
	}
	if !strings.Contains(verification.Detail, "PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", verification.Detail)
	}
}
