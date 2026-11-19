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
// them, which is the shape the harness's own export prints. `summary` is a pointer because the importer
// requires the member on a compaction message and accepts an empty string in it (measured), so the
// writer must be able to write `"summary": ""` where no other message type writes the member at all.
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
	Summary *string          `json:"summary,omitempty"`
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
//
// There is no file name: the artifact is a row inside the harness's own store, the document is staged
// under a random temporary name (outside the home) and removed once the importer has it, and nothing this
// build writes carries a name a caller could look for. The report's target_session carries the store's
// path and the import mechanism instead.
type OpenCodeSession struct {
	SessionID  string       `json:"session_id"`
	CLIVersion string       `json:"cli_version"`
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
	// The recorded directory is kept exactly as the read answered it: a path may legitimately hold
	// leading or trailing spaces, and trimming one would hand the document, the importer's `--directory`
	// and the printed resume command a directory the source did not record. Only emptiness is checked,
	// and against the trimmed value, because a directory that holds nothing but whitespace is no path.
	cwd := conv.Source.Cwd
	if strings.TrimSpace(cwd) == "" {
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
	// ResultAt is the instant of the record the carried result was recorded on: the tool part's own
	// `completed` member is that instant, not the call's (a result recorded half a second after its call
	// says the call took half a second, and writing the call's instant twice would state a zero-length
	// call the source did not record). It is the call's own instant where the result came from the
	// archive's derived summary, which is stored on the call row and has no record of its own.
	ResultAt time.Time
	// recordedEvents says the read recorded a result event of this call's -- whether or not this target
	// can write it as an outcome. A progress event is not an outcome here, but it is still evidence the
	// read holds: the archive's summary is a view of the call's recorded events, so a call whose read
	// holds one cannot use that summary as the only evidence of an outcome the events never stated
	// (resultStatesAnOutcome).
	recordedEvents bool
	// inferredResultID says the result was paired through a generated call id (a result that recorded
	// none), which is what the generated_call_ids caveat counts beside the calls themselves.
	inferredResultID bool
	// resultCompletionFallback says the read held no instant of the carried result's own record (a
	// result event the provider recorded without one, an archive summary stored on the call's row), so
	// the part's `completed` member is the containing record's instant: the result_completion_fallbacks
	// caveat counts it, only for a part that is written.
	resultCompletionFallback bool
	// It counts only what is written: a call whose result turns out to be unrepresentable is counted
	// under its own category and does not also claim a generated id.
	generatedID bool
}

// openCodePlan is one message's carried calls, in the message's own order.
type openCodePlan struct {
	calls []*openCodeCall
}

// openCodePlannedCall is one call the plan carries beside the place it holds in its source message:
// the message's calls are coalesced (distinctCalls), and the place is what the seam's pairing answers
// with, so the planned entry has to be filed under the occurrence rather than under the message alone.
type openCodePlannedCall struct {
	index int
	call  ToolCall
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
	// pairings answers, per recorded result occurrence of the document this conversion walks, the call
	// occurrence the seam's own order pairs it with (pairedCallOccurrences), and plannedCalls names the
	// call occurrences this plan carries. This target's message list is one line with no parent relation,
	// so a call the read's line leaves off it is not written at all: a result whose paired occurrence is
	// not planned is omitted and counted (unpaired_tool_results) rather than paired with an older call
	// that happens to share its id, which would state an outcome the source never recorded for it.
	pairings     map[resultOccurrence]callOccurrence
	plannedCalls map[callOccurrence]*openCodeCall
	// usedCallIDs is the namespace a minted id must stay out of: every tool-use id the read recorded
	// (each call's and each result's, reserved by reserveRecordedCallIDs before the plan mints anything)
	// plus every id already minted for an id-less call. The pairing map and the pending set are keyed by
	// id across the whole document, so a generated id reusing a recorded spelling would alias two
	// different calls.
	usedCallIDs map[string]struct{}
	clock       openCodeClock
	// ledger is the bookkeeping this family's writers share: the carried calls no result has answered
	// yet, which an id-less result may be paired with, and every tool name in the order it was first
	// written.
	ledger

	// Loss counters, one per category the report names (see losses()).
	lossCoalesced                   int
	lossInactiveBranch              int
	lossUnresolvedParent            int
	lossSidechain                   int
	lossSystemRecords               int
	lossToolRoles                   int
	lossUnclassified                int
	lossEmptyUser                   int
	lossEmptyAssistant              int
	lossNonDialogue                 int
	lossWhitespace                  int
	lossUnresolvedDialogue          int
	lossUnprojectedDialogue         int
	lossReasoning                   int
	lossNonAssistantReasoning       int
	lossUnpairedCalls               int
	lossUnpairedResults             int
	lossRepeatedResults             int
	lossNonterminal                 int
	lossEmptyResults                int
	lossWhitespaceRes               int
	lossResultBlocks                int
	lossUnresolved                  int
	lossStatuses                    int
	lossSubagentLinks               int
	lossResultSources               int
	lossEmptyInputs                 int
	lossGeneratedIDs                int
	lossResultCompletionFallbacks   int
	lossResultCompletionAdjustments int
	lossSubstitutedRecordInstants   int
	lossSyntheticCalls              int
	lossPreBoundary                 int
	lossEmptySummaries              int
	lossMessageModels               int
	lossModelFromTarget             int
	lossModelUnnamed                int
	lossGeneratedAgent              int
	lossCompactionReason            int
	lossCompletion                  int
	sawBoundary                     bool
}

// openCodeClock hands out message timestamps that never go backwards, and remembers the highest instant
// the document holds. The two are kept apart: one floor orders the document's *messages* against each
// other, the other is the document's own update time.
type openCodeClock struct {
	// lastMessage is the highest `created` handed to a message. The harness orders a session's records by
	// the sequence its import assigns, but a record's own `created` member is its evidence, and a source
	// whose messages are unordered would otherwise write a document whose times contradict its order. Only
	// a message raises it: a tool part's completion is an instant *inside* the document rather than the
	// start of a record, and letting one raise this floor moved a later call's recorded start up to an
	// earlier call's completion (round 15's finding: call A at t=0, call B at t=1, A's result at t=10 --
	// every record of the shape the carried Codex reader produces a message per call for, and B's start
	// became t=10, shortening its recorded span from 19 seconds to 10).
	lastMessage int64
	// lastInstant is the highest instant the document holds: a message's own `created`, or a part's
	// `completed`, which may lie after a later call's own start because calls overlap in the history the
	// source recorded. What it is read for is the session's own `time.updated` (session), which states
	// when the conversation the document holds ends rather than where a later record may begin.
	lastInstant int64
}

// next answers one message instant in Unix milliseconds, never below the previous answer, and records it
// as the document's highest instant too.
//
// The second answer says the instant the read recorded for this message is not the one written, because
// it precedes the previous answer: the document's message order is the read's own, and a message whose
// `created` stood before the one already written would state times contradicting that order. That
// substitution is the writer's to count (`substituted_record_instants`), and only once the message it
// belongs to is actually written: a message released because it turned out to carry nothing
// (writeAssistantMessage) releases its substitution with its reservations. A message the read answered
// no instant for at all takes the previous answer too and is not counted: there is no recorded instant
// to replace, and the value written is the instant the document's order already holds.
func (c *openCodeClock) next(at time.Time) (int64, bool) {
	millis := at.UTC().UnixMilli()
	substituted := !at.IsZero() && millis < c.lastMessage
	if at.IsZero() || millis < c.lastMessage {
		millis = c.lastMessage
	}
	c.lastMessage = millis
	if millis > c.lastInstant {
		c.lastInstant = millis
	}
	return millis, substituted
}

// span answers the two instants of one tool part's own time span: `floor`, the message's own instant,
// for `created` -- the plan stores every call of a record with that record's own instant, and the record
// is the message the part is written into, so the message's own instant is the call's recorded start --
// and the instant of the record the carried result was recorded on for `completed`, never before the
// start. Reading the pair this way keeps every part's span its own: taken through `next` one after the
// other, a sibling call's completion would sit between this call's two instants and move its recorded
// start up to that completion, silently rewriting a duration the source recorded. The pair is adjusted in
// the one way the document's own order allows -- a source whose records are out of order cannot write a
// part whose end precedes its start, so `completed` rises to the message's own instant -- and nothing
// else about it is rewritten. That rise rewrites an instant the read answered, so the part that took it
// is counted (`result_completion_adjustments`): the fallback row answers which instant the read answered
// where it held none of the result's own, and this one answers whether the answer was written, so a
// raised instant is never left to read as the recorded one. The span raises the document's highest
// instant, and never the floor the document's messages are ordered by: a completion that follows a later
// call's own start leaves that call's start where the source recorded it. The `floor` itself is the
// message's *written* instant, which may be one the clock substituted for an earlier recorded one
// (counted beside this row, `substituted_record_instants`): a part inside such a message then starts at
// the substituted instant, and this row's own raise is about the `completed` side alone.
func (c *openCodeClock) span(floor int64, resultAt time.Time) (int64, int64) {
	created := floor
	completed := resultAt.UTC().UnixMilli()
	if resultAt.IsZero() || completed < created {
		completed = created
	}
	if completed > c.lastInstant {
		c.lastInstant = completed
	}
	return created, completed
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
	// The seam's pairing is resolved over the whole document, before the plan drops the records the
	// read's line leaves off it: which call occurrence a recorded result answers is a property of the
	// read's own order, and the plan has to ask that answer rather than accept any carried call sharing
	// the id.
	converter.pairings = pairedCallOccurrences(conv)
	converter.plannedCalls = map[callOccurrence]*openCodeCall{}
	converter.clock.lastMessage = start.UnixMilli()
	converter.clock.lastInstant = start.UnixMilli()
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
// a tool part carries a call and its outcome together), then the message walk. Both passes ask the same
// question about the source's own relations -- which records are on the line the conversation reached --
// so it is answered once here: a record the walk does not write must contribute no tool evidence either,
// or its outcome would travel as if the call it answers were on the active line. The line is the read's
// own and a selection does not redefine it (sourceLineAnswers), so a window that ends on a branch's
// records omits them and counts them rather than reviving the branch.
func (c *openCodeConverter) convert() error {
	inactive, unresolved := sourceLineAnswers(c.conv)
	c.lossUnresolvedParent = unresolved
	c.planToolEvidence(inactive)
	return c.writeConversation(inactive)
}

// planToolEvidence walks the document once to pair every recorded result with the call it answers.
// inactive says which records the source's own relations place off the conversation's line; those
// records, and a subagent's own records, are not planned at all: the walk writes no message for them, so
// a result they carry is not the outcome of a call the document carries, and pairing it would revive
// omitted evidence as the active call's own answer. A call planned here is still dropped at write time
// when its result turns out to have no shape this target can express.
//
// A result with no recorded id is paired with the sole call that has no result yet, exactly as the
// family's writers do (with two or more calls outstanding the result could belong to any of them, and
// that ambiguity must not disappear); a result naming no carried call is counted, never given an
// invented call. A call whose recorded input this build cannot write as an object is still carried (an
// empty object is written for it, counted); what a *result* turns out to be decides at write time
// whether the call has a shape here at all.
//
// A result event the read records as progress rather than an outcome is never the carried result: the
// terminal event the same call recorded is what the part states, and a call whose read holds only
// progress has no outcome here (counted as unpaired_tool_calls). Both shapes are counted separately
// (nonterminal_tool_events), so a progress event is never presented as the call's outcome.
//
// The read's own ids are reserved before any is minted (reserveRecordedCallIDs): a generated id that
// reused a recorded spelling would alias two calls in the maps below.
func (c *openCodeConverter) planToolEvidence(inactive []bool) {
	c.reserveRecordedCallIDs()
	for index, message := range c.conv.Messages {
		if inactive[index] || message.IsSidechain {
			continue
		}
		calls := distinctCalls(message, func(callIndex int, call ToolCall) openCodePlannedCall {
			return openCodePlannedCall{index: callIndex, call: call}
		}, &c.lossCoalesced)
		for _, planned := range calls {
			call := planned.call
			id := strings.TrimSpace(call.ToolUseID)
			madeID := false
			if id == "" {
				id = c.freshCallID(index, len(c.plan[index].calls))
				madeID = true
			}
			entry := &openCodeCall{ID: id, Source: call, At: message.Timestamp, generatedID: madeID}
			c.plan[index].calls = append(c.plan[index].calls, entry)
			c.calls[id] = entry
			c.plannedCalls[callOccurrence{message: index, call: planned.index}] = entry
			c.ledger.pendingCalls[id] = struct{}{}
		}
		for resultIndex, result := range message.ToolResults {
			place := resultOccurrence{message: index, result: resultIndex}
			if !resultStatesAnOutcome(result.Status) {
				// The read records a call's progress as events of the call's own as well as its outcome
				// (internal/parser/termination.go's rule reads the same events: a Codex subagent's
				// `running` notification keeps the call pending until a notification with another status
				// arrives). The harness's tool states are `completed` and `error`, so a progress event is
				// no outcome this target may write -- and it must not consume the call's pending slot
				// either, or the terminal event that follows would read as a repeat of an outcome that
				// was never recorded. It is still marked on the call it names -- through the same pairing
				// a terminal event's own plan uses -- because the archive's derived summary of that call
				// is a view of this event rather than an outcome recorded beside it.
				if entry, ok := c.pairedCallEntry(result, place); ok {
					entry.recordedEvents = true
				}
				c.lossNonterminal++
				continue
			}
			inferred := false
			id := strings.TrimSpace(result.ToolUseID)
			var entry *openCodeCall
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
				entry = c.calls[id]
			} else {
				var ok bool
				entry, ok = c.pairedCallEntry(result, place)
				if !ok {
					// The read's own order pairs the result with a call occurrence, and that occurrence
					// is not in the line this plan carries: the read's line leaves it off, the selection
					// left it out, or the record carrying it is not planned here. Writing the result
					// under an older call that happens to share its id would state that call's outcome
					// the source never recorded.
					c.lossUnpairedResults++
					continue
				}
			}
			if entry == nil {
				c.lossUnpairedResults++
				continue
			}
			if entry.Result != nil {
				// The target's tool part holds one state per call, so a second outcome for the same call
				// has no place; the first recorded outcome is the one carried.
				c.lossRepeatedResults++
				continue
			}
			stored := result
			entry.Result = &stored
			// The part's own completion instant is the result's own record, where the read holds one: a
			// provider may record a result event after the record that carried its call (the committed
			// Codex fixture's call stands at 09:15:05 and its output at 09:15:06), and taking the
			// containing message's instant for both would state a zero-length span the source never
			// recorded (round 12's finding). The containing record's instant is written only where the
			// read holds no instant of the result's own record, and the report counts that fallback
			// (result_completion_fallbacks) rather than presenting it as the recorded one.
			entry.ResultAt = message.Timestamp
			if !result.Timestamp.IsZero() {
				entry.ResultAt = result.Timestamp
			} else {
				entry.resultCompletionFallback = true
			}
			entry.inferredResultID = inferred
			delete(c.ledger.pendingCalls, id)
		}
	}
	// An archive read carries a call's outcome both as the result events the archive kept on the call
	// and as the derived summary it holds in ResultContent. The events are the recorded evidence and the
	// summary a view of them, so the summary is used only for a call with no recorded result event of
	// its own -- where it is the only evidence the read holds of that outcome, the family's rule. An
	// event this target refuses to write counts here too: a call whose only recorded event is progress
	// has no outcome here, and its summary is that progress event's own text, so writing it would state
	// an outcome the source never recorded.
	for _, plan := range c.plan {
		for _, entry := range plan.calls {
			if entry.Result != nil || entry.recordedEvents {
				continue
			}
			result, ok := derivedResultSummary(entry.Source, entry.ID)
			if !ok {
				continue
			}
			entry.Result = &result
			// The summary is stored on the call's own row, not on a result record: the call's instant is
			// the only one the read holds for it, and it keeps the part's own span zero-length rather than
			// inventing a completion time nobody recorded. That is a completion instant the read cannot
			// answer for the result's own record, so it is counted like the event-time fallback above.
			entry.ResultAt = entry.At
			entry.resultCompletionFallback = true
		}
	}
}

// pairedCallEntry answers the planned call one recorded result event names, through the seam's own
// pairing: the call occurrence the read's own ancestry establishes where it does, else the occurrence the
// result's id records nearest before it -- or, for an event that
// recorded no id, the sole call still awaiting a result, where the source's own relation leaves no
// other call it could mean (the writers' disclosed inference). An event whose paired occurrence this
// plan does not carry answers nothing, which is the same answer the plan gives such an event: it is
// counted rather than paired with a call that merely shares the id.
func (c *openCodeConverter) pairedCallEntry(result ToolResult, place resultOccurrence) (*openCodeCall, bool) {
	if id := strings.TrimSpace(result.ToolUseID); id != "" {
		call, ok := c.pairings[place]
		if !ok {
			return nil, false
		}
		entry, ok := c.plannedCalls[call]
		return entry, ok
	}
	id, ok := c.solePendingCall()
	if !ok {
		return nil, false
	}
	entry, ok := c.calls[id]
	return entry, ok
}

// writeConversation walks the document once and writes every message it can express. inactive is the
// answer convert() took once for both passes, so the plan's evidence and this walk agree on which
// records the conversation's own line holds.
//
// A record the source's own relations place off the line its conversation reached is not written: this
// target's message list has no parent relation to hang a record off, and an assistant or user message
// written into it becomes part of the conversation a resumed turn reads, so an abandoned branch cannot
// travel here as recorded history without being revived as an active turn. It is omitted and counted
// (inactive_branch_records). The walk's own answer about the places it chose travels with it
// (unresolved_source_parents), as on the other flat target.
func (c *openCodeConverter) writeConversation(inactive []bool) error {
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
	reasoning := strings.TrimSpace(message.Thinking) != ""
	if role != parser.RoleUser && role != parser.RoleAssistant {
		// Only a user or assistant record is dialogue: a record the source wrote with any other role --
		// a tool's own output (the native tool role several providers keep), a system notice that was
		// not flagged as one, or a role this build does not classify -- must not be written as either.
		// It is counted under its own category instead, and its tool evidence still travels, in a
		// message of this conversion's own making.
		switch role {
		case parser.RoleTool:
			c.lossToolRoles++
		case parser.RoleSystem:
			c.lossSystemRecords++
		default:
			c.lossUnclassified++
		}
		if reasoning {
			// The harness holds a reasoning part on an assistant message (writeAssistantMessage) alone,
			// so this record's trace has no shape here and is omitted, counted rather than claimed as
			// carried.
			c.lossNonAssistantReasoning++
		}
		return c.writeSynthesizedToolEvidence(calls, message.Timestamp)
	}
	// The trace's own counting is role-bound like the dialogue state: the harness holds a `reasoning`
	// part on an assistant message, so only an assistant record's trace travels; a user record's has no
	// shape here and is counted as the omission it is.
	if reasoning {
		if role == parser.RoleAssistant {
			c.lossReasoning++
		} else {
			c.lossNonAssistantReasoning++
		}
	}
	// The dialogue state is asked and counted only for a record this writer goes on to consider as
	// dialogue: a record of another role is not dialogue on any route, and counting its Content under
	// the dialogue categories would report carriage (or omission) of text that was never read as
	// dialogue -- the other three writers ask after their role gate too (round 17's PD-2: on a
	// tool-role record this route counted unprojected_dialogue for text no document held).
	dialogue := dialogueOf(message)
	text := dialogue.Text
	if dialogueLossOf(dialogue) == dialogueLossUnresolved {
		// The read established no dialogue for this record and its Content is not the record's own bytes
		// (dialogueOf): the reader rendered the record's persisted reasoning trace into a Content that
		// also holds the record's text, or the record carries calls whose renderings a reader may have
		// inlined. No text part is written for it -- publishing the reader's rendering as the model's
		// words is the error this rule exists to prevent, and removing one by inference is the guess it
		// refuses -- and the record is counted. Its reasoning part and its tool evidence still travel.
		c.lossUnresolvedDialogue++
	} else if dialogueLossOf(dialogue) == dialogueLossUnprojected {
		// Content, not a projection, is what travels: the report says so per record
		// (unprojected_dialogue) rather than presenting the read's Content as the source's answer.
		c.lossUnprojectedDialogue++
	}
	if role == parser.RoleUser {
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
	}
	return c.writeAssistantMessage(message, text, calls, false)
}

// writeUserMessage writes one user message: the dialogue text, and the files member empty (the source
// read carries no attachment bytes; the harness's own user messages carry the member).
func (c *openCodeConverter) writeUserMessage(text string, at time.Time) error {
	id, err := openCodeFreshID(openCodeMessageIDPrefix)
	if err != nil {
		return err
	}
	files := []openCodeFile{}
	created, substituted := c.clock.next(at)
	if substituted {
		c.lossSubstitutedRecordInstants++
	}
	c.document.Messages = append(c.document.Messages, openCodeMessage{
		ID:    id,
		Time:  openCodeTimeSpan{Created: created},
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
	// The record's own instant is taken before its parts' own: the message precedes the parts it carries
	// in the document's own order, so its `created` member is taken first -- taken last, the later instant
	// of the record a result was recorded on would become the message's own time. A record that turns out
	// to carry nothing releases the reservation below, so a message the document does not hold cannot
	// raise a later record's floor; the substitution the clock answered is counted only where the message
	// is actually written, so a released message releases it too.
	reservedMessage, reservedInstant := c.clock.lastMessage, c.clock.lastInstant
	created, substituted := c.clock.next(message.Timestamp)
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
		part, ok, err := c.toolPart(call, created)
		if err != nil {
			return err
		}
		if ok {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		// The message is not written, so neither its own instant nor the completions its parts were given
		// stands in the document: both reservations are released, and a later record's own instant is not
		// raised by a message the document does not hold.
		c.clock.lastMessage, c.clock.lastInstant = reservedMessage, reservedInstant
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
	// The message's own instant, and with it the `created` member of every part inside it (span), is the
	// clock's answer: where that answer is the previous written message's instead of this record's own
	// instant, the substitution is counted (`substituted_record_instants`) rather than left to read as
	// the recorded start -- the parts' spans are read from this one instant, so a recorded call→result
	// duration can be shortened by it.
	if substituted {
		c.lossSubstitutedRecordInstants++
	}
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
	// The member is written even when the summary is empty: the importer requires it on a compaction
	// message (measured), and an empty summary still marks the boundary. A pointer is what keeps it
	// absent on every other message type while an empty string stays present here.
	summary := message.Content
	created, substituted := c.clock.next(message.Timestamp)
	if substituted {
		c.lossSubstitutedRecordInstants++
	}
	c.document.Messages = append(c.document.Messages, openCodeMessage{
		ID:      id,
		Time:    openCodeTimeSpan{Created: created},
		Type:    openCodeMessageCompaction,
		Summary: &summary,
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
// The state must carry the input object the part's `input` member was written with (an empty object
// where the source recorded none, or recorded one this target cannot write, counted), a non-empty result
// text for `completed`, and a type and message for `error`; the importer accepts no other state
// (measured: `pending` and `running` are refused), and a call whose result holds no text at all is left
// out and counted rather than written with an invented outcome. A result whose recorded status word
// names neither a failure nor `completed` is the same kind of answer: the vocabulary has no truthful
// state for it, so the call is left out and the result counted (tool_result_statuses) rather than
// written under `completed` or `error`.
func (c *openCodeConverter) toolPart(call *openCodeCall, floor int64) (openCodePart, bool, error) {
	if call.Result == nil {
		// The source recorded the call and no result: the target has no state for a call that has not
		// finished, so the call is left out and counted.
		c.lossUnpairedCalls++
		return openCodePart{}, false, nil
	}
	text, blocks := resultText(*call.Result)
	c.lossResultBlocks += blocks
	if strings.TrimSpace(text) == "" {
		if text == "" {
			c.lossEmptyResults++
		} else {
			c.lossWhitespaceRes++
		}
		return openCodePart{}, false, nil
	}
	input, ok := openCodeInputObject(call.Source.InputJSON)
	var state *openCodeToolState
	switch status := strings.ToLower(strings.TrimSpace(call.Result.Status)); status {
	case "errored", "error", "failed", "failure":
		state = &openCodeToolState{
			Status: openCodeToolError,
			Input:  input,
			Error:  &openCodeToolFail{Type: openCodeToolErrorType, Message: text},
		}
	case "", openCodeToolCompleted:
		state = &openCodeToolState{
			Status:  openCodeToolCompleted,
			Input:   input,
			Content: []openCodeBlock{{Type: openCodePartText, Text: text}},
		}
	default:
		// A recorded status word with a third meaning -- the `cancelled` outcome the inherited Vibe and
		// Amp readers preserve, a provider's own word -- has no state the target can state truthfully:
		// its vocabulary is `completed` and `error`, and its own reader treats any other word as a call
		// that has not finished (internal/parser/opencode.go's terminal rule reads only those two), so a
		// part written under either word would claim an outcome the source did not record. The call is
		// left out and the result counted rather than presented as a completion or a failure.
		c.lossStatuses++
		return openCodePart{}, false, nil
	}
	// The carried-with-caveat categories are counted here, past every omission guard: a call a guard
	// above left out carries no input and no result, and counting it as carried -- with the caveat that
	// its recorded input was replaced by the empty object, or that the text travelled as the source's
	// own unresolved placeholder -- would claim evidence the document does not hold.
	if !ok {
		c.lossEmptyInputs++
	}
	if call.Result.PersistedOutputUnresolved {
		c.lossUnresolved++
	}
	if strings.TrimSpace(call.Result.Source) != "" {
		// The read named which native record carried the result's text (a result block in a message, a
		// `function_call_output`, a wait call's output event, a subagent notification). The harness's
		// tool state carries the text and no such label -- its own read answers its own for its own
		// shapes -- so the source's label is omitted and counted rather than dropped in silence.
		c.lossResultSources++
	}
	if call.generatedID || call.inferredResultID {
		c.lossGeneratedIDs++
	}
	if strings.TrimSpace(call.Source.SubagentSessionID) != "" {
		// The source recorded which session this call spawned: the inherited Codex parser sets the child
		// session id on a spawn_agent call, and the archive read carries the same column. The harness's
		// tool part carries an id, a name, an input and a state and has no field for a child session, so
		// the link has no shape to travel in -- and a conversion's unit is the selected session, not the
		// sessions it spawned, so the child is neither read nor converted. Counted rather than dropped in
		// silence: the call's part is written below.
		c.lossSubagentLinks++
	}
	if call.resultCompletionFallback {
		c.lossResultCompletionFallbacks++
	}
	// The vocabulary holds the calls' own names: a call that recorded none is not a name this report
	// lists, so an empty one is not noted.
	if name := strings.TrimSpace(call.Source.Name); name != "" {
		c.noteToolName(name)
	}
	c.carried.ToolCalls++
	c.carried.ToolResults++
	// The item carries the input the part's own `input` member was written with, not the source's
	// spelling of it: a value this target could not write as an object was replaced by the empty object
	// (counted above), and the round-trip walk compares what the store holds against what was written.
	// The source's own input is still the thing the run accounts for, which is what the empty_tool_inputs
	// caveat states.
	c.items = append(c.items,
		TargetItem{Kind: ItemToolCall, CallID: call.ID, ToolName: call.Source.Name, InputJSON: string(input)},
		TargetItem{Kind: ItemToolResult, CallID: call.ID, Text: text},
	)
	// The part's own time is required by the importer: the call's recorded instant for `created`, and
	// the instant of the record the carried result was recorded on for `completed` (the plan keeps the
	// pairing, so this is the result's own record -- or the call's own instant where the result came from
	// the archive's derived summary). The pair is read through the clock's own span, under the message's
	// instant as the floor, so a sibling's completion cannot move this call's recorded start and an
	// unordered source still cannot write a part whose end precedes its start.
	created, completed := c.clock.span(floor, call.ResultAt)
	// The span's one adjustment is counted here, past every omission guard above and only for a part that
	// is written: where the read answered a non-zero instant for the result that precedes the part's own
	// `created` (an out-of-order source: the result's record before the call's), `completed` was raised to
	// the call's recorded start and the read's answer is not what the document holds. The row is kept
	// apart from result_completion_fallbacks on purpose -- that one answers *which* instant the read
	// answered (the containing record's, where it held none of the result's own), this one answers whether
	// the answer was written -- so a part whose result had no instant of its own and whose fallback was
	// raised too is counted in both, and a reader can tell the two reads apart.
	if at := call.ResultAt.UTC().UnixMilli(); !call.ResultAt.IsZero() && at < created {
		c.lossResultCompletionAdjustments++
	}
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
	// The session's own `updated` is the highest instant the document holds -- a message's own instant or
	// a part's completion -- so a document whose last message ends before a result it carries does not
	// state an end that precedes its own evidence.
	if c.clock.lastInstant > c.document.Info.Time.Updated {
		c.document.Info.Time.Updated = c.clock.lastInstant
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

// openCodeCallID is the positional spelling of an id for a call that recorded none: two id-less calls in
// one message are two calls. The spelling alone is not the id -- freshCallID walks it past the ids the
// document already answers -- because the pairing maps are keyed by id, and reusing a recorded spelling
// would answer a result with a call it does not name.
func openCodeCallID(messageIndex, callIndex int) string {
	return fmt.Sprintf("call_%d_%d", messageIndex, callIndex)
}

// reserveRecordedCallIDs registers every tool-use id the read holds -- each call's and each result's,
// across every record the read answered, carried or not -- as one no minted id may reuse. The pairing
// map is keyed by id across the whole document: a generated id reusing a recorded call's spelling would
// replace that call's entry (handing its recorded result to the id-less call), and one reusing a
// result's spelling would take a result this conversion cannot pair and present it as the invented
// call's outcome. A record the source's relations place off the conversation's line is reserved too: its
// call is not carried, so nothing would be replaced -- but a carried result naming it must still be
// answered as unpaired rather than as the outcome of a call this conversion invented an id for.
func (c *openCodeConverter) reserveRecordedCallIDs() {
	c.usedCallIDs = make(map[string]struct{})
	for _, message := range c.conv.Messages {
		for _, call := range message.ToolCalls {
			if id := strings.TrimSpace(call.ToolUseID); id != "" {
				c.usedCallIDs[id] = struct{}{}
			}
		}
		for _, result := range message.ToolResults {
			if id := strings.TrimSpace(result.ToolUseID); id != "" {
				c.usedCallIDs[id] = struct{}{}
			}
		}
	}
}

// freshCallID answers the id one id-less call is written under: the positional spelling, walked upward
// by a suffix until it is one the document does not already answer. The candidates are read from
// usedCallIDs, so no minted id can name a recorded call or result, and two id-less calls never share an
// id.
func (c *openCodeConverter) freshCallID(messageIndex, callIndex int) string {
	base := openCodeCallID(messageIndex, callIndex)
	id := base
	for suffix := 1; ; suffix++ {
		if _, taken := c.usedCallIDs[id]; !taken {
			break
		}
		id = fmt.Sprintf("%s_%d", base, suffix)
	}
	c.usedCallIDs[id] = struct{}{}
	return id
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

// openCodeUnestablishedOutcome is this route's own answer for a record whose dialogue was not
// established (dialogueUnestablished), passed to dialogueLossRows: what the writer does not write and
// where the record's reasoning goes. The harness's assistant message holds a reasoning part, so a trace
// travels where the record's role is the assistant's -- as that message's reasoning part -- and has no
// shape on a record of another role, where it is counted as omitted (non_assistant_reasoning); no text
// part is written either way.
const openCodeUnestablishedOutcome = "no text part is written for it, this count is not a claim that " +
	"the record wrote none, and the record's own reasoning still travels as the assistant message's " +
	"reasoning part where the record's role is the assistant's -- the role the harness holds that part " +
	"on alone -- while a trace on a record of another role has no shape here and is counted as omitted " +
	"(non_assistant_reasoning)"

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
	losses := []Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossReasoning,
			Detail: "OpenCode persists a reasoning trace as a `reasoning` part of the assistant message " +
				"that produced it, and that part is the shape the source's own trace is written into: " +
				"the trace travels as the source recorded it, with no provider-native payload forged " +
				"for it. A trace recorded on a record of another role has no such part; that " +
				"omission is counted separately (non_assistant_reasoning). A resumed turn reads the " +
				"trace it reads as history rather than replaying it",
		},
		{
			Category:    "non_assistant_reasoning",
			Disposition: DispositionOmitted,
			Count:       c.lossNonAssistantReasoning,
			Detail: "the harness writes a reasoning trace as a `reasoning` part of the assistant " +
				"message that produced it and of no other role, so a trace recorded on a record whose " +
				"role is not the assistant's -- a user prompt carrying one, or a record of a role " +
				"this format has no message for -- has no shape here and is omitted. The record's " +
				"own text is governed by its own category; this count names the trace alone",
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
	}
	// The two rows the dialogue states are reported under come from the one constructor
	// (dialogueLossRows), with this route's own answer for an unestablished record.
	losses = append(losses, dialogueLossRows(c.lossUnresolvedDialogue, c.lossUnprojectedDialogue,
		openCodeUnestablishedOutcome)...)
	return append(losses, []Loss{
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
			Detail: "the source recorded these calls with no outcome: either no result at all, or only " +
				"progress events whose text says the call is still working (the events themselves are " +
				"counted as nonterminal_tool_events). The harness's importer accepts only a completed or " +
				"failed tool state (measured: `pending` and `running` are refused), so a call that has not " +
				"finished has no shape here and is left out. Writing it with an invented error or an empty " +
				"output would fabricate an outcome",
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
			Detail: "the source recorded more than one outcome for one call; the harness's tool part holds " +
				"one state per call, so the first recorded outcome is the one carried and the extra " +
				"outcomes are named here rather than merged into a state the source never recorded",
		},
		{
			Category:    "nonterminal_tool_events",
			Disposition: DispositionOmitted,
			Count:       c.lossNonterminal,
			Detail: "the read records a call's progress as events of its own beside the call's outcome " +
				"(the shape the inherited termination rule reads: a Codex subagent's `running` " +
				"notification streams in while its call is still executing, and the RooCode/Kilo chunk " +
				"readers record streamed chunks the same way). The harness's tool states are `completed` " +
				"and `error`, so a progress event's text is not written: doing so would state an outcome " +
				"the source had not recorded when it wrote those words. The terminal result the same call " +
				"recorded is what the part carries, and a call whose read holds only progress has no " +
				"outcome here (counted as unpaired_tool_calls)",
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
			Category:    "tool_result_sources",
			Disposition: DispositionOmitted,
			Count:       c.lossResultSources,
			Detail: "the read names which native record carried each result's text (a result block " +
				"inside a message, a `function_call_output`, a wait call's own output event, a " +
				"subagent notification: internal/parser's readers answer this per shape). The " +
				"harness's tool state carries the text and no such label -- its own read answers " +
				"the analogous question for its own shapes -- so the source's label is omitted, " +
				"counted per result whose part this conversion wrote and that recorded one",
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
			Disposition: DispositionOmitted,
			Count:       c.lossStatuses,
			Detail: "the harness's own tool states are `completed` and `error`; a result whose recorded " +
				"status says the call failed is written as an `error` state whose message is the recorded " +
				"output text. A result whose recorded status word is a third meaning -- the `cancelled` " +
				"outcome the inherited Vibe and Amp readers preserve, a provider's own word -- has no " +
				"truthful state here: writing it under `completed` would state a success the source did " +
				"not record, and the word is not a failure either, so no part is written for that call " +
				"and the result is counted here rather than carried under an outcome it does not name",
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
			Category:    "coalesced_tool_call_projections",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossCoalesced,
			Detail: "the source read recorded these calls once for every file they touched (the " +
				"inherited Codex reader projects one apply_patch into one call per file, every " +
				"projection sharing the call's id and its input); OpenCode's tool part carries one id " +
				"and one input, so the call is written once and what has no field is the per-file " +
				"attribution of a second projection",
		},
		{
			Category:    "subagent_session_links",
			Disposition: DispositionOmitted,
			Count:       c.lossSubagentLinks,
			Detail: "the source recorded which session these carried calls spawned (the inherited " +
				"Codex parser sets a spawn_agent call's child session id, and the archive read carries " +
				"the same column). The harness's tool part carries an id, a name, an input and a state, " +
				"so there is no field for a child session: the call itself is written, and what is " +
				"omitted is the attribution of the session it spawned. The child session is neither " +
				"read nor converted -- a conversion's unit is the selected session -- and the count is " +
				"of the calls this conversion wrote: a call no part was written for is not counted as " +
				"a carried link",
		},
		{
			Category:    "result_completion_fallbacks",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossResultCompletionFallbacks,
			Detail: "a tool part's `completed` member is the instant of the record the carried result " +
				"was recorded on, which a source read answers for a result event of its own " +
				"(`timestamp` on the interchange document's result) and an archive read answers from " +
				"the stored event's own column. Where the read holds no instant of the result's own " +
				"record -- a provider-recorded event with no timestamp, or the archive's derived " +
				"summary, which is stored on the call's own row -- the instant of the record that " +
				"carried the result is written (for a folded event, the call's own record), so the " +
				"part states the call's instant twice rather than one this build invented; only parts " +
				"that were written are counted here, and a fallback instant that itself precedes the " +
				"call's own record is raised to it and counted beside this row " +
				"(result_completion_adjustments)",
		},
		{
			Category:    "result_completion_adjustments",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossResultCompletionAdjustments,
			Detail: "the document's message order is the read's own, and a tool part may not end " +
				"before it starts: where the instant the read answered for the carried result " +
				"precedes the part's own `created` -- an out-of-order source, whose result record " +
				"stands before the record that carried the call -- `completed` is raised to the " +
				"part's `created`, the call's own recorded start, and the read's answer is not what " +
				"the document holds. The instant written is a recorded one (the call's start), never " +
				"one this build invented, and this row counts each written part that took the " +
				"raise; a result whose record holds no instant at all is the other read, counted " +
				"as result_completion_fallbacks",
		},
		{
			Category:    "substituted_record_instants",
			Disposition: DispositionCarriedWithCaveat,
			Count:       c.lossSubstitutedRecordInstants,
			Detail: "the read recorded an instant for each of these messages, and the document does " +
				"not hold it: the document's message order is the read's own, and a message whose " +
				"recorded instant precedes the previous written message's instant is written at that " +
				"instant instead -- an instant the read recorded for a message, never one this build " +
				"invented. A tool part's `created` member is the message's own written instant, so " +
				"the parts inside such a message start at the substituted instant, and a call→result " +
				"span the source recorded is shortened by the substitution wherever the call's own " +
				"record was one of these. The count is per written message -- the substitution is the " +
				"message's own, and a message released (one that turned out to carry nothing) " +
				"releases it -- and the record's own instant is in the source session alone",
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
	}...)
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

// openCodeDocumentShape answers whether one conversion's own document bytes are the shape this writer
// writes: the comparison's precondition, not its evidence. The store's own rows are what the structure
// check reads (the document the importer consumed is the conversion's intent), and a caller that hands
// this check bytes this writer does not write gets that answer instead of a comparison of the store
// against items that do not belong to those bytes.
func openCodeDocumentShape(data []byte) error {
	var document openCodeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%w: %v", ErrOpenCodeDocumentShape, err)
	}
	return nil
}

// ErrOpenCodeDocumentShape reports a document whose own bytes are not the shape this writer writes,
// which a structure check reports rather than comparing against a misread of the file.
var ErrOpenCodeDocumentShape = errors.New("the installed document is not an OpenCode import document")
