package ccc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The committed fixtures, as references: the provider is the provider's registry name, the id is the
// identifier the provider itself reports.
var (
	claudeFixtureRef = Ref{Provider: "claude", ID: "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"}
	// claudeToolingFixtureRef is the second Claude transcript: the tool-heavy shapes a conversion has to
	// report (a reasoning trace, a subagent turn, an errored tool result).
	claudeToolingFixtureRef = Ref{Provider: "claude", ID: "b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef"}
	codexFixtureRef         = Ref{Provider: "codex", ID: "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"}
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

// TestToolingFixtureCarriesTheShapesTheWriterMustNotMisread pins the two native records a conversion has
// to read correctly, through the real parse of the committed bytes rather than a hand-built document.
//
// The first is a reasoning-only turn: the provider records its thinking block and no dialogue, so the
// message's Content is the rendered trace and its dialogue projection is empty. A conversion that fell
// back to Content would write the reasoning as the agent's own words while the report said the trace was
// omitted. The second is a tool result whose recorded text is itself a JSON string -- a tool that printed
// a JSON-encoded value -- which a second decode would change.
func TestToolingFixtureCarriesTheShapesTheWriterMustNotMisread(t *testing.T) {
	cfg := fixtureConfig(t)
	conversation, err := NewSourceReader(cfg).Read(context.Background(), claudeToolingFixtureRef)
	if err != nil {
		t.Fatalf("reading the tooling fixture: %v", err)
	}
	const reasoning = "Naming it from the constant rather than from memory."
	sawReasoningOnly := false
	for _, message := range conversation.Messages {
		if message.Thinking == reasoning {
			sawReasoningOnly = true
			if !message.DialogueProjected || message.Dialogue != "" {
				t.Errorf("the reasoning-only record's projection = %q (projected %v), want the provider's own "+
					"empty dialogue", message.Dialogue, message.DialogueProjected)
			}
			if !strings.Contains(message.Content, reasoning) {
				t.Errorf("the reasoning-only record's content %q does not render the trace, so the fixture "+
					"does not model the shape this rule is about", message.Content)
			}
		}
	}
	if !sawReasoningOnly {
		t.Fatalf("the tooling fixture holds no reasoning-only record, so nothing pins what a writer must "+
			"do with one (the fixture carries %d message(s))", len(conversation.Messages))
	}

	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(rollout.Data), reasoning) {
		t.Error("the conversion wrote the reasoning trace as the agent's dialogue")
	}
	if loss := lossCount(t, rollout.Losses, "reasoning_text"); loss.Count != 2 {
		t.Errorf("reasoning_text = %+v, want both persisted traces counted", loss)
	}
	outputs := map[string]string{}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionOutput {
			outputs[payloadField(t, line, "call_id")] = payloadField(t, line, "output")
		}
	}
	if got := outputs["toolu_01Tooling00000000000000003"]; got != `"maxBackoffSeconds"` {
		t.Errorf("the JSON-string result was written as %q, want the text the tool printed, quotation "+
			"marks included", got)
	}
}

