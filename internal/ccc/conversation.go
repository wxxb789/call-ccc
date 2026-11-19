// Package ccc is the call/ccc continuation core.
//
// It reads a native agent session into a typed, transient Conversation document and reads the same
// session back out of the AgentsView SQLite archive, both through inherited AgentsView code: source
// discovery and parsing come from go.kenn.io/agentsview/internal/parser, the archive write path from
// internal/sync, and the archive read path from internal/db. Nothing here reimplements a source
// format, opens SQLite directly, or keeps a second index.
//
// The package deliberately imports no CLI, terminal or HTTP package and holds no process-global
// state, so a future embedder (an AgentsView command, a test harness, another tool) can call it
// directly. cmd/ccc does argument handling and presentation on top of it.
//
// Writes exist only where a caller names them: OpenArchive opens read-only, and NewIngestor takes an
// explicit database path instead of a default user data directory. That keeps a live AgentsView
// archive out of reach of an accidental conversion, which is what the design's non-destructive rule
// asks for.
package ccc

import (
	"fmt"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// Ref identifies one source session: a provider and that provider's identifier for the session (a
// native session id, or a filesystem path when the caller has one).
type Ref struct {
	Provider string
	ID       string
}

// SourceIdentity is the provenance of a session: who recorded it, where it lives, and what the
// provider's own metadata says about it. It is carried through the document so a later target writer
// can decide what it can express instead of guessing from the transcript text.
type SourceIdentity struct {
	Provider         string    `json:"provider"`
	Agent            string    `json:"agent,omitempty"`
	SessionID        string    `json:"session_id"`
	RawSessionID     string    `json:"raw_session_id,omitempty"`
	SourcePath       string    `json:"source_path,omitempty"`
	DisplayPath      string    `json:"display_path,omitempty"`
	Project          string    `json:"project,omitempty"`
	Cwd              string    `json:"cwd,omitempty"`
	GitBranch        string    `json:"git_branch,omitempty"`
	GitRemote        string    `json:"git_remote,omitempty"`
	Machine          string    `json:"machine,omitempty"`
	SessionKind      string    `json:"session_kind,omitempty"`
	Entrypoint       string    `json:"entrypoint,omitempty"`
	ParentSessionID  string    `json:"parent_session_id,omitempty"`
	RelationshipType string    `json:"relationship_type,omitempty"`
	SourceVersion    string    `json:"source_version,omitempty"`
	Fingerprint      string    `json:"fingerprint,omitempty"`
	FileSize         int64     `json:"file_size,omitempty"`
	FileModified     time.Time `json:"file_modified,omitempty"`
}

// ToolCall is one tool invocation recorded on a message. InputJSON is the provider's own raw input
// object; a writer that cannot express it must say so rather than emitting a synthesized one.
type ToolCall struct {
	ToolUseID         string `json:"tool_use_id,omitempty"`
	Name              string `json:"name,omitempty"`
	Category          string `json:"category,omitempty"`
	InputJSON         string `json:"input_json,omitempty"`
	FilePath          string `json:"file_path,omitempty"`
	SkillName         string `json:"skill_name,omitempty"`
	SubagentSessionID string `json:"subagent_session_id,omitempty"`
	// ResultContent is the summary the archive stores on the call itself. The inherited write path
	// derives it from the call's ResultEvents (internal/db stores it only when it is not the sole
	// content-bearing event's text, and a read restores it from that event when the column was cleared),
	// so beside those events it is a view of them rather than a separately recorded outcome -- which is
	// why a writer carries it only for a call with no recorded result of its own. It is then the only
	// evidence the read holds of the outcome (an archive row whose events were not written); a source
	// read carries a call's outcome as a Message with ToolResults instead.
	ResultContent     string `json:"result_content,omitempty"`
	ResultContentSize int    `json:"result_content_size,omitempty"`
}

// ToolResult is the recorded outcome of a tool call, kept apart from the call so a translation can
// report a result it cannot carry instead of inlining it as assistant prose. ContentRaw is the
// provider's own raw content field for a tool-result block, or the content text of a result event the
// provider attached to its call; Source says which kind it is, and both reads map it the same way.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Source    string `json:"source,omitempty"`
	// Timestamp is the instant of the record the result itself was recorded on, where the read holds
	// one: a result event the provider attached to its call carries the event's own timestamp
	// (parser.ParsedToolResultEvent.Timestamp), which may be later than the call's own record, and a
	// result block recorded inside a message carries that message's own instant (messageFromParsed). It
	// is distinct from the call's instant: a writer that states a call's span needs both, and taking the
	// call's record for the completion turns a recorded span into one the source never recorded (round
	// 12's finding). Zero means the read holds no instant of the result's own record -- an event the
	// provider recorded without one, an archive row whose stored event carries none -- and a writer then
	// falls back to the containing record's instant and says so rather than presenting the fallback as
	// the recorded one.
	Timestamp   time.Time `json:"timestamp,omitempty"`
	ContentRaw  string    `json:"content_raw,omitempty"`
	ContentSize int       `json:"content_size,omitempty"`
	// ContentDecoded says which representation ContentRaw holds. False (the zero value) means the
	// provider's own content field as it recorded it -- JSON, a string or an array of blocks -- which a
	// writer decodes once through the inherited reader. True means text a read has already decoded (a
	// result event's content, an archive row), which must never be decoded again: a tool whose own
	// output is literally a JSON string would otherwise lose its quotation marks.
	ContentDecoded bool `json:"content_decoded,omitempty"`
	// PersistedOutputUnresolved carries the provider's own parse-time answer that this result's content
	// is the *placeholder* naming a side file the parser could not read, not the output itself
	// (parser.ParsedToolResult.PersistedOutputUnresolved). ContentRaw then holds the placeholder text,
	// which is indistinguishable from genuine output unless this flag is read: a caller that presents
	// such a result as the tool's output would report text the tool never produced. Only a source read
	// can answer it -- the archive projection stores no such flag -- so it is always false on an
	// archive read.
	PersistedOutputUnresolved bool `json:"persisted_output_unresolved,omitempty"`
}

