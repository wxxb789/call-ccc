// Command ccc inspects an existing agent session through the AgentsView source-side implementation
// this repository carries unmodified.
//
// It is a thin shell over go.kenn.io/agentsview/internal/ccc: argument handling and presentation
// only, so the same capability is available to a caller that embeds the core (and to the parity
// oracle, which calls it directly).
//
//	ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]
//	ccc help
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.kenn.io/agentsview/internal/ccc"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

const usage = `ccc — inspect an agent session through AgentsView's own reader

usage:
  ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]
  ccc help

  <provider> is the provider identifier AgentsView uses for the agent that wrote the session
  (for example claude, codex). The identifier after it is the provider's own session id, or the
  path to the session's source file.

  inspect reports the session's identity, agent, source and conversation shape. It reads the
  source session through the provider's own reader and, when an archive is present, says what the
  archive holds for the same session. It never writes: the source session and any archive are read
  only.

options:
  --json         write the inspection as one JSON object on stdout (diagnostics stay on stderr)
  --data-dir DIR read the archive in DIR (%s/sessions.db) instead of the configured data directory

exit codes:
  0  the session was found and reported
  1  nothing was found, the reference was ambiguous, or the read failed
  2  the command line could not be understood
`

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, usage, defaultDataDirLabel())
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintf(stdout, usage, defaultDataDirLabel())
		return exitOK
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ccc: unknown command %q\n\n", args[0])
		fmt.Fprintf(stderr, usage, defaultDataDirLabel())
		return exitUsage
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", false, "write the inspection as JSON")
	dataDir := flags.String("data-dir", "", "data directory whose sessions.db is consulted")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]\n")
	}
	flagArgs, operands, err := splitFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitUsage
	}
	if err := flags.Parse(flagArgs); err != nil {
		return exitUsage
	}
	if rest := flags.Args(); len(rest) > 0 {
		operands = append(operands, rest...)
	}
	if len(operands) != 1 {
		flags.Usage()
		return exitUsage
	}
	ref, err := parseRef(operands[0])
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitUsage
	}

	cfg, err := config.LoadReadOnly()
	if err != nil {
		fmt.Fprintf(stderr, "ccc: reading the AgentsView configuration: %v\n", err)
		return exitFailure
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
		cfg.DBPath = filepath.Join(*dataDir, "sessions.db")
	}

	reader := ccc.NewSourceReader(cfg)
	inspection, err := ccc.Inspect(context.Background(), ccc.InspectOptions{
		Ref:         ref,
		Reader:      reader,
		ArchivePath: cfg.DBPath,
	})
	if err != nil {
		var ambiguous *ccc.AmbiguousError
		if errors.As(err, &ambiguous) {
			fmt.Fprintf(stderr, "ccc: %v\n", err)
			fmt.Fprintf(stderr, "ccc: pass one of those session ids instead of a guess\n")
			return exitFailure
		}
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitFailure
	}
	if inspection.Source == nil && inspection.Archive == nil {
		fmt.Fprintf(stderr, "ccc: no session found for %s:%s (providers: %s)\n",
			ref.Provider, ref.ID, strings.Join(reader.Providers(), ", "))
		return exitFailure
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(inspectDocument{
			UpstreamRevision: upstreamRevision,
			Inspection:       inspection,
		}); err != nil {
			fmt.Fprintf(stderr, "ccc: writing the inspection: %v\n", err)
			return exitFailure
		}
		return exitOK
	}
	writeHuman(stdout, inspection)
	return exitOK
}

// inspectDocument is the JSON document `inspect --json` writes: the inspection the core produced,
// plus the carried AgentsView revision this binary was built from. The embedded struct keeps the
// core's fields at the top level, so a consumer reads `source`, `archive` and `warnings` directly.
type inspectDocument struct {
	UpstreamRevision string `json:"upstream_revision"`
	ccc.Inspection
}

// splitFlags separates options from operands wherever they appear.
//
// Go's flag package stops at the first operand, so `ccc inspect claude:<id> --json` would treat
// `--json` as an operand; an inspection is read by people and agents who write the reference first.
// A value-taking option keeps consuming the next argument, `--` ends the options, and everything
// else is an operand.
func splitFlags(args []string) (optionArgs, operands []string, err error) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--":
			return optionArgs, append(operands, args[index+1:]...), nil
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			operands = append(operands, arg)
		default:
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") || name == "h" || name == "help" {
				optionArgs = append(optionArgs, arg)
				continue
			}
			switch name {
			case "data-dir":
				if index+1 >= len(args) {
					return nil, nil, fmt.Errorf("--%s needs a directory", name)
				}
				optionArgs = append(optionArgs, arg, args[index+1])
				index++
			default:
				optionArgs = append(optionArgs, arg)
			}
		}
	}
	return optionArgs, operands, nil
}

