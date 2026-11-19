package ccc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	// The spellings are literals on purpose: they are the carried reader's own record types
	// (`internal/parser/codex.go`'s codexTypeSessionMeta / codexTypeTurnContext /
	// codexTypeResponseItem / codexTypeEventMsg), so a writer whose spelling drifts fails here
	// instead of moving this assertion with it — the same class the acceptance audit named at :1113.
	want := []string{
		"session_meta",
		"turn_context",
		"response_item", "event_msg", // the user turn and its view event
		"event_msg", "response_item", // the assistant's, in the target's own order
		"response_item",
		"response_item",
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

// TestCodexRolloutReportsEveryCategoryItKnows pins the report's completeness as far as a list can: every
// category the writer reports today is named below, each with the count and disposition stated here and
// a non-empty detail, so a category that stopped being reported, or whose count moved, fails here
// instead of passing as a zero. The list is kept in step with the writer by hand: a category the writer
// gained and this case did not name would pass, and the pass that walks the report itself -- catching
// that direction -- is the Pi case's (TestPiSessionReportsEveryLossCategory).
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
		"unresolved_dialogue":           {0, DispositionOmitted},
		"unprojected_dialogue":          {1, DispositionCarriedWithCaveat},
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
		"tool_result_sources":           {0, DispositionOmitted},
		"result_completion_fallbacks":   {2, DispositionCarriedWithCaveat},
		"result_record_instants":        {0, DispositionOmitted},
		"substituted_record_instants":   {0, DispositionCarriedWithCaveat},
		"tool_name_vocabulary":          {2, DispositionCarriedWithCaveat},
		"provider_binding":              {0, DispositionCarriedWithCaveat},
		"inactive_branch_records":       {0, DispositionOmitted},
		"unresolved_source_parents":     {0, DispositionCarriedWithCaveat},
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

// TestCodexRolloutKeepsAnActiveResultAcrossAnInterveningBranch pins the same projection on the route
// whose target log has no parent relation: the inherited Pi reader answers a record under a tool result
// as hanging off the message that made the call, so a record the read answers that way may have hung off
// the result. When a branch's turn was written in between, the result still stands on the source's own
// line -- call → result → what follows -- so the result is written and only the branch's own turn is
// omitted as inactive.
func TestCodexRolloutKeepsAnActiveResultAcrossAnInterveningBranch(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "u1"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
			DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
				InputJSON: `{"pattern":"maxBackoff"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), SourceUUID: "r3", SourceParentUUID: "c2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_1",
				ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "left turn",
			Dialogue: "left turn", SourceUUID: "b4", SourceParentUUID: "u1"},
		Message{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "after the result",
			Dialogue: "after the result", SourceUUID: "d5", SourceParentUUID: "c2"},
	)
	// The source is a Pi session: its reader answers a record under a tool result as hanging off the
	// message that made the call, which is what makes this shape a projection rather than an exact link.
	conversation.Source.Provider = string(parser.AgentPi)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	loss := lossCount(t, rollout.Losses, "inactive_branch_records")
	if loss.Count != 1 || loss.Disposition != DispositionOmitted {
		t.Errorf("inactive_branch_records = %+v, want only the branch's own turn omitted: the result "+
			"stands on the source's own line", loss)
	}
	// The walk's answer about that place is not the read's: which of the call and its result the last
	// record hung off is what the projection leaves open, and the count names it. Without a category for
	// it the rollout would present the retained result as the source's own line by right.
	unresolved := lossCount(t, rollout.Losses, "unresolved_source_parents")
	if unresolved.Count != 1 || unresolved.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("unresolved_source_parents = %+v, want the one relation the read's projection left "+
			"unanswerable: the walk chained the last record through the result standing between it and "+
			"the call it named", unresolved)
	}
	for _, phrase := range []string{"visible ancestor", "does not hold"} {
		if !strings.Contains(unresolved.Detail, phrase) {
			t.Errorf("the detail does not name the relations the count stands for (%q missing): %q",
				phrase, unresolved.Detail)
		}
	}
	data := string(rollout.Data)
	if !strings.Contains(data, "internal/retry/config.go:41") {
		t.Errorf("the rollout does not hold the recorded result:\n%s", data)
	}
	if strings.Contains(data, "left turn") {
		t.Error("the rollout holds the abandoned branch's turn: a record written into this target's log " +
			"becomes model-visible history, so the branch was revived as an active turn")
	}
	if rollout.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the recorded result written", rollout.Carried)
	}

	// The other reading, for a source whose parent answers are exact: a Claude transcript's parentUuid
	// names a record rather than a projection, so a record naming the call really hung off the call --
	// the result stands off the line, is omitted as the read's own relations place it, and no relation
	// is counted as unanswerable.
	exact := conversation
	exact.Source.Provider = "claude"
	rollout, err = ConvertToCodexRollout(exact, testOptions())
	if err != nil {
		t.Fatalf("converting with an exact read: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 2 {
		t.Errorf("inactive_branch_records = %+v, want the result and the branch: the read answered the "+
			"record's parent exactly, so nothing is projected", loss)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v, want 0: an exact read answered every place the walk "+
			"took", loss)
	}
	if strings.Contains(string(rollout.Data), "internal/retry/config.go:41") {
		t.Error("the rollout holds the result although the read places it off the line")
	}

	// The adjacent shape, for a source whose answers are exact: a record naming a call *immediately
	// after* that call's result. Nothing stands between them, so the read's answer decides -- the record
	// hangs off the call and the result is a sibling the source placed beside it. Chaining the record
	// through the sibling instead (the projection heuristic applied without its gate) would pull the
	// sibling's own words -- here a user turn beside the result -- onto the line as active history.
	adjacent := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "u1"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
			DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
				InputJSON: `{"pattern":"maxBackoff"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), Content: "SIBLING-DIALOGUE",
			Dialogue: "SIBLING-DIALOGUE", SourceUUID: "r3", SourceParentUUID: "c2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_1",
				ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "after the result",
			Dialogue: "after the result", SourceUUID: "d4", SourceParentUUID: "c2"},
	)
	rollout, err = ConvertToCodexRollout(adjacent, testOptions())
	if err != nil {
		t.Fatalf("converting the adjacent exact shape: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 1 {
		t.Errorf("inactive_branch_records = %+v, want the sibling result omitted: the read answered every "+
			"record's parent exactly and nothing descends from the result", loss)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v, want 0: an exact read answered every place the walk "+
			"took", loss)
	}
	for _, sibling := range []string{"SIBLING-DIALOGUE", "internal/retry/config.go:41"} {
		if strings.Contains(string(rollout.Data), sibling) {
			t.Errorf("the rollout holds %q from the sibling result, which the read's own relations place "+
				"off the line: a record chained through it would revive that turn as active history",
				sibling)
		}
	}

	// The same adjacent shape under a read that *does* project: the reader answers the record as hanging
	// off the call although the result is the record before it, and the file's own continuation
	// (call → result → what follows) is the reading the walk takes, so the result stays on the line. The
	// place is counted: a record the source placed directly under the call and one it placed under the
	// call's result are the same answer in the read, so which of the two the file held is not answerable
	// and the line's reading of it is this conversion's, not the read's.
	projectedAdjacent := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "u1"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
			DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
				InputJSON: `{"pattern":"maxBackoff"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), SourceUUID: "r3", SourceParentUUID: "c2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_1",
				ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "after the result",
			Dialogue: "after the result", SourceUUID: "d4", SourceParentUUID: "c2"},
	)
	projectedAdjacent.Source.Provider = string(parser.AgentPi)
	rollout, err = ConvertToCodexRollout(projectedAdjacent, testOptions())
	if err != nil {
		t.Fatalf("converting the adjacent projected shape: %v", err)
	}
	if !strings.Contains(string(rollout.Data), "internal/retry/config.go:41") {
		t.Errorf("the rollout does not hold the result the walk keeps on the source's own line:\n%s",
			rollout.Data)
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 0 {
		t.Errorf("inactive_branch_records = %+v, want none: the result stands on the line the walk took",
			loss)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_source_parents"); loss.Count != 1 {
		t.Errorf("unresolved_source_parents = %+v, want the one place the read's projection left "+
			"unanswerable: the record names the call while a result of that call stands directly before "+
			"it, and the file's own order is the reading this conversion took", loss)
	}
}

// TestCodexRolloutOmitsAFoldedOutcomeOnAnAbandonedBranch pins the line walk's answer for the shape an
// archive read gives a call's outcome: the inherited projection attaches a result-only row to the
// call's own message and drops the row, so the read holds the outcome *on* the message that made the
// call. A later record naming that message's own parent could not have hung off the outcome, so it
// stays on the line while the call and its outcome -- a branch the conversation left -- come off it.
// The step the walk used to take matched the carrier by the record *it* hangs off, so it read this
// record as the continuation of the result and put the abandoned call back on the line with no relation
// counted.
func TestCodexRolloutOmitsAFoldedOutcomeOnAnAbandonedBranch(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "u1"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
			DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
				InputJSON: `{"pattern":"maxBackoff"}`}},
			ToolResults: []ToolResult{{ToolUseID: "toolu_1",
				ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28, ContentDecoded: true}}},
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "after the branch",
			Dialogue: "after the branch", SourceUUID: "d3", SourceParentUUID: "u1"},
	)
	conversation.Source.Provider = string(parser.AgentPi)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 1 {
		t.Errorf("inactive_branch_records = %+v, want the one record whose call branch the source's own "+
			"relations leave off the line", loss)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v, want 0: the record names the call's parent, and no "+
			"result of that parent stands anywhere for the read to have projected through", loss)
	}
	for _, abandoned := range []string{"[grep: maxBackoff]", "internal/retry/config.go:41"} {
		if strings.Contains(string(rollout.Data), abandoned) {
			t.Errorf("the rollout holds %q from the abandoned call branch, which the source's own "+
				"relations place off the line:\n%s", abandoned, rollout.Data)
		}
	}
	for _, text := range []string{"start", "after the branch"} {
		if !strings.Contains(string(rollout.Data), text) {
			t.Errorf("the conversation's own turn %q is not in the rollout:\n%s", text, rollout.Data)
		}
	}
}

// TestCodexRolloutCountsThePlacesItsWalkCouldNotAnswer pins the walk's accounting for the other two
// relations a read can leave open, the ones the shared category's detail names beside the projection: a
// record the read gave an identity but no parent, and a record naming a parent this read does not hold.
// Both are written on the line -- the file's order is the only evidence there is -- and both are counted
// (unresolved_source_parents), because the source's own place for them is not the one the line was built
// from. The tree writers count the same two places (TestPiSessionWritesTheSourcesOwnRelations,
// TestPiSessionCountsAnIdentifiedRecordTheReadLeftParentless); this case keeps the Codex route's report
// from saying less than its own detail claims.
func TestCodexRolloutCountsThePlacesItsWalkCouldNotAnswer(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "u1"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "one", Dialogue: "one",
			SourceUUID: "a2", SourceParentUUID: "u1"},
		// The read answered this record an identity and no parent: it is not the conversation's first, so
		// the walk's place for it -- the record before it -- is this conversion's.
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "two", Dialogue: "two",
			SourceUUID: "a3"},
		// The read named a parent it does not hold: the record follows the record before it.
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "three", Dialogue: "three",
			SourceUUID: "a4", SourceParentUUID: "gone"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	loss := lossCount(t, rollout.Losses, "unresolved_source_parents")
	if loss.Count != 2 || loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("unresolved_source_parents = %+v, want the two relations the walk could not answer", loss)
	}
	if lossDetail := loss.Detail; !strings.Contains(lossDetail, "not the conversation's first") ||
		!strings.Contains(lossDetail, "does not hold") {
		t.Errorf("the detail does not name the relations the count stands for: %q", lossDetail)
	}
	for _, text := range []string{"start", "one", "two", "three"} {
		if !strings.Contains(string(rollout.Data), text) {
			t.Errorf("the record %q is not in the rollout, although the count names a place and not an "+
				"omission", text)
		}
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 0 {
		t.Errorf("inactive_branch_records = %+v, want none: every record is on the line the walk reached",
			loss)
	}

	// The identity-less document is the other case: the read answered no place for any record, so its own
	// order is all there is to follow, and following it invents nothing.
	plain := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "one", Dialogue: "one"},
	)
	rollout, err = ConvertToCodexRollout(plain, testOptions())
	if err != nil {
		t.Fatalf("converting an identity-less document: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v for an identity-less read, want 0: its order is the "+
			"read's own answer", loss)
	}
}

// TestCodexRolloutPairsAnIdLessResultWithItsCall pins the pairing rule: the target pairs a call and its
// output by id, so a source that recorded none gets one generated from the message's own position in the
// document — and two id-less calls make the pairing a guess, which is reported as an unpaired result
// instead. Two of them in two messages whose ordinals were never assigned are still two calls.
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

	// Two messages whose ordinals were never assigned (both zero), each carrying one id-less call: keyed
	// by the call's own place in the document, the two calls stay two calls, so the outstanding set
	// holds both and this result is still refused. Keyed by the ordinal they would share one id, the set
	// would read one call, and the result would pair -- which the report would then claim as carried.
	crossMessage := testConversation(
		Message{Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Role: string(parser.RoleAssistant), Content: "[Read: /y]",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/y"}`}}},
		Message{Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of one of them"}}},
	)
	rollout, err = ConvertToCodexRollout(crossMessage, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Errorf("carried %d result(s) for two id-less calls in two messages, which is a pairing the "+
			"source did not record", rollout.Carried.ToolResults)
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the ambiguous result counted",
			lossCount(t, rollout.Losses, "unpaired_tool_results"))
	}
	if lossCount(t, rollout.Losses, "generated_call_ids").Count != 2 {
		t.Errorf("generated_call_ids = %+v, want the two calls and no inferred pairing",
			lossCount(t, rollout.Losses, "generated_call_ids"))
	}
	ids := []string{}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionCall {
			ids = append(ids, payloadField(t, line, "call_id"))
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Errorf("the two messages' id-less calls carry ids %q, want one distinct id per call", ids)
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

	// No call is outstanding at all: an id-less result has nothing to pair with, so it is counted and
	// not written. This is the shape a read takes when the result's call is not in the conversation, and
	// the pairing rule has to answer it the same way it answers two or three outstanding calls -- the
	// source recorded no id, and there is no call this conversion wrote for it to belong to.
	noOutstanding := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "NO-CALL-TO-PAIR-WITH"}}},
	)
	rollout, err = ConvertToCodexRollout(noOutstanding, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want no result: there was no outstanding call to pair with",
			rollout.Carried)
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the result with no call counted",
			lossCount(t, rollout.Losses, "unpaired_tool_results"))
	}
	if lossCount(t, rollout.Losses, "generated_call_ids").Count != 0 {
		t.Errorf("generated_call_ids = %+v, want none: no id could be generated for a result with no call",
			lossCount(t, rollout.Losses, "generated_call_ids"))
	}
	if strings.Contains(string(rollout.Data), "NO-CALL-TO-PAIR-WITH") {
		t.Error("the rollout holds a result with no call to pair it with")
	}

	// The result names a call, and this conversion did not write that call: the recorded id is the
	// source's own, but no call of that id is in the conversation (the call preceded the read, or the
	// provider recorded a result whose call it did not carry). An output without its call is not a pair,
	// and inventing the call would state a tool invocation the source never recorded -- so the result is
	// omitted and counted, and the call the conversion did write stays outstanding.
	foreignCall := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_written", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_not_written",
				ContentRaw: "THE WORDS OF A RESULT WHOSE CALL IS NOT HERE"}}},
	)
	rollout, err = ConvertToCodexRollout(foreignCall, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want the call written and the foreign result omitted", rollout.Carried)
	}
	if lossCount(t, rollout.Losses, "unpaired_tool_results").Count != 1 {
		t.Errorf("unpaired_tool_results = %+v, want the result naming an unwritten call counted",
			lossCount(t, rollout.Losses, "unpaired_tool_results"))
	}
	if strings.Contains(string(rollout.Data), "THE WORDS OF A RESULT WHOSE CALL IS NOT HERE") {
		t.Error("the rollout holds a result whose call this conversion did not write")
	}
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == codexItemFunctionOutput {
			t.Errorf("the rollout holds a %s for a call that was not written: %s", codexItemFunctionOutput,
				line.Payload)
		}
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

