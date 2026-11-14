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
		"archive:      " + filepath.Join(dataDir, "sessions.db") + " [full]",
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

// TestUsageErrors pins the command line contract: a reference without a provider, an unknown
// provider, a missing operand and an unknown command are usage errors, not silent guesses.
func TestUsageErrors(t *testing.T) {
	isolatedEnvironment(t)
	cases := [][]string{
		{"inspect"},
		{"inspect", "9f"},
		{"inspect", "no-such-agent:whatever"},
		{"inspect", "claude:"},
		{"frobnicate"},
		{},
	}
	for _, args := range cases {
		code, _, stderr := runCLI(t, args...)
		if code != exitUsage {
			t.Errorf("ccc %v: exit %d, want %d (stderr %q)", args, code, exitUsage, stderr)
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

// TestHelpNeedsNoEnvironment pins that a first-time user can read the usage without a data directory.
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
}
