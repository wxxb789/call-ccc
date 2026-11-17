package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// OMPBinaryDefault is the executable an OMP target is resumed with when the caller named none.
const OMPBinaryDefault = "omp"

// ompHomeEnv is the environment variable that selects the OMP data directory a run reads: the harness
// has no command-line flag for the directory itself, so a resume command that does not set it opens the
// caller's own default home rather than the directory the session was installed into.
//
// It is the same name Pi reads (`PI_CODING_AGENT_DIR`), which is OMP's own answer and not an assumption
// borrowed from Pi: measured on 18.18.7, a scratch directory named through it is the one the harness
// writes its sessions into, and its own help page lists it as the session storage directory. Two
// constants with one value, each named by the harness it belongs to, so a route's sentence says which
// harness it is talking about.
const ompHomeEnv = "PI_CODING_AGENT_DIR"

var ompVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

// ompSessionDirEnv is the environment variable that names the directory holding session files. Both the
// printed invocation and the checks set it to the directory the session was installed into
// (ompSessionTree) -- the *project directory* below the data directory's sessions root, because the
// variable names a directory of session files directly: measured on 18.18.7, naming the `sessions` root
// above a session's project directory answers that no such session is installed, while naming the project
// directory resolves it, and naming it is what keeps a caller's leftover value from sending the printed
// line to a tree this session is not in.
//
// It is the same name Pi's route sets, and the two routes set the same *kind* of value: Pi's lookup wants
// the project directory too. Two constants with one value, each named by the harness it belongs to.
const ompSessionDirEnv = "PI_CODING_AGENT_SESSION_DIR"

// ProbeOMPBinary finds the target executable and asks it for its version. A missing binary is an answer,
// not an error: a conversion can still be written (and verified structurally), but the report says the
// native half could not run. Measured: `omp --version` prints `omp/18.18.7`.
func ProbeOMPBinary(ctx context.Context, binary string) harnessProbe {
	return probeBinary(ctx, binary, OMPBinaryDefault, ompVersionFrom, func(path, output string) string {
		return fmt.Sprintf("%s --version answered %q, which names no version",
			path, summariseText(strings.TrimSpace(output), 120))
	})
}

// ompVersionFrom answers the version an `omp --version` line names: the first semantic version on it.
func ompVersionFrom(output string) string {
	return ompVersionPattern.FindString(output)
}

// ompLoadEnvDenied are the environment variables the load check removes before it starts the harness's
// own loader: the variables that point a run at a model or carry model credentials. The scratch data
// directory the check builds defines its provider with `auth: none`, so the loader resolves the session
// without reaching a provider at all -- and removing these is what makes that a measurement rather than
// a hope on a host that happens to hold a key.
var ompLoadEnvDenied = []string{
	"PI_API_KEY",
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_OAUTH_TOKEN",
	"OPENAI_API_KEY",
	"OPENROUTER_API_KEY",
	"GOOGLE_API_KEY",
	"GEMINI_API_KEY",
	"XAI_API_KEY",
	"DEEPSEEK_API_KEY",
	"GROQ_API_KEY",
	"MISTRAL_API_KEY",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AZURE_OPENAI_API_KEY",
	// The built-in provider OMP reads its own gateway settings from, and the variable the host that ran
	// this increment happens to hold its gateway key in: both are named so a run cannot quietly bill a
	// turn while the check claims it made none.
	"LITELLM_API_KEY",
	"LITELLM_BASE_URL",
}

// OMPLoadOptions configures one native load verification.
type OMPLoadOptions struct {
	Binary    string
	SessionID string
	Cwd       string
	// Directory and FileName are the project directory and file name the session occupies, which the
	// scratch copy reproduces so the harness resolves it exactly as it would the installed one.
	Directory string
	FileName  string
	Data      []byte
	Installed string
	Timeout   time.Duration
	Stderr    io.Writer
}

