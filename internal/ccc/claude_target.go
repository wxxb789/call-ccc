package ccc

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// ClaudeFormatVersion is the Claude Code version this writer's transcript format was verified
// against, when the installed target binary cannot be probed for its own version. Every record carries
// the version the file was written as, and a caller that cannot ask the target which version it is
// reports this pin instead (target_session.cli_version_source = "writer-pin").
const ClaudeFormatVersion = "2.1.326"

// ClaudeTargetFormat names the on-disk format a Claude Code target session is written in: the
// transcript JSONL the harness keeps under <config>/projects/<project-dir>/<session-id>.jsonl, which
// is the format its own loader resumes and the inherited AgentsView Claude reader parses.
const ClaudeTargetFormat = "claude-code-transcript-jsonl"

// ErrNoTargetCwd reports a source session whose read recorded no working directory. The Claude Code
// loader finds a session through the working directory it is run from -- measured on the tested
// version: the same session id answers "No conversation found" from any directory whose project-dir
// spelling differs -- so a session installed without one could never be resumed while a report called
// it installed. The conversion is refused before anything is written rather than guessed at.
var ErrNoTargetCwd = errors.New("the source session records no working directory")

// ClaudeTargetOptions configures one conversion. SessionID and CLIVersion are normally left empty: the
// identity must be fresh and the version is probed or pinned by the caller.
type ClaudeTargetOptions struct {
	// SessionID is the target session's identity: the file name and every record's sessionId. Empty
	// means "generate a fresh UUID v4", which is what a conversion wants and what the target's own
	// sessions are named; a caller that passes one (a test, a repeated run) is responsible for it not
	// naming a session that already exists.
	SessionID string
	// CLIVersion is the value written to every record's "version" field.
	CLIVersion string
	// Now is the conversion instant, used for identities and for a source that recorded no times.
	Now time.Time
	// MaxToolNames bounds how many distinct tool names the report spells out.
	MaxToolNames int
}

// claudeRecord is one transcript line. Field order is this struct's order, so two conversions of the
// same document produce the same bytes.
//
// The envelope the target's loader needs is small -- measured on Claude Code 2.1.326 by removing one
// field at a time: uuid, type, timestamp, cwd, sessionId and message.content are enough to load and
// resume a session -- so the writer writes those and the couple of constants the harness's own records
// carry (isSidechain, userType), and nothing it would have to invent.
type claudeRecord struct {
	ParentUUID  *string        `json:"parentUuid"`
	IsSidechain bool           `json:"isSidechain"`
	Type        string         `json:"type"`
	UUID        string         `json:"uuid"`
	Timestamp   string         `json:"timestamp"`
	Cwd         string         `json:"cwd,omitempty"`
	SessionID   string         `json:"sessionId"`
	Version     string         `json:"version"`
	UserType    string         `json:"userType,omitempty"`
	Message     *claudeMessage `json:"message,omitempty"`
	// Subtype marks the compaction-boundary record the loader truncates the model-visible history at
	// (measured: {"type":"system","subtype":"compact_boundary"} together with the isCompactSummary
	// record below is what makes the target apply the boundary; the flag alone changed nothing).
	Subtype string `json:"subtype,omitempty"`
	// IsCompactSummary marks the summary record that stands in place of the summarized history, which
	// is also the record the inherited reader classifies as the boundary and reads the summary text
	// from.
	IsCompactSummary bool `json:"isCompactSummary,omitempty"`
}

// claudeMessage is the record's message payload. Model is deliberately absent: the target's field
// states which model produced the words, and neither the source's model nor a target model ran this
// conversation, so writing one would attribute it to a model that did not answer it.
type claudeMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type,omitempty"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Claude transcript roles and block kinds, as the target writes them.
const (
	claudeTypeUser      = "user"
	claudeTypeAssistant = "assistant"
	claudeTypeSystem    = "system"

	claudeBlockText       = "text"
	claudeBlockToolUse    = "tool_use"
	claudeBlockToolResult = "tool_result"

	claudeSubtypeCompactBoundary = "compact_boundary"

	claudeUserTypeExternal = "external"

	claudeGeneratedCallPrefix = "toolu_ccc_"
)

