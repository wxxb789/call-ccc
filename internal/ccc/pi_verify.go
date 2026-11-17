package ccc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// PiBinaryDefault is the executable a Pi target is resumed with when the caller named none.
const PiBinaryDefault = "pi"

var piVersionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

// ProbePiBinary finds the target executable and asks it for its version. A missing binary is an answer,
// not an error: a conversion can still be written (and verified structurally), but the report says the
// native half could not run. Measured: `pi --version` prints `1.5.2`.
func ProbePiBinary(ctx context.Context, binary string) harnessProbe {
	return probeBinary(ctx, binary, PiBinaryDefault, piVersionFrom, func(path, output string) string {
		return fmt.Sprintf("%s --version answered %q, which names no version",
			path, summariseText(strings.TrimSpace(output), 120))
	})
}

// piVersionFrom answers the version a `pi --version` line names: the first semantic version on it.
func piVersionFrom(output string) string {
	return piVersionPattern.FindString(output)
}

// piLoadEnvDenied are the environment variables the load check removes before it starts the harness's
// own loader. Pi is told which home to read by one variable, and the rest are the variables that point
// a run at a model or carry model credentials, so the loader resolves the installed session without
// reaching a provider.
var piLoadEnvDenied = []string{
	"PI_API_KEY",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"OPENROUTER_API_KEY",
	"GOOGLE_API_KEY",
	"GEMINI_API_KEY",
	"XAI_API_KEY",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AZURE_OPENAI_API_KEY",
}

// PiLoadOptions configures one native load verification.
type PiLoadOptions struct {
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

// VerifyPiLoad asks the target harness's own loader to open a copy of the converted session and report
// the conversation it built, with no model turn.
//
// The check installs a throw-away copy of the converted bytes into a scratch agent directory (a resumed
// Pi session gets the harness's own records appended to it, so driving the loader against the installed
// file would alter it), starts `pi --mode rpc` there with the environment variables that could reach a
// model removed, and asks the harness two questions: `get_state`, whose own answer must name this
// session and the number of messages the file holds, and `get_messages`, whose roles must be the ones
// the conversion wrote, in order. It then runs the same first question with a freshly generated id
// nothing installed under: that control arm must make the harness answer `No session found matching`,
// which is what makes the first arm an answer about *this* session rather than about a process that
// started.
//
// A protocol or version change that makes the harness refuse the file is reported as not-verified with
// the harness's own words, never as a pass.
func VerifyPiLoad(ctx context.Context, opts PiLoadOptions) LoadVerification {
	verification := LoadVerification{
		State:     VerificationStateNotVerified,
		Method:    loadMethodPi,
		SessionID: opts.SessionID,
		Path:      opts.Installed,
		Cwd:       opts.Cwd,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = PiBinaryDefault
	}
	path, err := resolvePiExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf(
			"%s is not on PATH, so the installed session's native load could not be verified", binary)
		return verification
	}
	verification.Binary = path

	before, beforeErr := installedDigest(opts.Installed)

	scratch, err := os.MkdirTemp("", "ccc-pi-load-")
	if err != nil {
		verification.Detail = fmt.Sprintf("a scratch data directory could not be created for the check: %v", err)
		return verification
	}
	defer os.RemoveAll(scratch)
	copyPath, _, err := InstallPiSession(PiSession{
		Directory: opts.Directory,
		FileName:  opts.FileName,
		Data:      opts.Data,
	}, scratch)
	if err != nil {
		verification.Detail = fmt.Sprintf("the check's own copy of the session could not be written: %v", err)
		return verification
	}
	sessionsRoot := filepath.Join(scratch, piSessionsSubdir)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = piLoadTimeout
	}
	loaderCwd, err := piLoaderCwd(scratch, opts.Cwd)
	if err != nil {
		verification.Detail = fmt.Sprintf(
			"the installed session's native load could not be verified: %v", err)
		return verification
	}
	state, err := piAskLoader(ctx, path, scratch, loaderCwd, opts.SessionID, opts.Data, timeout, opts.Stderr)
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
	_, controlErr := piAskLoader(ctx, path, scratch, loaderCwd, unknown, nil, timeout, opts.Stderr)
	if controlErr == nil {
		verification.Detail = fmt.Sprintf(
			"the control arm answered for session %q, which this check never installed, so the loader is "+
				"not answering from the installed file", unknown)
		return verification
	}
	if !strings.Contains(controlErr.Error(), "No session found matching") {
		verification.Detail = fmt.Sprintf(
			"the control arm did not report the unknown session as missing; it answered: %s",
			summariseText(controlErr.Error(), 300))
		return verification
	}
	planted, plantedErr := piSessionNamed(sessionsRoot, unknown)
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
			"unknown session id answered \"No session found matching\". The check ran against a "+
			"throw-away copy of the converted bytes (%s) with %s removed from the environment, so the "+
			"loader read the session without a model turn, and the installed file is unchanged (%s)",
		stateSummary(state), copyPath, strings.Join(piLoadEnvDenied, ", "), shortDigest(before))
	return verification
}