// TestCodexRolloutKeepsALiteralThinkingBlockThroughAnInheritedReader pins the semantic rule through a
// reader that renders nothing: internal/parser's Vibe reader records a message's own content and its
// reasoning_content independently -- Content is the recorded text, Thinking the recorded trace, no
// dialogue projection and no rendering of either into the other (internal/parser/vibe.go's
// convertVibeMessage) -- so a record whose content *is* a `[Thinking]…[/Thinking]` block is the model's
// own words, and nothing may drop them: not the whole-content rendering shape, which asks the block's
// words to *be* the record's own trace, and not the rules a reader that renders the trace would set
// (Vibe is not named by thinkingRenderingReaders, and no projection covers the record). Both arms put a
// trace beside the block, one of them the block's own words: a rendering is only ever a rendering
// *of the record's own trace* by a reader known to write one, and neither holds here. The trace still
// travels as the record's reasoning (counted here: the Codex target has no shape for a plaintext trace),
// so nothing is lost; the words the source recorded are what the target must not state the agent never
// said.
func TestCodexRolloutKeepsALiteralThinkingBlockThroughAnInheritedReader(t *testing.T) {
	const literal = "[Thinking]\nDocumentation example\n[/Thinking]"
	cases := []struct {
		name  string
		trace string
	}{
		{"the trace holds none of the block's words", "private trace"},
		{"the trace holds the block's words exactly", "Documentation example"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			sessionDir := filepath.Join(root, "session_20260613_123456_00000000-0000-4000-8000-000000000001")
			if err := os.MkdirAll(sessionDir, 0o700); err != nil {
				t.Fatalf("creating the session directory: %v", err)
			}
			transcript := strings.Join([]string{
				`{"role":"user","content":"Ask about the docs.","message_id":"m1"}`,
				`{"role":"assistant","content":"[Thinking]\nDocumentation example\n[/Thinking]",` +
					`"reasoning_content":"` + testCase.trace + `","message_id":"m2","model":"vibe-model"}`,
			}, "\n") + "\n"
			transcriptPath := filepath.Join(sessionDir, "messages.jsonl")
			if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
				t.Fatalf("writing the transcript: %v", err)
			}
			cfg := fixtureConfig(t)
			cfg.AgentDirs[parser.AgentVibe] = []string{root}
			conversation, err := NewSourceReader(cfg).Read(context.Background(),
				Ref{Provider: "vibe", ID: transcriptPath})
			if err != nil {
				t.Fatalf("reading the Vibe transcript: %v", err)
			}
			if len(conversation.Messages) != 2 || conversation.Messages[1].Role != string(parser.RoleAssistant) {
				t.Fatalf("the read answered %d message(s), want the transcript's two with the assistant second",
					len(conversation.Messages))
			}
			dialogue := dialogueOf(conversation.Messages[1])
			if dialogue.Text != literal || dialogue.State != dialogueFromContent {
				t.Errorf("dialogueOf over the read's own record = (%q, %v), want the model's own words %q "+
					"as the record's own bytes: the reader renders nothing, so nothing shows the block to "+
					"be a rendering of the trace recorded beside it", dialogue.Text, dialogue.State, literal)
			}
			rollout, err := ConvertToCodexRollout(conversation, testOptions())
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			if !strings.Contains(string(rollout.Data), "Documentation example") {
				t.Error("the rollout does not carry the record's literal block: its words were dropped as though " +
					"they were a rendering of the trace recorded beside them")
			}
			if loss := lossCount(t, rollout.Losses, "reasoning_text"); loss.Count != 1 {
				t.Errorf("reasoning_text = %+v, want the record's trace counted once", loss)
			}
		})
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
// target reads the file's order. The clock's substitution is also the report's: a record whose own
// recorded instant precedes the instant already written is stamped with that instant, and the report
// counts each written timestamp it was substituted on (substituted_record_instants), so the written
// record cannot read as the source's own answer for its instant. The shared helper this pins is
// entryClock (route_shared.go), which the Claude, Pi and OMP writers drive the same way; this route is
// where the clock's own regression scenario already stands, and its writer stamps one record as several
// entries, so the per-written-timestamp shape of the count is visible here too.
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
	// The out-of-order record's own instant (09:00:00) is not in the file: every entry the writer stamps
	// for it carries the previous written record's instant (10:00:10) -- the response item and the view
	// event both, because one record's stamp is one instant on all of its entries.
	stamped := 0
	for _, line := range lines {
		if !strings.Contains(string(line.Payload), "Earlier") {
			continue
		}
		stamped++
		if line.Timestamp != "2026-05-05T10:00:10Z" {
			t.Errorf("the out-of-order record's %s entry is stamped %s, want the previous written "+
				"record's instant 2026-05-05T10:00:10Z", line.Type, line.Timestamp)
		}
	}
	if stamped == 0 {
		t.Fatal("the file holds no entry for the out-of-order record")
	}
	// The count is per written timestamp: the out-of-order message above is one record, but the writer
	// stamps it as two entries (a response item and the view event), and each of those written timestamps
	// is counted. Its detail states what the substitution did with the recorded instant.
	loss := lossCount(t, rollout.Losses, "substituted_record_instants")
	if loss.Count != stamped || loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("substituted_record_instants = %+v, want the %d written timestamp(s) of the "+
			"out-of-order record counted as carried-with-caveat", loss, stamped)
	}
	if !strings.Contains(loss.Detail, "never one this build invented") {
		t.Errorf("substituted_record_instants detail = %q, want the instant written in the recorded "+
			"one's place named", loss.Detail)
	}

	// The control: an ordered source counts nothing, so the counter above is the order and not the
	// clock's normal path.
	ordered := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask",
			Timestamp: time.Date(2026, 5, 5, 10, 0, 10, 0, time.UTC)},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Answer", Dialogue: "Answer",
			Timestamp: time.Date(2026, 5, 5, 10, 0, 11, 0, time.UTC)},
	)
	rollout, err = ConvertToCodexRollout(ordered, testOptions())
	if err != nil {
		t.Fatalf("converting the ordered source: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "substituted_record_instants"); loss.Count != 0 {
		t.Errorf("substituted_record_instants = %+v, want zero for an ordered source", loss)
	}
}

