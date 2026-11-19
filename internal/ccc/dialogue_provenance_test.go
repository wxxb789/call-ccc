package ccc

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/parser"
)

// The dialogue boundary: pinned at the seam every target reads its message text through (dialogueOf),
// through the readers that produce each shape, and at the writers that act on it.
//
// The closure criterion these cases discharge: for every route that claims exact dialogue, the text the
// target receives can be traced to a source-owned answer that is independent of the display rendering --
// the reader's own dialogue projection, or Content where the reader records Content as the source wrote
// it -- and a reader's display markers or joins may change without changing what any target receives as
// dialogue. Two consequences are pinned here:
//
//   - a record the read projected keeps the projection byte for byte (an explicitly empty projection
//     stays empty), whatever the reader's Content spells beside it; and
//   - a record whose flattened Content mixes the reader's rendering of the record's own reasoning trace
//     with the record's own text is not guessed at: no target publishes any of it as the model's words,
//     and the conversion counts the record (unresolved_dialogue) instead of claiming a conversion it
//     cannot support.
//
// Where a case's expected text is an answer the source owns, it is spelled from the native fixture's own
// typed parts -- read by the test itself -- and never from the shared rule's answer or from the writer's
// output. A native round trip proves the target stored what it was given; it does not prove the
// converter gave it the right words, so no case here uses the writer's own items as its expectation.
func TestDialogueOfNamesWhatEstablishesARecordsText(t *testing.T) {
	const (
		trace   = "Inspect the parser before answering."
		spoken  = "The retry cap is four attempts."
		foreign = "words the read answered no trace for"
	)
	rendering := func(text string) string {
		return thinkingRenderOpen + "\n" + text + "\n" + thinkingRenderClose
	}
	cases := []struct {
		name    string
		message Message
		want    string
		state   dialogueState
	}{
		// The projection is the authority, and it is asked before Content at all.
		{"the projection is the text whatever the rendering spells beside it",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Dialogue: spoken, DialogueProjected: true,
				Content: rendering(trace) + "\n" + spoken},
			spoken, dialogueProjected},
		{"an explicitly empty projection leaves the record with no dialogue",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				DialogueProjected: true, Content: rendering(trace)},
			"", dialogueProjected},
		{"a document that records its text on Dialogue without the flag keeps it",
			Message{Role: "user", Content: "Ask", Dialogue: "Ask"},
			"Ask", dialogueProjected},
		// No projection: Content is the record's own bytes only where nothing shows a rendering in it.
		{"a reader that renders nothing records Content as the source wrote it",
			Message{Role: "assistant", Thinking: trace, Content: rendering(trace)},
			rendering(trace), dialogueFromContent},
		{"a reader that renders the trace, on a record that carries none",
			Message{Role: "assistant", ContentRendersThinking: true, Content: spoken},
			spoken, dialogueFromContent},
		{"nothing stands in Content, so there are no bytes whose ownership could be wrong",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true},
			"", dialogueFromContent},
		// Content that is exactly the reader's own rendering of the record's whole trace.
		{"Content is byte for byte the reader's rendering of the record's whole trace",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content: rendering(trace)},
			"", dialogueRenderingOnly},
		{"the same block with one byte beside it is not that shape",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content: rendering(trace) + "\n"},
			"", dialogueUnestablished},
		{"a block whose words are a substring of the trace is not that shape either",
			Message{Role: "assistant", Thinking: "private reasoning", ContentRendersThinking: true,
				Content: rendering("private")},
			"", dialogueUnestablished},
		{"a block the record's trace does not hold is not that shape either",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content: rendering(foreign)},
			"", dialogueUnestablished},
		// Nothing established: the reader renders the trace into Content and the record carries one, or
		// the record carries calls. Several arms below are shapes review rounds and a consult produced as
		// counterexamples to the removal machinery this rule replaces; none of them is guessed at now,
		// whatever the bytes spell, because the reader's rendering is written from the trace's own text
		// and two different native part orders produce the same Content and the same trace.
		{"the rendering stands before the record's text (the readers' own order)",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content: rendering(trace) + "\n" + spoken},
			"", dialogueUnestablished},
		{"the rendering stands after the text it followed (ZCode's part order)",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content: spoken + "\n" + rendering(trace)},
			"", dialogueUnestablished},
		{"one trace rendered as two blocks around the record's text",
			Message{Role: "assistant", Thinking: "part one\n\npart two", ContentRendersThinking: true,
				Content: rendering("part one") + "\n" + spoken + "\n" + rendering("part two")},
			"", dialogueUnestablished},
		{"a text part that quotes the whole trace beside its rendering (round 12's shape)",
			Message{Role: "assistant", Thinking: "private reasoning", ContentRendersThinking: true,
				Content: "Before\n" + rendering("private reasoning") + "\nAfter\n" +
					rendering("private reasoning")},
			"", dialogueUnestablished},
		{"a text part that ends with an opener the rendering's closer closes (round 13's finding)",
			Message{Role: "assistant", Thinking: "private", ContentRendersThinking: true,
				Content: "Before\n" + rendering("private") + "\nAfter\n" + thinkingRenderOpen + "\n" +
					rendering("private")},
			"", dialogueUnestablished},
		{"a trace whose own text holds a closer, beside a literal block (round 14's finding)",
			Message{Role: "assistant", Thinking: "alpha\n[/Thinking]\nbeta", ContentRendersThinking: true,
				Content: rendering("alpha\n[/Thinking]\nbeta") + "\n" + "Before\n" + rendering("beta") +
					"\nAfter"},
			"", dialogueUnestablished},
		{"a thinking part whose own edge whitespace the reading used to trim (round 15's finding)",
			Message{Role: "assistant", Thinking: "alpha \n\nbeta", ContentRendersThinking: true,
				Content: rendering("alpha ") + "\n" + rendering("beta") + "\n" +
					"Before\n" + rendering("alpha \n\nbeta") + "\nAfter"},
			"", dialogueUnestablished},
		{"a text part quoting the whole trace, beside a trace part of its own (the consult's arm 2)",
			Message{Role: "assistant", Thinking: "a\n\n\n", ContentRendersThinking: true,
				Content: rendering("a\n\n\n") + "\n" + rendering("a") + "\n" + spoken},
			"", dialogueUnestablished},
		{"a record carrying calls has nothing established from its Content",
			Message{Role: "assistant", Content: "calling",
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read"}}},
			"", dialogueUnestablished},
		{"a call-carrying record whose reader also renders its trace has nothing established either",
			Message{Role: "assistant", Thinking: trace, ContentRendersThinking: true,
				Content:   rendering(trace) + "\n" + spoken,
				ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read"}}},
			"", dialogueUnestablished},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := dialogueOf(testCase.message)
			if got.Text != testCase.want || got.State != testCase.state {
				t.Errorf("dialogueOf(Content %q, Thinking %q, projected %v, renders %v, %d call(s)) = "+
					"(%q, %v), want (%q, %v)", testCase.message.Content, testCase.message.Thinking,
					testCase.message.DialogueProjected, testCase.message.ContentRendersThinking,
					len(testCase.message.ToolCalls), got.Text, got.State, testCase.want, testCase.state)
			}
		})
	}
}

