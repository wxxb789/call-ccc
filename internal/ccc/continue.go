package ccc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// Verification levels a caller may ask for. "load" is the default because it is the target harness's own
// loader answering offline; "resume" runs one real turn and is the only level that proves the session
// continues.
const (
	VerifyNone   = "none"
	VerifyLoad   = "load"
	VerifyResume = "resume"
)

// Conversion and installation states, which a caller reads apart: the conversion is what the target
// document holds, and the installation is whether the session is where the target harness keeps its
// sessions. The verification states live with their verifications (codex_verify.go).
const (
	ConversionStateConverted      = "converted"
	InstallationStateInstalled    = "installed"
	InstallationStateNotInstalled = "not-installed"
)

// Source-file comparison results: what the source session's own bytes were shown to be after the
// conversion. There are three answers, not two -- the file was shown to be its same bytes, the file
// moved while it was converted, or the comparison did not run at all (an archive read has no file of its
// own, and a run can stop before comparing) -- and folding the third into either of the others would
// state something the run did not observe.
const (
	SourceCheckUnchanged   = "unchanged"
	SourceCheckChanged     = "changed"
	SourceCheckNotVerified = "not-verified"
)

// ErrUnsupportedTarget reports a target harness this build has no writer for.
var ErrUnsupportedTarget = errors.New("unsupported target harness")

// CheckVerifyOptions answers whether a verification request is one this build can honour, in the words
// the CLI refuses a command line with as well: the level must be none, load (the default when it is
// empty) or resume, and resume needs the turn's prompt, because the turn becomes part of the session.
// Continue calls it too, so a caller embedding the core and one driving the CLI are told the same
// thing about the same request.
func CheckVerifyOptions(verify, resumePrompt string) error {
	level := strings.ToLower(strings.TrimSpace(verify))
	switch level {
	case "", VerifyNone, VerifyLoad, VerifyResume:
	default:
		return fmt.Errorf("unknown verification level %q (none, load, resume)", verify)
	}
	if level == VerifyResume && strings.TrimSpace(resumePrompt) == "" {
		return errors.New(
			"a resume verification runs one turn in the installed session, so it needs --resume-prompt: " +
				"call/ccc does not write a prompt of its own into the session")
	}
	return nil
}

// ContinueOptions configures one conversion: what to read, where to write it, and how far to verify.
type ContinueOptions struct {
	// Ref identifies the source session, exactly as Inspect does.
	Ref Ref

	// Target names the harness the conversation continues in ("codex").
	Target string

	// Config supplies the provider roots and enabled agents, as in InspectOptions.
	Config config.Config
	// Reader, when set, is used instead of building one from Config.
	Reader *SourceReader
	// ArchivePath is an AgentsView archive consulted when the source session is not readable from its
	// own file. Empty means "source only".
	ArchivePath string

	// TargetHome is the target harness's data directory. Empty means "resolve it from the environment"
	// ($CODEX_HOME, else ~/.codex).
	TargetHome string
	// TargetBinary is the target harness's executable. Empty means CodexBinaryDefault on PATH.
	TargetBinary string

	// DryRun converts and reports without installing anything.
	DryRun bool
	// Verify is "none", "load" (default) or "resume".
	Verify string
	// ResumePrompt is the turn a "resume" verification runs. It is required for that level, because the
	// turn becomes part of the session.
	ResumePrompt string

	// Timeout bounds one verification command: one app-server request for the provider probe and the
	// load check, or one resume turn. Zero means the per-level default (60 seconds per app-server
	// request, 5 minutes for a resume turn).
	Timeout time.Duration
	// Now is the conversion instant; zero means time.Now.
	Now time.Time
	// MaxToolNames bounds how many tool names a report spells out.
	MaxToolNames int

	// Stderr receives the target harness's own diagnostics while a verification runs.
	Stderr io.Writer
}

// FormatLimit states something the target format cannot express at all, as opposed to what this run
// happened to leave out: a limit is a property of the pair of formats, not of one conversion, which is
// why it carries no count.
type FormatLimit struct {
	Category string `json:"category"`
	Detail   string `json:"detail"`
}

// ConversionReport is the conversion half: what the target document holds and what it does not.
type ConversionReport struct {
	State          string   `json:"state"`
	Format         string   `json:"format"`
	ReadFrom       string   `json:"read_from"`
	DryRun         bool     `json:"dry_run"`
	Carried        Carried  `json:"carried"`
	Losses         []Loss   `json:"losses"`
	SourceFidelity Fidelity `json:"source_fidelity"`
}

