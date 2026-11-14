# 0010 — Compact selected conversation regions at different densities

**Status:** Provisional — refine before activation.  
**Prerequisite:** [0009 — Select which recorded context crosses a harness boundary](0009-selective-context.md).

## Intended advance

A user can specify that some history deserves **high-density compaction**, while selected important sections retain their original detail or receive only light noise reduction. The target receives useful, user-controlled context without call/ccc deciding globally what matters.

## Material bounds

Compaction is a visible, opt-in context transformation, never an implicit condition for ordinary continuation. It must not claim to recreate hidden model memory, execute historical tools or discover missing external context. Original history remains recoverable unchanged. Record the distinction between quoted/original evidence and generated summary or rewritten text.

## Provisional acceptance basis

- The same source conversation can yield meaningfully different native-resumable targets under at least two user-chosen density policies, with protected passages demonstrably intact.
- Human and coding-agent callers can inspect what was compacted and the material information sacrificed; unconfigured identity continuation remains unaffected.
- Reasonable comparison against the full-history baseline shows that the selected retained decisions and key details are useful for the intended continuation, rather than merely producing shorter text.

## Refinement needed before activation

Settle the minimum user-facing control surface and what supplies the compaction intelligence (including whether an LLM is necessary, what model authority is permitted, and how cost and privacy are exposed). Choose real evidence-rich transcripts and a meaningful quality judgment before committing to precise acceptance. Do not invent token-reduction targets without measurement.
