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

Each departure is recorded here rather than in a rewritten section (0007 §6.1's habit), with the
measurement that decided it, followed by the review round's own:

1. **`CheckSelectionOptions` separates the half of the grammar that needs no read.** The plan put every
   refusal in `Select`. The syntax half (`parseSelectionSpec`) needs nothing but the string, so it is
   exposed as its own checker beside `CheckVerifyOptions`, called by the CLI before the read and by
   `Continue` — which is what gives a caller embedding the core and one driving the CLI *the same
   sentence* for the same mistake (§3.1's own requirement) and lets a malformed spec fail before any
   session is opened. The space-based refusals stay in `Select`, because only a read can answer them, and
   the CLI maps their sentinel to the usage exit code.

2. **The strict refusal's exit code is the usage code (2), and it is a default taken here.** D3 asks for a
   refusal by name before anything is installed; D5 fixes the usage code for the ill-formed arms and says
   nothing about the strict one. Both are a command line that cannot be honoured as written, refused
   before any file exists, so both exit 2 and the help page's exit-code paragraph says so. It is
   reversible in one branch of `cmd/ccc/main.go` and is recorded in `docs/decisions/OPEN-QUESTIONS.md`.

3. **The seam pairs by recorded id only.** §3.2 said "a recorded call and its recorded result"; the only
   link a *document* carries is a non-empty `ToolUseID`, so the closure is over those ids, in both
   directions, to a fixed point (a counterpart can itself hold another call's result). A call or a result
   the source recorded with **no** id carries no recorded link at all, and the seam neither invents one nor
   carries candidates for it: each writer's own rule for an id-less result (the sole outstanding call it
   wrote) reads the selected line, and that rule is *already* disclosed — a written result whose id was
   inferred is counted (`generated_call_ids`, "the id it is written under is the pairing with the one call
   that was outstanding"). Deciding this in the seam instead would mean re-implementing five writers' own
   inferences in a target-agnostic function. **Superseded by §11.11**: the second review round changed the
   closure to pairing by the read's own order (a repeated `ToolUseID` does not expand a selection by id
   alone), and the same round separated "answered" from "progress" in the undischarged count.

4. **The D4 probe drives the pipeline as well as the writers.** The plan's §4 asked for "one test file
   converting documents through the five routes"; the insertion point the change actually adds is
   `continueWith`, so the probe prints both: the writers' own arm (35 route × document conversions, each
   with its normalized bytes, item lines and non-zero loss rows) *and* a pipeline arm per route × document
   (`continueWith`, dry run, the selection flag absent, seven normalized report lines each), which is what
   would catch a defect that filtered a document without the flag. The pipeline arm skips the OpenCode
   target: that route's installation *is* the harness's own importer, which a dry run still runs, so its
   unflagged path is covered by the standalone runs in the record's own sections. **Superseded by
   §11.11**: the second review round's probe covers the OpenCode target's arm too (its dry run installs
   into a scratch store and the arm records the harness's own answer or refusal), and walks every
   committed fixture rather than seven.

5. **The awkward-shape fixture is a fourth Claude transcript, and one of its records is not the `system`
   subtype.** The plan asked for "a system record"; the first draft used Claude's `subtype: hook_result`
   line, and the read of that draft **dropped it entirely** (measured: ten records, nine messages) — the
   Claude reader skips the provider's own bookkeeping lines, so a selection could not have carried it and
   nothing would have been pinned. The fixture therefore carries the shape the read *does* hold as a
   system record: a tool result written on a record whose whole text block is a `<system-reminder>`, which
   the inherited parser promotes to `IsSystem` with its `ToolResults` populated (the tooling fixture's own
   shape, reused). That record is also the second split pair's half, so one record pins both the system
   record and the second necessity arm.

6. **The report spells its classes as regions, not as a list of ordinals** (§5's "regions it names"): a
   contiguous run is `a-b`, a lone value `n`, ascending and each value once, using the specification's own
   grammar, so a report of a large selection stays bounded and a reader can paste a region back into
   `--select`. The space is reported as min/max/records/contiguous, and the *refusals* spell the read's
   actual values (with their gaps), which is what a caller needs to write the next spec.

7. **The report's own two warnings.** The plan asked for the transformation to be named in the human block
   and the JSON; the run also carries it on the report's warning channel (the selection and its counts, and
   a second warning when a carried call has no recorded result), so a caller that reads only the exit code
   or the human block's head cannot take the conversion for the whole read.

8. **The CLI case for "selects nothing" names an arm the fixture cannot produce.** Every ordinal of the
   fixture's space is present, so a well-formed spec that selects nothing is not expressible against it;
   the arm is pinned by the Go case over a gappy read, and the CLI case's own table pins the refusals the
   read can reach (syntax, duplicate, out-of-space, both strict arms) with "nothing was written" as the
   shared assertion.

9. **`scripts/evidence`'s claim reader and this record.** The run this increment records is a **load**-level
   conversion (no turn was run), so its record quotes no resumed turn's answer and §5's cells avoid the
   word "answer" where they quote inline code; the record is deliberately not added to
   `test_evidence_claims.py`'s `ROUTE_DOCUMENTS`, which requires §5 to credit a resumed run's answer.

10. **The review round's fix makes the line a property of the read.** §3.2 and §9 decide what a selection
    *carries*; they left open what the writers' inactive-branch and leaf answers are when a window ends on a
    record the read's own walk places off its line, and the first implementation derived them from the
    reduced document — so a selection could revive an abandoned branch and resume from it with no refusal,
    no count and no report field (the review's R-01, demonstrated on the Pi history fixture). The fix
    answers the read's walk before reducing and carries it per kept record
    (`Conversation.sourceLineInactive`); the tree routes' leaf guard refuses by the read's answer
    (`ErrUnrepresentableLeaf`) and the line routes omit and count the off-line records
    (`inactive_branch_records`), `TestSelectionEndingOffTheReadsLine` pins it, and the default for "may a
    selection choose the resumed line" is recorded in Q-0009-04. The same round corrected the refusal
    criterion's wording in `AGENTS.md`, `docs/conventions.md`, `README.md` and `cmd/ccc/main.go` (a region
    reaching outside the ordinals the read supplied — not "an ordinal the read does not hold": a region
    spanning a gap carries the records it holds), corrected the false "before the target harness is asked
    anything" (its executable's version is probed earlier, before the read), strengthened the B2 case to
    assert the system record's own disposition rather than naming it in the row, and dropped the CLI case
    that had claimed — without asserting — the "selects nothing" arm (§11.8's own reason).

11. **The second review round's six findings.** The review of `9242e49..fc05ac4` found two P1, three P2
    and one P3 item; this round fixes all six, and the changes are departures the plan's sections did not
    state:

    * the **nearest retained ancestor** answers a reduced tree's ancestry: §3.2 said the seam carries a
      call/result structure, and §10's line rule (§11.10) covered the *leaf*, but neither said what a
      record hangs off when the window left its own parent out. A sparse window could therefore chain the
      leaf to the entry written before it and make an off-line branch record an ancestor of the resumed
      conversation (the review's R-06, demonstrated on the Pi history fixture: `--select 0,3,5`). `Select`
      now walks the read's own relations through the excluded records and carries, per kept record, the
      nearest ancestor the window also carries (`Conversation.sourceKeptAncestor`); the tree writers chain
      the entry there (`parentPlace`, `keptAncestorEntry`); and a window where that ancestry is gone —
      the fallback would land under an off-line record — is refused by name (`ErrUnrepresentableLeaf`,
      gated to reduced documents so the unflagged path is unchanged, as §4's probe shows). `Q-0009-04`
      records the default and `TestSelectionKeepsASparseWindowOnTheReadsLine` pins both halves by walking
      the written `parentId` chain.
    * the **supplied-blank refusal**: §3.1 said the CLI's contract is "the value names a selection, or
      there is no selection", and the first implementation read `--select ""` as the flag's absence. The
      review's R-07: an automation's unset variable must not publish the whole history. The CLI now reads
      `flag.Visit`, `ContinueOptions.SelectionSupplied` carries the answer into the core, and a supplied
      blank is refused with `CheckSelectionOptions`' own sentence and exit 2, before the read; `Q-0009-03`
      records the default.
    * the **order-based pairing** and the **progress-versus-outcome** count: §11.3's id-only closure is
      superseded (see above). A result now pairs with the nearest prior call of the same id
      (`selectionPairs`, the reading `internal/parser/termination.go`'s `hasOrphanedToolCall` takes), so a
      reused id's later call carries nothing but itself; and `undischargedCalls` discharges a call only
      where a result that states an outcome follows it (`resultStatesAnOutcome`, shared with the OpenCode
      writer), so a `running` notification is not read as an answer. The undischarged warning no longer
      claims every target records the call: it names the writer's own answer, and the OpenCode route's
      `unpaired_tool_calls` is what the CLI case asserts beside a nonzero count on both report surfaces.
    * the **D4 probe's coverage** is superseded (see §11.4 above): every committed fixture × five routes,
      writer *and* pipeline arms, the OpenCode target included, with the exact probe revision retained as
      `docs/evidence/0009-d4-probe.txt`.
    * the **README example** now names `--select 0-3,7` (in range for its eight-record fixture) and the
      paragraph beside it states the order pairing, the progress rule, the supplied-blank refusal and the
      ancestry rule.

12. **The third review round's six findings.** The review of `9242e49..7e61075` found two P1, three P2 and
    one P3 item; this round fixes all six. The departures the plan's sections did not state are four:

    * the **ancestry walk takes the writers' projection step** (review R-12): §11.11 answered a reduced
      tree's ancestry from the read's *raw* relations, so on a Pi-family source a window naming a call and
      the record the reader answers as hanging off it chained that record to the *call* and left the result
      the selection had carried as a necessity in the file and off the walk the target's loader makes.
      `selectionKeptAncestors` now follows the place the writers resolve (`sourceToolResultBefore`, the same
      tool-result step `sourceLineParent` takes), so the carried result is the ancestor the record hangs
      off. `TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall` pins it on the committed Pi fixture;
      `S6-projection` cites it.
    * the **ancestry answer and the revival check travel by the record's own position** (review R-13): the
      answer was keyed by the record's identity, so a record the read answered a `parentUuid` for but no
      `uuid` (the inherited Claude reader preserves the two independently) lost it, and both the fallback's
      revival check and its naming skipped the empty identity — a retained leaf could therefore hang off
      the abandoned branch's entry with no refusal. `Conversation.sourceKeptAncestor` now holds the reduced
      document's index of the nearest retained ancestor, the tree stamps each record's position as it walks
      and keeps its entry and omitted place under it (`sourceTree.record`, `entryByRecord`,
      `omittedEntryByRecord`), and the refusal names a record without an identity by its ordinal.
      `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor` pins both halves; `S6-identity`
      cites it.
    * the **undischarged count is the pairing's own walk in the writers' own call unit** (review R-14): the
      count read every call row as a call and consumed outcomes one per row, so a projected invocation
      (one `apply_patch` the Codex reader expands into one row per file) was reported undischarged while the
      writer wrote it as one answered call, and two calls recorded before two outcomes let the second
      outcome discharge the earlier call the seam pairs with nothing. `selectionPairing` answers the
      closure's pairs and the outcome-answered ids in one walk, and the count uses the writers' own
      `distinctCalls` unit. `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` covers
      both shapes; `S2-units` cites it.
    * the **selection is decided before the source-side leaf guard, and the D4 probe's measurement is
      strengthened** (reviews R-16, R-15, R-17): `continueWith` now asks `Select` before `piSourceLeafCheck`
      (the guard still runs against the read, so a spec refusal — a command-line answer, usage exit code —
      is not reported as a statement about the source file's leaf; `Q-0009-05` records the default and its
      reversal; `TestContinueValidatesASelectionBeforeTheSourceLeafGuard`, `D5-order`); the retained probe
      canonicalizes identities per value instead of folding them to a class-wide placeholder and its
      pipeline arm installs under an absent target binary and fingerprints the whole folded report and the
      bytes the installation published (supersedes §11.11's probe again; `docs/evidence/0009-d4-probe.txt`
      is the exact revision); and the D2 cases now assert the excluded class's own regions (the CLI's
      `["0-1","4-10"]` and the human line, and the core case's partition check), so deleting the class's
      values fails them.


    The fourth review round (`10f8096`; two P1, three P2, three P3 — one P2 and one P3 on the evidence
    itself) leaves seven departures of its own:

    * the **writers write the seam's pairing, not an id match** (review R-19, a P1): a flat target's
      document is one line, a call the read's line leaves off it is not written there at all, and both flat
      writers accepted a result by looking its id up in the calls they had written — so in a window where
      the seam pairs a result with an *inactive* call reusing the same id, the Codex rollout wrote the
      result as another output of an older call with that id, and OpenCode filed it as a second outcome for
      that call. `pairedCallOccurrences` answers the seam's pairing at occurrence granularity (the unit
      `distinctCalls` writes), `Select`'s closure and `selectionPairing` read the same walk, and each flat
      writer asks whether the paired *occurrence* was written/planned: a result whose paired call cannot
      travel is omitted and counted (`unpaired_tool_results`) instead of rebound by id.
      `TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID` pins both flat writers.
    * the **identity-less parentless fallback goes through the revival guard** (review R-18, a P1):
      `parentPlace` sent a record with neither an identity nor a parent straight to the entry written before
      it, so a window whose previous entry stood for an off-line record chained it there and the leaf guard
      skipped it (an empty identity is not a line answer). The fallback's own check (`fallbackPlace`) now
      runs for that place too, so the tree `3 → 1 → 0` the review traced is refused by name instead of
      reviving the branch; the count stays the read's own (an identity-less record's place invents
      nothing). The extended `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor` pins both
      the refusal and the window that excludes the abandoned sibling.
    * the **inactive ancestor answers for an off-line record** (review R-20): `keptAncestorEntry` rejected
      an *inactive* retained ancestor unconditionally, so a sparse window rewrote an off-line record's
      recorded ancestry to the entry written before it (a record of another branch). An off-line record now
      keeps an off-line ancestor — keeping a branch off the active walk is recorded history the writer
      reproduces — while an *on-line* record is still refused through one, which
      `TestSelectionKeepsAnInactiveBranchesRecordedAncestry` pins on both arms.
    * the **projection the ancestry answer crossed is counted** (review R-21): `selectionKeptAncestors`
      takes the tool-result step the writers take, but the answer did not carry that step, so the writer
      consumed it as uncounted. `keptAncestorAnswer` now names it and `parentPlace` counts it
      (`unresolved_source_parents`), as the writers count the projection they resolve themselves; the
      extended `TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall` asserts the count on the window
      that isolates the projection and beside the unflagged conversion of the same read.
    * the **undischarged count's id-less arm** (review R-22): the count skipped every call the source
      recorded with no id, so an id-less call in a read with no results at all was reported by no surface
      while OpenCode's own report counted it `unpaired_tool_calls`. It is now counted where the carried
      document holds no id-less result at or after it (no outcome in the line can answer it), and left to
      the writers' disclosed inference where one is carried.
      `TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` pins all three shapes.
    * the **duplicate-region refusal is the grammar's own** (review R-23): overlap was checked only after a
      successful read, so `--select 1,1` against a missing session was reported as a source failure instead
      of the usage refusal. `parseSelectionSpec` checks it (`checkSelectionOverlap`), which
      `CheckSelectionOptions` runs before the read.
      `TestCheckSelectionOptionsRefusesASpecificationWithoutARead` and
      `TestContinueRefusesADuplicateRegionWithoutAReadableSource` pin the core and CLI halves.
    * the **D4 probe compares recorded instants** (review R-24, with the probe's own control): the
      normalizer replaced every RFC 3339 instant and every numeric `timestamp`/`created`/`updated` with a
      placeholder, so a same-length change to a source-recorded time folded to the same digest — the probe
      could not establish D4's byte-preservation claim for recorded instants. The retained revision
      normalizes only what a conversion discovers (the checkout root, scratch paths, the source file's
      filesystem mtime) plus the identity canonicalization, and carries `d4ProbeNormalizerControl`, whose
      printed lines show a recorded-instant change moving the digest and a minted-identity change not.
      that revision's own run; the two trees' shared 1024 lines are byte-identical with
      recorded instants compared (the sixth round's revision prints 1109).
    * the **D3 pins** (reviews R-25, R-26): the closure cases exercised isolated pairs and the strict case
      checked neither ordinal, so a one-pass closure and a refusal with swapped ordinals would both have
      passed. `TestSelectionClosesTheCounterpartGraphToAFixedPoint` needs a *backward* two-hop pull (a
      record the window pulled in carries a result of another call), and the strict case asserts each
      sentence half with the ordinal of the side it names.

    The fifth review round (`dfe5a90`; two P1, two P2 — one P1 and one P2 on the seam's pairing, one P1
    on the tree writers, one P2 on the undischarged accounting, one P2 on the D4 probe itself) leaves
    five departures:

    * the **pairing honors the read's own recorded ancestry where it establishes the call** (review 1/4, a
      P1): the seam paired a result with the nearest prior call the id records, so a result whose recorded
      parent named its call was paired with an *inactive sibling* reusing the id later in file order —
      strict refused a pair the source recorded together, a window naming the result pulled in the
      sibling's arguments instead of the call's, and the round-4 occurrence checks then dropped a result
      the base writers retained (a D4 break). `pairedCallOccurrences` now walks the result's recorded
      parent chain (its own record first, the identities the read holds, bounded) and pairs with the call
      the walk names; the nearest-prior-occurrence rule stands only where the ancestry establishes none.
      `TestSelectionPairsAResultByItsRecordedAncestry` pins the window, the strict pair, the sibling's own
      undischarged call and every route's written output, with and without a selection.
    * the **seam's pairing travels with the reduced document** (review 1/4, a P1, second half): a window
      that left out the records the ancestry walk steps through made a writer re-reading the pairing fall
      back to a nearer *carried* occurrence the read never named. `Select` carries its answers in the
      reduced document's positions (`Conversation.sourceResultPairs`), so the writers ask the walk the
      closure asked. `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry` pins it on all
      five routes (without the carried answers the result is dropped and counted on the flat targets and
      the tree route refuses the window outright).
    * the **tree writers validate the paired occurrence against the result's written ancestry** (review
      2/4, a P1): Claude, Pi and OMP accepted a result through a file-wide id map, so a result whose
      paired call stood on another branch was written as that call's outcome while the walk the target's
      loader makes held only the output. The tree now records each written entry's place and the record
      it was written for (`sourceTree.entryPlaces`, `recordPlace`) and answers
      `recordOnWrittenPath`; a result whose paired occurrence is not on its own written ancestry is
      omitted and counted (`unpaired_tool_results`), and the call still travels as recorded history where
      the window carried it. `TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath` pins both
      writer families; `Q-0009-06` records the disposition for both.
    * the **undischarged count reads the archive's own summary and the id-less status** (review 3/4, a
      P2): a call whose only outcome evidence is the summary the archive stores on it
      (`ToolCall.ResultContent`, which the routes write as its completed outcome) was counted
      undischarged and warned about, and an id-less *progress* event suppressed the id-less call's count
      although it states no outcome. `summaryDischarges` mirrors the writers' own guard
      (`derivedResultSummary` beside `callHasRecordedResult`), and the id-less scan asks
      `resultStatesAnOutcome`. `TestSelectionCountsASummaryOnlyCallAsAnswered` and the extended
      `TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` pin both arms.
    * the **D4 probe folds only identities a conversion mints** (review 4/4, a P2): the normalizer folded
      every uuid and every `call_…`/`ses_…`/`msg_…` token by class shape, so a consistent same-length
      rename of a recorded Codex `call_id` preserved the written length and the folded digest — the probe
      could not see it. Each document's recorded identities (`d4ProbeRecordedIDs`) are installed on the
      normalizer and left as recorded, and two controls (`recorded-call-id-change`,
      `recorded-uuid-change`) pin it; that revision's own run is §4's predecessor (1024/1149/125 lines,
      shared diff silent, the recorded-id-removal mutation failing the control), and the sixth round's
      revision moves §4's counts to 1109/1234.

    The sixth review round (`812fec9`; three P1, two P2 — the default path moving with a
    reduction-scoped rule, the id-less inference placing a result off its walk, the Codex writer emitting
    two invocations for one projected call, the result's tool name coming from a reused id's later
    sibling, and a second `Select` reviving an abandoned branch) leaves six departures:

    * the **reduction-scoped disposition is explicit** (review 1/5, a P1): the written-ancestry
      acceptance of the round-5 fix acted on *every* run, so a read whose pairing names a call the
      result's own written ancestry does not reach lost the result where the base writer kept it — a D4
      break the committed fixtures did not cover. `Conversation.selectionReduced()` is the signal the
      writers ask, the default path keeps the id acceptance each writer always had
      (`resultCallWritten`/`pairedCallEntry`), and the D4 probe gains the off-path case: §4's shared diff
      is silent over 1109 lines including it, while the mutation that removes the scoping moves exactly
      that case's 36 lines.
    * the **id-less inference reads the result's written path** (review 2/5, a P1): with the window's
      last outstanding call standing on another branch, the writer inferred the id-less result onto it
      and wrote an output whose invocation the resumed context cannot see (the file-wide outstanding set
      held it because the *tree* writes off-line records as history). `soleResultCall` restricts the
      candidates to the calls that stand on the walk the result's record hangs off
      (`sourceTree.recordOnWrittenPath`), and `TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath`
      pins both arms (no eligible call → omitted and counted; exactly one → the entry states its id).
    * the **selected Codex conversion writes the invocation unit once** (review 3/5, a P1): two
      `function_call` records shared one id for one reported call. `projectedCalls` folds a message's
      rows sharing an id on a reduced document (counting them under `coalesced_tool_call_projections`,
      a category the default path's report does not carry — `Q-0009-08`), and the test asserts the one
      call, the one output and both runs' row counts.
    * the **result's tool name comes from the paired occurrence** (review 4/5, a P2): Pi/OMP's file-wide
      name map answered with the last write for the reused id, so a window's result could state the
      inactive sibling's tool. `writtenCalls` keys the name by occurrence, and the extended
      `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry` gives the sibling a different
      name and asserts the serialized entry's.
    * a **selection over a selection is refused** (review 5/5, a P2): `ErrSelectionReduced` answers
      before any specification is read, `TestSelectRefusesADocumentASelectionProduced` pins the three
      arms, and `Q-0009-07` records the default and its reversal.
    * the **D4 probe compares a case the fixture set lacked** (review 1/5, the second half): the retained
      revision materializes the off-path transcript at run time (`d4ProbeOffPathTranscript`) so both
      trees read identical records for it, and §4's numbers move to 1109/1234 with the 125-line
      increment fixture unchanged.

13. **The seventh review round's three findings.** The review of `9242e49..dea73c0` found two P1 and one
    P2 — the tree writers' id-less inference reading completion file-wide, the tree leaf guard passing an
    identity-less leaf chained off the line, and the D1 citation's membership-only check — and asks for a
    machine-checked pin per rule the touched surface claims. It leaves three departures:

    * **the id-less inference asks completion per occurrence and per written path** (review 1/3, a P1):
      the reduced writers' inference read the file-wide outstanding set, so a recorded outcome the window
      carried on a *sibling* branch cleared a call that was still outstanding on the id-less result's own
      path, and the result was inferred onto a later call although both were unanswered where the result
      stood. Each writer now remembers, per call occurrence, the records whose written results answered it
      (`answeredCalls`) and asks whether any of them stands on the result's own written ancestry
      (`answeredOnWrittenPath`, `sourceTree.recordOnWrittenPath`); the default path keeps the ledger's
      file-wide question (D4). The extended `TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath`
      pins the sibling-outcome arm (both routes outstanding → omitted and counted, no id minted), the
      sibling outcome carried on the trees as recorded history, and the unflagged contrast.
    * **the tree leaf guard answers by the record's own position** (review 2/3, a P1): the identity-keyed
      leaf check returned early on an empty source id, so a selected window whose last record was an
      identity-less record chained off the read's line through its *explicit parent* (the direct ancestor
      lookup, ahead of the guarded places) converted -- the target's loader would have resumed the branch
      the source's conversation left. `Select` now carries the read's own walk membership per kept record
      (`Conversation.sourceLineHolds`), and `lastEntryLineRefusal` walks the entry written last up its
      written ancestry (`sourceTree.entryPlaces`) and refuses by name where the record it resolves to is
      not on the read's line; the identity path stays for a document the read returned.
      `TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine` pins the two windows and the two controls
      (an identity-less leaf on the line, the abandoned branch kept as history once the line's own leaf is
      the window's end), and `TestSelectionKeepsAnInactiveBranchesRecordedAncestry` gains the stranded
      record's arm (an off-line record falling back to an off-line entry as recorded history, not a
      revival) after its mutation showed that half unpinned.
    * **the D1 pin decodes each route's evidence** (review 3/3, a P2): the cited case checked only
      substring membership, so a duplicated tool-result entry left every assertion green. It now decodes
      each of the five routes' documents into their ordered evidence -- dialogue, calls with id, name and
      canonical argument object, results with their paired call id and text, target-owned envelopes
      excluded -- and compares the exact slice, with the raw excluded-material scan kept beside it; two
      executed mutations (a duplicated Pi result entry, a moved Codex output link) fail it.
      `docs/evidence/0009-citations.tsv` gains the `S6-leaf` row and names the round's refinements in its
      extended S2-idless-path and S6-inactive rows.

14. **The eighth review round's four findings.** The review of `9242e49..6bcf91c` found three P1s and one
    P3 -- an identity-less off-line record reviving through a window that retains neither record's
    ancestry, two outstanding occurrences sharing an id collapsing into one eligible call, progress
    closing an outstanding call in the reduced writers, and the missing D3 fixed-point citation -- and
    asks for a machine-checked pin per rule the touched surface claims. It leaves four departures:

    * **a reduced document's line answer is the read's own per-position membership, for identity-less
      records too** (review 1/4, a P1): the flags mark a record off the line only where the read holds an
      identity for it, so the window `2-3` over the identity-less-leaf fixture -- the off-line child and
      the line's own leaf, neither ancestry retained -- converted with the leaf chained under the child on
      the trees and the child written into the line targets' logs. `sourceLineAnswers` and
      `sourceLineFlags` read `Conversation.sourceLineHolds` for a reduced document, the tree's off-line
      registration (`wrote`/`offLineEntry`) and the refusal's second name are keyed by the record's own
      position (`revivedParent`), and the flagless path keeps the flags' long-standing answer (D4). The
      extended `TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine` pins the refusal's two names and
      the line targets' omission with its count.
    * **the reduced id-less inference counts call occurrences** (review 2/4, a P1): Claude's and Pi/OMP's
      `soleResultCall` rejected a second eligible occurrence only when its id differed, and Codex's and
      OpenCode's id-keyed pending sets read two occurrences as one call -- Pi's map iteration order could
      pick either occurrence's tool name, and OpenCode's `calls` map attached the result to the later one.
      Each writer's reduced inference now requires exactly one eligible occurrence (whatever ids they
      carry) and otherwise omits and counts the result; the reused-id arm of
      `TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath` pins all five routes and the
      unflagged contrast.
    * **progress is not an answer in the reduced writers' outstanding set** (review 3/4, a P1): Claude,
      Pi/OMP and Codex cleared a call on a `running` event, so a later id-less result was inferred onto a
      second call although both were still unanswered. The reduced path records a call's completion only
      where the status states an outcome (`resultStatesAnOutcome`), the event itself still travels, and
      the flagless path keeps the ledger's answer (D4). The new
      `TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` pins the omission, the counts and the
      progress event's own travel (OpenCode's plan already refused progress as an outcome and counts the
      event `nonterminal_tool_events`).
    * **the D3 fixed-point closure gains its citation row** (review 4/4, a P3): the cited D3 case exercises
      isolated pairs, which a single forward pass carries correctly; `D3-fixedpoint` now cites
      `TestSelectionClosesTheCounterpartGraphToAFixedPoint`, whose chain requires `2 → 1 → 0`.
      `docs/evidence/0009-citations.tsv` gains the five rows (`S5-identityless`, `S6-identityless`,
      `S2-idless-occurrences`, `S7-writers`, `D3-fixedpoint`) and the round's header block.

15. **The ninth review round's four findings.** The review of `9242e49..1f6e7be` found two P1s and two
    P2s -- a reused recorded id serialized twice so an inferred output read back under the other
    occurrence, an omitted outcome still narrowing the reduced id-less inference, a summary on an id-less
    call inferred off the outstanding set although its owner was known, and the flat targets counting a
    merely excluded parent as an unresolved relation -- and asks for a machine-checked pin per rule the
    touched surface claims. It leaves five departures:

    * **a written call occurrence's emitted id is unique in the document** (review 1/4, a P1): a reduced
      document writes every call occurrence the read holds, and the source may record one id on two of
      them; each target pairs a call with its output by id (Codex's sink keeps the last call registered
      for an id, Claude's reader one tool_use block per id, Pi's reader the block that registered it,
      OpenCode keys the part by it), so an occurrence written under the shared id could not be told from
      the earlier one and the output inferred for the earlier occurrence read back under the later. On a
      reduced document the later occurrence is now written under an id generated from its own place --
      the walk `freshCallID` keeps it clear of every id the document already states, recorded spellings
      included -- and the substitution is counted (`colliding_call_ids`), a reduction-scoped row exactly
      as `coalesced_tool_call_projections` is (the unflagged report keeps its own category list, D4).
      The writers also write each result under the *owning occurrence's* emitted id, never the recorded
      id read back off the result. `TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs`
      pins all five routes (distinct ids, each result under the occurrence it answers, the counts, and
      the unflagged contrast), with a Codex read-back arm asserting the inherited reader's own
      attachment; four executed mutations (one per writer's minting line) fail it.
    * **a result omitted from the document does not answer its call** (review 2/4, a P1): the reduced
      writers recorded a call's completion before the result's text guards, so a result whose text the
      output field cannot carry (empty or whitespace) left the outstanding set and a later id-less result
      was inferred onto a second call although neither had an outcome in the document. The reduced
      ledger's update moved past every omission guard; the flagged path keeps the ledger's long-standing
      answer (D4). A summary the read stored on a call (`ToolCall.ResultContent`) is likewise written for
      the occurrence that stored it, never inferred off the outstanding set (review 3/4, a P2): the
      writers' result helper reads the owning occurrence's emitted id for it (with two eligible calls the
      inference used to drop an output whose owner was already known).
      `TestSelectionAnswersOnlyForTheOutcomesItWrites` pins the omission, the counts and a writable
      control arm on all five routes; `TestSelectionAnswersASummaryFromItsOwningOccurrence` pins the
      summary's own owner (all five routes). Two executed mutations (the ledger restored before the
      guards; the summary case falling back to the inference) fail them.
    * **the round-trip check asserts the owning call of every result** (review 1/4, the second half): the
      comparison identified a result by its call id and text, so a file whose output attached to the
      wrong occurrence of a shared id verified as the intended one. A result item now states the place of
      the call item it was written for (`TargetItem.OwnerCall`, which the reduced writers know from the
      read's pairing or their disclosed inference), the read side derives the owning occurrence the way
      the targets' readers resolve one (the last call the read-back states under the result's id), and
      the comparison annotates every result key with the owning call's evidence (its id, tool and
      arguments) -- a relation the reporter names without carrying any text.
      `TestCodexRoundTripAssertsTheOwningCallOfEachResult` pins the reassignment report and a control
      read-back that attaches each output to the occurrence it was written for; one executed mutation
      (the annotation dropped) fails it. The comparison stays reduction-aware where it must: a writer
      that states no owner has its result read against the reader's own last-under-the-id rule, which is
      what the default path's acceptance has always meant.
    * **the flat targets count the read's own relation answers, not the window's walk** (review 4/4, a
      P2): `sourceLineAnswers` recomputed `unresolved_source_parents` by re-walking the reduced records,
      so a parent the caller merely excluded (`--select 0,5` over the Pi history fixture's chain
      5 → 1 → 0) was reported as a relation the read could not place. The count now reads the selection's
      own ancestry answers (`Conversation.sourceKeptAncestor`, the walk the tree writers take): a
      written record counts once where that answer crossed the tool-result projection, where it reached
      no retained ancestor although it named a parent, or where it named none while not being the line's
      first written record -- and a whole-read selection reports exactly the unflagged conversion's own
      number. `TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer` pins the sparse windows, the leaf
      that retains no ancestor, the 1,4 window's two places (the tree route's own number) and the
      whole-read equality; one executed mutation (the count recomputed over the window) fails it.
    * **OpenCode's id-less inference will not replace a recorded outcome** (review 2/4's shape, this
      route's own answer): the plan's inference read `entry.Result != nil` as answered, so a call whose
      planned result the document cannot state (whitespace text, a status the tool states cannot spell)
      left the eligible set and the id-less result was inferred onto another call. The plan now asks
      `openCodeOutcomeWritable` for the ambiguity (an unwritable planned result leaves its call
      unanswered, so two unanswered occurrences make the result ambiguous) and for the placement: the
      call an inferred result is written *for* must hold no planned result at all, because a part states
      one outcome and the read's own evidence of a different one is not this conversion's to replace. An
      inference the route's part has no room for is omitted and counted (`unpaired_tool_results`). This
      case pins both arms (`TestSelectionAnswersOnlyForTheOutcomesItWrites`); one executed mutation (the
      eligibility restored) fails it for OpenCode.

