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

// ConvertToPiSession turns a Conversation into a Pi session: a fresh identity, one entry per piece of
// evidence the target can express, and the report of what it cannot. It writes no file; installation
// is the caller's step (InstallPiSession).
//
// The source session's recorded working directory is required: Pi resolves a session id through the
// project directory of the process's own working directory, so a session installed without one could
// never be resumed while a report called it installed. A document that records none is refused here,
// before anything is written.
func ConvertToPiSession(conv Conversation, opts PiTargetOptions) (PiSession, error) {
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
		cliVersion = PiFormatVersion
	}
	startedAt := firstRecordedTime(conv.Messages)
	if startedAt.IsZero() {
		startedAt = opts.Now.UTC()
	}
	if startedAt.IsZero() {
		startedAt = time.Unix(0, 0).UTC()
	}
	builder := &piSessionBuilder{
		sessionID:       sessionID,
		cwd:             conv.Source.Cwd,
		cliVersion:      cliVersion,
		opts:            opts,
		clock:           newEntryClock(startedAt, piEntryTimestampLayout),
		headerTimestamp: startedAt.UTC().Format(piEntryTimestampLayout),
		callNames:       map[string]string{},
		ledger:          ledger{pendingCalls: map[string]struct{}{}},
		sourceFidelity:  conv.Fidelity,
		tree:            newSourceTree(),
	}
	builder.writeConversation(conv)
	if builder.idErr != nil {
		return PiSession{}, builder.idErr
	}
	return PiSession{
		SessionID:  sessionID,
		FileName:   piSessionFileName(sessionID, startedAt),
		Directory:  PiProjectDir(conv.Source.Cwd),
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
	opts       PiTargetOptions
	carried    Carried
	items      []TargetItem
	idErr      error
	callNames  map[string]string
	ledger
	lastModel      string
	sourceFidelity Fidelity

	// tree maps the source's own record relations onto this session's entries, so the file is the
	// source's own shape rather than a flat chain: each entry hangs off the entry of the record the
	// source named as its parent, and a branch the conversation left stays off the walk the target's own
	// loader makes. Its unresolved count is what the report's unresolved_source_parents names.
	tree sourceTree

	writtenMessages        int
	lossReasoning          int
	lossSidechain          int
	lossSystemRecords      int
	lossToolRoles          int
	lossUnclassified       int
	lossEmptyUser          int
	lossEmptyAssistant     int
	lossNonDialogue        int
	lossWhitespace         int
	lossUnpaired           int
	lossEmptyResults       int
	lossWhitespaceRes      int
	lossResultBlocks       int
	lossUnresolved         int
	lossStatuses           int
	lossEmptyInputs        int
	lossGeneratedIDs       int
	lossCoalesced          int
	lossSubagentLinks      int
	lossModels             int
	lossCarriedCallEntries int
	lossUnrecordedUsage    int
	lossPreBoundary        int
	lossEmptySummary       int
	sawBoundary            bool
	// headerTimestamp is the session's own start instant, fixed when the builder is made: the header's
	// timestamp is the session's identity rather than a position in the entry stream, so it must not be
	// advanced by the entries written after it.
	headerTimestamp string
}

