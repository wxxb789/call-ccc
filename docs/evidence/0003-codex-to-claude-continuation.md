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

The command converts the committed Codex fixture — copied into a scratch source root with its recorded
working directory rewritten, the one mechanical edit §6.1's native check also makes — into an isolated
Claude Code home:

```console
$ CODEX_SESSIONS_DIR=/tmp/smoke2/src \
  ./bin/ccc continue codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 --to claude \
  --target-home /tmp/smoke2/home \
  --target-binary /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude
codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 → claude
  source:       /tmp/smoke2/src/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
  read from:    source
  conversation: 6 message(s), 2 user, 4 assistant, 1 tool call(s), 1 compaction boundary(ies)
  converted:    claude-code-transcript-jsonl, 3314 bytes: 4 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  omitted:      message_model_attribution ×5
  carried-with-caveat: pre_compaction_history ×2
  installed:    /tmp/smoke2/home/projects/-tmp-smoke2-ws/bf066719-a469-4450-b281-d45e258dd2f8.jsonl [hard-link]
  source file:  unchanged (sha256 605998b9b8f367c2fe09ae208b2e8c012bfec08b115a9c3cb2acac0f7d1452b8, taken before and after the conversion)
  target id:    bf066719-a469-4450-b281-d45e258dd2f8
  working dir:  /tmp/smoke2/ws
  resume:       cd /tmp/smoke2/ws && CLAUDE_CONFIG_DIR=/tmp/smoke2/home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume bf066719-a469-4450-b281-d45e258dd2f8
  non-interactive resume: cd /tmp/smoke2/ws && CLAUDE_CONFIG_DIR=/tmp/smoke2/home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude -p --output-format json --resume bf066719-a469-4450-b281-d45e258dd2f8 -- '<prompt>'
  cli_version:  2.1.326 (probe)
  structure:    verified (the inherited AgentsView Claude reader over the installed session)
  native load:  verified (the target harness's own loader (claude --bare --resume, with no model credentials in its environment, plus an unknown-id control))
  native resume: not-requested (…)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

(exit code 0. The `load` check's own sentence under `native load:` is abridged here to its state, as the
earlier recording abridged it; §6.5's re-measurement prints the check's own words and states the same
result. The `cd /tmp/smoke2/ws` in the command is the working directory the *copy* records: the committed
fixture records `/home/user/code/example-api`, and §6.1's native-check run reads the committed file's
own.)

**This block is the fix round's own re-recording** (§6.5, finding 3): it is the same command run again on
the fixed tree, so the printed non-interactive form carries the option terminator it did not have before,
and the `source:` line names the copy the run actually read — the earlier block showed the committed
fixture's path beside a digest that was always the copy's (the apply round's own note at §6.2).

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
is unchanged (the digest the check took of the installed file, named below)
```

The digest in that sentence is the **installed** file's, never the source's: `claude_verify.go` prints
`shortDigest` of `installedDigest(opts.Installed)`, taken before and after the loader ran, and
`opts.Installed` is the installed path. Re-measured in the apply round on this host — the same harness,
the committed fixture relocated into a scratch directory (its recorded cwd rewritten to
`/tmp/recheck/ws`), its own scratch root and `--target-home`:

```console
$ CODEX_SESSIONS_DIR=/tmp/recheck/codex /tmp/recheck/ccc continue codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 \
    --to claude --target-home /tmp/recheck/home2 --target-binary …/claude --json     # exit 0
… "verification": {"load": {"state": "verified", … "detail": "… the installed file is unchanged (c856d661817f)"}}
$ sha256sum /tmp/recheck/codex/2026/05/04/rollout-…-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
48db4ecb5ae4ab2c8644176d529a90253a89c5528f064004eef62d6085c955ec   # the source copy
$ sha256sum /tmp/recheck/home2/projects/-tmp-recheck-ws/5c074093-f85f-4526-afa1-5ed379d44adb.jsonl
c856d661817f101ce62f18ba9c4442d287881daa8e9f8f5bfae1293be0a624a2   # the installed transcript
```

Two different files, two different digests: the value inside the load detail belongs to the installed
file. An earlier revision of this block quoted §1's source digest there — a value no single run can
print, since the same run's source check reports the *source* file's digest (§1) and the load check the
installed file's.

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
| assistant turn `It is set in internal/retry/config.go.` + `shell` call `{"command":"grep -rn maxBackoffSeconds internal/retry"}` | two assistant records carrying different `message.id`s, because the source recorded the call (a `function_call` item) and the text (the later assistant message) as two different messages and the id is keyed per source message; the call's record holds the `tool_use` block its result record pairs with by `tool_use_id`, and the read-back shows both | carried (`tool_calls ×1`) |
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

Re-measured in the apply round, the same script against the same harness but **without a resume prompt**
— that level needs model access, which the round did not use, and the script never supplies credentials
of its own:

```console
$ python3 scripts/claude-native-check.py --claude /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude
… (this record's selection of the run's own lines; the paths are abridged the way the blocks above
abridge them)
PASS: the installed session is readable and writable only by its owner — mode 0600
PASS: the installed file is one JSON object per line — 3674 bytes (the target's own read of it is the load check)
PASS: the target harness's own loader resolves the installed session — verified — …
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
PASS: the source session is unchanged — sha256 100d252ea32055b2… (the scratch source root is identical)
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 1 skipped
```

exit code **0**. Every check that needs no model access ran, including the owner-only mode check, the
JSONL check and the loader/control pair; the one skipped check is the real turn, named as not
attempted. The round's own edits to the script (the shared-mechanism module, the three defect fixes and
the renames) are therefore exercised end to end against the installed harness; the run recorded above
stays the round that had model access.

`scripts/tests/test_claude_native_check.py` pins the harness offline (19 cases, no `claude`, no `go`):
the shell reading of both reported command lines (the renderer's own rendering accepted; a quoted
assignment, an unquoted shell metacharacter — a bare `&` included — a glued `&&`, a quoted operator, a
split working directory and a line naming another session/home refused), the project-directory spelling,
the comparison ordering — driven through the shipped `main` with its subprocess seams, where a mutating
run must fail exactly the three protected comparisons and exit 1 — and, since the apply round, the
platform branch of the owner-only mode check (a host that reports no POSIX mode bits answers SKIP, never
FAIL) and the named failure of a malformed installed line (both checks' case files carry that one), and
since the fix round the shell the turn is read by: every turn case asserts it is the shell the seam
answered, and a host whose seam answers none gets a named SKIP with that reason rather than a line read by
whatever `shell=True` would select (§6.3, finding 4).

### 6.2 The gate

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 188 tests in 4.964s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared … | owned: 28 project-owned prefix(es); files seen by git: 5643 | …
  PASS     secrets  no leaks found in 94 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

exit code **0**. The `test` step's three packages are `cmd/ccc`, `internal/ccc` and
`internal/ccc/parity`; `selftest` runs `scripts/tests` (188 cases: 0001's and 0002's own, this
increment's four new Go-side case files are inside the `test` step, the new native-check cases and the
evidence-claims rule) and re-runs itself in a child whose `PATH` holds no tools and once with the
platform probe answering Windows-without-a-compiler. 0002's behaviour runs unchanged inside that step:
the help case's pinned `--to` spelling was widened to name both targets (and gained both routes' example
lines), and the refusal case now names a target this build does not write (`--to gemini`), with an
empty-target case added beside it — neither assertion was weakened, and the route table's refactor
changed no other pinned string, state or command.
This block is the apply round's own run, on the tree that carries the accepted simplify findings; the
counts those findings moved are named below. In the implement round the
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
every count of that tree unchanged from the run before the move.

