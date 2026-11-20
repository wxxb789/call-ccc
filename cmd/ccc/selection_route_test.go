package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// sha256Hex answers one byte string's SHA-256 in hex, for the tree digests the refusal cases compare.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fileDigest answers one file's SHA-256, failing when it cannot be read.
func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return sha256Hex(data)
}

// selectionFixtureProject is the committed Claude transcript the selection cases read: the one fixture
// whose shape carries a call whose result sits in the next record, a call the source never answered, a
// compaction summary and a tool result on a system record. The session id is read out of the fixture tree
// rather than spelled here, so the case cannot drift from the fixture it names.
const selectionFixtureProject = "-home-user-code-awaiting"

// selectionFixture reads the selection fixture's session id and path out of the committed tree.
func selectionFixture(t *testing.T) (sessionID, path string) {
	t.Helper()
	fixture, ok := claudeFixtures(t)[selectionFixtureProject]
	if !ok {
		t.Fatalf("the selection fixture (%s) is not below the fixture root", selectionFixtureProject)
	}
	return fixture.sessionID, fixture.path
}

// targetTreeDigest answers a digest of every file below a directory, keyed by its path relative to it, so
// a case can say that a refused run wrote nothing at all rather than only that one expected file is
// absent. A directory that does not exist hashes to the empty map.
func targetTreeDigest(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256Hex(data)
		relative, _ := filepath.Rel(root, path)
		files[relative] = sum
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("hashing %s: %v", root, err)
	}
	return files
}

// reportedSelectionDocument decodes the selection section of the CLI's own JSON document: the cases read
// the CLI's output into their own declaration rather than the production type, so a field the CLI renames
// or drops is a failing case and not a silent zero.
type reportedSelectionDocument struct {
	Selection *struct {
		Spec   string `json:"spec"`
		Strict bool   `json:"strict"`
		Space  struct {
			Min        int  `json:"min"`
			Max        int  `json:"max"`
			Records    int  `json:"records"`
			Contiguous bool `json:"contiguous"`
		} `json:"ordinal_space"`
		Included struct {
			Count   int      `json:"count"`
			Regions []string `json:"regions"`
		} `json:"included"`
		Excluded struct {
			Count   int      `json:"count"`
			Regions []string `json:"regions"`
		} `json:"excluded"`
		NecessitiesAdded struct {
			Count int `json:"count"`
			Added []struct {
				Ordinal  int `json:"ordinal"`
				PulledBy int `json:"pulled_by"`
			} `json:"added"`
		} `json:"necessities_added"`
		CallsUndischarged struct {
			Count   int      `json:"count"`
			Regions []string `json:"regions"`
		} `json:"calls_undischarged"`
		Records []struct {
			Ordinal   int      `json:"ordinal"`
			Necessity bool     `json:"necessity"`
			PulledBy  *int     `json:"pulled_by"`
			Became    string   `json:"became"`
			Reason    string   `json:"reason"`
			Written   []string `json:"written"`
		} `json:"records"`
	} `json:"selection"`
}

// TestContinueWithASelectionReportsTheClassesAndFates pins the report's half of the selection: the JSON
// carries the space the specification was written against, the four classes with their counts and the
// ordinals they sit at, and D1's per-record table -- every carried record with what became of it and the
// item kinds the target wrote for it, the necessity marked with the ordinal that pulled it in.
func TestContinueWithASelectionReportsTheClassesAndFates(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, sourcePath := selectionFixture(t)
	sourceBefore := fileDigest(t, sourcePath)

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "2", "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedSelectionDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	selection := document.Selection
	if selection == nil {
		t.Fatalf("the report carries no selection section:\n%s", stdout)
	}
	if selection.Spec != "2" || selection.Strict {
		t.Errorf("selection = %+v, want the specification 2 without the strict arm", selection)
	}
	space := selection.Space
	if space.Min != 0 || space.Max != 10 || space.Records != 11 || !space.Contiguous {
		t.Errorf("ordinal space = %+v, want the read's own values 0..10 with no gap", space)
	}
	if selection.Included.Count != 1 || strings.Join(selection.Included.Regions, ",") != "2" {
		t.Errorf("included = %+v, want the one record the specification named", selection.Included)
	}
	if selection.Excluded.Count != 9 || strings.Join(selection.Excluded.Regions, ",") != "0-1,4-10" {
		t.Errorf("excluded = %+v, want the nine records the selection left behind, 0-1,4-10",
			selection.Excluded)
	}
	if selection.NecessitiesAdded.Count != 1 || len(selection.NecessitiesAdded.Added) != 1 ||
		selection.NecessitiesAdded.Added[0].Ordinal != 3 ||
		selection.NecessitiesAdded.Added[0].PulledBy != 2 {
		t.Errorf("necessities_added = %+v, want the result at ordinal 3 pulled in by ordinal 2",
			selection.NecessitiesAdded)
	}
	if selection.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the window holds no unanswered call",
			selection.CallsUndischarged)
	}
	if len(selection.Records) != 2 {
		t.Fatalf("records = %+v, want the pair's two records accounted for", selection.Records)
	}
	if selection.Records[0].Ordinal != 2 || selection.Records[0].Became != "native_history" ||
		!slices.Equal(selection.Records[0].Written, []string{"tool_call"}) {
		t.Errorf("record 2 = %+v, want it written as the call", selection.Records[0])
	}
	if selection.Records[1].Ordinal != 3 || !selection.Records[1].Necessity ||
		selection.Records[1].PulledBy == nil || *selection.Records[1].PulledBy != 2 ||
		selection.Records[1].Became != "native_history" ||
		!slices.Equal(selection.Records[1].Written, []string{"tool_result"}) {
		t.Errorf("record 3 = %+v, want the necessity written as the result", selection.Records[1])
	}
	// A dry run installs nothing, and the source's own bytes are the same file afterwards.
	if digest := fileDigest(t, sourcePath); digest != sourceBefore {
		t.Errorf("the source file moved during the conversion: %s -> %s", sourceBefore, digest)
	}
	if files := targetTreeDigest(t, targetHome); len(files) != 0 {
		t.Errorf("a dry run wrote %d file(s) under the target home: %v", len(files), files)
	}
}