// claudeContentBlock is one content block. Text, ToolUseID/Name/Input and the error flag are written
// one shape per kind, so a reader never sees a block claiming to be two kinds at once.
type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// ID, Name and Input are a tool_use block's own members. Input is the source's recorded input
	// object verbatim: a writer that synthesized one would state a call the source never recorded.
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// ToolUseID and the members below belong to a tool_result block. IsError is written only where the
	// source's own status says the call failed; the target's field is a boolean and the source's status
	// is a word, so a status with no error meaning is reported rather than translated into one.
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// ClaudeTranscript is one converted target session, in memory and ready to install. Data is the exact
// byte string the target harness reads; Items is what the writer placed in it, for the round-trip
// check, and is never serialized into a CLI document.
type ClaudeTranscript struct {
	SessionID  string       `json:"session_id"`
	CLIVersion string       `json:"cli_version"`
	FileName   string       `json:"file_name"`
	Project    string       `json:"project"`
	Directory  string       `json:"directory"`
	Cwd        string       `json:"cwd,omitempty"`
	StartedAt  time.Time    `json:"started_at,omitempty"`
	Data       []byte       `json:"-"`
	Items      []TargetItem `json:"-"`
	Carried    Carried      `json:"carried"`
	Losses     []Loss       `json:"losses"`
}

// ConvertToClaudeTranscript turns a Conversation into a Claude Code transcript: a fresh identity, one
// record per piece of evidence the target can express, and the report of what it cannot. It writes no
// file; installation is the caller's step (InstallClaudeTranscript).
//
// The source session's recorded working directory is required: the target's loader finds a session
// through the working directory it is run from, so a session installed without one could never be
// resumed while a report called it installed. A document that records none is refused here, before
// anything is written.
func ConvertToClaudeTranscript(conv Conversation, opts ClaudeTargetOptions) (ClaudeTranscript, error) {
	cwd := strings.TrimSpace(conv.Source.Cwd)
	if cwd == "" {
		return ClaudeTranscript{}, ErrNoTargetCwd
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	if opts.MaxToolNames <= 0 {
		opts.MaxToolNames = defaultMaxToolNames
	}
	sessionID := strings.TrimSpace(opts.SessionID)
	if sessionID == "" {
		generated, err := newUUIDV4()
		if err != nil {
			return ClaudeTranscript{}, fmt.Errorf("generating a target session id: %w", err)
		}
		sessionID = generated
	}
	cliVersion := strings.TrimSpace(opts.CLIVersion)
	if cliVersion == "" {
		cliVersion = ClaudeFormatVersion
	}

	startedAt := conv.Source.FileModified.UTC()
	if startedAt.IsZero() {
		startedAt = now
	}
	if first := firstRecordedTime(conv.Messages); !first.IsZero() && first.Before(startedAt) {
		startedAt = first
	}

	builder := &claudeTranscriptBuilder{
		sessionID:      sessionID,
		cwd:            cwd,
		cliVersion:     cliVersion,
		clock:          newCodexClock(startedAt),
		callOwners:     map[string]struct{}{},
		opts:           opts,
		sourceFidelity: conv.Fidelity,
	}
	builder.encoder = json.NewEncoder(&builder.buffer)
	builder.encoder.SetEscapeHTML(false)
	builder.writeConversation(conv)
	if builder.uuidErr != nil {
		return ClaudeTranscript{}, builder.uuidErr
	}
	carried := builder.carried
	carried.Bytes = builder.buffer.Len()
	project := ClaudeProjectDir(cwd)
	return ClaudeTranscript{
		SessionID:  sessionID,
		CLIVersion: cliVersion,
		FileName:   sessionID + ".jsonl",
		Project:    project,
		Directory:  project,
		Cwd:        cwd,
		StartedAt:  startedAt,
		Data:       bytes.Clone(builder.buffer.Bytes()),
		Items:      builder.items,
		Carried:    carried,
		Losses:     builder.losses(),
	}, nil
}

// ClaudeProjectDir spells a working directory the way the target's session discovery does: every
// character outside [A-Za-z0-9] becomes "-", and nothing else changes. Measured on Claude Code
// 2.1.326, whose own session for /tmp/probe3/WS.Upper Case_1+plus was filed under
// -tmp-probe3-WS-Upper-Case-1-plus: the separator, the dot, the underscore, the space and the plus all
// become "-", while case is preserved.
//
// The rule is spelled here rather than read from the inherited parser because the inherited tree only
// *decodes* a project directory (internal/parser/project.go's GetProjectName) and has no encoder; a
// decoded project name is a display name, not this directory's spelling.
func ClaudeProjectDir(cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			// Every other character, non-ASCII included, is one dash: the harness spells the directory
			// from the directory string itself, so a rune it cannot spell is not this writer's to guess.
			b.WriteByte('-')
		}
	}
	return b.String()
}

