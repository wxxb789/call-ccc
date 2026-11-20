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
	TargetCodex    = "codex"
	TargetClaude   = "claude"
	TargetPi       = "pi"
	TargetOMP      = "omp"
	TargetOpenCode = "opencode"
)

// Targets names every target harness this build writes, in the order a caller lists them.
func Targets() []string {
	return []string{TargetCodex, TargetClaude, TargetPi, TargetOMP, TargetOpenCode}
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
	// RecordFates is the per-source-record accounting of a document a selection produced (D1), filled by
	// a writer whose route keeps one. Empty for every other document and route, which is what leaves an
	// unflagged run's report as it was.
	RecordFates []RecordFate
	Raw         any
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
	// Agent is the target home's own primary agent id, for a format whose records must name one. It is
	// empty for every target whose records carry no agent, and for one whose harness was not asked.
	Agent string
	// ModelVariant is the variant the harness's own model answer carried, where its reference has one
	// (OpenCode's does): empty for every other target and for an answer without one.
	ModelVariant string
}

// targetConvertInput is what one target's writer is configured with: the version the document is written
// as, and the endpoints the target's own harness answered for itself -- Provider is the provider label a
// new session of it would run under (a Codex `model_provider` key, OMP's configured provider), and Model
// is the model id that answer names, for a format that records one.
type targetConvertInput struct {
	CLIVersion string
	Provider   string
	Model      string
	// Agent is the target home's own primary agent id where its harness was asked for one (a format
	// whose assistant records must name an agent), empty otherwise.
	Agent string
	// ModelVariant is the variant the harness's own model answer carried, where its model reference has
	// one; empty otherwise.
	ModelVariant string
	// ProviderDetail is what the target's own harness answered about that endpoint (or why it was not
	// asked), so a writer whose format records the binding can state the answer rather than leaving its
	// presence or absence to be read off a count.
	ProviderDetail string
	Now            time.Time
	MaxToolNames   int
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

// targetInstallInput is everything one target's installation is given: the directory to install into, the
// executable the installation runs, and the deadline and debug channel an installation that is the
// harness's own command needs. Every target's Install receives all of it, and the routes that publish a
// file of their own read only Home.
type targetInstallInput struct {
	Home   string
	Binary string
	// Timeout bounds the harness's own command, where the installation runs one.
	Timeout time.Duration
	Stderr  io.Writer
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
	// DryRunWarnings are what a dry run has to say about the fields it did not fill in because the
	// harness's own answer was not asked.
	DryRunWarnings func(facts targetFacts, probe harnessProbe) []string

	Convert func(conv Conversation, in targetConvertInput) (convertedTarget, error)
	Install func(ctx context.Context, session convertedTarget, in targetInstallInput) (string, string, error)
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
	case TargetPi:
		return piRoute(), nil
	case TargetOMP:
		return ompRoute(), nil
	case TargetOpenCode:
		return openCodeRoute(), nil
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
				ID:          rollout.SessionID,
				FileName:    rollout.FileName,
				Directory:   rollout.Directory,
				Cwd:         rollout.Cwd,
				StartedAt:   rollout.StartedAt,
				Data:        rollout.Data,
				Carried:     rollout.Carried,
				Losses:      rollout.Losses,
				RecordFates: rollout.RecordFates,
				Raw:         rollout,
			}, nil
		},
		Install: func(_ context.Context, session convertedTarget, in targetInstallInput) (string, string, error) {
			return InstallCodexRollout(session.Raw.(CodexRollout), in.Home)
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
		Install: func(_ context.Context, session convertedTarget, in targetInstallInput) (string, string, error) {
			return InstallClaudeTranscript(session.Raw.(ClaudeTranscript), in.Home)
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

// piRoute is the route increment 0005 adds: a Pi session JSONL file, installed under the project
// directory Pi's own discovery derives from the session's working directory, verified through Pi's own
// RPC loader (no model turn) and one real turn through the reported invocation.
func piRoute() targetRoute {
	return targetRoute{
		Name:          TargetPi,
		Format:        PiTargetFormat,
		HomeEnv:       piHomeEnv,
		BinaryDefault: PiBinaryDefault,
		PinVersion:    PiFormatVersion,
		FormatLimits:  piFormatLimits(piFamilyShape{}),
		ResolveHome:   ResolvePiHome,
		Probe:         ProbePiBinary,
		Facts:         piFacts,
		// A Pi session binds no model provider of its own: the writer records the source's per-message
		// model where the source recorded one, and a dry run leaves nothing unfilled that the harness
		// would otherwise have answered.
		DryRunWarnings: func(targetFacts, harnessProbe) []string { return nil },
		Convert: func(conv Conversation, in targetConvertInput) (convertedTarget, error) {
			session, err := ConvertToPiSession(conv, PiTargetOptions{
				CLIVersion:   in.CLIVersion,
				Now:          in.Now,
				MaxToolNames: in.MaxToolNames,
			})
			if err != nil {
				return convertedTarget{}, err
			}
			return convertedTarget{
				ID:        session.SessionID,
				FileName:  session.FileName,
				Directory: session.Directory,
				Cwd:       session.Cwd,
				StartedAt: session.StartedAt,
				Data:      session.Data,
				Carried:   session.Carried,
				Losses:    session.Losses,
				Raw:       session,
			}, nil
		},
		Install: func(_ context.Context, session convertedTarget, in targetInstallInput) (string, string, error) {
			return InstallPiSession(session.Raw.(PiSession), in.Home)
		},
		StructureRoot: func(home string) string { return filepath.Join(home, piSessionsSubdir) },
		ReadBack: func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error) {
			return VerifyPiSessionStructure(ctx, root, session.Raw.(PiSession), cfg)
		},
		Load: func(ctx context.Context, in targetVerificationInput) LoadVerification {
			return VerifyPiLoad(ctx, PiLoadOptions{
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
			// The turn is the invocation's own words, run through the same environment and working
			// directory the printed line names, so the check exercises what the report prints.
			return VerifyPiResume(ctx, PiResumeOptions{
				Home:        in.Home,
				Binary:      in.Binary,
				SessionID:   in.SessionID,
				ProjectDir:  in.Directory,
				Cwd:         in.Cwd,
				SessionPath: in.Path,
				Prompt:      in.Prompt,
				Timeout:     in.Timeout,
				Stderr:      in.Stderr,
			})
		},
		Invocation: func(session convertedTarget, in targetInvocationInput) targetInvocation {
			// This invocation changes into the session's working directory, so its words are the program
			// this run probed and verified (invocationBinary): a bare name re-searched by a shell from
			// there can find a different program.
			binary := invocationBinary(in.Binary, in.Probe)
			// The interactive line names the session and nothing else; the exec form is piResumeArguments,
			// the invocation VerifyPiResume runs -- print mode included, so a pasted line runs the turn
			// rather than opening the TUI.
			arguments := []string{binary, "--session", session.ID}
			execArguments := append([]string{binary},
				piResumeArguments(session.ID, resumePromptPlaceholder)...)
			return targetInvocation{
				Arguments:     arguments,
				ExecArguments: execArguments,
				// The environment is the check's own (piResumeEnvironment): the data directory and the
				// sessions tree the session was installed into, so a caller's leftover
				// PI_CODING_AGENT_SESSION_DIR cannot send the pasted line looking in another tree than
				// the one the check verified.
				Environment: piResumeEnvironment(in.Home, session.Directory),
				// Measured: Pi resolves a session id through the project directory of the process's own
				// working directory, so the invocation has to run from the session's own.
				WorkingDir: session.Cwd,
			}
		},
		Fields: func(session convertedTarget, facts targetFacts) targetFacts {
			// Pi's session records no provider binding of its own, so the report states the document's own
			// format version and the last model the source's messages recorded -- and, where it names no
			// provider, says why: the read answers the model each source message recorded rather than the
			// endpoint that served it, and recording the harness label (`pi`, `claude`, `codex`) in the
			// provider's place would name a Pi provider the caller's configuration need not define.
			target := session.Raw.(PiSession)
			facts.CLIVersion = target.CLIVersion
			facts.ProviderSource = "unrecorded"
			facts.ProviderDetail = "no provider is recorded: a Pi session binds none to itself, and the " +
				"source read answers the model each message recorded rather than the endpoint that " +
				"served it (the read's session-level provider names the harness that wrote the words). " +
				"Every converted assistant entry carries that model; model_provider_binding states the " +
				"format's own limit"
			facts.Model = target.Model
			return facts
		},
		StructureMethod:       structureMethodPi,
		StructureMethodDryRun: structureMethodPiDryRun,
		LoadMethod:            loadMethodPi,
		ResumeMethod:          resumeMethodPi,
		StructureReader:       structureReaderPi,
	}
}

// piFacts records which version the session's records are written as: the probed one when the installed
// binary answered, else the pinned version this writer's format was verified against. Pi's own session
// records no version field of its own that a loader checks, and it binds no model provider, so nothing
// else is asked of the harness here.
func piFacts(_ context.Context, in targetFactsInput) targetFacts {
	facts := targetFacts{CLIVersion: PiFormatVersion, CLIVersionSource: "writer-pin"}
	if in.Probe.Version != "" {
		facts.CLIVersion = in.Probe.Version
		facts.CLIVersionSource = "probe"
	}
	return facts
}

// A Pi session binds no model to itself, and the models its turns record are the *source's* -- a
// source-side provider label (`claude`, `codex`, `pi` as AgentsView spells the agent that wrote the words)
// with a model id from that provider's catalog. Pi's own providers are the endpoints its configuration
// defines, so the source's label names an endpoint the target home need not have, and the source's model
// id may be ambiguous across the endpoints it does have or missing from them: measured, a printed
// `--provider claude` stopped a run before it opened the session (`Unknown provider "claude"`), and a
// printed `--model claude-sonnet-4-5` stopped one before it opened the session too ("ambiguous across
// providers ... No matching provider is authenticated"). A resume command that cannot open the session is
// not a resume command, so the route prints neither and the resumed turn runs under whatever model the
// caller's own Pi configuration offers — which is what the `model_provider_binding` format limit and the
// `message_model_attribution` caveat state, while the conversation itself carries the source's model on
// every assistant entry it was recorded with.

// ompRoute is the route increment 0006 adds: an OMP session JSONL file -- the Pi family's own tree, with
// the records OMP's format adds (a rewritable title slot line, and a session-level model binding the
// harness restores the session's model from) -- installed under the project directory *OMP's* own
// discovery derives from the session's working directory, verified through OMP's own RPC loader (no model
// turn) and one real turn through the reported invocation.
func ompRoute() targetRoute {
	return targetRoute{
		Name:          TargetOMP,
		Format:        OMPTargetFormat,
		HomeEnv:       ompHomeEnv,
		BinaryDefault: OMPBinaryDefault,
		PinVersion:    OMPFormatVersion,
		FormatLimits:  ompFormatLimits(),
		ResolveHome:   ResolveOMPHome,
		Probe:         ProbeOMPBinary,
		Facts:         ompFacts,
		// A dry run never starts the harness, and this format needs its answer: the session's own model
		// binding and the endpoint label on every assistant record are what the harness restores a session
		// from and what its request builder reads. A run that installs asks, and records its own answer.
		DryRunWarnings: func(facts targetFacts, probe harnessProbe) []string {
			if !probe.Found {
				return nil
			}
			return []string{"this was a dry run, so the target harness was not started and the model its " +
				"data directory resolves for a new session of it was not asked: the written copy records " +
				"no session model binding and no endpoint label on its assistant records. An installed run " +
				"asks the harness and records its own answer, which is what this format needs for its " +
				"loader to open the session again"}
		},
		Convert: func(conv Conversation, in targetConvertInput) (convertedTarget, error) {
			session, err := ConvertToOMPSession(conv, OMPTargetOptions{
				CLIVersion:     in.CLIVersion,
				Now:            in.Now,
				MaxToolNames:   in.MaxToolNames,
				Provider:       in.Provider,
				Model:          in.Model,
				ProviderDetail: in.ProviderDetail,
			})
			if err != nil {
				return convertedTarget{}, err
			}
			return convertedTarget{
				ID:        session.SessionID,
				FileName:  session.FileName,
				Directory: session.Directory,
				Cwd:       session.Cwd,
				StartedAt: session.StartedAt,
				Data:      session.Data,
				Carried:   session.Carried,
				Losses:    session.Losses,
				Raw:       session,
			}, nil
		},
		Install: func(_ context.Context, session convertedTarget, in targetInstallInput) (string, string, error) {
			return InstallOMPSession(session.Raw.(PiSession), in.Home)
		},
		StructureRoot: func(home string) string { return filepath.Join(home, ompSessionsSubdir) },
		ReadBack: func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error) {
			return VerifyOMPSessionStructure(ctx, root, session.Raw.(PiSession), cfg)
		},
		Load: func(ctx context.Context, in targetVerificationInput) LoadVerification {
			return VerifyOMPLoad(ctx, OMPLoadOptions{
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
			// The turn is the invocation's own words, run through the same environment and working
			// directory the printed line names, so the check exercises what the report prints.
			return VerifyOMPResume(ctx, OMPResumeOptions{
				Home:        in.Home,
				Binary:      in.Binary,
				SessionID:   in.SessionID,
				ProjectDir:  in.Directory,
				Cwd:         in.Cwd,
				SessionPath: in.Path,
				Prompt:      in.Prompt,
				Timeout:     in.Timeout,
				Stderr:      in.Stderr,
			})
		},
		Invocation: func(session convertedTarget, in targetInvocationInput) targetInvocation {
			// This invocation changes into the session's working directory, so its words are the program
			// this run probed and verified (invocationBinary): a bare name re-searched by a shell from
			// there can find a different program.
			binary := invocationBinary(in.Binary, in.Probe)
			// The interactive line names the session and nothing else; the exec form is
			// ompResumeArguments, the invocation VerifyOMPResume runs -- print mode included, so a pasted
			// line runs the turn rather than opening the harness's picker or TUI.
			arguments := []string{binary, "--resume", session.ID}
			execArguments := append([]string{binary},
				ompResumeArguments(session.ID, resumePromptPlaceholder)...)
			return targetInvocation{
				Arguments:     arguments,
				ExecArguments: execArguments,
				// The environment is the check's own (ompResumeEnvironment): the data directory and the
				// directory inside it that holds this session's file, so a caller's leftover
				// PI_CODING_AGENT_SESSION_DIR cannot send the pasted line looking in another tree than
				// the one the check verified.
				Environment: ompResumeEnvironment(in.Home, session.Directory),
				// The session belongs to its recorded directory, and the printed line continues the work
				// there: measured, this harness resolves a session by id from any directory, so a caller
				// who runs it elsewhere still finds the session rather than forking one.
				WorkingDir: session.Cwd,
			}
		},
		Fields: func(session convertedTarget, facts targetFacts) targetFacts {
			// The session's records state the target home's own model and endpoint, so the report states
			// them as that: `model_provider` is the provider a new session of this harness runs under (the
			// value asked of it, and the label every converted assistant record carries), and `model` is
			// that answer's model id. Both are the harness's answer rather than the source's, and the
			// provider detail is what it said.
			target := session.Raw.(PiSession)
			facts.CLIVersion = target.CLIVersion
			return facts
		},
		StructureMethod:       structureMethodOMP,
		StructureMethodDryRun: structureMethodOMPDryRun,
		LoadMethod:            loadMethodOMP,
		ResumeMethod:          resumeMethodOMP,
		StructureReader:       structureReaderOMP,
	}
}

// ompFacts records which version the session's records are written as -- the probed one when the installed
// binary answered, else the pinned version this writer's format was verified against -- and asks the
// harness itself which model a new session of it would run under.
//
// The model is not a convenience: OMP's loader restores a session's model from the record this route
// writes, and refuses a session whose recorded model the target home cannot resolve (measured), so the
// answer is what makes a converted session openable there. It is asked of the harness rather than read out
// of its configuration file (ProbeOMPModel), and a dry run does not ask it at all -- starting the harness
// against a caller's data directory is exactly the sort of write a preview must not perform.
func ompFacts(ctx context.Context, in targetFactsInput) targetFacts {
	facts := targetFacts{CLIVersion: OMPFormatVersion, CLIVersionSource: "writer-pin"}
	if in.Probe.Version != "" {
		facts.CLIVersion = in.Probe.Version
		facts.CLIVersionSource = "probe"
	}
	probe := OMPModelProbe{}
	switch {
	case in.DryRun:
		probe.Detail = "a dry run does not start the target harness against its data directory, so the " +
			"model that home resolves for a new session of it was not asked and none is recorded"
	case in.Probe.Found:
		probe = ProbeOMPModel(ctx, in.Home, in.Probe.Binary, in.Timeout, in.Stderr)
	default:
		// The harness could not be located at all: the reason the probe carries is the same answer this
		// step does not have, and the report must state it rather than an unexplained absence.
		probe.Detail = in.Probe.Detail + ", so the model that home resolves for a new session of it was " +
			"not asked and none is recorded"
	}
	facts.ModelProvider = probe.Provider
	facts.Model = probe.Model
	facts.ProviderDetail = probe.Detail
	facts.ProviderSource = "unrecorded"
	if probe.Found {
		facts.ProviderSource = probe.Source
	}
	return facts
}

// openCodeRoute is the route increment 0007 adds: the harness's own session import document, installed
// by the harness's own importer (there is no file to write -- a session is a row in the harness's
// database), read back through the inherited AgentsView OpenCode reader by its own `path#id` reference,
// verified through the harness's own reader (a `session export`, measured to change none of the
// session's own records) and one real turn through the printed invocation.
func openCodeRoute() targetRoute {
	return targetRoute{
		Name:          TargetOpenCode,
		Format:        OpenCodeTargetFormat,
		HomeEnv:       openCodeDataRootEnv,
		BinaryDefault: OpenCodeBinaryDefault,
		PinVersion:    OpenCodeFormatVersion,
		FormatLimits:  openCodeFormatLimits(),
		ResolveHome:   ResolveOpenCodeHome,
		Probe:         ProbeOpenCodeBinary,
		Facts:         openCodeFacts,
		// A dry run leaves this format's own answer unasked: every assistant record names an agent and a
		// model reference, and the session's own model binding is what a resumed turn runs under -- none
		// of which the harness's importer answers, and it is the importer that does run for a dry run
		// (against the throw-away copy the structure check reads). A run that installs asks the harness's
		// own agent listing, and records its answer.
		DryRunWarnings: func(facts targetFacts, probe harnessProbe) []string {
			if !probe.Found {
				return nil
			}
			return []string{"this was a dry run, so the harness's own agent listing was not asked and " +
				"the agent and model its home resolves for a new session of it are not recorded: the " +
				"written document records no session model binding, the built-in default agent's name on " +
				"its assistant records, and the empty model id on the records the source read recorded no " +
				"model for. An installed run asks the harness and records its own answer"}
		},
		Convert: func(conv Conversation, in targetConvertInput) (convertedTarget, error) {
			session, err := ConvertToOpenCodeSession(conv, OpenCodeTargetOptions{
				CLIVersion:     in.CLIVersion,
				Provider:       in.Provider,
				Model:          in.Model,
				ModelVariant:   in.ModelVariant,
				Agent:          in.Agent,
				ProviderDetail: in.ProviderDetail,
				Now:            in.Now,
				MaxToolNames:   in.MaxToolNames,
			})
			if err != nil {
				return convertedTarget{}, err
			}
			// This route files nothing inside a session root: the harness's own store is the artifact,
			// and the session's recorded directory travels as Cwd. FileName stays empty -- there is no
			// file to name, so the report's target_session names none.
			return convertedTarget{
				ID:        session.SessionID,
				Cwd:       session.Cwd,
				StartedAt: session.StartedAt,
				Data:      session.Data,
				Carried:   session.Carried,
				Losses:    session.Losses,
				Raw:       session,
			}, nil
		},
		Install: func(ctx context.Context, session convertedTarget, in targetInstallInput) (string, string, error) {
			return InstallOpenCodeSession(ctx, session.Raw.(OpenCodeSession), in)
		},
		// The store is what the harness reads sessions from; the inherited reader's own `path#id`
		// reference inside it is the check's business (VerifyOpenCodeSessionStructure).
		StructureRoot: func(home string) string { return openCodeDataDir(home) },
		ReadBack: func(ctx context.Context, root string, session convertedTarget, cfg config.Config) (StructureVerification, error) {
			return VerifyOpenCodeSessionStructure(ctx, root, session.Raw.(OpenCodeSession), cfg)
		},
		Load: func(ctx context.Context, in targetVerificationInput) LoadVerification {
			return VerifyOpenCodeLoad(ctx, in)
		},
		Resume: func(ctx context.Context, in targetVerificationInput) ResumeVerification {
			// The turn is the invocation's own words, run through the same environment and working
			// directory the printed line names, so the check exercises what the report prints.
			return VerifyOpenCodeResume(ctx, in)
		},
		Invocation: func(session convertedTarget, in targetInvocationInput) targetInvocation {
			// This invocation changes into the session's recorded working directory, so its words are
			// the program this run probed and verified (invocationBinary): a bare name re-searched by a
			// shell from there can find a different program. The data root is named for the same reason
			// the other routes name their home: a caller's leftover setting would otherwise send the
			// pasted line to another store than the one the check read.
			binary := invocationBinary(in.Binary, in.Probe)
			arguments := append([]string{binary}, openCodeResumeArguments(session.ID, "")...)
			execArguments := append([]string{binary},
				openCodeResumeArguments(session.ID, resumePromptPlaceholder)...)
			return targetInvocation{
				Arguments:     arguments,
				ExecArguments: execArguments,
				Environment:   openCodeResumeEnvironment(in.Home),
				WorkingDir:    session.Cwd,
			}
		},
		Fields: func(session convertedTarget, facts targetFacts) targetFacts {
			// The document records the target home's own model and provider on the session and the
			// endpoint label on every assistant record, so the report states them as that: the provider
			// a new session of this harness runs under (the value asked of it) and that answer's model.
			target := session.Raw.(OpenCodeSession)
			facts.CLIVersion = target.CLIVersion
			return facts
		},
		StructureMethod:       structureMethodOpenCode,
		StructureMethodDryRun: structureMethodOpenCodeDryRun,
		LoadMethod:            loadMethodOpenCode,
		ResumeMethod:          resumeMethodOpenCode,
		StructureReader:       structureReaderOpenCode,
	}
}