// TestDialogueOfIgnoresTheDisplayRenderingOfAProjectedRecord is the closure criterion's own arm: two
// records whose source answered the same projection and whose reader's display rendering differs -- the
// rendering before the text and after it, joined differently, with a literal marker block beside it --
// reach every target as the same dialogue, byte for byte. A reader changing how it spells Content
// therefore cannot change what a target receives, as long as its projection does not change.
func TestDialogueOfIgnoresTheDisplayRenderingOfAProjectedRecord(t *testing.T) {
	const (
		trace  = "Inspect the parser before answering."
		spoken = "The retry cap is four attempts."
	)
	rendering := func(text string) string {
		return thinkingRenderOpen + "\n" + text + "\n" + thinkingRenderClose
	}
	spellings := map[string]string{
		"the rendering stands before the text": rendering(trace) + "\n" + spoken,
		"the rendering stands after the text":  spoken + "\n" + rendering(trace),
		"two blocks around the text":           rendering("part one") + "\n" + spoken + "\n" + rendering("part two"),
		"a literal marker block beside it":     spoken + "\n" + rendering("words the trace does not hold"),
		"the rendering alone":                  rendering(trace),
	}
	for name, content := range spellings {
		t.Run(name, func(t *testing.T) {
			message := Message{Role: "assistant", Content: content, Thinking: trace,
				ContentRendersThinking: true, Dialogue: spoken, DialogueProjected: true}
			if got := dialogueOf(message); got.Text != spoken || got.State != dialogueProjected {
				t.Errorf("dialogueOf over Content %q = (%q, %v), want the projection %q byte for byte "+
					"(%v)", content, got.Text, got.State, spoken, dialogueProjected)
			}
			// The same document through a writer: the text the target receives is the projection, and
			// none of the reader's rendering reaches it.
			rollout, err := ConvertToCodexRollout(testConversation(message), testOptions())
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			var texts []string
			for _, item := range rollout.Items {
				if item.Kind == ItemAssistant {
					texts = append(texts, item.Text)
				}
			}
			if len(texts) != 1 || texts[0] != spoken {
				t.Errorf("the rollout's assistant text = %q, want the projection %q", texts, spoken)
			}
			if strings.Contains(string(rollout.Data), trace) {
				t.Errorf("the rollout holds the record's persisted trace, which is not dialogue and not a "+
					"shape this target carries: %s", rollout.Data)
			}
		})
	}
}

// TestCodexRolloutCountsWhatItCouldNotEstablish pins the report's half of the rule on a hand-built
// document: a record whose dialogue the read could not establish contributes no text and is counted
// (unresolved_dialogue, omitted); a record whose text is the reader's Content rather than a projection
// travels and is counted as that (unprojected_dialogue, carried-with-caveat); and a record whose Content
// is exactly its reader's rendering of its own trace contributes no text and is not counted as a lost
// dialogue, because the rendering is the trace the read carries as the record's reasoning.
func TestCodexRolloutCountsWhatItCouldNotEstablish(t *testing.T) {
	rendering := thinkingRenderOpen + "\nprivate trace\n" + thinkingRenderClose
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask",
			DialogueProjected: true},
		// Unprojected, the reader renders the trace into Content, and the record carries one: nothing is
		// established, so nothing is written.
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Thinking: "private trace",
			ContentRendersThinking: true, Content: rendering + "\nThe answer."},
		// Unprojected, the reader renders nothing: Content is the record's own bytes and travels, with
		// the report naming that it is not a source-owned projection.
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: "A plain answer."},
		// Unprojected, and Content is exactly the reader's rendering of the record's whole trace.
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Thinking: "only a trace",
			ContentRendersThinking: true,
			Content:                thinkingRenderOpen + "\nonly a trace\n" + thinkingRenderClose},
	)
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var texts []string
	for _, item := range rollout.Items {
		if item.Kind == ItemAssistant {
			texts = append(texts, item.Text)
		}
	}
	if len(texts) != 1 || texts[0] != "A plain answer." {
		t.Errorf("the rollout's assistant text = %q, want only the record whose Content is its own bytes",
			texts)
	}
	if strings.Contains(string(rollout.Data), rendering) || strings.Contains(string(rollout.Data), "The answer.") {
		t.Errorf("the rollout holds a record's Content the read could not establish as its text: %s",
			rollout.Data)
	}
	if strings.Contains(string(rollout.Data), "only a trace") {
		t.Error("the rollout holds the reader's rendering of the reasoning-only record's trace as " +
			"dialogue, which states words the model never wrote")
	}
	for category, want := range map[string]struct {
		count       int
		disposition string
	}{
		"unresolved_dialogue":  {1, DispositionOmitted},
		"unprojected_dialogue": {1, DispositionCarriedWithCaveat},
		"reasoning_text":       {2, DispositionOmitted},
	} {
		loss := lossCount(t, rollout.Losses, category)
		if loss.Count != want.count || loss.Disposition != want.disposition {
			t.Errorf("%s = %+v, want count %d and disposition %q", category, loss, want.count,
				want.disposition)
		}
	}
}

