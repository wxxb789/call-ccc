# 0009a — Freeze the native oracle: evidence

Contract: [`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md) (left in `docs/goals/`
with its row marked *Implemented on `feat/0009a-native-oracle`; awaiting operator acceptance*, per
`AGENTS.md`; the increment that merges it archives it).
Plan: [`docs/plans/0009a-native-oracle.md`](../plans/0009a-native-oracle.md).
Citations: [`0009a-citations.tsv`](0009a-citations.tsv) (the machine-checked half of §6).
Host: Codex `0.170.0` (`codex-cli 0.170.0`), the pinned AgentsView carry
`83195c8a6861b8c74a4b6cb87512817fed6f8584`. Every command below ran on this host (`linux x64`), in this
repository, on branch `feat/0009a-native-oracle`; output is quoted verbatim.

**The increment in one line.** `scripts/codex-native-check.py` gains one frozen, offline half — the
well-formedness **oracle** for Codex session artifacts (`validate_artifact`, reachable as
`--validate <artifact> [--receipt <document>]`) — exercised by committed synthetic Codex JSONL fixtures
and one mutation per rule, with its native checks untouched. **No Go file changed**: this increment's
changed paths are `docs/**`, `AGENTS.md` and `scripts/**` only, and
`git status --short | grep '\.go$'` prints nothing.

**The review fix round (r1).** The oracle's first review found eight defects (a receipt blind to which
result survived, excluded dialogue surviving in the harness's own turn-view channel, an unordered plan
blind to a moved compaction boundary, a `null` receipt read as no receipt, a JSONL reader splitting on
more than the line feed and repairing invalid bytes, a clock admitting the leap second `60`, default
diagnostics echoing conversation text, and this record's own probe/baseline claims). Every quoted run
below was taken again on the fixed tree; §1's run, §2's failure list, §3's before/after pair and §7's
gate are the fixed state, and the probe of §0 is the corrected one (its invocation now names its
argument, and its counters include the pre-`turn_context` record counts the prose cites).

## 0. What was measured before anything was written

The rules are statements about the Codex rollout format, so they were measured first on the format's own
artifacts — Codex `0.170.0`'s own sessions under `$HOME/.codex/sessions` (25 files) — and later (§5) on
what this build's writer installs. The probe below is throw-away: it is quoted here and was removed
afterwards (the record's commands ran under `/tmp/ccc-0009a-evidence/`, which is not part of the tree).

```
$ python3 - "$HOME/.codex/sessions" <<'PY'
"""What the oracle's rules rest on: the host's own Codex sessions, read line by line (throw-away probe,
quoted in the evidence record, then removed)."""
import collections, datetime, glob, json, sys

envelopes, items, events, roles, blocks, members = (collections.Counter() for _ in range(6))
firsts, decreasing, unbalanced, malformed, blank_outputs, blank_events = (
    collections.Counter(), 0, 0, 0, 0, 0)
output_types = collections.Counter()
before_turn, before_turn_roles = collections.Counter(), collections.Counter()
paths = sorted(glob.glob(sys.argv[1] + '/**/*.jsonl', recursive=True))
for path in paths:
    calls, outputs, previous, first, inside_turn = {}, collections.Counter(), None, True, False
    for line in open(path, errors='replace'):
        line = line.strip()
        if not line:
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError:
            malformed += 1
            continue
        if first:
            firsts[record.get('type')] += 1
            first = False
        envelopes[record.get('type')] += 1
        if record.get('type') == 'turn_context':
            inside_turn = True
        stamp = record.get('timestamp')
        if isinstance(stamp, str):
            try:
                instant = datetime.datetime.fromisoformat(stamp.replace('Z', '+00:00'))
                if previous is not None and instant < previous:
                    decreasing += 1
                previous = instant
            except ValueError:
                pass
        payload = record.get('payload') if isinstance(record.get('payload'), dict) else {}
        if record.get('type') == 'session_meta':
            members.update(payload.keys())
        if record.get('type') == 'response_item':
            items[payload.get('type')] += 1
            if not inside_turn:
                before_turn[payload.get('type')] += 1
                if payload.get('type') == 'message':
                    before_turn_roles[payload.get('role')] += 1
            if payload.get('type') == 'message':
                roles[payload.get('role')] += 1
                for block in payload.get('content') or []:
                    if isinstance(block, dict):
                        blocks[block.get('type')] += 1
            if payload.get('type') in ('function_call', 'custom_tool_call'):
                calls[payload.get('call_id')] = calls.get(payload.get('call_id'), 0) + 1
            if payload.get('type') in ('function_call_output', 'custom_tool_call_output'):
                outputs[payload.get('call_id')] += 1
                output_types[type(payload.get('output')).__name__] += 1
                if not str(payload.get('output') or '').strip():
                    blank_outputs += 1
        if record.get('type') == 'event_msg':
            events[payload.get('type')] += 1
            if payload.get('type') in ('user_message', 'agent_message') and (
                    not isinstance(payload.get('message'), str) or not payload['message'].strip()):
                blank_events += 1
    if set(calls) != set(outputs) or any(count > 1 for count in calls.values()):
        unbalanced += 1

print("sessions:", len(paths))
print("first record of each session:", dict(firsts))
print("envelope types:", dict(envelopes))
print("session_meta members:", dict(members))
print("response_item payload types:", dict(items))
print("event_msg payload types:", dict(events))
print("message roles:", dict(roles), "| content block types:", dict(blocks))
print("call output member types (both families):", dict(output_types), "| blank:", blank_outputs)
print("blank or non-string user_message/agent_message events:", blank_events)
print("records whose instant precedes the record before them:", decreasing)
print("sessions with a call id not answered / repeated / an output without its call:", unbalanced)
print("lines that are not JSON:", malformed)
print("response_item records before the first turn_context:", dict(before_turn),
      "| messages among them by role:", dict(before_turn_roles))
PY
sessions: 25
first record of each session: {'session_meta': 25}
envelope types: {'session_meta': 25, 'response_item': 1613, 'event_msg': 1920, 'turn_context': 307, 'compacted': 1, 'world_state': 25, 'token_usage_record': 165, 'inter_agent_communication_metadata': 19}
session_meta members: {'id': 25, 'timestamp': 25, 'cwd': 25, 'originator': 25, 'cli_version': 25, 'source': 25, 'model_provider': 24, 'base_instructions': 24, 'git': 13, 'thread_source': 21, 'session_id': 21, 'runtime_workspace_roots': 21, 'history_mode': 21, 'context_window': 21, 'parent_thread_id': 5, 'agent_nickname': 4, 'agent_path': 4, 'multi_agent_version': 5, 'creator_user_id': 15, 'creator_account_id': 15}
response_item payload types: {'message': 233, 'reasoning': 408, 'function_call': 290, 'function_call_output': 290, 'custom_tool_call': 174, 'custom_tool_call_output': 174, 'web_search_call': 21, 'agent_message': 19, 'additional_tools': 3, 'configuration_update': 1}
event_msg payload types: {'task_started': 47, 'user_message': 24, 'token_count': 961, 'agent_reasoning': 314, 'agent_message': 55, 'item_completed': 450, 'task_complete': 44, 'turn_aborted': 3, 'thread_settings_applied': 21, 'thread_goal_updated': 1}
message roles: {'developer': 57, 'user': 72, 'assistant': 104} | content block types: {'input_text': 200, 'output_text': 104}
call output member types (both families): {'str': 342, 'list': 122} | blank: 12
blank or non-string user_message/agent_message events: 0
records whose instant precedes the record before them: 0
sessions with a call id not answered / repeated / an output without its call: 0
lines that are not JSON: 0
response_item records before the first turn_context: {'message': 73, 'additional_tools': 3} | messages among them by role: {'developer': 45, 'user': 27, 'assistant': 1}
```

What each answer fixed, and why the rule is the one that got stated:

| measured | the rule it grounds, or the rule it stops |
| --- | --- |
| 25/25 sessions open with `session_meta`, and every one of the 25 files holds exactly one | `oracle.session-meta` (first and only) |
| `id`, `originator` and `cli_version` on 25/25 (`source` too — but it is an *object* on the five subagent sessions, so it is not required to be a string) | `oracle.session-meta-members` names these three and not `source` |
| envelope types are exactly these eight | `oracle.record-type`'s closed set; `payload` types are open (19 item types, 10 event types) and the oracle enumerates none of them but the ones it reads |
| every message's content blocks are `input_text`/`output_text`, the two types the carried reader reads text from (`internal/parser/codex.go`'s `extractCodexTextBlocks`) | `oracle.message` |
| no `user_message`/`agent_message` event holds a blank or non-string `message` | `oracle.event-message` |
| no record's instant precedes the record before it (over 4 075 records) | `oracle.clock` |
| every call id is answered exactly once, none repeats, no output names an absent call | `oracle.call-pairing` |
| **12 blank outputs exist** on Codex's own files, and a `custom_tool_call_output`'s `output` is a list 122 times | the rules that were **not** stated: no rule requires an output to be non-blank or a string — a rule written from this build's writer's habits instead of from the format's own files would have refused the format's own files |
| 73 `response_item` messages stand *before* the first `turn_context` across the 25 files (45 developer, 27 user, 1 assistant; plus 3 `additional_tools`) | no rule requires a record to stand inside a turn — measured, so not stated |

## 1. The oracle and its rules (D1)

`scripts/codex-native-check.py` holds the oracle in one delimited section (its header states the freeze
and the oracle lock, §4 below). Fourteen rules, each one sentence, each a statement about the artifact
(or the artifact and an explicit manifest receipt):

| rule | states |
| --- | --- |
| `oracle.jsonl-objects` | every non-empty line of the artifact is exactly one JSON object |
| `oracle.envelope` | every record carries a parseable `timestamp`, a non-empty `type` and an object `payload` |
| `oracle.record-type` | every record's `type` is one the rollout format defines — the five this route's writer emits and the further ones Codex 0.170.0 writes; payload types stay open |
| `oracle.session-meta` | the artifact's first record is its `session_meta`, and the artifact carries exactly one |
| `oracle.session-meta-members` | that record's `id`, `originator` and `cli_version` are non-empty strings |
| `oracle.clock` | the envelope instants never decrease: no written record is stamped before the record written before it |
| `oracle.message` | a `response_item` message's `role` is a non-empty string and its `content` is a non-empty list of blocks typed as text the carried reader reads (`input_text`, `output_text`, `text`), at least one of them non-blank |
| `oracle.event-message` | a `user_message`/`agent_message` event carries a non-empty string `message` — the record the harness's own view of the turn is built from |
| `oracle.call-pairing` | every call's `call_id` is a non-empty string no other call of the artifact carries, and every call output names a call the artifact carries |
| `oracle.receipt` | a manifest receipt is an object whose `occurrences` are entries with a `kind` of this route's occurrence vocabulary, the identity that kind needs and a `disposition` of `emitted` or `omitted`; an unreadable receipt is reported and not compared with the artifact |
| `oracle.plan-emitted` | every occurrence the receipt marks `emitted` is in the artifact as many times as it is marked — a dialogue message in both of the target's channels for it: the model-visible `response_item` and the turn-view `event_msg` |
| `oracle.plan-accounted` | the artifact holds nothing the receipt does not mark `emitted`: an occurrence the receipt marks `omitted` and one it names nowhere are both refused — the plan is total, in both of the target's dialogue channels |
| `oracle.plan-order` | the occurrences the receipt marks `emitted` stand in the artifact in the receipt's own order — a compaction boundary moved past later turns compacts a different history even though every count is unchanged |
| `oracle.plan-pairs` | a call and its recorded result carry one disposition: a `call_id` whose call and result the receipt splits is the pair the closure rule forbids |

Its inputs are committed, hand-constructed synthetic Codex JSONL under
`scripts/tests/testdata/codex-oracle/`: `session.jsonl` (17 records — two turns, a call and its result, a
compaction boundary, plus one `world_state` and one `token_usage_record`, record types this build's
writer never emits) with `session-receipt.json`; `reduced.jsonl` (the same session's own records with the
second turn taken out — its lines are a subsequence of `session.jsonl`'s, asserted) with
`reduced-receipt.json`, which marks the two dropped messages `omitted`. No fixture is a converted source
session and none is derived from one.

**D1's negative half** is the suite's mutation table, one or more mutations per rule, each asserted to
turn *its* rule red and no other, with the rule table and the mutation table asserted in bijection:

```
$ cd scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle -v
test_a_receipt_that_declares_nothing_cannot_pass (test_codex_native_check.CodexArtifactOracle.test_a_receipt_that_declares_nothing_cannot_pass)
A plan that accounts for no occurrence, and one that cannot be read, both refuse the artifact. ... ok
test_a_reduced_artifact_is_validated_from_the_artifact_alone (test_codex_native_check.CodexArtifactOracle.test_a_reduced_artifact_is_validated_from_the_artifact_alone)
A reduced artifact and its plan are validated with nothing but the two documents in hand. ... ok
test_a_result_substituted_under_one_call_id_is_refused (test_codex_native_check.CodexArtifactOracle.test_a_result_substituted_under_one_call_id_is_refused)
Two results under one call id are distinguishable, and their recorded text is the identity. ... ok
test_every_rule_has_a_mutation_that_turns_its_fixture_red (test_codex_native_check.CodexArtifactOracle.test_every_rule_has_a_mutation_that_turns_its_fixture_red)
The rule table and the mutation table are one contract, read from both ends. ... ok
test_occurrence_identity_is_counted_not_deduplicated (test_codex_native_check.CodexArtifactOracle.test_occurrence_identity_is_counted_not_deduplicated)
The same message written twice is two occurrences, and a plan that counts it once refuses it. ... ok
test_the_citations_manifest_accounts_for_every_rule (test_codex_native_check.CodexArtifactOracle.test_the_citations_manifest_accounts_for_every_rule)
Every oracle rule is either a row of this increment's citations manifest or named in it. ... ok
test_the_committed_fixtures_validate (test_codex_native_check.CodexArtifactOracle.test_the_committed_fixtures_validate)
The positive fixtures are green, with and without their plans, and the reduced one is the ... ok
test_the_instant_reader_dates_an_offset_by_its_sign (test_codex_native_check.CodexArtifactOracle.test_the_instant_reader_dates_an_offset_by_its_sign)
`oracle_instant` dates the format's numeric offsets by their sign, not only its `Z`. ... ok
test_the_oracle_reads_the_artifacts_own_bytes (test_codex_native_check.CodexArtifactOracle.test_the_oracle_reads_the_artifacts_own_bytes)
The JSONL framing is the line feed and the parsing is JSON's, not Python's lenient half. ... ok
test_the_validate_mode_reports_a_verdict (test_codex_native_check.CodexArtifactOracle.test_the_validate_mode_reports_a_verdict)
`--validate` answers for one artifact (and its plan): PASS, FAIL and INCOMPLETE, with the ... ok

----------------------------------------------------------------------
Ran 10 tests in 0.041s

OK
```

The quote is this review-fix round's re-run, over the tree whose class holds ten cases: the four carried
from the increment and its earlier stages (among them the numeric offset's sign,
`test_the_instant_reader_dates_an_offset_by_its_sign`, and occurrence multiplicity,
`test_occurrence_identity_is_counted_not_deduplicated` — the two that pin what the mutation table cannot
see), and this round's two: `test_a_result_substituted_under_one_call_id_is_refused` (a result's identity
is its call and its recorded text) and `test_the_oracle_reads_the_artifacts_own_bytes` (the line-feed
split, and invalid UTF-8 reported rather than repaired).

The mutations number 31 for the 14 rules: `oracle.jsonl-objects` and `oracle.envelope` carry four each,
`oracle.session-meta`, `oracle.message`, `oracle.receipt` and `oracle.plan-accounted` three each,
`oracle.session-meta-members`, `oracle.call-pairing` and `oracle.plan-emitted` two each, and the
remaining five one each. Two of them, run
through the shipped command rather than in-process, show what the report reads like — the first a
structural rule, the second the plan/pair rule that is the reason the receipt exists at all (a call
whose recorded result is left out while the call travels). Each file below is a copy of a committed
fixture changed in exactly one way, by throw-away commands in the same scratch directory:

```
$ python3 - <<'PY' > /tmp/ccc-0009a-evidence/cli-version-dropped.jsonl
import json
lines = open("scripts/tests/testdata/codex-oracle/session.jsonl").read().splitlines()
record = json.loads(lines[0])
record["payload"].pop("cli_version")
lines[0] = json.dumps(record, separators=(",", ":"))
print("\n".join(lines))
PY
$ python3 - <<'PY' > /tmp/ccc-0009a-evidence/pair-split.jsonl
import json
lines = [line for line in open("scripts/tests/testdata/codex-oracle/session.jsonl").read().splitlines()
         if '"function_call_output"' not in line]
print("\n".join(lines))
PY
$ python3 - <<'PY' > /tmp/ccc-0009a-evidence/pair-split-receipt.json
import json
receipt = json.load(open("scripts/tests/testdata/codex-oracle/session-receipt.json"))
for entry in receipt["occurrences"]:
    if entry["kind"] == "tool_result":
        entry["disposition"] = "omitted"
print(json.dumps(receipt, separators=(",", ":")))
PY
```

The first artifact drops `session_meta.cli_version`; the second drops the `function_call_output` record
while its receipt marks the `tool_result` occurrence `omitted`:

```
$ python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/cli-version-dropped.jsonl
oracle: oracle.session-meta-members line 1 — session_meta.cli_version is None, not a non-empty string
oracle: FAIL — 17 record(s), 7 occurrence(s), 1 violation(s)
$ echo $?
1
```

```
$ python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/pair-split.jsonl \
      --receipt /tmp/ccc-0009a-evidence/pair-split-receipt.json
oracle: oracle.plan-pairs — call 'call_oracle_0001': the receipt gives the call and its recorded result different dispositions — a call and its recorded result travel together
oracle: FAIL — 16 record(s), 6 occurrence(s), 1 violation(s)
$ echo $?
1
```

The mode's own edges were run by hand as well, because a validator's failure modes are part of its
surface: an artifact holding no records at all is refused by the rule that opens the artifact (not by a
crash), an input that cannot be read at all or whose bytes are not UTF-8 text is `INCOMPLETE` with exit 2
— never a pass — and a receipt document that is not a plan (a file holding `null`) is a supplied value
the receipt rule refuses rather than an absent receipt:

```
$ : > /tmp/ccc-0009a-evidence/empty.jsonl && python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/empty.jsonl
oracle: oracle.session-meta — the artifact holds no records
oracle: FAIL — 0 record(s), 0 occurrence(s), 1 violation(s)
$ echo $?
1
$ python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/a-directory
oracle: INCOMPLETE — the artifact /tmp/ccc-0009a-evidence/a-directory cannot be read: [Errno 21] Is a directory: '/tmp/ccc-0009a-evidence/a-directory'
$ echo $?
2
$ printf 'not json\n' > /tmp/ccc-0009a-evidence/not-json.txt && python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/not-json.txt
oracle: oracle.session-meta — the artifact holds no records
oracle: oracle.jsonl-objects line 1 — not JSON (Expecting value at column 1)
oracle: FAIL — 0 record(s), 0 occurrence(s), 2 violation(s)
$ echo $?
1
$ printf '{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"id":"\xff"}}\n' > /tmp/ccc-0009a-evidence/not-utf8.jsonl \
  && python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/not-utf8.jsonl
oracle: INCOMPLETE — the artifact /tmp/ccc-0009a-evidence/not-utf8.jsonl is not UTF-8 text: 'utf-8' codec can't decode byte 0xff in position 75: invalid start byte
$ echo $?
2
$ printf 'null' > /tmp/ccc-0009a-evidence/null-receipt.json \
  && python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/session.jsonl \
      --receipt /tmp/ccc-0009a-evidence/null-receipt.json
oracle: oracle.receipt — the receipt is not an object carrying an `occurrences` list
oracle: FAIL — 17 record(s), 7 occurrence(s), 1 violation(s)
$ echo $?
1
```

## 2. D2 — the oracle needs no knowledge of the source or the selection

Two mechanical statements, one structural and one behavioural. **Structurally**, the oracle's input is
the artifact's text and, at most, a parsed receipt: `validate_artifact(text, receipt=<document>)` takes
no path — a supplied document, with the absence of a receipt as the argument's own default — and the case
drives it with `builtins.open`, `io.open` and `os.open` — the three Python-level
names a file read goes through, `pathlib`'s `read_text`/`open` resolving the middle one at call time —
all refusing to hand out any file, with the case first showing each refusal fires
(`test_a_reduced_artifact_is_validated_from_the_artifact_alone` — a reading that reached for a source
transcript, the fixture on disk or any other document could not have answered). **Behaviourally**, a
reduced artifact is validated against its plan from a directory that holds nothing else:

```
$ mkdir /tmp/ccc-0009a-evidence/reduced-only
$ cp scripts/tests/testdata/codex-oracle/reduced.jsonl \
     scripts/tests/testdata/codex-oracle/reduced-receipt.json /tmp/ccc-0009a-evidence/reduced-only/
$ ls -l /tmp/ccc-0009a-evidence/reduced-only
total 8
-rw-rw-r-- 1 azureuser azureuser  732 Nov 21 04:19 reduced-receipt.json
-rw-rw-r-- 1 azureuser azureuser 1973 Nov 21 04:19 reduced.jsonl
$ python3 scripts/codex-native-check.py --validate /tmp/ccc-0009a-evidence/reduced-only/reduced.jsonl \
      --receipt /tmp/ccc-0009a-evidence/reduced-only/reduced-receipt.json
oracle: PASS — 12 record(s), 5 occurrence(s)
$ echo $?
0
```

The other direction keeps "reduced" a statement about the artifact rather than about an oracle that
ignores plans: the *unreduced* artifact against the *reduced* plan names the two messages the plan left
out — each in both of the artifact's dialogue channels, the model-visible record and the turn-view event,
each at its own line — and nothing else:

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/session.jsonl \
      --receipt /tmp/ccc-0009a-evidence/reduced-only/reduced-receipt.json
oracle: oracle.plan-accounted line 14 — message user (8 character(s)): the artifact holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 15 — message user (8 character(s)): the artifact's turn view holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 16 — message assistant (24 character(s)): the artifact's turn view holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 17 — message assistant (24 character(s)): the artifact holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: FAIL — 17 record(s), 7 occurrence(s), 4 violation(s)
$ echo $?
1
```

## 3. D3 — the unflagged Codex acceptance, before and after

The native checks are untouched: the oracle is a separate section and `--validate` returns before the
native path's first line. That is measured, not asserted — the shipped check was run twice at the end of
this increment, once against a pristine extraction of the base revision pinned by **sha** (`git archive
21d43fe1a3c81bb59504bfaae23c8c618edf7ae2`, the commit this increment started from; named by its sha
because `git archive HEAD` extracts whatever head the caller is on — after the increment that is the
reviewed head, not the baseline), and once in this tree, both
times with no prompt flag (no model turn, `--resume-prompt` unset). The two runs' check lines are
identical in name and state (28 lines each: 26 `PASS` + 2 `SKIP`; the session ids and digests differ, as
fresh identities always do), and both exit 0:

```
$ git archive 21d43fe1a3c81bb59504bfaae23c8c618edf7ae2 | tar -x -C /tmp/ccc-0009a-evidence/before-tree
$ cd /tmp/ccc-0009a-evidence/before-tree && python3 scripts/codex-native-check.py \
      --repo /tmp/ccc-0009a-evidence/before-tree > /tmp/ccc-0009a-evidence/native-check-before.txt ; echo $?
0
$ cd /home/azureuser/repos/call-ccc && python3 scripts/codex-native-check.py \
      > /tmp/ccc-0009a-evidence/native-check-after.txt ; echo $?
0
$ tail -1 /tmp/ccc-0009a-evidence/native-check-before.txt
26 check(s) passed, 0 failed, 2 skipped
$ tail -1 /tmp/ccc-0009a-evidence/native-check-after.txt
26 check(s) passed, 0 failed, 2 skipped
$ diff <(grep -oE '^(PASS|FAIL|SKIP): [^—]*' /tmp/ccc-0009a-evidence/native-check-before.txt) \
       <(grep -oE '^(PASS|FAIL|SKIP): [^—]*' /tmp/ccc-0009a-evidence/native-check-after.txt) && echo identical
identical
```

The after-run's own end, verbatim (the full transcript is `native-check-after.txt` in the scratch
directory; `docs/harness.md`'s `scripts/codex-native-check.py` entry is where this check's own shape,
its isolation flags and its repeat scenario are described):

