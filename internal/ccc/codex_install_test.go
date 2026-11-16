package ccc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// TestCodexRoundTripReportNamesDifferencesWithoutTheirText pins what a difference report is allowed to
// hold. The report is serialized into the CLI's JSON document and printed as a warning, and the items it
// compares are dialogue, tool arguments and tool outputs -- so a difference names the item (its kind,
// the call it belongs to, the tool it names) and identifies its text by size and digest, and no byte of
// that text reaches the report. The texts below stand in for the private content a real session holds.
//
// The case is the failure path on purpose: the successful path was the only one carrying a no-content
// assertion, and a report that quotes the very text it could not match is the one place this rule is
// easiest to lose.
func TestCodexRoundTripReportNamesDifferencesWithoutTheirText(t *testing.T) {
	const (
		writtenDialogue = "summarize the retry policy in internal/retry/policy.go"
		readDialogue    = "summarize the retry policy in internal/retry/client.go"
		writtenInput    = `{"command":"cat ~/.config/example-api/credentials.json"}`
		readInput       = `{"command":"cat ~/.config/example-api/tokens.json"}`
		writtenOutput   = "the credential file's own private contents"
		readOutput      = "another private tool output, of the same shape"
	)
	rollout := CodexRollout{Items: []TargetItem{
		{Kind: ItemUser, Text: writtenDialogue},
		{Kind: ItemToolCall, CallID: "toolu_01Private", ToolName: "Bash", InputJSON: writtenInput},
		{Kind: ItemToolResult, CallID: "toolu_01Private", Text: writtenOutput},
	}}
	readBack := Conversation{Messages: []Message{
		{Role: string(parser.RoleUser), Content: readDialogue},
		{
			Role:      string(parser.RoleAssistant),
			ToolCalls: []ToolCall{{ToolUseID: "toolu_01Private", Name: "Bash", InputJSON: readInput}},
			ToolResults: []ToolResult{
				{ToolUseID: "toolu_01Private", ContentRaw: readOutput, ContentDecoded: true},
			},
		},
	}}

	roundTrip := compareCodexRoundTrip(rollout, readBack, nil)
	if len(roundTrip.differences) != 3 {
		t.Fatalf("differences = %q, want one per altered item", roundTrip.differences)
	}
	report := strings.Join(roundTrip.differences, "; ")
	for _, content := range []string{writtenDialogue, readDialogue, writtenInput, readInput,
		writtenOutput, readOutput} {
		if strings.Contains(report, content) {
			t.Errorf("the difference report carries the source content it compared: %q\nreport: %s",
				content, report)
		}
	}
	// What identifies an item without being any of its text: the category, the ordinal, the call and the
	// tool it belongs to, and the size and digest of the text itself.
	for _, want := range []string{
		"dialogue 1 differs",
		"user_message (" + describeText(writtenDialogue) + ")",
		"tool call 1 differs",
		"call toolu_01Private to Bash (" + describeText(writtenInput) + ")",
		"tool result 1 differs",
		"call toolu_01Private's result (" + describeText(writtenOutput) + ")",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("the difference report does not name %q:\n%s", want, report)
		}
	}

	// A digest is only useful if it distinguishes the two texts being compared: the report carries the
	// digest of the side each item came from, not one digest for the pair.
	if strings.Count(report, describeText(readOutput)) != 1 {
		t.Errorf("the read-back side's own digest is not named exactly once:\n%s", report)
	}
	// And it has to be that text's own digest, in the format the report documents, so a reader outside
	// this program can hash the same words and match the line. Spelled here rather than asked of
	// describeText, which is what the assertion above already used.
	sum := sha256.Sum256([]byte(readDialogue))
	if want := fmt.Sprintf("%d bytes, sha256 %x", len(readDialogue), sum[:]); !strings.Contains(report, want) {
		t.Errorf("the read-back dialogue is not identified by its own SHA-256 (%q):\n%s", want, report)
	}
}

