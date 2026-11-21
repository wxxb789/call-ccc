package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/ccc/internal/ccc"
)

// ompFixtureRef is the committed Oh My Pi session, as the reference the CLI takes: the provider is the
// agent's own label and the id is the one its header records.
const ompFixtureRef = "omp:01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4"

// ompShellUnsafeHome answers an OMP data directory whose path a POSIX shell would act on -- it holds a
// space -- so the printed resume line has to quote it on every host, for the same reason the other
// routes' own helper does.
func ompShellUnsafeHome(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "omp home")
}

// TestContinueToOMPConvertsInstallsAndReportsTheRoute pins the OMP target through the CLI: one documented
// non-interactive invocation converts a supported source session into a new OMP session, the report
// distinguishes conversion, installation and verification, the file lands under OMP's own project-directory
// spelling, and the printed resume invocation names the data directory, the session's own directory and
// the working directory.
func TestContinueToOMPConvertsInstallsAndReportsTheRoute(t *testing.T) {
	isolatedEnvironment(t)
	home := ompShellUnsafeHome(t)
	// The directory OMP files the fixture's recorded working directory under, asked of OMP's own rule:
	// the spelling's own literal cases (including the `--…--` outside-both form) are the route table's
	// (`internal/ccc/omp_target_test.go`), while a literal here would be host-shaped -- on a platform whose
	// rooted paths carry a volume, the absolute form spells that into the name too.
	sessionsDir := filepath.Join(home, "sessions", ccc.OMPProjectDir("/home/user/code/example-api"))
	sourcePath := filepath.Join(fixtureRoot(t), "claude", "-home-user-code-example-api",
		"9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl")
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source fixture: %v", err)
	}

	code, stdout, stderr := runCLI(t, "continue", "claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10",
		"--to", "omp", "--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Target != "omp" || document.TargetSession.Provider != "omp" {
		t.Errorf("target = %q / provider = %q, want the OMP route", document.Target, document.TargetSession.Provider)
	}
	if document.Conversion.State != "converted" || document.Conversion.Format != "omp-session-jsonl" {
		t.Errorf("conversion = %+v, want a converted OMP session", document.Conversion)
	}
	if document.Installation.State != "installed" {
		t.Fatalf("installation = %+v, want the session installed", document.Installation)
	}
	// The installer asks OMP's own rule rather than Pi's unconditionally; the route table's case covers the
	// branches, and this one pins the file the CLI reported is really there, under the directory that rule
	// gives for the fixture's working directory.
	if filepath.Dir(document.TargetSession.Path) != sessionsDir {
		t.Errorf("installed under %s, want OMP's own spelling of the working directory (%s)",
			filepath.Dir(document.TargetSession.Path), sessionsDir)
	}
	if _, err := os.Stat(document.TargetSession.Path); err != nil {
		t.Errorf("the reported path %s is not there: %v", document.TargetSession.Path, err)
	}
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure = %+v, want the installed session read back as written",
			document.Verification.Structure)
	}
	// The report names the limits this format has: the family's shared ones under OMP's own name, plus the
	// two OMP adds -- its title slot and the session-level model binding its loader restores a session
	// from. Pi's own `model_provider_binding` limit is *not* named here: an OMP session does bind a model,
	// so carrying that sentence over would state the opposite of what this format does.
	limits := map[string]bool{}
	for _, limit := range document.TargetSession.FormatLimits {
		limits[limit.Category] = true
	}
	for _, want := range []string{"session_model_binding", "title_slot", "turn_boundaries",
		"attachments_and_images", "tool_vocabulary"} {
		if !limits[want] {
			t.Errorf("format limits = %v, want %s named", limits, want)
		}
	}
	if limits["model_provider_binding"] {
		t.Errorf("format limits = %v: Pi's provider-binding limit claims a session binds no provider, "+
			"which is the opposite of what an OMP session's own records carry", limits)
	}
	named := false
	for _, limit := range document.TargetSession.FormatLimits {
		if strings.Contains(limit.Detail, "a Pi session") {
			t.Errorf("format limit %s = %q names Pi's session, want this format's own harness",
				limit.Category, limit.Detail)
		}
		named = named || strings.Contains(limit.Detail, "an Oh My Pi session")
	}
	if !named {
		t.Error("no format limit names this format's own harness: the family's sentences were carried " +
			"over unchanged")
	}
	if document.TargetSession.ResumeEnvironment["PI_CODING_AGENT_DIR"] != home {
		t.Errorf("resume environment = %v, want the data directory named", document.TargetSession.ResumeEnvironment)
	}
	if document.TargetSession.ResumeEnvironment["PI_CODING_AGENT_SESSION_DIR"] != sessionsDir {
		t.Errorf("resume environment = %v, want the directory holding the session's file named: naming the "+
			"sessions root above it answers that no such session is installed",
			document.TargetSession.ResumeEnvironment)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "cd /home/user/code/example-api") {
		t.Errorf("resume command = %q, want it to change into the session's working directory",
			document.TargetSession.ResumeCommand)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "--resume "+document.TargetSession.SessionID) {
		t.Errorf("resume command = %q, want the session named by id", document.TargetSession.ResumeCommand)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "PI_CODING_AGENT_DIR='"+home+"'") {
		t.Errorf("resume command = %q, want the data directory quoted, not left bare (the home holds a space)",
			document.TargetSession.ResumeCommand)
	}
	if after, err := os.ReadFile(sourcePath); err != nil || string(after) != string(sourceBefore) {
		t.Errorf("the source fixture moved: %v", err)
	}
	if entries, err := os.ReadDir(sessionsDir); err != nil {
		t.Fatalf("reading the target home: %v", err)
	} else if len(entries) != 1 {
		t.Errorf("the target home holds %d session(s), want exactly the one that was installed", len(entries))
	}
}

