package ccc

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// OpenCode (opencode.ai) is a harness whose sessions live inside a SQLite database rather than in one
// file per session: an installed session is a row in `<data directory>/opencode.db`, and the harness's
// own `opencode session import` command is what writes it. What this writer produces is therefore not
// the store itself but the **import document** the harness consumes -- the same `{info, messages}`
// document its own `opencode session export` prints -- and installation is the harness's own importer
// (opencode_install.go). Every fact this file rests on was measured against the installed harness
// (2.2.5) and is recorded in docs/evidence/0007-opencode-target-continuation.md:
//
//   - the document is the harness's own shape, and an import preserves `info.id` and the message and
//     part payloads exactly (measured by structural diff of a re-exported session);
//   - the members the importer refuses to see missing are `info.id`, `info.projectID` (whose value it
//     overwrites), `info.cost`, `info.tokens`, `info.time.updated`, `info.location.directory`, an
//     assistant message's `agent` and `model{id, providerID}`, and -- for a tool part -- `state.input`,
//     the `state` body for its status, and its own `time` member; everything else this writer leaves out
//     rather than filling with an invented value;
//   - the tool states it accepts are `completed` (with a non-empty content text) and `error` (with
//     `error.type` and `error.message`); `pending` and `running` are refused, so a call with no
//     recorded result has no shape here and is left out and counted;
//   - a compaction message is accepted with `summary`, `status` and `reason`, and `reason` is one of
//     the harness's own words (`auto`/`manual`);
//   - `info.model` is the binding a resumed turn runs under, and a session without one resumes under
//     the home's configured default model.
//
// The writer's own source-side rules are the family's (pi_target.go, route_shared.go): which record is
// dialogue, how a result pairs with its call, which records a flat target cannot hold, and every loss
// category's count.

// OpenCodeTargetFormat names the document an OpenCode target session is written as: the harness's own
// session import document, the JSON its `session export` prints and its `session import` consumes.
const OpenCodeTargetFormat = "opencode-import-document"

// OpenCodeFormatVersion is the OpenCode version this writer's document shape was verified against, and
// the version the report states when the installed binary could not be probed:
//
//	opencode --version   →  opencode v2.2.5
const OpenCodeFormatVersion = "2.2.5"

// The harness's own container facts: the data directory is always the `opencode` directory below the
// XDG data home (measured: `opencode debug paths` answers `data <xdg data home>/opencode`), and the
// store inside it is `opencode.db`.
const (
	openCodeDataDirName = "opencode"
	openCodeDBName      = "opencode.db"
)

// openCodeSessionIDPrefix and openCodeMessageIDPrefix are the harness's own id prefixes, and
// openCodeIDTailLength the length of the tail it writes after them (measured: every session and message
// id in a real store is the prefix, an underscore, and 26 characters of its own alphanumerics). The
// encoding the harness's own generator puts in that tail is not reproduced: measured, an id of any
// string is imported, listed and exported, so this writer mints a fresh identity of the harness's own
// *shape* rather than a value whose meaning it would have to invent.
const (
	openCodeSessionIDPrefix = "ses_"
	openCodeMessageIDPrefix = "msg_"
	openCodeIDTailLength    = 26
	openCodeIDAlphabet      = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// The harness's own message types this writer emits. `system`, `synthetic`, `skill`, `idle`,
// `agent-switched`, `model-switched` and `shell` are records the harness writes for its own bookkeeping
// and lifecycle; a conversion writes none of them (an idle marker and a title are the harness's own to
// record when it runs a session).
const (
	openCodeMessageUser       = "user"
	openCodeMessageAssistant  = "assistant"
	openCodeMessageCompaction = "compaction"
	openCodePartText          = "text"
	openCodePartReasoning     = "reasoning"
	openCodePartTool          = "tool"
	openCodeToolCompleted     = "completed"
	openCodeToolError         = "error"
	// openCodeToolErrorType is the error type the harness's own failed tool parts carry (read out of a
	// real store); the schema requires a type and the source read records none.
	openCodeToolErrorType = "tool.execution"
	// openCodeCompactionReason is the harness's own word for a compaction no user command triggered
	// (`auto`/`manual` are the values its schema accepts, measured). The source read answers no reason,
	// so the value is the target's own vocabulary and the report counts it (compaction_reason).
	openCodeCompactionReason = "auto"
	// openCodeCompactionStatus is the only status a real compaction message was observed with, and the
	// one the inherited reader treats as the boundary itself (`data.Status == "completed" || ""`).
	openCodeCompactionStatus = "completed"
	// openCodeDefaultAgent is the harness's own built-in default primary agent, written when it could not
	// be asked (measured: an empty configuration still lists `build` among its agents).
	openCodeDefaultAgent = "build"
)

// openCodeReasoningOpen/Close are the inherited reader's own rendering of a `reasoning` part: it joins
// every text part and every wrapped reasoning text with line feeds (internal/parser's
// loadOpenCodeV2Messages). The round-trip comparison spells the document's own assistant dialogue the
// same way, because that rendering is what any read of the installed session shows for the parts the
// writer wrote.
const (
	openCodeReasoningOpen  = "[Thinking]\n"
	openCodeReasoningClose = "\n[/Thinking]"
)

// openCodeDocument is the import document: the harness's own `{info, messages}` shape.
type openCodeDocument struct {
	Info     openCodeSessionInfo `json:"info"`
	Messages []openCodeMessage   `json:"messages"`
}

// openCodeSessionInfo is the document's session half. The members without `omitempty` are the ones the
// importer refuses to see missing; `model`, `title`, `subpath` and `outcome` are optional and are left
// out when this build has nothing true to put in them.
type openCodeSessionInfo struct {
	ID        string           `json:"id"`
	ProjectID string           `json:"projectID"`
	Model     *openCodeModel   `json:"model,omitempty"`
	Cost      float64          `json:"cost"`
	Tokens    openCodeTokens   `json:"tokens"`
	Time      openCodeTimeSpan `json:"time"`
	Title     string           `json:"title,omitempty"`
	Subpath   string           `json:"subpath,omitempty"`
	Outcome   string           `json:"outcome,omitempty"`
	Location  openCodeLocation `json:"location"`
}

// openCodeLocation is where the session's working directory is recorded; the importer overwrites the
// directory with the one it was given, which is why the writer and the importer must be handed the same
// directory (the source's own recorded working directory).
type openCodeLocation struct {
	Directory string `json:"directory"`
}

// openCodeModel is the harness's own model reference: the model id, the provider it belongs to, and an
// optional variant. Both `id` and `providerID` are required wherever the harness accepts one.
type openCodeModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitempty"`
}

// openCodeTokens is the session's own usage block; every member is required. This build writes zeros
// (the conversation document carries no usage) and names that as the `usage_zeroed` caveat.
type openCodeTokens struct {
	Input     int64              `json:"input"`
	Output    int64              `json:"output"`
	Reasoning int64              `json:"reasoning"`
	Cache     openCodeTokenCache `json:"cache"`
}