// TestSummarizeCountsEachRoleSeparately pins the role classification every counts report rests on:
// user, assistant, system and tool are four roles, and a message recorded with one of them is counted
// under that one alone.
//
// The committed fixtures exercise the user/assistant pair (and pin their counts), but neither records a
// system-role or a tool-role message, so the two counts that reach zero in both fixtures were the ones
// a classifier could fold into another number while every fixture test stayed green -- which is what
// happened: a standalone tool output (parser.RoleTool, the role the inherited Devin reader preserves a
// native `"tool"` record as) was counted as a system message, so a tool-only session reported a system
// message no record ever wrote. The list here is built directly and each message carries exactly one
// classified property, so every field of Counts is pinned by its own line and no other rule can produce
// these numbers: three assistant messages against one user-role summary, one system record against one
// system message (they are not the same rule), one sidechain, one boundary, and *two* tool calls against
// one tool result -- the one pair the fixtures cannot separate, since both of theirs hold exactly one of
// each, so a summariser that counted the wrong slice would agree with every fixture count there is. The
// last message carries a role this core does not classify: it is counted in Messages and in no role
// field, so a `default` clause that folded an unrecognised role into an existing count -- the way the
// tool role used to land in the system count -- has to fail here rather than pass as a detail.
func TestSummarizeCountsEachRoleSeparately(t *testing.T) {
	messages := []Message{
		{Ordinal: 0, Role: string(parser.RoleUser), Content: "a question"},
		{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "an answer",
			ToolCalls: []ToolCall{
				{ToolUseID: "toolu_01Count000000000000000000", Name: "Read"},
				{ToolUseID: "toolu_01Count000000000000000001", Name: "Grep"},
			}},
		{Ordinal: 2, Role: string(parser.RoleTool), Content: "the tool's own output",
			ToolResults: []ToolResult{{ToolUseID: "toolu_01Count000000000000000000",
				ContentRaw: "the tool's own output"}}},
		{Ordinal: 3, Role: string(parser.RoleSystem), Content: "a system notice"},
		{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "an injected notice", IsSystem: true},
		{Ordinal: 5, Role: string(parser.RoleAssistant), Content: "a sidechain turn", IsSidechain: true},
		{Ordinal: 6, Role: string(parser.RoleUser), Content: "a compaction summary",
			IsCompactBoundary: true},
		{Ordinal: 7, Role: "unrecognised", Content: "a role this core does not classify"},
	}
	got := Summarize(messages)
	want := Counts{
		Messages: 8, UserMessages: 2, AssistantMessages: 3, SystemMessages: 1, ToolMessages: 1,
		SystemRecords: 1, SidechainMessages: 1, CompactBoundaries: 1, ToolCalls: 2, ToolResults: 1,
	}
	if got != want {
		t.Errorf("Summarize(messages) = %+v, want %+v", got, want)
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

// TestRelativePathReferenceIgnoresARootThatDoesNotHoldIt pins the other half of the same rule: a root
// that does *not* hold the session is not a candidate for the reference.
//
// A provider's changed-path classifier answers for a path that is absent as well as for one that is
// there -- jsonlMissingPathFallbackAllowed exists so a watcher does not drop a deletion -- so asking a
// second root about a spelling it does not hold used to add a source for a file nothing has: the
// reference became ambiguous between the session the first root holds and a path no root can read, and
// a read of that second candidate failed while fingerprinting instead of reaching the session. A
// classification with no file behind it is not a source this reader can read, so it is dropped and the
// root that does hold the session answers.
func TestRelativePathReferenceIgnoresARootThatDoesNotHoldIt(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeRoot := filepath.Join(fixtureRoot(t), "claude")
	relativeName := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	relative := Ref{Provider: "claude", ID: relativeName}
	cfg.AgentDirs[parser.AgentClaude] = []string{claudeRoot, t.TempDir()}

	conversation, err := NewSourceReader(cfg).Read(context.Background(), relative)
	if err != nil {
		t.Fatalf("a root that does not hold the session made %s unreadable: %v", relative.ID, err)
	}
	if conversation.Source.SessionID != claudeFixtureRef.ID {
		t.Errorf("the relative reference read session %q, want %q",
			conversation.Source.SessionID, claudeFixtureRef.ID)
	}
	want := filepath.Join(claudeRoot, relativeName)
	if conversation.Source.SourcePath != want {
		t.Errorf("the relative reference reports source %q, want the root that holds it (%s)",
			conversation.Source.SourcePath, want)
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

// changedPathProvider is a parser.Provider stand-in for the reader's own rules: it answers the two
// questions resolvePathReference asks -- the stored-path lookup, and what a changed path classifies as
// -- and nothing else, so a case can hand the reader a classification no provider's fixtures can
// produce. The embedded interface is nil, and a case that reached another method would panic rather
// than be answered quietly.
type changedPathProvider struct {
	parser.Provider
	sources []parser.SourceRef
}

func (p changedPathProvider) FindSource(
	context.Context, parser.FindSourceRequest,
) (parser.SourceRef, bool, error) {
	return parser.SourceRef{}, false, nil
}

func (p changedPathProvider) SourcesForChangedPath(
	context.Context, parser.ChangedPathRequest,
) ([]parser.SourceRef, error) {
	return p.sources, nil
}

// TestChangedPathClassificationWithSeveralSourcesIsAmbiguous pins the reader's rule for one spelling
// whose provider answer names more than one source: the candidates are reported, never resolved to the
// first. It is pinned on the reader itself, because what it protects is the reader's refusal to pick --
// the provider's answer is "these transcripts read this path" while a reference names one session.
func TestChangedPathClassificationWithSeveralSourcesIsAmbiguous(t *testing.T) {
	candidates := make([]parser.SourceRef, 0, 2)
	for _, name := range []string{"one.jsonl", "two.jsonl"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		candidates = append(candidates, parser.SourceRef{
			Provider: parser.AgentClaude, Key: path, DisplayPath: path, FingerprintKey: path})
	}
	// The spelling itself is absent, so only the classification below can answer -- which is the shape
	// a reference takes when the provider maps a path (a sidecar, a store member) onto its sources.
	spelling := filepath.Join(t.TempDir(), "unrelated.jsonl")
	_, found, err := resolvePathReference(context.Background(), changedPathProvider{sources: candidates},
		[]string{spelling}, Ref{Provider: "claude", ID: spelling})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("a spelling two sources read answered found=%v err=%v, want an ambiguity report",
			found, err)
	}
	if len(ambiguous.Candidates) != len(candidates) {
		t.Errorf("ambiguous candidates = %v, want both sources", ambiguous.Candidates)
	}
}

// TestChangedPathClassificationWithAVirtualIdentityIsKept pins the other acceptance route of the same
// rule: a candidate whose identity is not a local file (a provider's store member, which it records as
// `<container>#<session>`) is kept when the path the reference named is there, because that path is
// what the classification was made from. Requiring every candidate's identity to be a file would drop
// a source such a provider can read simply because it is not stored as one file per session.
func TestChangedPathClassificationWithAVirtualIdentityIsKept(t *testing.T) {
	container := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(container, []byte("a store, not a session file\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", container, err)
	}
	virtual := container + "#0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"
	provider := changedPathProvider{sources: []parser.SourceRef{{
		Provider: parser.AgentHermes, Key: virtual, DisplayPath: virtual, FingerprintKey: virtual}}}

	source, found, err := resolvePathReference(context.Background(), provider, []string{container},
		Ref{Provider: "hermes", ID: container})
	if err != nil || !found {
		t.Fatalf("a store member behind a present path answered found=%v err=%v", found, err)
	}
	if source.FingerprintKey != virtual {
		t.Errorf("the reference resolved %q, want the store member %q", source.FingerprintKey, virtual)
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

// fixedSourceProvider answers the reader's three questions from what a case sets: the source a
// reference resolves to, a constant fingerprint, and one parse outcome. It is the stand-in for the one
// shape no committed fixture produces -- a provider whose *lookup* normalises a reference onto a source
// that parses to a different session (Piebald resolves a fork reference such as `42-7` to its base chat
// 42), or one whose parse outcome a case wants to control row by row -- and it also keeps the
// ProviderConfig and ParseRequest it was built and asked with, so the configuration a reader gives
// a provider is observable. The embedded interface is nil: a case that reached another method
// would panic rather than be answered quietly.
type fixedSourceProvider struct {
	parser.Provider
	source  parser.SourceRef
	outcome parser.ParseOutcome
	config  parser.ProviderConfig
	request parser.ParseRequest
}

func (p *fixedSourceProvider) FindSource(
	context.Context, parser.FindSourceRequest,
) (parser.SourceRef, bool, error) {
	return p.source, true, nil
}

func (p *fixedSourceProvider) Fingerprint(
	context.Context, parser.SourceRef,
) (parser.SourceFingerprint, error) {
	return parser.SourceFingerprint{Key: "fixed"}, nil
}

func (p *fixedSourceProvider) Parse(
	_ context.Context, req parser.ParseRequest,
) (parser.ParseOutcome, error) {
	p.request = req
	return p.outcome, nil
}

// fixedSourceFactory hands the reader one provider instance, whatever configuration it asks with.
type fixedSourceFactory struct {
	def      parser.AgentDef
	provider *fixedSourceProvider
}

func (f fixedSourceFactory) Definition() parser.AgentDef { return f.def }

func (f fixedSourceFactory) Capabilities() parser.Capabilities { return parser.Capabilities{} }

func (f fixedSourceFactory) NewProvider(cfg parser.ProviderConfig) parser.Provider {
	f.provider.config = cfg
	return f.provider
}

// readerOverFixedProvider builds a reader whose only provider is the given stand-in, for an agent the
// registry knows (so a reference can name it) and the configuration supplies.
func readerOverFixedProvider(
	t *testing.T, cfg config.Config, agent parser.AgentType,
) (*SourceReader, *fixedSourceProvider) {
	t.Helper()
	def, ok := parser.AgentByType(agent)
	if !ok {
		t.Fatalf("the registry does not know %q", agent)
	}
	provider := &fixedSourceProvider{}
	reader := &SourceReader{
		cfg:       cfg,
		factories: map[parser.AgentType]parser.ProviderFactory{agent: fixedSourceFactory{def: def, provider: provider}},
	}
	return reader, provider
}

// parsedSessionFor builds the minimal parsed session a parse outcome carries for one identity.
func parsedSessionFor(agent parser.AgentType, id, rawID string) parser.ParseResult {
	return parser.ParseResult{Session: parser.ParsedSession{
		Agent:           agent,
		ID:              id,
		SourceSessionID: rawID,
		File:            parser.FileInfo{Path: "/tmp/" + rawID + ".jsonl"},
	}}
}

// TestReadRejectsASingleResultThatIsNotTheReference pins the identity rule for the one shape the
// shortcut broke: a provider resolves a *fork* reference to the base conversation it forks from, so a
// basic chat parses to the single result `piebald:42` while the caller asked for `piebald:42-7`. The
// reader used to take the sole result as the requested session, which reported the base conversation as
// a fork that does not exist. The requested identity is now required to appear in the parse outcome --
// the same check the sync engine makes before it treats Piebald's chat mtime as a fork's
// (internal/sync's providerSessionSourceMtime).
func TestReadRejectsASingleResultThatIsNotTheReference(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentPiebald)
	provider.source = parser.SourceRef{
		Provider:       parser.AgentPiebald,
		ConfiguredRoot: cfg.InstallationID,
		DisplayPath:    "/sessions/app.db#42",
	}
	provider.outcome = parser.ParseOutcome{Results: []parser.ParseResultOutcome{
		{Result: parsedSessionFor(parser.AgentPiebald, "piebald:42", "42")},
	}}

	if _, err := reader.Read(ctx, Ref{Provider: "piebald", ID: "42-7"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a fork that does not exist: err = %v, want ErrNotFound", err)
	}
	if provider.request.Source.DisplayPath != "/sessions/app.db#42" {
		t.Errorf("the reader did not ask the provider to parse the resolved source: %+v",
			provider.request)
	}

	// The fork the chat really produced is the reference's own answer.
	provider.outcome = parser.ParseOutcome{Results: []parser.ParseResultOutcome{
		{Result: parsedSessionFor(parser.AgentPiebald, "piebald:42", "42")},
		{Result: parsedSessionFor(parser.AgentPiebald, "piebald:42-7", "42-7")},
	}}
	conversation, err := reader.Read(ctx, Ref{Provider: "piebald", ID: "42-7"})
	if err != nil {
		t.Fatalf("reading the fork that exists: %v", err)
	}
	if conversation.Source.SessionID != "piebald:42-7" {
		t.Errorf("the read reported session %q, want the fork the reference named",
			conversation.Source.SessionID)
	}
}

// TestReadAcceptsTheOnlyResultOfAPathReference pins the other half of the same rule: a reference that
// names a filesystem path names a *source*, and the provider's own resolution of that path is the
// session's identity -- so the single result is the answer even though the path is not a session id and
// an identity comparison could never match it. The parse states that it read the source in full
// (`ResultSetComplete`), which is what a real single-file provider always states
// (internal/parser's singleFileSourceSet.Parse) and what makes the sole result the source's only
// conversation; the incomplete half of this pair is TestReadRefusesAPartialContainer.
func TestReadAcceptsTheOnlyResultOfAPathReference(t *testing.T) {
	cfg := fixtureConfig(t)
	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
	path := filepath.Join(cfg.AgentDirs[parser.AgentClaude][0], "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl")
	provider.source = parser.SourceRef{
		Provider:       parser.AgentClaude,
		ConfiguredRoot: cfg.AgentDirs[parser.AgentClaude][0],
		DisplayPath:    path,
		FingerprintKey: path,
	}
	provider.outcome = parser.ParseOutcome{
		ResultSetComplete: true,
		Results: []parser.ParseResultOutcome{
			{Result: parsedSessionFor(parser.AgentClaude, "claude:"+claudeFixtureRef.ID, claudeFixtureRef.ID)},
		},
	}
	conversation, err := reader.Read(context.Background(), Ref{Provider: "claude", ID: path})
	if err != nil {
		t.Fatalf("reading by path: %v", err)
	}
	if conversation.Source.SessionID != "claude:"+claudeFixtureRef.ID {
		t.Errorf("the path read reported session %q", conversation.Source.SessionID)
	}
}

// forkedClaudeTranscript writes one Claude Code transcript whose DAG forks -- a main branch with more
// than three user turns after the fork point and a second branch leaving an early assistant entry, the
// shape internal/parser's TestParseClaudeSession_MetadataOnForkSessions parses into a main session and a
// fork -- under a throw-away provider root, and returns that root's configuration with the file's path.
func forkedClaudeTranscript(t *testing.T) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	// A real Claude transcript is named after the session it records.
	path := filepath.Join(root, "-workspace", "sess-orig.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	entry := func(role, uuid, parent string, content any, second int) map[string]any {
		line := map[string]any{
			"type": role, "timestamp": fmt.Sprintf("2024-01-01T10:00:%02dZ", second),
			"sessionId": "sess-orig", "version": "3.5.0", "cwd": "/workspace",
			"gitBranch": "feat/forks", "uuid": uuid,
			"message": map[string]any{"content": content},
		}
		if parent != "" {
			line["parentUuid"] = parent
		}
		return line
	}
	say := func(text string) any { return []map[string]any{{"type": "text", "text": text}} }
	entries := []map[string]any{
		entry("user", "a", "", "start", 0),
		entry("assistant", "b", "a", say("ok"), 1),
		entry("user", "c", "b", "q1", 2),
		entry("assistant", "d", "c", say("a1"), 3),
		entry("user", "e", "d", "q2", 4),
		entry("assistant", "f", "e", say("a2"), 5),
		entry("user", "g", "f", "q3", 6),
		entry("assistant", "h", "g", say("a3"), 7),
		entry("user", "k", "h", "q4", 8),
		entry("assistant", "l", "k", say("a4"), 9),
		// The fork leaves the early assistant entry, so one file holds two sessions.
		entry("user", "fork-u1", "b", "forked question", 10),
		entry("assistant", "fork-a1", "fork-u1", say("forked answer"), 11),
	}
	lines := make([]string, 0, len(entries))
	for _, line := range entries {
		encoded, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("encoding a transcript line: %v", err)
		}
		lines = append(lines, string(encoded))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {root}}
	return cfg, path
}

// TestReadReportsAForkedPathAsAmbiguous pins how a path reference answers a parse that produced more
// than one session.
//
// A Claude Code transcript whose DAG forks parses to the main branch and the fork -- two sessions from
// one file, the shape the inherited parser's own fork test pins -- and a path names a source rather
// than one of the sessions a parse of it produces. The read used to answer that path as *absent*: the
// CLI said "no session found" for a transcript that is there, and the two sessions it holds were
// thrown away. The design's rule for an ambiguous identifier -- return the candidates instead of
// guessing -- applies, and the parse's own session identities are what a caller can pass back.
func TestReadReportsAForkedPathAsAmbiguous(t *testing.T) {
	cfg, path := forkedClaudeTranscript(t)
	reader := NewSourceReader(cfg)
	conversation, err := reader.Read(context.Background(), Ref{Provider: "claude", ID: path})
	if err == nil {
		t.Fatalf("a forked transcript read as one session: %+v", conversation.Source)
	}
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("reading a forked transcript by path answered %v, want an ambiguity report", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("the ambiguity is also reported as absence: %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous candidates = %v, want the two sessions the file holds", ambiguous.Candidates)
	}
	// The identities are the parser's own for this fixture (internal/parser names the main session
	// after the file and the branch after the entry it left), which is what the archive stores and what
	// conversationFromParse reports: a caller passes one back instead of guessing.
	got := map[string]bool{}
	for _, candidate := range ambiguous.Candidates {
		got[candidate] = true
	}
	for _, want := range []string{"sess-orig", "sess-orig-fork-u1"} {
		if !got[want] {
			t.Errorf("candidates %v are missing the parsed session %q", ambiguous.Candidates, want)
		}
	}
	if len(got) != 2 {
		t.Errorf("both candidates are the same session: %v", ambiguous.Candidates)
	}

	// A fork identity names the session but no file of its own: the inherited source lookup resolves a
	// session id to `<id>.jsonl` under a configured root, so an id-only reference to the fork answers
	// "not found" at the source (an archive can still hold it) instead of quietly returning the main
	// session the file is named after.
	if _, err := reader.Read(context.Background(), Ref{Provider: "claude", ID: "sess-orig-fork-u1"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading the fork identity as a session answered %v, want ErrNotFound", err)
	}
}

// TestProvidersNamesStoredAgentLabels pins the names this engine resolves, which is what a caller is
// told to choose from and what the CLI's reference parsing asks for (ccc.KnownProvider).
//
// A provider's name is not the only name its sessions carry: the Codebuff provider writes free-tier
// sessions under the Freebuff label, so `freebuff:<project>:<timestamp>` is a reference a caller can
// write and has to see in the list. The list is not a free-for-all either: a name no provider writes is
// absent, so "known provider" cannot pass by accepting anything.
func TestProvidersNamesStoredAgentLabels(t *testing.T) {
	providers := NewSourceReader(fixtureConfig(t)).Providers()
	for _, want := range []string{"codebuff", "claude", "codex", "freebuff"} {
		if !slices.Contains(providers, want) {
			t.Errorf("the provider list is missing %q: %v", want, providers)
		}
	}
	if slices.Contains(providers, "no-such-agent") || slices.Contains(providers, "") {
		t.Errorf("the provider list names something no provider writes: %v", providers)
	}
	for provider, want := range map[string]bool{
		"claude": true, "codebuff": true, "freebuff": true,
		"no-such-agent": false, "": false, "unknown": false,
	} {
		if got := KnownProvider(provider); got != want {
			t.Errorf("KnownProvider(%q) = %v, want %v", provider, got, want)
		}
	}
}

// TestReadRefusesAPathWhoseParseIsIncomplete pins the path rule for a source the provider could not read
// in full: a sole parse result is the source's session *only when the parse read the source in full*.
//
// The shape is the inherited partial-container outcome, reproduced field for field from
// internal/parser's TestOpenClawSQLiteStoreMalformedMemberIsolation: one readable member, two members
// the provider recorded as source errors, and ResultSetComplete false. A path names the *source*, so
// that one result is not the source's only conversation -- the reference used to resolve to the readable
// member, and the same reference turns into an ambiguity error as soon as a malformed sibling parses,
// which means the parse's own failure decided which conversation the caller meant. The refusal carries
// the provider's diagnostics and the identity it did read, so a caller has an explicit identity to pass
// back, and a reference that names a session id is unaffected: it asks for one identity, and another
// member's failure says nothing about that identity.
func TestReadRefusesAPathWhoseParseIsIncomplete(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentOpenClaw)
	const path = "/sessions/main/agent/openclaw-agent.sqlite"
	provider.source = parser.SourceRef{
		Provider:       parser.AgentOpenClaw,
		DisplayPath:    path,
		FingerprintKey: path,
	}
	partial := parser.ParseOutcome{
		Results: []parser.ParseResultOutcome{
			{Result: parsedSessionFor(parser.AgentOpenClaw, "openclaw:main:good", "main:good")},
		},
		SourceErrors: []parser.SourceError{
			{SessionID: "openclaw:main:bad", Err: errors.New("openclaw SQLite: malformed member: wrong header")},
			{SessionID: "openclaw:main:bad-json", Err: errors.New("openclaw SQLite: malformed member: invalid JSON")},
		},
	}
	provider.outcome = partial

	_, err := reader.Read(ctx, Ref{Provider: "openclaw", ID: path})
	if err == nil {
		t.Fatal("a path whose parse was incomplete was read as the member the parse happened to return")
	}
	if !errors.Is(err, ErrPartialSource) {
		t.Errorf("err = %v, want %v", err, ErrPartialSource)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("an incomplete parse is reported as absence: %v", err)
	}
	for _, want := range []string{
		"openclaw:main:good", "openclaw:main:bad", "openclaw:main:bad-json",
		"name one of those session ids instead of the path",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not carry %q: %v", want, err)
		}
	}

	// An outcome with *no* result is not this classification: nothing was read, so the answer stays the
	// classification that shape already has (a provider failure, because it carries source errors), and
	// the incompleteness of an empty result set must not turn "the provider could not read this source"
	// into "one of the members it read is not the one you meant".
	provider.outcome = parser.ParseOutcome{SourceErrors: partial.SourceErrors}
	if _, err := reader.Read(ctx, Ref{Provider: "openclaw", ID: path}); !errors.Is(err, ErrProviderFailed) {
		t.Errorf("a path whose parse produced no result: err = %v, want %v", err, ErrProviderFailed)
	}
	provider.outcome = partial

	// The identity the incomplete parse did read is the answer to a reference that names it: the
	// refusal is about what a *path* cannot establish, not about the members the parse returned.
	conversation, err := reader.Read(ctx, Ref{Provider: "openclaw", ID: "openclaw:main:good"})
	if err != nil {
		t.Fatalf("reading the identity the incomplete parse returned: %v", err)
	}
	if conversation.Source.SessionID != "openclaw:main:good" {
		t.Errorf("the by-id read reported session %q", conversation.Source.SessionID)
	}
	if len(conversation.Messages) != 0 {
		t.Errorf("the by-id read returned %d message(s), want the member's own", len(conversation.Messages))
	}
}

// TestReadRefusesAPartialTraeContainer pins the same rule on a real container through the inherited
// provider, not on a stand-in.
//
// Trae stores its sessions as a list inside one VS Code state database, and its container parse answers
// with the members that list holds while reporting the result set incomplete when an entry it cannot
// read sits beside them (internal/parser's decodeTraeSessionSnapshot marks the snapshot incomplete and
// keeps the records it could read). A reference to that container path names the *source*, so the one
// parseable member must not become its answer -- the caller gets the provider's verdict plus the
// identity to name instead, and that identity is readable by id. The control is the same container shape
// with only the readable entry: the path reference still answers its one session, so the refusal is
// about the unreadable entry rather than about containers.
func TestReadRefusesAPartialTraeContainer(t *testing.T) {
	ctx := context.Background()
	const sessionID = "5c1d2f30-9b47-4a2c-8f0e-1d2b3c4a5e60"
	valid := traeSessionEntry(sessionID)

	root, dbPath := traeContainer(t, valid+","+traeMalformedEntry)
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentTrae: {root}}

	_, err := NewSourceReader(cfg).Read(ctx, Ref{Provider: "trae", ID: dbPath})
	if err == nil {
		t.Fatal("a container whose stored list holds an unreadable entry was read as the member that parses")
	}
	if !errors.Is(err, ErrPartialSource) {
		t.Errorf("err = %v, want %v", err, ErrPartialSource)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("a partial container parse is reported as absence: %v", err)
	}
	if !strings.Contains(err.Error(), "trae:"+sessionID) {
		t.Errorf("the refusal does not name the member it read: %v", err)
	}

	// The identity the refusal names is what a caller passes back, and the provider reads it: a member
	// reference goes through the container's member parse.
	conversation, err := NewSourceReader(cfg).Read(ctx, Ref{Provider: "trae", ID: "trae:" + sessionID})
	if err != nil {
		t.Fatalf("reading the member the partial container parse named: %v", err)
	}
	if conversation.Source.SessionID != "trae:"+sessionID {
		t.Errorf("the by-id read reported session %q", conversation.Source.SessionID)
	}
	if len(conversation.Messages) != 2 {
		t.Errorf("the by-id read returned %d message(s), want the member's two", len(conversation.Messages))
	}

	// The control: only the readable entry is in the list, and the path reference answers that one
	// session.
	cleanRoot, cleanPath := traeContainer(t, valid)
	cleanCfg := fixtureConfig(t)
	cleanCfg.AgentDirs = map[parser.AgentType][]string{parser.AgentTrae: {cleanRoot}}
	byPath, err := NewSourceReader(cleanCfg).Read(ctx, Ref{Provider: "trae", ID: cleanPath})
	if err != nil {
		t.Fatalf("reading a container whose list holds one readable entry: %v", err)
	}
	if byPath.Source.SessionID != "trae:"+sessionID {
		t.Errorf("the complete container read reported session %q", byPath.Source.SessionID)
	}
}

// TestReadReportsAnOpenClawContainersMembersInsteadOfPickingOne pins what a path reference to the other
// kind of partial container actually answers, measured on the inherited provider the partial-container
// case was written for.
//
// OpenClaw's own stored-path lookup resolves a *member* path and refuses the container path, so a
// reference to the container goes to the changed-path classifier, which enumerates the container's
// session ids -- the malformed ones included -- as member sources. Several of those are several
// candidates, so the reference is reported as ambiguous rather than resolved to the one member that
// parses; the caller is handed the member identities (the `#<agent>:<session>` spellings the archive
// stores) and one of them names a single session. That is why the incomplete-sole-result branch is
// pinned on Trae and on the inherited outcome as a stand-in: this provider never hands this reader a
// container source with results to choose from.
func TestReadReportsAnOpenClawContainersMembersInsteadOfPickingOne(t *testing.T) {
	ctx := context.Background()
	root, dbPath := openClawContainer(t, map[string][]string{
		"good": openClawContainerEvents("good"),
		"bad": {
			`{"type":"session","id":"wrong-header","timestamp":"2026-09-22T10:00:00Z"}`,
			`{"type":"message","message":{"role":"user","content":"bad"}}`,
		},
		"bad-json": {`{"type":"session"`},
	})
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentOpenClaw: {root}}

	_, err := NewSourceReader(cfg).Read(ctx, Ref{Provider: "openclaw", ID: dbPath})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("a container path resolved without naming its members: err = %v", err)
	}
	if len(ambiguous.Candidates) != 3 {
		t.Fatalf("candidates = %v, want the container's three members", ambiguous.Candidates)
	}
	for _, want := range []string{"#main:good", "#main:bad", "#main:bad-json"} {
		found := false
		for _, candidate := range ambiguous.Candidates {
			found = found || strings.HasSuffix(candidate, want)
		}
		if !found {
			t.Errorf("candidates %v are missing %q", ambiguous.Candidates, want)
		}
	}

	// The member identity is the answer to a reference that names it, and a member that cannot be read
	// reports the provider's own diagnostics instead of a session.
	conversation, err := NewSourceReader(cfg).Read(ctx, Ref{Provider: "openclaw", ID: "openclaw:main:good"})
	if err != nil {
		t.Fatalf("reading the container member by id: %v", err)
	}
	if conversation.Source.SessionID != "openclaw:main:good" {
		t.Errorf("the by-id read reported session %q", conversation.Source.SessionID)
	}
	if _, err := NewSourceReader(cfg).Read(ctx, Ref{Provider: "openclaw", ID: "openclaw:main:bad-json"}); err == nil {
		t.Error("a malformed member was read as a session")
	} else if !strings.Contains(err.Error(), "malformed member") {
		t.Errorf("the malformed member's failure lost the provider's diagnostics: %v", err)
	}
}