// TestCodexRolloutCountsTheSubagentSessionLinkItsLogDoesNotHold pins the disclosure for the one piece of
// a `spawn_agent` call this rollout does not state: which session the call spawned. The inherited Codex
// parser records that child session's id on the call (ToolCall.SubagentSessionID) and the reader
// preserves it, and the rollout has a record shape that holds the attribution -- the
// `collab_agent_spawn_end` event payload whose call_id and new_thread_id the inherited reader turns
// into this link (internal/parser/codex.go:700-707,834-865) -- but this conversion writes no such event:
// the function_call carries the call's id, name and arguments and nothing names the child. The omission
// is counted rather than left silent, the child session is neither read nor converted, and the count is
// of the calls this conversion wrote.
func TestCodexRolloutCountsTheSubagentSessionLinkItsLogDoesNotHold(t *testing.T) {
	const childSession = "codex:019c9c96-6ee7-77c0-ba4c-380f844289d5"
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "run a child agent",
			Dialogue: "run a child agent", DialogueProjected: true},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "spawning a child",
			Dialogue: "spawning a child", DialogueProjected: true,
			ToolCalls: []ToolCall{
				{ToolUseID: "call_spawn", Name: "spawn_agent",
					InputJSON: `{"agent_type":"awaiter"}`, SubagentSessionID: childSession},
				{ToolUseID: "call_wait", Name: "wait", InputJSON: `{"ids":["019c9c96"]}`},
			}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting the document: %v", err)
	}
	loss := lossCount(t, rollout.Losses, "subagent_session_links")
	if loss.Count != 1 || loss.Disposition != DispositionOmitted {
		t.Errorf("subagent_session_links = %+v, want the one written call's link counted as omitted", loss)
	}
	if !strings.Contains(loss.Detail, "child session") {
		t.Errorf("the category's reason does not name the relationship it counts: %q", loss.Detail)
	}
	// The route's own behaviour, pinned: the call itself is written with its id and name, and the child's
	// id is nowhere in the log -- neither in the call record nor in the output that carries the
	// notification summary in a real rollout's place.
	installed := string(rollout.Data)
	for _, want := range []string{`"call_id":"call_spawn"`, `"name":"spawn_agent"`, `"call_id":"call_wait"`} {
		if !strings.Contains(installed, want) {
			t.Errorf("the written rollout does not hold %s:\n%s", want, installed)
		}
	}
	if strings.Contains(installed, "019c9c96-6ee7-77c0-ba4c-380f844289d5") {
		t.Errorf("the child session's id is in the file although the report says the link is omitted:\n%s",
			installed)
	}
	if rollout.Carried.ToolCalls != 2 {
		t.Errorf("carried %d call(s), want the two written calls", rollout.Carried.ToolCalls)
	}
}

