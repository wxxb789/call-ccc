package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestPiVersionFrom pins the version line the probe reads: the harness answers `1.5.2`, and the report
// records the version itself rather than the product name beside it.
func TestPiVersionFrom(t *testing.T) {
	for _, testCase := range []struct{ output, want string }{
		{"1.5.2 (pi)\n", "1.5.2"},
		{"1.0.0", "1.0.0"},
		{"pi 1.5.2\n", "1.5.2"},
		{"no version here", ""},
		{"", ""},
	} {
		if got := piVersionFrom(testCase.output); got != testCase.want {
			t.Errorf("piVersionFrom(%q) = %q, want %q", testCase.output, got, testCase.want)
		}
	}
}

// TestProbePiBinaryReportsAMissingBinary pins the answer a host without the harness gets: an answer, not
// an error, with the reason named, so a conversion can still be written and verified structurally.
func TestProbePiBinaryReportsAMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	probe := ProbePiBinary(context.Background(), "pi")
	if probe.Found {
		t.Fatalf("probe = %+v, want it not found on an empty PATH", probe)
	}
	if !strings.Contains(probe.Detail, "PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", probe.Detail)
	}
}

// TestPiLoadEnvironmentRemovesEveryModelPath pins what the offline load check hands its harness: the
// data directory it must read, and none of the variables that could reach a model. The documented set is
// written out here as literals -- a case that looped over the check's own list could not fail for a name
// the list is missing, and that is the failure this contract has to catch. One name is also placed under
// a second spelling, because the environment it is removed from is case-insensitive on Windows: the scan
// below reads every name the built environment carries in its own upper case, so a spelling the check
// forgot to fold is red here rather than silently handed to the harness there.
func TestPiLoadEnvironmentRemovesEveryModelPath(t *testing.T) {
	documented := []string{
		"PI_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"XAI_API_KEY",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AZURE_OPENAI_API_KEY",
	}
	if !slices.Equal(piLoadEnvDenied, documented) {
		t.Errorf("piLoadEnvDenied = %v, want the harness's documented model paths %v",
			piLoadEnvDenied, documented)
	}
	for _, name := range documented {
		t.Setenv(name, "a-model-credential")
	}
	// A second spelling of one denied name, and the two directory variables the check owns: the home is
	// replaced, and the sessions tree is removed so no leftover setting can point the harness elsewhere.
	t.Setenv(strings.ToLower(documented[1]), "a-model-credential-under-another-spelling")
	t.Setenv(piHomeEnv, "/somewhere/else")
	t.Setenv(piSessionDirEnv, "/somewhere/else/sessions")
	t.Setenv("UNRELATED_SETTING", "kept")

	environment := piEnvironment("/scratch/home", piLoadEnvDenied)
	joined := strings.Join(environment, "\n")
	denied := make(map[string]struct{}, len(documented)+1)
	for _, name := range documented {
		denied[strings.ToUpper(name)] = struct{}{}
	}
	denied[piSessionDirEnv] = struct{}{}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, still := denied[strings.ToUpper(name)]; still {
			t.Errorf("the environment still carries %s, whatever its spelling", entry)
		}
	}
	if !strings.Contains(joined, piHomeEnv+"=/scratch/home") {
		t.Errorf("the environment does not select the check's own data directory: %v", environment)
	}
	if !strings.Contains(joined, "UNRELATED_SETTING=kept") {
		t.Error("the check dropped a variable it does not own")
	}
}

// TestPiLoaderCwd pins the working directory the load check's harness process runs in: the session's own
// recorded directory, which must be a directory this host really has -- an empty or missing one is an
// answer of its own, named, rather than read as a fork prompt later.
func TestPiLoaderCwd(t *testing.T) {
	if _, err := piLoaderCwd("", ""); err == nil {
		t.Error("piLoaderCwd accepted a session that records no working directory")
	} else if !strings.Contains(err.Error(), "working directory") {
		t.Errorf("error = %q, want it to say the session records no working directory", err)
	}

	missing := filepath.Join(t.TempDir(), "not-here")
	if _, err := piLoaderCwd("", missing); err == nil {
		t.Error("piLoaderCwd accepted a recorded working directory this host does not have")
	} else if !strings.Contains(err.Error(), missing) {
		t.Errorf("error = %q, want it to name the recorded directory %q", err, missing)
	}

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing a file: %v", err)
	}
	if _, err := piLoaderCwd("", file); err == nil {
		t.Error("piLoaderCwd accepted a recorded path that is not a directory")
	} else if !strings.Contains(err.Error(), file) {
		t.Errorf("error = %q, want it to name the recorded path %q", err, file)
	}

	existing := t.TempDir()
	got, err := piLoaderCwd("a-scratch-home", existing)
	if err != nil {
		t.Fatalf("piLoaderCwd(%q) = %v, want the session's own directory", existing, err)
	}
	if got != existing {
		t.Errorf("piLoaderCwd(%q) = %q, want the process to run in the session's own directory", existing, got)
	}
}

