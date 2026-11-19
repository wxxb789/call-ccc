package ccc

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrBadSelection reports a selection specification this build refuses: one that is not a list of
// ordinal values and regions, one that names an ordinal the read's own ordinals do not hold, or one that
// would carry no record at all. A bad selection is refused, never repaired: a spec silently read as
// something else would write a target session the caller did not ask for, and the caller's next move is
// to write a different spec, not to inspect the source.
var ErrBadSelection = errors.New("the selection cannot be honoured")

// ErrStrictSelection reports a selection that --strict-selection refuses: it would carry a recorded tool
// call without its recorded result, or a recorded result without its recorded call, and the caller asked
// for the refusal instead of the counterpart being carried. Nothing is written before the refusal, so the
// source session and the target's data directory are as they were.
var ErrStrictSelection = errors.New(
	"the selection separates a recorded tool call from its recorded result")

// OrdinalSpace is the space a selection spec is written against and a refusal names: the ordinal values
// the read itself supplied, which are the source's own numbering as the read preserved it rather than a
// position in the read (a reader that assigns a serial counter produces 0..n-1, while one that preserves
// a source field -- Shelley's `sequence_id` -- produces the source's own values, so 0 may name no record
// and the last record may sit above n-1). Contiguous is true when the values are min..max with no gap;
// a nonzero Min or a gap is what makes a spec written by position select the wrong records, which is why
// the report states the space rather than leaving the caller to assume one.
type OrdinalSpace struct {
	Min        int  `json:"min"`
	Max        int  `json:"max"`
	Records    int  `json:"records"`
	Contiguous bool `json:"contiguous"`
}

// SelectionClass is one class of the selection's own accounting: how many pieces of evidence it holds
// and the ordinal values they sit at, spelled as regions with the specification's own grammar (a
// contiguous run as `a-b`, a lone value as `n`). The count is the class's own unit -- records for the
// carried, excluded and necessity classes, calls for the undischarged one -- and the regions name the
// ordinals of the records that unit sits on, so a message carrying two undischarged calls appears once.
type SelectionClass struct {
	Count   int      `json:"count"`
	Regions []string `json:"regions"`
}

// SelectionNecessity is one record a selection carried that the spec did not name: the ordinal of the
// record added, and the ordinal of the selected record whose recorded call or result pulled it in. The
// pair is what makes the addition inspectable -- a caller can see that `--select 2` carried 3 because the
// call recorded at 2 and the result recorded at 3 are one structure.
type SelectionNecessity struct {
	Ordinal  int `json:"ordinal"`
	PulledBy int `json:"pulled_by"`
}

// SelectionReport is what one selection did to the read, in the classes the goal names: the space the
// spec was written against, the records it selected that the read holds, the records it left behind, the
// counterparts carried because a recorded call and its recorded result are one structure, and the calls
// the source itself left undischarged. Included plus necessities are what the target writer is handed;
// included plus necessities plus excluded are exactly the records the read returned, so a caller can
// reconcile the three counts with the source block's own message count.
type SelectionReport struct {
	Spec   string       `json:"spec"`
	Strict bool         `json:"strict"`
	Space  OrdinalSpace `json:"ordinal_space"`
	// Included, Excluded, NecessitiesAdded and CallsUndischarged are the four classes; the regions in the
	// first two name ordinal values the read supplied, never the target's own post-write numbering.
	Included          SelectionClass       `json:"included"`
	Excluded          SelectionClass       `json:"excluded"`
	NecessitiesAdded  SelectionNecessities `json:"necessities_added"`
	CallsUndischarged SelectionClass       `json:"calls_undischarged"`
}

// SelectionNecessities is the necessity class: the count of records added, and each addition with the
// ordinal that pulled it in.
type SelectionNecessities struct {
	Count int                  `json:"count"`
	Added []SelectionNecessity `json:"added"`
}

// CheckSelectionOptions answers whether a selection request is one this build can honour, in the words
// the CLI refuses a command line with as well: a specification has to be the grammar parseSelectionSpec
// reads, and asking for the strict arm without naming a selection is a request about nothing. The
// refusals that need the read -- an ordinal the read's own space does not hold, a spec that selects
// nothing -- are Select's, because only a read can answer them; Continue calls both, so a caller
// embedding the core and one driving the CLI are told the same thing about the same request.
func CheckSelectionOptions(selection string, strict bool) error {
	spec := strings.TrimSpace(selection)
	if spec == "" {
		if strict {
			return fmt.Errorf(
				"%w: --strict-selection refuses a selection that would separate a recorded call from its "+
					"recorded result, so it needs --select to name the selection it is about", ErrBadSelection)
		}
		return nil
	}
	_, err := parseSelectionSpec(spec)
	return err
}

