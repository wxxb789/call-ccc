package ccc

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.kenn.io/agentsview/internal/parser"
)

// Selection is which recorded regions of a conversation cross into the target, chosen by the ordinal
// values the read supplied (Message.Ordinal) rather than by position. One specification is a
// comma-separated list of regions -- `0-5,9,12-` -- where a bare ordinal names one record, `N-M` is a
// closed range, and `N-` names ordinal N and every higher ordinal the read holds. The whole matrix of the
// specification's own rules lives in this file, as a pure function of the document, so an embedding
// caller and the CLI are told the same sentence for the same mistake.

// ErrBadSelection reports a selection specification this build refuses: one that is not a list of
// ordinal values and regions, one that names an ordinal the read's own ordinals do not hold, or one that
// would carry no record at all. A bad selection is refused, never repaired: a spec silently read as
// something else would write a target session the caller did not ask for, and the caller's next move is
// to write a different spec, not to inspect the source.
var ErrBadSelection = errors.New("the selection cannot be honoured")

// ErrStrictSelection reports a selection that --strict-selection refuses: it would carry a recorded tool
// call without the recorded result that answers it, or a recorded result without its recorded call, and
// the caller asked for the refusal instead of the counterpart being carried. Nothing is written before
// the refusal, so the source session and the target's data directory are as they were.
var ErrStrictSelection = errors.New(
	"the selection separates a recorded tool call from its recorded result")

// ErrSelectionUnrepresentable reports a selection whose counterpart the reference route cannot carry
// without changing the disposition the source gave it. Two shapes are known, and both refuse the whole
// selection by name -- the pair and both of its ordinals -- by default as well as under
// --strict-selection: (1) the record a recorded call/result pair's other half would pull in is one the
// read's own line leaves inactive, and this target's log is one sequence of records with no parent
// relation -- a record written there becomes model-visible history -- so carrying it to satisfy closure
// would activate a branch the source's conversation left; and (2) the counterpart is a record the route's
// own writer does not publish -- its recorded outcome has no text this format's output could carry, for
// one, or its whole shape is one this target has no place for -- so the pair would be written with half
// its recorded structure while the selection's own report declared the counterpart carried. The caller's
// move in both is a specification that holds both records.
var ErrSelectionUnrepresentable = errors.New(
	"the selection separates a recorded tool call from its recorded result, and the record the pair " +
		"requires cannot be carried by this route")

// ErrSelectionUnsupportedPair reports a selection asked of a pair this build does not honour. The flag
// is accepted on the reference pair alone -- a Claude Code source with a Codex target -- because the
// other routes' writers do not consume the seam yet: ignoring the flag would install history the caller
// excluded, and handing them the reduced history would let them re-infer from a punctured stream what
// the seam exists to carry. The refusal is answered before the source is opened and before anything is
// installed, so a refused run reads no source and writes no file.
var ErrSelectionUnsupportedPair = errors.New(
	"a selection is honoured on one pair only: a Claude Code source with a Codex target (claude:… " +
		"--to codex)")

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

// ErrSelectionUnsupportedWriter reports a document a selection produced handed to a target writer that
// does not consume the seam. The pair gate already refuses the CLI's routes (a Claude Code source with a
// Codex target alone honours the flag), so a command line cannot reach one of these writers with a
// reduced document; the refusal exists for a caller embedding the core, which can apply Select itself
// and then hand the window to any writer. Writing it there would let the writer recompute from the
// punctured stream exactly the cross-record facts the seam exists to carry -- the OpenCode writer's own
// line walk would make a selection-omitted result the conversation's new leaf, for one -- so the writer
// refuses by name instead. The caller's move is the reference route's writer (ConvertToCodexRollout) or
// no selection at all.
var ErrSelectionUnsupportedWriter = errors.New(
	"a document a selection produced cannot be written here: only the Codex writer consumes the " +
		"selection seam")

// selectionUnsupportedWriterRefusal words the writer-side refusal for one target, so every writer that
// does not consume the seam says the same thing and names the target it is.
func selectionUnsupportedWriterRefusal(target string) error {
	return fmt.Errorf("%w: this run would write %q", ErrSelectionUnsupportedWriter, target)
}

// IsSelectionRefusal reports whether a run failed because of the selection it was handed: a
// specification this build refuses, a strict refusal, a counterpart the route cannot carry, a pair the
// flag is not honoured on, or a writer that does not consume the seam. The CLI answers these with the
// usage exit code, because the run's command line is what has to change.
func IsSelectionRefusal(err error) bool {
	return errors.Is(err, ErrBadSelection) || errors.Is(err, ErrStrictSelection) ||
		errors.Is(err, ErrSelectionUnrepresentable) || errors.Is(err, ErrSelectionUnsupportedPair) ||
		errors.Is(err, ErrSelectionUnsupportedWriter) || errors.Is(err, ErrSelectionReduced)
}

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
// included, excluded and necessity classes, calls for the undischarged one -- and the regions name the
// ordinals of the records that unit sits on, so a message carrying two undischarged calls appears once
// while both calls are counted.
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

// SelectionNecessities is the necessity class: the count of records added, and each addition with the
// ordinal that pulled it in.
type SelectionNecessities struct {
	Count int                  `json:"count"`
	Added []SelectionNecessity `json:"added"`
}

// What became of one record the selection carried, as the writer's own walk answered it. There are two
// answers rather than three: a refused selection is refused whole, before anything is written, so no
// report holds a record whose fate was a refusal.
const (
	// BecameNativeHistory means the target document holds at least one written record for this source
	// record.
	BecameNativeHistory = "native_history"
	// BecameLost means the target document holds no written record for this source record; Reason names
	// the category the writer counted it under.
	BecameLost = "lost"
)

