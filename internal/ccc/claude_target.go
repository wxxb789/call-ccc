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
	// The recorded directory travels as it was recorded: trimming answers only whether one was recorded
	// at all. A directory name may legitimately end in whitespace, and every use this value has -- the
	// project directory the target's own discovery spells (ClaudeProjectDir), the recorded `cwd`, the
	// printed `cd`, and the directory the native verification runs from -- has to agree with the source.
	cwd := conv.Source.Cwd
	if strings.TrimSpace(cwd) == "" {
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
		sessionID:        sessionID,
		cwd:              cwd,
		cliVersion:       cliVersion,
		clock:            newEntryClock(startedAt, time.RFC3339Nano),
		pairings:         pairedCallOccurrences(conv),
		callOwners:       map[string]struct{}{},
		callPlaces:       map[callOccurrence]claudeWrittenCall{},
		answeredCalls:    map[callOccurrence][]int{},
		selectionReduced: conv.selectionReduced(),
		opts:             opts,
		sourceFidelity:   conv.Fidelity,
		tree:             newSourceTree(conv),
	}
	builder.encoder = json.NewEncoder(&builder.buffer)
	builder.encoder.SetEscapeHTML(false)
	builder.writeConversation(conv)
	if builder.uuidErr != nil {
		return ClaudeTranscript{}, builder.uuidErr
	}
	if err := selectedLeafRefusal(conv, &builder.tree); err != nil {
		return ClaudeTranscript{}, err
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
	clock      *entryClock
	opts       ClaudeTargetOptions

	items   []TargetItem
	carried Carried
	// pairings answers, per recorded result occurrence of the document this conversion walks, the call
	// occurrence the seam's own pairing names for it (pairedCallOccurrences, or the answers the selection
	// carried beside the reduced document). A document a selection produced checks that occurrence against
	// the written walk (sourceTree.recordOnWrittenPath): the paired occurrence has to stand on the
	// result's own written ancestry, or the file would hold an output whose invocation is off the walk a
	// resumed session makes.
	pairings map[resultOccurrence]callOccurrence
	// callOwners names every call id this walk wrote anywhere in the file, which is the acceptance a
	// document that is not a selection's asks of a recorded result (resultCallWritten): the writer
	// accepted a result when some written call shared the id, and that default path is byte-identical to
	// the same run before selections existed (goal 0009's D4).
	callOwners map[string]struct{}
	// callPlaces names, per written call occurrence, the id its tool_use block stands under and the index
	// in the transcript's item list of the call item it wrote (claudeWrittenCall). A result of a reduced
	// document is written under the owning occurrence's emitted id -- not the recorded id where the
	// document states another (colliding_call_ids, or a call that recorded none) -- and it names that
	// occurrence's call item as the one it answers (TargetItem.OwnerCall); an id-less result's inference
	// also reads the occurrences here (soleResultCall).
	callPlaces map[callOccurrence]claudeWrittenCall
	// answeredCalls names, per call occurrence, the records whose written results *stating an outcome*
	// answered it (each record's own position in the document): the completion a reduced document's
	// id-less inference reads. The question is per occurrence and per path -- whether an answer stands on
	// the written walk of the result being serialized -- because a result the window carries on another
	// branch is recorded history there and answers nothing here, while the ledger's file-wide id set (the
	// default path's answer) cannot say which branch an answer belongs to. A recorded event whose status
	// says the call is still running is not an answer and is not recorded here
	// (resultStatesAnOutcome): progress leaves the call outstanding where the result stands.
	answeredCalls map[callOccurrence][]int
	// selectionReduced says the document this conversion walks is one Select produced
	// (Conversation.selectionReduced): the written-ancestry acceptance and the path-restricted inference
	// exist for the reduction, and an unflagged run keeps the writer's earlier answers.
	selectionReduced bool
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
	lossEmptySummary           int
	lossToolRoleMessages       int
	lossUnclassifiedRoles      int
	lossEmptyUserMessages      int
	lossEmptyAssistantMessages int
	lossWhitespaceDialogue     int
	lossNonDialogueText        int
	lossUnresolvedDialogue     int
	lossUnprojectedDialogue    int
	lossUnpairedResults        int
	lossUnresolved             int
	lossStatuses               int
	lossPreBoundary            int
	lossEmptyInputs            int
	lossGeneratedIDs           int
	lossCoalescedProjections   int
	lossCollidingIDs           int
	lossSubagentLinks          int
	lossResultFallbacks        int
	lossResultInstants         int
	lossResultSources          int
	lossEmptyResults           int
	lossWhitespaceResults      int
	lossResultBlocks           int
	lossModels                 int

	ledger

	sawBoundary bool
	// tree maps the source's own record relations onto the transcript's records. The loader walks a
	// record's ancestors from the file's last record (measured: it warns "none carry parentUuid links;
	// only N reached the resumed conversation" for a file whose records are not linked), so every record
	// this writer publishes beyond the first carries a parent: the record the source named, the result
	// standing between it and the call it names, or -- for a relation the read cannot answer -- the
	// record before it, which the report counts (unresolved_source_parents). Chaining every record to
	// the one before it instead would put an abandoned branch's prompt on the walk the loader makes and
	// leave the source's own line behind a compaction marker that stands off it, which is what the
	// source's own relation is read to prevent.
	tree sourceTree
}

