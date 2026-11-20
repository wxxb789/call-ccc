package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// selectionFixtureConversation reads the committed selection fixture through the inherited reader: the
// selection's own rules are about a *read* document, so the cases that pin them read one, while the cases
// the plan says a Claude read cannot produce build their document directly.
func selectionFixtureConversation(t *testing.T) Conversation {
	t.Helper()
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), claudeSelectionFixtureRef)
	if err != nil {
		t.Fatalf("reading the selection fixture: %v", err)
	}
	if got := len(conversation.Messages); got != 11 {
		t.Fatalf("the selection fixture reads as %d record(s), want 11: the committed bytes and the "+
			"reader have drifted from the shape these cases name", got)
	}
	return conversation
}

// optionalLoss answers one category's count without failing when the category is absent: the mutation
// arms of the D6 cases assert on documents a writer must treat differently, and "the row is not there"
// is one of the answers they compare.
func optionalLoss(losses []Loss, category string) (int, bool) {
	for _, loss := range losses {
		if loss.Category == category {
			return loss.Count, true
		}
	}
	return 0, false
}

// selectionOrdinals answers the ordinals a document holds, in its own order.
func selectionOrdinals(conversation Conversation) []int {
	ordinals := make([]int, 0, len(conversation.Messages))
	for _, message := range conversation.Messages {
		ordinals = append(ordinals, message.Ordinal)
	}
	return ordinals
}

// rolloutHolds answers whether one written rollout holds a piece of text at all.
func rolloutHolds(data []byte, text string) bool {
	return strings.Contains(string(data), text)
}

// rolloutOutputFor answers the function_call_output text written for one call id, and whether the file
// holds such a record at all: the D6 progress case is about the summary fallback writing an output the
// read never stated an outcome for, so the case asks the record itself rather than any occurrence of the
// text.
func rolloutOutputFor(t *testing.T, data []byte, callID string) (string, bool) {
	t.Helper()
	for _, line := range decodeRollout(t, data) {
		if line.Type != codexRecordResponseItem || payloadField(t, line, "type") != codexItemFunctionOutput {
			continue
		}
		if payloadField(t, line, "call_id") != callID {
			continue
		}
		return payloadField(t, line, "output"), true
	}
	return "", false
}

// TestSelectionMatchesTheOrdinalValuesTheReadSupplied pins the space a specification is written against:
// the values the read itself supplied, which are the source's own numbering rather than a position. The
// Shelley shape (a reader that preserves its source's `sequence_id`) is built directly, because no
// committed fixture carries a space that does not start at 0; the refusal for a positional reading is
// what the case is about, and the fixture read is where the space's own sentence is checked.
func TestSelectionMatchesTheOrdinalValuesTheReadSupplied(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, report, err := Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting ordinal 2: %v", err)
	}
	if report.Space.Min != 0 || report.Space.Max != 10 || report.Space.Records != 11 ||
		!report.Space.Contiguous {
		t.Errorf("ordinal space = %+v, want the read's own values 0..10 with no gap", report.Space)
	}
	if got := selectionOrdinals(selected); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("carried ordinals = %v, want the selected record 2 and its recorded result 3", got)
	}

	// The Shelley shape: five records whose ordinals are the source's own 1..5, so 0 names no record and
	// the last position is 4.
	shelley := testConversation(
		Message{Ordinal: 1, Role: "user", Content: "one", Dialogue: "one", SourceUUID: "s1"},
		Message{Ordinal: 2, Role: "assistant", Content: "two", Dialogue: "two", SourceUUID: "s2",
			SourceParentUUID: "s1"},
		Message{Ordinal: 3, Role: "user", Content: "three", Dialogue: "three", SourceUUID: "s3",
			SourceParentUUID: "s2"},
		Message{Ordinal: 4, Role: "assistant", Content: "four", Dialogue: "four", SourceUUID: "s4",
			SourceParentUUID: "s3"},
		Message{Ordinal: 5, Role: "user", Content: "five", Dialogue: "five", SourceUUID: "s5",
			SourceParentUUID: "s4"},
	)
	selected, report, err = Select(shelley, "1-5", false)
	if err != nil {
		t.Fatalf("selecting the Shelley space 1-5: %v", err)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("carried ordinals = %v, want the five values the read supplied", got)
	}
	if report.Space.Min != 1 || report.Space.Max != 5 || report.Space.Records != 5 ||
		!report.Space.Contiguous {
		t.Errorf("Shelley space = %+v, want 1..5: 5 record(s), min..max with no gap", report.Space)
	}
	// A positional reading is refused: 0 names no record there, and `0-` names nothing it should.
	for _, spec := range []string{"0", "0-"} {
		if _, _, err := Select(shelley, spec, false); !errors.Is(err, ErrBadSelection) {
			t.Errorf("Select(%q) on the Shelley space: err = %v, want ErrBadSelection", spec, err)
		} else if !strings.Contains(err.Error(), "1-5") {
			t.Errorf("Select(%q) refusal does not name the space: %v", spec, err)
		}
	}
}

// TestSelectionRefusesABadSpecificationByName pins D5's three refusals and the grammar: a malformed token
// (including an inverted range and a duplicated ordinal), an ordinal outside the read's own space, and a
// well-formed in-space specification that names no record. Every refusal is the core's own sentence and
// names the specification's own fault, and both ordinal spaces are exercised -- the fixture's 0-based one
// and the Shelley shape's 1-based one -- so a positional reading fails a case rather than passing one.
func TestSelectionRefusesABadSpecificationByName(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	for _, testCase := range []struct {
		spec string
		want string
	}{
		{spec: "x", want: `"x" is not an ordinal`},
		{spec: "-1", want: `"-1" is not an ordinal`},
		{spec: "-", want: `"-" is not an ordinal`},
		{spec: "1,,2", want: `"" is not an ordinal`},
		{spec: "5-3", want: "inverted"},
		{spec: "1-3,3", want: "already names"},
		{spec: "2-4,3", want: "already names"},
		{spec: "11", want: "outside the read's own ordinal values"},
		{spec: "2-12", want: "outside the read's own ordinal values"},
		{spec: "", want: "the specification is empty"},
	} {
		if _, _, err := Select(conversation, testCase.spec, false); !errors.Is(err, ErrBadSelection) {
			t.Errorf("Select(%q): err = %v, want ErrBadSelection", testCase.spec, err)
		} else if !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("Select(%q) refusal = %q, want it to name %q", testCase.spec, err, testCase.want)
		}
	}
	// The out-of-space refusals name the space the read produced, so a caller reads the numbers that
	// exist rather than the positions assumed.
	if _, _, err := Select(conversation, "11", false); err == nil || !strings.Contains(err.Error(), "0-10") {
		t.Errorf("the out-of-space refusal does not name the read's own values: %v", err)
	}

	// A space with a gap: every ordinal a specification names may fall between the read's own values.
	gapped := testConversation(
		Message{Ordinal: 1, Role: "user", Content: "one", Dialogue: "one", SourceUUID: "g1"},
		Message{Ordinal: 2, Role: "assistant", Content: "two", Dialogue: "two", SourceUUID: "g2",
			SourceParentUUID: "g1"},
		Message{Ordinal: 5, Role: "user", Content: "five", Dialogue: "five", SourceUUID: "g5",
			SourceParentUUID: "g2"},
	)
	if _, _, err := Select(gapped, "3-4", false); !errors.Is(err, ErrBadSelection) ||
		!strings.Contains(err.Error(), "no record would be carried") {
		t.Errorf("Select(3-4) over a gapped space: err = %v, want the selects-nothing refusal", err)
	}
	if _, report, err := Select(gapped, "1-2", false); err != nil || report.Included.Count != 2 {
		t.Errorf("Select(1-2) over the gapped space: report = %+v, err = %v", report, err)
	}

	// CheckSelectionOptions answers the same refusals before any read, and refuses a supplied value that
	// names nothing (an unset shell variable), a value that holds whitespace alone whatever the presence
	// bit says, as well as the strict arm -- supplied at all, a false value included -- without a
	// selection.
	if err := CheckSelectionOptions("", false, false, true); !errors.Is(err, ErrBadSelection) {
		t.Errorf("a supplied empty specification: err = %v, want ErrBadSelection", err)
	}
	if err := CheckSelectionOptions("", true, false, false); !errors.Is(err, ErrBadSelection) {
		t.Errorf("--strict-selection without --select: err = %v, want ErrBadSelection", err)
	}
	if err := CheckSelectionOptions("", false, true, false); !errors.Is(err, ErrBadSelection) {
		t.Errorf("--strict-selection=false without --select: err = %v, want ErrBadSelection: the "+
			"flag's presence is the request, and a false value must not read as its absence", err)
	}
	if err := CheckSelectionOptions(" 0-2 ", true, false, true); err != nil {
		t.Errorf("a well-formed specification: err = %v, want nil", err)
	}
	if err := CheckSelectionOptions(" 0-2 ", false, true, true); err != nil {
		t.Errorf("a well-formed specification with the strict arm spelled false: err = %v, want nil", err)
	}
	if err := CheckSelectionOptions("", false, false, false); err != nil {
		t.Errorf("the flag's absence: err = %v, want nil", err)
	}
	if err := CheckSelectionOptions("   ", false, false, false); !errors.Is(err, ErrBadSelection) {
		t.Errorf("whitespace alone without the presence bit: err = %v, want ErrBadSelection", err)
	}
}