// TestPiRPCReadsAndComparesTheLoadersAnswer pins the three byte-level reads the load check is built from:
// the protocol's own response records are read in order out of a stream that also holds a session event
// and a non-JSON diagnostic, a refused command surfaces the harness's own words, and the loader's own
// `get_messages` answer is compared with the file's model-visible history -- agreement answers "", and a
// count or a role that disagrees answers its own sentence.
func TestPiRPCReadsAndComparesTheLoadersAnswer(t *testing.T) {
	const sessionID = "01a1c335-0000-4000-8000-000000000000"
	data := []byte(strings.Join([]string{
		`{"type":"session","version":3,"id":"` + sessionID + `","cwd":"/home/user/code/example-api"}`,
		`{"id":"aa11bb22","type":"message","parentId":null,"message":{"role":"user","content":"hi"}}`,
		`{"id":"cc33dd44","type":"message","parentId":"aa11bb22","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`,
	}, "\n") + "\n")

	stdout := piRPCStdout(sessionID, 2, []string{"user", "assistant"})
	responses := readPiResponses(stdout)
	if len(responses) != 2 {
		t.Fatalf("readPiResponses kept %d response(s) %+v, want the two and not the session event or the "+
			"non-JSON diagnostic", len(responses), responses)
	}
	if responses[0].Command != "get_state" || responses[1].Command != "get_messages" {
		t.Errorf("responses = %+v, want them in the order the harness wrote them", responses)
	}
	state, err := readPiState(stdout)
	if err != nil {
		t.Fatalf("readPiState: %v", err)
	}
	if state.SessionID != sessionID || state.MessageCount != 2 {
		t.Errorf("state = %+v, want the harness's own answer naming the session and its count", state)
	}

	detail, err := piMessageDetail(stdout, data)
	if err != nil {
		t.Fatalf("piMessageDetail: %v", err)
	}
	if detail != "" {
		t.Errorf("piMessageDetail = %q, want \"\" when the loader's roles agree with the file", detail)
	}

	// A count that disagrees: the sentence names both numbers.
	countMismatch := piRPCStdout(sessionID, 2, []string{"user"})
	detail, err = piMessageDetail(countMismatch, data)
	if err != nil {
		t.Fatalf("piMessageDetail(count mismatch): %v", err)
	}
	if !strings.Contains(detail, "built 1") || !strings.Contains(detail, "holds 2") {
		t.Errorf("detail = %q, want it to name the loader's count and the file's own", detail)
	}

	// A role that disagrees: the sentence names the position and both roles.
	roleMismatch := piRPCStdout(sessionID, 2, []string{"assistant", "user"})
	detail, err = piMessageDetail(roleMismatch, data)
	if err != nil {
		t.Fatalf("piMessageDetail(role mismatch): %v", err)
	}
	if !strings.Contains(detail, "position 1") || !strings.Contains(detail, "assistant") ||
		!strings.Contains(detail, "user") {
		t.Errorf("detail = %q, want it to name the position and the two roles that differ", detail)
	}

	// A refused command surfaces the harness's own words, never a pass.
	refusedState := `{"type":"response","command":"get_state","success":false,"error":"the loader refused this session"}` + "\n"
	if _, err := readPiState(refusedState); err == nil {
		t.Error("readPiState accepted a refused get_state")
	} else if !strings.Contains(err.Error(), "the loader refused this session") {
		t.Errorf("error = %q, want the harness's own words", err)
	}
	refusedMessages := `{"type":"response","command":"get_messages","success":false,"error":"no messages for this session"}` + "\n"
	if _, err := piMessageDetail(refusedMessages, data); err == nil {
		t.Error("piMessageDetail accepted a refused get_messages")
	} else if !strings.Contains(err.Error(), "no messages for this session") {
		t.Errorf("error = %q, want the harness's own words", err)
	}
}