// TestCodexRolloutCountsTheResultRecordInstantsItDoesNotWrite pins the disclosure for the instant a
// carried result was recorded on. ToolResult.Timestamp answers the instant of the result's own record --
// a result event's envelope instant, which may be later than the record that carried its call -- and a
// function_call_output's envelope states the record's own instant, but this conversion stamps it with
// the containing message's instant: the read's own answer for the result is not what the file holds, and
// the two reads are counted apart -- the read answered an instant and the containing record's is written
// instead (result_record_instants, omitted), or the read answered none and the containing record's
// instant stands in (result_completion_fallbacks, carried-with-caveat). A result whose own recorded
// instant is the containing message's is counted in neither: the file states the read's answer.
func TestCodexRolloutCountsTheResultRecordInstantsItDoesNotWrite(t *testing.T) {
	at := time.Date(2026, 5, 5, 9, 15, 5, 0, time.UTC)
	late := at.Add(time.Second)
	same := time.Date(2026, 5, 5, 9, 15, 7, 0, time.UTC)
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "run it", Dialogue: "run it",
			DialogueProjected: true},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "calling",
			Dialogue: "calling", DialogueProjected: true, Timestamp: at,
			ToolCalls: []ToolCall{
				{ToolUseID: "call_late", Name: "shell", InputJSON: `{"command":"ls"}`},
				{ToolUseID: "call_none", Name: "shell", InputJSON: `{"command":"pwd"}`},
				{ToolUseID: "call_same", Name: "shell", InputJSON: `{"command":"date"}`},
			}},
		Message{Ordinal: 2, Role: string(parser.RoleUser), Timestamp: same, ToolResults: []ToolResult{
			{ToolUseID: "call_late", ContentRaw: "late output", ContentDecoded: true, Timestamp: late,
				Source: "wait_output"},
			{ToolUseID: "call_none", ContentRaw: "no instant", ContentDecoded: true},
			{ToolUseID: "call_same", ContentRaw: "own record", ContentDecoded: true,
				// The read answered an instant equal to the containing record's here (a block-shaped
				// result), so what the file states is the read's own answer.
				Timestamp: same},
		}},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting the document: %v", err)
	}
	omitted := lossCount(t, rollout.Losses, "result_record_instants")
	if omitted.Count != 1 || omitted.Disposition != DispositionOmitted {
		t.Errorf("result_record_instants = %+v, want the one written result whose own instant the file "+
			"does not state", omitted)
	}
	if !strings.Contains(omitted.Detail, "result's own recorded instant") {
		t.Errorf("the category's reason does not name the value it counts: %q", omitted.Detail)
	}
	fallbacks := lossCount(t, rollout.Losses, "result_completion_fallbacks")
	if fallbacks.Count != 1 || fallbacks.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("result_completion_fallbacks = %+v, want the one written result the read answered no "+
			"instant for", fallbacks)
	}
	sources := lossCount(t, rollout.Losses, "tool_result_sources")
	if sources.Count != 1 || sources.Disposition != DispositionOmitted {
		t.Errorf("tool_result_sources = %+v, want the one written result whose recorded channel label "+
			"the output has no member for", sources)
	}
	// The route's own behaviour, pinned: the late result's text travels, and the instant the read
	// answered for its own record is nowhere in the file -- the output record states the containing
	// record's instant instead.
	installed := string(rollout.Data)
	if !strings.Contains(installed, "late output") {
		t.Errorf("the written rollout does not hold the late result's text:\n%s", installed)
	}
	if want := late.Format(time.RFC3339Nano); strings.Contains(installed, want) {
		t.Errorf("the file states the result's own recorded instant %s although the report says it is "+
			"not written:\n%s", want, installed)
	}
	if loss := lossCount(t, rollout.Losses, "substituted_record_instants"); loss.Count != 0 {
		t.Errorf("substituted_record_instants = %+v, want none: this document's records are ordered",
			loss)
	}
}

// TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage pins the pair of records every written
// dialogue message needs, and that they carry the same text under the target's own record kinds.
//
// The target writes each message twice: a `response_item` (the record its model history is built from)
// and an `event_msg` of its own kind (the record its own view of the turn is built from). Measured on
// the tested CLI version: a rollout holding only the response_item loads and resumes, while the
// harness's own view of that session shows no turn at all, so "installed" would be a claim about a file
// rather than about a session a person or an agent can open. The plan's record table
// (docs/plans/0002-claude-to-codex-continuation.md §2.2) is what this test holds the writer to, and the
// native half (scripts/codex-native-check.py, not a gate step) is where the view itself is read back.
func TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "Answer", Dialogue: "Answer"},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}

	type record struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Message string `json:"message"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"payload"`
	}
	parsed := []record{}
	for _, line := range strings.Split(strings.TrimSpace(string(rollout.Data)), "\n") {
		var one record
		if err := json.Unmarshal([]byte(line), &one); err != nil {
			t.Fatalf("the rollout holds a line that is not a record: %v\n%s", err, line)
		}
		parsed = append(parsed, one)
	}

	// Every written dialogue message: the model-visible record under its role's own block type, and the
	// event record of the kind the target writes for that role.
	for _, item := range rollout.Items {
		if item.Kind != ItemUser && item.Kind != ItemAssistant {
			continue
		}
		// The literals are the target's own record kinds (docs/plans/0002-claude-to-codex-continuation.md
		// §2.2), named here as the file must spell them rather than through this writer's constants: a
		// constant renamed on both sides of the comparison would leave the file saying something the
		// target does not read. The two envelope spellings are pinned the same way, from the carried
		// reader rather than from this writer: the upstream parser names them `response_item` and
		// `event_msg` (internal/parser/codex.go's codexTypeResponseItem/codexTypeEventMsg), so a writer
		// whose spelling drifts fails here instead of moving this assertion with it.
		role, blockType, eventType := "user", "input_text", "user_message"
		if item.Kind == ItemAssistant {
			role, blockType, eventType = "assistant", "output_text", "agent_message"
		}
		var modelRecords, eventRecords int
		for _, one := range parsed {
			if one.Type == "response_item" && one.Payload.Type == "message" &&
				one.Payload.Role == role {
				for _, block := range one.Payload.Content {
					if block.Type == blockType && block.Text == item.Text {
						modelRecords++
					}
				}
			}
			if one.Type == "event_msg" && one.Payload.Type == eventType &&
				one.Payload.Message == item.Text {
				eventRecords++
			}
		}
		if modelRecords != 1 {
			t.Errorf("the file holds %d model-visible record(s) for %s %q, want exactly one",
				modelRecords, role, item.Text)
		}
		if eventRecords != 1 {
			t.Errorf("the file holds %d %s event(s) for %q, want exactly one: the harness's own view of "+
				"the turn is built from that record, and its absence is a session the target loads but "+
				"does not show", eventRecords, eventType, item.Text)
		}
	}
}