// TestSelectionCarriesTheCounterpartOfASeparatedPair pins D3's default arm on the fixture shape the plan
// names: the call recorded at ordinal 2 is answered by a result the source recorded at ordinal 3, one
// record later, so a specification whose boundary falls between them separates a recorded pair. The
// counterpart is carried and named with the ordinal that pulled it in, and the written rollout holds the
// call with its recorded output.
func TestSelectionCarriesTheCounterpartOfASeparatedPair(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, report, err := Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting the call alone: %v", err)
	}
	if report.Included.Count != 1 || strings.Join(report.Included.Regions, ",") != "2" {
		t.Errorf("included = %+v, want the one record the specification named", report.Included)
	}
	if report.NecessitiesAdded.Count != 1 || len(report.NecessitiesAdded.Added) != 1 ||
		report.NecessitiesAdded.Added[0].Ordinal != 3 || report.NecessitiesAdded.Added[0].PulledBy != 2 {
		t.Errorf("necessities_added = %+v, want the result at ordinal 3 pulled in by ordinal 2",
			report.NecessitiesAdded)
	}
	if report.Excluded.Count != 9 || strings.Join(report.Excluded.Regions, ",") != "0-1,4-10" {
		t.Errorf("excluded = %+v, want the nine records the selection left behind, 0-1,4-10", report.Excluded)
	}
	// The classes reconcile with the read: included, the necessity and the excluded records are the
	// read's own records.
	if report.Included.Count+report.NecessitiesAdded.Count+report.Excluded.Count != report.Space.Records {
		t.Errorf("included(%d) + necessities(%d) + excluded(%d) != the read's %d record(s)",
			report.Included.Count, report.NecessitiesAdded.Count, report.Excluded.Count,
			report.Space.Records)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the call with its recorded result", rollout.Carried)
	}
	if !rolloutHolds(rollout.Data, "build-ok: 3 packages compiled") {
		t.Errorf("the rollout does not hold the recorded result the counterpart rule carried:\n%s",
			rollout.Data)
	}
	if output, ok := rolloutOutputFor(t, rollout.Data, "toolu_09Selection0000000000000001"); !ok ||
		output != "build-ok: 3 packages compiled" {
		t.Errorf("the call's written output = %q (present=%v)", output, ok)
	}
}

// TestSelectionWarningsNameTheWindow pins D2.2's warning channel: a window that leaves records behind
// says how many of the read's records travelled and where the rest sat, a carried counterpart is named
// with the ordinal that pulled it in, and a call the source never answered is named as the source's own
// state -- each line on the condition that makes it true, with the arithmetic the report's own classes
// state.
func TestSelectionWarningsNameTheWindow(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	_, report, err := Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting the call: %v", err)
	}
	warnings := strings.Join(selectionWarnings(&report), "\n")
	for _, want := range []string{
		`the selection carried 2 of the read's 11 record(s); 9 record(s) at 0-1,4-10 did not travel ` +
			`(specification "2")`,
		"the selection carried 1 record(s) the specification did not name",
		"ordinals 3 (pulled by 2)",
	} {
		if !strings.Contains(warnings, want) {
			t.Errorf("the warnings do not hold %q:\n%s", want, warnings)
		}
	}
	if strings.Contains(warnings, "undischarged") {
		t.Errorf("a window that holds no undischarged call still warns about one:\n%s", warnings)
	}

	// The undischarged arm: ordinal 5's call is the one the source never answered.
	_, report, err = Select(conversation, "5", false)
	if err != nil {
		t.Fatalf("selecting the undischarged call: %v", err)
	}
	if warnings := strings.Join(selectionWarnings(&report), "\n"); !strings.Contains(warnings,
		"the source left 1 call(s) undischarged in the carried records (ordinals 5)") {
		t.Errorf("the undischarged warning is absent or miscounted:\n%s", warnings)
	}
}

// TestSelectionRefusesASeparatedPairUnderStrictSelection pins D3's strict arm: the same specification is
// refused by name before anything is converted, naming the call, the result and both ordinals -- from
// either side of the pair -- and a specification that holds both records is accepted under the same flag.
func TestSelectionRefusesASeparatedPairUnderStrictSelection(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	for _, testCase := range []struct {
		spec string
		want []string
	}{
		{spec: "2", want: []string{"toolu_09Selection0000000000000001", "ordinal 2", "ordinal 3"}},
		{spec: "3", want: []string{"toolu_09Selection0000000000000001", "ordinal 3", "ordinal 2"}},
		{spec: "7", want: []string{"toolu_09Selection0000000000000003", "ordinal 7", "ordinal 8"}},
	} {
		if _, _, err := Select(conversation, testCase.spec, true); !errors.Is(err, ErrStrictSelection) {
			t.Errorf("Select(%q, strict): err = %v, want ErrStrictSelection", testCase.spec, err)
		} else {
			for _, want := range testCase.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Select(%q, strict) refusal = %q, want it to name %q", testCase.spec, err, want)
				}
			}
		}
	}
	selected, report, err := Select(conversation, "2-3", true)
	if err != nil {
		t.Fatalf("selecting both halves of the pair under strict: %v", err)
	}
	if !report.Strict || report.Included.Count != 2 || report.NecessitiesAdded.Count != 0 {
		t.Errorf("report = %+v, want the two named records and no necessity", report)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("carried ordinals = %v, want the pair's two records", got)
	}
}

