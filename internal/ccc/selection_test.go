package ccc

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The selection fixture's own recorded ids and strings, spelled here rather than read out of the read:
// the cases assert that the *fixture's* words and call ids are what crossed, so a conversion that mangled
// either cannot pass by agreeing with the read it produced.
const (
	selectionBuildCallID = "toolu_09Selection0000000000000001"
	selectionReadCallID  = "toolu_09Selection0000000000000002"
	selectionCapCallID   = "toolu_09Selection0000000000000003"
	selectionBuildResult = "build-ok: 3 packages compiled"
	selectionCapResult   = "internal/retry/config.go:41: maxBackoffSeconds = 30"

	selectionQuestion = "Start the long build"
	selectionAnswer   = "The cap is maxBackoffSeconds = 30"
	selectionSummary  = "Summary: the build succeeded"
	selectionFollowup = "And the cap?"
)

// selectionProbeOptions is what the selection cases hand each route's writer: fixed answers, so a written
// document is a function of the carried conversation alone.
func selectionProbeOptions() targetConvertInput {
	return targetConvertInput{
		CLIVersion: "selection-test-0.0",
		Provider:   "selection-test-provider",
		Model:      "selection-test-model",
		Now:        time.Date(2026, 5, 8, 8, 0, 0, 0, time.UTC),
	}
}

// readSelectionFixture reads the third Claude fixture, the shape the selection's rules are pinned on.
func readSelectionFixture(t *testing.T) Conversation {
	t.Helper()
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), claudeSelectionFixtureRef)
	if err != nil {
		t.Fatalf("reading the selection fixture: %v", err)
	}
	return conversation
}

// selectionOrdinals answers the ordinal values a conversation's records hold, in read order.
func selectionOrdinals(conversation Conversation) []int {
	ordinals := make([]int, 0, len(conversation.Messages))
	for _, message := range conversation.Messages {
		ordinals = append(ordinals, message.Ordinal)
	}
	return ordinals
}

// TestSelectionMatchesTheOrdinalValuesTheReadSupplied pins the selector's whole space: a specification
// names the ordinal *values* the read supplied, never a position, and every refusal is decided against
// that space. The Shelley-shaped document is the measured shape that makes the difference visible
// (internal/parser/shelley.go assigns the source's own `sequence_id`, so five records carry ordinals
// 1..5 and a positional reading of `0-` would silently select a different conversation).
func TestSelectionMatchesTheOrdinalValuesTheReadSupplied(t *testing.T) {
	shelley := Conversation{Messages: []Message{
		{Ordinal: 1, Role: "user", Content: "one"},
		{Ordinal: 2, Role: "assistant", Content: "two"},
		{Ordinal: 3, Role: "user", Content: "three"},
		{Ordinal: 4, Role: "assistant", Content: "four"},
		{Ordinal: 5, Role: "user", Content: "five"},
	}}

	selected, report, err := Select(shelley, "1-5", false)
	if err != nil {
		t.Fatalf("selecting 1-5: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Errorf("carried ordinals = %v, want all five records", got)
	}
	if report.Space != (OrdinalSpace{Min: 1, Max: 5, Records: 5, Contiguous: true}) {
		t.Errorf("space = %+v, want the read's own values 1..5", report.Space)
	}
	if report.Included.Count != 5 || len(report.Included.Regions) != 1 || report.Included.Regions[0] != "1-5" {
		t.Errorf("included = %+v, want the five records as one region", report.Included)
	}
	if report.Excluded.Count != 0 || report.NecessitiesAdded.Count != 0 || report.CallsUndischarged.Count != 0 {
		t.Errorf("report = %+v, want no exclusion, no necessity and no undischarged call", report)
	}

	// An open region runs from its own value through the highest the read holds.
	selected, report, err = Select(shelley, "3-", false)
	if err != nil {
		t.Fatalf("selecting 3-: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Errorf("carried ordinals = %v, want 3,4,5", got)
	}
	if report.Excluded.Count != 2 {
		t.Errorf("excluded = %+v, want the two records below 3", report.Excluded)
	}

	// A value the space does not hold is refused naming the space, and an open region checks its own
	// first value -- `0-` on this space is not read as "from the beginning".
	for _, spec := range []string{"0", "0-", "6", "0-5"} {
		if _, _, err := Select(shelley, spec, false); !errors.Is(err, ErrBadSelection) {
			t.Errorf("selecting %q on a 1..5 space: err = %v, want a refusal", spec, err)
		} else if !strings.Contains(err.Error(), "ordinals 1-5") {
			t.Errorf("selecting %q: %v does not name the space the read supplied", spec, err)
		}
	}

	// A space with gaps: an in-space value that names no record selects nothing, while a region that
	// reaches across a gap carries the records it does hold.
	gappy := Conversation{Messages: []Message{
		{Ordinal: 0, Role: "user", Content: "zero"},
		{Ordinal: 2, Role: "assistant", Content: "two"},
		{Ordinal: 5, Role: "user", Content: "five"},
	}}
	_, report, err = Select(gappy, "0-5", false)
	if err != nil {
		t.Fatalf("selecting 0-5 on a gappy space: %v", err)
	}
	if report.Space.Contiguous || report.Space.Records != 3 {
		t.Errorf("space = %+v, want three records that are not contiguous", report.Space)
	}
	if len(report.Included.Regions) != 3 {
		t.Errorf("included regions = %v, want the read's own values spelled one by one", report.Included.Regions)
	}
	selected, _, err = Select(gappy, "2-4", false)
	if err != nil {
		t.Fatalf("selecting 2-4 on a gappy space: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 2 {
		t.Errorf("carried ordinals = %v, want the one record the region holds", got)
	}
	if _, _, err := Select(gappy, "1", false); !errors.Is(err, ErrBadSelection) {
		t.Errorf("selecting a value that falls in the gap: err = %v, want a refusal", err)
	} else if !strings.Contains(err.Error(), "falls between") {
		t.Errorf("selecting a value that falls in the gap: %v, want the sentence that names the gap", err)
	}
}

// TestSelectionRefusesABadSpecificationBySentence pins D5: an ill-formed specification is refused with the
// core's own sentence and nothing is carried, instead of being repaired into a spec the caller did not
// write. Every case names the substring the sentence has to hold, so a refusal that lost its reason is a
// failing case rather than a passing one.
func TestSelectionRefusesABadSpecificationBySentence(t *testing.T) {
	conversation := readSelectionFixture(t)
	cases := []struct {
		spec string
		want string
	}{
		{"", "the specification is empty"},
		{"   ", "the specification is empty"},
		{"1,", "empty region"},
		{"a", "is not an ordinal value"},
		{"1-x", "is not an ordinal value"},
		{"-3", "is not an ordinal value"},
		{"1.5", "is not an ordinal value"},
		{"5-3", "runs backwards"},
		{"1-2-3", "is not a region"},
		{"1-3,3", "already names"},
		{"1-3,2-", "already names"},
		{"11", "do not hold"},
		{"3-11", "do not hold"},
		{"11-", "do not hold"},
	}
	for _, testCase := range cases {
		_, _, err := Select(conversation, testCase.spec, false)
		if !errors.Is(err, ErrBadSelection) {
			t.Errorf("selecting %q: err = %v, want ErrBadSelection", testCase.spec, err)
			continue
		}
		if !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("selecting %q: %v does not hold %q", testCase.spec, err, testCase.want)
		}
	}

	// The specifications a caller actually writes are accepted, gaps and all: a region's values do not
	// have to be a read's own.
	for _, spec := range []string{"0", "0-3", "0-3,5", "2-3,5-6", "5-", "10", "0-10"} {
		if _, _, err := Select(conversation, spec, false); err != nil {
			t.Errorf("selecting %q: err = %v, want it honoured", spec, err)
		}
	}
}

// TestCheckSelectionOptionsRefusesASpecificationWithoutARead pins the half of the grammar that needs no
// read: a caller embedding the core and one driving the CLI are told the same thing about the same
// command line, before anything is opened.
func TestCheckSelectionOptionsRefusesASpecificationWithoutARead(t *testing.T) {
	if err := CheckSelectionOptions("", false, false); err != nil {
		t.Errorf("no selection: err = %v, want it accepted (the flag is absent)", err)
	}
	if err := CheckSelectionOptions("0-5", false, false); err != nil {
		t.Errorf("a well-formed spec: err = %v, want it accepted", err)
	}
	if err := CheckSelectionOptions("5-3", false, false); !errors.Is(err, ErrBadSelection) {
		t.Errorf("an inverted region: err = %v, want ErrBadSelection", err)
	}
	// The duplicate-region half needs no read either: `--select 1,1` is the caller's own command line, and
	// a missing or unreadable source must not mask it as a source failure. The check is decided on the
	// specification's own regions (an open one included), never on the ordinals a read happens to hold.
	for _, spec := range []string{"1,1", "1-3,3", "1,1-", "1-2,2-3"} {
		err := CheckSelectionOptions(spec, false, false)
		if !errors.Is(err, ErrBadSelection) {
			t.Errorf("a region named twice (%q): err = %v, want ErrBadSelection", spec, err)
		} else if !strings.Contains(err.Error(), "already names") {
			t.Errorf("a region named twice (%q): %v does not name the duplicate region", spec, err)
		}
	}
	// A value the caller supplied but left blank is a refused request, not the flag's absence: an unset
	// shell variable expanded into --select "$SPEC" must not publish the whole history.
	for _, spec := range []string{"", "   "} {
		err := CheckSelectionOptions(spec, false, true)
		if !errors.Is(err, ErrBadSelection) {
			t.Errorf("a supplied blank value (%q): err = %v, want ErrBadSelection", spec, err)
		} else if !strings.Contains(err.Error(), "the specification is empty") {
			t.Errorf("a supplied blank value (%q): %v does not name the empty specification", spec, err)
		}
	}
	err := CheckSelectionOptions("", true, false)
	if !errors.Is(err, ErrBadSelection) {
		t.Errorf("--strict-selection without a selection: err = %v, want a refusal", err)
	} else if !strings.Contains(err.Error(), "--strict-selection") {
		t.Errorf("--strict-selection without a selection: %v does not name the flag", err)
	}
}

// TestSelectionCarriesTheCounterpartOfASeparatedCallAndResult pins the seam's default arm in both
// directions: a selection that carries a recorded call without its recorded result (or a recorded result
// without its recorded call) carries the counterpart instead, names it with the ordinal that pulled it in,
// and the target writer then holds the pair -- not a call whose output stayed behind, and not a tool
// result the writer could only count as unpaired.
func TestSelectionCarriesTheCounterpartOfASeparatedCallAndResult(t *testing.T) {
	conversation := readSelectionFixture(t)
	cases := []struct {
		spec      string
		carried   []int
		necessity SelectionNecessity
	}{
		// The call is selected and its result is not (the result's record is the very next one).
		{"2", []int{2, 3}, SelectionNecessity{Ordinal: 3, PulledBy: 2}},
		// The result is selected and its call is not.
		{"3", []int{2, 3}, SelectionNecessity{Ordinal: 2, PulledBy: 3}},
		// The second pair straddles a record the provider marked system-injected.
		{"8", []int{7, 8}, SelectionNecessity{Ordinal: 7, PulledBy: 8}},
		{"7", []int{7, 8}, SelectionNecessity{Ordinal: 8, PulledBy: 7}},
	}
	for _, testCase := range cases {
		selected, report, err := Select(conversation, testCase.spec, false)
		if err != nil {
			t.Fatalf("selecting %q: %v", testCase.spec, err)
		}
		if got := selectionOrdinals(selected); len(got) != len(testCase.carried) {
			t.Errorf("selecting %q: carried ordinals = %v, want %v", testCase.spec, got, testCase.carried)
		} else {
			for index, want := range testCase.carried {
				if got[index] != want {
					t.Errorf("selecting %q: carried ordinal %d = %d, want %d", testCase.spec, index, got[index], want)
				}
			}
		}
		if report.NecessitiesAdded.Count != 1 || len(report.NecessitiesAdded.Added) != 1 ||
			report.NecessitiesAdded.Added[0] != testCase.necessity {
			t.Errorf("selecting %q: necessities = %+v, want %+v", testCase.spec,
				report.NecessitiesAdded, testCase.necessity)
		}
		if report.Included.Count != 1 {
			t.Errorf("selecting %q: included = %+v, want the one record the spec named", testCase.spec, report.Included)
		}
	}

	// The pair reaches the target as a pair: the writer holds the call and one output for it, and counts
	// no unpaired result.
	selected, _, err := Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting 2: %v", err)
	}
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the selection: %v", target, err)
		}
		if converted.Carried.ToolCalls != 1 || converted.Carried.ToolResults != 1 {
			t.Errorf("%s: carried = %+v, want the call and the one result the necessity carried",
				target, converted.Carried)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: the counterpart rule carried the call the "+
				"result answers", target, count)
		}
		bytes := string(converted.Data)
		for _, want := range []string{selectionBuildCallID, selectionBuildResult} {
			if !strings.Contains(bytes, want) {
				t.Errorf("%s: the written document does not hold %q", target, want)
			}
		}
	}
}