// VerifyOMPLoad asks the target harness's own loader to open a copy of the converted session and report
// the conversation it built, with no model turn.
//
// The check installs a throw-away copy of the converted bytes into a scratch data directory and starts
// `omp --mode rpc` there with the environment variables that could reach a model removed. Two facts
// about this harness decide the shape of the check, and both are measured:
//
//   - the loader *writes* into the session it opens (it appends a `custom` session_exit record), so
//     driving it against the installed file would alter what the conversion published: the copy is what
//     it is given, and the installed file's digest is compared before and after;
//   - the harness refuses to start at all in a data directory that defines no model, so the scratch home
//     carries a provider definition built from the converted file's *own* records -- the provider and
//     model the session names -- with `auth: none`, because the check must need no credential to prove a
//     load. The provider's model is the session's own binding, which is also what the harness restores a
//     session's model from; a session whose records name none is reported as one this check cannot
//     install a home for rather than as a pass.
//
// It then asks the harness two questions: `get_state`, whose own answer must name this session, the
// number of messages the file holds and a session file that is the copy, and `get_messages`, whose roles
// must be the ones the conversion wrote, in order. Finally it runs the same first question with a
// freshly generated id nothing installed under: that control arm must make the harness answer that the
// session was not found, which is what makes the first arm an answer about *this* session rather than
// about a process that started.
//
// A protocol or version change that makes the harness refuse the file is reported as not-verified with
// the harness's own words, never as a pass.
func VerifyOMPLoad(ctx context.Context, opts OMPLoadOptions) LoadVerification {
	verification := LoadVerification{
		State:     VerificationStateNotVerified,
		Method:    loadMethodOMP,
		SessionID: opts.SessionID,
		Path:      opts.Installed,
		Cwd:       opts.Cwd,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = OMPBinaryDefault
	}
	path, err := resolveOMPExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf(
			"%s is not on PATH, so the installed session's native load could not be verified", binary)
		return verification
	}
	verification.Binary = path

	provider, model, ok := ompSessionBinding(opts.Data)
	if !ok {
		verification.Detail = "the converted session names no model and no provider in its own records, " +
			"and this harness refuses to start in a data directory that defines no model: the load could " +
			"not be verified against a home built for it"
		return verification
	}

	before, beforeErr := installedDigest(opts.Installed)

	scratch, err := os.MkdirTemp("", "ccc-omp-load-")
	if err != nil {
		verification.Detail = fmt.Sprintf("a scratch data directory could not be created for the check: %v", err)
		return verification
	}
	defer os.RemoveAll(scratch)
	if err := writeOMPScratchHome(scratch, provider, model); err != nil {
		verification.Detail = fmt.Sprintf("the check's own data directory could not be written: %v", err)
		return verification
	}
	copyPath, _, err := InstallOMPSession(PiSession{
		Directory: opts.Directory,
		FileName:  opts.FileName,
		Data:      opts.Data,
	}, scratch)
	if err != nil {
		verification.Detail = fmt.Sprintf("the check's own copy of the session could not be written: %v", err)
		return verification
	}
	// The sessions tree the invocation names: the project directory below the scratch root the copy was
	// installed into, which is where the installer files a session and what the printed line names.
	copySessionDir := filepath.Join(scratch, ompSessionsSubdir, opts.Directory)
	loaderCwd, cwdRefusal := ompLoaderCwd(scratch, opts.Cwd)
	if cwdRefusal != nil {
		verification.Detail = cwdRefusal.Error()
		return verification
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = ompLoadTimeout
	}
	state, err := ompAskLoader(ctx, path, scratch, copySessionDir, loaderCwd, opts.SessionID, opts.Data, timeout, opts.Stderr)
	if err != nil {
		verification.Detail = fmt.Sprintf(
			"the harness's own loader did not answer for session %s: %v", opts.SessionID, err)
		return verification
	}
	if state.SessionID != opts.SessionID {
		verification.Detail = fmt.Sprintf(
			"the harness answered about session %q, not the installed %q", state.SessionID, opts.SessionID)
		return verification
	}
	if want := piExpectedMessageCount(opts.Data); want > 0 && state.MessageCount != want {
		verification.Detail = fmt.Sprintf(
			"the harness loaded session %s with %d message(s), while the session file holds %d: the "+
				"conversation the loader built is not the one this conversion wrote",
			opts.SessionID, state.MessageCount, want)
		return verification
	}
	if state.SessionFile != "" && !sameFile(state.SessionFile, copyPath) {
		verification.Detail = fmt.Sprintf(
			"the harness answered from %s, not from the copy this check installed at %s",
			state.SessionFile, copyPath)
		return verification
	}
	// The control arm: the same question about an identity that exists nowhere. Its answer is what turns
	// "the harness started" into "the harness resolved this session".
	unknown, err := newUUIDV4()
	if err != nil {
		verification.Detail = fmt.Sprintf("a control identity could not be generated: %v", err)
		return verification
	}
	_, controlErr := ompAskLoader(ctx, path, scratch, copySessionDir, loaderCwd, unknown, nil, timeout, opts.Stderr)
	if controlErr == nil {
		verification.Detail = fmt.Sprintf(
			"the control arm answered for session %q, which this check never installed, so the loader is "+
				"not answering from the installed file", unknown)
		return verification
	}
	if !strings.Contains(controlErr.Error(), ompMissingSessionMarker) {
		verification.Detail = fmt.Sprintf(
			"the control arm did not report the unknown session as missing; it answered: %s",
			summariseText(controlErr.Error(), 300))
		return verification
	}
	planted, plantedErr := piSessionNamed(filepath.Join(scratch, ompSessionsSubdir), unknown)
	if plantedErr != nil {
		verification.Detail = fmt.Sprintf(
			"the control arm did not report the unknown session as missing from a tree this check could "+
				"not read: %v", plantedErr)
		return verification
	}
	if planted != "" {
		verification.Detail = fmt.Sprintf(
			"the control arm wrote a session file for the unknown id %s (%s), so the missed lookup did "+
				"not stop before a new session was created", unknown, planted)
		return verification
	}

	if beforeErr != nil {
		verification.Detail = fmt.Sprintf(
			"the harness's loader resolved the installed session %s and the installed file could not be "+
				"read to confirm the check left it alone: %v", opts.SessionID, beforeErr)
		return verification
	}
	after, afterErr := installedDigest(opts.Installed)
	if afterErr != nil || after != before {
		verification.Detail = fmt.Sprintf(
			"the harness's loader resolved the installed session %s, and the check cannot report the "+
				"installed file as unchanged afterwards (%v)", opts.SessionID, afterErr)
		return verification
	}

	verification.State = VerificationStateVerified
	verification.Detail = fmt.Sprintf(
		"the harness's own loader read the session and answered about it (%s); the same question with an "+
			"unknown session id was answered \"%s\". The check ran against a throw-away copy of the "+
			"converted bytes (%s) in a scratch data directory whose provider (%s/%s) needs no credential, "+
			"with %s removed from the environment, so the loader read the session without a model turn, "+
			"and the installed file is unchanged (%s)",
		stateSummary(state), ompMissingSessionMarker, copyPath, provider, model,
		strings.Join(ompLoadEnvDenied, ", "), shortDigest(before))
	return verification
}

