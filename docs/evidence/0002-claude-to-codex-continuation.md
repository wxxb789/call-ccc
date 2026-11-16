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
omitted or narrowed category with its count and reason (eighteen categories, always present, including the
zero ones — round 3's §8 added `malformed_source_lines`, and the §1 and §2 blocks here were recorded
before that and print what the writer reported then; §3's runs are this tree's, and print eighteen).

The source check is three states of its own: `installation.source_check_state` is `unchanged`, `changed`
or `not-verified` — the comparison did not run, which is what an archive read and a stopped run answer —
with `detail` carrying that answer's own words. Round 5 replaced the `installation.source_unchanged`
boolean the §1/§2 blocks above were recorded with, because a boolean answered "did the file move?" for a
comparison that never ran (§10). The CLI's own tests pin each state's wording, the `--json` document's
shape and that diagnostics stay on stderr (`cmd/ccc/main_test.go`).

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
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 1 tool call(s), 18 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 3970 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9: {"messages": 7, "user_messages": 2, "assistant_messages": 5, "system_messages": 0, "tool_messages": 0, "system_records": 1, "sidechain_messages": 0, "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1}; conversion carried: 2 user message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies)
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9 [legacy] cwd=/home/user/code/example-api
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9' exec_resume_command="CODEX_HOME=<tmp>/codex-home <codex> exec resume 01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9 '<prompt>'" environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 75593 bytes
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

The target's own view of that installed session (`codex app-server` → `thread/read`, printed by the
harness) and the answer its own resume produced:

```
the target harness's own view of the installed session:
    thread 01a1e29c-5f5e-70bd-ab5e-262b3fbcdbf9 [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: I will read the retry policy first.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
      turn rollout-11
        userMessage: Now check where the backoff cap is configured.
        agentMessage: The cap is configured in internal/retry/config.go as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - File read: none — this conversation contains no file read (no tool call, no file contents); `internal/retry/policy.go` was only named in the prior summary text, and `internal/retry/config.go` only in the follow-up claim.
    - Attempt limit: four, per that summary text — unverified here, since no file was actually read.

    Want me to open `internal/retry/policy.go` and `internal/retry/config.go` now and confirm both against the real code?
```