// openClawContainer writes one OpenClaw SQLite container -- the inherited multi-session source whose
// container parse returns the members it could read and records the others as source errors -- under a
// throw-away provider root with the layout the provider discovers (`<root>/<agentID>/agent/<db>`, the
// shape internal/parser's own container cases write), and returns the root and the container path.
func openClawContainer(t *testing.T, sessions map[string][]string) (root, dbPath string) {
	t.Helper()
	root = t.TempDir()
	dbPath = filepath.Join(root, "main", "agent", "openclaw-agent.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("preparing the OpenClaw container directory: %v", err)
	}
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("creating %s: %v", dbPath, err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE transcript_events (
			session_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			event_json TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (session_id, seq)
		) STRICT
	`); err != nil {
		t.Fatalf("creating the container's schema: %v", err)
	}
	for sessionID, events := range sessions {
		for seq, event := range events {
			if _, err := database.Exec(
				`INSERT INTO transcript_events(session_id, seq, event_json, created_at) VALUES (?, ?, ?, ?)`,
				sessionID, seq, event, 1_700_000_000_000+seq,
			); err != nil {
				t.Fatalf("writing member %s of the container: %v", sessionID, err)
			}
		}
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the container: %v", err)
	}
	return root, dbPath
}

// openClawContainerEvents is one readable container member: the session header and one user turn, in
// the shape internal/parser's OpenClaw SQLite cases use.
func openClawContainerEvents(sessionID string) []string {
	return []string{
		`{"type":"session","version":3,"id":"` + sessionID + `","timestamp":"2026-09-22T10:00:00Z","cwd":"/workspace/example"}`,
		`{"type":"message","id":"m1","timestamp":"2026-09-22T10:00:01Z","message":{"role":"user","content":"hello from the container","timestamp":"2026-09-22T10:00:01Z"}}`,
	}
}

// traeContainer writes one Trae state database -- a VS Code `state.vscdb` holding the provider's
// `memento/icube-ai-agent-storage` value -- under a throw-away provider root with the layout the
// provider discovers (`<root>/workspaceStorage/<workspace>/state.vscdb`), and returns the root and the
// container path. `list` becomes the stored session list verbatim, which is where an entry the provider
// cannot read lives: its loader marks the snapshot incomplete and keeps the records it could read.
func traeContainer(t *testing.T, list string) (root, dbPath string) {
	t.Helper()
	root = t.TempDir()
	dbPath = filepath.Join(root, "workspaceStorage", "example-workspace", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("preparing the Trae container directory: %v", err)
	}
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("creating %s: %v", dbPath, err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(
		`CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	); err != nil {
		t.Fatalf("creating the container's schema: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO ItemTable(key, value) VALUES (?, ?)`,
		"memento/icube-ai-agent-storage", `{"list":[`+list+`]}`,
	); err != nil {
		t.Fatalf("writing the container's session list: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the container: %v", err)
	}
	return root, dbPath
}

// freebuffSession writes one Codebuff-layout session whose run-state classifies it as Freebuff -- the
// shape internal/parser's own TestParseCodebuffSession_FreebuffClassification pins: the same on-disk
// layout, with an agent type containing "free" -- under a throw-away provider root, and returns that
// root, the transcript's path, and the project and timestamp that make up the session's native id. The
// parser records it as `freebuff:<project>:<timestamp>` under the Codebuff provider, which is the
// identity pair the alias cases exercise.
func freebuffSession(t *testing.T) (root, chatPath, project, timestamp string) {
	t.Helper()
	project, timestamp = "example-project", "2026-07-15T20-01-32.065Z"
	root = t.TempDir()
	directory := filepath.Join(root, project, "chats", timestamp)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("preparing the Freebuff session directory: %v", err)
	}
	transcript := `[
		{"id": "user-1", "variant": "user", "content": "Where is the retry cap configured?",
		 "timestamp": "2026-07-15T20:01:33Z"},
		{"id": "ai-1", "variant": "ai", "content": "", "timestamp": "2026-07-15T20:01:34Z",
		 "blocks": [{"type": "text", "textType": "text",
		             "content": "It is set in internal/retry/config.go."}]}
	]`
	runState := `{"sessionState": {"mainAgentState": {"agentType": "base2-free-minimax-m3"},` +
		` "fileContext": {"cwd": "/workspace/example"}}}`
	chatPath = filepath.Join(directory, "chat-messages.json")
	for path, body := range map[string]string{
		chatPath: transcript,
		filepath.Join(directory, "run-state.json"): runState,
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	return root, chatPath, project, timestamp
}

// traeSessionEntry is one readable Trae session list entry, and traeMalformedEntry is one its loader
// cannot read (no session id, so no record and no identity): the pair a partial container holds. The
// shape is the provider's own stored list (`internal/parser`'s decodeTraeSessionSnapshot reads
// `sessionId`, `messages` and an optional `agentTaskContent`).
func traeSessionEntry(sessionID string) string {
	return `{"sessionId":"` + sessionID + `","createdAt":"2026-09-22T10:00:00Z",` +
		`"updatedAt":"2026-09-22T10:05:00Z","model":"trae-model","messages":[` +
		`{"role":"user","content":"Where is the retry cap configured?",` +
		`"timestamp":"2026-09-22T10:00:01Z","turnIndex":0},` +
		`{"role":"assistant","content":"It is set in internal/retry/config.go.",` +
		`"timestamp":"2026-09-22T10:00:02Z","turnIndex":1}]}`
}

// traeMalformedEntry is the entry that makes a Trae container's snapshot incomplete: a record the
// loader skips (no `sessionId`), which is the shape internal/parser's snapshot rule marks malformed.
const traeMalformedEntry = `{"createdAt":"2026-09-22T11:00:00Z"}`

// TestReadReportsAProviderFailureInsteadOfAbsence pins the classification of the one outcome shape the
// inherited Cursor provider uses for an operational failure: the parse returns a nil Go error and
// carries the failure in ParseOutcome.SourceErrors with no result. That is not the answer "no such
// session" -- the session may well exist and the provider could not read it -- and a caller that
// reported it as absence would send the user looking for a session that is there. The provider's own
// diagnostics travel with the error, because the outcome is the only place they exist.
func TestReadReportsAProviderFailureInsteadOfAbsence(t *testing.T) {
	cfg := fixtureConfig(t)
	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentCursor)
	const sessionID = "3f9c8f3a-7b12-4e4a-8a19-2f0b6c5d7e01"
	provider.source = parser.SourceRef{
		Provider:       parser.AgentCursor,
		DisplayPath:    "/home/user/.cursor/projects/example/session.jsonl",
		FingerprintKey: "/home/user/.cursor/projects/example/session.jsonl",
	}
	provider.outcome = parser.ParseOutcome{
		ResultSetComplete: true,
		SourceErrors: []parser.SourceError{{
			SourceKey:   "cursor-state-store",
			DisplayPath: "/home/user/.cursor/state.db",
			SessionID:   sessionID,
			Err:         errors.New("the store is locked by another reader"),
			Retryable:   true,
		}},
	}

	_, err := reader.Read(context.Background(), Ref{Provider: "cursor", ID: sessionID})
	if err == nil {
		t.Fatal("a provider that produced no result was read as a session")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("an operational provider failure is reported as absence: %v", err)
	}
	if !errors.Is(err, ErrProviderFailed) {
		t.Errorf("err = %v, want %v", err, ErrProviderFailed)
	}
	for _, want := range []string{"the store is locked by another reader", "/home/user/.cursor/state.db"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure does not carry the provider's diagnostics (%q): %v", want, err)
		}
	}
}

