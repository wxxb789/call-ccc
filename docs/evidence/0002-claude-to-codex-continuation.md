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
39e11668cc1cf73a6fb38cd544fe9db8a4b5ae17f4e8270eb0a3e9e3800c1046  internal/ccc/testdata/fixtures/claude/-home-user-code-tooling/b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef.jsonl
d9957a8d6f5731a3df9ce1f175ffe9e228c71f041cd765b03f743852aad1d68b  internal/ccc/testdata/fixtures/codex/2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
```

The first fixture is the one goal 0001 recorded (dialogue, a tool call with its result, a compaction
boundary); the second is new here (`internal/ccc/fixturegen`) and carries the shapes a conversion has to
report: a persisted reasoning trace, two tool calls whose results arrive as their own user lines — one of
them marked `is_error` — a subagent turn marked `isSidechain`, and a provider system record.
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
  installed:    <tmp>/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1dfbf-9b69-7063-ae8e-9142650f1090.jsonl [hard-link]
  target id:    01a1dfbf-9b69-7063-ae8e-9142650f1090
  working dir:  /home/user/code/example-api
  resume:       codex resume 01a1dfbf-9b69-7063-ae8e-9142650f1090
  non-interactive resume: codex exec resume 01a1dfbf-9b69-7063-ae8e-9142650f1090 "<prompt>"
  cli_version:  0.169.0 (probe)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  verified (the target harness's own loader (codex app-server, thread/resume))
                the harness's loader answered with this session's own id and path; opening a session makes the harness record its own settings in it (measured: one thread_settings_applied record), and no session other than the one it opened is touched
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                not requested (--verify load)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

`ccc help` explains that invocation without any other documentation (`ccc continue` is its first usage
line and the example above is spelled out in it), and `ccc continue -h` prints the one-line usage
(`exit 0`, stdout). The written file itself starts with the record the target's reader requires:

```
{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta","payload":{"id":"01a1dfbf-9b69-7063-ae8e-9142650f1090","timestamp":"2026-05-04T09:15:00Z","cwd":"/home/user/code/example-api","originator":"ccc","cli_version":"0.169.0","source":"cli","thread_source":"user","model_provider":"openai"}}
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
    --target-home <tmp>/home6 --verify resume \
    --resume-prompt "Answer only from this conversation: which file did you read, and what was the attempt limit?" --json
exit 0
conversion.carried:     {"messages": 5, "user_messages": 2, "assistant_messages": 3, "tool_calls": 1,
                         "tool_results": 1, "compaction_boundaries": 1, "turns": 2, "bytes": 3136}
installation:           {"state": "installed", "path": "<tmp>/home6/sessions/2026/05/04/rollout-2026-05-04T09-15-00-<id>.jsonl",
                         "mechanism": "hard-link", "created": true, "source_unchanged": true,
                         "detail": "sha256 2bd510821d4abf83…, taken before and after the conversion"}
verification.load:      "verified" (the target harness's own loader (codex app-server, thread/resume))
verification.resume:    "verified" (exit_code 0, session_bytes_before 3970, session_bytes_after 68799)
target_session:         model_provider "tencent-litellm" (source "target-thread-defaults"), cli_version "0.169.0"
answer:                 - File: `internal/retry/policy.go`
                        - Attempt limit: four
```

The three blocks a consumer reads apart: `conversion.state` (`converted`), `installation.state`
(`installed`), `verification.load`/`verification.resume` (`verified` / `not-verified` /
`not-requested`), each with its own `method` and `detail`, and `conversion.losses[]` naming every
omitted or narrowed category with its count and reason (twelve categories, always present, including the
zero ones). The CLI's own tests pin each state's wording, the `--json` document's shape and that
diagnostics stay on stderr (`cmd/ccc/main_test.go`).

## 3. Codex loads the installed session and continues from it

`scripts/codex-native-check.py` re-derives all of it; the blocks below are two real runs of this tree
(target-harness diagnostics that are not verdicts — its sandbox probe and PATH-alias warnings — are
dropped here, and the script's own `NOISE` filter keeps them out of its summaries). Every run mints its
own session identity, and the resumed turn's own words and byte counts differ per run, so the ids and
numbers below are that one run's. Both fixtures, with a real turn each:

```
$ python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which file did you read, and what was the attempt limit?"
exit 0
PASS: ccc builds with the flags upstream ships
PASS: the conversion reports carried evidence and its limits — 5 message(s), 1 tool call(s), 12 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1dfc1-6f68-710e-94fd-4ae29746c873.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1dfc1-6f68-710e-94fd-4ae29746c873
PASS: the installed file is JSONL the target reads — 3970 bytes
PASS: the inherited reader reads the installed file back as what was written — {"session_id": "codex:01a1dfc1-6f68-…", "messages": 7,
      "user_messages": 2, "assistant_messages": 5, "tool_calls": 1, "tool_results": 1, "compaction_boundaries": 1}
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1dfc1-6f68-… [legacy] cwd=/home/user/code/example-api
PASS: the target's own view lists the converted turns — 8 line(s)
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 69930 bytes
11 check(s) passed, 0 failed, 0 skipped
```

The target's own view of that installed session (`codex app-server` → `thread/read`, printed by the
harness) and the answer its own resume produced:

```
the target harness's own view of the installed session:
    thread 01a1dfc1-6f68-710e-94fd-4ae29746c873 [legacy] provider=tencent-litellm
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: I will read the retry policy first.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
      turn rollout-11
        userMessage: Now check where the backoff cap is configured.
        agentMessage: The cap is configured in internal/retry/config.go as maxBackoffSeconds.

the turn's own output (the target's answer, as the harness printed it):
    - File read: none — I made no tool calls in this conversation, so no file was actually read.
    - Attempt limit: four was stated, but that came from an unverified claim, not from any file I read.
```

Why "no file was actually read" is the right answer *for this session*: the fixture compacts after its
tool call, the conversion carries that boundary as the target's `compacted` record, and Codex applies
that record to its own model-visible history — so the resumed turn continues from the source's own active
context (the summary plus the turns after the boundary), exactly as the source's own session would, while
the file keeps the turns before it. An earlier run of this same check, over a rollout written without the
target's `event_msg` message records, could not see even the post-boundary turns and answered "no tool
call exists in this conversation"; that is what writing both record kinds buys. The second fixture has no
boundary, and there the tool evidence reaches the resumed turn:

```
$ python3 scripts/codex-native-check.py --session b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef \
    --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
    --resume-prompt "Answer only from this conversation, one line each: which two tools appear, what did the first return, and what did the second result say?"
exit 0
PASS: the conversion reports carried evidence and its limits — 5 message(s), 2 tool call(s), 12 loss categories
PASS: the session is installed under the target's own sessions tree — sessions/2026/05/05/rollout-2026-05-05T10-00-00-01a1dfc1-9291-7e59-a462-9da4523dead7.jsonl [hard-link]
PASS: the target identity is new and is not the source's — 01a1dfc1-9291-7e59-a462-9da4523dead7
PASS: the installed file is JSONL the target reads — 4113 bytes
PASS: the inherited reader reads the installed file back as what was written — {"session_id": "codex:01a1dfc1-9291-…", "messages": 7,
      "user_messages": 2, "assistant_messages": 5, "tool_calls": 2, "tool_results": 2, "compaction_boundaries": 0}
PASS: the source session is unchanged — sha256 39e11668cc1cf73a… (fixture tree identical)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1dfc1-9291-… [legacy] cwd=/home/user/code/tooling
PASS: the target's own view lists the converted turns — 8 line(s)
PASS: a real turn continues from the installed session — exit 0, session 4113 -> 69597 bytes
11 check(s) passed, 0 failed, 0 skipped
```

```
the turn's own output (the target's answer, as the harness printed it):
    - Tools: `Read` and `Bash`.
    - First returned: `package retry` / `const MaxAttempts = 4` from `/home/user/code/tooling/internal/retry/policy.go`.
    - Second said: `Error: no such file or directory` (the `grep -rn maxBackoffSeconds internal/retry` command failed).