// bytes answers the session's own bytes: the header, then every entry, each terminated by a line feed.
// It also records the size it produced as the conversion's carried byte count, so the reported count is
// the file's own size rather than a second length computed from the same buffer.
func (b *piSessionBuilder) bytes() []byte {
	var session bytes.Buffer
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
		b.writeCarriedCalls(message)
		return
	}
	text := dialogueText(message)
	calls := b.distinctCalls(message)
	hasThinking := strings.TrimSpace(message.Thinking) != ""
	if hasThinking {
		// Pi's `thinking` block is the format's own shape for a persisted reasoning trace and carries no
		// signature this build would have to forge, so the source's trace travels as the source
		// recorded it. The Claude route must omit it; this one must not pretend it did.
		b.lossReasoning++
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
		b.lossWhitespace++
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
			Model:      b.modelField(message),
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
// The provider is deliberately *not* written. Pi's `provider` field means the endpoint that served the
// words, and the inherited read does not answer that: it reports the model each source message recorded
// (Message.Model) but not the endpoint, and its session-level provider names the *harness* that wrote
// the words (`pi`, `claude`, `codex`) -- a label the target's own configuration need not define as a
// provider. Writing that label into the record was measured wrong (0005's review): the committed Pi
// fixture records `example-proxy`, and the conversion wrote `pi` while the report claimed the source's
// own provider travelled verbatim. Nothing is written in its place, and the report says so
// (message_model_attribution).
func (b *piSessionBuilder) modelField(message Message) string {
	model := strings.TrimSpace(message.Model)
	if model == "" {
		return ""
	}
	b.lastModel = model
	b.lossModels++
	return model
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

// writeCarriedCalls writes the calls one message carries when that message's own role has no entry in
// the target to hold them: a record the provider flagged as system-injected, or one written with a role
// this build does not classify, whose text is omitted but whose calls and results are real recorded
// evidence. Pi records a call as a block of the assistant message that made it, so the calls are written
// in an entry of their own; the entry holds no dialogue, because the source recorded the words on a
// record whose role this target has no message for, and the count says the entry is this conversion's.
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
	for _, result := range message.ToolResults {
		b.writeToolResult(result, message.Timestamp)
	}
	for _, entry := range calls {
		call := entry.call
		if strings.TrimSpace(call.ResultContent) == "" && call.ResultContentSize == 0 {
			continue
		}
		if callHasRecordedResult(message.ToolResults, call.ToolUseID) {
			continue
		}
		b.writeToolResult(ToolResult{
			ToolUseID:      call.ToolUseID,
			ContentRaw:     call.ResultContent,
			ContentSize:    call.ResultContentSize,
			ContentDecoded: true,
		}, message.Timestamp)
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
	if callID == "" {
		// The call's place in the document keys its generated id -- the message's position in the walk
		// and the call's index in that message -- and not the message's own ordinal: an embedding
		// caller may leave ordinals unassigned, and two messages' id-less calls must not collapse into
		// one id, which would make the pairing below read one call where two exist.
		callID = fmt.Sprintf("%s%d_%d", piGeneratedCallBase, b.writtenMessages, index)
		b.lossGeneratedIDs++
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
	b.pendingCalls[callID] = struct{}{}
	b.carried.ToolCalls++
	b.items = append(b.items, TargetItem{Kind: ItemToolCall, CallID: callID, ToolName: name, InputJSON: arguments})
	return piContentBlock{Type: piBlockToolCall, ID: callID, Name: name, Arguments: json.RawMessage(arguments)}
}

// writeToolResult writes one result as a toolResult entry, and answers whether it was written.
func (b *piSessionBuilder) writeToolResult(result ToolResult, at time.Time) bool {
	callID := strings.TrimSpace(result.ToolUseID)
	resultIDInferred := false
	if callID == "" {
		pending, ok := b.solePendingCall()
		if !ok {
			b.lossUnpaired++
			return false
		}
		callID = pending
		resultIDInferred = true
	} else if _, ok := b.callNames[callID]; !ok {
		// The result names a call this conversion did not write: a result without its call is not a
		// pair, and inventing the call would fabricate a tool invocation.
		b.lossUnpaired++
		return false
	}
	delete(b.pendingCalls, callID)
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
	if resultIDInferred {
		b.lossGeneratedIDs++
	}
	if result.PersistedOutputUnresolved {
		b.lossUnresolved++
	}
	message := &piMessage{
		Role:       piRoleToolResult,
		Content:    []piContentBlock{{Type: piBlockText, Text: text}},
		ToolCallID: callID,
		ToolName:   b.callNames[callID],
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
	b.tree.markToolResult()
	b.write(piEntry{
		Type:      piEntryMessage,
		Timestamp: b.clock.next(at),
		message:   message,
	})
	b.carried.ToolResults++
	b.items = append(b.items, TargetItem{Kind: ItemToolResult, CallID: callID, Text: text})
	return true
}

// piMessageMillis is one message's own timestamp: Pi writes Unix milliseconds inside `message`.
func piMessageMillis(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UTC().UnixMilli()
}

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *piSessionBuilder) losses() []Loss {
	toolDetail := toolVocabularyDetail(b.toolNameOrder, b.opts.MaxToolNames, "argument objects")
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = preBoundaryHistoryDetail()
	}
	modelDetail := "the source recorded no model on any message"
	if b.lossModels > 0 {
		modelDetail = "Pi's message record states which model and which provider produced the words. The " +
			"model travels verbatim -- it is what the source's own message recorded, and the target's own " +
			"totals then count the turn under the model that wrote it. The provider does not: the " +
			"inherited read answers no endpoint attribution for a message (the Pi reader projects a " +
			"message's model and not its provider), and the read's session-level provider names the " +
			"*harness* that wrote the words -- `claude`, `codex`, `pi` -- which is not an endpoint the " +
			"target's own configuration defines, so no provider is written rather than that label. A Pi " +
			"session binds no provider to itself, so nothing is refused when the resumed session runs " +
			"under another model"
	}
	return []Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossReasoning,
			Detail: "Pi persists a reasoning trace as a `thinking` content block of the assistant " +
				"message that produced it, and that block is the shape the source's own trace is " +
				"written into: the trace travels as the source recorded it, with no signature forged " +
				"for it. A resumed turn reads it as history rather than replaying it",
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
				"whose role (a provider's system record, a native tool row) has no Pi message to hold " +
				"them, and Pi records a call as a block of the assistant message that made it. The entry " +
				"carries the recorded call blocks and their results and no dialogue, because the " +
				"source's own words were recorded on a record whose text this conversion omits",
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
			Category:    "tool_name_vocabulary",
			Disposition: DispositionCarriedWithCaveat,
			Count:       len(b.toolNameOrder),
			Detail:      toolDetail,
		},
		{
			Category:    "message_model_attribution",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossModels,
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
	}
}

// piFormatLimits is what this pair of formats cannot express at all, as opposed to what one run
// happened to leave out: a limit carries no count, because it holds for every conversion.
func piFormatLimits() []FormatLimit {
	return []FormatLimit{
		{
			Category: "session_metadata_record",
			Detail: "a Pi session header carries the session's identity, its start time and its working " +
				"directory and nothing else, so a source's own session-level fields (a Codex " +
				"session_meta's originator, cli version and source kind; a Claude transcript's version " +
				"field) have no field to travel in. The conversation's own messages are unaffected",
		},
		{
			Category: "model_provider_binding",
			Detail: "a Pi session binds no model provider to itself: a resumed turn uses whatever model " +
				"and provider the caller's Pi configuration selects, whatever the records name. A " +
				"converted session records no provider (the source read answers none -- see " +
				"message_model_attribution) and therefore cannot be refused for recording the wrong one",
		},
		{
			Category: "turn_boundaries",
			Detail: "Codex records a turn_context record at each turn boundary; a Pi session has no turn " +
				"record, so a converted session records no turn boundaries in the target's vocabulary " +
				"(the target groups its own turns when it resumes)",
		},
		{
			Category: "attachments_and_images",
			Detail: "the source read carries no attachment or image bytes, so this conversion has none " +
				"to write and reports no count for them: a per-run \"0 omitted\" would be a claim the " +
				"read cannot support",
		},
		{
			Category: "tool_vocabulary",
			Detail: "tool names travel verbatim as recorded evidence; the target's own tool set is " +
				"unaffected by them, and a resumed turn reads them as history rather than replaying them",
		},
	}
}
