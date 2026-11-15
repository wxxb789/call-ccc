package ccc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// ErrNotFound reports that a reference resolved to nothing a provider can read. Callers distinguish
// it from a provider failure so an inspection can say "not found" instead of "broken".
var ErrNotFound = errors.New("session not found")

// AmbiguousError reports a reference that matches more than one source. The candidates are returned
// instead of a guess, which is what the design asks for when an identifier is ambiguous.
type AmbiguousError struct {
	Ref        Ref
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%s:%s matches %d sources: %s",
		e.Ref.Provider, e.Ref.ID, len(e.Candidates), strings.Join(e.Candidates, ", "))
}

// SourceReader reads native agent sessions through the inherited AgentsView providers. It holds no
// global state; the provider set and roots come from the configuration it was built with.
type SourceReader struct {
	cfg       config.Config
	factories map[parser.AgentType]parser.ProviderFactory
}

// NewSourceReader builds a reader over the providers the configuration enables. The provider set is
// the same one the inherited sync engine uses (metadata-bound, disabled agents dropped), so a
// session this reader finds is one a sync would find.
func NewSourceReader(cfg config.Config) *SourceReader {
	factories := make(map[parser.AgentType]parser.ProviderFactory)
	for _, factory := range cfg.LocalProviderFactories() {
		factories[factory.Definition().Type] = factory
	}
	return &SourceReader{cfg: cfg, factories: factories}
}

// Providers lists the provider identifiers this reader can resolve, sorted.
func (r *SourceReader) Providers() []string {
	providers := make([]string, 0, len(r.factories))
	for agent := range r.factories {
		providers = append(providers, string(agent))
	}
	sort.Strings(providers)
	return providers
}

// AgentLabel returns the provider's display name, and whether the provider exists at all.
func (r *SourceReader) AgentLabel(provider string) (string, bool) {
	def, ok := parser.AgentByType(parser.AgentType(provider))
	if !ok {
		return "", false
	}
	return def.DisplayName, true
}

// provider builds one provider instance bound to the configured roots.
func (r *SourceReader) provider(def parser.AgentDef, factory parser.ProviderFactory) parser.Provider {
	return factory.NewProvider(parser.ProviderConfig{
		Roots:   r.cfg.ResolveDirs(def.Type),
		Machine: r.cfg.InstallationID,
	})
}

func (r *SourceReader) resolveFactory(provider string) (parser.AgentDef, parser.ProviderFactory, error) {
	def, ok := parser.AgentByType(parser.AgentType(provider))
	if !ok {
		return parser.AgentDef{}, nil, fmt.Errorf("unknown provider %q (known: %s)",
			provider, strings.Join(r.Providers(), ", "))
	}
	factory, ok := r.factories[def.Type]
	if !ok {
		return def, nil, fmt.Errorf("provider %q is disabled in this configuration", provider)
	}
	return def, factory, nil
}

// ErrNoLocalSource reports a reference that names a host: a session synced from another machine has no
// source file on this one, which is what the inherited engine says about one too (its FindSourceFile
// answers "" for a `host~...` id). Callers report it as a source that is not readable here rather than
// as a failure -- an archive can hold the session -- but they must never answer it with a local session
// that merely shares the native id.
var ErrNoLocalSource = errors.New("host-qualified session has no local source")

// Resolve finds the source a reference names without parsing it.
func (r *SourceReader) Resolve(ctx context.Context, ref Ref) (parser.SourceRef, bool, error) {
	def, factory, err := r.resolveFactory(ref.Provider)
	if err != nil {
		return parser.SourceRef{}, false, err
	}
	if strings.TrimSpace(ref.ID) == "" {
		return parser.SourceRef{}, false, fmt.Errorf("%s: empty session id", ref.Provider)
	}
	return resolveWith(ctx, r.provider(def, factory), r.cfg.ResolveDirs(def.Type), def, ref)
}

// resolveWith asks one provider instance for the reference's source.
//
// A reference that names a filesystem path is offered as a stored path under every spelling it may
// have (referencePaths: a relative one is relative to a configured root), because providers resolve a
// real path to its session identity; a path no provider claims is then classified as a changed path,
// and a classification that yields several candidates is reported as ambiguous rather than resolved to
// the first one. A reference that names a host is refused: the inherited providers would strip the host
// and answer with this machine's session of the same native id.
func resolveWith(
	ctx context.Context, provider parser.Provider, roots []string, def parser.AgentDef, ref Ref,
) (parser.SourceRef, bool, error) {
	id := ref.ID
	if strings.HasPrefix(id, ref.Provider+":") {
		id = strings.TrimPrefix(id, ref.Provider+":")
	}
	if looksLikePath(id) {
		return resolvePathReference(ctx, provider, referencePaths(roots, id), ref)
	}
	if host, _, ok := hostQualifiedReference(ref); ok {
		// A remote session's identity is the one the reference named. The providers cannot read it from
		// here (parser.ProviderNormalizeRawSessionID drops the host and searches this machine), so the
		// reader says there is no local source instead of reporting a different conversation.
		return parser.SourceRef{}, false, fmt.Errorf("%w: %s (host %s)", ErrNoLocalSource, ref.ID, host)
	}
	request := parser.ProviderFindRequestWithRawSessionID(def, parser.FindSourceRequest{RawSessionID: id})
	return provider.FindSource(ctx, request)
}

