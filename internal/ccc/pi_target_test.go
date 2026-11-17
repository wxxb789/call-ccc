package ccc

import (
	"encoding/json"
	"errors"
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
	// The name is the session's own start instant with its separators replaced by dashes, pinned whole
	// rather than by a suffix: a name pinned only by `_<id>.jsonl` would stay green for any stamp spelling
	// (a padded millisecond field among them), and the stamp is the one part of the name the harness reads
	// nothing else from -- the session it resolves is named by its id under the project directory.
	if want := "2026-05-05T10-00-00Z_" + session.SessionID + ".jsonl"; session.FileName != want {
		t.Errorf("file name = %q, want %q", session.FileName, want)
	}

	// The tree: the first entry has no parent, and this document records no relation of its own (every
	// message's SourceParentUUID is empty), so the entries keep the read's order -- every later one points
	// at the entry before it. A broken chain loads and then loses everything before the break, so it is
	// the file's whole conversation. The case that pins the *recorded* relation (a branch, a tool result,
	// an omitted record) is TestPiSessionWritesTheSourcesOwnRelations below.
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

// TestPiSessionWritesTheSourcesOwnRelations pins the tree rule: a document that records the source's own
// relations (Message.SourceUUID and Message.SourceParentUUID, what the inherited readers answer) is
// written as the tree the source recorded, never as one flat chain.
//
// The shape is the one this route's review measured going wrong. It holds a line that branched off an
// earlier record -- so a turn the conversation left stands *off* the walk the target's own loader makes
// from the file's last entry instead of being presented as an active one; a tool result, whose source
// relation the reader answers as the message that made the call while the source's own file runs
// call -> result -> what follows (writing the result after that message would leave a resumed session
// with a call and no result); and a record the target omits, whose child must still be chained to the
// nearest *written* ancestor. A writer that chained every entry to the one before it fails every one of
// these assertions -- which is exactly the defect this case exists to keep out.
func TestPiSessionWritesTheSourcesOwnRelations(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	conversation := Conversation{
		Source: SourceIdentity{Provider: "pi", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
				SourceUUID: "s1", Timestamp: at(0)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "answer", Dialogue: "answer",
				SourceUUID: "s2", SourceParentUUID: "s1", Timestamp: at(1)},
			{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
				DialogueProjected: true, SourceUUID: "s3", SourceParentUUID: "s2", Timestamp: at(2),
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
					InputJSON: `{"pattern":"maxBackoff"}`}}},
			// The tool result record: the reader answers its parent as the call message, and answers the
			// record *after* it as hanging off that message too.
			{Ordinal: 3, Role: string(parser.RoleUser), SourceUUID: "s4", SourceParentUUID: "s3",
				Timestamp: at(3), ToolResults: []ToolResult{{ToolUseID: "toolu_1",
					ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
			{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "after the result",
				Dialogue: "after the result", SourceUUID: "s5", SourceParentUUID: "s3", Timestamp: at(4)},
			// The branch: a turn recorded later that hangs off the second record.
			{Ordinal: 5, Role: string(parser.RoleAssistant), Content: "left turn", Dialogue: "left turn",
				SourceUUID: "s6", SourceParentUUID: "s2", Timestamp: at(5)},
			// A record with nothing to write, and a turn whose parent it is.
			{Ordinal: 6, Role: string(parser.RoleAssistant), SourceUUID: "s7", SourceParentUUID: "s5",
				Timestamp: at(6)},
			{Ordinal: 7, Role: string(parser.RoleAssistant), Content: "after the omitted record",
				Dialogue: "after the omitted record", SourceUUID: "s8", SourceParentUUID: "s7",
				Timestamp: at(7)},
			// A record naming a parent this read does not hold: the entry follows the one before it, and
			// the report counts the relation as one this conversion wrote rather than the source's.
			{Ordinal: 8, Role: string(parser.RoleAssistant), Content: "parent outside the read",
				Dialogue: "parent outside the read", SourceUUID: "s9", SourceParentUUID: "gone",
				Timestamp: at(8)},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	ids := map[string]string{}
	for id, text := range texts {
		if text != "" {
			ids[text] = id
		}
	}
	parentIDOf := func(text string) string {
		t.Helper()
		id, ok := ids[text]
		if !ok {
			t.Fatalf("no entry carries the text %q:\n%s", text, session.Data)
		}
		for _, line := range lines[1:] {
			if line.ID != id {
				continue
			}
			if line.ParentID == nil {
				return ""
			}
			return *line.ParentID
		}
		t.Fatalf("no entry carries the id %q", id)
		return ""
	}
	parentTextOf := func(text string) string {
		t.Helper()
		return texts[parentIDOf(text)]
	}
	if got := parentTextOf("answer"); got != "start" {
		t.Errorf("the second record hangs off %q, want the record it named (%q)", got, "start")
	}
	if got := parentTextOf("start"); got != "" {
		t.Errorf("the first record hangs off %q, want null: the source recorded no parent for it", got)
	}
	// The result follows the message that made the call, and the turn recorded after the result follows
	// the result: the call/result pair stays on one line, as the source's own file holds it.
	resultID := ""
	for _, line := range lines[1:] {
		if line.Type == "message" && strings.Contains(string(line.Message), `"role":"toolResult"`) {
			resultID = line.ID
		}
	}
	if resultID == "" {
		t.Fatalf("no tool result entry was written:\n%s", session.Data)
	}
	resultParentID := ""
	for _, line := range lines[1:] {
		if line.ID == resultID {
			if line.ParentID == nil {
				t.Fatal("the tool result entry carries no parent, so it hangs off no call")
			}
			resultParentID = *line.ParentID
		}
	}
	callBlocks := 0
	for _, line := range lines[1:] {
		if line.ID != resultParentID {
			continue
		}
		var message struct {
			Content []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"content"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("decoding the tool result's parent: %v", err)
		}
		for _, block := range message.Content {
			if block.Type == "toolCall" && block.ID == "toolu_1" {
				callBlocks++
			}
		}
	}
	if callBlocks != 1 {
		t.Errorf("the tool result hangs off an entry holding %d block(s) calling toolu_1, want the "+
			"message that made the call", callBlocks)
	}
	if got := parentIDOf("after the result"); got != resultID {
		t.Errorf("the record after a tool result hangs off %q, want the tool result entry %q: a writer "+
			"that wrote the result after it would leave a resumed session with a call and no result",
			got, resultID)
	}
	// The branch: the abandoned turn hangs off the record it named, not off the entry written before it.
	if got := parentTextOf("left turn"); got != "answer" {
		t.Errorf("the abandoned turn hangs off %q, want the record it named (%q)", got, "answer")
	}
	// The omitted record: its child is chained past it to the nearest written ancestor.
	if got := parentTextOf("after the omitted record"); got != "after the result" {
		t.Errorf("the record after an omitted one hangs off %q, want the nearest written ancestor (%q)",
			got, "after the result")
	}
	// The unresolvable relation: the entry follows the entry before it, and the report says so.
	if got := parentTextOf("parent outside the read"); got != "after the omitted record" {
		t.Errorf("the record naming a parent outside the read hangs off %q, want the entry before it", got)
	}
	// The two counted relations: the record whose named parent is not in the read, and the record that
	// names the call the result of that call stands between (the read answers a record the source
	// placed under the call and one it placed under the result with the same parent, so the place the
	// conversion took -- through the result, the file's own order -- is its reading and the report says
	// so; a count that ignored the adjacent result would call the second relation the read's own).
	if loss := lossCount(t, session.Losses, "unresolved_source_parents"); loss.Count != 2 {
		t.Errorf("unresolved_source_parents = %d, want the record whose named parent is not in the read "+
			"and the record naming the call its result stands directly before", loss.Count)
	}
	// The walk the target's loader makes from the file's last entry: the abandoned turn is not on it, and
	// the call/result pair is.
	onPath := piWalkIDs(t, lines)
	for _, text := range []string{"start", "answer", "after the result", "after the omitted record",
		"parent outside the read"} {
		if !onPath[ids[text]] {
			t.Errorf("%q is not on the walk the loader makes from the file's last entry, and it is part "+
				"of the conversation", text)
		}
	}
	for _, id := range []string{resultID, resultParentID} {
		if !onPath[id] {
			t.Errorf("the call/result pair is not on the walk the loader makes (entry %q): a resumed "+
				"session would hold a call, or a result, without the other", id)
		}
	}
	if onPath[ids["left turn"]] {
		t.Error("the abandoned turn is on the walk the loader makes: the branch was flattened into the " +
			"active conversation")
	}
	if !strings.Contains(string(session.Data), "left turn") {
		t.Error("the abandoned turn is not in the file at all: the branch is recorded history and must " +
			"travel, off the walk rather than dropped")
	}
	want := []string{"user", "assistant", "assistant", "toolResult", "assistant", "assistant", "assistant"}
	roles := piModelVisibleRoles(session.Data)
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("the model-visible history = %v, want %v (the source's own line, with the abandoned turn "+
			"off it)", roles, want)
	}
}

// TestPiSessionWritesNoProviderItCannotAnswer pins the attribution rule for an assistant entry: the
// source read answers the *model* each message recorded -- which travels verbatim, so the target's own
// totals count the turn under the model that wrote it -- and no endpoint attribution at all, while the
// read's session-level provider names the harness that wrote the words. Writing that label into Pi's
// `provider` field would name an endpoint the caller's configuration need not define; the entry records
// the model alone and the report says why.
func TestPiSessionWritesNoProviderItCannotAnswer(t *testing.T) {
	conversation := piFixtureConversation()
	// The harness label the read reports as the session's provider: if the writer used it, the file would
	// hold this string.
	conversation.Source.Provider = "example-harness"
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	data := string(session.Data)
	if strings.Contains(data, `"provider"`) || strings.Contains(data, "example-harness") {
		t.Errorf("the converted session names a provider (%q appears in it), while the read answers none "+
			"for the source's turns:\n%s", "example-harness", data)
	}
	if !strings.Contains(data, `"model":"claude-sonnet-4-5"`) {
		t.Errorf("the converted session does not carry the model the source's messages recorded:\n%s", data)
	}
	loss := lossCount(t, session.Losses, "message_model_attribution")
	if loss.Count != 2 {
		t.Errorf("message_model_attribution = %+v, want the fixture's two model-bearing messages", loss)
	}
	if !strings.Contains(loss.Detail, "endpoint") {
		t.Errorf("the loss detail does not say the endpoint attribution is what could not travel: %q",
			loss.Detail)
	}
}

// TestPiSessionCountsAnIdentifiedRecordTheReadLeftParentless pins the count's own edge: a record the read
// answered an identity for but *no* parent is not the conversation's first, and the entry written for it
// follows the entry before it -- a relation this conversion invented and the report counts, exactly as
// the category's detail says. A read that records no identity at all is the other case: its order is
// linear, following it invents nothing, and the count stays zero.
func TestPiSessionCountsAnIdentifiedRecordTheReadLeftParentless(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	base := func(messages []Message) Conversation {
		return Conversation{
			Source:   SourceIdentity{Provider: "pi", SessionID: "source-session", Cwd: "/work/project"},
			Messages: messages,
			Fidelity: Fidelity{ReadFrom: "source"},
		}
	}
	session, err := ConvertToPiSession(base([]Message{
		{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
			SourceUUID: "s1", Timestamp: at(0)},
		{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "one", Dialogue: "one",
			SourceUUID: "s2", SourceParentUUID: "s1", Timestamp: at(1)},
		{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "two", Dialogue: "two",
			SourceUUID: "s3", Timestamp: at(2)},
	}), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if loss := lossCount(t, session.Losses, "unresolved_source_parents"); loss.Count != 1 {
		t.Errorf("unresolved_source_parents = %d, want the one identified record the read left parentless",
			loss.Count)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	for _, line := range lines[1:] {
		if texts[line.ID] != "two" {
			continue
		}
		if line.ParentID == nil || texts[*line.ParentID] != "one" {
			t.Errorf("the parentless record's entry does not follow the entry before it: %s", session.Data)
		}
	}
	// The same document without identities: a linear read, whose order is followed rather than invented.
	plain, err := ConvertToPiSession(base([]Message{
		{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start", Timestamp: at(0)},
		{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "one", Dialogue: "one", Timestamp: at(1)},
		{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "two", Dialogue: "two", Timestamp: at(2)},
	}), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting an identity-less document: %v", err)
	}
	if loss := lossCount(t, plain.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %d for an identity-less read, want 0: its order is the read's "+
			"own answer", loss.Count)
	}
}

// TestPiSessionChainsPastAMixedRecordToItsResult pins the rule for the record shape a Claude transcript
// actually writes: one user record can hold both the prompt text and a tool result (the harness puts a
// blocked call's output and whatever the user typed in the same record). Pi has no such record, so the
// conversion writes the text as a user entry and the result as its own toolResult entry -- and the
// record that follows hangs off the *result*, the final entry the source record became, not off the
// text. Mapping the source record to its first entry instead was measured wrong: the tool result ended
// up off the walk the target's loader makes, so a resumed session read a call's output as missing while
// the file held it.
func TestPiSessionChainsPastAMixedRecordToItsResult(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	conversation := Conversation{
		Source: SourceIdentity{Provider: "claude", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Content: "run the tests", Dialogue: "run the tests",
				SourceUUID: "u1", Timestamp: at(0)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "running them",
				Dialogue: "running them", SourceUUID: "a1", SourceParentUUID: "u1", Timestamp: at(1),
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "bash",
					InputJSON: `{"command":"go test"}`}}},
			// The mixed record: the prompt text and the call's result in one source record.
			{Ordinal: 2, Role: string(parser.RoleUser), Dialogue: "the output:", DialogueProjected: true,
				Content: "the output:", SourceUUID: "u2", SourceParentUUID: "a1", Timestamp: at(2),
				ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: `"ok"`, ContentSize: 4}}},
			{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "all green", Dialogue: "all green",
				SourceUUID: "a2", SourceParentUUID: "u2", Timestamp: at(3)},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	ids := map[string]string{}
	for id, text := range texts {
		if text != "" {
			ids[text] = id
		}
	}
	parentTextOf := func(text string) string {
		t.Helper()
		id, ok := ids[text]
		if !ok {
			t.Fatalf("no entry carries the text %q:\n%s", text, session.Data)
		}
		for _, line := range lines[1:] {
			if line.ID != id {
				continue
			}
			if line.ParentID == nil {
				return ""
			}
			return texts[*line.ParentID]
		}
		t.Fatalf("no entry carries the id %q", id)
		return ""
	}
	if got := parentTextOf("all green"); got != "ok" {
		t.Errorf("the record after the mixed record hangs off the entry holding %q, want the mixed "+
			"record's tool result (%q): mapping the source record to its first entry instead leaves the "+
			"result off the walk the target's loader makes", got, "ok")
	}
	resultID := ids["ok"]
	if onPath := piWalkIDs(t, lines); !onPath[resultID] {
		t.Error("the mixed record's tool result is not on the walk the loader makes from the file's last " +
			"entry, so a resumed session reads a call whose output the file holds but the conversation " +
			"does not")
	}
	want := []string{"user", "assistant", "user", "toolResult", "assistant"}
	if got := piModelVisibleRoles(session.Data); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the model-visible history = %v, want %v", got, want)
	}
}

