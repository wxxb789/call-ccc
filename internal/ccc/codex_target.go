package ccc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.kenn.io/agentsview/internal/parser"
)

// CodexFormatVersion is the Codex CLI version whose rollout reader this writer's output was verified
// against (docs/evidence/0002-claude-to-codex-continuation.md). The reader requires the rollout's
// session_meta to carry a cli_version; when the target binary cannot be probed, this is the value
// written, and the report says so ("writer-pin") instead of pretending the host's version was read.
const CodexFormatVersion = "0.169.0"

// CodexTargetFormat names the on-disk format a Codex target session is written in: the "portable"
// rollout JSONL the target still discovers, reads, resumes and migrates, not its paginated SQLite
// store, which belongs to the target harness (a second writer of it would be a competing writer to a
// live Codex home).
const CodexTargetFormat = "codex-rollout-jsonl"

// Conversion dispositions. A disposition states what happened to a category of source evidence, so a
// caller can never read "converted" as "everything travelled".
const (
	// DispositionOmitted means the evidence is not in the target at all.
	DispositionOmitted = "omitted"
	// DispositionCarriedWithCaveat means the evidence is in the target but its meaning is narrower or
	// different there, and the entry's detail says how.
	DispositionCarriedWithCaveat = "carried-with-caveat"
)

// Loss is one category of source evidence a conversion did not carry unchanged. Every category the
// writer knows is reported on every run, including the ones whose count is zero, so a consumer never
// has to infer what was left out.
type Loss struct {
	Category    string `json:"category"`
	Disposition string `json:"disposition"`
	Count       int    `json:"count"`
	Detail      string `json:"detail"`
}

// Carried counts the evidence the writer put into the target.
type Carried struct {
	Messages             int `json:"messages"`
	UserMessages         int `json:"user_messages"`
	AssistantMessages    int `json:"assistant_messages"`
	ToolCalls            int `json:"tool_calls"`
	ToolResults          int `json:"tool_results"`
	CompactionBoundaries int `json:"compaction_boundaries"`
	Turns                int `json:"turns"`
	Bytes                int `json:"bytes"`
}

// TargetItemKind names one piece of evidence as it was written into the target. The round-trip check
// compares this list with what the inherited reader makes of the installed file, which is the only
// way to tell "the writer meant to carry this" from "the file really holds it".
type TargetItemKind string

const (
	ItemUser               TargetItemKind = "user_message"
	ItemAssistant          TargetItemKind = "assistant_message"
	ItemToolCall           TargetItemKind = "tool_call"
	ItemToolResult         TargetItemKind = "tool_result"
	ItemCompactionBoundary TargetItemKind = "compaction_boundary"
)

// TargetItem is one written piece of evidence.
type TargetItem struct {
	Kind TargetItemKind `json:"kind"`
	// Text is the dialogue or summary text of a message item.
	Text string `json:"text,omitempty"`
	// CallID, ToolName and InputJSON describe a tool call; CallID and Text the result of one.
	CallID    string `json:"call_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	InputJSON string `json:"input_json,omitempty"`
}

// CodexRollout is one converted target session, in memory and ready to install. Data is the exact
// byte string the target harness reads; Items is what the writer placed in it, for the round-trip
// check, and is never serialized into a CLI document.
type CodexRollout struct {
	SessionID  string       `json:"session_id"`
	CLIVersion string       `json:"cli_version"`
	FileName   string       `json:"file_name"`
	Directory  string       `json:"directory"`
	Cwd        string       `json:"cwd,omitempty"`
	StartedAt  time.Time    `json:"started_at,omitempty"`
	Data       []byte       `json:"-"`
	Items      []TargetItem `json:"-"`
	Carried    Carried      `json:"carried"`
	Losses     []Loss       `json:"losses"`
}

// CodexTargetOptions configures one conversion. SessionID and CLIVersion are normally left empty: the
// identity must be fresh and the version is probed or pinned by the caller.
type CodexTargetOptions struct {
	// SessionID is the target session's identity. Empty means "generate a fresh one" (UUID v7), which
	// is what a conversion wants; tests and repeat runs pass an explicit value.
	SessionID string
	// CLIVersion is the value written to session_meta.cli_version, which the target's reader requires.
	CLIVersion string
	// Now is the conversion instant, used for identities and for a source that recorded no times.
	Now time.Time
	// Provider is the target harness's own configured model provider, recorded as the session's
	// provider binding. The target's loader refuses a session whose recorded provider (or lack of one)
	// does not match the configuration it is resumed with, and a home that has already migrated to
	// paginated threads refuses one that records none at all; the value therefore comes from the
	// target's own configuration answer, never from the source session.
	Provider string
	// MaxToolNames bounds how many distinct tool names the report spells out, so converting a session
	// with hundreds of tools cannot produce an unbounded document.
	MaxToolNames int
}

// codexEnvelope is one rollout line: an envelope with a timestamp and a payload. The target's reader
// requires the envelope's timestamp to parse, and the payload's own shape identifies the record.
type codexEnvelope struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   any    `json:"payload"`
}

type codexSessionMetaPayload struct {
	ID            string    `json:"id"`
	Timestamp     string    `json:"timestamp"`
	Cwd           string    `json:"cwd,omitempty"`
	Originator    string    `json:"originator"`
	CLIVersion    string    `json:"cli_version"`
	Source        string    `json:"source"`
	ThreadSource  string    `json:"thread_source,omitempty"`
	ModelProvider string    `json:"model_provider,omitempty"`
	Git           *codexGit `json:"git,omitempty"`
}

// codexGit carries the source session's repository provenance. commit_hash is deliberately absent:
// the source document records no commit, and inventing one would state which revision the
// conversation happened at.
type codexGit struct {
	Branch        string `json:"branch,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
}

type codexTurnContextPayload struct {
	TurnID string `json:"turn_id"`
	Cwd    string `json:"cwd,omitempty"`
}

type codexMessagePayload struct {
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []codexContent `json:"content"`
}

type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexFunctionCallPayload struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
}

type codexFunctionCallOutputPayload struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type codexCompactedPayload struct {
	Message string `json:"message"`
}

// codexUserMessagePayload and codexAgentMessagePayload are the target's own UI events for a turn's
// messages. The target writes both a response_item (the record its model history is built from) and one
// of these (the record its own view of the conversation is built from), and its view shows nothing for
// a rollout that holds only the response_item — measured: a written session without these events loads
// and resumes, but the harness's own thread view of it is empty.
type codexUserMessagePayload struct {
	Type         string `json:"type"`
	Message      string `json:"message"`
	Images       []any  `json:"images"`
	LocalImages  []any  `json:"local_images"`
	TextElements []any  `json:"text_elements"`
}

type codexAgentMessagePayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Record type names, as the target writes them.
const (
	codexRecordSessionMeta   = "session_meta"
	codexRecordTurnContext   = "turn_context"
	codexRecordResponseItem  = "response_item"
	codexRecordEventMsg      = "event_msg"
	codexRecordCompacted     = "compacted"
	codexEventUserMessage    = "user_message"
	codexEventAgentMessage   = "agent_message"
	codexItemMessage         = "message"
	codexItemFunctionCall    = "function_call"
	codexItemFunctionOutput  = "function_call_output"
	codexRoleUser            = "user"
	codexRoleAssistant       = "assistant"
	codexContentInputText    = "input_text"
	codexContentOutputText   = "output_text"
	codexOriginator          = "ccc"
	codexSourceCLI           = "cli"
	codexThreadSourceUser    = "user"
	codexGeneratedCallPrefix = "call_ccc_"
	codexGeneratedTurnPrefix = "turn_ccc_"
)

