package ccc

import (
	"bytes"
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

// TestOMPVersionFrom pins the version line the probe reads: the harness answers `omp/18.18.7`, and the
// report records the version itself rather than the product name beside it.
func TestOMPVersionFrom(t *testing.T) {
	for _, testCase := range []struct{ output, want string }{
		{"omp/18.18.7\n", "18.18.7"},
		{"18.18.7", "18.18.7"},
		{"oh my pi 19.0.0\n", "19.0.0"},
		{"no version here", ""},
		{"", ""},
	} {
		if got := ompVersionFrom(testCase.output); got != testCase.want {
			t.Errorf("ompVersionFrom(%q) = %q, want %q", testCase.output, got, testCase.want)
		}
	}
}

// TestProbeOMPBinaryReportsAMissingBinary pins the answer a host without the harness gets: an answer, not
// an error, with the reason named, so a conversion can still be written and verified structurally.
func TestProbeOMPBinaryReportsAMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	probe := ProbeOMPBinary(context.Background(), OMPBinaryDefault)
	if probe.Found {
		t.Fatalf("probe = %+v, want it not found on an empty PATH", probe)
	}
	if !strings.Contains(probe.Detail, "PATH") {
		t.Errorf("detail = %q, want it to say the binary is not on PATH", probe.Detail)
	}
}

// TestOMPLoadEnvironmentRemovesEveryModelPath pins what the offline load check hands its harness: the
// data directory it must read, and none of the variables that could reach a model. The documented set is
// written out here as literals -- a case that looped over the check's own list could not fail for a name
// the list is missing, and that is the failure this contract has to catch.
//
// The second half is the OMP-specific rule this route exists to not get wrong: the sessions *tree*
// variable is named, with the session's own project directory rather than the `sessions` root above it.
// Measured on 18.18.7: the variable names the directory that holds session files *directly*, so naming
// this directory resolves the session while naming the root above a session's project directory answers
// that no such session is installed -- and naming it is also what keeps a caller's leftover value (a Pi
// user's, most obviously) from sending the lookup to a tree this session is not in.
func TestOMPLoadEnvironmentRemovesEveryModelPath(t *testing.T) {
	documented := []string{
		"PI_API_KEY",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_OAUTH_TOKEN",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"XAI_API_KEY",
		"DEEPSEEK_API_KEY",
		"GROQ_API_KEY",
		"MISTRAL_API_KEY",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AZURE_OPENAI_API_KEY",
		"LITELLM_API_KEY",
		"LITELLM_BASE_URL",
	}
	if !slices.Equal(ompLoadEnvDenied, documented) {
		t.Errorf("ompLoadEnvDenied = %v, want the harness's documented model paths %v",
			ompLoadEnvDenied, documented)
	}
	for _, name := range documented {
		t.Setenv(name, "a-model-credential")
	}
	t.Setenv(strings.ToLower(documented[1]), "a-model-credential-under-another-spelling")
	t.Setenv(piSessionDirEnv, "/somewhere/else/sessions")
	t.Setenv(ompHomeEnv, "/somewhere/else")
	t.Setenv("UNRELATED_SETTING", "kept")

	environment := ompEnvironment("/scratch/home", "/scratch/home/sessions/-project-", ompLoadEnvDenied)
	denied := make(map[string]struct{}, len(documented)+1)
	for _, name := range documented {
		denied[strings.ToUpper(name)] = struct{}{}
	}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, still := denied[strings.ToUpper(name)]; still {
			t.Errorf("the environment still carries %s, whatever its spelling", entry)
		}
		if strings.EqualFold(name, ompSessionDirEnv) && entry != ompSessionDirEnv+"=/scratch/home/sessions/-project-" {
			t.Errorf("the environment names %s, want the session's own directory: %v", entry, environment)
		}
	}
	joined := strings.Join(environment, "\n")
	if !strings.Contains(joined, ompHomeEnv+"=/scratch/home") {
		t.Errorf("the environment does not select the check's own data directory: %v", environment)
	}
	if !strings.Contains(joined, "UNRELATED_SETTING=kept") {
		t.Error("the check dropped a variable it does not own")
	}
	if strings.Contains(joined, "/somewhere/else") {
		t.Errorf("the caller's own session directory survived: %v", environment)
	}
}

