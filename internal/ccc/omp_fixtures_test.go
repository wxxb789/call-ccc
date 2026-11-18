package ccc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// ompFixturePath answers one committed OMP fixture's path. An OMP session is filed one project directory
// below its root (measured: the provider's own discovery requires that level for this agent, while the Pi
// provider accepts a file at the root), and the directory name is the absolute `--…--` spelling, which is
// the one branch of OMP's own rule that does not depend on the machine the fixtures were generated on.
func ompFixturePath(t *testing.T, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(fixtureRoot(t), "omp", "*", id+".jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one committed OMP fixture for %s, found %v (%v)", id, matches, err)
	}
	return matches[0]
}

// TestOMPFixtureCarriesTheHabitssOwnRecords pins the OMP-specific shapes a conversion must handle, from the
// fixture's own bytes and from what the inherited reader makes of them:
//
//   - the file opens with the harness's fixed-width title slot line (256 bytes, line feed included), which
//     is the one line of the file that is not an entry of the conversation's tree;
//   - the session header is the *second* line, so a reader that took the first line for the header would
//     report no session at all;
//   - the harness's `custom` notices are not turns: the read counts them as no message (the history
//     fixture's exit notice is the case below);
//   - the binding the harness restores a session's model from is one `<provider>/<id>` string.
func TestOMPFixtureCarriesTheHabitssOwnRecords(t *testing.T) {
	data, err := os.ReadFile(ompFixturePath(t, ompFixtureRef.ID))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var slot struct {
		Type      string `json:"type"`
		Version   int    `json:"v"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updatedAt"`
		Pad       string `json:"pad"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &slot); err != nil {
		t.Fatalf("the fixture's first line is not one JSON object: %v", err)
	}
	if slot.Type != piEntryTitle || slot.Version != 1 {
		t.Fatalf("the first line = %+v, want the harness's title slot", slot)
	}
	if got := len(lines[0]) + 1; got != ompTitleSlotWidth {
		t.Errorf("the title slot line is %d bytes including its line feed, want %d", got, ompTitleSlotWidth)
	}
	if strings.TrimSpace(slot.Pad) != "" {
		t.Errorf("the slot's pad holds %q, want spaces only", slot.Pad)
	}
	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &header); err != nil {
		t.Fatalf("the fixture's second line is not one JSON object: %v", err)
	}
	if header.Type != piEntrySession || header.ID != ompFixtureRef.ID {
		t.Errorf("line 2 = %+v, want the session header naming %s", header, ompFixtureRef.ID)
	}
	var binding struct {
		Type  string `json:"type"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &binding); err != nil {
		t.Fatalf("the fixture's third line is not one JSON object: %v", err)
	}
	if binding.Type != piEntryModelChange || !strings.Contains(binding.Model, "/") {
		t.Errorf("line 3 = %+v, want the session's model binding as one `<provider>/<id>` string", binding)
	}

	conversation := readFixtureConversation(t, ompFixtureRef)
	if conversation.Source.Provider != string(parser.AgentOMP) || conversation.Source.Agent != "OhMyPi" {
		t.Errorf("identity = %+v, want the OMP session", conversation.Source)
	}
	if conversation.Source.SessionID != "omp:"+ompFixtureRef.ID {
		t.Errorf("session id = %q, want the header's own id %q", conversation.Source.SessionID, "omp:"+ompFixtureRef.ID)
	}
	if conversation.Source.Cwd != "/home/user/code/example-api" {
		t.Errorf("cwd = %q, want the header's recorded working directory", conversation.Source.Cwd)
	}
	counts := Summarize(conversation.Messages)
	want := Counts{Messages: 5, UserMessages: 2, AssistantMessages: 3, ToolCalls: 1, ToolResults: 1}
	if counts != want {
		t.Errorf("counts = %+v, want %+v (the harness's `custom` notices are not turns)", counts, want)
	}
	for _, message := range conversation.Messages {
		if message.IsSystem {
			t.Errorf("the read holds a system record (%+v), and this fixture records none", message)
		}
	}
	// The fixture's own words, projected out of its first answer's content blocks: the assertion is about
	// the text a read can lose rather than about a shape, and the fixture's dialogue is asserted nowhere
	// else. A read that answered an empty dialogue, or carried none, fails here.
	if message := conversation.Messages[1]; !message.DialogueProjected ||
		message.Dialogue != "I will look for the retry configuration." {
		t.Errorf("the read holds %q (projected %v) as the fixture's first answer, want its own words (%+v)",
			message.Dialogue, message.DialogueProjected, message)
	}
}