func (b *claudeTranscriptBuilder) write(record claudeRecord) {
	if record.ParentUUID == nil {
		record.ParentUUID = b.tree.entryParent()
	}
	b.encoder.Encode(record)
	if record.UUID != "" {
		b.tree.wrote(record.UUID)
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
	b.tree.startRecord(message.SourceUUID, message.SourceParentUUID)
	defer b.tree.endRecord()
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
			Type:      claudeTypeSystem,
			UUID:      b.newUUID(),
			Timestamp: b.clock.next(message.Timestamp),
			Cwd:       b.cwd,
			SessionID: b.sessionID,
			Version:   b.cliVersion,
			Subtype:   claudeSubtypeCompactBoundary,
		})
		b.write(claudeRecord{
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
		if strings.TrimSpace(message.Content) == "" {
			// The boundary's summary holds no text (the inherited Codex reader trims a compacted
			// record's summary, so a source summary of whitespace alone arrives here empty too, and a
			// remote compaction records none at all). The pair above is still written -- it is what
			// makes the target apply the boundary -- but there is no summary text in it to travel, and
			// the report says so rather than presenting the empty record as a carried summary.
			b.lossEmptySummary++
		}
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
	dialogue := dialogueOf(message)
	if len(message.ToolResults) == 0 && len(message.ToolCalls) == 0 && strings.TrimSpace(message.Thinking) != "" &&
		strings.TrimSpace(dialogue.Text) == "" && dialogue.State != dialogueUnestablished {
		// Nothing but a persisted reasoning trace: the target's thinking blocks are its own
		// provider-signed artifacts and a plaintext one written in their place would present source
		// reasoning as a target-native trace (goal 0013 owns this). The state is asked beside the text
		// for the same reason it is on the Codex route: a record whose dialogue the read could not
		// establish is counted as that, not as a record that held nothing but a trace.
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
	} else if dialogueLossOf(dialogue) == dialogueLossUnresolved {
		// The read established no dialogue for this record and its Content is not the record's own
		// bytes (dialogueOf): the reader rendered the record's persisted reasoning trace into a Content
		// that also holds the record's text, or the record carries calls whose renderings a reader may
		// have inlined. The text is not written -- publishing the reader's rendering as the model's
		// words is the error this rule exists to prevent, and deleting the rendering by inference is
		// the guess it refuses -- and the record is counted. Its tool evidence still travels below.
		b.lossUnresolvedDialogue++
	} else {
		if dialogueLossOf(dialogue) == dialogueLossUnprojected {
			// Content, not a projection, is what travels: the report says so per record
			// (unprojected_dialogue) rather than presenting the read's Content as the source's answer.
			b.lossUnprojectedDialogue++
		}
		switch text := dialogue.Text; {
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
//
// The source read can carry one call more than once: the inherited Codex reader projects one multi-file
// apply_patch into one call per file it names, every projection sharing the call's id and its input
// (internal/parser's handleFunctionCall, so file-keyed views can group by path). The target's tool_use
// block carries one id and one input and its reader keeps one block per id, so the projections are
// written as the single call they are projections of, and the ones not written are counted
// (coalesced_tool_call_projections) rather than leaving a file whose read-back holds fewer calls than
// the report claims to have carried.
//
// An archive read can also hold a call's only outcome in the summary column beside it (ToolCall's
// ResultContent) with no result event of its own; that summary is written for such a call, under the
// same rule the Codex writer follows (callHasRecordedResult).
func (b *claudeTranscriptBuilder) writeToolEvidence(message Message) {
	calls := b.distinctCalls(message)
	pending := make([]claudeContentBlock, 0, len(message.ToolResults)+len(calls))
	// carrier is the source record a record hanging off these outcomes would read as its parent
	// (toolResultCarrierRecord); the outcomes travel as one user record below, so one answer is taken.
	// The message's own identity is the answer where the message carries a call at all -- the read folded
	// the outcome onto the call's own message, so a record the source's file placed under that outcome
	// answers the message -- and the message's read-answered parent otherwise: a result record of its own
	// carries no call, and the reader answers a result's descendants with its visible ancestor, the parent
	// the result record was given. Registering the folded shape under the record the call's message hangs
	// off instead was measured wrong: it chained a sibling of the call through the abandoned call's
	// result, reviving the call branch as the target's active history.
	carrier := toolResultCarrierRecord(message)
	for _, entry := range calls {
		// A call is written on a record of its own -- the shape the harness writes for one block of one
		// message -- carrying the id its message's dialogue record shares, so both readers put them back
		// into one message.
		b.toolCall(entry.index, entry.call, message.Timestamp)
	}
	for index, result := range message.ToolResults {
		var paired *callOccurrence
		if place, ok := b.pairings[resultOccurrence{message: b.messageIndex, result: index}]; ok {
			paired = &place
		}
		block, ok := b.toolResult(paired, result, message.Timestamp)
		if ok {
			pending = append(pending, block)
		}
	}
	// An archive read carries a call's outcome both as the result events the archive kept on the call and
	// as the summary it holds in ResultContent. That summary is derived from those same events by the
	// inherited write path (internal/db stores it only when it is not the sole content-bearing event's
	// text, and restores it from that event when the column was cleared), so beside them it is a view of
	// the recorded evidence rather than another outcome recorded there -- writing it too would present a
	// normalized display aggregate as a separately recorded tool output. A summary whose call carries no
	// result at all is the only evidence the read holds of that outcome, and it is written.
	for _, entry := range calls {
		call := entry.call
		result, ok := derivedResultSummary(call, call.ToolUseID)
		if !ok {
			continue
		}
		if callHasRecordedResult(message.ToolResults, call.ToolUseID) {
			continue
		}
		// The summary is the call's own evidence and the call was written just above, on this very
		// record: the pairing to hand the writer is that occurrence, so the ancestry check asks about the
		// one place the pair really stands at.
		place := callOccurrence{message: b.messageIndex, call: entry.index}
		block, ok := b.toolResult(&place, result, message.Timestamp)
		if ok {
			pending = append(pending, block)
		}
	}
	// A record's results travel together on one user record, the shape the harness writes for the
	// results of one assistant message.
	if len(pending) > 0 {
		b.tree.markToolResult(carrier)
		b.write(claudeRecord{
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

// projectedCall is one call the writer writes, with the place it holds in the source message: an id-less
// call's generated id is derived from that place (toolCall), so the projection list has to keep it.
type projectedCall struct {
	index int
	call  ToolCall
}

// distinctCalls answers one message's calls as this target writes them, in the message's own order: a
// call whose id the message already carried is another projection of that call (the per-file expansion
// above) and not a second invocation, so only the first of them is written and each one skipped is
// counted. A call with no recorded id is never treated as a repeat: its id is generated from its own
// place in the message, so two of them are two calls.
func (b *claudeTranscriptBuilder) distinctCalls(message Message) []projectedCall {
	return distinctCalls(message, func(index int, call ToolCall) projectedCall {
		return projectedCall{index: index, call: call}
	}, &b.lossCoalescedProjections)
}

// toolCall writes one call as an assistant record holding a single tool_use block.
//
// The id its block is written under is the recorded one, except where a document a selection produced
// already wrote an earlier occurrence under it: the target pairs a call with its output by id (its reader
// keeps one tool_use block per id), so the occurrence is written under an id generated from its own place
// and the substitution is counted (colliding_call_ids) rather than left to read as the recorded id. A
// run without a selection keeps the ids the source recorded, exactly as before (goal 0009's D4).
func (b *claudeTranscriptBuilder) toolCall(index int, call ToolCall, at time.Time) {
	callID := strings.TrimSpace(call.ToolUseID)
	switch {
	case callID == "":
		// The call's place in the document keys its generated id -- the message's position in the walk
		// and the call's index in that message -- and not the message's own ordinal: an embedding caller
		// may leave ordinals unassigned, and two messages' id-less calls must not collapse into one id.
		// A shared id would make the pairing below read one call, so an id-less result could pair with
		// "the" outstanding call while two are. The id is walked past every id this walk already wrote,
		// so it can never alias a recorded spelling either.
		callID = b.freshCallID(fmt.Sprintf("%s%d_%d", claudeGeneratedCallPrefix, b.messageIndex, index))
		b.lossGeneratedIDs++
	case b.selectionReduced && b.callIDTaken(callID):
		// A document a selection produced writes each call occurrence the read holds, and the source
		// recorded this id on an earlier occurrence too. The target pairs a call with its output by id --
		// its reader keeps one tool_use block per id -- so an occurrence written under the shared id
		// could not be told from the earlier one: an output inferred for the earlier occurrence would
		// read back as this one's outcome. The occurrence is written under an id generated from its own
		// place (the same shape an id-less call takes), and the substitution is counted
		// (colliding_call_ids) rather than left to read as the recorded id. A run without a selection
		// keeps the ids the source recorded, exactly as before (goal 0009's D4).
		callID = b.freshCallID(fmt.Sprintf("%s%d_%d", claudeGeneratedCallPrefix, b.messageIndex, index))
		b.lossCollidingIDs++
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
	if strings.TrimSpace(call.SubagentSessionID) != "" {
		// The source recorded which session this call spawned: the inherited Codex parser sets the child
		// session id on a spawn_agent call, and the archive read carries the same column. The target's
		// tool_use block carries an id, a name and an input and has no field for a child session, so the
		// link has no shape to travel in -- and a conversion's unit is the selected session, not the
		// sessions it spawned, so the child is neither read nor converted. Counted rather than dropped in
		// silence: the call itself is written below.
		b.lossSubagentLinks++
	}
	b.write(claudeRecord{
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
	if b.pendingCalls == nil {
		b.pendingCalls = map[string]struct{}{}
	}
	b.pendingCalls[callID] = struct{}{}
	// The occurrence, not the id: a later result is paired by the read's own answer about *which* call
	// occurrence it names, and this is the one whose entry stands here. The id set beside it is the
	// acceptance a document that is not a selection's asks (resultCallWritten); the item index is the
	// owner a result written for this occurrence states (TargetItem.OwnerCall).
	occurrence := callOccurrence{message: b.messageIndex, call: index}
	b.callPlaces[occurrence] = claudeWrittenCall{id: callID, item: len(b.items)}
	b.callOwners[callID] = struct{}{}
	b.carried.ToolCalls++
	b.items = append(b.items, TargetItem{Kind: ItemToolCall, CallID: callID, ToolName: name, InputJSON: input})
}

// claudeWrittenCall is one call occurrence this walk wrote: the id its tool_use block stands under --
// which is not the recorded id where an earlier occurrence already held that one (colliding_call_ids) --
// and the index in the transcript's item list of the call item itself, which is what a result written for
// the occurrence states as its owner (TargetItem.OwnerCall), so the round-trip check can assert the
// attachment the target's own reader makes.
type claudeWrittenCall struct {
	id   string
	item int
}

// callIDTaken answers whether some call this walk already wrote stands under one id: the target pairs a
// call with its output by id, so a second occurrence under the same id cannot be told from the first.
func (b *claudeTranscriptBuilder) callIDTaken(callID string) bool {
	_, taken := b.callOwners[callID]
	return taken
}

// freshCallID answers an id the document does not already state: base where no written call holds it, and
// otherwise base with a suffix counted until one is free, so a generated id can never alias a recorded
// spelling or another generated one.
func (b *claudeTranscriptBuilder) freshCallID(base string) string {
	id := base
	for suffix := 2; b.callIDTaken(id); suffix++ {
		id = fmt.Sprintf("%s_%d", base, suffix)
	}
	return id
}

// resultCallWritten answers whether a recorded result that names a call may be written under that call's
// id: whether some written call stands under the id. This is the default path's own acceptance. A
// document a selection produced never asks it: the switch in toolResult reads the paired occurrence's
// entry instead (or omits the result where the read's pairing names none), because the id alone cannot
// say which occurrence an output answers where two written calls share it. A conversation the read
// returned keeps the question this writer has always asked, so a run without a selection is
// byte-identical to the same run before selections existed (goal 0009's D4).
func (b *claudeTranscriptBuilder) resultCallWritten(callID string) bool {
	_, ok := b.callOwners[callID]
	return ok
}

// soleResultCall answers the call an id-less result may be paired with, and whether there is exactly one
// such call. A conversation the read returned asks the ledger's own question -- the one call still
// outstanding anywhere in the walk. A document a selection produced asks the narrower one: the sole call
// occurrence that is outstanding *on the written walk of the result's record*
// (sourceTree.recordOnWrittenPath) and that no written result stating an outcome, standing on that walk,
// has answered (answeredOnWrittenPath). This file holds the records the read's line leaves off it as
// recorded history, so the outstanding set can include a call on another branch; inferring the result
// onto such a call would write an output whose invocation the resumed context cannot see, and
// `generated_call_ids` discloses the inference rather than a placement. The eligible set is counted in
// call *occurrences*: two occurrences the source recorded under one id are two calls, and a result the
// read left no link for could belong to either, so the inference picks no call where two are eligible --
// whatever ids they carry. The result is then omitted and counted.
func (b *claudeTranscriptBuilder) soleResultCall() (callOccurrence, string, bool) {
	if !b.selectionReduced {
		id, ok := b.solePendingCall()
		return callOccurrence{}, id, ok
	}
	found := ""
	foundAt := callOccurrence{}
	foundAny := false
	for occurrence, written := range b.callPlaces {
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
			return callOccurrence{}, "", false
		}
		found, foundAt, foundAny = written.id, occurrence, true
	}
	return foundAt, found, foundAny
}

// answeredOnWrittenPath answers whether a written result standing on the place now being walked has
// already answered one call occurrence. A result the writer accepted answers its call where it stands:
// the records whose results answered the occurrence are remembered (answeredCalls) and the question is
// whether any of them stands on the written ancestry of the result being serialized
// (sourceTree.recordOnWrittenPath). An answer on another branch leaves the call outstanding here, which
// the ledger's own file-wide id set cannot express: that set is what cleared a call a sibling branch's
// outcome had answered, so an id-less result was inferred onto a later call the read never paired it
// with although both calls were outstanding on the result's own path.
func (b *claudeTranscriptBuilder) answeredOnWrittenPath(occurrence callOccurrence) bool {
	for _, record := range b.answeredCalls[occurrence] {
		if b.tree.recordOnWrittenPath(record) {
			return true
		}
	}
	return false
}

// toolResult writes one result as a tool_result block, and answers whether it was written. paired is the
// call occurrence the seam's own pairing names for the result (pairedCallOccurrences), or the call a
// derived summary came from; nil where the read holds no call occurrence for it.
//
// The id the block is written under is the owning occurrence's *emitted* id, never the recorded id read
// back off the result: a document a selection produced may have written that occurrence under an id
// generated from its own place (colliding_call_ids, or a call that recorded none), and a result written
// under the recorded spelling would read back as the outcome of whichever occurrence does hold it. An
// id-less result whose owner is not known from a pairing -- an independent result record the read left no
// link for -- is inferred from the outstanding set, and only there is an inference disclosed
// (generated_call_ids).
func (b *claudeTranscriptBuilder) toolResult(paired *callOccurrence, result ToolResult, at time.Time) (claudeContentBlock, bool) {
	callID := strings.TrimSpace(result.ToolUseID)
	// resultIDInferred says the id below is one this conversion answered for the result: the source
	// recorded none of its own, so the id is the owning occurrence's generated one or the sole outstanding
	// call's. The counter it feeds is incremented past the omission guards, with the written block,
	// because an inferred id on a result this conversion does not write is not evidence the file holds.
	resultIDInferred := false
	answered := callOccurrence{}
	switch {
	case b.selectionReduced && paired != nil:
		// The pairing names the occurrence this result answers -- the read's own answer for a recorded
		// result, or the call a derived summary came from -- and the id the file holds for it is the one
		// that occurrence was written under, which is not the recorded id where the document states
		// another (a colliding id, an id-less call's generated one).
		//
		// The occurrence has to be one this walk wrote *and* one that stands on the written walk: this
		// writer keeps the records the read's line leaves off it as recorded history, so a paired call
		// can be in the file and still off the walk the target's loader makes from the result. Accepting
		// the pairing there would write a tool_result whose invocation the resumed session cannot see,
		// which is what the reduction's window can produce (the paired call stands on another branch).
		written, ok := b.callPlaces[*paired]
		if !ok || !b.tree.recordOnWrittenPath(paired.message) {
			b.lossUnpairedResults++
			return claudeContentBlock{}, false
		}
		if callID == "" {
			// A derived summary the read stored on the call itself (ToolCall.ResultContent): its owner
			// is the occurrence it came from, so the result may not be inferred off the outstanding set
			// -- an inference picks no call where two are eligible, and would drop an output whose owner
			// was already known.
			resultIDInferred = true
		}
		callID = written.id
		answered = *paired
	case callID == "":
		occurrence, pending, ok := b.soleResultCall()
		if !ok {
			b.lossUnpairedResults++
			return claudeContentBlock{}, false
		}
		callID = pending
		answered = occurrence
		resultIDInferred = true
	case b.selectionReduced:
		// A result that names an id, on a reduced document, whose read pairing names no written
		// occurrence: omitted and counted exactly as the paired-absent case above.
		b.lossUnpairedResults++
		return claudeContentBlock{}, false
	default:
		// The default path keeps the acceptance it has always asked: some written call stands under the
		// id (goal 0009's D4).
		if !b.resultCallWritten(callID) {
			b.lossUnpairedResults++
			return claudeContentBlock{}, false
		}
	}
	if !b.selectionReduced {
		// The default path keeps the ledger's own answer: the result clears its id from the file-wide
		// set, whether or not the text below gives it a block (goal 0009's D4).
		delete(b.pendingCalls, callID)
	}
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
	if b.selectionReduced {
		// The accepted result answers its call on the place it stands at, and only where it is
		// *written*: a result whose text the guard above omitted -- empty or whitespace -- has no answer
		// on the written path, so a later id-less result still sees the call outstanding and is omitted
		// rather than inferred onto another call. A result whose recorded status says the call is still
		// running is progress rather than an answer (resultStatesAnOutcome): it travels as the evidence
		// it is, and it leaves no mark on the outstanding set either.
		if resultStatesAnOutcome(result.Status) {
			b.answeredCalls[answered] = append(b.answeredCalls[answered], b.messageIndex)
			delete(b.pendingCalls, callID)
		}
	}
	if resultIDInferred {
		b.lossGeneratedIDs++
	}
	if result.PersistedOutputUnresolved {
		b.lossUnresolved++
	}
	// The record this block is written into (the record of the message the result travelled on) states
	// that message's instant: a result's own recorded instant is not what the file holds, and the two
	// reads of it are counted apart -- the read answered an instant and the containing record's is
	// written instead, or the read answered none and the containing record's instant stands in.
	if result.Timestamp.IsZero() {
		b.lossResultFallbacks++
	} else if !result.Timestamp.Equal(at) {
		b.lossResultInstants++
	}
	if strings.TrimSpace(result.Source) != "" {
		// The read named which native record carried the result's text (a result block in a message, a
		// `function_call_output`, a wait call's output event, a subagent notification). The target's
		// block carries the text and no such label -- its own read answers its own for its own shapes --
		// so the source's label is omitted and counted rather than dropped in silence.
		b.lossResultSources++
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
	item := TargetItem{Kind: ItemToolResult, CallID: callID, Text: text}
	if b.selectionReduced {
		// A document a selection produced knows the owning occurrence (the pairing, or the disclosed
		// inference that picked one), so the result item states the place of that occurrence's call item
		// and the round-trip check asserts the attachment the reader makes instead of trusting that an id
		// two occurrences could share identifies the right one.
		if written, ok := b.callPlaces[answered]; ok {
			owner := written.item
			item.OwnerCall = &owner
		}
	}
	b.items = append(b.items, item)
	return block, true
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

// compactJSON answers the compact spelling of one recorded JSON object, or "" when the bytes are not
// one: the target's tool_use block carries an object, so a recorded array, scalar or null is refused
// exactly as bytes that are not JSON at all are, and the caller writes -- and reports -- its empty
// object for it. (A valid JSON text starting with '{' is an object, so the shape test needs no parse of
// its own.) The encoder that writes the record stores the object compacted, so an item list that kept
// the source's spacing would claim bytes the file does not hold.
func compactJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed[0] != '{' || !json.Valid([]byte(trimmed)) {
		return ""
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, []byte(trimmed)); err != nil {
		return ""
	}
	return buffer.String()
}

// claudeUnestablishedOutcome is this route's own answer for a record whose dialogue was not established
// (dialogueUnestablished), passed to dialogueLossRows: what the writer does not write and where the
// record's reasoning goes. This target records reasoning as its provider's own signed artifact, so a
// plaintext trace has no shape here -- it is counted as omitted above (reasoning_text). The clause first
// said the trace "still travels as reasoning_text", which is no route's answer and contradicts this
// route's own reasoning_text row (round 17's PD-3).
const claudeUnestablishedOutcome = "the text is not written, this count is not a claim that the record " +
	"wrote none, and a trace the record carries is omitted with it -- counted under reasoning_text " +
	"above, because this target has no plaintext-trace shape: it records reasoning as its provider's " +
	"own signed artifact"

// losses reports every category the writer knows about, in a fixed order, so a caller can read the
// report without knowing which categories this run happened to produce.
func (b *claudeTranscriptBuilder) losses() []Loss {
	names := b.toolNameOrder
	toolDetail := toolVocabularyDetail(names, b.opts.MaxToolNames, "input objects")
	preBoundaryDetail := "no compaction boundary was carried"
	if b.sawBoundary {
		preBoundaryDetail = preBoundaryHistoryDetail()
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
	coalescedDetail := "the source read recorded no call more than once"
	if b.lossCoalescedProjections > 0 {
		coalescedDetail = "the source read recorded these calls once for every file they touched: the " +
			"inherited Codex reader projects one apply_patch into one call per file it names, every " +
			"projection sharing the call's id and its input, so the archive's file-keyed views can " +
			"group by path. The target's tool_use block carries one id and one input, and its reader " +
			"keeps one block per id, so the call is written as the single call those projections are " +
			"projections of, and a call whose recorded id repeats with some other input is answered the " +
			"same way. The call and its input travel -- the patch's own text names every file it " +
			"touched -- and what has no field in the target is the per-file attribution of a second " +
			"projection, which is what this count names"
	}
	losses := []Loss{
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
				"content is not dialogue it rendered either (a tool result the provider recorded with " +
				"a user role): the target has no message that would not state the user said those words",
		},
	}
	// The two rows the dialogue states are reported under come from the one constructor
	// (dialogueLossRows), with this route's own answer for an unestablished record.
	losses = append(losses, dialogueLossRows(b.lossUnresolvedDialogue, b.lossUnprojectedDialogue,
		claudeUnestablishedOutcome)...)
	losses = append(losses, []Loss{
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
			Category:    "coalesced_tool_call_projections",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCoalescedProjections,
			Detail:      coalescedDetail,
		},
		{
			Category:    "subagent_session_links",
			Disposition: DispositionOmitted,
			Count:       b.lossSubagentLinks,
			Detail: "the source recorded which session these carried calls spawned (the inherited " +
				"Codex parser sets a spawn_agent call's child session id, and the archive read carries " +
				"the same column). The target's tool_use block carries an id, a name and an input, so " +
				"there is no field for a child session: the call itself is written, and what is omitted " +
				"is the attribution of the session it spawned. The child session is neither read nor " +
				"converted -- a conversion's unit is the selected session -- and the count is of the " +
				"calls this conversion wrote, so a projection coalesced into another call is not " +
				"counted twice",
		},
		{
			Category:    "tool_result_sources",
			Disposition: DispositionOmitted,
			Count:       b.lossResultSources,
			Detail: "the read names which native record carried each result's text (a result block " +
				"inside a message, a `function_call_output`, a wait call's own output event, a " +
				"subagent notification: internal/parser's readers answer this per shape). The " +
				"target's tool_result block carries the text and no such label -- its own read " +
				"answers the analogous question for its own shapes -- so the source's label is " +
				"omitted, counted per result this conversion wrote that recorded one",
		},
		{
			Category:    "result_completion_fallbacks",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossResultFallbacks,
			Detail: "a tool_result block carries the result's text, and the record carrying it states " +
				"the instant of the message the result travelled on. Where the read holds no instant " +
				"of the result's own record -- a provider-recorded event with no timestamp, or the " +
				"archive's derived summary, which is stored on the call's own row and has no record " +
				"of its own -- that containing record's instant is written, an instant the read " +
				"answered for that record rather than one this build invented; only results this " +
				"conversion wrote are counted here",
		},
		{
			Category:    "result_record_instants",
			Disposition: DispositionOmitted,
			Count:       b.lossResultInstants,
			Detail: "the source recorded the instant the result itself was recorded on (a result " +
				"event's own envelope instant, which may be later than the record that carried its " +
				"call, and which the archive read answers from the stored event's own column). A " +
				"Claude Code tool_result block carries an id, its content and an error flag and no " +
				"instant of its own, so the record carrying it states the containing message's " +
				"instant instead: the result's own recorded instant is not what the file holds. " +
				"Counted per result this conversion wrote and only where the read answered one",
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
			Category:    "message_model_attribution",
			Disposition: DispositionOmitted,
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
			Detail: "the source recorded a compaction boundary whose summary holds no text (the " +
				"inherited Codex reader trims a compacted record's summary, so a summary of whitespace " +
				"alone arrives the same way). The boundary pair is still written, because the marker " +
				"and the summary record are what make the target apply the boundary; the summary record " +
				"holds the empty text rather than an invented summary, and no summary text travels",
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
	// The colliding-id category exists only where the collision can be substituted, exactly as the Codex
	// writer scopes it: only a document a selection produced writes every call occurrence the read holds
	// beside the ids the source recorded, and an unflagged run keeps those ids (goal 0009's D4), so its
	// report keeps its own category list unchanged -- a zero row cannot be added to every report. The
	// count is zero on a reduced document with no collision, so the completeness rule holds within the
	// selection surface as the other writers' does.
	if b.selectionReduced {
		losses = append(losses, Loss{
			Category:    "colliding_call_ids",
			Disposition: DispositionCarriedWithCaveat,
			Count:       b.lossCollidingIDs,
			Detail: "these call occurrences recorded an id an earlier written occurrence already " +
				"holds; the target pairs a call with its output by id (its reader keeps one tool_use " +
				"block per id), so the occurrence was written under an id generated from its own place " +
				"and the file does not state the id the source recorded for it",
		})
	}
	return losses
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
