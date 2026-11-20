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

**The review fix round (r2).** The oracle's second review found seven defects, all in the oracle half: a
call identity that ignored the emitted tool name and input (the same `call_id` could certify a different
invocation); a `compacted` record without a summary standing outside receipt accounting although the
carried reader records a boundary for every one of them; a turn-view dialogue order that was counted but
not checked (two prompts swapped between turns passed); a blank test using Python's wider `str.strip`
where the writer's own test is Go's `strings.TrimSpace`, so U+001C dialogue the writer preserves was
refused; default diagnostics that interpolated malformed member values (a prompt written into
`timestamp` or `call_id` was echoed into a report printed by default); a deeply nested receipt whose
`RecursionError` escaped instead of ending `INCOMPLETE`; and §5's probe reading `"$artifact"` without
ever assigning it. Every quoted run below was taken again on the fixed tree: §1's case run and its
mutation counts, §1's and §2's `--validate` runs, §3's unflagged native check, §5's converted-artifact
probe (whose commands now derive the path from the conversion report) and §7's gate.

**The review fix round (r3).** The oracle's third review found seven defects, and its own close-out
instruction was applied rather than a per-finding patch: every rule the touched surface claims was
re-verified by one executed revert-mutation (§8). The findings: a compaction record's native
`replacement_history` held dialogue the receipt could not account for (the omitted prompt could survive
inside the boundary); a `custom_tool_call`'s recorded input lives in `input`, but the identity read only
`arguments`, so an empty receipt member certified any input and the actual input mismatched; call
ownership was checked against the artifact's whole set of ids, so an output written before its call
passed; the instant matcher took Python's `$` (a trailing line feed passed) and `\d` (a Unicode decimal
digit passed where no RFC3339 reader parses it); an empty receipt passed against a metadata-only
artifact, contrary to the plan's "a receipt that declares nothing cannot pass"; the unmatched-output
diagnostic echoed a string `call_id` verbatim; and §3's replay was not portable as written (the tar
destination was never created and the after command hard-coded the author's checkout). Every quoted run
below was taken again on the r3 tree, §3's replay now stands on a recipe that creates its scratch
directory and derives the checkout, and §8 records the six revert-mutations — one per fix — with the
failure each one produced.

