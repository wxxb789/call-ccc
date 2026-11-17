# Evidence — 0003 Continue a Codex session in Claude Code

Contract: [`docs/goals/0003-codex-to-claude-continuation.md`](../goals/0003-codex-to-claude-continuation.md).
Plan: [`docs/plans/0003-codex-to-claude-continuation.md`](../plans/0003-codex-to-claude-continuation.md).
Increment 0002's evidence: [`0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md).

Every command below was run on this host, in this tree, with the exit code and the output it printed.
Nothing here is re-typed from memory; where a number appears it was counted by the run that names it.

Host facts this increment was measured against:

| Fact | Value |
| --- | --- |
| Claude Code CLI | `2.1.326 (Claude Code)`, npm `@anthropic-ai/claude-code@2.1.326` |
| where it is installed here | `npm install --prefix ~/.local/share/ccc-native/claude-cli @anthropic-ai/claude-code@2.1.326`; the executable is `<prefix>/node_modules/.bin/claude` (not on `PATH`, so every run below names it with `--target-binary`) |
| model access used by the `resume` level | the host's own provider configuration, pointed at an OpenAI-compatible LiteLLM proxy (`ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_MODEL=deepseek-v4.1-flash-fast`); no credential was copied into any isolated home |
| data directories | every run used an isolated `--target-home`; no developer data directory was read or written |

## 0. What the target format was measured to be, before any writer code existed

The measurements below are the format's contract for this increment. Each was taken with a hand-written
or captured transcript in an isolated `CLAUDE_CONFIG_DIR`, driven through the installed binary, and each
is recorded with the observation it produced.

