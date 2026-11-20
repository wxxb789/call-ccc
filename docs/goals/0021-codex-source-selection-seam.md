# 0021 — Fan the selection seam out to the Codex **source** direction

**Status:** Ready 2026-11-20.
**Prerequisite:** [0009](0009-selective-context.md).

## Intended advance

The seam contract holds when the **source** is a Codex rollout: the read-side pairing the seam carries
(`sourceResultPairs`, resolved over the read *before* the reduction) must survive a selection whose window
left a result's ancestry out, and Codex's off-path branch records (`inactive_branch_records`) must be
omitted and counted, never re-parented onto a synthesized boundary node.

## Bounds

As 0009. This is a source-direction increment: it proves the seam on the read side, and no target writer is
rewritten here.

## Done when

- Goal 0009's D1–D5 hold with a Codex rollout as the source.
- **D6 for the Codex source read**: a result whose recorded ancestry the window left out is paired as the
  read paired it, and an off-path branch whose fork point fell inside the window is omitted and counted.
- The two negative cases, red without / green with the carried disposition.

## Non-scope

The other three routes; ID-less ownership inference.

## Acceptance surface

`docs/evidence/0021-citations.tsv`.

## Stop and report

As 0009.
