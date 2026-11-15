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
	"bytes"
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
	"go.kenn.io/agentsview/internal/stringutil"
)

const usage = `ccc — continue one agent session in another, through AgentsView's own reader

usage:
  ccc continue <provider>:<session-id-or-path> --to codex [options]
  ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]
  ccc help

  <provider> is the provider identifier AgentsView uses for the agent that wrote the session
  (for example claude, codex), or a stored agent label such a provider writes when it has no
  identifier of its own (freebuff, which the Codebuff provider records). The identifier after it is
  the provider's own session id, or the path to the session's source file -- absolute, or relative to
  one of that provider's configured source roots. An id may carry a host (host~<id>): that is a
  session synced from another machine, which has no source file here, so only an archive can answer
  it.

  continue reads that session through the inherited AgentsView reader, writes it as a NEW native
  session of the target harness, installs it where that harness keeps its sessions, and reports the
  target's own session id and resume command. It never changes the source session and never replaces
  an existing target session. The common invocation is:
      ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex
  and the result says which command resumes it (codex resume <session-id>).

  inspect reports the session's identity, agent, source and conversation shape. It reads the
  source session through the provider's own reader and, when an archive is present, says what the
  archive holds for the same session. It never writes: the source session and any archive are read
  only.

continuation options:
  --to NAME          target harness to write (codex)
  --json             write the result as one JSON object on stdout (diagnostics stay on stderr)
  --dry-run          convert and verify without writing anything into the target's data directory
  --target-home DIR  that harness's data directory (default: $CODEX_HOME, else ~/.codex)
  --target-binary P  that harness's executable (default: codex on PATH); the printed resume commands
                     name it the way it is given here
  --data-dir DIR     read the AgentsView archive in DIR (%s/sessions.db) when the source file itself
                     cannot answer the reference
  --verify LEVEL     none, load (default) or resume: load asks the target harness's own loader to
                     open the installed session offline; resume runs one real turn in it and needs
                     --resume-prompt
  --resume-prompt T  the turn --verify resume runs; call/ccc never writes a prompt of its own into a
                     session

inspection options:
  --json             write the inspection as one JSON object on stdout (diagnostics stay on stderr)
  --data-dir DIR     read the archive in DIR (%s/sessions.db) instead of the configured data directory

exit codes:
  0  the session was found and reported, the conversion was written and the verification that was
     asked for either passed or could not run: the target harness is not installed, or a dry run
     installed nothing for it to load
  1  nothing was found, the reference was ambiguous, the conversion or installation failed, or a
     requested verification ran and failed
  2  the command line could not be understood
`

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// inspectUsage is the one-line usage `ccc inspect` prints for `-h`, for a usage error and for a
// missing operand: the flag package's help request and a malformed command line get the same text,
// and only the stream and the exit code tell them apart.
const inspectUsage = "usage: ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, usage, defaultDataDirLabel(), defaultDataDirLabel())
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintf(stdout, usage, defaultDataDirLabel(), defaultDataDirLabel())
		return exitOK
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "continue":
		return runContinue(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ccc: unknown command %q\n\n", args[0])
		fmt.Fprintf(stderr, usage, defaultDataDirLabel(), defaultDataDirLabel())
		return exitUsage
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "write the inspection as JSON")
	dataDir := flags.String("data-dir", "", "data directory whose sessions.db is consulted")
	operands, code, handled := parseCommandLine(flags, inspectUsage, args, stdout, stderr)
	if handled {
		return code
	}
	if len(operands) != 1 {
		fmt.Fprint(stderr, inspectUsage)
		return exitUsage
	}
	ref, err := parseRef(operands[0])
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitUsage
	}

	cfg, err := readConfiguration(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitFailure
	}

	reader := ccc.NewSourceReader(cfg)
	inspection, err := ccc.Inspect(context.Background(), ccc.InspectOptions{
		Ref:         ref,
		Config:      cfg,
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
		// Neither side answered the reference. The inspection's warnings say why -- a provider that
		// failed operationally, a session recorded on another machine, an archive that is not there --
		// and dropping them would report a provider failure as an absent session. They go to stderr
		// with the failure they explain; stdout carries a result, and there is none.
		for _, warning := range inspection.Warnings {
			fmt.Fprintf(stderr, "ccc: %s\n", oneLine(warning))
		}
		fmt.Fprintf(stderr, "ccc: no session found for %s:%s (providers: %s)\n",
			ref.Provider, ref.ID, strings.Join(reader.Providers(), ", "))
		return exitFailure
	}

	if *asJSON {
		if !writeJSONDocument(stdout, stderr, inspectDocument{
			UpstreamRevision: upstreamRevision,
			Inspection:       inspection,
		}, "inspection") {
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

// parseCommandLine fills a command's flag set from args and answers its operands.
//
// The flag package writes both its errors and a requested help text to one stream. They are not the
// same thing to a caller -- a usage request is not a command line that could not be understood -- so
// its output is buffered here and routed once the parse has answered. handled is false when the command
// line was answered while parsing (the help text printed to stdout for a request, or the flag error to
// stderr) and the caller returns exitCode; otherwise the operands are splitFlags' answer, and the flag
// set holds what was parsed.
func parseCommandLine(
	flags *flag.FlagSet, usage string, args []string, stdout, stderr io.Writer,
) (operands []string, exitCode int, handled bool) {
	var flagOutput bytes.Buffer
	flags.SetOutput(&flagOutput)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), usage)
	}
	flagArgs, operands, err := splitFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return nil, exitUsage, true
	}
	if err := flags.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, flagOutput.String())
			return nil, exitOK, true
		}
		fmt.Fprint(stderr, flagOutput.String())
		return nil, exitUsage, true
	}
	return operands, exitOK, false
}

