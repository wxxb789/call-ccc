package ccc

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
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

// The OpenCode route's native verification: a probe of the installed binary, the harness's own answer
// about the agent and model a new session of it would run under (the facts the writer records), the
// harness's own reader answering about the installed session (the load level, which bills nothing), and
// one real turn through the printed invocation (the resume level). Every fact about the harness's answers
// is measured and recorded in docs/evidence/0007-opencode-target-continuation.md:
//
//   - `opencode --version` answers a plain version line;
//   - `opencode debug agents --standalone` answers its own agent list as JSON, the first primary,
//     non-hidden entry being the default agent (`build` with the owner's configuration and with an empty
//     one, its model null when no model is configured);
//   - `opencode session export <id> --standalone` answers the session's own document and exits 1 with
//     `Session not found: <id>` for an id the store does not hold; the command changes none of the
//     session's own records (measured: identical rows and identical exported documents around it, while
//     the database *file's* bytes move with its WAL handling -- which is why this level, unlike the Pi
//     and OMP loads, may answer from the installed home);
//   - `opencode run --session <id>` runs a turn, appends its own records (a user message and an idle
//     marker even when the turn fails), and *creates* a session when the id is unknown -- which is why
//     the load level uses `session export` and its control arm asks about an id nothing installed.

// OpenCodeBinaryDefault is the executable name the printed resume commands use when the caller named
// none.
const OpenCodeBinaryDefault = "opencode"

// openCodeProbeTimeout bounds one question asked of the harness (its agent listing, its session export).
// Measured: each answers in about a second, and a harness that cannot answer is stopped by this deadline
// rather than by the caller's patience.
const openCodeProbeTimeout = 90 * time.Second

// openCodeResumeTimeout bounds one real resume turn: a turn is a model call in the caller's own session,
// so it is given a generous deadline rather than the load check's.
const openCodeResumeTimeout = 5 * time.Minute

// openCodeUnknownSessionID is the id the load check's control arm asks about: an id in the harness's own
// spelling that no import of this build mints (its tail is not the shape a fresh identity has, and the
// answer is required to name the id it could not find).
const openCodeUnknownSessionID = "ses_unknown_control_0000000000"

// openCodeLoadEnvDenied are the environment names the load check removes on top of the database override:
// the caller's inline or named configuration could define providers, and a credential name the harness's
// built-in providers read must not reach this level. The load is a read that bills nothing; removing them
// is what makes that a property of the check rather than of the harness's behaviour.
var openCodeLoadEnvDenied = []string{
	"OPENCODE_API_KEY",
	"OPENCODE_CONFIG_CONTENT",
	"OPENCODE_CONFIG_DIR",
	"OPENCODE_PASSWORD",
	"OPENCODE_SERVER_PASSWORD",
}

var openCodeVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

// ProbeOpenCodeBinary locates the installed harness and asks it for its version, the shape every route's
// probe shares.
func ProbeOpenCodeBinary(ctx context.Context, binary string) harnessProbe {
	return probeBinary(ctx, binary, OpenCodeBinaryDefault,
		func(output string) string { return openCodeVersionPattern.FindString(output) },
		func(path, output string) string {
			return fmt.Sprintf("%s --version printed no version: %s", path, summariseText(output, 200))
		})
}

// openCodeAgentProbe is what the installed harness answered about the agent and model a new session of it
// would run under: its own primary agent's id and that agent's resolved model, or why it could not
// answer. It is the target's own answer -- asked of the harness (`debug agents`) rather than read out of
// its configuration file -- and it is what the route writes into every assistant message's agent member
// and into the session's own model binding.
type openCodeAgentProbe struct {
	Agent    string
	Provider string
	Model    string
	Variant  string
	Found    bool
	Source   string
	Detail   string
}

// openCodeAgentAnswer is one entry of the harness's own agent listing, as far as this probe reads it.
type openCodeAgentAnswer struct {
	ID     string         `json:"id"`
	Mode   string         `json:"mode"`
	Hidden bool           `json:"hidden"`
	Model  *openCodeModel `json:"model"`
}

