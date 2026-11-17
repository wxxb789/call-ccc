package ccc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// piLine is one decoded line of a written Pi session, with its payload left raw so a test can assert on
// the exact fields the target reads.
type piLine struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
	Summary   string          `json:"summary"`
}

// decodePiSession decodes every line of a written session, failing when a line is not a JSON object:
// the target reads JSONL, so a line it cannot parse is a defect however plausible the writer's intent
// was.
func decodePiSession(t *testing.T, data []byte) []piLine {
	t.Helper()
	text := strings.TrimSpace(string(data))
	if text == "" {
		t.Fatal("the session is empty")
	}
	var lines []piLine
	for index, line := range strings.Split(text, "\n") {
		var decoded piLine
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d is not a JSON object: %v", index+1, err)
		}
		if decoded.Type == "" {
			t.Fatalf("line %d carries no type: %s", index+1, line)
		}
		lines = append(lines, decoded)
	}
	return lines
}

// piFixtureConversation is the document every writer case starts from: two turns, a tool call answered
// by its result, and a persisted reasoning trace beside the second turn's dialogue.
func piFixtureConversation() Conversation {
	return Conversation{
		Source: SourceIdentity{Provider: "claude", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Content: "Check the retry policy.",
				Dialogue: "Check the retry policy.", Timestamp: time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "I will read it.",
				Dialogue: "I will read it.", Thinking: "Read the policy file first.",
				Model: "claude-sonnet-4-5", Timestamp: time.Date(2026, 5, 5, 10, 0, 1, 0, time.UTC)},
			{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "[Read: /work/project/policy.go]",
				DialogueProjected: true, Model: "claude-sonnet-4-5",
				Timestamp: time.Date(2026, 5, 5, 10, 0, 2, 0, time.UTC),
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read",
					InputJSON: `{"file_path":"/work/project/policy.go"}`}}},
			{Ordinal: 3, Role: string(parser.RoleUser), Timestamp: time.Date(2026, 5, 5, 10, 0, 3, 0, time.UTC),
				ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: `"const MaxAttempts = 4\n"`, ContentSize: 22}}},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
}

