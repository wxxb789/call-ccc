package ccc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The committed fixtures, as references: the provider is the provider's registry name, the id is the
// identifier the provider itself reports.
var (
	claudeFixtureRef = Ref{Provider: "claude", ID: "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"}
	codexFixtureRef  = Ref{Provider: "codex", ID: "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"}
)

// fixtureRoot is the directory the committed sanitized fixtures live in; each provider subdirectory
// is that provider's configured source root, with the layout that provider's discovery expects.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "fixtures"))
	if err != nil {
		t.Fatalf("resolving the fixture root: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture root %s: %v", root, err)
	}
	return root
}

// fixturePath is one committed fixture file inside its provider's root.
func fixturePath(t *testing.T, relative string) string {
	t.Helper()
	return filepath.Join(fixtureRoot(t), relative)
}

// fixtureConfig points the inherited providers at the committed fixtures and the archive at a
// throw-away directory, so nothing a test does can reach a real session or a real archive.
func fixtureConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("building the default configuration: %v", err)
	}
	root := fixtureRoot(t)
	dir := t.TempDir()
	cfg.DataDir = dir
	cfg.DBPath = filepath.Join(dir, "sessions.db")
	cfg.AgentDirs = map[parser.AgentType][]string{
		parser.AgentClaude: {filepath.Join(root, "claude")},
		parser.AgentCodex:  {filepath.Join(root, "codex")},
	}
	cfg.DisabledAgents = nil
	return cfg
}

// hashTree hashes every file below root, keyed by its path relative to root.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		relative, _ := filepath.Rel(root, path)
		hashes[relative] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("hashing %s: %v", root, err)
	}
	return hashes
}

// TestFixturesMatchTheirGenerator pins that the committed fixtures are exactly what the committed
// generator writes, so the bytes in testdata cannot drift from the shape their comments describe.
func TestFixturesMatchTheirGenerator(t *testing.T) {
	committed := hashTree(t, fixtureRoot(t))
	regenerated := filepath.Join(t.TempDir(), "fixtures")
	cmd := exec.Command("go", "run", "./fixturegen", "-root", regenerated)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running fixturegen: %v\n%s", err, output)
	}
	// The generator writes the same relative paths; compare content, not location.
	generated := hashTree(t, regenerated)
	if len(committed) != len(generated) {
		t.Fatalf("committed fixture count %d, generated %d", len(committed), len(generated))
	}
	for path, sum := range committed {
		if generated[path] != sum {
			t.Errorf("fixture %s differs from what fixturegen writes; re-run "+
				"`go run ./internal/ccc/fixturegen`", path)
		}
	}
	if unchanged := hashTree(t, fixtureRoot(t)); len(unchanged) != len(committed) {
		t.Errorf("the fixture tree changed during the test: %d -> %d file(s)",
			len(committed), len(unchanged))
	}
}

