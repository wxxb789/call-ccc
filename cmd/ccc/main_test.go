package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/ccc"
	"go.kenn.io/agentsview/internal/config"
)

// fixtureRoot is the committed fixture tree: one directory per provider, each of which is that
// provider's source root.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "internal", "ccc", "testdata", "fixtures"))
	if err != nil {
		t.Fatalf("resolving the fixture root: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture root %s: %v", root, err)
	}
	return root
}

// claudeFixtureFile is one committed Claude fixture: the id the provider reports (the file's stem) and
// its path.
type claudeFixtureFile struct {
	sessionID string
	path      string
}

// claudeFixtures returns every committed Claude fixture, keyed by its project directory: each provider
// subdirectory is that provider's source root, and each project directory below it is one session.
func claudeFixtures(t *testing.T) map[string]claudeFixtureFile {
	t.Helper()
	root := filepath.Join(fixtureRoot(t), "claude")
	projects, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading the Claude fixture root: %v", err)
	}
	fixtures := map[string]claudeFixtureFile{}
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, project.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", project.Name(), err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
				continue
			}
			fixtures[project.Name()] = claudeFixtureFile{
				sessionID: strings.TrimSuffix(file.Name(), ".jsonl"),
				path:      filepath.Join(root, project.Name(), file.Name()),
			}
		}
	}
	if len(fixtures) == 0 {
		t.Fatalf("no Claude fixture below %s", root)
	}
	return fixtures
}

// claudeFixture returns the committed Claude fixture for the example-api project: its session id (its
// file name) and path. The id is read from the tree rather than written into this file: the fixture's
// identity is its file name, and a test that repeated it would have to be edited whenever the fixture
// is renewed.
func claudeFixture(t *testing.T) (sessionID, path string) {
	t.Helper()
	fixture, ok := claudeFixtures(t)["-home-user-code-example-api"]
	if !ok {
		t.Fatalf("the example-api Claude fixture is not below the fixture root")
	}
	return fixture.sessionID, fixture.path
}

// isolatedEnvironment points the CLI at throw-away directories: no test may read the developer's
// real data directory, configuration or archive.
func isolatedEnvironment(t *testing.T) (dataDir string) {
	t.Helper()
	root := fixtureRoot(t)
	home := t.TempDir()
	dataDir = filepath.Join(home, "agentsview")
	t.Setenv("HOME", home)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Join(root, "claude"))
	t.Setenv("CODEX_SESSIONS_DIR", filepath.Join(root, "codex"))
	return dataDir
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// stubCodexEnv makes a child of this test binary behave as the target harness.
const stubCodexEnv = "CCC_TEST_STUB_CODEX"

// stubMarkerEnv names a file the stub harness writes when it is started in app-server mode. A run that
// leaves the target harness unstarted (a dry run) is pinned by the file's absence, and a run that starts
// it is pinned by its presence, so the absence is an assertion rather than a stub that never writes.
const stubMarkerEnv = "CCC_TEST_STUB_MARKER"

// The stub harness's modes. Each names one behaviour a native check is about, so the check's own rule
// can be exercised offline instead of only against the real harness (which the gate does not require):
// a loader that answers with the session's own identity and path (stubLoadModeEnv) and one that answers
// with another session's, a thread whose defaults name a provider (stubProviderEnv) or whose explicit
// configuration does (stubConfigProviderEnv), and a turn that appends to the installed session
// (stubTurnModeEnv) or answers without touching it.
const (
	stubLoadModeEnv       = "CCC_TEST_STUB_LOAD"
	stubLoadSelfMode      = "self"
	stubLoadOtherMode     = "other"
	stubLoadOtherPathMode = "other-path"
	stubProviderEnv       = "CCC_TEST_STUB_PROVIDER"
	stubConfigProviderEnv = "CCC_TEST_STUB_CONFIG_PROVIDER"
	stubTurnModeEnv       = "CCC_TEST_STUB_TURN"
	stubTurnGrowMode      = "grow"
	stubTurnSilentMode    = "silent"
	stubTurnFailMode      = "fail"
	// stubTurnFailCode is the status the failing turn exits with: a value no other answer uses, so a
	// case can tell the reported exit code from the constant a report might otherwise carry.
	stubTurnFailCode = 7
)