// defaultMaxToolNames bounds how many distinct tool names a report spells out by default.
const defaultMaxToolNames = 12

// ConvertToCodexRollout turns a Conversation into a Codex rollout JSONL session: a fresh identity, one
// envelope per record, and the report of what could not be carried. It writes no file; installation is
// the caller's step (InstallCodexRollout).
func ConvertToCodexRollout(conv Conversation, opts CodexTargetOptions) (CodexRollout, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if opts.MaxToolNames <= 0 {
		opts.MaxToolNames = defaultMaxToolNames
	}

	sessionID := opts.SessionID
	if sessionID == "" {
		generated, err := uuid.NewV7()
		if err != nil {
			return CodexRollout{}, fmt.Errorf("generating a target session id: %w", err)
		}
		sessionID = generated.String()
	}
	cliVersion := strings.TrimSpace(opts.CLIVersion)
	if cliVersion == "" {
		cliVersion = CodexFormatVersion
	}

	source := conv.Source
	startedAt := source.FileModified.UTC()
	if startedAt.IsZero() {
		startedAt = now
	}
	if first := firstRecordedTime(conv.Messages); !first.IsZero() && first.Before(startedAt) {
		startedAt = first
	}

	builder := &codexRolloutBuilder{
		sessionID:        sessionID,
		cwd:              source.Cwd,
		cliVersion:       cliVersion,
		clock:            newEntryClock(startedAt, time.RFC3339Nano),
		gitBranch:        source.GitBranch,
		gitRemote:        source.GitRemote,
		callOwners:       map[string]struct{}{},
		selectionReduced: conv.selectionReduced(),
		opts:             opts,
		sourceFidelity:   conv.Fidelity,
	}
	// One encoder writes every line: JSONL is one object per line, and the encoder's own newline is
	// what separates them. HTML escaping is off because the target stores the conversation's text as it
	// was recorded, not as JSON would quote it for a browser.
	builder.encoder = json.NewEncoder(&builder.buffer)
	builder.encoder.SetEscapeHTML(false)
	builder.sessionMeta(startedAt)
	builder.writeConversation(conv)
	carried := builder.carried
	carried.Bytes = builder.buffer.Len()
	rollout := CodexRollout{
		SessionID:  sessionID,
		CLIVersion: cliVersion,
		FileName:   codexRolloutFileName(sessionID, startedAt),
		Directory:  codexRolloutDirectory(startedAt),
		Cwd:        source.Cwd,
		StartedAt:  startedAt,
		Data:       bytes.Clone(builder.buffer.Bytes()),
		Items:      builder.items,
		Carried:    carried,
		Losses:     builder.losses(),
	}
	return rollout, nil
}

// firstRecordedTime is the earliest timestamp the source recorded, so a session whose file has no
// mtime still starts where its conversation does.
func firstRecordedTime(messages []Message) time.Time {
	var first time.Time
	for _, message := range messages {
		if message.Timestamp.IsZero() {
			continue
		}
		if first.IsZero() || message.Timestamp.Before(first) {
			first = message.Timestamp
		}
	}
	return first.UTC()
}

// codexRolloutBuilder accumulates the rollout's lines and the report while it walks the document.
type codexRolloutBuilder struct {
	buffer     bytes.Buffer
	encoder    *json.Encoder
	sessionID  string
	cwd        string
	cliVersion string
	clock      *entryClock
	opts       CodexTargetOptions
	gitBranch  string
	gitRemote  string

	items   []TargetItem
	carried Carried
	// messageIndex is the place of the message being written in the document's walk. It keys the id
	// generated for a call that recorded none (toolCall): a message's own ordinal is not unique when an
	// embedding caller never assigned one, and two calls sharing a generated id would read as one call.
	messageIndex int
	// pairings answers, per recorded result occurrence of the document this conversion walks, the call
	// occurrence the seam's own order pairs it with (pairedCallOccurrences), and writtenCalls names the
	// call occurrences whose entries this walk wrote. A document a selection produced checks the paired
	// occurrence against that set: this target's log is one line of records with no parent relation, so a
	// call the walk leaves off the line is not in the file at all, and a result whose paired occurrence is
	// not written is omitted and counted (unpaired_tool_results) rather than written under an older call
	// that happens to share its id, which would present this result as that call's outcome.
	pairings     map[resultOccurrence]callOccurrence
	writtenCalls map[callOccurrence]bool
	// callOwners names every call id this walk wrote, which is the acceptance a document that is not a
	// selection's asks of a recorded result (resultCallWritten): the writer accepted a result when some
	// written call shared the id, and that default path is byte-identical to the same run before
	// selections existed (goal 0009's D4).
	callOwners map[string]struct{}
	// selectionReduced says the document this conversion walks is one Select produced
	// (Conversation.selectionReduced): the paired-occurrence acceptance and the row coalescing
	// (projectedCalls) exist for the reduction, and an unflagged run keeps the writer's earlier answers.
	selectionReduced bool

	// sourceFidelity is the completeness and the provider's own classification of the read itself.
	// Lines the provider's reader could not turn into a message never reach this writer as evidence at
	// all, so the conversion's report has to name them from the read's own count; and a read the provider
	// classified as a summary fallback, or as not current for its data version, is named from that
	// classification the same way: a caller that read "converted" alone would not know the source was
	// partial or that the conversation is the provider's fallback rather than the session's recorded
	// transcript.
	sourceFidelity Fidelity

	lossReasoning           int
	lossSidechain           int
	lossSystemRecords       int
	lossToolRoleMessages    int
	lossUnclassifiedRoles   int
	lossEmptyUserMessages   int
	lossWhitespaceDialogue  int
	lossNonDialogueText     int
	lossUnresolvedDialogue  int
	lossUnprojectedDialogue int
	lossUnpairedResults     int
	lossUnresolved          int
	lossOutcomes            int
	lossPreBoundary         int
	lossEmptyInputs         int
	lossGeneratedIDs        int
	lossCoalesced           int
	lossSubagentLinks       int
	lossResultFallbacks     int
	lossResultInstants      int
	lossResultSources       int
	lossEmptyResults        int
	lossWhitespaceResults   int
	lossResultBlocks        int
	lossInactiveBranch      int
	lossUnresolvedParents   int

	ledger

	sawBoundary bool
	turnOpen    bool
}

func (b *codexRolloutBuilder) write(record codexEnvelope) {
	b.encoder.Encode(record)
}

func (b *codexRolloutBuilder) sessionMeta(startedAt time.Time) {
	payload := codexSessionMetaPayload{
		ID:            b.sessionID,
		Timestamp:     startedAt.Format(time.RFC3339Nano),
		Cwd:           b.cwd,
		Originator:    codexOriginator,
		CLIVersion:    b.cliVersion,
		Source:        codexSourceCLI,
		ThreadSource:  codexThreadSourceUser,
		ModelProvider: strings.TrimSpace(b.opts.Provider),
	}
	// The source's own repository provenance travels with the session; the commit hash does not,
	// because the source document records none.
	if branch := strings.TrimSpace(b.gitBranch); branch != "" || strings.TrimSpace(b.gitRemote) != "" {
		payload.Git = &codexGit{Branch: branch, RepositoryURL: b.gitRemote}
	}
	b.write(codexEnvelope{
		Timestamp: b.clock.next(startedAt),
		Type:      codexRecordSessionMeta,
		Payload:   payload,
	})
}

