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
  converted:    pi-session-jsonl, 3091 bytes: 6 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  carried-with-caveat: unrecorded_token_usage ×4
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: message_model_attribution ×2
  carried-with-caveat: pre_compaction_history ×4
  omitted:      malformed_source_lines ×1
  installed:    /tmp/ev5/pi-home/sessions/--home-user-code-example-api--/2026-05-04T09-15-00Z_01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1
  working dir:  /home/user/code/example-api
  resume:       cd /home/user/code/example-api && PI_CODING_AGENT_DIR=/tmp/ev5/pi-home ccc-no-such-harness --session 01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1
  non-interactive resume: cd /home/user/code/example-api && PI_CODING_AGENT_DIR=/tmp/ev5/pi-home ccc-no-such-harness --session 01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1 -- '<prompt>'
  cli_version:  1.5.2 (writer-pin)
  structure:    verified (the inherited AgentsView Pi reader over the installed session)
  native load:  not-verified (the target harness's own loader (pi --mode rpc, get_state/get_messages, with no model variables in its environment, plus an unknown-id control))
                ccc-no-such-harness is not on PATH, so the installed session's native load could not be verified (ccc-no-such-harness is not on PATH)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume (pi -p --session))
                not requested (--verify load)
  warning:      the target harness was not found on PATH: the conversation is converted and installed at /tmp/ev5/pi-home/sessions/--home-user-code-example-api--/2026-05-04T09-15-00Z_01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1.jsonl, and its native load is unverified
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The same run without a target binary present exits 0 (the native half could not run, which is reported,
not failed), and the installed session's bytes are what its record says:

```console
{"type":"session","version":3,"id":"01a1e9f2-d50d-755d-a9ac-1d2a2d227bd1","timestamp":"2026-05-04T09-15-00.000Z","cwd":"/home/user/code/example-api"}
{"type":"message","id":"…","parentId":null,"timestamp":"2026-05-04T09-15-00.000Z","message":{"role":"user","content":"Summarize the retry policy in internal/retry/policy.go.","timestamp":1777886100000}}
…
{"type":"message","id":"…","parentId":"…","timestamp":"2026-05-04T09-15-09.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Four attempts with exponential backoff, capped at 30s between attempts."}],"provider":"claude","model":"claude-sonnet-4-5","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0,"totalFromProvider":false}},"stopReason":"stop","timestamp":1777886109000}}
```

(The installed file's own bytes are printed in the native run below's `entries {'compaction': 1, …}`
line; the excerpt above is the same document's head and tail with the entry ids elided.)

## 2. The Pi target verified by Pi's own loader, natively

`CCC_PI_NATIVE_PROVIDER=tencent-litellm python3 scripts/pi-native-check.py --resume-prompt "Reply with
exactly: PROBE-TURN-OK" --expect PROBE-TURN-OK --json`, exit code 0 — the run prints two verdicts beyond
its own checks: the loader's answer, and the turn's own output:

```console
PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 6 message(s), 1 call(s), 1 result(s); loss categories named: 29
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr No session found matching '53d26c35-5ceb-4e4a-b391-3f0c925eee38'
PASS: the installed session exists at the reported path — sessions/--tmp-ccc-pi-native-check-456iff4z-workspace--/2026-05-04T09-15-00Z_01a1ea30-ddec-7896-9b8b-9b7e738df4aa.jsonl [hard-link]
PASS: the installed file is a Pi session, installed where its working directory spells — sessions/--tmp-ccc-pi-native-check-456iff4z-workspace--/2026-05-04T09-15-00Z_01a1ea30-ddec-7896-9b8b-9b7e738df4aa.jsonl; entries {'compaction': 1, 'message': 10, 'role:assistant': 5, 'role:system': 1, 'role:toolResult': 1, 'role:user': 3, 'session': 1, 'thinking_level_change': 1}
PASS: the installed file carries the source's own first prompt — 'Summarize the retry policy in internal/retry/policy.go.'
PASS: the installed file holds the source's tool call and its result — call block(s) ['Read'], toolResult record(s) 1
PASS: the installed file's assistant records carry a usage object — 5 assistant message(s), 0 without a usage object
PASS: the load check verified the installed session natively — the harness's own loader read the session and answered about it (sessionId "01a1ea30-ddec-7896-9b8b-9b7e738df4aa", messageCount 3); the same question with an unknown session id answered "No session found matching". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-pi-load-4104021790/sessions/--tmp-ccc-pi-native-check-456iff4z-workspace--/2026-05-04T09-15-00Z_01a1ea30-ddec-78 …[the rest of this output was dropped]
PASS: the harness's own loader answers about the installed session — sessionId '01a1ea30-ddec-7896-9b8b-9b7e738df4aa', messageCount 6
PASS: the loader's own message list is the file's model-visible history — loader ['compactionSummary', 'user', 'assistant', 'system', 'user', 'assistant'] vs file ['compactionSummary', 'user', 'assistant', 'system', 'user', 'assistant']
PASS: the unknown-id control arm reports no such session — No session found matching '01a1c335-dead-beef-0000-000000000000'
PASS: the load check ran against a copy and left the installed file alone
PASS: the resume check verified one real turn through the printed invocation — the harness answered from the installed session in one turn and wrote its own records into it (the session file went from 3107 to 33039 bytes)
```

the turn's own output (the target's answer, as the harness printed it):
    PROBE-TURN-OK

```console
PASS: the turn's answer contains 'PROBE-TURN-OK' — PROBE-TURN-OK
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-456iff4z/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-pi-native-check-456iff4z/pi-home /home/azureuser/.local/share/mise/installs/pi/latest/pi/pi --session 01a1ea30-ddec-7896- …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-456iff4z/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-pi-native-check-456iff4z/pi-home /home/azureuser/.local/share/mise/installs/pi/latest/pi/pi --session 01a1ea30-ddec-7896- …[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — 01a1ea30-ddec-7896-9b8b-9b7e738df4aa is new among the session id(s) the home's own header lines record
RESULT: PASS — 23 check(s), none failed
```

What the two arms answer, in Pi's own words rather than this record's: `get_state` for the installed
session — `{"sessionId":"01a1ea30-ddec-7896-9b8b-9b7e738df4aa","messageCount":3, …}` (the count is the
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
session and wrote its own records into it (`3107 → 33039` bytes), and the answer is the block above.

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
  installed:    /tmp/ev5/claude-home/projects/-home-user-code-example-api/6d326207-d7e8-42f1-aa5d-a3540725e418.jsonl [hard-link]
  source file:  unchanged (sha256 7395c3b0309a09501d9041b36ad46f471c513584e220940eb325174bc111a2e6, taken before and after the conversion)
  target id:    6d326207-d7e8-42f1-aa5d-a3540725e418
  working dir:  /home/user/code/example-api
  resume:       cd /home/user/code/example-api && CLAUDE_CONFIG_DIR=/tmp/ev5/claude-home ccc-no-such-harness --resume 6d326207-d7e8-42f1-aa5d-a3540725e418
  non-interactive resume: cd /home/user/code/example-api && CLAUDE_CONFIG_DIR=/tmp/ev5/claude-home ccc-no-such-harness -p --output-format json --resume 6d326207-d7e8-42f1-aa5d-a3540725e418 -- '<prompt>'
  cli_version:  2.1.326 (writer-pin)
  structure:    verified (the inherited AgentsView Claude reader over the installed session)
  native load:  not-verified (the target harness's own loader (claude --bare --resume, with no model credentials in its environment, plus an unknown-id control))
                ccc-no-such-harness is not on PATH, so the installed session's native load could not be verified (ccc-no-such-harness is not on PATH)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume (claude -p --resume))
                not requested (--verify load)
  warning:      the target harness was not found on PATH: the conversation is converted and installed at /tmp/ev5/claude-home/projects/-home-user-code-example-api/6d326207-d7e8-42f1-aa5d-a3540725e418.jsonl, and its native load is unverified
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

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
PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 26
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr No conversation found with session ID: b79b21f7-1ff1-4dec-8457-09c91501ac17
PASS: the installed session exists at the reported path — projects/-tmp-ccc-pi-native-check-kb11tuuq-workspace/afc702a9-efc3-478e-9e52-7c1004448a1f.jsonl [hard-link]
PASS: the load check verified the installed session natively — the harness's loader resolved the installed session and answered about it (subtype "success", ter…[the rest of this output was dropped]
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-kb11tuuq/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-pi-native-check-kb11tuuq/claude-home /home/azureuser/.local/share/ccc-native/cla…[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-kb11tuuq/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-pi-native-check-kb11tuuq/claude-home /home/azureuser/.local/share/ccc-native/cla…[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
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

What the conversion does **not** do, and says so: it never synthesises a `branch_summary` entry (the
source records no summary for the abandoned path, and writing one would invent it), it never presents the
abandoned turn as an active one, and it never drops the branch either — the records the source wrote stay
in the file. `sidechain_messages` is the category for records the *reader* marks as a subagent's turn;
those are omitted and counted, and `unresolved_source_parents` counts the relations the read could not
answer.

## 5. What travels, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| a Claude Code conversation with a tool call, its result and a compaction boundary | converted into a Pi session whose file the native run's own counts describe: `entries {'compaction': 1, 'message': 10, 'role:assistant': 5, 'role:system': 1, 'role:toolResult': 1, 'role:user': 3, 'session': 1, 'thinking_level_change': 1}` — one header, one compaction record, the messages, and the harness's own `thinking_level_change` record beside them (§2) |
| the tool call and its result | `PASS: the installed file holds the source's tool call and its result — call block(s) ['Read'], toolResult record(s) 1` (§2); the inherited Pi reader agrees on the round trip (`structure: verified` in §1 and §3) |
| the source's own words | `PASS: the installed file carries the source's own first prompt — 'Summarize the retry policy in internal/retry/policy.go.'` (§2) |
| a Pi conversation with a tool call and its result, as a **source** | converted into a Claude Code transcript with `1 tool call(s), 1 tool result(s)` carried (§3), read back `verified` by the inherited Claude Code reader |
| the source's recorded provider and model | carried verbatim on each converted Pi assistant entry (`"provider":"claude","model":"claude-sonnet-4-5"` in §1's excerpt) and reported as the `message_model_attribution` caveat, while the session binds no model of its own (`model_provider_binding` in the format limits) |
| the converted history as the **answer** of a real turn in the installed Pi session | the recorded turn's answer holds the words the check asked for — `PROBE-TURN-OK` (§2); the same run reports the turn's own growth beside it (3107 → 33039 bytes, §2, and §8 explains why a re-run's number differs) |
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
gate: go=… targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3601 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 237 tests in 7.928s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 29 project-owned prefix(es); files seen by git: 5669 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 114 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

No gate step was added, skipped, bounded or narrowed: the new tests live in `internal/ccc` and `cmd/ccc`,
which `scripts/gate-targets.txt` already builds and runs, the new Python case module lives in
`scripts/tests/`, which the `selftest` step already discovers, and the carry's `parity` step is unchanged
because the pin, the manifest's carried records and every carried byte are untouched (the manifest gained
one `owned` line, which is this repository's own file list, not a carried path).

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
| one existing case lost a `t.Skip` and now runs on every host: `TestPiLoadCheckRefusesWhenTheRecordedWorkingDirectoryIsNotHere` owns the missing directory it asks about and answers the harness lookup through the check's own seam | `internal/ccc/continue_pi_test.go` |
| one existing case gained the third provider's row: the no-line-feed rule now holds for a Pi source too, with the boundary's offset claim asserted per provider (the Pi provider's declarations claim none, and its boundary says so) | `internal/ccc/continue_growth_test.go` (the `pi-complete-turn.jsonl` fragment is now consumed, which is what it was committed for) |
| the native harness's own cases: the walk, the full invocation word vector, the session identity read from headers, the usage rule, the guarded provider read, the named skip for a host with no provider, the `--claude` flag, and the build output's location | `scripts/tests/test_pi_native_check.py` (15 → 30 cases) |

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

**Goal 0004 moved to the archive in this increment.** It was still sitting in `docs/goals/` with a
"Implemented on `feat/0004-repeatable-evolving-session-handoffs`" status while `origin/main` already
carried its merge (`8a8265a feat(0004): Repeat continuation as source history grows (#6)`), which is the
condition `AGENTS.md` states for archiving. The file moved unchanged to
`docs/goals/archive/0004-repeatable-evolving-session-handoffs.md`, its own `Status` line was left as the
record of its acceptance, and the three links that named it from `docs/plans/` and `docs/evidence/` were
retargeted at the archive path.

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
- **An unanswerable source relation is carried as the entry before it.** Where a record names a parent the
  read does not hold, the written entry follows the entry before it and the run counts
  `unresolved_source_parents`; the count names the *relation* this conversion invented, not a dropped
  record — every record the reader produced is still written. Reversal: write only the source's active
  path and count the off-path records as omitted (Q-0005-07).
- **The parity oracle's Pi case covers the ordinary fixture only** (Q-0005-06). The branch/compaction
  fixture's reader projection is pinned by this tree's tests and by the conversation the conversion
  carries; it has no recorded upstream dump yet.
- **A source whose recorded working directory is absent on this machine converts but cannot be loaded
  here** (Q-0005-05): the load check reports `not-verified` naming the missing directory.

## 8. Pinned numbers, as the runs printed them

| Number | Where it came from |
| --- | --- |
| `2051 bytes` / `sha256 2bd510821d4abf83…` | the Claude fixture's boundary in §1 (the same digest 0002–0004's records hold for that fixture) |
| `3091 bytes`, `6 message(s)`, `1 call(s)`, `1 result(s)`, `1 boundary(ies)` | the Pi document §1 converted (`pi-session-jsonl`), and the installed file's own size |
| `29 loss categories named` | the native run's own count of `conversion.losses[]` entries for the Pi target (§2); the fix pass added `unresolved_source_parents`, which the writer reports with its count on every run |
| `26 loss categories named`, `4 message(s)`, `1 call(s)`, `1 result(s)` | the Pi → Claude Code conversion (§3); unchanged by the fix pass, because the category it added belongs to the Pi *target*'s report |
| `3107 → 33039 bytes` | the installed Pi session's growth across one real turn (§2). What the check asserts is that the file grew, not a byte count, so a later run of the same check measures a different end: the run §2 quotes measured it as printed here, and the block it replaced had measured `3123 → 33055` (an earlier re-run of that one `3123 → 33042`) |
| `23 checks, none failed` / `12 checks, none failed` | the two native runs' verdicts (§2, §3); both were re-run for the fix pass this record names. §3's run has one check more than the `11` the first run reported, because the fix pass records the non-Pi target's load state as a check of its own |
| the branch fixture's loader answers | `messageCount 3` / `[user, assistant, assistant]` for the *source* file and for its conversion, against `messageCount 4` / `[compactionSummary, user, assistant, assistant]` for the conversion the pre-fix writer produced (§4) |
| `237 selftest cases`, `5553 carried files`, `29 owned prefixes`, `3601 gofmt-clean files`, `114 owned file(s)` scanned | the gate run (§6); the selftest count was `222` before the fix pass's cases |
