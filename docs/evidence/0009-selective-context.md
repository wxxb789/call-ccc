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
| what a `subtype: hook_result` line reads as | **nothing**: the Claude reader skips the provider's own bookkeeping lines (measured on the fixture's first draft — ten records, nine messages — which is why the fixture's system record is the system-reminder promotion shape instead; plan §11.5) |
| what a target does with an undischarged call | Codex, Claude, Pi and OMP write the call with no output; OpenCode's format has no state for a call that has not finished, so it leaves the call out and counts it (`unpaired_tool_calls`, measured on the fixture through `ConvertToOpenCodeSession`) |
| the seam's own two dispositions | a selection that names the call of ordinal 2 alone carries the result at ordinal 3 as a `necessities_added` entry (`pulled_by` 2), and the Codex writer then holds the pair with `unpaired_tool_results = 0`; the same selection under `--strict-selection` is refused by name, before anything is written |
| what an unflagged run writes | nothing the previous revision did not: a throw-away before/after probe over five routes × seven documents prints the same 588 lines and the same normalized bytes on HEAD's tree and on this tree (§4) |

## 1. The recorded run: a selection converts, installs and **Codex's own loader opens it**

`bin/ccc continue claude:c1a74e28-… --to codex --select 2 --verify load` (the fixture is a *copy* under a
scratch tree, so nothing the run does can touch the committed bytes; the target home is a scratch one):

```
$ go build -tags fts5 -o bin/ccc ./cmd/ccc
$ mkdir -p /tmp/ccc-0009-ev/src/claude/-home-user-code-awaiting /tmp/ccc-0009-ev/codex-home
$ cp internal/ccc/testdata/fixtures/claude/-home-user-code-awaiting/*.jsonl \
      /tmp/ccc-0009-ev/src/claude/-home-user-code-awaiting/
$ CLAUDE_PROJECTS_DIR=/tmp/ccc-0009-ev/src/claude bin/ccc continue \
    claude:c1a74e28-3b0d-4f91-8d27-6e5b90c4a183 --to codex \
    --target-home /tmp/ccc-0009-ev/codex-home --select 2 --verify load --json
exit 0
```

The three states the core design keeps apart, from the report's own document:

