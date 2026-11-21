package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openCodeExportAnswer builds the harness's own document answer for one session id with n messages, the
// shape its `session export` prints.
func openCodeExportAnswer(sessionID, directory string, messages int) string {
	stored := make([]json.RawMessage, 0, messages)
	for index := range messages {
		stored = append(stored, json.RawMessage(
			fmt.Sprintf(`{"id":"msg_%d","type":"user","time":{"created":%d},"text":"turn %d","files":[]}`,
				index, 1_700_000_000_000+index, index)))
	}
	encoded, _ := json.Marshal(map[string]any{
		"info": map[string]any{
			"id":       sessionID,
			"projects": []string{"the route's own document holds members this check does not ask about"},
			"location": map[string]any{"directory": directory},
		},
		"messages": stored,
	})
	return string(encoded)
}

// TestOpenCodeFactsAskTheHarnesssOwnAgentList pins the route's facts step: the harness's own agent
// listing answers which agent and model a new session of it would run under (the first primary,
// non-hidden entry), a home that resolves no model answers the agent alone with the reason as its detail,
// and a dry run asks nothing at all.
func TestOpenCodeFactsAskTheHarnesssOwnAgentList(t *testing.T) {
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("preparing the home: %v", err)
	}
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	var environment []string
	stubOpenCodeRun(t, &runOpenCodeAgentsProbe, func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		environment = env
		if len(argv) != 3 || argv[0] != "debug" || argv[1] != "agents" {
			return "", "", errors.New("the stub does not implement " + strings.Join(argv, " "))
		}
		return `[
			{"id":"hidden-one","mode":"primary","hidden":true,"model":{"id":"hidden-model","providerID":"p"}},
			{"id":"explore","mode":"subagent","hidden":false,"model":null},
			{"id":"build","mode":"primary","hidden":false,"model":{"id":"deepseek-v4.1-flash-fast","providerID":"tencent-litellm","variant":"max"}}
		]`, "", nil
	})
	facts := openCodeFacts(context.Background(), targetFactsInput{
		Home: home, Probe: harnessProbe{Binary: "opencode", Found: true, Version: "2.2.5"},
		Timeout: 30 * time.Second,
	})
	if facts.CLIVersion != "2.2.5" || facts.CLIVersionSource != "probe" {
		t.Errorf("cli version = %q (%q), want the probed version", facts.CLIVersion, facts.CLIVersionSource)
	}
	if facts.Agent != "build" || facts.ModelProvider != "tencent-litellm" ||
		facts.Model != "deepseek-v4.1-flash-fast" || facts.ModelVariant != "max" {
		t.Errorf("facts = %+v, want the harness's own primary agent and its resolved model", facts)
	}
	if facts.ProviderSource != "the harness's own debug agents answer" {
		t.Errorf("provider source = %q, want the harness's own answer named", facts.ProviderSource)
	}
	if !strings.Contains(facts.ProviderDetail, "tencent-litellm/deepseek-v4.1-flash-fast") {
		t.Errorf("provider detail = %q, want the harness's own answer quoted", facts.ProviderDetail)
	}
	if len(environment) == 0 {
		t.Fatal("the probe ran with no environment")
	}
	rooted := false
	for _, entry := range environment {
		if entry == openCodeDataRootEnv+"="+openCodeDataRoot(home) {
			rooted = true
		}
	}
	if !rooted {
		t.Errorf("environment = %q, want the home's data root named", environment)
	}

	// A home that resolves no model answers the agent and says why there is no model.
	stubOpenCodeRun(t, &runOpenCodeAgentsProbe, func(
		_ context.Context, _ string, _ string, _ []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		return `[{"id":"build","mode":"primary","hidden":false,"model":null}]`, "", nil
	})
	facts = openCodeFacts(context.Background(), targetFactsInput{
		Home: home, Probe: harnessProbe{Binary: "opencode", Found: true},
	})
	if facts.Agent != "build" || facts.Model != "" || facts.ModelProvider != "" {
		t.Errorf("facts = %+v, want the agent answered and no model claimed", facts)
	}
	if !strings.Contains(facts.ProviderDetail, "names no model") {
		t.Errorf("provider detail = %q, want it to state that the home resolves no model", facts.ProviderDetail)
	}

	// A dry run asks nothing: starting the harness against a caller's data directory is what a preview
	// must not do.
	asked := false
	stubOpenCodeRun(t, &runOpenCodeAgentsProbe, func(
		_ context.Context, _ string, _ string, _ []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		asked = true
		return "", "", nil
	})
	facts = openCodeFacts(context.Background(), targetFactsInput{
		Home: home, DryRun: true, Probe: harnessProbe{Binary: "opencode", Found: true},
	})
	if asked {
		t.Error("a dry run started the harness")
	}
	if facts.Agent != "" || facts.Model != "" {
		t.Errorf("facts = %+v, want nothing recorded for a dry run", facts)
	}
	if !strings.Contains(facts.ProviderDetail, "dry run") {
		t.Errorf("provider detail = %q, want the dry run named as the reason", facts.ProviderDetail)
	}
}