// piEntryTexts answers each entry's own dialogue text, keyed by entry id: the plain content string a user
// or tool-result record carries, or the text blocks an assistant record carries. It is what a case reads
// a written session by, so an assertion can name an entry by what a reader would see in it.
func piEntryTexts(t *testing.T, lines []piLine) map[string]string {
	t.Helper()
	texts := map[string]string{}
	for _, line := range lines[1:] {
		if line.Type != "message" {
			continue
		}
		var message struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("decoding entry %s: %v", line.ID, err)
		}
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(message.Content, &blocks); err != nil {
				continue
			}
			var builder strings.Builder
			for _, block := range blocks {
				if block.Type != "text" || block.Text == "" {
					continue
				}
				if builder.Len() > 0 {
					builder.WriteString("\n")
				}
				builder.WriteString(block.Text)
			}
			text = builder.String()
		}
		texts[line.ID] = text
	}
	return texts
}

// piWalkIDs answers the ids of the entries one written session's walk covers: the path from the file's
// last entry back through parentId, which is what the target's own loader follows (piModelVisibleRoles
// reads the roles off the same walk). A case uses it to name the entries a loader would see -- and the
// ones it would not, which is what makes a flattened branch visible to a test.
func piWalkIDs(t *testing.T, lines []piLine) map[string]bool {
	t.Helper()
	if len(lines) < 2 {
		t.Fatal("the session holds no entry")
	}
	byID := map[string]piLine{}
	for _, line := range lines[1:] {
		byID[line.ID] = line
	}
	onPath := map[string]bool{}
	for cursor := lines[len(lines)-1].ID; cursor != ""; {
		line, ok := byID[cursor]
		if !ok || onPath[cursor] {
			break
		}
		onPath[cursor] = true
		if line.ParentID == nil {
			break
		}
		cursor = *line.ParentID
	}
	return onPath
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
	// What stands before the boundary, read off the file rather than described: the fixture's prompt, the
	// assistant turn that answered it and the call-only entry Pi records the tool call in -- three entries
	// of the kind the report counts as messages, and two dialogue items among them (the call-only entry
	// carries no words of its own) -- plus the tool result as an entry of its own, which the report counts
	// as a result rather than as a message. That composition is what the count is of, so the count alone
	// cannot state it: two dialogue items and three message entries are different claims.
	roles := []string{}
	for _, line := range lines {
		if line.ID == boundary.ID {
			break
		}
		if line.Type != "message" {
			continue
		}
		var message struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("decoding an entry before the boundary: %v", err)
		}
		roles = append(roles, message.Role)
	}
	if want := "user,assistant,assistant,toolResult"; strings.Join(roles, ",") != want {
		t.Errorf("the message entries before the boundary = %v, want %s: the prompt, the assistant turn, "+
			"the call-only entry the call block is written in, and the tool result", roles, want)
	}
	preBoundary := lossCount(t, session.Losses, "pre_compaction_history")
	if preBoundary.Count != 3 {
		t.Errorf("pre_compaction_history = %d, want the three message entries carried before the boundary "+
			"(the tool result is an entry of the file but a result in the report, not a message)",
			preBoundary.Count)
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
		"synthesized_tool_call_messages",
		"unclassified_role_messages", "empty_user_messages", "empty_assistant_messages",
		"non_dialogue_content", "whitespace_only_dialogue", "unpaired_tool_results",
		"empty_tool_results", "whitespace_only_tool_results", "unrepresentable_result_blocks",
		"unresolved_tool_outputs", "unrecorded_token_usage", "tool_result_statuses",
		"empty_tool_inputs", "generated_call_ids", "coalesced_tool_call_projections",
		"subagent_session_links", "tool_name_vocabulary", "message_model_attribution",
		"pre_compaction_history", "empty_compaction_summary", "malformed_source_lines",
		"summary_transcript", "source_needs_retry", "unresolved_source_parents",
	}
	listed := make(map[string]struct{}, len(wanted))
	for _, category := range wanted {
		listed[category] = struct{}{}
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
	// The other direction: every category the report gives a disposition to is one this case lists, so a
	// category added to the writer cannot pass silently by being absent from the list above.
	for _, loss := range session.Losses {
		if _, known := listed[loss.Category]; !known {
			t.Errorf("the report carries category %q, which this case does not list: add it here so a "+
				"category added to the writer cannot pass silently", loss.Category)
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

// TestPiSessionKeepsAReasoningTraceUnderWhitespaceDialogue pins the shape a record with a trace and a
// whitespace-only dialogue projects to: the inherited Pi reader drops a record left with no text and no
// tool evidence, so the whitespace alone is what is dropped -- the thinking block stays, because the
// trace is not dialogue and writing the record through writeCarriedCalls instead lost it while
// reasoning_text reported it as carried.
func TestPiSessionKeepsAReasoningTraceUnderWhitespaceDialogue(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	conversation := Conversation{
		Source: SourceIdentity{Provider: "claude", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
				SourceUUID: "u1", Timestamp: at(0)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), DialogueProjected: true, Dialogue: "   ",
				Thinking: "The retry cap is in the config file.", SourceUUID: "a1",
				SourceParentUUID: "u1", Timestamp: at(1)},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	data := string(session.Data)
	if !strings.Contains(data, `"thinking":"The retry cap is in the config file."`) {
		t.Errorf("the written session holds no thinking block for the trace:\n%s", data)
	}
	if strings.Contains(data, `"type":"text"`) {
		t.Errorf("the written session holds a text block for a whitespace-only dialogue:\n%s", data)
	}
	if loss := lossCount(t, session.Losses, "whitespace_only_dialogue"); loss.Count != 1 {
		t.Errorf("whitespace_only_dialogue = %+v, want the record's whitespace counted", loss)
	}
	if loss := lossCount(t, session.Losses, "reasoning_text"); loss.Count != 1 ||
		loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("reasoning_text = %+v, want the trace carried with a caveat", loss)
	}
	if roles := piModelVisibleRoles(session.Data); strings.Join(roles, ",") != "user,assistant" {
		t.Errorf("the model-visible history = %v, want the record's own entry on the walk", roles)
	}
}

// TestPiSessionChainsThroughAToolResultAcrossAnInterveningBranch pins the projection rule's other half:
// the inherited Pi reader answers a record under a tool result as hanging off the message that made the
// call (a result's descendants resolve to their visible ancestor), so a record the read answers that way
// may in the source's own file have hung off the result. When another record was written in between --
// here a branch the conversation left -- the result is still the reading the file's own order supports,
// the entry is chained through it, and the report counts the relation as one the read did not establish.
func TestPiSessionChainsThroughAToolResultAcrossAnInterveningBranch(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	conversation := Conversation{
		Source: SourceIdentity{Provider: "pi", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
				SourceUUID: "u1", Timestamp: at(0)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
				DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1", Timestamp: at(1),
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
					InputJSON: `{"pattern":"maxBackoff"}`}}},
			// The result: the reader answers its parent as the call message.
			{Ordinal: 2, Role: string(parser.RoleUser), SourceUUID: "r3", SourceParentUUID: "c2",
				Timestamp: at(2), ToolResults: []ToolResult{{ToolUseID: "toolu_1",
					ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
			// The branch: a turn the conversation left, written after the result.
			{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "left turn", Dialogue: "left turn",
				SourceUUID: "b4", SourceParentUUID: "u1", Timestamp: at(3)},
			// The record that continues from the result: the reader answers its parent as the call too,
			// because the result is not a visible ancestor.
			{Ordinal: 4, Role: string(parser.RoleAssistant), Content: "after the result",
				Dialogue: "after the result", SourceUUID: "d5", SourceParentUUID: "c2", Timestamp: at(4)},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	ids := map[string]string{}
	for id, text := range texts {
		if text != "" {
			ids[text] = id
		}
	}
	parentTextOf := func(text string) string {
		t.Helper()
		id, ok := ids[text]
		if !ok {
			t.Fatalf("no entry carries the text %q:\n%s", text, session.Data)
		}
		for _, line := range lines[1:] {
			if line.ID != id {
				continue
			}
			if line.ParentID == nil {
				return ""
			}
			return texts[*line.ParentID]
		}
		t.Fatalf("no entry carries the id %q", id)
		return ""
	}
	if got := parentTextOf("after the result"); got != "internal/retry/config.go:41" {
		t.Errorf("the record continuing from the result hangs off %q, want the result's own entry (%q): "+
			"chaining it to the call instead leaves the recorded output off the walk a resumed session "+
			"makes", got, "internal/retry/config.go:41")
	}
	onPath := piWalkIDs(t, lines)
	for _, text := range []string{"start", "internal/retry/config.go:41", "after the result"} {
		if !onPath[ids[text]] {
			t.Errorf("%q is not on the walk the loader makes from the file's last entry", text)
		}
	}
	if onPath[ids["left turn"]] {
		t.Error("the abandoned turn is on the walk the loader makes: the branch was flattened into the " +
			"active conversation")
	}
	if !strings.Contains(string(session.Data), "left turn") {
		t.Error("the abandoned turn is not in the file at all: the branch is recorded history and must " +
			"travel, off the walk rather than dropped")
	}
	loss := lossCount(t, session.Losses, "unresolved_source_parents")
	if loss.Count != 1 {
		t.Errorf("unresolved_source_parents = %+v, want the one relation the read's projection left "+
			"unanswerable", loss)
	}
	for _, phrase := range []string{"visible ancestor", "tool result"} {
		if !strings.Contains(loss.Detail, phrase) {
			t.Errorf("the detail does not name the projection the count stands for (%q missing): %q",
				phrase, loss.Detail)
		}
	}

	// The other reading: with an exact read (a Claude transcript's parentUuid names a record, not a
	// projection), the record's answer is its own, the entry hangs off the call it named, and nothing is
	// counted as unanswerable -- the result stays in the file, off the walk, as the read places it.
	exact := conversation
	exact.Source.Provider = "claude"
	session, err = ConvertToPiSession(exact, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting with an exact read: %v", err)
	}
	lines = decodePiSession(t, session.Data)
	texts = piEntryTexts(t, lines)
	ids = map[string]string{}
	for id, text := range texts {
		if text != "" {
			ids[text] = id
		}
	}
	if got := parentTextOf("after the result"); got != "" {
		t.Errorf("the record hangs off %q, want the call's own entry (no dialogue): the read answered "+
			"the relation exactly and no projection is undone", got)
	}
	if loss := lossCount(t, session.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v, want 0: an exact read leaves nothing unanswerable", loss)
	}

	// The adjacent shape: a record naming a call *immediately after* that call's result, where the
	// reader has nothing to project and the read's answers are its own. The record hangs off the call it
	// named; the result -- here carrying a user turn beside its output, the shape a Claude transcript
	// writes -- stays in the file off the walk, as recorded history, instead of being chained in as an
	// ancestor.
	adjacent := func(provider string, siblingDialogue bool) Conversation {
		dialogue := ""
		if siblingDialogue {
			dialogue = "SIBLING-DIALOGUE"
		}
		return Conversation{
			Source: SourceIdentity{Provider: provider, SessionID: "source-session", Cwd: "/work/project"},
			Messages: []Message{
				{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
					SourceUUID: "u1", Timestamp: at(0)},
				{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "[grep: maxBackoff]",
					DialogueProjected: true, SourceUUID: "c2", SourceParentUUID: "u1", Timestamp: at(1),
					ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "grep",
						InputJSON: `{"pattern":"maxBackoff"}`}}},
				{Ordinal: 2, Role: string(parser.RoleUser), Content: dialogue, Dialogue: dialogue,
					SourceUUID: "r3", SourceParentUUID: "c2", Timestamp: at(2),
					ToolResults: []ToolResult{{ToolUseID: "toolu_1",
						ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28}}},
				{Ordinal: 3, Role: string(parser.RoleAssistant), Content: "after the result",
					Dialogue: "after the result", SourceUUID: "d4", SourceParentUUID: "c2",
					Timestamp: at(3)},
			},
			Fidelity: Fidelity{ReadFrom: "source"},
		}
	}
	// readAdjacent converts one spelling of the shape and re-reads the outer variables the assertions
	// below use, because every assertion here is about the file's own bytes and the parent chain in it.
	readAdjacent := func(conversation Conversation) []Loss {
		t.Helper()
		converted, err := ConvertToPiSession(conversation, PiTargetOptions{})
		if err != nil {
			t.Fatalf("converting the adjacent shape: %v", err)
		}
		session = converted
		lines = decodePiSession(t, session.Data)
		texts = piEntryTexts(t, lines)
		ids = map[string]string{}
		for id, text := range texts {
			if text != "" {
				ids[text] = id
			}
		}
		onPath = piWalkIDs(t, lines)
		return session.Losses
	}

	losses := readAdjacent(adjacent("claude", true))
	if got := parentTextOf("after the result"); got != "" {
		t.Errorf("the adjacent record hangs off %q, want the call's own entry (no dialogue): the read "+
			"answered the relation exactly, and the result is a sibling it placed beside it", got)
	}
	for _, text := range []string{"SIBLING-DIALOGUE", "internal/retry/config.go:41"} {
		if !strings.Contains(string(session.Data), text) {
			t.Errorf("the sibling result's %q is not in the file at all: it is recorded history and must "+
				"travel, off the walk rather than dropped", text)
		}
		if onPath[ids[text]] {
			t.Errorf("%q is on the walk the loader makes: a record chained through the sibling result "+
				"would revive that turn as active history", text)
		}
	}
	if loss := lossCount(t, losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("unresolved_source_parents = %+v, want 0: nothing stands between the record and the "+
			"call it named, so the read's answer is the whole relation", loss)
	}
	for _, text := range []string{"start", "after the result"} {
		if !onPath[ids[text]] {
			t.Errorf("the conversation's own turn %q is not on the walk the loader makes", text)
		}
	}

	// The reading a projected source keeps for the same shape: the reader answers the record as hanging
	// off the call although the result is the record before it, and the file's own continuation
	// (call → result → what follows) is the writers' tool-result step -- which keeps the result on the
	// walk. The step is counted, because the two source files are one answer in the read: a record the
	// source placed directly under the call and one it placed under the call's result both read as
	// naming the call, so adjacency does not settle which of the two the file held and the place the
	// conversion takes (through the result) is its reading rather than the read's. The fixture is the
	// ordinary Pi shape: the result is a record of its own, with no dialogue beside it.
	losses = readAdjacent(adjacent("pi", false))
	if got := parentTextOf("after the result"); got != "internal/retry/config.go:41" {
		t.Errorf("the adjacent record hangs off %q, want the result's own entry: a projected read's "+
			"record follows the tool result standing before it", got)
	}
	for _, text := range []string{"internal/retry/config.go:41", "after the result"} {
		if !onPath[ids[text]] {
			t.Errorf("%q is not on the walk the loader makes from the file's last entry", text)
		}
	}
	if loss := lossCount(t, losses, "unresolved_source_parents"); loss.Count != 1 {
		t.Errorf("unresolved_source_parents = %+v, want the one place the read's projection left "+
			"unanswerable: a projected record standing immediately after a result of the call it names "+
			"may have hung off either, and the report must not present the conversion's reading as the "+
			"read's own answer", loss)
	}
}

// TestPiSessionKeepsAFoldedOutcomeOffAnAbandonedBranch pins the relation a call's *folded* outcome
// has. An archive read is not the native one: the inherited projection attaches a result-only row to
// the call's own message and drops the row, so the read that reaches a conversion can hold the outcome
// on the message that made the call (as that message's tool results, or as the summary the call
// carries). A record that follows such a message and names the message's own parent could not have
// hung off the outcome -- the outcome answers a call of that message, so a record the source's file
// placed under it reads as naming the message itself -- and registering the outcome under the parent
// instead chained the record through the abandoned call's result, reviving the call branch as the
// target's active history. Both spellings of the folded shape are exercised, because the two writers'
// loops that can carry an outcome are different branches of the same rule.
func TestPiSessionKeepsAFoldedOutcomeOffAnAbandonedBranch(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	folded := func(outcomeAsResults bool) Conversation {
		call := ToolCall{ToolUseID: "toolu_1", Name: "grep", InputJSON: `{"pattern":"maxBackoff"}`}
		if !outcomeAsResults {
			call.ResultContent = `"internal/retry/config.go:41"`
			call.ResultContentSize = 28
		}
		callMessage := Message{Ordinal: 1, Role: string(parser.RoleAssistant),
			Content: "[grep: maxBackoff]", DialogueProjected: true, SourceUUID: "c2",
			SourceParentUUID: "u1", Timestamp: at(1), ToolCalls: []ToolCall{call}}
		if outcomeAsResults {
			callMessage.ToolResults = []ToolResult{{ToolUseID: "toolu_1",
				ContentRaw: `"internal/retry/config.go:41"`, ContentSize: 28, ContentDecoded: true}}
		}
		return Conversation{
			Source: SourceIdentity{Provider: "pi", SessionID: "source-session", Cwd: "/work/project"},
			Messages: []Message{
				{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
					SourceUUID: "u1", Timestamp: at(0)},
				callMessage,
				// The call's branch is abandoned: the record after it names the call's own parent, so the
				// source's own line runs start -> after, with the call and its outcome off it.
				{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "after", Dialogue: "after",
					SourceUUID: "d3", SourceParentUUID: "u1", Timestamp: at(2)},
			},
			Fidelity: Fidelity{ReadFrom: "archive"},
		}
	}
	for _, spelling := range []struct {
		name            string
		outcomeAsResult bool
	}{
		{"the outcome as the call's message's own results", true},
		{"the outcome as the call's summary", false},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			session, err := ConvertToPiSession(folded(spelling.outcomeAsResult), PiTargetOptions{})
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			lines := decodePiSession(t, session.Data)
			texts := piEntryTexts(t, lines)
			ids := map[string]string{}
			for id, text := range texts {
				if text != "" {
					ids[text] = id
				}
			}
			parentTextOf := func(text string) string {
				t.Helper()
				id, ok := ids[text]
				if !ok {
					t.Fatalf("no entry carries the text %q:\n%s", text, session.Data)
				}
				for _, line := range lines[1:] {
					if line.ID != id {
						continue
					}
					if line.ParentID == nil {
						return ""
					}
					return texts[*line.ParentID]
				}
				t.Fatalf("no entry carries the id %q", id)
				return ""
			}
			if got := parentTextOf("after"); got != "start" {
				t.Errorf("the record the conversation continues with hangs off %q, want the record it "+
					"named: the call's outcome answers a call of the call's message, so a record naming "+
					"that message's parent cannot have hung off the outcome, and chaining it through the "+
					"outcome revives the abandoned call branch as an ancestor", got)
			}
			if !strings.Contains(string(session.Data), "internal/retry/config.go:41") {
				t.Error("the abandoned call's outcome is not in the file at all: it is recorded history " +
					"and must travel, off the walk rather than dropped")
			}
			onPath := piWalkIDs(t, lines)
			if onPath[ids["internal/retry/config.go:41"]] || onPath[ids["[grep: maxBackoff]"]] {
				t.Error("the abandoned call or its outcome is on the walk the loader makes from the " +
					"file's last entry: the branch was flattened into the active conversation")
			}
			for _, text := range []string{"start", "after"} {
				if !onPath[ids[text]] {
					t.Errorf("the conversation's own turn %q is not on the walk the loader makes", text)
				}
			}
			if loss := lossCount(t, session.Losses, "unresolved_source_parents"); loss.Count != 0 {
				t.Errorf("unresolved_source_parents = %+v, want 0: the record names the call's parent, "+
					"and no result of that parent stands anywhere for the read to have projected through",
					loss)
			}
		})
	}
}