```
conversion:   "state": "converted", "format": "codex-rollout-jsonl", "read_from": "source",
              carried {messages 0, user_messages 0, assistant_messages 0, tool_calls 1, tool_results 1,
              compaction_boundaries 0, turns 1, bytes 896}
installation: "state": "installed", "mechanism": "hard-link",
              "path": /tmp/ccc-0009-ev/codex-home/sessions/2026/05/08/rollout-2026-05-08T08-00-04-01a1f47c-f0b2-7f3b-a65f-311dff722825.jsonl
              "source_check_state": "unchanged", "detail": sha256 7d3f25bd09b9b5518ba90641bf23b164df1664de623ea4d06526967877306431, taken before and after the conversion
verification.structure: "state": "verified", "method": the inherited AgentsView Codex reader over the installed session,
              read_back {messages 1, assistant_messages 1, tool_calls 1, tool_results 1}
verification.load: "state": "verified", "method": the target harness's own loader (codex app-server, thread/resume),
              "detail": the harness's loader answered with this session's own id and path; opening a session makes the
              harness record its own settings in it (measured: one thread_settings_applied record), and no session other
              than the one it opened is touched
              thread_id 01a1f47c-f0b2-7f3b-a65f-311dff722825, path as above,
              binary /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex, binary_version 0.170.0
target_session: provider codex, session_id 01a1f47c-f0b2-7f3b-a65f-311dff722825,
              resume_command CODEX_HOME=/tmp/ccc-0009-ev/codex-home codex resume 01a1f47c-f0b2-7f3b-a65f-311dff722825
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
  source:       /tmp/ccc-0009-ev/src/claude/-home-user-code-awaiting/c1a74e28-3b0d-4f91-8d27-6e5b90c4a183.jsonl
  read from:    source
  conversation: 11 message(s), 4 user, 7 assistant, 3 tool call(s), 1 compaction boundary(ies)
  boundary:     2902 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2902: the file ends at a record boundary
  selection:    5-10: the read's own ordinals are 0-10 (11 record(s))
  selected:     6 record(s) at ordinals 5-10
  excluded:     5 record(s) at ordinals 0-4
  necessities:  0 record(s) added: none
  undischarged: 1 carried call(s) with no recorded result at ordinals 5
  converted:    codex-rollout-jsonl, 2168 bytes: 2 message(s), 2 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  omitted:      system_records ×1
  omitted:      tool_result_sources ×1
  carried-with-caveat: tool_name_vocabulary ×2
  carried-with-caveat: unresolved_source_parents ×2
  installation: dry run, nothing written to /tmp/ccc-0009-ev/codex-home
  source file:  unchanged (sha256 7d3f25bd09b9b5518ba90641bf23b164df1664de623ea4d06526967877306431, taken before and after the conversion)
  target id:    01a1f47d-3743-7547-ad4b-da4a9b9184ac (proposed; nothing was installed)
  working dir:  /home/user/code/awaiting
  resume:       CODEX_HOME=/tmp/ccc-0009-ev/codex-home codex resume 01a1f47d-3743-7547-ad4b-da4a9b9184ac (proposed; nothing was installed)
  non-interactive resume: CODEX_HOME=/tmp/ccc-0009-ev/codex-home codex exec resume 01a1f47d-3743-7547-ad4b-da4a9b9184ac '<prompt>' (proposed; nothing was installed)
  cli_version:  0.170.0 (probe)
  structure:    verified (the inherited AgentsView Codex reader over a throw-away copy (dry run))
  native load:  not-verified (the target harness's own loader (codex app-server, thread/resume))
                a dry run installs nothing, so the target harness has nothing to load
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                not requested (--verify load)
  warning:      the conversion carries a selection of the read: the specification "5-10" selected 6 record(s) at ordinals 5-10, 5 record(s) are excluded, and 0 record(s) were added as a structural necessity (a recorded call and its recorded result are carried together)
  warning:      1 carried call(s) at ordinals 5 have no recorded result anywhere in the read (the source itself left them undischarged); the target records them as the calls they are
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
      --target-home /tmp/ccc-0009-ev/codex-home --select 5-3
ccc: the selection cannot be honoured: the region "5-3" runs backwards: a region's first ordinal has to
be at or below its last
exit=2

$ … --select 42
ccc: the selection cannot be honoured: the selection names ordinal 42, which the read's own ordinals do
not hold: the read returned 11 record(s) at ordinals 0-10, and a specification names values from that space
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
the one rollout §1 wrote, and the run printed no report on stdout. The home's other files are the
harness's own bookkeeping, which its loader wrote when §1 opened the session; the source copy's bytes are
unchanged.) The same three refusals, plus the duplicates and the "selects nothing" arm over a gappy read,
are the Go cases named in [`0009-citations.tsv`](0009-citations.tsv).

## 4. D4: the unflagged run, before and after

A throw-away probe (`internal/ccc/zz_d4_probe_test.go`: added, run, quoted here, removed) converts seven
documents (both Claude transcripts, both Codex rollouts, the Pi fixture, the OMP fixture and the OpenCode
document) through every one of the five routes' writers — printing each conversion's written bytes'
digest over a normalized copy (minted uuids, the routes' own minted id spellings, the scratch paths and
the checkout root folded), every item line and every non-zero loss row — **and** runs the *pipeline*
(`continueWith`, dry run, `--verify none`, the selection flag absent) per route × document, printing seven
normalized report lines each (the conversion block and its rows, the installation, the target session,
the verification states, the source counts and boundary, the warnings). One probe revision produced every
run quoted here (the normalization above was folded before the pair below, and the exact revision that
printed these lines is the one the record describes); it was run twice per tree, on HEAD's tree (a
throw-away worktree at `9242e49` with the identical probe file copied in) and on this tree:

```console
$ diff /tmp/ccc-d4-before/probe.txt /tmp/ccc-d4-after/probe.txt   # silent
$ diff -r /tmp/ccc-d4-before/files /tmp/ccc-d4-after/files        # silent
$ wc -l /tmp/ccc-d4-before/probe.txt
588 /tmp/ccc-d4-before/probe.txt
```

588 lines = 35 `PROBE-BYTES` (5 routes × 7 documents) + 322 `PROBE-ITEM` + 35 `PROBE-LOSS` + 196
`PROBE-RUN` (4 routes × 7 documents × 7 lines; the OpenCode *target*'s pipeline arm is its own importer's
run and is covered by §1–§3's standalone runs instead). The first two diffs above are silent: the same 35
byte digests, the same item lines, the same non-zero loss rows and the same 196 report lines, for every
route × document pair — so no route's written bytes, no item line and no report field moved for a run
without the flag. The normalization is stable: each tree was run twice, and the two runs of one tree are
byte-identical to each other (`diff` silent on both sides), which is what makes the folded digests a
statement about the conversion rather than about the ids the conversion mints.

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
| `--select 2 --strict-selection` | refused by name, both ordinals and the call id named, nothing published | §3's third block; `TestSelectionRefusesASeparatedPairUnderStrictSelection` |
| the read call at ordinal 5, which the source never answered | carried as the call it is, with no result invented for it | `calls_undischarged {count 1, regions ["5"]}`; §2's human line |
| the compaction summary at ordinal 6 and the system record at ordinal 8 (whose tool result answers the cap call at 7) | carried inside a window that names them, boundary and system record included; the record's text is the target's own `system_records` row, its tool result travels | §2's carried counts (`compaction_boundaries 1`, `tool_results 1`) and its loss rows |
| every record the specification and the necessity did not name | stayed behind: their own words and call ids are absent from the written document | §1's table; `TestSelectionWritesExactlyTheSelectedOrdinals` (five routes, present/absent evidence per case) |
| the source file itself | read whole, measured before and after | `source_check_state: unchanged` with the fixture copy's `sha256`, §1 |
| a call the source left unanswered, at a target whose format has no state for it (OpenCode) | the route leaves the call out and counts it — its own limit, not the selection's claim | `unpaired_tool_calls ×1`; `TestSelectionWritesExactlyTheSelectedOrdinals`'s `TargetOpenCode` arm |
| a run without `--select` | the same conversion it was, byte for byte | §4's two silent diffs |
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
  PASS     selftest Ran 442 tests in 8.832s (rc=0)
  PASS     parity   … (5553 carried file(s) identical to the pinned revision; 27 declared deviations + 1
                    generated input present as declared; 32 project-owned prefix(es))
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009-citations.tsv: 14 row(s) verified
  PASS     secrets  no leaks found in 195 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
exit=0
```

