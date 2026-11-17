package ccc

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
)

// A target harness this build can write. Every other name is refused: a target that has no writer would
// otherwise be answered with a plausible-looking file.
const (
	TargetCodex  = "codex"
	TargetClaude = "claude"
)

// Targets names every target harness this build writes, in the order a caller lists them.
func Targets() []string {
	return []string{TargetCodex, TargetClaude}
}

// harnessProbe is what a target harness's own binary answered about itself: where it is, whether it is
// installed at all, the version it reported and, when it could not answer, why. One type answers for
// every target, because every target's probe asks the harness the same question.
type harnessProbe struct {
	Binary  string
	Found   bool
	Version string
	Detail  string
}

// convertedTarget is one target document, in memory and ready to install: the fresh identity, the bytes
// the target harness reads, and the report of what the conversion carried. Raw is the target's own
// document, which only its route's own steps read.
type convertedTarget struct {
	// ID is the fresh native identity, FileName the name it takes and Directory the place inside the
	// target's session root it is filed under.
	ID        string
	FileName  string
	Directory string
	// Cwd is the working directory the session records, and StartedAt the instant it starts at.
	Cwd       string
	StartedAt time.Time
	// Data is the exact byte string the target harness reads.
	Data    []byte
	Carried Carried
	Losses  []Loss
	Raw     any
}

// targetFactsInput is what a target's own harness is asked with. It is answered after the source read,
// because what a target's loader checks can depend on the session's own working directory.
type targetFactsInput struct {
	Home   string
	DryRun bool
	Probe  harnessProbe
	// SourceCwd is the working directory the source session recorded.
	SourceCwd string
	Timeout   time.Duration
	Stderr    io.Writer
}

// targetFacts is what only the target harness could answer, in the report's own vocabulary: the version
// its records are written as, and (for a target whose loader checks one) the provider binding. A route's
// Fields answers the same value for the session it wrote, with the document's own format version in it,
// so what the report states about a session is never re-typed out of the facts that produced it.
type targetFacts struct {
	CLIVersion       string
	CLIVersionSource string
	ModelProvider    string
	ProviderSource   string
	ProviderDetail   string
	Model            string
}

// targetConvertInput is what one target's writer is configured with.
type targetConvertInput struct {
	CLIVersion   string
	Provider     string
	Now          time.Time
	MaxToolNames int
}

// targetInvocationInput names the harness the printed resume commands must name, and carries the probe
// that answered about it.
type targetInvocationInput struct {
	Binary string
	// Probe is the target probe's own answer about Binary. Found says it located the executable, and
	// Binary is then where it found it -- the program the conversion asked and the native checks ran. A
	// route whose invocation changes its working directory names that path rather than a spelling a
	// shell would search PATH for again from the new directory (invocationBinary).
	Probe harnessProbe
	Home  string
}

// targetInvocation is how one installed session is resumed: the argument vector, the environment that
// selects the data directory, and the working directory the invocation has to run from (empty when the
// target does not care).
type targetInvocation struct {
	Arguments     []string
	ExecArguments []string
	Environment   map[string]string
	WorkingDir    string
}

// targetVerificationInput is everything a native verification of one target session is given.
type targetVerificationInput struct {
	Home      string
	Binary    string
	SessionID string
	Cwd       string
	Path      string
	Directory string
	FileName  string
	Data      []byte
	Prompt    string
	Timeout   time.Duration
	Stderr    io.Writer
}

// targetRoute is one target harness's half of a continuation: what its data directory is called, what its
// format can express, and how one conversion of it is written, installed and verified. Continue itself is
// target-agnostic -- everything a target answers differently lives here -- so a new target is one value
// rather than a second copy of the pipeline.
type targetRoute struct {
	Name   string
	Format string
	// HomeEnv is the environment variable that selects the target's data directory, which its printed
	// resume commands set: none of the supported harnesses has a flag for it.
	HomeEnv string
	// BinaryDefault is the executable name the printed resume commands use when the caller named none.
	BinaryDefault string
	// PinVersion is the format version written when the installed binary cannot be probed.
	PinVersion   string
	FormatLimits []FormatLimit

	ResolveHome func(explicit string) (string, bool, error)
	Probe       func(ctx context.Context, binary string) harnessProbe
	// Facts asks the target's own harness what only it can answer. It is not called for a dry run or for
	// a target whose format records no such thing.
	Facts func(ctx context.Context, in targetFactsInput) targetFacts
	// DryRunWarnings are what a dry run has to say about the fields it did not fill in because the harness
	// was never started.
	DryRunWarnings func(facts targetFacts, probe harnessProbe) []string

	Convert func(conv Conversation, in targetConvertInput) (convertedTarget, error)
	Install func(session convertedTarget, home string) (string, string, error)
	// StructureRoot is the directory inside a data directory that holds the target's sessions.
	StructureRoot func(home string) string
	ReadBack      func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error)
	Load          func(ctx context.Context, in targetVerificationInput) LoadVerification
	Resume        func(ctx context.Context, in targetVerificationInput) ResumeVerification
	Invocation    func(session convertedTarget, in targetInvocationInput) targetInvocation
	Fields        func(session convertedTarget, facts targetFacts) targetFacts

	// What each check reads, named once per target, so a check that did not run still reports the
	// mechanism it would have used.
	StructureMethod       string
	StructureMethodDryRun string
	LoadMethod            string
	ResumeMethod          string
	// StructureReader names the inherited reader the structural check read the file back through, in the
	// words the report's warnings use.
	StructureReader string
}

