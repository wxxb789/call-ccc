# Plan — 0009 Select which recorded context crosses a harness boundary

Contract: [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape references: 0008 ([`docs/plans/0008-opencode-source-continuation.md`](0008-opencode-source-continuation.md))
for a plan whose plan-time facts were measured before anything was written; 0004
([`0004-repeatable-evolving-session-handoffs.md`](0004-repeatable-evolving-session-handoffs.md)) for the
boundary and counted-disclosure rules the selection report joins (unchanged here).

**Narrowed 2026-11-20 (owner fork A).** This plan supersedes the plan the goal-0009 activation increment
landed. It describes **one reference route — Claude Code → Codex**. The superseded wider attempt (PR #14) is
closed; its review rounds are an adversarial-shape corpus, and its two unresolved behaviour findings are this
plan's §4 negative cases.

## 1. What this increment adds, and what it must not

Adds: one opt-in flag on `ccc continue` (`--select <spec>`, with `--strict-selection`), one pure function
between the read and the conversion, **the seam dispositions the target writer and the read-back verifier
consume instead of re-inferring** (D6), the vendor writer/converter changes for the **Claude Code → Codex**
route, one report section, and the tests that pin the goal's `Done when` clauses.

Must not: filter inside a reader (a read answers the source as it is), filter inside a writer (each writer
stays a faithful projection), change any default path, touch the other four target routes, or grow into
compaction, summaries, redaction or reasoning-trace editing (0010–0013).

## 2. Measured: the ordinal space, and what real sources look like

- The read supplies `Message.Ordinal`, and it is the source's own numbering as the read preserved it — not a
  position. Most routes assign a 0-based serial counter (`internal/parser/codex.go:2118`'s
  `NewCodexCollectingSink(0)`; `internal/parser/opencode.go:1373`'s `ordinal := 0`; Pi's own counter,
  `internal/parser/pi.go:146`; and Claude Code, this route's own source, `extractMessagesContext`'s
  ordinal — `internal/parser/claude.go:2790,2818`), but Shelley's assigns the source's own `sequence_id`
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

### 3.3 The carried dispositions (D6 — the fork-A addition)

The read→selection seam's answers for some dispositions exist on the superseded attempt's tree, not on this
increment's base tree: `internal/ccc/conversation.go` on `archive/0009-pr14-pre-forkA` carries
`sourceResultPairs`, `sourceKeptAncestor`, `sourceLineInactive`, `sourceLineHolds` and `sourceFileLine`,
while this tree's `Conversation` is `Source`, `Messages` and `Fidelity` only. This increment **adds** —
drawing on the tag where it fits — the set for the reference route so that the Codex writer and the
read-back verifier consume the seam's decisions rather than re-deriving them from the punctured stream:

1. **call↔result identity**, including ID-less occurrences (`sourceResultPairs`; **new** on this tree);
2. **operation lifecycle finality** — completed / failed / in-progress / interrupted (**new**);
3. **omitted-by-selection vs absent-in-source** (**new**);
4. **compaction ledger** — the calls *and* results a boundary accounts for (**new**);
5. **retained-vs-already-written ancestry** (`sourceKeptAncestor`; **new** on this tree).

ID-less ownership inference on a selected run is **not** performed: an ambiguous shape is refused or
explicitly omitted (goal's non-scope).

## 4. Verification

- **Byte-identical default (D4):** the throw-away before/after probe 0007 and 0008 used — one test file
  converting documents through the five routes, **the flag absent in every run**, run on the tree before and
  after the change; each run prints every written file's digest (over a normalized copy, the minted ids
  differing run to run), every item line and the non-zero report rows, and the two runs' outputs are diffed.
  Flag-present versus flag-absent cannot discharge it.
- **Reproduction (D1):** a fixture with a known ordinal map plus a native load of the **Codex** target (the
  level the route already supports), pairing every selected record with its disposition and its source
  ordinal — the accounting is over **every** selected record, not one native record per ordinal.
- **The seam's negative cases (D6):** the two behaviour findings the superseded PR never closed, as
  mutations: (a) filter a paired progress event out of the plan and assert the published outcome is **not**
  `completed`; (b) replace calls with a compaction boundary and assert the replaced calls are **counted**.
  Each must be red without the carried disposition and green with it.
- **Refusals (D3, D5):** each of §3.1's three refusals is a named case with a mutation that disables its
  branch and turns the case red, and the space-based ones run over both a 0-based fixture and the
  Shelley-shaped one (§7), so a positional reading of the space fails a case rather than passing one.
- **Native load is a separate, two-tier claim** (the fork-A consult's finding): the offline gate asserts the
  per-route wire schema and the structural invariants against committed fixtures; the real loader
  (`scripts/codex-native-check.py`) asserts the marker/kept-record expectations against the installed
  artifact. A mirror-image local reader is never the proof of a writer's contract.

## 5. CLI and machine-readable output

`--select` and `--strict-selection` appear in `continueUsage`; the report gains a selection section (human
lines) and the same fields in `--json`: `ordinal_space` (`min`, `max`, `records`, `contiguous`), then
`included`, `excluded`, `necessities_added` and `calls_undischarged`, each with counts and the regions it
names (ordinal values as the read supplied them), plus the per-selected-record disposition table of D1. Exit
codes unchanged.

## 6. Evidence plan

`docs/evidence/0009-selective-context.md` (the record, sections as its siblings have them), the counts it
maintains, the mutation table for every refusal **and for the two D6 negative cases**, and the D4 probe's
verbatim output; `docs/evidence/0009-citations.tsv` at close-out, scoped to discrete decisions, with the
Codex native-load check named in the manifest header as deliberately not covered rather than carried as a
row (the citations gate verifies Go tests only).

## 7. Fixture plan

One fixture with a deliberately awkward shape a Claude read can produce: a call whose result sits in a
later message than the selection boundary, a call with no recorded result, a system record and a compact
boundary inside the selected region — the boundary being a text summary with no calls or results
(`extractCompactSummary`, `internal/parser/claude.go:3419`, written as a system record with
`IsCompactBoundary`). It is generated by the repository's own generator and pinned by the existing
`TestFixturesMatchTheirGenerator` discipline.

The two D6 negative cases need shapes a Claude read cannot produce — a nonterminal tool-result status
(Claude's `parseToolResult` sets only a terminal or `errored` status, `internal/parser/content.go:117`) and
a compaction record that accounts for the calls and results it replaced (a Claude compact boundary is a
text summary with neither) — so they run on document-level constructions in the selection's own test, as
the superseded PR built them (`TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` and the
`compaction_record_tool_results` case on `archive/0009-pr14-pre-forkA`).

- A case whose ordinals do not start at 0 — Shelley's measured shape (§2): a `Conversation` whose
  `Message.Ordinal` values are 1..5 as its `sequence_id` recorded them, built in the selection's own test
  (the seam takes the read document, so no inherited reader is edited, forked or re-pinned). It pins that
  `1-5` selects exactly those five records and that `0` and `0-` are refused naming the space.

## 8. Documentation and bookkeeping

`README.md`, `AGENTS.md` (one binding bullet for the selection rule and one for the seam law), the goal file,
`docs/decisions/OPEN-QUESTIONS.md` (the fork-A decision and its consult), and the goal-0008 archive move —
already paid by the goal-0009 activation increment (0008 reads *Accepted: merged onto `main` as `9f17535`;
archived*).

## 9. Defaults taken where only the owner could decide, and out of scope

Owner-approved defaults: selection applies before the target is chosen (so it composes with every route);
`--verify resume` runs over the selected history and the verify report says the history was selected;
regions match the ordinal values the read supplied (§2), never the target's post-write numbering.

Out of scope: the other four target routes (their own later increments; owner decision 2026-11-20),
compaction densities (0010), summaries (0011), redaction (0012), reasoning-trace edits (0013), ID-less
ownership inference, content- or tool-name-based selection, interactive pickers, turn-level shorthand.

## 10. Stage order and gate

`implement → gate → simplify → apply → gate → review → fix → gate → commit → push → PR → CI → machine
review → merge`, i.e. the series' own order; `python3 scripts/gate.py` must end in `GATE: PASS` before each
commit, and the citations manifest must exist before an acceptance audit or the merge. The conductor's
**contractive breaker** (2026-11-20) blocks the run if the reviewed diff grows more than 25% in a fix round,
if findings do not strictly decrease across three rounds, or past six ordinary rounds — an
abstraction-boundary failure to report, not a budget to raise.

## 11. Where the implementation departed from this plan, and why

None yet. This plan was rewritten for fork A before any implementation, and each departure is recorded here
rather than in a rewritten section (0007 §6.1's habit).
