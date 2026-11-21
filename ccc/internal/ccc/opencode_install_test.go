package ccc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/ccc/internal/ccc/opencodefixture"
)

// writeOpenCodeTestStore materialises one written document as the rows the harness's own importer would
// have written for it, so the route's structure check can read a real store back through the inherited
// reader instead of needing the harness installed. The rows are built by internal/ccc/opencodefixture,
// which is a fixture and not a writer of this build's: the product never writes SQL into an OpenCode
// store.
func writeOpenCodeTestStore(t *testing.T, dataDir string, written OpenCodeSession) string {
	t.Helper()
	path, sessionID, err := opencodefixture.Store(dataDir, written.Data)
	if err != nil {
		t.Fatalf("building the store: %v", err)
	}
	if written.SessionID != "" && sessionID != written.SessionID {
		t.Fatalf("the store holds session %s, want the document's own id %s", sessionID, written.SessionID)
	}
	return path
}

// openCodeStoreMessage reads one message out of a store into the writer's own type, so a case can change
// the member it is about and write the message back the way the store held it: what the case edits is a
// value, not a spelling. The first message of the type, in the store's own order.
func openCodeStoreMessage(t *testing.T, database *sql.DB, messageType string) (string, openCodeMessage) {
	t.Helper()
	var id, data string
	if err := database.QueryRow(`SELECT id, data FROM session_message WHERE type = ?
		ORDER BY seq LIMIT 1`, messageType).Scan(&id, &data); err != nil {
		t.Fatalf("finding a %s message: %v", messageType, err)
	}
	var message openCodeMessage
	if err := json.Unmarshal([]byte(data), &message); err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	return id, message
}

// writeOpenCodeStoreMessage writes one message back into the row it came from.
func writeOpenCodeStoreMessage(t *testing.T, database *sql.DB, id string, message openCodeMessage) {
	t.Helper()
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("encoding the message: %v", err)
	}
	if _, err := database.Exec(`UPDATE session_message SET data = ? WHERE id = ?`, string(encoded), id); err != nil {
		t.Fatalf("rewriting the message: %v", err)
	}
}

// openCodeTestInstallInput is the installation input one check is given: the home the conversion resolved,
// the executable a probe would have found, and a deadline.
func openCodeTestInstallInput(home, binary string) targetInstallInput {
	return targetInstallInput{
		Home:    home,
		Binary:  binary,
		Timeout: 30 * time.Second,
	}
}

// stubOpenCodeRun swaps one of the route's run seams for a case's own answer and restores it when the case
// ends: what a check makes of the harness's answer is the rule under test, so the answer is supplied
// instead of the host's harness.
func stubOpenCodeRun(t *testing.T, seam *openCodeHarnessRun, stub openCodeHarnessRun) {
	t.Helper()
	previous := *seam
	*seam = stub
	t.Cleanup(func() { *seam = previous })
}

// stubOpenCodeExecutable answers the executable lookup the same way, for the same reason.
func stubOpenCodeExecutable(t *testing.T, stub func(string) (string, error)) {
	t.Helper()
	previous := resolveOpenCodeExecutable
	resolveOpenCodeExecutable = stub
	t.Cleanup(func() { resolveOpenCodeExecutable = previous })
}

// openCodeTestConversion converts the committed Claude fixture as the route does.
func openCodeTestConversion(t *testing.T) OpenCodeSession {
	t.Helper()
	conversation := readFixtureConversation(t, claudeFixtureRef)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	return session
}

