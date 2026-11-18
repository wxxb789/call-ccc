package ccc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"go.kenn.io/agentsview/internal/parser"
)

// openCodeTargetTestOptions is a conversion configured as the route configures one: a fresh identity,
// the pinned version, and the target home's own answers (a provider, a model, an agent), which is what
// makes every required member of the document answerable without a harness.
func openCodeTargetTestOptions() OpenCodeTargetOptions {
	return OpenCodeTargetOptions{
		CLIVersion:     OpenCodeFormatVersion,
		Provider:       "target-provider",
		Model:          "target-model",
		ModelVariant:   "max",
		Agent:          "build",
		ProviderDetail: "the harness resolved `target-provider/target-model` for its primary agent `build`",
		Now:            time.UnixMilli(1_800_000_000_000).UTC(),
	}
}

// openCodeTestConversation is one hand-built conversation whose pieces are the shapes the writer's rules
// are about: dialogue, a persisted reasoning trace, a call with a recorded result, a failed result, and
// the edge shapes each loss category names.
func openCodeTestConversation(messages ...Message) Conversation {
	conversation := Conversation{
		Source:   SourceIdentity{Provider: "claude", SessionID: "source-session", Cwd: "/tmp/ccc-opencode-test"},
		Messages: messages,
	}
	for index := range conversation.Messages {
		conversation.Messages[index].Ordinal = index
	}
	return conversation
}

func openCodeTestCall(id, name, input string) ToolCall {
	return ToolCall{ToolUseID: id, Name: name, InputJSON: input}
}

func openCodeTestResult(id, status, raw string) ToolResult {
	return ToolResult{ToolUseID: id, Status: status, ContentRaw: raw, ContentSize: len(raw)}
}

// openCodeLossCount answers one loss category's count, and whether the category is reported at all: every
// category the writer knows is reported on every run, so a missing one is a defect.
func openCodeLossCount(session OpenCodeSession, category string) (int, bool) {
	for _, loss := range session.Losses {
		if loss.Category == category {
			return loss.Count, true
		}
	}
	return 0, false
}

func openCodeRequireLoss(t *testing.T, session OpenCodeSession, category string, want int) {
	t.Helper()
	count, ok := openCodeLossCount(session, category)
	if !ok {
		t.Fatalf("the report names no %s category", category)
	}
	if count != want {
		t.Errorf("%s = %d, want %d", category, count, want)
	}
}

// openCodeLossDetail answers one loss category's own sentence, which is where the report says what it did
// with what the read could not answer.
func openCodeLossDetail(t *testing.T, session OpenCodeSession, category string) string {
	t.Helper()
	for _, loss := range session.Losses {
		if loss.Category == category {
			return loss.Detail
		}
	}
	t.Fatalf("the report names no %s category", category)
	return ""
}

// TestConvertToOpenCodeSessionWritesTheHarnesssOwnDocumentShape pins the document against the harness's
// measured import rules: the members it refuses to see missing are all there, the optional members this
// build has nothing true for are absent, and every value the writer could not know is the empty or zero
// value the report discloses rather than an invented one.
func TestConvertToOpenCodeSessionWritesTheHarnesssOwnDocumentShape(t *testing.T) {
	conversation := readFixtureConversation(t, claudeFixtureRef)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	if session.Cwd != conversation.Source.Cwd {
		t.Errorf("cwd = %q, want the source's own recorded directory %q", session.Cwd, conversation.Source.Cwd)
	}
	if !strings.HasPrefix(session.SessionID, openCodeSessionIDPrefix) ||
		len(session.SessionID) != len(openCodeSessionIDPrefix)+openCodeIDTailLength {
		t.Errorf("session id = %q, want the harness's own prefix and %d characters after it",
			session.SessionID, openCodeIDTailLength)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	info := document.Info
	if info.ID != session.SessionID {
		t.Errorf("info.id = %q, want the conversion's own %q", info.ID, session.SessionID)
	}
	if info.ProjectID != "" {
		t.Errorf("info.projectID = %q, want the empty string: the importer requires the member and "+
			"overwrites its value with the project its own rule computes", info.ProjectID)
	}
	if info.Cost != 0 || info.Tokens != (openCodeTokens{}) {
		t.Errorf("usage = %v/%+v, want zeros: the conversation document carries no usage", info.Cost, info.Tokens)
	}
	if info.Title != "" || info.Subpath != "" || info.Outcome != "" {
		t.Errorf("info title/subpath/outcome = %q/%q/%q, want them absent: the harness records its own",
			info.Title, info.Subpath, info.Outcome)
	}
	if info.Time.Created == 0 || info.Time.Updated == 0 {
		t.Errorf("info.time = %+v, want both required instants", info.Time)
	}
	if info.Location.Directory != session.Cwd {
		t.Errorf("info.location.directory = %q, want the recorded working directory %q",
			info.Location.Directory, session.Cwd)
	}
	if info.Model == nil || info.Model.ID != "target-model" || info.Model.ProviderID != "target-provider" ||
		info.Model.Variant != "max" {
		t.Errorf("info.model = %+v, want the target home's own answered model reference", info.Model)
	}
	seen := map[string]int{}
	for index, message := range document.Messages {
		if !strings.HasPrefix(message.ID, openCodeMessageIDPrefix) ||
			len(message.ID) != len(openCodeMessageIDPrefix)+openCodeIDTailLength {
			t.Errorf("message %d id = %q, want the harness's own prefix and length", index, message.ID)
		}
		seen[message.ID]++
		if message.Time.Created == 0 {
			t.Errorf("message %d carries no created instant, which the importer requires", index)
		}
		switch message.Type {
		case openCodeMessageUser:
			if message.Text == "" {
				t.Errorf("message %d is a user message with no text", index)
			}
			if message.Files == nil || len(*message.Files) != 0 {
				t.Errorf("message %d files = %v, want the empty member the harness's own user messages carry",
					index, message.Files)
			}
		case openCodeMessageAssistant:
			// Measured on 2.2.5: an assistant message whose `time` carries `created` alone is accepted by
			// the importer and then dropped without an error -- the store holds no row for it and the
			// harness's own exporter does not show it -- so this member is required rather than optional,
			// and the structural read-back is what disclosed it.
			if message.Time.Completed == 0 || message.Time.Completed < message.Time.Created {
				t.Errorf("message %d time = %+v, want a completion instant at or after the record's own "+
					"creation: the importer silently drops an assistant message without one", index, message.Time)
			}
			if message.Agent != "build" {
				t.Errorf("message %d agent = %q, want the target home's own primary agent", index, message.Agent)
			}
			if message.Model == nil || message.Model.ID == "" || message.Model.ProviderID == "" {
				t.Errorf("message %d model = %+v, want a complete reference: the importer requires it", index, message.Model)
			}
			if len(message.Content) == 0 {
				t.Errorf("message %d carries no content", index)
			}
			for partIndex, part := range message.Content {
				if part.Time != nil {
					continue
				}
				if part.Type == openCodePartTool {
					t.Errorf("message %d part %d is a tool part with no time member, which the importer requires",
						index, partIndex)
				}
			}
		case openCodeMessageCompaction:
			if message.Summary == nil || message.Status != openCodeCompactionStatus || message.Reason != openCodeCompactionReason {
				t.Errorf("message %d compaction = %+v, want the harness's own three required members", index, message)
			}
		default:
			t.Errorf("message %d type = %q, want a type this writer writes", index, message.Type)
		}
	}
	if len(seen) != len(document.Messages) {
		t.Errorf("message ids are not unique: %d message(s), %d id(s)", len(document.Messages), len(seen))
	}
}

// TestConvertToOpenCodeSessionCarriesTheFixturesEvidence pins what the fixture's own conversation becomes:
// the two user turns, the four assistant turns, the tool call with its result, and the compaction
// boundary -- each counted, and each dialogue item spelled the way a read of the installed session shows
// the parts (the reader joins a message's dialogue with its wrapped reasoning).
func TestConvertToOpenCodeSessionCarriesTheFixturesEvidence(t *testing.T) {
	conversation := readFixtureConversation(t, claudeFixtureRef)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	carried := session.Carried
	if carried.Messages != 7 || carried.UserMessages != 2 || carried.AssistantMessages != 4 ||
		carried.CompactionBoundaries != 1 || carried.ToolCalls != 1 || carried.ToolResults != 1 ||
		carried.Turns != 2 {
		t.Errorf("carried = %+v, want the fixture's two user turns, four assistant messages, one boundary "+
			"and its call/result pair", carried)
	}
	if carried.Bytes != len(session.Data) {
		t.Errorf("carried bytes = %d, want the document's own %d", carried.Bytes, len(session.Data))
	}
	var users, assistants []string
	var boundaries []string
	var calls, results []TargetItem
	for _, item := range session.Items {
		switch item.Kind {
		case ItemUser:
			users = append(users, item.Text)
		case ItemAssistant:
			assistants = append(assistants, item.Text)
		case ItemCompactionBoundary:
			boundaries = append(boundaries, item.Text)
		case ItemToolCall:
			calls = append(calls, item)
		case ItemToolResult:
			results = append(results, item)
		}
	}
	wantUsers := []string{
		"Summarize the retry policy in internal/retry/policy.go.",
		"Now check where the backoff cap is configured.",
	}
	if strings.Join(users, "|") != strings.Join(wantUsers, "|") {
		t.Errorf("user items = %q, want %q", users, wantUsers)
	}
	if len(assistants) != 3 {
		t.Errorf("assistant items = %q, want the fixture's three spoken assistant turns (the call-bearing "+
			"record carries no dialogue)", assistants)
	}
	if len(boundaries) != 1 || !strings.HasPrefix(boundaries[0], "Summary: the retry policy caps") {
		t.Errorf("boundary items = %q, want the fixture's compact summary", boundaries)
	}
	if len(calls) != 1 || calls[0].ToolName != "Read" ||
		calls[0].CallID != "toolu_01Example0000000000000000" {
		t.Errorf("call items = %+v, want the fixture's own Read call", calls)
	}
	if len(results) != 1 || !strings.Contains(results[0].Text, "MaxAttempts = 4") {
		t.Errorf("result items = %+v, want the fixture's own recorded output", results)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
	openCodeRequireLoss(t, session, "empty_tool_results", 0)
	openCodeRequireLoss(t, session, "reasoning_text", 0)
	// The fixture's tool part carries the two records' own instants: the call's record at 09:15:06.000Z
	// and the result's at 09:15:06.500Z, which the committed bytes record half a second apart. A part
	// whose `completed` was the call's own instant would state a call the source did not record.
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	callAt := time.Date(2026, 5, 4, 9, 15, 6, 0, time.UTC)
	toolParts := 0
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type != openCodePartTool {
				continue
			}
			toolParts++
			if part.Time == nil || part.Time.Created != callAt.UnixMilli() ||
				part.Time.Completed != callAt.Add(500*time.Millisecond).UnixMilli() {
				t.Errorf("the fixture's tool part time = %+v, want created %d and completed %d",
					part.Time, callAt.UnixMilli(), callAt.Add(500*time.Millisecond).UnixMilli())
			}
		}
	}
	if toolParts != 1 {
		t.Errorf("the document holds %d tool part(s), want the fixture's one call", toolParts)
	}
	// Every assistant message written carries the completion instant the importer requires (measured:
	// without it the harness accepts the document and drops the message), and the report names that the
	// source answered no span for it.
	openCodeRequireLoss(t, session, "assistant_completion_instant", carried.AssistantMessages)
}