**The review fix round (r4).** The oracle's fourth review found four defects, and its close-out
instruction was applied again: the surface the round touched was re-derived by executed revert-mutations
rather than argued about (§9). The findings: receipt validation certified an artifact that still held
omitted dialogue — the reduced plan's omitted prompt written into the delivered content of an appended
**inter-agent** `agent_message` (a **user** message to the carried reader), which neither accounting rule
saw at all; the call identity was the carried reader's projection of a call's input, so the writer's own
`arguments: "{}"` and a missing member were one occurrence — a receipt naming the written `"{}"` failed
against the correct artifact while one naming the read's empty answer certified both; a decoded string
that strict UTF-8 cannot encode (JSON's `\ud800` escape) and a call input nested past Python's parser
each ended a run as a traceback rather than a verdict; and §8's recorded instant revert could not
re-derive the failures it quoted (two capture groups where `oracle_instant` reads eight) while its
scratch-copy instructions omitted the citations TSV the full test class reads. The refusal was stated
over the *class* the first finding opens — every `response_item` a read turns into an occurrence the
receipt's vocabulary cannot name, the inter-agent message, an item type the reader's own switch does not
take carrying a role and readable text, and an `image_generation_call` among them — and §9 records one
executed revert per fix, with the failure each one produced and the before/after CLI verdicts on the
finding's own artifacts.

**The review fix round (r7).** The oracle's seventh review found five defects, and the close-out
instruction was applied again (§12): two in what a receipt can establish — a result whose call the read
answers elsewhere (a `wait_agent` output is read from its own `status` object) or whose output no read
appends a result from was certified by the receipt's identity, and a call's recorded input written as a
non-string member was matched by the identity's JSON spelling while the carried reader answers the
member's raw bytes; one in what the order rule states — a record's turn placement (a `turn_context` moved
across records) is outside the receipt's vocabulary, now stated as the rule's scope and pinned by a case;
one in this record's own replay recipes — a scratch path removed recursively, and an earlier recipe
redirecting into a directory no command created, now one `mktemp -d` root, reused and never deleted; and
one in the mutation corpus — the envelope's missing-`type` arm and the metadata record's `originator` arm
had no negative fixture. Every quoted run below was taken again on the fixed tree: §1's case run and
mutation counts, §1's and §2's `--validate` runs, §3's unflagged native check, §5's converted-artifact
probe and §6–§7's citations and gate runs.

**The replay's own scratch root.** The sections below read and write their throw-away files under one
root, allocated once here and never deleted, so a replay cannot collide with — or remove — a directory it
did not create:

```
$ root_scratch=$(mktemp -d "${TMPDIR:-/tmp}/ccc-0009a-evidence.XXXXXX")
$ echo "$root_scratch"
/tmp/ccc-0009a-evidence.jrYpjh
```

Every command below is written against `$root_scratch` and the checkout the caller stands in. The
outputs quoted beside the commands are each run's own, and the authoring host's earlier runs used the
fixed root `/tmp/ccc-0009a-evidence`: an output that spells a path is that run's own directory, not one a
replay needs to create.

## 0. What was measured before anything was written

The rules are statements about the Codex rollout format, so they were measured first on the format's own
artifacts — Codex `0.170.0`'s own sessions under `$HOME/.codex/sessions` (25 files) — and later (§5) on
what this build's writer installs. The probe below is throw-away: it is quoted here and was removed
afterwards (the record's commands ran under one throw-away root — `/tmp/ccc-0009a-evidence/` on this
host, and a replay's own `$root_scratch` above — which is not part of the tree).

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
| every call id is answered exactly once, none repeats, no output names an absent call | `oracle.call-pairing` — the order arm (an output before its call) is measured by §5's sweep over the same 25 files |
| **12 blank outputs exist** on Codex's own files, and a `custom_tool_call_output`'s `output` is a list 122 times | the rules that were **not** stated: no rule requires an output to be non-blank or a string — a rule written from this build's writer's habits instead of from the format's own files would have refused the format's own files |
| 73 `response_item` messages stand *before* the first `turn_context` across the 25 files (45 developer, 27 user, 1 assistant; plus 3 `additional_tools`) | no rule requires a record to stand inside a turn — measured, so not stated |

## 1. The oracle and its rules (D1)

`scripts/codex-native-check.py` holds the oracle in one delimited section (its header states the freeze
and the oracle lock, §4 below). Fourteen rules, each one sentence, each a statement about the artifact
(or the artifact and an explicit manifest receipt):

| rule | states |
| --- | --- |
| `oracle.jsonl-objects` | every non-empty line of the artifact is exactly one JSON object, every member name written once inside it — a repeated name is a line two JSON readers answer differently for (this oracle's decoder keeps the last value, the carried reader's `gjson` the first), refused before comparison |
| `oracle.envelope` | every record carries a parseable `timestamp` — one whole, ASCII RFC3339 instant (a trailing line feed or a non-ASCII digit is refused) — a non-empty `type` and an object `payload` |
| `oracle.record-type` | every record's `type` is one the rollout format defines — the five this route's writer emits and the further ones Codex 0.170.0 writes; payload types stay open |
| `oracle.session-meta` | the artifact's first record is its `session_meta`, and the artifact carries exactly one |
| `oracle.session-meta-members` | that record's `id`, `originator` and `cli_version` are non-empty strings |
| `oracle.clock` | the envelope instants never decrease: no written record is stamped before the record written before it |
| `oracle.message` | a `response_item` message's `role` is a non-empty string and its `content` is a non-empty list of blocks typed as text the carried reader reads (`input_text`, `output_text`, `text`), at least one of them non-blank |
| `oracle.event-message` | a `user_message`/`agent_message` event carries a non-empty string `message` — the record the harness's own view of the turn is built from |
| `oracle.call-pairing` | every call's `call_id` is a non-empty string no other call of the artifact carries and the call carries the non-empty `name` the carried reader registers a call for — a record without one is no call, so an output naming it attaches to nothing — and every call output names a call the artifact carries **before it** |
| `oracle.receipt` | a manifest receipt is an object whose `occurrences` are entries with a `kind` of this route's occurrence vocabulary, the identity that kind needs — a tool call's `call_id`, `name` and recorded input (the member the artifact writes it in, `arguments` then `input`, read exactly as written), a tool result's `call_id` and recorded text — and a `disposition` of `emitted` or `omitted`; a receipt that declares no occurrences is refused rather than compared, and an unreadable receipt is reported and not compared with the artifact |
| `oracle.plan-emitted` | every occurrence the receipt marks `emitted` is in the artifact as many times as it is marked — a dialogue message in both of the target's channels for it: the model-visible `response_item` and the turn-view `event_msg` |
| `oracle.plan-accounted` | the artifact holds nothing the receipt does not mark `emitted`: an occurrence the receipt marks `omitted`, one it names nowhere, a compaction boundary's `replacement_history`, a `response_item` a read turns into an occurrence the receipt's vocabulary cannot name — the inter-agent `agent_message` addressed to the session's own agent path, any other payload type whose `user`/`assistant` role and readable text a read appends as a message, an `image_generation_call`, a `message` in a role outside the writer's two carrying readable text (a native shape Codex's own files carry, which the vocabulary cannot name), and a `custom_tool_call_output` whose carried read answers the result's outcome (a member this route's writer's own `function_call_output` results have no place for, review round r6's finding) — a shape whose own read or identity this oracle does not model: a call carrying both `arguments` and `input`, a call whose recorded input or result whose recorded text is written as a member that is not a string (the receipt's identity member is a string, and a non-string member's own bytes are what the carried read answers), and an object-valued `content` a read iterates the member values of (`ForEach`) — and a result the carried read does not establish: one whose call the read answers elsewhere (`spawn_agent`'s output links a subagent session and appends no result; `wait`/`wait_agent` answer per-agent results from the output's own `status` object — the exec-cell poll reads as an ordinary result and is exempt) or one that carries no output a read appends a result from — are equally refused; the plan is total, in both of the target's dialogue channels |
| `oracle.plan-order` | the occurrences the receipt marks `emitted` stand in the artifact in the receipt's own order, and so do its dialogue messages in the artifact's turn view — a compaction boundary moved past later turns, or two prompts in each other's turns, compacts a different history even though every count is unchanged; the receipt's vocabulary names no turn, so which `turn_context` record a record stands after is outside what a receipt can state and not compared |
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
test_a_boundarys_replacement_history_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_boundarys_replacement_history_is_refused_with_a_plan)
The omitted prompt written into a boundary's `replacement_history` is refused in receipt mode. ... ok
test_a_call_substituted_under_one_call_id_is_refused (test_codex_native_check.CodexArtifactOracle.test_a_call_substituted_under_one_call_id_is_refused)
Two invocations under one call id are distinguishable, and name and input are the identity. ... ok
test_a_call_without_a_usable_name_is_no_call (test_codex_native_check.CodexArtifactOracle.test_a_call_without_a_usable_name_is_no_call)
A call whose `name` is not a non-empty string is refused, and owns no output. ... ok
test_a_compacted_record_without_a_summary_is_counted (test_codex_native_check.CodexArtifactOracle.test_a_compacted_record_without_a_summary_is_counted)
Every `compacted` record is a boundary occurrence, with or without a summary. ... ok
test_a_custom_calls_recorded_input_is_part_of_its_identity (test_codex_native_check.CodexArtifactOracle.test_a_custom_calls_recorded_input_is_part_of_its_identity)
A `custom_tool_call`'s recorded input — the member Codex's own files write it in — is the ... ok
test_a_custom_tool_call_output_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_custom_tool_call_output_is_refused_with_a_plan)
A recorded result in the custom tool family is refused with a plan, green without one. ... ok
test_a_developer_role_message_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_developer_role_message_is_refused_with_a_plan)
A `message` in a role the receipt does not name is refused with a plan, green without one. ... ok
test_a_line_with_a_duplicate_member_is_refused (test_codex_native_check.CodexArtifactOracle.test_a_line_with_a_duplicate_member_is_refused)
A line whose object writes a member name twice is refused before any comparison, in both modes. ... ok
test_a_malformed_member_is_named_without_echoing_its_text (test_codex_native_check.CodexArtifactOracle.test_a_malformed_member_is_named_without_echoing_its_text)
Default diagnostics name a malformed value by its place and kind, never by its contents. ... ok
test_a_moved_turn_context_is_outside_the_receipts_vocabulary (test_codex_native_check.CodexArtifactOracle.test_a_moved_turn_context_is_outside_the_receipts_vocabulary)
The receipt names no turns, so `oracle.plan-order` compares order, not turn membership. ... ok
test_a_non_string_identity_member_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_non_string_identity_member_is_refused_with_a_plan)
A call's recorded input or a result's recorded text written as a non-string member is refused ... ok
test_a_receipt_that_declares_nothing_cannot_pass (test_codex_native_check.CodexArtifactOracle.test_a_receipt_that_declares_nothing_cannot_pass)
A plan that declares no occurrences, and one that cannot be read, both refuse the artifact. ... ok
test_a_reduced_artifact_is_validated_from_the_artifact_alone (test_codex_native_check.CodexArtifactOracle.test_a_reduced_artifact_is_validated_from_the_artifact_alone)
A reduced artifact and its plan are validated with nothing but the two documents in hand. ... ok
test_a_result_substituted_under_one_call_id_is_refused (test_codex_native_check.CodexArtifactOracle.test_a_result_substituted_under_one_call_id_is_refused)
Two results under one call id are distinguishable, and their recorded text is the identity. ... ok
test_a_result_the_receipt_cannot_establish_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_result_the_receipt_cannot_establish_is_refused_with_a_plan)
A result whose call the read answers elsewhere, or whose output a read appends nothing from, ... ok
test_a_written_empty_object_input_is_part_of_the_identity (test_codex_native_check.CodexArtifactOracle.test_a_written_empty_object_input_is_part_of_the_identity)
The writer's own `arguments: "{}"` is the written input, and a receipt naming it matches. ... ok
test_decoded_input_never_aborts_validation (test_codex_native_check.CodexArtifactOracle.test_decoded_input_never_aborts_validation)
A lone surrogate in a decoded string and a string member nested past Python's parser both end ... ok
test_every_rule_has_a_mutation_that_turns_its_fixture_red (test_codex_native_check.CodexArtifactOracle.test_every_rule_has_a_mutation_that_turns_its_fixture_red)
The rule table and the mutation table are one contract, read from both ends. ... ok
test_occurrence_identity_is_counted_not_deduplicated (test_codex_native_check.CodexArtifactOracle.test_occurrence_identity_is_counted_not_deduplicated)
The same message written twice is two occurrences, and a plan that counts it once refuses it. ... ok
test_receipt_validation_refuses_the_occurrences_it_cannot_name (test_codex_native_check.CodexArtifactOracle.test_receipt_validation_refuses_the_occurrences_it_cannot_name)
The omitted prompt written into a shape the receipt's vocabulary cannot name is refused. ... ok
test_the_citations_manifest_accounts_for_every_rule (test_codex_native_check.CodexArtifactOracle.test_the_citations_manifest_accounts_for_every_rule)
Every oracle rule is either a row of this increment's citations manifest or named in it. ... ok
test_the_committed_fixtures_validate (test_codex_native_check.CodexArtifactOracle.test_the_committed_fixtures_validate)
The positive fixtures are green, with and without their plans, and the reduced one is the ... ok
test_the_instant_reader_dates_an_offset_by_its_sign (test_codex_native_check.CodexArtifactOracle.test_the_instant_reader_dates_an_offset_by_its_sign)
`oracle_instant` dates the format's numeric offsets by their sign, not only its `Z`. ... ok
test_the_oracle_reads_the_artifacts_own_bytes (test_codex_native_check.CodexArtifactOracle.test_the_oracle_reads_the_artifacts_own_bytes)
The JSONL framing is the line feed and the parsing is JSON's, not Python's lenient half. ... ok
test_the_oracles_blank_test_is_the_writers (test_codex_native_check.CodexArtifactOracle.test_the_oracles_blank_test_is_the_writers)
U+001C is not space to the writer, so a text of U+001C alone is dialogue, in both channels. ... ok
test_the_validate_mode_reports_a_verdict (test_codex_native_check.CodexArtifactOracle.test_the_validate_mode_reports_a_verdict)
`--validate` answers for one artifact (and its plan): PASS, FAIL and INCOMPLETE, with the ... ok

----------------------------------------------------------------------
Ran 26 tests in 0.110s