// TestOpenCodeHomeRules pins the one rule that makes the reported home, the environment the printed line
// sets and the directory the harness reads the same directory: the harness always reads
// `$XDG_DATA_HOME/opencode`, so a home whose base name is `opencode` is that directory (the variable takes
// its parent), and any other name is read as the XDG data home whose `opencode` directory holds the
// sessions.
func TestOpenCodeHomeRules(t *testing.T) {
	t.Setenv(openCodeDataRootEnv, "")
	home, explicit, err := ResolveOpenCodeHome(filepath.Join(t.TempDir(), openCodeDataDirName))
	if err != nil {
		t.Fatalf("resolving an explicit home: %v", err)
	}
	if !explicit {
		t.Error("an explicit home was not reported as the caller's own")
	}
	if openCodeDataRoot(home) != filepath.Dir(home) || openCodeDataDir(home) != home {
		t.Errorf("home %q resolves to root %q / data %q, want itself", home, openCodeDataRoot(home), openCodeDataDir(home))
	}
	other := filepath.Join(t.TempDir(), "scratch")
	rooted, _, err := ResolveOpenCodeHome(other)
	if err != nil {
		t.Fatalf("resolving a directory that is not named opencode: %v", err)
	}
	if openCodeDataRoot(rooted) != other || openCodeDataDir(rooted) != filepath.Join(other, openCodeDataDirName) {
		t.Errorf("home %q resolves to root %q / data %q, want the harness's own rule under it",
			rooted, openCodeDataRoot(rooted), openCodeDataDir(rooted))
	}
	// The data root is spelled the host's own way -- an absolute temporary directory, as a caller's own
	// XDG_DATA_HOME is -- rather than as a POSIX literal, because the value is resolved through the host's
	// own path rules: on a host whose spelling is not POSIX's, `/tmp/...` is not an absolute path, and the
	// rule this case is about is that the value is the *root* the home's `opencode` directory hangs below.
	dataRoot := filepath.Join(t.TempDir(), "ccc-opencode-xdg-root")
	t.Setenv(openCodeDataRootEnv, dataRoot)
	fromEnv, explicit, err := ResolveOpenCodeHome("")
	if err != nil {
		t.Fatalf("resolving the home from the environment: %v", err)
	}
	if explicit {
		t.Error("a home taken from the environment was reported as the caller's own")
	}
	if !filepath.IsAbs(fromEnv) {
		t.Errorf("home = %q, want an absolute directory: the store it names is read by its absolute path",
			fromEnv)
	}
	if fromEnv != filepath.Join(dataRoot, openCodeDataDirName) {
		t.Errorf("home = %q, want the harness's own directory below the environment's data home", fromEnv)
	}
}

// TestOpenCodeEnvironmentSetsTheDataRootAndNothingElse pins what every OpenCode command this route runs is
// given: the caller's own environment with the data root set and the database override removed (it would
// point the harness at another store), and -- where a check asks for them -- a scratch configuration and
// state directory.
func TestOpenCodeEnvironmentSetsTheDataRootAndNothingElse(t *testing.T) {
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	t.Setenv(openCodeDataRootEnv, "/tmp/caller-data-root")
	t.Setenv("OPENCODE_DB", "/tmp/caller-elsewhere.db")
	t.Setenv("OPENCODE_KEEP_ME", "yes")
	environment := openCodeEnvironment(home, map[string]string{"XDG_CONFIG_HOME": "/tmp/scratch-config"})
	got := map[string]string{}
	for _, entry := range environment {
		name, value, _ := strings.Cut(entry, "=")
		got[strings.ToUpper(name)] = value
	}
	if got[openCodeDataRootEnv] != filepath.Dir(home) {
		t.Errorf("%s = %q, want the home's data root %q", openCodeDataRootEnv, got[openCodeDataRootEnv], filepath.Dir(home))
	}
	if _, ok := got["OPENCODE_DB"]; ok {
		t.Error("the caller's database override reached the harness, which would write into another store")
	}
	if got["OPENCODE_KEEP_ME"] != "yes" {
		t.Error("the caller's own environment was not carried")
	}
	if got["XDG_CONFIG_HOME"] != "/tmp/scratch-config" {
		t.Errorf("XDG_CONFIG_HOME = %q, want the scratch configuration this call asked for", got["XDG_CONFIG_HOME"])
	}
}