// TestContinueWithASelectionInstallsExactlyTheWindow pins the target half: a real conversion with a
// selection installs a rollout holding exactly the carried records -- the kept dialogue and tool evidence
// present, the excluded ordinals' own words absent -- and the human report names the transformation and
// the per-record fates.
func TestContinueWithASelectionInstallsExactlyTheWindow(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, _ := selectionFixture(t)

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "0-4")...)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	for _, want := range []string{
		"  selection:    0-4; ordinal space 0..10: 11 record(s)",
		"  included:     5 record(s) at 0-4",
		"  excluded:     6 record(s) at 5-10",
		"  necessities:  none added",
		"  record 0       native history (user_message)",
		"  record 2       native history (tool_call)",
		"  record 3       native history (tool_result)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human report does not hold %q:\n%s", want, stdout)
		}
	}
	installed := onlyFileBelow(t, filepath.Join(targetHome, "sessions"))
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("reading the installed rollout: %v", err)
	}
	for _, kept := range []string{
		"Start the long build, then tell me where the retry cap lives.",
		"build-ok: 3 packages compiled",
		"The build finished. Reading the retry configuration.",
	} {
		if !strings.Contains(string(data), kept) {
			t.Errorf("the installed rollout does not hold the carried record %q:\n%s", kept, data)
		}
	}
	for _, excluded := range []string{
		"And the cap?",
		"the workspace state was restored",
		"Summary: the build succeeded and the retry configuration is being read.",
		"internal/retry/config.go:41: maxBackoffSeconds = 30",
	} {
		if strings.Contains(string(data), excluded) {
			t.Errorf("the installed rollout holds %q from a record the selection excluded", excluded)
		}
	}
	// The human report's non-zero classes, on windows whose classes are not empty: ordinal 2 pulls its
	// counterpart in as a necessity and the necessity's own record line says so; ordinal 5 is the call
	// the source never answered, which the report names as undischarged rather than as a lost record.
	code, stdout, stderr = runCLI(t, continueArgs(sessionID, "--select", "2", "--dry-run")...)
	if code != exitOK {
		t.Fatalf("selecting 2: exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	for _, want := range []string{
		"  included:     1 record(s) at 2",
		"  excluded:     9 record(s) at 0-1, 4-10",
		"  necessities:  1 added: 3 (pulled by 2)",
		"  undischarged: 0 call(s) at none",
		"  record 3       native history (tool_result) [necessity, pulled by 2]",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human report of --select 2 does not hold %q:\n%s", want, stdout)
		}
	}
	code, stdout, stderr = runCLI(t, continueArgs(sessionID, "--select", "5", "--dry-run")...)
	if code != exitOK {
		t.Fatalf("selecting 5: exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	for _, want := range []string{
		"  undischarged: 1 call(s) at 5",
		"  record 5       native history (tool_call)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human report of --select 5 does not hold %q:\n%s", want, stdout)
		}
	}
}

// onlyFileBelow answers the one file below a directory, failing when there is not exactly one: the cases
// that read an installed session know the conversion wrote one rollout, and a second file would mean the
// installation did something the case is not asserting about.
func onlyFileBelow(t *testing.T, root string) string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(found) != 1 {
		t.Fatalf("%s holds %d file(s), want exactly one: %v", root, len(found), found)
	}
	return found[0]
}