| Measurement | Result |
| --- | --- |
| `claude import codex --dry-run` in an isolated home | `claude import` is not yet available in this build. Run `claude` and use /mcp or edit ~/.claude/settings.json directly. — there is **no** native import route to prefer |
| session layout | `<config dir>/projects/<project-dir>/<session-id>.jsonl`; `<session-id>` is the file stem |
| project-directory spelling | each character outside `[A-Za-z0-9]` becomes `-`; a real session started in `/tmp/probe3/WS.Upper Case_1+plus` was filed under `-tmp-probe3-WS-Upper-Case-1-plus` (case preserved) |
| discovery | `claude -p --resume <id>` finds a session through the process's **working directory**: run from any directory whose project-dir spelling differs it answered `No conversation found with session ID: <id>` and exited 1 (measured from a sibling directory, and with the file placed under a different project dir) |
| the envelope a file needs | a hand-written transcript with only `uuid`, `parentUuid`, `type`, `timestamp`, `cwd`, `sessionId`, `version` and `message` loaded and continued a turn (answered `PLUMBBOB`, a fact recorded only in the converted history); removing `version`, `userType`, `gitBranch`, `permissionMode`, `promptId`, `toolUseResult`, `sourceToolAssistantUUID`, `usage` or `model` changed nothing |
| the parent chain | the loader walks `parentUuid` from the last record: a file whose records carry no links produced `Warning: Resume transcript (session …) has 2 user/assistant records but none carry parentUuid links; only 1 reached the resumed conversation. Conversation reconstruction walks parentUuid from the last record, so unlinked records are dropped …` |
| the loader's own request | a logging loopback proxy in front of the model endpoint recorded the messages the loader built: the whole converted history, one API message per record, assistant records sharing a `message.id` merged into one, tool blocks passed through verbatim |
| `isSidechain: true` records | **dropped** from the model-visible history (a subagent's token never reached the model) |
| `tool_result.is_error: true` | passed through verbatim (the error text answered a question) |
| a compaction boundary | `{"type":"system","subtype":"compact_boundary"}` **plus** `{"type":"user","isCompactSummary":true,…}` truncates the model-visible history to the boundary — the loader's request began with the summary record and a pre-boundary fact answered `NOT-IN-CONTEXT`; the `isCompactSummary` flag **alone**, without the marker record, changed nothing |
| a resume turn | appends the harness's own records to the session file (measured 1 005 → 135 309 bytes for one turn: `api-request`, `api-request-blob`, `cost-state`, `last-prompt`, …), none of which the writer fabricates |
| a credential-less run | `claude -p --output-format json --resume <id>` with no model credentials reachable still **resolves** the session: it printed a result document naming that session (`"terminal_reason": "api_error"`, `"result": "Not logged in · Please run /login"`) and exited 1, while the same command with an unknown id printed `No conversation found with session ID: <id>` on stderr and no document at all. This is the offline `load` oracle; `--bare` makes the no-credential condition the harness's own contract rather than a property of the environment |

## 1. One documented, non-interactive command, with the native identity and the resume command

The command converts the committed Codex fixture into an isolated Claude Code home:

```console
$ CODEX_SESSIONS_DIR=$PWD/internal/ccc/testdata/fixtures/codex \
  ./bin/ccc continue codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 --to claude \
  --target-home /tmp/smoke2/home \
  --target-binary /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude
codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 → claude
  source:       /home/azureuser/repos/call-ccc/internal/ccc/testdata/fixtures/codex/2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
  read from:    source
  conversation: 6 message(s), 2 user, 4 assistant, 1 tool call(s), 1 compaction boundary(ies)
  converted:    claude-code-transcript-jsonl, 3314 bytes: 4 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  omitted:      message_model_attribution ×5
  carried-with-caveat: pre_compaction_history ×2
  installed:    /tmp/smoke2/home/projects/-tmp-smoke2-ws/df9d4f48-baa7-4b6b-ac53-85791680941d.jsonl [hard-link]
  source file:  unchanged (sha256 605998b9b8f367c2fe09ae208b2e8c012bfec08b115a9c3cb2acac0f7d1452b8, taken before and after the conversion)
  target id:    df9d4f48-baa7-4b6b-ac53-85791680941d
  working dir:  /tmp/smoke2/ws
  resume:       cd /tmp/smoke2/ws && CLAUDE_CONFIG_DIR=/tmp/smoke2/home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume df9d4f48-baa7-4b6b-ac53-85791680941d
  non-interactive resume: cd /tmp/smoke2/ws && CLAUDE_CONFIG_DIR=/tmp/smoke2/home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude -p --output-format json --resume df9d4f48-baa7-4b6b-ac53-85791680941d '<prompt>'
  cli_version:  2.1.326 (probe)
  structure:    verified (the inherited AgentsView Claude reader over the installed session)
  native load:  verified (the target harness's own loader (claude --bare --resume, with no model credentials in its environment, plus an unknown-id control))
  native resume: not-requested (…)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

(exit code 0. The `cd /tmp/smoke2/ws` in the command is the fixture's recorded working directory after
the run's own copy relocated it; the committed fixture records `/home/user/code/example-api`, and
§3's native-check run shows the committed one.)

`ccc help` and `ccc continue -h` state the invocation, both targets and the meaning of every flag;
`cmd/ccc/claude_route_test.go` pins the line, the identity and the resume spelling, and
`cmd/ccc/main_test.go`'s help case pins both quickstart lines.

## 2. Conversion, installation and native verification are three separate, machine-readable states

`--json` writes one object. The three states of the run above, quoted from its own output (the same run
with `--json`):

```json
{
  "conversion": {"state": "converted", "format": "claude-code-transcript-jsonl", "read_from": "source",
                 "carried": {"messages": 4, "user_messages": 2, "assistant_messages": 2,
                             "tool_calls": 1, "tool_results": 1, "compaction_boundaries": 1,
                             "turns": 0, "bytes": 3314}},
  "installation": {"state": "installed", "mechanism": "hard-link", "created": true,
                   "source_check_state": "unchanged"},
  "verification": {"structure": {"state": "verified"},
                   "load": {"state": "verified"},
                   "resume": {"state": "not-requested"}}
}
```

Exit codes are 0002's, unchanged: a conversion or installation failure, and a *requested* verification
that ran and failed, exit 1; a verification that could not run (the binary is absent, a dry run installed
nothing) is reported as not-verified and does not fail the run; `--verify resume` requires
`--resume-prompt`, because the turn becomes part of the caller's session. The loss report lists every
category it knows on every run, count and disposition included; the three categories above are the ones
this fixture produces (`message_model_attribution ×5`, `pre_compaction_history ×2`,
`tool_name_vocabulary ×1`).

## 3. Claude Code's own loader recognizes the installed session and accepts a continuing turn

Three independent observations, in increasing strength.

**(a) The loader's own request, recorded by a loopback proxy.** The converted session was resumed with
the model endpoint replaced by a logging proxy; the request the loader built from the installed file is
the converted history itself:

```text
user       "Compacted: the retry cap question was answered from internal/retry/config.go.\n" + "And the attempt limit?"
assistant  "Four attempts, in internal/retry/policy.go."
user       "<system-reminder>Attribution for git commits …", "<the new prompt>"
system     "# Environment …"
```

The pre-boundary turns (the tool call `shell`, its output `internal/retry/config.go:41: maxBackoffSeconds
= 30`, the first answer) are absent — the target applied its own compaction boundary to its model-visible
history — and the tool pair is present in the file and in the loader's read of the post-boundary part.

**(b) The `load` level, offline.** From the run above, `verification.load`:

```text
the harness's loader resolved the installed session and answered about it (subtype "success", terminal
reason "api_error", result "Not logged in · Please run /login"); the same command with an unknown session
id answered "No conversation found with session ID: 6d7d7656-0fb6-472a-9c9f-56b679d7bfc6". The check ran
against a throw-away copy of the converted bytes (/tmp/ccc-claude-load-…/projects/-tmp-smoke2-ws/df9d4f48-….jsonl)
with --bare and with ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL, ANTHROPIC_MODEL,
ANTHROPIC_DEFAULT_HAIKU_MODEL, ANTHROPIC_DEFAULT_OPUS_MODEL, ANTHROPIC_DEFAULT_SONNET_MODEL,
ANTHROPIC_SMALL_FAST_MODEL, CLAUDE_CODE_OAUTH_TOKEN, CLAUDE_CODE_USE_BEDROCK, CLAUDE_CODE_USE_VERTEX
removed from the environment, so the loader read the session without a model turn, and the installed file
is unchanged (605998b9b8f3…)
```

**(c) One real turn, from the reported invocation.** With the host's model access and
`--verify resume --resume-prompt "Summarise in one sentence what this conversation records about retry
attempts and where that is set."`:

