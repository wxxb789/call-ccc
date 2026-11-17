package ccc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// decodeTranscript reads a converted transcript back as a list of raw records, so a test asserts what the
// file the target harness receives really holds rather than what the writer kept in memory.
func decodeTranscript(t *testing.T, transcript ClaudeTranscript) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(transcript.Data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the transcript holds a line that is not one JSON object: %v\n%s", err, line)
		}
		records = append(records, record)
	}
	return records
}

// messageBlocks answers one record's content as a list of content blocks, whatever spelling the writer
// chose for it: a plain string (one text block, as the harness writes a single text block) or an array.
func messageBlocks(t *testing.T, record map[string]any) []map[string]any {
	t.Helper()
	message, ok := record["message"].(map[string]any)
	if !ok {
		t.Fatalf("record holds no message: %v", record)
	}
	switch content := message["content"].(type) {
	case string:
		return []map[string]any{{"type": "text", "text": content}}
	case []any:
		var blocks []map[string]any
		for _, raw := range content {
			block, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("a content block is not an object: %v", raw)
			}
			blocks = append(blocks, block)
		}
		return blocks
	default:
		t.Fatalf("message content is neither a string nor an array: %v", message["content"])
		return nil
	}
}

// recordBlocks answers one record's content blocks, or nothing when the record holds no message (the
// boundary marker, or a record kind this writer does not write). It is the reader the walks over the
// whole file use, where a record without a message is a fact rather than a failure.
func recordBlocks(record map[string]any) []map[string]any {
	message, ok := record["message"].(map[string]any)
	if !ok {
		return nil
	}
	switch content := message["content"].(type) {
	case string:
		return []map[string]any{{"type": "text", "text": content}}
	case []any:
		var blocks []map[string]any
		for _, raw := range content {
			if block, ok := raw.(map[string]any); ok {
				blocks = append(blocks, block)
			}
		}
		return blocks
	default:
		return nil
	}
}

// TestClaudeProjectDirSpelling pins the rule the target's session discovery uses, measured on Claude Code
// 2.1.326: a session for /tmp/probe3/WS.Upper Case_1+plus was filed under
// -tmp-probe3-WS-Upper-Case-1-plus. The loader finds a session only through the directory its own working
// directory spells, so a writer that got this wrong would install a session nothing could resume.
func TestClaudeProjectDirSpelling(t *testing.T) {
	for _, testCase := range []struct{ cwd, want string }{
		{"/Users/alice/code/my-app", "-Users-alice-code-my-app"},
		{"/tmp/probe3/WS.Upper Case_1+plus", "-tmp-probe3-WS-Upper-Case-1-plus"},
		{"/home/user/project", "-home-user-project"},
		{"C:\\Users\\alice\\project", "C--Users-alice-project"},
		{"/tmp/café-app", "-tmp-caf--app"},
		{"relative/dir", "relative-dir"},
	} {
		if got := ClaudeProjectDir(testCase.cwd); got != testCase.want {
			t.Errorf("ClaudeProjectDir(%q) = %q, want %q", testCase.cwd, got, testCase.want)
		}
	}
}

