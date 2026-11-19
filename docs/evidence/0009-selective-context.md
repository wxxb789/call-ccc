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
`…` marks only a line the record's own layout cut, and nothing else was edited. **Redactions:** every
excerpt that would spell a host-specific path — this checkout's root, the Go and Codex toolchain install
paths — carries a labeled placeholder in its place and is marked *redacted*: `docs/conventions.md`'s
fixture rule forbids absolute host paths in committed evidence, and the fourth fix round redacted the
third round's excerpts (the Codex binary path in §1, the gate transcripts' checkout and toolchain paths in
§6). Nothing else was changed in a quoted line.

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
| what an unflagged run writes | nothing the previous revision did not: a throw-away before/after probe over five routes × the eleven fixtures the two trees share prints the same 1024 lines and the same normalized bytes on HEAD's tree and on this tree — with source-recorded instants *and recorded identities* in the comparison (the normalizer folds only what a conversion mints or discovers, §4) — and the increment's new fixture adds its own 125 lines with no before-run (§4; the probe's exact source is `docs/evidence/0009-d4-probe.txt`) |

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
              binary <codex binary> (redacted: a host-specific toolchain path), binary_version 0.170.0
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

* the **writer arm** (`route.Convert` with fixed options): the length of the written bytes and the sha256
  of them over a normalized copy, the carried counts, and every non-zero loss row;
* the **pipeline arm** (`Continue`, `--verify none`, the selection flag absent, an *installing* run into a
  scratch home under a target binary no host has): the conversion block and its rows, the installation, the
  target session, the verification states, the source counts and the warnings, each folded the same way,
  *plus* two measurements the earlier probe revision left out — the sha256 of the **whole folded report**
  (every field of it, whether a line names the field or not) and the sha256 of the **bytes the installation
  published**, so the arm is compared on what it actually wrote and not only on what the writer handed the
  installer.

  The arm runs under `ccc-no-such-target-harness` rather than letting each route find this host's harness:
  what it compares is the conversion and its installation, and a host's installed harness would put the
  host's own answers into the comparison — measured on one tree, two runs: a Codex app-server handshake
  reported a timeout where its neighbour reported a closed stream, and the report digest moved by eleven
  bytes between the two runs although every printed field agreed. Under the absent binary every route's
  answer about the host is fixed text, the OpenCode target's arm records the refusal to run an importer as
  its own `error=` line (`artifact=none`, nothing published), and the harness's own answers — versions,
  model providers, importer behaviour — stay in the native sections and checks, where they are measured
  against the harness itself.

The normalized copy is folded with the identities **canonicalized** rather than replaced by a class-wide
placeholder: every distinct uuid, Pi entry id (in a JSON `id`/`parentId`/`firstKeptEntryId` position) and
OpenCode identity gets a token of its own — the same token for every occurrence of the same value,
numbered by first appearance over the whole folded text — so a definition and its references still agree
after folding while a *reparenting* from one non-null parent to another cannot fold to what it folded to
before. The earlier probe revision's blanket placeholders could not show that difference, which is what
the review's R-15 demonstrated. **Only identities a conversion *mints* are folded**: the identities the
read itself carries (`d4ProbeRecordedIDs`: the records' own identities, the tool ids of their calls and
results, the session's ids) are installed on the normalizer per document and left exactly as recorded, so
a change to a source-recorded identity is a bytes change the digest sees. The earlier revision folded
every `call_…`/`ses_…`/`msg_…` token by class shape and canonicalized every uuid, so a consistent
same-length rename of a recorded Codex `call_id` (in the call and its output) preserved both the written
length and the folded digest, which is what the review's fourth-round finding measured; the retained
revision's `recorded-call-id-change` and `recorded-uuid-change` controls pin the rule.

**Only what a conversion discovers is folded, and the fourth and fifth rounds narrowed that set.** The
rules are the
checkout's own root, scratch paths (`/tmp/…`), the source file's *filesystem* mtime
(`"file_modified":"…"`, a property of the checkout rather than of the source's recorded bytes), and the
canonicalization of **minted** identities only — the recorded ones the read carries are compared as
recorded (above). The rounds before the fourth also replaced **every** RFC 3339 instant and every
numeric `timestamp`/`created`/`updated` with a placeholder — which, as review R-24 measured, folded a
same-length change to a *source-recorded* time to the same digest: the probe could not establish D4's
byte-preservation claim for recorded instants. The retained revision drops those four rules, so recorded
instants and recorded identities are compared, and the probe itself now carries
`d4ProbeNormalizerControl`, whose five printed lines are the normalizer's own control:

```console
PROBE-CONTROL recorded-instant-change moves-digest=true
PROBE-CONTROL recorded-millisecond-change moves-digest=true
PROBE-CONTROL recorded-call-id-change moves-digest=true
PROBE-CONTROL recorded-uuid-change moves-digest=true
PROBE-CONTROL minted-identity-change moves-digest=false
```

A recorded RFC 3339 instant changed by one second, a recorded millisecond field changed by one, a
recorded call id renamed consistently in its call and its output, and a recorded uuid renamed everywhere
all move the folded digest; two values a conversion minted fold to the same token. The control runs in the
same process as the lines it vouches for, and a run in which any answer is wrong fails the probe (the
mutation this round records runs the rule's removal and prints `recorded-call-id-change` and
`recorded-uuid-change` as `false`). (The one rule that does remain load-bearing between checkouts is the
mtime fold: dropping it moves exactly the 45 `report` lines — nine shared documents × five routes — and no
writer or artifact line, so the fold is narrower than the blanket set it replaces.)

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
1024 /tmp/ccc-d4-before1/probe.txt
1149 /tmp/ccc-d4-after1/probe.txt
$ diff /tmp/ccc-d4-before-shared.txt /tmp/ccc-d4-after-shared.txt   # silent
$ grep -c 'claude:c1a74e28' /tmp/ccc-d4-after1/probe.txt
125
```

The line counts are what the probe's own arms produce: the after tree's 1149 = 60 `PROBE-WRITE` (5 routes
× 12 documents) + 384 `PROBE-LOSS` + 705 `PROBE-RUN`; the pre-0009 tree's 1024 = 55 + 341 + 628 (5 routes ×
11 documents), and the increment's new fixture accounts for the 125-line difference. Both stability diffs
are silent, so the folded digests are a statement about the conversion rather than about the ids it mints;
the shared-document diff is silent over all **1024 lines** — every route's written bytes, every loss row,
every report field, every digest of an installed artifact and both OpenCode-target arms (writer and
pipeline) included — so no route's conversion moved for a run without the flag on any document the two
revisions share, *with source-recorded instants and recorded identities in the comparison* (the round-3
and round-4 records' same claim rested on a normalizer that masked them; the `PROBE-CONTROL` lines above
are what makes this revision's claim stronger). The pipeline arm covers 50 of its 60 runs end to end; the
other ten are the branch-switch
fixtures (`pi:01a1c337-…`, `omp:01a1c33a-…`) on all five routes, whose arm stops at the source-side leaf
refusal before any writer runs, so their conversion block and `error=` line are compared like every other
line. Where the installation wrote the writer's own bytes the two digests agree, which is what makes the
artifact line a measurement of the installation rather than a restatement of the writer arm:

```console
$ grep 'pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi ' /tmp/ccc-d4-after1/probe.txt | grep 'PROBE-WRITE\|artifact'
PROBE-WRITE pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi written=2163 folded=2105 digest=471978ad873994d985d602968073088225ee221071c5897a932f543eb62b9f89 carried={Messages:4 UserMessages:1 AssistantMessages:3 ToolCalls:1 ToolResults:1 CompactionBoundaries:0 Turns:0 Bytes:2163}
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi artifact written=2163 folded=2105 digest=471978ad873994d985d602968073088225ee221071c5897a932f543eb62b9f89
$ grep -m1 'PROBE-WRITE pi:01a1c336-' /tmp/ccc-d4-after1/probe.txt
PROBE-WRITE pi:01a1c336-4b02-78d3-9e15-6c7a2f0b8d41 claude written=2895 folded=2172 digest=57c2890a5459281755a46737a166f8de36d44f51c53df4273bcb9cc08a54fc94 carried={Messages:5 UserMessages:2 AssistantMessages:3 ToolCalls:0 ToolResults:0 CompactionBoundaries:1 Turns:0 Bytes:2895}
$ grep 'pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode ' /tmp/ccc-d4-after1/probe.txt | grep -E 'error|artifact'
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode error=ccc-no-such-target-harness is not on PATH, and this target's installation is the harness's own importer: exec: "ccc-no-such-target-harness": executable file not found in $PATH
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode artifact=none installation=not-installed
```

(The digests above are not comparable to the previous probe revisions': the earliest folded every
identity to a class-wide placeholder, masked every instant, and digested no report and no artifact, and
the fourth round's — while comparing recorded instants — still folded every uuid and every
`call_…`/`ses_…`/`msg_…` token by shape, so this revision's folded lengths and digests differ wherever a
recorded identity stands in the measured text (`pi:01a1c335-…`'s Pi writer line is the example quoted:
its folded length moved 2051 → 2105 while the written bytes did not). What is comparable is this
revision's own before/after pair, which was produced by one probe revision on both trees.)

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
| a reused `ToolUseID` where the result's own recorded parent names the call it answers (call at 3, an inactive sibling at 4, the result at 5) | the read's own ancestry decides: the window carries the call at ordinal 3 rather than the later sibling occurrence, strict is honoured for the pair, the sibling's own call is counted undischarged, and every route writes the result under the call it answers — on the trees, both halves on the walk from the file's last entry — while the unflagged conversion keeps the call's output too | `TestSelectionPairsAResultByItsRecordedAncestry` |
| a window whose reduced document no longer holds the records the result's ancestry walk steps through (the sibling carried, the step excluded) | the pairing the read answered travels with the window: every route still writes the result under the call the read named, never under the nearer carried sibling the read's line leaves off | `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry` |
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
| a run without `--select` | the same conversion it was, byte for byte | §4's silent diffs over all 1024 shared-document lines, with source-recorded instants *and recorded identities* in the comparison (§4's `PROBE-CONTROL` lines) |
| a carried result whose paired call is not on its written ancestry (an inactive sibling reusing the id on a flat target; a call on another branch for a tree target) | omitted and counted on every route — on the flat targets the line leaves the call off, and on the trees the call is written but stands off the walk the target's loader makes from the result; a call the window carried still travels as recorded history, off that walk | `TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath`; `Q-0009-06` |
| an off-line record whose nearest retained ancestor is off-line too (a sparse window over a branch) | keeps that ancestor: the entry hangs where the source recorded it, and the branch stays off the walk from the file's last entry; a window whose *on-line* record would land under an off-line entry is still refused by name | `TestSelectionKeepsAnInactiveBranchesRecordedAncestry` |
| an id-less call in a carried document that holds no id-less result *that states an outcome* at or after it | counted `calls_undischarged` (no outcome in the line could be paired with it; an id-less progress event is not one), and the route whose format has no state for such a call counts `unpaired_tool_calls` beside it; where an id-less outcome result *is* carried the count asserts neither disposition and the writer's `generated_call_ids` names the pairing it inferred | `TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` |
| an archive call whose only outcome evidence is the summary on the call itself (`ToolCall.ResultContent`, no recorded result event) | not counted `calls_undischarged`: the source recorded the outcome in the archive's derived column, and the routes write that summary as the call's completed outcome; the same summary beside a progress-only event keeps the count, because the writers refuse to present progress as an outcome | `TestSelectionCountsASummaryOnlyCallAsAnswered` |
| a specification that names a region twice (`--select 1,1`) against a source that cannot be read | the specification's own refusal — the duplicate-region sentence and the usage exit code — because the check is the grammar's and needs no read; the control run without `--select` still reports the missing source | `TestCheckSelectionOptionsRefusesASpecificationWithoutARead`; `TestContinueRefusesADuplicateRegionWithoutAReadableSource` |
| a source whose reader answers a call and its result events on the *same* record (the committed OpenCode document) | the record is carried whole with no necessity and nothing undischarged — the document already holds that structure in one unit | `TestSelectionReportsClassesThatReconcileWithTheRead`'s second arm (1 call + 2 result events on the carried record, classes partitioning the read) |
| a call the reader projects into several rows sharing one id, with one recorded result (the committed Codex `apply_patch` fixture) | the rows are one call and the recorded result answers it: nothing is counted undischarged, and the writer coalesces the rows into the one call it writes | `calls_undischarged {count 0}`; the writer's own `coalesced_tool_call_projections ×1`, one carried call and one result; `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` |
| two calls reusing an id, both recorded before two outcomes | both outcomes pair with the nearest call recorded before them (the later one), so the earlier call is the one the source recorded no result for | `calls_undischarged {count 1, regions ["0"]}`; the closure carries nothing for it; `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` |
| the Pi fixture's record after a tool result, which the reader answers as naming the call while the file names the result | a window carrying the call and that record adds the result as a necessity *and* chains the record through it: the walk from the file's last entry holds the call, the result and the record — and the projection the chain crossed is counted, as the unflagged conversion of the same read counts it | written Pi `parentId` walk (`--select 1,4 --to pi`); `unresolved_source_parents` 2 on the 1,4 window (the excluded root plus the projection) and 1 on 0,1,4 and on the whole read; `TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall` |
| a record whose recorded parent the window left out while the read answered no identity for it (the Claude reader's `parentUuid` without `uuid`) | it hangs off its nearest retained ancestor — the answer travels by the record's own position — and a window with no retained ancestor is refused by name, the check keyed by position too; a record that names no parent at all goes through the same revival check, so a window whose previous entry stands for an off-line record is refused instead of reviving the branch | Claude `parentUuid` and the written Pi walk for `0,1,3`; `ErrUnrepresentableLeaf` for `1,3` and for the parentless `0,1,3`; `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor` |
| a command line whose selection the read's ordinals refuse *and* whose source file the leaf guard refuses | the specification's own refusal: the core's sentence, the usage exit code, nothing installed | `TestContinueValidatesASelectionBeforeTheSourceLeafGuard` (the same fixture without `--select` still refuses with the leaf error, exit 1); `Q-0009-05` |

## 6. Gate, tests and the citations manifest

The gate is `python3 scripts/gate.py`, run alone (never beside a native check on this host), and it must
print `GATE: PASS`:

```console
$ python3 scripts/gate.py; echo "exit=$?"
gate: <checkout> (redacted: this checkout's own root)
gate: go=<go toolchain> (redacted) targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.659s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 30 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

(`parity`'s detail is elided in this transcript with `…` — the step prints it as one long line, and the
numbers this record quotes are the ones it holds. The transcript is the fifth fix round's own run, taken
after the throw-away probe file was removed and after the round's evidence edits, and its two leading
lines are the redacted placeholders the record's header describes; `selftest`'s duration is that run's own.
`0009-citations.tsv`'s row count moved 27 → 30: the round's three new rows (`S2-ancestry`, `S2-path`,
`S2-summary`) each cite a test that exists and passes, and three rows' artifact text was rewritten to the
assertions their tests gained or lost. `secrets` holds 196 files and `gofmt`, `test`, `parity`
hold the counts of the previous run: this round adds no file — the probe recipe was already retained, and
every change landed in a file the increment already carries.)

`docs/evidence/0009-citations.tsv` holds one row per obligation a gate-runnable Go test discharges, in
this increment's own id spaces:

* `D1`–`D5` — the goal's five `Done when` clauses (D4 has **no row**: its evidence is §4's throw-away
  before/after probe, which no gate step runs — the form-based carve-out 0007 and 0008's manifests state;
  the reason is in the manifest's header comment; `D5-blank` is the supplied-blank refusal, one arm of
  D5's refusal shapes);
* `B1`–`B2` — the goal's two `Bounds` clauses;
* `S1`–`S7` — the rules of `AGENTS.md`'s selection paragraph, in the paragraph's own order (opt-in and the
  untouched default; the recorded-link seam with the counterpart carried and named — `S2-reuse` adds the
  reused-id half, `S2-ancestry` the recorded ancestry the pairing honors where it establishes the call
  and the carried pairing that travels with a window, `S2-occurrence` the writers' pairing of the
  recorded occurrence on both a flat line and a tree, `S2-path` the tree writers' written-ancestry check,
  `S2-summary` the archive's own summary as outcome evidence, and `S2-idless` the
  id-less call's own count; the refusals with the core's sentence and the usage code before any install —
  `D5-duplicate` adds the grammar's duplicate-region half; the report's
  classes and the read's own ordinal space; the line rule, with `S6` for its ancestry half, `S6-inactive`
  for the off-line record that keeps an off-line ancestor, `S6-identity` and `S6-projection` for the
  position-keyed answer and the counted projection, and `S7` for a
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
gate: <checkout> (redacted: this checkout's own root)
gate: go=<go toolchain> (redacted) targets=./cmd/ccc/... ./internal/ccc/...
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

### The third review round's findings (`7e61075`), fixed and re-measured

The review of `9242e49..7e61075` found six items (two P1, three P2, one P3): two behavioural defects in the
ancestry accounting, one in the undischarged accounting, one in the refusal order, one measurement
weakening and one test-coverage gap. This round fixes all six; every fix was measured against the pre-fix
tree first — the same tree with only the four fixed source files reverted and the new/extended tests kept,
so each test is shown to fail for the reason the finding names and pass after:

```console
$ git diff > /tmp/fix-round3.patch
$ git checkout -- internal/ccc/conversation.go internal/ccc/selection.go internal/ccc/route_shared.go internal/ccc/continue.go
$ go test -tags fts5 -count=1 -run 'TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall|TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor|TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers' ./internal/ccc/
--- FAIL: TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall (0.00s)
    selection_test.go:1012: the walk from the file's last entry does not hold "internal/retry/config.go:41: maxBackoffSeconds = 30", which the report names as carried (the walk holds ["The cap is 30 seconds of backoff between attempts." "I will look for the retry configuration."])
--- FAIL: TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor (0.00s)
    selection_test.go:1076: the record the read answered no identity for hangs off 74cec810-b931-4398-b262-12be1a2869c3, want its nearest retained ancestor's entry ("cbee500c-97aa-427f-b700-81663168cf09"): the identity it never had must not drop the answer and chain it to the entry written before it, which stands for the abandoned branch
    selection_test.go:1104: pi: the walk from the file's last entry holds the abandoned branch's record: the identity-less record was chained to the entry written before it instead of to its nearest retained ancestor
    selection_test.go:1118: converting a window with no retained ancestor: err = <nil>, want ErrUnrepresentableLeaf
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.00s)
    selection_test.go:1252: calls_undischarged = {Count:1 Regions:[1]}, want none: the two rows are one call (the writer's own distinctCalls unit) and the recorded result answers it
    selection_test.go:1283: calls_undischarged = {Count:0 Regions:[]}, want the earlier call at ordinal 0: the seam pairs both outcomes with the nearest call the id records before them (ordinal 1), so ordinal 0 is the call the source recorded no result for
FAIL
$ go test -tags fts5 -count=1 -run TestContinueValidatesASelectionBeforeTheSourceLeafGuard ./cmd/ccc/
--- FAIL: TestContinueValidatesASelectionBeforeTheSourceLeafGuard (0.01s)
    selection_route_test.go:287: exit 1, want 2 (stdout "", stderr "ccc: the source's own loader selects a leaf this conversion cannot write as the target's: the source's own file …/01a1c337-6d1e-7f84-9b2c-4e5f6a7b8c90.jsonl selects the leaf branch_summary entry e0718293, and that leaf's own path holds a branch_summary record (id e0718293, role -) which this read does not carry and whose kind this build cannot say the harness's own conversation context leaves out, so the conversation a resumed target would read is not the one the source's loader builds\n")
FAIL
$ git apply --include=internal/ccc/conversation.go --include=internal/ccc/selection.go --include=internal/ccc/route_shared.go --include=internal/ccc/continue.go /tmp/fix-round3.patch
$ go test -tags fts5 -count=1 -run 'TestSelectionKeepsACarriedResult|TestSelectionChainsARecord|TestSelectionCountsTheCallUnits|TestContinueValidates' ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.042s
ok  	go.kenn.io/agentsview/cmd/ccc	0.041s
```

(The failure transcript is the output as it was printed; the two uuids in the second case are the ones
that run minted, and the elided CLI path is this checkout's own fixture root.)

**R-12 — a sparse selection could put an answered tool call on the active path without its result (P1),
fixed.** `selectionKeptAncestors` walked only the read's raw relations, so on the committed Pi fixture
`01a1c335-…` — whose reader answers the record after a tool result as hanging off the message that made
the call, while the file runs call → result → what follows — a window naming the call and the record after
the result answered the *call* as the record's nearest retained ancestor: the written leaf walked
`4 → 1`, leaving the result the selection had carried as a necessity in the file and off the walk the
target's loader makes. The ancestry walk now takes the **tool-result step the writers take where the read
may project one** (`sourceToolResultBefore`, the same rule `sourceLineParent` and the tree writers use), so
the result is the ancestor the reduced tree chains the record through and the walk holds all three carried
records. Measured on the fixture (`--select 1,4 --to pi`): the walk from the file's last entry covers the
call's entry, the result's entry and the record that follows, where before the fix it held the call and the
record alone with the result's entry stranded. `TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall`
pins it, and the `S6-projection` row cites it.

**R-13 — a retained record without an identity could reactivate an abandoned branch (P1), fixed.** A
Claude read can answer a record's `parentUuid` with no `uuid` of its own (the inherited reader preserves
the two fields independently), and both the ancestry answer and the revival check were keyed by identity:
the answer was dropped in `newSourceTree` and never consulted, and the fallback's check skipped the empty
identity, so `--select 0,1,3` chained the record to the entry written before it — the abandoned branch's —
with no refusal. The ancestry answer is now carried and consulted **by the record's own position**:
`Conversation.sourceKeptAncestor` holds the reduced document's index of the nearest retained ancestor (or
-1), the tree stamps each record's position as it walks (`sourceTree.record`) and keeps its entry and
omitted place under that position (`entryByRecord`, `omittedEntryByRecord`), and the fallback's revival
check asks by position too, naming the record by its ordinal where it has no identity
(`recordDescription`). Measured on a synthetic read of that shape: the window `0,1,3` writes the record
under the root's entry (the Claude transcript's `parentUuid` and the written Pi session's walk both show
it), and the window `1,3` — nothing retained to hang it on — is refused by name (`ErrUnrepresentableLeaf`)
instead of accepted. `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor` pins both halves,
and the `S6-identity` row cites it.

**R-14 — `calls_undischarged` used a different call identity from the pairing and the writers (P2),
fixed.** The count treated every recorded call row as a call and consumed outcomes one per row, so the
committed Codex `apply_patch` fixture — one invocation the reader projects into two rows sharing the id,
with one recorded result — was reported as one undischarged call which the Claude writer coalesces into one
*answered* call, and the two-calls-two-outcomes shape (both calls recorded before both results) let the
second outcome discharge the earlier call the seam's own pairing never answers. The count is now derived
from the **same walk that pairs the seam** (`selectionPairing` answers both the closure's pairs and the
outcome-answered ids) and counted in the unit the writers write (`distinctCalls`: a message's calls
sharing a non-empty id are one projected invocation). Measured: the patch fixture's full read reports
`calls_undischarged 0` with the writer's own `coalesced_tool_call_projections ×1`, one carried call and
one result; the two-calls-two-outcomes read reports the earlier call alone at ordinal 0, and the closure
carries nothing for it. `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` pins both
shapes, and the `S2-units` row cites it.

**R-15 — the D4 probe could miss the ancestry regression it is meant to rule out (P2), fixed.** The
retained probe replaced every Pi `parentId`, entry `id` and uuid with one class-wide placeholder, so
reparenting an entry from one non-null parent to another folded to the same bytes, and its pipeline arm
neither fingerprinted the report it produced nor the artifact the run published. The probe now
*canonicalizes* identities — each distinct value gets a token of its own, the same token for every
occurrence, numbered by first appearance, so a definition and its references still agree while a
reparented reference cannot — and its pipeline arm is an **installing** run against a scratch home under a
target binary no host has, whose lines carry the sha256 of the whole folded report (every report field,
named or not) and of the bytes the installation published; for the OpenCode target, whose artifact is a
row inside the harness's own store, the line says so instead of digesting bytes that are not the record.
Measured (§4): both trees' two runs are byte-identical, the shared documents' 1024 lines are byte-identical
before and after, and the increment's new fixture adds its own 125 lines.

**R-16 — the Pi source-leaf guard could mask an out-of-range selection (P2), fixed.** `continueWith`
asked the source-side leaf guard before deciding the selection, so on the committed branch-switch fixture
`--select 99` was reported as the leaf refusal (exit 1) instead of the out-of-range sentence and the usage
exit code. The selection is now decided **first** — a specification the read's own ordinals refuse is a
statement about the caller's command line — and the leaf guard still runs against the *read*, before the
reduced document is handed on, exactly as before, so a valid specification still meets it. Measured through
the CLI: with `--select 99` the usage exit code and the space sentence; without a selection the same
fixture still refuses with the leaf error (exit 1), nothing installed either way.
`TestContinueValidatesASelectionBeforeTheSourceLeafGuard` pins both arms, and the `D5-order` row cites it;
`Q-0009-05` records the default and its reversal.

**R-17 — the D2 cases did not pin the excluded regions (P3), fixed.** The CLI case asserted the excluded
*count* and a human line that stopped before the region values, and the core reconciliation case accepted
any excluded region list (an empty one passed its presence-only loop). The CLI case now asserts
`["0-1","4-10"]` in `--json` and `excluded: 9 record(s) at ordinals 0-1,4-10` in the human block, and the
core case asserts that the excluded class's regions *are* the read's own records the window did not carry —
the classes a partition of the read, the count equal to the regions' own count — so deleting
`Excluded.Regions` fails both cited cases.

The round's documentation moves are folded into §7: the control file's line sentence (the projection step
and the position-keyed ancestry), the seam sentence (the call unit the undischarged count reads) and the
refusal list (the selection's own answer before a source-side refusal); `docs/conventions.md`'s mirrored
rule; the plan's §11.12; `Q-0009-04`'s ancestry detail and the new `Q-0009-05`; and the four new citation
rows. The gate that round ran closed at `GATE: PASS`, exit 0, with `0009-citations.tsv` at 23 rows (the
transcript §6 carried until the fourth round replaced it with its own run).

### The fourth review round's findings (`10f8096`), fixed and re-measured

The review of `9242e49..10f8096` found nine items (two P1, five P2, two P3) — two behavioural defects
(identity-less leaf revival; a result rebound to another call sharing its id), three accounting defects
(an inactive branch's rewritten ancestry, an uncounted projection, an uncollected id-less call), one
measurement weakening (the D4 normalizer) and three test/evidence ones (the D3 pins, the duplicate-region
refusal's order, the redactions). This round fixes all nine. The round's instructions demanded a
*systematic close-out*: every rule the fixed surface claims is proved by one executed mutation — the fixed
line reverted, the test that claims to pin it run, the failure recorded, the line restored. Ten mutations
were run; every one failed for the reason its finding names, and the working tree's diff after the round is
byte-identical to the one the mutations were taken from (only the intended new test assertion was added
after that reference was saved):

```console
$ # M1: parentPlace identity-less fallback reverted to priorEntry (--select 0,1,3, parentless record)
$ go test -tags fts5 -count=1 -run TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor ./internal/ccc/
--- FAIL: TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor (0.00s)
    selection_test.go:1201: converting a window whose identity-less last record has no parent: err = <nil>,
    want ErrUnrepresentableLeaf: the fallback would chain it to the entry of the abandoned child
exit=1
$ # M2: codex toolResult answers any recorded occurrence with the id, written or not
$ go test -tags fts5 -count=1 -run TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID ./internal/ccc/
--- FAIL: TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID (0.00s)
    selection_test.go:1608: codex carried = {… ToolCalls:1 ToolResults:2 …}, want A's call and A's result:
    C is off the read's line and B's result's paired call is C
    selection_test.go:1616: the rollout holds B's result, whose paired call occurrence (ordinal 4) the
    read's line leaves off it: the id-blind reading would present it as another output of A:
        {"timestamp":"…","type":"response_item","payload":{"type":"function_call","name":"Bash",…}}
        {"timestamp":"…","type":"response_item","payload":{"type":"function_call_output","call_id":
        "toolu_reused_across_occurrences","output":"the first call's own output"}}
        {"timestamp":"…","type":"response_item","payload":{"type":"function_call_output","call_id":
        "toolu_reused_across_occurrences","output":"the later call's own output"}}
    selection_test.go:1620: codex unpaired_tool_results = 0, want the one carried result whose paired
    call is not in the written document
exit=1
$ # M3: OpenCode plan pairs a terminal result by id lookup alone
$ go test -tags fts5 -count=1 -run TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID ./internal/ccc/
--- FAIL: TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID (0.00s)
    selection_test.go:1642: unpaired_tool_results = 0, want 1
exit=1
$ # M4: keptAncestorEntry rejects an inactive ancestor unconditionally
$ go test -tags fts5 -count=1 -run TestSelectionKeepsAnInactiveBranchesRecordedAncestry ./internal/ccc/
--- FAIL: TestSelectionKeepsAnInactiveBranchesRecordedAncestry (0.00s)
    selection_test.go:1284: the off-line grandchild hangs off "the line child", want its recorded ancestor
    "the branch parent": the entry written before it stands for a record of the other branch, and
    rewriting the branch's own relation would change history the source did record
exit=1
$ # M5: keptAncestorEntry answers the projection as uncounted
$ go test -tags fts5 -count=1 -run TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall ./internal/ccc/
--- FAIL: TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall (0.00s)
    selection_test.go:1044: unresolved_source_parents = {… Count:1 …}, want the two places the 1,4
    window's walk could not answer: the excluded root and the read's tool-result projection
    selection_test.go:1063: the 0,1,4 window counts unresolved_source_parents = {… Count:0 …}, want the one
    relation the read's projection left unanswerable
exit=1
$ # M6: undischargedCalls skips every id-less call
$ go test -tags fts5 -count=1 -run TestSelectionCountsAnIdLessCallWithNoRecordedOutcome ./internal/ccc/
--- FAIL: TestSelectionCountsAnIdLessCallWithNoRecordedOutcome (0.00s)
    selection_test.go:1677: calls_undischarged = {Count:0 Regions:[]}, want the one id-less call: the
    carried document holds no result that could answer it
    selection_test.go:1716: calls_undischarged = {Count:0 Regions:[]}, want the call at ordinal 1: the
    id-less result stands before it in the line and cannot be paired with it
exit=1
$ # M7: the duplicate-region refusal checked only after a successful read
$ go test -tags fts5 -count=1 -run 'TestCheckSelectionOptionsRefusesASpecificationWithoutARead|TestContinueRefusesADuplicateRegionWithoutAReadableSource' ./internal/ccc/ ./cmd/ccc/
--- FAIL: TestCheckSelectionOptionsRefusesASpecificationWithoutARead (0.00s)
    selection_test.go:207: a region named twice ("1,1"): err = <nil>, want ErrBadSelection
    … (1-3,3, 1,1-, 1-2,2-3 likewise)
--- FAIL: TestContinueRefusesADuplicateRegionWithoutAReadableSource (0.02s)
    selection_route_test.go:332: exit 1, want 2 (stderr "ccc: session not found: claude:0000…\n")
    selection_route_test.go:338: stderr "…" does not name the duplicate region
exit=1
$ # M8: selectionClosure walks one forward pass instead of to a fixed point
$ go test -tags fts5 -count=1 -run TestSelectionClosesTheCounterpartGraphToAFixedPoint ./internal/ccc/
--- FAIL: TestSelectionClosesTheCounterpartGraphToAFixedPoint (0.00s)
    selection_test.go:1742: carried ordinals = [1 2], want the whole chain 0,1,2: the record the window
    pulled in carries a result of the first call
exit=1
$ # M9: strictSelectionRefusal swaps the two ordinals
$ go test -tags fts5 -count=1 -run TestSelectionRefusesASeparatedPairUnderStrictSelection ./internal/ccc/
--- FAIL: TestSelectionRefusesASeparatedPairUnderStrictSelection (0.00s)
    selection_test.go:549: strictly selecting "2": … the call toolu_…0001 recorded at ordinal 3 is selected
    and its recorded result at ordinal 2 is not … does not hold "recorded at ordinal 2 is selected"
    … (the other three arms likewise, both ordinals each)
exit=1
$ # M10: the probe folds no discovered mtime, so a report field that differs between checkouts is compared
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-probe-m10/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.812s        # the probe itself passes; the comparison moves
$ diff <(grep -v 'claude:c1a74e28' /tmp/ccc-d4-before1/probe.txt) <(grep -v 'claude:c1a74e28' /tmp/ccc-d4-probe-m10/probe.txt) | grep -c '^<'
45        # exactly the PROBE-RUN report lines (nine shared documents × five routes); no writer or artifact line
```

(The mutation transcript is the output as it was printed, laid out to one screen; the elisions are `…` and
wrap only inside a message. M10 measures the probe rather than a code line: the D4 probe's normalizer is
the subject R-24 names, so its mutation is the rule that remains — the mtime fold — removed, and the moved
lines are the report digests only.)

**R-18 — an identity-less leaf could reactivate an abandoned branch (P1), fixed.** A record with neither
an identity nor a recorded parent took `parentPlace`'s fallback straight to the entry written before it,
so a reduced window whose previous entry stood for an off-line record chained the identity-less record
there, and the leaf guard could not see it (its last-record half asks by identity). The fallback's own
check (`fallbackPlace`) now runs for that place too: the tree `3 → 1 → 0` of the review's trace is refused
by name (`ErrUnrepresentableLeaf`, naming the identity-less record by its ordinal and the off-line entry's
record), and the count stays the read's own (an identity-less record's place is the read's order, which
invents nothing). The extended `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor` pins the
refusal, its wording, and the control window `0,3` that excludes the abandoned sibling and converts with
the record under the root's entry.

**R-19 — a selected result could be written as the outcome of a different call (P1), fixed.** On a flat
target a call the read's line leaves off the document is not written at all, and both flat writers
accepted a result by looking its id up among the calls they had written: on the review's read — call A
answered, call B, an inactive sibling C reusing the id, B's result — the seam pairs B's result with C, the
closure carries C, the walk drops C, and the writers then wrote B's result as another output of A. The
pairing is now answered at occurrence granularity (`pairedCallOccurrences`, the same walk
`selectionPairing` and the closure read, keyed by the occurrence the writers write — the first row of a
message's coalesced id group, which is what `distinctCalls` writes), `Select` carries it as the read's own
answer, and each flat writer asks whether the *paired occurrence* was written (`writtenCalls` on Codex,
`plannedCalls` on OpenCode): a result whose paired call cannot travel is omitted and counted
(`unpaired_tool_results`), which is what the category's own sentence already says. The tree writers keep
their id check because they write every carried record, so the paired occurrence is always in the file.
(The fifth round superseded that reading: being in the file is not standing on the result's own written
ancestry, and R-29 replaces the id check with that one.)
`TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID` pins both directions: the rollout holds A's
own output and not B's, Codex counts the omission and the inactive record, and OpenCode's plan counts the
same omission and no longer files B's result as a second outcome of A
(`repeated_tool_results` asserted zero).

**R-20 — a sparse selection rewrote ancestry inside an inactive branch (P2), fixed.**
`keptAncestorEntry` refused an *inactive* retained ancestor unconditionally, so the review's read
(`0 → 1 → 2 → 4` off the line, `0 → 3 → 5` on it; `--select 0-1,3-5`) attached ordinal 4 to ordinal 3's
entry instead of ordinal 1's. An off-line record now keeps an off-line ancestor: hanging a branch under
the entry the source recorded is what the writer reproduces, and the branch stays off the walk from the
file's last entry. An *on-line* record is still refused through an off-line ancestor (the shape the guard
exists for), and `TestSelectionKeepsAnInactiveBranchesRecordedAncestry` pins both arms by walking the
written `parentId` chain and asserting the refusal (`f6 (ordinal 5)` chained to `b2 (ordinal 1)`).

**R-21 — an inferred tool-result parent could travel without its loss disclosure (P2), fixed.**
`selectionKeptAncestors` takes the writers' tool-result step, but the answer did not say so, so a writer
consuming it recorded no `unresolved_source_parents` for the relation. `keptAncestorAnswer` now carries
`projected` and `parentPlace` counts it, exactly as the writers count the projection they resolve
themselves. Measured on the committed Pi fixture: `--select 1,4` reports 2 (its excluded root *and* the
projection), and `--select 0,1,4` and the unflagged whole read report 1 — the same place, so the window's
number is the read's own answer rather than one the reduction invented. The extended
`TestSelectionKeepsACarriedResultOnTheWalkThroughItsCall` asserts all three counts and the detail's
wording.

**R-22 — an id-less call with no result was reported as zero undischarged calls (P2), fixed.**
`undischargedCalls` skipped every call the source recorded with no id, even where the carried document
held no result at all — while OpenCode's own report counted the call `unpaired_tool_calls`. The count now
includes an id-less call when the carried document holds no id-less result at or after it (no outcome in
the line can answer it), and leaves the call to the writers' disclosed inference where one is carried, so
the count asserts neither answered nor undischarged for an ambiguity the source itself left.
`TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` pins the three shapes (no result; an id-less result
after the call, where the writer's `generated_call_ids` names the inference; an id-less result *before*
the call, which cannot answer it) and the OpenCode route's own agreeing count.

**R-23 — duplicate-region syntax errors were masked by source-read failures (P2), fixed.** Overlap was
checked only after a successful read, so `--select 1,1` against a missing session was answered with the
source failure instead of the usage refusal. The check moved into the grammar step
(`parseSelectionSpec` → `checkSelectionOverlap`, running before any read), and `Select`'s space check keeps
the other half. Measured through the CLI: `--select 1,1` with a missing session answers the duplicate
sentence with exit 2 and installs nothing, while the same command line without `--select` still reports
`session not found` (exit 1). `TestCheckSelectionOptionsRefusesASpecificationWithoutARead` covers the
closed and open region shapes; `TestContinueRefusesADuplicateRegionWithoutAReadableSource` covers the CLI,
including "nothing was written".

**R-24 — the D4 probe masked changes to recorded timestamps (P2), fixed.** The normalizer replaced every
RFC 3339 instant and every numeric `timestamp`/`created`/`updated`, so a one-second change to a source
timestamp (same digit count) folded to the same digest and length. The retained revision (§4) folds only
what a conversion discovers — the checkout root, scratch paths, the discovered mtime — plus the identity
canonicalization, compares recorded instants, and carries `d4ProbeNormalizerControl`, whose three printed
lines pin both halves (a recorded instant/millisecond change moves the digest; a minted identity change
does not). Re-run: both trees' runs are stable, the shared 1024 lines are byte-identical with recorded
instants in the comparison (the round-3 record's same claim had rested on the masking rules), and the
line counts (1024 / 1149 / 125) are unchanged next to the narrower fold set. `docs/evidence/0009-d4-probe.txt`
is the exact revision that printed §4's transcript.

**R-25 — the D3 tests did not pin fixed-point closure or the refusal's ordinals (P3), fixed.** The closure
cases exercised isolated pairs (and the OpenCode one put both halves on one record), so a single forward
pass survived them, and the strict case checked neither ordinal. `TestSelectionClosesTheCounterpartGraphToAFixedPoint`
selects the *last* record of a chain (call 0 → result 0 + call 1 → result 1): the window pulls in ordinal
1, and only a second pass pulls in ordinal 0 — the case M8 above fails on a one-pass closure — with each
addition's puller asserted; `TestSelectionRefusesASeparatedPairUnderStrictSelection` now asserts each
sentence half with the ordinal of the side it names (M9 swaps them and fails).

**R-26 — committed evidence exposed host-specific paths (P3), fixed.** §1's Codex binary path and both
gate transcripts' checkout and toolchain paths are replaced by labeled placeholders, the excerpts are
marked *redacted*, and the record's header states the convention and the redaction (`docs/conventions.md`'s
fixture rule: no absolute host paths in committed evidence). The round's own new transcript uses the
placeholders from the start.

The round's own verification: the two packages' Go cases run green, the probe re-ran as §4 quotes
it, and the gate §6 quotes is this round's own run — `GATE: PASS`, exit 0, with `0009-citations.tsv` grown
23 → 27 rows (`S2-occurrence`, `S2-idless`, `S6-inactive`, `D5-duplicate`) and four rows' artifact text
extended (`S6-projection`, `S6-identity`, `D5-syntax`, `D5-space`; the cited tests are the extended ones,
so the rows and the cases still name one another). 439 → 444 top-level `Test…` functions in the two
packages' test files: the five the round adds are the four core cases and the CLI case named above.

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	17.979s
ok  	go.kenn.io/agentsview/cmd/ccc	6.309s
```

### The fifth review round's findings (`dfe5a90`), fixed and re-measured

The review of `9242e49..dfe5a90` found four items (two P1, two P2): the pairing ignored a result's exact
recorded ancestry where an inactive sibling reused the id later in file order (and the round-4 occurrence
checks then dropped a result the base writers retained, breaking D4); the tree writers accepted a result
whose paired call was written but off the result's own written ancestry; `calls_undischarged` disagreed
with the writers' own outcome evidence (a summary-only archive call counted as unanswered, an id-less
progress event suppressing the count); and the D4 probe's normalizer folded recorded identities by class
shape, so a consistent rename of a recorded tool id could not move its digest. This round fixes all four.
The round's instructions demanded a *systematic close-out*: every rule the fixed surface claims is proved
by one executed mutation — the fixed line reverted, the test that claims to pin it run, the failure
recorded, the line restored. Eight mutations were run; every one failed for the reason its finding names,
and the tree after the restores is the revision §4's probe measured (its output re-diffed byte-identical):

```console
$ # M1: the pairing's ancestry arm disabled (nearest prior occurrence alone)
$ go test -tags fts5 -count=1 -run TestSelectionPairsAResultByItsRecordedAncestry ./internal/ccc/
--- FAIL: TestSelectionPairsAResultByItsRecordedAncestry (0.00s)
    selection_test.go:1647: carried ordinals = [4 5], want 3,5: the result names the call at ordinal 3 as
    its parent
    selection_test.go:1652: necessities = {Count:1 Added:[{Ordinal:4 PulledBy:5}]}, want the call at
    ordinal 3 pulled in by the result
    selection_test.go:1663: carried ordinals = [1 2 4 5], want the window's 1,5 with the counterparts 2
    (A's result) and 3 (the call the last result answers)
exit=1
$ # M2: the carried pairing table ignored, so a writer re-reads the pairing over the window
$ go test -tags fts5 -count=1 -run TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry ./internal/ccc/
--- FAIL: TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry (0.00s)
    selection_test.go:1900: codex: the written document does not hold the result the read paired with the
    call at ordinal 1: re-reading the pairing over the window falls back to the carried sibling, which
    the read's line leaves off it
    selection_test.go:1905: codex: unpaired_tool_results = 1, want 0: the paired call is the carried one
    the read named, not the sibling
    … (opencode likewise; claude refuses the window: b3, the record its last entry stands for, is off
    the line the read's last record answers)
exit=1
$ # M3: recordOnWrittenPath answers true unconditionally (the tree writers' id map restored)
$ go test -tags fts5 -count=1 -run TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath (0.00s)
    selection_test.go:1984: claude: unpaired_tool_results = 0, want the one result whose paired call is
    off its written ancestry
    selection_test.go:1994: claude: the written document holds the result under a call that is not on its
    written ancestry
    … (pi and omp likewise)
exit=1
$ # M4: summaryDischarges answers false (the old count)
$ go test -tags fts5 -count=1 -run TestSelectionCountsASummaryOnlyCallAsAnswered ./internal/ccc/
--- FAIL: TestSelectionCountsASummaryOnlyCallAsAnswered (0.00s)
    selection_test.go:2166: calls_undischarged = {Count:1 Regions:[0]}, want none: the archive's own
    summary of the call is the outcome evidence the read holds
exit=1
$ # M5: the id-less scan counts any id-less result, progress included
$ go test -tags fts5 -count=1 -run TestSelectionCountsAnIdLessCallWithNoRecordedOutcome ./internal/ccc/
--- FAIL: TestSelectionCountsAnIdLessCallWithNoRecordedOutcome (0.00s)
    selection_test.go:2130: calls_undischarged = {Count:0 Regions:[]}, want the call counted: an id-less
    progress event states no outcome
exit=1
$ # M6: the probe's normalizer folds recorded identities again (the record set ignored)
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-probe-m6/probe.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
PROBE-CONTROL recorded-instant-change moves-digest=true
PROBE-CONTROL recorded-millisecond-change moves-digest=true
PROBE-CONTROL recorded-call-id-change moves-digest=false
PROBE-CONTROL recorded-uuid-change moves-digest=false
PROBE-CONTROL minted-identity-change moves-digest=false
--- FAIL: TestZZD4Probe (0.66s)
    zz_d4_probe_test.go:505: the normalizer folds two different recorded call ids to the same bytes: a
    source-recorded tool id change has to move the digest
    zz_d4_probe_test.go:505: the normalizer folds two different recorded uuids to the same bytes: a
    source-recorded record identity change has to move the digest
exit=1
$ # M7: the id-less arm drops the summary discharge
$ go test -tags fts5 -count=1 -run TestSelectionCountsASummaryOnlyCallAsAnswered ./internal/ccc/
--- FAIL: TestSelectionCountsASummaryOnlyCallAsAnswered (0.00s)
    selection_test.go:2221: calls_undischarged = {Count:1 Regions:[0]}, want none: the archive's summary
    is the outcome evidence the read holds for the id-less call too
exit=1
$ # M8: the tree's written-entry places carry no record (the acceptance side of recordOnWrittenPath)
$ go test -tags fts5 -count=1 -run 'TestSelectionPairsAResultByItsRecordedAncestry|TestPiSessionChainsPastAMixedRecordToItsResult' ./internal/ccc/
--- FAIL: TestPiSessionChainsPastAMixedRecordToItsResult (0.00s)
    pi_target_test.go:597: the record after the mixed record hangs off the entry holding "the output:",
    want the mixed record's tool result ("ok"): mapping the source record to its first entry instead
    leaves the result off the walk the target's loader makes
    pi_target_test.go:603: the mixed record's tool result is not on the walk the loader makes from the
    file's last entry, …
    pi_target_test.go:609: the model-visible history = [user assistant user assistant], want [user
    assistant user toolResult assistant]
--- FAIL: TestSelectionPairsAResultByItsRecordedAncestry (0.00s)
    selection_test.go:1791: claude: unpaired_tool_results = 2, want 0: the result's paired call is
    written on its own ancestry
    … (pi and omp likewise; the recorded walk holds the call and not the result)
exit=1
```

(The mutation transcript is the output as it was printed; the elisions are `…` and wrap only inside a
message. M6 measures the probe's own control: it is the rule R-31 names, and the two control lines it
flips are the machine-checked pin. M8 is the acceptance side of `recordOnWrittenPath` — the places the
check reads have to carry the record they stand for — and it is pinned by the increment's existing
mixed-record case beside this round's new one.)

**R-27 — the pairing let an inactive sibling override a result's recorded parent (P1), fixed.** The seam
paired a result with the nearest prior call the same id records (`pairedCallOccurrences`), so on a
Claude-shaped read — call A at 1 answered at 2, call B at 3, an inactive sibling C at 4 reusing the id,
B's result at 5 naming B as its recorded parent — B's result was paired with C: `--select 3,5
--strict-selection` refused a pair the source recorded together, selecting 5 alone pulled in C's
arguments instead of B's, and the round-4 occurrence checks then dropped B's result on the flat routes
(the base writers had retained it — the D4 break the review measured). The pairing now honors the read's
own ancestry where it establishes the call: `pairedCallOccurrences` walks the result's recorded parent
chain (its own record first, then the identities the read holds, bounded by the document's own size) and
pairs with the call the walk names; the nearest-prior-occurrence rule stands only where the ancestry
establishes none. `TestSelectionPairsAResultByItsRecordedAncestry` pins the window (`1,5` carries 1,2,3,5
with C excluded), the strict pair, the sibling's own undischarged call, every route's written output and
the unflagged conversion's retention of B's output.

**R-28 — a window could substitute the pairing the read answered (P1), fixed.** The reduction drops the
records an ancestry walk may step through, so a writer re-reading the pairing over the reduced document
could fall back to a nearer *carried* occurrence of the id — a sibling the read's line leaves off — and
drop (or misplace) a result the closure had carried the counterpart for. `Select` now carries its own
answers beside the reduced document (`Conversation.sourceResultPairs`, in the reduced document's
positions, translated for exactly the pairs whose both halves the window carries), and
`pairedCallOccurrences` returns them where they exist. `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry`
pins it on all five routes: without the carried answers the result is dropped and counted on the flat
targets and the tree route refuses the window outright (M2).

**R-29 — the tree writers accepted a result whose paired call was off its written ancestry (P1), fixed.**
Claude, Pi and OMP write every carried record — including off-line branches — so their file-wide id maps
(`callOwners`, `callNames`) admitted a result whose paired occurrence was written *somewhere*, even when
it stood on another branch: on the review's window the final result hung off A's result through its
nearest retained ancestor while its paired call was C, a sibling, so the walk the target's loader made
from the result held the output and never the invocation. The tree now records each written entry's place
and the record it was written for (`sourceTree.entryPlaces` in `wrote`, `recordPlace` at `startRecord`,
because `pending.parentID` is cleared by the record's first written entry) and answers
`recordOnWrittenPath` by walking that chain; both tree writers ask it where they accepted the id before,
and a result whose paired occurrence is not on its own written ancestry is omitted and counted
(`unpaired_tool_results`) while the call still travels as recorded history where the window carried it.
`TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath` pins both writer families, the count, the
result's absence from the file, and the call's presence in the file *and* off the Claude and Pi walks;
`Q-0009-06` records the disposition for both families.

**R-30 — `calls_undischarged` disagreed with the writers' own outcome evidence (P2), fixed.** An archive
call whose only evidence of an outcome is the summary the archive stores on the call itself
(`ToolCall.ResultContent`, with no recorded result event) was counted undischarged and warned about —
while the routes write that summary as the call's completed outcome (`derivedResultSummary` beside
`callHasRecordedResult`, the writers' own guard). The count now discharges such a call through the same
guard (`summaryDischarges`), while a summary beside a progress-only event keeps the count (the writers
refuse to present progress as an outcome). In the inverse direction, an id-less *progress* result
suppressed an id-less call's count even though it states no outcome; the id-less scan now asks
`resultStatesAnOutcome`, so a running/pending/streaming event leaves the call counted and an outcome
status leaves it to the writers' disclosed inference, exactly as before.
`TestSelectionCountsASummaryOnlyCallAsAnswered` pins the summary arm and the progress control; the
extended `TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` pins the status-aware arm.

**R-31 — the D4 probe folded recorded identities by class shape (P2), fixed.** The normalizer folded
every uuid and every `call_…`/`ses_…`/`msg_…` token and canonicalized every uuid, so a consistent
same-length rename of a recorded Codex `call_id` — in the call and its output — preserved both the written
length and the folded digest; the probe's byte-preservation claim could not see it. Each document's
recorded identities (`d4ProbeRecordedIDs`: the records' own identities, the calls' and results' tool ids,
the session's ids) are now installed on the normalizer and skipped by the fold, so a recorded identity is
compared as recorded, and `d4ProbeNormalizerControl` gains two printed lines pinning it
(`recorded-call-id-change`, `recorded-uuid-change`; five lines now). §4's transcript is the revised
probe's own run against the same two trees: the line counts are the previous round's (1024 / 1149 / 125),
both per-tree stability diffs and the shared-document diff are silent — now with recorded identities in
the comparison — the controls agree between the trees, and the digests of texts carrying recorded
identities moved as the fold narrowed (`pi:01a1c335-…`'s Pi writer line: folded 2051 → 2105 with the
written bytes unchanged).

The round's own verification: the two packages' Go cases run green (below), the probe re-ran as §4
quotes it, and the gate below is this round's own run — `GATE: PASS`, exit 0, with
`0009-citations.tsv` grown 27 → 30 rows (`S2-ancestry`, `S2-path`, `S2-summary`) and three rows' artifact
text updated (`S2-reuse`, `S2-occurrence` — rewritten to the off-path test — and `S2-idless`).
441 → 444 top-level `Test…` functions in the two packages' test files (counted at HEAD and in this tree):
the round adds four cases and replaces the one the review's first finding named —
`TestSelectionPairsAResultByItsRecordedAncestry` and
`TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath` cover what
`TestSelectionDoesNotRebindAResultToAnotherCallSharingItsID` pinned (the read's pairing, both writer
families), `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry` pins the carried answers,
and `TestSelectionCountsASummaryOnlyCallAsAnswered` pins the summary evidence.

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	17.968s
ok  	go.kenn.io/agentsview/cmd/ccc	6.001s
```

## 7. Docs, rules and bookkeeping

* `AGENTS.md` — a new binding bullet ("A conversion carries only what the caller selected …") outside the
  per-harness bullets, and `internal/ccc`'s file list in "How to work here" names `selection.go`. The
  control file moves because this increment changes a rule an agent must follow and the core's layout. The
  second fix round rewrote the bullet's seam sentence (pairing by the read's own order), added the
  progress-is-not-an-outcome sentence, extended the line sentence with the ancestry rule, and added the
  supplied-blank refusal to the refusal list. The third fix round extended the line sentence (the ancestry
  walk takes the tool-result step the writers take, and the answer and the revival check travel by the
  record's own position), added the call unit the undischarged count reads to the seam sentence, and added
  the "a bad selection's own refusal is answered even where the source would also refuse" clause to the
  refusal list. The fourth fix round added the flat writers' occurrence pairing to the seam sentence, the
  id-less call's counting rule beside the id-less record's, the inactive-ancestor and counted-projection
  clauses to the line sentence, and the duplicate-region clause to the refusal list. The fifth fix round
  rewrote the seam sentence (the pairing honors the read's own recorded ancestry where it establishes the
  call, else the nearest prior occurrence of the id), extended the writers' sentence to the tree targets
  (the paired occurrence has to stand on the result's own written ancestry), and added the archive
  summary's outcome evidence beside the progress-is-not-an-outcome clause and the *states-an-outcome*
  condition to the id-less call's count.
* `docs/conventions.md` — the layout block names `selection.go`; a new rule under "Rules of the work"
  states the specification's grammar, the seam, the refusals and the report's classes, with the second
  round's refinements (order pairing, progress, ancestry, supplied blank) folded into the same rule, the
  third round's (the projection step and the position-keyed ancestry, the call unit the undischarged
  count reads, the refusal order) folded in beside them, and the fourth round's (the flat writers'
  occurrence pairing, the id-less call's count, the inactive ancestor and the counted projection, the
  duplicate-region refusal's grammar half) folded in as well, and the fifth round's (the ancestry-first
  pairing, the tree writers' written-ancestry check, the summary-as-outcome-evidence and the
  states-an-outcome condition) folded in beside them.
* `docs/plans/0009-selective-context.md` — §11 records thirteen departures with the measurement behind each
  (the third round's four in §11.12) and the fourth and fifth rounds' seven and five at its close.
* `docs/goals/README.md` — 0009's row reads *Implemented on `feat/0009-selective-context`; awaiting
  operator acceptance*, and the goal file's own `Status:` line reads the same (the form 0007 and 0008
  carried while their increments awaited acceptance); nothing else was ready to archive (0008 was archived
  by the activation increment that landed this plan, and 0010–0013 are provisional while 0014–0017 are
  ready-but-unstarted).
* `docs/decisions/OPEN-QUESTIONS.md` — Q-0009-01 (the strict refusal's exit code); Q-0009-02 (rewritten:
  the seam pairs a result with the call the read's own ancestry establishes where it does, else the
  nearest prior call of the same id, an id-less record carries no link); Q-0009-03 (rewritten: a supplied
  blank value is refused, not read as the flag's absence); Q-0009-04 (the
  read's line is not the selection's to redefine, ancestry included — the third round's detail names the
  projection step and the position-keyed answer); Q-0009-05 (a command line whose selection the read's
  ordinals refuse is answered as that, with the usage exit code, before a source-side refusal the same
  command line would also meet); and Q-0009-06 (rewritten this round: a result whose paired call is not
  on its written ancestry — a flat line that leaves the call off, a tree whose walk does not hold the
  pairing — is omitted and counted rather than refused, the default for both writer families and its
  reversal).
* `README.md` — an example line (`--select 0-3,7`, in range for its fixture), a paragraph on the flag and
  its report, and `Select` in the engine list. Unchanged by the third and fourth rounds: the paragraph's
  claims (the order pairing, the progress rule, the blank refusal, the ancestry rule) are still what the
  code does. The fifth round rewrote the paragraph sentence that names the pairing rule (the ancestry
  first, the nearest prior occurrence of the id otherwise); its other claims stand.
* `docs/harness.md` — unchanged: this increment adds no gate step and no knob, and the citations step it
  already requires is the one that reads the new manifest.
* The throw-away probe file (§4) and the pre-change worktree were removed before the gate run; the probe's
  exact source is retained beside this record (`0009-d4-probe.txt`), and the third round replaced it with
  the revision that printed §4's runs (canonicalized identities, the report and artifact digests of the
  installing pipeline arm), the fourth round with the one it quotes (recorded instants compared
  instead of masked, `d4ProbeNormalizerControl` beside them), and the fifth with the one this record
  quotes (recorded identities left as recorded — `d4ProbeRecordedIDs` — with the
  `recorded-call-id-change`/`recorded-uuid-change` control lines pinning it). The
  fixture generator (`internal/ccc/fixturegen`) and the committed fixture tree move together, pinned by
  the existing `TestFixturesMatchTheirGenerator`.
