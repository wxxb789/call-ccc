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
	"sort"
	"strings"
	"time"
)

// ClaudeBinaryDefault is the executable a Claude Code target is resumed with when the caller named none.
const ClaudeBinaryDefault = "claude"

var claudeVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

// ProbeClaudeBinary finds the target executable and asks it for its version. A missing binary is an
// answer, not an error: a conversion can still be written (and verified structurally), but the report
// says the native half could not run. Measured: `claude --version` prints `2.1.326 (Claude Code)`.
func ProbeClaudeBinary(ctx context.Context, binary string) harnessProbe {
	if strings.TrimSpace(binary) == "" {
		binary = ClaudeBinaryDefault
	}
	probe := harnessProbe{Binary: binary}
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
	if version := claudeVersionFrom(output); version != "" {
		probe.Version = version
		return probe
	}
	probe.Detail = fmt.Sprintf("%s --version printed no version: %s", path, summariseText(output, 120))
	return probe
}

// claudeVersionFrom answers the version a `claude --version` line names. Measured on 2.1.326, whose
// whole answer is `2.1.326 (Claude Code)`; the pattern reads the version out of that line rather than
// requiring the exact spelling, because the product name beside it is the harness's own to change.
func claudeVersionFrom(output string) string {
	return claudeVersionPattern.FindString(output)
}

// claudeLoadEnvDenied are the environment variables the load check removes before it runs the target's
// loader: every one of them can point a run at a model or carry model credentials, and the load check's
// promise is that the loader resolves the installed session without a model turn. The names are listed
// in the verification's own detail, so a reader of the report knows exactly which variables were taken
// out of the environment rather than having to trust that none mattered.
var claudeLoadEnvDenied = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_BASE_URL",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
}

// ClaudeLoadOptions configures one native load verification.
type ClaudeLoadOptions struct {
	// Binary is the harness to drive; empty means ClaudeBinaryDefault.
	Binary string
	// SessionID is the identity the conversion wrote.
	SessionID string
	// Cwd is the session's working directory, which the harness has to be run from: measured, the
	// loader finds a session through the working directory it is started in.
	Cwd string
	// Directory and FileName are where inside a data directory the session is filed, and Data is the
	// converted byte string: the check drives the loader against a throw-away copy of exactly those
	// bytes rather than against the installed session, because the harness writes its own records into
	// the session it resumes (measured) and a verification must not be the thing that alters the session
	// the caller is about to continue.
	Directory string
	FileName  string
	Data      []byte
	// Installed is the path the session really occupies, whose bytes the check reports on: they are
	// compared before and after, so "the check did not touch the installed session" is an observation
	// rather than a promise.
	Installed string
	Timeout   time.Duration
	// Stderr receives the harness's own diagnostics while the check runs.
	Stderr io.Writer
}

// claudeLoadAnswer is the part of the harness's own result document this check reads. Measured on Claude
// Code 2.1.326: with no model credentials reachable, the loader still resolves the session, reports it,
// and answers with a result document naming that session and the model call it could not make.
type claudeLoadAnswer struct {
	Subtype        string `json:"subtype"`
	SessionID      string `json:"session_id"`
	IsError        bool   `json:"is_error"`
	Result         string `json:"result"`
	TerminalReason string `json:"terminal_reason"`
}