// openCodeTokenCache is the cache half of the usage block.
type openCodeTokenCache struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
}

// openCodeTimeSpan is one record's own time members, in Unix milliseconds. A message carries `created`
// (and, where the harness recorded one, `completed`); a tool part carries `created` and, once its call
// has a result, `completed`. `ran` and `idle` are members the harness itself writes and this build has
// nothing true for.
type openCodeTimeSpan struct {
	Created   int64 `json:"created"`
	Updated   int64 `json:"updated,omitempty"`
	Ran       int64 `json:"ran,omitempty"`
	Completed int64 `json:"completed,omitempty"`
	Idle      int64 `json:"idle,omitempty"`
}

// openCodeMessage is one message, in the harness's own per-type shape: a `user` message carries `text`
// (and `files`); an `assistant` message carries `agent`, `model` and `content`; a `compaction` message
// carries `summary`, `status` and `reason`. The optional members are absent on the types that do not use
// them, which is the shape the harness's own export prints.
type openCodeMessage struct {
	ID      string           `json:"id"`
	Time    openCodeTimeSpan `json:"time"`
	Type    string           `json:"type"`
	Text    string           `json:"text,omitempty"`
	Files   *[]openCodeFile  `json:"files,omitempty"`
	Agent   string           `json:"agent,omitempty"`
	Model   *openCodeModel   `json:"model,omitempty"`
	Content []openCodePart   `json:"content,omitempty"`
	Finish  string           `json:"finish,omitempty"`
	Summary string           `json:"summary,omitempty"`
	Status  string           `json:"status,omitempty"`
	Reason  string           `json:"reason,omitempty"`
}

// openCodeFile is one entry of a user message's own attachments. This build carries none -- the source
// read holds no attachment bytes -- and writes the member empty, the shape the harness's own user
// messages carry.
type openCodeFile struct {
	Name string `json:"name,omitempty"`
	MIME string `json:"mime,omitempty"`
	Data string `json:"data,omitempty"`
}

// openCodePart is one content block of an assistant message: a `text` part (dialogue), a `reasoning`
// part (a persisted trace), or a `tool` part (a call, with the state its recorded result puts it in).
type openCodePart struct {
	Type  string             `json:"type"`
	ID    string             `json:"id,omitempty"`
	Name  string             `json:"name,omitempty"`
	Text  string             `json:"text,omitempty"`
	State *openCodeToolState `json:"state,omitempty"`
	Time  *openCodeTimeSpan  `json:"time,omitempty"`
}

// openCodeToolState is one tool part's state. The importer requires `input`, and reads the body by
// `status`: `completed` needs a non-empty `content` text, `error` needs `error.type` and
// `error.message`. `metadata` is the harness's own record of what the tool did (an exit code, a
// truncation flag); this build has only what the source recorded, which is the result text.
type openCodeToolState struct {
	Status  string            `json:"status"`
	Input   json.RawMessage   `json:"input"`
	Content []openCodeBlock   `json:"content,omitempty"`
	Error   *openCodeToolFail `json:"error,omitempty"`
}

// openCodeBlock is one block of a tool part's own content: the text channel the result travels in.
type openCodeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// openCodeToolFail is a failed tool part's own error, in the harness's own shape: a type and a message.
// The message is the text the source recorded for the result.
type openCodeToolFail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// OpenCodeSession is one converted target session, in memory and ready to install. Data is the exact
// byte string the harness's importer reads; Items is what the writer placed in it, for the round-trip
// check, and is never serialized into a CLI document.
type OpenCodeSession struct {
	SessionID  string       `json:"session_id"`
	CLIVersion string       `json:"cli_version"`
	FileName   string       `json:"file_name"`
	Cwd        string       `json:"cwd,omitempty"`
	StartedAt  time.Time    `json:"started_at,omitempty"`
	Data       []byte       `json:"-"`
	Items      []TargetItem `json:"-"`
	Carried    Carried      `json:"carried"`
	Losses     []Loss       `json:"losses"`
}

// OpenCodeTargetOptions configures one conversion to OpenCode. SessionID and CLIVersion are normally
// left empty: the identity must be fresh and the version is probed or pinned by the caller. Provider,
// Model and Agent are the *target home's* own answers, asked of the harness (the route's Facts step):
// the provider and model a new session of it would run under, and the primary agent it lists. They are
// never the source's.
type OpenCodeTargetOptions struct {
	CLIVersion string
	SessionID  string
	Provider   string
	Model      string
	// ModelVariant is the variant the harness's own model answer carried, where it has one.
	ModelVariant string
	Agent        string
	// ProviderDetail is what the harness's own answer said, or why it was not asked; it words the
	// report's own sentence about the binding this document records.
	ProviderDetail string
	Now            time.Time
	MaxToolNames   int
}

// ConvertToOpenCodeSession turns a Conversation into OpenCode's own session import document: a fresh
// identity, one message per piece of evidence the format can express, and the report of what it cannot
// carry. It writes no file; installation is the caller's step (InstallOpenCodeSession), which hands the
// document to the harness's own importer.
//
// The source session's recorded working directory is required: the importer records it as the session's
// own directory (`--directory`) and refuses a directory that does not exist, so a document written
// without one could not be installed where the harness would read it. A document that records none is
// refused here (ErrNoTargetCwd), before anything is written.
func ConvertToOpenCodeSession(conv Conversation, opts OpenCodeTargetOptions) (OpenCodeSession, error) {
	cwd := strings.TrimSpace(conv.Source.Cwd)
	if cwd == "" {
		return OpenCodeSession{}, fmt.Errorf("%w: the OpenCode importer records the session's working "+
			"directory and refuses a session whose directory is unknown", ErrNoTargetCwd)
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		id, err := openCodeFreshID(openCodeSessionIDPrefix)
		if err != nil {
			return OpenCodeSession{}, err
		}
		sessionID = id
	}
	converter := newOpenCodeConverter(conv, opts, sessionID, cwd)
	if err := converter.convert(); err != nil {
		return OpenCodeSession{}, err
	}
	return converter.session()
}

// openCodeCall is one call this conversion carries, with the result paired to it. A call holds the
// recorded id (or one generated for a call that recorded none) and the source's own call, so the
// archive read's derived result summary can stand in where the read holds no result of its own.
type openCodeCall struct {
	ID     string
	Source ToolCall
	At     time.Time
	Result *ToolResult
	// inferredResultID says the result was paired through a generated call id (a result that recorded
	// none), which is what the generated_call_ids caveat counts beside the calls themselves.
	inferredResultID bool
	// It counts only what is written: a call whose result turns out to be unrepresentable is counted
	// under its own category and does not also claim a generated id.
	generatedID bool
}

// openCodePlan is one message's carried calls, in the message's own order.
type openCodePlan struct {
	calls []*openCodeCall
}

