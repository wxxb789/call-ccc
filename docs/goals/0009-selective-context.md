# 0009 — Select which recorded context crosses a harness boundary

**Status:** Refined 2026-11-19 (owner approved the refinement the same day; prerequisite accepted); ready to activate  
**Prerequisite:** [0003 — Continue a Codex session in Claude Code](archive/0003-codex-to-claude-continuation.md).

## Intended advance

A caller can name **which recorded regions of a conversation** cross into the new native session, instead
of always carrying the whole selected snapshot. Selection is a transparent, opt-in transformation of the
document that is read: it never infers project state, never reads Git, never follows linked sessions and
never calls a model. The unconfigured command keeps today's behaviour byte for byte.

## Bounds

Selection is a pure function of the recorded document: messages, their own order, their recorded tool
calls and results, and nothing else. Source history and existing destination history stay unchanged. The
original order and provenance remain inspectable in the report. Where a selection would separate a
recorded call from its recorded result, the counterpart is carried and named as a structural necessity —
a selection never turns a recorded structure into a false one, and never claims to recreate hidden model
memory.

## Done when

- **D1 — The selected set is what is written.** For a fixture with a known shape, a selection reproduces
  exactly the selected ordinals (plus any disclosed necessity) as the target's records; the target is
  natively loadable, and the evidence pairs each written record with the ordinal it came from.
- **D2 — The report names the transformation.** Included regions, excluded regions, necessities added and
  calls the source itself left undischarged are each reported per class, with counts, in both the human
  report and `--json`; the counts agree with the conversion's own `losses` vocabulary.
- **D3 — The seam has a defined behaviour, and it is provable in both directions.** A selection that
  splits a recorded call/result pair carries the counterpart by default and discloses it; the same
  selection with `--strict-selection` is refused by name before anything is installed.
- **D4 — The default path is untouched.** A run without a selection flag is byte-identical to the same run
  before the change, proven by a before/after probe over the existing fixtures.
- **D5 — A bad selection is refused, not repaired.** An ill-formed spec (non-numeric, inverted range,
  out-of-range ordinal, duplicated region) is refused with the core's own sentence and the usage exit code,
  before any install.

## Acceptance surface

`docs/evidence/0009-citations.tsv` — one row per clause a gate-runnable Go test discharges, authored at
close-out (the repository's own rule, and what the conductor now requires before an audit or a merge). D4's
frozen-byte probe is a Python case the required `selftest` step runs and is named in the record rather than
given a row (the carve-out 0007 and 0008 both state).

## Stop and report

If a source archive read cannot establish the region a caller named — an ordinal that exists only in the
archive projection, a call whose result the source recorded without one — report the gap; do not invent a
recovered region. Stop on missing user-only authority or irreversible changes; otherwise use reversible
defaults and end with evidence paired to every completion condition.
