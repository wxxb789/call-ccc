package ccc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/parser"
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

// TestSelectionWritesExactlyTheSelectedOrdinals pins D1's reproduction half over every route: the
// written document holds the evidence of exactly the selected records (and the counterparts a recorded
// pair required) and the evidence of no excluded record, so "the selection reproduced the selected
// ordinals" is read off the target's own bytes rather than off the conversation this build built.
//
// The check is exact for every one of the five routes: each document is decoded into its ordered
// evidence -- dialogue, calls with their ids, names and argument objects, results with the call ids they
// pair and the texts they carry, the target's own envelopes left out -- and compared to the expected
// slice, so multiplicity, order and every call/result link are asserted rather than a membership of
// substrings (a duplicated tool-result entry, for one, leaves a second token no expectation holds). The
// fixture's raw bytes are scanned for the excluded material besides, because a token the extractor knew
// would appear in the evidence and one written into a field it does not read must both be absent.
//
// One shape is not the selection's to write: a call the source never answered has no state in OpenCode's
// own format (measured on 2.2.5 -- its importer accepts only a completed or failed tool state), so that
// route leaves the call out and counts it under its own category. The case pins both halves: the route
// holds no evidence of the call, and its report counts the omission by name.
func TestSelectionWritesExactlyTheSelectedOrdinals(t *testing.T) {
	conversation := readSelectionFixture(t)
	type omission struct {
		evidence string
		category string
	}
	// The call argument objects in the one canonical spelling the decoded evidence is printed in (keys
	// sorted, no HTML escaping), written out as the fixture recorded them.
	call := func(id, name, input string) string { return "call " + id + " " + name + " " + input }
	result := func(id, text string) string { return "result " + id + " " + text }
	const (
		buildInput = `{"command":"sleep 30 && echo build-ok","description":"run the long build"}`
		readInput  = `{"file_path":"/home/user/code/awaiting/internal/retry/config.go"}`
		capInput   = `{"command":"grep -rn maxBackoffSeconds internal/retry","description":"find where the backoff cap is configured"}`
	)
	cases := []struct {
		spec string
		// evidence is every route's ordered evidence unless routeEvidence overrides it for one route.
		evidence      []string
		routeEvidence map[string][]string
		absent        []string
		// omitted is keyed by route: the evidence that route's own format cannot hold, with the loss
		// category its report counts the omission under.
		omitted map[string]omission
	}{
		{
			// The call alone: the result's own record is the necessity that travels with it, and every
			// other record's words -- the first prompt, the assistant's turns, the later question and its
			// answer -- stay out of the target.
			spec: "2",
			evidence: []string{
				call(selectionBuildCallID, "Bash", buildInput),
				result(selectionBuildCallID, selectionBuildResult),
			},
			absent: []string{selectionQuestion, "Starting the build now.", "The build finished.",
				selectionReadCallID, selectionCapCallID, selectionFollowup, selectionAnswer, selectionSummary},
		},
		{
			// The undischarged call alone: it travels, and nothing else does -- except on OpenCode,
			// whose importer has no state for a call the source never answered.
			spec: "5",
			evidence: []string{
				call(selectionReadCallID, "Read", readInput),
			},
			routeEvidence: map[string][]string{TargetOpenCode: nil},
			absent:        []string{selectionBuildCallID, selectionCapCallID, selectionFollowup, selectionAnswer},
			omitted: map[string]omission{
				TargetOpenCode: {evidence: selectionReadCallID, category: "unpaired_tool_calls"},
			},
		},
		{
			// A window that holds one whole pair: both halves travel, the earlier pair stays behind.
			spec: "7-8",
			evidence: []string{
				call(selectionCapCallID, "Bash", capInput),
				result(selectionCapCallID, selectionCapResult),
			},
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
			want := testCase.evidence
			if override, ok := testCase.routeEvidence[target]; ok {
				want = override
			}
			got := selectionEvidence(t, target, converted.Data)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: selecting %q wrote the evidence\n got: %q\nwant: %q", target, testCase.spec,
					got, want)
			}
			bytes := string(converted.Data)
			omitted := testCase.omitted[target]
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

// selectionEvidence decodes one route's written document into the ordered evidence of the conversation
// it holds: each dialogue text, each call with the id, name and argument object it was written with, and
// each result with the call id it pairs and the text it carries. Each target's own envelopes -- session
// metadata, turn contexts, model bindings, the target's notification echo of a message -- are left out;
// a block, part or payload the extractor does not know fails the case, so a writer that adds one is a
// failing case rather than a passing one. Multiplicity and order are the returned slice's own.
func selectionEvidence(t *testing.T, target string, data []byte) []string {
	t.Helper()
	var evidence []string
	text := func(t *testing.T, value any) {
		t.Helper()
		evidence = append(evidence, "text "+value.(string))
	}
	call := func(id, name string, input any) {
		evidence = append(evidence, "call "+id+" "+name+" "+canonicalEvidenceJSON(t, input))
	}
	blockType := func(t *testing.T, block map[string]any) string {
		t.Helper()
		kind, _ := block["type"].(string)
		if kind == "" {
			t.Fatalf("%s: a content block carries no type: %v", target, block)
		}
		return kind
	}
	switch target {
	case TargetClaude:
		for _, record := range decodeTranscript(t, ClaudeTranscript{Data: data}) {
			if _, ok := record["message"]; !ok {
				continue // a record this writer owns: the compaction marker and the like
			}
			for _, block := range messageBlocks(t, record) {
				switch blockType(t, block) {
				case claudeBlockText:
					text(t, block["text"])
				case claudeBlockToolUse:
					call(block["id"].(string), block["name"].(string), block["input"])
				case claudeBlockToolResult:
					evidence = append(evidence, "result "+block["tool_use_id"].(string)+" "+
						evidenceText(t, target, block["content"]))
				default:
					t.Fatalf("%s: unhandled content block %v", target, block)
				}
			}
		}
	case TargetPi, TargetOMP:
		for _, line := range decodePiSession(t, data) {
			switch line.Type {
			case "session", "title", "model_change":
				continue // the family's own envelopes: the session header, OMP's title slot, its model binding
			case "message":
			default:
				t.Fatalf("%s: unhandled entry type %q", target, line.Type)
			}
			var message struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"toolCallId"`
			}
			if err := json.Unmarshal(line.Message, &message); err != nil {
				t.Fatalf("%s: decoding entry %s: %v", target, line.ID, err)
			}
			var plain string
			if err := json.Unmarshal(message.Content, &plain); err == nil {
				text(t, plain)
				continue
			}
			var blocks []map[string]any
			if err := json.Unmarshal(message.Content, &blocks); err != nil {
				t.Fatalf("%s: entry %s holds content that is neither text nor blocks: %s", target, line.ID,
					message.Content)
			}
			if message.Role == "toolResult" {
				for _, block := range blocks {
					switch blockType(t, block) {
					case piBlockText:
						evidence = append(evidence, "result "+message.ToolCallID+" "+block["text"].(string))
					default:
						t.Fatalf("%s: unhandled toolResult block %v", target, block)
					}
				}
				continue
			}
			for _, block := range blocks {
				switch blockType(t, block) {
				case piBlockText:
					text(t, block["text"])
				case piBlockThinking:
					evidence = append(evidence, "reasoning "+block["thinking"].(string))
				case piBlockToolCall:
					call(block["id"].(string), block["name"].(string), block["arguments"])
				default:
					t.Fatalf("%s: unhandled content block %v", target, block)
				}
			}
		}
	case TargetCodex:
		for _, line := range decodeRollout(t, data) {
			if line.Type != codexRecordResponseItem {
				continue // session_meta, turn_context, event_msg and compacted are the target's own
			}
			var payload map[string]any
			if err := json.Unmarshal(line.Payload, &payload); err != nil {
				t.Fatalf("%s: decoding a response item: %v", target, err)
			}
			switch kind, _ := payload["type"].(string); kind {
			case codexItemMessage:
				content, _ := payload["content"].([]any)
				for _, raw := range content {
					block, _ := raw.(map[string]any)
					switch blockType(t, block) {
					case "input_text", "output_text", "text":
						text(t, block["text"])
					default:
						t.Fatalf("%s: unhandled message content %v", target, block)
					}
				}
			case codexItemFunctionCall:
				call(payload["call_id"].(string), payload["name"].(string), payload["arguments"])
			case codexItemFunctionOutput:
				evidence = append(evidence, "result "+payload["call_id"].(string)+" "+
					evidenceText(t, target, payload["output"]))
			default:
				t.Fatalf("%s: unhandled response item %v", target, payload)
			}
		}
	case TargetOpenCode:
		var document struct {
			Messages []struct {
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("%s: decoding the import document: %v", target, err)
		}
		for _, message := range document.Messages {
			for _, part := range message.Content {
				switch blockType(t, part) {
				case "text":
					text(t, part["text"])
				case "tool":
					state, ok := part["state"].(map[string]any)
					if !ok {
						t.Fatalf("%s: a tool part carries no state: %v", target, part)
					}
					call(part["id"].(string), part["name"].(string), state["input"])
					content, _ := state["content"].([]any)
					for _, raw := range content {
						block, _ := raw.(map[string]any)
						switch blockType(t, block) {
						case "text":
							evidence = append(evidence, "result "+part["id"].(string)+" "+
								block["text"].(string))
						default:
							t.Fatalf("%s: unhandled tool output %v", target, block)
						}
					}
				default:
					t.Fatalf("%s: unhandled message part %v", target, part)
				}
			}
		}
	default:
		t.Fatalf("selectionEvidence has no reader for target %q", target)
	}
	return evidence
}

// evidenceText answers one block or payload value as the text evidence it is: a plain string, or a
// list of text blocks joined the way the targets' own readers join them (a block list with anything
// but text in it is a shape the extractor does not know).
func evidenceText(t *testing.T, target string, raw any) string {
	t.Helper()
	if value, ok := raw.(string); ok {
		return value
	}
	blocks, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s: a text value is neither a string nor a block list: %v", target, raw)
	}
	var builder strings.Builder
	for _, entry := range blocks {
		block, _ := entry.(map[string]any)
		kind, _ := block["type"].(string)
		if kind != "text" {
			t.Fatalf("%s: a text block list holds a %q block: %v", target, kind, block)
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(block["text"].(string))
	}
	return builder.String()
}

// canonicalEvidenceJSON prints one call's argument object in the one form the expectations are written
// in: keys sorted, no HTML escaping, whatever spelling the target held it in (an object, or the JSON
// *string* Codex's arguments field carries).
func canonicalEvidenceJSON(t *testing.T, raw any) string {
	t.Helper()
	if text, ok := raw.(string); ok {
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			t.Fatalf("an argument value is not JSON: %v (%q)", err, text)
		}
		raw = decoded
	}
	var buffer strings.Builder
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(raw); err != nil {
		t.Fatalf("canonicalizing an argument object: %v", err)
	}
	return strings.TrimRight(buffer.String(), "\n")
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

// TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer pins the eighth round's fourth finding: the flat
// targets' unresolved_source_parents count is computed from the *read's* own relation answers, so a parent
// the caller excluded is not reported as a relation the read could not place. On the Pi history fixture,
// --select 0,5 leaves out ordinal 1, the leaf's own recorded parent, while the read's chain 5 -> 1 -> 0
// reaches the carried root: the window reports 0, exactly the whole read's own answer. The count still
// names the places the read did not answer -- the same fixture's leaf alone retains no ancestor at all, so
// the conversion's own place is counted once per flat route; and on the call fixture's window 1,4 the
// count is the excluded root and the read's tool-result projection, the same two places the tree route
// reports for it, so the flat routes and the tree routes cannot disagree about the read.
func TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer(t *testing.T) {
	history, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piHistoryFixtureRef)
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	// The read's own answer for the whole document: nothing unresolved.
	route, err := targetRouteFor(TargetCodex)
	if err != nil {
		t.Fatalf("route %s: %v", TargetCodex, err)
	}
	whole, err := route.Convert(history, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the whole read: %v", err)
	}
	if loss := lossCount(t, whole.Losses, "unresolved_source_parents"); loss.Count != 0 {
		t.Errorf("the whole read counts unresolved_source_parents = %+v, want 0: every record's place is "+
			"the read's own answer", loss)
	}

	flatRoutes := []string{TargetCodex, TargetOpenCode}
	counts := []struct {
		spec string
		want int
		why  string
	}{
		{"0,5", 0, "the leaf's parent (ordinal 1) is excluded but the read's chain reaches the carried " +
			"root, so the place is the read's own answer"},
		{"5", 1, "the leaf alone retains no ancestor at all: its entry follows nothing the read answered"},
	}
	for _, arm := range counts {
		selected, _, err := Select(history, arm.spec, false)
		if err != nil {
			t.Fatalf("selecting %s: %v", arm.spec, err)
		}
		for _, target := range flatRoutes {
			route, err := targetRouteFor(target)
			if err != nil {
				t.Fatalf("route %s: %v", target, err)
			}
			converted, err := route.Convert(selected, selectionProbeOptions())
			if err != nil {
				t.Fatalf("%s: converting %s: %v", target, arm.spec, err)
			}
			if loss := lossCount(t, converted.Losses, "unresolved_source_parents"); loss.Count != arm.want {
				t.Errorf("%s: window %s counts unresolved_source_parents = %+v, want %d: %s",
					target, arm.spec, loss, arm.want, arm.why)
			}
		}
	}

	// The call fixture's 1,4 window, where the count is not zero: the two places are the read's own --
	// the excluded root (no retained ancestor) and the chain that crossed the read's tool-result
	// projection -- and the flat routes report the same number the tree route's own case asserts.
	calls, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piFixtureRef)
	if err != nil {
		t.Fatalf("reading the call fixture: %v", err)
	}
	selected, _, err := Select(calls, "1,4", false)
	if err != nil {
		t.Fatalf("selecting 1,4: %v", err)
	}
	for _, target := range flatRoutes {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the call window: %v", target, err)
		}
		if loss := lossCount(t, converted.Losses, "unresolved_source_parents"); loss.Count != 2 {
			t.Errorf("%s: the 1,4 call window counts unresolved_source_parents = %+v, want the two "+
				"places the tree route reports for it: the excluded root and the projection", target, loss)
		}
	}

	// A whole-read selection does not move the count: every written record's place is the read's own
	// answer, so the reduction of every record reports what the unflagged conversion reports -- the call
	// fixture's one projection place, the history fixture's none.
	for _, arm := range []struct {
		conversation Conversation
		want         int
	}{{calls, 1}, {history, 0}} {
		selected, _, err := Select(arm.conversation, "0-", false)
		if err != nil {
			t.Fatalf("selecting every record: %v", err)
		}
		for _, target := range flatRoutes {
			route, err := targetRouteFor(target)
			if err != nil {
				t.Fatalf("route %s: %v", target, err)
			}
			unflagged, err := route.Convert(arm.conversation, selectionProbeOptions())
			if err != nil {
				t.Fatalf("%s: converting the read: %v", target, err)
			}
			if got := lossCount(t, unflagged.Losses, "unresolved_source_parents").Count; got != arm.want {
				t.Fatalf("%s: the unflagged conversion counts unresolved_source_parents = %d, want %d",
					target, got, arm.want)
			}
			converted, err := route.Convert(selected, selectionProbeOptions())
			if err != nil {
				t.Fatalf("%s: converting the whole-read selection: %v", target, err)
			}
			if got := lossCount(t, converted.Losses, "unresolved_source_parents").Count; got != arm.want {
				t.Errorf("%s: the whole-read selection counts unresolved_source_parents = %d, want the "+
					"unflagged conversion's own %d: a selection of every record does not move the count",
					target, got, arm.want)
			}
		}
	}
}

// selectionCallAndResultLines answers the evidence lines one written document holds for tool calls and
// their results, in the document's own order: "call <id> <name> <input>" and "result <id> <text>", the
// spellings selectionEvidence answers every route with. A case that needs one call's own line asks here.
func selectionCallAndResultLines(t *testing.T, target string, data []byte) []string {
	t.Helper()
	var lines []string
	for _, line := range selectionEvidence(t, target, data) {
		if strings.HasPrefix(line, "call ") || strings.HasPrefix(line, "result ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// openCodeToolPartTimes answers one written import document's tool parts' own `time` members, in the
// document's order: what a case reads to assert the exact instant a part's outcome was written at, rather
// than only the text it carries.
func openCodeToolPartTimes(t *testing.T, data []byte) []*openCodeTimeSpan {
	t.Helper()
	var document openCodeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decoding the import document: %v", err)
	}
	var times []*openCodeTimeSpan
	for _, message := range document.Messages {
		for _, part := range message.Content {
			if part.Type == openCodePartTool {
				times = append(times, part.Time)
			}
		}
	}
	return times
}

// TestSelectionAnswersOnlyForTheOutcomesItWrites pins the eighth round's second finding: a recorded
// result the conversion omits does not answer its call on the written path. Two calls on the selected
// path, a linked terminal result for the first whose text is whitespace only (the target's output field
// cannot carry it), and then an id-less result: both calls are unanswered in the document, so the id-less
// result may not be inferred onto the second one -- it is omitted and counted, and no id is minted for
// it. The control arm makes the first call's outcome writable: then it answers the call, the second
// call is the sole outstanding occurrence, and the id-less result is written under it as before.
func TestSelectionAnswersOnlyForTheOutcomesItWrites(t *testing.T) {
	const (
		blankOutput = "   "
		firstOutput = "the first call's own output"
		lateOutput  = "the late output"
	)
	withBlankFirst := func(firstRaw string) Conversation {
		return testConversation(
			Message{Ordinal: 0, Role: "assistant", Content: "call a", Dialogue: "call a",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Bash", InputJSON: `{"command":"a"}`}}},
			Message{Ordinal: 1, Role: "user", Content: "answer a", Dialogue: "answer a",
				ToolResults: []ToolResult{{ToolUseID: "toolu_a", ContentRaw: firstRaw}}},
			Message{Ordinal: 2, Role: "assistant", Content: "call b", Dialogue: "call b",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_b", Name: "Bash", InputJSON: `{"command":"b"}`}}},
			Message{Ordinal: 3, Role: "user", Content: "the late result", Dialogue: "the late result",
				ToolResults: []ToolResult{{ContentRaw: lateOutput}}},
		)
	}
	selected, _, err := Select(withBlankFirst(blankOutput), "0-", false)
	if err != nil {
		t.Fatalf("selecting the omitted-outcome window: %v", err)
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
		for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
			if strings.Contains(line, lateOutput) {
				t.Errorf("%s: the written document holds the id-less result although no written result "+
					"answered either call: %q", target, line)
			}
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one id-less result neither call's "+
				"written path answers", target, got)
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: the inference picked no call, so no id was "+
				"minted for the result", target, got)
		}
		if got := lossCount(t, converted.Losses, "whitespace_only_tool_results").Count; got != 1 {
			t.Errorf("%s: whitespace_only_tool_results = %d, want the omitted result's own count",
				target, got)
		}
	}

	control, _, err := Select(withBlankFirst(firstOutput), "0-", false)
	if err != nil {
		t.Fatalf("selecting the writable-outcome window: %v", err)
	}
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(control, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the control window: %v", target, err)
		}
		written := false
		for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
			if strings.Contains(line, lateOutput) {
				written = true
			}
		}
		if !written {
			t.Errorf("%s: the control's id-less result was not written although its sole outstanding "+
				"call is the second one", target)
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 0 {
			t.Errorf("%s: the control counts unpaired_tool_results = %d, want 0", target, got)
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 1 {
			t.Errorf("%s: the control counts generated_call_ids = %d, want the one inferred result",
				target, got)
		}
	}
}

// TestSelectionAnswersASummaryFromItsOwningOccurrence pins the eighth round's third finding: a summary
// the read stored on an id-less call (ToolCall.ResultContent) is that call's own outcome, so the writer
// writes it under the id that occurrence stands under and never infers a different owner from the
// outstanding set. The shape is the one pár the inference dropped: two outstanding calls on one record,
// the id-less one first and holding its only outcome as the summary. Every route writes the summary, so
// the call's own outcome evidence is in the document; the flags below assert which count carries it.
func TestSelectionAnswersASummaryFromItsOwningOccurrence(t *testing.T) {
	const (
		idlessSummary = "the id-less call's own outcome"
		firstSummary  = "the first call's own outcome"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "the two calls", Dialogue: "the two calls",
			ToolCalls: []ToolCall{
				// The id-less call first: the shape whose summary the blank-id branch used to infer off
				// the outstanding set although its owner was the record that stored it.
				{Name: "Bash", InputJSON: `{"command":"idless"}`,
					ResultContent: idlessSummary, ResultContentSize: len(idlessSummary)},
				{ToolUseID: "toolu_first", Name: "Bash", InputJSON: `{"command":"first"}`,
					ResultContent: firstSummary, ResultContentSize: len(firstSummary)},
			}},
	)
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the summary window: %v", err)
	}
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the summary window: %v", target, err)
		}
		ids := map[string]string{}
		for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
			if strings.HasPrefix(line, "call ") {
				fields := strings.SplitN(line, " ", 4)
				if len(fields) != 4 {
					t.Fatalf("%s: a call line is not 'call <id> <name> <input>': %q", target, line)
				}
				ids[fields[3]] = fields[1]
			}
		}
		idlessCallID, ok := ids[`{"command":"idless"}`]
		if !ok {
			t.Fatalf("%s: the written document holds no call for the id-less occurrence", target)
		}
		firstCallID, ok := ids[`{"command":"first"}`]
		if !ok {
			t.Fatalf("%s: the written document holds no call for the id-carrying occurrence", target)
		}
		for _, pair := range []struct{ id, summary, who string }{
			{idlessCallID, idlessSummary, "the id-less call's summary must be written under the id " +
				"that occurrence stands under, not inferred onto the other call"},
			{firstCallID, firstSummary, "the id-carrying call's summary travels under its recorded id"},
		} {
			found := false
			for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
				if line == "result "+pair.id+" "+pair.summary {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: the document holds no result %s %q: %s", target, pair.id, pair.summary,
					pair.who)
			}
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: both summaries' owners were known",
				target, got)
		}
		// Codex, Claude and Pi/OMP count the id-less call and its summary as two records written under
		// generated ids; OpenCode's tool part holds the call and its state together, so its own
		// vocabulary counts the part's one id.
		wantGenerated := 2
		if target == TargetOpenCode {
			wantGenerated = 1
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != wantGenerated {
			t.Errorf("%s: generated_call_ids = %d, want %d", target, got, wantGenerated)
		}
	}
}

// TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall pins the ninth round's first finding for
// the route that has the shape: a call whose only recorded outcome is the archive's writable
// ResultContent summary is answered on the written path, so a later id-less result may not be inferred
// onto it -- the summary pass would skip it (its result slot is occupied) and the recorded outcome would
// be replaced by an unrelated result the source never wrote beside it. The reduced document writes the
// summary for its call and omits the id-less result (counted). The default path keeps the ledger's
// id-keyed answer, which is what a run without a selection has always had (goal 0009's D4): there the
// call is still outstanding and the id-less result is written under it.
func TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall(t *testing.T) {
	const (
		summaryText = "the call's own recorded outcome"
		lateText    = "the unrelated late output"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_summary", Name: "Bash",
				InputJSON:     `{"command":"a"}`,
				ResultContent: summaryText, ResultContentSize: len(summaryText)}}},
		Message{Ordinal: 1, Role: "user", Content: "the late result", Dialogue: "the late result",
			ToolResults: []ToolResult{{ContentRaw: lateText, ContentSize: len(lateText)}}},
	)
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the summary window: %v", err)
	}
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("route %s: %v", TargetOpenCode, err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the summary window: %v", err)
	}
	if !strings.Contains(string(converted.Data), summaryText) {
		t.Errorf("the selected document does not hold the call's own recorded summary: the id-less " +
			"result took the slot the summary was owed")
	}
	if strings.Contains(string(converted.Data), lateText) {
		t.Errorf("the selected document holds the id-less result although the only call is answered by " +
			"its summary: an inference onto it would replace the recorded outcome")
	}
	if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 1 {
		t.Errorf("unpaired_tool_results = %d, want the one id-less result no written call answers", got)
	}
	if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 0 {
		t.Errorf("generated_call_ids = %d, want 0: nothing was inferred", got)
	}

	// The default path keeps the ledger's id-keyed answer: the summary-only call is still outstanding
	// there, so the id-less result is written under it (counted as an inference) and the summary is not
	// -- the answer a run without a selection has always had (goal 0009's D4).
	unflagged, err := route.Convert(conversation, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the read without a selection: %v", err)
	}
	if !strings.Contains(string(unflagged.Data), lateText) {
		t.Errorf("the unflagged document does not hold the id-less result: the ledger's id-keyed " +
			"outstanding set is the default path's answer")
	}
	if strings.Contains(string(unflagged.Data), summaryText) {
		t.Errorf("the unflagged document holds the summary the id-less result displaced: the default " +
			"path's bytes are what a run without a selection has always written")
	}
	if got := lossCount(t, unflagged.Losses, "generated_call_ids").Count; got != 1 {
		t.Errorf("unflagged generated_call_ids = %d, want the one inferred result", got)
	}

	// The summary counts as an answer only where the written path can carry it: a summary whose text is
	// whitespace only is not a part this target writes, so the call stays unanswered and the id-less
	// result is what the document can hold -- inferred onto it and counted, exactly as a call with no
	// writable outcome has always been read (openCodeResultWritable's text guard).
	blank := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_summary", Name: "Bash",
				InputJSON:     `{"command":"a"}`,
				ResultContent: "   ", ResultContentSize: 3}}},
		Message{Ordinal: 1, Role: "user", Content: "the late result", Dialogue: "the late result",
			ToolResults: []ToolResult{{ContentRaw: lateText, ContentSize: len(lateText)}}},
	)
	selectedBlank, _, err := Select(blank, "0-", false)
	if err != nil {
		t.Fatalf("selecting the blank-summary window: %v", err)
	}
	blankConverted, err := route.Convert(selectedBlank, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the blank-summary window: %v", err)
	}
	if !strings.Contains(string(blankConverted.Data), lateText) {
		t.Errorf("the selected document does not hold the id-less result: a whitespace-only summary is " +
			"no outcome the target can write, so the call was still unanswered")
	}
	if got := lossCount(t, blankConverted.Losses, "generated_call_ids").Count; got != 1 {
		t.Errorf("blank-summary generated_call_ids = %d, want the one inferred result", got)
	}
}

// TestSelectionReservesARecordedResultsSlotFromAnIdLessInference pins the tenth round's first finding:
// an id-less result may be inferred only onto a call occurrence the document's own records do not
// already answer. The shape is a Pi read's: call B, call A, an id-less result, and then a result that
// records A's id, with the window carrying A and both result records and leaving B out. Both halves of
// the recorded pair are retained, so the explicit result must take A's one output slot and the id-less
// event -- which no record links to a call -- must be omitted and counted; inferring it onto A instead
// displaced the recorded pairing (the explicit result then read as a repeat) and wrote the source's
// words as the outcome of an invocation it never recorded them for. The second arm holds the reservation
// to what it claims: a recorded result whose text this target cannot write reserves nothing, so the
// id-less event may still be inferred onto its call, and a third carried call makes the id-less event
// ambiguous even though the reserved call is going to be answered.
func TestSelectionReservesARecordedResultsSlotFromAnIdLessInference(t *testing.T) {
	const (
		firstInput   = `{"command":"first"}`
		secondInput  = `{"command":"second"}`
		thirdInput   = `{"command":"third"}`
		firstCall    = "toolu_first"
		secondCall   = "toolu_second"
		thirdCall    = "toolu_third"
		idLessText   = "the id-less output"
		recordedText = "the recorded output"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call b", Dialogue: "call b",
			ToolCalls: []ToolCall{{ToolUseID: firstCall, Name: "Bash", InputJSON: firstInput}}},
		Message{Ordinal: 1, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: secondCall, Name: "Bash", InputJSON: secondInput}}},
		Message{Ordinal: 2, Role: "user", Content: "the id-less output", Dialogue: "the id-less output",
			ToolResults: []ToolResult{{ContentRaw: idLessText, ContentSize: len(idLessText)}}},
		Message{Ordinal: 3, Role: "user", Content: "the recorded output", Dialogue: "the recorded output",
			ToolResults: []ToolResult{{ToolUseID: secondCall, ContentRaw: recordedText,
				ContentSize: len(recordedText)}}},
	)
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("route %s: %v", TargetOpenCode, err)
	}
	assertReserved := func(t *testing.T, arm string, carried Conversation, spec string) {
		t.Helper()
		selected, _, err := Select(carried, spec, false)
		if err != nil {
			t.Fatalf("%s: selecting %s: %v", arm, spec, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", arm, err)
		}
		written := map[string]string{}
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if strings.HasPrefix(line, "call ") {
				fields := strings.SplitN(line, " ", 4)
				if len(fields) != 4 {
					t.Fatalf("%s: a call line is not 'call <id> <name> <input>': %q", arm, line)
				}
				written[fields[3]] = fields[1]
			}
		}
		secondID, ok := written[secondInput]
		if !ok {
			t.Fatalf("%s: the written document holds no call for the occurrence the recorded result "+
				"answers", arm)
		}
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if strings.Contains(line, idLessText) {
				t.Errorf("%s: the written document holds the id-less result: the recorded result's slot "+
					"was reserved, so an inference may not take it (%q)", arm, line)
			}
		}
		found := false
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if line == "result "+secondID+" "+recordedText {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the written document does not hold the recorded result under the id the "+
				"occurrence it answers stands under: the inference displaced a recorded pairing whose "+
				"two halves the window carried", arm)
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one id-less result no call it could be "+
				"inferred onto answers", arm, got)
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: nothing was inferred", arm, got)
		}
	}
	// The window 1-3 carries A and both result records and leaves B out.
	assertReserved(t, "the recorded pairing's window", conversation, "1-3")

	// The ambiguity arm: the same window plus a third carried call no result answers. The reserved call
	// is still unanswered where the id-less result stands, so the inference has no single call to name.
	withThird := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call b", Dialogue: "call b",
			ToolCalls: []ToolCall{{ToolUseID: firstCall, Name: "Bash", InputJSON: firstInput}}},
		Message{Ordinal: 1, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: secondCall, Name: "Bash", InputJSON: secondInput}}},
		Message{Ordinal: 2, Role: "assistant", Content: "call c", Dialogue: "call c",
			ToolCalls: []ToolCall{{ToolUseID: thirdCall, Name: "Bash", InputJSON: thirdInput}}},
		Message{Ordinal: 3, Role: "user", Content: "the id-less output", Dialogue: "the id-less output",
			ToolResults: []ToolResult{{ContentRaw: idLessText, ContentSize: len(idLessText)}}},
		Message{Ordinal: 4, Role: "user", Content: "the recorded output", Dialogue: "the recorded output",
			ToolResults: []ToolResult{{ToolUseID: secondCall, ContentRaw: recordedText,
				ContentSize: len(recordedText)}}},
	)
	assertReserved(t, "the ambiguous window", withThird, "1-4")

	// The control: a recorded result whose text the target's output field cannot carry (whitespace only)
	// is no outcome on the written path, so it reserves nothing -- the id-less event is the only outcome
	// evidence that can travel and is inferred onto the call, disclosed as an inference, exactly as a
	// call whose only recorded outcome is unwritable has always been read.
	blank := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call b", Dialogue: "call b",
			ToolCalls: []ToolCall{{ToolUseID: firstCall, Name: "Bash", InputJSON: firstInput}}},
		Message{Ordinal: 1, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: secondCall, Name: "Bash", InputJSON: secondInput}}},
		Message{Ordinal: 2, Role: "user", Content: "the id-less output", Dialogue: "the id-less output",
			ToolResults: []ToolResult{{ContentRaw: idLessText, ContentSize: len(idLessText)}}},
		Message{Ordinal: 3, Role: "user", Content: "   ", Dialogue: "   ",
			ToolResults: []ToolResult{{ToolUseID: secondCall, ContentRaw: "   ", ContentSize: 3}}},
	)
	selected, _, err := Select(blank, "1-3", false)
	if err != nil {
		t.Fatalf("selecting the blank-outcome window: %v", err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the blank-outcome window: %v", err)
	}
	if !strings.Contains(string(converted.Data), idLessText) {
		t.Errorf("the written document does not hold the id-less result: a recorded result this target " +
			"cannot write reserves no slot, so the inference was the only outcome that could travel")
	}
	if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 1 {
		t.Errorf("blank-outcome generated_call_ids = %d, want the one inferred result", got)
	}
}

// TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite pins the reservation's own question: a slot is
// reserved for the outcome the *written* part is owed, so a result standing where this plan writes no
// tool evidence reserves nothing -- a record the read's own line leaves off it, a subagent's record, a
// record the writer publishes as a compaction boundary alone (writeMessage's own skips) -- and neither
// does a progress event, which is no outcome the part could state. Every arm's read is the finding's own
// order: a call, then the id-less result, then the event whose slot would be reserved. The id-less result
// is written under the call's own id in every arm, disclosed as an inference, and the event that could
// not answer it is counted; without the conditions the call would look answered and the id-less event --
// the only outcome evidence the document can carry -- would be dropped in favor of a result that never
// travels.
func TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite(t *testing.T) {
	const (
		callID     = "toolu_slot"
		callInput  = `{"command":"slot"}`
		idLessText = "the id-less output"
		owedText   = "the event whose slot would have been reserved"
	)
	conversation := func(owed Message, askParent string) Conversation {
		return testConversation(
			Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u1"},
			Message{Ordinal: 1, Role: "assistant", Content: "the call", Dialogue: "the call",
				SourceUUID: "a2", SourceParentUUID: "u1",
				ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: callInput}}},
			Message{Ordinal: 2, Role: "user", Content: "the id-less output",
				Dialogue:   "the id-less output",
				SourceUUID: "r3", SourceParentUUID: "a2",
				ToolResults: []ToolResult{{ContentRaw: idLessText, ContentSize: len(idLessText)}}},
			owed,
			Message{Ordinal: 4, Role: "user", Content: "the ask", Dialogue: "the ask",
				SourceUUID: "t5", SourceParentUUID: askParent},
		)
	}
	outcome := func(parent string) Message {
		return Message{Ordinal: 3, Role: "user", Content: owedText, Dialogue: owedText,
			SourceUUID: "s4", SourceParentUUID: parent,
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: owedText,
				ContentSize: len(owedText)}}}
	}
	progress := Message{Ordinal: 3, Role: "user", Content: owedText, Dialogue: owedText,
		SourceUUID: "s4", SourceParentUUID: "r3",
		ToolResults: []ToolResult{{ToolUseID: callID, Status: "running", ContentRaw: owedText,
			ContentSize: len(owedText)}}}
	sidechain := outcome("r3")
	sidechain.IsSidechain = true
	compaction := outcome("r3")
	compaction.IsCompactBoundary = true
	for _, arm := range []struct {
		name    string
		carried Conversation
	}{
		// The event's record hangs off the root while the conversation continues through the id-less
		// result, so the read's own line leaves it off the walk and the plan never installs it. The ask
		// record chains through the id-less result, not through the owed record.
		{"a record the read's line leaves off", conversation(outcome("u1"), "r3")},
		// A subagent's record: the plan and the writer both skip a sidechain record, though the record
		// stands on the line (the ask record chains through it).
		{"a subagent's record", conversation(sidechain, "s4")},
		// A compaction boundary record: the writer publishes the boundary alone and never its evidence.
		{"a compaction boundary record", conversation(compaction, "s4")},
		// A progress event names the call but states no outcome the tool states can spell, so the part is
		// owed nothing.
		{"a progress event", conversation(progress, "s4")},
	} {
		selected, _, err := Select(arm.carried, "0-", false)
		if err != nil {
			t.Fatalf("%s: selecting the whole read: %v", arm.name, err)
		}
		route, err := targetRouteFor(TargetOpenCode)
		if err != nil {
			t.Fatalf("route %s: %v", TargetOpenCode, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", arm.name, err)
		}
		written := ""
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if strings.HasPrefix(line, "call ") && strings.Contains(line, callInput) {
				written = strings.Fields(line)[1]
			}
		}
		if written == "" {
			t.Fatalf("%s: the written document holds no call for the occurrence", arm.name)
		}
		found := false
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if line == "result "+written+" "+idLessText {
				found = true
			}
			if strings.Contains(line, owedText) {
				t.Errorf("%s: the written document holds the event the reservation was about (%q): a "+
					"result this plan does not write is not the outcome the part is owed", arm.name, line)
			}
		}
		if !found {
			t.Errorf("%s: the written document does not hold the id-less result under the call's own "+
				"id: a result the plan does not write reserved the slot an inference could have filled",
				arm.name)
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 1 {
			t.Errorf("%s: generated_call_ids = %d, want the one inferred result", arm.name, got)
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: the id-less result was written under the "+
				"call it could mean", arm.name, got)
		}
	}
}

// TestSelectionPlansNoEvidenceFromACompactionRecord pins the twelfth round's third finding: on a document
// a selection produced, the plan installs no outcome from a record the writer publishes as a boundary
// alone (writeCompaction), so the one outcome slot a call holds is left to the recorded result the
// document *can* state. The shape is the finding's own order: a call, a writable result on a compaction
// boundary record, and a later writable result for the same call on an ordinary record. The reservation
// pass skipped the compaction record because the writer publishes no evidence from it, but the plan
// installed that record's result into the call's slot -- and the part, written from the call's own
// message, would have carried the boundary record's wording as the call's outcome although no message
// holds that state, while the later recorded result read as `repeated_tool_results` although both halves
// of the recorded pair travelled. Both passes read the same eligibility now. A run without a selection
// keeps the plan it has always made (goal 0009's D4): the call's part carries the first recorded outcome
// and the later result is the repeat it was.
func TestSelectionPlansNoEvidenceFromACompactionRecord(t *testing.T) {
	const (
		callID      = "toolu_compaction_record"
		callInput   = `{"command":"slot"}`
		boundaryOut = "the outcome the boundary record carries"
		laterOut    = "the later recorded outcome"
		boundaryTxt = "the history before the boundary, summarised"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call",
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: callInput}}},
		Message{Ordinal: 1, Role: "user", Content: boundaryTxt, Dialogue: boundaryTxt,
			IsCompactBoundary: true,
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: boundaryOut,
				ContentSize: len(boundaryOut)}}},
		Message{Ordinal: 2, Role: "user", Content: laterOut, Dialogue: laterOut,
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: laterOut, ContentSize: len(laterOut)}}},
	)
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("route %s: %v", TargetOpenCode, err)
	}
	selected, _, err := Select(conversation, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the window: %v", err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	lines := selectionCallAndResultLines(t, TargetOpenCode, converted.Data)
	if !slices.Contains(lines, "result "+callID+" "+laterOut) {
		t.Errorf("the written document states no outcome for the call (%v): the later recorded result is "+
			"the outcome the document can state, and the boundary record's event is not evidence this "+
			"document holds", lines)
	}
	for _, line := range lines {
		if strings.Contains(line, boundaryOut) {
			t.Errorf("the written document holds the boundary record's event as an outcome (%q): a record "+
				"the writer publishes as the boundary alone states no tool evidence", line)
		}
	}
	if !strings.Contains(string(converted.Data), boundaryTxt) {
		t.Error("the written document does not hold the boundary's own summary: the compaction record is " +
			"still published as the boundary it is")
	}
	for category, want := range map[string]int{
		"repeated_tool_results": 0, // the later recorded result is the outcome, not a repeat
		"unpaired_tool_results": 0, // and the boundary record's event is not counted as an outcome either
		"unpaired_tool_calls":   0, // the call's part is written with that outcome
	} {
		if got := lossCount(t, converted.Losses, category).Count; got != want {
			t.Errorf("selected %s = %d, want %d", category, got, want)
		}
	}

	// The default path: the plan installs the compaction record's result into the call's slot as it always
	// has, so the part carries that record's wording and the later recorded result is the repeat -- the
	// bytes and counts a run without a selection has always had (goal 0009's D4).
	unflagged, err := route.Convert(conversation, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the read without a selection: %v", err)
	}
	unflaggedLines := selectionCallAndResultLines(t, TargetOpenCode, unflagged.Data)
	if !slices.Contains(unflaggedLines, "result "+callID+" "+boundaryOut) {
		t.Errorf("the unflagged document states no outcome for the call (%v): the default path's plan "+
			"carries the first recorded outcome it always has", unflaggedLines)
	}
	if slices.Contains(unflaggedLines, "result "+callID+" "+laterOut) {
		t.Errorf("the unflagged document holds the later recorded result (%v): the first recorded outcome "+
			"keeps the slot on the default path", unflaggedLines)
	}
	if got := lossCount(t, unflagged.Losses, "repeated_tool_results").Count; got != 1 {
		t.Errorf("unflagged repeated_tool_results = %d, want the one extra outcome after the first "+
			"recorded one", got)
	}
}

// TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs pins the eighth round's first
// finding: a document a selection produced writes every call occurrence the read holds, and when the
// source recorded one id on two of them the later occurrence is written under an id generated from its
// own place -- otherwise the target's own reader (which pairs an output with the id) would attach the
// inferred output of the earlier occurrence to the later one. The window below is the finding's shape:
// calls A and B share an id, the recorded result answers B (the nearest call the id records before it),
// and an id-less result follows, which the read leaves no link for and the writers infer onto A. Every
// route must write the two occurrences under distinct ids, pair each result with the occurrence it
// answers, and count the substitution (colliding_call_ids); a run without a selection keeps the recorded
// ids on both occurrences and does not carry the category at all.
func TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs(t *testing.T) {
	const (
		reusedID     = "toolu_reused"
		firstOutput  = "the first result"
		lateOutput   = "the late result"
		firstCommand = `{"command":"a"}`
		secondCmd    = `{"command":"b"}`
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call a", Dialogue: "call a",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: firstCommand}}},
		Message{Ordinal: 1, Role: "assistant", Content: "call b", Dialogue: "call b",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: secondCmd}}},
		Message{Ordinal: 2, Role: "user", Content: "the first result", Dialogue: "the first result",
			ToolResults: []ToolResult{{ToolUseID: reusedID, ContentRaw: firstOutput}}},
		Message{Ordinal: 3, Role: "user", Content: "the late result", Dialogue: "the late result",
			ToolResults: []ToolResult{{ContentRaw: lateOutput}}},
	)
	selected, report, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the reused-id window: %v", err)
	}
	if report.CallsUndischarged.Count != 1 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "0" {
		t.Fatalf("calls_undischarged = %+v, want the earlier call at ordinal 0: the seam pairs the "+
			"recorded result with the nearest call the id records before it (ordinal 1), so the late "+
			"result is the earlier occurrence's inference", report.CallsUndischarged)
	}
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the reused-id window: %v", target, err)
		}
		ids := map[string]string{}
		for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
			if strings.HasPrefix(line, "call ") {
				fields := strings.SplitN(line, " ", 4)
				if len(fields) != 4 {
					t.Fatalf("%s: a call line is not 'call <id> <name> <input>': %q", target, line)
				}
				ids[fields[3]] = fields[1]
			}
		}
		firstID, secondID := ids[firstCommand], ids[secondCmd]
		if firstID == "" || secondID == "" {
			t.Fatalf("%s: the document does not hold both call occurrences (ids %v)", target, ids)
		}
		if firstID == secondID {
			t.Errorf("%s: both call occurrences are written under %q: the target pairs a call with its "+
				"output by id, so the earlier occurrence's inferred output would read back under the "+
				"later one", target, firstID)
		}
		if firstID != reusedID {
			t.Errorf("%s: the first occurrence is written under %q, want the id the source recorded on "+
				"it (%q): the id is substituted on the later occurrence alone", target, firstID, reusedID)
		}
		for _, pair := range []struct{ id, text, who string }{
			{firstID, lateOutput, "the id-less result is the earlier occurrence's own inference"},
			{secondID, firstOutput, "the recorded result answers the later occurrence"},
		} {
			found := false
			for _, line := range selectionCallAndResultLines(t, target, converted.Data) {
				if line == "result "+pair.id+" "+pair.text {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: no result %s %q in the document (%s)", target, pair.id, pair.text, pair.who)
			}
		}
		if got := lossCount(t, converted.Losses, "colliding_call_ids").Count; got != 1 {
			t.Errorf("%s: colliding_call_ids = %d, want the one occurrence written under a generated id",
				target, got)
		}
		if got := lossCount(t, converted.Losses, "generated_call_ids").Count; got != 1 {
			t.Errorf("%s: generated_call_ids = %d, want the one id-less result's inference", target, got)
		}
		if got := lossCount(t, converted.Losses, "unpaired_tool_results").Count; got != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: both results' owners are in the document",
				target, got)
		}
	}

	// The Codex arm asserts the owning call on the target's *own read-back*: the inherited reader folds
	// an output onto the message of the last call registered for the id, so the file-level attachment --
	// not only the serialized ids -- has to name the right occurrence.
	codexRoute, err := targetRouteFor(TargetCodex)
	if err != nil {
		t.Fatalf("route %s: %v", TargetCodex, err)
	}
	converted, err := codexRoute.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the reused-id window: %v", TargetCodex, err)
	}
	messages := codexReadBackMessages(t, converted.Raw.(CodexRollout))
	attached := map[string][]string{}
	for _, message := range messages {
		var calls []string
		for _, call := range message.ToolCalls {
			calls = append(calls, call.ToolUseID)
		}
		for _, result := range message.ToolResults {
			for _, id := range calls {
				if result.ToolUseID == id {
					attached[id] = append(attached[id], result.ContentRaw)
				}
			}
		}
	}
	var lateOwner, firstOwner string
	for id, outputs := range attached {
		for _, output := range outputs {
			switch output {
			case lateOutput:
				lateOwner = id
			case firstOutput:
				firstOwner = id
			}
		}
	}
	if lateOwner == firstOwner || lateOwner == "" || firstOwner == "" {
		t.Fatalf("the read-back attaches the outputs to calls %q/%q, want one each: the inferred output "+
			"belongs to the earlier occurrence and the recorded one to the later (attachments %v)",
			lateOwner, firstOwner, attached)
	}

	// The unflagged run keeps the ids the source recorded on both occurrences and its report keeps its
	// own category list: the substitution exists only where a selection writes every occurrence the read
	// holds (goal 0009's D4).
	for _, target := range Targets() {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		unflagged, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		for _, line := range selectionCallAndResultLines(t, target, unflagged.Data) {
			if strings.HasPrefix(line, "call ") && !strings.Contains(line, " "+reusedID+" ") {
				t.Errorf("%s: the unflagged conversion writes a call under %q, want the recorded id on "+
					"every occurrence", target, line)
			}
		}
		for _, loss := range unflagged.Losses {
			if loss.Category == "colliding_call_ids" {
				t.Errorf("%s: the unflagged conversion carries the reduction's own colliding_call_ids row",
					target)
			}
		}
	}
}

// TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall pins the D4 half of the colliding-id rule
// (round 9's finding 3): an id-less call is written under the plain positional spelling a run without a
// selection has always written, even where an earlier *recorded* call already holds that spelling -- the
// duplicate id the pre-0009 writer produced, byte for byte -- while a reduced document walks the id past
// the written ones (freshCallID), because an occurrence written under the shared id could not be told
// from the earlier one. The read records the three writer families' positional spellings for the
// id-less call's own place on one record, so one conversation pins each writer's prefix. OpenCode is
// absent on purpose: its positional id was already walked past taken ids before selections existed, so
// there is no pre-0009 byte to restore (goal 0009's D4).
func TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall(t *testing.T) {
	const (
		first  = `{"command":"first"}`
		second = `{"command":"second"}`
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "the recorded calls",
			Dialogue: "the recorded calls",
			ToolCalls: []ToolCall{
				{ToolUseID: "call_ccc_1_0", Name: "Bash", InputJSON: first},
				{ToolUseID: "toolu_ccc_1_0", Name: "Bash", InputJSON: first},
				{ToolUseID: "ccc1_0", Name: "Bash", InputJSON: first},
			}},
		Message{Ordinal: 1, Role: "assistant", Content: "the id-less call",
			Dialogue:  "the id-less call",
			ToolCalls: []ToolCall{{Name: "Bash", InputJSON: second}}},
	)
	// The positional spelling each writer mints for the id-less call's own place: message 1, call 0.
	positional := map[string]string{
		TargetCodex:  "call_ccc_1_0",
		TargetClaude: "toolu_ccc_1_0",
		TargetPi:     "ccc1_0",
		TargetOMP:    "ccc1_0",
	}
	callsUnder := func(t *testing.T, target string, data []byte) map[string]int {
		t.Helper()
		counts := map[string]int{}
		for _, line := range selectionCallAndResultLines(t, target, data) {
			if fields := strings.Fields(line); strings.HasPrefix(line, "call ") && len(fields) > 1 {
				counts[fields[1]]++
			}
		}
		return counts
	}
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	for _, target := range []string{TargetCodex, TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		id := positional[target]
		unflagged, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		counts := callsUnder(t, target, unflagged.Data)
		if counts[id] != 2 || counts[id+"_2"] != 0 {
			t.Errorf("%s: the unflagged conversion writes %v under %q, want the recorded call and the "+
				"plain positional spelling for the id-less call: the collision is what the writer has "+
				"always written (goal 0009's D4)", target, counts, id)
		}
		for _, loss := range unflagged.Losses {
			if loss.Category == "colliding_call_ids" {
				t.Errorf("%s: the unflagged conversion carries the reduction's own colliding_call_ids row",
					target)
			}
		}
		reduced, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the selected read: %v", target, err)
		}
		counts = callsUnder(t, target, reduced.Data)
		if counts[id] != 1 || counts[id+"_2"] != 1 {
			t.Errorf("%s: the selected conversion writes %v, want the recorded call under %q and the "+
				"id-less call walked past it to %q: two occurrences must not share one id on a reduced "+
				"document", target, counts, id, id+"_2")
		}
	}
}

// TestSelectionReportsAResultWhoseAncestryLeavesItsCall pins the ninth round's fourth finding: the owner
// assertion of the round-trip comparison has to read the result's own recorded ancestry, not the last
// call sharing its id in file order. The document is a whole-read selection whose written tree holds
// sibling assistant entries A (with the call) and B; the mutated file is the conversion's own bytes with
// exactly one field changed -- the result's parent relation, moved from A's branch to B's. Everything
// else is untouched, so the counts, the dialogue and the result's own text all still match, and only the
// result's parent walk can no longer reach its invocation. The comparison must report it; the control
// arm verifies the same file unmutated, so the flip is the mutation's. Pi, OMP (the same comparison) and
// the Claude transcript (the other tree format whose documents state owners) are each exercised.
func TestSelectionReportsAResultWhoseAncestryLeavesItsCall(t *testing.T) {
	const (
		callID      = "toolu_x"
		siblingText = "sibling b"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u0"},
		Message{Ordinal: 1, Role: "assistant", Content: "call a", Dialogue: "call a",
			SourceUUID: "a1", SourceParentUUID: "u0",
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: `{"command":"a"}`}}},
		Message{Ordinal: 2, Role: "assistant", Content: siblingText, Dialogue: siblingText,
			SourceUUID: "b2", SourceParentUUID: "u0"},
		Message{Ordinal: 3, Role: "user", Content: "the result", Dialogue: "the result",
			SourceUUID: "r3", SourceParentUUID: "a1",
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: "the output"}}},
	)
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	for _, target := range []string{TargetPi, TargetOMP, TargetClaude} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		home := t.TempDir()
		path, _, err := route.Install(context.Background(), converted, targetInstallInput{Home: home})
		if err != nil {
			t.Fatalf("%s: installing the session: %v", target, err)
		}
		verify := func(t *testing.T, arm string) StructureVerification {
			t.Helper()
			verification, err := route.ReadBack(context.Background(), route.StructureRoot(home),
				converted, fixtureConfig(t))
			if err != nil {
				t.Fatalf("%s (%s): reading the installed session back: %v", target, arm, err)
			}
			return verification
		}
		if control := verify(t, "control"); control.State != VerificationStateVerified {
			t.Fatalf("%s: the unmutated conversion verifies as %s: %s", target, control.State,
				control.Detail)
		}
		installed, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: reading the installed session: %v", target, err)
		}
		mutated, ok := mutateResultParent(t, target, installed, callID, siblingText)
		if !ok {
			t.Fatalf("%s: the installed session holds no result entry under %q whose parent can be "+
				"moved to the sibling entry", target, callID)
		}
		if err := os.WriteFile(path, mutated, 0o600); err != nil {
			t.Fatalf("%s: rewriting the installed session: %v", target, err)
		}
		moved := verify(t, "mutated")
		if moved.State == VerificationStateVerified {
			t.Errorf("%s: the comparison verified a result whose own parent walk cannot reach its "+
				"invocation: it resolved the owner by id order, not by the relation the file records",
				target)
		}
	}
}

// TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord pins the tenth round's third
// finding: the Claude writer emits one record per content block of one source record and every record of
// one source message carries that message's id, so the reader merges a dialogue record and a tool-call
// record into the run's *last* one -- whose own parent names the entry that was merged away. The owner
// check walked only the uuids the read-back holds as messages, stopped at that merged-away link and
// reported a correct conversion as missing the invocation its own installed chain reaches. The shape is
// the finding's: call A, a later assistant record carrying dialogue and call B (two entries sharing one
// message id), then A's recorded result, which pairs with A by the read's own ancestry.
func TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord(t *testing.T) {
	const (
		callA = "toolu_merged_a"
		callB = "toolu_merged_b"
		bText = "call b and its outcome"
	)
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u0"},
		Message{Ordinal: 1, Role: "assistant",
			SourceUUID: "a1", SourceParentUUID: "u0",
			ToolCalls: []ToolCall{{ToolUseID: callA, Name: "Bash", InputJSON: `{"command":"a"}`}}},
		Message{Ordinal: 2, Role: "assistant", Content: bText, Dialogue: bText,
			SourceUUID: "b2", SourceParentUUID: "a1",
			ToolCalls: []ToolCall{{ToolUseID: callB, Name: "Bash", InputJSON: `{"command":"b"}`}}},
		Message{Ordinal: 3, Role: "user", Content: "a's result", Dialogue: "a's result",
			SourceUUID: "r3", SourceParentUUID: "b2",
			ToolResults: []ToolResult{{ToolUseID: callA, ContentRaw: "the output"}}},
	)
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	route, err := targetRouteFor(TargetClaude)
	if err != nil {
		t.Fatalf("route %s: %v", TargetClaude, err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	home := t.TempDir()
	path, _, err := route.Install(context.Background(), converted, targetInstallInput{Home: home})
	if err != nil {
		t.Fatalf("installing the session: %v", err)
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	// The case is only about a merged chain if the installed file really holds one: two assistant
	// records sharing a message id, the earlier of them merged away by the reader.
	merged := 0
	ids := map[string]int{}
	for _, line := range bytes.Split(bytes.TrimSpace(installed), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decoding an installed record: %v", err)
		}
		if record["type"] != "assistant" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		id, _ := message["id"].(string)
		if id == "" {
			continue
		}
		ids[id]++
	}
	for _, count := range ids {
		if count > 1 {
			merged++
		}
	}
	if merged != 1 {
		t.Fatalf("the installed session holds %d assistant message id(s) carried by several records, "+
			"want the one the finding's shape writes (the dialogue and the tool call of one source "+
			"record)", merged)
	}
	verification, err := route.ReadBack(context.Background(), route.StructureRoot(home), converted,
		fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Errorf("the conversion verifies as %s: %s; the result's own installed ancestry reaches the "+
			"invocation it states, through the entry the reader merged away",
			verification.State, verification.Detail)
	}
}

// mutateResultParent moves one installed result entry's recorded parent from the branch that holds its
// call to the sibling entry named by its text, changing no other field: the file keeps every record and
// every word, so only the result's own recorded relation moves. It answers false when the file holds no
// such result entry or no such sibling entry -- the test then fails rather than asserting on a mutation
// that never happened. The line shapes are the two tree formats': a Pi-family entry carries `id` /
// `parentId`, a Claude record `uuid` / `parentUuid`, and both carry the message (with its role or its
// content blocks) under `message`.
func mutateResultParent(t *testing.T, target string, installed []byte, callID, siblingText string) ([]byte, bool) {
	t.Helper()
	fields := struct{ idField, parentField string }{}
	switch target {
	case TargetPi, TargetOMP:
		fields = struct{ idField, parentField string }{"id", "parentId"}
	case TargetClaude:
		fields = struct{ idField, parentField string }{"uuid", "parentUuid"}
	default:
		t.Fatalf("mutateResultParent: no parent relation is known for target %s", target)
	}
	decode := func(line []byte) (map[string]any, bool) {
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			return nil, false
		}
		return record, true
	}
	lines := bytes.Split(installed, []byte("\n"))
	siblingID := ""
	resultLine := -1
	for index, line := range lines {
		record, ok := decode(line)
		if !ok {
			continue
		}
		id, _ := record[fields.idField].(string)
		if id == "" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		if messageHoldsText(message, siblingText) {
			siblingID = id
		}
		if messageHoldsToolResult(message, callID) {
			resultLine = index
		}
	}
	if siblingID == "" || resultLine < 0 {
		return nil, false
	}
	record, ok := decode(lines[resultLine])
	if !ok {
		return nil, false
	}
	record[fields.parentField] = siblingID
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("re-encoding the mutated result entry: %v", err)
	}
	lines[resultLine] = encoded
	return bytes.Join(lines, []byte("\n")), true
}

// messageHoldsText reports whether one entry's message content carries a text block equal to text.
func messageHoldsText(message map[string]any, text string) bool {
	if message == nil {
		return false
	}
	blocks, _ := message["content"].([]any)
	for _, block := range blocks {
		blockMap, _ := block.(map[string]any)
		if blockMap == nil {
			continue
		}
		if blockText, _ := blockMap["text"].(string); blockText == text {
			return true
		}
	}
	return false
}

// messageHoldsToolResult reports whether one entry's message content carries a tool result under callID.
func messageHoldsToolResult(message map[string]any, callID string) bool {
	if message == nil {
		return false
	}
	if role, _ := message["role"].(string); role == "toolResult" {
		if id, _ := message["toolCallId"].(string); id == callID {
			return true
		}
	}
	blocks, _ := message["content"].([]any)
	for _, block := range blocks {
		blockMap, _ := block.(map[string]any)
		if blockMap == nil {
			continue
		}
		if blockMap["type"] == "tool_result" {
			if id, _ := blockMap["tool_use_id"].(string); id == callID {
				return true
			}
		}
	}
	return false
}

// codexReadBackMessages installs one Codex rollout into a throw-away home and answers the messages the
// inherited reader reads from the installed file: the target's own read-back, which is what the round
// trip and the owning-call assertions have to be about.
func codexReadBackMessages(t *testing.T, rollout CodexRollout) []Message {
	t.Helper()
	home := t.TempDir()
	if _, _, err := InstallCodexRollout(rollout, home); err != nil {
		t.Fatalf("installing the rollout: %v", err)
	}
	sessions := filepath.Join(home, codexSessionsSubdir)
	cfg := fixtureConfig(t)
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs))
	for name, roots := range cfg.AgentDirs {
		dirs[name] = roots
	}
	dirs[parser.AgentCodex] = []string{sessions}
	cfg.AgentDirs = dirs
	cfg.DisabledAgents = nil
	readBack, err := NewSourceReader(cfg).Read(context.Background(), Ref{
		Provider: string(parser.AgentCodex),
		ID:       filepath.Join(sessions, filepath.FromSlash(rollout.Directory), rollout.FileName),
	})
	if err != nil {
		t.Fatalf("reading the installed rollout back: %v", err)
	}
	return readBack.Messages
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

// TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine pins the leaf guard against the window the
// identity-keyed lookup cannot see: an identity-less record whose recorded parent is a record the read's
// own line leaves off it, carried as the window's *last* record. The direct ancestor lookup chains it to
// that parent's entry -- before the guarded retained-ancestor and fallback places run -- so the tree
// walks `2 → 1 → 0` and the target's loader would resume a branch the source's conversation left. The
// guard answers by the record's own *position* (sourceLineHolds, the read's walk membership kept through
// the selection) and through the entry's actual written ancestry, so this window is refused by name
// before anything is installed. The controls keep the refusal about the line and not about the shape: a
// window whose last record is on the line -- the identity-less record chained to an on-line parent, or
// the same abandoned-branch record once the window keeps the line's own leaf -- converts.
func TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine(t *testing.T) {
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the abandoned turn", Dialogue: "the abandoned turn",
			SourceUUID: "b2", SourceParentUUID: "u1"},
		// The record the read answered no identity for, chained to the abandoned turn: the read's line --
		// the walk from ordinal 3 through u1 -- reaches neither record.
		Message{Ordinal: 2, Role: "assistant", Content: "the abandoned child",
			Dialogue: "the abandoned child", SourceUUID: "", SourceParentUUID: "b2"},
		Message{Ordinal: 3, Role: "assistant", Content: "the active line", Dialogue: "the active line",
			SourceUUID: "c3", SourceParentUUID: "u1"},
	)
	for _, testCase := range []struct {
		spec  string
		holds []bool
	}{
		// The window keeps the abandoned branch's record and its identity-less child: both are off the
		// line the read's last record answers, and the child is the window's leaf.
		{"0-2", []bool{true, false, false}},
		// The identity-less child alone with the root: its retained ancestor is the root, so the chain
		// runs the on-line way -- but the child itself is still off the read's line and would be the
		// resumed session's beginning.
		{"0,2", []bool{true, false}},
	} {
		selected, _, err := Select(conversation, testCase.spec, false)
		if err != nil {
			t.Fatalf("selecting %q: %v", testCase.spec, err)
		}
		if got := selected.sourceLineHolds; !reflect.DeepEqual(got, testCase.holds) {
			t.Fatalf("selecting %q: sourceLineHolds = %v, want the read's own membership for the kept "+
				"records %v: ordinal 2 is off the line the last record answers", testCase.spec, got,
				testCase.holds)
		}
		for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
			route, err := targetRouteFor(target)
			if err != nil {
				t.Fatalf("route %s: %v", target, err)
			}
			_, err = route.Convert(selected, selectionProbeOptions())
			if !errors.Is(err, ErrUnrepresentableLeaf) {
				t.Errorf("%s: converting %q: err = %v, want ErrUnrepresentableLeaf: the window's last "+
					"record is the identity-less one, chained to the entry of the abandoned turn", target,
					testCase.spec, err)
				continue
			}
			for _, want := range []string{"the record at ordinal 2", "the read answered no identity for it"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s: the refusal for %q (%v) does not name %q", target, testCase.spec, err, want)
				}
			}
		}
	}

	// The line keeps the last word: ordinal 2 is left out and the on-line leaf is the window's own end,
	// with the abandoned branch's record present as recorded history off the walk.
	selected, _, err := Select(conversation, "0,1,3", false)
	if err != nil {
		t.Fatalf("selecting 0,1,3: %v", err)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting 0,1,3 (the line's own leaf, the abandoned record as history): %v",
				target, err)
		}
	}

	// A window that retains neither record's ancestry: the identity-less off-line child and the line's
	// own leaf, and nothing else. The child has no entry to stand on, so the node written before the
	// leaf's entry is the child's own, and the fallback's revival check has to refuse the shape by the
	// record's own *position* -- the child carries no identity for the check to key on. The line targets
	// omit the child and count it: the position-based membership is the only answer its identity-less
	// record has, and writing it into a one-line log makes the abandoned branch model-visible history.
	noAncestry, _, err := Select(conversation, "2-3", false)
	if err != nil {
		t.Fatalf("selecting 2-3: %v", err)
	}
	if got := noAncestry.sourceLineHolds; !reflect.DeepEqual(got, []bool{false, true}) {
		t.Fatalf("selecting 2-3: sourceLineHolds = %v, want [false true]: the identity-less child stands "+
			"off the line the read's last record answers, and the leaf's own ancestry is not in the window",
			got)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		_, err = route.Convert(noAncestry, selectionProbeOptions())
		if !errors.Is(err, ErrUnrepresentableLeaf) {
			t.Errorf("%s: converting %q: err = %v, want ErrUnrepresentableLeaf: the window's leaf would "+
				"be chained to the entry of the identity-less record the read's line leaves off it",
				target, "2-3", err)
			continue
		}
		for _, want := range []string{"c3 (ordinal 3)",
			"the record at ordinal 2 (the read answered no identity for it)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: the refusal for %q (%v) does not name %q", target, "2-3", err, want)
			}
		}
	}
	for _, target := range []string{TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(noAncestry, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting %q: %v", target, "2-3", err)
		}
		if data := string(converted.Data); strings.Contains(data, "the abandoned child") {
			t.Errorf("%s: the written document holds the identity-less record the read's line leaves off "+
				"it: written into this target's log it becomes model-visible history", target)
		} else if !strings.Contains(data, "the active line") {
			t.Errorf("%s: the written document does not hold the line's own leaf", target)
		}
		if count := lossCount(t, converted.Losses, "inactive_branch_records").Count; count != 1 {
			t.Errorf("%s: inactive_branch_records = %d, want the one record the read's own walk does not "+
				"reach", target, count)
		}
	}

	// The identity-less record with an explicit parent that *is* on the line: the read's own order
	// reaches it (the last record names no parent, so the walk steps to the record before it), and the
	// guard answers on the line. Refusing this shape would be a refusal about the identity and not about
	// the line.
	onLine := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the word", Dialogue: "the word",
			SourceUUID: "", SourceParentUUID: "u1"},
		Message{Ordinal: 2, Role: "assistant", Content: "the last word", Dialogue: "the last word"},
	)
	if got := sourceLineMembershipForTest(onLine.Messages); !reflect.DeepEqual(got, []bool{true, true, true}) {
		t.Fatalf("the on-line control's read membership = %v, want every record on the line: the walk "+
			"steps from the parentless last record to the one before it", got)
	}
	selected, _, err = Select(onLine, "0-1", false)
	if err != nil {
		t.Fatalf("selecting 0-1 over the on-line identity-less record: %v", err)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(selected, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting a window whose identity-less leaf stands on the read's line: %v",
				target, err)
		}
	}
}

// sourceLineMembershipForTest answers the read's own walk membership for a document no selection reduced,
// through the same computation Select carries (inactiveSourceRecords): the test's own reader for the
// control above, so the expected membership is the rule's answer and not a hand-counted one.
func sourceLineMembershipForTest(messages []Message) []bool {
	_, onLine, _ := inactiveSourceRecords(messages, "")
	return onLine
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

	// The other side of the same revival check: an *off-line* record whose own parent the read does not
	// hold has no relation to resolve, so `parentPlace` takes the entry written before it -- here an
	// off-line entry -- and that place is recorded history, not a revival: the record itself stands off
	// the read's line, so it is not a record the line holds chained under an abandoned entry. The window
	// converts, the branch travels in the file as the window carried it, and the walk from the file's
	// last entry stays the read's own line.
	stranded := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "the branch parent", Dialogue: "the branch parent",
			SourceUUID: "b2", SourceParentUUID: "u1"},
		// The record whose own parent this read does not hold: off the line (the walk from ordinal 3
		// through u1 reaches neither record), with no relation the writer can place.
		Message{Ordinal: 2, Role: "assistant", Content: "the stranded record",
			Dialogue: "the stranded record", SourceUUID: "c3", SourceParentUUID: "not-in-this-read"},
		Message{Ordinal: 3, Role: "assistant", Content: "the active line", Dialogue: "the active line",
			SourceUUID: "d4", SourceParentUUID: "u1"},
	)
	strandedWindow, _, err := Select(stranded, "0-3", false)
	if err != nil {
		t.Fatalf("selecting the whole stranded read: %v", err)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		if _, err := route.Convert(strandedWindow, selectionProbeOptions()); err != nil {
			t.Errorf("%s: converting a window whose off-line record falls back to an off-line entry: %v "+
				"want no refusal: the fallback's revival check is about a record the read's line holds",
				target, err)
		}
	}
	route, err = targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	strandedSession, err := route.Convert(strandedWindow, selectionProbeOptions())
	if err != nil {
		t.Fatalf("%s: converting the stranded window: %v", TargetPi, err)
	}
	strandedLines := decodePiSession(t, strandedSession.Data)
	strandedTexts := piEntryTexts(t, strandedLines)
	strandedWalk := map[string]bool{}
	for id := range piWalkIDs(t, strandedLines) {
		strandedWalk[strandedTexts[id]] = true
	}
	for _, want := range []string{"start", "the active line"} {
		if !strandedWalk[want] {
			t.Errorf("the walk from the stranded file's last entry does not hold %q", want)
		}
	}
	for _, unwanted := range []string{"the branch parent", "the stranded record"} {
		if strandedWalk[unwanted] {
			t.Errorf("the walk from the stranded file's last entry holds %q, a record the read's line "+
				"leaves off it", unwanted)
		}
		if !strings.Contains(string(strandedSession.Data), unwanted) {
			t.Errorf("the written document does not hold %q at all: it is recorded history the window "+
				"carried, and the rule is about the walk, not the file", unwanted)
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

	// The Codex route writes the same unit on a reduced document: one function_call per invocation and the
	// one recorded output, with the projected rows not written counted -- writing both rows would put two
	// invocations with the same id and one output in a file the report says holds one call. The unflagged
	// conversion keeps the two rows the writer has always written (goal 0009's D4: the default path's
	// bytes do not move), so the coalescing belongs to the reduction and this case pins both halves.
	countPayloads := func(t *testing.T, rollout CodexRollout) map[string]int {
		t.Helper()
		counts := map[string]int{}
		for _, line := range decodeRollout(t, rollout.Data) {
			counts[payloadField(t, line, "type")]++
		}
		return counts
	}
	selectedRollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the whole selected read to codex: %v", err)
	}
	if counts := countPayloads(t, selectedRollout); counts["function_call"] != 1 ||
		counts["function_call_output"] != 1 {
		t.Errorf("the selected codex conversion writes %v, want one function_call and one "+
			"function_call_output for the one invocation", counts)
	}
	if selectedRollout.Carried.ToolCalls != 1 || selectedRollout.Carried.ToolResults != 1 {
		t.Errorf("the selected codex conversion carried = %+v, want the one invocation and its outcome",
			selectedRollout.Carried)
	}
	if loss := lossCount(t, selectedRollout.Losses, "coalesced_tool_call_projections"); loss.Count != 1 {
		t.Errorf("coalesced_tool_call_projections = %+v, want the one projected row the selected "+
			"conversion coalesces into the call it writes", loss)
	}
	unflaggedRollout, err := ConvertToCodexRollout(patch, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the whole read to codex without a selection: %v", err)
	}
	if counts := countPayloads(t, unflaggedRollout); counts["function_call"] != 2 ||
		counts["function_call_output"] != 1 {
		t.Errorf("the unflagged codex conversion writes %v, want the two rows and the one output the "+
			"writer has always written: the default path is byte-identical to the same run before "+
			"selections existed", counts)
	}
	for _, loss := range unflaggedRollout.Losses {
		if loss.Category == "coalesced_tool_call_projections" {
			t.Errorf("the unflagged codex report names %+v: no row is coalesced on the default path, "+
				"and its report is byte-identical to the same run before selections existed", loss)
		}
	}

	// The read-back is what the structural checker compares, and the inherited reader expands every
	// written `apply_patch` record into one call row per file it names: the selected file's one
	// invocation reads back as two rows and the unflagged file's two records as four, so a comparison of
	// raw read-back rows reports the selected document as missing a call although the file holds the one
	// invocation it wrote (round 9's finding 2 -- the finding's cited test stopped at the written
	// envelopes and never ran the read-back). The check compares invocation units -- the file's own call
	// records -- for a document a selection produced, so the selected conversion verifies; the negative
	// arm holds the fold to the file (a document claiming one invocation against a file holding two
	// actual call envelopes is still reported), and round 10's finding 2 holds it to the *document*: the
	// same check run unflagged keeps the rows-against-items verdict its revisions before the fold had.
	installAndVerify := func(t *testing.T, arm string, rollout CodexRollout) (StructureVerification, []byte) {
		t.Helper()
		home := t.TempDir()
		if _, _, err := InstallCodexRollout(rollout, home); err != nil {
			t.Fatalf("%s: installing the rollout: %v", arm, err)
		}
		verification, err := VerifyCodexRolloutStructure(context.Background(),
			filepath.Join(home, codexSessionsSubdir), rollout, fixtureConfig(t))
		if err != nil {
			t.Fatalf("%s: verifying the rollout: %v", arm, err)
		}
		installed, err := os.ReadFile(filepath.Join(home, codexSessionsSubdir,
			filepath.FromSlash(rollout.Directory), rollout.FileName))
		if err != nil {
			t.Fatalf("%s: reading the installed rollout: %v", arm, err)
		}
		return verification, installed
	}
	selectedVerification, _ := installAndVerify(t, "selected", selectedRollout)
	if selectedVerification.State != VerificationStateVerified {
		t.Errorf("the selected codex conversion verifies as %s: %s; the file holds the one invocation "+
			"the document wrote, however many rows the reader's per-file projection shows",
			selectedVerification.State, selectedVerification.Detail)
	}
	selectedRows := 0
	for _, message := range codexReadBackMessages(t, selectedRollout) {
		selectedRows += len(message.ToolCalls)
	}
	if selectedRows != 2 {
		t.Errorf("the selected file reads back as %d call row(s), want the two the reader's per-file "+
			"projection of one `apply_patch` record shows", selectedRows)
	}
	unflaggedVerification, unflaggedInstalled := installAndVerify(t, "unflagged", unflaggedRollout)
	// The fold is the reduction's (codexRoundTripReader(rollout.SelectionReduced)), and this arm is the
	// half of goal 0009's D4 that keeps it so: the unflagged comparison reads the reader's per-file rows
	// as calls and reports the file's extra rows, exactly as every revision before the fold did. The
	// verdict, the warning that restates it and the CLI exit status that follows from it are the default
	// path's own; the round-10 finding 2 measured them moving when the fold ran unflagged, and the
	// before/after probe (docs/evidence/0009-d4-probe.txt) is the record of both revisions.
	if unflaggedVerification.State != VerificationStateNotVerified {
		t.Errorf("the unflagged codex conversion verifies as %s, want the not-verified verdict every "+
			"revision before the fold answered: the fold is the reduction's, and a run without a "+
			"selection keeps the comparison, the verdict and the exit status it has always had",
			unflaggedVerification.State)
	}
	if !strings.Contains(unflaggedVerification.Detail, "is in the file but was not written") {
		t.Errorf("the unflagged detail = %q, want the reader's per-file rows reported as calls the "+
			"document did not write", unflaggedVerification.Detail)
	}
	if roundTrip := compareCodexRoundTrip(unflaggedRollout,
		Conversation{Messages: codexReadBackMessages(t, unflaggedRollout)},
		unflaggedInstalled); len(roundTrip.differences) == 0 {
		t.Error("the unflagged comparison folded the reader's per-file rows into the file's records: " +
			"the invocation-unit fold belongs to a document a selection produced")
	}
	if roundTrip := compareCodexRoundTrip(selectedRollout,
		Conversation{Messages: codexReadBackMessages(t, unflaggedRollout)},
		unflaggedInstalled); len(roundTrip.differences) == 0 {
		t.Error("the comparison accepted a document claiming one invocation against a file holding two " +
			"actual `function_call` envelopes")
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

// TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters pins the writers' half of the progress
// rule on a reduced document: a recorded result whose status says the call is still running is not an
// answer -- the selection's own count reads the event that way -- so it must not leave the outstanding
// set a reduced writer's id-less inference reads either, and the writers must not present it as the
// call's outcome. The read records call A, a `running` event of A, call B, and an id-less result on the
// same path: both calls are outstanding where the result stands, so the result is omitted and counted,
// and the progress event itself is omitted and named on every route -- Codex's function_call_output,
// Claude's tool_result block, the Pi family's toolResult message and OpenCode's tool part all carry an
// outcome or nothing, so a written event would read back as the call's *answer* (round 10's finding 5;
// the default arm below is the half of goal 0009's D4 that keeps writing it on the unflagged path).
func TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters(t *testing.T) {
	const progressText = "Still working"
	const outcomeText = "the late output"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call a", Dialogue: "call a",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_wait", Name: "Bash", InputJSON: `{"command":"a"}`}}},
		Message{Ordinal: 2, Role: "user", Content: "the check", Dialogue: "the check",
			SourceUUID: "r3", SourceParentUUID: "a2",
			ToolResults: []ToolResult{{ToolUseID: "toolu_wait", Status: "running",
				ContentRaw: progressText, ContentSize: len(progressText)}}},
		Message{Ordinal: 3, Role: "assistant", Content: "call b", Dialogue: "call b",
			SourceUUID: "b4", SourceParentUUID: "r3",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_next", Name: "Bash", InputJSON: `{"command":"b"}`}}},
		Message{Ordinal: 4, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "c5", SourceParentUUID: "b4",
			ToolResults: []ToolResult{{ContentRaw: outcomeText, ContentSize: len(outcomeText)}}},
	)
	selected, report, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	if report.CallsUndischarged.Count != 2 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "1,3" {
		t.Fatalf("calls_undischarged = %+v, want the calls at ordinals 1 and 3: the running event is not "+
			"an outcome", report.CallsUndischarged)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the progress window: %v", target, err)
		}
		data := string(converted.Data)
		if strings.Contains(data, outcomeText) {
			t.Errorf("%s: the written document holds the id-less result: both calls on its path were "+
				"outstanding (A's only recorded event is progress), so the inference had no single call "+
				"to name", target)
		}
		if strings.Contains(data, progressText) {
			t.Errorf("%s: the written document holds the recorded progress event as the call's output: "+
				"the format carries an outcome or nothing, so the words would read back as the call's "+
				"answer", target)
		}
		// The event is omitted and named on every route's own vocabulary: the target's outcome shape
		// cannot hold a call that is still running, so the text does not travel and the count says so.
		if count := lossCount(t, converted.Losses, "nonterminal_tool_events").Count; count != 1 {
			t.Errorf("%s: nonterminal_tool_events = %d, want the one recorded progress event the "+
				"format cannot carry as progress", target, count)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result no single call answers",
				target, count)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: the inference picked no call", target, count)
		}
	}
	// The default path keeps the ledger's own answer: the running event clears the call's id there, so
	// the one outstanding id is B's and the result is written under it -- the writer's answer before
	// selections existed, which a run without the flag must keep (goal 0009's D4). The progress event
	// itself is written there too, which is the same answer those writers have always had and the other
	// half of the round-10 finding 5's scoping: the omission above is the reduction's. OpenCode is left
	// out of this arm: its plan already refuses a progress event as an outcome, so its ledger holds A and
	// B and its default path omits the result too.
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), outcomeText) {
			t.Errorf("%s: the unflagged conversion drops the id-less result: the ledger's id-keyed set "+
				"is the default path's answer", target)
		}
		if !strings.Contains(string(converted.Data), progressText) {
			t.Errorf("%s: the unflagged conversion drops the recorded progress event: a run without a "+
				"selection keeps writing it, which is the answer its writer has always had", target)
		}
		// The category is the reduction's own, as the colliding-id and coalesced-projection rows are: a
		// zero row may not be added to the default path's report (goal 0009's D4, whole report included).
		for _, loss := range converted.Losses {
			if loss.Category == "nonterminal_tool_events" {
				t.Errorf("%s: the unflagged conversion carries the reduction's own "+
					"nonterminal_tool_events row", target)
			}
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
// recorded command: one half of what a case walks a written tree for.
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
	callIDs := map[string]string{}
	resultOnWalk := ""
	for _, record := range claudeWalkRecords(t, decodeTranscript(t, transcript)) {
		for _, block := range recordBlocks(record) {
			switch block["type"] {
			case "tool_use":
				input, ok := block["input"].(map[string]any)
				if !ok {
					continue
				}
				if command, ok := input["command"].(string); ok {
					id, _ := block["id"].(string)
					callIDs[command] = id
				}
			case "tool_result":
				if block["content"] == resultBText {
					resultOnWalk, _ = block["tool_use_id"].(string)
				}
			}
		}
	}
	if callIDs["one"] == "" || callIDs["two"] == "" || resultOnWalk == "" {
		t.Fatalf("the walk from the transcript's last record holds the calls %v and the result %q: "+
			"want both calls of the reused id and the output the result carries", callIDs, resultOnWalk)
	}
	if callIDs["one"] != reusedID {
		t.Errorf("the first occurrence stands under %q, want the id the source recorded on it (%q)",
			callIDs["one"], reusedID)
	}
	if callIDs["two"] == callIDs["one"] {
		t.Errorf("both occurrences of the reused id stand under %q: the target pairs a tool result "+
			"with its tool_use by id, so the output would read back under the wrong invocation",
			callIDs["two"])
	}
	if count := lossCount(t, transcript.Losses, "colliding_call_ids").Count; count != 1 {
		t.Errorf("colliding_call_ids = %d, want the one occurrence written under an id generated from "+
			"its own place", count)
	}
	if resultOnWalk != callIDs["two"] {
		t.Errorf("the walk holds the result under %q, want the id of the call it answers (%q): a resumed "+
			"session reads the output only under the invocation it answers", resultOnWalk, callIDs["two"])
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
	callOnWalk := false
	for _, line := range lines[1:] {
		if onPath[line.ID] && strings.Contains(string(line.Message), `{"command":"two"}`) {
			callOnWalk = true
		}
	}
	if !callOnWalk {
		t.Errorf("the walk from the Pi file's last entry holds no record of the call the result answers:\n%s",
			session.Data)
	}
	// The result's own entry names the call it answers, and that call is the occurrence the read paired
	// it with -- written, when a second occurrence of the id stands in the window, under an id generated
	// from its own place (colliding_call_ids): the file pairs the two by id.
	callID, resultCallID := "", ""
	for _, line := range selectionCallAndResultLines(t, TargetPi, session.Data) {
		switch {
		case strings.HasPrefix(line, "call ") && strings.Contains(line, `{"command":"two"}`):
			callID = strings.Fields(line)[1]
		case strings.HasPrefix(line, "result ") && strings.HasSuffix(line, " "+resultBText):
			resultCallID = strings.Fields(line)[1]
		}
	}
	if callID == "" || resultCallID != callID {
		t.Errorf("the Pi session writes the result under %q and the call it answers under %q, want one "+
			"id: a resumed session reads the output only under the invocation it answers",
			resultCallID, callID)
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
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Read", InputJSON: `{"command":"one"}`}}},
		// The inactive sibling reusing the id -- with a different tool name, so a result that named the
		// wrong occurrence would be caught by its name too: its parent is the root, so it is off the
		// read's line (4 → c4 → a2 → u1), and it is the occurrence the reduced document's own order would
		// choose.
		Message{Ordinal: 2, Role: "assistant", Content: "the sibling", Dialogue: "the sibling",
			SourceUUID: "b3", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Write", InputJSON: `{"command":"two"}`}}},
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

	// The result record states the name of the occurrence the read paired the result with, not the name
	// of the later sibling that reused the id: the family's name map is keyed by id and holds the last
	// write, so only an occurrence-keyed answer can name the call the resumed walk actually reaches.
	for _, target := range []string{TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		named := false
		for _, line := range decodePiSession(t, converted.Data) {
			if line.Message == nil || !strings.Contains(string(line.Message), resultText) {
				continue
			}
			named = true
			var message struct {
				ToolCallID string `json:"toolCallId"`
				ToolName   string `json:"toolName"`
			}
			if err := json.Unmarshal(line.Message, &message); err != nil {
				t.Fatalf("%s: decoding the result entry: %v", target, err)
			}
			if message.ToolCallID != reusedID || message.ToolName != "Read" {
				t.Errorf("%s: the result entry = %+v, want the occurrence the read paired with the result "+
					"(%s/Read), not the later sibling's Write", target, message, reusedID)
			}
		}
		if !named {
			t.Errorf("%s: the written session holds no result entry with %q", target, resultText)
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

	// The same read without a selection: the written-ancestry disposition belongs to the reduction (goal
	// 0009's D4 -- a run without `--select` is byte-identical to the same run before selections existed),
	// so the default path keeps the acceptance it always had: some written call shares the id, wherever
	// in the file it stands, and the result answers it. The rule the window above is held to must not act
	// here, or the unflagged run's bytes and unpaired_tool_results would move with it.
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), branchResult) {
			t.Errorf("%s: the unflagged conversion does not hold the result: the default path's "+
				"acceptance -- a written call sharing the id -- is what a run without a selection keeps",
				target)
		}
		if converted.Carried.ToolResults != 1 || converted.Carried.ToolCalls != 1 {
			t.Errorf("%s: unflagged carried = %+v, want the call and the result, as before the selection "+
				"existed", target, converted.Carried)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unflagged unpaired_tool_results = %d, want 0", target, count)
		}
	}
}

// TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath pins the inference a reduced document's
// writer makes for a result the source recorded with no id: it may name only a call that stands on the
// written walk of the result's record -- the sole outstanding call in the line it wrote. The read: a root
// at 0, sibling calls at 1 and 2, and an id-less result at 3 whose recorded parent is the call at 2.
// Selecting 0,1,3 leaves the call at 1 as the only call outstanding in the file, and a tree target writes
// record 1 as history off the walk -- but the result's nearest retained ancestor is the root, so the call
// at 1 does not stand on the walk the result's entry hangs off: inferring the result onto it would write
// an output whose invocation the resumed context cannot see, which `generated_call_ids` does not disclose
// (the caveat names an inference, not a placement), so the result is omitted and counted instead. The
// positive arm -- the same window with the call whose record is on the result's own ancestry -- shows the
// inference still fires where it can be placed, and the entry states that call's id.
func TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath(t *testing.T) {
	const answerText = "the answer"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call one", Dialogue: "call one",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_one", Name: "Bash", InputJSON: `{"command":"one"}`}}},
		Message{Ordinal: 2, Role: "assistant", Content: "call two", Dialogue: "call two",
			SourceUUID: "b3", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_two", Name: "Bash", InputJSON: `{"command":"two"}`}}},
		// The id-less result: its recorded parent is the call at ordinal 2, so the window's own ancestry
		// answer is the root, and the call at 1 -- the only one left outstanding -- is a sibling of that
		// place.
		Message{Ordinal: 3, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "c4", SourceParentUUID: "b3",
			ToolResults: []ToolResult{{ContentRaw: answerText, ContentSize: len(answerText)}}},
	)

	offPath, _, err := Select(conversation, "0,1,3", false)
	if err != nil {
		t.Fatalf("selecting 0,1,3: %v", err)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(offPath, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if strings.Contains(string(converted.Data), answerText) {
			t.Errorf("%s: the written document holds the id-less result, inferred onto the call at "+
				"ordinal 1, which does not stand on the walk the result's entry hangs off", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result no eligible call answers",
				target, count)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: the inference picked no call, so it minted no "+
				"pairing either", target, count)
		}
	}
	transcript, err := ConvertToClaudeTranscript(offPath, ClaudeTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the window to claude: %v", err)
	}
	if transcript.Carried.ToolCalls != 1 || !strings.Contains(string(transcript.Data), "toolu_one") {
		t.Errorf("the window's call at ordinal 1 is not in the transcript: it is recorded history the "+
			"selection carried, and the rule is about the walk, not the file: %+v", transcript.Carried)
	}

	// The call whose record *is* the result's retained ancestor: the sole outstanding call the walk
	// reaches, so the inference names it and the entry states its id.
	onPath, _, err := Select(conversation, "0,2,3", false)
	if err != nil {
		t.Fatalf("selecting 0,2,3: %v", err)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(onPath, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), answerText) {
			t.Errorf("%s: the written document does not hold the id-less result the sole eligible call "+
				"answers", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unpaired_tool_results = %d, want 0: the inference had an eligible call", target,
				count)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 1 {
			t.Errorf("%s: generated_call_ids = %d, want the one pairing the inference minted", target, count)
		}
	}
	transcript, err = ConvertToClaudeTranscript(onPath, ClaudeTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the eligible window to claude: %v", err)
	}
	pairedID := ""
	for _, record := range decodeTranscript(t, transcript) {
		for _, block := range recordBlocks(record) {
			if block["type"] == "tool_result" {
				pairedID, _ = block["tool_use_id"].(string)
			}
		}
	}
	if pairedID != "toolu_two" {
		t.Errorf("the transcript's tool_result block answers %q, want toolu_two: the eligible call's own "+
			"id", pairedID)
	}
	piRoute, err := targetRouteFor(TargetPi)
	if err != nil {
		t.Fatalf("route %s: %v", TargetPi, err)
	}
	session, err := piRoute.Convert(onPath, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the eligible window to pi: %v", err)
	}
	pairedID = ""
	for _, line := range decodePiSession(t, session.Data) {
		if line.Message == nil || !strings.Contains(string(line.Message), answerText) {
			continue
		}
		var message struct {
			ToolCallID string `json:"toolCallId"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("decoding the Pi result entry: %v", err)
		}
		pairedID = message.ToolCallID
	}
	if pairedID != "toolu_two" {
		t.Errorf("the Pi result entry answers %q, want toolu_two: the eligible call's own id", pairedID)
	}
	rollout, err := ConvertToCodexRollout(onPath, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the eligible window to codex: %v", err)
	}
	pairedID = ""
	for _, line := range decodeRollout(t, rollout.Data) {
		if payloadField(t, line, "type") == "function_call_output" {
			pairedID = payloadField(t, line, "call_id")
		}
	}
	if pairedID != "toolu_two" {
		t.Errorf("the rollout's function_call_output answers %q, want toolu_two: the eligible call's own "+
			"id", pairedID)
	}

	// An outcome the read recorded on a *sibling* branch answers its call there and must not clear it for
	// the id-less result on this path: A's recorded outcome stands off the walk the id-less result's entry
	// hangs off, and B is outstanding beside A where the result stands. The inference sees two calls and
	// names neither -- a completion set keyed by id over the whole file would read A as answered
	// everywhere and infer the result onto B, a pairing the read never recorded, on the very path where
	// both calls were left unanswered.
	const siblingAnswer = "the sibling outcome"
	sibling := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call A", Dialogue: "call A",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_a", Name: "Bash", InputJSON: `{"command":"a"}`}}},
		// A's recorded outcome, on a branch of its own: the nearest call the id records before it is A,
		// so the read pairs it with A, and the selection carries it as A's counterpart when the window
		// names A.
		Message{Ordinal: 2, Role: "user", SourceUUID: "s3", SourceParentUUID: "u1",
			ToolResults: []ToolResult{{ToolUseID: "toolu_a", ContentRaw: siblingAnswer,
				ContentSize: len(siblingAnswer)}}},
		Message{Ordinal: 3, Role: "assistant", Content: "the middle", Dialogue: "the middle",
			SourceUUID: "m4", SourceParentUUID: "a2"},
		Message{Ordinal: 4, Role: "assistant", Content: "call B", Dialogue: "call B",
			SourceUUID: "b5", SourceParentUUID: "m4",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_b", Name: "Bash", InputJSON: `{"command":"b"}`}}},
		Message{Ordinal: 5, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "c6", SourceParentUUID: "b5",
			ToolResults: []ToolResult{{ContentRaw: answerText, ContentSize: len(answerText)}}},
	)
	selected, report, err := Select(sibling, "1,3-5", false)
	if err != nil {
		t.Fatalf("selecting 1,3-5: %v", err)
	}
	if report.NecessitiesAdded.Count != 1 {
		t.Fatalf("selecting 1,3-5: necessities = %+v, want the sibling outcome carried as A's counterpart",
			report.NecessitiesAdded)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the sibling-outcome window: %v", target, err)
		}
		if strings.Contains(string(converted.Data), answerText) {
			t.Errorf("%s: the written document holds the id-less result: both calls on its own path were "+
				"outstanding (A was answered only on the sibling branch), so the inference had no call to "+
				"name", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result no eligible call answers",
				target, count)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: the inference picked no call, so it minted no "+
				"pairing either", target, count)
		}
	}
	// The tree writers keep the sibling outcome in the file as recorded history off the walk -- it is the
	// counterpart the selection carried -- while the line writers count it among the records the read's
	// line leaves off and omit it with them. Either way the id-less result is omitted above, which is
	// what tells the two dispositions apart: an inference reading the file-wide set wrote the result
	// under B while this record was in the file.
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the sibling-outcome window: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), siblingAnswer) {
			t.Errorf("%s: the written document does not hold the sibling outcome the selection carried as "+
				"A's counterpart", target)
		}
	}
	// The default path keeps the ledger's own question -- the one call still outstanding anywhere in the
	// file -- so the same read without a selection infers the result onto B (goal 0009's D4: the default
	// path's answer is the one it always had). The contrast is the finding's point: the reduction's
	// inference reads completion per occurrence and per path.
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(sibling, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), answerText) {
			t.Errorf("%s: the unflagged conversion drops the id-less result: the ledger's file-wide set "+
				"is the default path's answer", target)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 1 {
			t.Errorf("%s: unflagged generated_call_ids = %d, want the one pairing the ledger's answer "+
				"mints", target, count)
		}
	}

	// Two occurrences the source recorded under one id, both unanswered, and an id-less result after them
	// on the same path: the eligible set holds two call occurrences, and a result the read left no link
	// for could belong to either. Reading the set by id would see one outstanding call -- Codex's and
	// OpenCode's pending sets are keyed by id, Pi's inference rejected a second eligible occurrence only
	// when its id differed, and the family's id-keyed maps hold the last write -- and the result would be
	// written under the later occurrence, a pairing the read never recorded. The reduced inference counts
	// occurrences on the written path, so the result is omitted and counted; the same read without a
	// selection keeps the ledger's own id-keyed answer (goal 0009's D4).
	const reusedAnswer = "the ambiguous output"
	reused := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "root", Dialogue: "root", SourceUUID: "u1"},
		Message{Ordinal: 1, Role: "assistant", Content: "call one", Dialogue: "call one",
			SourceUUID: "a2", SourceParentUUID: "u1",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_shared", Name: "Bash", InputJSON: `{"command":"one"}`}}},
		Message{Ordinal: 2, Role: "assistant", Content: "the middle", Dialogue: "the middle",
			SourceUUID: "m3", SourceParentUUID: "a2"},
		Message{Ordinal: 3, Role: "assistant", Content: "call two", Dialogue: "call two",
			SourceUUID: "b4", SourceParentUUID: "m3",
			ToolCalls: []ToolCall{{ToolUseID: "toolu_shared", Name: "Bash", InputJSON: `{"command":"two"}`}}},
		Message{Ordinal: 4, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "c5", SourceParentUUID: "b4",
			ToolResults: []ToolResult{{ContentRaw: reusedAnswer, ContentSize: len(reusedAnswer)}}},
	)
	selected, report, err = Select(reused, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole reused-id read: %v", err)
	}
	if report.CallsUndischarged.Count != 2 ||
		strings.Join(report.CallsUndischarged.Regions, ",") != "1,3" {
		t.Fatalf("calls_undischarged = %+v, want the two calls the source left unanswered: the id the two "+
			"occurrences share is carried by two distinct records", report.CallsUndischarged)
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the reused-id window: %v", target, err)
		}
		if strings.Contains(string(converted.Data), reusedAnswer) {
			t.Errorf("%s: the written document holds the id-less result: two call occurrences sharing the "+
				"id were outstanding, so the result could belong to either and the inference had no call "+
				"to name", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result no single call answers",
				target, count)
		}
		if count := lossCount(t, converted.Losses, "generated_call_ids").Count; count != 0 {
			t.Errorf("%s: generated_call_ids = %d, want 0: the inference picked no call, so it minted no "+
				"pairing either", target, count)
		}
	}
	for _, target := range []string{TargetClaude, TargetPi, TargetOMP, TargetCodex, TargetOpenCode} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(reused, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the reused-id read without a selection: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), reusedAnswer) {
			t.Errorf("%s: the unflagged conversion drops the id-less result: the ledger's id-keyed set "+
				"is the default path's answer", target)
		}
	}
}