OK
```

The quote is the r7 review-fix round's re-run, over the tree whose class holds twenty-six cases: the
nineteen of the r4 state (the sixteen of the r3 state — among them the numeric offset's sign,
`test_the_instant_reader_dates_an_offset_by_its_sign`, and occurrence multiplicity,
`test_occurrence_identity_is_counted_not_deduplicated`, the two that pin what the mutation table cannot
see; r1's result-substitution and own-bytes cases; r2's call-substitution, empty-boundary, blank-test and
no-echo cases; r3's custom-call-input and boundary-`replacement_history` cases — and the r4 round's
three:
`test_receipt_validation_refuses_the_occurrences_it_cannot_name` (every shape a read turns into an
occurrence the receipt's vocabulary cannot name is refused where a plan is compared — the finding's
inter-agent `agent_message` attack among them — with the read's own gates as green controls),
`test_a_written_empty_object_input_is_part_of_the_identity` (the writer's `arguments: "{}"` is the
written input: a receipt naming it matches, the read's empty identity refuses it, and the two written
forms are two occurrences) and `test_decoded_input_never_aborts_validation` (a lone surrogate and an
input nested past Python's parser each end in the documented verdict and exit code through the shipped
CLI, never a traceback). The r5 round's tree holds twenty: the nineteen above plus
`test_a_line_with_a_duplicate_member_is_refused` (a line whose object writes a member name twice, spliced
as raw JSONL, is refused structurally, with its plan, and in the receipt document), with the round's new
arms inside `test_a_written_empty_object_input_is_part_of_the_identity` (a call carrying both `arguments`
and `input`: the written identity stands and receipt mode refuses the shape) and
`test_receipt_validation_refuses_the_occurrences_it_cannot_name` (an object-valued `content` under the
fallback item and the addressed inter-agent message, with the read's own gates as green controls). The
r6 review-fix round's tree holds twenty-three: the twenty above plus that round's three cases —
`test_a_developer_role_message_is_refused_with_a_plan` (a `message` in a role the receipt does not name
is refused only where a plan is compared; a native shape Codex's own files carry, admitted structurally,
and its readable text is what makes it an occurrence),
`test_a_custom_tool_call_output_is_refused_with_a_plan` (a result in the family whose read answers an
outcome this route's writer's own results have no member for is refused only where a plan is compared,
the call id and recorded text unchanged) and
`test_a_call_without_a_usable_name_is_no_call` (a call whose `name` is not the non-empty string the
carried reader registers is refused with its orphaned output, and no receipt — the fixture's own or one
edited to `name: ""` — can certify the pair) — with the round's four mutations and their executed
reverts in §11. The r7 tree this quote is taken on holds twenty-six: the twenty-three above plus this
round's three cases —
`test_a_result_the_receipt_cannot_establish_is_refused_with_a_plan` (a result whose call the read
answers elsewhere — the finding's `wait_agent` rename, the `spawn_agent` and non-rerouted `wait` arms
among them — or whose output no read appends a result from is refused where a plan is compared, with the
exec-cell `wait` poll as the green control),
`test_a_non_string_identity_member_is_refused_with_a_plan` (a call's recorded input or a result's
recorded text written as a non-string member is refused where a plan is compared, the coerced JSON
spelling it would otherwise be matched on pinned beside it, and the explicit-`null` identity `null`
distinguished from the absent member's empty string) and
`test_a_moved_turn_context_is_outside_the_receipts_vocabulary` (the finding's own `turn_context` move
validates green: neither document names a turn, which is the scope the order rule states) — with the
round's four mutations and their executed reverts in §12.

The r2 round's own cases were also run against the r1 checker from a throw-away copy (`git show
87d3018:scripts/codex-native-check.py` into `/tmp/probe-r1/scripts/`, with this round's test module and
fixtures beside it), so each one is a case about its finding rather than a restatement of the shipped
code: all six fail there with the defect's own signature — the call substitution refused nothing
(`[] != ['oracle.plan-accounted', 'oracle.plan-emitted']`), the appended empty boundary was not the last
occurrence, U+001C dialogue drew `oracle.message`/`oracle.event-message` violations, the no-echo case
found `timestamp {'prompt': 'SECRET-PROMPT-ALPHA'}` in the printed report, the nesting arm ended in
`RecursionError` instead of exit 2, and the mutation-table case found the two new mutations turning
nothing red.

The mutations number 51 for the 14 rules: `oracle.plan-accounted` carries twelve; `oracle.envelope`
seven; `oracle.jsonl-objects` five; `oracle.receipt`, `oracle.session-meta` and `oracle.call-pairing`
four each; `oracle.message` and `oracle.session-meta-members` three each; `oracle.plan-emitted`,
`oracle.event-message` and `oracle.plan-order` two each; and `oracle.record-type`, `oracle.clock` and
`oracle.plan-pairs` one each. The r7 round added four of them — a record without its envelope `type`
(`oracle.envelope`), the metadata record without its `originator` (`oracle.session-meta-members`), a
result under a call the read answers elsewhere and a call whose recorded input is not a string (both
`oracle.plan-accounted`) — and §12 shows each one red only with this round's code in place. The r6 round
added four of them — a record standing before the metadata
(`oracle.session-meta`), a call without a usable name (`oracle.call-pairing`), the plan's omitted prompt
written into a `developer`-role message and a recorded result rewritten into the custom tool family (both
`oracle.plan-accounted`) — and §11 shows each one red only with this round's code in place. The r5 round
added one of them — a line holding a member name twice, under
`oracle.jsonl-objects`, spliced into the raw JSON text because no dict can hold the repeated name — and
§10 shows it, and the round's other two refusals, red only with this round's code in place. The r4 round
added three of them — the omitted prompt written into an inter-agent message, into an item type the
receipt cannot name, and an image-generation call the receipt cannot name, all under
`oracle.plan-accounted` — and §9 shows each one red only with this round's refusal
in place; the r3 round added five — the trailing line feed and the non-ASCII digit under
`oracle.envelope`, the output written before its call under `oracle.call-pairing`, the receipt that
declares no occurrences under `oracle.receipt`, and the boundary carrying the omitted dialogue under
`oracle.plan-accounted` — and §8 shows each one surviving its fix's absence.
Two of the fifty-one, run
through the shipped command rather than in-process, show what the report reads like — the first a
structural rule, the second the plan/pair rule that is the reason the receipt exists at all (a call
whose recorded result is left out while the call travels). Each file below is a copy of a committed
fixture changed in exactly one way, by throw-away commands under the replay's own root:

```
$ python3 - <<'PY' > "$root_scratch/cli-version-dropped.jsonl"
import json
lines = open("scripts/tests/testdata/codex-oracle/session.jsonl").read().splitlines()
record = json.loads(lines[0])
record["payload"].pop("cli_version")
lines[0] = json.dumps(record, separators=(",", ":"))
print("\n".join(lines))
PY
$ python3 - <<'PY' > "$root_scratch/pair-split.jsonl"
import json
lines = [line for line in open("scripts/tests/testdata/codex-oracle/session.jsonl").read().splitlines()
         if '"function_call_output"' not in line]
print("\n".join(lines))
PY
$ python3 - <<'PY' > "$root_scratch/pair-split-receipt.json"
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
$ python3 scripts/codex-native-check.py --validate "$root_scratch/cli-version-dropped.jsonl"
oracle: oracle.session-meta-members line 1 — session_meta.cli_version is null, not a non-empty string
oracle: FAIL — 17 record(s), 7 occurrence(s), 1 violation(s)
$ echo $?
1
```

```
$ python3 scripts/codex-native-check.py --validate "$root_scratch/pair-split.jsonl" \
      --receipt "$root_scratch/pair-split-receipt.json"
oracle: oracle.plan-pairs — call sha256:a28e32f073a7: the receipt gives the call and its recorded result different dispositions — a call and its recorded result travel together
oracle: FAIL — 16 record(s), 6 occurrence(s), 1 violation(s)
$ echo $?
1
```

The mode's own edges were run by hand as well, because a validator's failure modes are part of its
surface: an artifact holding no records at all is refused by the rule that opens the artifact (not by a
crash), an input that cannot be read at all or whose bytes are not UTF-8 text is `INCOMPLETE` with exit 2
— never a pass — and a receipt document that is not a plan (a file holding `null`) is a supplied value
the receipt rule refuses rather than an absent receipt (the directory case's input is created first —
review round r7's finding: the earlier recipe read a directory no command in the record made):

```
$ : > "$root_scratch/empty.jsonl" && python3 scripts/codex-native-check.py --validate "$root_scratch/empty.jsonl"
oracle: oracle.session-meta — the artifact holds no records
oracle: FAIL — 0 record(s), 0 occurrence(s), 1 violation(s)
$ echo $?
1
$ mkdir -p "$root_scratch/a-directory" && python3 scripts/codex-native-check.py --validate "$root_scratch/a-directory"
oracle: INCOMPLETE — the artifact /tmp/ccc-0009a-evidence.jrYpjh/a-directory cannot be read: [Errno 21] Is a directory: '/tmp/ccc-0009a-evidence.jrYpjh/a-directory'
$ echo $?
2
$ printf 'not json\n' > "$root_scratch/not-json.txt" && python3 scripts/codex-native-check.py --validate "$root_scratch/not-json.txt"
oracle: oracle.session-meta — the artifact holds no records
oracle: oracle.jsonl-objects line 1 — not JSON (Expecting value at column 1)
oracle: FAIL — 0 record(s), 0 occurrence(s), 2 violation(s)
$ echo $?
1
$ printf '{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"id":"\xff"}}\n' > "$root_scratch/not-utf8.jsonl" \
  && python3 scripts/codex-native-check.py --validate "$root_scratch/not-utf8.jsonl"
oracle: INCOMPLETE — the artifact /tmp/ccc-0009a-evidence.jrYpjh/not-utf8.jsonl is not UTF-8 text: 'utf-8' codec can't decode byte 0xff in position 75: invalid start byte
$ echo $?
2
$ printf 'null' > "$root_scratch/null-receipt.json" \
  && python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/session.jsonl \
      --receipt "$root_scratch/null-receipt.json"
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
$ mkdir -p "$root_scratch/reduced-only"
$ cp scripts/tests/testdata/codex-oracle/reduced.jsonl \
     scripts/tests/testdata/codex-oracle/reduced-receipt.json "$root_scratch/reduced-only/"
