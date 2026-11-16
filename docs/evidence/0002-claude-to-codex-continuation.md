# Evidence — 0002 Continue a Claude Code session in Codex

Contract: [`docs/goals/0002-claude-to-codex-continuation.md`](../goals/0002-claude-to-codex-continuation.md).
Plan: [`docs/plans/0002-claude-to-codex-continuation.md`](../plans/0002-claude-to-codex-continuation.md).
Every command below was run on the branch, and every block is its real output, with the repository root
rewritten to `<repo>`, the user's home to `<home>` and the throw-away directories to `<tmp>` so no host
path is committed.

Host: linux/amd64, 4 cores, 7 GB RAM. Toolchain: `go1.27.4`, `python3 3.15.2`.
Pinned revision: `83195c8a6861b8c74a4b6cb87512817fed6f8584` (`v0.47.0-181-g83195c8a`).
Target harness: `codex-cli 0.169.0` (`<home>/.local/share/mise/installs/codex/latest/bin/codex`), with
this host's own `config.toml` and a model provider this host's environment can actually call.

The two committed Claude fixtures this increment converts, and their digests before and after every run
below (the same digests; §4 shows the checks that compare them):

```
$ sha256sum internal/ccc/testdata/fixtures/claude/*/*.jsonl internal/ccc/testdata/fixtures/codex/2026/05/04/*.jsonl
2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6  internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
6dd85d06444bb1483f21519a014e803b8c10f03896372a7040834cdb35ffc31a  internal/ccc/testdata/fixtures/claude/-home-user-code-tooling/b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef.jsonl
d9957a8d6f5731a3df9ce1f175ffe9e228c71f041cd765b03f743852aad1d68b  internal/ccc/testdata/fixtures/codex/2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
```

The first fixture is the one goal 0001 recorded (dialogue, a tool call with its result, a compaction
boundary); the second is new here (`internal/ccc/fixturegen`) and carries the shapes a conversion has to
report: a persisted reasoning trace beside its dialogue and one recorded alone (the provider projects an
empty dialogue for it while its `Content` renders the trace), three tool calls whose results arrive as
their own user lines — one marked `is_error`, one whose recorded text is itself a JSON string — a
subagent turn marked `isSidechain`, and a provider system record.
`TestFixturesMatchTheirGenerator` re-runs the generator and requires the committed bytes.

## 1. One documented, non-interactive command, with the native identity and resume command

