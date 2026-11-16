package ccc

import (
	"encoding/json"
	"sort"
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

// TestCodexRolloutDoesNotApplyASidechainCompactionBoundary pins the exclusion order: the inherited
// Claude parser keeps both flags on one record (its own TestParseClaudeSession_CompactBoundary is that
// shape), and such a record is a subagent's own summary, not a boundary of the main conversation.
// Applying it as the target's compacted record would replace the main conversation's model-visible
// history with a subagent's summary, and would leave the record out of the sidechain count while doing
// it -- so the exclusion is checked before the boundary is written.
func TestCodexRolloutDoesNotApplyASidechainCompactionBoundary(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Main question", Dialogue: "Main question"},
		// The combined shape: a subagent's record that also carries the source's compaction marker.
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "SUBAGENT-SUMMARY",
			IsSidechain: true, IsCompactBoundary: true, IsSystem: true},
		Message{Ordinal: 2, Role: string(parser.RoleUser), Content: "Follow-up", Dialogue: "Follow-up"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	for _, line := range decodeRollout(t, rollout.Data) {
		if line.Type == codexRecordCompacted {
			t.Fatalf("the conversion wrote the target's compacted record for a subagent's summary: %s",
				line.Payload)
		}
	}
	if strings.Contains(string(rollout.Data), "SUBAGENT-SUMMARY") {
		t.Error("the rollout holds a subagent summary the report says is omitted")
	}
	if rollout.Carried.CompactionBoundaries != 0 {
		t.Errorf("boundaries carried = %d, want none: the summary belongs to a subagent",
			rollout.Carried.CompactionBoundaries)
	}
	if loss := lossCount(t, rollout.Losses, "sidechain_messages"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("sidechain_messages = %+v, want the combined record counted as a subagent's", loss)
	}
	if loss := lossCount(t, rollout.Losses, "pre_compaction_history"); loss.Count != 0 {
		t.Errorf("pre_compaction_history = %+v, want no boundary applied to the main conversation", loss)
	}
	if rollout.Carried.Messages != 2 {
		t.Errorf("messages carried = %d, want the two main-thread messages",
			rollout.Carried.Messages)
	}
}

// TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText pins the split the writer makes for a
// record the provider flagged as system-injected: the record's own text is not the conversation, while a
// tool result recorded on that same record is the recorded output of a call this conversion carried --
// so suppressing the text must not suppress the output, and the output must not travel twice. The shape
// is the provider's own rather than a hand-made one: the inherited Claude parser keeps a promoted system
// reminder's tool results on the message it marks IsSystem
// (internal/parser's TestParseClaudeSession_SystemReminderPromotionPreservesToolResults), and the
// committed tooling fixture holds it -- TestToolingFixtureCarriesASystemRemindersToolResult is the same
// rule read through that fixture's real parse.
func TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Run it", Dialogue: "Run it"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Bash: ls]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Bash", InputJSON: `{"command":"ls"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), IsSystem: true,
			Content:     "<system-reminder>remember the workspace state</system-reminder>",
			ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: "the tool's output"}}},
		Message{Ordinal: 3, Role: string(parser.RoleUser), IsSystem: true,
			Content: "<system-reminder>a notice with nothing to carry</system-reminder>"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	data := string(rollout.Data)
	for _, absent := range []string{"remember the workspace state", "a notice with nothing to carry"} {
		if strings.Contains(data, absent) {
			t.Errorf("the rollout holds %q, the text of a record the provider flagged as system-injected", absent)
		}
	}
	if got := strings.Count(data, "the tool's output"); got != 1 {
		t.Errorf("the recorded output appears %d time(s) in the rollout, want exactly once, paired with "+
			"its call", got)
	}
	outputs := 0
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") != codexItemFunctionOutput ||
			payloadField(t, line, "call_id") != "toolu_1" {
			continue
		}
		outputs++
		if got := payloadField(t, line, "output"); got != "the tool's output" {
			t.Errorf("the paired output = %q, want the recorded result text", got)
		}
	}
	if outputs != 1 {
		t.Errorf("function_call_output records for toolu_1 = %d, want exactly one", outputs)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 || rollout.Carried.Messages != 1 {
		t.Errorf("carried = %+v, want the call, its output once and the one dialogue message", rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "system_records"); loss.Count != 2 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("system_records = %+v, want both records' own text counted as omitted", loss)
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
		// The shape the inherited Omnigent reader keeps for an orphan function_call_output: a record
		// with the native tool role, the tool's own words as its content, and a result whose call this
		// conversation does not hold.
		Message{Ordinal: 10, Role: string(parser.RoleTool), Content: "the tool's own output",
			ToolResults: []ToolResult{{ToolUseID: "toolu_gone", ContentRaw: "the tool's own output"}}},
		Message{Ordinal: 11, Role: "unrecognised", Content: "a role this build does not classify"},
		Message{Ordinal: 12, Role: string(parser.RoleUser)},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	for category, want := range map[string]struct {
		count       int
		disposition string
	}{
		"reasoning_text":                {2, DispositionOmitted},
		"sidechain_messages":            {1, DispositionOmitted},
		"system_records":                {1, DispositionOmitted},
		"tool_role_messages":            {1, DispositionOmitted},
		"unclassified_role_messages":    {1, DispositionOmitted},
		"empty_user_messages":           {1, DispositionOmitted},
		"whitespace_only_dialogue":      {0, DispositionOmitted},
		"non_dialogue_content":          {0, DispositionOmitted},
		"unpaired_tool_results":         {2, DispositionOmitted},
		"empty_tool_results":            {0, DispositionOmitted},
		"whitespace_only_tool_results":  {0, DispositionOmitted},
		"unrepresentable_result_blocks": {0, DispositionOmitted},
		"unresolved_tool_outputs":       {1, DispositionCarriedWithCaveat},
		"tool_result_outcomes":          {1, DispositionCarriedWithCaveat},
		"empty_tool_inputs":             {1, DispositionCarriedWithCaveat},
		"generated_call_ids":            {0, DispositionCarriedWithCaveat},
		"tool_name_vocabulary":          {2, DispositionCarriedWithCaveat},
		"provider_binding":              {0, DispositionCarriedWithCaveat},
		"pre_compaction_history":        {0, DispositionCarriedWithCaveat},
		"malformed_source_lines":        {0, DispositionOmitted},
		"summary_transcript":            {0, DispositionCarriedWithCaveat},
		"source_needs_retry":            {0, DispositionCarriedWithCaveat},
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
	for _, absent := range []string{
		"Private reasoning", "Subagent said this", "System notice", "no call for this",
		// The tool role is not a dialogue role, and its result here has no call to pair with: the words
		// belong to neither the source's agent nor its user, so the file must hold them nowhere.
		"the tool's own output", "a role this build does not classify",
	} {
		if strings.Contains(data, absent) {
			t.Errorf("the rollout holds %q, which the report says was omitted", absent)
		}
	}
}

// TestCodexRolloutDoesNotReportOmittedResultsAsCarried pins the counting rule of the two carried-with-
// caveat result categories: a result this conversion does not write -- its call is not in the
// conversation, or it holds no text to write -- carries no output at all, so it is counted as the
// omission it is and never as evidence that travelled with a caveat. The same two flags on results that
// are written are counted, which is what makes the zeros above a statement about the omission rather
// than about the flags being ignored.
func TestCodexRolloutDoesNotReportOmittedResultsAsCarried(t *testing.T) {
	omitted := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		// A result whose call this conversation does not hold, carrying both flags.
		Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{
			{ToolUseID: "toolu_gone", Status: "errored", ContentRaw: "the orphan's recorded text",
				PersistedOutputUnresolved: true},
		}},
		// A result paired with a call that is carried, but with no text to write, carrying both flags.
		Message{Ordinal: 3, Role: string(parser.RoleUser), ToolResults: []ToolResult{
			{ToolUseID: "toolu_1", Status: "errored", PersistedOutputUnresolved: true},
		}},
	)
	rollout, err := ConvertToCodexRollout(omitted, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Fatalf("carried = %+v, want no result written", rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "unpaired_tool_results"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("unpaired_tool_results = %+v, want the orphan counted as omitted", loss)
	}
	if loss := lossCount(t, rollout.Losses, "empty_tool_results"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("empty_tool_results = %+v, want the textless result counted as omitted", loss)
	}
	for _, category := range []string{"unresolved_tool_outputs", "tool_result_outcomes"} {
		loss := lossCount(t, rollout.Losses, category)
		if loss.Count != 0 || loss.Disposition != DispositionCarriedWithCaveat {
			t.Errorf("%s = %+v, want 0 and %q: a result that was not written carried nothing",
				category, loss, DispositionCarriedWithCaveat)
		}
	}
	if strings.Contains(string(rollout.Data), "the orphan's recorded text") {
		t.Error("the rollout holds the orphan's text, which the report says was not carried")
	}

	written := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{
				{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`},
				{ToolUseID: "toolu_2", Name: "Read", InputJSON: `{"file_path":"/y"}`},
			}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{
			{ToolUseID: "toolu_1", Status: "errored", ContentRaw: `"Error: missing"`},
			{ToolUseID: "toolu_2", ContentRaw: "placeholder for a side file",
				PersistedOutputUnresolved: true},
		}},
	)
	rollout, err = ConvertToCodexRollout(written, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 2 {
		t.Fatalf("carried = %+v, want both results written", rollout.Carried)
	}
	for category, want := range map[string]int{"unresolved_tool_outputs": 1, "tool_result_outcomes": 1} {
		if loss := lossCount(t, rollout.Losses, category); loss.Count != want {
			t.Errorf("%s = %+v, want %d: the flag is on a result whose output was written",
				category, loss, want)
		}
	}
}

// TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult pins the role rule for the native tool
// role: a tool-role message is the tool's own output, so its text is never written as a dialogue
// message. A paired output travels as the target's own function_call_output and nowhere else -- not
// twice, and never as an assistant turn the source never recorded -- and an output whose call this
// conversion did not carry travels nowhere, named by its own category and by the unpaired-result one.
func TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult(t *testing.T) {
	// The shape the inherited Devin reader keeps for a tool node: the role is tool, the dialogue
	// projection is empty, and the tool's output is both the record's content and its result's content.
	paired := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleTool), Content: "the tool's own output",
			ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: "the tool's own output"}}},
	)
	rollout, err := ConvertToCodexRollout(paired, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 1 || rollout.Carried.Messages != 1 || rollout.Carried.AssistantMessages != 0 {
		t.Errorf("carried = %+v, want the output carried once and the tool record counted as no message",
			rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "tool_role_messages"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("tool_role_messages = %+v, want the tool record omitted as dialogue", loss)
	}
	data := string(rollout.Data)
	if got := strings.Count(data, "the tool's own output"); got != 1 {
		t.Errorf("the tool's own output appears %d time(s) in the rollout, want once, in its function_call_output", got)
	}
	for _, line := range decodeRollout(t, rollout.Data) {
		if !strings.Contains(string(line.Payload), "the tool's own output") {
			continue
		}
		if payloadField(t, line, "type") != codexItemFunctionOutput {
			t.Errorf("a %s record carries the tool's own words outside its function_call_output: %s",
				line.Type, line.Payload)
		}
	}

	// The orphan shape: the call this output belongs to is not in the conversation, so there is no pair
	// to carry it with -- and the record's text must not be written as a message either.
	orphan := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleTool), Content: "ORPHAN-TOOL-OUTPUT",
			ToolResults: []ToolResult{{ToolUseID: "toolu_missing", ContentRaw: "ORPHAN-TOOL-OUTPUT"}}},
	)
	rollout, err = ConvertToCodexRollout(orphan, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(rollout.Data), "ORPHAN-TOOL-OUTPUT") {
		t.Error("the rollout holds a tool record's words although neither its message nor its result was carried")
	}
	if loss := lossCount(t, rollout.Losses, "unpaired_tool_results"); loss.Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the orphan result counted", loss)
	}
	if loss := lossCount(t, rollout.Losses, "tool_role_messages"); loss.Count != 1 {
		t.Errorf("tool_role_messages = %+v, want the orphan tool record counted", loss)
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

	// Three outstanding calls: the pairing rule has to survive another call arriving, so the ambiguity
	// is a property of the calls that have no result yet and not of the last call written.
	threeCalls := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`},
				{Name: "Read", InputJSON: `{"file_path":"/y"}`},
				{Name: "Read", InputJSON: `{"file_path":"/z"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of one of the three"}}},
	)
	rollout, err = ConvertToCodexRollout(threeCalls, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Errorf("carried %d result(s) for three id-less calls: the result could belong to any of them",
			rollout.Carried.ToolResults)
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the ambiguous result counted",
			lossCount(t, rollout.Losses, "unpaired_tool_results"))
	}

	// A result that names its own call answers that one and leaves the call that is still outstanding
	// as the only candidate for an id-less result after it: the source recorded one unanswered call.
	afterExplicit := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Read", InputJSON: `{"file_path":"/x"}`},
				{ToolUseID: "toolu_b", Name: "Read", InputJSON: `{"file_path":"/y"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_a", ContentRaw: "content of /x"}}},
		Message{Ordinal: 3, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of the one that is left"}}},
	)
	rollout, err = ConvertToCodexRollout(afterExplicit, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 2 {
		t.Fatalf("carried = %+v, want both results", rollout.Carried)
	}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") != codexItemFunctionOutput {
			continue
		}
		if output := payloadField(t, line, "output"); output == "content of the one that is left" &&
			payloadField(t, line, "call_id") != "toolu_b" {
			t.Errorf("the id-less result was paired with %q, want the only call still unanswered",
				payloadField(t, line, "call_id"))
		}
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 0 {
		t.Errorf("unpaired_tool_results = %+v, want none: one call was still outstanding",
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

// TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue pins the distinction the writer needs from
// the source. A provider that classifies its own dialogue projects an empty one for a message whose
// Content renders a reasoning trace, and such a message has no dialogue in the target: the trace is
// counted as reasoning_text, never written as the agent's own words beside a report that says it was
// omitted. A document whose source classified nothing keeps the Content fallback, which is what a
// caller building a document by hand relies on.
func TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask",
			DialogueProjected: true},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant),
			Content:           "[Thinking]\nPrivate reasoning\n[/Thinking]",
			DialogueProjected: true,
			Thinking:          "Private reasoning"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.Messages != 1 || rollout.Carried.AssistantMessages != 0 {
		t.Errorf("carried = %+v, want the one user message and no assistant message", rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "reasoning_text"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("reasoning_text = %+v, want the reasoning-only record counted as omitted", loss)
	}
	if strings.Contains(string(rollout.Data), "Private reasoning") {
		t.Error("the rollout holds the reasoning trace the report says is omitted, as the agent's dialogue")
	}

	// The same shape with the projection the source did not make: nothing classified the message, so its
	// Content is the message's text.
	unclassified := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask",
			DialogueProjected: true},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Spoken text"},
	)
	rollout, err = ConvertToCodexRollout(unclassified, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if !strings.Contains(string(rollout.Data), "Spoken text") {
		t.Error("a message whose source classified no projection lost its text")
	}

	// A record the source classified as carrying no dialogue but holding text, with no tool evidence:
	// the target has no message that would not state the user said those words, and the report names
	// that category rather than calling the record empty.
	nonDialogue := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "the tool's own output",
			DialogueProjected: true},
	)
	rollout, err = ConvertToCodexRollout(nonDialogue, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(rollout.Data), "the tool's own output") {
		t.Error("a record the source classified as not dialogue was written as the user's own words")
	}
	if loss := lossCount(t, rollout.Losses, "non_dialogue_content"); loss.Count != 1 {
		t.Errorf("non_dialogue_content = %+v, want the record counted", loss)
	}
	if loss := lossCount(t, rollout.Losses, "empty_user_messages"); loss.Count != 0 {
		t.Errorf("empty_user_messages = %+v, want a record whose text was dropped not called empty", loss)
	}
}

// TestCodexRolloutDecodesEncodedContentExactlyOnce pins which representation ContentRaw holds: the
// provider's own content field is JSON and is decoded once, while text a read has already decoded -- a
// result event, an archive row -- is used exactly as it stands. A tool whose own output is literally the
// JSON string `"alpha"` would otherwise lose its quotation marks, and a second decode of an
// already-decoded output is a difference the round-trip check would report as a defect the file does not
// have.
func TestCodexRolloutDecodesEncodedContentExactlyOnce(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Bash: x]",
			ToolCalls: []ToolCall{
				{ToolUseID: "toolu_1", Name: "Bash", InputJSON: `{"command":"x"}`},
				{ToolUseID: "toolu_2", Name: "Bash", InputJSON: `{"command":"y"}`},
			}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{
			// The provider's encoded content field: the JSON string's own text is the output.
			{ToolUseID: "toolu_1", ContentRaw: `"a quoted \"output\""`},
			// A result event: the read decoded this already, and the quotation marks are the tool's own.
			{ToolUseID: "toolu_2", ContentRaw: `"alpha"`, ContentDecoded: true},
		}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	outputs := map[string]string{}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionOutput {
			outputs[payloadField(t, line, "call_id")] = payloadField(t, line, "output")
		}
	}
	if outputs["toolu_1"] != `a quoted "output"` {
		t.Errorf("encoded content decoded as %q, want the provider's own string", outputs["toolu_1"])
	}
	if outputs["toolu_2"] != `"alpha"` {
		t.Errorf("already-decoded content written as %q, want it unchanged: a second decode strips the "+
			"quotation marks the tool really printed", outputs["toolu_2"])
	}
	if rollout.Carried.ToolResults != 2 {
		t.Errorf("carried = %+v, want both results", rollout.Carried)
	}
}

// TestCodexRolloutReportsToolResultBlocksTheTargetCannotCarry pins the block rule: the target's output
// field carries text, so a block the reader's text channel carries nothing from is counted -- and a
// content field holding only such blocks is not written as its own JSON, which would present an image
// payload as the tool's words.
func TestCodexRolloutReportsToolResultBlocksTheTargetCannotCarry(t *testing.T) {
	mixed := `[{"type":"text","text":"the text","cache_control":{"type":"ephemeral"}},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]`
	onlyImage := `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]`
	plain := `[{"type":"text","text":"just text","cache_control":{"type":"ephemeral"}}]`
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Bash: x]",
			ToolCalls: []ToolCall{
				{ToolUseID: "toolu_1", Name: "Bash", InputJSON: `{"command":"x"}`},
				{ToolUseID: "toolu_2", Name: "Bash", InputJSON: `{"command":"y"}`},
				{ToolUseID: "toolu_3", Name: "Bash", InputJSON: `{"command":"z"}`},
			}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{
			{ToolUseID: "toolu_1", ContentRaw: mixed},
			{ToolUseID: "toolu_2", ContentRaw: onlyImage},
			{ToolUseID: "toolu_3", ContentRaw: plain},
		}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	outputs := map[string]string{}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionOutput {
			outputs[payloadField(t, line, "call_id")] = payloadField(t, line, "output")
		}
	}
	if outputs["toolu_1"] != "the text" {
		t.Errorf("mixed content output = %q, want its text", outputs["toolu_1"])
	}
	if outputs["toolu_3"] != "just text" {
		t.Errorf("text block output = %q, want its text", outputs["toolu_3"])
	}
	if _, written := outputs["toolu_2"]; written {
		t.Errorf("a content field holding only an image block was written as %q", outputs["toolu_2"])
	}
	// Two blocks the target has no field for (one in each of two results), and one result this read
	// holds no text for. A block whose text was carried is not counted, cache marker and all.
	if loss := lossCount(t, rollout.Losses, "unrepresentable_result_blocks"); loss.Count != 2 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("unrepresentable_result_blocks = %+v, want the two image blocks counted", loss)
	}
	if loss := lossCount(t, rollout.Losses, "empty_tool_results"); loss.Count != 1 {
		t.Errorf("empty_tool_results = %+v, want the image-only result counted once", loss)
	}
	if strings.Contains(string(rollout.Data), "QUFB") {
		t.Error("the rollout holds the image payload's bytes, which the report says are not carried")
	}
}

// TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults pins the archive rule: the archive
// keeps a call's outcome as the result events it recorded on the call and holds their derived summary in
// the column beside them (internal/db restores the sole event's text into that column, or stores the
// aggregate it derives from several events). A summary is therefore a view of the recorded evidence, and
// a conversion writes it only when the call has no recorded result of its own -- the shape of an archive
// row whose events were not written -- where it is the only evidence of that outcome. Writing it beside
// its own events would present a display aggregate as a separately recorded tool output.
func TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults(t *testing.T) {
	// The archive shape: the outcome is folded onto the call's own message, so the events sit beside the
	// calls they answer (messageFromArchive), and the call's summary is the column the archive holds.
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[a batch of calls]",
			ToolCalls: []ToolCall{
				// One event, its own text restored into the column by the read: the same outcome.
				{ToolUseID: "toolu_1", Name: "Bash", InputJSON: `{"command":"ls"}`,
					ResultContent: "the recorded output", ResultContentSize: 19},
				// Two events and the aggregate the inherited write path derives from them, stored in
				// the column because it is not either event's text: still the same two outcomes.
				{ToolUseID: "toolu_2", Name: "Task", InputJSON: `{"prompt":"x"}`,
					ResultContent:     "agent-1:\nfirst\n\nagent-2:\nsecond",
					ResultContentSize: len("agent-1:\nfirst\n\nagent-2:\nsecond")},
				// A summary whose call carries no result at all: the only evidence of that outcome.
				{ToolUseID: "toolu_3", Name: "Bash", InputJSON: `{"command":"du"}`,
					ResultContent: "summary-only outcome", ResultContentSize: 20},
			},
			ToolResults: []ToolResult{
				{ToolUseID: "toolu_1", ContentRaw: "the recorded output", ContentDecoded: true},
				{ToolUseID: "toolu_2", ContentRaw: "first", ContentDecoded: true},
				{ToolUseID: "toolu_2", ContentRaw: "second", ContentDecoded: true},
			}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	outputs := []string{}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionOutput {
			outputs = append(outputs, payloadField(t, line, "call_id")+": "+payloadField(t, line, "output"))
		}
	}
	want := []string{
		"toolu_1: the recorded output",
		"toolu_2: first",
		"toolu_2: second",
		"toolu_3: summary-only outcome",
	}
	sort.Strings(outputs)
	sort.Strings(want)
	if strings.Join(outputs, " | ") != strings.Join(want, " | ") || rollout.Carried.ToolResults != 4 {
		t.Errorf("function_call_output records = %q (carried %d results), want the recorded results once "+
			"each and the summary only where no result was recorded: %q",
			outputs, rollout.Carried.ToolResults, want)
	}
}

// TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce pins the report's outermost omission: a
// line the provider's reader skipped never became a message, so no per-message category can count it,
// and a conversion that reported only its own per-message counts would let "converted" stand for a
// conversation the source file holds more of. The read's own count travels into the report, and an
// unfinished final record -- which the reader counts among those lines -- is named in the reason.
func TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce(t *testing.T) {
	partial := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
	)
	partial.Fidelity = Fidelity{ReadFrom: "source", MalformedLines: 2, Truncated: true}
	rollout, err := ConvertToCodexRollout(partial, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	loss := lossCount(t, rollout.Losses, "malformed_source_lines")
	if loss.Count != 2 || loss.Disposition != DispositionOmitted {
		t.Errorf("malformed_source_lines = %+v, want the read's own count as an omission", loss)
	}
	if !strings.Contains(loss.Detail, "truncated") {
		t.Errorf("the reason does not name the unfinished final record it counts: %q", loss.Detail)
	}

	// A read that produced a message for every line reports the same category with a zero count and a
	// reason that says so, which is what makes the count above readable as this run's answer.
	complete := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
	)
	rollout, err = ConvertToCodexRollout(complete, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "malformed_source_lines"); loss.Count != 0 ||
		!strings.Contains(loss.Detail, "readable") {
		t.Errorf("malformed_source_lines = %+v, want a zero count for a complete read", loss)
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
