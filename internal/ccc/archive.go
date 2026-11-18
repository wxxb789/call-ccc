package ccc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/sync"
)

// ErrNoArchive reports that no archive exists at the path an inspection was given. It is not an
// error condition for a source-side inspection: a session can be inspected before it is ever synced.
var ErrNoArchive = errors.New("no archive at this path")

// ErrTruncatedCandidates reports a reference whose candidate list the inherited suffix lookup could
// not complete: the archive holds more rows whose id ends in the requested native id than this lookup
// reads. Neither "not found" nor a single row is an answer such a list can support -- more rows may
// carry the requested provider and host beyond it -- so the lookup refuses instead of guessing.
var ErrTruncatedCandidates = errors.New("more sessions end in this identity than a lookup can read")

// How the inherited raw-suffix lookup is asked for candidates. It answers "up to limit" rows, most
// recently active first (db.FindSessionIDsByRawSuffix), and every rule here *filters* that answer by
// provider and host: a filtered answer is an answer only when the unfiltered one is complete. The
// lookup is therefore first asked for a handful of rows and then asked again, four times wider each
// time, until the inherited query answers fewer rows than it was allowed -- which is its own way of
// saying it had nothing more to give -- or until the cap, where the caller is told the list is
// truncated rather than handed a subset it cannot tell apart from the whole.
const (
	// rawSuffixCandidateLimit is the first page size asked for.
	rawSuffixCandidateLimit = 5
	// rawSuffixCandidateCap bounds the growth: an archive holding more rows than this that end in one
	// native id (each of them a separate remote copy or fork of one session) is not a shape a
	// reference can be resolved against by reading a subset of them.
	rawSuffixCandidateCap = 500
)

// Archive is a read-only view of an AgentsView SQLite archive. It never migrates, writes or fences
// the archive: the inherited read-only open needs no writer lock, so an inspection cannot disturb a
// running AgentsView daemon or the archive itself.
type Archive struct {
	database *db.DB
	path     string
	// roots is where each provider keeps its sessions, as the configuration answers it. One rule needs
	// it: a *relative* filesystem reference is resolved against those roots (referencePaths) before it
	// is looked up, because the archive stores the absolute path `sessions.file_path` was written with
	// and a relative spelling cannot equal it. Nil means no configuration was supplied -- an absolute
	// reference still resolves, a relative one is then only asked about as it was written.
	roots map[parser.AgentType][]string
}

// OpenArchive opens the archive at path read-only. A missing or empty path yields ErrNoArchive so
// callers can report "no archive" instead of a failure.
//
// The archive is opened without a configuration, so a relative filesystem reference cannot be
// resolved against a provider root: the roots a relative path is relative *to* come from the
// configuration, and a caller that has one passes it to openArchive through Inspect.
func OpenArchive(ctx context.Context, path string) (*Archive, error) {
	return openArchive(ctx, path, nil)
}

// openArchive opens the archive read-only with the provider roots a relative reference resolves
// against.
func openArchive(
	ctx context.Context, path string, roots map[parser.AgentType][]string,
) (*Archive, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: no path given", ErrNoArchive)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNoArchive, path)
		}
		return nil, err
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("%w: %s is empty", ErrNoArchive, path)
	}
	database, err := db.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("opening archive %s read-only: %w", path, err)
	}
	return &Archive{database: database, path: path, roots: roots}, nil
}

// providerRoots answers where the referenced provider keeps its sessions, or nil when no configuration
// was supplied.
//
// A reference written with a stored agent label that has no registry entry of its own (Freebuff) reads
// its sessions from where the provider that writes them keeps them: FREEBUFF_CONFIG_DIR re-roots the
// same projects path the Codebuff provider discovers (docs/configuration.md), so the owner's configured
// roots answer for the label too (storedAgentOwner). Without that, a *relative* Freebuff path reference
// would be resolved against no root at all and looked up only as it was written.
func (a *Archive) providerRoots(provider string) []string {
	if a.roots == nil || provider == "" {
		return nil
	}
	if roots := a.roots[parser.AgentType(provider)]; len(roots) > 0 {
		return roots
	}
	owner, ok := storedAgentOwner(parser.AgentType(provider))
	if !ok {
		return nil
	}
	return a.roots[owner]
}