// TargetReport describes the session that was written and how to resume it.
type TargetReport struct {
	Provider         string        `json:"provider"`
	Format           string        `json:"format"`
	FormatLimits     []FormatLimit `json:"format_limits"`
	Home             string        `json:"home"`
	Binary           string        `json:"binary,omitempty"`
	BinaryFound      bool          `json:"binary_found"`
	BinaryVersion    string        `json:"binary_version,omitempty"`
	BinaryDetail     string        `json:"binary_detail,omitempty"`
	SessionID        string        `json:"session_id"`
	FileName         string        `json:"file_name"`
	Path             string        `json:"path,omitempty"`
	Cwd              string        `json:"cwd,omitempty"`
	StartedAt        time.Time     `json:"started_at,omitempty"`
	CLIVersion       string        `json:"cli_version"`
	CLIVersionSource string        `json:"cli_version_source"`
	ModelProvider    string        `json:"model_provider,omitempty"`
	ProviderSource   string        `json:"model_provider_source,omitempty"`
	ProviderDetail   string        `json:"model_provider_detail,omitempty"`
	Model            string        `json:"model,omitempty"`
	// ResumeArguments and ExecResumeArguments are the invocation itself, for a caller that runs the
	// harness rather than pasting a line into a shell: the argument vector that resumes the session,
	// with the prompt placeholder a caller replaces in the exec form. ResumeEnvironment is the
	// environment that selects the data directory the session was installed into; Codex has no flag for
	// that directory (its own loader takes it from CODEX_HOME alone), so a caller that runs the harness
	// without it resumes whatever home their shell already points at. ResumeCommand and
	// ExecResumeCommand are the same two invocations rendered as a POSIX shell line -- environment
	// first, every word quoted where a shell would read it as syntax -- which is what a person or an
	// agent pastes.
	ResumeArguments     []string          `json:"resume_arguments"`
	ExecResumeArguments []string          `json:"exec_resume_arguments"`
	ResumeEnvironment   map[string]string `json:"resume_environment"`
	ResumeCommand       string            `json:"resume_command"`
	ExecResumeCommand   string            `json:"exec_resume_command"`
}

// InstallationReport is the installation half: whether the session is where the target harness keeps
// its sessions, and what became of the source file's own bytes.
type InstallationReport struct {
	State     string `json:"state"`
	Path      string `json:"path,omitempty"`
	Mechanism string `json:"mechanism,omitempty"`
	Created   bool   `json:"created"`
	// SourceCheckState is one of the SourceCheck* states; Detail carries that state's own words -- the
	// digest it compared, what moved, or what stopped the run. A not-verified state with an empty Detail
	// is a comparison that never ran, which is what the report says when a run stops before it.
	SourceCheckState string `json:"source_check_state"`
	Detail           string `json:"detail,omitempty"`
}

// VerificationReport keeps the three answers apart: what the file holds, whether the target harness's
// loader read it, and whether a turn actually continued from it.
type VerificationReport struct {
	Structure StructureVerification `json:"structure"`
	Load      LoadVerification      `json:"load"`
	Resume    ResumeVerification    `json:"resume"`
}

// ContinueResult is one conversion's outcome, in the shape cmd/ccc serializes.
type ContinueResult struct {
	Ref           Ref                `json:"ref"`
	Target        string             `json:"target"`
	Source        *SessionReport     `json:"source,omitempty"`
	Conversion    ConversionReport   `json:"conversion"`
	TargetSession TargetReport       `json:"target_session"`
	Installation  InstallationReport `json:"installation"`
	Verification  VerificationReport `json:"verification"`
	Warnings      []string           `json:"warnings,omitempty"`
}

// conversionSourceReport summarises the source for a conversion's own report.
//
// It is reportFrom without the opening prompt's preview. A conversion report is a status report a
// caller collects, not an inspection of the source's content, and the core design forbids logging
// conversation bodies, tool arguments or full prompts by default; the preview reportFrom fills (up to
// firstMessageBytes of source text, the whole prompt when it is short) is display text for the
// inspection surface, which is the command that asks to see the session. The rule lives here rather
// than inside reportFrom so an inspection keeps its own documented preview.
func conversionSourceReport(conversation Conversation) SessionReport {
	report := reportFrom(conversation)
	report.FirstUserMessage = ""
	return report
}

// Continue converts a source session into a new native session of the target harness, installs it, and
// reports conversion, installation and native verification as three separate states.
//
// The pipeline is target-agnostic: everything a target harness answers differently -- what its data
// directory is called, what its format can express, how one session of it is written, installed and
// verified -- comes from the route targets.go keeps for it. Nothing here opens the source for writing,
// and nothing replaces a target session: the identity is fresh, the installation is exclusive, and a
// target harness whose data directory is not there is an error rather than a directory invented here.
//
// It is the engine's entry point and takes no CLI type.
func Continue(ctx context.Context, opts ContinueOptions) (ContinueResult, error) {
	target := strings.ToLower(strings.TrimSpace(opts.Target))
	if target == "" {
		return ContinueResult{}, fmt.Errorf("%w: choose one (%s)", ErrUnsupportedTarget, strings.Join(Targets(), ", "))
	}
	route, err := targetRouteFor(target)
	if err != nil {
		return ContinueResult{}, err
	}
	if err := CheckVerifyOptions(opts.Verify, opts.ResumePrompt); err != nil {
		return ContinueResult{}, err
	}
	level := strings.ToLower(strings.TrimSpace(opts.Verify))
	if level == "" {
		level = VerifyLoad
	}
	return continueWith(ctx, opts, route, level)
}