// TestCodexRolloutRetiresACallWhenItsResultIsOmitted pins the other half of the pairing rule: a recorded
// result answers its call even when this conversion writes no output for it -- the text is empty, holds
// only whitespace, or holds only blocks the target's output field has no room for. The outstanding set is
// what an id-less result may be paired with, so a call whose own result arrived must leave it; left
// pending, a later id-less result would be attributed to a call that already has an answer, or refused as
// ambiguous while every call it could belong to is answered.
func TestCodexRolloutRetiresACallWhenItsResultIsOmitted(t *testing.T) {
	// (a) An omitted result followed by an id-less result: the omitted result answers its call, so the
	// id-less result has nothing to pair with and is counted rather than attributed to the answered call.
	omitted := []struct {
		name   string
		result ToolResult
		loss   string
		blocks int
	}{
		{name: "no text at all", result: ToolResult{ToolUseID: "toolu_a"}, loss: "empty_tool_results"},
		{name: "whitespace only", result: ToolResult{ToolUseID: "toolu_a", ContentRaw: "  \n  "},
			loss: "whitespace_only_tool_results"},
		{name: "an image block only", loss: "empty_tool_results", blocks: 1,
			result: ToolResult{ToolUseID: "toolu_a",
				ContentRaw: `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]`}},
	}
	for _, omittedCase := range omitted {
		conversation := testConversation(
			Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
			Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
			Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{omittedCase.result}},
			Message{Ordinal: 3, Role: string(parser.RoleUser),
				ToolResults: []ToolResult{{ContentRaw: "A LATER RESULT WITH NO ID"}}},
		)
		rollout, err := ConvertToCodexRollout(conversation, testOptions())
		if err != nil {
			t.Fatalf("converting (%s): %v", omittedCase.name, err)
		}
		if rollout.Carried.ToolResults != 0 {
			t.Errorf("(%s) carried = %+v, want no result: the call the id-less result could pair with "+
				"was answered by the omitted result", omittedCase.name, rollout.Carried)
		}
		if loss := lossCount(t, rollout.Losses, omittedCase.loss); loss.Count != 1 {
			t.Errorf("(%s) %s = %+v, want the omitted result counted", omittedCase.name, omittedCase.loss, loss)
		}
		if loss := lossCount(t, rollout.Losses, "unrepresentable_result_blocks"); loss.Count != omittedCase.blocks {
			t.Errorf("(%s) unrepresentable_result_blocks = %+v, want %d", omittedCase.name, loss, omittedCase.blocks)
		}
		if loss := lossCount(t, rollout.Losses, "unpaired_tool_results"); loss.Count != 1 {
			t.Errorf("(%s) unpaired_tool_results = %+v, want the id-less result counted, not paired with "+
				"the call whose own result is recorded", omittedCase.name, loss)
		}
		if strings.Contains(string(rollout.Data), "A LATER RESULT WITH NO ID") {
			t.Errorf("(%s) the rollout attributes the id-less result to the call whose own result is "+
				"recorded", omittedCase.name)
		}
	}

	// (b) Two results for the same call, one of them omitted, in both orders: the omitted result is
	// counted, and the other result -- before or after the retirement -- is still accepted, because
	// acceptance is by the call this conversion wrote and not by what stays outstanding.
	for _, order := range []struct {
		name    string
		results []ToolResult
	}{
		{name: "the recorded output first", results: []ToolResult{
			{ToolUseID: "toolu_a", ContentRaw: "the recorded output"}, {ToolUseID: "toolu_a"}}},
		{name: "the omitted result first", results: []ToolResult{
			{ToolUseID: "toolu_a"}, {ToolUseID: "toolu_a", ContentRaw: "the recorded output"}}},
	} {
		conversation := testConversation(
			Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
			Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
			Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: order.results},
		)
		rollout, err := ConvertToCodexRollout(conversation, testOptions())
		if err != nil {
			t.Fatalf("converting (%s): %v", order.name, err)
		}
		if rollout.Carried.ToolResults != 1 {
			t.Errorf("(%s) carried = %+v, want the one representable result written", order.name, rollout.Carried)
		}
		if loss := lossCount(t, rollout.Losses, "empty_tool_results"); loss.Count != 1 {
			t.Errorf("(%s) empty_tool_results = %+v, want the omitted result counted", order.name, loss)
		}
		if loss := lossCount(t, rollout.Losses, "unpaired_tool_results"); loss.Count != 0 {
			t.Errorf("(%s) unpaired_tool_results = %+v, want none: the second result names the call this "+
				"conversion wrote", order.name, loss)
		}
		outputs := 0
		for _, line := range decodeRollout(t, rollout.Data) {
			if payloadField(t, line, "type") != codexItemFunctionOutput {
				continue
			}
			outputs++
			output, callID := payloadField(t, line, "output"), payloadField(t, line, "call_id")
			if output != "the recorded output" || callID != "toolu_a" {
				t.Errorf("(%s) the written output is %q for call %q, want the recorded text under toolu_a",
					order.name, output, callID)
			}
		}
		if outputs != 1 {
			t.Errorf("(%s) the rollout holds %d function_call_output record(s), want exactly one", order.name, outputs)
		}
	}

	// (c) An omitted result for one of two outstanding calls: the answered call leaves the outstanding
	// set, so the id-less result after it pairs with the call that really has no result instead of being
	// refused as ambiguous for the answered call's sake.
	twoCalls := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x and /y]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Read", InputJSON: `{"file_path":"/x"}`},
				{ToolUseID: "toolu_b", Name: "Read", InputJSON: `{"file_path":"/y"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ToolUseID: "toolu_a"}}},
		Message{Ordinal: 3, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "the answer to the one that is left"}}},
	)
	rollout, err := ConvertToCodexRollout(twoCalls, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the id-less result paired with the one call that has none", rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "unpaired_tool_results"); loss.Count != 0 {
		t.Errorf("unpaired_tool_results = %+v, want none: one call was still unanswered", loss)
	}
	outputs := 0
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") != codexItemFunctionOutput {
			continue
		}
		outputs++
		output, callID := payloadField(t, line, "output"), payloadField(t, line, "call_id")
		if callID != "toolu_b" {
			t.Errorf("the id-less result was paired with %q, want the call with no result (toolu_b)", callID)
		}
		if output != "the answer to the one that is left" {
			t.Errorf("the paired output is %q, want the id-less result's own text", output)
		}
	}
	if outputs != 1 {
		t.Errorf("the rollout holds %d function_call_output record(s), want exactly one under toolu_b", outputs)
	}
}

// TestCodexRolloutCountsAnInferredResultPairingOnlyWhenItIsWritten pins the result half of the
// generated_call_ids rule: a result the source recorded without an id of its own is written under the
// pairing this conversion inferred from the one call still outstanding, and that pairing is the only id
// the record has -- so it is counted only for a result that reached the file. A result whose output is
// empty, holds only whitespace, or holds only blocks the target's output field has no room for is not
// written at all: its omission is named by its own category, and the pairing inferred for it is not
// evidence the file holds. Counted as carried, the report would claim an id the file never holds, which
// is the same defect the output text is already spared. The same pairing on a representable result is
// counted, which is what makes the zero a statement about the omission rather than about the counter
// being ignored.
func TestCodexRolloutCountsAnInferredResultPairingOnlyWhenItIsWritten(t *testing.T) {
	omitted := []struct {
		name   string
		result ToolResult
		loss   string
		blocks int
	}{
		{name: "no text at all", loss: "empty_tool_results",
			result: ToolResult{Status: "errored", PersistedOutputUnresolved: true}},
		{name: "whitespace only", loss: "whitespace_only_tool_results",
			result: ToolResult{ContentRaw: "  \n  "}},
		{name: "an image block only", loss: "empty_tool_results", blocks: 1,
			result: ToolResult{ContentRaw: `[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]`}},
	}
	for _, omittedCase := range omitted {
		// One outstanding call, carrying the id the source recorded for it: the id-less result below is
		// the one whose pairing with that call has to be inferred.
		conversation := testConversation(
			Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
			Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
			Message{Ordinal: 2, Role: string(parser.RoleUser), ToolResults: []ToolResult{omittedCase.result}},
		)
		rollout, err := ConvertToCodexRollout(conversation, testOptions())
		if err != nil {
			t.Fatalf("converting (%s): %v", omittedCase.name, err)
		}
		if rollout.Carried.ToolResults != 0 {
			t.Errorf("(%s) carried = %+v, want no result written", omittedCase.name, rollout.Carried)
		}
		if loss := lossCount(t, rollout.Losses, omittedCase.loss); loss.Count != 1 {
			t.Errorf("(%s) %s = %+v, want the omitted result counted under its own category",
				omittedCase.name, omittedCase.loss, loss)
		}
		if loss := lossCount(t, rollout.Losses, "unrepresentable_result_blocks"); loss.Count != omittedCase.blocks {
			t.Errorf("(%s) unrepresentable_result_blocks = %+v, want %d: blocks are counted as blocks "+
				"whatever happens to the result carrying them", omittedCase.name, loss, omittedCase.blocks)
		}
		for _, category := range []string{"generated_call_ids", "unresolved_tool_outputs",
			"tool_result_outcomes"} {
			loss := lossCount(t, rollout.Losses, category)
			if loss.Count != 0 || loss.Disposition != DispositionCarriedWithCaveat {
				t.Errorf("(%s) %s = %+v, want 0 and %q: the id-less result was not written, so neither "+
					"the pairing inferred for it nor its own flags carried anything",
					omittedCase.name, category, loss, DispositionCarriedWithCaveat)
			}
		}
		for _, item := range rollout.Items {
			if item.Kind == ItemToolResult {
				t.Errorf("(%s) the item list holds a %s for call %q although the report says the result "+
					"was not written", omittedCase.name, item.Kind, item.CallID)
			}
		}
	}

	// The same shape with output the target's field can hold: the inferred pairing is the id the record
	// is written under, and the category counts it once -- the call's own id was recorded, so nothing
	// else in this conversion is counted here.
	written := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask"},
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[Read: /x]",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read", InputJSON: `{"file_path":"/x"}`}}},
		Message{Ordinal: 2, Role: string(parser.RoleUser),
			ToolResults: []ToolResult{{ContentRaw: "content of /x"}}},
	)
	rollout, err := ConvertToCodexRollout(written, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if rollout.Carried.ToolResults != 1 {
		t.Fatalf("carried = %+v, want the id-less result written under the outstanding call", rollout.Carried)
	}
	if loss := lossCount(t, rollout.Losses, "generated_call_ids"); loss.Count != 1 {
		t.Errorf("generated_call_ids = %+v, want the written result's inferred pairing counted once", loss)
	}
	outputs := 0
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") != codexItemFunctionOutput {
			continue
		}
		outputs++
		output, callID := payloadField(t, line, "output"), payloadField(t, line, "call_id")
		if callID != "toolu_1" || output != "content of /x" {
			t.Errorf("the written output is %q for call %q, want the id-less result's text under toolu_1",
				output, callID)
		}
	}
	if outputs != 1 {
		t.Errorf("the rollout holds %d function_call_output record(s), want exactly one", outputs)
	}
}
