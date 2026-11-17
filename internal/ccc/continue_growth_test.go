package ccc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot is the increment's central property:
// one session converted twice while it grows yields two sessions, each attributed to the bytes it was read
// from, and the earlier one is left exactly as the first conversion wrote it.
//
// The second conversion is what the goal asks a caller to be able to do deliberately: the source gained a
// turn, and the later target holds it while the earlier target does not.
func TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot(t *testing.T) {
	fixture := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	fragment := growthFragment(t, "claude-complete-turn.jsonl")

	for _, testCase := range []struct {
		name   string
		target string
	}{
		{"into codex", TargetCodex},
		{"into claude", TargetClaude},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := fixtureConfig(t)
			cfg, sourcePath := sourceFixtureCopy(t, cfg, parser.AgentClaude, fixture,
				filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
			home := t.TempDir()
			// The unrelated session a Claude Code home already holds, and its digest: nothing about a
			// repeated conversion may reach it, which is the observation the comparison below makes.
			unrelated, unrelatedBefore := "", ""
			if testCase.target == TargetClaude {
				// The Claude Code home holds a space and an unrelated session, exactly as the route's own
				// tests use it.
				home, unrelated = claudeTargetHome(t)
				unrelatedBefore = sha256Of(t, mustRead(t, unrelated))
			}

			before := mustRead(t, sourcePath)
			beforeGrowth := sha256Of(t, before)
			beforeSize := int64(len(before))
			first := continueInto(t, cfg, home, testCase.target)

			if first.Conversion.State != ConversionStateConverted ||
				first.Installation.State != InstallationStateInstalled {
				t.Fatalf("conversion = %+v, installation = %+v, want a converted, installed session",
					first.Conversion, first.Installation)
			}
			firstBoundary := boundaryOf(t, first)
			if firstBoundary.Bytes != beforeSize || firstBoundary.RecordsEndBytes != beforeSize ||
				firstBoundary.UnpublishedTailBytes != 0 {
				t.Errorf("the first boundary = %+v, want the whole %d-byte file with nothing unpublished",
					firstBoundary, beforeSize)
			}
			if firstBoundary.SHA256 != beforeGrowth {
				t.Errorf("the first boundary digests %s, want the bytes read before the conversion (%s)",
					firstBoundary.SHA256, beforeGrowth)
			}
			if firstBoundary.Path != sourcePath || first.Source.Identity.SourcePath != sourcePath {
				t.Errorf("the first boundary names %q and the read %q, want %q",
					firstBoundary.Path, first.Source.Identity.SourcePath, sourcePath)
			}
			if warning := strings.Join(first.Warnings, " "); strings.Contains(warning, "unfinished record") {
				t.Errorf("a source with no unpublished tail warned about one: %v", first.Warnings)
			}
			firstInstalled := mustRead(t, first.Installation.Path)

			// The source gains one turn: the file grows by exactly the committed fragment, and nothing
			// else about it moves.
			grown := appendGrowth(t, sourcePath, "claude-complete-turn.jsonl")
			if grown != beforeSize+int64(len(fragment)) {
				t.Fatalf("the grown source is %d bytes, want %d (%d + the %d-byte fragment)",
					grown, beforeSize+int64(len(fragment)), beforeSize, len(fragment))
			}
			afterGrowth := sha256Of(t, mustRead(t, sourcePath))
			if afterGrowth == beforeGrowth {
				t.Fatal("the grown source digests the same as before: the case's premise does not hold")
			}

			second := continueInto(t, cfg, home, testCase.target)

			// Distinguishable: a second fresh identity, installed beside the first, and neither replaces
			// the other.
			if second.TargetSession.SessionID == first.TargetSession.SessionID {
				t.Errorf("the second conversion reused the first session id %q",
					second.TargetSession.SessionID)
			}
			if second.Installation.Path == first.Installation.Path {
				t.Errorf("the second conversion wrote the first conversion's file %q",
					second.Installation.Path)
			}
			if _, err := os.Stat(first.Installation.Path); err != nil {
				t.Errorf("the first conversion's session is gone after the second: %v", err)
			}
			// The earlier target is unchanged even though its source moved: nothing merges a later source
			// snapshot into a session a caller may already have continued.
			if got := sha256Of(t, mustRead(t, first.Installation.Path)); got != sha256Of(t, firstInstalled) {
				t.Errorf("the first conversion's session changed when the source grew and was converted again")
			}
			if unrelated != "" && sha256Of(t, mustRead(t, unrelated)) != unrelatedBefore {
				t.Errorf("the unrelated session in the Claude home changed when the source grew and was " +
					"converted again")
			}

			// Correctly attributed: the later boundary is the grown file's own bytes, and the earlier one
			// is still the bytes the first conversion read.
			secondBoundary := boundaryOf(t, second)
			if secondBoundary.SHA256 != afterGrowth || secondBoundary.Bytes != grown {
				t.Errorf("the second boundary = %+v, want the grown file (%d bytes, %s)",
					secondBoundary, grown, afterGrowth)
			}
			if secondBoundary.UnpublishedTailBytes != 0 ||
				secondBoundary.RecordsEndBytes != secondBoundary.Bytes {
				t.Errorf("the second boundary = %+v, want the whole grown file published", secondBoundary)
			}
			if secondBoundary.SHA256 == firstBoundary.SHA256 {
				t.Errorf("both conversions report the same boundary digest %s", secondBoundary.SHA256)
			}
			if first.Source.Counts.Messages != 8 || second.Source.Counts.Messages != 10 {
				t.Errorf("message counts = %d then %d, want 8 then 10: the later read must include the "+
					"history the source gained", first.Source.Counts.Messages, second.Source.Counts.Messages)
			}
			if second.Source.Identity.SourcePath != sourcePath {
				t.Errorf("the second read names %q, want %q", second.Source.Identity.SourcePath, sourcePath)
			}

			// The appended turn's own words: in the later target document, absent from the earlier one.
			secondInstalled := mustRead(t, second.Installation.Path)
			for _, text := range []string{
				"What budget do retries share per session?",
				"The retry budget is 120 seconds per session, held in internal/retry/budget.go.",
			} {
				if !strings.Contains(string(secondInstalled), text) {
					t.Errorf("the later target document does not hold the appended turn's %q", text)
				}
				if strings.Contains(string(firstInstalled), text) {
					t.Errorf("the earlier target document holds %q, which the source gained after it was "+
						"converted from it", text)
				}
			}
		})
	}
}