Why the first fixture's answer has this shape (its own wording varies run to run): the fixture compacts
after its tool call, the conversion carries that boundary as the target's `compacted` record, and Codex
applies that record to its own model-visible history — so the resumed turn continues from the source's
own active context (the summary and the turns after the boundary), which is why this run could quote the
attempt limit and the summarized file from the summary's own words and the cap's file from the
post-boundary turn, while answering that the earlier claims came from the summary text rather than a
tool call — the file keeps the turns before the boundary with their tool evidence, but the target does not
continue from them. An earlier run of
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
PASS: the conversion reports carried evidence and a non-zero loss category — 5 message(s), 3 tool call(s), 18 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/05/rollout-2026-05-05T10-00-00-01a1e29c-82bd-7c27-9ba6-f456957c33dd.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1e29c-82bd-7c27-9ba6-f456957c33dd, not one of the 1 id(s) already in the home
PASS: the installed file is one JSON object per line — 4611 bytes (the target's own read of it is the load check)
PASS: the inherited reader reads the installed file back as what was written — ccc inspect codex:01a1e29c-82bd-7c27-9ba6-f456957c33dd: {"messages": 8, "user_messages": 2, "assistant_messages": 6, "system_messages": 0, "tool_messages": 0, "system_records": 0, "sidechain_messages": 0, "compact_boundaries": 0, "tool_calls": 3, "tool_results": 3}; conversion carried: 2 user message(s), 3 tool call(s), 3 tool result(s), 0 boundary(ies)
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e29c-82bd-7c27-9ba6-f456957c33dd [legacy] cwd=/home/user/code/tooling
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the reported resume command selects the installation home — resume_command='CODEX_HOME=<tmp>/codex-home <codex> resume 01a1e29c-82bd-7c27-9ba6-f456957c33dd' exec_resume_command="CODEX_HOME=<tmp>/codex-home <codex> exec resume 01a1e29c-82bd-7c27-9ba6-f456957c33dd '<prompt>'" environment={'CODEX_HOME': '<tmp>/codex-home'}
PASS: a real turn continues from the installed session — exit 0, session 4611 -> 74390 bytes
PASS: the source session is unchanged — sha256 415bbc31a6cd541f… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
13 check(s) passed, 0 failed, 0 skipped
```

```
the target harness's own view of the installed session:
    thread 01a1e29c-82bd-7c27-9ba6-f456957c33dd [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Check the retry policy and tell me the attempt limit.
        agentMessage: I will read the policy file first.
        agentMessage: The attempt limit is four.
      turn rollout-12
        userMessage: Then name the file that holds the cap.
        agentMessage: internal/retry/config.go, as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - Tools: `Read` and `Bash`.
    - First (`Read` of `/home/user/code/tooling/internal/retry/policy.go`) returned the file: `package retry` / `const MaxAttempts = 4`.
    - Last (`Bash`: `python3 -c 'import json; print(json.dumps("maxBackoffSeconds"))'`) returned `"maxBackoffSeconds"`.

    Note: that last call only echoed the string I passed it — nothing in this conversation shows the cap lives in `internal/retry/config.go`; the one search of that directory errored out.
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
    thread 01a1e272-66ba-7f1d-99c8-a81ab277366b [legacy] provider=tencent-litellm
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
| `Read` call + its result ("package retry … MaxAttempts = 4") | reached the resumed turn ("First returned: the body of `policy.go` — `package retry` followed by `const MaxAttempts = 4`.") and the file holds the pair | carried (`tool_calls ×3`, `tool_results ×3`) |
| `Bash` call whose result the source marked `is_error` | the file holds the pair, and this run's answer names `Bash` among the tools that appear (its wording does not quote the failure) | carried; the recorded outcome has no target field (`tool_result_outcomes ×1`) |
| `Bash` call whose result text is a JSON string (`"maxBackoffSeconds"`), recorded on the same record as a system reminder | reached the resumed turn as that text, quotation marks included ("Last returned: the exact output `"maxBackoffSeconds"` …") | carried, decoded exactly once; the record's reminder text is not written (`system_records ×1`, round 4 below) |
| assistant "The attempt limit is four." | `agentMessage: The attempt limit is four.` | carried |
| a subagent turn marked `isSidechain` | absent | omitted (`sidechain_messages ×1`) |
| the provider's `system` record | absent (the inherited reader drops it before the document, too) | not a message the source read produces, so no writer category counts it |
| tool names `Read`/`Bash`, arguments verbatim | the calls are in the file with those names and inputs; the target's own tool set is Codex's | carried with caveat (`tool_name_vocabulary ×2`) |
| the session's provider | recorded as the target's own effective provider | carried with caveat (`provider_binding ×1`) |

Everything a conversion cannot carry is named by category with a count and a reason on *every* run
(`internal/ccc/codex_target_test.go`'s `TestCodexRolloutReportsEveryCategoryItKnows` requires all
eighteen categories to be present and to be truthful — the omitted text is not in the file — and
`TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` pins the read-completeness one, whose count
is the read's own; `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` covers the archive-read path,
where the report
also says the evidence came from a projection; `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce`
covers the archive's other half, where one recorded outcome is exposed as both an event and a restored
summary and must be written once).

## 6. Tests, gate, and what each pins

```
$ CGO_ENABLED=1 go test -tags fts5 -count=1 ./cmd/ccc/... ./internal/ccc/...     # rc=0
ok  	go.kenn.io/agentsview/cmd/ccc	1.772s
ok  	go.kenn.io/agentsview/internal/ccc	9.696s
?   	go.kenn.io/agentsview/internal/ccc/fixturegen	[no test files]
ok  	go.kenn.io/agentsview/internal/ccc/parity	1.236s
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
| a nested run that breaks inside a subtest is named by the case that read the child, not raised as an `IndexError` in the reader itself | `test_a_child_broken_inside_a_subtest_is_reported_by_name` |
| the printed resume invocation names the data directory the session was installed into, quoted where a shell would act on a word, and a resume verification reports the command it ran the same way | `TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome`, `TestResumeVerificationCommandNamesTheHomeItRanAgainst`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueJSONDistinguishesConversionInstallationAndVerification` |
| exclusive install (a taken name refused, a filesystem without hard links refused rather than renamed over) and dry run writes nothing (no directory for a home the caller named that is not there yet, and the target harness is never started), dry run reports its own states, prompt required, target/level validation, home resolution | `TestInstallCodexRolloutNeverReplacesASession`, `TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks`, `TestContinueDryRunWritesNothing`, `TestContinueDryRunReportsWhatItVerified`, `TestContinueDryRunInstallsNothingThroughTheCLI`, `TestContinueRequiresAPromptBeforeItRunsATurn`, `TestContinueRefusesAnUnsupportedTargetOrLevel`, `TestContinueTargetHomeResolution` |
| the round-trip check compares rather than agrees | `TestStructureVerificationNamesWhatTheFileDoesNotHold` |
| the source's own digest is taken before the conversion and compared after it; a moved source is warned about; a projection and a stopped run answer `not-verified` rather than claiming a comparison | `TestSourceCheckComparesTheFileBeforeAndAfter`, `TestSourceChangedWarningsNameTheMove`, `TestSourceCheckSaysWhenThereWasNoFileToCompare` |
| a result the conversion does not write is never reported as carried with a caveat, while the same two flags on a result that *is* written are counted | `TestCodexRolloutDoesNotReportOmittedResultsAsCarried` |
| a failure after the conversion keeps the report it reached — the published installation and identity, the conversion's counts — with every check that did not run `not-verified` and naming the reason: a source that vanished after publication, a temporary file the installer could not remove, a home that could not be created | `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall`, `TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved`, `TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated` |
| the CLI's three states, human shape, source check, partial-read fidelity line, exit codes, help, that a failure carrying a report still writes it (and a failure with none writes nothing), that a run which stopped before publishing renders the report's own `not-installed` state with its identity marked proposed, that a dry run's block says it wrote nothing and marks its identity proposed, and that the installed line names the session's own path and its mechanism | `TestContinueJSONDistinguishesConversionInstallationAndVerification`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueHumanOutputReportsTheSourceCheck`, `TestContinueHumanOutputNamesAPartialSourceRead`, `TestContinueUsageErrors`, `TestContinueReportsAFailedVerificationAsAFailure`, `TestContinueWritesTheReportOfAFailureAfterPublication`, `TestContinueHumanOutputReportsAPrePublicationFailure`, `TestContinueDryRunInstallsNothingThroughTheCLI`, `TestContinueRefusesAMissingDataDirectoryNobodyNamed`, `TestContinueHelpIsARequestNotAFailure` |

Gate, run on this tree (`python3 scripts/gate.py`, exit 0):

```
gate: <repo>
gate: go=<home>/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3581 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 153 tests in 5.597s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 26 project-owned prefix(es); files seen by git: 5625 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 73 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
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
— and a still later one moved that result onto a record the provider wrote as a system reminder (§9);
it is still not an oracle case: its digest (`415bbc31…`, above) is the one §3's tooling run reports as
`the source session is unchanged`, and `TestDumpMatchesTheRecordedUpstreamAgreement` (in the
gate's `test` step) still holds the recorded dumps against the unchanged first fixtures.
`scripts/upstream-compare.py` itself was **not** re-run here (it needs network and a pristine
checkout of the pin); its next run is the operator's, and the offline half of the same comparison passes.

The last repair to this increment came from CI rather than from a clause: `gate (windows-latest)` failed
in the `test` step on the two CLI cases that read the resume line back. The line is rendered the way a
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

The `selftest` step is 153 cases — the 151 the round-5 block printed, plus the two
`ProtectedArtifactsAfterNativeVerification` cases round 6 adds (§11); the block above is refreshed from
round 6's own run. `docs/harness.md`
records both halves: the path-spelling constraint as a fourth Windows answer its Platform boundary
section pins, and the dangling line as what the self-check cases read a child's per-case lines for.

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
record states changed at that round; §3's per-run ids and byte counts were re-recorded in round 6 (§11),
whose runs are the ones the blocks above now hold.

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
ordering this round is about, observed in a real run and not only in the offline pin. §3's two blocks are
those runs.

Docs changed with the code: this record's §3 (both runs re-recorded in the new order), §4 and §6 (the
ordering rule and its offline pin; the refreshed `go test` and gate blocks; the CLI row's new test), and
`docs/harness.md`'s native-harness entry (the "before and after" guarantee now says after the loader and
the turn). `docs/decisions/OPEN-QUESTIONS.md` gains `Q-0002-15`, the human block's spelling of a
not-installed report and of a proposed identity. `AGENTS.md` is unchanged: no repository layout, gate
step, command, read order or agent rule moved this round. Goal bookkeeping: 0002 is still
`Implemented …; awaiting operator acceptance`, so nothing moves to `docs/goals/archive/` and
`docs/goals/README.md`'s row is already accurate.
