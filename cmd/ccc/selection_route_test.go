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
	// The excluded class's regions are the values themselves, spelled as the specification's own grammar:
	// the two records below the named one and the seven above it. A count alone cannot say which
	// ordinals were left behind, and a report that lost the regions would still pass one.
	if got := strings.Join(selection.Excluded.Regions, ","); got != "0-1,4-10" {
		t.Errorf("excluded regions = %q, want the nine excluded records spelled 0-1,4-10", got)
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
		"excluded:     9 record(s) at ordinals 0-1,4-10",
		"necessities:  1 record(s) added: 3 (pulled in by 2)",
		"undischarged: 0 carried call(s) with no recorded outcome at ordinals none",
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
			"a value supplied blank",
			[]string{"--select", ""},
			"the specification is empty",
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

// TestContinueValidatesASelectionBeforeTheSourceLeafGuard pins the order of the two refusals one Pi-family
// run can reach. A specification the read's own ordinals refuse is a command line that cannot be honoured:
// the usage exit code and the core's own sentence naming the space, never a statement about the source
// file's leaf. The fixture is the branch-switch session, whose file's last entry is a branch summary the
// read's own last record does not stand for -- so its whole read is refused by the source-side leaf guard
// -- and the case runs the same command line with and without the out-of-range specification: with it the
// answer is the selection refusal, without it the leaf error, which is what makes the case about the order
// of the two answers rather than about either check having stopped refusing.
func TestContinueValidatesASelectionBeforeTheSourceLeafGuard(t *testing.T) {
	isolatedEnvironment(t)
	home := t.TempDir()
	const branch = "pi:01a1c337-6d1e-7f84-9b2c-4e5f6a7b8c90"
	code, stdout, stderr := runCLI(t, "continue", branch, "--to", "pi", "--target-home", home,
		"--target-binary", missingTargetBinary, "--select", "99", "--json")
	if code != exitUsage {
		t.Errorf("exit %d, want %d (stdout %q, stderr %q)", code, exitUsage, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("the refused run wrote a report to stdout: %q", stdout)
	}
	for _, want := range []string{"99", "do not hold", "the read returned"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q does not hold %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "selects a leaf") {
		t.Errorf("the selection refusal answers with the source-leaf error: %q", stderr)
	}

	// The control arm: the same invocation without a selection reaches the source-side leaf guard and is
	// refused by it, with the conversion exit code (this fixture's read cannot be written as a Pi session
	// at all).
	code, _, stderr = runCLI(t, "continue", branch, "--to", "pi", "--target-home", home,
		"--target-binary", missingTargetBinary, "--json")
	if code != exitFailure {
		t.Errorf("the control run: exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if !strings.Contains(stderr, "selects a leaf") {
		t.Errorf("the control run's stderr %q does not hold the source-leaf refusal", stderr)
	}
	if files := targetTreeDigest(t, home); len(files) != 0 {
		t.Errorf("the refused runs left %d file(s) under the target home: %v", len(files), files)
	}
}

// TestContinueRefusesADuplicateRegionWithoutAReadableSource pins the duplicate-region refusal's own half
// of D5 through the CLI: `--select 1,1` is a property of the caller's command line and of nothing the
// read holds, so a source that cannot be read at all must not mask it as a source failure. The answer is
// the core's own sentence and the usage exit code, with nothing installed; the control arm -- the same
// missing source without the flag -- still reports the source failure, which is what makes this an
// ordering claim rather than a claim that either check stopped refusing.
func TestContinueRefusesADuplicateRegionWithoutAReadableSource(t *testing.T) {
	isolatedEnvironment(t)
	home := t.TempDir()
	const missing = "00000000-0000-4000-8000-000000000000"
	args := []string{"continue", "claude:" + missing, "--to", "codex", "--target-home", home,
		"--target-binary", missingTargetBinary}

	code, stdout, stderr := runCLI(t, append(append([]string(nil), args...), "--select", "1,1")...)
	if code != exitUsage {
		t.Errorf("exit %d, want %d (stdout %q, stderr %q)", code, exitUsage, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("the refused run wrote a report to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "already names") {
		t.Errorf("stderr %q does not name the duplicate region", stderr)
	}
	if strings.Contains(stderr, "session not found") {
		t.Errorf("the duplicate-region refusal was answered as a source failure: %q", stderr)
	}
	if files := targetTreeDigest(t, home); len(files) != 0 {
		t.Errorf("the refused run left %d file(s) under the target home: %v", len(files), files)
	}

	code, _, stderr = runCLI(t, args...)
	if code != exitFailure {
		t.Errorf("the control run: exit %d, want %d (stderr %q)", code, exitFailure, stderr)
	}
	if !strings.Contains(stderr, "session not found") {
		t.Errorf("the control run's stderr %q does not report the missing source", stderr)
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

	// A value the caller supplied but left blank is not the flag's absence: it is refused with the
	// core's sentence and the usage code, is pinned with an installation requested in
	// TestContinueRefusesABadSelectionBeforeInstalling, and never reaches the report -- an unset shell
	// variable must not turn `--select "$SPEC"` into the whole history.
	code, emptyStdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "", "--dry-run", "--json")...)
	if code != exitUsage {
		t.Fatalf("the empty selection: exit %d, want %d (stdout %q, stderr %q)", code, exitUsage,
			emptyStdout, stderr)
	}
	if !strings.Contains(stderr, "the specification is empty") {
		t.Errorf("the empty selection: stderr %q does not name the empty specification", stderr)
	}
	if emptyStdout != "" {
		t.Errorf("the empty selection wrote a report to stdout: %q", emptyStdout)
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

// TestContinueReportsUndischargedCallsOnBothSurfaces pins D2's nonzero arm: --select 5 carries the call
// the source never answered, and both report surfaces count it -- the --json selection block and the
// human block's undischarged line. Beside it, the target writer's own answer names what it did with the
// call: the OpenCode route's report counts it unpaired_tool_calls (its format has no state for a call
// with no outcome), which is the writer's report rather than the warning's claim.
func TestContinueReportsUndischargedCallsOnBothSurfaces(t *testing.T) {
	continueEnvironment(t)
	sessionID, _ := selectionFixture(t)

	code, stdout, stderr := runCLI(t, continueArgs(sessionID, "--select", "5", "--dry-run", "--json")...)
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
	if document.Selection.CallsUndischarged.Count != 1 ||
		strings.Join(document.Selection.CallsUndischarged.Regions, ",") != "5" {
		t.Errorf("calls_undischarged = %+v, want the one call the source never answered, at ordinal 5",
			document.Selection.CallsUndischarged)
	}
	if document.Conversion.Carried.ToolCalls != 1 || document.Conversion.Carried.ToolResults != 0 {
		t.Errorf("carried = %+v, want the call alone: no result was invented for it",
			document.Conversion.Carried)
	}

	// The human block says the same thing, and its warning defers to the writer instead of claiming the
	// target records a call it may have no shape for.
	code, human, stderr := runCLI(t, continueArgs(sessionID, "--select", "5", "--dry-run")...)
	if code != exitOK {
		t.Fatalf("human run: exit %d (stderr %q)", code, stderr)
	}
	for _, want := range []string{
		"undischarged: 1 carried call(s) with no recorded outcome at ordinals 5",
		"1 carried call(s) at ordinals 5 have no recorded result that states an outcome anywhere in the read",
		"the conversion's losses name what the writer left out",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("the human block does not hold %q:\n%s", want, human)
		}
	}

	// The OpenCode route's own report answers what became of the call: the writer leaves it out (its
	// format has no state for a call that has not finished) and counts it unpaired_tool_calls. The run
	// installs into a throw-away home through the stub binary the route's other cases use; the window
	// 5-10 keeps the later records too, because a target document holding no message at all is a shape
	// this route's own store cannot be read back from.
	home := openCodeHomeFor(t)
	binary := stubOpenCodeBinary(t)
	code, stdout, stderr = runCLI(t, "continue", "claude:"+sessionID, "--to", "opencode",
		"--target-home", home, "--target-binary", binary,
		"--select", "5-10", "--verify", "none", "--json")
	if code != exitOK {
		t.Fatalf("opencode run: exit %d, want %d (stderr %q)\n%s", code, exitOK, stderr, stdout)
	}
	var openCode reportedContinueDocument
	if err := json.Unmarshal([]byte(stdout), &openCode); err != nil {
		t.Fatalf("opencode stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if openCode.Selection == nil || openCode.Selection.CallsUndischarged.Count != 1 {
		t.Errorf("opencode selection = %+v, want the one undischarged call reported beside the target's "+
			"own answer", openCode.Selection)
	}
	found := false
	for _, loss := range openCode.Conversion.Losses {
		if loss.Category == "unpaired_tool_calls" {
			found = loss.Count == 1
		}
	}
	if !found {
		t.Errorf("the OpenCode route does not count the undischarged call under unpaired_tool_calls; "+
			"losses = %+v", openCode.Conversion.Losses)
	}
	if openCode.Conversion.Carried.ToolCalls != 1 {
		t.Errorf("the OpenCode route carried %d call(s), want the window's one answered call: the "+
			"undischarged call is the omission its report names", openCode.Conversion.Carried.ToolCalls)
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
		"  undischarged: 0 carried call(s) with no recorded outcome at ordinals none\n",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the gappy space's block does not hold %q:\n%s", want, block)
		}
	}
}