```

Both tool calls, the first result's content and the second result's recorded *failure* reached the
resumed turn through the conversion — written as `function_call`/`function_call_output` records with the
source's own tool names, its recorded arguments verbatim and the result text decoded from the source's
own content field.

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
  `the unrelated session in the target home is unchanged`, plus `source_unchanged: true` in the JSON.
  The harness digests every file below the fixture root and the unrelated session it plants in the
  target home before the conversion and compares them after it; `TestContinueKeepsTheSourceAndItsDirectoryIntactForBothFixtures`
  does the same in-process over the whole fixture tree.
- New identity: the target id is a UUID v7 that is neither the source's session id nor any id already in
  the home (`TestCodexRolloutGeneratesAFreshIdentity`,
  `TestContinueConvertsInstallsAndReadsBackTheFixture`), and the install itself is exclusive:
  `TestInstallCodexRolloutNeverReplacesASession` writes a session and then requires the second install at
  the same path to fail with `ErrTargetExists` while the file keeps the first write's digest. The
  mechanism the run reports (`hard-link`) is an exclusive link; the fallback for filesystems without
  hard links checks that the name is free and renames, and never replaces anything either.
- The source session is never opened for writing: the reader is the inherited one (read-only), the
  archive path is opened read-only when consulted, and the pipeline's only write is the target file.

## 5. A representative transcript keeps its supported semantics; the rest is reported

Source facts for the tool-heavy fixture (`ccc inspect … --json`, unchanged from goal 0001's command):

```
counts:  {"messages": 10, "user_messages": 4, "assistant_messages": 6, "system_messages": 0, "tool_messages": 0,
          "system_records": 0, "sidechain_messages": 1, "compact_boundaries": 0, "tool_calls": 2, "tool_results": 2}