```
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical; the converted copy identical too: sha256 e2921e995915855d…, 2540 bytes)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)

26 check(s) passed, 0 failed, 2 skipped
```

This round's review findings changed the oracle section, its tests and its fixtures, and no Go file (the
increment's Bounds; `git status --short | grep '\.go$'` prints nothing): the same unflagged command was
run once more over the fixed tree, and its 28 check lines' names and states are identical to the
before-run's and to the increment's earlier after-runs, with the same closing count
(`26 check(s) passed, 0 failed, 2 skipped`) and exit 0 — so D3's claim holds for the increment's final
bytes.

## 4. D4 — the freeze, stated where a later lane reads it

The freeze is stated in four places, and the reason is stated with it: a fix round that moves this
checker *alongside* production code moves both sides of "the implementation satisfies the oracle" at
once, which is the shape the two cross-family consults named as goal 0009's root cause
(`docs/plans/0009-decomposition.md`); the conductor's **oracle-lock** signal blocks it.

| where | what it says |
| --- | --- |
| `scripts/codex-native-check.py`, the oracle section's header and the module docstring | the oracle, its rules and `--validate` are read-only for the rest of goal 0009; the oracle lock and its reason; what an ordinary increment looks like after goal 0009 (add the rule or the record type, and the mutation that turns its fixture red) |
| `docs/harness.md`, the `scripts/codex-native-check.py` entry | the offline half, the freeze and the lock, and that the rule↔mutation bijection is what the required `selftest` step re-derives |
| `AGENTS.md`, under *What is binding* | the lock as a rule the next lane follows, with the increment that froze it named |
| this record (§1, §4) and the plan | the same statement with the row-per-rule ledger |

