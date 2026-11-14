package ccc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// Archive is a read-only view of an AgentsView SQLite archive. It never migrates, writes or fences
// the archive: the inherited read-only open needs no writer lock, so an inspection cannot disturb a
// running AgentsView daemon or the archive itself.
type Archive struct {
	database *db.DB
	path     string
}

// OpenArchive opens the archive at path read-only. A missing or empty path yields ErrNoArchive so
// callers can report "no archive" instead of a failure.
func OpenArchive(ctx context.Context, path string) (*Archive, error) {
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
	return &Archive{database: database, path: path}, nil
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
	Path             string    `json:"path"`
	SessionID        string    `json:"session_id"`
	Agent            string    `json:"agent,omitempty"`
	AgentLabel       string    `json:"agent_label,omitempty"`
	Project          string    `json:"project,omitempty"`
	Cwd              string    `json:"cwd,omitempty"`
	GitBranch        string    `json:"git_branch,omitempty"`
	SourcePath       string    `json:"source_path,omitempty"`
	SourceVersion    string    `json:"source_version,omitempty"`
	StartedAt        time.Time `json:"started_at,omitempty"`
	EndedAt          time.Time `json:"ended_at,omitempty"`
	MessageCount     int       `json:"message_count"`
	UserMessageCount int       `json:"user_message_count"`
	MalformedLines   int       `json:"malformed_lines,omitempty"`
	Truncated        bool      `json:"truncated,omitempty"`
}

// resolveSessionID maps a reference onto the archive's stored session id.
//
// An identifier is looked up as the archive's `<agent>:<native id>` spelling first and then by the
// inherited raw-suffix answer, so a native id resolves even when the stored one carries an agent or
// machine prefix. A reference that names a file resolves through the file the session was read from
// (`sessions.file_path`, exact path and agent): the stored id is `<agent>:<native id>`, so a path can
// never be one. More than one candidate is reported as ambiguous instead of picking one.
func (a *Archive) resolveSessionID(ctx context.Context, ref Ref) (string, bool, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return "", false, fmt.Errorf("%s: empty session id", ref.Provider)
	}
	if looksLikePath(ref.ID) {
		ids, err := a.database.ListSessionIDsByFilePath(ctx, filepath.Clean(ref.ID), ref.Provider)
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
	session, err := a.database.GetSession(ctx, qualifiedID(ref))
	if err != nil {
		return "", false, err
	}
	if session != nil {
		return session.ID, true, nil
	}
	raw := rawSessionID(ref)
	matches, err := a.database.FindSessionIDsByRawSuffix(ctx, raw, 5)
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
		Path:             a.path,
		SessionID:        session.ID,
		Agent:            session.Agent,
		AgentLabel:       session.AgentLabel,
		Project:          session.Project,
		Cwd:              session.Cwd,
		GitBranch:        session.GitBranch,
		SourceVersion:    session.SourceVersion,
		StartedAt:        parseTime(session.StartedAt),
		EndedAt:          parseTime(session.EndedAt),
		MessageCount:     session.MessageCount,
		UserMessageCount: session.UserMessageCount,
		MalformedLines:   session.ParserMalformedLines,
		Truncated:        session.IsTruncated,
	}
	if session.FilePath != nil {
		status.SourcePath = *session.FilePath
	}
	return status, true, nil
}

// Read returns the archived session as a Conversation. The read names the projection the stored rows
// show (storageProjection), so a caller can tell a narrowed archive from a complete one.
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
			ReadFrom:       "archive",
		},
	}
	for _, archived := range messages {
		conversation.Messages = append(conversation.Messages, messageFromArchive(archived))
	}
	conversation.Fidelity.Projection = storageProjection(conversation.Messages)
	return conversation
}

// storageProjection names the content policy an archive read shows for a session.
//
// A read-only handle cannot know the writer's policy: the storage boundary belongs to the writer's
// own handle and OpenReadOnly never tightens one, so the projection is derived from the rows the read
// returns -- which is also what a reader needs, since it says what the archive kept and dropped. The
// three policies leave distinguishable traces in the stored rows (internal/db's archive_content
// projection): usage clears every message body and keeps only usage rows, transcripts keeps bodies
// while dropping tool inputs and result content, and full keeps tool payloads. A read that shows a
// tool payload is therefore full, one that shows a body but no payload is transcripts, and one that
// shows neither is usage -- as is an archive whose source carried none of that text, which reads the
// same way.
func storageProjection(messages []Message) string {
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
		return "full"
	case len(messages) > 0 && text:
		return "transcripts"
	default:
		return "usage"
	}
}

func messageFromArchive(archived db.Message) Message {
	message := Message{
		Ordinal:           archived.Ordinal,
		Role:              archived.Role,
		Content:           archived.Content,
		Thinking:          archived.ThinkingText,
		Timestamp:         parseArchiveTimestamp(archived.Timestamp),
		Model:             archived.Model,
		IsSystem:          archived.IsSystem,
		IsSidechain:       archived.IsSidechain,
		IsCompactBoundary: archived.IsCompactBoundary,
	}
	if archived.DialogueText != nil {
		message.Dialogue = *archived.DialogueText
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
		// gone.
		for _, event := range call.ResultEvents {
			message.ToolResults = append(message.ToolResults, ToolResult{
				ToolUseID:   event.ToolUseID,
				Status:      event.Status,
				Source:      event.Source,
				ContentRaw:  event.Content,
				ContentSize: event.ContentLength,
			})
		}
	}
	for _, result := range archived.ToolResults {
		message.ToolResults = append(message.ToolResults, ToolResult{
			ToolUseID:   result.ToolUseID,
			Status:      result.Status,
			Source:      result.Source,
			ContentRaw:  result.ContentRaw,
			ContentSize: result.ContentLength,
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

// qualifiedID is the archive's `<agent>:<native id>` spelling of a reference.
func qualifiedID(ref Ref) string {
	raw := rawSessionID(ref)
	if ref.Provider == "" {
		return raw
	}
	return ref.Provider + ":" + raw
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