// TestSelectionLeavesAnUndischargedCallAsTheSourceLeftIt pins the source's own state the selection must
// not change: the fixture's read call at ordinal 5 is never answered, so it is carried as recorded -- the
// call without an output -- and reported in calls_undischarged. The writer's report keeps the same read
// (outstanding_tool_calls): the call is written and no outcome is written for it, because the source
// never established one.
func TestSelectionLeavesAnUndischargedCallAsTheSourceLeftIt(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, report, err := Select(conversation, "0-6", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "5" {
		t.Errorf("calls_undischarged = %+v, want the read call at ordinal 5", report.CallsUndischarged)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if rollout.Carried.ToolCalls != 2 {
		t.Errorf("carried = %+v, want both calls (the answered build call and the unanswered read call)",
			rollout.Carried)
	}
	if _, ok := rolloutOutputFor(t, rollout.Data, "toolu_09Selection0000000000000002"); ok {
		t.Errorf("the rollout writes an output for the call the source never answered")
	}
	if output, ok := rolloutOutputFor(t, rollout.Data, "toolu_09Selection0000000000000001"); !ok ||
		output != "build-ok: 3 packages compiled" {
		t.Errorf("the answered call's output = %q (present=%v)", output, ok)
	}
	if count, ok := optionalLoss(rollout.Losses, "outstanding_tool_calls"); !ok || count != 1 {
		t.Errorf("outstanding_tool_calls = %d (present=%v), want the unanswered call counted", count, ok)
	}
	// The boundary inside the carried region is published as the target's own compacted record.
	if rollout.Carried.CompactionBoundaries != 1 {
		t.Errorf("carried = %+v, want the compaction boundary inside the window carried", rollout.Carried)
	}
}

// TestSelectionAccountsForEveryCarriedRecord pins D1: for the fixture's known shape, the target carries
// exactly the selected ordinals plus the disclosed necessity, every carried record has a fate in the
// report, and the written item kinds are paired with the ordinal they came from. Exact accounting does
// not require one native record per selected ordinal -- a record can become several items, and a record
// the read's line places off the window is lost with a reason.
func TestSelectionAccountsForEveryCarriedRecord(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, report, err := Select(conversation, "0-6", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	fillSelectionRecords(&report, rollout.RecordFates)

	if len(report.Records) != 7 {
		t.Fatalf("the report accounts for %d record(s), want the seven the selection carried",
			len(report.Records))
	}
	wantItems := map[int][]string{
		0: {"user_message"},
		1: {"assistant_message"},
		2: {"tool_call"},
		3: {"tool_result"},
		4: {"assistant_message"},
		5: {"tool_call"},
		6: {"compaction_boundary"},
	}
	for _, record := range report.Records {
		want := wantItems[record.Ordinal]
		if record.Became != BecameNativeHistory {
			t.Errorf("record %d: became %q (%s), want native history", record.Ordinal, record.Became,
				record.Reason)
			continue
		}
		if !slices.Equal(record.Written, want) {
			t.Errorf("record %d: written %v, want %v", record.Ordinal, record.Written, want)
		}
	}
	// The rollup reconciles: every item the writer placed in the file is paired with an ordinal, and the
	// carried counts agree with the item list.
	paired := 0
	for _, record := range report.Records {
		paired += len(record.Written)
	}
	if paired != len(rollout.Items) {
		t.Errorf("%d written item(s) are paired to ordinals, but the writer placed %d in the file",
			paired, len(rollout.Items))
	}
	if rollout.Carried.UserMessages+rollout.Carried.AssistantMessages != 3 {
		t.Errorf("carried = %+v, want the three dialogue records of the window", rollout.Carried)
	}

	// The necessity's own row says it was pulled in, and the pair's two records each carry their half.
	selected, report, err = Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting the call: %v", err)
	}
	rollout, err = ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the pair: %v", err)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	if len(report.Records) != 2 {
		t.Fatalf("the pair's report accounts for %d record(s), want 2", len(report.Records))
	}
	if report.Records[0].Necessity || report.Records[1].Necessity == false {
		t.Errorf("records = %+v, want the result (and only it) marked as a necessity", report.Records)
	}
	if report.Records[1].PulledBy == nil || *report.Records[1].PulledBy != 2 {
		t.Errorf("the necessity's pulled_by = %v, want ordinal 2", report.Records[1].PulledBy)
	}
}

// TestSelectionAccountsForRecordsSharingAnOrdinal pins D1 for the shape the space's own text allows: two
// records may share one ordinal value, and each still gets its own fate -- the report's rows and the
// writer's fates are both in the document's own order, and the table pairs them by position rather than
// by a value two records share, so neither row receives the other record's items.
func TestSelectionAccountsForRecordsSharingAnOrdinal(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "a", Dialogue: "a", SourceUUID: "a0"},
		Message{Ordinal: 1, Role: "assistant", Content: "b", Dialogue: "b", SourceUUID: "b1",
			SourceParentUUID: "a0"},
		Message{Ordinal: 1, Role: "user", Content: "c", Dialogue: "c", SourceUUID: "c1",
			SourceParentUUID: "b1"},
	)
	selected, report, err := Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the whole space: %v", err)
	}
	if report.Space.Records != 3 || report.Space.Min != 0 || report.Space.Max != 1 ||
		!report.Space.Contiguous {
		t.Errorf("ordinal space = %+v, want three records over 0..1 with no gap", report.Space)
	}
	if len(report.Records) != 3 {
		t.Fatalf("the report accounts for %d record(s), want the three the specification named",
			len(report.Records))
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	// Each row holds its own record's item kind, in the document's own order: the two records sharing
	// ordinal 1 must not each receive both fates.
	wantItems := []string{"user_message", "assistant_message", "user_message"}
	for index, record := range report.Records {
		if record.Became != BecameNativeHistory ||
			!slices.Equal(record.Written, []string{wantItems[index]}) {
			t.Errorf("record %d (ordinal %d): became %q written %v, want the one item %s", index,
				record.Ordinal, record.Became, record.Written, wantItems[index])
		}
	}
	// The rollup reconciles: every item the writer placed in the file is paired with exactly one record.
	paired := 0
	for _, record := range report.Records {
		paired += len(record.Written)
	}
	if paired != len(rollout.Items) {
		t.Errorf("%d written item(s) are paired to records, but the writer placed %d in the file",
			paired, len(rollout.Items))
	}
}

// siblingShape is the exact-parent shape internal/ccc/codex_target_test.go pins, as a read the selection
// cases build directly: u1 → c2 (the call), with r3 (the result) and d4 (the reply) both hanging off c2.
// The read's line runs d4 → c2 → u1, so the result is off it, and a window that ends on the result must
// not move the line.
func siblingShape(resultText string) Conversation {
	return testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "[grep: maxBackoff]", DialogueProjected: true,
			SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_sib", Name: "grep",
				InputJSON: `{"pattern":"maxBackoff"}`}}},
		Message{Ordinal: 2, Role: "user", SourceUUID: "r3", SourceParentUUID: "c2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_sib", ContentRaw: resultText,
				ContentSize: len(resultText)}}},
		Message{Ordinal: 3, Role: "assistant", Content: "the reply", Dialogue: "the reply",
			SourceUUID: "d4", SourceParentUUID: "c2"},
	)
}

// TestSelectionEndsAWindowOffTheReadsLineWithoutMovingIt pins D6's pre-cut line disposition: the
// selection keeps the result and cuts the reply, so the reduced stream's last record is the off-line
// result. The carried line answer (taken over the read before the cut) keeps the writer from making it
// the new leaf; without the carried answer -- the mutation clears the seam -- the writer re-walks the
// window, activates the result and turns the arm red.
func TestSelectionEndsAWindowOffTheReadsLineWithoutMovingIt(t *testing.T) {
	const resultText = "OFF-LINE-RESULT-TEXT"
	conversation := siblingShape(resultText)
	selected, _, err := Select(conversation, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("carried ordinals = %v, want the three the specification named", got)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if loss := lossCount(t, rollout.Losses, "inactive_branch_records"); loss.Count != 1 {
		t.Errorf("inactive_branch_records = %+v, want the off-line result omitted and counted", loss)
	}
	if rollout.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want no result written: the read's own line places it off the window's "+
			"last record", rollout.Carried)
	}
	if rolloutHolds(rollout.Data, resultText) {
		t.Errorf("the rollout holds the off-line result: a record written into this target's log " +
			"becomes model-visible history, so the window revived the branch the read left")
	}
	// The mutation: the same reduced document with only the line answer invalidated -- every record
	// answered as on-line -- while selected mode stays on, so the writer's own walk over the window
	// reaches the result and writes it. (Clearing the seam's marker instead would switch the whole
	// writer to the unflagged path, which would make this arm pass for the wrong reason.)
	mutated := selected
	mutated.sourceLineInactive = make([]bool, len(mutated.Messages))
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	if err != nil {
		t.Fatalf("converting the mutated window: %v", err)
	}
	if !rolloutHolds(mutatedRollout.Data, resultText) {
		t.Errorf("the mutation did not turn the arm red: without the carried pre-cut line answer the " +
			"writer wrote no result, so this case would pass without the seam")
	}
	if count, _ := optionalLoss(mutatedRollout.Losses, "inactive_branch_records"); count != 0 {
		t.Errorf("the mutated run counts %d inactive branch record(s): without the carried pre-cut line "+
			"answer the writer read the result as on the line", count)
	}
}

// TestSelectionAnswersNoProgressEventAsACallOutcome pins D6's lifecycle disposition on the shape the plan
// names: the call carries the archive's derived summary, and the read's only recorded event of it is a
// progress event whose record the selection omits. The carried finality (in-progress) keeps the writer
// from publishing the summary as the call's completion: no function_call_output is written for the call,
// and the call is counted outstanding. The mutation clears the seam, and the writer's own summary
// fallback publishes the progress text as the call's answer -- red.
func TestSelectionAnswersNoProgressEventAsACallOutcome(t *testing.T) {
	const progressText = "Still working"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "spawning", Dialogue: "spawning",
			SourceUUID: "p1",
			ToolCalls: []ToolCall{{ToolUseID: "call_wait", Name: "wait",
				ResultContent: progressText, ResultContentSize: len(progressText)}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "p2", SourceParentUUID: "p1",
			ToolResults: []ToolResult{{ToolUseID: "call_wait", Status: "running",
				ContentRaw: progressText, ContentSize: len(progressText)}}},
	)
	selected, report, err := Select(conversation, "0", false)
	if err != nil {
		t.Fatalf("selecting the call: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the call counted: its only recorded event is progress",
			report.CallsUndischarged)
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities_added = %+v, want none: a progress event is not the pair's counterpart",
			report.NecessitiesAdded)
	}
	if report.Excluded.Count != 1 || strings.Join(report.Excluded.Regions, ",") != "1" {
		t.Errorf("excluded = %+v, want the progress record omitted by the specification", report.Excluded)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if output, ok := rolloutOutputFor(t, rollout.Data, "call_wait"); ok {
		t.Errorf("the rollout writes an output for the call (%q): the read's only recorded event of it "+
			"is progress, so the summary would state a completion the source never recorded", output)
	}
	if rolloutHolds(rollout.Data, progressText) {
		t.Errorf("the rollout holds the progress text: it belongs to the event the selection omitted, " +
			"and the summary is that event's view rather than an outcome")
	}
	if count, ok := optionalLoss(rollout.Losses, "outstanding_tool_calls"); !ok || count != 1 {
		t.Errorf("outstanding_tool_calls = %d (present=%v), want the call counted", count, ok)
	}

	// The mutation: the same reduced document with this call's carried dispositions invalidated to what
	// the reduced stream alone would suggest -- no recorded event for the call in the window, and the
	// surviving summary looking like the completion -- while selected mode stays on. The writer then
	// publishes the summary: the behaviour the carried answers exist to stop. (Clearing the seam's
	// marker instead would switch the whole writer to the unflagged path, so the arm would pass even if
	// selected mode recomputed these answers from the window.)
	mutated := selected
	place := callOccurrence{message: 0, call: 0}
	mutated.sourceCallFinality = map[callOccurrence]callFinality{place: callFinalityCompleted}
	mutated.sourceCallRecordedEvents = map[callOccurrence]bool{}
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	if err != nil {
		t.Fatalf("converting the mutated window: %v", err)
	}
	if output, ok := rolloutOutputFor(t, mutatedRollout.Data, "call_wait"); !ok || output != progressText {
		t.Errorf("the mutation did not turn the arm red: without the carried dispositions the "+
			"writer wrote no summary output (present=%v, %q)", ok, output)
	}
}

