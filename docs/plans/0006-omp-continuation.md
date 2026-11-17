# Plan — 0006 Continue conversations into and out of Oh My Pi

Contract: [`docs/goals/0006-omp-continuation.md`](../goals/0006-omp-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0005 ([`docs/plans/0005-pi-continuation.md`](0005-pi-continuation.md)).

Written before implementation. Every OMP-side fact below was **measured on this host against the
installed OMP CLI (`18.18.7`, the `omp` binary at
`~/.local/share/mise/installs/oh-my-pi/latest/omp`)**, in throw-away data directories, before the
writer existed; the measurements, the commands and the raw answers are repeated in
`docs/evidence/0006-omp-continuation.md`.

## 1. What this increment adds

1. A **target writer** for OMP under `internal/ccc` (`omp_target.go`): a `Conversation` becomes an OMP
   session JSONL file (the Pi-family v3 tree, plus the records OMP's own writer puts there and its own
   loader needs), with a fresh native session id and the fidelity report of what could not be carried.
2. An **installer** (`omp_install.go`): the session is published exclusively into the project
   directory **OMP's own spelling** derives from the session's working directory — which is not Pi's
   spelling — and read back through the inherited AgentsView OMP reader.
3. **Native verification** through OMP's own loader (`omp_verify.go`): an offline `load` level that
   starts `omp --mode rpc --no-tools --resume=<id>` against a throw-away copy in a scratch home that
   defines a credential-free provider, reads the harness's own `get_state`/`get_messages` answers, and
   runs the unknown-id control arm; and a `resume` level that runs one real turn through the reported
   invocation.
4. **The OMP source direction, unchanged in shape**: `omp:<session-id-or-path> → --to codex|claude|pi`
   already works through the same route table, reader and writers, because the inherited AgentsView OMP
   reader is registered (measured: `ccc inspect omp:<real id>` reads a real 200-message session with no
   code change). No OMP-specific shortcut is added; the common intermediate model carries it.
5. Committed **native-shaped OMP fixtures** (`internal/ccc/testdata/fixtures/omp/…`, written by the
   existing `internal/ccc/fixturegen`) that carry the OMP-specific shapes (the rewritable title slot
   line first, `custom` records, `runtime` metadata) beside the family's ordered dialogue, a tool
   call/result pair, a compaction boundary and a branch; plus a growth fragment
   (`internal/ccc/testdata/growth/omp-complete-turn.jsonl`).
6. A repeatable native-verification harness, `scripts/omp-native-check.py`, deliberately **not** a gate
   step (it needs the OMP binary installed and, for a real turn, model access), plus the offline Go
   tests that hold the writer, the installer, the routes and the OMP-specific regression cases down.

## 2. The route: an OMP session, verified by OMP's own loader

### 2.1 Why a written session and not a native import

| Observation (measured, `omp 18.18.7`) | Result |
| --- | --- |
| `omp --help` | `--from-claude` / `--from-codex` exist ("Import a Claude Code session into OMP"), and there is no `--to-*` or export-to-another-harness route. The import is a **picker-driven** in-harness flow with no argument that names a source session, a target directory or a result on stdout; it also presupposes a Claude/Codex source, which cannot serve the other supported sources (`pi`, `omp` itself). It is therefore not a route this pipeline can drive for a named source and a reported identity | 
| session files on disk | `<agent dir>/sessions/<project spelling>/<timestamp>_<id>.jsonl`, one JSON object per line |
| the agent directory | `PI_CODING_AGENT_DIR` relocates it (default `~/.omp/agent`) — **the same variable name Pi uses**; two homes never see each other's sessions |
| `PI_CODING_AGENT_SESSION_DIR` | replaces the sessions *root* and is searched **flat**: a session placed in a project subdirectory of the named tree is *not* found (`Error: Session "<id>" not found.`), while one directly in it is |
| id lookup | `--resume <id>` resolves by prefix through the home's sessions tree **regardless of the process's working directory** (measured from another directory: the same session, no fork offer, no relocation) |
| hand-written file | a v3 file holding a header, `model_change`, `thinking_level_change`, `user`, `assistant` (with a `toolCall` block), `toolResult` and a second `assistant` entry **loads**: `omp --mode rpc --resume=<id>` answered `get_state` with that session id and `messageCount: 4`, and `get_messages` with the four roles in order |
| control arm | the same command with an id nothing installed answers `Error: Session "<id>" not found.` and exits 1 |
| the load itself writes | the open session gains a `custom` record (`customType: "session_exit"`, `{reason:"dispose",kind:"normal",recordedAt:…}`), so a load check must drive a **copy** |
| real turn | `omp -p --resume=<id> … "Reply with exactly: PROBE-REPLY-OK"` answered `PROBE-REPLY-OK`, exit 0, and **appended** its own records (`custom/date-cwd-reminder`, the user turn, the assistant answer, `custom/session_exit`); the pre-existing bytes are byte-identical: a turn only appends |
| a home with no model | refuses to start at all (`No default model selected. Use /login, set an API key environment variable, or select a local model`) — a load check's scratch home must define one |
| `models.yml` `apiKey: $VAR` / `${VAR}` | **not** expanded (sent literally: `401 … Received=${TE****KEY}`); the built-in `litellm` provider reads `LITELLM_BASE_URL`/`LITELLM_API_KEY` from the environment and a real turn answered `LITELLM-OK` with no credential in any file |
| a provider defined with `auth: none` | accepted, and the harness starts and loads a session with no credential anywhere |
| a title slot line | every session OMP itself writes starts with `{"type":"title","v":1,"title":…,"updatedAt":…,"pad":…}`, a **fixed 256-byte line** (the shipped `SESSION_TITLE_SLOT_BYTES = 256`; measured: all 300 sampled first lines are exactly 256 bytes including the newline). A file **without** the slot loads and resumes too (measured) |
| OMP's project-dir spelling | three branches, from the shipped `yi()` (confirmed by observation): a working directory under the OS temporary directory spells `-tmp` + its path relative to it (`/tmp/x/y` → `-tmp-x-y`, and this branch wins when both apply); otherwise under the home directory, `-` + the home-relative path (`/home/u/repos/p` → `-repos-p`); otherwise Pi's absolute spelling (`--<path without its leading separator, separators → ->--`) |
| a session filed under Pi's spelling | is **moved** into OMP's own project directory by the harness itself (measured: the file left the directory this run installed it into), so installing under Pi's spelling mutates the installation |
| the model binding | the session's model is restored from its `model_change` record's `model` (a single `provider/id` string) when that resolves in the home, else from the session's own recorded message pairs; a pair the home cannot resolve is refused (`SessionModelRestoreError: Could not restore model <provider>/<id>`, exit 1). A session with no model recorded anywhere is refused the same way (`undefined/undefined`), while one whose only record is its header loads |
| the per-message pair | **every** assistant record needs a non-empty `model` and a non-empty `provider`, whatever the session's own binding says: dropping either from one record of an otherwise loadable session makes the next turn fail while building its request (`undefined is not an object (evaluating 'e.length')`), and `api`, `usage`, `stopReason` and `runtime` are all optional (each measured by dropping it and re-running) |
| a foreign provider label on a message | accepted at load **and** at a real turn (measured with a provider the home does not define, and with an `api` the home does not use): it is inert once the session's own binding resolves |
| without credentials | the load path resolves a session with `auth: none` and no key; only a real turn needs one |

Consequences taken as this increment's defaults:

- **Write the session file, not a store.** OMP's own session JSONL is what its loader reads, what the
  inherited AgentsView OMP reader parses (so both halves of the evidence are independent
  implementations), and the only artifact a resume picks up.
- **The OMP spelling, not Pi's.** The installer files the session under the project directory OMP's own
  `yi()` computes — the three-branch rule above, which for a working directory under `/tmp` or under
  the home is a *different* name from Pi's `--…--`. Installing under Pi's spelling would leave the
  session where OMP's own migration moves it, which is a different file path from the one the
  conversion reported.
- **The printed resume command names the home, the working directory *and* the directory that holds the
  session's file.** The plan first read the measurement as "never name `PI_CODING_AGENT_SESSION_DIR`"
  (line 48's flat-search finding); the second measurement corrected it: the variable names the directory
  holding session files **directly**, so naming the *project directory* resolves the session and naming
  the `sessions` root above it answers `Error: Session "<id>" not found.` — naming it is therefore what
  keeps a caller's leftover value, or a home's own `settings.json`, from hiding the session the pasted
  line was verified against, and the shipped line names it. The exec form is
  `omp -p --resume <id> -- <prompt>`, the invocation the resume check runs.
- **A fresh identity per conversion**, installed exclusively, with the source never opened for writing
  — the rules every other route already follows.
- **The session's own model binding is the target home's own answer, asked of the harness** (the
  `codexFacts` precedent): a `model_change` record naming `<provider>/<id>` the harness answered for
  itself. Without one, OMP resolves the session's model from its *messages* — and a converted message
  carries the source's model, which the target home need not define — so the session would be refused
  by every home that does not happen to know the source's model. Where the harness answers nothing
  (a home with no model configured, or a dry run that does not start it), no binding is written and the
  report says so, exactly as the Codex route treats its own unanswered provider.
- **Every assistant record carries the source's `model` and the target home's own configured
  `provider`.** The provider is the field OMP's request builder requires and the read answers no
  endpoint; the conventions' rule is that such a field states "the target's own configured endpoint
  where the harness was asked", so that is what is written, and the report names it as the
  attribution the read cannot answer rather than presenting it as the source's.
- **The title slot line is written, empty.** It is OMP's own initial record shape (OMP writes exactly
  this line, with an empty `title`, into every session it starts and rewrites it in place when the
  session is titled), it is 256 bytes so a later rewrite is in place, and omitting it would leave a file
  whose shape differs from every session the harness writes itself.
- **A branch the source recorded is not flattened**: like the Pi route, each entry hangs off the entry
  of the record the source named (`Message.SourceParentUUID`, which the inherited reader answers from an
  OMP session's `parentId`), so OMP's own loader walks the source's active path and a branch the
  conversation left stays off it as recorded history. The sidechain, unresolved-parent, selected-leaf
  and projection rules are the Pi route's own code, unchanged (`route_shared.go`, `pi_source_leaf.go`),
  which is the point of sharing the family: OMP is in both provider lists already.

### 2.2 The exact records written

One JSONL file, one object per line, in this order (entry timestamps ISO 8601 as OMP writes them;
message timestamps Unix milliseconds):

| Source evidence | OMP entry |
| --- | --- |
| — (the format's own first line) | `{"type":"title","v":1,"title":"","updatedAt":<session start>,"pad":"<spaces>"}` — exactly 256 bytes including the line feed |
| file header | `{"type":"session","version":3,"id":<fresh uuid v7>,"timestamp":<session start>,"cwd":<source cwd>}` |
| the harness's own configured model (asked) | `{"type":"model_change","id":…,"parentId":null,"timestamp":…,"model":"<provider>/<id>","resolvedModelIsFallback":false}` — omitted when the harness answered none, and no `thinking_level_change` is written (the read answers no thinking level and OMP writes none for a session it did not run) |
| each user message with dialogue | `{"type":"message",…,"message":{"role":"user","content":<text>,"timestamp":<ms>}}` |
| each assistant message with dialogue | `{"type":"message",…,"message":{"role":"assistant","content":[…],"model":<source model>,"provider":<the harness's own configured provider>,"stopReason":"stop","timestamp":<ms>,"usage":{…}}}` — `content` holds a `thinking` block for recorded reasoning, a `text` block for dialogue and one `toolCall` block per call, in that order; `usage` is always present with zero counts (reported as a caveat, as on the Pi route) |
| tool calls | `{"type":"toolCall","id":<recorded id or a generated one>,"name":<source tool name>,"arguments":<recorded input object, compacted>}` inside the assistant message's content |
| tool results | `{"type":"message",…,"message":{"role":"toolResult","toolCallId":…,"toolName":…,"content":[{"type":"text","text":<decoded output>}],"isError":<only when the source's status says it failed>,"timestamp":<ms>}}` |
| a compaction boundary | `{"type":"compaction","id":…,"parentId":…,"timestamp":…,"summary":<the source's summary>,"firstKeptEntryId":<its own id>,"systemMessage":null}` (measured: OMP's loader builds a `compactionSummary` message from exactly this record) |

No `runtime` member is written: OMP's own entries carry `{"version": "<harness version>"}` because the
harness wrote them, and a converted record was written by this build, so the field is left out
(measured optional). Nothing else OMP's writer adds (`custom` reminders, tool-execution records,
`thinking_level_change`, a non-empty title) is invented for a conversation the target did not run.

### 2.3 Mapping rules, category by category

The Pi route's table applies unchanged — the writer is `piSessionBuilder`'s sibling sharing the same
loss vocabulary (`dialogueText`, `whitespace_only_dialogue`, `reasoning_text`, `generated_call_ids`,
`coalesced_tool_call_projections`, `empty_tool_inputs`, `unpaired_tool_results`, `empty_tool_results`,
`whitespace_only_tool_results`, `unrepresentable_result_blocks`, `tool_result_statuses`,
`unresolved_tool_outputs`, `system_records`, `tool_role_messages`, `unclassified_role_messages`,
`empty_assistant_messages`, `empty_user_messages`, `non_dialogue_content`, `sidechain_messages`,
`pre_compaction_history`, `empty_compaction_summary`, `unresolved_source_parents`,
`source_system_records`) with these OMP-specific entries beside them:

| Category | Rule | Why |
| --- | --- | --- |
| The session's own model binding | written from the harness's own answer, counted with a caveat (`target_model_binding`), and omitted with its detail when the harness answered none | OMP restores a session's model from this record, and a converted session whose messages name the source's model would otherwise be refused by the target home |
| The endpoint label on every assistant record | the harness's own configured provider, counted with a caveat (`message_model_attribution`) | measured: OMP's request builder requires a non-empty provider on every assistant record, and the read answers no endpoint |
| An assistant record whose model the read did not answer | the harness's own configured model id, counted in the same caveat | measured: the field must be non-empty for the session to stay resumable |
| The title slot line | written, empty, counted with a caveat (`title_slot_written`) | it is OMP's own initial record and the format's own title behaviour; Pi's files have no such line, which is exactly the difference this route must not lose |

**Format limits** (properties of the pair, not of one run): OMP's session header carries only identity,
timestamp and working directory, so a source's own session-level metadata has no field to travel in (as
on the Pi route); the provider binding a converted record carries is the *target home's* own endpoint
and not the source's — the read answers no endpoint attribution at all — while the model each assistant
record states is the source's own; the target's `custom` records (its reminders, tool-execution and
lifecycle events) have no counterpart in the read and none is invented; attachments and images carry no
bytes through this read; tool names travel as the source's own vocabulary.

## 3. Installation

- Target home: `--target-home DIR`, else `$PI_CODING_AGENT_DIR`, else `~/.omp/agent`. A home that does
  not exist is an error unless the caller named it explicitly (then it is created); call/ccc never
  invents an OMP data directory, and writes nothing into the home except the session file.
- Install path: `<home>/sessions/<project spelling>/<UTC timestamp>_<session id>.jsonl`, where the
  spelling is OMP's own three-branch rule (§2.1). The alias directories OMP's own writer links beside
  the canonical one (`shadowedHomeDirName`, `hashedDirName`) are **not** written: they are lookup
  aliases, and the canonical directory is what the harness's own lookup reads (measured: a session in
  the canonical directory resolves by id).
- Creation is exclusive and never replaces anything, by the mechanism the other installers use: bytes
  to a sibling temporary file (mode 0o600), the final name taken with a hard link; a filesystem without
  hard links is refused (`ErrNoExclusiveInstall`).
- The source-file digest comparison is 0002/0003/0005's, unchanged.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism | Cost | Reported as |
| --- | --- | --- | --- |
| structure | the written file is parsed back through the **inherited AgentsView OMP reader** and compared item by item with what the writer placed in it | offline, always | `verification.structure` |
| `load` | `omp --mode rpc --no-tools --resume=<id>` runs against a **throw-away copy** of the converted bytes in a scratch data directory that defines a credential-free provider (`auth: none`) for the provider+model the file's own records name, with the copy filed under the session's own spelling of its recorded working directory. `get_state` must answer with this session's id, the message count from this session's file and a session file that is the copy; `get_messages` must answer the roles written, in order. The control arm asks about an id nothing installed and must be answered `not found`, and the copy's own bytes must show the loader wrote nothing into the installed file | offline, needs the OMP binary | `verification.load` |
| `resume` | the reported invocation runs **one real turn** from the session's working directory; the exit status, the harness's own answer and the session file's growth (its two sizes) are reported. The byte-level half — that the bytes the file already held are a prefix of what it holds after the turn — is measured by the route's native check instead (`scripts/omp-native-check.py`, `_run_omp_turn`, which reads the file around its own turn and reports it as "only appended"); the CLI's check reports the growth, and keeps no digest of the pre-existing bytes | one model turn | `verification.resume` |

`--verify` defaults to `load`; the exit-code rules are 0002/0003/0005's.

## 5. CLI and machine-readable output

```
ccc continue <provider>:<session-id-or-path> --to omp [--json] [--dry-run] [--target-home DIR]
             [--target-binary PATH] [--data-dir DIR] [--verify none|load|resume] [--resume-prompt TEXT]
ccc continue omp:<session-id-or-path> --to codex|claude|pi …      (the source direction, unchanged)
ccc inspect omp:<session-id-or-path> [--json] [--data-dir DIR]
```

The JSON document keeps the established shape (`upstream_revision`, `source`, `target_session`,
`conversion`, `installation`, `verification`, `warnings`); what changes with the target is
`target_session.format` (`omp-session-jsonl`), `provider` (`omp`), the format-limit list, the reported
`cli_version` (probed, else the writer pin `18.18.7`) and the resume invocations. `--to omp` is
accepted wherever the existing targets are (help text, the refusal message's list, `Targets()`).

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| an OMP session continues in another supported harness, natively verified | `internal/ccc` route tests over the committed OMP fixtures (`--to claude`, `--to codex`, `--to pi`), and `scripts/omp-native-check.py --direction omp-to-claude`: the converted transcript loaded by **Claude Code's own loader** (its `load` half needs no model), quoted in the evidence record |
| a supported source continues in OMP, natively verified | `scripts/omp-native-check.py` (default direction `claude-to-omp`): OMP's own RPC loader answering `get_state`/`get_messages` for the installed session plus the unknown-id control arm, and `--resume-prompt` for one real turn whose answer and session growth are quoted |
| OMP-specific history and installation requirements distinguished from Pi's, with a meaningful regression case | the measured table in §2.1 as the evidence record's §0, and the regression cases: the project-directory spelling's three branches (a test that fails if the OMP installer uses Pi's spelling), the 256-byte title slot line (a test that fails if the converted file's first line is not a title slot of that width), the required per-assistant-record `model`+`provider` pair (a test that fails if either is empty on any assistant record), and the source-side case that an OMP `custom` record on the leaf path is accounted for rather than refused (a test over a fixture that holds one) |
| shared Pi-family logic does not break the accepted Pi routes; no OMP-specific source parser | the existing `internal/ccc`/`cmd/ccc` suites unchanged and passing in the gate, `parity` (no carried file changes), and the increment's own change set: every OMP-specific line is in the new route files plus the family's shared machinery, never in `internal/parser` |
| loss and compatibility warnings actionable; source and unrelated target sessions unchanged; both directions through the ordinary CLI/JSON contract | the native check's before/after digests (source tree, a planted unrelated session, every session the home already held), each run's reported losses with counts, and the CLI's JSON documents for both directions in the evidence record |

The native harness is **not** a gate step (it needs OMP installed and, for a real turn, model access),
registered in `docs/harness.md` next to the other three with the same status. The gate keeps deciding
the offline half.

## 7. Gate, docs, bookkeeping

- Gate: **no new step and no new required step.** New tests live in `internal/ccc` and `cmd/ccc`, which
  `scripts/gate-targets.txt` already builds and runs; the fixtures are pinned by the existing
  `TestFixturesMatchTheirGenerator`. The new native check's own offline cases live in
  `scripts/tests/test_omp_native_check.py`, which the existing `selftest` step discovers: the spelling's
  three branches (the same table `internal/ccc/omp_target_test.go` pins), the seeding's credential policy
  (no host value reaches a written file), the file-reading helpers, the printed invocation's two
  assignments, and the missing-harness `INCOMPLETE` verdict.
- `AGENTS.md`: the route list, the writer/installer/verifier file names, the fixture directory, the new
  native check and the OMP-specific rules (its own project-directory spelling, the title slot, the
  required assistant-record pair, the session model binding being the harness's own answer).
- `docs/conventions.md`: the layout block gains the OMP files; the install rule gains OMP's spelling
  and the reason the printed line names its session-dir variable (the corrected measurement above); the
  fixture note gains the
  OMP tree.
- `docs/harness.md`: the new script's entry and why it is not a step.
- `README.md`: the quickstart's `--to omp` invocation and its resume command.
- `docs/goals/README.md`: 0005 is merged onto `main` as `731dd1d`, so it moves to
  `docs/goals/archive/` with its index row in this increment; 0006 → "Implemented on
  `feat/0006-omp-continuation`; awaiting operator acceptance".
- `docs/decisions/OPEN-QUESTIONS.md`: entries for the defaults this run had to take alone (the title
  slot written empty; the harness-answered provider on every assistant record; the session binding
  written from the harness's answer; the printed resume command naming the home, the working directory and
  the directory holding the session's file; a source whose recorded working directory this host lacks being
  answered by the load check rather than refused; the alias directories not written).
- `docs/upstream.md` and the carry are untouched: no carried file changes.

## 8. Out of scope, with reasons

- **`omp --from-claude` / `--from-codex` as a route**: a picker-driven in-harness import with no
  argument naming the source session, no target directory and no machine-readable result; it cannot be
  driven for the sources that matter here (`pi`, `omp`), and the core design's "converted / installed /
  natively resumable" contract needs an identity this route cannot report. The measurement is recorded
  in §2.1 rather than the option being adopted silently.
- **A second OMP session index, a daemon, or a watcher**: the goal's bounds and the core design forbid
  it; the conversion is a file write and a read.
- **Writing OMP's alias directories** (`hashedDirName`, `shadowedHomeDirName`) or a non-empty title:
  aliases are lookup aids OMP creates when it writes, and a title is the harness's own generation to
  make, not this build's guess.
- **OMP's `custom` records as carried history**: the read does not hold them (the inherited parser
  keeps the message roles and the compaction boundary), and they are the harness's own bookkeeping
  rather than turns; writing invented ones would fabricate history.
- **`ccc formats --json`**: still deferred, as in 0002–0005.
