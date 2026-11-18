package ccc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// openCodeUnrelatedSessionID is the id of the session a target home already holds when the route is
// exercised: the conversion must leave it in place.
const openCodeUnrelatedSessionID = "ses_zzunrelated0000000000000000"

// openCodeTargetHome answers a data directory whose store already holds one unrelated session, and the
// bytes that session's own rows are: what a conversion must not touch.
func openCodeTargetHome(t *testing.T) (string, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	options := openCodeTargetTestOptions()
	options.SessionID = openCodeUnrelatedSessionID
	unrelated, err := ConvertToOpenCodeSession(openCodeTestConversation(
		Message{Role: "user", Dialogue: "an unrelated session", DialogueProjected: true,
			Content: "an unrelated session", Timestamp: openCodeTargetTestOptions().Now},
	), options)
	if err != nil {
		t.Fatalf("building the unrelated session: %v", err)
	}
	writeOpenCodeTestStore(t, openCodeDataDir(home), unrelated)
	return home, openCodeStoreRows(t, openCodeStorePath(home), openCodeUnrelatedSessionID)
}

// openCodeStoreRows answers one session's own rows out of a store as text, so a case can show that a
// conversion left a session it did not install exactly as it was.
func openCodeStoreRows(t *testing.T, storePath, sessionID string) string {
	t.Helper()
	database, err := sql.Open("sqlite3", storePath)
	if err != nil {
		t.Fatalf("opening %s: %v", storePath, err)
	}
	defer database.Close()
	rows := map[string][]map[string]any{}
	sessionRows, err := database.Query(`SELECT id, project_id, title, directory, time_created, time_updated
		FROM session_v2 WHERE id = ?`, sessionID)
	if err != nil {
		t.Fatalf("reading the session row: %v", err)
	}
	for sessionRows.Next() {
		var id, projectID, title, directory string
		var created, updated int64
		if err := sessionRows.Scan(&id, &projectID, &title, &directory, &created, &updated); err != nil {
			t.Fatalf("scanning the session row: %v", err)
		}
		rows["session"] = append(rows["session"], map[string]any{
			"id": id, "project_id": projectID, "title": title, "directory": directory,
			"time_created": created, "time_updated": updated,
		})
	}
	sessionRows.Close()
	messageRows, err := database.Query(`SELECT id, type, seq, data FROM session_message
		WHERE session_id = ? ORDER BY seq`, sessionID)
	if err != nil {
		t.Fatalf("reading the messages: %v", err)
	}
	for messageRows.Next() {
		var id, kind, data string
		var seq int64
		if err := messageRows.Scan(&id, &kind, &seq, &data); err != nil {
			t.Fatalf("scanning a message: %v", err)
		}
		rows["messages"] = append(rows["messages"], map[string]any{
			"id": id, "type": kind, "seq": seq, "data": data,
		})
	}
	messageRows.Close()
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("encoding the rows: %v", err)
	}
	return string(encoded)
}

// stubOpenCodeImporterRunning runs the harness's own importer for the route: the exclusivity question is
// answered with the harness's own refusal, and the import stages the document the route handed over into
// the store -- the same document, read from the file the route wrote and written through the fixture
// writer, so the structure check reads back what the route actually staged. The store it writes into is
// the one the environment names, exactly as the harness's own importer resolves its data directory, so
// the same stub answers an installed run and the dry run's throw-away copy.
func stubOpenCodeImporterRunning(t *testing.T) *[][]string {
	t.Helper()
	var invocations [][]string
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		invocations = append(invocations, argv)
		if len(argv) > 1 && argv[0] == "session" && argv[1] == "import" {
			data, err := os.ReadFile(argv[2])
			if err != nil {
				return "", "", err
			}
			// The importer is handed the document the route staged, and the store the fixture writer
			// builds holds exactly those bytes: the structure check then reads the route's own document
			// back, not a case's copy of it.
			var document openCodeDocument
			if err := json.Unmarshal(data, &document); err != nil {
				return "", "", err
			}
			writeOpenCodeTestStore(t, filepath.Join(openCodeDataRootOf(env), openCodeDataDirName),
				OpenCodeSession{Data: data})
			return "Imported session: " + document.Info.ID + "\n", "", nil
		}
		return "", "Session not found: " + argv[2] + "\n", errors.New("exit status 1")
	})
	return &invocations
}

