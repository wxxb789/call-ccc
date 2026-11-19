package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/ccc/opencodefixture"
)

// openCodeStubEnv makes a child of this test binary behave as the OpenCode harness. The stub answers the
// harness's own protocol -- its version, its agent listing, its importer, its exporter and one resume turn
// -- so the CLI's own path (arguments, environment, the report, the printed line) is exercised without a
// host harness and without a platform-specific script.
const openCodeStubEnv = "CCC_TEST_STUB_OPENCODE"

// openCodeStubVersion is what the stub answers `--version` with: a version the route's own pattern reads,
// so the report carries a version the stub really printed.
const openCodeStubVersion = "opencode 2.2.5-stub"

// openCodeStubDataDir answers the harness's own data directory for the environment the stub was started
// with: the data root the caller named, spelled the way the harness itself resolves it.
func openCodeStubDataDir() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("the stub harness was started with no XDG_DATA_HOME")
	}
	return filepath.Join(root, "opencode"), nil
}

// openCodeStubDocumentPath answers where the stub keeps the document one session's exporter and turn
// answer from: a copy beside the store the importer wrote, under a directory name of the stub's own. The
// store itself is written through internal/ccc/opencodefixture, so the session the report verifies is
// read back by the inherited reader from the harness's own rows -- what the real harness's importer
// writes -- rather than from a document only this stub understands.
func openCodeStubDocumentPath(sessionID string) (string, error) {
	dataDir, err := openCodeStubDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "stub-documents", sessionID+".json"), nil
}

// stubOpenCode is the stub harness: it implements the importer, the exporter and one resume turn, and
// refuses everything else, so a check the stub is not about keeps its own honest answer.
func stubOpenCode(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Println(openCodeStubVersion)
		return 0
	case "debug":
		// Its own agent listing: one hidden primary agent, one subagent, and the primary agent a new
		// session of it runs under with the model its home resolves.
		fmt.Println(`[
			{"id":"hidden-agent","mode":"primary","hidden":true,"model":null},
			{"id":"explore","mode":"subagent","hidden":false,"model":null},
			{"id":"build","mode":"primary","hidden":false,` +
			`"model":{"id":"stub-model","providerID":"stub-provider","variant":"max"}}
		]`)
		return 0
	case "session":
		if len(args) < 2 {
			return 2
		}
		switch args[1] {
		case "import":
			return stubOpenCodeImport(args[2:])
		case "export":
			return stubOpenCodeExport(args[2:])
		default:
			fmt.Fprintln(os.Stderr, "the stub harness does not implement session "+args[1])
			return 2
		}
	case "run":
		return stubOpenCodeTurn(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "the stub harness does not implement "+args[0])
		return 2
	}
}

