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
      §4's transcript is that revision's own run; the two trees' shared 1024 lines are byte-identical with
      recorded instants compared.
    * the **D3 pins** (reviews R-25, R-26): the closure cases exercised isolated pairs and the strict case
      checked neither ordinal, so a one-pass closure and a refusal with swapped ordinals would both have
      passed. `TestSelectionClosesTheCounterpartGraphToAFixedPoint` needs a *backward* two-hop pull (a
      record the window pulled in carries a result of another call), and the strict case asserts each
      sentence half with the ordinal of the side it names.