// dialogueRoute is one route's own conversion of a hand-built document, as the dialogue cases below
// read it: the items and the bytes it wrote, its report, and the shape its format has for a persisted
// reasoning trace -- empty where the target records reasoning as its provider's own artifact, so a
// plaintext trace has nowhere to travel.
type dialogueRoute struct {
	name       string
	traceShape string
	items      []TargetItem
	data       []byte
	losses     []Loss
}

// dialogueRoutes runs one hand-built document through all four target writers, so a case can state what
// the dialogue rule requires of every route and see each route's own answer. Every writer reads its
// message text through dialogueOf and counts it through dialogueLossOf, so the same document answers the
// same state on all four routes; what differs per route -- its shape for a reasoning trace, its
// categories' own wording -- is the route's own and is what the cases assert.
func dialogueRoutes(t *testing.T, conversation Conversation) []dialogueRoute {
	t.Helper()
	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting to Codex: %v", err)
	}
	transcript, err := ConvertToClaudeTranscript(conversation, ClaudeTargetOptions{})
	if err != nil {
		t.Fatalf("converting to Claude: %v", err)
	}
	session, err := ConvertToPiSession(conversation, PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting to Pi: %v", err)
	}
	openCode, err := ConvertToOpenCodeSession(conversation, OpenCodeTargetOptions{})
	if err != nil {
		t.Fatalf("converting to OpenCode: %v", err)
	}
	return []dialogueRoute{
		{name: "codex", items: rollout.Items, data: rollout.Data, losses: rollout.Losses},
		{name: "claude", items: transcript.Items, data: transcript.Data, losses: transcript.Losses},
		{name: "pi", traceShape: `"type":"thinking"`, items: session.Items, data: session.Data,
			losses: session.Losses},
		{name: "opencode", traceShape: `"type":"reasoning"`, items: openCode.Items, data: openCode.Data,
			losses: openCode.Losses},
	}
}

