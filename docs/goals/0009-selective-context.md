# 0009 — Select which recorded context crosses a harness boundary

**Status:** Refined 2026-11-20 (owner fork A). The increment is **narrowed to one reference route — Claude
Code → Codex**. A wider attempt (PR #14: 19 machine-review rounds, 0 clean, reviewed diff 191 KB → 1.47 MB)
was closed unmerged; two blind cross-family consults found it non-contractive, and its review log is kept as
an adversarial-shape corpus for this increment's negative cases. Prerequisite accepted.
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

**The seam is the contract.** A selection deletes the positional context a downstream writer needs to answer
three questions that may legitimately differ: what the **source** established about an invocation, what the
**selection retained**, and what the **target will publish**. So the read→selection seam must carry those
answers **once**, and no target writer and no read-back verifier may re-infer a cross-record fact from the
already-reduced stream — the reduced stream cannot distinguish "absent in source" from "omitted by
selection". The dispositions the seam carries are named in D6.

## Non-scope (deliberate, not deferred by accident)

- **The other four target routes** (Pi, OMP, OpenCode, Codex-as-source). Each is its own later increment that
  must prove the same seam end to end before the next one starts (owner decision 2026-11-20).
- **ID-less ownership inference on a selected run is deferred.** An ambiguous shape is refused or explicitly
  omitted, never inferred — uniformity across five targets is not a goal of this increment.

## Done when

- **D1 — Every selected record has an attributable disposition.** For a fixture with a known shape, the
  target carries exactly the selected ordinals (plus any disclosed necessity), and the report accounts for
  **every selected record**: it became native history, was refused, or was lost — each with a reason — with
  the written record paired to the ordinal it came from. Exact accounting does not require one native record
  per selected ordinal.
- **D2 — The report names the transformation.** Included regions, excluded regions, necessities added and
  calls the source itself left undischarged are each reported per class, with counts, in both the human
  report and `--json`; the counts agree with the conversion's own `losses` vocabulary.
- **D3 — The seam has a defined behaviour, and it is provable in both directions.** A selection that
  splits a recorded call/result pair carries the counterpart by default and discloses it; the same
  selection with `--strict-selection` is refused by name before anything is installed.
- **D4 — The default path is untouched.** A run without a selection flag is byte-identical to the same run
  before the change, proven by a before/after probe over the existing fixtures. The frozen unflagged
  behaviour is **preserved**, not promoted into a requirement of the new path.
- **D5 — A bad selection is refused, not repaired.** An ill-formed spec (non-numeric, inverted range,
  out-of-range ordinal, duplicated region) is refused with the core's own sentence and the usage exit code,
  before any install.
- **D6 — No writer, and no read-back verifier, infers a cross-record fact from the reduced stream.** The seam
  carries, once, the dispositions a writer needs: (a) call↔result identity, including ID-less; (b) operation
  lifecycle finality — completed / failed / in-progress / interrupted; (c) omitted-by-selection vs
  absent-in-source; (d) the calls **and** results a compaction boundary accounts for; (e) retained-vs-already-
  written ancestry; (f) the record's own membership on the source's active line, answered before the cut — a
  selection that removes the record the source's line ended at must not move the line. Negative cases from
  the superseded PR must be **red without** the carried disposition and **green with** it, the selected
  records held fixed: an omitted progress event must not be published as a *completed* outcome, a call a
  compaction record replaced must not vanish uncounted, and a window ending on a record the source's line
  left off must not activate it.
- **D7 — The flag is a capability of one pair, and the others say so.** `--select` and `--strict-selection`
  are accepted on the reference route alone; every other source/target pair is refused by name before the
  source is opened and before anything is installed — never silently ignored (which would install history
  the caller excluded) and never handed a stream a writer on that route would read without the seam.

## Acceptance surface

`docs/evidence/0009-citations.tsv` — one row per **discrete decision** this increment makes; the
per-harness **native-load check** for the reference route is a separate machine half, named in the manifest
header as deliberately not covered rather than carried as a row (the citations gate verifies Go tests only).
The ledger does not attempt to prove universal soundness by example: a row is one gate-runnable Go test that
discharges a named decision, and a `∀`-shaped claim is discharged by a machine check, not by an LLM reading a
citation table (the fork-A consult's finding).
D4's evidence is the throw-away before/after probe 0007 and 0008 used — a test file added, run, quoted in the
record, then removed — so the record's verbatim output is the evidence and the manifest header names D4 with
that reason instead of carrying a row (a row is one gate-runnable Go test, so evidence a gate step does not
run cannot hold one).

## Stop and report

If a source archive read cannot establish the region a caller named — an ordinal that exists only in the
archive projection, a call whose result the source recorded without one — report the gap; do not invent a
recovered region. Stop on missing user-only authority or irreversible changes; otherwise use reversible
defaults and end with evidence paired to every completion condition. If the increment's reviewed diff stops
contracting (the conductor's contractive breaker trips), stop and report it as an abstraction-boundary
failure rather than spending more rounds.