// TestInstallOpenCodeSessionAsksTheHarnesssOwnImporter pins the installation: the harness's own importer
// is what is run, the document is staged outside the home (and removed once the importer has it), the
// answer it must give is its own `Imported session: <id>` line naming this conversion's id, and the two
// ways the store can already hold the id are both refused rather than reported as this run's install.
func TestInstallOpenCodeSessionAsksTheHarnesssOwnImporter(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	var invocations [][]string
	var stagedPath string
	var staged []byte
	var environment []string
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		invocations = append(invocations, argv)
		environment = env
		if len(argv) > 3 && argv[0] == "session" && argv[1] == "import" {
			stagedPath = argv[2]
			data, err := os.ReadFile(stagedPath)
			if err != nil {
				return "", "", err
			}
			staged = data
			return "Imported session: " + session.SessionID + "\n", "", nil
		}
		// The exclusivity question, asked before anything is staged: the id does not resolve.
		return "", "Session not found: " + session.SessionID + "\n", errors.New("exit status 1")
	})
	path, mechanism, err := InstallOpenCodeSession(context.Background(), session, openCodeTestInstallInput(home, "opencode"))
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	if path != openCodeStorePath(home) {
		t.Errorf("path = %q, want the store the session lives in (%q)", path, openCodeStorePath(home))
	}
	if !strings.Contains(mechanism, "opencode session import") {
		t.Errorf("mechanism = %q, want it to name the harness's own importer", mechanism)
	}
	if string(staged) != string(session.Data) {
		t.Error("the importer was handed bytes other than the conversion's document")
	}
	if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the staged document %s was left behind", stagedPath)
	}
	if len(invocations) != 2 || invocations[1][1] != "import" ||
		invocations[1][len(invocations[1])-1] != "--standalone" {
		t.Fatalf("invocations = %q, want the exclusivity question and then the importer", invocations)
	}
	importArguments := strings.Join(invocations[1], " ")
	if !strings.Contains(importArguments, "--directory "+session.Cwd) {
		t.Errorf("import arguments = %q, want the session's recorded directory handed to the importer", importArguments)
	}
	for _, entry := range environment {
		if strings.HasPrefix(entry, openCodeDataRootEnv+"=") {
			if entry != openCodeDataRootEnv+"="+openCodeDataRoot(home) {
				t.Errorf("environment = %q, want the home's data root named", entry)
			}
		}
	}

	// An `already exists` answer (which the harness prints on stderr with exit 0) is not this run's
	// install.
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		if len(argv) > 1 && argv[1] == "import" {
			return "", "Session already exists\n", nil
		}
		return "", "Session not found: " + session.SessionID + "\n", errors.New("exit status 1")
	})
	if _, _, err := InstallOpenCodeSession(context.Background(), session, openCodeTestInstallInput(home, "opencode")); err == nil {
		t.Fatal("an `already exists` answer was reported as this run's install")
	}

	// An id that already resolves is refused before the importer is run at all.
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		return `{"info":{"id":"` + session.SessionID + `"},"messages":[]}`, "", nil
	})
	if _, _, err := InstallOpenCodeSession(context.Background(), session, openCodeTestInstallInput(home, "opencode")); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("installing over an existing id: %v, want ErrTargetExists", err)
	}

	// A missing harness is the boundary it is: this target's installation is the harness's own command,
	// and the refusal names both the executable that is missing and the importer this route needs.
	stubOpenCodeExecutable(t, func(name string) (string, error) {
		return "", errors.New("not found: " + name)
	})
	if _, _, err := InstallOpenCodeSession(context.Background(), session, openCodeTestInstallInput(home, "opencode")); err == nil {
		t.Fatal("installing without the harness succeeded")
	} else if !strings.Contains(err.Error(), "opencode") || !strings.Contains(err.Error(), "importer") {
		t.Errorf("refusal = %v, want the missing harness and this target's importer named", err)
	}
}

