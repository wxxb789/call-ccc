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
| what an unflagged run writes | nothing the previous revision did not: a throw-away before/after probe over five routes × the eleven fixtures the two trees share **plus two materialized transcripts the committed set does not cover** (the round-6 off-path case and the round-9 colliding-id case) prints the same normalized bytes on the pre-0009 tree and on this tree, with source-recorded instants *and recorded identities* in the comparison (the normalizer folds only what a conversion mints or discovers, §4) — the shared-document diff is **silent over every one of the 1183 lines**, the two-file `apply_patch` fixture's Codex structural verdict included: the invocation-unit comparison is the reduction's own (round 10's finding 2), so an unflagged run keeps the verdict, the warning and the exit status every revision before the fold answered — and the increment's new fixture adds its own 125 lines with no before-run (§4; the probe's exact source is `docs/evidence/0009-d4-probe.txt`) |

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
Claude transcript — **plus two transcripts the committed set does not cover, materialized at run time so
both trees read identical records**: the round-6 off-path case (`d4ProbeOffPathTranscript`) and the
round-9 colliding-id case (`d4ProbeCollisionTranscript`) — and for each document × each of the five routes
it does two things:

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
was run twice. The pre-0009 tree is a throw-away worktree with the identical probe file copied in. The
revision adds the round-6 review's case to the comparison (its finding 1) and the round-9 review's case
(its finding 3): the off-path transcript and the colliding-id transcript the committed fixtures do not
carry are materialized at run time under a scratch Claude root (`d4ProbeOffPathTranscript`,
`d4ProbeCollisionTranscript`), so both trees read identical records for them and the cases travel with the
probe rather than with either tree's fixture set:

```console
$ git worktree add --detach /tmp/ccc-d4-r10-before 9242e49b2d38aa8474f530c46e26fc3ee3b7a226
$ cp docs/evidence/0009-d4-probe.txt /tmp/ccc-d4-r10-before/internal/ccc/zz_d4_probe_test.go
$ cd /tmp/ccc-d4-r10-before
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r10/before1.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r10/before2.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ cd -
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r10/after1.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r10/after2.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ diff /tmp/ccc-d4-r10/before1.txt /tmp/ccc-d4-r10/before2.txt   # silent: one tree's runs agree
$ diff /tmp/ccc-d4-r10/after1.txt  /tmp/ccc-d4-r10/after2.txt    # silent
$ grep -v 'claude:c1a74e28' /tmp/ccc-d4-r10/before1.txt > /tmp/ccc-d4-r10/before-shared.txt
$ grep -v 'claude:c1a74e28' /tmp/ccc-d4-r10/after1.txt  > /tmp/ccc-d4-r10/after-shared.txt
$ wc -l /tmp/ccc-d4-r10/before1.txt /tmp/ccc-d4-r10/after1.txt
1183 /tmp/ccc-d4-r10/before1.txt
1308 /tmp/ccc-d4-r10/after1.txt
$ awk '{print $1}' /tmp/ccc-d4-r10/after1.txt | sort | uniq -c
     70 PROBE-WRITE
    425 PROBE-LOSS
    813 PROBE-RUN
$ diff /tmp/ccc-d4-r10/before-shared.txt /tmp/ccc-d4-r10/after-shared.txt   # silent
$ grep -c 'claude:c1a74e28' /tmp/ccc-d4-r10/after1.txt
125
```

The line counts are what the probe's own arms produce: the after tree's 1308 = 70 `PROBE-WRITE` (5 routes
× 14 documents) + 425 `PROBE-LOSS` + 813 `PROBE-RUN`; the pre-0009 tree's 1183 = 65 + 382 + 736 (5 routes ×
13 documents: the eleven pre-0009 fixtures plus the two materialized transcripts), and the increment's new
fixture accounts for the 125-line difference. Both stability diffs are silent, so the folded digests are a
statement about the conversion rather than about the ids it mints. The shared-document diff is silent over
**every one of the 1183 lines** — the two-file `apply_patch` Codex fixture (`codex:5b2d7c14-…`, the rollout
the increment's selection tests select from) included, whose verification line, warning and report digest
are the pre-0009 tree's own: the invocation-unit comparison is a document a selection produced's (round
10's finding 2), so that fixture's unflagged conversion keeps the `not-verified` verdict — the inherited
reader projects its one written `function_call` record into one call row per file it names, and the
rows-against-items comparison reports the extra rows, exactly as every revision before the fold did — and
the verdict, the warning, the report digest and the CLI exit status that follows from them are the default
path's own. Every `artifact` line and every writer line agrees on both trees (the written bytes are
identical), so no conversion this probe runs writes a byte the pre-0009 tree did not, *with
source-recorded instants and recorded identities in the comparison* (the round-3 and round-4 records' same
claim rested on a normalizer that masked them; the `PROBE-CONTROL` lines above are what makes this
revision's claim stronger).

The one line-group the ninth round's revision of this section recorded moving -- that fixture's Codex
report digest, its verification line and the warning that restated the verdict -- is the pre-0009 tree's
own again, and the line the review quoted as the finding is what the default path answers:

```console
$ grep 'codex:5b2d7c14-6e8a-4f31-b0c5-9d7e21a4c8fe codex verification' /tmp/ccc-d4-r10/after1.txt
PROBE-RUN codex:5b2d7c14-6e8a-4f31-b0c5-9d7e21a4c8fe codex verification structure=not-verified load=not-requested resume=not-requested structure_detail=the installed session does not hold the evidence in the target document: tool call 3 is in the file but was not written: call call_02Example000000000000000000 to apply_patch (128 bytes, sha256 552563bd…); tool call 4 is in the file but was not written: call call_02Example000000000000000000 to apply_patch (128 bytes, sha256 552563bd…)
```

The pipeline arm runs every conversion the probe lists:
each of the 70 runs prints its conversion block, and the arms that stop early stop at the same
environment-answered points on both trees — the two branch-switch fixtures (`pi:01a1c337-…`,
`omp:01a1c33a-…`) on the four non-OpenCode routes (8 runs, two lines each: the source-side leaf refusal
before any writer runs), every OpenCode run (14, the absent target binary the arm deliberately names),
and the new off-path case's Claude arm, whose `structure` line is `not-verified` with the warning quoted
below — the installed file's own reader walks the parent DAG from its tip, the walk that leaves the branch
record out, and both trees answer identically. Where the installation wrote the writer's own bytes the two
digests agree, which is what makes the artifact line a measurement of the installation rather than a
restatement of the writer arm:

```console
$ grep 'pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi ' /tmp/ccc-d4-r6/after1.txt | grep 'PROBE-WRITE\|artifact'
PROBE-WRITE pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi written=2163 folded=2105 digest=471978ad873994d985d602968073088225ee221071c5897a932f543eb62b9f89 carried={Messages:4 UserMessages:1 AssistantMessages:3 ToolCalls:1 ToolResults:1 CompactionBoundaries:0 Turns:0 Bytes:2163}
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 pi artifact written=2163 folded=2105 digest=471978ad873994d985d602968073088225ee221071c5897a932f543eb62b9f89
$ grep -m1 'PROBE-WRITE pi:01a1c336-' /tmp/ccc-d4-r6/after1.txt
PROBE-WRITE pi:01a1c336-4b02-78d3-9e15-6c7a2f0b8d41 claude written=2895 folded=2172 digest=57c2890a5459281755a46737a166f8de36d44f51c53df4273bcb9cc08a54fc94 carried={Messages:5 UserMessages:2 AssistantMessages:3 ToolCalls:0 ToolResults:0 CompactionBoundaries:1 Turns:0 Bytes:2895}
$ grep 'pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode ' /tmp/ccc-d4-r6/after1.txt | grep -E 'error|artifact'
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode error=ccc-no-such-target-harness is not on PATH, and this target's installation is the harness's own importer: exec: "ccc-no-such-target-harness": executable file not found in $PATH
PROBE-RUN pi:01a1c335-7098-71c2-a91b-b5295c0464f5 opencode artifact=none installation=not-installed
```

The new case's own lines, all three tree routes carrying the result the read's pairing names (the call is
written as recorded history and the result is accepted by the id a written call shares, which is the
default path's acceptance; the flat routes' line leaves the branch record off, so their call set is empty
and their result is omitted on both revisions):

```console
$ grep 'off-path' /tmp/ccc-d4-r6/after1.txt | grep 'PROBE-WRITE'
PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 claude written=2350 folded=1753 digest=57659d2b9b97763c7bd42f71f1e0bdec15548562406e1c61e04b84869b27cdcc carried={Messages:4 UserMessages:3 AssistantMessages:1 ToolCalls:1 ToolResults:1 CompactionBoundaries:0 Turns:0 Bytes:2350}
PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 codex written=1941 folded=1817 digest=bde981808a0b31a044f8d46f0e9697db068bccda598017f3722053cd953f62ef carried={Messages:4 UserMessages:3 AssistantMessages:1 ToolCalls:0 ToolResults:0 CompactionBoundaries:0 Turns:3 Bytes:1941}
PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 omp written=2346 folded=2276 digest=f9f0341459d92faa61bf4c2abc2526f5903c411065a59728c72f24f51ceaa314 carried={Messages:5 UserMessages:3 AssistantMessages:2 ToolCalls:1 ToolResults:1 CompactionBoundaries:0 Turns:0 Bytes:2346}
PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 opencode written=917 folded=792 digest=d8234f353d972d73f2191d4c0be8859fa90c734a96728a1625c4db5279f69d43 carried={Messages:4 UserMessages:3 AssistantMessages:1 ToolCalls:0 ToolResults:0 CompactionBoundaries:0 Turns:3 Bytes:917}
PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 pi written=1802 folded=1738 digest=ebf55752094cc37d4ecfb4e89db161a72c96cb12928c4ab5d8a8be1c06400846 carried={Messages:5 UserMessages:3 AssistantMessages:2 ToolCalls:1 ToolResults:1 CompactionBoundaries:0 Turns:0 Bytes:1802}
```

The case is what makes the probe catch the regression it was added for (the round-6 finding 1): with the
four writers' scoping condition flipped — `resultCallWritten`/`pairedCallEntry` taking the reduced
document's branch unconditionally, which is the pre-fix head — the shared diff moves exactly 36 lines, all
of them this document's, while every committed fixture's lines stay identical:

```console
$ # the mutation: the four writers' `selectionReduced` scoping removed from the acceptance
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r6/mutant.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ grep -v 'claude:c1a74e28' /tmp/ccc-d4-r6/mutant.txt > /tmp/ccc-d4-r6/mutant-shared.txt
$ diff /tmp/ccc-d4-r6/before-shared.txt /tmp/ccc-d4-r6/mutant-shared.txt > /tmp/ccc-d4-r6/mutant.diff
$ grep -c '^[<>]' /tmp/ccc-d4-r6/mutant.diff
36
$ grep '^[<>]' /tmp/ccc-d4-r6/mutant.diff | sed -E 's/^[<>] PROBE-[A-Z-]+ ([^ ]+) .*/\1/' | sort -u
claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31
$ grep '^[<>]' /tmp/ccc-d4-r6/mutant.diff | grep -E 'claude written=|unpaired_tool_results' | head -4
< PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 claude written=2350 folded=1753 digest=57659d2b… carried={Messages:4 … ToolCalls:1 ToolResults:1 …}
< PROBE-LOSS claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 claude tool_result_sources omitted 1
> PROBE-WRITE claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 claude written=1913 folded=1409 digest=cd372f4f… carried={Messages:4 … ToolCalls:1 ToolResults:0 …}
> PROBE-LOSS claude:off-path-4d1b7c2e-8a3f-4c11-9b6d-0e5f7a2c8d31 claude unpaired_tool_results omitted 1
```

(The digests above are not comparable to the previous probe revisions': the earliest folded every
identity to a class-wide placeholder, masked every instant, and digested no report and no artifact, and
the fourth round's — while comparing recorded instants — still folded every uuid and every
`call_…`/`ses_…`/`msg_…` token by shape, so this revision's folded lengths and digests differ wherever a
recorded identity stands in the measured text (`pi:01a1c335-…`'s Pi writer line is the example quoted:
its folded length moved 2051 → 2105 while the written bytes did not). What is comparable is this
revision's own before/after pair, which was produced by one probe revision on both trees.)

The round-9 revision adds the colliding-id transcript and its mutant run for the same reason the round-6
case was added (its finding 3): the committed fixtures record no call whose id is the positional spelling
the writers mint for a later id-less call, so the default path's treatment of that collision — the
duplicate spelling the pre-0009 writer emits, `call_ccc_2_0` twice — was outside the comparison. The
transcript records the three families' positional spellings (`call_ccc_2_0`, `toolu_ccc_2_0`, `ccc2_0`)
on the record the read numbers first and an id-less call on the record it numbers second, and the
pre-0009 tree and this tree print the same 74 lines for it (the `PROBE-WRITE` lines below are the
evidence; the five digests are equal, not merely the lengths). The pre-fix head (`0d213e5`, the round-8
tree, whose writers walk every generated id past the taken ones) differs from the pre-0009 tree in
exactly this document's 32 lines and in no other shared line — the four tree writers' `PROBE-WRITE`
lines and the four pipeline arms' conversion, report and artifact lines, two bytes or a digest apart
where the `_2` suffix stands:

```console
$ git worktree add --detach /tmp/ccc-d4-r9-mutant 0d213e5ee11186455d9f51bbd2f1f9c80e69c1ba
$ cp docs/evidence/0009-d4-probe.txt /tmp/ccc-d4-r9-mutant/internal/ccc/zz_d4_probe_test.go
$ cd /tmp/ccc-d4-r9-mutant
$ CCC_D4_PROBE_OUT=/tmp/ccc-d4-r9/mutant1.txt go test -tags fts5 -count=1 -run TestZZD4Probe ./internal/ccc/
$ cd -
$ diff <(grep 'claude:collision' /tmp/ccc-d4-r9/before1.txt) \
       <(grep 'claude:collision' /tmp/ccc-d4-r9/mutant1.txt) | grep -c '^[<>]'
32
$ grep 'claude:collision' /tmp/ccc-d4-r9/before1.txt | grep 'PROBE-WRITE' | sed -E 's/ (PROBE|written).*//' | head -6
PROBE-WRITE claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40 claude
PROBE-WRITE claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40 codex
PROBE-WRITE claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40 omp
PROBE-WRITE claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40 opencode
PROBE-WRITE claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40 pi
$ grep 'claude:collision' /tmp/ccc-d4-r9/before1.txt | grep 'PROBE-WRITE' | grep -oE 'written=[0-9]+ folded=[0-9]+'
written=2566 folded=1899
written=1883 folded=1782
written=2087 folded=2029
written=550 folded=475
written=1543 folded=1491
$ grep 'claude:collision' /tmp/ccc-d4-r9/after1.txt | grep 'PROBE-WRITE' | grep -oE 'written=[0-9]+ folded=[0-9]+'
written=2566 folded=1899
written=1883 folded=1782
written=2087 folded=2029
written=550 folded=475
written=1543 folded=1491
$ grep 'claude:collision' /tmp/ccc-d4-r9/mutant1.txt | grep 'PROBE-WRITE' | grep -oE 'written=[0-9]+ folded=[0-9]+'
written=2568 folded=1901
written=1885 folded=1775
written=2089 folded=2031
written=550 folded=475
written=1545 folded=1493
$ awk '$2 != "claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183" && \
       $2 != "claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40"' \
      /tmp/ccc-d4-r9/before1.txt | sort > /tmp/ccc-d4-r9/base-rest.txt
$ awk '$2 != "claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183" && \
       $2 != "claude:collision-c0b1d2e3-4f50-4a61-8b72-9c8d7e6f5a40"' \
      /tmp/ccc-d4-r9/mutant1.txt | sort > /tmp/ccc-d4-r9/mutant-shared.txt
$ diff /tmp/ccc-d4-r9/base-rest.txt /tmp/ccc-d4-r9/mutant-shared.txt | wc -l
0
```

OpenCode's five lines are equal on all three trees, which is the finding's own scope note measured: its
positional id was already walked past the taken ones before selections existed, so there is no pre-0009
byte to restore there. The fixed tree's 74 lines are identical to the pre-0009 tree's, so the finding's
input is inside the comparison and the default path's byte identity for it is what the equality asserts.

The flag-absent comparison cannot discharge D4 by itself and is not asked to: what the selection *does*
when it is present is D1/D2's evidence (§1–§2 and the Go cases), and the flag-present-versus-flag-absent
comparison is a different claim (pinned in `cmd/ccc`'s case: a specification naming every record of the
read carries the same records the unflagged run carries, and its report holds the unflagged run's own loss
rows plus the reduction's own `coalesced_tool_call_projections` row — the Codex writer's category that
exists only on a document a selection produced, exactly so that the default path's report is untouched by
D4; the assertion names that one row rather than requiring none).

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
| a call whose read holds only progress events (`running`/`pending`/…, the Codex subagent notification) | not read as answered: the progress is not an outcome, so the call is counted undischarged; on a run that carries a selection the event itself is omitted and counted (`nonterminal_tool_events`, a row scoped to the reduction) because these formats carry an outcome or nothing, so a written event reads back as the call's own outcome, while a run without a selection keeps writing it (D4) | `TestSelectionCountsACallWithOnlyProgressEventsUndischarged`; `TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` |
| the compaction summary at ordinal 6 and the system record at ordinal 8 (whose tool result answers the cap call at 7) | carried inside a window that names them, boundary and system record included; the record's text is the target's own `system_records` row, its tool result travels | §2's carried counts (`compaction_boundaries 1`, `tool_results 1`) and its loss rows |
| every record the specification and the necessity did not name | stayed behind: their own words and call ids are absent from the written document | §1's table; `TestSelectionWritesExactlyTheSelectedOrdinals` (five routes, present/absent evidence per case) |
| a sparse window on the Pi history fixture: `--select 0,3,5` keeps the branch record at 3 and the active leaf at 5, leaving 5's recorded parent at 1 out | the leaf and the branch record both hang off their nearest retained ancestor (ordinal 0), so the walk from the file's last entry reaches ordinals 5 and 0 only; the branch record stays in the file as recorded history, off the walk. Measured on the written Pi file (`--to pi --verify none`): entries `6381b178` (ordinal 0) → `70bce9ed` (ordinal 3, the branch) and `a446a8b5` (ordinal 5, the leaf), both under `6381b178`, so the leaf's `parentId` chain is `a446a8b5 → 6381b178` | `TestSelectionKeepsASparseWindowOnTheReadsLine`; §4's probe covers the unflagged path |
| a window whose records have no retained ancestor at all (`--select 3,5` on the same fixture) | refused by name before anything is installed: the fallback would hang the leaf under the off-line branch record's entry | `ccc: … 5566cc77 (ordinal 5) … would be chained to the entry of 0011aa22 (ordinal 3), which that line leaves off it …` (exit 1, `ErrUnrepresentableLeaf`); `TestSelectionKeepsASparseWindowOnTheReadsLine` |
| a value supplied blank (`--select ""`) | refused as a request that names nothing, never read as the flag's absence: an automation's empty variable must not publish the whole history | §3's third block; `TestCheckSelectionOptionsRefusesASpecificationWithoutARead`, `TestContinueRefusesABadSelectionBeforeInstalling` |
| the source file itself | read whole, measured before and after | `source_check_state: unchanged` with the fixture copy's `sha256`, §1 |
| a call the source left unanswered, at a target whose format has no state for it (OpenCode) | the route leaves the call out and counts it — its own limit, named by its own report rather than claimed for every target | `unpaired_tool_calls ×1`; `TestSelectionWritesExactlyTheSelectedOrdinals`'s `TargetOpenCode` arm and `TestContinueReportsUndischargedCallsOnBothSurfaces` |
| a run without `--select` | the same conversion it was, byte for byte — the reduction-scoped dispositions (the written-ancestry acceptance, the path-restricted inference, the reserved inference slot, the walked-past generated ids, the omitted progress event) do not act, the writer's report keeps its own category list, and the Codex structural check keeps the rows-against-items comparison (so its verdict, warning and the CLI exit status that follows them are the ones every revision before the fold answered) | §4's shared-document diff over the 1183 lines (the two materialized cases included), with source-recorded instants *and recorded identities* in the comparison (§4's `PROBE-CONTROL` lines) and **silent** — the flag the ninth round's revision recorded moving is back to the pre-0009 tree's own (round 10's finding 2, this round); `TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath`'s unflagged arm, `TestUnflaggedFlatConversionKeepsTheIDAcceptance`, `TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall` and `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers`' unflagged verdict arm |
| a carried result whose paired call is not on its written ancestry (an inactive sibling reusing the id on a flat target; a call on another branch for a tree target) | omitted and counted on every route — on the flat targets the line leaves the call off, and on the trees the call is written but stands off the walk the target's loader makes from the result; a call the window carried still travels as recorded history, off that walk. The disposition belongs to the reduction: a run without `--select` keeps the acceptance its writer always had (some written call shares the id), which is what §4's silent diff measures | `TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath` (its unflagged arm); `Q-0009-06` |
| an id-less result whose only outstanding call stands off the written walk of the result's record (a sparse `0,1,3` window keeps call 1 and drops the call at 2 whose record was the result's recorded parent) | the inference may name only a call the walk the result's entry hangs off reaches: with no eligible call the result is omitted and counted, and with exactly one the entry states its own id (the positive `0,2,3` arm) | `TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath` |
| a result under a reused id where a later inactive sibling carries a *different* tool name (the window keeps the call the read paired with the result and the sibling) | the result record states the paired occurrence's id and name, not the last write for the id | `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry` |
| a selection applied to a document a selection already produced | refused by name (`ErrSelectionReduced`), answered before any specification is read: the line, ancestry and pairing answers a selection carries are the read's own and cannot be re-derived from the window, and a second reduction would revive the branch the first left off the line. The read itself is never rewritten, so the caller applies the specification to it | `TestSelectRefusesADocumentASelectionProduced`; `Q-0009-07` |
| an off-line record whose nearest retained ancestor is off-line too (a sparse window over a branch) | keeps that ancestor: the entry hangs where the source recorded it, and the branch stays off the walk from the file's last entry; a window whose *on-line* record would land under an off-line entry is still refused by name | `TestSelectionKeepsAnInactiveBranchesRecordedAncestry` |
| an id-less call in a carried document that holds no id-less result *that states an outcome* at or after it | counted `calls_undischarged` (no outcome in the line could be paired with it; an id-less progress event is not one), and the route whose format has no state for such a call counts `unpaired_tool_calls` beside it; where an id-less outcome result *is* carried the count asserts neither disposition and the writer's `generated_call_ids` names the pairing it inferred | `TestSelectionCountsAnIdLessCallWithNoRecordedOutcome` |
| an archive call whose only outcome evidence is the summary on the call itself (`ToolCall.ResultContent`, no recorded result event) | not counted `calls_undischarged`: the source recorded the outcome in the archive's derived column, and the routes write that summary as the call's completed outcome; the same summary beside a progress-only event keeps the count, because the writers refuse to present progress as an outcome | `TestSelectionCountsASummaryOnlyCallAsAnswered` |
| a window that keeps a call and *both* its result records — an id-less one before the record that names the call | the recorded result takes the one slot the part states and the id-less event is omitted (`unpaired_tool_results`) rather than inferred onto the call: a result the conversion writes reserves its call occurrence, so an inference may not displace a recorded pairing whose two halves the window carried; a result the conversion does not write (an off-line, subagent or compaction-boundary record's, a progress event) reserves nothing and the inference still fires, disclosed (`generated_call_ids`), while a reserved occurrence still counts as unanswered where the id-less result stands, so an ambiguous window omits the event | `TestSelectionReservesARecordedResultsSlotFromAnIdLessInference`; `TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite` |
| a Claude transcript whose result stands under a merged assistant entry (the writer emits one record per content block of one source record and the reader merges them into the run's last entry, which keeps the merged-away entry's parent) | the owner assertion resolves the invocation through the *installed file*'s own entry chain, stepping past the entry the read-back holds no message for, so a correct conversion verifies where a read-back-only walk stopped and reported the result missing; a chain the file cannot answer still ends, and a result entry reparented to a sibling is still reported | `TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord`; `TestSelectionReportsAResultWhoseAncestryLeavesItsCall` (the mutation control) |
| a specification that names a region twice (`--select 1,1`) against a source that cannot be read | the specification's own refusal — the duplicate-region sentence and the usage exit code — because the check is the grammar's and needs no read; the control run without `--select` still reports the missing source | `TestCheckSelectionOptionsRefusesASpecificationWithoutARead`; `TestContinueRefusesADuplicateRegionWithoutAReadableSource` |
| a source whose reader answers a call and its result events on the *same* record (the committed OpenCode document) | the record is carried whole with no necessity and nothing undischarged — the document already holds that structure in one unit | `TestSelectionReportsClassesThatReconcileWithTheRead`'s second arm (1 call + 2 result events on the carried record, classes partitioning the read) |
| a call the reader projects into several rows sharing one id, with one recorded result (the committed Codex `apply_patch` fixture) | the rows are one call and the recorded result answers it: nothing is counted undischarged, and the writer coalesces the rows into the one call it writes — Codex included, which on a reduced document emits one `function_call` per invocation and one output, and reports `coalesced_tool_call_projections`; the structural check folds the reader's rows into the file's own record units for that document alone (the same check run unflagged keeps the rows-against-items verdict), and it folds only the rows the file's records account for — a call record the reader reads under that id of a type whose key this build does not state, or a row whose key no record states, keeps the rows standing so an extra invocation is reported | `calls_undischarged {count 0}`; the writer's own `coalesced_tool_call_projections ×1`, one carried call and one result; `TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers`; `TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute` |
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
  PASS     selftest Ran 442 tests in 8.818s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 36 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

(`parity`'s detail is elided in this transcript with `…` — the step prints it as one long line, and the
numbers this record quotes are the ones it holds. The transcript is the sixth fix round's own run, taken
after the throw-away probe file was removed and before this section's and the round section's transcript
fills, which no gate step reads, and its two leading lines are the redacted placeholders the record's
header describes; `selftest`'s duration is that run's own. `0009-citations.tsv`'s row count moved 30 → 36:
the round's six new rows (`S2-default`, `S2-default-flat`, `S2-idless-path`, `S2-units-codex`, `S2-name`,
`S3-reduced`) each cite a test that exists and passes, and the `S2-units` row's artifact text was extended
to the Codex arm. `secrets` holds 196 files and `gofmt`, `test`, `parity` hold the counts of the previous
run: this round adds no file — the probe recipe was already retained, and every change landed in a file
the increment already carries.)

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
  `S2-summary` the archive's own summary as outcome evidence, `S2-idless` the
  id-less call's own count, and the sixth round's refinements — `S2-default` and `S2-default-flat` the
  default path's kept acceptance (D4's rule with its own Go pins, D4's byte evidence staying with §4),
  `S2-idless-path` the inference's written walk, `S2-units-codex` the Codex writer's one written
  invocation, and `S2-name` the paired occurrence's tool name; the refusals with the core's sentence and
  the usage code before any install —
  `S3-reduced` the second-selection refusal, `D5-duplicate` the grammar's duplicate-region half; the
  report's classes and the read's own ordinal space; the line rule, with `S6` for its ancestry half,
  `S6-inactive` for the off-line record that keeps an off-line ancestor, `S6-leaf` for the leaf guard's
  position-keyed answer (the seventh round's), `S6-identity` and `S6-projection`
  for the position-keyed answer and the counted projection, and `S7` for a
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
Measured (that round's probe revision, §4's predecessor): both trees' two runs are byte-identical, the
shared documents' 1024 lines are byte-identical before and after, and the increment's new fixture adds its
own 125 lines.

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
instants in the comparison (the round-3 record's same claim had rested on the masking rules), and that
revision's line counts (1024 / 1149 / 125) were unchanged next to the narrower fold set (the sixth round's
revision, §4's current one, prints 1109 / 1234 with the off-path case added). Each round's copy of the
probe is `docs/evidence/0009-d4-probe.txt` as that round left it; the sixth round's copy is §4's.

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
(`recorded-call-id-change`, `recorded-uuid-change`; five lines now). That revision's own run against the
same two trees (§4's predecessor; the sixth round's revision, which adds the off-path case, prints
1109 / 1234): the line counts were the previous round's (1024 / 1149 / 125),
both per-tree stability diffs and the shared-document diff were silent — now with recorded identities in
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

### The sixth review round's findings (`812fec9` + working tree), fixed and re-measured

The review of `9242e49..812fec9` found five items (three P1, two P2): the round-5 writers' written-ancestry
acceptance acted on runs without a selection, so a read whose pairing names a call the result's own written
ancestry does not reach lost the result where the base writer kept it (a D4 break the committed fixtures do
not cover, with the review's own input); the id-less inference could name an outstanding call that stands
on another branch, writing an output whose invocation the resumed context cannot see; the selected Codex
conversion wrote two `function_call` records for one projected invocation; Pi's and OMP's result record
could state the tool name of a reused id's later sibling; and a second `Select` over its own reduction
re-derived the line from the window and revived the branch the first reduction had kept off it. The round
fixes all five, and the sentence that keeps them from drifting is one: **everything a writer does
differently because a selection was asked acts only on a document a selection produced**
(`Conversation.selectionReduced()`), so a run without the flag keeps the acceptance, the report vocabulary
and the bytes it had before selections existed. The round's instructions demanded a systematic close-out:
every rule the fixed surface claims is proved by one executed mutation — the fixed line reverted, the test
that claims to pin it run, the failure recorded, the line restored. Eight mutations were run; every one
failed for the reason its finding names, and the tree after the restores is the revision §4's probe and
§6's gate measured:

```console
$ # M1: the four writers' selectionReduced scoping removed (the acceptance takes the reduced branch always;
$ #     the same mutation §4 runs against the probe, whose shared diff moves its 36 lines)
$ go test -tags fts5 -count=1 -run 'TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath|TestUnflaggedFlatConversionKeepsTheIDAcceptance' ./internal/ccc/
--- FAIL: TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath (0.00s)
    selection_test.go:2129: claude: the unflagged conversion does not hold the result: the default path's
    acceptance -- a written call sharing the id -- is what a run without a selection keeps
    selection_test.go:2134: claude: unflagged carried = {Messages:4 … ToolCalls:1 ToolResults:0 …}, want
    the call and the result, as before the selection existed
    selection_test.go:2138: claude: unflagged unpaired_tool_results = 1, want 0
    … (pi and omp likewise)
--- FAIL: TestUnflaggedFlatConversionKeepsTheIDAcceptance (0.00s)
    selection_test.go:2337: codex: the unflagged conversion drops the result: the read's pairing names an
    off-line occurrence, but the default path's acceptance is the id a written call shares
    selection_test.go:2342: codex: unflagged carried = {Messages:5 … ToolResults:0 …}, want the result
    carried as before the selection existed
    selection_test.go:2355: codex: unflagged unpaired_tool_results = 1, want 0
    … (opencode likewise)
exit=1
$ # M2a: the tree writers' id-less eligibility check bypassed (any outstanding call eligible again)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    selection_test.go:2186: claude: the written document holds the id-less result, inferred onto the call at
    ordinal 1, which does not stand on the walk the result's entry hangs off
    selection_test.go:2190: claude: unpaired_tool_results = 0, want the one result no eligible call answers
    selection_test.go:2194: claude: generated_call_ids = 1, want 0: the inference picked no call, so it
    minted no pairing either
    … (pi and omp likewise)
exit=1
$ # M2b: the eligibility check answers false everywhere (no reduced document's id-less result inferable)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    selection_test.go:2223: claude: the written document does not hold the id-less result the sole eligible
    call answers
    selection_test.go:2227: claude: unpaired_tool_results = 1, want 0: the inference had an eligible call
    selection_test.go:2231: claude: generated_call_ids = 0, want the one pairing the inference minted
    … (pi and omp likewise)
exit=1
$ # M3a: the Codex row coalescing taken unconditionally
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.00s)
    selection_test.go:1511: the unflagged codex conversion writes map[:2 agent_message:1 function_call:1
    function_call_output:1 message:2 user_message:1], want the two rows and the one output the writer has
    always written: the default path is byte-identical to the same run before selections existed
exit=1
$ # M3b: the coalescing removed (the pre-fix writer)
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.00s)
    selection_test.go:1494: the selected codex conversion writes map[:2 … function_call:2 …], want one
    function_call and one function_call_output for the one invocation
    selection_test.go:1498: the selected codex conversion carried = {Messages:2 … ToolCalls:2 ToolResults:1 …},
    want the one invocation and its outcome
exit=1
$ # M3c: the coalesced category taken unconditionally (a zero row in every codex report)
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.00s)
    selection_test.go:1517: the unflagged codex report names {Category:coalesced_tool_call_projections …}:
    no row is coalesced on the default path, and its report is byte-identical to the same run before
    selections existed
exit=1
$ # M4: the result's tool name read from the file-wide name map (the pre-fix answer)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry ./internal/ccc/
--- FAIL: TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry (0.00s)
    selection_test.go:1986: pi: the result entry = {ToolCallID:toolu_reused_broken_ancestry ToolName:Write},
    want the occurrence the read paired with the result (toolu_reused_broken_ancestry/Read), not the later
    sibling's Write
    … (omp likewise)
exit=1
$ # M5: the second-selection guard removed
$ go test -tags fts5 -count=1 -run TestSelectRefusesADocumentASelectionProduced ./internal/ccc/
--- FAIL: TestSelectRefusesADocumentASelectionProduced (0.01s)
    selection_test.go:2414: selecting the reduction (strict=false) = <nil>, want ErrSelectionReduced
    selection_test.go:2414: selecting the reduction (strict=true) = <nil>, want ErrSelectionReduced
    selection_test.go:2418: selecting the reduction with an empty specification = the selection cannot be
    honoured: the specification is empty: … want ErrSelectionReduced: the input's own refusal is answered
    before any specification is read
exit=1
```

(The mutation transcript is each run's printed output, with `…` eliding the messages the record's layout
cut; `mutate_many.sh` restores every edited file after its run, and the tree the restores leave is the one
§4's probe and §6's gate measured. M1's two `--- FAIL` blocks are the two cases the one mutation turns
red; the same mutation run against the probe is §4's 36-line diff.)

**R-32 — the reduction-scoped disposition acted on the default path (P1), fixed.** The round-5 fix made
Claude, Pi and OMP require the result's paired occurrence to stand on its own written ancestry, but asked
that check of *every* run: on a read whose pairing names an occurrence the result's written ancestry does
not reach (the review's input: a branch call whose record the read's line leaves off, the result paired
with it by the nearest prior occurrence) the head writers dropped a result the base writers retained —
different bytes and a different `unpaired_tool_results` count for a run without a flag, which D4 forbids.
Each writer's acceptance is now scoped by `Conversation.selectionReduced()`: a reduced document asks the
paired occurrence (the written walk on the trees, the written occurrence on the line targets), and a
conversation the read returned asks what the writer always asked — the id set it wrote (`callOwners` on
Claude and Codex, `callNames` on Pi/OMP, the planned call map on OpenCode). The unflagged arms of
`TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath` (trees) and
`TestUnflaggedFlatConversionKeepsTheIDAcceptance` (both line targets, on a read whose pairing names an
off-line occurrence) pin the default answer, and §4's probe carries the review's input as a materialized
off-path transcript: silent over 1109 shared lines, and the M1 mutation above moves exactly that case's 36
lines.

**R-33 — the id-less inference could place a result off its walk (P1), fixed.** A reduced document's tree
files hold the records the read's line leaves off as history, so the ledger's outstanding set can contain a
call on another branch; the inference read that whole set, so a sparse window's sole outstanding call — a
sibling of the result's own retained ancestor — answered the id-less result and the output was written
under a call the resumed context cannot see (`generated_call_ids` discloses an inference, not a placement).
`soleResultCall` now answers only the sole outstanding call that stands on the written walk of the result's
record (`sourceTree.recordOnWrittenPath`), and a result with no eligible call is omitted and counted;
`TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath` pins both arms (the sparse `0,1,3` window
omits and counts, the `0,2,3` window infers the ancestor call and states its id).

**R-34 — one reported invocation, two written ones (P1), fixed.** The inherited Codex reader projects one
`apply_patch` into one call per file it names, and the seam and the other four writers read those rows as
one invocation (`distinctCalls`); the Codex writer iterated the rows, so `--select 0-` over the committed
patch fixture emitted two `function_call` records with one id and one output for a call the report counted
once. `projectedCalls` folds a message's rows sharing an id on a reduced document (the rows not written
counted under `coalesced_tool_call_projections`), and the summary arm walks the same row list so a folded
projection's summary is not written twice. The category is carried only by a reduced document's report
(`Q-0009-08`): an unconditional zero row would move every codex report digest — measured at the row's 450
folded bytes — which is the D4 break R-32's probe case exists to catch, and the M3c mutation above is what
fails when the row stops being reduction-scoped.
`TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` asserts the selected conversion's
one call, one output and counted row beside the unflagged run's two rows and absent category.

**R-35 — a result could state the wrong occurrence's tool name (P2), fixed.** Pi's and OMP's result record
takes its `toolName` from the file-wide id map, whose last write wins; a window that carries an inactive
sibling reusing the id *after* the call the read paired the result with therefore labelled the resumed
result with the sibling's tool. The writer now keeps each written occurrence's id and name
(`writtenCalls`) and resolves a named result's name from the occurrence the pairing names; the id-less arm
takes the name of the occurrence it inferred to. `TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry`
now gives the sibling a different name and asserts the serialized result entry's id and name.

**R-36 — a second `Select` revived an abandoned branch (P2), fixed.** `Select` over a document it had
produced re-derived the line from the window's own last record, so a window that carried the Pi history
fixture's ordinals 2-4 as history off the read's line came back with those records active — the revival
the carried line answers exist to prevent. `Select` now refuses a document a selection produced
(`ErrSelectionReduced`) before any specification is read, because the line, ancestry and pairing answers
are the read's own and cannot be re-derived from the window; the read itself is never rewritten, so the
caller applies the specification to it. `TestSelectRefusesADocumentASelectionProduced` pins the first
selection's line flags and conversion, both strict arms, the empty-specification order and the handed
document unchanged; `Q-0009-07` records the default and its reversal (preservation instead of refusal).

The round's own verification: the two packages' Go cases run green (below), the probe re-ran as §4 quotes
it, and the gate below is this round's own run — `GATE: PASS`, exit 0, with `0009-citations.tsv` grown
30 → 36 rows (`S2-default`, `S2-default-flat`, `S2-idless-path`, `S2-units-codex`, `S2-name`, `S3-reduced`)
and the `S2-units` row's artifact text extended to the Codex arm. 447 → 450 top-level `Test…` functions in
the two packages' test files (counted at HEAD and in this tree): the round adds the three cases named
above and extends `TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath`,
`TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry`,
`TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` and
`cmd/ccc`'s `TestContinueWithoutASelectionReportsNoSelection` (whose whole-read-selection arm now names the
reduction's own report row instead of requiring none — the same records, the report difference stated).

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	17.886s
ok  	go.kenn.io/agentsview/cmd/ccc	4.945s
```

### The seventh review round's findings (`dea73c0` + working tree), fixed and re-measured

The review of `9242e49..dea73c0` found three items (two P1, one P2): the tree writers' id-less inference
read completion file-wide, so a recorded outcome the window carried on a *sibling* branch cleared a call
still outstanding on the result's own path and the result was inferred onto a later call both were
unanswered at (finding 1); the tree leaf guard answered by identity alone, so a selected window whose last
record was an identity-less record chained off the read's line through its *explicit* parent converted and
the target's loader would resume the branch the source's conversation left (finding 2); and the D1
citation's case checked substring membership, so a duplicated tool-result entry left it green (finding 3).
This round fixes all three and follows the review's systematic close-out: every rule the touched surface
claims was proved by **one executed mutation** — the rule's line reverted, the case that claims to pin it
run, the failure recorded, the line restored. One mutation came out of the close-out **unpinned** and the
round pins it: the fallback's off-line-record exemption (its mutation is silent over the then-existing
suite; the stranded-record arm this round added is what fails it now). Eleven mutations were run in all;
every one failed for the reason its rule names (the exemption's after its pin), and
`grep -rn 'false &&\|&& false' internal/ccc/*.go` is empty after the restores (each mutation is quoted
below as `M<number>`, the round's own findings-mutations `F…` first):

```console
$ # F2: lastEntryLineRefusal's position-keyed branch disabled for reduced documents
$ go test -tags fts5 -count=1 -run TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    claude: converting "0-2": err = <nil>, want ErrUnrepresentableLeaf: the window's last record is the
    identity-less one, chained to the entry of the abandoned turn
    (the same for pi, omp and the "0,2" window)
exit=1
$ # F1: answeredOnWrittenPath answering "answered anywhere" (the file-wide set the old code read)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    claude: the written document holds the id-less result: both calls on its own path were outstanding
    (A was answered only on the sibling branch), so the inference had no call to name
    claude: unpaired_tool_results = 0, want the one result no eligible call answers
    claude: generated_call_ids = 1, want 0: the inference picked no call, so it minted no pairing either
    (the same for pi and omp)
exit=1
$ # F3a: the Pi writer's writeToolEvidence writes each result entry twice
$ go test -tags fts5 -count=1 -run TestSelectionWritesExactlyTheSelectedOrdinals ./internal/ccc/
--- FAIL: TestSelectionWritesExactlyTheSelectedOrdinals (0.00s)
    pi: selecting "2" wrote the evidence
     got: ["call toolu_…01 Bash {…}" "result toolu_…01 build-ok: 3 packages compiled" "result toolu_…01 …"]
    want: ["call toolu_…01 Bash {…}" "result toolu_…01 build-ok: 3 packages compiled"]
    (the same for omp, and for "7-8")
exit=1
$ # F3b: the Codex writer's function_call_output states callID + "-wrong"
$ go test -tags fts5 -count=1 -run TestSelectionWritesExactlyTheSelectedOrdinals ./internal/ccc/
--- FAIL: TestSelectionWritesExactlyTheSelectedOrdinals (0.00s)
    codex: selecting "2" wrote the evidence
     got: ["call toolu_…01 Bash {…}" "result toolu_…01-wrong build-ok: 3 packages compiled"]
    want: ["call toolu_…01 Bash {…}" "result toolu_…01 build-ok: 3 packages compiled"]
exit=1
$ # M1: soleResultCall drops the on-written-path restriction (S2-idless-path)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    pi: the written document holds the id-less result, inferred onto the call at ordinal 1, which does not
    stand on the walk the result's entry hangs off   (the same for omp)
exit=1
$ # M2: resultCallWritten's reduced branch accepts any paired occurrence (S2-path)
$ go test -tags fts5 -count=1 -run TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionOmitsAResultWhosePairedCallIsOffItsWrittenPath (0.00s)
    pi: unpaired_tool_results = 0, want the one result whose paired call is off its written ancestry
    pi: the written document holds the result under a call that is not on its written ancestry
    (the same for omp)
exit=1
$ # M3: resultToolName answers the id map's last write (S2-name)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry ./internal/ccc/
--- FAIL: TestSelectionKeepsTheReadsPairingWhenTheWindowBreaksItsAncestry (0.00s)
    pi: the result entry = {ToolCallID:toolu_reused_broken_ancestry ToolName:Write}, want the occurrence
    the read paired with the result (…/Read), not the later sibling's Write   (the same for omp)
exit=1
$ # M4: resultCallWritten answers the written-ancestry question on every run (the D4 gate)
$ go test -tags fts5 -count=1 -run TestUnflaggedFlatConversionKeepsTheIDAcceptance ./internal/ccc/
--- FAIL: TestUnflaggedFlatConversionKeepsTheIDAcceptance (0.00s)
    pi: the unflagged conversion drops the result: the read's pairing names an off-line occurrence, but
    the default path's acceptance is the id a written call shares   (the same for omp)
exit=1
$ # M5a: fallbackPlace's off-line-record exemption removed (the stranded-record arm added this round)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsAnInactiveBranchesRecordedAncestry ./internal/ccc/
--- FAIL: TestSelectionKeepsAnInactiveBranchesRecordedAncestry (0.00s)
    claude: converting a window whose off-line record falls back to an off-line entry: … c3 (ordinal 2),
    a record on the line the read's last record answers, would be chained to the entry of b2 (ordinal 1)
    … want no refusal: the fallback's revival check is about a record the read's line holds
    (the same for pi and omp)
exit=1
$ # M6: the identity-keyed leaf refusal disabled
$ go test -tags fts5 -count=1 ./internal/ccc/
--- FAIL: TestPiSessionRefusesASelectedLeafOffTheLine (0.00s)
    error = <nil>, want ErrUnrepresentableLeaf: the read's leaf is the empty turn, whose line does not
    hold the branch entry written last
exit=1
```

(The M5b mutation — `fallbackPlace`'s revival note removed — failed three cases at once:
`TestSelectionKeepsASparseWindowOnTheReadsLine`, `TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor`
and `TestSelectionKeepsAnInactiveBranchesRecordedAncestry`, each on the arm the finding's rule names; it is
not requoted here.) The round's verification: the two packages' cases run green (below), `0009-citations.tsv`
moves 36 → **37** rows (the `S6-leaf` row; the `S2-idless-path` and `S6-inactive` rows extended), and the
two packages' test files carry 450 → **451** top-level `Test…` functions (one added,
`TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine`; none removed). The D4 probe re-ran over the
two trees (`git worktree add --detach /tmp/ccc-0009-base 9242e49b…`, the probe copied into both):
1109 / 1234 lines, the shared-document diff silent and the 125-line increment fixture absent from the
base run — §4's numbers reproduced with the round's changes, so no number in §4 moved.

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	19.686s
ok  	go.kenn.io/agentsview/cmd/ccc	5.991s
$ python3 scripts/gate.py; echo "exit=$?"
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.917s (rc=0)
  PASS     parity   carried: 5553 file(s) identical to the pinned revision
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 37 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns
GATE: PASS
exit=0
```

### The eighth review round's findings (`6bcf91c`), fixed and re-measured

The review of `9242e49..6bcf91c` found four items (three P1, one P3): a selected window that retained
neither record's ancestry converted with the line's own leaf chained under an identity-less off-line
record on the trees (the target's loader resumed the abandoned branch) and with that record written into
the line targets' logs (finding 1); two call occurrences the source recorded under one id collapsed into
one eligible call, so an id-less result was inferred onto the later occurrence -- and Pi/OMP could take
either occurrence's tool name by map order (finding 2); a `running` event cleared an outstanding call in
Claude, Pi/OMP and Codex, so a later id-less result was inferred onto a second call although both were
still unanswered where the result stood (finding 3); and the D3 citation named only the isolated-pair
case, which a single forward pass carries correctly (finding 4). This round fixes all four and follows the
review's systematic close-out: every rule the touched surface claims was proved by **one executed
mutation** -- the rule's line reverted, the case that claims to pin it run, the failure recorded, the line
restored. The three findings' own shapes were run on the pre-fix tree first (`F1`-`F3`; finding 4's rule is
a citation, proved by the `citations` step below), then the fixed lines were reverted one at a time --
twelve lines in all across `M1`-`M7`, the writers sharing a rule counted per writer. Every run failed for
the reason its rule names, and
`grep -rn 'false &&\|&& false' internal/ccc/*.go` is empty after the restores:

```console
$ # F1: the pre-fix line: the window "2-3" over the identity-less-leaf fixture (finding 1's shape)
$ go test -tags fts5 -count=1 -run TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    selection_test.go:1576: claude: converting "2-3": err = <nil>, want ErrUnrepresentableLeaf: the
    window's leaf would be chained to the entry of the identity-less record the read's line leaves off it
    (the same for pi and omp)
    selection_test.go:1598: codex: the written document holds the identity-less record the read's line
    leaves off it: written into this target's log it becomes model-visible history
    selection_test.go:1604: codex: inactive_branch_records = 0, want the one record the read's own walk
    does not reach   (the same for opencode)
exit=1
$ # F2: the pre-fix inference: two occurrences sharing an id and an id-less result (finding 2's shape)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    selection_test.go:3005: claude: the written document holds the id-less result: two call occurrences
    sharing the id were outstanding, so the result could belong to either and the inference had no call to
    name
    selection_test.go:3010: claude: unpaired_tool_results = 0, want the one result no single call answers
    selection_test.go:3014: claude: generated_call_ids = 1, want 0: the inference picked no call
    … (pi, omp, codex and opencode likewise)
exit=1
$ # F3: the pre-fix progress reading: a `running` event of A, call B, an id-less result (finding 3's shape)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters ./internal/ccc/
--- FAIL: TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters (0.00s)
    selection_test.go:2131: claude: the written document holds the id-less result: both calls on its path
    were outstanding (A's only recorded event is progress), so the inference had no single call to name
    … (pi, omp and codex likewise; opencode already refused the event)
exit=1
$ # M1: sourceLineAnswers' membership branch reverted (the flags return; S5-identityless)
$ go test -tags fts5 -count=1 -run TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    selection_test.go:1598: codex: the written document holds the identity-less record the read's line
    leaves off it   (the same for opencode, with inactive_branch_records = 0)
exit=1
$ # M2: sourceLineFlags' membership branch reverted (the trees read the flags again; S6-identityless)
$ go test -tags fts5 -count=1 -run 'TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine|TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor|TestSelectionKeepsAnInactiveBranchesRecordedAncestry' ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    selection_test.go:1576: claude: converting "2-3": err = <nil>, want ErrUnrepresentableLeaf
    (the same for pi and omp)
exit=1
$ # M3: the off-line registration keyed by identity again (t.pending.sourceID != ""; S6-identityless)
$ go test -tags fts5 -count=1 -run 'TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine|TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor' ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    selection_test.go:1576: claude: converting "2-3": err = <nil>, want ErrUnrepresentableLeaf
    (the same for pi and omp)
exit=1
$ # M4: the refusal's second name read from the child's position (S6-identityless's message half)
$ go test -tags fts5 -count=1 -run 'TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine|TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor|TestSelectionKeepsAnInactiveBranchesRecordedAncestry' ./internal/ccc/
--- FAIL: TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine (0.00s)
    selection_test.go:1584: claude: the refusal for "2-3" (… would be chained to the entry of c3
    (ordinal 3) …) does not name "the record at ordinal 2 (the read answered no identity for it)"
    (the same for pi and omp)
--- FAIL: TestSelectionChainsARecordWithoutAnIdentityToItsRetainedAncestor (0.00s)
    selection_test.go:1426: the refusal … does not name b2
--- FAIL: TestSelectionKeepsAnInactiveBranchesRecordedAncestry (0.00s)
    selection_test.go:1747: the refusal … does not name b2 (ordinal 1)
exit=1
$ # M5: each writer's occurrence count reverted to the id check (S2-idless-occurrences)
$ go test -tags fts5 -count=1 -run TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath ./internal/ccc/
--- FAIL: TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath (0.00s)
    selection_test.go:3005: claude: the written document holds the id-less result: two call occurrences
    sharing the id were outstanding …   (pi, omp, codex and opencode likewise)
exit=1
$ # M6: each reduced writer's outcome gate removed (progress marks the call answered again; S7-writers)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters ./internal/ccc/
--- FAIL: TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters (0.00s)
    selection_test.go:2131: claude: the written document holds the id-less result …   (pi, omp and codex
    likewise)
exit=1
$ # M7: OpenCode's progress branch disabled (the event read as an outcome; S7-writers' OpenCode arm)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters ./internal/ccc/
--- FAIL: TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters (0.00s)
    selection_test.go:2131: opencode: the written document holds the id-less result …
    selection_test.go:2147: opencode: nonterminal_tool_events = 0, want the one recorded progress event
    the harness's tool states cannot hold
exit=1
```

**R-37 — an identity-less off-line record could revive through the fallback (P1), fixed.** The read's
line flags mark a record off the line only where the read holds an identity for it, and the writers'
off-line registration was keyed by the identity beside it, so the window `2-3` over the identity-less-leaf
fixture -- the abandoned branch's identity-less child and the line's own leaf, with neither record's
ancestry retained -- wrote the leaf chained under the child on the trees (the target's loader walked
`3 → 2` and would resume the branch the source's conversation left) and wrote the child into Codex's and
OpenCode's line. `sourceLineAnswers` and `sourceLineFlags` read the read's own walk membership per record
position (`Conversation.sourceLineHolds`) for a document a selection produced, the off-line registration
and the revival pair are keyed by that position (`offLineEntry`, `revivedParent`), and the refusal names
both records by position (`recordDescription`; the identity-keyed `recordWithOrdinal` became dead and was
deleted). The flagless path keeps the flags' long-standing answer (D4; §4's probe below is silent over
1109 shared lines). `TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine` gains the `2-3` arm (the
tree refusal naming ordinal 2 and ordinal 3, the line targets' omission with `inactive_branch_records`
1), and the four mutations M1-M4 fail it -- including the argument swap M4, which shows the refusal's
second name is what three cases pin.

**R-38 — two occurrences sharing an id were one eligible call (P1), fixed.** Claude's and Pi/OMP's
`soleResultCall` rejected a second eligible occurrence only when its id differed (`found != "" &&
found != id`), and Codex's and OpenCode's pending sets are keyed by id: a read recording call A, call B
and an id-less result -- both calls unanswered and both on the result's written path -- had one eligible
"call", so the result was written under it (Pi/OMP taking the tool name of whichever occurrence map order
reached last, OpenCode's `calls` map holding the later write). Each reduced inference now requires exactly
one eligible *occurrence*, whatever ids they carry, and the ambiguous result is omitted and counted;
Codex's `writtenCalls` becomes occurrence-to-id (`map[callOccurrence]string`) with `answeredCalls` beside
it, and OpenCode's plan answers through `soleOutstandingPlannedCall`. The reused-id arm of
`TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath` pins all five routes and the unflagged
contrast (the ledger's id-keyed set still writes the result -- D4), and the M5 mutation fails it on all
five routes.

**R-39 — progress closed a pending call in the reduced writers (P1), fixed.** The selection's own count
reads a `running` event as progress rather than an outcome, but Claude and Pi/OMP appended every accepted
result to `answeredCalls` and Codex deleted its id from `pendingCalls`, so a later id-less result was
inferred onto a second call although both were outstanding where it stood. The reduced path now records a
call's completion only where `resultStatesAnOutcome` says so -- the event itself still travels as the
recorded evidence it is, and the flagless path keeps the ledger's answer (D4). OpenCode's plan already
refused progress as an outcome (`nonterminal_tool_events`); M7 shows the new case pins that too. The new
`TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` pins the omission, the counts, the progress
event's travel on the routes whose format holds it, and the unflagged contrast; the M6 mutation fails it
on claude, pi, omp and codex.

**R-40 — the D3 fixed-point closure had no citation row (P3), fixed.** The D3 rows cited
`TestSelectionCarriesTheCounterpartOfASeparatedCallAndResult` and the strict case, which exercise
isolated pairs a single forward pass carries correctly; `TestSelectionClosesTheCounterpartGraphToAFixedPoint`
(`2 → 1 → 0`, each addition named with its puller) is now cited as `D3-fixedpoint`, so the required
`citations` step re-derives the transitive rule. The manifest gains four more rows for this round's
refinements (`S5-identityless`, `S6-identityless`, `S2-idless-occurrences`, `S7-writers`) and the round's
header block.

The round's verification: the two packages' Go cases run green (below), the D4 probe re-ran over the two
trees (`git worktree add --detach /tmp/ccc-d4-base 9242e49b…`, the retained probe revision copied into
both): 1109 / 1234 lines, each tree's two runs identical, the shared-document diff silent and the
125-line increment fixture absent from the base run -- §4's numbers reproduced with the round's changes,
so no number in §4 moved. The gate below is this round's own run -- `GATE: PASS`, exit 0, with
`0009-citations.tsv` grown 37 → **42** rows. 451 → **452** top-level `Test…` functions in the
`./cmd/ccc/... ./internal/ccc/...` test files (counted at HEAD and in this tree): the round adds one case
(`TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters`) and extends
`TestSelectionRefusesAnIdentityLessLeafChainedOffTheLine` and
`TestSelectionInfersAnIdLessResultOnlyToACallOnItsWrittenPath`; none deleted.

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	18.695s
ok  	go.kenn.io/agentsview/cmd/ccc	5.193s
$ python3 scripts/gate.py; echo "exit=$?"
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.967s (rc=0)
  PASS     parity   carried: 5553 file(s) identical to the pinned revision
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 42 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns
GATE: PASS
exit=0
```

### The ninth review round's findings (`1f6e7be`), fixed and re-measured

The review of `9242e49..1f6e7be` found four items (two P1, two P2): a reduced document wrote two call
occurrences of one recorded id under that id, so an output inferred for the earlier occurrence read back
under the later one -- and the round-trip check, comparing ids and texts, could not see it (finding 1); a
result the conversion omits (its text is whitespace only) still left the reduced ledger, so a later id-less
result was inferred onto a second call although neither had an outcome in the document (finding 2); a
summary the read stored on an id-less call was inferred off the outstanding set, so with two outstanding
calls an output whose owner was already known was dropped (finding 3); and the flat targets recomputed
`unresolved_source_parents` over the window alone, reporting a parent the caller had merely excluded as one
the read could not place (finding 4). This round fixes all four and follows the review's systematic
close-out: every rule the touched surface claims was proved by **one executed mutation** -- the rule's
line reverted, the case that claims to pin it run, the failure recorded, the line restored -- and every
rule the class reaches was pinned, so the four writers' minting lines each have their own mutation. Nine
lines in all, across `M1`-`M9`; `grep -rn 'false &&\|&& false\|true ||' internal/ccc/*.go` is empty after
the restores:

```console
$ # M1: Codex's colliding-id case disabled (case false && …; finding 1, S2-colliding-ids)
$ go test -tags fts5 -count=1 -run TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs ./internal/ccc/
--- FAIL: TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs (0.10s)
    selection_test.go:1600: codex: both call occurrences are written under "toolu_reused": the target
    pairs a call with its output by id, so the earlier occurrence's inferred output would read back under
    the later one
    selection_test.go:1623: codex: colliding_call_ids = 0, want the one occurrence written under a
    generated id
    selection_test.go:1673: the read-back attaches the outputs to calls "toolu_reused"/"toolu_reused",
    want one each: the inferred output belongs to the earlier occurrence and the recorded one to the
    later (attachments map[toolu_reused:[the first result the late result]])
exit=1
$ # M2: the same line in the Claude writer (S2-colliding-ids)
$ go test -tags fts5 -count=1 -run TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs ./internal/ccc/
--- FAIL: TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs (0.20s)
    selection_test.go:1600: claude: both call occurrences are written under "toolu_reused": …
    selection_test.go:1623: claude: colliding_call_ids = 0, want the one occurrence written under a
    generated id
exit=1
$ # M3: the same line in the Pi family writer (serves Pi and OMP; S2-colliding-ids)
$ go test -tags fts5 -count=1 -run TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs ./internal/ccc/
--- FAIL: TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs (0.14s)
    selection_test.go:1600: pi: both call occurrences are written under "toolu_reused": …
    selection_test.go:1623: pi: colliding_call_ids = 0, …
    selection_test.go:1600: omp: both call occurrences are written under "toolu_reused": …
    selection_test.go:1623: omp: colliding_call_ids = 0, …
exit=1
$ # M4: OpenCode's minting condition disabled (else if false && …; S2-colliding-ids)
$ go test -tags fts5 -count=1 -run TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs ./internal/ccc/
--- FAIL: TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs (0.12s)
    selection_test.go:1600: opencode: both call occurrences are written under "toolu_reused": …
    selection_test.go:1623: opencode: colliding_call_ids = 0, …
exit=1
$ # M5: the round-trip comparison's owner annotation dropped (if true || !known …; S2-owning-call)
$ go test -tags fts5 -count=1 -run TestCodexRoundTripAssertsTheOwningCallOfEachResult ./internal/ccc/
--- FAIL: TestCodexRoundTripAssertsTheOwningCallOfEachResult (0.00s)
exit=1
$ # M6: Codex's reduced ledger update restored before the text guards (S7-written-outcomes)
$ go test -tags fts5 -count=1 -run TestSelectionAnswersOnlyForTheOutcomesItWrites ./internal/ccc/
--- FAIL: TestSelectionAnswersOnlyForTheOutcomesItWrites (0.00s)
exit=1
$ # M7: the summary case falling back to the inference (case … && callID != ""; S2-summary-owner)
$ go test -tags fts5 -count=1 -run TestSelectionAnswersASummaryFromItsOwningOccurrence ./internal/ccc/
--- FAIL: TestSelectionAnswersASummaryFromItsOwningOccurrence (0.00s)
    selection_test.go:1517: codex: the document holds no result call_ccc_0_0 "the id-less call's own
    outcome": the id-less call's summary must be written under the id that occurrence stands under, not
    inferred onto the other call
    selection_test.go:1522: codex: unpaired_tool_results = 1, want 0: both summaries' owners were known
    selection_test.go:1533: codex: generated_call_ids = 1, want 2
exit=1
$ # M8: OpenCode's id-less inference reading the planned slot again (soleInferablePlannedCall →
$ #     soleOutstandingPlannedCall; S7-written-outcomes)
$ go test -tags fts5 -count=1 -run TestSelectionAnswersOnlyForTheOutcomesItWrites ./internal/ccc/
--- FAIL: TestSelectionAnswersOnlyForTheOutcomesItWrites (0.00s)
    selection_test.go:1401: opencode: the written document holds the id-less result although no written
    result answered either call: "result toolu_b the late output"
    selection_test.go:1406: opencode: unpaired_tool_results = 0, want the one id-less result neither
    call's written path answers
    selection_test.go:1410: opencode: generated_call_ids = 1, want 0: the inference picked no call, so no
    id was minted for the result
exit=1
$ # M9: sourceLineAnswers' reduced branch re-walking the window (finding 4, S6-read-answers)
$ go test -tags fts5 -count=1 -run TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer ./internal/ccc/
--- FAIL: TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer (0.00s)
    selection_test.go:1275: codex: window 0,5 counts unresolved_source_parents = {… Count:1 …}, want 0:
    the leaf's parent (ordinal 1) is excluded but the read's chain reaches the carried root, so the place
    is the read's own answer
    selection_test.go:1275: opencode: window 0,5 counts unresolved_source_parents = {… Count:1 …}, want 0
exit=1
```

The round also re-ran §4's probe revision unchanged over the two trees (`git worktree add --detach
/tmp/ccc-d4-before 9242e49b…`, the retained probe copied in): 1109 / 1234 lines, each tree's two runs
identical, the shared-document diff silent and the 125-line increment fixture absent from the base run --
§4's numbers reproduced with the round's changes. The probe caught one thing the first revision of the
owner annotation would have moved: the comparison first annotated *every* result (deriving an owner where
the writer states none), and the off-path Claude pipeline arm's already-`not-verified` detail gained a
result-attachment clause (`report digest` moved, folded 18857 → 19603). The annotation is therefore gated
on the document *stating* an owner (`TargetItem.OwnerCall`, which only a reduced document's writers set):
a document that states none keeps the id-and-text comparison, the shared diff is silent again, and the
assertion is active exactly where the finding put it. The gate is recorded here rather than in §4 because
it is this round's finding that needed it.

The round's verification: the two packages' Go cases run green (below), the D4 probe re-ran over the two
trees (§4's numbers reproduced, above), and the gate below is this round's own run -- `GATE: PASS`, exit 0,
with `0009-citations.tsv` grown 42 → **47** rows. 452 → **457** top-level `Test…` functions in the
`./cmd/ccc/... ./internal/ccc/...` test files: the round adds five cases
(`TestSelectionCountsOnlyThePlacesTheReadCouldNotAnswer`,
`TestSelectionAnswersOnlyForTheOutcomesItWrites`, `TestSelectionAnswersASummaryFromItsOwningOccurrence`,
`TestSelectionWritesCollidingCallOccurrencesUnderDistinctTargetIDs`,
`TestCodexRoundTripAssertsTheOwningCallOfEachResult`) and extends
`TestSelectionPairsAResultByItsRecordedAncestry` (its Claude arm now asserts the emitted ids and the
attachment instead of the recorded id on the later occurrence, the round's own contract change) and
`TestContinueWithoutASelectionReportsNoSelection` (the whole-read selection's report now holds the two
reduction-scoped rows); none deleted. The five new rows
(`S2-colliding-ids`, `S2-owning-call`, `S7-written-outcomes`, `S2-summary-owner`, `S6-read-answers`) each
cite a test that exists in `./internal/ccc` and passes. The round's first gate run failed `gofmt` alone
(`internal/ccc/opencode_target.go`, `internal/ccc/selection_test.go` — field alignment and a comment block
the round's edits had left unaligned); `gofmt -w` on the two files was the only change, and the run below
is the final tree's own -- taken after this record's edits, with the two packages also run on their own
first:

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	17.953s
ok  	go.kenn.io/agentsview/cmd/ccc	5.484s
$ python3 scripts/gate.py; echo "exit=$?"
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.663s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 47 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

(`parity`'s detail line is elided here as §6 elides it; the numbers in the elision are the ones it holds.)

### The tenth review round's findings (`0d213e5`), fixed and re-measured

The review of `9242e49..0d213e5` found four items (two P1, two P2), all fixed: the OpenCode plan inferred
a later id-less result onto a call whose only recorded outcome is the writable `ResultContent` summary the
summary pass was about to install, replacing the recorded outcome (finding 1); the selected two-file
`apply_patch` Codex conversion failed the structural check although the file held exactly the invocation
written, because the comparison counted the reader's per-file projection rows as calls (finding 2, with
the omitted test counting written envelopes and never running the read-back); the round's unconditional
`freshCallID` moved an unflagged conversion's bytes for an id-less call whose positional spelling an
earlier recorded call already held (finding 3, a D4 break the probe's fixture set could not see); and the
owner lookup found a call by file order, so a tree conversion whose result entry was reparented to a
sibling verified (finding 4). §4's revision is updated in this round (the colliding-id transcript,
§4 above); the writers', the checker's and the inference's rules are named in §5's cells and in the plan's
item 16, and the row of rules this round touches each has one executed mutation below -- the line
reverted, the case that claims to pin it run, the failure recorded, the line restored;
`grep -rn 'if true\|if false\|&& false\|false &&\|true ||' internal/ccc/*.go` is empty after the
restores:

```console
$ # M1: OpenCode's summary-only answer removed (c.plannedOutcomeWritable → openCodeOutcomeWritable; S7-summary-outcome)
$ go test -tags fts5 -count=1 -run TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall ./internal/ccc/
--- FAIL: TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall (0.00s)
    selection_test.go:1574: the selected document does not hold the call's own recorded summary: the id-less result took the slot the summary was owed
exit=1
$ # M2: the summary's writability loosened to its status alone (the text guard dropped; S7-summary-outcome)
$ go test -tags fts5 -count=1 -run TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall ./internal/ccc/
--- FAIL: TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall (0.00s)
    selection_test.go:1628: the selected document does not hold the id-less result: a whitespace-only summary is no outcome the target can write, so the call was still unanswered
    selection_test.go:1632: blank-summary generated_call_ids = 0, want the one inferred result
exit=1
$ # M3: the default path made summary-aware (the reduced rule added to the ledger branch; S7-summary-outcome's D4 arm)
$ go test -tags fts5 -count=1 -run TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall ./internal/ccc/
--- FAIL: TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall (0.00s)
    selection_test.go:1596: the unflagged document does not hold the id-less result: the ledger's id-keyed outstanding set is the default path's answer
    selection_test.go:1600: the unflagged document holds the summary the id-less result displaced: the default path's bytes are what a run without a selection has always written
    selection_test.go:1604: unflagged generated_call_ids = 0, want the one inferred result
exit=1
$ # M4: Codex's generated-id walk made unconditional again (if b.selectionReduced → if true; S2-positional-ids)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall ./internal/ccc/
--- FAIL: TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall (0.00s)
    selection_test.go:1862: codex: the unflagged conversion writes map[call_ccc_1_0:1 call_ccc_1_0_2:1 ccc1_0:1 toolu_ccc_1_0:1] under "call_ccc_1_0", want the recorded call and the plain positional spelling for the id-less call
exit=1
$ # M5: the reduced path's walk removed (if b.selectionReduced → if false; S2-positional-ids' reduced arm)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall ./internal/ccc/
--- FAIL: TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall (0.00s)
    selection_test.go:1878: codex: the selected conversion writes map[call_ccc_1_0:2 ccc1_0:1 toolu_ccc_1_0:1], want the recorded call under "call_ccc_1_0" and the id-less call walked past it
exit=1
$ # M6: the invocation units bypassed (codexCallEvidence handed nil; S2-invocation-units)
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.16s)
    selection_test.go:2905: the selected codex conversion verifies as not-verified: … tool call 2 is in the file but was not written …
    selection_test.go:2919: the unflagged codex conversion verifies as not-verified: … tool call 3 … tool call 4 …
exit=1
$ # M7: every id collapsed to one unit (the emitted dedupe added; S2-invocation-units' negative arm)
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.67s)
    selection_test.go:2919: the unflagged codex conversion verifies as not-verified: … tool call 2 is in the target document but not in the file …
    selection_test.go:2925: the comparison accepted a document claiming one invocation against a file holding two actual `function_call` envelopes
exit=1
$ # M8: Pi/OMP ancestry cleared (reader.ownersByAncestry = true → false; S2-owner-ancestry)
$ go test -tags fts5 -count=1 -run TestSelectionReportsAResultWhoseAncestryLeavesItsCall ./internal/ccc/
--- FAIL: TestSelectionReportsAResultWhoseAncestryLeavesItsCall (0.88s)
    selection_test.go:1955: pi: the comparison verified a result whose own parent walk cannot reach its invocation: it resolved the owner by id order, not by the relation the file records
    selection_test.go:1955: omp: the comparison verified a result whose own parent walk cannot reach its invocation: …
exit=1
$ # M9: Claude's ancestry cleared (roundTripReader{… ownersByAncestry: true → false}; S2-owner-ancestry's Claude arm)
$ go test -tags fts5 -count=1 -run TestSelectionReportsAResultWhoseAncestryLeavesItsCall ./internal/ccc/
--- FAIL: TestSelectionReportsAResultWhoseAncestryLeavesItsCall (0.37s)
    selection_test.go:1955: claude: the comparison verified a result whose own parent walk cannot reach its invocation: …
exit=1
```

The round re-ran §4's probe revision (its new input included) over the two trees
(`git worktree add --detach /tmp/ccc-d4-r9-before 9242e49b…`, the retained probe copied in): 1183 / 1307
lines, each tree's two runs identical, the shared-document diff holding exactly the codex patch fixture's
three verification lines (finding 2's fix: `not-verified` → `verified`, the warning gone, the report
digest with them; the artifact lines and every other shared line identical), and the colliding-id
transcript's 74 lines equal on both trees. The pre-fix head's mutant run (the same probe revision on
`0d213e5`) differs from the pre-0009 tree in exactly that document's 32 lines and nothing else — §4
quotes both. OpenCode's five lines are equal on all three trees, the finding's own scope note measured.

The round's verification: the two packages' Go cases run green (below), the D4 probe re-ran over the two
trees (§4's numbers, above), and the gate below is this round's own run -- `GATE: PASS`, exit 0, with
`0009-citations.tsv` grown 47 → **51** rows. 457 → **460** top-level `Test…` functions in the
`./cmd/ccc/... ./internal/ccc/...` test files: the round adds three cases
(`TestSelectionDoesNotInferAnIDLessResultOntoASummaryOnlyCall`,
`TestSelectionKeepsTheUnflaggedPositionalIDForAnIDLessCall`,
`TestSelectionReportsAResultWhoseAncestryLeavesItsCall`) and extends two
(`TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` now runs the structural check on
the selected and unflagged patch conversions and holds the negative arm; the F1 case gained the
blank-summary and default-path arms); none deleted. The four new rows (`S2-positional-ids`,
`S2-invocation-units`, `S7-summary-outcome`, `S2-owner-ancestry`) each cite a test that exists in
`./internal/ccc` and passes. The first `gofmt -l` after the round's edits named
`internal/ccc/selection_test.go` alone (one field alignment); `gofmt -w` on it was the only change:

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	34.885s
ok  	go.kenn.io/agentsview/cmd/ccc	13.707s
$ python3 scripts/gate.py; echo "exit=$?"
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.705s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 51 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

The throw-away probe file and the three worktrees (`before`, `mutant`, and this round's copy inside the
working tree) were removed before the gate run; `git status` lists the round's own files and no
probe.

### The eleventh review round's findings (`88478b6`), fixed and re-measured

The review of `9242e49..88478b6` found five items (two P1, three P2), all fixed: OpenCode's plan let an
id-less result take the one slot a call's written recorded result was owed, so a window that kept both
halves of a recorded pair wrote the source's words as an outcome it never recorded them for and counted
the recorded result as a repeat (finding 1); the Codex invocation-unit fold ran unconditionally, so the
committed two-file `apply_patch` fixture's *unflagged* conversion moved from the verdict its revisions
before the fold answered to `verified` -- a report, warning and CLI exit status change on the default path,
contrary to D4 (finding 2); the fold replaced every row under an id its `function_call` units stated, so a
`custom_tool_call` appended under that id produced no difference at all (finding 4); the owner walk stopped
at a uuid the Claude reader's merge had removed from the read-back, reporting a correct conversion although
the installed chain reached the invocation (finding 3); and the reduced writers counted a `running` event
as progress but still wrote its text as the call's output, which these formats read back as the call's
answer (finding 5). §4's numbers are re-measured in this round (the shared-document diff is silent again);
the writers', the checker's and the inference's rules are named in §5's cells and in the plan's item 17,
and every rule this round touches has one executed mutation below -- the line reverted, the case that
claims to pin it run, the failure recorded, the line restored. One condition did not survive its own
mutation -- the reservation's separate progress test, whose answer is `openCodeResultWritable`'s already,
because the two status vocabularies are disjoint -- and it was deleted rather than kept unpinned (the
progress arm stays and is what `M3` fails on):

```console
$ # M1: the reservation's candidate rule reverted (the reservedSlots test dropped from the candidate arm; S7-reserved-slot)
$ go test -tags fts5 -count=1 -run TestSelectionReservesARecordedResultsSlotFromAnIdLessInference ./internal/ccc/
--- FAIL: TestSelectionReservesARecordedResultsSlotFromAnIdLessInference (0.00s)
    selection_test.go:1724: the recorded pairing's window: the written document holds the id-less result: the recorded result's slot was reserved, so an inference may not take it ("result toolu_second the id-less output")
    selection_test.go:1724: the recorded pairing's window: the written document does not hold the recorded result under the id the occurrence it answers stands under: the inference displaced a recorded pairing whose two halves the window carried
    selection_test.go:1724: the recorded pairing's window: unpaired_tool_results = 0, want the one id-less result no call it could be inferred onto answers
exit=1
$ # M2: a reserved occurrence treated as answered (skipped instead of counted; S7-reserved-slot's ambiguity arm)
$ go test -tags fts5 -count=1 -run TestSelectionReservesARecordedResultsSlotFromAnIdLessInference ./internal/ccc/
--- FAIL: TestSelectionReservesARecordedResultsSlotFromAnIdLessInference (0.00s)
    selection_test.go:1741: the ambiguous window: the written document holds the id-less result: the recorded result's slot was reserved, so an inference may not take it ("result toolu_third the id-less output")
    selection_test.go:1741: the ambiguous window: unpaired_tool_results = 0, want the one id-less result no call it could be inferred onto answers
    selection_test.go:1741: the ambiguous window: generated_call_ids = 1, want 0: nothing was inferred
exit=1
$ # M3: the reservation's writability guard removed (!openCodeResultWritable; S7-reserved-slot's blank control and conditions case's progress arm)
$ go test -tags fts5 -count=1 -run TestSelectionReserves ./internal/ccc/
--- FAIL: TestSelectionReservesARecordedResultsSlotFromAnIdLessInference (0.00s)
    selection_test.go:1770: blank-outcome generated_call_ids = 0, want the one inferred result
--- FAIL: TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite (0.00s)
    selection_test.go:1856: a progress event: the written document holds no call for the occurrence
exit=1
$ # M4: the reservation's line check removed (inactive[place.message]; S7-reserved-slot's plan-writes conditions)
$ go test -tags fts5 -count=1 -run TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite ./internal/ccc/
--- FAIL: TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite (0.00s)
    selection_test.go:1856: a record the read's line leaves off: the written document holds no call for the occurrence
exit=1
$ # M5: the reservation's subagent check removed (message.IsSidechain; the same conditions)
$ go test -tags fts5 -count=1 -run TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite ./internal/ccc/
--- FAIL: TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite (0.00s)
    selection_test.go:1856: a subagent's record: the written document holds no call for the occurrence
exit=1
$ # M6: the reservation's compaction check removed (message.IsCompactBoundary; the same conditions)
$ go test -tags fts5 -count=1 -run TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite ./internal/ccc/
--- FAIL: TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite (0.00s)
    selection_test.go:1864: a compaction boundary record: the written document holds the event the reservation was about ("result toolu_slot the event whose slot would have been reserved"): a result this plan does not write is not the outcome the part is owed
    selection_test.go:1869: a compaction boundary record: the written document does not hold the id-less result under the call's own id: a result the plan does not write reserved the slot an inference could have filled
    selection_test.go:1874: a compaction boundary record: generated_call_ids = 0, want the one inferred result
exit=1
$ # M7: the reservation keyed by the result's own place instead of the paired occurrence (S7-reserved-slot's pairing condition)
$ go test -tags fts5 -count=1 -run TestSelectionReservesARecordedResultsSlotFromAnIdLessInference ./internal/ccc/
--- FAIL: TestSelectionReservesARecordedResultsSlotFromAnIdLessInference (0.00s)
    selection_test.go:1724: the recorded pairing's window: the written document holds the id-less result: the recorded result's slot was reserved, so an inference may not take it ("result toolu_second the id-less output")
    selection_test.go:1724: the recorded pairing's window: the written document does not hold the recorded result under the id the occurrence it answers stands under: the inference displaced a recorded pairing whose two halves the window carried
exit=1
$ # M8: the Codex fold ungated (the reader installs codexCallEvidence unconditionally; S2-invocation-units' D4 scoping)
$ go test -tags fts5 -count=1 -run TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers ./internal/ccc/
--- FAIL: TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers (0.12s)
    selection_test.go:3258: the unflagged codex conversion verifies as verified, want the not-verified verdict every revision before the fold answered: the fold is the reduction's, and a run without a selection keeps the comparison, the verdict and the exit status it has always had
    selection_test.go:3264: the unflagged detail = "", want the reader's per-file rows reported as calls the document did not write
    selection_test.go:3270: the unflagged comparison folded the reader's per-file rows into the file's records: the invocation-unit fold belongs to a document a selection produced
exit=1
$ # M9: the fold's attribution rule reverted (codexFoldableRows → the id-level count; S2-fold-attribution)
$ go test -tags fts5 -count=1 -run TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute ./internal/ccc/
--- FAIL: TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute (0.45s)
    codex_install_test.go:251: differences = ["tool call 1 differs (written call call_02Example000000000000000000 to apply_patch (128 bytes, sha256 552563bd…), read back call  to  (0 bytes, sha256 e3b0c442…))" "tool call 2 is in the file but was not written: call call_02Example000000000000000000 to apply_patch (128 bytes, sha256 552563bd…)"], want the invocation the file's own `custom_tool_call` states and the document does not: the fold may discard only the rows the file's own records account for
exit=1
$ # M10: the fold's unit scan restricted to the writer's own envelope (the custom_tool_call/image_generation_call cases dropped; S2-fold-attribution's same-key arm)
$ go test -tags fts5 -count=1 -run TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute ./internal/ccc/
--- FAIL: TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute (0.09s)
    codex_install_test.go:278: differences = [], want the second call record reported although its reader's rows hold the id, the name and the input the document's own call holds: the file states two records and the document one
exit=1
$ # M11: the installed chain not read by the owner walk (the Claude reader's entryParents cleared; S2-owner-merged-entry)
$ go test -tags fts5 -count=1 -run TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord ./internal/ccc/
--- FAIL: TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord (0.56s)
    selection_test.go:2288: the conversion verifies as not-verified: the installed session does not hold the evidence in the target document: tool result 1 differs (written call toolu_merged_a's result (10 bytes, sha256 e77c830a…) written for the invocation of call toolu_merged_a to Bash, read back call toolu_merged_a's result (10 bytes, sha256 e77c830a…)); the result's own installed ancestry reaches the invocation it states, through the entry the reader merged away
exit=1
$ # M12: the selected progress guard removed (selectionReduced && !resultStatesAnOutcome; S7-writers)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters ./internal/ccc/
--- FAIL: TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters (0.00s)
    selection_test.go:3416: codex: the written document holds the recorded progress event as the call's output: the format carries an outcome or nothing, so the words would read back as the call's answer
    selection_test.go:3423: codex: nonterminal_tool_events = 0, want the one recorded progress event the format cannot carry as progress
exit=1
$ # M13: the progress guard's scoping removed (b.selectionReduced dropped; S7-writers' default arm)
$ go test -tags fts5 -count=1 -run TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters ./internal/ccc/
--- FAIL: TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters (0.00s)
    selection_test.go:3455: codex: the unflagged conversion drops the recorded progress event: a run without a selection keeps writing it, which is the answer its writer has always had
exit=1
```

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	19.978s
ok  	go.kenn.io/agentsview/cmd/ccc	6.418s
$ python3 scripts/gate.py; echo "exit=$?"
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.881s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es); 5727 file(s) seen by git)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 54 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

The round re-ran §4's probe revision over the two trees (its numbers are §4's): 1183 / 1308 lines, each
tree's two runs identical, **the shared-document diff silent over all 1183 lines** -- the flag the ninth
round's revision of §4 recorded moving is back to the pre-0009 tree's own, so the default path's written
bytes, its whole report, its structural verdict, its warnings and the CLI exit status are again what they
were before the fold existed.

The round's verification: the two packages' Go cases run green (above), the D4 probe re-ran over the two
trees (§4's numbers), and the gate above is this round's own run -- `GATE: PASS`, exit 0, with
`0009-citations.tsv` grown 51 → **54** rows. 460 → **464** top-level `Test…` functions under
`./cmd/ccc/... ./internal/ccc/...`: the round adds four cases
(`TestSelectionReservesARecordedResultsSlotFromAnIdLessInference`,
`TestSelectionReservesNoSlotForAResultThePlanDoesNotWrite`,
`TestClaudeRoundTripResolvesAResultOwnerThroughAMergedAssistantRecord`,
`TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute`) and extends four
(`TestSelectionCountsTheCallUnitsTheWritersCoalesceAndThePairingAnswers` gains the unflagged comparison,
verdict and detail arms; `TestSelectionKeepsProgressOnlyCallsOutstandingForTheWriters` now requires the
omission and its count on all five routes plus the absent row and the event's continued travel on the
default path; `TestCodexRoundTripReportsACallRecordTheFoldCannotAttribute` gained the same-key arm; the
CLI's whole-read arm names the third reduction-only row); none deleted. The three new rows
(`S7-reserved-slot`, `S2-fold-attribution`, `S2-owner-merged-entry`) each cite a test that exists in
`./internal/ccc` and passes, and `S2-invocation-units`/`S7-writers` were rewritten to the rules this round
scoped. `gofmt -l ./cmd/ccc ./internal/ccc` named nothing after the round's edits.


### The twelfth review round's findings (`2e2e8a2` + working tree), fixed and re-measured

The review of `9242e49..2e2e8a2` found four items (two P1, two P2), all fixed; the review's carried-tree
parity section found all 31 changed paths project-owned and every carried blob matching the pin, and this
round changes no carried file either — the carried tree is untouched and `upstream-parity.py` is the gate
step that keeps saying so. The round follows the review's systematic close-out: every rule the touched
surface claims is proved by one executed mutation (`M1`–`M12` below), each mutation's observed failure
recorded with the test that catches it.

* **Finding 1 (P1) — a whitespace-only API selection published the whole read.**
  `CheckSelectionOptions` trimmed its value and read a trimmed-empty string as the flag's absence when the
  presence bit was unset, and `continueWith` tested the trimmed value as well, so an embedding caller that
  set `ContinueOptions.Selection` to `" \t "` without `SelectionSupplied` converted and installed the whole
  conversation with no selection report — the one thing the request was written to prevent. The flag's
  absence is now the literal empty string with no presence bit (`CheckSelectionOptions`:
  `supplied || selection != ""`), and `continueWith` enters the selection for any non-empty value, where
  `Select`'s own trim refusal answers a whitespace-only value instead of the conversion skipping the
  reduction. `TestSelectionRefusesAWhitespaceOnlyAPIRequest` pins both layers (four whitespace shapes, with
  and without the bit; `Continue` and `continueWith` each asserted to refuse `ErrBadSelection` with the
  target home empty, beside a well-formed control that installs) and the supplied-empty arm stays in
  `TestCheckSelectionOptionsRefusesASpecificationWithoutARead`; mutations `M1` and `M2` fail it.

* **Finding 2 (P1) — a selection could resume a tool-result branch the native Pi file left behind.** The
  copy's appended turn hangs off the *call's* entry, so the file's own leaf path is turn → call → … and the
  result entry stands beside the turn off that path — the shape a `/tree` switch back to the call leaves.
  The inherited reader answers the turn's parent as the call (a result's descendants resolve to their
  visible ancestor), so the read cannot tell that from a file whose turn hangs off the result; its
  reconstruction chained through the result and marked it on the read's line, `--select 0-2` passed the
  tree writers' leaf guard, and the written session's last entry stood for the result: a resumed target
  session would continue a branch the source's file left, and the source-side leaf check (which confirms the
  read's *last* record only) answered nothing. The conversion now reads the source file's own entries once,
  before the selection (`readPiSourceFile`; the source-side leaf check asks the same read, so the file is
  parsed once and the selection's refusals are still answered before the check's), and the file's own leaf
  path — the walk from its last entry back through `parentId`, the walk the harness's own loader builds the
  context from — is the line a selection's flags are taken from for every record the read answered an
  identity for (`Conversation.sourceFileLine`, consumed by `Select`). The writers' tool-result place no
  longer chains an on-line record through a result entry that stands for a record the line leaves off
  (`resultEntryLeavesTheLine`), so `--select 0-` writes the file's own walk. A record the read gave no
  identity for keeps the read's own order answer, an archive row and a file that cannot answer keep it, and
  a run without a selection keeps the reconstruction's line and its projection — the bytes D4 measures.
  `TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath` pins the refusal (Pi and Claude, naming the
  result's identity and ordinal, nothing installed), the line target's omission and count, the window
  ending on the file's own path, the selected whole read's model-visible history equalling the source
  file's own (with the projection's one remaining count), and the unflagged arm (projection taken, its own
  count); `TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities` pins the identity-less
  boundary on a Pi v1 file. Mutations `M3`–`M6` fail them.

* **Finding 3 (P2) — an unwritable event consumed a reserved OpenCode result slot.** The plan installed
  whatever result reached a call's one slot first, so on a reduced window holding a call, an id-less
  terminal output, a blank result naming the call and a later non-blank result naming it too, the blank
  event took the slot, the later recorded result was counted `repeated_tool_results`, and the part was
  dropped at write time when the stored text was read — no outcome, although one was representable (the
  reservation that kept the id-less inference off the occurrence had assumed the writable result was the one
  owed it). `displacingOpenCodeOutcome` lets a later writable recorded result take a slot an unwritable held
  result occupies on a reduced document, and `countUnwritableResult` counts the displaced event exactly as
  `toolPart` counts the same result (its unrepresentable blocks, then no text, whitespace-only text or a
  status the tool states cannot spell) so nothing is lost; a run without a selection keeps the first
  recorded outcome in the slot. `TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult` pins
  the finding's window for all three unwritable shapes, the categories, and the default path's unchanged
  answer; mutations `M7`–`M9` fail it.

* **Finding 4 (P2) — an id-less progress event could erase a recorded OpenCode summary.** The progress
  branch marked a call as having recorded result evidence through the same pairing a terminal event uses,
  which for an id-less event is that writer's own inference (`soleOutstandingPlannedCall` when the plan is
  reduced, the ledger otherwise), so a call whose only recorded outcome was its stored summary had the
  summary suppressed as a view of an event the source never linked to it and the part dropped as unpaired.
  The marking now reads the source's own link only (a non-empty `ToolUseID`), which restores 0007's
  id-keyed rule on the default path as well;
  `TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress` pins the finding's arm, the
  id-recorded control (the summary is the event's view and is not written) and the unflagged arms;
  mutations `M10`–`M12` fail it.

The round's mutations, each run against the round's tree with the named test, every one failing for the
reason its rule names and restored afterwards (the working tree's `git diff` after the round holds only the
round's own edits):

```console
$ # M1: the presence bit alone decides the flag's absence (CheckSelectionOptions' `supplied || selection != ""` → `supplied`)
$ go test -tags fts5 -count=1 -run TestSelectionRefusesAWhitespaceOnlyAPIRequest ./internal/ccc/
--- FAIL: TestSelectionRefusesAWhitespaceOnlyAPIRequest (0.02s)
    selection_test.go:4755: CheckSelectionOptions(" ", supplied=false): err = <nil>, want ErrBadSelection
exit=1

$ # M2: continueWith tests the trimmed value (`opts.Selection != ""` → `strings.TrimSpace(opts.Selection) != ""`)
$ go test -tags fts5 -count=1 -run TestSelectionRefusesAWhitespaceOnlyAPIRequest ./internal/ccc/
--- FAIL: TestSelectionRefusesAWhitespaceOnlyAPIRequest (0.04s)
    selection_test.go:4794: continueWith with a whitespace-only selection: err = <nil>, want ErrBadSelection: a value that names nothing is a refused request, never the flag's absence
exit=1

$ # M3: the file's line answer not carried (continueWith assigns nil)
$ go test -tags fts5 -count=1 -run TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath ./internal/ccc/
--- FAIL: TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath (0.01s)
    continue_pi_test.go:2201: pi: --select 0-2 = <nil>, want ErrUnrepresentableLeaf: the window ends on the tool result the source's file ran beside the branch the conversation continued on
exit=1

$ # M4: the membership's walk is not the file's leaf walk (every entry marked on the path)
$ go test -tags fts5 -count=1 -run TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath ./internal/ccc/
--- FAIL: TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath (0.11s)
    continue_pi_test.go:2201: pi: --select 0-2 = <nil>, want ErrUnrepresentableLeaf: the window ends on the tool result the source's file ran beside the branch the conversation continued on
exit=1

$ # M5: an identity-less record answered off the file's path (the `id == "" → continue` dropped)
$ go test -tags fts5 -count=1 -run TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities ./internal/ccc/
--- FAIL: TestContinueKeepsTheReadsOrderForAFileRecordingNoEntryIdentities (0.00s)
    continue_pi_test.go:2338: pi: --select 1: the source's own loader selects a leaf this conversion cannot write as the target's: the record at ordinal 1 (the read answered no identity for it), the record the entry written last stands for, is off the line the read's last record answers, so a resumed session would continue a branch the source's conversation left
exit=1

$ # M6: the projection taken onto an off-line result (resultEntryLeavesTheLine → false)
$ go test -tags fts5 -count=1 -run TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath ./internal/ccc/
--- FAIL: TestContinueRefusesASelectedWindowOffTheSourceFilesLeafPath (0.11s)
    continue_pi_test.go:2269: pi: the selected session's model-visible history = ["Where is the retry cap configured?" "I will look for the retry configuration." "internal/retry/config.go:41: maxBackoffSeconds = 30" "And the retry budget?"], want the source file's own ["Where is the retry cap configured?" "I will look for the retry configuration." "And the retry budget?"]: a resumed session must continue the source's own line, not the reconstruction's projection
exit=1

$ # M7: no displacement of an unwritable outcome (displacingOpenCodeOutcome → false)
$ go test -tags fts5 -count=1 -run TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult ./internal/ccc/
--- FAIL: TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult (0.00s)
    selection_test.go:4880: an empty-text result: the written document holds no outcome for the call: the unwritable event that stood before the writable one consumed the slot the writable record is owed
exit=1

$ # M8: the displaced result not counted (countUnwritableResult returns at once)
$ go test -tags fts5 -count=1 -run TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult ./internal/ccc/
--- FAIL: TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult (0.00s)
    selection_test.go:4884: an empty-text result: empty_tool_results = 0, want the one displaced event counted under the category the writer counts the same result under
exit=1

$ # M9: the displacement not scoped to the reduction (`selectionReduced() && ` dropped)
$ go test -tags fts5 -count=1 -run TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult ./internal/ccc/
--- FAIL: TestSelectionYieldsAnUnwritableOutcomesSlotToALaterRecordedResult (0.00s)
    selection_test.go:4970: the unflagged document holds the later recorded result ("result toolu_unwritable_slot the later recorded result"): the first recorded outcome keeps the slot on the default path, and its bytes are what a run without a selection has always written
exit=1

$ # M10: an id-less progress event marks the call (the id guard dropped)
$ go test -tags fts5 -count=1 -run TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress ./internal/ccc/
--- FAIL: TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress (0.00s)
    selection_test.go:5038: the selected document does not hold the call's own recorded summary: the id-less progress event suppressed it through a link the source never recorded ([])
exit=1

$ # M11: no progress event marks at all (`if false {` in the guard)
$ go test -tags fts5 -count=1 -run TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress ./internal/ccc/
--- FAIL: TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress (0.00s)
    selection_test.go:5061: the selected document states tool evidence ([call toolu_summary_progress Bash {"command":"a"} result toolu_summary_progress the call's own recorded outcome]): the read's own event for the call is progress, and the summary is that event's view rather than an outcome recorded beside it
exit=1

$ # M12: the marking scoped to the reduction (`&& c.conv.selectionReduced()` added)
$ go test -tags fts5 -count=1 -run TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress ./internal/ccc/
--- FAIL: TestSelectionWritesASummaryAnIDLessProgressEventDoesNotSuppress (0.00s)
    selection_test.go:5101: the unflagged document states tool evidence ([call toolu_summary_progress Bash {"command":"a"} result toolu_summary_progress the call's own recorded outcome]): the read's own event for the call is progress, and the summary is that event's view rather than an outcome recorded beside it
exit=1
```

The round re-ran §4's probe revision unchanged over the two trees
(`git worktree add --detach /tmp/ccc-d4-r11-before 9242e49b…`, the retained probe copied into both, no
selection flag in any run): 1183 / 1308 lines, each tree's two runs byte-identical, and the shared-document
diff **silent** — the 125 lines only this tree prints are the increment's own selection fixture
(`claude:c1a74e28…`), which the pre-0009 tree does not hold. So the default path's bytes, reports and
published artifacts are what the pre-0009 revision wrote, the fourth finding's restored marking and the
guards above included; the selected documents this round changes are selections, which the probe does not
run.

The round's verification: 464 → **469** top-level `Test…` functions in the two packages' test files, the
five new cases named above and none deleted; `0009-citations.tsv` grown 54 → **60** rows, the six new rows
(`D5-whitespace`, `S5-file-path`, `S6-file-projection`, `S5-file-identityless`, `S7-slot-writability`,
`S7-progress-link`) each citing a test that exists in `./internal/ccc` and passes (the citations step's own
answer, below). `gofmt -l` names nothing; the round's documentation moves are folded into §7.

```console
$ go test -tags fts5 -count=1 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	18.669s
ok  	go.kenn.io/agentsview/cmd/ccc	5.280s
$ python3 scripts/gate.py; echo "exit=$?"
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3627 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.801s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5727 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 60 row(s) verified
  PASS     secrets  no leaks found in 196 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
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
  condition to the id-less call's count. The sixth fix round scoped the writers' sentence to a reduced
  document (the default path keeps the id acceptance its writers always had, report included), added the
  id-less inference's written walk and the paired occurrence's tool name to the seam sentence, the Codex
  coalescing to the call-unit sentence, and `ErrSelectionReduced` to the refusal list. The seventh fix
  round sharpened the id-less inference's sentence (completion is per call occurrence and per written
  path: a recorded outcome on another branch does not clear the call for a result standing here) and the
  line sentence's leaf clause (the guard answers by the record's own position through the entries actually
  written, so a record the read gave no identity for is refused like any other). The eighth fix round
  extended the id-less inference's sentence to the call *occurrence* (two outstanding occurrences sharing
  an id are two calls, so an ambiguous result is omitted rather than bound to either) and the progress
  clause to the reduced writers' own outstanding set, and the line sentence to the per-position membership
  (a line target omits what the read's walk does not reach, identity-less records included, and the
  off-line registration the revival check reads is by that position too). The ninth fix round added the
  written-outcome condition and the summary's owner to the id-less inference's sentence, the colliding-id
  substitution beside it, the read-answered count to the line sentence's ancestry clause, and the
  round-trip check's owner assertion to the offline-half sentence (a written result's owning call is
  compared wherever the document states one). The tenth fix round extended the offline-half sentence to a
  reader that projects one call record into several rows (the calls are compared as the file's own record
  units, one record per invocation) and to a tree format's ancestry-resolved owner, and the id-less
  inference's sentence to the writable summary's answer and the reduction-scoped generated-id walk (a run
  without a selection keeps the plain positional spelling, a recorded id's duplicate included). The
  eleventh fix round added the reserved slot to the id-less inference's sentence (where a target's part
  holds one outcome per call, a recorded result the conversion writes reserves its call occurrence, while
  one it does not write -- an off-line, subagent or compaction-boundary record's, one whose text or status
  has no shape -- reserves nothing and a reserved occurrence still counts as unanswered), the progress
  event's omission to the progress clause (`nonterminal_tool_events`, scoped to the reduction as
  `coalesced_tool_call_projections` is, with the unflagged run keeping the event and its category list),
  and the reduction scoping of the invocation-unit fold plus the fold's attribution rule and the installed
  entry chain to the offline-half sentence. The twelfth fix round added the file-backed line rule to the
  line sentence (a Pi-family read whose own file stands behind it takes that file's own leaf path as the
  selection's line for the records the read answered an identity for, while an unreduced read keeps the
  reconstruction and its projection), the projection's yield to that sentence's own offline half (the
  tool-result place is not taken where the result's entry stands for a record the line leaves off while the
  record being written is one the line holds), the same writability rule to the reserved-slot sentence (an
  event the part cannot state is counted as the writer counts the same result and yields the slot to a
  later recorded outcome the part can state), the id-recorded marking to the progress clause (an id-less
  event, a link only that writer's inference makes, does not suppress the call's stored summary), and the
  whitespace-alone refusal to the refusal list (the flag's absence being the literal empty string with no
  presence bit).
* `docs/conventions.md` — the layout block names `selection.go`; a new rule under "Rules of the work"
  states the specification's grammar, the seam, the refusals and the report's classes, with the second
  round's refinements (order pairing, progress, ancestry, supplied blank) folded into the same rule, the
  third round's (the projection step and the position-keyed ancestry, the call unit the undischarged
  count reads, the refusal order) folded in beside them, and the fourth round's (the flat writers'
  occurrence pairing, the id-less call's count, the inactive ancestor and the counted projection, the
  duplicate-region refusal's grammar half) folded in as well, the fifth round's (the ancestry-first
  pairing, the tree writers' written-ancestry check, the summary-as-outcome-evidence and the
  states-an-outcome condition) folded in beside them, and the sixth round's (the reduction-scoped
  writers' disposition with the default path's report untouched, the id-less inference's written walk,
  paired occurrence's tool name, the Codex coalescing and its reduction-only category, and the
  `ErrSelectionReduced` refusal) folded in as well, and the seventh round's (the id-less inference's
  per-occurrence completion and the leaf guard's position-keyed answer) folded in beside them, and the
  eighth round's (the reduced line answer as the read's per-position membership for identity-less records,
  the occurrence-counted id-less inference, and the progress rule on the reduced writers' side) folded in
  as well, and the ninth round's (the colliding-id substitution and the round-trip owner assertion, the
  written-outcome ledger, the summary's owner, and the read-answered unresolved count) folded in beside
  them, and the tenth round's (the invocation-unit comparison and the ancestry-resolved owner assertion,
  the writable summary's written-path answer, and the reduction-scoped generated-id walk) folded in as
  well, and the eleventh round's (the reduction-scoped *verdict* of that comparison, the fold's
  attribution rule, the installed-chain owner walk, the reserved slot a written recorded result is owed,
  and the omitted progress event on the reduced writers' side) folded in beside them, and the twelfth
  round's (the file-backed Pi read's line as the source file's own leaf path with the unreduced
  reconstruction kept, the outcome slot's writability on a reduced document, the id-recorded progress
  marking, and the whitespace-alone refusal beside the supplied-empty one) folded in as well.
* `docs/plans/0009-selective-context.md` — §11 records the increment's numbered departures with the
  measurement behind each (the third round's four in §11.12) and the fourth through eleventh rounds' seven,
  five, six, three, four, five, four and five at its close (the seventh's as item 13, the eighth's as item
  14, the ninth's as item 15, the tenth's as item 16, the eleventh's as item 17, and the twelfth's four as
  item 18, with the twelve mutations that pin them in this record's round section).
* `docs/goals/README.md` — 0009's row reads *Implemented on `feat/0009-selective-context`; awaiting
  operator acceptance*, and the goal file's own `Status:` line reads the same (the form 0007 and 0008
  carried while their increments awaited acceptance); nothing else was ready to archive (0008 was archived
  by the activation increment that landed this plan, and 0010–0013 are provisional while 0014–0017 are
  ready-but-unstarted). The twelfth fix round re-checked the goal files for the same rule and found the
  same answer: 0009 awaits operator acceptance, 0014–0017 are Ready and unstarted, 0010–0013 provisional —
  nothing accepted was waiting for the archive, so no goal file and no index row moved.
* `docs/decisions/OPEN-QUESTIONS.md` — Q-0009-01 (the strict refusal's exit code); Q-0009-02 (rewritten:
  the seam pairs a result with the call the read's own ancestry establishes where it does, else the
  nearest prior call of the same id, an id-less record carries no link); Q-0009-03 (rewritten: a supplied
  blank value is refused, not read as the flag's absence); Q-0009-04 (the
  read's line is not the selection's to redefine, ancestry included — the third round's detail names the
  projection step and the position-keyed answer); Q-0009-05 (a command line whose selection the read's
  ordinals refuse is answered as that, with the usage exit code, before a source-side refusal the same
  command line would also meet); Q-0009-06 (rewritten this round: a result whose paired call is not
  on its written ancestry — a flat line that leaves the call off, a tree whose walk does not hold the
  pairing — is omitted and counted rather than refused, the default for both writer families and its
  reversal); Q-0009-07 (a selection over a document a selection already produced is refused by name, not
  composed — the reversal preserves the answers across a second reduction); Q-0009-08 (a reduced
  document's report carries the Codex writer's `coalesced_tool_call_projections` row and the default
  path's does not, with the whole-read-selection comparison naming that one row — the reversal makes the
  category unconditional and moves the probe's report lines); and Q-0009-09 (this round: a reduced
  document's id-less inference reads completion per call occurrence and per written path while the default
  path keeps the ledger's file-wide question, and a tree conversion refuses an identity-less leaf that the
  read's line does not hold — the reversals are a path-aware default and a passing identity-less leaf);
  and Q-0009-10 (this round: a reduced document's line answer is the read's walk membership per record
  position — identity-less records included — and its id-less inference counts call occurrences and
  outcome-stating answers, while the default path keeps the flags and the ledger's id-keyed set — the
  reversals return the reduced writers to those and reopen the round's mutations, or ask the same question
  on the flagless path and break D4); and Q-0009-11 (this round: a reduced document substitutes a colliding
  recorded id, asserts result ownership where a document states one, answers only for writeable outcomes,
  writes a summary for the call that stored it, and reads the flat targets' unresolved count from the
  read's own relation answers — the reversals are the pre-fix lines the round's nine mutations record, or
  the same substitution and assertion on the flagless path, which moves its bytes and report and breaks
  D4); and Q-0009-12 (this round: the Codex structural check reads the installed file's own
  `function_call` records as invocation units — its verdict for the two-file `apply_patch` fixture moves
  `not-verified` → `verified` while the written bytes do not — OpenCode's reduced id-less inference counts
  a writable summary-only outcome as answered, the generated-id walk stays reduction-scoped so the default
  path keeps the plain positional spelling, and a tree owner assertion reads the result's recorded
  ancestry; the reversals are the pre-fix lines the round's nine mutations record, or the same rules on
  the flagless path, which moves what the default path's report states about a file whose bytes it wrote);
  and Q-0009-13 (this round: the Codex fold is the reduction's own -- an unflagged run keeps the
  rows-against-items verdict, the warning, the report digest and the CLI exit status every revision before
  the fold answered -- OpenCode's reservation of the slot a written recorded result is owed, the reduced
  writers' omission of a progress event their formats cannot carry as progress (a row the default path's
  report does not hold) with the event's continued travel nowhere else, the fold's attribution rule (rows
  the file's own records do not account for keep standing, and a record type whose call key this build does
  not restate never folds) and the installed-chain owner walk past an entry the reader merged away; the
  reversals are the pre-fix lines this round's thirteen mutations record); and Q-0009-14 (this round: the
  file-backed Pi read's line is the file's own leaf path for a *selection* only, an identity-less record
  keeps the read's own order answer, and the OpenCode outcome slot's writability and the id-recorded
  progress marking are the reduction's own, with a run without a selection keeping the reconstruction's
  line, its projection and its ledger's answers — the reversals are the pre-fix lines the round's twelve
  mutations record, while carrying the file's answer to the default path would move its bytes, counts and
  omissions and break what D4 measures).
* `README.md` — an example line (`--select 0-3,7`, in range for its fixture), a paragraph on the flag and
  its report, and `Select` in the engine list. Unchanged by the third and fourth rounds: the paragraph's
  claims (the order pairing, the progress rule, the blank refusal, the ancestry rule) are still what the
  code does. The fifth round rewrote the paragraph sentence that names the pairing rule (the ancestry
  first, the nearest prior occurrence of the id otherwise); its other claims stand. The sixth round scoped
  the paragraph's written-ancestry sentence to the runs that carry a selection, added the id-less
  inference's written walk and named the second-selection refusal. The seventh round extended the id-less
  sentence (the eligible call must also be unanswered on that walk) and the tree-leaf sentence (the
  refusal answers by record position, an identity-less end included). The eighth round extended the id-less
  sentence (the eligible occurrence must also have an unanswered walk, two occurrences sharing an id are
  ambiguous, and progress is not an answer on the reduced writers' side) and the line sentence (a line
  target omits what the read's walk does not reach, identity-less records included, and an on-line record
  is never chained under an identity-less off-line one). The ninth round extended the id-less sentence
  (an outcome the document cannot carry does not answer its call; a summary is written for the call that
  stored it) and added the colliding-id substitution, the owning-call assertion and the read-answered
  unresolved count. The tenth round extended the summary sentence (a writable summary answers its call, so
  a later id-less result is omitted rather than replacing it), the comparison sentence's owner assertion
  (a projecting reader's rows are compared as the file's own record units; a tree owner is resolved
  through the result's recorded ancestry) and the closing "the reduction's dispositions act only there"
  sentence (an unflagged run writes an id-less call's plain positional spelling, a recorded id's duplicate
  included). The eleventh round extended the outcome sentence (a selected run omits a progress event and
  counts it rather than writing the call's answer) and the id-less sentence (a written recorded result
  reserves its call's slot), the comparison sentence (rows the file's records do not account for keep
  standing, and a tree owner follows the installed file's own entry chain past an entry its reader merged
  away), and the closing sentence (the unflagged run keeps the progress event, the rows-against-items
  verdict for a Codex session and the reduction's own loss rows). The twelfth round added the file-backed
  line rule (a Pi-family session read from its file takes the file's own leaf path as the selection's line
  for the records the read answered an identity for, so a result the file left beside the conversation is
  not put back on the resumed walk and a window ending on one is refused by name), the whitespace-alone
  refusal beside the supplied-empty one, the outcome slot's writability yield on a run that carries a
  selection (an event the part cannot state yields the slot to a later recorded outcome it can state), and
  the id-recorded progress marking (an id-less event does not stand in for the call's stored summary).
* `docs/harness.md` — unchanged: this increment adds no gate step and no knob, and the citations step it
  already requires is the one that reads the new manifest.
* The throw-away probe file (§4) and the pre-change worktree were removed before the gate run; the probe's
  exact source is retained beside this record (`0009-d4-probe.txt`), and the third round replaced it with
  the revision that printed §4's runs (canonicalized identities, the report and artifact digests of the
  installing pipeline arm), the fourth round with the one it quotes (recorded instants compared
  instead of masked, `d4ProbeNormalizerControl` beside them), and the fifth with the one this record
  quotes (recorded identities left as recorded — `d4ProbeRecordedIDs` — with the
  `recorded-call-id-change`/`recorded-uuid-change` control lines pinning it), and the sixth with the one
  this record quotes (the off-path transcript materialized at run time, `d4ProbeOffPathTranscript`, so the
  review's input is compared on both trees); the seventh, eighth and ninth rounds re-ran that same revision
  unchanged over both trees (1109/1234 lines, the shared diff silent — §4's numbers reproduced), the ninth
  round after the owner assertion was gated on a document stating an owner (the ungated revision had moved
  the off-path Claude arm's already-`not-verified` detail), and the eleventh round re-ran it unchanged a
  third time over both trees (1183 / 1308 lines, the shared diff silent -- §4's numbers, with the ninth
  round's one disclosed line-group back to the pre-0009 tree's own). The
  fixture generator (`internal/ccc/fixturegen`) and the committed fixture tree move together, pinned by
  the existing `TestFixturesMatchTheirGenerator`.