// Message is one ordered turn of a conversation. Thinking and dialogue text are the provider's
// persisted reasoning and side text where the source format records them; they are carried as
// evidence, never as authority to replay anything.
type Message struct {
	Ordinal int    `json:"ordinal"`
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	// Dialogue is what the provider said, without the renderings of the blocks that are not dialogue.
	// It is the read's own per-record answer where the read made one (parser.ParsedMessage.DialogueText,
	// carried as DialogueProjected below) and the text a caller recorded directly otherwise, which is
	// what dialogueOf answers from: this field is the authority for a record's dialogue, and Content is
	// never consulted beside it.
	Dialogue string `json:"dialogue,omitempty"`
	// DialogueProjected says whether the source answered the dialogue question for this message at all.
	// True means Dialogue is the source's own projection, an explicitly empty one included -- the
	// source's own statement that the record has no dialogue to carry, which is an exact conversion of a
	// record with none. False means the read answered nothing, and the text is then decided by the
	// record's own evidence (dialogueOf): Content where the reader records Content as the source wrote
	// it, no dialogue where Content is exactly the reader's rendering of the record's whole trace, and
	// nothing established where Content is not the record's own bytes at all. "Explicitly empty" and
	// "unanswered" are different states and are never collapsed into one.
	DialogueProjected bool `json:"dialogue_projected,omitempty"`
	// ContentRendersThinking says whether the reader that produced Content builds the record's persisted
	// reasoning trace into it as a `[Thinking]…[/Thinking]` block (readerRendersThinkingTrace, held
	// against the carried readers). It is the one reader-level fact the dialogue question needs: a record
	// this reader produced that carries a trace has no text the read can establish, because the reader's
	// rendering is written from the trace's own text and the record's own words are one flattened string
	// with it -- two different native part orders produce the same Content and the same trace. False
	// means no such rendering is established for this reader: either it records Content as the source
	// wrote it (internal/parser's Vibe reader copies the record's content and reasoning_content
	// independently) or this build knows no reader for its label, and in both cases Content travels as
	// the record's text while the report says that is what it is (unprojected_dialogue).
	ContentRendersThinking bool         `json:"content_renders_thinking,omitempty"`
	Thinking               string       `json:"thinking,omitempty"`
	Timestamp              time.Time    `json:"timestamp,omitempty"`
	Model                  string       `json:"model,omitempty"`
	ToolCalls              []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults            []ToolResult `json:"tool_results,omitempty"`
	// IsSystem marks a record the provider itself flagged as system-injected (a compaction summary,
	// a notice). It is independent of Role: a compaction summary is recorded as an assistant record.
	IsSystem          bool `json:"is_system,omitempty"`
	IsSidechain       bool `json:"is_sidechain,omitempty"`
	IsCompactBoundary bool `json:"is_compact_boundary,omitempty"`
	// SourceUUID is the provider's own identity for this record and SourceParentUUID the record it
	// hangs off, both empty where the provider records neither (a Codex rollout has no per-message
	// identity). They are the read's own answer about the source's structure, not a second parse: a
	// provider whose records form a tree -- a Pi session's `id`/`parentId`, a Claude transcript's
	// `uuid`/`parentUuid` -- answers them per record, and a target format that is itself a tree needs
	// them to write the source's own shape instead of inventing one flat chain. SourceParentUUID is the
	// provider's *visible* ancestor where the parser resolves one (a Pi tool result is not one: a record
	// that follows a tool result names the message that made the call), which is why a writer that wants
	// the source's line must ask the document's own order as well.
	SourceUUID       string `json:"source_uuid,omitempty"`
	SourceParentUUID string `json:"source_parent_uuid,omitempty"`
}

