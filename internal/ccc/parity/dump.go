// Package parity produces the canonical dump that proves this tree's inherited AgentsView behaviour
// agrees with the pinned upstream revision.
//
// The dump is deterministic and host-independent: it hashes every content-bearing field, includes
// only fixture-derived values, and names the fields it deliberately leaves out. Two trees running the
// same oracle source over the same fixture bytes must therefore produce byte-identical dumps, which
// is what makes a diff of the two a real comparison rather than a claim.
//
// The package imports only inherited packages (plus the call/ccc core that wraps them), so the same
// source compiles in a pristine checkout of the pinned revision: scripts/upstream-compare.py copies
// internal/ccc there, runs cmd/ccc's oracle equivalent, and diffs the two dumps.
package parity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/ccc"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// ToolID identifies the dump format. It changes when the dumped shape changes, so a stale committed
// dump cannot be mistaken for an agreement. It is /3 from the round-10 fix pass, where the counts
// object gained the tool-role count (a message list's "tool" rows were being reported as system
// messages, so the dumped counts changed with the classification).
const ToolID = "call-ccc-parity-oracle/3"

// ExcludedFields names what the dump leaves out and why, so a reader can tell a deliberate exclusion
// from an omission that hides a difference.
var ExcludedFields = []string{
	"absolute paths (the two trees are checked out at different locations)",
	"file mtime and inode (the fixtures are copied separately for each side)",
	"archive created_at / sync bookkeeping timestamps (host clock)",
	"database file name and data directory (per-side temporary directories)",
}

// Options describes one comparison run.
type Options struct {
	// Fixtures is a directory holding one subdirectory per provider, each of which is that
	// provider's configured source root.
	Fixtures string
	// WorkDir holds the isolated archive this run writes through the inherited sync engine.
	WorkDir string
	// Provider is the fixture to run; the case is the provider's name.
	Provider string
}

// SessionDump is what one read of a session looks like in the dump.
type SessionDump struct {
	SessionID      string       `json:"session_id"`
	Provider       string       `json:"provider"`
	Agent          string       `json:"agent"`
	RawSessionID   string       `json:"raw_session_id,omitempty"`
	SourceFile     string       `json:"source_file"`
	Project        string       `json:"project,omitempty"`
	Cwd            string       `json:"cwd,omitempty"`
	GitBranch      string       `json:"git_branch,omitempty"`
	SourceVersion  string       `json:"source_version,omitempty"`
	ReadFrom       string       `json:"read_from"`
	ContentShown   string       `json:"content_shown,omitempty"`
	MalformedLines int          `json:"malformed_lines"`
	Truncated      bool         `json:"truncated"`
	Counts         ccc.Counts   `json:"counts"`
	Messages       []MessageRow `json:"messages"`
}

// MessageRow is one message with its content reduced to a digest.
type MessageRow struct {
	Ordinal           int             `json:"ordinal"`
	Role              string          `json:"role"`
	Timestamp         string          `json:"timestamp,omitempty"`
	Model             string          `json:"model,omitempty"`
	IsSystem          bool            `json:"is_system"`
	IsSidechain       bool            `json:"is_sidechain"`
	IsCompactBoundary bool            `json:"is_compact_boundary"`
	ContentSHA256     string          `json:"content_sha256"`
	ThinkingSHA256    string          `json:"thinking_sha256"`
	DialogueSHA256    string          `json:"dialogue_sha256"`
	ToolCalls         []ToolCallRow   `json:"tool_calls,omitempty"`
	ToolResults       []ToolResultRow `json:"tool_results,omitempty"`
}

