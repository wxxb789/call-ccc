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

**Revised 2026-11-20 (review r1).** The pair gate (§3.4), the pre-cut line disposition (§3.3 item 6) and the
D6/D3/pair-gate verification arms (§4) answer the first machine review of this revision; the claim that the
flag "composes with every route" was wrong for this increment and is replaced by §3.4.

## 1. What this increment adds, and what it must not

Adds: one opt-in flag on `ccc continue` (`--select <spec>`, with `--strict-selection`), accepted on the
**reference pair alone** (a Claude Code source and a Codex target, §3.4), one pure function between the read
and the conversion, **the seam dispositions the target writer and the read-back verifier consume instead of
re-inferring** (D6), the vendor writer/converter changes for the **Claude Code → Codex** route, one report
section, and the tests that pin the goal's `Done when` clauses.

Must not: filter inside a reader (a read answers the source as it is), filter inside a writer (each writer
stays a faithful projection), change any default path, touch the other four target routes, accept the
selection flag on any pair but `claude:… --to codex` (an unsupported pair is refused by name before the
source is opened or anything is installed — §3.4), or grow into compaction, summaries, redaction or
reasoning-trace editing (0010–0013).

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
spec may name a region that exists in one read's ordinal space and not another's. Parsing and refusal
live in the core (`internal/ccc`), so a caller embedding the core and a caller driving the CLI get the
same sentence for the same mistake (`cmd/ccc/main.go` already forwards the core's own wording for
`--verify`; the same pattern is followed).

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
5. **retained-vs-already-written ancestry** (`sourceKeptAncestor`; **new** on this tree) — the nearest
   ancestor the read holds that the selection also carries, per retained record: what a tree writer chains
   to when the record's own parent was cut;