// TestSelectionAnswersTheLastOutcomeTheReadRecorded pins D6's finality as a function of the recorded
// document: a read that answered one call twice -- an errored result and a later completion -- has one
// answer, the last result it recorded, and the seam carries that answer whichever way the pairing map
// happens to iterate. Without the read-order walk the map's own iteration order decides, so the same
// document answers completed or failed from run to run; the repeated calls below are what make that
// nondeterminism red.
func TestSelectionAnswersTheLastOutcomeTheReadRecorded(t *testing.T) {
	document := func(first, second string) Conversation {
		return testConversation(
			Message{Ordinal: 0, Role: "user", Content: "question", Dialogue: "question", SourceUUID: "u1"},
			Message{Ordinal: 1, Role: "assistant", Content: "calling", Dialogue: "calling",
				SourceUUID: "c2", SourceParentUUID: "u1",
				ToolCalls: []ToolCall{{ToolUseID: "call_x", Name: "Bash", InputJSON: `{}`}}},
			Message{Ordinal: 2, Role: "user", SourceUUID: "r3", SourceParentUUID: "c2",
				ToolResults: []ToolResult{{ToolUseID: "call_x", Status: first,
					ContentRaw: "first", ContentSize: len("first")}}},
			Message{Ordinal: 3, Role: "user", SourceUUID: "r4", SourceParentUUID: "c2",
				ToolResults: []ToolResult{{ToolUseID: "call_x", Status: second,
					ContentRaw: "second", ContentSize: len("second")}}},
		)
	}
	for _, testCase := range []struct {
		first, second string
		want          callFinality
	}{
		{first: "errored", second: "completed", want: callFinalityCompleted},
		{first: "completed", second: "errored", want: callFinalityFailed},
	} {
		conversation := document(testCase.first, testCase.second)
		place := callOccurrence{message: 1, call: 0}
		paired := pairedCallOccurrences(conversation)
		for index, message := range conversation.Messages {
			for resultIndex := range message.ToolResults {
				if got := paired[resultOccurrence{message: index, result: resultIndex}]; got != place {
					t.Fatalf("%s then %s: the result at record %d pairs to %+v, want %+v: the case's "+
						"premise is that both results answer the one call", testCase.first,
						testCase.second, index, got, place)
				}
			}
		}
		for attempt := range 200 {
			if got := callFinalities(conversation, paired)[place]; got != testCase.want {
				t.Fatalf("%s then %s: attempt %d answered %q, want %q: the finality is the read's last "+
					"recorded outcome, never the pairing map's iteration order", testCase.first,
					testCase.second, attempt, got, testCase.want)
			}
		}
		// The same answer crosses the seam: a window the caller named holds the finality the read's own
		// document established.
		selected, _, err := Select(conversation, "0-3", false)
		if err != nil {
			t.Fatalf("%s then %s: selecting the whole document: %v", testCase.first, testCase.second, err)
		}
		if got := selected.sourceCallFinality[place]; got != testCase.want {
			t.Errorf("%s then %s: the carried finality = %q, want %q", testCase.first, testCase.second,
				got, testCase.want)
		}
	}
}

// TestSelectionPublishesAProgressEventOnlyThroughItsCall pins the other half of the progress rule: a
// progress event the *caller* names is retained in the window, and the writer still does not publish its
// text as the call's answer -- this format's function_call_output carries an outcome or nothing -- so the
// event is omitted and counted (nonterminal_tool_events) while the call is written and counted
// outstanding. Without a selection the same record is written as it always has been (D4's frozen path).
func TestSelectionPublishesAProgressEventOnlyThroughItsCall(t *testing.T) {
	const progressText = "Still working"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "spawning", Dialogue: "spawning",
			SourceUUID: "p1",
			ToolCalls:  []ToolCall{{ToolUseID: "call_wait", Name: "wait"}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "p2", SourceParentUUID: "p1",
			ToolResults: []ToolResult{{ToolUseID: "call_wait", Status: "running",
				ContentRaw: progressText, ContentSize: len(progressText)}}},
	)
	selected, report, err := Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting both records: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if _, ok := rolloutOutputFor(t, rollout.Data, "call_wait"); ok {
		t.Errorf("the rollout publishes the retained progress event as the call's answer")
	}
	if count, ok := optionalLoss(rollout.Losses, "nonterminal_tool_events"); !ok || count != 1 {
		t.Errorf("nonterminal_tool_events = %d (present=%v), want the retained progress event counted",
			count, ok)
	}
	if count, ok := optionalLoss(rollout.Losses, "outstanding_tool_calls"); !ok || count != 1 {
		t.Errorf("outstanding_tool_calls = %d (present=%v), want the call counted", count, ok)
	}
	// The record's own fate names the category the walk charged it to: the table's contract is that a
	// lost record's reason is the loss category behind it, not the fallback sentence for a record no
	// count explains.
	fillSelectionRecords(&report, rollout.RecordFates)
	if len(report.Records) != 2 {
		t.Fatalf("the report accounts for %d record(s), want the two the specification named",
			len(report.Records))
	}
	if report.Records[1].Became != BecameLost || report.Records[1].Reason != "nonterminal_tool_events" {
		t.Errorf("record 1's fate = %+v, want it lost under the nonterminal_tool_events category",
			report.Records[1])
	}
	// The default path keeps the answer its writer has always had: a run without a selection writes the
	// event as the call's output and carries no reduction row.
	unflagged, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting the read without a selection: %v", err)
	}
	if output, ok := rolloutOutputFor(t, unflagged.Data, "call_wait"); !ok || output != progressText {
		t.Errorf("the unflagged conversion's output = %q (present=%v), want the recorded event text",
			output, ok)
	}
	if _, ok := optionalLoss(unflagged.Losses, "nonterminal_tool_events"); ok {
		t.Errorf("the unflagged report carries the reduction's own row: %+v", unflagged.Losses)
	}
}

