# 0009a — Freeze the native oracle

**Status:** Ready 2026-11-21 (owner fork A2: goal 0009 decomposed into four ordered increments — the
native oracle frozen first, so the verifier can never again be co-mutated with the implementation it judges).
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

- **D1 — The checker declares its complete rule registry, and the registry is executed, not asserted.**
  Every well-formedness rule the oracle enforces is exactly one id in `ORACLE_RULES` (the checker's own
  registry), and `python3 scripts/codex-native-check.py --selftest-mutations` proves, for **every**
  declared rule id, a positive fixture that passes, a negative fixture that fails citing that rule
  alone, and a `--disable-rule <RULE_ID>` flip that turns the same negative fixture green again — a
  closed set equality between the declared rule ids and the tested ones, over synthetic Codex JSONL
  fixtures committed for the purpose. A declared id with no fixture pair, a fixture pair naming no
  declared id, or any of the three arms disagreeing fails the command.
- **D2 — The oracle needs no knowledge of the source or the selection.** Its input is the **artifact** (plus,
  at most, an explicit manifest receipt); it never re-derives Claude Code structure or selection semantics
  from a transcript. A reduced artifact is validated with the source absent from the process. The
  **reader-visible projection** — the records an inherited *read* of the artifact produces — is the
  **manifest receipt's own statement** (its producer is 0009c/0009d, this goal's Non-scope), never
  something this oracle infers from the artifact's bytes: the oracle judges the artifact's own structure
  and the receipt's own declarations, and a shape whose receipt-facing identity the artifact alone
  cannot state is refused where a plan is compared rather than modelled.
- **D3 — The existing acceptance path is unchanged.** The check on the unflagged Codex acceptance still
  produces the same verdict, and this increment's report says so with the verbatim output.
- **D4 — The freeze is stated where a later lane reads it**, and the reason is mechanical, not moral: a round
  that moves this checker together with production code blocks the series (oracle lock).

## Non-scope

Any Go change; the other four target routes; and the manifest receipt's *producer* (0009c/0009d),
including the **reader-visible projection** it must carry — which records a read of the artifact
returns (the dropped, rewritten, deduped, suppressed and expanded shapes among them) is that receipt's
own statement, and this oracle makes no inference about it from the artifact's bytes.

## Acceptance surface

`docs/evidence/0009a-citations.tsv` — discrete decisions plus the Codex native-load check.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority or
irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause above.
