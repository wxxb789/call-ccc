# Plan — 0005 Continue conversations into and out of Pi

Contract: [`docs/goals/0005-pi-continuation.md`](../goals/0005-pi-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0003 ([`docs/plans/0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md)).

Written before implementation. Every Pi-side fact below was **measured on this host against the
installed Pi CLI (`1.5.2`, the `pi` binary at
`~/.local/share/mise/installs/pi/latest/pi/pi`)**, in throw-away homes, before the writer existed; the
measurements, the commands and the raw answers are repeated in
`docs/evidence/0005-pi-continuation.md`.

## 1. What this increment adds

1. A **target writer** for Pi under `internal/ccc` (`pi_target.go`): a `Conversation` becomes a Pi
   session JSONL file (version 3, tree entries linked by `id`/`parentId`, a fresh native session id),
   plus the fidelity report of what could not be carried.
2. An **installer** (`pi_install.go`): the session is published exclusively into the project directory
   Pi's own discovery derives from the session's working directory
   (`<home>/sessions/--<cwd>--/<timestamp>_<id>.jsonl`), and read back through the **inherited
   AgentsView Pi reader** for the structural comparison.
3. **Native verification** through Pi's own loader (`pi_verify.go`): an offline `load` level that
   starts `pi --mode rpc` against a throw-away copy and reads the harness's own answer to
   `get_state`/`get_messages` (no model turn), with an unknown-id control arm; and a `resume` level
   that runs one real turn through the reported invocation and reports the harness's own answer and
   the session file's growth.
4. **The Pi source direction, unchanged in shape**: `pi:<session-id-or-path> → --to codex|claude` works
   through the same route table, the same reader and the same writers as every other source, because
   the inherited AgentsView Pi reader is already registered. No Pi-specific shortcut is added: the
   common intermediate model is what carries it (pinned by a test and demonstrated natively).
5. A CLI target and source: `ccc continue pi:<session-id-or-path> --to claude|codex …` and
   `ccc continue claude:<id> --to pi …`, with the same help, JSON, exit-code and error conventions.
6. Committed **native-shaped Pi fixtures** (`internal/ccc/testdata/fixtures/pi/…`, written by the
   existing `internal/ccc/fixturegen`) that carry ordered dialogue with a tool call/result pair, a
   branched history and a compacted history; plus a growth fragment
   (`internal/ccc/testdata/growth/pi-complete-turn.jsonl`) so a Pi source can grow between two
   conversions.
7. A repeatable native-verification harness, `scripts/pi-native-check.py`, deliberately **not** a gate
   step (it needs the Pi binary installed and, for a real turn, model access), plus the offline Go
   tests that hold the writer, the installer, the routes and the branch/compaction semantics down.

## 2. The route: a Pi session, verified by Pi's own loader

### 2.1 Why a written session and not an importer

| Observation (measured, `pi 1.5.2`) | Result |
| --- | --- |
| `pi --help` | no import/convert command exists: `install`, `remove`, `update`, `list`, `config`, `auth`, `mcp` are the subcommands. There is **no** native import route to prefer |
| session files on disk | `<agent-dir>/sessions/--<cwd spelling>--/<timestamp>_<session-id>.jsonl`, one JSON object per line; the spelling is `--` + the cwd with its leading separator removed and every `/`, `\` and `:` replaced by `-`, + `--` (`getDefaultSessionDirPath` in the shipped bundle) |
| the agent directory | `PI_CODING_AGENT_DIR` relocates it (default `~/.pi/agent`); two homes never see each other's sessions |
| `--session-dir DIR` | replaces the *sessions* root, and a lookup under it is additionally filtered to sessions whose recorded `cwd` equals the process's own |
| id lookup | `pi --session <id>` resolves by (a) the project directory of the process's cwd, (b) a prefix match over that directory's sessions, (c) a global scan across the sessions root; a session found only in another project is **offered for forking**, not resumed in place |
| hand-written file | a v3 file holding a header, `model_change`, `thinking_level_change`, `user`, `assistant` (with a `toolCall` block), `toolResult` and a second `assistant` entry **loads**: `pi --mode rpc … get_state` answered with that session id and `messageCount: 4`, and `get_messages` with the four roles in order |
| control arm | the same command with an id nothing installed answers `No session found matching '<id>'` and exits 1 |
| unknown-id session behaviour | `pi -p … --session <unknown-id>` does **not** fail: it starts a *new* session with that id. The control arm therefore asserts the missed lookup happens before a new session is written (measured: no new file appeared) |
| real turn | `pi -p --session-dir … --session <id> … "Reply with exactly: PROBE-REPLY-OK"` answered `PROBE-REPLY-OK`, exit 0, and **appended** the harness's own records (its own system message, the user turn, the assistant answer) to the session file |
| source bytes after a turn | the pre-existing records are byte-identical: a turn only appends |
| a session whose records carry source model fields (`provider`, `model`, `thinkingLevel` on message objects) | accepted: the turn ran and answered, and the appended records are the harness's own |
| no leading system message | accepted at load and at resume: Pi contributes its own system prompt at request time (its own `systemMessage` records the prompt and tool loadout) |
| without credentials | `pi -p` answers `No API key found…`/`No models available. Use /login…`; with no credentials in the home and none in the environment, the load path still resolves the session |
| `auth.json` in a fresh home | written empty (`{}`) by the harness itself; nothing needs seeding for the loader |

Consequences taken as this increment's defaults:

- **Write the session file, not a store.** Pi's own session JSONL is what its loader reads, what the
  inherited AgentsView Pi reader parses (so both halves of the evidence are independent
  implementations), and the only artifact a resume picks up.
- **One entry per piece of evidence**, linked by `id`/`parentId` into the source's own tree: the
  `parentId` chain is what Pi's own `buildSessionPath` walks from the last entry, so a file whose chain is
  broken would load but lose everything before the break, and a file whose chain is a single line this
  build invented would present a branch the conversation left as an active turn.
- **The header's `cwd` is the source session's recorded working directory**, and the file is installed
  under the project directory Pi derives from it; a source that recorded none is refused
  (`ErrNoTargetCwd`) rather than installed where no lookup finds it.
- **The printed resume command carries the working directory, the data directory and the sessions tree
  inside it** (`cd <cwd> && PI_CODING_AGENT_DIR=<home> PI_CODING_AGENT_SESSION_DIR=<home>/sessions/<project> pi --session <id> …`,
  and the exec form is `pi -p --session <id> -- <prompt>`, the invocation the resume check runs): the
  first is the only way Pi's own lookup sees a session that belongs to another project, the second
  selects the home it was installed into, and the third names the tree explicitly because Pi reads it
  ahead of the home's default *and* ahead of the home's `settings.json` `sessionDir` — a caller's
  leftover setting, or a settings file naming another directory, would otherwise send the printed line
  looking where the session is not. Print mode is what makes the "non-interactive" line non-interactive:
  `--session <id> -- <prompt>` with terminal stdin and stdout opens the TUI (measured on 1.5.2).
- **A Pi session records no provider binding of its own** (a message may name a provider and a model),
  so `--to pi` never writes the source's provider as the session's own and never invents one: a resumed
  turn uses whatever the caller's Pi configuration offers.
- **The source's per-message *model* is carried verbatim on the assistant entries; no provider is
  written** (Pi's `provider` field means the endpoint that served the words, and the read answers no
  endpoint attribution — it reports the model each source message recorded and a *session-level* provider
  that names the harness; this pass's review measured the earlier revision writing that harness label
  where the fixture records `example-proxy`, so the entry now states the model and the report records no
  provider, Q-0005-08). The session itself binds no model, and the printed resume command names neither a
  provider nor a model, because measured a flagged provider or model stops Pi before it opens the session.
- **Reasoning is carried as the source recorded it**: Pi's `thinking` block *is* its native shape, and
  unlike Claude Code's signed thinking blocks there is no target signature to forge. This narrows the
  loss list relative to the other routes and is reported.
- **A compaction record is written as what the source's boundary carries**: `firstKeptEntryId` = the
  record's own id (the source does not say which entries a compaction kept — it records the summary and
  the boundary — so claiming a kept range would invent one), a null `systemMessage` (Pi patches the
  prompt in at request time), and the pre-boundary entries stay in the file exactly as Pi's own
  compaction leaves them. The target's own loader then rebuilds its model-visible history from the
  summary, which is the source's own semantic.
- **A branch the source recorded is not flattened**: the inherited reader answers each record's own
  identity and the record it hangs off (a Pi `parentId`, a Claude `parentUuid`), and this writer chains
  every entry to the entry of the record the source named — so the target file is the source's own tree,
  the target's own loader walks the source's active path from the file's last entry, and a branch the
  conversation left stays off that path as recorded history. Nothing is synthesised: no `branch_summary`
  is invented for an abandoned path, and no inactive branch is written as an active user turn. What the
  reader marks as a sidechain is omitted and counted (`sidechain_messages`), and a source relation the read
  cannot resolve is counted too (`unresolved_source_parents`). a target that is *not* a tree takes the
  other branch of the same rule: a Codex rollout has no parent relation to hang a record off, and a
  response item written into it becomes model-visible history (measured: the branch fixture converted to a
  rollout read back as `6` messages where the source's own line holds `3`), so that route writes the
  source's own line and omits the records the source placed off it, counted as `inactive_branch_records`
  (Q-0005-09); the same walk's own accounting -- the places it chose that the read did not answer -- is
  counted under the same `unresolved_source_parents` category, because a record kept on the line by a
  relation the read never established must not be presented as the source's own (Q-0005-12). 0005's review
  measured the alternative this rule replaced, for both kinds of target:
  chaining every entry to/writing every record beside the one before it put an abandoned branch's user
  prompt on the active path and hid the source's own line behind a compaction record that stood off it —
  the fixtures, the writer case and the conversion cases pin the source's tree, the loader's own answer
  and the rollout's own line.

### 2.2 The exact records written

One JSONL file, one object per line, in this order (entry timestamps are ISO 8601 like Pi's own;
message timestamps are Unix milliseconds, which is what Pi writes inside `message`):

| Source evidence | Pi entry |
| --- | --- |
| file header | `{"type":"session","version":3,"id":<fresh uuid v7 (Pi's own `createSessionId`)>,"timestamp":<the session's own start, fixed before the entries>,"cwd":<source cwd>}` |
| the source's recorded model | carried per assistant entry (`model`, below); no provider is written (the read answers no endpoint attribution, Q-0005-08) and no session-level `model_change` is written, because a Pi session binds no model and the model that wrote these words is carried where the words are |
| each user message with dialogue | `{"type":"message","id":…,"parentId":…,"timestamp":…,"message":{"role":"user","content":<text>,"timestamp":<ms>}}` |
| each assistant message with dialogue | `{"type":"message",…,"message":{"role":"assistant","content":[{…}],"model":<source model>,"usage":{…},"stopReason":"stop","timestamp":<ms>}}` — `content` holds a `thinking` block for recorded reasoning, a `text` block for dialogue, and one `toolCall` block per call, in that order; `usage` is always present (measured: the loader reads `message.usage.input` for every assistant message it loads) with zero counts and is reported as a caveat |
| tool calls | `{"type":"toolCall","id":<recorded tool-use id or a generated one>,"name":<source tool name>,"arguments":<recorded input object, compacted>}` inside the assistant message's content |
| tool results | `{"type":"message",…,"message":{"role":"toolResult","toolCallId":…,"toolName":…,"content":[{"type":"text","text":<decoded output>}],"isError":<only when the source's own status says the call failed>,"timestamp":<ms>}}` |
| a compaction boundary | `{"type":"compaction","id":…,"parentId":<the entry of the record the boundary's own parent names>,"timestamp":…,"summary":<the source's summary text>,"firstKeptEntryId":<its own id>,"systemMessage":null}` — the kept id is the record's own (Pi's retain-none shape) and the checkpoint is null, because this build has no prompt checkpoint to write and claiming a kept range would invent one |
| a message the source recorded with the tool role | written as the toolResult shape above, paired with its call, and counted (`tool_role_messages`), exactly as the other two writers treat a native tool row |

Every entry carries `id` (8 hex characters, Pi's own shape) and `parentId`: the entry of the record the
*source* named as this record's parent (`Message.SourceParentUUID`, which the inherited reader answers from
a Pi session's `parentId` and a Claude transcript's `parentUuid`), `null` for a record the read answers with
no parent at all — so the file is the source's own tree rather than one chain this build invented, and a
branch the conversation left stands off the path the target's loader walks from the file's last entry, as
recorded history, exactly where the source put it. A record whose named parent has no entry of its own (the
target omitted it) is chained to the place that record's own walk resolved — the nearest ancestor that does
have an entry, the tool result's own entry where the place was resolved through it — and, where that walk
resolved nothing (its recorded parent was read later, with nothing yet written to fall back to), to the
nearest ancestor the read's own relations reach and this conversion wrote; a record that follows a tool
result is chained to the result **where the read may answer a projection**, because the reader answers a Pi
tool result as *not* a visible ancestor while the source's own file runs call → result → what follows, and
writing the result after the record would leave a resumed session with a call and no result. The step is
taken only where the read may project: it exists to undo a projection, and an exact answer (a Claude
transcript's `parentUuid`) names the record it means, so a result standing beside a record the read chains
to the same message is that record's *sibling* — chaining through it would chain the record to a place the
source placed it elsewhere and revive the sibling's own words as an ancestor (Q-0005-12). Where it is
taken it is counted, whether or not the result stands immediately before the record: the reader answers a
record under a tool result as hanging off the message that made the call, so whether the record hung off
the call or off a result of it is not answerable from the read, and the entry is chained through the
nearest result of that call written before it (`unresolved_source_parents`) — the reading the source's own
file order (call → result → what follows) supports. The step keys on the record a record *hanging off* the
outcome would answer as its parent (`toolResultCarrierRecord`): the message's own identity where the read
folded the outcome onto the call's own message — the archive projection's shape, and a message carrying a
call at all — and the result record's read-answered parent otherwise, so a sibling of the call naming that
parent is not chained through the abandoned call's result (Q-0005-13). A relation the read cannot resolve
at all follows the entry before it and is counted (`unresolved_source_parents`) instead of being presented
as the source's.
The file ends with a line feed, and no entry is written after a
message that was omitted, so the chain never points at something the file does not hold. **The selected
leaf is part of this rule**: the target's own loader walks from the last entry written and the source's from
the read's last record, so a source whose last record writes no entry while the entry written last stands
for a record the read's last record does not reach is refused (`ErrUnrepresentableLeaf`) rather than written
as a file that resumes a branch the source's conversation left. The source side of that rule is the
source *file*'s own answer rather than the read's last record — the carried Pi reader skips tree entries the
harness's own loader builds context from, so a file that switched branches with `/tree` can end above a
record the read does not answer — and the source-side check, the fixture and the measured harness answer are
in the evidence record's sixth review round (Q-0005-16).

### 2.3 Mapping rules, category by category

| Category | Rule | Why |
| --- | --- | --- |
| Dialogue text | `dialogueText(message)` — the source's own dialogue projection when it made one, else `Content` for a message with no calls | the reader's `Content` renders calls into text (`[Read: <path>]`); writing it beside the blocks would duplicate the call |
| Dialogue that holds only whitespace | the whitespace **omitted**, counted (`whitespace_only_dialogue`); a record that also holds a reasoning trace keeps the trace (its own `thinking` block, with the `reasoning_text` caveat) | the inherited Pi reader drops a record left with no text and no tool evidence, so a written *dialogue* would be in the file and absent from every read of it — but the trace is not dialogue and Pi's `thinking` block is its own shape, so an assistant record holding both keeps what the reader does read |
| Reasoning (`Message.Thinking`) | **carried** as the message's `thinking` block, counted with a caveat (`reasoning_text`) | Pi's `thinking` block is the format's own shape for a persisted reasoning trace; the source's trace is what the source recorded, and this read holds no signature to forge |
| Tool calls | Pi's `toolCall` block with the recorded name, arguments and id | recorded evidence; the target's own tool vocabulary is unaffected and a resumed turn reads the call as history |
| A call with no recorded id | a generated `ccc<message>_<index>` id, counted (`generated_call_ids`) | Pi pairs a call and its result by id |
| A call whose id repeats inside one message | written once, the further projections counted (`coalesced_tool_call_projections`) | the inherited Codex reader projects one `apply_patch` into one call per file; Pi's block carries one id and one input |
| An empty or non-object call input | `{}`, counted (`empty_tool_inputs`) | Pi's block carries an arguments object |
| Tool results | written under the call's id with the output text `resultText` decodes; a result whose call this conversion did not write is **omitted** and counted (`unpaired_tool_results`) | pairing is by id on both sides; inventing a call would fabricate an invocation |
| An empty / whitespace-only / block-only tool result | **omitted**, counted (`empty_tool_results`, `whitespace_only_tool_results`, `unrepresentable_result_blocks`) | the reader's text channel carries the block text and nothing else |
| A result the source recorded as a failure | `isError: true`; any other recorded status is carried as text and counted (`tool_result_statuses`) | Pi's field is a boolean |
| A result whose persisted output the provider could not read | carried as its placeholder, counted with a caveat (`unresolved_tool_outputs`) | the placeholder is the only text the read holds; it is not fabricated output |
| `IsSystem` records that are not boundaries | text **omitted**, counted (`system_records`); their tool evidence still carried | Pi's system entries are its own prompt and tool loadout; another provider's bookkeeping is not that |
| Native `tool` role messages | written as the paired result, counted (`tool_role_messages`) | the tool's words belong in a result, never in a user or assistant turn |
| Roles this build does not classify | **omitted**, counted (`unclassified_role_messages`) | writing one as user or assistant would state a role the source never recorded |
| An assistant message with neither dialogue, reasoning nor tool evidence | **omitted**, counted (`empty_assistant_messages`) | Pi has no record for an empty message |
| A user message with neither dialogue nor tool evidence | **omitted**, counted (`empty_user_messages` or `non_dialogue_content`) | as above, and a rendered non-dialogue payload must not be put in the user's mouth |
| `IsSidechain` messages | **omitted**, counted (`sidechain_messages`) | a subagent's turn in the same file; Pi's own loader keeps them out of the session's context, and writing one as an ordinary turn would state the conversation's agent said it |
| The model the source recorded on a message | **carried** (`model`, with no provider written), counted with a caveat (`message_model_attribution`) | Pi's field means the model that produced these words, which is what the source recorded; the endpoint is not answered by the read (Q-0005-08) |
| Pre-boundary history | **carried**, counted (`pre_compaction_history`) | the entries stay in the file as Pi's own compaction leaves them; the compaction record is what makes Pi rebuild context from the summary |
| A compaction boundary whose summary holds no text | the record is still written, counted (`empty_compaction_summary`) | Pi requires a summary field; an empty one is the honest record of a boundary whose text the source did not hold |
| Lines the source reader could not parse / a summary-fallback read / a read the provider says needs a retry | reported from the read's own fidelity (`malformed_source_lines`, `summary_transcript`, `source_needs_retry`) | identical to the other writers, because it is the read's own property |

Every category is listed on every run with its count (zero included) and its disposition, exactly as
the Codex and Claude writers do.

**Format limits** (properties of the pair, not of one run): Pi's session header carries only identity,
timestamp and working directory, so a source's own session-level metadata (a Codex `session_meta`'s
originator/cli version/source kind, a Claude transcript's version field) has no field to travel in;
provider bindings on the *session* (Codex's `model_provider`, whose loader refuses a mismatch) have no
counterpart — a Pi session records no provider for itself, so a conversion records none and a resumed
turn takes whichever model the caller's configuration selects; turn boundaries
(Codex's `turn_context`) have no Pi record; attachments and images carry no bytes through this read;
tool names travel as the source's own vocabulary.

## 3. Installation

- Target home: `--target-home DIR`, else `$PI_CODING_AGENT_DIR`, else `~/.pi/agent`. A home that does
  not exist is an error unless the caller named it explicitly (then it is created); call/ccc never
  invents a Pi data directory, and writes nothing into the home except the session file.
- Install path: `<home>/sessions/--<cwd spelling>--/<UTC timestamp>_<session id>.jsonl`, the layout
  `getDefaultSessionDirPath` computes and the one Pi's own `_persist` writes.
- Creation is exclusive and never replaces anything, by the mechanism the Codex and Claude installers
  use: bytes to a sibling temporary file (mode 0o600), the final name taken with a hard link; a
  filesystem without hard links is refused (`ErrNoExclusiveInstall`) rather than published by a rename.
- The source-file digest comparison is 0002/0003's, unchanged.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism | Cost | Reported as |
| --- | --- | --- | --- |
| structure | the written file is parsed back through the **inherited AgentsView Pi reader** and compared item by item with what the writer placed in it (dialogue order and text, call/result pairing, the compaction record and its summary) | offline, always | `verification.structure` |
| `load` | `pi --mode rpc` runs against a **throw-away copy** of the converted bytes, in a scratch agent directory whose sessions tree holds that copy under its own working directory, with the environment variables that could reach a model removed. The check asks the harness's own loader `get_state` (which must answer with this session's id and the recorded message count from this session's file) and `get_messages` (whose roles must be the ones written, in order), and reads `get_session_stats` for Pi's own view of the conversation's shape. A control arm asks the same question about an id nothing installed, which must answer `No session found matching` | offline, needs the Pi binary | `verification.load` |
| `resume` | the reported invocation runs **one real turn** from the session's working directory through the session dir the session was installed under; the exit status, the harness's own answer and the session file's growth are reported, and the file's pre-existing bytes are re-digested to show the turn only appended | one model turn | `verification.resume` |

`--verify` defaults to `load`; the exit-code rules are 0002/0003's. The load check never writes into the
installed session (it drives a copy), and the resume check reports the growth it measured.

## 5. CLI and machine-readable output

```
ccc continue <provider>:<session-id-or-path> --to pi [--json] [--dry-run] [--target-home DIR]
             [--target-binary PATH] [--data-dir DIR] [--verify none|load|resume] [--resume-prompt TEXT]
ccc continue pi:<session-id-or-path> --to codex|claude …      (the source direction, unchanged shape)
ccc inspect pi:<session-id-or-path> [--json] [--data-dir DIR]
```

The JSON document keeps the established shape (`upstream_revision`, `source`, `target_session`,
`conversion`, `installation`, `verification`, `warnings`); what changes with the target is
`target_session.format` (`pi-session-jsonl`), `provider` (`pi`), the format-limit list, the reported
`cli_version` (probed, else the writer pin `1.5.2`) and the resume invocations, which name the home and
the working directory. `--to pi` is accepted wherever the existing targets are (help text, the refusal
message's list, `Targets()`).

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| a representative Claude-origin conversation converts to a new Pi-native session and **is resumed by Pi** (native load/resume, not a parser round-trip) | `scripts/pi-native-check.py` on this host: the `load` half's own report (Pi's `get_state`/`get_messages` answers naming the installed session, plus the control arm), and `--resume-prompt` for one real turn whose answer and session growth are quoted; the raw transcript of the run in the evidence record |
| a representative Pi-origin conversation continues in a previously supported target harness, with native load/resume evidence, demonstrating the common intermediate model | `scripts/pi-native-check.py --direction pi-to-claude` (Pi fixture → `--to claude`), the same native check 0003 established, plus the converter's own JSON report naming the inherited Pi reader as the source of the read and the Claude reader as the structural check's reader; `internal/ccc` route tests pin that no Pi-specific branch exists in the pipeline |
| tests/fixtures show ordered dialogue and tool-call/result relationships stay coherent, and a branched or compacted Pi history is not flattened into an invented active conversation; material loss explicit | the committed Pi fixtures and the tests over them: parse identity and order, round-trip through the writer and the installed file, call/result pairing, the compaction record's own semantics (`firstKeptEntryId` = its own id, summary text carried, pre-boundary entries still present), the branch fixture's own tree walk (a message entry off it, and each off-path entry reaching the path through its parents), the writer case that pins the source relation (branch, tool result, omitted record, unresolvable parent), the conversion case that requires the target file's model-visible walk to equal the source file's, and the `losses[]` block with counts for the branched/compacted fixture in the evidence record |
| the source and existing target sessions remain intact; accepted Claude/Codex behaviour not regressed; inherited AgentsView code unchanged | the native check's before/after digests (source fixture, a planted unrelated session, every session the home already held), the existing `internal/ccc` and `cmd/ccc` suites in the gate's `test` step, and `parity` (no carried file changes) |
| the CLI exposes both directions with the same help, JSON result and error conventions | `ccc help`/`ccc continue -h` text, the JSON documents of both directions in the evidence record, `cmd/ccc` tests for the new target and for the refusal list, and the exit codes reported beside each run |

The native harness is **not** a gate step (it needs Pi installed and, for a real turn, model access),
registered in `docs/harness.md` next to the other two with the same status. The gate keeps deciding the
offline half.

## 7. Gate, docs, bookkeeping

- Gate: **no new step and no new required step.** New tests live in `internal/ccc` and `cmd/ccc`, which
  `scripts/gate-targets.txt` already builds and runs; the fixtures are pinned by the existing
  `TestFixturesMatchTheirGenerator`.
- `AGENTS.md`: the layout line (the Pi writer/installer/verifier, its fixture directory,
  `scripts/pi-native-check.py`), the read order stays as it is, and the rule that a Pi session's
  discoverability comes from its recorded working directory is stated where the Claude one is.
- `docs/conventions.md`: the layout block gains the Pi files; the rule about a target's own loader
  deciding discoverability is extended with Pi's project-directory spelling; the fixture note gains the
  Pi tree.
- `docs/harness.md`: the new script's entry and why it is not a step.
- `README.md`: the quickstart's `--to pi` invocation and its resume command.
- `docs/goals/README.md`: 0005 → "Implemented on `feat/0005-pi-continuation`; awaiting operator
  acceptance". 0004 is *implemented on its branch, not merged*, so it stays where it is.
- `docs/decisions/OPEN-QUESTIONS.md`: entries for the defaults this run had to take alone (the rpc-based
  offline load oracle and its control arm; the carried reasoning trace; `firstKeptEntryId` = own id;
  the branch/sidechain rule (and Q-0005-09 for the one-line target); the model field carried and the
  provider the read cannot answer (Q-0005-08); the resume command's working directory;
  the fixture's invented home path).
- `docs/upstream.md` and the carry are untouched: no carried file changes.

## 8. Out of scope, with reasons

- **A second Pi session index, a daemon, or a watcher**: the goal's bounds and the core design forbid
  it; the conversion is a file write and a read.
- **`ccc formats --json`**: still deferred, as in 0002/0003; the JSON document already distinguishes the
  three states per route.
- **Pi's OMP-family targets** (Oh My Pi, OMO, StepCode, Prime Agent): goal 0006 owns OMP; the readers are
  already inherited, and this increment's writer states the format family it writes (Pi's own session
  JSONL) without claiming the others' resume semantics.
- **Pi extensions, MCP configuration, skills, prompt templates, auth**: a conversion writes one session
  file; it never writes `settings.json`, `models.json`, `auth.json` or an extension.
- **Following `parentSession`/`branchedFrom` chains**: the goal says a branched history must not be
  flattened, not that the parent session must be read; the selected session is the unit.
- **Interactive resume and the session picker**: the resume command is printed for the caller.

## 9. Ambiguities, and the reversible defaults taken

1. **Where the session is installed.** Options: the project-grouped default layout (found by
   `--session <id>` from the session's own directory), or a flat `--session-dir` root (found only when
   the process's cwd equals the session's recorded cwd and the file sits directly in that root).
   Default: the project-grouped layout, because it is the one Pi's own writes produce and the only one
   the printed `cd <cwd> && PI_CODING_AGENT_DIR=<home> PI_CODING_AGENT_SESSION_DIR=<home>/sessions/<project> pi --session <id>`
   resolves whatever the caller's shell or the home's `settings.json` says about a session directory.
   Reversal: install into a flat root and print `--session-dir`.
2. **A source with no recorded working directory.** Default: refuse (`ErrNoTargetCwd`), as 0003 does,
   because Pi's lookup is project-scoped. Reversal: install anyway and document the flag needed.
3. **Reasoning.** Default: carry it (Pi's native `thinking` block) and disclose. Reversal: omit it,
   as the Claude route must.
4. **`firstKeptEntryId` and `systemMessage` on a carried compaction.** Default: the record's own id
   (no kept range claimed) and no system message (Pi installs its own prompt at request time).
   Reversal: reconstruct a checkpoint, which would mean inventing a prompt.
5. **Inactive branches.** Default: never synthesise them, and reproduce the source's own relation instead
   — on the tree-shaped targets every record hangs off the record the source named, so an abandoned branch
   is off the target's active path as recorded history (the source's own semantic, and what its loader
   shows), and a sidechain is omitted and counted. A target whose format is one line of records takes the
   reversal for its own shape: the Codex rollout writes the source's own line and counts the records the
   source placed off it (`inactive_branch_records`), because a response item written there is
   model-visible history (Q-0005-09), and its walk's own accounting of the places it chose travels as the
   same `unresolved_source_parents` category the tree writers report (Q-0005-12). The remaining
   reversal — write a `branch_summary` entry — would
   require inventing a summary the source did not record.
6. **The resume command's model flags.** Default: print neither a provider nor a model. The source's
   provider is a source-side label Pi's own configuration need not define, and a flagged model can leave
   the target unable to open the session at all; the model attribution travels on the entries instead and
   is reported as a caveat. Reversal: print the source's model (measured to fail on this host with
   `ambiguous across providers … No matching provider is authenticated`).
