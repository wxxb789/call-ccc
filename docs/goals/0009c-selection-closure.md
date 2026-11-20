# 0009c — Selection and closure, pure and offline

**Status:** Ready 2026-11-21 (owner fork A2: goal 0009 decomposed into four ordered increments — the
native oracle frozen first, so the verifier can never again be co-mutated with the implementation it judges).
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md); the
decomposition's rationale, the two cross-family consults it came from, and the per-increment proof obligations
are in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md).

**Prerequisite:** [0009b](0009b-source-binding.md).

## Intended advance

The selector, its refusal surface, and **closure by construction**: selection resolves every
obligation **before any writer sees the stream**, so a post-selection stream cannot contain an orphan
call/output half, an empty container or an illegal adjacency. The closed unit's obligation names **both
occurrences** — *this call* and *this result* — because "this record produced history" cannot prove "this pair
survived".

## Bounds

Zero target writers, zero native checks. The selector stays a pure function: no I/O, no clock, no
globals, no planner.

## Done when

- **D1 — Ordinal selection and the three refusals** (malformed, outside the ordinal space, selects
  nothing) hold exactly as the umbrella contract states, each with a mutation that turns it red.
- **D2 — Closure by construction:** the carried decision is a **typed per-occurrence triple** (what the read
  established / what the selection requires / what the target plans to emit or refuse), and the closure
  obligation names both occurrences.
- **D3 — Closure is proved by property, not by example:** generative checks over **small event graphs** assert
  no orphan call or output, no empty container, no illegal adjacency, and exact conservation of source item
  dispositions — a class of claims the citation ledger cannot carry (goal 0010's Q10 boundary).
- **D4 — `strict` is a second decision, not a repair:** the same valid selection is refused by name before
  anything is installed.

## Non-scope

Every target writer; both native checks; the CLI surface beyond the refusals.

## Acceptance surface

`docs/evidence/0009c-citations.tsv` — discrete decisions plus the property-check run.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority or
irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause above.