// ProbeOpenCodeAgents asks the installed harness which agent and model a new session of it would run
// under: the first entry of its own agent list whose mode is `primary` and which is not hidden -- the
// agent the harness runs a session with when the session selects none, measured to be `build` both with
// the owner's configuration and with an empty one -- and that entry's resolved model, where the home
// resolves one.
//
// The question is asked with the data directory this conversion will install into, because that is the
// directory the printed resume command reads. A home that does not exist yet is not created for the
// question: a throw-away data root is used instead, and the harness's *configuration* -- which is what
// the answer is about -- is the same either way.
func ProbeOpenCodeAgents(
	ctx context.Context, home, binary string, timeout time.Duration, stderr io.Writer,
) openCodeAgentProbe {
	probe := openCodeAgentProbe{}
	path, err := resolveOpenCodeExecutable(binary)
	if err != nil {
		probe.Detail = fmt.Sprintf("%s is not on PATH, so its configuration could not be asked for", binary)
		return probe
	}
	if timeout <= 0 {
		timeout = openCodeProbeTimeout
	}
	environment, cleanup, err := openCodeProbeEnvironment(home)
	if err != nil {
		probe.Detail = "the question could not be asked without creating the home: " + err.Error()
		return probe
	}
	defer cleanup()
	arguments := []string{"debug", "agents", "--standalone"}
	output, diagnostics, err := runOpenCodeAgentsProbe(ctx, path, "", arguments, environment, timeout, stderr)
	if err != nil {
		probe.Detail = "the harness did not answer its own agent listing: " +
			summariseText(firstNonEmptyText(output, diagnostics, err.Error()), 300)
		return probe
	}
	agent, ok := openCodePrimaryAgent(output)
	if !ok {
		probe.Detail = "the harness's own agent listing names no primary agent, so this home names no " +
			"agent a new session of it would run under"
		return probe
	}
	probe.Agent = strings.TrimSpace(agent.ID)
	probe.Found = true
	probe.Source = "the harness's own debug agents answer"
	if agent.Model == nil {
		probe.Detail = fmt.Sprintf("the harness's own agent listing names `%s` as its primary agent and "+
			"resolves no model for it, so this home names no model a new session of it would run under",
			probe.Agent)
		return probe
	}
	probe.Provider = strings.TrimSpace(agent.Model.ProviderID)
	probe.Model = strings.TrimSpace(agent.Model.ID)
	probe.Variant = strings.TrimSpace(agent.Model.Variant)
	if probe.Provider == "" || probe.Model == "" {
		probe.Detail = fmt.Sprintf("the harness's own agent listing names `%s` as its primary agent and "+
			"resolves no complete model for it, so this home names no model a new session of it would "+
			"run under", probe.Agent)
		return probe
	}
	probe.Detail = fmt.Sprintf("the harness resolved `%s/%s` for its primary agent `%s`",
		probe.Provider, probe.Model, probe.Agent)
	return probe
}

// openCodePrimaryAgent answers the entry of the harness's own agent listing that a new session runs
// under: the first primary, non-hidden agent.
func openCodePrimaryAgent(output string) (openCodeAgentAnswer, bool) {
	var agents []openCodeAgentAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &agents); err != nil {
		return openCodeAgentAnswer{}, false
	}
	for _, agent := range agents {
		if agent.Hidden || strings.TrimSpace(agent.Mode) != "primary" {
			continue
		}
		if strings.TrimSpace(agent.ID) == "" {
			continue
		}
		return agent, true
	}
	return openCodeAgentAnswer{}, false
}

// openCodeProbeEnvironment builds the environment the agent question is asked under: the environment the
// conversion's own commands run with when the harness would find a data directory there, and a
// throw-away data root otherwise, so asking about a home that does not exist yet cannot create one. The
// cleanup removes the throw-away root.
func openCodeProbeEnvironment(home string) ([]string, func(), error) {
	if info, err := os.Stat(openCodeDataDir(home)); err == nil && info.IsDir() {
		return openCodeEnvironment(home, nil), func() {}, nil
	}
	scratch, err := os.MkdirTemp("", "ccc-opencode-probe-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("creating a scratch data root: %w", err)
	}
	scratchHome := filepath.Join(scratch, openCodeDataDirName)
	return openCodeEnvironment(scratchHome, nil), func() { os.RemoveAll(scratch) }, nil
}

