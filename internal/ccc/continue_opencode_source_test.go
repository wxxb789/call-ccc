package ccc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/ccc/opencodefixture"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// The committed OpenCode fixture (internal/ccc/fixturegen's openCodeSession), as the source direction
// reads it: one invented conversation -- two turns, the first of which read a file through the `read`
// tool -- recorded in the shape the harness's own `session export` writes.
//
// The identities are named once here because a case reads the fixture's own bytes, the store's rows and
// the installed target's records by them: a member this build got wrong is then caught by reading the
// fixture rather than by agreeing with the conversion's own report.
const (
	openCodeSourceSessionID = "ses_0008opencodefixture00000001"
	openCodeSourceCallID    = "call_0008opencodefixture000b01"
	openCodeSourceIdleID    = "msg_0008opencodefixture0000a06"
	openCodeSourceQuestion  = "Which file holds the retry cap, and what is the cap?"
	openCodeSourceFollowup  = "And how many attempts does the policy allow?"
	openCodeSourceAnswer    = "Four attempts, and thirty seconds of backoff between them."
	// openCodeSourceTrace and openCodeSourceRenderedText are the persisted reasoning trace and the text the
	// inherited reader builds out of the same record's parts: the trace verbatim between its markers, then
	// the record's own text, joined with newlines. The read records no trace text of its own (Thinking is
	// empty for every record this reader produces), which is the fact that makes the record's dialogue
	// unprojected and its text travel with that caveat counted.
	openCodeSourceTrace        = "The grep hit is maxBackoffSeconds = 30 in internal/retry/config.go."
	openCodeSourceCap          = "The cap is `maxBackoffSeconds = 30` in internal/retry/config.go."
	openCodeSourceRenderedText = "[Thinking]\n" + openCodeSourceTrace + "\n[/Thinking]\n" + openCodeSourceCap
	openCodeSourceToolInput    = `{"path":"/home/user/code/example-api/internal/retry/config.go"}`
	openCodeSourceToolResult   = "Read file /home/user/code/example-api/internal/retry/config.go, lines 38-42\n" +
		"38: // policy.go\n40: const maxAttempts = 4\n41: const maxBackoffSeconds = 30\n42: "
	openCodeSourceCwd = "/home/user/code/example-api"
)

// openCodeSourceFixtureRef is the fixture as a reference: the provider's own identifier and the session id
// the document records.
var openCodeSourceFixtureRef = Ref{Provider: "opencode", ID: openCodeSourceSessionID}

// openCodeSourceFixtureDocument reads the committed fixture document's own bytes. A case compares a target
// with *these* bytes rather than with the conversion's own items, so a writer that mangled a tool name, an
// argument or a result cannot pass by agreeing with itself.
func openCodeSourceFixtureDocument(t *testing.T) []byte {
	t.Helper()
	path := fixturePath(t, filepath.Join("opencode", openCodeSourceSessionID+".json"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the OpenCode fixture %s: %v", path, err)
	}
	return data
}

// openCodeSourceStore builds one throw-away OpenCode store out of the committed fixture document and
// answers the directory that holds it. That directory is the provider's configured root for this agent --
// the directory whose `opencode.db` its own discovery reads -- so a reader pointed at it resolves the
// session exactly as it does for a caller who set OPENCODE_DIR.
func openCodeSourceStore(t *testing.T, document []byte) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), openCodeDataDirName)
	path, id, err := opencodefixture.Store(root, document)
	if err != nil {
		t.Fatalf("building the OpenCode store: %v", err)
	}
	if id != openCodeSourceSessionID {
		t.Fatalf("the store holds session %s, want the fixture's own id %s", id, openCodeSourceSessionID)
	}
	want := filepath.Join(root, openCodeStoreName())
	if path != want {
		t.Fatalf("the store is at %s, want %s", path, want)
	}
	return root
}

// openCodeStoreName is the file name the harness keeps its sessions in, read from the fixture package so a
// case never spells it twice.
func openCodeStoreName() string { return opencodefixture.StoreName }

// openCodeSourceConfig points the inherited providers at one OpenCode root beside the committed fixtures'
// own roots, and the archive at a throw-away directory.
func openCodeSourceConfig(t *testing.T, root string) config.Config {
	t.Helper()
	cfg := fixtureConfig(t)
	dirs := make(map[parser.AgentType][]string, len(cfg.AgentDirs)+1)
	for agent, roots := range cfg.AgentDirs {
		dirs[agent] = roots
	}
	dirs[parser.AgentOpenCode] = []string{root}
	cfg.AgentDirs = dirs
	return cfg
}