// TestInstallOpenCodeSessionKeepsTheImportersOwnRefusal pins the nonzero branch's evidence rule: an
// import the harness refuses (measured: a missing recorded directory answers
// `LocationNotFoundError: Location not found: <dir>` and exits 1) must carry the harness's own words in
// the returned error -- a core API caller that left ContinueOptions.Stderr unset has nowhere else to read
// them -- while the process error stays wrapped for errors.Is/As. The refusal is also the case that must
// *stay* a refusal: the reconciliation asks the harness's own reader, it answers that the id does not
// resolve, and no publication is claimed.
func TestInstallOpenCodeSessionKeepsTheImportersOwnRefusal(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	refusalErr := errors.New("exit status 1")
	const refusedText = "LocationNotFoundError: Location not found: /gone"
	exports := 0
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		if len(argv) > 1 && argv[1] == "import" {
			return "", refusedText + "\n", refusalErr
		}
		exports++
		return "", "Session not found: " + session.SessionID + "\n", refusalErr
	})
	path, _, err := InstallOpenCodeSession(context.Background(), session,
		openCodeTestInstallInput(home, "opencode"))
	if err == nil {
		t.Fatal("a refused import was reported as an install")
	}
	if !strings.Contains(err.Error(), refusedText) {
		t.Errorf("refusal = %v, want the harness's own refusal preserved", err)
	}
	if !errors.Is(err, refusalErr) {
		t.Errorf("refusal = %v, want the process error still wrapped", err)
	}
	if errors.Is(err, ErrInstallationUnresolved) {
		t.Errorf("refusal = %v, want the store's own answer read as an answer: it refused this id", err)
	}
	if path != "" {
		t.Errorf("path = %q, want none: the harness's own reader answered that the store does not hold it", path)
	}
	// The refusal is the harness's answer, so the question was asked once beside the import: a store the
	// reader could not answer about is a different outcome (the unresolved case below).
	if exports != 2 {
		t.Errorf("exports = %d, want the exclusivity question and the one reconciliation question", exports)
	}
}

