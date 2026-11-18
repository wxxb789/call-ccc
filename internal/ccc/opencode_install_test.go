package ccc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/ccc/opencodefixture"
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

// openCodeTestInstallInput is the installation input one check is given: the home the conversion resolved,
// the executable a probe would have found, and a deadline.
func openCodeTestInstallInput(home, binary string) targetInstallInput {
	return targetInstallInput{
		Home:    home,
		Binary:  binary,
		Probe:   harnessProbe{Binary: binary, Found: true},
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
	t.Setenv(openCodeDataRootEnv, "/tmp/ccc-opencode-xdg-root")
	fromEnv, explicit, err := ResolveOpenCodeHome("")
	if err != nil {
		t.Fatalf("resolving the home from the environment: %v", err)
	}
	if explicit {
		t.Error("a home taken from the environment was reported as the caller's own")
	}
	if fromEnv != filepath.Join("/tmp/ccc-opencode-xdg-root", openCodeDataDirName) {
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

	// A missing harness is the boundary it is: this target's installation is the harness's own command.
	stubOpenCodeExecutable(t, func(name string) (string, error) {
		return "", errors.New("not found: " + name)
	})
	if _, _, err := InstallOpenCodeSession(context.Background(), session, openCodeTestInstallInput(home, "opencode")); err == nil {
		t.Fatal("installing without the harness succeeded")
	} else if !strings.Contains(err.Error(), openCodeImportMechanism[strings.Index(openCodeImportMechanism, "(")+1:len(openCodeImportMechanism)-1]) {
		t.Logf("refusal = %v (named the importer through its own sentence)", err)
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

// TestVerifyOpenCodeSessionStructureAcceptsTheImportersOwnReEncoding pins the one normalization the
// comparison makes for this target: the harness stores a tool part's `state.input` as a parsed value and
// re-encodes it, so the document's own `{"file_path": "…"}` is held and read back as `{"file_path":"…"}`
// (measured on 2.2.5). A store holding the same JSON value in the harness's own spelling is the same
// evidence, while a changed value is a difference -- the walk reports what the store holds, not the
// whitespace the source happened to record.
func TestVerifyOpenCodeSessionStructureAcceptsTheImportersOwnReEncoding(t *testing.T) {
	session := openCodeTestConversion(t)
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	writeOpenCodeTestStore(t, openCodeDataDir(home), session)
	database, err := sql.Open("sqlite3", openCodeStorePath(home))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer database.Close()
	rewrite := func(t *testing.T, mutate func(message map[string]any)) {
		t.Helper()
		var id, data string
		if err := database.QueryRow(`SELECT id, data FROM session_message WHERE type = ?
			ORDER BY seq LIMIT 1`, openCodeMessageAssistant).Scan(&id, &data); err != nil {
			t.Fatalf("finding an assistant message: %v", err)
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(data), &message); err != nil {
			t.Fatalf("parsing the message: %v", err)
		}
		mutate(message)
		encoded, err := json.Marshal(message)
		if err != nil {
			t.Fatalf("encoding the message: %v", err)
		}
		if _, err := database.Exec(`UPDATE session_message SET data = ? WHERE id = ?`, string(encoded), id); err != nil {
			t.Fatalf("rewriting the message: %v", err)
		}
	}

	// The importer's own spelling: re-encoding every member (which compacts the input's JSON) leaves the
	// evidence the same.
	rewrite(t, func(message map[string]any) {})
	verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
	if err != nil {
		t.Fatalf("reading the re-encoded session back: %v", err)
	}
	if verification.State != VerificationStateVerified {
		t.Fatalf("structure = %+v, want a store holding the same value in the importer's own spelling to "+
			"verify (detail %q)", verification, verification.Detail)
	}

	// A changed value is a difference, whichever piece of evidence it is. The attribution members (the
	// agent, the model) are not part of this walk: what it compares is the conversation the read-back
	// shows -- dialogue, calls, results, boundaries -- and the report names the attribution members
	// separately (the writer's own categories).
	for _, mutate := range []func(message map[string]any){
		func(message map[string]any) {
			if parts, ok := message["content"].([]any); ok && len(parts) > 0 {
				if part, ok := parts[0].(map[string]any); ok {
					part["text"] = "edited"
				}
			}
		},
		func(message map[string]any) { message["content"] = []any{} },
	} {
		before := func() string {
			var data string
			if err := database.QueryRow(`SELECT data FROM session_message WHERE type = ?
				ORDER BY seq LIMIT 1`, openCodeMessageAssistant).Scan(&data); err != nil {
				t.Fatalf("reading the message: %v", err)
			}
			return data
		}()
		rewrite(t, mutate)
		verification, err := VerifyOpenCodeSessionStructure(context.Background(), openCodeDataDir(home), session, fixtureConfig(t))
		if err != nil {
			t.Fatalf("reading the edited session back: %v", err)
		}
		if verification.State == VerificationStateVerified {
			t.Errorf("structure = %+v, want an edited assistant message reported as a difference", verification)
		}
		if _, err := database.Exec(`UPDATE session_message SET data = ? WHERE type = ?`,
			before, openCodeMessageAssistant); err != nil {
			t.Fatalf("restoring the message: %v", err)
		}
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