// TestSelectionCarriesASystemRecordAndReportsUndischargedCalls pins the dispositions the seam does not
// change: a compaction boundary and a system record the selection names are carried, a call the source
// itself left undischarged stays exactly as it is and is counted -- the counterpart rule cannot carry a
// result the read never held -- and the system record's own system-injected text is not written as a
// message: the route counts the record under its own `system_records` category while the tool evidence
// recorded on the same record travels.
func TestSelectionCarriesASystemRecordAndReportsUndischargedCalls(t *testing.T) {
	conversation := readSelectionFixture(t)

	// The undischarged call alone: it is carried, and the report names it.
	selected, report, err := Select(conversation, "5", false)
	if err != nil {
		t.Fatalf("selecting 5: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 5 {
		t.Errorf("carried ordinals = %v, want the record that made the call", got)
	}
	if report.CallsUndischarged.Count != 1 || strings.Join(report.CallsUndischarged.Regions, ",") != "5" {
		t.Errorf("calls_undischarged = %+v, want the one call the source never answered, at ordinal 5",
			report.CallsUndischarged)
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities = %+v, want none: the read holds no result for that call",
			report.NecessitiesAdded)
	}
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the undischarged call: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want the call alone", rollout.Carried)
	}
	if !strings.Contains(string(rollout.Data), selectionReadCallID) {
		t.Error("the written document does not hold the undischarged call's own id")
	}

	// A window that holds the boundary, the second pair and the system record: everything the window names
	// travels, the undischarged call inside it is still counted, and nothing is added.
	selected, report, err = Select(conversation, "5-10", false)
	if err != nil {
		t.Fatalf("selecting 5-10: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 6 {
		t.Errorf("carried ordinals = %v, want the six records of the window", got)
	}
	if report.CallsUndischarged.Count != 1 {
		t.Errorf("calls_undischarged = %+v, want the one call inside the window", report.CallsUndischarged)
	}
	if report.NecessitiesAdded.Count != 0 || report.Excluded.Count != 5 {
		t.Errorf("report = %+v, want no necessity and the five records before the window excluded", report)
	}
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if converted.Carried.CompactionBoundaries != 1 {
			t.Errorf("%s: carried = %+v, want the one compaction boundary inside the window",
				target, converted.Carried)
		}
		// Both calls inside the window reach the target, and the one result the read holds for them: the
		// read call is the call the source never answered. OpenCode's own format is the exception, and its
		// own report says why: the harness's importer accepts only a completed or failed tool state, so a
		// call with no outcome has no shape there and is left out and counted (unpaired_tool_calls).
		wantCalls, wantResults := 2, 1
		if target == TargetOpenCode {
			wantCalls = 1
		}
		if converted.Carried.ToolCalls != wantCalls || converted.Carried.ToolResults != wantResults {
			t.Errorf("%s: carried = %+v, want %d call(s) with the %d result(s) the read holds for them",
				target, converted.Carried, wantCalls, wantResults)
		}
		if target == TargetOpenCode {
			if count := lossCount(t, converted.Losses, "unpaired_tool_calls").Count; count != 1 {
				t.Errorf("%s: unpaired_tool_calls = %d, want the undischarged call counted by the route "+
					"whose format has no state for it", target, count)
			}
		}
		bytes := string(converted.Data)
		held := []string{selectionCapCallID, selectionCapResult}
		if target != TargetOpenCode {
			held = append(held, selectionReadCallID)
		}
		for _, want := range held {
			if !strings.Contains(bytes, want) {
				t.Errorf("%s: the written document does not hold %q", target, want)
			}
		}
		if target == TargetOpenCode && strings.Contains(bytes, selectionReadCallID) {
			t.Errorf("%s: the written document holds the undischarged call's id, which this route's own "+
				"report says it cannot write", target)
		}
		// The system record's own text is not dialogue: the target writes its tool evidence and the record
		// itself is counted under the route's own category rather than written as a user turn -- so the
		// case asserts both halves, the text's absence and the count's presence, rather than choosing a
		// rendering every format family holds differently.
		if strings.Contains(bytes, "the workspace state was restored") {
			t.Errorf("%s: the written document holds the system record's own text, so a record the "+
				"provider flagged as system-injected is presented as the conversation's own turn", target)
		}
		if count := lossCount(t, converted.Losses, "system_records").Count; count != 1 {
			t.Errorf("%s: system_records = %d, want the one system record inside the window counted by "+
				"the route", target, count)
		}
		for _, unwanted := range []string{selectionQuestion, selectionBuildResult} {
			if strings.Contains(bytes, unwanted) {
				t.Errorf("%s: the written document holds %q, which the selection excluded", target, unwanted)
			}
		}
	}
}

// TestSelectionEndingOffTheReadsLine pins the leaf/inactive rule under a selection: the line the target's
// loader walks is the read's own -- the walk from its last record -- and a window a selection carries must
// not redefine it. The Pi history fixture carries exactly that shape: its last record is the file's own
// last entry (so the source-side leaf check passes), and the branch the conversation left before that
// turn is still in the file, three records the read's own walk places off its line. A window ending on
// those records is not written as if the branch were the resumed context: the tree routes refuse by name
// (ErrUnrepresentableLeaf), and the line routes omit the off-line records and count them
// (inactive_branch_records), the same category the unflagged conversion counts for the whole read.
func TestSelectionEndingOffTheReadsLine(t *testing.T) {
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piHistoryFixtureRef)
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	if len(conversation.Messages) != 6 {
		t.Fatalf("the fixture's read holds %d record(s), want the six its file carries",
			len(conversation.Messages))
	}
	// The fixture's own shape, restated from its recorded relations rather than read out of the function
	// under test: the walk from the read's last record reaches ordinals 0, 1 and 5, while the branch the
	// conversation left (the compaction summary and the retry-cap question and answer) sits at 2-4.
	const (
		onLineFirst  = "How tall is the tower in the plaza?"
		onLineSecond = "The tower in the plaza is 42 metres tall."
		abandoned    = "And the retry cap?"
		abandonedToo = "Thirty seconds, in internal/retry/config.go."
	)

	// The window 0-4: the read's on-line prefix plus the whole off-line branch, so the window's own last
	// record (ordinal 4) is one the read's walk never reached.
	selected, report, err := Select(conversation, "0-4", false)
	if err != nil {
		t.Fatalf("selecting 0-4: %v", err)
	}
	if report.Included.Count != 5 || report.Excluded.Count != 1 {
		t.Errorf("selecting 0-4: included %d, excluded %d, want the five records of the window and the one "+
			"after it", report.Included.Count, report.Excluded.Count)
	}

	// A tree route refuses: the entry written last would stand for an off-line record, and the target's
	// own loader walks from it -- the branch revived as the resumed context the guard exists to refuse.
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Errorf("%s: converting a window ending off the read's line: err = %v, want "+
				"ErrUnrepresentableLeaf", target, err)
		}
	}

	// A line route writes only the records the read's own line reaches and counts the branch it cannot
	// hold -- the same three records the unflagged conversion of the whole read omits.
	route, err := targetRouteFor(TargetCodex)
	if err != nil {
		t.Fatalf("route %s: %v", TargetCodex, err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the selection: %v", TargetCodex, err)
	}
	if count := lossCount(t, converted.Losses, "inactive_branch_records").Count; count != 3 {
		t.Errorf("%s: inactive_branch_records = %d, want the branch's three off-line records (ordinals 2-4)",
			TargetCodex, count)
	}
	bytes := string(converted.Data)
	for _, want := range []string{onLineFirst, onLineSecond} {
		if !strings.Contains(bytes, want) {
			t.Errorf("%s: the written document does not hold %q, a record on the read's own line",
				TargetCodex, want)
		}
	}
	for _, unwanted := range []string{abandoned, abandonedToo} {
		if strings.Contains(bytes, unwanted) {
			t.Errorf("%s: the written document holds %q: a record written into this target's log becomes "+
				"model-visible history, so the branch was revived as an active turn", TargetCodex, unwanted)
		}
	}

	// A window that ends on the read's own line is written: the refusal is about the window's own last
	// record, not about the records a window leaves behind.
	selected, _, err = Select(conversation, "0-1", false)
	if err != nil {
		t.Fatalf("selecting 0-1: %v", err)
	}
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude, TargetCodex} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting a window ending on the read's line: %v", target, err)
		}
	}
}

// TestSelectionRefusesASeparatedPairUnderStrictSelection pins D3's second direction: the same selection
// with --strict-selection is refused by name, naming the calls and both ordinals, before anything is
// carried -- and a selection that separates nothing is honoured with the flag set.
func TestSelectionRefusesASeparatedPairUnderStrictSelection(t *testing.T) {
	conversation := readSelectionFixture(t)
	cases := []struct {
		spec string
		call string
		// want holds the sentence halves the refusal has to state, each with the ordinal of the side it
		// names: the refusal has to say which record the selection named and which one it left behind, so
		// a case that checked only the id and the sentence's shape would pass a refusal that swapped the
		// two ordinals or dropped them.
		want []string
	}{
		{"2", selectionBuildCallID, []string{
			"recorded at ordinal 2 is selected", "its recorded result at ordinal 3 is not"}},
		{"3", selectionBuildCallID, []string{
			"recorded at ordinal 3 is selected", "the call itself at ordinal 2 is not"}},
		{"7", selectionCapCallID, []string{
			"recorded at ordinal 7 is selected", "its recorded result at ordinal 8 is not"}},
		{"8", selectionCapCallID, []string{
			"recorded at ordinal 8 is selected", "the call itself at ordinal 7 is not"}},
	}
	for _, testCase := range cases {
		selected, report, err := Select(conversation, testCase.spec, true)
		if !errors.Is(err, ErrStrictSelection) {
			t.Errorf("strictly selecting %q: err = %v, want ErrStrictSelection", testCase.spec, err)
			continue
		}
		for _, want := range append([]string{testCase.call, "--strict-selection"}, testCase.want...) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("strictly selecting %q: %v does not hold %q", testCase.spec, err, want)
			}
		}
		if len(selected.Messages) != 0 || report.Spec != "" {
			t.Errorf("strictly selecting %q: a refused selection answered a conversation and a report",
				testCase.spec)
		}
	}

	// A selection that separates nothing is honoured, and the report says which arm ran.
	selected, report, err := Select(conversation, "2-3", true)
	if err != nil {
		t.Fatalf("strictly selecting an unseparated pair: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 2 {
		t.Errorf("carried ordinals = %v, want both records of the pair", got)
	}
	if !report.Strict {
		t.Error("the report does not say the strict arm ran")
	}
	// The whole session, and a window that holds every pair, are unseparated too.
	for _, spec := range []string{"0-10", "5-10", "2-3,7-8"} {
		if _, _, err := Select(conversation, spec, true); err != nil {
			t.Errorf("strictly selecting %q: err = %v, want it honoured", spec, err)
		}
	}
}