// openCodeDataRootOf answers the data root an invocation's own environment names, which is where the
// harness's importer resolves the store it writes into.
func openCodeDataRootOf(environment []string) string {
	for _, entry := range environment {
		if name, value, ok := strings.Cut(entry, "="); ok && strings.EqualFold(name, openCodeDataRootEnv) {
			return value
		}
	}
	return ""
}

// TestContinueIntoOpenCodeConvertsInstallsAndReadsBack is the route's central property test: one call
// turns a real Claude Code session into a session of the target harness through that harness's own
// importer, leaves the source and an unrelated session in the target store untouched, reads the installed
// session back through the inherited OpenCode reader as what the writer placed in it, and reports the
// format's own limits and losses.
func TestContinueIntoOpenCodeConvertsInstallsAndReadsBack(t *testing.T) {
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)
	home, unrelatedBefore := openCodeTargetHome(t)
	invocations := stubOpenCodeImporterRunning(t)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetOpenCode,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyLoad,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}

	if result.Target != TargetOpenCode {
		t.Errorf("target = %q, want %q", result.Target, TargetOpenCode)
	}
	if result.Conversion.State != ConversionStateConverted || result.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", result.Conversion)
	}
	if result.Conversion.Format != OpenCodeTargetFormat {
		t.Errorf("format = %q, want %q", result.Conversion.Format, OpenCodeTargetFormat)
	}
	if result.TargetSession.Provider != TargetOpenCode {
		t.Errorf("provider = %q, want %q", result.TargetSession.Provider, TargetOpenCode)
	}
	if result.Installation.State != InstallationStateInstalled {
		t.Fatalf("installation = %+v, want the session installed", result.Installation)
	}
	if result.TargetSession.Path != openCodeStorePath(home) {
		t.Errorf("installed path = %q, want the store the importer writes into (%q)",
			result.TargetSession.Path, openCodeStorePath(home))
	}
	if !strings.Contains(result.Installation.Mechanism, "opencode session import") {
		t.Errorf("mechanism = %q, want the harness's own importer named", result.Installation.Mechanism)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed session read back as what was written",
			result.Verification.Structure)
	}
	if result.Verification.Structure.ReadBack.ToolCalls != 1 ||
		result.Verification.Structure.ReadBack.ToolResults != 1 ||
		result.Verification.Structure.ReadBack.CompactionBoundaries != 1 {
		t.Errorf("read-back = %+v, want the fixture's own call, result and boundary",
			result.Verification.Structure.ReadBack)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q, want the source unchanged", result.Installation.SourceCheckState)
	}
	if hashFile(t, sourcePath) != sourceBefore {
		t.Error("the conversion changed the source session")
	}
	if after := openCodeStoreRows(t, openCodeStorePath(home), openCodeUnrelatedSessionID); after != unrelatedBefore {
		t.Error("the conversion changed a session it did not install")
	}
	// The id the report names is the one the harness's own store holds, and no other session's.
	count := 0
	if err := queryOpenCodeInt(t, openCodeStorePath(home),
		`SELECT COUNT(*) FROM session_v2`, &count); err != nil {
		t.Fatalf("counting the store's sessions: %v", err)
	}
	if count != 2 {
		t.Errorf("the store holds %d sessions, want the unrelated one and this conversion's", count)
	}

	// The report lists the format's own limits and the losses this pair of formats has, including the
	// two the target home's own answers decide.
	limits := map[string]bool{}
	for _, limit := range result.TargetSession.FormatLimits {
		limits[limit.Category] = true
	}
	for _, want := range []string{"session_container", "provider_and_model", "session_title_and_outcome"} {
		if !limits[want] {
			t.Errorf("format limits %v do not name %s", limits, want)
		}
	}
	losses := map[string]Loss{}
	for _, loss := range result.Conversion.Losses {
		losses[loss.Category] = loss
	}
	for _, want := range []string{"usage_zeroed", "target_model_id_attribution", "compaction_reason",
		"target_model_binding", "message_model_attribution", "pre_compaction_history"} {
		if _, ok := losses[want]; !ok {
			t.Errorf("the conversion report carries no %s entry", want)
		}
	}
	// The target home's own answers are what the document attributes, and the report says where they came
	// from: the probe was pointed at a binary that is not installed here, so the run recorded nothing and
	// says so rather than inventing a provider.
	if detail := losses["target_model_binding"].Detail; !strings.Contains(detail, missingTargetBinary) {
		t.Errorf("target_model_binding detail = %q, want the harness that answered nothing named (%s is not "+
			"installed here)", detail, missingTargetBinary)
	}

	// The invocation: the harness's own non-interactive resume, the data root the harness reads, and the
	// session's own recorded directory -- and the importer was handed exactly one staged document.
	if result.TargetSession.ResumeEnvironment[openCodeDataRootEnv] != openCodeDataRoot(home) {
		t.Errorf("resume environment = %v, want the data root the harness reads named",
			result.TargetSession.ResumeEnvironment)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand,
		openCodeDataRootEnv+"="+shellQuote(openCodeDataRoot(home))) {
		t.Errorf("resume command = %q, want the data root named", result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand,
		"run --standalone --session "+result.TargetSession.SessionID) {
		t.Errorf("resume command = %q, want the harness's own resume named by id",
			result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, "cd /home/user/code/example-api") {
		t.Errorf("resume command = %q, want it to change into the session's recorded directory",
			result.TargetSession.ResumeCommand)
	}
	if len(*invocations) != 2 {
		t.Errorf("the importer was run %d time(s) as %q, want the exclusivity question and one import",
			len(*invocations), *invocations)
	}
	if result.Verification.Structure.Method != structureMethodOpenCode {
		t.Errorf("structure method = %q, want the OpenCode reader's own sentence",
			result.Verification.Structure.Method)
	}
	// The native half is not claimed: the harness this run was pointed at is not installed, so the load
	// check reports what it could not run rather than a pass.
	if result.Verification.Load.State == VerificationStateVerified {
		t.Errorf("load = %+v, want it not verified: %s is not installed here",
			result.Verification.Load, missingTargetBinary)
	}
}

// queryOpenCodeInt answers one integer out of a store.
func queryOpenCodeInt(t *testing.T, storePath, query string, into *int) error {
	t.Helper()
	database, err := sql.Open("sqlite3", storePath)
	if err != nil {
		return err
	}
	defer database.Close()
	return database.QueryRow(query).Scan(into)
}

// TestContinueIntoOpenCodeDryRunAsksNothingAndInstallsNothing pins the dry run: the caller's data
// directory is untouched (a dry run checks the conversion against a throw-away copy of the same bytes,
// and for this target the copy is materialised by the harness's own importer, which is what the check
// reads back), the facts that depend on the harness's own answer are named as unasked rather than left
// to be read from an empty record, and nothing is reported as installed.
func TestContinueIntoOpenCodeDryRunAsksNothingAndInstallsNothing(t *testing.T) {
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	agents := 0
	var roots []string
	stubOpenCodeExecutable(t, func(string) (string, error) { return "/stub/opencode", nil })
	stubOpenCodeRun(t, &runOpenCodeAgentsProbe, func(
		_ context.Context, _ string, _ string, _ []string, _ []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		agents++
		return `[{"id":"build","mode":"primary","hidden":false,"model":{"id":"m","providerID":"p"}}]`, "", nil
	})
	stubOpenCodeRun(t, &runOpenCodeImporter, func(
		_ context.Context, _ string, _ string, argv []string, env []string, _ time.Duration, _ io.Writer,
	) (string, string, error) {
		roots = append(roots, openCodeDataRootOf(env))
		if len(argv) > 1 && argv[0] == "session" && argv[1] == "import" {
			data, err := os.ReadFile(argv[2])
			if err != nil {
				return "", "", err
			}
			var document openCodeDocument
			if err := json.Unmarshal(data, &document); err != nil {
				return "", "", err
			}
			writeOpenCodeTestStore(t, filepath.Join(openCodeDataRootOf(env), openCodeDataDirName),
				OpenCodeSession{Data: data})
			return "Imported session: " + document.Info.ID + "\n", "", nil
		}
		return "", "Session not found: " + argv[2] + "\n", errors.New("exit status 1")
	})
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetOpenCode,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyLoad,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("a dry run failed: %v", err)
	}
	if agents != 0 {
		t.Errorf("the dry run asked the harness for its own agent list %d time(s): a preview starts nothing "+
			"that a conversion does not need", agents)
	}
	if _, err := os.Stat(openCodeStorePath(home)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the dry run left a store in the caller's data directory (%s)", openCodeStorePath(home))
	}
	for _, root := range roots {
		if root == openCodeDataRoot(home) || root == "" {
			t.Errorf("the dry run's own importer ran against %q, want a throw-away root", root)
		}
		if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the dry run left its throw-away data root %s behind", root)
		}
	}
	if result.Installation.State != InstallationStateNotInstalled {
		t.Errorf("installation = %+v, want nothing installed", result.Installation)
	}
	if result.Conversion.Format != OpenCodeTargetFormat {
		t.Errorf("format = %q, want the conversion still described for the dry run", result.Conversion.Format)
	}
	// The throw-away copy *is* read back and does verify; what makes it a dry run is where the copy was
	// materialised and that the check says so.
	if result.Verification.Structure.State != VerificationStateVerified ||
		result.Verification.Structure.Method != structureMethodOpenCodeDryRun {
		t.Errorf("structure = %+v, want the throw-away copy read back through the reader (%q)",
			result.Verification.Structure, structureMethodOpenCodeDryRun)
	}
	if result.Verification.Structure.TargetPath != "" {
		t.Errorf("structure target path = %q, want none: the copy is removed and is not the session's home",
			result.Verification.Structure.TargetPath)
	}
	losses := map[string]Loss{}
	for _, loss := range result.Conversion.Losses {
		losses[loss.Category] = loss
	}
	if detail := losses["target_model_binding"].Detail; !strings.Contains(detail, "dry run") {
		t.Errorf("target_model_binding detail = %q, want the dry run named as why the harness answered "+
			"nothing", detail)
	}
	// The agent member is required by the importer and the harness's own default was measured, so the
	// document names it -- and the report says it was not asked rather than answered.
	if detail := losses["target_agent"].Detail; !strings.Contains(detail, "not be asked") {
		t.Errorf("target_agent detail = %q, want the unasked agent named", detail)
	}
	if result.Verification.Load.State != VerificationStateNotVerified ||
		!strings.Contains(result.Verification.Load.Detail, "dry run") {
		t.Errorf("load = %+v, want the dry run named as why nothing could be loaded", result.Verification.Load)
	}
}