// TestPiModelVisibleRolesAndExpectedMessageCount pins the byte-level walk the load check compares the
// harness's answer with, against the committed fixtures' own bytes and against literals.
//
// The walk is the harness's own -- from the file's last entry back through parentId, which is what its
// loader builds a session's path from -- so an entry the file holds but no later record hangs off is
// recorded history rather than context, and only a compaction entry *on* that path is the boundary the
// loader rebuilds its context from. The counts are pinned as literals (measured through the harness's own
// `get_state` answers for these files, and re-derived in 0005's evidence record), never as the walk's own
// length, which would be a tautology.
func TestPiModelVisibleRolesAndExpectedMessageCount(t *testing.T) {
	ordinary, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the Pi fixture: %v", err)
	}
	if got, want := piModelVisibleRoles(ordinary), []string{"user", "assistant", "toolResult", "assistant", "assistant"}; !slices.Equal(got, want) {
		t.Errorf("piModelVisibleRoles(ordinary fixture) = %v, want %v", got, want)
	}
	if got := piExpectedMessageCount(ordinary); got != 5 {
		t.Errorf("piExpectedMessageCount(ordinary fixture) = %d, want the literal 5", got)
	}

	history, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piHistoryFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the Pi history fixture: %v", err)
	}
	// The history fixture's compaction entry -- and the turn that follows it -- hang off an entry the
	// file's last record does not continue from, so they are recorded history rather than context.
	// Measured on the tested harness: its own loader answers this file with `messageCount` 3 and exactly
	// the roles below, no `compactionSummary` among them. A walk that read "the last compaction entry in
	// file order, then every message after it" would answer 4 and report a boundary the loader never
	// rebuilt anything from -- the reading the reader's own answer for a Pi file does not support, since
	// a record after a tool result names the message that made the call rather than the result.
	if got, want := piModelVisibleRoles(history), []string{"user", "assistant", "assistant"}; !slices.Equal(got, want) {
		t.Errorf("piModelVisibleRoles(history fixture) = %v, want %v", got, want)
	}
	if got := piExpectedMessageCount(history); got != 3 {
		t.Errorf("piExpectedMessageCount(history fixture) = %d, want the literal 3", got)
	}

	// The other side of the boundary rule, written here rather than taken from a fixture: a compaction
	// entry *on* the walk is the boundary the loader rebuilds from, so it is reported as the one
	// compactionSummary entry and the messages before it on the path leave the count.
	compacted := []byte(strings.Join([]string{
		`{"type":"session","version":3,"id":"01a1c335-0000-4000-8000-000000000000",` +
			`"timestamp":"2026-05-05T11:00:00.000Z","cwd":"/tmp/x"}`,
		`{"type":"message","id":"aaaa1111","parentId":null,"timestamp":"2026-05-05T11:00:01.000Z",` +
			`"message":{"role":"user","content":"before the boundary","timestamp":1795026001000}}`,
		`{"type":"compaction","id":"bbbb2222","parentId":"aaaa1111",` +
			`"timestamp":"2026-05-05T11:01:00.000Z","summary":"what came before","firstKeptEntryId":"bbbb2222",` +
			`"systemMessage":null}`,
		`{"type":"message","id":"cccc3333","parentId":"bbbb2222","timestamp":"2026-05-05T11:02:00.000Z",` +
			`"message":{"role":"user","content":"after the boundary","timestamp":1795026120000}}`,
		"",
	}, "\n"))
	if got, want := piModelVisibleRoles(compacted), []string{"compactionSummary", "user"}; !slices.Equal(got, want) {
		t.Errorf("piModelVisibleRoles(a compacted file) = %v, want %v", got, want)
	}
	if got := piExpectedMessageCount(compacted); got != 2 {
		t.Errorf("piExpectedMessageCount(a compacted file) = %d, want the literal 2", got)
	}
}