// readOpenCodeSourceFixture reads the committed fixture through the inherited provider.
func readOpenCodeSourceFixture(t *testing.T, root string) Conversation {
	t.Helper()
	conversation, err := NewSourceReader(openCodeSourceConfig(t, root)).Read(
		context.Background(), openCodeSourceFixtureRef)
	if err != nil {
		t.Fatalf("reading the OpenCode fixture: %v", err)
	}
	return conversation
}

// lossByCategory answers one loss row, which every routed writer reports on every run.
func lossByCategory(t *testing.T, losses []Loss, category string) Loss {
	t.Helper()
	for _, loss := range losses {
		if loss.Category == category {
			return loss
		}
	}
	t.Fatalf("the report names no %s row: %+v", category, losses)
	return Loss{}
}

// TestOpenCodeSourceReadAnswersItsOwnProviderAndRecords pins the whole source half of goal 0008's route:
// the session is resolved by the inherited AgentsView OpenCode provider from the configured root (no
// bespoke SQL reader, no second parser), and the conversation documented here is the reader's own answer
// for the store's records -- including the parts that are *not* the model's words: the reasoning trace the
// reader renders into the record's Content, the progress event a completed tool part produces beside its
// terminal one, and the `idle` record the reader keeps out of the conversation because it is a session
// control rather than a message.
func TestOpenCodeSourceReadAnswersItsOwnProviderAndRecords(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	conversation := readOpenCodeSourceFixture(t, root)

	if conversation.Source.Provider != "opencode" {
		t.Errorf("provider = %q, want opencode", conversation.Source.Provider)
	}
	if conversation.Source.Agent != "OpenCode" {
		t.Errorf("agent = %q, want the registry's own display name", conversation.Source.Agent)
	}
	if conversation.Source.SessionID != "opencode:"+openCodeSourceSessionID {
		t.Errorf("session id = %q", conversation.Source.SessionID)
	}
	// The read's identity is the provider's own virtual path for a member of its store: no file holds this
	// conversation's bytes, which is the fact the boundary block states later.
	wantPath := parser.VirtualSourcePath(filepath.Join(root, openCodeStoreName()), openCodeSourceSessionID)
	if conversation.Source.SourcePath != wantPath || conversation.Source.DisplayPath != wantPath {
		t.Errorf("source path = %q/%q, want %q", conversation.Source.SourcePath,
			conversation.Source.DisplayPath, wantPath)
	}
	if conversation.Source.Cwd != openCodeSourceCwd {
		t.Errorf("working directory = %q, want the store's own recorded directory %q",
			conversation.Source.Cwd, openCodeSourceCwd)
	}
	if conversation.Source.Project != "example_api" {
		t.Errorf("project = %q, want the name the store's worktree answers", conversation.Source.Project)
	}
	if conversation.Fidelity.ReadFrom != "source" {
		t.Errorf("read_from = %q, want source: this read is the provider's, not the archive's",
			conversation.Fidelity.ReadFrom)
	}

	// The store holds six records; the conversation holds five, because an `idle` record is a session
	// control the reader's own switch skips. Nothing is counted for it anywhere: the read never held it,
	// and a conversion reports the read's counts rather than the store's rows.
	wantCounts := Counts{
		Messages: 5, UserMessages: 2, AssistantMessages: 3, ToolCalls: 1, ToolResults: 2,
	}
	if got := Summarize(conversation.Messages); got != wantCounts {
		t.Errorf("counts = %+v, want %+v", got, wantCounts)
	}
	for _, message := range conversation.Messages {
		if message.SourceUUID == openCodeSourceIdleID {
			t.Errorf("the read holds the store's idle record as a message: %+v", message)
		}
	}

	// No record the OpenCode reader produces carries a trace of its own, and none is a dialogue projection:
	// every text these records write is the reader's Content for them. That is what the conversion counts
	// as unprojected_dialogue on both targets, and it is asserted here so a later reader change that starts
	// recording traces or projections fails this case rather than silently changing the report.
	for index, message := range conversation.Messages {
		if message.Thinking != "" {
			t.Errorf("message %d records a trace (%q); this reader renders traces into Content instead",
				index, message.Thinking)
		}
		if message.DialogueProjected || message.Dialogue != "" {
			t.Errorf("message %d answers a dialogue projection (%+v); this reader answers none", index,
				message)
		}
		if !message.ContentRendersThinking {
			t.Errorf("message %d is not marked as produced by a reader that renders a trace into Content",
				index)
		}
	}

	if got := conversation.Messages[0].Content; got != openCodeSourceQuestion {
		t.Errorf("the first user record's text = %q", got)
	}
	if got := conversation.Messages[3].Content; got != openCodeSourceFollowup {
		t.Errorf("the second user record's text = %q", got)
	}

	// The tool part: one call, and the two result events the reader answers for a completed part -- the
	// progress event the part's own `time.ran` produces (no text at all) beside the terminal event that
	// carries the part's recorded content at the instant it completed.
	call := conversation.Messages[1].ToolCalls
	if len(call) != 1 {
		t.Fatalf("tool calls = %+v, want the fixture's one read", call)
	}
	if call[0].ToolUseID != openCodeSourceCallID || call[0].Name != "read" || call[0].Category != "Read" {
		t.Errorf("call = %+v", call[0])
	}
	if call[0].InputJSON != openCodeSourceToolInput {
		t.Errorf("call input = %q, want the store's own spelling %q", call[0].InputJSON,
			openCodeSourceToolInput)
	}
	results := conversation.Messages[1].ToolResults
	if len(results) != 2 {
		t.Fatalf("tool results = %+v, want the progress event and the terminal one", results)
	}
	if results[0].Status != "started" || results[0].Source != "tool_execution" ||
		strings.TrimSpace(results[0].ContentRaw) != "" {
		t.Errorf("the progress event = %+v, want a textless started event", results[0])
	}
	if results[1].Status != "completed" || results[1].ContentRaw != openCodeSourceToolResult {
		t.Errorf("the terminal event = %+v", results[1])
	}
	if !results[0].Timestamp.Before(results[1].Timestamp) {
		t.Errorf("the progress event (%v) is not before the terminal one (%v)",
			results[0].Timestamp, results[1].Timestamp)
	}

	// The record with the persisted reasoning trace: the trace verbatim between the reader's markers, then
	// the record's own text -- the read's own rendering, which is what a conversion carries with the caveat
	// counted rather than removing or inventing anything.
	if got := conversation.Messages[2].Content; got != openCodeSourceRenderedText {
		t.Errorf("the reasoning record's text = %q, want the reader's own rendering %q", got,
			openCodeSourceRenderedText)
	}
	if got := conversation.Messages[4].Content; got != openCodeSourceAnswer {
		t.Errorf("the second answer's text = %q, want the fixture's own text %q", got, openCodeSourceAnswer)
	}
}