// TestContinueIntoOpenCodeWithoutTheHarnessNamesTheBoundary pins what a run that has no harness to hand
// the document to reports: this target's installation is the harness's own importer, so a missing binary
// is the boundary -- reported, not worked around with a writer of this build's own.
func TestContinueIntoOpenCodeWithoutTheHarnessNamesTheBoundary(t *testing.T) {
	home := filepath.Join(t.TempDir(), openCodeDataDirName)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetOpenCode,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err == nil {
		t.Fatal("a run with no harness to import through succeeded")
	}
	if !strings.Contains(err.Error(), missingTargetBinary) ||
		!strings.Contains(err.Error(), "importer") {
		t.Errorf("refusal = %v, want the missing harness and the importer named", err)
	}
	if result.Installation.State == InstallationStateInstalled {
		t.Errorf("installation = %+v, want nothing reported as installed", result.Installation)
	}
	if _, statErr := os.Stat(openCodeStorePath(home)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the run left a store at %s", openCodeStorePath(home))
	}
}

// TestOpenCodeRouteIsRegistered pins what the route table answers: the OpenCode target is one this build
// writes, its route names the harness's own binary and data directory, and an unknown target's refusal
// lists it.
func TestOpenCodeRouteIsRegistered(t *testing.T) {
	if !slices.Contains(Targets(), TargetOpenCode) {
		t.Errorf("Targets() = %v, want the OpenCode target among them", Targets())
	}
	route, err := targetRouteFor(TargetOpenCode)
	if err != nil {
		t.Fatalf("targetRouteFor(%q): %v", TargetOpenCode, err)
	}
	if route.Format != OpenCodeTargetFormat || route.BinaryDefault != OpenCodeBinaryDefault ||
		route.HomeEnv != openCodeDataRootEnv || route.PinVersion != OpenCodeFormatVersion {
		t.Errorf("route = %+v, want the OpenCode harness's own format, binary, data directory and pin", route)
	}
	if route.LoadMethod != loadMethodOpenCode || route.StructureReader != structureReaderOpenCode ||
		route.ResumeMethod != resumeMethodOpenCode {
		t.Errorf("route = %+v, want the OpenCode checks' own sentences", route)
	}
	_, err = targetRouteFor("open-code")
	if err == nil {
		t.Fatal("an unknown target name was accepted")
	}
	if !strings.Contains(err.Error(), TargetOpenCode) {
		t.Errorf("refusal = %v, want it to name the OpenCode target among the ones this build writes", err)
	}
}