// TestOMPLoaderCwdRefusesARecordedDirectoryThisHostDoesNotHave pins the rule the harness itself applies:
// a recorded working directory that is not a directory here is refused with the harness's own sentence,
// rather than quietly running the loader somewhere else and reporting its failure as this check's.
func TestOMPLoaderCwdRefusesARecordedDirectoryThisHostDoesNotHave(t *testing.T) {
	scratch := t.TempDir()
	present := t.TempDir()
	if cwd, err := ompLoaderCwd(scratch, present); err != nil || cwd != present {
		t.Errorf("ompLoaderCwd(%q) = (%q, %v), want the recorded directory itself", present, cwd, err)
	}
	if cwd, err := ompLoaderCwd(scratch, "   "); err != nil || cwd != scratch {
		t.Errorf("ompLoaderCwd(a recording with no directory) = (%q, %v), want the check's own data directory",
			cwd, err)
	}
	cwd, err := ompLoaderCwd(scratch, filepath.Join(scratch, "not-here"))
	if err == nil {
		t.Fatalf("ompLoaderCwd accepted a recorded directory this host does not have (%q)", cwd)
	}
	for _, want := range []string{"no longer exists", "recorded directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to quote the harness's own refusal (%q)", err, want)
		}
	}
}

// TestOMPSessionBindingReadsTheBindingThenTheMessages pins where the load check reads the provider and
// model a scratch data directory has to define: the session's own `model_change` binding when it holds
// one, else the pair the last assistant record carries -- the same order the harness restores a session's
// model in (measured).
func TestOMPSessionBindingReadsTheBindingThenTheMessages(t *testing.T) {
	bound := []byte(`{"type":"title"}` + "\n" +
		`{"type":"model_change","id":"m","parentId":null,"model":"home/model-a"}` + "\n" +
		`{"type":"message","id":"a","parentId":"m","message":{"role":"assistant","model":"source-model","provider":"source-proxy"}}` + "\n")
	provider, model, ok := ompSessionBinding(bound)
	if !ok || provider != "home" || model != "model-a" {
		t.Errorf("binding = (%q, %q, %v), want the session's own binding", provider, model, ok)
	}

	unbound := []byte(`{"type":"message","id":"a","parentId":null,"message":{"role":"assistant","model":"source-model","provider":"source-proxy"}}` + "\n")
	provider, model, ok = ompSessionBinding(unbound)
	if !ok || provider != "source-proxy" || model != "source-model" {
		t.Errorf("binding = (%q, %q, %v), want the last assistant record's pair", provider, model, ok)
	}

	if _, _, ok := ompSessionBinding([]byte(`{"type":"message","id":"u","parentId":null,"message":{"role":"user","content":"hi"}}` + "\n")); ok {
		t.Error("a file holding no model of any kind was read as one the check can build a home for")
	}
}

