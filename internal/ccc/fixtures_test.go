package ccc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// TestMissingPersistedToolOutputKeepsItsFidelitySignal pins the one place a Claude read can say that a
// tool result's content is not the tool's output.
//
// The inherited parser replaces a tool_result's placeholder with the bytes of the side file the tool
// wrote when it can read that file, and marks the block
// (parser.ParsedToolResult.PersistedOutputUnresolved) when it cannot. The content then stays the
// placeholder text -- "Full output saved to: <path>" -- which is indistinguishable from genuine output
// unless that flag is read: the adapter used to drop it, so an inspection counted the result as
// evidence and a translation could carry the placeholder as if the tool had produced it. Both
// occurrences are in the fixture, so the signal is pinned per result rather than per session.
func TestMissingPersistedToolOutputKeepsItsFidelitySignal(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "example-api", "session")
	resolvedPath := filepath.Join(sessionDir, "tool-results", "resolved.txt")
	missingPath := filepath.Join(sessionDir, "tool-results", "gone.txt")
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		t.Fatalf("creating the side-file directory: %v", err)
	}
	fullOutput := "the whole log the tool wrote\n"
	if err := os.WriteFile(resolvedPath, []byte(fullOutput), 0o644); err != nil {
		t.Fatalf("writing the resolved side file: %v", err)
	}

	lines := []string{
		`{"type":"user","timestamp":"2026-01-01T00:00:00Z","uuid":"u1","message":{"content":"run it"},"cwd":"/tmp/example-api"}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","uuid":"a1","parentUuid":"u1","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"make logs"}}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:02Z","uuid":"u2","parentUuid":"a1","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":` + jsonString(t, "Full output saved to: "+resolvedPath) + `,"is_error":false}]},"toolUseResult":{"persistedOutputPath":` + jsonString(t, resolvedPath) + `}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:03Z","uuid":"a2","parentUuid":"u2","message":{"content":[{"type":"tool_use","id":"toolu_2","name":"Bash","input":{"command":"make logs"}}]}}`,
		`{"type":"user","timestamp":"2026-01-01T00:00:04Z","uuid":"u3","parentUuid":"a2","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_2","content":` + jsonString(t, "Full output saved to: "+missingPath) + `,"is_error":false}]},"toolUseResult":{"persistedOutputPath":` + jsonString(t, missingPath) + `}}`,
	}
	sessionFile := filepath.Join(root, "example-api", "session.jsonl")
	if err := os.WriteFile(sessionFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("writing the session: %v", err)
	}

	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {root}}
	reader := NewSourceReader(cfg)
	ctx := context.Background()
	ref := Ref{Provider: "claude", ID: sessionFile}
	conversation, err := reader.Read(ctx, ref)
	if err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	if len(conversation.Messages) != 5 {
		t.Fatalf("the session parsed into %d message(s), want 5", len(conversation.Messages))
	}
	resolved := conversation.Messages[2].ToolResults
	unresolved := conversation.Messages[4].ToolResults
	if len(resolved) != 1 || len(unresolved) != 1 {
		t.Fatalf("tool results = %+v / %+v, want one each", resolved, unresolved)
	}
	if resolved[0].PersistedOutputUnresolved {
		t.Errorf("a result whose side file is readable is marked unresolved: %+v", resolved[0])
	}
	if !strings.Contains(resolved[0].ContentRaw, fullOutput[:10]) {
		t.Errorf("the readable side file's bytes are not the result's content: %q", resolved[0].ContentRaw)
	}
	if !unresolved[0].PersistedOutputUnresolved {
		t.Errorf("a result naming a side file that could not be read is not marked: %+v", unresolved[0])
	}
	if !strings.Contains(unresolved[0].ContentRaw, "Full output saved to:") {
		t.Errorf("the unresolved result's content is not the placeholder: %q", unresolved[0].ContentRaw)
	}
	if got := conversation.Fidelity.UnresolvedToolOutputs; got != 1 {
		t.Errorf("fidelity reports %d unresolved tool output(s), want 1", got)
	}

	inspection, err := Inspect(ctx, InspectOptions{Ref: ref, Config: cfg, Reader: reader})
	if err != nil {
		t.Fatalf("inspecting the session: %v", err)
	}
	if inspection.Source == nil {
		t.Fatalf("the inspection read no source: %v", inspection.Warnings)
	}
	if got := inspection.Source.Fidelity.UnresolvedToolOutputs; got != 1 {
		t.Errorf("the inspection reports %d unresolved tool output(s), want 1", got)
	}
	warning := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warning, "persisted output") || !strings.Contains(warning, "could not read") {
		t.Errorf("the inspection does not report the missing persisted output: %v", inspection.Warnings)
	}
}

// jsonString encodes one JSON string value, so a fixture line can carry a path with the separators and
// escapes of the platform it is built on.
func jsonString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %q as JSON: %v", value, err)
	}
	return string(encoded)
}