// stubOpenCodeImport answers the harness's own importer: the document the caller staged becomes a session
// of the stub's store, and the answer is the line the real harness prints.
func stubOpenCodeImport(args []string) int {
	if len(args) == 0 {
		return 2
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not read the staged document:", err)
		return 1
	}
	var document struct {
		Info struct {
			ID string `json:"id"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness refused the staged document:", err)
		return 1
	}
	if document.Info.ID == "" {
		fmt.Fprintln(os.Stderr, "the stub harness refused a document with no session id")
		return 1
	}
	documentPath, documentErr := openCodeStubDocumentPath(document.Info.ID)
	if documentErr != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not resolve its store:", documentErr)
		return 1
	}
	if _, err := os.Stat(documentPath); err == nil {
		// The harness's own answer for a session it already holds: this run did not install it.
		fmt.Fprintln(os.Stderr, "Session already exists")
		return 0
	}
	dataDir, err := openCodeStubDataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not resolve its store:", err)
		return 1
	}
	if _, _, err := opencodefixture.Store(dataDir, data); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not write the session:", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(documentPath), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not create its store:", err)
		return 1
	}
	if err := os.WriteFile(documentPath, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not write the session:", err)
		return 1
	}
	fmt.Println("Imported session: " + document.Info.ID)
	return 0
}

// stubOpenCodeExport answers the harness's own exporter: a session of the stub's store, or the harness's
// own refusal naming the id it does not hold.
func stubOpenCodeExport(args []string) int {
	if len(args) == 0 {
		return 2
	}
	id := args[0]
	path, err := openCodeStubDocumentPath(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not resolve its store:", err)
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Session not found: "+id)
		return 1
	}
	os.Stdout.Write(data)
	fmt.Println()
	return 0
}

// stubOpenCodeTurn answers one non-interactive turn: the session grows by the prompt and an answer, and the
// turn prints the answer the way the harness prints one. A session the stub does not hold is refused here
// (the stub, unlike the real harness, does not create one).
func stubOpenCodeTurn(args []string) int {
	sessionID, prompt := "", ""
	for index, argument := range args {
		if argument == "--session" && index+1 < len(args) {
			sessionID = args[index+1]
		}
		if argument == "--" && index+1 < len(args) {
			prompt = args[index+1]
		}
	}
	if sessionID == "" {
		fmt.Fprintln(os.Stderr, "the stub harness was asked for a turn in no session")
		return 2
	}
	path, err := openCodeStubDocumentPath(sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not resolve its store:", err)
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Session not found: "+sessionID)
		return 1
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not read the session:", err)
		return 1
	}
	messages, _ := document["messages"].([]any)
	now := time.Now().UnixMilli()
	next := len(messages)
	messages = append(messages,
		map[string]any{
			"id": fmt.Sprintf("msg_stub_%d", next), "type": "user", "files": []any{},
			"time": map[string]any{"created": now}, "text": prompt,
		},
		map[string]any{
			"id": fmt.Sprintf("msg_stub_%d", next+1), "type": "assistant", "agent": "build", "files": []any{},
			"time":    map[string]any{"created": now + 1, "completed": now + 2},
			"model":   map[string]any{"id": "stub-model", "providerID": "stub-provider"},
			"content": []any{map[string]any{"type": "text", "text": openCodeStubAnswer}},
		})
	document["messages"] = messages
	encoded, err := json.Marshal(document)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not encode the session:", err)
		return 1
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "the stub harness could not write the session:", err)
		return 1
	}
	fmt.Println(`{"type":"text","text":"` + openCodeStubAnswer + `"}`)
	return 0
}

// openCodeStubAnswer is what one stub turn answers: the words the case asks the turn for, so the report's
// own output can be told from an answer nobody printed.
const openCodeStubAnswer = "STUB-TURN-OK"

// stubOpenCodeBinary answers the path of the stub harness: this test binary, which TestMain routes into
// stubOpenCode. The environment variable is set for the CLI under test and inherited by the child.
func stubOpenCodeBinary(t *testing.T) string {
	t.Helper()
	t.Setenv(openCodeStubEnv, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolving the test binary: %v", err)
	}
	return executable
}

// openCodeHomeFor answers a target home whose data directory the harness itself would read, with a path a
// POSIX shell acts on (a space), so the printed line has to quote it.
func openCodeHomeFor(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "opencode home")
}

// TestContinueToOpenCodeThroughTheCLI pins the OpenCode target through the ordinary non-interactive
// command: the conversion, the installation through the harness's own importer and the verification
// levels are reported as separate states, the report names the format and its version, the invocation is
// the harness's own resume with the data root it reads, and the structural read-back opens the store the
// importer wrote.
func TestContinueToOpenCodeThroughTheCLI(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	home := openCodeHomeFor(t)
	binary := stubOpenCodeBinary(t)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "opencode",
		"--target-home", home, "--target-binary", binary, "--verify", "resume",
		"--resume-prompt", "Reply with exactly: "+openCodeStubAnswer, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)\n%s", code, exitOK, stderr, stdout)
	}
	var document struct {
		Target     string `json:"target"`
		Conversion struct {
			State    string `json:"state"`
			Format   string `json:"format"`
			ReadFrom string `json:"read_from"`
			Carried  struct {
				Messages             int `json:"messages"`
				UserMessages         int `json:"user_messages"`
				AssistantMessages    int `json:"assistant_messages"`
				CompactionBoundaries int `json:"compaction_boundaries"`
				ToolCalls            int `json:"tool_calls"`
				ToolResults          int `json:"tool_results"`
			} `json:"carried"`
			Losses []struct {
				Category string `json:"category"`
				Detail   string `json:"detail"`
			} `json:"losses"`
		} `json:"conversion"`
		Source struct {
			Boundary struct {
				State  string `json:"state"`
				SHA256 string `json:"sha256"`
			} `json:"boundary"`
		} `json:"source"`
		TargetSession struct {
			Provider          string            `json:"provider"`
			CLIVersion        string            `json:"cli_version"`
			CLIVersionSource  string            `json:"cli_version_source"`
			ModelProvider     string            `json:"model_provider"`
			Model             string            `json:"model"`
			SessionID         string            `json:"session_id"`
			FileName          *string           `json:"file_name"`
			Path              string            `json:"path"`
			ResumeCommand     string            `json:"resume_command"`
			ResumeEnvironment map[string]string `json:"resume_environment"`
			FormatLimits      []struct {
				Category string `json:"category"`
			} `json:"format_limits"`
		} `json:"target_session"`
		Installation struct {
			State            string `json:"state"`
			Path             string `json:"path"`
			Mechanism        string `json:"mechanism"`
			SourceCheckState string `json:"source_check_state"`
		} `json:"installation"`
		Verification struct {
			Structure struct {
				State  string `json:"state"`
				Method string `json:"method"`
				Detail string `json:"detail"`
			} `json:"structure"`
			Load struct {
				State  string `json:"state"`
				Detail string `json:"detail"`
			} `json:"load"`
			Resume struct {
				State    string `json:"state"`
				Output   string `json:"output"`
				ExitCode *int   `json:"exit_code"`
			} `json:"resume"`
		} `json:"verification"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Target != "opencode" || document.TargetSession.Provider != "opencode" {
		t.Errorf("target = %q / provider = %q, want the OpenCode target", document.Target,
			document.TargetSession.Provider)
	}
	// This route's artifact is a row inside the harness's own store, not a file: the document names the
	// store's path and the import mechanism, and no file_name a consumer could look for.
	if document.TargetSession.FileName != nil {
		t.Errorf("target_session.file_name = %q, want none: this route writes no file a caller could name",
			*document.TargetSession.FileName)
	}
	// The refusals this run asks for on purpose -- the exclusivity probe and the load check's unknown-id
	// control arm -- are the harness's own answers, and its diagnostics about them are not the caller's to
	// read: a successful run prints no `Session not found` line.
	if strings.Contains(stderr, "Session not found") {
		t.Errorf("stderr = %q, want the expected refusals' own diagnostics kept out of it", stderr)
	}
	if document.Conversion.State != "converted" || document.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", document.Conversion)
	}
	if document.Conversion.Format != "opencode-import-document" {
		t.Errorf("format = %q, want the importer's own document named", document.Conversion.Format)
	}
	// The version is the harness's own answer -- the stub prints one -- rather than the pin this writer
	// was measured against, and the report says which of the two it read.
	if document.TargetSession.CLIVersion != "2.2.5" || document.TargetSession.CLIVersionSource != "probe" {
		t.Errorf("cli version = %q (%q), want the version the harness printed, read from the probe",
			document.TargetSession.CLIVersion, document.TargetSession.CLIVersionSource)
	}
	if document.TargetSession.ModelProvider != "stub-provider" || document.TargetSession.Model != "stub-model" {
		t.Errorf("model = %q / provider = %q, want the target home's own answer named",
			document.TargetSession.Model, document.TargetSession.ModelProvider)
	}
	carried := document.Conversion.Carried
	if carried.Messages != 7 || carried.UserMessages != 2 || carried.AssistantMessages != 4 ||
		carried.ToolCalls != 1 || carried.ToolResults != 1 || carried.CompactionBoundaries != 1 {
		t.Errorf("carried = %+v, want the fixture's own conversation and tool evidence", carried)
	}
	if document.Source.Boundary.State == "" || document.Source.Boundary.SHA256 == "" {
		t.Errorf("source boundary = %+v, want the digest of the bytes this conversion covers",
			document.Source.Boundary)
	}
	if document.Installation.State != "installed" || document.Installation.Path !=
		filepath.Join(home, "opencode", "opencode.db") {
		t.Errorf("installation = %+v, want the session reported installed in the harness's own store",
			document.Installation)
	}
	if !strings.Contains(document.Installation.Mechanism, "opencode session import") {
		t.Errorf("mechanism = %q, want the harness's own importer named", document.Installation.Mechanism)
	}
	if document.Installation.SourceCheckState != "unchanged" {
		t.Errorf("source check = %q, want the source unchanged", document.Installation.SourceCheckState)
	}
	// Three states, three checks: the conversion, the importer's own answer, and the reader's own answer
	// about the installed session -- the structural read-back opens the store the importer wrote and
	// compares it with the document, so it is the check that would fail if the importer had answered
	// without writing.
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure = %+v, want the store the importer wrote read back as the document",
			document.Verification.Structure)
	}
	if document.Verification.Load.State != "verified" {
		t.Errorf("load = %+v, want the harness's own exporter to have verified the session",
			document.Verification.Load)
	}
	if document.Verification.Resume.State != "verified" {
		t.Errorf("resume = %+v, want the stub's own turn to have verified the resumed session",
			document.Verification.Resume)
	}
	if !strings.Contains(document.Verification.Resume.Output, openCodeStubAnswer) {
		t.Errorf("resume output = %q, want the turn's own answer", document.Verification.Resume.Output)
	}
	if document.Verification.Resume.ExitCode == nil || *document.Verification.Resume.ExitCode != 0 {
		t.Errorf("resume exit code = %v, want zero", document.Verification.Resume.ExitCode)
	}
	if got := document.TargetSession.ResumeEnvironment["XDG_DATA_HOME"]; got != home {
		t.Errorf("resume environment = %v, want the harness's own data root named",
			document.TargetSession.ResumeEnvironment)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "run --standalone --session "+
		document.TargetSession.SessionID) {
		t.Errorf("resume command = %q, want the harness's own resume named by id",
			document.TargetSession.ResumeCommand)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "XDG_DATA_HOME='"+home+"'") {
		t.Errorf("resume command = %q, want the data root named as the harness reads it (quoted: the path "+
			"holds a space)", document.TargetSession.ResumeCommand)
	}
	limits := map[string]bool{}
	for _, limit := range document.TargetSession.FormatLimits {
		limits[limit.Category] = true
	}
	for _, want := range []string{"session_container", "provider_and_model", "session_title_and_outcome"} {
		if !limits[want] {
			t.Errorf("format limits %v do not name %s", limits, want)
		}
	}
	losses := map[string]string{}
	for _, loss := range document.Conversion.Losses {
		losses[loss.Category] = loss.Detail
	}
	for _, want := range []string{"usage_zeroed", "target_model_id_attribution", "target_model_binding",
		"compaction_reason"} {
		if _, ok := losses[want]; !ok {
			t.Errorf("the report carries no %s entry", want)
		}
	}
	if !strings.Contains(losses["target_model_id_attribution"], "target home's own configured model id") {
		t.Errorf("target_model_id_attribution = %q, want the member's provenance stated",
			losses["target_model_id_attribution"])
	}
}

