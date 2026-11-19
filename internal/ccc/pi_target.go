package ccc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.kenn.io/agentsview/internal/parser"
)

// PiFormatVersion is the Pi version this writer's session format was verified against, when the
// installed target binary cannot be probed for its own version. Pi's session header carries no version
// field of its own (its `version` is the *format* version), so the writer's answer is recorded in the
// document the conversion installs under and reported as `target_session.cli_version_source =
// "writer-pin"`.
const PiFormatVersion = "1.5.2"

// PiSessionVersion is the session format version this writer writes: Pi's `CURRENT_SESSION_VERSION`,
// whose entries are a tree linked by `id`/`parentId`. It is measured, not chosen: Pi 1.5.2 migrates
// anything older on load, and a file that declares a version it is not would be rewritten into one.
const PiSessionVersion = 3

// PiTargetFormat names the on-disk format a Pi target session is written in: the session JSONL Pi
// keeps under `<agent dir>/sessions/--<cwd spelling>--/<timestamp>_<session id>.jsonl`, which is what
// its own loader resumes and what the inherited AgentsView Pi reader parses.
const PiTargetFormat = "pi-session-jsonl"

// piEntryTimestampLayout is the shape of a session entry's timestamp, as Pi's own records carry it:
// ISO 8601 in UTC with exactly three fractional digits (`2026-11-10T02:07:01.784Z`).
const piEntryTimestampLayout = "2006-01-02T15:04:05.000Z"

// Pi session entry types, as the target writes them.
const (
	piEntrySession      = "session"
	piEntryMessage      = "message"
	piEntryCompaction   = "compaction"
	piRoleUser          = "user"
	piRoleAssistant     = "assistant"
	piRoleToolResult    = "toolResult"
	piBlockText         = "text"
	piBlockThinking     = "thinking"
	piBlockToolCall     = "toolCall"
	piStopEndTurn       = "stop"
	piStopToolUse       = "toolUse"
	piGeneratedCallBase = "ccc"
)

// PiTargetOptions configures one conversion. SessionID and CLIVersion are normally left empty: the
// identity must be fresh and the version is probed or pinned by the caller.
type PiTargetOptions struct {
	// SessionID overrides the fresh identity; a test uses it to pin the bytes. An empty value asks for
	// one, which is what a conversion needs.
	SessionID string
	// CLIVersion is the target version the document is written as, when the caller probed one.
	CLIVersion string
	// Now pins the instant the header and the file name are derived from when the document recorded no
	// message time at all.
	Now time.Time
	// MaxToolNames bounds how many tool names the report's vocabulary category spells out.
	MaxToolNames int
}

// piSessionHeader is the file's first line: Pi's own session header. It carries no `id`/`parentId` --
// it is metadata rather than an entry in the tree.
type piSessionHeader struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

// piEntry is one entry in the session's tree. message and payload carry the record kind's own fields,
// and exactly one of them is used per entry; the encoder is what decides which, because Go's own
// struct tags cannot spell two shapes on one record without one of them appearing as null.
type piEntry struct {
	Type      string
	ID        string
	ParentID  *string
	Timestamp string
	message   *piMessage
	payload   any
}

// piMessage is a `message` entry's payload: Pi's own persisted message shape. Usage is written for
// every assistant record -- measured: a resumed Pi run reads `message.usage.input` for each assistant
// message it loads and fails with `undefined is not an object` when one has none, while a zeroed usage
// object is accepted. The numbers Pi writes are its own accounting; this writer's are the source's.
type piMessage struct {
	Role       string   `json:"role"`
	Content    any      `json:"content"`
	Model      string   `json:"model,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	Usage      *piUsage `json:"usage,omitempty"`
	StopReason string   `json:"stopReason,omitempty"`
	ToolCallID string   `json:"toolCallId,omitempty"`
	ToolName   string   `json:"toolName,omitempty"`
	IsError    *bool    `json:"isError,omitempty"`
	Timestamp  int64    `json:"timestamp"`
}

// piUsage is Pi's usage object, as the target's own records carry it. Every number is a count of tokens;
// this writer writes zeros for all of them, which is what Q-0005-01 decided: the counts Pi writes are its
// own accounting of the tokens it spent, and the conversation carried here was not produced by a model
// this session called, so neither the source's own recorded counts nor a number this build computed is
// attributed to this session. usageFor states the same, and names what the zeros cost.
type piUsage struct {
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cacheRead"`
	CacheWrite int     `json:"cacheWrite"`
	Reasoning  int     `json:"reasoning"`
	Total      int     `json:"totalTokens"`
	Cost       *piCost `json:"cost,omitempty"`
}

// piCost is the usage object's cost breakdown, in the shape the target's records carry. All zero for a
// converted session: the words were produced by the source's model, whose provider billed the source's
// session, and no number this build could write would be that provider's rate.
type piCost struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheRead    float64 `json:"cacheRead"`
	CacheWrite   float64 `json:"cacheWrite"`
	Total        float64 `json:"total"`
	FromProvider bool    `json:"totalFromProvider"`
}

// piContentBlock is one block of a message's content array. Exactly the members the block's own kind
// uses are written, so no block claims to be two kinds at once.
type piContentBlock struct {
	Type      string          `json:"type"`
	Thinking  string          `json:"thinking,omitempty"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// piCompaction is the payload of the one entry this writer writes that is not a message, in the target's
// own field names.
type piCompaction struct {
	Summary          string `json:"summary"`
	FirstKeptEntryID string `json:"firstKeptEntryId"`
	// SystemMessage is written as an explicit null: Pi's own compaction entries carry the checkpoint the
	// prompt was rebuilt from, and this writer has none to write, so the record says so rather than
	// leaving the member out for a loader to interpret. The source's boundary reaches this writer as the
	// summary and the boundary flag alone -- the inherited reader projects the record's other members
	// away -- so there is no token count here to carry either, and none is invented.
	SystemMessage *string `json:"systemMessage"`
}

// MarshalJSON writes the compaction payload in the target's own field order: the summary and the kept
// entry first, then the checkpoint member.
func (c piCompaction) MarshalJSON() ([]byte, error) {
	return orderedJSON{write: func(object *jsonObject) error {
		if err := object.field("summary", c.Summary); err != nil {
			return err
		}
		if err := object.field("firstKeptEntryId", c.FirstKeptEntryID); err != nil {
			return err
		}
		return object.field("systemMessage", nil)
	}}.MarshalJSON()
}

// jsonObject writes one JSON object's members in the order they are added: the target's own records
// carry their type first, and encoding/json would sort a map's keys, so the record is built by hand.
// Each addition is either a name and a value to marshal, or a nested object whose members are spliced
// into this one (the entry kinds that are not messages carry their own fields beside the base ones).
type jsonObject struct {
	buffer bytes.Buffer
	count  int
}

func (o *jsonObject) open() {
	o.buffer.WriteByte('{')
	o.count = 0
}

func (o *jsonObject) close() {
	o.buffer.WriteByte('}')
}

// field adds one member. A value the caller left empty is the caller's to skip: every field this writer
// adds is written, so a record never claims a shape it does not have.
func (o *jsonObject) field(name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return o.raw(name, encoded)
}

// raw adds one member whose bytes are already JSON.
func (o *jsonObject) raw(name string, encoded []byte) error {
	if o.count > 0 {
		o.buffer.WriteByte(',')
	}
	o.count++
	nameJSON, err := json.Marshal(name)
	if err != nil {
		return err
	}
	o.buffer.Write(nameJSON)
	o.buffer.WriteByte(':')
	o.buffer.Write(encoded)
	return nil
}

// merge splices one encoded JSON object's members into this one.
func (o *jsonObject) merge(encoded []byte) error {
	trimmed := bytes.TrimSpace(encoded)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return fmt.Errorf("a record payload encoded as %s, which is not a JSON object", trimmed)
	}
	members := bytes.TrimSpace(trimmed[1 : len(trimmed)-1])
	if len(members) == 0 {
		return nil
	}
	if o.count > 0 {
		o.buffer.WriteByte(',')
	}
	o.buffer.Write(members)
	o.count++
	return nil
}

func (o *jsonObject) bytes() []byte {
	return o.buffer.Bytes()
}

// orderedJSON is a JSON value whose members keep the order the record's own fields are added in.
type orderedJSON struct {
	write func(*jsonObject) error
}

func (m orderedJSON) MarshalJSON() ([]byte, error) {
	var object jsonObject
	object.open()
	if m.write != nil {
		if err := m.write(&object); err != nil {
			return nil, err
		}
	}
	object.close()
	return object.bytes(), nil
}

// MarshalJSON writes one entry: the base fields in the target's order, then the record kind's own
// fields, then the message payload when the entry is a message.
func (e piEntry) MarshalJSON() ([]byte, error) {
	return orderedJSON{write: func(object *jsonObject) error {
		if err := object.field("type", e.Type); err != nil {
			return err
		}
		if err := object.field("id", e.ID); err != nil {
			return err
		}
		// The first entry of a session has no parent, and the target spells that as a null rather than by
		// leaving the member out.
		parent := []byte("null")
		if e.ParentID != nil {
			encoded, err := json.Marshal(*e.ParentID)
			if err != nil {
				return err
			}
			parent = encoded
		}
		if err := object.raw("parentId", parent); err != nil {
			return err
		}
		if err := object.field("timestamp", e.Timestamp); err != nil {
			return err
		}
		if e.message != nil {
			if err := object.field("message", *e.message); err != nil {
				return err
			}
		}
		if e.payload != nil {
			encoded, err := json.Marshal(e.payload)
			if err != nil {
				return err
			}
			if err := object.merge(encoded); err != nil {
				return err
			}
		}
		return nil
	}}.MarshalJSON()
}

// MarshalJSON writes the message payload: role and content first, then the fields this writer sets, in
// the order the target's own records carry them, and the message's own timestamp last.
func (m piMessage) MarshalJSON() ([]byte, error) {
	return orderedJSON{write: func(object *jsonObject) error {
		if err := object.field("role", m.Role); err != nil {
			return err
		}
		if err := object.field("content", m.Content); err != nil {
			return err
		}
		for _, field := range []struct {
			name  string
			value any
		}{
			{name: "model", value: m.Model},
			{name: "provider", value: m.Provider},
			{name: "usage", value: m.Usage},
			{name: "stopReason", value: m.StopReason},
			{name: "toolCallId", value: m.ToolCallID},
			{name: "toolName", value: m.ToolName},
			{name: "isError", value: m.IsError},
		} {
			if isZeroField(field.value) {
				continue
			}
			if err := object.field(field.name, field.value); err != nil {
				return err
			}
		}
		return object.field("timestamp", m.Timestamp)
	}}.MarshalJSON()
}

// isZeroField reports whether a message field carries nothing and is left out of the record.
func isZeroField(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == ""
	case *piUsage:
		return typed == nil
	case *bool:
		return typed == nil
	}
	return value == nil
}

// PiSession is one converted target session, in memory and ready to install. Data is the exact byte
// string the target harness reads; Items is what the writer placed in it, for the round-trip check, and
// is never serialized into a CLI document.
type PiSession struct {
	SessionID  string
	FileName   string
	Directory  string
	Cwd        string
	StartedAt  time.Time
	CLIVersion string
	Data       []byte
	Carried    Carried
	Losses     []Loss
	Items      []TargetItem
	// Model is the last model the source's own messages recorded: what each converted assistant entry
	// carries, and what the report names beside the provider it deliberately does not record. Empty when
	// the source recorded none.
	Model string
}

// piFamilyShape names what one target in the Pi family writes beside the conversation the family
// shares. Its zero value is Pi's own shape, which is what the Pi route passes; OMP's own shape is
// measured, not guessed, and each member below carries the measurement that decided it.
type piFamilyShape struct {
	// Name is the harness whose format this is, as the report's own sentences name it ("pi", "omp"). It
	// selects the family's own loss wording; nothing about the records written depends on it.
	Name string
	// titleSlot writes the target's rewritable title-slot line before the session header. OMP writes one
	// into every session it starts (a fixed-width, 256-byte line holding an empty title that the harness
	// later rewrites in place); Pi's files have no such line. A file without one still loads and resumes
	// (measured), so the line is the target's own shape rather than a requirement -- and the shape is
	// what this route must not lose.
	titleSlot bool
	// assistantProvider is the endpoint label every assistant record states. OMP's request builder reads
	// it on every assistant record of the session it resumes and fails to build the request when one is
	// missing (`undefined is not an object (evaluating 'e.length')`, measured on 18.18.7 by dropping the
	// member from one record of an otherwise loadable session), and the read answers no endpoint for a
	// message -- so this is the *target home's* own configured provider, asked of the harness, which is
	// the value the design's provider rule allows where the read answers the model alone. Empty means the
	// family's own rule: Pi writes none, because Pi tolerates a record without one.
	assistantProvider string
	// assistantModelFallback is the model id written on an assistant record whose source message
	// recorded none. OMP requires a non-empty model on every assistant record for the same measured
	// reason; the harness's own configured model is the only model this route can state there, and the
	// report counts every record it was needed for. Empty means the family's own rule (Pi omits the
	// member and reports the record as one whose model the read did not answer).
	assistantModelFallback string
	// sessionBinding is the `<provider>/<id>` a session-level model record states, written before the
	// conversation. OMP restores a session's model from that record when it resolves in the target home,
	// and falls back to the pairs the session's own messages recorded when it does not -- which on a
	// converted session are the source's, a model the target home need not define, so a session without
	// the record is refused by a home that does not know the source's model (`SessionModelRestoreError:
	// Could not restore model <provider>/<id>`, measured). It must stand on the walk the loader makes
	// from the file's leaf: a binding record nothing chains to is not restored (measured) -- which is
	// why the first record's entry hangs off it (sourceTree.preamble). Empty means the family's own
	// rule: Pi's session binds no model to itself and writes no such record.
	sessionBinding string
	// sessionBindingDetail is what the report's own caveat says about the binding record, or about why
	// none was written. It is filled by the route, which is where the harness's answer (or its absence)
	// is known.
	sessionBindingDetail string
}

