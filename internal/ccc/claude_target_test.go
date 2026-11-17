package ccc

import (
	"context"
	"encoding/json"
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

// recordsOfType answers the transcript's records of one type, in file order.
func recordsOfType(records []map[string]any, kind string) []map[string]any {
	var found []map[string]any
	for _, record := range records {
		if record["type"] == kind {
			found = append(found, record)
		}
	}
	return found
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
	var callID, resultID string
	for _, record := range records {
		for _, block := range recordBlocks(record) {
			switch block["type"] {
			case claudeBlockToolUse:
				callID, _ = block["id"].(string)
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

	// The assistant text and its call are two records of one message: the target's readers merge records
	// that share a message id, and a writer that reused one across messages would merge those instead.
	messageIDs := map[string][]map[string]any{}
	for _, record := range records {
		if record["type"] != claudeTypeAssistant {
			continue
		}
		message, _ := record["message"].(map[string]any)
		id, _ := message["id"].(string)
		if id == "" {
			t.Fatalf("an assistant record carries no message id: %v", record)
		}
		messageIDs[id] = append(messageIDs[id], record)
	}
	if _, ok := messageIDs[assistantMessageIDWithText(t, records, "It is set in internal/retry/config.go.")]; !ok {
		t.Error("the assistant text and its tool call do not share a message id")
	}
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

// handWrittenConversation builds a document with the shapes one conversion has to name, so each rule is
// asserted against a source that really holds it.
func handWrittenConversation(messages ...Message) Conversation {
	return Conversation{
		Source:   SourceIdentity{Provider: "claude", SessionID: "source-1", Cwd: "/home/user/project"},
		Messages: messages,
	}
}

// TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers pins the two halves of the disclosure rule: the
// report lists every category it knows about (so a consumer never infers what was left out), and the
// records the file holds are exactly the ones the counts say were carried.
func TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers(t *testing.T) {
	conversation := handWrittenConversation(
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
	counts := lossCounts(transcript.Losses)
	for _, category := range []string{
		"reasoning_text", "sidechain_messages", "system_records", "tool_role_messages",
		"unclassified_role_messages", "empty_user_messages", "non_dialogue_content",
		"whitespace_only_dialogue", "unpaired_tool_results", "empty_tool_results",
		"whitespace_only_tool_results", "unrepresentable_result_blocks", "unresolved_tool_outputs",
		"tool_result_statuses", "empty_tool_inputs", "generated_call_ids", "tool_name_vocabulary",
		"message_model_attribution", "pre_compaction_history", "malformed_source_lines",
		"summary_transcript", "source_needs_retry",
	} {
		if _, ok := counts[category]; !ok {
			t.Errorf("the report does not list the category %q", category)
		}
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
		if got := counts[testCase.category]; got != testCase.want {
			t.Errorf("the report counts %s = %d, want %d (%v)",
				testCase.category, got, testCase.want, lossDetail(transcript.Losses, testCase.category))
		}
	}
	if got := counts["tool_result_statuses"]; got != 0 {
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
}

// TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory pins the one source shape this
// target cannot install: the loader finds a session through the directory it is run from, so a session
// with no recorded working directory could never be resumed while a report called it installed.
func TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	conversation := handWrittenConversation(
		Message{Ordinal: 0, Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true})
	conversation.Source.Cwd = ""
	if _, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{}); err != ErrNoTargetCwd {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
}

// TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall pins the pairing rule: an id-less result
// may be paired while exactly one carried call is unanswered, and never when the choice would be a guess.
func TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall(t *testing.T) {
	paired := handWrittenConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "calling", DialogueProjected: false,
			ToolCalls: []ToolCall{{Name: "shell", InputJSON: `{"command":"ls"}`}}},
		Message{Ordinal: 1, Role: "user", ToolResults: []ToolResult{{ContentRaw: "output", ContentDecoded: true}}},
	)
	transcript, err := ConvertToClaudeTranscript(paired, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	counts := lossCounts(transcript.Losses)
	if counts["generated_call_ids"] != 2 {
		t.Errorf("generated_call_ids = %d, want 2 (the call's id and the result's pairing)", counts["generated_call_ids"])
	}
	if counts["unpaired_tool_results"] != 0 {
		t.Errorf("unpaired_tool_results = %d, want 0: one call was outstanding", counts["unpaired_tool_results"])
	}
	if transcript.Carried.ToolResults != 1 {
		t.Errorf("carried %d result(s), want 1", transcript.Carried.ToolResults)
	}

	ambiguous := handWrittenConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "calling twice",
			ToolCalls: []ToolCall{{Name: "shell", InputJSON: `{}`}, {Name: "shell", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "user", ToolResults: []ToolResult{{ContentRaw: "output", ContentDecoded: true}}},
	)
	transcript, err = ConvertToClaudeTranscript(ambiguous, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if got := lossCounts(transcript.Losses)["unpaired_tool_results"]; got != 1 {
		t.Errorf("unpaired_tool_results = %d, want 1: two calls were outstanding and the pairing would be a guess", got)
	}
}

// lossCounts indexes a report's categories by name.
func lossCounts(losses []Loss) map[string]int {
	counts := make(map[string]int, len(losses))
	for _, loss := range losses {
		counts[loss.Category] = loss.Count
	}
	return counts
}

// lossDetail answers one category's detail, for a failure message that says why the count is what it is.
func lossDetail(losses []Loss, category string) string {
	for _, loss := range losses {
		if loss.Category == category {
			return loss.Detail
		}
	}
	return "the category is absent"
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