// TestInstallOpenCodeSessionReconcilesAFailedImport pins what a failed importer process means for this
// route. Measured on 2.2.5, the harness's own importer commits the session and then exits, so a deadline
// or a cancellation between those two moments leaves the session in the store while this run observed
// only a process error -- and a run that reported that as "nothing was published" would have its caller
// convert the same conversation again, leaving a second snapshot of it in the store with no record that
// either happened. Each arm is the evidence its own scenario leaves, and the path the installer returns
// is what the report answers "installed?" from (continue.go):
//
//   - the harness's own `Imported session: <id>` line was captured before the process failed: its own
//     acknowledgment is the answer, and the published identity travels beside the error;
//   - no acknowledgment, and the harness's own reader now resolves the id this conversion minted: the
//     store holds the session, so the path travels beside the error;
//   - no acknowledgment, and the reader answers its own `Session not found: <id>`: nothing was published,
//     which is the one refusal this installer may report as an absence;
//   - no acknowledgment and no answer from the reader: the outcome is not established, and the error
//     carries ErrInstallationUnresolved so the report can say `unresolved` rather than `not-installed`.
//
// The reconciliation also has to survive the failure that made it necessary: the caller's own deadline
// (or cancellation) is what stopped the import, so the arm whose context is already dead is the one that
// pins the question being asked anyway -- and the stub honours the context it is handed, exactly as the
// route's own runner does (`runHarnessInvocation`), so an unanswered question cannot pass by the stub
// ignoring the deadline it was given.
func TestInstallOpenCodeSessionReconcilesAFailedImport(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	importFailure := errors.New("timed out after 30s")
	document := `{"info":{"id":"` + session.SessionID + `"},"messages":[]}`

	arms := []struct {
		name        string
		cancelled   bool
		imported    func() (string, string, error)
		probe       func() (string, string, error)
		wantPath    bool
		wantExports int
		unresolved  bool
		wantError   []string
	}{
		{
			name: "acknowledged and then failed",
			imported: func() (string, string, error) {
				return "Imported session: " + session.SessionID + "\n", "", importFailure
			},
			probe: func() (string, string, error) {
				// Never reached: the harness's own acknowledgment is already the answer.
				return "", "", errors.New("the reconciliation asked although the output answered")
			},
			wantPath:    true,
			wantExports: 1,
			wantError:   []string{"Imported session: " + session.SessionID},
		},
		{
			name: "the store holds it though nothing was acknowledged",
			imported: func() (string, string, error) {
				return "", "", importFailure
			},
			probe:       func() (string, string, error) { return document, "", nil },
			wantPath:    true,
			wantExports: 2,
			wantError:   []string{"the store now holds the session"},
		},
		{
			name:      "the caller's own deadline stopped the import and the store still answers",
			cancelled: true,
			imported: func() (string, string, error) {
				return "", "", context.Canceled
			},
			probe:       func() (string, string, error) { return document, "", nil },
			wantPath:    true,
			wantExports: 2,
			wantError:   []string{"the store now holds the session"},
		},
		{
			name: "the reader answers that the store does not hold it",
			imported: func() (string, string, error) {
				return "", "LocationNotFoundError: Location not found: /gone\n", importFailure
			},
			probe: func() (string, string, error) {
				return "", "Session not found: " + session.SessionID + "\n", errors.New("exit status 1")
			},
			wantExports: 2,
			wantError:   []string{"LocationNotFoundError"},
		},
		{
			name: "nothing answers whether it was published",
			imported: func() (string, string, error) {
				return "", "", importFailure
			},
			probe: func() (string, string, error) {
				return "", "", errors.New("unable to open database file")
			},
			unresolved:  true,
			wantExports: 2,
			wantError:   []string{"unable to open database file"},
		},
	}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			exports, imports := 0, 0
			stubOpenCodeRun(t, &runOpenCodeImporter, func(
				ctx context.Context, _ string, _ string, argv []string, _ []string, _ time.Duration,
				_ io.Writer,
			) (string, string, error) {
				// The invocation is counted before the deadline is applied: the count is what the route
				// asked of the harness, not what the harness could answer, and the cancelled arm is the
				// one whose questions the caller's own context stops.
				if len(argv) > 1 && argv[1] == "import" {
					imports++
					if err := ctx.Err(); err != nil {
						return "", "", err
					}
					return arm.imported()
				}
				if len(argv) != 4 || argv[0] != "session" || argv[1] != "export" || argv[2] != session.SessionID {
					return "", "", fmt.Errorf("unexpected invocation %q", argv)
				}
				exports++
				// The route's own runner fails an invocation whose context is done, so the stub does too:
				// an arm that passed without the reconciliation's own context would not be pinning it.
				if err := ctx.Err(); err != nil {
					return "", "", err
				}
				if exports == 1 {
					// The exclusivity question, asked before anything is staged: the id does not resolve.
					return "", "Session not found: " + session.SessionID + "\n", errors.New("exit status 1")
				}
				return arm.probe()
			})
			ctx := context.Background()
			if arm.cancelled {
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			path, mechanism, err := InstallOpenCodeSession(ctx, session,
				openCodeTestInstallInput(home, "opencode"))
			if err == nil {
				t.Fatal("a failed importer process was reported as an install with no failure")
			}
			if errors.Is(err, ErrInstallationUnresolved) != arm.unresolved {
				t.Errorf("err = %v, want the unresolved outcome %v", err, arm.unresolved)
			}
			if want := openCodeStorePath(home); (path == want) != arm.wantPath {
				t.Errorf("path = %q, want the store's own path %v (%q)", path, arm.wantPath, want)
			}
			if arm.wantPath && !strings.Contains(mechanism, "opencode session import") {
				t.Errorf("mechanism = %q, want the harness's own importer named beside the failure", mechanism)
			}
			for _, want := range arm.wantError {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to carry %q", err, want)
				}
			}
			if imports != 1 {
				t.Errorf("imports = %d, want the one the arm's importer answered", imports)
			}
			// The question is asked once per failed import, and it is asked even where the caller's own
			// context is already done -- that is what keeps the report from asserting an absence nobody
			// observed. An answered acknowledgment is the one case that needs no question asked.
			if exports != arm.wantExports {
				t.Errorf("exports = %d, want %d: the exclusivity question%s", exports, arm.wantExports,
					" and the reconciliation question a failed import that acknowledged nothing is asked")
			}
		})
	}
}