$ ls -l "$root_scratch/reduced-only"
total 8
-rw-rw-r-- 1 azureuser azureuser  814 Nov 21 06:51 reduced-receipt.json
-rw-rw-r-- 1 azureuser azureuser 1973 Nov 21 06:51 reduced.jsonl
$ python3 scripts/codex-native-check.py --validate "$root_scratch/reduced-only/reduced.jsonl" \
      --receipt "$root_scratch/reduced-only/reduced-receipt.json"
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
      --receipt "$root_scratch/reduced-only/reduced-receipt.json"
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
native path's first line. That is measured, not asserted — the shipped check was run twice on the r3
tree, once against a pristine extraction of the base revision pinned by **sha** (`git archive
21d43fe1a3c81bb59504bfaae23c8c618edf7ae2`, the commit this increment started from; named by its sha
because `git archive HEAD` extracts whatever head the caller is on — after the increment that is the
reviewed head, not the baseline), and once in this tree, both times with no prompt flag (no model turn,
`--resume-prompt` unset). The block below is the whole recipe: it creates its scratch directory first and
derives the checkout from the shell's own working directory (`repo=$PWD`, run from the repository root),
so a fresh checkout can follow it as written (review round r3's finding: the earlier block extracted into
a directory it never created and named the author's checkout for the after run). The two runs' check
lines are identical in name and state (28 lines each: 26 `PASS` + 2 `SKIP`; the session ids and digests
differ, as fresh identities always do), and both exit 0:

```
$ repo=$PWD                                     # the checkout this record was taken in (its root)
$ scratch="$root_scratch/d3"
$ mkdir -p "$scratch/before-tree"
$ git -C "$repo" archive 21d43fe1a3c81bb59504bfaae23c8c618edf7ae2 | tar -x -C "$scratch/before-tree"
$ (cd "$scratch/before-tree" && python3 scripts/codex-native-check.py \
      --repo "$scratch/before-tree" > "$scratch/native-check-before.txt" 2>&1); echo $?
0
$ python3 scripts/codex-native-check.py > "$scratch/native-check-after.txt" 2>&1; echo $?
0
$ tail -1 "$scratch/native-check-before.txt"
26 check(s) passed, 0 failed, 2 skipped
$ tail -1 "$scratch/native-check-after.txt"
26 check(s) passed, 0 failed, 2 skipped
$ diff <(grep -oE '^(PASS|FAIL|SKIP): [^—]*' "$scratch/native-check-before.txt") \
       <(grep -oE '^(PASS|FAIL|SKIP): [^—]*' "$scratch/native-check-after.txt") && echo identical
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

The r1–r7 review fix rounds changed the oracle section, its tests and its fixtures, and no Go
file (the increment's Bounds; `git status --short | grep '\.go$'` prints nothing): the same unflagged
command was run once more over each fixed tree (most recently r7's, with the portable recipe above), and
each run's 28 check lines' names and states are identical to the before-run's and to the increment's
earlier after-runs, with the same closing count (`26 check(s) passed, 0 failed, 2 skipped`) and exit 0 —
so D3's claim holds for the increment's final bytes. The r7 run's own pair is the one this section
quotes: `native-check-before.txt` from the base revision (`21d43fe1`, `exit 0`) and
`native-check-after.txt` from this tree (`exit 0`), their 28 check lines `identical` under the recipe's
own `diff`, and the after-run's tail is the block above verbatim.

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
acceptance runs — validated where it landed (the probe derives the artifact's path from the conversion
report, r2's finding 7: it was first written reading `"$artifact"` without ever assigning it):

```
$ go build -tags=fts5 -o "$root_scratch/ccc-r7" ./cmd/ccc    # exit 0
$ mkdir -p "$root_scratch/conversion/home"
$ CLAUDE_PROJECTS_DIR=$PWD/internal/ccc/testdata/fixtures/claude \
  "$root_scratch/ccc-r7" continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex \
      --target-home "$root_scratch/conversion/home" --target-binary codex --json \
      > "$root_scratch/conversion/document.json" ; echo $?
0
$ artifact=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["target_session"]["path"])' \
      "$root_scratch/conversion/document.json") && echo "$artifact"
/tmp/ccc-0009a-evidence.jrYpjh/conversion/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1fb28-a878-7169-9bc9-761198635844.jsonl
$ python3 scripts/codex-native-check.py --validate "$artifact" --json
{
  "artifact": "/tmp/ccc-0009a-evidence.jrYpjh/conversion/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1fb28-a878-7169-9bc9-761198635844.jsonl",
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
handed in here.) Re-measured in round 7, with this record's own tree: a fresh conversion of the same
fixture (the same `continue claude:… --to codex` invocation, into
`$root_scratch/conversion/home`, the artifact path above) validates as
`oracle: PASS — 17 record(s), 8 occurrence(s)`
with exit 0 — the round's added refusals do not refuse an artifact this route installs.

