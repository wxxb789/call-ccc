# Plan — 0009 Select which recorded context crosses a harness boundary

Contract: [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0008 ([`docs/plans/0008-opencode-source-continuation.md`](0008-opencode-source-continuation.md))
for a plan whose plan-time facts were measured before anything was written, and 0004
([`0004-repeatable-evolving-session-handoffs.md`](0004-repeatable-evolving-session-handoffs.md)) for the
boundary and counted-disclosure rules the selection report joins (both unchanged here).

Written before implementation, from the refinement proposal the owner approved on 2026-11-19: the
selection model, the seam's arms and the reversible defaults below are the owner's decisions and are not
the implement lane's to re-open. The lane owns this file's final text and its §11 — the place each
departure from this plan is recorded rather than rewritten here (0007 §6.1's habit).

## 1. What this increment adds, and what it must not

Adds: one opt-in flag on `ccc continue` (`--select <spec>`, with `--strict-selection`), one pure function
between the read and the conversion, one report section, and the tests that pin the goal's `Done when`
clauses. Must not: filter inside a reader (a read answers the source as it is), filter inside a writer
(each writer stays a faithful projection), change any default path, or grow into compaction, summaries,
redaction or reasoning-trace editing (0010–0013).

## 2. Measured: the ordinal space, and what real sources look like

- The read supplies `Message.Ordinal`, and it is the source's own numbering as the read preserved it — not a
  position. Most routes assign a 0-based serial counter (`internal/parser/codex.go:2118`'s
  `NewCodexCollectingSink(0)`; `internal/parser/opencode.go:1373`'s `ordinal := 0`; Pi's own counter,
  `internal/parser/pi.go:146`), but Shelley's assigns the source's own `sequence_id`
  (`internal/parser/shelley.go:612` — `Ordinal: int(r.sequenceID)` — and its reader test expects ordinals
  1..5 for five records, `internal/parser/shelley_test.go:210-212`). Both read paths carry the value
  through unchanged (`internal/ccc/reader.go:782`, `internal/ccc/archive.go:580`), so on a Shelley-shaped
  read `0` is an ordinal that names no record and ordinal 5 stands where the last zero-based position is 4.
  Nothing here renumbers: the selection and the report act on the ordinal values a route supplied, and the
  report states the space it saw (§5) so a caller writes a spec against the numbers that exist rather than
  the positions it assumes. Ordinal values are still the only selector every route has — a Codex rollout
  records no per-message identity while a Pi/omp session records `id`/`parentId`
  (`internal/ccc/conversation.go`).
- Real sessions on this host, read through `bin/ccc inspect --json` (counts only; no message content was
  read or stored). Codex is a **census** — every `rollout-*.jsonl` under `~/.codex/sessions`
  (`~/.codex/sessions/**/rollout-*.jsonl`, 25 files) — messages p50 2 / max 253, calls 467 vs results 445
  (**4 sessions with a call the source never discharged**, 22 calls without a recorded result). OpenCode is
  the store's **five sessions with the newest `time_updated`** (`~/.local/share/opencode/opencode.db`,
  `session_v2`) — messages p50 2 / max 2 / sum 8, no calls. Pi and omp are each a **sample of 8 sessions
  inspected during the refinement**: its "8 recent" selection was not recorded, so these are that sample's
  counts and not the output of a rule a later run repeats — pi messages p50 52 / max 172, calls 352 =
  results 352; omp p50 116 / max 365, calls 645 = results 645. Long sessions are real, and an undischarged
  call is a state the formats carry.

## 3. The interface and the seam (the owner's decision)

### 3.1 The selection specification

`--select "0-5,9,12-"` — the numbers are `Message.Ordinal` values (§2), comma-separated: a bare ordinal
selects one record, `N-M` is a closed range inclusive of both ends, and `N-` selects ordinal `N` and every
higher ordinal present. Gaps are legal — an ordinal a source never used simply selects nothing — so one
spec may name a region that exists on one route and not another. Parsing and refusal live in the core
(`internal/ccc`), so a caller embedding the core and a caller driving the CLI get the same sentence for the
same mistake (`cmd/ccc/main.go` already forwards the core's own wording for `--verify`; the same pattern is
followed).

Refusals (D5) are defined against the **ordinal space the read produced** — the minimum, the maximum and
the records present, the same space §5 reports — and each is its own named case:

1. **Malformed syntax** — a non-numeric token, an inverted range (`5-3`), or a region that selects an
   ordinal an earlier region already selected (`1-3,3`; the goal's *duplicated region*) → refused.
2. **Outside the ordinal space** — a literal number below the minimum or above the maximum → refused,
   naming the space it saw. An open `N-` carries no upper number, so its own `N` is what is checked: `N-` is
   fine whenever `N` is within the space and selects from `N` through the highest ordinal present, while
   `0-` on Shelley's 1..5 space is refused naming the space rather than read as "from the beginning".
3. **Selects nothing** — a well-formed, in-space spec that matches no record at all (every ordinal it names
   falls in a gap) → refused, naming the same space, rather than installing a session with no history.

A bad selection is refused, not repaired (D5): every refusal carries the core's own sentence and the usage
exit code, before any install.

`internal/ccc/selection.go`:
`Select(conv Conversation, spec string, strict bool) (Conversation, SelectionReport, error)` — pure, no I/O,
no clock, no globals, so its whole matrix is unit-testable.

### 3.2 The seam (the owner's decision)

- Default: a selection that separates a recorded call from its recorded result **carries the counterpart**
  and lists it as a necessity (`necessities_added`), with the ordinal that pulled it in.
- `--strict-selection`: the same selection is **refused by name** before anything is installed.
- A call the source itself left undischarged stays as it is and is reported (`calls_undischarged`).
- Nothing is renumbered: the selection matches the ordinal values the read supplied (§2;
  `internal/ccc/reader.go:782` and `internal/ccc/archive.go:580` carry them through in both read paths);
  the target writer's own numbering is the writer's business and is disclosed as a transformation, not
  presented as the source's.

## 4. Verification

- **Byte-identical default (D4):** the throw-away before/after probe 0007 and 0008 used — one test file
  converting documents through the five routes, **the flag absent in every run**, run on the tree before and
  after the change; each run prints every written file's digest (over a normalized copy, the minted ids
  differing run to run), every item line and the non-zero report rows, and the two runs' outputs are diffed.
  Flag-present versus flag-absent cannot discharge it; that comparison is the selection's own claim — the
  written records a reduced run has (D1) and the report section naming it (D2) — not the untouched default.
- **Reproduction (D1):** a fixture with a known ordinal map plus a native load of the target (the level the
  route already supports), pairing each written record with its source ordinal.
- **Refusals (D3, D5):** each of §3.1's three refusals is a named case with a mutation that disables its
  branch and turns the case red, and the space-based ones run over both a 0-based fixture and the
  Shelley-shaped one (§7), so a positional reading of the space fails a case rather than passing one.

## 5. CLI and machine-readable output

`--select` and `--strict-selection` appear in `continueUsage`; the report gains a selection section (human
lines) and the same fields in `--json`: `ordinal_space` (`min`, `max`, `records`, `contiguous`) — the space
a spec is written against and the space a refusal names, so a nonzero start or a gap is visible rather than
silent — then `included`, `excluded`, `necessities_added` and `calls_undischarged`, each with counts and the
regions it names (ordinal values as the read supplied them). Exit codes unchanged.

## 6. Evidence plan

`docs/evidence/0009-selective-context.md` (the record, sections as its siblings have them), the counts it
maintains, the mutation table for every refusal, and the D4 probe's verbatim output (the probe is the
throw-away before/after file: added, run, quoted, removed — its output is the evidence, and the citations
manifest's header names D4 instead of holding a row);
`docs/evidence/0009-citations.tsv` at close-out.

## 7. Fixture plan

One fixture with a deliberately awkward shape: a call whose result sits in a later message than the
selection boundary, a call with no recorded result, a system record and a compact boundary inside the
selected region. It is generated by the repository's own generator and pinned by the existing
`TestFixturesMatchTheirGenerator` discipline.

- A case whose ordinals do not start at 0 — Shelley's measured shape (§2): a `Conversation` whose
  `Message.Ordinal` values are 1..5 as its `sequence_id` recorded them, built in the selection's own test
  (the seam takes the read document, so no inherited reader is edited, forked or re-pinned). It pins that
  `1-5` selects exactly those five records and that `0` and `0-` are refused naming the space.

## 8. Documentation and bookkeeping

`README.md`, `AGENTS.md` (one binding bullet for the selection rule), `docs/conventions.md` (the spec's
grammar), `docs/harness.md` (nothing changes in the gate beyond the citations step it already requires),
the goal file (refined text from the approved proposal), and the goal 0008 archive move — paid by the
goal-0009 activation increment that landed this plan (0008 now reads *Accepted: merged onto `main` as
`9f17535`; archived* in `docs/goals/archive/`).

## 9. Defaults taken where only the owner could decide, and out of scope

Owner-approved defaults (not the lane's to change): selection applies before the target is chosen (so it
composes with every route); `--verify resume` runs over the selected history and the verify report says the
history was selected; regions match the ordinal values the read supplied (§2), never the target's
post-write numbering.

Out of scope: compaction densities (0010), summaries (0011), redaction (0012), reasoning-trace edits
(0013), content- or tool-name-based selection, interactive pickers, turn-level shorthand.

## 10. Stage order and gate

`implement → gate → simplify → apply → gate → review → fix → gate → commit → push → PR → CI → machine
review → merge`, i.e. the series' own order; `python3 scripts/gate.py` must end in `GATE: PASS` before each
commit, and the citations manifest must exist before an acceptance audit or the merge (the conductor
enforces both since 2026-11-19).

## 11. Where the implementation departed from this plan, and why

None yet: this plan was written before the implementation, and each departure is recorded here rather than
in a rewritten section (0007 §6.1's habit).
