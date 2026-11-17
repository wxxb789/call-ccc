package ccc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/stringutil"
)

// firstMessageBytes bounds the preview an inspection reports for a session's opening user message.
// The full text stays in the Conversation; this is display text and is truncated at a rune boundary.
const firstMessageBytes = 240

// InspectOptions selects what an inspection reads. Reader and Archive are optional: the zero value
// reads the source session only, and ArchivePath makes the inspection consult an archive too.
type InspectOptions struct {
	Ref Ref

	// Config supplies the provider roots and enabled agents. Zero value means "ask the environment":
	// the reader falls back to the inherited default configuration. The same roots are what a *relative*
	// filesystem reference is resolved against when an archive is consulted, so a caller that supplies
	// only a Reader should supply the configuration it built that reader from.
	Config config.Config

	// Reader, when set, is used instead of building one from Config.
	Reader *SourceReader

	// ArchivePath is the SQLite archive to consult. Empty means "do not consult an archive".
	ArchivePath string
}

// SessionReport summarises one read of a session: its provenance, its counts and the few facts a
// caller needs before deciding what to do with it. It carries no message bodies beyond the opening
// prompt preview, so an inspection never dumps a conversation into a log.
type SessionReport struct {
	Identity         SourceIdentity `json:"identity"`
	Counts           Counts         `json:"counts"`
	ToolNames        []string       `json:"tool_names,omitempty"`
	FirstUserMessage string         `json:"first_user_message,omitempty"`
	StartedAt        time.Time      `json:"started_at,omitempty"`
	EndedAt          time.Time      `json:"ended_at,omitempty"`
	Fidelity         Fidelity       `json:"fidelity"`
}

// ArchiveReport is what the archive holds, plus how it compares with the source read.
type ArchiveReport struct {
	Status ArchiveStatus `json:"status"`
	Counts Counts        `json:"counts"`
	// ContentShown names what the read of the archive itself shows -- its tool payloads and message
	// text (see contentShown in archive.go) -- and not the storage policy the writer enforced, which
	// a read-only handle cannot see.
	ContentShown string `json:"content_shown,omitempty"`
	// SameMessagesAsSource reports that every message the archive holds also appears in the source
	// read at the same ordinal with the same role and content, and that each tool result the source
	// records on such a message is held by the archive too, with the same body and the same recorded
	// outcome wherever both reads show one. The archive stores a projection, so it may hold *fewer*
	// rows than the source -- a tool-result-only row is folded into its tool call and counted in
	// FoldedToolResults -- but it must not hold a different one, and it must not be missing an outcome
	// the source recorded on a row both of them have. It is nil when no source read happened, so a
	// false value is always the result of a comparison that ran.
	SameMessagesAsSource *bool `json:"same_messages_as_source"`
	// Warnings names every difference this comparison found.
	Warnings []string `json:"warnings,omitempty"`
	// FoldedToolResults counts source rows the archive does not keep as messages because their
	// content lives on the tool call instead.
	FoldedToolResults int `json:"folded_tool_results,omitempty"`
}

