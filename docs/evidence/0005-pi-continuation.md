# Evidence — 0005 Continue conversations into and out of Pi

Contract: [`docs/goals/0005-pi-continuation.md`](../goals/0005-pi-continuation.md).
Plan: [`docs/plans/0005-pi-continuation.md`](../plans/0005-pi-continuation.md).
Route evidence this increment stands beside:
[`0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md) and
[`0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md) (the accepted routes whose
pipeline, report shape and conventions the Pi route reuses).

Every command below was run on this host, in this tree, with the exit code and the output it printed.
Nothing here is re-typed from memory; where a number appears it was counted by the run that names it.
Absolute paths are as the runs printed them; the repository root is spelled `<repo>` and scratch
directories are under `/tmp/`. No real session content is quoted: every fixture string is invented, and
the one real model turn ran against the committed fixture's own conversation.

Host facts this increment was measured against:

| Fact | Value |
| --- | --- |
| Pi CLI | `pi 1.5.2`, resolved from `~/.local/share/mise/installs/pi/latest/pi/pi` |
| model access used by the real turn | the host's own Pi provider configuration, reached through the throw-away home's seeded `models.json` (the provider's key left as an `$ENV` reference); no credential was copied or printed |
| data directories | every run used an isolated `--target-home` / scratch agent directory; no developer data directory was read or written |
| build | `go build -tags fts5 -o /tmp/ccc-bin ./cmd/ccc` (the gate's own `build` step builds the same packages) |

## 0. What was measured before any writer code existed

Every row is a run against the installed binary, in throw-away directories, with hand-written session
files (probe scripts under `/tmp/pi-probe/`, not shipped).

| Measurement | Result |
| --- | --- |
| the format's location and spelling | `<agent dir>/sessions/--<cwd spelling>--/<timestamp>_<session id>.jsonl`; the spelling is the cwd with its leading separator removed and every `/`, `\`, `:` replaced by `-`, wrapped in `--` (`getDefaultSessionDirPath` in the installed bundle; confirmed against a real home: `/home/azureuser/.hermes/cache/scratch` → `--home-azureuser-.hermes-cache-scratch--`) |
| a hand-written v3 session | loads: `pi --mode rpc --session <id>` answered `get_state` with `sessionId` and `messageCount: 4` and `get_messages` with the roles `["user","assistant","toolResult","assistant"]` in order |
| the unknown-id control arm | the same command with an id nothing installed prints `No session found matching '<id>'` and answers no `get_state` response |
| a session that records no `cwd` | the lookup cannot be scoped to it: `pi --session <id>` from another directory prints `Session found in different project: <recorded>` and asks `Fork this session into current directory?` instead of loading it |
| a real turn | `pi -p --session <id> … "Reply with exactly: PROBE-TURN-OK"` answered `PROBE-REPLY-OK`-style output with exit 0 and **appended** the harness's own records (its own system message, the user turn, the assistant answer) to the session file; the pre-existing records were unchanged byte for byte |
| `message.usage` on an assistant record | **required**: a written assistant entry with no usage object stops a resumed run with `undefined is not an object (evaluating 'message.usage.input')`; a zeroed usage object is accepted and the turn continues (measured twice: once with a reasoning block beside the dialogue, once with a tool call and its result) |
| a `thinking` block | accepted: the turn ran and answered with the trace present in the history |
| the RPC loader's effect on the session file | an RPC run that loads the session **appends** its own records (a `thinking_level_change` among them) to the file it opened: the same file, hashed before and after a load, differed by those bytes |
| `get_state.messageCount` | not the file's `message` count: for a session with a compaction boundary it counts the loader's model-visible history (the boundary rendered as `compactionSummary`, then every message entry after it) — measured `3` for a file holding `7` message entries |
| `--provider` / `--model` on the printed line | both stop a run before the session is opened when the target home cannot serve them: `--provider claude` → `Unknown provider "claude"`; `--model claude-sonnet-4-5` → `ambiguous across providers: anthropic/…, cloudflare-ai-gateway/…, opencode/…, radius/…; No matching provider is authenticated` |
| the agent directory | `PI_CODING_AGENT_DIR` relocates it; a fresh agent directory is written as `auth.json` `{}` plus an empty `models-store.json` by the harness itself, and discovery/loading work with no credential present |
| a seeded `models.json` | provider definitions the host holds can be copied with `apiKey`/`headers` dropped and an `$ENV` reference written back; the whole document's schema is all-or-nothing, so an unrelated provider left in with its credential fields removed invalidated the file (`Provider "opencode-go": must specify "baseUrl", "headers", "compat", "modelOverrides", or "models"`) |

## 1. `--to pi`: a Claude Code conversation as a new Pi session

`ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to pi --target-home /tmp/ev5/pi-home
--target-binary ccc-no-such-harness --data-dir /tmp/ev5/data`, with
`CLAUDE_PROJECTS_DIR=<repo>/internal/ccc/testdata/fixtures/claude`, exit code 0:

```console
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → pi
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2051 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2051: the file ends at a record boundary
  converted:    pi-session-jsonl, 3051 bytes: 6 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  carried-with-caveat: unrecorded_token_usage ×4
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: message_model_attribution ×2
  carried-with-caveat: unresolved_source_parents ×1
  carried-with-caveat: pre_compaction_history ×4
  omitted:      malformed_source_lines ×1
  installed:    /tmp/ev5/pi-home/sessions/--home-user-code-example-api--/2026-05-04T09-15-00Z_01a1ea8c-1596-747d-99a4-38ab74adaa55.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1ea8c-1596-747d-99a4-38ab74adaa55
  working dir:  /home/user/code/example-api
  resume:       cd /home/user/code/example-api && PI_CODING_AGENT_DIR=/tmp/ev5/pi-home PI_CODING_AGENT_SESSION_DIR=/tmp/ev5/pi-home/sessions/--home-user-code-example-api-- ccc-no-such-harness --session 01a1ea8c-1596-747d-99a4-38ab74adaa55
  non-interactive resume: cd /home/user/code/example-api && PI_CODING_AGENT_DIR=/tmp/ev5/pi-home PI_CODING_AGENT_SESSION_DIR=/tmp/ev5/pi-home/sessions/--home-user-code-example-api-- ccc-no-such-harness -p --session 01a1ea8c-1596-747d-99a4-38ab74adaa55 -- '<prompt>'
  cli_version:  1.5.2 (writer-pin)
  structure:    verified (the inherited AgentsView Pi reader over the installed session)
  native load:  not-verified (the target harness's own loader (pi --mode rpc, get_state/get_messages, with no model variables in its environment, plus an unknown-id control))
                ccc-no-such-harness is not on PATH, so the installed session's native load could not be verified (ccc-no-such-harness is not on PATH)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume (pi -p --session))
                not requested (--verify load)
  warning:      the target harness was not found on PATH: the conversation is converted and installed at /tmp/ev5/pi-home/sessions/--home-user-code-example-api--/2026-05-04T09-15-00Z_01a1ea8c-1596-747d-99a4-38ab74adaa55.jsonl, and its native load is unverified
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
EXIT=0
```

The same run without a target binary present exits 0 (the native half could not run, which is reported,
not failed), and the installed session's bytes are what its record says:

```console
{"type":"session","version":3,"id":"01a1ea8c-1596-747d-99a4-38ab74adaa55","timestamp":"2026-05-04T09-15-00.000Z","cwd":"/home/user/code/example-api"}
{"type":"message","id":"…","parentId":null,"timestamp":"2026-05-04T09-15-00.000Z","message":{"role":"user","content":"Summarize the retry policy in internal/retry/policy.go.","timestamp":1777886100000}}
…
{"type":"message","id":"…","parentId":"…","timestamp":"2026-05-04T09-16:36.000Z","message":{"role":"assistant","content":[{"type":"text","text":"The cap is configured in internal/retry/config.go as maxBackoffSeconds."}],"model":"claude-sonnet-4-5","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0,"totalFromProvider":false}},"stopReason":"stop","timestamp":1777886196000}}
```

(The installed file's own bytes are printed in the native run below's `entries {'compaction': 1, …}`
line; the excerpt above is the same document's head and tail with the entry ids elided.)

## 2. The Pi target verified by Pi's own loader, natively

`CCC_PI_NATIVE_PROVIDER=tencent-litellm python3 scripts/pi-native-check.py --resume-prompt "Reply with
exactly: PROBE-TURN-OK" --expect PROBE-TURN-OK`, exit code 0 — the run prints two verdicts beyond
its own checks: the loader's answer, and the turn's own output:

```console
repo:        <repo>
direction:   claude-to-pi (claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → pi)
target:      /home/azureuser/.local/share/mise/installs/pi/latest/pi/pi (/home/azureuser/.local/share/mise/installs/pi/latest/pi/pi)
pi:          /home/azureuser/.local/share/mise/installs/pi/latest/pi/pi (1.5.2)
source:      internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl (copied to /tmp/ccc-pi-native-check-9_3yb_9m/claude-source/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl)
workspace:   /tmp/ccc-pi-native-check-9_3yb_9m/workspace -> project dir --tmp-ccc-pi-native-check-9_3yb_9m-workspace--
target home: /tmp/ccc-pi-native-check-9_3yb_9m/pi-home
pi models:   seeded provider(s) tencent-litellm from /home/azureuser/.pi/agent; left out (not needed here): ghc-proxy, ghc-proxy-chat, opencode-go; every credential field dropped, keys resolved from the environment
the source fixture's recorded working directory (/home/user/code/example-api) is rewritten to the workspace, so the session belongs to a path this run owns and the target's own lookup can find what it installs

PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 6 message(s), 1 call(s), 1 result(s); loss categories named: 29
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr No session found matching '89772696-7dbe-420b-9052-0cf2f41d9e24'
PASS: the installed session exists at the reported path — sessions/--tmp-ccc-pi-native-check-9_3yb_9m-workspace--/2026-05-04T09-15-00Z_01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2.jsonl [hard-link]
PASS: the installed file is a Pi session, installed where its working directory spells — sessions/--tmp-ccc-pi-native-check-9_3yb_9m-workspace--/2026-05-04T09-15-00Z_01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2.jsonl; entries {'compaction': 1, 'message': 10, 'role:assistant': 5, 'role:system': 1, 'role:toolResult': 1, 'role:user': 3, 'session': 1, 'thinking_level_change': 1}
PASS: the installed file carries the source's own first prompt — 'Summarize the retry policy in internal/retry/policy.go.'
PASS: the installed file holds the source's tool call and its result — call block(s) ['Read'], toolResult record(s) 1
PASS: the installed file's assistant records carry a usage object — 5 assistant message(s), 0 without a usage object
PASS: the load check verified the installed session natively — the harness's own loader read the session and answered about it (sessionId "01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2", messageCount 3); the same question with an unknown session id answered "No session found matching". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-pi-load-1527431558/sessions/--tmp-ccc-pi-native-check-9_3yb_9m-workspace--/2026-05-04T09-15-00Z_01a1ea8c-3713-71 …[the rest of this output was dropped]
PASS: the harness's own loader answers about the installed session — sessionId '01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2', messageCount 6
PASS: the loader's own message list is the file's model-visible history — loader ['compactionSummary', 'user', 'assistant', 'system', 'user', 'assistant'] vs file ['compactionSummary', 'user', 'assistant', 'system', 'user', 'assistant']
PASS: the unknown-id control arm reports no such session — No session found matching '01a1c335-dead-beef-0000-000000000000'
PASS: the load check ran against a copy and left the installed file alone
PASS: the resume check verified one real turn through the printed invocation — the harness answered from the installed session in one turn and wrote its own records into it (the session file went from 3067 to 32986 bytes)
the turn's own output (the target's answer, as the harness printed it):
    PROBE-TURN-OK

PASS: the turn's answer contains 'PROBE-TURN-OK' — PROBE-TURN-OK
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-9_3yb_9m/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-pi-native-check-9_3yb_9m/pi-home PI_CODING_AGENT_SESSION_DIR=/tmp/ccc-pi-native-check-9_3yb_9m/pi-home/sessions/--tmp-ccc …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-9_3yb_9m/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-pi-native-check-9_3yb_9m/pi-home PI_CODING_AGENT_SESSION_DIR=/tmp/ccc-pi-native-check-9_3yb_9m/pi-home/sessions/--tmp-ccc …[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — 01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2 is new among the session id(s) the home's own header lines record
scratch removed (/tmp/ccc-pi-native-check-9_3yb_9m)
RESULT: PASS — 23 check(s), none failed
```

What the two arms answer, in Pi's own words rather than this record's: `get_state` for the installed
session — `{"sessionId":"01a1ea8c-3713-71d9-ba22-7cb5a73e8ef2","messageCount":3, …}` (the count is the
message-visible history *at the moment each arm asked*, which is why the CLI's own load check — the arm
that drives a throw-away copy of the bytes the conversion installed — answers `3`, while the check's own
re-ask below, made after the real turn appended the harness's records, answers `6`) — and the unknown-id
arm's `No session found matching '01a1c335-dead-beef-0000-000000000000'`. The load arm runs with
`PI_API_KEY`, `ANTHROPIC_API_KEY`,
`OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `GOOGLE_API_KEY`, `GEMINI_API_KEY`, `XAI_API_KEY`,
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `AZURE_OPENAI_API_KEY` removed from the child
environment (the list `piLoadEnvDenied` carries, pinned name for name by a test), so the loader read the
session without a model turn. Three of the block's lines end where the script's own bounds cut them
(`…[the rest of this output was dropped]`): the load check's detail at 400 characters and the two printed
invocation lines at 200 each. The second half
ran one real turn through the invocation the report printed: the harness answered from the installed
session and wrote its own records into it (`3067 → 32986` bytes), and the answer is the block above.

Note what the *file* holds after the turn, because the `entries` line and the two counts must reconcile:
the `10` message entries the check counted are the `7` the pre-turn file held — the conversion's own `6`
(as the conversion's carried count says: three user, two assistant and the call-only entry the `toolCall`
block is written in) **plus** the `toolResult` entry, which the report counts as a result rather than as a
message — and the `3` records the turn appended (the harness's own system message, its user turn and its
answer), with the harness's own `thinking_level_change` record standing beside them and no message among
those. The count that reads `3` is the pre-turn file's model-visible history (the compaction entry plus
the two turns after it, which is what the CLI's load check asked about); the count that reads `6` is the
same walk after those three records landed on its path.

## 3. `pi:` as a source: a Pi session continuing in Claude Code

`ccc continue pi:01a1c335-7098-71c2-a91b-b5295c0464f5 --to claude --target-home /tmp/ev5/claude-home
--target-binary ccc-no-such-harness --data-dir /tmp/ev5/data`, with
`PI_DIR=<repo>/internal/ccc/testdata/fixtures/pi`, exit code 0:

```console
pi:01a1c335-7098-71c2-a91b-b5295c0464f5 → claude
  source:       <repo>/internal/ccc/testdata/fixtures/pi/01a1c335-7098-71c2-a91b-b5295c0464f5.jsonl
  read from:    source
  conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)
  boundary:     2713 byte(s) read
  records:      no line-record offsets claimed: the provider's own declarations do not make this source a transcript of appended line records
  converted:    claude-code-transcript-jsonl, 2773 bytes: 4 message(s), 1 tool call(s), 1 tool result(s), 0 boundary(ies), 0 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  omitted:      message_model_attribution ×3
  carried-with-caveat: unresolved_source_parents ×1
  installed:    /tmp/ev5r4/claude-home/projects/-home-user-code-example-api/39b1b6a1-e24b-460a-9159-1ed3f8996ead.jsonl [hard-link]
  source file:  unchanged (sha256 7395c3b0309a09501d9041b36ad46f471c513584e220940eb325174bc111a2e6, taken before and after the conversion)
  target id:    39b1b6a1-e24b-460a-9159-1ed3f8996ead
  working dir:  /home/user/code/example-api
  resume:       cd /home/user/code/example-api && CLAUDE_CONFIG_DIR=/tmp/ev5r4/claude-home ccc-no-such-harness --resume 39b1b6a1-e24b-460a-9159-1ed3f8996ead
  non-interactive resume: cd /home/user/code/example-api && CLAUDE_CONFIG_DIR=/tmp/ev5r4/claude-home ccc-no-such-harness -p --output-format json --resume 39b1b6a1-e24b-460a-9159-1ed3f8996ead -- '<prompt>'
  cli_version:  2.1.326 (writer-pin)
  structure:    verified (the inherited AgentsView Claude reader over the installed session)
  native load:  not-verified (the target harness's own loader (claude --bare --resume, with no model credentials in its environment, plus an unknown-id control))
                ccc-no-such-harness is not on PATH, so the installed session's native load could not be verified (ccc-no-such-harness is not on PATH)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume (claude -p --resume))
                not requested (--verify load)
  warning:      the target harness was not found on PATH: the conversation is converted and installed at /tmp/ev5r4/claude-home/projects/-home-user-code-example-api/39b1b6a1-e24b-460a-9159-1ed3f8996ead.jsonl, and its native load is unverified
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
EXIT=0
```

The block is this pass's re-run (the earlier revision's identical block carried no
`unresolved_source_parents` line: the fixture is the *linear* Pi shape, and a record that follows a tool
result is now counted where it was previously read as the file's own continuation — Q-0005-13 (b)); the
converted bytes, the message/call/result counts and the source digest are unchanged, and the generated
target id and scratch root differ per run.

`ccc inspect pi:01a1c335-7098-71c2-a91b-b5295c0464f5 --data-dir /tmp/ev5/data`, exit code 0:

```console
pi:01a1c335-7098-71c2-a91b-b5295c0464f5
  agent:        Pi
  session id:   pi:01a1c335-7098-71c2-a91b-b5295c0464f5
  source:       <repo>/internal/ccc/testdata/fixtures/pi/01a1c335-7098-71c2-a91b-b5295c0464f5.jsonl
  project:      example_api
  working dir:  /home/user/code/example-api
  conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)
  tools:        grep
  recorded:     2026-05-04T09:15:01Z .. 2026-05-04T09:15:09Z
  first prompt: Where is the retry cap configured?
  archive:      none at /tmp/ev5/data/sessions.db
  warning:      no archive at /tmp/ev5/data/sessions.db; reporting source facts only
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The same direction through the route's own native harness,
`python3 scripts/pi-native-check.py --direction pi-to-claude --claude
~/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --json`, exit code **0**:

```console
repo:        <repo>
direction:   pi-to-claude (pi:01a1c335-7098-71c2-a91b-b5295c0464f5 → claude)
target:      /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude (/home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude)
claude:      /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude
source:      internal/ccc/testdata/fixtures/pi/01a1c335-7098-71c2-a91b-b5295c0464f5.jsonl (copied to /tmp/ccc-pi-native-check-t2aov7rb/pi-source/01a1c335-7098-71c2-a91b-b5295c0464f5.jsonl)
workspace:   /tmp/ccc-pi-native-check-t2aov7rb/workspace
target home: /tmp/ccc-pi-native-check-t2aov7rb/claude-home
the source fixture's recorded working directory (/home/user/code/example-api) is rewritten to the workspace, so the session belongs to a path this run owns and the target's own lookup can find what it installs

PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 27
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr No conversation found with session ID: 437188a8-9a9d-45a6-80fd-89914ab63bbf
PASS: the installed session exists at the reported path — projects/-tmp-ccc-pi-native-check-t2aov7rb-workspace/f1169c9a-958c-42fd-bed4-80d8c0dbba53.jsonl [hard-link]
PASS: the load check verified the installed session natively — the harness's loader resolved the installed session and answered about it (subtype "success", terminal reason "api_error", result "Not logged in · Please run /login"); the same command with an unknown session id answered "No conversation found with session ID: 437188a8-9a9d-45a6-80fd-89914ab63bbf". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-claude-load-2626303448/proj …[the rest of this output was dropped]
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-t2aov7rb/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-pi-native-check-t2aov7rb/claude-home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume f …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-t2aov7rb/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-pi-native-check-t2aov7rb/claude-home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude -p --outpu …[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
scratch removed (/tmp/ccc-pi-native-check-t2aov7rb)
RESULT: PASS — 12 check(s), none failed
```

The Claude Code CLI *is* installed on this host — at
`~/.local/share/ccc-native/claude-cli/node_modules/.bin/claude`, the path 0003's record pins, answering
`claude --version` → `2.1.326` — it is simply not on `PATH`, and the harness had no way to name it. That
is what `--claude PATH` is for: it names the Claude Code binary for this direction the way `--pi` names
the Pi one, so the direction's native half runs here instead of being driven with a `PATH` prefix. What
the loader's own answer says is in the `the load check verified the installed session natively` line
above (the CLI's `verification.load.state`, quoted with the harness's own words: `subtype "success"`, the
session id, and the unknown-id control arm's `No conversation found with session ID: …`), and the earlier
claim that this half could not run here — with the reason that Claude Code was not installed — was
wrong. Nothing else about the direction changed: the read is still the inherited **Pi** reader's (the
source side), the conversion the shared pipeline's, and the installed transcript verified by the
inherited **Claude Code** reader (the target side) with the tool call and its result carried across. The
native half is re-derivable with the command above, and a host that has no Claude Code at all gets that
stated rather than a pass (`FAIL: the target harness (claude) is installed` and exit 2 — the verdict
0003's evidence record already established for that harness). The route's own Go tests pin the same
properties offline for both directions
(`TestContinueFromPiThroughTheCommonModel`, which runs `pi:… → --to claude` and `pi:… → --to codex`).

## 4. A branched and compacted Pi history is not flattened

The committed second fixture (`pi/01a1c336-4b02-78d3-9e15-6c7a2f0b8d41.jsonl`) is the shape a conversion
is easiest to get wrong: an active path (a prompt, an answer, and a later assistant turn whose `parentId`
names the entry the answer's own line continued from) with a **branch the conversation left** standing off
it — a compaction boundary and the prompt/answer exchange that followed it, which the file's last entry
never reaches. What Pi's own loader builds from that file is therefore not the summary and the exchange
but the three turns of the last entry's own path.

0005's review measured the conversion this route shipped before its fix: chaining every written entry to
the one before it put the compaction entry *on* the walk, so the loader read
`[compactionSummary, user, assistant, assistant]` for the converted session while the source's own file
answers `[user, assistant, assistant]` — the abandoned prompt "And the retry cap?" presented as an active
user turn, and the source's visible line hidden behind a boundary that stood off it. The writer now
chains each entry to the entry of the record the source named as its parent, so the target file is the
source's tree. Re-derived on this host, offline (a one-off probe under `/tmp/`, not shipped, driving
Pi's own RPC loader the way the harness does: the fixture copied with its recorded `cwd` rewritten to the
probe's own workspace, converted with `/tmp/ccc-bin continue pi:… --to pi --target-home … --target-binary
pi --verify load --json`, then `pi --mode rpc --session <id>` with `get_state`/`get_messages` against
copies of the source and of the conversion, each in its own scratch agent directory):

```console
=== BEFORE (every entry chained to the one before it)
cli load:    verified — the harness's own loader read the session and answered about it (sessionId "01a1ea30-8bcc-7d97-b0c7-6a00b055f941", messageCount 4); the same question with an unknown session id answered "No session fo…[the rest of this output was dropped]
loader(source):      messageCount 3 roles ['user', 'assistant', 'assistant']
loader(conversion):  messageCount 4 roles ['compactionSummary', 'user', 'assistant', 'assistant']
same message list:   False
=== AFTER (the source's own relation reproduced)
cli load:    verified — the harness's own loader read the session and answered about it (sessionId "01a1ea30-a1c5-721d-8c6e-db10c18816e9", messageCount 3); the same question with an unknown session id answered "No session fo…[the rest of this output was dropped]
loader(source):      messageCount 3 roles ['user', 'assistant', 'assistant']
loader(conversion):  messageCount 3 roles ['user', 'assistant', 'assistant']
same message list:   True
```

| Property | Evidence |
| --- | --- |
| the fixture's own branch is a branch, not one line | `TestPiHistoryFixtureBranchIsNotOnTheActivePath` walks the fixture file's own `id`/`parentId` tree from its last entry (the walk the harness's own loader makes) and asserts that at least one *message* entry — a user record among them — is off that path, that every off-path entry reaches the path through its own parents, and that the last entry is on its own path |
| the conversion keeps the source's own conversation | `TestContinueIntoPiKeepsABranchedSourcesOwnConversation` converts the fixture `pi:… → --to pi` and requires the installed file's model-visible walk (`piModelVisibleRoles`, the loader's own rule) to equal the source file's, which is the measured `[user, assistant, assistant]`; the abandoned prompt and its answer are in the file and off that walk, and the compaction entry the source recorded off its path is written and stays off it |
| the writer's own rule, pinned without a fixture | `TestPiSessionWritesTheSourcesOwnRelations` converts a document whose records carry their own identities and parents and asserts each written parent: a branch hangs off the record it named, a record after a tool result hangs off the result (so a resumed session is never left with a call and no result), a record whose parent the target omitted is chained to the nearest written ancestor, and a record naming a parent outside the read follows the entry before it and is counted (`unresolved_source_parents` = 1) |
| the source read reports the boundary once, with its summary | `TestContinueFromPiKeepsACompactedAndBranchedHistoryCoherent` reads the fixture through the inherited Pi reader and asserts exactly one `IsCompactBoundary` message whose text is the fixture's own summary |
| the target gets the boundary as its own record, not as invented turns | the same case asserts the conversion carried one boundary, that the target's own read-back sees one, and that `pre_compaction_history` names the turns the boundary stands in front of |
| the boundary's record names no kept range and no checkpoint | `TestPiSessionCarriesACompactionBoundaryAsTheTargetsOwnRecord`: `firstKeptEntryId` is the record's own id (Pi's retain-none shape) and `systemMessage` is an explicit `null` |
| the branch's turn is *recorded history*, not an invented active turn | the writer writes exactly the entries the reader gave it, in the file's own order, and the walk the target's own loader makes is the source's; the assistant turn the conversation left stays in the file, off that walk, as the source's own file holds it (the BEFORE/AFTER block above is that assertion taken natively) |

The same fixture is the shape this increment's *outbound* routes had to be told about, and the review
round this pass answers measured the difference there too: the Piper → Claude Code and Pi → Codex
conversions this build shipped wrote every record/message, so the compaction boundary and the retry
exchange the fixture records *off* its own line became part of the target's model-visible conversation —
the abandoned prompt standing as an active user turn. The two routes now handle it in the two ways their
formats allow, and each is pinned by a case that measures the *context* (the walk the target's loader
makes from the file's last record) rather than a file-wide role count:

| Property | Evidence |
| --- | --- |
| a Claude Code target is written as the source's own tree | `TestContinueFromPiKeepsACompactedAndBranchedHistoryCoherent` converts the fixture `pi:… → --to claude` and requires the installed transcript's own walk — the path from its last record back through `parentUuid`, read by `transcriptWalkTexts` from the file's own bytes — to equal the source file's walk (`[user, assistant, assistant]`): the abandoned prompt and its answer are in the file *off* that walk, and the compaction pair stands beside the branch as the source recorded it. Read back through the inherited Claude reader: `structure: verified` with the boundary the conversion wrote. The case is red against the sequential chain this build shipped (measured while this pass was written: that walk held the summary, the abandoned prompt and its answer, in the file's own order). |
| a Codex rollout target writes the source's line and counts the rest | `TestContinueFromPiIntoCodexOmitsAnAbandonedBranch`: the conversion reports `inactive_branch_records = 3` (omitted), `carried.CompactionBoundaries = 0`, and the installed rollout holds no `compacted` record — the boundary stood on the branch, and a record written there would cut a line it was never on; the read-back is the source's three turns and the branch's words are absent from the file, because a response item written into this target's log becomes model-visible history. |
| the archive read answers the same relations as the source read | `TestContinueFromTheArchiveKeepsAPiSessionsBranch`: the fixture is ingested into an isolated archive, its source copy removed, and the pipeline's fallback conversion (`read from: archive`) must write the same model-visible walk as the source file's own, with `unresolved_source_parents = 0`. It is red on the tree before this pass (`messageFromArchive` dropped the two identity columns while the inherited `messages` table held them): that conversion's walk was `[user, assistant, user, assistant, assistant]`. |
| a record that emits dialogue and a result is one place in the tree | `TestPiSessionChainsPastAMixedRecordToItsResult`: the entry written for the record's tool result is what the record after it hangs off (the record's *final* entry), the result is on the loader's walk, and the walk's roles are `[user, assistant, user, toolResult, assistant]` — red against mapping the source record to its first entry, which leaves the result off the walk while the file still holds it. |

What the conversions do **not** do, and say so: none synthesises a `branch_summary` entry (the source
records no summary for the abandoned path, and writing one would invent it) and none presents the
abandoned turn as an active one. The two tree-shaped targets keep every record the source wrote — off the
walk when the source held it off one — while the Codex target, whose log has no parent relation to hang a
record off, omits the off-line records and counts them (`inactive_branch_records`) instead of reviving
them as active history. `sidechain_messages` is the category for records the *reader* marks as a
subagent's turn; those are omitted and counted, and `unresolved_source_parents` counts the relations the
read could not answer.

## 5. What travels, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| a Claude Code conversation with a tool call, its result and a compaction boundary | converted into a Pi session whose file the native run's own counts describe: `entries {'compaction': 1, 'message': 10, 'role:assistant': 5, 'role:system': 1, 'role:toolResult': 1, 'role:user': 3, 'session': 1, 'thinking_level_change': 1}` — one header, one compaction record, the messages, and the harness's own `thinking_level_change` record beside them (§2) |
| the tool call and its result | `PASS: the installed file holds the source's tool call and its result — call block(s) ['Read'], toolResult record(s) 1` (§2); the inherited Pi reader agrees on the round trip (`structure: verified` in §1 and §3) |
| the source's own words | `PASS: the installed file carries the source's own first prompt — 'Summarize the retry policy in internal/retry/policy.go.'` (§2) |
| a Pi conversation with a tool call and its result, as a **source** | converted into a Claude Code transcript with `1 tool call(s), 1 tool result(s)` carried (§3), read back `verified` by the inherited Claude Code reader |
| the model the source's messages recorded | carried verbatim on each converted Pi assistant entry (`"model":"claude-sonnet-4-5"` in §1's excerpt) and reported as the `message_model_attribution` caveat; **no provider** is written, because the read answers the model each source message recorded but not the endpoint that served it (the read's session-level provider names the harness that wrote the words), and the report records none (`model_provider_source` = `unrecorded`, with the reason in `model_provider_detail`), while the session binds no model of its own (`model_provider_binding` in the format limits) |
| a Pi conversation with a branch the conversation left, as a **source** | `pi:… → --to claude` is written as the source's own tree, so the target's own walk is the source's; `pi:… → --to codex` omits the records the source placed off its line and counts them (`inactive_branch_records ×3`) — both pinned by the cases §4's table names |
| the converted history as the **answer** of a real turn in the installed Pi session | the recorded turn's answer holds the words the check asked for — `PROBE-TURN-OK` (§2); the same run reports the turn's own growth beside it (3067 → 32986 bytes, §2, and §8 explains why a re-run's number differs) |
| the source file during and after every run | `source file: unchanged (sha256 …, taken before and after the conversion)` in each CLI run (§1, §3), and in the native run `PASS: the source fixture tree is unchanged — 1 file(s) digested before and after` with the planted unrelated session and every session the home already held unchanged beside it (§2) |
| a session the target home already held | untouched in every run: `PASS: the planted unrelated session is unchanged` and `PASS: every session the home already held is unchanged` (§2), and `PASS: the conversion wrote no id the home already held` |

The row that rests on the target model's own words is the `PROBE-TURN-OK` one: it is the block the
harness printed under `the turn's own output (the target's answer, as the harness printed it):`, quoted
verbatim above, and the case that keeps a record from quoting an answer it did not make reads exactly
that block (`scripts/tests/test_evidence_claims.py`, which this record is registered with).

## 6. Gate, tests, docs and bookkeeping

`python3 scripts/gate.py`, exit code 0:

```console
gate: <repo>
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3601 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 251 tests in 5.845s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 29 project-owned prefix(es); files seen by git: 5669 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 121 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

No gate step was added, skipped, bounded or narrowed: the new tests live in `internal/ccc` and `cmd/ccc`,
which `scripts/gate-targets.txt` already builds and runs, the new Python case module lives in
`scripts/tests/`, which the `selftest` step already discovers, and the carry's `parity` step is unchanged
because the pin, the manifest's carried records and every carried byte are untouched (the manifest gained
one `owned` line, which is this repository's own file list, not a carried path). The fifth review round
adds no Python case (its five new cases are Go ones), so the `selftest` count stays `251`; the narrowed
tree's `func Test` count moves `274 → 279`.

Test counts and the changes to existing cases:

| Change | Where |
| --- | --- |
| the writer's record shape, the tree link, the usage object, the call block, the compaction record, the omissions, the loss categories, the cwd refusal, the project spelling | `internal/ccc/pi_target_test.go` (new, 8 cases) |
| the route's property tests: conversion + installation + read-back, the invocation, the dry run, the cwd refusal, both `pi:` source directions, the compacted/branched fixture, the fixture's own tree walk, the load check's refusal, the carried byte count | `internal/ccc/continue_pi_test.go` (new, 8 cases) |
| the CLI: `--to pi` end to end with the JSON document, `pi:` as a source, `inspect pi:…` | `cmd/ccc/pi_route_test.go` (new, 3 cases) |
| the fixture tree gained the Pi provider root and its two fixtures, and the parity oracle a Pi case | `internal/ccc/fixtures_test.go` (`fixtureConfig` gains the Pi root), `internal/ccc/fixturegen/main.go`, `internal/ccc/parity/dump.go`, `scripts/upstream-compare.py`, `internal/ccc/parity/testdata/oracle/pi.json` |
| one existing CLI case was updated, not weakened: `TestContinueRefusesATargetThisBuildDoesNotWrite` asserted the refusal names every target this build writes, and the build now writes three | `cmd/ccc/claude_route_test.go` |
| the verifier's pure walks, its two seams and their decisions: the version line, the environment denylist, the loader cwd, the RPC answers, the model-visible walk, the probe's missing-binary answer, the load check's verified and refused legs, the resume check's growth and refusals | `internal/ccc/pi_verify_test.go` (new, 10 cases) |
| the installer's exclusive hard link, its 0600 request, the `ErrTargetExists` refusal and the no-hard-link refusal | `internal/ccc/pi_install_test.go` (new, 2 cases) |
| the native harness's own rules, offline: the printed invocation read the way a POSIX shell reads it, the `--direction` selection, the `INCOMPLETE` verdict, the answer block's four-space shape, and the shared fixture-copy rewrite | `scripts/tests/test_pi_native_check.py` (new, 15 cases) |
| the applied simplify findings, each with a case that fails without it: the Pi loss categories closed in both directions, the read-back pinned to the fixture's own `1`/`1`, the abandoned branch asserted as an assistant record and not a prompt, the pipeline compared with a direct writer call over the same read, one pair of harness bounds, and the two verifier seams | `internal/ccc/pi_target_test.go`, `internal/ccc/continue_pi_test.go`, `internal/ccc/codex_verify_test.go`, `internal/ccc/pi_verify.go` (cases added; none removed or weakened) |
| the fix pass's cases: the source relation (a branch, a tool result, a record the target omits, a parent outside the read), the branch fixture's own tree, the conversion that must keep the source's walk, the control arm that wrote a session of its own, the sessions tree the check cannot read, the whole produced file name, and the pre-boundary composition read off the file | `internal/ccc/pi_target_test.go` (8 → 9 cases), `internal/ccc/continue_pi_test.go` (8 → 9), `internal/ccc/pi_verify_test.go` (the same 10 cases, with subtests added to `TestVerifyPiLoadRefusesEveryBadAnswer`) |
| this pass's cases, one per review finding it answers: the projection read across an intervening branch (Pi and Codex, each with an exact-read counter-arm), a reasoning trace kept under whitespace-only dialogue, the leaf selection refused when the entry written last is off the read's own line and the same document with a representable leaf, the printed invocation read back against the check that runs it, and the credential-free loader copy with the sessions tree the loader run names | `internal/ccc/pi_target_test.go` (12 → 15 cases), `internal/ccc/codex_target_test.go` (20 → 21), `internal/ccc/claude_target_test.go` (10 → 11), `internal/ccc/continue_pi_test.go` (11 → 12), `internal/ccc/pi_verify_test.go` (the same 10 cases, with the environment, session-tree and invocation assertions extended), `scripts/tests/test_pi_native_check.py` (35 → 39) |
| one existing case lost a `t.Skip` and now runs on every host: `TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere` owns the missing directory it asks about and answers the harness lookup through the check's own seam | `internal/ccc/continue_pi_test.go` |
| one existing case gained the third provider's row: the no-line-feed rule now holds for a Pi source too, with the boundary's offset claim asserted per provider (the Pi provider's declarations claim none, and its boundary says so) | `internal/ccc/continue_growth_test.go` (the `pi-complete-turn.jsonl` fragment is now consumed, which is what it was committed for) |
| the native harness's own cases: the walk, the full invocation word vector, the session identity read from headers, the usage rule, the guarded provider read, the named skip for a host with no provider, the `--claude` flag, and the build output's location | `scripts/tests/test_pi_native_check.py` (15 → 30 cases) |
| the fix pass this round answers: the Pi → Claude Code history conversion measured by the installed transcript's own walk, the Pi → Codex conversion that omits the off-line records and reports no boundary, the archive fallback that must answer the same relations as the source read, the mixed dialogue/result record that must leave its result on the walk, the assistant entry that records the model and no provider it cannot answer, and the identified-but-parentless record whose invented relation the count covers | `internal/ccc/continue_pi_test.go` (9 → 11 cases), `internal/ccc/pi_target_test.go` (9 → 12) |
| the two loss-category lists gained the categories the new code paths report (`unresolved_source_parents` for the Claude target; `inactive_branch_records` for the Codex target), and one existing case was retargeted at the path it now pins (`NoProviderConfiguredIsANamedSkip` runs with `give_home=False`, so it still exercises the seeded-scratch-home path whose skip names the host's configuration) | `internal/ccc/claude_target_test.go`, `internal/ccc/codex_target_test.go`, `scripts/tests/test_pi_native_check.py` |
| the native harness's own cases for the fixes: `--pi PATH` resolving the executable the conversion is driven through (asserted against the version query and `--target-binary`), the missing-Pi run's `RESULT: INCOMPLETE` verdict and the reverse direction needing no Pi at all, and the run's own home being seeded while a caller-named home is never written to (asserted both in the report and against the file system) | `scripts/tests/test_pi_native_check.py` (30 → 35 cases) |
| the third review round's cases: the adjacent exact-parent shape (a record naming a call immediately after that call's result) on both the tree writer and the Codex walk, the Codex report's accounting of the places its walk could not answer, and the seed's nested credential levels (a provider's `models` entry and its `modelOverrides` entry, plus the two fields' asymmetry — the key re-spelled as an `$ENV` reference, a header reference dropped) | `internal/ccc/pi_target_test.go`, `internal/ccc/codex_target_test.go` (one new case), `scripts/tests/test_pi_native_check.py` (39 → 41 cases) |
| the fourth review round's cases: a call's outcome the archive folds onto the call's own message, in both spellings (`ToolResults` on that message and the call's `ResultContent` summary), on the Pi tree writer, the Claude tree writer, the Codex line walk and the pipeline's archive read — each with a sibling turn naming the call's parent that must not be chained through the abandoned call's result — plus the seed's endpoint policy (every URL branch of `credential_free_url` at both levels a definition can name an endpoint, the credential-free counter-arm, and the run-level refusal naming the provider and recording the named skip) | `internal/ccc/pi_target_test.go`, `internal/ccc/claude_target_test.go`, `internal/ccc/codex_target_test.go`, `internal/ccc/continue_pi_test.go` (one new case each), `scripts/tests/test_pi_native_check.py` (41 → 44 cases) |
| the fifth review round's cases: the omission/projection composition on both tree writers (a Pi chain call → result → omitted empty assistant → next turn, which a re-resolved raw relation leaves the result off the walk of), the same tree case that pins `endRecord`'s two effects (the place kept, the walk dropped), the omitted record whose own walk resolved nothing and whose child follows the document's relations instead, and a read that recorded a cycle (a record naming itself, and a two-record loop) which the size bound ends | `internal/ccc/pi_target_test.go` (16 → 20 cases), `internal/ccc/claude_target_test.go` (12 → 13) |

The upstream comparison (`python3 scripts/upstream-compare.py --record`, network + a pristine checkout at
the pin) ran three cases and agreed on all three:

```console
claude: fixture sha256=2bd510821d4abf83… ours=6409a23c502469a3… pristine=6409a23c502469a3… agreement=yes
codex: fixture sha256=d9957a8d6f5731a3… ours=531e5c0fb19fc51a… pristine=531e5c0fb19fc51a… agreement=yes
pi: fixture sha256=7395c3b0309a0950… ours=6751fa8b209e9077… pristine=6751fa8b209e9077… agreement=yes
RESULT: PASS — 3 case(s) agree byte for byte with the pinned revision
```

Docs and bookkeeping in this increment: `AGENTS.md` (the route list, the third native check and its
`INCOMPLETE` verdict, the Pi provider statement, the layout line, the project-spelling rule), 
`docs/conventions.md` (layout, the discoverability rule for two path-scoped harnesses and the copy the
native check drives, the fixture rule), `docs/harness.md` (the new script's entry and why it is not a
step), `README.md` (`--to pi`, `pi:` as a source, both resume lines), `docs/goals/README.md` (0005 →
implemented on this branch; 0004 archived, see below), `docs/decisions/OPEN-QUESTIONS.md` (Q-0005-01 …
Q-0005-06), and this record. `docs/upstream.md` and every carried file are untouched.

The simplify pass that applied the accepted findings to this increment changed: `internal/ccc/route_shared.go`
(new — the install/verify/probe/turn machinery the three routes now share, so `AGENTS.md`'s and
`docs/conventions.md`'s layout lines name it), `docs/harness.md` (the shared native-check mechanisms,
`rewrite_source_cwd` and `bounded_block`, named in the `native_check_common.py` entry),
`docs/plans/0005-pi-continuation.md` (the native check's real flag spelling, `--direction pi-to-claude`),
and `cmd/ccc/main.go` (the usage text no longer claims a Pi resume command names a provider or a model;
it prints neither). §2's and §3's native blocks and §6's and §8's numbers are that pass's runs, and
§1's and §3's CLI conversion counts were re-derived unchanged (same bytes, counts and digests).

The fix pass that applied 0005's review findings changed `internal/ccc/pi_target.go` (every entry is
chained to the entry of the record the source named as its parent, and a relation the read cannot answer
follows the entry before it and is counted under the new `unresolved_source_parents` category),
`internal/ccc/conversation.go` and `internal/ccc/reader.go` (`Message.SourceUUID` /
`Message.SourceParentUUID` — the read's own answer about the source's tree, which the writer needs and
the interchange document did not carry), `internal/ccc/pi_verify.go` (the model-visible walk is the
loader's own path walk, and the control arm's scan walks the sessions tree instead of listing the root's
own entries), `scripts/pi-native-check.py` (its `model_visible_roles` mirrors that walk; `--claude PATH`;
the build under `bin/`, which `.gitignore` covers; the planted session removed in a `finally`; the named
`SKIP` a host with no provider configured now gets instead of a traceback; the printed invocation compared
as a whole word vector; the session identity read out of each file's header; the `usage` object parsed
rather than searched for as a substring; and the load state recorded for the non-Pi target), `AGENTS.md`,
`docs/conventions.md`, `docs/harness.md`, `docs/plans/0005-pi-continuation.md`, `README.md` and the cases
named in §6's table. Every number §2, §3 and §6 quote above was re-derived in that pass with the harness
itself (`23` and `12` checks, `29` Pi loss categories, the `3107 → 33039` growth), and §1's CLI
conversion was re-derived unchanged (same counts, bytes and source digest — only the generated session id
and file name differ per run); §8 lists them, and §4's BEFORE/AFTER block is the fix's own native
measurement.

The fix pass this round answers changed, for the outbound routes, `internal/ccc/route_shared.go` (the
shared `sourceTree`: one relation-resolution rule — a record's final entry is the place its children hang
off, a tool result stands between a call and what follows it, a relation the read cannot answer follows
the entry before it and is counted, and `inactiveSourceRecords` answers the same question for a target
whose format is a single line), `internal/ccc/claude_target.go` (each record is chained to the entry of
the record the source named as its parent, and the boundary pair's records stay beside the branch the
source recorded them beside), `internal/ccc/codex_target.go` (the source's own line is written; records
the source's relations place off it are omitted and counted as `inactive_branch_records`),
`internal/ccc/archive.go` (`messageFromArchive` carries `SourceUUID`/`SourceParentUUID`, which the
inherited messages table held and the adapter dropped), `internal/ccc/pi_target.go` and
`internal/ccc/targets.go` (a converted assistant entry records the source's model and **no provider**,
because the read answers no endpoint attribution — the read's session-level provider names the harness —
so the report records none and says why, instead of writing the harness label the earlier revision
wrote), `internal/ccc/route_shared.go`'s shared caveat texts (`unresolvedSourceParentsDetail`,
`preBoundaryHistoryDetail` — the pre-boundary caveat now says what the target does where the source
recorded the boundary *off* its own line), `scripts/pi-native-check.py` (the seed is written only into
the scratch home the run makes itself and a caller-named `--target-home` is read, never written to;
`--pi PATH` names the executable the *conversion* is driven through, not only the one this harness
queries; a missing Pi for `--direction claude-to-pi` is the `RESULT: INCOMPLETE` verdict before any check
runs, while the reverse direction runs without Pi), `AGENTS.md`, `docs/conventions.md`,
`docs/harness.md`, `docs/goals/archive/0004-repeatable-evolving-session-handoffs.md` (the prerequisite
link now resolves inside `archive/` and the status matches the index), `README.md`, and the cases §6's
table names (Go test functions `258 → 263`, none removed; the suite `237 → 242`). §1's and §3's CLI
blocks and §2's, §3's and §6's native/gate blocks above are this pass's re-derived runs (the Pi document
is `3051` bytes where the earlier revision's was `3091` — the two `"provider":"…"` fields it no longer
writes — and the Pi → Claude Code transcript is unchanged at `2773` bytes, because that source is one
line), and §8 lists the numbers.

**The CI repair pass** answers the one real failure of `gate (windows-latest)` at this branch's head: the
`selftest` step failed on `scripts/tests/test_pi_native_check.py`'s
`test_the_loader_run_names_the_sessions_tree_the_printed_line_names`, which asserted a POSIX spelling of a
synthetic home — `str(Path("/loader-home"))` is `\loader-home` on Windows, so the value the check passed
through could never read back as the literal — and computed the expected sessions tree from that
POSIX-shaped string while the run's own value was joined with the host's separator. Only that one case
changed: it now drives the home under this host's own flavour and under `PureWindowsPath` (what `Path` is
on Windows), compares the agent directory against the home the check was given, and takes the expected
tree from the same `pi_session_tree` the printed line is built from. No case was removed, skipped or
weakened, and the repaired case still fails against three broken spellings of the shipped `ask_pi_loader`
(the sessions-tree variable never set, the agent directory handed the sessions tree, the tree computed
from the workspace's parent — measured in this pass, under both flavours, in a throw-away harness outside
the tree). The suite's case count is unchanged at 246, so §8's numbers and the transcripts above stand,
and `python3 scripts/gate.py` ends in `GATE: PASS` with every step `PASS` on this pass's re-run.

**Goal 0004 moved to the archive in this increment.** It was still sitting in `docs/goals/` with a
"Implemented on `feat/0004-repeatable-evolving-session-handoffs`" status while `origin/main` already
carried its merge (`8a8265a feat(0004): Repeat continuation as source history grows (#6)`), which is the
condition `AGENTS.md` states for archiving. The file moved to
`docs/goals/archive/0004-repeatable-evolving-session-handoffs.md`, the three links that named it from
`docs/plans/` and `docs/evidence/` were retargeted at the archive path, and — in this fix pass, after the
review round found both — its own prerequisite link now resolves inside `archive/` (it had been left
spelling `archive/0002-…`, which the move made `archive/archive/0002-…`) and its `Status` line reads
`Accepted: merged onto main as 8a8265a; archived`, matching the index row instead of still awaiting
acceptance.

**The third review round's fix pass** answers that round's three findings. `internal/ccc/route_shared.go`
gates the tool-result step on the read that projects — the step exists to undo a projection, and an exact
answer (a Claude transcript's `parentUuid`) names the record it means, so a result standing beside a
record the read chains to the same message is that record's *sibling*; chaining through it revived the
sibling — a user record can hold dialogue beside its result — as an ancestor of the active path, on both
the tree writers and the Codex walk — and `inactiveSourceRecords` answers the count beside its flags: the
places the Codex line walk chose that the read did not answer (a record chained through a result standing
between it and the call it named; a record the read left parentless that is not the conversation's first;
a record naming a parent this read does not hold). `internal/ccc/codex_target.go` reports it as
`unresolved_source_parents` (carried-with-caveat) with its own detail; the records themselves are still
written. The fixture-backed conversions' other counts are unchanged (the whole narrowed suite is green and
the branch fixture's `inactive_branch_records = 3` still holds). `scripts/pi-native-check.py`'s seed
scrubs credential-bearing fields at every level Pi's schema allows one (a provider's `apiKey`/`headers`, a
`models` entry's `headers`, a `modelOverrides` entry's `headers`) instead of only the provider's own
level; on this host no provider definition carries a nested credential field, so the seeded document (and
every printed number the harness reports) is byte-identical, and the check was re-run anyway:
`claude-to-pi` PASS — 23 checks, none failed, one real turn through the printed invocation — and
`pi-to-claude` PASS — 12 checks, none failed.

Each rule this pass adds is pinned by one executed mutation (revert the line, run the case that claims it,
restore the line, record the failure): the ungated tree step fails
`TestPiSessionChainsThroughAToolResultAcrossAnInterveningBranch`, the ungated Codex walk, the uncounted
projection, the uncounted parentless-identified place and the uncounted parent-not-held place fail
`TestCodexRolloutKeepsAnActiveResultAcrossAnInterveningBranch` /
`TestCodexRolloutCountsThePlacesItsWalkCouldNotAnswer`, and the shallow seed filter fails the three
seeding cases — measured in this pass, every mutation reverted before the gate run. `AGENTS.md`,
`docs/conventions.md`, `docs/plans/0005-pi-continuation.md`, `docs/harness.md` and a new Q-0005-12 (with
Q-0005-10's exact-read clause corrected) carry the rule, and §6's and §8's numbers are this pass's gate
run (`248 selftest cases`, `270 Go test functions`, `121 owned file(s)` scanned).

**The fourth review round's fix pass** answers that round's three findings — the class that had survived two
rounds, so each rule it adds now carries an executed mutation (revert the line, run the case that claims
it, restore the line, record the failure) and the rules the round touched were re-audited the same way
(eleven mutations, seven on the Go side and four on the Python side; one mutation showed `parentFor`'s copy
of the projection gate to be unreachable — registration is gated at its single site — so that copy was
deleted and the gate's own site mutated instead, which fails the exact-read assertions).
`internal/ccc/route_shared.go` registers a written tool result under the record a record *hanging off* it
would read as its parent (`toolResultCarrierRecord`, one function for all three routes) instead of under
the walking record's read-answered parent. The archive read folds a result-only row onto the call's own
message and drops the row
(`ingest.PairAndFilterContext` → `archive.messageFromArchive`), so the walking record *is* the call's
message: registering its outcome under that message's parent let a sibling of the call be chained through
the abandoned call's result — reviving the call branch — on the tree writers and, through
`sourceToolResultBefore`'s carrier test, on the Codex walk. The projection step's place is now counted
wherever it is taken, including when the result stands immediately before the record (`parentFor` lost its
uncounted immediate-result shortcut and `sourceLineParent` returns the step counted): the two source trees
`C, R(parent=C), A(parent=C)` and `C, R(parent=C), A(parent=R)` are the same answer in the read, so
adjacency cannot settle which the file held — the round's measurement. The registration itself stays gated
on the read that may project. `internal/ccc/pi_target.go` passes the carrier per written result and
`internal/ccc/claude_target.go` once per outcome-holding record (the outcomes travel as one user record
there), so both writers use the one function. `scripts/pi-native-check.py` refuses a selected provider
whose endpoint URL carries a credential (`carries_credential_endpoint`: URL userinfo, a query or a
fragment, at any level a definition can name an endpoint) — the provider is left out of the seeded
document *whole* rather than copied or rewritten to a credential-free address, the `pi models:` line names
it, and a run that seeded nothing because of it records the named `SKIP`; the URL policy itself moved to
the shared `scripts/native_check_common.py` (`credential_free_url`) and `scripts/codex-native-check.py`
now imports it, so the two seeding checks cannot drift. The counts this changes are disclosed in §3 and
§8: the committed *linear* Pi fixture's conversions report `unresolved_source_parents ×1` on both outbound
routes where the earlier revision reported nothing, with the converted bytes unchanged. Both adjacent
projection assertions were updated with their comments (`0 → 1`, and `TestPiSessionWritesTheSourcesOwnRelations`
`1 → 2`); no case was removed, skipped or weakened. The native halves were re-run in this pass:
`claude-to-pi` `21 check(s), none failed` (the §2 block's 23-check run is the same check list with the two
real-turn checks) and `pi-to-claude` `12 check(s), none failed`; §1's CLI conversion was re-derived
unchanged (same bytes, counts, boundary and source digest). `AGENTS.md` is deliberately unchanged: this
pass alters no layout, gate step, command, read order or agent rule — what it changes is harness behaviour
and conversion relations, recorded in `docs/harness.md` (the Pi check's entry gained the endpoint rule and
the shared-module entry gained `credential_free_url`), `docs/conventions.md` (the tree rule's counting
clause), `docs/plans/0005-pi-continuation.md` (the same clause, corrected where the plan states it) and
Q-0005-13/Q-0005-14.

**The fifth review round's fix pass** answers that round's one finding, on the class that had survived two
rounds, so the surface was closed out with an executed mutation per rule rather than one patch for the one
shape. The finding: where Q-0005-10's projection step met Q-0005-07's omitted-record consequence, an
omitted record's *resolved* place was discarded. For the Pi chain `U → C → R → E → A` (`C` makes a call,
`R` answers it, `E` is an empty assistant record, `A` names `E`) the inherited reader answers `E`'s parent
as `C` while the source's own file placed it under `R`, so `startRecord` resolved `E`'s first-entry parent
to `R`'s entry — and `endRecord` threw that resolution away when `E` produced no entry, after which `A`
re-walked `E`'s *raw* relation to `C`'s entry. The converted path became `U → C → A` with the written
result standing in the file off the walk the target's loader makes, although the report counted it as
carried (the finding was itself static; this pass executed the reproduction: both new writer cases are red
on the tree before the fix — `the record continuing from an omitted one hangs off "", want the omitted
record's resolved place, the tool result ("internal/retry/config.go:41")`, with the Pi case's
`the model-visible history = [user assistant assistant], want [user assistant toolResult assistant]`, and
the Claude case naming the call's record where the result's is expected). `internal/ccc/route_shared.go`
keeps the place under the omitted record's identity and answers a child with it before stepping through the
raw relation (`omittedAncestorEntry`, registered by `endRecord` exactly when nothing was written for the
record — `wrote` clears the resolved parent, so a record with entries of its own needs no flag to be told
apart from one that wrote nothing); a record whose own walk resolved nothing keeps no place, and a child
of it follows the document's relations to the nearest written ancestor as before. Both tree writers get the
rule through the shared tree. Five Go cases were added (no Python case, so the `selftest` count is
unchanged): the composition on the Pi writer and on the Claude writer, the tree-level case that pins both
of `endRecord`'s effects (the place kept; the walk dropped, so a write that follows an ended record records
no relation of it), the omitted record with no resolved place whose child follows the document's relations
to its written ancestor, and a read that recorded a cycle (a record naming itself and a two-record loop)
that the walk's size bound ends. The mutation sweep — revert the line, run the case that names it, restore
the line — covered the resolution surface's rules; every mutation below failed its named case except the
two the sweep found unpinned, whose pins were added and whose mutations then failed:

| Rule (`internal/ccc/route_shared.go`) | Line disabled | Observed |
| --- | --- | --- |
| an omitted record's resolved place answers a child (`writtenAncestor`) | `omittedAncestorEntry` lookup deleted | both new writer cases FAIL (`the record continuing from an omitted one hangs off ""`, `… hangs off <call record>`) and the Pi model-visible history reads `[user assistant assistant]` |
| the place is registered (`endRecord`) | registration body deleted | the same two cases and the tree case FAIL (`the omitted record's place = "", false`) |
| the walk is closed after an omission (`endRecord`) | `t.pending = sourceRecordOrigin{}` deleted | whole suite green — the rule was unpinned; the tree case added for it then FAILs (`an entry written after a record ended recorded "s1" as the record it stands for`) |
| a record's *final* entry is the place its children follow (`wrote`) | map write made first-entry-wins | `TestPiSessionChainsPastAMixedRecordToItsResult` FAIL (`hangs off … "the output:"`) |
| a written record's entry answers for a reader of it (`writtenAncestor`) | `writtenEntryID` answer replaced by "not found" | `TestPiSessionWritesTheSourcesOwnRelations` FAIL (`the abandoned turn hangs off "after the result", want … "answer"`; count `7`, walk flattened) |
| a written tool result answers for a record naming its carrier (`parentFor`) | `toolResultEntry` lookup deleted | the two projection cases FAIL (result off the walk, count `0`) |
| the projection is gated on the read that may project (`wrote`) | `projectedParents` gate dropped | the exact-read counter-arms FAIL (`Count:1`, want `0`; a sibling result on the walk) |
| the projection's place is counted (`parentFor`) | `t.unresolved++` deleted in the projection branch | three count assertions FAIL (`Count:0`, want `1`/`2`) |
| a written result is registered under its *carrier*, not the walker's parent (`wrote`) | key changed to `pending.sourceParent` | both folded-outcome cases FAIL (the record after the call hangs off the abandoned outcome) |
| the document's own relations answer a child whose omitted parent resolved nothing (`startRecord`/`writtenAncestor`) | `sourceRecordParents` fill deleted | whole suite green — unpinned; the forward-parent case added for it then FAILs (`hangs off "middle", want … "the recorded parent"`) |
| the walk may step at least once (`writtenAncestor`) | bound set to `steps <= 0` | the same case FAILs (the child falls back to the entry before it) |
| a cycle the source recorded cannot keep the walk going (`writtenAncestor`) | bound deleted | the cycle case FAILs by its own deadline (`the conversion did not finish`) |
| the parentless record is counted, the identity-less read is not, the first record is not (`startRecord`) | each of the three conditions dropped in turn | the parentless case FAILs each time (`Count:0`/`2`, want `1`; `2`, want `0`) |
| a parent the read cannot resolve follows the entry before it and is counted (`parentFor`) | fallback's `t.unresolved++` deleted | the relations case FAILs (`Count:1`, want `2`) |

One line was **removed** by the sweep rather than pinned: `writtenAncestor`'s early exit for a record
naming itself (`next == sourceID`) survived its own mutation with the whole package green — the size bound
answers the same relation one step later, so the branch carried only the loop's iteration count, which no
caller can observe. The cycle case pins the termination rule through the bound, and the function's own
comment states why no early exit sits beside it. No case was removed, skipped or weakened; the rules the
sweep found unpinned were pinned by the new cases above, and the one branch that could not be is the line
removed here. `AGENTS.md` gained one clause (the sentence about the source's own relation now says an
omitted record keeps the place its own walk resolved), because that sentence is the file's
binding statement of the writer rule this pass fixed; `docs/conventions.md` gained
the same clause in the tree-rule bullet, `docs/plans/0005-pi-continuation.md`'s statement of the rule was
corrected to match, and Q-0005-15 records the default. The native halves were re-run in this pass too,
because the fix moves the parents a tree conversion writes: `pi-to-claude` prints
`RESULT: PASS — 12 check(s), none failed` (its conversion line still `carried 4 message(s), 1 call(s), 1
result(s)` with `27` loss categories) and `claude-to-pi` prints `RESULT: PASS — 21 check(s), none failed`
(`carried 6 message(s), 1 call(s), 1 result(s)`, `29` categories; the §2 block's 23-check run is the same
check list with the two real-turn checks) — the same verdicts, counts and installed-file roles the fourth
round recorded, which is the expected outcome of a fix whose shape (an omitted record standing under a
written tool result, with a later record naming it) no committed fixture reaches. No gate step, layout,
command or read-order line changed.

**The sixth review round's fix pass** answers that round's two findings and closes the leaf surface out
systematically. The first is the finding the writer half of the leaf rule had left open: the carried Pi
reader skips the tree entries the harness's own loader builds context from, so the read's last record is
not by itself the entry the source's file selects as its leaf — a session the user switched back to an
earlier branch with `/tree` ends with a `branch_summary` entry whose `parentId` names the branch now
continued, while the abandoned branch's own last message is still the read's last record. Measured here
with the installed harness (Pi 1.5.2, its own RPC loader against a copy of the new fixture whose recorded
working directory was rewritten to a scratch path, everything else byte-identical):

```text
$ (cd <scratch cwd> && PI_CODING_AGENT_DIR=<home> PI_CODING_AGENT_SESSION_DIR=<home>/sessions/--…-- \
     pi --mode rpc --session 01a1c337-6d1e-7f84-9b2c-4e5f6a7b8c90 --no-tools --offline <<'EOF'
   {"id":"state","type":"get_state"}
   {"id":"messages","type":"get_messages"}
   EOF)
state: sessionId=01a1c337-6d1e-7f84-9b2c-4e5f6a7b8c90  messageCount=4
messages: [(user, 'How long does the archive writer wait for its lock?'), assistant, assistant, (branchSummary, …)]
```

`internal/ccc/pi_source_leaf.go` is the fix. A conversion whose source is one of the Pi-family readers
(the carried Pi parser: `pi`, `omp`, `omo`, `stepcode`, `prime-agent`, `direct-api`; not `tau`, whose
reader replays its own leaf) now reads the *source file's own entries* with the load check's own entry
reader and walks its own leaf path, from the file's last entry back through `parentId`:

- the deepest entry on that path the read answered an identity for must be the record the read's last
  message stands for, or the conversion is refused (`ErrUnrepresentableLeaf`) — the refusal names the
  file's leaf, the record the path reaches, and the read's last record (or that the read answered none for
  it, or that the path reaches no record the read holds);
- every entry on the path the read did not answer an identity for must be a kind the harness's own loader
  keeps out of the conversation (`piEntryKindsOutsideTheContext`, each kind's reason named in the code: the
  pinned `docs/session-format.md`'s own "does not participate in LLM context" records, the settings and
  metadata entries, and the OMP title/usage-attribution entries the parser reads by name); a kind this
  build does not know — `branch_summary`, `custom_message`, `context_edit`, or anything a newer harness
  writes — is refused rather than assumed harmless;
- a file that records no identity anywhere (a Pi v1 session, which the reader accepts and names from the
  filename) answers its own order rather than a tree: the last entry whose kind is one the read carries ends
  the walk, and every record after it must be a context-free kind;
- a read with no records, and a file with nothing beyond its header, are answered without walking: there is
  no leaf that could be resumed wrongly.

Where no source file stands behind the read the entry is *stated*, not judged: an archive row, a
conversation with no file of its own, and a file that moved while it was being read (the same size match
`fillReadCoverage` states for the boundary) each add one warning to the run, quoted here as the tests
observe them:

```text
the entry the source's own loader selects as the conversation's leaf was not established: the conversion
read from archive rather than from a source file of its own, so the conversion writes the line of the read's
own last record and no source file answers which entry the harness's own loader would walk from

the entry the source's own loader selects as the conversation's leaf was not established: the source file
holds 2713 byte(s) while the read's own snapshot of this file recorded 2626, so the file moved while it was
being read and the entry its own bytes end with is not shown to be the entry this read's last record stands
for
```

The new committed fixture `pi/01a1c337-6d1e-7f84-9b2c-4e5f6a7b8c90.jsonl` is the branch switch in the
form the format document defines it (the summary's `parentId` is the entry the continued branch hangs off,
`fromId` the leaf left behind), generated by the same `internal/ccc/fixturegen` and pinned against it by
`TestFixturesMatchTheirGenerator`. The second finding is the load check's working directory: it is now read
exactly as the session recorded it — trimming decides only whether a directory was named at all — because
the writer, the installation directory, the session header and the printed resume command all keep the
recorded spelling, and a trimmed `os.Stat` asked this host about a different directory than the session
names.

Fourteen Go cases were added (no Python case, so the `selftest` count is unchanged) and no case was removed,
skipped or weakened: the fixture's own facts and the refusal it earns for all three targets; the two
context-free shapes that must still convert (a trailing rename record; a five-record chain above the read's
last record); the dropped-context-record refusal on its own (a switch whose summary hangs off the read's
last record, which the identity comparison alone would accept); the reached-another-branch refusal; the
reaches-no-record refusal; the identity-free file's two answers; the two no-walk answers; the scan's line
bound; the walk's cycle bound; the writer's answer for a read that answered no leaf identity; and the two
load-check cases for a recorded directory whose name ends with a space. The mutation sweep — revert the
line, run the case that names it, restore the line — covered the whole surface this pass touches; every
mutation failed its named case (the statement's own two reason fallbacks were pinned the same way, by
extending the one case that observes them), and the two that survived were closed rather than
explained:

| Rule (`internal/ccc/pi_source_leaf.go` unless noted) | Line disabled | Observed |
| --- | --- | --- |
| a context-free kind on the leaf path asks nothing of the read (`piEntryKindsOutsideTheContext`) | `"session_info"` made false | `TestContinueKeepsARecordOutsideTheContextOutOfTheLeafRefusal` FAIL |
| a context-kind record the read does not carry is refused | the allow-list condition made false | `TestContinueRefusesADroppedContextRecordOnTheLeafsPath` FAIL before that case existed for it (the branch-switch fixture's own refusal was reached by the identity rule one step later), which is why the case was added |
| the deepest record the read holds on the path must be the read's last record | `deepest != lastID` made false | `TestContinueRefusesALeafPathThatReachesAnotherBranch` FAIL |
| a leaf path with no record of the read is refused as that | the branch made false | `TestContinueRefusesALeafPathWithNoRecordTheReadHolds` FAIL |
| a read that answered no identity for its last record is refused as that | the branch made false | `TestPiSourceLeafRefusalRefusesAReadWithNoIdentityForItsLastRecord` FAIL |
| an identity-free file's last carried-kind record ends the walk | the accept made false | `TestPiSourceLeafRefusalAnswersAnIdentityFreeFileByOrder` FAIL |
| an identity-free file's context-kind record is refused | the allow-list condition made false | same case FAIL |
| a read with no records is answered without walking | the guard made false | `TestPiSourceLeafRefusalAnswersAReadWithNoRecordsAndAFileWithNoEntries` FAIL |
| a file with nothing beyond its header is answered | the guard made false | same case FAIL (index out of range on the empty entry slice) |
| a line beyond the scan's bound stops the scan with its error | `scanner.Err()` answer made nil | `TestPiScanConversationEntriesStopsAtItsLineBound` FAIL |
| the leaf is judged only where the source file stands behind the read | the gate made false | `TestContinueFromTheArchiveKeepsAPiSessionsBranch` FAIL (the assertion was strengthened to name the gate's own reason, because the size branch's warning covered the same phrase) |
| a file that moved while it was read is stated, not judged | the size comparison made false | `TestPiSourceLeafCheckStatesTheLeafItCouldNotEstablish` FAIL |
| the boundary pass's own reason travels into the statement | the detail branch made false | same case FAIL (the statement no longer quotes the reason the pass gave) |
| a conversation no boundary pass answered for gets the check's own sentence | that sentence replaced | same case FAIL |
| the rule's scope is the Pi-family readers | `"claude"` added to the family list | `TestContinueIntoPiConvertsInstallsAndReadsBack` FAIL |
| the walk's bound is the file's own entries (`for steps := 0; steps <= len(entries)`) | bound deleted | `TestPiSourceLeafRefusalEndsAWalkThroughARecordedCycle` FAIL by the case's own 30 s deadline |
| the bound is the file's own entries, not a depth constant | bound set to `steps <= 1` | `TestContinueWalksALeafPathLongerThanOneStep` FAIL |
| a recorded working directory is read as recorded (`internal/ccc/pi_verify.go`) | the trimmed spelling stat'd and returned (the pre-fix shape) | `TestPiLoaderCwd` FAIL (`… "/tmp/…/project " is not readable here … stat /tmp/…/project: no such file`) and `TestVerifyPiLoadRunsFromTheRecordedDirectorySpelling` FAIL (load `not-verified` with the same sentence) |
| trimming decides only whether a directory was named at all | the empty check made exact-match | `TestPiLoaderCwd` FAIL (the whitespace-only value answered as a missing path) |
| the writer's leaf refusal (`internal/ccc/route_shared.go`, unchanged) | the off-line refusal made false | the existing writer cases FAIL (`TestClaudeTranscriptRefusesASelectedLeafOffTheLine`, the Pi twin) |

Two rules the sweep could not fail were **removed or reported**: the walk's `visited` set was inert (every
entry's answer is idempotent and the file's-entry bound already ends a cycle, so removing it left the whole
suite green — the same reason `writtenAncestor` keeps no early exit, and the code's own comment now says
so), and `selectedLeafRefusal`'s two defensive guards — the empty-identity early answer and the loop's
fall-through for a record the read does not hold (`internal/ccc/route_shared.go`, unchanged by this pass) —
are behaviourally inert under mutation: an identity-free record is never marked inactive
(`inactiveSourceRecords`), so the loop's match on an empty identity always answers nil, and the writers only
ever hand the check an identity a read message carries. They are left as they were: the pass's finding is
the source-side half of the leaf rule, and pinning or deleting another function's defensive guards would
restyle code this round does not touch. Also unchanged: the run's report shape (the two disclosures travel
in `warnings`, the channel the boundary and fidelity caveats already use) and every gate step.

**The seventh review round's fix pass** answers that round's three findings, all on the Pi route's
verification surface, and closes out every rule it touches with an executed mutation (revert the line, run
the case that names it, restore the line, record the failure). The three findings:

- **An empty Pi read is judged by the file's own entries** (`internal/ccc/pi_source_leaf.go`). The check
  returned before reading the file when the read held no record, so a session header plus a
  `custom_message` — an entry the carried Pi parser skips while Pi's own loader builds a message from it —
  converted as an *empty* session with no counted omission. The file is now read first, and the empty read
  is answered by `piEmptyReadRefusal`: every entry on the path Pi's loader walks from (the file's last
  entry, then its ancestors) must be a kind the loader keeps out of the context, and a file that records no
  identity is asked by its own order (every entry it holds must be such a kind, because the file's order is
  the answer its shape takes). A header-only file still answers nil, a metadata-only file still converts,
  and a dropped record *off* the leaf path is still recorded history rather than context — the reading the
  walk already made for a read that holds records.
- **The load check's RPC arm reads its harness's answers whole** (`internal/ccc/pi_verify.go`). The arm
  captured stdout through the 1 MiB diagnostic bound (`harnessOutputMax`), and a `get_messages` answer
  carries a whole conversation's *contents*: a legitimate history larger than the bound was truncated, the
  incomplete JSON was dropped, and the check reported a successful native load as unverified — the default
  CLI invocation then exited nonzero *after* installation. The runner now takes the harness's stdout as a
  pipe and `readPiResponses` reads each line as it arrives into `piRPCResponse`, which names only the
  fields the check reads. (That line-at-a-time shape was handed to `encoding/json`'s `Decoder.Decode`,
  which buffers the whole value it is given before it unmarshals — measured at ~4× a 32 MiB reply, so the
  *reply* was held despite the bounded pieces `piProtocolLines` handed out; the eighth review round's fix
  pass replaced it with the token-by-token walk the code claims, and the claim now states its own residual
  bound — one JSON token at a time.) No diagnostic bound applies to the protocol document, and a line the
  decoder abandons is drained to its own line feed (`piProtocolLines`), so a malformed line cannot hide
  the answers after it, and a stream that ends mid-line answers the line it ended with.
- **The converted file's entry read reads every entry the writer can emit** (`internal/ccc/pi_verify.go`).
  The byte-backed read of a converted file bounded a line at 8 MiB and discarded the scan error, so a file
  holding a large record between the conversation and its leaf was judged by a prefix: the walk started
  from the wrong leaf and the native check compared the loader's correct answer against a shorter list.
  The read now answers every line of the file — the bytes are in hand, and a bound the writer did not
  promise is what leaves a caller judging a prefix — and the line-to-entry shape both scans read is one
  function (`piEntryFromLine`), with the file-backed scan's own bound (the leaf check's 64 MiB, the carried
  parser's `maxLineSize`) still stopping with the error it is.

The mutations, one per rule, each run against the case that names it and restored afterwards:

| Rule | Line disabled | Observed |
| --- | --- | --- |
| an empty read is still judged by the file's own entries (`internal/ccc/pi_source_leaf.go`) | the `piEmptyReadRefusal` branch replaced by the removed `if len(messages) == 0 { return nil }` | `TestPiSourceLeafRefusalRefusesADroppedRecordBehindAnEmptyRead` FAIL, both sub-cases (`the leaf check = <nil>, want ErrUnrepresentableLeaf` — the dropped `custom_message`, and the identity-free file) |
| the load arm's answers are read whole, not cut at the diagnostic bound (`internal/ccc/pi_verify.go`) | `readPiResponses(stdout)` replaced by a prefix of `harnessOutputMax` bytes re-parsed (the pre-fix shape) | `TestPiRPCCarriesAnAnswerLargerThanTheExcerptBound` FAIL — `piMessageDetail: the harness answered no get_messages response`, the sentence the finding predicted; the case drives the real `runPiRPC` against a stub process of this build, so the pipe, the incremental read and the parse are the production path |
| a line the decoder abandons is drained to its own line feed (`piProtocolLines.next`) | the drain loop removed | `TestPiProtocolLinesAnswersOneLineAtATime/a_reader_that_abandons_a_line_loses_only_that_line` FAIL — the abandoned line's remainder was answered as the next line |
| the stream's end answers "no further line" exactly (`piProtocolLines.next`) | (no mutation needed: the case was written first and failed against the first implementation) | `TestPiProtocolLinesAnswersOneLineAtATime/a_line_longer_than_the_buffer_arrives_in_pieces` FAIL — `next answered true past the last line` — so the end-of-stream peek was added |
| every entry of a converted file is read, past any record size (`piConversationEntries`) | the read replaced by the old `piScanConversationEntries(…, 8<<20)` whose error is discarded | `TestVerifyPiLoadReadsEveryEntryOfAConvertedFile` FAIL — `piModelVisibleRoles = [user], want [user assistant]`: the walk started from the entry before the large record |
| a stream the read could not finish is an error, not a shorter answer (`piProtocolLines.err`) | the answer replaced by `return nil` | `TestPiRPCReadsAndComparesTheLoadersAnswer` FAIL — `readPiResponses on a stream that failed = 2 response(s), <nil>; want the two answers and the read error` |
| a nonzero exit is reported with the harness's own stderr (`runPiRPC`) | the wait-error branch's condition disabled (`false && …`) | `TestPiRPCCarriesAnAnswerLargerThanTheExcerptBound` FAIL — `runPiRPC answered no error for a harness that exited nonzero` (the stub process exits 3 with `the stub harness failed on purpose`) |
| a harness that never exits is answered by the run's deadline (`runPiRPC`) | the deadline branch's condition disabled | same case FAIL — `the failure "signal: killed; stderr: " does not name the deadline it answered by`: without the branch, a killed process reads as a run failure rather than as the deadline it passed |

`python3 scripts/gate.py`, exit code 0, every step `PASS`:

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3602 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 251 tests in 5.815s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) … owned: 29 project-owned prefix(es); files seen by git: 5671 … declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 123 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The native half was re-derived against the installed harness, because the fix changes a measured consumer
(`scripts/pi-native-check.py` drives this build's load check). Without `--resume-prompt` — the arm this
pass changes is the offline load check, and the turn arm is untouched — so `21` checks rather than §2's
`23`, which is the same list plus the two real-turn checks; exit 0:

```console
PASS: the load check verified the installed session natively — the harness's own loader read the session and
answered about it (sessionId "01a1eb4b-992e-7796-b010-fcfc15f82022", messageCount 3); the same question with
an unknown session id answered "No session found matching". The check ran against a throw-away copy …
PASS: the loader's own message list is the file's model-visible history — loader ['compactionSummary',
'user', 'assistant'] vs file ['compactionSummary', 'user', 'assistant']
PASS: the unknown-id control arm reports no such session — No session found matching '01a1c335-dead-beef-0000-000000000000'
RESULT: PASS — 21 check(s), none failed
```

The carried counts, loss-category count and installed-file roles are the fourth through sixth rounds' own
(`6`/`1`/`1`, `29` categories), which is the expected outcome of a fix whose shape — how the check reads
its harness's answer, and which entries of a converted file it reads — no committed fixture's conversion
changes. The fix pass changed `internal/ccc/pi_source_leaf.go` (the empty-read guard and
`piEmptyReadRefusal`; both scans now share `piEntryFromLine`), `internal/ccc/pi_verify.go` (the response
types, `readPiResponses`/`piProtocolLines`, the pipe-reading runner, `piConversationEntries`/
`piEntryFromLine`, and the removed 8 MiB bound), `internal/ccc/route_shared.go` (the shared bound's
comment now says what it bounds: a turn's answer and an RPC arm's diagnostics, not a protocol answer), and
the cases: four new cases (`internal/ccc/pi_verify_test.go`) and the empty read's refusal case plus the
extended no-records case (`internal/ccc/continue_pi_test.go`), with the read-error answer, the runner's
failure and deadline arms and the byte-level reads added as assertions to the cases that already stand for
those surfaces, and the fake RPC harness now parsing its own canned protocol through the same reader. The
narrowed tree's `func Test` count moves `293 → 297`; the `selftest` count stays `251` (no Python case).
`AGENTS.md` is not edited: no layout, gate step, command, read order or agent rule changed, and this is
said here rather than by touching the file.

### The eighth review round's fix pass

Two findings, answered with behaviour that changed rather than acknowledged, and every rule each fix
touches closed out with an executed mutation (revert the line, run the case that names it, restore the
line, record the failure). The two findings:

- **Ordinary Pi system records made the source direction fail** (`internal/ccc/pi_source_leaf.go`). Pi
  writes a *system* record into every session it has run a turn in — "the first request of a session
  persists one with every prompt section and tool declaration, and later changes persist as system
  messages that patch `sections` by name and list `toolsAdded`/`toolsRemoved`" (the pinned
  `docs/session-format.md`, "message", shipped with 1.5.2) — and the sixth round's leaf rule refused any
  uncarried record on the file's leaf path whose kind the harness's loader builds context from. The
  inherited Pi parser keeps only the roles user, assistant and toolResult, so a session Pi itself had
  extended converted for no target: the recorded turn §2 measures (system record, user turn, answer)
  reproduced here natively, below, refused with `ErrUnrepresentableLeaf` before this pass. The record is
  now the one *accounted* omission: `piAccountedSourceOmission` names the role, the walk and the
  identity-free/empty-read branches of the check count the records they pass (`piSystemRecordLoss`), and
  the count travels into `conversion.losses[]` as the category `source_system_records` — absent when the
  source's file held none, because the count is the check's own reading of the file's entries and no
  target writer ever sees the record. What is omitted is the harness's prompt and tool-loadout state, not
  a turn: the conversation's own records are unaffected, a target harness resumes under its own prompt,
  and Pi rebuilds a session's prompt from its configuration when a session records no system message at
  all. A message record of any role this build does not name is still refused, and so is every other
  uncarried context record (`custom_message`, `branch_summary`, an unknown kind).
- **The RPC decoder still buffered the complete reply** (`internal/ccc/pi_verify.go`). The seventh round's
  line-at-a-time reader was handed to `encoding/json`'s `Decoder.Decode`, which reads the whole JSON value
  into its buffer before unmarshalling it: a `get_messages` answer still held the entire conversation, so
  the "scanned past rather than held" claim was untrue. The walk is now `encoding/json/jsontext` driven
  token by token (the shape this repository reads streamed JSON with,
  `internal/remotesync/manifest_decode.go`): the members the check names are taken, each `data.messages`
  entry's own `role` is taken, and every other value is consumed by token (`piSkipValue`), so no value the
  answer carries is materialised whole. Measured by the new case: an `8504402`-byte reply read four times
  allocates `556160` bytes (`0.065 ×` the reply) with the walk, and allocated `134671368` bytes
  (`15.8 ×` the reply, `~4 ×` per read) when the same case was run against `Decoder.Decode` — the
  mutation below. What remains held is one JSON token at a time: a text block Pi wrote as a single string
  arrives as one token, so the reply's size is no longer what the read's memory follows.

The mutations, one per rule, each run against the case that names it and restored afterwards (the file's
bytes were hashed before and after each run; every restore matched):

| Rule | Line disabled | Observed |
| --- | --- | --- |
| a Pi system record on the leaf path is accounted for, not refused (`internal/ccc/pi_source_leaf.go`) | `piAccountedSourceOmission` made to answer `false` | `TestContinueCarriesASessionPiItselfHasExtended` FAIL — `the leaf check on a session Pi itself has extended = …selects a leaf this conversion cannot write as the target's…` (the pre-fix refusal, reproduced natively below as well) |
| the counted omission travels into the report (`internal/ccc/continue.go`) | `result.Conversion.Losses = append(…)` replaced by `_ = leaf.Losses` | same case FAIL, all three targets: no `source_system_records` entry among the conversion's losses |
| only the named role is accounted for, and an unknown role is refused | predicate made `kind == piEntryMessage` (any role) | `TestContinueRefusesAMessageRoleItCannotAccountFor` FAIL — `the leaf check = <nil>, want ErrUnrepresentableLeaf` for a `message` record with role `developer` |
| a record the walk re-reaches through a cycle is counted once | the `counted` visited set removed | `TestPiSourceLeafRefusalCountsACycledSystemRecordOnce` FAIL — `the check counted 3 system record(s), want the literal 2` |
| the category is reported only where the source's own file held such a record | the `omitted > 0` guard removed | `TestContinueKeepsARecordOutsideTheContextOutOfTheLeafRefusal` FAIL — the conversion reports `{Category:source_system_records … Count:0 …}` for a file that holds none |
| an identity-free file's own order accounts too (`piSourceLeafRefusal`) | the accounted branch removed from that loop | `TestPiSourceLeafRefusalAnswersAnIdentityFreeFileByOrder` FAIL, the `system record after the last message` sub-case |
| an empty read accounts too (`piEmptyReadRefusal`) | the walk's accounted case made `false` | `TestPiSourceLeafRefusalAnswersAReadWithNoRecordsAndAFileWithNoEntries` FAIL — `…an empty session would be installed in place of the conversation the source's own loader builds…` and `counted 0 system record(s), want 1` |
| the reply is not held whole (`readPiResponses`) | the token walk replaced by `json.NewDecoder(lines).Decode(&response)` (the seventh round's shape) | `TestPiRPCReadsALargeAnswerWithoutHoldingIt` FAIL — `reading a 8504402 byte answer 4 time(s) allocated 134671368 byte(s), more than one reply's worth` |
| each message's own role is taken, and no other member is | the role member skipped instead of read | `TestPiRPCReadsAndComparesTheLoadersAnswer` FAIL — `the harness's loader read a  at position 1, while the session file's own history holds a user there` |
| an unnamed member's value is consumed as one value (`piSkipValue`) | the container walk removed (the first token dropped) | `TestPiRPCReadsANestedValueAsOneMember` FAIL — `the answer's command = "nonsense", want "get_state"`: a key inside the unnamed `meta` member was read as the answer's own |

The native half was re-derived against the installed harness. Both directions without a turn are
**unchanged** from the seventh round's re-run — `RESULT: PASS — 21 check(s), none failed` for
`claude-to-pi` and `RESULT: PASS — 12 check(s), none failed` for `pi-to-claude`, the same carried counts
(`6`/`4` messages, `1` call, `1` result) and the same loss-category counts (`29`/`27`) — which is the
expected outcome for a pass whose source-direction change needs a *native-produced* Pi file (the
committed fixture records no system record).

That shape was then produced and converted natively, in one run: `CCC_PI_NATIVE_PROVIDER=tencent-litellm
python3 scripts/pi-native-check.py --resume-prompt "Reply with exactly: PROBE-TURN-OK" --expect
PROBE-TURN-OK --keep` (exit 0, `RESULT: PASS — 23 check(s), none failed`, the loader's own answer
`['compactionSummary', 'user', 'assistant', 'system', 'user', 'assistant']`), which left Pi's own turn in
the installed session whose own entries now read `'role:system': 1`; the grown file was then converted
onward with the built binary,

```console
PI_DIR=<scratch>/pi-home /tmp/ccc-bin continue pi:01a1eb68-0856-7461-a067-14d2d3ac6bf3 --to claude \
  --target-home <scratch>/claude-home --target-binary …/claude --data-dir <scratch>/data
  conversation: 10 message(s), 4 user, 6 assistant, 1 tool call(s), 1 compaction boundary(ies)
  converted:    claude-code-transcript-jsonl, 5000 bytes: 7 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  omitted:      source_system_records ×1
  native load:  verified (the target harness's own loader …, plus an unknown-id control)
EXIT=0
```

— with the same command run against a binary built with the accounting predicate made `false` answering
the pre-fix state, exit 1: `ccc: the source's own loader selects a leaf this conversion cannot write as
the target's: the source's own file … selects the leaf message entry ddb56f13, and that leaf's own path
holds a message record (id c891221b, role system) which this read does not carry …`. The source file (13
lines, one `role:system` record) and the installed Claude transcript (11 lines) reconcile the omission:
the transcript holds **no** system record while holding the turn's own prompt and answer
(`Reply with exactly: PROBE-TURN-OK` / `PROBE-TURN-OK`).

`python3 scripts/gate.py`, exit code 0:

```console
gate: /home/azureuser/repos/call-ccc
  PASS     gofmt    3602 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 251 tests in 5.915s (rc=0)
  PASS     parity   … carried: 5553 file(s) identical to the pinned revision … owned: 29 project-owned prefix(es); files seen by git: 5671 …
  PASS     secrets  no leaks found in 123 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`AGENTS.md` is not edited: the pass changes no layout, gate step, command, read order or agent rule — the
leaf rule it implements is the one AGENTS.md already states, and the new category is an increment detail
recorded in the evidence record and `docs/decisions/OPEN-QUESTIONS.md` (Q-0005-17). No goal file is moved:
0005 is still the increment under review (`docs/goals/0005-pi-continuation.md`, "awaiting operator
acceptance"), so nothing was archived and the goals index is unchanged.

## 7. Limits and residual risks

- **The Pi → previously-supported-target native half did not run for the harness's first run here**
  (Claude Code was installed but not on `PATH`, and the harness had no flag to name it — a premise the
  fix pass measured to be false). It now runs: §3's block is that run, with `--claude PATH` naming the
  binary 0003's record pins, and the direction's load state recorded from the CLI's own report. A host
  with no Claude Code at all still gets `FAIL: the target harness (claude) is installed` and exit 2
  rather than a pass.
- **The report's `converted: … 0 turn(s)` counter is not filled for the Pi route.** Turn boundaries are a
  Codex concept; the field stays zero because this writer records none. It is not a claim about the
  session, but a reader comparing routes may read it as one.
- **Carried histories are under-counted by Pi's own context estimate.** Every converted assistant entry
  carries a zeroed `usage` object (Q-0005-01), which is what keeps the loader able to read the file at
  all; the target's own context-window estimate for the inherited turns is built from those numbers until
  the first resumed turn reports its own.
- **The Pi route writes no session-level provider binding and prints no model flag** (Q-0005-04): a
  resumed turn runs under whatever the caller's Pi configuration selects. A caller who needs a specific
  model passes Pi's own `--provider`/`--model` flags, which this route deliberately does not guess.
- **An unanswerable source relation is carried as the entry before it, on the tree-shaped targets.** Where
  a record names a parent the read does not hold, the written entry follows the entry before it and the
  run counts `unresolved_source_parents`; the count names the *relation* this conversion invented, not a
  dropped record — every record the reader produced is still written (Q-0005-07). The Codex target takes
  that decision's other branch, because its format has no parent relation at all: it writes the source's
  line and counts the records the source placed off it as `inactive_branch_records` (Q-0005-09), so that
  route *does* omit records by design where the two tree-shaped routes keep them — and its line walk's own
  accounting is counted under the same `unresolved_source_parents` category (Q-0005-12), so a record the
  walk kept on the line through a relation the read did not answer is not presented as the source's own.
- **A Pi session's own system records are omitted, and counted** (`source_system_records`, Q-0005-17). Pi
  writes one into every session it has run a turn in — its own prompt and tool loadout, not a turn — and
  the inherited reader does not carry the role, so no target entry holds it. The count is the source-side
  leaf check's own reading of the file's entries, so it is reported exactly where a source file stood
  behind the read: an archive-backed read cannot count these records (the row holds the read's own answer,
  which never carried them), and a file that moved while it was being read is the same case. The count
  covers the records on the file's own leaf path; a system record off that path is recorded history the
  source's own loader does not replay either.
- **The Pi → Claude Code route's branch handling is pinned offline, not by driving Claude Code against a
  branch-bearing transcript.** The case asserts the installed transcript's own `parentUuid` walk (the walk
  the loader follows, measured for the loader's link requirement) against the source file's, and the
  inherited reader's round trip; the native check that drives Claude Code's loader converts the *ordinary*
  fixture, and this record has no measured Claude loader answer for a transcript whose last record's path
  bypasses a compaction pair. The Pi → Codex half is the same shape: its context is pinned by the
  inherited reader's read-back and the file's own bytes, not by driving Codex's app server against the
  omitted-branch rollout.
- **A converted Pi assistant entry records the model and no provider** (Q-0005-08). The inherited read
  answers the model each source message recorded but no endpoint attribution, and the read's session-level
  provider names the harness that wrote the words, so the entry states the model and the report says a
  provider is not recorded (`model_provider_source` = `unrecorded`). A reader that needs the source
  endpoint has to ask the source side; this build does not guess it.
- **The parity oracle's Pi case covers the ordinary fixture only** (Q-0005-06). The branch/compaction
  fixture's reader projection is pinned by this tree's tests and by the conversation the conversion
  carries; it has no recorded upstream dump yet.
- **A source whose recorded working directory is absent on this machine converts but cannot be loaded
  here** (Q-0005-05): the load check reports `not-verified` naming the missing directory.
- **The projection count now includes the ordinary linear case.** A record that follows a tool result on a
  Pi-family read is counted (`unresolved_source_parents`) even where the result stands immediately before
  it, because the read answers a record under the call and one under the call's result identically
  (Q-0005-13 (b)): the count names the reading this conversion took rather than a defect in the source, and
  an ordinary linear Pi conversation therefore reports one counted place per result a later record follows
  — the committed Pi fixture's own conversions do (§3, §8). A reader that treats the count as "records at
  risk" reads it more strongly than the report claims: the entries and their order are the source's own in
  that shape.

## 8. Pinned numbers, as the runs printed them

| Number | Where it came from |
| --- | --- |
| `2051 bytes` / `sha256 2bd510821d4abf83…` | the Claude fixture's boundary in §1 (the same digest 0002–0004's records hold for that fixture) |
| `3051 bytes`, `6 message(s)`, `1 call(s)`, `1 result(s)`, `1 boundary(ies)` | the Pi document §1 converted (`pi-session-jsonl`), and the installed file's own size; `3091` before this pass dropped the two `"provider":"…"` fields a converted assistant entry no longer writes |
| `2773 bytes`, `4 message(s)`, `1 call(s)`, `1 result(s)` | the Pi → Claude Code transcript §3 converted — unchanged by this pass, because that source's records are one line and the tree rule writes the same parents the sequential chain did |
| `29 loss categories named` | the native run's own count of `conversion.losses[]` entries for the Pi target (§2); the earlier fix pass added `unresolved_source_parents`, which the writer reports with its count on every run |
| `27 loss categories named`, `4 message(s)`, `1 call(s)`, `1 result(s)` | the Pi → Claude Code conversion (§3); `26` before this pass, which gave the Claude writer the `unresolved_source_parents` category too |
| `inactive_branch_records = 3`, `carried.CompactionBoundaries = 0` | the Pi → Codex conversion of the branch fixture (§4): the three records the source's relations place off its line, omitted; the Codex-target loss list gained that category in that pass |
| `2231 bytes`, `4 message(s)`, `1 call(s)`, `1 result(s)`, `1 turn(s)`, `unresolved_source_parents ×1` | the Pi → Codex conversion of the *other* committed Pi fixture (`pi:01a1c335-…`, the same source §3 converts to Claude), measured in the fourth round: the rollout's own counts, with the one counted place the adjacent projection step takes (Q-0005-13 (b)); the earlier revision reported that category at `0` on this conversion |
| `unresolved_source_parents = 1` on the projection case and `= 2` on the walk's other two places | the Codex-target cases (`TestCodexRolloutKeepsAnActiveResultAcrossAnInterveningBranch`, `TestCodexRolloutCountsThePlacesItsWalkCouldNotAnswer`): the Codex-target loss list gained the category in the third review round's fix pass, with a detail naming the three relations it counts (Q-0005-12). The fourth round added the adjacent place to every route's count (Q-0005-13 (b)): the intervening case is still `1`, the adjacent one is now `1` where it was `0`, `TestPiSessionWritesTheSourcesOwnRelations` asserts `2` (the parent outside the read and the record naming the call its result stands directly before), and the committed linear Pi fixture converts with `×1` on both outbound routes (§3; the row below pins the same conversion's bytes) |
| `3067 → 32999 bytes` | the installed Pi session's growth across one real turn (§2). What the check asserts is that the file grew, not a byte count, so a later run measures different ends: the runs this record has quoted measured `3123 → 33055` / `3123 → 33042` (the first revision), `3107 → 33039` (the earlier fix pass) and `3067 → 32986`, then `3067 → 32999`, in the third review round's fix pass |
| `23 checks, none failed` / `12 checks, none failed` | the two native runs' verdicts (§2, §3); both were re-run for this pass. §3's run has one check more than the `11` the first run reported, because the earlier fix pass records the non-Pi target's load state as a check of its own. The fourth round's re-runs (no turn) report `21 check(s), none failed` for `claude-to-pi` — the §2 block's run is the same check list plus the two real-turn checks — and `12 check(s), none failed` for `pi-to-claude`, both with the loader's own verdicts unchanged; the fifth round's re-runs (no turn) print the same `21`/`12` lines, with the same carried counts (`6`/`4` messages, `1` call, `1` result) and loss-category counts (`29`/`27`) as the fourth round's; the seventh round's re-run (no turn, `claude-to-pi`) prints `21 check(s), none failed` at exit 0, with the loader's own verdict unchanged (`messageCount 3`, `[compactionSummary, user, assistant]`, the unknown-id control arm answering `No session found matching`) |
| the branch fixture's loader answers | `messageCount 3` / `[user, assistant, assistant]` for the *source* file and for its conversion, against `messageCount 4` / `[compactionSummary, user, assistant, assistant]` for the conversion the pre-fix writer produced (§4) |
| the branch-switch fixture's loader answer | `messageCount 4` / `[user, assistant, assistant, branchSummary]` from Pi 1.5.2's own RPC loader against a copy of `pi/01a1c337-…` (the sixth review round's fix pass, §6): the file's leaf is the switch's `branch_summary`, and the read's own last record -- the abandoned branch's last turn -- is not on the path the loader walks |
| `556160 byte(s)` vs `134671368 byte(s)` | the RPC read's own measurement (`TestPiRPCReadsALargeAnswerWithoutHoldingIt`, §6): an `8504402`-byte `get_messages` reply read four times allocates `556160` bytes (`0.065 ×` the reply) with the eighth round's token-by-token walk, and allocated `134671368` bytes (`15.8 ×` the reply, `≈4 ×` per read) when the same case ran with `encoding/json`'s `Decoder.Decode` — the mutation that fails it |
| `source_system_records ×1`, `13 → 11` lines, exit `0` (was `1`) | the eighth round's native probe (§6): a Pi session a real turn extended (13 lines, one `role:system` record) converted to Claude Code with the omission counted and Claude's own loader verified; the same command against a binary with the accounting predicate disabled answered exit 1 with the refusal naming the system record |
| `251 selftest cases`, `302 Go test functions`, `5553 carried files`, `29 owned prefixes`, `3602 gofmt-clean files`, `5671 files seen by git`, `123 owned file(s)` scanned | the gate run (§6) and the current case count: the selftest count went `246 → 248` with the third review round's two Python cases (`seed_pi_home`'s nested-credential cases) and `248 → 251` with the fourth round's three (the endpoint-policy cases at both levels, the credential-free counter-arm, and the run-level refusal), while the narrowed tree's `func Test` count went `269 → 270` with the third round's one new Go case, `270 → 274` with the fourth round's four (one per route: the Pi tree writer, the Claude tree writer, the Codex line walk and the pipeline's archive read) and `274 → 279` with the fifth round's five Go cases (the omission/projection composition on the Pi writer and on the Claude writer, `endRecord`'s two-effects case, the omitted record with no resolved place, and the recorded-cycle case), the selftest count unchanged because the fifth round added no Python case; the sixth round's fix pass took it `279 → 293` with fourteen Go cases (the branch-switch fixture's facts and its refusal for all three targets, the two context-free shapes that must still convert, the dropped-context-record refusal and the two other refusals the walk answers, the identity-free file's two answers, the two no-walk answers, the scan's line bound, the walk's cycle bound, the writer's answer for a read with no leaf identity, and the two load-check cases for a recorded directory whose name ends with a space) and left the selftest count at `251` (no Python case) and the owned-file count at `123` (the new source-side file and the new committed fixture); the seventh round's fix pass took it `293 → 297` with four Go cases (the empty read's dropped-record refusal, the runner's answer larger than the excerpt bound against a stub process, the converted file read past a large entry, and the protocol line reader), and the eighth round's fix pass took it `297 → 302` with five Go cases (the native-shape conversion and the unaccounted-role refusal in `internal/ccc/continue_pi_test.go`; the counted cycle, the reply whose read is not held whole, and the unnamed nested member in `internal/ccc/pi_verify_test.go`), both passes printing `251` selftest cases, `3602` gofmt-clean files and `5671` files seen by git for this tree. The owned-file count includes the ignored build/test artifacts the working tree holds (the check's `bin/` and the two `__pycache__` directories), which the scan stages with the owned files; a fresh checkout counts fewer, and the pre-fix tree's own gate run (a detached worktree of `711e256`) printed `114` |