// ToolCallRow is one tool call, with the provider's raw input reduced to a digest.
type ToolCallRow struct {
	Name         string `json:"name"`
	Category     string `json:"category,omitempty"`
	ToolUseID    string `json:"tool_use_id,omitempty"`
	InputSHA256  string `json:"input_sha256"`
	ResultSHA256 string `json:"result_sha256,omitempty"`
	ResultSize   int    `json:"result_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}

// ToolResultRow is one tool result.
type ToolResultRow struct {
	ToolUseID     string `json:"tool_use_id,omitempty"`
	Source        string `json:"source,omitempty"`
	Status        string `json:"status,omitempty"`
	ContentSHA256 string `json:"content_sha256"`
	ContentSize   int    `json:"content_size"`
}

// Fixture describes the bytes both sides read.
type Fixture struct {
	RelativeToRoot string `json:"relative_to_root"`
	Bytes          int    `json:"bytes"`
	SHA256         string `json:"sha256"`
}

// Dump is one comparison run.
type Dump struct {
	Tool           string      `json:"tool"`
	Case           string      `json:"case"`
	Fixture        Fixture     `json:"fixture"`
	Parse          SessionDump `json:"parse"`
	Archive        SessionDump `json:"archive"`
	ExcludedFields []string    `json:"excluded_fields"`
	Notes          []string    `json:"notes,omitempty"`
}

// Run parses one fixture, writes it through the inherited sync engine into an isolated archive, reads
// it back through the inherited read path, and returns the canonical dump.
func Run(ctx context.Context, opts Options) (Dump, error) {
	root, err := filepath.Abs(filepath.Join(opts.Fixtures, opts.Provider))
	if err != nil {
		return Dump{}, err
	}
	fixture, err := findFixture(root)
	if err != nil {
		return Dump{}, err
	}
	cfg, err := isolatedConfig(root, opts.WorkDir)
	if err != nil {
		return Dump{}, err
	}
	reader := ccc.NewSourceReader(cfg)
	ref := ccc.Ref{Provider: opts.Provider, ID: fixture}

	parsed, err := reader.Read(ctx, ref)
	if err != nil {
		return Dump{}, fmt.Errorf("parsing %s: %w", fixture, err)
	}
	ingestor, err := ccc.NewIngestor(cfg, cfg.DBPath)
	if err != nil {
		return Dump{}, err
	}
	if err := ingestor.IngestPaths(ctx, fixture); err != nil {
		return Dump{}, err
	}
	archive, err := ccc.OpenArchive(ctx, cfg.DBPath)
	if err != nil {
		return Dump{}, err
	}
	defer archive.Close()
	archived, err := archive.Read(ctx, ccc.Ref{Provider: parsed.Source.Provider, ID: parsed.Source.SessionID})
	if err != nil {
		return Dump{}, fmt.Errorf("reading the archived session: %w", err)
	}

	content, err := os.ReadFile(fixture)
	if err != nil {
		return Dump{}, err
	}
	return Dump{
		Tool: ToolID,
		Case: opts.Provider,
		Fixture: Fixture{
			RelativeToRoot: strings.TrimPrefix(filepath.ToSlash(fixture), filepath.ToSlash(root)+"/"),
			Bytes:          len(content),
			SHA256:         digest(content),
		},
		Parse:          dumpSession(parsed, root),
		Archive:        dumpSession(archived, root),
		ExcludedFields: ExcludedFields,
	}, nil
}

// Marshal renders a dump the way it is committed: indented, one field per line, key order fixed by
// the struct, with a trailing newline.
func Marshal(dump Dump) ([]byte, error) {
	encoded, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	return encoded, nil
}

// findFixture returns the single JSONL fixture below a provider root. More than one is an error
// rather than a silent pick, so a comparison always names the bytes it read.
func findFixture(root string) (string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no .jsonl fixture under %s", root)
	case 1:
		return found[0], nil
	default:
		sort.Strings(found)
		return "", fmt.Errorf("%d fixtures under %s; a case reads exactly one: %s",
			len(found), root, strings.Join(found, ", "))
	}
}

// isolatedConfig points the inherited providers at one fixture root and every write at a temporary
// directory; the installation id is cleared so it cannot leak a host identity into the dump.
func isolatedConfig(root, workDir string) (config.Config, error) {
	cfg, err := config.Default()
	if err != nil {
		return config.Config{}, err
	}
	provider := parser.AgentType(filepath.Base(root))
	cfg.DataDir = workDir
	cfg.DBPath = filepath.Join(workDir, "sessions.db")
	cfg.AgentDirs = map[parser.AgentType][]string{provider: {root}}
	cfg.DisabledAgents = nil
	cfg.InstallationID = ""
	return cfg, nil
}

func dumpSession(conversation ccc.Conversation, root string) SessionDump {
	counts := ccc.Summarize(conversation.Messages)
	dump := SessionDump{
		SessionID:      conversation.Source.SessionID,
		Provider:       conversation.Source.Provider,
		Agent:          conversation.Source.Agent,
		RawSessionID:   conversation.Source.RawSessionID,
		SourceFile:     relativeTo(root, conversation.Source.SourcePath),
		Project:        conversation.Source.Project,
		Cwd:            conversation.Source.Cwd,
		GitBranch:      conversation.Source.GitBranch,
		SourceVersion:  conversation.Source.SourceVersion,
		ReadFrom:       conversation.Fidelity.ReadFrom,
		ContentShown:   conversation.Fidelity.ContentShown,
		MalformedLines: conversation.Fidelity.MalformedLines,
		Truncated:      conversation.Fidelity.Truncated,
		Counts:         counts,
		Messages:       make([]MessageRow, 0, len(conversation.Messages)),
	}
	for _, message := range conversation.Messages {
		row := MessageRow{
			Ordinal:           message.Ordinal,
			Role:              message.Role,
			Model:             message.Model,
			IsSystem:          message.IsSystem,
			IsSidechain:       message.IsSidechain,
			IsCompactBoundary: message.IsCompactBoundary,
			ContentSHA256:     digest([]byte(message.Content)),
			ThinkingSHA256:    digest([]byte(message.Thinking)),
			DialogueSHA256:    digest([]byte(message.Dialogue)),
		}
		if !message.Timestamp.IsZero() {
			row.Timestamp = message.Timestamp.UTC().Format(time.RFC3339Nano)
		}
		for _, call := range message.ToolCalls {
			row.ToolCalls = append(row.ToolCalls, ToolCallRow{
				Name:         call.Name,
				Category:     call.Category,
				ToolUseID:    call.ToolUseID,
				InputSHA256:  digest([]byte(call.InputJSON)),
				ResultSHA256: digest([]byte(call.ResultContent)),
				ResultSize:   call.ResultContentSize,
				FilePath:     call.FilePath,
			})
		}
		for _, result := range message.ToolResults {
			row.ToolResults = append(row.ToolResults, ToolResultRow{
				ToolUseID:     result.ToolUseID,
				Source:        result.Source,
				Status:        result.Status,
				ContentSHA256: digest([]byte(result.ContentRaw)),
				ContentSize:   result.ContentSize,
			})
		}
		dump.Messages = append(dump.Messages, row)
	}
	return dump
}

// relativeTo keeps the path below the provider root and drops the rest: the two trees are checked out
// at different absolute locations, so an absolute path could never match.
func relativeTo(root, path string) string {
	if path == "" {
		return ""
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		// A provider may report a virtual or rewritten path; keeping only its base name is the most
		// that can be compared across checkouts.
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