// TestMain lets this test binary act as a target harness when a child of it is asked to. One case needs
// a harness whose verification runs and fails, and the stub is this binary itself, so the test needs no
// host tooling and no platform-specific script.
func TestMain(m *testing.M) {
	if os.Getenv(stubCodexEnv) != "" {
		os.Exit(stubCodex(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// stubTargetBinary answers the path of a harness whose app server refuses to load a session: this test
// binary, which TestMain routes into stubCodex.
func stubTargetBinary(t *testing.T) string {
	t.Helper()
	t.Setenv(stubCodexEnv, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolving the test binary: %v", err)
	}
	return executable
}

// stubCodex is the stub harness: it answers --version and serves the app-server handshake, so a
// verification really runs, and refuses every other request, so every verification it serves fails.
func stubCodex(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Println("codex-cli 0.0.0-stub")
		return 0
	case "app-server":
		// The marker is written before the handshake so that a started harness leaves it even when the
		// caller stops reading: the dry-run case reads its absence, and the installed case its presence.
		if marker := os.Getenv(stubMarkerEnv); marker != "" {
			if err := os.WriteFile(marker, []byte("started\n"), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, "the stub harness could not write its marker:", err)
				return 1
			}
		}
		return serveStubAppServer()
	case "exec":
		return stubTurn(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "the stub harness does not implement "+args[0])
		return 2
	}
}

// serveStubAppServer answers the two-line handshake a verification opens with, then whatever the
// environment tells it to answer. With no mode set it refuses every request after the handshake, which
// is what makes a load check run and come back not verified.
//
// The modes are the harness behaviours the offline cases pin: a loader that answers with the session's
// own identity and path (stubLoadModeEnv), one that answers with another session's (the same variable,
// stubLoadOtherMode), and a thread whose defaults name a provider (stubProviderEnv). Each mode answers
// exactly the request it is about, and every other request is still refused, so a check a mode is not
// about keeps its own honest answer instead of a green one.
func serveStubAppServer() int {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var message struct {
			ID     *int           `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := decoder.Decode(&message); err != nil {
			return 0
		}
		if message.ID == nil {
			continue // the initialized notification carries no id
		}
		answer := map[string]any{"jsonrpc": "2.0", "id": *message.ID}
		if result, ok := stubAnswer(message.Method, message.Params); ok {
			answer["result"] = result
		} else {
			answer["error"] = map[string]any{"code": -32601, "message": "the stub harness refuses " + message.Method}
		}
		if err := encoder.Encode(answer); err != nil {
			return 1
		}
	}
}

// stubAnswer is what the stub answers for one request: the handshake always, and otherwise only the
// request the mode named by the environment is about.
func stubAnswer(method string, params map[string]any) (map[string]any, bool) {
	switch method {
	case "initialize":
		return map[string]any{}, true
	case "thread/resume":
		mode := os.Getenv(stubLoadModeEnv)
		if mode == "" {
			return nil, false
		}
		id, _ := params["threadId"].(string)
		path, ok := stubSessionPath(id)
		if !ok {
			return nil, false
		}
		// The other-session mode answers with an identity that is not the one the file holds: it is the
		// loader that echoes a thread it did not open, which is what the check must not accept.
		if mode == stubLoadOtherMode {
			id = "00000000-0000-7000-8000-000000000000"
		}
		if mode == stubLoadOtherPathMode {
			path += ".other"
		}
		return map[string]any{"thread": map[string]any{
			"id": id, "sessionId": id, "path": path, "historyMode": "legacy",
		}}, true
	case "thread/start":
		provider := os.Getenv(stubProviderEnv)
		if provider == "" {
			return nil, false
		}
		return map[string]any{"thread": map[string]any{"modelProvider": provider, "model": "a-stub-model"}}, true
	case "config/read":
		provider := os.Getenv(stubConfigProviderEnv)
		if provider == "" {
			return nil, false
		}
		return map[string]any{"config": map[string]any{
			"model_provider": provider, "model": "a-stub-model",
		}}, true
	}
	return nil, false
}

// stubTurn answers the non-interactive turn the environment describes: one that appends a record to the
// installed session (stubTurnGrowMode), or one that answers without touching it (stubTurnSilentMode).
func stubTurn(args []string) int {
	if len(args) < 2 || args[0] != "resume" {
		fmt.Fprintln(os.Stderr, "the stub harness does not implement exec "+strings.Join(args, " "))
		return 2
	}
	id := args[1]
	switch mode := os.Getenv(stubTurnModeEnv); mode {
	case stubTurnGrowMode:
		path, ok := stubSessionPath(id)
		if !ok {
			fmt.Fprintln(os.Stderr, "the stub harness found no installed session for "+id)
			return 1
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			fmt.Fprintln(os.Stderr, "the stub harness could not open the installed session:", err)
			return 1
		}
		defer file.Close()
		record := `{"timestamp":"2026-05-04T10:00:00Z","type":"event_msg",` +
			`"payload":{"type":"agent_message","message":"a stub turn"}}` + "\n"
		if _, err := file.WriteString(record); err != nil {
			fmt.Fprintln(os.Stderr, "the stub harness could not append the turn:", err)
			return 1
		}
		fmt.Println("the stub harness answered from the installed session")
		return 0
	case stubTurnSilentMode:
		fmt.Println("the stub harness answered without appending to the installed session")
		return 0
	case stubTurnFailMode:
		fmt.Fprintln(os.Stderr, "the stub harness could not continue the session")
		return stubTurnFailCode
	default:
		fmt.Fprintln(os.Stderr, "the stub harness does not implement exec resume")
		return 2
	}
}

// stubSessionPath finds the session file for one identity under the data directory the runner set, the
// way the target's own resume path discovers it.
func stubSessionPath(id string) (string, bool) {
	home := os.Getenv("CODEX_HOME")
	if strings.TrimSpace(home) == "" {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
	if err != nil || len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// TestInspectJSONReportsIdentityAndConversation is the CLI half of the goal's first clause: a
// separately buildable binary inspects a real, sanitized session and reports usable identity, agent,
// source and conversation information — with stdout holding only the JSON document.
func TestInspectJSONReportsIdentityAndConversation(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, "inspect", "claude:"+sessionID, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var inspection ccc.Inspection
	if err := json.Unmarshal([]byte(stdout), &inspection); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if inspection.Agent != "Claude Code" {
		t.Errorf("agent = %q", inspection.Agent)
	}
	if inspection.Source == nil {
		t.Fatalf("no source report: %s", stdout)
	}
	source := inspection.Source
	if source.Identity.Provider != "claude" || source.Identity.SessionID == "" {
		t.Errorf("identity = %+v", source.Identity)
	}
	if !strings.HasSuffix(source.Identity.SourcePath, sessionID+".jsonl") {
		t.Errorf("source path = %q", source.Identity.SourcePath)
	}
	if source.Counts.Messages != 8 || source.Counts.ToolCalls != 1 || source.Counts.CompactBoundaries != 1 {
		t.Errorf("counts = %+v", source.Counts)
	}
	if source.Fidelity.MalformedLines != 1 {
		t.Errorf("fidelity = %+v", source.Fidelity)
	}
	if len(source.ToolNames) != 1 || source.ToolNames[0] != "Read" {
		t.Errorf("tool names = %v", source.ToolNames)
	}
	if inspection.Archive != nil {
		t.Errorf("an archive was reported although the data directory is empty: %+v", inspection.Archive)
	}
	if !strings.Contains(stdout, `"upstream_revision": "`+upstreamRevision+`"`) {
		t.Errorf("the JSON omits the carried revision: %s", stdout)
	}
}

// TestInspectHumanOutputIsReadable pins the human shape an operator reads first.
func TestInspectHumanOutputIsReadable(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, "inspect", "claude:"+sessionID)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"claude:" + sessionID,
		"agent:        Claude Code",
		"conversation: 8 message(s)",
		"tools:        Read",
		"1 malformed line(s)",
		"carried:      AgentsView " + upstreamRevision,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output is missing %q:\n%s", want, stdout)
		}
	}
}

// TestInspectReportsArchiveStateWithoutCreatingOne pins that a data directory holding no archive is
// reported as such (not as a failure) and that reading installs nothing.
func TestInspectReportsArchiveStateWithoutCreatingOne(t *testing.T) {
	dataDir := isolatedEnvironment(t)
	if _, err := os.Stat(filepath.Join(dataDir, "sessions.db")); !os.IsNotExist(err) {
		t.Fatalf("the isolated data directory already holds an archive: %v", err)
	}
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, "inspect", "claude:"+sessionID)
	if code != 0 {
		t.Fatalf("inspection failed with exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "archive:      none at ") {
		t.Errorf("the inspection does not report that no archive exists: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "sessions.db")); !os.IsNotExist(err) {
		t.Errorf("inspect created an archive: %v", err)
	}
}

// TestInspectReadsTheArchiveItIsGiven covers the other half of the archive contract through the CLI:
// after the inherited sync engine has written the fixture into an isolated data directory, an
// inspection of that directory reports the archived session and how it compares with the source read.
func TestInspectReadsTheArchiveItIsGiven(t *testing.T) {
	dataDir := isolatedEnvironment(t)
	cfg, err := config.LoadReadOnly()
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	cfg.DataDir = dataDir
	cfg.DBPath = filepath.Join(dataDir, "sessions.db")
	ingestor, err := ccc.NewIngestor(cfg, cfg.DBPath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	sessionID, fixture := claudeFixture(t)
	if err := ingestor.IngestPaths(context.Background(), fixture); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}

	code, stdout, stderr := runCLI(t, "inspect", "claude:"+sessionID, "--data-dir", dataDir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"archive:      " + filepath.Join(dataDir, "sessions.db") + " [tool-payloads]",
		"archived:     7 message(s)",
		"agreement:    every archived message matches the source read",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output is missing %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stdout, "tool-result-only message(s) are stored in the archive folded") {
		t.Errorf("the inspection does not explain the folded tool-result row:\n%s", stdout)
	}
}

// TestInspectResolvesAnArchiveBySourcePath pins the reference form the README documents against a
// data directory that holds the session: `ccc inspect claude:/path/to/<id>.jsonl --data-dir DIR` has
// to report the archived session, not say it is not in the archive yet.
func TestInspectResolvesAnArchiveBySourcePath(t *testing.T) {
	dataDir := isolatedEnvironment(t)
	cfg, err := config.LoadReadOnly()
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	cfg.DataDir = dataDir
	cfg.DBPath = filepath.Join(dataDir, "sessions.db")
	ingestor, err := ccc.NewIngestor(cfg, cfg.DBPath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	_, fixture := claudeFixture(t)
	if err := ingestor.IngestPaths(context.Background(), fixture); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}

	code, stdout, stderr := runCLI(t, "inspect", "claude:"+fixture, "--data-dir", dataDir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "archived id:") {
		t.Errorf("the archive was not reported for a path reference:\n%s", stdout)
	}
	if strings.Contains(stdout, "is not in "+cfg.DBPath+" yet") {
		t.Errorf("the inspection says the session is not in the archive:\n%s", stdout)
	}
}

// TestInspectHumanOutputForAnArchiveOnlySession pins the human lines an operator reads when the source
// is gone but the archive holds the session (writeHuman's "not readable here" branch), and that the
// missing source is not reported as a comparison: the agreement line says the comparison did not run,
// and the JSON leaves the field null rather than shipping a false that a real mismatch would share.
func TestInspectHumanOutputForAnArchiveOnlySession(t *testing.T) {
	dataDir := isolatedEnvironment(t)
	cfg, err := config.LoadReadOnly()
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	cfg.DataDir = dataDir
	cfg.DBPath = filepath.Join(dataDir, "sessions.db")
	ingestor, err := ccc.NewIngestor(cfg, cfg.DBPath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	sessionID, fixture := claudeFixture(t)
	if err := ingestor.IngestPaths(context.Background(), fixture); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	// The provider root the CLI resolves is emptied: the source exists only in the archive now.
	t.Setenv("CLAUDE_PROJECTS_DIR", t.TempDir())

	code, stdout, stderr := runCLI(t, "inspect", "claude:"+sessionID, "--data-dir", dataDir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"source:       not readable here (see warnings)",
		"archive:      " + filepath.Join(dataDir, "sessions.db") + " [tool-payloads]",
		"agreement:    not compared (the source was not readable)",
		"warning:      no readable source session for claude:" + sessionID,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output is missing %q:\n%s", want, stdout)
		}
	}
	// The JSON must not carry a comparison result either: false would be indistinguishable from a
	// comparison that ran and found a difference, so the field is null when no source was read.
	code, stdout, stderr = runCLI(t, "inspect", "claude:"+sessionID, "--data-dir", dataDir, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var document struct {
		Archive struct {
			ContentShown         string `json:"content_shown"`
			SameMessagesAsSource *bool  `json:"same_messages_as_source"`
		} `json:"archive"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, `"same_messages_as_source": null`) {
		t.Errorf("the JSON does not report the comparison as not run: %s", stdout)
	}
	if document.Archive.SameMessagesAsSource != nil {
		t.Errorf("same_messages_as_source = %v, want null", *document.Archive.SameMessagesAsSource)
	}
	if document.Archive.ContentShown != "tool-payloads" {
		t.Errorf("content_shown = %q, want %q", document.Archive.ContentShown, "tool-payloads")
	}
}

// TestUsageErrors pins the command line contract: a reference without a provider, an unknown
// provider, a missing operand, an unknown option and an unknown command are usage errors, not silent
// guesses. A usage error's text belongs on stderr: stdout carries a result, and a help request is the
// one flag answer that is not a usage error (TestHelpNeedsNoEnvironment).
func TestUsageErrors(t *testing.T) {
	isolatedEnvironment(t)
	cases := [][]string{
		{"inspect"},
		{"inspect", "9f"},
		{"inspect", "no-such-agent:whatever"},
		{"inspect", "claude:"},
		{"inspect", "claude:9f2c1d47", "--no-such-option"},
		{"frobnicate"},
		{},
	}
	for _, args := range cases {
		code, stdout, stderr := runCLI(t, args...)
		if code != exitUsage {
			t.Errorf("ccc %v: exit %d, want %d (stderr %q)", args, code, exitUsage, stderr)
		}
		if stdout != "" {
			t.Errorf("ccc %v: a usage error wrote to stdout (%q)", args, stdout)
		}
	}
}

// TestInspectAcceptsAStoredAgentLabel pins that the CLI understands a provider name that is a stored
// agent label rather than a registry provider of its own.
//
// The Codebuff provider records its free-tier sessions under the Freebuff label, and the identity the
// archive reports for one (`freebuff:<project>:<timestamp>`) is exactly what a caller passes back to
// inspect it. The CLI refused that name as an unknown provider before the core saw it -- exit 2, with
// no lookup at all -- so a session the archive holds could not be named. The answer here is a missing
// session (the isolated environment holds no such archive), not a usage error, and the failure names
// the label and the providers that were tried.
func TestInspectAcceptsAStoredAgentLabel(t *testing.T) {
	isolatedEnvironment(t)
	code, stdout, stderr := runCLI(t, "inspect", "freebuff:example-project:2026-07-15T20-01-32.065Z")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if stdout != "" {
		t.Errorf("a failed run wrote a result to stdout (%q)", stdout)
	}
	if strings.Contains(stderr, "unknown provider") {
		t.Errorf("the stored agent label was refused as an unknown provider: %s", stderr)
	}
	for _, want := range []string{"freebuff:example-project:2026-07-15T20-01-32.065Z", "freebuff"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the failure does not name %q: %s", want, stderr)
		}
	}
}

// TestMissingSessionIsAFailureWithACandidateList pins the not-found path: the message names the
// providers that were tried, so an agent can correct the reference instead of re-reading --help.
func TestMissingSessionIsAFailureWithACandidateList(t *testing.T) {
	isolatedEnvironment(t)
	code, _, stderr := runCLI(t, "inspect", "claude:00000000-0000-4000-8000-000000000000")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if !strings.Contains(stderr, "claude") || !strings.Contains(stderr, "codex") {
		t.Errorf("the failure does not name the providers that were tried: %s", stderr)
	}
}

// TestNoResultExitKeepsTheDiagnostics pins what the CLI prints when neither the source side nor the
// archive answers a reference: the inspection's own diagnostics reach stderr, and stdout carries
// nothing, because a failed run has no result to report.
//
// The branch used to print "no session found" and nothing else, which discarded the reason the
// inspection held: an operational provider failure (the Cursor provider's retryable store failure is
// the inherited shape) reaches an inspection as a warning while the archive is consulted, and with no
// archive holding the session the operator was told the session does not exist instead of why it could
// not be read. Both diagnostics this branch can reach without a provider failure are pinned here -- a
// reference nothing readable answers, and one recorded on another machine -- and both spellings of the
// command line, so the `--json` path cannot swallow a diagnostic either.
func TestNoResultExitKeepsTheDiagnostics(t *testing.T) {
	dataDir := isolatedEnvironment(t)
	const unknown = "00000000-0000-4000-8000-000000000000"
	cases := []struct {
		name     string
		operand  string
		diagnose string
	}{
		{
			name:     "nothing readable answers",
			operand:  "claude:" + unknown,
			diagnose: "no readable source session for claude:" + unknown,
		},
		{
			name:     "recorded on another machine",
			operand:  "claude:host-a~" + unknown,
			diagnose: "was recorded on another machine",
		},
	}
	for _, testCase := range cases {
		for _, asJSON := range []bool{false, true} {
			spelling := ""
			if asJSON {
				spelling = " --json"
			}
			t.Run(testCase.name+spelling, func(t *testing.T) {
				args := []string{"inspect", testCase.operand, "--data-dir", dataDir}
				if asJSON {
					args = append(args, "--json")
				}
				code, stdout, stderr := runCLI(t, args...)
				if code != exitFailure {
					t.Errorf("ccc %v: exit %d, want %d (stderr %q)", args, code, exitFailure, stderr)
				}
				if stdout != "" {
					t.Errorf("ccc %v: a failed inspection wrote to stdout (%q)", args, stdout)
				}
				if !strings.Contains(stderr, testCase.diagnose) {
					t.Errorf("ccc %v: the inspection's diagnostic %q is missing from stderr: %s",
						args, testCase.diagnose, stderr)
				}
				// The archive is not there either, and that is the other warning the inspection held:
				// it reaches stderr too rather than being collapsed into "no session found".
				if !strings.Contains(stderr, "no archive at ") {
					t.Errorf("ccc %v: the missing-archive diagnostic is absent: %s", args, stderr)
				}
				if !strings.Contains(stderr, "no session found for "+testCase.operand) {
					t.Errorf("ccc %v: the not-found line is absent: %s", args, stderr)
				}
			})
		}
	}
}

// TestHelpNeedsNoEnvironment pins that a first-time user can read the usage without a data directory,
// and that asking `inspect` for it is a help request rather than a command line that could not be
// understood: the usage text is the answer, so it goes to stdout and the exit code is the success one.
func TestHelpNeedsNoEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTSVIEW_DATA_DIR", filepath.Join(t.TempDir(), "absent"))
	code, stdout, _ := runCLI(t, "help")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"ccc inspect", "--json", "exit codes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help is missing %q", want)
		}
	}
	for _, args := range [][]string{{"inspect", "-h"}, {"inspect", "--help"}} {
		code, stdout, stderr := runCLI(t, args...)
		if code != exitOK {
			t.Fatalf("ccc %v: exit %d, want %d (stderr %q)", args, code, exitOK, stderr)
		}
		if !strings.Contains(stdout, "usage: ccc inspect") {
			t.Errorf("ccc %v: stdout does not carry the usage line: %q", args, stdout)
		}
	}
}

