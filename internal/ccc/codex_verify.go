package ccc

import (
	"bytes"
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
	"sync"
	"time"

	"go.kenn.io/agentsview/internal/stringutil"
)

// CodexBinaryDefault is the executable name a conversion probes for the target harness. The probe is
// also what decides which cli_version the written session records and whether a native verification can
// run at all.
const CodexBinaryDefault = "codex"

// cccClientVersion identifies this client to the target's app server during a verification.
const cccClientVersion = "0.1.0"

// Native verification states. "not-verified" is not a failure of the conversion: it says the target
// harness could not answer the question, and the detail says why.
const (
	VerificationStateVerified     = "verified"
	VerificationStateNotVerified  = "not-verified"
	VerificationStateNotRequested = "not-requested"
)

var codexVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

// CodexProbe is what the target harness answered when asked for its version.
type CodexProbe struct {
	Binary  string `json:"binary"`
	Found   bool   `json:"found"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// ProbeCodexBinary finds the target executable and asks it for its version. A missing binary is an
// answer, not an error: a conversion can still be written (and verified structurally), but the report
// says the native half could not run.
func ProbeCodexBinary(ctx context.Context, binary string) CodexProbe {
	if strings.TrimSpace(binary) == "" {
		binary = CodexBinaryDefault
	}
	probe := CodexProbe{Binary: binary}
	path, err := exec.LookPath(binary)
	if err != nil {
		probe.Detail = fmt.Sprintf("%s is not on PATH", binary)
		return probe
	}
	probe.Found = true
	probe.Binary = path
	output, err := runBounded(ctx, path, 30*time.Second, "--version")
	if err != nil {
		probe.Detail = fmt.Sprintf("%s --version failed: %v", path, err)
		return probe
	}
	if version := codexVersionPattern.FindString(output); version != "" {
		probe.Version = version
		return probe
	}
	probe.Detail = fmt.Sprintf("%s --version printed no version: %s", path, summariseText(output, 120))
	return probe
}

// runBounded runs one command with a deadline and returns its combined output, bounded to a size a
// report can hold.
func runBounded(ctx context.Context, binary string, timeout time.Duration, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, args...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return output.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return output.String(), err
}

// CodexConfigProbe is the target harness's own answer about the provider and model a session of its
// gets: the values its loader checks a converted session against. call/ccc never parses the harness's
// configuration file; the harness answers for itself through the same app server its own clients use.
//
// The answer comes from a thread the harness starts for the question and keeps in memory only
// (`thread/start` with an ephemeral thread), because that is what reports the *effective* provider: the
// configuration's explicit setting, or the harness's own default when it names none. Reading the
// configuration alone reports the explicit setting only, and a session recorded against a missing value
// is exactly the one the harness refuses to resume (measured).
type CodexConfigProbe struct {
	Found    bool   `json:"found"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Source   string `json:"source,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// ProbeCodexConfig asks the installed target harness which provider and model a new session of it
// would use. cwd, when it is a directory on this machine, asks the harness for the configuration as
// seen from there, so a project layer cannot be missed.
func ProbeCodexConfig(
	ctx context.Context, home, binary, cwd string, timeout time.Duration, stderr io.Writer,
) CodexConfigProbe {
	probe := CodexConfigProbe{}
	if strings.TrimSpace(binary) == "" {
		binary = CodexBinaryDefault
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		probe.Detail = fmt.Sprintf("%s is not on PATH, so its configuration could not be asked for", binary)
		return probe
	}
	session, err := startCodexAppServer(ctx, path, home, timeout, stderr)
	if err != nil {
		probe.Detail = err.Error()
		return probe
	}
	defer session.Close()
	if err := session.handshake(); err != nil {
		probe.Detail = "the target's app server did not answer initialize: " + err.Error()
		return probe
	}

	params := map[string]any{"ephemeral": true}
	if dir := strings.TrimSpace(cwd); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			params["cwd"] = dir
		}
	}
	if response, err := session.call("thread/start", params); err == nil {
		if thread, ok := response["thread"].(map[string]any); ok {
			if provider := stringField(thread, "modelProvider"); provider != "" {
				probe.Found = true
				probe.Provider = provider
				probe.Model = stringField(thread, "model")
				probe.Source = "target-thread-defaults"
				return probe
			}
		}
	}
	// A harness whose thread/start cannot answer still says what its configuration names explicitly.
	response, err := session.call("config/read", map[string]any{})
	if err != nil {
		probe.Detail = "the target harness did not answer thread/start or config/read: " + err.Error()
		return probe
	}
	config, ok := response["config"].(map[string]any)
	if !ok {
		probe.Detail = "the target harness answered config/read without a configuration"
		return probe
	}
	probe.Found = true
	probe.Provider = stringField(config, "model_provider")
	probe.Model = stringField(config, "model")
	probe.Source = "target-config"
	return probe
}

// LoadVerification reports whether the target harness's own loader read the installed session.
type LoadVerification struct {
	State         string `json:"state"`
	Method        string `json:"method"`
	Detail        string `json:"detail,omitempty"`
	ThreadID      string `json:"thread_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Path          string `json:"path,omitempty"`
	HistoryMode   string `json:"history_mode,omitempty"`
	Cwd           string `json:"cwd,omitempty"`
	Binary        string `json:"binary,omitempty"`
	BinaryVersion string `json:"binary_version,omitempty"`
}

