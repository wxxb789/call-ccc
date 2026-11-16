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
	default:
		fmt.Fprintln(os.Stderr, "the stub harness does not implement "+args[0])
		return 2
	}
}

// serveStubAppServer answers the two-line handshake a verification opens with and refuses every request
// after it, which is what makes a load check run and come back not verified.
func serveStubAppServer() int {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var message struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := decoder.Decode(&message); err != nil {
			return 0
		}
		if message.ID == nil {
			continue // the initialized notification carries no id
		}
		answer := map[string]any{"jsonrpc": "2.0", "id": *message.ID}
		if message.Method == "initialize" {
			answer["result"] = map[string]any{}
		} else {
			answer["error"] = map[string]any{"code": -32601, "message": "the stub harness refuses " + message.Method}
		}
		if err := encoder.Encode(answer); err != nil {
			return 1
		}
	}
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
	_, targetHome := continueEnvironment(t)
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
			State           string `json:"state"`
			Created         bool   `json:"created"`
			SourceUnchanged bool   `json:"source_unchanged"`
			Mechanism       string `json:"mechanism"`
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
	for _, spelling := range []string{`"state": "converted"`, `"state": "installed"`} {
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
		!document.Installation.SourceUnchanged || document.Installation.Mechanism == "" {
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
	if want := "CODEX_HOME=" + targetHome + " " + missingTargetBinary + " resume " +
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

// TestContinueHumanOutputNamesTheResumeCommand pins the human block: an operator reads the target
// identity, where it was installed and the command that resumes it without opening the JSON.
func TestContinueHumanOutputNamesTheResumeCommand(t *testing.T) {
	_, targetHome := continueEnvironment(t)
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
		// environment rather than from the command line.
		"resume:       CODEX_HOME=" + targetHome + " " + missingTargetBinary + " resume ",
		"non-interactive resume: CODEX_HOME=" + targetHome + " " + missingTargetBinary + " exec resume ",
		"structure:    verified",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("human output is missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "{") {
		t.Errorf("the human output carries JSON:\n%s", stdout)
	}
}

// TestContinueHumanOutputReportsTheSourceCheck pins the human half of the goal's source clause: the
// block shows whether the transcript the conversation came from is its same file afterwards, and a
// source that moved while it was converted is named rather than left to the JSON.
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

	var moved bytes.Buffer
	writeContinuation(&moved, ccc.ContinueResult{
		Ref:        ccc.Ref{Provider: "claude", ID: sessionID},
		Target:     ccc.TargetCodex,
		Conversion: ccc.ConversionReport{State: ccc.ConversionStateConverted, Format: ccc.CodexTargetFormat},
		TargetSession: ccc.TargetReport{
			Home:          targetHome,
			SessionID:     "01a1e000-0000-7000-8000-000000000001",
			ResumeCommand: "codex resume 01a1e000-0000-7000-8000-000000000001",
		},
		Installation: ccc.InstallationReport{
			State:           ccc.InstallationStateInstalled,
			Path:            filepath.Join(targetHome, "session.jsonl"),
			SourceUnchanged: false,
			Detail:          "the source session changed while it was converted (sha256 aa before, bb after)",
		},
	})
	if !strings.Contains(moved.String(), "source file:  changed while it was converted (the source session changed") {
		t.Errorf("a moved source is not named in the human output:\n%s", moved.String())
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
