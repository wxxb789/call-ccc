package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

// claudeFixture returns the committed Claude fixture's session id (its file name) and path. The id is
// read from the tree rather than written into this file: the fixture's identity is its file name, and
// a test that repeats it would have to be edited whenever the fixture is renewed.
func claudeFixture(t *testing.T) (sessionID, path string) {
	t.Helper()
	var found []string
	err := filepath.WalkDir(filepath.Join(fixtureRoot(t), "claude"),
		func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
				found = append(found, path)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("finding the Claude fixture: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one Claude fixture, found %d", len(found))
	}
	return strings.TrimSuffix(filepath.Base(found[0]), ".jsonl"), found[0]
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