// TestPiSessionRecordShape pins the entries, their order and their fields: the target reads this file,
// so every entry's shape is a contract with it rather than a local preference. The measured facts it
// carries are the header's own members (Pi's loader reads its first line for the session id and the
// working directory), the tree link every entry needs (`id`/`parentId`, which the harness's own
// `buildSessionPath` walks from the last entry), and the `message.usage` object Pi's loader indexes
// into.
func TestPiSessionRecordShape(t *testing.T) {
	session, err := ConvertToPiSession(piFixtureConversation(), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodePiSession(t, session.Data)

	if lines[0].Type != "session" {
		t.Fatalf("the first line is a %s, not the session header", lines[0].Type)
	}
	var header struct {
		Version   int    `json:"version"`
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Cwd       string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(string(session.Data), "\n", 2)[0]), &header); err != nil {
		t.Fatalf("decoding the header: %v", err)
	}
	if header.Version != PiSessionVersion {
		t.Errorf("version = %d, want the current session version %d", header.Version, PiSessionVersion)
	}
	if header.ID != session.SessionID {
		t.Errorf("header id = %q, want the document's own session id %q", header.ID, session.SessionID)
	}
	if header.Cwd != "/work/project" {
		t.Errorf("header cwd = %q, want the source's recorded working directory", header.Cwd)
	}
	if header.Timestamp == "" {
		t.Error("the header carries no timestamp")
	}
	if session.Directory != "--work-project--" {
		t.Errorf("directory = %q, want Pi's own spelling of the working directory", session.Directory)
	}
	if !strings.HasSuffix(session.FileName, "_"+session.SessionID+".jsonl") {
		t.Errorf("file name = %q, want <timestamp>_<session id>.jsonl", session.FileName)
	}

	// The tree: the first entry has no parent, every later one points at the entry before it. A broken
	// chain loads and then loses everything before the break, so it is the file's whole conversation.
	if lines[1].ParentID != nil {
		t.Errorf("the first entry's parentId = %v, want null", *lines[1].ParentID)
	}
	for index := 2; index < len(lines); index++ {
		if lines[index].ParentID == nil {
			t.Fatalf("entry %d carries no parent", index)
		}
		if *lines[index].ParentID != lines[index-1].ID {
			t.Errorf("entry %d chained to %q, want the entry before it %q",
				index, *lines[index].ParentID, lines[index-1].ID)
		}
	}

	var roles []string
	for _, line := range lines[1:] {
		if line.Type != "message" {
			continue
		}
		var message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			Usage   *struct {
				Input  int `json:"input"`
				Output int `json:"output"`
				Cost   *struct {
					Total float64 `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
			StopReason string `json:"stopReason"`
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("decoding a %s message: %v", line.Type, err)
		}
		roles = append(roles, message.Role)
		switch message.Role {
		case "assistant":
			if message.Usage == nil {
				t.Errorf("an assistant message carries no usage object: measured, the target's loader " +
					"indexes into message.usage for every assistant message it reads")
			}
			if message.StopReason == "" {
				t.Errorf("an assistant message carries no stopReason")
			}
		case "toolResult":
			if message.ToolCallID != "toolu_1" {
				t.Errorf("the tool result names call %q, want the recorded %q", message.ToolCallID, "toolu_1")
			}
			if message.ToolName != "Read" {
				t.Errorf("the tool result names tool %q, want the recorded %q", message.ToolName, "Read")
			}
		}
	}
	want := []string{"user", "assistant", "assistant", "toolResult"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("roles = %v, want %v", roles, want)
	}

	// The call block is inside the message that made it, which is how the target records one.
	callEntry := lines[3]
	var callMessage struct {
		Content []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"content"`
	}
	if err := json.Unmarshal(callEntry.Message, &callMessage); err != nil {
		t.Fatalf("decoding the call message: %v", err)
	}
	if len(callMessage.Content) != 1 || callMessage.Content[0].Type != "toolCall" {
		t.Fatalf("the call message's content = %s, want one toolCall block",
			string(callEntry.Message))
	}
	if callMessage.Content[0].ID != "toolu_1" || callMessage.Content[0].Name != "Read" {
		t.Errorf("the call block = %+v, want the recorded id and name", callMessage.Content[0])
	}
	if compactJSON(string(callMessage.Content[0].Arguments)) != `{"file_path":"/work/project/policy.go"}` {
		t.Errorf("the call arguments = %s, want the recorded input object compacted",
			string(callMessage.Content[0].Arguments))
	}
}

// TestPiSessionCarriesReasoningAsItsOwnBlock pins the rule this route does not share with the Claude
// one: Pi's `thinking` block is the target format's own shape for a persisted trace and carries no
// signature this build would have to forge, so the trace travels as the source recorded it and the
// report says it did (a caveat, not an omission).
func TestPiSessionCarriesReasoningAsItsOwnBlock(t *testing.T) {
	session, err := ConvertToPiSession(piFixtureConversation(), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if !strings.Contains(string(session.Data), `{"type":"thinking","thinking":"Read the policy file first."}`) {
		t.Error("the written session holds no thinking block for the source's reasoning trace")
	}
	loss := lossCount(t, session.Losses, "reasoning_text")
	if loss.Count != 1 {
		t.Errorf("reasoning_text count = %d, want the one message that carried a trace", loss.Count)
	}
	if loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("reasoning_text disposition = %q, want %q: the trace is carried", loss.Disposition, DispositionCarriedWithCaveat)
	}
}

// TestPiSessionEntryIDsAreDistinct pins the identity of the tree: two entries sharing an id collapse the
// path a loader walks, and a v7's leading bits are a millisecond clock, so entries written in the same
// millisecond would share theirs.
func TestPiSessionEntryIDsAreDistinct(t *testing.T) {
	long := piFixtureConversation()
	for index := range 40 {
		long.Messages = append(long.Messages, Message{
			Ordinal:   4 + index,
			Role:      string(parser.RoleUser),
			Content:   "another turn",
			Dialogue:  "another turn",
			Timestamp: time.Date(2026, 5, 5, 10, 1, index, 0, time.UTC),
		})
	}
	session, err := ConvertToPiSession(long, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range decodePiSession(t, session.Data)[1:] {
		if seen[line.ID] {
			t.Fatalf("entry id %q is used twice", line.ID)
		}
		seen[line.ID] = true
		if len(line.ID) != 8 {
			t.Errorf("entry id %q is not eight characters", line.ID)
		}
	}
}

// TestPiSessionCarriesACompactionBoundaryAsTheTargetsOwnRecord pins what a compacted source becomes: a
// `compaction` entry whose `firstKeptEntryId` is its own id (the target's own retain-none shape: the
// source records the summary and the boundary, not which entries the boundary kept, so claiming a kept
// range would invent one), a null checkpoint (the target installs its own prompt at request time), and
// the pre-boundary turns still in the file where the target's own compaction leaves them.
func TestPiSessionCarriesACompactionBoundaryAsTheTargetsOwnRecord(t *testing.T) {
	conversation := piFixtureConversation()
	conversation.Messages = append(conversation.Messages,
		Message{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "The user asked about retries.",
			IsSystem: true, IsCompactBoundary: true,
			Timestamp: time.Date(2026, 5, 5, 10, 5, 0, 0, time.UTC)},
		Message{Ordinal: 5, Role: string(parser.RoleUser), Content: "And the attempt limit?",
			Dialogue: "And the attempt limit?", Timestamp: time.Date(2026, 5, 5, 10, 6, 0, 0, time.UTC)},
	)
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	var boundary *piLine
	for index := range lines {
		if lines[index].Type == "compaction" {
			boundary = &lines[index]
			break
		}
	}
	if boundary == nil {
		t.Fatalf("no compaction entry was written:\n%s", session.Data)
	}
	var payload struct {
		Summary          string          `json:"summary"`
		FirstKeptEntryID string          `json:"firstKeptEntryId"`
		SystemMessage    json.RawMessage `json:"systemMessage"`
	}
	if err := json.Unmarshal([]byte(rawLine(t, session.Data, boundary.ID)), &payload); err != nil {
		t.Fatalf("decoding the compaction entry: %v", err)
	}
	if payload.Summary != "The user asked about retries." {
		t.Errorf("summary = %q, want the source's own summary text", payload.Summary)
	}
	if payload.FirstKeptEntryID != boundary.ID {
		t.Errorf("firstKeptEntryId = %q, want the entry's own id %q (the target's retain-none shape)",
			payload.FirstKeptEntryID, boundary.ID)
	}
	if string(payload.SystemMessage) != "null" {
		t.Errorf("systemMessage = %s, want an explicit null: this build has no checkpoint to write",
			payload.SystemMessage)
	}
	if !strings.Contains(string(session.Data), "Check the retry policy.") {
		t.Error("the pre-boundary turns are not in the file: the target's own compaction keeps them")
	}
	// Three dialogue entries stand before the boundary (two user turns and one assistant turn; the
	// fourth source message is the tool result, which is tool evidence rather than a message entry).
	// The count is of the entry stream the boundary replaced, which is what the target rebuilds its
	// context from.
	preBoundary := lossCount(t, session.Losses, "pre_compaction_history")
	if preBoundary.Count != 3 {
		t.Errorf("pre_compaction_history = %d, want the three messages carried before the boundary", preBoundary.Count)
	}
}

// rawLine answers one entry's own bytes by its id, for a test that needs the object as the file holds it.
func rawLine(t *testing.T, data []byte, id string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, `"id":"`+id+`"`) {
			return line
		}
	}
	t.Fatalf("no entry with id %q in the session", id)
	return ""
}

// TestPiSessionOmitsWhatItCannotExpress pins the omission side of the contract: a sidechain (a subagent's
// turn in the same file, which the target's own loader keeps out of the session's context), an
// unclassified role, and an assistant record with nothing in it are all left out and counted, never
// written as a turn of the conversation.
func TestPiSessionOmitsWhatItCannotExpress(t *testing.T) {
	conversation := piFixtureConversation()
	conversation.Messages = append(conversation.Messages,
		Message{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "subagent said this",
			Dialogue: "subagent said this", IsSidechain: true,
			Timestamp: time.Date(2026, 5, 5, 10, 7, 0, 0, time.UTC)},
		Message{Ordinal: 5, Role: "somefuturerole", Content: "an unrecognised record",
			Dialogue: "an unrecognised record", Timestamp: time.Date(2026, 5, 5, 10, 8, 0, 0, time.UTC)},
		Message{Ordinal: 6, Role: string(parser.RoleAssistant),
			Timestamp: time.Date(2026, 5, 5, 10, 9, 0, 0, time.UTC)},
	)
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	for _, text := range []string{"subagent said this", "an unrecognised record"} {
		if strings.Contains(string(session.Data), text) {
			t.Errorf("the file holds %q, which this route must not write as a turn", text)
		}
	}
	for _, category := range []string{"sidechain_messages", "unclassified_role_messages", "empty_assistant_messages"} {
		if loss := lossCount(t, session.Losses, category); loss.Count != 1 {
			t.Errorf("%s = %d, want 1", category, loss.Count)
		}
	}
}

// TestPiSessionReportsEveryLossCategory pins the report's own completeness rule: every category is
// listed on every run, with its count and its disposition, so a caller never has to infer what a run
// left out. A category that disappeared from the report would make a loss invisible.
func TestPiSessionReportsEveryLossCategory(t *testing.T) {
	session, err := ConvertToPiSession(piFixtureConversation(), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	dispositions := map[string]bool{
		DispositionOmitted:           true,
		DispositionCarriedWithCaveat: true,
	}
	wanted := []string{
		"reasoning_text", "sidechain_messages", "system_records", "tool_role_messages",
		"unclassified_role_messages", "empty_user_messages", "empty_assistant_messages",
		"non_dialogue_content", "whitespace_only_dialogue", "unpaired_tool_results",
		"empty_tool_results", "whitespace_only_tool_results", "unrepresentable_result_blocks",
		"unresolved_tool_outputs", "unrecorded_token_usage", "tool_result_statuses",
		"empty_tool_inputs", "generated_call_ids", "coalesced_tool_call_projections",
		"subagent_session_links", "tool_name_vocabulary", "message_model_attribution",
		"pre_compaction_history", "empty_compaction_summary", "malformed_source_lines",
		"summary_transcript", "source_needs_retry",
	}
	for _, category := range wanted {
		loss := lossCount(t, session.Losses, category)
		if !dispositions[loss.Disposition] {
			t.Errorf("%s has disposition %q, which is not a disposition this report uses",
				category, loss.Disposition)
		}
		if strings.TrimSpace(loss.Detail) == "" {
			t.Errorf("%s carries no detail, so a caller cannot read what the count means", category)
		}
	}
}

// TestPiSessionRefusesASourceWithoutAWorkingDirectory pins the refusal: the target resolves a session id
// through the project directory spelled from the process's own working directory, so a session installed
// without one could never be resumed while a report called it installed.
func TestPiSessionRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	conversation := piFixtureConversation()
	conversation.Source.Cwd = "   "
	if _, err := ConvertToPiSession(conversation, PiTargetOptions{}); err == nil {
		t.Fatal("a source that records no working directory was accepted")
	} else if err != ErrNoTargetCwd {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
}

// TestPiProjectDirMatchesTheTargetsOwnSpelling pins the spelling the installer files a session under
// against the code the installed harness runs, measured on the target host.
func TestPiProjectDirMatchesTheTargetsOwnSpelling(t *testing.T) {
	cases := map[string]string{
		"/home/azureuser/.hermes/cache/scratch": "--home-azureuser-.hermes-cache-scratch--",
		"/tmp/pi-probe3-abc/work":               "--tmp-pi-probe3-abc-work--",
		"/work/project":                         "--work-project--",
	}
	for cwd, want := range cases {
		if got := PiProjectDir(cwd); got != want {
			t.Errorf("PiProjectDir(%q) = %q, want %q", cwd, got, want)
		}
	}
}