// continueEnvironment points the CLI at throw-away directories for a continuation: the committed
// fixture tree as the source, an empty directory as the target harness's data directory, and a target
// binary name no host has, so the native half is exercised deterministically instead of probing
// whatever the machine running the test happens to have installed.
func continueEnvironment(t *testing.T) (dataDir, targetHome string) {
	t.Helper()
	dataDir = isolatedEnvironment(t)
	targetHome = t.TempDir()
	t.Setenv("CODEX_HOME", targetHome)
	return dataDir, targetHome
}

// shellUnsafeHome answers a target data directory whose path a POSIX shell would act on — it holds a
// space — and points the CLI at it. The resume line is rendered the way a POSIX shell reads it, so a
// home like this one is quoted on every host, while a host's own temp path is spelled differently:
// bare where nothing in it is shell syntax (POSIX) and quoted where it holds backslashes (Windows). An
// assertion written against the host's own home therefore pins whichever arm that host happens to
// spell and fails on the other, which is not the rule: the rule is that the line selects the directory
// the session was installed into, quoted exactly when a shell would otherwise act on the word.
func shellUnsafeHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "codex home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatalf("creating the target home: %v", err)
	}
	t.Setenv("CODEX_HOME", home)
	return home
}

// continueArgs is the common invocation every continuation test starts from.
func continueArgs(sessionID string, extra ...string) []string {
	args := []string{"continue", "claude:" + sessionID, "--to", "codex", "--target-binary", missingTargetBinary}
	return append(args, extra...)
}

// missingTargetBinary is a name no host has: the CLI tests must not depend on the machine's tooling.
const missingTargetBinary = "ccc-no-such-target-harness"