// openCodeConverter is one conversion in progress: the document being built, the report's counters, and
// the paired-call plan the two passes share.
type openCodeConverter struct {
	conv      Conversation
	opts      OpenCodeTargetOptions
	sessionID string
	cwd       string

	document openCodeDocument
	items    []TargetItem
	carried  Carried

	plan  []openCodePlan
	calls map[string]*openCodeCall
	clock openCodeClock
	// ledger is the bookkeeping this family's writers share: the carried calls no result has answered
	// yet, which an id-less result may be paired with, and every tool name in the order it was first
	// written.
	ledger

	// Loss counters, one per category the report names (see losses()).
	lossCoalesced        int
	lossInactiveBranch   int
	lossUnresolvedParent int
	lossSidechain        int
	lossSystemRecords    int
	lossToolRoles        int
	lossUnclassified     int
	lossEmptyUser        int
	lossEmptyAssistant   int
	lossNonDialogue      int
	lossWhitespace       int
	lossReasoning        int
	lossUnpairedCalls    int
	lossUnpairedResults  int
	lossRepeatedResults  int
	lossEmptyResults     int
	lossWhitespaceRes    int
	lossResultBlocks     int
	lossUnresolved       int
	lossStatuses         int
	lossEmptyInputs      int
	lossGeneratedIDs     int
	lossSyntheticCalls   int
	lossPreBoundary      int
	lossEmptySummaries   int
	lossMessageModels    int
	lossModelFromTarget  int
	lossModelUnnamed     int
	lossGeneratedAgent   int
	lossCompactionReason int
	lossCompletion       int
	sawBoundary          bool
}

// openCodeClock hands out message timestamps that never go backwards: the harness orders a session's
// records by the sequence its import assigns, but a record's own `created` member is its evidence, and a
// source whose messages are unordered would otherwise write a document whose times contradict its order.
type openCodeClock struct {
	last int64
}

// next answers one instant in Unix milliseconds, never below the previous answer.
func (c *openCodeClock) next(at time.Time) int64 {
	millis := at.UTC().UnixMilli()
	if at.IsZero() || millis < c.last {
		millis = c.last
	}
	c.last = millis
	return millis
}

// newOpenCodeConverter prepares one conversion, including the clock the document's times come from.
func newOpenCodeConverter(
	conv Conversation, opts OpenCodeTargetOptions, sessionID, cwd string,
) *openCodeConverter {
	start := opts.Now.UTC()
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if first := firstMessageTime(conv.Messages); !first.IsZero() {
		start = first.UTC()
	}
	converter := &openCodeConverter{
		conv:      conv,
		opts:      opts,
		sessionID: sessionID,
		cwd:       cwd,
		calls:     map[string]*openCodeCall{},
		ledger:    ledger{pendingCalls: map[string]struct{}{}},
		plan:      make([]openCodePlan, len(conv.Messages)),
	}
	converter.clock.last = start.UnixMilli()
	converter.document = openCodeDocument{
		Info: openCodeSessionInfo{
			ID: sessionID,
			// The importer requires the member and overwrites its value with the project id its own
			// rule computes for the directory; the empty string is written rather than a hash this
			// build would have to invent.
			ProjectID: "",
			Cost:      0,
			Tokens:    openCodeTokens{},
			Time:      openCodeTimeSpan{Created: start.UnixMilli(), Updated: start.UnixMilli()},
			Location:  openCodeLocation{Directory: cwd},
		},
	}
	if model := converter.sessionModel(); model != nil {
		converter.document.Info.Model = model
	}
	return converter
}

// firstMessageTime answers the first recorded message instant, for the session's own start: the
// document's `info.time.created` should be a time the conversation holds rather than the wall clock the
// conversion happened to run at. It is the first instant a record carries, not the earliest of them, and
// the clock the message times go through raises a record whose own instant is out of order, so the
// document's times never contradict its order.
func firstMessageTime(messages []Message) time.Time {
	for _, message := range messages {
		if !message.Timestamp.IsZero() {
			return message.Timestamp
		}
	}
	return time.Time{}
}

// sessionModel is the session's own model binding: the target home's own answer, both halves required
// (a model with no provider names no endpoint, and the harness's own model references carry both). It
// is absent when the harness was not asked or answered nothing, which the report states.
func (c *openCodeConverter) sessionModel() *openCodeModel {
	provider := strings.TrimSpace(c.opts.Provider)
	model := strings.TrimSpace(c.opts.Model)
	if provider == "" || model == "" {
		return nil
	}
	return &openCodeModel{ID: model, ProviderID: provider, Variant: strings.TrimSpace(c.opts.ModelVariant)}
}

// convert runs the two passes: the tool-evidence plan (which pairs every result with its call, because
// a tool part carries a call and its outcome together), then the message walk.
func (c *openCodeConverter) convert() error {
	c.planToolEvidence()
	if err := c.writeConversation(); err != nil {
		return err
	}
	return nil
}

// planToolEvidence walks the document once to pair every recorded result with the call it answers.
//
// A result with no recorded id is paired with the sole call that has no result yet, exactly as the
// family's writers do (with two or more calls outstanding the result could belong to any of them, and
// that ambiguity must not disappear); a result naming no carried call is counted, never given an
// invented call. A call whose recorded input this build cannot write as an object is still carried (an
// empty object is written for it, counted); what a *result* turns out to be decides at write time
// whether the call has a shape here at all.
func (c *openCodeConverter) planToolEvidence() {
	for index, message := range c.conv.Messages {
		calls := distinctCalls(message, func(_ int, call ToolCall) ToolCall { return call }, &c.lossCoalesced)
		for _, call := range calls {
			id := strings.TrimSpace(call.ToolUseID)
			madeID := false
			if id == "" {
				id = openCodeCallID(index, len(c.plan[index].calls))
				madeID = true
			}
			entry := &openCodeCall{ID: id, Source: call, At: message.Timestamp, generatedID: madeID}
			c.plan[index].calls = append(c.plan[index].calls, entry)
			c.calls[id] = entry
			c.ledger.pendingCalls[id] = struct{}{}
		}
		for _, result := range message.ToolResults {
			id := strings.TrimSpace(result.ToolUseID)
			inferred := false
			if id == "" {
				sole, ok := c.solePendingCall()
				if !ok {
					// No call is outstanding, or more than one is: the result cannot be placed, and
					// guessing a call for it would state an invocation the source never recorded.
					c.lossUnpairedResults++
					continue
				}
				id = sole
				inferred = true
			}
			entry, ok := c.calls[id]
			if !ok {
				c.lossUnpairedResults++
				continue
			}
			if entry.Result != nil {
				// The target's tool part holds one state per call, so a second result for the same call
				// has no place; the first is the recorded outcome.
				c.lossRepeatedResults++
				continue
			}
			stored := result
			entry.Result = &stored
			entry.inferredResultID = inferred
			delete(c.ledger.pendingCalls, id)
		}
	}
	// An archive read carries a call's outcome both as the result events the archive kept on the call
	// and as the derived summary it holds in ResultContent. The events are the recorded evidence and the
	// summary a view of them, so the summary is used only for a call with no result of its own, where it
	// is the only evidence the read holds of that outcome -- the family's rule.
	for _, plan := range c.plan {
		for _, entry := range plan.calls {
			if entry.Result != nil {
				continue
			}
			result, ok := derivedResultSummary(entry.Source, entry.ID)
			if !ok {
				continue
			}
			entry.Result = &result
		}
	}
}