// TestOpenCodeSourceResolvesByIdentifierAndByItsOwnPath pins that both questions the provider itself
// answers name the same session: a caller who knows only the store's path can pass the provider's own
// virtual-path spelling (`<database>#<session id>`) and reach the same conversation as a caller who names
// the session id under a configured root.
func TestOpenCodeSourceResolvesByIdentifierAndByItsOwnPath(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	cfg := openCodeSourceConfig(t, root)
	reader := NewSourceReader(cfg)

	byID, err := reader.Read(context.Background(), openCodeSourceFixtureRef)
	if err != nil {
		t.Fatalf("reading by session id: %v", err)
	}
	byPath, err := reader.Read(context.Background(), Ref{
		Provider: "opencode",
		ID:       parser.VirtualSourcePath(filepath.Join(root, openCodeStoreName()), openCodeSourceSessionID),
	})
	if err != nil {
		t.Fatalf("reading by the provider's own virtual path: %v", err)
	}
	if byPath.Source.SessionID != byID.Source.SessionID {
		t.Errorf("the two reference forms name %q and %q", byID.Source.SessionID, byPath.Source.SessionID)
	}
	if got, want := Summarize(byPath.Messages), Summarize(byID.Messages); got != want {
		t.Errorf("the two reference forms read %+v and %+v", got, want)
	}
}