// TestContinueRefusesABadSelectionWithTheUsageExitCode pins D5's exit-code rule at the CLI: every refusal
// -- malformed syntax, an inverted range, a duplicated ordinal, an ordinal outside the read's own space,
// a specification that names nothing, and the strict refusal of a separated pair -- answers with the
// usage exit code, the core's own sentence on stderr, and no installation at all. It also pins that the
// flag's presence is what makes a blank value a refusal rather than the flag's absence.
func TestContinueRefusesABadSelectionWithTheUsageExitCode(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, sourcePath := selectionFixture(t)
	sourceBefore := fileDigest(t, sourcePath)
	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "malformed", args: []string{"--select", "x"}, want: `"x" is not an ordinal`},
		{name: "inverted", args: []string{"--select", "5-3"}, want: "inverted"},
		{name: "duplicated", args: []string{"--select", "1-3,3"}, want: "already names"},
		{name: "outside the space", args: []string{"--select", "11"},
			want: "outside the read's own ordinal values"},
		{name: "blank but supplied", args: []string{"--select", ""},
			want: "the specification is empty"},
		{name: "strict pair", args: []string{"--select", "2", "--strict-selection"},
			want: "separates a recorded tool call from its recorded result"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, continueArgs(sessionID, testCase.args...)...)
			if code != exitUsage {
				t.Errorf("exit %d, want %d (stdout %q, stderr %q)", code, exitUsage, stdout, stderr)
			}
			if !strings.Contains(stderr, testCase.want) {
				t.Errorf("stderr = %q, want it to name %q", stderr, testCase.want)
			}
			if stdout != "" {
				t.Errorf("a refused run wrote a report: %q", stdout)
			}
			if files := targetTreeDigest(t, targetHome); len(files) != 0 {
				t.Errorf("a refused run wrote %d file(s) under the target home: %v", len(files), files)
			}
		})
	}
	if digest := fileDigest(t, sourcePath); digest != sourceBefore {
		t.Errorf("a refused run moved the source file: %s -> %s", sourceBefore, digest)
	}
}

// TestSelectionIsRefusedOnEveryOtherPairBeforeTheSourceIsOpened pins D7 at the CLI and the "zero reader
// calls" arm the plan asks for: on every pair but the reference one, the flag is refused by name before
// the source is opened, so the run's sentence is the refusal and never a read failure -- the sources here
// do not resolve at all, and a run that opened one would say so -- and nothing is installed.
func TestSelectionIsRefusedOnEveryOtherPairBeforeTheSourceIsOpened(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, _ := selectionFixture(t)
	// The Claude reference cannot resolve: the root below is empty, so a read through it fails before
	// any conversion, and an error that is not a read failure is an answer the run gave without reading.
	t.Setenv("CLAUDE_PROJECTS_DIR", t.TempDir())
	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "a supported target with a non-Claude source",
			args: []string{"continue", "codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812", "--to", "codex",
				"--select", "0-", "--target-binary", missingTargetBinary}},
		{name: "the Claude source with another supported target",
			args: []string{"continue", "claude:" + sessionID, "--to", "pi",
				"--select", "0-", "--target-binary", missingTargetBinary}},
		{name: "the strict arm on another pair",
			args: []string{"continue", "claude:" + sessionID, "--to", "claude",
				"--select", "0-", "--strict-selection", "--target-binary", missingTargetBinary}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, testCase.args...)
			if code != exitUsage {
				t.Errorf("exit %d, want %d (stdout %q, stderr %q)", code, exitUsage, stdout, stderr)
			}
			if !strings.Contains(stderr, "a selection is honoured on one pair only") {
				t.Errorf("stderr = %q, want the pair refusal", stderr)
			}
			// The read the run would have made fails (the root is empty); the refusal is not that
			// failure, which is what makes this arm about the source never being opened.
			if strings.Contains(stderr, "not found") || strings.Contains(stderr, "no such") {
				t.Errorf("stderr reads as a source-read failure: %q", stderr)
			}
			if files := targetTreeDigest(t, targetHome); len(files) != 0 {
				t.Errorf("a refused run wrote %d file(s) under the target home: %v", len(files), files)
			}
		})
	}
	// The control arm: the reference pair accepts the same specification, so the gate refuses the *pair*,
	// never the flag everywhere. The fixture root comes back, because a refusal that read nothing has to
	// be told apart from one that read nothing *successfully*.
	t.Setenv("CLAUDE_PROJECTS_DIR", filepath.Join(fixtureRoot(t), "claude"))
	code, _, stderr := runCLI(t, continueArgs(sessionID, "--select", "0-", "--dry-run")...)
	if code != exitOK {
		t.Errorf("the reference pair with the same specification: exit %d, want %d (stderr %q)",
			code, exitOK, stderr)
	}
}

// TestTheUnflaggedRunKeepsItsReportWithoutASelectionSection pins D4's report half at the CLI: a run
// without the flag carries no selection object at all, so a consumer of the unflagged document reads
// exactly what it read before the option existed.
func TestTheUnflaggedRunKeepsItsReportWithoutASelectionSection(t *testing.T) {
	_, _ = continueEnvironment(t)
	sessionID, _ := selectionFixture(t)
	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if _, ok := raw["selection"]; ok {
		t.Errorf("the unflagged report carries a selection section:\n%s", stdout)
	}
	if !strings.Contains(stdout, `"conversion"`) {
		t.Errorf("the unflagged report is missing its conversion block:\n%s", stdout)
	}
}