// TestSelectionCountsTheEvidenceACompactionRecordAccountsFor pins D6's compaction ledger on the shape
// the plan calls the Codex one: a boundary record that itself carries a call and the result the boundary
// replaced. The writer publishes the boundary alone and counts the accounted call and result; without
// the carried ledger (the mutation clears the seam) the evidence vanishes uncounted -- red. The same
// shape with a boundary the read cannot answer (a Codex read skips `replacement_history`) is reported as
// unestablished rather than as none, and a read whose format records the summary alone (Claude) answers
// none.
func TestSelectionCountsTheEvidenceACompactionRecordAccountsFor(t *testing.T) {
	boundaryShape := func(provider string, calls []ToolCall, results []ToolResult) Conversation {
		conversation := testConversation(
			Message{Ordinal: 0, Role: "user", Content: "before", Dialogue: "before", SourceUUID: "u1"},
			Message{Ordinal: 1, Role: "assistant", Content: "the replaced history, summarised",
				IsSystem: true, IsCompactBoundary: true, SourceUUID: "b2", SourceParentUUID: "u1",
				ToolCalls: calls, ToolResults: results},
			Message{Ordinal: 2, Role: "assistant", Content: "after", Dialogue: "after",
				SourceUUID: "a3", SourceParentUUID: "b2"},
		)
		conversation.Source.Provider = provider
		return conversation
	}
	const callID = "call_replaced"
	const outcome = "the replaced outcome"
	synthetic := boundaryShape(string(parser.AgentCodex),
		[]ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: `{"command":"x"}`}},
		[]ToolResult{{ToolUseID: callID, ContentRaw: outcome, ContentSize: len(outcome)}})
	selected, _, err := Select(synthetic, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the boundary window: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the boundary window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "compaction_record_tool_calls"); !ok || count != 1 {
		t.Errorf("compaction_record_tool_calls = %d (present=%v), want the accounted call counted",
			count, ok)
	}
	if count, ok := optionalLoss(rollout.Losses, "compaction_record_tool_results"); !ok || count != 1 {
		t.Errorf("compaction_record_tool_results = %d (present=%v), want the accounted result counted",
			count, ok)
	}
	if count, ok := optionalLoss(rollout.Losses, "unestablished_compaction_history"); !ok || count != 0 {
		t.Errorf("unestablished_compaction_history = %d (present=%v), want none: the boundary record "+
			"itself carries the ledger", count, ok)
	}
	if !rolloutHolds(rollout.Data, "the replaced history, summarised") {
		t.Errorf("the rollout does not hold the boundary's own summary:\n%s", rollout.Data)
	}
	if _, ok := rolloutOutputFor(t, rollout.Data, callID); ok {
		t.Errorf("the rollout writes the accounted result as an outcome: the boundary record's evidence " +
			"is the history the boundary replaced")
	}
	// The mutation: the same reduced document with only the boundary ledger invalidated -- the seam
	// answers nothing for the boundary record -- while selected mode stays on. The writer reads the
	// missing answer as the unknown it is (nothing counted, unestablished_compaction_history = 1), so
	// the arm is red for the recomputation the carried ledger exists to prevent, not because the whole
	// writer fell back to the unflagged path.
	mutated := selected
	mutated.sourceBoundaries = nil
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	if err != nil {
		t.Fatalf("converting the mutated boundary window: %v", err)
	}
	if count, ok := optionalLoss(mutatedRollout.Losses, "compaction_record_tool_results"); ok && count != 0 {
		t.Errorf("the mutation did not turn the arm red: the uncounted conversion still reports %d "+
			"accounted result(s)", count)
	}
	if count, ok := optionalLoss(mutatedRollout.Losses, "unestablished_compaction_history"); !ok || count != 1 {
		t.Errorf("unestablished_compaction_history = %d (present=%v) for the mutated conversion, want "+
			"the missing ledger reported as the unknown it is, never as none", count, ok)
	}

	// The unknown state: the same boundary with no evidence of its own, read through a provider whose
	// reader skips the replaced history (Codex). The seam answers *not established*, and the writer
	// discloses it as that -- never as none.
	unknown := boundaryShape(string(parser.AgentCodex), nil, nil)
	selected, _, err = Select(unknown, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the unknown boundary window: %v", err)
	}
	rollout, err = ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the unknown boundary window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "unestablished_compaction_history"); !ok || count != 1 {
		t.Errorf("unestablished_compaction_history = %d (present=%v), want the one boundary whose "+
			"replaced history the read cannot establish", count, ok)
	}
	if count, _ := optionalLoss(rollout.Losses, "compaction_record_tool_calls"); count != 0 {
		t.Errorf("compaction_record_tool_calls = %d, want 0: an unestablished ledger counts nothing",
			count)
	}

	// The none state: a provider whose reader records the summary alone (Claude) answers an
	// established-empty ledger, and no boundary is reported as unestablished.
	none := boundaryShape(string(parser.AgentClaude), nil, nil)
	selected, _, err = Select(none, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the Claude boundary window: %v", err)
	}
	rollout, err = ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the Claude boundary window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "unestablished_compaction_history"); !ok || count != 0 {
		t.Errorf("unestablished_compaction_history = %d (present=%v), want none: a Claude boundary is "+
			"an established *none*", count, ok)
	}
}

// TestSelectionRefusesACounterpartOffTheReadsLine pins D3's closure eligibility: a specification that
// names the call alone would have to carry a result the read's own line leaves off, and this target's log
// has no parent relation to hang a branch off, so the whole selection is refused by name -- the pair and
// both of its ordinals. The same specification over a linear shape (the result on the line) carries the
// counterpart as a necessity instead.
func TestSelectionRefusesACounterpartOffTheReadsLine(t *testing.T) {
	conversation := siblingShape("OFF-LINE-RESULT-TEXT")
	// Under --strict-selection the separated pair is refused by the strict sentence itself; without it
	// the pair's off-line counterpart is what makes the whole selection impossible. Both are refusals by
	// name, which is what the case pins.
	if _, _, err := Select(conversation, "1", false); !errors.Is(err, ErrSelectionUnrepresentable) {
		t.Errorf("Select(1): err = %v, want ErrSelectionUnrepresentable", err)
	} else {
		for _, want := range []string{"ordinal 1", "ordinal 2", "off the line"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %q: %v", want, err)
			}
		}
	}
	if _, _, err := Select(conversation, "1", true); !errors.Is(err, ErrStrictSelection) {
		t.Errorf("Select(1, strict): err = %v, want ErrStrictSelection", err)
	} else if !strings.Contains(err.Error(), "ordinal 2") {
		t.Errorf("the strict refusal does not name the pair's other ordinal: %v", err)
	}
	// The linear shape: the result stands on the line below its call, so the counterpart is carried.
	linear := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call", DialogueProjected: true,
			SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_lin", Name: "grep", InputJSON: `{}`}}},
		Message{Ordinal: 2, Role: "user", SourceUUID: "r3", SourceParentUUID: "c2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_lin", ContentRaw: "the outcome",
				ContentSize: len("the outcome")}}},
	)
	selected, report, err := Select(linear, "1", false)
	if err != nil {
		t.Fatalf("selecting the linear call: %v", err)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("carried ordinals = %v, want the call and its counterpart", got)
	}
	if report.NecessitiesAdded.Count != 1 {
		t.Errorf("necessities_added = %+v, want the one counterpart", report.NecessitiesAdded)
	}
	// The mixed entry: a record the read folded a result onto can carry a call of its own, so the
	// refusal's sentence must name the roles as the read recorded them. Here the *result* sits on its own
	// record beside a call of that record's own, and the pair's call record stands off the line: reading
	// the roles back off the record that pulled the counterpart in would call the result's record the call.
	mixed := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call", DialogueProjected: true,
			SourceUUID: "c2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_mixed", Name: "grep", InputJSON: `{}`}}},
		Message{Ordinal: 2, Role: "user", SourceUUID: "r3", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_own", Name: "read", InputJSON: `{}`}},
			ToolResults: []ToolResult{{ToolUseID: "toolu_mixed", ContentRaw: "the outcome",
				ContentSize: len("the outcome")}}},
		Message{Ordinal: 3, Role: "assistant", Content: "the reply", Dialogue: "the reply",
			SourceUUID: "d4", SourceParentUUID: "r3"},
	)
	if _, _, err := Select(mixed, "2", false); !errors.Is(err, ErrSelectionUnrepresentable) {
		t.Errorf("Select(mixed, 2): err = %v, want ErrSelectionUnrepresentable", err)
	} else if !strings.Contains(err.Error(),
		"the call recorded at ordinal 1 and the result recorded at ordinal 2") {
		t.Errorf("the mixed-entry refusal names the roles wrongly: %v", err)
	}
}