// TestEveryRouteCountsWhatTheReadCouldNotEstablish drives the case above's document once per route: the
// read could not establish the record's text, so no route writes it and every route counts it
// (unresolved_dialogue). What each route does with the rest is the route's own and is pinned here: the
// record's tool evidence travels on every route, the trace travels exactly where the format has a shape
// for it (Pi's `thinking` block, the harness's reasoning part) and is counted as omitted where it has
// none (Codex and Claude record reasoning as their provider's own artifact), and each route states the
// same counts under its own categories. The unresolved_dialogue row's clause is prose, but the fact it
// states is checked here: a route whose reasoning row says the trace is omitted must not hold it. The
// document also carries a user-role record's trace, which no format here has a shape for: it is absent
// from every document and counted as omitted under each route's own category for it
// (non_assistant_reasoning on the two routes that write a trace at all), never claimed as carried.
func TestEveryRouteCountsWhatTheReadCouldNotEstablish(t *testing.T) {
	const (
		trace  = "private trace"
		answer = "The answer."
		plain  = "A plain answer."
		// A trace recorded on a user record, which a carried reader really does produce. No format
		// below writes a trace outside the assistant message that produced it, so this record's trace
		// has no shape on any route and must be counted as the omission it is, never as carried.
		userTrace = "user trace"
	)
	rendering := thinkingRenderOpen + "\n" + trace + "\n" + thinkingRenderClose
	conversation := testConversation(
		Message{Ordinal: 0, Role: string(parser.RoleUser), Content: "Ask", Dialogue: "Ask",
			DialogueProjected: true},
		// Unprojected, the reader renders the trace into Content, and the record carries a call with its
		// result: nothing about the record's text is established, on any route.
		Message{Ordinal: 1, Role: string(parser.RoleAssistant), Thinking: trace,
			ContentRendersThinking: true, Content: rendering + "\n" + answer,
			ToolCalls: []ToolCall{{ToolUseID: "toolu_1", Name: "Read",
				InputJSON: `{"file_path":"/work/project/policy.go"}`}},
			ToolResults: []ToolResult{{ToolUseID: "toolu_1", ContentRaw: `"recorded output"`,
				ContentSize: 16}}},
		// Unprojected, the reader renders nothing: Content is the record's own bytes and travels.
		Message{Ordinal: 2, Role: string(parser.RoleAssistant), Content: plain},
		// Unprojected, and Content is exactly the reader's rendering of the record's whole trace: no
		// dialogue, and the rendering is not double-carried.
		Message{Ordinal: 3, Role: string(parser.RoleAssistant), Thinking: "only a trace",
			ContentRendersThinking: true,
			Content:                thinkingRenderOpen + "\nonly a trace\n" + thinkingRenderClose},
		// A user record carrying a trace of its own, with a projection so its dialogue is not the
		// question this case asks.
		Message{Ordinal: 4, Role: string(parser.RoleUser), Content: "Ask again", Dialogue: "Ask again",
			DialogueProjected: true, Thinking: userTrace},
	)
	for _, route := range dialogueRoutes(t, conversation) {
		t.Run(route.name, func(t *testing.T) {
			// No text is written for the unestablished record: neither the record's own words nor the
			// reader's rendering of the trace reaches the document, and no item holds the words as
			// written text.
			for _, forbidden := range []string{rendering + "\n" + answer, answer} {
				if strings.Contains(string(route.data), forbidden) {
					t.Errorf("the document holds %q, which the read could not establish as the record's "+
						"text: %s", forbidden, route.data)
				}
			}
			for _, item := range route.items {
				if strings.Contains(item.Text, answer) {
					t.Errorf("an item holds %q as written text: %+v", answer, item)
				}
			}
			// The trace travels where the target has a shape for it, and only there.
			hasTrace := strings.Contains(string(route.data), trace)
			if wantTrace := route.traceShape != ""; hasTrace != wantTrace {
				t.Errorf("the document holds the record's trace = %v, want %v (this route's shape for "+
					"a trace: %q)", hasTrace, wantTrace, route.traceShape)
			}
			if route.traceShape != "" && !strings.Contains(string(route.data), route.traceShape) {
				t.Errorf("the document holds the trace but not in this route's own shape %q: %s",
					route.traceShape, route.data)
			}
			// The user record's trace has no shape on any of these formats: each writes a trace
			// inside the assistant message that produced it (Pi, OpenCode) or has no plaintext-trace
			// shape at all (Codex, Claude). It is absent from every document, whatever the role-blind
			// reasoning count used to say.
			if strings.Contains(string(route.data), userTrace) {
				t.Errorf("the document holds the user record's trace %q, which this route's format "+
					"cannot carry outside an assistant message: %s", userTrace, route.data)
			}
			// The tool evidence travels on every route: the call, its recorded output, and an item
			// naming the result.
			for _, want := range []string{"toolu_1", "recorded output"} {
				if !strings.Contains(string(route.data), want) {
					t.Errorf("the document does not hold the record's tool evidence %q: %s", want,
						route.data)
				}
			}
			wroteResult := false
			for _, item := range route.items {
				wroteResult = wroteResult || item.Kind == ItemToolResult
			}
			if !wroteResult {
				t.Errorf("no item names the record's tool result: %+v", route.items)
			}
			// Each route's counts are the route's own: the one unestablished record and the one record
			// whose text is the reader's Content, under the shared categories.
			for category, want := range map[string]struct {
				count       int
				disposition string
			}{
				"unresolved_dialogue":  {1, DispositionOmitted},
				"unprojected_dialogue": {1, DispositionCarriedWithCaveat},
			} {
				loss := lossCount(t, route.losses, category)
				if loss.Count != want.count || loss.Disposition != want.disposition {
					t.Errorf("%s = %+v, want count %d and disposition %q", category, loss, want.count,
						want.disposition)
				}
			}
			// Every trace the records carry is accounted for by a count, and the count says what the
			// document holds: the two assistant records' traces travel on the routes that have the
			// shape for them (carried with a caveat), and are omitted on the routes that have none
			// (Codex and Claude record reasoning as their provider's own artifact) -- where the user
			// record's trace is omitted too, inside the same count.
			reasoning := lossCount(t, route.losses, "reasoning_text")
			wantReasoningCount, wantReasoningDisposition := 2, DispositionCarriedWithCaveat
			if route.traceShape == "" {
				wantReasoningCount, wantReasoningDisposition = 3, DispositionOmitted
			}
			if reasoning.Count != wantReasoningCount || reasoning.Disposition != wantReasoningDisposition {
				t.Errorf("reasoning_text = %+v, want %d trace(s) counted %q beside this route's trace "+
					"shape %q", reasoning, wantReasoningCount, wantReasoningDisposition, route.traceShape)
			}
			if route.traceShape != "" {
				// The user record's trace has no shape in either format that has one (each writes a
				// trace inside the assistant message that produced it), so it is counted as omitted
				// under the route's own category, never claimed as carried by reasoning_text.
				nonAssistant := lossCount(t, route.losses, "non_assistant_reasoning")
				if nonAssistant.Count != 1 || nonAssistant.Disposition != DispositionOmitted {
					t.Errorf("non_assistant_reasoning = %+v, want the user record's trace counted as one "+
						"omission", nonAssistant)
				}
			}
		})
	}
}

// TestWhitespaceOnlyContentKeepsItsOwnCategory pins a shape the dialogue rule must not fold into "no
// text": Content that is whitespace alone, with no projection, is the record's own bytes -- the
// dialogueFromContent state carrying those bytes -- so each route reports it under the category it
// reached before the rule replaced the readings (round 17's PD-1: folding it in dropped such an
// assistant record from Codex's report with no category at all, and the other three routes called it
// empty). Codex, Claude and OpenCode name it as the source's whitespace dialogue; Pi, whose reader drops
// a record left with neither text nor tool evidence, counts it under its own empty-record category for
// the role. No route writes the whitespace either way -- what is pinned is the disclosure.
func TestWhitespaceOnlyContentKeepsItsOwnCategory(t *testing.T) {
	const blank = "   "
	for _, testCase := range []struct {
		name       string
		role       parser.RoleType
		piCategory string
	}{
		{"assistant", parser.RoleAssistant, "empty_assistant_messages"},
		{"user", parser.RoleUser, "empty_user_messages"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			message := Message{Ordinal: 0, Role: string(testCase.role), Content: blank}
			if got := dialogueOf(message); got.State != dialogueFromContent || got.Text != blank {
				t.Fatalf("dialogueOf over a whitespace-only record = (%q, %v), want the record's own "+
					"bytes %q (%v)", got.Text, got.State, blank, dialogueFromContent)
			}
			for _, route := range dialogueRoutes(t, testConversation(message)) {
				category := "whitespace_only_dialogue"
				if route.name == "pi" {
					category = testCase.piCategory
				}
				loss := lossCount(t, route.losses, category)
				if loss.Count != 1 || loss.Disposition != DispositionOmitted {
					t.Errorf("%s = %+v, want the one record counted as omitted", category, loss)
				}
				// No dialogue category claims the text travelled or was left unestablished, and nothing
				// is written for the record: the target's own reader reads a message whose text is blank
				// after trimming as no message at all.
				for _, other := range []string{"unprojected_dialogue", "unresolved_dialogue"} {
					if loss := lossCount(t, route.losses, other); loss.Count != 0 {
						t.Errorf("%s = %+v, want none: the record's Content is its own bytes, and no "+
							"route writes them", other, loss)
					}
				}
				if len(route.items) != 0 {
					t.Errorf("the conversion wrote %+v for a record whose whole text is whitespace",
						route.items)
				}
			}
		})
	}
}