// TestUnflaggedFlatConversionKeepsTheIDAcceptance pins the same scoping on the two line targets (and the
// shape that measures it): the read's own pairing names an off-line occurrence for the result -- so a
// reduced document omits the result and counts it -- while a call occurrence sharing the id stands on the
// line the unflagged walk writes. The shape is the one the line walk can produce and the readers can
// record: record 6 (the occurrence the id's file order names) hangs off record 5 while the walk from the
// read's last record steps 7 → 5 → 3 → 2 → 0, and record 3's recorded parent is an identity this read
// does not hold, so the walk takes the record before it exactly as the writers' own line rule does. The
// unflagged conversion keeps the acceptance the flat writers have always had -- some written call stands
// under the id -- so the result travels (goal 0009's D4: the default path is byte-identical to the same
// run before selections existed); the selection-scoped disposition is what a reduced document asks.
func TestUnflaggedFlatConversionKeepsTheIDAcceptance(t *testing.T) {
	const callID = "toolu_offline_occurrence"
	const resultText = "the read output"
	conversation := testConversation(
		Message{Ordinal: 0, Role: "user", Content: "start", Dialogue: "start", SourceUUID: "n0"},
		Message{Ordinal: 1, Role: "assistant", Content: "skipped one", Dialogue: "skipped one",
			SourceUUID: "n1", SourceParentUUID: "n0"},
		Message{Ordinal: 2, Role: "assistant", Content: "the call", Dialogue: "the call",
			SourceUUID: "n2", SourceParentUUID: "n0",
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: `{"command":"one"}`}}},
		// The record whose recorded parent this read does not hold: the line walk steps to the record
		// before it, which is what keeps records 1 and 4 off the line.
		Message{Ordinal: 3, Role: "assistant", Content: "the step", Dialogue: "the step",
			SourceUUID: "n3", SourceParentUUID: "not-in-this-read"},
		Message{Ordinal: 4, Role: "assistant", Content: "skipped two", Dialogue: "skipped two",
			SourceUUID: "n4", SourceParentUUID: "n3"},
		Message{Ordinal: 5, Role: "assistant", Content: "the mid", Dialogue: "the mid",
			SourceUUID: "n5", SourceParentUUID: "n3"},
		// The occurrence the id's file order names for the result: off the walk 7 → 5 → 3 → 2 → 0.
		Message{Ordinal: 6, Role: "assistant", Content: "the later call", Dialogue: "the later call",
			SourceUUID: "n6", SourceParentUUID: "n5",
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: `{"command":"two"}`}}},
		Message{Ordinal: 7, Role: "user", Content: "the ask", Dialogue: "the ask",
			SourceUUID: "n7", SourceParentUUID: "n5",
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: resultText,
				ContentSize: len(resultText)}}},
	)
	for _, target := range []string{TargetCodex, TargetOpenCode, TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", target, err)
		}
		if !strings.Contains(string(converted.Data), resultText) {
			t.Errorf("%s: the unflagged conversion drops the result: the read's pairing names an "+
				"off-line occurrence, but the default path's acceptance is the id a written call shares",
				target)
		}
		if converted.Carried.ToolResults != 1 {
			t.Errorf("%s: unflagged carried = %+v, want the result carried as before the selection "+
				"existed", target, converted.Carried)
		}
		// The line targets write the one call the walk reaches; the trees also write the off-line
		// occurrence as recorded history, and neither writer reads the pairing to decide the result.
		wantCalls := 2
		if target == TargetCodex || target == TargetOpenCode {
			wantCalls = 1
		}
		if converted.Carried.ToolCalls != wantCalls {
			t.Errorf("%s: unflagged carried = %+v, want %d call(s)", target, converted.Carried, wantCalls)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 0 {
			t.Errorf("%s: unflagged unpaired_tool_results = %d, want 0", target, count)
		}
	}
	selected, _, err := Select(conversation, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	for _, target := range []string{TargetCodex, TargetOpenCode, TargetClaude, TargetPi, TargetOMP} {
		route, err := targetRouteFor(target)
		if err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the selection: %v", target, err)
		}
		if strings.Contains(string(converted.Data), resultText) {
			t.Errorf("%s: the reduced document writes the result under the id of an occurrence the "+
				"read's pairing does not name", target)
		}
		if count := lossCount(t, converted.Losses, "unpaired_tool_results").Count; count != 1 {
			t.Errorf("%s: unpaired_tool_results = %d, want the one result whose paired occurrence the "+
				"reduction does not carry on its line", target, count)
		}
	}
}