// TestContinueFromOpenCodeThroughTheCommonModel pins the source direction end to end for both accepted
// target routes: an OpenCode session continues in Codex and in Claude Code through the same route table
// and the same reader every other source uses, with no OpenCode-specific branch in the pipeline. What the
// conversion cannot state about a container member is stated rather than invented: the boundary is
// `not-taken` naming the store and the member, and the source comparison does not run.
func TestContinueFromOpenCodeThroughTheCommonModel(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	cfg := openCodeSourceConfig(t, root)
	conversation := readOpenCodeSourceFixture(t, root)

	for _, target := range []string{TargetCodex, TargetClaude} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          openCodeSourceFixtureRef,
				Target:       target,
				Config:       cfg,
				TargetHome:   home,
				TargetBinary: missingTargetBinary,
			})
			if err != nil {
				t.Fatalf("continuing %s into %s: %v", openCodeSourceFixtureRef.ID, target, err)
			}
			if result.Source == nil {
				t.Fatal("the result carries no source report")
			}
			if result.Source.Identity.Provider != "opencode" {
				t.Errorf("source provider = %q", result.Source.Identity.Provider)
			}
			if result.Source.Identity.SourcePath != conversation.Source.SourcePath {
				t.Errorf("source path = %q, want the provider's virtual path %q",
					result.Source.Identity.SourcePath, conversation.Source.SourcePath)
			}
			if result.Conversion.State != ConversionStateConverted {
				t.Fatalf("conversion = %+v, want the conversation converted", result.Conversion)
			}
			if result.Conversion.Carried.ToolCalls != 1 || result.Conversion.Carried.ToolResults != 1 {
				t.Errorf("carried = %+v, want the fixture's call and its terminal result",
					result.Conversion.Carried)
			}

			// Every written message is counted as unprojected dialogue -- the reader answered no projection
			// for any record -- and the progress event is counted as a result with no text rather than
			// dropped in silence.
			unprojected := lossByCategory(t, result.Conversion.Losses, "unprojected_dialogue")
			if unprojected.Disposition != DispositionCarriedWithCaveat ||
				unprojected.Count != result.Conversion.Carried.Messages {
				t.Errorf("unprojected_dialogue = %+v, want every carried message (%d)",
					unprojected, result.Conversion.Carried.Messages)
			}
			progress := lossByCategory(t, result.Conversion.Losses, "empty_tool_results")
			if progress.Disposition != DispositionOmitted || progress.Count != 1 {
				t.Errorf("empty_tool_results = %+v, want the fixture's one textless progress event", progress)
			}

			if result.Verification.Structure.State != VerificationStateVerified {
				t.Fatalf("structure = %+v, want the installed session read back as written",
					result.Verification.Structure)
			}
			if result.Verification.Structure.ReadBack.ToolCalls != result.Conversion.Carried.ToolCalls ||
				result.Verification.Structure.ReadBack.ToolResults != result.Conversion.Carried.ToolResults {
				t.Errorf("read-back = %+v, want the %d call(s) and %d result(s) carried",
					result.Verification.Structure.ReadBack, result.Conversion.Carried.ToolCalls,
					result.Conversion.Carried.ToolResults)
			}
			directWriterAgreesWithThePipeline(t, target, conversation, result)
		})
	}
}

// TestOpenCodeSourceBoundaryStatesTheContainerItCannotMeasure pins what a conversion of a store member may
// and may not claim: the member's bytes are the container's rows, so the boundary names the member and the
// store, claims no digest of either as this conversation's own, and the source comparison is reported as
// one that did not run. A conversion that claimed the database file's digest would present every member's
// bytes as this member's, and the harness's own writes move those bytes with its WAL handling.
func TestOpenCodeSourceBoundaryStatesTheContainerItCannotMeasure(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	result, err := Continue(context.Background(), ContinueOptions{
		Ref:          openCodeSourceFixtureRef,
		Target:       TargetCodex,
		Config:       openCodeSourceConfig(t, root),
		TargetHome:   t.TempDir(),
		TargetBinary: missingTargetBinary,
	})
	if err != nil {
		t.Fatalf("continuing %s into codex: %v", openCodeSourceFixtureRef.ID, err)
	}
	if result.Source == nil || result.Source.Boundary == nil {
		t.Fatal("the conversion reports no boundary block")
	}
	boundary := result.Source.Boundary
	if boundary.State != SourceBoundaryNotTaken {
		t.Errorf("boundary = %+v, want not-taken for a container member", boundary)
	}
	for _, want := range []string{openCodeSourceSessionID, openCodeStoreName(), root} {
		if !strings.Contains(boundary.Detail, want) {
			t.Errorf("the boundary detail does not name %q: %q", want, boundary.Detail)
		}
	}
	if boundary.SHA256 != "" || boundary.Bytes != 0 || boundary.RecordOffsets != "" ||
		boundary.ReadCoverage != "" {
		t.Errorf("the boundary claims a file's bytes for a container member: %+v", boundary)
	}
	if result.Installation.SourceCheckState != SourceCheckNotVerified || result.Installation.Detail == "" {
		t.Errorf("installation = %+v, want the source comparison reported as one that did not run",
			result.Installation)
	}
}