// piSessionNamed answers the path of a session file below root whose own name contains name, and "" when
// the tree holds none.
//
// The walk is over the whole tree and not over the sessions root's direct children: the harness files a
// session under the project directory it spells from the session's working directory
// (`<root>/--<project>--/<timestamp>_<id>.jsonl`), so a session a harness wrote while this check was
// asking about an unknown id sits one level below the root, where a listing of the root's own entries
// would never see it. A tree this cannot read is an error, because the answer "no such session was
// written" is exactly what the caller derives from finding none.
func piSessionNamed(root, name string) (string, error) {
	found := ""
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || found != "" {
			return nil
		}
		if strings.Contains(entry.Name(), name) {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return found, nil
}

// piLoadState is the part of the harness's own `get_state` answer this check reads.
type piLoadState struct {
	SessionFile  string `json:"sessionFile"`
	SessionID    string `json:"sessionId"`
	MessageCount int    `json:"messageCount"`
}

// piLoadTimeout bounds one load check arm. Measured: an RPC harness answered a written session in well
// under a second, and a harness that cannot answer is stopped by this deadline rather than by the
// caller's patience.
const piLoadTimeout = 90 * time.Second

// piRPCResponse is one answer. A session event carries other fields and no `type` of "response".
type piRPCResponse struct {
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// piAskLoader starts the harness in its own RPC mode against the scratch home, asks it for the state of
// one session and for that session's messages, and answers what its own loader said.
//
// The session is resolved by id under the scratch sessions root: the scratch home's project directory
// for the session's recorded working directory is where the copy was installed, and the process runs
// from that same directory, so the harness's own lookup finds it exactly the way the printed resume
// command makes it find the installed one. A lookup that finds nothing answers `No session found
// matching` on stderr and exits zero after starting a fresh in-memory session, so the missing answer --
// not the exit status -- is what this reads.
func piAskLoader(
	ctx context.Context, binary, home, cwd, sessionID string, data []byte,
	timeout time.Duration, stderr io.Writer,
) (piLoadState, error) {
	// The check asks the loader the way the printed resume command does: the session is named by id and
	// resolved through the agent directory's own default layout, which is where the copy was installed.
	// The agent directory is the environment's (piHomeEnv, set by runPiRPC), and the process runs from
	// the session's recorded working directory, because the harness resolves an id through the project
	// directory of the process's own working directory and a pair that disagrees is answered with a fork
	// offer rather than a load. Measured on the installed harness: naming the sessions tree with
	// `--session-dir` instead does not resolve the id at all, while the layout below resolves it and
	// answers `get_state` for it.
	arguments := []string{
		"--mode", "rpc",
		"--session", sessionID,
		"--no-tools",
		"--offline",
	}
	stdin := strings.Join([]string{
		`{"id":"state","type":"get_state"}`,
		`{"id":"messages","type":"get_messages"}`,
	}, "\n") + "\n"
	stdout, err := runPiLoaderRPC(ctx, binary, cwd, home, arguments, stdin, timeout, stderr)
	if err != nil {
		return piLoadState{}, err
	}
	state, err := readPiState(stdout)
	if err != nil {
		return piLoadState{}, err
	}
	if len(data) > 0 {
		if detail, err := piMessageDetail(stdout, data); err != nil {
			return piLoadState{}, err
		} else if detail != "" {
			return piLoadState{}, errors.New(detail)
		}
	}
	return state, nil
}

// piLoaderCwd is the working directory the load check's harness process runs in: the session's own
// recorded working directory.
//
// Measured, and the reason it is the recorded path rather than a stand-in inside the scratch tree: the
// harness resolves a session id through the project directory spelled from the process's own working
// directory, and the session's header records a working directory of its own. When the two paths are not
// the same path, the harness answers `Session found in different project: <the recorded one>` and offers
// to *fork* the session into the current directory instead of resuming it in place -- which is not what
// the conversation's own resume command does, and not what this check is verifying. A directory created
// inside the scratch tree cannot spell the same project unless the session's working directory was inside
// that tree to begin with, so the check runs from the recorded directory itself and leaves the session
// where the conversion put it: the harness reads the session and writes nothing on this arm.
//
// A recorded directory that does not exist here is an answer of its own: the harness would offer the same
// fork prompt (or fail to start), so the caller reports that the load could not be verified from the
// session's own working directory rather than reading a fork prompt as a pass.
func piLoaderCwd(_ string, sessionCwd string) (string, error) {
	trimmed := strings.TrimSpace(sessionCwd)
	if trimmed == "" {
		return "", errors.New("the session records no working directory")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", fmt.Errorf(
			"the session's recorded working directory %s is not readable here, and the harness resolves a "+
				"session id through the process's own working directory: %w", trimmed, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("the session's recorded working directory %s is not a directory", trimmed)
	}
	return trimmed, nil
}

// stateSummary is the harness's own view of the session, for the verification detail.
func stateSummary(state piLoadState) string {
	return fmt.Sprintf("sessionId %q, messageCount %d", state.SessionID, state.MessageCount)
}

// readPiState reads the `get_state` answer out of one RPC run's stdout.
func readPiState(stdout string) (piLoadState, error) {
	for _, response := range readPiResponses(stdout) {
		if response.Command != "get_state" {
			continue
		}
		if !response.Success {
			return piLoadState{}, fmt.Errorf("the harness refused get_state: %s", summariseText(response.Error, 200))
		}
		var state piLoadState
		if err := json.Unmarshal(response.Data, &state); err != nil {
			return piLoadState{}, fmt.Errorf("the harness's get_state answer could not be read: %w", err)
		}
		return state, nil
	}
	return piLoadState{}, errors.New("the harness answered no get_state response")
}

// piMessageAnswer is the harness's `get_messages` answer: the messages its own loader built from the
// file, as the harness serialises them.
type piMessageAnswer struct {
	Messages []struct {
		Role string `json:"role"`
	} `json:"messages"`
}

// piMessageDetail compares the roles and count the harness's loader reported with what the session
// file's own bytes say its model-visible history is, and answers a detail sentence naming the
// difference, or "" when they agree. It is the second half of the load check: the first reads the
// count out of `get_state`, and this asks the loader to name the messages themselves.
//
// A `compactionSummary` is how the loader reports the compaction entry it rebuilt its context from, and
// it stands where the entry stands in the file, so the comparison is between the file's own list of
// model-visible entries (with the boundary rendered as that one summary) and the loader's.
func piMessageDetail(stdout string, data []byte) (string, error) {
	for _, response := range readPiResponses(stdout) {
		if response.Command != "get_messages" {
			continue
		}
		if !response.Success {
			return "", fmt.Errorf("the harness refused get_messages: %s", summariseText(response.Error, 200))
		}
		var answer piMessageAnswer
		if err := json.Unmarshal(response.Data, &answer); err != nil {
			return "", fmt.Errorf("the harness's get_messages answer could not be read: %w", err)
		}
		want := piModelVisibleRoles(data)
		got := make([]string, 0, len(answer.Messages))
		for _, message := range answer.Messages {
			got = append(got, message.Role)
		}
		if len(got) == 0 {
			return "the harness's own loader built no messages from the installed session", nil
		}
		if len(got) != len(want) {
			return fmt.Sprintf(
				"the harness's loader built %d message(s) from the session file, while the file's own "+
					"model-visible history holds %d: the conversation the loader built is not the one the "+
					"file records", len(got), len(want)), nil
		}
		for index := range want {
			if got[index] != want[index] {
				return fmt.Sprintf(
					"the harness's loader read a %s at position %d, while the session file's own history "+
						"holds a %s there", got[index], index+1, want[index]), nil
			}
		}
		return "", nil
	}
	return "", errors.New("the harness answered no get_messages response")
}

// piModelVisibleRoles answers the roles one session file's model-visible history holds, in order: the
// latest compaction entry the harness's own loader finds *on the path it walks* (reported by the harness
// as `compactionSummary`), then every message entry after it on that path, and nothing off the path.
//
// The walk is the harness's own, re-derived from its answer rather than assumed (measured on the tested
// version): it starts at the file's last entry, follows parentId back to the tree's root, and reports the
// reversed path -- so an entry the file holds but no later record hangs off is recorded history and not
// context, and a compaction entry that stands off that path is not a boundary the loader rebuilds
// anything from (the inherited reader's own answer for a Pi tool result additionally *names* a record
// after a result as hanging off the message that made the call, which is why the path is read from the
// file's own parentId values and never from a document this build holds). The count and the roles cannot
// drift: both are this walk's own answer.
func piModelVisibleRoles(data []byte) []string {
	records := piConversationEntries(data)
	if len(records) == 0 {
		return nil
	}
	indexByID := make(map[string]int, len(records))
	for index, entry := range records {
		if entry.ID != "" {
			indexByID[entry.ID] = index
		}
	}
	// The path: from the file's last entry back through parentId, in file order. A parentId the file does
	// not hold, a record naming itself and a loop each end the walk where they are reached, which is the
	// same partial path the harness's own loader would have.
	reversed := make([]piConversationEntry, 0, len(records))
	visited := make(map[int]bool, len(records))
	for index := len(records) - 1; ; {
		if visited[index] {
			break
		}
		visited[index] = true
		entry := records[index]
		reversed = append(reversed, entry)
		if entry.ParentID == nil {
			break
		}
		next, ok := indexByID[*entry.ParentID]
		if !ok {
			break
		}
		index = next
	}
	path := make([]piConversationEntry, len(reversed))
	for index, entry := range reversed {
		path[len(reversed)-1-index] = entry
	}
	boundary := -1
	for index, entry := range path {
		if entry.Type == piEntryCompaction {
			boundary = index
		}
	}
	roles := make([]string, 0, len(path))
	if boundary >= 0 {
		roles = append(roles, "compactionSummary")
	}
	for _, entry := range path[boundary+1:] {
		if entry.Type != piEntryMessage {
			continue
		}
		roles = append(roles, entry.Message.Role)
	}
	return roles
}

// readPiResponses reads the RPC protocol's own response records out of stdout, in order. Session events,
// which describe activity rather than answer a command, are skipped; a line that is not JSON is skipped
// too, because the harness writes its diagnostics on stderr and a protocol change would otherwise be
// reported as a parse failure of the whole stream rather than as the missing answer it is.
func readPiResponses(stdout string) []piRPCResponse {
	var responses []piRPCResponse
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}
		var response piRPCResponse
		if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
			continue
		}
		if response.Type != "response" {
			continue
		}
		responses = append(responses, response)
	}
	return responses
}