// ResumeVerification reports whether the target harness loaded the session and produced a new turn
// from it. It is the only state that proves "natively resumable", and it is never claimed from a
// successful parse of the file.
type ResumeVerification struct {
	State              string `json:"state"`
	Method             string `json:"method"`
	Command            string `json:"command,omitempty"`
	Detail             string `json:"detail,omitempty"`
	Output             string `json:"output,omitempty"`
	OutputTruncated    bool   `json:"output_truncated,omitempty"`
	ExitCode           *int   `json:"exit_code,omitempty"`
	SessionBytesBefore int64  `json:"session_bytes_before,omitempty"`
	SessionBytesAfter  int64  `json:"session_bytes_after,omitempty"`
}

// CodexLoadOptions configures one native load verification.
type CodexLoadOptions struct {
	Home         string
	Binary       string
	SessionID    string
	ExpectedPath string
	Timeout      time.Duration
	// Stderr receives the app server's own diagnostics; the verification's detail carries a bounded
	// excerpt of them, so a failure to start is reported rather than swallowed.
	Stderr io.Writer
}

// VerifyCodexLoad asks the target harness to load the installed session through its own app server
// ("thread/resume", the request the interactive resume path uses), and requires the loaded thread to be
// the one this conversion wrote: a different id is not an answer, a loader that names no path is not an
// answer either, and an error is reported with the server's own message.
//
// The app-server protocol is version-bound and experimental, which is why the tested target version is
// recorded in every report and why a protocol failure is "not-verified" with its reason rather than a
// pass.
func VerifyCodexLoad(ctx context.Context, opts CodexLoadOptions) LoadVerification {
	verification := LoadVerification{
		State:  VerificationStateNotVerified,
		Method: loadMethod,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = CodexBinaryDefault
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so the installed session's native load could not be verified", binary)
		return verification
	}
	verification.Binary = path

	session, err := startCodexAppServer(ctx, path, opts.Home, opts.Timeout, opts.Stderr)
	if err != nil {
		verification.Detail = err.Error()
		return verification
	}
	defer session.Close()

	if err := session.handshake(); err != nil {
		verification.Detail = "the target's app server did not answer initialize: " + err.Error() + session.stderrExcerpt()
		return verification
	}
	response, err := session.call("thread/resume", map[string]any{"threadId": opts.SessionID})
	if err != nil {
		verification.Detail = "the target harness could not load the installed session: " + err.Error() + session.stderrExcerpt()
		return verification
	}
	thread, ok := response["thread"].(map[string]any)
	if !ok {
		verification.Detail = "the target harness answered thread/resume without a thread" + session.stderrExcerpt()
		return verification
	}
	loadedID := stringField(thread, "id")
	if loadedID != opts.SessionID {
		verification.Detail = fmt.Sprintf(
			"the target harness loaded thread %q for session %q, so it is not the conversion's own session",
			loadedID, opts.SessionID)
		return verification
	}
	// The loader must name the file it opened: the id alone is an identity the harness chose to echo,
	// while the path is what says the session came from this conversion's file.
	loadedPath := stringField(thread, "path")
	if loadedPath == "" {
		verification.Detail = "the target harness's loader answered without the loaded session's path, " +
			"so the file it opened could not be confirmed as the installed one"
		return verification
	}
	if opts.ExpectedPath != "" && !sameFile(loadedPath, opts.ExpectedPath) {
		verification.Detail = fmt.Sprintf(
			"the target harness loaded %q, not the installed file %q", loadedPath, opts.ExpectedPath)
		return verification
	}
	verification.State = VerificationStateVerified
	verification.ThreadID = loadedID
	verification.SessionID = stringField(thread, "sessionId")
	verification.Path = loadedPath
	verification.HistoryMode = stringField(thread, "historyMode")
	verification.Cwd = stringField(thread, "cwd")
	// The app server's own chatter (sandbox probes, PATH-alias warnings) is forwarded to the caller's
	// stderr while it runs; a verification that succeeded does not repeat it in its detail, where it
	// would read as a defect of the session. What it does say is the one write this check causes:
	// opening a session is not read-only for the harness, which records its own settings in it.
	verification.Detail = "the harness's loader answered with this session's own id and path; opening a " +
		"session makes the harness record its own settings in it (measured: one thread_settings_applied " +
		"record), and no session other than the one it opened is touched"
	return verification
}

