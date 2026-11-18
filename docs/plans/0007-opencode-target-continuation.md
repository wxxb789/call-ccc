# Plan — 0007 Continue supported conversations in OpenCode

Contract: [`docs/goals/0007-opencode-target-continuation.md`](../goals/0007-opencode-target-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0006 ([`docs/plans/0006-omp-continuation.md`](0006-omp-continuation.md)) — the last
target route — with 0002 for the route contract and 0004 for the source-boundary rules (unchanged here).

Written before implementation. Every OpenCode-side fact below was **measured on this host against the
installed OpenCode CLI (`2.2.5`, the `opencode` binary at
`~/.npm-global/lib/node_modules/@opencode/cli/bin/opencode.exe`, resolved from `~/.npm-global/bin/opencode`)**,
in throw-away homes (`XDG_DATA_HOME` under `/tmp`, `HOME`/`XDG_CONFIG_HOME`/`XDG_STATE_HOME` isolated),
before the writer existed; the measurements, commands and raw answers are repeated in
`docs/evidence/0007-opencode-target-continuation.md` §0–§1.

## 1. What this increment adds

1. A **target writer** for OpenCode under `internal/ccc` (`opencode_target.go`): a `Conversation` becomes
   OpenCode's own **session import document** — the `{info, messages}` JSON document
   `opencode session import` consumes — with a fresh native session id, fresh message ids, and the
   fidelity report of what could not be carried.
2. An **installer** (`opencode_install.go`): the document is materialised by the **harness's own
   importer** (`opencode session import <file> --directory <cwd> --standalone`) run with
   `XDG_DATA_HOME` selecting the target data directory; this build writes no SQLite row, no second
   writer, and nothing the harness would not read.
3. **Native verification** (`opencode_verify.go`):
   - *structure* — the installed session is read back through the **inherited AgentsView OpenCode v2
     reader** (offline; it reads `session_v2`/`session_message` from the database file the importer
     wrote) and compared item by item with what the writer placed in the document;
   - *load* — the harness's own reader (`opencode session export <id> --standalone`) answers about the
     installed session, with an unknown-id **control arm** (`Session not found: <id>`);
   - *resume* — the reported invocation runs **one real turn** through `opencode run --standalone
     --session <id>` and the session's growth is measured by the harness's own export.
4. A repeatable native-verification harness `scripts/opencode-native-check.py` (deliberately **not** a
   gate step) and its offline cases `scripts/tests/test_opencode_native_check.py`.
5. The usual documentation, evidence and bookkeeping (plan, evidence record, `AGENTS.md`,
   `docs/conventions.md`, `docs/harness.md`, `README.md`, `docs/goals/README.md`).

No fixture directory is added: 0007 is a **target-only** increment (OpenCode as a *source* is goal 0008,
whose reads come from the same inherited provider). The target document is exercised by the route's own
tests, by the inherited reader reading the installed session, and by the native check against the
harness's own export.

## 2. The route: an OpenCode session, installed by OpenCode's own importer

### 2.1 Why the importer, and not a written database row

| Observation (measured, `opencode 2.2.5`) | Result |
| --- | --- |
| `opencode session import <file\|URL> [--directory DIR]` exists and prints `Imported session: <id>` on stdout, exit 0 | the goal's "appropriately supported official import interface" exists; writing rows into `opencode.db` with ad hoc SQL is both forbidden by the goal's Bounds and unnecessary |
| `opencode session export [<session>]` exists and prints the harness's own JSON document for a session | the harness's own reader answers about the installed session |
| `opencode run --session <id>` exists (`--session`: "Session ID to continue, or **to create if it does not exist**") | a real turn is possible, but an unknown id *creates* a session, so the load check must not use `run` as its control arm (`session export` is the answering command: an unknown id exits 1 with `Session not found: <id>`) |
| the import document is exactly the export document: `{info, messages}`, and an imported session re-exports **identically** except `info.projectID` (recomputed from `--directory`), `info.location.directory` (set to `--directory`) and `info.time.updated` (the import instant) | the writer's document is the harness's own format, not a shape this build invented; the round trip is faithful at the message/part level (measured by structural JSON diff over a 11-message, 467 KB session) |
| import **preserves `info.id`** (`Imported session: ses_…`, the id the document carried) | the conversion mints its own fresh identity; the harness adopts it |
| a second import of the same id answers `Session already exists` on **stderr**, exit **0**, and leaves the existing session untouched | the installer must treat "already exists" as *not installed by this run*, never as success |
| an import that fails (`Bad Request` for a shape fault, `Internal Server Error` for a colliding message id) leaves the store **unchanged**: the refused session's rows are absent, no partial conversation is written | "a failed or partially completed import produces a truthful, recoverable result" |
| `--directory` must exist: `/nonexistent/xyz` → `LocationNotFoundError: Location not found: /nonexistent/xyz`, exit 1 | the recorded working directory is a real requirement of the importer (reported in its own words); a source that recorded none is still refused earlier (`ErrNoTargetCwd`) |
| the recorded directory is also required at **resume**: with it deleted, `session export` still answers (exit 0), `run --session <id>` answers `Location not found: <dirname>`, exit 1 | the load check works without the directory; the resume check reports the harness's own refusal when this host lacks it (the OMP route's rule) |
| the session a run *needs*: `info.model` is the binding a turn runs under — with an empty config, `run` answers `provider.no-route: Model unavailable: tencent-kimi/kimi-k3` (the document's own `info.model`); with `info.model` absent it ran under the **config's default model** (measured: the config default was attempted, `step_start` emitted), and with a nonsense per-message `agent` it still resolved the model | `info.model` is the session's own model binding, written from the harness's own answer; per-message `model.providerID`/`agent` do not gate a turn |
| a **failed** turn still writes to the session (user message + `idle` record added; `idle_outcome: failed`) | any check that runs a turn drives the **installed** session (as on Pi/OMP a real turn does); only the write-free `session export` may be used as the default level |
| `session export` is **content-neutral**: rows (`session_message`), the session row (`time_viewed` stays NULL) and the exported JSON are byte-identical before and after; the database *file's* bytes do move (WAL handling) | the load check may answer from the installed home without copying the database (the check states what it measured); the report never claims the database file's bytes are unchanged |
| `session list` is **project-scoped**: run from the session's recorded directory it lists the session, from another directory it answers nothing | "a newly identifiable OpenCode session" is shown through the harness's own discovery, from the session's own directory |
| the data directory is `$XDG_DATA_HOME/opencode` (`opencode debug paths`: `data /home/…/.local/share/opencode`); there is **no** `OPENCODE_DATA`; `OPENCODE_DB` overrides the database path alone; `XDG_STATE_HOME` holds the service registration | the printed invocation pins the home with `XDG_DATA_HOME` (its parent), never `OPENCODE_DB` alone (that would leave the harness's other directories in the caller's own home) |
| `--standalone` runs a private server; without it a **managed background service** is used and a stray service's port collision fails the command (`Managed service port … already in use`) | every invocation this route runs — install, probe, load, resume — passes `--standalone`, and so does the printed resume line |
| required document fields, by bisect (each dropped ⇒ `Bad Request`): `info.id`, `info.projectID` (value **ignored**, recomputed; any string is accepted, `""` included), `info.cost`, `info.tokens{input,output,reasoning,cache{read,write}}`, `info.time.updated`, `info.location.directory`; assistant message `agent`, `model{id,providerID}`; tool part `state.input`, `state` content for `completed`, `state.error{type,message}` for `error`, and **`time`**; compaction `summary`, `status`, `reason` (`reason` is the harness's own enum: `auto`/`manual` accepted, `source-recorded` refused) | the writer writes exactly the fields the importer requires; optional fields (`title`, `subpath`, `info.model`, `info.outcome`, `time.idle`, `finish`, `executed`, `metadata`, reasoning `native`/`time`, idle messages) are left out rather than filled with invented values |
| tool states the importer accepts: `completed` (with a **non-empty** `content` block holding a non-empty text) and `error` (with `error.type` and `error.message`); `pending`/`running` are **refused** | a call with no recorded result, or with an empty result text, has no writable shape: it is left out and counted, never written as an invented error or a pending state |
| message ids may be any string (`msg_` + 26 characters of any shape imported and re-exported), but must be unique across the database (a duplicate aborts the import) | the conversion mints fresh message ids beside the fresh session id |
| `info.model`, message `model` and `agent` are stored verbatim, including `providerID: ""` and a nonsense agent id | absent answers can be written as empty/omitted rather than invented |

Consequences taken as this increment's defaults:

- **The conversion writes the harness's own import document and the harness's own importer installs it.**
  Everything else (a written database row, a hand-built SQLite file) is refused by the goal's Bounds and
  by the core design's "no competing writer".
- **The data directory is the directory the harness's own rule computes** (`$XDG_DATA_HOME/opencode`).
  `--target-home DIR` names the OpenCode data directory; a directory whose base name is not `opencode`
  is read as the XDG data root whose `opencode` directory is the data directory — the harness's own rule,
  applied to the name the caller gave, so the reported home, the `XDG_DATA_HOME` the invocation prints
  and the directory the harness reads are the same directory (see §3.1).
- **A fresh identity the harness adopts**, exclusive by construction: the installer first asks the
  harness whether the id already resolves (`session export <id>`), refuses the install if it does
  (`ErrTargetExists`), then imports and requires the harness's own `Imported session: <id>` answer; an
  `already exists` answer from a concurrent writer is answered as "not installed by this run" too.
- **The session's own model binding and agent are the target home's own answers, asked of the
  harness** (`opencode debug agents --standalone`: the first primary, non-hidden agent — measured
  `build` with the owner's configuration *and* with an empty one, with the resolved
  `{id, providerID, variant}` beside it), written into `info.model` (omitted, with its detail, when the
  harness answered none — a session with no binding resumes under the home's configured default,
  measured) and into every assistant record's `agent`. The *provider* a message's `model` states is the
  same target-home answer where the harness was asked (the read answers the source's model but no
  endpoint), the message's `id` is the source's own model where the read answered one and the target
  home's model id where it did not, and the id is left empty where neither answered — the member is
  required and its values need not be (measured: an empty id is imported, stored and exported as
  written). Every record's attribution is counted and described (`message_model_attribution`, and
  `target_model_id_attribution` for the records the read answered no model for), never the source's
  provider.
- **`info.projectID` is written as the empty string**, not a hash this build would have to invent: the
  importer requires the field and overwrites it with the project id its own rule computes for
  `--directory` (measured: the value in the document is ignored).
- **Zero usage counts.** The document's `info.cost`/`info.tokens` are required, and the conversation
  document carries no usage (the read's own message fields hold none), so they are written as zeros and
  named as a caveat (`usage_zeroed`) — as the Pi and OMP routes do for their `usage` blocks.
- **No title, no idle marker, no `finish`, no `outcome`.** A session's title, its idle marker and the
  finish reason of a turn are the harness's own to record; the read answers none of the three and the
  importer accepts their absence.

### 2.2 The document written

One JSON document (`encoding/json` with a stable field order is not required by the importer; the
writer emits `{info, messages}` in that order):

| Member | Source evidence | Value written |
| --- | --- | --- |
| `info.id` | — | a fresh `ses_` identity (the harness adopts it) |
| `info.projectID` | — | `""` (the importer requires the member and overwrites its value) |
| `info.model` | the harness's own answer (`debug agents`) | `{id, providerID, variant}` — omitted, with the report's detail, when the harness answered none |
| `info.cost`, `info.tokens` | — | zeros (`usage_zeroed` caveat) |
| `info.time.created` | the session's start (the first message's timestamp, else the run's clock) | Unix ms |
| `info.time.updated` | the same instant | Unix ms (the importer rewrites it to the import instant) |
| `info.location.directory` | the source session's recorded working directory | that directory (required; refused earlier when the source recorded none) |
| each **user** message | a user message's dialogue | `{id, time:{created}, text, files:[]}` |
| each **assistant** message | an assistant message's dialogue, reasoning and calls | `{id, time:{created,completed}, type:"assistant", agent, model, content:[…]}` |
| — dialogue | the message's dialogue projection | one `{type:"text", text}` block |
| — reasoning | the message's persisted reasoning trace | one `{type:"reasoning", text}` block (no `native`: the read answers no provider-native payload) |
| — tool call with a recorded, non-empty **outcome** | the call and its paired terminal result | one `{type:"tool", id, name, state, time:{created,completed}}` block with `state.status` `"completed"` (`input` = the object written for the call's own input, `content:[{type:"text", text:<the result text>}]`) or `"error"` (`error:{type:"tool.execution", message:<the result text>}`). A result event whose status is progress rather than an outcome (the inherited readers' `running`/`pending`/`streaming`/`started`/`in_progress`: a Codex subagent notification streams in while its call executes) is **not** an outcome: the terminal event the same call recorded is what the part carries (counted as `nonterminal_tool_events`), and a call whose read holds only progress has no outcome here (counted as `unpaired_tool_calls`). A recorded status word with a third meaning — the `cancelled` outcome the Vibe and Amp readers preserve, a provider's own word — is not written either: the two states would each claim an outcome the source did not record, so the call is left out and the result counted (`tool_result_statuses`). The archive's derived summary column is a view of a call's result events, not a second outcome: it is written only for a call whose read recorded no result event at all, so a column restored from a progress event (the shape `messageFromArchive` carries beside that event) is not written as a `completed` state either |
| a compaction boundary | a record the source marked as a boundary | `{id, type:"compaction", time, status:"completed", reason:"auto", summary}` |
| timestamps | the recorded timestamps | a message's `time.created` is its own record's instant; a message's `time.completed` is that same instant (a zero-length turn: the read answers one instant per record, `assistant_completion_instant`); a tool part's `time.created` is the call's record instant and its `time.completed` the instant of the record the carried result was recorded on (the call's own, where the result came from the archive's derived summary). Each part's pair is read under the message's own instant as the clock's floor and raises the floor to its own end afterwards: siblings recorded on one message start together, so one call's completion cannot move another's recorded start, and an unordered source still cannot write a part whose end precedes its start |
| the document's message order | the read's own order | the order the conversation document holds (the importer assigns `seq` from it) |

### 2.3 Mapping rules, category by category

The family vocabulary the Pi/OMP writers use applies where the reason is the same; each category is
counted with the same shape (`Loss{Category, Count, Detail}`), and OpenCode-specific entries are added:

| Category | Rule | Why |
| --- | --- | --- |
| `dialogueText`, `whitespace_only_dialogue`, `reasoning_text`, `generated_call_ids`, `coalesced_tool_call_projections`, `empty_tool_inputs`, `unpaired_tool_results`, `empty_tool_results`, `whitespace_only_tool_results`, `unrepresentable_result_blocks`, `unresolved_tool_outputs`, `system_records`, `tool_role_messages`, `unclassified_role_messages`, `empty_assistant_messages`, `empty_user_messages`, `non_dialogue_content`, `sidechain_messages`, `pre_compaction_history`, `empty_compaction_summary`, `unresolved_source_parents` | the family's rules, unchanged in meaning (the source read's own projections, the pairing of results to calls, the empty-evidence counters) | one loss vocabulary for every route |
| `unpaired_tool_calls` | a call the source recorded with no outcome — no result at all, or only progress events: **left out** and counted | the importer accepts only `completed`/`error` tool states (measured); writing `pending` is refused, and inventing an error would fabricate a result |
| `nonterminal_tool_events` | a result event the read records as progress (`running`/`pending`/`streaming`/`started`/`in_progress`): **not written**; the terminal event the same call recorded is what the part carries, and a call with only progress is counted as `unpaired_tool_calls` — and is not given the archive's summary column in its place, because that column is a view of the events the read recorded | the harness's tool states are `completed` and `error`; writing a progress event's text under `completed` would state an outcome the source had not recorded when it wrote those words (measured source shape: a Codex subagent's `running` notification followed by its completion) |
| `tool_result_statuses` | a result whose recorded status word names neither a failure nor `completed` — the `cancelled` outcome the Vibe and Amp readers preserve, a provider's own word: the call is **left out** and the result counted (a failure word is written as the `error` state the target has a field for) | the harness's own states are `completed` and `error`, and its own reader resolves only those two as terminal (`internal/parser/opencode.go`); writing the word under `completed` would state a success the source did not record, and under `error` a failure |
| `empty_tool_results` | a call whose recorded result text is empty: the call is **left out** and counted | a `completed` state needs a non-empty text (measured); the empty result is a projection, not an instruction to invent |
| `message_model_attribution` | the provider on every assistant record is the **target home's own** answer where the harness was asked, and the model id is the source's where the read answered one, else the target home's — and the empty id where neither answered (measured: the member is required, its values need not be), which the report's own detail names | the read answers the source's model but no endpoint; the member cannot be left out, and no model this build cannot name is invented for it |
| `target_model_binding` | `info.model` written from the harness's own answer (or omitted, with its detail, when it answered none) | a turn runs under this binding; the source's model there would be a claim about the target's endpoint |
| `target_agent` | every assistant record's `agent` is the target home's own primary agent id (written `build` when the harness answered none: its built-in default agent) | the field is required and the read answers no agent; naming the source's agent would name an agent the target does not have |
| `usage_zeroed` | `info.cost`/`info.tokens` zeros | the conversation document holds no usage |
| `tool_part_times` | the `time` member a tool part requires is the recorded instant, else the entry clock | the member is required; the clock never goes backwards |
| `compaction_reason` | the boundary's `reason` is written `auto` | the member is required and must be one of the harness's own values (measured); the source read answers no reason, and the count names the value as the target's vocabulary rather than the source's answer |

