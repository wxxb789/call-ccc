# Plan — 0003 Continue a Codex session in Claude Code

Contract: [`docs/goals/archive/0003-codex-to-claude-continuation.md`](../goals/archive/0003-codex-to-claude-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0002 ([`docs/plans/0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md)).

Written before implementation. Every target-side fact below was **measured on this host against the
installed Claude Code CLI (`2.1.326 (Claude Code)`, npm `@anthropic-ai/claude-code@2.1.326`, installed
into an isolated prefix at `~/.local/share/ccc-native/claude-cli`)** before any writer code existed.
Every measurement is repeated in `docs/evidence/0003-codex-to-claude-continuation.md`.

## 1. What this increment adds

1. A **target writer** for Claude Code under `internal/ccc`: a `Conversation` becomes a Claude Code
   transcript JSONL file (`claude_target.go`), plus the fidelity report of what could not be carried.
2. An **installer** (`claude_install.go`): the transcript takes a fresh native identity, is published
   into the target's own `<config>/projects/<project-dir>/<session-id>.jsonl` exclusively, and is read
   back through the **inherited AgentsView Claude reader** for the structural comparison.
3. **Native verification** through Claude Code's own loader (`claude_verify.go`): an offline `load`
   level that drives the installed binary with `--bare` and with the environment variables that could
   reach a model removed, so the loader resolves the installed session and reports it with **no model
   turn**, plus a control arm for a session id nothing installed; and a `resume` level that runs one real
   turn through the reported invocation and records the harness's own answer and the session file's
   growth.
4. A **second target route in the same pipeline**: `Continue` dispatches on the target, so the 0002
   Codex route and the new Claude route share the reader, the report shape, the source-file check and
   the failure handling rather than forking them.
5. A CLI target: `ccc continue codex:<session-id-or-path> --to claude [--json] [--dry-run]
   [--target-home DIR] [--target-binary PATH] [--verify none|load|resume] [--resume-prompt TEXT]`.
6. A repeatable native-verification harness, `scripts/claude-native-check.py`, deliberately **not** a
   gate step (it needs Claude Code installed and, for a real turn, model access), plus the offline Go
   tests that hold the writer, the installer and the pipeline down.

## 2. The route: a Claude Code transcript, verified by Claude Code's own loader

### 2.1 Why a written transcript and not an importer

| Observation (measured, `claude 2.1.326`) | Result |
| --- | --- |
| `claude import codex --dry-run` (with a Codex home present) | ``claude import`` is not yet available in this build. Run `claude` and use /mcp or edit ~/.claude/settings.json directly. — there is **no** native import route to prefer |
| session files on disk | `<config dir>/projects/<project-dir>/<session-id>.jsonl`, one JSON object per line, `<session-id>` the file stem |
| `CLAUDE_CONFIG_DIR` | relocates the whole config directory; two homes never see each other's sessions |
| resume discovery | `claude -p --resume <id>` finds a session **through the process's own working directory**: run from any directory whose project-dir spelling differs, it answers `No conversation found with session ID: <id>` and exits 1 (measured from a sibling directory and with the file placed under a different project dir) |
| project-dir spelling | each character outside `[A-Za-z0-9]` becomes `-`: `/tmp/probe3/WS.Upper Case_1+plus` → `-tmp-probe3-WS-Upper-Case-1-plus` (uppercase preserved) |
| the loader's own view of a session | resuming sends the loader's reconstructed messages to the model endpoint; the request is recorded by a loopback proxy, so what the loader read is directly observable |
| a hand-written transcript | loads and continues: a file holding only `user`/`assistant` records (`uuid`, `parentUuid`, `cwd`, `sessionId`, `version`, `timestamp`, `message`) resumed a turn that answered from the converted history (`PLUMBBOB`) |
| envelope extras (`userType`, `entrypoint`, `gitBranch`, `version`, `permissionMode`, `promptId`, `toolUseResult`, `sourceToolAssistantUUID`, `usage`, `model`) | none is required to load or to resume (measured by removing each) |
| records with `isSidechain: true` | **dropped** from the model-visible history (a subagent's token never reached the model) |
| `{"type":"system","subtype":"compact_boundary"}` + `{"type":"user","isCompactSummary":true, …}` | truncates the model-visible history to the boundary: the loader's request began with the summary record and the pre-boundary fact was gone (`NOT-IN-CONTEXT`); the `isCompactSummary` flag **alone**, without the boundary record, changed nothing |
| `tool_result` blocks with `is_error: true` | passed through to the model verbatim (the error text answered a question) |
| a resume turn | **appends** the harness's own records to the session file (measured 1 005 → 135 309 bytes for one turn: `api-request`, `api-request-blob`, `cost-state`, `last-prompt`, …), none of which the writer fabricates |

Consequences taken as this increment's defaults:

- **Write the transcript, not a store.** Claude Code's own session file is the format its loader reads,
  the format the inherited AgentsView Claude reader parses (so both sides of the evidence come from
  independent implementations), and the only format a resume picks up.
- **One record per source message**, with its content blocks (`message.content` as a block array), and
  a fresh unique `message.id` per assistant record: the inherited reader merges *consecutive assistant
  records that share a `message.id`* as streaming chunks of one response, so sharing an id would merge
  two messages. Measured: a single record with several blocks reconstructs to exactly the same request
  as the block-per-record shape the harness itself writes.
- **The session's identity is the file name**: the new UUID v4 is both the file stem and every record's
  `sessionId` (the inherited reader treats a record whose `sessionId` differs from the stem as a
  *child* session).
- **The project directory is derived from the source session's recorded working directory**, because
  the loader finds a session only there. A source read that recorded no working directory is refused
  before anything is written (`ErrNoTargetCwd`), instead of inventing a directory that would make the
  session undiscoverable while the report called it installed.
- **The printed resume command carries both the data directory and the working directory**
  (`cd <cwd> && CLAUDE_CONFIG_DIR=<home> claude --resume <id>`): without the first, the harness looks in
  the caller's own home; without the second, its project-directory lookup cannot see the session.
- **No `message.model` is written and no provider is recorded.** Claude Code's transcript records no
  provider, so a resumed session is bound to nothing; the source's per-message model is *not* written
  into the target's model field, because that field states which model produced the words and the
  conversation was not produced by one of the caller's target models. Both are reported (a loss
  category and a format limit).
- **A compaction boundary is written as the pair the harness itself writes** (the `system`
  `compact_boundary` marker plus the `isCompactSummary` summary record), so the target applies its own
  compaction semantics to its model-visible history while the file keeps the pre-boundary turns.

### 2.2 The exact records written

One JSONL file, one object per line, in this order, each timestamp the source's own instant written as
RFC3339 in UTC by the shared `codexClock` (`time.RFC3339Nano`, so a `.000` fraction is dropped: a
measured line carries `"timestamp":"2026-05-04T09:15:02Z"`):

| Source evidence | Claude Code record |
| --- | --- |
| each user message with dialogue | `{"type":"user","uuid":<v4>,"parentUuid":<previous record's uuid or null>,"sessionId":<new v4>,"cwd":<source cwd>,"version":<probed or pinned>,"isSidechain":false,"timestamp":<source time or session start>,"message":{"role":"user","content":<text>}}` |
| each assistant message with dialogue | `{"type":"assistant",…,"message":{"id":"msg_<fresh>","type":"message","role":"assistant","content":[{"type":"text","text":…}]}}` — no `stop_reason`, `stop_sequence` or `usage`: the writer records nothing it would have to invent, because the harness's own loader does not need them |
| tool calls on a message | one assistant record **per call**, holding a single `{"type":"tool_use","id":<recorded tool-use id or a generated one>,"name":<source tool name>,"input":<recorded input JSON, compacted by the writer's own encoder>}` block, sharing the message's `message.id` — the shape the harness writes for one block of one message; both readers put the records back into one message |
| tool results | one `user` record whose `content` is `[{"type":"tool_result","tool_use_id":…,"content":<decoded output text>[,"is_error":true]}]`, the results of one assistant message travelling together |
| compaction boundary | `{"type":"system","subtype":"compact_boundary",…}` followed by `{"type":"user","isCompactSummary":true,…,"message":{"role":"user","content":<summary text>}}` |

Text is a plain JSON string for a message with exactly one text block and an array for anything else;
both are the harness's own shapes and both were measured to survive resume. `parentUuid` chains every
record to the one before it, so the file is one DAG path (the inherited reader needs a single root and
resolvable parents to take its DAG path, and degrades to its linear path otherwise).

### 2.3 Mapping rules, category by category

| Category | Rule | Why |
| --- | --- | --- |
| Dialogue text | `Message.Dialogue` when the reader supplied a dialogue projection, else `Message.Content`; **for a message that carries tool calls, only the dialogue projection is written as text** | the reader's `Content` renders calls into text (`[Read: <path>]`), so writing both would duplicate the call in the target's history |
| Dialogue text that holds only whitespace | **omitted**, counted (`whitespace_only_dialogue`) | the inherited Claude reader drops a record whose text is empty after preprocessing and that carries no tool result, so a written record would be in the file and absent from every read of it |
| Tool calls | `tool_use` block with the source's own name, verbatim `input` object and the recorded tool-use id | it is the recorded evidence; inventing a target tool vocabulary would forge a call the source agent never made |
| Tool results | `tool_result` block under the call's `tool_use_id`, with `parser.DecodeContent(raw)` text | the archive/source store a content field that is a JSON string when the provider encoded it; the decoded text is what the tool produced |
| A result whose output holds only whitespace | **omitted**, counted (`whitespace_only_tool_results`) | the inherited reader's block parser reads text that is only whitespace as no text, so a written record would be in the file and absent from every read of it |
| A result the source recorded with a status | `is_error: true` **only** where the source's own status says the call failed; any other status is carried as text and counted (`tool_result_statuses`) | `tool_result.is_error` is the target's own field and was measured to survive resume, so a failure the source recorded is representable; a status with no error meaning has no target field and is named as a narrowing |
| Results folded onto a call (archive reads) | the call's recorded result events become its `tool_result`; `ResultContent`/`ResultContentSize` become one only for a call with no recorded result of its own | as in 0002: beside the recorded events the summary is a view of them, and without them it is the only evidence the read holds |
| Tool results with no matching call | **omitted**, counted | a `tool_result` without its call is not a pair; inventing a call would fabricate a tool invocation |
| Compaction boundary | the `system` marker + the `isCompactSummary` summary record | measured: that pair is what makes the target truncate its model-visible history at the boundary, which is the target's own semantic for the source's `compacted` record |
| Messages before a compaction boundary | **carried**, counted (`pre_compaction_history`, caveat) | the transcript records them and the identity transform preserves available history; the boundary record is what stops them being presented as active context |
| `Thinking` | **omitted**, counted (`reasoning_text`) | the target's thinking blocks are the target's own provider-signed artifacts; writing plaintext in their place would present source-private reasoning as a target-native trace (goal 0013 owns this) |
| `IsSidechain` messages | **omitted**, counted (`sidechain_messages`) | measured: the loader drops `isSidechain` records from the model-visible history, so carrying them would put evidence in the file that no read of it — including the conversion's own round-trip check — could show. They are a subagent's turns in the same transcript, and the target has no subagent attribution for a flat message |
| `IsSystem` records that are not compaction boundaries | the record's own text **omitted**, counted (`system_records`); its tool evidence **carried** | the target's system channel is its own; a promoted system reminder's tool result is still the recorded outcome of a call this conversion carried |
| Messages recorded with the tool role | text carried only as its paired `tool_result` block, counted (`tool_role_messages`) | the tool's words belong in the result block, never as a user or assistant message |
| A message whose model the source recorded | **omitted**, counted (`message_model_attribution`) | the target's `message.model` states which model produced the words; the source's model did not run this session, and writing it would attribute the conversation to it |
| Lines the source reader could not parse, a summary-fallback read, a read the provider says needs a retry | **reported from the read's own fidelity** (`malformed_source_lines`, `summary_transcript`, `source_needs_retry`) | a converted report that omitted them would stand for a conversation the source file holds more of |
| Attachments/images | described as a **format limit** | the source read carries no attachment bytes, so a per-conversion count of "0 omitted" would be a claim the read cannot support |

Every category is listed in the output on every run with its count (including zero) and its
disposition, exactly as the Codex writer does.

**Format limits** (properties of the format pair, not of one run): the target transcript has no
session-metadata record (so the source's `session_meta` per-session fields — originator, cli version,
source kind — have no target record); it has no turn-boundary record (Codex's `turn_context`); it has no
provider field; its model-visible history is bounded by compaction records it owns; attachments carry
no bytes through this read; tool names are the source's vocabulary, not the target's tool set.

## 3. Installation

- Target home resolution: `--target-home DIR`, else `$CLAUDE_CONFIG_DIR`, else `~/.claude`. A home that
  does not exist is an error unless `--target-home` was given explicitly (then it is created); call/ccc
  never invents a Claude Code data directory, and writes nothing into the home except the session file.
- Install path: `<home>/projects/<project-dir from the source cwd>/<new uuid v4>.jsonl`.
- Creation is exclusive and never replaces anything, by the same mechanism the Codex installer uses:
  bytes to a sibling temporary file (0o600), final name taken with a hard link; a filesystem without
  hard links is refused (`ErrNoExclusiveInstall`) rather than published by a rename.
- The source-file digest comparison (`installation.source_check_state`) is unchanged from 0002: taken
  before the conversion, compared after, `unchanged` / `changed` / `not-verified` with the reason.
- Nothing else in the target home is touched: no `.claude.json`, no settings, no credentials, no other
  session.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism | Cost | Reported as |
| --- | --- | --- | --- |
| structure | the written file is parsed back through the **inherited AgentsView Claude reader** and compared with what the writer placed in it (dialogue order and text, tool call/result pairing, the boundary and its summary) | offline, always | `verification.structure` |
| `load` | the installed `claude` binary runs `--bare -p --output-format json --resume <id>` against a **throw-away copy** of the converted bytes in a scratch config directory, with the environment variables that could reach a model removed. The loader resolves the installed session and its own result document names it (`session_id`, `terminal_reason`), which is the offline answer; a control arm runs the same command with a freshly generated, unknown session id, which must make the loader answer `No conversation found with session ID` — that contrast is what makes the check an answer about *this* session. No model turn runs and no model access is needed | offline, needs the target binary | `verification.load` |
| `resume` | the reported invocation runs **one real turn** in the session's working directory with the caller's own environment; the command's exit status, the harness's own answer, and the session file's growth are reported | one model turn | `verification.resume` |

`--bare` is what keeps that run offline: the harness then takes its authentication strictly from
`ANTHROPIC_API_KEY` or an `apiKeyHelper`, neither of which the check provides, so no credential store is
read and no turn can be billed — and the check drives the loader against a *copy* because a resumed
session gets the harness's own records appended to it (measured), which a verification must not do to the
session the caller is about to continue.

Two mechanisms were measured and rejected before this one: a loopback endpoint answering `401` made the
harness retry past 120 s (a refused connection did the same), and a *live* endpoint would have billed a
turn at the default verification level. The request the loader builds from a converted file was still
recorded — with a logging proxy, as a design instrument rather than as part of the shipped check — and it
is what §2.1's "loader's own view of a session" row reports.

`--verify` defaults to `load`; exit codes are 0002's: conversion/installation failure or a *requested*
verification that ran and failed exits 1; a verification that could not run (the binary is absent, a
dry run installed nothing) is reported as not verified and does not fail the run; `--verify resume`
requires `--resume-prompt`.

## 5. CLI and machine-readable output

```
ccc continue <provider>:<session-id-or-path> --to claude [--json] [--dry-run] [--target-home DIR]
             [--target-binary PATH] [--data-dir DIR] [--verify none|load|resume] [--resume-prompt TEXT]
ccc continue <provider>:<session-id-or-path> --to codex  … (unchanged, 0002)
ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]
ccc help
```

The JSON document keeps 0002's shape exactly (`upstream_revision`, `source`, `target_session`,
`conversion`, `installation`, `verification`, `warnings`); the states, the three verification blocks and
the failure behavior are the same, so one consumer reads both routes. What changes with the target:
`target_session.format` (`claude-code-transcript-jsonl`), `target_session.provider` (`claude`), the
format-limit list, the reported `cli_version` (`2.1.326` probed, else the writer pin) and the resume
invocations, which name both the home and the working directory.

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| a single non-interactive command selects a Codex session and produces a Claude Code-native resume identity and command, demonstrated on a representative session | `ccc continue codex:<fixture> --to claude --target-home <isolated> --json` output in the evidence record, the same run without `--json`, and `ccc help` / `ccc continue -h`; `cmd/ccc/main_test.go` and `internal/ccc/continue_test.go` |
| Claude Code's native loader recognizes the installed session and accepts a continuing turn; loader-level evidence | `scripts/claude-native-check.py` on this host: the `load` check's own report (the loader's answer naming the installed session, with the unknown-id control arm) and a real turn run through the reported invocation (`--resume-prompt`), whose answer quotes a converted fact and whose session file grew, both in the evidence record |
| structured messages and tool events translate through the shared representation, ordering preserved, material loss explicit; an input/output semantic comparison including at least one nontrivial tool interaction | the source↔target table in the evidence record, the round-trip test (`claude_target_test.go`, `claude_install_test.go`), the `losses[]` block of the run's JSON, and the loader's own request — measured once through the logging loopback proxy and recorded in the evidence record's §3(a); the native check re-derives the loader's verdict and the real turn, not that capture |
| 0002's behavior still passes its accepted verification; the new route does not duplicate AgentsView's parsing | 0002's behavior re-verified by its tests within the gate's `test` step (two lines that named the newly supported target were updated — the CLI's `--to codex` spelling and the refusal case's target — and no assertion was weakened; see the evidence record's §6.2), the unchanged fixture/oracle tests, and `parity` (no carried file changes) |
| JSON and error behavior consumable without a skill or interactive prompt; neither original history nor unrelated target sessions modified | the CLI's JSON documents, the native check's before/after hashes of the source fixture, a planted unrelated session and every other session in the home, and the exclusive-install test |

The native harness is **not** a gate step (it needs Claude Code installed and, for a real turn, model
access), registered in `docs/harness.md` next to `scripts/upstream-compare.py` and
`scripts/codex-native-check.py` with the same status. The gate keeps deciding the offline half.

## 7. Gate, docs, bookkeeping

- Gate: **no new step and no new required step.** The new tests live in `internal/ccc` and `cmd/ccc`,
  which `scripts/gate-targets.txt` already builds and runs. `scripts/tests/test_evidence_claims.py`
  gains the 0003 evidence record as a second file it checks (its rule — an inline-code quotation in a
  §5 comparison cell that credits a recorded run's answer must occur verbatim in that run's recorded
  output — is what keeps the record honest), and the `selftest` step runs it.
- `AGENTS.md`: the layout (the Claude target writer and `scripts/claude-native-check.py`) and the target
  rule that a resumability claim is only ever the target harness's own answer.
- `docs/harness.md`: the new script's entry and why it is not a step.
- `docs/conventions.md`: whatever the increment changes about the rules of the work — the source-cwd rule
  for a target whose loader discovers sessions through the working directory, and the second target in
  the layout line. (Written into `docs/conventions.md` rather than `AGENTS.md`: it is a rule of the work,
  not of the increment's layout, and the evidence record's §6.2 names where each hunk landed.)
- `README.md`: the quickstart's `--to claude` invocation and its resume command.
- `docs/goals/README.md`: 0003 → "Implemented on `feat/0003-codex-to-claude-continuation`; awaiting
  operator acceptance". No goal moved in the implementation round: 0001 was already
  archived and 0002 was still awaiting operator acceptance (its merge onto `main` is what archives it).
  *After the fact:* a later bookkeeping round did move 0002 to `docs/goals/archive/` once `main` carried
  its merge (`8e1c502`), retargeting every link that named it; the move and its re-verified counts are
  recorded in the evidence record's §6.2.
- `docs/decisions/OPEN-QUESTIONS.md`: entries for the defaults this run had to take alone (the record
  shape, the cwd requirement, the compaction pair, sidechain omission, the offline `load` oracle and
  its control arm, the omitted model attribution, the resume-command working directory).
- `docs/upstream.md` and the carry are untouched: no carried file changes in this increment.

## 8. Out of scope, with reasons

- **`ccc formats --json`**: still deferred (0002's reason stands); the `--json` document already
  distinguishes the three states for the two routes that exist.
- **Summarization, selection, redaction, LLM calls**: excluded by the goal's bounds.
- **Driving the target's own `/import`, plugins, or settings**: `claude import` answers "not yet
  available in this build"; nothing in this increment writes settings, hooks or MCP configuration.
- **Writing Claude Code's sidecars** (tool-results files, subagent transcripts): the source read carries
  no bytes for them, and the writer never fabricates a side file a result's text would then name.
- **A second Claude Code source parser**: the round-trip check uses the inherited reader; the loader's
  request comparison reads the *loader's* output, not the file.
- **Copying credentials or login state into an isolated home**: the native check runs a real turn only
  when the host's own environment already has model access, and says so when it does not.
- **Interactive resume (`claude --resume` TUI), `--fork-session`, worktrees**: the resume command is
  printed for the caller.
- **Claude Code → Codex (0002, done), repeatable handoffs (0004), other targets**: separate goals.

## 9. Ambiguities, and the reversible defaults taken

1. **The source's working directory decides discoverability.** A Codex session whose read records no
   `cwd` cannot be installed where any loader would find it. Default: refuse the conversion
   (`ErrNoTargetCwd`) and report the conversion block without an installation, rather than invent a
   directory. Reversal: fall back to the caller's working directory, disclosed in the report.
2. **One record per message, `message.id` fresh per assistant record.** Default: a fresh id (the
   harness merged two records that share one). Reversal: derive ids from the source's own message
   identity where it has one, which would need a per-source mapping.
3. **`is_error` only for a recorded failure.** Default: write the flag where the source's status says
   the call failed, and count every other status as a narrowing. Reversal: write no flag at all (0002's
   `tool_result_outcomes` caveat).
4. **The model field.** Default: omit the source's per-message model and record no provider; disclose
   both. Reversal: write the source's model, which would claim the target's own model produced the
   conversation.
5. **`--verify load` runs the harness with `--bare` and no credential variables**, not the caller's
   model access, and adds an unknown-id control arm. Default: the check is offline, runs against a copy
   of the converted bytes, and answers "the loader resolved *this* session"; the report names the
   mechanism and quotes the harness's own answer. Reversal: an online load check against the caller's own
   endpoint, which costs a turn, writes into the installed session and cannot run without model access.
6. **The resume command carries the working directory.** Default: `cd <cwd> && CLAUDE_CONFIG_DIR=<home>
   claude --resume <id>`, rendered the way 0002 renders its commands (every word quoted where a shell
   would read it). Reversal: print the invocation without the `cd` and document the requirement.
7. **Pre-boundary history** is carried and the boundary is what makes the target ignore it (measured),
   exactly as in 0002. Reversal: stop the file at the boundary.
8. **`isSidechain` records are omitted** because the loader drops them from the model-visible history —
   carrying them would put evidence in the file that no read of it shows. Reversal: write them with the
   flag, and report that the target's own reads do not surface them.
