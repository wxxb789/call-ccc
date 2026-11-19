package ccc

import (
	"strings"

	"go.kenn.io/agentsview/internal/parser"
)

// thinkingRenderingReaders names the readers whose Content carries the reader's own rendering of the
// record's persisted reasoning trace: a `[Thinking]…[/Thinking]` block written from the same text the
// record records as its trace (parser.ParsedMessage.ThinkingText, in the `[Thinking]\n<text>\n
// [/Thinking]` shape internal/parser's block reader and the providers that follow it write). It is the
// one fact that makes a record's Content not the record's own text, and it is what dialogueOf asks: a
// record whose read answered no dialogue projection and whose reader is named here contributes no text
// where the record carries a trace, because the reader's rendering and the record's own words are one
// flattened string with no source-owned answer separating them.
//
// Every entry is the carried reader itself -- internal/parser is the source-side authority, and the
// construction site is named -- and the list names two shapes, because what an entry does differs by
// shape:
//
//   - readers that render a trace and project no dialogue of their own. Their records carry the
//     rendering and nothing answers what the model said, so a record of theirs that carries a trace has
//     no established dialogue at all (a record the read answered no trace for is what the paragraph
//     after this list states: the entry cannot act for it, and its text travels counted): Cline
//     (cline.go:601-620), Codebuff (codebuff_decode.go:696,712),
//     Crush (crush.go:558), DeepSeek TUI (deepseek_tui.go:204), Evener (evener.go:418), Forge
//     (forge.go:250-256), Goose (goose.go:488), legacy Kilo (kilo_legacy.go:914), Kimi and Kimi Work
//     (kimi.go:374,590; the Kimi Work provider parses through the same reader), Muse (muse.go:492),
//     RooCode (roocode.go:488), Tau (tau.go:405), ZCode
//     (zcode.go:560), ChatGPT (chatgpt.go:401-413, reached from buildChatGPTMessages),
//     Claude.ai (claude_ai.go:151-156), Copilot (copilot.go:200-208), Gemini (gemini.go:400-415),
//     Hermes and Augure Desktop (hermes.go:319-321,517-519,1349-1351; the Augure Desktop provider
//     parses through the same reader), OpenCode, Kilo and MiMo Code (opencode.go:1726-1731,
//     opencode_v2.go:280-286; the Kilo and MiMo Code providers parse through the same reader),
//     Reasonix (reasonix.go:89-93) and Zencoder (zencoder.go:371-377).
//   - readers that render a trace *and* project a dialogue. Their projection answers first for every
//     record a source read produces (dialogueOf asks Message.DialogueProjected before Content is
//     consulted), so the entry is inert there; it is what keeps a record *without* a projection that
//     carries a trace -- a legacy archive row, a row the archive wrote before it stored the projection,
//     or a path of the same reader that records Content as the source wrote it -- from publishing the
//     reader's rendering as the model's words: Amp (amp.go:100-120), Claude Code
//     (claude.go:1481-1564,2844-2925), Codex, TraeX, Augure Code and Interpreter
//     (codex.go:658-668; the three variant providers parse through the same reader), Cursor
//     (cursor.go:592-594), Devin (devin.go:583-587,977-1005), Grok
//     (grok.go:1160-1164), iFlow (iflow.go:369-399), OpenClaude (openclaude.go:519-530,557-559),
//     OpenClaw (openclaw.go:131-144,158-164), OpenHands (openhands.go:440-450, its block's
//     construction site), the Pi family -- Pi, OMP, OMO, Step Code, Direct API and Prime Agent
//     (pi.go:509-549; every one of the six labels is served by this one reader) -- and QClaw
//     (qclaw.go:98-117,131-138). Amp, Claude Code, Cursor, iFlow and OpenClaw
//     build Content through content.go's ExtractTextContent, which writes the same block
//     (content.go:50-56).
//
// Two things are deliberately not on the list, and each is the answer that keeps the source's own
// words:
//
//   - a reader that records Content as the source wrote it. internal/parser's Vibe reader copies the
//     record's `content` and `reasoning_content` independently (vibe.go:353-371) and renders neither
//     into the other, so a `[Thinking]…[/Thinking]` block standing in such a record is text the record
//     carries -- a model writing the markers itself -- whatever the trace holds, and it is never
//     treated as a rendering.
//   - a label this build does not know. No rendering of the trace into Content is established for it,
//     so its records are treated as the second kind too: the text travels, and the report says per
//     record what that text is (unprojected_dialogue: the reader's Content rather than a source-owned
//     projection). A rendering left standing there is a duplication the report names, while text
//     dropped as though it were a rendering is bytes the source recorded that no later step can put
//     back.
//
// The entries act only where the read answered the record's trace: dialogueOf asks the reader's name
// and a non-blank Message.Thinking together, because a rendering is written from the trace's own text
// and only a trace the read recorded can show the two apart. A reader that renders the trace into
// `Content` without the read recording the trace's own text -- Message.Thinking empty for every record
// it produces, which is how the eight block-writing readers the list gained for their rendering sites
// (ChatGPT, Claude.ai, Copilot, Gemini, Hermes, OpenCode, Reasonix, Zencoder) behave in the carried
// tree -- is therefore a reader no entry can act for: its records reach the record's-own-bytes state,
// the text travels (the rendered block among it where the record has one) and the conversion counts
// them (unprojected_dialogue). That is the same answer the unknown-label default takes, and for the
// same reason: nothing in the read separates the rendering from the record's own words.
//
// The list is maintained against the carried readers: an upstream update that brings in a reader which
// renders a trace into Content (docs/upstream.md's procedure) has to add it here, or that reader's
// records travel with the rendering carried as dialogue.
var thinkingRenderingReaders = map[parser.AgentType]struct{}{
	// Readers that render a trace and project no dialogue of their own.
	parser.AgentAugureDesktop: {},
	parser.AgentChatGPT:       {},
	parser.AgentClaudeAI:      {},
	parser.AgentCline:         {},
	parser.AgentCodebuff:      {},
	parser.AgentCopilot:       {},
	parser.AgentCrush:         {},
	parser.AgentDeepSeekTUI:   {},
	parser.AgentEvener:        {},
	parser.AgentForge:         {},
	parser.AgentGemini:        {},
	parser.AgentGoose:         {},
	parser.AgentHermes:        {},
	parser.AgentKilo:          {},
	parser.AgentKiloLegacy:    {},
	parser.AgentKimi:          {},
	parser.AgentKimiWork:      {},
	parser.AgentMiMoCode:      {},
	parser.AgentMuse:          {},
	parser.AgentOpenCode:      {},
	parser.AgentReasonix:      {},
	parser.AgentRooCode:       {},
	parser.AgentTau:           {},
	parser.AgentZCode:         {},
	parser.AgentZencoder:      {},
	// Readers that render a trace and project a dialogue: the entry is what covers a record of theirs
	// the read answered no projection for.
	parser.AgentAmp:         {},
	parser.AgentAugureCode:  {},
	parser.AgentClaude:      {},
	parser.AgentCodex:       {},
	parser.AgentCursor:      {},
	parser.AgentDevin:       {},
	parser.AgentDirectAPI:   {},
	parser.AgentGrok:        {},
	parser.AgentIflow:       {},
	parser.AgentInterpreter: {},
	parser.AgentOMO:         {},
	parser.AgentOMP:         {},
	parser.AgentOpenClaude:  {},
	parser.AgentOpenClaw:    {},
	parser.AgentOpenHands:   {},
	parser.AgentPi:          {},
	parser.AgentPrimeAgent:  {},
	parser.AgentQClaw:       {},
	parser.AgentStepCode:    {},
	parser.AgentTraeX:       {},
}