Second, the other direction of soundness: the oracle accepts every artifact the format's own writer
produced on this host, not only the ones this build writes. The sweep is also the output-before-call
rule's own measurement against those files (r3): every native session stands each output after its call
under the rule (25 green), so no file the format's own writer produced on this host writes an output
first.

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
does not: no row claims a Go test runs the oracle. The `oracle.jsonl-objects` row is explicit that its
cited test pins the one-object clause alone — the rule's member-name-uniqueness clause is pinned by the
`selftest` table's `a line holding a member name twice` mutation and
`test_a_line_with_a_duplicate_member_is_refused` (review round r5's finding) — and the header names
review round r5's other two refusals among the python halves. The header also names review round r6's three
cases and their mutations — a `message` in a role outside the writer's two and a `custom_tool_call_output`
refused only where a plan is compared (`oracle.plan-accounted`), and a call without the non-empty `name`
the carried reader registers refused with its orphaned output (`oracle.call-pairing`) — and review round
r7's four refusals and their mutations — a result whose call the read answers elsewhere and a call whose
recorded input or result whose recorded text is not a string (both `oracle.plan-accounted`), the
envelope's missing-`type` arm (`oracle.envelope`) and the metadata record's missing-`originator` arm
(`oracle.session-meta-members`) — because each is
re-derived by the `selftest` step's mutation table and dedicated cases (§11, §12), not by a Go test: this
increment adds none, since it changes no Go file. The five receipt rules are named there as not covered,
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
  PASS     selftest Ran 468 tests in 9.312s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5739 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009a-citations.tsv: 9 row(s) verified
  PASS     secrets  no leaks found in 208 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
$ echo $?
0
```

The `selftest` step's 468 tests are the whole `scripts/tests/` suite with this increment's oracle cases in
it (§1) — twenty-six in the class after the r7 fix round: the twenty-three of the r6 state (the twenty of
the r5 state — the nineteen of the r4 state, plus the r5 round's
`test_a_line_with_a_duplicate_member_is_refused`, with that round's new arms inside
`test_a_written_empty_object_input_is_part_of_the_identity` and
`test_receipt_validation_refuses_the_occurrences_it_cannot_name` — plus the r6 round's three, §1), plus
this round's three, §1 — and the
`citations` step's 9 rows are §6's manifest (its header names the r7 round's new cases, so the run above
is this round's). The `gofmt`, `build`, `vet` and `test` steps answer about an
unchanged Go tree: this increment's diff holds no Go file (`git status --short | grep '\.go$'` prints
nothing).

The gate (and the citations run above) was run after this round's last edit to the oracle, its tests and
the fixtures, so the transcript above is a run over the tree this record's other quotes were taken on;
this record's own prose was edited after it, and a re-run over the tree as it now stands prints the same
steps — `selftest Ran 468 tests in 9.584s`, `GATE: PASS`, exit 0. The `selftest` timing differs between runs (the r1 transcript's 8.871s, r2's 9.183s,
r3's 8.964s, r4's 9.140s, r5's 9.103s, r6's 9.387s and 9.099s and this round's 9.312s and 9.584s), which is why each transcript is one run's own line
rather than a rounded one. The r7 round's gate run is the transcript above; its exit code is `0` and its
verdict line is the last one, `GATE: PASS`.

## 8. The round-3 revert-mutations

The r3 close-out instruction was to leave each touched rule with an executed mutation rather than a
static argument, so each of the six fixes was reverted in a throw-away copy of the shipped checker and
the case that claims the rule was run against the reverted tree: the fix's own pin must fail there, and
each block below quotes the reverted snippet and the failure the run printed. The traceback frames
between the failing test's line and the assertion are elided with `...` in those quotes — the assertion
line and the run's closing count are the run's own. Five of the six
are also pinned in the mutation table itself (§1: the envelope's two new mutations, the pairing mutation,
the receipt mutation and the plan-accounted mutation), and those five studies show the table's case
failing on the mutation the fix made red-able; the sixth (the digest naming) is pinned by the no-echo
case alone.

The scratch tree each study needs is named exactly (review round r4's finding 4: the instructions used to
leave out two of its members, and a run without them fails for the wrong reason — the shipped checker
imports `scripts/native_check_common.py`, and the class's
`test_the_citations_manifest_accounts_for_every_rule` reads `docs/evidence/0009a-citations.tsv`):
`scripts/codex-native-check.py`, `scripts/native_check_common.py`,
`scripts/tests/test_codex_native_check.py`, `scripts/tests/testdata/codex-oracle/` and
`docs/evidence/0009a-citations.tsv`, laid out as §9's `fresh` block shows — one fresh tree per study,
entered before any edit or run, with the cache isolation applied to that run (review round r5's
finding 4). The r3 quotes below were taken before
that correction and their closing counts say `Ran 16 tests` — this record's r3 tree; §9's re-run of the
same revert on the r4 tree says `Ran 19 tests`.

**`oracle.envelope` — one whole ASCII instant.** The matcher reverted to a two-capture-group spelling in
the r3 record, and that regex cannot re-derive the failures it quoted: `oracle_instant` reads groups 1
through 8 (`int(match.group(index)) for index in range(1, 7)`, `match.group(7)`, `match.group(8)`), so
the shorter matcher dies inside the reader before any assertion runs — measured on the r4 tree, with the
matcher applied exactly as the r3 block prints it:

```
$ fresh instant-short
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
start = text.index("ORACLE_INSTANT = re.compile(")
end = text.index("def oracle_instant(")
reverted = '''ORACLE_INSTANT = re.compile(
    r"^(\\d{4})-\\d{2}-\\d{2}[Tt ]\\d{2}:\\d{2}:\\d{2}(?:\\.\\d{1,9})?([Zz]|[+-]\\d{2}:\\d{2})$")


'''
path.write_text(text[:start] + reverted + text[end:], encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
...
  File ".../scripts/codex-native-check.py", line 943, in oracle_instant
    year, month, day, hour, minute, second = (int(match.group(index)) for index in range(1, 7))
                                             ~~~^^^^^^^^^^^^^^^^^^^^
ValueError: invalid literal for int() with base 10: 'Z'
...
Ran 19 tests in 0.064s

FAILED (errors=65)
```

(Every case errors — the reader unpacks the matcher's sixth group into `month` and finds the zone there,
so the run never reaches an assertion; the failure is a `ValueError` on this host, not the `IndexError`
the finding named, and either way the quoted r3 assertion failures could not have come from that recipe.)
The executable revert keeps the group count and changes only what the rule is about — `$` for the
whole-string `\Z` and `\d` for the ASCII `[0-9]`:

```
$ fresh instant
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
start = text.index("ORACLE_INSTANT = re.compile(")
end = text.index("def oracle_instant(")
reverted = ('''ORACLE_INSTANT = re.compile(
    r"^(\\d{4})-(\\d{2})-(\\d{2})[Tt ](\\d{2}):(\\d{2}):(\\d{2})"
    r"(?:\\.(\\d{1,9}))?([Zz]|[+-]\\d{2}:\\d{2})$")


''')
path.write_text(text[:start] + reverted + text[end:], encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
...
AssertionError: Items in the second set but not the first:
'oracle.envelope' : 'an instant with a trailing line feed' on session.jsonl turned [] red instead of only oracle.envelope: []
...
Ran 19 tests in 0.082s

FAILED (failures=2)
```

(The other failure is the same case over `'an instant written with a non-ASCII digit'`; under the
shorter two-group matcher the run dies with `IndexError` before either mutation is judged, which is why
the r3 block's failure text could not have come from the recipe it printed.)

**`oracle.call-pairing` — ownership in record order.** The result branch reverted to checking a set
preloaded from every call in the artifact, and the output-first mutation turns nothing red:

```
$ cd /tmp/r3-mutation/pairing/scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle
...
AssertionError: Items in the second set but not the first:
'oracle.call-pairing' : 'an output written before its call' on session.jsonl turned [] red instead of only oracle.call-pairing: []
----------------------------------------------------------------------
Ran 16 tests in 0.065s

FAILED (failures=1)
```

**`oracle.receipt` — a receipt that declares nothing.** The empty-`occurrences` refusal removed, and the
case shows both arms: the metadata-only artifact passes vacuously and the session fixture draws
`plan-accounted` instead of the receipt refusal:

```
$ cd /tmp/r3-mutation/empty-receipt/scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle
...
AssertionError: Lists differ: [] != ['oracle.receipt']
...
'oracle.receipt' : 'a receipt that declares no occurrences' on session.jsonl turned ['oracle.plan-accounted'] red instead of only oracle.receipt: ...
----------------------------------------------------------------------
Ran 16 tests in 0.066s

FAILED (failures=3)
```

**`oracle.plan-accounted` — a boundary's `replacement_history`.** The comparison-time refusal removed,
and both the mutation and the dedicated case fail (nothing red where the dialogue stands in the member):

```
$ cd /tmp/r3-mutation/boundary-history/scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle
...
AssertionError: Items in the second set but not the first:
'oracle.plan-accounted' : 'a boundary carrying the dialogue the plan omits' on reduced.jsonl turned [] red instead of only oracle.plan-accounted: []
----------------------------------------------------------------------
Ran 16 tests in 0.064s

FAILED (failures=2)
```

**`oracle.receipt` / `oracle_call_arguments` — a custom call's recorded input.** The identity reader
reverted to `arguments` alone, and the custom-call case fails on the occurrence identity itself — the
input-only call is counted with an empty invocation:

```
$ cd /tmp/r3-mutation/custom-input/scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle
...
AssertionError: ('tool_call', 'call_oracle_0001', 'apply_patch', '*** Begin Patch\n*** End Patch') not found in [..., ('tool_call', 'call_oracle_0001', 'apply_patch', ''), ...] : the custom call is not counted by its recorded input
----------------------------------------------------------------------
Ran 16 tests in 0.063s

FAILED (failures=1)
```

**The digest naming — default diagnostics echo no string `call_id`.** The five digest sites reverted to
`{call_id!r}`/`{key[1]!r}`, and the no-echo case prints the marker it planted:

```
$ cd /tmp/r3-mutation/digest/scripts/tests && python3 -m unittest test_codex_native_check.CodexArtifactOracle.test_a_malformed_member_is_named_without_echoing_its_text
...
AssertionError: 'call output names call_id sha256:a2f5c40261d1' not found in "oracle: ... call output names call_id 'call SECRET-PROMPT-ALPHA', which no call written before it carries\noracle: oracle.plan-accounted line 9 — tool result 'call SECRET-PROMPT-ALPHA' (8 character(s)): ..."
----------------------------------------------------------------------
Ran 1 test in 0.004s

FAILED (failures=1)
```

## 9. The round-4 revert-mutations

The r4 close-out instruction was r3's, applied to this round's surface: every rule the touched surface
claims was reverted in a throw-away copy of the shipped checker, the case that claims it was run against
the reverted tree, and each block below quotes the revert's own command and the failure the run printed —
so a rule that survived its revert without failing would be visible here as an unpinned one. The scratch
tree is written out in full, because two of its members are easy to leave out and both are load-bearing:
`scripts/native_check_common.py`, which the shipped checker imports, and
`docs/evidence/0009a-citations.tsv`, which the class's own
`test_the_citations_manifest_accounts_for_every_rule` reads (review round r4's finding 4: §8's
instructions named neither, and the manifest it did not name makes that case error before any rule under
study fails).

**Each study runs in a fresh, explicitly entered scratch tree (review round r5's finding 4; its own
directory is unique since review round r7's finding 4 — the recipe used to `rm -rf` the path it was
about to create, which would remove whatever was already there).** The setup
as first printed left the caller in the repository after the baseline's subshell, so the next study's
`sed -i '…' scripts/codex-native-check.py` would edit the **live checkout** when read from the repository
root, and one shared tree carried each study's revert into the next. The blocks below are re-spelled so a
reader cannot do that: `fresh` creates one **new** tree per study named by its argument — a `mktemp -d`
directory under the replay's own `$root_scratch`, never an existing path — reads the five files
from the repository root captured in `$root`, and `cd`s into that tree; every edit and every run is then
relative to the tree just entered, and every run carries its own `PYTHONPYCACHEPREFIX` for the reason
§1's r2 note gives (the loaded-from-source module's whole-second mtime cache can serve the pre-revert
revision). The failure text is the r4 run's own; the byte edits it names are the ones the re-spelled
commands make.

```
$ root=$PWD    # the repository root: the only directory the copying commands read from
$ fresh() {
    scratch=$(mktemp -d "$root_scratch/study-$1.XXXXXX")
    mkdir -p "$scratch/scripts/tests" "$scratch/docs/evidence"
    cp "$root/scripts/codex-native-check.py" "$root/scripts/native_check_common.py" "$scratch/scripts/"
    cp "$root/scripts/tests/test_codex_native_check.py" "$scratch/scripts/tests/"
    cp -r "$root/scripts/tests/testdata" "$scratch/scripts/tests/testdata"
    cp "$root/docs/evidence/0009a-citations.tsv" "$scratch/docs/evidence/"
    cd "$scratch"
}
$ fresh baseline
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
...
Ran 26 tests in 0.106s

OK
```

**`oracle.plan-accounted` — the occurrences the receipt cannot name.** The comparison-time refusal removed
(the call deleted from `oracle_reading`), the class's three new mutations turn nothing red and the
dedicated case finds nothing refused:

```
$ fresh plan-accounted
$ sed -i '/violations += oracle_unaccounted_occurrence_violations(records)/d' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
...
AssertionError: Items in the second set but not the first:
'oracle.plan-accounted' : 'an omitted message written into an inter-agent message' on reduced.jsonl turned [] red instead of only oracle.plan-accounted: []
----------------------------------------------------------------------
Ran 19 tests in 0.074s

FAILED (failures=4)
```

(The other three failures are the same case over `'an omitted message written into an item type the
receipt cannot name'` and `'an image-generation call the receipt cannot name'`, and the dedicated case's
`[] != ['oracle.plan-accounted']` on the finding's own inter-agent attack.)

**`oracle.receipt` — a call's recorded input is what the artifact writes.** The identity reader reverted
to the carried read's projection (the r3 body plus the `_oracle_valid_json` it calls), and the
written-input case fails on the occurrence identity itself while the decoded-input case shows the nested
input escaping validation as a `RecursionError`:

```
$ fresh call-arguments
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
start = text.index("def oracle_call_arguments(")
end = text.index("def oracle_boundary_text(")
old = '''def oracle_call_arguments(payload: dict) -> str:
    """The r3 reader-mirror reverted onto this tree, for the mutation study."""
    for member in ("arguments", "input"):
        if member not in payload:
            continue
        value = payload[member]
        if isinstance(value, str):
            trimmed = value.strip()
            if trimmed == "" or trimmed in ("{}", "[]"):
                continue
            return trimmed if _oracle_valid_json(trimmed) else value
        raw = json.dumps(value, separators=(",", ":"), ensure_ascii=False)
        if raw in ("{}", "[]"):
            continue
        return raw
    return ""


def _oracle_valid_json(text: str) -> bool:
    try:
        json.loads(text, parse_constant=_reject_json_constant)
    except ValueError:
        return False
    return True


'''
path.write_text(text[:start] + old + text[end:], encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
...
AssertionError: ('tool_call', 'call_oracle_0001', 'Read', '{}') not found in [..., ('tool_call', 'call_oracle_0001', 'Read', ''), ...] : the written empty object is not the call's recorded input
...
RecursionError: Stack overflow (used 8152 kB) while decoding a JSON array from a unicode string
...
Ran 19 tests in 0.082s

FAILED (failures=1, errors=1)
```

(The first failure is `test_a_written_empty_object_input_is_part_of_the_identity`; the error is
`test_decoded_input_never_aborts_validation`'s nested arm, with the `RecursionError` raised out of
`oracle_call_arguments` → `_oracle_valid_json` → `json.loads` and escaping `oracle_report`.)

**`oracle_digest` — a decoded string strict UTF-8 cannot encode.** The digest reverted to strict
`utf-8`, and the finding's own path — an unmatched output whose `call_id` is a lone surrogate — ends the
shipped CLI in a traceback instead of a verdict. The surrogate-bearing artifact is generated inside the
scratch tree first (review round r6's finding: the recipe as first printed read the input from outside
the tree and reached the missing-artifact `INCOMPLETE` branch instead of this traceback, so the
generation is part of the recipe now):

```
$ fresh digest
$ python3 - <<'PY'
import json
lines = open("scripts/tests/testdata/codex-oracle/session.jsonl", encoding="utf-8").read().splitlines()
for index, line in enumerate(lines):
    record = json.loads(line)
    payload = record.get("payload") or {}
    if payload.get("type") == "function_call_output" and payload.get("call_id") == "call_oracle_0001":
        payload["call_id"] = "\ud800"
        lines[index] = json.dumps(record, separators=(",", ":"))
        break
else:
    raise SystemExit("the fixture no longer holds the output this study edits")
open("r4-surrogate.jsonl", "w", encoding="utf-8").write("\n".join(lines) + "\n")
PY
$ sed -i "s/text.encode('utf-8', 'surrogatepass')/text.encode('utf-8')/" scripts/codex-native-check.py
$ PYTHONPYCACHEPREFIX="$scratch/pyc" python3 scripts/codex-native-check.py --validate r4-surrogate.jsonl
Traceback (most recent call last):
  ...
  File ".../scripts/codex-native-check.py", line 1396, in oracle_call_pairing_violations
    f"call output names call_id {oracle_digest(call_id)}, which "
                                 ~~~~~~~~~~~~~^^^^^^^^^^^^^^^^^
  File ".../scripts/codex-native-check.py", line 1122, in oracle_digest
    return f"sha256:{hashlib.sha256(text.encode('utf-8')).hexdigest()[:12]}"
                                    ~~~~~~~~~~~^^^^^^^^^
UnicodeEncodeError: 'utf-8' codec can't encode character '\ud800' in position 0: surrogates not allowed
exit 1
```

(The quoted traceback is the r6 re-run's own, from the recipe above: `json.dumps` escapes the lone
surrogate as `\ud800` in the line, this oracle's decoder turns the escape back into the code point, and
the digest then cannot encode it — the line numbers are the tree this recipe makes. The class's case
fails there too, at its own digest literal — `oracle_digest("\ud800")` raises before the CLI arm is
reached; `test_decoded_input_never_aborts_validation` is the case the CLI arm above belongs to.)
**§8's instant recipe** is the fourth study: its revert is re-derived in §8 itself, where the exact
command and the failure it prints now stand.

**The finding's own artifacts, before and after.** The three attacks and the written-input arms were also
driven through the shipped `--validate` against the artifacts the finding names, with the pre-round checker
(`git show 1fb171b:scripts/codex-native-check.py` into a copy of the scratch tree) as the before arm. Each
plant is a copy of a committed fixture with one record or member written by a throw-away command, the same
shapes §1's mutation table builds: the reduced plan's omitted prompt (`"And now?"`) as an appended
inter-agent message's delivered content, as a `compat_message` payload's readable text, and as an
`image_generation_call`'s sibling record; the session fixture's call with `arguments: "{}"`, and with the
member removed (its receipt naming the written `"{}"`, and the read's empty answer as a second receipt);
and the call's input written as 200 000 nested brackets. The before arm certifies every dialogue attack
and accepts the read's empty identity for the written `{}`; the after arm refuses each one:

```
$ (cd "$root_scratch/r4-before" && python3 scripts/codex-native-check.py --validate attack-agent-message.jsonl \
      --receipt scripts/tests/testdata/codex-oracle/reduced-receipt.json)
oracle: PASS — 13 record(s), 5 occurrence(s)
exit 0
$ python3 scripts/codex-native-check.py --validate "$root_scratch/attack-agent-message.jsonl" \
      --receipt scripts/tests/testdata/codex-oracle/reduced-receipt.json
oracle: oracle.plan-accounted line 13 — the artifact holds an inter-agent `agent_message` addressed to the session's own agent path carrying delivered text (a string of 8 character(s)); the receipt's vocabulary cannot name that occurrence, so the plan cannot account for it — this route's writer writes no such record
oracle: FAIL — 13 record(s), 5 occurrence(s), 1 violation(s)
exit 1
```

The other two dialogue attacks repeat the same pair (`PASS — 13 record(s), 5 occurrence(s)` before,
one `oracle.plan-accounted` violation after: `... whose payload type is a string of 14 character(s)
carrying a `user` role and readable text (a string of 8 character(s)) ...` for the unknown item, and
`... an `image_generation_call`, which a read appends as a tool call whose identity (the payload's `id`
and raw text) ...` for the third). The written-input pair:

```
$ (cd "$root_scratch/r4-before" && python3 scripts/codex-native-check.py --validate written-empty-object.jsonl \
      --receipt written-empty-object-receipt.json)
oracle: oracle.plan-emitted — tool call sha256:a28e32f073a7: the receipt marks 1 emitted, the artifact holds 0
oracle: oracle.plan-accounted line 8 — tool call sha256:a28e32f073a7: the artifact holds 1, the receipt marks 0 emitted (the receipt names no occurrence with this identity)
oracle: FAIL — 17 record(s), 7 occurrence(s), 2 violation(s)
exit 1
$ python3 scripts/codex-native-check.py --validate "$root_scratch/written-empty-object.jsonl" \
      --receipt "$root_scratch/written-empty-object-receipt.json"
oracle: PASS — 17 record(s), 7 occurrence(s)
exit 0
```

and the read's empty identity, which the before arm reads as the same occurrence and the after arm
refuses (the same two `oracle.plan-emitted`/`oracle.plan-accounted` lines, `FAIL`, exit 1), while the
removed member matches that identity in both arms (the written form and the removed member are two
occurrences, each named by its own receipt). The nested input:

```
$ (cd "$root_scratch/r4-before" && python3 scripts/codex-native-check.py --validate nested-input.jsonl)
...
RecursionError: Stack overflow (used 8156 kB) while decoding a JSON array from a unicode string
exit 1
$ python3 scripts/codex-native-check.py --validate "$root_scratch/nested-input.jsonl"
oracle: PASS — 17 record(s), 7 occurrence(s)
exit 0
```

## 10. The round-5 revert-mutations

The r5 close-out instruction was r3's and r4's, applied to this round's surface: every rule the touched
surface claims was reverted in a fresh, throw-away tree of its own, the case that claims it was run
against the reverted tree, and each block below quotes the revert's own command and the failure the run
printed — so a rule that survived its revert without failing would be visible here as an unpinned one.
The trees are §9's `fresh` recipe (one tree per study, entered before the edit; each run carrying its own
`PYTHONPYCACHEPREFIX`), and the failure text is each run's own. The shipped tree is not one of them: the
studies read the five files from it and wrote only the trees `fresh` created.

**`oracle.jsonl-objects` — a member name written twice (the artifact reader).** Both
`object_pairs_hook=_oracle_unique_members` call sites removed, and the new case, the no-echo case's name
arm and the mutation table each fail:

```
$ fresh uniqueness
$ sed -i 's/, object_pairs_hook=_oracle_unique_members//; s/object_pairs_hook=_oracle_unique_members//' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
....FF.....F........
...
AssertionError: Lists differ: [] != ['oracle.jsonl-objects']
...
FAIL: test_a_malformed_member_is_named_without_echoing_its_text ... AssertionError: 0 != 1 : exit 0 for the planted documents: oracle: PASS — 17 record(s), 7 occurrence(s)
...
FAIL: test_every_rule_has_a_mutation_that_turns_its_fixture_red ... (rule='oracle.jsonl-objects', mutation='a line holding a member name twice')
...
'oracle.jsonl-objects' : 'a line holding a member name twice' on session.jsonl turned [] red instead of only oracle.jsonl-objects: []
...
Ran 20 tests in 0.083s

FAILED (failures=3)
```

**The receipt reader's own half of the same rule.** Only `oracle_report`'s hook removed — the artifact
line reader keeps its uniqueness — and the duplicate case fails at its CLI arm, because the collided
receipt is read (last-wins) and compared instead of being refused as unreadable:

```
$ fresh receipt-uniqueness
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
old = ('document = json.loads(raw.decode("utf-8"), parse_constant=_reject_json_constant,\n'
       '                                  object_pairs_hook=_oracle_unique_members)')
new = 'document = json.loads(raw.decode("utf-8"), parse_constant=_reject_json_constant)'
assert old in text, "the receipt parse is no longer spelled this way"
path.write_text(text.replace(old, new, 1), encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle.test_a_line_with_a_duplicate_member_is_refused)
oracle: oracle.plan-emitted — message user (22 character(s)): the receipt marks 1 emitted, the artifact holds 0
oracle: oracle.jsonl-objects line 4 — one member name is written twice in one object, so two JSON readers answer different records for the line
oracle: FAIL — 11 record(s), 4 occurrence(s), 2 violation(s)
F
...
AssertionError: 1 != 2 : exit 1 for a receipt with a repeated member:
----------------------------------------------------------------------
Ran 1 test in 0.003s

FAILED (failures=1)
```

**`oracle.plan-accounted` — an object-valued `content`.** The content guard neutralized
(`if not isinstance(content, dict):` → `if True:`, which leaves the call-members refusal before it in
force), and the fallback item's and the addressed inter-agent message's object-content arms both find
nothing refused:

```
$ fresh object-content
$ sed -i 's/if not isinstance(content, dict):/if True:/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
.............FF......
...
FAIL: test_receipt_validation_refuses_the_occurrences_it_cannot_name ... (refused='an item type the receipt cannot name, object-valued content')
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']
...
 : an item type the receipt cannot name, object-valued content was not refused as an unnameable occurrence: []
...
FAIL: ... (refused='an inter-agent message whose delivered content is an object')
...
 : an inter-agent message whose delivered content is an object was not refused as an unnameable occurrence: []
----------------------------------------------------------------------
Ran 20 tests in 0.088s

FAILED (failures=2)
```

**`oracle.plan-accounted` — a call carrying both `arguments` and `input`.** The both-members condition
neutralized (`and "arguments" in payload and "input" in payload` → `and False`), and the written-input
case's new arm certifies the two-member call through the receipt naming `"{}"`:

```
$ fresh both-members
$ sed -i 's/and "arguments" in payload and "input" in payload/and False/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
.........F..........
...
FAIL: test_a_written_empty_object_input_is_part_of_the_identity
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']
...
 : a receipt naming the written `{}` certified the two-member call: []
----------------------------------------------------------------------
Ran 20 tests in 0.083s

FAILED (failures=1)
```

## 11. The round-6 revert-mutations

The r6 close-out instruction was the r3–r5 one, applied to this round's surface: every rule the touched
surface claims was reverted in a fresh, throw-away tree of its own, the case that claims it was run
against the reverted tree, and each block below quotes the revert's own command and the failure the run
printed — so a rule that survived its revert without failing would be visible here as an unpinned one.
The trees are §9's `fresh` recipe (one tree per
study, entered before the edit; each run carrying its own `PYTHONPYCACHEPREFIX`, the reason §1's r2 note
gives), and the failure text is each run's own. The shipped tree is not one of them: the studies read the
five files from it and wrote only the trees `fresh` created.

**`oracle.session-meta` — the artifact's opening position.** The positional arm removed from the rule's
own comparison (`elif not metas or metas[0] != records[0][0]:` → `elif not metas:`), and this round's
mutation — the arm no mutation had reached before (the deleted, duplicated and emptied fixtures all fail
without it) — turns nothing red:

```
$ fresh session-meta
$ sed -i 's/elif not metas or metas\[0\] != records\[0\]\[0\]:/elif not metas:/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
..............F........
...
FAIL: test_every_rule_has_a_mutation_that_turns_its_fixture_red (test_codex_native_check.CodexArtifactOracle.test_every_rule_has_a_mutation_that_turns_its_fixture_red) (rule='oracle.session-meta', mutation='a record standing before the metadata')
...
AssertionError: Items in the second set but not the first:
'oracle.session-meta' : 'a record standing before the metadata' on session.jsonl turned [] red instead of only oracle.session-meta: []

----------------------------------------------------------------------
Ran 23 tests in 0.090s

FAILED (failures=1)
```

**`oracle.call-pairing` — the call's usable name.** The name gate deleted, and this round's mutation,
which removes exactly the member the gate reads and leaves the call id, the arguments and the recorded
output untouched, turns nothing red while the nameless record is registered — with its result attached
to it again:

```
$ fresh call-name
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
old = '''            if oracle_call_name(payload) == "":
                violations.append(Violation("oracle.call-pairing", line,
                                            f"the call's `name` is "
                                            f"{oracle_value_text(payload.get('name'))} — the carried "
                                            f"reader registers no call for a record without a non-empty "
                                            f"string `name` (`handleFunctionCall`), so no output "
                                            f"attaches to it"))
                continue
'''
assert text.count(old) == 1, "the name gate is no longer spelled this way"
path.write_text(text.replace(old, "", 1), encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
..F...........F........
...
FAIL: test_a_call_without_a_usable_name_is_no_call (test_codex_native_check.CodexArtifactOracle.test_a_call_without_a_usable_name_is_no_call)
...
AssertionError: Lists differ: [] != ['oracle.call-pairing', 'oracle.call-pairing']

Second list contains 2 additional elements.
First extra element 0:
'oracle.call-pairing'

- []
+ ['oracle.call-pairing', 'oracle.call-pairing'] : the nameless call was not refused with its orphaned output: []
...
FAIL: ... (rule='oracle.call-pairing', mutation='a call without a usable name')
...
'oracle.call-pairing' : 'a call without a usable name' on session.jsonl turned [] red instead of only oracle.call-pairing: []

----------------------------------------------------------------------
Ran 23 tests in 0.090s

FAILED (failures=2)
```

**`oracle.plan-accounted` — a `message` in a role the receipt cannot name.** The role arm's readable-text
gate neutralized (`if oracle_blank(text):` → `if True:`), and this round's mutation — the plan's omitted
prompt written into a `developer`-role message — is again a shape nothing refuses:

```
$ fresh developer-message
$ sed -i 's/^            if oracle_blank(text):$/            if True:  # reverted/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
......F.......F........
...
FAIL: test_a_developer_role_message_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_developer_role_message_is_refused_with_a_plan)
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']

Second list contains 1 additional elements.
First extra element 0:
'oracle.plan-accounted'

- []
+ ['oracle.plan-accounted'] : the omitted prompt inside a developer-role message certified the artifact: []
...
FAIL: ... (rule='oracle.plan-accounted', mutation='an omitted message written into a developer-role message')
...
'oracle.plan-accounted' : 'an omitted message written into a developer-role message' on reduced.jsonl turned [] red instead of only oracle.plan-accounted: []

----------------------------------------------------------------------
Ran 23 tests in 0.093s

FAILED (failures=2)
```

**`oracle.plan-accounted` — a result in the custom tool family.** The family's gate neutralized
(`if oracle_blank(oracle_result_text(payload)):` → `if True:`), and this round's mutation — the recorded
output's own line rewritten into the other family with its call id and text intact — is certified by the
fixture's receipt again, a read of it answering an outcome the receipt cannot state:

```
$ fresh custom-output
$ sed -i 's/^            if oracle_blank(oracle_result_text(payload)):$/            if True:  # reverted/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
.....F........F........
...
FAIL: test_a_custom_tool_call_output_is_refused_with_a_plan (test_codex_native_check.CodexArtifactOracle.test_a_custom_tool_call_output_is_refused_with_a_plan)
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']

Second list contains 1 additional elements.
First extra element 0:
'oracle.plan-accounted'

- []
+ ['oracle.plan-accounted'] : the custom family result certified the artifact: []
...
FAIL: ... (rule='oracle.plan-accounted', mutation='a recorded result rewritten into the custom tool family')
...
'oracle.plan-accounted' : 'a recorded result rewritten into the custom tool family' on session.jsonl turned [] red instead of only oracle.plan-accounted: []

----------------------------------------------------------------------
Ran 23 tests in 0.092s

FAILED (failures=2)
```

The round's fifth study — `oracle_digest`'s revert — is §9's, whose recipe this round completed (the
surrogate-bearing artifact is generated inside the scratch tree, review round r6's finding) and re-ran,
printing the traceback §9 quotes.

## 12. The round-7 revert-mutations

The r7 close-out instruction was the r3–r6 one, applied to this round's surface: every rule the touched
surface claims was reverted in a fresh, throw-away tree of its own, the case that claims it was run
against the reverted tree, and each block below quotes the revert's own command and the failure the run
printed — so a rule that survived its revert without failing would be visible here as an unpinned one.
The trees are §9's `fresh` recipe (one tree per study, entered before the edit; each run carrying its own
`PYTHONPYCACHEPREFIX`), whose own allocation this round moved to a unique directory, and the failure text
is each run's own. The shipped tree is not one of them: the studies read the five files from it and wrote
only the trees `fresh` created. The round's sixth study is the scope pin, and its *mutation* runs the
other way: the rule the round did **not** add is injected, to show that the case pinning the scope fails
the moment a later round widens it.

**`oracle.plan-accounted` — the result a read does not establish.** The refusal deleted from the reading
(`oracle_result_read_violations`' call), and this round's mutation — the finding's `wait_agent` rename —
is certified again, the dedicated case's arms and the mutation table each finding nothing refused:

```
$ fresh result-read
$ sed -i '/violations += oracle_result_read_violations(records)/d' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
..............FFFFF..F........
...
FAIL: test_a_result_the_receipt_cannot_establish_is_refused_with_a_plan ... (arm="the finding's own `wait_agent` rename")
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']
...
FAIL: test_every_rule_has_a_mutation_that_turns_its_fixture_red ... (rule='oracle.plan-accounted', mutation='a result under a call the read answers elsewhere')
...
'oracle.plan-accounted' : 'a result under a call the read answers elsewhere' on session.jsonl turned [] red instead of only oracle.plan-accounted: []
...
Ran 26 tests in 0.106s

FAILED (failures=6)
```

**`oracle.plan-accounted` — the identity member that is not a string.** Both new arm gates neutralized
(`read is not None and not isinstance(read[1], str)` and `"output" in payload and not isinstance(output,
str)`, each replaced by `if False:`), and this round's mutation — the call's `arguments` rewritten as a
JSON object — is matched by the identity's compact spelling again, while every arm of the dedicated case
finds nothing refused:

```
$ fresh non-string
$ sed -i 's/if read is not None and not isinstance(read\[1\], str):/if False:  # reverted/' scripts/codex-native-check.py
$ sed -i 's/if "output" in payload and not isinstance(output, str):/if False:  # reverted/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
..........FFFF......F........
...
FAIL: test_a_non_string_identity_member_is_refused_with_a_plan ... (arm="the call's recorded input as a JSON object, spaces after its colons")
...
AssertionError: Lists differ: [] != ['oracle.plan-accounted']
...
FAIL: ... (rule='oracle.plan-accounted', mutation='a call whose recorded input is not a string')
...
'oracle.plan-accounted' : 'a call whose recorded input is not a string' on session.jsonl turned [] red instead of only oracle.plan-accounted: []
...
Ran 26 tests in 0.111s

FAILED (failures=5)
```

**`oracle.envelope` — the record's own `type`.** The type arm's gate deleted
(`if not isinstance(kind, str) or oracle_blank(kind):` → `if False:`), and this round's mutation — the
`world_state` record's `type` member removed — turns nothing red although `oracle.record-type` is silent
about a missing one by design:

```
$ fresh envelope-type
$ sed -i 's/if not isinstance(kind, str) or oracle_blank(kind):/if False:  # reverted/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle.test_every_rule_has_a_mutation_that_turns_its_fixture_red)
F
...
'oracle.envelope' : 'a record without its type' on session.jsonl turned [] red instead of only oracle.envelope: []
...
Ran 1 test in 0.045s

FAILED (failures=1)
```

**`oracle.session-meta-members` — the metadata record's `originator`.** The member list reverted to the
two the earlier mutations reached (`("id", "cli_version")`), and this round's mutation — `originator`
removed — turns nothing red:

```
$ fresh originator
$ sed -i 's/for member in ("id", "originator", "cli_version"):/for member in ("id", "cli_version"):  # reverted/' scripts/codex-native-check.py
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle.test_every_rule_has_a_mutation_that_turns_its_fixture_red)
F
...
'oracle.session-meta-members' : 'no originator' on session.jsonl turned [] red instead of only oracle.session-meta-members: []
...
Ran 1 test in 0.046s

FAILED (failures=1)
```

**The result reader's absent-member arm.** The reader reverted to the spelling that conflated the absent
member with a written `null` (`output = payload.get("output")` then `if output is None: return ""`), and
the dedicated case's identity arms fail: the explicit-`null` result is no longer the string the carried
read answers, and the two absences are one identity again:

```
$ fresh result-text
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
old = '''    if "output" not in payload:
        return ""
    output = payload["output"]
    if isinstance(output, str):
        return output
    return json.dumps(output, separators=(",", ":"), ensure_ascii=False)'''
new = '''    output = payload.get("output")
    if isinstance(output, str):
        return output
    if output is None:
        return ""
    return json.dumps(output, separators=(",", ":"), ensure_ascii=False)'''
assert old in text, "the result reader is no longer spelled this way"
path.write_text(text.replace(old, new, 1), encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle.test_a_non_string_identity_member_is_refused_with_a_plan)
FF
...
AssertionError: ('tool_result', 'call_oracle_0001', 'null') not found in [... ('tool_result', 'call_oracle_0001', ''), ...] : the non-string member is not the occurrence's identity
...
AssertionError: '' == '' : the absent member and the written `null` are one identity
...
Ran 1 test in 0.005s

FAILED (failures=2)
```

**The scope pin — a widened rule.** The rule this round did **not** add is injected instead: a message's
model record and its view record may not stand on either side of a `turn_context` boundary. With it, the
finding's own move is refused, and the case that pins the scope fails — which is what makes the case a
pin rather than a tautology, and what will force a later round that adds turn placement to update the
rule, the record and the case together:

```
$ fresh turn-scope
$ python3 - <<'PY'
from pathlib import Path
path = Path("scripts/codex-native-check.py")
text = path.read_text(encoding="utf-8")
old = '''            view_occurrences = oracle_view_occurrences(records)
'''
new = '''            view_occurrences = oracle_view_occurrences(records)
            # injected study check: no turn boundary may fall inside a dialogue message's two records
            boundaries = [line for line, record in records if record.get("type") == "turn_context"]
            model_lines, view_lines = {}, {}
            for key, line in occurrences:
                if key[0] == "message":
                    model_lines.setdefault(key, line)
            for key, line in view_occurrences:
                view_lines.setdefault(key, line)
            for key in set(model_lines) & set(view_lines):
                low, high = sorted((model_lines[key], view_lines[key]))
                if any(low < boundary < high for boundary in boundaries):
                    violations.append(Violation("oracle.plan-order", 0,
                                                "injected study check: a turn boundary falls inside a "
                                                "dialogue message's two records"))
'''
assert text.count(old) == 1, "the reading block is no longer spelled this way"
path.write_text(text.replace(old, new, 1), encoding="utf-8")
PY
$ (cd scripts/tests && PYTHONPYCACHEPREFIX="$scratch/pyc" \
      python3 -m unittest test_codex_native_check.CodexArtifactOracle)
.........F................
...
FAIL: test_a_moved_turn_context_is_outside_the_receipts_vocabulary (...)
...
AssertionError: Lists differ: [Violation(rule='oracle.plan-order', ...)] != []
...
Ran 26 tests in 0.105s

FAILED (failures=1)
```

## 13. What this record does not claim

- **No native claim about the fixtures.** Codex never opened the synthetic fixtures or the reduced
  artifact: the oracle is the offline half, and the native half of this route stays where it was
  (the unflagged check of §3, and 0009d's own increment for the flagged path).
- **The record-type set is closed on purpose.** A Codex version that adds a record type makes the oracle
  refuse a legitimate file; that is disclosed in the rule's own sentence and in the plan's trade-offs, and
  it is the cheaper defect — an oracle whose rules cannot be broken cannot judge anything.
- **The receipt's schema is this increment's freeze of a producer that does not exist yet.** It is
  deliberately the smallest schema that can catch "the wrong call or result was omitted" (per-occurrence
  kind, identity and disposition) — a tool call's identity being its `call_id`, `name` and recorded
  input and a tool result's its `call_id` and recorded text, a dialogue message being read against both
  of the artifact's channels, and the emitted order — dialogue included — against the receipt's own in
  both channels — and 0009c/0009d must satisfy it rather than tune it: that is what the oracle
  lock is for. Nothing in this increment emits a receipt. A tool-call entry's `name` is read for its
  type, not its emptiness: an entry naming the empty string is a receipt whose *artifact* half cannot
  exist (the nameless call is refused structurally, so no artifact can satisfy the entry), which
  `test_a_call_without_a_usable_name_is_no_call` pins; the schema is not narrowed beyond that, because
  the receipt is refused by its comparison, not by its own reader.
- **A record's turn is outside the receipt's vocabulary (review round r7's finding).** The plan states
  occurrences, their identities, dispositions and order, and names no turn; the artifact's dialogue
  records name no turn either. So `oracle.plan-order` compares the two flattened sequences and nothing
  about where a record stands relative to `turn_context` records — a `turn_context` moved across records
  changes the turn the carried read answers for the records after it and still validates green. The
  rule's own sentence says so, the plan and `docs/harness.md` repeat it, and
  `test_a_moved_turn_context_is_outside_the_receipts_vocabulary` pins it (with §12's inverted study:
  injecting the turn check turns that case red, so a later round cannot widen the scope silently —
  widening it deliberately means changing the rule sentence, this record and the case together).
- **Two refusals exist only where a plan is compared (review round r7's finding).** A result whose call
  the read answers elsewhere or whose output no read appends a result from, and a call's recorded input
  or a result's recorded text written as a non-string member, are refused in receipt mode and silent
  without one — a native rollout may legitimately carry them (twelve blank outputs and 122 list-valued
  `custom_tool_call_output` outputs measured on Codex 0.170.0's own files, §0), and the structural rules
  state nothing about the arm a read takes or a member's spelling. The `wait`/`spawn_agent` exemption is
  a *mirror* of `handleFunctionCall`'s routing, not a proof that every `wait` shape reads as an ordinary
  result: the exec-cell condition is read from the call's recorded args, and args this oracle cannot read
  exactly fall on the refusing side (the same fail-closed rule `oracle_exec_cell_wait` states).
- **Form, not meaning.** A green artifact is one that is well-formed and (with a plan) holds exactly the
  occurrences the plan places; the oracle does not judge a conversation's meaning, and a red one names a
  rule, never a remedy.