// TestCodexRolloutCarriesTheClaudeFixturesOwnDialogue traces the target's text to the source's own typed
// parts for a whole committed session. The expectation is built here from the fixture file's own
// records: every user or assistant record's `text` blocks, joined as the inherited reader joins them
// (one feed), with a record that holds a `tool_use` or `tool_result` block contributing no dialogue (its
// rendering `[Read: …]` is a display artifact of the call, which travels as the call it is) and the one
// compaction summary traveling as the target's own boundary record rather than as dialogue. Nothing here
// reads the answer out of the shared rule or out of the conversion: the fixture's bytes are the
// authority, and the case shows both that the projections the read answered equal them and that the
// rollout's written dialogue equals them.
func TestCodexRolloutCarriesTheClaudeFixturesOwnDialogue(t *testing.T) {
	path := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	expected, renderings := claudeFixtureDialogue(t, path)
	if len(expected) == 0 || len(renderings) == 0 {
		t.Fatalf("the fixture walk answered %d dialogue text(s) and %d rendering(s), so nothing here "+
			"would be pinned", len(expected), len(renderings))
	}
	conversation := readFixtureConversation(t, claudeFixtureRef)

	// The reader's own answer, record by record: a projection that equals the fixture's own typed parts.
	var projected []string
	for _, message := range conversation.Messages {
		if message.IsCompactBoundary || len(message.ToolCalls) > 0 {
			continue
		}
		if !message.DialogueProjected {
			continue
		}
		if strings.TrimSpace(message.Dialogue) == "" {
			continue
		}
		projected = append(projected, message.Dialogue)
	}
	if strings.Join(projected, "\x00") != strings.Join(expected, "\x00") {
		t.Errorf("the read's projections = %q, want the fixture's own text blocks %q", projected, expected)
	}

	rollout, err := ConvertToCodexRollout(conversation, testOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var texts []string
	for _, item := range rollout.Items {
		switch item.Kind {
		case ItemUser, ItemAssistant:
			texts = append(texts, item.Text)
		}
	}
	if strings.Join(texts, "\x00") != strings.Join(expected, "\x00") {
		t.Errorf("the rollout's dialogue = %q, want the fixture's own text blocks %q", texts, expected)
	}
	for _, rendering := range renderings {
		if strings.Contains(string(rollout.Data), rendering) {
			t.Errorf("the rollout holds the reader's own rendering %q of a tool call, which is not "+
				"dialogue the source recorded", rendering)
		}
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_dialogue"); loss.Count != 0 {
		t.Errorf("unresolved_dialogue = %+v, want none: every dialogue record of this fixture answered a "+
			"projection", loss)
	}
}

// claudeFixtureDialogue answers the fixture's own records' dialogue and the renderings that must not
// reach a target, both read from the file's bytes: the last line is the committed fixture's torn trailing
// record, which the reader reports as a malformed line and which this walk stops at rather than guesses
// about.
func claudeFixtureDialogue(t *testing.T, path string) (dialogue, renderings []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	type block struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	type record struct {
		Type             string          `json:"type"`
		IsCompactSummary bool            `json:"isCompactSummary"`
		Message          json.RawMessage `json:"message"`
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry record
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// The fixture's committed torn last line: everything before it was read above.
			break
		}
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		var message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &message); err != nil {
			t.Fatalf("decoding the fixture record: %v", err)
		}
		if entry.IsCompactSummary {
			// A compaction boundary: the target's own record for it, not a message's dialogue.
			continue
		}
		switch content := message.Content.(type) {
		case string:
			dialogue = append(dialogue, content)
		case []any:
			var texts []string
			for _, raw := range content {
				encoded, err := json.Marshal(raw)
				if err != nil {
					t.Fatalf("re-encoding a fixture block: %v", err)
				}
				var parsed block
				if err := json.Unmarshal(encoded, &parsed); err != nil {
					t.Fatalf("decoding a fixture block: %v", err)
				}
				switch parsed.Type {
				case "text":
					texts = append(texts, parsed.Text)
				case "tool_use":
					// The call travels as the call it is; the text the reader inlines for it in
					// Content is the reader's own rendering, and it is what must not travel as
					// dialogue. The rendering is spelled with the inherited reader's own function
					// (parser.ToolUseRendering) over this block's own name and input. A tool_result
					// block puts no rendering in Content (ExtractTextContent collects it without
					// appending anything), so only a call's block is walked here.
					if rendered := parser.ToolUseRendering(parsed.Name, string(parsed.Input)); rendered != "" {
						renderings = append(renderings, rendered)
					}
				}
			}
			if len(texts) > 0 {
				dialogue = append(dialogue, strings.Join(texts, "\n"))
			}
		}
	}
	return dialogue, renderings
}