// TestConvertToClaudeTranscriptWritesTheMeasuredShape pins the records a conversion of the committed Codex
// fixture produces against what the target's loader was measured to need: the envelope the loader walks
// (uuid, parentUuid chain, cwd, sessionId, version, timestamp), one record per component of a message,
// the message id its records share, the tool pair, and the two records a compaction boundary takes.
func TestConvertToClaudeTranscriptWritesTheMeasuredShape(t *testing.T) {
	conversation := readFixtureConversation(t, codexFixtureRef)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{
		SessionID: "8f1a1d2c-0000-4000-8000-000000000001",
		Now:       time.Date(2026, 5, 4, 9, 15, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	records := decodeTranscript(t, transcript)
	if len(records) == 0 {
		t.Fatal("the transcript is empty")
	}
	if transcript.FileName != transcript.SessionID+".jsonl" {
		t.Errorf("the file name is %q, want the session id and .jsonl", transcript.FileName)
	}
	if transcript.Directory != ClaudeProjectDir(conversation.Source.Cwd) {
		t.Errorf("the project directory is %q, want the source cwd's spelling", transcript.Directory)
	}

	previous := ""
	lastTime := time.Time{}
	for index, record := range records {
		if got := record["sessionId"]; got != transcript.SessionID {
			t.Errorf("record %d carries sessionId %v, want the file stem %q", index, got, transcript.SessionID)
		}
		if got := record["version"]; got != ClaudeFormatVersion {
			t.Errorf("record %d carries version %v, want the pinned format version", index, got)
		}
		if got := record["cwd"]; got != conversation.Source.Cwd {
			t.Errorf("record %d carries cwd %v, want the source's %q", index, got, conversation.Source.Cwd)
		}
		uuid, _ := record["uuid"].(string)
		if uuid == "" {
			t.Fatalf("record %d has no uuid", index)
		}
		parent, present := record["parentUuid"]
		if index == 0 {
			if present && parent != nil {
				t.Errorf("the first record's parentUuid is %v, want null: the loader walks the chain from a root", parent)
			}
		} else if got, _ := parent.(string); got != previous {
			t.Errorf("record %d's parentUuid is %v, want the previous record's %q: the loader walks that chain",
				index, parent, previous)
		}
		previous = uuid
		parsed, err := time.Parse(time.RFC3339, record["timestamp"].(string))
		if err != nil {
			t.Fatalf("record %d's timestamp does not parse: %v", index, err)
		}
		if parsed.Before(lastTime) {
			t.Errorf("record %d's timestamp goes backwards (%s after %s)", index, parsed, lastTime)
		}
		lastTime = parsed
	}

	// The boundary is the pair the harness itself writes: the marker the loader truncates at, and the
	// summary record that stands in place of the summarized turns.
	var boundarySeen, summarySeen bool
	for index, record := range records {
		if record["type"] == claudeTypeSystem && record["subtype"] == claudeSubtypeCompactBoundary {
			boundarySeen = true
			if next := records[index+1]; next["type"] != claudeTypeUser || next["isCompactSummary"] != true {
				t.Errorf("the record after the boundary marker is %v, want the summary record", next)
			}
		}
		if record["isCompactSummary"] == true {
			summarySeen = true
			blocks := messageBlocks(t, record)
			if len(blocks) != 1 || blocks[0]["type"] != claudeBlockText || blocks[0]["text"] == "" {
				t.Errorf("the summary record does not hold its text as one text block: %v", blocks)
			}
		}
	}
	if !boundarySeen || !summarySeen {
		t.Errorf("the transcript holds boundary=%v summary=%v, want both", boundarySeen, summarySeen)
	}

	// The tool pair: one assistant record holding the call, and one user record holding the result under
	// the same id.
	var callID, callMessageID, resultID string
	for _, record := range records {
		for _, block := range recordBlocks(record) {
			switch block["type"] {
			case claudeBlockToolUse:
				callID, _ = block["id"].(string)
				callMessageID, _ = record["message"].(map[string]any)["id"].(string)
				if block["name"] != "shell" {
					t.Errorf("the tool call's name is %v, want the source's own %q", block["name"], "shell")
				}
				if input, ok := block["input"].(map[string]any); !ok || input["command"] == nil {
					t.Errorf("the tool call holds no recorded input object: %v", block["input"])
				}
				if record["type"] != claudeTypeAssistant {
					t.Errorf("a tool call is on a %v record, want an assistant record", record["type"])
				}
			case claudeBlockToolResult:
				resultID, _ = block["tool_use_id"].(string)
				if content, _ := block["content"].(string); content == "" {
					t.Errorf("the result block holds no text: %v", block)
				}
				if record["type"] != claudeTypeUser {
					t.Errorf("a tool result is on a %v record, want a user record", record["type"])
				}
			}
		}
	}
	if callID == "" || resultID != callID {
		t.Errorf("the file pairs call %q with result %q, want the same id", callID, resultID)
	}

	// The message id is the grouping both readers rebuild one message from: the records of one source
	// message share it, and records of different messages carry different ones. The fixture records the
	// `shell` call and the assistant text as two different source messages (a `function_call` item and
	// the later assistant message), so an id shared here would be the writer merging what the source
	// kept apart. The shared-id half of the rule is pinned on a message that carries both components in
	// TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers.
	grouped := assistantRecordsByMessageID(t, records)
	textMessageID := assistantMessageIDWithText(t, records, "It is set in internal/retry/config.go.")
	if got := len(grouped[textMessageID]); got != 1 {
		t.Errorf("the assistant text's message id groups %d assistant record(s), want its own record alone", got)
	}
	if got := len(grouped[callMessageID]); got != 1 {
		t.Errorf("the tool call's message id groups %d assistant record(s), want the call's own record alone: "+
			"the source recorded the call as a message of its own", got)
	}
	if lastMessageID := assistantMessageIDWithText(t, records, "Four attempts, in internal/retry/policy.go."); lastMessageID == textMessageID || lastMessageID == callMessageID {
		t.Errorf("two of the fixture's three assistant messages carry message id %q", lastMessageID)
	}
}

// assistantRecordsByMessageID groups the assistant records by the message id each carries, and fails on
// a record without one: the id is what tells both readers which records are one message, so a record
// missing it is one no read of the file can put back where the writer meant it.
func assistantRecordsByMessageID(t *testing.T, records []map[string]any) map[string][]map[string]any {
	t.Helper()
	grouped := map[string][]map[string]any{}
	for _, record := range records {
		if record["type"] != claudeTypeAssistant {
			continue
		}
		message, _ := record["message"].(map[string]any)
		id, _ := message["id"].(string)
		if id == "" {
			t.Fatalf("an assistant record carries no message id: %v", record)
		}
		grouped[id] = append(grouped[id], record)
	}
	return grouped
}

// assistantMessageIDWithText answers the message id of the assistant record whose text is want.
func assistantMessageIDWithText(t *testing.T, records []map[string]any, want string) string {
	t.Helper()
	for _, record := range records {
		if record["type"] != claudeTypeAssistant {
			continue
		}
		for _, block := range messageBlocks(t, record) {
			if block["type"] == claudeBlockText && block["text"] == want {
				message, _ := record["message"].(map[string]any)
				id, _ := message["id"].(string)
				return id
			}
		}
	}
	t.Fatalf("no assistant record holds the text %q", want)
	return ""
}

// TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers pins the two halves of the disclosure rule: the
// report lists every category it knows about (so a consumer never infers what was left out), and the
// records the file holds are exactly the ones the counts say were carried.
func TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "Where is the retry cap configured?",
			Dialogue: "Where is the retry cap configured?", DialogueProjected: true, Timestamp: time.Unix(1000, 0)},
		Message{Ordinal: 1, Role: "assistant", Content: "At the top of config.go. [Read: /x]",
			Dialogue: "At the top of config.go.", DialogueProjected: true, Thinking: "let me look",
			Model: "gpt-5.6-luna", Timestamp: time.Unix(1001, 0),
			ToolCalls: []ToolCall{
				{ToolUseID: "toolu_a", Name: "shell",
					InputJSON: `{"command":  "grep -rn cap internal/retry"}`},
				{ToolUseID: "toolu_b", Name: "shell", InputJSON: `{"command":"cat /dev/null"}`},
			}},
		Message{Ordinal: 2, Role: "user", Content: "", Timestamp: time.Unix(1002, 0),
			ToolResults: []ToolResult{
				{ToolUseID: "toolu_a", ContentRaw: "config.go:41: cap = 30", ContentDecoded: true,
					Status: "errored"},
				{ToolUseID: "toolu_missing", ContentRaw: "nowhere", ContentDecoded: true},
				{ToolUseID: "toolu_b", ContentRaw: "   \n  ", ContentDecoded: true},
			}},
		Message{Ordinal: 3, Role: "assistant", Content: "Four attempts.", DialogueProjected: true,
			IsSidechain: true, Timestamp: time.Unix(1003, 0)},
		Message{Ordinal: 4, Role: "system", Content: "provider notice", IsSystem: true,
			Timestamp: time.Unix(1004, 0)},
		Message{Ordinal: 5, Role: "tool", Content: "raw tool output", Timestamp: time.Unix(1005, 0)},
		Message{Ordinal: 6, Role: "user", Content: "   ", Dialogue: "   ", DialogueProjected: true,
			Timestamp: time.Unix(1006, 0)},
		Message{Ordinal: 7, Role: "assistant", Content: "It is set in policy.go.",
			Dialogue: "It is set in policy.go.", DialogueProjected: true, Timestamp: time.Unix(1007, 0)},
		Message{Ordinal: 8, Role: "assistant", Content: "Compacted summary", IsCompactBoundary: true,
			Timestamp: time.Unix(1008, 0)},
	)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the document: %v", err)
	}
	// Every category is reported on every run, so each one is read through lossCount: a category the
	// report dropped is a failure here, not a zero that reads like a real count.
	for _, category := range []string{
		"reasoning_text", "sidechain_messages", "system_records", "tool_role_messages",
		"unclassified_role_messages", "empty_user_messages", "non_dialogue_content",
		"whitespace_only_dialogue", "unpaired_tool_results", "empty_tool_results",
		"whitespace_only_tool_results", "unrepresentable_result_blocks", "unresolved_tool_outputs",
		"tool_result_statuses", "empty_tool_inputs", "generated_call_ids", "coalesced_tool_call_projections",
		"tool_name_vocabulary",
		"message_model_attribution", "pre_compaction_history", "empty_compaction_summary",
		"malformed_source_lines", "summary_transcript", "source_needs_retry",
	} {
		lossCount(t, transcript.Losses, category)
	}
	for _, testCase := range []struct {
		category string
		want     int
	}{
		{"reasoning_text", 1},
		{"sidechain_messages", 1},
		{"system_records", 1},
		{"tool_role_messages", 1},
		{"whitespace_only_dialogue", 1},
		{"unpaired_tool_results", 1},
		{"whitespace_only_tool_results", 1},
		{"message_model_attribution", 1},
		{"tool_name_vocabulary", 1},
		{"pre_compaction_history", 3},
		{"empty_user_messages", 0},
		{"summary_transcript", 0},
	} {
		if got := lossCount(t, transcript.Losses, testCase.category).Count; got != testCase.want {
			t.Errorf("the report counts %s = %d, want %d (%+v)",
				testCase.category, got, testCase.want, lossCount(t, transcript.Losses, testCase.category))
		}
	}
	if got := lossCount(t, transcript.Losses, "tool_result_statuses").Count; got != 0 {
		// The one status this document records says the call failed, which the target's block can say.
		t.Errorf("a failed status was counted as unrepresentable: %d", got)
	}

	// The file holds no record for anything counted as omitted: a reasoning trace, a sidechain turn, a
	// system notice, a tool-role message, whitespace-only text.
	for _, record := range decodeTranscript(t, transcript) {
		for _, block := range recordBlocks(record) {
			if block["type"] == "thinking" {
				t.Errorf("the transcript holds a reasoning block: %v", block)
			}
			if text, _ := block["text"].(string); text != "" && strings.TrimSpace(text) == "" {
				t.Errorf("the transcript holds whitespace-only text: %q", text)
			}
			if content, _ := block["content"].(string); content != "" && strings.TrimSpace(content) == "" {
				t.Errorf("the transcript holds a whitespace-only tool result: %q", content)
			}
		}
		if record["isSidechain"] == true {
			t.Errorf("the transcript holds a sidechain record: %v", record)
		}
	}
	// The failed result carries the target's own field for it, and the call's input is the compacted JSON
	// the encoder writes rather than the source's own spacing.
	var sawFailedResult bool
	for _, record := range decodeTranscript(t, transcript) {
		for _, block := range recordBlocks(record) {
			if block["type"] == claudeBlockToolResult && block["tool_use_id"] == "toolu_a" {
				sawFailedResult = block["is_error"] == true
			}
			if block["type"] == claudeBlockToolUse {
				if got, _ := block["input"].(map[string]any); got == nil {
					t.Fatalf("the call's input is not an object: %v", block["input"])
				}
			}
		}
	}
	if !sawFailedResult {
		t.Error("a result whose recorded status says the call failed was not written with is_error")
	}
	if transcript.Carried.ToolCalls != 2 || transcript.Carried.ToolResults != 1 {
		t.Errorf("carried %d call(s) and %d result(s), want two calls and the one result that had text",
			transcript.Carried.ToolCalls, transcript.Carried.ToolResults)
	}
	if transcript.Carried.Messages != 3 || transcript.Carried.UserMessages != 1 ||
		transcript.Carried.AssistantMessages != 2 {
		t.Errorf("carried %+v, want three messages (one user, two assistant)", transcript.Carried)
	}
	if transcript.Carried.CompactionBoundaries != 1 {
		t.Errorf("carried %d boundary(ies), want one", transcript.Carried.CompactionBoundaries)
	}

	// The message id is the grouping both readers rebuild one message from: the one source message that
	// carries dialogue and two calls became three assistant records, and all three share exactly one id,
	// while the other message's records carry a different one. A writer that reused an id across
	// messages would merge them; one that generated an id per record would split this message.
	records := decodeTranscript(t, transcript)
	grouped := assistantRecordsByMessageID(t, records)
	sharedID := assistantMessageIDWithText(t, records, "At the top of config.go.")
	if got := len(grouped[sharedID]); got != 3 {
		t.Errorf("the message carrying the dialogue and two calls is %d assistant record(s) under message id %q, want 3",
			got, sharedID)
	}
	for _, record := range grouped[sharedID] {
		blocks := recordBlocks(record)
		if len(blocks) != 1 {
			t.Errorf("a record of the dialogue-and-calls message holds %d block(s), want one: %v", len(blocks), blocks)
			continue
		}
		if kind := blocks[0]["type"]; kind != claudeBlockText && kind != claudeBlockToolUse {
			t.Errorf("a record of the dialogue-and-calls message holds a %v block", kind)
		}
	}
	if otherID := assistantMessageIDWithText(t, records, "It is set in policy.go."); otherID == sharedID {
		t.Errorf("two different messages carry the same message id %q", sharedID)
	}
}

