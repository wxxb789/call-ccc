package ccc

import (
	"bytes"
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
	"unicode/utf8"

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
//
// The installation has three answers, not two: whether the session is installed, is not, or -- where the
// installation is another process's write -- could not be established. The third is the answer a run
// owes when the target harness's own importer failed after it may have committed (a deadline, a
// cancellation) and the harness's own reader could not say either way; reporting that as
// InstallationStateNotInstalled would assert an absence nobody observed, and reporting it as
// InstallationStateInstalled would assert a publication nobody observed. It is reported by the installer
// through ErrInstallationUnresolved (see opencode_install.go, the route whose installation is the
// harness's own importer).
const (
	ConversionStateConverted      = "converted"
	InstallationStateInstalled    = "installed"
	InstallationStateNotInstalled = "not-installed"
	InstallationStateUnresolved   = "unresolved"
)

// ErrInstallationUnresolved reports an installation whose outcome the run could not establish: the
// installer ran, it did not observe its own success, and the evidence that could resolve the question
// answered nothing. It exists so the report can say `unresolved` instead of `not-installed`, which is a
// claim about the target's store that such a run has no basis for.
var ErrInstallationUnresolved = errors.New("the installation's outcome could not be established")

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

// Source-boundary states: whether a conversion's snapshot of the source was measured against the source
// file's own bytes at all. There are two answers, not three -- a boundary was taken, or it was not and the
// detail says what kept it from being taken (an archive read has no source file open; a container member's
// bytes are not one file's) -- and folding the second into an empty block would leave a caller unable to
// tell "no file to measure" from "this build measured nothing".
const (
	SourceBoundaryTaken    = "taken"
	SourceBoundaryNotTaken = "not-taken"
)

// Read-coverage states: what the file's bytes at the boundary have to do with the read's own snapshot of
// it. They are taken from the read's own record of the file it opened (ParsedSession.File.Size), which is
// a size and not a digest of the bytes it read, so the comparison a conversion can support is a size
// comparison: the file is the size the read recorded, or it is not -- and a source whose size moved while
// it was being read says so instead of attributing a conversation to bytes it may not have read.
//
// Neither state is a byte-identity claim, which is why neither is named "verified": a file replaced by
// different bytes of the same length after the read leaves every size equal, and no recorded artifact can
// tell the replacement from what the read opened -- the states therefore say what was compared, and the
// detail beside them says what a size cannot show.
const (
	SourceBoundaryCoverageSizeMatch    = "size-match"
	SourceBoundaryCoverageSizeMismatch = "size-mismatch"
)

// Record-offset states: whether the two offsets beside them are claimed for this file. Both halves are the
// provider's and the file's own to answer -- the provider's declarations say its source is a transcript of
// appended line records, and the file's own bytes have to be able to be that transcript -- so a claim is
// never this build's guess about a format, and a file whose bytes are a container of the records gets the
// file's bytes and digest with the offsets omitted.
const (
	SourceBoundaryRecordsClaimed    = "claimed"
	SourceBoundaryRecordsNotClaimed = "not-claimed"
)

// Why the record offsets were not claimed. The reasons are answers, not prose: a caller that reads only
// the machine fields still learns whether the provider declares such a source at all or whether this file's
// own bytes are something else, which are different things to do about it (one is the provider's format,
// the other is this representation of it).
const (
	recordOffsetsNotDeclared = "the provider's own declarations do not make this source a transcript of " +
		"appended line records"
	recordOffsetsNotText = "the file's own bytes are not a transcript of text lines (a compressed or " +
		"otherwise binary container of the records, or a sequence that does not decode as text), so no byte " +
		"offset into them is a record boundary"
)

// SourceBoundary is where one conversion's snapshot of the source stops: the source file's own bytes at the
// boundary instant (its size and the digest of exactly those bytes), how those bytes relate to the read's own
// snapshot of the file (a size comparison -- see the read-coverage states -- since that snapshot holds no
// digest of what it opened), and -- for a source whose records are the lines a harness appends -- the offset
// at which the file's last line-feed-terminated record ends and the bytes of the record after it that the
// file had not terminated.
//
// It exists because a conversion is a one-way snapshot of a source that may keep growing: the digest and
// the byte offsets are what make two conversions of one source distinguishable and each one attributable to
// the file it read, the coverage answer says what a size comparison can establish about that attribution,
// and the unterminated tail is the honest answer to a source captured while it was still being written --
// a tail the inherited readers handle differently (the Claude reader counts an unterminated
// trailing record as malformed and flags the session truncated; the Codex reader deliberately counts
// nothing for a live session's tail) and that this block states from the file's own bytes.
//
// It claims only what bytes can show. An unterminated tail is not called unpublished: whether the read
// carried one is the reader's own answer (an unterminated record that is a complete record is read; one the
// reader cannot read is dropped and counted where the provider counts such lines), and the read's fidelity
// and the conversion's own counts are where that answer is. What this block adds is the byte fact the read
// may not have stated at all: the file's bytes end without a line feed, so the source may still be writing
// that record.
type SourceBoundary struct {
	// State is SourceBoundaryTaken or SourceBoundaryNotTaken.
	State string `json:"state"`
	// Detail is the state's own words: how the boundary was taken, or what kept it from being taken.
	Detail string `json:"detail,omitempty"`

	// Path is the source session's own file, as the read recorded it.
	Path string `json:"path,omitempty"`
	// Bytes is that file's size at the boundary instant, and SHA256 the digest of exactly those bytes. They
	// are taken by the same streamed pass the pre-conversion comparison uses, so a conversion costs one
	// read of the file, not two, for both answers.
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`

	// ReadBytes is the size the read itself recorded for this file when it opened it (the provider's own
	// ParsedSession.File.Size). It is filled only where the provider's own declarations make that size this
	// file's own -- the two providers whose source is a transcript of appended lines are the two that record
	// the transcript's own size -- and is absent where the read recorded none, because comparing a size this
	// build may not read as a file's own would state a move that did not happen.
	ReadBytes int64 `json:"read_bytes,omitempty"`
	// ReadCoverage is SourceBoundaryCoverageSizeMatch when the file's size is the size the read's own
	// snapshot recorded for it, and SourceBoundaryCoverageSizeMismatch when it is not: the file's size moved
	// while it was being read, so which of these bytes the conversion covered is not established.
	// ReadCoverageDetail says which two sizes were compared and what a size comparison can and cannot show
	// (a size match is not byte identity, and this block claims no more than it compared). Empty when there
	// was no size to compare.
	ReadCoverage string `json:"read_coverage,omitempty"`
	// ReadCoverageDetail is that state's own words.
	ReadCoverageDetail string `json:"read_coverage_detail,omitempty"`

	// RecordOffsets is SourceBoundaryRecordsClaimed when the two offsets below are claimed for this file and
	// SourceBoundaryRecordsNotClaimed when this block states the file's bytes and claims no record boundary.
	// RecordOffsetsDetail names what kept them from being claimed.
	RecordOffsets string `json:"record_offsets"`
	// RecordOffsetsDetail is why the offsets were not claimed (recordOffsetsNotDeclared,
	// recordOffsetsNotText), absent when they were.
	RecordOffsetsDetail string `json:"record_offsets_detail,omitempty"`

	// TerminatedRecordsEndBytes is the offset just after the file's last line feed: where the file's
	// line-feed-terminated records end. Equal to Bytes when the file itself ends at a line feed. The bytes
	// after it (UnterminatedTailBytes) are the file's unterminated record -- one record, because only the
	// last line of a file can lack its terminator.
	TerminatedRecordsEndBytes int64 `json:"terminated_records_end_bytes,omitempty"`
	// UnterminatedTailBytes counts the file's unterminated record: the bytes after
	// TerminatedRecordsEndBytes, which no line feed follows. The source may still be extending it -- a
	// writer appends a record's line feed last -- so this is the byte fact behind "the file holds a record
	// this snapshot may not": whether the read carried it is the read's own answer, and another conversion
	// once the source has been written is what reads a finished one.
	UnterminatedTailBytes int64 `json:"unterminated_tail_bytes,omitempty"`
}

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

// SourceReads is the reading a continue run needs from a source reader: one recorded session by its
// reference. *SourceReader is this build's implementation; the interface exists so a caller -- an
// embedder, or the pair-gate's own case -- can hand in a reader that observes whether a run read
// anything at all, which is the only way "refused before the source is opened" is shown rather than
// inferred from an error message. Where the reader also carries the two answers this build's own reader
// can give -- the configuration it was built with, and a provider's own declaration that its source is a
// transcript of appended line records -- they are asked through a type assertion; an injected reader
// that carries neither is read conservatively (the caller's Config is completed from disk, and a
// boundary claims no record offsets), never answered with a guess.
type SourceReads interface {
	Read(ctx context.Context, ref Ref) (Conversation, error)
}

// ContinueOptions configures one conversion: what to read, where to write it, and how far to verify.
type ContinueOptions struct {
	// Ref identifies the source session, exactly as Inspect does.
	Ref Ref

	// Target names the harness the conversation continues in: TargetCodex or TargetClaude, in the words
	// Targets names them. Any other name is refused.
	Target string

	// Config supplies the provider roots and enabled agents, as in InspectOptions.
	Config config.Config
	// Reader, when set, is used instead of building one from Config. It is an interface (SourceReads) so
	// a caller can hand in a reader that observes the reads themselves -- the pair gate's promise that a
	// refused pair costs no read is only observable at the reader -- and *SourceReader is what this build
	// passes.
	Reader SourceReads
	// ArchivePath is an AgentsView archive consulted when the source session is not readable from its
	// own file. Empty means "source only".
	ArchivePath string

	// TargetHome is the target harness's data directory. Empty means "resolve it from the environment,
	// the way the target's own route resolves it": $CODEX_HOME, else ~/.codex for Codex;
	// $CLAUDE_CONFIG_DIR, else ~/.claude for Claude Code.
	TargetHome string
	// TargetBinary is the target harness's executable. Empty means the target's own default on PATH
	// (CodexBinaryDefault for Codex, ClaudeBinaryDefault for Claude Code).
	TargetBinary string

	// DryRun converts and reports without installing anything.
	DryRun bool
	// Selection names which of the read's own records to carry, as a specification of ordinal values and
	// regions ("0-5,9,12-"); the literal empty string with SelectionSupplied false means the whole read,
	// exactly as a conversion behaved before the option existed, while any other value -- whitespace
	// alone included, presence bit or not -- is a request that has to name something and is refused when
	// it cannot (CheckSelectionOptions). The ordinals are the values the read supplied
	// (Message.Ordinal), never a position, and a record the spec did not name is carried when a recorded
	// call or result it is paired with is (Select).
	Selection string
	// SelectionSupplied says the caller supplied the selection option at all, even when its value came
	// out empty. An unset shell variable expanded into `--select "$SPEC"` is a request whose value names
	// nothing, and it is refused (ErrBadSelection, the usage exit code) before anything is read or
	// installed instead of being read as the flag's absence: a run that installed the whole history is
	// the one thing the caller's command was written to prevent.
	SelectionSupplied bool
	// StrictSelection refuses a selection that would carry a recorded call without the recorded result
	// that answers it (or the reverse), by name and before anything is installed, instead of carrying
	// the counterpart.
	StrictSelection bool
	// StrictSelectionSupplied says the caller wrote --strict-selection at all, whatever its value: an
	// explicit `--strict-selection=false` is the flag's presence, and the presence is what the strict
	// arm's own refusal and the pair gate ask about. A false value tracks as a request that has to be
	// answered rather than silently turning the run into one the flag was never given.
	StrictSelectionSupplied bool
	// Verify is "none", "load" (default) or "resume".
	Verify string
	// ResumePrompt is the turn a "resume" verification runs. It is required for that level, because the
	// turn becomes part of the session.
	ResumePrompt string

	// Timeout bounds one verification command against the target harness: for Codex, one app-server
	// request for the provider probe and the load check, or one resume turn; for Claude Code, one loader
	// arm of the load check, or one resume turn. Zero means the per-level default (60 seconds per
	// app-server request, 90 seconds per Claude Code loader arm, 5 minutes for a resume turn).
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

// TargetReport describes the session that was written and how to resume it. FileName is the name of the
// file the session occupies inside its home; a route whose artifact is a row in the harness's own store
// rather than a file (OpenCode) names none, and the member is then absent rather than naming a file that
// exists nowhere.
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
	FileName         string        `json:"file_name,omitempty"`
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
	Selection     *SelectionReport   `json:"selection,omitempty"`
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

// selectionRequested answers whether a command line asked for a selection at all: a specification that
// names something, a value the caller supplied (even an empty one, which CheckSelectionOptions refuses
// before this is asked), or the strict arm -- supplied at all, whatever its value. It is the presence
// test the pair gate and the conversion use, so a run without the flag keeps its exact behaviour and a
// run with it is answered about the flag rather than silently ignored; `--strict-selection=false` is
// the flag, and only its absence is not.
func selectionRequested(selection string, supplied, strict, strictSupplied bool) bool {
	return supplied || strict || strictSupplied || strings.TrimSpace(selection) != ""
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
	// The selection's own refusals, in the order the caller's command line reads: the specification has
	// to be this build's grammar at all, and the pair has to be one whose writer consumes the seam.
	// Both are answered here, before any source is opened and before anything is installed; a refused
	// run costs no read of the source and no target identity, and the CLI maps the errors to the usage
	// exit code (IsSelectionRefusal).
	if err := CheckSelectionOptions(opts.Selection, opts.StrictSelection,
		opts.StrictSelectionSupplied, opts.SelectionSupplied); err != nil {
		return ContinueResult{}, err
	}
	if selectionRequested(opts.Selection, opts.SelectionSupplied, opts.StrictSelection,
		opts.StrictSelectionSupplied) {
		if err := CheckSelectionPair(opts.Ref, target); err != nil {
			return ContinueResult{}, err
		}
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

	// The source's own bytes are measured once, after the read and before anything is written: the digest
	// this pass takes is the one the post-installation comparison re-reads, and the boundary it answers is
	// what makes one conversion's snapshot distinguishable from another's -- which source file, which bytes
	// of it, whether the file is still the size the read's own snapshot recorded (a size match, which is
	// what a size can show), and how many bytes of an unterminated record the file had left when the
	// boundary was taken. The property the design asks for is that the transcript a conversation came from
	// is the same file afterwards, whatever the provider's own fingerprint happens to hash.
	boundary, err := takeSourceBoundary(reader, conversation, readFrom)
	if err != nil {
		return ContinueResult{}, err
	}
	if result.Source != nil {
		result.Source.Boundary = &boundary
	}
	sourceDigest := boundary.SHA256
	// A source captured while it was still being written is reported in the warnings as well as in the
	// boundary block: the conversion is real, and what is missing from it is a record the file had not
	// finished writing, which another conversion carries.
	result.Warnings = append(result.Warnings, boundaryWarnings(&boundary)...)

	// The selection, when the caller named one, is decided here: after the read and the boundary, because
	// it is a transformation of the *document* (the source file's bytes are measured whole and never
	// rewritten), and before the source-side leaf guard below, because a specification the read's own
	// ordinals refuse is a statement about the caller's command line -- the usage exit code, the core's
	// own sentence -- and must not be reported as something about the source file's leaf. Everything
	// downstream -- the writer, the structural read-back, the load check, the printed invocation -- is
	// about the session that is actually written, so the reduced document is what travels on. Nothing is
	// installed before this point, so a spec this build cannot honour costs no file and no target
	// identity. The leaf check below is asked about the *read*, never the window: which entry the
	// source's own loader walks from is a property of the source file and the read, and a window the
	// selection carries neither moves it nor narrows the read's own last record.
	conversationForWrite := conversation
	if selectionRequested(opts.Selection, opts.SelectionSupplied, opts.StrictSelection,
		opts.StrictSelectionSupplied) {
		selected, report, err := Select(conversation, opts.Selection, opts.StrictSelection)
		if err != nil {
			return ContinueResult{}, err
		}
		conversationForWrite = selected
		result.Selection = &report
	}

	// The record the source's own loader selects as the conversation's leaf, asked of the source file's own
	// entries: for the Pi-family readers the read's own last record is not that answer (pi_source_leaf.go),
	// and a conversion written from a record the file's leaf path does not reach would resume a branch the
	// source's conversation left. Where no source file stands behind the read the leaf is not established,
	// and the report says so rather than presenting the read's own last record as the source's leaf. The
	// same pass counts the records on that path the read does not carry whose omission the conversion
	// accounts for -- records the read never held, so no target writer can count them -- and those travel
	// into the conversion's losses below.
	leaf, err := piSourceLeafCheck(conversation, readFrom, boundary)
	if err != nil {
		return ContinueResult{}, err
	}
	if leaf.Warning != "" {
		result.Warnings = append(result.Warnings, leaf.Warning)
	}

	// What only the target's own harness can answer (Codex: the model provider its loader checks). It is
	// asked here, after the read, because the answer can depend on the session's own working directory.
	facts := route.Facts(ctx, targetFactsInput{
		Home:      home,
		DryRun:    opts.DryRun,
		Probe:     probe,
		SourceCwd: conversation.Source.Cwd,
		Timeout:   opts.Timeout,
		Stderr:    opts.Stderr,
	})
	if strings.TrimSpace(facts.CLIVersion) == "" {
		facts.CLIVersion = route.PinVersion
	}
	if strings.TrimSpace(facts.CLIVersionSource) == "" {
		facts.CLIVersionSource = "writer-pin"
	}

	session, err := route.Convert(conversationForWrite, targetConvertInput{
		CLIVersion:     facts.CLIVersion,
		Provider:       facts.ModelProvider,
		Model:          facts.Model,
		ModelVariant:   facts.ModelVariant,
		Agent:          facts.Agent,
		ProviderDetail: facts.ProviderDetail,
		Now:            opts.Now,
		MaxToolNames:   opts.MaxToolNames,
	})
	if err != nil {
		return ContinueResult{}, err
	}
	if result.Selection != nil {
		// D1's per-record table is filled from the writer's own walk, which is the only place that knows
		// what each record became: the item kinds written for it, or the category it was counted under.
		// The undischarged class is split against those fates in the same pass, because only the walk
		// says which of the source's undischarged calls sit on records the conversion did not publish,
		// and the two report surfaces (the selection's class and the conversion's
		// outstanding_tool_calls) reconcile through that split.
		fillSelectionRecords(result.Selection, session.RecordFates)
		fillSelectionUndischargedSplit(result.Selection)
		// D3's closure eligibility, answered where the writer's own walk has just answered it: a
		// counterpart the route did not publish refuses the whole selection by name, before anything is
		// installed and before any verification, rather than leaving the pair half written while the
		// necessity class declared the counterpart carried. Nothing is published before this point, so
		// the refusal costs no target file and no target identity -- the same shape as the refusals
		// raised before the read.
		if err := lostNecessityRefusal(result.Selection); err != nil {
			return ContinueResult{}, err
		}
		// The selection's warnings travel here rather than beside the selection itself: the undischarged
		// one names the split against the writer's fates, which only the walk above can answer.
		result.Warnings = append(result.Warnings, selectionWarnings(result.Selection)...)
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
	// The source-side check's counted omissions stand beside the target writer's own categories: the
	// records are ones the read never held, so the writer has no record of them to count
	// (piSystemRecordLoss). The category is absent when the source file held none.
	result.Conversion.Losses = append(result.Conversion.Losses, leaf.Losses...)
	// The invocation is built here, where the session identity exists, and rendered twice: as the
	// argument vector and environment a caller runs, and as the shell line a person pastes.
	resumeName := strings.TrimSpace(opts.TargetBinary)
	if resumeName == "" {
		resumeName = route.BinaryDefault
	}
	invocation := route.Invocation(session, targetInvocationInput{Binary: resumeName, Probe: probe, Home: home})
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
		if _, _, err := route.Install(ctx, session, targetInstallInput{
			Home:    scratch,
			Binary:  probe.Binary,
			Timeout: opts.Timeout,
			Stderr:  opts.Stderr,
		}); err != nil {
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
		// harness's own answer was not asked; the route says what that means for its format.
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
	path, mechanism, err := route.Install(ctx, session, targetInstallInput{
		Home:    home,
		Binary:  probe.Binary,
		Timeout: opts.Timeout,
		Stderr:  opts.Stderr,
	})
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
		// "installed?" from the path above, and this error is what the run failed on. An installer that
		// could not establish its own outcome says so through ErrInstallationUnresolved: the report then
		// answers `unresolved` rather than `not-installed`, because the run observed neither a
		// publication nor its absence. A path alongside that error is an answer of its own -- the path is
		// the session the installer published -- so the state is unresolved only where no path came back.
		if errors.Is(err, ErrInstallationUnresolved) && path == "" {
			result.Installation.State = InstallationStateUnresolved
		}
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
		Home:      home,
		Binary:    probe.Binary,
		SessionID: session.ID,
		Cwd:       session.Cwd,
		Path:      path,
		Directory: session.Directory,
		FileName:  session.FileName,
		Data:      session.Data,
		Timeout:   opts.Timeout,
		Stderr:    opts.Stderr,
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
		// "was not shown to hold" rather than "does not hold": the same state covers a comparison that
		// ran and found the file missing evidence, and a reader that could not read the file back at
		// all, where nothing was shown about what it holds either way.
		result.Warnings = append(result.Warnings, "the installed session was not shown to hold every piece of "+
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

	structureMethodPi       = "the inherited AgentsView Pi reader over the installed session"
	structureMethodPiDryRun = "the inherited AgentsView Pi reader over a throw-away copy (dry run)"
	// The Pi load check drives the harness's own RPC loader against a throw-away copy, with the
	// variables that could reach a model removed, and asks the same question about an unknown id beside
	// it: see pi_verify.go.
	loadMethodPi = "the target harness's own loader (pi --mode rpc, get_state/get_messages, with no " +
		"model variables in its environment, plus an unknown-id control)"

	resumeMethodPi = "one turn in the installed session through the target harness's non-interactive " +
		"resume (pi -p --session)"
	// The Claude load check drives the harness's own loader with --bare and with the environment
	// variables that could reach a model removed, so the loader resolves the installed session without a
	// model turn, and runs an unknown-id control arm beside it: see claude_verify.go.
	loadMethodClaude = "the target harness's own loader (claude --bare --resume, with no model " +
		"credentials in its environment, plus an unknown-id control)"

	resumeMethodCodex = "one turn in the installed session through the target harness's non-interactive resume"
	// The Claude resume check runs the reported invocation as it is printed, which is also the exec form
	// the report carries.
	resumeMethodClaude = "one turn in the installed session through the target harness's non-interactive " +
		"resume (claude -p --resume)"

	// structureReaderCodex and structureReaderClaude name the inherited reader each route's structural
	// check reads a file back through: the verification's own detail and the report's warning for a
	// written message the reader does not read its own text for both name that reader, so the name lives
	// once per route and no target's check can name another target's reader.
	structureReaderCodex  = "Codex"
	structureReaderClaude = "Claude Code"
	structureReaderPi     = "Pi"
	structureReaderOMP    = "Oh My Pi"

	// The OMP route's own sentences. Its format is the Pi family's, but its harness is a different
	// program with its own loader flags and its own data directory, so a reader sees which harness
	// answered rather than a family label.
	structureMethodOMP       = "the inherited AgentsView OMP reader over the installed session"
	structureMethodOMPDryRun = "the inherited AgentsView OMP reader over a throw-away copy (dry run)"
	// The OMP load check drives the harness's own RPC loader against a throw-away copy in a scratch data
	// directory that defines a credential-free provider, and asks the same question about an unknown id
	// beside it: see omp_verify.go.
	loadMethodOMP = "the target harness's own loader (omp --mode rpc, get_state/get_messages, in a " +
		"scratch data directory whose provider needs no credential, plus an unknown-id control)"

	resumeMethodOMP = "one turn in the installed session through the target harness's non-interactive " +
		"resume (omp -p --resume)"

	// The OpenCode route. Its target artifact is a session inside the harness's own SQLite store, so the
	// structure check reads the installed session back through the inherited OpenCode reader by its own
	// reference form (`<database path>#<session id>`), the load check asks the harness's own reader
	// (`session export`) -- a command measured to leave the session's own records unchanged, unlike a
	// turn -- and the resume check runs the reported invocation's one real turn.
	structureMethodOpenCode       = "the inherited AgentsView OpenCode reader over the installed session"
	structureMethodOpenCodeDryRun = "the inherited AgentsView OpenCode reader over a throw-away copy (dry run)"
	loadMethodOpenCode            = "the target harness's own reader (opencode session export, with a " +
		"scratch configuration and no model turn, plus an unknown-id control)"
	resumeMethodOpenCode = "one turn in the installed session through the target harness's " +
		"non-interactive resume (opencode run --session)"
	structureReaderOpenCode = "OpenCode"
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
// Inspect does: a caller-supplied reader this build built lends its own configuration over the caller's
// zero value, while an injected reader (SourceReads) has none to lend and the zero Config is completed
// from disk exactly as it would be with no reader at all.
func resolveContinueConfig(opts ContinueOptions) (config.Config, SourceReads, error) {
	cfg := opts.Config
	reader := opts.Reader
	if cfg.AgentDirs == nil {
		if concrete, ok := reader.(*SourceReader); ok {
			cfg = concrete.cfg
		} else {
			loaded, err := config.LoadReadOnly()
			if err != nil {
				return config.Config{}, nil, fmt.Errorf("reading the AgentsView configuration: %w", err)
			}
			cfg = loaded
		}
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
	ctx context.Context, reader SourceReads, cfg config.Config, ref Ref, archivePath string,
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

// takeSourceBoundary measures where one conversion's snapshot of the source stops. It answers the source
// session's own file, its size and the digest of those bytes, how those bytes relate to the read's own
// snapshot of the file, and -- when the provider's own declarations say the source is a transcript of
// appended line records -- where the file's last line-feed-terminated record ends and how many bytes of an
// unterminated record follow it.
//
// It is called once per conversion, after the session was read and before anything is written, and it is
// the *same* pass that answers the digest the post-installation comparison re-reads: a conversion therefore
// pays for one read of the source file here, exactly as it did before the boundary existed.
//
// A read whose identity is not one file's name is answered as a boundary that was not taken, with the
// reason, rather than as an error the caller would have to turn back into one: an archive read has no
// source file open, and a container member's bytes are the container's, not this conversation's
// (sourceFileOf). Only a file that cannot be read stops the run, the way the pre-conversion digest already
// did.
func takeSourceBoundary(reader SourceReads, conversation Conversation, readFrom string) (SourceBoundary, error) {
	if readFrom != "source" {
		return SourceBoundary{
			State:  SourceBoundaryNotTaken,
			Detail: "the conversion read an archive row, so no source file was open at all",
		}, nil
	}
	file := sourceFileOf(conversation)
	if file.reason != "" {
		return SourceBoundary{State: SourceBoundaryNotTaken, Detail: file.reason}, nil
	}
	if strings.TrimSpace(file.path) == "" {
		// A read that reported no path is the shape the source comparison stops on (sourceCheckAfter), and
		// the boundary says the same thing here rather than measuring a file nobody named.
		return SourceBoundary{
			State:  SourceBoundaryNotTaken,
			Detail: "the source read reported no path, so no file holds this conversation's bytes",
		}, nil
	}
	declared := sourceRecordsAreLines(reader, conversation.Source.Provider)
	boundary, err := sourceBoundaryOfFile(file.path, declared)
	if err != nil {
		return SourceBoundary{}, fmt.Errorf("reading the source session's own bytes: %w", err)
	}
	if declared {
		// The read's own record of the file it opened is compared with the file this boundary measured.
		// It is asked only where the provider's declarations make that number this file's own
		// (sourceRecordsAreLines): a provider whose source is a layout records a composite size, and
		// comparing it with one file's would state a move that did not happen.
		fillReadCoverage(&boundary, conversation.Source.FileSize)
	}
	return boundary, nil
}

// fillReadCoverage states what the file's bytes at the boundary have to do with the size the read's own
// snapshot recorded for it. A source that grew while it was being read is the case this answers: the bytes
// below are the file's, but which of them the conversion covered is then not established, and the report
// says so instead of attributing an older conversation to newer bytes with nothing to notice it by.
//
// What the read's own record holds is a size (ParsedSession.File.Size) and no digest of the bytes it
// opened, so equal sizes establish a size match and not byte identity: a file replaced by different bytes
// of the same length after the read leaves both sizes equal and is invisible here and everywhere else in
// the report (the digest below is of the file as it stands, and the source comparison's two digests are
// both taken after the read). The detail therefore names the comparison and its limit rather than claiming
// the conversion read these bytes, which is the claim a size cannot support.
func fillReadCoverage(boundary *SourceBoundary, readBytes int64) {
	if readBytes <= 0 {
		// The read recorded no size for this file: there is nothing to compare, and a comparison against
		// zero would state a move this build did not measure.
		return
	}
	boundary.ReadBytes = readBytes
	if readBytes == boundary.Bytes {
		boundary.ReadCoverage = SourceBoundaryCoverageSizeMatch
		boundary.ReadCoverageDetail = fmt.Sprintf(
			"the read's own snapshot of this file is %d byte(s) and the file holds %d: the file is the size "+
				"the read recorded, which is a size match and not a byte-identity check -- the read's own "+
				"record holds no digest of the bytes it opened, so a same-length replacement of the file "+
				"after it is not ruled out", readBytes, boundary.Bytes)
		return
	}
	boundary.ReadCoverage = SourceBoundaryCoverageSizeMismatch
	boundary.ReadCoverageDetail = fmt.Sprintf(
		"the read's own snapshot of this file is %d byte(s) while the file holds %d: the file moved while "+
			"it was being read, so which of these bytes the conversion covered is not established",
		readBytes, boundary.Bytes)
}

// sourceRecordsAreLines answers whether one provider's own declarations say its source is a transcript of
// line records: the capability that says the source grows by appended lines (which its read can only
// re-answer by parsing the whole file again). It is the declaration that makes the offset just after the
// file's last line feed a *record* boundary rather than an arbitrary byte -- the first half of the gate;
// the second half is the file's own bytes (sourceBoundaryOfFile), because a declaration about a format is
// not a statement about this representation of it.
//
// The two providers whose declarations name such a source are also the two whose parse records this file's
// own size (Claude Code's transcript, Codex's rollout), which is what makes the read's own snapshot a size
// this build may compare with one file's (fillReadCoverage).
//
// The other candidate declaration this gate once accepted was the capability that says a read counts the
// records it could not parse (ContentCapabilities.MalformedLineCount). It is not the same claim, and the
// carried tree measures the difference: QwenPaw counts malformed *entries* inside one whole-file JSON
// document (internal/parser/qwenpaw.go's entry loop, declared by qwenpaw_provider.go), CodeBuddy does the
// same for a manifest object, and DeepSeek Harness's source may be a compressed container whose raw bytes
// hold no line feed at all -- so a complete file of theirs was reported as a line-recorded source whose
// entire content was one unfinished record (Q-0004-07). A gate decision is superseded by evidence, not by
// a preference, and the record is appended rather than rewritten.
//
// Anything else answers false, including a provider this reader cannot resolve (a disabled provider, a
// stored agent label with no factory of its own): the boundary then states the file's bytes and claims no
// record boundary, because that claim is the provider's to make and never this build's guess about a
// format. A provider whose format is line-recorded but that declares nothing therefore gets no byte-level
// record boundary, which is a narrowing this build states rather than a claim it invents.
func sourceRecordsAreLines(reader SourceReads, provider string) bool {
	concrete, ok := reader.(*SourceReader)
	if !ok {
		// An injected reader cannot answer a provider's own declarations: the boundary still measures the
		// file's bytes and digest, and only the record offsets are withheld -- a claim this build would
		// otherwise make from an answer nobody gave.
		return false
	}
	_, factory, err := concrete.resolveFactory(provider)
	if err != nil {
		return false
	}
	return factory.Capabilities().Source.IncrementalAppend == parser.CapabilitySupported
}

// sourceBoundaryOfFile reads one file once, answering its size, the digest of exactly those bytes, and --
// when the caller asked for a record boundary -- whether the file's own bytes can be a transcript of
// appended line records at all and where its last line-feed-terminated record ends.
//
// The record offsets are byte facts about the file, not a parse of it: terminated_records_end is the offset
// just after the file's last line feed, and the bytes after it are the file's unterminated record -- the
// one record a file can leave unterminated, because a record that follows another record's line feed is
// itself terminated or it is the last. A file with no line feed at all has no terminated record in it (0,
// the whole file unterminated), and an empty file has neither.
//
// The claim is withheld when the file's own bytes cannot be that transcript. A provider may declare that
// its source is appended lines while this *representation* of the source is a container of them -- Codex
// reads its own compressed rollouts (internal/parser's codex_rollout_open.go: the file is decoded and no
// reader ever resumes from an offset into those compressed bytes) -- and an offset found by scanning such a
// file for line feeds is not a record boundary, nor are the bytes after it an unterminated record. The
// file's digest and size are still exactly what they say they are, which is what a caller compares two
// conversions of one source by.
func sourceBoundaryOfFile(path string, recordsAreLines bool) (SourceBoundary, error) {
	file, err := os.Open(path)
	if err != nil {
		return SourceBoundary{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	// One bounded buffer for a source that can be hundreds of megabytes: the caller's cost is the file's
	// size, not a copy of it.
	buffer := make([]byte, 32<<10)
	var size, recordsEnd int64
	text := textStream{valid: true}
	for {
		read, err := file.Read(buffer)
		if read > 0 {
			chunk := buffer[:read]
			_, _ = hasher.Write(chunk)
			if recordsAreLines {
				if last := bytes.LastIndexByte(chunk, '\n'); last >= 0 {
					recordsEnd = size + int64(last) + 1
				}
				text.write(chunk)
			}
			size += int64(read)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return SourceBoundary{}, err
		}
	}
	if recordsAreLines {
		text.done()
	}
	boundary := SourceBoundary{
		State:  SourceBoundaryTaken,
		Detail: "the conversation's own source file, read after the session and before anything was written",
		Path:   path,
		Bytes:  size,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}
	switch {
	case !recordsAreLines:
		boundary.RecordOffsets = SourceBoundaryRecordsNotClaimed
		boundary.RecordOffsetsDetail = recordOffsetsNotDeclared
	case !text.valid:
		boundary.RecordOffsets = SourceBoundaryRecordsNotClaimed
		boundary.RecordOffsetsDetail = recordOffsetsNotText
	default:
		boundary.RecordOffsets = SourceBoundaryRecordsClaimed
		boundary.TerminatedRecordsEndBytes = recordsEnd
		boundary.UnterminatedTailBytes = size - recordsEnd
	}
	return boundary, nil
}

// textStream answers whether a file's bytes are text (valid UTF-8), one chunk at a time, carrying the bytes
// of a rune that a chunk boundary split to the chunk that completes it. It is the file's half of the
// record-offset gate: a transcript of lines is text, and a container of one is not -- a zstd frame's own
// header is not valid UTF-8 -- so a file this stream rejects cannot be the records, whatever its provider
// declares about the format.
//
// At most three bytes of a chunk can be a split rune (the longest rune is four bytes), so one small array
// answers the question without copying the chunk.
type textStream struct {
	valid bool
	tail  [utf8.UTFMax - 1]byte
	tailN int
}

// write adds one chunk to the verdict.
func (s *textStream) write(chunk []byte) {
	if !s.valid || len(chunk) == 0 {
		return
	}
	if s.tailN > 0 {
		// The carried bytes are, by construction, the valid beginning of one rune. Complete it with as many
		// of this chunk's bytes as it needs, and keep carrying it when the chunk is too short to finish it:
		// a chunk this small is not evidence that the rune is invalid, and the decision belongs to the bytes
		// that complete it (or to done, if the file ends first).
		need := runeLen(s.tail[0]) - s.tailN
		if need > len(chunk) {
			// need > len(chunk) implies tailN+len(chunk) < runeLen(s.tail[0]) <= utf8.UTFMax, so the whole
			// chunk fits in the carry beside what is already there.
			s.tailN += copy(s.tail[s.tailN:], chunk)
			return
		}
		var head [utf8.UTFMax]byte
		n := copy(head[:], s.tail[:s.tailN])
		n += copy(head[n:], chunk[:need])
		if _, size := utf8.DecodeRune(head[:n]); size <= 1 {
			// Two halves that no rune joins: the carried bytes were a valid prefix only of a sequence this
			// chunk rules out.
			s.valid = false
			return
		}
		s.tailN = 0
		chunk = chunk[need:]
	}
	head, tail := splitTrailingRune(chunk)
	if !utf8.Valid(head) {
		s.valid = false
		return
	}
	s.tailN = copy(s.tail[:], tail)
}

// done is what the caller says when the file has no more bytes: a rune still carried at that point was
// never completed, and a file whose bytes end inside a rune is not text -- no later byte can finish a
// sequence the file does not hold.
func (s *textStream) done() {
	if s.tailN > 0 {
		s.valid = false
		s.tailN = 0
	}
}

// splitTrailingRune splits one chunk at the start of a rune its own end cuts short: head is what the caller
// validates on its own, and tail (at most three bytes) is the beginning of a rune whose remaining bytes are
// in the next chunk. A chunk that ends at a rune boundary has an empty tail, and so does one ending in bytes
// no rune begins with -- those are the caller's own validation to reject.
func splitTrailingRune(chunk []byte) (head, tail []byte) {
	for back := 1; back <= utf8.UTFMax-1 && back <= len(chunk); back++ {
		lead := chunk[len(chunk)-back]
		if lead < utf8.RuneSelf {
			return chunk, nil
		}
		if lead&0xC0 == 0xC0 {
			if runeLen(lead) > back {
				return chunk[:len(chunk)-back], chunk[len(chunk)-back:]
			}
			return chunk, nil
		}
		// A continuation byte: the rune it belongs to begins further back.
	}
	return chunk, nil
}

// runeLen answers how many bytes the multi-byte rune a leading byte begins has. A byte that begins no rune
// utf8 accepts is sized as the longest one, so it is carried rather than validated alone: utf8's own decode
// is what rejects it, on the chunk that would have completed it.
func runeLen(lead byte) int {
	switch {
	case lead >= 0xF0:
		return 4
	case lead >= 0xE0:
		return 3
	default:
		return 2
	}
}

// boundaryWarnings names the two boundary answers a caller reading only the report's own channel must not
// miss: the file's unterminated record, and a file that is not the size the read's own snapshot recorded.
//
// The first is the byte fact behind "the source may still be writing this": a record's line feed is what a
// writer appends last, so a file ending without one may be mid-append. It is *not* a claim that the record
// is left out of the conversion -- whether an unterminated record is part of the read is the reader's own
// answer (an unterminated record that is a complete record is read; one the reader cannot read is dropped),
// and a warning that asserted the omission was false for the first case. What the caller gets is the bytes,
// the read's own fidelity beside them, and the goal's remedy.
//
// The second is the read's own snapshot disagreeing with the file it measured: the report would otherwise
// attribute this conversation to bytes the read may never have covered, and nothing would say so.
//
// A boundary that was not taken is not warned about: nothing measured a tail or a size, and the block's own
// state and detail already say why there is none.
func boundaryWarnings(boundary *SourceBoundary) []string {
	if boundary == nil || boundary.State != SourceBoundaryTaken {
		return nil
	}
	var warnings []string
	if boundary.ReadCoverage == SourceBoundaryCoverageSizeMismatch {
		warnings = append(warnings, fmt.Sprintf(
			"the source file is not the size the read's own snapshot recorded for it (%d byte(s) read, %d "+
				"byte(s) now): the file moved while it was being read, so which of its bytes this conversion "+
				"covers is not established -- converting again reads the file it holds now",
			boundary.ReadBytes, boundary.Bytes))
	}
	if boundary.UnterminatedTailBytes > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"the source file's last %d byte(s) hold no line feed: its bytes end at byte %d while its last "+
				"line-feed-terminated record ends at byte %d, so the source may still be writing that record. "+
				"Whether this conversion carries it is the read's own answer -- the read's fidelity and the "+
				"conversion's own counts say what it read -- and converting again once the source has been "+
				"written reads the file again",
			boundary.UnterminatedTailBytes, boundary.Bytes, boundary.TerminatedRecordsEndBytes))
	}
	return warnings
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
//
// It is the boundary pass with the record offsets unused, so one implementation answers both the digest a
// comparison needs and the boundary a conversion reports -- a second loop over the same bytes would be a
// second read of a large file for the same answer.
func fileDigest(path string) (string, error) {
	boundary, err := sourceBoundaryOfFile(path, false)
	if err != nil {
		return "", err
	}
	return boundary.SHA256, nil
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

// promptTerminator is the option terminator a target's non-interactive form puts between its own flags
// and the caller's prompt. Without it a prompt whose own first word is an option is read as one -- a
// turn asking about `--help` would print help instead of reaching the session -- so the printed exec
// form carries it, and the resume verification and the load check run behind it.
const promptTerminator = "--"

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