// targetRouteFor answers the route for a target name, or ErrUnsupportedTarget naming what this build
// writes.
func targetRouteFor(target string) (targetRoute, error) {
	switch target {
	case TargetCodex:
		return codexRoute(), nil
	case TargetClaude:
		return claudeRoute(), nil
	default:
		return targetRoute{}, fmt.Errorf("%w: %q (this build writes %s)",
			ErrUnsupportedTarget, target, strings.Join(Targets(), ", "))
	}
}

// codexRoute is the route increment 0002 established: a Codex rollout, installed into the target's
// sessions tree, verified through Codex's own app-server loader and a real resume turn.
func codexRoute() targetRoute {
	return targetRoute{
		Name:          TargetCodex,
		Format:        CodexTargetFormat,
		HomeEnv:       codexHomeEnv,
		BinaryDefault: CodexBinaryDefault,
		PinVersion:    CodexFormatVersion,
		FormatLimits:  codexFormatLimits(),
		ResolveHome:   ResolveCodexHome,
		Probe:         ProbeCodexBinary,
		Facts:         codexFacts,
		DryRunWarnings: func(facts targetFacts, probe harnessProbe) []string {
			if !probe.Found {
				return nil
			}
			return []string{"a dry run does not start the target harness, so the conversion records no " +
				"model provider; a run that installs asks the harness for its own answer"}
		},
		Convert: func(conv Conversation, in targetConvertInput) (convertedTarget, error) {
			rollout, err := ConvertToCodexRollout(conv, CodexTargetOptions{
				CLIVersion:   in.CLIVersion,
				Provider:     in.Provider,
				Now:          in.Now,
				MaxToolNames: in.MaxToolNames,
			})
			if err != nil {
				return convertedTarget{}, err
			}
			return convertedTarget{
				ID:        rollout.SessionID,
				FileName:  rollout.FileName,
				Directory: rollout.Directory,
				Cwd:       rollout.Cwd,
				StartedAt: rollout.StartedAt,
				Data:      rollout.Data,
				Carried:   rollout.Carried,
				Losses:    rollout.Losses,
				Raw:       rollout,
			}, nil
		},
		Install: func(session convertedTarget, home string) (string, string, error) {
			return InstallCodexRollout(session.Raw.(CodexRollout), home)
		},
		StructureRoot: func(home string) string { return filepath.Join(home, codexSessionsSubdir) },
		ReadBack: func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error) {
			return VerifyCodexRolloutStructure(ctx, root, session.Raw.(CodexRollout), cfg)
		},
		Load: func(ctx context.Context, in targetVerificationInput) LoadVerification {
			return VerifyCodexLoad(ctx, CodexLoadOptions{
				Home:         in.Home,
				Binary:       in.Binary,
				SessionID:    in.SessionID,
				ExpectedPath: in.Path,
				Timeout:      in.Timeout,
				Stderr:       in.Stderr,
			})
		},
		Resume: func(ctx context.Context, in targetVerificationInput) ResumeVerification {
			return VerifyCodexResume(ctx, CodexResumeOptions{
				Home:        in.Home,
				Binary:      in.Binary,
				SessionID:   in.SessionID,
				Prompt:      in.Prompt,
				WorkingDir:  in.Cwd,
				SessionPath: in.Path,
				Timeout:     in.Timeout,
				Stderr:      in.Stderr,
			})
		},
		Invocation: func(session convertedTarget, in targetInvocationInput) targetInvocation {
			return targetInvocation{
				Arguments:     []string{in.Binary, "resume", session.ID},
				ExecArguments: []string{in.Binary, "exec", "resume", session.ID, resumePromptPlaceholder},
				Environment:   map[string]string{codexHomeEnv: in.Home},
			}
		},
		Fields: func(session convertedTarget, facts targetFacts) targetFacts {
			// The document's own format version is the version its records are written as; the rest of
			// the facts are the harness's own answers, reported as they came.
			facts.CLIVersion = session.Raw.(CodexRollout).CLIVersion
			return facts
		},
		StructureMethod:       structureMethodCodex,
		StructureMethodDryRun: structureMethodCodexDryRun,
		LoadMethod:            loadMethodCodex,
		ResumeMethod:          resumeMethodCodex,
		StructureReader:       structureReaderCodex,
	}
}