// newUUIDV4 is a fresh target identity. The target's own session ids are UUID v4 (every session file on
// the tested version is named with one) and its `--session-id` flag takes a UUID, so the writer
// generates the version the format's own identities are.
func newUUIDV4() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

// claudeTranscriptBuilder accumulates the transcript's lines and the report while it walks the
// document.
type claudeTranscriptBuilder struct {
	buffer     bytes.Buffer
	encoder    *json.Encoder
	sessionID  string
	cwd        string
	cliVersion string
	clock      *codexClock
	opts       ClaudeTargetOptions

	items      []TargetItem
	carried    Carried
	callOwners map[string]struct{}
	// messageIDs maps a source message's place in the document to the message id every assistant record
	// written for it carries, so the target's two readers put that message's blocks back together.
	messageIDs map[int]string
	// messageIndex is the place of the message being written, which keys messageIDs.
	messageIndex int
	// uuidErr records a failure to generate an identity. The walk keeps its shape (an identity is needed
	// per record) and Convert reports the failure instead of returning a transcript: a caller must never
	// install a file whose records were built without one.
	uuidErr error
	// sourceFidelity is the completeness and the provider's own classification of the read itself, as
	// in the Codex writer: a line the provider could not read produced no message, so only the read's
	// own count can name it, and a summary fallback is not the session's recorded transcript.
	sourceFidelity Fidelity

	lossReasoning              int
	lossSidechain              int
	lossSystemRecords          int
	lossToolRoleMessages       int
	lossUnclassifiedRoles      int
	lossEmptyUserMessages      int
	lossEmptyAssistantMessages int
	lossWhitespaceDialogue     int
	lossNonDialogueText        int
	lossUnpairedResults        int
	lossUnresolved             int
	lossStatuses               int
	lossPreBoundary            int
	lossEmptyInputs            int
	lossGeneratedIDs           int
	lossEmptyResults           int
	lossWhitespaceResults      int
	lossResultBlocks           int
	lossModels                 int

	foreignToolNames map[string]struct{}
	toolNameOrder    []string

	// pendingCalls holds the carried calls no result has answered yet, for the same reason the Codex
	// writer keeps it: an id-less result may only be paired while exactly one call is outstanding.
	pendingCalls map[string]struct{}
	sawBoundary  bool
	// parentUUID is the uuid of the record written last, which the next record's parentUuid names. The
	// loader walks that chain (measured: it warns "none carry parentUuid links; only N reached the
	// resumed conversation" for a file whose records are not linked), so the writer always links.
	parentUUID *string
}

func (b *claudeTranscriptBuilder) write(record claudeRecord) {
	b.encoder.Encode(record)
	if record.UUID != "" {
		uuid := record.UUID
		b.parentUUID = &uuid
	}
}

// writeConversation walks the document once and writes every record it can express. The message's place
// in the walk keys its shared message id (messageID), so two messages are never one message even in a
// document whose ordinals were never assigned.
func (b *claudeTranscriptBuilder) writeConversation(conv Conversation) {
	for index, message := range conv.Messages {
		b.messageIndex = index
		b.writeMessage(message)
	}
}