// TestUnpublishedSourceTailIsBoundaryedAndNeverPublished is the goal's incomplete-tail clause: a source
// captured while its writer was still appending publishes no part of the unfinished record, the read's own
// message counts do not move, and the boundary and the warning channel say what the file holds beyond the
// snapshot.
//
// Both accepted source providers are driven, because they answer the same situation differently: the Claude
// reader counts an unterminated trailing record as malformed and flags the session truncated, while the
// Codex reader deliberately counts nothing for a live session's tail (internal/parser's codex.go). The
// boundary is computed from the file's own bytes, so it states the same fact for both -- which is the point
// of taking it from bytes rather than from a provider's classification.
func TestUnpublishedSourceTailIsBoundaryedAndNeverPublished(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		agent    parser.AgentType
		ref      Ref
		fixture  string
		target   string
		fragment string
	}{
		{
			name: "a Claude source", agent: parser.AgentClaude, ref: claudeFixtureRef,
			fixture: filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"),
			target:  TargetCodex, fragment: "claude-unfinished-record.jsonl",
		},
		{
			name: "a Codex source", agent: parser.AgentCodex, ref: codexFixtureRef,
			fixture: filepath.Join("codex", "2026", "05", "04",
				"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"),
			target: TargetClaude, fragment: "codex-unfinished-record.jsonl",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fragment := growthFragment(t, testCase.fragment)
			destination := filepath.Join("-home-user-code-example-api", testCase.ref.ID+".jsonl")
			if testCase.agent == parser.AgentCodex {
				destination = filepath.Join("2026", "05", "04", "rollout-2026-05-04T09-15-00-"+testCase.ref.ID+".jsonl")
			}
			cfg := fixtureConfig(t)
			cfg, sourcePath := sourceFixtureCopy(t, cfg, testCase.agent,
				fixturePath(t, testCase.fixture), destination)
			home := t.TempDir()
			// The unrelated session a Claude Code home already holds, digested before either conversion:
			// converting an unfinished tail into that home must leave it alone as well.
			unrelated, unrelatedBefore := "", ""
			if testCase.target == TargetClaude {
				home, unrelated = claudeTargetHome(t)
				unrelatedBefore = sha256Of(t, mustRead(t, unrelated))
			}

			complete := continueRefInto(t, cfg, home, testCase.target, testCase.ref)
			completeBoundary := boundaryOf(t, complete)
			if completeBoundary.UnpublishedTailBytes != 0 {
				t.Fatalf("the committed fixture's boundary = %+v, want nothing unpublished in it",
					completeBoundary)
			}
			completeSize := completeBoundary.Bytes

			// The tail: a record the writer had not finished when the file was read.
			if grown := appendGrowth(t, sourcePath, testCase.fragment); grown != completeSize+int64(len(fragment)) {
				t.Fatalf("the grown source is %d bytes, want %d", grown, completeSize+int64(len(fragment)))
			}
			captured := continueRefInto(t, cfg, home, testCase.target, testCase.ref)

			boundary := boundaryOf(t, captured)
			if boundary.Bytes != completeSize+int64(len(fragment)) {
				t.Errorf("the boundary's size = %d, want the file's %d bytes",
					boundary.Bytes, completeSize+int64(len(fragment)))
			}
			if boundary.RecordsEndBytes != completeSize || boundary.UnpublishedTailBytes != int64(len(fragment)) {
				t.Errorf("the boundary = %+v, want its records to end at byte %d with the %d-byte unfinished "+
					"record unpublished", boundary, completeSize, len(fragment))
			}
			if boundary.SHA256 != sha256Of(t, mustRead(t, sourcePath)) {
				t.Errorf("the boundary digests %s, want the file's own bytes", boundary.SHA256)
			}
			if warning := strings.Join(captured.Warnings, " "); !strings.Contains(warning, "unfinished record") ||
				!strings.Contains(warning, "converting again") {
				t.Errorf("warnings = %v, want one naming the unfinished record and the way to carry it",
					captured.Warnings)
			}
			if captured.Installation.SourceCheckState != SourceCheckUnchanged {
				t.Errorf("source check = %q (%s), want the source reported unchanged: reading a session must "+
					"not write to it", captured.Installation.SourceCheckState, captured.Installation.Detail)
			}

			// Nothing of the unfinished record is published: the conversation the target was written from
			// is the one the complete file produced, and the fragment's own text is in neither the written
			// messages nor the installed document.
			if captured.Conversion.Carried.Messages != complete.Conversion.Carried.Messages ||
				captured.Conversion.Carried.UserMessages != complete.Conversion.Carried.UserMessages ||
				captured.Conversion.Carried.AssistantMessages != complete.Conversion.Carried.AssistantMessages ||
				captured.Conversion.Carried.ToolCalls != complete.Conversion.Carried.ToolCalls ||
				captured.Conversion.Carried.ToolResults != complete.Conversion.Carried.ToolResults {
				t.Errorf("carried evidence changed with only an unfinished record appended: %+v, was %+v",
					captured.Conversion.Carried, complete.Conversion.Carried)
			}
			fragmentText := unfinishedRecordText(t, testCase.fragment)
			// The control on the search itself: the fragment's words are in the source file this
			// conversion read, so a document that held them would be found holding them.
			if !strings.Contains(string(mustRead(t, sourcePath)), fragmentText) {
				t.Fatalf("the case's own search text %q is not in the source it searches for it in", fragmentText)
			}
			installed := mustRead(t, captured.Installation.Path)
			if strings.Contains(string(installed), fragmentText) {
				t.Errorf("the installed session holds %q, which the source had not finished writing", fragmentText)
			}
			if unrelated != "" && sha256Of(t, mustRead(t, unrelated)) != unrelatedBefore {
				t.Errorf("the unrelated session in the Claude home changed while the unfinished tail was " +
					"converted")
			}
		})
	}
}