// TestVerifyOpenCodeLoadAnswersOnlyForThisSession pins the load level: the harness's own reader must
// answer with this session's document, the same question about an unknown id must be refused naming that
// id, and the check runs with a scratch configuration and state directory so no provider, credential or
// configuration of the caller's is involved.
func TestVerifyOpenCodeLoadAnswersOnlyForThisSession(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	var environment []string
	stubOpenCodeRun(t, &runOpenCodeExportDocument, func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		environment = env
		switch argv[2] {
		case session.SessionID:
			return openCodeExportAnswer(session.SessionID, session.Cwd, 7), "", nil
		case openCodeUnknownSessionID:
			return "", "Session not found: " + openCodeUnknownSessionID + "\n", errors.New("exit status 1")
		default:
			return "", "", errors.New("the stub does not implement " + strings.Join(argv, " "))
		}
	})
	verification := VerifyOpenCodeLoad(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Cwd: session.Cwd,
		Timeout: 30 * time.Second,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("load = %+v, want the harness's own reader to verify the installed session", verification)
	}
	if verification.SessionID != session.SessionID || verification.Path != openCodeStorePath(home) ||
		verification.Cwd != session.Cwd {
		t.Errorf("load = %+v, want the session's own id, the store and its recorded directory", verification)
	}
	if !strings.Contains(verification.Detail, openCodeUnknownSessionID) {
		t.Errorf("detail = %q, want the control arm's own refusal named", verification.Detail)
	}
	got := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		got[strings.ToUpper(name)] = value
	}
	if got["XDG_CONFIG_HOME"] == "" || got["XDG_STATE_HOME"] == "" {
		t.Errorf("environment = %q, want scratch configuration and state directories", environment)
	}
	if _, ok := got["OPENCODE_API_KEY"]; ok {
		t.Error("a credential name reached the load check")
	}

	// A control arm that answers a session is not a contrast: the check cannot be about this session.
	stubOpenCodeRun(t, &runOpenCodeExportDocument, func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		return openCodeExportAnswer(strings.TrimSpace(argv[2]), session.Cwd, 1), "", nil
	})
	verification = VerifyOpenCodeLoad(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Timeout: 30 * time.Second,
	})
	if verification.State == VerificationStateVerified {
		t.Error("a control arm that answered a session verified the load")
	}

	// An answer about another session is not an answer about this one.
	stubOpenCodeRun(t, &runOpenCodeExportDocument, func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		if strings.TrimSpace(argv[2]) == session.SessionID {
			return openCodeExportAnswer("ses_another_00000000000000000000", session.Cwd, 3), "", nil
		}
		return "", "Session not found: " + openCodeUnknownSessionID, errors.New("exit status 1")
	})
	verification = VerifyOpenCodeLoad(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Timeout: 30 * time.Second,
	})
	if verification.State == VerificationStateVerified {
		t.Error("an answer about another session verified the load")
	}
}