// VerifyClaudeLoad asks the target harness's own loader to open the installed session and report the
// conversation it built, with no model turn.
//
// The check installs a throw-away copy of the converted bytes into a scratch data directory (the harness
// writes its own bookkeeping records into a session it resumes, so driving the loader against the
// installed file would alter it), runs `claude --bare -p --output-format json --resume <id>` there with
// the environment variables that could reach a model removed, and requires the harness's own answer to
// name the session this conversion wrote. It runs the same command a second time with a freshly
// generated, unknown session id: that control arm must make the loader answer that no such conversation
// exists, which is what makes the first arm an answer about *this* session rather than about the loader
// starting at all.
//
// `--bare` is what keeps the run offline: the harness then takes its authentication strictly from
// ANTHROPIC_API_KEY or an apiKeyHelper, neither of which this check provides, so no credential store is
// read and no turn can be billed. A protocol or version change that makes the loader refuse the file is
// reported as not-verified with the harness's own words, never as a pass.
func VerifyClaudeLoad(ctx context.Context, opts ClaudeLoadOptions) LoadVerification {
	verification := LoadVerification{
		State:     VerificationStateNotVerified,
		Method:    loadMethodClaude,
		SessionID: opts.SessionID,
		Path:      opts.Installed,
		Cwd:       opts.Cwd,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = ClaudeBinaryDefault
	}
	path, err := resolveClaudeExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so the installed session's native load could not be verified", binary)
		return verification
	}
	verification.Binary = path

	before, beforeErr := installedDigest(opts.Installed)

	scratch, err := os.MkdirTemp("", "ccc-claude-load-")
	if err != nil {
		verification.Detail = fmt.Sprintf("a scratch data directory could not be created for the check: %v", err)
		return verification
	}
	defer os.RemoveAll(scratch)
	copyPath, _, err := InstallClaudeTranscript(ClaudeTranscript{
		Directory: opts.Directory,
		FileName:  opts.FileName,
		Data:      opts.Data,
	}, scratch)
	if err != nil {
		verification.Detail = fmt.Sprintf("the check's own copy of the session could not be written: %v", err)
		return verification
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = claudeLoadTimeout
	}
	answer, stderr, runErr := runClaudeLoad(ctx, path, scratch, opts.Cwd, opts.SessionID, timeout, opts.Stderr)
	var resolved claudeLoadAnswer
	// The harness exits non-zero on this arm too: measured, it resolves the session and then reports the
	// model call it could not make as a failed result. So the answer is read from its own document, and a
	// non-zero status without one is what says the loader refused the file or never ran.
	if jsonErr := json.Unmarshal([]byte(lastJSONLine(answer)), &resolved); jsonErr != nil {
		reason := fmt.Sprintf("stdout: %s; stderr: %s", summariseText(answer, 200), summariseText(stderr, 200))
		if runErr != nil {
			reason = fmt.Sprintf("%v; %s", runErr, reason)
		}
		verification.Detail = fmt.Sprintf(
			"the harness answered nothing this check could read for session %s (%s)", opts.SessionID, reason)
		return verification
	}
	if resolved.SessionID != opts.SessionID {
		verification.Detail = fmt.Sprintf(
			"the harness answered about session %q, not the installed %q",
			resolved.SessionID, opts.SessionID)
		return verification
	}

	// The control arm: the same command for an identity that exists nowhere. Its answer is what turns
	// "the harness started" into "the harness resolved this session" -- a loader that resolves nothing
	// reports no conversation for the unknown id, and a loader that reports a conversation for an id it
	// does not have would be answering from something other than the file this check installed.
	unknown, err := newUUIDV4()
	if err != nil {
		verification.Detail = fmt.Sprintf("a control identity could not be generated: %v", err)
		return verification
	}
	// The control arm is expected to exit non-zero with no result document at all: that is the harness
	// saying it found no such session.
	controlOut, controlErr, _ := runClaudeLoad(ctx, path, scratch, opts.Cwd, unknown, timeout, opts.Stderr)
	if controlJSON := lastJSONLine(controlOut); controlJSON != "" {
		verification.Detail = fmt.Sprintf(
			"the control arm answered about session %q, which this check never installed, so the loader "+
				"is not answering from the installed file", unknown)
		return verification
	}
	if !strings.Contains(controlErr, "No conversation found with session ID") {
		verification.Detail = fmt.Sprintf(
			"the control arm did not report the unknown session as missing; it answered: %s",
			summariseText(strings.TrimSpace(controlOut+" "+controlErr), 300))
		return verification
	}

	if beforeErr != nil {
		verification.Detail = fmt.Sprintf(
			"the harness's loader resolved the installed session %s (its own answer named it) and the "+
				"installed file could not be read to confirm the check left it alone: %v",
			opts.SessionID, beforeErr)
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
		"the harness's loader resolved the installed session and answered about it (%s); the same "+
			"command with an unknown session id answered \"No conversation found with session ID: %s\". "+
			"The check ran against a throw-away copy of the converted bytes (%s) with --bare and with %s "+
			"removed from the environment, so the loader read the session without a model turn, and the "+
			"installed file is unchanged (%s)",
		claudeAnswerSummary(resolved), unknown, copyPath, strings.Join(claudeLoadEnvDenied, ", "),
		shortDigest(before))
	return verification
}

