package ccc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The OpenCode route's installation is the harness's own importer, and its structure check reads the
// installed session back through the inherited AgentsView OpenCode reader. What makes both possible is
// measured and recorded in docs/evidence/0007-opencode-target-continuation.md:
//
//   - the data directory is `<XDG data home>/opencode` (its own `debug paths` says so, and there is no
//     variable that names the directory itself), so the directory this route reports and the environment
//     its commands set are the same directory under the harness's own rule;
//   - `opencode session import <file> --directory DIR` preserves the document's `info.id`, rewrites the
//     project and directory from `DIR` (which must exist: a missing one is refused with
//     `LocationNotFoundError`), answers `Imported session: <id>` on stdout, and answers
//     `Session already exists` on stderr with exit 0 when the id is already there -- leaving the
//     existing session untouched. A refused import leaves the store unchanged.
//
// The import document is written to a temporary file outside the home and removed afterwards, so the
// home receives nothing this build made: what is in it was written by the harness's own importer.

// openCodeDataRootEnv is the environment variable that selects the harness's data directory: the
// directory it reads is always `opencode` below the XDG data home, so the value is that directory's
// *parent*. It is what every command this route runs and prints sets.
const openCodeDataRootEnv = "XDG_DATA_HOME"

// openCodeDBEnv is the variable that overrides the database file alone. Every command this route runs
// has it removed from the caller's environment -- an inherited value would point the harness at another
// store -- and the printed resume line and the resume check name this home's own store in it instead,
// because a printed shell line can name a value but cannot express a removal.
const openCodeDBEnv = "OPENCODE_DB"

// openCodeImportMechanism is the mechanism the installation report names.
const openCodeImportMechanism = "the target harness's own session importer (opencode session import)"

// openCodeImportFileMode is the mode the staged document is written with: readable and writable by its
// owner, by nobody else. The file holds the conversation and lives outside the home, so it is restricted
// like every other artifact this build writes.
const openCodeImportFileMode os.FileMode = 0o600

// ResolveOpenCodeHome answers the OpenCode data directory a conversion installs into: the caller's
// explicit directory, else `$XDG_DATA_HOME/opencode`, else `~/.local/share/opencode`. The boolean
// reports whether the answer came from the caller, which is what decides whether a missing directory may
// be created.
//
// The environment variable names the data *root*, not the data directory (the harness appends its own
// `opencode`), so the value is used as a root rather than as the home. A caller's own spelling is kept
// whole: it is resolved exactly as it was written, because a directory name may begin or end with a
// space.
func ResolveOpenCodeHome(explicit string) (string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		home, err := filepath.Abs(explicit)
		if err != nil {
			return "", true, fmt.Errorf("resolving %s: %w", explicit, err)
		}
		return home, true, nil
	}
	if fromEnv := os.Getenv(openCodeDataRootEnv); strings.TrimSpace(fromEnv) != "" {
		root, err := filepath.Abs(fromEnv)
		if err != nil {
			return "", false, fmt.Errorf("resolving $%s: %w", openCodeDataRootEnv, err)
		}
		return filepath.Join(root, openCodeDataDirName), false, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolving the home directory: %w", err)
	}
	return filepath.Join(dir, ".local", "share", openCodeDataDirName), false, nil
}

// openCodeDataRoot answers the value the harness's data-directory variable takes for a home this route
// resolved: the home itself when it is a directory the harness's own rule names (`…/opencode`), and the
// home itself as the XDG data root otherwise, so that `$XDG_DATA_HOME/opencode` is that directory. One
// rule, applied to the name the caller gave, so the directory the report states, the environment the
// printed commands set and the directory the harness reads are the same one.
func openCodeDataRoot(home string) string {
	if filepath.Base(home) == openCodeDataDirName {
		return filepath.Dir(home)
	}
	return home
}