// installedRecords reads one installed target session's own records, so a case compares the target harness's
// file with the source fixture's bytes rather than with the writer's own document.
func installedRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the installed session %s: %v", path, err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("parsing a record of %s: %v", path, err)
		}
		records = append(records, record)
	}
	return records
}

// TestOpenCodeSourceToolEvidenceReachesEachTarget pins the goal's own clause for a tool-bearing source: the
// call and its result travel as the target's own records, and the text travels exactly as the source
// recorded it. The comparison reads the *fixture document's* bytes for the expected values and the
// *installed file* for the written ones -- never the conversion's own items -- so a writer that mangled a
// name, an argument or a result cannot pass by agreeing with itself. What cannot travel is counted: the
// progress event is not another output (the file holds one per call) and the report says so.
func TestOpenCodeSourceToolEvidenceReachesEachTarget(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	cfg := openCodeSourceConfig(t, root)

	for _, target := range []string{TargetCodex, TargetClaude} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			result, err := Continue(context.Background(), ContinueOptions{
				Ref:          openCodeSourceFixtureRef,
				Target:       target,
				Config:       cfg,
				TargetHome:   home,
				TargetBinary: missingTargetBinary,
			})
			if err != nil {
				t.Fatalf("continuing %s into %s: %v", openCodeSourceFixtureRef.ID, target, err)
			}
			records := installedRecords(t, result.TargetSession.Path)

			// The call and its result, read out of the installed file's own records through each target
			// format's own shapes: a Codex rollout writes a function_call and a function_call_output, a
			// Claude transcript a tool_use and a tool_result block.
			var names, arguments, outputs, texts []string
			outputsForCall := 0
			for _, record := range records {
				if payload, ok := record["payload"].(map[string]any); ok {
					switch payload["type"] {
					case "function_call":
						name, _ := payload["name"].(string)
						args, _ := payload["arguments"].(string)
						names = append(names, name)
						arguments = append(arguments, args)
					case "function_call_output":
						output, _ := payload["output"].(string)
						outputs = append(outputs, output)
						if callID, _ := payload["call_id"].(string); callID == openCodeSourceCallID {
							outputsForCall++
						}
					case "message":
						collectTexts(&texts, payload["content"])
					}
				}
				if message, ok := record["message"].(map[string]any); ok {
					collectBlocks(&names, &arguments, &outputs, &outputsForCall, &texts, message)
				}
			}

			// The recorded input object survives as its value: the target's own encoding is the target's
			// (a Codex rollout writes the arguments as a JSON string, a Claude transcript keeps the object),
			// so the comparison is of values rather than of a spelling.
			if len(names) != 1 || names[0] != "read" {
				t.Errorf("the installed file's tool names = %v, want the fixture's one read", names)
			}
			if len(arguments) != 1 || !sameJSONValue(t, arguments[0], openCodeSourceToolInput) {
				t.Errorf("the installed file's recorded input = %v, want the fixture's own value %s",
					arguments, openCodeSourceToolInput)
			}
			if len(outputs) != 1 || outputs[0] != openCodeSourceToolResult {
				t.Errorf("the installed file's tool output = %v, want the fixture's own result text %q",
					outputs, openCodeSourceToolResult)
			}
			if outputsForCall != 1 {
				t.Errorf("the installed file holds %d output(s) for %s: the textless progress event is "+
					"not a second output", outputsForCall, openCodeSourceCallID)
			}
			progress := lossByCategory(t, result.Conversion.Losses, "empty_tool_results")
			if progress.Count != 1 {
				t.Errorf("empty_tool_results = %+v, want the one progress event the file does not hold",
					progress)
			}
			// The source's own words travel exactly: the second answer is written byte for byte, and the
			// record that carries a reasoning trace travels as the reader's rendering of it -- the trace's
			// own text between the reader's markers -- under the counted caveat, never removed and never
			// summarized.
			if !holdsText(texts, openCodeSourceAnswer) {
				t.Errorf("the installed file does not hold the fixture's own answer %q: %v",
					openCodeSourceAnswer, texts)
			}
			if !holdsText(texts, openCodeSourceRenderedText) {
				t.Errorf("the installed file does not hold the reader's rendering of the reasoning record: %v",
					texts)
			}
			if !holdsText(texts, openCodeSourceQuestion) || !holdsText(texts, openCodeSourceFollowup) {
				t.Errorf("the installed file does not hold the fixture's own prompts: %v", texts)
			}
		})
	}
}