// runClaudeHarnessTurn is the seam both checks drive the target harness through. It is named so a test
// can answer as a harness whose protocol behaviour is not the host's business: the rule under test is
// what the check makes of the harness's answer, not which build of the harness is installed here, and
// the real subprocess path is exercised by scripts/claude-native-check.py against a real binary.
var runClaudeHarnessTurn = runClaudeTurn

// claudeLoadTimeout bounds one load check arm. Measured: an installed session answered in 0.4 s, and a
// harness that cannot answer is stopped by this deadline rather than by the caller's patience.
const claudeLoadTimeout = 90 * time.Second

// runClaudeLoad runs one load-check arm and returns the harness's stdout, its stderr, and the error the
// command itself reported. Both arms run with --bare and with the variables that could reach a model
// removed, so neither can be a billed turn.
func runClaudeLoad(
	ctx context.Context, binary, home, cwd, sessionID string, timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	options := ClaudeResumeOptions{Home: home, Binary: binary, SessionID: sessionID, Cwd: cwd}
	arguments := []string{"--bare", "-p", "--output-format", "json", "--resume", sessionID, promptTerminator, "load check"}
	return runClaudeHarnessTurn(ctx, binary, options, timeout, arguments, claudeEnvironment(home, claudeLoadEnvDenied), stderr)
}