// codexFacts asks Codex itself which model provider a new session of it would use: its loader refuses a
// session whose recorded provider (or lack of one) does not match the configuration it is resumed with,
// and a home that has already migrated to paginated threads refuses one that records none at all.
func codexFacts(ctx context.Context, in targetFactsInput) targetFacts {
	facts := targetFacts{CLIVersionSource: "writer-pin"}
	if in.Probe.Version != "" {
		facts.CLIVersion = in.Probe.Version
		facts.CLIVersionSource = "probe"
	}
	probe := CodexConfigProbe{}
	switch {
	case in.DryRun:
		probe = CodexConfigProbe{Detail: "a dry run does not start the target harness against its data " +
			"directory, so which model provider a new session of it would use was not asked and none is " +
			"recorded; an installed run records the harness's own answer"}
	case in.Probe.Found:
		probe = ProbeCodexConfig(ctx, in.Home, in.Probe.Binary, in.SourceCwd, in.Timeout, in.Stderr)
	}
	facts.ModelProvider = probe.Provider
	facts.ProviderDetail = probe.Detail
	facts.ProviderSource = "unrecorded"
	if probe.Found && strings.TrimSpace(probe.Provider) != "" {
		facts.ProviderSource = probe.Source
	}
	facts.Model = probe.Model
	return facts
}

// claudeRoute is the route increment 0003 adds: a Claude Code transcript, filed under the target's
// projects directory for the session's working directory, verified through Claude Code's own loader (no
// model turn) and one real turn through the reported invocation.
func claudeRoute() targetRoute {
	return targetRoute{
		Name:          TargetClaude,
		Format:        ClaudeTargetFormat,
		HomeEnv:       claudeHomeEnv,
		BinaryDefault: ClaudeBinaryDefault,
		PinVersion:    ClaudeFormatVersion,
		FormatLimits:  claudeFormatLimits(),
		ResolveHome:   ResolveClaudeHome,
		Probe:         ProbeClaudeBinary,
		Facts:         claudeFacts,
		// The Claude transcript records no provider, so there is nothing a dry run left unfilled: a
		// resumed turn uses whatever configuration the caller runs it with.
		DryRunWarnings: func(targetFacts, harnessProbe) []string { return nil },
		Convert: func(conv Conversation, in targetConvertInput) (convertedTarget, error) {
			transcript, err := ConvertToClaudeTranscript(conv, ClaudeTargetOptions{
				CLIVersion:   in.CLIVersion,
				Now:          in.Now,
				MaxToolNames: in.MaxToolNames,
			})
			if err != nil {
				return convertedTarget{}, err
			}
			return convertedTarget{
				ID:        transcript.SessionID,
				FileName:  transcript.FileName,
				Directory: transcript.Directory,
				Cwd:       transcript.Cwd,
				StartedAt: transcript.StartedAt,
				Data:      transcript.Data,
				Carried:   transcript.Carried,
				Losses:    transcript.Losses,
				Raw:       transcript,
			}, nil
		},
		Install: func(session convertedTarget, home string) (string, string, error) {
			return InstallClaudeTranscript(session.Raw.(ClaudeTranscript), home)
		},
		StructureRoot: func(home string) string { return filepath.Join(home, claudeProjectsSubdir) },
		ReadBack: func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error) {
			return VerifyClaudeTranscriptStructure(ctx, root, session.Raw.(ClaudeTranscript), cfg)
		},
		Load: func(ctx context.Context, in targetVerificationInput) LoadVerification {
			return VerifyClaudeLoad(ctx, ClaudeLoadOptions{
				Binary:    in.Binary,
				SessionID: in.SessionID,
				Cwd:       in.Cwd,
				Directory: in.Directory,
				FileName:  in.FileName,
				Data:      in.Data,
				Installed: in.Path,
				Timeout:   in.Timeout,
				Stderr:    in.Stderr,
			})
		},
		Resume: func(ctx context.Context, in targetVerificationInput) ResumeVerification {
			return VerifyClaudeResume(ctx, ClaudeResumeOptions{
				Home:        in.Home,
				Binary:      in.Binary,
				SessionID:   in.SessionID,
				Cwd:         in.Cwd,
				SessionPath: in.Path,
				Prompt:      in.Prompt,
				Timeout:     in.Timeout,
				Stderr:      in.Stderr,
			})
		},
		Invocation: func(session convertedTarget, in targetInvocationInput) targetInvocation {
			// This invocation changes into the session's working directory, so its words are the program
			// this run probed and verified: the probe's own answer when it found one (already the path the
			// native checks resolved), and otherwise the caller's spelling resolved against the caller's
			// directory (invocationBinary). A bare name re-searched by a shell from the session's
			// directory can find a different program there, which is what naming the probed path closes.
			binary := invocationBinary(in.Binary, in.Probe)
			return targetInvocation{
				Arguments: []string{binary, "--resume", session.ID},
				// The `--` is the terminator this route's own turn runs behind (claude_verify.go): without
				// it a prompt whose own first word is an option (`--help`) is read as one by the harness,
				// so the printed command would run something other than the turn verification ran.
				ExecArguments: []string{binary, "-p", "--output-format", "json", "--resume", session.ID, promptTerminator, resumePromptPlaceholder},
				Environment:   map[string]string{claudeHomeEnv: in.Home},
				// Measured: the loader finds a session through the working directory it is run from, so
				// the invocation has to run from the session's own (the printed command says so).
				WorkingDir: session.Cwd,
			}
		},
		Fields: func(session convertedTarget, facts targetFacts) targetFacts {
			// The Claude transcript records no provider, so claudeFacts answered none: the report states
			// the document's own format version beside those empty fields.
			facts.CLIVersion = session.Raw.(ClaudeTranscript).CLIVersion
			return facts
		},
		StructureMethod:       structureMethodClaude,
		StructureMethodDryRun: structureMethodClaudeDryRun,
		LoadMethod:            loadMethodClaude,
		ResumeMethod:          resumeMethodClaude,
		StructureReader:       structureReaderClaude,
	}
}

