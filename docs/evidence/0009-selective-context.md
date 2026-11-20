# Evidence — 0009 Select which recorded context crosses a harness boundary

Increment of [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md) (reference route:
Claude Code → Codex). Plan: [`docs/plans/0009-selective-context.md`](../plans/0009-selective-context.md)
(§11 is the implementation's own departure list). Obligation rows:
[`0009-citations.tsv`](0009-citations.tsv).

The tree this record measures is the working tree of branch `feat/0009-selective-context` at base revision
`f3e9ec8`, Go 1.27.4, uncommitted (19 changed/added paths). Every command below was run in the repository
root unless its own `$` line says otherwise. Nothing was committed, pushed or rewritten.

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
  says `cli_version: 0.169.0 (writer-pin)` — the version `internal/ccc/codex_install.go` pins — while the
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
D7.1–D7.2.

## 3. D6: the seam, and the superseded PR's negative shapes

Each negative case is *red without* the carried disposition and *green with* it, on one reduced document
whose selected records are held fixed — the mutation clears the seam field and the writer's own walk
publishes what the seam exists to stop. The mutation is the same in all three (the document a selection
produced, with `sourceLineInactive = nil`); what differs is the shape and the assertion:

| negative case (superseded PR) | green (with the seam) | red (mutation) | test |
|---|---|---|---|
| an omitted progress event must not be published as a *completed* outcome | no `function_call_output` for the call; `outstanding_tool_calls=1`; the progress text absent | writer's summary fallback publishes `"Still working"` as the call's output | `TestSelectionAnswersNoProgressEventAsACallOutcome` (D6.2) |
| a call a compaction record replaced must not vanish uncounted | `compaction_record_tool_calls=1`, `compaction_record_tool_results=1`, `unestablished_compaction_history=0`; no output written for the accounted call | `compaction_record_tool_results` row absent | `TestSelectionCountsTheEvidenceACompactionRecordAccountsFor` (D6.5) |
| a window ending on a record the source's line left off must not activate it | `inactive_branch_records=1`, `Carried.ToolResults=0`, the off-line text absent from the bytes | the writer's walk reaches the off-line result and writes its text | `TestSelectionEndsAWindowOffTheReadsLineWithoutMovingIt` (D6.7) |

Verbatim from the progress case (`internal/ccc/selection_test.go`):

```go
	// The mutation: the same reduced document, the carried lifecycle answer gone. The writer asks the
	// message-local question, finds no recorded result in the window and writes the summary -- the
	// behaviour the carried disposition exists to stop.
	mutated := selected
	mutated.sourceLineInactive = nil
	mutatedRollout, err := ConvertToCodexRollout(mutated, testOptions())
	…
	if output, ok := rolloutOutputFor(t, mutatedRollout.Data, "call_wait"); !ok || output != progressText {
		t.Errorf("the mutation did not turn the arm red: the carried lifecycle answer gone …")
	}
```

The remaining dispositions are pinned beside these:

- **D3.3 closure eligibility's own refusal** — `TestSelectionRefusesACounterpartOffTheReadsLine`: on the
  sibling shape (the pair's result sits off the record's line) naming the call alone refuses the whole
  selection by name, mentioning both ordinals and the fact that the counterpart is off the line —
  `ErrSelectionUnrepresentable` in the default arm, `ErrStrictSelection` under `--strict-selection`, both
  naming `ordinal 2`; the same specification over a linear shape carries the counterpart as a necessity.
- **D6.1 ID-less ownership** — `TestSelectionKeepsAnIDLessResultOutOfASelectedRun`: a result with no id
  whose pairing is not the read's own is left out of a selected run and counted, never inferred onto a
  call; an id-less result the read *did* pair travels with its call.
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

## 4. D4: the default path is byte-identical (the throw-away probe)

Form the goal names: a test file added, run, quoted, removed. The probe (a `*_test.go` in `internal/ccc`)
converted every file-backed committed fixture — the new selection fixture included — through every route
with **no selection flag** and printed, per
conversion, a sha256 over a normalized copy of the written bytes (minted identities normalized: UUIDs,
`msg_ccc_*`, the Pi-family 8-hex entry ids and `firstKeptEntryId`, OpenCode's `ses_*`/`msg_*`), the carried
counts and every non-zero report row. It was copied into a pristine `git archive` checkout of base
revision `f3e9ec8` (with the selection fixture's own bytes copied in beside it, since that file is new to
this increment and the probe must compare the same input set) and run in both trees:

```
$ (cd /tmp/ccc-0009-base && go test -count=1 -tags=fts5 -run TestD4UnflaggedProbe -v ./internal/ccc) \
    > /tmp/0009-d4-before.txt
$ go test -count=1 -tags=fts5 -run TestD4UnflaggedProbe -v ./internal/ccc > /tmp/0009-d4-after.txt
$ diff /tmp/0009-d4-before.txt /tmp/0009-d4-after.txt && echo identical
identical
```

**55 conversions** (11 fixtures × 5 target routes), **360 output lines**, `diff` exit 0: every digest, byte
count and report row is identical between the base tree and this tree. The after-tree output, verbatim, is
committed as [`0009-d4-probe.txt`](0009-d4-probe.txt) (head: `fixture claude:9f2c1d47… target codex
digest=… bytes=3105 carried=5/1/1/1/2`; tail: `fixture omp:… target opencode digest=… bytes=1137
carried=4/0/0/0/2`). The OpenCode **source** direction is the one fixture pair the probe does not drive: a
member of that store is materialised by `internal/ccc/opencodefixture` and planted through the harness's
own importer, which is the native check's ground, not a file-conversion probe's. The probe file was
deleted from both trees afterwards; no gate step runs it, which is why the goal's acceptance surface names
D4 in the manifest header instead of a citation row.

## 5. The native arm: the target harness's own loader (D3/D7's machine half)

`scripts/codex-native-check.py` was extended with a re-runnable *selection arm*: after the existing
conversion/load checks it converts the fixture twice with `--select 0,4` (a window holding the first user
turn and the last assistant turn, excluding the middle by name), reads both installed rollouts through
Codex's own app-server (`thread/resume`), and asserts the loader's own view holds the kept marker and not
the excluded one — then converts a third time to prove the same command re-runs into its own fresh session.
Run against the host's Codex:

```
$ python3 scripts/codex-native-check.py --codex /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex
…                                                                                          # exit 0
PASS: the selection arm's conversion 1 reports the named window — included=['0', '4'] excluded=['1-3', '5-7'] undischarged=0
PASS: the target harness's own loader opens the selection arm's session 1 — thread/resume -> 01a1f919-3a81-… [legacy] cwd=/home/user/code/example-api

the target harness's own view of the selection arm's session 1:
    thread 01a1f919-3a81-… [legacy] provider=openai
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
PASS: the selection arm's session 1 holds the carried window and not the rest — 2 item(s); kept marker present=True, excluded present=False
PASS: the selection arm's conversion 2 reports the named window — included=['0', '4'] excluded=['1-3', '5-7'] undischarged=0
PASS: the target harness's own loader opens the selection arm's session 2 — thread/resume -> 01a1f919-3c7e-… [legacy] cwd=/home/user/code/example-api
PASS: the selection arm's session 2 holds the carried window and not the rest — 2 item(s); kept marker present=True, excluded present=False
PASS: the same selection command re-runs into its own fresh sessions — two invocation(s) installed 01a1f919-3a81-… then 01a1f919-3c7e-…
…
conversions: 4 session(s) installed (01a1f919-3856-…, then the selection arm's 01a1f919-3a81-… and 01a1f919-3c7e-…, then 01a1f919-3e7c-…); real turn(s) run: 0
33 check(s) passed, 0 failed, 2 skipped
```

The two skips are the check's unrequested real turns (`--resume-prompt`); every arm that *was* requested
passed. The arm's assertions are themselves non-vacuous: `scripts/tests/test_codex_native_check.py`'s
`test_the_selection_arms_own_checks_are_not_vacuous` drives the check's subprocess/app-server seams with a
stubbed harness and shows a wrong view (an excluded marker present, a missing kept marker, a mutation of
the applied path) makes its arm fail — the required `selftest` step runs it. The load arm and its python
case are named in the citations manifest header as deliberately not row-carried (the gate runs Go tests;
the native half is not a gate step, per AGENTS.md's rule that a native check is re-runnable and
deliberately not a gate step).

## 6. Gate

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 443 tests in 8.939s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision (compared through git's own object store); reported by cmd/ccc: 83195c8a…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 20 row(s) verified
  PASS     secrets  no leaks found in 200 file(s) this repository owns …
GATE: PASS
```

Exit code 0; no step partial, skipped or bounded (the `partial:` banner would name one). The `test`
step runs `go test -count=1 -tags fts5 ./cmd/ccc/... ./internal/ccc/...` (the flags come from
`scripts/gate-goflags.txt`), and its `3 package(s) ok` are `cmd/ccc`, `internal/ccc` and
`internal/ccc/parity` — so the gate's green covers every Go case §1–§3 cite. `citations` re-derived
this increment's 20 rows in this very run — the cited test exists in the named package and passed.

Before this final run the tree was formatted (`gofmt -w internal/ccc/fixturegen/main.go`, whose earlier
miss was the gate's own `gofmt` step catching it) and the throw-away probe file was removed; the run
above is the tree as it stands, working tree uncommitted.

## 7. Bookkeeping

- `AGENTS.md` grew in the same increment, as the rule requires: the seam paragraph now names the six
  carried dispositions and the reference route's `--select`/`--strict-selection` contract, and the layout
  paragraph names `internal/ccc/selection.go` and `conversation.go`.
- `docs/harness.md`'s native-check entry gained the selection arm
  (`scripts/codex-native-check.py`'s flagged pair and what it holds the harness's own view to), and
  `docs/goals/0009-selective-context.md`'s own `Status:` line now states the implementation, its branch
  and its evidence record.
- `docs/plans/0009-selective-context.md` §11 (implementation) records the seven departures/readings the
  plan left open, each with the test that pins it.
- `README.md` documents `--select`/`--strict-selection` in the `ccc continue` section.
- `docs/goals/README.md` carries this increment's row; the goal file stays in `docs/goals/` (the increment
  is not merged; the operator owns the merge and archive move).
- No other tree was touched: the source fixtures are read-only to this build, the scratch homes under
  `/tmp` were created by these runs, and nothing was committed, pushed or rewritten.
