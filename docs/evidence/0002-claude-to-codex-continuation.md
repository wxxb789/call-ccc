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
415bbc31a6cd541f9bc1ff2b99dc4532abe81f218e14dc5c97c7399f3b212873  internal/ccc/testdata/fixtures/claude/-home-user-code-tooling/b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef.jsonl
d9957a8d6f5731a3df9ce1f175ffe9e228c71f041cd765b03f743852aad1d68b  internal/ccc/testdata/fixtures/codex/2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
```

The first fixture is the one goal 0001 recorded (dialogue, a tool call with its result, a compaction
boundary); the second is new here (`internal/ccc/fixturegen`) and carries the shapes a conversion has to
report: a persisted reasoning trace beside its dialogue and one recorded alone (the provider projects an
empty dialogue for it while its `Content` renders the trace), three tool calls whose results arrive as
their own user lines — one marked `is_error`, one whose recorded text is itself a JSON string and arrives
on a record the provider wrote as a system reminder (round 4 below) — a subagent turn marked
`isSidechain`, and a provider system record.
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
omitted or narrowed category with its count and reason (twenty-two categories, always present, including
the zero ones — round 3's §8 added `malformed_source_lines`, round 7's §12 the two whitespace-only ones,
round 10's §15 the two the provider's own classification of the read feeds, `summary_transcript` and
`source_needs_retry`; the §1 and §2 blocks here were recorded before those additions and print what the
writer reported then, and §3's blocks are §15's re-run of this tree, printing twenty-two).

The source check is three states of its own: `installation.source_check_state` is `unchanged`, `changed`
or `not-verified` — the comparison did not run, which is what an archive read and a stopped run answer —
with `detail` carrying that answer's own words. Round 5 replaced the `installation.source_unchanged`
boolean the §1/§2 blocks above were recorded with, because a boolean answered "did the file move?" for a
comparison that never ran (§10). The CLI's own tests pin each state's wording, the `--json` document's
shape and that diagnostics stay on stderr (`cmd/ccc/main_test.go`).

## 3. Codex loads the installed session and continues from it

`scripts/codex-native-check.py` re-derives all of it; the blocks below are two real runs of this tree
(target-harness diagnostics that are not verdicts — its sandbox probe and PATH-alias warnings — are
dropped here, and the script's own `NOISE` filter keeps them out of its summaries; the `seeded from:`
line each run prints is kept, because it is what says the throw-away home was built from named settings
rather than from a copy of the host's `config.toml`). Host paths are written the way the harness's
summaries abbreviate them (`<codex>`, `<tmp>/codex-home`, `~/.codex/config.toml`). Every run mints its
own session identity, and the resumed turn's own words and byte counts differ per run, so the ids and
numbers below are that one run's. The read-back line in each block is the script's own `ccc inspect`
invocation reading the installed file, not the conversion's verdict about itself (the gate holds the
core's round-trip comparison down separately, in `TestStructureVerificationNamesWhatTheFileDoesNotHold`).
Both fixtures, with a real turn each:

```
$ python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which file did you read, and what was the attempt limit?"
exit 0
seeded from: ~/.codex/config.toml — copied model, model_provider, [model_providers.tencent-litellm].name, [model_providers.tencent-litellm].base_url, [model_providers.tencent-litellm].wire_api, [model_providers.tencent-litellm].env_key; not copied (the throw-away home is not a copy of the host's configuration: it is seeded with the settings that answer the loader's question, never with one that can hold a credential, so a provider whose credential is written there must take it from the environment): model_reasoning_effort, plan_mode_reasoning_effort, approvals_reviewer, model_post_turn_compact_threshold_percent, model_auto_compact_token_limit, [projects], [features], [tui]
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 1 tool call(s), 22 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e354-eccc-7119-8434-50a197d0b8b6.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e354-eccc-7119-8434-50a197d0b8b6, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 3970 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e354-eccc-7119-8434-50a197d0b8b6: {"messages": 7, "user_messages": 2, "assistant_messages": 5, "system_messages": 0, "tool_messages": 0, "system_records": 1, "sidechain_messages": 0, "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1}; conversion carried: 2 user message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies)
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e354-eccc-7119-8434-50a197d0b8b6 [legacy] cwd=/home/user/code/example-api
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e354-eccc-7119-8434-50a197d0b8b6' exec_resume_command="CODEX_HOME=<tmp>/codex-home <codex> exec resume 01a1e354-eccc-7119-8434-50a197d0b8b6 '<prompt>'" environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 68533 bytes
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

The target's own view of that installed session (`codex app-server` → `thread/read`, printed by the
harness) and the answer its own resume produced:

```
the target harness's own view of the installed session:
    thread 01a1e354-eccc-7119-8434-50a197d0b8b6 [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: I will read the retry policy first.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
      turn rollout-11
        userMessage: Now check where the backoff cap is configured.
        agentMessage: The cap is configured in internal/retry/config.go as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - I did not read any file in this conversation.
    - The attempt limit is four.

    (Note: the earlier replies claiming `internal/retry/policy.go`, `internal/retry/config.go`, and `maxBackoffSeconds` were not backed by any file read here, so those claims are unverified.)
```