// TestContinueJSONDistinguishesConversionInstallationAndVerification is the CLI half of the goal's
// machine-readable clause: one JSON object on stdout reports what was converted, where it was
// installed, and which verification actually ran — three states a reader can tell apart.
func TestContinueJSONDistinguishesConversionInstallationAndVerification(t *testing.T) {
	continueEnvironment(t) // the fixture tree as the source and an isolated data directory
	targetHome := shellUnsafeHome(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	var document struct {
		UpstreamRevision string `json:"upstream_revision"`
		Target           string `json:"target"`
		Conversion       struct {
			State    string `json:"state"`
			ReadFrom string `json:"read_from"`
			DryRun   bool   `json:"dry_run"`
			Carried  struct {
				Messages  int `json:"messages"`
				ToolCalls int `json:"tool_calls"`
			} `json:"carried"`
			Losses []struct {
				Category    string `json:"category"`
				Disposition string `json:"disposition"`
				Count       int    `json:"count"`
				Detail      string `json:"detail"`
			} `json:"losses"`
		} `json:"conversion"`
		TargetSession struct {
			SessionID        string `json:"session_id"`
			Path             string `json:"path"`
			ResumeCommand    string `json:"resume_command"`
			BinaryFound      bool   `json:"binary_found"`
			CLIVersion       string `json:"cli_version"`
			CLIVersionSource string `json:"cli_version_source"`
			FormatLimits     []struct {
				Category string `json:"category"`
				Detail   string `json:"detail"`
			} `json:"format_limits"`
			ResumeArguments     []string          `json:"resume_arguments"`
			ExecResumeArguments []string          `json:"exec_resume_arguments"`
			ResumeEnvironment   map[string]string `json:"resume_environment"`
		} `json:"target_session"`
		Installation struct {
			State            string `json:"state"`
			Created          bool   `json:"created"`
			SourceCheckState string `json:"source_check_state"`
			Mechanism        string `json:"mechanism"`
		} `json:"installation"`
		Verification struct {
			Structure struct {
				State string `json:"state"`
			} `json:"structure"`
			Load struct {
				State  string `json:"state"`
				Detail string `json:"detail"`
			} `json:"load"`
			Resume struct {
				State string `json:"state"`
			} `json:"resume"`
		} `json:"verification"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	// The parsed assertions below use the exported state constants; the document's own spellings are the
	// wire contract the plan records, so they are pinned literally here.
	for _, spelling := range []string{`"state": "converted"`, `"state": "installed"`,
		`"source_check_state": "unchanged"`} {
		if !strings.Contains(stdout, spelling) {
			t.Errorf("the JSON does not spell a state as documented (%s):\n%s", spelling, stdout)
		}
	}
	if document.UpstreamRevision != upstreamRevision {
		t.Errorf("upstream_revision = %q, want %q", document.UpstreamRevision, upstreamRevision)
	}
	if document.Target != "codex" || document.Conversion.State != ccc.ConversionStateConverted ||
		document.Conversion.ReadFrom != "source" || document.Conversion.DryRun {
		t.Errorf("conversion block = %+v", document.Conversion)
	}
	if document.Conversion.Carried.Messages != 5 || document.Conversion.Carried.ToolCalls != 1 {
		t.Errorf("carried = %+v", document.Conversion.Carried)
	}
	if len(document.Conversion.Losses) == 0 {
		t.Error("the JSON reports no loss categories at all")
	}
	for _, loss := range document.Conversion.Losses {
		if loss.Category == "" || loss.Detail == "" || loss.Disposition == "" {
			t.Errorf("a loss entry is not readable: %+v", loss)
		}
	}
	if document.Installation.State != ccc.InstallationStateInstalled || !document.Installation.Created ||
		document.Installation.SourceCheckState != ccc.SourceCheckUnchanged ||
		document.Installation.Mechanism == "" {
		t.Errorf("installation block = %+v", document.Installation)
	}
	if !strings.HasPrefix(document.TargetSession.Path, targetHome) {
		t.Errorf("the session was installed at %q, outside %q", document.TargetSession.Path, targetHome)
	}
	if document.TargetSession.SessionID == "" ||
		!strings.Contains(document.TargetSession.ResumeCommand, document.TargetSession.SessionID) {
		t.Errorf("the report does not name a resumable identity: %+v", document.TargetSession)
	}
	// The invocation is exact, not prose: the argument vector and the environment are what a caller
	// runs, and both command lines have to select the data directory the session was installed into.
	// Codex takes that directory from CODEX_HOME alone -- it has no flag for it -- so a line without it
	// would open the caller's own default home.
	wantResume := []string{missingTargetBinary, "resume", document.TargetSession.SessionID}
	wantExecResume := []string{missingTargetBinary, "exec", "resume", document.TargetSession.SessionID, "<prompt>"}
	if !slices.Equal(document.TargetSession.ResumeArguments, wantResume) {
		t.Errorf("resume_arguments = %q, want %q", document.TargetSession.ResumeArguments, wantResume)
	}
	if !slices.Equal(document.TargetSession.ExecResumeArguments, wantExecResume) {
		t.Errorf("exec_resume_arguments = %q, want %q", document.TargetSession.ExecResumeArguments, wantExecResume)
	}
	if len(document.TargetSession.ResumeEnvironment) != 1 ||
		document.TargetSession.ResumeEnvironment["CODEX_HOME"] != targetHome {
		t.Errorf("resume_environment = %v, want CODEX_HOME=%s", document.TargetSession.ResumeEnvironment, targetHome)
	}
	// The home is single-quoted because of the space in it (the renderer quotes a word exactly when a
	// POSIX shell would act on it); the harness name and the session id hold nothing a shell would act
	// on, so both stand bare.
	if want := "CODEX_HOME='" + targetHome + "' " + missingTargetBinary + " resume " +
		document.TargetSession.SessionID; document.TargetSession.ResumeCommand != want {
		t.Errorf("resume_command = %q, want %q", document.TargetSession.ResumeCommand, want)
	}
	if document.TargetSession.BinaryFound {
		t.Errorf("the report claims to have found %q", missingTargetBinary)
	}
	if document.TargetSession.CLIVersion == "" || document.TargetSession.CLIVersionSource != "writer-pin" {
		t.Errorf("cli version report = %+v", document.TargetSession)
	}
	if len(document.TargetSession.FormatLimits) == 0 {
		t.Error("the JSON states no format limits")
	}
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure verification = %+v", document.Verification.Structure)
	}
	if document.Verification.Load.State != "not-verified" ||
		!strings.Contains(document.Verification.Load.Detail, missingTargetBinary) {
		t.Errorf("load verification = %+v, want an explicit not-verified naming the absent harness",
			document.Verification.Load)
	}
	if document.Verification.Resume.State != ccc.VerificationStateNotRequested {
		t.Errorf("resume verification = %+v, want %s: this run asked for the load level only, and resume "+
			"is the one state a real turn earns", document.Verification.Resume,
			ccc.VerificationStateNotRequested)
	}
	if strings.Contains(stdout, "ccc:") {
		t.Errorf("stdout carries a diagnostic: %s", stdout)
	}
}

// claudeOpeningPrompt reads the first user record's own text out of a Claude transcript: the words a
// continuation report must not carry. It is read from the fixture rather than repeated here, the way the
// fixture's id is.
func claudeOpeningPrompt(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	var record struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(first), &record); err != nil {
		t.Fatalf("decoding the transcript's first record: %v", err)
	}
	if strings.TrimSpace(record.Message.Content) == "" {
		t.Fatalf("the transcript's first record carries no user text: %s", first)
	}
	return record.Message.Content
}

// sourceContentNeedles answers every piece of source content a conversion report and the CLI's wire
// document must not carry: each message's dialogue and rendered content, every recorded reasoning trace,
// every tool call's arguments, and every tool result's output. The needles are read out of the
// conversation the committed fixture makes -- through the same inherited reader the conversion uses --
// so the assertions do not repeat fixture text and they cover the shapes the rule names, not only the
// opening prompt.
func sourceContentNeedles(t *testing.T, cfg config.Config, sessionID string) []string {
	t.Helper()
	conversation, err := ccc.NewSourceReader(cfg).Read(context.Background(),
		ccc.Ref{Provider: "claude", ID: sessionID})
	if err != nil {
		t.Fatalf("reading the fixture conversation: %v", err)
	}
	var needles []string
	add := func(text string) {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			needles = append(needles, trimmed)
		}
	}
	for _, message := range conversation.Messages {
		add(message.Dialogue)
		add(message.Content)
		add(message.Thinking)
		for _, call := range message.ToolCalls {
			add(call.InputJSON)
		}
		for _, result := range message.ToolResults {
			add(result.ContentRaw)
		}
	}
	if len(needles) == 0 {
		t.Fatal("the fixture conversation holds no content: the content rule would be asserted vacuously")
	}
	return needles
}

// TestContinueJSONCarriesNoSourceContent pins the wire half of the content rule: ordinary `ccc continue
// --json` output — installed and dry run, the reports a caller collects — must not carry the source's
// opening prompt, its dialogue, its tool arguments or its tool output. The core design forbids logging
// conversation bodies or full prompts by default, and the preview `ccc inspect` shows is that command's
// own field (it is the one that asks to see the session).
func TestContinueJSONCarriesNoSourceContent(t *testing.T) {
	continueEnvironment(t)
	sessionID, path := claudeFixture(t)
	prompt := claudeOpeningPrompt(t, path)
	cfg, err := config.LoadReadOnly()
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	needles := append(sourceContentNeedles(t, cfg, sessionID), prompt)
	for _, extra := range [][]string{{"--json"}, {"--dry-run", "--json"}} {
		code, stdout, stderr := runCLI(t, continueArgs(sessionID, extra...)...)
		if code != exitOK {
			t.Fatalf("exit %d for %v, stderr: %s", code, extra, stderr)
		}
		if strings.Contains(stdout, "first_user_message") {
			t.Errorf("the %v report carries a first_user_message field:\n%s", extra, stdout)
		}
		for _, needle := range needles {
			if strings.Contains(stdout, needle) {
				t.Errorf("the %v report carries source content %q:\n%s", extra, needle, stdout)
			}
		}
	}
}

// TestContinueHumanBlockCarriesNoSourceContent pins the same rule for the human block: `ccc continue`
// without `--json` prints the report to stdout, and a person reading it must not be handed the
// conversation's words, its tool arguments or its tool output either. The block's own lines are asserted
// to be there as well, so the case cannot pass by the report being empty.
func TestContinueHumanBlockCarriesNoSourceContent(t *testing.T) {
	continueEnvironment(t)
	sessionID, path := claudeFixture(t)
	prompt := claudeOpeningPrompt(t, path)
	cfg, err := config.LoadReadOnly()
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	needles := append(sourceContentNeedles(t, cfg, sessionID), prompt)
	for _, extra := range [][]string{{}, {"--dry-run"}} {
		code, stdout, stderr := runCLI(t, continueArgs(sessionID, extra...)...)
		if code != exitOK {
			t.Fatalf("exit %d for %v, stderr: %s", code, extra, stderr)
		}
		for _, want := range []string{"source file:", "converted:", "structure:"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("the %v human block is missing its own line %q:\n%s", extra, want, stdout)
			}
		}
		for _, needle := range needles {
			if strings.Contains(stdout, needle) {
				t.Errorf("the %v human block carries source content %q:\n%s", extra, needle, stdout)
			}
		}
	}
}

// The two prompts the inherited Codex reader reads differently from the way they were written. The
// first is the injected-context envelope itself -- the prefix Codex writes for the AGENTS.md context it
// injects, which its own reader therefore classifies as injected context rather than as a turn -- and
// the reader reads no message for it. The second carries text after a complete envelope, and the reader
// reads that text: the shape a later review found failing, because the check resolved the written
// message away and then compared the reader's shorter reading against the document.
const (
	agentsMDPrompt    = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	envelopeAndPrompt = "<INSTRUCTIONS>quoted rules</INSTRUCTIONS>\nReview the changes"
)

// envelopePromptTranscript writes a native-shaped Claude transcript whose opening turn is prompt, under
// a throw-away project root, and points CLAUDE_PROJECTS_DIR at it. It returns the session id, which is
// the file's own name.
func envelopePromptTranscript(t *testing.T, sessionID, prompt string) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, "claude", "-home-user-code-example-api")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	lines := []string{
		`{"cwd":"/home/user/code/example-api","message":{"content":` + mustJSON(t, prompt) + `},` +
			`"sessionId":"` + sessionID + `","timestamp":"2026-05-04T09:15:00.000Z","type":"user",` +
			`"version":"1.0.30"}`,
		`{"message":{"content":[{"text":"I read the policy.","type":"text"}],` +
			`"model":"claude-sonnet-4-5"},"timestamp":"2026-05-04T09:15:04.000Z","type":"assistant",` +
			`"version":"1.0.30"}`,
	}
	path := filepath.Join(directory, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Join(root, "claude"))
	return sessionID
}