// referencePaths returns the filesystem spellings a path reference may be stored under.
//
// An absolute reference is itself, cleaned, and a home-prefixed one is offered as it was written:
// expanding `~` is the shell's job, nothing in the inherited providers does it, and joining it to a
// root would invent a path under a directory named `~`. A relative one is resolved against each
// configured provider root: the inherited providers compare a candidate with their own absolute roots
// (the Claude provider resolves the pair with filepath.Rel) and reject a mixed absolute/relative pair,
// and the archive stores the absolute path `sessions.file_path` was written with, so a relative spelling
// matches neither. The roots a relative path is relative to are the ones the configuration gives the
// provider -- the only reading that does not depend on the working directory of the process doing the
// inspecting. A provider with no configured roots leaves the reference as it was written, which is all a
// provider without roots could answer anyway, and a root listed twice is asked about once, since the
// answer cannot differ.
func referencePaths(roots []string, id string) []string {
	cleaned := filepath.Clean(id)
	if filepath.IsAbs(cleaned) || len(roots) == 0 || strings.HasPrefix(cleaned, "~") {
		return []string{cleaned}
	}
	paths := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		joined := filepath.Clean(filepath.Join(root, cleaned))
		if _, ok := seen[joined]; ok {
			continue
		}
		seen[joined] = struct{}{}
		paths = append(paths, joined)
	}
	return paths
}

// resolvePathReference asks one provider about every spelling a path reference may be stored under.
//
// Each spelling is put through the provider's own two questions in the same order: what a stored path
// resolves to, and -- for a spelling no stored path claims -- what a changed path of that name
// classifies as. A classification is not a readable source on its own: the inherited classifiers
// answer for a path that is *absent* as well as for one that is there (jsonlMissingPathFallbackAllowed
// exists so a watcher does not drop a deletion), so a candidate no file stands behind is dropped
// (readableSources) -- otherwise a relative reference would be ambiguous between the root that holds
// the session and one that does not, and a source deleted after its archive was written would be
// reported as readable and then fail while fingerprinting instead of falling back to the archive. A
// classification that yields several readable sources for one spelling is reported as it stands,
// because the provider's answer is "these transcripts read this path" and the reference names one
// session; when several spellings resolve, the caller is told which sessions the reference could mean
// rather than being handed the first.
func resolvePathReference(
	ctx context.Context, provider parser.Provider, paths []string, ref Ref,
) (parser.SourceRef, bool, error) {
	sources := make([]parser.SourceRef, 0, len(paths))
	for _, path := range paths {
		source, found, err := provider.FindSource(ctx, parser.FindSourceRequest{StoredFilePath: path})
		if err != nil {
			return parser.SourceRef{}, false, err
		}
		if !found {
			candidates, err := provider.SourcesForChangedPath(ctx, parser.ChangedPathRequest{
				Path:      path,
				EventKind: "write",
			})
			if err != nil {
				return parser.SourceRef{}, false, err
			}
			candidates = readableSources(path, candidates)
			switch len(candidates) {
			case 0:
				continue
			case 1:
				source = candidates[0]
			default:
				return parser.SourceRef{}, false, &AmbiguousError{
					Ref: ref, Candidates: displayedPaths(candidates),
				}
			}
		}
		sources = append(sources, source)
	}
	switch len(sources) {
	case 0:
		return parser.SourceRef{}, false, nil
	case 1:
		return sources[0], true, nil
	default:
		return parser.SourceRef{}, false, &AmbiguousError{Ref: ref, Candidates: displayedPaths(sources)}
	}
}