// fakePiRPC stands in for the target harness's RPC mode in the load-check case below: it answers the
// check's two arms the way the installed harness was measured to, and records what it was asked, so the
// test can assert both the check's verdict and what the check really ran.
type fakePiRPC struct {
	// installed is the session id the fake resolves; any other id gets the not-found arm.
	installed string
	// answerSessionID, when non-empty, is reported in get_state instead of the requested id.
	answerSessionID string
	// messageCount, when non-zero, overrides the count reported in get_state.
	messageCount int
	// roles is the get_messages answer the fake builds; empty answers no messages.
	roles []string
	// controlSucceeds makes the unknown-session arm answer a success response rather than the measured
	// missing-session error.
	controlSucceeds bool
	// controlErrorText replaces the missing-session text of the unknown arm.
	controlErrorText string
	// plantControl makes the unknown arm leave a session file of its own behind, in the scratch sessions
	// tree below the home it was driven with and one project directory down -- the shape a harness's own
	// write takes -- so the check's tree scan is exercised by an arm that *did* write a session while
	// still reporting the missed lookup.
	plantControl bool
	// removeSessionsRoot makes the unknown arm delete the scratch sessions tree, so the check is answered
	// with a tree it cannot read rather than with "nothing was written there".
	removeSessionsRoot bool
	// mutateInstalledPath, when non-empty, is appended to once the installed arm runs, so the check
	// observes the installed file changing underneath it.
	mutateInstalledPath string
	calls               []fakePiRPCCall
}

type fakePiRPCCall struct {
	executable string
	cwd        string
	home       string
	arguments  []string
	stdin      string
}

func (f *fakePiRPC) run(
	_ context.Context, binary, cwd, home string, arguments []string, stdin string,
	_ time.Duration, _ io.Writer,
) (string, error) {
	f.calls = append(f.calls, fakePiRPCCall{
		executable: binary, cwd: cwd, home: home, arguments: arguments, stdin: stdin,
	})
	sessionID := ""
	if index := slices.Index(arguments, "--session"); index >= 0 && index+1 < len(arguments) {
		sessionID = arguments[index+1]
	}
	if sessionID != f.installed {
		if f.controlSucceeds {
			return piRPCStdout(sessionID, 1, []string{"user"}), nil
		}
		if f.plantControl {
			planted := filepath.Join(home, "sessions", "--planted-project--",
				"2026-01-01T00-00-00-000Z_"+sessionID+".jsonl")
			if err := os.MkdirAll(filepath.Dir(planted), 0o700); err != nil {
				return "", fmt.Errorf("the fake could not plant a control session: %w", err)
			}
			if err := os.WriteFile(planted, []byte(`{"type":"session","version":3,"id":"`+sessionID+`"}`+"\n"), 0o600); err != nil {
				return "", fmt.Errorf("the fake could not plant a control session: %w", err)
			}
		}
		if f.removeSessionsRoot {
			if err := os.RemoveAll(filepath.Join(home, "sessions")); err != nil {
				return "", fmt.Errorf("the fake could not remove the scratch sessions tree: %w", err)
			}
		}
		text := f.controlErrorText
		if text == "" {
			text = "No session found matching " + sessionID
		}
		return "", errors.New(text)
	}
	if f.mutateInstalledPath != "" {
		if file, err := os.OpenFile(f.mutateInstalledPath, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			file.WriteString("{\"type\":\"message\",\"id\":\"appended\"}\n")
			file.Close()
		}
	}
	reported := sessionID
	if f.answerSessionID != "" {
		reported = f.answerSessionID
	}
	count := f.messageCount
	if count == 0 {
		count = len(f.roles)
	}
	return piRPCStdout(reported, count, f.roles), nil
}

// piRPCStdout is one RPC run's stdout in the shape the harness was measured to write: a session event
// and a non-JSON diagnostic, which the reads skip, then a get_state answer and a get_messages answer.
func piRPCStdout(sessionID string, messageCount int, roles []string) string {
	messages := make([]map[string]string, 0, len(roles))
	for _, role := range roles {
		messages = append(messages, map[string]string{"role": role})
	}
	stateData, _ := json.Marshal(map[string]any{
		"sessionId": sessionID, "messageCount": messageCount, "sessionFile": "",
	})
	messagesData, _ := json.Marshal(map[string]any{"messages": messages})
	state, _ := json.Marshal(map[string]any{
		"type": "response", "command": "get_state", "success": true, "data": json.RawMessage(stateData),
	})
	answers, _ := json.Marshal(map[string]any{
		"type": "response", "command": "get_messages", "success": true, "data": json.RawMessage(messagesData),
	})
	return strings.Join([]string{
		`{"type":"session","sessionId":"` + sessionID + `"}`,
		"pi: loading",
		string(state),
		string(answers),
	}, "\n") + "\n"
}