// openCodeDataDir answers the harness's own data directory for a resolved home: the home itself when its
// base name is `opencode` (the rule's own answer for any XDG data home), and `<home>/opencode` otherwise.
func openCodeDataDir(home string) string {
	return filepath.Join(openCodeDataRoot(home), openCodeDataDirName)
}

// openCodeStorePath answers the database file an installed session lives in.
func openCodeStorePath(home string) string {
	return filepath.Join(openCodeDataDir(home), openCodeDBName)
}

// openCodeStoreRef answers the inherited reader's own reference to one session in a store: the database
// file's absolute path and the session's id, joined by `#` (internal/parser's OpenCodeSQLiteVirtualPath,
// the spelling its own discovery produces). The database's base name must be `opencode.db` and the path
// must lie under a configured root, which is exactly where the importer put it.
func openCodeStoreRef(dataDir, sessionID string) string {
	return filepath.Join(dataDir, openCodeDBName) + "#" + sessionID
}

// openCodeEnvironment builds the environment every OpenCode command this route runs is given: the
// caller's own, with the data-root variable set to this home's root, the database override removed (it
// would point the harness at another store), and -- for the checks -- a scratch configuration and state
// directory and the credential names they must not read. Nothing else is copied or rewritten. A caller
// that needs this home's own store named in the override (the resume check, so it runs the environment
// the report prints) passes it in set: the removal filters the caller's value, and the set member is
// this run's own.
func openCodeEnvironment(home string, set map[string]string, denied ...string) []string {
	blocked := append([]string{openCodeDBEnv}, denied...)
	return childEnvironment(openCodeDataRoot(home), openCodeDataRootEnv, blocked, set)
}

// openCodeImporterExecutable resolves the harness executable the importer runs: the probed path when the
// probe found one, else the caller's spelling through PATH. A target with no harness cannot install
// anything, which the caller reports as the boundary it is.
func openCodeImporterExecutable(binary string) (string, error) {
	name := strings.TrimSpace(binary)
	if name == "" {
		name = OpenCodeBinaryDefault
	}
	path, err := resolveOpenCodeExecutable(name)
	if err != nil {
		return "", fmt.Errorf("%s is not on PATH, and this target's installation is the harness's own "+
			"importer: %w", name, err)
	}
	return path, nil
}

// openCodeTimeout answers the deadline one harness invocation gets: the caller's own when it named one,
// else the route's own deadline for a question asked of the harness. An import is a local write the
// harness answers in about a second; a caller that named no deadline is not a caller that asked for an
// instant failure.
func openCodeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return openCodeProbeTimeout
	}
	return timeout
}