16. **The tenth review round's four findings.** The review of `9242e49..0d213e5` found two P1s and two
    P2s; all four are fixed, and the round's surface is pinned by nine executed mutations (recorded in the
    evidence record and the stage report).

    * **a writable summary-only outcome is answered before a later id-less result is inferred** (review
      1/4, a P1): OpenCode's plan installs an archive read's `ResultContent` summary after the result walk,
      so the walk saw a call whose only recorded outcome is that summary as unanswered, inferred a later
      id-less result onto it, and the summary pass then skipped the call with its slot occupied -- the
      source's recorded outcome replaced by a result it never wrote beside it. The reduced id-less
      inference now asks `plannedOutcomeWritable`: a writable summary the summary pass will install counts
      as answered (so it neither makes a later result ambiguous nor may take the slot), an unwritable one
      (whitespace text, a status the tool states cannot spell) leaves the call unanswered exactly as
      before. The default path keeps the ledger's id-keyed answer -- the ledger never held the summary --
      so its bytes are untouched (D4).
      `TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall` pins the summary surviving, the
      omission and its counts, the unwritable-summary arm and the unchanged default path; two executed
      mutations (the summary check removed; its writability loosened) fail it, and one (the default path
      made summary-aware) fails its D4 arm.
    * **the structural check compares a Codex file's invocation units, not the reader's per-file rows**
      (review 2/4, a P1): the inherited reader projects one written `apply_patch` `function_call` record
      into one call row per file it names, so a selected (coalesced) rollout read back as two rows against
      one intended call and `verification.structure` came back `not-verified` although the file held
      exactly the invocation written; the omitted test counted written envelopes and never ran the
      read-back. The Codex comparison now answers its call evidence from the installed file's own
      `function_call` records (`codexInstalledCallUnits`/`codexCallEvidence`): one record is one unit and
      its rows are the reader's view of it, the rows of *unstated* ids stay the read-back's own, and a
      file holding fewer rows under an id than records keeps the read-back's rows so an unread record is
      still reported -- two actual call envelopes are two units and are never folded together.
      `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` now verifies the selected
      and unflagged conversions of the committed two-file patch fixture and requires the negative arm (a
      one-invocation document against a file holding two `function_call` envelopes) to be reported; two
      executed mutations (the fold bypassed; every id collapsed to one unit) fail it. The default path's
      *conversion bytes* do not move; its structural verdict for this fixture moves `not-verified` →
      `verified`, which is the finding's own subject and is disclosed in §4's shared-document diff.
    * **the generated-id walk is selection-scoped** (review 3/4, a P2): round 8's `freshCallID` was
      applied to an id-less call unconditionally, so an unflagged conversion whose earlier recorded call
      already held the positional spelling (`call_ccc_2_0`) wrote `call_ccc_2_0_2` where the pre-0009
      writer wrote the duplicate spelling -- a D4 break. The default path writes the plain positional
      spelling again (duplicate included, exactly the bytes the writer always wrote); only a reduced
      document walks the id past the written ones. `TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall`
      pins both halves on Codex, Claude and Pi/OMP (OpenCode's positional id was already walked past
      taken ids before selections existed, so it has no pre-0009 byte to restore); the D4 probe gains the
      colliding-id transcript (`d4ProbeCollisionTranscript`) the committed fixtures do not carry, and the
      pre-fix head moves exactly that document's 32 lines. Two executed mutations (the walk made
      unconditional; the reduced walk removed) fail the test.
    * **a tree-format owner assertion reads the result's own recorded ancestry** (review 4/4, a P2): the
      round-trip owner lookup found the last call under the result's id in file order, so a Pi or Claude
      file whose result entry had been reparented to a sibling verified although the result's own parent
      walk could not reach its invocation. The tree formats (`piRoundTripReader`, the shared Pi/OMP
      comparison; `claudeRoundTripReader`) now resolve a stated owner through the result's recorded parent
      chain -- the nearest ancestor message stating the result's id -- and leave the occurrence ownerless
      where the chain reaches no such call, while the flat Codex rule is unchanged.
      `TestSelectionReportsAResultWhoseAncestryLeavesItsCall` installs a tree conversion, changes only
      the result entry's parent relation to the sibling's entry, and requires the check to report it while
      the unmutated control verifies (Pi, OMP and Claude). Two executed mutations (each reader's ancestry
      flag cleared) fail it.