// TestHostQualifiedReferenceHasNoLocalSource pins the source side of a reference to a session recorded
// on another machine: it is refused with its own answer, so a caller can say why the source is
// unreadable without reading a *local* session of the same native id in its place.
func TestHostQualifiedReferenceHasNoLocalSource(t *testing.T) {
	reader := NewSourceReader(fixtureConfig(t))
	ctx := context.Background()
	qualified := Ref{Provider: "codex", ID: "host-a~codex:" + codexFixtureRef.ID}
	if _, err := reader.Read(ctx, qualified); !errors.Is(err, ErrNoLocalSource) {
		t.Errorf("reading a host-qualified reference answered %v, want %v", err, ErrNoLocalSource)
	}
	if _, found, err := reader.Resolve(ctx, qualified); found || !errors.Is(err, ErrNoLocalSource) {
		t.Errorf("resolving a host-qualified reference answered found=%v err=%v", found, err)
	}
	// The host is what is refused, not the id: the same native id without one still reads the fixture,
	// so a reader that refused host-qualified references wholesale would pass the two checks above and
	// fail this one.
	if conversation, err := reader.Read(ctx, codexFixtureRef); err != nil {
		t.Errorf("reading the local session: %v", err)
	} else if conversation.Source.SessionID == "" {
		t.Error("the local read lost its session id")
	}
}

// TestRelativePathReferenceResolvesThroughTheProviderRoot pins what a relative filesystem reference
// means.
//
// A provider compares a candidate path with its own absolute roots (the Claude provider resolves the
// pair with filepath.Rel), so a relative spelling used to reach it unchanged and an existing session was
// reported as missing; the archive had the same mismatch, because it stores the absolute path the writer
// recorded. A relative reference is therefore resolved against the provider's configured roots -- the
// only reading that does not depend on the working directory of the process doing the inspecting -- and
// the relative and absolute references then name the same session.
func TestRelativePathReferenceResolvesThroughTheProviderRoot(t *testing.T) {
	cfg := fixtureConfig(t)
	reader := NewSourceReader(cfg)
	ctx := context.Background()
	absolute := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	relative := Ref{Provider: "claude", ID: filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")}

	byRelative, err := reader.Read(ctx, relative)
	if err != nil {
		t.Fatalf("reading %s: %v", relative.ID, err)
	}
	byAbsolute, err := reader.Read(ctx, Ref{Provider: "claude", ID: absolute})
	if err != nil {
		t.Fatalf("reading %s: %v", absolute, err)
	}
	if byRelative.Source.SessionID != byAbsolute.Source.SessionID {
		t.Errorf("the relative reference read session %q, the absolute one %q",
			byRelative.Source.SessionID, byAbsolute.Source.SessionID)
	}
	if byRelative.Source.SourcePath != absolute {
		t.Errorf("the relative reference reports source %q, want %q",
			byRelative.Source.SourcePath, absolute)
	}

	// A relative reference no configured root holds is not read from the working directory instead: the
	// answer is an error (the provider's own, for a path its classifier accepts but cannot stat), never
	// a session.
	if conversation, err := reader.Read(ctx, Ref{Provider: "claude",
		ID: filepath.Join("no-such-project", "session.jsonl")}); err == nil {
		t.Errorf("a relative reference no root holds read %+v", conversation.Source)
	}

	// A home-prefixed reference is offered as written -- the shell expands `~`, not this reader -- so it
	// is not joined to a root, which would ask the provider about a session under a directory named `~`
	// and let its changed-path classifier answer with a source for a path that is not there.
	if _, found, err := reader.Resolve(ctx, Ref{Provider: "claude", ID: "~/session.jsonl"}); err != nil || found {
		t.Errorf("a home-prefixed reference resolved to found=%v err=%v", found, err)
	}
}

// TestRelativePathReferenceIsAmbiguousOnlyAcrossDistinctRoots pins the two ends of resolving a relative
// reference against the configured roots: two roots that both hold the session are two candidates for
// one reference (reported, not picked), while the same root listed twice is one root.
func TestRelativePathReferenceIsAmbiguousOnlyAcrossDistinctRoots(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeRoot := filepath.Join(fixtureRoot(t), "claude")
	relativeName := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	relative := Ref{Provider: "claude", ID: relativeName}

	second := t.TempDir()
	data, err := os.ReadFile(filepath.Join(claudeRoot, relativeName))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	copied := filepath.Join(second, relativeName)
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatalf("creating the second root: %v", err)
	}
	if err := os.WriteFile(copied, data, 0o644); err != nil {
		t.Fatalf("copying the fixture into the second root: %v", err)
	}

	ctx := context.Background()
	cfg.AgentDirs[parser.AgentClaude] = []string{claudeRoot, second}
	reader := NewSourceReader(cfg)
	_, err = reader.Read(ctx, relative)
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("a reference two roots hold answered %v, want an ambiguity report", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %v, want both roots' paths", ambiguous.Candidates)
	}

	cfg.AgentDirs[parser.AgentClaude] = []string{claudeRoot, claudeRoot}
	reader = NewSourceReader(cfg)
	if _, err := reader.Read(ctx, relative); err != nil {
		t.Errorf("the same root listed twice made one session ambiguous: %v", err)
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