// InstallOpenCodeSession hands a converted document to the harness's own importer and reports where the
// session it materialised lives: `<data directory>/opencode.db`, with the session's own id inside it.
//
// Nothing is written by this build into the home: the document is staged as a temporary file outside it,
// the importer consumes it, and the file is removed. Exclusivity is asked of the harness, twice: the id
// this conversion minted must not already resolve (`session export` answers `Session not found` for it),
// and the harness's own answer to the import must be `Imported session: <id>` with this document's id --
// an `already exists` answer is a failure rather than a success, because this run did not install it and
// another session's bytes are not this conversion's to claim.
//
// A process failure is reconciled rather than read as an absence: the harness's own acknowledgment line
// in the captured output is its answer that the store holds this session, and -- where the process failed
// without acknowledging anything -- the harness's own reader is asked whether this conversion's id
// resolves now. The path is then returned beside the error (the convention every installer here follows,
// continue.go), so the report answers "installed?" from what was observed; where neither the
// acknowledgment nor the reader answers, the returned error carries ErrInstallationUnresolved and the
// report says `unresolved`, never `not-installed`.
func InstallOpenCodeSession(
	ctx context.Context, session OpenCodeSession, in targetInstallInput,
) (string, string, error) {
	path, err := openCodeImporterExecutable(in.Binary)
	if err != nil {
		return "", "", err
	}
	home := in.Home
	environment := openCodeEnvironment(home, nil)
	if answer, answered := openCodeSessionExists(ctx, path, session.SessionID, environment, in); answered {
		return "", "", fmt.Errorf("%w: %s is already a session of the harness's own store at %s: %s",
			ErrTargetExists, session.SessionID, openCodeStorePath(home), answer)
	}
	document, err := os.CreateTemp("", "ccc-opencode-import-*.json")
	if err != nil {
		return "", "", fmt.Errorf("staging the import document: %w", err)
	}
	staged := document.Name()
	defer removeFile(staged)
	if err := document.Chmod(openCodeImportFileMode); err != nil {
		document.Close()
		return "", "", fmt.Errorf("restricting %s: %w", staged, err)
	}
	if _, err := document.Write(session.Data); err != nil {
		document.Close()
		return "", "", fmt.Errorf("writing %s: %w", staged, err)
	}
	if err := document.Sync(); err != nil {
		document.Close()
		return "", "", fmt.Errorf("flushing %s: %w", staged, err)
	}
	if err := document.Close(); err != nil {
		return "", "", fmt.Errorf("closing %s: %w", staged, err)
	}
	arguments := []string{"session", "import", staged, "--directory", session.Cwd, "--standalone"}
	output, diagnostics, err := runOpenCodeImporter(ctx, path, "", arguments, environment,
		openCodeTimeout(in.Timeout), in.Stderr)
	if err != nil {
		// A failed process is not by itself an answer about the store. Measured: the harness's own
		// importer commits the session and then exits, so a deadline or a cancellation between the two
		// leaves the session published while this run sees only a process error -- and reporting that as
		// "not installed" would tell the caller nothing was published about a store that holds the
		// session, whose next conversion would publish a second, undisclosed snapshot of one
		// conversation. Which of the two happened is asked, in the order the evidence is available.
		answered := output + diagnostics
		if _, ok := openCodeImportAnswer(answered, session.SessionID); ok {
			// The harness's own acknowledgment is already in the captured output: it answered
			// `Imported session: <id>` for this document, so the store holds it whatever happened to the
			// process afterwards. The path is returned beside the error, which is what lets the report
			// keep the identity the published session is resumed by (continue.go) instead of calling the
			// installation absent.
			return openCodeStorePath(home), openCodeImportMechanism, fmt.Errorf(
				"the harness's own importer answered `Imported session: %s` and then failed: %w: %s",
				session.SessionID, err, openCodeAnswerSentence(answered))
		}
		switch outcome, answer := openCodeReconcileImport(ctx, path, session.SessionID, environment,
			openCodeTimeout(in.Timeout)); outcome {
		case openCodeReconciledInstalled:
			// The harness's own reader now resolves the id this conversion minted, which it did not
			// before the import was staged: the store holds the session this run handed over, so the
			// installation is reported with the failure beside it.
			return openCodeStorePath(home), openCodeImportMechanism, fmt.Errorf(
				"the harness's own importer failed and the store now holds the session it was handed, so "+
					"the import was published: %w: %s (%s)", err, openCodeAnswerSentence(answered), answer)
		case openCodeReconciledAbsent:
			// The harness's own reader answered that this id does not resolve, and the same reader
			// answered the same way before the import was staged: nothing was published. The harness's
			// own words travel beside the process error here, as on every other refusal: a refusal is
			// what a caller can act on (measured: a missing recorded directory answers
			// `LocationNotFoundError: Location not found: <dir>` and exits 1), and a core API caller
			// that left ContinueOptions.Stderr unset has nowhere else to read them. The excerpt's bound
			// is the exit-zero path's own sentence rule.
			return "", "", fmt.Errorf("the harness's own importer could not be run: %w: %s",
				err, openCodeAnswerSentence(answered))
		default:
			// Neither the acknowledgment nor the reader answered: the run has no basis for either
			// answer, so it hands back the third one (continue.go's InstallationStateUnresolved)
			// instead of asserting an absence nobody observed.
			return "", "", fmt.Errorf(
				"%w: the harness's own importer failed (%s) and the store could not answer whether it "+
					"holds this session (%s): %w", ErrInstallationUnresolved,
				openCodeAnswerSentence(answered), answer, err)
		}
	}
	if _, ok := openCodeImportAnswer(output+diagnostics, session.SessionID); !ok {
		return "", "", fmt.Errorf("the harness's own importer did not accept the document: %s",
			openCodeAnswerSentence(output+diagnostics))
	}
	return openCodeStorePath(home), openCodeImportMechanism, nil
}