// CodexResumeOptions configures one native resume verification: a real turn in the installed session.
type CodexResumeOptions struct {
	Home       string
	Binary     string
	SessionID  string
	Prompt     string
	WorkingDir string
	// SessionPath is the installed session file, which the verification measures before and after the
	// turn: a turn that answered without appending anything did not continue the session.
	SessionPath string
	Timeout     time.Duration
	MaxOutput   int
	// Stderr receives the target harness's own diagnostics while the turn runs.
	Stderr io.Writer
}

// VerifyCodexResume runs one turn in the installed session with the target harness's non-interactive
// resume command and reports what it did: the command, its exit status, and the session file's size
// before and after, which is what shows the turn was appended to the session rather than merely
// answered.
//
// The turn runs read-only with approvals disabled, so a verification cannot change the workspace it
// resumes into, and the front of its output is returned (bounded) as the observable answer.
func VerifyCodexResume(ctx context.Context, opts CodexResumeOptions) ResumeVerification {
	verification := ResumeVerification{
		State:  VerificationStateNotVerified,
		Method: resumeMethod,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = CodexBinaryDefault
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so no turn could be run in the installed session", binary)
		return verification
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		verification.Detail = "no resume prompt was given, and call/ccc does not invent one: it becomes part of the session"
		return verification
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	args := []string{"exec", "resume", opts.SessionID, opts.Prompt,
		"--skip-git-repo-check",
		"-c", "sandbox_mode=\"read-only\"",
		"-c", "approval_policy=\"never\"",
	}
	verification.Command = resumeCommandLine(path, args, opts.Home)

	before, _ := fileSize(opts.SessionPath)
	output, diagnostics, err := runCodexTurn(ctx, path, opts, timeout, args)
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	verification.ExitCode = &exitCode
	verification.Output, verification.OutputTruncated = boundedText(output, opts.MaxOutput)
	verification.SessionBytesBefore = before
	if err != nil {
		detail := fmt.Sprintf("the target harness exited %d", exitCode)
		if text := strings.TrimSpace(strings.Join([]string{output, diagnostics}, "\n")); text != "" {
			detail += ": " + summariseText(text, 400)
		}
		verification.Detail = detail
		return verification
	}
	after, err := fileSize(opts.SessionPath)
	if err != nil {
		verification.Detail = "the turn ran, but the session file could not be measured afterwards: " + err.Error()
		return verification
	}
	verification.SessionBytesAfter = after
	if after <= before {
		verification.Detail = fmt.Sprintf(
			"the turn ran, but the session file did not grow (%d bytes before and after), so the turn was "+
				"not appended to the installed session", before)
		return verification
	}
	verification.State = VerificationStateVerified
	return verification
}

// resumeCommandLine is the command a resume verification reports: the binary it resolved, the arguments
// it runs, and the environment that selects the data directory the turn runs against. Codex takes that
// directory from CODEX_HOME alone (it has no flag for it), so a reader who pastes the reported command
// without that environment resumes whichever home their shell points at, not the one that was verified.
func resumeCommandLine(binary string, arguments []string, home string) string {
	environment := map[string]string{}
	if home = strings.TrimSpace(home); home != "" {
		environment[codexHomeEnv] = home
	}
	return renderResumeCommand(append([]string{binary}, arguments...), environment)
}

// runCodexTurn runs one non-interactive turn and returns its answer and its diagnostics separately: the
// answer is what the harness wrote to stdout, the diagnostics everything it wrote to stderr.
//
// The executable is resolved before the command's directory is set. A relative Cmd.Path is evaluated by
// the operating system relative to the directory the child runs in, and this turn runs in the source
// session's working directory -- so a caller's "./tools/codex" would otherwise be looked for there: a
// different executable, or none, even though the probe that found it ran from the caller's directory.
// The reported command (VerifyCodexResume's own Command field and the conversation's resume lines) keeps
// the caller's requested spelling.
func runCodexTurn(ctx context.Context, binary string, opts CodexResumeOptions, timeout time.Duration, args []string) (string, string, error) {
	executable, err := resolveCodexExecutable(binary)
	if err != nil {
		return "", "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, executable, args...)
	if dir := strings.TrimSpace(opts.WorkingDir); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			command.Dir = dir
		}
	}
	command.Env = append(os.Environ(), "CODEX_HOME="+opts.Home)
	// The turn's answer is what the harness writes to stdout; its progress and warnings go to stderr,
	// which the caller sees as they arrive and which is kept separately — a failed turn's reason is
	// reported from there, and a successful one's answer is not mixed with its own chatter.
	var answer, diagnostics bytes.Buffer
	command.Stdout = &answer
	if opts.Stderr != nil {
		command.Stderr = io.MultiWriter(&diagnostics, opts.Stderr)
	} else {
		command.Stderr = &diagnostics
	}
	err = command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return answer.String(), diagnostics.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return answer.String(), diagnostics.String(), err
}