17. **The eleventh review round's five findings.** The review of `9242e49..88478b6` found two P1s and
    three P2s; all five are fixed, and the round's surface is pinned by fourteen executed mutations
    (recorded in the evidence record and the stage report).

    * **an id-less inference may not take the slot a recorded result is owed** (review 1/5, a P1):
      OpenCode's part holds one outcome per call, and the plan installed whatever result reached the entry
      first, so on a reduced document whose window kept call A and *both* its result records -- an id-less
      one before the one naming A -- the id-less event took A's only slot and the recorded result was
      counted `repeated_tool_results`: an inference displacing a recorded pairing although both halves of
      it travelled. `reserveResultSlots` answers the occurrences whose outcome the document states through
      a result the conversion *writes* (its record on the plan's line and published, its text and status
      spellable by the tool states: `openCodeResultWritable`, which already refuses every progress word),
      and `soleInferablePlannedCall` neither offers such an occurrence nor lets it look answered -- it
      still counts among the unanswered, so an ambiguous window omits the id-less event rather than
      binding it to another call. `TestSelectionReservesARecordedResultsSlotFromAnIdLessInference` pins
      the finding's window, the ambiguity arm and the unwritable-result control;
      `TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite` pins the reservation's own conditions
      (off-line, subagent, compaction-boundary and progress arms). Four executed mutations (the candidate
      test dropped; a reserved occurrence skipped as answered; the writability guard removed; the
      reservation keyed by the result's own place) fail them, and three more (each plan-writes condition
      dropped) fail the conditions case.
    * **the Codex invocation-unit fold is the reduction's own** (review 2/5, a P1): round 9's fold ran on
      every comparison, so the committed two-file `apply_patch` fixture's *unflagged* conversion moved from
      the `not-verified` verdict its revisions before the fold answered to `verified` -- a report, warning
      and CLI-exit-status change on the default path, contrary to D4. `CodexRollout` now states whether the
      document is one a selection produced, and `codexRoundTripReader(reduced)` installs the fold only
      there; the D4 probe's shared-document diff is silent over all 1183 lines again (§4).
      `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` requires the unflagged
      comparison to report the reader's rows (and its verdict to stay `not-verified`) while the selected one
      verifies; one executed mutation (the fold ungated) fails it.
    * **the fold folds only the rows a file's own records account for** (review 4/5, a P2): the units were
      the writer's own `function_call` envelopes alone and the fold replaced *every* row under an id the
      units stated, so a `custom_tool_call` appended under that id -- a call the inherited reader reads --
      produced no difference at all. `codexInstalledCallUnits` now counts every record type the reader
      reads as a call (the writer's envelope keyed; a type whose key this build does not restate as an
      unkeyed unit, which never folds), and `codexFoldableRows` folds an id only where every row's key is
      one a unit states and every unit is keyed. `TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute`
      pins the finding's differing-name case and the same-key case (the file states two records under one
      id and the reader's rows cannot tell them apart); two executed mutations (the attribution rule
      reverted; the unit scan restricted to the writer's envelope) fail it.
    * **a tree owner is resolved through the installed chain** (review 3/5, a P2): the Claude reader merges
      the assistant entries of one message id into the run's last entry, which keeps the parent of the entry
      it merged away, so the owner walk -- bounded by the read-back's own messages -- stopped at that
      absent uuid and reported a correct conversion as missing the invocation its installed chain reaches.
      `roundTripReader.entryParents` answers the installed file's own entry relations (the Claude reader's
      `uuid`/`parentUuid`) and the walk steps through an entry the read-back holds no message for;
      `TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord` pins the finding's shape, and one
      executed mutation (the reader's `entryParents` cleared) fails it; the reparenting case stays the
      negative control (its own mutation arm still reports a broken link).
    * **a selected progress event is omitted, not written as an outcome** (review 5/5, a P2): the writers
      left a `running` call outstanding in their inference but still serialized the event's text as an
      ordinary output, and these formats carry an outcome or nothing, so the artifact claimed an answer the
      source had not recorded. On a reduced document Codex, Claude and Pi/OMP now omit the event and count
      it (`nonterminal_tool_events`, a row scoped to the reduction as `coalesced_tool_call_projections` is,
      because a zero row may not be added to the default path's report); a run without a selection keeps
      writing it (D4). `TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` pins the omission, the
      count on all five routes, the absent row on the default path and the event's continued travel there;
      two executed mutations (the guard removed; its scoping removed) fail it.

18. **The twelfth review round's four findings.** The review of `9242e49..2e2e8a2` found four items (two
    P1s, two P2s); all four are fixed, and every rule the touched surface claims is pinned by an executed
    mutation (twelve in all, recorded in the evidence record and the stage report).

    * **the flag's absence is the literal empty string with no presence bit** (review 1/4, a P1):
      `CheckSelectionOptions` trimmed its value and treated a trimmed-empty string as the flag's absence
      when `SelectionSupplied` was unset, and `continueWith` tested the trimmed value as well, so an
      embedding caller that set `Selection: " \t "` converted and installed the whole conversation with no
      selection report -- the one thing the request was written to prevent. The flag's absence is now
      exactly the literal empty string with no presence bit (`supplied || selection != ""`), and
      `continueWith` enters the selection for any non-empty value, where `Select`'s own trim refusal
      answers a whitespace-only value instead of the conversion skipping the reduction.
      `TestSelectionRefusesAWhitespaceOnlyAPIRequest` pins both layers (four whitespace shapes, with and
      without the bit; `Continue` and `continueWith` each asserted to refuse `ErrBadSelection` with nothing
      installed, beside a well-formed control that installs) and
      `TestCheckSelectionOptionsRefusesASpecificationWithoutARead` keeps the supplied-empty arm. Two
      executed mutations (the presence bit alone deciding the absence; the trimmed predicate at
      `continueWith`) fail it.
    * **a file-backed Pi read's line is the file's own leaf path** (review 2/4, a P1): a Pi file whose
      result entry and whose later record are *siblings* under the same call -- the shape a `/tree` switch
      back to the call leaves -- reads with the later record's parent answering the call, and the read's
      reconstruction cannot tell that from a record the file placed under the result: its tool-result step
      chained through the result and marked it on the read's line. A window ending there (`--select 0-2`)
      then passed the tree writers' leaf guard, wrote the result as the target's last entry, and the
      resumed session continued a branch the source's file left -- while the source-side leaf check, which
      only confirms the read's *last* record, answered nothing. The conversion now reads the source file's
      own entries once (readPiSourceFile, before the selection; the source-side leaf check asks the same
      read) and its own leaf path -- the walk from the file's last entry back through `parentId`, the walk
      the harness's own loader builds the context from -- is the line a selection's flags are taken from
      for every record the read answered an identity for (`Conversation.sourceFileLine`, consumed by
      `Select`; a record the read gave no identity for, an archive row and a file that cannot answer keep
      the read's own order answer, and an unreduced read keeps the reconstruction's line, D4). The writer's
      tool-result place then does not chain an on-line record through the result entry when that entry
      stands for a record the line leaves off (`resultEntryLeavesTheLine`), so `--select 0-` writes the
      file's own tree (`unresolved_source_parents` names the one off-line record whose file relation the
      projection still resolves) and the default path keeps the projection it has always taken.
      `TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath` pins the refusal (Pi and Claude, naming
      the result's identity and ordinal), the line target's omission and count, the window ending on the
      file's own path, the selected session's walk equalling the source file's own, and the unflagged arm
      (projection taken, its own count). Four executed mutations (the file's answer not carried; the walk
      not the file's leaf walk; an identity-less record answered off the path; the projection taken onto an
      off-line result) fail it, and `TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities`
      pins the identity-less boundary on a Pi v1 file whose own order is the conversation.
    * **an unwritable event yields the OpenCode outcome slot** (review 3/4, a P2): the planner installed
      whatever result reached a call's one slot first, so on a reduced window holding a call, an id-less
      terminal output, a blank result naming the call and a later non-blank result naming it too, the blank
      event took the slot, the later result was counted a repeat, and the part was dropped at write time --
      no outcome, although one was representable (the reservation that kept the id-less inference off the
      occurrence had assumed the writable result was the one owed it). `displacingOpenCodeOutcome` now
      lets a later writable recorded result take a slot an unwritable held result occupies, and
      `countUnwritableResult` counts the displaced event exactly as `toolPart` counts the same result (its
      blocks, then no text / whitespace only / a status the tool states cannot spell) so nothing is lost;
      the yield is the reduction's own, and a run without a selection keeps the first recorded outcome in
      the slot. `TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult` pins the finding's
      window for all three unwritable shapes, the categories, and the default path's unchanged answer;
      three executed mutations (no yield; the displaced event not counted; the yield unscoped) fail it.
    * **only an id-recorded progress event stands in for a call's summary** (review 4/4, a P2): the
      progress branch marked the call a `running` event named through the same pairing a terminal event
      uses, which for an id-less event is this writer's own inference (`soleOutstandingPlannedCall`), so a
      call whose only recorded outcome was its stored summary had the summary suppressed as a view of an
      event the source never linked to it and the part dropped as unpaired. The marking now reads the
      source's own link only (a non-empty `ToolUseID`), restoring 0007's id-keyed rule on the default path
      as well; `TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress` pins the finding's arm,
      the id-recorded control (the summary is not written) and the unflagged arms. Three executed mutations
      (an id-less event marking; no event marking; the marking scoped to the reduction) fail it.