// TestFixtureParseReportsIdentityAndConversation pins what an inspection can rely on for each proven
// provider: the identity the parser reports, the conversation shape, the tool call, the compaction
// boundary, and the malformed trailing line that must be counted rather than turned into a message.
func TestFixtureParseReportsIdentityAndConversation(t *testing.T) {
	reader := NewSourceReader(fixtureConfig(t))
	ctx := context.Background()

	claude, err := reader.Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the Claude fixture: %v", err)
	}
	if claude.Source.Provider != string(parser.AgentClaude) {
		t.Errorf("provider = %q, want %q", claude.Source.Provider, parser.AgentClaude)
	}
	if claude.Source.Agent != "Claude Code" {
		t.Errorf("agent label = %q", claude.Source.Agent)
	}
	if claude.Source.SourceVersion != "1.0.30" {
		t.Errorf("source version = %q, want 1.0.30", claude.Source.SourceVersion)
	}
	if claude.Source.Project != "example_api" || claude.Source.Cwd != "/home/user/code/example-api" {
		t.Errorf("project/cwd = %q/%q", claude.Source.Project, claude.Source.Cwd)
	}
	claudeCounts := Summarize(claude.Messages)
	want := Counts{
		Messages: 8, UserMessages: 3, AssistantMessages: 5, SystemRecords: 1,
		CompactBoundaries: 1, ToolCalls: 1, ToolResults: 1,
	}
	if claudeCounts != want {
		t.Errorf("Claude counts = %+v, want %+v", claudeCounts, want)
	}
	if claude.Fidelity.MalformedLines != 1 {
		t.Errorf("malformed lines = %d, want 1 (the truncated final line)",
			claude.Fidelity.MalformedLines)
	}
	if claude.Fidelity.ReadFrom != "source" {
		t.Errorf("read_from = %q, want source", claude.Fidelity.ReadFrom)
	}
	if !strings.Contains(claude.Messages[2].ToolCalls[0].InputJSON, "internal/retry/policy.go") {
		t.Errorf("tool input lost the file path: %q", claude.Messages[2].ToolCalls[0].InputJSON)
	}
	if !claude.Messages[5].IsCompactBoundary || !claude.Messages[5].IsSystem {
		t.Errorf("the compaction summary is not marked as a boundary: %+v", claude.Messages[5])
	}

	codex, err := reader.Read(ctx, codexFixtureRef)
	if err != nil {
		t.Fatalf("reading the Codex fixture: %v", err)
	}
	if codex.Source.Provider != string(parser.AgentCodex) || codex.Source.Agent != "Codex" {
		t.Errorf("codex identity = %+v", codex.Source)
	}
	codexCounts := Summarize(codex.Messages)
	wantCodex := Counts{
		Messages: 6, UserMessages: 2, AssistantMessages: 4, SystemRecords: 1,
		CompactBoundaries: 1, ToolCalls: 1, ToolResults: 1,
	}
	if codexCounts != wantCodex {
		t.Errorf("Codex counts = %+v, want %+v", codexCounts, wantCodex)
	}
	if codex.Fidelity.MalformedLines != 0 {
		t.Errorf("codex malformed lines = %d, want 0", codex.Fidelity.MalformedLines)
	}
	// Codex records a function call's output as an event on the call rather than as a message of its
	// own, so the call's message is the only place its shell output can appear in a source read. The
	// parser attaches the event to the call; a read that dropped it would report a call with no
	// outcome while an archive read of the same session kept the body.
	codexResults := codex.Messages[1].ToolResults
	if len(codexResults) != 1 {
		t.Fatalf("codex: tool results on the call's message = %+v, want the shell output",
			codexResults)
	}
	shellOutput := "internal/retry/config.go:41: maxBackoffSeconds = 30"
	if !strings.Contains(codexResults[0].ContentRaw, shellOutput) {
		t.Errorf("codex: the source read lost the shell output: %+v", codexResults[0])
	}
	if codexResults[0].ToolUseID != codex.Messages[1].ToolCalls[0].ToolUseID ||
		codexResults[0].Source != "function_call_output" ||
		codexResults[0].ContentSize != len(shellOutput) {
		t.Errorf("codex: the result row does not describe the call's output: %+v", codexResults[0])
	}

	report := reportFrom(claude)
	if report.FirstUserMessage != "Summarize the retry policy in internal/retry/policy.go." {
		t.Errorf("first user message = %q", report.FirstUserMessage)
	}
	if len(report.ToolNames) != 1 || report.ToolNames[0] != "Read" {
		t.Errorf("tool names = %v, want [Read]", report.ToolNames)
	}
	if report.StartedAt.IsZero() || report.EndedAt.IsZero() || !report.StartedAt.Before(report.EndedAt) {
		t.Errorf("time range = %s .. %s", report.StartedAt, report.EndedAt)
	}
}

// TestFixturesCarryNoHostPathsOrSecrets pins the sanitization rule for committed fixtures: they are
// invented content, so the tree's own root, a home directory, and credential shapes stay out of them.
func TestFixturesCarryNoHostPathsOrSecrets(t *testing.T) {
	root := fixtureRoot(t)
	home, _ := os.UserHomeDir()
	needles := []string{"sk-ant-", "ghp_", "AKIA", "Bearer ", "BEGIN PRIVATE KEY"}
	if home != "" {
		needles = append(needles, home)
	}
	if repo, err := filepath.Abs(".."); err == nil {
		needles = append(needles, repo)
	}
	checked := 0
	for path := range hashTree(t, root) {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		checked++
		for _, needle := range needles {
			if strings.Contains(string(data), needle) {
				t.Errorf("%s contains %q", path, needle)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no fixtures were checked")
	}
}

// TestEmptyParseOutcomeIsExplained pins the explanation an inspection prints when a read finds
// nothing: it has to come from the provider's own outcome -- its skip reason, its per-source errors,
// the sessions it excluded -- rather than from a guess, because the string is what the not-found
// error a caller reads is built from. The test is deliberately fixture-free: it pins the reader's
// explanation helpers, not a provider's file format.
func TestEmptyParseOutcomeIsExplained(t *testing.T) {
	if got := describeOutcome(parser.ParseOutcome{}); got != "the provider reported no result" {
		t.Errorf("an outcome with nothing to report explains itself as %q", got)
	}
	explained := describeOutcome(parser.ParseOutcome{
		SkipReason:         parser.SkipNoSession,
		ExcludedSessionIDs: []string{"a", "b"},
		SourceErrors: []parser.SourceError{
			{DisplayPath: "one.jsonl", Err: errors.New("unreadable")},
			{SessionID: "two", Err: errors.New("truncated")},
		},
	})
	for _, want := range []string{
		"skipped: no session at this source",
		"source error on one.jsonl: unreadable",
		"source error on two: truncated",
		"2 excluded session(s)",
	} {
		if !strings.Contains(explained, want) {
			t.Errorf("the explanation is missing %q: %s", want, explained)
		}
	}
	for reason, want := range map[parser.SkipReason]string{
		parser.SkipNoSession:         "no session at this source",
		parser.SkipUnsupportedSource: "unsupported source",
		parser.SkipNonInteractive:    "non-interactive session",
		parser.SkipShadowedBySidecar: "shadowed by a sidecar source",
	} {
		if got := skipReasonText(reason); got != want {
			t.Errorf("skipReasonText(%d) = %q, want %q", reason, got, want)
		}
	}
	if got := skipReasonText(parser.SkipReason(200)); got != "reason 200" {
		t.Errorf("an unknown skip reason reads as %q", got)
	}
}