// readConfiguration answers the inherited configuration, with the command's --data-dir override
// applied: an override points both the data directory and the archive path at it.
func readConfiguration(dataDir string) (config.Config, error) {
	cfg, err := config.LoadReadOnly()
	if err != nil {
		return config.Config{}, fmt.Errorf("reading the AgentsView configuration: %w", err)
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
		cfg.DBPath = filepath.Join(dataDir, "sessions.db")
	}
	return cfg, nil
}

// writeJSONDocument writes one command's result: one indented JSON object on stdout, with a write
// failure reported on stderr, because the result that failed to write is not a result.
func writeJSONDocument(stdout, stderr io.Writer, document any, what string) bool {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		fmt.Fprintf(stderr, "ccc: writing the %s: %v\n", what, err)
		return false
	}
	return true
}

// writeCarried is the footer both commands' human output ends with: the AgentsView revision this binary
// carries unmodified.
func writeCarried(out io.Writer) {
	fmt.Fprintf(out, "  carried:      AgentsView %s, unmodified (scripts/upstream-parity.py)\n", upstreamRevision)
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
			if takesValue(name) {
				if index+1 >= len(args) {
					return nil, nil, fmt.Errorf("--%s needs a value", name)
				}
				optionArgs = append(optionArgs, arg, args[index+1])
				index++
				continue
			}
			optionArgs = append(optionArgs, arg)
		}
	}
	return optionArgs, operands, nil
}

// takesValue answers which options consume the following argument. The list is explicit so that a
// misspelled option is refused by the flag set instead of swallowing the reference.
func takesValue(name string) bool {
	switch name {
	case "data-dir", "to", "target-home", "target-binary", "verify", "resume-prompt":
		return true
	default:
		return false
	}
}

// parseRef splits a `<provider>:<id-or-path>` reference. The provider is required: guessing one from
// an id or a path would silently pick a reader for the wrong format. What counts as a provider name is
// the core's answer (ccc.KnownProvider), not a registry lookup here: a stored agent label such as
// `freebuff` -- the identity the Codebuff provider records and the archive reports for its free-tier
// sessions -- is one a caller may write, and refusing it here would leave the CLI unable to name the
// session an archive hands back.
func parseRef(operand string) (ccc.Ref, error) {
	provider, id, found := strings.Cut(operand, ":")
	if !found || provider == "" || id == "" {
		return ccc.Ref{}, fmt.Errorf(
			"reference %q is not <provider>:<session-id-or-path> (for example claude:9f2c1d47-...)", operand)
	}
	if !ccc.KnownProvider(provider) {
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
		fmt.Fprintf(out, "  archive:      %s [%s]\n", archived.Status.Path, archived.ContentShown)
		fmt.Fprintf(out, "  archived id:  %s\n", archived.Status.SessionID)
		fmt.Fprintf(out, "  archived:     %s\n", describeCounts(archived.Counts))
		switch {
		case archived.SameMessagesAsSource == nil:
			fmt.Fprintf(out, "  agreement:    not compared (the source was not readable)\n")
		case *archived.SameMessagesAsSource:
			fmt.Fprintf(out, "  agreement:    every archived message matches the source read\n")
		default:
			fmt.Fprintf(out, "  agreement:    the archive and the source read differ (see warnings)\n")
		}
	}
	for _, warning := range inspection.Warnings {
		fmt.Fprintf(out, "  warning:      %s\n", oneLine(warning))
	}
	writeCarried(out)
}

