package ccc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The three routes -- Codex, Claude Code and Pi -- carry the same install/verify/probe/turn machinery
// three times over, differing only in a handful of route-specific facts (their subdirectory, their mode
// constant, the inherited reader each verifies through, the environment variable that selects their data
// directory, their default executable and the shape of a version line). This file holds that machinery
// once, with each difference passed in, so a later change lands in one place instead of three. The route
// files keep their own option types, method constants, agent constant, target-path join, round-trip
// comparator and blocked-environment names as their readable face.

// harnessOutputMax and harnessDiagnosticsMax bound what one harness run may add to this process's
// memory, for both the RPC arm and the turn arm of the checks. The answer a report quotes is cut to
// boundedText's own limit anyway (8192 bytes by default), and a harness flooding either stream is
// answered by its deadline and an excerpt rather than by this process growing with every byte it writes.
// The diagnostics bound is the size the Codex app server keeps of its own, 4096 bytes.
const (
	harnessOutputMax      = 1 << 20
	harnessDiagnosticsMax = 4096
)

// entryClock hands out timestamps that never go backwards: each target orders its records by the
// timestamp it records, and a source whose messages are unordered (a DAG, a replayed fork) would
// otherwise write a file whose order contradicts its own times. The layout is the route's own: the Codex
// and Claude routes write `time.RFC3339Nano`, and Pi writes milliseconds, always three digits, always
// UTC (`2026-11-10T02:07:01.784Z`, piEntryTimestampLayout).
type entryClock struct {
	last   time.Time
	layout string
}

func newEntryClock(start time.Time, layout string) *entryClock {
	return &entryClock{last: start.UTC(), layout: layout}
}

func (c *entryClock) next(at time.Time) string {
	at = at.UTC()
	if at.IsZero() || at.Before(c.last) {
		at = c.last
	}
	c.last = at
	return at.Format(c.layout)
}

// ledger accumulates the bookkeeping the three writers share while they walk a document: the calls
// carried with no result yet, and every tool name seen so far in the order it was first written.
type ledger struct {
	// pendingCalls holds the carried calls no result has answered yet. It is what an id-less result may
	// be paired with, and only while exactly one call is outstanding: with two or more, the result could
	// belong to any of them, and that ambiguity must not disappear just because a further call arrived.
	pendingCalls     map[string]struct{}
	foreignToolNames map[string]struct{}
	toolNameOrder    []string
}

// solePendingCall answers the one call an id-less result may belong to: exactly one carried call with
// no result yet.
func (l *ledger) solePendingCall() (string, bool) {
	if len(l.pendingCalls) != 1 {
		return "", false
	}
	for callID := range l.pendingCalls {
		return callID, true
	}
	return "", false
}

// noteToolName records one tool name in the order it is first written.
func (l *ledger) noteToolName(name string) {
	if l.foreignToolNames == nil {
		l.foreignToolNames = map[string]struct{}{}
	}
	if _, ok := l.foreignToolNames[name]; ok {
		return
	}
	l.foreignToolNames[name] = struct{}{}
	l.toolNameOrder = append(l.toolNameOrder, name)
}

// distinctCalls answers one message's calls with the calls whose id the message already carried dropped,
// in the message's own order: such a call is another projection of a call already taken (the inherited
// Codex reader expands one apply_patch into one call per file it names, every projection sharing the id
// and the input) and not a second invocation, so only the first is kept and every dropped projection
// charges the caller's own loss counter. A call with no recorded id is never treated as a repeat: its id
// is generated from its own place in the message, so two of them are two calls.
func distinctCalls[C any](message Message, project func(index int, call ToolCall) C, coalesced *int) []C {
	calls := make([]C, 0, len(message.ToolCalls))
	seen := make(map[string]struct{}, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		id := strings.TrimSpace(call.ToolUseID)
		if id != "" {
			if _, ok := seen[id]; ok {
				*coalesced++
				continue
			}
			seen[id] = struct{}{}
		}
		calls = append(calls, project(index, call))
	}
	return calls
}