// writeConversation walks the document once and writes every record it can express. The message's place
// in the walk keys the ids generated for calls that recorded none (toolCall), so two messages whose
// ordinals were never assigned cannot collapse into one call.
//
// A record the source's own relations place off the line its loader walks is not written. This target's
// log is one sequence of records with no parent relation to hang a record off, and a response item
// written into it becomes part of the conversation a resumed turn reads: an abandoned branch cannot
// travel here as recorded history without being revived as an active turn, so it is omitted and counted
// (inactive_branch_records). The branch itself is still in the source session, where its own
// conversation can be converted on demand. The line is the read's own: a selection does not redefine it
// (sourceLineAnswers), so a window that ends on the branch's records omits them and counts them too.
//
// The walk's own answer about the places it chose travels with it: where the read left a record's place
// open -- a record the read left parentless or gave a parent this read does not hold, or one chained
// through a tool result standing between it and the call it named -- the record's place on this line is
// this conversion's reading rather than the read's, and the report counts it (unresolved_source_parents)
// instead of presenting the line as the source's own by right.
func (b *codexRolloutBuilder) writeConversation(conv Conversation) {
	inactive, unresolved := sourceLineAnswers(conv)
	b.lossUnresolvedParents = unresolved
	// The pairing the seam reads (pairedCallOccurrences) is resolved over the whole document this
	// conversion walks, before the walk drops the records the read's line leaves off it: which call
	// occurrence a recorded result answers is a property of the read's own order, and the walk has to
	// ask that answer rather than accept any written call that shares the id.
	b.pairings = pairedCallOccurrences(conv)
	b.writtenCalls = map[callOccurrence]bool{}
	for index, message := range conv.Messages {
		b.messageIndex = index
		if inactive[index] {
			b.lossInactiveBranch++
			continue
		}
		b.writeMessage(message)
	}
}

func (b *codexRolloutBuilder) writeMessage(message Message) {
	if message.IsSidechain {
		// A subagent's turn, recorded in the same transcript. The target has no subagent attribution
		// for a flat message, so carrying it as an ordinary turn would state that the conversation's
		// own agent said it. A subagent record that also carries the source's compaction marker is
		// still a subagent's own record -- the inherited Claude parser keeps both flags on one record
		// (internal/parser's TestParseClaudeSession_CompactBoundary) -- so it is excluded here, before
		// the boundary rule below: its summary stands for the subagent's history, and applying it as
		// the target's compacted record would replace the main conversation's model-visible history
		// with it.
		b.lossSidechain++
		return
	}
	if message.IsCompactBoundary {
		// A compaction boundary is the target's own record for the same event: the summary the source
		// kept in place of the history before it. The target applies it to its model-visible history,
		// and the turns before it stay in the file (counted as carried, with the caveat reported).
		b.lossPreBoundary = b.carried.Messages
		b.sawBoundary = true
		b.write(codexEnvelope{
			Timestamp: b.clock.next(message.Timestamp),
			Type:      codexRecordCompacted,
			Payload:   codexCompactedPayload{Message: message.Content},
		})
		b.carried.CompactionBoundaries++
		b.items = append(b.items, TargetItem{Kind: ItemCompactionBoundary, Text: message.Content})
		return
	}
	if message.IsSystem {
		// A record the provider itself flagged as system-injected (a reminder, a notice). Its own text
		// is not the conversation: the target has no system role, its developer channel carries the
		// target's own instructions, and the text is counted as omitted. The record can still carry
		// tool evidence, though -- the inherited Claude parser keeps a promoted system reminder's tool
		// results on the message it marks IsSystem (internal/parser's
		// TestParseClaudeSession_SystemReminderPromotionPreservesToolResults) -- and a result dropped
		// here would leave the call it answers without its recorded output while the report named only
		// the record's text. So the text alone is suppressed; the tool evidence travels exactly as a
		// dialogue record's would, because a call and its output carry no role of their own.
		b.lossSystemRecords++
		b.writeToolEvidence(message)
		return
	}
	dialogue := dialogueOf(message)
	if len(message.ToolResults) == 0 && len(message.ToolCalls) == 0 && strings.TrimSpace(message.Thinking) != "" &&
		strings.TrimSpace(dialogue.Text) == "" && dialogue.State != dialogueUnestablished {
		// Nothing but a persisted reasoning trace: the target has no record for a persisted reasoning
		// trace that is not the target's own encrypted artifact (goal 0013 owns this), so it is counted,
		// not written as a message with empty text. The test is the same one the message rule below
		// makes, and the state is asked beside it: a record whose dialogue the read could not establish
		// is counted as that, not as a reasoning-only record (its Content was not this message's text,
		// and the report must not say the record held nothing but a trace when what it held was a text
		// no answer separates from the rendering).
		b.lossReasoning++
		return
	}
	if strings.TrimSpace(message.Thinking) != "" {
		b.lossReasoning++
	}

	role := parser.RoleType(message.Role)
	if role != parser.RoleUser && role != parser.RoleAssistant {
		// Only a user or assistant record is dialogue. A record the source wrote with any other role --
		// a tool's own output (the native tool role several providers keep), a system notice that was
		// not flagged as one, or a role this build does not classify -- must not be written as either:
		// presenting it as the conversation's user or agent would state a role the source never
		// recorded. It is counted under its own category instead. Its tool calls and results are still
		// written below, because a call and its output carry no role of their own in the target, and
		// dropping real evidence silently is what the report exists to prevent.
		switch role {
		case parser.RoleTool:
			b.lossToolRoleMessages++
		case parser.RoleSystem:
			b.lossSystemRecords++
		default:
			b.lossUnclassifiedRoles++
		}
		if len(message.ToolCalls) == 0 && len(message.ToolResults) == 0 {
			// Nothing but the role itself: the count above is the whole of what the target does not have.
			return
		}
	} else if dialogueLossOf(dialogue) == dialogueLossUnresolved {
		// The read established no dialogue for this record and its Content is not the record's own
		// bytes (dialogueOf): the reader rendered the record's persisted reasoning trace into a Content
		// that also holds whatever the record's own text was, or the record carries calls whose
		// renderings a reader may have inlined. Publishing either would state words the source never
		// recorded as spoken, and removing the rendering by inference is the guess this rule exists to
		// refuse, so the text is not written and the record is counted. Its tool evidence still travels
		// below: a call and its output carry no role of their own.
		b.lossUnresolvedDialogue++
	} else {
		if dialogueLossOf(dialogue) == dialogueLossUnprojected {
			// Content, not a projection, is what travels: the report says so per record
			// (unprojected_dialogue) rather than presenting the read's Content as the source's own
			// answer about what the model said.
			b.lossUnprojectedDialogue++
		}
		switch text := dialogue.Text; {
		case text != "" && strings.TrimSpace(text) == "":
			// The source recorded dialogue text that holds only whitespace. The target's own reader
			// reads a message whose text is blank after trimming as no message at all (internal/parser's
			// handleResponseItem returns before appending one), so a written record would be in the file
			// and absent from every read of it -- including this conversion's own round-trip check,
			// which would then report the file as missing evidence it holds. The text is not written and
			// is named here; the record's tool evidence is still written below, because a call and its
			// output carry no role of their own.
			b.lossWhitespaceDialogue++
		case text != "" && role == parser.RoleUser:
			b.startTurn(message.Timestamp)
			b.messageItem(ItemUser, codexRoleUser, codexContentInputText, text, message.Timestamp)
		case text != "":
			b.messageItem(ItemAssistant, codexRoleAssistant, codexContentOutputText, text, message.Timestamp)
		case role == parser.RoleUser && len(message.ToolResults) == 0 && len(message.ToolCalls) == 0:
			// A user record with no dialogue and no tool evidence. Two shapes reach here: a record
			// holding no text at all, and one whose text the source itself classified as not dialogue
			// (a tool result the provider recorded with a user role). Writing the second as that user's
			// turn would state that the user said the tool's words, so it is counted under its own
			// category rather than reported as an empty record.
			if strings.TrimSpace(message.Content) == "" {
				b.lossEmptyUserMessages++
			} else {
				b.lossNonDialogueText++
			}
			return
		}
	}

	b.writeToolEvidence(message)
}