// openCodeSessionExists asks the harness whether the id this conversion minted already resolves in the
// home, before anything is staged. An answered export is that answer; a refusal of any kind (the
// command's own `Session not found`, a store the harness cannot open, a deadline) is not -- the importer
// gives its own answer next, and a failure to ask must not be read as "the id is free".
//
// The refusal is the expected answer, so its own diagnostic line (`Session not found: <id>`) is discarded
// rather than forwarded: it is the harness's words about a session that is supposed not to exist yet, and
// the returned text is what a refusal this run reports is worded from.
func openCodeSessionExists(
	ctx context.Context, binary, sessionID string, environment []string, in targetInstallInput,
) (string, bool) {
	arguments := []string{"session", "export", sessionID, "--standalone"}
	output, diagnostics, err := runOpenCodeImporter(ctx, binary, "", arguments, environment,
		openCodeTimeout(in.Timeout), io.Discard)
	if err != nil {
		return "", false
	}
	return summariseText(output+diagnostics, 200), true
}

// openCodeReconciliation is what the harness's own reader answered about the session id a failed import
// minted: that the store holds it, that it does not, or nothing that answers the question.
type openCodeReconciliation int

const (
	// openCodeReconciledInstalled is an answered export: the id resolves, and -- the id having been shown
	// not to resolve before the import was staged, and having been minted by this run -- the session the
	// reader answered about is the one this import handed over.
	openCodeReconciledInstalled openCodeReconciliation = iota
	// openCodeReconciledAbsent is the harness's own refusal naming this id: the store does not hold it.
	openCodeReconciledAbsent
	// openCodeReconciledUnanswered is neither: the question itself could not be answered (a store the
	// harness could not open, a deadline), so the store's content is unknown rather than absent.
	openCodeReconciledUnanswered
)

// openCodeReconcileImport asks the harness's own reader whether the session id a failed import minted
// resolves in the store, and answers which of the three answers it gave. It is a question asked of the
// harness, so it is bounded like every other one, and it is asked with a context that outlives the
// caller's own deadline: the deadline (or cancellation) that stopped the import is exactly what would
// make this question unable to answer, and an unanswered question is what makes the run report an
// installation it could not establish rather than one it observed to be absent. What the question can
// never do is publish anything: `session export` is measured to change none of a session's own records.
func openCodeReconcileImport(
	ctx context.Context, binary, sessionID string, environment []string, timeout time.Duration,
) (openCodeReconciliation, string) {
	arguments := []string{"session", "export", sessionID, "--standalone"}
	output, diagnostics, err := runOpenCodeImporter(context.WithoutCancel(ctx), binary, "", arguments,
		environment, timeout, io.Discard)
	answer := strings.TrimSpace(output + "\n" + diagnostics)
	if err == nil {
		return openCodeReconciledInstalled, summariseText(answer, 200)
	}
	if openCodeRefusalNamesSession(answer, sessionID) {
		return openCodeReconciledAbsent, summariseText(answer, 200)
	}
	return openCodeReconciledUnanswered, summariseText(firstNonEmptyText(answer, err.Error()), 200)
}