6. **the read's own line disposition, answered before the cut** (`sourceLineInactive`/`sourceLineHolds`;
   **new** on this tree) — for every retained record, whether the walk from the read's *last* record
   reached it, carried from the read as it was **before** the selection reduced the stream. It is separate
   from 5 because the two answer different questions: ancestry says what a writer may chain to, the line
   disposition says whether the record belongs on the resumed conversation at all — and a selection that
   cuts the record the read's line ended at must not move the line. The exact-parent sibling shape
   `internal/ccc/codex_target_test.go` already pins (a result and the final reply are siblings; the result
   is off the read's line) is the failure mode: without the carried answer, a walk over the reduced stream
   starts at the retained result and makes it the new leaf. The Codex writer's line rule
   (`writeConversation`) and the conversion's read-back round-trip check consume this answer instead of
   re-walking the reduced stream; `nil` means the document is the read itself, and the walk is taken
   exactly as before (D4).

ID-less ownership inference on a selected run is **not** performed: an ambiguous shape is refused or
explicitly omitted (goal's non-scope).

### 3.4 Which pairs accept selection

The flag is accepted on the **reference pair alone** — a Claude Code source and a Codex target. The pair
is known from the invocation itself (the reference's own provider label and `--to`), so the decision costs
no read: any other pair with `--select` or `--strict-selection` is **refused by name before the source is
opened and before anything is installed**, with the core's own sentence and the usage exit code (§3.1's
pattern for a bad spec). The refusal lives in the core's `Continue` entry, which sees both words before
anything runs, so a caller embedding the core and one driving the CLI get the same sentence; the CLI maps
it to the usage exit code exactly as it does for `--verify`.

Both alternatives would be wrong, which is why the refusal exists: **ignoring the flag** installs history
the caller excluded, and **passing the reduced history on** to a writer that does not consume the seam
would bypass D6 on a route this increment has not carried it to. The pair gate refuses the *flag*, not the
route: the same spec on the reference pair converts, and a pair with no route at all keeps its existing
`ErrUnsupportedTarget` refusal.

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
  mutations whose selected records are **fixed** — both runs walk the same reduced document and only the
  carried disposition is removed (its seam fields set back to nil), asserting the wire output and the loss
  counts on both sides. A check that only looks for the omitted record's own text would pass without the
  seam, so neither arm may be stated that way.
  - (a) A call whose archive-derived `ResultContent` the selection retains while the paired **progress**
    record it was derived from is omitted: with the carried lifecycle answer the summary is not published
    as the call's outcome (no `function_call_output` for the call; the call counted outstanding), and
    without it the writer's own summary fallback (`derivedResultSummary`, `internal/ccc/codex_target.go`)
    writes the summary as completion — red.
  - (b) A Codex rollout whose selection retains a **compaction boundary record that accounts for a call**
    and the result it replaced: the writer publishes the boundary alone and counts the accounted call and
    result by the report's own category (`compaction_record_tool_results` on the superseded tree); without
    the carried boundary ledger they vanish uncounted — red. The superseded corpus's cited boundary case
    carries a result and no call, so it cannot detect an uncounted Codex call; this arm's shape is the
    Codex one, the call on the boundary record.
- **The pre-cut line (D6/(f)):** the exact-parent sibling shape `internal/ccc/codex_target_test.go` pins —
  a result and the final reply are siblings, the result off the read's line — with a selection that keeps
  the result and cuts the reply: the reduced stream's last record is the inactive result, and the written
  rollout still leaves it off the line and counts it (`inactive_branch_records`), never activating it. The
  mutation clears the carried pre-cut answer on the same records; the walk starts at the reduced stream's
  own last record, activates it, and the arm turns red.
- **Refusals (D5):** each of §3.1's three refusals is a named case with a mutation that disables its
  branch and turns the case red, and the space-based ones run over both a 0-based fixture and the
  Shelley-shaped one (§7), so a positional reading of the space fails a case rather than passing one.
- **The split pair in both modes (D3):** the same valid selection that separates a recorded call from its
  recorded result runs twice over one fixture — by default the counterpart result is **written** and the
  report names it as a necessity (`necessities_added`, with the ordinal that pulled it in); under
  `--strict-selection` the same invocation is **refused by name** before anything is installed (the named
  target home holds no session), the refusal naming the call and both ordinals. §3.1's three refusals
  cannot stand in for this case: none of them reaches §3.2's counterpart rule. Each direction gets its own
  mutation — a flag that does nothing turns the strict arm red by installing, a dropped necessity turns the
  default arm red by writing a call without its recorded counterpart.
- **The pair gate (§3.4):** one named case over at least two unsupported pairs — a supported target with a
  non-Claude source, and the Claude source with another supported target — asserts the same refused-by-name
  sentence, the usage exit code and that nothing was installed; the mutation removes the gate and the arms
  turn red by installing instead of refusing. The same selection on the reference pair converts, so the
  case cannot pass by refusing the flag everywhere.
- **Native load is a separate, two-tier claim** (the fork-A consult's finding): the offline gate asserts the
  per-route wire schema and the structural invariants against committed fixtures; the real loader
  (`scripts/codex-native-check.py`) asserts the marker/kept-record expectations against the installed
  artifact. A mirror-image local reader is never the proof of a writer's contract.

## 5. CLI and machine-readable output

`--select` and `--strict-selection` appear in `continueUsage` beside the pair that accepts them (§3.4);
the report gains a selection section (human lines) and the same fields in `--json`: `ordinal_space`
(`min`, `max`, `records`, `contiguous`), then
`included`, `excluded`, `necessities_added` and `calls_undischarged`, each with counts and the regions it
names (ordinal values as the read supplied them), plus the per-selected-record disposition table of D1. Exit
codes unchanged.

## 6. Evidence plan

`docs/evidence/0009-selective-context.md` (the record, sections as its siblings have them), the counts it
maintains, the mutation table for every refusal, for the pair gate **and for the three D6 negative cases**,
and the D4 probe's verbatim output; `docs/evidence/0009-citations.tsv` at close-out, scoped to discrete
decisions, with the Codex native-load check named in the manifest header as deliberately not covered
rather than carried as a row (the citations gate verifies Go tests only).

## 7. Fixture plan

One fixture with a deliberately awkward shape a Claude read can produce: a call whose result sits in a
later message than the selection boundary, a call with no recorded result, a system record and a compact
boundary inside the selected region — the boundary being a text summary with no calls or results
(`extractCompactSummary`, `internal/parser/claude.go:3419`, written as a system record with
`IsCompactBoundary`). It is generated by the repository's own generator and pinned by the existing
`TestFixturesMatchTheirGenerator` discipline.

The D6 negative cases need shapes a Claude read cannot produce — a nonterminal tool-result status
(Claude's `parseToolResult` sets only a terminal or `errored` status, `internal/parser/content.go:117`) and
a compaction record that accounts for the calls and results it replaced (a Claude compact boundary is a
text summary with neither) — so they run on document-level constructions in the selection's own test, and
the shapes are the superseded corpus's (`TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` and
`TestSelectionPlansNoEvidenceFromACompactionRecord` on `archive/0009-pr14-pre-forkA`), rebuilt as §4's
fixed-records mutations: the archived progress case selects `0-`, so it exercises a retained progress
event and cannot carry the mutation arm itself, and the archived compaction case's boundary record carries
a result and no call.

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

Owner-approved defaults: selection applies before the target is chosen — the pure function runs on the
document, which is what lets a later increment extend it to another route — while the flag itself is
accepted on the reference pair alone, every other pair being refused by name before anything runs (§3.4);
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