// TestOMPYAMLScalarQuotesExoticValues pins the rule that keeps the scratch data directory's own YAML the
// shape the harness's reader expects: a value that is a plain scalar stays as it is, and one holding a
// character YAML acts on -- or holding nothing at all -- becomes a double-quoted JSON string. The provider
// and model ids are the target home's own configuration answers, so an id this build did not anticipate is
// ordinary; the Python check's twin rule is pinned in scripts/tests/test_omp_native_check.py.
func TestOMPYAMLScalarQuotesExoticValues(t *testing.T) {
	for _, testCase := range []struct{ value, want string }{
		{"example-proxy", "example-proxy"},
		{"a:b", `"a:b"`},
		{"a b", `"a b"`},
		{"", `""`},
		{"provider#one", `"provider#one"`},
		{"line\nbreak", `"line\nbreak"`},
	} {
		if got := ompYAMLScalar(testCase.value); got != testCase.want {
			t.Errorf("ompYAMLScalar(%q) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

// fakeOMPRPC stands in for the harness's RPC mode. Its run signature is the OMP runner's own -- a data
// directory and no sessions tree -- and it answers the protocol text a real run writes, so the check's
// reader is exercised rather than stubbed around.
type fakeOMPRPC struct {
	installed         string
	roles             []string
	answerSessionID   string
	messageCount      int
	sessionFile       string
	controlErrorText  string
	refuseEveryCall   string
	omitModel         bool
	controlSucceeds   bool
	plantControl      bool
	mutateInstalledAt string
	calls             []fakeOMPRPCCall
}

type fakeOMPRPCCall struct {
	executable string
	cwd        string
	home       string
	sessionDir string
	arguments  []string
	stdin      string
	// copyPath and copyBytes are the session file the check's copy occupies in the data directory the arm
	// was handed, read while the arm runs: the check removes its scratch data directory when it is done, so
	// a test that looked afterwards would be reading a path nothing is at.
	copyPath  string
	copyBytes []byte
	// models is the provider definition that data directory carries, captured the same way.
	models string
}

// capturedCopy answers the session file an arm's data directory holds for one id, and the provider
// definition beside it: what the check wrote into its own scratch directory, read while that directory
// is there.
func capturedCopy(home, sessionDir, id string) (string, []byte, string) {
	matches, _ := filepath.Glob(filepath.Join(sessionDir, "*"+id+"*.jsonl"))
	copyPath := ""
	var copyBytes []byte
	if len(matches) > 0 {
		copyPath = matches[0]
		copyBytes, _ = os.ReadFile(copyPath)
	}
	models, _ := os.ReadFile(filepath.Join(home, "models.yml"))
	return copyPath, copyBytes, string(models)
}

func (f *fakeOMPRPC) run(
	_ context.Context, binary, cwd, home, sessionDir string, arguments []string, stdin string,
	_ time.Duration, _ io.Writer,
) ([]piRPCResponse, error) {
	sessionID := ""
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "--resume=") {
			sessionID = strings.TrimPrefix(argument, "--resume=")
		}
	}
	copyPath, copyBytes, models := capturedCopy(home, sessionDir, sessionID)
	f.calls = append(f.calls, fakeOMPRPCCall{
		executable: binary, cwd: cwd, home: home, sessionDir: sessionDir,
		arguments: arguments, stdin: stdin, copyPath: copyPath, copyBytes: copyBytes, models: models,
	})
	if f.refuseEveryCall != "" {
		return nil, errors.New(f.refuseEveryCall)
	}
	if sessionID != f.installed {
		if f.controlSucceeds {
			return readPiResponses(strings.NewReader(ompRPCStdout(sessionID, 1, []string{"user"}, "", true)))
		}
		if f.plantControl {
			planted := filepath.Join(home, "sessions", "-planted-project-",
				"2026-01-01T00-00-00-000Z_"+sessionID+".jsonl")
			if err := os.MkdirAll(filepath.Dir(planted), 0o700); err != nil {
				return nil, fmt.Errorf("the fake could not plant a control session: %w", err)
			}
			if err := os.WriteFile(planted, []byte(`{"type":"session","version":3,"id":"`+sessionID+`"}`+"\n"), 0o600); err != nil {
				return nil, fmt.Errorf("the fake could not plant a control session: %w", err)
			}
		}
		text := f.controlErrorText
		if text == "" {
			text = "Error: Session \"" + sessionID + "\" not found.\nRun `omp --resume` without an " +
				"argument to pick from recent sessions"
		}
		return nil, errors.New(text)
	}
	if f.mutateInstalledAt != "" {
		if file, err := os.OpenFile(f.mutateInstalledAt, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			file.WriteString("{\"type\":\"custom\",\"customType\":\"session_exit\"}\n")
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
	return readPiResponses(strings.NewReader(ompRPCStdout(reported, count, f.roles, f.sessionFile, !f.omitModel)))
}

// ompRPCStdout is one RPC run's stdout in the shape the harness was measured to write: a ready event and
// its own chatter, which the reads skip, then a get_state answer and a get_messages answer.
func ompRPCStdout(sessionID string, messageCount int, roles []string, sessionFile string, withModel bool) string {
	messages := make([]map[string]string, 0, len(roles))
	for _, role := range roles {
		messages = append(messages, map[string]string{"role": role})
	}
	state := map[string]any{
		"sessionId": sessionID, "messageCount": messageCount, "sessionFile": sessionFile,
	}
	if withModel {
		state["model"] = map[string]string{"provider": "example-proxy", "id": "example-model-2"}
	}
	stateData, _ := json.Marshal(state)
	messagesData, _ := json.Marshal(map[string]any{"messages": messages})
	stateAnswer, _ := json.Marshal(map[string]any{
		"type": "response", "command": "get_state", "success": true, "data": json.RawMessage(stateData),
	})
	answers, _ := json.Marshal(map[string]any{
		"type": "response", "command": "get_messages", "success": true, "data": json.RawMessage(messagesData),
	})
	return strings.Join([]string{
		`{"type":"ready","protocolVersion":1,"supportedProtocolVersions":[1,2]}`,
		`{"type":"extension_ui_request","id":"1","method":"setWidget"}`,
		string(stateAnswer),
		string(answers),
	}, "\n") + "\n"
}

// fakeOMPTurn stands in for the harness's turn in the resume cases, and records what it was asked.
type fakeOMPTurn struct {
	growPath    string
	answer      string
	diagnostics string
	runErr      error
	calls       []fakeOMPTurnCall
}

type fakeOMPTurnCall struct {
	executable string
	cwd        string
	home       string
	sessionDir string
	arguments  []string
}

func (f *fakeOMPTurn) run(
	_ context.Context, executable, cwd, home, sessionDir string, args []string,
	_ time.Duration, _ io.Writer,
) (string, string, error) {
	f.calls = append(f.calls, fakeOMPTurnCall{
		executable: executable, cwd: cwd, home: home, sessionDir: sessionDir, arguments: args,
	})
	if f.growPath != "" {
		if file, err := os.OpenFile(f.growPath, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			file.WriteString("{\"type\":\"custom\",\"customType\":\"session_exit\"}\n")
			file.Close()
		}
	}
	return f.answer, f.diagnostics, f.runErr
}

// withFakeOMPHarness installs one fake harness for the duration of a test: the two seams the checks drive
// a harness through -- the loader's RPC mode and the resume turn -- are answered by the fakes that are
// given (nil leaves a seam at its real implementation), and the executable lookup is answered by a path
// of the test's own. What these tests pin is what each check makes of the harness's answer, not whether
// this host has OMP installed.
func withFakeOMPHarness(t *testing.T, rpc *fakeOMPRPC, turn *fakeOMPTurn) string {
	t.Helper()
	originalRPC := runOMPLoaderRPC
	if rpc != nil {
		runOMPLoaderRPC = rpc.run
	}
	t.Cleanup(func() { runOMPLoaderRPC = originalRPC })
	originalTurn := runOMPResumeTurn
	if turn != nil {
		runOMPResumeTurn = turn.run
	}
	t.Cleanup(func() { runOMPResumeTurn = originalTurn })
	resolved := filepath.Join(t.TempDir(), OMPBinaryDefault)
	originalLookup := resolveOMPExecutable
	resolveOMPExecutable = func(string) (string, error) { return resolved, nil }
	t.Cleanup(func() { resolveOMPExecutable = originalLookup })
	return resolved
}

// installedOMPSession writes a converted OMP session into a fresh home and answers the home, a working
// directory that exists here, the file's own bytes and the installed path.
func installedOMPSession(t *testing.T) (string, string, []byte, string) {
	t.Helper()
	conversation := readFixtureConversation(t, ompFixtureRef)
	cwd := t.TempDir()
	session, err := ConvertToOMPSession(conversation, OMPTargetOptions{
		SessionID: ompFixtureRef.ID, Provider: "example-proxy", Model: "example-model-2", Now: time.Unix(0, 0),
	})
	if err != nil {
		t.Fatalf("converting the OMP fixture: %v", err)
	}
	home := t.TempDir()
	path, _, err := InstallOMPSession(session, home)
	if err != nil {
		t.Fatalf("installing the OMP session: %v", err)
	}
	return home, cwd, session.Data, path
}

// TestVerifyOMPLoadResolvesTheInstalledSessionAndItsControl pins the load check's rule: the harness's own
// loader must name the installed session and the file's model-visible count, the same question for an
// unknown id must be answered as a missed lookup, the loader must be driven against a throw-away copy in
// a scratch data directory (never the installed file, which this harness writes its own records into),
// and the scratch home must define the provider and model the converted session's own records name -- a
// home with no model refuses to start at all (measured).
func TestVerifyOMPLoadResolvesTheInstalledSessionAndItsControl(t *testing.T) {
	home, cwd, data, installed := installedOMPSession(t)
	before := hashFile(t, installed)
	directory := ompProjectDirFor(cwd, "/home/nobody", "/var/empty")
	// The fake answers the converted file's own walk as a literal -- the converted conversation's five
	// model-visible records, in the file's own order -- rather than reading it back off the walk's own
	// function: a case whose fake and expectation both come from the code under test cannot redden when
	// that walk regresses. The walk function itself is pinned against the committed fixtures' bytes with
	// literals in TestPiModelVisibleRolesAndExpectedMessageCount.
	rpc := &fakeOMPRPC{installed: ompFixtureRef.ID,
		roles: []string{"user", "assistant", "toolResult", "assistant", "assistant"}}
	resolved := withFakeOMPHarness(t, rpc, nil)

	verification := VerifyOMPLoad(context.Background(), OMPLoadOptions{
		Binary: OMPBinaryDefault, SessionID: ompFixtureRef.ID, Cwd: cwd,
		Directory: directory,
		FileName:  filepath.Base(installed),
		Data:      data, Installed: installed, Timeout: time.Minute,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("load = %+v, want a verified load", verification)
	}
	if len(rpc.calls) != 2 {
		t.Fatalf("the check made %d call(s), want the loader and its control", len(rpc.calls))
	}
	for index, call := range rpc.calls {
		if call.home == home {
			t.Errorf("arm %d drove the loader against the installed data directory rather than a copy (%s)",
				index, call.home)
		}
		if call.cwd != cwd {
			t.Errorf("arm %d ran in %q, want the session's own working directory %q", index, call.cwd, cwd)
		}
		if call.executable != resolved {
			t.Errorf("arm %d ran %q, want the path the check resolved (%q)", index, call.executable, resolved)
		}
		if !slices.Contains(call.arguments, "--mode") || !slices.Contains(call.arguments, "rpc") {
			t.Errorf("arm %d ran %v, want the harness's own RPC mode", index, call.arguments)
		}
		resumed := ""
		for _, argument := range call.arguments {
			if strings.HasPrefix(argument, "--resume=") {
				resumed = strings.TrimPrefix(argument, "--resume=")
			}
		}
		if resumed == "" {
			t.Errorf("arm %d asked the harness to open no session: %v", index, call.arguments)
		}
		if index == 0 && resumed != ompFixtureRef.ID {
			t.Errorf("arm 0 asked about %q, want the installed session %q", resumed, ompFixtureRef.ID)
		}
		if index == 1 && resumed == ompFixtureRef.ID {
			t.Errorf("the control arm asked about the installed session rather than an unknown id")
		}
		if !strings.Contains(call.home, "ccc-omp-load-") {
			t.Errorf("arm %d read %s, want the throw-away data directory the check made", index, call.home)
		}
		if want := filepath.Join(call.home, ompSessionsSubdir, directory); call.sessionDir != want {
			t.Errorf("arm %d named the sessions tree %q, want the copy's own %q: the printed invocation "+
				"must be what the check runs", index, call.sessionDir, want)
		}
		if index == 0 {
			if call.copyPath == "" {
				t.Errorf("the loader's data directory holds no copy of the converted bytes")
			} else if !bytes.Equal(call.copyBytes, data) {
				t.Errorf("the copy the loader was given is not the converted bytes (%d bytes vs %d)",
					len(call.copyBytes), len(data))
			}
		} else if call.copyPath != "" {
			t.Errorf("the control arm's data directory holds a session for an id nothing installed: %s",
				call.copyPath)
		}
		// The scratch home's own definition is the file's answer: the harness refuses to start in a
		// directory that names no model, and the model it restores is the session's own.
		if !strings.Contains(call.models, "example-proxy") || !strings.Contains(call.models, "example-model-2") {
			t.Errorf("arm %d's provider definition does not name the session's own pair:\n%s", index, call.models)
		}
		if !strings.Contains(call.models, "auth: none") {
			t.Errorf("arm %d's provider definition asks for a credential, and this check makes none:\n%s",
				index, call.models)
		}
	}
	if !strings.Contains(verification.Detail, "not found") {
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

// TestVerifyOMPLoadRefusesEveryBadAnswer pins each refusal the load check implements: an answer about
// another session, a count that disagrees with the file, a session file that is not the check's copy, a
// control arm that answers at all, a control arm whose error is not the missed lookup, a control arm that
// plants a session, an installed file the check cannot report as unchanged, a converted file that names no
// model to build a home for, and a recorded working directory this host does not have.
func TestVerifyOMPLoadRefusesEveryBadAnswer(t *testing.T) {
	home, cwd, data, installed := installedOMPSession(t)
	directory := ompProjectDirFor(cwd, "/home/nobody", "/var/empty")
	roles := piModelVisibleRoles(data)
	load := func(t *testing.T, rpc *fakeOMPRPC, data []byte, cwd string) LoadVerification {
		t.Helper()
		withFakeOMPHarness(t, rpc, nil)
		return VerifyOMPLoad(context.Background(), OMPLoadOptions{
			Binary: OMPBinaryDefault, SessionID: ompFixtureRef.ID, Cwd: cwd,
			Directory: directory, FileName: filepath.Base(installed),
			Data: data, Installed: installed, Timeout: time.Minute,
		})
	}
	base := func() *fakeOMPRPC { return &fakeOMPRPC{installed: ompFixtureRef.ID, roles: roles} }

	t.Run("a different session id answered", func(t *testing.T) {
		rpc := base()
		rpc.answerSessionID = "99999999-9999-4999-8999-999999999999"
		if verification := load(t, rpc, data, cwd); verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the harness answered about another session", verification)
		}
	})

	t.Run("a message count that disagrees with the file", func(t *testing.T) {
		rpc := base()
		rpc.messageCount = len(roles) + 3
		verification := load(t, rpc, data, cwd)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the loader's count is not the file's", verification)
		}
		if !strings.Contains(verification.Detail, "message") {
			t.Errorf("detail = %q, want it to name the count it compared", verification.Detail)
		}
	})

	t.Run("a message list whose roles disagree with the file", func(t *testing.T) {
		rpc := base()
		// The same count, so the count check passes and only the role-by-role comparison can refuse this
		// arm: the harness's loader has to answer the file's own walk, not merely a list as long. The
		// first two roles are swapped -- the converted file's walk starts with `user` and `assistant`
		// (pinned as a literal in the verified case above), so the first position differs.
		rpc.roles = slices.Clone(roles)
		rpc.roles[0], rpc.roles[1] = rpc.roles[1], rpc.roles[0]
		verification := load(t, rpc, data, cwd)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the loader's message roles are not the file's", verification)
		}
		if !strings.Contains(verification.Detail, "position") {
			t.Errorf("detail = %q, want it to name the position whose role differs", verification.Detail)
		}
	})

	t.Run("an answer from another file", func(t *testing.T) {
		rpc := base()
		rpc.sessionFile = filepath.Join(home, "sessions", directory, filepath.Base(installed))
		if verification := load(t, rpc, data, cwd); verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the harness answered from the installed file", verification)
		}
	})

	t.Run("a control arm that answers", func(t *testing.T) {
		rpc := base()
		rpc.controlSucceeds = true
		if verification := load(t, rpc, data, cwd); verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the control arm answered", verification)
		}
	})

	t.Run("a control arm whose error is not the missed lookup", func(t *testing.T) {
		rpc := base()
		rpc.controlErrorText = "Error: No default model selected."
		verification := load(t, rpc, data, cwd)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the control arm failed for another reason", verification)
		}
		if !strings.Contains(verification.Detail, "No default model selected") {
			t.Errorf("detail = %q, want the control arm's own words quoted", verification.Detail)
		}
	})

	t.Run("a control arm that wrote a session", func(t *testing.T) {
		rpc := base()
		rpc.plantControl = true
		if verification := load(t, rpc, data, cwd); verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the missed lookup created a session", verification)
		}
	})

	t.Run("an installed file the check altered", func(t *testing.T) {
		rpc := base()
		rpc.mutateInstalledAt = installed
		verification := load(t, rpc, data, cwd)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the installed file moved", verification)
		}
	})

	t.Run("a file that names no model", func(t *testing.T) {
		placeholder := []byte(`{"type":"message","id":"u","parentId":null,"message":{"role":"user","content":"hi"}}` + "\n")
		verification := load(t, base(), placeholder, cwd)
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: no scratch home can be built for it", verification)
		}
		if !strings.Contains(verification.Detail, "model") {
			t.Errorf("detail = %q, want it to say the file names no model", verification.Detail)
		}
	})

	t.Run("a recorded directory this host does not have", func(t *testing.T) {
		verification := load(t, base(), data, filepath.Join(cwd, "not-here"))
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the harness refuses such a session", verification)
		}
		if !strings.Contains(verification.Detail, "no longer exists") {
			t.Errorf("detail = %q, want the harness's own refusal quoted", verification.Detail)
		}
	})

	t.Run("a binary that is not installed", func(t *testing.T) {
		original := resolveOMPExecutable
		resolveOMPExecutable = func(string) (string, error) { return "", errors.New("not found") }
		t.Cleanup(func() { resolveOMPExecutable = original })
		verification := VerifyOMPLoad(context.Background(), OMPLoadOptions{
			Binary: OMPBinaryDefault, SessionID: ompFixtureRef.ID, Cwd: cwd,
			Directory: directory, FileName: filepath.Base(installed),
			Data: data, Installed: installed, Timeout: time.Minute,
		})
		if verification.State == VerificationStateVerified {
			t.Fatalf("load = %+v, want it refused: the harness is not installed", verification)
		}
		if !strings.Contains(verification.Detail, "PATH") {
			t.Errorf("detail = %q, want it to name the missing binary", verification.Detail)
		}
	})
}

