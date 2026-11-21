# 0009a — Freeze the native oracle

**Status:** **Held 2026-11-21 — rescope pending** (owner fork A2: goal 0009 decomposed into four ordered increments — the
native oracle frozen first, so the verifier can never again be co-mutated with the implementation it judges).
This file's offline-oracle premise is superseded: the Codex well-formedness judgment belongs to the `codex`
binary itself, not to a standalone specification this repository freezes and mutates. **The hold is
operator-enforced** — the dispatcher checks a goal's prerequisites, not the index's status column, and 0003 is
already accepted, so this status line plus the operator's stand-down are what keep the increment from being
dispatched. The acceptance clauses below are **unchanged** and take effect again when the re-scope lands.
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md); the
decomposition's rationale, the two cross-family consults it came from, and the per-increment proof obligations
are in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md).

**Prerequisite:** [0003 — Continue a Codex session in Claude Code](archive/0003-codex-to-claude-continuation.md).

## Intended advance

`scripts/codex-native-check.py`'s Codex well-formedness rules become a **standalone, frozen
specification**: it validates Codex session artifacts — including reduced/selective ones — against
hand-constructed positive and negative fixtures, with **no Go changes at all** in this increment. Once merged,
the file is **read-only for the rest of goal 0009**; the conductor's oracle-lock signal enforces that.

## Bounds

No Go change; nothing outside `scripts/codex-native-check.py`, its tests and synthetic Codex
fixtures; the existing unflagged Codex acceptance keeps reporting the same verdict.

## Done when

- **D1 — Every oracle rule has its own negative fixture.** Each well-formedness rule is stated over
  **synthetic Codex JSONL fixtures**, with a mutation that removes the rule and turns its fixture red.
- **D2 — The oracle needs no knowledge of the source or the selection.** Its input is the **artifact** (plus,
  at most, an explicit manifest receipt); it never re-derives Claude Code structure or selection semantics
  from a transcript. A reduced artifact is validated with the source absent from the process.
- **D3 — The existing acceptance path is unchanged.** The check on the unflagged Codex acceptance still
  produces the same verdict, and this increment's report says so with the verbatim output.
- **D4 — The freeze is stated where a later lane reads it**, and the reason is mechanical, not moral: a round
  that moves this checker together with production code blocks the series (oracle lock).

## Non-scope

Any Go change; the manifest receipt's *producer* (0009c/0009d); the other four target routes.

## Acceptance surface

`docs/evidence/0009a-citations.tsv` — discrete decisions plus the Codex native-load check.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority or
irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause above.