// TestVerifyOpenCodeResumeMeasuresGrowthThroughTheHarnesssOwnReader pins the resume level: the turn runs
// through the printed invocation's own words, and the session's growth is read back through the
// harness's own export (a turn appends records to the session it opens) -- so a harness that answered
// without the store reading differently is not a verified resume, exactly as a file-backed route refuses
// one whose file did not grow.
func TestVerifyOpenCodeResumeMeasuresGrowthThroughTheHarnesssOwnReader(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	exports := 0
	var turn []string
	// The resume arm reads the session through its own export (the seam the load level shares) and runs
	// the turn through its own, so a case answers both.
	answer := func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		switch argv[0] {
		case "session":
			exports++
			return openCodeExportAnswer(session.SessionID, session.Cwd, 6+exports), "", nil
		case "run":
			turn = argv
			return `{"type":"text","text":"PROBE-REPLY-OK"}`, "", nil
		default:
			return "", "", errors.New("the stub does not implement " + strings.Join(argv, " "))
		}
	}
	both := func(fn openCodeHarnessRun) {
		stubOpenCodeRun(t, &runOpenCodeResumeTurn, fn)
		stubOpenCodeRun(t, &runOpenCodeExportDocument, fn)
	}
	both(answer)
	verification := VerifyOpenCodeResume(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Cwd: session.Cwd,
		Prompt: "Reply with exactly: PROBE-REPLY-OK", Timeout: 30 * time.Second,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("resume = %+v, want the turn verified", verification)
	}
	if verification.ExitCode == nil || *verification.ExitCode != 0 {
		t.Errorf("exit code = %v, want zero", verification.ExitCode)
	}
	if !strings.Contains(verification.Output, "PROBE-REPLY-OK") {
		t.Errorf("output = %q, want the harness's own answer", verification.Output)
	}
	if !strings.Contains(verification.Detail, "7 message(s) before the turn and 8 after") {
		t.Errorf("detail = %q, want the growth through the harness's own reader", verification.Detail)
	}
	if len(turn) == 0 || turn[0] != "run" || !strings.Contains(strings.Join(turn, " "),
		"--session "+session.SessionID) {
		t.Errorf("the turn ran as %q, want the printed invocation's own words", turn)
	}
	if !strings.HasPrefix(verification.Command, "cd "+shellQuote(session.Cwd)+" && ") ||
		!strings.Contains(verification.Command, openCodeDataRootEnv+"="+shellQuote(openCodeDataRoot(home))) {
		t.Errorf("command = %q, want the printed invocation: the recorded directory and the data root named",
			verification.Command)
	}

	// A turn that answers without the store reading differently is not a resume.
	both(func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		if argv[0] == "session" {
			return openCodeExportAnswer(session.SessionID, session.Cwd, 6), "", nil
		}
		return `{"type":"text","text":"answered"}`, "", nil
	})
	verification = VerifyOpenCodeResume(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Cwd: session.Cwd,
		Prompt: "a turn", Timeout: 30 * time.Second,
	})
	if verification.State == VerificationStateVerified {
		t.Error("a turn that left the session's own reader reading the same messages verified")
	}

	// A failing turn reports the harness's own exit status and words.
	both(func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		if argv[0] == "session" {
			return openCodeExportAnswer(session.SessionID, session.Cwd, 6), "", nil
		}
		return `{"type":"error","error":{"type":"provider.no-route","message":"Model unavailable"}}`,
			"", errors.New("exit status 7")
	})
	verification = VerifyOpenCodeResume(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Cwd: session.Cwd,
		Prompt: "a turn", Timeout: 30 * time.Second,
	})
	if verification.State == VerificationStateVerified {
		t.Error("a failing turn verified")
	}
	// A run that did not exit zero cannot answer a status: the harness's own error is what is reported.
	if verification.ExitCode == nil || *verification.ExitCode == 0 {
		t.Errorf("exit code = %v, want a nonzero status for a failed run", verification.ExitCode)
	}
	if !strings.Contains(verification.Output+verification.Detail, "provider.no-route") {
		t.Errorf("output/detail = %q/%q, want the harness's own failure words carried",
			verification.Output, verification.Detail)
	}

	// A resume with no prompt is not attempted: a prompt becomes part of the session.
	verification = VerifyOpenCodeResume(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Timeout: 30 * time.Second,
	})
	if verification.State == VerificationStateVerified || !strings.Contains(verification.Detail, "prompt") {
		t.Errorf("resume = %+v, want the missing prompt named", verification)
	}
}