```
$ CLAUDE_PROJECTS_DIR=<repo>/internal/ccc/testdata/fixtures/claude \
    <repo>/bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex --target-home <tmp>/home
exit 0
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → codex
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  converted:    codex-rollout-jsonl, 3127 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: provider_binding ×1
  carried-with-caveat: pre_compaction_history ×3
  installed:    <tmp>/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e21f-93c8-7115-87f4-eb65fc09efc6.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e21f-93c8-7115-87f4-eb65fc09efc6
  working dir:  /home/user/code/example-api
  resume:       CODEX_HOME=<tmp>/home codex resume 01a1e21f-93c8-7115-87f4-eb65fc09efc6
  non-interactive resume: CODEX_HOME=<tmp>/home codex exec resume 01a1e21f-93c8-7115-87f4-eb65fc09efc6 '<prompt>'
  cli_version:  0.169.0 (probe)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  verified (the target harness's own loader (codex app-server, thread/resume))
                the harness's loader answered with this session's own id and path; opening a session makes the harness record its own settings in it (measured: one thread_settings_applied record), and no session other than the one it opened is touched
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                not requested (--verify load)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

Both printed commands select the data directory the session was installed into, and they are the shell
rendering of what the JSON reports separately (`target_session.resume_arguments`,
`exec_resume_arguments`, `resume_environment`): Codex takes its data directory from `CODEX_HOME` alone —
it has no flag for it — so a command that named only the binary and the session id would open the
caller's own default home instead. The JSON document of the same run (`--json`, same fixture) carries:

```
"resume_arguments": ["codex", "resume", "<target id>"],
"exec_resume_arguments": ["codex", "exec", "resume", "<target id>", "<prompt>"],
"resume_environment": {"CODEX_HOME": "<tmp>/home"},
"resume_command": "CODEX_HOME=<tmp>/home codex resume <target id>",
"exec_resume_command": "CODEX_HOME=<tmp>/home codex exec resume <target id> '<prompt>'"
```

A word that needs quoting is quoted with POSIX rules — `cmd/ccc/main_test.go` and
`internal/ccc/continue_test.go` pin a home and a binary path holding spaces — and the same rendering is
what a resume verification reports for the command it ran
(`continue_test.go`'s `TestResumeVerificationCommandNamesTheHomeItRanAgainst`).

`ccc help` explains that invocation without any other documentation (`ccc continue` is its first usage
line and the example above is spelled out in it), and `ccc continue -h` prints the one-line usage
(`exit 0`, stdout). The written file's first line is the record the target's reader requires:

```
{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"id":"01a1e21f-93c8-7115-87f4-eb65fc09efc6","timestamp":"2026-05-04T09:15:00Z","cwd":"/home/user/code/example-api","originator":"ccc","cli_version":"0.169.0","source":"cli","thread_source":"user","model_provider":"openai"}}
```

Exit codes observed on this tree: the conversions in §1–§3 `0`; an unknown target (`--to gemini`) `2`; a
resume verification without a prompt `2`; a target data directory nobody named (`CODEX_HOME` pointing at
a path that is not there) `1` with

```
ccc: the target harness has no data directory at <tmp>/absent: pass --target-home (or set CODEX_HOME)
to say where that harness keeps its sessions
```

## 2. Conversion, installation and native verification are separate, machine-readable states

The same invocation with `--json` and a resume verification, in a data directory whose configuration
names a provider this host can call (`<tmp>/home6`, whose `config.toml` is a copy of the host's with two
top-level keys written into it; the host's own file, credentials and sessions were not touched):

```
$ <repo>/bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex \
    --target-home <tmp>/home7 --verify resume \
    --resume-prompt "Answer only from this conversation: which file did you read, and what was the attempt limit?" --json
exit 0
conversion.carried:     {"messages": 5, "user_messages": 2, "assistant_messages": 3, "tool_calls": 1,
                         "tool_results": 1, "compaction_boundaries": 1, "turns": 2, "bytes": 3136}
installation:           {"state": "installed", "path": "<tmp>/home7/sessions/2026/05/04/rollout-2026-05-04T09-15-00-<id>.jsonl",
                         "mechanism": "hard-link", "created": true, "source_unchanged": true,
                         "detail": "sha256 2bd510821d4abf83…, taken before and after the conversion"}