// piFamilyConversion is one Pi-family target's own half of a conversion: the version a document that
// recorded no probed one is written as, the directory its installer files a session under, and the
// shape of the records its own format carries around the conversation.
type piFamilyConversion struct {
	PinVersion string
	ProjectDir func(cwd string) string
	Shape      piFamilyShape
}

// ConvertToPiSession turns a Conversation into a Pi session: a fresh identity, one entry per piece of
// evidence the target can express, and the report of what it cannot. It writes no file; installation
// is the caller's step (InstallPiSession).
//
// The source session's recorded working directory is required: Pi resolves a session id through the
// project directory of the process's own working directory, so a session installed without one could
// never be resumed while a report called it installed. A document that records none is refused here,
// before anything is written.
func ConvertToPiSession(conv Conversation, opts PiTargetOptions) (PiSession, error) {
	return convertToPiFamilySession(conv, opts, piFamilyConversion{
		PinVersion: PiFormatVersion,
		ProjectDir: PiProjectDir,
	})
}

// convertToPiFamilySession is the one conversion both family targets run: the writer walks the document
// into the family's v3 tree, and the family's own half decides the records around it and the pin a
// document with no probed version is written as.
func convertToPiFamilySession(conv Conversation, opts PiTargetOptions, family piFamilyConversion) (PiSession, error) {
	if strings.TrimSpace(conv.Source.Cwd) == "" {
		return PiSession{}, ErrNoTargetCwd
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		generated, err := uuid.NewV7()
		if err != nil {
			return PiSession{}, fmt.Errorf("generating the session id: %w", err)
		}
		sessionID = generated.String()
	}
	cliVersion := strings.TrimSpace(opts.CLIVersion)
	if cliVersion == "" {
		cliVersion = family.PinVersion
	}
	startedAt := firstRecordedTime(conv.Messages)
	if startedAt.IsZero() {
		startedAt = opts.Now.UTC()
	}
	if startedAt.IsZero() {
		startedAt = time.Unix(0, 0).UTC()
	}
	builder := &piSessionBuilder{
		sessionID:        sessionID,
		cwd:              conv.Source.Cwd,
		cliVersion:       cliVersion,
		shape:            family.Shape,
		opts:             opts,
		clock:            newEntryClock(startedAt, piEntryTimestampLayout),
		headerTimestamp:  startedAt.UTC().Format(piEntryTimestampLayout),
		callNames:        map[string]string{},
		writtenCalls:     map[callOccurrence]piWrittenCall{},
		answeredCalls:    map[callOccurrence][]int{},
		selectionReduced: conv.selectionReduced(),
		pairings:         pairedCallOccurrences(conv),
		ledger:           ledger{pendingCalls: map[string]struct{}{}},
		sourceFidelity:   conv.Fidelity,
		tree:             newSourceTree(conv),
	}
	builder.writePreamble()
	builder.writeConversation(conv)
	if builder.idErr != nil {
		return PiSession{}, builder.idErr
	}
	if err := selectedLeafRefusal(conv, &builder.tree); err != nil {
		return PiSession{}, err
	}
	return PiSession{
		SessionID:  sessionID,
		FileName:   piSessionFileName(sessionID, startedAt),
		Directory:  family.ProjectDir(conv.Source.Cwd),
		Cwd:        conv.Source.Cwd,
		StartedAt:  startedAt,
		CLIVersion: cliVersion,
		Data:       builder.bytes(),
		Carried:    builder.carried,
		Losses:     builder.losses(),
		Items:      builder.items,
		Model:      builder.lastModel,
	}, nil
}

// piSessionFileName is the name Pi's own writer gives a session file: the session's start instant in UTC
// with its `:` and `.` separators replaced by `-`, an underscore, and the session id. An instant with no
// fractional second therefore reads `2026-11-10T02-07-01Z` and one with a millisecond
// `2026-11-10T02-07-01-784Z`; the installed harness resolves a session by its id under the project
// directory whatever the stamp says (measured), so the name is written as the instant the session carries
// rather than padded to a fixed width.
func piSessionFileName(sessionID string, at time.Time) string {
	stamp := at.UTC().Format(time.RFC3339)
	stamp = strings.NewReplacer(":", "-", ".", "-").Replace(stamp)
	return fmt.Sprintf("%s_%s.jsonl", stamp, sessionID)
}