// continueUsage is the one-line usage `ccc continue` prints for `-h`, for a usage error and for a
// missing operand.
const continueUsage = "usage: ccc continue <provider>:<session-id-or-path> --to codex [--json] [--dry-run]\n" +
	"                         [--target-home DIR] [--target-binary PATH] [--data-dir DIR]\n" +
	"                         [--verify none|load|resume] [--resume-prompt TEXT]\n"

func runContinue(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("continue", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "write the result as JSON")
	dryRun := flags.Bool("dry-run", false, "convert without installing")
	target := flags.String("to", "", "target harness to write")
	targetHome := flags.String("target-home", "", "the target harness's data directory")
	targetBinary := flags.String("target-binary", "", "the target harness's executable")
	dataDir := flags.String("data-dir", "", "data directory whose sessions.db is consulted")
	verify := flags.String("verify", ccc.VerifyLoad, "verification level: none, load or resume")
	resumePrompt := flags.String("resume-prompt", "", "the turn a resume verification runs")
	operands, code, handled := parseCommandLine(flags, continueUsage, args, stdout, stderr)
	if handled {
		return code
	}
	if len(operands) != 1 {
		fmt.Fprint(stderr, continueUsage)
		return exitUsage
	}
	if strings.TrimSpace(*target) == "" {
		fmt.Fprint(stderr, continueUsage)
		fmt.Fprintf(stderr, "ccc: --to names the target harness (codex)\n")
		return exitUsage
	}
	if !strings.EqualFold(strings.TrimSpace(*target), ccc.TargetCodex) {
		fmt.Fprintf(stderr, "ccc: this build writes %s, not %q\n", ccc.TargetCodex, *target)
		return exitUsage
	}
	// The core owns the wording of a refused verification request, so a caller driving the CLI and one
	// embedding the core get the same sentence for the same mistake; the level itself is passed on as
	// it was written (the core normalizes it when it resolves the level).
	if err := ccc.CheckVerifyOptions(*verify, *resumePrompt); err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitUsage
	}
	ref, err := parseRef(operands[0])
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitUsage
	}

	cfg, err := readConfiguration(*dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitFailure
	}

	result, err := ccc.Continue(context.Background(), ccc.ContinueOptions{
		Ref:          ref,
		Target:       *target,
		Config:       cfg,
		Reader:       ccc.NewSourceReader(cfg),
		ArchivePath:  cfg.DBPath,
		TargetHome:   *targetHome,
		TargetBinary: *targetBinary,
		DryRun:       *dryRun,
		Verify:       *verify,
		ResumePrompt: *resumePrompt,
		Stderr:       stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ccc: %v\n", err)
		return exitFailure
	}

	if *asJSON {
		if !writeJSONDocument(stdout, stderr, continueDocument{
			UpstreamRevision: upstreamRevision,
			ContinueResult:   result,
		}, "result") {
			return exitFailure
		}
	} else {
		writeContinuation(stdout, result)
	}

	if code := continuationExit(result); code != exitOK {
		for _, warning := range result.Warnings {
			fmt.Fprintf(stderr, "ccc: %s\n", oneLine(warning))
		}
		return code
	}
	return exitOK
}

// continuationExit decides the run's exit code from the three states. A verification that could not run
// because the target harness is not installed — or because a dry run installed nothing — is reported,
// not failed: the conversation really was converted. A verification the caller asked for that ran and
// did not pass is a failure, because the caller asked precisely that question.
func continuationExit(result ccc.ContinueResult) int {
	if result.Conversion.State != ccc.ConversionStateConverted ||
		result.Verification.Structure.State != ccc.VerificationStateVerified {
		return exitFailure
	}
	if result.Conversion.DryRun {
		// A dry run installs nothing, so the levels that could not run are reported not verified with
		// the dry-run reason; the conversion and its structural check are what this run asked for.
		return exitOK
	}
	if result.Installation.State != ccc.InstallationStateInstalled {
		return exitFailure
	}
	if !result.TargetSession.BinaryFound {
		return exitOK
	}
	if result.Verification.Load.State == ccc.VerificationStateNotVerified {
		return exitFailure
	}
	if result.Verification.Resume.State == ccc.VerificationStateNotVerified {
		return exitFailure
	}
	return exitOK
}