The lock's enforcement lives in the conductor, not in this tree: nothing here can stop a later round
from editing the checker, which is why the statement and the mechanical signal (a round that moves a
gate/checker script alongside production code) are both written down rather than one of them assumed.

## 5. The oracle on artifacts it did not construct

A fixture is not evidence that the oracle reads what the writer installs. Both directions were measured
on this host. First, a fresh conversion of the committed Claude fixture — the same route the unflagged
acceptance runs — validated where it landed:

```
$ go build -tags=fts5 -o /tmp/ccc-0009a-evidence/ccc ./cmd/ccc      # exit 0
$ CLAUDE_PROJECTS_DIR=$PWD/internal/ccc/testdata/fixtures/claude \
  /tmp/ccc-0009a-evidence/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex \
      --target-home /tmp/ccc-0009a-evidence/conversion/home --target-binary codex --json \
      > /tmp/ccc-0009a-evidence/conversion/document.json ; echo $?
0
$ python3 scripts/codex-native-check.py --validate \
      /tmp/ccc-0009a-evidence/conversion/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1fa9e-a598-7326-943c-822aa0668c97.jsonl --json
{
  "artifact": "/tmp/ccc-0009a-evidence/conversion/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1fa9e-a598-7326-943c-822aa0668c97.jsonl",
  "receipt": null,
  "records": 17,
  "occurrences": 8,
  "violations": [],
  "verdict": "PASS"
}
oracle: PASS — 17 record(s), 8 occurrence(s)
$ echo $?
0
$ python3 -c 'import collections, json, sys; print(dict(collections.Counter(((r := json.loads(l)).get("type"), (r.get("payload") or {}).get("type")) for l in open(sys.argv[1]) if l.strip())))' "$artifact"
{('session_meta', None): 1, ('turn_context', None): 2, ('response_item', 'message'): 5, ('event_msg', 'user_message'): 2, ('event_msg', 'agent_message'): 3, ('response_item', 'function_call'): 1, ('response_item', 'function_call_output'): 1, ('compacted', None): 1, ('event_msg', 'thread_settings_applied'): 1}
```