// readerRendersThinkingTrace answers whether the reader that produced a session's records is one that
// builds the record's persisted reasoning trace into Content (thinkingRenderingReaders).
//
// The session's own agent label is the reader's name, and a label that names a stored agent of a
// provider -- Freebuff for Codebuff -- is resolved to that provider first, exactly as the archive's own
// lookups do (storedAgentOwner). A label this build does not know answers false: no rendering of the
// trace into Content is established for it, so its records keep the answer that preserves the source's
// own words (see the list's own paragraph on that default).
func readerRendersThinkingTrace(agent parser.AgentType) bool {
	if owner, ok := storedAgentOwner(agent); ok {
		agent = owner
	}
	_, renders := thinkingRenderingReaders[agent]
	return renders
}

// thinkingRenderOpen and thinkingRenderClose are the readers' own markers around a rendered reasoning
// trace: internal/parser's block reader and the providers that follow it write
// `[Thinking]\n<trace>\n[/Thinking]`, and internal/parser.IsThinkingOnlyContent recognises the same two
// words.
const (
	thinkingRenderOpen  = "[Thinking]"
	thinkingRenderClose = "[/Thinking]"
)

// dialogueState names what establishes the text a record contributes as the model's own words. A
// record's dialogue is exactly one of these states, and the three surfaces that must agree about a
// record agree through it: the interchange document carries the read's own answers per record
// (Message.Dialogue, Message.DialogueProjected, Message.ContentRendersThinking), the target holds the
// per-message shape this record's state produced (a message or part with its text, or none), and the
// conversion's report counts the state's own category (unprojected_dialogue, unresolved_dialogue) or,
// where nothing was left out, no category at all.
type dialogueState uint8