// TestPickResultRequiresTheReferenceIdentity pins the selection helper's own answers, the ones a fixture
// cannot produce for every spelling: which results count as the reference (its raw id, the provider's
// prefixed id, the native id the stored id ends in), that a lone result is not the answer unless it is
// the reference, that several results are filtered by identity rather than picked, and that a path
// reference is answered by the source's own single result *only when the parse read the source in full*
// -- while several results for a path, and one result for a path whose parse reported its result set
// incomplete, name no single session for the helper to return. Read reports each of those as its own
// answer instead (TestReadReportsAForkedPathAsAmbiguous, TestReadRefusesAPartialContainer).
func TestPickResultRequiresTheReferenceIdentity(t *testing.T) {
	one := func(id, raw string) parser.ParseOutcome {
		return parser.ParseOutcome{
			ResultSetComplete: true,
			Results: []parser.ParseResultOutcome{
				{Result: parsedSessionFor(parser.AgentPiebald, id, raw)},
			},
		}
	}
	base := parsedSessionFor(parser.AgentPiebald, "piebald:42", "42")
	fork := parsedSessionFor(parser.AgentPiebald, "piebald:42-7", "42-7")
	both := parser.ParseOutcome{
		ResultSetComplete: true,
		Results: []parser.ParseResultOutcome{
			{Result: base}, {Result: fork},
		},
	}
	pathRef := Ref{Provider: "claude", ID: "/tmp/sessions/" + claudeFixtureRef.ID + ".jsonl"}
	cases := []struct {
		name    string
		outcome parser.ParseOutcome
		ref     Ref
		found   bool
	}{
		{"single result, reference in its raw id", one("piebald:42", "42"),
			Ref{Provider: "piebald", ID: "42"}, true},
		{"single result, reference in the prefixed id", one("piebald:42", "42"),
			Ref{Provider: "piebald", ID: "piebald:42"}, true},
		{"single result, reference is another session", one("piebald:42", "42"),
			Ref{Provider: "piebald", ID: "42-7"}, false},
		{"several results, the reference is the second", both,
			Ref{Provider: "piebald", ID: "42-7"}, true},
		{"several results, none is the reference", both,
			Ref{Provider: "piebald", ID: "42-9"}, false},
		{"no results", parser.ParseOutcome{}, Ref{Provider: "piebald", ID: "42"}, false},
		{"path reference, one result, complete result set",
			one("claude:"+claudeFixtureRef.ID, claudeFixtureRef.ID), pathRef, true},
		{"path reference, one result, incomplete result set", parser.ParseOutcome{
			Results: []parser.ParseResultOutcome{
				{Result: parsedSessionFor(parser.AgentClaude, "claude:"+claudeFixtureRef.ID, claudeFixtureRef.ID)},
			}}, pathRef, false},
		{"path reference, several results (no single session to return)", parser.ParseOutcome{
			ResultSetComplete: true,
			Results: []parser.ParseResultOutcome{
				{Result: parsedSessionFor(parser.AgentClaude, "claude:a", "a")},
				{Result: parsedSessionFor(parser.AgentClaude, "claude:b", "b")}}}, pathRef, false},
		{"session reference, one result, incomplete result set (the identity is what a session reference asks for)",
			parser.ParseOutcome{Results: []parser.ParseResultOutcome{
				{Result: parsedSessionFor(parser.AgentClaude, "claude:"+claudeFixtureRef.ID, claudeFixtureRef.ID)},
				{Result: parsedSessionFor(parser.AgentClaude, "claude:other", "other")},
			}}, Ref{Provider: "claude", ID: claudeFixtureRef.ID}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, found := pickResult(testCase.outcome, testCase.ref)
			if found != testCase.found {
				t.Fatalf("found = %v, want %v", found, testCase.found)
			}
			if found && result.Result.Session.ID == "" {
				t.Error("a found result carries no identity")
			}
		})
	}
}

