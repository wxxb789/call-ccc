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

// ErrUnsupportedTarget reports a target harness this build has no writer for.
var ErrUnsupportedTarget = errors.New("unsupported target harness")

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

	// Timeout bounds one verification command. Zero means the per-level default.
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
	Provider          string        `json:"provider"`
	Format            string        `json:"format"`
	FormatLimits      []FormatLimit `json:"format_limits"`
	Home              string        `json:"home"`
	Binary            string        `json:"binary,omitempty"`
	BinaryFound       bool          `json:"binary_found"`
	BinaryVersion     string        `json:"binary_version,omitempty"`
	BinaryDetail      string        `json:"binary_detail,omitempty"`
	SessionID         string        `json:"session_id"`
	FileName          string        `json:"file_name"`
	Path              string        `json:"path,omitempty"`
	Cwd               string        `json:"cwd,omitempty"`
	StartedAt         time.Time     `json:"started_at,omitempty"`
	CLIVersion        string        `json:"cli_version"`
	CLIVersionSource  string        `json:"cli_version_source"`
	ModelProvider     string        `json:"model_provider,omitempty"`
	ProviderSource    string        `json:"model_provider_source,omitempty"`
	ProviderDetail    string        `json:"model_provider_detail,omitempty"`
	Model             string        `json:"model,omitempty"`
	ResumeCommand     string        `json:"resume_command"`
	ExecResumeCommand string        `json:"exec_resume_command"`
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
	level := strings.ToLower(strings.TrimSpace(opts.Verify))
	switch level {
	case "":
		level = VerifyLoad
	case VerifyNone, VerifyLoad, VerifyResume:
	default:
		return ContinueResult{}, fmt.Errorf("unknown verification level %q (none, load, resume)", opts.Verify)
	}
	if level == VerifyResume && strings.TrimSpace(opts.ResumePrompt) == "" {
		return ContinueResult{}, errors.New(
			"a resume verification runs one turn in the installed session, so it needs --resume-prompt: " +
				"call/ccc does not write a prompt of its own into the session")
	}

	result := ContinueResult{Ref: opts.Ref, Target: target}

	home, explicitHome, err := ResolveCodexHome(opts.TargetHome)
	if err != nil {
		return ContinueResult{}, err
	}
	if info, err := os.Stat(home); err != nil {
		if !explicitHome {
			return ContinueResult{}, fmt.Errorf(
				"%w at %s: pass --target-home (or set CODEX_HOME) to say where that harness keeps its "+
					"sessions", ErrTargetHomeMissing, home)
		}
		if err := os.MkdirAll(home, 0o700); err != nil {
			return ContinueResult{}, fmt.Errorf("creating %s: %w", home, err)
		}
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
	// Without a harness to ask, the session records none, and the report says so.
	configProbe := CodexConfigProbe{}
	if probe.Found {
		// The provider is asked for as seen from the session's own working directory: the harness
		// resolves project configuration layers from there, and a different directory can answer with a
		// different provider.
		configProbe = ProbeCodexConfig(ctx, home, probe.Binary, conversation.Source.Cwd, opts.Timeout, opts.Stderr)
	}
	providerSource := "target-defaults"
	if !configProbe.Found || strings.TrimSpace(configProbe.Provider) == "" {
		providerSource = "unrecorded"
	} else if configProbe.Source != "" {
		providerSource = configProbe.Source
	}
	// The printed resume commands name the harness the way the caller asked for it (or "codex"), not the
	// absolute path the probe resolved: a command a person or an agent pastes must be the one they run.
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
		State:          "converted",
		Format:         CodexTargetFormat,
		ReadFrom:       readFrom,
		DryRun:         opts.DryRun,
		Carried:        rollout.Carried,
		Losses:         rollout.Losses,
		SourceFidelity: conversation.Fidelity,
	}
	result.TargetSession = TargetReport{
		Provider:          target,
		Format:            CodexTargetFormat,
		FormatLimits:      codexFormatLimits(),
		Home:              home,
		Binary:            probe.Binary,
		BinaryFound:       probe.Found,
		BinaryVersion:     probe.Version,
		BinaryDetail:      probe.Detail,
		SessionID:         rollout.SessionID,
		FileName:          rollout.FileName,
		Cwd:               rollout.Cwd,
		StartedAt:         rollout.StartedAt,
		CLIVersion:        rollout.CLIVersion,
		CLIVersionSource:  cliVersionSource,
		ModelProvider:     configProbe.Provider,
		ProviderSource:    providerSource,
		ProviderDetail:    configProbe.Detail,
		Model:             configProbe.Model,
		ResumeCommand:     fmt.Sprintf("%s resume %s", resumeName, rollout.SessionID),
		ExecResumeCommand: fmt.Sprintf("%s exec resume %s \"<prompt>\"", resumeName, rollout.SessionID),
	}
	verification := VerificationReport{
		Load:   LoadVerification{State: VerificationStateNotRequested, Method: loadMethod},
		Resume: ResumeVerification{State: VerificationStateNotRequested, Method: resumeMethod},
	}

	if opts.DryRun {
		// A dry run touches no target directory: the conversion is verified against a throw-away copy of
		// the same bytes, which is what makes the check comparable with an installed run.
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
		verification.Load.Detail = "a dry run installs nothing, so the target harness has nothing to load"
		verification.Resume.Detail = "a dry run installs nothing, so no turn can continue from it"
		result.Verification = verification
		result.Installation = InstallationReport{
			State:           "not-installed",
			Created:         false,
			SourceUnchanged: true,
			Detail:          "dry run: the session was converted and verified against a throw-away copy, and nothing was written to " + home,
		}
		return result, nil
	}

	path, mechanism, err := InstallCodexRollout(rollout, home)
	if err != nil {
		return ContinueResult{}, err
	}
	result.TargetSession.Path = path
	result.Installation = InstallationReport{
		State:           "installed",
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
			verification.Resume.State = VerificationStateNotVerified
			verification.Resume.Detail = verification.Load.Detail
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

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// codexFormatLimits states what the target format cannot express at all, so a consumer reads them as
// capabilities of the pair rather than as this run's omissions.
func codexFormatLimits() []FormatLimit {
	return []FormatLimit{
		{
			Category: "attachments",
			Detail: "the source read carries no attachment or image bytes, so a conversion cannot say " +
				"whether a session had any, and this report claims no count either way",
		},
		{
			Category: "provider_and_model",
			Detail: "the written session records no model_provider and no model: the target's own " +
				"configuration supplies them at resume, and a converted session was not produced by a " +
				"model of the target's",
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