// writeToolEvidence writes the calls and results a record carries: each call, each result paired with
// the call it answers, and -- for an archive read, where a call's outcome is also held as the derived
// summary in ResultContent -- that summary only for a call with no recorded result of its own
// (callHasRecordedResult). It is asked for by a dialogue record and by a record whose own text is not
// written (a system-injected record), because a call and its output carry no role of their own in the
// target and dropping real evidence silently is what the report exists to prevent. A record with no
// tool evidence contributes nothing here.
func (b *codexRolloutBuilder) writeToolEvidence(message Message) {
	calls := b.projectedCalls(message)
	for _, entry := range calls {
		b.toolCall(entry.index, entry.call, message.Timestamp)
	}
	for index, result := range message.ToolResults {
		// The pairing the seam reads, keyed by the result's own place: a result with a recorded id
		// answers the call occurrence the read's own ancestry establishes where it does, else the one the
		// id records nearest before it, and none where the read holds no such occurrence. nil is passed
		// for the latter, which resultCallWritten counts as an unpaired result on a reduced document
		// unless the recorded id is empty and the sole outstanding call answers it below.
		var paired *callOccurrence
		if place, ok := b.pairings[resultOccurrence{message: b.messageIndex, result: index}]; ok {
			paired = &place
		}
		b.toolResult(paired, result, message.Timestamp)
	}
	// An archive read carries a call's outcome both as the result events the archive kept on the call
	// and as the summary it holds in ResultContent. That summary is derived from those same events by
	// the inherited write path (internal/db stores it only when it is not the sole content-bearing
	// event's text, and restores it from that event when the column was cleared), so beside them it is
	// a view of the recorded evidence rather than another outcome recorded there -- writing it too would
	// present a normalized display aggregate as a separately recorded tool output. Every row written
	// above is walked here too (calls), so a coalesced projection's summary is not written twice; the
	// summary is written only when the call has no recorded result of its own, where it is the only
	// evidence the read holds of that outcome.
	for _, entry := range calls {
		call := entry.call
		result, ok := derivedResultSummary(call, call.ToolUseID)
		if !ok {
			continue
		}
		if callHasRecordedResult(message.ToolResults, call.ToolUseID) {
			continue
		}
		// The summary is the call's own evidence, so it is paired with the call occurrence the walk just
		// wrote rather than with a document occurrence: there is no recorded result here.
		place := callOccurrence{message: b.messageIndex, call: entry.index}
		b.toolResult(&place, result, message.Timestamp)
	}
}

// codexProjectedCall is one call this writer writes, with the place it holds in its source message: the
// occurrence the seam's pairing and writtenCalls are keyed by.
type codexProjectedCall struct {
	index int
	call  ToolCall
}

// projectedCalls answers one record's calls as this conversion writes them. A document that is not a
// selection's writes every row, exactly as it did before selections existed (goal 0009's D4). A document
// a selection produced writes one row per invocation: the inherited Codex reader projects one
// `apply_patch` into one call per file it names, every row sharing the call's id and input, and the
// selection's own accounting already reads those rows as one call (distinctCalls) -- the committed patch
// fixture is the measured shape. Writing every row here would put two function_call records with one id
// and one output in the file for a single invocation the report counts once; the rows not written are
// counted (coalesced_tool_call_projections).
func (b *codexRolloutBuilder) projectedCalls(message Message) []codexProjectedCall {
	if !b.selectionReduced {
		calls := make([]codexProjectedCall, 0, len(message.ToolCalls))
		for index, call := range message.ToolCalls {
			calls = append(calls, codexProjectedCall{index: index, call: call})
		}
		return calls
	}
	return distinctCalls(message, func(index int, call ToolCall) codexProjectedCall {
		return codexProjectedCall{index: index, call: call}
	}, &b.lossCoalesced)
}

// derivedResultSummary answers the archive read's own summary of one call as the ToolResult a writer may
// carry where the call has no recorded result of its own (callHasRecordedResult): the summary is a view
// of the outcome only where there is no result to carry instead, and the boolean answers whether the read
// holds a summary at all (neither text nor a size). toolUseID is the id the result is written under -- the
// call's own recorded id, or the one a conversion minted for a call that recorded none. The guard and the
// value are one function because every writer of this family applies them together; which of a record's
// results is "recorded" stays each writer's own decision, made by the caller.
func derivedResultSummary(call ToolCall, toolUseID string) (ToolResult, bool) {
	if strings.TrimSpace(call.ResultContent) == "" && call.ResultContentSize == 0 {
		return ToolResult{}, false
	}
	return ToolResult{
		ToolUseID:      toolUseID,
		ContentRaw:     call.ResultContent,
		ContentSize:    call.ResultContentSize,
		ContentDecoded: true,
	}, true
}

// callHasRecordedResult reports whether the conversation already carries a recorded result for a call,
// which is what makes the archive's summary of that call a derived view of recorded evidence rather
// than a second outcome. The archive holds a call's outcome as the events it kept on the call and, when
// they differ from their own text, as the summary in ResultContent: internal/db's
// DedupToolCallResultSummary stores the summary only when it is not the sole content-bearing event's
// text, and RestoreToolCallResultContent refills it from that event when the column was cleared. So a
// call with any result of its own has its outcome carried as that result, whatever the summary text
// happens to be; a summary whose call carries no result at all is the only evidence the read holds of
// the outcome, and it is written.
func callHasRecordedResult(results []ToolResult, callID string) bool {
	for _, result := range results {
		if result.ToolUseID == callID {
			return true
		}
	}
	return false
}

// startTurn opens a turn: the target groups records into turns, and a user message is where a turn
// begins. A caller that may already have one open checks b.turnOpen itself.
func (b *codexRolloutBuilder) startTurn(at time.Time) {
	id, err := uuid.NewV7()
	turnID := ""
	if err == nil {
		turnID = id.String()
	} else {
		turnID = fmt.Sprintf("%s%08x", codexGeneratedTurnPrefix, b.carried.Turns)
	}
	b.write(codexEnvelope{
		Timestamp: b.clock.next(at),
		Type:      codexRecordTurnContext,
		Payload:   codexTurnContextPayload{TurnID: turnID, Cwd: b.cwd},
	})
	b.carried.Turns++
	b.turnOpen = true
}