// TestReadUsesTheConfiguredSourceMachine pins the machine a read reports for a source under a root the
// configuration labels as another machine's: it is that label, not this installation's identity, because
// the machine is one of the parse request's inputs (the providers record it on the parsed session) and
// the inherited sync engine resolves it from the same configured mapping.
func TestReadUsesTheConfiguredSourceMachine(t *testing.T) {
	ctx := context.Background()
	claudeRoot := filepath.Join(fixtureRoot(t), "claude")
	for name, machines := range map[string]map[parser.AgentType]map[string]string{
		"labelled":     {parser.AgentClaude: {claudeRoot: "archivebox"}},
		"this machine": {parser.AgentClaude: {claudeRoot: "localbox"}},
		"unlabelled":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := fixtureConfig(t)
			cfg.InstallationID = "localbox"
			cfg.SourceMachines = machines
			conversation, err := NewSourceReader(cfg).Read(ctx, claudeFixtureRef)
			if err != nil {
				t.Fatalf("reading the fixture: %v", err)
			}
			want := "localbox"
			if name == "labelled" {
				want = "archivebox"
			}
			if conversation.Source.Machine != want {
				t.Errorf("a source under a %s root reports machine %q, want %q", name,
					conversation.Source.Machine, want)
			}
		})
	}
}

