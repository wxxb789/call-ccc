# Plan — 0009a — Freeze the native oracle

Contract: [`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md).
Rationale and the full decomposition: [`docs/plans/0009-decomposition.md`](0009-decomposition.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).

Written before implementation. The umbrella goal file [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md)
still carries the increment's `Done when` clauses D1–D7 and the seam's carried-disposition set (D6); this
increment proves the subset named in its own contract, and nothing here re-opens a settled decision (the
reference route, the refusal surface, the ID-less deferral).

## Order and dependency

This is increment **a of 4** of the goal-0009 decomposition. It is dispatched only after its
prerequisite is accepted; the four are strictly ordered because each one removes a class of defect from every
diff that follows it.

## What this increment must not touch

Any Go change; the manifest receipt's *producer* (0009c/0009d); the other four target routes.

## Design (public shape, edge cases, trade-offs)

**Where it lives.** `scripts/codex-native-check.py` gains one section — the frozen oracle — and one mode of
`main` (`--validate`), and nothing else in that file moves. The native path (the checks the unflagged Codex
acceptance runs) is untouched, so its verdict is the regression clause's own evidence, taken before and after.

**Public shape.**

- `ORACLE_RULES: dict[str, str]` — rule id → the sentence the rule states. The ids are the report's own
  vocabulary and the test's mutation-table keys.
- `class Violation(NamedTuple): rule: str; line: int; detail: str` — `line` is the artifact record's own
  1-based line, `0` for a whole-artifact or receipt-side violation.
- `validate_artifact(text: str, receipt: dict | None = None, name: str = "artifact") -> list[Violation]` —
  pure: text and a parsed receipt in, violations out, no filesystem and no process state.
- `--validate <artifact> [--receipt <document>] [--json]` — the CLI half: prints `oracle: PASS` /
  `oracle: FAIL — <n> violation(s)` / `oracle: INCOMPLETE — <reason>`, exit 0/1/2.

**Rules (13), and why each is sound.** Each is measured on Codex `0.170.0`'s own 25 host sessions *and* on
artifacts this build's writer produces (the record-type set, first-record-is-session_meta, exactly one
session_meta, non-empty `id`/`originator`/`cli_version`, non-decreasing envelope instants, every message's
blocks typed as the carried reader's own text channel, every `user_message`/`agent_message` event carrying a
non-empty string, no duplicate call id and no output naming an absent call). Structural rules are stated over
the artifact alone; three are stated over the artifact **and** an explicit receipt:

| id | states |
| --- | --- |
| `oracle.jsonl-objects` | every non-empty line is exactly one JSON object |
| `oracle.envelope` | every record holds a parseable `timestamp`, a non-empty `type`, an object `payload` |
| `oracle.record-type` | `type` is one the format defines (the measured set; **payload** types stay open) |
| `oracle.session-meta` | the artifact's first record is its only `session_meta` |
| `oracle.session-meta-members` | its payload's `id`, `originator`, `cli_version` are non-empty strings |
| `oracle.clock` | envelope instants never decrease |
| `oracle.message` | a `response_item` message: non-empty `role`; a non-empty `content` whose blocks are typed `input_text`/`output_text`/`text` with string `text`, at least one non-blank |
| `oracle.event-message` | a `user_message`/`agent_message` event carries a non-empty string `message` |
| `oracle.call-pairing` | call ids are non-empty and unique; every call output names a call the artifact carries |
| `oracle.receipt` | the receipt is an object whose `occurrences` are well-formed entries (`kind`, the identity its kind needs, `disposition` ∈ `emitted`/`omitted`) |
| `oracle.plan-emitted` | every occurrence the receipt marks emitted is present in the artifact, as many times as it is marked |
| `oracle.plan-accounted` | the artifact holds nothing the receipt does not mark emitted — an occurrence the receipt marks omitted, and an occurrence it names nowhere, are equally refused |
| `oracle.plan-pairs` | a call and its recorded result carry the same disposition |

**Edge-case semantics the contract fixes.**

- *An unanswered call is legal; an unclaimed output is not.* The writer retires a call whose recorded result
  it does not write (`TestCodexRolloutRetiresACallWhenItsResultIsOmitted`), so the artifact alone cannot
  prove a pair; the pair obligation is the receipt's (`oracle.plan-pairs`), stated over *occurrences*, not
  over the records that contain them.
- *Multiplicity is identity-counted.* Two identical prompts are two occurrences; the plan's marks are counted,
  so `emitted` and `omitted` entries of one identity never silently cancel.
- *A receipt that declares nothing cannot pass* — it accounts for no occurrence, so `oracle.plan-accounted`
  refuses the artifact (the same fail-closed rule the citations manifest carries).
- *Unknown payload types are accepted* (Codex's own event and item vocabulary is open and measured);
  unknown **envelope** types are refused, because this build writes five and reads a measured eight.
- The oracle judges this route's occurrence vocabulary — a message (the writer's own two roles, user and
  assistant), a tool call, a tool result and a compaction boundary; other records are not occurrences of it
  rather than silently accepted as ones.

**Fixtures.** `scripts/tests/testdata/codex-oracle/` holds the hand-constructed synthetic Codex JSONL and its
receipts: `session.jsonl` (a two-turn session with a call, its result and a compaction boundary, plus one
`world_state` and one `token_usage_record` so a real-only envelope type is exercised) with
`session-receipt.json`; `reduced.jsonl` — the same session's own records with the second turn taken out —
with `reduced-receipt.json`, which is D2's fixture: a reduced artifact validated against its plan, the source
absent from the process. Negative fixtures are the shipped rule table's own mutations, held in the test as
one mutation (or more) per rule, each asserted to turn *that rule* red and no other.

**Freeze and lock.** D4's statement lives in four places a later lane reads: the oracle section's own header
comment (read-only for the rest of goal 0009; the reason is the oracle lock, a round that moves this checker
alongside production code), `docs/harness.md`'s native-check entry, `AGENTS.md`'s binding rules, and this
increment's evidence record.

**Trade-offs taken.**

- *No rule demands that every call is answered*: measurement shows the writer retires such calls by design, and
  a rule the tree's own behaviour would violate is a false oracle, not a strict one.
- *The record-type set is closed*: a Codex version that adds one makes the oracle refuse a legitimate file —
  which is visible, cheap to repair after goal 0009, and strictly better than an oracle whose rules nobody can
  break.
- *The receipt's schema is frozen here, minimal*: per-occurrence kind + identity + disposition, no source
  semantics, no per-record fates (0009c's triple carries those; the oracle only compares plan and artifact).
  This is the increment's most speculative surface and is deliberately the smallest one that can catch "the
  wrong result was omitted".
- *The oracle is not wired into the native path in this increment*: D3 makes the acceptance path's verdict a
  regression clause, and 0009d consumes the oracle from its own lowering checks.

**Out of scope, with reasons.** Any Go change (Bounds); the receipt's producer (Non-scope); the other routes
(Non-scope); a native (Codex) run as an oracle rule (the oracle is the offline half; the native half stays the
existing check's); the writer's own adoption of the rules (0009d).

## Evidence

`docs/evidence/0009a-citations.tsv` — discrete decisions plus the Codex native-load check. The manifest carries **discrete decisions plus this increment's native-load check** — not a
∀-shaped claim proved by example (owner Q1 = C, 2026-11-20).

The record (`docs/evidence/0009a-native-oracle.md`) carries, with verbatim output and exit codes: the
measurements the rules rest on (§0); the oracle's positive fixtures and the mutation run over all 13 rules
(§1–§2); D2's reduced-artifact validation with the source absent (§3); the unflagged Codex acceptance run
taken **before and after** this increment (§4); and `--validate` runs over a real converted artifact (§5).

**The manifest's own rows — an ambiguity, decided.** `docs/conventions.md` holds a citations row to be one
gate-runnable **Go** test, while this increment makes no Go change at all; the goal's acceptance surface names
the file anyway. Options were: (a) ship no file and record why — the conventions' own rule for a goal with no
Go-test-dischargeable obligation; (b) ship rows citing the **existing** Go tests that pin each structural
rule's ground in the tree's own behaviour; (c) add a Go test — refused, it is a Go change. (b) is taken: a row
names one rule and the test that already pins the behaviour the rule restates (e.g. `oracle.clock` —
`TestCodexRolloutStampsRecordsInOneOrderedRun`), so the row is re-derived by the gate, and the manifest's
header states the exact claim a row makes, names the python halves (`selftest` cases) and the receipt rules
(no Go ground exists: 0009c/0009d own them) as deliberately not covered. Nothing in a row claims a Go test runs
the oracle.

## Stage order and the convergence rule

`implement → gate → simplify → apply → gate → review → fix → gate → commit → push → PR → CI → machine review
→ merge`. The conductor blocks on a convergence signal (executable-surface churn, a rising P0/P1 count
conjoined with executable churn, or the oracle lock — a round that moves a gate/checker script alongside
production code) and on the absolute review budget; a trip is an **abstraction-boundary failure to report**,
never a budget to raise.

## Where the implementation departed from this plan

One departure from the design above: the shape line's
`validate_artifact(text: str, receipt: dict | None = None, name: str = "artifact")` shipped as
`validate_artifact(text: str, receipt: object | None = None) -> list[Violation]` — `name` is not there,
and `receipt` is typed `object | None`, deliberately broader than a parsed mapping so a receipt that
cannot be read is *reported* (`oracle.receipt`, `oracle_report`'s INCOMPLETE arm) rather than refused by
the signature. The oracle is otherwise the section and the mode as written, the native path is
untouched, and the fixtures are the committed synthetic files. Three details settled while measuring, in
the open because each one is a rule that was deliberately **not** stated (the increment's whole point is
stating what holds, not what would be nice to hold):

- *No rule requires an output's text to be non-blank or a string, and none requires a record to stand
  inside a turn.* Codex 0.170.0's own 25 host sessions hold twelve blank call outputs, one list-valued
  `custom_tool_call_output.output`, and 73 `response_item` messages before the first `turn_context`
  (57 developer, 27 user, 1 assistant); a rule written from this writer's habits would refuse the
  format's own files (evidence record §0).
- *No rule requires every call to be answered.* The writer retires a call whose recorded result it does
  not write (`TestCodexRolloutRetiresACallWhenItsResultIsOmitted`), so the artifact alone cannot tell "the
  source recorded no result" from "the result was left out": that is the receipt's own statement, and
  `oracle.plan-pairs` is the rule that refuses a split plan. `oracle.plan-pairs` therefore covers both
  directions of a split in one sentence, while its mutation pins the call-emitted/result-omitted arm —
  the only arm an artifact can hold without also breaking `oracle.call-pairing` (an output with no call).
- *The predecessor's claim, not its shape.* The decomposition names PR #16's oracle work as this
  increment's starting point; what that head held in `scripts/codex-native-check.py` was the **native**
  selection arm (a `--select` conversion whose target view is asserted to be exactly the planned window).
  Its *claim* — the artifact holds exactly the planned occurrences, both halves of a pair among them, and
  nothing the plan left out — is what this increment states **offline** over an explicit receipt, because
  A2-1 may not touch Go and the `--select` CLI only arrives with 0009d. The native arm itself stays
  PR #16's, to be rebuilt in 0009d against this frozen oracle.