// TestSourceBoundaryOfFileStatesTheRecordOffsets pins the pass itself, byte by byte: a file whose last line
// is not terminated has its records ending at the last line feed, a file with no line feed at all has no
// complete record in it, and an empty file has neither.
func TestSourceBoundaryOfFileStatesTheRecordOffsets(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		content         string
		recordsAreLines bool
		wantSize        int64
		wantRecordsEnd  int64
		wantTail        int64
	}{
		{"two complete lines", "{\"a\":1}\n{\"b\":2}\n", true, 16, 16, 0},
		{"an unfinished last line", "{\"a\":1}\n{\"b\":", true, 13, 8, 5},
		{"no line feed at all", "{\"a\":1}", true, 7, 0, 7},
		{"an empty file", "", true, 0, 0, 0},
		// A source whose records are not the lines a harness appends: the file's own bytes are still
		// measured, and no record boundary is claimed for them.
		{"a source that is not line-recorded", "{\"a\":1}\n{\"b\":", false, 13, 0, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("writing the source: %v", err)
			}
			boundary, err := sourceBoundaryOfFile(path, testCase.recordsAreLines)
			if err != nil {
				t.Fatalf("taking the boundary: %v", err)
			}
			if boundary.State != SourceBoundaryTaken {
				t.Fatalf("state = %q, want %q", boundary.State, SourceBoundaryTaken)
			}
			if boundary.Bytes != testCase.wantSize {
				t.Errorf("bytes = %d, want %d", boundary.Bytes, testCase.wantSize)
			}
			if boundary.SHA256 != sha256Of(t, mustRead(t, path)) {
				t.Errorf("sha256 = %s, want the file's own digest", boundary.SHA256)
			}
			if boundary.RecordsAreLines != testCase.recordsAreLines {
				t.Errorf("records_are_lines = %v, want %v", boundary.RecordsAreLines, testCase.recordsAreLines)
			}
			if boundary.RecordsEndBytes != testCase.wantRecordsEnd {
				t.Errorf("records_end = %d, want %d", boundary.RecordsEndBytes, testCase.wantRecordsEnd)
			}
			if boundary.UnpublishedTailBytes != testCase.wantTail {
				t.Errorf("unpublished tail = %d, want %d",
					boundary.UnpublishedTailBytes, testCase.wantTail)
			}
		})
	}
	// The other arm of the same rule: a file that cannot be read is a stopped run naming what it was
	// doing, never a boundary with no bytes in it -- "the source has no file to measure" and "the file
	// could not be read" are different answers, and only the first is a state. The assertion is on the
	// error's own kind rather than on a message, which differs per platform.
	if _, err := sourceBoundaryOfFile(filepath.Join(t.TempDir(), "absent.jsonl"), true); err == nil {
		t.Error("a boundary was reported for a file that is not there")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want the file error itself (the file is not there)", err)
	}
}

// TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure pins the ways a boundary is not taken: an
// archive read had no source file open, a container member's bytes are the container's rather than this
// conversation's, and a read that named no path at all has no file to measure. Each is reported as a state
// with its own reason, never as an empty block a caller would have to guess about -- and never as an error,
// because the conversation itself was read; the stop a *comparison* makes for the same shapes is pinned
// beside this one (TestSourceCheckStopsAReadThatNamesNoFileAtAll).
func TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		readFrom string
		path     string
		want     string
	}{
		{"an archive read", "archive", "/absent/session.jsonl", "archive row"},
		{"a container member", "source",
			parser.VirtualSourcePath("/containers/store.sqlite", "member-1"), "member"},
		{"a read that named no path", "source", "", "no path"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reader := NewSourceReader(fixtureConfig(t))
			conversation := Conversation{Source: SourceIdentity{Provider: "claude", SourcePath: testCase.path}}
			boundary, err := takeSourceBoundary(reader, conversation, testCase.readFrom)
			if err != nil {
				t.Fatalf("taking the boundary: %v", err)
			}
			if boundary.State != SourceBoundaryNotTaken {
				t.Fatalf("state = %q (%s), want %q", boundary.State, boundary.Detail, SourceBoundaryNotTaken)
			}
			if !strings.Contains(boundary.Detail, testCase.want) {
				t.Errorf("detail = %q, want it to name %q", boundary.Detail, testCase.want)
			}
			if boundary.SHA256 != "" || boundary.Bytes != 0 {
				t.Errorf("boundary = %+v, want no bytes claimed", boundary)
			}
		})
	}
}