// Close releases the archive handle.
func (a *Archive) Close() error {
	if a == nil || a.database == nil {
		return nil
	}
	return a.database.Close()
}

// Path is the archive file this handle reads.
func (a *Archive) Path() string { return a.path }

// ArchiveStatus is what an archive holds for one session.
type ArchiveStatus struct {
	Path          string `json:"path"`
	SessionID     string `json:"session_id"`
	Agent         string `json:"agent,omitempty"`
	AgentLabel    string `json:"agent_label,omitempty"`
	Project       string `json:"project,omitempty"`
	Cwd           string `json:"cwd,omitempty"`
	GitBranch     string `json:"git_branch,omitempty"`
	SourcePath    string `json:"source_path,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	// TranscriptFidelity is the classification the writer stored for this session
	// (parser.TranscriptFidelitySummary for a degraded fallback read). Empty means the row records
	// none, which upstream treats as full.
	TranscriptFidelity string    `json:"transcript_fidelity,omitempty"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	EndedAt            time.Time `json:"ended_at,omitempty"`
	MessageCount       int       `json:"message_count"`
	UserMessageCount   int       `json:"user_message_count"`
	MalformedLines     int       `json:"malformed_lines,omitempty"`
	Truncated          bool      `json:"truncated,omitempty"`
}

// resolveSessionID maps a reference onto the archive's stored session id.
//
// An identifier is looked up as the archive's `<agent>:<native id>` spelling first and then by the
// inherited raw-suffix answer, so a native id resolves even when the stored one carries an agent or
// machine prefix; that answer is narrowed to the rows this provider recorded (sessionIDsForProvider),
// because it has no agent predicate of its own, and it is read in full before those filters decide
// anything (sessionIDsByRawSuffix: a truncated candidate list establishes nothing). "The rows this
// provider recorded" means every label the provider writes, from the inherited mapping
// (providerStoresAgent) -- the Codebuff provider records free-tier sessions as Freebuff -- so a
// reference and the row it names do not have to carry the provider's own name. A reference that
// names a file resolves through the file the session was read from (`sessions.file_path`, exact path
// and agent): the stored id is `<agent>:<native id>`, so a path can never be one. A reference that
// names a host explicitly is resolved against that host and nothing else (resolveHostQualified), and
// more than one candidate is reported as ambiguous instead of picking one.
func (a *Archive) resolveSessionID(ctx context.Context, ref Ref) (string, bool, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return "", false, fmt.Errorf("%s: empty session id", ref.Provider)
	}
	if looksLikePath(ref.ID) {
		ids, err := a.sessionIDsForPaths(ctx, referencePaths(a.providerRoots(ref.Provider), ref.ID), ref.Provider)
		if err != nil {
			return "", false, err
		}
		switch len(ids) {
		case 0:
			return "", false, nil
		case 1:
			return ids[0], true, nil
		default:
			return "", false, &AmbiguousError{Ref: ref, Candidates: ids}
		}
	}
	if host, hostID, ok := hostQualifiedReference(ref); ok {
		return a.resolveHostQualified(ctx, ref, host, hostID)
	}
	session, err := a.database.GetSession(ctx, qualifiedID(ref))
	if err != nil {
		return "", false, err
	}
	if session != nil {
		return session.ID, true, nil
	}
	raw := rawSessionID(ref)
	matches, err := a.sessionIDsByRawSuffix(ctx, ref, raw)
	if err != nil {
		return "", false, err
	}
	matches, err = a.sessionIDsForProvider(ctx, matches, ref.Provider)
	if err != nil {
		return "", false, err
	}
	switch len(matches) {
	case 0:
		return "", false, nil
	case 1:
		return matches[0], true, nil
	default:
		return "", false, &AmbiguousError{Ref: ref, Candidates: matches}
	}
}