// TestSelectionReportsClassesThatReconcileWithTheRead pins D2's arithmetic: the three record classes are a
// partition of the read's own records, each necessity names the ordinal that pulled it in, and the
// conversation the writer is handed holds exactly the included records and the necessities. It runs over
// two source shapes on purpose: the Claude fixture, whose call and result live in different records (so the
// necessity machinery is exercised), and the committed OpenCode document, whose reader answers a call and
// its result events on the *same* record (so the seam's unit -- the record -- keeps a structure the
// document already holds whole).
func TestSelectionReportsClassesThatReconcileWithTheRead(t *testing.T) {
	conversation := readSelectionFixture(t)
	present := map[int]bool{}
	for _, message := range conversation.Messages {
		present[message.Ordinal] = true
	}
	for _, spec := range []string{"0", "2", "3", "5-10", "0-3,7-8", "0-10"} {
		selected, report, err := Select(conversation, spec, false)
		if err != nil {
			t.Fatalf("selecting %q: %v", spec, err)
		}
		if got := report.Included.Count + report.NecessitiesAdded.Count + report.Excluded.Count; got != len(conversation.Messages) {
			t.Errorf("selecting %q: %d + %d + %d = %d, want the read's %d record(s)", spec,
				report.Included.Count, report.NecessitiesAdded.Count, report.Excluded.Count, got,
				len(conversation.Messages))
		}
		if got := len(selected.Messages); got != report.Included.Count+report.NecessitiesAdded.Count {
			t.Errorf("selecting %q: the writer was handed %d record(s), want the %d included plus the %d "+
				"necessit%s", spec, got, report.Included.Count, report.NecessitiesAdded.Count,
				map[bool]string{true: "y", false: "ies"}[report.NecessitiesAdded.Count == 1])
		}
		carried := map[int]bool{}
		for _, message := range selected.Messages {
			carried[message.Ordinal] = true
		}
		for _, added := range report.NecessitiesAdded.Added {
			if !present[added.Ordinal] || !present[added.PulledBy] {
				t.Errorf("selecting %q: necessity %+v names an ordinal the read does not hold",
					spec, added)
			}
		}
		for _, regions := range []SelectionClass{report.Included, report.Excluded, report.CallsUndischarged} {
			for _, region := range regions.Regions {
				for _, ordinal := range regionOrdinals(t, region) {
					if !present[ordinal] {
						t.Errorf("selecting %q: the report names ordinal %d, which the read does not hold",
							spec, ordinal)
					}
				}
			}
		}
		// Every included ordinal and every necessity is carried, and nothing else is.
		included := map[int]bool{}
		for _, region := range report.Included.Regions {
			for _, ordinal := range regionOrdinals(t, region) {
				included[ordinal] = true
			}
		}
		for _, added := range report.NecessitiesAdded.Added {
			included[added.Ordinal] = true
		}
		if len(included) != len(carried) {
			t.Errorf("selecting %q: carried ordinals = %v, want the included and necessary records %v",
				spec, carried, included)
		}
		for ordinal := range carried {
			if !included[ordinal] {
				t.Errorf("selecting %q: ordinal %d was carried and no class names it", spec, ordinal)
			}
		}
		// The excluded class is the partition's other half, and its own count is its regions' count: the
		// read's own records the window did not carry are exactly the ordinals spelled there, so a report
		// that kept the count and lost the values is a failing case rather than one the presence-only loop
		// above passes.
		excluded := map[int]bool{}
		for _, region := range report.Excluded.Regions {
			for _, ordinal := range regionOrdinals(t, region) {
				excluded[ordinal] = true
			}
		}
		if len(excluded) != report.Excluded.Count {
			t.Errorf("selecting %q: the excluded class counts %d record(s) and its regions name %d",
				spec, report.Excluded.Count, len(excluded))
		}
		for ordinal := range present {
			if excluded[ordinal] == included[ordinal] {
				t.Errorf("selecting %q: ordinal %d is named by the excluded class and the carried classes "+
					"alike, or by neither: the classes are a partition of the read's own %d record(s)",
					spec, ordinal, len(conversation.Messages))
			}
		}
	}

	// The second shape: the OpenCode document's own read, whose tool-bearing record holds the call *and*
	// both of its result events (the reader's progress event and the terminal one). A selection naming that
	// record carries the whole structure without a necessity, because the document already holds it in one
	// record -- and the classes still partition the read.
	source := readOpenCodeSourceFixture(t, openCodeSourceStore(t, openCodeSourceFixtureDocument(t)))
	if len(source.Messages) < 2 {
		t.Fatalf("the OpenCode fixture read holds %d message(s), want the documented conversation",
			len(source.Messages))
	}
	toolRecord := -1
	for index, message := range source.Messages {
		if len(message.ToolCalls) > 0 || len(message.ToolResults) > 0 {
			toolRecord = index
		}
	}
	if toolRecord < 0 {
		t.Fatal("the OpenCode fixture read holds no tool evidence: the arm would check nothing")
	}
	selected, report, err := Select(source, strconv.Itoa(source.Messages[toolRecord].Ordinal), false)
	if err != nil {
		t.Fatalf("selecting the OpenCode read's tool record: %v", err)
	}
	if len(selected.Messages) != 1 {
		t.Errorf("carried %d record(s) of the OpenCode read, want the one the specification named",
			len(selected.Messages))
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities = %+v, want none: the read holds that call and its results in one record",
			report.NecessitiesAdded)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the record's call has its recorded results with it",
			report.CallsUndischarged)
	}
	if carried := len(selected.Messages[0].ToolCalls) + len(selected.Messages[0].ToolResults); carried != 3 {
		t.Errorf("the carried record holds %d piece(s) of tool evidence, want the call and its two result "+
			"events", carried)
	}
	if report.Included.Count+report.NecessitiesAdded.Count+report.Excluded.Count != len(source.Messages) {
		t.Errorf("the OpenCode read's classes account for %d of its %d records",
			report.Included.Count+report.NecessitiesAdded.Count+report.Excluded.Count, len(source.Messages))
	}
}

// regionOrdinals reads one spelled region back into the ordinal values it names, so a case can check a
// report's own spelling rather than compare strings the report happens to produce.
func regionOrdinals(t *testing.T, region string) []int {
	t.Helper()
	regions, err := parseSelectionSpec(region)
	if err != nil {
		t.Fatalf("the report spells %q, which the specification's own grammar does not read: %v", region, err)
	}
	var ordinals []int
	for _, parsed := range regions {
		if parsed.open {
			t.Fatalf("the report spells an open region %q, which names no bounded set of ordinals", region)
		}
		for value := parsed.first; value <= parsed.last; value++ {
			ordinals = append(ordinals, value)
		}
	}
	return ordinals
}

// TestSelectionWritesExactlyTheSelectedOrdinals pins D1's reproduction half over every route: the written
// document holds the evidence of exactly the selected records (and the counterparts a recorded pair
// required) and the evidence of no excluded record, so "the selection reproduced the selected ordinals"
// is read off the target's own bytes rather than off the conversation this build built.
//
// One shape is not the selection's to write: a call the source never answered has no state in OpenCode's
// own format (measured on 2.2.5 -- its importer accepts only a completed or failed tool state), so that
// route leaves the call out and counts it under its own category. The case pins both halves: the route
// does not hold the call, and its report counts the omission by name.
func TestSelectionWritesExactlyTheSelectedOrdinals(t *testing.T) {
	conversation := readSelectionFixture(t)
	type omission struct {
		evidence string
		category string
	}
	cases := []struct {
		spec   string
		held   []string
		absent []string
		// omitted is keyed by route: the evidence that route's own format cannot hold, with the loss
		// category its report counts the omission under.
		omitted map[string]omission
	}{
		{
			// The call alone: the result's own record is the necessity that travels with it, and every
			// other record's words -- the first prompt, the assistant's turns, the later question and its
			// answer -- stay out of the target.
			spec: "2",
			held: []string{selectionBuildCallID, selectionBuildResult},
			absent: []string{selectionQuestion, "Starting the build now.", "The build finished.",
				selectionReadCallID, selectionCapCallID, selectionFollowup, selectionAnswer, selectionSummary},
		},
		{
			// The undischarged call alone: it travels, and nothing else does.
			spec:   "5",
			held:   []string{selectionReadCallID},
			absent: []string{selectionBuildCallID, selectionCapCallID, selectionFollowup, selectionAnswer},
			omitted: map[string]omission{
				TargetOpenCode: {evidence: selectionReadCallID, category: "unpaired_tool_calls"},
			},
		},
		{
			// A window that holds one whole pair: both halves travel, the earlier pair stays behind.
			spec:   "7-8",
			held:   []string{selectionCapCallID, selectionCapResult},
			absent: []string{selectionBuildCallID, selectionBuildResult, selectionFollowup, selectionAnswer},
		},
	}
	for _, testCase := range cases {
		selected, _, err := Select(conversation, testCase.spec, false)
		if err != nil {
			t.Fatalf("selecting %q: %v", testCase.spec, err)
		}
		for _, target := range Targets() {
			route, err := targetRouteFor(target)
			if err != nil {
				t.Fatalf("route %s: %v", target, err)
			}
			converted, err := route.Convert(selected, selectionProbeOptions())
			if err != nil {
				t.Fatalf("%s: converting the selection %q: %v", target, testCase.spec, err)
			}
			bytes := string(converted.Data)
			omitted := testCase.omitted[target]
			for _, want := range testCase.held {
				if want == omitted.evidence {
					continue
				}
				if !strings.Contains(bytes, want) {
					t.Errorf("%s: selecting %q wrote a document without %q", target, testCase.spec, want)
				}
			}
			if omitted.evidence != "" {
				if strings.Contains(bytes, omitted.evidence) {
					t.Errorf("%s: selecting %q wrote a document holding %q, which this route's own report "+
						"says it cannot write", target, testCase.spec, omitted.evidence)
				}
				if count := lossCount(t, converted.Losses, omitted.category).Count; count == 0 {
					t.Errorf("%s: selecting %q does not count the %q omission under %q", target,
						testCase.spec, omitted.evidence, omitted.category)
				}
			}
			for _, unwanted := range testCase.absent {
				if strings.Contains(bytes, unwanted) {
					t.Errorf("%s: selecting %q wrote a document holding %q, which the selection excluded",
						target, testCase.spec, unwanted)
				}
			}
		}
	}
}

