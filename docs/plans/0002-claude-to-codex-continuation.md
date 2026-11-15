# Plan — 0002 Continue a Claude Code session in Codex

Contract: [`docs/goals/0002-claude-to-codex-continuation.md`](../goals/0002-claude-to-codex-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Written before implementation. The target-side facts below were **measured on this host against the
installed Codex CLI (`codex-cli 0.169.0`)** before any code was written; every measurement is repeated
in `docs/evidence/0002-claude-to-codex-continuation.md`.

## 1. What this increment adds

1. A **target writer** for Codex under `internal/ccc`: a `Conversation` becomes a Codex rollout JSONL
   file, plus the fidelity report of what could not be carried.
2. A **continuation pipeline** (`Continue`) that reads a source session through the inherited
   AgentsView reader (source file first, archive second), writes the target, installs it into a Codex
   data directory without overwriting anything, and reports **converted / installed / natively
   resumable** as three separate states.
3. **Native verification** through Codex's own interfaces: `thread/resume` on `codex app-server`
   (offline load) and `codex exec resume <id> <prompt>` (an actual new turn).
4. A CLI: `ccc continue <provider>:<id-or-path> --to codex [--json] [--dry-run] [--target-home DIR]
   [--data-dir DIR] [--verify none|load|resume --resume-prompt TEXT]`.
5. A repeatable native-verification harness, `scripts/codex-native-check.py`, deliberately **not** a
   gate step (it needs Codex and, for a real turn, model access), plus the offline Go tests that hold
   the writer and the pipeline down.

## 2. Target route: a Codex rollout, verified by Codex's own loader

### 2.1 Why a written rollout and not a Codex-internal store

Codex 0.169.0 keeps two session representations: the **legacy rollout JSONL** under
`<codex-home>/sessions/<YYYY>/<MM>/<DD>/rollout-<stamp>-<uuid>.jsonl`, and a **paginated** thread store
(SQLite). Measured behaviour of the installed binary, with isolated `CODEX_HOME`s (every measurement is
repeated in the evidence record):

| Observation | Measured result |
| --- | --- |
| A hand-written rollout in the legacy shape, discovered by `codex migrate-rollouts` | `"status": "eligible"` |
| `codex migrate-rollouts` on a rollout holding a deliberately corrupt line and a bogus item type | still `"eligible"`: the command classifies from metadata, so it is **not** a parse oracle |
| `codex exec resume <that id> "<question about the converted history>"` | the new turn answered from the converted history (user turn, tool call, tool output and compaction summary all reached the model) |
| `codex app-server` + `thread/resume {threadId}` against the installed session | loads it; the request the interactive resume path uses, and offline (no model call) |
| A rollout without `payload.cli_version` in `session_meta` | `thread/resume` fails: `rollout … does not start with session metadata` |
| A rollout that records **no** `model_provider`, in a home whose state already holds migrated threads | `thread/resume` fails: `This chat uses model provider "", which is no longer configured` |
| The same rollout with `model_provider` recorded as the home's effective provider | `thread/resume` succeeds, in a fresh home and in a migrated one |
| `codex app-server` + `thread/start {ephemeral:true}` | answers the provider and model a **new** session would use (`openai` and the configured model, on a host whose configuration names no provider explicitly), writing no session file — while `config/read` reports only the *explicit* setting (null there) |
| A rollout whose messages are only `response_item`s (no `event_msg`) | loads and resumes, but the harness's own `thread/read` view of it is empty: no turns, no preview |
| The same rollout with the `event_msg` `user_message`/`agent_message` records the harness writes beside them | `thread/read` returns the turns with their messages, and the preview is the first user message |
| `codex resume <id>` in a terminal | cannot be exercised on this host: no TTY is available (`TERM=dumb…`), so the interactive loader is verified through the same `thread/resume` request its clients use |

Consequences taken as the increment's defaults:

- **Write the portable rollout, not the paginated store.** The paginated store is Codex's own
  SQLite schema; writing it from here would be a competing writer to a live Codex home, which the
  design forbids. The rollout is the format Codex reads, migrates and resumes, and it is the one the
  inherited AgentsView Codex reader also parses (so both sides of the evidence come from
  independent implementations).
- **Write the records the harness writes itself**: a `response_item` (the model-visible history) *and*
  the `event_msg` message event (the harness's own view of the turn). Without the second, a resumed
  session shows nothing in the TUI or in `thread/read`, which would make "installed" a claim about a
  file rather than about a session.
- **Never run `migrate-rollouts --apply` as part of installation.** It is Codex's own maintenance
  path, it failed with `missing_sqlite_metadata` in a home that already held a migrated thread on the
  tested version, and `codex resume`/`codex exec resume` work without it. The docs say the target
  harness may migrate the session itself, and the JSON output does not claim migration.
- **Record the target's *effective* provider**, asked of the harness itself (`thread/start` on an
  ephemeral thread, whose answer includes the harness's own default; `config/read` as the fallback).
  Recording nothing is what the harness refuses in a home that has migrated; recording the *source's*
  provider would be false; recording the configuration's explicit value alone is null on a host that
  relies on the default. The report says which answer was used, and the value is reported as a caveat
  (it binds the session to that provider, so changing the configuration later makes the harness refuse
  to resume it). No model is recorded, so the target keeps choosing its own.
- **`cli_version` is required by the reader**, so the writer always records one: the version the
  installed `codex --version` reports when it can be probed, otherwise the pinned constant
  `codexFormatVersion = "0.169.0"`, the version this writer's format was verified against. Which one
  was used is stated in the output (`target.cli_version_source`: `probe` or `writer-pin`).
- **`originator: "ccc"`, `source: "cli"`, `thread_source: "user"`.** The originator names the program
  that wrote the file (call/ccc) instead of impersonating `codex_cli_rs`; `source: "cli"` is what
  keeps the session in the interactive resume picker rather than behind `--include-non-interactive`.

### 2.2 The exact record shape written

One JSONL file, one object per line, each `{"timestamp": <RFC3339 UTC>, "type": …, "payload": {…}}`,
in this order:

| Source evidence | Codex record |
| --- | --- |
| session start | `session_meta` `{id: <new uuid v7>, timestamp, cwd: <source cwd>, originator: "ccc", cli_version, source: "cli", thread_source: "user", model_provider?: <the target's own effective provider>, git: {branch?, repository_url?}}` |
| each turn boundary | `turn_context` `{turn_id: <new uuid v7>, cwd: <source cwd>}` |
| user message text | `response_item` `{type: "message", role: "user", content: [{type: "input_text", text: …}]}` **and** `event_msg` `{type: "user_message", message: …}` |
| assistant message text | `event_msg` `{type: "agent_message", message: …}` **and** `response_item` `{type: "message", role: "assistant", content: [{type: "output_text", text: …}]}` (the harness writes the event first) |
| tool call | `response_item` `{type: "function_call", name: <source tool name>, arguments: <recorded input JSON, verbatim>, call_id: <recorded tool-use id or a generated one>}` |
| tool result | `response_item` `{type: "function_call_output", call_id: …, output: <recorded result text>}` |
| compaction boundary | `compacted` `{message: <recorded summary text>}` |

The timestamp of every record is the source message's recorded time when the source has one, else the
session's start time; the file is written in one pass, buffered, with each line re-marshalled from a
typed struct so no source byte is copied unparsed.

`cwd`: the source session's recorded working directory. It is carried because it is the provenance of
the conversation (and `thread/read` reports it), and the printed resume command says to resume from
that directory.

### 2.3 Mapping rules, category by category

| Category | Rule | Why |
| --- | --- | --- |
| Dialogue text | `Message.Dialogue` when the reader supplied a dialogue projection, else `Message.Content`; **for a message that carries tool calls, only the dialogue projection is written as text** | the reader's `Content` renders calls into text (`[Read: <path>]`), so writing both would duplicate the call in the target's history |
| Tool calls | written as `function_call` with the source's own name, verbatim `input_json`, and the recorded tool-use id | it is the recorded evidence; inventing a target tool vocabulary (`Read`→`exec_command`, …) would forge a call the source agent never made |
| Tool results | written as `function_call_output` under the call's id, with `parser.DecodeContent(raw)` text | the archive/source both store a content field that is a JSON string when the provider encoded it; the decoded text is what the tool produced |
| Results folded onto a call (archive reads) | `ToolCall.ResultContent`/`ResultContentSize` become the call's `function_call_output` | the archive stores a tool-result-only row folded onto its call; dropping it would lose the outcome |
| Compaction boundary | `compacted` with the summary text | Codex's own record for the same event; the target applies it to its model-visible history, so the converted session continues with the source's *active* context while the file keeps the pre-boundary turns |
| Messages before a compaction boundary | **carried** (not dropped) | the transcript records them, and the identity transform preserves available history; the boundary record is what stops them being presented as active context |
| `Thinking` | **omitted**, counted | the target's reasoning items carry provider-encrypted content; writing plaintext as `reasoning` would present source-private reasoning as a target-native artifact (goal 0013 owns this) |
| `IsSidechain` messages | **omitted**, counted | they are a subagent's turns recorded in the same transcript; the target has no subagent attribution for a flat message, so carrying them as ordinary turns would forge who said what |
| `IsSystem` records that are not compaction boundaries | **omitted**, counted | the target has no system role; its `developer` channel carries the target's own instructions, and writing provider bookkeeping there would present it as instructions |
| Tool results with no matching call | **omitted**, counted | a `function_call_output` without its call is not a pair; inventing a call would fabricate a tool invocation |
| A result whose persisted output the provider could not read (`PersistedOutputUnresolved`) | carried, **flagged** | its content is the placeholder naming the side file, not the output; the report says so |
| A result recorded with an outcome (`status`) | carried, **flagged** | the target's `function_call_output` has no outcome field; the recorded text travels as-is and the report says the status is not representable |
| Non-native tool names | carried, **flagged** | measured: the target loads them and its model-visible history shows them, and a resume answered from a call/output pair written this way |
| Attachments/images | described as a **format limit** in the output, not counted | the source read carries no attachment bytes, so a per-conversion count of "0 omitted" would be a claim the read cannot support |

Every category is listed in the output on every run, with its count (including zero) and its
disposition, so a consumer never has to infer what was left out.

## 3. Installation

- Target home resolution: `--target-home DIR`, else `$CODEX_HOME`, else `~/.codex`. A home that does
  not exist is an error unless `--target-home` was given explicitly (then it is created); call/ccc
  never invents a Codex data directory, and never writes outside the home it was given.
- Install path: `<home>/sessions/<YYYY>/<MM>/<DD>/rollout-<stamp>-<new uuid v7>.jsonl`, the layout the
  target's own discovery walks.
- Creation is exclusive (`O_CREATE|O_EXCL`, 0o600) after writing to a sibling temporary file and
  renaming, so an existing session can never be replaced and a reader never sees a partial file. A
  collision is reported, never retried into a different identity silently.
- The source is re-fingerprinted after installation and the result reported
  (`installation.source_unchanged`); nothing in the pipeline opens the source for writing.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism | Cost | Reported as |
| --- | --- | --- | --- |
| structure | the written file is parsed back through the **inherited AgentsView Codex reader** and compared with the source document (dialogue, roles, tool pairing, boundary) | offline, always | `verification.structure` |
| `load` | `codex app-server` (`--listen stdio://`) + `initialize` + `thread/resume {threadId}`, requiring the returned thread's id/sessionId and path to be ours | offline, needs the target binary | `verification.load` |
| `resume` | `codex exec resume <id> --resume-prompt <text>` in the target home; the command's output is captured, its exit status reported, and the session file's growth recorded | one model turn | `verification.resume` |

`load` was chosen over `codex migrate-rollouts --thread <id>` because the latter is a *metadata* query:
it reported a rollout with a deliberately corrupt line as `eligible`, while `thread/resume` rejects a
rollout Codex cannot read (measured on the missing `cli_version`) and returns the loaded thread for one
it can. It is also the request the interactive resume path uses, which matters because no TTY is
available on this host to drive `codex resume` itself. The app-server protocol is experimental and
version-bound, which is why the tested version is recorded and a protocol failure is reported as **not
verified with the reason**, never as a pass.

The harness's own *view* of the converted session (`thread/read`, printed by
`scripts/codex-native-check.py`) is what the evidence record compares the source against; it only shows
anything when the writer emits the `event_msg` message records, which is one of the measured reasons
they are written.

`--verify` defaults to `load`. Exit codes: conversion/installation failure, or a *requested*
verification that ran and failed, exit 1; a verification that could not run because the target binary
is absent is reported as not verified and does not fail the run (the conversation really was converted
and installed); `--verify resume` requires `--resume-prompt` (the turn it runs becomes part of the
user's session, so its text is the caller's, not an invented default).

## 5. CLI and machine-readable output

```
ccc continue <provider>:<session-id-or-path> --to codex [--json] [--dry-run] [--target-home DIR]
             [--data-dir DIR] [--verify none|load|resume] [--resume-prompt TEXT]
ccc inspect <provider>:<session-id-or-path> [--json] [--data-dir DIR]
ccc help
```

Shape discipline is unchanged from 0001: stdout carries the result (one object with `--json`),
diagnostics and progress go to stderr, `--json` output is stable, the human output is a short block,
and `--dry-run` converts and reports without touching any directory (the target file is built in
memory and never written).

The JSON document (one object) carries: `upstream_revision`, `source` (identity, counts, fidelity,
`read_from`: `source`/`archive`), `target` (`provider`, `format`, `format_limits`, `home`, `binary`,
`binary_found`, `binary_version`, `session_id`, `file_name`, `path`, `cwd`, `started_at`, `cli_version`,
`cli_version_source`, `model_provider`, `model_provider_source`, `model`, `resume_command`,
`exec_resume_command`), `conversion` (`state`, `format`, `read_from`, `dry_run`, `carried`, `losses[]`,
`source_fidelity`), `installation` (`state`, `path`, `mechanism`, `created`, `source_unchanged`,
`detail`), `verification` (`structure`, `load`, `resume` — each `{state: "verified"|"not-verified"|
"not-requested", …}`) and `warnings[]`.

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| one documented non-interactive command, with the native resume identity/command shown | `ccc continue claude:9f2c1d47-… --to codex --target-home <isolated> --json` output in the evidence record, plus the same run without `--json`; `ccc help` / `ccc continue -h` text; `internal/ccc/continue_test.go` and `cmd/ccc/main_test.go` |
| Codex loads the **installed** session and continues with a new turn | `scripts/codex-native-check.py` run on this host: `thread/resume` output (codex's own loader, isolated `CODEX_HOME`) and a real `codex exec resume <id> "<question>"` turn whose answer quotes converted facts; the session file's growth after the turn |
| a representative transcript keeps supported semantics, unsupported content reported | the `claude` fixture round-trip test (write → parse with the inherited Codex reader → compare), a second fixture/throwaway transcript carrying thinking, a sidechain message, a system record, an unpaired result and an errored result (`continue_test.go`), the `losses[]` block in the run's JSON, and the source↔target table in the evidence record |
| source unchanged, target identity new, no unrelated session replaced | the harness hashes the source fixture, a pre-existing unrelated Codex session and the target home's session tree before/after; `continue_test.go` asserts the same in-process (`hashTree`, existing-file hash), and the writer's exclusive create is pinned by a test |
| machine-readable output distinguishes conversion/installation/native-resume; help explains the common invocation | the three JSON blocks above, one test per state (`cmd/ccc/main_test.go`), and the `--help` text in the evidence record |

The native harness is **not** a gate step: it needs the Codex binary and, for a real turn, model
access. It is registered in `docs/harness.md` next to `scripts/upstream-compare.py`, with the same
status (`it exists, it is re-runnable, it is not what the gate decides`). The gate keeps deciding the
offline half: round-trip, identity, install, loss reporting, CLI shape, and the parity/selftest steps
it already runs.

## 7. Gate, docs, bookkeeping

- Gate: **no new step and no new required step**. The offline tests live in `internal/ccc` and
  `cmd/ccc`, which `scripts/gate-targets.txt` already builds and runs; the native check is a script,
  not a step. `docs/harness.md` gains the script's entry and the reason it is not a step.
- `AGENTS.md` grows: layout (`internal/ccc` target writers + `scripts/codex-native-check.py`), the new
  command, the rule that a target writer's resumability claim is only ever shown through the target
  harness's own loader, the rule that a converted session records no foreign provider/model, and the
  bookkeeping note for `docs/goals/archive/`.
- `docs/conventions.md`: the target-side rules (write a fresh native identity, never overwrite, never
  copy credentials/login stores, isolated data directories for tests and demos, disclose losses by
  category).
- `README.md`: the quickstart gains `ccc continue … --to codex` with the resume command.
- `docs/goals/README.md`: 0002 → "Implemented on `feat/0002-claude-to-codex-continuation`; awaiting
  operator acceptance"; 0001 moves to `docs/goals/archive/` (it is merged onto `main` as
  `ef6a61c`, which is the state the index itself says ends the "awaiting acceptance" wording).
- `docs/decisions/OPEN-QUESTIONS.md`: new entries for the defaults this run had to take alone
  (provider/model omission, `originator`/`source` values, `cli_version` source, the compaction-boundary
  decision, the sidechain/system/reasoning omissions, the verification-level defaults and exit codes).
- `docs/upstream.md` and the carry are untouched: no carried file changes in this increment.

## 8. Out of scope, with reasons

- **`ccc formats --json`** and a capability table for every provider: goal 0002's contract asks for a
  machine-readable distinction between conversion, installation and resume verification, which this
  increment's `--json` provides; publishing a capability matrix now would have to describe writers
  that do not exist (0003+).
- **Summarization, selection, redaction, LLM calls**: excluded by the goal's own bounds.
- **Driving Codex's paginated thread store or `migrate-rollouts --apply`**: a competing writer to a
  live Codex home; the target harness migrates on its own.
- **Interactive launch of Codex, pickers, TUI automation**: the goal asks for non-interactive
  conversion; the resume command is printed for the caller to run.
- **Copying auth/login state into an isolated home**: the design forbids it; the native check runs a
  real turn only when the host already has model access configured, and says so when it does not.
- **A second Codex source parser or a second session index**: the source side stays the inherited
  AgentsView reader, and the round-trip check uses it rather than a new one.
- **Codex→Claude (0003), repeatable/evolving handoffs (0004), Pi (0005)**: separate goals.

## 9. Ambiguities, and the reversible defaults taken

1. **Pre-boundary messages** — the goal says both "preserve available history by default" and "do not
   revive deliberately inactive history as though it were active". Default: carry them **and** carry
   the boundary as `compacted`; the target's own compaction semantics then decide what its model sees
   (measured: a `compacted` record truncates the model-visible history to the summary, and a resumed
   turn on the fixture answers from the summary and the post-boundary turns only), while the file keeps
   the recorded turns. Reversal: one branch in the writer to stop at the boundary.
2. **The provider binding** (measured in §2.1): recorded from the target's own answer because a session
   that records none is refused by the harness's loader in a home that has migrated, and because the
   configuration's explicit value is null on a host that relies on the harness's default. Reversal: stop
   passing `Provider` in `CodexTargetOptions` — the field is one value the writer omits when empty.
3. **`cli_version`** must be present for the target to read the file, and call/ccc did not write the
   conversation. Default: the probed target version, else a pinned constant naming the version the
   format was verified against, disclosed per run. Reversal: always write the constant, or refuse to
   convert when the target binary is absent.
4. **Verification default** (`load`) and exit codes: a missing target binary reports "not verified"
   without failing the run, while a requested verification that fails exits 1. Reversal: make `load`
   required (refuse to convert when Codex is absent) or make the absent binary a failure.
5. **`--verify resume` requires `--resume-prompt`**: a default prompt would insert call/ccc's words
   into the user's session. Reversal: a documented default prompt.
6. **Tool names are carried verbatim**, including names the target has no tool for; the report names the
   vocabulary as a caveat, and the measured target behaviour (a resumed turn answered from a foreign
   call/output pair) is stated rather than hidden. Reversal: a name mapping table, which would rewrite
   recorded evidence.
7. **The second fixture is a new parity-reserved file, not a replacement**: the oracle's cases now name
   the fixture they read (`internal/ccc/parity`'s `Cases`, mirrored by `scripts/upstream-compare.py`),
   so adding a transcript shape cannot silently change what a recorded upstream dump describes. The
   recorded dumps were not re-recorded: the cases still read the same bytes, and the offline test
   compares them byte for byte.
8. **No TTY on this host** means `codex resume` (the interactive TUI) cannot be driven here; the load
   verification uses the same `thread/resume` request, and the limit is stated in the evidence record
   rather than papered over.
