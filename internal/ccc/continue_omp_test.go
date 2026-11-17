package ccc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompTargetHome answers a data directory with one unrelated session already in it, in the layout the
// harness itself files sessions under (one project directory below the sessions root): the conversion
// must leave that session alone.
func ompTargetHome(t *testing.T) (string, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "omp home")
	unrelated := filepath.Join(home, ompSessionsSubdir, "-home-user-other-project",
		"2026-01-02T00-00-00-000Z_00000000-0000-4000-8000-0000000000ff.jsonl")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatalf("creating the unrelated session's directory: %v", err)
	}
	if err := os.WriteFile(unrelated, []byte(
		`{"type":"session","version":3,"id":"00000000-0000-4000-8000-0000000000ff","timestamp":"2026-01-02T00:00:00.000Z","cwd":"/home/user/other-project"}`+"\n"),
		0o600); err != nil {
		t.Fatalf("writing the unrelated session: %v", err)
	}
	return home, unrelated
}

// TestContinueIntoOMPConvertsInstallsAndReadsBack is the route's central property test: one call turns a
// real Claude Code session into a new OMP session, installs it under the project directory OMP's own
// spelling derives from the session's working directory, leaves the source and an unrelated session
// untouched, reads the installed file back through the inherited OMP reader as what the writer placed in
// it, and reports the format's own limits and losses -- including the two OMP's format adds.
func TestContinueIntoOMPConvertsInstallsAndReadsBack(t *testing.T) {
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	sourceBefore := hashFile(t, sourcePath)
	home, unrelated := ompTargetHome(t)
	unrelatedBefore := hashFile(t, unrelated)

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetOMP,
		Config:       fixtureConfig(t),
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}

	if result.Target != TargetOMP {
		t.Errorf("target = %q, want %q", result.Target, TargetOMP)
	}
	if result.Conversion.State != ConversionStateConverted || result.Conversion.ReadFrom != "source" {
		t.Errorf("conversion = %+v, want a conversion read from the source", result.Conversion)
	}
	if result.Conversion.Format != OMPTargetFormat {
		t.Errorf("format = %q, want %q", result.Conversion.Format, OMPTargetFormat)
	}
	if result.TargetSession.Provider != TargetOMP {
		t.Errorf("provider = %q, want %q", result.TargetSession.Provider, TargetOMP)
	}
	if result.Installation.State != InstallationStateInstalled {
		t.Fatalf("installation = %+v, want the session installed", result.Installation)
	}
	wantDir := filepath.Join(home, ompSessionsSubdir, OMPProjectDir("/home/user/code/example-api"))
	if filepath.Dir(result.TargetSession.Path) != wantDir {
		t.Errorf("installed under %s, want OMP's own spelling of the working directory (%s)",
			filepath.Dir(result.TargetSession.Path), wantDir)
	}
	if !strings.HasSuffix(result.TargetSession.FileName, "_"+result.TargetSession.SessionID+".jsonl") {
		t.Errorf("file name = %q, want <timestamp>_<session id>.jsonl", result.TargetSession.FileName)
	}
	if result.Verification.Structure.State != VerificationStateVerified {
		t.Errorf("structure = %+v, want the installed file read back as what was written",
			result.Verification.Structure)
	}
	if result.Verification.Structure.ReadBack.ToolCalls != 1 || result.Verification.Structure.ReadBack.ToolResults != 1 {
		t.Errorf("read-back = %+v, want the fixture's own call and result",
			result.Verification.Structure.ReadBack)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q, want the source unchanged", result.Installation.SourceCheckState)
	}
	if hashFile(t, sourcePath) != sourceBefore {
		t.Error("the conversion changed the source session")
	}
	if hashFile(t, unrelated) != unrelatedBefore {
		t.Error("the conversion changed an unrelated session in the target home")
	}

	// The report lists the limits this pair of formats has, including the two OMP's format adds: the title
	// slot the harness rewrites in place, and the session's own model binding.
	limits := map[string]bool{}
	for _, limit := range result.TargetSession.FormatLimits {
		limits[limit.Category] = true
	}
	for _, want := range []string{"title_slot", "session_model_binding"} {
		if !limits[want] {
			t.Errorf("format limits %v do not name %s", limits, want)
		}
	}
	losses := map[string]Loss{}
	for _, loss := range result.Conversion.Losses {
		losses[loss.Category] = loss
	}
	if _, ok := losses["target_model_binding"]; !ok {
		t.Error("the conversion report carries no target_model_binding loss")
	}
	if !strings.Contains(losses["target_model_binding"].Detail, missingTargetBinary) {
		t.Errorf("target_model_binding detail = %q, want it to name the harness that answered nothing "+
			"(%s is not installed here)", losses["target_model_binding"].Detail, missingTargetBinary)
	}
	if _, ok := losses["title_slot_written"]; !ok {
		t.Error("the conversion report carries no title_slot_written loss")
	}

	// The invocation: the data directory, the directory holding the session's file, and the session's own
	// working directory -- and the printed line is the exec form's own words.
	if result.TargetSession.ResumeEnvironment[ompHomeEnv] != home {
		t.Errorf("resume environment = %v, want the data directory named",
			result.TargetSession.ResumeEnvironment)
	}
	if result.TargetSession.ResumeEnvironment[ompSessionDirEnv] !=
		filepath.Join(home, ompSessionsSubdir, OMPProjectDir("/home/user/code/example-api")) {
		t.Errorf("resume environment = %v, want the session's own directory named: naming the sessions root "+
			"above it answers that no such session is installed",
			result.TargetSession.ResumeEnvironment)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, "cd /home/user/code/example-api") {
		t.Errorf("resume command = %q, want it to change into the session's working directory",
			result.TargetSession.ResumeCommand)
	}
	if !strings.Contains(result.TargetSession.ResumeCommand, "--resume "+result.TargetSession.SessionID) {
		t.Errorf("resume command = %q, want the session named by id", result.TargetSession.ResumeCommand)
	}

	// The native half is not claimed: the harness this run was pointed at is not installed, so both checks
	// report what they could not run rather than a pass.
	if result.Verification.Load.State == VerificationStateVerified {
		t.Errorf("load = %+v, want it not verified: %s is not installed here",
			result.Verification.Load, missingTargetBinary)
	}
	if result.Verification.Structure.Method != structureMethodOMP {
		t.Errorf("structure method = %q, want the OMP reader's own sentence", result.Verification.Structure.Method)
	}
}