func (b *codexRolloutBuilder) messageItem(kind TargetItemKind, role, contentType, text string, at time.Time) {
	if !b.turnOpen {
		b.startTurn(at)
	}
	if role == codexRoleUser {
		b.write(codexEnvelope{
			Timestamp: b.clock.next(at),
			Type:      codexRecordResponseItem,
			Payload: codexMessagePayload{
				Type:    codexItemMessage,
				Role:    role,
				Content: []codexContent{{Type: contentType, Text: text}},
			},
		})
		b.write(codexEnvelope{
			Timestamp: b.clock.next(at),
			Type:      codexRecordEventMsg,
			Payload: codexUserMessagePayload{
				Type:         codexEventUserMessage,
				Message:      text,
				Images:       []any{},
				LocalImages:  []any{},
				TextElements: []any{},
			},
		})
	} else {
		// The target writes the agent's event before its response item; the order is its own, and
		// following it keeps a converted session shaped like one the target wrote itself.
		b.write(codexEnvelope{
			Timestamp: b.clock.next(at),
			Type:      codexRecordEventMsg,
			Payload:   codexAgentMessagePayload{Type: codexEventAgentMessage, Message: text},
		})
		b.write(codexEnvelope{
			Timestamp: b.clock.next(at),
			Type:      codexRecordResponseItem,
			Payload: codexMessagePayload{
				Type:    codexItemMessage,
				Role:    role,
				Content: []codexContent{{Type: contentType, Text: text}},
			},
		})
	}
	b.carried.Messages++
	if kind == ItemUser {
		b.carried.UserMessages++
	} else {
		b.carried.AssistantMessages++
	}
	b.items = append(b.items, TargetItem{Kind: kind, Text: text})
}

func (b *codexRolloutBuilder) toolCall(index int, call ToolCall, at time.Time) {
	if !b.turnOpen {
		b.startTurn(at)
	}
	callID := strings.TrimSpace(call.ToolUseID)
	if callID == "" {
		// The call's place in the document keys its generated id -- the message's position in the walk
		// and the call's index in that message -- and not the message's own ordinal: an embedding caller
		// may leave ordinals unassigned, and two messages' id-less calls must not collapse into one id.
		// A shared id would make the pairing rule below read one call, so an id-less result could pair
		// with "the" outstanding call while two are.
		callID = fmt.Sprintf("%s%d_%d", codexGeneratedCallPrefix, b.messageIndex, index)
		b.lossGeneratedIDs++
	}
	input := strings.TrimSpace(call.InputJSON)
	if input == "" {
		// The target's call record carries an arguments string, and the source recorded no input
		// object for this call: an empty object keeps the call's identity and its output paired, and
		// the report says the input was empty rather than leaving a reader to read "" as JSON.
		input = "{}"
		b.lossEmptyInputs++
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "unknown_tool"
	}
	b.noteToolName(name)
	if strings.TrimSpace(call.SubagentSessionID) != "" {
		// The source recorded which session this call spawned: the inherited Codex parser sets the child
		// session id on a spawn_agent call, and the archive read carries the same column. The rollout
		// holds that attribution in its own event records -- the `collab_agent_spawn_end` and
		// `sub_agent_activity` event_msg payloads carrying the call's id and the child's thread id,
		// which the inherited reader turns into this link (internal/parser/codex.go:700-707,834-865) --
		// and this conversion's writer emits no such record, so the link has no place in the file. The
		// call itself is written below; counted rather than dropped in silence, and the child is neither
		// read nor converted.
		b.lossSubagentLinks++
	}
	b.write(codexEnvelope{
		Timestamp: b.clock.next(at),
		Type:      codexRecordResponseItem,
		Payload: codexFunctionCallPayload{
			Type:      codexItemFunctionCall,
			Name:      name,
			Arguments: input,
			CallID:    callID,
		},
	})
	// The occurrence, not the id: a later result is paired by the read's own answer about *which* call
	// occurrence it names, and this is the one whose entry stands here. The id set beside it is the
	// acceptance a document that is not a selection's asks (resultCallWritten).
	b.writtenCalls[callOccurrence{message: b.messageIndex, call: index}] = true
	b.callOwners[callID] = struct{}{}
	if b.pendingCalls == nil {
		b.pendingCalls = map[string]struct{}{}
	}
	b.pendingCalls[callID] = struct{}{}
	b.carried.ToolCalls++
	b.items = append(b.items, TargetItem{Kind: ItemToolCall, CallID: callID, ToolName: name, InputJSON: input})
}

// toolResult writes one tool result. paired is the call occurrence the seam's own order pairs the result
// with: the pairing the read recorded, keyed by the result's place in the document
// (pairedCallOccurrences), or the call a derived summary came from. It is nil where the read holds no
// call occurrence the result's id records before it.
func (b *codexRolloutBuilder) toolResult(paired *callOccurrence, result ToolResult, at time.Time) {
	callID := strings.TrimSpace(result.ToolUseID)
	// resultIDInferred says the pairing below is the only id this result has: the source recorded none of
	// its own. The counter it feeds is incremented past the omission guards, together with the written
	// result, because an inferred pairing on a result this conversion does not write is not evidence the
	// file holds.
	resultIDInferred := false
	if callID == "" {
		pending, ok := b.solePendingCall()
		if !ok {
			// No call is outstanding, or several are and the result could belong to any of them: the
			// source recorded no id, so the pairing would be an invention.
			b.lossUnpairedResults++
			return
		}
		callID = pending
		resultIDInferred = true
	} else if !b.resultCallWritten(paired, callID) {
		// The acceptance is resultCallWritten's own answer: the paired occurrence's written entry for a
		// reduced document, and the id-existence check the default path has always asked.
		b.lossUnpairedResults++
		return
	}
	// The recorded result answers its call here, whatever its output turns out to be below: a call whose
	// result arrived leaves the outstanding set, so a later id-less result cannot be attributed to it.
	delete(b.pendingCalls, callID)
	text, blocks := resultText(result)
	b.lossResultBlocks += blocks
	if strings.TrimSpace(text) == "" {
		if text == "" {
			// The target's output field carries text; a result with none leaves nothing to write, and an
			// empty string would read as "the tool returned nothing" rather than "the source recorded no
			// output text".
			b.lossEmptyResults++
		} else {
			// The read holds text, but every byte of it is whitespace. The inherited Codex reader trims a
			// function_call_output's text and reads an output that is only whitespace as no result at all
			// (internal/parser's parseCodexFunctionOutput, whose handler returns before recording a
			// result), so a written record would be in the file and absent from every read of it --
			// including this conversion's own round-trip check, which would report the file as missing
			// evidence it holds. The omission is counted under its own category rather than as a result
			// that recorded no text.
			b.lossWhitespaceResults++
		}
		return
	}
	// The carried-with-caveat categories are counted here, past every omission guard: a result this
	// conversion does not write carries no output, and counting it as carried (with the caveat that its
	// text travelled without its recorded outcome, as an unresolved placeholder, or under a pairing the
	// source never recorded) would claim evidence the file does not hold.
	if resultIDInferred {
		// The result recorded no id of its own, so the id it is written under is the pairing with the one
		// call that was outstanding: only a written result carries that caveat.
		b.lossGeneratedIDs++
	}
	if result.PersistedOutputUnresolved {
		// The source's own parse-time answer that the content is the placeholder naming a side file it
		// could not read, not the tool's output. The text travels as recorded and the report says what it
		// is.
		b.lossUnresolved++
	}
	if strings.TrimSpace(result.Status) != "" {
		b.lossOutcomes++
	}
	// The output record is stamped with the instant of the message the result travelled on, not with
	// the instant the read answered for the result's own record: the two reads are counted apart -- the
	// read answered an instant and the containing record's is written instead, or the read answered none
	// and the containing record's instant stands in.
	if result.Timestamp.IsZero() {
		b.lossResultFallbacks++
	} else if !result.Timestamp.Equal(at) {
		b.lossResultInstants++
	}
	if strings.TrimSpace(result.Source) != "" {
		// The read named which native record carried the result's text (a result block in a message, a
		// `function_call_output`, a wait call's output event, a subagent notification). The target's
		// output carries the text and no such label -- its own read answers its own for its own shapes --
		// so the source's label is omitted and counted rather than dropped in silence.
		b.lossResultSources++
	}
	b.write(codexEnvelope{
		Timestamp: b.clock.next(at),
		Type:      codexRecordResponseItem,
		Payload: codexFunctionCallOutputPayload{
			Type:   codexItemFunctionOutput,
			CallID: callID,
			Output: text,
		},
	})
	b.carried.ToolResults++
	b.items = append(b.items, TargetItem{Kind: ItemToolResult, CallID: callID, Text: text})
}