// continueWith runs one conversion along a target's route.
func continueWith(ctx context.Context, opts ContinueOptions, route targetRoute, level string) (ContinueResult, error) {
	result := ContinueResult{Ref: opts.Ref, Target: route.Name}

	home, explicitHome, err := route.ResolveHome(opts.TargetHome)
	if err != nil {
		return ContinueResult{}, err
	}
	// The home is resolved, and checked for shape, before anything else happens; it is *created* only by
	// the installation that would write into it. A missing directory nobody named is still refused.
	homeMissing := false
	if info, err := os.Stat(home); err != nil {
		if !explicitHome {
			return ContinueResult{}, fmt.Errorf(
				"%w at %s: pass --target-home (or set %s) to say where that harness keeps its sessions",
				ErrTargetHomeMissing, home, route.HomeEnv)
		}
		homeMissing = true
	} else if !info.IsDir() {
		return ContinueResult{}, fmt.Errorf("%w: %s is not a directory", ErrTargetHomeMissing, home)
	}

	probe := route.Probe(ctx, opts.TargetBinary)
	cfg, reader, err := resolveContinueConfig(opts)
	if err != nil {
		return ContinueResult{}, err
	}
	conversation, readFrom, readWarnings, err := readForConversion(ctx, reader, cfg, opts.Ref, opts.ArchivePath)
	if err != nil {
		return ContinueResult{}, err
	}
	result.Source = new(conversionSourceReport(conversation))
	result.Warnings = append(result.Warnings, readWarnings...)
	// The provider's own classification of this read travels into the report's warnings, not only into
	// its fidelity block and the conversion's loss list: a summary fallback, a result the provider says
	// is not current and a read whose tool outputs could not be read are all reads a caller must not take
	// for the session's recorded transcript.
	result.Warnings = append(result.Warnings, fidelityWarnings(conversation.Fidelity)...)

	// The source's digest is taken before anything is written and compared again afterwards: the
	// property the design asks for is that the transcript a conversation came from is the same file
	// afterwards, whatever the provider's own fingerprint happens to hash.
	sourceDigest, err := sourceFileDigest(conversation, readFrom)
	if err != nil {
		return ContinueResult{}, err
	}

	// What only the target's own harness can answer (Codex: the model provider its loader checks). It is
	// asked here, after the read, because the answer can depend on the session's own working directory.
	facts := route.Facts(ctx, targetFactsInput{
		Home:        home,
		HomeMissing: homeMissing,
		DryRun:      opts.DryRun,
		Probe:       probe,
		SourceCwd:   conversation.Source.Cwd,
		Timeout:     opts.Timeout,
		Stderr:      opts.Stderr,
	})
	if strings.TrimSpace(facts.CLIVersion) == "" {
		facts.CLIVersion = route.PinVersion
	}
	if strings.TrimSpace(facts.CLIVersionSource) == "" {
		facts.CLIVersionSource = "writer-pin"
	}

	session, err := route.Convert(conversation, targetConvertInput{
		CLIVersion:   facts.CLIVersion,
		Provider:     facts.ModelProvider,
		Now:          opts.Now,
		MaxToolNames: opts.MaxToolNames,
	})
	if err != nil {
		return ContinueResult{}, err
	}
	result.Conversion = ConversionReport{
		State:          ConversionStateConverted,
		Format:         route.Format,
		ReadFrom:       readFrom,
		DryRun:         opts.DryRun,
		Carried:        session.Carried,
		Losses:         session.Losses,
		SourceFidelity: conversation.Fidelity,
	}
	// The invocation is built here, where the session identity exists, and rendered twice: as the
	// argument vector and environment a caller runs, and as the shell line a person pastes.
	resumeName := strings.TrimSpace(opts.TargetBinary)
	if resumeName == "" {
		resumeName = route.BinaryDefault
	}
	invocation := route.Invocation(session, targetInvocationInput{Binary: resumeName, Home: home})
	fields := route.Fields(session, facts)
	result.TargetSession = TargetReport{
		Provider:            route.Name,
		Format:              route.Format,
		FormatLimits:        route.FormatLimits,
		Home:                home,
		Binary:              probe.Binary,
		BinaryFound:         probe.Found,
		BinaryVersion:       probe.Version,
		BinaryDetail:        probe.Detail,
		SessionID:           session.ID,
		FileName:            session.FileName,
		Cwd:                 session.Cwd,
		StartedAt:           session.StartedAt,
		CLIVersion:          fields.CLIVersion,
		CLIVersionSource:    fields.CLIVersionSource,
		ModelProvider:       fields.ModelProvider,
		ProviderSource:      fields.ProviderSource,
		ProviderDetail:      fields.ProviderDetail,
		Model:               fields.Model,
		ResumeArguments:     invocation.Arguments,
		ExecResumeArguments: invocation.ExecArguments,
		ResumeEnvironment:   invocation.Environment,
		ResumeCommand:       renderInvocation(invocation.Arguments, invocation.Environment, invocation.WorkingDir),
		ExecResumeCommand:   renderInvocation(invocation.ExecArguments, invocation.Environment, invocation.WorkingDir),
	}
	verification := VerificationReport{
		Structure: StructureVerification{State: VerificationStateNotVerified, Method: route.StructureMethod},
		Load:      LoadVerification{State: VerificationStateNotRequested, Method: route.LoadMethod},
		Resume:    ResumeVerification{State: VerificationStateNotRequested, Method: route.ResumeMethod},
	}
	// The installation block exists from here on, whatever the run does next: a run that stops before
	// publishing anything still answers "is it installed?" -- with not-installed -- and says which checks
	// it did not reach, instead of handing back a document with an empty block.
	result.Installation = InstallationReport{
		State:            InstallationStateNotInstalled,
		SourceCheckState: SourceCheckNotVerified,
	}

	if opts.DryRun {
		// A dry run touches no target directory: the conversion is verified against a throw-away copy of
		// the same bytes, which is what makes the check comparable with an installed run. The home it
		// would install into is named but not created, and the report says which state that is. The
		// structure check reads that copy, so its method says so before it runs: a run that stops before
		// the check reports the mechanism it did not use rather than an empty one.
		verification.Structure.Method = route.StructureMethodDryRun
		scratch, err := os.MkdirTemp("", "ccc-dry-run-")
		if err != nil {
			return reportAfterFailure(result, verification, level, fmt.Errorf("creating a scratch directory: %w", err))
		}
		defer os.RemoveAll(scratch)
		if _, _, err := route.Install(session, scratch); err != nil {
			return reportAfterFailure(result, verification, level, fmt.Errorf("verifying the conversion: %w", err))
		}
		structure, err := route.ReadBack(ctx, route.StructureRoot(scratch), session, cfg)
		if err != nil {
			return reportAfterFailure(result, verification, level, err)
		}
		structure.Method = route.StructureMethodDryRun
		structure.TargetPath = ""
		verification.Structure = structure
		switch level {
		case VerifyNone:
			verification.Load.Detail = "verification was not requested (--verify none)"
			verification.Resume.Detail = "verification was not requested (--verify none)"
		default:
			// The level was asked for, and a dry run installed nothing for the target harness to load:
			// the state says the check did not verify rather than that nobody asked for it, and each
			// detail carries the dry-run reason.
			verification.Load.State = VerificationStateNotVerified
			verification.Load.Detail = "a dry run installs nothing, so the target harness has nothing to load"
			if level == VerifyResume {
				verification.Resume.State = VerificationStateNotVerified
				verification.Resume.Detail = "a dry run installs nothing, so no turn can continue from it"
			} else {
				verification.Resume.Detail = "not requested (--verify " + level + ")"
			}
		}
		result.Verification = verification
		result.Warnings = append(result.Warnings, structureWarnings(route, structure)...)
		// The dry run writes nothing, so the source file is compared exactly the way an installed run
		// compares it: the report claims the check only when it ran.
		sourceState, sourceDetail, err := sourceCheckAfter(conversation, readFrom, sourceDigest)
		if err != nil {
			return reportAfterFailure(result, verification, level, err)
		}
		result.Installation.SourceCheckState = sourceState
		result.Installation.Detail = sourceDetail
		result.Warnings = append(result.Warnings, sourceChangedWarnings(sourceState)...)
		// A dry run's conversion may record nothing a target harness would have filled in, because the
		// harness was never started against the home; the route says what that means for its format.
		result.Warnings = append(result.Warnings, route.DryRunWarnings(facts, probe)...)
		if homeMissing {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"the target data directory %s does not exist and a dry run did not create it; an installed "+
					"run would", home))
		}
		return result, nil
	}

	if homeMissing {
		// The installation creates the directory it writes into, and only then: a run that ends before
		// this point (a refused reference, an unreadable source, a dry run) leaves the filesystem as it
		// found it.
		if err := os.MkdirAll(home, 0o700); err != nil {
			return reportAfterFailure(result, verification, level, fmt.Errorf("creating %s: %w", home, err))
		}
	}
	path, mechanism, err := route.Install(session, home)
	if path != "" {
		// The installer returns the path it published even when it also reports a failure (a temporary
		// file it could not remove): the report carries the identity the installed session can be resumed
		// by, and the failure is answered beside it rather than instead of it.
		result.TargetSession.Path = path
		result.Installation = InstallationReport{
			State:            InstallationStateInstalled,
			Path:             path,
			Mechanism:        mechanism,
			Created:          true,
			SourceCheckState: SourceCheckNotVerified,
		}
	}
	if err != nil {
		// Nothing was published, or a published session's own failure: either way the report answers
		// "installed?" from the path above, and this error is what the run failed on.
		return reportAfterFailure(result, verification, level, err)
	}
	sourceState, sourceDetail, err := sourceCheckAfter(conversation, readFrom, sourceDigest)
	if err != nil {
		return reportAfterFailure(result, verification, level, err)
	}
	result.Installation.SourceCheckState = sourceState
	result.Installation.Detail = sourceDetail
	result.Warnings = append(result.Warnings, sourceChangedWarnings(sourceState)...)

	verificationInput := targetVerificationInput{
		Home:          home,
		Binary:        probe.Binary,
		BinaryVersion: probe.Version,
		SessionID:     session.ID,
		Cwd:           session.Cwd,
		Path:          path,
		Directory:     session.Directory,
		FileName:      session.FileName,
		Data:          session.Data,
		Timeout:       opts.Timeout,
		Stderr:        opts.Stderr,
	}
	structure, err := route.ReadBack(ctx, route.StructureRoot(home), session, cfg)
	if err != nil {
		return reportAfterFailure(result, verification, level, err)
	}
	verification.Structure = structure
	switch level {
	case VerifyNone:
		verification.Load.Detail = "verification was not requested (--verify none)"
		verification.Resume.Detail = "verification was not requested (--verify none)"
	default:
		if !probe.Found {
			verification.Load.State = VerificationStateNotVerified
			verification.Load.Detail = fmt.Sprintf(
				"%s is not on PATH, so the installed session's native load could not be verified%s",
				resumeName, probeDetailSuffix(probe))
			// A resume verification nobody asked for stays not-requested even here: the three states
			// exist to keep "could not run" apart from "nobody asked", and the load check's own failure
			// is what the block above already reports.
			if level == VerifyResume {
				verification.Resume.State = VerificationStateNotVerified
				verification.Resume.Detail = verification.Load.Detail
			} else {
				verification.Resume.Detail = "not requested (--verify " + level + ")"
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"the target harness was not found on PATH: the conversation is converted and installed at %s, "+
					"and its native load is unverified", path))
			break
		}
		verification.Load = route.Load(ctx, verificationInput)
		verification.Load.BinaryVersion = probe.Version
		if level == VerifyResume {
			resumeInput := verificationInput
			resumeInput.Prompt = opts.ResumePrompt
			verification.Resume = route.Resume(ctx, resumeInput)
		} else {
			verification.Resume.Detail = "not requested (--verify " + level + ")"
		}
	}
	result.Verification = verification
	result.Warnings = append(result.Warnings, structureWarnings(route, structure)...)
	if verification.Structure.State != VerificationStateVerified {
		result.Warnings = append(result.Warnings, "the installed session does not hold every piece of "+
			"evidence the conversion wrote: "+verification.Structure.Detail)
	}
	return result, nil
}

