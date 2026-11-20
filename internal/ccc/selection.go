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

// ErrSelectionReduced reports a selection asked to reduce a conversation a selection already produced:
// the per-record line answer, the ancestry answers and the pairing a selection carries are the read's
// own -- keyed to the read's records and to the walk its loader makes -- and a second reduction could
// only re-derive them from the window, which is exactly how a window would revive a branch the first
// selection left off the line (the line is measured over the read, before the document is reduced). The
// refusal is the input's own and is answered before any specification is read, because the
// specification cannot be honoured however well-formed it is; the caller's move is to apply it to the
// read, which a selection never rewrites.
var ErrSelectionReduced = errors.New(
	"a selection cannot reduce a document a selection produced: its line, ancestry and pairing answers " +
		"are the read's own and cannot be re-derived from the window; apply the specification to the read")

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
// reads, a value the caller *supplied* has to name something -- an empty `--select "$SPEC"` whose shell
// variable was unset is a refused request, not the flag's absence, because a run that installed the whole
// history would be the one thing the caller's command was written to prevent -- and asking for the strict
// arm without naming a selection is a request about nothing. The refusals that need the read -- an
// ordinal the read's own space does not hold, a spec that selects nothing -- are Select's, because only a
// read can answer them; Continue calls both, so a caller embedding the core and one driving the CLI are
// told the same thing about the same request.
//
// The *flag's absence* is the literal empty string with no presence bit, and nothing else: a value that
// holds whitespace alone names nothing whether or not the caller set the bit -- an API caller that hands
// Continue `" \t "` has asked for a selection, and reading it as the flag's absence would install the
// whole history, the one thing the request was written to prevent -- so it is refused with the same
// sentence the empty supplied value is.
func CheckSelectionOptions(selection string, strict, supplied bool) error {
	spec := strings.TrimSpace(selection)
	if spec == "" {
		if supplied || selection != "" {
			return emptySelectionRefusal()
		}
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

// emptySelectionRefusal words the refusal for an empty specification: Select answers it for a spec it was
// handed, and CheckSelectionOptions answers it for a value a caller supplied but left blank. One sentence
// covers both surfaces, so a caller whose shell expanded an unset variable reads the same words as one
// who passed the empty string to the core.
func emptySelectionRefusal() error {
	return fmt.Errorf(
		"%w: the specification is empty: name the ordinal values to carry, as regions like 0-5,9,12-",
		ErrBadSelection)
}

// Select applies one selection specification to a read conversation and answers the conversation to carry,
// the report of what the selection did, and any refusal.
//
// It is a pure function of the document: the messages, their own order, their recorded identities and
// relations, their recorded tool calls and results, the source's own provider label -- which decides
// whether its reader may answer a projection -- and, where the conversion's own read pass established it,
// the source file's own leaf-path answer about the line (Conversation.sourceFileLine), and nothing else.
// It never reads a file, follows a linked session or infers project state: the file's answer travels on
// the conversation, taken by the pass that read the file (continueWith). The selection matches the ordinal
// values the read supplied, never a position in the read and never the target's post-write numbering;
// nothing is renumbered, so the ordinals a report names are the ones a caller wrote the spec with. The
// conversation it returns carries the read's own per-record line answer
// (Conversation.sourceLineInactive), because the line the target's loader walks is the read's and a
// window must not redefine it: a window ending on a record the read's walk places off that line is
// refused by a tree target's leaf guard and omitted and counted by a line target's walk.
//
// One structure is carried whole or not at all: a recorded call and the recorded result that answers it
// are paired from the read's own relations -- a result answers the call the read's own recorded ancestry
// names where it names one (the call the read holds on the result's own parent chain that records the
// same non-empty id), and otherwise the nearest recorded call the same non-empty id records before it
// (pairedCallOccurrences), the rule the inherited readers' own termination check takes -- and a selection
// that would carry one without the other carries the counterpart instead, named in NecessitiesAdded with
// the ordinal that pulled it in. A reused id does not make an earlier pair's records the counterpart of a
// later call: the later call has no recorded result and is counted undischarged, not expanded by id alone.
// A record the source recorded with no id is not such a structure -- the document holds no link for it --
// and the selection neither invents one nor carries a counterpart the read never recorded; the target's
// writer then reads the selected line with its own rule for an id-less result, which it discloses as the
// pairing it inferred. Strict refuses the separated structure by name instead of carrying the counterpart.
// The pairing the read answered travels with the reduced document (Conversation.sourceResultPairs), so a
// writer asking about a carried result cannot be answered by a nearer occurrence the window happened to
// carry in place of the call the read named.
//
// A conversation a selection already produced is refused by name (ErrSelectionReduced), before any
// specification is read: the answers such a document carries are the read's own, and a second reduction
// would have to re-derive the line and the ancestry from the window -- which is how a window that ends
// on a branch would revive it. The read itself is never rewritten, so the caller applies the
// specification to the read.
func Select(conversation Conversation, spec string, strict bool) (Conversation, SelectionReport, error) {
	if conversation.selectionReduced() {
		return Conversation{}, SelectionReport{}, ErrSelectionReduced
	}
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return Conversation{}, SelectionReport{}, emptySelectionRefusal()
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

	// The read's own line is answered here, before the document is reduced: which records the source's
	// relations place off the line its loader walks is a property of the read, and the writers' guard
	// exists so a target session a resumed turn would continue from cannot be a branch the source's
	// conversation left. The subset's own walk would make its last record the line's end, so the answer
	// travels with the reduced conversation (sourceLineInactive) and the writers consult it instead of
	// re-deriving one from the window. Beside it, the nearest ancestor the read holds that the window
	// also carries is answered per kept record (sourceKeptAncestor): a record whose own parent the
	// selection left out is chained to that ancestor rather than to the entry written before it, which
	// is what keeps a reduced tree the read's own line instead of reviving a branch the window carried.
	//
	// Where the source's own file stands behind this read, the file's own leaf path answers a record's
	// place (sourceFileLine, established by the read pass): the reconstruction's tool-result projection
	// cannot tell a record the source placed under a call from one it placed under that call's result, so
	// a record standing beside the result in the file's order reads as hanging off it -- and a window that
	// ends on the result of a call the conversation continued past would then pass the writers' leaf guard
	// and resume a branch the source's file left (round 11's second finding). The file's own walk settles
	// the question for every record the read answered an identity for; the reconstruction answers for the
	// rest (and where no file stands behind the read).
	readLineInactive, readLineHolds, _ := inactiveSourceRecords(conversation.Messages, conversation.Source.Provider)
	for index, answer := range conversation.sourceFileLine {
		if !answer.answered {
			continue
		}
		readLineHolds[index] = answer.holds
		readLineInactive[index] = !answer.holds
	}
	keptAncestors := selectionKeptAncestors(conversation, carried)

	messages := make([]Message, 0, selectedCount+len(necessities))
	lineInactive := make([]bool, 0, selectedCount+len(necessities))
	lineHolds := make([]bool, 0, selectedCount+len(necessities))
	// fileParents carries the file's own relation per kept record, in the reduced document's positions,
	// for the tree writers: the entry the file's bytes name as the record's parent, or the empty string
	// where no file answered for it (Conversation.sourceFileParent).
	fileParents := make([]string, 0, selectedCount+len(necessities))
	// reducedOf maps a read record's index to its position in the reduced document, -1 for a record the
	// selection left out; readIndexes is the reverse, one entry per carried record.
	reducedOf := make([]int, len(conversation.Messages))
	readIndexes := make([]int, 0, selectedCount+len(necessities))
	includedOrdinals := []int{}
	excludedOrdinals := []int{}
	for index, message := range conversation.Messages {
		reducedOf[index] = -1
		switch {
		case selected[index]:
			includedOrdinals = append(includedOrdinals, message.Ordinal)
		case carried[index]:
		default:
			excludedOrdinals = append(excludedOrdinals, message.Ordinal)
			continue
		}
		reducedOf[index] = len(messages)
		readIndexes = append(readIndexes, index)
		messages = append(messages, message)
		lineInactive = append(lineInactive, readLineInactive[index])
		lineHolds = append(lineHolds, readLineHolds[index])
		fileParent := ""
		if index < len(conversation.sourceFileLine) {
			fileParent = strings.TrimSpace(conversation.sourceFileLine[index].parent)
		}
		fileParents = append(fileParents, fileParent)
	}
	// The ancestry answers are the read's own indices (selectionKeptAncestors), and the writers consult
	// the *reduced* document's own positions, so each answer is carried across: the ancestor is carried
	// by construction, and its reduced index is the position whose entry the record's entry follows.
	keptAncestor := make([]keptAncestorAnswer, len(messages))
	for reduced, readIndex := range readIndexes {
		answer := keptAncestors[readIndex]
		if answer.index >= 0 {
			answer.index = reducedOf[answer.index]
		}
		keptAncestor[reduced] = answer
	}
	// The seam's own pairing travels beside them, in the reduced document's positions: the writers have
	// to ask the walk the closure asked, not re-read the window -- a record the window left out can be
	// one the read's ancestry walk stepped through, and the same id occurring nearer in the reduced
	// document would then pair a result with a call the read did not name for it.
	readPairs := pairedCallOccurrences(conversation)
	resultPairs := make(map[resultOccurrence]callOccurrence, len(readPairs))
	for place, call := range readPairs {
		if reducedOf[place.message] < 0 || reducedOf[call.message] < 0 {
			continue
		}
		resultPairs[resultOccurrence{message: reducedOf[place.message], result: place.result}] =
			callOccurrence{message: reducedOf[call.message], call: call.call}
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
		Source:             conversation.Source,
		Messages:           messages,
		Fidelity:           conversation.Fidelity,
		sourceLineInactive: lineInactive,
		sourceLineHolds:    lineHolds,
		sourceKeptAncestor: keptAncestor,
		sourceResultPairs:  resultPairs,
		sourceFileParent:   fileParents,
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
			"%d carried call(s) at ordinals %s have no recorded result that states an outcome anywhere in "+
				"the read (the source itself left them undischarged); whether a target can hold a call with "+
				"no recorded outcome is that target's own writer's answer, and the conversion's losses "+
				"name what the writer left out",
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
// content selection -- and every refusal is ErrBadSelection in the words a caller can act on. The
// duplicate-region refusal is part of this step because it is a property of the specification's own
// regions and of nothing the read holds: a command line `--select 1,1` is answered as the caller's own
// refusal even where the source cannot be read at all (CheckSelectionOptions), rather than masked as a
// source failure.
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
		if err := checkSelectionOverlap(regions, region); err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	return regions, nil
}

// checkSelectionOverlap refuses a region that names an ordinal an earlier region of the same
// specification already names. It is decided on the regions themselves, never on the ordinals a read
// happens to hold, so the grammar step answers it before any read is opened.
func checkSelectionOverlap(earlier []selectionRegion, region selectionRegion) error {
	for _, previous := range earlier {
		if region.overlaps(previous) {
			return fmt.Errorf(
				"%w: the region %q names an ordinal the region %q already names; a specification names "+
					"each ordinal once", ErrBadSelection, region.spell(), previous.spell())
		}
	}
	return nil
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
// name ordinal values the space holds. The duplicate-region half is the grammar's own (parseSelectionSpec
// and checkSelectionOverlap) and is answered before a read is opened, so a missing source cannot mask it;
// what needs the read is this step. An open region carries no upper number, so its own first is what is
// checked: `n-` is fine whenever n is within the space and selects from n through the highest ordinal
// present, while `0-` on a space of 1..5 is refused naming the space rather than read as "from the
// beginning". spelled is the space's own ordinal values as regions, which a refusal quotes instead of a
// min..max range that would claim values the read never recorded.
func checkSelectionRegions(regions []selectionRegion, space OrdinalSpace, spelled string) error {
	for _, region := range regions {
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

// callOccurrence names one recorded call the read holds with a non-empty id, by its place in the
// document's own order: the message's position in the walk and the call's index among that message's own
// calls. The place, not the id, identifies an occurrence -- a source may record one id on several calls,
// and the seam's pairing depends on which occurrence stands where.
type callOccurrence struct {
	message int
	call    int
}

// resultOccurrence names one recorded result the same way.
type resultOccurrence struct {
	message int
	result  int
}

// pairedCallOccurrences answers the seam's own pairing at occurrence granularity: for every recorded
// result that names an id, the call occurrence the result's own recorded relations name where they do --
// the call the read holds on the result's own ancestry chain (its own record, else the nearest record its
// recorded parent walk reaches) that records the same non-empty id, so a reused id later in file order
// cannot override the call the source placed the result under -- and otherwise the call occurrence the
// same non-empty id records nearest before it, in the read's own order, the reading the inherited
// readers' termination check takes (internal/parser/termination.go's hasOrphanedToolCall). The answer is
// the seam's own: the closure's pairs, the report's undischarged count and the writers cannot disagree
// about which call a recorded result answers.
//
// A conversation a selection produced carries the pairing its own read answered
// (Conversation.sourceResultPairs), in the reduced document's positions: the window may have left the
// records the ancestry walk steps through out of the document, and re-reading the pairing over it would
// let a nearer carried occurrence override the answer the read gave -- the flat writers would then write
// the result under a call the read did not pair it with. A conversation the read returned carries no
// such answers (nil), and this walk resolves them.
//
// Within one message the calls are walked before the results (the order every writer's own ledger
// takes), and the occurrence the answer names is the *unit* the writers write: a message's calls sharing
// an id are one invocation the reader projected into several rows (distinctCalls), so the first row of
// such a group is the occurrence that stands for the unit, exactly as the message-level pairing names
// the message. It is the granularity the flat targets' writers need: their document is one line of
// records, a call the line leaves off it is not written at all, and a result whose *paired occurrence*
// was not written has to be omitted and counted rather than written under an older call that happens to
// share its id. A call or result the source recorded with no id is not an occurrence: the document holds
// no link for it, and the writers' own inference for an id-less result reads the selected line
// (disclosed as generated_call_ids).
func pairedCallOccurrences(conversation Conversation) map[resultOccurrence]callOccurrence {
	if conversation.sourceResultPairs != nil {
		return conversation.sourceResultPairs
	}
	byIdentity := make(map[string]int, len(conversation.Messages))
	for index, message := range conversation.Messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			if _, seen := byIdentity[id]; !seen {
				byIdentity[id] = index
			}
		}
	}
	// firstCallOn answers the occurrence that stands for the call a record records under one non-empty
	// id: the first row of the group, because a message's calls sharing an id are one invocation the
	// reader projected into several rows (distinctCalls).
	firstCallOn := func(index int, id string) (callOccurrence, bool) {
		for callIndex, call := range conversation.Messages[index].ToolCalls {
			if strings.TrimSpace(call.ToolUseID) == id {
				return callOccurrence{message: index, call: callIndex}, true
			}
		}
		return callOccurrence{}, false
	}
	// ancestryCall answers the call occurrence the result's recorded ancestry establishes: the nearest
	// record on the walk from the result's own record (itself first, where the read folded the outcome
	// onto the call's own message) through the recorded parent relations that records the same id. The
	// walk steps through the identities the read answered and the records it holds -- an identity the
	// read does not hold ends it -- and is bounded by the document's own size so a cycle a source
	// recorded cannot keep it going.
	ancestryCall := func(index int, id string) (callOccurrence, bool) {
		if call, ok := firstCallOn(index, id); ok {
			return call, true
		}
		at := index
		for steps := 0; steps <= len(conversation.Messages); steps++ {
			parent := strings.TrimSpace(conversation.Messages[at].SourceParentUUID)
			if parent == "" {
				return callOccurrence{}, false
			}
			next, ok := byIdentity[parent]
			if !ok || next >= at {
				return callOccurrence{}, false
			}
			if call, ok := firstCallOn(next, id); ok {
				return call, true
			}
			at = next
		}
		return callOccurrence{}, false
	}
	mostRecent := map[string]callOccurrence{}
	paired := map[resultOccurrence]callOccurrence{}
	for index, message := range conversation.Messages {
		seen := map[string]bool{}
		for callIndex, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ToolUseID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			mostRecent[id] = callOccurrence{message: index, call: callIndex}
		}
		for resultIndex, result := range message.ToolResults {
			id := strings.TrimSpace(result.ToolUseID)
			if id == "" {
				continue
			}
			place := resultOccurrence{message: index, result: resultIndex}
			if call, ok := ancestryCall(index, id); ok {
				paired[place] = call
				continue
			}
			if call, ok := mostRecent[id]; ok {
				paired[place] = call
			}
		}
	}
	return paired
}

// selectionKeptAncestors answers, for every record of the read, the nearest ancestor the read holds that
// the selection carries, or -1 where there is none: the walk starts at the record's own recorded parent
// and steps until it reaches a carried record, the read runs out of relation, or the step budget the
// document's own size sets ends a cycle the source recorded. Each step takes the source file's own
// relation where a file stands behind the read and answered for the record the walk is at
// (sourceFileLine's parent, the file's `parentId`), and the read's own answers below for every other
// record. Taking the file's relation is what keeps a reduced tree the file's own tree: the read answers a
// record the file placed under a tool result as hanging off the message that made the call (a projection
// the writers' own step resolves, sourceToolResultBefore), so a walk that used the read's answer for an
// *excluded* record could step through the projection to a result the file ran beside the line and leave
// the window hanging off an off-line entry -- while the file's own chain runs through the result the
// window carries, and the writers' guard refuses a window the file's own chain represents. The
// previous-record fallback the readers' own line walk takes where the read answered no relation is *not*
// taken here: a record whose relation cannot be walked is answered no ancestor, and the writer's own
// fallback decides its place -- refusing the conversion where that place would put an off-line record on
// the walk.
//
// Each answer carries whether resolving it took the projected step: the read answers a record under a
// call and one under that call's result the same way, so an answer that crossed the step is a relation
// this conversion resolved rather than one the read named, and the writer counts it
// (unresolved_source_parents) exactly where it counts the projection it resolves itself. An answer that
// followed the file's own parent is the source's relation and is not counted.
//
// It is a pure function of the recorded document, like the selection itself: the relations are the
// read's own answers and the file's own answer where it stands behind the read, and the map is keyed by
// the identities the read supplied.
func selectionKeptAncestors(conversation Conversation, carried []bool) []keptAncestorAnswer {
	byIdentity := make(map[string]int, len(conversation.Messages))
	for index, message := range conversation.Messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			if _, seen := byIdentity[id]; !seen {
				byIdentity[id] = index
			}
		}
	}
	parentAnswersProject := projectedParentProvider(conversation.Source.Provider)
	ancestors := make([]keptAncestorAnswer, len(conversation.Messages))
	for index := range ancestors {
		ancestors[index] = keptAncestorAnswer{index: -1}
	}
	for index := range conversation.Messages {
		if !carried[index] {
			continue
		}
		answer := keptAncestorAnswer{index: -1}
		at := index
		for steps := 0; steps <= len(conversation.Messages); steps++ {
			next := -1
			projected := false
			// The source file's own relation first, where one stands behind the read and answered for
			// the record the walk is at: the file's bytes name the record's parent themselves, and the
			// read's own answer for the record may be the projection a file-backed Pi-family read makes
			// (a record the file placed under a tool result reads as hanging off the message that made
			// the call). The step through an *excluded* record is where the raw relation alone gets it
			// wrong: the excluded record's read parent may be the projected message while the file's
			// own chain runs through the result the window carries, and following the projection would
			// step past that result to a sibling the file's own line leaves off -- the window then
			// chains the record to an off-line entry, and the writers' guard refuses a window the
			// file's own chain represents. A file parent that does not resolve (an entry the read does
			// not hold, a record outside the read) leaves the read's own relation below to decide,
			// exactly as it did before the file answered anything.
			if at < len(conversation.sourceFileLine) {
				fileAnswer := conversation.sourceFileLine[at]
				if fileAnswer.answered {
					if fileParent := strings.TrimSpace(fileAnswer.parent); fileParent != "" {
						if held, ok := byIdentity[fileParent]; ok && held != index {
							next = held
						}
					}
				}
			}
			if next < 0 {
				parent := strings.TrimSpace(conversation.Messages[at].SourceParentUUID)
				if parent == "" {
					break
				}
				if parentAnswersProject {
					if resultAt, ok := sourceToolResultBefore(conversation.Messages, at, parent); ok {
						next = resultAt
						projected = true
					}
				}
				if next < 0 {
					held, ok := byIdentity[parent]
					if !ok || held == index {
						break
					}
					next = held
				}
			}
			if projected {
				answer.projected = true
			}
			if carried[next] {
				answer.index = next
				break
			}
			at = next
		}
		ancestors[index] = answer
	}
	return ancestors
}

// selectionPair is one recorded call and the recorded result that answers it: the messages the two
// occurrences sit on and the id they share. It is what the seam's structure rule carries whole.
type selectionPair struct {
	id     string
	call   int
	result int
}

// selectionPairing is the seam's own reading of the recorded call/result links, in one walk
// (pairedCallOccurrences) so that the closure's pairs, the report's undischarged count and the flat
// targets' writers cannot disagree about which call a recorded result answers. pairs holds every recorded
// result against the call the read's own recorded ancestry names where it does, else the nearest call
// occurrence the same non-empty id recorded *before* it -- the reading the inherited readers' own
// termination check takes
// (internal/parser/termination.go's hasOrphanedToolCall counts only results recorded after the call it
// answers), so a reused id does not make an earlier pair's records the counterpart of a later call -- and
// answered names, per call message, the ids of the calls an *outcome* result thus answers. The pairing is
// the read's own relation rather than a consumption: two results after one call both pair with it (a
// streamed terminal event beside a summary), and a call the source recorded after an outcome that lies
// before it is not answered by it.
func selectionPairing(conversation Conversation) ([]selectionPair, map[int]map[string]bool) {
	paired := pairedCallOccurrences(conversation)
	var pairs []selectionPair
	answered := map[int]map[string]bool{}
	for index, message := range conversation.Messages {
		for resultIndex, result := range message.ToolResults {
			id := strings.TrimSpace(result.ToolUseID)
			if id == "" {
				continue
			}
			call, ok := paired[resultOccurrence{message: index, result: resultIndex}]
			if !ok {
				continue
			}
			pairs = append(pairs, selectionPair{id: id, call: call.message, result: index})
			if !resultStatesAnOutcome(result.Status) {
				continue
			}
			if answered[call.message] == nil {
				answered[call.message] = map[string]bool{}
			}
			answered[call.message][id] = true
		}
	}
	return pairs, answered
}

// selectionPairs answers the pairs half of selectionPairing, for the closure and the strict refusal.
func selectionPairs(conversation Conversation) []selectionPair {
	pairs, _ := selectionPairing(conversation)
	return pairs
}

// selectionClosure answers which messages the selection carries: the selected ones, plus the counterparts
// a recorded pair requires. It walks the carried set to a fixed point, because a counterpart can itself
// hold a pair whose other half lies outside the selection (a record carrying one call's result and the
// next call), and each addition is reported with the ordinal that pulled it in. The walk is deterministic:
// messages are visited in read order and a message's pairs in the read's own order, so the same document
// and spec always answer the same report.
//
// With strict, the first separating pair refuses by name instead, before anything is carried and long
// before anything is installed.
func selectionClosure(
	conversation Conversation, selected []bool, strict bool,
) ([]bool, []SelectionNecessity, error) {
	pairs := selectionPairs(conversation)
	carried := append([]bool(nil), selected...)
	if strict {
		for _, pair := range pairs {
			if carried[pair.call] == carried[pair.result] {
				continue
			}
			return nil, nil, strictSelectionRefusal(conversation, pair, carried[pair.call])
		}
	}
	// counterparts names, per message, the other side of every pair that touches it: the walk carries a
	// pair whole from either direction, and a pair whose halves sit on one record is already whole.
	counterparts := make([][]int, len(conversation.Messages))
	for _, pair := range pairs {
		if pair.call == pair.result {
			continue
		}
		counterparts[pair.call] = append(counterparts[pair.call], pair.result)
		counterparts[pair.result] = append(counterparts[pair.result], pair.call)
	}
	var necessities []SelectionNecessity
	for {
		changed := false
		for index := range conversation.Messages {
			if !carried[index] {
				continue
			}
			for _, member := range counterparts[index] {
				if carried[member] {
					continue
				}
				carried[member] = true
				necessities = append(necessities, SelectionNecessity{
					Ordinal:  conversation.Messages[member].Ordinal,
					PulledBy: conversation.Messages[index].Ordinal,
				})
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return carried, necessities, nil
}

// strictSelectionRefusal words the refusal for one separated pair, naming the call and the result it
// cannot keep apart, the two ordinals they were recorded at, and which side the selection named.
func strictSelectionRefusal(conversation Conversation, pair selectionPair, callSelected bool) error {
	call := conversation.Messages[pair.call]
	result := conversation.Messages[pair.result]
	if callSelected {
		return fmt.Errorf(
			"%w: the call %s recorded at ordinal %d is selected and its recorded result at ordinal %d is "+
				"not, so the conversion would carry a call whose recorded output stays behind; name a region "+
				"that holds both records, or drop --strict-selection to carry the counterpart as a "+
				"necessity", ErrStrictSelection, pair.id, call.Ordinal, result.Ordinal)
	}
	return fmt.Errorf(
		"%w: the result of the call %s recorded at ordinal %d is selected and the call itself at ordinal "+
			"%d is not, so the conversion would carry a tool result whose recorded call stays behind; name "+
			"a region that holds both records, or drop --strict-selection to carry the counterpart as a "+
			"necessity", ErrStrictSelection, pair.id, result.Ordinal, call.Ordinal)
}

// undischargedCalls answers the recorded calls the source itself left undischarged among the carried
// records: a call no later recorded outcome answers, per the seam's own pairing rule (selectionPairing:
// a result answers the call occurrence the read's own ancestry establishes where it does, else the
// nearest call occurrence the same non-empty id records before it, so a reused id does not make an
// earlier call's records the counterpart of a later call). It is the source's own state and not the
// selection's -- the counterpart rule cannot carry a result the read never held -- so it is reported
// rather than changed.
//
// The unit is the writers' own (distinctCalls): a message's calls sharing a non-empty id are one
// invocation the reader projected into several rows (the inherited Codex reader expands one `apply_patch`
// into one call per file it names), so the extra rows are not extra calls and one recorded outcome
// discharges the invocation -- the same reading the closure takes, which pairs and carries such a message's
// call once. A call the source recorded with no id has no recorded link either way, and it is counted
// where the carried document holds no id-less result that states an outcome at or after it: no outcome in
// the line could be paired with it by any rule, so it is a call the source left undischarged and the count
// says so. Where the carried document does hold such a result, which call it answers is the writers' own
// disclosed inference (the sole outstanding call in the line it wrote, counted as generated_call_ids)
// rather than a link the read recorded, and the call is left to that report instead of being read as
// answered -- or as undischarged -- by a guess in the other direction.
//
// An archive read's own summary of a call is outcome evidence too: where the call carries one and the
// read holds no recorded result of its own for it (callHasRecordedResult, the writers' own guard, under
// which they write that summary as the call's outcome), the source recorded the outcome -- in the
// archive's derived column rather than as a result event -- and the call is not among the undischarged,
// exactly as the routes that write the summary do not leave it out. Where a recorded result event does
// stand for the call, the summary is a view of it and the call's disposition follows that event, not the
// column.
//
// Progress is not an answer: a result whose recorded status says the call is still running (a Codex
// subagent's `running` notification, a streamed chunk) is the read's record of a call that has not
// finished, and counting it as an outcome would state that the source answered a call it left pending --
// the inherited termination rule reads the same event the same way, an id-less progress event cannot
// establish an outcome either, and the writers that refuse to write such an event as an outcome count the
// call on their own side (unpaired_tool_calls on OpenCode). The answers are selectionPairing's own (one
// walk for both the closure and this count), so no second accounting can disagree with the pairing: an
// earlier result does not resolve a later call, and a later outcome does not retroactively answer an
// earlier call the seam paired with nothing.
func undischargedCalls(conversation Conversation, carried []bool) (int, []int) {
	_, answered := selectionPairing(conversation)
	// summaryDischarges answers whether the archive's own summary of a call is the outcome evidence the
	// read holds for it: the summary exists (ResultContent) and the read holds no recorded result of its
	// own for the call, which is exactly where the writers carry the summary as the call's outcome rather
	// than treating it as a view of a recorded event (callHasRecordedResult).
	summaryDischarges := func(call ToolCall, results []ToolResult, id string) bool {
		if _, ok := derivedResultSummary(call, id); !ok {
			return false
		}
		return !callHasRecordedResult(results, id)
	}
	// idLessResultAfter answers, per message position, whether the carried document holds an id-less
	// result that states an outcome at or after it: the writers' inference (the sole outstanding call in
	// the line it wrote) can reach a call from a result that follows it in the line, never from one
	// recorded before it -- and a progress event does not state an outcome, so it cannot establish one
	// for the call either.
	idLessResultAfter := make([]bool, len(conversation.Messages)+1)
	for index := len(conversation.Messages) - 1; index >= 0; index-- {
		idLessResultAfter[index] = idLessResultAfter[index+1]
		if !carried[index] {
			continue
		}
		for _, result := range conversation.Messages[index].ToolResults {
			if strings.TrimSpace(result.ToolUseID) == "" && resultStatesAnOutcome(result.Status) {
				idLessResultAfter[index] = true
				break
			}
		}
	}
	count := 0
	var ordinals []int
	for index, message := range conversation.Messages {
		if !carried[index] {
			continue
		}
		units, discharged := 0, 0
		seen := map[string]bool{}
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ToolUseID)
			if id == "" {
				if !idLessResultAfter[index] && !summaryDischarges(call, message.ToolResults, "") {
					units++
				}
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			units++
			if answered[index][id] || summaryDischarges(call, message.ToolResults, id) {
				discharged++
			}
		}
		if undischarged := units - discharged; undischarged > 0 {
			count += undischarged
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