// TestOpenCodeRefusalNamesSession pins what counts as the harness's own answer that its store does not
// hold one session: the `Session not found: <id>` line naming *that* id. The reconciliation reads the
// absence only from this answer, so anything else -- a refusal about another session, a diagnostic that
// merely mentions the id, an empty answer -- is not read as this store's answer about this id and leaves
// the outcome unresolved rather than claimed absent.
func TestOpenCodeRefusalNamesSession(t *testing.T) {
	const id = "ses_01J0000000000000000000000"
	for _, testCase := range []struct {
		name   string
		answer string
		want   bool
	}{
		{name: "the harness's own refusal", answer: "Session not found: " + id + "\n", want: true},
		{name: "the refusal beside other diagnostics",
			answer: "warning: a config file was ignored\nSession not found: " + id, want: true},
		{name: "a refusal about another session",
			answer: "Session not found: ses_other00000000000000000000\n"},
		{name: "a failure that mentions the id without refusing it",
			answer: "Error: unable to open database file (session " + id + ")"},
		{name: "a refusal naming no session", answer: "Session not found: \n"},
		{name: "no answer at all", answer: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := openCodeRefusalNamesSession(testCase.answer, id); got != testCase.want {
				t.Errorf("openCodeRefusalNamesSession(%q, %q) = %v, want %v", testCase.answer, id, got,
					testCase.want)
			}
		})
	}
}

// TestVerifyOpenCodeSessionStructureReadsTheStoreBack pins the structure check end to end: the inherited
// OpenCode reader resolves the session by its own `<store>#<id>` reference, reads the dialogue (the
// rendering the document's parts produce under a read), the call with its result and the boundary, and
// the comparison answers verified with no difference -- while a store whose own row was edited answers
// not-verified with the difference named.
func TestVerifyOpenCodeSessionStructureReadsTheStoreBack(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the installed session verified (detail %q)", verification, verification.Detail)
	}
	if verification.TargetPath != openCodeStoreRef(openCodeDataDir(home), session.SessionID) {
		t.Errorf("target path = %q, want the reader's own reference to the store and the session",
			verification.TargetPath)
	}
	counts := verification.ReadBack
	// The reader reads the compaction row as a system record under the user role, so the user count
	// includes it while the boundary count names it; the rest are the document's own messages.
	if counts.Messages != 7 || counts.UserMessages != 3 || counts.AssistantMessages != 4 ||
		counts.ToolCalls != 1 || counts.ToolResults != 1 || counts.CompactionBoundaries != 1 {
		t.Errorf("read back = %+v, want the document's own seven messages and its call/result pair", counts)
	}
	if verification.NotReadBackVerbatim != 0 {
		t.Errorf("not_read_back_verbatim = %d, want every written message read back", verification.NotReadBackVerbatim)
	}
	if verification.Detail != "" {
		t.Errorf("detail = %q, want none: the read-back showed every written item", verification.Detail)
	}

	// The installed store is the evidence the check rests on: a row the harness's own reader would read
	// differently is a difference, not a pass.
	database, err := sql.Open("sqlite3", openCodeStorePath(home))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer database.Close()
	var id string
	if err := database.QueryRow(`SELECT id FROM session_message WHERE type = ? ORDER BY seq LIMIT 1`,
		openCodeMessageUser).Scan(&id); err != nil {
		t.Fatalf("finding a user message: %v", err)
	}
	if _, err := database.Exec(`UPDATE session_message SET data = json_set(data, '$.text', 'edited') WHERE id = ?`,
		id); err != nil {
		t.Fatalf("editing the store: %v", err)
	}
	verification, err = VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the edited session back: %v", err)
	}
	if verification.State != VerificationStateNotVerified {
		t.Fatalf("structure = %+v, want a store whose dialogue was edited reported as a difference", verification)
	}
	if !strings.Contains(verification.Detail, "dialogue") {
		t.Errorf("detail = %q, want it to name the dialogue difference", verification.Detail)
	}
}