(17 records and 8 occurrences. The `thread_settings_applied` `event_msg` in that count is the harness's
own: the conversion's `load` verification opened the installed session through Codex's app-server, and
opening a session makes the harness record its own settings in it — measured for this route in
`docs/evidence/0002-claude-to-codex-continuation.md`'s §2 table. The oracle takes the file as it stands;
the appended record is an `event_msg`, not an occurrence of the receipt's vocabulary, and no receipt was
handed in here.)

Second, the other direction of soundness: the oracle accepts every artifact the format's own writer
produced on this host, not only the ones this build writes.

```
$ green=0; red=0; for artifact in $(find "$HOME/.codex/sessions" -name '*.jsonl'); do \
    if python3 scripts/codex-native-check.py --validate "$artifact" >/dev/null 2>&1; then \
      green=$((green+1)); else red=$((red+1)); echo "red: $artifact"; fi; done; \
  echo "Codex's own sessions: $green green, $red red"
Codex's own sessions: 25 green, 0 red
```

## 6. Citations

`docs/evidence/0009a-citations.tsv` is this increment's acceptance surface. It carries nine rows, one per
oracle rule whose sentence restates a claim about this tree's own bytes that an existing Go test already
pins (the writer's record shape, its clock, its per-message records and events, its refusal to write an
output whose call it did not write), and its header states exactly what such a row claims and what it
does not: no row claims a Go test runs the oracle. The five receipt rules are named there as not covered,
with the reason that their producer does not exist yet — goal 0009c/0009d own it, and this goal's
Non-scope says so — and their re-derivation is the `selftest` case of §1. Because this increment changes
no Go file (its Bounds), no row cites a new test; the convention that a row is one gate-runnable Go test
(`docs/conventions.md`) is met by the existing ones, and the rest of the increment's obligations are
python cases the required `selftest` step runs (§1, §2) or the native run of §3.