// openCodeRefusalNamesSession answers whether the harness's own words are its refusal to resolve one
// session: the `Session not found: <id>` line it answers on stderr with exit 1 for an id its store does
// not hold (measured on 2.2.5, and the same answer the load check's unknown-id control arm requires).
// The id is compared with the one asked about, so a refusal about another session -- or a failure whose
// diagnostic line merely mentions one -- is not read as this store's answer about this id.
func openCodeRefusalNamesSession(answer, sessionID string) bool {
	for _, line := range strings.Split(answer, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Session not found:")
		if ok && strings.TrimSpace(rest) == sessionID {
			return true
		}
	}
	return false
}

// openCodeImportAnswer answers whether the importer's own output is the answer for this document: the
// `Imported session: <id>` line naming this conversion's session id. `Session already exists` (which the
// harness prints on stderr with exit 0) is not that answer.
func openCodeImportAnswer(output, sessionID string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "Imported session:")
		if !ok {
			continue
		}
		id := strings.TrimSpace(rest)
		if id != sessionID {
			return id, false
		}
		return id, true
	}
	return "", false
}

// openCodeAnswerSentence words what the harness answered, for a failure that must carry the harness's own
// words rather than a sentence this build made up.
func openCodeAnswerSentence(output string) string {
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		return summariseText(trimmed, 300)
	}
	return "it answered nothing"
}

// VerifyOpenCodeSessionStructure reads the installed session back through the inherited AgentsView
// OpenCode reader and compares it, item by item, with what the writer intended to carry. dataDir is the
// target's data directory (the directory the harness reads sessions from); cfg supplies the provider
// configuration (its OpenCode roots are replaced, never mutated: the map is copied).
//
// The reference is the reader's own virtual path -- the store's absolute path and the session's id -- so
// the check reads the session this conversion installed rather than resolving the id through a session
// tree that could answer with another copy of it. The bytes the comparison's installed side reads are
// the document the importer was handed: the harness keeps no per-session file, and the document is what
// this conversion published to it (the importer's own fidelity is measured and recorded in the evidence
// record).
func VerifyOpenCodeSessionStructure(
	ctx context.Context, dataDir string, session OpenCodeSession, cfg config.Config,
) (StructureVerification, error) {
	return verifyStructure(ctx, dataDir, parser.AgentOpenCode,
		structureMethodOpenCode, structureReaderOpenCode, structureReaderOpenCode,
		openCodeStoreRef(dataDir, session.SessionID),
		func(string) ([]byte, error) { return session.Data, nil },
		func(readBack Conversation, installed []byte) roundTripComparison {
			return compareOpenCodeRoundTrip(session, readBack, installed)
		}, cfg)
}