// TestVerifyOpenCodeSessionStructureFailsWhenTheStoreDroppedARow pins the check's evidence rule for this
// route: a written message is resolved against the *store's* own dialogue as the inherited reader reads
// it back, never against the document the importer was handed -- those bytes are the conversion's intent.
// The harness's own importer silently drops an assistant message whose `time` carries `created` alone
// (measured), so a store that no longer holds a written message must fail verification as the difference
// it is, not be excused as a projection of the reader's with the document standing in as evidence.
func TestVerifyOpenCodeSessionStructureFailsWhenTheStoreDroppedARow(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true, Timestamp: at},
		// The dialogue-only assistant message the measured silent drop leaves no row for.
		Message{Role: "assistant", Content: "hi", Dialogue: "hi", DialogueProjected: true,
			Model: "source-model", Timestamp: at.Add(time.Second)},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	database, err := sql.Open("sqlite3", openCodeStorePath(home))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer database.Close()
	id, _ := openCodeStoreMessage(t, database, openCodeMessageAssistant)
	if _, err := database.Exec(`DELETE FROM session_message WHERE id = ?`, id); err != nil {
		t.Fatalf("dropping the store's row: %v", err)
	}
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home),
		session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the dropped session back: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Fatalf("structure = %+v, want a store that no longer holds a written message reported as a "+
			"difference", verification)
	}
	if verification.NotReadBackVerbatim != 0 {
		t.Errorf("not_read_back_verbatim = %d, want 0: the message is not in the store, so the reader made "+
			"no projection of it", verification.NotReadBackVerbatim)
	}
	if !strings.Contains(verification.Detail, "dialogue") {
		t.Errorf("detail = %q, want it to name the dialogue the store does not hold", verification.Detail)
	}
}

// TestVerifyOpenCodeSessionStructureAcceptsTheImportersOwnReEncoding pins the comparison's rule for a tool
// part's `state.input`, the one member the harness re-encodes: measured on 2.2.5, the store keeps the JSON
// value and the key order the document recorded while dropping the recorded whitespace, so the source's
// own spelling of the value is not what the walk asks about. A call whose input records two keys in an
// order Go's own encoder would not write verifies through the store's spelling of the same value, while a
// changed value is the difference the walk names.
func TestVerifyOpenCodeSessionStructureAcceptsTheImportersOwnReEncoding(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	// The source's own spelling: the recorded key order, and the whitespace the source recorded. The
	// document compacts the member when it is written, and that is the spelling the store holds -- the
	// fixture writes the document's own member bytes, and the harness's importer keeps the recorded order
	// while dropping the recorded whitespace (measured) -- so the writer's item and the store's member are
	// two spellings of one value from here on.
	recordedInput := `{"limit": 5, "file_path": "internal/retry/policy.go"}`
	conversation := openCodeTestConversation(
		Message{Role: "assistant", Timestamp: at,
			ToolCalls: []ToolCall{openCodeTestCall("toolu_reencoded", "Read", recordedInput)}},
		Message{Role: "user", Timestamp: at.Add(time.Second), ToolResults: []ToolResult{
			openCodeTestResult("toolu_reencoded", "completed", `"the recorded output"`)}},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	// The case's own input, stated so the dimension it names cannot go missing silently: the document
	// holds the recorded key order, not Go's sorted spelling of it.
	if !strings.Contains(string(session.Data), `"input":{"limit":5,"file_path":"internal/retry/policy.go"}`) {
		t.Fatalf("the document does not hold the recorded key order this case is about: %s", session.Data)
	}
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	database, err := sql.Open("sqlite3", openCodeStorePath(home))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer database.Close()

	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the store's own spelling of the recorded value to verify (detail %q)",
			verification, verification.Detail)
	}
	if verification.Detail != "" {
		t.Errorf("detail = %q, want none: the read-back showed every written item", verification.Detail)
	}

	// The value is what the walk asks about: the store holding the same spelling with the call's value
	// changed is the difference it names.
	id, message := openCodeStoreMessage(t, database, openCodeMessageAssistant)
	changed := false
	for index := range message.Content {
		part := &message.Content[index]
		if part.Type == openCodePartTool && part.ID == "toolu_reencoded" {
			part.State.Input = json.RawMessage(`{"limit":7,"file_path":"internal/retry/policy.go"}`)
			changed = true
		}
	}
	if !changed {
		t.Fatal("the store holds no part for the call this case is about")
	}
	writeOpenCodeStoreMessage(t, database, id, message)
	verification, err = VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the edited session back: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Errorf("structure = %+v, want a changed tool input reported as a difference", verification)
	}
	if !strings.Contains(verification.Detail, "tool call") {
		t.Errorf("detail = %q, want it to name the tool call difference", verification.Detail)
	}
}

