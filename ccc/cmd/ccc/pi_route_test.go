package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// piFixtureRef is the committed Pi session, as the reference the CLI takes: the provider is Pi's
// registry name and the id is the one its header records.
const piFixtureRef = "pi:01a1c335-7098-71c2-a91b-b5295c0464f5"

// piShellUnsafeHome answers a Pi data directory whose path a POSIX shell would act on -- it holds a
// space -- so the printed resume line has to quote it on every host, for the same reason the Claude
// route's own helper does.
func piShellUnsafeHome(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "pi home")
}

// TestContinueToPiConvertsInstallsAndReportsTheRoute pins the Pi target through the CLI: one documented
// non-interactive invocation converts a supported source session into a new Pi session, the report
// distinguishes conversion, installation and verification, and the resume invocation it prints names the
// data directory and the working directory Pi resolves a session through.
func TestContinueToPiConvertsInstallsAndReportsTheRoute(t *testing.T) {
	isolatedEnvironment(t)
	home := piShellUnsafeHome(t)
	sourcePath := filepath.Join(fixtureRoot(t), "claude", "-home-user-code-example-api",
		"9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl")
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the source fixture: %v", err)
	}

	code, stdout, stderr := runCLI(t, "continue", "claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10",
		"--to", "pi", "--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Target != "pi" || document.TargetSession.Provider != "pi" {
		t.Errorf("target = %q / provider = %q, want the Pi route", document.Target, document.TargetSession.Provider)
	}
	if document.Conversion.State != "converted" || document.Conversion.Format != "pi-session-jsonl" {
		t.Errorf("conversion = %+v, want a converted Pi session", document.Conversion)
	}
	if document.Installation.State != "installed" {
		t.Fatalf("installation = %+v, want the session installed", document.Installation)
	}
	if filepath.Dir(document.TargetSession.Path) !=
		filepath.Join(home, "sessions", "--home-user-code-example-api--") {
		t.Errorf("installed under %s, want the project directory the working directory spells to",
			filepath.Dir(document.TargetSession.Path))
	}
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure = %+v, want the installed session read back as written",
			document.Verification.Structure)
	}
	// The report names the format's own limits rather than inventing a provider binding: a Pi session
	// records none for itself.
	limits := map[string]bool{}
	for _, limit := range document.TargetSession.FormatLimits {
		limits[limit.Category] = true
	}
	if !limits["model_provider_binding"] {
		t.Errorf("format limits = %v, want the provider-binding limit named", limits)
	}
	// The invocation carries the data directory, the session and the working directory.
	if document.TargetSession.ResumeEnvironment["PI_CODING_AGENT_DIR"] != home {
		t.Errorf("resume environment = %v, want the data directory named", document.TargetSession.ResumeEnvironment)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "cd /home/user/code/example-api") {
		t.Errorf("resume command = %q, want it to change into the session's working directory",
			document.TargetSession.ResumeCommand)
	}
	if !strings.Contains(document.TargetSession.ResumeCommand, "--session "+document.TargetSession.SessionID) {
		t.Errorf("resume command = %q, want the session named by id", document.TargetSession.ResumeCommand)
	}
	// The helper's home holds a space, so the renderer has to quote it: left bare, the pasted line would
	// run the harness with a data directory the shell split in two.
	if !strings.Contains(document.TargetSession.ResumeCommand, "PI_CODING_AGENT_DIR='"+home+"'") {
		t.Errorf("resume command = %q, want the data directory quoted, not left bare (the home holds a space)",
			document.TargetSession.ResumeCommand)
	}
	// The source is untouched, and the target home holds exactly the session that was installed: the home
	// is an empty directory this case owns, so a run that published anything else beside it is red here
	// rather than only in a later diff. The walk is over the whole sessions tree, because the install
	// files the session under the project directory the working directory spells.
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("re-reading the source fixture: %v", err)
	}
	if string(after) != string(sourceBefore) {
		t.Error("the conversion changed the source fixture")
	}
	sessions := []string{}
	walkErr := filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessions = append(sessions, entry.Name())
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("enumerating the target home: %v", walkErr)
	}
	if len(sessions) != 1 || sessions[0] != filepath.Base(document.TargetSession.Path) {
		t.Errorf("the home holds %v, want exactly the installed session %q",
			sessions, filepath.Base(document.TargetSession.Path))
	}
	installed, err := os.ReadFile(document.TargetSession.Path)
	if err != nil {
		t.Fatalf("reading the installed session: %v", err)
	}
	if len(installed) == 0 {
		t.Error("the installed session is empty")
	}
}

// TestContinueFromPiThroughTheCLI pins the source direction: the same command line continues a Pi session
// in a previously supported harness, and the report names the inherited Pi reader as the read's source.
func TestContinueFromPiThroughTheCLI(t *testing.T) {
	isolatedEnvironment(t)
	home := t.TempDir()
	code, stdout, stderr := runCLI(t, "continue", piFixtureRef, "--to", "claude",
		"--target-home", home, "--target-binary", missingTargetBinary, "--json")
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document struct {
		reportedContinueDocument
		Source struct {
			Identity struct {
				Provider string `json:"provider"`
			} `json:"identity"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Source.Identity.Provider != "pi" {
		t.Fatalf("source = %+v, want the Pi session reported as the source", document.Source)
	}
	if document.Conversion.State != "converted" || document.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", document.Conversion)
	}
	if document.Conversion.Carried.ToolCalls == 0 || document.Conversion.Carried.ToolResults == 0 {
		t.Errorf("carried = %+v, want the fixture's call and its result", document.Conversion.Carried)
	}
	if document.Verification.Structure.State != "verified" {
		t.Errorf("structure = %+v, want the installed Claude transcript read back as written",
			document.Verification.Structure)
	}
}

// TestInspectAPiSession pins the inspection surface for the new source: the CLI reports the session's
// identity, agent, working directory and conversation shape without a target.
func TestInspectAPiSession(t *testing.T) {
	isolatedEnvironment(t)
	code, stdout, stderr := runCLI(t, "inspect", piFixtureRef, "--json")
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
			} `json:"identity"`
			Counts struct {
				ToolCalls   int `json:"tool_calls"`
				ToolResults int `json:"tool_results"`
			} `json:"counts"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(stdout), &inspection); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if inspection.Ref.Provider != "pi" {
		t.Errorf("provider = %q, want pi", inspection.Ref.Provider)
	}
	if inspection.Source.Identity.Agent != "Pi" {
		t.Errorf("agent = %q, want the registry's display name for the provider", inspection.Source.Identity.Agent)
	}
	if inspection.Source.Counts.ToolCalls != 1 || inspection.Source.Counts.ToolResults != 1 {
		t.Errorf("counts = %+v, want the fixture's call and result", inspection.Source.Counts)
	}
}