// resolveHostQualified resolves a reference that names a host, without the raw-ID answer that would
// substitute a local session.
//
// Upstream writes a session synced from another machine as `<host>~<the id it had there>`
// (internal/remotesync/import.go rebuilds stored ids that way) and treats the host as part of the
// session's identity (parser.StripHostPrefix). The archive's own spelling is therefore tried first,
// exactly as given -- and answered only when the row it names was recorded by the provider the
// reference asks for. A stored id is `<agent>:<native id>`, and a provider that stores its ids without
// that prefix (Claude does) makes the host-qualified spelling one that *every* provider's reference
// can name: an exact answer returned without asking whose conversation it is told an inspection that
// `codex:host-a~<a claude id>` was a Codex session. A miss, or a row of another agent, is then
// answered by the inherited suffix lookup -- read in full before anything is filtered out of it, so
// an answer is never established from a truncated candidate list (sessionIDsByRawSuffix) -- narrowed
// to the rows that carry the requested host *and* a label the requested provider writes
// (providerStoresAgent: the Codebuff provider stores free-tier sessions as Freebuff), so a caller who
// writes `host~<native id>` where the stored id is `host~<agent>:<native id>` still resolves, while a
// row from another host, or a *local* row that happens to share the native id, is never reported as the
// session the reference named. All of those are different conversations, and an inspection that
// returned one would be reading the wrong session rather than reporting an absent one.
func (a *Archive) resolveHostQualified(
	ctx context.Context, ref Ref, host, hostID string,
) (string, bool, error) {
	session, err := a.database.GetSession(ctx, hostID)
	if err != nil {
		return "", false, err
	}
	// The archive's own spelling answers for the provider it was recorded under and no other: a
	// remote row's id is `<host>~<the id it had there>`, and a provider whose stored ids carry no
	// agent prefix (Claude's do not) makes this exact spelling one that *any* provider's reference can
	// name. Returning it unasked is how `codex:host-a~<a claude id>` resolved the Claude row and an
	// inspection reported another provider's conversation under the requested label; a row of another
	// agent falls through to the filtered lookup below, which is where the provider predicate lives.
	// The predicate is the inherited provider-to-stored-label mapping (providerStoresAgent), so a
	// remote row the Codebuff provider recorded as Freebuff is answered for a `codebuff:` reference.
	if session != nil && providerStoresAgent(ref.Provider, session.Agent) {
		return session.ID, true, nil
	}
	matches, err := a.sessionIDsByRawSuffix(ctx, ref, rawSessionID(ref))
	if err != nil {
		return "", false, err
	}
	matches, err = a.sessionIDsForProvider(ctx, matches, ref.Provider)
	if err != nil {
		return "", false, err
	}
	matches = sessionIDsForHost(matches, host)
	switch len(matches) {
	case 0:
		return "", false, nil
	case 1:
		return matches[0], true, nil
	default:
		return "", false, &AmbiguousError{Ref: ref, Candidates: matches}
	}
}

// sessionIDsByRawSuffix answers the stored ids ending in a reference's native id, or reports that the
// inherited lookup could not answer them completely.
//
// The inherited query takes a limit and answers the most recently active rows up to it; the caller's
// provider and host filters are applied afterwards, so a limit that cut the answer short would let
// those filters establish an absence out of a subset. The query is therefore re-asked with a wider
// limit until it returns fewer rows than it was allowed, and a list that is still full at the cap is
// reported as ErrTruncatedCandidates rather than passed on as if it were the whole answer.
func (a *Archive) sessionIDsByRawSuffix(ctx context.Context, ref Ref, raw string) ([]string, error) {
	limit := rawSuffixCandidateLimit
	for {
		asked := min(limit, rawSuffixCandidateCap)
		ids, err := a.database.FindSessionIDsByRawSuffix(ctx, raw, asked)
		if err != nil {
			return nil, err
		}
		if len(ids) < asked {
			return ids, nil
		}
		if asked == rawSuffixCandidateCap {
			return nil, fmt.Errorf(
				"%w: %s matches %d stored id(s) ending in %q, which is as many as this lookup reads; "+
					"name the session's stored id, with its agent and host prefix, to resolve it",
				ErrTruncatedCandidates, ref.Provider+":"+ref.ID, len(ids), raw)
		}
		limit = asked * 4
	}
}