// TestOMPResumeInvocationAndEnvironment pins the invocation the report prints and the resume check runs,
// and the environment it runs in: print mode with the session id and the option terminator, the data
// directory the session was installed into, and the session's own project directory below its sessions
// root. Measured on 18.18.7: the sessions-tree variable names the directory that holds session files
// *directly*, so naming this directory resolves the session while naming the `sessions` root above it
// answers that no such session is installed -- and naming it keeps a caller's leftover value from sending
// the printed line to a tree this session is not in.
func TestOMPResumeInvocationAndEnvironment(t *testing.T) {
	arguments := ompResumeArguments("01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4", "do the thing")
	want := []string{"-p", "--resume", "01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4", "--", "do the thing"}
	if !slices.Equal(arguments, want) {
		t.Errorf("ompResumeArguments = %v, want %v", arguments, want)
	}
	environment := ompResumeEnvironment("/scratch/home", "-home-user-Code-project-")
	if environment[ompHomeEnv] != "/scratch/home" {
		t.Errorf("ompResumeEnvironment = %v, want the data directory named", environment)
	}
	if want := filepath.Join("/scratch/home", ompSessionsSubdir, "-home-user-Code-project-"); environment[ompSessionDirEnv] != want {
		t.Errorf("ompResumeEnvironment names %q, want the directory that holds the session's file (%q): "+
			"measured, naming the sessions root above it answers that no such session is installed, and "+
			"naming this directory is also what keeps a caller's leftover value from hiding it",
			environment[ompSessionDirEnv], want)
	}
}