// TestContinueToOpenCodeHumanOutputCarriesTheResumeLine pins the human block: the file the run wrote is
// named as the store, and the printed invocation is the harness's own resume with the data root set --
// quoted, because a data directory's path is a value a shell acts on.
func TestContinueToOpenCodeHumanOutputCarriesTheResumeLine(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	home := openCodeHomeFor(t)
	binary := stubOpenCodeBinary(t)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "opencode",
		"--target-home", home, "--target-binary", binary, "--verify", "none")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)\n%s", code, exitOK, stderr, stdout)
	}
	if !strings.Contains(stdout, filepath.Join(home, "opencode", "opencode.db")) {
		t.Errorf("the human block does not name the store it wrote:\n%s", stdout)
	}
	if !strings.Contains(stdout, "run --standalone --session ") {
		t.Errorf("the human block does not print the resume command:\n%s", stdout)
	}
	if !strings.Contains(stdout, "XDG_DATA_HOME='"+home+"'") {
		t.Errorf("the human block does not name the data root:\n%s", stdout)
	}
	if !strings.Contains(stdout, "structure:    verified") {
		t.Errorf("the human block does not report the structural read-back:\n%s", stdout)
	}
	if !strings.Contains(stdout, "opencode-import-document") {
		t.Errorf("the human block does not name the format:\n%s", stdout)
	}
}

// TestContinueToOpenCodeWithoutTheHarnessThroughTheCLI pins the boundary through the command line: this
// target's installation is the harness's own importer, so a harness that is not installed is reported as
// the boundary it is -- naming the binary -- and nothing is reported as installed.
func TestContinueToOpenCodeWithoutTheHarnessThroughTheCLI(t *testing.T) {
	isolatedEnvironment(t)
	sessionID, _ := claudeFixture(t)
	home := openCodeHomeFor(t)
	code, stdout, stderr := runCLI(t, "continue", "claude:"+sessionID, "--to", "opencode",
		"--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code == exitOK {
		t.Fatalf("a run with no harness to import through exited %d\n%s", code, stdout)
	}
	if !strings.Contains(stderr, missingTargetBinary) || !strings.Contains(stderr, "importer") {
		t.Errorf("stderr = %q, want the missing harness and the importer named", stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "opencode", "opencode.db")); err == nil {
		t.Error("the failed run left a store behind")
	}
}