// SelectionRecord is one record the selection carried -- one the specification named, or one a
// counterpart pulled in -- with the fate the writer gave it: what became of it, the loss category that
// explains a record the target does not hold, and the item kinds the target wrote for it. It is D1's
// accounting: every carried record is in the table, whether or not the target holds a record of its own
// for it.
type SelectionRecord struct {
	Ordinal   int  `json:"ordinal"`
	Necessity bool `json:"necessity,omitempty"`
	// PulledBy is the ordinal whose recorded call or result pulled a necessity in; nil for a record the
	// specification named (ordinal 0 is a real ordinal, so the field is a pointer rather than a sentinel).
	PulledBy *int `json:"pulled_by,omitempty"`
	// Became is BecameNativeHistory or BecameLost; Reason is the loss category behind a lost record and
	// empty otherwise.
	Became string `json:"became"`
	Reason string `json:"reason,omitempty"`
	// Written is the item kinds the target wrote for this record: one or more for a record that became
	// native history, and empty for a lost one. A record can become history and still have evidence
	// counted under a loss category (a system record whose text is omitted while its tool result is
	// written), which the conversion's own losses state beside this table.
	Written []string `json:"written,omitempty"`
	// CallsUndischarged is how many calls this record carries that the read left without an outcome (the
	// unit CallsUndischarged counts). Together with Became and Written it lets a caller check the class's
	// split: a record whose fate wrote its calls contributes them to the conversion's
	// outstanding_tool_calls, and a lost one carries them into CallsUndischargedLost instead.
	CallsUndischarged int `json:"calls_undischarged,omitempty"`
}

// SelectionReport is what one selection did to the read, in the classes the goal names: the space the
// spec was written against, the records it selected that the read holds, the records it left behind, the
// counterparts carried because a recorded call and its recorded result are one structure, the calls the
// source itself left undischarged, and the per-record disposition table the writer's own walk filled in.
// The classes reconcile with the read: Included plus NecessitiesAdded plus Excluded are exactly the
// records the read returned -- a necessity is a record the specification did not name, so Included does
// not hold it -- and a caller can check that against the source block's own message count.
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
	// CallsUndischargedLost is the part of CallsUndischarged whose records the writer's own walk did not
	// publish: those calls are lost with their records (each row's own category is on the per-record
	// table), so they are not in the conversion's outstanding_tool_calls -- the rest of the class is, and
	// this split is what makes the two report surfaces reconcile. It is filled from the writer's fates
	// (fillSelectionUndischargedSplit); zero on a report the writer's walk has not answered, which the
	// warning channel words as an answer not yet made rather than as "none lost".
	CallsUndischargedLost SelectionClass `json:"calls_undischarged_lost"`
	// Records pairs every carried record with the fate the writer gave it (D1).
	Records []SelectionRecord `json:"records"`
}

// CheckSelectionOptions answers whether a selection request is one this build can honour, in the words
// the CLI refuses a command line with as well: a specification has to be the grammar parseSelectionSpec
// reads, a value the caller *supplied* has to name something -- an empty `--select "$SPEC"` whose shell
// variable was unset is a refused request, not the flag's absence, because a run that installed the whole
// history would be the one thing the caller's command was written to prevent -- and the strict arm, with
// or without a value (an explicit `--strict-selection=false` is still the flag's presence), is a request
// about a selection and needs --select to name the selection it is about. The refusals that need the
// read -- an ordinal the read's own space does not hold, a spec that selects nothing -- are Select's,
// because only a read can answer them; Continue calls both, so a caller embedding the core and one
// driving the CLI are told the same thing about the same request.
//
// The *flag's absence* is the literal empty string with no presence bit, and nothing else: a value that
// holds whitespace alone names nothing whether or not the caller set the bit, so it is refused with the
// same sentence the empty supplied value is.
func CheckSelectionOptions(selection string, strict, strictSupplied, supplied bool) error {
	spec := strings.TrimSpace(selection)
	if spec == "" {
		if supplied || selection != "" {
			return emptySelectionRefusal()
		}
		if strict || strictSupplied {
			return fmt.Errorf(
				"%w: --strict-selection is the strict arm of a selection, so it needs --select to name "+
					"the selection it is about -- the flag's presence is the request, whether its value "+
					"is spelled true or false", ErrBadSelection)
		}
		return nil
	}
	_, err := parseSelectionSpec(spec)
	return err
}