// compareOpenCodeRoundTrip names every way a read-back of the installed session differs from the
// document the writer handed the importer, through the shared walk with the OpenCode reader's own rules.
//
// Two of those rules are the route's own. The reader renders a message's persisted reasoning into its
// text (`[Thinking]…[/Thinking]`, joined with the dialogue by line feeds), so the dialogue it shows for an
// assistant message is the rendering of the parts this writer wrote -- and the walk's intended side is
// spelled the same way, from the document's own bytes, because a comparison of the writer's dialogue
// against the reader's rendering would report every carried reasoning trace as a difference. The reader
// also keeps a system record (a compaction row included) as its own classification, which the route
// classifies away the way the family's routes do; a compaction row is compared as the boundary it is.
//
// What the walk resolves written messages against is the store's own dialogue -- the read-back's
// messages, under the route's own reader rules -- and not the document the importer was handed. Those
// bytes are the conversion's *intent*, not evidence about what the store holds, and this route's reader
// projects none of the records this writer writes (its records are the user's and the assistant's own
// messages, and the boundary row is compared as the boundary it is), so the store's own dialogue records
// are exactly the dialogue the read-back shows: a written message the store no longer holds fails
// verification as the difference it is, rather than being excused as a reading the reader projected away.
// The harness's own importer can drop a row silently (measured: an assistant message whose `time` carries
// `created` alone is accepted and then absent from the store), and the read-back is the only side that can
// show it.
//
// The document's own bytes are still read once, for the shape refusal this check owns: a caller that
// hands it bytes this writer does not write gets that answer rather than a comparison of the store
// against items that do not belong to those bytes.
func compareOpenCodeRoundTrip(
	session OpenCodeSession, readBack Conversation, installed []byte,
) roundTripComparison {
	if err := openCodeDocumentShape(installed); err != nil {
		return roundTripComparison{differences: []string{err.Error()}}
	}
	// A tool part's `state.input` is the one member the harness re-encodes: measured on 2.2.5, a
	// document's own `{"limit": 5, "file_path": "…"}` is held in the store -- and answered by the
	// harness's own reader -- as `{"limit":5,"file_path":"…"}`, so the store keeps the JSON *value* and
	// the recorded key order while the recorded whitespace is not what it keeps. Both sides are therefore
	// canonicalized to the value: the intended side from the source's own spelling, the read-back's calls
	// from the store's. A value that does not parse is left as recorded so the walk reports it instead of
	// hiding it.
	items := make([]TargetItem, 0, len(session.Items))
	for _, item := range session.Items {
		if item.Kind == ItemToolCall {
			item.InputJSON = canonicalJSONValue(item.InputJSON)
		}
		items = append(items, item)
	}
	reader := openCodeRoundTripReader()
	return compareRoundTrip(items, canonicalOpenCodeReadBack(readBack), reader,
		newDialogueRecords(readBackDialogueKeys(readBack, reader)))
}

// canonicalOpenCodeReadBack re-spells a read-back's tool calls through canonicalJSONValue, for the same
// reason the intended side is re-spelled: the store answers this member in its own spelling, and the walk
// compares the value. The messages and the calls they carry are copied rather than rewritten in place --
// the read-back belongs to the caller, and a check must not edit the evidence it was handed.
func canonicalOpenCodeReadBack(readBack Conversation) Conversation {
	messages := make([]Message, len(readBack.Messages))
	copy(messages, readBack.Messages)
	for index := range messages {
		if len(messages[index].ToolCalls) == 0 {
			continue
		}
		calls := make([]ToolCall, len(messages[index].ToolCalls))
		copy(calls, messages[index].ToolCalls)
		for call := range calls {
			calls[call].InputJSON = canonicalJSONValue(calls[call].InputJSON)
		}
		messages[index].ToolCalls = calls
	}
	readBack.Messages = messages
	return readBack
}

// canonicalJSONValue re-encodes one JSON value into the one spelling this comparison asks about: compact,
// with a map's keys in Go's own order (sorted). Both sides of the OpenCode round trip pass through it --
// the writer's item, which carries the source's spelling of the value, and the read-back's call, which
// carries the store's -- because neither spelling is what the walk is about. A number keeps its own
// spelling through `json.Number`, because re-encoding a large integer through a float would change the
// value the call recorded.
func canonicalJSONValue(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return text
	}
	return string(encoded)
}

// openCodeRoundTripReader is the inherited OpenCode reader's own rules for the read-back walk: a record
// the reader read as system-injected (a system, synthetic or skill row) carries no message, a boundary
// row is the boundary it marks, and the two roles the reader reads dialogue for stand for the user's and
// the assistant's messages. Its dialogue for one message is the message's own text, which is the
// rendering the reader built from the parts.
func openCodeRoundTripReader() roundTripReader {
	return roundTripReader{
		classifiesAway: func(message Message) bool { return message.IsSystem },
		kindOf: func(message Message) (TargetItemKind, bool) {
			switch parser.RoleType(message.Role) {
			case parser.RoleUser:
				return ItemUser, true
			case parser.RoleAssistant:
				return ItemAssistant, true
			default:
				return "", false
			}
		},
		dialogueOf: func(message Message) string { return message.Content },
	}
}