// TestVerifyOMPResumeRequiresAnAnswerAndGrowth pins the resume check's rule: the harness's own answer plus
// a session file that grew, run through the invocation the report prints, from the session's own working
// directory and with the printed environment. A missing prompt, a missing binary, a harness that fails,
// an empty answer and a file that did not grow are each reported as not-verified with their own reason.
func TestVerifyOMPResumeRequiresAnAnswerAndGrowth(t *testing.T) {
	_, cwd, _, installed := installedOMPSession(t)
	resume := func(t *testing.T, turn *fakeOMPTurn, prompt string) ResumeVerification {
		t.Helper()
		withFakeOMPHarness(t, nil, turn)
		return VerifyOMPResume(context.Background(), OMPResumeOptions{
			Home: filepath.Dir(installed), Binary: OMPBinaryDefault,
			SessionID: ompFixtureRef.ID, ProjectDir: filepath.Base(filepath.Dir(installed)),
			Cwd: cwd, SessionPath: installed, Prompt: prompt, Timeout: time.Minute,
		})
	}

	t.Run("the harness answers and appends", func(t *testing.T) {
		turn := &fakeOMPTurn{growPath: installed, answer: "PROBE-REPLY-OK"}
		verification := resume(t, turn, "Reply with exactly: PROBE-REPLY-OK")
		if verification.State != VerificationStateVerified {
			t.Fatalf("resume = %+v, want a verified turn", verification)
		}
		if len(turn.calls) != 1 {
			t.Fatalf("the check ran %d turn(s), want one", len(turn.calls))
		}
		call := turn.calls[0]
		if !slices.Equal(call.arguments, ompResumeArguments(ompFixtureRef.ID, "Reply with exactly: PROBE-REPLY-OK")) {
			t.Errorf("the check ran %v, want the invocation the report prints", call.arguments)
		}
		if call.cwd != cwd {
			t.Errorf("the turn ran in %q, want the session's own working directory %q", call.cwd, cwd)
		}
		if !strings.Contains(verification.Command, "--resume") {
			t.Errorf("command = %q, want the printed invocation", verification.Command)
		}
	})

	t.Run("no prompt", func(t *testing.T) {
		turn := &fakeOMPTurn{growPath: installed, answer: "x"}
		verification := resume(t, turn, "   ")
		if verification.State == VerificationStateVerified || len(turn.calls) != 0 {
			t.Fatalf("resume = %+v, want it refused without running anything", verification)
		}
	})

	t.Run("the harness fails", func(t *testing.T) {
		turn := &fakeOMPTurn{answer: "", diagnostics: "Error: No API key found", runErr: errors.New("exit status 1")}
		verification := resume(t, turn, "a prompt")
		if verification.State == VerificationStateVerified {
			t.Fatalf("resume = %+v, want it refused", verification)
		}
		if !strings.Contains(verification.Detail, "No API key found") {
			t.Errorf("detail = %q, want the harness's own words quoted", verification.Detail)
		}
	})

	t.Run("an answer with no growth", func(t *testing.T) {
		turn := &fakeOMPTurn{answer: "PROBE-REPLY-OK"}
		verification := resume(t, turn, "a prompt")
		if verification.State == VerificationStateVerified {
			t.Fatalf("resume = %+v, want it refused: the session file did not grow", verification)
		}
		if !strings.Contains(verification.Detail, "did not") {
			t.Errorf("detail = %q, want it to say the session did not grow", verification.Detail)
		}
	})

	t.Run("a binary that is not installed", func(t *testing.T) {
		original := resolveOMPExecutable
		resolveOMPExecutable = func(string) (string, error) { return "", errors.New("not found") }
		t.Cleanup(func() { resolveOMPExecutable = original })
		verification := VerifyOMPResume(context.Background(), OMPResumeOptions{
			Home: filepath.Dir(installed), Binary: OMPBinaryDefault,
			SessionID: ompFixtureRef.ID, ProjectDir: filepath.Base(filepath.Dir(installed)),
			Cwd: cwd, SessionPath: installed, Prompt: "a prompt", Timeout: time.Minute,
		})
		if verification.State == VerificationStateVerified {
			t.Fatalf("resume = %+v, want it refused: the harness is not installed", verification)
		}
	})
}