// TestOMPHistoryFixtureKeepsTheHabitssNoticeOffTheConversation pins the one OMP-specific source case this
// increment adds to the shared leaf rule: the harness's own `custom` notice stands on the file's leaf path
// (a run writes it there), the read does not carry the kind, and the harness's own documentation places it
// outside the model context -- so the conversion accounts for it rather than refusing the session. The
// fixture's own last entry is that notice, and the read's last record is the turn it hangs off.
func TestOMPHistoryFixtureKeepsTheHabitssNoticeOffTheConversation(t *testing.T) {
	data, err := os.ReadFile(ompFixturePath(t, ompHistoryFixtureRef.ID))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	type entry struct {
		Type       string  `json:"type"`
		ID         string  `json:"id"`
		ParentID   *string `json:"parentId"`
		CustomType string  `json:"customType"`
		Message    struct {
			Role string `json:"role"`
		} `json:"message"`
	}
	var entries []entry
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded := entry{}
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("decoding a fixture line: %v", err)
		}
		if decoded.Type == "session" || decoded.Type == piEntryTitle {
			continue
		}
		entries = append(entries, decoded)
	}
	last := entries[len(entries)-1]
	if last.Type != "custom" || last.CustomType == "" {
		t.Fatalf("the fixture's last entry = %+v, want the harness's own notice", last)
	}
	if last.ParentID == nil {
		t.Fatal("the fixture's notice carries no parent, so it stands on no leaf path")
	}

	conversation := readFixtureConversation(t, ompHistoryFixtureRef)
	counts := Summarize(conversation.Messages)
	// The compaction boundary is the one system record the read holds (the inherited parser projects it as
	// an assistant record flagged as a boundary), and the notice is not a message at all.
	if counts.Messages != 6 || counts.CompactBoundaries != 1 || counts.SystemRecords != 1 {
		t.Errorf("counts = %+v, want the boundary and no message for the notice", counts)
	}
	deepest := conversation.Messages[len(conversation.Messages)-1]
	if deepest.SourceUUID != *last.ParentID {
		t.Errorf("the read's last record is %q, want the entry the notice hangs off (%q): the notice itself "+
			"is not a turn and the read cannot carry it", deepest.SourceUUID, *last.ParentID)
	}

	session, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting the fixture: %v", err)
	}
	// The converted file may hold no `custom` record at all, and its only `title` record is the file's
	// first line.
	converted := ompEntries(t, session.Data)
	for index, entry := range converted {
		if entry.Type == "custom" || (entry.Type == piEntryTitle && index != 0) {
			t.Errorf("the converted session holds a %s record the read never carried", entry.Type)
		}
	}
	// The one title slot is the file's own first line and the format's own record, not a claim about a
	// notice the source held.
	if converted[0].Type != piEntryTitle {
		t.Errorf("the converted session's first line = %q, want the title slot", converted[0].Type)
	}
}

