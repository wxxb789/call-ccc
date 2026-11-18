package ccc

import "go.kenn.io/agentsview/internal/parser"

// thinkingRenderingReaders names the readers whose records carry the record's own persisted reasoning
// trace in Content as a `[Thinking]…[/Thinking]` block, so that a block whose words the trace holds is
// the reader's rendering of the trace rather than dialogue (dialogueFromContent), and what stands
// beside the block is the record's own text.
//
// Every entry is the carried reader itself -- internal/parser is the source-side authority, and the
// construction site is named -- and each of them writes the block from the same text it records as the
// record's trace, with no dialogue projection of its own:
//
//   - Cline (internal/parser/cline.go:601,617-620), RooCode (roocode.go:488) and legacy Kilo
//     (kilo_legacy.go:914) render a reasoning record as a block, and a record carrying text beside it
//     as the block joined to that text;
//   - Codebuff (codebuff_decode.go:696,712), Crush (crush.go:558), DeepSeek TUI (deepseek_tui.go:204),
//     Evener (evener.go:418), Forge (forge.go:252), Goose (goose.go:488), Kimi (kimi.go:374,590), Muse
//     (muse.go:492), Tau (tau.go:405) and ZCode (zcode.go:560) render each thinking part where it
//     stands in the record's own part order.
//
// Two bounds hold this list to what it claims. A reader that renders *and* projects a dialogue never
// reaches the removal at all -- dialogueText answers with the projection, an explicitly empty one
// included, before Content is consulted -- so the readers that project (Codex's reasoning rows, Pi's
// and OMP's thinking blocks, the shared block reader behind Claude's and Amp's transcripts) are
// deliberately not listed and do not need to be. A reader that records Content as the source wrote it
// is not a renderer either: internal/parser's Vibe reader copies the record's `content` and
// `reasoning_content` independently (vibe.go:353-371) and renders neither into the other, so a
// `[Thinking]…[/Thinking]` block standing in such a record is text the record carries -- a model
// writing the markers itself -- whatever the record's trace holds, and it is never removed.
//
// A reader this build does not know is treated as the second kind, which is the answer that keeps the
// source's own words: a rendering left standing in the dialogue is a duplication the report can show,
// while dialogue deleted as though it were a rendering is text the source recorded that no later step
// can put back. The list is maintained against the carried readers: an upstream update that brings in
// a reader which renders a trace into Content (docs/upstream.md's procedure) has to add it here, or
// that reader's records travel with the rendering carried as dialogue.
var thinkingRenderingReaders = map[parser.AgentType]struct{}{
	parser.AgentCline:       {},
	parser.AgentCodebuff:    {},
	parser.AgentCrush:       {},
	parser.AgentDeepSeekTUI: {},
	parser.AgentEvener:      {},
	parser.AgentForge:       {},
	parser.AgentGoose:       {},
	parser.AgentKiloLegacy:  {},
	parser.AgentKimi:        {},
	parser.AgentMuse:        {},
	parser.AgentRooCode:     {},
	parser.AgentTau:         {},
	parser.AgentZCode:       {},
}

// readerRendersThinkingTrace answers whether the reader that produced a session's records is one that
// builds the record's persisted reasoning trace into Content (thinkingRenderingReaders).
//
// The session's own agent label is the reader's name, and a label that names a stored agent of a
// provider -- Freebuff for Codebuff -- is resolved to that provider first, exactly as the archive's own
// lookups do (storedAgentOwner). A label this build does not know answers false: no provenance is
// established for it, so no removal is authorised by it.
func readerRendersThinkingTrace(agent parser.AgentType) bool {
	if owner, ok := storedAgentOwner(agent); ok {
		agent = owner
	}
	_, renders := thinkingRenderingReaders[agent]
	return renders
}