// TestZCodeRecordsAreNotGuessedThroughTheUnmodifiedReader pins the refusal at the route that produces
// the shape, through internal/parser's ZCode reader itself: an assistant record carrying a persisted
// thinking part and a text part answers no dialogue projection, and its Content is the reader's own
// rendering of the trace beside the record's text -- one flattened string that two different native part
// orders produce. Each arm below is a shape a review round (13, 14, 15) or a cross-family consult
// produced as a counterexample to the removal machinery the shared rule used to run; what the arms now
// assert is that the shape is not read at all: no text reaches the target, the record is counted as
// unresolved, and the bytes stay where the source recorded them.
func TestZCodeRecordsAreNotGuessedThroughTheUnmodifiedReader(t *testing.T) {
	const (
		trace  = "private reasoning"
		answer = "The retry cap is four attempts."
	)
	thinkingPart := `{"type":"thinking","thinking":` + jsonString(t, trace) + `}`
	textPart := func(value string) string {
		return `{"type":"text","text":` + jsonString(t, value) + `}`
	}
	rendered := thinkingRenderOpen + "\n" + trace + "\n" + thinkingRenderClose
	// The shapes, each as the parts a ZCode store records: a plain mixed record in both part orders, a
	// text part that quotes the whole trace, one whose own text holds the closer marker, one that ends
	// with an opener the rendering closes, and a trace whose own text is the rendering's whole words with
	// a text part quoting it.
	cases := []struct {
		name      string
		parts     []string
		text      string
		wantBytes string
	}{
		{"the trace part stands before the record's text", []string{thinkingPart, textPart(answer)},
			answer, rendered + "\n" + answer},
		{"the trace part stands after the record's text", []string{textPart(answer), thinkingPart},
			answer, answer + "\n" + rendered},
		{"a text part quotes the whole rendering", []string{thinkingPart, textPart(rendered)},
			rendered, rendered + "\n" + rendered},
		{"a text part ends with an opener the rendering closes",
			[]string{thinkingPart, textPart("Before\n" + thinkingRenderOpen)},
			"Before\n" + thinkingRenderOpen, rendered + "\n" + "Before\n" + thinkingRenderOpen},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			conversation := zCodeMixedConversation(t, testCase.parts...)
			sawRecord := false
			for _, message := range conversation.Messages {
				if message.Thinking != trace {
					continue
				}
				sawRecord = true
				if message.DialogueProjected || message.Dialogue != "" {
					t.Fatalf("the ZCode record's dialogue = %q (projected %v), want none: this shape is a "+
						"record the source answered no projection for", message.Dialogue,
						message.DialogueProjected)
				}
				if !message.ContentRendersThinking {
					t.Fatal("the read did not establish the reader's rendering provenance for the record, " +
						"so nothing here drives the rule this case is about")
				}
				// The reader's own shape, spelled from the parts this case wrote: the parts' bytes, joined
				// as the reader joins them (one feed).
				if message.Content != testCase.wantBytes {
					t.Fatalf("the ZCode record's content = %q, want the reader's own parts' bytes %q",
						message.Content, testCase.wantBytes)
				}
				if got := dialogueOf(message); got.State != dialogueUnestablished || got.Text != "" {
					t.Errorf("dialogueOf over the ZCode record = (%q, %v), want no text and the "+
						"unestablished state: the record's own text and the reader's rendering of the "+
						"trace are one string", got.Text, got.State)
				}
			}
			if !sawRecord {
				t.Fatalf("the read holds no record carrying %q, so nothing here pins the shape this case "+
					"is about", trace)
			}
			rollout, err := ConvertToCodexRollout(conversation, testOptions())
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			var texts []string
			for _, item := range rollout.Items {
				if item.Kind == ItemAssistant {
					texts = append(texts, item.Text)
				}
			}
			if len(texts) != 0 {
				t.Errorf("the rollout's assistant text = %q, want none: the record's dialogue was not "+
					"established, and publishing the reader's rendering as the model's words is what this "+
					"rule refuses", texts)
			}
			if testCase.text != "" && strings.Contains(string(rollout.Data), testCase.text) {
				t.Errorf("the rollout holds the record's Content, which the read could not establish as "+
					"its text: %s", rollout.Data)
			}
			if loss := lossCount(t, rollout.Losses, "unresolved_dialogue"); loss.Count != 1 ||
				loss.Disposition != DispositionOmitted {
				t.Errorf("unresolved_dialogue = %+v, want the one record counted as omitted", loss)
			}
		})
	}

	// The control: the same reader on a record that carries no trace. Its Content is the record's own
	// text, because this reader renders nothing else into Content, so the text travels and the report
	// names that it is the reader's Content rather than a projection.
	plain := zCodeMixedConversation(t, textPart(answer))
	rollout, err := ConvertToCodexRollout(plain, testOptions())
	if err != nil {
		t.Fatalf("converting a record without a trace: %v", err)
	}
	var texts []string
	for _, item := range rollout.Items {
		if item.Kind == ItemAssistant {
			texts = append(texts, item.Text)
		}
	}
	if len(texts) != 1 || texts[0] != answer {
		t.Errorf("the rollout's assistant text = %q, want the record's own text %q", texts, answer)
	}
	if loss := lossCount(t, rollout.Losses, "unprojected_dialogue"); loss.Count != 2 ||
		loss.Disposition != DispositionCarriedWithCaveat {
		t.Errorf("unprojected_dialogue = %+v, want both records whose text is the reader's Content (the "+
			"session's prompt and the record's own text) counted as carried with a caveat", loss)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_dialogue"); loss.Count != 0 {
		t.Errorf("unresolved_dialogue = %+v, want none: the record carries no trace for the reader to "+
			"have rendered", loss)
	}
}

