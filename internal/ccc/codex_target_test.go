package ccc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.kenn.io/agentsview/internal/parser"
)

// rolloutLine is one decoded line of a written rollout, with its payload left raw so a test can assert
// on the exact fields the target reads.
type rolloutLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// decodeRollout decodes every line of a written rollout, failing when a line is not a JSON object: the
// target reads JSONL, so a line it cannot parse is a defect however plausible the writer's intent was.
func decodeRollout(t *testing.T, data []byte) []rolloutLine {
	t.Helper()
	text := strings.TrimSpace(string(data))
	if text == "" {
		t.Fatal("the rollout is empty")
	}
	var lines []rolloutLine
	for index, line := range strings.Split(text, "\n") {
		var decoded rolloutLine
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d is not a JSON object: %v", index+1, err)
		}
		if decoded.Type == "" || decoded.Timestamp == "" {
			t.Fatalf("line %d carries no type or timestamp: %s", index+1, line)
		}
		lines = append(lines, decoded)
	}
	return lines
}

// payloadField reads one field out of a line's payload.
func payloadField(t *testing.T, line rolloutLine, field string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(line.Payload, &payload); err != nil {
		t.Fatalf("decoding a %s payload: %v", line.Type, err)
	}
	value, _ := payload[field].(string)
	return value
}

// lossCount answers one category's count from a report, and fails when the category is missing: every
// category is reported on every run, including the ones whose count is zero, so a consumer never has to
// infer what was left out.
func lossCount(t *testing.T, losses []Loss, category string) Loss {
	t.Helper()
	for _, loss := range losses {
		if loss.Category == category {
			return loss
		}
	}
	t.Fatalf("the report does not name the %q category", category)
	return Loss{}
}

// testConversation builds a Conversation directly: the writer's rules are about the document it is
// given, so a test of them needs no provider and no source file.
func testConversation(messages ...Message) Conversation {
	return Conversation{
		Source: SourceIdentity{
			Provider:    "claude",
			SessionID:   "source-session",
			SourcePath:  "/source/session.jsonl",
			Cwd:         "/work/project",
			GitBranch:   "main",
			GitRemote:   "https://example.invalid/project.git",
			Fingerprint: "fingerprint",
		},
		Messages: messages,
		Fidelity: Fidelity{ReadFrom: "source"},
	}
}

func testOptions() CodexTargetOptions {
	return CodexTargetOptions{SessionID: "01a1e000-0000-7000-8000-000000000001"}
}