// ompMissingSessionMarker is the sentence OMP's own lookup answers with when an id it was asked to
// resume is installed nowhere: `Error: Session "<id>" not found.` (measured). The control arm reads it
// rather than the exit status, because both arms exit non-zero when the harness cannot start at all.
const ompMissingSessionMarker = "not found"

// ompLoadTimeout bounds one load check arm. The harness starts a full runtime, so this is more generous
// than Pi's: measured, an RPC run answers a written session in a few seconds, and a harness that cannot
// answer is stopped by this deadline rather than by the caller's patience.
const ompLoadTimeout = 120 * time.Second

// ompCheckFileMode is the mode the check asks the OS to give the two configuration files it writes into
// the scratch data directory it made itself: readable and writable by its owner, by nobody else.
const ompCheckFileMode os.FileMode = 0o600

// ompAskLoader starts the harness in its own RPC mode against the scratch data directory, asks it for the
// state of one session and for that session's messages, and answers what its own loader said.
//
// The session is resolved by its own id through the directory it was filed in: measured, OMP's lookup
// finds a session by id whatever the process's working directory is, so the arm does not depend on
// starting from the session's own directory (the printed resume command still does, because that is where
// a caller wants to continue the work) -- but it does depend on naming that directory, because the
// sessions-tree variable names the directory that holds session files *directly*: naming the `sessions`
// root above a session's project directory answers that no such session is installed (measured), and
// naming the project directory resolves it. The check names the same directory the printed line does, so a
// caller's leftover setting cannot send the printed line somewhere this check never looked.
func ompAskLoader(
	ctx context.Context, binary, home, sessionDir, cwd, sessionID string, data []byte,
	timeout time.Duration, stderr io.Writer,
) (piLoadState, error) {
	arguments := []string{
		"--mode", "rpc",
		"--no-tools",
		"--resume=" + sessionID,
	}
	stdin := strings.Join([]string{
		`{"id":"state","type":"get_state"}`,
		`{"id":"messages","type":"get_messages"}`,
	}, "\n") + "\n"
	responses, err := runOMPLoaderRPC(ctx, binary, cwd, home, sessionDir, arguments, stdin, timeout, stderr)
	if err != nil {
		return piLoadState{}, err
	}
	state, err := readPiState(responses)
	if err != nil {
		return piLoadState{}, err
	}
	if len(data) > 0 {
		if detail, err := piMessageDetail(responses, data); err != nil {
			return piLoadState{}, err
		} else if detail != "" {
			return piLoadState{}, errors.New(detail)
		}
	}
	return state, nil
}