// TestBoundaryWarningsNameAnUnfinishedRecord pins the warning half: a tail nobody measured is not warned
// about, and a measured one is named with both offsets and the remedy the goal states.
func TestBoundaryWarningsNameAnUnfinishedRecord(t *testing.T) {
	if warnings := boundaryWarnings(nil); len(warnings) != 0 {
		t.Errorf("a report with no boundary warned: %v", warnings)
	}
	notTaken := &SourceBoundary{State: SourceBoundaryNotTaken, Detail: "an archive read"}
	if warnings := boundaryWarnings(notTaken); len(warnings) != 0 {
		t.Errorf("a boundary that was not taken warned: %v", warnings)
	}
	complete := &SourceBoundary{State: SourceBoundaryTaken, Bytes: 1792, RecordsEndBytes: 1792}
	if warnings := boundaryWarnings(complete); len(warnings) != 0 {
		t.Errorf("a file with nothing unpublished warned: %v", warnings)
	}
	warnings := boundaryWarnings(&SourceBoundary{
		State: SourceBoundaryTaken, Bytes: 1835, RecordsEndBytes: 1792, UnpublishedTailBytes: 43,
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	for _, want := range []string{"unfinished record", "1835", "1792", "43 byte(s)", "converting again"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("the warning %q does not name %q", warnings[0], want)
		}
	}
}

// TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations pins the gate on the record offsets: they are
// claimed for the providers whose own declarations say their sources are line records, and not for a
// provider that declares nothing this build may read as that -- the name it cannot resolve included.
//
// The three middle rows are the regression case for the gate's own premise (Q-0004-07): each is a provider
// in the carried tree whose source is one whole-file JSON document (a QwenPaw session file, a CodeBuddy
// manifest) or a possibly compressed container (DeepSeek Harness), each declares
// ContentCapabilities.MalformedLineCount -- a count of malformed *entries*, not of unparsed lines -- and
// none may be reported as a line-recorded source. The gate once accepted that count as the declaration and
// reported a complete file of theirs as one unfinished record; this table is what fails when it does again.
func TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations(t *testing.T) {
	reader := NewSourceReader(fixtureConfig(t))
	for _, testCase := range []struct {
		provider string
		want     bool
	}{
		{"claude", true},
		{"codex", true},
		{string(parser.AgentQwenPaw), false},
		{string(parser.AgentCodeBuddy), false},
		{string(parser.AgentDeepSeekHarness), false},
		{"no-such-provider", false},
		{"", false},
	} {
		if got := sourceRecordsAreLines(reader, testCase.provider); got != testCase.want {
			t.Errorf("sourceRecordsAreLines(%q) = %v, want %v", testCase.provider, got, testCase.want)
		}
	}
	if sourceRecordsAreLines(nil, "claude") {
		t.Error("a reader-less call claimed a record boundary: no declaration was read")
	}
}

// TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary is the end-to-end half of the gate's premise
// (Q-0004-07): a complete QwenPaw session -- one whole-file JSON document, the provider's own shape, with no
// trailing line feed -- converts without the run stating anything false about it. The boundary must state
// the file's bytes and claim no record offset, no warning may call the file's own content an unfinished
// record, and the digest must be the bytes the conversion read. The defect this pins reported
// `records_are_lines: true` with the whole file as unpublished tail and a "converting again carries them"
// warning for a file whose read had consumed every byte.
func TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary(t *testing.T) {
	cfg := fixtureConfig(t)
	root := t.TempDir()
	directory := filepath.Join(root, "default", "sessions")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("creating the QwenPaw sessions directory: %v", err)
	}
	// The provider's on-disk shape, as its own parser documents it: one document whose agent memory holds
	// [message, metadata] pairs. Written compactly and without a trailing newline, the way the provider's
	// writer leaves a session file.
	document := `{"agent":{"memory":{"content":[[` +
		`{"id":"u1","name":"user","role":"user","content":[{"type":"text","text":"Which file holds the retry budget?"}],"metadata":{},"timestamp":"2026-05-04 09:15:00.000"},[]],` +
		`[{"id":"a1","name":"Friday","role":"assistant","content":[{"type":"text","text":"internal/retry/budget.go."}],"metadata":{},"timestamp":"2026-05-04 09:15:01.000"},[]]` +
		`]}}}`
	sourcePath := filepath.Join(directory, "mine.json")
	if err := os.WriteFile(sourcePath, []byte(document), 0o600); err != nil {
		t.Fatalf("writing the QwenPaw session: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentQwenPaw: {root}}

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          Ref{Provider: string(parser.AgentQwenPaw), ID: sourcePath},
		Target:       TargetCodex,
		Config:       cfg,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing from the QwenPaw session: %v", err)
	}
	if result.Conversion.ReadFrom != "source" {
		t.Fatalf("read_from = %q, want the source file this case wrote", result.Conversion.ReadFrom)
	}
	if result.Source.Counts.Messages != 2 {
		t.Errorf("read %d message(s), want the document's own 2", result.Source.Counts.Messages)
	}
	boundary := boundaryOf(t, result)
	if boundary.RecordsAreLines {
		t.Errorf("boundary = %+v, want no record boundary claimed: the source is one JSON document, not a "+
			"transcript of appended line records", boundary)
	}
	if boundary.Bytes != int64(len(document)) || boundary.SHA256 != sha256Of(t, mustRead(t, sourcePath)) {
		t.Errorf("boundary = %+v, want the file's own %d bytes and digest", boundary, len(document))
	}
	if boundary.RecordsEndBytes != 0 || boundary.UnpublishedTailBytes != 0 {
		t.Errorf("boundary = %+v, want no record offset and no unpublished tail claimed", boundary)
	}
	if warning := strings.Join(result.Warnings, " "); strings.Contains(warning, "unfinished record") {
		t.Errorf("the conversion warned about an unfinished record in a complete file: %v", result.Warnings)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q (%s), want the source reported unchanged",
			result.Installation.SourceCheckState, result.Installation.Detail)
	}
}