// CheckSelectionPair answers whether this build honours a selection on the invocation at hand: the
// reference pair is a Claude Code source read with a Codex target, and every other pair is refused by
// name before the source is opened. Both halves are known from the invocation itself -- the reference's
// own provider label and the target's name -- so the decision costs no read, and a caller embedding the
// core and one driving the CLI are told the same sentence.
func CheckSelectionPair(ref Ref, target string) error {
	provider := strings.ToLower(strings.TrimSpace(ref.Provider))
	// No stored-label normalization applies: every registered provider is its own owner, and the one
	// provider with a second stored label is Codebuff→Freebuff, so a label that is not `claude` is not
	// this pair. Re-check this if an upstream update ever gives Claude a stored alias.
	if provider == string(parser.AgentClaude) && strings.EqualFold(strings.TrimSpace(target), TargetCodex) {
		return nil
	}
	return fmt.Errorf(
		"%w: this run reads %q and writes %q", ErrSelectionUnsupportedPair, ref.Provider, target)
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

// selectionRegion is one parsed region of a specification: the closed range Lo..Hi where Hi is the
// highest ordinal an open region can reach (an open `N-` carries no upper number of its own, so its Hi
// is only a bound the read's own space then answers against).
type selectionRegion struct {
	Lo int
	Hi int
	// open says the region was written `N-`: it names every ordinal from N through the read's highest.
	open bool
}

// names answers whether the region covers one ordinal value.
func (r selectionRegion) names(ordinal int) bool {
	return ordinal >= r.Lo && ordinal <= r.Hi
}

// overlaps answers whether two regions share an ordinal: the same value written twice, or two ranges
// that intersect, with an open region covering every ordinal above its lower bound -- a spec that names
// an ordinal an open region already names is redundant however far that region's read reaches, and a
// region outside the space is refused by the space check whatever the overlap answer is (the overlap
// refusal is answered first because the specification's own grammar is what it is about).
func (r selectionRegion) overlaps(other selectionRegion) bool {
	if r.open || other.open {
		high := r.Hi
		if r.open {
			high = math.MaxInt
		}
		otherHigh := other.Hi
		if other.open {
			otherHigh = math.MaxInt
		}
		return r.Lo <= otherHigh && other.Lo <= high
	}
	return r.Lo <= other.Hi && other.Lo <= r.Hi
}

// spell renders the region with the specification's own grammar.
func (r selectionRegion) spell() string {
	if r.open {
		return strconv.Itoa(r.Lo) + "-"
	}
	if r.Lo == r.Hi {
		return strconv.Itoa(r.Lo)
	}
	return strconv.Itoa(r.Lo) + "-" + strconv.Itoa(r.Hi)
}

// parseSelectionSpec reads one specification: comma-separated tokens, each a bare ordinal, a closed
// range or an open one. A malformed token (empty between commas, a non-numeric part, an inverted range)
// is refused with the sentence that names the token, and a token that repeats an ordinal an earlier
// region already names is refused as the goal's *duplicated region* -- read as a set, the specification
// has to name each ordinal once, because a caller who wrote one twice meant something this grammar
// cannot express.
func parseSelectionSpec(spec string) ([]selectionRegion, error) {
	tokens := strings.Split(spec, ",")
	regions := make([]selectionRegion, 0, len(tokens))
	for _, token := range tokens {
		region, err := parseSelectionRegion(token)
		if err != nil {
			return nil, err
		}
		for _, earlier := range regions {
			if earlier.overlaps(region) {
				return nil, fmt.Errorf(
					"%w: the region %q names an ordinal the earlier region %q already names: a "+
						"specification names each ordinal once, as one region", ErrBadSelection,
					region.spell(), earlier.spell())
			}
		}
		regions = append(regions, region)
	}
	return regions, nil
}

// parseSelectionRegion reads one token: `N`, `N-M` or `N-`.
func parseSelectionRegion(token string) (selectionRegion, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return selectionRegion{}, fmt.Errorf(
			"%w: %q is not an ordinal or a region: write values and ranges like 0-5,9,12- (a comma "+
				"between regions)", ErrBadSelection, token)
	}
	lower, upper, hasRange := strings.Cut(trimmed, "-")
	if strings.TrimSpace(lower) == "" {
		// A token like `-1` or `-` cuts at its first dash, leaving an empty lower half: the refusal quotes
		// the whole token the caller wrote, the way every other malformed-token sentence does, rather
		// than the empty half the cut produced.
		return selectionRegion{}, fmt.Errorf(
			"%w: %q is not an ordinal: write values and ranges like 0-5,9,12-", ErrBadSelection, trimmed)
	}
	lo, err := selectionOrdinal(lower)
	if err != nil {
		return selectionRegion{}, err
	}
	if !hasRange {
		return selectionRegion{Lo: lo, Hi: lo}, nil
	}
	if strings.TrimSpace(upper) == "" {
		return selectionRegion{Lo: lo, Hi: lo, open: true}, nil
	}
	hi, err := selectionOrdinal(upper)
	if err != nil {
		return selectionRegion{}, err
	}
	if hi < lo {
		return selectionRegion{}, fmt.Errorf(
			"%w: the region %q is inverted: a range is written low-high, like 0-5", ErrBadSelection, trimmed)
	}
	return selectionRegion{Lo: lo, Hi: hi}, nil
}

// selectionOrdinal reads one ordinal value: a non-negative decimal number, exactly.
func selectionOrdinal(text string) (int, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf(
			"%w: %q holds no ordinal: write values and ranges like 0-5,9,12-", ErrBadSelection, text)
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 0 {
		return 0, fmt.Errorf(
			"%w: %q is not an ordinal: write values and ranges like 0-5,9,12-", ErrBadSelection, trimmed)
	}
	return value, nil
}

// checkSelectionRegions answers whether a well-formed specification names this read's own ordinals: every
// literal endpoint lies inside the space the read produced. An open region's own lower number is what is
// checked; what it reaches is the space's own maximum, so `N-` is fine whenever N is within the space
// while `0-` on a read whose ordinals start at 1 is refused naming the space.
func checkSelectionRegions(regions []selectionRegion, space OrdinalSpace, spelled string) error {
	for _, region := range regions {
		if region.Lo < space.Min || region.Lo > space.Max {
			return fmt.Errorf(
				"%w: the ordinal %d is outside the read's own ordinal values (%s): a specification "+
					"names the values this read supplied, never a position in it", ErrBadSelection,
				region.Lo, spelled)
		}
		// An inverted range is refused at parse time (Hi >= Lo) and Lo is in the space by the guard
		// above, so only the upper endpoint can still fall outside it.
		if !region.open && region.Hi > space.Max {
			return fmt.Errorf(
				"%w: the ordinal %d is outside the read's own ordinal values (%s): a specification "+
					"names the values this read supplied, never a position in it", ErrBadSelection,
				region.Hi, spelled)
		}
	}
	return nil
}

// regionsName answers whether any region covers one ordinal value.
func regionsName(regions []selectionRegion, ordinal int) bool {
	for _, region := range regions {
		if region.names(ordinal) {
			return true
		}
	}
	return false
}

// ordinalSpaceOf answers the space one read produced: its lowest and highest ordinal values, how many
// records it holds, and whether the values are min..max with no gap. Two records sharing an ordinal are
// two records (the count is of messages, like the report's own classes), and the space is what the
// specification is checked against.
func ordinalSpaceOf(messages []Message) OrdinalSpace {
	space := OrdinalSpace{}
	if len(messages) == 0 {
		return space
	}
	seen := make(map[int]struct{}, len(messages))
	space.Min, space.Max = messages[0].Ordinal, messages[0].Ordinal
	for _, message := range messages {
		seen[message.Ordinal] = struct{}{}
		if message.Ordinal < space.Min {
			space.Min = message.Ordinal
		}
		if message.Ordinal > space.Max {
			space.Max = message.Ordinal
		}
	}
	space.Records = len(messages)
	space.Contiguous = space.Max-space.Min+1 == len(seen)
	return space
}

// spaceOrdinals answers the read's ordinal values in ascending order, each once.
func spaceOrdinals(messages []Message) []int {
	seen := make(map[int]struct{}, len(messages))
	values := make([]int, 0, len(messages))
	for _, message := range messages {
		if _, ok := seen[message.Ordinal]; ok {
			continue
		}
		seen[message.Ordinal] = struct{}{}
		values = append(values, message.Ordinal)
	}
	sort.Ints(values)
	return values
}