// TestSelectRefusesADocumentASelectionProduced pins the guard against reducing a reduction. The Pi
// history fixture's ordinals 2-4 stand off the line its last record answers, so a selection that carries
// 0-4 carries them as recorded history off the read's line, and the writers keep counting them inactive.
// A second selection over that reduced document would re-measure the line from its own last record --
// ordinal 4, now the walk's end -- and hand the writers a document whose branch is active again, which is
// exactly the revival the carried line answers exist to prevent. The refusal is by name and is answered
// before any specification is read; the document it was handed is left as it was, and the read itself was
// never rewritten, so the caller applies the specification to the read.
func TestSelectRefusesADocumentASelectionProduced(t *testing.T) {
	conversation, err := NewSourceReader(fixtureConfig(t)).Read(context.Background(), piHistoryFixtureRef)
	if err != nil {
		t.Fatalf("reading the history fixture: %v", err)
	}
	selected, _, err := Select(conversation, "0-4", false)
	if err != nil {
		t.Fatalf("selecting 0-4: %v", err)
	}
	want := []bool{false, false, true, true, true}
	if !reflect.DeepEqual(selected.sourceLineInactive, want) {
		t.Fatalf("the first selection's line flags = %v, want %v: the fixture's ordinals 2-4 stand off "+
			"the line its last record answers", selected.sourceLineInactive, want)
	}
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the window: %v", err)
	}
	if count := lossCount(t, rollout.Losses, "inactive_branch_records").Count; count != 3 {
		t.Fatalf("the window's inactive_branch_records = %d, want the three records the read's line "+
			"leaves off it: the reduced document still carries the read's own line", count)
	}
	for _, strict := range []bool{false, true} {
		if _, _, err := Select(selected, "0-4", strict); !errors.Is(err, ErrSelectionReduced) {
			t.Errorf("selecting the reduction (strict=%v) = %v, want ErrSelectionReduced", strict, err)
		}
	}
	if _, _, err := Select(selected, "", false); !errors.Is(err, ErrSelectionReduced) {
		t.Errorf("selecting the reduction with an empty specification = %v, want ErrSelectionReduced: "+
			"the input's own refusal is answered before any specification is read", err)
	}
	if len(selected.Messages) != 5 || !reflect.DeepEqual(selected.sourceLineInactive, want) {
		t.Errorf("the refused call changed the document it was handed: %d message(s), line flags %v",
			len(selected.Messages), selected.sourceLineInactive)
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

// TestSelectionRefusesAWhitespaceOnlyAPIRequest pins the eleventh round's first finding: the selection
// flag's *absence* is the literal empty string with no presence bit, and nothing else. A value that holds
// whitespace alone names nothing however the caller spelled it, and an embedding caller that sets
// ContinueOptions.Selection to " \t " without the presence bit has asked for a selection -- reading it as
// the flag's absence would install the whole history, the one thing the request was written to prevent.
// The conversion path is checked at both layers: Continue's own check refuses before anything is read,
// and the step Continue hands the value to (continueWith, asked directly because Continue's check cannot
// let such a value through) refuses it rather than publishing the whole read. The positive control shows
// the assertion is about the request and not about an empty target home: a well-formed selection installs
// a session.
func TestSelectionRefusesAWhitespaceOnlyAPIRequest(t *testing.T) {
	for _, selection := range []string{" ", "\t", " \t\n", "\u00a0"} {
		for _, supplied := range []bool{false, true} {
			err := CheckSelectionOptions(selection, false, supplied)
			if !errors.Is(err, ErrBadSelection) {
				t.Errorf("CheckSelectionOptions(%q, supplied=%v): err = %v, want ErrBadSelection",
					selection, supplied, err)
			} else if !strings.Contains(err.Error(), "the specification is empty") {
				t.Errorf("CheckSelectionOptions(%q, supplied=%v): %v does not name the empty specification",
					selection, supplied, err)
			}
		}
	}
	options := func(selection string, supplied bool) ContinueOptions {
		return ContinueOptions{
			Ref:               claudeSelectionFixtureRef,
			Target:            TargetCodex,
			Config:            fixtureConfig(t),
			TargetHome:        t.TempDir(),
			TargetBinary:      missingTargetBinary,
			Selection:         selection,
			SelectionSupplied: supplied,
		}
	}
	// Continue's own check: the refusal is the specification's, nothing was read and nothing installed.
	home := t.TempDir()
	opts := options(" \t ", false)
	opts.TargetHome = home
	if _, err := Continue(context.Background(), opts); !errors.Is(err, ErrBadSelection) {
		t.Errorf("Continue with a whitespace-only selection: err = %v, want ErrBadSelection", err)
	}
	if files := hashTree(t, home); len(files) != 0 {
		t.Errorf("the target home holds %v after a refused request, want nothing installed", files)
	}
	// Continue hands the value to continueWith, which takes a non-empty value as a request: it must reach
	// Select's own refusal rather than skipping the selection and installing the whole read.
	route, err := targetRouteFor(TargetCodex)
	if err != nil {
		t.Fatalf("route %s: %v", TargetCodex, err)
	}
	innerHome := t.TempDir()
	innerOpts := options(" \t ", false)
	innerOpts.TargetHome = innerHome
	if _, err := continueWith(context.Background(), innerOpts, route, VerifyNone); !errors.Is(err, ErrBadSelection) {
		t.Errorf("continueWith with a whitespace-only selection: err = %v, want ErrBadSelection: a value "+
			"that names nothing is a refused request, never the flag's absence", err)
	}
	if files := hashTree(t, innerHome); len(files) != 0 {
		t.Errorf("the target home holds %v after the inner refusal, want nothing installed", files)
	}
	// The positive control: a well-formed request converts and installs.
	controlHome := t.TempDir()
	control := options("0", true)
	control.TargetHome = controlHome
	if _, err := Continue(context.Background(), control); err != nil {
		t.Fatalf("Continue with a well-formed single-ordinal selection: %v", err)
	}
	if files := hashTree(t, controlHome); len(files) == 0 {
		t.Error("the well-formed selection installed nothing: the refusal assertions would pass on a run " +
			"that never installs at all")
	}
}

// TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult pins the eleventh round's third
// finding. The OpenCode planner's output slot is the recorded outcome the part can *state*: a result whose
// text the output field holds nothing of (no text, whitespace only) or whose status word the tool states
// cannot spell is counted under the category the writer counts the same result under (toolPart's own
// guards), and it yields the slot to a later recorded outcome the part can write. The window is the
// finding's shape: a call, an id-less terminal output, and then two results naming the call, the first
// unwritable and the second writable. The later result reserves the call's slot from the id-less inference
// (round 10's rule: an inference may not fill the slot the recorded result is owed), so the id-less event
// is omitted and counted -- and without the yield the unwritable event standing before the writable one
// consumed the slot itself: the writable result read as a repeat, the part was dropped at write time, and
// the document stated no outcome although one was representable. The default path keeps its long-standing
// answer (the ledger's id-keyed inference writes the id-less event under the call) -- a run without a
// selection is byte-identical to the same run before selections existed (goal 0009's D4).
func TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult(t *testing.T) {
	const (
		callID     = "toolu_unwritable_slot"
		callInput  = `{"command":"slot"}`
		idLessText = "the id-less output"
		laterText  = "the later recorded output"
	)
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("route %s: %v", TargetOpenCode, err)
	}
	for _, arm := range []struct {
		name string
		held ToolResult
		// category is the loss category the held event is counted under, exactly as the writer counts the
		// same result when the call holds it.
		category string
	}{
		{"an empty-text result", ToolResult{ToolUseID: callID, ContentRaw: ""}, "empty_tool_results"},
		{"a whitespace-only result", ToolResult{ToolUseID: callID, ContentRaw: "   ", ContentSize: 3},
			"whitespace_only_tool_results"},
		{"a status the tool states cannot spell", ToolResult{ToolUseID: callID, Status: "cancelled",
			ContentRaw: "the cancelled output", ContentSize: len("the cancelled output")}, "tool_result_statuses"},
	} {
		conversation := testConversation(
			Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call",
				ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: callInput}}},
			Message{Ordinal: 1, Role: "user", Content: idLessText, Dialogue: idLessText,
				ToolResults: []ToolResult{{ContentRaw: idLessText, ContentSize: len(idLessText)}}},
			Message{Ordinal: 2, Role: "user", Content: "held", Dialogue: "held",
				ToolResults: []ToolResult{arm.held}},
			Message{Ordinal: 3, Role: "user", Content: laterText, Dialogue: laterText,
				ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: laterText,
					ContentSize: len(laterText)}}},
		)
		selected, _, err := Select(conversation, "0-3", false)
		if err != nil {
			t.Fatalf("%s: selecting the window: %v", arm.name, err)
		}
		converted, err := route.Convert(selected, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the window: %v", arm.name, err)
		}
		written := false
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, converted.Data) {
			if line == "result "+callID+" "+laterText {
				written = true
			}
			if strings.Contains(line, idLessText) {
				t.Errorf("%s: the written document holds the id-less result (%q): the recorded result's "+
					"slot was reserved, so an inference may not take it", arm.name, line)
			}
		}
		if !written {
			t.Errorf("%s: the written document holds no outcome for the call: the unwritable event that "+
				"stood before the writable one consumed the slot the writable record is owed", arm.name)
		}
		if got := lossCount(t, converted.Losses, arm.category).Count; got != 1 {
			t.Errorf("%s: %s = %d, want the one displaced event counted under the category the writer "+
				"counts the same result under", arm.name, arm.category, got)
		}
		for category, want := range map[string]int{
			"unpaired_tool_results":   1, // the id-less event, its call's slot reserved
			"unpaired_tool_calls":     0, // the call's part is written
			"repeated_tool_results":   0, // the writable result is the one carried
			"generated_call_ids":      0, // nothing was inferred
			"nonterminal_tool_events": 0,
		} {
			if got := lossCount(t, converted.Losses, category).Count; got != want {
				t.Errorf("%s: %s = %d, want %d", arm.name, category, got, want)
			}
		}

		// The default path keeps the ledger's answer: the id-less event is inferred onto the call and
		// written, both later results are counted repeats, and the writable one is not carried -- the
		// bytes a run without a selection has always written (goal 0009's D4).
		unflagged, err := route.Convert(conversation, selectionProbeOptions())
		if err != nil {
			t.Fatalf("%s: converting the read without a selection: %v", arm.name, err)
		}
		idLessWritten, laterWritten := false, false
		for _, line := range selectionCallAndResultLines(t, TargetOpenCode, unflagged.Data) {
			if strings.Contains(line, idLessText) {
				idLessWritten = true
			}
			if strings.Contains(line, laterText) {
				laterWritten = true
			}
		}
		if !idLessWritten {
			t.Errorf("%s: the unflagged document holds no outcome for the call: the ledger's id-keyed "+
				"outstanding set is the default path's answer and the id-less event is what it infers",
				arm.name)
		}
		if laterWritten {
			t.Errorf("%s: the unflagged document holds the later recorded result: the default path's bytes "+
				"are what a run without a selection has always written", arm.name)
		}
		if got := lossCount(t, unflagged.Losses, "repeated_tool_results").Count; got != 2 {
			t.Errorf("%s: unflagged repeated_tool_results = %d, want the two results after the first "+
				"recorded outcome", arm.name, got)
		}
	}

	// The yield's scope, on its own shape (no id-less event in the way): the same conversation with and
	// without a selection separates the reduced answer from the default path's. Unselected, the first
	// recorded outcome keeps the slot even though the part cannot state it -- the blank result is the one
	// carried (counted, its part dropped at write time) and the later recorded result is a repeat, the
	// bytes and counts a run without a selection has always had (goal 0009's D4); yielding there would
	// change them. A mutation making the yield unconditional is what this arm exists to refuse.
	const laterOnly = "the later recorded result"
	scope := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call",
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: callInput}}},
		Message{Ordinal: 1, Role: "user", Content: "held", Dialogue: "held",
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: "   ", ContentSize: 3}}},
		Message{Ordinal: 2, Role: "user", Content: laterOnly, Dialogue: laterOnly,
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: laterOnly,
				ContentSize: len(laterOnly)}}},
	)
	selectedScope, _, err := Select(scope, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the two-result scope window: %v", err)
	}
	selectedBytes, err := route.Convert(selectedScope, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the two-result scope window: %v", err)
	}
	wroteLater := false
	for _, line := range selectionCallAndResultLines(t, TargetOpenCode, selectedBytes.Data) {
		if line == "result "+callID+" "+laterOnly {
			wroteLater = true
		}
	}
	if !wroteLater {
		t.Error("the selected window holds no outcome for the call: the unwritable held result must yield " +
			"the slot to the later recorded one")
	}
	unflaggedScope, err := route.Convert(scope, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the two-result scope read without a selection: %v", err)
	}
	for _, line := range selectionCallAndResultLines(t, TargetOpenCode, unflaggedScope.Data) {
		if line == "result "+callID+" "+laterOnly {
			t.Errorf("the unflagged document holds the later recorded result (%q): the first recorded "+
				"outcome keeps the slot on the default path, and its bytes are what a run without a "+
				"selection has always written", line)
		}
	}
	if got := lossCount(t, unflaggedScope.Losses, "repeated_tool_results").Count; got != 1 {
		t.Errorf("unflagged two-result repeated_tool_results = %d, want the one extra outcome after the "+
			"first recorded one", got)
	}
	if got := lossCount(t, unflaggedScope.Losses, "whitespace_only_tool_results").Count; got != 1 {
		t.Errorf("unflagged two-result whitespace_only_tool_results = %d, want the held result counted "+
			"as the writer counts it", got)
	}

	// The completion instant travels with the result that won the slot, not with the event it displaced:
	// the held event records no instant of its own (its `completed` would fall back to the containing
	// record's, counted), and the later writable result records one -- the part must state *that* instant
	// and the report must count no fallback, because a flag left standing from the displaced event would
	// name a fallback for a result whose own recorded instant was written (round 12's fourth finding).
	at := func(second int) time.Time { return time.Date(2026, 5, 8, 8, 0, second, 0, time.UTC) }
	timed := testConversation(
		Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call", Timestamp: at(0),
			ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: callInput}}},
		Message{Ordinal: 1, Role: "user", Content: "held", Dialogue: "held", Timestamp: at(1),
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: "   ", ContentSize: 3}}},
		Message{Ordinal: 2, Role: "user", Content: laterOnly, Dialogue: laterOnly, Timestamp: at(2),
			ToolResults: []ToolResult{{ToolUseID: callID, ContentRaw: laterOnly,
				ContentSize: len(laterOnly), Timestamp: at(3)}}},
	)
	selectedTimed, _, err := Select(timed, "0-2", false)
	if err != nil {
		t.Fatalf("selecting the timestamped window: %v", err)
	}
	timedSession, err := route.Convert(selectedTimed, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the timestamped window: %v", err)
	}
	times := openCodeToolPartTimes(t, timedSession.Data)
	if len(times) != 1 || times[0] == nil || times[0].Created != at(0).UnixMilli() ||
		times[0].Completed != at(3).UnixMilli() {
		t.Errorf("the displaced slot's part times = %+v, want created %d (the call's record) and "+
			"completed %d (the later result's own recorded instant)", times, at(0).UnixMilli(),
			at(3).UnixMilli())
	}
	if got := lossCount(t, timedSession.Losses, "result_completion_fallbacks").Count; got != 0 {
		t.Errorf("result_completion_fallbacks = %d, want 0: the written outcome's own recorded instant "+
			"was used, so no fallback belongs to it -- a flag raised for the event it displaced is not "+
			"the written result's answer", got)
	}
	if got := lossCount(t, timedSession.Losses, "whitespace_only_tool_results").Count; got != 1 {
		t.Errorf("whitespace_only_tool_results = %d, want the displaced event still counted as the writer "+
			"counts the same result", got)
	}
}

// TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress pins the eleventh round's fourth
// finding, and the twelfth round's second finding's scope. The finding: on a document a selection
// produced, the OpenCode progress branch marks a call as one whose recorded events stand in for its
// archive summary only where the event *records that call's own id*. An id-less progress event is paired
// by this writer's own inference (pairedCallEntry), a link the source never made, and an inferred link
// may not suppress the call's own recorded summary evidence: the summary would then be read as a view of
// an event that may belong to another call, the part is left with no outcome and the call is counted
// unanswered although the read holds one. The id-recorded control arm keeps the marking: the read's own
// event for that call says the call is still running, so the summary is that event's view and the writers
// refuse to present it as an outcome.
//
// The rule is the reduction's alone. The default path keeps the writer's long-standing answer, measured
// on the base revision for the twelfth round's second finding: an id-less event names the sole call still
// awaiting a result there (callNamedByResult), so the call is marked, the summary is *not* written as an
// outcome and the call is counted unanswered -- the bytes, counts and whole report a run without a
// selection has always had (goal 0009's D4). The unflagged arm below pins that answer, and it is what a
// mutation scoping the reduction's rule no further than the reduction leaves untouched.
func TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress(t *testing.T) {
	const (
		callID       = "toolu_summary_progress"
		summary      = "the call's own recorded outcome"
		progressText = "Still working"
	)
	conversation := func(progress ToolResult) Conversation {
		return testConversation(
			Message{Ordinal: 0, Role: "assistant", Content: "call", Dialogue: "call",
				ToolCalls: []ToolCall{{ToolUseID: callID, Name: "Bash", InputJSON: `{"command":"a"}`,
					ResultContent: summary, ResultContentSize: len(summary)}}},
			Message{Ordinal: 1, Role: "user", Content: progressText, Dialogue: progressText,
				ToolResults: []ToolResult{progress}},
		)
	}
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("route %s: %v", TargetOpenCode, err)
	}
	// outcomeLines answers the tool parts one written document states: the evidence lines a call and its
	// outcome travel as, so a summary in a message's dialogue cannot stand in for a written outcome.
	outcomeLines := func(t *testing.T, data []byte) []string {
		t.Helper()
		return selectionCallAndResultLines(t, TargetOpenCode, data)
	}
	// The finding's arm: the read's only event for the call is id-less progress, so the summary is the
	// call's outcome evidence and is written.
	selected, _, err := Select(conversation(ToolResult{Status: "running", ContentRaw: progressText,
		ContentSize: len(progressText)}), "0-1", false)
	if err != nil {
		t.Fatalf("selecting the summary with an id-less progress event: %v", err)
	}
	converted, err := route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the summary window: %v", err)
	}
	found := false
	for _, line := range outcomeLines(t, converted.Data) {
		if line == "result "+callID+" "+summary {
			found = true
		}
	}
	if !found {
		t.Errorf("the selected document does not hold the call's own recorded summary: the id-less "+
			"progress event suppressed it through a link the source never recorded (%v)",
			outcomeLines(t, converted.Data))
	}
	if got := lossCount(t, converted.Losses, "unpaired_tool_calls").Count; got != 0 {
		t.Errorf("unpaired_tool_calls = %d, want 0: the read holds an outcome for the call", got)
	}
	if got := lossCount(t, converted.Losses, "nonterminal_tool_events").Count; got != 1 {
		t.Errorf("nonterminal_tool_events = %d, want the one progress event counted as what it is", got)
	}

	// The id-recorded control: the read's own event for the call says it is still running, so the summary
	// is that event's view and is not written as an outcome.
	selected, _, err = Select(conversation(ToolResult{ToolUseID: callID, Status: "running",
		ContentRaw: progressText, ContentSize: len(progressText)}), "0-1", false)
	if err != nil {
		t.Fatalf("selecting the summary with an id-recorded progress event: %v", err)
	}
	converted, err = route.Convert(selected, selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the id-recorded progress window: %v", err)
	}
	if lines := outcomeLines(t, converted.Data); len(lines) != 0 {
		t.Errorf("the selected document states tool evidence (%v): the read's own event for the call is "+
			"progress, and the summary is that event's view rather than an outcome recorded beside it", lines)
	}
	if got := lossCount(t, converted.Losses, "unpaired_tool_calls").Count; got != 1 {
		t.Errorf("unpaired_tool_calls = %d, want the call counted: its only recorded event is progress", got)
	}

	// The default path keeps the writer's long-standing rule, which the twelfth round's second finding
	// measured on the base revision: the call the event names -- the id it recorded, or the sole call
	// still awaiting a result -- is marked as one whose recorded events stand in for its archive summary.
	// An id-less event names that sole call there, so the summary is *not* written as an outcome and the
	// call is counted unanswered on the written path: the bytes and counts a run without a selection has
	// always had (goal 0009's D4), which scoping the id-only rule to the reduction is what preserves.
	unflagged, err := route.Convert(conversation(ToolResult{Status: "running", ContentRaw: progressText,
		ContentSize: len(progressText)}), selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the read without a selection: %v", err)
	}
	if lines := outcomeLines(t, unflagged.Data); len(lines) != 0 {
		t.Errorf("the unflagged document states tool evidence (%v): the id-less progress event names the "+
			"sole call awaiting a result on the default path, so its summary is that event's view and is "+
			"not written as an outcome", lines)
	}
	if got := lossCount(t, unflagged.Losses, "unpaired_tool_calls").Count; got != 1 {
		t.Errorf("unflagged unpaired_tool_calls = %d, want the call counted: the default path marks it "+
			"through the event's own pairing and the part is written with no outcome", got)
	}
	if got := lossCount(t, unflagged.Losses, "nonterminal_tool_events").Count; got != 1 {
		t.Errorf("unflagged nonterminal_tool_events = %d, want the one progress event counted as what it is",
			got)
	}

	// The id-recorded control on the default path: the read's own event for the call says it is still
	// running there too, so the summary is that event's view and the writers refuse to present it --
	// the same answer whether or not a selection ran, and the arm that made the default path's answer
	// visible when the reduction's id-only rule was scoped to reduced documents.
	unflaggedControl, err := route.Convert(conversation(ToolResult{ToolUseID: callID, Status: "running",
		ContentRaw: progressText, ContentSize: len(progressText)}), selectionProbeOptions())
	if err != nil {
		t.Fatalf("converting the read with an id-recorded progress event without a selection: %v", err)
	}
	if lines := outcomeLines(t, unflaggedControl.Data); len(lines) != 0 {
		t.Errorf("the unflagged document states tool evidence (%v): the read's own event for the call is "+
			"progress, and the summary is that event's view rather than an outcome recorded beside it", lines)
	}
	if got := lossCount(t, unflaggedControl.Losses, "unpaired_tool_calls").Count; got != 1 {
		t.Errorf("unflagged id-recorded unpaired_tool_calls = %d, want the call counted: its only recorded "+
			"event is progress", got)
	}
}