// structureWarnings answers what the structural check found, for the report's own channel: a caller that
// reads only the exit code, or only the human block's head, must not take "verified" as "the target's own
// read of the session shows every message with the text the file holds", and a written message the
// inherited reader reads no message for -- or reads only the text an injected-context envelope preceded
// -- is exactly that case. A check that did not verify is answered where it failed, by the warning that
// names what the file does not hold.
func structureWarnings(route targetRoute, structure StructureVerification) []string {
	if structure.State != VerificationStateVerified || structure.NotReadBackVerbatim == 0 {
		return nil
	}
	return []string{fmt.Sprintf("the installed session holds %d written message(s) the inherited %s "+
		"reader does not read their own text for (verification.not_read_back_verbatim): their dialogue "+
		"records are in the file, so the conversion carried them and the reader's own projection of the "+
		"file is what the read-back shows instead", structure.NotReadBackVerbatim, route.StructureReader)}
}

// What each check reads, named once per target, so a check that did not run still reports the mechanism
// it would have used: a reader sees which question went unanswered, not an empty field.
const (
	structureMethodCodex       = "the inherited AgentsView Codex reader over the installed session"
	structureMethodCodexDryRun = "the inherited AgentsView Codex reader over a throw-away copy (dry run)"
	loadMethodCodex            = "the target harness's own loader (codex app-server, thread/resume)"

	structureMethodClaude       = "the inherited AgentsView Claude reader over the installed session"
	structureMethodClaudeDryRun = "the inherited AgentsView Claude reader over a throw-away copy (dry run)"
	// The Claude load check drives the harness's own loader with --bare and with the environment
	// variables that could reach a model removed, so the loader resolves the installed session without a
	// model turn, and runs an unknown-id control arm beside it: see claude_verify.go.
	loadMethodClaude = "the target harness's own loader (claude --bare --resume, with no model " +
		"credentials in its environment, plus an unknown-id control)"

	resumeMethod = "one turn in the installed session through the target harness's non-interactive resume"
	// The Claude resume check runs the reported invocation as it is printed, which is also the exec form
	// the report carries.
	resumeMethodClaude = "one turn in the installed session through the target harness's non-interactive " +
		"resume (claude -p --resume)"
)

