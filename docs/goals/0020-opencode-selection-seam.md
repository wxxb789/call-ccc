# 0020 — Fan the selection seam out to OpenCode

**Status:** Ready 2026-11-20.
**Prerequisite:** [0009](0009-selective-context.md).

## Intended advance

The seam contract holds for **OpenCode**, the linear database-backed target. Its route-specific constraints
are the point of this increment: OpenCode silently drops an assistant record without `time.completed`;
its store's bytes move with the harness's WAL handling, so source immutability must be measured on
`session export` **records**, not on the database file's bytes; and its single outcome slot per call makes
the reserved-slot / writability rules observable.

## Bounds

As 0009. Source-kind-aware immutability (records, not bytes) and a preventative isolation of any subprocess
(`XDG_DATA_HOME` / `OPENCODE_DB`, `--standalone`) are part of the increment, not an optimisation.

## Done when

- Goal 0009's D1–D5 hold on the OpenCode route.
- **D6 on the OpenCode route**, including the two negative cases, over the slot/writability rules and
  `time.completed`.
- Source immutability is asserted on `session export` records for this source kind.
- `scripts/opencode-native-check.py` asserts `session export --standalone`'s view.

## Non-scope

The other three routes; ID-less ownership inference.

## Acceptance surface

`docs/evidence/0020-citations.tsv`.

## Stop and report

As 0009.

---