// sessionIDsForPaths answers the ids the archive records under any of the spellings a filesystem
// reference may have (see referencePaths).
//
// A path answers the way the source side does while the source is there: the rows the archive records
// as that source's *current* members (`ListSessionIDsByFilePath`, which excludes the rows of a source
// AgentsView has reconciled away -- exactly the rows a current parse of that file would emit). Only
// when no spelling has any does the archive fall back to the rows it still holds under those paths,
// read from the archive's own per-provider row listing (`ListSessionPathRecordsForAgents`, which
// reports each stored row's file path and whether its source is missing). That second step is what
// makes a reference to a deleted source resolve: reconciliation marks the row source-missing and keeps
// the conversation, so without it `ccc inspect claude:/path/<gone>.jsonl` reported no session at all
// while `ccc inspect claude:<id>` read the conversation, and the two spellings of one session
// disagreed about whether the archive holds it. It is a fallback rather than the only lookup because
// the listing reads one row per session the provider ever recorded, while the path-keyed lookup is
// indexed: with 20 000 rows for one provider the listing measures ~27 ms against ~21 µs for the
// indexed query (docs/evidence/0001-agentsview-parity-and-inspection.md, section 7), and a path
// reference to a source that is there -- the ordinary case -- has no reason to pay that.
//
// Both queries take agent names, and both are asked for *every* stored label the provider may have
// written (providerStoredAgentNames), not only for the provider's own name: a row the Codebuff provider
// recorded as Freebuff is that provider's session under this path, and asking about `codebuff` alone
// would drop it.
func (a *Archive) sessionIDsForPaths(
	ctx context.Context, paths []string, provider string,
) ([]string, error) {
	agents := providerStoredAgentNames(provider)
	current, err := a.currentSessionIDsForPaths(ctx, paths, agents)
	if err != nil {
		return nil, err
	}
	if len(current) > 0 {
		return current, nil
	}
	records, err := a.database.ListSessionPathRecordsForAgents(ctx, agents)
	if err != nil {
		return nil, err
	}
	return pathRecordIDs(records, paths), nil
}

// currentSessionIDsForPaths asks the archive's path-keyed lookup about every spelling and every stored
// agent label. Each answer is the ids recorded as that source's current members; a spelling-no-row and a
// label-no-row contribute nothing, and the rows of one provider cannot collide because `sessions.id` is
// the table's primary key and a row matches one stored path and one agent at most. The merged answer is
// sorted by id here: each inherited query orders its own rows, and which candidate list an ambiguous
// reference reports must not depend on the order the labels were asked in.
func (a *Archive) currentSessionIDsForPaths(
	ctx context.Context, paths []string, agents []string,
) ([]string, error) {
	var ids []string
	for _, agent := range agents {
		for _, path := range paths {
			found, err := a.database.ListSessionIDsByFilePath(ctx, path, agent)
			if err != nil {
				return nil, err
			}
			ids = append(ids, found...)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// pathRecordIDs selects the retained rows a filesystem reference names, and orders them.
//
// A row answers when the path it was read from is one of the reference's spellings; a user-trashed row
// does not, exactly as a trashed row answers no reference that names its stored id (the inherited
// `GetSession` requires `deleted_at IS NULL`), and a source-missing row does, because the conversation
// is still in the archive under that very path. The inherited record query does not order its answer,
// so the ids are sorted here: which candidate list an ambiguous reference reports must not depend on
// the plan SQLite chose for that query.
func pathRecordIDs(records []db.SessionPathRecord, paths []string) []string {
	wanted := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		wanted[path] = struct{}{}
	}
	ids := make([]string, 0, len(paths))
	for _, record := range records {
		if record.Trashed {
			continue
		}
		if _, ok := wanted[record.FilePath]; !ok {
			continue
		}
		ids = append(ids, record.ID)
	}
	sort.Strings(ids)
	return ids
}

// sessionIDsForHost keeps the candidates whose stored id names the requested host.
func sessionIDsForHost(ids []string, host string) []string {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if candidateHost, _ := parser.StripHostPrefix(id); candidateHost == host {
			kept = append(kept, id)
		}
	}
	return kept
}

// sessionIDsForProvider keeps the candidates the requested provider recorded.
//
// The inherited suffix lookup answers "which stored ids end in this native id" and has no agent
// predicate, while a native id can belong to any agent: without this narrowing, `codex:<a claude
// session's id>` would resolve the Claude row and an inspection would report another agent's
// conversation under this reference. The archive's own `agent` column decides, exactly as it does for
// a reference that names a file (ListSessionIDsByFilePath takes the agent with the path) -- and it
// decides through the inherited provider-to-stored-label mapping (providerStoresAgent), because a
// provider may write more than one label: the Codebuff provider records free-tier sessions as Freebuff,
// so `codebuff:<project>:<timestamp>` names a row whose `agent` column reads `freebuff`, and comparing
// that column with the provider's own name drops exactly those rows -- a Freebuff session read from its
// source then disappeared from the archive the moment that source was gone.
func (a *Archive) sessionIDsForProvider(
	ctx context.Context, ids []string, provider string,
) ([]string, error) {
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		session, err := a.database.GetSession(ctx, id)
		if err != nil {
			return nil, err
		}
		if session == nil || !providerStoresAgent(provider, session.Agent) {
			continue
		}
		kept = append(kept, id)
	}
	return kept, nil
}