// readableSources keeps the changed-path classifications a file actually stands behind.
//
// The inherited providers classify a path that does not exist -- deliberately, so a deletion is not
// dropped by the watcher -- and construct a source from the path's own layout to do it. Such a
// classification is not a source this reader can read: `Fingerprint` stats the file and fails, so the
// reference would be answered with a source that cannot be read, and a caller would lose the archive
// that holds the session to that error. A candidate is therefore kept when a regular file stands
// behind it -- the spelling that was asked about, or the path the provider says that spelling is (a
// companion file's owning transcript, a store member's container file) -- and the same predicate the
// providers' own stored-path lookup applies is what decides (parser.IsRegularFile, which is what
// keeps a changed *present* path's classification): a reference no file stands behind resolves
// nothing here, which is what lets an inspection report the archive instead.
func readableSources(spelling string, candidates []parser.SourceRef) []parser.SourceRef {
	spellingPresent := parser.IsRegularFile(spelling)
	kept := make([]parser.SourceRef, 0, len(candidates))
	for _, candidate := range candidates {
		if spellingPresent || parser.IsRegularFile(candidate.FingerprintKey) {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// displayedPaths names sources by the path a caller can act on, for an ambiguity report.
func displayedPaths(sources []parser.SourceRef) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.DisplayPath)
	}
	return names
}