// collectTexts appends every text a target message's content carries, in a Codex rollout's shape (a list of
// objects with a `text` member).
func collectTexts(texts *[]string, content any) {
	blocks, ok := content.([]any)
	if !ok {
		return
	}
	for _, block := range blocks {
		if object, ok := block.(map[string]any); ok {
			if text, ok := object["text"].(string); ok {
				*texts = append(*texts, text)
			}
		}
	}
}

// collectBlocks appends what a Claude transcript message's own blocks carry: a tool_use block's name and
// input object, a tool_result block's text, and every block's text.
func collectBlocks(
	names, arguments, outputs *[]string, outputsForCall *int, texts *[]string, message map[string]any,
) {
	if text, ok := message["content"].(string); ok {
		*texts = append(*texts, text)
		return
	}
	blocks, ok := message["content"].([]any)
	if !ok {
		return
	}
	for _, block := range blocks {
		object, ok := block.(map[string]any)
		if !ok {
			continue
		}
		switch object["type"] {
		case "tool_use":
			name, _ := object["name"].(string)
			encoded, err := json.Marshal(object["input"])
			if err != nil {
				continue
			}
			*names = append(*names, name)
			*arguments = append(*arguments, string(encoded))
		case "tool_result":
			if _, ok := object["content"].(string); !ok {
				continue
			}
			*outputs = append(*outputs, object["content"].(string))
			if callID, _ := object["tool_use_id"].(string); callID == openCodeSourceCallID {
				*outputsForCall++
			}
		case "text":
			if text, ok := object["text"].(string); ok {
				*texts = append(*texts, text)
			}
		}
	}
}

// sameJSONValue reports whether two JSON spellings carry the same value, which is the comparison a target's
// own encoding allows: a writer must not change the recorded arguments, and how it spells them is its own.
func sameJSONValue(t *testing.T, left, right string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(left), &a); err != nil {
		t.Fatalf("parsing %q: %v", left, err)
	}
	if err := json.Unmarshal([]byte(right), &b); err != nil {
		t.Fatalf("parsing %q: %v", right, err)
	}
	return jsonEqual(a, b)
}

// jsonEqual compares two decoded JSON values.
func jsonEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

// holdsText reports whether one of the collected texts is exactly this text.
func holdsText(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}

// TestOpenCodeSourceStoreIsNeverOpenedForWriting pins the non-destructive half of the route for a source
// that is a store rather than a file: reading it and converting it leave its bytes and its directory exactly
// as they were -- no write into the store, and no journal or write-ahead file created beside it.
func TestOpenCodeSourceStoreIsNeverOpenedForWriting(t *testing.T) {
	document := openCodeSourceFixtureDocument(t)
	root := openCodeSourceStore(t, document)
	cfg := openCodeSourceConfig(t, root)
	before := hashTree(t, root)

	conversation := readOpenCodeSourceFixture(t, root)
	if got := Summarize(conversation.Messages); got.Messages == 0 {
		t.Fatal("the fixture store read no messages at all")
	}
	for _, target := range []string{TargetCodex, TargetClaude} {
		if _, err := Continue(context.Background(), ContinueOptions{
			Ref:          openCodeSourceFixtureRef,
			Target:       target,
			Config:       cfg,
			TargetHome:   t.TempDir(),
			TargetBinary: missingTargetBinary,
		}); err != nil {
			t.Fatalf("continuing %s into %s: %v", openCodeSourceFixtureRef.ID, target, err)
		}
	}

	after := hashTree(t, root)
	if len(after) != len(before) {
		t.Fatalf("the source store directory holds %d file(s) after the conversions, want %d: %v",
			len(after), len(before), after)
	}
	for path, digest := range before {
		if after[path] != digest {
			t.Errorf("the source store's %s moved: %s -> %s", path, digest, after[path])
		}
	}
}