// fakePiTurn stands in for the target harness's turn in the resume case below, and records what it was
// asked. growPath, when non-empty, is appended to the way the harness writes its own records into a
// session it resumes.
type fakePiTurn struct {
	growPath    string
	answer      string
	diagnostics string
	runErr      error
	calls       []fakePiTurnCall
}

type fakePiTurnCall struct {
	executable string
	cwd        string
	home       string
	arguments  []string
}

func (f *fakePiTurn) run(
	_ context.Context, executable, cwd, home string, args []string, _ time.Duration, _ io.Writer,
) (string, string, error) {
	f.calls = append(f.calls, fakePiTurnCall{
		executable: executable, cwd: cwd, home: home, arguments: args,
	})
	if f.growPath != "" {
		if file, err := os.OpenFile(f.growPath, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			file.WriteString("{\"type\":\"message\",\"id\":\"appended\"}\n")
			file.Close()
		}
	}
	return f.answer, f.diagnostics, f.runErr
}

// fakePiHarness is what withFakePiHarness answers with: the path the checks' lookup was answered with.
type fakePiHarness struct {
	resolvedExecutable string
}

// withFakePiHarness installs one fake harness for the duration of a test: the two seams the checks drive
// a harness through -- the loader's RPC mode and the resume turn -- are answered by the fakes that are
// given (nil leaves a seam at its real implementation), and the executables lookup is answered by a path
// of the test's own. What these tests pin is what each check makes of the harness's answer, not whether
// this host has Pi installed.
func withFakePiHarness(t *testing.T, rpc *fakePiRPC, turn *fakePiTurn) *fakePiHarness {
	t.Helper()
	originalRPC := runPiLoaderRPC
	if rpc != nil {
		runPiLoaderRPC = rpc.run
	}
	t.Cleanup(func() { runPiLoaderRPC = originalRPC })
	originalTurn := runPiResumeTurn
	if turn != nil {
		runPiResumeTurn = turn.run
	}
	t.Cleanup(func() { runPiResumeTurn = originalTurn })
	resolved := filepath.Join(t.TempDir(), PiBinaryDefault)
	originalLookup := resolvePiExecutable
	resolvePiExecutable = func(string) (string, error) { return resolved, nil }
	t.Cleanup(func() { resolvePiExecutable = originalLookup })
	return &fakePiHarness{resolvedExecutable: resolved}
}

// installedPiSession writes a converted Pi session into a fresh home and answers the home, a working
// directory that exists here, the file's own bytes and the installed path.
func installedPiSession(t *testing.T) (string, string, []byte, string) {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, filepath.Join("pi", piFixtureRef.ID+".jsonl")))
	if err != nil {
		t.Fatalf("reading the Pi fixture: %v", err)
	}
	home := t.TempDir()
	cwd := t.TempDir()
	path, _, err := InstallPiSession(PiSession{
		SessionID: piFixtureRef.ID,
		Directory: PiProjectDir(cwd),
		FileName:  "2026-05-04T09-15-00-000Z_" + piFixtureRef.ID + ".jsonl",
		Cwd:       cwd,
		Data:      data,
	}, home)
	if err != nil {
		t.Fatalf("installing the Pi session: %v", err)
	}
	return home, cwd, data, path
}

