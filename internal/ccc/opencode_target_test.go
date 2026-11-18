package ccc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	// Every assistant message written carries the completion instant the importer requires (measured:
	// without it the harness accepts the document and drops the message), and the report names that the
	// source answered no span for it.
	openCodeRequireLoss(t, session, "assistant_completion_instant", carried.AssistantMessages)
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
	blocks := parts["call-blocks"]
	if blocks.State == nil || len(blocks.State.Content) != 1 || blocks.State.Content[0].Text != "shown" {
		t.Errorf("call-blocks = %+v, want the text channel's own text", blocks)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)
	openCodeRequireLoss(t, session, "empty_tool_results", 1)
	openCodeRequireLoss(t, session, "repeated_tool_results", 1)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 1)
	openCodeRequireLoss(t, session, "empty_tool_inputs", 1)
	openCodeRequireLoss(t, session, "tool_result_statuses", 0)
	openCodeRequireLoss(t, session, "unrepresentable_result_blocks", 1)
	if session.Carried.ToolCalls != 5 || session.Carried.ToolResults != 5 {
		t.Errorf("carried = %+v, want the five calls whose parts were written", session.Carried)
	}
}

// TestOpenCodeFlatTargetOmitsABranchAndCountsIt pins the flat target's source-side rule: a record the
// read's own relations place off the line its conversation reached is omitted (a message list has no
// parent relation to hang it on, and writing it would revive an abandoned branch as active history), and
// the walk's own count of the places it could not answer exactly is reported.
func TestOpenCodeFlatTargetOmitsABranchAndCountsIt(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Dialogue: "first", DialogueProjected: true, Content: "first",
			SourceUUID: "rec-1", Timestamp: at},
		Message{Role: "assistant", Dialogue: "on the line", DialogueProjected: true, Content: "on the line",
			SourceUUID: "rec-2", SourceParentUUID: "rec-1", Timestamp: at.Add(time.Second)},
		Message{Role: "assistant", Dialogue: "left behind", DialogueProjected: true, Content: "left behind",
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
	for _, message := range document.Messages {
		if strings.Contains(message.Text, "left behind") {
			t.Error("a branch the conversation left was written as an active message")
		}
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
		Message{Role: "assistant", Thinking: "only a trace", Content: "[Thinking]\nonly a trace", Timestamp: at},
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
