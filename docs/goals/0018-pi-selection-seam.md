# 0018 — Fan the selection seam out to Pi

**Status:** Ready 2026-11-20 (owner decision: one route per increment).
**Prerequisite:** [0009 — Select which recorded context crosses a harness boundary](0009-selective-context.md).

## Intended advance

The seam contract proven on goal 0009's reference route (Claude Code → Codex) holds for the **Pi** target: a
selection's cross-record facts are carried once across the read→selection seam, and Pi's writer, its
read-back verifier and its native check consume them instead of re-inferring them from the reduced stream.

## Bounds

Identical to 0009: a pure selection over the recorded document, source untouched, default path byte-identical,
no new flags, no other route touched. Pi's own tree shape (`id`/`parentId`) is the route-specific surface:
the seam's retained-ancestry disposition must be what the Pi writer chains each entry under.

## Done when

- Goal 0009's D1–D5 hold **on the Pi route**.
- **D6 on the Pi route** — no Pi writer and no read-back verifier infers a cross-record fact from the reduced
  stream; the two negative cases the superseded PR never closed are red without the carried disposition and
  green with it: an omitted progress event must not be published as a *completed* outcome, and a call a
  compaction record replaced must not vanish uncounted — including across Pi's tree re-parenting.
- `scripts/pi-native-check.py` asserts the marker/kept-record expectations against the **real** loader
  (`get_messages`), not against a mirror reader.

## Non-scope

The other three routes (0019–0021); ID-less ownership inference on a selected run.

## Acceptance surface

`docs/evidence/0018-citations.tsv` — one row per discrete decision plus the Pi native-load check.

## Stop and report

As 0009. If the reviewed diff stops contracting, report it as an abstraction-boundary failure.

---