The apply round (the accepted simplify findings — this record's own last editing round) re-ran this gate
and the native check's offline half on the tree it edited. The counts that moved, and why: `selftest`
**185 → 188** (three new offline cases — the owner-only mode check's platform branch, and one
malformed-line failure case per native check), `parity`'s `owned` **27 → 28** and `files seen by git`
**5642 → 5643** (the two native checks' shared-mechanism module, `scripts/native_check_common.py`,
registered in `scripts/upstream-import.py`'s `OWNED` and the manifest's `owned` record), and `secrets`
**93 → 94** file(s) for the same new file. `gofmt`'s 3591 and the three `test` packages do not move. No
case in either suite was deleted, skipped or weakened: one Claude-side case was **added** (a converted
prompt the inherited reader projects away must make the verification detail name the **Claude Code**
reader, not the Codex one — the defect it pins is described below), and four had their assertions
strengthened (the load check's denied-environment contract is now asserted against the harness's
documented names written out as literals; the committed fixture's round trip must report an *empty*
detail; every loss category is read through the presence-guarded `lossCount`), and one test-side
declaration was merged without changing an assertion (the two routes' CLI cases now decode one shared
document type instead of two hand-copied ones). The round also corrects two record
statements: §3(b)'s quoted digest is the **installed** file's, re-measured above, and §1's digest is the
relocated source copy *that* run read, not the committed fixture's own (`sha256 d9957a8d…`), because the
source check digests the file it read. The copy is reproducible from the tree: parsing each line of the
committed fixture, rewriting the **first** record's `payload.cwd` to `/tmp/smoke2/ws` and re-encoding each
line with `json.dumps`' default separators is byte for byte the file whose `sha256` is
`605998b9b8f367c2fe09ae208b2e8c012bfec08b115a9c3cb2acac0f7d1452b8` — a JSON-level relocation, which is
why a global `sed` of the recorded path answers a different digest.

Two failures the increment's own new rules caught before this green run, and their fixes, stated
because a gate that only ever passed would not show that the rules bite: the new native-check case file
had reused seven case *method* names that already exist in the Codex file, and the suite's per-case
accounting keys cases by method name, so seven cases collapsed into seven statuses and `selftest` failed
the count (`178 != 185`) until the names were made distinct; and the evidence-claims rule failed on this
record twice — a §5 cell credited the run's answer while quoting file-side spellings (reworded to claim
only what it can show), and the record claimed an answer it had not yet recorded (§6.1's block was
added).

The apply round also fixed three defects the simplify stage had demonstrated, and the first is why the
added case above exists: (1) the structure verification's detail named "the inherited **Codex** reader"
on *every* route, because the sentence was hard-coded in the comparison type both routes now share, so a
Claude conversion whose read-back was not verbatim reported a reader that never read the file — while the
report's *warning* for the same condition already named "the inherited **Claude Code** reader". `detail()`
now takes the reader's name from the route's own constant (`structureReaderCodex`/`structureReaderClaude`),
and the added case fails against the old shape on both of its assertions. (2) `scripts/claude-native-check.py`
refused every shell metacharacter in the printed invocation's spelling *except a bare `&`*, so
`CLAUDE_CONFIG_DIR=a&b …` — two commands to a POSIX shell, which is exactly what the check exists to
catch — was accepted; the exemption is gone and a rejection case stands beside the `;` case. (3) A
malformed line in an installed file raised out of both native-check scripts instead of failing a named
check, contradicting the harnesses' own contract of one verdict line per check; a shared JSONL helper now
reports the failing line and its parse error, and each script's case file pins the named failure with
exit 1.