// TestCodexRoundTripCountsWhatTheReaderProjectsAway pins the two answers a written dialogue message the
// read-back shows no message for can have, and what tells them apart: the installed file's own bytes.
//
// The inherited Codex reader reads no message for a user text block that opens with `# AGENTS.md` -- it
// reads that envelope as injected context rather than as a turn -- so a genuine prompt with that prefix
// is written by the conversion and then absent from every read of the file. Reporting it as a missing
// message failed a correct run; accepting it unconditionally would accept a message the file does not
// hold. The file is therefore asked, and the answers are counted apart: the text is the reader's
// projection where the file holds it, and a loss where it does not.
func TestCodexRoundTripCountsWhatTheReaderProjectsAway(t *testing.T) {
	const projected = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	const assistant = "I read the policy."
	rollout := CodexRollout{Items: []TargetItem{
		{Kind: ItemUser, Text: projected},
		{Kind: ItemAssistant, Text: assistant},
	}}
	// The read-back the inherited reader really produces for such a file: the projected message is gone
	// and the assistant's is there.
	readBack := Conversation{Messages: []Message{
		{Role: string(parser.RoleAssistant), Content: assistant},
	}}

	t.Run("the file holds the text", func(t *testing.T) {
		// The bytes are the record this writer writes for a user message, taken from the writer's own
		// encoding of the text so the case pins the check and not a hand-spelled literal.
		installed := []byte(`{"type":"response_item","payload":{"role":"user","content":[{"type":` +
			`"input_text","text":` + codexJSONString(t, projected) + `}]}}`)
		roundTrip := compareCodexRoundTrip(rollout, readBack, installed)
		if len(roundTrip.differences) != 0 {
			t.Errorf("differences = %q, want none: the file holds the text the read-back does not show",
				roundTrip.differences)
		}
		if roundTrip.notReadBackAsDialogue != 1 {
			t.Errorf("notReadBackAsDialogue = %d, want 1", roundTrip.notReadBackAsDialogue)
		}
		if detail := roundTrip.detail(); !strings.Contains(detail, "1 written message(s)") ||
			strings.Contains(detail, projected) {
			t.Errorf("the verification detail = %q, want the count named and no content", detail)
		}
	})

	t.Run("the file does not hold the text", func(t *testing.T) {
		// The control: the same document and the same read-back, with a file that never held the text.
		// This is the mutation the projection resolution must not swallow -- a writer that dropped the
		// message would otherwise pass.
		installed := []byte(`{"type":"response_item","payload":{"role":"user","content":[]}}`)
		roundTrip := compareCodexRoundTrip(rollout, readBack, installed)
		if roundTrip.notReadBackAsDialogue != 0 {
			t.Errorf("notReadBackAsDialogue = %d, want 0: the file does not hold the text",
				roundTrip.notReadBackAsDialogue)
		}
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) == 0 ||
			!strings.Contains(report, "dialogue 1 differs") ||
			!strings.Contains(report, describeText(projected)) {
			t.Errorf("differences = %q, want the written message named by its own size and digest",
				roundTrip.differences)
		}
		if detail := roundTrip.detail(); detail != "" {
			t.Errorf("a comparison with a difference reports a verification detail: %q", detail)
		}
	})

	t.Run("the text is in the file, but not as a record's own string", func(t *testing.T) {
		// The check looks for a JSON string, which is the shape every message text is written in. A byte
		// run that merely contains the words -- an unescapable spelling, a text appended to another
		// field -- is not a record carrying them, and it must not read as the message being present.
		installed := []byte(projected + "\n")
		roundTrip := compareCodexRoundTrip(rollout, readBack, installed)
		if roundTrip.notReadBackAsDialogue != 0 || len(roundTrip.differences) == 0 {
			t.Errorf("round trip = %+v, want the message reported missing: raw bytes are not a record",
				roundTrip)
		}
	})
}

// codexJSONString spells text the way the writer's own encoder writes it inside a record.
func codexJSONString(t *testing.T, text string) string {
	t.Helper()
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		t.Fatalf("encoding %q: %v", text, err)
	}
	return strings.TrimSuffix(encoded.String(), "\n")
}