// TestRenderingReadersReasoningOnlyRecordKeepsItsTraceOnce pins the whole-content shape through the
// inherited reader that produces it: internal/parser's Cline reader answers a reasoning-only record as an
// assistant message whose Content is exactly `[Thinking]\n<trace>\n[/Thinking]` -- the reader's own
// rendering of the record's whole trace -- with no dialogue projection. The record has no dialogue, so
// the target carries the trace once, as the record's reasoning, and never as the rendering; a record of
// the same reader that also carries text is not established (its Content mixes the two with nothing to
// separate them), and the archive read of the same session answers the same states from the stored
// labels.
func TestRenderingReadersReasoningOnlyRecordKeepsItsTraceOnce(t *testing.T) {
	root := t.TempDir()
	sessionID := "cli-0001"
	sessionDir := filepath.Join(root, "data", "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("creating the session directory: %v", err)
	}
	metadata := `{"session_id":"` + sessionID + `","version":1,"model":"cline-model"}`
	messages := `{"version":1,"messages":[` +
		`{"id":"m1","role":"user","content":[{"type":"text","text":"Ask about the docs."}],` +
		`"ts":1780000000000},` +
		`{"id":"m2","role":"assistant","content":[{"type":"thinking","thinking":"reasoning only"}],` +
		`"ts":1780000001000,"modelInfo":{"id":"cline-model","provider":"openai"}},` +
		`{"id":"m3","role":"assistant","content":[{"type":"thinking","thinking":"private trace"},` +
		`{"type":"text","text":"The answer."}],"ts":1780000002000}]}`
	for name, contents := range map[string]string{
		sessionID + ".json":          metadata,
		sessionID + ".messages.json": messages,
	} {
		if err := os.WriteFile(filepath.Join(sessionDir, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs[parser.AgentCline] = []string{root}
	sessionPath := filepath.Join(sessionDir, sessionID+".json")
	conversation, err := NewSourceReader(cfg).Read(context.Background(),
		Ref{Provider: "cline", ID: sessionPath})
	if err != nil {
		t.Fatalf("reading the Cline session: %v", err)
	}
	if len(conversation.Messages) != 3 {
		t.Fatalf("the read answered %d message(s), want the transcript's three", len(conversation.Messages))
	}
	only := conversation.Messages[1]
	if only.Thinking != "reasoning only" || only.DialogueProjected {
		t.Fatalf("the reasoning-only record = (trace %q, projected %v), want the recorded trace and no "+
			"projection", only.Thinking, only.DialogueProjected)
	}
	if want := thinkingRenderOpen + "\nreasoning only\n" + thinkingRenderClose; only.Content != want {
		t.Fatalf("the reasoning-only record's content = %q, want the reader's own rendering %q",
			only.Content, want)
	}
	if got := dialogueOf(only); got.State != dialogueRenderingOnly || got.Text != "" {
		t.Errorf("dialogueOf over the reasoning-only record = (%q, %v), want no dialogue (%v)",
			got.Text, got.State, dialogueRenderingOnly)
	}
	mixed := conversation.Messages[2]
	if got := dialogueOf(mixed); got.State != dialogueUnestablished || got.Text != "" {
		t.Errorf("dialogueOf over the reader's mixed record = (%q, %v), want nothing established (%v)",
			got.Text, got.State, dialogueUnestablished)
	}

	// The archive read answers the same states from the stored row's agent label: the store holds what
	// the reader produced, so its label is the reader's name.
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, sessionPath); err != nil {
		t.Fatalf("ingesting the Cline session: %v", err)
	}
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	fromArchive, err := archive.Read(ctx, Ref{Provider: "cline", ID: "cline:" + sessionID})
	if err != nil {
		t.Fatalf("reading the Cline session from the archive: %v", err)
	}
	if len(fromArchive.Messages) != 3 {
		t.Fatalf("the archive read answered %d message(s), want the transcript's three",
			len(fromArchive.Messages))
	}
	if got := dialogueOf(fromArchive.Messages[1]); got.State != dialogueRenderingOnly {
		t.Errorf("dialogueOf over the archived reasoning-only record = (%q, %v), want no dialogue (%v)",
			got.Text, got.State, dialogueRenderingOnly)
	}
	if got := dialogueOf(fromArchive.Messages[2]); got.State != dialogueUnestablished {
		t.Errorf("dialogueOf over the archived mixed record = (%q, %v), want nothing established (%v)",
			got.Text, got.State, dialogueUnestablished)
	}
}

// TestRenderingReaderLabelsCoverEveryLabelTheirReadersServe pins the closed list the dialogue rule's
// reader table is: every label whose carried reader builds the record's persisted trace into Content is
// named in thinkingRenderingReaders, so a record of that reader the read answered no projection for (a
// legacy archive row, a path of the reader that records Content as the source wrote it) cannot publish
// the reader's rendering as the model's words. The groups below are the labels the shared readers serve;
// a label appended to a shared reader without its table entry fails here (round 3 of PR #9's machine
// review found the Pi family's three extra labels that way, and the sweep the round ran added the rest).
//
// The case then drives one of the named labels -- OMO, whose reader's projection a row the archive wrote
// before it stored the projection does not carry -- through the archive read and a conversion: the stored
// label is the reader's name, the read answers ContentRendersThinking for it, the record's text is not
// established (its Content is the reader's rendering of the trace beside the record's own text), and the
// conversion counts the record (unresolved_dialogue) instead of publishing the rendering as dialogue.
func TestRenderingReaderLabelsCoverEveryLabelTheirReadersServe(t *testing.T) {
	groups := map[string][]parser.AgentType{
		// One carried reader per group, named by the site that writes the block.
		"the Pi reader (pi.go:509-549)": {
			parser.AgentPi, parser.AgentOMP, parser.AgentOMO, parser.AgentStepCode,
			parser.AgentDirectAPI, parser.AgentPrimeAgent,
		},
		"the Kimi reader (kimi.go:374,590, shared with Kimi Work)": {
			parser.AgentKimi, parser.AgentKimiWork,
		},
		"the Codex reader (codex.go:658-668, shared with its variants)": {
			parser.AgentCodex, parser.AgentAugureCode, parser.AgentInterpreter, parser.AgentTraeX,
		},
		"the Hermes reader (hermes.go:319-321,517-519,1349-1351, shared with Augure Desktop)": {
			parser.AgentHermes, parser.AgentAugureDesktop,
		},
		"the OpenCode reader (opencode.go:1726-1731, opencode_v2.go:280-286, shared with Kilo and MiMo Code)": {
			parser.AgentOpenCode, parser.AgentKilo, parser.AgentMiMoCode,
		},
	}
	for reader, labels := range groups {
		for _, label := range labels {
			if !readerRendersThinkingTrace(label) {
				t.Errorf("%s serves the label %q and no table entry answers for it: a record of that "+
					"reader the read answered no projection for would travel with the rendering "+
					"carried as dialogue", reader, label)
			}
		}
	}
	// The default keeps the source's own words: a reader that records Content as the source wrote it,
	// and a label this build does not know, take no entry.
	for _, label := range []parser.AgentType{parser.AgentVibe, "a-label-this-build-does-not-know"} {
		if readerRendersThinkingTrace(label) {
			t.Errorf("the label %q answers that its reader renders the trace into Content", label)
		}
	}

	// The archive arm. A session of the OMO label whose assistant record carries a persisted trace and a
	// text part; the archive row is cleared of its stored projection below, which is the shape a row
	// written before the archive stored the projection has.
	const (
		trace   = "Inspect the parser before answering."
		spoken  = "The retry cap is four attempts."
		sessID  = "019c9c96-6ee7-77c0-ba4c-380f844289d5"
		session = `{"type":"session","version":3,"id":"` + sessID + `","timestamp":"2026-05-04T09:15:00.000Z","cwd":"/work/project"}` + "\n" +
			`{"type":"message","id":"m1","timestamp":"2026-05-04T09:15:01.000Z","message":{"role":"user","content":"Ask about the retry cap."}}` + "\n" +
			`{"type":"message","id":"m2","timestamp":"2026-05-04T09:15:02.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"` + trace + `"},{"type":"text","text":"` + spoken + `"}],"model":"pi-model"}}` + "\n"
	)
	root := t.TempDir()
	sessionPath := filepath.Join(root, "omo-session.jsonl")
	if err := os.WriteFile(sessionPath, []byte(session), 0o600); err != nil {
		t.Fatalf("writing the session: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs[parser.AgentOMO] = []string{root}
	source, err := NewSourceReader(cfg).Read(context.Background(), Ref{Provider: "omo", ID: sessionPath})
	if err != nil {
		t.Fatalf("reading the OMO session: %v", err)
	}
	if len(source.Messages) != 2 {
		t.Fatalf("the source read answered %d message(s), want the session's two", len(source.Messages))
	}
	if !source.Messages[1].DialogueProjected {
		t.Fatal("the source read answered no projection for the assistant record, so this case is not " +
			"about the legacy-row shape it pins")
	}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, sessionPath); err != nil {
		t.Fatalf("ingesting the OMO session: %v", err)
	}
	// Clear the stored projection: the row this case reads is the shape a row the archive wrote before
	// it stored the projection has, and clearing is what makes the read answer no projection for it.
	database, err := sql.Open("sqlite3", archivePath)
	if err != nil {
		t.Fatalf("opening the archive store: %v", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE messages SET dialogue_text = NULL"); err != nil {
		t.Fatalf("clearing the stored projection: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the archive store: %v", err)
	}

	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	fromArchive, err := archive.Read(ctx, Ref{Provider: "omo", ID: "omo:" + sessID})
	if err != nil {
		t.Fatalf("reading the OMO session from the archive: %v", err)
	}
	if len(fromArchive.Messages) != 2 {
		t.Fatalf("the archive read answered %d message(s), want the session's two",
			len(fromArchive.Messages))
	}
	record := fromArchive.Messages[1]
	rendering := thinkingRenderOpen + "\n" + trace + "\n" + thinkingRenderClose
	if record.Thinking != trace || record.DialogueProjected {
		t.Fatalf("the archived record = (trace %q, projected %v), want the stored trace and no "+
			"projection", record.Thinking, record.DialogueProjected)
	}
	if record.Content != rendering+"\n"+spoken {
		t.Fatalf("the archived record's content = %q, want the reader's own rendering beside the "+
			"record's text %q", record.Content, rendering+"\n"+spoken)
	}
	if !record.ContentRendersThinking {
		t.Fatal("the archive read did not name the record's rendering reader from the stored label: " +
			"the omo label is the Pi reader's, and the table is what the read answers this flag from")
	}
	if got := dialogueOf(record); got.State != dialogueUnestablished || got.Text != "" {
		t.Errorf("dialogueOf over the archived OMO record = (%q, %v), want nothing established (%v)",
			got.Text, got.State, dialogueUnestablished)
	}
	// The conversion's own answer: the record's text is not published, and the record is counted rather
	// than presented as a conversion it cannot support.
	rollout, err := ConvertToCodexRollout(fromArchive, testOptions())
	if err != nil {
		t.Fatalf("converting the archive read: %v", err)
	}
	var texts []string
	for _, item := range rollout.Items {
		if item.Kind == ItemAssistant {
			texts = append(texts, item.Text)
		}
	}
	if len(texts) != 0 {
		t.Errorf("the rollout's assistant text = %q, want none: the record's dialogue was not "+
			"established", texts)
	}
	if strings.Contains(string(rollout.Data), rendering) {
		t.Errorf("the rollout holds the reader's rendering although the report does not establish the "+
			"record's text:\n%s", rollout.Data)
	}
	if loss := lossCount(t, rollout.Losses, "unresolved_dialogue"); loss.Count != 1 ||
		loss.Disposition != DispositionOmitted {
		t.Errorf("unresolved_dialogue = %+v, want the one record counted as omitted", loss)
	}
}
