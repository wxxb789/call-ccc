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

// Resolve finds the source a reference names without parsing it.
func (r *SourceReader) Resolve(ctx context.Context, ref Ref) (parser.SourceRef, bool, error) {
	def, factory, err := r.resolveFactory(ref.Provider)
	if err != nil {
		return parser.SourceRef{}, false, err
	}
	if strings.TrimSpace(ref.ID) == "" {
		return parser.SourceRef{}, false, fmt.Errorf("%s: empty session id", ref.Provider)
	}
	return resolveWith(ctx, r.provider(def, factory), def, ref)
}

// resolveWith asks one provider instance for the reference's source.
//
// A reference that names a filesystem path is offered as a stored path first, because providers
// resolve a real path to its session identity; a path no provider claims is then classified as a
// changed path, and a classification that yields several candidates is reported as ambiguous rather
// than resolved to the first one.
func resolveWith(
	ctx context.Context, provider parser.Provider, def parser.AgentDef, ref Ref,
) (parser.SourceRef, bool, error) {
	id := ref.ID
	if strings.HasPrefix(id, ref.Provider+":") {
		id = strings.TrimPrefix(id, ref.Provider+":")
	}
	if looksLikePath(id) {
		if source, found, err := provider.FindSource(ctx, parser.FindSourceRequest{
			StoredFilePath: filepath.Clean(id),
		}); err != nil {
			return parser.SourceRef{}, false, err
		} else if found {
			return source, true, nil
		}
		candidates, err := provider.SourcesForChangedPath(ctx, parser.ChangedPathRequest{
			Path:      filepath.Clean(id),
			EventKind: "write",
		})
		if err != nil {
			return parser.SourceRef{}, false, err
		}
		switch len(candidates) {
		case 0:
			return parser.SourceRef{}, false, nil
		case 1:
			return candidates[0], true, nil
		default:
			names := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				names = append(names, candidate.DisplayPath)
			}
			return parser.SourceRef{}, false, &AmbiguousError{Ref: ref, Candidates: names}
		}
	}
	request := parser.ProviderFindRequestWithRawSessionID(def, parser.FindSourceRequest{RawSessionID: id})
	return provider.FindSource(ctx, request)
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
	source, found, err := resolveWith(ctx, provider, def, ref)
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
	return Conversation{
		Source:   identity,
		Messages: messages,
		Fidelity: Fidelity{
			MalformedLines: session.MalformedLines,
			Truncated:      session.IsTruncated,
			SourceVersion:  session.SourceVersion,
			ReadFrom:       "source",
		},
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
			ToolUseID:   result.ToolUseID,
			Status:      result.Status,
			Source:      result.Source,
			ContentRaw:  result.ContentRaw,
			ContentSize: result.ContentLength,
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