// writeConversation walks the document once and writes every message it can express.
//
// A record the source's own relations place off the line its conversation reached is not written: this
// target's message list has no parent relation to hang a record off, and an assistant or user message
// written into it becomes part of the conversation a resumed turn reads, so an abandoned branch cannot
// travel here as recorded history without being revived as an active turn. It is omitted and counted
// (inactive_branch_records). The walk's own answer about the places it chose travels with it
// (unresolved_source_parents), as on the other flat target.
func (c *openCodeConverter) writeConversation() error {
	inactive, unresolved := inactiveSourceRecords(c.conv.Messages, c.conv.Source.Provider)
	c.lossUnresolvedParent = unresolved
	for index, message := range c.conv.Messages {
		if inactive[index] {
			c.lossInactiveBranch++
			continue
		}
		if err := c.writeMessage(message, c.plan[index].calls); err != nil {
			return err
		}
	}
	return nil
}

// writeMessage writes the messages one source record contributes. A source record is one message here
// where its role is the user's or the assistant's; a record the target has no message for can still
// carry tool evidence, which is written in a message of this conversion's own making (a tool part can
// only live inside an assistant message) and counted as synthesized_tool_call_messages.
func (c *openCodeConverter) writeMessage(message Message, calls []*openCodeCall) error {
	if message.IsSidechain {
		// A subagent's turn, recorded in the same transcript. The target has no subagent attribution: a
		// message written for it would state that the conversation's own agent said it (the `agent`
		// member names the target's own agent, not a source-side subagent).
		c.lossSidechain++
		return nil
	}
	if message.IsCompactBoundary {
		return c.writeCompaction(message)
	}
	if message.IsSystem {
		// A record the provider itself flagged as system-injected (a reminder, a notice). Its own text is
		// not the conversation; a tool call or result recorded on the same record still travels, in a
		// message of this conversion's own making.
		c.lossSystemRecords++
		return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
	}
	role := parser.RoleType(message.Role)
	text := dialogueText(message)
	reasoning := strings.TrimSpace(message.Thinking) != ""
	if reasoning {
		c.lossReasoning++
	}
	switch role {
	case parser.RoleUser:
		switch {
		case text != "" && strings.TrimSpace(text) == "":
			c.lossWhitespace++
			return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
		case text != "":
			if err := c.writeUserMessage(text, message.Timestamp); err != nil {
				return err
			}
			return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
		case len(calls) == 0 && !reasoning:
			if strings.TrimSpace(message.Content) == "" {
				c.lossEmptyUser++
			} else {
				// The source itself classified this record as carrying no dialogue (a tool result the
				// provider recorded with a user role); writing it as the user's turn would state that
				// the user said the tool's words.
				c.lossNonDialogue++
			}
			return nil
		default:
			return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
		}
	case parser.RoleAssistant:
		return c.writeAssistantMessage(message, text, calls, false)
	default:
		switch role {
		case parser.RoleTool:
			c.lossToolRoles++
		case parser.RoleSystem:
			c.lossSystemRecords++
		default:
			c.lossUnclassified++
		}
		return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
	}
}

// writeUserMessage writes one user message: the dialogue text, and the files member empty (the source
// read carries no attachment bytes; the harness's own user messages carry the member).
func (c *openCodeConverter) writeUserMessage(text string, at time.Time) error {
	id, err := openCodeFreshID(openCodeMessageIDPrefix)
	if err != nil {
		return err
	}
	files := []openCodeFile{}
	c.document.Messages = append(c.document.Messages, openCodeMessage{
		ID:    id,
		Time:  openCodeTimeSpan{Created: c.clock.next(at)},
		Type:  openCodeMessageUser,
		Text:  text,
		Files: &files,
	})
	c.items = append(c.items, TargetItem{Kind: ItemUser, Text: text})
	c.carried.Messages++
	c.carried.UserMessages++
	c.carried.Turns++
	return nil
}

