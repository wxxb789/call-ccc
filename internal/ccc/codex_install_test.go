package ccc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// TestCodexRoundTripReportNamesDifferencesWithoutTheirText pins what a difference report is allowed to
// hold. The report is serialized into the CLI's JSON document and printed as a warning, and the items it
// compares are dialogue, tool arguments and tool outputs -- so a difference names the item (its kind,
// the call it belongs to, the tool it names) and identifies its text by size and digest, and no byte of
// that text reaches the report. The texts below stand in for the private content a real session holds.
//
// The case is the failure path on purpose: the successful path was the only one carrying a no-content
// assertion, and a report that quotes the very text it could not match is the one place this rule is
// easiest to lose.
func TestCodexRoundTripReportNamesDifferencesWithoutTheirText(t *testing.T) {
	const (
		writtenDialogue = "summarize the retry policy in internal/retry/policy.go"
		readDialogue    = "summarize the retry policy in internal/retry/client.go"
		writtenInput    = `{"command":"cat ~/.config/example-api/credentials.json"}`
		readInput       = `{"command":"cat ~/.config/example-api/tokens.json"}`
		writtenOutput   = "the credential file's own private contents"
		readOutput      = "another private tool output, of the same shape"
	)
	rollout := CodexRollout{Items: []TargetItem{
		{Kind: ItemUser, Text: writtenDialogue},
		{Kind: ItemToolCall, CallID: "toolu_01Private", ToolName: "Bash", InputJSON: writtenInput},
		{Kind: ItemToolResult, CallID: "toolu_01Private", Text: writtenOutput},
	}}
	readBack := Conversation{Messages: []Message{
		{Role: string(parser.RoleUser), Content: readDialogue},
		{
			Role:      string(parser.RoleAssistant),
			ToolCalls: []ToolCall{{ToolUseID: "toolu_01Private", Name: "Bash", InputJSON: readInput}},
			ToolResults: []ToolResult{
				{ToolUseID: "toolu_01Private", ContentRaw: readOutput, ContentDecoded: true},
			},
		},
	}}

	roundTrip := compareCodexRoundTrip(rollout, readBack, nil)
	if len(roundTrip.differences) != 3 {
		t.Fatalf("differences = %q, want one per altered item", roundTrip.differences)
	}
	report := strings.Join(roundTrip.differences, "; ")
	for _, content := range []string{writtenDialogue, readDialogue, writtenInput, readInput,
		writtenOutput, readOutput} {
		if strings.Contains(report, content) {
			t.Errorf("the difference report carries the source content it compared: %q\nreport: %s",
				content, report)
		}
	}
	// What identifies an item without being any of its text: the category, the ordinal, the call and the
	// tool it belongs to, and the size and digest of the text itself.
	for _, want := range []string{
		"dialogue 1 differs",
		"user_message (" + describeText(writtenDialogue) + ")",
		"tool call 1 differs",
		"call toolu_01Private to Bash (" + describeText(writtenInput) + ")",
		"tool result 1 differs",
		"call toolu_01Private's result (" + describeText(writtenOutput) + ")",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("the difference report does not name %q:\n%s", want, report)
		}
	}

	// A digest is only useful if it distinguishes the two texts being compared: the report carries the
	// digest of the side each item came from, not one digest for the pair.
	if strings.Count(report, describeText(readOutput)) != 1 {
		t.Errorf("the read-back side's own digest is not named exactly once:\n%s", report)
	}
	// And it has to be that text's own digest, in the format the report documents, so a reader outside
	// this program can hash the same words and match the line. Spelled here rather than asked of
	// describeText, which is what the assertion above already used.
	sum := sha256.Sum256([]byte(readDialogue))
	if want := fmt.Sprintf("%d bytes, sha256 %x", len(readDialogue), sum[:]); !strings.Contains(report, want) {
		t.Errorf("the read-back dialogue is not identified by its own SHA-256 (%q):\n%s", want, report)
	}
}