// runPiRPC starts one RPC harness run, writes the given commands to its stdin, closes it, and answers
// its stdout. Both streams are bounded and drained on their own goroutines, so a harness that floods
// either pipe or never exits is answered by its deadline rather than by this process growing with it.
//
// The environment is the caller's own with the home variable set and the model variables removed: a
// host's PATH and locale still reach the harness, and nothing in it can reach a model.
func runPiRPC(
	ctx context.Context, binary, cwd, home string, arguments []string, stdin string,
	timeout time.Duration, stderr io.Writer,
) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, arguments...)
	command.Dir = cwd
	command.Env = piEnvironment(home, piLoadEnvDenied)
	command.Stdin = strings.NewReader(stdin)
	answer := newBoundedBuffer(harnessOutputMax)
	diagnostics := newBoundedBuffer(harnessDiagnosticsMax)
	command.Stdout = answer
	if stderr != nil {
		command.Stderr = io.MultiWriter(diagnostics, stderr)
	} else {
		command.Stderr = diagnostics
	}
	err := command.Run()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return answer.String(), fmt.Errorf("timed out after %s", timeout)
	}
	if err != nil {
		return answer.String(), fmt.Errorf("%v; stderr: %s", err, summariseText(diagnostics.String(), 300))
	}
	return answer.String(), nil
}