// hasAnswer reports whether a check carries an answer of its own: it verified, or it came back
// not-verified with its own reason. Every other shape -- including the placeholder a report starts from,
// a not-verified state with no detail -- is a check that has not answered.
func hasAnswer(state, detail string) bool {
	return state == VerificationStateVerified || strings.TrimSpace(detail) != ""
}

// reportAfterFailure answers a failure that happened after the conversion was written: the report keeps
// everything the run completed -- the conversion, and the installation with the target identity when the
// session was published -- and every check the failure kept from running says it did not run, naming what
// stopped the run. The error is still returned, so the run fails; what it must not do is cost the caller
// the identity of a session that is already installed, or the loss report of a conversion that is already
// written.
//
// It fills in the checks that have no answer yet and never overwrites one that answered: the dry run's
// structural read-back runs before the source comparison, so a failure after it must leave its own
// verdict standing rather than downgrading it to "the run stopped".
func reportAfterFailure(
	result ContinueResult, verification VerificationReport, level string, err error,
) (ContinueResult, error) {
	reason := "the run stopped before this check ran: " + err.Error()
	if !hasAnswer(verification.Structure.State, verification.Structure.Detail) {
		verification.Structure.State = VerificationStateNotVerified
		verification.Structure.Detail = reason
	}
	if !hasAnswer(verification.Load.State, verification.Load.Detail) {
		if level == VerifyNone {
			// Nobody asked for the native checks, and a run that stopped is not a reason to blur that
			// state with "could not run": the three states answer different questions.
			verification.Load.Detail = "verification was not requested (--verify none)"
		} else {
			verification.Load.State = VerificationStateNotVerified
			verification.Load.Detail = reason
		}
	}
	if !hasAnswer(verification.Resume.State, verification.Resume.Detail) {
		switch {
		case level == VerifyResume:
			verification.Resume.State = VerificationStateNotVerified
			verification.Resume.Detail = reason
		case level == VerifyNone:
			verification.Resume.Detail = "verification was not requested (--verify none)"
		default:
			verification.Resume.Detail = "not requested (--verify " + level + ")"
		}
	}
	result.Verification = verification
	if result.Installation.SourceCheckState == SourceCheckNotVerified && result.Installation.Detail == "" {
		// The source file was never compared, because the failure came first: the report names what
		// stopped the run rather than leaving the installation block's reason empty.
		result.Installation.Detail = reason
	}
	// The report's own channel as well: a caller that reads only the warnings must not have to infer
	// from a state field that the run did not finish.
	result.Warnings = append(result.Warnings, "the run stopped after the conversion: "+err.Error())
	return result, err
}