// TestContinueFromOMPThroughTheCLI pins the source direction: the same command line continues an OMP
// session in a previously supported harness, and the report names the inherited OMP reader as the read's
// source.
func TestContinueFromOMPThroughTheCLI(t *testing.T) {
	isolatedEnvironment(t)
	home := t.TempDir()
	code, stdout, stderr := runCLI(t, "continue", ompFixtureRef, "--to", "claude",
		"--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document struct {
		reportedContinueDocument
		Source struct {
			Identity struct {
				Provider string `json:"provider"`
				Agent    string `json:"agent"`
			} `json:"identity"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Source.Identity.Provider != "omp" || document.Source.Identity.Agent != "OhMyPi" {
		t.Fatalf("source = %+v, want the OMP session reported as the source", document.Source)
	}
	if document.Conversion.State != "converted" || document.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", document.Conversion)
	}
	if document.Conversion.Carried.ToolCalls == 0 || document.Conversion.Carried.ToolResults == 0 {
		t.Errorf("carried = %+v, want the fixture's call and its result", document.Conversion.Carried)
	}
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure = %+v, want the transcript read back as written", document.Verification.Structure)
	}
}

// TestInspectAnOMPSession pins the inspection surface for the new source: the CLI reports the session's
// identity, agent and conversation shape without a target, over the fixture that carries OMP's own
// records (the title slot line and the harness's `custom` notices) beside the family's turns.
func TestInspectAnOMPSession(t *testing.T) {
	isolatedEnvironment(t)
	code, stdout, stderr := runCLI(t, "inspect", ompFixtureRef, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var inspection struct {
		Ref struct {
			Provider string `json:"provider"`
			ID       string `json:"id"`
		} `json:"ref"`
		Source struct {
			Identity struct {
				Agent string `json:"agent"`
				Cwd   string `json:"cwd"`
			} `json:"identity"`
			Counts struct {
				UserMessages      int `json:"user_messages"`
				AssistantMessages int `json:"assistant_messages"`
				ToolCalls         int `json:"tool_calls"`
				ToolResults       int `json:"tool_results"`
			} `json:"counts"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(stdout), &inspection); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if inspection.Ref.Provider != "omp" {
		t.Errorf("provider = %q, want omp", inspection.Ref.Provider)
	}
	if inspection.Source.Identity.Agent != "OhMyPi" {
		t.Errorf("agent = %q, want the registry's display name for the provider", inspection.Source.Identity.Agent)
	}
	if inspection.Source.Identity.Cwd != "/home/user/code/example-api" {
		t.Errorf("cwd = %q, want the fixture's recorded working directory", inspection.Source.Identity.Cwd)
	}
	// The fixture's own shape: the two turns and the third assistant answer, the call and its result --
	// and none of the harness's `custom` notices or its title slot read as a message.
	if counts := inspection.Source.Counts; counts.UserMessages != 2 || counts.AssistantMessages != 3 ||
		counts.ToolCalls != 1 || counts.ToolResults != 1 {
		t.Errorf("counts = %+v, want the fixture's own shape (2 user, 3 assistant, 1 call, 1 result)", counts)
	}
}

// TestTargetRefusalNamesOMP pins the refusal list: a caller who misspells the target harness is told which
// targets this build writes, and the new one is among them.
func TestTargetRefusalNamesOMP(t *testing.T) {
	isolatedEnvironment(t)
	code, _, stderr := runCLI(t, "continue", "claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10",
		"--to", "oh-my-pi", "--target-home", t.TempDir(), "--target-binary", missingTargetBinary)
	if code == exitOK {
		t.Fatalf("exit %d, want a refusal naming the targets this build writes", code)
	}
	if !strings.Contains(stderr, "omp") {
		t.Errorf("stderr = %q, want the OMP target named in the refusal", stderr)
	}
}