const (
	// dialogueProjected: the read answered this record's dialogue itself
	// (parser.ParsedMessage.DialogueText, carried as Message.DialogueProjected), and Text is that
	// answer -- an explicitly empty projection included, which is the source's own statement that the
	// record has no dialogue to carry. "Explicitly empty" and "unanswered" are different states and are
	// never collapsed: the first is an exact conversion of a record with no dialogue, the second leaves
	// the text to the rules below.
	dialogueProjected dialogueState = iota
	// dialogueFromContent: the read answered no projection, and Content is the record's own bytes. The
	// read answered no trace for the record (Message.Thinking blank), or the reader that produced it is
	// not one whose Content carries a rendering of the record's reasoning trace
	// (thinkingRenderingReaders); and the record carries no call whose rendering a reader may have
	// inlined into Content. A reader's entry in that list does not act for a record whose read answered
	// no trace: the reader writes the rendering into Content from the trace's own text, and with no
	// trace recorded nothing in Content is shown to be one -- the text travels, the rendering among it
	// where the record has one, and the conversion counts it (unprojected_dialogue, below). Text is
	// Content, byte for byte, and the conversion reports the record
	// as carried without a source-owned projection (unprojected_dialogue) rather than as a projection:
	// what travelled is the read's Content, and a caller can see on every run how many records rest on
	// that answer instead of on one the source gave.
	dialogueFromContent
	// dialogueRenderingOnly: the read answered no projection, and Content is byte for byte the reader's
	// own rendering of the record's whole trace (`[Thinking]\n<trace>\n[/Thinking]`, the shape the
	// readers write for a record whose only part is its reasoning). The record has no dialogue: the
	// rendering is the trace the read already carries as the record's reasoning, and writing it as
	// dialogue would state the trace twice, once as words the model never wrote. This is the one shape
	// the rendering is *equal* to, not a reading of bytes that contain it: the equality is exact and
	// the trace is the record's own, so no literal text can be eaten by it (a record whose text stands
	// beside its rendering has more bytes in Content than the block, and a record whose text *is* the
	// block has no trace for the block to be equal to).
	dialogueRenderingOnly
	// dialogueUnestablished: the read answered no projection and Content is not the record's own bytes:
	// the reader renders the record's persisted reasoning trace into Content and the record carries one,
	// or the record carries calls whose renderings a reader may have inlined into it. Which bytes the
	// model wrote cannot be recovered from the record. The reader's rendering is written from the
	// trace's own text, so the same Content and the same trace arise from different native part orders
	// -- a record whose reasoning part follows its text and one whose text part quotes the block it
	// renders produce identical Content and identical trace -- and no exactness is available: the text
	// is not written, the record's tool evidence still is, and the conversion counts the record
	// (unresolved_dialogue) instead of guessing. Text is empty.
	dialogueUnestablished
)

// recordDialogue is one record's dialogue as the read establishes it.
type recordDialogue struct {
	// Text is the text the record contributes as the model's own words: the projection, Content where
	// Content is established as the record's own bytes, and empty where the read established none
	// (dialogueProjected with an empty projection, dialogueRenderingOnly) or could not
	// (dialogueUnestablished).
	Text string
	// State names what established Text.
	State dialogueState
}

