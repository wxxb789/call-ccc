package ccc

import (
	"context"
	"errors"
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
		want string
	}{
		{"2", selectionBuildCallID, "is selected and its recorded result"},
		{"3", selectionBuildCallID, "is selected and the call itself"},
		{"7", selectionCapCallID, "is selected and its recorded result"},
		{"8", selectionCapCallID, "is selected and the call itself"},
	}
	for _, testCase := range cases {
		selected, report, err := Select(conversation, testCase.spec, true)
		if !errors.Is(err, ErrStrictSelection) {
			t.Errorf("strictly selecting %q: err = %v, want ErrStrictSelection", testCase.spec, err)
			continue
		}
		for _, want := range []string{testCase.call, testCase.want, "--strict-selection"} {
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