// TestOpenCodeResumePinsTheDatabaseItPrints pins the printed environment's own rule: the resume line and
// the resume check both name the data root and the database file inside it, so a caller's inherited
// OPENCODE_DB cannot send the pasted command to another store -- where, measured on 2.2.5, `run --session
// <id>` *creates* a session for the id instead of resolving the installed one. The environment that
// reaches the harness is asserted, not only the dictionary the report renders: what a caller pastes is
// what was verified.
func TestOpenCodeResumePinsTheDatabaseItPrints(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	t.Setenv(openCodeDBEnv, "/tmp/caller-elsewhere.db")
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	var environment []string
	exports := 0
	answer := func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		environment = env
		if argv[0] == "session" {
			exports++
			return openCodeExportAnswer(session.SessionID, session.Cwd, 6+exports), "", nil
		}
		return `{"type":"text","text":"answered"}`, "", nil
	}
	stubOpenCodeRun(t, &runOpenCodeResumeTurn, answer)
	stubOpenCodeRun(t, &runOpenCodeExportDocument, answer)
	verification := VerifyOpenCodeResume(context.Background(), targetVerificationInput{
		Home: home, Binary: "opencode", SessionID: session.SessionID, Cwd: session.Cwd,
		Prompt: "a turn", Timeout: 30 * time.Second,
	})
	if verification.State != VerificationStateVerified {
		t.Fatalf("resume = %+v, want the turn verified", verification)
	}
	got := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		got[strings.ToUpper(name)] = value
	}
	if got[openCodeDBEnv] != openCodeStorePath(home) {
		t.Errorf("%s = %q, want this home's own store %q: the caller's inherited override would send the "+
			"turn to another database", openCodeDBEnv, got[openCodeDBEnv], openCodeStorePath(home))
	}
	if got[openCodeDataRootEnv] != openCodeDataRoot(home) {
		t.Errorf("%s = %q, want the home's data root %q", openCodeDataRootEnv,
			got[openCodeDataRootEnv], openCodeDataRoot(home))
	}
	// The printed line names the same pair, so the environment the harness was run with is the one the
	// report prints.
	pin := openCodeDBEnv + "=" + shellQuote(openCodeStorePath(home))
	if !strings.Contains(verification.Command, pin) {
		t.Errorf("command = %q, want it to pin the store this way: %s", verification.Command, pin)
	}
}

// openCodeExportStubEnv makes a child of this test binary answer one session export, so the arm's own
// runner and walk are exercised without the host's harness -- the way the Codex and Pi turn cases start
// their stub.
const openCodeExportStubEnv = "CCC_TEST_STUB_OPENCODE_EXPORT"

// The stub's document: the session's own id and directory, and enough messages that the document is
// larger than the turn-output excerpt bound by a wide margin while no single message is.
const (
	openCodeExportStubAnswer   = "one session export larger than the excerpt bound"
	openCodeExportStubID       = "ses_exportstub000000000000"
	openCodeExportStubDir      = "/tmp/ccc-export-stub"
	openCodeExportStubMessages = 512
	openCodeExportStubBlock    = 4 << 10
)

// openCodeExportStubHarness prints the stub's document on stdout: one JSON object, the harness's own
// shape, with members the check does not ask about beside the ones it does.
func openCodeExportStubHarness(t *testing.T) {
	t.Helper()
	var document strings.Builder
	document.WriteString(`{"info":{"id":"` + openCodeExportStubID + `","location":{"directory":"` +
		openCodeExportStubDir + `"},"tokens":{"input":1,"output":2},"cost":0}`)
	document.WriteString(`,"messages":[`)
	for index := range openCodeExportStubMessages {
		if index > 0 {
			document.WriteByte(',')
		}
		document.WriteString(fmt.Sprintf(`{"id":"msg_%d","type":"user","text":"`, index))
		document.WriteString(strings.Repeat("x", openCodeExportStubBlock))
		document.WriteString(`"}`)
	}
	document.WriteString(`]}`)
	if document.Len() <= harnessOutputMax {
		t.Fatalf("the stub's document holds %d byte(s), want more than the %d-byte excerpt bound: the case "+
			"is about an export no excerpt could hold", document.Len(), harnessOutputMax)
	}
	fmt.Fprintln(os.Stdout, document.String())
}

