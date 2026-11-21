# 0009a — Freeze the native oracle: evidence

Contract: [`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md).
Plan: [`docs/plans/0009a-native-oracle.md`](../plans/0009a-native-oracle.md).
Citations: [`0009a-citations.tsv`](0009a-citations.tsv) (the machine-checked half of §6).
Host: Codex `0.170.0`, the pinned AgentsView carry
`83195c8a6861b8c74a4b6cb87512817fed6f8584`. Every command below ran on this host (`linux x64`), in this
repository, on branch `feat/0009a-native-oracle`; output is quoted verbatim, with host toolchain and
scratch paths elided where they appeared. **No Go file changed**: the repair's diff touches
`scripts/codex-native-check.py`, `scripts/tests/test_codex_native_check.py` and this record plus the
goal file, and nothing under `internal/` or `cmd/`.

**The mechanism repair (this record's rewrite).** The increment's first twelve review rounds found 4–6
defects every round on one surface and never closed, the reviewed diff growing monotonically
(147 KB → 611 KB). The increment answered that with the abstraction-boundary repair its decomposition
called for, on a cross-family consult's diagnosis: **D1** quantified over the checker's own Python
code ("every rule has its own negative fixture") — an obligation whose denominator grows with every
fix round — and the **receipt half modelled what an inherited *read* of the artifact produces**, a
projection the artifact's own bytes do not state. Both are replaced:

- **D1 is now a finite, executed registry** (§1). `ORACLE_RULES` is the single source of truth, the
  mutation corpus `ORACLE_MUTATIONS` lives beside it in the checker, and
  `--selftest-mutations` executes the closed equality: for every declared rule id, its positive
  fixture passes, its negative fixture fails citing that rule alone, and the same negative fixture
  passes with that rule disabled. The shell-surgery "revert recipe" replays this record used to carry
  are deleted with it; this command is the replay.
- **The receipt half was scoped to the artifact's own structure.** The rules that mirrored reader
  behaviour — a Python shadow of `internal/parser/codex.go`'s drops, rewrites, dedupes, suppressions
  and expansions (`oracle_message_read_text`, `oracle_user_message_read_text`,
  `oracle_read_consumes_subagent_notification`, `oracle_unaccounted_occurrence_violations`,
  `oracle_unmodelled_shape_violations`, `oracle_result_read_violations`, `oracle_apply_patch_files`,
  `oracle_call_input_member`, `oracle_exec_cell_wait`, `oracle_fork_lineage_violations`, and the
  helpers they shared) are **removed, not patched**: the artifact's bytes do not state what a read
  answers, so no check over them can close. Every rule that remains is decidable from the artifact's
  own bytes, and every class-1 finding of the twelve rounds has exactly one home (§5) — a surviving
  structural rule, or the reader-visible projection as the **manifest receipt's own statement**, whose
  producer is 0009c/0009d (the goal's Non-scope).
- **The contract was amended in the same increment**: D1 and D2 restated, and the Non-scope extended
  to name the reader-visible projection the receipt must carry
  ([`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md)).

## 1. The oracle, its rule registry, and the writer's own bytes (D1)

The registry is `ORACLE_RULES` — fourteen ids, one sentence each in the checker:

`oracle.jsonl-objects`, `oracle.envelope`, `oracle.record-type`, `oracle.session-meta`,
`oracle.session-meta-members`, `oracle.clock`, `oracle.message`, `oracle.event-message`,
`oracle.call-pairing`, `oracle.receipt`, `oracle.plan-emitted`, `oracle.plan-accounted`,
`oracle.plan-order`, `oracle.plan-pairs`.