// Status reports what the archive holds for a reference, without reading its messages.
func (a *Archive) Status(ctx context.Context, ref Ref) (ArchiveStatus, bool, error) {
	id, found, err := a.resolveSessionID(ctx, ref)
	if err != nil || !found {
		return ArchiveStatus{}, false, err
	}
	// The full row carries the source file metadata (path, size, mtime) that the base row omits; an
	// inspection that reported a session without its source would lose the provenance it exists for.
	session, err := a.database.GetSessionFull(ctx, id)
	if err != nil || session == nil {
		return ArchiveStatus{}, false, err
	}
	status := ArchiveStatus{
		Path:               a.path,
		SessionID:          session.ID,
		Agent:              session.Agent,
		AgentLabel:         session.AgentLabel,
		Project:            session.Project,
		Cwd:                session.Cwd,
		GitBranch:          session.GitBranch,
		SourceVersion:      session.SourceVersion,
		TranscriptFidelity: session.TranscriptFidelity,
		StartedAt:          parseTime(session.StartedAt),
		EndedAt:            parseTime(session.EndedAt),
		MessageCount:       session.MessageCount,
		UserMessageCount:   session.UserMessageCount,
		MalformedLines:     session.ParserMalformedLines,
		Truncated:          session.IsTruncated,
	}
	if session.FilePath != nil {
		status.SourcePath = *session.FilePath
	}
	return status, true, nil
}

// Read returns the archived session as a Conversation. The read names what the stored rows show
// (contentShown) -- an archive read cannot see the writer's storage policy -- so a caller can tell a
// read that carries no tool payload from one that does.
func (a *Archive) Read(ctx context.Context, ref Ref) (Conversation, error) {
	id, found, err := a.resolveSessionID(ctx, ref)
	if err != nil {
		return Conversation{}, err
	}
	if !found {
		return Conversation{}, fmt.Errorf("%w: %s:%s is not in %s", ErrNotFound, ref.Provider, ref.ID, a.path)
	}
	session, err := a.database.GetSessionFull(ctx, id)
	if err != nil {
		return Conversation{}, err
	}
	if session == nil {
		return Conversation{}, fmt.Errorf("%w: %s%s is not in %s", ErrNotFound, ref.Provider, ref.ID, a.path)
	}
	messages, err := a.database.GetAllMessages(ctx, id)
	if err != nil {
		return Conversation{}, fmt.Errorf("reading messages of %s: %w", id, err)
	}
	return conversationFromArchive(*session, messages), nil
}

