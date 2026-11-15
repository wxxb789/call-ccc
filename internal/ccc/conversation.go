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
	// ResultContent is the outcome the archive stores on the call itself. The archive folds a
	// tool-result-only message into its call, so this field is how an archive read carries that
	// evidence; a source read carries it as a Message with ToolResults instead.
	ResultContent     string `json:"result_content,omitempty"`
	ResultContentSize int    `json:"result_content_size,omitempty"`
}

// ToolResult is the recorded outcome of a tool call, kept apart from the call so a translation can
// report a result it cannot carry instead of inlining it as assistant prose. ContentRaw is the
// provider's own raw content field for a tool-result block, or the content text of a result event the
// provider attached to its call; Source says which kind it is, and both reads map it the same way.
type ToolResult struct {
	ToolUseID   string `json:"tool_use_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Source      string `json:"source,omitempty"`
	ContentRaw  string `json:"content_raw,omitempty"`
	ContentSize int    `json:"content_size,omitempty"`
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
	Ordinal     int          `json:"ordinal"`
	Role        string       `json:"role"`
	Content     string       `json:"content,omitempty"`
	Dialogue    string       `json:"dialogue,omitempty"`
	Thinking    string       `json:"thinking,omitempty"`
	Timestamp   time.Time    `json:"timestamp,omitempty"`
	Model       string       `json:"model,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
	// IsSystem marks a record the provider itself flagged as system-injected (a compaction summary,
	// a notice). It is independent of Role: a compaction summary is recorded as an assistant record.
	IsSystem          bool `json:"is_system,omitempty"`
	IsSidechain       bool `json:"is_sidechain,omitempty"`
	IsCompactBoundary bool `json:"is_compact_boundary,omitempty"`
}

// Fidelity records what the source itself said about the completeness of this read. A caller that
// reports "converted" while MalformedLines is non-zero is misreporting.
type Fidelity struct {
	MalformedLines int    `json:"malformed_lines,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	SourceVersion  string `json:"source_version,omitempty"`
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

// Counts summarises a conversation without re-reading its messages.
type Counts struct {
	Messages          int `json:"messages"`
	UserMessages      int `json:"user_messages"`
	AssistantMessages int `json:"assistant_messages"`
	SystemMessages    int `json:"system_messages"`
	// SystemRecords counts records the provider flagged as system-injected; those are also counted
	// under the role they were recorded with, so the two are not disjoint.
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
		case parser.RoleSystem, parser.RoleTool:
			counts.SystemMessages++
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