// Inspection is the result of inspecting one session.
type Inspection struct {
	Ref    Ref            `json:"ref"`
	Agent  string         `json:"agent,omitempty"`
	Source *SessionReport `json:"source,omitempty"`
	// ArchivePath is the archive that was consulted, whether or not it exists.
	ArchivePath string         `json:"archive_path,omitempty"`
	Archive     *ArchiveReport `json:"archive,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
}

// Inspect reads a session through inherited AgentsView code and reports what it found: identity,
// agent, source, conversation shape, and — when an archive is given — whether that archive holds the
// same session. It is the entry point the CLI calls, and it takes no CLI type.
//
// A source the reader cannot read is a warning rather than an error whenever an archive could still
// answer the reference: a session recorded on another machine (ErrNoLocalSource), a provider that
// failed operationally (ErrProviderFailed), a provider the configuration disables for local
// ingestion (ErrSourceDisabled) and a source the provider could not read in full (ErrPartialSource)
// all leave the archive in reach, and each warning carries the reader's own answer so the four are
// distinguishable. A reference no provider knows, or a failure none of those classifications covers,
// is returned as an error.
func Inspect(ctx context.Context, opts InspectOptions) (Inspection, error) {
	// The configuration answers where each provider keeps its sessions. The reader resolves a relative
	// filesystem reference against those roots and the archive is opened with the same answer, so both
	// lookups normalize a reference the same way; a caller that supplied a reader has already answered
	// it, and its answer is the one the source side is using.
	cfg := opts.Config
	reader := opts.Reader
	switch {
	case cfg.AgentDirs == nil && reader != nil:
		cfg = reader.cfg
	case cfg.AgentDirs == nil:
		loaded, err := config.LoadReadOnly()
		if err != nil {
			return Inspection{}, fmt.Errorf("reading the AgentsView configuration: %w", err)
		}
		cfg = loaded
	}
	if reader == nil {
		reader = NewSourceReader(cfg)
	}

	inspection := Inspection{Ref: opts.Ref, ArchivePath: opts.ArchivePath}
	if label, ok := reader.AgentLabel(opts.Ref.Provider); ok {
		inspection.Agent = label
	}

	var sourceMessages []Message
	conversation, err := reader.Read(ctx, opts.Ref)
	switch {
	case err == nil:
		report := reportFrom(conversation)
		inspection.Source = &report
		sourceMessages = conversation.Messages
		inspection.Warnings = append(inspection.Warnings, fidelityWarnings(report.Fidelity)...)
	case errors.Is(err, ErrNotFound):
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"no readable source session for %s:%s", opts.Ref.Provider, opts.Ref.ID))
	case errors.Is(err, ErrNoLocalSource):
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"%s was recorded on another machine, so this one has no source file to read for it; "+
				"an archive can still hold the session", opts.Ref.ID))
	case errors.Is(err, ErrProviderFailed):
		// The provider failed operationally, which is not the same answer as "no such session": the
		// reader's error carries the provider's own diagnostics, and the archive is still consulted --
		// a session the source side cannot read may well be one an archive holds.
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"%v; an archive can still hold the session", err))
	case errors.Is(err, ErrPartialSource):
		// The provider read the source but not in full. That is neither absence nor a session: the
		// members it did read exist, and which conversation a *path* names is exactly what an
		// incomplete read cannot say (the reader refuses to pick one, reader.go's partialPathParse).
		// The warning carries the provider's diagnostics and the identities it did read, so a caller
		// can name one of them, and the archive is still consulted for the session the reference
		// could name.
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"%v; an archive can still hold the session", err))
	case errors.Is(err, ErrSourceDisabled):
		// The configuration disables this provider for local ingestion. That is the provider's own
		// state, not the session's: the inherited setting ("Disabling Session Providers",
		// docs/configuration.md) restricts local discovery and ingestion only and keeps previously
		// archived sessions available, so the archive is still consulted instead of answering the
		// reference with an error. The warning carries the reader's own phrase, so a caller can tell a
		// disabled provider from an absent one and from an absent session.
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"%v; the setting restricts local discovery only, so an archive can still hold the session", err))
	default:
		return Inspection{}, err
	}

	if opts.ArchivePath != "" {
		archive, err := openArchive(ctx, opts.ArchivePath, cfg.AgentDirs)
		switch {
		case errors.Is(err, ErrNoArchive):
			inspection.Warnings = append(inspection.Warnings,
				"no archive at "+opts.ArchivePath+"; reporting source facts only")
		case err != nil:
			return Inspection{}, err
		default:
			defer archive.Close()
			report, found, err := archiveReport(ctx, archive, opts.Ref, sourceMessages, inspection.Source != nil)
			if err != nil {
				return Inspection{}, err
			}
			if found {
				inspection.Archive = &report
				inspection.Warnings = append(inspection.Warnings, contentWarnings(report)...)
				inspection.Warnings = append(inspection.Warnings, report.Warnings...)
			} else if inspection.Source != nil {
				inspection.Warnings = append(inspection.Warnings,
					fmt.Sprintf("the source session is not in %s yet", archive.Path()))
			}
		}
	}
	if inspection.Agent == "" {
		// The provider name this reference used has no registry def of its own (a stored agent label such
		// as Freebuff), so the provider lookup above had no display name to answer with. The session the
		// inspection did read recorded one for itself -- the parse's own label, or the label the writer
		// stored on the archived row -- and that is the agent an operator reads this inspection for. A
		// name this lookup made up would be a second source of truth for an inherited label.
		switch {
		case inspection.Source != nil:
			inspection.Agent = inspection.Source.Identity.Agent
		case inspection.Archive != nil:
			inspection.Agent = inspection.Archive.Status.AgentLabel
		}
	}
	return inspection, nil
}

// archiveReport is built from the read archive plus, when the source was readable (sourceRead), the
// comparison of the two message sets.
func archiveReport(
	ctx context.Context, archive *Archive, ref Ref, sourceMessages []Message, sourceRead bool,
) (ArchiveReport, bool, error) {
	status, found, err := archive.Status(ctx, ref)
	if err != nil || !found {
		return ArchiveReport{}, false, err
	}
	conversation, err := archive.Read(ctx, ref)
	if err != nil {
		return ArchiveReport{}, false, err
	}
	report := ArchiveReport{
		Status:       status,
		Counts:       Summarize(conversation.Messages),
		ContentShown: conversation.Fidelity.ContentShown,
	}
	if sourceRead {
		same, folded, warnings := compareMessages(sourceMessages, conversation.Messages)
		report.SameMessagesAsSource = &same
		report.FoldedToolResults = folded
		report.Warnings = warnings
	}
	if report.FoldedToolResults > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"%d tool-result-only message(s) are stored in the archive folded into their tool call",
			report.FoldedToolResults))
	}
	return report, true, nil
}

// compareMessages reports whether every archived message also appears in the source at the same
// ordinal, and names each message that does not. Messages whose content the archive stores elsewhere
// (a tool-result-only row, folded into its call) are counted, not treated as a mismatch -- but only
// when the archive really holds that result: a stale archive that stopped before the result arrived
// keeps its call and no outcome, and counting the source's row as folded would report evidence the
// archive does not have. A folded result is held by key -- the call and the content length -- and its
// body and recorded outcome are compared too, wherever the archive keeps them: two same-length outcomes
// are one key, so a pair of different outcomes, or a pair that disagree about whether the call failed,
// would otherwise pass as agreement.
//
// A message both reads have is compared in full: its role, its content, the tool calls it records, and
// the tool results it carries. The last of those is the only difference an archive written before a
// call's outcome arrived can have -- the call's row is the same ordinal, role and text as the later
// source's, and the outcome is an event *on* that row (Codex's function_call_output) rather than a row
// of its own, so a comparison that stopped at role and content reported a stale archive as holding the
// conversation. A result the source records on a row the archive also has must be held by that row,
// wherever the archive keeps it (a result row of its own, an event, or the call's own content), and a
// result the archive's own rows record must be held by the source read, so neither read can hold
// evidence the other lost. Where both reads expose a result's body, the bodies are compared too; a body
// only one of them shows (a blocked category, a writer that stored the length and no text) is not a
// difference this comparison can see. The outcomes the two reads record for a result are compared
// wherever the archive shows a row that carries one, in both directions: a read that records an error
// the other does not is a difference either way round.
//
// A call is compared the same way: both reads keep the provider's own call identities, so a call only
// the archive holds is a difference, and a call both hold is compared by name and by the input each
// retained -- a Claude `Edit` renders as `[Edit: <file_path>]` in the message text, so two edits of the
// same file reach the message comparison identical while the inputs differ.
func compareMessages(source, archived []Message) (bool, int, []string) {
	byOrdinal := make(map[int]Message, len(source))
	for _, message := range source {
		byOrdinal[message.Ordinal] = message
	}
	archivedOrdinals := make(map[int]struct{}, len(archived))
	for _, message := range archived {
		archivedOrdinals[message.Ordinal] = struct{}{}
	}
	results := archivedResultKeys(archived)
	foldedSources := foldedSourceResults(source, archivedOrdinals)
	foldedBodies := archivedResultBodies(archived)
	foldedStatuses := archivedResultStatuses(archived)
	same, folded := true, 0
	var warnings []string
	for _, message := range archived {
		origin, ok := byOrdinal[message.Ordinal]
		if !ok {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d is not in the source read", message.Ordinal))
			continue
		}
		if origin.Role != message.Role || origin.Content != message.Content {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d differs from the source (role %q/%q)",
				message.Ordinal, origin.Role, message.Role))
		}
		if problems := differingCallProblems(origin.ToolCalls, message.ToolCalls); len(problems) > 0 {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d disagrees with the source about its tool call(s): %s",
				message.Ordinal, strings.Join(problems, "; ")))
		}
		if missing := unheldResults(
			origin.ToolResults, archivedResultKeys([]Message{message})); len(missing) > 0 {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d does not hold the tool result(s) the source records on it: %s",
				message.Ordinal, strings.Join(missing, ", ")))
		}
		if missing := unheldResults(
			message.ToolResults, heldSourceResults(origin.ToolResults, foldedSources)); len(missing) > 0 {
			// The other direction, which the loop above cannot see: the archive's own result rows are
			// evidence this read recorded, and a source that no longer records them is a different
			// conversation from the archived one -- a transcript whose result row is gone, or one read
			// from a source that lost the outcome the archive kept.
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d holds the tool result(s) the source read does not: %s",
				message.Ordinal, strings.Join(missing, ", ")))
		}
		if names := differingResultBodyNames(origin.ToolResults, messageResultBodies(message)); len(names) > 0 {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d holds a different body for tool result %s",
				message.Ordinal, strings.Join(names, ", ")))
		}
		if names := differingResultOutcomeNames(
			origin.ToolResults, messageResultStatuses(message)); len(names) > 0 {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d records a different outcome for tool result %s",
				message.Ordinal, strings.Join(names, "; ")))
		}
	}
	for _, message := range source {
		if _, ok := archivedOrdinals[message.Ordinal]; ok {
			continue
		}
		if len(message.ToolResults) > 0 && strings.TrimSpace(message.Content) == "" {
			missing := unheldResults(message.ToolResults, results)
			if len(missing) == 0 {
				folded++
				// The row is held, but holding it is not agreement: the archive's body and outcome for
				// the same call and length can still be different ones, and a folded row is exactly
				// where the two reads stop sharing an ordinal, so nothing else compares them.
				if names := differingResultBodyNames(message.ToolResults, foldedBodies); len(names) > 0 {
					same = false
					warnings = append(warnings, fmt.Sprintf(
						"the archive holds a different body for tool result %s of source message #%d",
						strings.Join(names, ", "), message.Ordinal))
				}
				if names := differingResultOutcomeNames(
					message.ToolResults, foldedStatuses); len(names) > 0 {
					same = false
					warnings = append(warnings, fmt.Sprintf(
						"the archive records a different outcome for tool result %s of source message #%d",
						strings.Join(names, "; "), message.Ordinal))
				}
				continue
			}
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"source message #%d carries tool result(s) the archive does not hold: %s",
				message.Ordinal, strings.Join(missing, ", ")))
			continue
		}
		same = false
		warnings = append(warnings, fmt.Sprintf(
			"source message #%d is not in the archive", message.Ordinal))
	}
	return same, folded, warnings
}

// resultKey identifies a tool result the way both reads spell it: the call it answers and the size of
// the content that came back. The body is deliberately not part of it -- an archive read may show a
// result whose body its writer did not store (a blocked category keeps the length and a digest) while
// the source read has the body -- and the size is what tells two rows of one id apart. Bodies are
// compared separately, keyed by this pair (differingResultBodyNames), wherever both reads show one, and
// so are the outcomes the two reads record (differingResultOutcomeNames).
type resultKey struct {
	toolUseID string
	size      int
}

// archivedResultKeys collects every tool result the archived messages hold, wherever the archive keeps
// it: as a result row of its own, as an event folded onto its call, or as the content a call row
// carries directly (a call whose outcome the archive stored on the call and nowhere else).
func archivedResultKeys(messages []Message) map[resultKey]struct{} {
	keys := make(map[resultKey]struct{})
	for _, message := range messages {
		for _, result := range message.ToolResults {
			keys[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}] = struct{}{}
		}
		for _, call := range message.ToolCalls {
			if call.ResultContent != "" || call.ResultContentSize > 0 {
				keys[resultKey{toolUseID: call.ToolUseID, size: call.ResultContentSize}] = struct{}{}
			}
		}
	}
	return keys
}

// unheldResults names the tool results of a source message the archived messages do not hold, so a
// row whose content is stored elsewhere can be told from one the archive never received.
func unheldResults(results []ToolResult, archived map[resultKey]struct{}) []string {
	var missing []string
	for _, result := range results {
		if _, ok := archived[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}]; ok {
			continue
		}
		missing = append(missing, resultName(result))
	}
	return missing
}

// resultName names a tool result the way a warning can: its tool-use id, or its size when the provider
// recorded none.
func resultName(result ToolResult) string {
	if result.ToolUseID != "" {
		return result.ToolUseID
	}
	return fmt.Sprintf("a %d-byte result with no tool-use id", result.ContentSize)
}

// foldedSourceResults collects the results the source read records on rows the archive keeps no message
// for: the tool-result-only rows the inherited write path folds onto their call. Those rows have no
// ordinal in the archive -- it starts at the call that carries their content -- so their evidence can
// only be recognized by key, wherever the archive keeps the same result. A source row the archive
// dropped that carries text of its own adds its results here too: the row itself is already a named
// difference, and its keys only widen what the archive's own rows are accepted against.
func foldedSourceResults(source []Message, archivedOrdinals map[int]struct{}) map[resultKey]struct{} {
	keys := make(map[resultKey]struct{})
	for _, message := range source {
		if _, ok := archivedOrdinals[message.Ordinal]; ok {
			continue
		}
		for _, result := range message.ToolResults {
			keys[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}] = struct{}{}
		}
	}
	return keys
}

// heldSourceResults is the set of results the source read holds for one archive message: the ones the
// source records on the message at that ordinal, plus the ones it records on the rows the archive
// folded away. An archive row's result is compared against both, because folding is exactly where the
// two reads stop sharing an ordinal.
func heldSourceResults(onMessage []ToolResult, folded map[resultKey]struct{}) map[resultKey]struct{} {
	if len(onMessage) == 0 {
		return folded
	}
	held := make(map[resultKey]struct{}, len(folded)+len(onMessage))
	for key := range folded {
		held[key] = struct{}{}
	}
	for _, result := range onMessage {
		held[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}] = struct{}{}
	}
	return held
}

// differingCallProblems names the ways the archived message's tool calls disagree with the source
// message's, in the same two directions the result comparison reads: a call the archive holds and the
// source read does not, and -- for a call both reads hold -- a name or an input they disagree about.
// Identity matters because a call is what a tool result is attached to: a message whose calls differ is
// a different message even when its rendered text is the same.
//
// Calls are paired by the tool-use id both reads carry from the same provider field, so a call is
// identified by what its provider called it rather than by its position. A provider that records no id
// leaves position as the only identity there is, so id-less calls are paired in the order each read
// listed them.
//
// The name and the input are compared only where both reads retain them: the storage projection keeps
// call names and blanks inputs (internal/db's transcriptMessages), so an input the archive does not
// show is not one it disagrees about, and reading it as a difference would report a difference the
// archive's own read cannot support. Where both show one, they are compared in the inherited writer's
// own spelling (db.SanitizeUTF8, the normalisation internal/sync's parsediff applies when it decides
// whether a stored call still matches its parse), so a control byte the writer stripped is not
// reported as a changed input.
func differingCallProblems(source, archived []ToolCall) []string {
	byKey := make(map[string]ToolCall, len(source))
	for i, key := range callKeys(source) {
		byKey[key] = source[i]
	}
	var problems []string
	for i, key := range callKeys(archived) {
		call := archived[i]
		origin, ok := byKey[key]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"the archive holds a tool call the source read does not: %s", callName(call)))
			continue
		}
		if origin.Name != "" && call.Name != "" && origin.Name != call.Name {
			problems = append(problems, fmt.Sprintf(
				"tool call %s is named %q in the archive and %q in the source",
				callName(call), call.Name, origin.Name))
		}
		if origin.InputJSON != "" && call.InputJSON != "" &&
			db.SanitizeUTF8(origin.InputJSON) != db.SanitizeUTF8(call.InputJSON) {
			problems = append(problems, fmt.Sprintf(
				"tool call %s carries a different input in the archive than in the source", callName(call)))
		}
	}
	return problems
}

// callKeys pairs calls the way both reads identify a message's calls: the provider's own tool-use id
// when the read recorded one, and otherwise the call's position among the message's id-less calls,
// ordered the way that read ordered them.
func callKeys(calls []ToolCall) []string {
	keys := make([]string, len(calls))
	anonymous := 0
	for i, call := range calls {
		if call.ToolUseID != "" {
			keys[i] = "id:" + call.ToolUseID
			continue
		}
		keys[i] = "at:" + strconv.Itoa(anonymous)
		anonymous++
	}
	return keys
}

// callName names a tool call for a warning, from whichever of the two fields the provider recorded.
func callName(call ToolCall) string {
	switch {
	case call.ToolUseID != "" && call.Name != "":
		return call.ToolUseID + " (" + call.Name + ")"
	case call.ToolUseID != "":
		return call.ToolUseID
	case call.Name != "":
		return fmt.Sprintf("the %s call with no id", call.Name)
	default:
		return "a call with no id and no name"
	}
}

// messageResultBodies collects the bodies the archived *message itself* carries, keyed by resultKey.
// A call's own result content is deliberately not part of this scope: the archive may store a
// *summarized* call row whose text the source row never showed, and comparing a source body against a
// summary would report a difference the archive's own read cannot support. A result the archive kept as
// an event folded onto the call is in the message's own result rows and is compared.
func messageResultBodies(message Message) map[resultKey][]string {
	bodies := make(map[resultKey][]string)
	collectResultBodies(bodies, message.ToolResults)
	return bodies
}

// archivedResultBodies collects every body the archived messages show for a result key, wherever the
// archive keeps it: a result row of its own, an event folded onto its call, or the call row's own
// result content. It is the scope of the folded comparison -- the source row that carried the result
// has no ordinal in the archive, so its body can only be found somewhere else in the read.
func archivedResultBodies(messages []Message) map[resultKey][]string {
	bodies := make(map[resultKey][]string)
	for _, message := range messages {
		collectResultBodies(bodies, message.ToolResults)
		for _, call := range message.ToolCalls {
			if call.ResultContent != "" {
				key := resultKey{toolUseID: call.ToolUseID, size: call.ResultContentSize}
				bodies[key] = append(bodies[key], call.ResultContent)
			}
		}
	}
	return bodies
}

// collectResultBodies adds the non-empty bodies of the given result rows to the map. A body the archive
// narrowed away (a blocked category keeps the length and no text) adds nothing, so a key is present only
// when the archive really shows a body for it -- which is what keeps an absent body from being read as a
// difference.
func collectResultBodies(into map[resultKey][]string, results []ToolResult) {
	for _, result := range results {
		if result.ContentRaw == "" {
			continue
		}
		key := resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}
		into[key] = append(into[key], result.ContentRaw)
	}
}

// differingResultBodyNames names the source's tool results whose bodies the given archived bodies
// disagree about: a body is shown for the result's key and none of them is the source's own. Several
// bodies can share one key (a call row's content and the event it was restored from), so one match is
// agreement.
//
// Both spellings of the source's body are accepted: the archive stores the result's *text*, decoding
// the provider's raw JSON content field on the way in (parser.DecodeContent -- the step
// internal/ingest's pairer and internal/sync's incremental writer both apply), while a source read
// carries that raw field itself, so a body stored in either representation is recognized as the same
// body rather than reported as a difference.
func differingResultBodyNames(source []ToolResult, shown map[resultKey][]string) []string {
	var names []string
	for _, result := range source {
		if result.ContentRaw == "" {
			continue
		}
		bodies := shown[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}]
		if len(bodies) == 0 || resultBodyMatches(bodies, result.ContentRaw) {
			continue
		}
		names = append(names, resultName(result))
	}
	return names
}

// resultBodyMatches reports whether any body the archive shows for a result is the source's own body,
// in either representation: the raw content field as the provider wrote it, or its decoded text.
func resultBodyMatches(archived []string, raw string) bool {
	decoded := parser.DecodeContent(raw)
	for _, body := range archived {
		if body == raw || (decoded != "" && body == decoded) {
			return true
		}
	}
	return false
}

// messageResultStatuses collects the outcomes the archived *message itself* records, keyed by
// resultKey. It is the same scope messageResultBodies reads -- the message's own result rows, which is
// where an archive read materializes a call's folded result events -- so a call's own summary is out of
// it as well.
func messageResultStatuses(message Message) map[resultKey][]string {
	statuses := make(map[resultKey][]string)
	collectResultStatuses(statuses, message.ToolResults)
	return statuses
}

// archivedResultStatuses collects the outcomes the archived messages record, keyed by resultKey,
// wherever the archive keeps a result row: a result row of its own, or the event it folded onto the
// call. A key is present only when the archive really shows such a row: a call row's own result content
// is not one -- the archive stores no status on the call -- so a key the archive holds only there has
// no outcome to compare, and leaves the comparison to the bodies.
func archivedResultStatuses(messages []Message) map[resultKey][]string {
	statuses := make(map[resultKey][]string)
	for _, message := range messages {
		collectResultStatuses(statuses, message.ToolResults)
	}
	return statuses
}

// collectResultStatuses adds each result row's recorded status to the map. An empty status is added
// too: it is what a read records for a result its provider did not mark as an error, so it is a value
// the two reads must agree about rather than an absence to look past.
func collectResultStatuses(into map[resultKey][]string, results []ToolResult) {
	for _, result := range results {
		key := resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}
		into[key] = append(into[key], result.Status)
	}
}

// differingResultOutcomeNames names the source's tool results whose recorded *outcome* the archive's
// rows disagree about, each with what the two reads record, so a warning can say what differed.
//
// The comparison is of sets, because one key can be held by several rows (an event delivered twice, a
// result row beside the event it was restored from, one row per agent), and it is symmetric, because
// either read can be the one that records a failure: an archived result recorded as errored against a
// source result that now reads as a success is the same difference as the reverse, and the claim that
// every archived message matches the source read must carry neither. A result whose key the archive
// holds only without a status-bearing row (a call row's own content) is skipped -- the archive shows no
// outcome for it, which is the allowance the body comparison makes as well -- while a key the archive
// does show an outcome for must agree about it.
func differingResultOutcomeNames(source []ToolResult, shown map[resultKey][]string) []string {
	var names []string
	sourceStatuses := make(map[resultKey][]string, len(source))
	named := make(map[resultKey]string, len(source))
	keys := make([]resultKey, 0, len(source))
	for _, result := range source {
		key := resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}
		if _, seen := named[key]; !seen {
			named[key] = resultName(result)
			keys = append(keys, key)
		}
		sourceStatuses[key] = append(sourceStatuses[key], result.Status)
	}
	for _, key := range keys {
		archived, ok := shown[key]
		if !ok || statusSetsAgree(archived, sourceStatuses[key]) {
			continue
		}
		names = append(names, fmt.Sprintf("%s (the archive records %s, the source records %s)",
			named[key], outcomeSummary(archived), outcomeSummary(sourceStatuses[key])))
	}
	return names
}

// statusSetsAgree reports whether the statuses one read records for a result are exactly the ones the
// other read records: every status the archive shows is one the source records, and every status the
// source records is one the archive shows.
func statusSetsAgree(archived, source []string) bool {
	recorded := make(map[string]struct{}, len(source))
	for _, status := range source {
		recorded[status] = struct{}{}
	}
	shown := make(map[string]struct{}, len(archived))
	for _, status := range archived {
		shown[status] = struct{}{}
		if _, ok := recorded[status]; !ok {
			return false
		}
	}
	for status := range recorded {
		if _, ok := shown[status]; !ok {
			return false
		}
	}
	return true
}

// outcomeSummary names the outcomes a read records for one result, for a warning: the distinct statuses
// in a stable order, with the empty one spelled out as "no status". A read records that value for a
// result its provider did not mark as an error, and a warning that printed nothing for it would read as
// if the read had answered nothing at all.
func outcomeSummary(statuses []string) string {
	seen := make(map[string]struct{}, len(statuses))
	distinct := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		if status == "" {
			distinct = append(distinct, "no status")
			continue
		}
		distinct = append(distinct, strconv.Quote(status))
	}
	sort.Strings(distinct)
	return strings.Join(distinct, ", ")
}

// contentWarnings names what an archive read could not show, because a caller that treats a narrowed
// archive read as the whole conversation would be reading a different session than the one recorded.
//
// Each warning states what the read shows, never which storage policy wrote the archive: a read-only
// handle cannot see that policy, so an archive that dropped tool payloads and a session that recorded
// none read the same way, and a message claiming one of them would report an omission that may never
// have happened. The stored transcript classification is the exception, and it is a fact about the row
// rather than about a policy: the writer recorded what the provider said about the read it stored, and
// a session whose provider could only produce a degraded fallback reads as that fallback.
func contentWarnings(report ArchiveReport) []string {
	var warnings []string
	if report.Status.TranscriptFidelity == parser.TranscriptFidelitySummary {
		warnings = append(warnings,
			"the stored transcript is classified as a summary fallback, so the archive holds a "+
				"degraded read of this session rather than its recorded transcript")
	}
	switch report.ContentShown {
	case "message-text":
		return append(warnings, "the archive read shows no tool inputs or results: whether the archive "+
			"dropped them or the session recorded none is not visible to a read")
	case "none":
		warning := "the archive read shows no message text and no tool inputs or results"
		if report.Status.MessageCount > report.Counts.Messages {
			warning += fmt.Sprintf(
				"; it returns %d of the %d message(s) the session row records, so whole messages are "+
					"not in this archive", report.Counts.Messages, report.Status.MessageCount)
		}
		return append(warnings, warning)
	default:
		return warnings
	}
}

// reportFrom summarises a conversation.
func reportFrom(conversation Conversation) SessionReport {
	report := SessionReport{
		Identity: conversation.Source,
		Counts:   Summarize(conversation.Messages),
		Fidelity: conversation.Fidelity,
	}
	names := map[string]struct{}{}
	for _, message := range conversation.Messages {
		for _, call := range message.ToolCalls {
			if call.Name != "" {
				names[call.Name] = struct{}{}
			}
		}
		if report.FirstUserMessage == "" && message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			report.FirstUserMessage = stringutil.SafeTruncate(message.Content, firstMessageBytes)
		}
		if !message.Timestamp.IsZero() {
			if report.StartedAt.IsZero() || message.Timestamp.Before(report.StartedAt) {
				report.StartedAt = message.Timestamp
			}
			if message.Timestamp.After(report.EndedAt) {
				report.EndedAt = message.Timestamp
			}
		}
	}
	if len(names) > 0 {
		report.ToolNames = make([]string, 0, len(names))
		for name := range names {
			report.ToolNames = append(report.ToolNames, name)
		}
		sort.Strings(report.ToolNames)
	}
	return report
}
