# 0019 — Fan the selection seam out to Oh My Pi (OMP)

**Status:** Ready 2026-11-20.
**Prerequisite:** [0009](0009-selective-context.md).

## Intended advance

The seam contract holds for the **OMP** target. OMP shares a format family with Pi but has its own resume
semantics, which must not be collapsed into Pi's: the carried dispositions are consumed by OMP's own writer
and verifier, and OMP's requirement that every assistant record carry a non-empty `model` and `provider`
is satisfied by the target's own projection, never by stamping imported values onto synthetic text.

## Bounds

As 0009. No shared helper may be "generalised" by weakening either route's semantics; 0018 and 0019 share a
format family, not a resume contract.

## Done when

- Goal 0009's D1–D5 hold on the OMP route.
- **D6 on the OMP route**, including the two negative cases, over OMP's tree shape and its `model`/`provider`
  requirement.
- `scripts/omp-native-check.py` asserts the real loader's view (`get_messages`).

## Non-scope

The other three routes; ID-less ownership inference.

## Acceptance surface

`docs/evidence/0019-citations.tsv`.

## Stop and report

As 0009.

---