// runOMPRPC starts one OMP RPC run with the check's own environment: the caller's own with the data
// directory set, the sessions tree named as the directory the session is filed in, and the model
// variables removed.
func runOMPRPC(
	ctx context.Context, binary, cwd, home, sessionDir string, arguments []string, stdin string,
	timeout time.Duration, stderr io.Writer,
) ([]piRPCResponse, error) {
	return runHarnessRPC(ctx, binary, cwd, arguments, stdin,
		ompEnvironment(home, sessionDir, ompLoadEnvDenied), timeout, stderr)
}

// runOMPLoaderRPC is the seam the load check drives the harness's RPC mode through: the rule under test
// is what the check makes of the harness's answer, not which build of the harness a host has installed,
// so a test answers the protocol itself rather than the check standing or falling on the host's harness.
var runOMPLoaderRPC = runOMPRPC

// ompLoaderCwd answers the working directory the load check's harness process runs in, or the reason the
// check cannot run at all.
//
// The harness resolves a session by id from any directory (measured), *and* refuses a session whose
// recorded working directory it cannot find: run against a session recording a path this host does not
// have, it answers `Error: Session "<id>" belongs to a directory that no longer exists (<path>); run
// interactively to move it into the current project.` and exits. So the session's own recorded directory
// is used when it is a directory here -- which is where the printed resume command runs the harness -- and
// a recorded directory this host does not have is answered as the refusal it is, naming the harness's own
// sentence, rather than as a check that quietly ran somewhere else and failed. A recording that names no
// directory at all is not this case: the session's file is the check's, it was written by this build, and
// the run uses the scratch data directory it was installed under.
func ompLoaderCwd(scratch, sessionCwd string) (string, error) {
	trimmed := strings.TrimSpace(sessionCwd)
	if trimmed == "" {
		return scratch, nil
	}
	info, err := os.Stat(trimmed)
	if err == nil && info.IsDir() {
		return trimmed, nil
	}
	return "", fmt.Errorf(
		"the installed session records the working directory %s, which is not a directory on this host, "+
			"and the harness refuses a session whose recorded directory it cannot find (measured on "+
			"18.18.7: `Error: Session \"<id>\" belongs to a directory that no longer exists (<dir>); run "+
			"interactively to move it into the current project.`), so the native load was not verified "+
			"here: the session is a conversion of a source that recorded that directory, and its load is "+
			"verifiable on a host that has it", trimmed)
}

// ompSessionBinding answers the provider and model the converted file's own records name: the session's
// `model_change` binding when it holds one, else the pair the last assistant record carries. Both are
// what the harness reads a session's model from, in that order, so a scratch home built from this answer
// is one the load can start in.
func ompSessionBinding(data []byte) (string, string, bool) {
	entries := piConversationEntries(data)
	provider, model := "", ""
	for _, entry := range entries {
		if entry.Type == piEntryModelChange {
			recorded := strings.TrimSpace(entry.Model)
			if slash := strings.Index(recorded, "/"); slash > 0 && slash < len(recorded)-1 {
				return recorded[:slash], recorded[slash+1:], true
			}
			continue
		}
		if entry.Type != piEntryMessage || strings.TrimSpace(entry.Message.Role) != piRoleAssistant {
			continue
		}
		if strings.TrimSpace(entry.Message.Provider) != "" && strings.TrimSpace(entry.Message.Model) != "" {
			provider, model = strings.TrimSpace(entry.Message.Provider), strings.TrimSpace(entry.Message.Model)
		}
	}
	if provider == "" || model == "" {
		return "", "", false
	}
	return provider, model, true
}

