package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/ccc"
)

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
		sum := sha256.Sum256(data)
		relative, _ := filepath.Rel(root, path)
		files[relative] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("hashing %s: %v", root, err)
	}
	return files
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestContinueWithASelectionCarriesTheSelectedRecordsAndReportsTheClasses pins the CLI half of the
// selection's report: one non-interactive invocation carries only what the specification named (plus the
// recorded counterpart of a separated call/result pair), and `--json` states the transformation in the
// four classes the goal names, with the ordinals the read supplied.
func TestContinueWithASelectionCarriesTheSelectedRecordsAndReportsTheClasses(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, sourcePath := selectionFixture(t)
	sourceBefore := fileDigest(t, sourcePath)

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "2", "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Selection == nil {
		t.Fatalf("the report carries no selection section:\n%s", stdout)
	}
	selection := document.Selection
	if selection.Spec != "2" || selection.Strict {
		t.Errorf("selection = %+v, want the specification 2 without the strict arm", selection)
	}
	// The fixture's own ordinals are 0..10 with no gap: the space is what a caller writes a spec against,
	// and the report states it rather than leaving a positional reading possible.
	if selection.OrdinalSpace.Min != 0 || selection.OrdinalSpace.Max != 10 ||
		selection.OrdinalSpace.Records != 11 || !selection.OrdinalSpace.Contiguous {
		t.Errorf("ordinal space = %+v, want the read's own values 0..10 with no gap", selection.OrdinalSpace)
	}
	if selection.Included.Count != 1 || strings.Join(selection.Included.Regions, ",") != "2" {
		t.Errorf("included = %+v, want the one record the specification named", selection.Included)
	}
	if selection.Excluded.Count != 9 {
		t.Errorf("excluded = %+v, want the nine records the selection left behind", selection.Excluded)
	}
	// The call recorded at ordinal 2 is answered by a result recorded at ordinal 3: the selection named
	// only the call, and the counterpart is carried and named with the ordinal that pulled it in.
	if selection.NecessitiesAdded.Count != 1 || len(selection.NecessitiesAdded.Added) != 1 ||
		selection.NecessitiesAdded.Added[0].Ordinal != 3 || selection.NecessitiesAdded.Added[0].PulledBy != 2 {
		t.Errorf("necessities_added = %+v, want the result at ordinal 3 pulled in by ordinal 2",
			selection.NecessitiesAdded)
	}
	if selection.CallsUndischarged.Count != 0 {
		t.Errorf("calls_undischarged = %+v, want none: the window holds no unanswered call", selection.CallsUndischarged)
	}
	// The counts reconcile with the conversion's own: the writer was handed the two carried records, and
	// what it holds for them is their one call/result pair. The conversion's message count is the target's
	// own vocabulary -- a record that carried only a tool call or only a result is not a message in either
	// format family -- so the class counts above, not this one, are what names the records carried.
	if document.Conversion.Carried.ToolCalls != 1 || document.Conversion.Carried.ToolResults != 1 {
		t.Errorf("carried = %+v, want the one call/result pair the selected records hold",
			document.Conversion.Carried)
	}
	if document.Source.Counts.Messages != selection.Included.Count+selection.NecessitiesAdded.Count+selection.Excluded.Count {
		t.Errorf("the read reports %d record(s) and the selection's classes account for %d",
			document.Source.Counts.Messages,
			selection.Included.Count+selection.NecessitiesAdded.Count+selection.Excluded.Count)
	}

	// The human block says the same thing without being asked for JSON, and it holds no conversation text:
	// the classes are counts and ordinals, which is what a report of a transformation owes a reader.
	code, human, stderr := runCLI(t, continueArgs(sessionID, "--select", "2", "--dry-run")...)
	if code != exitOK {
		t.Fatalf("human run: exit %d (stderr %q)", code, stderr)
	}
	for _, want := range []string{
		"selection:    2: the read's own ordinals are 0-10 (11 record(s))",
		"selected:     1 record(s) at ordinals 2",
		"excluded:     9 record(s) at ordinals",
		"necessities:  1 record(s) added: 3 (pulled in by 2)",
		"undischarged: 0 carried call(s) with no recorded result at ordinals none",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("the human block does not hold %q:\n%s", want, human)
		}
	}
	for _, needle := range []string{
		"Start the long build", "Starting the build now.", "The build finished.",
		"build-ok: 3 packages compiled", "And the cap?", "maxBackoffSeconds = 30",
		"internal/retry/config.go:41",
	} {
		if strings.Contains(human, needle) {
			t.Errorf("the selection block hands the caller the conversation's own words (%q):\n%s", needle, human)
		}
	}
	if !strings.Contains(human, "selection:") {
		t.Error("the human block does not print the selection it applied")
	}

	// The source the selection read is untouched, and a dry run wrote nothing into the target home.
	if after := fileDigest(t, sourcePath); after != sourceBefore {
		t.Errorf("the source fixture moved: %s -> %s", sourceBefore, after)
	}
	if files := targetTreeDigest(t, targetHome); len(files) != 0 {
		t.Errorf("the target home holds %d file(s) after a dry run: %v", len(files), files)
	}
}