// resolveContinueConfig answers the configuration and reader a conversion reads with, the same way
// Inspect does: a caller-supplied reader's own configuration wins over the caller's zero value.
func resolveContinueConfig(opts ContinueOptions) (config.Config, *SourceReader, error) {
	cfg := opts.Config
	reader := opts.Reader
	switch {
	case cfg.AgentDirs == nil && reader != nil:
		cfg = reader.cfg
	case cfg.AgentDirs == nil:
		loaded, err := config.LoadReadOnly()
		if err != nil {
			return config.Config{}, nil, fmt.Errorf("reading the AgentsView configuration: %w", err)
		}
		cfg = loaded
	}
	if reader == nil {
		reader = NewSourceReader(cfg)
	}
	return cfg, reader, nil
}

// readForConversion reads the source session, falling back to an archive when the source side cannot
// answer a reference an archive holds. The fallback is what makes a conversion possible for a session
// whose file is gone, and it is reported: an archive read is a projection, and its fidelity block says
// what it could not show.
//
// Which failure a caller sees is decided by the archive, not by the source: the source's own error is
// answered only when the archive holds no match for the reference (the two sides then say the same
// thing), while an archive that refuses the reference -- several candidate identities, a lookup it
// could not complete, a query that failed -- is reported as that, because "the source session was not
// found" would hide the very candidates a caller needs to name the session.
func readForConversion(
	ctx context.Context, reader *SourceReader, cfg config.Config, ref Ref, archivePath string,
) (Conversation, string, []string, error) {
	conversation, err := reader.Read(ctx, ref)
	if err == nil {
		return conversation, "source", nil, nil
	}
	// The classifications that leave an archive in reach: the source side failed to answer a reference
	// an archive can still hold. ErrProviderFailed covers both of the shapes it has -- a parse outcome
	// carrying operational source errors and no result, and a fingerprint or a parse that failed at the
	// I/O level (reader.go's Read). The second shape used to arrive as a bare error and stopped the
	// conversion here, before the archive was consulted, which reported a session an archive holds as
	// one this machine cannot read at all.
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNoLocalSource), errors.Is(err, ErrProviderFailed),
		errors.Is(err, ErrPartialSource), errors.Is(err, ErrSourceDisabled):
	default:
		return Conversation{}, "", nil, err
	}
	if strings.TrimSpace(archivePath) == "" {
		return Conversation{}, "", nil, err
	}
	archive, openErr := openArchive(ctx, archivePath, cfg.AgentDirs)
	if openErr != nil {
		if errors.Is(openErr, ErrNoArchive) {
			return Conversation{}, "", nil, err
		}
		return Conversation{}, "", nil, openErr
	}
	defer archive.Close()
	archived, readErr := archive.Read(ctx, ref)
	if readErr != nil {
		// The archive's own answer, unless it is absence. An ambiguous reference carries the candidate
		// identities that would resolve it, ErrTruncatedCandidates says the lookup could not complete
		// the list, and a query failure is the archive's to report -- returning the source side's error
		// instead would present every one of them as "no session", which is exactly what the source's
		// answer is not. Only an archive that holds no match leaves the source's failure as the answer,
		// because then the two sides agree: the reference names no session this machine can read.
		if !errors.Is(readErr, ErrNotFound) {
			return Conversation{}, "", nil, readErr
		}
		return Conversation{}, "", nil, err
	}
	warning := fmt.Sprintf(
		"the source session was read from the archive at %s, not from its own file (%v); an archive read "+
			"is a projection, and the reported fidelity says what it could not show", archivePath, err)
	return archived, "archive", []string{warning}, nil
}