// PiProjectDir spells a working directory the way Pi's own session discovery does, from the code the
// installed harness runs (`getDefaultSessionDirPath`): the leading separator is removed, every `/`,
// `\` and `:` becomes `-`, and the result is wrapped in `--` on both sides. Measured against a real
// home: `/home/azureuser/.hermes/cache/scratch` is filed under
// `--home-azureuser-.hermes-cache-scratch--`.
func PiProjectDir(cwd string) string {
	trimmed := strings.TrimLeft(cwd, `/\`)
	trimmed = strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(trimmed)
	return "--" + trimmed + "--"
}

// piSessionBuilder accumulates the session's entries and the report while it walks the document.
type piSessionBuilder struct {
	buffer     bytes.Buffer
	sessionID  string
	cwd        string
	cliVersion string
	clock      *entryClock
	shape      piFamilyShape
	opts       PiTargetOptions
	carried    Carried
	items      []TargetItem
	idErr      error
	callNames  map[string]string
	// writtenCalls names each call occurrence this walk wrote, with the id, name and item index it was
	// written under. A reduced document's result record states the name and the owning call item of the
	// occurrence the read paired the result with: callNames is keyed by id and the last write wins, so a
	// reused id's later occurrence -- the inactive sibling a window may carry as history -- would
	// otherwise supply its name, and an id two occurrences share cannot say which one an output answers.
	writtenCalls map[callOccurrence]piWrittenCall
	// answeredCalls names, per call occurrence, the records whose written results *stating an outcome*
	// answered it (each record's own position in the document): the completion a reduced document's
	// id-less inference reads. The question is per occurrence and per path -- whether an answer stands on
	// the written walk of the result being serialized -- because a result the window carries on another
	// branch is recorded history there and answers nothing here, while the ledger's file-wide id set (the
	// default path's answer) cannot say which branch an answer belongs to. A recorded event whose status
	// says the call is still running is not an answer and is not recorded here (resultStatesAnOutcome):
	// progress leaves the call outstanding where the result stands.
	answeredCalls map[callOccurrence][]int
	// selectionReduced says the document this conversion walks is one Select produced
	// (Conversation.selectionReduced): the written-ancestry acceptance, the path-restricted inference and
	// the occurrence-keyed name exist for the reduction, and an unflagged run keeps the earlier answers.
	selectionReduced bool
	// pairings answers, per recorded result occurrence of the document this conversion walks, the call
	// occurrence the seam's own pairing names for it (pairedCallOccurrences, or the answers the selection
	// carried beside the reduced document). A document a selection produced checks that occurrence against
	// the written walk (sourceTree.recordOnWrittenPath): the paired occurrence has to stand on the
	// result's own written ancestry, or the file would hold a toolResult whose invocation is off the walk
	// a resumed session makes.
	pairings map[resultOccurrence]callOccurrence
	ledger
	lastModel      string
	sourceFidelity Fidelity

	// tree maps the source's own record relations onto this session's entries, so the file is the
	// source's own shape rather than a flat chain: each entry hangs off the entry of the record the
	// source named as its parent, and a branch the conversation left stays off the walk the target's own
	// loader makes. Its unresolved count is what the report's unresolved_source_parents names.
	tree sourceTree

	writtenMessages           int
	lossReasoning             int
	lossNonAssistantReasoning int
	lossSidechain             int
	lossSystemRecords         int
	lossToolRoles             int
	lossUnclassified          int
	lossEmptyUser             int
	lossEmptyAssistant        int
	lossNonDialogue           int
	lossWhitespace            int
	lossUnresolvedDialogue    int
	lossUnprojectedDialogue   int
	lossUnpaired              int
	lossNonterminal           int
	lossEmptyResults          int
	lossWhitespaceRes         int
	lossResultFallbacks       int
	lossResultInstants        int
	lossResultSources         int
	lossResultBlocks          int
	lossUnresolved            int
	lossStatuses              int
	lossEmptyInputs           int
	lossGeneratedIDs          int
	lossCoalesced             int
	lossCollidingIDs          int
	lossSubagentLinks         int
	lossModels                int
	lossModelFallbacks        int
	lossTitleSlot             int
	lossSessionBinding        int
	lossCarriedCallEntries    int
	lossUnrecordedUsage       int
	lossPreBoundary           int
	lossEmptySummary          int
	sawBoundary               bool
	// headerTimestamp is the session's own start instant, fixed when the builder is made: the header's
	// timestamp is the session's identity rather than a position in the entry stream, so it must not be
	// advanced by the entries written after it.
	headerTimestamp string
}

// writePreamble writes the records a family's own format carries before the conversation, and records
// where the source's first record hangs off them.
//
// OMP's session-level model binding is the one such record this build writes: it is an entry of the
// file's own tree (its id and parentId are the format's), it is not a source record, and the walk the
// harness's loader makes from the file's last entry has to reach it -- so the source's first record's
// entry is chained to it (sourceTree.preamble) rather than left parentless, which is the shape OMP's own
// writer produces too (its first turn's entry hangs off its setting records). A family whose format
// carries no such record writes nothing here and leaves every entry exactly where it was.
func (b *piSessionBuilder) writePreamble() {
	binding := strings.TrimSpace(b.shape.sessionBinding)
	if binding == "" {
		return
	}
	entry := piEntry{
		Type:      piEntryModelChange,
		ID:        b.newEntryID(),
		Timestamp: b.headerTimestamp,
		payload:   piModelChange{Model: binding},
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		if b.idErr == nil {
			b.idErr = fmt.Errorf("encoding the %s record: %w", piEntryModelChange, err)
		}
		return
	}
	b.buffer.Write(encoded)
	b.buffer.WriteByte('\n')
	b.lossSessionBinding++
	b.tree.preamble(entry.ID)
}

// bytes answers the session's own bytes: the family's own title line where it writes one, the header,
// the records the preamble wrote, and then every entry, each terminated by a line feed. It also records
// the size it produced as the conversion's carried byte count, so the reported count is the file's own
// size rather than a second length computed from the same buffer.
func (b *piSessionBuilder) bytes() []byte {
	var session bytes.Buffer
	if b.shape.titleSlot {
		session.Write(piTitleSlotLine(b.headerTimestamp))
		b.lossTitleSlot++
	}
	header := piSessionHeader{
		Type:      piEntrySession,
		Version:   PiSessionVersion,
		ID:        b.sessionID,
		Timestamp: b.headerTimestamp,
		Cwd:       b.cwd,
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		return nil
	}
	session.Write(encoded)
	session.WriteByte('\n')
	session.Write(b.buffer.Bytes())
	b.carried.Bytes = session.Len()
	return session.Bytes()
}

// writeConversation walks the document once and writes every entry it can express. The entries keep the
// read's own order; what the read recorded about the source's relations (Message.SourceUUID and
// Message.SourceParentUUID) is what each entry's parent is decided from.
func (b *piSessionBuilder) writeConversation(conv Conversation) {
	for _, message := range conv.Messages {
		b.writeMessage(message)
	}
}

// write adds one entry to the session, chaining it to the entry the source's own relation names for the
// record it stands for and to the entry before it when the record's first entry has no parent of its
// own (a record written as a second piece of evidence, a conversation's first record). It also records
// what the entry stands for, because the record that follows decides its parent from it -- and the
// record's *final* entry is the one it leaves behind, so a record written as several entries is one
// place in the tree.
func (b *piSessionBuilder) write(entry piEntry) {
	if entry.ID == "" {
		entry.ID = b.newEntryID()
	}
	if entry.ParentID == nil {
		entry.ParentID = b.tree.entryParent()
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		if b.idErr == nil {
			b.idErr = fmt.Errorf("encoding a %s entry: %w", entry.Type, err)
		}
		return
	}
	b.buffer.Write(encoded)
	b.buffer.WriteByte('\n')
	b.tree.wrote(entry.ID)
}

// newEntryID is one entry's id: eight lowercase hex characters, which is the shape Pi's own entries
// carry (the installed harness's `generateId` is a UUID's first eight characters). The UUID is a v4 --
// random in every one of those characters -- and not a v7, whose leading bits are a millisecond clock:
// two entries written in the same millisecond would share their timestamp half, and two entries sharing
// an id collapse the tree the file is.
func (b *piSessionBuilder) newEntryID() string {
	generated, err := uuid.NewRandom()
	if err != nil {
		if b.idErr == nil {
			b.idErr = fmt.Errorf("generating an entry id: %w", err)
		}
		return "00000000"
	}
	return strings.ReplaceAll(generated.String(), "-", "")[:8]
}

func (b *piSessionBuilder) writeMessage(message Message) {
	// The record is registered whether or not it is written: a record the target leaves out has no entry
	// of its own, and the records that hang off it must still be chained to the nearest ancestor this
	// conversion *did* write. Its first entry is chained by the source's own relation, and the origin
	// carries what the next record's parent resolution asks about (this record's identity, its own
	// parent, and whether it is written as a tool result). Cleared however this record ends, so an
	// omitted record cannot leak a relation onto the next one's first entry.
	b.tree.startRecord(message.SourceUUID, message.SourceParentUUID)
	defer b.tree.endRecord()
	if message.IsSidechain {
		// A subagent's turn recorded in the same session file. Pi's own loader keeps sidechain records
		// out of the session's context and the target has no subagent attribution for a flat message,
		// so a written entry would state that the conversation's own agent said it.
		b.lossSidechain++
		return
	}
	if message.IsCompactBoundary {
		b.writeCompaction(message)
		return
	}
	if message.IsSystem {
		// A record the provider itself flagged as system-injected. Pi's system entries are its own
		// prompt and tool loadout; another provider's bookkeeping is not that. The record's tool
		// evidence is still real evidence, so only the text is suppressed.
		b.lossSystemRecords++
		b.writeCarriedCalls(message)
		return
	}
	role := parser.RoleType(message.Role)
	if role != parser.RoleUser && role != parser.RoleAssistant {
		switch role {
		case parser.RoleTool:
			b.lossToolRoles++
		case parser.RoleSystem:
			b.lossSystemRecords++
		default:
			b.lossUnclassified++
		}
		if strings.TrimSpace(message.Thinking) != "" {
			// A trace is written as the `thinking` block of an assistant message (writeAssistantMessage)
			// and of no other role; on this record it has no shape in this format and is omitted,
			// counted rather than claimed as carried.
			b.lossNonAssistantReasoning++
		}
		b.writeCarriedCalls(message)
		return
	}
	dialogue := dialogueOf(message)
	text := dialogue.Text
	if dialogueLossOf(dialogue) == dialogueLossUnresolved {
		// The read established no dialogue for this record and its Content is not the record's own bytes
		// (dialogueOf): the reader rendered the record's persisted reasoning trace into a Content that
		// also holds the record's text, or the record carries calls whose renderings a reader may have
		// inlined. No text is published for it -- the rendering is not words the model wrote, and the
		// removal of one by inference is the guess this rule refuses -- and the record is counted. Its
		// trace still travels below, as Pi's `thinking` block, and its tool evidence with it.
		b.lossUnresolvedDialogue++
	} else if dialogueLossOf(dialogue) == dialogueLossUnprojected {
		// Content, not a projection, is what travels: the report says so per record
		// (unprojected_dialogue) rather than presenting the read's Content as the source's answer.
		b.lossUnprojectedDialogue++
	}
	calls := b.distinctCalls(message)
	hasThinking := strings.TrimSpace(message.Thinking) != ""
	if hasThinking {
		// Pi's `thinking` block is the format's own shape for a persisted reasoning trace and carries no
		// signature this build would have to forge, so the source's trace travels as the source
		// recorded it -- on an assistant message, the only role writeAssistantMessage writes the block
		// on. The Claude route must omit it; this one must not pretend it did, and a user record's
		// trace, which has no shape here, is counted as the omission it is.
		if role == parser.RoleAssistant {
			b.lossReasoning++
		} else {
			b.lossNonAssistantReasoning++
		}
	}
	switch {
	case strings.TrimSpace(text) == "" && !hasThinking && len(message.ToolResults) == 0 && len(message.ToolCalls) == 0:
		if role == parser.RoleAssistant {
			b.lossEmptyAssistant++
		} else if strings.TrimSpace(message.Content) == "" {
			b.lossEmptyUser++
		} else {
			// The source itself classified this record as carrying no dialogue, and its content is a
			// rendering this writer must not put in the user's mouth.
			b.lossNonDialogue++
		}
		return
	case text != "" && strings.TrimSpace(text) == "":
		// Dialogue that holds only whitespace: the inherited Pi reader drops a record left with no text
		// and no tool evidence, so a written entry would be in the file and absent from every read of it.
		// What is dropped is the whitespace, and only it: a record that also recorded a reasoning trace
		// keeps the trace, because Pi's `thinking` block is the trace's own shape and the trace is not
		// dialogue (writing the record through writeCarriedCalls instead would omit it while
		// reasoning_text reported it as carried).
		b.lossWhitespace++
		if role == parser.RoleAssistant && hasThinking {
			b.writeAssistantMessage("", message, calls)
			b.writeToolEvidence(message, calls)
			return
		}
		b.writeCarriedCalls(message)
		return
	}
	if role == parser.RoleUser {
		if text != "" {
			b.writeUserMessage(text, message.Timestamp)
		}
		if len(calls) > 0 {
			// A user record can carry calls of its own -- the reader pairs a tool row onto proposed calls,
			// and a provider that records a call on the same record as the prompt keeps it there. Pi
			// records a call as a block of the assistant message that made it, so the calls travel as the
			// entry writeCarriedCalls builds and are counted as this conversion's, never as the user's own
			// words; the comment it carries applies here exactly.
			b.writeCarriedCalls(message)
			return
		}
		// A user record can also carry tool results -- a provider that records a result on the same
		// record as the prompt. Pi has no one record for both, so the result travels as its own
		// toolResult entry, in the order the source recorded it.
		b.writeToolEvidence(message, calls)
		return
	}
	// An assistant message and its calls are one entry: Pi records a call as a block of the message
	// that made it, so the calls' blocks go into the same content array as the reasoning and the
	// dialogue, and only the results follow as entries of their own.
	b.writeAssistantMessage(text, message, calls)
	b.writeToolEvidence(message, calls)
}

// writeUserMessage writes one user prompt as the target records it: content is a plain JSON string.
func (b *piSessionBuilder) writeUserMessage(text string, at time.Time) {
	b.write(piEntry{
		Type:      piEntryMessage,
		Timestamp: b.clock.next(at),
		message: &piMessage{
			Role:      piRoleUser,
			Content:   text,
			Timestamp: piMessageMillis(at),
		},
	})
	b.writtenMessages++
	b.carried.Messages++
	b.carried.UserMessages++
	b.items = append(b.items, TargetItem{Kind: ItemUser, Text: text})
}

// writeAssistantMessage writes one assistant message as the target records it: one entry whose content
// blocks are the reasoning trace, the dialogue and the calls, in that order.
func (b *piSessionBuilder) writeAssistantMessage(text string, message Message, calls []piProjectedCall) {
	blocks := make([]piContentBlock, 0, 2+len(calls))
	if thinking := strings.TrimSpace(message.Thinking); thinking != "" {
		blocks = append(blocks, piContentBlock{Type: piBlockThinking, Thinking: message.Thinking})
	}
	if text != "" {
		blocks = append(blocks, piContentBlock{Type: piBlockText, Text: text})
	}
	for _, entry := range calls {
		blocks = append(blocks, b.callBlock(entry.index, entry.call))
	}
	b.write(piEntry{
		Type:      piEntryMessage,
		Timestamp: b.clock.next(message.Timestamp),
		message: &piMessage{
			Role:       piRoleAssistant,
			Content:    blocks,
			Model:      b.entryModel(message),
			Provider:   b.shape.assistantProvider,
			Usage:      b.usageFor(),
			StopReason: b.stopReasonFor(calls),
			Timestamp:  piMessageMillis(message.Timestamp),
		},
	})
	b.writtenMessages++
	b.carried.Messages++
	b.carried.AssistantMessages++
	if text != "" {
		b.items = append(b.items, TargetItem{Kind: ItemAssistant, Text: text})
	}
}

// modelField answers the model one assistant entry records, or "" when the source recorded none.
//
// The model travels verbatim: Pi's `model` field means the model that produced these words, which is
// what the source's own message recorded, and the target's own totals then count the turn under the
// model that wrote it rather than under one of the caller's.
//
// Pi's records deliberately carry no provider. Pi's `provider` field means the endpoint that served
// the words, and the inherited read does not answer that: it reports the model each source message
// recorded (Message.Model) but not the endpoint, and its session-level provider names the *harness*
// that wrote the words (`pi`, `claude`, `codex`) -- a label the target's own configuration need not
// define as a provider. Writing that label into the record was measured wrong (0005's review): the
// committed Pi fixture records `example-proxy`, and the conversion wrote `pi` while the report claimed
// the source's own provider travelled verbatim. Pi's records write nothing in its place, and the report
// says so (message_model_attribution; piModelDetail words the family's own answer). A family whose
// format requires a provider states the target home's own configured endpoint instead, and the shape
// carries that value (piFamilyShape.assistantProvider).
func (b *piSessionBuilder) modelField(message Message) string {
	model := strings.TrimSpace(message.Model)
	if model == "" {
		return ""
	}
	b.lastModel = model
	b.lossModels++
	return model
}

// entryModel answers the model one assistant entry states, and is where a family whose format requires
// a non-empty model states the only one it can: the harness's own configured model id, for a record
// whose source message recorded none (piFamilyShape.assistantModelFallback carries the measurement).
// The count it charges is the report's own statement that the model on those records is the target
// home's answer rather than the source's, so a caller reading the file cannot mistake it for evidence
// about who wrote the words.
func (b *piSessionBuilder) entryModel(message Message) string {
	model := b.modelField(message)
	if model != "" || b.shape.assistantModelFallback == "" {
		return model
	}
	b.lossModelFallbacks++
	return b.shape.assistantModelFallback
}

// piModelDetail words the message_model_attribution caveat for one family, because the two formats
// answer the same question differently and each route's report has to say which one it is.
func piModelDetail(family string) string {
	if family == piFamilyOMP {
		return "OMP's message record states which model and which provider produced the words, and its own " +
			"request builder reads both on every assistant record of the session it resumes: a record " +
			"without a provider (or without a model) makes the next turn fail while it builds its request " +
			"(measured on 18.18.7, `undefined is not an object (evaluating 'e.length')`, by dropping the " +
			"member from one record of an otherwise loadable session). The model travels verbatim where " +
			"the source's own message recorded one -- the record states that recorded model -- while the " +
			"provider is the *target home's* own configured endpoint, asked of the harness: the inherited " +
			"read answers no endpoint attribution for a message, and the source's own provider label is a " +
			"name the target home need not define, so the field states the endpoint this session runs " +
			"under rather than one that served the words. A record whose model the read did not answer " +
			"carries the harness's own configured model id for the same measured reason. What the source's " +
			"conversation is, and which model wrote each turn the read answered, is unchanged by it"
	}
	return "Pi's message record states which model and which provider produced the words. The " +
		"model travels verbatim -- it is what the source's own message recorded, and the target's own " +
		"totals then count the turn under the model that wrote it. The provider does not: the " +
		"inherited read answers no endpoint attribution for a message (the Pi reader projects a " +
		"message's model and not its provider), and the read's session-level provider names the " +
		"*harness* that wrote the words -- `claude`, `codex`, `pi` -- which is not an endpoint the " +
		"target's own configuration defines, so no provider is written rather than that label. A Pi " +
		"session binds no provider to itself, so nothing is refused when the resumed session runs " +
		"under another model"
}

// stopReasonFor answers the entry's stop reason: a turn that made calls stopped to make them, and one
// that did not finished its own turn.
func (b *piSessionBuilder) stopReasonFor(calls []piProjectedCall) string {
	if len(calls) > 0 {
		return piStopToolUse
	}
	return piStopEndTurn
}

// usageFor answers the usage object one assistant entry carries. Measured: Pi's own loader reads
// `message.usage.input` for every assistant message it loads, and a message without a usage object
// stops the run with `undefined is not an object (evaluating 'message.usage.input')`, while a zeroed
// usage object is accepted and the turn continues.
//
// Every count is zero, and that is the report's own statement rather than an estimate: the counts Pi
// writes are its own accounting of the tokens it spent, and the conversation carried here was not
// produced by a model this session called -- the source's provider billed the source's session, whose
// numbers are recorded there and nowhere in this document. This build therefore declines to attribute
// either the source's totals or a computed guess to this session, and the loss category says so,
// including what it costs: the target's own estimate of the inherited context is built from these
// numbers, so a long carried history reads to Pi as a cheaper context than it is until the first
// resumed turn reports its own usage.
func (b *piSessionBuilder) usageFor() *piUsage {
	b.lossUnrecordedUsage++
	return &piUsage{Cost: &piCost{}}
}

// writeCompaction writes one compaction boundary as the target's own compaction entry.
//
// firstKeptEntryId is the entry's own id, which is the shape Pi writes for a retain-none compaction:
// the source records the summary and the boundary but not which entries the boundary kept, so claiming
// a kept range would invent one, while the record itself makes Pi rebuild its model-visible history
// from the summary -- the source's own semantic. systemMessage is null because Pi installs its own
// prompt and tool loadout at request time; a checkpoint this writer reconstructed would be a prompt
// nobody recorded.
func (b *piSessionBuilder) writeCompaction(message Message) {
	b.lossPreBoundary = b.carried.Messages
	b.sawBoundary = true
	id := b.newEntryID()
	if strings.TrimSpace(message.Content) == "" {
		b.lossEmptySummary++
	}
	b.write(piEntry{
		Type:      piEntryCompaction,
		ID:        id,
		Timestamp: b.clock.next(message.Timestamp),
		payload: piCompaction{
			Summary:          message.Content,
			FirstKeptEntryID: id,
		},
	})
	b.carried.CompactionBoundaries++
	b.items = append(b.items, TargetItem{Kind: ItemCompactionBoundary, Text: message.Content})
}

// writeCarriedCalls writes the calls one message carries when this conversion writes no message entry
// of that record's own that can carry them: a record the provider flagged as system-injected, one
// written with a role this build writes no message for (a native tool row, a system or unclassified
// role, or a user record whose own entry is text alone), or an assistant record whose dialogue is
// whitespace alone (writeMessage's whitespace branch). Pi records a call as a block of the assistant
// message that made it, so the calls are written in an entry of their own; the entry holds no dialogue,
// because the words the source recorded on the record are the part this conversion does not write, and
// the count says the entry is this conversion's. Its model is the read's own answer for that record,
// stated the same way writeAssistantMessage states it through entryModel: the source's where the read
// answered one, and the family's configured fallback only where it answered none.
func (b *piSessionBuilder) writeCarriedCalls(message Message) {
	calls := b.distinctCalls(message)
	if len(calls) == 0 {
		b.writeToolEvidence(message, calls)
		return
	}
	blocks := make([]piContentBlock, 0, len(calls))
	for _, entry := range calls {
		blocks = append(blocks, b.callBlock(entry.index, entry.call))
	}
	b.write(piEntry{
		Type:      piEntryMessage,
		Timestamp: b.clock.next(message.Timestamp),
		message: &piMessage{
			Role:       piRoleAssistant,
			Content:    blocks,
			Model:      b.entryModel(message),
			Provider:   b.shape.assistantProvider,
			Usage:      b.usageFor(),
			StopReason: piStopToolUse,
			Timestamp:  piMessageMillis(message.Timestamp),
		},
	})
	b.writtenMessages++
	b.carried.Messages++
	b.carried.AssistantMessages++
	b.lossCarriedCallEntries++
	b.writeToolEvidence(message, calls)
}

// writeToolEvidence writes the results one message carries, and the recorded outcomes of calls whose
// only evidence is the summary the read holds beside the call.
//
// The calls themselves are already in the message's own entry where its role has one that can hold them
// (writeAssistantMessage), and are written as an entry of this conversion's own where it does not
// (writeCarriedCalls); either way the call is registered before this runs, so its result has a pair.
// What is left here is the results.
func (b *piSessionBuilder) writeToolEvidence(message Message, calls []piProjectedCall) {
	// The pairing is keyed by the record's own position in the document, which the tree stamps as the
	// record is started (one startRecord per message, so the walk position is that key): writtenMessages
	// counts written entries instead, and a record that writes several entries would move it.
	record := b.tree.record
	for index, result := range message.ToolResults {
		var paired *callOccurrence
		if place, ok := b.pairings[resultOccurrence{message: record, result: index}]; ok {
			paired = &place
		}
		b.writeToolResult(paired, result, message.Timestamp, toolResultCarrierRecord(message))
	}
	for _, entry := range calls {
		call := entry.call
		result, ok := derivedResultSummary(call, call.ToolUseID)
		if !ok {
			continue
		}
		if callHasRecordedResult(message.ToolResults, call.ToolUseID) {
			continue
		}
		// The summary is the call's own evidence and the call was written just above, within this very
		// record: the pairing to hand the writer is that occurrence, so the ancestry check asks about the
		// one place the pair really stands at.
		place := callOccurrence{message: record, call: entry.index}
		b.writeToolResult(&place, result, message.Timestamp, toolResultCarrierRecord(message))
	}
}

// projectedCall is one call the writer writes, with the place it holds in the source message: an
// id-less call's generated id is derived from that place.
type piProjectedCall struct {
	index int
	call  ToolCall
}

// distinctCalls answers one message's calls as this target writes them, in the message's own order: a
// call whose id the message already carried is another projection of that call (the inherited Codex
// reader expands one apply_patch into one call per file it names, every projection sharing the id and
// the input) and not a second invocation.
func (b *piSessionBuilder) distinctCalls(message Message) []piProjectedCall {
	return distinctCalls(message, func(index int, call ToolCall) piProjectedCall {
		return piProjectedCall{index: index, call: call}
	}, &b.lossCoalesced)
}

// callBlock answers one call as the block the target records: Pi's toolCall block, with the recorded
// id, name and argument object. The call is registered here because this is where its identity is
// decided, so a following result can pair with it.
func (b *piSessionBuilder) callBlock(index int, call ToolCall) piContentBlock {
	callID := strings.TrimSpace(call.ToolUseID)
	switch {
	case callID == "":
		// The call's place in the document keys its generated id -- the message's position in the walk
		// and the call's index in that message -- and not the message's own ordinal: an embedding
		// caller may leave ordinals unassigned, and two messages' id-less calls must not collapse into
		// one id, which would make the pairing below read one call where two exist. The plain positional
		// spelling is what a run without a selection writes even where another written call already
		// holds it, exactly as it did before selections existed (goal 0009's D4); a reduced document
		// walks the id past every id the file already states (freshCallID), because a generated id there
		// must not alias a recorded spelling.
		callID = fmt.Sprintf("%s%d_%d", piGeneratedCallBase, b.writtenMessages, index)
		if b.selectionReduced {
			callID = b.freshCallID(callID)
		}
		b.lossGeneratedIDs++
	case b.selectionReduced && b.callIDTaken(callID):
		// A document a selection produced writes each call occurrence the read holds, and the source
		// recorded this id on an earlier occurrence too. The target pairs a call with its output by id --
		// Pi's reader attaches a toolResult to the call whose block registered the id -- so an
		// occurrence written under the shared id could not be told from the earlier one: an output
		// inferred for the earlier occurrence would read back as this one's outcome. The occurrence is
		// written under an id generated from its own place (the same shape an id-less call takes), and
		// the substitution is counted (colliding_call_ids) rather than left to read as the recorded id.
		// A run without a selection keeps the ids the source recorded, exactly as before (goal 0009's
		// D4).
		callID = b.freshCallID(fmt.Sprintf("%s%d_%d", piGeneratedCallBase, b.writtenMessages, index))
		b.lossCollidingIDs++
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "unknown_tool"
	}
	b.noteToolName(name)
	arguments := compactJSON(call.InputJSON)
	if arguments == "" {
		// The source recorded no argument object for this call (or recorded bytes that are not one);
		// the empty object keeps the call's identity and its output paired, and the report says so.
		arguments = "{}"
		b.lossEmptyInputs++
	}
	if strings.TrimSpace(call.SubagentSessionID) != "" {
		// The source recorded which session this call spawned; Pi's toolCall block has no field for a
		// child session, and a conversion's unit is the selected session.
		b.lossSubagentLinks++
	}
	b.callNames[callID] = name
	// The item index is read before the call item is appended, so it is the position that item will
	// take: a result written for this occurrence states it as its owner (TargetItem.OwnerCall).
	b.writtenCalls[callOccurrence{message: b.tree.record, call: index}] = piWrittenCall{id: callID, name: name, item: len(b.items)}
	b.pendingCalls[callID] = struct{}{}
	b.carried.ToolCalls++
	b.items = append(b.items, TargetItem{Kind: ItemToolCall, CallID: callID, ToolName: name, InputJSON: arguments})
	return piContentBlock{Type: piBlockToolCall, ID: callID, Name: name, Arguments: json.RawMessage(arguments)}
}

// callIDTaken answers whether some call this walk already wrote stands under one id: the target pairs a
// call with its output by id, so a second occurrence under the same id cannot be told from the first.
func (b *piSessionBuilder) callIDTaken(callID string) bool {
	_, taken := b.callNames[callID]
	return taken
}

// freshCallID answers an id the document does not already state: base where no written call holds it,
// and otherwise base with a suffix counted until one is free, so a generated id can never alias a
// recorded spelling or another generated one.
func (b *piSessionBuilder) freshCallID(base string) string {
	id := base
	for suffix := 2; b.callIDTaken(id); suffix++ {
		id = fmt.Sprintf("%s_%d", base, suffix)
	}
	return id
}

// piWrittenCall is one call occurrence this writer has written, keyed by the call's own occurrence in
// the document: the id it stands under -- which is not the recorded id where an earlier occurrence
// already held that one (colliding_call_ids) -- the name it was written with, and the index in this
// session's item list of the call item itself, which is what a result written for the occurrence states
// as its owner (TargetItem.OwnerCall), so the round-trip check can assert the attachment the target's
// own reader makes. A result under a reused id needs the name of the occurrence it answers, which the
// file-wide name map cannot give (it holds the last write for the id).
type piWrittenCall struct {
	id   string
	name string
	// item is the position in b.items of the call item this occurrence was written as, the owner a
	// result item states so the round-trip check asserts the reader attaches the output to the right
	// occurrence rather than to whichever call holds an id two occurrences could share.
	item int
}

// resultCallWritten answers whether a recorded result that names a call may be written under that call's
// id on a conversation the read returned: whether some written call stands under the id. A document a
// selection produced asks the seam's own order instead -- the paired occurrence's entry, checked where
// the result's pairing is read (writeToolResult) -- so this check now serves the default path alone, and a
// run without a selection keeps the acceptance its writer always had and is byte-identical to the same
// run before selections existed (goal 0009's D4).
func (b *piSessionBuilder) resultCallWritten(callID string) bool {
	_, ok := b.callNames[callID]
	return ok
}

// soleResultCall answers the call an id-less result may be paired with, with the name its result record
// states, and whether there is exactly one such call. A conversation the read returned asks the ledger's
// own question -- the one call still outstanding anywhere in the walk. A document a selection produced
// asks the narrower one: the sole call occurrence that is outstanding *on the written walk of the
// result's record* (sourceTree.recordOnWrittenPath) and that no written result stating an outcome,
// standing on that walk, has answered (answeredOnWrittenPath). This family's files hold the records the
// read's line leaves off it as recorded history, so the outstanding set can include a call on another
// branch; inferring the result onto such a call would write an outcome whose invocation the resumed
// context cannot see, and `generated_call_ids` discloses the inference rather than a placement. The
// eligible set is counted in call *occurrences*: two occurrences the source recorded under one id are
// two calls, and a result the read left no link for could belong to either, so the inference picks no
// call where two are eligible -- whatever ids they carry. The result is then omitted and counted.
func (b *piSessionBuilder) soleResultCall() (piWrittenCall, callOccurrence, bool) {
	if !b.selectionReduced {
		id, ok := b.solePendingCall()
		if !ok {
			return piWrittenCall{}, callOccurrence{}, false
		}
		return piWrittenCall{id: id, name: b.callNames[id]}, callOccurrence{}, true
	}
	found := piWrittenCall{}
	foundAt := callOccurrence{}
	foundAny := false
	for occurrence, written := range b.writtenCalls {
		if b.answeredOnWrittenPath(occurrence) {
			// A written result on the place now being walked has answered this call occurrence: the call
			// is not outstanding here, whatever a result on another branch did to the call's id.
			continue
		}
		if !b.tree.recordOnWrittenPath(occurrence.message) {
			continue
		}
		if foundAny {
			// A second eligible occurrence: the result could belong to either one, and sharing an id does
			// not merge them -- the read recorded two calls.
			return piWrittenCall{}, callOccurrence{}, false
		}
		found, foundAt, foundAny = written, occurrence, true
	}
	return found, foundAt, foundAny
}

// answeredOnWrittenPath answers whether a written result standing on the place now being walked has
// already answered one call occurrence. A result the writer accepted answers its call where it stands:
// the records whose results answered the occurrence are remembered (answeredCalls) and the question is
// whether any of them stands on the written ancestry of the result being serialized
// (sourceTree.recordOnWrittenPath). An answer on another branch leaves the call outstanding here, which
// the ledger's own file-wide id set cannot express: that set is what cleared a call a sibling branch's
// outcome had answered, so an id-less result was inferred onto a later call the read never paired it
// with although both calls were outstanding on the result's own path.
func (b *piSessionBuilder) answeredOnWrittenPath(occurrence callOccurrence) bool {
	for _, record := range b.answeredCalls[occurrence] {
		if b.tree.recordOnWrittenPath(record) {
			return true
		}
	}
	return false
}

// resultToolName answers the tool name a result record states for a result that names a call: the name
// the file's id map holds for it (the last write for the id). A document a selection produced takes the
// name from the owning call occurrence instead -- the occurrence the read paired the result with, whose
// written name writeToolResult reads off writtenCalls -- so this serves the default path alone and a run
// without a selection keeps the answer its writer always had (goal 0009's D4).
func (b *piSessionBuilder) resultToolName(callID string) string {
	return b.callNames[callID]
}

// writeToolResult writes one result as a toolResult entry, and answers whether it was written.
// carrierRecord is the source record a record hanging off this outcome would read as its parent
// (toolResultCarrierRecord): it is given to the tree so a later record naming that source record can be
// chained through this entry rather than off the walk the target's loader makes. paired is the call
// occurrence the seam's own pairing names for the result (pairedCallOccurrences), or the call a derived
// summary came from; nil where the read holds no call occurrence for it.
//
// On a document a selection produced the id and name the entry is written under are the owning
// occurrence's *emitted* ones, never the recorded id read back off the result, and the result item states
// that occurrence's call item (TargetItem.OwnerCall): an occurrence may have been written under an id
// generated from its own place (colliding_call_ids, or a call that recorded none), and a result written
// under the recorded spelling would read back as the outcome of whichever occurrence does hold it. A
// result that names no id whose owner is not known from a pairing is inferred from the outstanding set,
// and only there is an inference disclosed (generated_call_ids).
func (b *piSessionBuilder) writeToolResult(paired *callOccurrence, result ToolResult, at time.Time, carrierRecord string) bool {
	callID := strings.TrimSpace(result.ToolUseID)
	// resultIDInferred says the pairing below is the only id this result has: the source recorded none of
	// its own. The counter it feeds is incremented past the omission guards, together with the written
	// result, because an inferred pairing on a result this conversion does not write is not evidence the
	// file holds.
	resultIDInferred := false
	toolName := ""
	answered := callOccurrence{}
	switch {
	case b.selectionReduced && paired != nil:
		// The pairing names the occurrence this result answers -- the read's own answer for a recorded
		// result, or the call a derived summary came from -- and the id and name the file holds for it
		// are the ones that occurrence was written under, which are not the recorded ones where the
		// document states another (a colliding id, an id-less call's generated one).
		//
		// The occurrence has to be one this walk wrote *and* one that stands on the written walk: this
		// writer keeps the records the read's line leaves off it as recorded history, so a paired call can
		// be in the file and still off the walk the target's loader makes from the result. Accepting the
		// pairing there would write a toolResult whose invocation the resumed session cannot see, which is
		// what the reduction's window can produce (the paired call stands on another branch).
		written, ok := b.writtenCalls[*paired]
		if !ok || !b.tree.recordOnWrittenPath(paired.message) {
			// The paired occurrence has no entry in this file, or its entry is off the walk the resumed
			// session reads: accepting the recorded id because some other written call shares it would
			// present this result as that call's outcome.
			b.lossUnpaired++
			return false
		}
		if callID == "" {
			// A derived summary the read stored on the call itself (ToolCall.ResultContent): its owner
			// is the occurrence it came from, so the result may not be inferred off the outstanding set
			// -- an inference picks no call where two are eligible, and would drop an output whose owner
			// was already known.
			resultIDInferred = true
		}
		callID, toolName = written.id, written.name
		answered = *paired
	case callID == "":
		written, occurrence, ok := b.soleResultCall()
		if !ok {
			// No call is outstanding, or several are and the result could belong to any of them: the
			// source recorded no id, so the pairing would be an invention.
			b.lossUnpaired++
			return false
		}
		callID, toolName = written.id, written.name
		resultIDInferred = true
		answered = occurrence
	case b.selectionReduced:
		// A result that names an id, on a reduced document, whose read pairing names no written
		// occurrence: omitted and counted exactly as the paired-absent case above.
		b.lossUnpaired++
		return false
	default:
		// The default path keeps the acceptance it has always asked: some written call stands under the
		// id (goal 0009's D4).
		if !b.resultCallWritten(callID) {
			b.lossUnpaired++
			return false
		}
		toolName = b.resultToolName(callID)
	}
	if !b.selectionReduced {
		// The default path keeps the ledger's own answer: the result clears its id from the file-wide
		// set, whatever its status (goal 0009's D4 -- a run without a selection keeps the answer its
		// writer had).
		delete(b.pendingCalls, callID)
	}
	if b.selectionReduced && !resultStatesAnOutcome(result.Status) {
		// The read records a call's progress as events of the call's own beside its outcome
		// (internal/parser/termination.go's rule): a Codex subagent's `running` notification, a streamed
		// chunk. This family's toolResult message carries the text and an `isError` flag and no outcome
		// status, so a written message reads back as the call's *answer* -- the family's reader takes a
		// toolResult for the call's outcome -- and the file would claim a completion the source had not
		// recorded when it wrote those words. A document a selection produced omits the event and names
		// it here (the same disposition OpenCode's plan takes, where the harness's tool states hold no
		// progress); a run without a selection keeps writing it, which is the answer it has always had
		// (goal 0009's D4; round 10's finding 5).
		b.lossNonterminal++
		return false
	}
	text, blocks := resultText(result)
	b.lossResultBlocks += blocks
	if strings.TrimSpace(text) == "" {
		if text == "" {
			b.lossEmptyResults++
		} else {
			b.lossWhitespaceRes++
		}
		return false
	}
	if b.selectionReduced {
		// The recorded result that states an outcome answers its call here, and only where it is
		// *written*: a call whose result this conversion omitted -- text that is empty or whitespace, a
		// guard above -- has no answer on the written path, so a later id-less result still sees it
		// outstanding and is omitted rather than inferred onto another call. A result whose recorded
		// status says the call is still running is progress rather than an answer
		// (resultStatesAnOutcome): it travels as the evidence it is, and it leaves no mark on the
		// outstanding set either.
		if resultStatesAnOutcome(result.Status) {
			b.answeredCalls[answered] = append(b.answeredCalls[answered], b.tree.record)
			delete(b.pendingCalls, callID)
		}
	}
	if resultIDInferred {
		b.lossGeneratedIDs++
	}
	if result.PersistedOutputUnresolved {
		b.lossUnresolved++
	}
	// The result's entry (and the message inside it) is stamped with the instant of the record the
	// result travelled on, not with the instant the read answered for the result's own record: the two
	// reads are counted apart -- the read answered an instant and the containing record's is written
	// instead, or the read answered none and the containing record's instant stands in.
	if result.Timestamp.IsZero() {
		b.lossResultFallbacks++
	} else if !result.Timestamp.Equal(at) {
		b.lossResultInstants++
	}
	if strings.TrimSpace(result.Source) != "" {
		// The read named which native record carried the result's text (a result block in a message, a
		// `function_call_output`, a wait call's output event, a subagent notification). The target's entry
		// carries the text and no such label -- its own read answers its own for its own shapes -- so the
		// source's label is omitted and counted rather than dropped in silence.
		b.lossResultSources++
	}
	message := &piMessage{
		Role:       piRoleToolResult,
		Content:    []piContentBlock{{Type: piBlockText, Text: text}},
		ToolCallID: callID,
		ToolName:   toolName,
		Timestamp:  piMessageMillis(at),
	}
	if strings.TrimSpace(result.Status) != "" {
		switch status := strings.ToLower(strings.TrimSpace(result.Status)); status {
		case "errored", "error", "failed", "failure":
			isError := true
			message.IsError = &isError
		default:
			// Pi's field is a boolean; any other status word has no field and is named rather than
			// translated into a meaning it may not have.
			b.lossStatuses++
		}
	}
	b.tree.markToolResult(carrierRecord)
	b.write(piEntry{
		Type:      piEntryMessage,
		Timestamp: b.clock.next(at),
		message:   message,
	})
	b.carried.ToolResults++
	item := TargetItem{Kind: ItemToolResult, CallID: callID, Text: text}
	if b.selectionReduced {
		// A document a selection produced knows the owning occurrence (the pairing, or the disclosed
		// inference that picked one), so the result item states the place of that occurrence's call item
		// and the round-trip check asserts the attachment the reader makes instead of trusting that an
		// id two occurrences could share identifies the right one.
		if written, ok := b.writtenCalls[answered]; ok {
			owner := written.item
			item.OwnerCall = &owner
		}
	}
	b.items = append(b.items, item)
	return true
}

// piMessageMillis is one message's own timestamp: Pi writes Unix milliseconds inside `message`.
func piMessageMillis(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UTC().UnixMilli()
}

// piFamilyUnestablishedOutcome is the Pi family's own answer for a record whose dialogue was not
// established (dialogueUnestablished), passed to dialogueLossRows: what the writer does not write and
// where the record's reasoning goes. This family's format persists a trace as a `thinking` content block
// of the assistant message that produced it, so a trace travels where the record's role is the
// assistant's -- as the block it was recorded in -- and has no shape on a record of another role, where
// it is counted as omitted (non_assistant_reasoning). The record's text does not travel either way (this
// is the clause the two wrong copies generalised from; round 17's PD-3).
const piFamilyUnestablishedOutcome = "the text is not written, this count is not a claim that the " +
	"record wrote none, and the record's own reasoning still travels as the `thinking` block it was " +
	"recorded in where the record's role is the assistant's -- the role this format writes the block " +
	"on alone -- while a trace on a record of another role has no shape here and is counted as omitted " +
	"(non_assistant_reasoning)"

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *piSessionBuilder) losses() []Loss {
	toolDetail := toolVocabularyDetail(b.toolNameOrder, b.opts.MaxToolNames, "argument objects")
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = preBoundaryHistoryDetail()
	}
	// The explanation is selected by the count the entry carries (modelAttributionCount): a family whose
	// format requires a non-empty model on every assistant record can have supplied the target home's own
	// model for *every* record, and lossModels is then zero while the file still states model ids this
	// conversion chose. A detail selected by the travelled count alone would call those ids unexplained.
	modelDetail := "the source recorded no model on any message"
	if b.modelAttributionCount() > 0 {
		modelDetail = piModelDetail(b.shape.Name)
	}
	losses := append([]Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossReasoning,
			Detail: "Pi persists a reasoning trace as a `thinking` content block of the assistant " +
				"message that produced it, and that block is the shape the source's own trace is " +
				"written into: the trace travels as the source recorded it, with no signature forged " +
				"for it. A trace recorded on a record of another role has no such block; that " +
				"omission is counted separately (non_assistant_reasoning). A resumed turn reads the " +
				"trace it reads as history rather than replaying it",
		},
		{
			Category:    "non_assistant_reasoning",
			Disposition: DispositionOmitted,
			Count:       b.lossNonAssistantReasoning,
			Detail: "Pi writes a reasoning trace as a `thinking` block of the assistant message that " +
				"produced it and of no other role, so a trace recorded on a record whose role is not " +
				"the assistant's -- a user prompt carrying one, or a record of a role this format " +
				"has no message for -- has no shape here and is omitted. The record's own text is " +
				"governed by its own category; this count names the trace alone",
		},
		{
			Category:    "sidechain_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossSidechain,
			Detail: "a sidechain message is a subagent's turn recorded in the same session file; Pi's own " +
				"loader keeps such records out of the session's context, so a written entry would be " +
				"evidence in the file that no read of it shows. A conversion's unit is the selected " +
				"session",
		},
		{
			Category:    "system_records",
			Disposition: DispositionOmitted,
			Count:       b.lossSystemRecords,
			Detail: "Pi's system entries carry its own prompt and tool loadout; a provider's bookkeeping " +
				"text does not belong in them. Only the record's own text is omitted; a tool call or " +
				"result recorded on the same record is written as the tool evidence it is",
		},
		{
			Category:    "tool_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossToolRoles,
			Detail: "a message the source recorded with the tool role is the tool's own output, and Pi's " +
				"toolResult entry is where a tool's words belong: the text is carried there, in a result " +
				"paired with the call it belongs to",
		},
		{
			Category:    "synthesized_tool_call_messages",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCarriedCallEntries,
			Detail: "these entries are this conversion's: the source recorded the calls on a record " +
				"this conversion writes no message entry of its own for -- a provider's system " +
				"record, a role the format has no message for, a user record whose calls its text " +
				"entry cannot hold, or an assistant record whose dialogue is whitespace alone -- and " +
				"Pi records a call as a block of the assistant message that made it, so the calls are " +
				"written in an entry of their own. The entry carries the recorded call blocks and " +
				"their results and no dialogue",
		},
		{
			Category:    "unclassified_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossUnclassified,
			Detail: "a record whose role this build does not classify has no entry in the target to go " +
				"to; writing it as a user or assistant message would state a role the source never recorded",
		},
		{
			Category:    "empty_user_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyUser,
			Detail: "a user record with neither dialogue text nor tool evidence carries nothing the " +
				"target can show, so no entry is written for it",
		},
		{
			Category:    "empty_assistant_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyAssistant,
			Detail: "an assistant record with neither dialogue, a reasoning trace nor tool evidence " +
				"carries nothing the target can show, so no entry is written for it (the inherited " +
				"reader produces that shape for a usage-only row)",
		},
		{
			Category:    "non_dialogue_content",
			Disposition: DispositionOmitted,
			Count:       b.lossNonDialogue,
			Detail: "the source itself classified these records as carrying no dialogue, and their " +
				"content is a rendering this writer must not put in the user's mouth",
		},
	}, dialogueLossRows(b.lossUnresolvedDialogue, b.lossUnprojectedDialogue,
		piFamilyUnestablishedOutcome)...)
	losses = append(append(losses, []Loss{
		{
			Category:    "whitespace_only_dialogue",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespace,
			Detail: "the source recorded dialogue text that holds nothing but whitespace; the inherited " +
				"Pi reader drops a record left with no text and no tool evidence, so a written entry " +
				"would be in the file but absent from every read of it",
		},
		{
			Category:    "unpaired_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossUnpaired,
			Detail: "a tool result whose call this conversion did not carry has no pair in the target, " +
				"and fabricating the call would state a tool invocation the source never recorded",
		},
		{
			Category:    "empty_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyResults,
			Detail: "Pi's tool result carries content blocks; a result this read holds no text for is not " +
				"written, because an empty block would read as an empty tool output",
		},
		{
			Category:    "whitespace_only_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespaceRes,
			Detail: "the source recorded output text that holds nothing but whitespace; the inherited " +
				"reader's block parser reads that as no text, so a written block would be in the file " +
				"but absent from every read of it",
		},
		{
			Category:    "unrepresentable_result_blocks",
			Disposition: DispositionOmitted,
			Count:       b.lossResultBlocks,
			Detail: "the reader's text channel carries a block's own text and nothing else: a block the " +
				"source recorded with other content (an image payload, a block type this build does not " +
				"read) is not written. The count is of blocks, not of results",
		},
		{
			Category:    "unresolved_tool_outputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossUnresolved,
			Detail: "the source's own parse said these results are the placeholder naming a side file it " +
				"could not read, not the tool's output; that text is what was carried, and only results " +
				"that were written are counted here",
		},
		{
			Category:    "unrecorded_token_usage",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossUnrecordedUsage,
			Detail: "Pi's own loader reads a usage object on every assistant message it loads and " +
				"stops with `undefined is not an object (evaluating 'message.usage.input')` on one " +
				"without it (measured), so each converted assistant entry carries a usage object whose " +
				"counts are zero. A zero count is not a claim that the words cost nothing: the source's " +
				"provider billed the source's session, whose totals are recorded there and nowhere in " +
				"this document, and this build declines to attribute them, or a computed guess, to a " +
				"session that did not spend them. What it costs is stated with the count: the target " +
				"builds its own estimate of the inherited context from these numbers, so a long carried " +
				"history reads to Pi as a cheaper context than it is until the first resumed turn " +
				"reports its own usage",
		},
		{
			Category:    "tool_result_statuses",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossStatuses,
			Detail: "Pi's tool result carries a boolean error flag; a result whose recorded status says " +
				"the call failed is written with it, and any other status is not written at all -- the " +
				"output text travels as recorded. Only results that were written are counted",
		},
		{
			Category:    "empty_tool_inputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossEmptyInputs,
			Detail: "the source recorded no argument object for these calls (or recorded one that is not " +
				"a JSON object); Pi's toolCall block carries an arguments object, so an empty object was " +
				"written",
		},
		{
			Category:    "generated_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossGeneratedIDs,
			Detail: "the source recorded no tool-use id for these calls or results; Pi pairs a call and " +
				"its result by id, so one was generated from the message's own position",
		},
		{
			Category:    "coalesced_tool_call_projections",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCoalesced,
			Detail: "the source read recorded these calls once for every file they touched (the inherited " +
				"Codex reader projects one apply_patch into one call per file, every projection sharing " +
				"the call's id and input); Pi's block carries one id and one input, so the call is " +
				"written once and what has no field is the per-file attribution of a second projection",
		},
		{
			Category:    "subagent_session_links",
			Disposition: DispositionOmitted,
			Count:       b.lossSubagentLinks,
			Detail: "the source recorded which session these calls spawned; Pi's toolCall block has no " +
				"field for a child session, and the child session is neither read nor converted -- a " +
				"conversion's unit is the selected session",
		},
		{
			Category:    "tool_result_sources",
			Disposition: DispositionOmitted,
			Count:       b.lossResultSources,
			Detail: "the read names which native record carried each result's text (a result block " +
				"inside a message, a `function_call_output`, a wait call's own output event, a " +
				"subagent notification: internal/parser's readers answer this per shape). The " +
				"target's toolResult entry carries the text and no such label -- its own read " +
				"answers the analogous question for its own shapes -- so the source's label is " +
				"omitted, counted per result this conversion wrote that recorded one",
		},
		{
			Category:    "result_completion_fallbacks",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossResultFallbacks,
			Detail: "a toolResult message entry carries the result's text, and its own timestamp is " +
				"stamped with the instant of the record the result travelled on. Where the read " +
				"holds no instant of the result's own record -- a provider-recorded event with no " +
				"timestamp, or the archive's derived summary, which is stored on the call's own row " +
				"and has no record of its own -- that containing record's instant is written, an " +
				"instant the read answered for that record rather than one this build invented; " +
				"only results this conversion wrote are counted here",
		},
		{
			Category:    "result_record_instants",
			Disposition: DispositionOmitted,
			Count:       b.lossResultInstants,
			Detail: "the source recorded the instant the result itself was recorded on (a result " +
				"event's own envelope instant, which may be later than the record that carried its " +
				"call, and which the archive read answers from the stored event's own column). A " +
				"toolResult entry carries its own timestamp and this conversion stamps it with the " +
				"containing record's instant, not with the result's own: the result's own recorded " +
				"instant is not what the file holds. Counted per result this conversion wrote and " +
				"only where the read answered one",
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
			Count:       len(b.toolNameOrder),
			Detail:      toolDetail,
		},
		{
			Category:    "message_model_attribution",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.modelAttributionCount(),
			Detail:      modelDetail,
		},
		{
			Category:    "unresolved_source_parents",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.tree.unresolved,
			Detail:      unresolvedSourceParentsDetail(),
		},
		{
			Category:    "pre_compaction_history",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossPreBoundary,
			Detail:      preBoundaryDetail,
		},
		{
			Category:    "empty_compaction_summary",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptySummary,
			Detail: "the source recorded a compaction boundary whose summary holds no text. The " +
				"compaction entry is still written, because that entry is what makes the target rebuild " +
				"its model-visible history at the boundary; its summary holds the empty text rather than " +
				"an invented summary",
		},
		{
			Category:    "malformed_source_lines",
			Disposition: DispositionOmitted,
			Count:       b.sourceFidelity.MalformedLines,
			Detail:      malformedLinesLossDetail(b.sourceFidelity),
		},
		{
			Category:    "summary_transcript",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(b.sourceFidelity.TranscriptFidelity == parser.TranscriptFidelitySummary),
			Detail:      summaryTranscriptLossDetail(b.sourceFidelity),
		},
		{
			Category:    "source_needs_retry",
			Disposition: DispositionCarriedWithCaveat,
			Count:       flagCount(b.sourceFidelity.NeedsRetry),
			Detail:      needsRetryLossDetail(b.sourceFidelity),
		},
	}...), b.familyLosses()...)
	// The colliding-id category exists only where the reduction can act. The writer's own completeness
	// rule -- every category on every run -- would put a zero row in every report, and a run without a
	// selection is byte-identical to the same run before selections existed (goal 0009's D4), whole report
	// included: the row is therefore carried by a document a selection produced, whose reduction is the
	// only thing that can write one occurrence beside another under the same recorded id, and an unflagged
	// report keeps its own category list unchanged. The count is zero on a reduced document with no
	// collision, so within the selection surface the completeness rule holds as the other rows' does.
	if b.selectionReduced {
		losses = append(losses, Loss{
			Category:    "colliding_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCollidingIDs,
			Detail: "these call occurrences recorded an id an earlier written occurrence already " +
				"holds; this target pairs a call with its output by id (Pi's reader attaches a " +
				"toolResult to the call whose block registered the id), so the occurrence was written " +
				"under an id generated from its own place and the file does not state the id the " +
				"source recorded for it",
		})
		// The progress category is reduction-scoped as well: only a document a selection produced omits
		// a recorded progress event, because only there does the writer act on the read's own rule -- a
		// call still running is not answered -- and the default path keeps writing the message, which is
		// the answer it has always had (goal 0009's D4), so its report keeps its own category list
		// unchanged. The count is zero on a reduced document with no progress event, so within the
		// selection surface the completeness rule holds as the other rows' does.
		losses = append(losses, Loss{
			Category:    "nonterminal_tool_events",
			Disposition: DispositionOmitted,
			Count:       b.lossNonterminal,
			Detail: "the read records a call's progress as events of its own beside the call's outcome " +
				"(the shape the inherited termination rule reads: a Codex subagent's `running` " +
				"notification streams in while its call is still executing, and the RooCode/Kilo chunk " +
				"readers record streamed chunks the same way). The family's toolResult message carries " +
				"the text and an isError flag and no outcome status, so a written message reads back as " +
				"the call's *answer* -- the family's reader takes a toolResult for the call's outcome -- " +
				"and the file would claim a completion the source had not recorded when it wrote those " +
				"words",
		})
	}
	return losses
}

// modelAttributionCount is what the message_model_attribution caveat counts: the assistant records whose
// model the source's own message recorded (carried verbatim) plus, on a family whose format requires a
// non-empty model on every assistant record, the records the read answered no model for, which carry
// the harness's own configured one.
func (b *piSessionBuilder) modelAttributionCount() int {
	return b.lossModels + b.lossModelFallbacks
}

// familyLosses answers the loss categories one family's format has and the other's does not, in a fixed
// order and with every count, so a caller of either route reads a complete list (zero counts included).
// Pi's list is empty: every category its writer knows is in the shared list above, and a family that
// added nothing must not grow a second block of zeroes.
func (b *piSessionBuilder) familyLosses() []Loss {
	if b.shape.Name != piFamilyOMP {
		return nil
	}
	// The detail is the route's own sentence, because it is where the harness's answer -- or the reason
	// there is none -- is known: a conversion that asked a harness and was answered says which model the
	// session records and why; one that asked nothing (a dry run, a home with no model) says that too, and
	// what the missing record costs. A family that writes no binding at all is not this case and never
	// reaches here (familyLosses answers nothing for it).
	bindingDetail := b.shape.sessionBindingDetail
	return []Loss{
		{
			Category:    "target_model_binding",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossSessionBinding,
			Detail:      bindingDetail,
		},
		{
			Category:    "title_slot_written",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossTitleSlot,
			Detail: "the target's own session format opens with a fixed-width title slot the harness " +
				"rewrites in place when it titles a session. It is written with an empty title -- OMP's " +
				"own initial state, and a title is the harness's own to generate -- and it is the one " +
				"line of the file that is not an entry of the conversation's tree. Pi's files have no " +
				"such line, which is why it is named here rather than assumed",
		},
	}
}

// piFormatLimits is what this pair of formats cannot express at all, as opposed to what one run
// happened to leave out: a limit carries no count, because it holds for every conversion.
//
// The family's shared half is the family's, and the sentences that name the harness whose file is
// written take the shape's own name (shape.sessionPhrase()): a Pi session's header, its missing turn
// record, its lack of attachment bytes and its tool vocabulary are the same claims for an OMP session.
// The provider binding is **Pi's own limit** and is not carried into OMP's (the condition below): a Pi
// session binds no model provider to itself, while an OMP session records one its loader restores and
// refuses -- that is the `session_model_binding` limit OMP's own list states instead (ompFormatLimits),
// so carrying this one into an OMP report would state the opposite of what its format does.
func piFormatLimits(shape piFamilyShape) []FormatLimit {
	limits := []FormatLimit{
		{
			Category: "session_metadata_record",
			Detail: shape.sessionPhrase() + " header carries the session's identity, its " +
				"start time and its working " +
				"directory and nothing else, so a source's own session-level fields (a Codex " +
				"session_meta's originator, cli version and source kind; a Claude transcript's version " +
				"field) have no field to travel in. The conversation's own messages are unaffected",
		},
	}
	if shape.Name != piFamilyOMP {
		limits = append(limits, FormatLimit{
			Category: "model_provider_binding",
			Detail: "a Pi session binds no model provider to itself: a resumed turn uses whatever model " +
				"and provider the caller's Pi configuration selects, whatever the records name. A " +
				"converted session records no provider (the source read answers none -- see " +
				"message_model_attribution) and therefore cannot be refused for recording the wrong one",
		})
	}
	return append(limits,
		FormatLimit{
			Category: "turn_boundaries",
			Detail: "Codex records a turn_context record at each turn boundary; " + shape.sessionPhrase() +
				" has no turn " +
				"record, so a converted session records no turn boundaries in the target's vocabulary " +
				"(the target groups its own turns when it resumes)",
		},
		FormatLimit{
			Category: "attachments_and_images",
			Detail: "the source read carries no attachment or image bytes, so this conversion has none " +
				"to write and reports no count for them: a per-run \"0 omitted\" would be a claim the " +
				"read cannot support",
		},
		FormatLimit{
			Category: "tool_vocabulary",
			Detail: "tool names travel verbatim as recorded evidence; the target's own tool set is " +
				"unaffected by them, and a resumed turn reads them as history rather than replaying them",
		},
	)
}

// sessionPhrase is this harness's session as a format limit's sentence names it, article included:
// "a Pi session" or "an Oh My Pi session". It is not Name, which is the report's own route word
// ("pi"/"omp") and reads as a scheme rather than a harness in prose like "a ... session header".
func (s piFamilyShape) sessionPhrase() string {
	if s.Name == piFamilyOMP {
		return "an Oh My Pi session"
	}
	return "a Pi session"
}