// openCodeFacts records which version the document is written as -- the probed one when the installed
// binary answered, else the pinned version this writer's shape was verified against -- and asks the
// harness itself which agent and model a new session of it would run under.
//
// The answer is not a convenience: the harness's assistant records must name an agent and a model
// reference, and the session's own model binding is what a resumed turn runs under, so the answer is
// what makes a converted session openable and resumable there. It is asked of the harness rather than
// read out of its configuration file, and a dry run does not ask it at all -- starting the harness
// against a caller's data directory is exactly the sort of write a preview must not perform.
func openCodeFacts(ctx context.Context, in targetFactsInput) targetFacts {
	facts := targetFacts{CLIVersion: OpenCodeFormatVersion, CLIVersionSource: "writer-pin"}
	if in.Probe.Version != "" {
		facts.CLIVersion = in.Probe.Version
		facts.CLIVersionSource = "probe"
	}
	probe := openCodeAgentProbe{}
	switch {
	case in.DryRun:
		probe.Detail = "a dry run does not ask the harness's own agent listing, so the agent and model a " +
			"new session of it would run under were not asked and none is recorded"
	case in.Probe.Found:
		probe = ProbeOpenCodeAgents(ctx, in.Home, in.Probe.Binary, in.Timeout, in.Stderr)
	default:
		probe.Detail = in.Probe.Detail + ", so the agent and model a new session of it would run under " +
			"were not asked and none is recorded"
	}
	facts.Agent = probe.Agent
	facts.ModelProvider = probe.Provider
	facts.Model = probe.Model
	facts.ModelVariant = probe.Variant
	facts.ProviderDetail = probe.Detail
	facts.ProviderSource = "unrecorded"
	if probe.Found {
		facts.ProviderSource = probe.Source
	}
	return facts
}

// openCodeExportSummary is what this route asks the harness's own session document about: the session's
// identity, the directory the harness files it under, and how many messages the document holds. Nothing
// else is read. An export answers a whole conversation, and the checks ask a count rather than the
// conversation, so the document is walked as the harness wrote it (readOpenCodeExportDocument), with
// every message consumed token by token and nothing of it held.
type openCodeExportSummary struct {
	SessionID string
	Directory string
	Messages  int
}

// openCodeExport asks the harness's own reader about one session and answers the fields this route asks
// about. The invocation is read as a protocol document rather than a turn's excerpt: measured on 2.2.5,
// `session export` prints the session's own document, which can be larger than any diagnostic bound, and
// a truncated document is not an answer about anything.
func openCodeExport(
	ctx context.Context, executable, sessionID string, environment []string, timeout time.Duration,
	stderr io.Writer,
) (openCodeExportSummary, error) {
	arguments := []string{"session", "export", sessionID, "--standalone"}
	output, diagnostics, err := runOpenCodeExportDocument(ctx, executable, "", arguments, environment,
		timeout, stderr)
	if err != nil {
		reason := strings.TrimSpace(output)
		if reason == "" {
			reason = strings.TrimSpace(diagnostics)
		}
		return openCodeExportSummary{}, fmt.Errorf("%s: %w", summariseText(reason, 300), err)
	}
	return readOpenCodeExportDocument(output)
}

// readOpenCodeExportDocument reads the fields the checks ask about out of the harness's own document as
// the harness wrote it. The document is walked token by token -- the walk is the one the Pi-family load
// reader uses for a whole-conversation answer (readPiResponses): a message this route does not ask about
// is consumed as it arrives rather than materialised, so a session larger than any diagnostic bound is
// read the same way as a small one, and only the session's id, its recorded directory and the number of
// messages it holds are kept.
func readOpenCodeExportDocument(output string) (openCodeExportSummary, error) {
	var summary openCodeExportSummary
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return summary, errors.New("it answered nothing")
	}
	decoder := jsontext.NewDecoder(strings.NewReader(trimmed))
	token, err := decoder.ReadToken()
	if err != nil {
		return summary, err
	}
	if token.Kind() != '{' {
		return summary, fmt.Errorf("it answered a JSON %v, want a document object", token.Kind())
	}
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return summary, err
		}
		switch name.String() {
		case "info":
			if err := readOpenCodeExportInfo(decoder, &summary); err != nil {
				return summary, err
			}
		case "messages":
			count, err := readOpenCodeExportMessageCount(decoder)
			if err != nil {
				return summary, err
			}
			summary.Messages = count
		default:
			if err := skipJSONValue(decoder); err != nil {
				return summary, err
			}
		}
	}
	if _, err := decoder.ReadToken(); err != nil { // the document object's closing brace
		return summary, err
	}
	// The harness answers one document: anything after it is not the session this check asked about, so
	// trailing content is refused rather than ignored (a caller's own words must not read as the
	// harness's document).
	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		return summary, errors.New("its answer holds more than one JSON value")
	}
	if strings.TrimSpace(summary.SessionID) == "" {
		return summary, errors.New("its document names no session id")
	}
	return summary, nil
}