// TestOpenCodeExportCarriesADocumentLargerThanTheExcerptBound pins the export arm's own rule: this route's
// load level and its resume level's growth measurement both read the session through the harness's own
// export, and a session whose document is larger than the turn-output bound must be read whole -- a
// truncated document is not an answer, and a check that read one would report a successful native load as
// unverified. The real runner and the real walk are exercised (this test binary is the stub harness), so
// the pipe, the read and the token walk are the production path; the count the walk answers is the whole
// document's own.
func TestOpenCodeExportCarriesADocumentLargerThanTheExcerptBound(t *testing.T) {
	if os.Getenv(openCodeExportStubEnv) != "" {
		openCodeExportStubHarness(t)
		// The harness answers one document and nothing else: the test framework's own summary line is not
		// the harness's output, and this route's walk refuses content after the document it asked for.
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolving the test binary: %v", err)
	}
	t.Setenv(openCodeExportStubEnv, openCodeExportStubAnswer)
	// The runner is driven with the stub's own argument vector -- the test binary is the harness, so the
	// production command line is not what selects the arm here -- and the walk reads exactly what the
	// child printed: the pipe, the read and the token walk are the production path.
	arguments := []string{"-test.run=^TestOpenCodeExportCarriesADocumentLargerThanTheExcerptBound$"}
	output, _, err := runRouteDocument(context.Background(), executable, "", arguments, os.Environ(),
		2*time.Minute, nil)
	if err != nil {
		t.Fatalf("running the stub harness: %v", err)
	}
	if len(output) <= harnessOutputMax {
		t.Fatalf("the stub answered %d byte(s), want more than the %d-byte excerpt bound: the case is "+
			"about an export no excerpt could hold", len(output), harnessOutputMax)
	}
	summary, err := readOpenCodeExportDocument(output)
	if err != nil {
		t.Fatalf("reading the export: %v", err)
	}
	if summary.SessionID != openCodeExportStubID || summary.Directory != openCodeExportStubDir ||
		summary.Messages != openCodeExportStubMessages {
		t.Errorf("summary = %+v, want the stub document's own id, directory and %d message(s): an excerpt "+
			"of the document cannot answer these", summary, openCodeExportStubMessages)
	}
}

// TestOpenCodeExportReaderKeepsOnlyWhatItAsksAbout pins the walk's own reading rule: the members the
// checks ask about are answered, a member they do not ask about is consumed as it arrives, and a value
// that is not the document this route reads is refused rather than read as one -- an empty answer, a JSON
// value that is not an object, a message list that is not an array, and an object with no session id.
func TestOpenCodeExportReaderKeepsOnlyWhatItAsksAbout(t *testing.T) {
	summary, err := readOpenCodeExportDocument(`{"info":{"id":"ses_x","location":{"directory":"/d",` +
		`"extra":{"deep":[1,2,{"a":"b"}]}},"usage":{"input":1}},"parts_unasked":[1,2,3],` +
		`"messages":[{"id":"m1","content":[{"type":"text","text":"hi"}]},{"id":"m2"}]}`)
	if err != nil {
		t.Fatalf("reading a document with unasked members: %v", err)
	}
	if summary.SessionID != "ses_x" || summary.Directory != "/d" || summary.Messages != 2 {
		t.Errorf("summary = %+v, want the document's own id, directory and message count", summary)
	}
	for _, answer := range []string{
		"",
		"[1,2,3]",
		`{"info":{"id":"ses_x"},"messages":{"not":"an array"}}`,
		`{"info":{"location":{"directory":"/d"}},"messages":[]}`,
		`{"info":{"id":"ses_x"},"messages":[]} and then the caller's own words`,
	} {
		if _, err := readOpenCodeExportDocument(answer); err == nil {
			t.Errorf("readOpenCodeExportDocument(%q) answered no error, want the shape refusal it is", answer)
		}
	}
}

// TestProbeOpenCodeBinaryReportsAMissingBinaryAndParsesTheVersionLine pins the two rules this case can
// drive without the host's harness: a missing binary is an answer rather than an error (the sibling
// routes' probes answer it the same way), and the version pattern takes the version out of the line the
// harness prints -- `opencode v2.2.5`, measured on the installed 2.2.5. The found-and-printed path itself
// needs the host's harness and is not driven here, on this route or on the sibling routes.
func TestProbeOpenCodeBinaryReportsAMissingBinaryAndParsesTheVersionLine(t *testing.T) {
	probe := ProbeOpenCodeBinary(context.Background(), missingTargetBinary)
	if probe.Found {
		t.Errorf("probe = %+v, want a missing binary reported as not found", probe)
	}
	if !strings.Contains(probe.Detail, missingTargetBinary) {
		t.Errorf("detail = %q, want the missing executable named", probe.Detail)
	}
	if openCodeVersionPattern.FindString("opencode v2.2.5") != "2.2.5" {
		t.Errorf("version pattern = %q, want the harness's own line to answer its version",
			openCodeVersionPattern.FindString("opencode v2.2.5"))
	}
}