// TestSelectionIsRefusedOnEveryOtherPair pins D7: the flag is honoured on the reference pair alone, and
// every other pair is refused by name in the core's Continue -- before the source is opened, so the
// refused run's reads of the source stay at zero: an error a read produced would be indistinguishable
// from one the gate produced after discarding a read, which is why the count itself is asserted. The
// control arm is the same invocation with the flag absent, which reaches the read and fails there
// instead.
func TestSelectionIsRefusedOnEveryOtherPair(t *testing.T) {
	cfg := configWithoutSources(t)
	for _, testCase := range []struct {
		ref    Ref
		target string
	}{
		{ref: Ref{Provider: "codex", ID: codexFixtureRef.ID}, target: TargetCodex},
		{ref: Ref{Provider: "claude", ID: claudeSelectionFixtureRef.ID}, target: TargetPi},
		{ref: Ref{Provider: "claude", ID: claudeSelectionFixtureRef.ID}, target: TargetOMP},
		{ref: Ref{Provider: "claude", ID: claudeSelectionFixtureRef.ID}, target: TargetClaude},
	} {
		reads := &countingSourceReads{}
		_, err := Continue(context.Background(), ContinueOptions{
			Ref:               testCase.ref,
			Target:            testCase.target,
			Config:            cfg,
			Reader:            reads,
			Selection:         "0-",
			SelectionSupplied: true,
		})
		if !errors.Is(err, ErrSelectionUnsupportedPair) {
			t.Errorf("Continue(%s → %s, --select): err = %v, want ErrSelectionUnsupportedPair",
				testCase.ref.Provider, testCase.target, err)
			continue
		}
		if !strings.Contains(err.Error(), testCase.ref.Provider) ||
			!strings.Contains(err.Error(), testCase.target) {
			t.Errorf("the pair refusal does not name the pair: %v", err)
		}
		if reads.calls != 0 {
			t.Errorf("Continue(%s → %s, --select) read the source %d time(s): the refusal must be "+
				"answered before the source is opened, and a read whose error was discarded would "+
				"still have opened it", testCase.ref.Provider, testCase.target, reads.calls)
		}
	}

	// The strict arm, supplied with an explicit false value, is still the flag's presence: the same
	// unsupported pair is refused by name rather than allowed past the gate to install the full history
	// the caller's command line kept out.
	reads := &countingSourceReads{}
	_, err := Continue(context.Background(), ContinueOptions{
		Ref:                     Ref{Provider: "claude", ID: claudeSelectionFixtureRef.ID},
		Target:                  TargetPi,
		Config:                  cfg,
		Reader:                  reads,
		Selection:               "0-",
		SelectionSupplied:       true,
		StrictSelection:         false,
		StrictSelectionSupplied: true,
	})
	if !errors.Is(err, ErrSelectionUnsupportedPair) {
		t.Errorf("Continue(claude → pi, --select 0- --strict-selection=false): err = %v, want "+
			"ErrSelectionUnsupportedPair -- a false value is the flag's presence, not its absence", err)
	}
	if reads.calls != 0 {
		t.Errorf("the strict-false refused run read the source %d time(s)", reads.calls)
	}

	// The strict arm alone -- a false value included -- names no selection at all, and is refused before
	// the pair gate even answers: a request about nothing is answered before it is answered about a pair.
	reads = &countingSourceReads{}
	_, err = Continue(context.Background(), ContinueOptions{
		Ref:                     Ref{Provider: "claude", ID: claudeSelectionFixtureRef.ID},
		Target:                  TargetPi,
		Config:                  cfg,
		Reader:                  reads,
		StrictSelectionSupplied: true,
	})
	if !errors.Is(err, ErrBadSelection) {
		t.Errorf("Continue(claude → pi, --strict-selection=false): err = %v, want ErrBadSelection", err)
	}
	if reads.calls != 0 {
		t.Errorf("the strict-only refused run read the source %d time(s)", reads.calls)
	}

	// The control arm: the same invocation without a selection is not refused by the pair gate. It
	// reaches the read, which fails -- the config's roots are empty -- and the failure is the read's.
	_, err = Continue(context.Background(), ContinueOptions{
		Ref:    Ref{Provider: "codex", ID: codexFixtureRef.ID},
		Target: TargetCodex,
		Config: cfg,
		Reader: NewSourceReader(cfg),
	})
	if err == nil {
		t.Fatal("continuing a missing source without a selection: err = nil, want a read failure")
	}
	if errors.Is(err, ErrSelectionUnsupportedPair) {
		t.Errorf("the unflagged control arm was refused by the pair gate: %v", err)
	}
}

// countingSourceReads answers one read call with an error, and counts the calls: the D7 pair gate's
// promise is that a refused pair costs no read of the source, and a count is the only place that promise
// is observable -- a read whose error a buggy run discarded would be invisible in the run's error
// sentence.
type countingSourceReads struct {
	calls int
}

func (r *countingSourceReads) Read(context.Context, Ref) (Conversation, error) {
	r.calls++
	return Conversation{}, errors.New("the source must not be opened for a refused pair")
}

// configWithoutSources is a configuration with no provider roots and no archive: a read through it
// cannot find a session, so an error that is not the read's is an answer the run gave before reading.
func configWithoutSources(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("building the default configuration: %v", err)
	}
	dir := t.TempDir()
	cfg.DataDir = dir
	cfg.DBPath = filepath.Join(dir, "sessions.db")
	cfg.AgentDirs = nil
	return cfg
}

// TestSelectionKeepsAnIDLessResultOutOfASelectedRun pins the increment's non-scope: an id-less result
// has no link the read recorded, and a selected run never infers one, so the result is omitted and
// counted (unpaired_tool_results). The unflagged run keeps the writer's own disclosed inference (the
// sole outstanding call) -- the behaviour a run without a flag has always had.
func TestSelectionKeepsAnIDLessResultOutOfASelectedRun(t *testing.T) {
	const outcome = "ID-LESS-OUTCOME"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call", SourceUUID: "c1",
			ToolCalls: []ToolCall{{ToolUseID: "call_a", Name: "Bash", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "r2", SourceParentUUID: "c1",
			ToolResults: []ToolResult{{ContentRaw: outcome, ContentSize: len(outcome)}}},
	)
	selected, report, err := Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting both records: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if rolloutHolds(rollout.Data, outcome) {
		t.Errorf("the selected run wrote the id-less result: the read recorded no link for it, and an " +
			"ambiguous shape is never inferred")
	}
	if count, ok := optionalLoss(rollout.Losses, "unpaired_tool_results"); !ok || count != 1 {
		t.Errorf("unpaired_tool_results = %d (present=%v), want the result counted", count, ok)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	if len(report.Records) != 2 {
		t.Fatalf("the report accounts for %d record(s), want the two the specification named",
			len(report.Records))
	}
	if report.Records[1].Became != BecameLost || report.Records[1].Reason != "unpaired_tool_results" {
		t.Errorf("record 1's fate = %+v, want it lost under the unpaired_tool_results category",
			report.Records[1])
	}
	unflagged, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting the read without a selection: %v", err)
	}
	if output, ok := rolloutOutputFor(t, unflagged.Data, "call_a"); !ok || output != outcome {
		t.Errorf("the unflagged conversion's output = %q (present=%v), want the writer's own disclosed "+
			"inference to carry it", output, ok)
	}
}