// runPiLoaderRPC is the seam the load check drives the target harness's RPC mode through. It is named so
// a test can answer as a harness whose protocol behaviour is not the host's business: the rule under
// test is what the check makes of the harness's answer, not which build of the harness is installed
// here, and the real subprocess path is exercised by scripts/pi-native-check.py against a real binary.
var runPiLoaderRPC = runPiRPC

// piEnvironment builds a child environment for one harness run: the caller's own environment with the
// variable that selects the agent directory set to home, the named variables removed, and the session
// directory variable removed too, so a leftover setting cannot point the harness at another tree.
func piEnvironment(home string, denied []string) []string {
	return childEnvironment(home, piHomeEnv, denied, piSessionDirEnv)
}

// PiResumeOptions configures one real resume turn.
type PiResumeOptions struct {
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

// VerifyPiResume runs one real turn in the installed session and reports what the harness did.
//
// It is the only state that proves a session continues: the invocation is the one the report prints, run
// from the session's own working directory (the directory the harness's lookup resolves the session
// through) with the caller's own environment, so the turn's model access is whatever the caller has
// configured -- this check never supplies credentials of its own. A missing binary, a harness that
// refuses the session and a harness that cannot reach a model are each reported as not-verified with the
// harness's own words. A harness that answers but leaves the installed session's bytes unchanged is
// refused the same way: the verified state says the harness wrote its own records into the session, and
// the two sizes it reports have to show it.
func VerifyPiResume(ctx context.Context, opts PiResumeOptions) ResumeVerification {
	verification := ResumeVerification{
		State:  VerificationStateNotVerified,
		Method: resumeMethodPi,
	}
	binary := opts.Binary
	if strings.TrimSpace(binary) == "" {
		binary = PiBinaryDefault
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		verification.Detail = "no resume prompt was given, and call/ccc does not invent one: it becomes part of the session"
		return verification
	}
	path, err := resolvePiExecutable(binary)
	if err != nil {
		verification.Detail = fmt.Sprintf("%s is not on PATH, so no turn could be run in the installed session", binary)
		return verification
	}
	// The invocation is the printed one: the session is named by id and resolved through its own data
	// directory's default layout, which is where the installer filed it.
	arguments := []string{"-p", "--session", opts.SessionID, promptTerminator, opts.Prompt}
	// The reported command is the invocation this check runs, the prompt included: a caller reading the
	// report sees exactly the line that produced the answer beside it.
	verification.Command = renderInvocation(append([]string{path}, arguments...),
		map[string]string{piHomeEnv: opts.Home}, opts.Cwd)

	before, beforeErr := fileSize(opts.SessionPath)
	if beforeErr != nil {
		verification.Detail = fmt.Sprintf("the installed session %s could not be measured before the turn: %v",
			opts.SessionPath, beforeErr)
		return verification
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = piResumeTimeout
	}
	answer, diagnostics, runErr := runPiResumeTurn(ctx, path, opts.Cwd, opts.Home, arguments, timeout, opts.Stderr)
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
	// The answer alone is not evidence that the turn continued the installed session: the sentence a
	// verified state reports says the harness wrote its own records into it, so the sizes must show it.
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

// piResumeTimeout bounds one real resume turn. A turn is a model call in the caller's own session, so it
// is given a generous deadline rather than the load check's.
const piResumeTimeout = 5 * time.Minute

// runPiTurn runs one non-interactive harness invocation and returns its stdout and stderr separately,
// both bounded. The command runs in the session's working directory when it exists, because the
// harness's own lookup resolves a session through the directory it is started in.
func runPiTurn(
	ctx context.Context, executable, cwd, home string, args []string, timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	return runRouteTurn(ctx, executable, cwd, args, piEnvironment(home, nil), timeout, stderr)
}

// runPiResumeTurn is the seam the resume check drives the target harness's turn through. It is named for
// the same reason runPiLoaderRPC is: what the check makes of the harness's answer is the rule under
// test, not which build of the harness a host happens to have installed, so a test answers the turn
// itself rather than standing or falling on the host's harness.
var runPiResumeTurn = runPiTurn

// resolvePiExecutable is the seam the two checks find the target harness through, named for the same
// reason the other routes' seams are: which build of the harness a host happens to have installed is
// not the rule under test, so a test answers the lookup with a path of its own rather than the checks
// standing or falling on the host's PATH.
var resolvePiExecutable = resolveTargetExecutable

// piExpectedMessageCount answers how many messages the harness's own loader builds from a session file
// that holds these bytes, counted the way its `messageCount` counts them.
//
// Measured on the tested version: `get_state`'s count is the loader's model-visible message list, which
// is the walk piModelVisibleRoles makes, so this answers that walk's own length -- the count and the
// roles cannot drift. It is computed from the file's own bytes rather than from the writer's document,
// and it is not simply the number of `message` entries: a session's context is built from its entries
// between the latest compaction boundary and the end of the path (the compaction itself becoming the
// `compactionSummary` entry), so entries the boundary replaced are not in the count, and each remaining
// message entry is. The file this writer writes has all of its entries on one path, so the count is: one
// for the latest compaction entry, plus every entry after it, plus nothing before it.
func piExpectedMessageCount(data []byte) int {
	return len(piModelVisibleRoles(data))
}

// piConversationEntry is the part of one entry these reads ask about: its record type, the tree link the
// model-visible walk follows, and, for a message entry, the role its payload records.
type piConversationEntry struct {
	Type     string  `json:"type"`
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Message  struct {
		Role string `json:"role"`
	} `json:"message"`
}

// piConversationEntries answers the file's own entries, in the file's own order, skipping the header and
// any line it cannot read.
func piConversationEntries(data []byte) []piConversationEntry {
	var records []piConversationEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry piConversationEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "" || entry.Type == piEntrySession {
			continue
		}
		records = append(records, entry)
	}
	return records
}

// piHomeEnv is the environment variable that selects the Pi data directory a run reads: the harness has
// no command-line flag for the directory itself, so a resume command that does not set it opens the
// caller's own default home rather than the directory the session was installed into.
const piHomeEnv = "PI_CODING_AGENT_DIR"

// piSessionDirEnv is the environment variable that relocates only the sessions tree. The checks remove
// it from the child environment: a leftover setting of the caller's would point the harness at another
// tree, and the flags the checks pass are what say which one to read.
const piSessionDirEnv = "PI_CODING_AGENT_SESSION_DIR"