// mustJSON is one string as a JSON literal.
func mustJSON(t *testing.T, text string) string {
	t.Helper()
	encoded, err := json.Marshal(text)
	if err != nil {
		t.Fatalf("encoding %q: %v", text, err)
	}
	return string(encoded)
}

// writerJSON is one string as the JSON literal the writer's own encoder writes. The target stores a
// conversation's text as it was recorded, so an installed session holds the words themselves rather than
// JSON's browser-safe spelling of any markup in them (html escaping off): a prompt holding a tag is in
// the file as that tag, and this is the literal a read of the file has to find.
func writerJSON(t *testing.T, text string) string {
	t.Helper()
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		t.Fatalf("encoding %q: %v", text, err)
	}
	return strings.TrimSuffix(encoded.String(), "\n")
}

// TestContinueInstallsPromptsTheReaderReadsDifferently pins the two runs a review found failing, one per
// shape of the inherited Codex reader's projection. A valid Claude prompt that opens with `# AGENTS.md`
// is carried into the installed session verbatim, but the reader reads such a text block as injected
// context and so reads *no message* for it; a prompt that carries text after a complete envelope is read
// back *shorter* than it was written. The structural check compared the document against those reads and
// reported the message as missing from the file that holds it -- for the second shape it resolved the
// written message away and then compared the reader's shorter reading as evidence the file did not hold
// -- so the run exited nonzero, with `--verify none` too, because the structural check always runs.
//
// The fix keeps the message in the file and counts it: the check asks the file for the message's own
// dialogue record, and reports the reader's projection instead of a loss. These cases are end to end and
// native-shaped -- a real transcript, read by the inherited parser, converted, installed, and read back
// by the inherited Codex reader -- and each runs every way the check can: installed, installed with
// `--verify none` (the spelling a finding named, since the structural check runs whatever level was
// asked for) and a dry run. They assert the three things that matter in each: the run succeeds, the
// report says which messages the read-back does not show with their own text, and the file really holds
// the prompt.
func TestContinueInstallsPromptsTheReaderReadsDifferently(t *testing.T) {
	isolatedEnvironment(t) // HOME, AGENTSVIEW_DATA_DIR and the committed fixture roots
	t.Setenv("CODEX_HOME", t.TempDir())

	for _, prompt := range []struct {
		name      string
		sessionID string
		text      string
	}{
		{
			name:      "the envelope is the whole prompt",
			sessionID: "01a1e000-3333-7000-8000-00000000000a",
			text:      agentsMDPrompt,
		},
		{
			name:      "the prompt carries text after the envelope",
			sessionID: "01a1e000-3333-7000-8000-00000000000b",
			text:      envelopeAndPrompt,
		},
	} {
		t.Run(prompt.name, func(t *testing.T) {
			sessionID := envelopePromptTranscript(t, prompt.sessionID, prompt.text)
			for _, testCase := range []struct {
				name      string
				extra     []string
				installed bool
			}{
				{name: "installed", extra: []string{"--json"}, installed: true},
				// The finding's own spelling: the structural check runs whatever level was asked for, so
				// `--verify none` is the shape in which the failure was reported as a run that verified
				// nothing.
				{name: "installed, --verify none", extra: []string{"--verify", "none", "--json"}, installed: true},
				{name: "dry run", extra: []string{"--dry-run", "--json"}},
			} {
				t.Run(testCase.name, func(t *testing.T) {
					code, stdout, stderr := runCLI(t, continueArgs(sessionID, testCase.extra...)...)
					if code != exitOK {
						t.Fatalf("exit %d, want %d: the file holds what the report claims it carried\nstderr: %s",
							code, exitOK, stderr)
					}
					var document struct {
						Conversion struct {
							State   string `json:"state"`
							Carried struct {
								UserMessages int `json:"user_messages"`
							} `json:"carried"`
							Losses []struct {
								Category string `json:"category"`
								Count    int    `json:"count"`
							} `json:"losses"`
						} `json:"conversion"`
						TargetSession struct {
							Path string `json:"path"`
						} `json:"target_session"`
						Verification struct {
							Structure struct {
								State               string `json:"state"`
								NotReadBackVerbatim int    `json:"not_read_back_verbatim"`
							} `json:"structure"`
						} `json:"verification"`
						Warnings []string `json:"warnings"`
					}
					if err := json.Unmarshal([]byte(stdout), &document); err != nil {
						t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
					}
					if document.Conversion.State != ccc.ConversionStateConverted {
						t.Errorf("conversion = %+v, want %s", document.Conversion, ccc.ConversionStateConverted)
					}
					// The message is carried, not omitted: it is in the file, and no loss category claims it.
					if document.Conversion.Carried.UserMessages != 1 {
						t.Errorf("carried = %+v, want the prompt carried as a user message",
							document.Conversion.Carried)
					}
					if document.Verification.Structure.State != "verified" {
						t.Fatalf("structure = %+v, want the reader's projection accounted for, not a failure",
							document.Verification.Structure)
					}
					if document.Verification.Structure.NotReadBackVerbatim != 1 {
						t.Errorf("not_read_back_verbatim = %d, want 1: the read-back does not show the "+
							"prompt's own text", document.Verification.Structure.NotReadBackVerbatim)
					}
					warning := ""
					for _, candidate := range document.Warnings {
						if strings.Contains(candidate, "not_read_back_verbatim") {
							warning = candidate
						}
					}
					if warning == "" {
						t.Errorf("no warning names the projection; warnings = %q", document.Warnings)
					}
					if !testCase.installed {
						return
					}
					// The count is only honest if the file holds the prompt: this is the sanity check on
					// the report's own claim, that a read of the installed session finds the text. The
					// precise rule -- the message's own dialogue record, not the text anywhere in the file
					// -- is what the core check asks, and its tests pin it; here the prompt is in the file
					// in the writer's own spelling of it, which is the literal a read of the file gives
					// back.
					installed, err := os.ReadFile(document.TargetSession.Path)
					if err != nil {
						t.Fatalf("reading the installed session: %v", err)
					}
					if !strings.Contains(string(installed), writerJSON(t, prompt.text)) {
						t.Errorf("the installed session does not hold the prompt verbatim:\n%s", installed)
					}
				})
			}
		})
	}
}

// TestContinueHumanOutputNamesTheResumeCommand pins the human block: an operator reads the target
// identity, where it was installed and the command that resumes it without opening the JSON.
func TestContinueHumanOutputNamesTheResumeCommand(t *testing.T) {
	continueEnvironment(t) // the fixture tree as the source and an isolated data directory
	targetHome := shellUnsafeHome(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID)...)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"claude:" + sessionID + " → codex",
		"converted:    codex-rollout-jsonl",
		"installed:    " + targetHome,
		// The printed command names the harness the caller named (here, the test's own absent one),
		// so a caller that runs a binary from an unusual path gets that path back -- and the data
		// directory the session was installed into, which Codex otherwise takes from the caller's own
		// environment rather than from the command line. That directory is quoted because of the space
		// above; the harness name and the session id hold nothing a shell would act on, so they stand
		// as they are.
		"resume:       CODEX_HOME='" + targetHome + "' " + missingTargetBinary + " resume ",
		"non-interactive resume: CODEX_HOME='" + targetHome + "' " + missingTargetBinary + " exec resume ",
		"structure:    verified",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output is missing %q:\n%s", want, stdout)
		}
	}
	// The installation line names the session's own file below the home and the mechanism the
	// installation reported (`hard-link` is the installer's only publication mechanism: a filesystem
	// that cannot take one refuses the install rather than publishing by rename, Q-0002-12).
	installedLine := ""
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "  installed:    ") {
			installedLine = line
		}
	}
	if !strings.HasPrefix(installedLine, "  installed:    "+
		filepath.Join(targetHome, "sessions")+string(filepath.Separator)) ||
		!strings.HasSuffix(installedLine, " [hard-link]") {
		t.Errorf("the installed line does not name the session's own path under the home and its mechanism: %q",
			installedLine)
	}
	if strings.Contains(stdout, "{") {
		t.Errorf("the human output carries JSON:\n%s", stdout)
	}
}

// TestContinueHumanOutputReportsTheSourceCheck pins the human half of the goal's source clause: the
// block shows what became of the transcript the conversation came from — its bytes compared, its bytes
// moved, or the comparison that did not run — rather than leaving any of the three to the JSON.
func TestContinueHumanOutputReportsTheSourceCheck(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID)...)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "source file:  unchanged (sha256 ") {
		t.Errorf("the human output does not show the source check:\n%s", stdout)
	}

	for _, testCase := range []struct {
		name     string
		check    string
		detail   string
		wantLine string
	}{
		{
			name:     "a source that moved while it was converted",
			check:    ccc.SourceCheckChanged,
			detail:   "the source session changed while it was converted (sha256 aa before, bb after)",
			wantLine: "source file:  changed while it was converted (the source session changed",
		},
		{
			name:     "a comparison that did not run",
			check:    ccc.SourceCheckNotVerified,
			detail:   "the run stopped before this check ran: re-reading the source session: no such file",
			wantLine: "source file:  not verified (the run stopped before this check ran",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var block bytes.Buffer
			writeContinuation(&block, ccc.ContinueResult{
				Ref:        ccc.Ref{Provider: "claude", ID: sessionID},
				Target:     ccc.TargetCodex,
				Conversion: ccc.ConversionReport{State: ccc.ConversionStateConverted, Format: ccc.CodexTargetFormat},
				TargetSession: ccc.TargetReport{
					Home:          targetHome,
					SessionID:     "01a1e000-0000-7000-8000-000000000001",
					ResumeCommand: "codex resume 01a1e000-0000-7000-8000-000000000001",
				},
				Installation: ccc.InstallationReport{
					State:            ccc.InstallationStateInstalled,
					Path:             filepath.Join(targetHome, "session.jsonl"),
					SourceCheckState: testCase.check,
					Detail:           testCase.detail,
				},
			})
			if !strings.Contains(block.String(), testCase.wantLine) {
				t.Errorf("the human output does not name %s:\n%s", testCase.name, block.String())
			}
		})
	}
}