```text
native resume: verified (one turn in the installed session through the target harness's non-interactive resume (claude -p --resume))
               the harness answered from the installed session in one turn and wrote its own records into it (the session file went from 3314 to 143138 bytes)
```

the turn's own output (the target's answer, as the harness printed it):
    One sentence: this conversation records that the retry cap is set in `internal/retry/config.go` and that the attempt limit is **four attempts**, set in `internal/retry/policy.go`.
    One caveat worth flagging: I just checked, and the working directory is empty — there is no `internal/retry/` directory, no `policy.go`, and no `config.go` anywhere in the workspace. Both answers came from my own earlier turn…

The same session asked about the *pre*-boundary fact (`What value does maxBackoffSeconds have, and which
file defines it? If that is not in your context, reply exactly NOT-IN-CONTEXT.`) answered
`NOT-IN-CONTEXT`: the boundary the writer carries is the target's own semantic, not a decoration.

**(d) The native-check script** (`scripts/claude-native-check.py`, §6) re-derives (a)–(c) end to end
against the real binary and prints each check's own verdict; its run is recorded in §6.

## 4. The source is unchanged, the identity is new, no other session moves

- `installation.source_check_state` is `unchanged` with the digest taken before and after the conversion
  (`sha256 605998b9…`, §1); the check compares the source file's own bytes, and a source that moved
  while it was converted is reported as `changed`.
- The identity is a fresh UUID v4 that names the file and every record's `sessionId`; `--target-home` is
  explicit in every run here, and a target home nobody named is refused rather than invented
  (`ErrTargetHomeMissing`).
- The install is exclusive: bytes to a sibling temporary file (mode 0600), the final name taken with a
  hard link; a taken name fails with `ErrTargetExists` and a filesystem without hard links is refused
  (`ErrNoExclusiveInstall`) rather than published by a rename. `claude_install_test.go` pins all three,
  including the mode on a host that reports POSIX bits and the request itself on one that does not.
- The native check plants an unrelated Claude session in the target home and hashes it, the source copy
  and every session the home already held **last** — after the loader ran and the turn happened — so a
  mutation during either is answered by the comparison rather than by an earlier verdict (§6).

## 5. A representative transcript keeps its supported semantics; the rest is reported