// writeOMPScratchHome writes the smallest data directory the harness will start in: a provider the
// converted session's own records name, declared with `auth: none` and one model (that session's), and
// the model role that selects it. Measured on 18.18.7: a data directory with no model refuses to start
// at all (`No default model selected`), and this definition is accepted without a credential anywhere.
//
// The definition is the check's own and holds no endpoint it would call: no turn is run here, and the
// credential-bearing settings of a caller's own home are neither read nor copied.
func writeOMPScratchHome(home, provider, model string) error {
	models := fmt.Sprintf(`providers:
  %s:
    baseUrl: https://example.invalid/v1
    api: openai-completions
    auth: none
    models:
    - id: %s
      name: %s
      reasoning: false
      input: [text]
      contextWindow: 1000
      maxTokens: 100
`, ompYAMLScalar(provider), ompYAMLScalar(model), ompYAMLScalar(model))
	if err := os.WriteFile(filepath.Join(home, "models.yml"), []byte(models), ompCheckFileMode); err != nil {
		return err
	}
	config := fmt.Sprintf("modelRoles:\n  default: %s\n", ompYAMLScalar(provider+"/"+model))
	return os.WriteFile(filepath.Join(home, "config.yml"), []byte(config), ompCheckFileMode)
}

// ompYAMLScalar spells one value for the check's own YAML, whose schema is small and known: a value that
// is a plain scalar in YAML's own rules is written as it is, and anything else is written as a
// double-quoted JSON string, which is a valid YAML scalar and is what keeps an exotic provider or model
// id from changing the document's shape.
func ompYAMLScalar(value string) string {
	if value != "" && !strings.ContainsAny(value, ":#{}[],&*!|>'\"%@`\n\t ") {
		return value
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// OMPResumeOptions configures one real resume turn.
type OMPResumeOptions struct {
	Home      string
	Binary    string
	SessionID string
	// ProjectDir is the project directory below Home's sessions tree that the session was installed
	// under, which is the directory the invocation names (ompResumeEnvironment): the check runs the
	// printed environment, so a lookup the printed line would not make cannot verify a session the
	// printed line cannot find.
	ProjectDir string
	Cwd        string
	// SessionPath is the installed file, whose growth the turn is reported by.
	SessionPath string
	Prompt      string
	Timeout     time.Duration
	Stderr      io.Writer
}

// ompResumeArguments is the one non-interactive resume invocation: print mode, the session's id, the
// option terminator and the turn's words. Both the line the report prints and the check that runs a real
// turn call it, so the invocation a caller pastes is the one that was verified. Print mode is what makes
// the line non-interactive, and a resume that names no session would open the harness's picker.
func ompResumeArguments(sessionID, prompt string) []string {
	return []string{"-p", "--resume", sessionID, promptTerminator, prompt}
}

// ompResumeEnvironment is the environment both the printed resume line and the resume check run the
// harness with: the data directory the session was installed into, and the directory inside it that holds
// this session's file.
//
// Both halves are explicit for the same reason Pi's route names both, and the second one has its own
// measured rule: OMP's sessions-tree variable names the directory that holds session files *directly*, so
// naming the `<home>/sessions` root above a session's project directory answers that no such session is
// installed, while naming the project directory itself resolves it -- and naming it is also what keeps a
// caller's leftover setting of that variable (a Pi user's, most obviously) from sending the printed line
// to a tree this session is not in.
func ompResumeEnvironment(home, projectDir string) map[string]string {
	return map[string]string{
		ompHomeEnv:       home,
		ompSessionDirEnv: ompSessionTree(home, projectDir),
	}
}

// ompSessionTree is the directory one converted session is filed in and looked up through: the session's
// project directory below the data directory's own `sessions` root. The installer and both halves of the
// invocation name this one spelling, so the printed line, the check and the file agree.
func ompSessionTree(home, projectDir string) string {
	return filepath.Join(home, ompSessionsSubdir, projectDir)
}

// ompEnvironment builds a child environment for one harness run: the caller's own environment with the
// data directory set to home, the sessions tree named as the directory the session is filed in, the named
// variables removed, and the model-reaching names the check denies removed too.
func ompEnvironment(home, sessionDir string, denied []string) []string {
	return childEnvironment(home, ompHomeEnv, denied, map[string]string{ompSessionDirEnv: sessionDir})
}

// VerifyOMPResume runs one real turn in the installed session and reports what the harness did.
//
// It is the only state that proves a session continues: the invocation is the one the report prints, run
// from the session's own working directory, with the caller's own environment, so the turn's model access
// is whatever the caller has configured -- this check never supplies credentials of its own. A missing
// binary, a harness that refuses the session and a harness that cannot reach a model are each reported as
// not-verified with the harness's own words. A harness that answers but leaves the installed session's
// bytes unchanged is refused the same way: the verified state says the harness wrote its own records into
// the session, and the two sizes it reports have to show it.
func VerifyOMPResume(ctx context.Context, opts OMPResumeOptions) ResumeVerification {
	verification := ResumeVerification{
		State:  VerificationStateNotVerified,
		Method: resumeMethodOMP,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = OMPBinaryDefault
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		verification.Detail = "no resume prompt was given, and call/ccc does not invent one: it becomes part of the session"
		return verification
	}
	path, err := resolveOMPExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so no turn could be run in the installed session", binary)
		return verification
	}
	arguments := ompResumeArguments(opts.SessionID, opts.Prompt)
	verification.Command = renderInvocation(append([]string{path}, arguments...),
		ompResumeEnvironment(opts.Home, opts.ProjectDir), opts.Cwd)

	before, beforeErr := fileSize(opts.SessionPath)
	if beforeErr != nil {
		verification.Detail = fmt.Sprintf("the installed session %s could not be measured before the turn: %v",
			opts.SessionPath, beforeErr)
		return verification
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = ompResumeTimeout
	}
	answer, diagnostics, runErr := runOMPResumeTurn(ctx, path, opts.Cwd, opts.Home,
		ompSessionTree(opts.Home, opts.ProjectDir), arguments, timeout, opts.Stderr)
	after, afterErr := fileSize(opts.SessionPath)
	verification.SessionBytesBefore = before
	verification.SessionBytesAfter = after
	growthDetail := turnGrowthDetail(before, after)
	if afterErr != nil {
		growthDetail = fmt.Sprintf(" (the installed session could not be measured after the turn: %v)", afterErr)
	}
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	verification.ExitCode = &exitCode
	verification.Output, verification.OutputTruncated = boundedText(strings.TrimSpace(answer), 0)
	if runErr != nil {
		reason := strings.TrimSpace(answer)
		if reason == "" {
			reason = strings.TrimSpace(diagnostics)
		}
		verification.Detail = fmt.Sprintf("the harness did not answer: %s (exit status %d)%s",
			summariseText(reason, 300), exitCode, growthDetail)
		return verification
	}
	if strings.TrimSpace(answer) == "" {
		verification.Detail = fmt.Sprintf("the harness exited successfully and reported no answer%s", growthDetail)
		return verification
	}
	if afterErr != nil {
		verification.Detail = "the turn ran, but the installed session could not be measured afterwards: " + afterErr.Error()
		return verification
	}
	if after <= before {
		verification.Detail = fmt.Sprintf(
			"the harness answered from the installed session in one turn, but the session file did not "+
				"grow (%d bytes before and after), so the turn was not appended to the installed session", before)
		return verification
	}
	verification.State = VerificationStateVerified
	verification.Detail = fmt.Sprintf(
		"the harness answered from the installed session in one turn and wrote its own records into it%s",
		turnGrowthDetail(before, after))
	return verification
}

// ompResumeTimeout bounds one real resume turn: a turn is a model call in the caller's own session, so it
// is given a generous deadline rather than the load check's.
const ompResumeTimeout = 5 * time.Minute

// runOMPTurn runs one non-interactive harness invocation and returns its stdout and stderr separately,
// both bounded. The command runs in the session's working directory when it exists, because that is where
// the printed invocation runs it.
func runOMPTurn(
	ctx context.Context, executable, cwd, home, sessionDir string, args []string,
	timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	return runRouteTurn(ctx, executable, cwd, args, ompEnvironment(home, sessionDir, nil), timeout, stderr)
}

// runOMPResumeTurn is the seam the resume check drives the target harness's turn through: what the check
// makes of the harness's answer is the rule under test, not which build of the harness a host happens to
// have installed.
var runOMPResumeTurn = runOMPTurn

// resolveOMPExecutable is the seam the two checks find the target harness through, named for the same
// reason the other routes' seams are: which build of the harness a host happens to have installed is not
// the rule under test.
var resolveOMPExecutable = resolveTargetExecutable

// OMPModelProbe is what the installed OMP harness answered about the model it resolves for a new session
// of itself: the provider label and the model id, or why it could not answer. It is the target's own
// answer -- asked of the harness rather than read out of its configuration file -- and it is what the
// route writes into the session's model binding and onto every assistant record.
type OMPModelProbe struct {
	Found    bool   `json:"found"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Source   string `json:"source,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ProbeOMPModel asks the installed harness which model a new session of it would run under.
//
// The question is the protocol's own `get_state` on a run that resumes nothing, with `--no-session` so the
// probe leaves no session behind (measured: the harness answers the resolved model -- provider and id --
// on such a run, and `--no-session` writes none), and with the data directory the conversion will install
// into, so the answer is the one that home resolves rather than this host's default. A home with no model
// configured answers nothing: measured, the harness refuses to start at all there (`No default model
// selected`), which is reported as the detail rather than as an empty answer.
func ProbeOMPModel(
	ctx context.Context, home, binary string, timeout time.Duration, stderr io.Writer,
) OMPModelProbe {
	probe := OMPModelProbe{}
	if strings.TrimSpace(binary) == "" {
		binary = OMPBinaryDefault
	}
	path, err := resolveOMPExecutable(binary)
	if err != nil {
		probe.Detail = fmt.Sprintf("%s is not on PATH, so its configuration could not be asked for", binary)
		return probe
	}
	if timeout <= 0 {
		timeout = ompProbeTimeout
	}
	responses, err := runOMPLoaderRPC(ctx, path, "", home, "",
		[]string{"--mode", "rpc", "--no-tools", "--no-session"},
		`{"id":"model","type":"get_state"}`+"\n", timeout, stderr)
	if err != nil {
		probe.Detail = "the harness did not answer get_state: " + summariseText(err.Error(), 300)
		return probe
	}
	identity, err := ompResolvedModel(responses)
	if err != nil {
		probe.Detail = err.Error()
		return probe
	}
	probe.Provider = strings.TrimSpace(identity.Provider)
	probe.Model = strings.TrimSpace(identity.ID)
	if probe.Provider == "" || probe.Model == "" {
		probe.Detail = "the harness answered get_state without a resolved model, so this home names no " +
			"model a new session of it would run under"
		return probe
	}
	probe.Found = true
	probe.Source = "the harness's own get_state answer"
	probe.Detail = fmt.Sprintf("the harness resolved `%s/%s` for a new session of it",
		probe.Provider, probe.Model)
	return probe
}

// ompProbeTimeout bounds one model probe. Measured: the answer arrives in about a second, and a harness
// that cannot answer is stopped by this deadline rather than by the caller's patience.
const ompProbeTimeout = 90 * time.Second

// ompResolvedModel answers the model identity a `get_state` answer names, or an error describing what the
// harness said instead.
func ompResolvedModel(responses []piRPCResponse) (piModelIdentity, error) {
	for _, response := range responses {
		if response.Command != "get_state" {
			continue
		}
		if !response.Success {
			return piModelIdentity{}, fmt.Errorf("the harness refused get_state: %s",
				summariseText(response.Error, 200))
		}
		return response.Data.Model, nil
	}
	return piModelIdentity{}, errors.New("the harness answered no get_state response")
}