// TestSelectionNamesTheCategoryBehindALostToolResult pins the per-record fate table for the results a
// reduced document omits for their own recorded shape: a result whose recorded text is empty, and one
// whose text is whitespace alone (the target's own reader would trim it away). Neither record wrote an
// item, so each row's reason has to be the loss category the writer counted -- the fallback sentence is
// for a record no count explains, not for one a category does.
func TestSelectionNamesTheCategoryBehindALostToolResult(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call a", Dialogue: "call a", SourceUUID: "c1",
			ToolCalls: []ToolCall{{ToolUseID: "call_a", Name: "Bash", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "r2", SourceParentUUID: "c1",
			ToolResults: []ToolResult{{ToolUseID: "call_a"}}},
		Message{Ordinal: 2, Role: "assistant", Content: "call b", Dialogue: "call b", SourceUUID: "c3",
			SourceParentUUID: "r2",
			ToolCalls:        []ToolCall{{ToolUseID: "call_b", Name: "Read", InputJSON: `{}`}}},
		Message{Ordinal: 3, Role: "user", SourceUUID: "r4", SourceParentUUID: "c3",
			ToolResults: []ToolResult{{ToolUseID: "call_b", ContentRaw: "  ", ContentSize: 2}}},
	)
	selected, report, err := Select(conversation, "0-3", false)
	if err != nil {
		t.Fatalf("selecting the whole document: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "empty_tool_results"); !ok || count != 1 {
		t.Errorf("empty_tool_results = %d (present=%v), want the empty result counted", count, ok)
	}
	if count, ok := optionalLoss(rollout.Losses, "whitespace_only_tool_results"); !ok || count != 1 {
		t.Errorf("whitespace_only_tool_results = %d (present=%v), want the whitespace result counted",
			count, ok)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	for _, want := range []struct {
		index  int
		reason string
	}{
		{index: 1, reason: "empty_tool_results"},
		{index: 3, reason: "whitespace_only_tool_results"},
	} {
		record := report.Records[want.index]
		if record.Became != BecameLost || record.Reason != want.reason {
			t.Errorf("record %d (ordinal %d): became %q reason %q, want lost under %q", want.index,
				record.Ordinal, record.Became, record.Reason, want.reason)
		}
	}
}

// TestSelectionRefusesADocumentASelectionProduced pins the one input a selection cannot answer for -- the
// per-record line, ancestry and pairing answers are the read's own, and re-deriving them from a window is
// how a window would revive a branch the first selection left off the line (applying the specification to
// the read is the caller's move) -- and its writer-side half: a document a selection produced handed to a
// writer that does not consume the seam is refused by name rather than written by a writer that would
// recompute those answers from the punctured stream. The reference route's own writer is the one that
// accepts it, so the refusal is the seam's boundary rather than a refusal of selected documents as such.
func TestSelectionRefusesADocumentASelectionProduced(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, _, err := Select(conversation, "0-3", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	if _, _, err := Select(selected, "0", false); !errors.Is(err, ErrSelectionReduced) {
		t.Errorf("reducing a reduced document: err = %v, want ErrSelectionReduced", err)
	}
	for _, testCase := range []struct {
		target string
		write  func() error
	}{
		{target: TargetClaude, write: func() error {
			_, err := ConvertToClaudeTranscript(selected, ClaudeTargetOptions{})
			return err
		}},
		{target: TargetPi, write: func() error {
			_, err := ConvertToPiSession(selected, PiTargetOptions{})
			return err
		}},
		{target: TargetOMP, write: func() error {
			_, err := ConvertToOMPSession(selected, OMPTargetOptions{})
			return err
		}},
		{target: TargetOpenCode, write: func() error {
			_, err := ConvertToOpenCodeSession(selected, OpenCodeTargetOptions{})
			return err
		}},
	} {
		err := testCase.write()
		if !errors.Is(err, ErrSelectionUnsupportedWriter) {
			t.Errorf("writing the selected document to %s: err = %v, want ErrSelectionUnsupportedWriter",
				testCase.target, err)
			continue
		}
		if !strings.Contains(err.Error(), testCase.target) {
			t.Errorf("the %s writer's refusal does not name the target it is: %v", testCase.target, err)
		}
	}
	// The reference route's writer consumes the seam, so the same document is written rather than
	// refused: the boundary is the writer's own consumption, not the document.
	if _, err := ConvertToCodexRollout(selected, testOptions()); err != nil {
		t.Errorf("the Codex writer refused a document a selection produced: %v", err)
	}
}

// TestSelectionKeepsTheNearestCarriedAncestor pins D6's ancestry disposition: for every carried record,
// the seam answers the nearest ancestor the read holds that the selection also carries -- what a writer
// whose target format is a tree chains an entry to when the record's own parent was cut. The reference
// route's Codex writer writes one sequence with no parent relation and chains nothing, so the answer is
// carried for the tree-target increments; this case is what keeps it from being computed wrongly (or
// re-derived) there.
func TestSelectionKeepsTheNearestCarriedAncestor(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "a", Dialogue: "a", SourceUUID: "a0"},
		Message{Ordinal: 1, Role: "assistant", Content: "b", Dialogue: "b", SourceUUID: "b1",
			SourceParentUUID: "a0"},
		Message{Ordinal: 2, Role: "user", Content: "c", Dialogue: "c", SourceUUID: "c2",
			SourceParentUUID: "b1"},
		Message{Ordinal: 3, Role: "assistant", Content: "d", Dialogue: "d", SourceUUID: "d3",
			SourceParentUUID: "c2"},
		Message{Ordinal: 4, Role: "assistant", Content: "e", Dialogue: "e", SourceUUID: "e4"},
	)
	selected, _, err := Select(conversation, "0,3", false)
	if err != nil {
		t.Fatalf("selecting the chain's ends: %v", err)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{0, 3}) {
		t.Fatalf("carried ordinals = %v, want the two the specification named", got)
	}
	// a's own ancestry is outside the read (it names none) -> -1; d's parent chain runs through the
	// excluded b and c to the carried a -> position 0 in the reduced document.
	if got := selected.sourceKeptAncestor; !slices.Equal(got, []int{-1, 0}) {
		t.Errorf("kept ancestors = %v, want [-1 0]: d chains to the nearest carried ancestor a", got)
	}
	// A record whose whole ancestry lies outside the window answers no ancestor, and so does a record the
	// read gave no identity.
	selected, _, err = Select(conversation, "3,4", false)
	if err != nil {
		t.Fatalf("selecting the tail: %v", err)
	}
	if got := selected.sourceKeptAncestor; !slices.Equal(got, []int{-1, -1}) {
		t.Errorf("kept ancestors = %v, want [-1 -1]: neither record's ancestry is carried", got)
	}
}

// TestSelectionTimeStampsAreNeverOlderThanTheReadsRecord pins that a selected window's written records
// keep the read's own instants: the selection changes which records travel, never when they were
// recorded, so every written message record is stamped with an instant the read recorded for one of its
// records and never one this build invented.
func TestSelectionTimeStampsAreNeverOlderThanTheReadsRecord(t *testing.T) {
	conversation := selectionFixtureConversation(t)
	selected, _, err := Select(conversation, "0-4", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	// Every instant the read recorded for its own records: the window's messages are 08:00:00..08:00:36,
	// and the writer's clock rule substitutes the previous written record's instant where the document's
	// order disagrees with its clock -- also an instant the read recorded. The citation claims the
	// writing keeps the read's own instants, so the case asks membership in this set rather than a bound.
	recorded := make(map[int64]struct{}, len(conversation.Messages))
	for _, message := range conversation.Messages {
		recorded[message.Timestamp.UnixNano()] = struct{}{}
	}
	lines := decodeRollout(t, rollout.Data)
	for _, line := range lines {
		if line.Type != codexRecordResponseItem {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(line.Payload, &payload); err != nil {
			t.Fatalf("decoding a payload: %v", err)
		}
		if payload["type"] != codexItemMessage {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			t.Fatalf("parsing %q: %v", line.Timestamp, err)
		}
		if _, ok := recorded[at.UnixNano()]; !ok {
			t.Errorf("a written record is stamped %s, which is no instant the read recorded", line.Timestamp)
		}
	}
}

// fixedSourceReads answers every read with one document a case built: the Continue-level cases drive the
// engine's whole flow -- the pair gate, the boundary question, the reduction, the writer and the refusal
// -- over a document no committed fixture can produce, and an injected reader is where the document
// enters (SourceReads).
type fixedSourceReads struct {
	conversation Conversation
}

func (r *fixedSourceReads) Read(context.Context, Ref) (Conversation, error) {
	return r.conversation, nil
}

// selectionContinueConversation is the document the Continue-level cases drive: a call whose recorded
// result has no text the target's output could carry. The source file's own path is cleared, because no
// file stands behind a document a case built: the boundary answers "no path" instead of stopping the run
// on a file that does not exist, exactly as it does for an archive read.
func selectionContinueConversation(t *testing.T) Conversation {
	t.Helper()
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "calling", Dialogue: "calling",
			SourceUUID: "c1",
			ToolCalls:  []ToolCall{{ToolUseID: "call_empty_result", Name: "Bash", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "r2", SourceParentUUID: "c1",
			ToolResults: []ToolResult{{ToolUseID: "call_empty_result"}}},
	)
	conversation.Source.SourcePath = ""
	conversation.Source.DisplayPath = ""
	return conversation
}

// selectionContinueFiles counts the files below a directory, so a refused run can be shown to have
// installed nothing at all rather than only that one expected file is absent.
func selectionContinueFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return count
}

// TestSelectionRefusesACounterpartTheWriterCannotCarry pins D3's writer-side closure eligibility: a
// selection that separates a recorded call from its recorded result carries the counterpart, but only
// where the route can represent it -- and this route's output carries text, so a result whose recorded
// text is empty is one the walk drops (empty_tool_results). The whole selection is refused by name
// before anything is installed (the pair and both ordinals, and the category the counterpart was
// counted under), rather than publishing a call whose declared necessity was silently dropped. The
// same records named explicitly are not refused: the result is then the caller's own selection rather
// than a declared necessity, and D1's accounting discloses it lost under its own category.
func TestSelectionRefusesACounterpartTheWriterCannotCarry(t *testing.T) {
	conversation := selectionContinueConversation(t)
	selected, report, err := Select(conversation, "0", false)
	if err != nil {
		t.Fatalf("selecting the call alone: %v", err)
	}
	if report.NecessitiesAdded.Count != 1 || report.NecessitiesAdded.Added[0].Ordinal != 1 ||
		report.NecessitiesAdded.Added[0].PulledBy != 0 {
		t.Fatalf("necessities_added = %+v, want the empty-text result at ordinal 1 pulled in by 0",
			report.NecessitiesAdded)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "empty_tool_results"); !ok || count != 1 {
		t.Fatalf("empty_tool_results = %d (present=%v), want the counterpart dropped by its own shape",
			count, ok)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	fillSelectionUndischargedSplit(&report)
	err = lostNecessityRefusal(&report)
	if !errors.Is(err, ErrSelectionUnrepresentable) {
		t.Fatalf("lostNecessityRefusal = %v, want ErrSelectionUnrepresentable", err)
	}
	for _, want := range []string{"ordinal 0", "ordinal 1", "empty_tool_results"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}

	// The engine's own flow: the same document read through an injected reader, the same selection, and
	// the refusal answered before anything is installed -- the target home stays empty and the run's
	// error is the refusal, not a conversion.
	home := t.TempDir()
	_, err = Continue(context.Background(), ContinueOptions{
		Ref:               Ref{Provider: "claude", ID: "selection-empty-result"},
		Target:            TargetCodex,
		Config:            fixtureConfig(t),
		Reader:            &fixedSourceReads{conversation: conversation},
		TargetHome:        home,
		TargetBinary:      missingTargetBinary,
		Selection:         "0",
		SelectionSupplied: true,
		Verify:            VerifyNone,
	})
	if !errors.Is(err, ErrSelectionUnrepresentable) {
		t.Fatalf("Continue with the split selection: err = %v, want ErrSelectionUnrepresentable", err)
	}
	if !strings.Contains(err.Error(), "ordinal 1") || !strings.Contains(err.Error(), "empty_tool_results") {
		t.Errorf("the engine's refusal does not name the counterpart and its category: %v", err)
	}
	if files := selectionContinueFiles(t, home); files != 0 {
		t.Errorf("a refused selection wrote %d file(s) into the target home", files)
	}

	// The explicit selection of both records is not refused: the caller named the result itself, so its
	// loss with a reason is D1's accounting rather than a declared necessity the writer dropped.
	selected, report, err = Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting both records: %v", err)
	}
	rollout, err = ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the explicit pair: %v", err)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	if err := lostNecessityRefusal(&report); err != nil {
		t.Errorf("an explicitly selected pair was refused: %v", err)
	}
	if len(report.Records) != 2 || report.Records[1].Necessity ||
		report.Records[1].Became != BecameLost || report.Records[1].Reason != "empty_tool_results" {
		t.Errorf("the explicit pair's rows = %+v, want the result lost under empty_tool_results and "+
			"not marked a necessity", report.Records)
	}
}

// TestSelectionNamesAnExcludedOwnerRatherThanALinklessResult pins D6(c) on the shape the reduced stream
// cannot state: the progress event alone is selected and its recorded call is left out. The read paired
// the two, so the writer must count the event under its own category (tool_results_of_excluded_calls)
// rather than under the category for a result the source never linked (unpaired_tool_results) -- and the
// mutation removes that one carried disposition while leaving selected mode on, so the writer falls into
// the link-less category and the arm turns red.
func TestSelectionNamesAnExcludedOwnerRatherThanALinklessResult(t *testing.T) {
	const progressText = "Still working"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "spawning", Dialogue: "spawning",
			SourceUUID: "p1",
			ToolCalls:  []ToolCall{{ToolUseID: "call_wait", Name: "wait"}}},
		Message{Ordinal: 1, Role: "user", SourceUUID: "p2", SourceParentUUID: "p1",
			ToolResults: []ToolResult{{ToolUseID: "call_wait", Status: "running",
				ContentRaw: progressText, ContentSize: len(progressText)}}},
	)
	selected, report, err := Select(conversation, "1", false)
	if err != nil {
		t.Fatalf("selecting the progress event: %v", err)
	}
	if got := selectionOrdinals(selected); !slices.Equal(got, []int{1}) {
		t.Fatalf("carried ordinals = %v, want the progress record alone", got)
	}
	if report.Excluded.Count != 1 || strings.Join(report.Excluded.Regions, ",") != "0" {
		t.Fatalf("excluded = %+v, want the call's record left out", report.Excluded)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "tool_results_of_excluded_calls"); !ok || count != 1 {
		t.Errorf("tool_results_of_excluded_calls = %d (present=%v), want the event counted under the "+
			"owner the selection left out", count, ok)
	}
	if count, _ := optionalLoss(rollout.Losses, "unpaired_tool_results"); count != 0 {
		t.Errorf("unpaired_tool_results = %d, want 0: the read paired this event, so its category is "+
			"the excluded owner, not a link the source never recorded", count)
	}
	if rolloutHolds(rollout.Data, progressText) {
		t.Errorf("the rollout holds the progress text: no record in the target states the event")
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	if len(report.Records) != 1 || report.Records[0].Became != BecameLost ||
		report.Records[0].Reason != "tool_results_of_excluded_calls" {
		t.Errorf("the event's row = %+v, want it lost under tool_results_of_excluded_calls",
			report.Records)
	}

	// The mutation: the same reduced document with only the excluded-owner disposition gone, while
	// selected mode stays on. The writer then reads the result as one the source never linked -- the
	// collapse the carried disposition exists to prevent.
	mutated := selected
	mutated.sourceExcludedResultOwners = map[resultOccurrence]bool{}
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	if err != nil {
		t.Fatalf("converting the mutated window: %v", err)
	}
	if count, ok := optionalLoss(mutatedRollout.Losses, "unpaired_tool_results"); !ok || count != 1 {
		t.Errorf("the mutation did not turn the arm red: unpaired_tool_results = %d (present=%v), want "+
			"the link-less category the mutation produces", count, ok)
	}
}