Source: the committed Codex fixture (`codex:0c8e5f21-…`, 6 messages, one tool call/result, one
compaction boundary). Target: the transcript §1 installed. The target column is what the **inherited
AgentsView Claude reader** made of the installed file (`ccc inspect claude:<id> --json` in a second
process, §3's `structure` check), and the answer column is the recorded run's own words (§3(c)).

| Source evidence | Target side | Reported as |
| --- | --- | --- |
| user turn `Where is the retry cap configured?` | carried as a `user` record; the read-back shows it as the session's first user message | carried (`messages ×4`) |
| assistant turn `It is set in internal/retry/config.go.` + `shell` call `{"command":"grep -rn maxBackoffSeconds internal/retry"}` | one assistant record holding the text and one holding the call, sharing a `message.id`; the read-back pairs them | carried (`tool_calls ×1`) |
| `shell` output `internal/retry/config.go:41: maxBackoffSeconds = 30` | a `user` record holding a `tool_result` block under the call's id; the read-back shows the same text | carried (`tool_results ×1`) |
| `compacted` record (`Compacted: the retry cap question was answered from internal/retry/config.go.`) | the target's own boundary pair (`system/subtype:"compact_boundary"` + the `isCompactSummary` summary record); the read-back reports `compact_boundaries ×1`, and the resumed turn could not read a pre-boundary fact (recorded in §3(c)) | carried (`compaction_boundaries ×1`), with the two pre-boundary turns counted under `pre_compaction_history` |
| assistant turn `And the attempt limit?` / `Four attempts, in internal/retry/policy.go.` | carried as ordinary turns after the boundary | carried; the resumed turn's own answer says `**four attempts**`, set in `internal/retry/policy.go` (recorded in §3(c)) |
| per-message model (`gpt-5.2-codex` on the source's turns) | **not written**: the target's `message.model` states which model produced the words | omitted (`message_model_attribution ×5`) |
| session metadata / `turn_context` records | no Claude record can hold them | format limits (`session_metadata_record`, `turn_boundaries`) |
| tool names (`shell`) | carried verbatim; the target's own tool set is unaffected | carried-with-caveat (`tool_name_vocabulary ×1`) |

The same table's rules are pinned by `internal/ccc/claude_target_test.go`
(`TestConvertToClaudeTranscriptWritesTheMeasuredShape`, `TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers`,
`TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall`,
`TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory`), by
`internal/ccc/claude_install_test.go` (the round trip, the round trip's detection of evidence the file
does not hold), and by `internal/ccc/continue_claude_test.go` (the route's counts and losses).

## 6. Tests, the native check and the gate

### 6.1 The native check, run end to end against the real harness

```console
$ ANTHROPIC_BASE_URL=… ANTHROPIC_AUTH_TOKEN=… ANTHROPIC_MODEL=deepseek-v4.1-flash-fast \
  ANTHROPIC_SMALL_FAST_MODEL=deepseek-v4.1-flash-fast CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 \
  python3 scripts/claude-native-check.py \
    --claude /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude \
    --resume-prompt "Summarise in one sentence what this conversation records about retry attempts and where that is set." \
    --expect "internal/retry/policy.go"
```

exit code **0**, 14 checks passed, 0 failed, 0 skipped. The run's own lines (abridged only where a path
is repeated; nothing else is paraphrased):

```text
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and a non-zero loss category — carried 4 message(s), 2 user message(s), 2 assistant message(s), 1 tool call(s), 1 tool result(s), 1 compaction boundary(ies), 3674 byte(s); non-zero loss categories: tool_name_vocabulary=1, message_model_attribution=5, pre_compaction_history=2
PASS: the session is installed under the project directory its working directory spells to — projects/-tmp-ccc-claude-native-check-kbd9e019-workspace-example-api/6038ac68-1841-45a6-a4af-80e81b714def.jsonl [hard-link]
PASS: the installed session is readable and writable only by its owner — mode 0600
PASS: the target identity is new and is not the source's — 6038ac68-1841-45a6-a4af-80e81b714def, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 3674 bytes (the target's own read of it is the load check)
PASS: the structural read-back of the installed file is verified — verified — the inherited AgentsView Claude reader over the installed session
PASS: the inherited reader reads the installed file back as what was written — ccc inspect claude:6038ac68-…: {"messages": 7, "user_messages": 3, "assistant_messages": 4, "system_messages": 0, "tool_messages": 0, "system_records": 1, "sidechain_messages": 0, "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1}; the conversion's own structure read-back: {"session_id": "6038ac68-…", "messages": 7, "user_messages": 3, "assistant_messages": 4, "tool_calls": 1, "tool_results": 1, "compaction_boundaries": 1}; carried: 1 tool call(s), 1 tool result(s), 1 compaction boundary(ies), 4 message(s)
PASS: the reported resume command names the binary, the session, the working directory and the data directory — resume_command='cd …/workspace/example-api && CLAUDE_CONFIG_DIR=…/claude-home …/claude --resume 6038ac68-…' exec_resume_command="…" environment={'CLAUDE_CONFIG_DIR': '…/claude-home'}
PASS: the target harness's own loader resolves the installed session — verified — …the same command with an unknown session id answered "No conversation found with session ID: 5932456c-36f1-442d-b187-e1468d3c46d4". The check ran against a throw-away copy of the converted bytes … with --bare and with ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL, ANTHROPIC_MODEL, … removed …
PASS: a real turn continues from the installed session — exit 0, session 3674 -> 158422 bytes; ran: cd …/workspace/example-api && CLAUDE_CONFIG_DIR=…/claude-home …/claude -p --output-format json --resume 6038ac68-… 'Summarise in one sentence …'; the answer holds 'internal/retry/policy.go', and that comparison decides this check
PASS: the source session is unchanged — sha256 0f737632e82f1382… (the scratch source root is identical)
PASS: the unrelated session in the target home is unchanged — sha256 5acb5987899c9b3d…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
14 check(s) passed, 0 failed, 0 skipped
```

the turn's own output (the target's answer, as the harness printed it):
    The conversation records a single claim about the attempt limit — that it is four attempts, set in `internal/retry/policy.go` (alongside a retry cap in `internal/retry/config.go`) — but I just checked and `/tmp/ccc-claude-native-check-kbd9e019/workspace/example-api/` is completely empty, so neither file exists and that answer was not grounded in anything in this workspace.

The run's header states the one mechanical edit it makes and why (the fixture's recorded working
directory is rewritten into a scratch directory that exists and spells to the project directory the
target's loader must be started in), that the load level is read from the conversion run's own report,
and that the probe and its control run in the same conversion; `--keep` was left off, so the scratch was
removed and the hashes above are the run's last act.

The script is re-runnable: a second run of the same command (its own scratch, its own identity) also
reported `14 check(s) passed, 0 failed, 0 skipped` and exit 0, its turn answering `the attempt limit is
four attempts, set in internal/retry/policy.go`.

`scripts/tests/test_claude_native_check.py` pins the harness offline (16 cases, no `claude`, no `go`):
the shell reading of both reported command lines (the renderer's own rendering accepted; a quoted
assignment, an unquoted shell metacharacter, a glued `&&`, a quoted operator, a split working directory
and a line naming another session/home refused), the project-directory spelling, and the comparison
ordering — driven through the shipped `main` with its subprocess seams, where a mutating run must fail
exactly the three protected comparisons and exit 1.

### 6.2 The gate

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 185 tests in 4.951s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared … | owned: 27 project-owned prefix(es); files seen by git: 5642 | …
  PASS     secrets  no leaks found in 93 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

exit code **0**. The `test` step's three packages are `cmd/ccc`, `internal/ccc` and
`internal/ccc/parity`; `selftest` runs `scripts/tests` (185 cases: 0001's and 0002's own, this
increment's four new Go-side case files are inside the `test` step, the new native-check cases and the
evidence-claims rule) and re-runs itself in a child whose `PATH` holds no tools and once with the
platform probe answering Windows-without-a-compiler. 0002's tests run unchanged inside that step: the
increment's pipeline refactor (the route table) changed no pinned string, state, or command.

This block is the fix round's own run, re-measured on the repaired tree. In the implement round the
`test` step was green only where Claude Code was installed: `claude_verify_test.go` stubbed the turn the
two checks drive, but not the executable they resolve it through, so three of its cases reported `claude
is not on PATH` anywhere else — a green that could not reproduce on a host, or a CI runner, without the
harness. Both host-facing seams are now the test's own (the turn, and the lookup), and the
missing-binary answer keeps its own cases against a `PATH` holding no harness, so this `test` step
passes with no Claude Code installed here. The re-measurement corrects one more number this record
carried: its `gofmt` count said 3582, which is the count of the tree the increment *started from*
(`git ls-tree -r 8e1c502 --name-only`, counting `*.go`: 3582); the increment's nine new Go files make
this tree's count 3591.

The bookkeeping round moved `docs/goals/0002-claude-to-codex-continuation.md` to
`docs/goals/archive/`: 0002 is accepted — `main` carries its squash merge, `8e1c502`, and the index's
own wording ends the "awaiting acceptance" state at the merge — so the `docs/goals/README.md` row now
reads `Accepted: merged onto `main` as `8e1c502`; archived`, the moved file's prerequisite link is
rewritten for its new directory, and every link that named the goal's old path is retargeted (goals
0003, 0004, 0007 and 0014–0017 name it as a prerequisite; 0002's plan and both of its evidence records
name it as their contract). The archived file's own `**Status:**` line is left as reviewed, per the
round's finding — `docs/decisions/OPEN-QUESTIONS.md` Q-0003-09 records that choice against 0001's
archived spelling. `python3 scripts/gate.py` was re-run on the moved tree: exit **0**, `GATE: PASS`,
every count in the block above unchanged.

