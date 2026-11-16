package ccc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
)

// Verification levels a caller may ask for. "load" is the default because it is the target harness's own
// loader answering offline; "resume" runs one real turn and is the only level that proves the session
// continues.
const (
	VerifyNone   = "none"
	VerifyLoad   = "load"
	VerifyResume = "resume"
)

// Targets this build can write. Every other name is refused: a target that has no writer would
// otherwise be answered with a plausible-looking file.
const (
	TargetCodex = "codex"
)

// Conversion and installation states, which a caller reads apart: the conversion is what the target
// document holds, and the installation is whether the session is where the target harness keeps its
// sessions. The verification states live with their verifications (codex_verify.go).
const (
	ConversionStateConverted      = "converted"
	InstallationStateInstalled    = "installed"
	InstallationStateNotInstalled = "not-installed"
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
// its sessions, and whether anything else moved.
type InstallationReport struct {
	State           string `json:"state"`
	Path            string `json:"path,omitempty"`
	Mechanism       string `json:"mechanism,omitempty"`
	Created         bool   `json:"created"`
	SourceUnchanged bool   `json:"source_unchanged"`
	Detail          string `json:"detail,omitempty"`
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

// Continue converts a source session into a new native session of the target harness, installs it, and
// reports conversion, installation and native verification as three separate states.
//
// It is the engine's entry point for goal 0002 and takes no CLI type. Nothing here opens the source for
// writing, and nothing replaces a target session: the identity is fresh, the installation is exclusive,
// and a target harness whose data directory is not there is an error rather than a directory invented
// here.
func Continue(ctx context.Context, opts ContinueOptions) (ContinueResult, error) {
	target := strings.ToLower(strings.TrimSpace(opts.Target))
	if target == "" {
		return ContinueResult{}, fmt.Errorf("%w: choose one (%s)", ErrUnsupportedTarget, TargetCodex)
	}
	if target != TargetCodex {
		return ContinueResult{}, fmt.Errorf("%w: %q (this build writes %s)", ErrUnsupportedTarget, opts.Target, TargetCodex)
	}
	if err := CheckVerifyOptions(opts.Verify, opts.ResumePrompt); err != nil {
		return ContinueResult{}, err
	}
	level := strings.ToLower(strings.TrimSpace(opts.Verify))
	if level == "" {
		level = VerifyLoad
	}

	result := ContinueResult{Ref: opts.Ref, Target: target}

	home, explicitHome, err := ResolveCodexHome(opts.TargetHome)
	if err != nil {
		return ContinueResult{}, err
	}
	// The home is resolved, and checked for shape, before anything else happens; it is *created* only by
	// the installation that would write into it. A missing directory nobody named is still refused.
	homeMissing := false
	if info, err := os.Stat(home); err != nil {
		if !explicitHome {
			return ContinueResult{}, fmt.Errorf(
				"%w at %s: pass --target-home (or set CODEX_HOME) to say where that harness keeps its "+
					"sessions", ErrTargetHomeMissing, home)
		}
		homeMissing = true
	} else if !info.IsDir() {
		return ContinueResult{}, fmt.Errorf("%w: %s is not a directory", ErrTargetHomeMissing, home)
	}

	probe := ProbeCodexBinary(ctx, opts.TargetBinary)
	cfg, reader, err := resolveContinueConfig(opts)
	if err != nil {
		return ContinueResult{}, err
	}
	conversation, readFrom, readWarnings, err := readForConversion(ctx, reader, cfg, opts.Ref, opts.ArchivePath)
	if err != nil {
		return ContinueResult{}, err
	}
	result.Source = new(reportFrom(conversation))
	result.Warnings = append(result.Warnings, readWarnings...)

	// The source's digest is taken before anything is written and compared again afterwards: the
	// property the design asks for is that the transcript a conversation came from is the same file
	// afterwards, whatever the provider's own fingerprint happens to hash.
	sourceDigest, err := sourceFileDigest(conversation, readFrom)
	if err != nil {
		return ContinueResult{}, err
	}

	cliVersion := probe.Version
	cliVersionSource := "probe"
	if cliVersion == "" {
		cliVersionSource = "writer-pin"
	}
	// The target's own configuration answer supplies the provider binding the written session records.
	// Without a harness to ask, the session records none, and the report says so. A dry run does not
	// start the harness against its data directory at all: a harness fixes up its own state in the home
	// it runs with (measured on Codex 0.169.0, whose app server writes its databases, logs and skills
	// there), and a dry run's promise is that it touches that directory nowhere -- so the provider goes
	// unrecorded, with the reason reported, and an installed run records the answer (Q-0002-11).
	configProbe := CodexConfigProbe{}
	if opts.DryRun {
		configProbe.Detail = "a dry run does not start the target harness against its data directory, " +
			"so which model provider a new session of it would use was not asked and none is recorded; " +
			"an installed run records the harness's own answer"
	} else if probe.Found {
		// The provider is asked for as seen from the session's own working directory: the harness
		// resolves project configuration layers from there, and a different directory can answer with a
		// different provider.
		configProbe = ProbeCodexConfig(ctx, home, probe.Binary, conversation.Source.Cwd, opts.Timeout, opts.Stderr)
	}
	// The provider source names where the target's own answer came from (Q-0002-01:
	// target-thread-defaults, target-config, unrecorded); a probe that found no provider records none
	// and says so.
	providerSource := "unrecorded"
	if configProbe.Found && strings.TrimSpace(configProbe.Provider) != "" {
		providerSource = configProbe.Source
	}
	// The printed resume commands name the harness the way the caller asked for it (or "codex"), not the
	// absolute path the probe resolved: a command a person or an agent pastes must be the one they run.
	// They name the data directory too, because a resume command without it resumes the caller's default
	// home rather than the directory this conversion installed into.
	resumeName := strings.TrimSpace(opts.TargetBinary)
	if resumeName == "" {
		resumeName = CodexBinaryDefault
	}
	rollout, err := ConvertToCodexRollout(conversation, CodexTargetOptions{
		CLIVersion:   cliVersion,
		Provider:     configProbe.Provider,
		Now:          opts.Now,
		MaxToolNames: opts.MaxToolNames,
	})
	if err != nil {
		return ContinueResult{}, err
	}
	result.Conversion = ConversionReport{
		State:          ConversionStateConverted,
		Format:         CodexTargetFormat,
		ReadFrom:       readFrom,
		DryRun:         opts.DryRun,
		Carried:        rollout.Carried,
		Losses:         rollout.Losses,
		SourceFidelity: conversation.Fidelity,
	}
	// The invocation is built here, where the session identity exists, and rendered twice: as the
	// argument vector and environment a caller runs, and as the shell line a person pastes.
	resumeArguments := []string{resumeName, "resume", rollout.SessionID}
	execResumeArguments := []string{resumeName, "exec", "resume", rollout.SessionID, resumePromptPlaceholder}
	resumeEnvironment := map[string]string{codexHomeEnv: home}
	result.TargetSession = TargetReport{
		Provider:            target,
		Format:              CodexTargetFormat,
		FormatLimits:        codexFormatLimits(),
		Home:                home,
		Binary:              probe.Binary,
		BinaryFound:         probe.Found,
		BinaryVersion:       probe.Version,
		BinaryDetail:        probe.Detail,
		SessionID:           rollout.SessionID,
		FileName:            rollout.FileName,
		Cwd:                 rollout.Cwd,
		StartedAt:           rollout.StartedAt,
		CLIVersion:          rollout.CLIVersion,
		CLIVersionSource:    cliVersionSource,
		ModelProvider:       configProbe.Provider,
		ProviderSource:      providerSource,
		ProviderDetail:      configProbe.Detail,
		Model:               configProbe.Model,
		ResumeArguments:     resumeArguments,
		ExecResumeArguments: execResumeArguments,
		ResumeEnvironment:   resumeEnvironment,
		ResumeCommand:       renderResumeCommand(resumeArguments, resumeEnvironment),
		ExecResumeCommand:   renderResumeCommand(execResumeArguments, resumeEnvironment),
	}
	verification := VerificationReport{
		Load:   LoadVerification{State: VerificationStateNotRequested, Method: loadMethod},
		Resume: ResumeVerification{State: VerificationStateNotRequested, Method: resumeMethod},
	}

	if opts.DryRun {
		// A dry run touches no target directory: the conversion is verified against a throw-away copy of
		// the same bytes, which is what makes the check comparable with an installed run. The home it
		// would install into is named but not created, and the report says which state that is.
		scratch, err := os.MkdirTemp("", "ccc-dry-run-")
		if err != nil {
			return ContinueResult{}, fmt.Errorf("creating a scratch directory: %w", err)
		}
		defer os.RemoveAll(scratch)
		if _, _, err := InstallCodexRollout(rollout, scratch); err != nil {
			return ContinueResult{}, fmt.Errorf("verifying the conversion: %w", err)
		}
		structure, err := VerifyCodexRolloutStructure(ctx, filepath.Join(scratch, codexSessionsSubdir), rollout, cfg)
		if err != nil {
			return ContinueResult{}, err
		}
		structure.Method = "the inherited AgentsView Codex reader over a throw-away copy (dry run)"
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
		// The dry run writes nothing, so the source file is compared exactly the way an installed run
		// compares it: the report claims the check only when it ran.
		sourceState, sourceDetail, err := sourceUnchangedAfter(conversation, readFrom, sourceDigest)
		if err != nil {
			return ContinueResult{}, err
		}
		result.Installation = InstallationReport{
			State:           InstallationStateNotInstalled,
			Created:         false,
			SourceUnchanged: sourceState,
			Detail:          sourceDetail,
		}
		result.Warnings = append(result.Warnings, sourceChangedWarnings(sourceState)...)
		// A dry run's conversion records no provider because the harness was never started against the
		// home. When there is a harness to ask, an installed run's file differs from this one in that
		// field, and a caller reading only the human block must not read the dry run's file as the one an
		// installed run would write.
		if probe.Found {
			result.Warnings = append(result.Warnings, "a dry run does not start the target harness, so "+
				"the conversion records no model provider; a run that installs asks the harness for its "+
				"own answer")
		}
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
			return ContinueResult{}, fmt.Errorf("creating %s: %w", home, err)
		}
	}
	path, mechanism, err := InstallCodexRollout(rollout, home)
	if err != nil {
		return ContinueResult{}, err
	}
	result.TargetSession.Path = path
	result.Installation = InstallationReport{
		State:           InstallationStateInstalled,
		Path:            path,
		Mechanism:       mechanism,
		Created:         true,
		SourceUnchanged: true,
	}
	sourceState, sourceDetail, err := sourceUnchangedAfter(conversation, readFrom, sourceDigest)
	if err != nil {
		return ContinueResult{}, err
	}
	result.Installation.SourceUnchanged = sourceState
	result.Installation.Detail = sourceDetail
	result.Warnings = append(result.Warnings, sourceChangedWarnings(sourceState)...)

	structure, err := VerifyCodexRolloutStructure(ctx, filepath.Join(home, codexSessionsSubdir), rollout, cfg)
	if err != nil {
		return ContinueResult{}, err
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
		verification.Load = VerifyCodexLoad(ctx, CodexLoadOptions{
			Home:         home,
			Binary:       probe.Binary,
			SessionID:    rollout.SessionID,
			ExpectedPath: path,
			Timeout:      opts.Timeout,
			Stderr:       opts.Stderr,
		})
		verification.Load.BinaryVersion = probe.Version
		if level == VerifyResume {
			verification.Resume = VerifyCodexResume(ctx, CodexResumeOptions{
				Home:        home,
				Binary:      probe.Binary,
				SessionID:   rollout.SessionID,
				Prompt:      opts.ResumePrompt,
				WorkingDir:  rollout.Cwd,
				SessionPath: path,
				Timeout:     opts.Timeout,
				Stderr:      opts.Stderr,
			})
		} else {
			verification.Resume.Detail = "not requested (--verify " + level + ")"
		}
	}
	result.Verification = verification
	if verification.Structure.State != VerificationStateVerified {
		result.Warnings = append(result.Warnings, "the installed session does not hold every piece of "+
			"evidence the conversion wrote: "+verification.Structure.Detail)
	}
	return result, nil
}

const (
	loadMethod   = "the target harness's own loader (codex app-server, thread/resume)"
	resumeMethod = "one turn in the installed session through the target harness's non-interactive resume"
)

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

// sourceFileDigest is the digest of the source session's own file, or "" when the read came from an
// archive: a projection has no file of its own to compare, and the caller says so instead of claiming a
// check that did not run.
func sourceFileDigest(conversation Conversation, readFrom string) (string, error) {
	if readFrom != "source" {
		return "", nil
	}
	return fileDigest(sourceFilePath(conversation))
}

// sourceFilePath is the source session's own file, as the read recorded it.
func sourceFilePath(conversation Conversation) string {
	path := conversation.Source.SourcePath
	if strings.TrimSpace(path) == "" {
		path = conversation.Source.DisplayPath
	}
	return path
}

// sourceUnchangedAfter compares the source file with the digest taken before the conversion, which is
// the property the design asks for: the transcript the conversation came from is the same afterwards.
// The comparison is of the file's own bytes before and after, not against a provider's fingerprint —
// those are different hashes for different readers (a provider may fingerprint a parse or a store
// member), and a check that compared them would report a change that did not happen.
func sourceUnchangedAfter(conversation Conversation, readFrom, before string) (bool, string, error) {
	if readFrom != "source" {
		return true, "the conversion read an archive row, so no source file was open at all", nil
	}
	path := sourceFilePath(conversation)
	if strings.TrimSpace(path) == "" {
		return false, "", errors.New("the source read reported no path, so its digest could not be compared")
	}
	if strings.TrimSpace(before) == "" {
		return false, "", errors.New("the source digest before the conversion was not taken")
	}
	after, err := fileDigest(path)
	if err != nil {
		return false, "", fmt.Errorf("re-reading the source session: %w", err)
	}
	if after != before {
		return false, fmt.Sprintf(
			"the source session changed while it was converted (sha256 %s before, %s after)", before, after), nil
	}
	return true, fmt.Sprintf("sha256 %s, taken before and after the conversion", after), nil
}

// sourceChangedWarnings is the report's warning for a source that moved while it was converted: the
// conversion and the installation are real, but the file no longer holds the bytes they read, and a
// caller that reads only the warnings must not miss it.
func sourceChangedWarnings(sourceUnchanged bool) []string {
	if sourceUnchanged {
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

func probeDetailSuffix(probe CodexProbe) string {
	if strings.TrimSpace(probe.Detail) == "" {
		return ""
	}
	return " (" + probe.Detail + ")"
}