// sourceFile answers where one conversation's own bytes are: the file a read resolved (`path`), or why no
// file holds them (`reason`). Exactly one of the two is set.
type sourceFile struct {
	path   string
	reason string
}

// sourceFileOf answers the file a source read's conversation lives in.
//
// A multi-session container is read through a *member* identity: the inherited providers spell it
// `parser.VirtualSourcePath(container, member)` (`<container>#<member>`), the parse records that spelling
// as the session's own path (`parser.ParsedSession.File.Path`), and this document carries it unchanged
// because it is what a caller resolves the member by. Such an identity is not a filesystem path -- which
// is why opening it is an error -- and no file holds the conversation's bytes: the transcript lives in the
// container's rows. The container's own bytes are not that file either: they hold every member the
// container carries, so digesting them answers a different question than the report asks (did *this*
// conversation's bytes move?) and would sometimes answer "unchanged" for a member a write did not reach
// (a database's write-ahead log, a buffered writer). The comparison is therefore reported as one that did
// not run, with the member it was about, instead of a conversion of a readable session being stopped by a
// check that cannot apply to it.
//
// An identity that names no file at all is not this case: it is a read that reported no path, and the
// caller reports that as a stopped run rather than as a state of a file.
func sourceFileOf(conversation Conversation) sourceFile {
	path := sourceFilePath(conversation)
	if container, member, ok := parser.ParseVirtualSourcePath(path); ok {
		return sourceFile{reason: fmt.Sprintf(
			"the source read resolved member %s inside the container %s, and no file holds this "+
				"member's own bytes to compare", member, container)}
	}
	return sourceFile{path: path}
}

// sourceFileDigest is the digest of the source session's own file, or "" when there is no file of its own
// to compare: a projection has no file of its own, and neither has a member read out of a container's
// rows, so the caller says the comparison did not run instead of claiming a check that did.
func sourceFileDigest(conversation Conversation, readFrom string) (string, error) {
	if readFrom != "source" {
		return "", nil
	}
	file := sourceFileOf(conversation)
	if file.path == "" {
		return "", nil
	}
	return fileDigest(file.path)
}

// sourceFilePath is the source session's own file, as the read recorded it.
func sourceFilePath(conversation Conversation) string {
	path := conversation.Source.SourcePath
	if strings.TrimSpace(path) == "" {
		path = conversation.Source.DisplayPath
	}
	return path
}

// sourceCheckAfter compares the source file with the digest taken before the conversion, which is the
// property the design asks for: the transcript the conversation came from is the same file afterwards.
// The comparison is of the file's own bytes before and after, not against a provider's fingerprint —
// those are different hashes for different readers (a provider may fingerprint a parse or a store
// member), and a check that compared them would report a change that did not happen.
//
// It answers with one of the SourceCheck* states and that state's own words. An error means the
// comparison could not run at all — a read that named no path, a digest that was never taken, a file that
// cannot be re-read — and the caller reports that as a stopped run rather than as a state the file earned.
// A read whose identity is no file's name is the other answer: the turn that would have compared it was
// never taken, and the state names the member identity instead (sourceFileOf).
func sourceCheckAfter(conversation Conversation, readFrom, before string) (string, string, error) {
	if readFrom != "source" {
		return SourceCheckNotVerified, "the conversion read an archive row, so no source file was open at all", nil
	}
	file := sourceFileOf(conversation)
	if file.reason != "" {
		return SourceCheckNotVerified, file.reason, nil
	}
	path := file.path
	if strings.TrimSpace(path) == "" {
		return "", "", errors.New("the source read reported no path, so its digest could not be compared")
	}
	if strings.TrimSpace(before) == "" {
		return "", "", errors.New("the source digest before the conversion was not taken")
	}
	after, err := fileDigest(path)
	if err != nil {
		return "", "", fmt.Errorf("re-reading the source session: %w", err)
	}
	if after != before {
		return SourceCheckChanged, fmt.Sprintf(
			"the source session changed while it was converted (sha256 %s before, %s after)", before, after), nil
	}
	return SourceCheckUnchanged, fmt.Sprintf("sha256 %s, taken before and after the conversion", after), nil
}