// TestOMPBranchFixtureIsRefusedRatherThanResumedWrongly pins the source-side leaf rule for this family
// member through OMP's own fixture: the file's last entry is the `/tree` switch's summary, which the
// harness's own loader builds a message from and the read does not carry, so the file's selected leaf is
// not the record the read's last message stands for -- and the conversion refuses rather than writing a
// session that resumes the branch the summary switched away from.
//
// The refusal comes from the conversion pipeline, not the writer: the file-side half of the rule needs the
// source's own bytes, which the route reads (piSourceLeafCheck), while the writer sees only the document.
func TestOMPBranchFixtureIsRefusedRatherThanResumedWrongly(t *testing.T) {
	_, err := Continue(context.Background(), ContinueOptions{
		Ref: ompBranchFixtureRef, Target: TargetOMP, Config: fixtureConfig(t),
		TargetHome: t.TempDir(), TargetBinary: missingTargetBinary, Verify: VerifyNone,
	})
	if err == nil {
		t.Fatal("a source whose own loader selects a different leaf was converted")
	}
	if !errors.Is(err, ErrUnrepresentableLeaf) {
		t.Fatalf("error = %v, want the family's own refusal (%v) with the source file's answer beside it",
			err, ErrUnrepresentableLeaf)
	}
	for _, want := range []string{"selects the leaf", "branch_summary", "does not carry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err, want)
		}
	}
}

// TestOMPFixturesAreReadThroughTheSameModelAsEveryOtherSource pins the source direction's shape at the
// reader level: an OMP session reaches the pipeline through the inherited reader and the shared document,
// with no OMP-specific branch, and the same document converts every target through the same writers.
func TestOMPFixturesAreReadThroughTheSameModelAsEveryOtherSource(t *testing.T) {
	reader := NewSourceReader(fixtureConfig(t))
	conversation, err := reader.Read(context.Background(), ompFixtureRef)
	if err != nil {
		t.Fatalf("reading the OMP fixture: %v", err)
	}
	if conversation.Fidelity.ReadFrom != "source" {
		t.Errorf("read_from = %q, want the source file", conversation.Fidelity.ReadFrom)
	}
	for _, target := range Targets() {
		if target == TargetOpenCode {
			// This target's installation *is* the harness's own importer -- a session is a row in the
			// harness's database and there is no file this build may write for it -- so a run with the
			// harness missing stops at the installation naming the program it needs, and the writer is
			// exercised directly here. What the pipeline does when it is missing is pinned by
			// TestOpenCodeInstallNeedsTheHarness in continue_opencode_test.go.
			session, err := ConvertToOpenCodeSession(conversation, OpenCodeTargetOptions{
				CLIVersion: OpenCodeFormatVersion,
				Now:        time.UnixMilli(1_800_000_000_000).UTC(),
			})
			if err != nil {
				t.Fatalf("converting into %s: %v", target, err)
			}
			if session.Carried.Messages != 4 || session.Carried.ToolCalls != 1 ||
				session.Carried.ToolResults != 1 {
				t.Errorf("%s: carried %+v, want the fixture's four messages and its call/result pair",
					target, session.Carried)
			}
			continue
		}
		result, err := Continue(context.Background(), ContinueOptions{
			Ref: ompFixtureRef, Target: target, Config: fixtureConfig(t),
			TargetHome: t.TempDir(), TargetBinary: missingTargetBinary, Verify: VerifyNone,
		})
		if err != nil {
			t.Fatalf("continuing into %s: %v", target, err)
		}
		if result.Conversion.State != ConversionStateConverted {
			t.Errorf("%s: conversion = %+v, want the session converted", target, result.Conversion)
		}
		// The read's own records include the tool-result record, which no writer writes as a message of
		// its own (the family pairs a result onto the call's entry), so the message count is the read's
		// records that carry dialogue or calls -- and the call and its result travel beside them.
		if result.Conversion.Carried.Messages != 4 || result.Conversion.Carried.ToolCalls != 1 ||
			result.Conversion.Carried.ToolResults != 1 {
			t.Errorf("%s: carried %+v, want the fixture's four messages and its call/result pair",
				target, result.Conversion.Carried)
		}
	}
}