// Select applies one selection specification to a read conversation and answers the conversation to carry,
// the report of what the selection did, and any refusal.
//
// It is a pure function of the document: the messages, their own order, their recorded tool calls and
// results, and nothing else. It never reads a file, follows a linked session or infers project state. The
// selection matches the ordinal values the read supplied, never a position in the read and never the
// target's post-write numbering; nothing is renumbered, so the ordinals a report names are the ones a
// caller wrote the spec with.
//
// One structure is carried whole or not at all: a recorded call and the recorded result that answers it
// (the two records whose ToolUseID is the same non-empty value) are paired by that recorded link, and a
// selection that would carry one without the other carries the counterpart instead, named in
// NecessitiesAdded with the ordinal that pulled it in. A record the source recorded with no id is not
// such a structure -- the document holds no link for it -- and the selection neither invents one nor
// carries a counterpart the read never recorded; the target's writer then reads the selected line with
// its own rule for an id-less result, which it discloses as the pairing it inferred. Strict refuses the
// separated structure by name instead of carrying the counterpart.
func Select(conversation Conversation, spec string, strict bool) (Conversation, SelectionReport, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return Conversation{}, SelectionReport{}, fmt.Errorf(
			"%w: the specification is empty: name the ordinal values to carry, as regions like 0-5,9,12-",
			ErrBadSelection)
	}
	regions, err := parseSelectionSpec(trimmed)
	if err != nil {
		return Conversation{}, SelectionReport{}, err
	}
	space := ordinalSpaceOf(conversation)
	if space.Records == 0 {
		return Conversation{}, SelectionReport{}, fmt.Errorf(
			"%w: the read returned no records, so a specification has no ordinal to name", ErrBadSelection)
	}
	spelled := strings.Join(spellOrdinalRuns(spaceOrdinals(conversation)), ",")
	if err := checkSelectionRegions(regions, space, spelled); err != nil {
		return Conversation{}, SelectionReport{}, err
	}

	selected := make([]bool, len(conversation.Messages))
	selectedCount := 0
	for index, message := range conversation.Messages {
		if regionsName(regions, message.Ordinal) {
			selected[index] = true
			selectedCount++
		}
	}
	if selectedCount == 0 {
		return Conversation{}, SelectionReport{}, fmt.Errorf(
			"%w: every ordinal the selection names falls between the read's own ordinals (%s), so no "+
				"record would be carried", ErrBadSelection, spelled)
	}

	carried, necessities, err := selectionClosure(conversation, selected, strict)
	if err != nil {
		return Conversation{}, SelectionReport{}, err
	}

	messages := make([]Message, 0, selectedCount+len(necessities))
	includedOrdinals := []int{}
	excludedOrdinals := []int{}
	for index, message := range conversation.Messages {
		switch {
		case selected[index]:
			includedOrdinals = append(includedOrdinals, message.Ordinal)
			messages = append(messages, message)
		case carried[index]:
			messages = append(messages, message)
		default:
			excludedOrdinals = append(excludedOrdinals, message.Ordinal)
		}
	}
	sort.Slice(necessities, func(i, j int) bool {
		if necessities[i].Ordinal != necessities[j].Ordinal {
			return necessities[i].Ordinal < necessities[j].Ordinal
		}
		return necessities[i].PulledBy < necessities[j].PulledBy
	})
	undischargedCalls, undischargedOrdinals := undischargedCalls(conversation, carried)

	report := SelectionReport{
		Spec:   trimmed,
		Strict: strict,
		Space:  space,
		Included: SelectionClass{
			Count:   selectedCount,
			Regions: spellOrdinalRuns(includedOrdinals),
		},
		Excluded: SelectionClass{
			Count:   len(excludedOrdinals),
			Regions: spellOrdinalRuns(excludedOrdinals),
		},
		NecessitiesAdded: SelectionNecessities{Count: len(necessities), Added: necessities},
		CallsUndischarged: SelectionClass{
			Count:   undischargedCalls,
			Regions: spellOrdinalRuns(undischargedOrdinals),
		},
	}
	selectedConversation := Conversation{
		Source:   conversation.Source,
		Messages: messages,
		Fidelity: conversation.Fidelity,
	}
	return selectedConversation, report, nil
}