// TestVerifyPiLoadResolvesTheInstalledSessionAndItsControl pins the load check's rule: the harness's own
// loader must name the installed session and the file's model-visible count, the same question for an
// unknown id must report no such session, the loader must be driven against a throw-away copy (never the
// installed file, which a resumed session gets written into), and the installed file must be untouched
// afterwards.
func TestVerifyPiLoadResolvesTheInstalledSessionAndItsControl(t *testing.T) {
	home, cwd, data, installed := installedPiSession(t)
	before := hashFile(t, installed)
	roles := piModelVisibleRoles(data)
	rpc := &fakePiRPC{installed: piFixtureRef.ID, roles: roles}
	probe := withFakePiHarness(t, rpc, nil)

	verification := VerifyPiLoad(context.Background(), PiLoadOptions{
		Binary: PiBinaryDefault, SessionID: piFixtureRef.ID, Cwd: cwd,
		Directory: PiProjectDir(cwd), FileName: filepath.Base(installed),
		Data: data, Installed: installed, Timeout: time.Minute,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("load = %+v, want a verified load", verification)
	}
	if len(rpc.calls) != 2 {
		t.Fatalf("the check made %d call(s), want the probe and its control", len(rpc.calls))
	}
	for index, call := range rpc.calls {
		if call.home == home {
			t.Errorf("arm %d drove the loader against the installed data directory rather than a copy (%s)",
				index, call.home)
		}
		if call.cwd != cwd {
			t.Errorf("arm %d ran in %q, want the session's own working directory %q", index, call.cwd, cwd)
		}
		if call.executable != probe.resolvedExecutable {
			t.Errorf("arm %d ran %q, want the path the check resolved (%q)",
				index, call.executable, probe.resolvedExecutable)
		}
	}
	if !strings.Contains(verification.Detail, "No session found matching") {
		t.Errorf("detail = %q, want the control arm's own answer quoted", verification.Detail)
	}
	if strings.Contains(verification.Detail, installed) {
		t.Errorf("detail = %q, want it to name the throw-away copy the check installed, not the installed file",
			verification.Detail)
	}
	if digest := hashFile(t, installed); digest != before {
		t.Error("the load check altered the installed session")
	}
}

// TestVerifyPiLoadRefusesEveryBadAnswer pins each refusal the load check implements: an answer about
// another session, a count that disagrees with the file, a control arm that answers at all, a control
// arm whose error is not the missed lookup, and an installed file the check cannot report as unchanged.
func TestVerifyPiLoadRefusesEveryBadAnswer(t *testing.T) {
	_, cwd, data, installed := installedPiSession(t)
	roles := piModelVisibleRoles(data)
	load := func(t *testing.T, rpc *fakePiRPC) LoadVerification {
		t.Helper()
		withFakePiHarness(t, rpc, nil)
		return VerifyPiLoad(context.Background(), PiLoadOptions{
			Binary: PiBinaryDefault, SessionID: piFixtureRef.ID, Cwd: cwd,
			Directory: PiProjectDir(cwd), FileName: filepath.Base(installed),
			Data: data, Installed: installed, Timeout: time.Minute,
		})
	}
	base := func() *fakePiRPC { return &fakePiRPC{installed: piFixtureRef.ID, roles: roles} }

	t.Run("a different session id answered", func(t *testing.T) {
		rpc := base()
		rpc.answerSessionID = "99999999-9999-4999-8999-999999999999"
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the harness answered about another session", verification)
		}
		if !strings.Contains(verification.Detail, "answered about session") {
			t.Errorf("detail = %q, want the identity mismatch named", verification.Detail)
		}
	})
	t.Run("a disagreeing count", func(t *testing.T) {
		rpc := base()
		rpc.messageCount = len(roles) + 1
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the count disagrees with the file", verification)
		}
		if !strings.Contains(verification.Detail, "while the session file holds") {
			t.Errorf("detail = %q, want it to name the file's own count", verification.Detail)
		}
	})
	t.Run("a control arm that answers successfully", func(t *testing.T) {
		rpc := base()
		rpc.controlSucceeds = true
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the control arm answered a session nothing installed",
				verification)
		}
		if !strings.Contains(verification.Detail, "control arm") {
			t.Errorf("detail = %q, want it to name the control arm", verification.Detail)
		}
	})
	t.Run("a control arm error that is not a missed lookup", func(t *testing.T) {
		rpc := base()
		rpc.controlErrorText = "the harness fell over for a reason of its own"
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the control arm did not report the session as missing",
				verification)
		}
		if !strings.Contains(verification.Detail, "did not report the unknown session as missing") {
			t.Errorf("detail = %q, want it to name the control arm's missing answer", verification.Detail)
		}
	})
	t.Run("a control arm that wrote a session of its own", func(t *testing.T) {
		rpc := base()
		rpc.plantControl = true
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the control arm left a session behind, so the missed "+
				"lookup did not stop before a new session was created", verification)
		}
		if !strings.Contains(verification.Detail, "wrote a session file") {
			t.Errorf("detail = %q, want it to name the session the control arm wrote", verification.Detail)
		}
		// The scan must be a walk of the sessions tree, not a listing of its own entries: the planted
		// session sits under a project directory, where a listing of the root's children never looks.
		if !strings.Contains(verification.Detail, "--planted-project--") {
			t.Errorf("detail = %q, want it to name the planted session's own path", verification.Detail)
		}
	})
	t.Run("a sessions tree the check cannot read", func(t *testing.T) {
		rpc := base()
		rpc.removeSessionsRoot = true
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the check could not read the tree it has to scan for a "+
				"session the control arm may have written", verification)
		}
		if !strings.Contains(verification.Detail, "could not read") {
			t.Errorf("detail = %q, want it to say the tree could not be read", verification.Detail)
		}
	})
	t.Run("the installed file changed underneath the check", func(t *testing.T) {
		rpc := base()
		rpc.mutateInstalledPath = installed
		verification := load(t, rpc)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the installed file changed during the check", verification)
		}
		if !strings.Contains(verification.Detail, "unchanged") {
			t.Errorf("detail = %q, want it to say the installed file cannot be reported as unchanged",
				verification.Detail)
		}
	})
}