// readOpenCodeExportInfo reads the document's `info` object for the two members this route asks about:
// the session's own id and the directory the harness files it under. Every other member the harness's
// own document carries (its usage, its model binding, its times) is consumed as it arrives.
func readOpenCodeExportInfo(dec *jsontext.Decoder, summary *openCodeExportSummary) error {
	token, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if token.Kind() != '{' {
		return fmt.Errorf("the document's info is a JSON %v, want an object", token.Kind())
	}
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return err
		}
		switch name.String() {
		case "id":
			value, err := dec.ReadToken()
			if err != nil {
				return err
			}
			if value.Kind() != '"' {
				return fmt.Errorf("the document's info.id is a JSON %v, want a string", value.Kind())
			}
			summary.SessionID = value.String()
		case "location":
			if err := readOpenCodeExportLocation(dec, summary); err != nil {
				return err
			}
		default:
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
	}
	_, err = dec.ReadToken() // the info object's closing brace
	return err
}

// readOpenCodeExportLocation reads the harness's own `info.location` object for the one member this route
// asks about: the directory the session is filed under.
func readOpenCodeExportLocation(dec *jsontext.Decoder, summary *openCodeExportSummary) error {
	token, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if token.Kind() != '{' {
		return fmt.Errorf("the document's info.location is a JSON %v, want an object", token.Kind())
	}
	for dec.PeekKind() != '}' {
		name, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if name.String() != "directory" {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
			continue
		}
		value, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if value.Kind() != '"' {
			return fmt.Errorf("the document's info.location.directory is a JSON %v, want a string",
				value.Kind())
		}
		summary.Directory = value.String()
	}
	_, err = dec.ReadToken() // the location object's closing brace
	return err
}

// readOpenCodeExportMessageCount counts the document's messages without holding any of them: the array is
// walked entry by entry, and each entry is consumed as a value of its own.
func readOpenCodeExportMessageCount(dec *jsontext.Decoder) (int, error) {
	token, err := dec.ReadToken()
	if err != nil {
		return 0, err
	}
	if token.Kind() != '[' {
		return 0, fmt.Errorf("the document's messages is a JSON %v, want an array", token.Kind())
	}
	count := 0
	for dec.PeekKind() != ']' {
		if err := skipJSONValue(dec); err != nil {
			return 0, err
		}
		count++
	}
	if _, err := dec.ReadToken(); err != nil { // the message array's closing bracket
		return 0, err
	}
	return count, nil
}

// VerifyOpenCodeLoad asks the target harness's own reader to resolve the installed session and answer
// about it. The check is an answer only if the document it answers with is this session's, and the same
// command with an unknown id must refuse it: that contrast is what makes the answer about *this*
// session rather than about the store. No model turn is involved, and the harness runs with a scratch
// configuration and state directory and without the credential names it could read, so nothing of the
// caller's is touched and nothing could be billed.
//
// The check answers from the installed home rather than from a copy: measured, `session export` changes
// none of the session's own records (the database file's bytes move with its WAL handling, which the
// detail says), and copying a live database another process may be writing would be a worse failure mode
// than the one it would prevent.
func VerifyOpenCodeLoad(ctx context.Context, in targetVerificationInput) LoadVerification {
	verification := LoadVerification{
		State:  VerificationStateNotVerified,
		Method: loadMethodOpenCode,
	}
	path, err := resolveOpenCodeExecutable(in.Binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so the installed session's native load "+
			"could not be verified", in.Binary)
		return verification
	}
	verification.Binary = path
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = openCodeProbeTimeout
	}
	configDir, err := os.MkdirTemp("", "ccc-opencode-load-config-")
	if err != nil {
		verification.Detail = "a scratch configuration directory could not be created: " + err.Error()
		return verification
	}
	defer os.RemoveAll(configDir)
	stateDir, err := os.MkdirTemp("", "ccc-opencode-load-state-")
	if err != nil {
		verification.Detail = "a scratch state directory could not be created: " + err.Error()
		return verification
	}
	defer os.RemoveAll(stateDir)
	environment := openCodeEnvironment(in.Home, map[string]string{
		"XDG_CONFIG_HOME": configDir,
		"XDG_STATE_HOME":  stateDir,
	}, openCodeLoadEnvDenied...)

	output, err := openCodeExport(ctx, path, in.SessionID, environment, timeout, in.Stderr)
	if err != nil {
		verification.Detail = "the harness's own reader did not resolve the installed session: " + err.Error()
		return verification
	}
	if output.SessionID != in.SessionID {
		verification.Detail = fmt.Sprintf("the harness's own reader answered about %s, not about the "+
			"installed session %s", output.SessionID, in.SessionID)
		return verification
	}
	// The control arm's refusal is the answer this check requires, so its own diagnostic line is
	// discarded: the contrast is read from the returned error, which carries the harness's words for the
	// failed run, and forwarding the line would print an expected refusal beside a verified run's output.
	_, controlErr := openCodeExport(ctx, path, openCodeUnknownSessionID, environment, timeout, io.Discard)
	if controlErr == nil {
		verification.Detail = "the same command with an unknown session id answered a " +
			"session, so its answer is not shown to be about the installed one"
		return verification
	}
	if !strings.Contains(controlErr.Error(), openCodeUnknownSessionID) {
		verification.Detail = fmt.Sprintf("the same command with an unknown session id refused it "+
			"without naming the id, so the contrast could not be read as this check's control: %s",
			summariseText(controlErr.Error(), 300))
		return verification
	}
	verification.State = VerificationStateVerified
	verification.SessionID = output.SessionID
	verification.Path = openCodeStorePath(in.Home)
	verification.Cwd = output.Directory
	verification.Detail = fmt.Sprintf("the harness's own reader resolved the installed session and "+
		"answered with its own id and %d message(s); the same command with an unknown session id "+
		"refused it, naming %s. The check ran with a scratch configuration and state directory and "+
		"without the credential names the harness could read, and it billed no model turn",
		output.Messages, openCodeUnknownSessionID)
	return verification
}

