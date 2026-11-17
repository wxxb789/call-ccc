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