// TestConversionFromAnArchiveReportsNoBoundaryToTake pins the end-to-end half of the not-taken state: a
// conversion that fell back to the archive says there was no source file to measure, in the boundary block
// as well as in the source comparison beside it.
func TestConversionFromAnArchiveReportsNoBoundaryToTake(t *testing.T) {
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	cfg = archiveFixtureCopy(t, cfg, sourcePath,
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          claudeFixtureRef,
		Target:       TargetCodex,
		Config:       cfg,
		ArchivePath:  cfg.DBPath,
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing from the archive: %v", err)
	}
	if result.Conversion.ReadFrom != "archive" {
		t.Fatalf("read_from = %q, want the archive fallback this case is about", result.Conversion.ReadFrom)
	}
	boundary := boundaryOf(t, result)
	if boundary.SHA256 != "" || boundary.Bytes != 0 {
		t.Errorf("boundary = %+v, want no bytes claimed for an archive read", boundary)
	}
	if result.Installation.SourceCheckState != SourceCheckNotVerified ||
		!strings.Contains(result.Installation.Detail, "archive") {
		t.Errorf("source_check_state = %q (%s), want a comparison that did not run and says why",
			result.Installation.SourceCheckState, result.Installation.Detail)
	}
	if warning := strings.Join(result.Warnings, " "); strings.Contains(warning, "unfinished record") {
		t.Errorf("an archive read warned about an unfinished source file: %v", result.Warnings)
	}
}

// continueInto converts the committed Claude fixture — the copy the caller's configuration points at —
// into one target home.
func continueInto(t *testing.T, cfg config.Config, home, target string) ContinueResult {
	t.Helper()
	return continueRefInto(t, cfg, home, target, claudeFixtureRef)
}

// continueRefInto converts one reference into one target home, with the missing target binary every case
// here uses so no host's installed harness decides what a case observes.
func continueRefInto(
	t *testing.T, cfg config.Config, home, target string, ref Ref,
) ContinueResult {
	t.Helper()
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          ref,
		Target:       target,
		Config:       cfg,
		TargetHome:   home,
		TargetBinary: missingTargetBinary,
		Verify:       VerifyNone,
	})
	if err != nil {
		t.Fatalf("continuing %s:%s into %s: %v", ref.Provider, ref.ID, target, err)
	}
	return result
}

// boundaryOf answers the boundary a conversion reported, failing the case when there is none: every
// conversion in this file reports a boundary block -- a not-taken state is a block, not a missing one --
// so a missing block is the defect, not a state.
func boundaryOf(t *testing.T, result ContinueResult) SourceBoundary {
	t.Helper()
	if result.Source == nil || result.Source.Boundary == nil {
		t.Fatalf("the conversion reported no source boundary at all: source = %+v", result.Source)
	}
	return *result.Source.Boundary
}

// mustRead reads one file a conversion wrote, so a case can search its bytes rather than trust a count.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

// sha256Of is an independent digest of bytes already read -- crypto/sha256 here rather than through the
// code the report's own boundary uses, so a case that checks a reported digest is not checking the
// function that produced it -- for comparing a file's content before and after another run.
func sha256Of(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// unfinishedRecordText answers the dialogue the one committed unfinished-record fragment holds, which is
// what a case searches a written document for: the fragment is a byte prefix, so it is read as the text
// field's own value up to the next quote (an unterminated record loses its closing quote with the rest
// of the record), never by parsing it as a record.
func unfinishedRecordText(t *testing.T, name string) string {
	t.Helper()
	data := string(growthFragment(t, name))
	const marker = `"text":"`
	start := strings.Index(data, marker)
	if start < 0 {
		t.Fatalf("the fragment %s holds no text field: %s", name, data)
	}
	text := data[start+len(marker):]
	if end := strings.Index(text, `"`); end >= 0 {
		text = text[:end]
	}
	if text == "" {
		t.Fatalf("the fragment %s holds no text after its text field: %s", name, data)
	}
	return text
}