// TestVerifyPiResumeReportsTheTurnAndTheGrowth pins the resume level's report through the turn seam: the
// growth sentence a verified turn carries, the harness's own words when it fails, and the missing-binary
// answer -- each with "verified" claimed only when a turn really produced an answer and grew the file.
func TestVerifyPiResumeReportsTheTurnAndTheGrowth(t *testing.T) {
	home, cwd, _, installed := installedPiSession(t)
	turn := &fakePiTurn{growPath: installed, answer: "It is set in internal/retry/config.go."}
	probe := withFakePiHarness(t, nil, turn)

	verification := VerifyPiResume(context.Background(), PiResumeOptions{
		Home: home, Binary: PiBinaryDefault, SessionID: piFixtureRef.ID, Cwd: cwd,
		SessionPath: installed, Prompt: "Where is the retry cap configured?",
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("resume = %+v, want a verified turn", verification)
	}
	want := fmt.Sprintf("the session file went from %d to %d bytes",
		verification.SessionBytesBefore, verification.SessionBytesAfter)
	if !strings.Contains(verification.Detail, want) {
		t.Errorf("detail = %q, want the growth sentence %q", verification.Detail, want)
	}
	if verification.SessionBytesAfter <= verification.SessionBytesBefore {
		t.Errorf("the report does not show the session growing (%d -> %d)",
			verification.SessionBytesBefore, verification.SessionBytesAfter)
	}
	if len(turn.calls) != 1 {
		t.Fatalf("the check made %d turn(s), want one", len(turn.calls))
	}
	if turn.calls[0].executable != probe.resolvedExecutable {
		t.Errorf("the turn ran %q, want the path the check resolved (%q)",
			turn.calls[0].executable, probe.resolvedExecutable)
	}
	if turn.calls[0].cwd != cwd {
		t.Errorf("the turn ran in %q, want the session's own working directory %q", turn.calls[0].cwd, cwd)
	}
	if !strings.Contains(strings.Join(turn.calls[0].arguments, " "), "Where is the retry cap configured?") {
		t.Error("the resume turn did not carry the caller's prompt")
	}

	failing := &fakePiTurn{diagnostics: "Not logged in · Please run /login", runErr: errors.New("exit status 1")}
	withFakePiHarness(t, nil, failing)
	verification = VerifyPiResume(context.Background(), PiResumeOptions{
		Home: home, Binary: PiBinaryDefault, SessionID: piFixtureRef.ID, Cwd: cwd,
		SessionPath: installed, Prompt: "Where is the retry cap configured?",
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it refused: the harness did not answer", verification)
	}
	if !strings.Contains(verification.Detail, "Not logged in") {
		t.Errorf("detail = %q, want the harness's own reason", verification.Detail)
	}
}

// TestVerifyPiResumeRefusesWithoutABinary pins the missing-binary answer: a PATH that holds no harness is
// reported as a reason naming it, never as a pass and never as a panic.
func TestVerifyPiResumeRefusesWithoutABinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	verification := VerifyPiResume(context.Background(), PiResumeOptions{
		Home: t.TempDir(), Binary: PiBinaryDefault, SessionID: piFixtureRef.ID,
		Cwd: t.TempDir(), SessionPath: filepath.Join(t.TempDir(), "session.jsonl"),
		Prompt: "how many attempts?",
	})
	if verification.State == VerificationStateVerified {
		t.Fatalf("resume = %+v, want it not verified", verification)
	}
	if !strings.Contains(verification.Detail, "PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", verification.Detail)
	}
}