Why the first fixture's answer has this shape (its own wording varies run to run): the fixture compacts
after its tool call, the conversion carries that boundary as the target's `compacted` record, and Codex
applies that record to its own model-visible history — so the resumed turn continues from the source's
own active context (the summary and the turns after the boundary). That is why this run could still state
the attempt limit (four, from the summary's own words) while saying it read no file here and that the
replies' claims about the files are unverified: those turns are in the file the conversion wrote, and the
target does not continue from them. An earlier run of this same check, over a rollout written without the
target's `event_msg` message records, could not see even the post-boundary turns and answered that no tool
call exists in this conversation; that is what writing both record kinds buys. The second fixture has no
boundary, and there the tool evidence reaches the resumed turn:

```
$ python3 scripts/codex-native-check.py --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef \
    --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which tools appear, what did the first return, and what did the last one return?"
exit 0
seeded from: ~/.codex/config.toml — copied model, model_provider, [model_providers.tencent-litellm].name, [model_providers.tencent-litellm].base_url, [model_providers.tencent-litellm].wire_api, [model_providers.tencent-litellm].env_key; not copied (the throw-away home is not a copy of the host's configuration: it is seeded with the settings that answer the loader's question, never with one that can hold a credential, so a provider whose credential is written there must take it from the environment): model_reasoning_effort, plan_mode_reasoning_effort, approvals_reviewer, model_post_turn_compact_threshold_percent, model_auto_compact_token_limit, [projects], [features], [tui]
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 3 tool call(s), 22 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/05/rollout-2026-05-05T10-00-00-01a1e355-0846-7016-88ff-a3579a47b8e0.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e355-0846-7016-88ff-a3579a47b8e0, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 4611 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e355-0846-7016-88ff-a3579a47b8e0: {"messages": 8, "user_messages": 2, "assistant_messages": 6, "system_messages": 0, "tool_messages": 0, "system_records": 0, "sidechain_messages": 0, "compact_boundaries": 0, "tool_calls": 3, "tool_results": 3}; conversion carried: 2 user message(s), 3 tool call(s), 3 tool result(s), 0 boundary(ies)
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e355-0846-7016-88ff-a3579a47b8e0 [legacy] cwd=/home/user/code/tooling
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e355-0846-7016-88ff-a3579a47b8e0' exec_resume_command="CODEX_HOME=<tmp>/codex-home <codex> exec resume 01a1e355-0846-7016-88ff-a3579a47b8e0 '<prompt>'" environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 4611 -> 69230 bytes
PASS: the source session is unchanged — sha256 415bbc31a6cd541f… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

```
the target harness's own view of the installed session:
    thread 01a1e355-0846-7016-88ff-a3579a47b8e0 [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Check the retry policy and tell me the attempt limit.
        agentMessage: I will read the policy file first.
        agentMessage: The attempt limit is four.
      turn rollout-12
        userMessage: Then name the file that holds the cap.
        agentMessage: internal/retry/config.go, as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    Tools: `exec_command` (twice, as shell commands).
    First return: `Error: no such file or directory` — the `grep -rn maxBackoffSeconds internal/retry` search found nothing.
    Last return: `"maxBackoffSeconds"` — the Python/json check printed the constant's name.
```

All three recorded tool calls reached the installed session through the conversion — written as
`function_call`/`function_call_output` records with the source's own tool names, its recorded arguments
verbatim and the result text decoded from the source's own content field — and this run's answer quotes
two of their outcomes in its own words: the `grep` call's recorded *failure* and the third call's text
`"maxBackoffSeconds"`. The third result is the one that shows the decode happens once: the tool printed a
JSON value, the source recorded the text `"maxBackoffSeconds"` — quotation marks included — and the
target's own answer quotes it in that spelling, where a second decode would have dropped the quotes. Which
call the answer counts as the first is the model's own reading of the conversation (it names the shell
calls, not the `Read` before them); the records the conversion wrote are what the file holds, and the
gate's round-trip comparison is what checks those.

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
the target home before and after every run, and the recorded `source_check_state` covers the file the
conversation was read from. The appended record is the *target's* own bookkeeping about a session it
opened, written after the conversion's structure check has already compared the file it wrote, and it is
valid to the inherited reader (it is an `event_msg` whose payload is the harness's settings).

## 4. The source is unchanged, the identity is new, no other session moves

- The three halves of each run above: `the source session is unchanged — … (fixture tree identical)`,
  `the unrelated session in the target home is unchanged` and `no pre-existing session in the target
  home changed`, plus `source_check_state: "unchanged"` in the JSON. The harness digests every file below
  the fixture root, the unrelated session it plants in the target home and every session file the home
  already held before the conversion, and compares them **after the loader and the real turn have run**
  (round 6 moved the first two comparisons there; the planted file is removed only once they have run),
  so a mutation during either native stage is answered by the comparison that runs last rather than by an
  earlier verdict standing in for it — pinned offline by
  `scripts/tests/test_codex_native_check.py`'s `ProtectedArtifactsAfterNativeVerification`;
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
          "system_records": 1, "sidechain_messages": 1, "compact_boundaries": 0, "tool_calls": 3, "tool_results": 3}
tools:   ["Bash", "Read"]
```

The target view the table reads is the tooling fixture's, printed by the same harness run as §3's second
half:

```
the target harness's own view of the installed session:
    thread 01a1e355-0846-7016-88ff-a3579a47b8e0 [legacy] provider=tencent-litellm
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
| `Read` call + its result ("package retry … MaxAttempts = 4") | reached the resumed turn (the answer names `Read` and its first return: "the contents of … `package retry` followed by `const MaxAttempts = 4`") and the file holds the pair | carried (`tool_calls ×3`, `tool_results ×3`) |
| `Bash` call whose result the source marked `is_error` | the file holds the pair, and this run's answer names `Bash` among the tools that appear (its wording does not quote the failure) | carried; the recorded outcome has no target field (`tool_result_outcomes ×1`) |
| `Bash` call whose result text is a JSON string (`"maxBackoffSeconds"`), recorded on the same record as a system reminder | reached the resumed turn as that text, quotation marks included (the answer's last return: `"maxBackoffSeconds"`) | carried, decoded exactly once; the record's reminder text is not written (`system_records ×1`, round 4 below) |
| assistant "The attempt limit is four." | `agentMessage: The attempt limit is four.` | carried |
| a subagent turn marked `isSidechain` | absent | omitted (`sidechain_messages ×1`) |
| the provider's `system` record | absent (the inherited reader drops it before the document, too) | not a message the source read produces, so no writer category counts it |
| tool names `Read`/`Bash`, arguments verbatim | the calls are in the file with those names and inputs; the target's own tool set is Codex's | carried with caveat (`tool_name_vocabulary ×2`) |
| the session's provider | recorded as the target's own effective provider | carried with caveat (`provider_binding ×1`) |

Everything a conversion cannot carry is named by category with a count and a reason on *every* run
(`internal/ccc/codex_target_test.go`'s `TestCodexRolloutReportsEveryCategoryItKnows` requires all
twenty-two categories to be present and to be truthful — the omitted text is not in the file — and
`TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` pins the read-completeness one, whose count
is the read's own; `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` covers the archive-read path,
where the report
also says the evidence came from a projection; `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce`
covers the archive's other half, where one recorded outcome is exposed as both an event and a restored
summary and must be written once).

## 6. Tests, gate, and what each pins

```
$ GOFLAGS=-mod=mod CGO_ENABLED=1 go test -tags fts5 -count=1 ./cmd/ccc/... ./internal/ccc/...     # rc=0
ok  	go.kenn.io/agentsview/cmd/ccc	2.827s
ok  	go.kenn.io/agentsview/internal/ccc	10.081s
?   	go.kenn.io/agentsview/internal/ccc/fixturegen	[no test files]
ok  	go.kenn.io/agentsview/internal/ccc/parity	0.838s
?   	go.kenn.io/agentsview/internal/ccc/parity/oracle	[no test files]
```

| Rule | Test |
| --- | --- |
| the written record shape, order and fields (session_meta, turn_context, response_item, event_msg, function_call/output, compacted) | `TestCodexRolloutRecordShape` |
| dialogue only (no rendering of calls), calls/results paired by id, boundaries kept with the turns before them | `TestCodexRolloutKeepsTheDialogueProjection`, `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt` |
| a reasoning-only record is counted, never written as dialogue — and the native fixture really has that shape | `TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue`, `TestToolingFixtureCarriesTheShapesTheWriterMustNotMisread` |
| an encoded content field is decoded once, text a read already decoded is not decoded again | `TestCodexRolloutDecodesEncodedContentExactlyOnce` |
| blocks the target's text-only output cannot carry are counted, and a field holding only such blocks is not written as its own JSON | `TestCodexRolloutReportsToolResultBlocksTheTargetCannotCarry` |
| an archive's summary is derived from the call's own result events, so it is written only for a call that has no recorded result — as is a genuinely summary-only row | `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults`, `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce` |
| a subagent record that also carries the source's compaction marker is omitted as a sidechain message, not applied as the main conversation's `compacted` boundary | `TestCodexRolloutDoesNotApplyASidechainCompactionBoundary` |
| the read's own malformed-line count travels into the loss report, with the unfinished tail named | `TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce`, `TestContinueConvertsInstallsAndReadsBackTheFixture` |
| every loss category reported, and truly absent from the file | `TestCodexRolloutReportsEveryCategoryItKnows` |
| text the target's reader reads as *nothing at all* is never written and is named — a whitespace-only tool result, a whitespace-only dialogue (whose record's tool evidence still travels), and a compact summary the reader trims | `TestContinueOmitsOutputTheInheritedReaderCannotReadBack`, `TestContinueOmitsDialogueTheInheritedReaderReadsAsNothing`, `TestContinueRoundTripsACompactionSummaryTheReaderTrims` |
| text the target's reader *projects away* although it is a real message is carried, is counted, and is never confused with a loss: the check asks the installed file's own dialogue record, the count reaches `verification.structure.not_read_back_verbatim` and a warning, and text the file does not hold stays a missing message — for both shapes of the projection, the envelope alone and the envelope followed by text | `TestContinueInstallsPromptsTheReaderReadsDifferently` (`cmd/ccc`, native Claude transcripts, one per shape), `TestCodexRoundTripCountsWhatTheReaderProjectsAway` |
| a read-back message shows one written message, and the count is read from the read-back's texts rather than from the pairing's answers (round 11): an omitted envelope beside one or two later turns carrying its text counts one message, not two | `TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce`, `TestCodexRoundTripCountsWhatTheReaderProjectsAway` |
| the file is asked for the message's own dialogue record — its record type, the payload's message type, the role, the content block's own type and text — counted so one record answers for one written message, and a read-back message is accounted for only by a written message's own text or by a run of it under that message's own kind, in the file's own order | `TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord`, `TestCodexRoundTripCountsARecordOncePerWrittenMessage`, `TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage`, `TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor`, `TestStructureVerificationAsksTheFileForTheMessageRecord` |
| a difference report identifies its items — category, ordinal, kind, call, tool, size, SHA-256 — and never carries the dialogue, arguments or output it compared | `TestCodexRoundTripReportNamesDifferencesWithoutTheirText`, `TestStructureVerificationNamesWhatTheFileDoesNotHold` |
| the installed file's dialogue records are read in the file's own order (round 10): a record the file holds where another message's would stand is a difference, a record the document does not write is a difference even where the reader projects it away, and the message's own record is asked for even where the read-back shows its text — at the comparison level and over a real installed file | `TestCodexRoundTripReportsDialogueRecordsOutOfOrder`, `TestStructureVerificationReportsDialogueRecordsOutOfOrder`, `TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord` |
| the provider's own classification of the read reaches every channel of a continuation (round 10): the same words in an inspection's and a conversion's warnings, and the `summary_transcript` / `source_needs_retry` categories with their counts and the provider's own retry reason | `TestContinueDisclosesASummaryTranscriptRead`, `TestContinueDisclosesARetryableRead`, `TestInspectReportsASummaryTranscriptClassification`, `TestInspectReportsARetryableResultWithItsMessages` |
| the round-trip comparison folds the reader's own normalisations — a trimmed result output, a trimmed compact summary, an empty argument object (the last was unpinned until its mutation went unnoticed and this round pinned it) | `TestContinueConvertsInstallsAndReadsBackTheFixture`, `TestContinueRoundTripsACompactionSummaryTheReaderTrims`, `TestCodexRoundTripFoldsAnEmptyArgumentObjectAsNoArguments` |
| the carried-link rules run on every host: the separator fold and the modified-link exemption are exercised against a real symlink *and* through the module's `symlink_target` seam, so a host whose checkouts cannot create a link still pins them | `scripts/tests/test_upstream_parity.py`'s `ParityCase` link cases (`link_shape_runs`, `selftest` step) |
| a conversion report carries no source conversation content, in the core and on the CLI's wire document (installed and dry run) | `TestContinueReportCarriesNoSourceContent`, `TestContinueJSONCarriesNoSourceContent` |
| a resume turn runs the executable the caller's path resolved to, even though the turn changes into the session's own working directory | `TestRunCodexTurnResolvesARelativeExecutableBeforeChangingDirectory` |
| the tool role is never written as dialogue: a paired output travels as `function_call_output` alone, an orphan one travels nowhere | `TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult` |
| a record the provider flagged as system-injected contributes its own text nowhere, while a tool result it carries travels once, paired with its call — through a hand-built document and through the committed fixture's real parse | `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText`, `TestToolingFixtureCarriesASystemRemindersToolResult` |
| id-less calls and results get paired deterministically, or are reported unpaired — for one, two and three outstanding calls | `TestCodexRolloutPairsAnIdLessResultWithItsCall` |
| the provider binding comes from the caller and is reported | `TestCodexRolloutRecordsTheProviderBindingItIsGiven` |
| fresh UUID v7 identity, one ordered timestamp run | `TestCodexRolloutGeneratesAFreshIdentity`, `TestCodexRolloutStampsRecordsInOneOrderedRun` |
| end-to-end: convert, install, read back through the inherited reader, source and neighbours unchanged | `TestContinueConvertsInstallsAndReadsBackTheFixture` |
| the archive fallback is used and disclosed | `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` |
| which failure a conversion reports when neither side answers: the archive's own answer (candidates, truncation, a failed query) when the source is unreadable and the archive refuses it too, the source's error only when the archive holds no match | `TestContinueReportsTheArchivesAmbiguityInsteadOfAbsence` |
| the app server's diagnostic buffer is bounded and synchronized: a failure report reads a snapshot, not a race with the goroutine os/exec copies stderr on | `TestBoundedBufferKeepsTheFirstBytes`, `TestBoundedBufferIsSafeForConcurrentWritesAndSnapshots` (the second is the case `go test -race` diagnoses) |
| the native check reads the two rendered resume-command lines the way a POSIX shell reads them, on both `pathlib` flavours, so a home the renderer quotes is accepted and an unquoted one a shell would split is not | `scripts/tests/test_codex_native_check.py` (the `selftest` step) |
| the native check compares the source tree and the planted session **after** the loader and the real turn, so a mutation during either stage fails the run — and an intact run passes the same comparisons | `scripts/tests/test_codex_native_check.py`'s `ProtectedArtifactsAfterNativeVerification` |
| the native check's throw-away home is seeded with settings that cannot hold a credential — never with a copy of the host's `config.toml` — and its report line names what was copied and what was left behind (round 12) | `scripts/tests/test_codex_native_check.py`'s `SeededHostConfiguration` |
| a nested run that breaks inside a subtest is named by the case that read the child, not raised as an `IndexError` in the reader itself | `test_a_child_broken_inside_a_subtest_is_reported_by_name` |
| the printed resume invocation names the data directory the session was installed into, quoted where a shell would act on a word, and a resume verification reports the command it ran the same way — the case that reads it back does so from a home whose name holds a space, so the quoted spelling is asserted on every host (the fifth red) | `TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome`, `TestResumeVerificationCommandNamesTheHomeItRanAgainst`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueJSONDistinguishesConversionInstallationAndVerification`, `TestResumeVerificationRequiresTheTurnToAppendToTheSession` |
| exclusive install (a taken name refused, a filesystem without hard links refused rather than renamed over) and dry run writes nothing (no directory for a home the caller named that is not there yet, and the target harness is never started), dry run reports its own states, prompt required, target/level validation, home resolution | `TestInstallCodexRolloutNeverReplacesASession`, `TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks`, `TestContinueDryRunWritesNothing`, `TestContinueDryRunReportsWhatItVerified`, `TestContinueDryRunInstallsNothingThroughTheCLI`, `TestContinueRequiresAPromptBeforeItRunsATurn`, `TestContinueRefusesAnUnsupportedTargetOrLevel`, `TestContinueTargetHomeResolution` |
| the round-trip check compares rather than agrees | `TestStructureVerificationNamesWhatTheFileDoesNotHold` |
| the structural check reads the file it was handed (the installed path), never a same-id copy the target's own sessions-tree lookup would prefer (round 12) | `TestStructureVerificationReadsTheInstalledPathItWasGiven` |
| the source's own digest is taken before the conversion and compared after it; a moved source is warned about; a projection, a read whose identity is a *member* inside a container (round 11) and a stopped run answer `not-verified` rather than claiming a comparison, while a read that names no path at all still stops the run | `TestSourceCheckComparesTheFileBeforeAndAfter`, `TestSourceChangedWarningsNameTheMove`, `TestSourceCheckSaysWhenThereWasNoFileToCompare`, `TestContinueFromAContainerMemberReportsNoFileToCompare`, `TestSourceCheckStopsAReadThatNamesNoFileAtAll` |
| a result the conversion does not write is never reported as carried with a caveat, while the same two flags on a result that *is* written are counted | `TestCodexRolloutDoesNotReportOmittedResultsAsCarried` |
| a failure after the conversion keeps the report it reached — the published installation and identity, the conversion's counts — with every check that did not run `not-verified` and naming the reason: a source that vanished after publication, a temporary file the installer could not remove, a home that could not be created | `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall`, `TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved`, `TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated` |
| the CLI's three states, human shape, source check, partial-read fidelity line, exit codes, help, that a failure carrying a report still writes it (and a failure with none writes nothing), that a run which stopped before publishing renders the report's own `not-installed` state with its identity marked proposed, that a dry run's block says it wrote nothing and marks its identity proposed, and that the installed line names the session's own path and its mechanism | `TestContinueJSONDistinguishesConversionInstallationAndVerification`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueHumanOutputReportsTheSourceCheck`, `TestContinueHumanOutputNamesAPartialSourceRead`, `TestContinueUsageErrors`, `TestContinueReportsAFailedVerificationAsAFailure`, `TestContinueWritesTheReportOfAFailureAfterPublication`, `TestContinueHumanOutputReportsAPrePublicationFailure`, `TestContinueDryRunInstallsNothingThroughTheCLI`, `TestContinueRefusesAMissingDataDirectoryNobodyNamed`, `TestContinueHelpIsARequestNotAFailure` |

Gate, run on this tree (`python3 scripts/gate.py`, exit 0):

```
gate: <repo>
gate: go=<home>/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3582 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 164 tests in 5.205s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 26 project-owned prefix(es); files seen by git: 5627 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 76 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`gofmt` counts the *tracked* `*.go` files (`git ls-files '*.go'`, minus a top-level `vendor/`): 3 582,
which is every `*.go` file on disk as well, because the test file round 8 added and left untracked until
the operator committed is tracked now.

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
— and a still later one moved that result onto a record the provider wrote as a system reminder (§9);
it is still not an oracle case: its digest (`415bbc31…`, above) is the one §3's tooling run reports as
`the source session is unchanged`, and `TestDumpMatchesTheRecordedUpstreamAgreement` (in the
gate's `test` step) still holds the recorded dumps against the unchanged first fixtures.
`scripts/upstream-compare.py` itself was **not** re-run here (it needs network and a pristine
checkout of the pin); its next run is the operator's, and the offline half of the same comparison passes.

The first repair to this increment that came from CI rather than from a clause: `gate (windows-latest)` failed
in the `test` step on the two CLI cases that then read the resume line back. The line is rendered the way a
POSIX shell reads it, and a Windows temp path holds backslashes a shell would act on, so the line quotes
the data directory there — while those two cases had been written against *this* host's temp path, which
is shell-safe and stands bare. They now install into `<tmp>/…/codex home`, a word the rule quotes on
every platform, and assert the quoted spelling: one assertion for both hosts instead of a platform
branch. Executed on this host: with `TMPDIR` pointed at a directory whose name holds a backslash (a
Windows-shaped temp root; e.g. `TMPDIR='/tmp/win\sim' go test -tags fts5 -count=1 ./cmd/ccc/...`) the two
cases failed against the old expectation with
`resume_command = "CODEX_HOME='…\…' ccc-no-such-target-harness resume …"` and pass against the new one,
while the rest of the package was green in both shapes; three mutations of the renderer — drop the
`CODEX_HOME=` assignment, never quote a word, quote every word — each turned both cases red, and the
mutated file was restored byte-identical (`sha256 fb2a59a2…`). `docs/harness.md` states the quoting as
one of the Windows answers its Platform boundary section pins.

The next red was the same kind, one layer inside the harness's own tests: `gate (windows-latest)` failed
the `selftest` step at head `524a20f` — `FAILED (failures=3, errors=2)`, the three failures all
`a correct rendering was rejected` for the homes `…/codex home/.codex` and `…/it's home/.codex`, and the
two errors the self-check cases that re-run this suite in a child. Both halves are the *fixtures* of the
offline half of the native check, not its shipped behaviour: the resume-command reader compares the words
a shell reads back with the home **as this host spells it** (`f"CODEX_HOME={home}"`, the same string the
conversion was handed), and the cases spelled their homes `/tmp/…` — which `Path` spells `\tmp\…` on
Windows, where the literal rendering in the case still said `/tmp/…`. On this host the same head is
green, so the shape was reproduced here the way the suite's other platform answers are: a scratch copy
with those call sites driven by `PureWindowsPath` (the class `Path` *is* on Windows) printed
`Ran 150 tests in 7.318s` / `FAILED (failures=3, errors=2)` — CI's own numbers. The two errors were the
second defect: the reader that maps a child's per-case lines took the dangling `<case> (…qualifier…) ... `
line the verbose runner leaves when a case's only reports come from subtests for a status line and raised
`IndexError`, so the child's broken cases were never named in the parent — the one message that exists to
say which case broke on a host nobody can watch.

The repair keeps the reader (it is the rule) and fixes both ends around it: the fixtures now name homes
**both `pathlib` flavours give back unchanged** (`codex-home`, `codex home`, `C:\Users\a b\.codex`,
`it's home`) and every case drives both flavours, so a fixture that holds on one platform alone fails
here; the rejection cases assert the shell's own reading first (`CODEX_HOME=codex` + `home`, another
home, another session, no assignment), so a rejection can no longer pass for a spelling accident. The
reader skips a line with no status, and a new case runs that child shape and requires the name to reach
the parent's message. Executed here, each mutation of the repaired tree:

| mutation | observed |
| --- | --- |
| `scripts/codex-native-check.py`: the substring form (`f"CODEX_HOME={home}" in command`) | `FAILED (failures=12)` — every quoted home rejected, the split home and the other session accepted |
| `…`: `shell_words` = `command.split()` (no shell reading) | `FAILED (failures=11)` — every rendering rejected (the quoted `<prompt>` alone is enough), an unreadable line read as words |
| `…`: `reported_invocation` = "any readable line" | `FAILED (failures=10)` — every rejection case red |
| `scripts/tests/test_gate.py`: the empty-status guard removed | that case **ERRORs** (`IndexError`), the parent reporting no case name |
| `scripts/tests/test_codex_native_check.py`: one fixture back to `/tmp/…` + a literal to match | `FAILED (failures=3)`: the accept case at `flavour='PureWindowsPath'` — the original defect now fails here, on the host that used to be blind to it — plus the two self-check cases the red child trips (their design) |

The third red was the same kind again, one layer further in: `gate (macos-latest)` and
`gate (windows-latest)` failed the `selftest` step at head `4cd0567` — both legs `FAILED (failures=4)`,
their child runs printing `failures=2, skipped=3 | broken:
test_a_mutation_during_the_resume_stage_is_caught_by_the_last_comparison,
`test_an_intact_run_passes_the_comparisons` — that is, the two `ProtectedArtifactsAfterNativeVerification`
cases round 6 added (§11): the ones both red legs really broke (their other two failures are the
self-check cases reading that red child), and ones the `ubuntu-latest` leg (green, 46 s) does not run
differently. Both legs were red for defects in those two cases' **fixtures**, never for the shipped
harness — and the two legs' defects were not the same one: this repair is the macOS spelling below, and
the Windows leg's own defect, the unquoted words of the same cases' fabricated command lines, is the
round after it. The macOS one:
`main` resolves what it is handed (`--repo` always, and `--codex-home` when the caller names one, which
this case does) and passes that resolved home to the conversion, so the home the document reports is the
home the harness asked for — while the case derived its whole scratch tree from `tempfile.mkdtemp`'s own
spelling. On macOS that spelling is reached through a symlink (`/var` → `private/var`), so `resolve()`
reads `/var/folders/…` back as `/private/var/folders/…`, and the case's stub then answered a
`ccc-native-check continue` command filed under the resolved path while it recognised only its own
unresolved one: both cases failed at their first stubbed command.

Reproduced on this host by giving `tempfile` a root reached through a symlink — `TMPDIR` pointing at a
directory whose name is one (`/tmp/cccrepro/var/tmp`, `var` → `private-var`) — where head `4cd0567`'s own
file, discovering that module's eight cases, printed `Ran 8 tests in 0.011s` / `FAILED (failures=2)`, the
first failure's detail being
`AssertionError: the case's stub was asked for an unexpected command:
['/tmp/cccrepro/private-var/tmp/ccc-native-check-case-…/repo/bin/ccc-native-check', 'continue',
'claude:9f2c1d47-…', '--to', 'codex', '--target-home',
'/tmp/cccrepro/private-var/tmp/…/codex-home', '--target-binary', …, '--json']` — the macOS shape, on a
host that was blind to it.

The repair fixes the case, not the harness (the harness is self-consistent: it compares against the home
it resolved and gave the conversion). The scratch root is resolved once, before anything is derived from
it, so every path the case creates, hands over and compares against holds the harness's own spelling on
every host. No assertion changed — the intact run must still exit 0 with both comparisons `PASS`, and
the mutating run must still fail exactly those two checks and pass the turn. The class's own docstring
was corrected where it stated the *other* direction ("both cases below fail" under the
moved-comparisons defect): measured, only the mutating case fails there, because the intact run has
nothing for a too-early comparison to miss. Executed here:

| run / mutation | observed |
| --- | --- |
| head `4cd0567`'s own file, `TMPDIR` through a symlink (the macOS shape; that module's 8 cases) | `Ran 8 tests in 0.011s` / `FAILED (failures=2)` — the stub's own unresolved spelling against the run's resolved one |
| the repaired file with that one line reverted, under the same `TMPDIR` (the class's 2 cases) | `Ran 2 tests in 0.008s` / `FAILED (failures=2)` |
| …and the same revert over the whole suite, under the same `TMPDIR` | `Ran 153 tests in 5.095s` / `FAILED (failures=4)`: the two cases **and** the two self-check cases that read the red child — the number and the broken names both CI legs printed. Restored byte-identical afterwards (`sha256 79d972bd…`, the digest taken before the revert) |
| the repaired case under the same `TMPDIR` (module scope) | `Ran 8 tests in 0.012s` / `OK` |
| the whole suite under the same `TMPDIR` | `Ran 153 tests in 5.037s` / `OK` |
| the rule the case pins, on a scratch copy of the repaired case: the two comparisons moved back above the loader and the turn | `Ran 2 tests in 0.011s` / `FAILED (failures=1)` — identical on a plain host and under the aliased `TMPDIR` — with `the mutating run's failures are not exactly the two protected-artifact comparisons: []`, both comparisons having answered on the pre-mutation state |

The fourth red was in the same cases' fixture again, and the macOS leg it had just repaired was green:
`gate (windows-latest)` failed the `selftest` step at head `1a476e0` — `FAILED (failures=4)`, its child
printing the same two broken names — while `ubuntu-latest` and `macos-latest` passed. The document
those cases fabricate spells the two reported command lines by joining their argument words bare
(`f"{assignment} " + " ".join(resume_arguments)`), which is a line the renderer under test
(`internal/ccc/continue.go`'s `renderResumeCommand`/`shellQuote`) only ever produces while every word is
shell-safe; the stubbed binary is a full path, so on Windows it holds backslashes, the check's own
reading of the line (the rule it exists to apply) eats them, and the check answers FAIL for a line the
fixture mis-rendered — leaving the mutating case failing its "failures are not exactly the two
protected-artifact comparisons" assertion and the intact case its "exit 0" one, the added name being
`the reported resume command selects the installation home`.

Reproduced on this host the way the suite's other Windows answers are, by pointing `TMPDIR` at a
directory whose name holds a backslash, so every path the case derives holds one as they all do on
Windows (`mkdir -p '/tmp/ccc-win\temp'`, then `TMPDIR='/tmp/ccc-win\temp' python3 -m unittest discover
-s scripts/tests`; the module scope is the same command with `-p test_codex_native_check.py`):

| run | observed |
| --- | --- |
| head `1a476e0`'s own file, that `TMPDIR`, the module's 8 cases | `Ran 8 tests in 0.177s` / `FAILED (failures=2)` |
| …the whole suite, that `TMPDIR` | `Ran 153 tests in 12.606s` / `FAILED (failures=4)`: the two cases **and** the two self-check cases that read the red child — CI's own number, over the two child names it printed |
| the repaired file, same `TMPDIR`, the module's 8 cases | `Ran 8 tests in 0.012s` / `OK` |
| …the whole suite, same `TMPDIR` | `Ran 153 tests in 5.320s` / `OK` |
| …the whole suite under the aliased `TMPDIR` above and on a plain host | `Ran 153 tests in 5.227s` / `OK` and `Ran 153 tests in 5.060s` / `OK` |

The repair is the fixture's, like the two before it: the reported lines are rendered the way the
renderer spells them — the environment's value, then every argument word, quoted where a POSIX shell
would act on it (`shlex.quote`, the same rule `shellQuote` implements) — and no assertion changed: the
intact run must still exit 0 with both comparisons `PASS`, and the mutating run must still fail exactly
those two checks and pass the turn. No platform branch was added for it: the rule's Windows spelling —
a quoted word holding backslashes reads back as itself — is already asserted on every host by the
module's `ResumeInvocationCheck` cases, which drive `PureWindowsPath` beside this host's `Path`.

The fifth red is this increment's own case, added by the sweep: `gate
(windows-latest)` failed the `test` step at head `37c0a10` on
`TestResumeVerificationRequiresTheTurnToAppendToTheSession` — `main_test.go:1947: the reported resume command
"CODEX_HOME='C:\Users\RUNNER~1\…\002' …" does not name "CODEX_HOME=C:\Users\RUNNER~1\…\002"`. The assertion
the sweep added to read the reported command out of the run's own document spelled the data directory as
*this host's* temp path (`"CODEX_HOME=" + target["home"]`), and a Windows temp path holds backslashes and a
`~`, which the rule quotes: the reported line was right and the expectation was host-shaped — the defect the
first red above repaired in the two cases that existed then, now in the third CLI case to read that line back
(Q-0002-13's default moves the test, not the renderer). The case installs into a home whose name holds a
space (`shellUnsafeHome`) and asserts the quoted spelling, exactly as those two do.

Reproduced on this host with a temp root whose name holds a space — a home the rule quotes on every host, and
the shape a Windows temp path has for this rule (a word it quotes) — rather than with the backslash-bearing
root the two cases above use: this
case's stub harness looks the installed session up with `filepath.Glob` over the home, and on a POSIX host a
backslash in the pattern is an escape, so a backslash-shaped `TMPDIR` fails the case for a reason the rule is
not about (measured on the repaired tree: `main_test.go:1924: exit 1, want 0 (stderr "the stub harness found
no installed session for …")`).

| run | observed |
| --- | --- |
| head `37c0a10`'s own case, `TMPDIR='/tmp/ccc quoted temp'` | `FAIL` — `main_test.go:1947: the reported resume command "CODEX_HOME='/tmp/ccc quoted temp/TestResume…/002' …" does not name "CODEX_HOME=/tmp/ccc quoted temp/TestResume…/002"` |
| the repaired case, that `TMPDIR` | `ok` |
| the repaired `cmd/ccc` package, that `TMPDIR` | `ok  	go.kenn.io/agentsview/cmd/ccc	1.830s` |
| the repaired case, a plain host | `ok` |
| the repaired case with the rule mutated (`continue.go:851`'s `shellUnsafeCharacters` with the space dropped, so a word holding one is left bare) | `FAIL` — `main_test.go:1947: the reported resume command "CODEX_HOME=…/codex home …" does not name "CODEX_HOME='…/codex home'"` |

The case's own mutations were re-executed on this tree rather than renumbered, and the anchors every
insertion in it moved are recorded with them: the matrix's E2 row carries all three (`main_test.go:1947` for
the reported-command mutation, `:1991` for the exit-code one, and the growth rule's `codex_verify.go:374` →
`main_test.go:1924`, now caught by the case's own exit-status assertion because the loader mode its audit
repair sets lets the unmutated run reach exit 0). `docs/harness.md` states the third case beside the two,
with the reason a backslash-shaped root cannot stand in for it here.

The `selftest` step is **164** cases on this tree — the 153 that sentence once counted, plus the cases the
later rounds and the sweep's `scripts/tests/test_gate.py` changes added (round 12's block records 158) — and
the blocks above are refreshed from each repair's own run. `docs/harness.md`
records all of it: the path-spelling constraint as its Platform boundary section's path bullet, now
including the scratch root the third repair resolves (with the aliased-`TMPDIR` run as that constraint's
instance on any host), the per-word quoting the fourth repair restored to the same cases' fixture as the
Windows-backslash bullet's own instance (with the backslash-`TMPDIR` run as its reproduction here), the
third CLI case to read a rendered line back as that bullet's newest instance (with the quoted-`TMPDIR` run
and the limit of the backslash trick for it), and
the dangling line as what the self-check cases read a child's per-case lines for.

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

## 8. Repair round 3 — the round-3 review's four findings, each pinned by mutation

The review asked for the rules of this surface to be proved by executing a mutation rather than by
argument, and named four defects: a subagent record that also carries the source's compaction marker was
applied as the main conversation's `compacted` boundary; an archive's derived summary was written as an
extra tool outcome beside the very events it is derived from; a relative `--target-binary` spelling
stopped resolving once the resume turn changed into the session's working directory; and the ordinary
human result said nothing about a source read that had skipped content. All four are fixed above, and the
block below is the repaired human result of the same fixture (throw-away data directory, target home and
absent harness, so the native half is deterministic; repository root and throw-away paths rewritten):

```
$ CLAUDE_PROJECTS_DIR=<repo>/internal/ccc/testdata/fixtures/claude \
    <tmp>/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex \
    --target-home <tmp>/home4 --target-binary ccc-no-such-target-harness --data-dir <tmp>/data
exit 0
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → codex
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  converted:    codex-rollout-jsonl, 3101 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: pre_compaction_history ×3
  omitted:      malformed_source_lines ×1
  installed:    <tmp>/home4/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e262-4c95-7d73-a982-c6bb14876d33.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e262-4c95-7d73-a982-c6bb14876d33
  working dir:  /home/user/code/example-api
  resume:       CODEX_HOME=<tmp>/home4 ccc-no-such-target-harness resume 01a1e262-4c95-7d73-a982-c6bb14876d33
  non-interactive resume: CODEX_HOME=<tmp>/home4 ccc-no-such-target-harness exec resume 01a1e262-4c95-7d73-a982-c6bb14876d33 '<prompt>'
  cli_version:  0.169.0 (writer-pin)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  not-verified (the target harness's own loader (codex app-server, thread/resume))
                ccc-no-such-target-harness is not on PATH, so the installed session's native load could not be verified (ccc-no-such-target-harness is not on PATH)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                not requested (--verify load)
  warning:      the target harness was not found on PATH: the conversation is converted and installed at <tmp>/home4/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e262-4c95-7d73-a982-c6bb14876d33.jsonl, and its native load is unverified
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The same invocation with `--json` reports `source.fidelity` as
`{"malformed_lines": 1, "source_version": "1.0.30", "read_from": "source"}` (the fixture's last line is
not JSON, and it is written as a complete record, so `truncated` is absent), `conversion.source_fidelity`
as the same block, and `conversion.losses[]` as the eighteen categories, three of them non-zero
(`tool_name_vocabulary ×1`, `pre_compaction_history ×3`, and the one this round added):

```
{ "category": "malformed_source_lines", "disposition": "omitted", "count": 1,
  "detail": "the provider's reader skipped these source line(s) as unreadable (invalid JSON, or a line
             the reader would not read), so no message in this conversation stands for them and no record
             here can carry what it could not read" }
```

The new category is the read's own count of lines whose content its parser could not turn into a message
(`internal/ccc/codex_target.go`): no per-message category can see them, and an unfinished final record —
which the parser counts among them — is named in the reason rather than reported as an extra omission
(`TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` runs both the truncated and the complete
shape).

Each new or changed rule was pinned by reverting its line, running the test that claims to pin it, and
restoring the file from a byte-identical copy (the sha256 of `internal/ccc/codex_target.go`
`338e1632…`, `internal/ccc/codex_verify.go` `b79031c0…` and `cmd/ccc/main.go` `741d5776…` was compared
after every restore, and the two `cmd/ccc/main.go` mutations were re-run against that file's final bytes,
after the wording change below):

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a subagent record that also carries the compaction marker is excluded before the boundary rule | the guard back to `message.IsSidechain && !message.IsCompactBoundary` | `TestCodexRolloutDoesNotApplyASidechainCompactionBoundary` FAIL: `the conversion wrote the target's compacted record for a subagent's summary: {"message":"SUBAGENT-SUMMARY"}` |
| an archive summary is a view of the call's recorded results, so it is not written beside them | `callHasRecordedResult` → `return false` | `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults` FAIL: 6 `function_call_output` records, `toolu_1`'s text twice and `toolu_2: agent-1:…/agent-2:…` as its own outcome, want 4 |
| a summary whose call has no recorded result *is* written (the fallback half) | `callHasRecordedResult` → `return true` | the same test FAIL: 3 records, `toolu_3: summary-only outcome` missing |
| a resume turn runs the executable the caller's path resolved to, not one re-resolved after the directory change | `resolveCodexExecutable(binary)` back to `exec.LookPath(binary)` | `TestRunCodexTurnResolvesARelativeExecutableBeforeChangingDirectory` FAIL: `fork/exec ./ccc.test: no such file or directory` |
| the human block names a partial source read | the `fidelity:` guard → `false` | `TestContinueHumanOutputNamesAPartialSourceRead` FAIL: `the human output is missing "fidelity:     1 malformed line(s)\n"` |
| an unfinished final record is named in the read's own words, not as something this CLI did | `truncatedSuffix` → `""` | the same test FAIL: `an unfinished final record is not named by the read's own words` |
| the read's own skipped-line count reaches the loss report | `sourceFidelity: conv.Fidelity` dropped from the builder | `TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` and `TestContinueConvertsInstallsAndReadsBackTheFixture` FAIL: count `0` with the `every source line this read produced a message for was readable` reason |
| the reason names the unfinished final record it counts | `if b.sourceFidelity.Truncated` → `false` | `TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` FAIL: the reason does not name it |
| the adjacent rule the fix sits beside: a sidechain record is omitted, never written as the conversation's own turn | the sidechain branch's `return` dropped | `TestCodexRolloutReportsEveryCategoryItKnows` FAIL: `the rollout holds "Subagent said this", which the report says was omitted` |
| the adjacent rule beside it: the turns before a carried boundary are counted | `b.lossPreBoundary = b.carried.Messages` → `0` | `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt` FAIL: `pre_compaction_history … Count:0 …, want 2 carried-with-caveat` |

The tail clause reads ", and the last record was still being written at read time" instead of the words
`inspect` already used for the same state (", truncated by the writer"): in this tree "the writer" is the
target writer, which never truncates a source, so that line could be read as a claim about the conversion.
`inspect` and `continue` share the one rendering (`truncatedSuffix`), and the fixture this repository
commits holds a *complete* malformed line, so its block shows the count without that clause (the CLI case
asserts both shapes).

The native-check blocks recorded in §3 then printed `17 loss categories`, the runs before this repair,
and the script's own condition (`any(loss["count"] > 0)`, with the count printed for a reader) is
unchanged. Round 4 re-ran that check on both fixtures, so §3 now holds this tree's runs and they print
eighteen; round 3 itself did not re-run it (that check needs the Codex binary — and, for a real turn,
model access — and is deliberately not a gate step), and nothing round 3 changed touched what the check
verifies beyond the printed count. Its offline half is the gate's `test` step, which passes on this tree
(§6).

## 9. Repair round 4 — a system-injected record's tool evidence travels

The round-4 review found one defect: `writeMessage` returned on a record the provider flagged as
system-injected *before* processing the tool evidence the same record can carry, so the result of an
earlier carried call disappeared without a tool-result loss category (only `system_records` rose). The
inherited Claude parser keeps that shape — `internal/parser`'s
`TestParseClaudeSession_SystemReminderPromotionPreservesToolResults` asserts `IsSystem` *and* a populated
`ToolResults` on one message — and the writer's branch suppressed both.

`writeMessage` now counts the record's own text as omitted and calls `writeToolEvidence`, the same helper
the dialogue path calls (extracted from that path unchanged): the text is not written, the calls and
results the record holds are. `system_records`'s detail says exactly that split. The regression is pinned
twice: `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText` on a hand-built document (the
output travels once, paired with its call; both records' texts are counted) and
`TestToolingFixtureCarriesASystemRemindersToolResult` through the committed fixture's real parse (the
third tool result now arrives on the mixed record, and the reminder's own words appear nowhere in the
rollout).

The second Claude fixture changed with it: its third tool result moved from its own plain user line to a
`claudeSystemReminderToolResult` record (a text block holding only `<system-reminder>…</system-reminder>`
plus the `tool_result` block), so the committed bytes exercise the parser's own mixed shape. New digest
`sha256 415bbc31a6cd541f9bc1ff2b99dc4532abe81f218e14dc5c97c7399f3b212873`, reproduced by
`TestFixturesMatchTheirGenerator`; the conversion's own bytes are unchanged (4611), because the same
single `function_call_output` is now written from the mixed record instead of the superseded plain line.
§3 re-ran the native check on both fixtures on this tree (18 loss categories each; the tooling run's
resumed turn still quotes the third output, `"maxBackoffSeconds"`, quotation marks included).

Each rule of the touched surface was pinned by executing one mutation, reverting the line, running the
tests that claim it, and restoring the file from a byte-identical copy (`internal/ccc/codex_target.go`
`3ab9b891…` and `internal/ccc/fixturegen/main.go` `98b63928…` compared after every restore; the fixture
regenerated back to `415bbc31…`):

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a system-injected record's own text is never written as dialogue | the branch's guard narrowed to `IsSystem && no tool evidence`, so a mixed record fell through to the dialogue path | `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText` FAIL: `the rollout holds "remember the workspace state"…` plus its carried counts; `TestToolingFixtureCarriesASystemRemindersToolResult` FAIL: `the rollout holds "the workspace state was restored" 2 time(s)…` and `system_records … Count:0 …, want … once` |
| the tool evidence such a record carries is written, once, paired with its call | `b.writeToolEvidence(message)` dropped from the branch (the pre-fix `return`) | the same two cases FAIL (`the recorded output appears 0 time(s)`, `function_call_output records for toolu_1 = 0`) and `TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures` FAIL: `carried = {… ToolCalls:3 ToolResults:2 …}, want all three calls and all three results` |
| the record is counted as omitted under `system_records` | `b.lossSystemRecords++` dropped | `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText` and `TestToolingFixtureCarriesASystemRemindersToolResult` FAIL: `system_records … Count:0` |
| the adjacent rule the extraction sits beside: an archive summary is written only for a call with no recorded result of its own | `callHasRecordedResult` → `return false` | `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults` FAIL: 6 `function_call_output` records (`toolu_1` twice, the aggregate as its own outcome), want 4; `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce` FAIL: `carried = {… ToolResults:4 …}` and the structure check names the duplicated and absent results |
| the committed fixture really holds the mixed shape the regression claims (else the regression passes vacuously) | the generator's mixed-record call reverted to the plain `ClaudeToolResultUserJSON` line, fixtures regenerated | `TestToolingFixtureCarriesASystemRemindersToolResult` FAIL: `the tooling fixture holds no system-injected record carrying the tool result toolu_01Tooling00000000000000003, so nothing pins what a writer must do with one` |

Tests, gate: `internal/ccc` top-level cases 103 → **105** (`cmd/ccc` 20, unchanged; the suite's own
`selftest` step 151, unchanged). `python3 scripts/gate.py` on the final tree — **exit 0, `GATE: PASS`**
(not partial; the block is the one in §6). No other number in this record changed: the fixture digests of
§1's two other files, the target file's byte count for the first fixture, and the recorded §1/§2 runs
stand as the runs they were.

## 10. Repair round 5 — a failure after publication keeps its report; an omitted result is not "carried"

The round-5 review found two defects, both of the same shape: a report claimed the opposite of what the
run had done.

**A published session's report was thrown away by a later failure.** `Continue` recorded the installation
and the new target path, and then discarded the whole report (`ContinueResult{}`) when the post-install
comparison of the source file's own bytes could no longer run — the session was installed, and the caller
received neither its path nor its identity, because `cmd/ccc` exits before writing the document when
`Continue` answers with an error. One level down the same discard existed in the installer: the file is
published by hard link and a temporary file that cannot be removed is reported *after* publication
(`InstallCodexRollout`), and its caller dropped the published path with the error.

`Continue` now answers a failure after the conversion with the report it reached, and `reportAfterFailure`
fills in the checks that have no answer — the check the stop kept from running is `not-verified` and names
the failure, a check the level never asked for stays `not-requested`, and a check that already answered
keeps its own state and detail (the dry run's structural read-back runs before the source comparison, so
`TestContinueKeepsTheDryRunReportWhenItsSourceCheckCannotRun` pins that it is not downgraded). The report's
`warnings[]` names the stopped run as well, so a caller reading only the warnings cannot miss it. The
installer's doc comment now states its contract — the returned path and the error are independent answers —
and its temporary-file removals go through a `removeFile` seam (`var removeFile = os.Remove`, beside the
existing `linkSession` one) so a cleanup failure is exercisable on any host, not only where a directory
permission can make a removal fail. `cmd/ccc` writes the report it was handed before it names the failure
and still exits 1; a failure that produced no conversion writes no document at all.

**Omitted results were reported as carried with a caveat.** `codex_target.go`'s `toolResult` incremented
`lossUnresolved` and `lossOutcomes` before the pairing and text guards, so a result whose call is not in the
conversation, or which holds no text to write, was counted as `carried-with-caveat` — the detail even said
its text "is what was carried" — while `carried.tool_results` stayed 0 and no output record existed. Both
counters are now incremented at the write site, past every omission guard, and the two categories' details
state it ("only results that were written are counted here"); the omitted cases keep their own categories
(`unpaired_tool_results`, `empty_tool_results`).

The source check is three states now, which is what the same class of defect looked like in the
installation block: `installation.source_check_state` is `unchanged`, `changed` or `not-verified`, the last
for a comparison that did not run — an archive read has no file of its own, and a run can stop before
comparing — with `detail` carrying that answer's own words. The boolean it replaces
(`installation.source_unchanged`, printed by the §1/§2 runs above) answered "did the file move?" for a
comparison that never ran: the archive read was reported `true` while its own detail said no file was open.
The human block renders the third state as `not verified (…)` (`cmd/ccc/main.go`'s `sourceCheckWord`).

Each rule of the touched surface was pinned by executing one mutation, reverting the line, running the tests
that claim it, and restoring the file from a byte-identical copy (`internal/ccc/continue.go`,
`internal/ccc/codex_target.go`, `cmd/ccc/main.go` compared by SHA-256 after every restore; all 13 restores
agreed):

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a published session is reported even when the install itself reports a cleanup failure | the published-path block and the failure's `reportAfterFailure` replaced by the pre-fix `if err != nil { return ContinueResult{}, err }` | `TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved` FAIL: `installation = {State: Path: …}, want the published session reported beside the failure` |
| a source-check failure after publication keeps the report | the same site, reduced to `return ContinueResult{}, err` | `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall` FAIL: `conversion = {State: …Carried:{Messages:0 …}}` and `installation = {State: …}, want the published session reported` |
| a failure before publication keeps the conversion report | the home-creation branch back to `return ContinueResult{}, fmt.Errorf(…)` | `TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated` FAIL: `installation = {State: …}, want not-installed` and `want the source check reported as not run, naming what stopped it` |
| a check that already answered is not downgraded by a later failure | `hasAnswer(...)` back to `Detail == ""` in `reportAfterFailure` | `TestContinueKeepsTheDryRunReportWhenItsSourceCheckCannotRun` FAIL: `structure = {State:not-verified … Detail:the run stopped before this check ran …}, want the read-back the dry run completed to keep its verdict` |
| the CLI writes the report a failure carried (and exits nonzero) | the error branch's report write dropped | `TestContinueWritesTheReportOfAFailureAfterPublication` FAIL (both spellings): `a failure that carried a report wrote nothing to stdout, so the target identity is lost` |
| a failure with no report writes no document | the branch's `Conversion.State == converted` test forced true | the same case FAIL: `a failure with no report wrote to stdout: "{ "upstream_revision": …"` |
| an archive read is a comparison that did not run, not "unchanged" | the archive branch back to `return SourceCheckUnchanged, …` | `TestSourceCheckSaysWhenThereWasNoFileToCompare` FAIL: `state = "unchanged" …, want a comparison that did not run`; `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` FAIL on the installation's state |
| a comparison that could not run is reported `not-verified`, not as a move | the error branch sets `SourceCheckChanged` before reporting | `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall` FAIL: `installation = {State:installed …}` and its "nothing observed a move" warning assertion |
| the moved-source warning is only for a comparison that ran and differed | `state != SourceCheckChanged` → `state == SourceCheckUnchanged` | `TestSourceChangedWarningsNameTheMove` FAIL (a `not-verified` check warned about a move) |
| an unresolved flag is counted only where the output was written | `lossUnresolved++` moved back above the pairing/text guards | `TestCodexRolloutDoesNotReportOmittedResultsAsCarried` FAIL: `unresolved_tool_outputs = {… Count:2 …}`, want 0 for the omitted cases and 1 for the written one |
| a recorded outcome is counted only where the output was written | `lossOutcomes++` moved back the same way | the same case FAIL: `tool_result_outcomes = {… Count:2 …}` |
| a written result with those flags *is* counted | the whole guarded counting block deleted | the same case FAIL (`Count:0` for both) and `TestCodexRolloutReportsEveryCategoryItKnows` FAIL: `tool_result_outcomes … Count:0 …`, `unresolved_tool_outputs … Count:0` |
| the omitted results are still counted as omissions | `lossUnpairedResults++` dropped from the missing-call branch | the same case FAIL (`unpaired_tool_results … Count:0`, want 1) and `TestCodexRolloutReportsEveryCategoryItKnows` FAIL (`Count:0`, want 2) |

Two regressions sit beside them as documents rather than mutations: the omitted-result case is built in
`TestCodexRolloutDoesNotReportOmittedResultsAsCarried` (an orphan result and a textless one, both carrying
both flags, against the same flags on two written results), and the CLI rule is driven through `continueRun`
— `ccc.Continue` under a name a test can replace — so the CLI's own answer is exercised without a conversion
that fails after publication.

Tests, gate: `internal/ccc` top-level cases 108 → **112** (`cmd/ccc` 20 → **21**; the suite's own `selftest`
step 151, unchanged; before = a detached worktree of `d95ebd2`, removed). `python3 scripts/gate.py` on the
final tree — **exit 0, `GATE: PASS`** (not partial; the block is the one in §6).

`scripts/codex-native-check.py` re-ran on both fixtures on this tree, each with a real turn
(`--model-provider tencent-litellm --model deepseek-v4.1-flash-fast --resume-prompt …`): **13/13 checks
passed, 0 failed, 0 skipped** each, `18 loss categories` each, the tooling fixture's installed file still
4611 bytes with 3 tool calls and 3 results (its resumed turn still quotes `internal/retry/policy.go` and
`MaxAttempts = 4`), the first fixture's read-back still 7 messages / 1 call / 1 result. No number this
record states changed at that round; §3's per-run ids and byte counts were re-recorded in round 6 (§11)
and again in round 7 (§12), whose runs are the ones the blocks above now hold.

Docs changed with the code: the plan's §3 (the three source-check states and the post-publication rule),
§4 (exit codes) and §5 (the document's shape and what a failed run writes), and this record's §2 note, §4 and
§6 table (`source_check_state`; the renamed tests; the new rules and their pins).

## 11. Repair round 6 — a failed install is reported as such; the native check's integrity comparisons run last

Round 6's review left two findings, both P2 and both fixed here; its carried-tree parity audit found no
increment-attributable defect, so nothing in the carry changed.

**1 — the human block tells the truth about a failed installation** (`cmd/ccc/main.go`). `writeContinuation`
branched only on `Conversion.DryRun`, so a non-dry run that stopped before publication printed
`installed: []` — an empty path where an installed path belongs — followed by a target id and two resume
commands, although `Installation.State` was `not-installed` (a home that cannot be created is exactly this
shape: `internal/ccc/continue.go` keeps `not-installed`, and
`TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated` already pinned the core's truthfulness;
the CLI's failure-report case covered only an already-published session). The block now renders the
report's own state — `installed:    not-installed (nothing was published under <home>)` — for any run
whose installation is not `installed`, and marks the three identity lines (`target id`, `resume`,
`non-interactive resume`) with `(proposed; nothing was installed)`: the conversion generated an identity,
but printing it the way an installed one is printed offers a session no harness can open. A dry run's
identity carries the same marker, for the same reason. Regression:
`TestContinueHumanOutputReportsAPrePublicationFailure` drives the real engine through the CLI into a home
whose parent chain is a file (so publication cannot happen), asserting the state line, the marker on each
of the three identity lines, that no `installed: <path>` line appears, and — through `--json` on the same
run — that the document says `not-installed` with no path while the proposed identity is still reported.

**2 — the native check's integrity comparisons run after every native stage**
(`scripts/codex-native-check.py`). The harness compared the source fixture tree and the planted session
right after its read-back check — that is, before `thread/resume` and before the real resume turn — and
the final `existing_sessions` comparison excludes the planted session by construction, so a mutation
during the load or the turn would have left the earlier `PASS` standing. Both comparisons now sit in the
block that runs after the loader *and* the turn and before the cleanup that unlinks the planted session,
and a failing comparison's detail names what moved (`the fixture tree changed: <paths>` /
`<digest> before, <digest> after`) instead of repeating the unchanged-tree wording. The ordering is
pinned offline, without the Codex binary, by `ProtectedArtifactsAfterNativeVerification` in
`scripts/tests/test_codex_native_check.py`: it drives the shipped `main` with its subprocess runner, app
server, `thread_view` call and `shutil.which` replaced by stubs, a stub `ccc continue` that installs a
real file into a throw-away home, and a stub resume turn that changes both protected artifacts while it
runs — the run must fail exactly those two checks (and pass the turn), while the same drive without the
mutation must exit 0 and pass both. That control is what stops a comparison that answers `FAIL` for its
own reasons from satisfying the pin.

Every rule of the touched surface was pinned by executing one mutation, running the test that claims it,
and restoring the file from a byte-identical copy (`cmd/ccc/main.go` and `scripts/codex-native-check.py`
compared by SHA-256 after every one of the seven restores; all agreed with the copies taken before the
first). The first two rules are the finding's; the rest are the arms and details of the same block, so a
regression in any of them is red rather than silently accepted:

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a run that stopped before publication renders `not-installed`, not an empty installed path | the installation `switch`'s `default` branch merged back into the pre-fix `else` (`installed: %s [%s]`) | `TestContinueHumanOutputReportsAPrePublicationFailure` FAIL: `the human block is missing "installed:    not-installed (nothing was published under …)"`, and the block it printed carries `installed:     []` — the finding's own shape |
| the identity lines of an unpublished run are marked proposed | the `if !published` assignment dropped | the same case FAIL: `identity lines marked as proposed = 0, want 3 (target id, resume, non-interactive resume)` |
| a dry run's block says it wrote nothing and carries no `installed:` line | the dry-run arm replaced by the installed spelling | `TestContinueDryRunInstallsNothingThroughTheCLI` FAIL: `the dry run's human block does not say it wrote nothing` |
| the installed line names the mechanism the installation reported | the `[%s]` mechanism dropped from the installed arm | `TestContinueHumanOutputNamesTheResumeCommand` FAIL: `the installed line does not name the session's own path under the home and its mechanism: "  installed:    …/rollout-….jsonl"` |
| the integrity comparisons run after the loader and the turn | the two `record` calls moved back to the read-back site (the pre-fix order) | `ProtectedArtifactsAfterNativeVerification` FAIL: `the mutating run's failures are not exactly the two protected-artifact comparisons: []` — both comparisons had answered `PASS` on the pre-mutation state |
| a failed comparison names what moved | the source check's `else` detail replaced by the unchanged-tree wording | the same case FAIL: `'the fixture tree changed:' not found in 'sha256 0af93f098a46ad99… (fixture tree identical)'` |
| the comparisons can fail (the case is not a tautology) | the source check's verdict forced to `False` | the same case's control FAIL: `'FAIL' != 'PASS'` — the intact run no longer passes |

Tests, gate: `cmd/ccc` 21 → **22** and `internal/ccc` 113 → **113** top-level `--- PASS` cases
(`go test -v` per package; before = a detached worktree of `389179e`, removed); the suite's own `selftest`
step 151 → **153** (the two new cases). `python3 scripts/gate.py` on the final tree — **exit 0,
`GATE: PASS`** (not partial; the block is the one in §6).

The native harness re-ran on both fixtures after the change, each with a real turn
(`--model-provider tencent-litellm --model deepseek-v4.1-flash-fast --resume-prompt …`): **13/13 checks
passed, 0 failed, 0 skipped** each, exit 0, with the two integrity lines printed after the turn — the
ordering this round is about, observed in a real run and not only in the offline pin. §3's two blocks were
those runs; round 7 re-recorded them (§12).

Docs changed with the code: this record's §3 (both runs re-recorded in the new order), §4 and §6 (the
ordering rule and its offline pin; the refreshed `go test` and gate blocks; the CLI row's new test), and
`docs/harness.md`'s native-harness entry (the "before and after" guarantee now says after the loader and
the turn). `docs/decisions/OPEN-QUESTIONS.md` gains `Q-0002-15`, the human block's spelling of a
not-installed report and of a proposed identity. `AGENTS.md` is unchanged: no repository layout, gate
step, command, read order or agent rule moved this round. Goal bookkeeping: 0002 is still
`Implemented …; awaiting operator acceptance`, so nothing moves to `docs/goals/archive/` and
`docs/goals/README.md`'s row is already accurate.

## 12. Repair round 7 — no conversation content in a conversion report; the writer never writes text the reader reads as nothing

Round 7's review left two P2 findings, both fixed here; its carried-tree parity audit found no
increment-attributable defect, so nothing in the carry changed.

**1 — a conversion report carries no source content** (`internal/ccc/continue.go`). `Continue` built its
source block with `reportFrom`, the inspection summariser, which fills `FirstUserMessage` with up to 240
bytes of the source's opening user message; the JSON serializer therefore shipped the whole prompt (when
it is short) under `source.first_user_message` in every `ccc continue --json` — dry runs included —
against the core design's rule that conversation bodies and full prompts are not logged by default (the
human block never printed it). `Continue` now builds the block with `conversionSourceReport`, which is
`reportFrom` without that preview; `ccc inspect` keeps its own field. Regressions:
`TestContinueReportCarriesNoSourceContent` (the field is empty, and no message text of the read appears
in the report's JSON, which the case takes from the read itself rather than repeating fixture content) and
`TestContinueJSONCarriesNoSourceContent` (`cmd/ccc`: the CLI's wire document, installed and dry run,
carries neither the field nor the fixture's opening prompt, which the case reads out of the fixture the
way it reads the fixture's id).

**2 — the writer never writes text the inherited reader reads as nothing**
(`internal/ccc/codex_target.go`, `internal/ccc/codex_install.go`). The writer rejected only an empty
result text, so a Claude tool result whose recorded content was `"\n"` was written as a
`function_call_output`, counted as carried, and expected by the round-trip comparison — while the
inherited Codex reader trims an output's text and records no result for one that is only whitespace
(`internal/parser`'s `handleFunctionCallOutput` returns before appending). The installed session then
failed the structural check ("tool result … is in the target document but not in the file") and the run
exited nonzero even with `--verify none`. A whitespace-only output is now omitted under its own category
(`whitespace_only_tool_results`), the textless one keeps `empty_tool_results`, and both are `omitted`.

The same class — text the writer copies verbatim that the target's own reader normalises away — had two
more members on the same surface, found by an executed probe after the finding rather than by a static
argument, and each is reachable from a real read:

- a **whitespace-only dialogue**, which is the shape a tool-result record carries when it also holds a
  blank text block: the inherited Claude parser's blank-message guard drops a blank record only while it
  carries no tool evidence, so such a record arrives with `Content`/`Dialogue` of `"\n"` beside its
  result. Written as a message, it was dropped by the target reader (`handleResponseItem` returns before
  appending when the text is blank after trimming) and the structural check failed
  (`dialogue 2 is in the target document but not in the file: "user_message:  "`). The text is now
  omitted under its own category (`whitespace_only_dialogue`), and the record's tool evidence still
  travels — a call and its output carry no role of their own;
- a **compact summary that ends in whitespace**: the inherited Claude parser joins a compact summary's
  text blocks without trimming (`extractCompactSummary`), so a summary the provider wrote verbatim
  reached the writer and was compared untrimmed against the reader's trimmed one
  (`compaction boundary 1 differs (written "Summary of the work so far. ", read back "Summary of the work so far.")`).
  The comparison now identifies a boundary by its trimmed summary, exactly as it already identified a
  result by its trimmed output.

The reader's fourth normalisation on this surface — an empty argument object is read as no arguments at
all — was found **unpinned** by its mutation (nothing went red), so it has a case of its own now:
`TestCodexRoundTripFoldsAnEmptyArgumentObjectAsNoArguments`.

Regressions for the finding and the class: `TestContinueOmitsOutputTheInheritedReaderCannotReadBack` (the
LF-only result through a real Claude transcript: omitted and counted, the file holds no
`function_call_output`, structure verified under `--verify none`), `TestContinueOmitsDialogueTheInheritedReaderReadsAsNothing`
(the blank dialogue, same measure, with the call and its output still carried), and
`TestContinueRoundTripsACompactionSummaryTheReaderTrims` (the summary is carried exactly as recorded and
the check does not fail); `TestCodexRolloutReportsEveryCategoryItKnows` now requires the two new
categories — twenty in all — and the new case above pins the call-side folding.

Every rule of the touched surface was pinned by executing one mutation, running the test that claims the
rule, and restoring the three touched `.go` files from byte-identical copies (SHA-256 compared after every
restore; the combined digest `66bdfcede9235aa7…` was the same before the first mutation and after the
last):

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a conversion report carries no source content (core) | `new(conversionSourceReport(conversation))` → `new(reportFrom(conversation))` | `TestContinueReportCarriesNoSourceContent` FAIL: `the report carries the opening prompt's preview "Summarize the retry policy in internal/retry/policy.go."`; `the JSON report carries a first_user_message field`; `the JSON report carries source message text "…"` |
| … and on the CLI's wire document | the same revert | `TestContinueJSONCarriesNoSourceContent` FAIL twice: `the [--json] report carries a first_user_message field` and `… carries the source's opening prompt "…"`, and the same for `[--dry-run --json]` |
| a whitespace-only result is not written as evidence | the guard back to `if text == "" {` | `TestContinueOmitsOutputTheInheritedReaderCannotReadBack` FAIL: `carried = {… ToolResults:1 …}`, `whitespace_only_tool_results … Count:0`, `the installed session holds a function_call_output record the inherited reader cannot read back`, `structure = {State:not-verified … tool result 1 is in the target document but not in the file …}` — the finding's own shape |
| such a result is counted under its own category, not as a textless one | `b.lossWhitespaceResults++` → `b.lossEmptyResults++` | the same case FAIL: `whitespace_only_tool_results … Count:0` and `empty_tool_results … Count:1 … want 0: the read held text, it was only whitespace` |
| a whitespace-only dialogue is not written as a message | the new switch arm made unreachable (`&& message.Ordinal < 0`) | `TestContinueOmitsDialogueTheInheritedReaderReadsAsNothing` FAIL: `carried = {… Messages:2 UserMessages:2 …}`, `whitespace_only_dialogue … Count:0`, `structure = {State:not-verified … dialogue 2 is in the target document but not in the file: "user_message:  "}` |
| a result's key is its trimmed output | `roundTripResultKey`'s `strings.TrimSpace(text)` → `text` | `TestContinueConvertsInstallsAndReadsBackTheFixture` FAIL: `tool result 1 differs (written "… MaxAttempts = 4 ", read back "… MaxAttempts = 4")` |
| a boundary's key is its trimmed summary | `roundTripBoundaryKey`'s `strings.TrimSpace(text)` → `text` | `TestContinueRoundTripsACompactionSummaryTheReaderTrims` FAIL: `compaction boundary 1 differs (written "Summary of the work so far. ", read back "Summary of the work so far.")` |
| an empty argument object is the same evidence as no arguments | `if input == "{}" \|\| input == "[]"` → `if input == "{ }" \|\| input == "[ ]"` | **nothing went red** — the rule was unpinned. The case this round adds FAILs under the same mutation: `tool call 1 differs (written "toolu_1 Read {}", read back "toolu_1 Read ")` |
| a result with no text at all is still omitted (the rule that predates this round) | the omission guard replaced by `if false {` | `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce` FAIL: `carried = {… ToolResults:3 …}, want the two results the archive retains`, and its structural check (`tool result 1 differs (written … read back …)`) |

Tests, gate: `internal/ccc/...` top-level `--- PASS` cases 113 → **118** (`internal/ccc` package 110 →
**115**; `cmd/ccc` 22 → **23**; `go test -tags fts5 -count=1 -v` per scope; before = a detached worktree
of `e578646`, removed); the suite's own `selftest` step 153, unchanged (no `scripts/` change this round).

The native harness re-ran on both fixtures with a real turn each
(`--model-provider tencent-litellm --model deepseek-v4.1-flash-fast --resume-prompt …`): **13/13 checks
passed, 0 failed, 0 skipped** each, exit 0, `20 loss categories` each — the count the script reads out of
the conversion's own report — and no conversion or structural verdict changed for either fixture; §3's
two blocks were those runs (round 10's §15 re-ran the check on a tree with two more loss categories and
re-recorded §3's blocks; the count above is round 7's, as its own runs printed it).

Docs changed with the code: this record's §1 note (the category count), §3 (both runs re-recorded), §5
(the tooling run's own answer wording and the count), §6 (the refreshed `go test` and gate blocks, the new
rules and their pins), §10 and §11 (their pointers to §3's blocks) and this §12.
`docs/decisions/OPEN-QUESTIONS.md` gains `Q-0002-16`, the two whitespace-only categories and why the text
is omitted rather than carried. `AGENTS.md` is unchanged: no repository layout, gate step, command, read
order or agent rule moved this round (the report states it). Goal bookkeeping: 0002 is still
`Implemented …; awaiting operator acceptance`, so nothing moves to `docs/goals/archive/` and
`docs/goals/README.md`'s row is already accurate.

## 13. Repair round 8 — a message the reader projects away; a failure report without content; link rules on a host that cannot link

Round 8's review left three P2 findings, all fixed here. Its carried-tree parity audit again found no
increment-attributable defect (the stored bytes agree with the trusted manifest, the working tree is
hashed directly, ignore rules and failed git probes fail or go INCOMPLETE), so nothing in the carry
changed; its one note about the manifest — that agreement with the manifest is not an independent
authentication of the manifest against an upstream checkout — is what `scripts/upstream-compare.py` and
the recorded comparison are for, unchanged.

**1 — valid Claude dialogue could fail the structural check** (`internal/ccc/codex_install.go`,
`internal/ccc/continue.go`). The inherited Codex reader reads *no message* for a user text block that
opens with `# AGENTS.md`: `preprocessCodexUserTextBlocks` classifies such a block as the context Codex
itself injects (`internal/parser/codex.go`). A Claude transcript may hold a genuine prompt with that
prefix, and the writer carries it verbatim — so the read-back showed the message missing from the file
that holds it, and the run exited nonzero **even with `--verify none`**, because the structural check
always runs. The fix keeps the message and makes the check ask the file:

- `compareCodexRoundTrip` now takes the installed file's own bytes. A written dialogue item the read-back
  shows no message for is resolved by `fileHoldsText`: the file holds the text as the JSON string a record
  carries it in (spelled with the writer's own encoder settings, and an unescaped quote can only open a
  string in a well-formed line, so a match is a string token whose value is exactly that text) → the item
  is counted in `not_read_back_as_dialogue`; otherwise it stays in the ordered comparison and is named
  there as the missing message it is. The rule is scoped to dialogue because that is where the reader's
  projection applies — a call, a result and a boundary are compared through keys the reader derives from
  the record itself;
- the count reaches `verification.structure.not_read_back_as_dialogue` (a new, additive JSON field) and
  `structureWarnings`, so a caller that reads only the exit code or the human block's head is told that
  the read-back shows fewer messages than the file holds. The conversion's `losses[]` is untouched: the
  message *is* carried, and the disclosure belongs to the verification, not to a loss the writer did not
  make. Q-0002-17 records the decision and the reversal. The regression is native-shaped and end to end —
  a real Claude transcript whose turn opens with `# AGENTS.md`, converted and installed through the CLI —
  and runs the three ways the check can: installed, installed with `--verify none` (the finding's own
  spelling) and a dry run (`TestContinueInstallsAPromptTheReaderProjectsAway`, plus the unit-level
  `TestCodexRoundTripCountsWhatTheReaderProjectsAway` for the file-holds-it / file-does-not split).

**2 — structural failure reports exposed source content** (`internal/ccc/codex_install.go`).
`summariseItem` returned up to 120 bytes of the item itself — dialogue, tool arguments, tool outputs — and
`compareSequences` embeds that in `Structure.Detail`, which is serialized into the JSON document and
printed as a warning. `describeItem`/`describeText` replace it: a difference now names the category, the
ordinal, the kind or the call and tool it belongs to, and identifies the text by its size and its SHA-256
(64 hex digits, taken over the text as the comparison holds it). An item whose whole text fits is no
longer quoted either, so there is no "short item" arm left to leak through.

**3 — positive symlink regressions depended on host privileges**
(`scripts/upstream-parity.py`, `scripts/tests/test_upstream_parity.py`). `make_link` fell back to writing
a regular file where `os.symlink` is unavailable or denied, and the positive cases then asserted *failure*
instead — so on such a host, removing the separator fold, or the modified-link exemption, satisfied them.
The shape question now goes through a named seam, `symlink_target(root, path)` (the filesystem answers it
by default), and the cases run **both** shapes their host can hold a link in: the file form with the seam
answering the target it holds, and, where the host really can create one, the real symlink. "Is anything
at this path?" and "what bytes does it hold?" stay with the filesystem, because git stores a link as its
target text and the file form's bytes *are* the link's content.

Every rule of the touched surface was pinned by executing one mutation, running the test that claims the
rule, and restoring the file from a byte-identical copy (`/tmp/mut/*`, taken before the first mutation;
`sha256sum -c` OK after every restore for `internal/ccc/codex_install.go`, `internal/ccc/continue.go` and
`scripts/upstream-parity.py`, the three mutated sources). The one mutation on a test file — the simulated
no-symlink host in `scripts/tests/test_upstream_parity.py` — was reverted by hand and the suite re-run
green (110 cases) before the next mutation:

| Rule | Mutation (the line reverted) | Observed |
| --- | --- | --- |
| a message the read-back does not show, which the file holds, is counted rather than reported missing | `fileHoldsText` → `return false` | `TestContinueInstallsAPromptTheReaderProjectsAway` FAIL **on the exit code the finding named**, in all three arms: `[installed] exit 1, want 0`, `[installed, --verify none] exit 1, want 0`, `[dry run] exit 1, want 0`; `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL `notReadBackAsDialogue = 0, want 1` |
| … and a message the file does *not* hold is still a missing message | `fileHoldsText` → `return true` | `TestStructureVerificationNamesWhatTheFileDoesNotHold` FAIL `the structure check accepted a report the file does not support`; the unit control FAIL `differences = [], want the written message named by its own size and digest` |
| the file must hold the text as a record's JSON string, not as a byte run | `fileHoldsText`'s encoded containment → `strings.Contains(string(installed), text)` | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL twice: `the_file_holds_the_text` (`differences = ["dialogue 1 differs (written user_message (63 bytes, sha256 …), read back assistant_message …)"]`) and `the_text_is_in_the_file,_but_not_as_a_record's_own_string` (`notReadBackAsDialogue:1`, `want the message reported missing`) |
| a difference report carries no compared content | `describeText` gained `, %q` | `TestCodexRoundTripReportNamesDifferencesWithoutTheirText` FAIL ×4 (`the difference report carries the source content it compared: "summarize the retry policy in internal/retry/policy.go"`, `… "internal/retry/client.go"`, `… "the credential file's own private contents"`, `… "another private tool output, of the same shape"`); `TestStructureVerificationNamesWhatTheFileDoesNotHold` FAIL with `dialogue 6 … assistant_message (32 bytes, sha256 …, \"a message the file does not hold\")` |
| the projection reaches the report | `structureWarnings` → `return nil` | `TestContinueInstallsAPromptTheReaderProjectsAway` FAIL both arms: `no warning names the projection; warnings = []` (dry run), the PATH warning only (installed) |
| … including on the dry-run branch | the dry-run branch's `structureWarnings` call removed | the same case FAIL on `[dry run]` only (`warnings = []`), the installed arm green — the mutation was scoped |
| a link's target is folded in the platform's own separator spelling | the fold branch made unreachable (`if False:`) | `test_a_link_target_in_the_platforms_own_spelling_is_matched` and `test_a_respelled_link_git_reports_changed_is_not_a_second_failure` FAIL in **both** shapes: `parity: FAIL — 1 carried file(s) differ from the pinned revision` |
| a respelled link git reports changed is not a second failure | `and not respelled` dropped from the modified-path rule | `test_a_respelled_link_git_reports_changed_is_not_a_second_failure` FAIL in both shapes: `1 carried file(s) differ from what git stores for them` |
| the shape rule consumes the link seam | `is_link = symlink_target(…) is not None` → `target.is_symlink()` | `test_a_symlink_record_hashes_the_link_target` and `test_a_link_target_that_platform_does_not_respell_is_drift` FAIL in the file-form shape: `1 carried path(s) changed file type` |
| the file-form disclosure is the seam's answer too | the counter's `symlink_target(…) is None` → `not (root / record.path).is_symlink()` | `test_a_symlink_record_hashes_the_link_target` FAIL: `'carried link(s) held as a file' unexpectedly found in "… symlinks hashed by their link target: 1\ncarried link(s) held as a file with the link's tar…"` |

Finding 3's own claim — that the *pre-fix* cases pass on a host that cannot create symlinks whether or not
the rule holds — was executed rather than argued: with `os.symlink` replaced by a function that raises
`NotImplementedError`, the pre-fix test file (read out of `HEAD` into a throw-away module) is **green both
with and without the fold rule** (`verdict: green — the rule is unpinned here`), while the fix's cases
under the same simulation are **red** with the fold mutation and green on the fixed checker (the 43
`ParityCase` cases pass). The first version of the new helper collected both shapes' patches up front and
left the *real* symlink on disk for both runs, which masked the shape-seam mutation — the helper is a
lazily-yielding generator for exactly that reason, and the mutation above is what proved it.

Tests, gate: top-level `--- PASS` cases 141 → **144** across `./cmd/ccc/... ./internal/ccc/...` (before:
`internal/ccc` 115, `./internal/ccc/...` 118, `cmd/ccc` 23; after: `internal/ccc` **117**,
`./internal/ccc/...` **120**, `cmd/ccc` **24**; `go test -tags fts5 -count=1 -v`, `--- PASS` lines at top
level). The Python suite's own `selftest` step is **153**, unchanged: this round rewrote existing link
cases and added no test method, and `test_upstream_parity` alone is 110 cases.

The native harness did **not** re-run, and its §3 numbers stand: it reads `conversion.carried`, the
installation block and its own `ccc inspect` read-back, none of which this increment changes, and the
projection disclosure is empty for both of its fixtures — measured, not assumed: `ccc continue --json`
over `9f2c1d47-…` and `b6d93f07-…` (the two sessions `scripts/codex-native-check.py` uses) exits 0,
prints `structure: verified` with no `not_read_back_as_dialogue` field and no projection warning, and
reports the same carried counts as §5/§3 record (5 messages; 1 and 3 tool calls).

Docs changed with the code: this record's §6 (the refreshed `go test` and gate blocks, the note on what
`gofmt` counts, and four table rows — the reader-projection rule, the no-content rule, and the
host-independent link rules) and this §13; `docs/harness.md`'s `parity` section (the shape answer is a
named seam now) and its Windows-answers list (the link bullet said the re-spelled shape was pinned "where
the host can create one", which was the dependency the finding named). `docs/decisions/OPEN-QUESTIONS.md`
gains **Q-0002-17** (how a conversion reports a message the reader reads as injected context, and why it
differs from Q-0002-16's blank text). `AGENTS.md` is unchanged: no repository layout, gate step, command,
read order or agent rule moved this round (the new file is a test inside an existing package, and the seam
is inside an existing script), which the stage report states. Goal bookkeeping: 0002 is still
`Implemented …; awaiting operator acceptance`, so nothing moves to `docs/goals/archive/` and
`docs/goals/README.md`'s row stays as it is.

## 14. Repair round 9 — the question the check asks the file; both shapes of the reader's projection; the reading relation

Round 9's review left two P2 findings, both about what round 8's fix asked the installed file, and both
fixed here. Its carried-tree parity audit found no increment-attributable defect again — the checker's own
mechanisms are the same ones round 8's §13 audited, and this round touched no carried file at all, which
the gate's `parity` step proves (5 553 carried files still byte-identical to the pin).

**1 — a session whose model-visible record was gone verified** (`internal/ccc/codex_install.go`). The
target writes every message twice: the `response_item` its model history is built from, and an `event_msg`
carrying the same text, which is what its own view of the conversation is built from — and the inherited
reader reads no message from the event. Round 8's `fileHoldsText` asked the file for the text as a JSON
string *anywhere in it*, so a message whose `response_item` had been dropped, with its event left in place,
was accepted: the event held the words, and the check reported the session verified while the target's
model history no longer held the message. A text quoted by a tool output answered the same way, and a
single occurrence of the text answered for every written message that had it. The oracle is now the file's
own dialogue record. `installedDialogueRecords` walks the installed bytes and counts, per identity, every
`response_item` whose payload is a message, whose role is the user's or the assistant's, and whose content
holds a block of the type this writer writes for that role carrying the message's own text
(`codexInstalledRecord.dialogueMessage`); the count is consumed as the comparison resolves messages, so one
record answers for one written message and a second message with the same text needs a second record. A
record with another record type, another payload type, another role, another block type, or an empty text
is not the message, and a line the walk cannot read is skipped rather than ending the walk.

**2 — a prompt that carried text after an envelope failed a correct run**. The inherited reader removes the
injected-context envelopes a user text block opens with (`preprocessCodexUserTextBlocks`), so a prompt
written verbatim as `<INSTRUCTIONS>quoted rules</INSTRUCTIONS>\nReview the changes` is read back as
`Review the changes`. Round 8 resolved the written message away — the file held its text — and then
compared the reader's *shorter* reading against the document, where it stood as evidence the file held a
message the conversion had not written: two differences, exit 1, in every arm (`--verify none` and
`--dry-run` included, because the structural check always runs). The comparison is now an ordered walk over
the written dialogue and the read-back (`compareDialogue`): a read-back message is accounted for by a
written message when it is that message's own text, or the reader's reading of it — a run of the message's
characters in their own order and under the message's own kind, the file still holding the message's record
(a reading is evidence about a message the file has; `pairReadBackMessage`, `readsAsDialogueMessage`,
`holdsTextInOrder`) — and the written messages the walk passes over are
resolved by the file: counted as the reader's projection where their record is there, reported missing
where it is not. A read-back message no written message accounts for is
reported as dialogue the document does not hold, unless the written message the walk stands on is missing
from the file too, in which case the two are reported as the same item altered. The relation is a
*structural* property of a deletion-based reading, not a copy of the reader's envelope list: the check
still does not re-implement `preprocessCodexUserTextBlocks`, which the carry rule forbids. The count's name
follows its meaning: `verification.structure.not_read_back_verbatim`, previously `not_read_back_as_dialogue`
(a partial projection *is* read back, just not verbatim).

Both findings were reproduced before the fix, in a copy of the review head (`18d8fbe`, `/tmp/prefix-r9`)
carrying this round's test files with the count's field renamed back to the pre-fix name so they compile
against the pre-fix code:

| Finding | Executed against the pre-fix code | Observed |
| --- | --- | --- |
| 1, unit | `go test -tags fts5 -count=1 -run TestCodexRoundTripCountsWhatTheReaderProjectsAway ./internal/ccc` | `…/the_reader_reads_the_text_the_envelope_preceded` FAIL `differences = ["dialogue 1 differs (written assistant_message (18 bytes, sha256 e46d83c1…), read back user_message (18 bytes, sha256 797fac90…))" "dialogue 2 is in the file but was not written: assistant_message (18 bytes, sha256 e46d83c1…)"], want none` — the shorter reading reported as unwritten evidence, and the message after it misreported with it |
| 1, end to end (`cmd/ccc`) | `go test -tags fts5 -count=1 -run TestContinueInstallsPromptsTheReaderReadsDifferently ./cmd/ccc` | all three arms of `[the prompt carries text after the envelope]`: `[installed] exit 1, want 0`, `[installed, --verify none] exit 1, want 0`, `[dry run] exit 1, want 0`; stderr: `the installed session does not hold the evidence in the target document: dialogue 1 differs (written assistant_message …), …; dialogue 2 is in the file but was not written: assistant_message …` |
| 2, unit | the same unit command | `…/the_text_is_only_in_the_turn's_own_event` FAIL `notReadBackVerbatim = 1, want 0: the file does not hold the message's record` and `differences = [], want the written message named by its own size and digest`; the same two lines for `…/the_record's_role_is_the_assistant's` and `…/the_content_block_is_the_type_this_writer_writes_for_the_other_role` |
| 2, occurrence | the same unit command | `TestCodexRoundTripCountsARecordOncePerWrittenMessage` FAIL `notReadBackVerbatim = 2, want 1` — one record answered for both written messages |
| 2, file level | `go test -tags fts5 -count=1 -run TestStructureVerificationAsksTheFileForTheMessageRecord ./internal/ccc` | `the structure check accepted a session whose message record is gone` |

Every rule of the touched surface was then pinned by executing one mutation in a scratch copy of this
working tree (`/tmp/mut-r9`, whose `internal/ccc/codex_install.go` and `internal/ccc/continue.go` were
hashed against this tree before the first mutation, and restored and re-hashed after each run: byte-identical
every time), running the test that claims the rule, and recording the failure:

| Rule | Mutation (the line reverted) | Observed |
| --- | --- | --- |
| a written message the read-back does not show verbatim is counted when the file's record holds it, and stays a missing message when it does not | `installedDialogueRecords` → `return map[string]int{}` (the file is never asked) | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL in both projection rows and the leading-line row (`differences = ["dialogue 1 is in the target document but not in the file: user_message (63 bytes, sha256 ced7dc71…)"], want none`, `notReadBackVerbatim = 0, want 1`), `TestCodexRoundTripCountsARecordOncePerWrittenMessage` FAIL, `TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor` FAIL ×6; `TestContinueInstallsPromptsTheReaderReadsDifferently` FAIL in **all six arms** (`exit 1, want 0`) |
| …and it is the *record*, not the text anywhere: the record's own type | `record.Type != codexRecordResponseItem` dropped | `…/the_record_is_another_kind_of_record` FAIL `notReadBackVerbatim = 1, want 0` |
| …the payload's message type | `record.Payload.Type != codexItemMessage` dropped | `…/the_payload_is_another_kind_of_message` FAIL, same two lines |
| …the role | assistant records read as the user's message | `…/the_record's_role_is_the_assistant's` FAIL, same two lines |
| …the content block's type, which is the writer's for that role | the user role's content type swapped for the assistant's | `…/the_content_block_is_the_type_this_writer_writes_for_the_other_role` FAIL, same two lines (plus 12 other cases) |
| …and an empty text is no message | `block.Text != ""` dropped | `TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage` FAIL `notReadBackVerbatim = 1, want 0: an empty block is no message` |
| one record answers for one written message | `holdDialogueRecord` consumes nothing | `TestCodexRoundTripCountsARecordOncePerWrittenMessage` FAIL `notReadBackVerbatim = 2, want 1` |
| a read-back message is a written message's reading only as a run of its characters, in order, under its own kind | `readsAsDialogueMessage` → `return true` | `TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor` FAIL: the three negative rows *and* the order row report no difference |
| …under its own kind | `readKind != kind` dropped | `…/the_reading_is_recorded_under_the_written_message's_own_kind` FAIL: `differences = []` where the message under another role must be unaccounted for |
| a reading is only evidence about a message the file holds | `records[key] == 0` dropped from the pairing | `…/the_shorter_reading_does_not_stand_in_for_a_missing_record` FAIL: `differences = []`, `notReadBackVerbatim = 1` — the reading stood in for a record the file does not hold |
| the walk pairs in the file's own order | `pairReadBackMessage(intended, cursor, …)` → `0` | `…/the_read-back's_order_is_the_file's_own_order` FAIL `differences = ["dialogue 2 is in the target document but not in the file: assistant_message (18 bytes, sha256 e46d83c1…)"], want the out-of-place read-back message named` |
| a line the walk cannot read is skipped, not the end of the walk | `json.Unmarshal` error → `return records` | `…/a_line_that_is_not_a_record_comes_first` FAIL `differences = ["dialogue 1 is in the target document but not in the file: …"], want none` |
| the count reaches the verification detail | `detail()` → `""` | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL ×3: `the verification detail = "", want the count named and no content` |
| the count reaches the report's warnings | `structureWarnings` never fires | `TestContinueInstallsPromptsTheReaderReadsDifferently` FAIL in all six arms: `no warning names the projection` |

Two of the mutations first written for this table were *not* valid experiments and are recorded as such
rather than as pins: replacing `holdDialogueRecord`'s body left `pairReadBackMessage` reading the same map
directly (an internally inconsistent mutation, which made the partial-projection arms pass), and dropping
the content-type *comparison* left the `contentType` variable unread (a build failure, not a test result).
The first was replaced by emptying the map (`installedDialogueRecords`), and it also exposed a real
sharpening: the projection branch now counts only where `holdDialogueRecord` consumed the record
(`if held := holdDialogueRecord(…); held && projected`), so the count is always backed by a record the file
was asked for; the second by swapping the role's content type, which is the same rule stated as a bug a
writer could have.

The native harness (`scripts/codex-native-check.py`) was not re-run: it needs the target harness installed
and, for a real turn, model access, and its inputs are the ones this round did not touch — the conversion's
carried counts, the installation block, and its own `ccc inspect` read-back — so §3's numbers stand. That
the projection disclosure is empty for both of its fixtures was measured rather than assumed: `ccc continue
--json` over `9f2c1d47-…` and `b6d93f07-…` (throw-away `HOME`, `AGENTSVIEW_DATA_DIR` and `CODEX_HOME`, the
fixture tree as the source, and a target binary no host has) exits 0 for both, prints `structure: verified`
with the `not_read_back_verbatim` field absent and no projection warning — the one warning each run carries
is the missing target harness — and reports the carried counts §5/§3 record (5 messages; 1 and 3 tool
calls).

Tests, gate: top-level `--- PASS` cases 144 → **149** across `./cmd/ccc/... ./internal/ccc/...` (before:
`internal/ccc` 117, `./internal/ccc/...` 120, `cmd/ccc` 24, `parity` 3 — a detached `git archive` checkout
of `18d8fbe` in `/tmp/head-r9`; after: `internal/ccc` **122**, `./internal/ccc/...` **125**, `cmd/ccc`
**24** (the CLI case was rewritten in place, not added), `parity` **3**; `go test -tags fts5 -count=1 -v`,
`--- PASS` lines at top level, `--- FAIL` 0). The Python suite's own `selftest` step is **153**, unchanged
(this round touched no `scripts/` file). `python3 scripts/gate.py` on this tree — **exit 0, `GATE: PASS`**
(not partial; the block is the one in §6).

Docs changed with the code: this record's §6 (the refreshed `go test` and gate blocks and the note on what
`gofmt` counts — the round-8 test file is committed now, so tracked and on-disk counts agree at 3 582) and
this §14, plus two rows of §6's rule table (the projection rule now names both shapes and the record oracle,
and a new row names the record shape, the occurrence count and the reading relation). `docs/decisions/OPEN-QUESTIONS.md`
gains **Q-0002-18** (what the check asks the file, and how a partial projection resolves; it supersedes the
mechanism half of Q-0002-17, not its default). `AGENTS.md` is unchanged: no repository layout, gate step,
command, read order or agent rule moved this round, which the stage report states. Goal bookkeeping: 0002 is
still `Implemented …; awaiting operator acceptance`, so nothing moves to `docs/goals/archive/` and
`docs/goals/README.md`'s row stays as it is.

## 15. Repair round 10 — the installed file's dialogue order; the provider's own classification of the read

Round 10's review left two P2 findings, both fixed here. Its carried-tree parity audit found no
increment-attributable defect again — the mechanisms it re-read are the ones §13 and §14 audited, and this
round touched no carried file, which the gate's `parity` step proves (5 553 carried files still
byte-identical to the pin) — and its one note (the normalized `skip` path's shape check accepts a symlink
for hashing and checks the followed file's executable bit) is the limitation the review itself filed as
predating the increment; it is unchanged.

**1 — a file whose projected dialogue records stood in another order verified**
(`internal/ccc/codex_install.go`). `installedDialogueRecords` reduced the installed file's dialogue to a
multiset and `compareDialogue` consumed it, so a written message the read-back does not show — the
inherited reader projects a user text block that opens with an injected-context envelope away, and a
genuine prompt with that prefix is carried verbatim — resolved against a record standing anywhere in the
file. Two distinct prompts the reader projects away, installed in the other order, left the multiset
unchanged and the read-back showing the same assistant message, so the check returned `verified` for a
file whose model-visible dialogue was not the document's. The records are now kept in the file's own order
(`dialogueRecords`: the ordered keys, and the positions still pending per identity) and every written
message is resolved against the record that stands where the document writes it —
`dialogueRecordInPlace`, `dialogueRecordOutOfPlace` (consumed where it stands, so one record still answers
for one written message, and reported as "out of order") or `dialogueRecordAbsent` (reported as a missing
message) — and a record no written message resolves is reported too: a record the reader projects away
reaches no read-back message, so the file's own records are the only side that can report it. One further
consequence of asking the file for the record at every position: the message's own record is asked for
even where the read-back shows its text, because the inherited reader reads `input_text`, `output_text`
and `text` blocks alike (internal/parser's `extractCodexTextBlocks`), so a record of another shape can put
the message's words in the read-back while the file does not hold the writer's dialogue record.

**2 — a continuation could present a degraded read as an ordinary success**
(`internal/ccc/continue.go`, `internal/ccc/conversation.go`, `internal/ccc/inspect.go`,
`internal/ccc/codex_target.go`). `Continue` accepted every known source provider but carried the
provider's own classifications of the read — a summary transcript, a result the provider says is not
current for its data version — only in `conversion.source_fidelity`; no warning named them and the loss
list did not either, so the Grok session whose chat history is missing (one synthesized fallback prompt)
converted, installed and verified as an ordinary success. The words are now built once
(`fidelityWarnings`, shared with `Inspect`, so an inspection and a continuation say the same thing) and
reach `warnings[]` and the human block's warning lines; `conversion.losses[]` gains `summary_transcript`
and `source_needs_retry` (both `carried-with-caveat`, count 1 when the classification holds and 0
otherwise, with the provider's own retry reason in the detail), beside `malformed_source_lines`, which had
already reported the read's own completeness. Q-0002-19 records the decision and what reverses it.

Both findings were reproduced before the fix, on a detached `git archive` checkout of the review head
(`02b9ed2`, `/tmp/prefix-r10`) carrying this round's test files:

| Finding | Executed against the pre-fix code | Observed |
| --- | --- | --- |
| 1, the swapped projections | `go test -tags fts5 -count=1 -run TestCodexRoundTripReportsDialogueRecordsOutOfOrder ./internal/ccc` | `…/the_two_projected_records_swapped_are_reported` FAIL `the check accepted a file whose dialogue records stand in another order: {differences:[] notReadBackVerbatim:2}`; `…/a_record_the_document_does_not_write_is_reported` FAIL `differences = "", want the record the document does not write named by its own size and digest` |
| 1, over a real installed file | `go test … -run TestStructureVerificationReportsDialogueRecordsOutOfOrder` | `…/a_document_whose_prompts_the_reader_projects_away` FAIL `the check accepted a file whose projected dialogue records stand in another order`; `…/a_fixture_whose_first_two_records_are_swapped` FAIL `the difference report does not name the out-of-place message by its size and digest: "… dialogue 2 is in the file but was not written: …"` — the pre-fix walk reported some other difference, never the record's own place |
| 1, a record of another shape whose text the reader reads back | `go test … -run TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord` | `…/the_reader_reads_the_text_back_from_another_record_shape` FAIL `differences = [], want the written message named by its own size and digest` |
| 2, the warnings | `go test … -run TestContinueDiscloses` | `TestContinueDisclosesASummaryTranscriptRead` FAIL `the classification is in the fidelity block but not in the report's warnings: []`; `TestContinueDisclosesARetryableRead` FAIL `the retry and the provider's own reason are not in the warnings: []` |
| 2, the loss list | the same command | `the report does not name the "summary_transcript" category`; `the report does not name the "source_needs_retry" category` |

Every rule of the touched surface was then pinned by executing one mutation in a scratch copy of this
working tree (`/tmp/mut-r10`, whose three mutated sources were hashed against this tree before the first
mutation and restored and re-hashed after every run — `ALL RESTORED: True`, byte-identical every time),
running the test that claims the rule, and recording the failure:

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| a written message whose record stands elsewhere in the file is a difference | `hold`'s out-of-place branch answered `dialogueRecordInPlace` (the multiset's acceptance) | `TestCodexRoundTripReportsDialogueRecordsOutOfOrder` FAIL `the check accepted a file whose dialogue records stand in another order: {differences:[] notReadBackVerbatim:2}`; `TestStructureVerificationReportsDialogueRecordsOutOfOrder` FAIL in **both** arms (`the check accepted a file whose projected dialogue records stand in another order`, and the fixture arm's wording assertion) |
| a record the document does not write is a difference | the walk over the file's unresolved records removed | `TestCodexRoundTripReportsDialogueRecordsOutOfOrder` FAIL `differences = "", want the record the document does not write named by its own size and digest` |
| the message's own record is asked for even where the read-back shows its text | the paired branch's `account(paired)` → `records.hold(intended[paired]) == dialogueRecordInPlace` (consume, report nothing) | `TestCodexRoundTripAsksTheFileForItsOwnDialogueRecord` FAIL `…/the_reader_reads_the_text_back_from_another_record_shape: differences = [], want the written message named by its own size and digest` |
| records are consumed in the document's order, not the pairing's | the paired message's record consumed before the written messages the pairing skipped | `TestCodexRoundTripReportsReadBackDialogueNoWrittenMessageAccountsFor` FAIL in all five rows (`differences = ["dialogue 1 is in the file but was not written: …" "dialogue 2 is out of order in the file: …"]`) |
| a summary transcript is disclosed in the report's warnings | the summary branch removed from `fidelityWarnings` | `TestContinueDisclosesASummaryTranscriptRead` FAIL `the classification is in the fidelity block but not in the report's warnings: []`; `TestInspectReportsASummaryTranscriptClassification` FAIL `the summary classification is not disclosed: []` |
| a summary transcript is a category in the conversion's loss list | `Count: flagCount(… TranscriptFidelitySummary)` → `flagCount(false)` | `TestContinueDisclosesASummaryTranscriptRead` FAIL `summary_transcript = {… Count:0 …}` |
| a retryable read is disclosed in the report's warnings | the retry branch removed from `fidelityWarnings` | `TestContinueDisclosesARetryableRead` FAIL (the warnings assertion); `TestInspectReportsARetryableResultWithItsMessages` FAIL |
| a retryable read is a category in the conversion's loss list | `Count: flagCount(b.sourceFidelity.NeedsRetry)` → `flagCount(false)` | `TestContinueDisclosesARetryableRead` FAIL `source_needs_retry = {… Count:0, Detail: "… (could not read the Codex session it was forked from (2d9b7e10-…) …"}` |

Tests, gate: top-level `--- PASS` cases 149 → **153** across `./cmd/ccc/... ./internal/ccc/...` (before:
`internal/ccc` 122, `./internal/ccc/...` 125, `cmd/ccc` 24 — measured on a detached `git archive` checkout
of `02b9ed2` in `/tmp/head-r10`; after: `internal/ccc` **126**, `./internal/ccc/...` **129**, `cmd/ccc`
**24**, the CLI's own cases untouched, `--- FAIL` 0; `go test -tags fts5 -count=1 -v`, `--- PASS` lines at
top level). The Python suite's own `selftest` step is **153**, unchanged (this round touched no
`scripts/` file). `python3 scripts/gate.py` on this tree — **exit 0, `GATE: PASS`** (not partial; the
block is the one in §6). `gofmt` still counts 3 582 files, and this round added none: five sources and
three test files were edited.

The native harness re-ran on both fixtures with a real turn each (`--model-provider tencent-litellm
--model deepseek-v4.1-flash-fast --resume-prompt …`): **13/13 checks passed, 0 failed, 0 skipped** each,
exit 0, `22 loss categories` each — two more than round 7's runs printed, the two categories this round
added — and no conversion or structural verdict changed for either fixture; §3's two blocks are those
runs, re-recorded there (which supersedes §12's pointer that §3 held its runs).

Docs changed with the code: this record's §1 note (the category count), §3 (both runs re-recorded, with
the view and answer blocks that belong to them), §5 (the tooling run's quoted view, the `Read` row's
answer wording, and the count), §6 (the refreshed `go test` and gate blocks, and the two new rule rows)
and this §15. `docs/decisions/OPEN-QUESTIONS.md` gains **Q-0002-19** (how a continuation discloses the
provider's own classification of the read, and why the route is disclosed rather than refused).
`AGENTS.md` is unchanged: no repository layout, gate step, command, read order or agent rule moved this
round (the changes live in existing package files and test files), which the stage report states. Goal
bookkeeping: 0002 is still `Implemented on feat/0002-claude-to-codex-continuation; awaiting operator
acceptance`, so nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s row stays as it is.

## 16. Repair round 11 — a member identity is not a file; the count is the read-back's texts

Round 11's review left two findings (a P2 and a P3), both fixed here, and both reproduced by execution
before the fix (Table A). Its carried-tree parity audit found no increment-attributable defect again — the
mechanisms it re-read are the ones §13 to §15 audited, and this round touched no carried file, which the
gate's `parity` step proves (5 553 carried files still byte-identical to the pin). Its one note is that
both findings were static traces and that no rehashing, build or test ran on its side.

**1 — continuing a readable SQLite-backed session failed before the conversion**
(`internal/ccc/continue.go`). A multi-session container is read through a *member* identity: the inherited
providers spell it `parser.VirtualSourcePath(container, member)` (`<container>#<member>`), the parse records
that spelling as the session's own path (`parser.ParsedSession.File.Path`), and this reader carries it
unchanged because it is what a caller resolves the member by. `sourceFileDigest` opened that spelling as a
filesystem path unconditionally, so a readable member of an OpenClaw container — the fixture the reader
suite already exercises (`TestReadReportsAnOpenClawContainersMembersInsteadOfPickingOne`) — failed with
`open /tmp/…/openclaw-agent.sqlite#main:good: no such file or directory` before the conversion, for every
`--verify` level and for `--dry-run`; the archive fallback could not help, because the source read had
already answered. The check now asks the read's own identity once (`sourceFileOf`, used by both halves) and
answers with the third state when that identity names no file: `installation.source_check_state` is
`not-verified`, the `detail` names the member and the container it belongs to, no warning claims a move, and
the conversion, the installation and the other verifications run exactly as they do for a file-backed
source. The container's own bytes are deliberately *not* digested: they hold every member the container
carries, so a digest of them answers a different question than the report asks (did *this* conversation's
bytes move?), would report `changed` for a sibling member's write, and would report `unchanged` for a change
that never reached the main file's bytes (a database's write-ahead log, a buffered writer) — a claim the
report must not make. The line the third state is *not* for is kept and now pinned on its own: a read that
reports no path at all, a digest that was never taken and a file that cannot be re-read still stop the run,
and `TestSourceCheckStopsAReadThatNamesNoFileAtAll` holds a read that names no file apart from a read that
names a *member*. The rule reaches the plan's §3 bullet and Q-0002-20.

**2 — an omitted envelope beside a later turn carrying the same text was counted twice**
(`internal/ccc/codex_install.go`). `notReadBackVerbatim` was read off the walk's pairing: a written message
the read-back accounted for through the reader's *reading* relation was counted as one the read-back does
not show, and the pairing is the earliest written message at or after the cursor that the read-back message
accounts for — its own text, or a run of that text's characters in their order. With `# AGENTS.md\nhello`
followed by `hello`, the inherited reader drops the first prompt (`preprocessCodexUserTextBlocks`) and
returns the second verbatim, and the returned `hello` is *both* a run of the envelope prompt's characters
and the later turn's own text: the greedy pairing took the envelope prompt, the walk counted it as
projected, counted the later turn as never read back, and the report named **2** where the read-back does
not show one message. The count is now read from the read-back's own texts rather than from the pairing's
answers: one claim per read-back message (`readBackTexts`), consumed in the document's own order, so a
message whose record the file holds where the document writes it is counted when the read-back holds no
message with its own text for it — the rule the field's own words
(`verification.structure.not_read_back_verbatim`) always stated. The pairing is unchanged and still answers
what it is for — which written message a read-back message is *about*, which is what the differences are
read apart by — so no alignment change was needed for the count, and the class is closed rather than
patched: the count is invariant under any choice of pairing. Both shapes the class has (one later turn
carrying the text, and two) now count **1**; the count read from *claims* is pinned separately
(`…/one read-back message shows one written message`), because the pre-fix walk answered that shape's count
by accident rather than by rule (Table A, last row).

### Table A — both findings reproduced by execution before the fix

The reproduced runs use the reviewed revision's own two source files (`git show HEAD:internal/ccc/…`, so
the code is `de82906`'s) with this round's tests present, and a throw-away probe on the untouched review
head for the abort's own message:

| Finding | Executed against the pre-fix code | Observed |
| --- | --- | --- |
| 1, the container member's abort | `go test -tags fts5 -count=1 -run TestContinueFromAContainerMemberReportsNoFileToCompare ./internal/ccc` | FAIL `TestContinueFromAContainerMemberReportsNoFileToCompare` — `continuing the container member: open /tmp/…/openclaw-agent.sqlite#main:good: no such file or directory`; the probe on the untouched head printed the same error with `conversion = ""` and an empty source-check state |
| 2, the overlapping count | `go test -tags fts5 -count=1 -run TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce ./internal/ccc` | both arms FAIL `not_read_back_verbatim = 2, want 1: the omitted prompt's own text is the one the read-back does not show`, detail `2 written message(s) are in the installed session and the inherited Codex reader does not read their own text from it …` |
| 1, the arm that stays a stopped run | `go test … -run TestSourceCheckStopsAReadThatNamesNoFileAtAll` | FAIL — the pre-fix digest traced an empty path (`open : no such file or directory`) instead of the read that reported none |
| 2, the claim rule's own arm | `go test … -run TestCodexRoundTripCountsWhatTheReaderProjectsAway` | PASS — the pre-fix walk satisfied that arm by accident, which is why the rule has a mutation of its own (Table B, B3) |

### Table B — every rule of the touched surface, one executed mutation each

Each mutation was applied to this working tree by a throw-away harness outside the repository (it lives in
`/tmp`, never in the tree), the test that claims the rule was run
(`go test -tags fts5 -count=1 ./internal/ccc/ -run …`), and every file was restored from a byte-identical
copy with the restore checked by SHA-256 after every case (four files — the two sources and the two test
files — 13 cases, every restore byte-identical; the harness refuses to continue on a mismatch):

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| A1 the file's own bytes are shown unchanged after the conversion | `continue.go`: the `unchanged` answer serves `changed` | `TestSourceCheckComparesTheFileBeforeAndAfter` FAIL — `an untouched source was reported as "changed": sha256 9848ab0b…` |
| A2 a file that moved while it was converted is reported changed | `continue.go`: `if after != before` → `if false` | `TestSourceCheckComparesTheFileBeforeAndAfter` FAIL — `a source that changed during the conversion was reported as "unchanged", want "changed"` |
| A3 an archive read takes no digest | `continue.go`: `sourceFileDigest`'s `readFrom != "source"` guard dropped | `TestSourceCheckSaysWhenThereWasNoFileToCompare` FAIL — `digest = "", err = open /absent/file.jsonl: no such file or directory; an archive read has no source file to digest` |
| A3 an archive read answers `not-verified` in its own words | `continue.go`: the archive answer serves `unchanged` | `TestSourceCheckSaysWhenThereWasNoFileToCompare` FAIL — `state = "unchanged", err = <nil>; want a comparison that did not run` |
| A4 a member identity is no file's name, so the check answers it rather than opening it | `continue.go`: `sourceFileOf`'s member branch removed (and its `parser` import, as before the fix) | `TestContinueFromAContainerMemberReportsNoFileToCompare` FAIL — `continuing the container member: open …/openclaw-agent.sqlite#main:good: no such file or directory` |
| A4 the member's answer is the third state, not a claim about a file | `continue.go`: `SourceCheckNotVerified` → `SourceCheckUnchanged` for a non-file identity | `TestContinueFromAContainerMemberReportsNoFileToCompare` FAIL — `source_check_state = "unchanged", want "not-verified": no file holds the member's own bytes` |
| A5 a read that names no path is a stopped run, not a state of a file | `continue.go`: `sourceCheckAfter`'s empty-path guard removed | `TestSourceCheckStopsAReadThatNamesNoFileAtAll` FAIL — `err = the source digest before the conversion was not taken, want a stopped run naming the read that reported no path` (the guard is what puts the read's own name in the error) |
| A6 a source file that cannot be re-read is a stopped run | `continue.go`: the re-read failure answered `unchanged` | `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall` FAIL — `a run whose source vanished after the installation reported success` |
| A7 only a changed source warns about a move | `continue.go`: `sourceChangedWarnings`' state test inverted | `TestSourceChangedWarningsNameTheMove` FAIL — `a "unchanged" source check produced warnings: [the source session changed while it was converted: …]` |
| B2 a message the read-back shows with its own text is not counted | `codex_install.go`: the count's `!shown.claim(…)` test dropped | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL — `notReadBackVerbatim = 2, want 1`; `TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce` FAIL in both arms |
| B3 one read-back message shows one written message | `codex_install.go`: `claim` answers membership without consuming the claim | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL — `notReadBackVerbatim = 0, want 1: the read-back shows the text once for two written messages` |
| B6 the pairing finds the written message the read-back accounts for | `codex_install.go`: `pairReadBackMessage` answers -1 for every message | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL — `differences = ["dialogue 1 is in the file but was not written: assistant_message (18 bytes, sha256 e46d83c1…)"], want none`; `TestStructureVerificationReportsDialogueRecordsOutOfOrder` and `TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce` FAIL too |
| B7 a reading pairs only where the file still holds the message's record | `codex_install.go`: the `records.holds` guard dropped | `TestCodexRoundTripCountsWhatTheReaderProjectsAway` FAIL — `differences = ["dialogue 1 is in the target document but not in the file: user_message (60 bytes, sha256 3cf14f33…)"], want the written message a…` |

Tests, gate: top-level `--- PASS` cases **153 → 156** across `./cmd/ccc/... ./internal/ccc/...` (before:
`internal/ccc` 126, `./internal/ccc/...` 129, `cmd/ccc` 24 — measured on a detached `git archive` checkout
of `de82906` in `/tmp/head-r11`; after: `internal/ccc` **129**, `./internal/ccc/...` **132**, `cmd/ccc`
**24**, the CLI's own cases untouched, `--- FAIL` 0; `go test -tags fts5 -count=1 -v`, `--- PASS` lines at
top level). The three new top-level cases are the container member's continuation, the overlapping envelope
and the line that stays a stopped run; the count-read-from-claims rule is a case this round added to
`TestCodexRoundTripCountsWhatTheReaderProjectsAway`. The Python suite's own `selftest` step is **153**,
unchanged (this round touched no `scripts/` file). `python3 scripts/gate.py` on this tree — **exit 0,
`GATE: PASS`** (not partial; the block is the one in §6, recorded by the run on the tree as delivered —
only documentation changed after the code, tests and scripts).

The native harness was not re-run, and the fix's own reach is the reason: this round changes neither the
converted rollout, nor the installation, nor the target-side checks — it changes which state the source-file
comparison answers for one source family, and how the structural count is read from a read-back. The two
runs §3 records are file-backed Claude transcripts whose source check is `unchanged` and whose structural
count is 0, and both are pinned offline (`TestContinueConvertsInstallsAndReadsBackTheFixture` asserts the
count is 0 and the fixture's own source hash), so no number those runs printed moves; §14's and §15's quoted
counts for their own shapes (1, 2 and 0) are unchanged as well — the tests that assert them pass untouched,
which is the evidence rather than an argument. Q-0002-20 records why a member read earns the run's third
state rather than a digest of the container.

Docs changed with the code: this §16, §6 (the `go test` and gate blocks re-recorded, and two rule rows) and
the plan's §3 source-check bullet (the member identity in the `not-verified` list, and the line the third
state is not for). `docs/decisions/OPEN-QUESTIONS.md` gains **Q-0002-20** (how the source check answers a
read whose identity is a container member, and why the third state is the answer rather than a digest of the
container). `AGENTS.md` is unchanged: no repository layout, gate step, command, read order or agent rule
moved this round — the changes live in existing package files and test files — which the stage report
states. Goal bookkeeping: 0002 is still `Implemented on feat/0002-claude-to-codex-continuation; awaiting
operator acceptance`, so nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s row stays as it
is.

## 17. Repair round 12 — the check reads the file it was handed; the throw-away home is seeded, not cloned

Round 12's review left two findings, both P2, both fixed here and both reproduced by execution before the
fix (Table A). Its carried-tree parity audit found no increment-attributable defect again — the supplied
patch changes no carried `file`/`skip` record and no upstream-owned source path, which the gate's `parity`
step proves (5 553 carried files still byte-identical to the pin, and the one prefix this increment added to
the manifest still names `scripts/codex-native-check.py`) — and its own verification limit was that the
audit was static: no rehash, build, test or reproduction ran on its side. Two of its notes are carried
rather than fixed, because neither is this increment's: `check_files` establishes agreement with the
manifest rather than an independent authentication of the manifest against upstream, and `check_deviations`
still has its pre-existing symlink-shape gap for the normalized `skip` record. The round also asked for a
systematic close-out instead of one finding at a time, so Table A is per *rule* of the surface this round
touched, with one executed mutation each.

**1 — the throw-away Codex home was seeded with the host's whole `config.toml`**
(`scripts/codex-native-check.py`). The script read the host's configuration and wrote it into the
throw-away home, changing only the two top-level keys `--model-provider`/`--model` name, while the line it
printed said "no credentials … were copied". A real `config.toml` can hold credentials: provider
authorization headers written out under `http_headers`, MCP servers' `env` tables, a token in a provider's
`query_params`, a credential in a URL. The seeding now copies **settings, never the document**:
`seed_configuration` parses the host configuration as TOML and carries only the settings this check has a
job for — `model`, `model_provider`, and the *named* provider's definition through an allowlist of
`name`/`base_url`/`wire_api`/`env_key`/`env_http_headers` — where `env_key` and `env_http_headers` hold the
*names* of environment variables, which is the environment-backed authentication the boundary asks for.
Everything else is left behind and named in the report line: the host's other top-level settings and tables,
every other provider definition, and the named provider's own credential-bearing settings. A provider whose
`base_url` carries a username or a password is left behind *whole* (half a definition would send the harness
to the provider's default endpoint instead of the one the host named), and a configuration that cannot be
read as TOML contributes nothing and says so. The run's line is now
`seeded from: … — copied …; not copied (…): …`, which is what §3's two runs print, and `docs/harness.md`
and the plan's §8 state the same rule.

**2 — the mandatory structural check resolved the session's id through the target's sessions tree**
(`internal/ccc/codex_install.go`). `VerifyCodexRolloutStructure` builds `verification.TargetPath` from the
path the conversion published, but read the file back with `Ref{ID: rollout.SessionID}`: that lookup goes
through the inherited `codexSourceSet.findSourceFile`, which enumerates every dated directory under the
sessions root, matches by the UUID in the file name, and prefers the lexically first copy — and
`canonicalSource` repeats that lookup. A target home that already held another copy of the id (or merely a
long history of unrelated sessions) therefore decided what the check compared and how much of the home it
walked, at every `--verify` level, although TargetPath already identified the installed file. The check now
asks the inherited reader for `verification.TargetPath`: a path reference resolves to that exact source
(`FindSource(StoredFilePath: …)` with `RequireFreshSource` unset), so neither `findSourceFile` nor
`canonicalSource` runs.
`TestStructureVerificationReadsTheInstalledPathItWasGiven` plants a decoy where that lookup prefers it — an
earlier dated directory under the same root, holding the same identity and one different model-visible
message — asserts the premise that the id lookup answers with the decoy, and requires the check to verify
the installed file anyway.

### Table A — every rule of the touched surface, one executed mutation each

Each mutation was applied to this working tree by a throw-away harness outside the repository (it lives in
`/tmp`, never in the tree), the tests that claim the rule were run, and every file was restored from a
byte-identical copy with the restore checked: `path.read_text()` equality for the script, and for the Go
file the reviewed head's own blob against the restored file (`git show HEAD:internal/ccc/codex_install.go`,
whose diff against the restored file holds only this round's lines) plus the green tests and the gate run
below.

| Rule | Mutation (the fix's line reverted) | Observed |
| --- | --- | --- |
| 1 the structural check reads the file it was handed | `codex_install.go`: the reader is asked for `rollout.SessionID` again (the reviewed head's own line) | `go test -tags fts5 -count=1 -run TestStructureVerificationReadsTheInstalledPathItWasGiven ./internal/ccc` FAIL — `the check read something other than the installed file: {State:not-verified … Detail:the installed session does not hold the evidence in the target document: dialogue 2 is in the file but was not written: assistant_message (35 bytes, sha256 ced7502d…)}` |
| 2 the host's configuration document is never copied (finding 1's own defect) | `codex-native-check.py`: the seeded text answers the host's own bytes (`text = host_text`) | `python3 -m unittest scripts.tests.test_codex_native_check` rc=1 — `FAIL: test_copies_only_the_settings_that_cannot_hold_a_credential`, `AssertionError: {'mod… 'model_reasoning_effort': 'high', 'model_providers': {'a-proxy': {… 'http_headers': {'Authorization': 'Bearer a-fixture-header-token'}, …}}}` (the copied bytes hold the credential); `ERROR: test_a_configuration_it_cannot_read_is_not_copied`; `FAIL: test_the_command_line_still_decides_what_the_home_answers_with`; `FAIL: test_does_not_copy_a_provider_whose_url_holds_a_credential` |
| 2 a credential written into a provider URL is not copied | `codex-native-check.py`: `credential_free_url` answers `True` | rc=1 — `FAIL: test_does_not_copy_a_provider_whose_url_holds_a_credential`, `AssertionError: 'model_providers' unexpectedly found in {… 'base_url': 'https://a-fixture-url-user:a-fixture-url-password@proxy.example/v1' …}` |
| 2 the command line still decides what the home answers with | `codex-native-check.py`: `overrides.get(key, document.get(key))` → `document.get(key)` | rc=1 — `FAIL: test_the_command_line_still_decides_what_the_home_answers_with`, `AssertionError: 'a-host-model' != 'a-named-model'`; `FAIL: test_a_configuration_it_cannot_read_is_not_copied` |
| 2 the report names what was left behind | `codex-native-check.py`: the report's `not_copied` clause dropped (`if self.not_copied:` → `if False:`) | rc=1 — `FAIL: test_copies_only_the_settings_that_cannot_hold_a_credential` (`[model_providers.a-proxy].http_headers` not in the line); `FAIL: test_a_configuration_with_nothing_to_seed_says_so` |
| 2 a configuration that cannot be read as TOML is not copied, and is reported | `codex-native-check.py`: the decode failure reports no problem (`problem = ""`) | rc=1 — `FAIL: test_a_configuration_it_cannot_read_is_not_copied`, `AssertionError: 'not readable as TOML' not found in 'nothing was copied: it holds none of the settings this check seeds'` |
| 2 the copied bytes are a TOML document holding exactly the copied settings | `codex-native-check.py`: `toml_string` answers the raw value (`" ".join(escaped)`) | rc=1 — `ERROR: test_copies_only_the_settings_that_cannot_hold_a_credential`, `ERROR: test_does_not_copy_a_provider_whose_url_holds_a_credential`, `ERROR: test_a_configuration_it_cannot_read_is_not_copied` (the seeded document does not parse) |

Tests, gate: top-level `--- PASS` cases **156 → 157** across `./cmd/ccc/... ./internal/ccc/...` (before:
`internal/ccc` 129, `./internal/ccc/...` 132, `cmd/ccc` 24 — the counts §16 recorded for the reviewed head;
after: `internal/ccc` **130**, `./internal/ccc/...` **133**, `cmd/ccc` **24**, `--- FAIL` 0;
`go test -tags fts5 -count=1 -v`, `--- PASS` lines at top level). The one new top-level case is the
installed-path read above. The Python suite's own `selftest` step is **153 → 158**:
`python3 -m unittest -v scripts.tests.test_codex_native_check` runs 13 cases, the five the seeding class
adds, and the gate's step ran `Ran 158 tests in 4.861s (rc=0)`. `python3 scripts/gate.py` on this tree —
**exit 0, `GATE: PASS`** (not partial; the block is the one in §6, re-recorded by the run on the tree as
delivered — only documentation changed after the code, tests and scripts).

The native harness **was** re-run, and it is this round's measured consumer: finding 1 changes what its
throw-away home is built from, so both §3 runs were re-executed against the fixed script and their blocks
replaced (`codex-cli 0.169.0`, `--model-provider tencent-litellm --model deepseek-v4.1-flash-fast`, a real
turn each; exit 0, `13 check(s) passed, 0 failed, 0 skipped` in both). The numbers those runs moved are the
run's own minted identity and byte counts, which §3 states are per-run; the counts §5's table reads (carried
messages, tool calls, boundaries) and the tooling fixture's digest `415bbc31…` are unchanged, and the
`seeded from:` line the fixed script prints is kept in both blocks.

Docs changed with the code: this §17, §3 (both run blocks and the two view/answer blocks re-recorded, and
their interpretations re-stated in the answers' own new words), §5's target-view id (it names the same run
as §3's second half), §6 (the gate block re-recorded and two rule rows), `docs/harness.md` (the seeding
rule) and the plan (§4's structure-level paragraph and §8's "copying auth/login state" bullet).
`docs/decisions/OPEN-QUESTIONS.md` gains **Q-0002-21** (which settings the native check seeds, and why an
allowlist rather than a copy with redaction). `AGENTS.md` is unchanged: no repository layout, gate step,
command, read order or agent rule moved this round — the changes live in existing package, script and test
files — which the stage report states. Goal bookkeeping: 0002 is still `Implemented on
feat/0002-claude-to-codex-continuation; awaiting operator acceptance`, so nothing moves to
`docs/goals/archive/` and `docs/goals/README.md`'s row stays as it is.