`parity` passes with **no carried file changed**: `docs/upstream.md` and `scripts/upstream-manifest.txt`
record the same pin, and the only ownership changes are the two new scripts
(`scripts/claude-native-check.py`, whose `owned` line this increment added, and its shared-mechanism
module `scripts/native_check_common.py`, whose line the apply round added: `scripts/upstream-import.py`'s
`OWNED` tuple and the manifest's `owned` records), which the step reports as part of the project-owned
set.

### 6.3 The fix round (review round 1's four findings)

Review round 1 read `8e1c5026…6de08c4b` and found four defects. Each was reproduced before it was fixed,
and each changed behaviour, a document or a case — none is a comment. No `Done when` clause moved.

**(1) One multi-file patch was written as two native tool calls.** The inherited Codex reader projects one
`apply_patch` into one call per file it names (`internal/parser`'s `handleFunctionCall`), every projection
sharing the call's id and its input — which is what the archive's file-keyed views group by. The Claude
writer wrote a `tool_use` record per projection under one `message.id`, and the inherited Claude reader
puts records that share a message id back into one message and keeps one block per id, so the installed
file held one call while the writer's item list claimed two: the mandatory structure check refused the
conversion *after* installing it. Reproduced on a new committed fixture (§ below) against the reviewed
tree, in the check's own words:

```text
the installed session does not hold the evidence in the target document: tool call 2 is in the target
document but not in the file: call call_02Example000000000000000000 to apply_patch (128 bytes, sha256 552563bd117ce783…)
```

`claudeTranscriptBuilder.distinctCalls` now writes the first projection of a call id and counts the ones
it did not write as their own record; that count is reported as its own category,
`coalesced_tool_call_projections` (carried with caveat: the call and its input travel — the patch's own
text names every file it touched — while a second projection's per-file attribution has no field in the
target). Pinned by `TestContinueIntoClaudeCoalescesOneCallsPerFileProjections` (the route's carried
counts, the reported category and its reason, exactly one `tool_use` and one `tool_result` block for the
call id in the installed file, the inherited reader's own read-back of one call, and structure
`verified`), and the category is read through `lossCount` in
`TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers`, which requires every known category to be
reported on every run.

**(2) A call whose only outcome was the archive's summary column vanished in silence.** `ToolCall.ResultContent`
is the summary an archive read holds beside a call; the Codex writer already carries it for a call with no
recorded result of its own (`callHasRecordedResult`), and the Claude writer never read the field — so the
outcome left the transcript *and* the item list, and the structure check passed over the omission, which
is what the finding is about. The same fallback now runs for this target under the same rule (the summary
is a view of the archive's own recorded events, so it is written only where no result was recorded).
Reproduced by removing the loop again: the file held `toolu_1`, `toolu_2 ×2` and no `toolu_3`, carried
three results instead of four, and the read-back showed three — with structure `verified` either way.
Pinned by `TestClaudeWriterWritesAnArchiveSummaryOnlyForACallWithoutResults` (the written blocks and
carried count, no `empty_tool_results` count, and the round trip: install, read back through the inherited
reader, structure `verified`).

**(3) A relative `--target-binary` was printed unresolved for an invocation that changes directory.** A
Claude invocation runs from the session's working directory (measured: the loader finds a session through
the directory it is run from), so the reviewed line was
`cd /home/user/code/example-api && CLAUDE_CONFIG_DIR=… tools/claude --resume …` while the conversion, the
probe and the native verification had resolved `tools/claude` in the caller's own directory — two
programs answered by one report, and a printed command that fails even after a successful verification.
Reproduced with the caller's directory and the session's directory distinct (the failure is the test's own
output against the reviewed tree):

```text
resume command "cd /home/user/code/example-api && CLAUDE_CONFIG_DIR='/tmp/…/003/claude home' tools/claude
--resume 93502ad1-…" does not name the executable in the caller's directory
```

`invocationBinary` resolves a spelling that names a path against the caller's working directory before the
route builds either vector or line, and leaves a bare name alone (a PATH lookup is not affected by a
directory change; the Codex route's invocation changes no directory and is untouched). Pinned by
`TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation`, which runs from a caller
directory distinct from the session's and asserts both argument vectors, both printed lines, and that a
bare name still travels verbatim. The `--target-binary` help line was corrected with it: it had said the
printed commands name the binary "the way it is given here", which this rule makes untrue for a path.

**(4) The native check executed a POSIX line through the platform's own command shell.** `run_shell` used
`subprocess.run(..., shell=True)`, which selects `cmd.exe` on Windows, where `cd … && CLAUDE_CONFIG_DIR=…
claude …` and its single-quote escaping are not a command at all: the real-turn check would fail before
the harness was reached and report the platform instead of the conversion. The offline cases replace
`run_shell`, so none of them could see it. The script now names the shell it reads the line with
(`posix_shell`: `/bin/sh` on a POSIX host, else the first `sh`/`bash` on `PATH`), passes it to
`run_shell(line, cwd, env, shell)`, and reports a host with no POSIX shell as a named **skip** with that
reason — the shape the owner-only mode check already uses for its platform boundary. Reproduced by
restoring the reviewed shape for one run (the platform's own shell, no boundary): the new case recorded
`PASS` instead of `SKIP`, and the intact case was handed `"the platform's own shell"` instead of the
seam's answer. Pinned by `test_a_host_with_no_posix_shell_reports_the_turn_as_a_boundary` (named skip with
the reason, no turn output printed, exit 0, and the shell runner never called) and by
`test_an_intact_claude_run_passes_the_comparisons` / `test_a_mutation_during_…`, which now also assert
which shell the turn was handed.

**The fixture the multi-file patch case needed** is committed and generated like the others:
`codex/2026/05/06/rollout-2026-05-06T10-30-00-5b2d7c14-…jsonl` (one `apply_patch` naming
`internal/retry/config.go` and `internal/retry/policy.go`), written by `internal/ccc/fixturegen` through
the pinned builders, so `TestFixturesMatchTheirGenerator` pins its bytes. It is project-owned
(`internal/ccc/` is an `owned` prefix), and it adds no manifest or `OWNED` entry: the parity step's
project-owned set is unchanged at 28 prefixes, with `files seen by git` 5643 → 5644.

**Re-measured in this round**, on this tree, against the installed harness — the load half (no model
access used, §7) and the turn's own code path:

```console
$ python3 scripts/claude-native-check.py --claude …/claude-cli/node_modules/.bin/claude      # exit 0
PASS: the conversion reports carried evidence and a non-zero loss category — carried 4 message(s), … 3674 byte(s); non-zero loss categories: tool_name_vocabulary=1, message_model_attribution=5, pre_compaction_history=2
PASS: the target harness's own loader resolves the installed session — verified — … "Not logged in · Please run /login" …
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
13 check(s) passed, 0 failed, 1 skipped

$ python3 scripts/claude-native-check.py --claude …/claude --resume-prompt "Summarise in one sentence what this conversation records about retry attempts and where that is set."   # exit 1
FAIL: a real turn continues from the installed session — exit 1, session 3674 -> 85802 bytes; ran through /bin/sh: cd /tmp/…/workspace/example-api && CLAUDE_CONFIG_DIR=/tmp/…/claude-home …/claude -p --output-format json --resume 071aaca2-… 'Summarise in one sentence …'
13 check(s) passed, 1 failed, 0 skipped
```

The second run is the documented boundary, not a regression: the line was read by the shell the script
named and reached the harness, whose own answer is the not-logged-in one this record already names in §7;
the file grew (3674 → 85802 bytes) and no protected artifact moved. Its check's detail line is the one
shape this round changed: it now says `ran through /bin/sh: …` so the report names the shell that read the
line. §6.1's block is the earlier round's run and is left as it was recorded.

**The gate, re-run on this tree** (§6.2's block is the apply round's run):

```console
$ python3 scripts/gate.py
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 189 tests in 5.773s (rc=0)
  PASS     parity   … owned: 28 project-owned prefix(es); files seen by git: 5644 | …
  PASS     secrets  no leaks found in 95 file(s) this repository owns …
GATE: PASS
```

exit code **0**. Counts that moved, and why: `selftest` **188 → 189** (one new offline case — the host
with no POSIX shell), `files seen by git` **5643 → 5644** and `secrets` **94 → 95** (the new committed
Codex fixture), while `gofmt`'s 3591, the three packages and `parity`'s 28 owned prefixes do not move. The
Go-side cases added are `TestContinueIntoClaudeCoalescesOneCallsPerFileProjections`,
`TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation` and
`TestClaudeWriterWritesAnArchiveSummaryOnlyForACallWithoutResults`; no existing case was deleted,
skipped or weakened, and the two that changed do so by reading more (the category enumeration, and the
turn cases' assertion about the shell).

`AGENTS.md` needed no change in this round: it carries the read order, the parity rule, the gate command
and the stage rules, none of which moved — the round added no command, no step and no rule an agent has
to follow. `docs/goals/README.md` and `docs/goals/archive/` likewise stay as the apply round left them:
0003 is this increment's open goal (`Implemented on …; awaiting operator acceptance`), and the accepted
0002 was archived in the previous round.

### 6.4 The CI repair round (`gate (windows-latest)`)

The hosted gate failed this branch's head `595e136` on one leg — `gate (windows-latest)`, job
133927852 of run 44050830659 — in the `test` step, the other two legs green:

```console
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  FAIL     test     go test -count=1 ./cmd/ccc/... ./internal/ccc/... -> rc=1
…
FAIL	go.kenn.io/agentsview/internal/ccc	17.764s
  PASS     selftest Ran 189 tests in 9.579s (rc=0)
GATE: FAIL · test
```

The step's printed detail is a bounded tail of the test run, so the failing case's own line was not
retained; the package it names is the one the case is in.

**The defect was in the case, not in the write path.**
`TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation` asserted the *bare*
spelling of the resolved executable in both printed lines, while the renderer spells every word the way a
POSIX shell reads it back — a word holding any of `shellUnsafeCharacters` is quoted, and every Windows
path holds backslashes, which are such characters. A host whose temp root holds a space is in the same
arm — the word a shell acts on is quoted, whatever the platform spells in it — and `TMPDIR` puts this host
there, which is what the reproduction below is: the case fails because the quoted word is presented to an
assertion spelled bare. The run, on the pre-fix file:

```console
$ TMPDIR='/tmp/ccc win' go test -count=1 -tags fts5 -run TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation ./internal/ccc/
--- FAIL: TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation (0.15s)
    continue_claude_test.go:395: resume command "cd /home/user/code/example-api && CLAUDE_CONFIG_DIR='/tmp/ccc win/…/003/claude home' '/tmp/ccc win/…/002/tools/claude' --resume df086aae-…" does not change into "/home/user/code/example-api" and name the executable there
    continue_claude_test.go:399: exec resume command "cd /home/user/code/example-api && CLAUDE_CONFIG_DIR='/tmp/ccc win/…/003/claude home' '/tmp/ccc win/…/002/tools/claude' -p --output-format json --resume df086aae-… '<prompt>'" does not name the executable in the caller's directory
FAIL	go.kenn.io/agentsview/internal/ccc	0.174s
```

exit code **1** on the pre-fix tree. The case now spells the caller's own directory `caller with a space`
— the shape `claudeTargetHome` in the same file and the CLI's `shellUnsafeHome` already pin — and asserts
the quoted word in both lines; the resolution assertions themselves (both argument vectors, the bare name
that travels verbatim) are unchanged. Nothing else moved: the case keeps its name, the write path is
untouched, and of the three Go cases §6.3 lists only this one changed — one fixture spelling and two
assertions.

**The case still fails when the rule it pins is broken.** Restoring the reviewed shape in a scratch copy
of `internal/ccc/targets.go` (`binary := invocationBinary(in.Binary)` → `binary := in.Binary`) fires all
four assertion lines — both argument vectors come back as the relative word, and both lines name it
unquoted — exit code **1**; the file was restored and `git diff` is clean for it.

**Re-run on this tree**, the case in the three host shapes, the whole Go suite on the space-named temp
root, and the gate:

```console
$ go test -count=1 -tags fts5 -run TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation ./internal/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.226s
$ TMPDIR='/tmp/ccc win' go test … -run TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation ./internal/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.129s
$ TMPDIR='/tmp/ccc-win\temp' go test … -run TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation ./internal/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.062s
```

each run exit code **0** — where the pre-fix file's own run above is exit code **1**.

```console
$ TMPDIR='/tmp/ccc win' go test -count=1 -tags fts5 ./internal/ccc/... ./cmd/ccc/...
ok  	go.kenn.io/agentsview/internal/ccc	12.272s
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.858s
ok  	go.kenn.io/agentsview/cmd/ccc	3.365s
```

each exit code **0**.

```console
$ python3 scripts/gate.py
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 189 tests in 4.960s (rc=0)
  PASS     parity   … owned: 28 project-owned prefix(es); files seen by git: 5644 | …
  PASS     secrets  no leaks found in 95 file(s) this repository owns …
GATE: PASS
```

exit code **0**. No count moves: `gofmt` 3591, the three packages, `selftest` 189, `files seen by git`
5644 and `secrets` 95 are §6.3's numbers, because no case was added or removed. On a root whose *name*
holds a backslash (the Windows-shaped trick `docs/harness.md` records) two `cmd/ccc` cases —
`TestCodexLoadVerificationRequiresTheSessionsOwnRecord`,
`TestResumeVerificationRequiresTheTurnToAppendToTheSession` — still fail on this host: that is the
artifact that file already names (the stub harness finds the installed session with `filepath.Glob`, and a
POSIX `Glob` reads a backslash in the pattern as an escape), not this round's subject; they are green on
the real Windows leg, where `cmd/ccc` is otherwise unchanged but for one `--help` line.

`AGENTS.md` needed no change in this round either: no command, step, layout or rule moved. The case now
follows the rule that file already states — a case whose subject is what a check makes of a target
harness's answer must hold on every host the docs name, not only on the host that runs it.
`docs/goals/README.md` and `docs/goals/archive/` are unchanged: 0003 is still this increment's open goal.

### 6.5 The fix round (review round 2's four findings)

Review round 2 read `8e1c5026…238d7bc9` and found four defects. Each was reproduced before it was fixed,
and each changed behaviour, a case or a document — none is a comment. No `Done when` clause moved. One
finding's stated mechanism is answered with evidence rather than accepted as written (finding 2), and the
remedy it asks for was applied anyway, because the shape it closes is real.

**(1) A recorded working directory whose last component ends in whitespace was trimmed.**
`ConvertToClaudeTranscript` read `strings.TrimSpace(conv.Source.Cwd)`, and that trimmed value is what the
transcript's `cwd`, the project directory the session is filed under, the printed `cd` and the native
verification's own `Dir` were derived from — while the inherited reader carries the source's string
verbatim. Reproduced by restoring the trim on this tree; the two new cases fail:

```console
$ go test -count=1 -tags fts5 -run 'TestConvertToClaudeTranscriptKeepsAWorkingDirectoryEndingInWhitespace|TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace' ./internal/ccc/   # exit 1
--- FAIL: TestConvertToClaudeTranscriptKeepsAWorkingDirectoryEndingInWhitespace (0.00s)
    claude_target_test.go:686: the target session records cwd "/home/user/code/project", want the source's own "/home/user/code/project "
    claude_target_test.go:689: the project directory is "-home-user-code-project", want "-home-user-code-project-", the spelling of the source's own directory
    claude_target_test.go:694: record 0 carries cwd /home/user/code/project, want the source's own "/home/user/code/project "
--- FAIL: TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace (0.07s)
    continue_claude_test.go:529: target working directory = "/home/user/code/project", want the source's own "/home/user/code/project "
    continue_claude_test.go:536: installed at "…/projects/-home-user-code-project/b2757ccb-….jsonl", want "…/projects/-home-user-code-project-/b2757ccb-….jsonl"
FAIL
```

The recorded string now travels as it was recorded, and the trim answers only *whether* one was recorded
(`strings.TrimSpace(cwd) == ""` → `ErrNoTargetCwd`), in the writer and in `runClaudeTurn`'s `Dir` alike.
Pinned by the writer's case (the record bytes, the project spelling, the whitespace-only refusal) and the
route's (the installed path below `projects/-home-user-code-project-`, the printed
`cd '/home/user/code/project ' && …`, and the structure read-back still `verified`).

**(2) The printed Claude invocation re-searched PATH after its `cd`.** The review's remedy is what this
round applied: `invocationBinary` now takes the probe's own answer and names it when it found one
(`invocationBinary(in.Binary, in.Probe)`), keeping the caller's spelling only where the probe found
nothing — a path-like spelling still resolved against the caller's directory, a bare name still left
exactly as written. The report's own `binary`/`binary_found` fields carry that same answer, so the
printed line and the report cannot disagree. Pinned by `TestClaudeInvocationNamesTheExecutableItsProbeFound`
(both argument vectors and the rendered line, with a probed path the renderer has to quote; and the
unresolved probe leaving the word alone), and the relative-spelling case still pins its own two arms.
Removing the probe from the call fires the new case:

```console
$ go test -count=1 -tags fts5 -run TestClaudeInvocationNamesTheExecutableItsProbeFound ./internal/ccc/   # exit 1
--- FAIL: TestClaudeInvocationNamesTheExecutableItsProbeFound (0.00s)
    continue_claude_test.go:463: arguments name "claude"/"claude", want the probed executable "/opt/ccc tools/claude"
FAIL
```

**The mechanism the finding states is not reachable on this build, and that is measured rather than
assumed.** With a relative PATH entry, a *bare* name is not found at all: Go's `os/exec.LookPath`
answers `ErrDot` for a resolution that is relative to the working directory (since Go 1.19). A two-line
program calling `exec.LookPath("fakeclaude")`, built here with go1.27.4 and run with
`PATH="tools:/usr/bin:/bin"` from a directory holding an executable `tools/fakeclaude`:

```text
bare lookup with a relative PATH entry: path="" err=exec: "fakeclaude": cannot run executable found relative to current directory
path-like lookup: path="tools/fakeclaude" err=<nil>
```

So the probe answers *found nothing* there and the report says so; what remains real is the step after it:
the printed line is read by a **shell**, and a shell resolves a relative PATH entry against the directory
it is in — the session's, after the `cd` — so a pasted bare name can select `<session dir>/tools/claude`
while the probe and the native verification ran another program. Naming the probed path is what closes
that, and it is why the remedy was applied even though the reviewed mechanism as written cannot occur.

**(3) The reported exec form omitted the prompt's option terminator.** `VerifyClaudeResume` runs its prompt
behind `--` while the printed exec form did not, so replacing `<prompt>` with `--help` made the reported
command ask for help instead of submitting the turn the verification ran. The terminator is now one named
constant (`promptTerminator`, `internal/ccc/continue.go`) used by the resume verification, the load check
and the route's printed exec form, whose vector is
`[binary, -p, --output-format, json, --resume, <id>, --, <prompt>]`. The Go cases compare the whole vector
and the rendered line (`internal/ccc/continue_claude_test.go`, `cmd/ccc/claude_route_test.go`), the native
check's own reading carries it (`scripts/claude-native-check.py`'s `want_exec_resume`, the constant
mirrored as `native_check_common.CONTINUE_TERMINATOR`), and the new offline case drives both shapes
through the shipped `main`: the intact vector passes the invocation check, the pre-fix vector fails
exactly that check and exits 1. Dropping the terminator fires the Go side:

```console
$ go test -count=1 -tags fts5 -run 'TestContinueIntoClaudeConvertsInstallsAndReadsBack|TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation' ./internal/ccc/   # exit 1
--- FAIL: TestContinueIntoClaudeConvertsInstallsAndReadsBack (0.13s)
    continue_claude_test.go:138: exec resume arguments = [ … "--resume" "e9e3a821-…" "<prompt>"], want [ … "--resume" "e9e3a821-…" "--" "<prompt>"]
--- FAIL: TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation (0.02s)
    continue_claude_test.go:406: exec resume arguments = [ … ], want the same resolved executable and the prompt behind "--"
FAIL
```

and `cmd/ccc`'s route case fails the same way (its two assertions name the full vector and the rendered
`… --resume <id> -- '<prompt>'`).

**(4) A discarded subagent-session link was invisible.** The inherited Codex parser records which session
a `spawn_agent` call spawned (`ToolCall.SubagentSessionID`; the carried case is
`internal/parser/codex_parser_test.go`'s "spawn_agent links child session…", which pins
`codex:<child id>`) and the shared reader preserves it (`internal/ccc/reader.go:800`), but the Claude
writer's `tool_use` block carries an id, a name and an input only — so the call travelled, the link did
not, and neither the item list nor the structure check could see the difference. It is now counted as its
own category, `subagent_session_links` (disposition `omitted`), whose reason states that the target has no
field for a child session and that the child session is neither read nor converted — a conversion's unit
is the selected session. The count is of the calls this conversion wrote, so a projection coalesced into
the call it repeats is not counted twice. Pinned by
`TestClaudeWriterCountsADiscardedSubagentSessionLink` (count, disposition, the reason's words, the
coalesced-projection discipline, the call's own evidence still in the file, and the child id in none of
it) and read through the category enumeration of `TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers`.
Removing the count fires it:

```console
$ go test -count=1 -tags fts5 -run TestClaudeWriterCountsADiscardedSubagentSessionLink ./internal/ccc/   # exit 1
--- FAIL: TestClaudeWriterCountsADiscardedSubagentSessionLink (0.00s)
    claude_target_test.go:645: subagent_session_links = {Category:subagent_session_links Disposition:omitted Count:0 Detail:the source recorded which session these carried calls spawned (…)} …, want the one written call's link counted as omitted
FAIL
```

**Re-measured in this round**, on this tree, against the installed harness — the load half and the
printed line's own turn path (this round had no model access, §7):

```console
$ python3 scripts/claude-native-check.py --claude …/claude-cli/node_modules/.bin/claude      # exit 0
PASS: the reported resume command names the binary, the session, the working directory and the data directory — resume_command='cd …/workspace/example-api && CLAUDE_CONFIG_DIR=…/claude-home …/claude --resume 42eea641-…' exec_resume_command="cd …/workspace/example-api && CLAUDE_CONFIG_DIR=…/claude-home …/claude -p --output-format json --resume 42eea641-… -- '<prompt>'" environment={'CLAUDE_CONFIG_DIR': '…/claude-home'}
PASS: the target harness's own loader resolves the installed session — verified — … "Not logged in · Please run /login" …
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
PASS: the source session is unchanged — sha256 399cbe7e15e07df7… (the scratch source root is identical)
13 check(s) passed, 0 failed, 1 skipped

$ python3 scripts/claude-native-check.py --claude …/claude --resume-prompt "Summarise in one sentence what this conversation records about retry attempts and where that is set."   # exit 1
FAIL: a real turn continues from the installed session — exit 1, session 3674 -> 85802 bytes; ran through /bin/sh: cd /tmp/…/workspace/example-api && CLAUDE_CONFIG_DIR=/tmp/…/claude-home …/claude -p --output-format json --resume b0422b4f-… -- 'Summarise in one sentence …'
13 check(s) passed, 1 failed, 0 skipped
```

The second run is §7's documented boundary again, and it is the strongest evidence this round can offer
for finding 3 without model access: the printed line *with the terminator* was read by the shell the
script names, reached the harness, and the harness wrote its own records into the installed session
(3674 → 85802 bytes) before answering with the not-logged-in words §7 records. The single failure is the
turn's own verdict, which needs a model. A turn that actually *answered* is §6.1's earlier run;
the exec spelling it shows (`… --resume 6038ac68-… 'Summarise in one sentence …'`) is the pre-fix one,
as are §6.3's FAIL line and §6.4's quoted failures — they are the runs that printed them and are left as
recorded, and §1's receipt above is the fix round's own re-recording.

**The gate, re-run on this tree**, after every code, script, case and record edit this round made:

```console
$ python3 scripts/gate.py
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 190 tests in 5.024s (rc=0)
  PASS     parity   … owned: 28 project-owned prefix(es); files seen by git: 5644 | …
  PASS     secrets  no leaks found in 95 file(s) this repository owns …
GATE: PASS
```

exit code **0**. This block is the round's final run. Two record edits followed it — the timing text inside
it and `docs/evidence/0003-obligation-matrix.md`'s own sweep section — and no gate step reads either: the
`selftest` step's evidence case reads §5's tables of the `*continuation*.md` records, and this round does
not touch them.

Counts that moved, and why: `selftest` **189 → 190** (one new offline case — the exec form's terminator,
driven in both shapes through the shipped `main`). `gofmt`'s 3591, the three packages,
`parity`'s 28 owned prefixes, `files seen by git` 5644 and `secrets` 95 do not move: this round edits
existing files and adds none. No case was deleted, skipped or weakened; four Go cases were added
(`TestConvertToClaudeTranscriptKeepsAWorkingDirectoryEndingInWhitespace`,
`TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace`,
`TestClaudeInvocationNamesTheExecutableItsProbeFound`,
`TestClaudeWriterCountsADiscardedSubagentSessionLink`) and the exec-form assertions in
`internal/ccc/continue_claude_test.go` and `cmd/ccc/claude_route_test.go` were replaced by stronger ones
(the whole argument vector plus the rendered line, where a substring of the vector was read before).

**The `--target-binary` help line was corrected a second time**, with the rule finding 2 gave it: it had
said the printed commands name the binary "the way it is given here, resolved against this directory"
(round 1's wording), which the probe's answer makes untrue for a bare name the probe resolved. It now says
the printed commands name the path the probe found, else the spelling given, and `README.md`'s sentence
about the same flag was corrected with it.

`AGENTS.md` needed no change in this round: no command, gate step, layout, read order or rule an agent
follows moved — the round changes what one writer records for one source field, what one printed
invocation names, and one loss category. Two records outside this file moved with it:
`docs/decisions/OPEN-QUESTIONS.md` gained Q-0003-11 (the printed invocation naming the probed path rather
than the caller's word, and its reversal), and `docs/goals/README.md` and `docs/goals/archive/` are
unchanged — 0003 is still this increment's open goal, no accepted goal file is sitting under
`docs/goals/`, and the bookkeeping rule therefore has nothing to move this round.

### 6.6 The second CI repair round (`gate (windows-latest)`)

The hosted gate failed this branch's head `a2670d1` in the leg and step §6.4 names — run `44056002123`,
job `133945434046`, 2m32s — with `ubuntu-latest` and `macos-latest` green:

```console
$ gh run view 44056002123 --job 133945434046 --log-failed   # verbatim, its own truncated detail included
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  FAIL     test     go test -count=1 ./cmd/ccc/... ./internal/ccc/... -> rc=1
…
FAIL
FAIL	go.kenn.io/agentsview/internal/ccc	21.168s
?   	go.kenn.io/agentsview/internal/ccc/fixturegen	[no test files]
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.996s
?   	go.kenn.io/agentsview/internal/ccc/parity/oracle	[no test files]
FAIL
  PASS     selftest Ran 190 tests in 16.479s (rc=0)
GATE: FAIL · test
```

exit code **1** on that leg. The step prints a bounded tail of the test run (`gate.py`'s `tail()`, its
last 2 500 characters), so the failing case's own line was not retained again and the identification is by
inspection rather than by the leg's own words: the package it names is the one §6.5's four new Go cases
are in, and of those exactly one reads the *host's* own answer about what a path is — the other three
assert constant strings, constant bytes and counts, and the four are the same on every platform but for
that one word. The mechanism below is what the leg then answers for that word.

**The defect was in the case's fixture, not in the write path.**
`TestClaudeInvocationNamesTheExecutableItsProbeFound` handed the route a probe answer spelled
`"/opt/ccc tools/claude"` and asserted the invocation names that word back. `invocationBinary` trusts the
platform's own answer about absoluteness — a found probe answer wins, and a word is re-spelled only when
`filepath.IsAbs` says it is not a path (`internal/ccc/targets.go`) — and Go's **Windows** `IsAbs` answers
false for that word: its `volumeNameLen` needs a drive letter, a device prefix (`\\.\`, `\\?\`, `\??\`) or
a `\\host\share` UNC form, and one leading separator with none of them is not a volume
(`$(go env GOROOT)/src/internal/filepathlite/path_windows.go:184` `IsAbs`, `:206` `volumeNameLen`, in the
toolchain this tree builds with). The toolchain's own test table states the same as the authority's
expectation: `$(go env GOROOT)/src/path/filepath/path_test.go:1105` — "All non-windows tests should fail,
because they have no volume letter" — with `{"/", false}` in `winisabstests` (`:1077`) and
`{"/usr/bin/gcc", true}` among the POSIX entries that table turns false there (`:1066`). An independent
implementation of the same platform's path rules, executed on this host, agrees about which group that
word is in:

```console
$ python3 -c "import ntpath, posixpath; print(ntpath.isabs('/opt/ccc tools/claude'), posixpath.isabs('/opt/ccc tools/claude'))"
False True
```

So on that leg the probe's word was resolved against the runner's working directory and the case's two
assertions fired. The consequential branch — a probe answer the running host does not call absolute — is
reproduced here rather than assumed, with a scratch case (deleted after the run, never in the tree): the
same route is handed two probe answers, one this host calls absolute and one it does not, and the
invocation names the second one's re-resolution instead of the probe's word:

```console
$ go test -count=1 -tags fts5 -v -run TestScratchProbeAnswerPlatformArm ./internal/ccc/   # exit 0
=== RUN   TestScratchProbeAnswerPlatformArm
    zz_platform_arm_scratch_test.go:25: probe answered "/opt/ccc tools/claude"; filepath.IsAbs=true; the invocation names "/opt/ccc tools/claude"
    zz_platform_arm_scratch_test.go:25: probe answered "opt/ccc tools/claude"; filepath.IsAbs=false; the invocation names "/home/azureuser/repos/call-ccc/internal/ccc/opt/ccc tools/claude"
--- PASS: TestScratchProbeAnswerPlatformArm (0.00s)
```

The second logged line is the shape Windows takes for the first word, which is what the failing leg's
assertion answered.

**The repair is the fixture.** The case now takes the probed path from the host's own temp root
(`probed := filepath.Join(t.TempDir(), "ccc tools", "claude")`), which is a path the host running the case
calls absolute — the spelling this case's own probe answer has, because the route's probe answers what
`os/exec.LookPath` found and a bare name's hit is a `PATH` hit: both platforms refuse a hit that is not
absolute (`ErrDot`, unless the caller opts out with `GODEBUG=execerrdot=0`) rather than answering a
relative one. One component holds a space, so the quoting rule the case is also about stays exercised on
every host, and the assertions themselves did not move: both argument vectors and the rendered line are
still compared with the probed path. `invocationBinary` and the write path are unchanged — a probe's word
is handed over as it stands whenever the host calls it absolute, which its own answer is — so only the
case's fixture moved.

**The case still fails when the rule it pins is broken.** Restoring the reviewed shape in a scratch copy
of `internal/ccc/targets.go` (`binary := invocationBinary(in.Binary, in.Probe)` → `binary := in.Binary`,
the file restored afterwards; `git diff` clean for it):

```console
$ go test -count=1 -tags fts5 -run TestClaudeInvocationNamesTheExecutableItsProbeFound ./internal/ccc/   # exit 1
--- FAIL: TestClaudeInvocationNamesTheExecutableItsProbeFound (0.00s)
    continue_claude_test.go:471: arguments name "claude"/"claude", want the probed executable "/tmp/TestClaudeInvocationNamesTheExecutableItsProbeFound2665719200/001/ccc tools/claude"
    continue_claude_test.go:478: the printed exec line "cd /home/user/code/project && CLAUDE_CONFIG_DIR='/tmp/ccc home' claude -p --output-format json --resume 8f1a1d2c-0000-4000-8000-000000000001 -- '<prompt>'" does not name the probed executable in the session's directory
FAIL
FAIL	go.kenn.io/agentsview/internal/ccc	0.023s
```

Both assertion lines fire; the repaired tree's own run of the same command is `ok … 0.021s`, exit **0**.

**Re-runs on this tree** — the case, the whole Go suite in two host shapes, and the gate:

```console
$ go test -count=1 -tags fts5 -run TestClaudeInvocationNamesTheExecutableItsProbeFound ./internal/ccc/
ok  	go.kenn.io/agentsview/internal/ccc	0.021s
$ go test -count=1 -tags fts5 ./internal/ccc/... ./cmd/ccc/...
ok  	go.kenn.io/agentsview/internal/ccc	10.806s
ok  	go.kenn.io/agentsview/internal/ccc/parity	2.152s
ok  	go.kenn.io/agentsview/cmd/ccc	3.527s
$ TMPDIR='/tmp/ccc win' go test -count=1 -tags fts5 ./internal/ccc/... ./cmd/ccc/...
ok  	go.kenn.io/agentsview/internal/ccc	10.152s
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.641s
ok  	go.kenn.io/agentsview/cmd/ccc	3.120s
```

each exit code **0**.

```console
$ python3 scripts/gate.py
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 190 tests in 5.024s (rc=0)
  PASS     parity   … owned: 28 project-owned prefix(es); files seen by git: 5644 | …
  PASS     secrets  no leaks found in 95 file(s) this repository owns …
GATE: PASS
```

exit code **0**. This block is the last gate run on this tree; the only edit after it is this section's own
text (`docs/harness.md`'s bullet was written before it), and no gate step reads it — the `selftest` step's
evidence case reads the `*continuation*.md` records' §5 tables, and this section is §6. No count moves:
`gofmt` 3591, the three packages, `selftest` 190, `files seen by git` 5644 and `secrets` 95 are §6.5's
numbers, because no case was added, removed, skipped or weakened — one fixture and its comment moved.

`docs/harness.md` records this answer under the platform boundary, as it does the platform answers before
it: it is a rule a case's author has to follow, not this round's history. `AGENTS.md` needed no change:
no command, gate step, layout or read order moved, and the rule the case broke is one that file already
states — no case may hold only on the host that runs it. `docs/goals/README.md` and
`docs/goals/archive/` are unchanged: 0003 is still this increment's open goal, awaiting operator
acceptance, so the bookkeeping rule has nothing to move. No owner-only question arose — the repair is
the default Q-0002-13 already records (the case moves to the shape that holds on every host) rather than
a decision — and no gate rule moved (`scripts/gate.py`, `scripts/upstream-parity.py`,
`scripts/gate-required.txt`, `scripts/gate-goflags.txt` and the workflow are untouched), so no
`goals/0003/ci-fix-declined-1.md` was written.

### 6.7 The fix round (review round 3's three findings)

Review round 3 read `8e1c5026…93180b3a` and found three defects. Each was reproduced on this tree before
it was fixed, and each changed behaviour, a case or a document — none is a comment. No `Done when` clause
moved. The round's instructions also asked for a machine-checked pin per rule: every rule this round
changed or added was broken on this tree, the case that names it run, and the tree restored (the
mutations and restores are tabulated in the obligation matrix's round-3 sweep; the failures are quoted
below).

**(1) A recorded tool input that is not a JSON object was written through.** `compactJSON` answered the
compacted bytes of every *valid JSON value*, while the target's `tool_use` block carries an object — and
the writer's own `empty_tool_inputs` detail already promised the fallback for "recorded one that is not a
JSON object". A Codex custom-tool input of `[1,2]`, `42`, `"ls"` or `null` therefore reached the file as
that value with the count still zero. The reading now refuses any text whose first non-space byte is not
`{` (a valid JSON text starting with `{` is an object, so the shape test needs no parse of its own), and
the caller's empty-object fallback applies and is counted. Reproduced by removing the shape test again;
the new case fails:

```console
$ go test -count=1 -tags fts5 -run TestClaudeWriterHoldsOnlyObjectToolInputs ./internal/ccc/   # exit 1
--- FAIL: TestClaudeWriterHoldsOnlyObjectToolInputs (0.00s)
    claude_target_test.go:457: empty_tool_inputs = 2, want 6: every recorded input that is not an object takes the fallback, not only an absent one
    claude_target_test.go:469: call toolu_array holds input []interface {}{1, 2}, want a JSON object: the target's block carries no other shape
    claude_target_test.go:469: call toolu_number holds input 42, want a JSON object: the target's block carries no other shape
    claude_target_test.go:469: call toolu_string holds input "ls", want a JSON object: the target's block carries no other shape
    claude_target_test.go:469: call toolu_null holds input <nil>, want a JSON object: the target's block carries no other shape
    claude_target_test.go:477: the transcript holds 3 tool_use block(s), want the document's 7 calls
FAIL
FAIL	go.kenn.io/agentsview/internal/ccc	0.026s
FAIL
```

`TestClaudeWriterHoldsOnlyObjectToolInputs` covers the scalar, array, string, null, whitespace-only and
unparsed shapes beside one recorded object — whose own spacing the file and the item list both hold
compacted — and reads every `tool_use` block back as an object. The repaired tree's own run of the same
selector is `ok … 0.022s`, exit **0**.

**(2) A generated call id was keyed by the message's ordinal, which two messages can share.** The
review's scenario is an embedding caller that never assigned ordinals: two assistant messages, each with
one id-less call, both received `toolu_ccc_0_0`, `pendingCalls` collapsed the two calls into one entry,
and a following id-less result paired with "the" outstanding call while two were. The writer already
tracks the message's place in the document for its shared message ids; a generated call id is now keyed
by that same place and the call's index in the message (`toolCall`), so `toolu_ccc_0_0` and
`toolu_ccc_1_0` are two calls and the pairing rule still refuses the guess.

**The same probe found the identical defect in the sibling Codex writer**, which this increment's diff
also changes: `codexRolloutBuilder.toolCall` was called with `message.Ordinal` and its generated ids
collapsed the same way. A scratch case — two ordinal-less messages with one id-less call each, then one
id-less result, converted through both writers before either line moved — printed

```text
claude call id "toolu_ccc_0_0"
claude call id "toolu_ccc_1_0"
claude generated_call_ids=2 unpaired=1 results=0
codex call id "call_ccc_0_0"
codex call id "call_ccc_0_0"
codex call id "call_ccc_0_0"
codex generated_call_ids=3 unpaired=0 results=1
```

(the scratch file was deleted after the run; the re-runnable form of the same reproduction is the
reviewed line's restoration below). The Codex writer now keeps the message's place in its walk for the
same reason, and both writers' pins live in their existing pairing cases
(`TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall`,
`TestCodexRolloutPairsAnIdLessResultWithItsCall`), whose new arm asserts the distinct ids, the refused
pairing, `unpaired_tool_results = 1` and no inferred pairing in `generated_call_ids`.

Restoring the reviewed line in each writer (`message.Ordinal` again) fires each writer's own case:

```console
$ go test -count=1 -tags fts5 -run TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall ./internal/ccc/   # exit 1
--- FAIL: TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall (0.00s)
    claude_target_test.go:709: unpaired_tool_results = 0, want 1: two calls in two messages were outstanding
    claude_target_test.go:712: generated_call_ids = 3, want 2: the two calls, with no pairing inferred
    claude_target_test.go:715: carried 1 result(s), want none: the id-less result could belong to either call
    claude_target_test.go:728: the two messages' id-less calls carry ids ["toolu_ccc_0_0" "toolu_ccc_0_0"], want one distinct id per call
FAIL
FAIL	go.kenn.io/agentsview/internal/ccc	0.022s
FAIL

$ go test -count=1 -tags fts5 -run TestCodexRolloutPairsAnIdLessResultWithItsCall ./internal/ccc/   # exit 1
--- FAIL: TestCodexRolloutPairsAnIdLessResultWithItsCall (0.00s)
    codex_target_test.go:689: carried 1 result(s) for two id-less calls in two messages, which is a pairing the source did not record
    codex_target_test.go:693: unpaired_tool_results = {Category:unpaired_tool_results Disposition:omitted Count:0 …}, want the ambiguous result counted
    codex_target_test.go:697: generated_call_ids = {Category:generated_call_ids Disposition:carried-with-caveat Count:3 …}, want the two calls and no inferred pairing
    codex_target_test.go:707: the two messages' id-less calls carry ids ["call_ccc_0_0" "call_ccc_0_0"], want one distinct id per call
FAIL
FAIL	go.kenn.io/agentsview/internal/ccc	0.022s
FAIL
```

Both writers' reviewed lines were put back after the runs, and each file's `git diff` was read to hold
exactly the round's intended change; the repaired tree's own run of the two cases under one selector is
`ok … 0.025s`, exit **0**.

**(3) The evidence case passed when its subject disappeared.** The increment had replaced the shipped
record's required nonempty answer claims (`assertTrue(claims)`) with `if not claims: continue`, so a
route record that lost its §5 comparison — the exact condition the case exists to reject — read as a
pass. The rule now lives where the finding put it: `ROUTE_DOCUMENTS` names the records that report a
converted route and the native turn that resumed it, the case asserts each of them is among the globbed
`*continuation*.md` documents (a rename or a lost glob fails rather than retires the requirement), and
`comparison_violations` reports a route record whose §5 credits no answer. A new case deletes §5's table
rows from each named record's own bytes (`comparison_removed`) and requires that reading to report the
loss.

Two mutations, both restored: the vacuity branch removed from `comparison_violations`, and — the
finding's own scenario — the real `docs/evidence/0003-codex-to-claude-continuation.md` with its §5 table
rows deleted in place (backup taken first; restored byte-for-byte afterwards, its sha256
`cfa1249a0fe54a5937a9a66a9719a8747feca23ea8b76f7546174266d09744a9` and its git state unchanged):

```console
$ python3 -m unittest scripts.tests.test_evidence_claims   # vacuity branch removed: exit 1
FAIL: test_a_route_record_whose_comparison_lost_its_table_is_reported
AssertionError: 0 != 1 : 0002-claude-to-codex-continuation.md: removing §5's table left the rule reporting [], want the lost answer claim reported
Ran 3 tests in 0.009s
FAILED (failures=1)

$ python3 -m unittest scripts.tests.test_evidence_claims   # the 0003 record's §5 table deleted on disk: exit 1
FF.
FAIL: test_a_route_record_whose_comparison_lost_its_table_is_reported
AssertionError: Lists differ: ['a route record's §5 credits no answer w[174 chars]tes'] != []
… : 0003-codex-to-claude-continuation.md: the shipped record does not pass the reading this case mutates
FAIL: test_every_shipped_comparison_only_quotes_its_recorded_run
AssertionError: Lists differ: ['a route record's §5 credits no answer w[174 chars]tes'] != []
… : 0003-codex-to-claude-continuation.md: a route record's §5 credits no answer with an inline-code quotation (2 recorded answer block(s) …): the comparison lost the run it cites
Ran 3 tests in 0.011s
FAILED (failures=2)
```

(the second run fired both the shipped reading and the new case's own pre-mutation check on that file,
which is why two failures were reported; the record was restored from its backup afterwards, byte for
byte, and `git status` shows it unmodified). The repaired tree's run of the module is
`Ran 3 tests in 0.011s … OK`, exit **0**.

**Native re-measure on this tree.** The Claude writer is a measured consumer, so its check was re-run
against the installed harness; the conversion's own numbers are §6.1's exactly (3674 bytes, the same
three non-zero loss categories, no `generated_call_ids` and no `empty_tool_inputs`, because the fixture
records both a call id and an object input):

```console
$ python3 scripts/claude-native-check.py --claude …/claude-cli/node_modules/.bin/claude   # exit 0
…
PASS: the installed file is one JSON object per line — 3674 bytes (the target's own read of it is the load check)
PASS: the target harness's own loader resolves the installed session — verified — … the same command with an unknown session id answered "No conversation found with session ID: …"
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
PASS: the source session is unchanged — sha256 dff6ad015aeb7363… (the scratch source root is identical)
13 check(s) passed, 0 failed, 1 skipped
```

The turn arm was run as well and answered the documented no-model-access boundary — exit **1**, with the
harness's own words (`13 check(s) passed, 1 failed, 0 skipped`; the failing check is `a real turn
continues from the installed session`, whose comparison needs an answer the harness cannot produce
without a provider). The Codex writer was re-measured the same way on its own route; it ran the offline
half to completion and the turn arm was **not** attempted, because this host's Codex has credentials
configured (`~/.codex/auth.json`) and the round's default verification level never bills a model turn:

```console
$ python3 scripts/codex-native-check.py   # exit 0
PASS: the target harness's own loader opens the installed session — thread/resume -> … [legacy] cwd=/home/user/code/example-api
PASS: the installed file is one JSON object per line — 3928 bytes (the target's own read of it is the load check)
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
12 check(s) passed, 0 failed, 1 skipped
```

**Re-runs and the gate.** The whole Go suite ran twice — once with `TMPDIR` pointed at a directory whose
name holds a space (the shell-unsafe root §6.6 exercises the quoting rules with, created first) and once
as itself — and the gate ran last:

```console
$ TMPDIR='/tmp/ccc win' go test -count=1 -tags fts5 ./cmd/ccc/... ./internal/ccc/...   # exit 0
ok  	go.kenn.io/agentsview/cmd/ccc	2.593s
ok  	go.kenn.io/agentsview/internal/ccc	9.682s
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.058s

$ go test -count=1 -tags fts5 ./cmd/ccc/... ./internal/ccc/...   # exit 0
ok  	go.kenn.io/agentsview/cmd/ccc	2.816s
ok  	go.kenn.io/agentsview/internal/ccc	10.301s
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.405s

$ python3 scripts/gate.py   # exit 0
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 191 tests in 4.991s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision … | owned: 28 project-owned prefix(es); files seen by git: 5644 | …
  PASS     secrets  no leaks found in 95 file(s) this repository owns …
GATE: PASS
```

Counts that moved, and why: Go top-level cases **206 → 207** (one new case,
`TestClaudeWriterHoldsOnlyObjectToolInputs`; the two pairing cases gained arms and kept their names), and
`selftest` **190 → 191** (one new offline case, the §5-table removal). No case was deleted, skipped,
weakened or renamed; `gofmt` 3591, the three packages, `carried` 5553, `owned` 28, `files seen by git`
5644 and `secrets` 95 are unchanged from §6.6.

**Nothing else moved.** `docs/harness.md`, `README.md`, `AGENTS.md` and the gate files are untouched: no
command, gate step, layout, read order or agent rule changed — the changed rules are the writer's own and
the evidence case's, and both are described where they live (the writer's comments, the record's §5
disclosure and the obligation matrix's sweep). `docs/goals/README.md` and `docs/goals/archive/` are
unchanged too: 0003 is still this increment's open goal, awaiting operator acceptance. No owner-only
question arose — the one judgement this round took alone (a generated id is keyed by the call's place in
the document, and the sibling writer's identical defect is fixed with it) is an implementation default
rather than a decision the owner must make, and the fix's reversibility is the mutation recorded above.

That gate block is the last gate run on this tree: the only edits after it are this section's own closing
lines and the obligation matrix's round-3 wording, which no gate step reads. The one step that does read
these records — the `selftest` step's evidence case, over the `*continuation*.md` records' §5 tables and
their answer blocks — was re-run on the file as it then stood: `Ran 191 tests in 5.073s`, exit 0.

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
  platform-aware branch 0002's installer test uses. The native check's owner-only check takes the same
  shape: on a host that reports no POSIX mode bits it is reported as a **skip** with that reason, never
  as a failure, so a green run on such a host cannot come from an assertion that could not be made.
- **What this increment did not do:** no per-clause mutation sweep (0002's matrix accumulated one during
  its acceptance rounds; the review stage that follows this increment owns it), and no re-measurement of
  0002's Codex route beyond re-running its tests (§6.2 names the two lines that named the newly
  supported target and were updated; no assertion was weakened).