func (b *claudeTranscriptBuilder) writeMessage(message Message) {
	if message.IsSidechain {
		// A subagent's turn recorded in the same transcript. Measured on the tested version: the loader
		// drops records flagged isSidechain from the model-visible history, so a written record would be
		// evidence in the file that no read of it -- including this conversion's own round-trip check --
		// could show. The target has no subagent attribution for a flat message either, so carrying it as
		// an ordinary turn would state that the conversation's own agent said it.
		b.lossSidechain++
		return
	}
	if message.IsCompactBoundary {
		// A compaction boundary, as the pair the harness itself writes: the boundary marker the loader
		// truncates the model-visible history at, and the summary record that stands in place of the
		// summarized turns. Both halves are needed -- measured: the summary record's isCompactSummary
		// flag alone changed nothing, while the marker plus the summary made the loader start its
		// model-visible history at the summary.
		b.lossPreBoundary = b.carried.Messages
		b.sawBoundary = true
		b.write(claudeRecord{
			ParentUUID: b.parentUUID,
			Type:       claudeTypeSystem,
			UUID:       b.newUUID(),
			Timestamp:  b.clock.next(message.Timestamp),
			Cwd:        b.cwd,
			SessionID:  b.sessionID,
			Version:    b.cliVersion,
			Subtype:    claudeSubtypeCompactBoundary,
		})
		b.write(claudeRecord{
			ParentUUID:       b.parentUUID,
			IsSidechain:      false,
			Type:             claudeTypeUser,
			UUID:             b.newUUID(),
			Timestamp:        b.clock.next(message.Timestamp),
			Cwd:              b.cwd,
			SessionID:        b.sessionID,
			Version:          b.cliVersion,
			UserType:         claudeUserTypeExternal,
			IsCompactSummary: true,
			Message:          &claudeMessage{Role: claudeTypeUser, Content: b.textContent(message.Content)},
		})
		b.carried.CompactionBoundaries++
		b.items = append(b.items, TargetItem{Kind: ItemCompactionBoundary, Text: message.Content})
		return
	}
	if message.IsSystem {
		// A record the provider itself flagged as system-injected. Its own text is not the conversation
		// and the target's system records are its own; the record's tool evidence is still real evidence,
		// so the text alone is suppressed and the calls and results travel as they would anywhere else.
		b.lossSystemRecords++
		b.writeToolEvidence(message)
		return
	}
	if len(message.ToolResults) == 0 && len(message.ToolCalls) == 0 && strings.TrimSpace(message.Thinking) != "" &&
		strings.TrimSpace(dialogueText(message)) == "" {
		// Nothing but a persisted reasoning trace: the target's thinking blocks are its own
		// provider-signed artifacts and a plaintext one written in their place would present source
		// reasoning as a target-native trace (goal 0013 owns this).
		b.lossReasoning++
		return
	}
	if strings.TrimSpace(message.Thinking) != "" {
		b.lossReasoning++
	}
	if strings.TrimSpace(message.Model) != "" {
		// The source recorded which model produced this message. The target's own field for that states
		// which model produced the words, and the source's model did not run this session: writing it
		// would attribute the conversation to it, and the target warns that it cannot restore a model it
		// does not recognize. It is counted instead.
		b.lossModels++
	}

	role := parser.RoleType(message.Role)
	if role != parser.RoleUser && role != parser.RoleAssistant {
		// Only a user or assistant record is dialogue; a record the source wrote with any other role must
		// not be written as either. Its tool evidence is still written below, because a call and its
		// output carry no role of their own in the target.
		switch role {
		case parser.RoleTool:
			b.lossToolRoleMessages++
		case parser.RoleSystem:
			b.lossSystemRecords++
		default:
			b.lossUnclassifiedRoles++
		}
		if len(message.ToolCalls) == 0 && len(message.ToolResults) == 0 {
			return
		}
	} else {
		switch text := dialogueText(message); {
		case text != "" && strings.TrimSpace(text) == "":
			// The source recorded dialogue text that holds only whitespace. The inherited reader's block
			// parser carries a text block whose text is only whitespace as no text, and drops a record
			// left with no text and no tool result, so a written record would be in the file and absent
			// from every read of it. The text is not written; the record's tool evidence still is.
			b.lossWhitespaceDialogue++
		case text != "" && role == parser.RoleUser:
			b.userRecord(text, message.Timestamp)
		case text != "":
			b.assistantRecord(text, message, message.Timestamp)
		case role == parser.RoleAssistant && len(message.ToolResults) == 0 && len(message.ToolCalls) == 0:
			// An assistant record with no dialogue and no tool evidence: the reader produces that shape
			// (a usage-only row), and the target has no record for a message with nothing in it. It is
			// counted rather than dropped in silence: a caller reading "converted" must be able to see
			// that the conversation the source holds had a message this one does not.
			b.lossEmptyAssistantMessages++
			return
		case role == parser.RoleUser && len(message.ToolResults) == 0 && len(message.ToolCalls) == 0:
			// A user record with no dialogue and no tool evidence: either no text at all, or text the
			// source itself classified as not dialogue (a tool result the provider recorded with a user
			// role). Writing the second as the user's turn would state that the user said the tool's
			// words, so it is counted under its own category.
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

// assistantRecord writes one assistant message's dialogue as a record of its own, carrying the message
// id its tool records share (see messageID): the harness writes one record per content block of one API
// message, and both sides of the target -- its loader and the inherited reader -- put records that share
// a message id back together into that one message (measured: a block-per-record file and a single
// multi-block record reconstruct to exactly the same request).
func (b *claudeTranscriptBuilder) assistantRecord(text string, message Message, at time.Time) {
	b.write(claudeRecord{
		ParentUUID:  b.parentUUID,
		IsSidechain: false,
		Type:        claudeTypeAssistant,
		UUID:        b.newUUID(),
		Timestamp:   b.clock.next(at),
		Cwd:         b.cwd,
		SessionID:   b.sessionID,
		Version:     b.cliVersion,
		UserType:    claudeUserTypeExternal,
		Message: &claudeMessage{
			ID:      b.messageID(),
			Type:    "message",
			Role:    claudeTypeAssistant,
			Content: b.blocksContent([]claudeContentBlock{{Type: claudeBlockText, Text: text}}),
		},
	})
	b.carried.Messages++
	b.carried.AssistantMessages++
	b.items = append(b.items, TargetItem{Kind: ItemAssistant, Text: text})
}

// messageID is the message id every assistant record of one source message carries. It is fresh per
// source message -- a shared id is what tells both readers that these records are one message, and a
// reused id would merge two messages into one -- and generated, because the target's own ids are
// assigned by its model provider and the source did not record one this format could reuse.
func (b *claudeTranscriptBuilder) messageID() string {
	if id, ok := b.messageIDs[b.messageIndex]; ok {
		return id
	}
	id := "msg_ccc_" + strings.ReplaceAll(b.newUUID(), "-", "")
	if b.messageIDs == nil {
		b.messageIDs = map[int]string{}
	}
	b.messageIDs[b.messageIndex] = id
	return id
}

// userRecord writes one user record holding a prompt's dialogue. It is only ever called with text: a
// record whose dialogue is empty is counted by writeMessage rather than written.
func (b *claudeTranscriptBuilder) userRecord(text string, at time.Time) {
	b.write(claudeRecord{
		ParentUUID:  b.parentUUID,
		IsSidechain: false,
		Type:        claudeTypeUser,
		UUID:        b.newUUID(),
		Timestamp:   b.clock.next(at),
		Cwd:         b.cwd,
		SessionID:   b.sessionID,
		Version:     b.cliVersion,
		UserType:    claudeUserTypeExternal,
		Message:     &claudeMessage{Role: claudeTypeUser, Content: b.textContent(text)},
	})
	b.carried.Messages++
	b.carried.UserMessages++
	b.items = append(b.items, TargetItem{Kind: ItemUser, Text: text})
}

// writeToolEvidence writes the calls and results a record carries: each call as a tool_use block on an
// assistant record, each result as a tool_result block on a user record -- the target's own shapes for
// the two halves of a tool interaction, and the shape the loader passes through to the model unchanged.
func (b *claudeTranscriptBuilder) writeToolEvidence(message Message) {
	pending := make([]claudeContentBlock, 0, len(message.ToolResults)+len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		// A call is written on a record of its own -- the shape the harness writes for one block of one
		// message -- carrying the id its message's dialogue record shares, so both readers put them back
		// into one message.
		b.toolCall(message, index, call, message.Timestamp)
	}
	for _, result := range message.ToolResults {
		block, ok := b.toolResult(result, message.Timestamp)
		if ok {
			pending = append(pending, block)
		}
	}
	// A record's results travel together on one user record, the shape the harness writes for the
	// results of one assistant message.
	if len(pending) > 0 {
		b.write(claudeRecord{
			ParentUUID:  b.parentUUID,
			IsSidechain: false,
			Type:        claudeTypeUser,
			UUID:        b.newUUID(),
			Timestamp:   b.clock.next(message.Timestamp),
			Cwd:         b.cwd,
			SessionID:   b.sessionID,
			Version:     b.cliVersion,
			UserType:    claudeUserTypeExternal,
			Message:     &claudeMessage{Role: claudeTypeUser, Content: b.blocksContent(pending)},
		})
	}
}

// toolCall writes one call as an assistant record holding a single tool_use block.
func (b *claudeTranscriptBuilder) toolCall(message Message, index int, call ToolCall, at time.Time) {
	callID := strings.TrimSpace(call.ToolUseID)
	if callID == "" {
		callID = fmt.Sprintf("%s%d_%d", claudeGeneratedCallPrefix, message.Ordinal, index)
		b.lossGeneratedIDs++
	}
	// The block's input member is an object, and the encoder writes it compacted: the item list keeps the
	// bytes the file really holds (what the round-trip check compares against a read-back of it), so the
	// recorded object is compacted once here rather than left in the source's own spacing.
	input := compactJSON(call.InputJSON)
	if input == "" {
		// The source recorded no input object for this call (or recorded bytes that are not one); the
		// empty object keeps the call's identity and its output paired, and the report says so.
		input = "{}"
		b.lossEmptyInputs++
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "unknown_tool"
	}
	b.noteToolName(name)
	b.write(claudeRecord{
		ParentUUID:  b.parentUUID,
		IsSidechain: false,
		Type:        claudeTypeAssistant,
		UUID:        b.newUUID(),
		Timestamp:   b.clock.next(at),
		Cwd:         b.cwd,
		SessionID:   b.sessionID,
		Version:     b.cliVersion,
		UserType:    claudeUserTypeExternal,
		Message: &claudeMessage{
			ID:   b.messageID(),
			Type: "message",
			Role: claudeTypeAssistant,
			Content: b.blocksContent([]claudeContentBlock{{
				Type:  claudeBlockToolUse,
				ID:    callID,
				Name:  name,
				Input: json.RawMessage(input),
			}}),
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

// toolResult writes one result as a tool_result block, and answers whether it was written.
func (b *claudeTranscriptBuilder) toolResult(result ToolResult, at time.Time) (claudeContentBlock, bool) {
	callID := strings.TrimSpace(result.ToolUseID)
	resultIDInferred := false
	if callID == "" {
		pending, ok := b.solePendingCall()
		if !ok {
			b.lossUnpairedResults++
			return claudeContentBlock{}, false
		}
		callID = pending
		resultIDInferred = true
	} else if _, ok := b.callOwners[callID]; !ok {
		// The result names a call this conversion did not write: a tool_result without its call is not a
		// pair, and inventing the call would fabricate a tool invocation.
		b.lossUnpairedResults++
		return claudeContentBlock{}, false
	}
	delete(b.pendingCalls, callID)
	text, blocks := resultText(result)
	b.lossResultBlocks += blocks
	if strings.TrimSpace(text) == "" {
		if text == "" {
			b.lossEmptyResults++
		} else {
			// Text that is only whitespace: the inherited reader's block parser reads it as no text, so a
			// written block would be in the file and absent from every read of it.
			b.lossWhitespaceResults++
		}
		return claudeContentBlock{}, false
	}
	if resultIDInferred {
		b.lossGeneratedIDs++
	}
	if result.PersistedOutputUnresolved {
		b.lossUnresolved++
	}
	block := claudeContentBlock{Type: claudeBlockToolResult, ToolUseID: callID, Content: text}
	if strings.TrimSpace(result.Status) != "" {
		switch status := strings.ToLower(strings.TrimSpace(result.Status)); status {
		case "errored", "error", "failed", "failure":
			// The source's own status says the call failed, and the target's tool_result block has a
			// field for exactly that.
			block.IsError = true
		default:
			// A status with no error meaning: the output text travels as it was recorded and the status
			// itself has no field in the target.
			b.lossStatuses++
		}
	}
	b.carried.ToolResults++
	b.items = append(b.items, TargetItem{Kind: ItemToolResult, CallID: callID, Text: text})
	return block, true
}

// solePendingCall answers the one call an id-less result may belong to: exactly one carried call with
// no result yet.
func (b *claudeTranscriptBuilder) solePendingCall() (string, bool) {
	if len(b.pendingCalls) != 1 {
		return "", false
	}
	for callID := range b.pendingCalls {
		return callID, true
	}
	return "", false
}

func (b *claudeTranscriptBuilder) noteToolName(name string) {
	if b.foreignToolNames == nil {
		b.foreignToolNames = map[string]struct{}{}
	}
	if _, ok := b.foreignToolNames[name]; ok {
		return
	}
	b.foreignToolNames[name] = struct{}{}
	b.toolNameOrder = append(b.toolNameOrder, name)
}

func (b *claudeTranscriptBuilder) newUUID() string {
	generated, err := newUUIDV4()
	if err != nil {
		// Every record needs an identity, so the walk cannot continue without one; the failure is
		// recorded and Convert reports it instead of returning a transcript nobody may install.
		b.uuidErr = fmt.Errorf("generating a record id: %w", err)
		return "00000000-0000-4000-8000-000000000000"
	}
	return generated
}

// textContent is a message's content as the harness writes it for one text block: a plain JSON string.
func (b *claudeTranscriptBuilder) textContent(text string) json.RawMessage {
	if text == "" {
		return json.RawMessage(`""`)
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		// A Go string always marshals; the fallback keeps the record well-formed rather than writing a
		// half-built line.
		return json.RawMessage(`""`)
	}
	return encoded
}

// blocksContent is a content array, encoded with escaping off so the conversation's text is stored as
// it was recorded.
func (b *claudeTranscriptBuilder) blocksContent(blocks []claudeContentBlock) json.RawMessage {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(blocks); err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// compactJSON answers the compact spelling of one recorded JSON value, or "" when the bytes are not
// valid JSON: the target's tool_use block carries an object, and the encoder that writes the record
// stores it compacted, so an item list that kept the source's spacing would claim bytes the file does
// not hold.
func compactJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return ""
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, []byte(trimmed)); err != nil {
		return ""
	}
	return buffer.String()
}

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *claudeTranscriptBuilder) losses() []Loss {
	names := b.toolNameOrder
	toolDetail := toolVocabularyDetail(names, b.opts.MaxToolNames, "input objects")
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = "the turns before a compaction boundary stay in the file and are counted " +
			"here: the target applies the boundary to its model-visible history (measured: a resumed " +
			"turn could not answer from the pre-boundary turns), so those turns are recorded history " +
			"the target does not continue from"
	}
	unreadableLinesDetail := malformedLinesLossDetail(b.sourceFidelity)
	summaryTranscriptDetail := summaryTranscriptLossDetail(b.sourceFidelity)
	needsRetryDetail := needsRetryLossDetail(b.sourceFidelity)
	statusDetail := "no carried result recorded a status the target has no field for"
	if b.lossStatuses > 0 {
		statusDetail = "the target's tool_result block has a boolean error field; a result whose " +
			"recorded status says the call failed is written with it, and any other status is not " +
			"written at all -- the output text travels as it was recorded, and the status word itself " +
			"is named here rather than translated into a meaning it may not have. Only results that " +
			"were written are counted"
	}
	modelDetail := "the source recorded no model on any message"
	if b.lossModels > 0 {
		modelDetail = "the target's message record states which model produced the words, and the " +
			"source's model did not run this session: writing the source's model there would attribute " +
			"the conversation to it. The target keeps choosing its own model, and the target's own " +
			"loader warns when a transcript names a model it cannot restore"
	}
	return []Loss{
		{
			Category:    "reasoning_text",
			Disposition: DispositionOmitted,
			Count:       b.lossReasoning,
			Detail: "the target records reasoning as its provider's own signed artifact; writing the " +
				"source's plaintext reasoning in its place would present it as a target-native trace",
		},
		{
			Category:    "sidechain_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossSidechain,
			Detail: "a sidechain message is a subagent's turn recorded in the same transcript; the " +
				"target's loader drops records flagged as sidechains from the model-visible history " +
				"(measured), so a written record would be evidence in the file that no read of it shows",
		},
		{
			Category:    "system_records",
			Disposition: DispositionOmitted,
			Count:       b.lossSystemRecords,
			Detail: "the target's system records are its own; a provider's bookkeeping text does not " +
				"belong in them. Only the record's own text is omitted; a tool call or result recorded " +
				"on the same record is written as the tool evidence it is",
		},
		{
			Category:    "tool_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossToolRoleMessages,
			Detail: "a message the source recorded with the tool role is the tool's own output, and the " +
				"target's tool_result block is where a tool's words belong: the text is carried only " +
				"there, paired with the call it belongs to, and never as a user or assistant message",
		},
		{
			Category:    "unclassified_role_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossUnclassifiedRoles,
			Detail: "a record whose role this build does not classify has no message in the target to go " +
				"to; writing it as a user or assistant message would state a role the source never recorded",
		},
		{
			Category:    "empty_user_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyUserMessages,
			Detail: "a user record with neither dialogue text nor tool evidence carries nothing the " +
				"target can show, so no record is written for it",
		},
		{
			Category:    "empty_assistant_messages",
			Disposition: DispositionOmitted,
			Count:       b.lossEmptyAssistantMessages,
			Detail: "an assistant record with neither dialogue text nor tool evidence carries nothing " +
				"the target can show, so no record is written for it. The inherited reader produces " +
				"that shape for a usage-only row",
		},
		{
			Category:    "non_dialogue_content",
			Disposition: DispositionOmitted,
			Count:       b.lossNonDialogueText,
			Detail: "the source itself classified these records as carrying no dialogue, and their " +
				"content is not dialogue it rendered either (a tool result the provider recorded with a " +
				"user role): the target has no message that would not state the user said those words",
		},
		{
			Category:    "whitespace_only_dialogue",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespaceDialogue,
			Detail: "the source recorded dialogue text that holds nothing but whitespace; the inherited " +
				"reader carries a text block whose text is only whitespace as no text and drops a record " +
				"left with no text and no tool result, so a written record would be in the file but " +
				"absent from every read of it and the round-trip check would report evidence the file " +
				"holds as missing. The text is not written, and any tool evidence the same record " +
				"carries still is",
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
			Detail: "the target's tool_result block carries text; a result this read holds no text for " +
				"is not written, because an empty string would read as an empty tool output. A content " +
				"field can hold no text at all, or hold only blocks this target has no field for " +
				"(counted as unrepresentable_result_blocks); output that holds only whitespace is " +
				"counted under its own category (whitespace_only_tool_results)",
		},
		{
			Category:    "whitespace_only_tool_results",
			Disposition: DispositionOmitted,
			Count:       b.lossWhitespaceResults,
			Detail: "the source recorded output text that holds nothing but whitespace; the inherited " +
				"reader's block parser reads that as no text, so a written block would be in the file " +
				"but absent from every read of it. The output is not written, and it is named here " +
				"rather than reported as a result that recorded no text",
		},
		{
			Category:    "unrepresentable_result_blocks",
			Disposition: DispositionOmitted,
			Count:       b.lossResultBlocks,
			Detail: "the target's tool_result block carries text, and the reader's text channel carries " +
				"a block's own text (or result text) and nothing else: a block the source recorded with " +
				"other content (an image payload, a block type this build does not read) is not written. " +
				"The count is of blocks, not of results, and a block whose text was carried is not counted",
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
			Category:    "tool_result_statuses",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossStatuses,
			Detail:      statusDetail,
		},
		{
			Category:    "empty_tool_inputs",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossEmptyInputs,
			Detail: "the source recorded no input object for these calls (or recorded one that is not a " +
				"JSON object); the target's tool_use block carries an input object, so an empty object " +
				"was written",
		},
		{
			Category:    "generated_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossGeneratedIDs,
			Detail: "the source recorded no tool-use id for these calls or results; the target pairs a " +
				"call and its result by id, so one was generated from the message's own position",
		},
		{
			Category:    "tool_name_vocabulary",
			Disposition: DispositionCarriedWithCaveat,
			Count:       len(names),
			Detail:      toolDetail,
		},
		{
			Category:    "message_model_attribution",
			Disposition: DispositionOmitted,
			Count:       b.lossModels,
			Detail:      modelDetail,
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
	}
}

// claudeFormatLimits is what this pair of formats cannot express at all, as opposed to what one run
// happened to leave out: a limit carries no count, because it holds for every conversion.
func claudeFormatLimits() []FormatLimit {
	return []FormatLimit{
		{
			Category: "session_metadata_record",
			Detail: "a Codex rollout opens with a session_meta record (originator, cli version, source " +
				"kind, git provenance); a Claude Code transcript has no session-level record at all, so " +
				"those fields have no record to travel in. The conversation's own messages are unaffected",
		},
		{
			Category: "turn_boundaries",
			Detail: "Codex records a turn_context record at each turn boundary; the Claude Code " +
				"transcript has no turn record, so a converted session records no turn boundaries in " +
				"the target's vocabulary (the target groups its own turns when it resumes)",
		},
		{
			Category: "model_provider_binding",
			Detail: "the Claude Code transcript records no model provider, so a converted session is " +
				"bound to none: a resumed turn uses whatever model and provider the caller's " +
				"configuration selects, and no configuration change can make the target refuse it",
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