// TestClaudeWriterReportsACompactionBoundaryWhoseSummaryHoldsNoText pins the disclosure for the one
// boundary shape whose summary cannot travel as text: the inherited Codex reader trims a compacted
// record's summary (internal/parser's handleCompacted), so a source summary of whitespace alone -- or a
// remote compaction that recorded none -- arrives as no text. The target's boundary pair is still
// written, because the pair is what makes the target apply the boundary, and the summary record and the
// report both say that no summary text traveled rather than presenting the boundary as a carried one.
func TestClaudeWriterReportsACompactionBoundaryWhoseSummaryHoldsNoText(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true},
		// The shape the reader produces for a `compacted` record whose message is whitespace-only (or
		// absent): a system boundary message with no content.
		Message{Ordinal: 1, Role: "assistant", IsSystem: true, IsCompactBoundary: true},
	)
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the document: %v", err)
	}
	if loss := lossCount(t, transcript.Losses, "empty_compaction_summary"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("empty_compaction_summary = %+v, want the summary that holds no text counted as omitted", loss)
	}
	if transcript.Carried.CompactionBoundaries != 1 {
		t.Errorf("carried = %+v, want the boundary still written", transcript.Carried)
	}
	var boundarySeen, summarySeen bool
	for _, record := range decodeTranscript(t, transcript) {
		if record["type"] != claudeTypeSystem || record["subtype"] != claudeSubtypeCompactBoundary {
			continue
		}
		boundarySeen = true
		for _, block := range recordBlocks(record) {
			if block["type"] == claudeBlockText {
				t.Errorf("the boundary marker holds text: %v", block)
			}
		}
	}
	for _, record := range decodeTranscript(t, transcript) {
		if record["isCompactSummary"] != true {
			continue
		}
		summarySeen = true
		blocks := messageBlocks(t, record)
		if len(blocks) != 1 || blocks[0]["type"] != claudeBlockText || blocks[0]["text"] != "" {
			t.Errorf("the summary record holds %v, want one empty text block: the summary text did not travel", blocks)
		}
	}
	if !boundarySeen || !summarySeen {
		t.Errorf("the transcript holds boundary=%v summary=%v, want both: the pair is what applies the boundary",
			boundarySeen, summarySeen)
	}
}