Two failures the increment's own new rules caught before this green run, and their fixes, stated
because a gate that only ever passed would not show that the rules bite: the new native-check case file
had reused seven case *method* names that already exist in the Codex file, and the suite's per-case
accounting keys cases by method name, so seven cases collapsed into seven statuses and `selftest` failed
the count (`178 != 185`) until the names were made distinct; and the evidence-claims rule failed on this
record twice — a §5 cell credited the run's answer while quoting file-side spellings (reworded to claim
only what it can show), and the record claimed an answer it had not yet recorded (§6.1's block was
added).

`parity` passes with **no carried file changed**: `docs/upstream.md` and `scripts/upstream-manifest.txt`
record the same pin, and the only ownership change is the new script
(`scripts/upstream-import.py`'s `OWNED` and the manifest's `owned` line), which the step reports as part
of the project-owned set.

## 7. Limits and residual risks

- **The `resume` level, and the native check's real turn, need model access.** On a host without it the
  turn is reported not-verified with the harness's own words (`Not logged in · Please run /login`); the
  offline `load` level still runs and still answers about *this* session, which is what keeps the default
  verification meaningful on a host with no provider configured.
- **`load` is bound to the flags the tested version honours.** It drives `claude --bare -p
  --output-format json --resume <id>` and reads the harness's own result document. A future version that
  changes `--bare`, the result document's shape or the not-logged-in path would be reported as
  not-verified with the harness's output quoted, never as a pass — but it would also mean this build's
  offline evidence needs re-measuring against that version.