tools:   ["Bash", "Read"]
```

The target view the table reads is the tooling fixture's, printed by the same harness run as §3's second
half:

```
the target harness's own view of the installed session:
    thread 01a1dfc1-9291-7e59-a462-9da4523dead7 [legacy] provider=tencent-litellm
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
| assistant "I will read the policy file first." (with a persisted reasoning trace) | `agentMessage: I will read the policy file first.` | carried; the reasoning trace omitted (`reasoning_text ×1`) |
| `Read` call + its result ("package retry … MaxAttempts = 4") | reached the resumed turn ("First returned: `package retry` / `const MaxAttempts = 4` from `/home/user/code/tooling/internal/retry/policy.go`") and the file holds the pair | carried (`tool_calls ×2`, `tool_results ×2`) |
| `Bash` call whose result the source marked `is_error` | reached the resumed turn ("Second said: `Error: no such file or directory` (the `grep -rn maxBackoffSeconds internal/retry` command failed)") | carried; the recorded outcome has no target field (`tool_result_outcomes ×1`) |
| assistant "The attempt limit is four." | `agentMessage: The attempt limit is four.` | carried |
| a subagent turn marked `isSidechain` | absent | omitted (`sidechain_messages ×1`) |
| the provider's `system` record | absent (the inherited reader drops it before the document, too) | `system_records ×0` |
| tool names `Read`/`Bash`, arguments verbatim | the calls are in the file with those names and inputs; the target's own tool set is Codex's | carried with caveat (`tool_name_vocabulary ×2`) |
| the session's provider | recorded as the target's own effective provider | carried with caveat (`provider_binding ×1`) |

Everything a conversion cannot carry is named by category with a count and a reason on *every* run
(`internal/ccc/codex_target_test.go`'s `TestCodexRolloutReportsEveryCategoryItKnows` requires all twelve
categories to be present and to be truthful — the omitted text is not in the file — and
`TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` covers the archive-read path, where the report
also says the evidence came from a projection).

## 6. Tests, gate, and what each pins

```
$ CGO_ENABLED=1 go test -tags fts5 -count=1 ./cmd/ccc/... ./internal/ccc/...     # rc=0
ok      go.kenn.io/agentsview/cmd/ccc              2.656s
ok      go.kenn.io/agentsview/internal/ccc         10.364s
ok      go.kenn.io/agentsview/internal/ccc/parity  2.222s
```