// TestProbeOMPModelAsksTheHarnessItself pins the fact the route writes into the session: the model a new
// session of this harness would run under, asked of the harness through its own protocol rather than read
// out of a configuration file. The probe must run with --no-session (it writes no session of its own) and
// must report what the harness answered, or the reason it answered nothing.
func TestProbeOMPModelAsksTheHarnessItself(t *testing.T) {
	rpc := &fakeOMPRPC{installed: ""}
	withFakeOMPHarness(t, rpc, nil)
	probe := ProbeOMPModel(context.Background(), "/scratch/home", OMPBinaryDefault, time.Minute, nil)
	if !probe.Found || probe.Provider != "example-proxy" || probe.Model != "example-model-2" {
		t.Fatalf("probe = %+v, want the harness's own answer", probe)
	}
	if len(rpc.calls) != 1 {
		t.Fatalf("the probe ran %d command(s), want one", len(rpc.calls))
	}
	if !slices.Contains(rpc.calls[0].arguments, "--no-session") {
		t.Errorf("the probe ran %v, want --no-session: a preview must leave no session behind",
			rpc.calls[0].arguments)
	}
	if rpc.calls[0].home != "/scratch/home" {
		t.Errorf("the probe ran against %q, want the data directory it was asked about", rpc.calls[0].home)
	}

	t.Run("a harness that answers no model", func(t *testing.T) {
		withFakeOMPHarness(t, &fakeOMPRPC{omitModel: true}, nil)
		probe := ProbeOMPModel(context.Background(), "/scratch/home", OMPBinaryDefault, time.Minute, nil)
		if probe.Found || probe.Detail == "" {
			t.Fatalf("probe = %+v, want it reported as an answer that named no model", probe)
		}
		if !strings.Contains(probe.Detail, "model") {
			t.Errorf("detail = %q, want it to say the answer named no model", probe.Detail)
		}
	})

	t.Run("a harness that cannot start", func(t *testing.T) {
		withFakeOMPHarness(t, &fakeOMPRPC{refuseEveryCall: "Error: No default model selected."}, nil)
		probe := ProbeOMPModel(context.Background(), "/scratch/home", OMPBinaryDefault, time.Minute, nil)
		if probe.Found || probe.Detail == "" {
			t.Fatalf("probe = %+v, want it reported as an answer it could not get", probe)
		}
		if !strings.Contains(probe.Detail, "No default model selected") {
			t.Errorf("detail = %q, want the harness's own refusal quoted", probe.Detail)
		}
	})
}
