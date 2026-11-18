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
	document := openCodeExportDocument{}
	document.Info.ID = sessionID
	document.Info.Location.Directory = directory
	for index := range messages {
		document.Messages = append(document.Messages, json.RawMessage(
			fmt.Sprintf(`{"id":"msg_%d","type":"user","time":{"created":%d},"text":"turn %d","files":[]}`,
				index, 1_700_000_000_000+index, index)))
	}
	encoded, _ := json.Marshal(document)
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
	stubOpenCodeRun(t, &runOpenCodeTurn, func(
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
	stubOpenCodeRun(t, &runOpenCodeTurn, func(
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
	stubOpenCodeRun(t, &runOpenCodeTurn, func(
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
		stubOpenCodeRun(t, &runOpenCodeTurn, fn)
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

// TestProbeOpenCodeVersionReadsTheHarnesssOwnLine pins the probe's own rule: the version is the one the
// harness printed, and a missing binary is an answer rather than an error.
func TestProbeOpenCodeVersionReadsTheHarnesssOwnLine(t *testing.T) {
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