// rooCodeReasoningOnlyConversation writes one RooCode task directory -- the provider's own
// `tasks/<task>/history_item.json` plus `ui_messages.json` shape, below a throw-away root -- and reads it
// through the inherited reader: the source shape this route's dialogue rule exists for. The task holds
// the provider's own `say=reasoning` record (the trace in the record's text field, which the reader
// renders into Content and answers as ThinkingText with no dialogue projection at all) between a user
// turn and an answer.
func rooCodeReasoningOnlyConversation(t *testing.T, trace string) Conversation {
	t.Helper()
	const (
		taskID      = "task-reasoning-only"
		startMillis = 1777886100000
	)
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("preparing the RooCode task directory: %v", err)
	}
	history := `{"apiConfigName":"anthropic/claude-sonnet-4-5","id":"` + taskID + `","task":"Check the ` +
		`retry policy","ts":1777886100000,"workspace":"/tmp/ccc-opencode-roocode"}`
	messages := `[{"say":"text","text":"Check the retry policy.","ts":1777886100000,"type":"say"},` +
		`{"say":"reasoning","text":` + jsonString(t, trace) + `,"ts":1777886101000,"type":"say"},` +
		`{"say":"text","text":"The cap is four attempts.","ts":1777886102000,"type":"say"}]`
	for name, content := range map[string]string{
		"history_item.json": history,
		"ui_messages.json":  messages,
	} {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentRooCode: {root}}
	conversation, err := NewSourceReader(cfg).Read(context.Background(),
		Ref{Provider: string(parser.AgentRooCode), ID: taskID})
	if err != nil {
		t.Fatalf("reading the RooCode source: %v", err)
	}
	return conversation
}

// TestOpenCodeCarriesARooCodeReasoningOnlyRecordOnce pins the writer's dialogue rule for the inherited
// source shape the rule exists for. RooCode's reader answers a `say=reasoning` record as an assistant
// message whose Content is the rendering of the record's own persisted trace and whose dialogue is not
// projected at all, so the writer's Content fallback used to write that trace twice -- once as the
// message's `reasoning` part (the shape OpenCode persists a trace in) and once as a `text` part holding
// the reader's `[Thinking]…[/Thinking]` rendering, which presents persisted reasoning as words the agent
// said. What this case reads is the serialized document's own parts: the trace once, in the part the
// format keeps a trace in, with no text part carrying the rendering; the report counting the trace as
// carried because it is; and the store read-back verifying the message through the inherited reader's own
// rendering of that single part.
func TestOpenCodeCarriesARooCodeReasoningOnlyRecordOnce(t *testing.T) {
	const trace = "Inspect the parser before answering."
	conversation := rooCodeReasoningOnlyConversation(t, trace)

	// The shape is asserted on the read, not assumed: the record carries the trace, the source projected
	// no dialogue for it, and its Content is the reader's own rendering of that trace.
	sawReasoningOnly := false
	for _, message := range conversation.Messages {
		if message.Thinking != trace {
			continue
		}
		sawReasoningOnly = true
		if message.DialogueProjected || message.Dialogue != "" {
			t.Fatalf("the RooCode record's dialogue = %q (projected %v), want none: the shape this rule is "+
				"about is a record the source answered no projection for", message.Dialogue,
				message.DialogueProjected)
		}
		if message.Content != openCodeReasoningOpen+trace+openCodeReasoningClose {
			t.Fatalf("the RooCode record's content = %q, want the reader's rendering %q of its own trace",
				message.Content, openCodeReasoningOpen+trace+openCodeReasoningClose)
		}
	}
	if !sawReasoningOnly {
		t.Fatalf("the read holds no record carrying %q, so nothing here pins what a writer does with the "+
			"shape this case is about", trace)
	}

	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	reasoningPart := -1
	for index, message := range document.Messages {
		if len(message.Content) > 0 && message.Content[0].Type == openCodePartReasoning {
			reasoningPart = index
		}
	}
	if reasoningPart < 0 {
		t.Fatalf("the document holds no reasoning part for the record's trace: %s", session.Data)
	}
	if parts := document.Messages[reasoningPart].Content; len(parts) != 1 ||
		parts[0].Type != openCodePartReasoning || parts[0].Text != trace {
		t.Errorf("the record's message holds %+v, want the trace once as the one reasoning part", parts)
	}
	if strings.Contains(string(session.Data), openCodeReasoningOpen) {
		t.Errorf("the document holds the reader's own `[Thinking]` rendering of the trace, which is not "+
			"dialogue the source recorded: %s", session.Data)
	}
	openCodeRequireLoss(t, session, "reasoning_text", 1)
	loss := lossCount(t, session.Losses, "reasoning_text")
	if loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("reasoning_text = %+v, want the trace the reasoning part carries counted as carried", loss)
	}

	// The read-back rule holds for the single part as it does for any other message: the comparison's
	// intended dialogue is the rendering the inherited reader makes of the parts, so a store holding the
	// message verifies with no difference.
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home),
		session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified || verification.Detail != "" {
		t.Errorf("structure = %+v, want the single-part message read back as what was written "+
			"(detail %q)", verification, verification.Detail)
	}

	// The rule recognises a *rendering of the record's own trace*, and the record's trace is what makes it
	// one: the same words on a record the read answered no trace for are the record's own text, and a
	// target must not drop text it cannot show to be a rendering of reasoning. (The writer's own double of
	// the shape: a reader that renders a trace without filling the trace field -- the inherited OpenCode
	// reader is one -- leaves Content as the message's text, which is a source-side matter of its own.)
	unrendered := openCodeTestConversation(Message{Role: "assistant", Timestamp: time.UnixMilli(1_700_000_000_000).UTC(),
		Content: openCodeReasoningOpen + "words the read answered no trace for" + openCodeReasoningClose})
	session, err = ConvertToOpenCodeSession(unrendered, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting a record with no persisted trace: %v", err)
	}
	if !strings.Contains(string(session.Data), "words the read answered no trace for") {
		t.Errorf("the document dropped a record's own text, which nothing shows to be the rendering of a "+
			"trace: %s", session.Data)
	}
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	parts := document.Messages[0].Content
	if len(parts) != 1 || parts[0].Type != openCodePartText {
		t.Errorf("the record's message holds %+v, want its text as the one text part", parts)
	}
}