Nine are structural (the artifact alone), one is the receipt's own schema, and four are stated over
the artifact and an explicit manifest receipt. Where a plan is compared, the artifact's occurrences
are read in both dialogue channels (the model-visible `response_item` and the turn-view `event_msg`)
and in the receipt's own order, and the shapes whose receipt-facing identity the artifact's own bytes
cannot state (a boundary's `replacement_history`, a call's two or non-string recorded-input members, a
result's non-string recorded-text member, a `session_meta` carrying `forked_from_id`) are refused
rather than modelled.

### 1.1 The closed equality, executed

```
$ python3 scripts/codex-native-check.py --selftest-mutations
oracle.selftest: oracle.call-pairing — an output naming no call: positive PASS, negative FAIL (oracle.call-pairing cited), --disable-rule PASS
oracle.selftest: oracle.call-pairing — a call id carried twice: positive PASS, negative FAIL (oracle.call-pairing cited), --disable-rule PASS
oracle.selftest: oracle.call-pairing — an output written before its call: positive PASS, negative FAIL (oracle.call-pairing cited), --disable-rule PASS
oracle.selftest: oracle.call-pairing — a call without a usable name: positive PASS, negative FAIL (oracle.call-pairing cited), --disable-rule PASS
oracle.selftest: oracle.call-pairing — a named call whose call_id is empty: positive PASS, negative FAIL (oracle.call-pairing cited), --disable-rule PASS
oracle.selftest: oracle.clock — two adjacent instants swapped: positive PASS, negative FAIL (oracle.clock cited), --disable-rule PASS
oracle.selftest: oracle.envelope — a record without its timestamp: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — a payload that is not an object: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — a record without its type: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — an offset the format forbids: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — an instant at the leap second: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — an instant with a trailing line feed: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.envelope — an instant written with a non-ASCII digit: positive PASS, negative FAIL (oracle.envelope cited), --disable-rule PASS
oracle.selftest: oracle.event-message — an event message that is not a string: positive PASS, negative FAIL (oracle.event-message cited), --disable-rule PASS
oracle.selftest: oracle.event-message — an event message that is blank: positive PASS, negative FAIL (oracle.event-message cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — a line that is not JSON: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — a line that parses as an array: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — a line holding a non-JSON constant: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — a line nested past reading: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — a line holding a member name twice: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.jsonl-objects — two call ids distinct to Python and one to the read: positive PASS, negative FAIL (oracle.jsonl-objects cited), --disable-rule PASS
oracle.selftest: oracle.message — a message with no content: positive PASS, negative FAIL (oracle.message cited), --disable-rule PASS
oracle.selftest: oracle.message — a block the oracle does not read: positive PASS, negative FAIL (oracle.message cited), --disable-rule PASS
oracle.selftest: oracle.message — a message whose text is blank: positive PASS, negative FAIL (oracle.message cited), --disable-rule PASS
oracle.selftest: oracle.message — a message with no role: positive PASS, negative FAIL (oracle.message cited), --disable-rule PASS
oracle.selftest: oracle.message — a block whose text is not a string beside a valid one: positive PASS, negative FAIL (oracle.message cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — an occurrence the plan marks omitted and the artifact holds: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — an occurrence the plan names nowhere: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — a boundary the plan names nowhere: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — a boundary carrying the dialogue the plan omits: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — an omitted message that survives in the turn view: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — a call whose recorded input is not a string: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — a call carrying both recorded-input members: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-accounted — a session_meta carrying explicit fork lineage: positive PASS, negative FAIL (oracle.plan-accounted cited), --disable-rule PASS
oracle.selftest: oracle.plan-emitted — a message the plan emits and the artifact lost: positive PASS, negative FAIL (oracle.plan-emitted cited), --disable-rule PASS
oracle.selftest: oracle.plan-emitted — an emitted message whose turn-view event is gone: positive PASS, negative FAIL (oracle.plan-emitted cited), --disable-rule PASS
oracle.selftest: oracle.plan-order — a compaction boundary moved past the later turns: positive PASS, negative FAIL (oracle.plan-order cited), --disable-rule PASS
oracle.selftest: oracle.plan-order — two prompts swapped between the turns' view events: positive PASS, negative FAIL (oracle.plan-order cited), --disable-rule PASS
oracle.selftest: oracle.plan-pairs — a call emitted while its recorded result is omitted: positive PASS, negative FAIL (oracle.plan-pairs cited), --disable-rule PASS
oracle.selftest: oracle.receipt — a disposition the vocabulary does not hold: positive PASS, negative FAIL (oracle.receipt cited), --disable-rule PASS
oracle.selftest: oracle.receipt — a result entry with no call_id: positive PASS, negative FAIL (oracle.receipt cited), --disable-rule PASS
oracle.selftest: oracle.receipt — a result entry with no recorded text: positive PASS, negative FAIL (oracle.receipt cited), --disable-rule PASS
oracle.selftest: oracle.receipt — a receipt that declares no occurrences: positive PASS, negative FAIL (oracle.receipt cited), --disable-rule PASS
oracle.selftest: oracle.record-type — a record type the format does not define: positive PASS, negative FAIL (oracle.record-type cited), --disable-rule PASS
oracle.selftest: oracle.session-meta — the metadata record deleted: positive PASS, negative FAIL (oracle.session-meta cited), --disable-rule PASS
oracle.selftest: oracle.session-meta — a second metadata record: positive PASS, negative FAIL (oracle.session-meta cited), --disable-rule PASS
oracle.selftest: oracle.session-meta — every record deleted: positive PASS, negative FAIL (oracle.session-meta cited), --disable-rule PASS
oracle.selftest: oracle.session-meta — a record standing before the metadata: positive PASS, negative FAIL (oracle.session-meta cited), --disable-rule PASS
oracle.selftest: oracle.session-meta-members — an empty id: positive PASS, negative FAIL (oracle.session-meta-members cited), --disable-rule PASS
oracle.selftest: oracle.session-meta-members — no cli_version: positive PASS, negative FAIL (oracle.session-meta-members cited), --disable-rule PASS
oracle.selftest: oracle.session-meta-members — no originator: positive PASS, negative FAIL (oracle.session-meta-members cited), --disable-rule PASS
oracle.selftest: PASS — 14 rule(s), 51 fixture pair(s): positive PASS, negative FAIL (each citing its own rule), --disable-rule PASS
$ echo $?
0
```

The command fails, naming the rule, on any of the ways the equality can break: a declared id with no
fixture pair, a fixture pair naming an id the registry does not declare, a mutation that edits
nothing, a negative fixture that turns a second rule red beside its own, or a negative fixture that
stays red with its own rule disabled. The suite's
`test_the_registry_and_its_corpus_are_one_closed_set` drives all three failure shapes by patching the
checker's own table, runs the shipped command once more as a subprocess and reads the committed
fixtures back around it — the run creates one scratch directory and removes it again, and touches no
committed byte.

### 1.2 The writer's own bytes

`scripts/codex-native-check.py --keep`'s scratch home (removed after the run) held a rollout this
build's writer installed for the committed Claude fixture, and the second one the run's repeat
conversion installed. The oracle over the first, with no receipt:

```
$ python3 scripts/codex-native-check.py --validate <the installed rollout>
oracle: PASS — 22 record(s), 10 occurrence(s)
```

The two committed synthetic fixtures and their manifest receipts:

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/session.jsonl \
      --receipt scripts/tests/testdata/codex-oracle/session-receipt.json
oracle: PASS — 17 record(s), 7 occurrence(s)

$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/reduced.jsonl \
      --receipt scripts/tests/testdata/codex-oracle/reduced-receipt.json
oracle: PASS — 12 record(s), 5 occurrence(s)
```

The plan comparison has teeth: the *full* artifact against the *reduced* plan — the turn the plan
marks omitted still stands in the artifact, in both dialogue channels — is refused, one finding per
record, each naming the channel:

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/session.jsonl \
      --receipt scripts/tests/testdata/codex-oracle/reduced-receipt.json
oracle: oracle.plan-accounted line 14 — message user (8 character(s)): the artifact holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 15 — message user (8 character(s)): the artifact's turn view holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 16 — message assistant (24 character(s)): the artifact's turn view holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: oracle.plan-accounted line 17 — message assistant (24 character(s)): the artifact holds 1, the receipt marks 0 emitted (the receipt marks 1 omitted)
oracle: FAIL — 17 record(s), 7 occurrence(s), 4 violation(s)
$ echo $?
1
```

The citations manifest's rows (§6) read §1 as their writer's-bytes half: each row names one Go test
that already pins the same claim about this tree's writer, and the statements those tests make — one
JSON object per line, the exact record sequence opening with one `session_meta` and its
`id`/`originator`/`cli_version`, RFC3339 instants in one nondecreasing order, one model-visible record
and one view event per message, a result only under a call already written — are the statements the
committed fixtures and the runs above re-derive from bytes.

### 1.3 The corpus

`ORACLE_MUTATIONS` holds 51 fixture pairs across the 14 rules; each pair names its fixture (and its
manifest receipt where the rule needs one) and a change that must turn exactly its own rule red and no
other, which is what makes a report a reading of one defect rather than a net. The suite's
`test_every_rule_has_a_mutation_that_turns_its_fixture_red` asserts the bijection from both ends and
the three arms of every pair — positive green, negative red citing its own rule alone, and the same
negative green with `--disable-rule` for that rule — and
`test_disable_rule_turns_exactly_one_rule_off` drives the flag through the CLI (a swapped-instant
artifact fails by default, passes with `oracle.clock` disabled, an undeclared id exits 2 naming
itself, `--disable-rule` without `--validate` is refused, and the JSON document names the rules the
reading was taken under).

## 2. D2 — the oracle needs no knowledge of the source or the selection

The oracle's input is the artifact's text and, at most, a parsed manifest receipt: no path, no source
session, no selection, no harness. The suite's
`test_a_reduced_artifact_is_validated_from_the_artifact_alone` drives the validator with
`builtins.open`, `io.open` and `os.open` refusing every file — and first shows each refusal fires — so
a reading that reached for a source transcript, the fixture on disk or any other document could not
have answered. The same claim from a scratch directory that holds nothing but the two documents:

```
$ scratch=$(mktemp -d)
$ cp scripts/tests/testdata/codex-oracle/reduced.jsonl "$scratch/artifact.jsonl"
$ cp scripts/tests/testdata/codex-oracle/reduced-receipt.json "$scratch/receipt.json"
$ cd "$scratch" && ls -A
artifact.jsonl
receipt.json
$ python3 "$repo/scripts/codex-native-check.py" --validate artifact.jsonl --receipt receipt.json
oracle: PASS — 12 record(s), 5 occurrence(s)
```

**What it does not decide (the repair's own scope).** The reader-visible projection — the records an
inherited *read* of the artifact produces — is the **manifest receipt's own statement**, whose
producer is 0009c/0009d (the goal's Non-scope). This oracle judges the artifact's own structure and
the receipt's own declarations; a shape whose receipt-facing identity the artifact alone cannot state
is refused where a plan is compared rather than read under either answer. §5 routes every class-1
finding of the twelve review rounds to a surviving structural rule or to that residual.

## 3. D3 — the existing acceptance path is unchanged

The native half of the checker is untouched by the repair: `git diff -U0 scripts/codex-native-check.py`
has hunks at line 60 (the module docstring's oracle paragraph) and at line 782 onward — the oracle
section's own header — and nothing between; the native checks end at line 772. Its re-run on the
repaired tree (each of its two conversions, its loader checks and its before/after comparisons):

```
$ python3 scripts/codex-native-check.py
...
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:…: {"messages": 7, "user_messages": 2, "assistant_messages": 5, "system_messages": 0, "tool_messages": 0, "system_records": 1, "sidechain_messages": 0, "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1}; conversion carried: 2 user message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies)
PASS: the target harness's own loader opens the installed session — thread/resume -> … [legacy] cwd=/home/user/code/example-api
PASS: the target harness's own loader opens the second installed session — thread/resume -> … [legacy] cwd=/home/user/code/example-api
...
26 check(s) passed, 0 failed, 2 skipped
$ echo $?
0
```

The recorded before verdict (this record's earlier revision) was the same: 26 checks passed, 0 failed,
2 skipped, exit 0. `--resume-prompt`/`--repeat-resume-prompt` were not run here: each runs one real
turn and needs model access, and the D3 statement is about the acceptance path's verdict, which the
run above (its two conversions, its two loader opens and its before/after comparisons) answers.

## 4. D4 — the freeze, stated where a later lane reads it

The checker's own section header states the oracle lock and its mechanical reason — a fix round that
moves this checker *alongside* the production code it judges moves both sides of "the implementation
satisfies the oracle" at once — and names the registry plus `--selftest-mutations` as the contract's
executed end. `docs/harness.md` and `AGENTS.md` carry the freeze for the lanes that read them, and
the conductor's oracle-lock signal is the enforcement, not a command in this tree.

**One disclosure.** `docs/harness.md`'s oracle paragraphs still describe the pre-repair refusal
surface — the reader-modelling rules this repair removed. That file is outside this repair's bounds
(the repair may change the checker, its tests, its fixtures, this record and the goal file), so the
stale description stands, named here rather than left silent; the checker's own section header is the
current statement of the oracle's surface, and the harness document needs the operator's own pass to
match it.

## 5. The class-1 findings, each with one home

The twelve review rounds' P1 findings and where each now lives. **Kept** names the rule id (and the
fixture pair or case) that decides the same defect from the artifact's own bytes; **residual** means
the defect needs the reader-visible projection, which is the manifest receipt's own statement the
receipt's producer (0009c/0009d) must carry — it is not decidable from the artifact and a receipt
alone, and no check over them can stand in for it.

| round | finding | disposition |
| --- | --- | --- |
| r1 | a receipt blind to which tool result survived | kept — a result's identity is its `call_id` and recorded text (`oracle.receipt`/`oracle.plan-emitted`/`oracle.plan-accounted`; `test_a_result_substituted_under_one_call_id_is_refused`) |
| r1 | excluded dialogue surviving in the turn-view channel | kept — the turn-view half of `oracle.plan-emitted`/`oracle.plan-accounted`; fixture "an omitted message that survives in the turn view" |
| r2 | the same `call_id` certifying a different invocation | kept — a call's identity is its `call_id`, written `name` and written input (`test_a_call_substituted_under_one_call_id_is_refused`) |
| r2 | a `compacted` record without a summary bypassing accounting | kept — every `compacted` record is a boundary occurrence; fixtures "a boundary the plan names nowhere", "a boundary without a summary" |
| r3 | dialogue inside a boundary's `replacement_history` | kept — refused where a plan is compared; fixture "a boundary carrying the dialogue the plan omits" |
| r3 | a custom call's `input` member ignored by the identity | kept — the identity reads the written `input` member for either family (`test_a_custom_calls_recorded_input_is_part_of_its_identity`) |
| r4 | the omitted prompt written into an inter-agent `agent_message` | residual — the record's read-answered role is a projection of the read |
| r4 | the writer's `arguments: "{}"` against a missing member | kept — the identity is the member the record writes (`test_a_written_empty_object_input_is_part_of_the_identity`) |
| r5 | a line whose object writes a member name twice | kept — `oracle.jsonl-objects`; fixture "a line holding a member name twice" |
| r5 | an object-valued `content` a read iterates | residual; on the vocabulary's own `message` item the shape is refused structurally (`oracle.message` requires a list) |
| r5 | a call carrying both `arguments` and `input` | kept — refused where a plan is compared; fixture "a call carrying both recorded-input members" |
| r6 | extra developer-role messages | residual — which roles a read answers is a projection of the read |
| r6 | a `custom_tool_call_output`'s read-answered outcome | residual |
| r7 | a `wait_agent` output whose read appends no result | residual |
| r8 | the ordinary review budget tripped (process) | this repair is the answer the rule asks for (an abstraction-boundary failure, re-decomposed); the rounds' ledger is the earlier revision of this record, and the dispatch was the conductor's |
| r8 | two results the read trims to one | residual |
| r9 | a subagent notification the read consumes | residual |
| r9 | lone-surrogate escapes making two call ids one | kept — `oracle.jsonl-objects` refuses the line before any identity comparison; fixture "two call ids distinct to Python and one to the read" |
| r10 | notification-prefixed user messages the read drops | residual |
| r10 | a padded `wait_agent` name the read trims before routing | residual |
| r11 | call ids colliding after the read's trim | residual |
| r12 | a documented replay able to edit the live checkout | removed with the shell replays; the replay is `--selftest-mutations` (§1.1), which writes only under the scratch directory it creates and removes |
| r12 | replayed parent history certified under `forked_from_id` | kept — refused where a plan is compared; fixture "a session_meta carrying explicit fork lineage" |

The remaining P2/P3 findings of the twelve rounds that touched this surface are pinned in the suite
rather than routed: diagnostics that name a malformed value by place and kind and never echo it
(`test_a_malformed_member_is_named_without_echoing_its_text`, and the digest naming of artifact- and
receipt-controlled strings), the leap second and trailing-line-feed and non-ASCII-digit instants
(three `oracle.envelope` fixtures), the empty and unreadable receipt dispositions
(`test_a_receipt_that_declares_nothing_cannot_pass`, the unreadable-receipt CLI arms), the
`RecursionError` that must end `INCOMPLETE` rather than escape (the receipt nesting arm), the
strict-UTF-8 reading of the artifact's own bytes
(`test_the_oracle_reads_the_artifacts_own_bytes`), the iterative surrogate scan
(`test_a_deeply_nested_object_is_scanned_without_aborting`), the writer's own blank test
(`test_the_oracles_blank_test_is_the_writers`), the counted multiplicity
(`test_occurrence_identity_is_counted_not_deduplicated`), and the D2 file-refusal and CLI-verdict
cases. The replay-path findings (r8, r9, r11, r12 among them) are answered by deleting the recipes:
this record quotes only commands whose output it shows, and `--selftest-mutations` is the one
mutation replay.

## 6. Citations

`docs/evidence/0009a-citations.tsv` holds this increment's rows: one row per structural oracle rule,
each naming the existing Go test that already pins the same claim about this tree's writer
(`TestCodexRolloutRecordShape`, `TestCodexRolloutStampsRecordsInOneOrderedRun`,
`TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage`,
`TestCodexRolloutDoesNotReportOmittedResultsAsCarried`), plus the header's declared list of what is
deliberately not covered — the receipt half, whose re-derivation is now `--selftest-mutations` (§1.1)
and the suite's corpus cases, exactly as the manifest states. The required `citations` step verifies
every row against a running Go test: `docs/evidence/0009a-citations.tsv: 9 row(s) verified` (§7).

## 7. The gate

```
$ python3 scripts/gate.py
gate: <repository>
gate: go=<toolchain> targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 469 tests in 11.085s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5739 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009a-citations.tsv: 9 row(s) verified
  PASS     secrets  no leaks found in 208 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
$ echo $?
0
```

The host toolchain path in the gate's own second line is elided (the gate prints it); the repository
path is elided the same way in §1–§3 where a scratch path appeared. The two lines the gate itself
truncates (parity, secrets) are quoted as the gate printed them.

## 8. What this record does not claim

- The native verdicts are this host's, one run each; the D3 statement is that the acceptance path's
  verdict is unchanged, not that the path was exercised on any other machine.
- No real turn was run here (`--resume-prompt`/`--repeat-resume-prompt`), so "the acceptance path is
  unchanged" is the offline-and-load half of that check, which is the half this increment is about.
- The reader-visible projection — the records a read of an artifact drops, rewrites, dedupes,
  suppresses or expands — is **not** certified by this oracle; it is the manifest receipt's own
  statement, whose producer is 0009c/0009d (the goal's Non-scope), and §5's residuals name every
  class-1 finding that waits on it.
- The freeze's enforcement is the conductor's oracle-lock signal, not a command in this tree;
  `docs/harness.md`'s oracle paragraphs still describe the pre-repair surface (§4's disclosure).
- The oracle's rules say nothing about whether an artifact's dialogue is a faithful conversion of any
  source — they are well-formedness and receipt-accounting rules over the artifact's own bytes, and
  the round-trip and reader-count evidence for conversions lives in the other increments' records.