// TestContinueHumanOutputNamesAPartialSourceRead pins the fidelity half of the ordinary human block: the
// committed fixture's transcript ends in a line that is not JSON (its own recorded shape), the inherited
// reader skips that line and counts it, and a run that reports conversion, installation and a verified
// structure must still tell a reader that the conversation it holds is not the whole of what the source
// file records — in the block's own fidelity line and in the loss category beside it.
func TestContinueHumanOutputNamesAPartialSourceRead(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID)...)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"conversation: 8 message(s)",
		// The exact line, newline included: this fixture writes its malformed line as a complete
		// record, so the unfinished-tail clause (", and the last record was still being written at
		// read time") must not be appended here — a suffix would put text before the newline and fail
		// this assertion.
		"fidelity:     1 malformed line(s)\n",
		"omitted:      malformed_source_lines ×1\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human output is missing %q:\n%s", want, stdout)
		}
	}

	// A read whose last record was still being written says so in the same line, in the read's own words:
	// the block names the file's state, not this CLI, which never writes a source file.
	var truncated bytes.Buffer
	writeContinuation(&truncated, ccc.ContinueResult{
		Ref:        ccc.Ref{Provider: "claude", ID: sessionID},
		Target:     ccc.TargetCodex,
		Source:     &ccc.SessionReport{Fidelity: ccc.Fidelity{MalformedLines: 1, Truncated: true}},
		Conversion: ccc.ConversionReport{State: ccc.ConversionStateConverted, Format: ccc.CodexTargetFormat},
	})
	if got := truncated.String(); !strings.Contains(got,
		"fidelity:     1 malformed line(s), and the last record was still being written at read time\n") {
		t.Errorf("an unfinished final record is not named by the read's own words:\n%s", got)
	}
}

// TestContinueDryRunInstallsNothingThroughTheCLI pins the flag's promise at the CLI boundary.
func TestContinueDryRunInstallsNothingThroughTheCLI(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, `"state": "not-installed"`) || !strings.Contains(stdout, `"dry_run": true`) {
		t.Errorf("the dry run does not say what it did:\n%s", stdout)
	}
	entries, err := os.ReadDir(targetHome)
	if err != nil {
		t.Fatalf("reading the target home: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a dry run wrote into the target home: %v", entries)
	}

	// The same dry run without `--json`: the block says what the run did — the dry-run line — and marks
	// each identity line it generated as proposed, because a dry run published none of it. A reader who
	// greps the block for an installation finds no `installed:` line at all.
	code, stdout, stderr = runCLI(t, continueArgs(sessionID, "--dry-run")...)
	if code != exitOK {
		t.Fatalf("exit %d for a dry run's human block (stderr %q)", code, stderr)
	}
	if !strings.Contains(stdout, "installation: dry run, nothing written to "+targetHome) {
		t.Errorf("the dry run's human block does not say it wrote nothing:\n%s", stdout)
	}
	if got := strings.Count(stdout, "(proposed; nothing was installed)"); got != 3 {
		t.Errorf("identity lines marked as proposed = %d, want 3 (target id, resume, "+
			"non-interactive resume):\n%s", got, stdout)
	}
	if strings.Contains(stdout, "\n  installed:") {
		t.Errorf("the dry run's human block carries an `installed:` line:\n%s", stdout)
	}
	if entries, err := os.ReadDir(targetHome); err != nil || len(entries) != 0 {
		t.Errorf("the dry run's human block wrote into the target home: %v (err %v)", entries, err)
	}

	// The same dry run with a harness that is present: the level was asked for and cannot run (nothing
	// is installed), which is reported as not verified with the dry-run reason — not as a failure. The
	// harness itself is never started: it leaves a mark when it is (a harness fixes up its own state in
	// the home it runs with), and the home — which does not exist yet here — is left absent.
	marker := filepath.Join(t.TempDir(), "harness-started")
	t.Setenv(stubMarkerEnv, marker)
	freshHome := filepath.Join(t.TempDir(), "fresh-home")
	code, stdout, stderr = runCLI(t,
		"continue", "claude:"+sessionID, "--to", "codex", "--target-binary", stubTargetBinary(t),
		"--target-home", freshHome, "--dry-run", "--json")
	if code != exitOK {
		t.Fatalf("exit %d for a dry run whose requested level could not run (stderr %q)", code, stderr)
	}
	if !strings.Contains(stdout, `"state": "not-verified"`) ||
		!strings.Contains(stdout, "a dry run installs nothing") {
		t.Errorf("the dry run does not report the requested level as not verified with its reason:\n%s", stdout)
	}
	if entries, err := os.ReadDir(targetHome); err != nil || len(entries) != 0 {
		t.Errorf("the dry run with a harness wrote into the target home: %v (err %v)", entries, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Error("a dry run started the target harness, which writes into the data directory it runs with")
	}
	if _, err := os.Stat(freshHome); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a dry run created the target home %s it was told it would not touch", freshHome)
	}

	// The same stub in a run that installs is started: without this half, the mark's absence above could
	// be a stub that never writes one. The load verification then fails (the stub refuses every request),
	// which is what an installed run with a refusing harness reports.
	marker = filepath.Join(t.TempDir(), "harness-started-installed")
	t.Setenv(stubMarkerEnv, marker)
	code, _, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", stubTargetBinary(t), "--json")
	if code != exitFailure {
		t.Fatalf("exit %d for an installed run whose verification the stub refuses (stderr %q)", code, stderr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("an installed run did not start the target harness: %v", err)
	}
}

// TestContinueUsageErrors pins what a caller gets wrong: no target, no reference, a level that does not
// exist, and a resume verification with no prompt of its own.
func TestContinueUsageErrors(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	for _, testCase := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"no target", []string{"continue", "claude:" + sessionID}, exitUsage, "--to"},
		{"unknown target", []string{"continue", "claude:" + sessionID, "--to", "gemini"}, exitUsage, "gemini"},
		{"unknown level", []string{"continue", "claude:" + sessionID, "--to", "codex",
			"--target-binary", missingTargetBinary, "--verify", "perhaps"}, exitUsage, "perhaps"},
		{"resume without a prompt", []string{"continue", "claude:" + sessionID, "--to", "codex",
			"--target-binary", missingTargetBinary, "--verify", "resume"}, exitUsage, "--resume-prompt"},
		{"no reference", []string{"continue", "--to", "codex"}, exitUsage, "usage: ccc continue"},
		{"unknown provider", []string{"continue", "nope:" + sessionID, "--to", "codex"}, exitUsage, "nope"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, testCase.args...)
			if code != testCase.code {
				t.Fatalf("exit %d, want %d (stderr %q)", code, testCase.code, stderr)
			}
			if !strings.Contains(stderr, testCase.want) {
				t.Errorf("stderr %q does not name %q", stderr, testCase.want)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Errorf("a refused command wrote to stdout: %q", stdout)
			}
		})
	}
}

