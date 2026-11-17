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
	// The sessions tree the invocation names: the project directory below the scratch root the copy was
	// installed into, which is where the installer files a session and what piResumeEnvironment names.
	copySessionDir := filepath.Join(sessionsRoot, opts.Directory)

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
	state, err := piAskLoader(ctx, path, scratch, copySessionDir, loaderCwd, opts.SessionID, opts.Data, timeout, opts.Stderr)
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
	_, controlErr := piAskLoader(ctx, path, scratch, copySessionDir, loaderCwd, unknown, nil, timeout, opts.Stderr)
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

// piRPCResponse is one answer, with only the fields this check asks about declared.
//
// A session event carries other fields and no `type` of "response". A `get_messages` answer carries the
// whole conversation's *contents* -- far more than any diagnostic bound -- and this check compares roles,
// so those members are left undeclared on purpose: the decode this type is read with (readPiResponses)
// scans a member it does not name past as the bytes arrive instead of holding it, and the answer to a
// large session's load is read with the same memory as a small one's.
type piRPCResponse struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Success bool              `json:"success"`
	Error   string            `json:"error"`
	Data    piRPCResponseData `json:"data"`
}

// piRPCResponseData is the part of an answer's `data` this check reads: the `get_state` fields and the
// `get_messages` roles, in the loader's own order.
type piRPCResponseData struct {
	SessionFile  string `json:"sessionFile"`
	SessionID    string `json:"sessionId"`
	MessageCount int    `json:"messageCount"`
	Messages     []struct {
		Role string `json:"role"`
	} `json:"messages"`
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
	ctx context.Context, binary, home, sessionDir, cwd, sessionID string, data []byte,
	timeout time.Duration, stderr io.Writer,
) (piLoadState, error) {
	// The check asks the loader the way the printed resume command does: the session is named by id and
	// resolved through the sessions tree the invocation names (piSessionDirEnv, set by piEnvironment) and
	// the process runs from the session's recorded working directory, because the harness resolves an id
	// through the project directory of the process's own working directory and a pair that disagrees is
	// answered with a fork offer rather than a load. Measured on 1.5.2: with the sessions tree named, the
	// harness resolves an id filed directly in it, which is where the installer files a session.
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
	responses, err := runPiLoaderRPC(ctx, binary, cwd, home, sessionDir, arguments, stdin, timeout, stderr)
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
//
// The recorded working directory is read exactly as the session recorded it: trimming decides whether a
// directory was named at all, and nothing else, because a directory's own name may begin or end with a
// space -- and the writer, the installation directory, the session header and the printed resume command
// all keep the recorded spelling. A trimmed stat and a trimmed answer would ask this host about a
// different directory than the one the session names: a session whose directory is `project ` would be
// reported as one the host does not have while the directory it names is there, and a run started in the
// trimmed spelling is a run from a directory the session did not record (resolveRouteHome reads its own
// value the same way, for the same reason).
func piLoaderCwd(_ string, sessionCwd string) (string, error) {
	if strings.TrimSpace(sessionCwd) == "" {
		return "", errors.New("the session records no working directory")
	}
	info, err := os.Stat(sessionCwd)
	if err != nil {
		return "", fmt.Errorf(
			"the session's recorded working directory %s is not readable here, and the harness resolves a "+
				"session id through the process's own working directory: %w", sessionCwd, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("the session's recorded working directory %s is not a directory", sessionCwd)
	}
	return sessionCwd, nil
}

// stateSummary is the harness's own view of the session, for the verification detail.
func stateSummary(state piLoadState) string {
	return fmt.Sprintf("sessionId %q, messageCount %d", state.SessionID, state.MessageCount)
}

// readPiState reads the `get_state` answer out of one RPC run's responses.
func readPiState(responses []piRPCResponse) (piLoadState, error) {
	for _, response := range responses {
		if response.Command != "get_state" {
			continue
		}
		if !response.Success {
			return piLoadState{}, fmt.Errorf("the harness refused get_state: %s", summariseText(response.Error, 200))
		}
		return piLoadState{
			SessionFile:  response.Data.SessionFile,
			SessionID:    response.Data.SessionID,
			MessageCount: response.Data.MessageCount,
		}, nil
	}
	return piLoadState{}, errors.New("the harness answered no get_state response")
}

// piMessageDetail compares the roles and count the harness's loader reported with what the session
// file's own bytes say its model-visible history is, and answers a detail sentence naming the
// difference, or "" when they agree. It is the second half of the load check: the first reads the
// count out of `get_state`, and this asks the loader to name the messages themselves.
//
// A `compactionSummary` is how the loader reports the compaction entry it rebuilt its context from, and
// it stands where the entry stands in the file, so the comparison is between the file's own list of
// model-visible entries (with the boundary rendered as that one summary) and the loader's.
func piMessageDetail(responses []piRPCResponse, data []byte) (string, error) {
	for _, response := range responses {
		if response.Command != "get_messages" {
			continue
		}
		if !response.Success {
			return "", fmt.Errorf("the harness refused get_messages: %s", summariseText(response.Error, 200))
		}
		want := piModelVisibleRoles(data)
		got := make([]string, 0, len(response.Data.Messages))
		for _, message := range response.Data.Messages {
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

// readPiResponses reads the RPC protocol's own response records out of a run's stdout, in order, as the
// harness writes them: each line is decoded on its own, and the decode names only the fields the check
// asks about (piRPCResponse), so a `get_messages` answer carrying a whole conversation's contents is
// scanned past rather than held -- a load answer is not bounded by anything here, and a legitimate
// history larger than any diagnostic bound is read the same way as a small one.
//
// Session events, which describe activity rather than answer a command, are skipped; a line that is not
// JSON is skipped too, because the harness writes its diagnostics on stderr and a protocol change would
// otherwise be reported as a parse failure of the whole stream rather than as the missing answer it is. A
// line the decoder does not read to its end is discarded by the next line's start (piProtocolLines), so a
// line's own malformed tail cannot hide the responses after it.
func readPiResponses(stdout io.Reader) ([]piRPCResponse, error) {
	lines := &piProtocolLines{reader: bufio.NewReader(stdout)}
	var responses []piRPCResponse
	for lines.next() {
		var response piRPCResponse
		if err := json.NewDecoder(lines).Decode(&response); err != nil {
			continue
		}
		if response.Type != "response" {
			continue
		}
		responses = append(responses, response)
	}
	return responses, lines.err()
}

// piProtocolLines reads one line at a time out of an RPC run's stdout, so a decoder can read a line's own
// JSON while it is still arriving: a piece of a long line is handed out and forgotten as the next is
// read, and the line's end is io.EOF for that decoder, so neither the decoder nor this reader ever holds
// a whole line, however large the answer is.
//
// A line is ended by its line feed (or by the stream's own end), and a piece the stream's buffer cuts is
// not a line end: a line longer than the buffer is handed out in as many pieces as it takes. A line the
// decoder abandons is drained by the following next(), so the line after it starts where the stream's own
// line feed says it does.
type piProtocolLines struct {
	reader *bufio.Reader
	// piece is the current line's bytes not yet handed to a reader. lineEnded reports that the stream will
	// answer no more of this line (its line feed was read, or the stream ended), and streamErr is what
	// ended the stream (io.EOF for a stream that simply ended); it is answered once the piece is handed
	// out.
	piece     []byte
	lineEnded bool
	streamErr error
	started   bool
}

// next advances past the current line and reports whether another line follows. The bytes the previous
// line's reader did not take are discarded; a stream that has ended answers false, after the line it
// ended with has been handed out. The end is asked with a peek, so a stream that ends right after a line
// feed answers "no line" here rather than leaving the caller to discover an empty one.
func (l *piProtocolLines) next() bool {
	if l.started {
		for !l.lineEnded {
			_, err := l.reader.ReadSlice('\n')
			switch {
			case err == nil:
				l.lineEnded = true
			case errors.Is(err, bufio.ErrBufferFull):
				// A piece of the same line still to come: read on.
			default:
				l.lineEnded = true
				l.streamErr = err
			}
		}
		if l.streamErr != nil {
			return false
		}
	} else {
		l.started = true
	}
	if _, err := l.reader.Peek(1); err != nil {
		if !errors.Is(err, io.EOF) {
			l.streamErr = err
		}
		return false
	}
	l.piece, l.lineEnded = nil, false
	return true
}

// Read answers the current line's bytes, a piece at a time. The line's end and the stream's end both
// answer io.EOF, which is what a decoder needs to read the line's own value: a line whose last piece had
// no line feed (a stream that ended mid-line) is answered the same way rather than being lost.
func (l *piProtocolLines) Read(p []byte) (int, error) {
	for len(l.piece) == 0 {
		if l.lineEnded {
			return 0, io.EOF
		}
		piece, err := l.reader.ReadSlice('\n')
		l.piece = piece
		switch {
		case err == nil:
			l.lineEnded = true
		case errors.Is(err, bufio.ErrBufferFull):
			// A piece of a longer line: handed out now, the rest read when it is spent.
		default:
			l.lineEnded = true
			l.streamErr = err
		}
	}
	n := copy(p, l.piece)
	l.piece = l.piece[n:]
	return n, nil
}

// err answers the read error that ended the stream, and nil for a stream that simply ended: io.EOF is
// the protocol's own end of output, not a failure of the run.
func (l *piProtocolLines) err() error {
	if errors.Is(l.streamErr, io.EOF) {
		return nil
	}
	return l.streamErr
}

// runPiRPC starts one RPC harness run, writes the given commands to its stdin, closes it, and answers the
// protocol responses its own loader wrote, read as the harness writes them (readPiResponses). The
// harness's stderr is still bounded and drained on its own goroutine, and the run keeps its deadline, so
// a harness flooding its diagnostics or never exiting is answered by an excerpt and the deadline rather
// than by this process growing with it; its *answers* are not cut, because a `get_messages` answer is a
// conversation and the check's questions cannot be answered by an excerpt of one.
//
// The environment is the caller's own with the home variable set and the model variables removed: a
// host's PATH and locale still reach the harness, and nothing in it can reach a model.
func runPiRPC(
	ctx context.Context, binary, cwd, home, sessionDir string, arguments []string, stdin string,
	timeout time.Duration, stderr io.Writer,
) ([]piRPCResponse, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, binary, arguments...)
	command.Dir = cwd
	command.Env = piEnvironment(home, sessionDir, piLoadEnvDenied)
	command.Stdin = strings.NewReader(stdin)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("connecting to the harness's own output: %w", err)
	}
	diagnostics := newBoundedBuffer(harnessDiagnosticsMax)
	if stderr != nil {
		command.Stderr = io.MultiWriter(diagnostics, stderr)
	} else {
		command.Stderr = diagnostics
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	// The answers are read to their end before waiting: exec closes the pipe when the process exits, so
	// Wait is called only once no more answers can arrive.
	responses, readErr := readPiResponses(stdout)
	waitErr := command.Wait()
	if runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return responses, fmt.Errorf("timed out after %s", timeout)
	}
	if waitErr != nil {
		return responses, fmt.Errorf("%v; stderr: %s", waitErr, summariseText(diagnostics.String(), 300))
	}
	if readErr != nil {
		return responses, fmt.Errorf("reading the harness's own answers: %w", readErr)
	}
	return responses, nil
}

// runPiLoaderRPC is the seam the load check drives the target harness's RPC mode through. It is named so
// a test can answer as a harness whose protocol behaviour is not the host's business: the rule under
// test is what the check makes of the harness's answer, not which build of the harness is installed
// here, and the real subprocess path is exercised both by scripts/pi-native-check.py against a real
// binary and by the runner's own case against a stub process
// (TestPiRPCCarriesAnAnswerLargerThanTheExcerptBound).
var runPiLoaderRPC = runPiRPC

// piEnvironment builds a child environment for one harness run: the caller's own environment with the
// variable that selects the agent directory set to home, the named variables removed, and the sessions
// tree named explicitly as the directory the session was installed into. Naming it is what makes the
// check run the printed invocation's own environment (piResumeEnvironment): Pi reads a session directory
// from PI_CODING_AGENT_SESSION_DIR ahead of the agent directory's default, so a caller's leftover setting
// pointed the printed line at another tree while this check, which removed the variable, looked in the
// default one -- a verified lookup the printed command did not make. Both halves now name the same tree.
func piEnvironment(home, sessionDir string, denied []string) []string {
	return childEnvironment(home, piHomeEnv, denied, map[string]string{piSessionDirEnv: sessionDir})
}

// piResumeEnvironment is the environment both the printed resume line and the resume check run the
// harness with: the agent directory the session was installed into, and the sessions tree inside it that
// holds the file. Both halves are explicit because Pi has no flag for either, and the session directory
// is the project directory the installer filed the session under -- measured on 1.5.2: naming the
// default `<home>/sessions` root does not resolve an id filed one project directory below it, while
// naming the project directory itself does.
func piResumeEnvironment(home, projectDir string) map[string]string {
	return map[string]string{
		piHomeEnv:       home,
		piSessionDirEnv: piSessionTree(home, projectDir),
	}
}

// piSessionTree is the directory one converted session is installed into and looked up in: the default
// sessions tree inside the agent directory, one project directory below it. The installer and both halves
// of the invocation name this one spelling, so the printed line, the check and the file agree.
func piSessionTree(home, projectDir string) string {
	return filepath.Join(home, piSessionsSubdir, projectDir)
}

// piResumeArguments is the one non-interactive resume invocation: print mode, the session's id, the
// option terminator and the turn's words. Both the line the report prints and the check that runs a real
// turn call it, so the invocation a caller pastes is the one that was verified. Print mode is not
// decoration: measured on 1.5.2, `pi --session <id> -- <prompt>` with terminal stdin and stdout opens the
// TUI and never runs the turn.
func piResumeArguments(sessionID, prompt string) []string {
	return []string{"-p", "--session", sessionID, promptTerminator, prompt}
}

// PiResumeOptions configures one real resume turn.
type PiResumeOptions struct {
	Home      string
	Binary    string
	SessionID string
	// ProjectDir is the project directory below Home's sessions tree that the session was installed
	// under, which is the sessions tree the invocation names (piResumeEnvironment): the check runs the
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
	// The invocation is the printed one -- piResumeArguments is what the route's printed line builds too
	// -- with the printed environment: the agent directory the session was installed into and the
	// sessions tree inside it, so a caller's leftover session-directory setting cannot make the printed
	// line search somewhere this check did not verify.
	arguments := piResumeArguments(opts.SessionID, opts.Prompt)
	// The reported command is the invocation this check runs, the prompt included: a caller reading the
	// report sees exactly the line that produced the answer beside it.
	verification.Command = renderInvocation(append([]string{path}, arguments...),
		piResumeEnvironment(opts.Home, opts.ProjectDir), opts.Cwd)

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
	answer, diagnostics, runErr := runPiResumeTurn(ctx, path, opts.Cwd, opts.Home,
		piSessionTree(opts.Home, opts.ProjectDir), arguments, timeout, opts.Stderr)
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
	ctx context.Context, executable, cwd, home, sessionDir string, args []string, timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	return runRouteTurn(ctx, executable, cwd, args, piEnvironment(home, sessionDir, nil), timeout, stderr)
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

// piConversationEntries answers a converted file's own entries, in the file's own order, skipping the
// header and any line it cannot read. It is the byte-shaped reading of the family's entry scan (a
// converted file is in hand as bytes, not as a stream): the bytes are already held, so every line of the
// file is read, whatever its size.
//
// The writer imposes no bound of its own on an entry -- a source record this build can carry is written
// as it was read -- so a bound here would be a bound the writer never promised, and a line beyond it
// would leave the caller judging a *prefix* of the file: a large entry standing between the file's real
// leaf and the entries before it would move the leaf the walk starts from and answer the loader's
// correct conversation as a mismatch. This read's error channel is the file-backed scan's
// (piScanConversationEntries); an answer is always the whole file.
func piConversationEntries(data []byte) []piConversationEntry {
	var records []piConversationEntry
	for len(data) > 0 {
		line := data
		if index := bytes.IndexByte(data, '\n'); index >= 0 {
			line, data = data[:index], data[index+1:]
		} else {
			data = nil
		}
		if entry, ok := piEntryFromLine(line); ok {
			records = append(records, entry)
		}
	}
	return records
}

// piEntryFromLine reads one line of the session format, answering false for a line this family's reads
// pass over: a blank line, a line that is not JSON, the session header, and a record that names no kind.
// It is the one reader of the entry shape both scans use -- the file-backed scan answers a line beyond
// its bound as its error, the byte-backed read answers every line -- so the shape two callers read is
// decided once, here.
func piEntryFromLine(line []byte) (piConversationEntry, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return piConversationEntry{}, false
	}
	var entry piConversationEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return piConversationEntry{}, false
	}
	if entry.Type == "" || entry.Type == piEntrySession {
		return piConversationEntry{}, false
	}
	return entry, true
}

// piHomeEnv is the environment variable that selects the Pi data directory a run reads: the harness has
// no command-line flag for the directory itself, so a resume command that does not set it opens the
// caller's own default home rather than the directory the session was installed into.
const piHomeEnv = "PI_CODING_AGENT_DIR"

// piSessionDirEnv is the environment variable that names the sessions tree itself. Both the printed
// invocation and the checks set it to the directory the session was installed into (piSessionTree),
// because Pi resolves a session directory as `--session-dir` first, then this variable, then the data
// directory's own `settings.json` `sessionDir` setting, and only then the default tree: a caller's
// leftover setting, or a settings file naming another directory, would otherwise send the printed
// command looking for a session the check found. Measured on 1.5.2: with `settings.json` naming another
// directory and this variable naming the project directory, the loader resolved the id; with the
// variable unset on the same data directory it answered `No session found matching`; and naming the
// `<home>/sessions` root does not resolve an id filed one project directory below it, while naming the
// project directory itself does.
const piSessionDirEnv = "PI_CODING_AGENT_SESSION_DIR"