// ordinalSpaceSentence spells a space the way a refusal names it: the region grammar of the actual
// ordinal values, how many records the read holds, and -- where the values are not the positions 0..n-1,
// because they do not start at 0 or hold a gap -- that the spec names values rather than positions, so a
// caller who wrote a position reads the number the specification should have used.
func ordinalSpaceSentence(space OrdinalSpace, values []int) string {
	positional := " (these are the read's own ordinal values)"
	if space.Min == 0 && space.Contiguous {
		positional = ""
	}
	return fmt.Sprintf("%s: %d record(s)%s", strings.Join(spellOrdinalRuns(values), ","), space.Records, positional)
}

// spellOrdinalRuns spells ordinal values as the specification's own regions, in ascending order and each
// value once: `0-2,5,7-9`. A class that names no ordinal answers an empty list rather than nil, so a
// zero-count class in the report is `"regions": []` -- a consumer reads "names none" from the field
// itself, never from an absent one.
func spellOrdinalRuns(values []int) []string {
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	regions := make([]string, 0, len(sorted))
	if len(sorted) == 0 {
		return regions
	}
	start, end := sorted[0], sorted[0]
	flush := func() {
		switch {
		case start == end:
			regions = append(regions, strconv.Itoa(start))
		default:
			regions = append(regions, strconv.Itoa(start)+"-"+strconv.Itoa(end))
		}
	}
	for _, value := range sorted[1:] {
		if value == end || value == end+1 {
			end = value
			continue
		}
		flush()
		start, end = value, value
	}
	flush()
	return regions
}

// Select applies one selection specification to a read conversation and answers the conversation to
// carry, the report of what the selection did, and any refusal.
//
// It is a pure function of the document: the messages, their own order, their recorded identities and
// relations, their recorded tool calls and results, and the source's own provider label -- which decides
// whether its reader may answer a projection -- and nothing else. It never reads a file, follows a linked
// session or infers project state. The selection matches the ordinal values the read supplied, never a
// position in the read and never the target's post-write numbering; nothing is renumbered, so the
// ordinals a report names are the ones a caller wrote the spec with.
//
// One structure is carried whole or not at all: a recorded call and the recorded result that *answers*
// it are paired from the read's own relations -- a result answers the call the read's own recorded
// ancestry names where it names one (the call the read holds on the result's own parent chain that
// records the same non-empty id), and otherwise the nearest recorded call the same non-empty id records
// before it (pairedCallOccurrences) -- and a selection that would carry one without the other carries
// the counterpart instead, named in NecessitiesAdded with the ordinal that pulled it in. A progress event
// is not an answer: a result whose recorded status says the call is still running is the read's record of
// a call that has not finished, so it is no pair's counterpart, and the call the source left undischarged
// stays as it is and is reported (CallsUndischarged). --strict-selection refuses a separated structure by
// name instead of carrying the counterpart. A counterpart is carried only where the route can represent
// it without changing the disposition the source gave it: where the record is one the read's line leaves
// off, the whole selection is refused by name (ErrSelectionUnrepresentable), because this target's log
// has no parent relation to hang a branch off and writing the record would activate it.
//
// The answers the reduced document can no longer give travel with it, keyed to the records it holds:
// the read's own line membership per record (sourceLineInactive/sourceLineUnresolved, taken before the
// cut), the read's pairing of results to calls (sourceResultPairs), the finality of every carried call
// (sourceCallFinality), the nearest carried ancestor per record (sourceKeptAncestor), and what each
// carried compaction boundary accounts for (sourceBoundaries). A writer and a read-back verifier consume
// those answers instead of re-deriving them from the reduced stream, which cannot distinguish "absent in
// source" from "omitted by selection".
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
	messages := conversation.Messages
	space := ordinalSpaceOf(messages)
	if space.Records == 0 {
		return Conversation{}, SelectionReport{}, fmt.Errorf(
			"%w: the read returned no records, so a specification has no ordinal to name", ErrBadSelection)
	}
	values := spaceOrdinals(messages)
	sentence := ordinalSpaceSentence(space, values)
	if err := checkSelectionRegions(regions, space, sentence); err != nil {
		return Conversation{}, SelectionReport{}, err
	}

	selected := make([]bool, len(messages))
	selectedCount := 0
	for index, message := range messages {
		if regionsName(regions, message.Ordinal) {
			selected[index] = true
			selectedCount++
		}
	}
	if selectedCount == 0 {
		return Conversation{}, SelectionReport{}, fmt.Errorf(
			"%w: every ordinal the specification names falls between the read's own ordinal values "+
				"(%s), so no record would be carried", ErrBadSelection, sentence)
	}

	// The read's own answers, taken before the document is reduced. The line is a property of the read --
	// which records the walk from its last record reaches -- and a subset's own walk would make its last
	// record the line's end; the pairing and the finality are relations the reduced stream can no longer
	// establish, because a record the window left out is indistinguishable there from one the source
	// never recorded.
	lineInactive, lineUnresolved, _ := readLineAnswers(messages, conversation.Source.Provider)
	paired := pairedCallOccurrences(conversation)
	carried, necessityAt, err := selectionClosure(
		conversation, selected, selectionPairs(conversation, paired), strict, lineInactive)
	if err != nil {
		return Conversation{}, SelectionReport{}, err
	}
	finality := callFinalities(conversation, paired)
	ancestors := selectionKeptAncestors(conversation, carried)
	ledgers := boundaryLedgers(conversation, carried)

	reducedMessages := make([]Message, 0, len(messages))
	reducedOf := make([]int, len(messages))
	for index := range reducedOf {
		reducedOf[index] = -1
	}
	for index, message := range messages {
		if !carried[index] {
			continue
		}
		reducedOf[index] = len(reducedMessages)
		reducedMessages = append(reducedMessages, message)
	}

	reduced := Conversation{Source: conversation.Source, Messages: reducedMessages, Fidelity: conversation.Fidelity}
	reduced.sourceLineInactive = make([]bool, len(reducedMessages))
	reduced.sourceLineUnresolved = make([]bool, len(reducedMessages))
	reduced.sourceKeptAncestor = make([]int, len(reducedMessages))
	reduced.sourceResultPairs = make(map[resultOccurrence]callOccurrence, len(paired))
	reduced.sourceExcludedResultOwners = make(map[resultOccurrence]bool)
	reduced.sourceCallRecordedEvents = make(map[callOccurrence]bool)
	reduced.sourceCallFinality = make(map[callOccurrence]callFinality, len(finality))
	reduced.sourceBoundaries = make(map[int]boundaryLedger, len(ledgers))
	for readIndex, at := range reducedOf {
		if at < 0 {
			continue
		}
		reduced.sourceLineInactive[at] = lineInactive[readIndex]
		reduced.sourceLineUnresolved[at] = lineUnresolved[readIndex]
		reduced.sourceKeptAncestor[at] = -1
		if ancestor := ancestors[readIndex]; ancestor >= 0 {
			reduced.sourceKeptAncestor[at] = reducedOf[ancestor]
		}
		if ledger, ok := ledgers[readIndex]; ok {
			reduced.sourceBoundaries[at] = ledger
		}
	}
	// The pairing the read established, split by what the reduced document can still name: a result
	// whose owner is carried keeps its entry (the writer writes it under that call), a result whose owner
	// the selection left out keeps the fact *that it was paired* in its own set (the states the reduced
	// stream would otherwise collapse into "the source recorded no link"), and every carried call the
	// read answered with any event is marked, so the summary question is answered from the read rather
	// than from a scan of the reduced map.
	for place, call := range paired {
		at, callAt := reducedOf[place.message], reducedOf[call.message]
		switch {
		case at < 0:
			// The result itself is not carried, so no writer asks about its owner.
		case callAt < 0:
			reduced.sourceExcludedResultOwners[resultOccurrence{message: at, result: place.result}] = true
		default:
			reduced.sourceResultPairs[resultOccurrence{message: at, result: place.result}] =
				callOccurrence{message: callAt, call: call.call}
		}
		if callAt >= 0 {
			reduced.sourceCallRecordedEvents[callOccurrence{message: callAt, call: call.call}] = true
		}
	}
	for place, answer := range finality {
		if at := reducedOf[place.message]; at >= 0 {
			reduced.sourceCallFinality[callOccurrence{message: at, call: place.call}] = answer
		}
	}

	// The report's rows are built in the document's own order, one per carried record -- the order the
	// reduced document holds its records and the writer's walk answers its fates in -- because the
	// ordinal value cannot identify a row: the read's own text allows two records to share one, and the
	// position is what pairs each row with the record it came from. The undischarged calls are counted
	// per record, so the class and its rows cannot disagree about where they sit and the fate fill can
	// split the class against what the writer published.
	undischargedAt := undischargedPerRecord(messages, carried, finality)
	included := make([]int, 0, selectedCount)
	excluded := make([]int, 0, len(messages)-selectedCount)
	records := make([]SelectionRecord, 0, len(reducedMessages))
	added := make([]SelectionNecessity, 0, len(reducedMessages)-selectedCount)
	for index, message := range messages {
		switch {
		case selected[index]:
			included = append(included, message.Ordinal)
			records = append(records, SelectionRecord{
				Ordinal: message.Ordinal, CallsUndischarged: undischargedAt[index],
			})
		case !carried[index]:
			excluded = append(excluded, message.Ordinal)
		default:
			// Carried because a recorded structure pulled it in: the row names the record that did.
			pulledBy := messages[necessityAt[index]].Ordinal
			records = append(records, SelectionRecord{
				Ordinal: message.Ordinal, Necessity: true, PulledBy: &pulledBy,
				CallsUndischarged: undischargedAt[index],
			})
			added = append(added, SelectionNecessity{Ordinal: message.Ordinal, PulledBy: pulledBy})
		}
	}

	undischarged, undischargedCount := undischargedClass(messages, carried, undischargedAt)
	return reduced, SelectionReport{
		Spec:              trimmed,
		Strict:            strict,
		Space:             space,
		Included:          SelectionClass{Count: len(included), Regions: spellOrdinalRuns(included)},
		Excluded:          SelectionClass{Count: len(excluded), Regions: spellOrdinalRuns(excluded)},
		NecessitiesAdded:  SelectionNecessities{Count: len(added), Added: added},
		CallsUndischarged: SelectionClass{Count: undischargedCount, Regions: spellOrdinalRuns(undischarged)},
		Records:           records,
	}, nil
}