(`parity`'s detail is elided in this transcript with `…` — the step prints it as one line of about 800
characters, and the numbers this record quotes are the ones it holds; the step's own leading `… | ` marks
inside that line are the gate's rendering of a long detail, not this record's cut. The transcript is one
run of this script on the increment's tree; `selftest`'s duration is that run's own. It is the run taken
after the increment's own, with the simplify findings applied: between the two runs one count moved —
`gofmt`, from `3624` to `3627`. `gofmt` lists `git ls-files` only, and the increment's three new `.go`
files were still untracked when its first run was taken, so committing them moved that count. Every other
count is the same in both runs; `selftest`'s duration moves run to run because the step times itself.)

`docs/evidence/0009-citations.tsv` holds one row per obligation a gate-runnable Go test discharges, in
this increment's own id spaces:

* `D1`–`D5` — the goal's five `Done when` clauses (D4 has **no row**: its evidence is §4's throw-away
  before/after probe, which no gate step runs — the form-based carve-out 0007 and 0008's manifests state;
  the reason is in the manifest's header comment);
* `B1`–`B2` — the goal's two `Bounds` clauses;
* `S1`–`S4` — the rules of `AGENTS.md`'s selection paragraph, in the paragraph's own order (opt-in and the
  untouched default; the recorded-link seam with the counterpart carried and named; the refusals with the
  core's sentence and the usage code before any install; the report's classes and the read's own ordinal
  space).

The native half of D1 ("the target is natively loadable") is §1's `verification.load: verified` — Codex's
own app server, quoted with the harness's version — and is not a gate step, so no row holds it.

## 7. Docs, rules and bookkeeping

* `AGENTS.md` — a new binding bullet ("A conversion carries only what the caller selected …") outside the
  per-harness bullets, and `internal/ccc`'s file list in "How to work here" names `selection.go`. The
  control file moves because this increment changes a rule an agent must follow and the core's layout.
* `docs/conventions.md` — the layout block names `selection.go`; a new rule under "Rules of the work"
  states the specification's grammar, the seam, the refusals and the report's classes.
* `docs/plans/0009-selective-context.md` — §11 records nine departures with the measurement behind each.
* `docs/goals/README.md` — 0009's row reads *Implemented on `feat/0009-selective-context`; awaiting
  operator acceptance*, and the goal file's own `Status:` line reads the same (the form 0007 and 0008
  carried while their increments awaited acceptance); nothing else was ready to archive (0008 was archived
  by the activation increment that landed this plan, and 0010–0013 are provisional while 0014–0017 are
  ready-but-unstarted).
* `docs/decisions/OPEN-QUESTIONS.md` — Q-0009-01 (the strict refusal's exit code), Q-0009-02 (the seam
  pairs by recorded id alone; an id-less record carries no link), Q-0009-03 (an empty `--select` value is
  read as the flag's absence).
* `README.md` — an example line, a paragraph on the flag and its report, and `Select` in the engine list.
* `docs/harness.md` — unchanged: this increment adds no gate step and no knob, and the citations step it
  already requires is the one that reads the new manifest.
* The throw-away probe file (§4) and the pre-change worktree were removed before the gate run; the
  fixture generator (`internal/ccc/fixturegen`) and the committed fixture tree move together, pinned by
  the existing `TestFixturesMatchTheirGenerator`.