// claudeFacts records which version the transcript's records are written as: the probed one when the
// installed binary answered, else the pinned version this writer's format was verified against. The
// Claude Code transcript records no provider, so nothing else is asked of the harness here.
func claudeFacts(_ context.Context, in targetFactsInput) targetFacts {
	facts := targetFacts{CLIVersion: ClaudeFormatVersion, CLIVersionSource: "writer-pin"}
	if in.Probe.Version != "" {
		facts.CLIVersion = in.Probe.Version
		facts.CLIVersionSource = "probe"
	}
	return facts
}

// invocationBinary answers the word an invocation's own argument vector carries for the target binary.
//
// The probe's own answer wins whenever it found one: that path is the program this run asked, and the
// one the native checks resolved and ran, so a printed command that changes its working directory names
// that program rather than a word a shell searches PATH for again from the new directory -- a search
// that can find a different program there. When the probe found nothing there is nothing better to name
// than the caller's spelling: a spelling that names a path is resolved against the caller's working
// directory (the directory change would otherwise make `--target-binary tools/claude` name a path beside
// the session), and a name with no separator is left exactly as the caller wrote it.
//
// The resolution is textual (filepath.Abs), so it also answers for a spelling that names nothing at all;
// whether a program is there is the probe's own answer beside it. A bare name the probe could not
// resolve -- Go's own lookup refuses a path found through a relative PATH entry rather than answering
// one silently -- stays the word the caller wrote, which is what keeps the printed line honest about
// having verified nothing.
func invocationBinary(binary string, probe harnessProbe) string {
	if probe.Found && probe.Binary != "" {
		binary = probe.Binary
	}
	if binary == "" || filepath.IsAbs(binary) {
		return binary
	}
	// "/" is a separator on every platform this build runs on, and filepath.Separator is this platform's
	// own: a name holding neither is a PATH lookup, and a backslash on a platform where it is an ordinary
	// filename character is not read as a path.
	if !strings.ContainsAny(binary, "/"+string(filepath.Separator)) {
		return binary
	}
	absolute, err := filepath.Abs(binary)
	if err != nil {
		return binary
	}
	return absolute
}