// undischargedPerRecord answers, per read record, how many of its calls the read left without an
// outcome: the calls as the class's own unit, counted per message so the class, the rows and the
// writer's own walk all rest on one number. A call a read never discharged is one whose finality is not
// an outcome the source established (unknown-undischarged and in-progress alike), and only carried
// records count -- a record the selection excluded contributes nothing.
func undischargedPerRecord(
	messages []Message, carried []bool, finality map[callOccurrence]callFinality,
) []int {
	counts := make([]int, len(messages))
	for index, message := range messages {
		if !carried[index] {
			continue
		}
		for callIndex := range message.ToolCalls {
			if !finality[callOccurrence{message: index, call: callIndex}].stateAnOutcome() {
				counts[index]++
			}
		}
	}
	return counts
}

// undischargedClass answers the class from the per-record counts: the calls as the class's own unit, and
// the ordinal values of the records that carry them, each value once.
func undischargedClass(messages []Message, carried []bool, perRecord []int) ([]int, int) {
	var ordinals []int
	count := 0
	for index, message := range messages {
		if !carried[index] || perRecord[index] == 0 {
			continue
		}
		count += perRecord[index]
		ordinals = append(ordinals, message.Ordinal)
	}
	return ordinals, count
}

// stateAnOutcome answers whether a finality is an outcome the source established, as opposed to the
// read's own not-established state (in-progress, unknown).
func (f callFinality) stateAnOutcome() bool {
	switch f {
	case callFinalityCompleted, callFinalityFailed, callFinalityInterrupted:
		return true
	default:
		return false
	}
}

// selectionPair is one recorded call and the recorded result that answers it: the message the result
// occurrence sits on and the message of the call it answers. It is what the seam's structure rule carries
// whole; the id is carried for the refusal's own words.
type selectionPair struct {
	id     string
	call   int
	result int
}