func conversationFromArchive(session db.Session, messages []db.Message) Conversation {
	identity := SourceIdentity{
		Provider:         session.Agent,
		Agent:            session.AgentLabel,
		SessionID:        session.ID,
		RawSessionID:     session.SourceSessionID,
		Project:          session.Project,
		Cwd:              session.Cwd,
		GitBranch:        session.GitBranch,
		Machine:          session.Machine,
		SessionKind:      session.SessionKind,
		Entrypoint:       session.Entrypoint,
		SourceVersion:    session.SourceVersion,
		RelationshipType: session.RelationshipType,
	}
	if session.FilePath != nil {
		identity.SourcePath = *session.FilePath
	}
	if session.FileSize != nil {
		identity.FileSize = *session.FileSize
	}
	if session.FileMtime != nil {
		identity.FileModified = time.Unix(0, *session.FileMtime)
	}
	if session.ParentSessionID != nil {
		identity.ParentSessionID = *session.ParentSessionID
	}
	conversation := Conversation{
		Source:   identity,
		Messages: make([]Message, 0, len(messages)),
		Fidelity: Fidelity{
			MalformedLines: session.ParserMalformedLines,
			Truncated:      session.IsTruncated,
			SourceVersion:  session.SourceVersion,
			// The classification the writer stored travels with the read: an archive of a session whose
			// provider could only produce a degraded fallback holds that fallback, and a caller told only
			// its message count would read it as the session's recorded transcript.
			TranscriptFidelity: session.TranscriptFidelity,
			ReadFrom:           "archive",
		},
	}
	rendersThinkingTrace := readerRendersThinkingTrace(parser.AgentType(session.Agent))
	for _, archived := range messages {
		conversation.Messages = append(conversation.Messages, messageFromArchive(archived, rendersThinkingTrace))
	}
	conversation.Fidelity.ContentShown = contentShown(conversation.Messages)
	return conversation
}

// contentShown names what an archive read shows for a session, not the storage policy the writer
// enforced.
//
// A read-only handle cannot see the writer's policy: the storage boundary belongs to the writer's own
// handle and OpenReadOnly never tightens one. What a read *can* see are the rows it returned, so the
// value describes those and nothing more -- "tool-payloads" when at least one tool input or result
// body came back, "message-text" when message text came back and no tool payload did, "none" when
// neither did. The middle case is deliberately not called a policy: an archive written under the
// transcripts projection and an archive of a session that recorded no tool payloads read the same way,
// and a caller must not be told which of the two it holds.
func contentShown(messages []Message) string {
	var text, toolPayload bool
	for _, message := range messages {
		if message.Content != "" || message.Thinking != "" || message.Dialogue != "" {
			text = true
		}
		for _, call := range message.ToolCalls {
			if call.InputJSON != "" || call.ResultContent != "" {
				toolPayload = true
			}
		}
		for _, result := range message.ToolResults {
			if result.ContentRaw != "" {
				toolPayload = true
			}
		}
	}
	switch {
	case toolPayload:
		return "tool-payloads"
	case text:
		return "message-text"
	default:
		return "none"
	}
}