// TestSelectionKeepsASparseWindowOnTheReadsLine pins the second half of the line rule: a tree conversion
// must not make a record the read's line leaves off it an ancestor of the resumed conversation. On the
// Pi history fixture, `--select 0,3,5` keeps an off-line branch record (ordinal 3) and the on-line leaf
// (ordinal 5) while leaving out ordinal 1, the leaf's own recorded parent. The selection answers the
// nearest retained ancestor for each -- ordinal 0, whose entry both hang off -- so the tree routes write
// it and the walk from the file's last entry reaches ordinals 5 and 0 alone: ordinal 3 stays in the file
// as recorded history, off the walk, never an ancestor. A window whose records have no retained ancestor
// at all (3,5) cannot hang the leaf on the line -- the fallback would put the off-line record's entry
// under it -- and the tree routes refuse by name before anything is installed. A line route writes the
// on-line records and counts the off-line record it omits.
func TestSelectionKeepsASparseWindowOnTheReadsLine(t *testing.T) {
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piHistoryFixtureRef)
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	if len(conversation.Messages) != 6 {
		t.Fatalf("the fixture's read holds %d record(s), want the six its file carries",
			len(conversation.Messages))
	}
	textAt := func(ordinal int) string {
		t.Helper()
		for _, message := range conversation.Messages {
			if message.Ordinal == ordinal {
				return message.Content
			}
		}
		t.Fatalf("the read holds no ordinal %d", ordinal)
		return ""
	}
	leafText := textAt(5)
	abandonedTexts := []string{textAt(2), textAt(3), textAt(4)}

	// The mixed window: the branch record is selected and carried, the leaf's recorded parent is not.
	// The selection's own ancestry answer hangs both under ordinal 0 (their nearest retained ancestor),
	// so every tree route writes it -- and the walk from the file's last entry must reach the two on-line
	// records alone, with the branch record present in the file as recorded history and off that walk.
	selected, report, err := Select(conversation, "0,3,5", false)
	if err != nil {
		t.Fatalf("selecting 0,3,5: %v", err)
	}
	if report.Included.Count != 3 || report.Excluded.Count != 3 {
		t.Errorf("selecting 0,3,5: included %d, excluded %d, want the three selected records and the "+
			"three left behind", report.Included.Count, report.Excluded.Count)
	}
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting a window whose recorded ancestry the selection answers: %v",
				target, err)
		}
	}
	piRoute, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := piRoute.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the window: %v", TargetPi, err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	if len(onPath) != 2 {
		t.Errorf("the walk from the file's last entry covers %d entry(ies), want the two on-line records "+
			"(the chain is %v)", len(onPath), onPath)
	}
	var walked []string
	for id := range onPath {
		walked = append(walked, texts[id])
	}
	joined := strings.Join(walked, "\n")
	for _, want := range []string{textAt(0), leafText} {
		if !strings.Contains(joined, want) {
			t.Errorf("the walk from the file's last entry does not hold %q; it holds:\n%s", want, joined)
		}
	}
	for _, unwanted := range abandonedTexts {
		if strings.Contains(joined, unwanted) {
			t.Errorf("the walk from the file's last entry holds %q, a record the read's line leaves off "+
				"it (the chain is %v)", unwanted, onPath)
		}
	}
	if !strings.Contains(string(session.Data), textAt(3)) {
		t.Error("the written document does not hold the selected branch record at all: it is recorded " +
			"history the selection carried, and the rule is about the walk, not the file")
	}

	// A window with no retained ancestor at all: nothing in it can hang the leaf on the read's line, and
	// the fallback would put the off-line record's entry under it. Every tree route refuses by name,
	// before anything is installed; the line route writes the on-line record and counts the one it omits.
	selected, _, err = Select(conversation, "3,5", false)
	if err != nil {
		t.Fatalf("selecting 3,5: %v", err)
	}
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		_, err = route.Convert(selected, selectionProbeOptions())
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Errorf("%s: converting a window whose records have no retained ancestor: err = %v, want "+
				"ErrUnrepresentableLeaf", target, err)
			continue
		}
		for _, want := range []string{"5566cc77", "0011aa22"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: the refusal %v does not name %s", target, err, want)
			}
		}
	}
	route, err := targetRouteFor(TargetCodex)
	if err != nil {
		t.Fatalf("route %s: %v", TargetCodex, err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the selection: %v", TargetCodex, err)
	}
	if count := lossCount(t, converted.Losses, "inactive_branch_records").Count; count != 1 {
		t.Errorf("%s: inactive_branch_records = %d, want the one off-line record the window names",
			TargetCodex, count)
	}
	bytes := string(converted.Data)
	if !strings.Contains(bytes, leafText) {
		t.Errorf("%s: the written document does not hold the record on the read's line", TargetCodex)
	}
	for _, unwanted := range abandonedTexts {
		if strings.Contains(bytes, unwanted) {
			t.Errorf("%s: the written document holds %q, a record the read's line leaves off it",
				TargetCodex, unwanted)
		}
	}

	// The smallest sparse window of the same shape -- ordinal 0 and the leaf alone -- converts too.
	selected, _, err = Select(conversation, "0,5", false)
	if err != nil {
		t.Fatalf("selecting 0,5: %v", err)
	}
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting the sparse window 0,5: %v", target, err)
		}
	}
}

// TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall pins the projection half of the ancestry rule
// on the committed Pi fixture: the reader answers the record after a tool result as hanging off the
// message that made the call (record 3 names the call in the read although the file names the result),
// while the source's own file runs call → result → what follows. A window that carries the call and the
// record after the result adds the result as a necessity, and the *result* is the ancestor the reduced
// tree must chain the record through: an ancestry answer resolved over raw relations alone reaches the
// call, chains the record's entry straight to it, and leaves the result in the file and off the walk the
// target's loader makes although the report names it as carried. Measured on the written Pi session: the
// walk from its last entry covers the call's entry, the result's entry and the record that follows.
func TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall(t *testing.T) {
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piFixtureRef)
	if err != nil {
		t.Fatalf("reading the Pi fixture: %v", err)
	}
	if len(conversation.Messages) != 5 {
		t.Fatalf("the fixture's read holds %d record(s), want the five its file carries",
			len(conversation.Messages))
	}
	// The fixture's own words, restated rather than read out of the read: the call record at ordinal 1,
	// its result at 2, the record the reader answers as naming the call at 3, and the record naming 3 at 4.
	const (
		callText   = "I will look for the retry configuration."
		resultText = "internal/retry/config.go:41: maxBackoffSeconds = 30"
		answerText = "The cap is 30 seconds of backoff between attempts."
	)
	selected, report, err := Select(conversation, "1,4", false)
	if err != nil {
		t.Fatalf("selecting 1,4: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Errorf("carried ordinals = %v, want the window 1,4 with the result at 2 as its necessity", got)
	}
	if report.NecessitiesAdded.Count != 1 ||
		report.NecessitiesAdded.Added[0] != (SelectionNecessity{Ordinal: 2, PulledBy: 1}) {
		t.Errorf("necessities = %+v, want the result at ordinal 2 pulled in by the call at ordinal 1",
			report.NecessitiesAdded)
	}
	route, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	walked := map[string]bool{}
	var walkTexts []string
	for id := range onPath {
		walked[texts[id]] = true
		walkTexts = append(walkTexts, texts[id])
	}
	for _, want := range []string{callText, resultText, answerText} {
		if !walked[want] {
			t.Errorf("the walk from the file's last entry does not hold %q, which the report names as "+
				"carried (the walk holds %q)", want, walkTexts)
		}
	}
	// The chain the writer hung there crossed the read's own tool-result projection: the record names the
	// call while a result of that call stands between them, so which of the two the source placed the
	// record under is not answerable from the read, and the relation is counted. Two places are counted on
	// this window -- the record's excluded ancestor (ordinal 0, the read's on-line root, which the window
	// 1,4 leaves out) and the projection -- and the count has to name both, because a window that lost
	// the projection's own place would present a relation this conversion resolved as the read's own.
	unresolved := lossCount(t, session.Losses, "unresolved_source_parents")
	if unresolved.Count != 2 || unresolved.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("unresolved_source_parents = %+v, want the two places the window's walk could not answer: "+
			"the excluded root and the read's tool-result projection", unresolved)
	}
	if !strings.Contains(unresolved.Detail, "visible ancestor") {
		t.Errorf("the detail does not name the projection the count stands for: %q", unresolved.Detail)
	}
	// The window that also carries the root isolates the projection: every other relation resolves to a
	// record the window holds, so the one count is the chain that crossed the tool-result step -- exactly
	// the count the unflagged conversion of the whole read reports for the same place, which is what makes
	// the window's number the read's own answer rather than one the reduction invented.
	rooted, _, err := Select(conversation, "0,1,4", false)
	if err != nil {
		t.Fatalf("selecting 0,1,4: %v", err)
	}
	rootedSession, err := route.Convert(rooted, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the 0,1,4 window: %v", TargetPi, err)
	}
	if got := lossCount(t, rootedSession.Losses, "unresolved_source_parents"); got.Count != 1 {
		t.Errorf("the 0,1,4 window counts unresolved_source_parents = %+v, want the one relation the "+
			"read's projection left unanswerable", got)
	}
	whole, err := route.Convert(conversation, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the whole read: %v", err)
	}
	if got := lossCount(t, whole.Losses, "unresolved_source_parents"); got.Count != 1 {
		t.Errorf("the unflagged conversion counts unresolved_source_parents = %+v, want the one place "+
			"the read's projection leaves open (the 0,1,4 window reports the same place)", got)
	}
}

// TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor pins the ancestry answer for a record
// the read answered no identity for. The inherited Claude reader preserves `parentUuid` and `uuid`
// independently (internal/parser/claude.go), so a record can name its parent while carrying no identity of
// its own: the read's shape here is a root (0), an abandoned child (1), the active child (2) and a final
// record (3) whose recorded parent is the active child. A window that carries the root and the abandoned
// child while leaving the active child out (`0,1,3`) must chain record 3 to its nearest retained ancestor,
// the root's entry -- dropping the answer because record 3 has no identity hangs it off the entry written
// before it, the abandoned child's, and revives that branch as an ancestor of the context a resumed
// session reads. A window with no retained ancestor at all (`1,3`) is refused by name instead of taking
// that fallback: the check asks by the record's own position, so the identity it never had cannot skip it.
func TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 5, 5, 10, 0, second, 0, time.UTC) }
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", DialogueProjected: true,
			SourceUUID: "u1", Timestamp: at(0)},
		Message{Ordinal: 1, Role: "assistant", Content: "the left turn", Dialogue: "the left turn",
			DialogueProjected: true, SourceUUID: "b2", SourceParentUUID: "u1", Timestamp: at(1)},
		Message{Ordinal: 2, Role: "assistant", Content: "the active line", Dialogue: "the active line",
			DialogueProjected: true, SourceUUID: "c3", SourceParentUUID: "u1", Timestamp: at(2)},
		// The record the read answered no identity for: its own recorded parent is the active child, and
		// the read holds no uuid of its own (the inherited Claude reader's two independent fields).
		Message{Ordinal: 3, Role: "user", Content: "the last word", Dialogue: "the last word",
			DialogueProjected: true, SourceUUID: "", SourceParentUUID: "c3", Timestamp: at(3)},
	)
	selected, _, err := Select(conversation, "0,1,3", false)
	if err != nil {
		t.Fatalf("selecting 0,1,3: %v", err)
	}
	transcript, err := ConvertToClaudeTranscript(selected, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	records := decodeTranscript(t, transcript)
	uuidOf := func(text string) string {
		t.Helper()
		for _, record := range records {
			for _, block := range recordBlocks(record) {
				if block["type"] == "text" && block["text"] == text {
					id, _ := record["uuid"].(string)
					return id
				}
			}
		}
		t.Fatalf("no record carries the text %q:\n%s", text, transcript.Data)
		return ""
	}
	parentOf := func(text string) any {
		t.Helper()
		id := uuidOf(text)
		for _, record := range records {
			if record["uuid"] == id {
				return record["parentUuid"]
			}
		}
		t.Fatalf("no record carries the uuid %q", id)
		return nil
	}
	if got := parentOf("the last word"); got != uuidOf("start") {
		t.Errorf("the record the read answered no identity for hangs off %v, want its nearest retained "+
			"ancestor's entry (%q): the identity it never had must not drop the answer and chain it to "+
			"the entry written before it, which stands for the abandoned branch", got, uuidOf("start"))
	}

	// The same window through the other tree writer: the walk from the written session's last entry
	// reaches the identity-less record and its retained ancestor, and not the abandoned branch.
	route, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the window: %v", TargetPi, err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	walked := map[string]bool{}
	for id := range onPath {
		walked[texts[id]] = true
	}
	for _, want := range []string{"start", "the last word"} {
		if !walked[want] {
			t.Errorf("%s: the walk from the file's last entry does not hold %q", TargetPi, want)
		}
	}
	if walked["the left turn"] {
		t.Errorf("%s: the walk from the file's last entry holds the abandoned branch's record: the "+
			"identity-less record was chained to the entry written before it instead of to its nearest "+
			"retained ancestor", TargetPi)
	}

	// A window that retains no ancestor: the fallback would put the identity-less record under the
	// abandoned branch's entry, and the guard refuses by name -- the check asks by the record's position,
	// so an empty identity cannot skip it.
	noAncestor, _, err := Select(conversation, "1,3", false)
	if err != nil {
		t.Fatalf("selecting 1,3: %v", err)
	}
	_, err = ConvertToClaudeTranscript(noAncestor, ClaudeTargetOptions{})
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Errorf("converting a window with no retained ancestor: err = %v, want ErrUnrepresentableLeaf", err)
	} else {
		for _, want := range []string{"the record at ordinal 3", "the read answered no identity for it", "b2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %v does not name %s", err, want)
			}
		}
	}

	// The parentless variant of the same shape: the final record carries no identity *and* names no
	// parent, so `parentPlace` has nothing to resolve and the read's own order is its place. On this
	// window that place is the entry written before it -- the abandoned child's -- and the fallback's
	// revival check has to refuse it all the same: an identity-less record's place is the read's own
	// order, but it must not put an off-line record on the walk a resumed session makes (`--select 0,1,3`
	// would write the tree 3 → 1 → 0 and revive ordinal 1, which the full read's own walk 3 → 2 → 0
	// leaves off it).
	parentless := conversation
	parentless.Messages = append([]Message(nil), conversation.Messages...)
	parentless.Messages[3].SourceParentUUID = ""
	selected, _, err = Select(parentless, "0,1,3", false)
	if err != nil {
		t.Fatalf("selecting 0,1,3 over the parentless record: %v", err)
	}
	_, err = ConvertToClaudeTranscript(selected, ClaudeTargetOptions{})
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Errorf("converting a window whose identity-less last record has no parent: err = %v, want "+
			"ErrUnrepresentableLeaf: the fallback would chain it to the entry of the abandoned child", err)
	} else {
		for _, want := range []string{"the record at ordinal 3", "the read answered no identity for it", "b2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %v does not name %s", err, want)
			}
		}
	}
	// The control: the same parentless record with the abandoned child left out of the window hangs off
	// the root's entry -- the entry written before it is on the read's own line -- and the walk from the
	// file's last entry holds the root and the record alone.
	selected, _, err = Select(parentless, "0,3", false)
	if err != nil {
		t.Fatalf("selecting 0,3 over the parentless record: %v", err)
	}
	transcript, err = ConvertToClaudeTranscript(selected, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the 0,3 window: %v", err)
	}
	records = decodeTranscript(t, transcript)
	if got := parentOf("the last word"); got != uuidOf("start") {
		t.Errorf("the parentless, identity-less record hangs off %v, want the entry of the record "+
			"before it on the read's own line (%q)", got, uuidOf("start"))
	}
}

// TestSelectionKeepsAnInactiveBranchesRecordedAncestry pins the ancestry rule's inactive half: a record
// the read's own line leaves off the walk may keep an ancestor that is itself off that line. The read is
// the shape where the two meet -- the branch 0 → 1 → 2 → 4 beside the line 0 → 3 → 5 -- and the window
// 0-1,3-5 leaves ordinal 2 out, so ordinal 4's nearest retained ancestor is ordinal 1. Both are off the
// read's line, keeping a branch off the active walk is recorded history the writer reproduces, and the
// entry has to hang where the source recorded it: rewriting ordinal 4 under the entry written before it
// (ordinal 3, a record of the other branch) would change a relation the source did record. The guard's
// other half still refuses the shape it exists for: a window whose *on-line* record would land under an
// off-line entry (1,5 here) is refused by name.
func TestSelectionKeepsAnInactiveBranchesRecordedAncestry(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the branch parent", Dialogue: "the branch parent",
			SourceUUID: "b2", SourceParentUUID: "u1"},
		Message{Ordinal: 2, Role: "assistant", Content: "the branch child", Dialogue: "the branch child",
			SourceUUID: "c3", SourceParentUUID: "b2"},
		Message{Ordinal: 3, Role: "assistant", Content: "the line child", Dialogue: "the line child",
			SourceUUID: "d4", SourceParentUUID: "u1"},
		Message{Ordinal: 4, Role: "assistant", Content: "the branch grandchild",
			Dialogue: "the branch grandchild", SourceUUID: "e5", SourceParentUUID: "c3"},
		Message{Ordinal: 5, Role: "assistant", Content: "the last word", Dialogue: "the last word",
			SourceUUID: "f6", SourceParentUUID: "d4"},
	)
	selected, _, err := Select(conversation, "0-1,3-5", false)
	if err != nil {
		t.Fatalf("selecting 0-1,3-5: %v", err)
	}
	route, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the window: %v", TargetPi, err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	entryOf := func(text string) piLine {
		t.Helper()
		for _, line := range lines {
			if texts[line.ID] == text {
				return line
			}
		}
		t.Fatalf("no entry carries the text %q", text)
		return piLine{}
	}
	parentTextOf := func(text string) string {
		t.Helper()
		line := entryOf(text)
		if line.ParentID == nil {
			return ""
		}
		return texts[*line.ParentID]
	}
	if got := parentTextOf("the branch grandchild"); got != "the branch parent" {
		t.Errorf("the off-line grandchild hangs off %q, want its recorded ancestor \"the branch parent\": "+
			"the entry written before it stands for a record of the other branch, and rewriting the "+
			"branch's own relation would change history the source did record", got)
	}
	// The walk from the file's last entry is the read's own line: the branch's three records are in the
	// file as recorded history and off that walk.
	onPath := piWalkIDs(t, lines)
	walked := map[string]bool{}
	for id := range onPath {
		walked[texts[id]] = true
	}
	for _, want := range []string{"start", "the line child", "the last word"} {
		if !walked[want] {
			t.Errorf("the walk from the file's last entry does not hold %q", want)
		}
	}
	for _, unwanted := range []string{"the branch parent", "the branch grandchild"} {
		if walked[unwanted] {
			t.Errorf("the walk from the file's last entry holds %q, a record the read's line leaves off it",
				unwanted)
		}
		if !strings.Contains(string(session.Data), unwanted) {
			t.Errorf("the written document does not hold %q at all: it is recorded history the selection "+
				"carried, and the rule is about the walk, not the file", unwanted)
		}
	}
	if strings.Contains(string(session.Data), "the branch child") {
		t.Error("the written document holds the excluded record's words: the window 0-1,3-5 does not " +
			"carry ordinal 2")
	}

	// The guard's other half: a window in which the *on-line* record's own ancestry is gone entirely, so
	// the fallback would chain it to the entry of an off-line record, is refused by name.
	noAncestor, _, err := Select(conversation, "1,5", false)
	if err != nil {
		t.Fatalf("selecting 1,5: %v", err)
	}
	_, err = route.Convert(noAncestor, selectionProbeOptions())
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Errorf("converting a window whose on-line record would hang under an off-line entry: err = %v, "+
			"want ErrUnrepresentableLeaf", err)
	} else {
		for _, want := range []string{"f6 (ordinal 5)", "b2 (ordinal 1)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal %v does not name %s", err, want)
			}
		}
	}
}