// selectionPairs answers the read's own pairs of answered calls and their results, in one walk of the
// pairing (pairedCallOccurrences) so the closure and the report cannot disagree about which call a
// recorded result answers. A result whose recorded status says the call is still running is no pair: it
// is the read's record of a call that has not finished, and a selection that carried it to stand beside
// the call would present progress as the call's answer.
func selectionPairs(conversation Conversation, paired map[resultOccurrence]callOccurrence) []selectionPair {
	var pairs []selectionPair
	for index, message := range conversation.Messages {
		for resultIndex, result := range message.ToolResults {
			id := strings.TrimSpace(result.ToolUseID)
			if id == "" || !resultStatesAnOutcome(result.Status) {
				continue
			}
			call, ok := paired[resultOccurrence{message: index, result: resultIndex}]
			if !ok {
				continue
			}
			pairs = append(pairs, selectionPair{id: id, call: call.message, result: index})
		}
	}
	return pairs
}

// selectionClosure answers which messages the selection carries: the selected ones, plus the counterparts
// a recorded pair requires. It walks the carried set to a fixed point, because a counterpart can itself
// hold a pair whose other half lies outside the selection, and each addition is recorded at the added
// record's own read position (necessityAt: the index of the record that pulled it in, -1 for every other
// record), because the report's rows pair with the writer's walk by position and two records may share an
// ordinal value. The walk is deterministic: messages are visited in read order, so the same document and
// spec always answer the same report.
//
// With strict, the first separating pair refuses by name instead, before anything is carried and long
// before anything is installed. Without it, a counterpart the read's line leaves inactive refuses the
// whole selection by name too (ErrSelectionUnrepresentable): this target's log is one sequence of records
// with no parent relation, so a record written there becomes model-visible history, and carrying an
// off-line counterpart to satisfy closure would activate a branch the source's conversation left.
func selectionClosure(
	conversation Conversation, selected []bool, pairs []selectionPair, strict bool, lineInactive []bool,
) ([]bool, []int, error) {
	carried := append([]bool(nil), selected...)
	if strict {
		for _, pair := range pairs {
			if carried[pair.call] == carried[pair.result] {
				continue
			}
			return nil, nil, strictSelectionRefusal(conversation, pair, carried[pair.call])
		}
	}
	counterparts := make([][]selectionPair, len(conversation.Messages))
	for _, pair := range pairs {
		counterparts[pair.call] = append(counterparts[pair.call], pair)
		counterparts[pair.result] = append(counterparts[pair.result], pair)
	}
	necessityAt := make([]int, len(conversation.Messages))
	for index := range necessityAt {
		necessityAt[index] = -1
	}
	for {
		changed := false
		for index := range conversation.Messages {
			if !carried[index] {
				continue
			}
			for _, pair := range counterparts[index] {
				member := pair.result
				if pair.call != index {
					member = pair.call
				}
				if carried[member] {
					continue
				}
				if member < len(lineInactive) && lineInactive[member] {
					return nil, nil, unrepresentableCounterpartRefusal(conversation, pair)
				}
				carried[member] = true
				necessityAt[member] = index
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return carried, necessityAt, nil
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

// unrepresentableCounterpartRefusal words the refusal for a counterpart the route cannot carry: the pair
// and both of its ordinals are named, because the caller's next move is to write a spec that holds both
// -- and the record's own off-line place is what makes the whole selection impossible rather than one
// addition unwise. The pair is passed whole rather than the two records: a record can carry a call of its
// own beside the result that answers another call, so reading the roles back off the pulled record's
// ToolCalls is how this sentence would name the wrong record as the call.
func unrepresentableCounterpartRefusal(conversation Conversation, pair selectionPair) error {
	return fmt.Errorf(
		"%w: the call recorded at ordinal %d and the result recorded at ordinal %d are one structure, and "+
			"the record the selection would have to carry beside the one it names is off the line the read's "+
			"last record answers; name a region whose records both stand on that line, or narrow the "+
			"selection", ErrSelectionUnrepresentable, conversation.Messages[pair.call].Ordinal,
		conversation.Messages[pair.result].Ordinal)
}

// callFinalities answers the read's own finality for every call occurrence it holds: completed, failed
// or interrupted where a recorded result states an outcome; in-progress where the read pairs recorded
// events with the call and none of them states an outcome; completed where the call carries the archive's
// own derived summary and the read pairs no recorded event with it (the summary is then the outcome
// evidence the read holds); and unknown-undischarged where the read holds no outcome evidence at all.
// A progress event beside a summary leaves the call in-progress: the summary is a view of that event, not
// an outcome recorded beside it. Where the read recorded more than one outcome-stating result for one
// call, the last one in the read's own order is the answer: the finality is a function of the recorded
// document, never of map iteration.
//
// A conversation a selection produced carries the answers its own read established
// (Conversation.sourceCallFinality), keyed to the reduced document's positions: the window cannot
// re-answer the question, because a result the window left out is indistinguishable there from one the
// source never recorded.
func callFinalities(
	conversation Conversation, paired map[resultOccurrence]callOccurrence,
) map[callOccurrence]callFinality {
	if conversation.selectionReduced() {
		return conversation.sourceCallFinality
	}
	// The events are gathered in the document's own read order -- never by ranging the pairing map, whose
	// iteration order is Go's own -- because a call a read answered more than once (an errored result and
	// a later completion) takes the outcome of the last result the read recorded: the answer is a
	// function of the recorded document, and this walk is what makes it one.
	events := make(map[callOccurrence][]ToolResult)
	for index, message := range conversation.Messages {
		for resultIndex, result := range message.ToolResults {
			place := resultOccurrence{message: index, result: resultIndex}
			if call, ok := paired[place]; ok {
				events[call] = append(events[call], result)
			}
		}
	}
	finality := make(map[callOccurrence]callFinality)
	for index, message := range conversation.Messages {
		for callIndex, call := range message.ToolCalls {
			place := callOccurrence{message: index, call: callIndex}
			outcome := callFinality("")
			progress := false
			for _, result := range events[place] {
				if !resultStatesAnOutcome(result.Status) {
					progress = true
					continue
				}
				outcome = finalityOfStatus(result.Status)
			}
			switch {
			case outcome != "":
				finality[place] = outcome
			case progress:
				finality[place] = callFinalityInProgress
			default:
				if _, ok := derivedResultSummary(call, strings.TrimSpace(call.ToolUseID)); ok {
					finality[place] = callFinalityCompleted
				} else {
					finality[place] = callFinalityUnknown
				}
			}
		}
	}
	return finality
}

// finalityOfStatus maps one recorded result's own status word onto the seam's finality vocabulary. The
// words are the inherited readers' own: a status that is not an outcome is progress (resultStatesAnOutcome),
// a word naming an error is a failure, a word naming a cancellation is an interruption, and every other
// word -- `completed`, a provider's own word, or no status at all -- is a completion. The distinction
// matters to the report, never to the target: the Codex function_call_output has no status field, so a
// written result's status is named by the conversion's own category whichever word it carried.
func finalityOfStatus(status string) callFinality {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "errored", "error", "failed", "failure":
		return callFinalityFailed
	case "interrupted", "cancelled", "canceled", "aborted":
		return callFinalityInterrupted
	default:
		return callFinalityCompleted
	}
}

// callFinality is the seam's answer about one carried call's recorded finality: what the source itself
// established, answered before the reduction. It is one value, never a synonym for *omitted by
// selection*, and no value is ever published as a completion by a writer without the evidence the read
// held for it.
type callFinality string

const (
	callFinalityCompleted   callFinality = "completed"
	callFinalityFailed      callFinality = "failed"
	callFinalityInProgress  callFinality = "in-progress"
	callFinalityInterrupted callFinality = "interrupted"
	// callFinalityUnknown is the read's own "the source did not establish an outcome": no recorded result
	// event and no derived summary answers the call. It is kept apart from in-progress (the read pairs
	// recorded events with the call and none states an outcome) and from every answer above.
	callFinalityUnknown callFinality = "unknown-undischarged"
)

// boundaryLedger is what one compaction boundary record accounts for, as the read answered it. There are
// three answers rather than two: the calls and results the boundary record itself carries (the synthetic
// shape a document-level construction may hold), none (the read's format records no replaced history the
// boundary accounts for -- a Claude transcript's boundary is a text summary with neither), and not
// established (the reader skips the record that holds the replaced history -- the inherited Codex reader
// drops `replacement_history` -- so which calls the boundary replaced is not answerable from the read). A
// writer counts the established ledger; an unestablished one is disclosed as the unknown it is, never
// read as none.
type boundaryLedger struct {
	established bool
	calls       int
	results     int
}

// boundaryLedgers answers, per read record the selection carries, what a compaction boundary accounts
// for. The record's own tool evidence is the ledger where it holds any (the shape a document-level
// construction may carry: a boundary record whose own calls and results are the history it replaced); a
// boundary record with none is *none established* where the read's own reader records the summary alone,
// and *not established* where that reader is known to skip the replaced history the format records.
func boundaryLedgers(conversation Conversation, carried []bool) map[int]boundaryLedger {
	ledgers := make(map[int]boundaryLedger)
	skipsReplacedHistory := boundaryReplacedHistorySkipped(conversation.Source.Provider)
	for index, message := range conversation.Messages {
		if !carried[index] || !message.IsCompactBoundary {
			continue
		}
		if len(message.ToolCalls) > 0 || len(message.ToolResults) > 0 {
			ledgers[index] = boundaryLedger{
				established: true,
				calls:       len(message.ToolCalls),
				results:     len(message.ToolResults),
			}
			continue
		}
		ledgers[index] = boundaryLedger{established: !skipsReplacedHistory}
	}
	return ledgers
}

// boundaryReplacedHistorySkipped answers whether one provider's reader skips the record that holds which
// calls a compaction boundary replaced. It is a property of the *reader*, not of this build's reading of
// the document: the inherited Codex reader drops `replacement_history` deliberately (its own
// handleCompacted copies the summary and not the replaced messages), so a read through it cannot answer
// the boundary's ledger and says so rather than reporting none, while a Claude transcript's boundary is a
// text summary with neither calls nor results and *none* is its own answer.
func boundaryReplacedHistorySkipped(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), string(parser.AgentCodex))
}

// selectionKeptAncestors answers, for every record of the read, the index of the nearest ancestor the
// read holds that the selection also carries, or -1 where there is none: the walk starts at the record's
// own recorded parent and steps until it reaches a carried record, the read runs out of relation, or the
// step budget the document's own size sets ends a cycle the source recorded. The previous-record fallback
// the readers' own line walk takes where the read answered no relation is *not* taken here: a record
// whose relation cannot be walked is answered no ancestor, and a tree writer's own fallback decides its
// place.
//
// The disposition is carried for the routes whose target format is a tree (the later increments): the
// reference route's Codex writer writes one sequence of records with no parent relation, so it chains
// nothing and consumes neither this answer nor the projection detail it would carry. It travels with the
// seam anyway, because a reduction that left a record's parent out of the window is exactly the fact a
// later route must not re-derive.
func selectionKeptAncestors(conversation Conversation, carried []bool) []int {
	byIdentity := make(map[string]int, len(conversation.Messages))
	for index, message := range conversation.Messages {
		if id := strings.TrimSpace(message.SourceUUID); id != "" {
			if _, seen := byIdentity[id]; !seen {
				byIdentity[id] = index
			}
		}
	}
	ancestors := make([]int, len(conversation.Messages))
	for index := range ancestors {
		ancestors[index] = -1
	}
	for index := range conversation.Messages {
		if !carried[index] {
			continue
		}
		at := index
		for steps := 0; steps <= len(conversation.Messages); steps++ {
			parent := strings.TrimSpace(conversation.Messages[at].SourceParentUUID)
			if parent == "" {
				break
			}
			held, ok := byIdentity[parent]
			if !ok || held >= at {
				break
			}
			if carried[held] {
				ancestors[index] = held
				break
			}
			at = held
		}
	}
	return ancestors
}

// fillSelectionRecords pairs the selection's carried records with the writer's own fates: what became
// of each record, the category a lost one was counted under, and the item kinds the target wrote for it.
// The pairing is positional -- the report's rows and the writer's fates are both in the document's own
// order, one per carried record -- because the ordinal value cannot identify a record: the read's own
// text allows two records to share one. A route whose writer keeps no fates, or a walk that did not
// answer one per carried record, is named as that on every row rather than mispaired: the table's own
// contract is that every carried record is accounted for.
func fillSelectionRecords(report *SelectionReport, fates []RecordFate) {
	if len(fates) != len(report.Records) {
		for index := range report.Records {
			report.Records[index].Became = BecameLost
			report.Records[index].Reason = "the writer's walk answered no fate for this record"
		}
		return
	}
	for index := range report.Records {
		record := &report.Records[index]
		fate := fates[index]
		record.Written = record.Written[:0]
		for _, kind := range fate.Items {
			record.Written = append(record.Written, string(kind))
		}
		if len(record.Written) > 0 {
			record.Became = BecameNativeHistory
			continue
		}
		record.Became = BecameLost
		record.Reason = fate.Reason
	}
}

// fillSelectionUndischargedSplit completes the undischarged class from the writer's own fates, which the
// per-record table has just been filled with: a carried record whose written item kinds include the calls
// it carries contributes its undischarged calls to the conversion's outstanding_tool_calls, and a record
// the walk did not publish (no tool_call item, whatever its fate -- a lost record, a record published as
// a compaction boundary alone) carries them into CallsUndischargedLost instead. The split is filled
// after fillSelectionRecords and reads only what that fill wrote, so the class and the table cannot
// disagree; a report whose fates have not been filled answers zero lost, which the field's own wording
// names as an answer not yet made.
func fillSelectionUndischargedSplit(report *SelectionReport) {
	lost := 0
	var ordinals []int
	for _, record := range report.Records {
		if record.CallsUndischarged == 0 {
			continue
		}
		if slices.Contains(record.Written, string(ItemToolCall)) {
			continue
		}
		lost += record.CallsUndischarged
		ordinals = append(ordinals, record.Ordinal)
	}
	report.CallsUndischargedLost = SelectionClass{Count: lost, Regions: spellOrdinalRuns(ordinals)}
}

// lostNecessityRefusal answers the refusal for a counterpart the route's own writer did not publish: the
// selection carried a record because a recorded call and the recorded result that answers it are one
// structure, and the writer's walk made nothing of it -- the fate's reason is the loss category it was
// counted under -- so the pair would be written with half its recorded structure while the report's
// necessity class declared the counterpart carried. D3's closure eligibility refuses such a selection by
// name rather than presenting a broken pair as a whole one, and it does so here, after the walk (the
// only place that knows what each record became) and before anything is installed. It is asked of the
// per-record table, so it uses the very fates the report publishes.
func lostNecessityRefusal(report *SelectionReport) error {
	for _, record := range report.Records {
		if !record.Necessity || record.Became != BecameLost {
			continue
		}
		pulledBy := 0
		if record.PulledBy != nil {
			pulledBy = *record.PulledBy
		}
		reason := record.Reason
		if reason == "" {
			reason = "no target record states this source record"
		}
		return fmt.Errorf(
			"%w: the recorded call and the recorded result at ordinals %d and %d are one structure, "+
				"and the record the selection carried as the counterpart (ordinal %d, pulled in by "+
				"ordinal %d) is not published by this route (%s): the pair would be written with its "+
				"recorded counterpart missing; name a region that holds both records",
			ErrSelectionUnrepresentable, pulledBy, record.Ordinal, record.Ordinal, pulledBy, reason)
	}
	return nil
}

// selectionWarnings names the selection's own answer in the report's warning channel, so a caller who
// reads the human block's head -- or only the warnings -- learns that the history is a window and what
// the window leaves behind rather than reading "converted" as "the whole conversation travelled".
func selectionWarnings(report *SelectionReport) []string {
	if report == nil {
		return nil
	}
	var warnings []string
	if report.Excluded.Count > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"the selection carried %d of the read's %d record(s); %d record(s) at %s did not travel "+
				"(specification %q)", report.Included.Count+report.NecessitiesAdded.Count,
			report.Space.Records, report.Excluded.Count, joinOrNone(report.Excluded.Regions), report.Spec))
	}
	if report.NecessitiesAdded.Count > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"the selection carried %d record(s) the specification did not name, because a recorded call "+
				"and the recorded result that answers it are one structure (ordinals %s)",
			report.NecessitiesAdded.Count, spellNecessityOrdinals(report.NecessitiesAdded.Added)))
	}
	if report.CallsUndischarged.Count > 0 {
		sentence := fmt.Sprintf(
			"the source left %d call(s) undischarged in the carried records (ordinals %s): no recorded "+
				"outcome answers them, and no outcome is written for them", report.CallsUndischarged.Count,
			joinOrNone(report.CallsUndischarged.Regions))
		if report.CallsUndischargedLost.Count > 0 {
			// Only a report whose writer's fates have been filled can answer this: those calls sit on
			// records the conversion did not publish, counted under each record's own category, and a
			// caller reconciling the two surfaces must not read the class as calls the target holds.
			sentence += fmt.Sprintf("; %d of them sit on records the conversion did not publish "+
				"(ordinals %s, counted under those records' own loss categories), so they are not in "+
				"the conversion's outstanding_tool_calls", report.CallsUndischargedLost.Count,
				joinOrNone(report.CallsUndischargedLost.Regions))
		}
		warnings = append(warnings, sentence)
	}
	return warnings
}

// spellNecessityOrdinals spells the ordinals a counterpart pulled in, each with the ordinal that pulled
// it, for the warning sentence.
func spellNecessityOrdinals(added []SelectionNecessity) string {
	parts := make([]string, 0, len(added))
	for _, necessity := range added {
		parts = append(parts, fmt.Sprintf("%d (pulled by %d)", necessity.Ordinal, necessity.PulledBy))
	}
	return strings.Join(parts, ", ")
}

// joinOrNone spells a region list for a sentence, answering "none" for an empty one.
func joinOrNone(regions []string) string {
	if len(regions) == 0 {
		return "none"
	}
	return strings.Join(regions, ",")
}