// selectionWarnings names one selection in the report's own channel, so a caller reading only the exit
// code or the human block's head cannot take the conversion for the whole read. The counts are the
// report's own classes; the wording says what the counts are of rather than leaving them to be inferred.
func selectionWarnings(report *SelectionReport) []string {
	if report == nil {
		return nil
	}
	warnings := []string{fmt.Sprintf(
		"the conversion carries a selection of the read: the specification %q selected %d record(s) at "+
			"ordinals %s, %d record(s) are excluded, and %d record(s) were added as a structural "+
			"necessity (a recorded call and its recorded result are carried together)",
		report.Spec, report.Included.Count, joinOrNone(report.Included.Regions),
		report.Excluded.Count, report.NecessitiesAdded.Count)}
	if report.CallsUndischarged.Count > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%d carried call(s) at ordinals %s have no recorded result anywhere in the read (the source "+
				"itself left them undischarged); the target records them as the calls they are",
			report.CallsUndischarged.Count, joinOrNone(report.CallsUndischarged.Regions)))
	}
	if report.Strict {
		warnings = append(warnings, "the selection ran with --strict-selection: had the selection "+
			"separated a recorded call from its recorded result, the conversion would have been refused "+
			"rather than carried one half")
	}
	return warnings
}

// selectionRegion is one parsed region of a specification: a closed range of ordinal values (First..Last,
// both inclusive) or an open one (First and every higher ordinal the read holds).
type selectionRegion struct {
	first int
	open  bool
	last  int
}

// names answers whether the region names one ordinal value.
func (r selectionRegion) names(ordinal int) bool {
	if ordinal < r.first {
		return false
	}
	return r.open || ordinal <= r.last
}

// overlaps answers whether two regions name a common ordinal value. An open region names every ordinal
// at or above its first, so it overlaps a region at or above its own first in turn -- a test on the
// regions themselves, not on the ordinals a read happens to hold, because `1-3,3` names 3 twice whether
// or not the read recorded an ordinal 3.
func (r selectionRegion) overlaps(other selectionRegion) bool {
	high := func(region selectionRegion) int {
		if region.open {
			return int(^uint(0) >> 1)
		}
		return region.last
	}
	return r.first <= high(other) && other.first <= high(r)
}

// spell writes one region with the specification's own grammar: `n`, `n-m` or `n-`.
func (r selectionRegion) spell() string {
	switch {
	case r.open:
		return strconv.Itoa(r.first) + "-"
	case r.first == r.last:
		return strconv.Itoa(r.first)
	default:
		return strconv.Itoa(r.first) + "-" + strconv.Itoa(r.last)
	}
}

// parseSelectionSpec reads one specification: regions separated by commas, each an ordinal value, a
// closed range `n-m` or an open one `n-`. It is the whole grammar -- no density, no turn shorthand, no
// content selection -- and every refusal is ErrBadSelection in the words a caller can act on.
func parseSelectionSpec(spec string) ([]selectionRegion, error) {
	var regions []selectionRegion
	for _, token := range strings.Split(spec, ",") {
		text := strings.TrimSpace(token)
		if text == "" {
			return nil, fmt.Errorf(
				"%w: the specification %q holds an empty region; regions are ordinal values and ranges "+
					"separated by commas (for example 0-5,9,12-)", ErrBadSelection, spec)
		}
		region, err := parseSelectionRegion(text)
		if err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	return regions, nil
}

// parseSelectionRegion reads one region token.
func parseSelectionRegion(token string) (selectionRegion, error) {
	parts := strings.Split(token, "-")
	switch len(parts) {
	case 1:
		value, err := selectionOrdinal(token)
		if err != nil {
			return selectionRegion{}, err
		}
		return selectionRegion{first: value, last: value}, nil
	case 2:
		first, err := selectionOrdinal(parts[0])
		if err != nil {
			return selectionRegion{}, err
		}
		if strings.TrimSpace(parts[1]) == "" {
			return selectionRegion{first: first, open: true}, nil
		}
		last, err := selectionOrdinal(parts[1])
		if err != nil {
			return selectionRegion{}, err
		}
		if last < first {
			return selectionRegion{}, fmt.Errorf(
				"%w: the region %q runs backwards: a region's first ordinal has to be at or below its "+
					"last", ErrBadSelection, token)
		}
		return selectionRegion{first: first, last: last}, nil
	default:
		return selectionRegion{}, fmt.Errorf(
			"%w: %q is not a region: a region is an ordinal value, a closed range n-m or an open range "+
				"n-", ErrBadSelection, token)
	}
}

// selectionOrdinal reads one non-negative ordinal value.
func selectionOrdinal(text string) (int, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.Trim(trimmed, "0123456789") != "" {
		return 0, fmt.Errorf(
			"%w: %q is not an ordinal value: a specification names the read's own ordinal values, which "+
				"are whole numbers, as values and ranges (for example 0-5,9,12-)", ErrBadSelection, text)
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not an ordinal value this build can read (%v)",
			ErrBadSelection, text, err)
	}
	return value, nil
}

