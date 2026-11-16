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
		sessionID:  sessionID,
		cwd:        source.Cwd,
		cliVersion: cliVersion,
		clock:      newCodexClock(startedAt),
		callOwners: map[string]struct{}{},
		gitBranch:  source.GitBranch,
		gitRemote:  source.GitRemote,
		opts:       opts,
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

// codexClock hands out envelope timestamps that never go backwards: the target orders records by the
// envelope's timestamp, and a source whose messages are unordered (a DAG, a replayed fork) would
// otherwise write a file whose order contradicts its own times.
type codexClock struct {
	last time.Time
}

func newCodexClock(start time.Time) *codexClock {
	return &codexClock{last: start.UTC()}
}

func (c *codexClock) next(at time.Time) string {
	at = at.UTC()
	if at.IsZero() || at.Before(c.last) {
		at = c.last
	}
	c.last = at
	return at.Format(time.RFC3339Nano)
}

// codexRolloutBuilder accumulates the rollout's lines and the report while it walks the document.
type codexRolloutBuilder struct {
	buffer     bytes.Buffer
	encoder    *json.Encoder
	sessionID  string
	cwd        string
	cliVersion string
	clock      *codexClock
	opts       CodexTargetOptions
	gitBranch  string
	gitRemote  string

	items      []TargetItem
	carried    Carried
	callOwners map[string]struct{}

	lossReasoning         int
	lossSidechain         int
	lossSystemRecords     int
	lossToolRoleMessages  int
	lossUnclassifiedRoles int
	lossEmptyUserMessages int
	lossNonDialogueText   int
	lossUnpairedResults   int
	lossUnresolved        int
	lossOutcomes          int
	lossPreBoundary       int
	lossEmptyInputs       int
	lossGeneratedIDs      int
	lossEmptyResults      int
	lossResultBlocks      int
	foreignToolNames      map[string]struct{}
	toolNameOrder         []string

	// pendingCalls holds the calls this conversion carried that no result has answered yet. It is what
	// an id-less result may be paired with, and only while exactly one call is outstanding: with two
	// or more, the result could belong to any of them, and that ambiguity must not disappear just
	// because a further call arrived.
	pendingCalls map[string]struct{}
	sawBoundary  bool
	turnOpen     bool
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

// writeConversation walks the document once and writes every record it can express.
func (b *codexRolloutBuilder) writeConversation(conv Conversation) {
	for _, message := range conv.Messages {
		b.writeMessage(message)
	}
}

func (b *codexRolloutBuilder) writeMessage(message Message) {
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
	if message.IsSidechain {
		// A subagent's turn, recorded in the same transcript. The target has no subagent attribution
		// for a flat message, so carrying it as an ordinary turn would state that the conversation's
		// own agent said it.
		b.lossSidechain++
		return
	}
	if message.IsSystem {
		b.lossSystemRecords++
		return
	}
	if len(message.ToolResults) == 0 && len(message.ToolCalls) == 0 && strings.TrimSpace(message.Thinking) != "" &&
		strings.TrimSpace(dialogueText(message)) == "" {
		// Nothing but a reasoning trace: the target has no record for a persisted reasoning trace that
		// is not the target's own encrypted artifact (goal 0013 owns that), so it is counted, not
		// written as a message with empty text. The test is the same one the message rule below makes,
		// so a provider that rendered its reasoning into Content cannot smuggle it in as dialogue when
		// its own projection says the message has none.
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
	} else {
		switch text := dialogueText(message); {
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

	for index, call := range message.ToolCalls {
		b.toolCall(message.Ordinal, index, call, message.Timestamp)
	}
	for _, result := range message.ToolResults {
		b.toolResult(result, message.Timestamp)
	}
	// An archive read folds a call's outcome onto the call itself as well as into the result events
	// that outcome came from: internal/db restores a summary it dropped from the call's own sole
	// content-bearing event, so the two are the same evidence. Every event is written above; a summary
	// is written only when no event of that call already holds it, which is what keeps one recorded
	// outcome from arriving as two function_call_output records.
	for _, call := range message.ToolCalls {
		if strings.TrimSpace(call.ResultContent) == "" && call.ResultContentSize == 0 {
			continue
		}
		if resultEventHoldsText(message.ToolResults, call.ToolUseID, call.ResultContent) {
			continue
		}
		b.toolResult(ToolResult{
			ToolUseID:      call.ToolUseID,
			ContentRaw:     call.ResultContent,
			ContentSize:    call.ResultContentSize,
			ContentDecoded: true,
		}, message.Timestamp)
	}
}

// resultEventHoldsText reports whether one of a message's result events already carries a call's
// summary text, which makes that summary a duplicate of an outcome the conversion writes anyway rather
// than a second outcome of the same call. The archive's own summary is the sole event's text byte for
// byte (internal/db's dedup restores a dropped summary from it) or an aggregate of several events, and
// the reader trims a result's text where it presents it, so the comparison is made in that trimmed
// namespace. An empty summary is compared the same way: a blocked archive result is stored as an empty
// summary beside an empty event, and counting it twice would report one dropped outcome as two.
func resultEventHoldsText(results []ToolResult, callID, text string) bool {
	trimmed := strings.TrimSpace(text)
	for _, result := range results {
		if result.ToolUseID != callID {
			continue
		}
		if content, _ := resultText(result); strings.TrimSpace(content) == trimmed {
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

func (b *codexRolloutBuilder) toolCall(ordinal, index int, call ToolCall, at time.Time) {
	if !b.turnOpen {
		b.startTurn(at)
	}
	callID := strings.TrimSpace(call.ToolUseID)
	if callID == "" {
		callID = fmt.Sprintf("%s%d_%d", codexGeneratedCallPrefix, ordinal, index)
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
	b.callOwners[callID] = struct{}{}
	if b.pendingCalls == nil {
		b.pendingCalls = map[string]struct{}{}
	}
	b.pendingCalls[callID] = struct{}{}
	b.carried.ToolCalls++
	b.items = append(b.items, TargetItem{Kind: ItemToolCall, CallID: callID, ToolName: name, InputJSON: input})
}

// solePendingCall answers the one call an id-less result may belong to: exactly one carried call with no
// result yet. Two or more outstanding calls make the pairing a guess -- the result could belong to any
// of them -- and no outstanding call leaves nothing to pair with.
func (b *codexRolloutBuilder) solePendingCall() (string, bool) {
	if len(b.pendingCalls) != 1 {
		return "", false
	}
	for callID := range b.pendingCalls {
		return callID, true
	}
	return "", false
}

func (b *codexRolloutBuilder) toolResult(result ToolResult, at time.Time) {
	if result.PersistedOutputUnresolved {
		// The source's own parse-time answer that the content is the placeholder naming a side file it
		// could not read, not the tool's output. The text travels as recorded and the report says what
		// it is.
		b.lossUnresolved++
	}
	if strings.TrimSpace(result.Status) != "" {
		b.lossOutcomes++
	}
	callID := strings.TrimSpace(result.ToolUseID)
	if callID == "" {
		pending, ok := b.solePendingCall()
		if !ok {
			// No call is outstanding, or several are and the result could belong to any of them: the
			// source recorded no id, so the pairing would be an invention.
			b.lossUnpairedResults++
			return
		}
		callID = pending
		b.lossGeneratedIDs++
	} else if _, ok := b.callOwners[callID]; !ok {
		// The result names a call this conversion did not write (the call preceded the source's
		// boundary, or the provider recorded a result whose call is not in the read). A
		// function_call_output without its call is not a pair, and inventing the call would fabricate
		// a tool invocation.
		b.lossUnpairedResults++
		return
	}
	text, blocks := resultText(result)
	b.lossResultBlocks += blocks
	if text == "" {
		// The target's output field carries text; a result with none leaves nothing to write, and an
		// empty string would read as "the tool returned nothing" rather than "the source recorded no
		// output text".
		b.lossEmptyResults++
		return
	}
	delete(b.pendingCalls, callID)
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

func (b *codexRolloutBuilder) noteToolName(name string) {
	if b.foreignToolNames == nil {
		b.foreignToolNames = map[string]struct{}{}
	}
	if _, ok := b.foreignToolNames[name]; ok {
		return
	}
	b.foreignToolNames[name] = struct{}{}
	b.toolNameOrder = append(b.toolNameOrder, name)
}

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *codexRolloutBuilder) losses() []Loss {
	names := b.toolNameOrder
	sort.Strings(names)
	shown := names
	if len(shown) > b.opts.MaxToolNames {
		shown = shown[:b.opts.MaxToolNames]
	}
	toolDetail := "no tool call was carried"
	if len(names) > 0 {
		toolDetail = fmt.Sprintf(
			"tool calls keep the source's own tool names and recorded arguments (%s%s); the target's "+
				"own tool set may not include them, and a resumed turn answers from them as history "+
				"rather than replaying them",
			strings.Join(shown, ", "), moreNames(len(names)-len(shown)))
	}
	emptyResultsDetail := "every carried tool result had recorded output text"
	if b.lossEmptyResults > 0 {
		emptyResultsDetail = "the target's function_call_output carries output text only; a result " +
			"this read holds no text for is not written, because an empty string would read as an " +
			"empty tool output. A content field can hold no text at all, or hold only blocks this " +
			"target has no field for (counted above as unrepresentable_result_blocks)"
	}
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = "the turns before a compaction boundary stay in the file and are counted " +
			"here: the target applies the boundary to its model-visible history, so those turns are " +
			"recorded history the target does not continue from"
	}
	return []Loss{
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
				"instructions, and a provider's bookkeeping does not belong there",
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
				"it could not read, not the tool's output; that text is what was carried",
		},
		{
			Category:    "tool_result_outcomes",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossOutcomes,
			Detail: "the target's function_call_output has no outcome field; the result's recorded " +
				"output text is carried as it was, and its recorded status is not",
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
			Category:    "pre_compaction_history",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossPreBoundary,
			Detail:      preBoundaryDetail,
		},
	}
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

// dialogueText is the message text the target receives. It is only ever asked for a record whose role
// is the user's or the assistant's: a message with any other role is not dialogue at all, and writeMessage
// counts it instead of writing it (see the role gate there).
//
// The source reader's Content is the provider's rendered transcript, which for a message that carries
// tool calls includes a rendering of the calls themselves (`[Read: <path>]`); the Dialogue projection
// is the same message without those structured payloads. A message that has dialogue has it: the
// projection where the source made one, an explicitly empty projection included -- a provider that
// projected nothing for a message whose Content renders something else (a persisted reasoning trace, a
// tool call) has said that message has no dialogue to carry, and falling back to the rendering would
// state words the source never recorded as spoken. A message whose source classified no projection has
// nothing to duplicate when it carries no calls, so its Content is the text.
func dialogueText(message Message) string {
	if strings.TrimSpace(message.Dialogue) != "" || message.DialogueProjected {
		return message.Dialogue
	}
	if len(message.ToolCalls) > 0 {
		return ""
	}
	return message.Content
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
