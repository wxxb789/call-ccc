# 0009d — Codex lowering and native acceptance

**Status:** Ready 2026-11-21 (owner fork A2: goal 0009 decomposed into four ordered increments — the
native oracle frozen first, so the verifier can never again be co-mutated with the implementation it judges).
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md); the
decomposition's rationale, the two cross-family consults it came from, and the per-increment proof obligations
are in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md).

**Prerequisite:** [0009c](0009c-selection-closure.md).

## Intended advance

`codex_target.go` lowers each closed unit **locally** — a stateless homomorphism from closed units
to Codex-native records — and Codex itself opens the intended artifact with the expected selected history.

## Bounds

Zero edits to the selection/closure rules and **zero edits to the frozen oracle** (0009a). The
unflagged baseline stays a separate regression obligation; the flags are exposed only at this increment.

## Done when

- **D1 — Every required occurrence has a truthful native representation, or the conversion refuses
  before installation** — no third outcome.
- **D2 — Offline read-back proves the artifact matches the planned dispositions** (the plan being the carried
  triple), and the **frozen** oracle validates the installed artifact.
- **D3 — Codex's own loader shows the expected history** for the selected fixture, and the excluded record's
  marker is absent — the two-tier native claim, with the selected arm re-runnable.
- **D4 — The pair gate and the report behaviour are proved end to end**, and the report's per-record
  disposition table accounts for **every** selected record.

## Non-scope

The other three target routes; any edit to `scripts/codex-native-check.py`.

## Acceptance surface

`docs/evidence/0009d-citations.tsv` — discrete decisions plus the Codex native-load check.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority or
irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause above.
