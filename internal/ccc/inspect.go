package ccc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
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
	// records on such a message is held by the archive too. The archive stores a projection, so it
	// may hold *fewer* rows than the source -- a tool-result-only row is folded into its tool call and
	// counted in FoldedToolResults -- but it must not hold a different one, and it must not be missing
	// an outcome the source recorded on a row both of them have. It is nil when no source read
	// happened, so a false value is always the result of a comparison that ran.
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
		if report.Fidelity.UnresolvedToolOutputs > 0 {
			inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
				"%d tool result(s) name persisted output this machine could not read; their content is "+
					"the placeholder that says where it was saved, not the output",
				report.Fidelity.UnresolvedToolOutputs))
		}
	case errors.Is(err, ErrNotFound):
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"no readable source session for %s:%s", opts.Ref.Provider, opts.Ref.ID))
	case errors.Is(err, ErrNoLocalSource):
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"%s was recorded on another machine, so this one has no source file to read for it; "+
				"an archive can still hold the session", opts.Ref.ID))
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
// archive does not have.
//
// A message both reads have is compared in full: its role, its content, and the tool results it
// carries. The last of those is the only difference an archive written before a call's outcome arrived
// can have -- the call's row is the same ordinal, role and text as the later source's, and the outcome
// is an event *on* that row (Codex's function_call_output) rather than a row of its own, so a comparison
// that stopped at role and content reported a stale archive as holding the conversation. A result the
// source records on a row the archive also has must be held by that row, wherever the archive keeps it
// (a result row of its own, an event, or the call's own content). Where both reads expose a result's
// body, the bodies are compared too; a body only one of them shows (a blocked category, a writer that
// stored the length and no text) is not a difference this comparison can see.
func compareMessages(source, archived []Message) (bool, int, []string) {
	byOrdinal := make(map[int]Message, len(source))
	for _, message := range source {
		byOrdinal[message.Ordinal] = message
	}
	archivedOrdinals := make(map[int]struct{}, len(archived))
	results := archivedResultKeys(archived)
	same, folded := true, 0
	var warnings []string
	for _, message := range archived {
		archivedOrdinals[message.Ordinal] = struct{}{}
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
		if missing := unheldResults(
			origin.ToolResults, archivedResultKeys([]Message{message})); len(missing) > 0 {
			same = false
			warnings = append(warnings, fmt.Sprintf(
				"archive message #%d does not hold the tool result(s) the source records on it: %s",
				message.Ordinal, strings.Join(missing, ", ")))
		}
		for _, differing := range differingResultBodies(
			message.Ordinal, origin.ToolResults, message.ToolResults) {
			same = false
			warnings = append(warnings, differing)
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
// the source read has the body -- and the size is what tells two rows of one id apart.
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
		if result.ToolUseID != "" {
			missing = append(missing, result.ToolUseID)
			continue
		}
		missing = append(missing,
			fmt.Sprintf("a %d-byte result with no tool-use id", result.ContentSize))
	}
	return missing
}

// differingResultBodies names the tool results both reads show a body for but disagree about. Only the
// results the *archived message itself* carries are compared: a body the archive kept on the call
// instead (a summarized call row) is one the source read never shows a body for, and the result key --
// the call and the length -- cannot tell a stored summary from the tool's own output.
func differingResultBodies(ordinal int, source, archived []ToolResult) []string {
	bodies := make(map[resultKey]string, len(archived))
	for _, result := range archived {
		if result.ContentRaw != "" {
			bodies[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}] = result.ContentRaw
		}
	}
	var warnings []string
	for _, result := range source {
		if result.ContentRaw == "" {
			continue
		}
		body, ok := bodies[resultKey{toolUseID: result.ToolUseID, size: result.ContentSize}]
		if !ok || body == result.ContentRaw {
			continue
		}
		name := result.ToolUseID
		if name == "" {
			name = fmt.Sprintf("a %d-byte result with no tool-use id", result.ContentSize)
		}
		warnings = append(warnings, fmt.Sprintf(
			"archive message #%d holds a different body for tool result %s", ordinal, name))
	}
	return warnings
}

// contentWarnings names what an archive read could not show, because a caller that treats a narrowed
// archive read as the whole conversation would be reading a different session than the one recorded.
//
// Each warning states what the read shows, never which storage policy wrote the archive: a read-only
// handle cannot see that policy, so an archive that dropped tool payloads and a session that recorded
// none read the same way, and a message claiming one of them would report an omission that may never
// have happened.
func contentWarnings(report ArchiveReport) []string {
	switch report.ContentShown {
	case "message-text":
		return []string{
			"the archive read shows no tool inputs or results: whether the archive dropped them or " +
				"the session recorded none is not visible to a read",
		}
	case "none":
		warning := "the archive read shows no message text and no tool inputs or results"
		if report.Status.MessageCount > report.Counts.Messages {
			warning += fmt.Sprintf(
				"; it returns %d of the %d message(s) the session row records, so whole messages are "+
					"not in this archive", report.Counts.Messages, report.Status.MessageCount)
		}
		return []string{warning}
	default:
		return nil
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