// TestCodexRoundTripAssertsTheOwningCallOfEachResult pins the owner half of the round-trip comparison: an
// id two written call occurrences share cannot say which invocation an output answers, so a result item
// states the place of the call item it was written for (TargetItem.OwnerCall) and the comparison asserts
// the attachment the inherited reader makes -- the call occurrence its own order resolves the output onto
// (the last call stated under the id). The document below is that shape: calls A and B share an id, the
// recorded output answers B, and the inferred output answers A; the read-back is the file's own reading,
// where the Codex sink folds both outputs onto B, the last call registered for the id. The comparison
// reports the reassignment and names the invocation the output was written for -- without carrying the
// evidence itself -- while a read-back that attaches each output to the occurrence it was written for
// holds no difference at all: the comparison is about the attachment, not about the id.
//
// The occurrence is not the call's fields (the thirteenth round's finding): an id two occurrences share
// can hold two invocations whose id, tool and *arguments* are equal, and then only the occurrence says
// which of them an output answers. The same shape with both invocations' arguments equal is compared
// below: keyed by the call's fields alone, moving an output from one occurrence to the other leaves every
// result key unchanged and the comparison silent, so the owner key carries the invocation's occurrence
// beside its fields and the reassignment is reported -- while the attached read-back, each output on the
// occurrence it was written for, still holds no difference.
func TestCodexRoundTripAssertsTheOwningCallOfEachResult(t *testing.T) {
	const (
		reusedID    = "toolu_reused"
		firstInput  = `{"command":"a"}`
		secondInput = `{"command":"b"}`
		lateOutput  = "the inferred output"
		earlyOutput = "the recorded output"
	)
	// The call items stand at indexes 1 and 3 of the list, so each result item's stated owner is the
	// occurrence the writer wrote it for: the recorded output for B, the inferred one for A.
	callA, callB := 1, 3
	// shape builds the document and the two read-backs the arms below share: the document as the writer
	// states it (the two repetitions of one id with the given arguments), the folded read-back where the
	// Codex sink attaches both outputs to the last call registered for the id, and the attached read-back
	// where each output stands on the occurrence it was written for.
	shape := func(first, second string) (CodexRollout, Conversation, Conversation) {
		rollout := CodexRollout{Items: []TargetItem{
			{Kind: ItemAssistant, Text: "the two calls"},
			{Kind: ItemToolCall, CallID: reusedID, ToolName: "Bash", InputJSON: first},
			{Kind: ItemAssistant, Text: "the calls' outcomes"},
			{Kind: ItemToolCall, CallID: reusedID, ToolName: "Bash", InputJSON: second},
			{Kind: ItemToolResult, CallID: reusedID, Text: earlyOutput, OwnerCall: &callB},
			{Kind: ItemToolResult, CallID: reusedID, Text: lateOutput, OwnerCall: &callA},
		}}
		folded := Conversation{Messages: []Message{
			{Role: string(parser.RoleAssistant), Content: "the two calls"},
			{Role: string(parser.RoleAssistant),
				ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: first}}},
			{Role: string(parser.RoleAssistant), Content: "the calls' outcomes"},
			{Role: string(parser.RoleAssistant),
				ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: second}},
				ToolResults: []ToolResult{
					{ToolUseID: reusedID, ContentRaw: earlyOutput, ContentDecoded: true},
					{ToolUseID: reusedID, ContentRaw: lateOutput, ContentDecoded: true},
				}},
		}}
		attached := Conversation{Messages: []Message{
			{Role: string(parser.RoleAssistant), Content: "the two calls"},
			{Role: string(parser.RoleAssistant),
				ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: first}},
				ToolResults: []ToolResult{
					{ToolUseID: reusedID, ContentRaw: lateOutput, ContentDecoded: true},
				}},
			{Role: string(parser.RoleAssistant), Content: "the calls' outcomes"},
			{Role: string(parser.RoleAssistant),
				ToolCalls: []ToolCall{{ToolUseID: reusedID, Name: "Bash", InputJSON: second}},
				ToolResults: []ToolResult{
					{ToolUseID: reusedID, ContentRaw: earlyOutput, ContentDecoded: true},
				}},
		}}
		return rollout, folded, attached
	}
	noCallOrResultDifference := func(t *testing.T, arm string, differences []string) {
		t.Helper()
		for _, difference := range differences {
			if strings.Contains(difference, categoryToolResult) || strings.Contains(difference, categoryToolCall) {
				t.Errorf("%s: difference = %q, want none about the calls or their results: each output "+
					"is read back on the occurrence it was written for", arm, difference)
			}
		}
	}
	// The hand-built read-backs carry no installed file bytes, and the dialogue walk resolves a written
	// message against the file's own records (with none, every written message is reported there). The
	// arms are about the result evidence, so their difference counts name the calls and results alone.
	callOrResultDifferences := func(differences []string) int {
		count := 0
		for _, difference := range differences {
			if strings.Contains(difference, categoryToolCall) || strings.Contains(difference, categoryToolResult) {
				count++
			}
		}
		return count
	}

	rollout, folded, attached := shape(firstInput, secondInput)
	roundTrip := compareCodexRoundTrip(rollout, folded, nil)
	if callOrResultDifferences(roundTrip.differences) == 0 {
		t.Fatal("the comparison accepted a result the read-back attached to the other occurrence of the " +
			"shared id")
	}
	report := strings.Join(roundTrip.differences, "; ")
	if !strings.Contains(report, "written for the invocation of call "+reusedID+" to Bash") {
		t.Errorf("the difference does not name the invocation the result was written for: %q", report)
	}
	for _, content := range []string{lateOutput, earlyOutput, firstInput, secondInput} {
		if strings.Contains(report, content) {
			t.Errorf("the difference carries the evidence it could not match: %q", report)
		}
	}
	// The control: a read-back whose order attaches each output to the occurrence it was written for
	// holds no *tool-result* difference at all.
	noCallOrResultDifference(t, "distinct arguments, attached", compareCodexRoundTrip(rollout, attached, nil).differences)

	// The equal-fields shape: the two invocations share their id, tool and arguments, so the occurrence
	// of the invocation is the only evidence that says which of them an output answers.
	sameRollout, sameFolded, sameAttached := shape(firstInput, firstInput)
	sameRoundTrip := compareCodexRoundTrip(sameRollout, sameFolded, nil)
	if callOrResultDifferences(sameRoundTrip.differences) == 0 {
		t.Fatal("the comparison accepted a result the read-back attached to the other occurrence of an id " +
			"whose invocations have identical fields: only the occurrence distinguishes them")
	}
	sameReport := strings.Join(sameRoundTrip.differences, "; ")
	if !strings.Contains(sameReport, "written for the invocation of call "+reusedID+" to Bash") {
		t.Errorf("the difference does not name the invocation the result was written for: %q", sameReport)
	}
	if !strings.Contains(sameReport, "(occurrence 2)") {
		t.Errorf("the difference does not name the occurrence the read-back attached the output to: %q",
			sameReport)
	}
	for _, content := range []string{lateOutput, earlyOutput, firstInput} {
		if strings.Contains(sameReport, content) {
			t.Errorf("the difference carries the evidence it could not match: %q", sameReport)
		}
	}
	noCallOrResultDifference(t, "equal arguments, attached",
		compareCodexRoundTrip(sameRollout, sameAttached, nil).differences)

	// The installed-file arm (round 15's finding 2): the equal-fields shape with real installed bytes. The
	// file holds two `function_call` records whose id, tool and arguments are equal and one output after
	// the second, which is the call the inherited reader attaches it to (the last call registered for the
	// id). The fold attributes a row by the key it states, so with two records it cannot tell apart it
	// cannot say which record a row projects: the rows stand for the comparison, and an output the
	// document wrote for the first occurrence is reported -- where a fold that collapsed both records to
	// occurrence 1 accepted it as that occurrence's own. The control whose document wrote the output for
	// the second occurrence holds no call/result difference.
	installed := []byte(strings.Join([]string{
		codexInstalledLine(t, codexRecordResponseItem, codexFunctionCallPayload{
			Type: codexItemFunctionCall, CallID: reusedID, Name: "Bash", Arguments: firstInput}),
		codexInstalledLine(t, codexRecordResponseItem, codexFunctionCallPayload{
			Type: codexItemFunctionCall, CallID: reusedID, Name: "Bash", Arguments: firstInput}),
		codexInstalledLine(t, codexRecordResponseItem, codexFunctionCallOutputPayload{
			Type: codexItemFunctionOutput, CallID: reusedID, Output: earlyOutput}),
	}, ""))
	fileRollout := CodexRollout{
		Directory: "2026/05/04", FileName: "rollout-2026-05-04-selection.jsonl",
		Data: installed, SelectionReduced: true,
	}
	installedDocument := func(owner int) CodexRollout {
		return CodexRollout{SelectionReduced: true, Items: []TargetItem{
			{Kind: ItemToolCall, CallID: reusedID, ToolName: "Bash", InputJSON: firstInput},
			{Kind: ItemToolCall, CallID: reusedID, ToolName: "Bash", InputJSON: firstInput},
			{Kind: ItemToolResult, CallID: reusedID, Text: earlyOutput, OwnerCall: &owner},
		}}
	}
	firstItem, secondItem := 0, 1
	fileReadBack := Conversation{Messages: codexReadBackMessages(t, fileRollout)}
	installedRoundTrip := compareCodexRoundTrip(installedDocument(firstItem), fileReadBack, installed)
	if callOrResultDifferences(installedRoundTrip.differences) == 0 {
		t.Fatal("the comparison accepted a result the read-back attached to the other occurrence of two " +
			"installed records whose fields are identical: the fold may not attribute a row the file's " +
			"own bytes cannot tell apart")
	}
	installedReport := strings.Join(installedRoundTrip.differences, "; ")
	if !strings.Contains(installedReport, reusedID) {
		t.Errorf("the difference does not name the id it is about: %q", installedReport)
	}
	if !strings.Contains(installedReport, "(occurrence 2)") {
		t.Errorf("the difference does not name the occurrence the read-back attached the output to: %q",
			installedReport)
	}
	for _, content := range []string{earlyOutput, firstInput} {
		if strings.Contains(installedReport, content) {
			t.Errorf("the difference carries the evidence it could not match: %q", installedReport)
		}
	}
	noCallOrResultDifference(t, "installed equal records, owned by the second occurrence",
		compareCodexRoundTrip(installedDocument(secondItem), fileReadBack, installed).differences)
}

// TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute pins the tenth round's fourth finding: the
// invocation-unit fold answers the reader's per-file projection of one written record, so it may fold
// only the rows that record accounts for. A `custom_tool_call` appended under the id of the one
// `function_call` the selected document wrote is an invocation the inherited reader reads (it is a call
// row with its own name and input) but the fold's units -- the writer's own `function_call` envelopes --
// do not account for, and folding the id wholesale discarded the read-back's rows in favor of the single
// unit, so the extra invocation produced no difference at all. The case drives the real writer, the real
// installer and the inherited reader: the control arm is the untampered file, where the fold is taken and
// the conversion holds no call difference, and the tampered arm must report the invocation the file
// states and the document does not.
func TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute(t *testing.T) {
	const extraCommand = "the extra invocation's own command"
	patch := readFixtureConversation(t, codexPatchFixtureRef)
	selected, _, err := Select(patch, "0-", false)
	if err != nil {
		t.Fatalf("selecting the whole read: %v", err)
	}
	rollout, err := ConvertToCodexRollout(selected, CodexTargetOptions{CLIVersion: "selection-test-0.0"})
	if err != nil {
		t.Fatalf("converting the whole selected read to codex: %v", err)
	}
	if !rollout.SelectionReduced {
		t.Fatal("the converted document does not say it is one a selection produced: the fold is gated " +
			"on that answer")
	}
	home := t.TempDir()
	if _, _, err := InstallCodexRollout(rollout, home); err != nil {
		t.Fatalf("installing the rollout: %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(home, codexSessionsSubdir,
		filepath.FromSlash(rollout.Directory), rollout.FileName))
	if err != nil {
		t.Fatalf("reading the installed rollout: %v", err)
	}
	callID, callName, callInput := "", "", ""
	for _, item := range rollout.Items {
		if item.Kind == ItemToolCall {
			callID, callName, callInput = item.CallID, item.ToolName, item.InputJSON
		}
	}
	if callID == "" {
		t.Fatal("the converted document holds no call item to attach the extra invocation to")
	}

	noCallDifference := func(t *testing.T, arm string, differences []string) {
		t.Helper()
		for _, difference := range differences {
			if strings.Contains(difference, categoryToolCall) || strings.Contains(difference, categoryToolResult) {
				t.Errorf("%s: difference = %q, want none about the calls or their results", arm, difference)
			}
		}
	}
	control := compareCodexRoundTrip(rollout,
		Conversation{Messages: codexReadBackMessages(t, rollout)}, installed)
	noCallDifference(t, "control", control.differences)

	// The extra invocation is a `custom_tool_call` the inherited reader reads as a call of its own: the
	// record type the writer never writes, under the id the document's own call holds.
	extra := codexInstalledLine(t, codexRecordResponseItem, codexFunctionCallPayload{
		Type:      codexItemCustomToolCall,
		CallID:    callID,
		Name:      "Bash",
		Arguments: `{"command":"` + extraCommand + `"}`,
	})
	tampered := []byte(strings.TrimRight(string(installed), "\n") + "\n" + extra)
	tamperedRollout := CodexRollout{Directory: rollout.Directory, FileName: rollout.FileName, Data: tampered}
	roundTrip := compareCodexRoundTrip(rollout,
		Conversation{Messages: codexReadBackMessages(t, tamperedRollout)}, tampered)
	reported := false
	for _, difference := range roundTrip.differences {
		if strings.Contains(difference, "to Bash") {
			reported = true
		}
		if strings.Contains(difference, extraCommand) {
			t.Errorf("difference = %q, want the extra invocation named without its text", difference)
		}
	}
	if !reported {
		t.Errorf("differences = %q, want the invocation the file's own `custom_tool_call` states and "+
			"the document does not: the fold may discard only the rows the file's own records account "+
			"for", roundTrip.differences)
	}

	// The same shape with the extra call's own name and input equal to the document's: the reader's rows
	// for the two records are equal in every field the comparison keys a call by, so nothing in the rows
	// says which record a row came from -- the file's own records are what tell them apart. A fold that
	// reads the id alone drops the units this record states because its rows look like the document's
	// own, so the extra invocation must be reported here too.
	sameKey := codexInstalledLine(t, codexRecordResponseItem, codexFunctionCallPayload{
		Type:      codexItemCustomToolCall,
		CallID:    callID,
		Name:      callName,
		Arguments: callInput,
	})
	tamperedSame := []byte(strings.TrimRight(string(installed), "\n") + "\n" + sameKey)
	sameRollout := CodexRollout{Directory: rollout.Directory, FileName: rollout.FileName, Data: tamperedSame}
	sameRoundTrip := compareCodexRoundTrip(rollout,
		Conversation{Messages: codexReadBackMessages(t, sameRollout)}, tamperedSame)
	namedExtra := false
	for _, difference := range sameRoundTrip.differences {
		if strings.Contains(difference, categoryToolCall) {
			namedExtra = true
		}
	}
	if !namedExtra {
		t.Errorf("differences = %q, want the second call record reported although its reader's rows "+
			"hold the id, the name and the input the document's own call holds: the file states two "+
			"records and the document one", sameRoundTrip.differences)
	}

	// The keyed-only case (round 17's fourth finding): every record the unit walk *reads* under the id is
	// the writer's own keyed `function_call`, and the read-back still holds a row whose key no unit
	// states. The extra record is a `function_call` whose `arguments` member is a JSON object where this
	// build's written records state a string: the unit walk's own reading cannot unmarshal the line, so
	// it states no unit for the record, while the inherited reader reads it as a call row all the same --
	// the row stands for a call the file's units do not account for, and the fold may not discard it by
	// taking the keyed units (which would leave the extra invocation unreported although the file states
	// it). The two `custom_tool_call` arms above reach the unkeyed-unit refusal first; this arm is the
	// one that reaches the unmatched-row refusal, so a mutation removing that condition is observable
	// here alone.
	objectArguments := codexInstalledLine(t, codexRecordResponseItem, map[string]any{
		"type":      codexItemFunctionCall,
		"call_id":   callID,
		"name":      "Bash",
		"arguments": map[string]string{"command": extraCommand},
	})
	tamperedObject := []byte(strings.TrimRight(string(installed), "\n") + "\n" + objectArguments)
	objectRollout := CodexRollout{Directory: rollout.Directory, FileName: rollout.FileName, Data: tamperedObject}
	objectRoundTrip := compareCodexRoundTrip(rollout,
		Conversation{Messages: codexReadBackMessages(t, objectRollout)}, tamperedObject)
	objectReported := false
	for _, difference := range objectRoundTrip.differences {
		if strings.Contains(difference, categoryToolCall) && strings.Contains(difference, "to Bash") &&
			strings.Contains(difference, callID) {
			objectReported = true
		}
		if strings.Contains(difference, extraCommand) {
			t.Errorf("keyed-only: difference = %q, want the extra invocation named without its text",
				difference)
		}
	}
	if !objectReported {
		t.Errorf("keyed-only: differences = %q, want the call the file's own `function_call` record "+
			"states although the read-back's row for it states a key no unit states: every unit under "+
			"the id is keyed, and the fold may take them only where every row states one of their keys",
			objectRoundTrip.differences)
	}
}