// resolveCodexExecutable answers the path a command will run: exec.LookPath's answer, made absolute
// where it is not already. LookPath returns an explicitly relative spelling unchanged (a path holding a
// separator is tried directly), and a command that changes the directory it runs in would evaluate that
// spelling against the new directory rather than against the caller's -- the difference between the
// harness the probe asked for its version and one that happens to sit under the session's own working
// directory.
func resolveCodexExecutable(binary string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

// codexAppServer is a minimal client for the target's app-server protocol over stdio: one initialize
// handshake, then the requests a verification needs. Lines that are not answers (the server's own
// notifications) are skipped rather than parsed as results.
type codexAppServer struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	lines   chan []byte
	stderr  *boundedBuffer
	// timeout bounds one request: the protocol is experimental, and a server that accepts a request and
	// then stops answering must not block a report forever. Each call gets its own bound; the process
	// itself dies with the context it was started under.
	timeout time.Duration
	nextID  int
	mu      sync.Mutex
	done    chan struct{}
}

// startCodexAppServer starts the target's app server with the harness's own data directory. A timeout
// of zero or less means the per-request default.
func startCodexAppServer(ctx context.Context, binary, home string, timeout time.Duration, stderr io.Writer) (*codexAppServer, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	command := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	command.Env = append(os.Environ(), "CODEX_HOME="+home)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("starting %s app-server: %w", binary, err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("starting %s app-server: %w", binary, err)
	}
	server := &codexAppServer{
		command: command,
		stdin:   stdin,
		lines:   make(chan []byte, 64),
		stderr:  newBoundedBuffer(4096),
		timeout: timeout,
		done:    make(chan struct{}),
	}
	if stderr != nil {
		command.Stderr = io.MultiWriter(server.stderr, stderr)
	} else {
		command.Stderr = server.stderr
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("starting %s app-server: %w", binary, err)
	}
	go server.readLines(stdout)
	go func() {
		command.Wait()
		close(server.done)
	}()
	return server, nil
}

func (s *codexAppServer) readLines(stdout io.Reader) {
	decoder := json.NewDecoder(stdout)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			close(s.lines)
			return
		}
		select {
		case s.lines <- raw:
		case <-s.done:
			return
		}
	}
}

func (s *codexAppServer) Close() {
	s.stdin.Close()
	if s.command.Process != nil {
		s.command.Process.Kill()
	}
	<-s.done
}