// Fidelity records what the source itself said about the completeness of this read. A caller that
// reports "converted" while MalformedLines is non-zero is misreporting.
type Fidelity struct {
	MalformedLines int    `json:"malformed_lines,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	SourceVersion  string `json:"source_version,omitempty"`
	// TranscriptFidelity is the provider's own classification of the transcript this read produced:
	// parser.TranscriptFidelityFull for a transcript read from the source, or
	// parser.TranscriptFidelitySummary for a degraded fallback that stands in for one the provider
	// could not read (history rows, a heuristic decode). Empty means the provider signalled nothing,
	// which upstream's own contract treats as full. A caller that reported a summary read as the
	// session's recorded transcript would overstate it. Both reads carry it: a source read from the
	// parse, an archive read from the row the writer stored it on.
	TranscriptFidelity string `json:"transcript_fidelity,omitempty"`
	// NeedsRetry and RetryReason are the provider's own answer that the result it returned is not
	// current for this parser's data version (parser.DataVersionNeedsRetry) and why
	// (parser.ParseResultOutcome.RetryReason). A result can carry a usable conversation and still need
	// one: a Codex fork whose parent page cannot be read keeps the copied parent messages until that
	// parent resolves. The messages are reported *and* this state beside them, never instead of them.
	// Only a source read can answer it; an archive row stores no such state.
	NeedsRetry  bool   `json:"needs_retry,omitempty"`
	RetryReason string `json:"retry_reason,omitempty"`
	// UnresolvedToolOutputs counts the tool results whose persisted output the provider could not read,
	// so their content is the placeholder that names it (ToolResult.PersistedOutputUnresolved). The
	// source said so; the aggregate is here so a summary can report it without walking the messages.
	// Zero on an archive read, which cannot answer the question at all.
	UnresolvedToolOutputs int `json:"unresolved_tool_outputs,omitempty"`
	// ReadFrom names where the bytes came from: "source" for the provider's own reader, "archive"
	// for the AgentsView SQLite projection.
	ReadFrom string `json:"read_from"`
	// ContentShown names what an archive read shows, not the storage policy the writer enforced: a
	// read-only handle cannot see that policy, so the value describes the rows this read returned.
	// "tool-payloads": at least one tool input or result body is visible. "message-text": message text
	// is visible and no tool payload is. "none": neither is visible. Empty for a source read.
	ContentShown string `json:"content_shown,omitempty"`
}

// Conversation is the transient interchange document: ordered messages with their tool pairing and
// the provenance needed to decide what a target format can carry. It is never persisted as a second
// database.
type Conversation struct {
	Source   SourceIdentity `json:"source"`
	Messages []Message      `json:"messages"`
	Fidelity Fidelity       `json:"fidelity"`
}

// fidelityWarnings names what the provider itself said about the completeness of one read, in the words
// every report of a read owes its caller. An inspection and a continuation both present the conversation
// as what the source recorded, and three of the provider's own classifications say the read is not that:
// a summary fallback standing in for a transcript the provider could not read, a result the provider
// says is not current for its data version, and tool outputs whose persisted content this machine could
// not read. The warnings are the report's own channel beside the fidelity block, which carries the same
// classifications as fields.
func fidelityWarnings(fidelity Fidelity) []string {
	var warnings []string
	if fidelity.NeedsRetry {
		// The provider returned a conversation *and* said the result is not current: a Codex fork whose
		// parent page cannot be read keeps the copied parent messages until that parent resolves, so the
		// messages are reported while their session boundary is named as unresolved. A report that
		// printed the conversation alone would present those copied parent turns as this session's own
		// recorded history.
		reason := ""
		if fidelity.RetryReason != "" {
			reason = " (" + fidelity.RetryReason + ")"
		}
		warnings = append(warnings, fmt.Sprintf(
			"the provider classified this read as needing a retry%s; it returned the messages it "+
				"could read, which may not be the session's complete recorded history",
			reason))
	}
	if fidelity.TranscriptFidelity == parser.TranscriptFidelitySummary {
		warnings = append(warnings,
			"the provider classified this read as a summary transcript: it is a degraded fallback "+
				"for a source it could not read in full, not the session's recorded transcript")
	}
	if fidelity.UnresolvedToolOutputs > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%d tool result(s) name persisted output this machine could not read; their content is "+
				"the placeholder that says where it was saved, not the output",
			fidelity.UnresolvedToolOutputs))
	}
	return warnings
}

// Counts summarises a conversation without re-reading its messages.
type Counts struct {
	Messages          int `json:"messages"`
	UserMessages      int `json:"user_messages"`
	AssistantMessages int `json:"assistant_messages"`
	SystemMessages    int `json:"system_messages"`
	// ToolMessages counts messages a provider recorded with the native tool role (parser.RoleTool: a
	// standalone tool-result record, which the inherited readers preserve -- internal/parser's Devin
	// reader maps a native `"tool"` record to it and its own test pins that). It is a separate count
	// because it is a separate role: no record was written as a system message, so a caller that read
	// the system count alone would report a session shape the source does not have.
	ToolMessages int `json:"tool_messages"`
	// SystemRecords counts records the provider flagged as system-injected; those are also counted
	// under the role they were recorded with, so the two are not disjoint -- and they are not the same
	// thing as SystemMessages, which counts the records *written with* the system role.
	SystemRecords     int `json:"system_records"`
	SidechainMessages int `json:"sidechain_messages"`
	CompactBoundaries int `json:"compact_boundaries"`
	ToolCalls         int `json:"tool_calls"`
	ToolResults       int `json:"tool_results"`
}

// Summarize counts roles, tool use and boundaries in one pass.
func Summarize(messages []Message) Counts {
	var counts Counts
	counts.Messages = len(messages)
	for _, message := range messages {
		switch parser.RoleType(message.Role) {
		case parser.RoleUser:
			counts.UserMessages++
		case parser.RoleAssistant:
			counts.AssistantMessages++
		case parser.RoleSystem:
			counts.SystemMessages++
		case parser.RoleTool:
			counts.ToolMessages++
		}
		if message.IsSystem {
			counts.SystemRecords++
		}
		if message.IsSidechain {
			counts.SidechainMessages++
		}
		if message.IsCompactBoundary {
			counts.CompactBoundaries++
		}
		counts.ToolCalls += len(message.ToolCalls)
		counts.ToolResults += len(message.ToolResults)
	}
	return counts
}