// TestSelectionAttributesASystemRecordsOmissionToItsRow pins D1's reason column for a record the writer
// drops by its kind: a selected standalone system reminder is counted (system_records), and its row must
// say so rather than fall back to the sentence for a record no count explains -- the row's whole contract
// is that a lost record's reason is the category behind its loss.
func TestSelectionAttributesASystemRecordsOmissionToItsRow(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "a system reminder", IsSystem: true,
			SourceUUID: "s1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the answer", Dialogue: "the answer",
			SourceUUID: "a2", SourceParentUUID: "s1"},
	)
	selected, report, err := Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting both records: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count, ok := optionalLoss(rollout.Losses, "system_records"); !ok || count != 1 {
		t.Fatalf("system_records = %d (present=%v), want the reminder counted", count, ok)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	if len(report.Records) != 2 {
		t.Fatalf("the report accounts for %d record(s), want the two the specification named",
			len(report.Records))
	}
	if report.Records[0].Became != BecameLost || report.Records[0].Reason != "system_records" {
		t.Errorf("the reminder's row = %+v, want it lost under system_records -- the category the "+
			"writer counted it under, not the fallback sentence", report.Records[0])
	}
}

// TestSelectionReconcilesUndischargedCallsWithTheWritersFates pins D6(b) and D1 on the shape the two
// report surfaces could disagree about: a selected sidechain record carries a call the source never
// answered, and the writer drops the whole record (sidechain_messages). calls_undischarged still counts
// the source's own state among the carried records (1), but its split says those calls did not travel
// (calls_undischarged_lost = 1) while the conversion's outstanding_tool_calls stays 0 -- and the row's
// own fate is the category the call went under, so total minus lost is what the target holds.
func TestSelectionReconcilesUndischargedCallsWithTheWritersFates(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "subagent turn", Dialogue: "subagent turn",
			IsSidechain: true, SourceUUID: "s1",
			ToolCalls: []ToolCall{{ToolUseID: "call_sub", Name: "Bash", InputJSON: `{}`}}},
		Message{Ordinal: 1, Role: "assistant", Content: "after", Dialogue: "after", SourceUUID: "a2"},
	)
	selected, report, err := Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Fatalf("calls_undischarged = %+v, want the source's own unanswered call on record 0",
			report.CallsUndischarged)
	}
	rollout, err := ConvertToCodexRollout(selected, testOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count, _ := optionalLoss(rollout.Losses, "outstanding_tool_calls"); count != 0 {
		t.Fatalf("outstanding_tool_calls = %d, want 0: the call's record was dropped whole", count)
	}
	if count, ok := optionalLoss(rollout.Losses, "sidechain_messages"); !ok || count != 1 {
		t.Fatalf("sidechain_messages = %d (present=%v), want the record's own category", count, ok)
	}
	fillSelectionRecords(&report, rollout.RecordFates)
	fillSelectionUndischargedSplit(&report)
	if report.CallsUndischargedLost.Count != 1 ||
		strings.Join(report.CallsUndischargedLost.Regions, ",") != "0" {
		t.Errorf("calls_undischarged_lost = %+v, want the one call whose record was not published",
			report.CallsUndischargedLost)
	}
	if len(report.Records) != 2 || report.Records[0].CallsUndischarged != 1 ||
		report.Records[0].Became != BecameLost || report.Records[0].Reason != "sidechain_messages" {
		t.Errorf("record 0's row = %+v, want one undischarged call lost under sidechain_messages",
			report.Records[0])
	}
	// The reconciliation the split exists for: the calls the target holds with no outcome are exactly
	// the class minus its lost part.
	outstanding, _ := optionalLoss(rollout.Losses, "outstanding_tool_calls")
	if written := report.CallsUndischarged.Count - report.CallsUndischargedLost.Count; written != outstanding {
		t.Errorf("the class minus its lost part = %d, but outstanding_tool_calls = %d: the two report "+
			"surfaces do not reconcile", written, outstanding)
	}
}