// messageFromArchive maps one stored row to the interchange document. rendersThinkingTrace is the
// reader's own answer about the row's Content, resolved from the row's agent label
// (readerRendersThinkingTrace): the store holds what the reader produced, so the label is the reader's
// name whether or not the source file is still on this machine.
func messageFromArchive(archived db.Message, rendersThinkingTrace bool) Message {
	message := Message{
		Ordinal:                archived.Ordinal,
		Role:                   archived.Role,
		Content:                archived.Content,
		Thinking:               archived.ThinkingText,
		Timestamp:              parseArchiveTimestamp(archived.Timestamp),
		Model:                  archived.Model,
		IsSystem:               archived.IsSystem,
		IsSidechain:            archived.IsSidechain,
		IsCompactBoundary:      archived.IsCompactBoundary,
		ContentRendersThinking: rendersThinkingTrace,
		// The writer stored the source record's own identity and the record it hangs off, so an archive
		// read answers the same relation a source read does: a target writer that reproduces the
		// source's tree (or filters to its active path) needs them, and dropping them here would make
		// the same session convert differently depending on which read reached it -- an archive-backed
		// Pi session would flatten into a chain the source's own loader would not walk.
		SourceUUID:       archived.SourceUUID,
		SourceParentUUID: archived.SourceParentUUID,
	}
	if archived.DialogueText != nil {
		message.Dialogue = *archived.DialogueText
		message.DialogueProjected = true
	}
	for _, call := range archived.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, ToolCall{
			ToolUseID:         call.ToolUseID,
			Name:              call.ToolName,
			Category:          call.Category,
			InputJSON:         call.InputJSON,
			FilePath:          call.FilePath,
			SkillName:         call.SkillName,
			SubagentSessionID: call.SubagentSessionID,
			ResultContent:     call.ResultContent,
			ResultContentSize: call.ResultContentLength,
		})
		// A tool result the source recorded as its own message is stored as events on the call
		// instead; carrying them keeps the result visible in an archive read rather than silently
		// gone. The archive stores result text, not the provider's encoded content field (the write
		// path decodes it into the column), so these are text a writer must use as it stands.
		for _, event := range call.ResultEvents {
			message.ToolResults = append(message.ToolResults, ToolResult{
				ToolUseID:      event.ToolUseID,
				Status:         event.Status,
				Source:         event.Source,
				ContentRaw:     event.Content,
				ContentSize:    event.ContentLength,
				ContentDecoded: true,
			})
		}
	}
	for _, result := range archived.ToolResults {
		// A result row of the archive holds stored text, not the provider's encoded content field: the
		// raw payload exists only while a parse is being written, and the write path decodes it into the
		// column. (Today the archive read path does not fill this field at all -- a call's outcomes
		// arrive as its events, carried just above -- so this loop is for the shapes that do.)
		message.ToolResults = append(message.ToolResults, ToolResult{
			ToolUseID:      result.ToolUseID,
			Status:         result.Status,
			Source:         result.Source,
			ContentRaw:     result.ContentRaw,
			ContentSize:    result.ContentLength,
			ContentDecoded: true,
		})
	}
	return message
}

func parseArchiveTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// writeOwnerLockFile is the lock file name an AgentsView writer owns while it writes, taken from the
// inherited direct-write path (cmd/agentsview/write_lock.go, which call/ccc cannot import).
const writeOwnerLockFile = "db.write.lock"

// qualifiedID is the archive's `<agent>:<native id>` spelling of a *local* reference. It is never the
// spelling of a host-qualified one: those are resolved by resolveHostQualified, because
// `<agent>:<native id>` would name this machine's session of the same native id.
func qualifiedID(ref Ref) string {
	raw := rawSessionID(ref)
	if ref.Provider == "" {
		return raw
	}
	return ref.Provider + ":" + raw
}

// hostQualifiedReference splits a reference that names a host explicitly, answering the host and the
// archive spelling that host-qualifies the reference.
//
// Remote-synced sessions are stored as `<host>~<native id>` and parser.StripHostPrefix calls the part
// before the tilde part of the session's identity, so a reference carrying one names one specific
// session rather than a native id that any number of hosts may also have. The provider prefix a caller
// writes in front of it (`codex:host-a~codex:<id>`, the CLI's own form) belongs to the reference, not
// to the identity, and is removed; what remains is the spelling the archive stores.
func hostQualifiedReference(ref Ref) (host, id string, ok bool) {
	id = ref.ID
	if ref.Provider != "" {
		id = strings.TrimPrefix(id, ref.Provider+":")
	}
	host, _ = parser.StripHostPrefix(id)
	if host == "" {
		return "", "", false
	}
	return host, id, true
}

// rawSessionID strips a provider prefix (and any host prefix) from a reference's identifier.
func rawSessionID(ref Ref) string {
	_, raw := parser.StripHostPrefix(ref.ID)
	if ref.Provider != "" {
		raw = strings.TrimPrefix(raw, ref.Provider+":")
	}
	return raw
}

// Ingestor writes source sessions into an archive through the inherited sync engine.
//
// It exists so the parse → archive → read path can be exercised (by tests and by the parity oracle)
// without going near a user's data: the constructor requires an explicit absolute database path, and
// IngestPaths takes the data directory's write-owner lock, so it cannot write while another
// AgentsView writer owns that directory. The CLI does not expose an ingestor at all.
type Ingestor struct {
	cfg    config.Config
	dbPath string
}