// TestSelectionPairsAReusedToolIDByRecordedOrder pins the seam against an id the source reused: the
// inherited readers' own regression (internal/parser/termination_test.go's "earlier matching result does
// not resolve final orphan") records a call at ordinal 0 answered by a result at ordinal 1, then a later
// call at ordinal 2 reusing the id with no result. Selecting the later call carries it alone -- expanding
// by id alone would import the earlier pair as if it were this call's counterpart -- and the report
// counts the later call undischarged. The strict arm is honoured for the later call and refuses either
// half of the older pair by name.
func TestSelectionPairsAReusedToolIDByRecordedOrder(t *testing.T) {
	reused := Conversation{Messages: []Message{
		{Ordinal: 0, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
		{Ordinal: 1, Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_reused", ContentRaw: "one"}}},
		{Ordinal: 2, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
	}}

	selected, report, err := Select(reused, "2", false)
	if err != nil {
		t.Fatalf("selecting the later call: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 2 {
		t.Errorf("carried ordinals = %v, want the later call alone: the earlier pair is not its counterpart",
			got)
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities = %+v, want none: the later call has no recorded result of its own",
			report.NecessitiesAdded)
	}
	if report.CallsUndischarged.Count != 1 || strings.Join(report.CallsUndischarged.Regions, ",") != "2" {
		t.Errorf("calls_undischarged = %+v, want the later call counted as the source left it",
			report.CallsUndischarged)
	}

	// The older pair still pairs in both directions.
	for _, testCase := range []struct {
		spec      string
		necessity SelectionNecessity
	}{
		{"0", SelectionNecessity{Ordinal: 1, PulledBy: 0}},
		{"1", SelectionNecessity{Ordinal: 0, PulledBy: 1}},
	} {
		selected, report, err := Select(reused, testCase.spec, false)
		if err != nil {
			t.Fatalf("selecting %q: %v", testCase.spec, err)
		}
		if got := selectionOrdinals(selected); len(got) != 2 || got[0] != 0 || got[1] != 1 {
			t.Errorf("selecting %q: carried ordinals = %v, want the older pair 0,1", testCase.spec, got)
		}
		if report.NecessitiesAdded.Count != 1 || report.NecessitiesAdded.Added[0] != testCase.necessity {
			t.Errorf("selecting %q: necessities = %+v, want %+v", testCase.spec,
				report.NecessitiesAdded, testCase.necessity)
		}
	}

	// Strict: the later call separates nothing, either half of the older pair is refused by name.
	if _, _, err := Select(reused, "2", true); err != nil {
		t.Errorf("strictly selecting the later call: err = %v, want it honoured (no pair is separated)", err)
	}
	for _, spec := range []string{"0", "1"} {
		if _, _, err := Select(reused, spec, true); !errors.Is(err, ErrStrictSelection) {
			t.Errorf("strictly selecting %q: err = %v, want ErrStrictSelection", spec, err)
		} else if !strings.Contains(err.Error(), "toolu_reused") {
			t.Errorf("strictly selecting %q: %v does not name the reused id", spec, err)
		}
	}

	// Two calls, one later result: the result answers the nearest preceding call, so the earlier call is
	// not answered by it and is not pulled into a selection of the result.
	twoCalls := Conversation{Messages: []Message{
		{Ordinal: 0, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
		{Ordinal: 1, Role: "assistant", Content: "between"},
		{Ordinal: 2, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
		{Ordinal: 3, Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_reused", ContentRaw: "two"}}},
	}}
	selected, report, err = Select(twoCalls, "0", false)
	if err != nil {
		t.Fatalf("selecting the earlier call: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 0 {
		t.Errorf("carried ordinals = %v, want the earlier call alone: the result answers the later call",
			got)
	}
	if report.CallsUndischarged.Count != 1 || strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the earlier call counted", report.CallsUndischarged)
	}
	selected, report, err = Select(twoCalls, "3", false)
	if err != nil {
		t.Fatalf("selecting the result: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("carried ordinals = %v, want the later call and its result", got)
	}
	if report.NecessitiesAdded.Count != 1 ||
		report.NecessitiesAdded.Added[0] != (SelectionNecessity{Ordinal: 2, PulledBy: 3}) {
		t.Errorf("necessities = %+v, want the later call pulled in by the result", report.NecessitiesAdded)
	}
}

// TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers pins calls_undischarged to the
// same call identity the seam's pairing and the writers use, in the two shapes the accounting used to
// disagree on:
//
//   - the committed Codex `apply_patch` fixture, whose reader projects one invocation into two call rows
//     sharing the id on one message, with one recorded result. The rows are one call (distinctCalls, the
//     rule every writer coalesces by: only the first row is written) and the result answers it, so
//     nothing is undischarged and the written document holds one answered call;
//   - two calls reusing an id recorded *before* two outcomes. The seam pairs both outcomes with the
//     nearest call occurrence the id records before them -- the later call -- so the earlier call is the
//     one the source recorded no result for; the count is the pairing's own answer, not a pop that lets
//     the second outcome discharge the earlier call, and the closure agrees (a window naming the earlier
//     call carries it alone).
func TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers(t *testing.T) {
	patch := readFixtureConversation(t, codexPatchFixtureRef)
	rows := 0
	for _, message := range patch.Messages {
		rows += len(message.ToolCalls)
	}
	if rows != 2 {
		t.Fatalf("the fixture's read holds %d call row(s), want the two the reader projects one "+
			"invocation into", rows)
	}
	selected, report, err := Select(patch, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the two rows are one call (the writer's own "+
			"distinctCalls unit) and the recorded result answers it", report.CallsUndischarged)
	}
	transcript, err := ConvertToClaudeTranscript(selected, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting the whole read: %v", err)
	}
	if loss := lossCount(t, transcript.Losses, "coalesced_tool_call_projections"); loss.Count != 1 {
		t.Errorf("coalesced_tool_call_projections = %+v, want the one projected row the writer coalesces "+
			"into the call it writes", loss)
	}
	if transcript.Carried.ToolCalls != 1 || transcript.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the one coalesced call with the result that answers it",
			transcript.Carried)
	}

	// The two-calls-two-outcomes shape: both calls are recorded before both results.
	reused := Conversation{Messages: []Message{
		{Ordinal: 0, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
		{Ordinal: 1, Role: "assistant", ToolCalls: []ToolCall{{ToolUseID: "toolu_reused", Name: "Bash"}}},
		{Ordinal: 2, Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_reused", ContentRaw: "one"}}},
		{Ordinal: 3, Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_reused", ContentRaw: "two"}}},
	}}
	selected, report, err = Select(reused, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 4 {
		t.Errorf("carried ordinals = %v, want the whole read", got)
	}
	if report.CallsUndischarged.Count != 1 || strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the earlier call at ordinal 0: the seam pairs both "+
			"outcomes with the nearest call the id records before them (ordinal 1), so ordinal 0 is the "+
			"call the source recorded no result for", report.CallsUndischarged)
	}
	// The closure reads the same pairing: naming the earlier call carries it alone, because no pair
	// touches it.
	selected, report, err = Select(reused, "0", false)
	if err != nil {
		t.Fatalf("selecting the earlier call: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 0 {
		t.Errorf("carried ordinals = %v, want the earlier call alone", got)
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities = %+v, want none: no recorded result pairs with the earlier call",
			report.NecessitiesAdded)
	}
}

// TestSelectionCountsACallWithOnlyProgressEventsUndischarged pins the source side of calls_undischarged
// against the inherited readers' own shape: a call whose recorded result event still says it is running
// (the Codex subagent notification internal/parser/codex_parser_test.go's "running-only wait output is
// preserved as a result event" case records) has not been answered, and the count must not read the
// progress as an outcome. The same event with an outcome status answers it, the event itself still
// travels with its call, and the status words the readers write for a streamed chunk are progress too.
func TestSelectionCountsACallWithOnlyProgressEventsUndischarged(t *testing.T) {
	withStatus := func(status string) Conversation {
		return Conversation{Messages: []Message{{
			Ordinal:   0,
			Role:      "assistant",
			ToolCalls: []ToolCall{{ToolUseID: "call_wait", Name: "wait"}},
			ToolResults: []ToolResult{{
				ToolUseID: "call_wait", Status: status, ContentRaw: "Still working",
			}},
		}}}
	}
	for _, status := range []string{"running", "pending", "streaming", "started", "in_progress"} {
		selected, report, err := Select(withStatus(status), "0", false)
		if err != nil {
			t.Fatalf("selecting a call whose only result is %q: %v", status, err)
		}
		if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 0 {
			t.Errorf("the %q event: carried ordinals = %v, want the record with its evidence",
				status, got)
		}
		if len(selected.Messages[0].ToolCalls) != 1 || len(selected.Messages[0].ToolResults) != 1 {
			t.Errorf("the %q event: the carried record lost its call or its recorded event: %+v",
				status, selected.Messages[0])
		}
		if report.CallsUndischarged.Count != 1 ||
			strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
			t.Errorf("the %q event: calls_undischarged = %+v, want the call counted: progress is not an "+
				"outcome", status, report.CallsUndischarged)
		}
	}
	for _, status := range []string{"", "completed", "errored", "failed"} {
		_, report, err := Select(withStatus(status), "0", false)
		if err != nil {
			t.Fatalf("selecting a call whose only result is %q: %v", status, err)
		}
		if report.CallsUndischarged.Count != 0 {
			t.Errorf("the %q event: calls_undischarged = %+v, want none: the read holds an outcome",
				status, report.CallsUndischarged)
		}
	}
}

// claudeWalkRecords answers the records one written transcript's walk covers: the path from the file's
// last record back through parentUuid, which is what the target's own loader follows. A case uses it to
// name the records a resumed conversation would see -- and the ones it would not.
func claudeWalkRecords(t *testing.T, records []map[string]any) []map[string]any {
	t.Helper()
	byUUID := map[string]map[string]any{}
	for _, record := range records {
		if id, _ := record["uuid"].(string); id != "" {
			byUUID[id] = record
		}
	}
	var walked []map[string]any
	seen := map[string]bool{}
	if len(records) == 0 {
		return walked
	}
	cursor, _ := records[len(records)-1]["uuid"].(string)
	for cursor != "" && !seen[cursor] {
		seen[cursor] = true
		record, ok := byUUID[cursor]
		if !ok {
			break
		}
		walked = append(walked, record)
		parent, _ := record["parentUuid"].(string)
		cursor = parent
	}
	return walked
}

// claudeRecordHoldsInput answers whether one written record holds a tool_use block with the id and the
// recorded command, and claudeRecordHoldsResult whether it holds a tool_result block with the text: the
// two halves a case walks a written tree for.
func claudeRecordHoldsInput(record map[string]any, callID, command string) bool {
	for _, block := range recordBlocks(record) {
		if block["type"] != "tool_use" || block["id"] != callID {
			continue
		}
		if input, ok := block["input"].(map[string]any); ok && input["command"] == command {
			return true
		}
	}
	return false
}

func claudeRecordHoldsResult(record map[string]any, text string) bool {
	for _, block := range recordBlocks(record) {
		if block["type"] == "tool_result" && block["content"] == text {
			return true
		}
	}
	return false
}

// TestSelectionPairsAResultByItsRecordedAncestry pins the pairing's ancestry arm and the tree writers'
// own check of it: the result at ordinal 5 names the call at ordinal 3 as its recorded parent, while an
// inactive sibling at ordinal 4 reuses the id later in file order. The read's own answer decides -- the
// window 1,5 carries the call the source placed the result under, not the later occurrence -- the strict
// arm is honoured for the pair, selecting the sibling alone counts its own call undischarged, and every
// route writes the result under the call it answers: on the trees, both halves stand on the walk the
// target's loader makes from the result. The same tree without a selection keeps the call's output too:
// the sibling is off the read's line and the result still answers the call the source recorded.
func TestSelectionPairsAResultByItsRecordedAncestry(t *testing.T) {
	const (
		reusedID    = "toolu_reused_across_occurrences"
		resultAText = "the first call's own output"
		resultBText = "the later call's own output"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: `{"command":"one"}`}}},
		Message{Ordinal: 2, Role: "user", SourceUUID: "r3", SourceParentUUID: "a2",
			ToolResults: []ToolResult{{ToolUseID: reusedID, ContentRaw: resultAText, ContentSize: len(resultAText)}}},
		Message{Ordinal: 3, Role: "assistant", SourceUUID: "b4", SourceParentUUID: "r3",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: `{"command":"two"}`}}},
		// The inactive sibling: the read's own line is the walk from the last record (5 → b4 → r3 → a2 →
		// u1), so this record -- whose parent is r3 like B's -- never stands on it. It reuses the id, and
		// it is the occurrence the id's file order would credit the last result to.
		Message{Ordinal: 4, Role: "assistant", SourceUUID: "c5", SourceParentUUID: "r3",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: `{"command":"three"}`}}},
		Message{Ordinal: 5, Role: "user", SourceUUID: "e6", SourceParentUUID: "b4",
			ToolResults: []ToolResult{{ToolUseID: reusedID, ContentRaw: resultBText, ContentSize: len(resultBText)}}},
	)

	// The result pulls in the call its recorded parent names, not the later sibling occurrence.
	selected, report, err := Select(conversation, "5", false)
	if err != nil {
		t.Fatalf("selecting 5: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Errorf("carried ordinals = %v, want 3,5: the result names the call at ordinal 3 as its parent",
			got)
	}
	if report.NecessitiesAdded.Count != 1 ||
		report.NecessitiesAdded.Added[0] != (SelectionNecessity{Ordinal: 3, PulledBy: 5}) {
		t.Errorf("necessities = %+v, want the call at ordinal 3 pulled in by the result",
			report.NecessitiesAdded)
	}

	// The window: A and B's result are what the specification named, A's result and B are the
	// counterparts the seam's pairing pulls in, and the inactive sibling stays behind.
	selected, report, err = Select(conversation, "1,5", false)
	if err != nil {
		t.Fatalf("selecting 1,5: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 5 {
		t.Fatalf("carried ordinals = %v, want the window's 1,5 with the counterparts 2 (A's result) and "+
			"3 (the call the last result answers)", got)
	}
	wantNecessities := []SelectionNecessity{{Ordinal: 2, PulledBy: 1}, {Ordinal: 3, PulledBy: 5}}
	if report.NecessitiesAdded.Count != 2 ||
		!reflect.DeepEqual(report.NecessitiesAdded.Added, wantNecessities) {
		t.Errorf("necessities = %+v, want %+v", report.NecessitiesAdded, wantNecessities)
	}
	if report.Excluded.Count != 2 || strings.Join(report.Excluded.Regions, ",") != "0,4" {
		t.Errorf("excluded = %+v, want the start record and the inactive sibling", report.Excluded)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: both results answer the calls the source recorded",
			report.CallsUndischarged)
	}

	// The strict arm: the pair the source recorded together is not a separation, and either the sibling
	// or the result alone still is.
	if _, _, err := Select(conversation, "3,5", true); err != nil {
		t.Errorf("strictly selecting 3,5: err = %v, want it honoured: the pair is whole", err)
	}
	if _, _, err := Select(conversation, "5", true); !errors.Is(err, ErrStrictSelection) {
		t.Errorf("strictly selecting 5: err = %v, want ErrStrictSelection for the pair 3,5", err)
	} else if !strings.Contains(err.Error(), reusedID) {
		t.Errorf("strictly selecting 5: %v does not name the reused id", err)
	}
	// The sibling's own call has no recorded result: the reused id does not discharge it.
	selected, report, err = Select(conversation, "4", false)
	if err != nil {
		t.Fatalf("selecting the sibling: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 1 || got[0] != 4 {
		t.Errorf("carried ordinals = %v, want the sibling alone", got)
	}
	if report.CallsUndischarged.Count != 1 || strings.Join(report.CallsUndischarged.Regions, ",") != "4" {
		t.Errorf("calls_undischarged = %+v, want the sibling's own call counted: the result at ordinal 5 "+
			"answers the call its recorded parent names", report.CallsUndischarged)
	}

	// The flat routes: the read's own pairing is what the occurrence checks are asked, so both outputs of
	// the reused id travel and the excluded sibling's input does not. The sibling's own record is off the
	// read's line, so the flat writers carry nothing of it and count the branch; its own call -- which no
	// recorded result answers -- is the count the report names.
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the sibling: %v", err)
	}
	if rollout.Carried.ToolCalls != 0 || rollout.Carried.ToolResults != 0 {
		t.Errorf("converting the sibling alone: carried = %+v, want nothing: the record is off the read's "+
			"line", rollout.Carried)
	}
	if count := lossCount(t, rollout.Losses, "inactive_branch_records").Count; count != 1 {
		t.Errorf("converting the sibling alone: inactive_branch_records = %d, want the off-line record "+
			"counted", count)
	}
	selectedWindow, _, err := Select(conversation, "1,5", false)
	if err != nil {
		t.Fatalf("selecting 1,5: %v", err)
	}
	for _, target := range []string{TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selectedWindow, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if converted.Carried.ToolCalls != 2 || converted.Carried.ToolResults != 2 {
			t.Errorf("%s: carried = %+v, want both calls and both results of the window",
				target, converted.Carried)
		}
		data := string(converted.Data)
		for _, want := range []string{resultAText, resultBText} {
			if !strings.Contains(data, want) {
				t.Errorf("%s: the written document does not hold %q", target, want)
			}
		}
		if strings.Contains(data, `"command":"three"`) {
			t.Errorf("%s: the written document holds the excluded sibling's input", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: each result's paired call is in the window",
				target, count)
		}
	}

	// The same document without a selection: the sibling is off the read's line and is dropped, and the
	// last result still answers the call the source recorded -- the base conversion's own behaviour.
	unflagged, err := ConvertToCodexRollout(conversation, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the whole read: %v", err)
	}
	if unflagged.Carried.ToolCalls != 2 || unflagged.Carried.ToolResults != 2 {
		t.Errorf("the unflagged codex conversion carries %+v, want both calls and both results",
			unflagged.Carried)
	}
	if !strings.Contains(string(unflagged.Data), resultBText) {
		t.Errorf("the unflagged rollout does not hold the last result's own output:\n%s", unflagged.Data)
	}
	if count := lossCount(t, unflagged.Losses, "unpaired_tool_results").Count; count != 0 {
		t.Errorf("unflagged codex unpaired_tool_results = %d, want 0: the result answers the call on the "+
			"read's line, not the sibling the line leaves off it", count)
	}
	if count := lossCount(t, unflagged.Losses, "inactive_branch_records").Count; count != 1 {
		t.Errorf("unflagged codex inactive_branch_records = %d, want the one off-line record",
			count)
	}

	// The tree routes: the result is written under the call the source recorded, and both stand on the
	// walk the target's loader makes from the last record; the dropped sibling is nowhere in the file.
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selectedWindow, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		data := string(converted.Data)
		if !strings.Contains(data, resultBText) {
			t.Errorf("%s: the written document does not hold the last result's output", target)
		}
		if strings.Contains(data, `"command":"three"`) {
			t.Errorf("%s: the written document holds the excluded sibling's input", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: the result's paired call is written on its "+
				"own ancestry", target, count)
		}
	}
	transcript, err := ConvertToClaudeTranscript(selectedWindow, ClaudeTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the window to claude: %v", err)
	}
	callOnWalk, resultOnWalk := false, false
	for _, record := range claudeWalkRecords(t, decodeTranscript(t, transcript)) {
		if claudeRecordHoldsInput(record, reusedID, "two") {
			callOnWalk = true
		}
		if claudeRecordHoldsResult(record, resultBText) {
			resultOnWalk = true
		}
	}
	if !callOnWalk || !resultOnWalk {
		t.Errorf("the walk from the transcript's last record holds the call = %v and the result = %v, "+
			"want both: a resumed session reads the output only under the invocation it answers",
			callOnWalk, resultOnWalk)
	}
	piRoute, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := piRoute.Convert(selectedWindow, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the window to pi: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	texts := piEntryTexts(t, lines)
	onPath := piWalkIDs(t, lines)
	resultID := ""
	for id, text := range texts {
		if text == resultBText {
			resultID = id
		}
	}
	if resultID == "" {
		t.Fatalf("the written Pi session holds no entry with the last result's text:\n%s", session.Data)
	}
	if !onPath[resultID] {
		t.Errorf("the walk from the Pi file's last entry does not hold the last result's entry")
	}
	callOnWalk = false
	for _, line := range lines[1:] {
		if onPath[line.ID] && strings.Contains(string(line.Message), `{"command":"two"}`) {
			callOnWalk = true
		}
	}
	if !callOnWalk {
		t.Errorf("the walk from the Pi file's last entry holds no record of the call the result answers:\n%s",
			session.Data)
	}
}

// TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry pins the seam's pairing travelling
// with the reduced document. The read: a call C at ordinal 1 answered (its recorded parent chain runs
// through the excluded record at 3) by the result at 4, and an inactive sibling S at ordinal 2 reusing
// the id. Selecting S and the result carries C, S and the result -- and the reduced document no longer
// holds the records the result's ancestry walk steps through, so a writer re-reading the pairing over
// the window would fall back to the nearest carried occurrence, S, and drop the result the read paired
// with C (S is off the read's line). The pairing the read answered travels beside the window
// (Conversation.sourceResultPairs), so every route writes the result under the call the read named.
func TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry(t *testing.T) {
	const reusedID = "toolu_reused_broken_ancestry"
	const resultText = "the answered call's own output"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the call", Dialogue: "the call",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: `{"command":"one"}`}}},
		// The inactive sibling reusing the id: its parent is the root, so it is off the read's line
		// (4 → c4 → a2 → u1), and it is the occurrence the reduced document's own order would choose.
		Message{Ordinal: 2, Role: "assistant", Content: "the sibling", Dialogue: "the sibling",
			SourceUUID: "b3", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: `{"command":"two"}`}}},
		// The record the result's recorded parent chain steps through: excluded by the window, so the
		// reduced document cannot walk the chain.
		Message{Ordinal: 3, Role: "assistant", Content: "the step", Dialogue: "the step",
			SourceUUID: "c4", SourceParentUUID: "a2"},
		Message{Ordinal: 4, Role: "user", SourceUUID: "d5", SourceParentUUID: "c4",
			ToolResults: []ToolResult{{ToolUseID: reusedID, ContentRaw: resultText,
				ContentSize: len(resultText)}}},
	)
	selected, report, err := Select(conversation, "2,4", false)
	if err != nil {
		t.Fatalf("selecting 2,4: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Fatalf("carried ordinals = %v, want the window's 2,4 with the call at ordinal 1 as the result's "+
			"counterpart", got)
	}
	if report.NecessitiesAdded.Count != 1 ||
		report.NecessitiesAdded.Added[0] != (SelectionNecessity{Ordinal: 1, PulledBy: 4}) {
		t.Errorf("necessities = %+v, want the call at ordinal 1 pulled in by the result",
			report.NecessitiesAdded)
	}
	for _, target := range []string{TargetCodex, TargetOpenCode, TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), resultText) {
			t.Errorf("%s: the written document does not hold the result the read paired with the call at "+
				"ordinal 1: re-reading the pairing over the window falls back to the carried sibling, "+
				"which the read's line leaves off it", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: the paired call is the carried one the read "+
				"named, not the sibling", target, count)
		}
	}
}

// TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath pins the tree writers' own half of the
// pairing. The read establishes no call for the result by ancestry -- its recorded parent's chain
// records no such id -- so the seam's own answer is the nearest prior occurrence, and that call stands
// on another branch: off the walk the target's loader makes from the result. The flat targets' line
// leaves the call off and they omit and count the result; the tree targets have to do the same rather
// than accept it through the file-wide id, because a tool_result written under a call that is not on its
// written ancestry states an invocation the source never placed there. The call itself still travels:
// it is recorded history the selection carried, in the file and off the walk.
func TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath(t *testing.T) {
	const branchResult = "the branch call's own output"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the branch turn", Dialogue: "the branch turn",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_branch", Name: "Bash",
				InputJSON: `{"command":"branch"}`}}},
		Message{Ordinal: 2, Role: "assistant", Content: "the active turn", Dialogue: "the active turn",
			SourceUUID: "c3", SourceParentUUID: "u1"},
		Message{Ordinal: 3, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "d4", SourceParentUUID: "c3"},
		// The result whose recorded parent's chain records no such id: the seam falls back to the nearest
		// prior occurrence, the call at ordinal 1 -- a sibling of the window's own line.
		Message{Ordinal: 4, Role: "user", SourceUUID: "e5", SourceParentUUID: "d4",
			ToolResults: []ToolResult{{ToolUseID: "toolu_branch", ContentRaw: branchResult,
				ContentSize: len(branchResult)}}},
	)
	selected, report, err := Select(conversation, "0,3-4", false)
	if err != nil {
		t.Fatalf("selecting 0,3-4: %v", err)
	}
	if got := selectionOrdinals(selected); len(got) != 4 || got[0] != 0 || got[1] != 1 || got[2] != 3 || got[3] != 4 {
		t.Fatalf("carried ordinals = %v, want the window's 0,3,4 with the call at ordinal 1 as the "+
			"result's counterpart", got)
	}
	if report.NecessitiesAdded.Count != 1 ||
		report.NecessitiesAdded.Added[0] != (SelectionNecessity{Ordinal: 1, PulledBy: 4}) {
		t.Errorf("necessities = %+v, want the call at ordinal 1 pulled in by the result",
			report.NecessitiesAdded)
	}

	for _, target := range []string{TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result whose paired call the line "+
				"leaves off", target, count)
		}
		if count := lossCount(t, converted.Losses, "inactive_branch_records").Count; count != 1 {
			t.Errorf("%s: inactive_branch_records = %d, want the one off-line record the window carried",
				target, count)
		}
		if strings.Contains(string(converted.Data), branchResult) {
			t.Errorf("%s: the written document holds the result, whose paired call is not on the line it "+
				"writes", target)
		}
	}

	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result whose paired call is off its "+
				"written ancestry", target, count)
		}
		data := string(converted.Data)
		if !strings.Contains(data, "the branch turn") {
			t.Errorf("%s: the written document does not hold the carried branch call at all: it is "+
				"recorded history the selection carried, and the rule is about the walk, not the file",
				target)
		}
		if strings.Contains(data, branchResult) {
			t.Errorf("%s: the written document holds the result under a call that is not on its written "+
				"ancestry", target)
		}
	}
	transcript, err := ConvertToClaudeTranscript(selected, ClaudeTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the window to claude: %v", err)
	}
	walkedBranchCall := false
	for _, record := range claudeWalkRecords(t, decodeTranscript(t, transcript)) {
		if claudeRecordHoldsInput(record, "toolu_branch", "branch") {
			walkedBranchCall = true
		}
	}
	if walkedBranchCall {
		t.Error("the walk from the transcript's last record holds the branch call, which the read's own " +
			"line leaves off it")
	}
	piRoute, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := piRoute.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the window to pi: %v", err)
	}
	lines := decodePiSession(t, session.Data)
	onPath := piWalkIDs(t, lines)
	for _, line := range lines[1:] {
		if onPath[line.ID] && strings.Contains(string(line.Message), `{"command":"branch"}`) {
			t.Error("the walk from the Pi file's last entry holds the branch call, which the read's own " +
				"line leaves off it")
		}
	}
}