// writeAssistantMessage writes one assistant message: a reasoning part for a persisted trace, a text
// part for the dialogue, and one tool part per carried call, in that order (the shape and the order the
// harness's own assistant messages carry). An empty message is counted rather than written.
//
// synthesized says this message exists because the calls it holds were recorded on a source record the
// target has no message for; it is carried-with-caveat evidence rather than a source message.
func (c *openCodeConverter) writeAssistantMessage(
	message Message, text string, calls []*openCodeCall, synthesized bool,
) error {
	parts := make([]openCodePart, 0, len(calls)+2)
	whitespaceDialogue := false
	if trace := message.Thinking; strings.TrimSpace(trace) != "" {
		parts = append(parts, openCodePart{Type: openCodePartReasoning, Text: trace})
	}
	switch {
	case text != "" && strings.TrimSpace(text) == "":
		c.lossWhitespace++
		whitespaceDialogue = true
	case text != "":
		parts = append(parts, openCodePart{Type: openCodePartText, Text: text})
	}
	for _, call := range calls {
		part, ok, err := c.toolPart(call, message.Timestamp)
		if err != nil {
			return err
		}
		if ok {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		if synthesized || whitespaceDialogue {
			// The calls it was made for all turned out to have no shape here, or the record's only
			// dialogue was whitespace: the count that names it is already spent, and an empty record is
			// not counted a second time.
			return nil
		}
		c.lossEmptyAssistant++
		return nil
	}
	id, err := openCodeFreshID(openCodeMessageIDPrefix)
	if err != nil {
		return err
	}
	// The harness's importer requires this record's own completion instant: measured on 2.2.5, an
	// assistant message whose `time` carries `created` alone is accepted and *silently dropped* -- the
	// store holds no row for it and the harness's own exporter does not show it (the structural read-back
	// is what caught it). The source's read answers one instant per record, not a span, so the value
	// written is that instant: the record reads as a zero-length turn rather than as a duration nobody
	// recorded, and the report names the choice (assistant_completion_instant).
	created := c.clock.next(message.Timestamp)
	span := openCodeTimeSpan{Created: created, Completed: created}
	c.document.Messages = append(c.document.Messages, openCodeMessage{
		ID:      id,
		Time:    span,
		Type:    openCodeMessageAssistant,
		Agent:   c.agent(),
		Model:   c.messageModel(message),
		Content: parts,
	})
	if synthesized {
		c.lossSyntheticCalls++
	}
	c.lossCompletion++
	c.carried.Messages++
	c.carried.AssistantMessages++
	if dialogue := openCodePartsDialogue(parts); strings.TrimSpace(dialogue) != "" {
		c.items = append(c.items, TargetItem{Kind: ItemAssistant, Text: dialogue})
	}
	return nil
}

// writeSynthesizedToolEvidence writes the calls and results a record carries whose own text is not
// written: a tool part can only live inside an assistant message, so the calls are written in a message
// of this conversion's own making, counted (synthesized_tool_call_messages). A record with no tool
// evidence contributes nothing.
func (c *openCodeConverter) writeSynthesizedToolEvidence(calls []*openCodeCall, at time.Time) error {
	if len(calls) == 0 {
		return nil
	}
	return c.writeAssistantMessage(Message{Timestamp: at}, "", calls, true)
}

// writeCompaction writes one compaction message: the boundary record the harness's own format has, with
// the summary the source kept in place of the history before it.
func (c *openCodeConverter) writeCompaction(message Message) error {
	if strings.TrimSpace(message.Content) == "" {
		c.lossEmptySummaries++
	}
	id, err := openCodeFreshID(openCodeMessageIDPrefix)
	if err != nil {
		return err
	}
	c.lossPreBoundary = c.carried.Messages
	c.sawBoundary = true
	c.lossCompactionReason++
	c.document.Messages = append(c.document.Messages, openCodeMessage{
		ID:      id,
		Time:    openCodeTimeSpan{Created: c.clock.next(message.Timestamp)},
		Type:    openCodeMessageCompaction,
		Summary: message.Content,
		Status:  openCodeCompactionStatus,
		Reason:  openCodeCompactionReason,
	})
	c.items = append(c.items, TargetItem{Kind: ItemCompactionBoundary, Text: message.Content})
	c.carried.Messages++
	c.carried.CompactionBoundaries++
	return nil
}

// toolPart writes one call's tool part from the result paired to it, or answers that the call has no
// shape here.
//
// The state must carry the recorded input object (an empty object is written where none was recorded,
// counted), a non-empty result text for `completed`, and a type and message for `error`; the importer
// accepts no other state (measured: `pending` and `running` are refused), and a call whose result holds
// no text at all is left out and counted rather than written with an invented outcome.
func (c *openCodeConverter) toolPart(call *openCodeCall, at time.Time) (openCodePart, bool, error) {
	if call.Result == nil {
		// The source recorded the call and no result: the target has no state for a call that has not
		// finished, so the call is left out and counted.
		c.lossUnpairedCalls++
		return openCodePart{}, false, nil
	}
	text, blocks := resultText(*call.Result)
	c.lossResultBlocks += blocks
	if call.Result.PersistedOutputUnresolved {
		c.lossUnresolved++
	}
	if strings.TrimSpace(text) == "" {
		if text == "" {
			c.lossEmptyResults++
		} else {
			c.lossWhitespaceRes++
		}
		return openCodePart{}, false, nil
	}
	input, ok := openCodeInputObject(call.Source.InputJSON)
	if !ok {
		c.lossEmptyInputs++
	}
	state := &openCodeToolState{Status: openCodeToolCompleted, Input: input}
	switch status := strings.ToLower(strings.TrimSpace(call.Result.Status)); status {
	case "errored", "error", "failed", "failure":
		state.Status = openCodeToolError
		state.Error = &openCodeToolFail{Type: openCodeToolErrorType, Message: text}
	default:
		if status != "" && status != openCodeToolCompleted {
			// The harness's own state vocabulary has the two words above and no other; any other status
			// word has no field here, and the result travels as the recorded text under `completed`.
			c.lossStatuses++
		}
		state.Content = []openCodeBlock{{Type: openCodePartText, Text: text}}
	}
	if call.generatedID || call.inferredResultID {
		c.lossGeneratedIDs++
	}
	// The vocabulary holds the calls' own names: a call that recorded none is not a name this report
	// lists, so an empty one is not noted.
	if name := strings.TrimSpace(call.Source.Name); name != "" {
		c.noteToolName(name)
	}
	c.carried.ToolCalls++
	c.carried.ToolResults++
	c.items = append(c.items,
		TargetItem{Kind: ItemToolCall, CallID: call.ID, ToolName: call.Source.Name, InputJSON: call.Source.InputJSON},
		TargetItem{Kind: ItemToolResult, CallID: call.ID, Text: text},
	)
	// The part's own time is required by the importer: the call's recorded instant when there is one,
	// and the result's own instant for `completed` (the message the result was recorded on, whose
	// timestamp the caller passed), both through the clock so a source whose messages are unordered
	// cannot write a part whose end precedes its start.
	created := c.clock.next(call.At)
	completed := c.clock.next(at)
	return openCodePart{
		Type:  openCodePartTool,
		ID:    call.ID,
		Name:  call.Source.Name,
		State: state,
		Time:  &openCodeTimeSpan{Created: created, Completed: completed},
	}, true, nil
}

// openCodeInputObject answers the call's recorded input as the object the importer requires: the
// recorded JSON when it is an object, and an empty object (counted as empty_tool_inputs) otherwise,
// because the state's `input` member may be empty but must exist.
func openCodeInputObject(raw string) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && json.Valid([]byte(trimmed)) && strings.HasPrefix(trimmed, "{") {
		return json.RawMessage(trimmed), true
	}
	return json.RawMessage("{}"), false
}

// agent is the agent every assistant message this conversion writes names: the target home's own
// primary agent, asked of the harness, or the harness's built-in default when it could not be asked.
func (c *openCodeConverter) agent() string {
	if agent := strings.TrimSpace(c.opts.Agent); agent != "" {
		return agent
	}
	c.lossGeneratedAgent++
	return openCodeDefaultAgent
}

// messageModel is the model reference one assistant message records: the source's own model id where the
// read answered one (the words' provenance), the target home's configured id where the read answered none
// and the harness answered, and the target home's own provider as the endpoint label where the harness
// was asked -- the read answers no endpoint, and the member cannot be left out.
//
// The member itself is required (measured on 2.2.5: the importer refuses a document whose assistant
// message carries no model at all), but its values need not be: where neither the read nor the harness
// answered a model, the id is written empty (measured: an empty id is imported, stored and exported as
// written), and the report's own details name the absence rather than a value this run cannot support.
// Every record's decision is counted (message_model_attribution); the records the read answered no model
// for are split by outcome (target_model_id_attribution counts the ones the target home's id was written
// for, and its detail names the ones left empty).
func (c *openCodeConverter) messageModel(message Message) *openCodeModel {
	provider := strings.TrimSpace(c.opts.Provider)
	model := strings.TrimSpace(message.Model)
	c.lossMessageModels++
	if model == "" {
		if model = strings.TrimSpace(c.opts.Model); model != "" {
			c.lossModelFromTarget++
		} else {
			c.lossModelUnnamed++
		}
	}
	return &openCodeModel{ID: model, ProviderID: provider}
}

// openCodePartsDialogue answers one assistant message's dialogue in the shape a read of the installed
// session shows it: the inherited readers join a message's text parts and its wrapped reasoning traces
// with line feeds, so that is the text this route compares a read-back against (the route's own read-back
// rule, spelled in opencode_install.go). The writer renders the parts it wrote with it and the document
// check renders the bytes it read with it: one function, because a drift between two copies of the rule is
// exactly the failure the check exists to catch.
func openCodePartsDialogue(parts []openCodePart) string {
	var texts []string
	for _, part := range parts {
		switch part.Type {
		case openCodePartText:
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		case openCodePartReasoning:
			if part.Text != "" {
				texts = append(texts, openCodeReasoningOpen+part.Text+openCodeReasoningClose)
			}
		}
	}
	return strings.Join(texts, "\n")
}