// TestVerifyOpenCodeSessionStructureReadsPersistedReasoningBack pins the route's own dialogue rule through
// a read-back: the inherited reader renders an assistant message's persisted reasoning as
// `[Thinking]…[/Thinking]` joined with the dialogue by a line feed, and the walk's intended side is spelled
// by openCodePartsDialogue for the same reason. The fixture the other read-back cases convert carries no
// persisted reasoning, so this is the case that dimension has: a store holding the trace verifies, while a
// store whose trace re-renders as other words is the dialogue difference the walk names.
func TestVerifyOpenCodeSessionStructureReadsPersistedReasoningBack(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	conversation := openCodeTestConversation(
		Message{Role: "user", Content: "hello", Dialogue: "hello", DialogueProjected: true, Timestamp: at},
		Message{Role: "assistant", Content: "hi", Dialogue: "hi", DialogueProjected: true,
			Thinking: "weigh the options", Model: "source-model", Timestamp: at.Add(time.Second)},
	)
	session, err := ConvertToOpenCodeSession(conversation, openCodeTargetTestOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want the reasoning-carrying message read back through the reader's own "+
			"rendering (detail %q)", verification, verification.Detail)
	}
	if verification.ReadBack.UserMessages != 1 || verification.ReadBack.AssistantMessages != 1 {
		t.Errorf("read back = %+v, want the two messages the document writes", verification.ReadBack)
	}
	if verification.NotReadBackVerbatim != 0 {
		t.Errorf("not_read_back_verbatim = %d, want the reasoning-carrying message read back verbatim",
			verification.NotReadBackVerbatim)
	}

	// The trace's own text is evidence: a store whose reasoning text re-renders as other words is not the
	// written message, and the walk names the dialogue difference. (This replacement's words are not a run
	// of the written text, so nothing can pair it as that message's reading.)
	database, err := sql.Open("sqlite3", openCodeStorePath(home))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer database.Close()
	id, message := openCodeStoreMessage(t, database, openCodeMessageAssistant)
	if len(message.Content) == 0 || message.Content[0].Type != openCodePartReasoning {
		t.Fatalf("the assistant message carries %+v, want the reasoning part this case is about first",
			message.Content)
	}
	message.Content[0].Text = "a different trace"
	writeOpenCodeStoreMessage(t, database, id, message)
	verification, err = VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the edited session back: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Errorf("structure = %+v, want a store whose reasoning text was changed reported as a difference",
			verification)
	}
	if !strings.Contains(verification.Detail, "dialogue") {
		t.Errorf("detail = %q, want it to name the dialogue difference", verification.Detail)
	}
}

// TestVerifyOpenCodeSessionStructureRefusesANonDocument pins the check's own answer about bytes that are
// not the document this writer writes: the comparison reports the shape failure instead of a pass.
func TestVerifyOpenCodeSessionStructureRefusesANonDocument(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	bare := session
	bare.Data = []byte("{")
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), bare, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the installed session back: %v", err)
	}
	if verification.State == VerificationStateVerified {
		t.Fatal("a document the writer never writes verified")
	}
	if !strings.Contains(verification.Detail, ErrOpenCodeDocumentShape.Error()) {
		t.Errorf("detail = %q, want the shape refusal named", verification.Detail)
	}
}