// TestSelectionCountsAnIdLessCallWithNoRecordedOutcome pins the id-less arm of calls_undischarged: the
// source recorded a call with no id, so it holds no link for it either way -- and where the carried
// document holds no id-less result that a writer's own inference could pair with it, no outcome in the
// line can answer it and the call is counted, with the target's own report agreeing (OpenCode has no
// state for a call that has not finished and counts it unpaired_tool_calls). Where such a result *is*
// carried, which call it answers is the writers' disclosed inference rather than the read's, and the
// count does not assert an outcome the source never recorded: the call is left to that report. A result
// recorded *before* the call cannot answer it under the writers' own rule (their inference reads the
// outstanding call the line wrote), so that shape is counted too -- and so is an id-less result whose own
// recorded status says the call is still running: progress is not an outcome, and the outcome-status
// control shows the same event's other arm.
func TestSelectionCountsAnIdLessCallWithNoRecordedOutcome(t *testing.T) {
	call := func(ordinal int) Message {
		return Message{Ordinal: ordinal, Role: "assistant",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`}}}
	}
	result := func(ordinal int, raw string) Message {
		return Message{Ordinal: ordinal, Role: "user",
			ToolResults: []ToolResult{{ContentRaw: raw, ContentSize: len(raw)}}}
	}

	// No result anywhere in the read: the call is undischarged, and the target whose format cannot hold
	// a call with no outcome agrees by counting it under its own category.
	alone := openCodeTestConversation(call(0))
	selected, report, err := Select(alone, "0", false)
	if err != nil {
		t.Fatalf("selecting the id-less call: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the one id-less call: the carried document holds no "+
			"result that could answer it", report.CallsUndischarged)
	}
	if report.NecessitiesAdded.Count != 0 {
		t.Errorf("necessities = %+v, want none: the read recorded no counterpart to carry",
			report.NecessitiesAdded)
	}
	session, err := ConvertToOpenCodeSession(selected, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the id-less call: %v", err)
	}
	if session.Carried.ToolCalls != 0 {
		t.Errorf("opencode carried = %+v, want no call: this route's format has no state for one with no "+
			"outcome", session.Carried)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 1)

	// An id-less result *after* the call: the writers' inference may pair the two (the sole outstanding
	// call in the line it wrote), so the count does not state either disposition and the writer's own
	// report names the inference instead.
	withResult := openCodeTestConversation(call(0), result(1, "the read output"))
	selected, report, err = Select(withResult, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the call with its id-less result: %v", err)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: an id-less result the carried line holds may answer "+
			"the call, and whether it does is the writer's own disclosed inference", report.CallsUndischarged)
	}
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the id-less pair: %v", err)
	}
	// The disclosure the ambiguity is left to: the writer names the generated id it minted for the call
	// and the inferred pairing it wrote the result under (generated_call_ids), so the ambiguity travels
	// with the conversion instead of the report asserting a pairing the source never recorded.
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 {
		t.Errorf("codex carried = %+v, want the call and the result the writer's own inference paired with it",
			rollout.Carried)
	}
	if count := lossCount(t, rollout.Losses, "generated_call_ids").Count; count != 2 {
		t.Errorf("generated_call_ids = %d, want the minted call id and the inferred pairing counted",
			count)
	}

	// A result recorded *before* the call cannot answer it: the writers' inference reads the outstanding
	// call the line wrote, so a result the line already passed is not a candidate, and the call is counted.
	before := openCodeTestConversation(result(0, "an earlier tool output"), call(1))
	_, report, err = Select(before, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the result before the call: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "1" {
		t.Errorf("calls_undischarged = %+v, want the call at ordinal 1: the id-less result stands before "+
			"it in the line and cannot be paired with it", report.CallsUndischarged)
	}

	// An id-less *progress* event is not an outcome either: the result says the call is still running, so
	// the count cannot be left to the writers' inference -- the call is counted, exactly as an id-bearing
	// progress-only call is, and the same event with an outcome status leaves the count to that inference.
	progress := openCodeTestConversation(call(0), Message{Ordinal: 1, Role: "user",
		ToolResults: []ToolResult{{Status: "running", ContentRaw: "Still working",
			ContentSize: len("Still working")}}})
	_, report, err = Select(progress, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the call with an id-less progress event: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the call counted: an id-less progress event states no "+
			"outcome", report.CallsUndischarged)
	}
	outcome := openCodeTestConversation(call(0), Message{Ordinal: 1, Role: "user",
		ToolResults: []ToolResult{{Status: "completed", ContentRaw: "the read output",
			ContentSize: len("the read output")}}})
	_, report, err = Select(outcome, "0-1", false)
	if err != nil {
		t.Fatalf("selecting the call with an id-less outcome event: %v", err)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: an id-less outcome event the carried line holds "+
			"may answer the call, and whether it does is the writer's own disclosed inference",
			report.CallsUndischarged)
	}
}

// TestSelectionCountsASummaryOnlyCallAsAnswered pins calls_undischarged against the archive's own
// outcome column. The read holds a call whose only evidence of an outcome is the summary the archive
// stores on the call itself (ToolCall.ResultContent), and the routes write that summary as the call's
// completed outcome (derivedResultSummary) -- so the call is answered, and the count must not warn that
// the read held no outcome for it. Where a recorded result event stands for the call, the summary is a
// view of that event and the call's disposition follows it: an outcome answers, and a progress event
// still does not, because the writers refuse to present progress as an outcome.
func TestSelectionCountsASummaryOnlyCallAsAnswered(t *testing.T) {
	const summary = "summary-only outcome"
	summaryOnly := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "reading", DialogueProjected: true, Content: "reading",
			ToolCalls: []ToolCall{{ToolUseID: "call_summary", Name: "Read",
				InputJSON: `{"file_path":"/x"}`, ResultContent: summary, ResultContentSize: len(summary)}}},
	)
	selected, report, err := Select(summaryOnly, "0", false)
	if err != nil {
		t.Fatalf("selecting the summary-only call: %v", err)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the archive's own summary of the call is the "+
			"outcome evidence the read holds", report.CallsUndischarged)
	}
	// The routes agree: the summary is written as the call's completed outcome, so a warning that the
	// read held no outcome would contradict what the conversion itself published.
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the summary-only call: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 {
		t.Errorf("codex carried = %+v, want the call with its summary as the outcome", rollout.Carried)
	}
	if !strings.Contains(string(rollout.Data), summary) {
		t.Error("the rollout does not hold the archive's summary of the call's outcome")
	}
	session, err := ConvertToOpenCodeSession(selected, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the summary-only call to opencode: %v", err)
	}
	openCodeRequireLoss(t, session, "unpaired_tool_calls", 0)
	openCodeRequireLoss(t, session, "unpaired_tool_results", 0)

	// The progress arm: the summary beside a `running` event is a view of that event, the writers refuse
	// to present it as an outcome, and the count follows them.
	progress := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "spawning", DialogueProjected: true, Content: "spawning",
			ToolCalls: []ToolCall{{ToolUseID: "call_pending", Name: "spawn_agent",
				InputJSON: `{"agent_type":"awaiter"}`, ResultContent: "Still working",
				ResultContentSize: len("Still working")}},
			ToolResults: []ToolResult{{ToolUseID: "call_pending", Status: "running",
				ContentRaw: "Still working", ContentSize: len("Still working"), ContentDecoded: true}}},
	)
	_, report, err = Select(progress, "0", false)
	if err != nil {
		t.Fatalf("selecting the progress-only call: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Errorf("calls_undischarged = %+v, want the call counted: its only recorded event is progress, "+
			"and the summary is that event's own text", report.CallsUndischarged)
	}

	// The id-less variant of the summary-only shape: the archive row recorded no call id, and the writers
	// carry the summary under the id they mint for the call (the sole outstanding call of the line), so
	// the count must not warn about an outcome the conversion writes.
	idless := openCodeTestConversation(
		Message{Role: "assistant", Dialogue: "reading", DialogueProjected: true, Content: "reading",
			ToolCalls: []ToolCall{{Name: "Read", InputJSON: `{"file_path":"/x"}`,
				ResultContent: summary, ResultContentSize: len(summary)}}},
	)
	selected, report, err = Select(idless, "0", false)
	if err != nil {
		t.Fatalf("selecting the id-less summary-only call: %v", err)
	}
	if report.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the archive's summary is the outcome evidence the "+
			"read holds for the id-less call too", report.CallsUndischarged)
	}
	rollout, err = ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the id-less summary-only call: %v", err)
	}
	if rollout.Carried.ToolCalls != 1 || rollout.Carried.ToolResults != 1 {
		t.Errorf("codex carried = %+v, want the call with its summary as the outcome", rollout.Carried)
	}
	if !strings.Contains(string(rollout.Data), summary) {
		t.Error("the rollout does not hold the archive's summary of the id-less call's outcome")
	}
}

// TestSelectionClosesTheCounterpartGraphToAFixedPoint pins D3's default arm as a fixed point rather than
// one forward pass: a counterpart can itself hold a pair whose other half lies outside the window (a
// record carrying one call's result and the next call), so a record a *newly added* record pulled in can
// pull in a second counterpart behind the walk's cursor. Selecting ordinal 2 alone must carry all three
// records, each named with its own puller; a single in-order pass would carry ordinal 1 and leave ordinal
// 0 -- the call whose result ordinal 1 also carries -- behind, writing a result without its call.
func TestSelectionClosesTheCounterpartGraphToAFixedPoint(t *testing.T) {
	conversation := Conversation{Messages: []Message{
		{Ordinal: 0, Role: "assistant",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_first", Name: "Read", InputJSON: `{"file_path":"/a"}`}}},
		{Ordinal: 1, Role: "assistant",
			ToolCalls:   []ToolCall{{ToolUseID: "toolu_second", Name: "Read", InputJSON: `{"file_path":"/b"}`}},
			ToolResults: []ToolResult{{ToolUseID: "toolu_first", ContentRaw: "the first output"}}},
		{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: "toolu_second", ContentRaw: "the second output"}}},
	}}
	selected, report, err := Select(conversation, "2", false)
	if err != nil {
		t.Fatalf("selecting 2: %v", err)
	}
	if got := selectionOrdinals(selected); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("carried ordinals = %v, want the whole chain 0,1,2: the record the window pulled in "+
			"carries a result of the first call", got)
	}
	want := []SelectionNecessity{{Ordinal: 0, PulledBy: 1}, {Ordinal: 1, PulledBy: 2}}
	if report.NecessitiesAdded.Count != 2 || !reflect.DeepEqual(report.NecessitiesAdded.Added, want) {
		t.Errorf("necessities = %+v, want %+v: each addition names the record that pulled it in",
			report.NecessitiesAdded, want)
	}
	if report.Included.Count != 1 || report.Excluded.Count != 0 {
		t.Errorf("report = %+v, want the one included record and no exclusion", report)
	}
}