// TestCodexRoundTripCountsWhatTheReaderProjectsAway pins the two shapes of a written dialogue message
// the read-back does not show with its own text, and what separates them from a loss: the installed
// file's own dialogue record.
//
// The inherited Codex reader removes the injected-context envelopes a user text block may open with
// before the block is a turn (internal/parser's preprocessCodexUserTextBlocks). A genuine user prompt
// can therefore be written verbatim and read back as no message at all (the whole prompt was the
// envelope), or read back shorter (the prompt carried text after the envelope, and the reader reads
// that text). Reporting either as a missing message failed a correct run -- the second shape was
// reported as evidence the file did not hold, because the reader's shorter reading stood in the ordered
// comparison where the written message had been resolved away -- and accepting a message the file does
// not hold would accept a loss. The file's own record is what tells the two apart, and both shapes
// resolve to the same count.
func TestCodexRoundTripCountsWhatTheReaderProjectsAway(t *testing.T) {
	const envelopeOnly = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	const enveloped = "<INSTRUCTIONS>quoted rules</INSTRUCTIONS>\nReview the changes"
	const readAfterEnvelope = "Review the changes"
	const assistant = "I read the policy."

	for _, testCase := range []struct {
		name     string
		prompt   string
		leading  string
		messages []Message
	}{
		{
			name:   "the reader reads no message for the prompt",
			prompt: envelopeOnly,
			messages: []Message{
				{Role: string(parser.RoleAssistant), Content: assistant},
			},
		},
		{
			name:   "the reader reads the text the envelope preceded",
			prompt: enveloped,
			messages: []Message{
				{Role: string(parser.RoleUser), Content: readAfterEnvelope},
				{Role: string(parser.RoleAssistant), Content: assistant},
			},
		},
		{
			// The walk reads the file's own bytes, and a line it cannot read as a record is skipped
			// rather than ending the walk: a session whose first line is not a record still has the
			// records below it counted.
			name:     "a line that is not a record comes first",
			prompt:   envelopeOnly,
			leading:  "not a record at all\n",
			messages: []Message{{Role: string(parser.RoleAssistant), Content: assistant}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rollout := CodexRollout{Items: []TargetItem{
				{Kind: ItemUser, Text: testCase.prompt},
				{Kind: ItemAssistant, Text: assistant},
			}}
			// The file is the writer's own records for both messages, so the case pins the check against
			// the shape a conversion really installs.
			installed := []byte(testCase.leading +
				codexDialogueRecord(t, ItemUser, testCase.prompt) +
				codexDialogueRecord(t, ItemAssistant, assistant))
			roundTrip := compareCodexRoundTrip(rollout, Conversation{Messages: testCase.messages}, installed)
			if len(roundTrip.differences) != 0 {
				t.Errorf("differences = %q, want none: the file's record holds the message the read-back "+
					"does not show with its own text", roundTrip.differences)
			}
			if roundTrip.notReadBackVerbatim != 1 {
				t.Errorf("notReadBackVerbatim = %d, want 1", roundTrip.notReadBackVerbatim)
			}
			if detail := roundTrip.detail(structureReaderCodex); !strings.Contains(detail, "1 written message(s)") ||
				strings.Contains(detail, testCase.prompt) {
				t.Errorf("the verification detail = %q, want the count named and no content", detail)
			}
		})
	}

	// The control for the second shape: the same document and the same read-back, with a file that never
	// held the message. The reader's shorter reading must not stand in for the message's own record --
	// that is the mutation the projection resolution must not swallow.
	t.Run("the shorter reading does not stand in for a missing record", func(t *testing.T) {
		rollout := CodexRollout{Items: []TargetItem{
			{Kind: ItemUser, Text: enveloped},
			{Kind: ItemAssistant, Text: assistant},
		}}
		installed := []byte(codexDialogueRecord(t, ItemAssistant, assistant))
		readBack := Conversation{Messages: []Message{
			{Role: string(parser.RoleUser), Content: readAfterEnvelope},
			{Role: string(parser.RoleAssistant), Content: assistant},
		}}
		roundTrip := compareCodexRoundTrip(rollout, readBack, installed)
		if roundTrip.notReadBackVerbatim != 0 {
			t.Errorf("notReadBackVerbatim = %d, want 0: the file does not hold the message's record",
				roundTrip.notReadBackVerbatim)
		}
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) == 0 ||
			!strings.Contains(report, describeText(enveloped)) ||
			!strings.Contains(report, describeText(readAfterEnvelope)) {
			t.Errorf("differences = %q, want the written message and the reader's shorter reading each "+
				"named by size and digest", roundTrip.differences)
		}
		// A comparison with a difference reports no count: the count says "in the file, not shown", and
		// nothing about this file says the message is in it.
		if detail := roundTrip.detail(structureReaderCodex); detail != "" {
			t.Errorf("a comparison with a difference reports a verification detail: %q", detail)
		}
	})

	// The rule the count stands on, pinned where its shape is one this reader does not produce today: one
	// read-back message shows one written message, so a text the document writes twice is shown twice only
	// when the read-back holds it twice. A count that asked whether the read-back holds the text anywhere
	// would answer the shape below as fully shown, and the inserted envelope case above is where the two
	// readings of "shown" part company in a file a run really installs.
	t.Run("one read-back message shows one written message", func(t *testing.T) {
		const prompt = "Review the changes"
		rollout := CodexRollout{Items: []TargetItem{
			{Kind: ItemUser, Text: prompt},
			{Kind: ItemUser, Text: prompt},
		}}
		installed := []byte(codexDialogueRecord(t, ItemUser, prompt) +
			codexDialogueRecord(t, ItemUser, prompt))
		readBack := Conversation{Messages: []Message{
			{Role: string(parser.RoleUser), Content: prompt},
		}}
		roundTrip := compareCodexRoundTrip(rollout, readBack, installed)
		if len(roundTrip.differences) != 0 {
			t.Errorf("differences = %q, want none: both messages' records are in the file where the "+
				"document writes them", roundTrip.differences)
		}
		if roundTrip.notReadBackVerbatim != 1 {
			t.Errorf("notReadBackVerbatim = %d, want 1: the read-back shows the text once for two written "+
				"messages", roundTrip.notReadBackVerbatim)
		}
	})
}