func (s *codexAppServer) notify(method string, params map[string]any) {
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stdin.Write(append(payload, '\n'))
}

// handshake performs the app-server handshake every verification opens with: initialize, whose answer
// names this client, then the initialized notification the protocol expects.
func (s *codexAppServer) handshake() error {
	if _, err := s.call("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "ccc", "version": cccClientVersion},
	}); err != nil {
		return err
	}
	s.notify("initialized", map[string]any{})
	return nil
}

// call sends one request and waits for its answer, bounded by the session's per-request timeout.
func (s *codexAppServer) call(method string, params map[string]any) (map[string]any, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if _, err := s.stdin.Write(append(payload, '\n')); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("writing %s: %w", method, err)
	}
	s.mu.Unlock()

	deadline := time.NewTimer(s.timeout)
	defer deadline.Stop()
	for {
		select {
		case raw, ok := <-s.lines:
			if !ok {
				return nil, fmt.Errorf("the app server closed its output while answering %s", method)
			}
			var message struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
					Code    int    `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(raw, &message); err != nil {
				continue
			}
			if message.ID == nil || *message.ID != id {
				// A notification, or an answer to an earlier request: not this call's answer.
				continue
			}
			if message.Error != nil {
				return nil, errors.New(message.Error.Message)
			}
			var result map[string]any
			if len(message.Result) > 0 {
				if err := json.Unmarshal(message.Result, &result); err != nil {
					return nil, fmt.Errorf("decoding the %s result: %w", method, err)
				}
			}
			return result, nil
		case <-s.done:
			return nil, fmt.Errorf("the app server exited while answering %s", method)
		case <-deadline.C:
			return nil, fmt.Errorf("the app server did not answer %s within %s", method, s.timeout)
		}
	}
}

// stderrExcerpt is the app server's own diagnostics, bounded, for a failure report.
func (s *codexAppServer) stderrExcerpt() string {
	text := strings.TrimSpace(s.stderr.String())
	if text == "" {
		return ""
	}
	return " (" + summariseText(text, 300) + ")"
}

// boundedBuffer keeps the first max bytes written to it: a diagnostic's opening lines say what went
// wrong, and a runaway log must not become the report.
//
// It is written by the goroutine os/exec copies a child's stderr with and read by whoever reports a
// failure, which may be while that child still runs: a verification that fails builds its detail from
// stderrExcerpt before the deferred Close stops the process, so writes and snapshots are guarded here
// rather than left to the caller's timing. Unsynchronized, those two are a data race on the buffer's own
// state and the excerpt in the report would be a torn read.
type boundedBuffer struct {
	mu   sync.Mutex
	max  int
	data bytes.Buffer
}

func newBoundedBuffer(max int) *boundedBuffer { return &boundedBuffer{max: max} }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if remaining := b.max - b.data.Len(); remaining > 0 {
		if len(p) > remaining {
			b.data.Write(p[:remaining])
		} else {
			b.data.Write(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

func stringField(object map[string]any, key string) string {
	if value, ok := object[key].(string); ok {
		return value
	}
	return ""
}

// sameFile reports whether two paths name the same file, following symlinks where the filesystem has
// them and falling back to a cleaned path comparison where it does not.
func sameFile(a, b string) bool {
	resolvedA, errA := filepath.EvalSymlinks(a)
	resolvedB, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return resolvedA == resolvedB
}

func fileSize(path string) (int64, error) {
	if strings.TrimSpace(path) == "" {
		return 0, os.ErrNotExist
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// boundedText trims a command's output to a size a report can hold, reporting whether it cut anything.
// The cut respects the text's own rune boundaries: the output is provider content, and half a rune
// would make the report invalid wherever it is copied.
func boundedText(text string, max int) (string, bool) {
	text = strings.TrimSpace(text)
	if max <= 0 {
		max = 8192
	}
	if len(text) <= max {
		return text, false
	}
	return stringutil.SafeTruncate(text, max), true
}

// summariseText collapses a text onto one bounded line, for a warning or a detail field.
func summariseText(text string, max int) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	if len(collapsed) <= max {
		return collapsed
	}
	return stringutil.SafeTruncate(collapsed, max) + "…"
}
