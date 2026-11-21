# Plan — 0009a — Freeze the native oracle

Contract: [`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md).
Rationale and the full decomposition: [`docs/plans/0009-decomposition.md`](0009-decomposition.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).

Written before implementation. The umbrella goal file [`docs/goals/0009-selective-context.md`](../goals/0009-selective-context.md)
still carries the increment's `Done when` clauses D1–D7 and the seam's carried-disposition set (D6); this
increment proves the subset named in its own contract, and nothing here re-opens a settled decision (the
reference route, the refusal surface, the ID-less deferral).

**The mechanism repair superseded the Design's receipt half (this increment's own fix, after review
rounds r1–r12).** The twelve review rounds found 4–6 defects every round on one surface — the receipt
half that mirrored what an inherited *read* of an artifact answers (a Python shadow of
`internal/parser/codex.go`) — and never closed. The repair re-decomposed the increment: **D1 is now a
finite, executed registry** (`ORACLE_RULES` + `--selftest-mutations`) and the receipt half is **scoped to
the artifact's own structure** — the reader-modelling rules are removed, not patched, because the
artifact's bytes do not state what a read answers, and that projection is the manifest receipt's own
statement (0009c/0009d, the Non-scope). The edge-case bullets below are the **pre-repair revision**:
each one that states a refusal over a read's answers is marked `[pre-repair]`, and its `§`-references
name the pre-repair record's sections. The shipped design is the shipped checker's own `ORACLE_RULES`
table and section header, `docs/harness.md`, and the evidence record's §1 (what shipped) and §5 (where
each of the twelve rounds' findings landed). The Evidence section below names the record's shipped
sections.

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

**Rules (14), and why each is sound.** Each is measured on Codex `0.170.0`'s own 25 host sessions *and* on
artifacts this build's writer produces (the record-type set, first-record-is-session_meta, exactly one
session_meta, non-empty `id`/`originator`/`cli_version`, non-decreasing envelope instants, every message's
blocks typed as the carried reader's own text channel, every `user_message`/`agent_message` event carrying
a non-empty string, no duplicate call id and no output naming an absent call). Structural rules are stated
over the artifact alone; four are stated over the artifact **and** an explicit receipt — and every row's
shipped wording is the `ORACLE_RULES` sentence in the shipped checker, the `oracle.plan-accounted` row
below having been replaced in place by the mechanism repair (the superseding note above):

| id | states |
| --- | --- |
| `oracle.jsonl-objects` | every non-empty line is exactly one JSON object, every member name written once inside it and no string it holds carrying an unpaired surrogate escape — a repeated name, and a `\ud800`-style escape (this oracle's decoder answers the code point, the carried reader's `gjson` U+FFFD, measured), are lines this oracle's decoder and the carried reader's `gjson` answer differently for, refused before comparison |
| `oracle.envelope` | every record holds a parseable `timestamp` — one whole, ASCII RFC3339 instant — a non-empty `type`, an object `payload` |
| `oracle.record-type` | `type` is one the format defines (the measured set; **payload** types stay open) |
| `oracle.session-meta` | the artifact's first record is its only `session_meta` |
| `oracle.session-meta-members` | its payload's `id`, `originator`, `cli_version` are non-empty strings |
| `oracle.clock` | envelope instants never decrease |
| `oracle.message` | a `response_item` message: non-empty `role`; a non-empty `content` whose blocks are typed `input_text`/`output_text`/`text` with string `text`, at least one non-blank |
| `oracle.event-message` | a `user_message`/`agent_message` event carries a non-empty string `message` |
| `oracle.call-pairing` | call ids are non-empty and unique and a call carries the non-empty `name` the carried reader registers — a record without one is no call, so an output naming it attaches to nothing; every call output names a call the artifact carries **before it** |
| `oracle.receipt` | the receipt is an object whose `occurrences` are well-formed entries (`kind`, the identity its kind needs — a result's is its call id **and its recorded text**, a call's its call id, name and recorded input, the receipt's `arguments` member naming whichever of the artifact's `arguments`/`input` members wrote it, read exactly as written — `disposition` ∈ `emitted`/`omitted`); a receipt that declares no occurrences is refused rather than compared |
| `oracle.plan-emitted` | every occurrence the receipt marks emitted is present in the artifact, as many times as it is marked — a dialogue message in both of the artifact's channels (the model-visible `response_item` and the turn-view `event_msg`) |
| `oracle.plan-accounted` | the artifact holds nothing the receipt does not mark emitted — an occurrence the receipt marks omitted, an occurrence it names nowhere, a compaction boundary's `replacement_history` (an empty list holds nothing), and the shapes whose receipt-facing identity the artifact's own bytes cannot state — a call whose recorded input is written as two members (`arguments` and `input`) or as a member that is not a string, a result whose recorded text is written as a member that is not a string, and a `session_meta` carrying `forked_from_id` — each refused where a comparison runs and silent without one, in both of the target's dialogue channels |
| `oracle.plan-order` | the emitted occurrences stand in the artifact in the receipt's own order, and so do its dialogue messages in the artifact's turn view — a compaction boundary moved past later turns compacts a different history with every count unchanged; the receipt names no turn, so which `turn_context` record a record stands after is outside what the receipt can state and is not compared |
| `oracle.plan-pairs` | a call and its recorded result carry the same disposition |

**Edge-case semantics the contract fixes.**

**Pre-repair revision.** The bullets marked `[pre-repair]` below state the reader-modelling refusals the
mechanism repair removed (the shapes a read drops, rewrites, dedupes, suppresses or expands); their
`§`-references name the pre-repair record's sections, and §5 of the shipped record routes each finding to
a surviving structural rule or to the manifest receipt's own statement (0009c/0009d, the Non-scope).

- *An unanswered call is legal; an unclaimed output is not.* The writer retires a call whose recorded result
  it does not write (`TestCodexRolloutRetiresACallWhenItsResultIsOmitted`), so the artifact alone cannot
  prove a pair; the pair obligation is the receipt's (`oracle.plan-pairs`), stated over *occurrences*, not
  over the records that contain them.
- *Multiplicity is identity-counted.* Two identical prompts are two occurrences; the plan's marks are counted,
  so `emitted` and `omitted` entries of one identity never silently cancel.
- *A receipt that declares nothing cannot pass* — an empty `occurrences` list is refused as a receipt
  **before** any comparison (both accounting rules would otherwise pass vacuously on a metadata-only
  artifact), the same fail-closed rule the citations manifest carries.
- *An output written before its call is refused* — the carried reader attaches such an event to nothing
  (`internal/parser/codex_collecting_sink.go`), and this route's writer never emits one there, so
  ownership is checked in record order rather than against the artifact's whole set of ids.
- *A boundary's `replacement_history` is refused where a plan is compared* — the native `compacted`
  record can hold the messages it replaced there (`internal/parser/codex_fidelity_test.go`), the
  receipt's boundary identity is its summary alone, and this route's writer emits only the summary.
- *An occurrence the receipt cannot name is refused where a plan is compared* — the carried reader turns
  more shapes into occurrences than the receipt's vocabulary names: an inter-agent `agent_message`
  addressed to the session's own agent path is a user message (`handleAgentMessage`), a payload type the
  reader's switch does not take is a message when it carries a `user`/`assistant` role and readable text
  (`handleResponseItem`'s default branch), and an `image_generation_call` is a tool call whose identity
  (`id` and raw payload text) the receipt's `call_id`/`name`/`arguments` identity has no members for — so
  each is refused rather than certified (review round r4's finding wrote the reduced plan's omitted prompt
  into the first shape and receipt validation passed it).
- *The identity a receipt is matched on is the member the artifact writes* — a call's recorded input is
  its `arguments` (then `input`) member exactly as written, not the carried read's projection of it:
  the writer's own `arguments: "{}"` (`internal/ccc/codex_target.go`'s `lossEmptyInputs` spelling) and a
  missing member are two occurrences, because a receipt that could not tell them apart certified an
  artifact whose written input it never named (review round r4's finding; the read's
  `extractCodexInputJSON` skips `{}`/`[]` and trims, which stays its own answer for dialogue, not the
  plan's identity).
- **[pre-repair]** *A shape whose read this oracle does not model is refused where a plan is compared, not
  read as empty or as its written member* — the reader-modelling half of this bullet is the repair's
  removed side, and the object-valued `content` it names is the structural `oracle.message` rule's now
  (the rule requires a non-empty list of text blocks, so the shape is refused in both modes, silent about
  the read the bullet mirrored). The both-members half survives and is stated in the row above: a call
  carrying both an `arguments` and an `input` member is refused where a plan is compared, silent without
  one, because a native rollout may carry it (review round r5's finding; §5 routes it).
- *A line whose object writes a member name twice is refused before any comparison* — Python's decoder
  keeps the last value for a repeated name and the carried reader's `gjson` the first, so such a line is
  one two readers answer differently for; the receipt document is read through the same rule, a repeated
  receipt member making the document unreadable rather than compared (review round r5's finding).
- **[pre-repair]** *A `message` in a role the receipt cannot name is refused where a plan is compared* — developer-role
  messages are a native shape Codex's own files carry (57 across the measured 25 sessions), so the
  structural `oracle.message` rule admits any non-empty role; the occurrence vocabulary names the
  writer's two roles, so a `message` carrying readable text in any other role is one the plan cannot
  account for and is refused where a receipt is compared, silent without one (review round r6's finding
  wrote the reduced plan's omitted prompt into a `developer`-role message).
- **[pre-repair]** *A result in the custom tool family is refused where a plan is compared* — `handleFunctionCallOutput`
  answers a `custom_tool_call_output`'s result with an outcome (the record's own `status`, `completed`
  where it holds none) while this route's writer's result record, `function_call_output`, carries no
  outcome member at all (its `tool_result_outcomes` loss states that), so the receipt's
  `call_id`/recorded-text identity cannot state what a read of the artifact answers: refused rather than
  certified, silent without a plan (review round r6's finding, whose mutation keeps the call id and the
  output text).
- *A call without a usable name owns no output* — `handleFunctionCall` reads the payload's `name` first
  and returns on an empty answer, so a record whose `name` is not a non-empty string (its `gjson` `.Str`
  answer empty for a missing or non-string member) is no call of the artifact: refused structurally with
  its orphaned output, and no receipt — the fixture's own or one edited to `name: ""` — certifies the
  pair (review round r6's finding). The gate is the read's empty-string test, not a trimming one: a
  whitespace-only string name is what the read registers.
- **[pre-repair]** *A result the carried read does not establish is refused where a plan is compared* — the receipt's
  result identity is its `call_id` and recorded text, and it is a statement about what a read answers
  only where the read's ordinary arm runs: `handleFunctionCallOutput` appends **no** result for a
  `spawn_agent` output (it links the call to the subagent session) and answers `wait`/`wait_agent`
  outputs from the output's own `status` object (one result per agent entry, none for the fixture's
  plain-text output), while a record with no `output` member or a Go-blank string one appends no result
  at all — the writer already refuses to write those by its own rule (`lossEmptyResults`,
  `lossWhitespaceResults`). One `wait` shape is exempt because the read routes it as an ordinary result:
  the exec-cell poll (`code_mode_wait`, mirrored by `oracle_exec_cell_wait`), and args this oracle cannot
  read exactly fall on the refusing side (review round r7's finding: the fixture's call renamed
  `wait_agent` with its output left as plain text certified a result no read returns). The family is
  asked of the name the read **registers** (`rememberToolCall` Go-trims it), not the written spelling,
  while the written name stays the occurrence identity (review round r10's second finding).
- *A non-string identity member is refused where a plan is compared, not matched by its JSON spelling* —
  the identity serializes a non-string `arguments`/`input`/`output` member back to compact JSON, which
  can reconstruct the receipt's own string key (a space after a colon is lost) while the carried reader
  answers the member's own raw bytes (`extractCodexInputJSON`, `parseCodexFunctionOutput`) and this
  route's writer writes strings (`internal/ccc/codex_target.go`'s `codexFunctionCallPayload`): the
  receipt names the member as written, so a member whose own spelling the decoded value cannot recover is
  refused rather than matched on a spelling neither reader answers (review round r7's finding). The
  `null` member's identity is the string `null` the read answers, distinguished from an absent member's
  empty string.
- *A record's turn is outside the receipt's vocabulary* — the plan states occurrences, identities,
  dispositions and order, and names no turn; the artifact's dialogue records name no turn either. A
  `turn_context` moved across records changes the turn the carried read answers for the records after it
  (`internal/parser/codex.go`) and still validates green, so the rule's own sentence says the placement
  is not compared, and the case pins the scope (review round r7's finding;
  `test_a_moved_turn_context_is_outside_the_receipts_vocabulary` is what holds widening the scope
  visible).
- **[pre-repair]** *A duplicate the read suppresses is refused where a plan is compared* — a result occurrence's identity
  is its `call_id` and recorded text, so two result records under one call id whose recorded texts trim
  to one text are two occurrences to the receipt and one result to every read:
  `CodexCollectingSink.appendToolResultEvent` drops an event whose status and `strings.TrimSpace`
  content equal an event the call already answers (`hasEquivalentCallResultEvent`), and a receipt naming
  both certifies a multiplicity no read of the artifact answers (review round r8's finding). Refused only
  where a plan is compared, because a native rollout may carry a repeated output and the structural
  rules state nothing about how many outputs one call id carries; the case pins both the trim-equal
  duplicate and the byte-identical one, with a second output whose text reads distinctly as the green
  control.
- **[pre-repair]** *A message the read consumes as a subagent notification is refused where a plan is compared* — a
  `user` record whose content parses as a `<subagent_notification>` body naming an agent and a terminal
  status text is consumed by `handleSubagentNotification` (`internal/parser/codex.go`): no read of the
  artifact answers the message the receipt names, and the result event the read attributes the delivered
  text to is stated by no record of the artifact, so the receipt's vocabulary cannot name it. Refused
  only where a plan is compared — the shape is the read's own (its attribution case is
  `internal/parser/codex_parser_test.go`'s) and none of the measured 25 native sessions carries one (the
  pre-repair revision's own probe, deleted with the refusal); the mutation
  `a consumed subagent notification the plan marks emitted` pins the refusal (review round r9's first
  finding).
- **[pre-repair]** *A message the read's own text path removes or rewrites is refused where a plan is compared* — the
  class's other half, which review round r10's finding showed the r9 controls sitting on: a
  notification-prefixed block the read does **not** consume is dropped by the user-text preprocessing
  (`isCodexSystemMessage` reads the tag as a composed system envelope, whether or not its body parses as
  one), a composed envelope prefix is stripped from a block (`stripCodexSystemPrefix`), and an empty
  string is dropped from the join — so the message a receipt names is one no read answers, or one whose
  text is not the record's written bytes. The refusal mirrors the per-record reads with the three new
  mutations; the plugin-discovery envelope is refused rather than read under either answer, because
  which record is the session's first user one is state the oracle does not model, and the reader's
  replay suppression is turn state outside the claim (both boundaries in §17 of the evidence record).
  The mirror's goal-envelope whitespace set is the reader's own Go `\s` — `[\t\n\f\r ]`, ASCII alone —
  so a nonbreaking space before `source="goal"` keeps the text travelling, where Python's `\s` read the
  block as system content (review round r11's third finding).
- **[pre-repair]** *A call's routing name is the name the read remembers, not the written spelling* — `rememberToolCall`
  Go-trims the emitted name and `toolCallNameForOutput` answers from it, so `" wait_agent "` is routed by
  the read as `wait_agent` and its plain-text output appends no result while the receipt's written-name
  identity still matches: the family question is asked of the trimmed name (`oracle_call_routing_name`),
  which review round r10's second finding showed the oracle asking of the raw one.
- **[pre-repair]** *A trim-colliding call id is refused where a plan is compared* — the carried read remembers calls by
  the Go-trimmed id (`rememberToolCall` keys and replaces the pending entry; `forgetToolCall` clears one
  by the written id it is handed), so two calls whose written ids differ only in whitespace are one
  entry to the read; an output under the trimmed spelling is routed by that entry — the last writer's
  name and arm — which this oracle does not model, so it is refused rather than read under either
  answer. The **padded** spelling's own output is answered by the name written beside its own call
  (`callNames`) and stays certifiable, and a single padded id with its receipt is the green control
  (review round r11's first finding; §16 of the evidence record).
- **[pre-repair]** *A repeated output under an `apply_patch` the read expands is refused as a shape the oracle does not
  model* — `handleFunctionCall` expands an `apply_patch` call into one call per file the patch names
  (`codexApplyPatchFiles`), the first output attaching where the pending position points and each later
  one to the last file's call, so a repetition the read trims to one text may be two kept results on two
  calls rather than the one `hasEquivalentCallResultEvent` suppresses. Whether it is kept or suppressed
  depends on the files the patch names, a projection this oracle does not model, so the repetition is
  refused with that statement rather than answered; the padded name, which the read does not expand, is
  the ordinary suppression (review round r11's second finding; §16 of the evidence record).
- *A call whose `call_id` is not a non-empty string is refused, with no output to fail pairing on its
  own* — `handleFunctionCall` passes the raw `call_id` on and `handleFunctionCallOutput` returns on an
  empty one, so the record is no call a read answers; the round-10 mutation writes the empty string,
  deletes the call's output and keeps its non-empty `name` and recorded arguments, because removing the
  guard left every earlier pairing mutation red for its own reasons (review round r10's fourth finding).
- *A line holding an unpaired surrogate escape is refused before any identity is compared* — JSON's
  `\ud800`-style escape decodes to a lone surrogate to this oracle and to U+FFFD to the carried reader's
  `gjson` (measured on the carried v1.20.0, §0 of the evidence record), so two distinct spellings are
  one string to a read: two call ids — the finding's own pair — would pass uniqueness and pairing while
  the read answers one id for both. The line is refused and dropped the way a repeated-member line is;
  the mutation `two call ids distinct to Python and one to the read` pins it, and the case shows the plan
  reader keeping the spellings apart, which is why only the structural refusal can see the collision
  (review round r9's second finding).
- *Every arm a rule states carries its own negative fixture* — the positional arm of
  `oracle.session-meta` ("the artifact opens with its one metadata record") is reached by a mutation that
  moves the metadata record behind another record with each slot keeping the instant it held, so the
  clock stays legal and no other rule can fail; the deleted, duplicated and emptied fixtures all fail
  without it (review round r6's finding). Likewise the envelope's non-empty-`type` arm, the metadata
  members' `originator` arm, the plan-accounted non-string-input and both-members arms, each with its own
  mutation (review round r7's finding), and the `oracle.message` role arm, which review round r8's
  finding named as having no negative fixture: removing its guard left every case green, and the round's
  artifact-only mutation pins it (`a message with no role`). Review round r9's finding named the rule's
  **string-type** arm (`content[i].text` must be a string) as missing its fixture too: a mutation that
  puts a non-string `text` beside a valid non-blank block is invisible to every other arm and mutation,
  and the round's artifact-only mutation `a block whose text is not a string beside a valid one` pins it.
  Review round r10's finding named the `oracle.call-pairing` non-empty-`call_id` guard the same way:
  the earlier pairing mutations stayed red without it (an unknown output owner, duplicate ids, an early
  output, a missing name), and the round's artifact-only mutation `a named call whose call_id is empty`
  — the call's output deleted with it — pins that guard alone.
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
  A result's identity is its call **and its recorded text**, and a call's its call id, tool name and recorded
  arguments, so one output — or one invocation — substituted for another under the same call id is caught; a
  dialogue message is read against both of the artifact's channels and the receipt's own order, in both of
  them. This is the increment's most speculative surface and is deliberately the smallest one
  that can catch "the wrong result was omitted" — the review fix rounds (r1 and r2) are what moved it from
  its first to this shape.
- *The oracle is not wired into the native path in this increment*: D3 makes the acceptance path's verdict a
  regression clause, and 0009d consumes the oracle from its own lowering checks.

**Out of scope, with reasons.** Any Go change (Bounds); the receipt's producer (Non-scope); the other routes
(Non-scope); a native (Codex) run as an oracle rule (the oracle is the offline half; the native half stays the
existing check's); the writer's own adoption of the rules (0009d).

## Evidence

`docs/evidence/0009a-citations.tsv` — discrete decisions plus the Codex native-load check. The manifest carries **discrete decisions plus this increment's native-load check** — not a
∀-shaped claim proved by example (owner Q1 = C, 2026-11-20).

The record (`docs/evidence/0009a-native-oracle.md`) carries, with verbatim output and exit codes: the
measurements the rules rest on (§0); the executed closed equality between `ORACLE_RULES` and its
mutation corpus, beside the writer's own bytes and the corpus (§1, §1.3); D2's reduced-artifact
validation with the source absent (§2); the unflagged Codex acceptance run taken **before and after**
this increment (§3); the freeze's statement (§4); the twelve review rounds' class-1 findings, each
routed to a surviving structural rule or to the manifest receipt's own statement (§5); the citations
(§6) and the gate (§7); and what the record does not claim (§8). The pre-repair revision's replay
recipes and its per-round revert-mutation sections are deleted with the reader-modelling rules they
pinned; the one mutation replay is `--selftest-mutations` (§1.1), which writes only under the scratch
directory it creates and removes.

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
`validate_artifact(text: str, receipt: object = <no receipt sentinel>) -> list[Violation]` — `name` is
not there, and `receipt` is typed `object`, deliberately broader than a parsed mapping so a receipt that
cannot be read is *reported* (`oracle.receipt`, `oracle_report`'s INCOMPLETE arm) rather than refused by
the signature, and its default is the **absence** of a receipt: a supplied document that is not a plan —
a file reading `null` among them — is that same rule's finding rather than read as an absent receipt.
The oracle is otherwise the section and the mode as written, the native path is
untouched, and the fixtures are the committed synthetic files. Three details settled while measuring, in
the open because each one is a rule that was deliberately **not** stated (the increment's whole point is
stating what holds, not what would be nice to hold):

- *No rule requires an output's text to be non-blank or a string, and none requires a record to stand
  inside a turn.* Codex 0.170.0's own 25 host sessions hold twelve blank call outputs, 122 list-valued
  `custom_tool_call_output` outputs, and 73 `response_item` messages before the first `turn_context`
  (45 developer, 27 user, 1 assistant); a rule written from this writer's habits would refuse the
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

**The review fix rounds moved the oracle while the increment was still under review** (the freeze binds the
rest of goal 0009, not this increment's own fix stage), and round 12's review met the abstraction-boundary
failure the superseding note above records: the mechanism repair replaced the shape this paragraph
enumerated — the reader-modelling receipt half — with the finite executed registry and the structural
refusals. The enumeration below is the **pre-repair** revision (its `§`-references name the pre-repair
record's sections; §5 of the shipped record routes each finding); the shipped shape is `ORACLE_RULES` and
its section header: a result's identity
includes its recorded text and a call's its name and recorded input (a same-id substitution of either is
refused, and the recorded input is the member the artifact **writes** — `arguments` then `input`, read
exactly as written, so the writer's own `"{}"` is not the carried read's empty answer — while the read's
own projection stays its answer for dialogue; and a call carrying **both** members is refused where a plan
is compared, review round r5's finding), every `response_item` a read turns into an occurrence the
receipt's vocabulary cannot name — the inter-agent `agent_message` addressed to the session's own agent
path, another payload type carrying a `user`/`assistant` role and readable text, an
`image_generation_call`, a `message` in a role outside the writer's two carrying readable text, and a
`custom_tool_call_output` whose read answers an outcome this route's writer's own results have no member
for — is refused where a plan is compared, an object-valued `content` a read iterates
the member values of is refused the same way (review round r5's finding), a call whose recorded input or
result whose recorded text is written as a non-string member is refused there too — its JSON spelling can
reconstruct the receipt's string key while the carried reader answers the member's raw bytes — and a
result the carried read does not establish (a call the read answers elsewhere — `spawn_agent`,
`wait`/`wait_agent`, the exec-cell poll excepted — or no output a read appends a result from) is refused
the same way (both review round r7's finding), or one whose recorded text is the text of the result
written before it after the read's own trim — `hasEquivalentCallResultEvent` suppresses the duplicate,
so a receipt naming both certifies a multiplicity no read answers (review round r8's finding), a `user`
message a read consumes as a subagent notification is refused the same way — no read answers the message
the receipt names, and the result the read attributes its text to is stated by no record (review round
r9's finding) — a line whose object writes a
member name twice — and a receipt whose object does — is refused before comparison because this oracle's
decoder and the carried reader's `gjson` answer a repeated name differently (review round r5's finding),
a line holding an unpaired surrogate escape — the finding's two call ids distinct to Python and one to
the read among them — is refused and dropped the same way (review round r9's finding),
a call whose `name` is not the non-empty string the carried reader registers is refused with its orphaned
output, and no receipt naming the empty name certifies the pair (review round r6's finding),
a dialogue message is read against both of
the artifact's channels
(`oracle.plan-emitted`/`oracle.plan-accounted`), `oracle.plan-order` refuses a moved
compaction boundary whose counts are unchanged and a turn-view dialogue order that is not the receipt's,
every `compacted` record is a boundary occurrence (an empty summary included) and a boundary carrying a
native `replacement_history` is refused where a plan is compared, an output written before its call is
refused (call ownership is checked in record order), a receipt that declares no occurrences is refused
before any comparison, the oracle's blank test is
the writer's own (`strings.TrimSpace`, not Python's wider `str.strip`), `oracle.envelope` refuses the leap
second `60` and any stamp that is not one whole ASCII RFC3339 instant — a trailing line feed and a
non-ASCII decimal digit among them — instead of remapping or half-matching it, `oracle.jsonl-objects`
reads the artifact's own bytes (LF split, no
non-JSON constants, no replacement-decoded text), an unreadable-by-nesting receipt is `INCOMPLETE` rather
than a traceback, a decoded string strict UTF-8 cannot encode is named by a digest that never raises
where the diagnostic needs one, default diagnostics name occurrences and refused values by type and
by digest without echoing their text, and a record's turn placement is stated as outside the receipt's
vocabulary rather than half-checked (review round r7's finding). The record's shipped sections are §1–§8:
§1.1 carries the one mutation replay (`--selftest-mutations`, executed over the corpus and asserted from
both ends) and §5 routes every one of the twelve rounds' findings, so the per-round revert-mutation
sections this paragraph used to cite are deleted with the recipes and rules they quoted.