// antigravityBrainTranscript writes one small Antigravity conversation's plaintext transcript -- a user
// step and a model step -- under a throw-away provider root, and returns that root. The transcript is
// the session source the inherited provider reads whenever the conversation has no database, which is
// what lets a case exercise the provider's foreign-root handling without a store.
func antigravityBrainTranscript(t *testing.T) (string, string) {
	t.Helper()
	const id = "5c1d2f30-9b47-4a2c-8f0e-1d2b3c4a5e60"
	root := t.TempDir()
	path := filepath.Join(root, "brain", id, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	lines := []string{
		`{"created_at":"2026-05-04T09:15:00Z","content":"Where is the retry cap configured?",` +
			`"source":"USER_EXPLICIT","status":"DONE","step_index":0,"type":"USER_INPUT"}`,
		`{"created_at":"2026-05-04T09:15:05Z","content":"It is set in internal/retry/config.go.",` +
			`"source":"MODEL","status":"DONE","step_index":1,"type":"PLANNER_RESPONSE"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	return root, id
}

// TestInspectReportsASummaryTranscriptClassification pins what a provider says about the completeness of
// the transcript it returned, in both reads. Upstream classifies a read explicitly: a Grok session whose
// summary survives but whose chat history does not is a `summary` fallback (its source version says
// `grok-summary-v1` and it synthesizes the opening prompt as its only turn), while a session read from
// its transcript is `full` and a provider that signals nothing leaves the field empty. Both new adapters
// dropped the field, so an inspection of a degraded fallback reported the same counts, tools and
// identity as one of the session's recorded transcript, with nothing saying the difference.
//
// The session is also not a retryable one: upstream's own Antigravity CLI cases classify reads as
// `summary` *and* current, so the classification is a statement of its own and is pinned apart from the
// retry state (TestInspectReportsARetryableResultWithItsMessages is the other side of that pair).
func TestInspectReportsASummaryTranscriptClassification(t *testing.T) {
	ctx := context.Background()
	cfg, summaryPath, ref := grokSummarySession(t)

	inspection, err := Inspect(ctx, InspectOptions{Ref: ref, Config: cfg})
	if err != nil {
		t.Fatalf("inspecting the summary-only session: %v", err)
	}
	if inspection.Source == nil {
		t.Fatalf("the summary-only session was not read: %v", inspection.Warnings)
	}
	source := inspection.Source
	if source.Fidelity.TranscriptFidelity != parser.TranscriptFidelitySummary {
		t.Errorf("transcript fidelity = %q, want %q",
			source.Fidelity.TranscriptFidelity, parser.TranscriptFidelitySummary)
	}
	if source.Fidelity.SourceVersion != "grok-summary-v1" {
		t.Errorf("source version = %q, want the provider's own summary spelling",
			source.Fidelity.SourceVersion)
	}
	if source.Fidelity.NeedsRetry {
		t.Errorf("a current summary read reports a pending retry: %+v", source.Fidelity)
	}
	if source.Counts.Messages == 0 {
		t.Errorf("the summary fallback read to no message(s), so this case pins nothing: %+v", source.Counts)
	}
	if warnings := strings.Join(inspection.Warnings, "\n"); !strings.Contains(warnings, "summary transcript") {
		t.Errorf("the summary classification is not disclosed: %v", inspection.Warnings)
	}

	// The same classification travels through the archive: the writer stored it on the row and an
	// archive read reports what the row says.
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, summaryPath); err != nil {
		t.Fatalf("ingesting the summary-only session: %v", err)
	}
	archived, err := Inspect(ctx, InspectOptions{
		Ref: ref, Config: cfg, Reader: NewSourceReader(cfg), ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the summary-only session against its archive: %v", err)
	}
	if archived.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", archived.Warnings)
	}
	if got := archived.Archive.Status.TranscriptFidelity; got != parser.TranscriptFidelitySummary {
		t.Errorf("the archived row's transcript fidelity = %q, want %q",
			got, parser.TranscriptFidelitySummary)
	}
	if warnings := strings.Join(archived.Warnings, "\n"); !strings.Contains(warnings, "summary fallback") {
		t.Errorf("the stored summary classification is not disclosed: %v", archived.Warnings)
	}

	// The control: a session read from its own transcript is not classified as a summary, and no warning
	// claims one -- so the rule cannot pass by warning about every read.
	fullCfg := fixtureConfig(t)
	full, err := Inspect(ctx, InspectOptions{Ref: claudeFixtureRef, Config: fullCfg})
	if err != nil {
		t.Fatalf("inspecting the transcript fixture: %v", err)
	}
	if full.Source == nil {
		t.Fatalf("the transcript fixture was not read: %v", full.Warnings)
	}
	if got := full.Source.Fidelity.TranscriptFidelity; got == parser.TranscriptFidelitySummary {
		t.Errorf("a transcript read is classified as a summary: %+v", full.Source.Fidelity)
	}
	if warnings := strings.Join(full.Warnings, "\n"); strings.Contains(warnings, "summary") {
		t.Errorf("a transcript read warns about a summary fallback: %v", full.Warnings)
	}
}

// grokSummarySession writes one Grok session whose chat history is missing -- only `summary.json` exists,
// which is the shape upstream's own summary-only cases use -- under a throw-away provider root, and
// returns that root's configuration, the summary file and the reference to inspect it with.
func grokSummarySession(t *testing.T) (config.Config, string, Ref) {
	t.Helper()
	const id = "3f5c8a21-7d94-4b6e-8a30-5c1e2f4b6d78"
	root := t.TempDir()
	path := filepath.Join(root, "grok", "-work-project", id, "summary.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	summary := `{"summary":"Pin the retry cap","firstPrompt":"Where is the retry cap configured?",` +
		`"modelId":"grok-code-fast","createdAt":"2026-09-01T10:00:00Z",` +
		`"updatedAt":"2026-09-01T10:05:00Z","lastActiveAt":"2026-09-01T10:06:00Z",` +
		`"numMessages":6,"worktreeLabel":"example-api","gitRootDir":"/work/project"}`
	if err := os.WriteFile(path, []byte(summary+"\n"), 0o600); err != nil {
		t.Fatalf("writing the summary: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentGrok: {filepath.Join(root, "grok")}}
	return cfg, path, Ref{Provider: "grok", ID: id}
}

// TestReadKeepsAForeignRootForeign pins the other half of the configured mapping: the source a provider
// resolves for a root that belongs to another machine is marked as remote by the provider itself, which
// is only possible when the reader hands the provider the configured root-to-machine labels. Antigravity
// is the provider that draws a conclusion from them -- a foreign root's working directory is metadata
// from another machine, so it is kept as a project hint and no local filesystem discovery runs for it --
// and its inherited tests pin that distinction for a provider that was configured with the mapping. This
// is the reader's own end of it: the same fixture, the same provider, told what the configuration knows,
// reports a remote working directory and the foreign machine's sessions.
func TestReadKeepsAForeignRootForeign(t *testing.T) {
	ctx := context.Background()
	root, id := antigravityBrainTranscript(t)
	for name, machines := range map[string]map[parser.AgentType]map[string]string{
		"foreign root": {parser.AgentAntigravity: {root: "archivebox"}},
		"local root":   {parser.AgentAntigravity: {root: "localbox"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := fixtureConfig(t)
			cfg.InstallationID = "localbox"
			cfg.SourceMachines = machines
			cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentAntigravity: {root}}
			reader := NewSourceReader(cfg)

			source, found, err := reader.Resolve(ctx, Ref{Provider: "antigravity", ID: id})
			if err != nil {
				t.Fatalf("resolving the transcript: %v", err)
			}
			if !found {
				t.Fatalf("the transcript was not resolved")
			}
			wantMachine := "localbox"
			if source.CwdResolution.State == parser.SourceCwdRemote {
				if name != "foreign root" {
					t.Errorf("a source under a %s is marked remote: %+v", name, source.CwdResolution)
				}
				wantMachine = "archivebox"
			} else if name == "foreign root" {
				t.Errorf("a source under a foreign root reports cwd state %v, want %v",
					source.CwdResolution.State, parser.SourceCwdRemote)
			}
			conversation, err := reader.Read(ctx, Ref{Provider: "antigravity", ID: id})
			if err != nil {
				t.Fatalf("reading the transcript: %v", err)
			}
			if conversation.Source.Machine != wantMachine {
				t.Errorf("a source under a %s reports machine %q, want %q", name,
					conversation.Source.Machine, wantMachine)
			}
		})
	}
}