// session assembles the finished conversion: the document's bytes, the items the round-trip check
// compares a read-back against, and the report.
func (c *openCodeConverter) session() (OpenCodeSession, error) {
	last := c.clock.last
	if last > c.document.Info.Time.Updated {
		c.document.Info.Time.Updated = last
	}
	data, err := json.Marshal(c.document)
	if err != nil {
		return OpenCodeSession{}, fmt.Errorf("encoding the OpenCode import document: %w", err)
	}
	c.carried.Bytes = len(data)
	loss := c.losses()
	return OpenCodeSession{
		SessionID:  c.sessionID,
		CLIVersion: c.opts.CLIVersion,
		FileName:   c.sessionID + ".json",
		Cwd:        c.cwd,
		StartedAt:  time.UnixMilli(c.document.Info.Time.Created).UTC(),
		Data:       data,
		Items:      c.items,
		Carried:    c.carried,
		Losses:     loss,
	}, nil
}

// openCodeFreshID mints one fresh identity in the harness's own spelling: its own prefix, an underscore
// and openCodeIDTailLength characters of its own alphanumerics from the system's random source. The
// value's own meaning (the harness's generator encodes a time in it) is not reproduced; measured, an id
// of any string is imported, listed and exported, and a fresh random identity is what keeps one
// conversion from colliding with another.
func openCodeFreshID(prefix string) (string, error) {
	raw := make([]byte, openCodeIDTailLength)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a target session identity: %w", err)
	}
	tail := make([]byte, openCodeIDTailLength)
	for index, value := range raw {
		tail[index] = openCodeIDAlphabet[int(value)%len(openCodeIDAlphabet)]
	}
	return prefix + string(tail), nil
}

// openCodeCallID mints the id of a call that recorded none, from its own place in the message: two
// id-less calls in one message are two calls, and a generated id must not collide with a recorded one.
func openCodeCallID(messageIndex, callIndex int) string {
	return fmt.Sprintf("call_%d_%d", messageIndex, callIndex)
}

