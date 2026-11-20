# Plan — 0009c — Selection and closure, pure and offline

Contract: [`docs/goals/0009c-selection-closure.md`](../goals/0009c-selection-closure.md).
Rationale and the full decomposition: [`docs/plans/0009-decomposition.md`](0009-decomposition.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).

Written before implementation. The umbrella goal file [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md)
still carries the increment's `Done when` clauses D1–D7 and the seam's carried-disposition set (D6); this
increment proves the subset named in its own contract, and nothing here re-opens a settled decision (the
reference route, the refusal surface, the ID-less deferral).

## Order and dependency

This is increment **c of 4** of the goal-0009 decomposition. It is dispatched only after its
prerequisite is accepted; the four are strictly ordered because each one removes a class of defect from every
diff that follows it.

## What this increment must not touch

Every target writer; both native checks; the CLI surface beyond the refusals.

## Evidence

`docs/evidence/0009c-citations.tsv` — discrete decisions plus the property-check run. The manifest carries **discrete decisions plus this increment's native-load check** — not a
∀-shaped claim proved by example (owner Q1 = C, 2026-11-20).

## Stage order and the convergence rule

`implement → gate → simplify → apply → gate → review → fix → gate → commit → push → PR → CI → machine review
→ merge`. The conductor blocks on a convergence signal (executable-surface churn, a rising P0/P1 count
conjoined with executable churn, or the oracle lock — a round that moves a gate/checker script alongside
production code) and on the absolute review budget; a trip is an **abstraction-boundary failure to report**,
never a budget to raise.

## Where the implementation departed from this plan

None yet.