```
$ python3 scripts/obligation-citations.py
docs/evidence/0002-citations.tsv: 36 row(s) verified
docs/evidence/0007-citations.tsv: 37 row(s) verified
docs/evidence/0008-citations.tsv: 13 row(s) verified
docs/evidence/0009a-citations.tsv: 9 row(s) verified
citations: 95 row(s) across 4 file(s) verified
citations: PASS
```

## 7. The gate

The gate is `python3 scripts/gate.py`, run alone (never beside a native check: both are memory-heavy on
this host), and it must print `GATE: PASS`:

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 452 tests in 8.871s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5739 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes ver…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009a-citations.tsv: 9 row(s) verified
  PASS     secrets  no leaks found in 208 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
$ echo $?
0
```

The `selftest` step's 452 tests are the whole `scripts/tests/` suite with this increment's oracle cases in
it (§1) — ten in the class after this fix round: the eight implemented and applied earlier, plus the two
this round added (a same-id output substitution refused through both accounting rules, and the artifact
read as its own bytes) — and the `citations` step's 9 rows are §6's manifest. The `gofmt`, `build`, `vet`
and `test` steps answer about an unchanged
Go tree: this increment's diff holds no Go file (`git status --short | grep '\.go$'` prints nothing).

The gate (and the citations run above) was re-run last, after this round's last edit to the oracle, its
tests, the fixtures and this record's prose — none of them a gate input the `selftest` cases read, which
is why the transcript above is the run over the tree this record's other quotes were taken on; the
`selftest` timing differs between runs, which is why the transcript is one run's own line rather than a
rounded one.

## 8. What this record does not claim

- **No native claim about the fixtures.** Codex never opened the synthetic fixtures or the reduced
  artifact: the oracle is the offline half, and the native half of this route stays where it was
  (the unflagged check of §3, and 0009d's own increment for the flagged path).
- **The record-type set is closed on purpose.** A Codex version that adds a record type makes the oracle
  refuse a legitimate file; that is disclosed in the rule's own sentence and in the plan's trade-offs, and
  it is the cheaper defect — an oracle whose rules cannot be broken cannot judge anything.
- **The receipt's schema is this increment's freeze of a producer that does not exist yet.** It is
  deliberately the smallest schema that can catch "the wrong result was omitted" (per-occurrence kind,
  identity and disposition) — a tool result's identity being its `call_id` and its recorded text, a
  dialogue message being read against both of the artifact's channels, and the emitted order against the
  receipt's own — and 0009c/0009d must satisfy it rather than tune it: that is what the oracle
  lock is for. Nothing in this increment emits a receipt.
- **Form, not meaning.** A green artifact is one that is well-formed and (with a plan) holds exactly the
  occurrences the plan places; the oracle does not judge a conversation's meaning, and a red one names a
  rule, never a remedy.