// sourceChangedWarnings is the report's warning for a source that moved while it was converted: the
// conversion and the installation are real, but the file no longer holds the bytes they read, and a
// caller that reads only the warnings must not miss it. A comparison that did not run is not this
// warning — it is not a claim that the file moved — and the state and its detail already say so.
func sourceChangedWarnings(state string) []string {
	if state != SourceCheckChanged {
		return nil
	}
	return []string{"the source session changed while it was converted: the file no longer holds the " +
		"bytes the conversion read"}
}

// fileDigest is one file's SHA-256, streamed rather than read into memory: a source transcript can be
// hundreds of megabytes, and the comparison needs its digest, not a copy of it. The io.Copy buffer is
// what keeps the read bounded (32 KiB at a time), so the caller's cost is the file's size, not twice it.
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// codexHomeEnv is the environment variable that selects the Codex data directory a run reads: Codex has
// no command-line flag for it, so a resume command that does not set it opens the caller's own default
// home rather than the directory the session was installed into.
const codexHomeEnv = "CODEX_HOME"

// claudeHomeEnv is the environment variable that selects the Claude Code data directory a run reads: the
// harness has no command-line flag for it, so a resume command that does not set it opens the caller's
// own default directory rather than the one the session was installed into.
const claudeHomeEnv = "CLAUDE_CONFIG_DIR"

// resumePromptPlaceholder is where the exec form of the resume invocation takes the caller's turn. It is
// a word of its own, not quoted shell syntax: a caller replaces it with the prompt's own words.
const resumePromptPlaceholder = "<prompt>"

// shellUnsafeCharacters are the characters a POSIX shell reads as syntax inside a word, so a word
// holding any of them has to be quoted; an empty word would vanish, so it is quoted too.
const shellUnsafeCharacters = " \t\n\r'\"\\$`&|;<>()*?[]{}!~#="

// renderResumeCommand spells one invocation the way a POSIX shell reads it: the environment that selects
// the target's data directory, then the words. Every word is quoted where a shell would act on it, so a
// home or a binary path holding a space cannot silently split into two arguments. The environment is
// spelled as a leading assignment (`CODEX_HOME=… codex resume …`), which is what makes a pasted command
// resume the session this report installed; the argument vector and environment beside it are the form a
// caller that runs the harness directly uses.
func renderResumeCommand(arguments []string, environment map[string]string) string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	words := make([]string, 0, len(names)+len(arguments))
	for _, name := range names {
		words = append(words, name+"="+shellQuote(environment[name]))
	}
	for _, argument := range arguments {
		words = append(words, shellQuote(argument))
	}
	return strings.Join(words, " ")
}

// renderInvocation is renderResumeCommand for a target whose session is only discoverable from a
// particular working directory: the line starts with the directory change, quoted the way the rest of the
// line is, so a person or an agent pasting it resumes the session this report installed rather than
// whatever the caller's shell happens to sit in. A target that does not care about the directory gets
// exactly the invocation names, with no `cd` at all.
func renderInvocation(arguments []string, environment map[string]string, workingDir string) string {
	command := renderResumeCommand(arguments, environment)
	if strings.TrimSpace(workingDir) == "" {
		return command
	}
	return "cd " + shellQuote(workingDir) + " && " + command
}

// shellQuote renders one command-line word so a POSIX shell reads it back as that word: left as it is
// when it holds nothing a shell would act on, and single-quoted otherwise, with an embedded quote closed
// and re-opened the way sh spells one.
func shellQuote(word string) string {
	if word != "" && !strings.ContainsAny(word, shellUnsafeCharacters) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// codexFormatLimits states what the target format cannot express at all, so a consumer reads them as
// capabilities of the pair rather than as this run's omissions.
func codexFormatLimits() []FormatLimit {
	return []FormatLimit{
		{
			Category: "attachments",
			Detail: "a message's own attachments and images are not in the source read at all: the " +
				"inherited reader keeps a message's text, so a conversion cannot say whether a " +
				"message had one and claims no count either way. A tool result is the exception: its " +
				"content field is read as the provider recorded it, and a block the target's " +
				"text-only output has no field for is reported by count " +
				"(unrepresentable_result_blocks)",
		},
		{
			Category: "provider_and_model",
			Detail: "the source's provider and model binding cannot travel: the written session records " +
				"the target's own configured model provider, which is what its loader checks, and no " +
				"model, so the target keeps choosing the model it resumes with",
		},
		{
			Category: "paginated_thread_store",
			Detail: "only the target's portable rollout is written; its paginated thread store is the " +
				"target harness's own, and it migrates the session on its own",
		},
		{
			Category: "replay",
			Detail: "carried tool calls and results are history: the target does not replay them, and a " +
				"resumed turn is free to run its own tools",
		},
	}
}

func probeDetailSuffix(probe harnessProbe) string {
	if strings.TrimSpace(probe.Detail) == "" {
		return ""
	}
	return " (" + probe.Detail + ")"
}