// claudeEnvironment builds a child environment for one harness run: the caller's own environment with
// the variable that selects the target's data directory set to home, and with the named variables
// removed. Nothing else is changed, so a host's PATH, locale and tool configuration still reach the
// harness.
//
// Every name is read in its own upper case, the way the denylist and os/exec itself read one: Windows'
// environment is case-insensitive and os/exec keeps the last value among the names it is handed, so a
// leftover lower- or mixed-case spelling of the home variable would, after the sort below, sit after
// the appended override and win there -- the check would then read the caller's own home instead of the
// directory it installed into.
func claudeEnvironment(home string, denied []string) []string {
	blocked := make(map[string]struct{}, len(denied))
	for _, name := range denied {
		blocked[strings.ToUpper(name)] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		folded := strings.ToUpper(name)
		if _, deny := blocked[folded]; deny {
			continue
		}
		if folded == claudeHomeEnv {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, claudeHomeEnv+"="+home)
	sort.Strings(environment)
	return environment
}

// ClaudeResumeOptions configures one real resume turn.
type ClaudeResumeOptions struct {
	Home      string
	Binary    string
	SessionID string
	Cwd       string
	// SessionPath is the installed file, whose growth the turn is reported by.
	SessionPath string
	Prompt      string
	Timeout     time.Duration
	Stderr      io.Writer
}

// VerifyClaudeResume runs one real turn in the installed session and reports what the harness did.
//
// It is the only state that proves a session continues: the invocation is the one the report prints, run
// from the session's own working directory (measured: that is the only directory the loader finds it
// from) with the caller's own environment, so the turn's model access is whatever the caller has
// configured -- this check never supplies credentials of its own. A missing binary, a harness that
// refuses the session and a harness that cannot reach a model are each reported as not-verified with the
// harness's own words. A harness that answers but leaves the installed session's bytes unchanged is
// refused the same way: the verified state says the harness wrote its own records into the session, and
// the two sizes it reports have to show it.
func VerifyClaudeResume(ctx context.Context, opts ClaudeResumeOptions) ResumeVerification {
	verification := ResumeVerification{
		State:  VerificationStateNotVerified,
		Method: resumeMethodClaude,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = ClaudeBinaryDefault
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		verification.Detail = "no resume prompt was given, and call/ccc does not invent one: it becomes part of the session"
		return verification
	}
	arguments := []string{"-p", "--output-format", "json", "--resume", opts.SessionID, promptTerminator, opts.Prompt}
	path, err := resolveClaudeExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so no turn could be run in the installed session", binary)
		return verification
	}
	// The reported command is the invocation this check runs, the prompt included: a caller reading the
	// report sees exactly the line that produced the answer beside it.
	verification.Command = renderInvocation(append([]string{path}, arguments...),
		map[string]string{claudeHomeEnv: opts.Home}, opts.Cwd)
	before, beforeErr := fileSize(opts.SessionPath)
	if beforeErr != nil {
		verification.Detail = fmt.Sprintf("the installed session %s could not be measured before the turn: %v", opts.SessionPath, beforeErr)
		return verification
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = claudeResumeTimeout
	}
	stdout, stderr, runErr := runClaudeHarnessTurn(ctx, path, opts, timeout, arguments, claudeEnvironment(opts.Home, nil), opts.Stderr)
	after, afterErr := fileSize(opts.SessionPath)
	verification.SessionBytesBefore = before
	verification.SessionBytesAfter = after
	// The growth every answer path reports: the two sizes when both were readable, and the measurement
	// failure named as such rather than shown as a session that went to 0 bytes.
	growthDetail := claudeTurnGrowthDetail(before, after)
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

	var answer claudeLoadAnswer
	parsed := json.Unmarshal([]byte(lastJSONLine(stdout)), &answer) == nil
	answerText := answer.Result
	if !parsed {
		answerText = strings.TrimSpace(stdout)
	}
	verification.Output, verification.OutputTruncated = boundedText(answerText, 0)
	if runErr != nil || answer.IsError {
		reason := strings.TrimSpace(answer.Result)
		if reason == "" {
			reason = strings.TrimSpace(stderr)
		}
		if reason == "" {
			reason = strings.TrimSpace(stdout)
		}
		verification.Detail = fmt.Sprintf(
			"the harness did not answer: %s (exit status %d)%s",
			summariseText(reason, 300), exitCode, growthDetail)
		return verification
	}
	if answer.SessionID != "" && answer.SessionID != opts.SessionID {
		verification.Detail = fmt.Sprintf("the turn answered about session %q, not the installed %q",
			answer.SessionID, opts.SessionID)
		return verification
	}
	if strings.TrimSpace(answer.Result) == "" {
		verification.Detail = fmt.Sprintf(
			"the harness exited successfully and reported no answer%s", growthDetail)
		return verification
	}
	// The answer alone is not evidence that the turn continued the installed session: the sentence a
	// verified state reports says the harness wrote its own records into it, so the sizes must show it.
	// The Codex route refuses the same shape for the same reason.
	if afterErr != nil {
		verification.Detail = "the turn ran, but the installed session could not be measured afterwards: " + afterErr.Error()
		return verification
	}
	if after <= before {
		verification.Detail = fmt.Sprintf(
			"the harness answered from the installed session in one turn, but the session file did not grow "+
				"(%d bytes before and after), so the turn was not appended to the installed session", before)
		return verification
	}
	verification.State = VerificationStateVerified
	verification.Detail = fmt.Sprintf(
		"the harness answered from the installed session in one turn and wrote its own records into it%s",
		claudeTurnGrowthDetail(before, after))
	return verification
}

// claudeResumeTimeout bounds one real resume turn. A turn is a model call in the caller's own session,
// so it is given a generous deadline rather than the load check's.
const claudeResumeTimeout = 5 * time.Minute

func claudeTurnGrowthDetail(before, after int64) string {
	if before == 0 && after == 0 {
		return ""
	}
	return fmt.Sprintf(" (the session file went from %d to %d bytes)", before, after)
}