// TestContinueIntoOMPDryRunReportsWhatItDidNotAsk pins the dry run's report: nothing is installed, the
// harness is never started, and the two facts that depend on it -- the session's model binding and the
// endpoint label every assistant record carries -- are named as unasked rather than left to be inferred
// from a missing record.
func TestContinueIntoOMPDryRunReportsWhatItDidNotAsk(t *testing.T) {
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetOMP,
		Config:       fixtureConfig(t),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		DryRun:       true,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Installation.State == InstallationStateInstalled {
		t.Errorf("installation = %+v, want a dry run to install nothing", result.Installation)
	}
	if result.Conversion.DryRun != true {
		t.Errorf("conversion = %+v, want the run reported as a dry run", result.Conversion)
	}
	for _, loss := range result.Conversion.Losses {
		if loss.Category != "target_model_binding" {
			continue
		}
		if loss.Count != 0 {
			t.Errorf("target_model_binding count = %d, want no binding written", loss.Count)
		}
		if !strings.Contains(loss.Detail, "dry run") {
			t.Errorf("detail = %q, want it to say the harness was not asked because this is a dry run",
				loss.Detail)
		}
		return
	}
	t.Error("the report carries no target_model_binding entry")
}

// TestContinueFromOMPThroughTheCommonModel pins the source direction: an OMP session continues in each
// previously supported target harness through the same route table, the same reader and the same writers,
// with no OMP-specific branch in the pipeline. The claim is made testable by the direct writer call below:
// the pipeline's conversion is compared with a direct conversion of the same document by the target's own
// writer, so an OMP-specific shortcut that bypassed the shared model is red.
func TestContinueFromOMPThroughTheCommonModel(t *testing.T) {
	conversation := readFixtureConversation(t, ompFixtureRef)
	for _, target := range []string{TargetClaude, TargetCodex, TargetPi} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          ompFixtureRef,
				Target:       target,
				Config:       fixtureConfig(t),
				TargetHome:   home,
				TargetBinary: missingTargetBinary,
				Verify:       VerifyNone,
			})
			if err != nil {
				t.Fatalf("continuing into %s: %v", target, err)
			}
			if result.Conversion.State != ConversionStateConverted {
				t.Fatalf("conversion = %+v, want the OMP session converted", result.Conversion)
			}
			if result.Conversion.ReadFrom != "source" {
				t.Errorf("read from = %q, want the source file", result.Conversion.ReadFrom)
			}
			if target == TargetPi {
				// The shared helper covers the Claude and Codex writers; Pi's own is compared here, over
				// the same document and by the same two answers (what was carried, and what the target's
				// own reader made of the file).
				direct, err := ConvertToPiSession(conversation, PiTargetOptions{
					SessionID: result.TargetSession.SessionID,
				})
				if err != nil {
					t.Fatalf("converting the same document with Pi's own writer: %v", err)
				}
				if got, want := carriedWithoutBytes(result.Conversion.Carried),
					carriedWithoutBytes(direct.Carried); got != want {
					t.Errorf("the pipeline carried %+v, while Pi's own writer over the same source carried %+v",
						got, want)
				}
				directHome := t.TempDir()
				if _, _, err := InstallPiSession(direct, directHome); err != nil {
					t.Fatalf("installing the direct conversion: %v", err)
				}
				directVerify, err := VerifyPiSessionStructure(context.Background(),
					filepath.Join(directHome, piSessionsSubdir), direct, fixtureConfig(t))
				if err != nil {
					t.Fatalf("reading the direct conversion back: %v", err)
				}
				if directVerify.ReadBack != result.Verification.Structure.ReadBack {
					t.Errorf("the pipeline read back %+v, while Pi's own writer over the same source read "+
						"back %+v: an OMP-specific shortcut is not the shared model's answer",
						result.Verification.Structure.ReadBack, directVerify.ReadBack)
				}
				return
			}
			directWriterAgreesWithThePipeline(t, target, conversation, result)
		})
	}
}

