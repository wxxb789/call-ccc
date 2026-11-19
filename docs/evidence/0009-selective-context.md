# 0009 — Select which recorded context crosses a harness boundary: evidence

Contract: [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md) (this increment
leaves it in `docs/goals/` with its row marked *Implemented on `feat/0009-selective-context`; awaiting
operator acceptance*, per `AGENTS.md`).
Plan: [`docs/plans/0009-selective-context.md`](../plans/0009-selective-context.md) (its §11 records every
departure this record's sections rest on).
Citations: [`0009-citations.tsv`](0009-citations.tsv) (the machine-checked half of §6).
Harness: Codex `0.170.0` (`codex-cli 0.170.0`, the loader the recorded run verified at the load level),
the pinned AgentsView carry `83195c8a6861b8c74a4b6cb87512817fed6f8584` (reported by every report's own
`upstream_revision` and by `ccc inspect`'s `carried:` line). Every command below ran on this host
(`linux x64`), in this repository, on branch `feat/0009-selective-context`. Output is quoted verbatim;
`…` marks only a line the record's own layout cut, and nothing else was edited.

## 0. What was measured before anything was written

| Question | Measured answer |
| --- | --- |
| what ordinals a read supplies | `Message.Ordinal` is the value the reader assigned: the Claude reader numbers its own records 0..n-1 (the selection fixture reads as eleven records, ordinals 0..10), while a reader that preserves a source field (Shelley's `sequence_id`) produces the source's own values — which is why the selector matches *values* and the space is reported (`selection.ordinal_space`) rather than assumed |
| where a call's result lives | the Claude fixture family records a call on one line and its result on a later one: the selection fixture's build call is at ordinal 2 and its result at ordinal 3, and its cap call at ordinal 7 is answered by the tool result written on the *system-reminder* record at ordinal 8 (`IsSystem` true with `ToolResults` populated, the tooling fixture's own measured shape) |
| a call the source never answered | the selection fixture's read call (ordinal 5) has no result anywhere in the read: the state `calls_undischarged` names, which no counterpart rule can discharge |
| a call whose only recorded result is *progress* | the inherited Codex readers record a subagent's `running` notification as a result event (`internal/parser/codex_parser_test.go`'s "running-only wait output is preserved as a result event"), and the review's finding: such an event is not an outcome, so the call is counted `calls_undischarged` (the status words the readers write for progress — `running`, `pending`, `streaming`, `started`, `in_progress` — are the ones `resultStatesAnOutcome` names) |
| what a `subtype: hook_result` line reads as | **nothing**: the Claude reader skips the provider's own bookkeeping lines (measured on the fixture's first draft — ten records, nine messages — which is why the fixture's system record is the system-reminder promotion shape instead; plan §11.5) |
| what a target does with an undischarged call | Codex, Claude, Pi and OMP write the call with no output; OpenCode's format has no state for a call that has not finished, so it leaves the call out and counts it (`unpaired_tool_calls`, measured on the fixture through `ConvertToOpenCodeSession`). The report's warning names both halves as the writer's own answer rather than claiming the target records the call |
| the seam's own two dispositions | a selection that names the call of ordinal 2 alone carries the result at ordinal 3 as a `necessities_added` entry (`pulled_by` 2), and the Codex writer then holds the pair with `unpaired_tool_results = 0`; the same selection under `--strict-selection` is refused by name, before anything is written |
| what an unflagged run writes | nothing the previous revision did not: a throw-away before/after probe over five routes × the eleven fixtures the two trees share prints the same 987 lines and the same normalized bytes on HEAD's tree and on this tree, and the increment's new fixture adds its own 121 lines with no before-run (§4; the probe's exact source is `docs/evidence/0009-d4-probe.txt`) |

## 1. The recorded run: a selection converts, installs and **Codex's own loader opens it**

`bin/ccc continue claude:c1a74e28-… --to codex --select 2 --verify load` (the fixture is a *copy* under a
scratch tree, so nothing the run does can touch the committed bytes; the target home is a scratch one):

```
$ go build -tags fts5 -o bin/ccc ./cmd/ccc
$ mkdir -p /tmp/ccc-0009-ev2/src/claude/-home-user-code-awaiting /tmp/ccc-0009-ev2/codex-home
$ cp internal/ccc/testdata/fixtures/claude/-home-user-code-awaiting/*.jsonl \
      /tmp/ccc-0009-ev2/src/claude/-home-user-code-awaiting/
$ CLAUDE_PROJECTS_DIR=/tmp/ccc-0009-ev2/src/claude bin/ccc continue \
    claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183 --to codex \
    --target-home /tmp/ccc-0009-ev2/codex-home --select 2 --verify load --json
exit 0
```

The three states the core design keeps apart, from the report's own document:

```
conversion:   "state": "converted", "format": "codex-rollout-jsonl", "read_from": "source",
              carried {messages 0, user_messages 0, assistant_messages 0, tool_calls 1, tool_results 1,
              compaction_boundaries 0, turns 1, bytes 896}
installation: "state": "installed", "mechanism": "hard-link",
              "path": /tmp/ccc-0009-ev2/codex-home/sessions/2026/05/08/rollout-2026-05-08T08-00-04-01a1f4d4-5923-763c-9203-89dba597f92f.jsonl
              "source_check_state": "unchanged", "detail": sha256 7d3f25bd09b9b5518ba90641bf23b164df1664de623ea4d06526967877306431, taken before and after the conversion
verification.structure: "state": "verified", "method": the inherited AgentsView Codex reader over the installed session,
              read_back {messages 1, assistant_messages 1, tool_calls 1, tool_results 1}
verification.load: "state": "verified", "method": the target harness's own loader (codex app-server, thread/resume),
              "detail": the harness's loader answered with this session's own id and path; opening a session makes the
              harness record its own settings in it (measured: one thread_settings_applied record), and no session other
              than the one it opened is touched
              thread_id 01a1f4d4-5923-763c-9203-89dba597f92f, path as above,
              binary /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex, binary_version 0.170.0
target_session: provider codex, session_id 01a1f4d4-5923-763c-9203-89dba597f92f,
              resume_command CODEX_HOME=/tmp/ccc-0009-ev2/codex-home codex resume 01a1f4d4-5923-763c-9203-89dba597f92f
              model_provider openai
selection:    "spec": "2", "strict": false,
              ordinal_space {min 0, max 10, records 11, contiguous true},
              included {count 1, regions ["2"]},
              excluded {count 9, regions ["0-1","4-10"]},
              necessities_added {count 1, added [{ordinal 3, pulled_by 2}]},
              calls_undischarged {count 0, regions null}
warnings:     the conversion carries a selection of the read: the specification "2" selected 1 record(s) at
              ordinals 2, 9 record(s) are excluded, and 1 record(s) were added as a structural necessity
              (a recorded call and its recorded result are carried together)
```

The stderr of that run is the harness's own (two `WARNING: proceeding, even though we could not create PATH
aliases: Refusing to create helper binaries under temporary dir "/tmp"` lines and two `ERROR
codex_app_server: Codex's Linux sandbox uses bubblewrap and needs access to create user namespaces.`
lines — the app-server probe and the load, measured identically for the accepted routes).

**D1's pairing: every record of the installed session with the ordinal it came from.** The rollout the
report names, read line by line:

```
 0 session_meta
 1 turn_context
 2 response_item  function_call        call_id=toolu_09Selection0000000000000001 name=Bash
                                      arguments={"command":"sleep 30 \u0026\u0026 echo build-ok","description":"run the long build"}
 3 response_item  function_call_output call_id=toolu_09Selection0000000000000001 output='build-ok: 3 packages compiled'
 4 event_msg      thread_settings_applied
```

| rolled-out record | source ordinal | the fixture's own recorded bytes there |
| --- | --- | --- |
| 2 `function_call` (`toolu_09Selection0000000000000001`, name `Bash`, arguments `{"command":"sleep 30 && echo build-ok","description":"run the long build"}`) | 2 | the `tool_use` block of the fixture's third line, name and input verbatim |
| 3 `function_call_output` (same call id, output `build-ok: 3 packages compiled`) | 3 | the `tool_result` block's content on the fixture's fourth line — the record the selection did **not** name and the necessity carried |
| 0, 1, 4 | — | the target's own envelopes (`session_meta`, `turn_context`) and the record Codex's loader wrote when it opened the session (`thread_settings_applied`); no source record stands behind them |

Nothing else of the read is in the file: ordinals 0, 1 and 4–10 (nine records, `excluded.regions`
`["0-1","4-10"]`) left no bytes. The target's own loader opened it at the load level with the session's
own id and path (§1's block), so the selection's target is not merely well-formed — it is one Codex's own
`thread/resume` loads.

## 2. The report's classes, on a window that holds the awkward shapes

`--select 5-10 --dry-run --json` (the window holds the undischarged call, the compaction summary, the cap
call and the system record that answers it, the follow-up question and the final record):

```
selection: {
 "spec": "5-10", "strict": false,
 "ordinal_space": {"min": 0, "max": 10, "records": 11, "contiguous": true},
 "included": {"count": 6, "regions": ["5-10"]},
 "excluded": {"count": 5, "regions": ["0-4"]},
 "necessities_added": {"count": 0, "added": null},
 "calls_undischarged": {"count": 1, "regions": ["5"]}
}
carried: {'messages': 2, 'user_messages': 1, 'assistant_messages': 1, 'tool_calls': 2, 'tool_results': 1,
          'compaction_boundaries': 1, 'turns': 2, 'bytes': 2168}
non-zero loss rows: system_records ×1, tool_result_sources ×1, tool_name_vocabulary ×2 (carried-with-caveat),
          unresolved_source_parents ×2 (carried-with-caveat)
```

The same run's human block (no `--json`), which is the other surface D2 asks for:

```
claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183 → codex
  source:       /tmp/ccc-0009-ev2/src/claude/-home-user-code-awaiting/c1a74e28-3b0d-4f91-8d27-6e5b90c4a183.jsonl
  read from:    source
  conversation: 11 message(s), 4 user, 7 assistant, 3 tool call(s), 1 compaction boundary(ies)
  boundary:     2902 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2902: the file ends at a record boundary
  selection:    5-10: the read's own ordinals are 0-10 (11 record(s))
  selected:     6 record(s) at ordinals 5-10
  excluded:     5 record(s) at ordinals 0-4
  necessities:  0 record(s) added: none
  undischarged: 1 carried call(s) with no recorded outcome at ordinals 5
  converted:    codex-rollout-jsonl, 2168 bytes: 2 message(s), 2 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  omitted:      system_records ×1
  omitted:      tool_result_sources ×1
  carried-with-caveat: tool_name_vocabulary ×2
  carried-with-caveat: unresolved_source_parents ×2
  installation: dry run, nothing written to /tmp/ccc-0009-ev2/codex-home
  source file:  unchanged (sha256 7d3f25bd09b9b5518ba90641bf23b164df1664de623ea4d06526967877306431, taken before and after the conversion)
  target id:    01a1f4d4-6cc5-747b-972e-2aa4102fd9ef (proposed; nothing was installed)
  working dir:  /home/user/code/awaiting
  resume:       CODEX_HOME=/tmp/ccc-0009-ev2/codex-home codex resume 01a1f4d4-6cc5-747b-972e-2aa4102fd9ef (proposed; nothing was installed)
  non-interactive resume: CODEX_HOME=/tmp/ccc-0009-ev2/codex-home codex exec resume 01a1f4d4-6cc5-747b-972e-2aa4102fd9ef '<prompt>' (proposed; nothing was installed)
  cli_version:  0.170.0 (probe)
  structure:    verified (the inherited AgentsView Codex reader over a throw-away copy (dry run))
  native load:  not-verified (the target harness's own loader (codex app-server, thread/resume))
                a dry run installs nothing, so the target harness has nothing to load
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                not requested (--verify load)
  warning:      the conversion carries a selection of the read: the specification "5-10" selected 6 record(s) at ordinals 5-10, 5 record(s) are excluded, and 0 record(s) were added as a structural necessity (a recorded call and its recorded result are carried together)
  warning:      1 carried call(s) at ordinals 5 have no recorded result that states an outcome anywhere in the read (the source itself left them undischarged); whether a target can hold a call with no recorded outcome is that target's own writer's answer, and the conversion's losses name what the writer left out
  warning:      a dry run does not start the target harness, so the conversion records no model provider; a run that installs asks the harness for its own answer
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The arithmetic D2 asks the counts to satisfy: `included.count + necessities_added.count + excluded.count`
= `6 + 0 + 5` = `11` = the read's own record count, and the same relation holds for every selection this
increment's Go cases run (`TestSelectionReportsClassesThatReconcileWithTheRead`). The classes name records
(and, for `calls_undischarged`, calls) in the same vocabulary the conversion's own loss rows use, and the
non-zero rows above are the conversion's, not the selection's.

## 3. The refusals: the core's own sentence, the usage code, nothing written

The target home held exactly one rollout before these runs (the one §1 installed) and exactly one after;
the source copy's `sha256` is `7d3f25bd…` before and after all of them.

```
$ CLAUDE_PROJECTS_DIR=… bin/ccc continue claude:c1a74e28-… --to codex \
      --target-home /tmp/ccc-0009-ev2/codex-home --select 5-3
ccc: the selection cannot be honoured: the region "5-3" runs backwards: a region's first ordinal has to
be at or below its last
exit=2

$ … --select 42
ccc: the selection cannot be honoured: the selection names ordinal 42, which the read's own ordinals do
not hold: the read returned 11 record(s) at ordinals 0-10, and a specification names values from that space
exit=2

$ … --select ""            # a supplied blank value, the review's finding: not the flag's absence
ccc: the selection cannot be honoured: the specification is empty: name the ordinal values to carry, as
regions like 0-5,9,12-
exit=2

$ … --select 2 --strict-selection
ccc: the selection separates a recorded tool call from its recorded result: the call
toolu_09Selection0000000000000001 recorded at ordinal 2 is selected and its recorded result at ordinal 3
is not, so the conversion would carry a call whose recorded output stays behind; name a region that holds
both records, or drop --strict-selection to carry the counterpart as a necessity
exit=2
```

(The strict run is the one that matters for "before anything is installed": the same command line without
the flag *does* install — §1 — and with it nothing was published: the home's `sessions/` tree still holds
the one rollout §1 wrote, and the run printed no report on stdout. The blank-value run is the review's
second finding and is refused with the same discipline; all four runs above are installing runs, and none
of them published a second rollout. The home's other files are the
harness's own bookkeeping, which its loader wrote when §1 opened the session; the source copy's bytes are
unchanged.) The same refusals, plus the duplicates and the "selects nothing" arm over a gappy read and the
blank value's grammar half, are the Go cases named in [`0009-citations.tsv`](0009-citations.tsv).

## 4. D4: the unflagged run, before and after

A throw-away probe (`internal/ccc/zz_d4_probe_test.go`; its exact revision, byte-for-byte the one that
printed these lines, is retained as [`0009-d4-probe.txt`](0009-d4-probe.txt)) walks **every committed
fixture document** — the eleven the pre-0009 tree and this tree share (two Claude transcripts, two Codex
rollouts, three Pi sessions, three Oh My Pi sessions, one OpenCode document) *and* the increment's new
Claude transcript — and for each document × each of the five routes it does two things:

* the **writer arm** (`route.Convert` with fixed options): the sha256 of the written bytes over a
  normalized copy (minted uuids and entry ids, the routes' `msg_`/`ses_` spellings, timestamps, the
  scratch and checkout paths folded), the carried counts, and every non-zero loss row;
* the **pipeline arm** (`Continue`, dry run, `--verify none`, the selection flag absent): the conversion
  block and its rows, the installation, the target session, the verification states, the source counts and
  the warnings, each folded the same way. The OpenCode *target*'s arm is included, unlike the earlier
  probe's: its dry run still installs into a scratch store through the harness's own importer, and the
  arm's lines record what the installed harness answered or refused (the run's error travels as its own
  `error=` line).

Exactly one probe revision produced every run below — the retained `.txt` is that revision — and each tree
was run twice. The pre-0009 tree is a throw-away worktree with the identical probe file copied in:

```console
$ git worktree add --detach /tmp/ccc-d4-before 9242e49b2d38aa8474f530c46e26fc3ee3b7a226
$ cp internal/ccc/zz_d4_probe_test.go /tmp/ccc-d4-before/internal/ccc/
$ cd /tmp/ccc-d4-before
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-before1/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-before2/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ cd -
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-after1/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-after2/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ diff /tmp/ccc-d4-before1/probe.txt /tmp/ccc-d4-before2/probe.txt   # silent: one tree's runs agree
$ diff /tmp/ccc-d4-after1/probe.txt  /tmp/ccc-d4-after2/probe.txt    # silent
$ grep -v 'claude:c1a74e28' /tmp/ccc-d4-before1/probe.txt > /tmp/ccc-d4-before-shared.txt
$ grep -v 'claude:c1a74e28' /tmp/ccc-d4-after1/probe.txt  > /tmp/ccc-d4-after-shared.txt
$ wc -l /tmp/ccc-d4-before1/probe.txt /tmp/ccc-d4-after1/probe.txt
 987 /tmp/ccc-d4-before1/probe.txt
1108 /tmp/ccc-d4-after1/probe.txt
$ diff /tmp/ccc-d4-before-shared.txt /tmp/ccc-d4-after-shared.txt   # silent
$ grep -c 'claude:c1a74e28' /tmp/ccc-d4-after1/probe.txt
121
```

The line counts are what the probe's own arms produce: the after tree's 1108 = 60 `PROBE-WRITE` (5 routes
× 12 documents) + 384 `PROBE-LOSS` + 664 `PROBE-RUN`; the pre-0009 tree's 987 = 55 + 341 + 591 (5 routes ×
11 documents), and the increment's new fixture accounts for the 121-line difference. Both stability diffs
are silent, so the folded digests are a statement about the conversion rather than about the ids it mints;
the shared-document diff is silent over all **987 lines** — every route's written bytes, every loss row,
every report field and both OpenCode-target arms (writer and pipeline) included — so no route's conversion
moved for a run without the flag on any document the two revisions share. The four fixtures the earlier
probe had left out and the OpenCode target's arm are covered explicitly:

```console
$ for pat in 'PROBE-WRITE pi:01a1c336-' 'PROBE-WRITE pi:01a1c337-' 'PROBE-WRITE omp:01a1c339-' \
      'PROBE-WRITE omp:01a1c33a-'; do
      diff <(grep -m1 "$pat" /tmp/ccc-d4-before-shared.txt) \
           <(grep -m1 "$pat" /tmp/ccc-d4-after-shared.txt); done                 # silent
$ diff <(grep 'claude:9f2c1d47-.* opencode' /tmp/ccc-d4-before-shared.txt) \
       <(grep 'claude:9f2c1d47-.* opencode' /tmp/ccc-d4-after-shared.txt)       # silent
$ grep -m1 'PROBE-WRITE pi:01a1c336-' /tmp/ccc-d4-after1/probe.txt
PROBE-WRITE pi:01a1c336-4b02-78d3-9e15-6c7a2f0b8d41 claude bytes=2895 digest=5c01641185f9a9b50968f4e9e2c931bd85b64fd49cb3cfe2f7f8b541065dde2e carried={Messages:5 UserMessages:2 AssistantMessages:3 ToolCalls:0 ToolResults:0 CompactionBoundaries:1 Turns:0 Bytes:2895}
$ grep 'claude:9f2c1d47-.* opencode' /tmp/ccc-d4-after1/probe.txt | head -2
PROBE-RUN claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 opencode conversion=converted format=opencode-import-document read_from=source dry_run=true carried={Messages:7 UserMessages:2 AssistantMessages:4 ToolCalls:1 ToolResults:1 CompactionBoundaries:1 Turns:2 Bytes:2131}
PROBE-RUN claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 opencode error=verifying the conversion: the harness's own importer could not be run: exit status 1: LocationNotFoundError: Location not found: /home/user/code/example-api at k (/$bunfs/root/chunk-hn7ztr6r.js:13:34510) … (definition) (/$bunfs/root/chunk-4wxrr8xe.…
```

(The OpenCode target's pipeline arm is deterministic here in the same way on both trees: the *real*
harness refuses a source whose recorded directory does not exist on this host, and the arm records that
refusal as its own `error=` line rather than hiding it — the `…` in the last line is the record's own cut
of a long stack, the step's answer is unedited. The four previously-uncovered fixtures' writer lines and
the OpenCode-target greps are byte-identical before and after.)

The flag-absent comparison cannot discharge D4 by itself and is not asked to: what the selection *does*
when it is present is D1/D2's evidence (§1–§2 and the Go cases), and the flag-present-versus-flag-absent
comparison is a different claim (pinned in `cmd/ccc`'s case: a specification naming every record of the
read converts exactly what the unflagged run converts).

## 5. The selection, clause by clause

| Source evidence (the fixture's own bytes) | What the selection did | Reported as |
| --- | --- | --- |
| the read's own ordinal values (0..10) | the specification is read against them, never against positions; `42` and `0-`-style values outside the space are refused naming the space | `selection.ordinal_space` `{min 0, max 10, records 11, contiguous true}`; `TestSelectionMatchesTheOrdinalValuesTheReadSupplied` |
| `--select 2` (the call record) | carried the call **and** the result record at ordinal 3, named as a necessity with its puller | `included 1 @ ["2"]`, `necessities_added 1 [{ordinal 3, pulled_by 2}]`; §1's rollout records 2 and 3 |
| `--select 3` (the result record) | carried the result **and** the call record at ordinal 2 (the other direction) | `TestSelectionCarriesTheCounterpartOfASeparatedCallAndResult` |
| a reused `ToolUseID`: a call at 0 answered at 1, a later call at 2 reusing the id | the later call has no recorded result of its own: it carries nothing but itself and is counted, not expanded by id alone; either half of the older pair still pairs in both directions | `TestSelectionPairsAReusedToolIDByRecordedOrder` (and `internal/parser/termination_test.go`'s own regression for the same shape) |
| `--select 2 --strict-selection` | refused by name, both ordinals and the call id named, nothing published | §3's fourth block; `TestSelectionRefusesASeparatedPairUnderStrictSelection` |
| the read call at ordinal 5, which the source never answered | carried as the call it is, with no outcome invented for it | `calls_undischarged {count 1, regions ["5"]}`; §2's human line |
| a call whose read holds only progress events (`running`/`pending`/…, the Codex subagent notification) | not read as answered: the progress is not an outcome, so the call is counted undischarged and its event still travels with it | `TestSelectionCountsACallWithOnlyProgressEventsUndischarged` |
| the compaction summary at ordinal 6 and the system record at ordinal 8 (whose tool result answers the cap call at 7) | carried inside a window that names them, boundary and system record included; the record's text is the target's own `system_records` row, its tool result travels | §2's carried counts (`compaction_boundaries 1`, `tool_results 1`) and its loss rows |
| every record the specification and the necessity did not name | stayed behind: their own words and call ids are absent from the written document | §1's table; `TestSelectionWritesExactlyTheSelectedOrdinals` (five routes, present/absent evidence per case) |
| a sparse window on the Pi history fixture: `--select 0,3,5` keeps the branch record at 3 and the active leaf at 5, leaving 5's recorded parent at 1 out | the leaf and the branch record both hang off their nearest retained ancestor (ordinal 0), so the walk from the file's last entry reaches ordinals 5 and 0 only; the branch record stays in the file as recorded history, off the walk. Measured on the written Pi file (`--to pi --verify none`): entries `6381b178` (ordinal 0) → `70bce9ed` (ordinal 3, the branch) and `a446a8b5` (ordinal 5, the leaf), both under `6381b178`, so the leaf's `parentId` chain is `a446a8b5 → 6381b178` | `TestSelectionKeepsASparseWindowOnTheReadsLine`; §4's probe covers the unflagged path |
| a window whose records have no retained ancestor at all (`--select 3,5` on the same fixture) | refused by name before anything is installed: the fallback would hang the leaf under the off-line branch record's entry | `ccc: … 5566cc77 (ordinal 5) … would be chained to the entry of 0011aa22 (ordinal 3), which that line leaves off it …` (exit 1, `ErrUnrepresentableLeaf`); `TestSelectionKeepsASparseWindowOnTheReadsLine` |
| a value supplied blank (`--select ""`) | refused as a request that names nothing, never read as the flag's absence: an automation's empty variable must not publish the whole history | §3's third block; `TestCheckSelectionOptionsRefusesASpecificationWithoutARead`, `TestContinueRefusesABadSelectionBeforeInstalling` |
| the source file itself | read whole, measured before and after | `source_check_state: unchanged` with the fixture copy's `sha256`, §1 |
| a call the source left unanswered, at a target whose format has no state for it (OpenCode) | the route leaves the call out and counts it — its own limit, named by its own report rather than claimed for every target | `unpaired_tool_calls ×1`; `TestSelectionWritesExactlyTheSelectedOrdinals`'s `TargetOpenCode` arm and `TestContinueReportsUndischargedCallsOnBothSurfaces` |
| a run without `--select` | the same conversion it was, byte for byte | §4's silent diffs over all 987 shared-document lines |
| a source whose reader answers a call and its result events on the *same* record (the committed OpenCode document) | the record is carried whole with no necessity and nothing undischarged — the document already holds that structure in one unit | `TestSelectionReportsClassesThatReconcileWithTheRead`'s second arm (1 call + 2 result events on the carried record, classes partitioning the read) |

## 6. Gate, tests and the citations manifest

The gate is `python3 scripts/gate.py`, run alone (never beside a native check on this host), and it must
print `GATE: PASS`:

```console
$ python3 scripts/gate.py; echo "exit=$?"
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.740s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 19 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

(`parity`'s detail is elided in this transcript with `…` — the step prints it as one long line, and the
numbers this record quotes are the ones it holds. The transcript is the run of this script on the tree this
fix stage leaves, taken after the throw-away probe file was removed; `selftest`'s duration is that run's
own. `0009-citations.tsv`'s row count moved 15 → 19: the round's four new rows (`S6`, `S7`, `S2-reuse`,
`D5-blank`) each cite an existing-or-new test that exists and passes. `secrets` moved 195 → 196 files
because the retained probe recipe (`0009-d4-probe.txt`) is one more file this repository owns. `gofmt`,
`test` and `parity` hold the counts of the previous run.)

`docs/evidence/0009-citations.tsv` holds one row per obligation a gate-runnable Go test discharges, in
this increment's own id spaces:

* `D1`–`D5` — the goal's five `Done when` clauses (D4 has **no row**: its evidence is §4's throw-away
  before/after probe, which no gate step runs — the form-based carve-out 0007 and 0008's manifests state;
  the reason is in the manifest's header comment; `D5-blank` is the supplied-blank refusal, one arm of
  D5's refusal shapes);
* `B1`–`B2` — the goal's two `Bounds` clauses;
* `S1`–`S7` — the rules of `AGENTS.md`'s selection paragraph, in the paragraph's own order (opt-in and the
  untouched default; the recorded-link seam with the counterpart carried and named — `S2-reuse` adds the
  reused-id half; the refusals with the core's sentence and the usage code before any install; the report's
  classes and the read's own ordinal space; the line rule, with `S6` for its ancestry half and `S7` for a
  call whose only recorded results state progress).

The native half of D1 ("the target is natively loadable") is §1's `verification.load: verified` — Codex's
own app server, quoted with the harness's version — and is not a gate step, so no row holds it.

### The review round's findings (`d8e211f` + working tree), fixed and re-measured

The review of `9242e49..d8e211f` found five items (one behavioural defect and four untrue statements); the
first fix round fixed all five on `d8e211f`'s tree, and the transcript below is that round's own gate run
(the tree the current round leaves is the one §6 quotes).

**R-01 — `--select` silently disabled the leaf/inactive guard (P1), fixed.** Both guards walked the
*reduced* slice: `selectedLeafRefusal` and the line writers' `inactiveSourceRecords` derived "the line" from
the window, so a window ending on a record the *read's* walk places off its line wrote a target session that
resumes the abandoned branch -- no refusal, no count. The fix makes the line a property of the read: `Select`
answers the read's own walk before reducing and carries it per kept record
(`Conversation.sourceLineInactive`), and the writers consult it (`sourceLineAnswers`; the leaf guard is
`selectedLeafRefusal(conversation, …)` now) instead of re-deriving one from the window; a line writer's
`unresolved_source_parents` count is taken over the records this conversion writes. Measured on this host on
the Pi history fixture (`01a1c336-4b02-78d3-9e15-6c7a2f0b8d41`, whose file's last entry *is* the read's last
record -- so the source-side leaf check passes -- while ordinals 2-4 are the branch the read's line leaves):

```console
$ … continue pi:01a1c336-… --to codex --select 2-4 --dry-run --json
carried {messages 0, …} ; inactive_branch_records 3 ; the abandoned texts ("And the retry cap?",
"Thirty seconds, in internal/retry/config.go.") are absent from the rollout
$ … continue pi:01a1c336-… --to pi --select 2-4 --dry-run
ccc: the source's own loader selects a leaf this conversion cannot write as the target's: 3344bb55, the
record its last entry stands for, is off the line the read's last record answers, so a resumed session
would continue a branch the source's conversation left     (exit=1 -- the leaf guard's own conversion code,
nothing installed; the refusal is `ErrUnrepresentableLeaf`, not a selection-refusal sentence)
$ … continue pi:01a1c336-… --to pi --select 0-1 --dry-run     # a window ending on the read's line
  converted:    pi-session-jsonl, 799 bytes: 2 message(s), …  (converted; the same window converts on codex)
```

The new case `TestSelectionEndingOffTheReadsLine` pins all of it (three tree routes refuse, the line route
omits and counts, a window ending on the line converts); run against the pre-fix tree it fails exactly as
the review demonstrated:

```console
$ git stash push -- <the seven fixed files>; go test ./internal/ccc/ -run TestSelectionEndingOffTheReadsLine
--- FAIL: TestSelectionEndingOffTheReadsLine
    selection_test.go:435: pi: converting a window ending off the read's line: err = <nil>, want ErrUnrepresentableLeaf
    selection_test.go:435: omp: … err = <nil>, want ErrUnrepresentableLeaf
    selection_test.go:435: claude: … err = <nil>, want ErrUnrepresentableLeaf
    selection_test.go:451: codex: inactive_branch_records = 0, want the branch's three off-line records (ordinals 2-4)
    selection_test.go:463: codex: the written document holds "And the retry cap?" …
    selection_test.go:463: codex: the written document holds "Thirty seconds, in internal/retry/config.go." …
$ git stash pop
```

The runs §1-§2 quote do not move: the same CLI runs through the pre-fix and post-fix binaries over a copy of
the Claude fixture print the same carried counts and the same non-zero loss rows for `--select 2`, `5-10`
and `0-10` (`jq -c '{c:.conversion.carried,l:[.conversion.losses[]|select(.count!=0)]}'`, `pre == post` for
each). `AGENTS.md`'s selection bullet gains the rule sentence ("**The line is the read's own** …"), and the
citations manifest gains its row (`S5`).

**R-02 — the CLI case named "a specification that selects nothing" pinned nothing (P2), fixed.** The case's
args were overwritten by a duplicate-region spec (`9-10,10`, already covered by the `1-3,3` case) and its
`want` was `""`, which `strings.Contains(stderr, "")` always satisfies. The case and the override are gone;
the comment states why the arm is unreachable from this fixture and where it *is* pinned (the gappy-read Go
case).

**R-03 — the refusal criterion said "an ordinal the read does not hold" (P3), fixed.** `AGENTS.md`,
`docs/conventions.md`, `README.md` and `cmd/ccc/main.go` (the `--select` help, the exit-code paragraph and
the exit-code mapping comment) now state the implemented rule: a region reaching outside the ordinals the
read supplied (below its lowest or above its highest), a spec that would carry no record, and the strict
refusal -- with "a region spanning a gap carries the records it holds" spelled beside it.

**R-04 — "before the target harness is asked anything" was false (P3), fixed.** `AGENTS.md` and the same
claim in `internal/ccc/continue.go`'s seam comment now say the harness's own *answers* are asked for after
the step, while its executable's version is probed earlier, before the read.

**R-05 — the B2 citation named a disposition the case did not assert (P3), fixed.** The cited case now
asserts both halves of the system record's disposition: its `<system-reminder>` text
(`the workspace state was restored`, measured as `dialogueFromContent` -- the record's own bytes, so writing
it would present it as the conversation's own turn) is absent from every route's document, and each route
counts the record (`system_records ×1`) while its tool evidence travels. The row's wording now says exactly
that.

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py; echo "exit=$?"
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.773s (rc=0)
  PASS     parity   (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input; 32 project-owned prefix(es); 5726 file(s) seen by git -- the step's
                    detail is rendered at length, this line keeps §6's spelling)
  PASS     citations docs/evidence/0002-citations.tsv: 36 | docs/evidence/0007-citations.tsv: 37 |
                    docs/evidence/0008-citations.tsv: 13 | docs/evidence/0009-citations.tsv: 15 row(s) verified
  PASS     secrets  no leaks found in 195 file(s) this repository owns
GATE: PASS
exit=0
```

The `0009-citations.tsv` count is 14 → 15 (the new `S5` row, its cited case existing and passing); `gofmt`,
`selftest` and the other steps hold §6's numbers -- the fix adds no file and no new selftest case, because
the two changed Go cases live in files the increment already carries. `docs/decisions/OPEN-QUESTIONS.md`
gains `Q-0009-04` (the read's line is not the selection's to redefine: the default this fix takes, and the
reversal if the owner wants a selection to be able to choose the resumed line). Bookkeeping: nothing to
archive -- 0009 is implemented and awaiting operator acceptance.

### The second review round's findings (`fc05ac4`), fixed and re-measured

The review of `9242e49..fc05ac4` found six items (two P1, three P2, one P3) and this round fixes all six on
`fc05ac4`'s tree; the gate §6 quotes is this tree's own run.

**R-06 — a sparse selection could reactivate an abandoned branch (P1), fixed.** `--select 0,3,5` on the
Pi history fixture kept the off-line branch record (ordinal 3) and the on-line leaf (5) while excluding 5's
recorded parent (1): the writer's fallback chained the leaf to the entry written before it -- ordinal 3's --
and the leaf-only guard accepted it, so the target's loader would walk `5 → 3 → 0` and revive the abandoned
retry-cap question as active history. The fix answers the *nearest retained ancestor* before the fallback:
`Select` walks the read's own relations through the records the window left out and carries, per kept
record, the nearest ancestor the window also carries (`Conversation.sourceKeptAncestor`); the tree writers
hang an entry off that ancestor's place (`parentPlace`, `keptAncestorEntry`); and a window where the
retained ancestry is gone entirely -- so the fallback would land under an off-line record -- is refused by
name (`ErrUnrepresentableLeaf`). The revival check is gated to reduced documents, so an unflagged
conversion keeps the writers' long-standing answer for a relation the read cannot place and §4's
before/after probe stays silent. Measured on the fixture (`--to pi --verify none --select 0,3,5`, exit 0):
the written entries are `6381b178` (ordinal 0), `70bce9ed` (ordinal 3, the branch) and `a446a8b5`
(ordinal 5, the leaf), the branch and the leaf both under `6381b178`, so the leaf's `parentId` chain is
`a446a8b5 → 6381b178` and the branch record stays in the file as recorded history, off the walk;
`--select 3,5` (nothing retained to hang the leaf on) is refused:

```
ccc: the source's own loader selects a leaf this conversion cannot write as the target's: 5566cc77
(ordinal 5), a record on the line the read's last record answers, would be chained to the entry of
0011aa22 (ordinal 3), which that line leaves off it: the relation is not in the document this conversion
writes (the source named a record the read does not hold, or a selection left the record's ancestry out),
so following the entry written before it would put an abandoned record on the walk a resumed session makes
(exit=1)
```

`TestSelectionKeepsASparseWindowOnTheReadsLine` pins both halves by walking the written `parentId` chain
(the walk covers the two on-line records alone; the branch record is in the file and off the walk), and the
allowed window `0,5` converts on all three tree routes.

**R-07 — a supplied blank `--select` published the whole session (P1), fixed.** The CLI could not tell
`--select ""` from the flag's absence through the value alone, so `--select "$SPEC"` with an unset variable
carried the whole read and printed no selection block -- and the earlier CLI case expected that bypass. The
CLI now reads `flag.Visit` (`runContinue`), `ContinueOptions.SelectionSupplied` carries the answer into the
core, and `CheckSelectionOptions(selection, strict, supplied)` refuses a supplied blank with the same
sentence `Select` answers a blank spec with (`emptySelectionRefusal`), exit 2, before the read and before
anything is installed. Measured through the CLI with an installation requested: §3's blank-value block
(exit 2, nothing published); `TestContinueRefusesABadSelectionBeforeInstalling` pins it (home empty, source
file's bytes unchanged), `TestCheckSelectionOptionsRefusesASpecificationWithoutARead` pins the grammar half,
and `TestContinueWithoutASelectionReportsNoSelection` asserts the refusal on the run that used to expect the
bypass. `Q-0009-03` is rewritten to the new default.

**R-08 — a reused tool id pulled unrelated history in and strict mode mislabelled a call as a result
(P2), fixed.** The closure grouped every occurrence of an id, so selecting the later call of the inherited
readers' own regression (`internal/parser/termination_test.go`: call at 0, result at 1, a later call at 2
reusing the id) imported the older pair, and strict mode named ordinal 0 -- another call -- as the selected
call's recorded result. Pairing now comes from the read's own order (`selectionPairs`): a result pairs with
the nearest call occurrence the same id records *before* it, the reading `hasOrphanedToolCall` takes; the
later call has no pair and carries nothing but itself (counted undischarged), the older pair still pairs in
both directions, and strict refuses either half of the older pair by name.
`TestSelectionPairsAReusedToolIDByRecordedOrder` pins all of it (plus the two-calls-one-later-result arm,
which the result pairs with the later call), and the `S2-reuse` citation row names it.

**R-09 — the undischarged-call report disagreed with the source and the target (P2), fixed.** A call whose
only recorded result is progress (`running`/`pending`/`streaming`/`started`/`in_progress`, the words the
inherited readers write for a streamed chunk) was read as answered; now `undischargedCalls` discharges a
call only where a result that states an *outcome* follows it (`resultStatesAnOutcome`, the predicate the
OpenCode writer already used, moved to `route_shared.go` and shared). In the other direction, the old
warning claimed the target records the call; it now says the target's own writer decides and the
conversion's losses name what it left out, and the human `undischarged:` line says "no recorded outcome".
`TestSelectionCountsACallWithOnlyProgressEventsUndischarged` pins the source side (progress statuses count,
outcome statuses do not), `TestContinueReportsUndischargedCallsOnBothSurfaces` pins the nonzero count on
both report surfaces and the OpenCode route's own `unpaired_tool_calls ×1` beside it (`--select 5-10`
installed through the stub binary; the window is used because a document whose selection writes no message
at all is a shape that route's own store cannot be read back from -- measured while writing the case).
`Q-0009-02`'s default now states both pairing halves.

**R-10 — the D4 evidence overstated its fixture and route coverage (P2), fixed.** §4 is rewritten around a
new probe (its exact source retained as [`0009-d4-probe.txt`](0009-d4-probe.txt)) that walks *every*
committed fixture -- the eleven the pre-0009 tree shares and the increment's new one -- through the five
routes' writers *and* the whole pipeline, the OpenCode-target arm included. Measured: 987 shared-document
lines with a silent before/after diff, 1108 lines on this tree, 121 lines for the fixture no earlier
revision holds, and silent re-run diffs per tree (§4's transcript). The earlier record's "seven documents /
588 lines / the OpenCode arm is covered by §§1–3" claims are gone; the new text states the new fixture's
coverage gap (no before-run can exist for a document no revision read).

**R-11 — the README's selection example was out of range for its fixture (P3), fixed.** The example line
now reads `--select 0-3,7` (the named Claude fixture reads as eight records, ordinals 0..7, with its
call/result pair inside 0-3), and the paragraph beside it names the order-based pairing, the
progress-versus-outcome rule, the supplied-blank refusal and the ancestry rule.

## 7. Docs, rules and bookkeeping

* `AGENTS.md` — a new binding bullet ("A conversion carries only what the caller selected …") outside the
  per-harness bullets, and `internal/ccc`'s file list in "How to work here" names `selection.go`. The
  control file moves because this increment changes a rule an agent must follow and the core's layout. The
  second fix round rewrote the bullet's seam sentence (pairing by the read's own order), added the
  progress-is-not-an-outcome sentence, extended the line sentence with the ancestry rule, and added the
  supplied-blank refusal to the refusal list.
* `docs/conventions.md` — the layout block names `selection.go`; a new rule under "Rules of the work"
  states the specification's grammar, the seam, the refusals and the report's classes, with the second
  round's refinements (order pairing, progress, ancestry, supplied blank) folded into the same rule.
* `docs/plans/0009-selective-context.md` — §11 records nine departures with the measurement behind each.
* `docs/goals/README.md` — 0009's row reads *Implemented on `feat/0009-selective-context`; awaiting
  operator acceptance*, and the goal file's own `Status:` line reads the same (the form 0007 and 0008
  carried while their increments awaited acceptance); nothing else was ready to archive (0008 was archived
  by the activation increment that landed this plan, and 0010–0013 are provisional while 0014–0017 are
  ready-but-unstarted).
* `docs/decisions/OPEN-QUESTIONS.md` — Q-0009-01 (the strict refusal's exit code); Q-0009-02 (rewritten:
  the seam pairs a result with the nearest prior call of the same id, an id-less record carries no link);
  Q-0009-03 (rewritten: a supplied blank value is refused, not read as the flag's absence); Q-0009-04 (the
  read's line is not the selection's to redefine, ancestry included).
* `README.md` — an example line (`--select 0-3,7`, in range for its fixture), a paragraph on the flag and
  its report, and `Select` in the engine list.
* `docs/harness.md` — unchanged: this increment adds no gate step and no knob, and the citations step it
  already requires is the one that reads the new manifest.
* The throw-away probe file (§4) and the pre-change worktree were removed before the gate run; the probe's
  exact source is retained beside this record (`0009-d4-probe.txt`). The
  fixture generator (`internal/ccc/fixturegen`) and the committed fixture tree move together, pinned by
  the existing `TestFixturesMatchTheirGenerator`.