// runClaudeTurn runs one non-interactive harness invocation and returns its stdout and stderr separately,
// both bounded. The command runs in the session's working directory when it exists, because the loader
// finds a session through the directory it is started in.
func runClaudeTurn(
	ctx context.Context, executable string, opts ClaudeResumeOptions, timeout time.Duration,
	args []string, environment []string, stderr io.Writer,
) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, executable, args...)
	// The directory is the source's own recorded value, as the transcript records it: trimming decides
	// only whether one was recorded, because a directory name may end in whitespace.
	if dir := opts.Cwd; strings.TrimSpace(dir) != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			command.Dir = dir
		}
	}
	command.Env = environment
	answer := newBoundedBuffer(claudeTurnOutputMax)
	diagnostics := newBoundedBuffer(claudeTurnDiagnosticsMax)
	command.Stdout = answer
	if stderr != nil {
		command.Stderr = io.MultiWriter(diagnostics, stderr)
	} else {
		command.Stderr = diagnostics
	}
	err := command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return answer.String(), diagnostics.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return answer.String(), diagnostics.String(), err
}

// claudeTurnOutputMax and claudeTurnDiagnosticsMax bound what one harness run may add to this process's
// memory, the way the Codex route's own bounds do: the answer a report quotes is cut to boundedText's
// limit anyway, and a harness flooding either stream is answered by its deadline and an excerpt rather
// than by this process growing with every byte it writes.
const (
	claudeTurnOutputMax      = 1 << 20
	claudeTurnDiagnosticsMax = 4096
)

// resolveTargetExecutable answers the path a command will run: exec.LookPath's answer, made absolute
// where it is not already, because a run that changes its working directory would evaluate a relative
// spelling against the new directory rather than against the caller's.
func resolveTargetExecutable(binary string) (string, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

// resolveClaudeExecutable is the seam the two checks find the target harness through, named for the same
// reason runClaudeHarnessTurn is: which build of the harness a host happens to have installed is not the
// rule under test, so a test answers the lookup with a path of its own rather than the checks standing
// or falling on the host's PATH. The lookup's own behaviour keeps its coverage through the real one:
// TestProbeClaudeBinaryReportsAMissingBinary and TestVerifyClaudeResumeRefusesWithoutABinary both ask a
// PATH that holds no harness.
var resolveClaudeExecutable = resolveTargetExecutable

// installedDigest is the digest of a file this check must not alter, or an error when the file cannot be
// read. An empty path is answered as an error by the caller's own reading of it: a load check with no
// installed path has nothing to report about one.
func installedDigest(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("no installed path was given")
	}
	return fileDigest(path)
}

// shortDigest renders a digest the way a report can hold it: the first twelve characters, which is
// enough to compare two readings and not enough to look like a claim about the whole file.
func shortDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}

// lastJSONLine answers the last line of stdout that parses as one JSON object, or "": a harness that
// writes progress lines before its result document is read for the document.
func lastJSONLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		var probe map[string]any
		if json.Unmarshal([]byte(line), &probe) == nil {
			return line
		}
	}
	return ""
}

// claudeAnswerSummary is the harness's own words about the answer it gave the load check, so a reader of
// the report sees whether the loader stopped at its model call or got further.
func claudeAnswerSummary(answer claudeLoadAnswer) string {
	summary := fmt.Sprintf("subtype %q", answer.Subtype)
	if strings.TrimSpace(answer.TerminalReason) != "" {
		summary += fmt.Sprintf(", terminal reason %q", answer.TerminalReason)
	}
	if strings.TrimSpace(answer.Result) != "" {
		summary += fmt.Sprintf(", result %q", summariseText(answer.Result, 120))
	}
	if !answer.IsError {
		// The loader reached a model, which means the host answered its call: the check still verified
		// that the session resolves, and the report says the run was not the offline one it is otherwise.
		summary += ", and the run reached a model"
	}
	return summary
}