// TestContinueRefusesAMissingDataDirectoryNobodyNamed pins the one path a continuation will not invent:
// $CODEX_HOME that is not there is an error naming the flag that would say where to write, and the
// directory is not created.
func TestContinueRefusesAMissingDataDirectoryNobodyNamed(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	missing := filepath.Join(t.TempDir(), "absent")
	t.Setenv("CODEX_HOME", missing)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", missingTargetBinary)
	if code != exitFailure {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if !strings.Contains(stderr, "--target-home") {
		t.Errorf("stderr %q does not name the flag that would say where to write", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("a refused command wrote to stdout: %q", stdout)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("the refused run created %s", missing)
	}
}

// TestContinueReportsAFailedVerificationAsAFailure pins the exit code rule: a verification the caller
// asked for, that ran and did not pass, fails the run — while a verification that could not run for
// lack of a harness keeps the conversion's success.
func TestContinueReportsAFailedVerificationAsAFailure(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	code, _, stderr := runCLI(t, continueArgs(sessionID, "--verify", "none")...)
	if code != exitOK {
		t.Fatalf("exit %d with --verify none, stderr: %s", code, stderr)
	}
	if code, _, _ := runCLI(t, continueArgs(sessionID)...); code != exitOK {
		t.Fatalf("exit %d: a verification that could not run because the harness is absent must not fail the conversion", code)
	}

	// The stub harness answers the handshake and refuses the load, so the verification runs and fails:
	// that is the case the exit-1 rule names.
	code, stdout, stderr := runCLI(t,
		"continue", "claude:"+sessionID, "--to", "codex", "--target-binary", stubTargetBinary(t), "--json")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d for a verification that ran and failed (stderr %q)", code, exitFailure, stderr)
	}
	var document struct {
		TargetSession struct {
			BinaryFound bool `json:"binary_found"`
		} `json:"target_session"`
		Verification struct {
			Load struct {
				State  string `json:"state"`
				Detail string `json:"detail"`
			} `json:"load"`
		} `json:"verification"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if !document.TargetSession.BinaryFound {
		t.Errorf("the report does not claim the harness that ran: %+v", document.TargetSession)
	}
	if document.Verification.Load.State != ccc.VerificationStateNotVerified {
		t.Errorf("load = %+v, want an explicit not-verified from the harness's own refusal",
			document.Verification.Load)
	}
	if !strings.Contains(document.Verification.Load.Detail, "the stub harness refuses thread/resume") {
		t.Errorf("the refusal does not carry the harness's own message: %q", document.Verification.Load.Detail)
	}
}

// TestContinueWritesTheReportOfAFailureAfterPublication pins the CLI's rule for a run that fails after
// the session was published: the report is what carries the target identity, so it is written before the
// failure is named, and the run still exits failure. The conversion is replaced here because the failure
// the rule is about happens inside it (a source that cannot be re-read, a temporary file that cannot be
// removed); what this test pins is the CLI's answer to a report that comes back beside an error, and that
// a failure with no report still writes nothing to stdout.
func TestContinueWritesTheReportOfAFailureAfterPublication(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	installed := filepath.Join(targetHome, "sessions", "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-01a1e000-0000-7000-8000-000000000001.jsonl")
	const session = "01a1e000-0000-7000-8000-000000000001"
	failure := errors.New("removing the temporary file " + filepath.Join(filepath.Dir(installed), ".rollout.tmp-1") +
		": remove: the file is in use")
	warning := "the run stopped after the conversion: " + failure.Error()
	report := ccc.ContinueResult{
		Ref:        ccc.Ref{Provider: "claude", ID: sessionID},
		Target:     ccc.TargetCodex,
		Conversion: ccc.ConversionReport{State: ccc.ConversionStateConverted, Format: ccc.CodexTargetFormat},
		TargetSession: ccc.TargetReport{
			Provider:          ccc.TargetCodex,
			Home:              targetHome,
			SessionID:         session,
			FileName:          filepath.Base(installed),
			Path:              installed,
			ResumeCommand:     "codex resume " + session,
			ExecResumeCommand: "codex exec resume " + session + " <prompt>",
		},
		Installation: ccc.InstallationReport{
			State: ccc.InstallationStateInstalled, Path: installed, Mechanism: "hard-link", Created: true,
			SourceCheckState: ccc.SourceCheckNotVerified,
			Detail:           "the run stopped before this check ran: " + failure.Error(),
		},
		Verification: ccc.VerificationReport{
			Structure: ccc.StructureVerification{State: ccc.VerificationStateNotVerified, Detail: "the run stopped"},
			Load:      ccc.LoadVerification{State: ccc.VerificationStateNotVerified, Detail: "the run stopped"},
			Resume: ccc.ResumeVerification{
				State: ccc.VerificationStateNotRequested, Detail: "not requested (--verify load)"},
		},
		Warnings: []string{warning},
	}

	run := continueRun
	continueRun = func(context.Context, ccc.ContinueOptions) (ccc.ContinueResult, error) {
		return report, failure
	}
	defer func() { continueRun = run }()

	for _, asJSON := range []bool{false, true} {
		name, args := "report", continueArgs(sessionID)
		if asJSON {
			name, args = "report --json", append(args, "--json")
		}
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, args...)
			if code != exitFailure {
				t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Fatal("a failure that carried a report wrote nothing to stdout, so the target identity is lost")
			}
			if !strings.Contains(stderr, failure.Error()) {
				t.Errorf("stderr %q does not name the failure", stderr)
			}
			if !strings.Contains(stderr, warning) {
				t.Errorf("stderr %q does not carry the report's warning", stderr)
			}
			if !asJSON {
				if !strings.Contains(stdout, "resume:       codex resume "+session) ||
					!strings.Contains(stdout, "installed:    "+installed) {
					t.Errorf("the human block does not name the installed identity:\n%s", stdout)
				}
				return
			}
			var document struct {
				TargetSession struct {
					SessionID string `json:"session_id"`
					Path      string `json:"path"`
				} `json:"target_session"`
				Installation struct {
					State            string `json:"state"`
					SourceCheckState string `json:"source_check_state"`
				} `json:"installation"`
			}
			if err := json.Unmarshal([]byte(stdout), &document); err != nil {
				t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
			}
			if document.TargetSession.SessionID != session || document.TargetSession.Path != installed {
				t.Errorf("the document does not carry the installed identity: %+v", document.TargetSession)
			}
			if document.Installation.State != ccc.InstallationStateInstalled ||
				document.Installation.SourceCheckState != ccc.SourceCheckNotVerified {
				t.Errorf("the document does not carry the installation: %+v", document.Installation)
			}
		})
	}

	// The other half of the rule: a failure that produced no conversion has no report to write, and the
	// error is the whole answer.
	continueRun = func(context.Context, ccc.ContinueOptions) (ccc.ContinueResult, error) {
		return ccc.ContinueResult{}, errors.New("no readable source session")
	}
	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--json")...)
	if code != exitFailure {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("a failure with no report wrote to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "no readable source session") {
		t.Errorf("stderr %q does not name the failure", stderr)
	}
}

// TestContinueHumanOutputReportsAPrePublicationFailure pins the human block of a run that converted and
// then stopped before publishing anything (this one's target home cannot be created): the installation
// line is the report's own state, `not-installed`, rather than an `installed:` line naming a session no
// harness can open, and the identity the conversion generated — target id, resume, non-interactive
// resume — is marked proposed, because the run published none of it. The same run's document is
// asserted beside it, so the block and the JSON answer "is it installed?" the same way.
func TestContinueHumanOutputReportsAPrePublicationFailure(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	// A file where the home's parent chain should be: nothing below it can be created.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("writing the blocking file: %v", err)
	}
	blockedHome := filepath.Join(blocker, "home")
	const proposed = " (proposed; nothing was installed)"

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--target-home", blockedHome)...)
	if code != exitFailure {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	for _, want := range []string{
		"converted:    codex-rollout-jsonl",
		"installed:    " + ccc.InstallationStateNotInstalled + " (nothing was published under " +
			blockedHome + ")",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human block is missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "installed:    "+blockedHome) {
		t.Errorf("the human block reports an installation the run never made:\n%s", stdout)
	}
	if got := strings.Count(stdout, proposed); got != 3 {
		t.Errorf("identity lines marked as proposed = %d, want 3 (target id, resume, "+
			"non-interactive resume):\n%s", got, stdout)
	}
	if !strings.Contains(stderr, "creating "+blockedHome) {
		t.Errorf("stderr %q does not name what stopped the run", stderr)
	}

	// The document of the same failure: the state the human line renders, and no path.
	code, stdout, stderr = runCLI(t, continueArgs(sessionID, "--target-home", blockedHome, "--json")...)
	if code != exitFailure {
		t.Fatalf("exit %d for the JSON spelling, want %d (stderr %q)", code, exitFailure, stderr)
	}
	var document struct {
		TargetSession struct {
			SessionID string `json:"session_id"`
			Path      string `json:"path"`
		} `json:"target_session"`
		Installation struct {
			State string `json:"state"`
			Path  string `json:"path"`
		} `json:"installation"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Installation.State != ccc.InstallationStateNotInstalled || document.Installation.Path != "" {
		t.Errorf("installation = %+v, want not-installed with no path", document.Installation)
	}
	if document.TargetSession.SessionID == "" || document.TargetSession.Path != "" {
		t.Errorf("target session = %+v, want the proposed identity with no installed path",
			document.TargetSession)
	}
}

// TestContinueHelpIsARequestNotAFailure pins the help path for the new command, and that the help page
// explains the common invocation on its own.
func TestContinueHelpIsARequestNotAFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	code, stdout, stderr := runCLI(t, "continue", "-h")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	if !strings.Contains(stdout, "usage: ccc continue") {
		t.Errorf("stdout does not carry the usage line: %q", stdout)
	}
	code, stdout, _ = runCLI(t, "help")
	if code != exitOK {
		t.Fatalf("help exit %d", code)
	}
	for _, want := range []string{
		"ccc continue <provider>:<session-id-or-path> --to codex",
		"ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex",
		"codex resume <session-id>",
		"--verify LEVEL",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

// continueDocumentOf reads one `ccc continue --json` run's document, requiring the run's exit status.
func continueDocumentOf(t *testing.T, code int, stdout, stderr string) map[string]any {
	t.Helper()
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	return document
}

// verificationBlock reads one verification level's block out of a continue document.
func verificationBlock(t *testing.T, document map[string]any, level string) map[string]any {
	t.Helper()
	verification, ok := document["verification"].(map[string]any)
	if !ok {
		t.Fatalf("the document carries no verification block: %+v", document)
	}
	block, ok := verification[level].(map[string]any)
	if !ok {
		t.Fatalf("the document carries no verification.%s block: %+v", level, verification)
	}
	return block
}

// TestCodexLoadVerificationRequiresTheSessionsOwnRecord pins the two directions of the load rule: a
// harness loader that answers with the installed session's own identity and path is a verified load, and
// one that answers with another session -- a loader that echoes an identity it did not open -- is not.
//
// The stub harness serves the app server itself, so both directions run offline on every platform, with
// no target harness installed: the rule this pins is the check's (the answer must be the conversion's
// own session, by id and by path), not the harness's presence. The real harness is exercised beside it
// by scripts/codex-native-check.py, which is not a gate step.
func TestCodexLoadVerificationRequiresTheSessionsOwnRecord(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	binary := stubTargetBinary(t)

	t.Setenv(stubLoadModeEnv, stubLoadSelfMode)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--verify", "load", "--json")
	document := continueDocumentOf(t, code, stdout, stderr)
	load := verificationBlock(t, document, "load")
	if load["state"] != ccc.VerificationStateVerified {
		t.Fatalf("load = %+v, want verified for a loader that answers with the installed session", load)
	}
	target, _ := document["target_session"].(map[string]any)
	if load["path"] != target["path"] {
		t.Errorf("the verified load names %v, want the installed file %v", load["path"], target["path"])
	}

	// The same run with a loader that answers with another session: the check must not call that a load.
	_, otherHome := continueEnvironment(t)
	t.Setenv(stubLoadModeEnv, stubLoadOtherMode)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", otherHome, "--verify", "load", "--json")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d for a loader that loaded another session (stderr %q)", code, exitFailure, stderr)
	}
	var other map[string]any
	if err := json.Unmarshal([]byte(stdout), &other); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	failed := verificationBlock(t, other, "load")
	if failed["state"] != ccc.VerificationStateNotVerified {
		t.Errorf("load = %+v, want not-verified for another session's identity", failed)
	}
	if detail, _ := failed["detail"].(string); !strings.Contains(detail, "not the conversion's own session") {
		t.Errorf("the refusal does not say the loaded thread was not this session: %q", detail)
	}

	// A loader that answers with the session's own id but names another file: the id alone is an
	// identity the harness chose to echo, so the file it opened is what the check must confirm.
	_, thirdHome := continueEnvironment(t)
	t.Setenv(stubLoadModeEnv, stubLoadOtherPathMode)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", thirdHome, "--verify", "load", "--json")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d for a loader that named another file (stderr %q)", code, exitFailure, stderr)
	}
	var third map[string]any
	if err := json.Unmarshal([]byte(stdout), &third); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	unnamed := verificationBlock(t, third, "load")
	if unnamed["state"] != ccc.VerificationStateNotVerified {
		t.Errorf("load = %+v, want not-verified when the loader named another file", unnamed)
	}
	if detail, _ := unnamed["detail"].(string); !strings.Contains(detail, "not the installed file") {
		t.Errorf("the refusal does not say the loader named another file: %q", detail)
	}
}