// openCodeResumeArguments is the non-interactive resume invocation: the session's id, the option
// terminator and the turn's words. Both the line the report prints and the check that runs a real turn
// call it, so the invocation a caller pastes is the one that was verified.
func openCodeResumeArguments(sessionID, prompt string) []string {
	return []string{"run", "--standalone", "--session", sessionID, "--", prompt}
}

// openCodeResumeEnvironment is the environment both the printed resume line and the resume check run the
// harness with: the data root the session's store lives under, and the store's own database file pinned
// in the override variable. It is the caller's own configuration and credentials otherwise -- a real turn
// needs a model, and this check never supplies one of its own. Both names have their own reason: the data
// root is what moves the harness's whole data directory (`opencode debug paths data` answers
// `$XDG_DATA_HOME/opencode`), and the database is pinned because a printed line can name a value but
// cannot express the removal the route's own commands apply -- an inherited `OPENCODE_DB` would otherwise
// send the pasted line to another store, where `run --session <id>` *creates* a session for the id
// (measured) instead of resolving the installed one. Measured on 2.2.5: the value is a path, and
// `debug paths db` answers exactly the file it names while `session export` resolves the session there
// and refuses it in another store.
func openCodeResumeEnvironment(home string) map[string]string {
	return map[string]string{
		openCodeDataRootEnv: openCodeDataRoot(home),
		openCodeDBEnv:       openCodeStorePath(home),
	}
}