// TestPiSessionRefusesASelectedLeafOffTheLine pins the refusal for a source whose selected leaf the
// target cannot be given. The target's own loader walks from the last entry a conversion wrote and the
// source's from the read's last record: here the read's last record is an empty assistant turn whose
// parent is the first record, while the entry a conversion would write last stands for a branch the
// conversation left -- so a resumed session would continue that branch. The refusal is narrow, and the
// same document with a representable leaf converts: the branch then stays in the file, off the walk.
func TestPiSessionRefusesASelectedLeafOffTheLine(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	base := func(leaf Message) Conversation {
		return Conversation{
			Source: SourceIdentity{Provider: "pi", SessionID: "source-session", Cwd: "/work/project"},
			Messages: []Message{
				{Ordinal: 0, Role: string(parser.RoleUser), Content: "start", Dialogue: "start",
					SourceUUID: "u1", Timestamp: at(0)},
				{Ordinal: 1, Role: string(parser.RoleAssistant), Content: "answer", Dialogue: "answer",
					SourceUUID: "a2", SourceParentUUID: "u1", Timestamp: at(1)},
				{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "left turn",
					Dialogue: "left turn", SourceUUID: "b3", SourceParentUUID: "a2", Timestamp: at(2)},
				leaf,
			},
			Fidelity: Fidelity{ReadFrom: "source"},
		}
	}
	empty := Message{Ordinal: 3, Role: string(parser.RoleAssistant), SourceUUID: "e4",
		SourceParentUUID: "u1", Timestamp: at(3)}
	if _, err := ConvertToPiSession(base(empty), PiTargetOptions{}); !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("error = %v, want ErrUnrepresentableLeaf: the read's leaf is the empty turn, whose line "+
			"does not hold the branch entry written last", err)
	}
	// The refusal names both records, so a caller can see which branch would have been continued.
	_, err := ConvertToPiSession(base(empty), PiTargetOptions{})
	if err == nil || !strings.Contains(err.Error(), "b3") {
		t.Errorf("error = %v, want it to name the off-line record the last entry stands for", err)
	}
	// The same tree with a leaf that has an entry: the conversion is written, the leaf's entry is the
	// file's last one, and the branch stays in the file off the walk.
	spoken := empty
	spoken.Content = "the last word"
	spoken.Dialogue = "the last word"
	session, err := ConvertToPiSession(base(spoken), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting a representable leaf: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	leafID := ""
	for id, text := range texts {
		if text == "the last word" {
			leafID = id
		}
	}
	if leafID == "" {
		t.Fatalf("no entry carries the leaf's dialogue:\n%s", session.Data)
	}
	if lines[len(lines)-1].ID != leafID {
		t.Errorf("the last entry is %q, want the leaf's own %q", lines[len(lines)-1].ID, leafID)
	}
	if !onPath[leafID] {
		t.Error("the leaf's entry is not on the walk the loader makes")
	}
	for id, text := range texts {
		if text == "left turn" && onPath[id] {
			t.Error("the branch's turn is on the walk: the branch was flattened into the active conversation")
		}
	}
}