| Rule | Test |
| --- | --- |
| the written record shape, order and fields (session_meta, turn_context, response_item, event_msg, function_call/output, compacted) | `TestCodexRolloutRecordShape` |
| dialogue only (no rendering of calls), calls/results paired by id, boundaries kept with the turns before them | `TestCodexRolloutKeepsTheDialogueProjection`, `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt` |
| every loss category reported, and truly absent from the file | `TestCodexRolloutReportsEveryCategoryItKnows` |
| id-less calls and results get paired deterministically, or are reported unpaired | `TestCodexRolloutPairsAnIdLessResultWithItsCall` |
| the provider binding comes from the caller and is reported | `TestCodexRolloutRecordsTheProviderBindingItIsGiven` |
| fresh UUID v7 identity, one ordered timestamp run | `TestCodexRolloutGeneratesAFreshIdentity`, `TestCodexRolloutStampsRecordsInOneOrderedRun` |
| end-to-end: convert, install, read back through the inherited reader, source and neighbours unchanged | `TestContinueConvertsInstallsAndReadsBackTheFixture` |
| the archive fallback is used and disclosed | `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` |
| exclusive install, dry run writes nothing, prompt required, target/level validation, home resolution | `TestInstallCodexRolloutNeverReplacesASession`, `TestContinueDryRunWritesNothing`, `TestContinueRequiresAPromptBeforeItRunsATurn`, `TestContinueRefusesAnUnsupportedTargetOrLevel`, `TestContinueTargetHomeResolution` |
| the round-trip check compares rather than agrees | `TestStructureVerificationNamesWhatTheFileDoesNotHold` |
| the source's own digest is taken before the conversion and compared after it; a projection says no file was compared | `TestSourceUnchangedComparesTheFileBeforeAndAfter`, `TestSourceUnchangedSaysWhenThereWasNoFileToCompare` |
| the CLI's three states, human shape, exit codes, help | `TestContinueJSONDistinguishesConversionInstallationAndVerification`, `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueUsageErrors`, `TestContinueRefusesAMissingDataDirectoryNobodyNamed`, `TestContinueHelpIsARequestNotAFailure` |

Gate, run on this tree after the last change (`python3 scripts/gate.py`, exit 0):

```
gate: <repo>
gate: go=<home>/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3574 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 135 tests in 5.486s (rc=0)
  PASS     parity   AgentsView 83195c8a… | carried: 5553 file(s) identical to the pinned revision (compared through git's own
                    object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree
                    stores) + 1 generated input(s) | owned: 26 project-owned prefix(es); files seen by git: 5623
  PASS     secrets  no leaks found in 70 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The file this increment adds outside the Go packages, `scripts/codex-native-check.py`, is inside a
prefix the carry manifest calls project-owned (added to `scripts/upstream-import.py`'s `OWNED` and to the
generated manifest, which `scripts/tests/test_upstream_parity.py` requires to agree; the Go files are
under the already-owned `internal/ccc/`). The oracle comparison's cases now name the
fixture each one reads (`internal/ccc/parity`'s `Cases`, mirrored by `scripts/upstream-compare.py`), so
the second Claude fixture is a *new case file* rather than a change to an existing record: the
committed dumps were not re-recorded and `internal/ccc/parity`'s offline test still compares them byte
for byte. `scripts/upstream-compare.py` itself was **not** re-run here (it needs network and a pristine
checkout of the pin); its next run is the operator's, and the offline half of the same comparison passes.

## 7. Limits and residual risks

- **The interactive TUI resume is unverified on this host** (no TTY): the loader behind it
  (`thread/resume`) is verified, the picker is not.
- **The app-server protocol is experimental and version-bound.** The verified version is recorded on
  every run and in `CodexFormatVersion`; a protocol change turns `load` into `not-verified` with the
  harness's own message rather than a pass.
- **The recorded provider binds the session** to the target's configuration at conversion time; a later
  provider change makes the harness refuse to resume it (its own message names both values). Recorded as
  `Q-0002-01`; the alternative — recording none — is refused outright by a home that has migrated to
  paginated threads, which is what a real home looks like.
- **`migrate-rollouts --apply` is deliberately not run**: on the tested version it failed with
  `missing_sqlite_metadata` for a rollout added to a home that already held a migrated thread; the
  target migrates on its own when it needs to, and the report claims no migration.
- **Tool evidence in the target's own item view**: `thread/read` lists a turn's messages; the calls and
  results are in the session file and in the model-visible history (proved by the resumed turn in §3)
  but not in that view. Stated as a format limit of the pair (`target_session.format_limits`).
- **Attachments are not counted**: the source read carries no attachment bytes, so a per-conversion
  count would be a claim the read cannot support; the limit is stated instead.