**Format limits** (properties of the pair, not of one run): the source's session-level title, its
turn-level finish reasons and its idle/outcome markers have no member this build may fill (the harness
owns them); persisted reasoning keeps its text but not any provider-native payload (`native` is the
harness's own artifact); attachments and images carry no bytes through this read; tool names travel as
the source's own vocabulary; usage is zeroed (above); the harness's own container is a database, so the
conversion publishes a *session in a store* rather than a file (the report names the store).

## 3. Installation

### 3.1 The home

- `--target-home DIR`, else `$XDG_DATA_HOME/opencode`, else `~/.local/share/opencode`.
- The directory the harness itself reads is `$XDG_DATA_HOME/opencode`. A home whose base name is
  `opencode` is that directory (the invocation sets `XDG_DATA_HOME` to its parent); any other name the
  caller gives is read as the XDG data root (the invocation sets `XDG_DATA_HOME` to it and the harness's
  own rule puts the data directory at `<name>/opencode`). One rule, applied to the name given, so the
  directory the report names, the directory the printed line selects and the directory the harness
  reads are the same one. A missing directory nobody named is `ErrTargetHomeMissing` (the pipeline's
  existing rule); a named one is created by the installation.
- The child environment is the caller's, with `XDG_DATA_HOME` (every spelling) replaced by that value,
  `OPENCODE_DB` removed, and `--standalone` passed so no managed service is used or touched. The probe
  and the facts query run the same way (with a `--standalone` private server).

### 3.2 The import

1. Refuse an id that already resolves: `opencode session export <id> --standalone` in the target home;
   exit 0 means the id exists ⇒ `ErrTargetExists` (nothing was written).
2. Write the document to a temporary file **outside the home** (`os.CreateTemp` in the OS temporary
   directory, mode 0o600, removed afterwards): the home receives nothing but what the harness itself
   writes.
3. Run `opencode session import <file> --directory <cwd> --standalone` with the environment above.
4. Require the harness's own answer: stdout must carry `Imported session: <id>` with **this** session's
   id; an `already exists` answer (stderr, exit 0) or any other answer is a reported failure with the
   harness's own words. A failed import left the store unchanged (measured); the report says
   `not-installed`.
5. Report the installation path as `<data dir>/opencode.db` and the mechanism as
   `the target harness's own session importer (opencode session import)`.

The source file digest comparison, the source boundary block and the "source unchanged" state are
0002/0004's machinery, unchanged.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism | Cost | Reported as |
| --- | --- | --- | --- |
| structure | the installed session is read back through the **inherited AgentsView OpenCode v2 reader** by its own reference form `<absolute db path>#<session id>` (the path must lie under the configured `opencode` root and be named `opencode.db`, which is exactly where the importer put it) and compared item by item with what the writer placed in the document. The comparison's installed side is the **document's own dialogue records** (the file this build wrote; the importer's row-level fidelity is measured and recorded), read in the document's order | offline, always | `verification.structure` |
| `load` | `opencode session export <id> --standalone` in the installed home (a scratch, empty `XDG_CONFIG_HOME`/`XDG_STATE_HOME`, so no provider or credential of the caller's is involved) must answer with **this** session's id and its messages; the unknown-id control arm must answer `Session not found: <id>`; the check states that the answer was measured content-neutral (rows and exported JSON unchanged; the database file's own bytes move with WAL handling) | offline, needs the binary | `verification.load` |
| `resume` | the reported exec invocation runs **one real turn** from the session's recorded directory with the caller's own configuration and environment (only `XDG_DATA_HOME` pinned) and `--format json`; the exit status, the harness's own output and the session's growth — counted through the harness's own export (message count before/after) — are reported. A failed turn also writes a user record and an `idle` record (measured), which the growth count shows | one model turn | `verification.resume` |

`--verify` defaults to `load`; the exit-code rules are 0002/0003/0005/0006's. The *load* level's
mechanism is `session export` and not `run`, because `run` **creates** a session for an unknown id
(measured) and appends records to the session it opens (measured).

The **structure** comparison needs one seam this increment adds to the shared machinery: the installed
bytes a comparison reads are no longer always the file at `TargetPath` (for this target the target
path is a *reference* `db#id`, and the document's own bytes are the evidence the walk reads), so
`verifyStructure` takes a small `installedBytes` closure (every existing route passes a closure reading
its installed file; the OpenCode route passes the document's own bytes). Nothing else changes for the
existing routes.

Two route-specific comparison rules, each with its reason:

- The inherited OpenCode reader **renders a persisted reasoning trace into the assistant's text**
  (`"[Thinking]\n" + trace + "\n[/Thinking]"`, joined with the dialogue by newlines — the reader's own
  projection; measured in `internal/parser/opencode_v2.go`'s `loadOpenCodeV2Messages`). The route's
  read-back therefore takes an assistant message's dialogue as its `Content` (the reader's own view of
  the parts the writer wrote) and the walk's *intended* side spells the writer's items for that message
  the same way; a mutation that drops, reorders or re-spells a part moves one side and is reported. The
  reader also folds every message's calls onto one message, so the route's rules ask the reader's own
  roles (`user`/`assistant`; a system record — including a compaction row — is classified away, a
  boundary is compared as a boundary).
- A call's evidence is compared through the reader's own keys, so a `completed` tool state compares as
  its result text, an `error` state compares as the text its `error.message` carries (the reader's own
  reading of the state), and an empty argument object compares as no arguments (the shared
  `roundTripCallKey` rule).

## 5. CLI and machine-readable output

```
ccc continue <provider>:<session-id-or-path> --to opencode [--json] [--dry-run] [--target-home DIR]
             [--target-binary PATH] [--data-dir DIR] [--verify none|load|resume] [--resume-prompt TEXT]
ccc inspect opencode:<session-id-or-path>                       (the inherited reader, unchanged)
```

The JSON document keeps the established shape (`upstream_revision`, `source`, `target_session`,
`conversion`, `installation`, `verification`, `warnings`); what changes with the target is
`target_session.format` (`opencode-import-document`), `provider` (`opencode`), the format-limit list, the
reported `cli_version` (probed, else the writer pin `2.2.5`) and the resume invocations. `--to opencode`
is accepted wherever the existing targets are (help text, the refusal message's list, `Targets()`).

The printed resume line is a POSIX one because it changes directory (the session's recorded working
directory) and pins the data root:

```
cd <cwd> && XDG_DATA_HOME=<data root> opencode run --standalone --session <id>
cd <cwd> && XDG_DATA_HOME=<data root> opencode run --standalone --session <id> -- <prompt>
```

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| a representative Claude-origin conversation becomes a newly identifiable OpenCode session, verified by a native load/export/resume path | `scripts/opencode-native-check.py` (default direction `claude-to-opencode`): the CLI's `ccc continue claude:<fixture> --to opencode --target-home <scratch> --verify load --json` report, the harness's own `session export` answering with the installed session's id and its messages (the tool evidence it holds compared with the **source fixture's own bytes** — the recorded arguments and result text, not the conversion's items, so a writer that mangled either cannot pass by agreeing with itself), `session list` from the session's own directory showing it, and — with `--resume-prompt` — one real turn (the check's own, run through the report's exec-form vector, read against the harness the check probed and the session the conversion installed; the conversion itself verifies at `load` and runs no turn, so one request is one turn) whose answer and whose session growth (message count through the harness's own export) the record quotes. Every command the check runs is deadlined: an overrun is a recorded failure of its own and a failed exit status, so a hung harness cannot skip the planted session's removal or the run's final `RESULT` line |
| import without clobbering another session or modifying the source; a failed/partial import is truthful and recoverable | the native check's planted unrelated session (digested before/after), the source tree's digests, the import-probe cases in `internal/ccc/opencode_install_test.go` (an existing id refuses; an "already exists" answer is not-installed; a failed import reports the harness's own words), and §0's measured "a refused import leaves the store unchanged" |
| the target preserves the supported dialogue and tool evidence; omissions are disclosed with source-related evidence | the route's writer tests over the committed Claude fixture (dialogue, reasoning, calls/results, boundary), the structure read-back through the inherited OpenCode reader, the native check's comparison of the harness's own export against the source fixture's own recorded arguments and result text, and every run's counted losses |
| the Claude-to-Codex route and source-side parity remain intact; the ordinary command and JSON output describe OpenCode target support and its version limitations | the existing suites unchanged (Codex/Claude/Pi/OMP route tests, `continue_growth_test.go`), `parity` on an unchanged carry, `cmd/ccc`'s help/report tests, and the gate's transcript in the evidence record |
| the goal's Stop-and-report boundary (no safe import path) | §0's measured import/export interface, the probe's version answer (`2.2.5`) and the install/verify failures that name the harness's own refusals |

The native harness is **not** a gate step (it needs the OpenCode binary and, for a real turn, model
access), registered in `docs/harness.md` next to the other four with the same status, and its offline
cases live in `scripts/tests/test_opencode_native_check.py` (discovered by the existing `selftest`
step). The gate keeps deciding the offline half: the writer, the installer's exclusivity rules, the
structure read-back over a temporary database the *harness* wrote, and the CLI's presentation.

### 6.1 What the implementation measured on top of §2.1

Two of the measurements above were added while making the route verify against the real importer, both
found by the structural read-back rather than by reading the harness:

- **An assistant message's own completion instant is required.** A message whose `time` carries `created`
  alone is accepted by the importer and then **silently dropped** (exit 0, no row, no export entry) —
  bisected over the message's members, and the only member that changed the outcome was `time.completed`.
  The writer now writes the record's own instant as that member and discloses the choice
  (`assistant_completion_instant`), and a Go case pins the member on every assistant message.
- **A tool part's `state.input` is re-spelled by the importer.** The document's own
  `{"limit": 5, "file_path": "…"}` (the recorded key order, with the source's whitespace) is held and
  read back as `{"limit":5,"file_path":"…"}`: the store keeps the JSON value and the recorded key order
  while the recorded whitespace is not what it keeps. The structural comparison therefore canonicalizes
  that one member's JSON on both sides (`canonicalJSONValue`) while comparing every other member as
  recorded; a changed value is still a difference, and Go cases drive the recorded order, a changed value
  and a reasoning part's read-back rendering.

A third measurement is about the harness's *invocation* rather than its format: `--standalone` placed
after a `--` terminator leaves the run on the managed background service (measured: the turn answered
from a store the check's own session was not in), so no invocation this build runs — and none the native
check runs — leaves the flag past a `--` terminator. The check names it immediately after every
subcommand; the build carries it last on the subcommands that take no terminator (`session import <file>
--directory <dir> --standalone`, `session export <id> --standalone`, `debug agents --standalone`) and
immediately after `run` on the resume turn, whose words follow the terminator (`run --standalone
--session <id> -- <prompt>`).

Two further measurements were added by the review round's fixes (`338df94..37366ff`): a result event the
read records as progress is not the call's outcome — the terminal event the same call recorded is what
the part carries (the source shape is the inherited Codex reader's subagent notification stream:
`running`, then `completed`), and a call whose read holds only progress has no outcome here — and the
values the round-trip comparison asks about are the ones actually written: a tool part's `completed` is
the instant of the record the carried result was recorded on, and a tool-call item carries the object the
part's `input` member holds (the empty object where the source's value is not one), with the source's own
input still counted as `empty_tool_inputs`. The native check now runs
the **report's own** exec-form vector for its turn — read against the harness it probed, the session it
installed, the flag's position and the placeholder — instead of reconstructing the words, and the
conversion is asked for `--verify load`, so one `--resume-prompt` is one real turn. Its named
`--target-home` is read through the harness's own rule first (a directory whose own name is `opencode` is
the data directory itself), so the conversion and the check read one store.

The third review round's fixes (`33645b3` + working tree) close the same surface from four sides. The
**archive's derived summary** is written only for a call whose read recorded no result event at all: a
progress event is evidence the read holds, so a call whose only recorded event is progress keeps no part
even though the archive's column was restored from that event's text (the shape `messageFromArchive`
carries). **Each tool part's pair is its own**: the part's start is the message's own instant — not
whatever a sibling's completion had raised the shared clock to — and its end is the record its result
travelled on, with a result record whose own instant precedes the call's raising the end to the start
rather than writing a part that ends before it begins, because the clock handed both instants out one after
the other and the first completion to arrive moved every later call's recorded start up to itself,
silently rewriting the durations the source recorded. The **native check compares the
store's tool part with the source fixture's own bytes** (its `source_tool_evidence` reader: the recorded
input value and result text, with `is_error` read as the `errored` status the reader records) rather than
with the conversion's own items, so a writer that mangled the arguments or the output cannot pass by
agreeing with itself. And the check's **every command is deadlined** through the one runner: an overrun is
a recorded failure of its own and a failed exit status, where an escaping `TimeoutExpired` would end the
run before the planted session's removal and before any final `RESULT` line — and would leave that
session in a caller-named home for the next run's planting to meet.

The fourth review round's fixes close the outcome vocabulary's third arm and the two boundaries around it.
A result whose recorded status word names **neither a failure nor `completed`** — the `cancelled` outcome
the inherited Vibe reader preserves verbatim, a provider's own word — is **left out and counted**
(`tool_result_statuses`), where the vocabulary's only two states would each claim an outcome the source
did not record (a completion under `completed`, a failure under `error`), and the harness's own reader
resolves only those two words as terminal (`internal/parser/opencode.go`), so the archive's summary of the
event cannot restore it either. The **recorded working directory is written as it was recorded**: only its
trimmed emptiness is checked, because a path may end in a space and the trimmed form is a different
directory from the one the document, the importer's `--directory` and the printed resume command must
name. The **native check hands the conversion the caller's own `--target-home` spelling** rather than its
own normalized root: each side applies the harness's rule once, so a caller spelling that itself names a
data directory (`…/opencode/opencode`) resolves to one store on both sides, where the normalized root
would be normalized a second time by the route and split the two. And the **branch case reads the members
the writer puts dialogue in** — a user message's `text` and an assistant message's content parts — and
asserts the last record's own path stays while the sibling branch is absent, where searching only
`message.Text` never saw an assistant branch at all and let a reversed expectation sit unnoticed.

The fifth review round's fixes (`1a88631` + working tree) close the generated-id namespace the pairing
runs in and the nonzero importer branch's own evidence. **An id minted for a call that recorded none is minted
outside every id the read recorded** — each call's and each result's, carried or not
(`reserveRecordedCallIDs`, `freshCallID`) — because this target's pairing map is keyed by id across the
whole document: reusing a recorded call's spelling would replace that call's own entry and hand its
recorded result to the id-less sibling, and reusing a spelling a recorded result names (including one
whose call the read does not carry) would take a result this conversion cannot pair and present it as the
invented call's own outcome. **A nonzero importer run keeps the harness's own refusal beside the process
error**: measured, a missing recorded directory answers `LocationNotFoundError: Location not found: <dir>`
with exit 1, and a core API caller that left `ContinueOptions.Stderr` unset has nowhere else to read it,
so the returned error now carries the same bounded sentence the exit-zero path reports and still wraps
the process error for `errors.Is`/`As`.

The sixth review round's fixes close the two halves of one question — *what a printed line and a named
harness actually name*. **Every assignment the printed line's word vector carries must stand unquoted as
`NAME=`** (`native_check_common.spelled_as_invocation`): a shell reads a quoted assignment — in *any*
position, not only the leading one the rule once read — as the *program* to run, so a line whose second
assignment is spelled `'XDG_DATA_HOME=…'` sets no data root while its words read back as the invocation.
The rule now requires each name the vector carries (`home_env`, `also`'s names, and the sessions-tree one
the Pi and OMP routes pass in `session_dir_env`, which reaches the spelling check through
`reported_invocation` for the first time — that name was compared as a word and never read as a spelling).
And **the harness word the native check drives is resolved once, absolutely, before any command runs**
(`opencode-native-check.harness_executable`): `shutil.which` answers a spelling that names a path
unchanged, so `--opencode ./tools/opencode` stayed relative and the stages that run *from the session's own
directory* — the export, its unknown-id control arm, the listing that must not name the session, and the
turn — raised `FileNotFoundError` out of the run, before its cleanup and its `RESULT` line, leaving the
planted session in a caller-named home (measured). The conversion's CLI resolves the same word against its
own directory, so one program now does the probing, the planting, the reading and the turn, and a spelling
that means different programs in the two directories is refused by the report's vector reading, which
names both words.

The seventh review round's fixes close the two remaining ways this route could state something untrue: one
about a source shape it reads, one about the installation it publishes. **A record whose Content is the
reader's own rendering of its persisted reasoning trace has no dialogue** (`thinkingRenderingOnly`,
`dialogueText`): RooCode's, Kilo legacy's and Cline's readers render a trace into Content without
projecting dialogue at all (internal/parser's own rule for the shape, `IsThinkingOnlyContent`), so the
writer's Content fallback wrote the trace twice on this target — once as the `reasoning` part the format
persists a trace in and once as a `text` part holding the rendering — presenting persisted reasoning as
words the agent said. The rule is applied only where the record actually carries a trace, so text nothing
shows to be a rendering of reasoning is not dropped, and the read-back rule is unchanged (the intended
dialogue is still the rendering the inherited reader makes of the parts). **A failed importer process is
reconciled, never read as an absence** (`InstallOpenCodeSession`): measured, the harness's own importer
commits the session and then exits, so a deadline or a cancellation in between leaves the store holding
the session while the run sees only a process error. The harness's own acknowledgment line in the captured
output is the first answer; the harness's own reader (`session export`) the second, asked with a context
that outlives the deadline that stopped the import because that deadline is exactly what would make the
question unable to answer; where neither answers, the run reports the installation block's third state,
`unresolved`, rather than a `not-installed` nobody observed. The published answer keeps the installer's own
convention — the path beside the error — so the report still carries the identity the session is resumed
by.

The eighth review round's fixes extend both halves of that sentence to the shapes a real reader produces
and a real store holds. **The reasoning rendering is removed from a record's own text, not only from a
record that is nothing but a rendering** (`dialogueFromContent`, the shared `dialogueText` every writer
reads its text through): ZCode's reader renders a persisted thinking part *beside* the record's text (its
own transcript case asserts `[Thinking]\n<trace>\n[/Thinking]\n<spoken>`), and the round-7 rule recognised
only the whole-Content shape, so such a record's trace was written twice on every target — once as the
record's reasoning and once inside the dialogue. The rendering is now removed where it stands (a reader
that keeps the record's part order may put it after the text it followed), only where the record's *own*
trace stands behind the block's words, and with the line feeds that joined it to its neighbours; a record
whose trace is empty, a block no trace of that record holds, a marker with no closer, and an empty block
keep their Content byte for byte, which is what makes the rule precision rather than a rewrite. **The
native check reconciles its own planting, and never removes what it cannot show is its own**: a store the
caller named is read for the id the check plants before anything is written (the store's own refusal,
naming that id, is the only answer that makes it free), an earlier run's leftover is recognised by the
store's own document — the check's fabricated id, directory and message text — and removed through the
harness's own delete, a session holding anything else stops the run untouched, and an import whose process
failed or overran its deadline after committing is read in the route's own order (its own acknowledgment
line, kept with the deadline, then the harness's reader) so its fixture is removed again instead of
staying to block every later run.

The ninth review round's fixes tighten both rules the eighth round wrote, because each was still loose by
one step. **The native check recognises its own leftover by the whole document, not by a member of it**
(`unrelated_is_this_checks`): the eighth round's predicate accepted any document holding the planted text
under the planted id and directory, so a leftover a caller had since **continued** — the very session the
check exists to leave alone — authorised `session delete` over the caller's own history. Measured natively
on 2.2.5: with the fixture planted and one turn run in it (the harness's own `run --session`, which
appends even when its model call fails), the pre-fix predicate answered the id free and deleted the
4-message session (`Session not found` afterwards), while the fixed one leaves it resolving with the
caller's turn and records the refusal. The comparison is now the complete fixture, member for member, with
only the two members the harness's own importer is measured to rewrite normalized away
(`without_importer_metadata`): `info.projectID`, recomputed from the session's recorded directory (the
planted `""` comes back as the directory's project hash), and `info.time.updated`, rewritten with the
import's own instant. **The rendering removal keeps the record's own bytes** (`thinkingRenderingOnly`,
`dialogueFromContent`): (a) the reasoning-only shortcut now requires the block's words to belong to the
record's own trace, exactly as the beside-the-text removal always did, so a `[Thinking]…[/Thinking]` block
a record carries as its own content — internal/parser's Vibe reader records content and
`reasoning_content` independently and renders nothing, and a model can write the markers itself — stays
the record's dialogue while the trace travels as its reasoning; and (b) the removal strips only the one
line feed the reader's own join put on each side of the rendering, so the feeds a neighbouring text part
records survive (ZCode joins its parts with `strings.Join(parts, "\n")`, and a recorded blank line is a
paragraph break), and the parts come out joined as the reader joins neighbours.

## 7. Gate, docs, bookkeeping

- Gate: **no new step and no new required step.** New tests live in `internal/ccc` and `cmd/ccc`, which
  `scripts/gate-targets.txt` already builds and runs; `scripts/tests/test_opencode_native_check.py` is
  discovered by the existing `selftest` step.
- `AGENTS.md`: the route list (writer/installer/verifier file names), the native check's name, and the
  OpenCode-specific rules (the importer as the only install path, the data directory's own rule and the
  `XDG_DATA_HOME` spelling, the fresh identity the harness adopts, the content-neutral export the load
  level uses, the required document members).
- `docs/conventions.md`: the layout block gains the three files; the install rule gains OpenCode's
  data-directory rule and the importer; the fixture note records why 0007 adds no fixture directory.
- `docs/harness.md`: the new script's entry and why it is not a step.
- `README.md`: the capability sentence, the quickstart's `--to opencode` invocation and resume line,
  the "what continue writes" paragraph, the loader list and the `--target-home`/`--target-binary` notes.
- `docs/goals/README.md`: goal **0006 is merged onto `main` as `338df94`** (HEAD), so it moves to
  `docs/goals/archive/` with its index row updated in this increment; **0007** takes the
  "Implemented on `feat/0007-opencode-target-continuation`; awaiting operator acceptance" spelling.
- `docs/decisions/OPEN-QUESTIONS.md`: entries for the defaults this run had to take alone (the importer
  as the only install path and installation requiring the binary; the data-directory rule and the
  `XDG_DATA_HOME` spelling; the empty `projectID`; the zeroed usage; the target-home agent/provider on
  every assistant record; the compaction `reason` written `auto`; the load level's export rather than
  `run`; the content-neutral export claim and the database file's own bytes).
- `docs/upstream.md` and the carry are untouched: no carried file changes.

## 8. Out of scope, with reasons

- **OpenCode as a source** (`--to codex|claude|pi|omp` from `opencode:<id>`): that is goal 0008. The
  inherited reader already parses OpenCode v2 databases (this increment reads the installed session
  through it), but wiring the source route, its fixture tree and its evidence is 0008's increment.
- **Writing the database ourselves** (or a `storage/` JSON fallback): forbidden by the goal's Bounds
  while an official importer exists, and a second writer is forbidden by the core design.
- **`OPENCODE_DB`-only pinning**: it moves the database but leaves the harness's other directories
  (`log`, `repos`, `shell`, snapshots) in the caller's own data directory, which the non-destructive
  rule forbids; `XDG_DATA_HOME` moves the whole data root.
- **Copying the caller's data directory for the load level**: measured unnecessary (`session export`
  is content-neutral), and copying a live database another process may be writing is a worse failure
  mode than the one it would prevent. The load level reports the measurement it rests on.
- **A committed OpenCode fixture directory**: 0007 is target-only; the document is the harness's own
  format and is exercised against the *installed* session and the harness's own export. The source
  fixtures stay as they are.
- **`ccc formats --json`**: still deferred, as in 0002–0006.