- **Two facts are the target's behaviour, not this build's promises.** The loader drops `isSidechain`
  records from the model-visible history, and it truncates that history at the last
  `system/subtype:"compact_boundary"` marker. Both were measured on `2.1.326`; the writer's response to
  them (omit sidechains and count them; carry the boundary pair) is the decision, and the measurements
  are recorded in §0 so a future version's change is visible rather than silent.
- **The session grows when it is resumed.** A real turn appends the harness's own records (measured
  3 314 → 143 138 bytes for one turn). That is the target's own session after the caller asked for a
  turn; the *load* check avoids it by running against a throw-away copy, and the `installation` block's
  source check is about the source file only.
- **No message-level model or provider is recorded**, by decision (§5 and
  `docs/decisions/OPEN-QUESTIONS.md` Q-0003-04): the target keeps choosing its own model, and the
  transcript binds the session to nothing.
- **The parentUuid chain is written, not validated by the loader's own warning channel.** The writer
  always links consecutive records; the native check and the structure comparison verify the file's
  chain by reading it, and the loader's own warning (measured in §0) is quoted in this record rather
  than parsed by the build.
- **Windows has no POSIX mode bits**, so the install-permission assertion checks the mode the installer
  *requests* there and the installed file's own bits where the platform reports them — the same
  platform-aware branch 0002's installer test uses.
- **What this increment did not do:** no per-clause mutation sweep (0002's matrix accumulated one during
  its acceptance rounds; the review stage that follows this increment owns it), and no re-measurement of
  0002's Codex route beyond running its tests unchanged.