// NewIngestor prepares an ingestor for one explicit archive path.
func NewIngestor(cfg config.Config, dbPath string) (*Ingestor, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("ingest requires an explicit archive path")
	}
	if !filepath.IsAbs(dbPath) {
		return nil, fmt.Errorf("ingest path %q must be absolute", dbPath)
	}
	return &Ingestor{cfg: cfg, dbPath: filepath.Clean(dbPath)}, nil
}

// DBPath is the archive this ingestor writes.
func (i *Ingestor) DBPath() string { return i.dbPath }

// IngestPaths parses the given source files and writes them into the archive, using the same sync
// engine configuration the local AgentsView sync uses. It holds the data directory's write-owner
// lock for the duration of the write.
//
// The lock is upstream's own protocol: an AgentsView direct writer takes an exclusive flock on
// `<data dir>/db.write.lock`, and the daemon owns the same file while it writes. call/ccc takes that
// lock rather than adding a second one, and it cannot perform upstream's live-daemon check (that code
// lives in a package main call/ccc cannot import), so the CLI exposes no ingestor at all: tests and
// the parity oracle write only into fresh directories they create.
func (i *Ingestor) IngestPaths(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("ingest needs at least one source path")
	}
	if err := os.MkdirAll(filepath.Dir(i.dbPath), 0o700); err != nil {
		return fmt.Errorf("preparing %s: %w", filepath.Dir(i.dbPath), err)
	}
	lock, err := i.acquireWriteLock()
	if err != nil {
		return err
	}
	defer lock.Unlock()
	database, err := db.Open(ctx, i.dbPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", i.dbPath, err)
	}
	defer database.Close()
	engine := sync.NewEngine(ctx, database, i.engineConfig())
	defer engine.Close()
	if err := engine.SyncPathsContext(ctx, paths); err != nil {
		return fmt.Errorf("syncing %s: %w", strings.Join(paths, ", "), err)
	}
	return nil
}

// acquireWriteLock takes the data directory's write-owner lock, or reports who holds it.
func (i *Ingestor) acquireWriteLock() (*flock.Flock, error) {
	path := filepath.Join(filepath.Dir(i.dbPath), writeOwnerLockFile)
	lock := flock.New(path)
	held, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("taking %s: %w", path, err)
	}
	if !held {
		return nil, fmt.Errorf(
			"%s is held by another writer: refusing to write into %s while an AgentsView write "+
				"owner is active there", path, filepath.Dir(i.dbPath))
	}
	return lock, nil
}

// engineConfig mirrors the configuration the local AgentsView sync builds (cmd/agentsview/sync.go
// localSyncEngineConfig, and cmd/agentsview/sync_worker.go workerEngineConfig), so an ingested
// session is written the way AgentsView would write it. Two things are deliberately absent:
//
//   - RebuildSources, which contributes configured non-filesystem sources to a full archive rebuild
//     and is consulted only there (internal/sync's resyncBuildLocked); an ingest of explicit paths
//     never reaches that path.
//   - The worker's ToolResultImages and AssetsDir, which offload tool-result images into the daemon's
//     assets directory; an ingestor writes only into the database path its caller names and has no
//     assets directory, so the engine keeps the policy the database handle carries.
func (i *Ingestor) engineConfig() sync.EngineConfig {
	cfg := i.cfg
	return sync.EngineConfig{
		AgentDirs:               cfg.AgentDirs,
		SourceMachines:          cfg.SourceMachines,
		ProviderMetadata:        cfg.ProviderMetadata,
		DisabledAgents:          cfg.DisabledAgents,
		Machine:                 cfg.InstallationID,
		BlockedResultCategories: cfg.ResultContentBlockedCategories,
		IncludeCwdPrefixes:      cfg.SyncIncludeCwdPrefixes,
		ScanProtectedPaths:      cfg.ScanProtectedPaths,
		ArchiveContent:          cfg.ArchiveContent,
		FrictionEnabled:         cfg.Friction.Enabled,
	}
}