// TestCodexRoundTripReportsDialogueRecordsOutOfOrder pins the file's own record order, which the
// read-back cannot report for the messages the reader projects away.
//
// A user text block that opens with an injected-context envelope is read as that context rather than as
// a turn (internal/parser's preprocessCodexUserTextBlocks), so a genuine prompt the writer carried
// verbatim comes back as no message at all. Such a message is resolved by the installed file's own
// dialogue record -- and a check that asked the file whether it holds the message's text *anywhere*
// answers yes wherever the text stands, however the records are ordered. Two prompts the reader projects
// away, installed in the other order, would then verify: the file holds both texts, the read-back is the
// same assistant message, and the conversation the file holds is not the one the document writes.
//
// The walk resolves the written messages against the file's dialogue records in the file's own order, so
// the message whose record stands elsewhere is reported. The same rule covers the record side of the
// comparison: a record the document does not write, which the reader projects away too, reaches no
// read-back message and is reported by the file's own records alone.
func TestCodexRoundTripReportsDialogueRecordsOutOfOrder(t *testing.T) {
	const first = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	const second = "# AGENTS.md\nThe retry budget is a constant in internal/retry/budget.go."
	const answer = "I read both notes."
	const unwritten = "# AGENTS.md\nA prompt this conversion did not write."
	document := CodexRollout{Items: []TargetItem{
		{Kind: ItemUser, Text: first},
		{Kind: ItemUser, Text: second},
		{Kind: ItemAssistant, Text: answer},
	}}
	// The read-back the inherited reader really produces for such a file: both prompts are read as
	// injected context, so the assistant's message is the one that comes back.
	readBack := Conversation{Messages: []Message{{Role: string(parser.RoleAssistant), Content: answer}}}

	t.Run("the file's records in the document's order verify", func(t *testing.T) {
		installed := []byte(codexDialogueRecord(t, ItemUser, first) +
			codexDialogueRecord(t, ItemUser, second) +
			codexDialogueRecord(t, ItemAssistant, answer))
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		if len(roundTrip.differences) != 0 || roundTrip.notReadBackVerbatim != 2 {
			t.Errorf("round trip = %+v, want both projected prompts counted and no difference", roundTrip)
		}
	})

	t.Run("the two projected records swapped are reported", func(t *testing.T) {
		installed := []byte(codexDialogueRecord(t, ItemUser, second) +
			codexDialogueRecord(t, ItemUser, first) +
			codexDialogueRecord(t, ItemAssistant, answer))
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) == 0 {
			t.Fatalf("the check accepted a file whose dialogue records stand in another order: %+v", roundTrip)
		}
		if !strings.Contains(report, "out of order") || !strings.Contains(report, describeText(first)) {
			t.Errorf("differences = %q, want the out-of-place message named by its own size and digest",
				report)
		}
	})

	t.Run("a record the document does not write is reported", func(t *testing.T) {
		// The extra record stands after the last written one and opens with an envelope too, so the
		// reader reads no message for it and the read-back cannot report it: the file's own records do.
		installed := []byte(codexDialogueRecord(t, ItemUser, first) +
			codexDialogueRecord(t, ItemUser, second) +
			codexDialogueRecord(t, ItemAssistant, answer) +
			codexDialogueRecord(t, ItemUser, unwritten))
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) == 0 || !strings.Contains(report, "was not written") ||
			!strings.Contains(report, describeText(unwritten)) {
			t.Errorf("differences = %q, want the record the document does not write named by its own "+
				"size and digest", report)
		}
	})
}

// TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord pins what the check asks the installed file: the
// dialogue record a conversion writes for the message -- the record's own type, the payload's message
// type, the role, and the content block the writer writes for that role -- counted, so one record
// answers for one written message.
//
// Every mutation below is a file a correct conversion would never write, and each holds the message's
// text *somewhere*: the target writes every message twice (the response_item its model history is built
// from, and the event its own view of the conversation is built from, which the inherited reader reads
// no message from), a tool output can quote the text, and raw bytes can spell it. A check that asked
// the file for the text anywhere would accept the first of those -- a session the target can load while
// the message is not in it. Most of the mutations the reader also reads no message from; the last one it
// reads the message's own text from, which is why the check reads the file's record even where the
// read-back shows the message.
func TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord(t *testing.T) {
	const prompt = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	rollout := CodexRollout{Items: []TargetItem{{Kind: ItemUser, Text: prompt}}}

	// A case's read-back defaults to the one the inherited reader really produces for such a file: the
	// prompt is read as injected context, so no message comes back for it.
	for _, testCase := range []struct {
		name      string
		installed []byte
		readBack  Conversation
	}{
		{name: "the file holds no record at all"},
		{
			name:      "the text is only in the turn's own event",
			installed: []byte(codexUserEventRecord(t, prompt)),
		},
		{
			name: "the record is another kind of record",
			installed: []byte(codexInstalledLine(t, codexRecordTurnContext, codexMessagePayload{
				Type:    codexItemMessage,
				Role:    codexRoleUser,
				Content: []codexContent{{Type: codexContentInputText, Text: prompt}},
			})),
		},
		{
			name: "the payload is another kind of message",
			installed: []byte(codexInstalledLine(t, codexRecordResponseItem, codexMessagePayload{
				Type:    "reasoning",
				Role:    codexRoleUser,
				Content: []codexContent{{Type: codexContentInputText, Text: prompt}},
			})),
		},
		{
			name:      "the record's role is the assistant's",
			installed: []byte(codexDialogueRecord(t, ItemAssistant, prompt)),
		},
		{
			name: "the content block is the type this writer writes for the other role",
			installed: []byte(codexInstalledLine(t, codexRecordResponseItem, codexMessagePayload{
				Type:    codexItemMessage,
				Role:    codexRoleUser,
				Content: []codexContent{{Type: codexContentOutputText, Text: prompt}},
			})),
		},
		{
			name: "the record carries no content block with the text",
			installed: []byte(codexInstalledLine(t, codexRecordResponseItem, codexMessagePayload{
				Type:    codexItemMessage,
				Role:    codexRoleUser,
				Content: []codexContent{{Type: codexContentInputText, Text: ""}},
			})),
		},
		{
			name:      "the text is a raw byte run",
			installed: []byte(prompt + "\n"),
		},
		{
			// The reader reads a text block whose type is input_text, output_text or text alike
			// (internal/parser's extractCodexTextBlocks), so this record comes back as this very user
			// message: a check that asked the read-back whether the message is there would accept a file
			// whose model-visible dialogue record is not the one this writer writes. The walk resolves
			// the message against the file's record whatever the read-back shows.
			name: "the reader reads the text back from another record shape",
			installed: []byte(codexInstalledLine(t, codexRecordResponseItem, codexMessagePayload{
				Type:    codexItemMessage,
				Role:    codexRoleUser,
				Content: []codexContent{{Type: codexContentOutputText, Text: prompt}},
			})),
			readBack: Conversation{Messages: []Message{
				{Role: string(parser.RoleUser), Content: prompt},
			}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			roundTrip := compareCodexRoundTrip(rollout, testCase.readBack, testCase.installed)
			if roundTrip.notReadBackVerbatim != 0 {
				t.Errorf("notReadBackVerbatim = %d, want 0: the file does not hold the message's record",
					roundTrip.notReadBackVerbatim)
			}
			report := strings.Join(roundTrip.differences, "; ")
			if len(roundTrip.differences) == 0 || !strings.Contains(report, categoryDialogue) ||
				!strings.Contains(report, describeText(prompt)) {
				t.Errorf("differences = %q, want the written message named by its own size and digest",
					roundTrip.differences)
			}
		})
	}
}

// TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage pins the other side of a record's shape:
// the writer never writes a message whose text is empty, and the target's own reader reads a block whose
// text is blank as no message at all (internal/parser's handleResponseItem drops it) -- so a document
// claiming such a message is a difference whatever the file holds, and an empty content block does not
// answer for it.
func TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage(t *testing.T) {
	rollout := CodexRollout{Items: []TargetItem{{Kind: ItemUser, Text: ""}}}
	roundTrip := compareCodexRoundTrip(rollout, Conversation{}, []byte(codexDialogueRecord(t, ItemUser, "")))
	if roundTrip.notReadBackVerbatim != 0 {
		t.Errorf("notReadBackVerbatim = %d, want 0: an empty block is no message", roundTrip.notReadBackVerbatim)
	}
	report := strings.Join(roundTrip.differences, "; ")
	if len(roundTrip.differences) != 1 || !strings.Contains(report, "not in the file") ||
		!strings.Contains(report, describeText("")) {
		t.Errorf("differences = %q, want the empty message named by its size and digest",
			roundTrip.differences)
	}
}

// TestCodexRoundTripCountsARecordOncePerWrittenMessage pins the count: one record answers for one
// written message. A source can record the same prompt twice (a re-ask), and both messages are carried
// -- so a single record must not be what both of them resolve against.
func TestCodexRoundTripCountsARecordOncePerWrittenMessage(t *testing.T) {
	const prompt = "# AGENTS.md\nThe retry policy lives in internal/retry/policy.go."
	rollout := CodexRollout{Items: []TargetItem{{Kind: ItemUser, Text: prompt}, {Kind: ItemUser, Text: prompt}}}
	// The same read-back shape as the single-message case: the reader reads no message for the text.
	roundTrip := compareCodexRoundTrip(rollout, Conversation{}, []byte(codexDialogueRecord(t, ItemUser, prompt)))
	if roundTrip.notReadBackVerbatim != 1 {
		t.Errorf("notReadBackVerbatim = %d, want 1", roundTrip.notReadBackVerbatim)
	}
	report := strings.Join(roundTrip.differences, "; ")
	if len(roundTrip.differences) != 1 || !strings.Contains(report, "dialogue 2") ||
		!strings.Contains(report, describeText(prompt)) {
		t.Errorf("differences = %q, want the second written message named by its own size and digest",
			roundTrip.differences)
	}
}

// TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor pins the read-back's side of the
// comparison: a message the read-back shows, which no written message accounts for, is evidence the file
// holds dialogue the conversion did not write.
//
// What a written message accounts for is the reader's own reading of it: the reader deletes the segments
// it classifies away (a leading injected-context envelope, a plugin envelope in the middle of a block)
// and never invents characters, so what it reads for a record is a run of that record's own characters
// in their own order. A message that is not such a run is not that message's reading -- neither text it
// does not hold, nor characters out of order -- and a run with a gap in it is, because a middle deletion
// is one of the reader's own shapes.
func TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor(t *testing.T) {
	const written = "Review <recommended_plugins>the plugin list</recommended_plugins>the changes"
	const gaps = "Review the changes"
	const assistant = "I read the policy."
	document := CodexRollout{Items: []TargetItem{{Kind: ItemUser, Text: written}, {Kind: ItemAssistant, Text: assistant}}}
	installed := []byte(codexDialogueRecord(t, ItemUser, written) +
		codexDialogueRecord(t, ItemAssistant, assistant))
	message := func(role parser.RoleType, content string) Message {
		return Message{Role: string(role), Content: content}
	}

	t.Run("the reader's reading of the message is accounted for", func(t *testing.T) {
		readBack := Conversation{Messages: []Message{
			message(parser.RoleUser, gaps),
			message(parser.RoleAssistant, assistant),
		}}
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		if len(roundTrip.differences) != 0 || roundTrip.notReadBackVerbatim != 1 {
			t.Errorf("round trip = %+v, want the written message counted and no difference",
				roundTrip)
		}
	})

	for _, testCase := range []struct {
		name     string
		readText string
	}{
		{name: "the read-back text holds characters the message does not", readText: gaps + "!"},
		{name: "the read-back text is another turn", readText: "an entirely other turn"},
		{name: "the read-back text holds the message's characters out of order",
			readText: "changes the Review"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			readBack := Conversation{Messages: []Message{
				message(parser.RoleUser, testCase.readText),
				message(parser.RoleAssistant, assistant),
			}}
			roundTrip := compareCodexRoundTrip(document, readBack, installed)
			report := strings.Join(roundTrip.differences, "; ")
			if len(roundTrip.differences) != 1 || !strings.Contains(report, "was not written") ||
				!strings.Contains(report, describeText(testCase.readText)) {
				t.Errorf("differences = %q, want the read-back message named by its own size and digest",
					roundTrip.differences)
			}
		})
	}

	t.Run("the reading is recorded under the written message's own kind", func(t *testing.T) {
		// The text is a run of the written user message's own text, but the reader recorded it under the
		// assistant's role: a message the reader puts in another role is not this message's reading, so
		// the read-back message is unaccounted for and the written message is still resolved by its
		// record.
		readBack := Conversation{Messages: []Message{
			message(parser.RoleAssistant, gaps),
			message(parser.RoleAssistant, assistant),
		}}
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) != 1 || !strings.Contains(report, "was not written") ||
			!strings.Contains(report, describeText(gaps)) || roundTrip.notReadBackVerbatim != 1 {
			t.Errorf("round trip = %+v, want the reading under another role unaccounted for and the "+
				"written message counted", roundTrip)
		}
	})

	t.Run("the read-back's order is the file's own order", func(t *testing.T) {
		// The two messages swapped: the read-back shows each text, but at the other message's place. The
		// walk pairs in the file's own order, so the message the read-back shows last is the one no
		// written message accounts for where it stands.
		readBack := Conversation{Messages: []Message{
			message(parser.RoleAssistant, assistant),
			message(parser.RoleUser, written),
		}}
		roundTrip := compareCodexRoundTrip(document, readBack, installed)
		report := strings.Join(roundTrip.differences, "; ")
		if len(roundTrip.differences) != 1 || !strings.Contains(report, "was not written") ||
			!strings.Contains(report, describeText(written)) {
			t.Errorf("differences = %q, want the out-of-place read-back message named", roundTrip.differences)
		}
	})
}

// codexInstalledLine spells one installed rollout line the way the writer writes one -- from the
// writer's own record types and encoder -- so a case about the file's records holds the shape a
// conversion really installs instead of a literal that can drift from it.
func codexInstalledLine(t *testing.T, recordType string, payload any) string {
	t.Helper()
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(codexEnvelope{
		Timestamp: "2026-05-04T09:15:00Z",
		Type:      recordType,
		Payload:   payload,
	}); err != nil {
		t.Fatalf("encoding an installed line: %v", err)
	}
	return encoded.String()
}

// codexDialogueRecord is the record this writer writes for one dialogue message: the model-visible
// response_item the target's history is built from.
func codexDialogueRecord(t *testing.T, kind TargetItemKind, text string) string {
	t.Helper()
	role, contentType := codexRoleUser, codexContentInputText
	if kind == ItemAssistant {
		role, contentType = codexRoleAssistant, codexContentOutputText
	}
	return codexInstalledLine(t, codexRecordResponseItem, codexMessagePayload{
		Type:    codexItemMessage,
		Role:    role,
		Content: []codexContent{{Type: contentType, Text: text}},
	})
}

// codexUserEventRecord is the record this writer writes for the target's own view of a user turn: the
// event the harness's conversation view is built from, which the inherited reader reads no message from.
func codexUserEventRecord(t *testing.T, text string) string {
	t.Helper()
	return codexInstalledLine(t, codexRecordEventMsg, codexUserMessagePayload{
		Type:         codexEventUserMessage,
		Message:      text,
		Images:       []any{},
		LocalImages:  []any{},
		TextElements: []any{},
	})
}