// looksLikePath reports whether an identifier is meant as a filesystem path rather than a native
// session id. Session ids are single path segments, so a separator, a home prefix or a known session
// file suffix all mean "path".
func looksLikePath(id string) bool {
	if filepath.IsAbs(id) || strings.HasPrefix(id, "~") {
		return true
	}
	if strings.ContainsAny(id, `/\`) {
		return true
	}
	switch strings.ToLower(filepath.Ext(id)) {
	case ".jsonl", ".json", ".db", ".sqlite", ".sqlite3":
		return true
	}
	return false
}

// Read resolves a reference and parses it into a Conversation using the provider's own reader.
func (r *SourceReader) Read(ctx context.Context, ref Ref) (Conversation, error) {
	def, factory, err := r.resolveFactory(ref.Provider)
	if err != nil {
		return Conversation{}, err
	}
	provider := r.provider(def, factory)
	source, found, err := resolveWith(ctx, provider, r.cfg.ResolveDirs(def.Type), def, ref)
	if err != nil {
		return Conversation{}, err
	}
	if !found {
		return Conversation{}, fmt.Errorf("%w: %s:%s", ErrNotFound, ref.Provider, ref.ID)
	}
	fingerprint, err := provider.Fingerprint(ctx, source)
	if err != nil {
		return Conversation{}, fmt.Errorf("fingerprinting %s: %w", source.DisplayPath, err)
	}
	outcome, err := provider.Parse(ctx, parser.ParseRequest{
		Source:      source,
		Fingerprint: fingerprint,
		Machine:     r.cfg.InstallationID,
	})
	if err != nil {
		return Conversation{}, fmt.Errorf("parsing %s: %w", source.DisplayPath, err)
	}
	result, ok := pickResult(outcome, ref)
	if !ok {
		return Conversation{}, fmt.Errorf("%w: %s:%s produced no session (%s)",
			ErrNotFound, ref.Provider, ref.ID, describeOutcome(outcome))
	}
	return conversationFromParse(result, source, fingerprint), nil
}

// pickResult chooses the parsed session a reference named. A multi-session source (a container or a
// database) returns several; the reference's own id decides, and a single result is taken as-is.
func pickResult(outcome parser.ParseOutcome, ref Ref) (parser.ParseResult, bool) {
	if len(outcome.Results) == 0 {
		return parser.ParseResult{}, false
	}
	if len(outcome.Results) == 1 {
		return outcome.Results[0].Result, true
	}
	_, raw := parser.StripHostPrefix(ref.ID)
	for _, candidate := range outcome.Results {
		session := candidate.Result.Session
		if session.SourceSessionID == raw || session.ID == ref.ID ||
			strings.HasSuffix(session.ID, ":"+raw) {
			return candidate.Result, true
		}
	}
	return parser.ParseResult{}, false
}

// describeOutcome explains an empty parse result without inventing one.
func describeOutcome(outcome parser.ParseOutcome) string {
	parts := []string{}
	if outcome.SkipReason != parser.SkipNone {
		parts = append(parts, "skipped: "+skipReasonText(outcome.SkipReason))
	}
	for _, sourceErr := range outcome.SourceErrors {
		target := sourceErr.DisplayPath
		if target == "" {
			target = sourceErr.SessionID
		}
		if target == "" {
			target = sourceErr.SourceKey
		}
		parts = append(parts, fmt.Sprintf("source error on %s: %v", target, sourceErr.Err))
	}
	if len(outcome.ExcludedSessionIDs) > 0 {
		parts = append(parts, fmt.Sprintf("%d excluded session(s)", len(outcome.ExcludedSessionIDs)))
	}
	if len(parts) == 0 {
		parts = append(parts, "the provider reported no result")
	}
	return strings.Join(parts, "; ")
}

// skipReasonText names the provider's own skip reasons; the enum has no String method, and an
// inspection that reported a bare number would be unreadable.
func skipReasonText(reason parser.SkipReason) string {
	switch reason {
	case parser.SkipNoSession:
		return "no session at this source"
	case parser.SkipUnsupportedSource:
		return "unsupported source"
	case parser.SkipNonInteractive:
		return "non-interactive session"
	case parser.SkipShadowedBySidecar:
		return "shadowed by a sidecar source"
	default:
		return fmt.Sprintf("reason %d", reason)
	}
}

func conversationFromParse(
	result parser.ParseResult, source parser.SourceRef, fingerprint parser.SourceFingerprint,
) Conversation {
	session := result.Session
	identity := SourceIdentity{
		Provider:         string(session.Agent),
		SessionID:        session.ID,
		RawSessionID:     session.SourceSessionID,
		SourcePath:       session.File.Path,
		DisplayPath:      source.DisplayPath,
		Project:          session.Project,
		Cwd:              session.Cwd,
		GitBranch:        session.GitBranch,
		GitRemote:        session.GitRemote,
		Machine:          session.Machine,
		SessionKind:      session.SessionKind,
		Entrypoint:       session.Entrypoint,
		ParentSessionID:  session.ParentSessionID,
		RelationshipType: string(session.RelationshipType),
		SourceVersion:    session.SourceVersion,
		Fingerprint:      fingerprint.Hash,
		FileSize:         session.File.Size,
	}
	if identity.Provider == "" {
		identity.Provider = string(source.Provider)
	}
	if session.File.Mtime != 0 {
		identity.FileModified = time.Unix(0, session.File.Mtime)
	}
	if def, ok := parser.AgentByType(parser.AgentType(identity.Provider)); ok {
		identity.Agent = def.DisplayName
	}
	if identity.Fingerprint == "" {
		identity.Fingerprint = fingerprint.Key
	}
	messages := make([]Message, 0, len(result.Messages))
	for _, parsed := range result.Messages {
		messages = append(messages, messageFromParsed(parsed))
	}
	fidelity := Fidelity{
		MalformedLines: session.MalformedLines,
		Truncated:      session.IsTruncated,
		SourceVersion:  session.SourceVersion,
		ReadFrom:       "source",
	}
	// Counted from the messages this read produced rather than from the parse result, so the summary a
	// caller reports and the flags it can see on each result cannot disagree.
	for _, message := range messages {
		for _, result := range message.ToolResults {
			if result.PersistedOutputUnresolved {
				fidelity.UnresolvedToolOutputs++
			}
		}
	}
	return Conversation{
		Source:   identity,
		Messages: messages,
		Fidelity: fidelity,
	}
}

func messageFromParsed(parsed parser.ParsedMessage) Message {
	message := Message{
		Ordinal:           parsed.Ordinal,
		Role:              string(parsed.Role),
		Content:           parsed.Content,
		Thinking:          parsed.ThinkingText,
		Timestamp:         parsed.Timestamp,
		Model:             parsed.Model,
		IsSystem:          parsed.IsSystem,
		IsSidechain:       parsed.IsSidechain,
		IsCompactBoundary: parsed.IsCompactBoundary,
	}
	if parsed.DialogueText != nil {
		message.Dialogue = *parsed.DialogueText
	}
	for _, call := range parsed.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, ToolCall{
			ToolUseID:         call.ToolUseID,
			Name:              call.ToolName,
			Category:          call.Category,
			InputJSON:         call.InputJSON,
			FilePath:          call.FilePath,
			SkillName:         call.SkillName,
			SubagentSessionID: call.SubagentSessionID,
		})
		// A provider that records a call's outcome as events on the call rather than as a message of
		// its own -- Codex's function_call_output, a subagent's terminal status -- keeps that outcome
		// there, and an archive read of the same session shows the same rows. Carrying them is what
		// keeps a source read from being the narrower of the two.
		for _, event := range call.ResultEvents {
			message.ToolResults = append(message.ToolResults, ToolResult{
				ToolUseID:   event.ToolUseID,
				Status:      event.Status,
				Source:      event.Source,
				ContentRaw:  event.Content,
				ContentSize: len(event.Content),
			})
		}
	}
	for _, result := range parsed.ToolResults {
		message.ToolResults = append(message.ToolResults, ToolResult{
			ToolUseID:                 result.ToolUseID,
			Status:                    result.Status,
			Source:                    result.Source,
			ContentRaw:                result.ContentRaw,
			ContentSize:               result.ContentLength,
			PersistedOutputUnresolved: result.PersistedOutputUnresolved,
		})
	}
	return message
}

// parseTime reads a session-row timestamp. The archive's timestamp columns hold more than one layout
// -- internal/db/analytics.go lists RFC 3339, RFC 3339Nano and two space-separated legacy forms, and
// parseArchiveTimestamp accepts the message-row spellings -- so this strict reader accepts RFC 3339
// only and reports anything else as the zero time instead of guessing.
func parseTime(value *string) time.Time {
	if value == nil || *value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
