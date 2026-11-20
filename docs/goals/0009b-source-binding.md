# 0009b — Source binding and route isolation

**Status:** Ready 2026-11-21 (owner fork A2: goal 0009 decomposed into four ordered increments — the
native oracle frozen first, so the verifier can never again be co-mutated with the implementation it judges).
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md); the
decomposition's rationale, the two cross-family consults it came from, and the per-increment proof obligations
are in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md).

**Prerequisite:** [0009a](0009a-native-oracle.md).

## Intended advance

The read's **cross-record facts are materialised once**, into a bound source representation, and the
route table plus the CLI gate the selection flags to the reference pair under a **keep-all** pass — behaviour
preserving for every route, so nothing downstream has to reconstruct what the read already knew.

## Bounds

No selection semantics in this increment: the keep-all pass must be indistinguishable from today's
unflagged conversion. No writer change beyond consuming the bound representation.

## Done when

- **D1 — Keep-all is byte-identical.** A keep-all selection produces the same target bytes as the
  unflagged conversion, on every route, proven by a before/after probe (D4 of 0009, inherited).
- **D2 — Binding is lossless and total.** Every occurrence identity, ownership link, lifecycle state and
  active-line fact the supported reads *can* answer is carried; the ones they **cannot** answer are carried as
  *unknown*, never guessed (goal 0009's D6(b) applied to the read side).
- **D3 — The flags are refused on every other pair before the source is opened**, asserted as **zero reader
  calls** (goal 0009's D7 inherited).
- **D4 — Route isolation is provable from the file list:** this increment's changed paths include
  `targets.go`, `route_shared.go`, `cmd/ccc/main.go` and the three non-reference targets, and no later
  increment of goal 0009 may touch them.

## Non-scope

Selection semantics (0009c); any target writer's lowering (0009d).

## Acceptance surface

`docs/evidence/0009b-citations.tsv` — discrete decisions plus the keep-all byte-identity probe.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority or
irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause above.