// zCodeMixedConversation writes one ZCode store -- the provider's own `<cli root>/db/db.sqlite` shape,
// below a throw-away root -- and reads one session out of it through the inherited reader: the source
// shape this route's dialogue rule is about. The assistant record's parts are the caller's own
// provider-shaped `data` JSON, in the provider's own order, so a case can put the persisted trace before
// the record's text, after it, or beside a text of the record's own that merely looks like a rendering.
func zCodeMixedConversation(t *testing.T, assistantParts ...string) Conversation {
	t.Helper()
	const (
		sessionID = "session-mixed-rendering"
		at        = "2026-07-06T13:00:00Z"
		answered  = "2026-07-06T13:00:03Z"
	)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatalf("preparing the ZCode store directory: %v", err)
	}
	database, err := sql.Open("sqlite3", filepath.Join(root, "db", parser.ZCodeDBName))
	if err != nil {
		t.Fatalf("opening the ZCode store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// The provider's own tables, as its reader queries them (internal/parser's zcode.go).
	if _, err := database.Exec(`
		CREATE TABLE session (id TEXT PRIMARY KEY NOT NULL, project_id TEXT, workspace_id TEXT,
			directory TEXT, title TEXT, time_created TEXT, time_updated TEXT);
		CREATE TABLE model_usage (session_id TEXT NOT NULL, turn_id TEXT, provider_id TEXT, model_id TEXT,
			status TEXT, input_tokens INTEGER, output_tokens INTEGER, reasoning_tokens INTEGER,
			cache_creation_input_tokens INTEGER, cache_read_input_tokens INTEGER,
			computed_total_tokens INTEGER, started_at TEXT, completed_at TEXT, duration_ms INTEGER,
			tool_call_count INTEGER);
		CREATE TABLE message (id TEXT PRIMARY KEY NOT NULL, session_id TEXT NOT NULL, time_created TEXT,
			data TEXT);
		CREATE TABLE part (id TEXT PRIMARY KEY NOT NULL, message_id TEXT NOT NULL, session_id TEXT NOT NULL,
			time_created TEXT, data TEXT);
	`); err != nil {
		t.Fatalf("creating the ZCode store's tables: %v", err)
	}
	write := func(query string, args ...any) {
		t.Helper()
		if _, err := database.Exec(query, args...); err != nil {
			t.Fatalf("writing the ZCode store: %v", err)
		}
	}
	write(`INSERT INTO session (id, project_id, workspace_id, directory, title, time_created, time_updated)
		VALUES (?, '', '', ?, ?, ?, ?)`, sessionID, "/tmp/ccc-opencode-zcode", "Mixed rendering", at,
		answered)
	write(`INSERT INTO message (id, session_id, time_created, data) VALUES (?, ?, ?, ?)`,
		"msg-user", sessionID, at, `{"role":"user"}`)
	write(`INSERT INTO part (id, message_id, session_id, time_created, data) VALUES (?, ?, ?, ?, ?)`,
		"part-user", "msg-user", sessionID, at, `{"type":"text","text":"Fix the login bug."}`)
	write(`INSERT INTO message (id, session_id, time_created, data) VALUES (?, ?, ?, ?)`,
		"msg-assistant", sessionID, answered, `{"role":"assistant"}`)
	for index, part := range assistantParts {
		write(`INSERT INTO part (id, message_id, session_id, time_created, data) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("part-assistant-%02d", index), "msg-assistant", sessionID, answered, part)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentZCode: {root}}
	conversation, err := NewSourceReader(cfg).Read(context.Background(),
		Ref{Provider: string(parser.AgentZCode), ID: sessionID})
	if err != nil {
		t.Fatalf("reading the ZCode source: %v", err)
	}
	return conversation
}

// zCodeAssistantParts answers the parts of the one assistant message a converted document holds: the
// cases below read the serialized document's own parts rather than the writer's intended items.
func zCodeAssistantParts(t *testing.T, session OpenCodeSession) []openCodePart {
	t.Helper()
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	for _, message := range document.Messages {
		if message.Type == openCodeMessageAssistant {
			return message.Content
		}
	}
	t.Fatalf("the document holds no assistant message: %s", session.Data)
	return nil
}

// TestOpenCodeCarriesAZCodeMixedRenderingOnce pins the writer's dialogue rule for the inherited
// mixed-rendering shape, through the inherited reader that produces it: a ZCode assistant record carrying
// both a persisted thinking part and text has that trace rendered into Content as a `[Thinking]…[/Thinking]`
// block *beside* the record's text, and projects no dialogue at all (internal/parser's ZCode reader, whose
// own transcript test asserts this shape). The writer's Content fallback used to write the trace twice --
// once as the message's `reasoning` part, the shape this target persists a trace in, and once inside a
// `text` part holding the rendering, which presents persisted reasoning as words the agent said -- with
// the round-trip comparison agreeing, because both sides derived from the same duplicated parts. What
// this case reads is the serialized document's own parts, the trace's own loss category, and the store
// read-back.
func TestOpenCodeCarriesAZCodeMixedRenderingOnce(t *testing.T) {
	const (
		trace  = "I should inspect the auth flow."
		answer = "I'll inspect the auth flow."
	)
	thinkingPart := `{"type":"thinking","thinking":` + jsonString(t, trace) + `}`
	textPart := func(value string) string {
		return `{"type":"text","text":` + jsonString(t, value) + `}`
	}
	rendered := openCodeReasoningOpen + trace + openCodeReasoningClose

	conversation := zCodeMixedConversation(t, thinkingPart, textPart(answer))

	// The shape is asserted on the read, not assumed: the record carries the trace, the source projected
	// no dialogue for it, and its Content is the reader's own rendering followed by the record's text.
	sawMixed := false
	for _, message := range conversation.Messages {
		if message.Thinking != trace {
			continue
		}
		sawMixed = true
		if message.DialogueProjected || message.Dialogue != "" {
			t.Fatalf("the ZCode record's dialogue = %q (projected %v), want none: the shape this rule is "+
				"about is a record the source answered no projection for", message.Dialogue,
				message.DialogueProjected)
		}
		if want := rendered + "\n" + answer; message.Content != want {
			t.Fatalf("the ZCode record's content = %q, want the reader's own rendering followed by the "+
				"record's text (%q)", message.Content, want)
		}
	}
	if !sawMixed {
		t.Fatalf("the read holds no record carrying %q, so nothing here pins what a writer does with the "+
			"shape this case is about", trace)
	}

	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	parts := zCodeAssistantParts(t, session)
	if len(parts) != 2 || parts[0].Type != openCodePartReasoning || parts[0].Text != trace ||
		parts[1].Type != openCodePartText || parts[1].Text != answer {
		t.Fatalf("the record's message holds %+v, want the trace once as the reasoning part and the "+
			"record's own text beside it", parts)
	}
	if strings.Contains(string(session.Data), openCodeReasoningOpen) {
		t.Errorf("the document holds the reader's own `[Thinking]` rendering of the trace, which is not "+
			"dialogue the source recorded: %s", session.Data)
	}
	openCodeRequireLoss(t, session, "reasoning_text", 1)
	if loss := lossCount(t, session.Losses, "reasoning_text"); loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("reasoning_text = %+v, want the trace the reasoning part carries counted as carried", loss)
	}
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home),
		session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified || verification.Detail != "" {
		t.Errorf("structure = %+v, want the message read back as what was written (detail %q)",
			verification, verification.Detail)
	}

	// The rendering is removed where it stands, not only where it leads: ZCode keeps the record's own part
	// order, so a text part recorded before the thinking part renders the trace *after* the text, and the
	// record's text is still the dialogue.
	trailing := zCodeMixedConversation(t, textPart(answer), thinkingPart)
	session, err = ConvertToOpenCodeSession(trailing, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting a record whose trace part follows its text: %v", err)
	}
	sawTrailing := false
	for _, message := range trailing.Messages {
		if message.Thinking != trace {
			continue
		}
		sawTrailing = true
		if want := answer + "\n" + rendered; message.Content != want {
			t.Fatalf("the ZCode record's content = %q, want the record's text followed by the rendering "+
				"(%q)", message.Content, want)
		}
	}
	if !sawTrailing {
		t.Fatalf("the read holds no record carrying %q in its trailing-part shape", trace)
	}
	parts = zCodeAssistantParts(t, session)
	if len(parts) != 2 || parts[0].Type != openCodePartReasoning || parts[0].Text != trace ||
		parts[1].Type != openCodePartText || parts[1].Text != answer {
		t.Errorf("a record whose trace part follows its text holds %+v, want the trace once as the "+
			"reasoning part and the record's own text beside it", parts)
	}

	// A `[Thinking]…[/Thinking]` block the record's own trace does not hold is text the record carries (a
	// model writing the markers itself), and a target must not drop text nothing shows to be reasoning.
	literal := "[Thinking]\nwords the record's trace does not hold\n[/Thinking]"
	lookalike := zCodeMixedConversation(t, thinkingPart, textPart(literal))
	session, err = ConvertToOpenCodeSession(lookalike, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting a record carrying the markers as its own text: %v", err)
	}
	parts = zCodeAssistantParts(t, session)
	if len(parts) != 2 || parts[0].Type != openCodePartReasoning || parts[0].Text != trace ||
		parts[1].Type != openCodePartText || parts[1].Text != literal {
		t.Errorf("the record's message holds %+v, want the trace once and its own marker-bearing text "+
			"kept as it stands", parts)
	}
}

// TestOpenCodeDialogueItemsAreTheReadersOwnRendering pins the route's comparison spelling: a message's
// dialogue item is what a read of the installed session shows for its parts, which for a persisted
// reasoning trace is the reader's wrapped `[Thinking]…[/Thinking]` text joined with the dialogue. A
// document whose items were the bare dialogue would report every carried trace as a difference.
func TestOpenCodeDialogueItemsAreTheReadersOwnRendering(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true, Timestamp: at},
		Message{
			Role: "assistant", Content: "hi", Dialogue: "hi", DialogueProjected: true,
			Thinking: "weigh the options", Model: "source-model", Timestamp: at.Add(time.Second),
		},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var assistant string
	for _, item := range session.Items {
		if item.Kind == ItemAssistant {
			assistant = item.Text
		}
	}
	want := openCodeReasoningOpen + "weigh the options" + openCodeReasoningClose + "\nhi"
	if assistant != want {
		t.Errorf("assistant item = %q, want the reader's own rendering %q", assistant, want)
	}
	openCodeRequireLoss(t, session, "reasoning_text", 1)

	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	message := document.Messages[len(document.Messages)-1]
	if len(message.Content) != 2 || message.Content[0].Type != openCodePartReasoning ||
		message.Content[1].Type != openCodePartText {
		t.Fatalf("assistant content = %+v, want a reasoning part then a text part", message.Content)
	}
	if message.Model == nil || message.Model.ID != "source-model" ||
		message.Model.ProviderID != "target-provider" {
		t.Errorf("assistant model = %+v, want the source's model id and the target home's provider", message.Model)
	}
	openCodeRequireLoss(t, session, "message_model_attribution", 1)
	openCodeRequireLoss(t, session, "target_model_id_attribution", 0)

	// A record the read answered no model for gets the target home's own model id, counted as that.
	conversation.Messages[1].Model = ""
	session, err = ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting without a source model: %v", err)
	}
	openCodeRequireLoss(t, session, "target_model_id_attribution", 1)
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if got := document.Messages[1].Model; got == nil || got.ID != "target-model" {
		t.Errorf("assistant model = %+v, want the target home's own model id", got)
	}
}

// TestOpenCodeToolPartsCarryOnlyWhatTheImporterAccepts pins every tool rule the evidence record measured:
// a completed state carries the recorded text, a failed result becomes an error state whose message is
// that text, a call with no result (or with no writable one) is left out and counted rather than written
// in a state the importer refuses, and an empty input object is written where none was recorded.
func TestOpenCodeToolPartsCarryOnlyWhatTheImporterAccepts(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Content: "[Read: x]", Timestamp: at,
			ToolCalls: []ToolCall{
				openCodeTestCall("call-ok", "Read", `{"file_path":"/x"}`),
				openCodeTestCall("call-failed", "Bash", `{"command":"false"}`),
				openCodeTestCall("call-unpaired", "Grep", `{"pattern":"x"}`),
				openCodeTestCall("call-empty", "Read", `{"file_path":"/y"}`),
				openCodeTestCall("call-noinput", "Read", ""),
				openCodeTestCall("call-repeated", "Read", `{"file_path":"/z"}`),
				openCodeTestCall("call-blocks", "Read", `{"file_path":"/w"}`),
				// The raw-text input shape the inherited Codex reader passes through unchanged for a
				// `custom_tool_call` (internal/parser's extractCodexInputJSON): not an object, so the
				// importer's required `input` member is written as the empty object and the replacement
				// is what the report names.
				openCodeTestCall("call-rawinput", "apply_patch", "*** Begin Patch\n*** End Patch"),
			},
		},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call-ok", "completed", `"line one\nline two"`),
			openCodeTestResult("call-failed", "errored", `"permission denied"`),
			openCodeTestResult("call-empty", "completed", ""),
			openCodeTestResult("call-noinput", "completed", `"ok"`),
			openCodeTestResult("call-repeated", "completed", `"first"`),
			openCodeTestResult("call-repeated", "completed", `"second"`),
			openCodeTestResult("call-blocks", "completed", `[{"type":"text","text":"shown"},{"type":"image","source":{"data":"AAAA"}}]`),
			openCodeTestResult("call-rawinput", "completed", `"applied"`),
			openCodeTestResult("call-nowhere", "completed", `"orphan"`),
		}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	parts := map[string]openCodePart{}
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				parts[part.ID] = part
			}
		}
	}
	ok := parts["call-ok"]
	if ok.State == nil || ok.State.Status != openCodeToolCompleted || len(ok.State.Content) != 1 ||
		ok.State.Content[0].Text != "line one\nline two" {
		t.Errorf("call-ok part = %+v, want a completed state carrying the recorded output", ok)
	}
	if ok.Time == nil || ok.Time.Created == 0 || ok.Time.Completed == 0 {
		t.Errorf("call-ok time = %+v, want the required created/completed pair", ok.Time)
	}
	// The pair's own instants are the records they were recorded on: the call's own message for
	// `created`, and the message the result travelled on for `completed` -- a call written with its own
	// instant twice would state a zero-length call the source did not record.
	if ok.Time.Created != at.UnixMilli() {
		t.Errorf("call-ok created = %d, want the call record's own instant %d", ok.Time.Created, at.UnixMilli())
	}
	if ok.Time.Completed != at.Add(time.Second).UnixMilli() {
		t.Errorf("call-ok completed = %d, want the result record's own instant %d",
			ok.Time.Completed, at.Add(time.Second).UnixMilli())
	}
	// Every part's pair is the records its call and its result were recorded on, not the previous part's
	// completion: these calls share one message and their results one later record, so every part states
	// the same recorded span. Asserting only the first call's start was what let a sibling's completion
	// rewrite the later calls' start times to it.
	for id, part := range parts {
		if part.Time == nil || part.Time.Created != at.UnixMilli() ||
			part.Time.Completed != at.Add(time.Second).UnixMilli() {
			t.Errorf("%s time = %+v, want created %d and completed %d: each part carries its own records' "+
				"instants", id, part.Time, at.UnixMilli(), at.Add(time.Second).UnixMilli())
		}
	}
	failed := parts["call-failed"]
	if failed.State == nil || failed.State.Status != openCodeToolError || failed.State.Error == nil ||
		failed.State.Error.Type != openCodeToolErrorType || failed.State.Error.Message != "permission denied" {
		t.Errorf("call-failed part = %+v, want an error state whose message is the recorded text", failed)
	}
	if failed.State.Content != nil {
		t.Errorf("call-failed content = %+v, want no content on an error state", failed.State.Content)
	}
	if _, ok := parts["call-unpaired"]; ok {
		t.Error("a call the source recorded with no result was written, and the importer accepts no state for it")
	}
	if _, ok := parts["call-empty"]; ok {
		t.Error("a call whose result holds no text was written, and the importer refuses an empty content")
	}
	repeated := parts["call-repeated"]
	if repeated.State == nil || len(repeated.State.Content) != 1 || repeated.State.Content[0].Text != "first" {
		t.Errorf("call-repeated part = %+v, want the first recorded result: one tool part holds one state", repeated)
	}
	noinput := parts["call-noinput"]
	if noinput.State == nil || string(noinput.State.Input) != "{}" {
		t.Errorf("call-noinput state = %+v, want the empty object the importer requires", noinput.State)
	}
	rawinput := parts["call-rawinput"]
	if rawinput.State == nil || string(rawinput.State.Input) != "{}" {
		t.Errorf("call-rawinput state = %+v, want the empty object the importer requires for an input "+
			"that is not one", rawinput.State)
	}
	// The round-trip walk compares the store against what was written, so the call's item carries the
	// input the part's own member holds -- the empty object where the recorded value is not one, the
	// source's own value (the store keeps the value and re-encodes the spelling) otherwise. The source's
	// own input is still what the run accounts for, which is the empty_tool_inputs caveat's own count.
	items := map[string]TargetItem{}
	for _, item := range session.Items {
		if item.Kind == ItemToolCall {
			items[item.CallID] = item
		}
	}
	for callID, want := range map[string]string{
		"call-ok":       `{"file_path":"/x"}`,
		"call-noinput":  "{}",
		"call-rawinput": "{}",
	} {
		if item, present := items[callID]; !present {
			t.Errorf("no tool-call item for %s", callID)
		} else if item.InputJSON != want {
			t.Errorf("%s item input = %q, want %q: the item carries what the part holds",
				callID, item.InputJSON, want)
		}
	}
	blocks := parts["call-blocks"]
	if blocks.State == nil || len(blocks.State.Content) != 1 || blocks.State.Content[0].Text != "shown" {
		t.Errorf("call-blocks = %+v, want the text channel's own text", blocks)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "empty_tool_results", 1)
	openCodeRequireLoss(t, session, "repeated_tool_results", 1)
	openCodeRequireLoss(t, session, "nonterminal_tool_events", 0)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 1)
	openCodeRequireLoss(t, session, "empty_tool_inputs", 2)
	openCodeRequireLoss(t, session, "tool_result_statuses", 0)
	openCodeRequireLoss(t, session, "unrepresentable_result_blocks", 1)
	if session.Carried.ToolCalls != 6 || session.Carried.ToolResults != 6 {
		t.Errorf("carried = %+v, want the six calls whose parts were written", session.Carried)
	}
}

// TestOpenCodeCaveatCountsFollowThePartThatWasWritten pins what the two caveat categories' own details
// state -- `empty_tool_inputs` that an empty object "was written" in the part's input member, and
// `unresolved_tool_outputs` that "only results that were written are counted here" -- by counting each
// of them on the writing path and on the paths that omit the part. The conditions are combined as the
// finding made them: a call with no argument object whose result's status word has no truthful state
// (the part is left out and the status counted), and an unresolved placeholder whose result holds no
// text at all (the part is left out for that). Counting either before its guard reported a part the
// document does not hold (round 11's finding).
func TestOpenCodeCaveatCountsFollowThePartThatWasWritten(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	unresolved := func(id, status, content string) ToolResult {
		return ToolResult{
			ToolUseID: id, Status: status,
			ContentRaw: content, ContentSize: len(content), PersistedOutputUnresolved: true,
		}
	}
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Content: "[Read: x]", Timestamp: at,
			ToolCalls: []ToolCall{
				openCodeTestCall("call-noinput-cancelled", "Bash", ""),
				openCodeTestCall("call-unresolved-cancelled", "Read", `{"file_path":"/a"}`),
				openCodeTestCall("call-unresolved-empty", "Read", `{"file_path":"/b"}`),
				openCodeTestCall("call-noinput-ok", "Read", ""),
				openCodeTestCall("call-unresolved-ok", "Read", `{"file_path":"/c"}`),
			}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call-noinput-cancelled", "cancelled", `"Stop requested"`),
			unresolved("call-unresolved-cancelled", "cancelled", `"the side file could not be read"`),
			unresolved("call-unresolved-empty", "completed", ""),
			openCodeTestResult("call-noinput-ok", "completed", `"ok"`),
			unresolved("call-unresolved-ok", "completed", `"the side file could not be read"`),
		}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	parts := openCodeToolParts(t, session)
	for _, id := range []string{"call-noinput-cancelled", "call-unresolved-cancelled", "call-unresolved-empty"} {
		if part, ok := parts[id]; ok {
			t.Errorf("%s part = %+v, want no part: the call has no state here or holds no output text, "+
				"so the caveat that names its input or its placeholder must not count it as carried", id, part)
		}
	}
	for _, id := range []string{"call-noinput-ok", "call-unresolved-ok"} {
		if _, ok := parts[id]; !ok {
			t.Errorf("%s has no part, want the written part its caveat counts", id)
		}
	}
	openCodeRequireLoss(t, session, "tool_result_statuses", 2)
	openCodeRequireLoss(t, session, "empty_tool_results", 1)
	openCodeRequireLoss(t, session, "empty_tool_inputs", 1)
	openCodeRequireLoss(t, session, "unresolved_tool_outputs", 1)
	if session.Carried.ToolCalls != 2 || session.Carried.ToolResults != 2 {
		t.Errorf("carried = %+v, want the two calls whose parts were written", session.Carried)
	}
	for _, category := range []string{"empty_tool_inputs", "unresolved_tool_outputs"} {
		if loss := lossCount(t, session.Losses, category); loss.Disposition != DispositionCarriedWithCaveat {
			t.Errorf("%s = %+v, want the category still reported as carried-with-caveat", category, loss)
		}
	}
}

// openCodeToolParts indexes one converted document's tool parts by the id they carry, so a case can ask
// what the pairing wrote without re-walking the document's messages itself.
func openCodeToolParts(t *testing.T, session OpenCodeSession) map[string]openCodePart {
	t.Helper()
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	parts := map[string]openCodePart{}
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				parts[part.ID] = part
			}
		}
	}
	return parts
}

// TestOpenCodeGeneratedCallIDsDoNotAliasRecordedOnes pins the namespace rule the pairing map needs: the
// map is keyed by id across the whole document, so an id minted for a call that recorded none must not
// reuse any spelling the read already holds. Reusing a recorded call's id would replace that call's own
// entry in the plan's map -- handing its recorded result to the id-less call and dropping the call the
// result actually answers -- and reusing an id a recorded result names would take a result this
// conversion cannot pair and present it as the invented call's outcome.
func TestOpenCodeGeneratedCallIDsDoNotAliasRecordedOnes(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	// The second call of message 1 records no id, and the first call of that same message carries the id
	// openCodeCallID(1, 1) would mint for it: the collision the generator must walk past.
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "hello", DialogueProjected: true, Content: "hello", Timestamp: at},
		Message{Role: "assistant", Dialogue: "two calls", DialogueProjected: true, Content: "two calls",
			Timestamp: at.Add(time.Second), ToolCalls: []ToolCall{
				openCodeTestCall("call_1_1", "Read", `{"file_path":"/recorded"}`),
				openCodeTestCall("", "Read", `{"file_path":"/id-less"}`),
			}},
		Message{Role: "user", Timestamp: at.Add(2 * time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call_1_1", "completed", `"the recorded call's output"`),
		}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	parts := openCodeToolParts(t, session)
	if len(parts) != 1 {
		t.Fatalf("the document holds %d tool part(s), want one: the id-less call has no outcome here",
			len(parts))
	}
	part, ok := parts["call_1_1"]
	if !ok {
		t.Fatalf("the document holds no part under the recorded call's own id: %v", parts)
	}
	if part.State == nil || string(part.State.Input) != `{"file_path":"/recorded"}` {
		t.Errorf("call_1_1 part = %+v, want the recorded call's own input %q: the id-less sibling must "+
			"not have taken its id", part, `{"file_path":"/recorded"}`)
	}
	if part.State == nil || len(part.State.Content) != 1 ||
		part.State.Content[0].Text != "the recorded call's output" {
		t.Errorf("call_1_1 part = %+v, want the result the source recorded for that id", part)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 0)
	openCodeRequireLoss(t, session, "generated_call_ids", 0)

	// The namespace is the whole document's, results included: a result whose recorded id no carried call
	// holds still keeps its spelling out of the minted ids, so it is answered as unpaired rather than as
	// the invented call's own outcome.
	orphaned := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "one call", DialogueProjected: true, Content: "one call",
			Timestamp: at, ToolCalls: []ToolCall{openCodeTestCall("", "Read", `{"file_path":"/id-less"}`)}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call_0_0", "completed", `"an orphan result"`),
		}},
	)
	session, err = ConvertToOpenCodeSession(orphaned, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the conversation whose result names no carried call: %v", err)
	}
	if parts := openCodeToolParts(t, session); len(parts) != 0 {
		t.Errorf("the document holds %v, want no tool part: the id-less call has no result and the result "+
			"names no carried call", parts)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 1)
	openCodeRequireLoss(t, session, "generated_call_ids", 0)

	// And the minted ids do not alias each other: two id-less calls in one message are two calls, so an
	// id-less result that arrives while both are outstanding has no sole call it could belong to and is
	// counted unpaired. Collapsing the two minted spellings into one would leave exactly one pending call
	// and the result would be attached to a call the source did not record it for.
	twoBare := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "two calls", DialogueProjected: true, Content: "two calls",
			Timestamp: at, ToolCalls: []ToolCall{
				openCodeTestCall("", "Read", `{"file_path":"/first"}`),
				openCodeTestCall("", "Read", `{"file_path":"/second"}`),
			}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("", "completed", `"the only recorded result"`),
		}},
	)
	session, err = ConvertToOpenCodeSession(twoBare, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the conversation with two id-less calls: %v", err)
	}
	if parts := openCodeToolParts(t, session); len(parts) != 0 {
		t.Errorf("the document holds %v, want no tool part: two calls were outstanding, so the id-less "+
			"result belongs to neither", parts)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 2)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 1)
	openCodeRequireLoss(t, session, "generated_call_ids", 0)
}

// TestOpenCodeFlatTargetOmitsABranchAndCountsIt pins the flat target's source-side rule: the line is
// the one the read's *last* record selects (here rec-3, which named rec-1), so that record's own text
// stays and the sibling branch the conversation left (rec-2, hanging off the same rec-1) is omitted --
// a message list has no parent relation to hang the branch on, and writing it would revive an abandoned
// branch as active history -- and the walk's own count of the places it could not answer exactly is
// reported. Both members the writer puts dialogue in are inspected: a user message's `text` and an
// assistant message's content parts. Reading only `message.Text` never saw an assistant branch at all,
// which is how a reversed expectation could sit here unnoticed.
func TestOpenCodeFlatTargetOmitsABranchAndCountsIt(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "first", DialogueProjected: true, Content: "first",
			SourceUUID: "rec-1", Timestamp: at},
		Message{Role: "assistant", Dialogue: "off the line", DialogueProjected: true, Content: "off the line",
			SourceUUID: "rec-2", SourceParentUUID: "rec-1", Timestamp: at.Add(time.Second)},
		Message{Role: "assistant", Dialogue: "on the line", DialogueProjected: true, Content: "on the line",
			SourceUUID: "rec-3", SourceParentUUID: "rec-1", Timestamp: at.Add(2 * time.Second)},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	var spoken []string
	for _, message := range document.Messages {
		if message.Text != "" {
			spoken = append(spoken, message.Text)
		}
		if dialogue := openCodePartsDialogue(message.Content); dialogue != "" {
			spoken = append(spoken, dialogue)
		}
	}
	dialogue := strings.Join(spoken, "\n")
	if !strings.Contains(dialogue, "on the line") {
		t.Errorf("dialogue = %q, want the last record's own \"on the line\": that record selects the line "+
			"the conversation reached", spoken)
	}
	if strings.Contains(dialogue, "off the line") {
		t.Errorf("dialogue = %q, want the sibling branch omitted: its text was written as an active "+
			"message", spoken)
	}
	openCodeRequireLoss(t, session, "inactive_branch_records", 1)
	// The branch record's own parent is one the read answered (it named the first record), so this walk
	// places every relation it visits: the branch is what is wrong, not the relation.
	openCodeRequireLoss(t, session, "unresolved_source_parents", 0)
	// A record with an identity but no parent, and not the conversation's first, is the other place the
	// walk counts: its entry follows the one before it, which is this conversion's reading.
	orphan := openCodeTestConversation(
		Message{Role: "user", Dialogue: "first", DialogueProjected: true, Content: "first",
			SourceUUID: "rec-1", Timestamp: at},
		Message{Role: "assistant", Dialogue: "with no parent recorded", DialogueProjected: true,
			Content: "with no parent recorded", SourceUUID: "rec-2", Timestamp: at.Add(time.Second)},
	)
	session, err = ConvertToOpenCodeSession(orphan, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	openCodeRequireLoss(t, session, "unresolved_source_parents", 1)
	openCodeRequireLoss(t, session, "inactive_branch_records", 0)
}

// TestOpenCodeCarriesATerminalOutcomeRatherThanProgress pins the rule the plan's own selection follows:
// a call whose read records a progress event and then its outcome carries the outcome -- the terminal
// event the source recorded is what the part states, and the progress text is not written as a completed
// outcome the source never recorded. The shape is the inherited Codex reader's own: a subagent
// notification stream (`running` while the call executes, then `completed`), whose two events live on the
// call's own record (internal/parser/codex_parser_test.go's "running subagent notification does not
// suppress later completion").
func TestOpenCodeCarriesATerminalOutcomeRatherThanProgress(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "run a child agent", DialogueProjected: true,
			Content: "run a child agent", Timestamp: at},
		Message{Role: "assistant", Dialogue: "spawning", DialogueProjected: true, Content: "spawning",
			Timestamp: at.Add(time.Second),
			ToolCalls: []ToolCall{openCodeTestCall("call_spawn", "spawn_agent", `{"agent_type":"awaiter"}`)},
			ToolResults: []ToolResult{
				openCodeTestResult("call_spawn", "running", "Still working"),
				openCodeTestResult("call_spawn", "completed", "Finished successfully"),
			}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(session.Data), "Still working") {
		t.Error("the progress event's text was written into the document: a call's outcome is the " +
			"terminal event the source recorded, not the progress it streamed while executing")
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	var spawn openCodePart
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool && part.ID == "call_spawn" {
				spawn = part
			}
		}
	}
	if spawn.State == nil || spawn.State.Status != openCodeToolCompleted || len(spawn.State.Content) != 1 ||
		spawn.State.Content[0].Text != "Finished successfully" {
		t.Errorf("call_spawn part = %+v, want the terminal event's own text under `completed`", spawn)
	}
	openCodeRequireLoss(t, session, "nonterminal_tool_events", 1)
	openCodeRequireLoss(t, session, "repeated_tool_results", 0)
	openCodeRequireLoss(t, session, "tool_result_statuses", 0)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)

	// A call whose read holds only progress has no outcome here: no part is written for it (the harness's
	// states have no word for a call that is still executing), the progress event is counted, and the call
	// itself is counted as having no outcome rather than written with the progress text under `completed`.
	progressOnly := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "spawning", DialogueProjected: true, Content: "spawning",
			Timestamp:   at,
			ToolCalls:   []ToolCall{openCodeTestCall("call_pending", "spawn_agent", `{"agent_type":"awaiter"}`)},
			ToolResults: []ToolResult{openCodeTestResult("call_pending", "running", "Still working")}},
	)
	session, err = ConvertToOpenCodeSession(progressOnly, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting a call with only progress: %v", err)
	}
	if strings.Contains(string(session.Data), "Still working") {
		t.Error("a progress-only event's text was written as the call's outcome")
	}
	openCodeRequireLoss(t, session, "nonterminal_tool_events", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "repeated_tool_results", 0)
	openCodeRequireNoToolPart(t, session)
}

// TestOpenCodeArchiveSummaryIsAViewOfRecordedEvents pins the fallback's own rule: the summary the archive
// keeps beside a call is a view of the result events it recorded on that call (internal/db restores the
// sole content-bearing event's text into the column), so it is written only for a call whose read
// recorded no result event at all. A call whose only recorded event is progress -- the shape
// messageFromArchive produces when the column was restored from a `running` notification -- has no
// outcome this target can write, and its summary is that event's own text: writing it would state a
// completed outcome the source never recorded while the report counts the event as omitted.
func TestOpenCodeArchiveSummaryIsAViewOfRecordedEvents(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	progress := "Still working"
	archive := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "spawning", DialogueProjected: true, Content: "spawning",
			Timestamp: at,
			ToolCalls: []ToolCall{{
				ToolUseID: "call_pending", Name: "spawn_agent", InputJSON: `{"agent_type":"awaiter"}`,
				ResultContent: progress, ResultContentSize: len(progress),
			}},
			ToolResults: []ToolResult{{
				ToolUseID: "call_pending", Status: "running", ContentRaw: progress,
				ContentSize: len(progress), ContentDecoded: true,
			}}},
	)
	session, err := ConvertToOpenCodeSession(archive, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the archive shape: %v", err)
	}
	if strings.Contains(string(session.Data), progress) {
		t.Error("the archive's summary of a progress event was written as the call's outcome")
	}
	openCodeRequireLoss(t, session, "nonterminal_tool_events", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireNoToolPart(t, session)

	// The rule's other arm: a call whose read recorded no result event at all keeps the archive's summary
	// as the only evidence of its outcome, exactly as the family's writers hold it.
	summaryOnly := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "reading", DialogueProjected: true, Content: "reading",
			Timestamp: at,
			ToolCalls: []ToolCall{{
				ToolUseID: "call_summary", Name: "Read", InputJSON: `{"file_path":"/x"}`,
				ResultContent: "summary-only outcome", ResultContentSize: len("summary-only outcome"),
			}}},
	)
	session, err = ConvertToOpenCodeSession(summaryOnly, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the summary-only shape: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	var part openCodePart
	for _, message := range document.Messages {
		for _, candidate := range message.Content {
			if candidate.Type == openCodePartTool && candidate.ID == "call_summary" {
				part = candidate
			}
		}
	}
	if part.State == nil || part.State.Status != openCodeToolCompleted || len(part.State.Content) != 1 ||
		part.State.Content[0].Text != "summary-only outcome" {
		t.Errorf("call_summary part = %+v, want the archive summary under `completed`: the read recorded "+
			"no result event for it", part)
	}
	openCodeRequireLoss(t, session, "nonterminal_tool_events", 0)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
}

// TestOpenCodeOmitsAStatusWordWithNoTruthfulState pins the outcome vocabulary's third arm: the harness's
// own states are `completed` and `error`, and a result whose recorded status word means neither -- the
// `cancelled` outcome the inherited Vibe reader preserves verbatim (internal/parser/vibe_unified.go,
// whose own case pins its `"Stop requested"` text) -- must not be presented as either. The call is left
// out and the result counted rather than written under `completed`, which would state a success the
// source did not record, or under `error`, which would state a failure; and the archive's summary of the
// same event cannot restore it, because the pairing consumes the recorded event and the fallback never
// fires for a call that has one.
func TestOpenCodeOmitsAStatusWordWithNoTruthfulState(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	cancelled := "Stop requested"
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "running the suite", DialogueProjected: true,
			Content: "running the suite", Timestamp: at,
			ToolCalls: []ToolCall{
				openCodeTestCall("call-ok", "Read", `{"file_path":"/x"}`),
				openCodeTestCall("call-cancelled", "Bash", `{"command":"sleep 60"}`),
			}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call-ok", "completed", `"done"`),
			openCodeTestResult("call-cancelled", "cancelled", cancelled),
		}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(session.Data), cancelled) {
		t.Error("a cancelled result's text was written into the document: neither tool state names that " +
			"outcome, and the words must not travel under one of them")
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	parts := map[string]openCodePart{}
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				parts[part.ID] = part
			}
		}
	}
	if part, ok := parts["call-cancelled"]; ok {
		t.Errorf("call-cancelled part = %+v, want no part: its recorded status word has no truthful state",
			part)
	}
	if ok := parts["call-ok"]; ok.State == nil || ok.State.Status != openCodeToolCompleted ||
		len(ok.State.Content) != 1 || ok.State.Content[0].Text != "done" {
		t.Errorf("call-ok part = %+v, want the completed part beside it unaffected", ok)
	}
	openCodeRequireLoss(t, session, "tool_result_statuses", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
	openCodeRequireLoss(t, session, "empty_tool_results", 0)
	if session.Carried.ToolCalls != 1 || session.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want only the call whose part was written", session.Carried)
	}

	// The archive shape of the same event: the summary column holds the cancelled event's own text (the
	// inherited write path restores it from that event), and it must not put the outcome back.
	archive := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "running the suite", DialogueProjected: true,
			Content: "running the suite", Timestamp: at,
			ToolCalls: []ToolCall{{
				ToolUseID: "call-cancelled", Name: "Bash", InputJSON: `{"command":"sleep 60"}`,
				ResultContent: cancelled, ResultContentSize: len(cancelled),
			}},
			ToolResults: []ToolResult{{
				ToolUseID: "call-cancelled", Status: "cancelled", ContentRaw: cancelled,
				ContentSize: len(cancelled), ContentDecoded: true,
			}}},
	)
	session, err = ConvertToOpenCodeSession(archive, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the archive shape: %v", err)
	}
	if strings.Contains(string(session.Data), cancelled) {
		t.Error("the archive's summary restored a cancelled event as the call's outcome")
	}
	openCodeRequireLoss(t, session, "tool_result_statuses", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
	openCodeRequireNoToolPart(t, session)
}

// TestOpenCodePreservesTheRecordedWorkingDirectory pins the directory's own bytes: the read answers the
// source's recorded path, and a path that ends (or begins) with a space is a different directory from its
// trimmed form -- the document's `location.directory`, the importer's `--directory` argument and the
// printed resume command all take the recorded path as it was recorded. Only an empty value is refused,
// and only against its trimmed form, because a directory that holds nothing but whitespace names no
// path the importer could record.
func TestOpenCodePreservesTheRecordedWorkingDirectory(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "hello", DialogueProjected: true, Content: "hello", Timestamp: at},
	)
	conversation.Source.Cwd = "/tmp/ccc opencode target "
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if session.Cwd != conversation.Source.Cwd {
		t.Errorf("session cwd = %q, want the recorded path %q verbatim", session.Cwd, conversation.Source.Cwd)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if document.Info.Location.Directory != conversation.Source.Cwd {
		t.Errorf("info.location.directory = %q, want the recorded path %q verbatim",
			document.Info.Location.Directory, conversation.Source.Cwd)
	}
	conversation.Source.Cwd = " \t "
	if _, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions()); err == nil {
		t.Fatal("a conversation whose working directory holds nothing but whitespace was converted")
	} else if !strings.Contains(err.Error(), ErrNoTargetCwd.Error()) {
		t.Errorf("error = %v, want the refusal to name ErrNoTargetCwd", err)
	}
}

// TestOpenCodeParallelToolSpansKeepTheirOwnInstants pins every call's recorded pair: calls recorded on one
// message started together, and one call's result arriving later must not move a sibling's recorded start
// up to that completion. Both instants used to be taken through the shared clock one after the other, so
// the first completion to arrive rewrote every later call's `created` to itself and the durations the
// source recorded disappeared from the store. The message's own instant is now the floor under each pair
// (so an unordered source still cannot write a part whose end precedes its start), and a completion only
// raises the clock's floor for the records that follow it.
func TestOpenCodeParallelToolSpansKeepTheirOwnInstants(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "two calls", DialogueProjected: true, Content: "two calls",
			Timestamp: at,
			ToolCalls: []ToolCall{
				openCodeTestCall("call-a", "Read", `{"file_path":"/a"}`),
				openCodeTestCall("call-b", "Read", `{"file_path":"/b"}`),
			}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call-a", "completed", `"a"`),
		}},
		Message{Role: "user", Timestamp: at.Add(2 * time.Second), ToolResults: []ToolResult{
			openCodeTestResult("call-b", "completed", `"b"`),
		}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	spans := map[string]*openCodeTimeSpan{}
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				spans[part.ID] = part.Time
			}
		}
	}
	want := map[string]openCodeTimeSpan{
		"call-a": {Created: at.UnixMilli(), Completed: at.Add(time.Second).UnixMilli()},
		"call-b": {Created: at.UnixMilli(), Completed: at.Add(2 * time.Second).UnixMilli()},
	}
	for id, expected := range want {
		span := spans[id]
		if span == nil || *span != expected {
			t.Errorf("%s time = %+v, want the records' own pair %+v: a sibling's completion must not "+
				"move a call's recorded start", id, span, expected)
		}
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
	openCodeRequireLoss(t, session, "repeated_tool_results", 0)

	// The one adjustment the pair may take: a source whose records are out of order -- here the call's
	// record carries the later instant and the result's record the earlier one, while the read's own order
	// is the one the document keeps -- cannot write a part whose end precedes its start. The part's whole
	// span is then the message's own instant, which the comment on the clock's own span and the plan's
	// timestamps row disclose; nothing else about the recorded pair is rewritten.
	unordered := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "later call", DialogueProjected: true, Content: "later call",
			Timestamp: at.Add(time.Second),
			ToolCalls: []ToolCall{openCodeTestCall("call-late", "Read", `{"file_path":"/c"}`)}},
		Message{Role: "user", Timestamp: at, ToolResults: []ToolResult{
			openCodeTestResult("call-late", "completed", `"c"`),
		}},
	)
	session, err = ConvertToOpenCodeSession(unordered, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting an unordered source: %v", err)
	}
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	span := (*openCodeTimeSpan)(nil)
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool && part.ID == "call-late" {
				span = part.Time
			}
		}
	}
	if span == nil || span.Created != at.Add(time.Second).UnixMilli() ||
		span.Completed != at.Add(time.Second).UnixMilli() {
		t.Errorf("call-late time = %+v, want the message's own instant %d twice: the result's record "+
			"precedes the call's, so the part may not end before it starts",
			span, at.Add(time.Second).UnixMilli())
	}
}

// TestOpenCodePlansNoEvidenceFromARecordItWillNotWrite pins the two passes' own consistency: the plan
// pairs every result with its call before the walk decides which records travel, so a record the source's
// own relations place off the conversation's line -- or a subagent's own record -- must contribute no
// outcome to an active call. A result the walk omits is omitted; pairing it anyway revives
// abandoned-branch evidence as the active call's own answer while the report counts the record as not
// carried.
func TestOpenCodePlansNoEvidenceFromARecordItWillNotWrite(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	call := openCodeTestCall("call-1", "Read", `{"file_path":"internal/retry/policy.go"}`)
	result := openCodeTestResult("call-1", "completed", `"the recorded output"`)
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "first", DialogueProjected: true, Content: "first",
			SourceUUID: "rec-1", Timestamp: at},
		Message{Role: "assistant", Dialogue: "calling", DialogueProjected: true, Content: "calling",
			SourceUUID: "rec-2", SourceParentUUID: "rec-1", Timestamp: at.Add(time.Second),
			ToolCalls: []ToolCall{call}},
		// The result's record is the call's own child, and so is the final record: the source's relations
		// make the two siblings, and the walk follows the final record's line past the result.
		Message{Role: "user", SourceUUID: "rec-3", SourceParentUUID: "rec-2",
			Timestamp: at.Add(2 * time.Second), ToolResults: []ToolResult{result}},
		Message{Role: "assistant", Dialogue: "final", DialogueProjected: true, Content: "final",
			SourceUUID: "rec-4", SourceParentUUID: "rec-2", Timestamp: at.Add(3 * time.Second)},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	openCodeRequireLoss(t, session, "inactive_branch_records", 1)
	// The active call has no recorded outcome on the line the walk follows: its part is left out and
	// named, and the omitted record's result is not written in the state that means "this call answered".
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 0)
	openCodeRequireNoToolPart(t, session)

	// A subagent's own record is not written either: a result recorded on it is not the active call's
	// outcome, whatever the call's own record is.
	subagent := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "calling", DialogueProjected: true, Content: "calling",
			Timestamp: at, ToolCalls: []ToolCall{call}},
		Message{Role: "user", IsSidechain: true, Timestamp: at.Add(time.Second),
			ToolResults: []ToolResult{result}},
	)
	session, err = ConvertToOpenCodeSession(subagent, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the subagent's record: %v", err)
	}
	openCodeRequireLoss(t, session, "sidechain_messages", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireNoToolPart(t, session)
}

// openCodeRequireNoToolPart fails when one document message holds a tool part, for the cases whose rule
// is that no evidence may have travelled from a record the walk does not write.
func openCodeRequireNoToolPart(t *testing.T, session OpenCodeSession) {
	t.Helper()
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				t.Errorf("the document holds a tool part for call %q: its evidence came from a record the "+
					"walk does not write", part.ID)
			}
		}
	}
}

// TestOpenCodeCompactionSummaryMemberIsPresentWhenEmpty pins the one member the importer requires on a
// compaction message and accepts empty in it: a boundary whose summary holds no text is written with
// `"summary": ""` (the inherited Pi reader can hand over exactly that shape), and the member stays absent
// on every other message type, which is the shape the harness's own export prints.
func TestOpenCodeCompactionSummaryMemberIsPresentWhenEmpty(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "hello", DialogueProjected: true, Content: "hello", Timestamp: at},
		Message{Role: "assistant", IsSystem: true, IsCompactBoundary: true, Content: "", Timestamp: at},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	openCodeRequireLoss(t, session, "empty_compaction_summary", 1)
	var document struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	compactions := 0
	for index, message := range document.Messages {
		var kind string
		if err := json.Unmarshal(message["type"], &kind); err != nil {
			t.Fatalf("message %d holds no readable type: %v", index, err)
		}
		summary, present := message["summary"]
		switch kind {
		case openCodeMessageCompaction:
			compactions++
			if !present {
				t.Errorf("the compaction message holds no summary member: the importer requires it")
			} else if string(summary) != `""` {
				t.Errorf("the compaction message's summary = %s, want the source's own empty summary "+
					"written as an empty string", summary)
			}
		default:
			if present {
				t.Errorf("a %s message holds a summary member (%s), want the member absent on every "+
					"message type that does not use it", kind, summary)
			}
		}
	}
	if compactions != 1 {
		t.Errorf("the document holds %d compaction message(s), want the one boundary this case converts",
			compactions)
	}
}

// TestOpenCodeWriterNamesWhatItCannotCarry pins the categories whose rules only this target has, and the
// one refusal the writer makes: a conversation with no recorded working directory cannot be installed
// where the harness would read it, because the importer records the directory it is given and refuses one
// that does not exist.
func TestOpenCodeWriterNamesWhatItCannotCarry(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "hello", DialogueProjected: true, Content: "hello", Timestamp: at},
		Message{Role: "assistant", Dialogue: "   ", DialogueProjected: true, Content: "   ", Timestamp: at},
		// The reasoning-only shape a rendering reader produces (RooCode's, Kilo's, Cline's): the trace in
		// a `[Thinking]…[/Thinking]` block with nothing else, no dialogue projection. Its words are not
		// dialogue, so the writer carries the trace once -- as the reasoning part -- and this case's own
		// count below is what the record contributes either way.
		Message{Role: "assistant", Thinking: "only a trace",
			Content: "[Thinking]\nonly a trace\n[/Thinking]", Timestamp: at},
		Message{Role: "tool", Content: "tool output", Timestamp: at},
		Message{Role: "assistant", IsSidechain: true, Dialogue: "subagent", DialogueProjected: true,
			Content: "subagent", Timestamp: at},
		Message{Role: "user", DialogueProjected: true, Content: "rendered something else", Timestamp: at},
		Message{Role: "user", Content: "", Timestamp: at},
		Message{Role: "assistant", Content: "", Timestamp: at},
		Message{Role: "assistant", IsSystem: true, Content: "a reminder", Timestamp: at},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	openCodeRequireLoss(t, session, "whitespace_only_dialogue", 1)
	openCodeRequireLoss(t, session, "reasoning_text", 1)
	openCodeRequireLoss(t, session, "tool_role_messages", 1)
	openCodeRequireLoss(t, session, "sidechain_messages", 1)
	openCodeRequireLoss(t, session, "non_dialogue_content", 1)
	openCodeRequireLoss(t, session, "empty_user_messages", 1)
	openCodeRequireLoss(t, session, "empty_assistant_messages", 1)
	openCodeRequireLoss(t, session, "system_records", 1)
	openCodeRequireLoss(t, session, "usage_zeroed", 1)
	openCodeRequireLoss(t, session, "compaction_reason", 0)

	// A compaction boundary is carried with the reason the harness's own vocabulary requires, counted.
	conversation = openCodeTestConversation(
		Message{Role: "assistant", IsSystem: true, IsCompactBoundary: true, Content: "Summary: x", Timestamp: at},
	)
	session, err = ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting a boundary: %v", err)
	}
	openCodeRequireLoss(t, session, "compaction_reason", 1)
	openCodeRequireLoss(t, session, "empty_compaction_summary", 0)

	// The directory rule: a source that recorded no working directory is refused before anything is
	// written.
	conversation.Source.Cwd = ""
	if _, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions()); err == nil {
		t.Fatal("a conversation with no recorded working directory was converted")
	} else if !strings.Contains(err.Error(), ErrNoTargetCwd.Error()) {
		t.Errorf("error = %v, want the refusal to name ErrNoTargetCwd", err)
	}
}

// TestOpenCodeWriterNeverLetsABindingGoUnnamed pins the attribution rules' own words: the session's own
// model binding comes from the harness's answer (and is absent, with its detail, when it was not asked),
// the provider on every record is the target home's own answer where it was asked, the agent every record
// names is that home's own too, and the model id is left empty -- never invented -- where neither the read
// nor the harness answered one.
func TestOpenCodeWriterNeverLetsABindingGoUnnamed(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "hello", DialogueProjected: true, Content: "hello",
			Model: "source-model", Timestamp: at},
	)
	unasked := openCodeTargetTestOptions()
	unasked.Provider = ""
	unasked.Model = ""
	unasked.ModelVariant = ""
	unasked.Agent = ""
	unasked.ProviderDetail = "a dry run does not start the target harness against its data directory"
	session, err := ConvertToOpenCodeSession(conversation, unasked)
	if err != nil {
		t.Fatalf("converting without the harness's answer: %v", err)
	}
	var document openCodeDocument
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if document.Info.Model != nil {
		t.Errorf("info.model = %+v, want it absent: the harness was not asked", document.Info.Model)
	}
	message := document.Messages[0]
	if message.Agent != openCodeDefaultAgent {
		t.Errorf("agent = %q, want the harness's built-in default when it could not be asked", message.Agent)
	}
	if message.Model == nil || message.Model.ID != "source-model" || message.Model.ProviderID != "" {
		t.Errorf("model = %+v, want the source's model id and no provider claimed", message.Model)
	}
	openCodeRequireLoss(t, session, "target_agent", 1)
	// The category is reported with no binding written: openCodeRequireLoss fails when the category is
	// missing, so this zero is the report naming it rather than a row that is absent.
	openCodeRequireLoss(t, session, "target_model_binding", 0)

	// The read answered no model and the harness was not asked either: the member is still written, with
	// the empty id, and the report's own detail says so rather than naming a model this run cannot
	// support. Measured on 2.2.5: the member is required and its values need not be -- an empty id is
	// imported, stored and exported as written, and a document whose assistant message carries no model
	// member at all is refused.
	unnamed := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "no model recorded", DialogueProjected: true,
			Content: "no model recorded", Timestamp: at},
	)
	session, err = ConvertToOpenCodeSession(unnamed, unasked)
	if err != nil {
		t.Fatalf("converting with neither model answer: %v", err)
	}
	if err := json.Unmarshal(session.Data, &document); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	message = document.Messages[0]
	if message.Model == nil || message.Model.ID != "" || message.Model.ProviderID != "" {
		t.Errorf("model = %+v, want the member written with the empty id: neither the read nor the "+
			"harness answered a model", message.Model)
	}
	openCodeRequireLoss(t, session, "target_model_id_attribution", 1)
	if detail := openCodeLossDetail(t, session, "target_model_id_attribution"); !strings.Contains(detail, "empty") {
		t.Errorf("target_model_id_attribution detail = %q, want the empty id it wrote named", detail)
	}
}
