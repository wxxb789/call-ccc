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
	// the reader falls back to the inherited default configuration.
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
	// SameMessagesAsSource reports that every message the archive holds also appears in the source
	// read at the same ordinal with the same role and content. The archive stores a projection, so it
	// may hold *fewer* rows than the source -- a tool-result-only row is folded into its tool call and
	// counted in FoldedToolResults -- but it must not hold a different one.
	SameMessagesAsSource bool `json:"same_messages_as_source"`
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
	reader := opts.Reader
	if reader == nil {
		cfg := opts.Config
		if cfg.AgentDirs == nil {
			loaded, err := config.LoadReadOnly()
			if err != nil {
				return Inspection{}, fmt.Errorf("reading the AgentsView configuration: %w", err)
			}
			cfg = loaded
		}
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
	case errors.Is(err, ErrNotFound):
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"no readable source session for %s:%s", opts.Ref.Provider, opts.Ref.ID))
	default:
		return Inspection{}, err
	}

	if opts.ArchivePath != "" {
		archive, err := OpenArchive(ctx, opts.ArchivePath)
		switch {
		case errors.Is(err, ErrNoArchive):
			inspection.Warnings = append(inspection.Warnings,
				"no archive at "+opts.ArchivePath+"; reporting source facts only")
		case err != nil:
			return Inspection{}, err
		default:
			defer archive.Close()
			report, found, err := archiveReport(ctx, archive, opts.Ref, sourceMessages)
			if err != nil {
				return Inspection{}, err
			}
			if found {
				inspection.Archive = &report
				inspection.Warnings = append(inspection.Warnings, projectionWarnings(report)...)
				inspection.Warnings = append(inspection.Warnings, report.Warnings...)
			} else if inspection.Source != nil {
				inspection.Warnings = append(inspection.Warnings,
					fmt.Sprintf("the source session is not in %s yet", archive.Path()))
			}
		}
	}
	return inspection, nil
}

// report is built from the read archive plus, when the source was readable, the comparison of the
// two message sets.
func archiveReport(
	ctx context.Context, archive *Archive, ref Ref, sourceMessages []Message,
) (ArchiveReport, bool, error) {
	status, found, err := archive.Status(ctx, ref)
	if err != nil || !found {
		return ArchiveReport{}, false, err
	}
	conversation, err := archive.Read(ctx, ref)
	if err != nil {
		return ArchiveReport{}, false, err
	}
	report := ArchiveReport{Status: status, Counts: Summarize(conversation.Messages)}
	if sourceMessages != nil {
		report.SameMessagesAsSource, report.FoldedToolResults, report.Warnings =
			compareMessages(sourceMessages, conversation.Messages)
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
// (a tool-result-only row, folded into its call) are counted, not treated as a mismatch.
func compareMessages(source, archived []Message) (bool, int, []string) {
	byOrdinal := make(map[int]Message, len(source))
	for _, message := range source {
		byOrdinal[message.Ordinal] = message
	}
	archivedOrdinals := make(map[int]struct{}, len(archived))
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
	}
	for _, message := range source {
		if _, ok := archivedOrdinals[message.Ordinal]; ok {
			continue
		}
		if len(message.ToolResults) > 0 && strings.TrimSpace(message.Content) == "" {
			folded++
			continue
		}
		same = false
		warnings = append(warnings, fmt.Sprintf(
			"source message #%d is not in the archive", message.Ordinal))
	}
	return same, folded, warnings
}

// projectionWarnings names what an archive's storage policy dropped, because a caller that treats a
// transcripts or usage projection as the whole conversation would be reading a different session
// than the one that was recorded.
func projectionWarnings(report ArchiveReport) []string {
	switch report.Status.Projection {
	case "", "full":
		return nil
	case "transcripts":
		return []string{"archive stores the transcripts projection: tool inputs and results are not in it"}
	case "usage":
		return []string{"archive stores the usage projection: message bodies are not in it"}
	default:
		return []string{"archive stores the " + report.Status.Projection + " projection"}
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