// openCodeFormatLimits states what this pair of formats cannot express at all, so a consumer reads them
// as capabilities of the pair rather than as this run's omissions.
func openCodeFormatLimits() []FormatLimit {
	return []FormatLimit{
		{
			Category: "session_container",
			Detail: "the harness keeps its sessions inside one SQLite database, not one file per session: " +
				"an installed session is a row in `<data directory>/opencode.db`, and the conversion " +
				"reports that store and the session's own id inside it. The document this conversion " +
				"writes is the harness's own import document, not a file the harness reads by path",
		},
		{
			Category: "session_title_and_outcome",
			Detail: "a session's title, the finish reason of a turn and the idle/outcome marker the " +
				"harness records when a session stops are the harness's own to make: the source read " +
				"holds none of them in the conversation document, and the importer accepts their absence",
		},
		{
			Category: "provider_and_model",
			Detail: "the endpoint label every assistant record states is the target home's own configured " +
				"provider (asked of the harness), not the source's, which the read does not hold; the " +
				"session's own model binding is the target home's answer too, so the source's provider " +
				"is never recorded as the target's",
		},
		{
			Category: "reasoning_native_payload",
			Detail: "a persisted reasoning trace travels as the `reasoning` part's own text; the part's " +
				"`native` member is the harness's record of what the provider actually returned and is " +
				"not written, because the source read holds no such payload",
		},
		{
			Category: "attachments_and_images",
			Detail: "the source read carries no attachment or image bytes, so this conversion has none to " +
				"write and reports no count for them: a per-run \"0 omitted\" would be a claim the read " +
				"cannot support. A tool result's own content field is the exception: it is read as the " +
				"provider recorded it, and a block the target's text channel has no field for is " +
				"reported by count (unrepresentable_result_blocks)",
		},
		{
			Category: "tool_vocabulary",
			Detail: "tool names travel verbatim as recorded evidence; the target's own tool set is " +
				"unaffected by them, and a resumed turn reads them as history rather than replaying them",
		},
		{
			Category: "usage",
			Detail: "the harness requires a usage block on the document's session info and this build " +
				"writes zeros in it: the conversation document carries no usage, and a count this build " +
				"computed would be attributed to a session that did not spend it (see the usage_zeroed " +
				"caveat for the same fact per run)",
		},
	}
}

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (c *openCodeConverter) losses() []Loss {
	toolDetail := toolVocabularyDetail(c.toolNameOrder, c.opts.MaxToolNames, "argument objects")
	preBoundaryDetail := "no compaction boundary was carried"
	if c.sawBoundary {
		preBoundaryDetail = preBoundaryHistoryDetail()
	}
	bindingDetail := openCodeBindingDetail(c.opts.ProviderDetail)
	modelDetail := "the source recorded no model on any message"
	if c.lossMessageModels > 0 {
		modelDetail = openCodeModelDetail(c.lossModelFromTarget, c.lossModelUnnamed,
			strings.TrimSpace(c.opts.Provider) != "")
	}
	modelIDDetail := openCodeModelIDDetail(c.lossModelFromTarget, c.lossModelUnnamed)
	unreadableLinesDetail := malformedLinesLossDetail(c.conv.Fidelity)
	summaryTranscriptDetail := summaryTranscriptLossDetail(c.conv.Fidelity)
	needsRetryDetail := needsRetryLossDetail(c.conv.Fidelity)
	return []Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossReasoning,
			Detail: "OpenCode persists a reasoning trace as a `reasoning` part of the assistant message " +
				"that produced it, and that part is the shape the source's own trace is written into: " +
				"the trace travels as the source recorded it, with no provider-native payload forged " +
				"for it. A resumed turn reads it as history rather than replaying it",
		},
		{
			Category:    "sidechain_messages",
			Disposition: DispositionOmitted,
			Count:       c.lossSidechain,
			Detail: "a sidechain message is a subagent's turn recorded in the same transcript; the agent " +
				"member an assistant message carries names an agent of the target home, so carrying a " +
				"subagent's turn as an ordinary message would state that the conversation's own agent " +
				"said it. A conversion's unit is the selected session",
		},
		{
			Category:    "system_records",
			Disposition: DispositionOmitted,
			Count:       c.lossSystemRecords,
			Detail: "a record the provider flagged as system-injected carries its own bookkeeping text, " +
				"not the conversation; only the record's own text is omitted, and a tool call or result " +
				"recorded on the same record is still written (in a message of this conversion's own " +
				"making, counted as synthesized_tool_call_messages)",
		},
		{
			Category:    "tool_role_messages",
			Disposition: DispositionOmitted,
			Count:       c.lossToolRoles,
			Detail: "a message the source recorded with the tool role is the tool's own output, and a " +
				"tool part is where a tool's words belong: the text is carried there, paired with the " +
				"call it answers, and never as a user or assistant message, which would state that the " +
				"conversation's own user or agent said the tool's words",
		},
		{
			Category:    "synthesized_tool_call_messages",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossSyntheticCalls,
			Detail: "these messages are this conversion's: a tool part can only live inside an assistant " +
				"message, and the source recorded these calls and results on a record this conversion " +
				"writes no message of its own for (a system record, a role the format has no message " +
				"for, or a user record whose dialogue the format cannot hold). The message carries the " +
				"recorded calls and their results and no dialogue",
		},
		{
			Category:    "unclassified_role_messages",
			Disposition: DispositionOmitted,
			Count:       c.lossUnclassified,
			Detail: "a record whose role this build does not classify has no message in the target to go " +
				"to; writing it as a user or assistant message would state a role the source never recorded",
		},
		{
			Category:    "empty_user_messages",
			Disposition: DispositionOmitted,
			Count:       c.lossEmptyUser,
			Detail: "a user record with neither dialogue text nor tool evidence carries nothing the " +
				"target can show, so no message is written for it",
		},
		{
			Category:    "empty_assistant_messages",
			Disposition: DispositionOmitted,
			Count:       c.lossEmptyAssistant,
			Detail: "an assistant record with neither dialogue, a reasoning trace nor tool evidence " +
				"carries nothing the target can show, so no message is written for it (the inherited " +
				"reader produces that shape for a usage-only row)",
		},
		{
			Category:    "non_dialogue_content",
			Disposition: DispositionOmitted,
			Count:       c.lossNonDialogue,
			Detail: "the source itself classified these records as carrying no dialogue, and their " +
				"content is not dialogue it rendered either (a tool result the provider recorded with a " +
				"user role): the target has no message that would not state the user said those words, " +
				"so the text is not written",
		},
		{
			Category:    "whitespace_only_dialogue",
			Disposition: DispositionOmitted,
			Count:       c.lossWhitespace,
			Detail: "the source recorded dialogue text that holds nothing but whitespace; a written text " +
				"part would be evidence the source never recorded as spoken, and the inherited reader " +
				"reads a message left with no text and no tool evidence as no message at all. The text " +
				"is not written, and any tool evidence the same record carries still is",
		},
		{
			Category:    "unpaired_tool_calls",
			Disposition: DispositionOmitted,
			Count:       c.lossUnpairedCalls,
			Detail: "the source recorded these calls with no result at all; the harness's importer " +
				"accepts only a completed or failed tool state (measured: `pending` and `running` are " +
				"refused), so a call that has not finished has no shape here and is left out. Writing " +
				"it with an invented error or an empty output would fabricate an outcome",
		},
		{
			Category:    "unpaired_tool_results",
			Disposition: DispositionOmitted,
			Count:       c.lossUnpairedResults,
			Detail: "a tool result whose call this conversion did not carry has no pair in the target, " +
				"and fabricating the call would state a tool invocation the source never recorded",
		},
		{
			Category:    "repeated_tool_results",
			Disposition: DispositionOmitted,
			Count:       c.lossRepeatedResults,
			Detail: "the source recorded more than one result for one call; the harness's tool part holds " +
				"one state per call, so the first recorded result is the one carried and the extra " +
				"records are named here rather than merged into a state the source never recorded",
		},
		{
			Category:    "empty_tool_results",
			Disposition: DispositionOmitted,
			Count:       c.lossEmptyResults,
			Detail: "the importer refuses a completed tool state whose content holds no text (measured), " +
				"and this read holds no text for these results: the call is left out and counted rather " +
				"than written with an invented output. A content field can hold no text at all, or hold " +
				"only blocks this target has no field for (counted above as unrepresentable_result_blocks)",
		},
		{
			Category:    "whitespace_only_tool_results",
			Disposition: DispositionOmitted,
			Count:       c.lossWhitespaceRes,
			Detail: "the source recorded output text that holds nothing but whitespace; the importer " +
				"refuses a completed tool state whose content holds no text after its own validation, so " +
				"the output is not written and is named here rather than reported as a result that " +
				"recorded no text",
		},
		{
			Category:    "unrepresentable_result_blocks",
			Disposition: DispositionOmitted,
			Count:       c.lossResultBlocks,
			Detail: "a tool part's content carries a block's own text and nothing else: a block the source " +
				"recorded with other content (an image payload, a block type this build does not read) is " +
				"not written. The count is of blocks, not of results, and a block whose text was carried " +
				"is not counted",
		},
		{
			Category:    "unresolved_tool_outputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossUnresolved,
			Detail: "the source's own parse said these results are the placeholder naming a side file it " +
				"could not read, not the tool's output; that text is what was carried, and only results " +
				"that were written are counted here",
		},
		{
			Category:    "tool_result_statuses",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossStatuses,
			Detail: "the harness's own tool states are `completed` and `error`; a result whose recorded " +
				"status says the call failed is written as an `error` state whose message is the recorded " +
				"output text, and any other status word is not written at all -- the output travels as " +
				"recorded under `completed`. Only results that were written are counted",
		},
		{
			Category:    "empty_tool_inputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossEmptyInputs,
			Detail: "the source recorded no argument object for these calls (or recorded one that is not " +
				"a JSON object); the harness's tool state carries an `input` member that must exist, so " +
				"an empty object was written",
		},
		{
			Category:    "generated_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossGeneratedIDs,
			Detail: "the source recorded no tool-use id for these calls or results; the target pairs a " +
				"tool part with the call its state belongs to, so one was generated from the message's " +
				"own position",
		},
		{
			Category:    "tool_name_vocabulary",
			Disposition: DispositionCarriedWithCaveat,
			Count:       len(c.toolNameOrder),
			Detail:      toolDetail,
		},
		{
			Category:    "message_model_attribution",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossMessageModels,
			Detail:      modelDetail,
		},
		{
			Category:    "target_model_binding",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(c.sessionModel() != nil),
			Detail:      bindingDetail,
		},
		{
			Category:    "target_agent",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossGeneratedAgent,
			Detail: "the harness's assistant records name the agent that produced them, and the source " +
				"read holds no agent; every converted assistant message names the target home's own " +
				"primary agent, asked of the harness (" + openCodeDefaultAgent + " is its built-in " +
				"default, written when it could not be asked). Naming the source's agent would name an " +
				"agent the target home need not define",
		},
		{
			Category:    "usage_zeroed",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(len(c.document.Messages) > 0),
			Detail: "the harness requires a usage block on the document's session info, and the " +
				"conversation document carries no usage, so the block's counts are zero. A zero count is " +
				"not a claim that the words cost nothing: the source's provider billed the source's " +
				"session, whose totals are recorded there and nowhere in this document, and this build " +
				"declines to attribute them, or a computed guess, to a session that did not spend them",
		},
		{
			Category:    "target_model_id_attribution",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossModelFromTarget + c.lossModelUnnamed,
			Detail:      modelIDDetail,
		},
		{
			Category:    "compaction_reason",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossCompactionReason,
			Detail: "a compaction message's `reason` member is one of the harness's own words and the " +
				"source read answers none: `" + openCodeCompactionReason + "` is written, which describes " +
				"the harness's own vocabulary rather than a reason the source recorded. The boundary " +
				"itself and its summary are the source's own evidence",
		},
		{
			Category:    "empty_compaction_summary",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossEmptySummaries,
			Detail: "the source recorded a compaction boundary whose summary holds no text; the harness's " +
				"compaction message carries the summary it was given, and an empty one still marks the " +
				"boundary, so it is written empty rather than invented",
		},
		{
			Category:    "assistant_completion_instant",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossCompletion,
			Detail: "the harness's importer requires every assistant message's own `time.completed`, and " +
				"a message whose `time` carries `created` alone is accepted and then dropped without an " +
				"error (measured on 2.2.5: no row, no `session export` entry -- the structural read-back " +
				"is what shows it). The source's read answers one instant per record rather than a span, " +
				"so the record's own instant is written as its completion: the message reads as a " +
				"zero-length turn instead of a duration nobody recorded",
		},
		{
			Category:    "inactive_branch_records",
			Disposition: DispositionOmitted,
			Count:       c.lossInactiveBranch,
			Detail: "the source's own relations record these turns as a branch the conversation left: no " +
				"later record descends from them, so the source's own loader does not build context from " +
				"them. This target's message list has no parent relation to hang a message off, and a " +
				"message written into it becomes part of the conversation a resumed turn reads, so the " +
				"branch cannot travel here as recorded history without being revived as an active turn; " +
				"it is omitted instead. The branch is still in the source session, and its own " +
				"conversation can be converted on demand like any other history",
		},
		{
			Category:    "unresolved_source_parents",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossUnresolvedParent,
			Detail:      unresolvedSourceParentsDetail(),
		},
		{
			Category:    "pre_compaction_history",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossPreBoundary,
			Detail:      preBoundaryDetail,
		},
		{
			Category:    "malformed_source_lines",
			Disposition: DispositionOmitted,
			Count:       c.conv.Fidelity.MalformedLines,
			Detail:      unreadableLinesDetail,
		},
		{
			Category:    "summary_transcript",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(c.conv.Fidelity.TranscriptFidelity == parser.TranscriptFidelitySummary),
			Detail:      summaryTranscriptDetail,
		},
		{
			Category:    "source_not_current",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(c.conv.Fidelity.NeedsRetry),
			Detail:      needsRetryDetail,
		},
	}
}