verification.load:      "verified" (the target harness's own loader (codex app-server, thread/resume))
verification.resume:    "verified" (exit_code 0, session_bytes_before 3970, session_bytes_after 73562)
verification.resume.command:
                        CODEX_HOME=<tmp>/home7 <codex> exec resume <id> 'Answer only from this conversation:
                        which file did you read, and what was the attempt limit?' --skip-git-repo-check
                        -c 'sandbox_mode="read-only"' -c 'approval_policy="never"'
target_session:         model_provider "tencent-litellm" (source "target-thread-defaults"), cli_version "0.169.0"
answer:                 From this conversation: the file summarized was `internal/retry/policy.go` (with the
                        backoff cap in `internal/retry/config.go`), and the attempt limit was four. That's all
                        the conversation states — no file contents or tool output appear here.
```

The three blocks a consumer reads apart: `conversion.state` (`converted`), `installation.state`
(`installed`), `verification.load`/`verification.resume` (`verified` / `not-verified` /
`not-requested`), each with its own `method` and `detail`, and `conversion.losses[]` naming every
omitted or narrowed category with its count and reason (seventeen categories, always present, including the
zero ones). The CLI's own tests pin each state's wording, the `--json` document's shape and that
diagnostics stay on stderr (`cmd/ccc/main_test.go`).

## 3. Codex loads the installed session and continues from it

`scripts/codex-native-check.py` re-derives all of it; the blocks below are two real runs of this tree
(target-harness diagnostics that are not verdicts — its sandbox probe and PATH-alias warnings — are
dropped here, and the script's own `NOISE` filter keeps them out of its summaries). Every run mints its
own session identity, and the resumed turn's own words and byte counts differ per run, so the ids and
numbers below are that one run's. The read-back line in each block is the script's own `ccc inspect`
invocation reading the installed file, not the conversion's verdict about itself (the gate holds the
core's round-trip comparison down separately, in `TestStructureVerificationNamesWhatTheFileDoesNotHold`).
Both fixtures, with a real turn each:

```
$ python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which file did you read, and what was the attempt limit?"
exit 0
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 1 tool call(s), 17 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e225-a986-7372-bf0f-2c51da684752.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e225-a986-7372-bf0f-2c51da684752, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 3970 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e225-a986-7372-bf0f-2c51da684752: {"messages": 7, "user_messages": 2, "assistant_messages": 5, "system_messages": 0, "tool_messages": 0, "system_records": 1, "sidechain_messages": 0, "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1}; conversion carried: 2 user message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies)
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e225-a986-7372-bf0f-2c51da684752 [legacy] cwd=/home/user/code/example-api
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e225-a986-7372-bf0f-2c51da684752' environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 70197 bytes
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

The target's own view of that installed session (`codex app-server` → `thread/read`, printed by the
harness) and the answer its own resume produced:

```
the target harness's own view of the installed session:
    thread 01a1e225-a986-7372-bf0f-2c51da684752 [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: I will read the retry policy first.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
      turn rollout-11
        userMessage: Now check where the backoff cap is configured.
        agentMessage: The cap is configured in internal/retry/config.go as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - File read: none — no files were actually read in this conversation (only file names were mentioned: `internal/retry/policy.go` and `internal/retry/config.go`).
    - Attempt limit: four.
```

Why the first fixture's answer has this shape (its own wording varies run to run): the fixture compacts
after its tool call, the conversion carries that boundary as the target's `compacted` record, and Codex
applies that record to its own model-visible history — so the resumed turn continues from the source's
own active context (the summary and the turns after the boundary), which is why this run could quote the
attempt limit and the summarized file from the summary's own words and the cap's file from the
post-boundary turn, while answering that no tool output appears in the conversation it continues — the
file keeps the turns before the boundary, but the target does not continue from them. An earlier run of
this same check, over a rollout written without the target's `event_msg` message records, could not see
even the post-boundary turns and answered that no tool call exists in this conversation; that is what
writing both record kinds buys. The second fixture has no boundary, and there the tool evidence reaches
the resumed turn:

```
$ python3 scripts/codex-native-check.py --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef \
    --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which tools appear, what did the first return, and what did the last one return?"
exit 0
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 3 tool call(s), 17 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/05/rollout-2026-05-05T10-00-00-01a1e225-b8ec-730d-bcbe-952e42cdf87a.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e225-b8ec-730d-bcbe-952e42cdf87a, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 4611 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e225-b8ec-730d-bcbe-952e42cdf87a: {"messages": 8, "user_messages": 2, "assistant_messages": 6, "system_messages": 0, "tool_messages": 0, "system_records": 0, "sidechain_messages": 0, "compact_boundaries": 0, "tool_calls": 3, "tool_results": 3}; conversion carried: 2 user message(s), 3 tool call(s), 3 tool result(s), 0 boundary(ies)
PASS: the source session is unchanged — sha256 6dd85d06444bb148… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e225-b8ec-730d-bcbe-952e42cdf87a [legacy] cwd=/home/user/code/tooling
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e225-b8ec-730d-bcbe-952e42cdf87a' environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 4611 -> 71230 bytes
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

```
the target harness's own view of the installed session:
    thread 01a1e225-b8ec-730d-bcbe-952e42cdf87a [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Check the retry policy and tell me the attempt limit.
        agentMessage: I will read the policy file first.
        agentMessage: The attempt limit is four.
      turn rollout-12
        userMessage: Then name the file that holds the cap.
        agentMessage: internal/retry/config.go, as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - Tools that appear: `Read` and `Bash`.
    - First (`Read` on `/home/user/code/tooling/internal/retry/policy.go`) returned `package retry` and `const MaxAttempts = 4`.
    - Last (`Bash` running `python3 -c 'import json; print(json.dumps("maxBackoffSeconds"))'`) returned `"maxBackoffSeconds"`.
```

All three tool calls, the first result's content, the second result's recorded *failure* and the third
result's text reached the resumed turn through the conversion — written as
`function_call`/`function_call_output` records with the source's own tool names, its recorded arguments
verbatim and the result text decoded from the source's own content field. The third result is the one
that shows the decode happens once: the tool printed a JSON value, the source recorded the text
`"maxBackoffSeconds"` — quotation marks included — and the target's own answer quotes it in that
spelling, where a second decode would have dropped the quotes.

**Limit, stated rather than papered over:** `codex resume` (the interactive TUI) could not be driven on
this host — no TTY is available (`ERROR: TERM is set to "dumb"…`), and the harness does not fake one.
Its loader is the same `thread/resume` request the app server answered above, which is what the `load`
verification exercises; the *interactive* surface itself is unverified here.

**What the verification touches in the target home** (measured on isolated homes, one step at a time):

| Step | `state_5.sqlite` `threads` rows | The converted session file |
| --- | --- | --- |
| the install alone (`--verify none`) | 0 (the provider is asked for with `thread/start` on an *ephemeral* thread, which registers nothing) | 3136 bytes — exactly the byte count the conversion reports |
| after the harness's loader opened it (`thread/resume`, the `load` verification) | 1: the converted session's id and provider | 3970 bytes: the harness appends its own `thread_settings_applied` record when a client opens a session, which is what `codex resume` does to any session it opens |
| after a real turn (`codex exec resume`) | unchanged | grows again with the turn's own records (§3) |

Nothing else moves: the commit compares the source fixture tree and the unrelated session it plants in
the target home before and after every run, and the recorded `source_unchanged` flag covers the file the
conversation was read from. The appended record is the *target's* own bookkeeping about a session it
opened, written after the conversion's structure check has already compared the file it wrote, and it is
valid to the inherited reader (it is an `event_msg` whose payload is the harness's settings).

## 4. The source is unchanged, the identity is new, no other session moves

- The two halves of each run above: `the source session is unchanged — … (fixture tree identical)` and
  `the unrelated session in the target home is unchanged`, plus `no pre-existing session in the target
  home changed` and `source_unchanged: true` in the JSON.
  The harness digests every file below the fixture root, the unrelated session it plants in the
  target home and every session file the home already held, before the conversion, and compares them
  after it (the planted file is removed again when the run ends);
  `TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures`
  does the same in-process over the whole fixture tree.
- New identity: the target id is a UUID v7 that is neither the source's session id nor any id already in
  the home (`TestCodexRolloutGeneratesAFreshIdentity`,
  `TestContinueConvertsInstallsAndReadsBackTheFixture`), and the install itself is exclusive:
  `TestInstallCodexRolloutNeverReplacesASession` writes a session and then requires the second install at
  the same path to fail with `ErrTargetExists` while the file keeps the first write's digest. The
  mechanism the run reports (`hard-link`) is an exclusive link. There is no other mechanism: a
  filesystem without hard links cannot take a name without replacing what is at it, so the conversion is
  refused there with `ErrNoExclusiveInstall` rather than published by a rename
  (`TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks`; the default and its reversal are recorded
  as `Q-0002-12`).
- The source session is never opened for writing: the reader is the inherited one (read-only), the
  archive path is opened read-only when consulted, and the pipeline's only write is the target file.

## 5. A representative transcript keeps its supported semantics; the rest is reported

Source facts for the tool-heavy fixture (`ccc inspect … --json`, unchanged from goal 0001's command):

```
counts:  {"messages": 13, "user_messages": 5, "assistant_messages": 8, "system_messages": 0, "tool_messages": 0,
          "system_records": 0, "sidechain_messages": 1, "compact_boundaries": 0, "tool_calls": 3, "tool_results": 3}
tools:   ["Bash", "Read"]
```

The target view the table reads is the tooling fixture's, printed by the same harness run as §3's second
half:

```
the target harness's own view of the installed session:
    thread 01a1e225-b8ec-730d-bcbe-952e42cdf87a [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Check the retry policy and tell me the attempt limit.
        agentMessage: I will read the policy file first.
        agentMessage: The attempt limit is four.
      turn rollout-12
        userMessage: Then name the file that holds the cap.
        agentMessage: internal/retry/config.go, as maxBackoffSeconds.
```

| Source evidence | Target view (the harness's own `thread/read`, above) | Reported as |
| --- | --- | --- |
| user "Check the retry policy and tell me the attempt limit." | `userMessage: Check the retry policy and tell me the attempt limit.` | carried |
| assistant "I will read the policy file first." (with a persisted reasoning trace) | `agentMessage: I will read the policy file first.` | carried; the reasoning trace omitted (`reasoning_text ×2`, both traces) |
| the reasoning-only turn (the provider projects an empty dialogue; `Content` renders the trace) | absent, and the trace's own words appear nowhere in the file | omitted (`reasoning_text ×2`); never written as the agent's dialogue |
| `Read` call + its result ("package retry … MaxAttempts = 4") | reached the resumed turn ("First (`Read` on `/home/user/code/tooling/internal/retry/policy.go`) returned `package retry` and `const MaxAttempts = 4`.") and the file holds the pair | carried (`tool_calls ×3`, `tool_results ×3`) |
| `Bash` call whose result the source marked `is_error` | the file holds the pair, and this run's answer names `Bash` among the tools that appear (its wording does not quote the failure) | carried; the recorded outcome has no target field (`tool_result_outcomes ×1`) |
| `Bash` call whose result text is a JSON string (`"maxBackoffSeconds"`) | reached the resumed turn as that text, quotation marks included ("Last (`Bash` running `python3 -c 'import json; print(json.dumps("maxBackoffSeconds"))'`) returned `"maxBackoffSeconds"`.") | carried, decoded exactly once |
| assistant "The attempt limit is four." | `agentMessage: The attempt limit is four.` | carried |
| a subagent turn marked `isSidechain` | absent | omitted (`sidechain_messages ×1`) |
| the provider's `system` record | absent (the inherited reader drops it before the document, too) | `system_records ×0` |
| tool names `Read`/`Bash`, arguments verbatim | the calls are in the file with those names and inputs; the target's own tool set is Codex's | carried with caveat (`tool_name_vocabulary ×2`) |
| the session's provider | recorded as the target's own effective provider | carried with caveat (`provider_binding ×1`) |

Everything a conversion cannot carry is named by category with a count and a reason on *every* run
(`internal/ccc/codex_target_test.go`'s `TestCodexRolloutReportsEveryCategoryItKnows` requires all
seventeen categories to be present and to be truthful — the omitted text is not in the file — and
`TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` covers the archive-read path, where the report
also says the evidence came from a projection; `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce`
covers the archive's other half, where one recorded outcome is exposed as both an event and a restored
summary and must be written once).

## 6. Tests, gate, and what each pins

```
$ CGO_ENABLED=1 go test -tags fts5 -count=1 ./cmd/ccc/... ./internal/ccc/...     # rc=0
ok  	go.kenn.io/agentsview/cmd/ccc	2.285s
ok  	go.kenn.io/agentsview/internal/ccc	10.984s
?   	go.kenn.io/agentsview/internal/ccc/fixturegen	[no test files]
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.963s
?   	go.kenn.io/agentsview/internal/ccc/parity/oracle	[no test files]
```

| Rule | Test |
| --- | --- |
| the written record shape, order and fields (session_meta, turn_context, response_item, event_msg, function_call/output, compacted) | `TestCodexRolloutRecordShape` |
| dialogue only (no rendering of calls), calls/results paired by id, boundaries kept with the turns before them | `TestCodexRolloutKeepsTheDialogueProjection`, `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt` |
| a reasoning-only record is counted, never written as dialogue — and the native fixture really has that shape | `TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue`, `TestToolingFixtureCarriesTheShapesTheWriterMustNotMisread` |
| an encoded content field is decoded once, text a read already decoded is not decoded again | `TestCodexRolloutDecodesEncodedContentExactlyOnce` |
| blocks the target's text-only output cannot carry are counted, and a field holding only such blocks is not written as its own JSON | `TestCodexRolloutReportsToolResultBlocksTheTargetCannotCarry` |
| an archive's double exposure of one outcome is written once, a genuinely distinct summary still is | `TestCodexRolloutWritesAnArchiveSummaryOnce`, `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce` |
| every loss category reported, and truly absent from the file | `TestCodexRolloutReportsEveryCategoryItKnows` |
| the tool role is never written as dialogue: a paired output travels as `function_call_output` alone, an orphan one travels nowhere | `TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult` |
| id-less calls and results get paired deterministically, or are reported unpaired — for one, two and three outstanding calls | `TestCodexRolloutPairsAnIdLessResultWithItsCall` |
| the provider binding comes from the caller and is reported | `TestCodexRolloutRecordsTheProviderBindingItIsGiven` |
| fresh UUID v7 identity, one ordered timestamp run | `TestCodexRolloutGeneratesAFreshIdentity`, `TestCodexRolloutStampsRecordsInOneOrderedRun` |
| end-to-end: convert, install, read back through the inherited reader, source and neighbours unchanged | `TestContinueConvertsInstallsAndReadsBackTheFixture` |
| the archive fallback is used and disclosed | `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` |
| the printed resume invocation names the data directory the session was installed into, quoted where a shell would act on a word, and a resume verification reports the command it ran the same way | `TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome`, `TestResumeVerificationCommandNamesTheHomeItRanAgainst`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueJSONDistinguishesConversionInstallationAndVerification` |
| exclusive install (a taken name refused, a filesystem without hard links refused rather than renamed over) and dry run writes nothing (no directory for a home the caller named that is not there yet, and the target harness is never started), dry run reports its own states, prompt required, target/level validation, home resolution | `TestInstallCodexRolloutNeverReplacesASession`, `TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks`, `TestContinueDryRunWritesNothing`, `TestContinueDryRunReportsWhatItVerified`, `TestContinueDryRunInstallsNothingThroughTheCLI`, `TestContinueRequiresAPromptBeforeItRunsATurn`, `TestContinueRefusesAnUnsupportedTargetOrLevel`, `TestContinueTargetHomeResolution` |
| the round-trip check compares rather than agrees | `TestStructureVerificationNamesWhatTheFileDoesNotHold` |
| the source's own digest is taken before the conversion and compared after it; a moved source is warned about; a projection says no file was compared | `TestSourceUnchangedComparesTheFileBeforeAndAfter`, `TestSourceChangedWarningsNameTheMove`, `TestSourceUnchangedSaysWhenThereWasNoFileToCompare` |
| the CLI's three states, human shape, source check, exit codes, help | `TestContinueJSONDistinguishesConversionInstallationAndVerification`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueHumanOutputReportsTheSourceCheck`, `TestContinueUsageErrors`, `TestContinueReportsAFailedVerificationAsAFailure`, `TestContinueRefusesAMissingDataDirectoryNobodyNamed`, `TestContinueHelpIsARequestNotAFailure` |

Gate, run on this tree after the last change (`python3 scripts/gate.py`, exit 0):

```
gate: <repo>
gate: go=<home>/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3580 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 144 tests in 7.323s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 26 project-owned prefix(es); files seen by git: 5623 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 68 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The file this increment adds outside the Go packages, `scripts/codex-native-check.py`, is inside a
prefix the carry manifest calls project-owned (added to `scripts/upstream-import.py`'s `OWNED` and to the
generated manifest, which `scripts/tests/test_upstream_parity.py` requires to agree; the Go files are
under the already-owned `internal/ccc/`). The oracle comparison's cases now name the
fixture each one reads (`internal/ccc/parity`'s `Cases`, mirrored by `scripts/upstream-compare.py`), so
adding a transcript shape cannot silently change what a recorded dump describes. The second Claude
fixture is a new file rather than a change to an existing record, and it is not one of the oracle's
cases: the committed dumps still describe the same two fixtures, they were not re-recorded, and
`internal/ccc/parity`'s offline test still compares them byte for byte. A later review round added two
records to that second fixture — the reasoning-only turn and the tool result whose text is a JSON string
— and it is still not an oracle case: its new digest (`6dd85d06…`, above) is the one both native runs
report as `the source session is unchanged`, and `TestDumpMatchesTheRecordedUpstreamAgreement` (in the
gate's `test` step) still holds the recorded dumps against the unchanged first fixtures.
`scripts/upstream-compare.py` itself was **not** re-run here (it needs network and a pristine
checkout of the pin); its next run is the operator's, and the offline half of the same comparison passes.

## 7. Limits and residual risks

- **The interactive TUI resume is unverified on this host** (no TTY): the loader behind it
  (`thread/resume`) is verified, the picker is not.
- **The app-server protocol is experimental and version-bound.** The verified version is recorded on
  every run and in `CodexFormatVersion`; a protocol change turns `load` into `not-verified` with the
  harness's own message rather than a pass, and a server that accepts a request and then stops
  answering is bounded by `ContinueOptions.Timeout` (60 s per app-server request by default) instead of
  blocking the report. `scripts/codex-native-check.py` makes its own requests with the same rule and its
  own deadline (`APP_SERVER_TIMEOUT`, 120 s), reads the server's stdout on a thread and drains its stderr
  on another, so a server that stalls or floods its diagnostics is reported as a failed check rather
  than hanging the run (proved with a stub server that answers nothing: the call ends at the deadline
  with the stub's diagnostics attached).
- **A filesystem without hard links cannot receive a converted session.** The only exclusive publication
  mechanism the installer has is a hard link; where the filesystem cannot take one, the installation is
  refused with `ErrNoExclusiveInstall` rather than published by a rename that could replace another
  installer's session. The default and its reversal are recorded as `Q-0002-12`.
- **The recorded provider binds the session** to the target's configuration at conversion time; a later
  provider change makes the harness refuse to resume it (its own message names both values). Recorded as
  `Q-0002-01`; the alternative — recording none — is refused outright by a home that has migrated to
  paginated threads, which is what a real home looks like.
- **`migrate-rollouts --apply` is deliberately not run**: on the tested version it failed with
  `missing_sqlite_metadata` for a rollout added to a home that already held a migrated thread; the
  target migrates on its own when it needs to, and the report claims no migration.
- **Tool evidence in the target's own item view**: `thread/read` lists a turn's messages; the calls and
  results are in the session file and in the model-visible history (proved by the resumed turn in §3)
  but not in that view. It is not one of `target_session.format_limits`' categories (those are the
  pair's unrepresentable limits — attachments, the provider and model binding, the paginated thread
  store and replay); the view block in §3, printed from the reply's own item list, is where the limit
  is shown.
- **Attachments are counted only where the read holds them.** A message's own images and attachments are
  not in the source read at all (its reader keeps a message's text), so a per-conversion count of those
  would be a claim the read cannot support and the limit is stated instead; a tool result is the
  exception, because its content field *is* read as the provider recorded it — a block that field
  contains and the target's text-only output has no place for is named by count
  (`unrepresentable_result_blocks`), and the format limit says exactly that.
- **A dry run records no model provider**: a dry run must not write into the target's data directory, so
  it does not start the harness against it (measured: a started Codex writes its databases, logs and
  skills into the home it runs with) — and starting the harness there is the only way to ask which
  provider a new session of it would use. The dry run's conversion therefore carries no
  `model_provider`, its `model_provider_source` is `unrecorded` with that reason, and the run warns about
  it; `Q-0002-11` records the choice.