// TestCodexRolloutRecordShape pins the records, their order and their fields: the target reads this
// file, so every record's shape is a contract with it rather than a local preference.
func TestCodexRolloutRecordShape(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Check the retry policy.",
			Dialogue: "Check the retry policy.", Timestamp: time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "I will read it.",
			Dialogue: "I will read it.", Timestamp: time.Date(2026, 5, 5, 10, 0, 1, 0, time.UTC)},
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "[Read: /work/project/policy.go]",
			Timestamp: time.Date(2026, 5, 5, 10, 0, 2, 0, time.UTC),
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/work/project/policy.go"}`}}},
		Message{Ordinal: 3, Role: string(parser.RoleUser), Timestamp: time.Date(2026, 5, 5, 10, 0, 3, 0, time.UTC),
			ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: `"const MaxAttempts = 4\n"`, ContentSize: 22}}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.SessionID != testOptions().SessionID {
		t.Errorf("session id = %q, want the one the options named", rollout.SessionID)
	}
	if rollout.CLIVersion != CodexFormatVersion {
		t.Errorf("cli_version = %q, want the writer's verified format version %q", rollout.CLIVersion, CodexFormatVersion)
	}
	if rollout.FileName != "rollout-2026-05-05T10-00-00-"+rollout.SessionID+".jsonl" {
		t.Errorf("file name = %q", rollout.FileName)
	}
	if rollout.Directory != "2026/05/05" {
		t.Errorf("directory = %q", rollout.Directory)
	}

	lines := decodeRollout(t, rollout.Data)
	types := make([]string, 0, len(lines))
	for _, line := range lines {
		types = append(types, line.Type)
	}
	want := []string{
		codexRecordSessionMeta,
		codexRecordTurnContext,
		codexRecordResponseItem, codexRecordEventMsg, // the user turn and its view event
		codexRecordEventMsg, codexRecordResponseItem, // the assistant's, in the target's own order
		codexRecordResponseItem,
		codexRecordResponseItem,
	}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Errorf("record types = %v, want %v", types, want)
	}

	meta := lines[0]
	for field, value := range map[string]string{
		"id":            rollout.SessionID,
		"cwd":           "/work/project",
		"originator":    codexOriginator,
		"source":        codexSourceCLI,
		"thread_source": codexThreadSourceUser,
		"cli_version":   CodexFormatVersion,
	} {
		if got := payloadField(t, meta, field); got != value {
			t.Errorf("session_meta.%s = %q, want %q", field, got, value)
		}
	}
	var metaPayload map[string]any
	if err := json.Unmarshal(meta.Payload, &metaPayload); err != nil {
		t.Fatalf("decoding session_meta: %v", err)
	}
	git, _ := metaPayload["git"].(map[string]any)
	if git["branch"] != "main" || git["repository_url"] != "https://example.invalid/project.git" {
		t.Errorf("git provenance = %v, want the source's branch and remote", git)
	}
	if _, ok := git["commit_hash"]; ok {
		t.Error("the conversion wrote a commit hash the source document does not record")
	}
	if _, ok := metaPayload["model_provider"]; ok {
		t.Error("the conversion recorded a provider although no target configuration was given")
	}

	if call := payloadField(t, lines[6], "name"); call != "Read" {
		t.Errorf("tool call name = %q, want the source's own name", call)
	}
	if input := payloadField(t, lines[6], "arguments"); input != `{"file_path":"/work/project/policy.go"}` {
		t.Errorf("tool call arguments = %q, want the recorded input verbatim", input)
	}
	if output := payloadField(t, lines[7], "output"); output != "const MaxAttempts = 4\n" {
		t.Errorf("tool result output = %q, want the decoded recorded content", output)
	}
	if callID := payloadField(t, lines[7], "call_id"); callID != "toolu_1" {
		t.Errorf("tool result call_id = %q, want the call's own id", callID)
	}
}

// TestCodexRolloutKeepsTheDialogueProjection pins the text rule: the source reader's Content renders a
// message's tool calls into text, so a message that carries calls must contribute only what the provider
// recorded as dialogue, and a call-only message must contribute no text at all.
func TestCodexRolloutKeepsTheDialogueProjection(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant),
			Content: "[Read: /x]\nAnd now the answer.", Dialogue: "And now the answer.",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "[Bash: list files]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_2", Name: "Bash", InputJSON: `{"command":"ls"}`}}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var texts []string
	for _, item := range rollout.Items {
		if item.Kind == ItemUser || item.Kind == ItemAssistant {
			texts = append(texts, item.Text)
		}
	}
	if len(texts) != 2 || texts[0] != "Ask" || texts[1] != "And now the answer." {
		t.Errorf("dialogue items = %q, want the two recorded texts and no rendering of the calls", texts)
	}
	if rollout.Carried.ToolCalls != 2 {
		t.Errorf("tool calls carried = %d, want 2", rollout.Carried.ToolCalls)
	}
}

// TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt pins the decision the plan
// records: the boundary travels as the target's own compacted record, and the turns before it stay in
// the file (counted, with the caveat reported) rather than being dropped as a selection this goal has no
// mandate to make.
func TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Old question", Dialogue: "Old question"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Old answer", Dialogue: "Old answer"},
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "Summary of the old turns",
			IsSystem: true, IsCompactBoundary: true},
		Message{Ordinal: 3, Role: string(parser.RoleUser), Content: "New question", Dialogue: "New question"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodeRollout(t, rollout.Data)
	boundary := -1
	for index, line := range lines {
		if line.Type == codexRecordCompacted {
			boundary = index
			if summary := payloadField(t, line, "message"); summary != "Summary of the old turns" {
				t.Errorf("compacted message = %q", summary)
			}
		}
	}
	if boundary < 0 {
		t.Fatal("the conversion wrote no compaction boundary")
	}
	if rollout.Carried.CompactionBoundaries != 1 {
		t.Errorf("boundaries carried = %d, want 1", rollout.Carried.CompactionBoundaries)
	}
	if pre := lossCount(t, rollout.Losses, "pre_compaction_history"); pre.Count != 2 ||
		pre.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("pre_compaction_history = %+v, want 2 carried-with-caveat", pre)
	}
	if !strings.Contains(string(rollout.Data), "Old question") {
		t.Error("the turns before the boundary were dropped instead of carried")
	}
	if rollout.Carried.Messages != 3 {
		t.Errorf("messages carried = %d, want the three dialogue messages", rollout.Carried.Messages)
	}
}