// TestResumeVerificationRequiresTheTurnToAppendToTheSession pins the resume rule in both directions: a
// turn that appends to the installed session is a verified resume -- with the command it ran and the
// status it exited with reported -- and one that answers without touching the file is not, because a
// harness that answered a question without continuing the session has not shown that the session is
// resumable. The last half is a turn the harness could not run: the report still carries the turn's own
// nonzero status, and the verdict is not-verified.
func TestResumeVerificationRequiresTheTurnToAppendToTheSession(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	binary := stubTargetBinary(t)

	// The loader answers with the installed session, so the requested resume level is the only thing
	// this run can still fail on: a resume verification loads the session first, and a load that never
	// answered would leave the turn unrun instead of pinning the turn rule below.
	t.Setenv(stubLoadModeEnv, stubLoadSelfMode)
	t.Setenv(stubTurnModeEnv, stubTurnGrowMode)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--verify", "resume", "--resume-prompt", "answer only from this conversation",
		"--json")
	document := continueDocumentOf(t, code, stdout, stderr)
	resume := verificationBlock(t, document, "resume")
	if resume["state"] != ccc.VerificationStateVerified {
		t.Fatalf("resume = %+v, want verified for a turn that appended to the session", resume)
	}
	before, after := numberField(t, resume, "session_bytes_before"), numberField(t, resume, "session_bytes_after")
	if after <= before {
		t.Errorf("the verified resume reports %v -> %v bytes, want growth", before, after)
	}
	// The command and the exit code are the turn's own: the invocation the check ran (its arguments and
	// the data directory it selected) and the status the harness exited with, both read from the run's
	// document rather than supplied by this case.
	target, _ := document["target_session"].(map[string]any)
	if exit, ok := resume["exit_code"].(float64); !ok || exit != 0 {
		t.Errorf("resume exit_code = %v, want the turn's own status 0", resume["exit_code"])
	}
	command, _ := resume["command"].(string)
	for _, want := range []string{
		"exec resume " + target["session_id"].(string),
		"--skip-git-repo-check",
		"CODEX_HOME=" + target["home"].(string),
	} {
		if !strings.Contains(command, want) {
			t.Errorf("the reported resume command %q does not name %q", command, want)
		}
	}

	_, otherHome := continueEnvironment(t)
	t.Setenv(stubTurnModeEnv, stubTurnSilentMode)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", otherHome, "--verify", "resume",
		"--resume-prompt", "answer only from this conversation", "--json")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d for a turn that appended nothing (stderr %q)", code, exitFailure, stderr)
	}
	var other map[string]any
	if err := json.Unmarshal([]byte(stdout), &other); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	silent := verificationBlock(t, other, "resume")
	if silent["state"] != ccc.VerificationStateNotVerified {
		t.Errorf("resume = %+v, want not-verified when the session did not grow", silent)
	}
	if detail, _ := silent["detail"].(string); !strings.Contains(detail, "did not grow") {
		t.Errorf("the refusal does not name the missing growth: %q", detail)
	}

	// A turn the harness could not run at all: its own exit status is what the report carries -- the
	// case asserts the stub's own code, so a report holding a constant instead of the turn's status
	// fails here, and the verdict stays not-verified with the harness's status named.
	_, thirdHome := continueEnvironment(t)
	t.Setenv(stubTurnModeEnv, stubTurnFailMode)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", thirdHome, "--verify", "resume",
		"--resume-prompt", "answer only from this conversation", "--json")
	if code != exitFailure {
		t.Fatalf("exit %d, want %d for a turn that could not run (stderr %q)", code, exitFailure, stderr)
	}
	var third map[string]any
	if err := json.Unmarshal([]byte(stdout), &third); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	failedTurn := verificationBlock(t, third, "resume")
	if failedTurn["state"] != ccc.VerificationStateNotVerified {
		t.Errorf("resume = %+v, want not-verified for a turn that exited nonzero", failedTurn)
	}
	if exit, ok := failedTurn["exit_code"].(float64); !ok || int(exit) != stubTurnFailCode {
		t.Errorf("resume exit_code = %v, want the turn's own status %d", failedTurn["exit_code"],
			stubTurnFailCode)
	}
	if detail, _ := failedTurn["detail"].(string); !strings.Contains(detail, fmt.Sprintf("exited %d", stubTurnFailCode)) {
		t.Errorf("the refusal does not name the harness's own status: %q", detail)
	}
}

// numberField reads one JSON number out of a report block.
func numberField(t *testing.T, block map[string]any, key string) float64 {
	t.Helper()
	value, ok := block[key].(float64)
	if !ok {
		t.Fatalf("the block carries no numeric %q: %+v", key, block)
	}
	return value
}

// TestProviderBindingIsTheTargetsOwnAnswer pins where the recorded provider comes from: the harness's
// own answer about a new session of its own (thread/start's thread defaults, or its configuration when
// that request is not answered) -- never the source's provider, which would be false -- and the written
// session records that answer, because the harness's loader checks it when the session is resumed. A
// dry run asks nothing and records none with that reason: the harness is never started against its data
// directory.
func TestProviderBindingIsTheTargetsOwnAnswer(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := claudeFixture(t)
	binary := stubTargetBinary(t)

	t.Setenv(stubProviderEnv, "a-stub-provider")
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--verify", "none", "--json")
	document := continueDocumentOf(t, code, stdout, stderr)
	target, _ := document["target_session"].(map[string]any)
	if target["model_provider"] != "a-stub-provider" {
		t.Fatalf("model_provider = %v, want the harness's own answer", target["model_provider"])
	}
	if target["model_provider_source"] != "target-thread-defaults" {
		t.Errorf("model_provider_source = %v, want the thread defaults' own name", target["model_provider_source"])
	}
	meta := sessionMetaOf(t, target["path"].(string))
	if meta["model_provider"] != "a-stub-provider" {
		t.Errorf("the installed session records %v as its provider, want the harness's own answer",
			meta["model_provider"])
	}

	// The fallback: a harness that does not answer thread/start but names a provider in its own
	// configuration answers that way, and the report says so instead of presenting it as a default.
	_, otherHome := continueEnvironment(t)
	t.Setenv(stubProviderEnv, "")
	t.Setenv(stubConfigProviderEnv, "a-configured-provider")
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", otherHome, "--verify", "none", "--json")
	document = continueDocumentOf(t, code, stdout, stderr)
	target, _ = document["target_session"].(map[string]any)
	if target["model_provider"] != "a-configured-provider" {
		t.Errorf("model_provider = %v, want the harness's configured provider", target["model_provider"])
	}
	if target["model_provider_source"] != "target-config" {
		t.Errorf("model_provider_source = %v, want the configuration's own name", target["model_provider_source"])
	}

	// The dry run's own answer: the harness is never started against the data directory, so no provider
	// is recorded, the report says which answer that is, and the reason names the dry run -- a caller
	// must not read the empty field as an answer the harness could not give.
	_, dryHome := continueEnvironment(t)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "codex",
		"--target-binary", binary, "--target-home", dryHome, "--dry-run", "--verify", "none", "--json")
	document = continueDocumentOf(t, code, stdout, stderr)
	target, _ = document["target_session"].(map[string]any)
	if provider, ok := target["model_provider"]; ok && provider != nil && provider != "" {
		t.Errorf("a dry run recorded model_provider = %v, want none: it never starts the harness", provider)
	}
	if target["model_provider_source"] != "unrecorded" {
		t.Errorf("model_provider_source = %v, want unrecorded for a dry run", target["model_provider_source"])
	}
	if detail, _ := target["model_provider_detail"].(string); !strings.Contains(detail, "dry run") {
		t.Errorf("the dry run's provider detail %q does not say the dry run is why nothing is recorded", detail)
	}
	warnings, _ := document["warnings"].([]any)
	joined := fmt.Sprint(warnings...)
	if !strings.Contains(joined, "records no model provider") {
		t.Errorf("warnings = %v, want one naming the provider a dry run does not record", warnings)
	}
}

// sessionMetaOf reads the session_meta record out of an installed rollout's own bytes.
func sessionMetaOf(t *testing.T, path string) map[string]any {
	t.Helper()
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(installed)), "\n") {
		var record struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the installed session holds a line that is not a record: %v", err)
		}
		if record.Type == "session_meta" {
			return record.Payload
		}
	}
	t.Fatalf("the installed session holds no session_meta record")
	return nil
}