// continueDocument is the JSON document `ccc continue --json` writes.
type continueDocument struct {
	UpstreamRevision string `json:"upstream_revision"`
	ccc.ContinueResult
}

func writeContinuation(out io.Writer, result ccc.ContinueResult) {
	fmt.Fprintf(out, "%s:%s → %s\n", result.Ref.Provider, result.Ref.ID, result.Target)
	if source := result.Source; source != nil {
		fmt.Fprintf(out, "  source:       %s\n", valueOr(source.Identity.SourcePath, source.Identity.DisplayPath))
		fmt.Fprintf(out, "  read from:    %s\n", result.Conversion.ReadFrom)
		fmt.Fprintf(out, "  conversation: %s\n", describeCounts(source.Counts))
	}
	carried := result.Conversion.Carried
	fmt.Fprintf(out, "  converted:    %s, %d bytes: %d message(s), %d tool call(s), %d tool result(s), %d boundary(ies), %d turn(s)\n",
		result.Conversion.Format, carried.Bytes, carried.Messages, carried.ToolCalls, carried.ToolResults,
		carried.CompactionBoundaries, carried.Turns)
	for _, loss := range result.Conversion.Losses {
		if loss.Count == 0 {
			continue
		}
		fmt.Fprintf(out, "  %-13s %s ×%d\n", loss.Disposition+":", loss.Category, loss.Count)
	}
	if result.Conversion.DryRun {
		fmt.Fprintf(out, "  installation: dry run, nothing written to %s\n", result.TargetSession.Home)
	} else {
		fmt.Fprintf(out, "  installed:    %s [%s]\n", result.Installation.Path, result.Installation.Mechanism)
	}
	fmt.Fprintf(out, "  source file:  %s\n", sourceCheckLine(result.Installation))
	fmt.Fprintf(out, "  target id:    %s\n", result.TargetSession.SessionID)
	if result.TargetSession.Cwd != "" {
		fmt.Fprintf(out, "  working dir:  %s\n", result.TargetSession.Cwd)
	}
	fmt.Fprintf(out, "  resume:       %s\n", result.TargetSession.ResumeCommand)
	fmt.Fprintf(out, "  non-interactive resume: %s\n", result.TargetSession.ExecResumeCommand)
	fmt.Fprintf(out, "  cli_version:  %s (%s)\n", result.TargetSession.CLIVersion, result.TargetSession.CLIVersionSource)
	fmt.Fprintf(out, "  structure:    %s (%s)\n", result.Verification.Structure.State, result.Verification.Structure.Method)
	fmt.Fprintf(out, "  native load:  %s (%s)\n", result.Verification.Load.State, result.Verification.Load.Method)
	if detail := result.Verification.Load.Detail; detail != "" {
		fmt.Fprintf(out, "                %s\n", oneLine(detail))
	}
	fmt.Fprintf(out, "  native resume: %s (%s)\n", result.Verification.Resume.State, result.Verification.Resume.Method)
	if detail := result.Verification.Resume.Detail; detail != "" {
		fmt.Fprintf(out, "                %s\n", oneLine(detail))
	}
	if answer := result.Verification.Resume.Output; answer != "" {
		// The turn's own words, bounded: it is the observable proof that the target continued from the
		// converted history rather than merely loading it.
		fmt.Fprintf(out, "  answer:       %s\n", oneLine(stringutil.SafeTruncate(answer, 400)))
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "  warning:      %s\n", oneLine(warning))
	}
	writeCarried(out)
}

// sourceCheckLine is the human spelling of the installation's source check: the goal asks the run to
// show that the transcript the conversation came from is its same file afterwards, so the boolean and
// the detail that says how it was compared are printed rather than left to the JSON.
func sourceCheckLine(installation ccc.InstallationReport) string {
	state := "unchanged"
	if !installation.SourceUnchanged {
		state = "changed while it was converted"
	}
	if detail := oneLine(installation.Detail); detail != "" {
		return state + " (" + detail + ")"
	}
	return state
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