// VerifyOpenCodeResume runs one real turn in the installed session and reports what the harness did.
//
// It is the only state that proves a session continues: the invocation is the one the report prints, run
// from the session's recorded working directory, with the caller's own environment, so the turn's model
// access is whatever the caller has configured -- this check never supplies credentials of its own. The
// session's growth is measured through the harness's own reader (its export's message count before and
// after the turn): a turn appends its own records to the session it opens, so a harness that answered
// without the store reading differently is refused the same way a file-backed route refuses one whose
// file did not grow.
func VerifyOpenCodeResume(ctx context.Context, in targetVerificationInput) ResumeVerification {
	verification := ResumeVerification{
		State:  VerificationStateNotVerified,
		Method: resumeMethodOpenCode,
	}
	path, err := resolveOpenCodeExecutable(in.Binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so no turn could be run in the installed session",
			in.Binary)
		return verification
	}
	if strings.TrimSpace(in.Prompt) == "" {
		verification.Detail = "no resume prompt was given, and call/ccc does not invent one: it becomes part of the session"
		return verification
	}
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = openCodeResumeTimeout
	}
	// The check runs the environment the report prints, so the invocation a caller pastes is the one that
	// was verified: the data root is set, and the store's own database is pinned where the caller's
	// override would otherwise decide which database the command reads (openCodeResumeEnvironment says
	// why the printed line names a value rather than removing one).
	environment := openCodeEnvironment(in.Home, map[string]string{
		openCodeDBEnv: openCodeStorePath(in.Home),
	})
	arguments := openCodeResumeArguments(in.SessionID, in.Prompt)
	verification.Command = renderInvocation(append([]string{path}, arguments...),
		openCodeResumeEnvironment(in.Home), in.Cwd)

	before, beforeErr := openCodeMessageCount(ctx, path, in.SessionID, environment, timeout, in.Stderr)
	if beforeErr != nil {
		verification.Detail = "the installed session could not be read through the harness's own reader " +
			"before the turn: " + beforeErr.Error()
		return verification
	}
	answer, diagnostics, runErr := runOpenCodeResumeTurn(ctx, path, in.Cwd, arguments, environment, timeout, in.Stderr)
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
	after, afterErr := openCodeMessageCount(ctx, path, in.SessionID, environment, timeout, in.Stderr)
	growthDetail := openCodeGrowthDetail(before, after, afterErr)
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
		verification.Detail = "the turn ran, but the installed session could not be read through the " +
			"harness's own reader afterwards: " + afterErr.Error()
		return verification
	}
	if after <= before {
		verification.Detail = fmt.Sprintf("the harness answered from the installed session in one turn, "+
			"but the harness's own reader still reads %d message(s), so the turn was not appended to the "+
			"installed session", before)
		return verification
	}
	verification.State = VerificationStateVerified
	verification.Detail = fmt.Sprintf("the harness answered from the installed session in one turn and "+
		"wrote its own records into it%s", growthDetail)
	return verification
}

// openCodeMessageCount reads the installed session through the harness's own reader and answers how many
// messages its own document holds. The harness answers about the session it holds; a refusal (a session
// the store cannot resolve) is reported as the failure it is.
func openCodeMessageCount(
	ctx context.Context, executable, sessionID string, environment []string, timeout time.Duration,
	stderr io.Writer,
) (int, error) {
	summary, err := openCodeExport(ctx, executable, sessionID, environment, timeout, stderr)
	if err != nil {
		return 0, err
	}
	if summary.SessionID != sessionID {
		return 0, fmt.Errorf("the harness's own reader answered about %s, not %s", summary.SessionID, sessionID)
	}
	return summary.Messages, nil
}

// openCodeGrowthDetail words the growth one turn produced, in the harness's own terms: the message count
// its own reader answers before and after.
func openCodeGrowthDetail(before, after int, afterErr error) string {
	if afterErr != nil {
		return fmt.Sprintf(" (the installed session could not be read through the harness's own reader "+
			"afterwards: %v)", afterErr)
	}
	return fmt.Sprintf(" (the harness's own reader read %d message(s) before the turn and %d after)", before, after)
}

// firstNonEmptyText answers the first of its arguments that holds a non-blank text, so a failure reports
// whichever stream the harness answered on and falls back to the error itself.
func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// openCodeHarnessRun is the shape of one harness invocation this route runs: the executable, the working
// directory it runs from, its arguments, its environment, the deadline and the diagnostics channel. It is
// named so a case can answer one seam with its own function instead of needing the host's harness.
type openCodeHarnessRun func(
	ctx context.Context, executable, cwd string, args []string, environment []string,
	timeout time.Duration, stderr io.Writer,
) (string, string, error)

// The seams the checks drive the harness through: what each check makes of the harness's own answer is
// the rule under test, not which build of the harness a host happens to have installed. A case answers
// the protocol itself and records the environment instead of the case needing the host's harness. This
// route runs every seam under the shared turn runner's own rule (route_shared.go): it adds no environment
// of its own, so there is no route-local wrapper between the seams and it. The export seam is the
// exception by name only: it runs the shared document runner, whose stdout is the harness's whole
// answer rather than a turn's excerpt, because a session export is a document this route reads a session
// out of (openCodeExport).
var (
	runOpenCodeTurn           openCodeHarnessRun = runRouteTurn
	runOpenCodeExportDocument openCodeHarnessRun = runRouteDocument
	runOpenCodeResumeTurn     openCodeHarnessRun = runRouteTurn
	runOpenCodeAgentsProbe    openCodeHarnessRun = runRouteTurn
	runOpenCodeImporter       openCodeHarnessRun = runRouteTurn
	resolveOpenCodeExecutable                    = resolveTargetExecutable
)
