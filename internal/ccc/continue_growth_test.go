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

	"github.com/klauspost/compress/zstd"

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
			if firstBoundary.Bytes != beforeSize ||
				firstBoundary.RecordOffsets != SourceBoundaryRecordsClaimed ||
				firstBoundary.TerminatedRecordsEndBytes != beforeSize ||
				firstBoundary.UnterminatedTailBytes != 0 {
				t.Errorf("the first boundary = %+v, want the whole %d-byte file with nothing unterminated",
					firstBoundary, beforeSize)
			}
			// The conversion is attributed to the file it read: the boundary compares the file's size with
			// the size the read's own record holds for it and reports the match as a match -- a size, which
			// is as far as that record can show.
			if firstBoundary.ReadBytes != beforeSize ||
				firstBoundary.ReadCoverage != SourceBoundaryCoverageSizeMatch {
				t.Errorf("the first boundary = %+v, want its read's own snapshot to be the file's %d bytes",
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
			if warning := strings.Join(first.Warnings, " "); strings.Contains(warning, "hold no line feed") {
				t.Errorf("a source with no unterminated record warned about one: %v", first.Warnings)
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
			if secondBoundary.UnterminatedTailBytes != 0 ||
				secondBoundary.TerminatedRecordsEndBytes != secondBoundary.Bytes ||
				secondBoundary.ReadCoverage != SourceBoundaryCoverageSizeMatch ||
				secondBoundary.ReadBytes != grown {
				t.Errorf("the second boundary = %+v, want the whole grown file, read as it stands", secondBoundary)
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

// TestUnterminatedSourceTailIsBoundaryedAndNeverPublished is the goal's incomplete-tail clause: a source
// captured while its writer was still appending publishes no part of the unfinished record, the read's own
// message counts do not move, and the boundary and the warning channel say what the file holds beyond the
// read.
//
// Both accepted source providers are driven, because they answer the same situation differently: the Claude
// reader counts an unterminated trailing record as malformed and flags the session truncated, while the
// Codex reader deliberately counts nothing for a live session's tail (internal/parser's codex.go). The
// boundary is computed from the file's own bytes, so it states the same fact for both -- which is the point
// of taking it from bytes rather than from a provider's classification.
//
// What the boundary states is the byte fact and no more: the file's bytes end without a line feed, so the
// record there had not been terminated. It does not call the record unpublished, because whether a read
// carried an unterminated record is the reader's own answer (the case below the next one appends a record
// without its line feed that both readers carry), and the warning must not say otherwise either.
func TestUnterminatedSourceTailIsBoundaryedAndNeverPublished(t *testing.T) {
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
			if completeBoundary.UnterminatedTailBytes != 0 {
				t.Fatalf("the committed fixture's boundary = %+v, want nothing unterminated in it",
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
			if boundary.RecordOffsets != SourceBoundaryRecordsClaimed ||
				boundary.TerminatedRecordsEndBytes != completeSize ||
				boundary.UnterminatedTailBytes != int64(len(fragment)) {
				t.Errorf("the boundary = %+v, want its terminated records to end at byte %d with the "+
					"%d-byte unterminated record after them", boundary, completeSize, len(fragment))
			}
			if boundary.ReadCoverage != SourceBoundaryCoverageSizeMatch || boundary.ReadBytes != boundary.Bytes {
				t.Errorf("the boundary = %+v, want the read's own snapshot to be the file it measured",
					boundary)
			}
			if boundary.SHA256 != sha256Of(t, mustRead(t, sourcePath)) {
				t.Errorf("the boundary digests %s, want the file's own bytes", boundary.SHA256)
			}
			// The warning channel names the bytes and the remedy, and it does not claim the record was left
			// out of the conversion: whether the read carried an unterminated record is the read's own
			// answer, and this file's reader (either accepted one) carries a record it can read whole.
			warning := strings.Join(captured.Warnings, " ")
			if !strings.Contains(warning, "hold no line feed") || !strings.Contains(warning, "converting again") {
				t.Errorf("warnings = %v, want one naming the unterminated record and the way to read it "+
					"finished", captured.Warnings)
			}
			for _, forbidden := range []string{"not part of this conversion", "unpublished", "are not part"} {
				if strings.Contains(warning, forbidden) {
					t.Errorf("warnings = %v, want no claim that the record was left out of the conversion "+
						"(%q)", captured.Warnings, forbidden)
				}
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
			tailText := fragmentText(t, testCase.fragment)
			// The control on the search itself: the fragment's words are in the source file this
			// conversion read, so a document that held them would be found holding them.
			if !strings.Contains(string(mustRead(t, sourcePath)), tailText) {
				t.Fatalf("the case's own search text %q is not in the source it searches for it in", tailText)
			}
			installed := mustRead(t, captured.Installation.Path)
			if strings.Contains(string(installed), tailText) {
				t.Errorf("the installed session holds %q, which the source had not finished writing", tailText)
			}
			if unrelated != "" && sha256Of(t, mustRead(t, unrelated)) != unrelatedBefore {
				t.Errorf("the unrelated session in the Claude home changed while the unfinished tail was " +
					"converted")
			}
		})
	}
}

// TestContinueCarriesACompleteRecordThatHasNoLineFeed is the contradiction the review measured, pinned in
// both directions: a complete record without its final line feed is a record every inherited reader this
// build reads carries (internal/parser's codex_cursor_test.go pins that for Codex; the Claude reader
// processes a valid unterminated line, and the Pi reader carries one too), so the conversion must carry its
// dialogue -- while the boundary may state the byte fact (the file ends without a line feed) and must not
// claim the record was left out.
//
// Each source provider is driven with its own committed growth fragment, converted into another harness:
// the fragment holds a further turn in that provider's record shape, and the two conversions of one row
// differ in one byte of the source -- the fragment as committed, and the same fragment with its last byte
// (its line feed) removed. Their conversations must be the same conversation, and only the second boundary
// may say the file's last bytes are an unterminated record. What the boundary may *claim* about those bytes
// is the provider's own answer (a provider whose declarations make its source a transcript of appended
// lines has the offsets named; the Pi provider's do not, and its boundary states the byte fact and its
// reason), which is the per-row `offsets` field: the record-offset gate is the reader's, and a case that
// required the same claim of every provider would assert something the reader itself refuses.
func TestContinueCarriesACompleteRecordThatHasNoLineFeed(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		agent       parser.AgentType
		ref         Ref
		fixture     string
		destination string
		fragment    string
		target      string
		// offsets is what this provider's own declarations make of a line-record boundary for the file:
		// SourceBoundaryRecordsClaimed where it is a transcript of appended lines whose bytes the offsets
		// may be given for, SourceBoundaryRecordsNotClaimed where the provider's declarations do not make
		// it one (the Pi provider), which the boundary states with its own reason instead.
		offsets string
	}{
		{
			name: "a Claude source into Codex", agent: parser.AgentClaude, ref: claudeFixtureRef,
			fixture:     filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"),
			destination: filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"),
			fragment:    "claude-complete-turn.jsonl", target: TargetCodex,
			offsets: SourceBoundaryRecordsClaimed,
		},
		{
			name: "a Codex source into Claude", agent: parser.AgentCodex, ref: codexFixtureRef,
			fixture: filepath.Join("codex", "2026", "05", "04",
				"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"),
			destination: filepath.Join("2026", "05", "04",
				"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"),
			fragment: "codex-complete-turn.jsonl", target: TargetClaude,
			offsets: SourceBoundaryRecordsClaimed,
		},
		{
			// The Pi source's own fragment: it chains to the ordinary fixture's last entry, so the grown
			// file is one further turn of that same conversation, and the provider's reader is the third
			// one the no-line-feed rule is held against. Its boundary claims no record offset, because
			// this provider's declarations do not make the source a transcript of appended line records.
			name: "a Pi source into Claude", agent: parser.AgentPi, ref: piFixtureRef,
			fixture:     filepath.Join("pi", piFixtureRef.ID+".jsonl"),
			destination: piFixtureRef.ID + ".jsonl",
			fragment:    "pi-complete-turn.jsonl", target: TargetClaude,
			offsets: SourceBoundaryRecordsNotClaimed,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := fixturePath(t, testCase.fixture)
			fragment := growthFragment(t, testCase.fragment)
			if !strings.HasSuffix(string(fragment), "\n") {
				t.Fatalf("the committed fragment %s does not end at a line feed: this case needs one to "+
					"remove", testCase.fragment)
			}
			// The dialogue the fragment's *last* record holds -- the record this case's second half leaves
			// unterminated -- read the way the case above reads an unfinished one: the fragment's own text
			// field, never by parsing it as a record.
			addedText := fragmentText(t, testCase.fragment)
			// The search is about the last record and nothing else: the text has to sit after the
			// second-to-last line feed, because the fragment's earlier records are terminated and the file
			// carries them whether or not the no-line-feed rule holds. A case that searched the fragment's
			// *first* record -- or a helper that read the first text field -- would stay green while the
			// record this case is about was dropped.
			lastRecord := string(fragment)[strings.LastIndex(strings.TrimSuffix(string(fragment), "\n"), "\n")+1:]
			if !strings.Contains(lastRecord, addedText) {
				t.Fatalf("the case searches for %q, which is not in the fragment's last record %q: the "+
					"no-line-feed rule is about that record alone", addedText, lastRecord)
			}

			convert := func(grow func(path string) int64) (ContinueResult, string) {
				t.Helper()
				cfg := fixtureConfig(t)
				cfg, sourcePath := sourceFixtureCopy(t, cfg, testCase.agent, fixture, testCase.destination)
				complete := mustSize(t, sourcePath)
				if grown := grow(sourcePath); grown <= complete {
					t.Fatalf("the grown source is %d bytes, want more than the %d-byte fixture", grown, complete)
				}
				return continueRefInto(t, cfg, t.TempDir(), testCase.target, testCase.ref), sourcePath
			}

			withFeed, feedPath := convert(func(path string) int64 {
				return appendGrowth(t, path, testCase.fragment)
			})
			noFeed, noFeedPath := convert(func(path string) int64 {
				return appendGrowthBytes(t, path, fragment[:len(fragment)-1])
			})

			// The fragment holds more than one record, each terminated by its own line feed: removing the
			// last byte leaves only the fragment's last record unterminated, and the file's terminated
			// records end after the one before it.
			withoutLastFeed := fragment[:len(fragment)-1]
			lastFeed := strings.LastIndex(string(withoutLastFeed), "\n")
			if lastFeed < 0 {
				t.Fatalf("the committed fragment %s holds no line feed but its last: this case needs one to "+
					"keep", testCase.fragment)
			}
			wantTail := int64(len(withoutLastFeed) - lastFeed - 1)

			feedBoundary := boundaryOf(t, withFeed)
			noFeedBoundary := boundaryOf(t, noFeed)
			sizeWithFeed := mustSize(t, feedPath)
			sizeWithoutFeed := mustSize(t, noFeedPath)
			if feedBoundary.UnterminatedTailBytes != 0 || feedBoundary.Bytes != sizeWithFeed {
				t.Errorf("the fragment-as-committed boundary = %+v, want a %d-byte file with nothing "+
					"unterminated", feedBoundary, sizeWithFeed)
			}
			if noFeedBoundary.Bytes != sizeWithoutFeed {
				t.Errorf("the fragment-without-its-line-feed boundary = %+v, want the file's own %d bytes",
					noFeedBoundary, sizeWithoutFeed)
			}
			// The offsets are the *provider's* own answer, and the boundary may state only what that answer
			// supports (the gate `SourceBoundaryRecordsClaimed` implements): a provider whose declarations
			// make its source a transcript of appended line records has the terminated records' end and the
			// unterminated tail named, and one whose declarations do not -- the Pi provider's -- says so,
			// with its reason, and states the byte fact alone. Either way the record is *read* by the
			// conversion, which is what this case is about and what the assertions below require.
			if got := noFeedBoundary.RecordOffsets; got != testCase.offsets {
				t.Errorf("the fragment-without-its-line-feed boundary claims %q, want %q for this provider's "+
					"own declarations", got, testCase.offsets)
			}
			if testCase.offsets == SourceBoundaryRecordsClaimed {
				if noFeedBoundary.TerminatedRecordsEndBytes != sizeWithoutFeed-wantTail ||
					noFeedBoundary.UnterminatedTailBytes != wantTail {
					t.Errorf("the fragment-without-its-line-feed boundary = %+v, want the terminated records "+
						"to end at byte %d with its last %d byte(s) unterminated",
						noFeedBoundary, sizeWithoutFeed-wantTail, wantTail)
				}
			} else if strings.TrimSpace(noFeedBoundary.RecordOffsetsDetail) == "" {
				t.Errorf("the boundary claims no record offset for this provider and carries no reason: %+v",
					noFeedBoundary)
			}
			// What the record is and what the conversion did with it: read in both cases, and written into
			// both target documents. A boundary that called the unterminated record unpublished while the
			// conversion carried it would be the defect this case exists for.
			if noFeed.Source == nil || withFeed.Source == nil {
				t.Fatalf("a conversion reported no source report: %+v / %+v", noFeed.Source, withFeed.Source)
			}
			if noFeed.Source.Counts.Messages != withFeed.Source.Counts.Messages ||
				noFeed.Conversion.Carried.Messages != withFeed.Conversion.Carried.Messages {
				t.Errorf("the record without its line feed read as %d message(s) and carried %d, against %d "+
					"and %d with it: both readers read a complete unterminated record",
					noFeed.Source.Counts.Messages, noFeed.Conversion.Carried.Messages,
					withFeed.Source.Counts.Messages, withFeed.Conversion.Carried.Messages)
			}
			if !strings.Contains(string(mustRead(t, noFeed.Installation.Path)), addedText) {
				t.Errorf("the installed session does not hold %q, the dialogue of the record the file had "+
					"not terminated; the read carries it", addedText)
			}
			if warning := strings.Join(noFeed.Warnings, " "); strings.Contains(warning, "not part of this conversion") {
				t.Errorf("warnings = %v, want no claim that the carried record was left out of the conversion",
					noFeed.Warnings)
			}
		})
	}
}

// TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets pins the record-offset gate's second half end
// to end: Codex reads its own compressed rollouts (internal/parser decompresses them and refuses any offset
// into the compressed bytes), so a conversion of one carries the decoded history while the boundary states
// the file's bytes and digest and claims no record offset for them. An offset found by scanning such a file
// for line feeds would be an arbitrary byte, and the bytes after it no record at all.
func TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets(t *testing.T) {
	fixture := fixturePath(t, filepath.Join("codex", "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"))
	rollout := "rollout-2026-05-04T09-15-00-" + codexFixtureRef.ID + ".jsonl"
	destination := filepath.Join("2026", "05", "04", rollout)

	plainCfg := fixtureConfig(t)
	plainCfg, _ = sourceFixtureCopy(t, plainCfg, parser.AgentCodex, fixture, destination)
	plainResult := continueRefInto(t, plainCfg, t.TempDir(), TargetClaude, codexFixtureRef)

	cfg := fixtureConfig(t)
	cfg, compressedPath := compressedCodexFixtureCopy(t, cfg, fixture,
		filepath.Join("2026", "05", "04", strings.TrimSuffix(rollout, ".jsonl")+".jsonl.zst"))
	before := mustRead(t, compressedPath)
	result := continueRefInto(t, cfg, t.TempDir(), TargetClaude, codexFixtureRef)

	if result.Conversion.ReadFrom != "source" {
		t.Fatalf("read_from = %q, want the compressed rollout read from its own file", result.Conversion.ReadFrom)
	}
	if result.Source == nil || plainResult.Source == nil {
		t.Fatalf("a conversion reported no source report: %+v / %+v", result.Source, plainResult.Source)
	}
	// The decoded history was carried: the same conversation the plain rollout of the same bytes produces.
	if result.Source.Counts.Messages != plainResult.Source.Counts.Messages ||
		result.Conversion.Carried.Messages != plainResult.Conversion.Carried.Messages {
		t.Errorf("the compressed rollout read %d message(s) and carried %d, the plain one %d and %d: the "+
			"reader decodes the compressed file, so the two must agree",
			result.Source.Counts.Messages, result.Conversion.Carried.Messages,
			plainResult.Source.Counts.Messages, plainResult.Conversion.Carried.Messages)
	}

	boundary := boundaryOf(t, result)
	if boundary.Bytes != int64(len(before)) || boundary.SHA256 != sha256Of(t, before) {
		t.Errorf("boundary = %+v, want the compressed file's own %d bytes and digest", boundary, len(before))
	}
	if boundary.RecordOffsets != SourceBoundaryRecordsNotClaimed ||
		boundary.RecordOffsetsDetail != recordOffsetsNotText {
		t.Errorf("boundary = %+v, want the record offsets withheld because the file's own bytes are a "+
			"container of the records (%s)", boundary, recordOffsetsNotText)
	}
	if boundary.TerminatedRecordsEndBytes != 0 || boundary.UnterminatedTailBytes != 0 {
		t.Errorf("boundary = %+v, want no record offset and no unterminated record claimed for a compressed "+
			"container", boundary)
	}
	// The file's own bytes are still what they are, and the read's own snapshot of them is the file: the two
	// answers the offsets are withheld *for* are the two a caller compares two conversions by.
	if boundary.ReadCoverage != SourceBoundaryCoverageSizeMatch || boundary.ReadBytes != int64(len(before)) {
		t.Errorf("boundary = %+v, want the read's own snapshot to be the file it measured", boundary)
	}
	if warning := strings.Join(result.Warnings, " "); strings.Contains(warning, "hold no line feed") ||
		strings.Contains(warning, "unfinished record") {
		t.Errorf("a compressed source warned about an unterminated record: %v", result.Warnings)
	}
	if result.Installation.SourceCheckState != SourceCheckUnchanged {
		t.Errorf("source check = %q (%s), want the source reported unchanged",
			result.Installation.SourceCheckState, result.Installation.Detail)
	}
	if after := mustRead(t, compressedPath); string(after) != string(before) {
		t.Errorf("the compressed source file changed while it was converted (%d bytes, was %d)",
			len(after), len(before))
	}
}

// TestSourceBoundaryOfFileStatesTheRecordOffsets pins the pass itself, byte by byte: a file whose last line
// is not terminated has its terminated records ending at the last line feed, a file with no line feed at all
// has no terminated record in it, and an empty file has neither. It also pins the two ways the offsets are
// withheld -- a provider that declares no such source, and a file whose own bytes cannot be that source
// (a compressed container of the records) -- because an offset found by scanning such a file for line feeds
// is not a record boundary and the bytes after it are not an unterminated record.
func TestSourceBoundaryOfFileStatesTheRecordOffsets(t *testing.T) {
	// A transcript whose multi-byte rune the pass's own 32 KiB read buffer cuts in half: the rune's lead
	// byte is the buffer's last byte, so only a validator that carries it to the next chunk reads this
	// file as the text it is.
	splitRune := "{\"x\":\"" + strings.Repeat("x", 32<<10-7) + "é\"}\n"
	for _, testCase := range []struct {
		name            string
		content         string
		recordsAreLines bool
		wantClaim       string
		wantDetail      string
		wantSize        int64
		wantRecordsEnd  int64
		wantTail        int64
	}{
		{
			name: "two complete lines", content: "{\"a\":1}\n{\"b\":2}\n", recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed, wantSize: 16, wantRecordsEnd: 16, wantTail: 0,
		},
		{
			name: "an unterminated last line", content: "{\"a\":1}\n{\"b\":", recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed, wantSize: 13, wantRecordsEnd: 8, wantTail: 5,
		},
		{
			name: "a complete record without its line feed", content: "{\"a\":1}\n{\"b\":2}", recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed, wantSize: 15, wantRecordsEnd: 8, wantTail: 7,
		},
		{
			name: "no line feed at all", content: "{\"a\":1}", recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed, wantSize: 7, wantRecordsEnd: 0, wantTail: 7,
		},
		{
			name: "an empty file", content: "", recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed,
		},
		{
			name: "a rune the read buffer splits", content: splitRune, recordsAreLines: true,
			wantClaim: SourceBoundaryRecordsClaimed,
			wantSize:  int64(len(splitRune)), wantRecordsEnd: int64(len(splitRune)),
		},
		{
			// A provider that declares no appended-line source: the file's own bytes are still measured,
			// and no record boundary is claimed for them.
			name: "a source that is not declared line-recorded", content: "{\"a\":1}\n{\"b\":",
			wantClaim: SourceBoundaryRecordsNotClaimed, wantDetail: recordOffsetsNotDeclared, wantSize: 13,
		},
		{
			// The representation half of the gate: a zstd frame's own header is not text, so this file
			// cannot be a transcript of lines whatever its provider declares (a Codex .jsonl.zst rollout).
			name:            "a compressed container of the records",
			content:         string([]byte{0x28, 0xB5, 0x2F, 0xFD, 0x00, 0x0A, 0x24, 0x00}) + "{\"a\":1}\n",
			recordsAreLines: true,
			wantClaim:       SourceBoundaryRecordsNotClaimed, wantDetail: recordOffsetsNotText, wantSize: 16,
		},
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
			if boundary.RecordOffsets != testCase.wantClaim {
				t.Errorf("record_offsets = %q, want %q", boundary.RecordOffsets, testCase.wantClaim)
			}
			if boundary.RecordOffsetsDetail != testCase.wantDetail {
				t.Errorf("record_offsets_detail = %q, want %q", boundary.RecordOffsetsDetail, testCase.wantDetail)
			}
			if boundary.TerminatedRecordsEndBytes != testCase.wantRecordsEnd {
				t.Errorf("terminated_records_end = %d, want %d",
					boundary.TerminatedRecordsEndBytes, testCase.wantRecordsEnd)
			}
			if boundary.UnterminatedTailBytes != testCase.wantTail {
				t.Errorf("unterminated tail = %d, want %d",
					boundary.UnterminatedTailBytes, testCase.wantTail)
			}
			// The pass answers nothing about the read: those two answers are filled from the read's own
			// record of the file it opened (takeSourceBoundary), never by a file scan.
			if boundary.ReadBytes != 0 || boundary.ReadCoverage != "" || boundary.ReadCoverageDetail != "" {
				t.Errorf("boundary = %+v, want no read-coverage claim from the file's bytes alone", boundary)
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

// TestTextStreamCarriesSplitRunes pins the validator the record-offset gate's second half rests on: bytes
// are text until a sequence that no rune can complete them to, a rune a chunk boundary cuts in half is
// decided by the chunk that completes it rather than by the one that begins it, and a file that ends inside
// a rune is not text at all.
func TestTextStreamCarriesSplitRunes(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		chunks []string
		want   bool
	}{
		{"ascii", []string{"{\"a\":1}\n", "{\"b\":2}\n"}, true},
		{"a rune split across chunks", []string{"{\"a\":\"é", "\"}"}, true},
		{"a four-byte rune split across chunks", []string{"a\U0001F600", "b"}, true},
		{"a rune completed one byte at a time", []string{"a", "\xF0\x9F", "\x98", "\x80"}, true},
		{"an encoded rune error", []string{"\uFFFD"}, true},
		{"a stray continuation byte", []string{"{\"a\":1}", "\x80"}, false},
		{"a zstd frame header", []string{"\x28\xB5\x2F\xFD"}, false},
		{"a rune no chunk completes", []string{"\xE2\x82", "x"}, false},
		{"a four-byte rune cut to three bytes", []string{"\xF0\x9F\x98", "x"}, false},
		{"a file that ends inside a rune", []string{"a", "\xF0\x9F\x98"}, false},
		{"a file that ends inside a two-byte rune", []string{"é"[0:1]}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stream := textStream{valid: true}
			for _, chunk := range testCase.chunks {
				stream.write([]byte(chunk))
			}
			// The pass says so too, once the file has no more bytes (sourceBoundaryOfFile).
			stream.done()
			if stream.valid != testCase.want {
				t.Errorf("text = %v, want %v", stream.valid, testCase.want)
			}
		})
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

// TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead pins the state a mid-read growth leaves behind,
// at the seam where it is visible: the read's own record of the file it opened is smaller than the file by
// the time the boundary pass measures it, so the boundary states the file's bytes and says the conversion's
// coverage of them is not established -- instead of attributing the conversation to bytes it may never have
// read while the source comparison reports the file unchanged.
//
// A conversion cannot reach this state end to end without racing its own read (the parse runs inside
// Continue, and a case that appended to the file while it ran would be a flake rather than a case), so the
// state is produced here exactly as the race leaves it, and the end-to-end half is the repeated-conversion
// case's assertion that a read which did keep up reports `read_coverage: size-match` with `read_bytes` equal
// to the file's size.
func TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	// What the read recorded for the file when it opened it, and the file the boundary pass then measures:
	// the second half of the file was appended while the read was reading it.
	const readBytes = 8
	whole := "{\"a\":1}\n{\"b\":2}\n"
	if err := os.WriteFile(path, []byte(whole), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	conversation := Conversation{Source: SourceIdentity{
		Provider:   "claude",
		SourcePath: path,
		FileSize:   readBytes,
	}}

	boundary, err := takeSourceBoundary(NewSourceReader(fixtureConfig(t)), conversation, "source")
	if err != nil {
		t.Fatalf("taking the boundary: %v", err)
	}
	if boundary.Bytes != int64(len(whole)) || boundary.SHA256 != sha256Of(t, mustRead(t, path)) {
		t.Errorf("boundary = %+v, want the file's own %d bytes and digest", boundary, len(whole))
	}
	if boundary.ReadBytes != readBytes || boundary.ReadCoverage != SourceBoundaryCoverageSizeMismatch {
		t.Errorf("boundary = %+v, want the read's own snapshot (%d byte(s)) reported as one this file is not",
			boundary, readBytes)
	}
	for _, want := range []string{"8 byte(s)", "the file holds 16", "not established"} {
		if !strings.Contains(boundary.ReadCoverageDetail, want) {
			t.Errorf("read_coverage_detail = %q, want it to name %q", boundary.ReadCoverageDetail, want)
		}
	}
	// The offsets stay what they are -- byte facts about the file this block reports -- and the coverage
	// state is what says the conversion is not shown to have covered them.
	if boundary.RecordOffsets != SourceBoundaryRecordsClaimed ||
		boundary.TerminatedRecordsEndBytes != int64(len(whole)) || boundary.UnterminatedTailBytes != 0 {
		t.Errorf("boundary = %+v, want the file's own record structure stated beside the coverage answer",
			boundary)
	}
	warnings := boundaryWarnings(&boundary)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one naming the file that moved while it was read", warnings)
	}
	for _, want := range []string{"8 byte(s) read", "16 byte(s) now", "not established", "converting again"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("the warning %q does not name %q", warnings[0], want)
		}
	}
	// The pass reads the source and writes nothing to it.
	if got := string(mustRead(t, path)); got != whole {
		t.Errorf("the source file is now %q, want the bytes it had (%q)", got, whole)
	}
}

// TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity pins the limit of the read-coverage answer, which
// is the state a file replaced by *different bytes of the same length* after the read leaves behind. The
// read's own record holds a size and no digest of the bytes it opened, so this pass measures the
// replacement and can compare one number with another: the sizes agree and the honest answer is a size
// match -- the file is the size the read recorded -- beside the digest of the file as it stands. What the
// report must never do is turn that agreement into a byte-identity claim (the pre-fix block said "the
// conversion read these bytes" and named the state `verified`), because nothing the run holds can tell the
// replacement from the bytes the read opened.
//
// The state is produced at the pass's own seam, as its moved-file sibling is: the replacement happens
// inside a conversion between the read and this pass, so a case that raced `Continue`'s own read would be
// a flake rather than a case.
func TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	// What the read opened, and what the file holds when the boundary pass measures it: another transcript
	// of exactly the same length, so no size comparison can tell them apart.
	read := "{\"a\":1}\n{\"b\":2}\n"
	replacement := "{\"a\":1}\n{\"c\":3}\n"
	if len(read) != len(replacement) {
		t.Fatalf("the case's own premise is broken: %d byte(s) read, %d byte(s) replacing them",
			len(read), len(replacement))
	}
	if err := os.WriteFile(path, []byte(replacement), 0o600); err != nil {
		t.Fatalf("writing the replacement: %v", err)
	}
	conversation := Conversation{Source: SourceIdentity{
		Provider:   "claude",
		SourcePath: path,
		FileSize:   int64(len(read)),
	}}

	boundary, err := takeSourceBoundary(NewSourceReader(fixtureConfig(t)), conversation, "source")
	if err != nil {
		t.Fatalf("taking the boundary: %v", err)
	}
	if boundary.ReadCoverage != SourceBoundaryCoverageSizeMatch {
		t.Errorf("read_coverage = %q, want %q: equal sizes are a size match, which is all the read's own "+
			"record can establish", boundary.ReadCoverage, SourceBoundaryCoverageSizeMatch)
	}
	if boundary.ReadBytes != int64(len(replacement)) || boundary.Bytes != int64(len(replacement)) {
		t.Errorf("boundary = %+v, want both sizes reported as the %d byte(s) each is",
			boundary, len(replacement))
	}
	// The digest is the file's own as it stands: it is the file the report names, and it is what two
	// conversions of one source are compared by. It is *not* the bytes the read opened, which no digest here
	// holds.
	if boundary.SHA256 != sha256Of(t, []byte(replacement)) {
		t.Errorf("boundary.sha256 = %q, want the file's own digest (%q): the block reports the bytes it "+
			"measured, whatever the read held", boundary.SHA256, sha256Of(t, []byte(replacement)))
	}
	for _, want := range []string{
		"16 byte(s)", "the file is the size the read recorded", "not a byte-identity check",
		"a same-length replacement", "not ruled out",
	} {
		if !strings.Contains(boundary.ReadCoverageDetail, want) {
			t.Errorf("read_coverage_detail = %q, want it to name %q", boundary.ReadCoverageDetail, want)
		}
	}
	// And the answer claims no move either: a size that matched is not evidence the file moved, so the run
	// warns of nothing here -- a warning would state a change nobody measured.
	if warnings := boundaryWarnings(&boundary); len(warnings) != 0 {
		t.Errorf("a size match warned: %v", warnings)
	}
	// The claim a size cannot support is also kept out by its own spelling: the pre-fix block answered equal
	// sizes with `read_coverage: verified` and the words "the conversion read these bytes", which the
	// same-length replacement above would have made false. Both are literal here, so this half is red
	// against the pre-fix code whatever the states are named.
	if boundary.ReadCoverage == "verified" || strings.Contains(boundary.ReadCoverageDetail, "read these bytes") {
		t.Errorf("boundary = %+v, want no byte-identity claim from a size comparison", boundary)
	}
}

// TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile pins the warning half: a tail nobody measured
// and a file nobody compared are not warned about, a measured tail is named with both offsets and the
// remedy the goal states, and a file that moved while it was read is named in the same channel -- the one a
// caller reading only the warnings must not have to infer from a field.
func TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile(t *testing.T) {
	if warnings := boundaryWarnings(nil); len(warnings) != 0 {
		t.Errorf("a report with no boundary warned: %v", warnings)
	}
	notTaken := &SourceBoundary{State: SourceBoundaryNotTaken, Detail: "an archive read"}
	if warnings := boundaryWarnings(notTaken); len(warnings) != 0 {
		t.Errorf("a boundary that was not taken warned: %v", warnings)
	}
	complete := &SourceBoundary{
		State: SourceBoundaryTaken, Bytes: 1792, TerminatedRecordsEndBytes: 1792,
		ReadBytes: 1792, ReadCoverage: SourceBoundaryCoverageSizeMatch,
	}
	if warnings := boundaryWarnings(complete); len(warnings) != 0 {
		t.Errorf("a file with nothing unterminated and no move warned: %v", warnings)
	}

	warnings := boundaryWarnings(&SourceBoundary{
		State: SourceBoundaryTaken, Bytes: 1835, TerminatedRecordsEndBytes: 1792,
		UnterminatedTailBytes: 43,
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one", warnings)
	}
	for _, want := range []string{"hold no line feed", "1835", "1792", "43 byte(s)", "converting again"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("the warning %q does not name %q", warnings[0], want)
		}
	}
	for _, forbidden := range []string{"not part of this conversion", "unpublished"} {
		if strings.Contains(warnings[0], forbidden) {
			t.Errorf("the warning %q claims the record was left out of the conversion (%q)",
				warnings[0], forbidden)
		}
	}

	// The read's own snapshot disagrees with the file it measured: the run stopped covering bytes it can
	// still name, and the channel a caller skims must say so.
	moved := boundaryWarnings(&SourceBoundary{
		State: SourceBoundaryTaken, Bytes: 1835, ReadBytes: 1792,
		ReadCoverage: SourceBoundaryCoverageSizeMismatch, TerminatedRecordsEndBytes: 1835,
	})
	if len(moved) != 1 {
		t.Fatalf("warnings = %v, want one for the file that moved while it was read", moved)
	}
	for _, want := range []string{"1792 byte(s) read", "1835 byte(s) now", "not established", "converting again"} {
		if !strings.Contains(moved[0], want) {
			t.Errorf("the warning %q does not name %q", moved[0], want)
		}
	}

	// Both answers at once: the file moved and its new bytes end in an unterminated record.
	both := boundaryWarnings(&SourceBoundary{
		State: SourceBoundaryTaken, Bytes: 1835, ReadBytes: 1792,
		ReadCoverage:              SourceBoundaryCoverageSizeMismatch,
		TerminatedRecordsEndBytes: 1792, UnterminatedTailBytes: 43,
	})
	if len(both) != 2 {
		t.Errorf("warnings = %v, want one for each answer", both)
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
	if boundary.RecordOffsets != SourceBoundaryRecordsNotClaimed ||
		boundary.RecordOffsetsDetail != recordOffsetsNotDeclared {
		t.Errorf("boundary = %+v, want no record boundary claimed: the source is one JSON document, not a "+
			"transcript of appended line records", boundary)
	}
	if boundary.Bytes != int64(len(document)) || boundary.SHA256 != sha256Of(t, mustRead(t, sourcePath)) {
		t.Errorf("boundary = %+v, want the file's own %d bytes and digest", boundary, len(document))
	}
	if boundary.TerminatedRecordsEndBytes != 0 || boundary.UnterminatedTailBytes != 0 {
		t.Errorf("boundary = %+v, want no record offset and no unterminated record claimed", boundary)
	}
	if warning := strings.Join(result.Warnings, " "); strings.Contains(warning, "hold no line feed") {
		t.Errorf("the conversion warned about an unterminated record in a complete file: %v", result.Warnings)
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

// mustSize answers one file's size. It reads the file rather than trusting a run's own report of it.
func mustSize(t *testing.T, path string) int64 {
	t.Helper()
	return int64(len(mustRead(t, path)))
}

// appendGrowthBytes appends exact bytes to a session file, for the case whose growth is a committed fragment
// with something removed from it (the fragment's own last byte, when a case needs a record the file has not
// terminated). It answers the grown file's size.
func appendGrowthBytes(t *testing.T, path string, data []byte) int64 {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening %s to grow it: %v", path, err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		t.Fatalf("appending %d byte(s) to %s: %v", len(data), path, err)
	}
	return mustSize(t, path)
}

// compressedCodexFixtureCopy writes one Codex rollout's bytes compressed, under a provider root of its own
// -- the representation Codex leaves behind for a rollout it has not appended to in a week, and the one its
// own reader decodes (internal/parser's codex_rollout_open.go) -- and answers the configuration together
// with the compressed file's path. The committed fixture never moves.
func compressedCodexFixtureCopy(
	t *testing.T, cfg config.Config, fixture, destination string,
) (config.Config, string) {
	t.Helper()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("building the zstd encoder: %v", err)
	}
	compressed := encoder.EncodeAll(mustRead(t, fixture), nil)
	if err := encoder.Close(); err != nil {
		t.Fatalf("closing the zstd encoder: %v", err)
	}
	root := t.TempDir()
	copyPath := filepath.Join(root, destination)
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
		t.Fatalf("creating the compressed rollout's directory: %v", err)
	}
	if err := os.WriteFile(copyPath, compressed, 0o600); err != nil {
		t.Fatalf("writing the compressed rollout: %v", err)
	}
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentCodex: {root}}
	return cfg, copyPath
}

// sha256Of is an independent digest of bytes already read -- crypto/sha256 here rather than through the
// code the report's own boundary uses, so a case that checks a reported digest is not checking the
// function that produced it -- for comparing a file's content before and after another run.
func sha256Of(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fragmentText answers the dialogue a growth fragment's *last* record holds -- the record a case that
// removes the fragment's final line feed leaves unterminated -- which is what a case searches a written
// document for: a fragment is a byte prefix of a record, so the field is read as its own value up to the
// next quote (an unterminated record loses its closing quote with the rest of the record), never by parsing
// the fragment as a record.
func fragmentText(t *testing.T, name string) string {
	t.Helper()
	data := string(growthFragment(t, name))
	const marker = `"text":"`
	start := strings.LastIndex(data, marker)
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