// TestContinueFromOMPIntoOMPKeepsTheFamilyTree pins the OMP-specific round trip: an OMP session converted
// into a *new* OMP session keeps the source's own relations, and the file the writer produces is read back
// by the inherited OMP reader as the same conversation -- with the source's title slot and `custom`
// records neither flattened into messages nor refused.
func TestContinueFromOMPIntoOMPKeepsTheFamilyTree(t *testing.T) {
	conversation := readFixtureConversation(t, ompHistoryFixtureRef)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          ompHistoryFixtureRef,
		Target:       TargetOMP,
		Config:       fixtureConfig(t),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing: %v", err)
	}
	if result.Verification.Structure.ReadBack.CompactionBoundaries != 1 {
		t.Errorf("read-back = %+v, want the compaction boundary carried", result.Verification.Structure.ReadBack)
	}
	direct, err := ConvertToOMPSession(conversation, OMPTargetOptions{SessionID: result.TargetSession.SessionID})
	if err != nil {
		t.Fatalf("converting the same document directly: %v", err)
	}
	if carriedWithoutBytes(result.Conversion.Carried) != carriedWithoutBytes(direct.Carried) {
		t.Errorf("the pipeline carried %+v, while the target's own writer carried %+v",
			carriedWithoutBytes(result.Conversion.Carried), carriedWithoutBytes(direct.Carried))
	}
}

// TestOMPRouteIsRegistered pins what the route table answers: the OMP target is one this build writes, its
// route names the harness's own binary and data directory, and an unknown target's refusal lists it.
func TestOMPRouteIsRegistered(t *testing.T) {
	if !slicesContains(Targets(), TargetOMP) {
		t.Errorf("Targets() = %v, want the OMP target among them", Targets())
	}
	if Targets()[len(Targets())-1] != TargetOMP {
		t.Errorf("Targets() = %v, want the new target last", Targets())
	}
	route, err := targetRouteFor(TargetOMP)
	if err != nil {
		t.Fatalf("targetRouteFor(%q): %v", TargetOMP, err)
	}
	if route.Format != OMPTargetFormat || route.BinaryDefault != OMPBinaryDefault ||
		route.HomeEnv != ompHomeEnv || route.PinVersion != OMPFormatVersion {
		t.Errorf("route = %+v, want the OMP harness's own format, binary, data directory and pin", route)
	}
	if route.LoadMethod != loadMethodOMP || route.StructureReader != structureReaderOMP {
		t.Errorf("route = %+v, want the OMP checks' own sentences", route)
	}
	_, err = targetRouteFor("oh-my-pi")
	if err == nil {
		t.Fatal("an unknown target name was accepted")
	}
	if !strings.Contains(err.Error(), TargetOMP) {
		t.Errorf("refusal = %v, want it to name the OMP target among the ones this build writes", err)
	}
}

// slicesContains reports whether one string is in a list, without pulling a helper's name into a case that
// asserts a route table.
func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