// resultCallWritten answers whether a recorded result that names a call may be written under that call's
// id. A document a selection produced asks the seam's own order: the call occurrence the read answers for
// the result has to have its entry in this file (writtenCalls). It does not when the read's line leaves
// the call off it, when the selection left it out, or when the record carrying it brings no tool
// evidence here -- this target's log is one line, so such a call is not in the file at all, and
// accepting the id because some other written call shares it would present this result as that call's
// outcome, an invocation the source never recorded. A conversation the read returned asks the question
// this writer has always asked -- whether any written call stands under the id -- so a run without a
// selection is byte-identical to the same run before selections existed (goal 0009's D4).
func (b *codexRolloutBuilder) resultCallWritten(paired *callOccurrence, callID string) bool {
	if b.selectionReduced {
		return paired != nil && b.writtenCalls[*paired]
	}
	_, ok := b.callOwners[callID]
	return ok
}

// codexUnestablishedOutcome is this route's own answer for a record whose dialogue was not established
// (dialogueUnestablished), passed to dialogueLossRows. It names what the writer does not write and where
// the record's reasoning goes: this target records reasoning as its provider's own encrypted artifact,
// so a plaintext trace has no shape here -- it is counted as omitted above (reasoning_text), never
// presented as carried. A clause saying the trace "still travels" is Pi's and OpenCode's answer, not
// this one (round 17's PD-3: two of the four copies carried it wrongly).
const codexUnestablishedOutcome = "the text is not written, this count is not a claim that the record " +
	"wrote none, and a trace the record carries is omitted with it -- counted under reasoning_text " +
	"above, because this target has no plaintext-trace shape: it records reasoning as its provider's " +
	"own encrypted artifact"

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *codexRolloutBuilder) losses() []Loss {
	names := b.toolNameOrder
	toolDetail := toolVocabularyDetail(names, b.opts.MaxToolNames, "arguments")
	emptyResultsDetail := "every carried tool result had recorded output text"
	if b.lossEmptyResults > 0 {
		emptyResultsDetail = "the target's function_call_output carries output text only; a result " +
			"this read holds no text for is not written, because an empty string would read as an " +
			"empty tool output. A content field can hold no text at all, or hold only blocks this " +
			"target has no field for (counted above as unrepresentable_result_blocks); output that " +
			"holds only whitespace is counted under its own category (whitespace_only_tool_results)"
	}
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = "the turns before a compaction boundary stay in the file and are counted " +
			"here: the target applies the boundary to its model-visible history, so those turns are " +
			"recorded history the target does not continue from"
	}
	// The read's own completeness is reported here rather than left to the fidelity block alone: a line
	// the provider's reader skipped produced no message, so no per-message category could count it, and
	// "converted" without this entry would read as though the conversation the target holds were the
	// whole of what the source file records. The provider's own classifications of the read are reported
	// the same way: a summary fallback is not the session's recorded transcript, and a result the
	// provider says is not current may not be the session's complete history, so "converted" without
	// those entries would read as though the conversation were the session as the source recorded it.
	unreadableLinesDetail := malformedLinesLossDetail(b.sourceFidelity)
	summaryTranscriptDetail := summaryTranscriptLossDetail(b.sourceFidelity)
	needsRetryDetail := needsRetryLossDetail(b.sourceFidelity)
	if b.sourceFidelity.NeedsRetry {
		needsRetryDetail += ". A Codex fork whose parent page cannot be read keeps the copied parent " +
			"messages until that parent resolves"
	}
	losses := []Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionOmitted,
			Count:       b.lossReasoning,
			Detail: "the target records reasoning as its provider's own encrypted artifact; writing the " +
				"source's plaintext reasoning in its place would present it as a target-native trace",
		},
		{
			Category:    "sidechain_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossSidechain,
			Detail: "a sidechain message is a subagent's turn recorded in the same transcript; the " +
				"target has no subagent attribution for a flat message, so carrying it as an ordinary " +
				"turn would state that the conversation's own agent said it",
		},
		{
			Category:    "system_records",
			Disposition: DispositionOmitted,
			Count:       b.lossSystemRecords,
			Detail: "the target has no system role: its developer channel carries the target's own " +
				"instructions, and a provider's bookkeeping text does not belong there. Only the " +
				"record's own text is omitted; a tool call or result recorded on the same record is " +
				"written as the tool evidence it is",
		},
		{
			Category:    "tool_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossToolRoleMessages,
			Detail: "a message the source recorded with the tool role is the tool's own output, and the " +
				"target's function_call_output is where a tool's words belong: the text is carried only " +
				"there, paired with the call it belongs to, and never as a user or agent message, which " +
				"would state that the conversation's own user or agent said the tool's words",
		},
		{
			Category:    "unclassified_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossUnclassifiedRoles,
			Detail: "a record whose role this build does not classify has no message in the target to go " +
				"to; writing it as a user or agent message would state a role the source never recorded",
		},
		{
			Category:    "empty_user_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyUserMessages,
			Detail: "a user record with neither dialogue text nor tool evidence carries nothing the " +
				"target can show, so no message is written for it",
		},
		{
			Category:    "non_dialogue_content",
			Disposition: DispositionOmitted,
			Count:       b.lossNonDialogueText,
			Detail: "the source itself classified these records as carrying no dialogue, and their " +
				"content is not dialogue it rendered either (a tool result the provider recorded with " +
				"a user role): the target has no message that would not state the user said those " +
				"words, so the text is not written",
		},
	}
	// The two rows the dialogue states are reported under come from the one constructor
	// (dialogueLossRows), with this route's own answer for an unestablished record, so no route can
	// word or count a state differently from another.
	losses = append(losses, dialogueLossRows(b.lossUnresolvedDialogue, b.lossUnprojectedDialogue,
		codexUnestablishedOutcome)...)
	losses = append(losses, []Loss{
		{
			Category:    "whitespace_only_dialogue",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespaceDialogue,
			Detail: "the source recorded dialogue text that holds nothing but whitespace; the inherited " +
				"Codex reader reads a message whose text is blank after trimming as no message at all, " +
				"so a written record would be in the file but absent from every read of it and the " +
				"round-trip check would report evidence the file holds as missing. The text is not " +
				"written, and any tool evidence the same record carries still is",
		},
		{
			Category:    "unpaired_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossUnpairedResults,
			Detail: "a tool result whose call this conversion did not carry has no pair in the target, " +
				"and fabricating the call would state a tool invocation the source never recorded",
		},
		{
			Category:    "empty_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyResults,
			Detail:      emptyResultsDetail,
		},
		{
			Category:    "whitespace_only_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespaceResults,
			Detail: "the source recorded output text that holds nothing but whitespace; the inherited " +
				"Codex reader trims a function_call_output's text and reads an output that is only " +
				"whitespace as no result at all, so a written record would be in the file but absent " +
				"from every read of it and the round-trip check would report evidence the file holds " +
				"as missing. The output is not written, and it is named here rather than reported as a " +
				"result that recorded no text",
		},
		{
			Category:    "unrepresentable_result_blocks",
			Disposition: DispositionOmitted,
			Count:       b.lossResultBlocks,
			Detail: "the target's function_call_output carries text only, and the reader's text " +
				"channel carries a block's own text (or result text) and nothing else: a block the " +
				"source recorded with other content (an image payload, a block type this build does " +
				"not read) is not written. The count is of blocks, not of results, and a block whose " +
				"text was carried is not counted",
		},
		{
			Category:    "unresolved_tool_outputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossUnresolved,
			Detail: "the source's own parse said these results are the placeholder naming a side file " +
				"it could not read, not the tool's output; that text is what was carried, and only " +
				"results that were written are counted here",
		},
		{
			Category:    "tool_result_outcomes",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossOutcomes,
			Detail: "the target's function_call_output has no outcome field; the result's recorded " +
				"output text is carried as it was, and its recorded status is not. Only results that " +
				"were written are counted here",
		},
		{
			Category:    "empty_tool_inputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossEmptyInputs,
			Detail: "the source recorded no input object for these calls; the target's call record needs " +
				"an arguments string, so an empty object was written",
		},
		{
			Category:    "generated_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossGeneratedIDs,
			Detail: "the source recorded no tool-use id for these calls or results; the target pairs a " +
				"call and its output by id, so one was generated from the message's own position",
		},
		{
			Category:    "subagent_session_links",
			Disposition: DispositionOmitted,
			Count:       b.lossSubagentLinks,
			Detail: "the source recorded which session these carried calls spawned (the inherited " +
				"Codex parser sets a spawn_agent call's child session id, and the archive read carries " +
				"the same column). The target's log holds that attribution in a spawn-completion event " +
				"of its own (`collab_agent_spawn_end`, whose payload carries the call's id and the " +
				"child's thread id, which the inherited reader turns into this link), and this " +
				"conversion writes no such record: the call itself is written with its id, name and " +
				"arguments, and what is omitted is the attribution of the session it spawned. The " +
				"child session is neither read nor converted -- a conversion's unit is the selected " +
				"session -- and the count is of the calls this conversion wrote",
		},
		{
			Category:    "tool_result_sources",
			Disposition: DispositionOmitted,
			Count:       b.lossResultSources,
			Detail: "the read names which native record carried each result's text (a result block " +
				"inside a message, a `function_call_output`, a wait call's own output event, a " +
				"subagent notification: internal/parser's readers answer this per shape). The " +
				"target's function_call_output carries the text and no such label -- its own read " +
				"answers the analogous question for its own shapes -- so the source's label is " +
				"omitted, counted per result this conversion wrote that recorded one",
		},
		{
			Category:    "result_completion_fallbacks",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossResultFallbacks,
			Detail: "a function_call_output carries the result's text, and its envelope is stamped " +
				"with the instant of the record the result travelled on. Where the read holds no " +
				"instant of the result's own record -- a provider-recorded event with no timestamp, " +
				"or the archive's derived summary, which is stored on the call's own row and has no " +
				"record of its own -- that containing record's instant is written, an instant the " +
				"read answered for that record rather than one this build invented; only results " +
				"this conversion wrote are counted here",
		},
		{
			Category:    "result_record_instants",
			Disposition: DispositionOmitted,
			Count:       b.lossResultInstants,
			Detail: "the source recorded the instant the result itself was recorded on (a result " +
				"event's own envelope instant, which may be later than the record that carried its " +
				"call, and which the archive read answers from the stored event's own column). A " +
				"function_call_output states its call, its text and the envelope's own timestamp, " +
				"and this conversion stamps that envelope with the containing record's instant, not " +
				"with the result's own: the result's own recorded instant is not what the file " +
				"holds. Counted per result this conversion wrote and only where the read answered " +
				"one",
		},
		{
			Category:    "substituted_record_instants",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.clock.substituted,
			Detail:      substitutedRecordInstantsDetail(),
		},
		{
			Category:    "tool_name_vocabulary",
			Disposition: DispositionCarriedWithCaveat,
			Count:       len(names),
			Detail:      toolDetail,
		},
		{
			Category:    "provider_binding",
			Disposition: DispositionCarriedWithCaveat,
			Count:       providerBindingCount(b.opts.Provider),
			Detail: "the session records the target's own configured model provider, which is what its " +
				"loader checks when the session is resumed; the value is a binding to the target's " +
				"configuration, not a claim that a target model produced the conversation, and changing " +
				"that configuration later makes the target refuse to resume it",
		},
		{
			Category:    "inactive_branch_records",
			Disposition: DispositionOmitted,
			Count:       b.lossInactiveBranch,
			Detail: "the source's own relations record these turns as a branch the conversation left: " +
				"no later record descends from them, so the source's own loader does not build context " +
				"from them. This target's log is one sequence of records with no parent relation to hang " +
				"a record off, and a response item written into it becomes part of the conversation a " +
				"resumed turn reads, so the branch cannot travel here as recorded history without being " +
				"revived as an active turn; it is omitted instead. The branch is still in the source " +
				"session, and its own conversation can be converted on demand like any other history",
		},
		{
			Category:    "unresolved_source_parents",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossUnresolvedParents,
			Detail: "a relation the read could not place exactly, so a record's place on this line is " +
				"this conversion's reading: the record named the message that made a call while a " +
				"result of that call stands between them in the file (the inherited Pi reader resolves " +
				"a result's descendants to their visible ancestor, so the read answers both relations " +
				"the same way, and the file's own reading -- call → result → what follows -- is what " +
				"keeps the result on this line); the record carried an identity but no parent, and is " +
				"not the conversation's first; or the record named a parent this read does not hold. " +
				"Each of those records is still written; what the count names is that the source's own " +
				"place for it is not the one this line was built from. A record the read answered no " +
				"identity for is never counted: the read's own order is all there is to follow, and " +
				"following it invents nothing",
		},
		{
			Category:    "pre_compaction_history",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossPreBoundary,
			Detail:      preBoundaryDetail,
		},
		{
			Category:    "malformed_source_lines",
			Disposition: DispositionOmitted,
			Count:       b.sourceFidelity.MalformedLines,
			Detail:      unreadableLinesDetail,
		},
		{
			Category:    "summary_transcript",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(b.sourceFidelity.TranscriptFidelity == parser.TranscriptFidelitySummary),
			Detail:      summaryTranscriptDetail,
		},
		{
			Category:    "source_needs_retry",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(b.sourceFidelity.NeedsRetry),
			Detail:      needsRetryDetail,
		},
	}...)
	// The coalesced-projection category exists only where the coalescing can act. The writer's own
	// completeness rule -- every category on every run -- would put a zero row in every codex report, and
	// a run without a selection is byte-identical to the same run before selections existed (goal 0009's
	// D4), whole report included: the row is therefore carried by a document a selection produced, whose
	// reduction is the only thing that can drop a projected row, and an unflagged report keeps its own
	// category list unchanged. The count is zero on a reduced document with no projected rows, so within
	// the selection surface the completeness rule holds as the other writers' does.
	if b.selectionReduced {
		losses = append(losses, Loss{
			Category:    "coalesced_tool_call_projections",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCoalesced,
			Detail: "the source read recorded these calls once for every file they touched (the " +
				"inherited Codex reader projects one apply_patch into one call per file, every " +
				"projection sharing the call's id and input); a document a selection produced writes " +
				"one function_call per invocation -- the unit the selection's own accounting reads " +
				"(distinctCalls), so one recorded output discharges it -- and what has no field is " +
				"the per-file attribution of the projections not written",
		})
	}
	return losses
}