// installFile writes one converted session into a target data directory and returns the path it
// occupies, plus the mechanism that put it there. The route names only what differs: the subdirectory it
// installs under, the project directory inside it, the file name, the bytes and the mode.
//
// Nothing existing is ever replaced: the bytes go to a sibling temporary file first and the final name is
// taken with a hard link, which is atomic where the filesystem has hard links and exclusive everywhere,
// so a taken name fails instead of being overwritten. A filesystem without hard links (some Windows
// volumes, some network mounts) has no exclusive way to publish the finished file, so the installation is
// refused (ErrNoExclusiveInstall) rather than published by rename: a rename replaces whatever is at the
// destination, which is exactly the overwrite this rule forbids. A reader therefore never sees a
// half-written session, and a conversion cannot replace a session another agent may already have
// continued.
//
// The returned path and the error are independent answers. A non-empty path means the session is
// published, and a non-nil error beside it is about something other than the publication itself -- a
// temporary file that could not be removed, reported rather than left behind in silence. A call that
// published nothing answers with an empty path, so the caller reads the two apart instead of inferring
// publication from the error.
func installFile(home, subdir, projectDir, fileName string, data []byte, mode os.FileMode) (string, string, error) {
	directory := filepath.Join(home, subdir, filepath.FromSlash(projectDir))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", directory, err)
	}
	target := filepath.Join(directory, fileName)
	if _, err := os.Lstat(target); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("checking %s: %w", target, err)
	}

	temp, err := os.CreateTemp(directory, "."+fileName+".tmp-")
	if err != nil {
		return "", "", fmt.Errorf("creating a temporary session file in %s: %w", directory, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		removeFile(tempName)
	}
	if err := temp.Chmod(mode); err != nil {
		cleanup()
		return "", "", fmt.Errorf("restricting %s: %w", tempName, err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return "", "", fmt.Errorf("writing %s: %w", tempName, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", "", fmt.Errorf("flushing %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		removeFile(tempName)
		return "", "", fmt.Errorf("closing %s: %w", tempName, err)
	}

	if err := linkSession(tempName, target); err != nil {
		removeFile(tempName)
		if errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("%w: %s", ErrTargetExists, target)
		}
		return "", "", fmt.Errorf("%w: %s: %v; install into a filesystem that supports hard links",
			ErrNoExclusiveInstall, directory, err)
	}
	if err := removeFile(tempName); err != nil {
		return target, "hard-link", fmt.Errorf("removing the temporary file %s: %w", tempName, err)
	}
	return target, "hard-link", nil
}

// verifyStructure reads an installed session back through the inherited AgentsView reader the route
// names and compares it, item by item, with what the writer intended to carry. sessionsDir is the
// target's own directory; targetPath is the file the conversion published (or the dry run's copy of the
// same bytes); cfg supplies the provider configuration (its roots for the route's agent are replaced,
// never mutated: the map is copied); compare walks the read-back against the file's own bytes with the
// route's reader rules.
//
// The read-back is the file this call names, read through the inherited reader's own path reference, so
// the check compares the file the conversion published rather than resolving an id through the target's
// session tree, which could answer with another copy of it (an id lookup enumerates every dated
// directory under the root and prefers the lexically first copy of the id, so an unrelated copy -- or
// simply a home with a long history -- would decide what is compared and how much of the home is walked).
func verifyStructure(
	ctx context.Context,
	sessionsDir string,
	agent parser.AgentType,
	method, readerMessage, readerLabel, targetPath string,
	compare func(readBack Conversation, installed []byte) roundTripComparison,
	cfg config.Config,
) (StructureVerification, error) {
	verification := StructureVerification{
		State:      VerificationStateNotVerified,
		Method:     method,
		TargetPath: targetPath,
	}
	readerConfig := cfg
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for name, roots := range cfg.AgentDirs {
		dirs[name] = roots
	}
	dirs[agent] = []string{sessionsDir}
	readerConfig.AgentDirs = dirs
	readerConfig.DisabledAgents = nil

	conversation, err := NewSourceReader(readerConfig).Read(ctx,
		Ref{Provider: string(agent), ID: verification.TargetPath})
	if err != nil {
		verification.Detail = "the inherited " + readerMessage + " reader could not read the installed session back: " + err.Error()
		return verification, nil
	}
	verification.ReadBack = readBackCounts(conversation)
	installed, err := os.ReadFile(verification.TargetPath)
	if err != nil {
		verification.Detail = "the installed session could not be read back for comparison: " + err.Error()
		return verification, nil
	}
	roundTrip := compare(conversation, installed)
	verification.NotReadBackVerbatim = roundTrip.notReadBackVerbatim
	if len(roundTrip.differences) > 0 {
		verification.Detail = "the installed session does not hold the evidence in the target document: " +
			strings.Join(roundTrip.differences, "; ")
		return verification, nil
	}
	verification.State = VerificationStateVerified
	verification.Detail = roundTrip.detail(readerLabel)
	return verification, nil
}

// plainRoundTripReader is the inherited reader's own rules for the read-back walk as the Codex and Pi
// readers share them: it marks no record as injected context (neither reader has such a classification),
// a tool record is no message of its own (the reader pairs the tool's output onto the call's message),
// and any other role the reader reads dialogue for stands for the assistant's turn -- the reader's own
// answer, which the check reports rather than second-guesses. The Claude reader's rules differ (it
// classifies injected context away and knows only the user and assistant roles) and are spelled per
// route.
func plainRoundTripReader() roundTripReader {
	return roundTripReader{
		classifiesAway: func(Message) bool { return false },
		kindOf: func(message Message) (TargetItemKind, bool) {
			switch parser.RoleType(message.Role) {
			case parser.RoleUser:
				return ItemUser, true
			case parser.RoleTool:
				return "", false
			default:
				return ItemAssistant, true
			}
		},
	}
}

// probeBinary finds a route's harness executable and asks it for its version: the shape the three routes'
// probes share. A missing binary is an answer, not an error: a conversion can still be written (and
// verified structurally), but the report says the native half could not run. versionOf reads the version
// out of the `--version` answer, and noVersion words the sentence the probe reports when that answer
// names none -- both the route's own, so each route's report reads as it always did.
func probeBinary(
	ctx context.Context, binary, defaultBinary string,
	versionOf func(output string) string,
	noVersion func(path, output string) string,
) harnessProbe {
	if strings.TrimSpace(binary) == "" {
		binary = defaultBinary
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
	if version := versionOf(output); version != "" {
		probe.Version = version
		return probe
	}
	probe.Detail = noVersion(path, output)
	return probe
}

// childEnvironment builds a child environment for one harness run: the caller's own environment with the
// variable that selects the route's data directory (homeEnv) set to home, the named variables removed,
// and any further names in alsoDeny removed too. Nothing else is changed, so a host's PATH, locale and
// tool configuration still reach the harness.
//
// Every name is read in its own upper case, the way the denylist and os/exec itself read one: Windows'
// environment is case-insensitive and os/exec keeps the last value among the names it is handed, so a
// leftover lower- or mixed-case spelling of the home variable would, after the sort below, sit after the
// appended override and win there -- the check would then read the caller's own home instead of the
// directory it installed into.
func childEnvironment(home, homeEnv string, denied []string, alsoDeny ...string) []string {
	blocked := make(map[string]struct{}, len(denied)+len(alsoDeny))
	for _, name := range denied {
		blocked[strings.ToUpper(name)] = struct{}{}
	}
	for _, name := range alsoDeny {
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
		if folded == homeEnv {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, homeEnv+"="+home)
	sort.Strings(environment)
	return environment
}

// runRouteTurn runs one non-interactive harness invocation and returns its stdout and stderr separately,
// both bounded. The command runs in cwd when it exists, because the harness's own lookup resolves a
// session through the directory it is started in, and runs with the environment the caller passes (the
// caller's own with the route's data-directory variable set and the model-reaching names removed).
func runRouteTurn(
	ctx context.Context, executable, cwd string, args []string, environment []string,
	timeout time.Duration, stderr io.Writer,
) (string, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, executable, args...)
	// The directory is the source's own recorded value: trimming decides only whether one was recorded,
	// because a directory name may end in whitespace.
	if dir := cwd; strings.TrimSpace(dir) != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			command.Dir = dir
		}
	}
	command.Env = environment
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
		return answer.String(), diagnostics.String(), fmt.Errorf("timed out after %s", timeout)
	}
	return answer.String(), diagnostics.String(), err
}

// resolveRouteHome answers the data directory a route's conversion installs into: the caller's explicit
// directory, else the route's environment variable, else its default under the user's home. The boolean
// reports whether the answer came from the caller, which is what decides whether a missing directory may
// be created.
//
// A value is read to answer two things, and only the first of them trims: trimming decides whether a
// directory was named at all, and the value is then resolved exactly as it was written, because a
// directory name may begin or end with a space.
func resolveRouteHome(explicit, homeEnv string, defaultDir ...string) (string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		home, err := filepath.Abs(explicit)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s: %w", explicit, err)
		}
		return home, true, nil
	}
	if fromEnv := os.Getenv(homeEnv); strings.TrimSpace(fromEnv) != "" {
		home, err := filepath.Abs(fromEnv)
		if err != nil {
			return "", false, fmt.Errorf("resolving $%s: %w", homeEnv, err)
		}
		return home, false, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolving the home directory: %w", err)
	}
	return filepath.Join(append([]string{dir}, defaultDir...)...), false, nil
}
