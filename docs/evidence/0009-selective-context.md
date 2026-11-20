# Evidence — 0009 Select which recorded context crosses a harness boundary

Increment of [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md) (reference route:
Claude Code → Codex). Plan: [`docs/plans/0009-selective-context.md`](../plans/0009-selective-context.md)
(§11 is the implementation's own departure list). Obligation rows:
[`0009-citations.tsv`](0009-citations.tsv).

The tree this record measures is the working tree of branch `feat/0009-selective-context` at base revision
`f3e9ec8`, Go 1.27.4, committed in stages as `3b71614` (the selection) and `f430f2a` (the applied
simplifications), with the **review r1 fix round**'s changes present in the working tree at the time of
writing (uncommitted — the operator owns the merge and the archive move; §3 names every finding's fix and
the test that pins it). Every command below was run in the repository root unless its own `$` line says
otherwise; transcripts re-taken after that round say so in their own section, and nothing was pushed or
rewritten.

## 0. What was measured before the increment was written

- The reference pair's three Claude fixtures are in
  `internal/ccc/testdata/fixtures/claude/`; the selection fixture this increment added —
  `-home-user-code-awaiting/c1a74e28-3b0d-4f91-8d27-6e5b90c4a183.jsonl` — is 11 JSONL records, 2902 bytes,
  sha256 `7d3f25bd09b9b5518ba90641bf23b164df1664de623ea4d06526967877306431`, project `-home-user-code-awaiting`,
  working directory `/home/user/code/awaiting`. Its ordinals are the read's own `Message.Ordinal` values
  `0..10`, contiguous: 0 user (the question), 1 assistant, 2 call `Bash sleep 30 && echo build-ok`, 3 its
  result, 4 assistant, 5 a call `Read` whose result the fixture does not record (undischarged at the source),
  6 a compaction boundary (`AddClaudeMetaUser`'s summary), 7 a call `Bash grep -rn maxBackoffSeconds
  internal/retry`, 8 a system-injected result on a record whose parent is 7, 9 the user's follow-up
  (And the cap?), 10 the assistant's answer. Those last three records are the ones the CLI markers of §1
  probe for absence — an excluded ordinal's own words must not appear in the installed rollout.
- Host tooling: `go1.27.4 linux/amd64`; Codex `codex-cli 0.170.0` at
  `/home/azureuser/.local/share/mise/installs/codex/latest/bin/codex`. The writer's own pinned report line
  says `cli_version: 0.169.0 (writer-pin)` — the version `CodexFormatVersion`
  (`internal/ccc/codex_target.go`) pins — while the
  native arm (§5) runs the host's 0.170.0 binary as the harness it is.
- The `--select` ordinal space is the read's own supplied values, never positions; the report's
  `ordinal_space` block states min/max/count/contiguity, and an ordinal outside it is refused by name (§2).

## 1. The reference route end to end (D1, D2, D3)

Built with `go build -tags=fts5 -o /tmp/ccc0009 ./cmd/ccc` and driven against the committed fixture root:

```
$ export CLAUDE_PROJECTS_DIR=$PWD/internal/ccc/testdata/fixtures/claude
$ /tmp/ccc0009 continue claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183 --to codex --select 2 --dry-run --json
```

The report's `selection` block, verbatim (D2's classes and D1's per-record fates; `--json`):

```json
{
  "spec": "2",
  "strict": false,
  "ordinal_space": { "min": 0, "max": 10, "records": 11, "contiguous": true },
  "included":  { "count": 1, "regions": ["2"] },
  "excluded":  { "count": 9, "regions": ["0-1", "4-10"] },
  "necessities_added": {
    "count": 1,
    "added": [ { "ordinal": 3, "pulled_by": 2 } ]
  },
  "calls_undischarged": { "count": 0, "regions": [] },
  "calls_undischarged_lost": { "count": 0, "regions": [] },
  "records": [
    { "ordinal": 2, "became": "native_history", "written": ["tool_call"] },
    { "ordinal": 3, "necessity": true, "pulled_by": 2,
      "became": "native_history", "written": ["tool_result"] }
  ]
}
```

Reading: the caller named ordinal 2 (a recorded call); ordinal 3 is its recorded result and travels as a
declared necessity (D3's default half), so included + necessities + excluded = 1 + 1 + 9 = 11 = the read's
records; every written item is paired with the ordinal it came from (D1). `calls_undischarged` is empty
because ordinal 5 (the source's own undischarged call) is outside the window.

The same selection installed (no `--dry-run`), with a scratch target home and a deliberately absent target
binary so the load level is left to §5's native run:

```
$ H=$(mktemp -d /tmp/ccc0009-home-XXXX)
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select 0-4 \
      --target-home "$H" --target-binary ccc-no-such-target-harness          # exit 0
claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183 → codex
  source:       …/fixtures/claude/-home-user-code-awaiting/c1a74e28-3b0d-4f91-8d27-6e5b90c4a183.jsonl
  read from:    source
  conversation: 11 message(s), 4 user, 7 assistant, 3 tool call(s), 1 compaction boundary(ies)
  boundary:     2902 byte(s) read; the file is the size the read's own snapshot recorded …
  records:      the source's records end at a line feed at byte 2902: the file ends at a record boundary
  selection:    0-4; ordinal space 0..10: 11 record(s)
  included:     5 record(s) at 0-4
  excluded:     6 record(s) at 5-10
  necessities:  none added
  undischarged: 0 call(s) at none
  record 0       native history (user_message)
  record 1       native history (assistant_message)
  record 2       native history (tool_call)
  record 3       native history (tool_result)
  record 4       native history (assistant_message)
  converted:    codex-rollout-jsonl, 1955 bytes: 3 message(s), 1 tool call(s), 1 tool result(s),
                0 boundary(ies), 1 turn(s)
  omitted:      tool_result_sources ×1
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: unresolved_source_parents ×4
  installed:    /tmp/ccc0009-home-1zdX/sessions/2026/05/08/rollout-2026-05-08T08-00-00-01a1f91b-b2ab-7ad5-a141-c18d18a691a7.jsonl [hard-link]
  source file:  unchanged (sha256 7d3f25bd…, taken before and after the conversion)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  not-verified (the target harness's own loader (codex app-server, thread/resume))
                ccc-no-such-target-harness is not on PATH, so the installed session's native load could not be verified
  warning:      the selection carried 5 of the read's 11 record(s); 6 record(s) at 5-10 did not travel (specification "0-4")
```

The installed rollout holds exactly the window's evidence and none of the excluded turns' content:

```
$ G=$(ls /tmp/ccc0009-home-1zdX/sessions/2026/05/08/*.jsonl)
$ for m in "build-ok" "And the cap?" "maxBackoffSeconds" "Summary: the build succeeded"; do
    printf '%-32s %s\n' "$m" "$(grep -c -- "$m" "$G")"; done
build-ok                         2
And the cap?                     0
maxBackoffSeconds                0
Summary: the build succeeded     0
```

`build-ok` appears twice because the kept call's input and its kept result each hold it (ordinals 2 and 3);
the excluded ordinals 5–10's markers appear zero times. The run's `records:` and boundary lines also state
that the *source* was read at its own end and that its bytes did not move (`source file: unchanged … taken
before and after`), i.e. the conversion wrote a fresh target session and replaced nothing.

Class reconciliation is pinned in Go by `TestSelectionAccountsForEveryCarriedRecord` (D1) and the report
halves by `cmd/ccc/selection_route_test.go`'s `TestContinueWithASelectionReportsTheClassesAndFates` and
`TestContinueWithASelectionInstallsExactlyTheWindow` (D2.1, D2.2) — the latter also drives the human
block's *non-zero* classes (`--select 2` renders `necessities:  1 added: 3 (pulled by 2)` and the
necessity's own record line `record 3       native history (tool_result) [necessity, pulled by 2]`;
`--select 5`, the call the source never answered, renders `undischarged: 1 call(s) at 5`); the rows are in
the citations manifest.

## 2. Refusals: the specification, strict closure, and the pair gate (D5, D7)

Every refusal below is the core's own sentence on stderr and the usage exit code; no target file is
written. The `$` lines are the actual invocations (fixture root as in §1):

```
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select "x" --dry-run
ccc: the selection cannot be honoured: "x" is not an ordinal: write values and ranges like 0-5,9,12-
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select=-1 --dry-run
ccc: the selection cannot be honoured: "-1" is not an ordinal: write values and ranges like 0-5,9,12-
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select "5-3" --dry-run
ccc: the selection cannot be honoured: the region "5-3" is inverted: a range is written low-high, like 0-5
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select "1-3,3" --dry-run
ccc: the selection cannot be honoured: the region "3" names an ordinal the earlier region "1-3" already names: a specification names each ordinal once, as one region
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select "11" --dry-run
ccc: the selection cannot be honoured: the ordinal 11 is outside the read's own ordinal values (0-10: 11 record(s)): a specification names the values this read supplied, never a position in it
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select "" --dry-run
ccc: the selection cannot be honoured: the specification is empty: name the ordinal values to carry, as regions like 0-5,9,12-
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to codex --select 2 --strict-selection --dry-run
ccc: the selection separates a recorded tool call from its recorded result: the call toolu_09Selection0000000000000001 recorded at ordinal 2 is selected and its recorded result at ordinal 3 is not, so the conversion would carry a call whose recorded output stays behind; name a region that holds both records, or drop --strict-selection to carry the counterpart as a necessity
exit=2
$ /tmp/ccc0009 continue claude:c1a74e28-… --to pi --select 0- --dry-run
ccc: a selection is honoured on one pair only: a Claude Code source with a Codex target (claude:… --to codex): this run reads "claude" and writes "pi"
exit=2
```

The last line is D7's core answer, raised before the read; the CLI half runs before the source is opened —
`TestSelectionIsRefusedOnEveryOtherPairBeforeTheSourceIsOpened` pins that the route's own reader is called
**zero** times on a refused pair and that no target file appears. The Go rows for §2 are D5.1–D5.3 and
D7.1–D7.2. The `--select=-1` line was re-taken after the review round's fix: the earlier build answered
`"" holds no ordinal` for a token whose dash left an empty lower half, and the sentence now quotes the
token the caller wrote.

## 3. D6: the seam, and the superseded PR's negative shapes

Each negative case is *red without* the carried disposition and *green with* it, on one reduced document
whose selected records are held fixed — and the mutation now **invalidates the individual answer while
selected mode stays on** (a mutation that clears `sourceLineInactive` switches the whole writer to the
unflagged path, so a case mutated that way would pass even if selected mode recomputed its answers from
the reduced records; the review round's fix replaced every such mutation). What differs per case is the
answer invalidated and the assertion:

| negative case (superseded PR) | green (with the seam) | red (mutation: the individual answer invalidated, selected mode on) | test |
|---|---|---|---|
| an omitted progress event must not be published as a *completed* outcome | no `function_call_output` for the call; `outstanding_tool_calls=1`; the progress text absent | `sourceCallFinality` answered `completed` and `sourceCallRecordedEvents` empty for the call: the writer's summary fallback publishes `"Still working"` as the call's output | `TestSelectionAnswersNoProgressEventAsACallOutcome` (D6.2) |
| a call a compaction record replaced must not vanish uncounted | `compaction_record_tool_calls=1`, `compaction_record_tool_results=1`, `unestablished_compaction_history=0`; no output written for the accounted call | `sourceBoundaries` emptied: the accounted counts fall to zero while `unestablished_compaction_history=1` names the missing ledger as the unknown it is | `TestSelectionCountsTheEvidenceACompactionRecordAccountsFor` (D6.5) |
| a window ending on a record the source's line left off must not activate it | `inactive_branch_records=1`, `Carried.ToolResults=0`, the off-line text absent from the bytes | `sourceLineInactive` answered all-on-line: the writer's walk reaches the off-line result and writes its text | `TestSelectionEndsAWindowOffTheReadsLineWithoutMovingIt` (D6.7) |

Verbatim from the progress case (`internal/ccc/selection_test.go`):

```go
	// The mutation: the same reduced document with this call's carried dispositions invalidated to what
	// the reduced stream alone would suggest -- no recorded event for the call in the window, and the
	// surviving summary looking like the completion -- while selected mode stays on. The writer then
	// publishes the summary: the behaviour the carried answers exist to stop. (Clearing the seam's
	// marker instead would switch the whole writer to the unflagged path, so the arm would pass even if
	// selected mode recomputed these answers from the window.)
	mutated := selected
	place := callOccurrence{message: 0, call: 0}
	mutated.sourceCallFinality = map[callOccurrence]callFinality{place: callFinalityCompleted}
	mutated.sourceCallRecordedEvents = map[callOccurrence]bool{}
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	…
	if output, ok := rolloutOutputFor(t, mutatedRollout.Data, "call_wait"); !ok || output != progressText {
		t.Errorf("the mutation did not turn the arm red: without the carried dispositions the "+
			"writer wrote no summary output (present=%v, %q)", ok, output)
	}
```

The remaining dispositions are pinned beside these:

- **D3.3 closure eligibility's own refusal** — `TestSelectionRefusesACounterpartOffTheReadsLine`: on the
  sibling shape (the pair's result sits off the record's line) naming the call alone refuses the whole
  selection by name, mentioning both ordinals and the fact that the counterpart is off the line —
  `ErrSelectionUnrepresentable` in the default arm, `ErrStrictSelection` under `--strict-selection`, both
  naming `ordinal 2`; the same specification over a linear shape carries the counterpart as a necessity.
  The refusal carries the pair's roles from the closure rather than re-reading them off the record that
  pulled the counterpart in: a mixed entry whose result's record carries a call of its own still answers
  "the call recorded at ordinal 1 and the result recorded at ordinal 2" (the case's mixed-entry arm,
  which the old role guess answered in the swapped order).
- **D6.1 ID-less ownership** — `TestSelectionKeepsAnIDLessResultOutOfASelectedRun`: a result with no id
  is left out of a selected run and counted (`unpaired_tool_results`), never inferred onto a call — the
  read recorded no link for it, and the pairing the seam carries holds only links the read established;
  the *unflagged* conversion of the same read keeps the writer's own disclosed inference (the sole
  outstanding call), the behaviour the goal's non-scope leaves untouched.
- **D6.3 a retained progress event is published only through its call** —
  `TestSelectionPublishesAProgressEventOnlyThroughItsCall`: the caller-named progress record is omitted
  from this format's output and counted (`nonterminal_tool_events` ×1) while the call is written and
  counted outstanding; the *unflagged* conversion of the same read keeps its historic answer (the event's
  text written as the call's output) and carries no reduction row — D4's frozen path inside the same case.
- **D6.4 unknown-undischarged vs omitted-by-selection** — `TestSelectionLeavesAnUndischargedCallAsTheSourceLeftIt`:
  a call the source never answered keeps its own finality and is counted (`outstanding_tool_calls`), while
  a call the *selection* dropped is counted in `excluded` and not in `undischarged`.
- **D6.5's three ledger states** — the same compaction case: the synthetic boundary whose record carries
  its own call and result answers the accounted ledger (green above); the same boundary with no evidence
  of its own read through a Codex provider (whose reader skips `replacement_history`) answers
  **not established** (`unestablished_compaction_history=1`, and no accounted call) — the fixture carries
  this state's label; a Claude boundary answers an established **none** (no unestablished row). On every
  real run of the reference route the unestablished state is 0 (a Claude boundary is an established none),
  which is why its own reachable-by-provider shape is a synthetic Go shape, not a CLI run.
- **D6.6 ancestry** — `TestSelectionKeepsTheNearestCarriedAncestor`: `a→b→c→d` with the window
  `0,3` carries `d`'s nearest carried ancestor `a` (position 0), and a record whose ancestry lies wholly
  outside the window answers none (`-1`). The Codex route has no consumer for it (one sequence, no parent
  relation); the tree-target increments will read it rather than re-derive it.
- **D6.7's pre-cut membership** — the third row above.
- **X1 a document a selection produced is refused as a selection input** —
  `TestSelectionRefusesADocumentASelectionProduced`: a reduced document's own seam fields are what the rules
  act on, so a second selection over it is refused by name rather than re-derived.
- **X2 recorded instants survive the cut** — `TestSelectionTimeStampsAreNeverOlderThanTheReadsRecord`: a
  selected window's written records keep the read's own instants; the writer's clock rule never re-times a
  record because a neighbour was cut.
- **D6.2's own order rule** — `TestSelectionAnswersTheLastOutcomeTheReadRecorded`: a call a read answered
  twice (an errored result and a later completion) takes the last outcome the read recorded, and every one
  of 200 calls answers the same value — the pairing map's iteration order never decides, which is D6's
  purity bound (the same document, the same answer). Without the read-order walk the case fails: the two
  orders it builds (`errored` then `completed`, and the reverse) both let the map's order pick the answer.
- **D1's reason column** — a lost record's reason is the loss category the walk charged it to, not the
  no-reason fallback: `TestSelectionPublishesAProgressEventOnlyThroughItsCall` (the retained progress
  event's record → `nonterminal_tool_events`), `TestSelectionKeepsAnIDLessResultOutOfASelectedRun` (the
  id-less result's record → `unpaired_tool_results`), and
  `TestSelectionNamesTheCategoryBehindALostToolResult` (the empty and whitespace-only results'
  records → `empty_tool_results` / `whitespace_only_tool_results`) each assert the table's `reason` is
  that category.

**The review round's fixes (r1), each with the gate-runnable test that pins it.** The seam grew the
answers the reduced stream was being asked to carry, and two refusals were added where the contract said
refuse:

- **D3's closure eligibility gained its writer-side shape** — a counterpart the route's own writer does
  not publish (its recorded outcome has no text this format's output could carry) refuses the whole
  selection by name, the pair and both ordinals, *before anything is installed*:
  `TestSelectionRefusesACounterpartTheWriterCannotCarry` drives the empty-result shape through `Select`,
  the writer's fates and `lostNecessityRefusal`, and then through `Continue` over an injected reader
  (the target home stays empty); the same records named *explicitly* still convert, the result's loss
  with its reason being D1's accounting rather than a declared necessity dropped.
- **D6(c) omitted-by-selection vs absent-in-source** — `sourceExcludedResultOwners` carries, per carried
  result, the read's own answer that it *was* paired with a call the selection left out, and the writer
  counts those under `tool_results_of_excluded_calls` instead of collapsing them with link-less results:
  `TestSelectionNamesAnExcludedOwnerRatherThanALinklessResult` (the progress event alone selected; the
  mutation empties that one set while selected mode stays on and the category flips to
  `unpaired_tool_results`).
- **D6(a)'s call-side event answer** — `sourceCallRecordedEvents` carries, per carried call, whether the
  read's pairing named any recorded event for it, and the writer's summary fallback consumes it instead of
  scanning the reduced pairing map (which cannot show an event the selection left out). The D6.2 negative
  case above is the red arm.
- **D6(b)'s split** — `calls_undischarged_lost` splits the undischarged class against the writer's own
  fates: the calls on records the writer did not publish are counted there (each row's own category names
  the record's loss), so the rest of the class is exactly the conversion's `outstanding_tool_calls` and
  the two report surfaces reconcile. `TestSelectionReconcilesUndischargedCallsWithTheWritersFates`
  (a selected sidechain record carrying an unanswered call) asserts the split and the arithmetic; a
  `calls lost:` line joins the human block when the split is non-zero.
- **D1's reason column for a system record** — the writer's `IsSystem` branch sets the record's fate
  reason to `system_records` (it set the counter but not the reason, so a standalone reminder's row said
  only "no target record states this source record"): `TestSelectionAttributesASystemRecordsOmissionToItsRow`.
- **X1's writer half** — writers that do not consume the seam (`ConvertToClaudeTranscript`,
  `ConvertToPiSession`, `ConvertToOMPSession`, `ConvertToOpenCodeSession`) refuse a document a selection
  produced by name (`ErrSelectionUnsupportedWriter`), rather than letting the OpenCode writer's own line
  walk make a selection-omitted result the window's new leaf:
  `TestSelectionRefusesADocumentASelectionProduced` drives all four and the reference writer as the
  control.
- **D7's strict arm is its presence, not its value** — `--strict-selection=false` is the flag: the CLI
  tracks the flag's presence beside `--select`'s, `ContinueOptions.StrictSelectionSupplied` feeds
  `CheckSelectionOptions` and `selectionRequested`, and the D7 test observes the run's reader calls
  (`SourceReads` is an interface so an injected reader can count them) instead of inferring "the source
  was not opened" from an error sentence: `TestSelectionIsRefusedOnEveryOtherPair` (core),
  `TestSelectionIsRefusedOnEveryOtherPairBeforeTheSourceIsOpened` (CLI, the strict-false arms included),
  `TestSelectionRefusesABadSpecificationByName` (the strict arm alone is a request about nothing).
- **The native arm asserts the window exactly** — `scripts/codex-native-check.py` asserts the ordered
  role/text window (a marker search accepted a one-item view, or the marker in the wrong role) and
  re-hashes the first selected artifact across the second invocation;
  `scripts/tests/test_codex_native_check.py` supplies five window controls and the overwrite control, and
  the real-loader run is quoted in §5.
- **D4's probe drove every committed fixture** — the OpenCode source included, materialised through
  `internal/ccc/opencodefixture` (§4).

## 4. D4: the default path is byte-identical (the throw-away probe)

Form the goal names: a test file added, run, quoted, removed. The probe (a `*_test.go` in `internal/ccc`)
converted every committed fixture — the file-backed ones, the new selection fixture included, **and the
OpenCode source fixture**, materialised from its committed document through `internal/ccc/opencodefixture`
and read back through the inherited provider — through every route with **no selection flag** and printed,
per conversion, a sha256 over a normalized copy of the written bytes (minted identities normalized: UUIDs,
`msg_ccc_*`, the Pi-family 8-hex entry ids and `firstKeptEntryId`, OpenCode's `ses_*`/`msg_*`), the carried
counts and every non-zero report row. It was copied into a pristine `git archive` checkout of base
revision `f3e9ec8` (with the selection fixture's own bytes copied in beside it, since that file is new to
this increment and the probe must compare the same input set) and run in both trees:

```
$ (cd /tmp/ccc-0009-base && CCC_D4_PROBE_OUT=/tmp/0009-d4-before.txt \
    go test -count=1 -tags=fts5 -run TestD4UnflaggedProbe ./internal/ccc)
$ CCC_D4_PROBE_OUT=/tmp/0009-d4-after.txt \
    go test -count=1 -tags=fts5 -run TestD4UnflaggedProbe ./internal/ccc
$ diff /tmp/0009-d4-before.txt /tmp/0009-d4-after.txt && echo identical
identical
```

**60 conversions** (12 fixtures × 5 target routes), **408 output lines**, `diff` exit 0: every digest, byte
count and report row is identical between the base tree and this tree. The after-tree output, verbatim, is
committed as [`0009-d4-probe.txt`](0009-d4-probe.txt) (head: `fixture claude:9f2c1d47… target codex
digest=… bytes=3101 carried=messages=5 user=2 assistant=3 calls=1 results=1 boundaries=1 turns=2`;
tail: `fixture opencode:ses_0008opencodefixture00000001 target opencode digest=… bytes=1865
carried=messages=5 user=2 assistant=3 calls=1 results=1 boundaries=0 turns=2`). The OpenCode **source**
direction is driven by the probe's own offline materialiser (`internal/ccc/opencodefixture` writes the
committed export document into a throw-away store, and the read goes through the inherited provider
exactly as `OPENCODE_DIR` would resolve it); the harness's own importer remains the native check's ground
for the *installed* half. The probe file was deleted from both trees afterwards; no gate step runs it,
which is why the goal's acceptance surface names D4 in the manifest header instead of a citation row.

The probe's own identity normalization is stated where it matters: a diff hidden inside `<entry>` would
have to be a difference in an eight-hex token, and the fixture set's recorded content carries none. Both
trees ran the *same* probe bytes and the same fixture set, and the HEAD tree was run twice with identical
output before the comparison (determinism measured, not assumed).

## 5. The native arm: the target harness's own loader (D3/D7's machine half)

`scripts/codex-native-check.py` was extended with a re-runnable *selection arm*: after the existing
conversion/load checks it converts the fixture twice with `--select 0,4` (the fixture's first user record
and the assistant record that answers it; the read's other turns are excluded by name), reads both
installed rollouts through Codex's own app-server (`thread/resume`), and asserts the loader's own view
lists the carried window **exactly** -- the ordered role/text pairs, both records, each in the role the
source recorded -- and nothing of the excluded records; the two invocations installing two distinct fresh
session ids, the first artifact's bytes surviving the second, is what proves the same command re-runs into
its own fresh session without replacing the earlier one.
Run against the host's Codex (re-taken on this tree, after the review round's fixes):

```
$ python3 scripts/codex-native-check.py --codex /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex
…                                                                                          # exit 0
PASS: the selection arm's conversion 1 reports the named window — included=['0', '4'] excluded=['1-3', '5-7'] undischarged=0; installed sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1f96c-4361-7689-93b5-c1fa7a074835.jsonl
PASS: the target harness's own loader opens the selection arm's session 1 — thread/resume -> 01a1f96c-4361-7689-93b5-c1fa7a074835 [legacy] cwd=/home/user/code/example-api

the target harness's own view of the selection arm's session 1:
    thread 01a1f96c-4361-7689-93b5-c1fa7a074835 [legacy] provider=openai
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
PASS: the selection arm's session 1 holds the carried window and not the rest — 2 item(s); messages=[('userMessage', 'Summarize the retry policy in internal/retry/policy.go.'), ('agentMessage', 'Four attempts with exponential backoff, capped at 30s between attempts.')] against the window [('userMessage', 'Summarize the retry policy in internal/retry/policy.go.'), ('agentMessage', 'Four attempts with exponential backoff, capped at 30s between attempts.')]; excluded present=False
PASS: the selection arm's conversion 2 reports the named window — included=['0', '4'] excluded=['1-3', '5-7'] undischarged=0; installed sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1f96c-4575-703f-93c1-055aef20178b.jsonl
PASS: the target harness's own loader opens the selection arm's session 2 — thread/resume -> 01a1f96c-4575-703f-93c1-055aef20178b [legacy] cwd=/home/user/code/example-api
PASS: the selection arm's session 2 holds the carried window and not the rest — 2 item(s); messages=[('userMessage', 'Summarize the retry policy in internal/retry/policy.go.'), ('agentMessage', 'Four attempts with exponential backoff, capped at 30s between attempts.')] against the window [('userMessage', 'Summarize the retry policy in internal/retry/policy.go.'), ('agentMessage', 'Four attempts with exponential backoff, capped at 30s between attempts.')]; excluded present=False
PASS: the same selection command re-runs into its own fresh sessions — two invocation(s) installed 01a1f96c-4361-7689-93b5-c1fa7a074835 then 01a1f96c-4575-703f-93c1-055aef20178b
PASS: the first selection's session is unchanged by the second — sha256 da8a733ad351c25d… before and after the second selection's conversion and load
…
conversions: 4 session(s) installed (01a1f96c-4148-7…, then the selection arm's 01a1f96c-4361-7… and 01a1f96c-4575-7…, then 01a1f96c-477d-7…); real turn(s) run: 0
34 check(s) passed, 0 failed, 2 skipped
```

The same script run for the *other* committed fixture is the arm's own unattempted path
(`--session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef`, the tooling read the arm's specification is not written
about):

```
$ python3 scripts/codex-native-check.py --codex /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex \
    --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef
SKIP: the selection arm converts the fixture it is written for — not attempted: --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef names another session than the arm's own fixture (9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10)
…
conversions: 2 session(s) installed (01a1f96c-71a5-7…, then 01a1f96c-73d3-7…; the selection arm did not run because --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef is not its fixture (9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10)); real turn(s) run: 0
26 check(s) passed, 0 failed, 3 skipped                                       # exit 0
```

Before this fix the same tooling-session run reported four *failed* checks (the arm asked whether the
tooling read holds a window its records never made), so this run is the executed contrast.

The two default-run skips are the check's unrequested real turns (`--resume-prompt`); every arm that *was*
requested passed. The arm's assertions are themselves non-vacuous: `scripts/tests/test_codex_native_check.py`'s
`test_the_selection_arms_own_checks_are_not_vacuous` drives the check's subprocess/app-server seams with a
stubbed harness through five window controls -- a document reporting another window, a view dropping the
window, a view listing only the kept record, a view listing the kept record under the user's role (the
shape a marker search accepted), and a view that also carries the excluded record's marker -- plus the
repeat assertion's own overwrite control (the second selection replacing the first artifact, which only the
first artifact's bytes comparison can catch); each control fails the arm's own checks and only those; a
further case drives a run whose `--session` is not the arm's fixture and asserts the arm is a named skip
rather than a run. The required `selftest` step runs all of them. The load
arm and its python
case are named in the citations manifest header as deliberately not row-carried (the gate runs Go tests;
the native half is not a gate step, per AGENTS.md's rule that a native check is re-runnable and
deliberately not a gate step).

## 6. Gate

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 444 tests in 9.173s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision (compared through git's own object store); reported by cmd/ccc: 83195c8a…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 25 row(s) verified
  PASS     secrets  no leaks found in 200 file(s) this repository owns …
GATE: PASS
```

Exit code 0; no step partial, skipped or bounded (the `partial:` banner would name one). The `test`
step runs `go test -count=1 -tags fts5 ./cmd/ccc/... ./internal/ccc/...` (the flags come from
`scripts/gate-goflags.txt`), and its `3 package(s) ok` are `cmd/ccc`, `internal/ccc` and
`internal/ccc/parity` — so the gate's green covers every Go case §1–§3 cite.
`citations` re-derived this increment's **25** rows in this very run — the cited test exists in the named
package and passed.

This transcript is the working tree *after* the review r1 fix round (uncommitted; the increment's earlier
commits are `3b71614` and `f430f2a`). The earlier run of this record listed `3624` file(s) because the
three new `.go` files were untracked then; the throw-away probe file (D4) was removed before both runs.

## 7. Bookkeeping

- `AGENTS.md` grew in the same increment, as the rule requires: the seam paragraph now names the carried
  dispositions and the reference route's `--select`/`--strict-selection` contract, and the layout
  paragraph names `internal/ccc/selection.go` and `conversation.go`. The review r1 fix round updated both
  selection bullets: the seam is eight fields (the excluded-owner answer and the call-side event answer
  joined the six), the writer-side closure refusal is named beside the line's, and the strict flag is its
  presence — an explicit false included.
- `docs/harness.md`'s native-check entry gained the selection arm
  (`scripts/codex-native-check.py`'s flagged pair and what it holds the harness's own view to), and
  `docs/goals/0009-selective-context.md`'s own `Status:` line now states the implementation, its branch
  and its evidence record. The review round corrected the entry's two untrue statements about the arm
  (which records `0,4` names; the arm runs at the load level and a prompt flag adds no turn to it) and
  recorded that the arm is the default fixture's own and a skip for another `--session`; the r1 round
  updated it again for the exact ordered window and the first artifact's bytes across the second
  invocation.
- `docs/plans/0009-selective-context.md` §11 (implementation) records the seven departures/readings the
  plan left open, each with the test that pins it, and its review-r1 subsection records this fix round's
  seven dispositions (the writer-side closure refusal, the two seam fields, the undischarged split, the
  writer refusals, the strict presence, the observed-reader D7 test, the per-case mutations, the exact
  native window and the probe's OpenCode-source arm).
- `README.md` documents `--select`/`--strict-selection` in the `ccc continue` section; the review round
  added the closure-eligibility outcome (a counterpart off the read's line refuses the whole selection),
  and the r1 round the writer-side refusal (a counterpart the route's writer cannot state truthfully),
  the strict flag's presence rule and the undischarged class's lost part.
- `docs/goals/README.md` carries this increment's row; the goal file stays in `docs/goals/` (the increment
  is not merged; the operator owns the merge and archive move).
- No other tree was touched: the source fixtures are read-only to this build, the scratch homes under
  `/tmp` were created by these runs, and nothing was pushed or rewritten — the increment's own commits are
  `3b71614` (the selection) and the simplify/apply round's `f430f2a`, with the review r1 fix round's
  changes left uncommitted in the working tree for the operator.