// flagCount reports a state of the read or of the run as the count a loss entry carries: one entry when
// the state holds, none otherwise. The state is a property of the read, not a number of items, which is
// why it is reported this way rather than as a count of messages.
func flagCount(set bool) int {
	if set {
		return 1
	}
	return 0
}

// toolVocabularyDetail answers the tool_name_vocabulary category's detail: the source's own tool names
// travel, the target's own tool set is unaffected by them, and a resumed turn reads them as history
// rather than replaying them. The sentence is shared, because the rule is; inputWord is the word each
// target's own format uses for what a call's recorded input is called. order is the source's tool names
// in the order they were noted; maxNames bounds how many the report spells out.
func toolVocabularyDetail(order []string, maxNames int, inputWord string) string {
	names := append([]string(nil), order...)
	sort.Strings(names)
	if len(names) == 0 {
		return "no tool call was carried"
	}
	shown := names
	if len(shown) > maxNames {
		shown = shown[:maxNames]
	}
	return fmt.Sprintf(
		"tool calls keep the source's own tool names and recorded %s (%s%s); the target's own tool set "+
			"may not include them, and a resumed turn answers from them as history rather than replaying "+
			"them",
		inputWord, strings.Join(shown, ", "), moreNames(len(names)-len(shown)))
}

// malformedLinesLossDetail answers the malformed_source_lines category's detail: what the provider's
// reader skipped as unreadable, and whether the source file's last line was still being written. It is a
// property of the read rather than of a target format, so both writers state it in the same words.
func malformedLinesLossDetail(fidelity Fidelity) string {
	detail := "every source line this read produced a message for was readable"
	if fidelity.MalformedLines > 0 {
		detail = "the provider's reader skipped these source line(s) as unreadable (invalid JSON, or a " +
			"line the reader would not read), so no message in this conversation stands for them and no " +
			"record here can carry what it could not read"
		if fidelity.Truncated {
			detail += "; the source file's last line was still being written when it was read (the " +
				"fidelity block's truncated flag), and it is one of them"
		}
	}
	return detail
}