// checkSelectionRegions refuses a specification against the space the read produced: each region has to
// name ordinal values the space holds, and no ordinal may be named twice. An open region carries no
// upper number, so its own first is what is checked: `n-` is fine whenever n is within the space and
// selects from n through the highest ordinal present, while `0-` on a space of 1..5 is refused naming the
// space rather than read as "from the beginning". spelled is the space's own ordinal values as regions,
// which a refusal quotes instead of a min..max range that would claim values the read never recorded.
func checkSelectionRegions(regions []selectionRegion, space OrdinalSpace, spelled string) error {
	for index, region := range regions {
		for _, earlier := range regions[:index] {
			if region.overlaps(earlier) {
				return fmt.Errorf(
					"%w: the region %q names an ordinal the region %q already names; a specification names "+
						"each ordinal once", ErrBadSelection, region.spell(), earlier.spell())
			}
		}
		if err := checkSelectionOrdinal(region.first, space, spelled); err != nil {
			return err
		}
		if !region.open {
			if err := checkSelectionOrdinal(region.last, space, spelled); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkSelectionOrdinal refuses one ordinal value outside the space the read produced, naming the space.
func checkSelectionOrdinal(ordinal int, space OrdinalSpace, spelled string) error {
	if ordinal >= space.Min && ordinal <= space.Max {
		return nil
	}
	return fmt.Errorf(
		"%w: the selection names ordinal %d, which the read's own ordinals do not hold: the read "+
			"returned %d record(s) at ordinals %s, and a specification names values from that space",
		ErrBadSelection, ordinal, space.Records, spelled)
}

// regionsName answers whether any region names one ordinal value.
func regionsName(regions []selectionRegion, ordinal int) bool {
	for _, region := range regions {
		if region.names(ordinal) {
			return true
		}
	}
	return false
}

// ordinalSpaceOf answers the space a specification is written against: the minimum, the maximum, the
// number of records, and whether the values are min..max with no gap.
func ordinalSpaceOf(conversation Conversation) OrdinalSpace {
	space := OrdinalSpace{Records: len(conversation.Messages)}
	if space.Records == 0 {
		return space
	}
	space.Min = conversation.Messages[0].Ordinal
	space.Max = conversation.Messages[0].Ordinal
	for _, message := range conversation.Messages {
		if message.Ordinal < space.Min {
			space.Min = message.Ordinal
		}
		if message.Ordinal > space.Max {
			space.Max = message.Ordinal
		}
	}
	space.Contiguous = space.Max-space.Min+1 == space.Records
	return space
}

// spaceOrdinals answers the ordinal values a conversation's records actually hold, in read order.
func spaceOrdinals(conversation Conversation) []int {
	values := make([]int, 0, len(conversation.Messages))
	for _, message := range conversation.Messages {
		values = append(values, message.Ordinal)
	}
	return values
}

// selectionLink answers one recorded link's two halves as a message index holds them: whether the message
// holds a call with the id and whether it holds a result with it.
func selectionLink(message Message, id string) (call, result bool) {
	for _, recorded := range message.ToolCalls {
		if strings.TrimSpace(recorded.ToolUseID) == id {
			call = true
		}
	}
	for _, recorded := range message.ToolResults {
		if strings.TrimSpace(recorded.ToolUseID) == id {
			result = true
		}
	}
	return call, result
}

// selectionLinks answers, for one message, every recorded link id it holds, in the message's own order
// and with each id once.
func selectionLinks(message Message) []string {
	var ids []string
	seen := map[string]struct{}{}
	note := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, call := range message.ToolCalls {
		note(call.ToolUseID)
	}
	for _, result := range message.ToolResults {
		note(result.ToolUseID)
	}
	return ids
}

// selectionClosure answers which messages the selection carries: the selected ones, plus the counterparts
// a recorded link requires. It walks the carried set to a fixed point, because a counterpart can itself
// hold a link whose other half lies outside the selection (a record carrying one call's result and the
// next call), and each addition is reported with the ordinal that pulled it in. The walk is deterministic:
// messages are visited in read order, a message's links in its own order, and a link's members in read
// order, so the same document and spec always answer the same report.
//
// With strict, the first separating link refuses by name instead, before anything is carried and long
// before anything is installed.
func selectionClosure(
	conversation Conversation, selected []bool, strict bool,
) ([]bool, []SelectionNecessity, error) {
	links := map[string][]int{}
	for index, message := range conversation.Messages {
		for _, id := range selectionLinks(message) {
			links[id] = append(links[id], index)
		}
	}
	carried := append([]bool(nil), selected...)
	var necessities []SelectionNecessity
	for {
		changed := false
		for index, message := range conversation.Messages {
			if !carried[index] {
				continue
			}
			for _, id := range selectionLinks(message) {
				for _, member := range links[id] {
					if carried[member] {
						continue
					}
					if strict {
						return nil, nil, strictSelectionRefusal(conversation, index, member, id)
					}
					carried[member] = true
					necessities = append(necessities, SelectionNecessity{
						Ordinal:  conversation.Messages[member].Ordinal,
						PulledBy: conversation.Messages[index].Ordinal,
					})
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return carried, necessities, nil
}

// strictSelectionRefusal words the refusal for one separating link, naming the call and the result it
// cannot keep apart and the two ordinals they were recorded at.
func strictSelectionRefusal(conversation Conversation, carried, member int, id string) error {
	carriedCall, carriedResult := selectionLink(conversation.Messages[carried], id)
	if carriedCall && !carriedResult {
		return fmt.Errorf(
			"%w: the call %s recorded at ordinal %d is selected and its recorded result at ordinal %d is "+
				"not, so the conversion would carry a call whose recorded output stays behind; name a region "+
				"that holds both records, or drop --strict-selection to carry the counterpart as a "+
				"necessity", ErrStrictSelection, id, conversation.Messages[carried].Ordinal,
			conversation.Messages[member].Ordinal)
	}
	return fmt.Errorf(
		"%w: the result of the call %s recorded at ordinal %d is selected and the call itself at ordinal "+
			"%d is not, so the conversion would carry a tool result whose recorded call stays behind; name "+
			"a region that holds both records, or drop --strict-selection to carry the counterpart as a "+
			"necessity", ErrStrictSelection, id, conversation.Messages[carried].Ordinal,
		conversation.Messages[member].Ordinal)
}

// undischargedCalls answers the calls the source itself left undischarged among the carried records: a
// call whose recorded id no result anywhere in the read answers. It is the source's own state and not the
// selection's -- the counterpart rule cannot carry a result the read never held -- so it is reported
// rather than changed. A call the source recorded with no id is not counted: the read holds no link for
// it either way, and calling it undischarged would assert a pairing the source never recorded.
func undischargedCalls(conversation Conversation, carried []bool) (int, []int) {
	answered := map[string]struct{}{}
	for _, message := range conversation.Messages {
		for _, result := range message.ToolResults {
			if id := strings.TrimSpace(result.ToolUseID); id != "" {
				answered[id] = struct{}{}
			}
		}
	}
	count := 0
	var ordinals []int
	for index, message := range conversation.Messages {
		if !carried[index] {
			continue
		}
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ToolUseID)
			if id == "" {
				continue
			}
			if _, ok := answered[id]; ok {
				continue
			}
			count++
			ordinals = append(ordinals, message.Ordinal)
		}
	}
	return count, ordinals
}

// spellOrdinalRuns spells ordinal values as the specification's own regions, in ascending order and each
// value once: `0-2,5,7-9`.
func spellOrdinalRuns(values []int) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	var runs []selectionRegion
	for _, value := range sorted {
		if len(runs) > 0 {
			last := &runs[len(runs)-1]
			// The values arrive in ascending order, so a value a run already holds can only be its end.
			if value <= last.last {
				continue
			}
			if value == last.last+1 {
				last.last = value
				continue
			}
		}
		runs = append(runs, selectionRegion{first: value, last: value})
	}
	spelled := make([]string, 0, len(runs))
	for _, run := range runs {
		spelled = append(spelled, run.spell())
	}
	return spelled
}

// joinOrNone spells a class's regions for a sentence, saying "none" where the class is empty rather than
// leaving a gap a reader would take for an elision.
func joinOrNone(regions []string) string {
	if len(regions) == 0 {
		return "none"
	}
	return strings.Join(regions, ",")
}