// TestClaudeWriterWritesAnArchiveSummaryOnlyForACallWithoutResults pins the archive rule for this
// target, which is the Codex writer's rule: the archive keeps a call's outcome as the result events it
// recorded on the call and holds their derived summary in the column beside them (internal/db restores
// the sole event's text into that column, or stores the aggregate it derives from several events). A
// summary is therefore a view of the recorded evidence, and a conversion writes it only when the call has
// no recorded result of its own -- the shape of an archive row whose events were not written -- where it
// is the only evidence of that outcome. Writing it beside its own events would present a display
// aggregate as a separately recorded tool output, and omitting it altogether would drop the only outcome
// the read holds while the structure check passed over the omission.
func TestClaudeWriterWritesAnArchiveSummaryOnlyForACallWithoutResults(t *testing.T) {
	// The archive shape: the outcome is folded onto the call's own message, so the events sit beside the
	// calls they answer (messageFromArchive), and the call's summary is the column the archive holds.
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "Ask", Dialogue: "Ask", DialogueProjected: true},
		Message{Ordinal: 1, Role: "assistant", Content: "[a batch of calls]",
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
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	outputs := []string{}
	for _, record := range decodeTranscript(t, transcript) {
		for _, block := range recordBlocks(record) {
			if block["type"] != claudeBlockToolResult {
				continue
			}
			content, _ := block["content"].(string)
			id, _ := block["tool_use_id"].(string)
			outputs = append(outputs, id+": "+content)
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
	if strings.Join(outputs, " | ") != strings.Join(want, " | ") || transcript.Carried.ToolResults != 4 {
		t.Errorf("tool_result blocks = %q (carried %d results), want the recorded results once each and "+
			"the summary only where no result was recorded: %q", outputs, transcript.Carried.ToolResults, want)
	}
	if got := lossCount(t, transcript.Losses, "empty_tool_results").Count; got != 0 {
		t.Errorf("empty_tool_results = %d, want 0: the summary-only outcome held text and was written", got)
	}

	// The written summary is in the item list too, so the check this conversion owes its caller reads it
	// back from the installed file rather than passing over an outcome the file does not hold.
	home := t.TempDir()
	if _, _, err := InstallClaudeTranscript(transcript, home); err != nil {
		t.Fatalf("installing: %v", err)
	}
	verification, err := VerifyClaudeTranscriptStructure(context.Background(),
		filepath.Join(home, claudeProjectsSubdir), transcript, fixtureConfig(t))
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file to read back as what was written", verification)
	}
	if verification.ReadBack.ToolResults != 4 {
		t.Errorf("read-back = %+v, want the summary-only outcome read back beside the recorded results",
			verification.ReadBack)
	}
}

// TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory pins the one source shape this
// target cannot install: the loader finds a session through the directory it is run from, so a session
// with no recorded working directory could never be resumed while a report called it installed.
func TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true})
	conversation.Source.Cwd = ""
	if _, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{}); err != ErrNoTargetCwd {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
}

// TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall pins the pairing rule: an id-less result
// may be paired while exactly one carried call is unanswered, and never when the choice would be a guess.
func TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall(t *testing.T) {
	paired := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "calling", DialogueProjected: false,
			ToolCalls: []ToolCall{{Name: "shell", InputJSON: `{"command":"ls"}`}}},
		Message{Ordinal: 1, Role: "user", ToolResults: []ToolResult{{ContentRaw: "output", ContentDecoded: true}}},
	)
	transcript, err := ConvertToClaudeTranscript(paired, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if got := lossCount(t, transcript.Losses, "generated_call_ids").Count; got != 2 {
		t.Errorf("generated_call_ids = %d, want 2 (the call's id and the result's pairing)", got)
	}
	if got := lossCount(t, transcript.Losses, "unpaired_tool_results").Count; got != 0 {
		t.Errorf("unpaired_tool_results = %d, want 0: one call was outstanding", got)
	}
	if transcript.Carried.ToolResults != 1 {
		t.Errorf("carried %d result(s), want 1", transcript.Carried.ToolResults)
	}

	ambiguous := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "calling twice",
			ToolCalls: []ToolCall{{Name: "shell", InputJSON: `{}`}, {Name: "shell", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "user", ToolResults: []ToolResult{{ContentRaw: "output", ContentDecoded: true}}},
	)
	transcript, err = ConvertToClaudeTranscript(ambiguous, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if got := lossCount(t, transcript.Losses, "unpaired_tool_results").Count; got != 1 {
		t.Errorf("unpaired_tool_results = %d, want 1: two calls were outstanding and the pairing would be a guess", got)
	}
}

// readFixtureConversation reads one committed fixture through the inherited reader, which is the document
// a conversion really receives: a test that hand-built one would not exercise the shape the provider
// produces.
func readFixtureConversation(t *testing.T, ref Ref) Conversation {
	t.Helper()
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), ref)
	if err != nil {
		t.Fatalf("reading %s:%s: %v", ref.Provider, ref.ID, err)
	}
	return conversation
}