// openCodeBindingDetail words what the report says about the session's own model binding: which model
// the harness answered for itself and what that record is for, or -- when nothing was written -- why
// not. The sentence is built here because this is where the answer and its absence are both in hand, and
// the report must state the difference rather than leave a caller to infer it from a missing record.
func openCodeBindingDetail(detail string) string {
	sentence := "the session's own model binding is the target home's own answer, asked of the harness " +
		"(`opencode debug agents`); a session without a binding resumes under whatever model that home's " +
		"configuration selects"
	if strings.TrimSpace(detail) == "" {
		return sentence
	}
	return sentence + ". The harness's own answer: " + detail
}

// openCodeModelDetail words the message_model_attribution caveat from this run's own facts: the endpoint
// label and the model id a converted assistant message states, where each value came from, and what is
// written where neither the read nor the harness answered a model -- the sentence is built here because
// this is where the answer and its absence are both in hand.
func openCodeModelDetail(fromTarget, unnamed int, asked bool) string {
	label := "the target home's own configured provider"
	if !asked {
		label = "the empty string, because the harness was not asked"
	}
	id := "the source's own model id where the read answered one, and the target home's own configured " +
		"id where the read answered none"
	switch {
	case fromTarget > 0 && unnamed > 0:
		id = fmt.Sprintf("the source's own model id where the read answered one, the target home's own "+
			"configured id on %d record(s) the read answered none for, and the empty id on %d record(s) "+
			"neither the read nor the harness answered a model for", fromTarget, unnamed)
	case unnamed > 0:
		id = "the source's own model id where the read answered one, and the empty id where neither the " +
			"read nor the harness answered a model; no model this run cannot name is written"
	}
	return "the harness's model reference carries both a model id and a provider, and the read answers " +
		"the source's model but no endpoint. A converted assistant message's endpoint label is " + label +
		", and its model id is " + id + ": the member records who said the words and which endpoint this " +
		"build could name, not which endpoint served the source's turn. The member itself is required and " +
		"its values need not be: measured on 2.2.5, the importer keeps an empty id as written and refuses " +
		"a document whose assistant message carries no model at all"
}

// openCodeModelIDDetail words the target_model_id_attribution caveat from this run's own facts: the
// assistant records the read answered no model for, and what the member holds for each -- the target
// home's configured id where the harness answered one, and the empty id where neither did, which the
// detail says rather than leaving a value this run cannot support implied.
func openCodeModelIDDetail(fromTarget, unnamed int) string {
	written := "the target home's own configured model id was written"
	switch {
	case fromTarget > 0 && unnamed > 0:
		written = fmt.Sprintf("the target home's own configured model id was written on %d of them, and "+
			"the id is left empty on %d, which is what the member records where no model was answered at all",
			fromTarget, unnamed)
	case unnamed > 0:
		written = "no model was answered for them, so the id is left empty rather than filled with a " +
			"value this run cannot support"
	}
	return "the source read answered no model for these assistant records, and the harness's model " +
		"reference must be present on every one: " + written + ". The session's own binding (above) is " +
		"what a resumed turn runs under, so this member is the record's provenance rather than the model " +
		"that produced the words"
}

// openCodeDocumentDialogue reads the dialogue a written document's own records hold, in the document's
// own order, for the round-trip comparison: every user message's text, and for an assistant message the
// rendering its text and reasoning parts produce under a read (the inherited readers join them).
//
// The question is this narrow on purpose, and it is asked of the document's own bytes rather than of the
// writer's in-memory items, so a document whose part order or text differs from what the writer meant to
// write cannot verify against its own intent.
func openCodeDocumentDialogue(data []byte) ([]string, error) {
	var document openCodeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOpenCodeDocumentShape, err)
	}
	var keys []string
	for _, message := range document.Messages {
		switch message.Type {
		case openCodeMessageUser:
			if message.Text != "" {
				keys = append(keys, dialogueKey(string(ItemUser), message.Text))
			}
		case openCodeMessageAssistant:
			if text := openCodePartsDialogue(message.Content); strings.TrimSpace(text) != "" {
				keys = append(keys, dialogueKey(string(ItemAssistant), text))
			}
		}
	}
	return keys, nil
}

// ErrOpenCodeDocumentShape reports a document whose own bytes are not the shape this writer writes,
// which a structure check reports rather than comparing against a misread of the file.
var ErrOpenCodeDocumentShape = errors.New("the installed document is not an OpenCode import document")