// TestContinueRefusesABadSelectionBeforeInstalling pins D5 and D3's second arm through the CLI: every
// refusal is the core's own sentence with the usage exit code, and it happens before anything is
// installed -- the target data directory holds exactly what it held before, and the source transcript's
// bytes are unchanged.
func TestContinueRefusesABadSelectionBeforeInstalling(t *testing.T) {
	_, targetHome := continueEnvironment(t)
	sessionID, sourcePath := selectionFixture(t)
	sourceBefore := fileDigest(t, sourcePath)
	homeBefore := targetTreeDigest(t, targetHome)
	if len(homeBefore) != 0 {
		t.Fatalf("the target home starts non-empty: %v", homeBefore)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			"an inverted region",
			[]string{"--select", "5-3"},
			"runs backwards",
		},
		{
			"a non-numeric token",
			[]string{"--select", "two"},
			"is not an ordinal value",
		},
		{
			"a region named twice",
			[]string{"--select", "1-3,3"},
			"already names",
		},
		{
			"an ordinal the read does not hold",
			[]string{"--select", "42"},
			"do not hold",
		},
		{
			"the strict arm without a selection",
			[]string{"--strict-selection"},
			"--strict-selection",
		},
		{
			"a selection the strict arm refuses",
			[]string{"--select", "2", "--strict-selection"},
			"separates a recorded tool call from its recorded result",
		},
	}
	// The specification that selects nothing is not reachable from this fixture -- every ordinal of its
	// space 0..10 is present, so a well-formed spec that names no record would have to name a value in no
	// space, which the space check refuses separately. That arm is pinned by the Go case over a gappy read
	// (`TestSelectionMatchesTheOrdinalValuesTheReadSupplied`); the point of the cases here is that no
	// refusal writes anything, so each names an arm the CLI can reach.

	for _, testCase := range cases {
		args := continueArgs(sessionID, testCase.args...)
		code, stdout, stderr := runCLI(t, args...)
		if code != exitUsage {
			t.Errorf("%s: exit %d, want %d (stdout %q, stderr %q)", testCase.name, code, exitUsage, stdout, stderr)
		}
		if !strings.Contains(stderr, testCase.want) {
			t.Errorf("%s: stderr %q does not hold %q", testCase.name, stderr, testCase.want)
		}
		if stdout != "" {
			t.Errorf("%s: the refused run wrote a report to stdout: %q", testCase.name, stdout)
		}
		if files := targetTreeDigest(t, targetHome); len(files) != 0 {
			t.Errorf("%s: the refused run left %d file(s) under the target home: %v",
				testCase.name, len(files), files)
		}
		if after := fileDigest(t, sourcePath); after != sourceBefore {
			t.Errorf("%s: the source fixture moved: %s -> %s", testCase.name, sourceBefore, after)
		}
	}

	// The strict refusal happens with an installation requested, which is what makes "before anything is
	// installed" an assertion: the same invocation without the flag installs (a separate case pins that),
	// and with it the refusal leaves the home empty.
	code, _, stderr := runCLI(t, continueArgs(sessionID, "--select", "2", "--strict-selection")...)
	if code != exitUsage {
		t.Errorf("strict refusal on an installing run: exit %d, want %d (stderr %q)", code, exitUsage, stderr)
	}
	if files := targetTreeDigest(t, targetHome); len(files) != 0 {
		t.Errorf("the strict refusal installed %d file(s): %v", len(files), files)
	}
}