// dialogueOf answers the text a record contributes as the model's own words, and what establishes it.
// Every target reads its message text through this one rule, so that the answer is the same on every
// route and a route can tell a converted record from one whose text was not established.
//
// A record's dialogue is only ever one of two things: what the source itself said it was, or the
// record's own bytes. The source's own answer is the projection (parser.ParsedMessage.DialogueText,
// captured while the reader still knows its part types and carried as DialogueProjected); where the
// read made one, it is the whole answer, and Content is not consulted at all -- a projection that is
// explicitly empty says the record has no dialogue to carry, and the reader's Content may render
// whatever it likes beside that answer.
//
// Where the read answered no projection, Content is the only evidence left, and the question is whether
// Content *is* the record's own text or a flattened rendering of something that is not dialogue (a
// persisted reasoning trace, a tool call). A block of `[Thinking]…[/Thinking]` beside text cannot be
// un-mixed by reading it: the readers write the trace's own text verbatim between their markers, so two
// different native part orders -- text followed by reasoning, and text that quotes the block -- produce
// the same Content, the same trace and the same reader label, and no inverse can tell which occurrence
// was the model's words. Three answers are therefore possible, and this function is where they are told
// apart (the conditions are the reader's own recorded behavior in thinkingRenderingReaders and the
// record's own fields):
//
//   - the record's own bytes (dialogueFromContent), where nothing shows a rendering in Content: the
//     reader is not one the list names, or the read answered no trace for the record, and the record
//     carries no calls;
//   - no dialogue at all (dialogueRenderingOnly), where Content is exactly the rendering of the
//     record's whole trace;
//   - nothing established (dialogueUnestablished), where the reader renders the trace into Content and
//     the record carries one, or the record carries calls. No text is published from such a record:
//     publishing the reader's rendering as the model's words is the error this rule exists to prevent,
//     and deleting a rendering by inference is the error three review rounds kept finding in the
//     fallback that used to guess. The honest answer is the partial conversion the report names
//     (unresolved_dialogue counts the records), and the source session keeps the bytes for a later,
//     better-informed route.
//
// A record whose Content is empty is dialogueFromContent with no text: no bytes stand in Content, so
// nothing about them can be got wrong, and no reader this build carries drops a record's own text from
// Content. "Empty" is exact, not blank after trimming: a Content of whitespace alone is bytes the
// record carries, so it takes the paths below as the record's own text -- reaching the writers' own
// whitespace rule (whitespace_only_dialogue, or the route's own category for that shape) -- instead of
// being folded in here, where it would reach them as no text at all and be reported as a record that
// carried nothing (round 17's PD-1: on Codex such an assistant record was dropped with no category
// naming it, and the other three routes reported it as empty).
func dialogueOf(message Message) recordDialogue {
	if strings.TrimSpace(message.Dialogue) != "" || message.DialogueProjected {
		// The projection is the answer. A non-empty Dialogue is one whether or not the read flagged it
		// (a document a caller built by hand records its text there); the flag is what makes an *empty*
		// Dialogue an answer -- the source's own "no dialogue" -- instead of an unanswered question.
		return recordDialogue{Text: message.Dialogue, State: dialogueProjected}
	}
	if message.Content == "" {
		return recordDialogue{State: dialogueFromContent}
	}
	if message.ContentRendersThinking && strings.TrimSpace(message.Thinking) != "" {
		if dialogueIsRenderingOnly(message) {
			return recordDialogue{State: dialogueRenderingOnly}
		}
		return recordDialogue{State: dialogueUnestablished}
	}
	if len(message.ToolCalls) > 0 {
		// The reader may have inlined the calls' own renderings into Content (internal/parser's
		// ParseToolCall.Rendering records the text a reader inlined), and the read answers no per-record
		// question about whether it did: the text is not established, and the calls themselves travel
		// as the tool evidence they are.
		return recordDialogue{State: dialogueUnestablished}
	}
	return recordDialogue{Text: message.Content, State: dialogueFromContent}
}

// dialogueIsRenderingOnly reports whether a record's Content is byte for byte the reader's own
// rendering of the record's whole trace (dialogueRenderingOnly): the opener at Content's first byte,
// the record's own trace between the markers with nothing added, and the closer ending Content. The
// comparison is exact -- no trimming, no tolerance, the raw trace's own bytes -- because the readers
// write the part's text verbatim between their markers, and a shape that differs from that by one byte
// is a shape this rule establishes nothing about.
func dialogueIsRenderingOnly(message Message) bool {
	return message.Content == thinkingRenderOpen+"\n"+message.Thinking+"\n"+thinkingRenderClose
}