// parseRef splits a `<provider>:<id-or-path>` reference. The provider is required: guessing one from
// an id or a path would silently pick a reader for the wrong format.
func parseRef(operand string) (ccc.Ref, error) {
	provider, id, found := strings.Cut(operand, ":")
	if !found || provider == "" || id == "" {
		return ccc.Ref{}, fmt.Errorf(
			"reference %q is not <provider>:<session-id-or-path> (for example claude:9f2c1d47-...)", operand)
	}
	if _, ok := parser.AgentByType(parser.AgentType(provider)); !ok {
		return ccc.Ref{}, fmt.Errorf("unknown provider %q", provider)
	}
	return ccc.Ref{Provider: provider, ID: id}, nil
}

func writeHuman(out io.Writer, inspection ccc.Inspection) {
	fmt.Fprintf(out, "%s:%s\n", inspection.Ref.Provider, inspection.Ref.ID)
	if inspection.Agent != "" {
		fmt.Fprintf(out, "  agent:        %s\n", inspection.Agent)
	}
	if source := inspection.Source; source != nil {
		identity := source.Identity
		fmt.Fprintf(out, "  session id:   %s\n", valueOr(identity.SessionID, "unknown"))
		fmt.Fprintf(out, "  source:       %s\n", valueOr(identity.SourcePath, identity.DisplayPath))
		if identity.Project != "" {
			fmt.Fprintf(out, "  project:      %s\n", identity.Project)
		}
		if identity.Cwd != "" {
			fmt.Fprintf(out, "  working dir:  %s\n", identity.Cwd)
		}
		if identity.GitBranch != "" {
			fmt.Fprintf(out, "  git branch:   %s\n", identity.GitBranch)
		}
		if identity.SourceVersion != "" {
			fmt.Fprintf(out, "  source version: %s\n", identity.SourceVersion)
		}
		fmt.Fprintf(out, "  conversation: %s\n", describeCounts(source.Counts))
		if len(source.ToolNames) > 0 {
			fmt.Fprintf(out, "  tools:        %s\n", strings.Join(source.ToolNames, ", "))
		}
		if !source.StartedAt.IsZero() {
			fmt.Fprintf(out, "  recorded:     %s .. %s\n",
				source.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
				source.EndedAt.UTC().Format("2006-01-02T15:04:05Z"))
		}
		if source.FirstUserMessage != "" {
			fmt.Fprintf(out, "  first prompt: %s\n", oneLine(source.FirstUserMessage))
		}
		if source.Fidelity.MalformedLines > 0 || source.Fidelity.Truncated {
			fmt.Fprintf(out, "  fidelity:     %d malformed line(s)%s\n",
				source.Fidelity.MalformedLines, truncatedSuffix(source.Fidelity.Truncated))
		}
	} else {
		fmt.Fprintf(out, "  source:       not readable here (see warnings)\n")
	}
	if archived := inspection.Archive; archived == nil {
		fmt.Fprintf(out, "  archive:      none at %s\n", valueOr(inspection.ArchivePath, "the configured data directory"))
	} else {
		fmt.Fprintf(out, "  archive:      %s [%s]\n", archived.Status.Path, archived.Projection)
		fmt.Fprintf(out, "  archived id:  %s\n", archived.Status.SessionID)
		fmt.Fprintf(out, "  archived:     %s\n", describeCounts(archived.Counts))
		if archived.SameMessagesAsSource {
			fmt.Fprintf(out, "  agreement:    every archived message matches the source read\n")
		} else {
			fmt.Fprintf(out, "  agreement:    the archive and the source read differ (see warnings)\n")
		}
	}
	for _, warning := range inspection.Warnings {
		fmt.Fprintf(out, "  warning:      %s\n", oneLine(warning))
	}
	fmt.Fprintf(out, "  carried:      AgentsView %s, unmodified (scripts/upstream-parity.py)\n", upstreamRevision)
}

func describeCounts(counts ccc.Counts) string {
	parts := []string{fmt.Sprintf("%d message(s)", counts.Messages)}
	if counts.UserMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d user", counts.UserMessages))
	}
	if counts.AssistantMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d assistant", counts.AssistantMessages))
	}
	if counts.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d tool call(s)", counts.ToolCalls))
	}
	if counts.CompactBoundaries > 0 {
		parts = append(parts, fmt.Sprintf("%d compaction boundary(ies)", counts.CompactBoundaries))
	}
	if counts.SidechainMessages > 0 {
		parts = append(parts, fmt.Sprintf("%d sidechain", counts.SidechainMessages))
	}
	return strings.Join(parts, ", ")
}

func truncatedSuffix(truncated bool) string {
	if truncated {
		return ", truncated by the writer"
	}
	return ""
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// defaultDataDirLabel is what the data directory resolves to when no override is given. It is a
// label for --help only; the directory itself comes from the inherited configuration.
func defaultDataDirLabel() string {
	cfg, err := config.LoadReadOnly()
	if err != nil || cfg.DataDir == "" {
		return "~/.agentsview"
	}
	return cfg.DataDir
}