// summaryTranscriptLossDetail answers the summary_transcript category's detail: the provider's own
// classification of the read, in the same words for both writers, because it is the read's own property.
func summaryTranscriptLossDetail(fidelity Fidelity) string {
	if fidelity.TranscriptFidelity == parser.TranscriptFidelitySummary {
		return "the provider classified this read as a summary transcript: the conversation carried here " +
			"is a degraded fallback standing in for a transcript the provider could not read in full, not " +
			"the session's recorded transcript"
	}
	return "the provider's own classification says this read is the session's recorded transcript"
}

// needsRetryLossDetail answers the source_needs_retry category's detail, up to what the provider itself
// said: whether the read is current, and why not when it is not. A target whose reader has its own
// reason to keep such a read says so beside this answer (see codex_target.go's losses, the fork case).
func needsRetryLossDetail(fidelity Fidelity) string {
	if !fidelity.NeedsRetry {
		return "the provider's own classification says this read is current for its data version"
	}
	reason := ""
	if fidelity.RetryReason != "" {
		reason = " (" + fidelity.RetryReason + ")"
	}
	return "the provider classified this read as needing a retry" + reason + ": it returned the messages " +
		"it could read, which may not be the session's complete recorded history"
}

// providerBindingCount reports whether this conversion recorded a provider binding.
func providerBindingCount(provider string) int {
	if strings.TrimSpace(provider) == "" {
		return 0
	}
	return 1
}

func moreNames(more int) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf(", +%d more", more)
}

// resultText is the text a tool result contributes to the target, plus the number of blocks its content
// holds that the target's output field cannot carry.
//
// A result the read already decoded -- a result event's content, an archive row -- is text as it
// stands: decoding it again would strip the quotation marks from a tool whose own output is a JSON
// string. An encoded content field goes through the inherited reader's text channel instead, and a
// field that channel carries nothing from is not written as its own JSON: that would put a payload the
// target cannot express (an image) in the output field as though the tool had said it. When the channel
// carries nothing and the field is not JSON at all, the field's own bytes are the output, which is the
// shape a record whose content was never JSON has.
func resultText(result ToolResult) (string, int) {
	if result.ContentDecoded {
		return result.ContentRaw, 0
	}
	blocks := unrepresentedResultBlocks(result.ContentRaw)
	if decoded := parser.DecodeContent(result.ContentRaw); decoded != "" {
		return decoded, blocks
	}
	if json.Valid([]byte(result.ContentRaw)) {
		return "", blocks
	}
	return result.ContentRaw, 0
}

// unrepresentedResultBlocks counts the blocks in an encoded tool-result content field that hold
// evidence the target's output field cannot carry.
//
// The reader's text channel carries a block's "text" (or its "result") and nothing else, so a block it
// carries nothing from -- an image payload, a tool reference, a block type this build does not read --
// is evidence a conversion would otherwise drop without saying so. Blocks the channel carries text from
// are not counted (their remaining members are the reader's own business), and neither is a block that
// holds nothing but its own type and the provider's cache marker. A content field that is not a JSON
// array of objects has no blocks at all.
func unrepresentedResultBlocks(raw string) int {
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil || blocks == nil {
		return 0
	}
	count := 0
	for _, block := range blocks {
		carried, holds := false, false
		for name, value := range block {
			if isJSONEmpty(value) {
				continue
			}
			switch strings.ToLower(name) {
			case "type", "cache_control":
				// The block's own kind and a provider's cache marker are not content.
			case "text", "result":
				carried = true
			default:
				holds = true
			}
		}
		if holds && !carried {
			count++
		}
	}
	return count
}

// isJSONEmpty reports whether a raw JSON value carries nothing: a null, an empty string, an empty
// array or an empty object.
func isJSONEmpty(value json.RawMessage) bool {
	switch strings.TrimSpace(string(value)) {
	case "", "null", `""`, "[]", "{}":
		return true
	}
	return false
}

// codexRolloutFileName is the name the target's own discovery reads: the date and a uuid, under the
// year/month/day directories that follow the same instant.
func codexRolloutFileName(sessionID string, at time.Time) string {
	return fmt.Sprintf("rollout-%s-%s.jsonl", at.UTC().Format("2006-01-02T15-04-05"), sessionID)
}

// codexRolloutDirectory is the sessions subdirectory a rollout for at lives in.
func codexRolloutDirectory(at time.Time) string {
	utc := at.UTC()
	return path.Join(utc.Format("2006"), utc.Format("01"), utc.Format("02"))
}