// dialogueLoss names the report category a record's dialogue is counted under. A record's dialogue
// either left nothing out -- it was projected, it had no dialogue, or its Content is its own text that
// the conversion wrote -- or it is counted under exactly one of the two categories below, and every
// route asks which through dialogueLossOf, so the four writers cannot count one state under different
// categories.
type dialogueLoss uint8

const (
	// dialogueLossNone: nothing about this record's dialogue was left out, so the report names no
	// dialogue category for it.
	dialogueLossNone dialogueLoss = iota
	// dialogueLossUnprojected: the read answered no projection and Content is the record's own bytes,
	// so the text written is the reader's own Content rather than an answer the source gave
	// (unprojected_dialogue, carried with a caveat).
	dialogueLossUnprojected
	// dialogueLossUnresolved: the read answered no projection and Content is not the record's own bytes
	// (the reader renders the trace into it and the record carries one, or the record carries calls
	// whose renderings a reader may have inlined), so no text is written for the record and the
	// omission is counted (unresolved_dialogue, omitted). The count is not a claim that the record wrote
	// none.
	dialogueLossUnresolved
)

// dialogueLossOf names the report category a record's dialogue is counted under, from the state
// dialogueOf answered. This is the one place that maps a state to a category: all four target writers
// read the answer from here (round 17's finding 3 -- the decision and the rows it feeds used to be
// copied four times, and the copies had already diverged), so a state can be counted under one
// category and no other.
func dialogueLossOf(dialogue recordDialogue) dialogueLoss {
	switch dialogue.State {
	case dialogueUnestablished:
		return dialogueLossUnresolved
	case dialogueFromContent:
		// Text that is only whitespace is not counted here: it is the record's own bytes and it travels
		// to the writer, which counts it under its own whitespace category (the text itself is never
		// written, because the target's own reader reads a blank-after-trimming message as no message).
		if strings.TrimSpace(dialogue.Text) != "" {
			return dialogueLossUnprojected
		}
	}
	return dialogueLossNone
}

// dialogueLossRows builds the two rows the dialogue states are reported under, so that what each state
// is called and what the row says about it are written once rather than copied per route (round 17's
// finding 3): a record whose text was not established (unresolved_dialogue, omitted) and a record whose
// text is the reader's own Content rather than a projection (unprojected_dialogue, carried with a
// caveat).
//
// unestablishedOutcome is the route's own answer for an unestablished record -- what its writer does not
// write, and where the record's reasoning goes -- because that differs by target, and it is passed per
// route rather than generalised: Pi and OpenCode write the trace in their own block or part, while
// Codex and Claude have no plaintext-trace shape and count the trace as omitted too (round 17's PD-3:
// the clause read "the record's own reasoning still travels as reasoning_text" on those two routes,
// whose own reasoning rows say the trace is omitted because the target records reasoning as its
// provider's own encrypted or signed artifact).
func dialogueLossRows(unresolved, unprojected int, unestablishedOutcome string) []Loss {
	return []Loss{
		{
			Category:    "unresolved_dialogue",
			Disposition: DispositionOmitted,
			Count:       unresolved,
			Detail: "the source reader answered no dialogue projection for these records, and their " +
				"Content is not the record's own text: the reader renders the record's persisted " +
				"reasoning trace into Content and the record carries one, or the record carries calls " +
				"whose renderings a reader may have inlined into it. Two different native part orders " +
				"can produce the same Content and the same trace, so which bytes the model wrote " +
				"cannot be recovered from the record; " + unestablishedOutcome +
				". The source session keeps the bytes",
		},
		{
			Category:    "unprojected_dialogue",
			Disposition: DispositionCarriedWithCaveat,
			Count:       unprojected,
			Detail: "the source reader answered no dialogue projection for these records, so the text " +
				"written for them is the reader's own Content for the record, byte for byte, rather " +
				"than an answer the source gave about what the model said. What the read established " +
				"is that nothing in Content is shown to be a rendering of the record's reasoning " +
				"trace: the reader is not one this build's list names as rendering the record's trace " +
				"into Content, or the read recorded no trace for the record -- and a reader that " +
				"renders a trace without recording the trace's own text carries the rendering here, " +
				"because the read answered nothing that could tell it from the record's words. That " +
				"is never an inference from the record's bytes; a label this build does not know is " +
				"treated the same way, and its records are counted here too",
		},
	}
}