// TestContinueWithoutASelectionReportsNoSelection pins the default path's report shape: a run that names
// no selection carries the whole read and has no selection section at all, in the JSON and in the human
// block -- which is how a consumer tells "no transformation ran" from "a transformation carried
// everything". Beside it, the same run with a specification that names every one of the read's own records
// is asserted to convert exactly what the unflagged run converts: the flag's absence and a selection of
// everything hand the writer the same document.
func TestContinueWithoutASelectionReportsNoSelection(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := selectionFixture(t)

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("exit %d, want %d (stderr %q)", code, exitOK, stderr)
	}
	var document reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if document.Selection != nil {
		t.Errorf("the report carries a selection section without the flag: %+v", document.Selection)
	}
	if strings.Contains(stdout, `"selection"`) {
		t.Errorf("the JSON document holds a selection member without the flag:\n%s", stdout)
	}
	if document.Conversion.Carried.Messages == 0 {
		t.Errorf("the unflagged run carried no message at all: %+v", document.Conversion.Carried)
	}

	// The empty value is the flag's absence (Q-0009-03): `ccc continue … --select "$SPEC"` with an empty
	// SPEC is the run without the flag, so the same document comes back with no selection section.
	code, emptyStdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "", "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("the empty selection: exit %d (stderr %q)", code, stderr)
	}
	var empty reportedContinueDocument
	if err := json.Unmarshal([]byte(emptyStdout), &empty); err != nil {
		t.Fatalf("the empty selection is not one JSON object: %v\n%s", err, emptyStdout)
	}
	if empty.Selection != nil || strings.Contains(emptyStdout, `"selection"`) {
		t.Errorf("the empty selection reported a selection section:\n%s", emptyStdout)
	}
	if empty.Conversion.Carried != document.Conversion.Carried {
		t.Errorf("the empty selection carried %+v, want the unflagged run's %+v",
			empty.Conversion.Carried, document.Conversion.Carried)
	}

	code, human, stderr := runCLI(t, continueArgs(sessionID, "--dry-run")...)
	if code != exitOK {
		t.Fatalf("human run: exit %d (stderr %q)", code, stderr)
	}
	for _, unwanted := range []string{"selection:", "selected:", "excluded:", "necessities:", "undischarged:"} {
		if strings.Contains(human, unwanted) {
			t.Errorf("the human block prints %q without the flag:\n%s", unwanted, human)
		}
	}

	// A specification that names every record of the read carries the same document: the same carried
	// counts and the same loss rows as the unflagged run, with every record in the included class and none
	// added as a necessity.
	code, selectedStdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "0-10", "--dry-run", "--json")...)
	if code != exitOK {
		t.Fatalf("the whole-read selection: exit %d (stderr %q)", code, stderr)
	}
	var selected reportedContinueDocument
	if err := json.Unmarshal([]byte(selectedStdout), &selected); err != nil {
		t.Fatalf("the whole-read selection is not one JSON object: %v\n%s", err, selectedStdout)
	}
	if selected.Selection == nil {
		t.Fatal("the whole-read selection carries no selection section")
	}
	if selected.Selection.Included.Count != document.Source.Counts.Messages ||
		selected.Selection.Excluded.Count != 0 || selected.Selection.NecessitiesAdded.Count != 0 {
		t.Errorf("the whole-read selection = %+v, want every record included and none elsewhere",
			selected.Selection)
	}
	if selected.Conversion.Carried != document.Conversion.Carried ||
		len(selected.Conversion.Losses) != len(document.Conversion.Losses) {
		t.Errorf("the whole-read selection converted %+v with %d loss row(s), want the unflagged run's "+
			"%+v with %d", selected.Conversion.Carried, len(selected.Conversion.Losses),
			document.Conversion.Carried, len(document.Conversion.Losses))
	}
	if selected.Verification.Structure.State != document.Verification.Structure.State {
		t.Errorf("the whole-read selection's structural read-back is %q, want the unflagged run's %q",
			selected.Verification.Structure.State, document.Verification.Structure.State)
	}
}

// TestWriteSelectionSpellsAGappySpace pins the human selection line's non-contiguous arm, which no CLI
// case reaches: the one committed fixture's space has no gap, so the suffix that states one has to be
// exercised over a report of its own. The space is spelled once as its bounds; the suffix states the gap
// without repeating them, and the classes render as they do for a contiguous space.
func TestWriteSelectionSpellsAGappySpace(t *testing.T) {
	var out bytes.Buffer
	writeSelection(&out, &ccc.SelectionReport{
		Spec:     "1,5",
		Space:    ccc.OrdinalSpace{Min: 1, Max: 5, Records: 3, Contiguous: false},
		Included: ccc.SelectionClass{Count: 2, Regions: []string{"1", "5"}},
		Excluded: ccc.SelectionClass{Count: 1, Regions: []string{"3"}},
	})
	block := out.String()
	for _, want := range []string{
		"  selection:    1,5: the read's own ordinals are 1-5 (3 record(s)) (the read's own values are not contiguous)\n",
		"  selected:     2 record(s) at ordinals 1,5\n",
		"  excluded:     1 record(s) at ordinals 3\n",
		"  necessities:  0 record(s) added: none\n",
		"  undischarged: 0 carried call(s) with no recorded result at ordinals none\n",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the gappy space's block does not hold %q:\n%s", want, block)
		}
	}
}