// TestCodexRolloutReportsEveryCategoryItKnows pins the report's completeness: a category that is not
// named cannot be read, and a count that is not reported looks like evidence that travelled.
func TestCodexRolloutReportsEveryCategoryItKnows(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Thinking only",
			Thinking: "Private reasoning"},
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "Subagent said this",
			Dialogue: "Subagent said this", IsSidechain: true},
		Message{Ordinal: 3, Role: string(parser.RoleSystem), Content: "System notice"},
		Message{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "Call with reasoning",
			Dialogue: "Call with reasoning", Thinking: "More reasoning",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read"}}},
		Message{Ordinal: 5, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_orphan", ContentRaw: "no call for this"}}},
		Message{Ordinal: 6, Role: string(parser.RoleAssistant), Content: "Errored result next",
			Dialogue:  "Errored result next",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_2", Name: "Bash", InputJSON: `{"command":"ls"}`}}},
		Message{Ordinal: 7, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_2", Status: "errored", ContentRaw: `"Error: missing"`}}},
		Message{Ordinal: 8, Role: string(parser.RoleAssistant), Content: "Unresolved next",
			Dialogue:  "Unresolved next",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_3", Name: "Read", InputJSON: `{"file_path":"/y"}`}}},
		Message{Ordinal: 9, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_3", ContentRaw: "placeholder for a side file",
				PersistedOutputUnresolved: true}}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	for category, want := range map[string]struct {
		count       int
		disposition string
	}{
		"reasoning_text":          {2, DispositionOmitted},
		"sidechain_messages":      {1, DispositionOmitted},
		"system_records":          {1, DispositionOmitted},
		"unpaired_tool_results":   {1, DispositionOmitted},
		"empty_tool_results":      {0, DispositionOmitted},
		"unresolved_tool_outputs": {1, DispositionCarriedWithCaveat},
		"tool_result_outcomes":    {1, DispositionCarriedWithCaveat},
		"empty_tool_inputs":       {1, DispositionCarriedWithCaveat},
		"generated_call_ids":      {0, DispositionCarriedWithCaveat},
		"tool_name_vocabulary":    {2, DispositionCarriedWithCaveat},
		"provider_binding":        {0, DispositionCarriedWithCaveat},
		"pre_compaction_history":  {0, DispositionCarriedWithCaveat},
	} {
		loss := lossCount(t, rollout.Losses, category)
		if loss.Count != want.count || loss.Disposition != want.disposition {
			t.Errorf("%s = %+v, want count %d and disposition %q", category, loss, want.count, want.disposition)
		}
		if strings.TrimSpace(loss.Detail) == "" {
			t.Errorf("%s carries no detail, so a reader cannot tell what happened to it", category)
		}
	}
	if !strings.Contains(lossCount(t, rollout.Losses, "tool_name_vocabulary").Detail, "Read") {
		t.Error("the tool-name report does not name the tools it carried")
	}
	// The omitted evidence is not in the file: a report that says "omitted" and a file that holds it
	// would be two different answers to the same question.
	data := string(rollout.Data)
	for _, absent := range []string{"Private reasoning", "Subagent said this", "System notice", "no call for this"} {
		if strings.Contains(data, absent) {
			t.Errorf("the rollout holds %q, which the report says was omitted", absent)
		}
	}
}

// TestCodexRolloutPairsAnIdLessResultWithItsCall pins the pairing rule: the target pairs a call and its
// output by id, so a source that recorded none gets one generated from the message's own position — and
// two id-less calls make the pairing a guess, which is reported as an unpaired result instead.
func TestCodexRolloutPairsAnIdLessResultWithItsCall(t *testing.T) {
	pairable := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of /x"}}},
	)
	rollout, err := ConvertToCodexRollout(pairable, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 {
		t.Fatalf("carried = %+v, want one call and its result", rollout.Carried)
	}
	if lossCount(t, rollout.Losses, "generated_call_ids").Count != 2 {
		t.Errorf("generated_call_ids = %+v, want the call and the result counted",
			lossCount(t, rollout.Losses, "generated_call_ids"))
	}
	lines := decodeRollout(t, rollout.Data)
	var callID, resultID string
	for _, line := range lines {
		switch payloadField(t, line, "type") {
		case codexItemFunctionCall:
			callID = payloadField(t, line, "call_id")
		case codexItemFunctionOutput:
			resultID = payloadField(t, line, "call_id")
		}
	}
	if callID == "" || callID != resultID {
		t.Errorf("call id %q and result id %q must be the same generated id", callID, resultID)
	}

	ambiguous := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`},
				{Name: "Read", InputJSON: `{"file_path":"/y"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of one of them"}}},
	)
	rollout, err = ConvertToCodexRollout(ambiguous, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Errorf("carried %d result(s) for two id-less calls, which is a pairing the source did not record",
			rollout.Carried.ToolResults)
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the ambiguous result counted",
			lossCount(t, rollout.Losses, "unpaired_tool_results"))
	}
}

// TestCodexRolloutRecordsTheProviderBindingItIsGiven pins that the binding comes from the caller (which
// asks the target), is reported, and is absent when there is nothing to record.
func TestCodexRolloutRecordsTheProviderBindingItIsGiven(t *testing.T) {
	conversation := testConversation(Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"})
	options := testOptions()
	options.Provider = "target-provider"
	rollout, err := ConvertToCodexRollout(conversation, options)
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodeRollout(t, rollout.Data)
	var metaPayload map[string]any
	if err := json.Unmarshal(lines[0].Payload, &metaPayload); err != nil {
		t.Fatalf("decoding session_meta: %v", err)
	}
	if got, ok := metaPayload["model_provider"]; !ok || got != "target-provider" {
		t.Errorf("session_meta.model_provider = %v (present: %v), want the caller's binding", got, ok)
	}
	if binding := lossCount(t, rollout.Losses, "provider_binding"); binding.Count != 1 {
		t.Errorf("provider_binding = %+v, want the binding reported", binding)
	}
	// With no version pinned by the caller, the writer records the version whose reader it was verified
	// against: the target requires the field, so a silent empty value would produce a file it refuses.
	if cliVersion := payloadField(t, lines[0], "cli_version"); cliVersion != CodexFormatVersion {
		t.Errorf("cli_version = %q, want the writer's verified format version %q", cliVersion, CodexFormatVersion)
	}
}

// TestCodexRolloutGeneratesAFreshIdentity pins the two properties the goal asks for at the identity
// level: the target session's id is new and it is one the target itself reads as a session id (a UUID),
// and the same conversion run twice produces different ones.
func TestCodexRolloutGeneratesAFreshIdentity(t *testing.T) {
	conversation := testConversation(Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"})
	first, err := ConvertToCodexRollout(conversation, CodexTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	second, err := ConvertToCodexRollout(conversation, CodexTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if first.SessionID == second.SessionID {
		t.Fatalf("two conversions produced the same id %q", first.SessionID)
	}
	for _, rollout := range []CodexRollout{first, second} {
		parsed, err := uuid.Parse(rollout.SessionID)
		if err != nil {
			t.Fatalf("session id %q is not a UUID the target can name: %v", rollout.SessionID, err)
		}
		if parsed.Version() != 7 {
			t.Errorf("session id %q is a UUID v%d; the target orders its ids by time (v7)", rollout.SessionID, parsed.Version())
		}
	}
	if first.SessionID == conversation.Source.SessionID {
		t.Error("the target identity is the source's own session id")
	}
}

// TestCodexRolloutStampsRecordsInOneOrderedRun pins the envelope timestamps: a source whose messages are
// unordered (a DAG, a replayed fork) must not produce a file whose records run backwards, because the
// target reads the file's order.
func TestCodexRolloutStampsRecordsInOneOrderedRun(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Later", Dialogue: "Later",
			Timestamp: time.Date(2026, 5, 5, 10, 0, 10, 0, time.UTC)},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Earlier", Dialogue: "Earlier",
			Timestamp: time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodeRollout(t, rollout.Data)
	previous := time.Time{}
	for index, line := range lines {
		stamp, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			t.Fatalf("line %d timestamp %q: %v", index+1, line.Timestamp, err)
		}
		if stamp.Before(previous) {
			t.Errorf("line %d goes back in time: %s after %s", index+1, line.Timestamp, previous)
		}
		previous = stamp
	}
}
