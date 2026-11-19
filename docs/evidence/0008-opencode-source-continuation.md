# 0008 — Continue OpenCode history in other harnesses: evidence

Contract: [`docs/goals/0008-opencode-source-continuation.md`](../goals/0008-opencode-source-continuation.md)
(this increment leaves it in `docs/goals/` with its row marked *Implemented on
`feat/0008-opencode-source-continuation`; awaiting operator acceptance*, per `AGENTS.md`: the goal moves to
`docs/goals/archive/` in the increment that merges it).
Plan: [`docs/plans/0008-opencode-source-continuation.md`](../plans/0008-opencode-source-continuation.md).
Citations: [`0008-citations.tsv`](0008-citations.tsv) (the machine-checked half of §6).
Harness: OpenCode `2.2.5` (`opencode --version` prints `opencode v2.2.5`), Codex `0.170.0`
(`codex-cli 0.170.0`), the pinned AgentsView carry `83195c8a6861b8c74a4b6cb87512817fed6f8584` (named by
every report's own `upstream_revision` and by `ccc inspect`'s `carried:` line). Every command below ran on this host (`linux x64`), in this repository, on
branch `feat/0008-opencode-source-continuation`. Output is quoted verbatim; `…[the rest of this output was
dropped]` is the harness's own elision of a bounded detail line, and nothing else was edited.

## 0. What was measured before anything was written

The plan's §2 holds the tables; this is what they rest on, all measured against the installed harness and
the inherited reader rather than read from a document.

| Question | Measured answer |
| --- | --- |
| where an OpenCode session lives | one SQLite store per data directory (`<data dir>/opencode.db`, `session_v2` + `session_message` rows); the data directory is `$XDG_DATA_HOME/opencode`, and the provider's configured root for this agent is the directory holding the store (`OPENCODE_DIR`, config key `opencode_dirs`, default `~/.local/share/opencode`) |
| how a reference resolves | `OPencode_DIR=<the store's directory>` + `opencode:<session id>` resolves through the inherited provider's own `FindSource` (`findOpenCodeFormatSourceFile` → `OpenCodeSQLiteSessionExists` → the virtual path `<db>#<id>`); the provider's own virtual-path spelling resolves the same session as a path reference (both read the same conversation) |
| what one read answers | the store's `user` message text; an assistant record's `content` parts rendered the way `internal/parser`'s `loadOpenCodeV2Messages` renders them — `text` parts joined with `\n`, each `reasoning` part as `[Thinking]\n<trace>\n[/Thinking]`; `ContentRendersThinking` true, `Thinking` **empty** and no dialogue projection for **every** record this reader produces; a `completed` tool part answered as **two** result events — the part's own `time.ran` as a textless `started` event beside the terminal one carrying `time.completed` and the part's `state.content` text |
| what the read does *not* hold | the store's `idle` record (a session control the reader's own switch skips): the fixture's store holds 6 records and the read holds 5, and nothing is counted for the sixth because the read never held it |
| the committed fixture's import round trip | `opencode session import <document> --directory <existing dir> --standalone` answers `Imported session: <id>` and preserves the document's own id; the store's own reader returns it member for member, rewriting exactly three members — `info.projectID` (recomputed from the directory), `info.location.directory` (set to `--directory`) and `info.time.updated` (the import instant). A text part's `time` member, which the importer drops, is therefore not in the committed fixture at all (the fixture carries the shape the harness itself produces) |
| the fixture store against the harness's own | a store `internal/ccc/opencodefixture` builds from the committed document reads **message for message identical** to the store the harness's own importer writes from it (compared by a throw-away probe: same messages, same order, same tool call, inputs and results, byte for byte — the compact fixture is what makes the tool input's spelling the store's own) |
| the store is never opened for writing | reading it and converting it leave the *fixture* store's bytes and its directory listing unchanged (`sha256` of `opencode.db` identical before and after an `inspect` and a `continue`; no `-wal`/`-shm` created). The **harness's** own store does grow `-wal`/`-shm` beside the file (its own WAL handling), which is why the route claims the store's *records* and never the database file's bytes |
| the boundary a member can state | `source.boundary.state` is `not-taken`, naming the member and the container, with no digest, no size and no record offsets claimed; `installation.source_check_state` is `not-verified` with the same detail |

## 1. An OpenCode session → Codex, with the harness's own importer (goal's clauses 1 and 4)

`scripts/opencode-native-check.py --direction opencode-to-codex` (deliberately **not** a gate step: it
needs the OpenCode binary to write the source store and Codex to load the target — and, for a turn, model
access). This run plants the committed fixture document through the harness's own importer, converts
through the CLI, and reads the target's own records; it runs no turn (see §2 for that):

```
$ python3 scripts/opencode-native-check.py --direction opencode-to-codex \
    --opencode /home/azureuser/.npm-global/bin/opencode
repo:        /home/azureuser/repos/call-ccc
direction:   opencode-to-codex (opencode:ses_0008opencodefixture00000001 → codex)
source:      internal/ccc/testdata/fixtures/opencode/ses_0008opencodefixture00000001.json (planted through the harness's own importer; the store is materialised by the harness, not committed)
workspace:   /tmp/ccc-opencode-source-check-nrxuilq8/workspace (the directory the session is planted under: the harness's own importer refuses one that does not exist)
data root:   /tmp/ccc-opencode-source-check-nrxuilq8/opencode-data (a scratch one this run made); the harness's own data directory is /tmp/ccc-opencode-source-check-nrxuilq8/opencode-data/opencode, its store opencode.db
target:      /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex (codex-cli 0.170.0); its home /tmp/ccc-opencode-source-check-nrxuilq8/codex-home
opencode:    /home/azureuser/.npm-global/bin/opencode (opencode v2.2.5)

PASS: ccc builds with the flags upstream ships
PASS: the source session is planted through the harness's own importer — the harness answered `Imported session: ses_0008opencodefixture00000001` (exit 0)
PASS: the harness's own reader answers the planted session's document — exit 0; id 'ses_0008opencodefixture00000001', 6 message(s)
PASS: the store holds the fixture document's own conversation — 6 message(s), 1 tool call(s) and their recorded results compared with the fixture's own bytes
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 31
PASS: ccc continue exits 0 — exit 0; stderr WARNING: proceeding, even though we could not create PATH aliases: CODEX_HOME points to "/tmp/ccc-opencode-source-check-nrxuilq8/codex-home", but that path does not exist
Error: CODEX_HOME points to "/tmp/ccc-opencode-source-check-nrxuilq8/codex-home", but that path does not exist
WARNING: proceedin …[the rest of this output was dropped]
PASS: the installation names the target's own mechanism and the installed rollout — installed [hard-link] -> '/tmp/ccc-opencode-source-check-nrxuilq8/codex-home/sessions/2026/05/08/rollout-2026-05-08T09-15-00-01a1f35b-5ab6-716a-84a1-6d13a59c05ea.jsonl'
PASS: the structural read-back verified the installed session
PASS: the target harness's own loader verified the installed session — the harness's loader answered with this session's own id and path; opening a session makes the harness record its own settings in it (measured: one thread_settings_applied record), and no session other than the one it opened is touched
PASS: the conversion reports the container-member boundary it does not claim a file for — not-taken: the source read resolved member ses_0008opencodefixture00000001 inside the container /tmp/ccc-opencode-source-check-nrxuilq8/opencode-data/opencode/opencode.db, and no file holds this member's own bytes to compare
PASS: the installed rollout holds the fixture's own prompts, tool call, result text and texts — 14 record(s); 2 prompt(s), 1 call(s) and their recorded results compared with the fixture's own bytes
PASS: the printed resume lines name this run's own home and session — CODEX_HOME=/tmp/ccc-opencode-source-check-nrxuilq8/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex resume 01a1f35b-5ab6-716a-84a1-6d13a59c05ea | CODEX_HOME=/tmp/ccc-opencode-source-check-nrxuilq8/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/code …[the rest of this output was dropped]
PASS: the unknown-id control arm reports no such session, naming the id — exit 1; Session not found: ses_unknown_control_0000000000
SKIP: a real turn was requested (--resume-prompt) — not requested: this run verifies the offline half (the planting, the conversion, the target's own loader, the installed rollout's records and the printed invocation) and runs no model turn
PASS: the source session's own records are unchanged — exit 0; its document is byte-identical before and after (the database file's own bytes are not claimed: this harness's writes move them with its WAL handling, which is why the boundary names no digest for a member of a store)
PASS: the planted session is removed through the harness's own session delete — Session ses_0008opencodefixture00000001 deleted
PASS: the removed session no longer resolves — exit 1; Session not found: ses_0008opencodefixture00000001
scratch removed (/tmp/ccc-opencode-source-check-nrxuilq8)
RESULT: PASS — 18 check(s), none failed
```

The three states the core design keeps apart are separate lines of that run: the *conversion* (the report's
`converted` and its counted losses), the *installation* (the target's own mechanism, `[hard-link]`, and the
rollout's path), and the target-native **load** (`the target harness's own loader verified the installed
session` — Codex's own app server, `thread/resume`, answering with the session's own id and path, offline).
The comparison the goal's second clause asks for is the rollout line: the *installed rollout's own records*
are read against the *fixture document's own bytes*, over every member a conversion writes for it, so a
writer that mangled a name, an argument or a result cannot pass by agreeing with itself. (The line's own
name is `the installed rollout holds what a conversion writes of the fixture's own prompts, tool call,
result text and texts` since the second review round — §6's own section is where the two readings and the
shape that made them necessary are measured; the block above is the run as it stood on the head it was
recorded on.)

## 2. The same direction with a real turn requested (the harness's own answer on this host)

```
$ python3 scripts/opencode-native-check.py --direction opencode-to-codex \
    --opencode /home/azureuser/.npm-global/bin/opencode \
    --resume-prompt "Reply with exactly: OPENCODE-SOURCE-OK" --expect OPENCODE-SOURCE-OK
repo:        /home/azureuser/repos/call-ccc
direction:   opencode-to-codex (opencode:ses_0008opencodefixture00000001 → codex)
source:      internal/ccc/testdata/fixtures/opencode/ses_0008opencodefixture00000001.json (planted through the harness's own importer; the store is materialised by the harness, not committed)
workspace:   /tmp/ccc-opencode-source-check-thoih22z/workspace (the directory the session is planted under: the harness's own importer refuses one that does not exist)
data root:   /tmp/ccc-opencode-source-check-thoih22z/opencode-data (a scratch one this run made); the harness's own data directory is /tmp/ccc-opencode-source-check-thoih22z/opencode-data/opencode, its store opencode.db
target:      /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex (codex-cli 0.170.0); its home /tmp/ccc-opencode-source-check-thoih22z/codex-home
opencode:    /home/azureuser/.npm-global/bin/opencode (opencode v2.2.5)

PASS: ccc builds with the flags upstream ships
PASS: the source session is planted through the harness's own importer — the harness answered `Imported session: ses_0008opencodefixture00000001` (exit 0)
PASS: the harness's own reader answers the planted session's document — exit 0; id 'ses_0008opencodefixture00000001', 6 message(s)
PASS: the store holds the fixture document's own conversation — 6 message(s), 1 tool call(s) and their recorded results compared with the fixture's own bytes
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 31
PASS: ccc continue exits 0 — exit 0; stderr WARNING: proceeding, even though we could not create PATH aliases: CODEX_HOME points to "/tmp/ccc-opencode-source-check-thoih22z/codex-home", but that path does not exist
Error: CODEX_HOME points to "/tmp/ccc-opencode-source-check-thoih22z/codex-home", but that path does not exist
WARNING: proceedin …[the rest of this output was dropped]
PASS: the installation names the target's own mechanism and the installed rollout — installed [hard-link] -> '/tmp/ccc-opencode-source-check-thoih22z/codex-home/sessions/2026/05/08/rollout-2026-05-08T09-15-00-01a1f34c-2da0-776d-8798-91ab664e66bd.jsonl'
PASS: the structural read-back verified the installed session
PASS: the target harness's own loader verified the installed session — the harness's loader answered with this session's own id and path; opening a session makes the harness record its own settings in it (measured: one thread_settings_applied record), and no session other than the one it opened is touched
PASS: the conversion reports the container-member boundary it does not claim a file for — not-taken: the source read resolved member ses_0008opencodefixture00000001 inside the container /tmp/ccc-opencode-source-check-thoih22z/opencode-data/opencode/opencode.db, and no file holds this member's own bytes to compare
PASS: the installed rollout holds the fixture's own prompts, tool call, result text and texts — 14 record(s); 2 prompt(s), 1 call(s) and their recorded results compared with the fixture's own bytes
PASS: the printed resume lines name this run's own home and session — CODEX_HOME=/tmp/ccc-opencode-source-check-thoih22z/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex resume 01a1f34c-2da0-776d-8798-91ab664e66bd | CODEX_HOME=/tmp/ccc-opencode-source-check-thoih22z/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/code …[the rest of this output was dropped]
PASS: the unknown-id control arm reports no such session, naming the id — exit 1; Session not found: ses_unknown_control_0000000000
FAIL: a real turn continues from the installed session — exit 1, session 3804 -> 109187 bytes

the turn's own output (the target's answer, as the harness printed it):
    WARNING: proceeding, even though we could not create PATH aliases: Refusing to create helper binaries under temporary dir "/tmp" (codex_home: AbsolutePathBuf("/tmp/ccc-opencode-source-check-thoih22z/codex-home"))
    OpenAI Codex v0.170.0
    --------
    workdir: /tmp/ccc-opencode-source-check-thoih22z/workspace
    model: gpt-6.2-sol
    provider: openai
    approval: never
    sandbox: read-only
    reasoning effort: none
    reasoning summaries: none
    session id: 01a1f34c-2da0-776d-8798-91ab664e66bd
    --------
    user
    Reply with exactly: OPENCODE-SOURCE-OK
    warning: Codex's Linux sandbox uses bubblewrap and needs access to create user namespaces.
    2026-11-19T10:13:39.305202Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    2026-11-19T10:13:39.339878Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    2026-11-19T10:13:39.576357Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    ERROR: Reconnecting... 2/5
    2026-11-19T10:13:39.980518Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    ERROR: Reconnecting... 3/5
    2026-11-19T10:13:40.864025Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    ERROR: Reconnecting... 4/5
    2026-11-19T10:13:42.599935Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    ERROR: Reconnecting... 5/5
    2026-11-19T10:13:45.730902Z ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown
    warning: Falling back from WebSockets to HTTPS transport. unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: wss://api.openai.com/v1/responses, cf-ray: a5ce71b0cf125871-ICN
    ERROR: Reconnecting... 1/5
    ERROR: Reconnecting... 2/5
    ERROR: Reconnecting... 3/5
    ERROR: Reconnecting... 4/5
    ERROR: Reconnecting... 5/5
    ERROR: unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses, cf-ray: a5ce71e31baeea20-ICN, request id: req_6c1b4d25874b417c9e4e58f51b706298
    ERROR: unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses, cf-ray: a5ce71e31baeea20-ICN, request id: req_6c1b4d25874b417c9e4e58f51b706298
FAIL: the turn's answer contains 'OPENCODE-SOURCE-OK'
PASS: the turn was appended to the installed session — 14 record(s) before the turn, 28 after; the records the conversion wrote are unchanged; the prompt is in the records it now holds
PASS: the source session's own records are unchanged — exit 0; its document is byte-identical before and after (the database file's own bytes are not claimed: this harness's writes move them with its WAL handling, which is why the boundary names no digest for a member of a store)
PASS: the planted session is removed through the harness's own session delete — Session ses_0008opencodefixture00000001 deleted
PASS: the removed session no longer resolves — exit 1; Session not found: ses_0008opencodefixture00000001
scratch removed (/tmp/ccc-opencode-source-check-thoih22z)
RESULT: FAIL — 2 check(s) failed: a real turn continues from the installed session, the turn's answer contains 'OPENCODE-SOURCE-OK'
```

The turn is *the check's own*, run through the report's own `exec_resume_arguments` read against the
harness this run probed and the session it converted (plus the check's isolation flags), and the answer
above is the harness's own bytes: a Codex turn in a **scratch** home answers `401 Unauthorized` because its
credential is the caller's own store and this build never copies a credential (the same boundary
`scripts/codex-native-check.py`'s own turn shows — §4). Three readings hold beside the refused turn and
pass: the conversion's own records survive the turn, the rollout's own records grew (14 → 28) and hold the
prompt, and the source store's own records are byte-identical before and after. A requested turn is
recorded as **failed** with the harness's own words — never a pass, never a skip — which is Q-0008-05's
default.

## 3. The OpenCode → Claude Code half, offline

Both accepted targets go through the same pipeline; this host has no Claude Code binary, so its native
half is 0003's accepted check (on a host with the binary) and what is re-derived here is the conversion,
the installation and the inherited Claude reader's structural read-back. The store was planted through the
harness's own importer for this run too:

```
$ cd /tmp/oc8/ev/ws && XDG_DATA_HOME=/tmp/oc8/ev/data opencode session import \
    /home/azureuser/repos/call-ccc/internal/ccc/testdata/fixtures/opencode/ses_0008opencodefixture00000001.json \
    --directory /tmp/oc8/ev/ws --standalone
Imported session: ses_0008opencodefixture00000001

$ OPENCODE_DIR=/tmp/oc8/ev/data/opencode /home/azureuser/repos/call-ccc/bin/ccc \
    inspect opencode:ses_0008opencodefixture00000001
opencode:ses_0008opencodefixture00000001
  agent:        OpenCode
  session id:   opencode:ses_0008opencodefixture00000001
  source:       /tmp/oc8/ev/data/opencode/opencode.db#ses_0008opencodefixture00000001
  project:      ws
  working dir:  /tmp/oc8/ev/ws
  conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)
  tools:        read
  recorded:     2026-05-08T09:15:00Z .. 2026-05-08T09:15:10Z
  first prompt: Which file holds the retry cap, and what is the cap?
  archive:      none at /home/azureuser/.agentsview/sessions.db
  warning:      no archive at /home/azureuser/.agentsview/sessions.db; reporting source facts only
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)

$ OPENCODE_DIR=/tmp/oc8/ev/data/opencode …/bin/ccc continue opencode:ses_0008opencodefixture00000001 \
    --to claude --target-home /tmp/oc8/ev/claudehome --json      # exit 0; the report's own fields:
  conversion:            "state": "converted", "format": "claude-code-transcript-jsonl",
                         "carried": 4 message(s), 2 user, 2 assistant, 1 tool call, 1 tool result, 0 turns
  installation:          "state": "installed", "mechanism": "hard-link",
                         "path": /tmp/oc8/ev/claudehome/projects/-tmp-oc8-ev-ws/ee012233-2325-410b-98ae-98313714ed53.jsonl
                         "source_check_state": "not-verified" (the container-member detail of §0)
  verification.structure "state": "verified", "method": "the inherited AgentsView Claude reader over the
                         installed session", "read_back": 6 message(s), 3 user, 3 assistant, 1 call, 1 result
  verification.load      "state": "not-verified", "detail": "claude is not on PATH, so the installed
                         session's native load could not be verified (claude is not on PATH)"
  resume_command         cd /tmp/oc8/ev/ws && CLAUDE_CONFIG_DIR=/tmp/oc8/ev/claudehome claude --resume ee012233-2325-410b-98ae-98313714ed53
```

The installed transcript's own records (read from the file the report names), which is the tool-bearing
semantic comparison for this half read at the file level:

```
user      | Which file holds the retry cap, and what is the cap?
assistant | [{"type": "tool_use", "id": "call_0008opencodefixture000b01", "name": "read", \
             "input": {"path": "/home/user/code/example-api/internal/retry/config.go"}}]
user      | [{"type": "tool_result", "tool_use_id": "call_0008opencodefixture000b01", \
             "content": "Read file /home/user/code/example-api/internal/retry/config.go, lines 38-42\n38: // policy.go\n40: const maxAttempts = 4\n41: const maxBackoffSeconds = 30\n42: "}]
assistant | [{"type": "text", "text": "[Thinking]\nThe grep hit is maxBackoffSeconds = 30 in internal/retry/config.go.\n[/Thinking]\nThe cap is `maxBackoffSeconds = 30` in internal/retry/config.go."}]
user      | And how many attempts does the policy allow?
assistant | [{"type": "text", "text": "Four attempts, and thirty seconds of backoff between them."}]
```

Two facts of that half are the reader's own and travel disclosed rather than silently: the reasoning-bearing
record's text is the reader's **rendering** (`[Thinking]…[/Thinking]` + the record's own text) counted
`unprojected_dialogue` (×4, carried-with-caveat, on both targets), and the tool part's textless `started`
event is left out of the target and counted (`empty_tool_results` ×1) while the terminal event's text is
what both targets hold (one output per call — pinned by
`internal/ccc/continue_opencode_source_test.go`'s `TestOpenCodeSourceToolEvidenceReachesEachTarget`).

## 4. The accepted routes still verify (goal's clause 3)

The OpenCode-**target** route, offline (the same script's other direction; 0007's own installation-by-the-
importer, planting, listing and removal discipline):

```
$ python3 scripts/opencode-native-check.py --opencode /home/azureuser/.npm-global/bin/opencode
…
RESULT: PASS — 32 check(s), none failed          # exit 0
```

The Codex route's own check (0002/0003/0004's suite), with its requested turn:

```
$ python3 scripts/codex-native-check.py --resume-prompt "Reply with exactly: CODEX-ROUTE-OK"
…
26 check(s) passed, 1 failed, 1 skipped          # exit 1
```

Its one failure is the same credential boundary §2 records, quoted from its own run:

```
FAIL: a real turn continues from the installed session — exit 1, session 3928 -> 164173 bytes

the turn's own output (the target's answer, as the harness printed it):
    WARNING: proceeding, even though we could not create PATH aliases: Refusing to create helper binaries under temporary dir "/tmp" (codex_home: AbsolutePathBuf("/tmp/ccc-native-check-4jaymw3u/codex-home"))
    OpenAI Codex v0.170.0
    --------
    workdir: /home/azureuser/repos/call-ccc
    …
```
(the summary line above it is that run's: `FAIL: a real turn continues from the installed session — exit 1,
session 3928 -> 164173 bytes`, the same 401 boundary §2 records.)

and its load/view half passed in the same run (`the target harness's own loader opens the installed
session`, `the target's own view of the second session lists the appended turn`, `the first session's own
view does not hold the appended turn`), so the accepted offline half is unchanged. The accepted
Claude-target route's own suites run inside the gate's `test` step (§6), and `parity` passes on an
unchanged carry — the goal's "source-side AgentsView packages and SQLite schema are unchanged".

## 5. The source → target comparison, clause by clause

| Source evidence (the fixture document's own bytes) | Target side (the installed Codex rollout's own records, §1) | Reported as |
| --- | --- | --- |
| the two user prompts (`Which file holds the retry cap…`, `And how many attempts…`) | the rollout's own `input_text` messages hold both, byte for byte | `carried.messages` 4, `user_messages` 2 (§1); pinned by `TestOpenCodeSourceToolEvidenceReachesEachTarget` |
| the `read` call: id `call_0008opencodefixture000b01`, name `read`, recorded input `{"path":"/home/user/code/example-api/internal/retry/config.go"}` | the rollout's `function_call` (same id, same name) and its `arguments` string, compared as the **value** (a rollout writes the arguments as a JSON string; the store keeps the parsed value) | `tool_calls` 1; `tool_name_vocabulary` ×1 (the source's own tool name) |
| the terminal tool result's text (`Read file …42: `) | the rollout's `function_call_output` for that call, byte for byte — and exactly **one** output for the call, because the textless `started` event is not written as a second one | `tool_results` 1, `empty_tool_results` ×1 |
| the assistant record with the persisted reasoning trace, and the record that has only text | the reader's own rendering of each, byte for byte (the trace's own text between the reader's markers, then the record's text) | `unprojected_dialogue` ×4 (carried-with-caveat); nothing removed from the text by inference |
| the store's `idle` session control | nothing (the read never held it as a message) | named as a format limit — the pair has no member for a session control — not counted as a loss |
| the source's own records around the run | the harness's own reader answers a byte-identical document before and after (§1's line); the *database file's* bytes are never claimed | `source.boundary` `not-taken` + the container's own detail; `source_check_state: not-verified` |
| the directed turn (`--resume-prompt`, §2) | the rollout's own records grew and hold the prompt, the conversion's records are unchanged, and the harness's own **answer** is its refusal — `unexpected status 401 Unauthorized: Missing bearer or basic authentication in header` — because a scratch Codex home holds no credential | a **failed** check naming the harness's own words, never a pass and never a skip (Q-0008-05) |

## 6. Gate, tests and bookkeeping

The gate is `python3 scripts/gate.py`, run alone (never beside a native check: both are memory-heavy on
this host), and it must print `GATE: PASS`:

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 417 tests in 7.477s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5719 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

(The run printed `GATE: PASS` with exit 0 on the tree the other sections quote; each step's own counts are
above, and the closed `opencode/` entry of the carried tree is what makes this increment's fixture the only
new one under `fixtures/`.)

The Go suites this increment adds to:
`go test -count=1 -tags fts5 ./internal/ccc/ ./cmd/ccc/...` — the new cases in
`internal/ccc/continue_opencode_source_test.go` (the read's own answers; both reference forms; the pipeline
for both targets; the container-member boundary; the tool evidence in each target's own records; the store
never opened for writing) and `cmd/ccc/opencode_source_route_test.go` (the report shape, the inspection,
the not-found failure). The fixture is pinned against its generator
(`internal/ccc/fixtures_test.go`'s `TestFixturesMatchTheirGenerator`), and the python cases for the new
direction live in `scripts/tests/test_opencode_native_check.py` — 95 cases, run by the `selftest` step,
among them the expectation's own rules (an open call's blanked input, a non-`read` part's structured
content, a part carrying a file block, an empty text block a target holds verbatim, a recorded result's
error message, a user record's attachments, an errored result and a textless one) and the refusals (a
document that names no session, a default fixture whose identity drifted, and a document carrying a message
kind the comparison does not model). The obligation-citation manifest is `docs/evidence/0008-citations.tsv`
(§ below).

`python3 scripts/obligation-citations.py`:

```
$ python3 scripts/obligation-citations.py
docs/evidence/0002-citations.tsv: 36 row(s) verified
docs/evidence/0007-citations.tsv: 37 row(s) verified
docs/evidence/0008-citations.tsv: 13 row(s) verified
citations: 86 row(s) across 3 file(s) verified
citations: PASS
```

Documentation and bookkeeping this increment carries: `AGENTS.md` **moves with this round** — the route
table's source side, the rules an agent must follow for an OpenCode *source* (the store's rows, the two
reference forms, the read's own answers and the counted caveat, the progress/terminal event pair, the
session controls, the container-member boundary, the committed fixture document, the check's second
direction) — so its OpenCode-source bullet, the fixture sentence and the native-check sentence were written
in the same increment. Also carried: `docs/conventions.md` (the layout block, the fixture rule's OpenCode
branch, the container-member rule, the check's two directions); `docs/harness.md` (the new direction, its
flags and its verdicts); `README.md` (the capability sentence, the `OpenCode → Codex` example, the source
roots, the OpenCode-source paragraph, `SourceReader`'s note); `docs/goals/README.md` (goal **0007** archived
with its row — its implementation is merged onto `main` as `e68f18a` — every link to it moved, and 0008's
row marked *Implemented … awaiting operator acceptance*); goal `0007-opencode-target-continuation.md`
moved to `docs/goals/archive/`; and `docs/decisions/OPEN-QUESTIONS.md` (Q-0008-01 … Q-0008-06).

### The review round's findings (`54f7643` + working tree), fixed and re-measured

The review of `79595de..54f7643` found two items and this round fixes both; the tree these measurements ran
on is `54f7643` plus this round's working tree. The reviewer's own gate run (on the pre-fix tree) printed
the same steps with `selftest` at 417 tests; this tree's run is quoted below.

**REV-1 — the expectation builder did not read every rule from the document (P2).**
`scripts/opencode-native-check.py`'s `opencode_tool_state_text` dropped empty `text` blocks and held neither
a part's recorded `state.error.message` nor a user record's `files` rendering, while the carried reader
appends *every* `type=="text"` block verbatim (`internal/parser/opencode_v2.go`:371-372), appends the error
message on its errored branch (`:399-403`, whose trigger is an `error` status *or* a `shell`/`bash` part
with a nonzero recorded exit), renders a user record's attachments (`:257-271`: `[Attachment: <name>]`
with the name falling back to the MIME type, a `text/*` payload's decoded text under it, each joined with a
newline where the text already holds something) and keeps a record only where its content survives its own
`strings.TrimSpace` filter (`:330`). That last rule's trimming is Go's set, not Python's: measured over every
code point, Go's space set is Python's `str.isspace` set minus the information separators `\x1c`-`\x1f`, so
the builder trims through an explicit mirror (`opencode_trimmed`/`opencode_is_space`, whose Latin-1 set is
spelled out for exactly those four separators).

Measured cross-language on this host, without the OpenCode binary (absent here — the native direction exits
2 naming it, so neither this round nor the reviewer's could drive the installed harness on this machine): a
throw-away Go probe (removed afterwards; `go test` over `internal/ccc`, `PROBE_OUT` naming a scratch
directory) planted one document through `internal/ccc/opencodefixture` and read it through
`NewSourceReader`. The document's `read` part records `state.content =
[{"type":"text","text":""},{"type":"text","text":"read 12 lines"}]`, its completed `bash` part records
`metadata.exit = 1` with `error.message = "boom"`, its user records carry four attachments (a `text/plain`
payload `aGVsZA==`, a non-text MIME, invalid base64, and an empty payload with no name and no MIME), a
whitespace-only text, and a text that is the single separator `\u001c`. The carried reader answered:

```
reader:  {"user_texts": ["look at these\n[Attachment: notes.txt]\nheld\n[Attachment: application/pdf]\n[Attachment: broken.txt]\n[Attachment: ]", "\u001c"], "call_names": ["read", "bash"], "call_inputs": ["{\"path\":\"/x\"}", "{\"command\":\"false\"}"], "call_results": ["\nread 12 lines", "exit 1\nboom"], "assistant_texts": []}
```

`git show HEAD:scripts/opencode-native-check.py`'s builder, on the same document, held
`call_results = ["read 12 lines", "exit 1"]` and `user_texts = ["look at these", "   ", "\u001c"]` — no
attachments rendering, and the whitespace-only record kept as a member the reader had dropped, while the
reader's separator record was one the pre-fix filter and the reader agreed on — and `evidence_differences`
answered the reviewer's measured false FAIL for both members (the comparison's other three members agreed;
the two lines below are the check's own words, joined here with ` || `):

```
the user text(s) are ["look at these\n[Attachment: notes.txt]\nheld\n[Attachment: application/pdf]\n[Attachment: broken.txt]\n[Attachment: ]", "\u001c"], the source recorded ["look at these", "   ", "\u001c"] || the recorded tool result(s) are ["\nread 12 lines", "exit 1\nboom"], the source recorded ["read 12 lines", "exit 1"]
```

The fixed builder's members are byte-identical to the reader's (`evidence_differences` → `[]`), and it
refuses — exit 2, before the harness is asked to import anything — a document carrying a `shell` message,
the one kind the reader answers as both a user record (the command) and tool evidence (the call and its
recorded output) while the comparison holds no member for either; comparing such a document silently as a
subset is what the refusal replaces. Six new cases in `scripts/tests/test_opencode_native_check.py` pin the
rules (the empty text block, the error message with its nonzero-exit arm, the attachments rendering, the
whitespace-only record the reader's own filter drops beside the separator one it keeps, and the refusal at
the comparison and at the whole-run level); run against the pre-fix script in a scratch copy of `scripts/`,
every one of the five comparison cases fails — the empty-block case differing exactly as the reviewer
measured (`['read 12 lines']` vs `['\nread 12 lines']`) — and the run-level refusal case errors instead
(its refusal never fires and the run reaches for the harness that is not installed: `FileNotFoundError:
'opencode'`).

**REV-2 — the two reference forms could not be told apart (P3, test strength).**
`TestOpenCodeSourceResolvesByIdentifierAndByItsOwnPath` now plants a **second** session in the same store
(the committed fixture's own document with its own identity and a single question of its own,
`internal/ccc/continue_opencode_source_test.go`'s `openCodeSourceSecondDocument`) and asserts that the
provider's virtual-path spelling resolves the member it names — that member's own session id and its own
single question — and that a member the store does not hold answers a not-found refusal. Measured with a
throw-away `go test -overlay` mutation of the carried provider (the path form forced to answer the
fixture's session whatever member it was asked about; the overlay and its inputs were scratch files,
removed afterwards): the **old** case (with its one-session store, at `HEAD`) passes under the mutation,
while the new case fails on both new assertions — `the second member's messages = …` holding the fixture's
five-message conversation, and `reading a member the store does not hold = <nil>`. That is the
distinguishing input the review said was missing.

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 423 tests in 7.706s (rc=0)
  PASS     parity   (the step's detail is §6's own rendering: 5553 carried file(s) identical to the pinned
                    revision, 27 declared deviations + 1 generated input, 4 symlinks by link target; git saw
                    5718 file(s) in this tree, 1 fewer than §6's run — that counter includes untracked
                    non-ignored files, which the reviewer observed too)
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The `selftest` step's count moved 417 → 423 with the six new cases (the module's own count moved 89 → 95);
the `citations` rows are unchanged — no cited test was renamed, added or removed — and the Go suites are the
same ones §6 names, with the reference-form case's assertions strengthened in place. `AGENTS.md` does not
move with this round: the fix changes no layout, gate step, command, read order or rule an agent follows —
it makes the check's comparison read the reader's own rules (and refuse what it cannot read) and strengthens
one test's input.

### The CI repair on the head `6fe0f14`, re-measured

The hosted `gate (windows-latest)` check failed on that head (run `44515743638`, job `135531169595`; the
watch log's own line is `real fail: gate (windows-latest) (2m11s)`, and the ubuntu leg passed) for a real
reason: `FAIL test`, `FAIL selftest REQUIRED but was FAIL: scripts/tests -> rc=1` (`Ran 423 tests in
13.342s`, `FAILED (failures=4)`), and `FAIL citations REQUIRED but was FAIL:
docs/evidence/0008-citations.tsv:42: rule D4: test TestContinueFromOpenCodeWritesTheEstablishedReport in
./cmd/ccc did not pass`. The four python failures close as two broken cases plus the two `HostCase` cases
they break: `scripts/tests/test_gate.py`'s `HostCase` pair re-runs the *whole* suite in a child and asserts
the child broke nothing, so each broken case of that child fails both nested-run cases as well — and the
last two failure blocks are the module's own copies, one whose message ends with the run it drove
(`RESULT: FAIL — 1 check(s) failed: the printed resume lines name this run's own home and session`) and one
the job quotes verbatim:

```
--- FAIL: TestInspectOpenCodeSourceNamesTheStoreAndTheConversation (0.57s)
    opencode_source_route_test.go:189: the inspection does not hold "source:       C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\TestInspectOpenCodeSourceNamesTheStoreAndTheConversation1442433199\\002\\opencode\\opencode.db#ses_0008opencodefixture00000001":
          source:       C:\Users\runneradmin\AppData\Local\Temp\TestInspectOpenCodeSourceNamesTheStoreAndTheConversation1442433199\002\opencode\opencode.db#ses_0008opencodefixture00000001
…
FAIL: test_the_right_line_is_read_as_the_invocation (test_opencode_native_check.SourceInvocationReading.test_the_right_line_is_read_as_the_invocation)
    AssertionError: False is not true
…
Ran 423 tests in 13.342s
FAILED (failures=4)
```

**CI-1 — a case compared two spellings of one path (Go, `cmd/ccc`; the citation row's failure above).**
`openCodeSourceStore` returned `filepath.Join(t.TempDir(), "opencode")` raw, while the read answers the root
*resolved*: the configuration resolves every configured session source before discovery reads it
(`internal/parser.ResolveProviderRoot` → `pathutil.ResolveAbsolute`: `filepath.Abs` + `filepath.EvalSymlinks`,
tolerating a non-existent tail), and Go's Windows `EvalSymlinks` normalizes every component through
`FindFirstFile`, so a short-named temp path is answered by its long name. Both of that file's
inspection-shaped cases spelled their expectation from the raw name. Reproduced on this host the moment the
two spellings are made to differ, without touching the shipped code (`TMPDIR` pointing at a symlinked
directory, so `t.TempDir()` spells the link and the read answers the target):

| Rule | Mutation | Observed |
| --- | --- | --- |
| a case's expectation is the spelling the read answers | `openCodeSourceStore`'s `return resolved` → `_ = resolved; return root` (executed under `TMPDIR=<a symlink to /tmp>`, restored afterwards — `git diff` back to this round's change) | both cases FAIL: `source path = /tmp/TestContinueFromOpenCodeWritesTheEstablishedReport…/opencode.db#…, want … "/tmp/ccc-link/TestContinueFromOpenCodeWritesTheEstablishedReport…/opencode.db#…"` and `the inspection does not hold "source:       /tmp/ccc-link/…"` — the CI's own two-spellings shape |
| a rendered resume line is read in the renderer's quoting | the loop `for _, want := range []string{"CODEX_HOME='" + targetHome + "'", …}` → the pre-round `strings.Contains(resume, "CODEX_HOME="+targetHome)`, executed and restored | `TestContinueFromOpenCodeWritesTheEstablishedReport` FAIL: `resume command = "CODEX_HOME='/tmp/…/003/codex home' ccc-no-such-target-harness resume 01a1f3a4-…", want the target home "/tmp/…/003/codex home" and the session id` — the quoted value the renderer writes cannot match the unquoted substring (on the Windows leg the temp path itself holds backslashes, so that arm fires there too) |

The repair is in `cmd/ccc/opencode_source_route_test.go`: `openCodeSourceStore` resolves the throw-away root
with the configuration's own rule (`pathutil.ResolveAbsolute`, the function `ResolveProviderRoot` calls) and
documents why, so no expectation is spelled from a name the read never answers; the target home is
`filepath.Join(t.TempDir(), "codex home")` — the shell-unsafe spelling `cmd/ccc/claude_route_test.go`,
`pi_route_test.go` and `omp_route_test.go` already use — and the printed line is asserted to hold
`CODEX_HOME='<that home>'`, the harness word and the session id, as `cmd/ccc/main_test.go`'s Codex cases do.
No assertion was weakened: the same facts are asserted, in the spelling the report really carries.

**CI-2 — two python cases spelled a Windows-unreadable line themselves (the fourth failure block and the
`HostCase` pair).** `SourceInvocationReading` hand-spelled `CODEX_HOME=/home/x` and handed it
`Path("/home/x")`: with `Path` being `WindowsPath` that value spells `\home\x`, so the line's own word
(`CODEX_HOME=/home/x`) is not the invocation's — `False is not true`, exactly the block the job quotes, in
the case the file's header already says must be built with the renderer's quoting and driven under both path
flavours. `SourceDirectionRun`'s stub built its report's two printed Codex lines the same way, unquoted, from
the case's real `codex-home`; the flow then read them as a shell does — a backslash an unquoted word hands to
the shell as an escape — and recorded the one failed check the third failure block names.

| Rule | Mutation | Observed |
| --- | --- | --- |
| the line a case hands the reading is the renderer's own spelling | the repaired `test_the_right_line_is_read_as_the_invocation` body → the pre-round hand-spelled `Path("/home/x")` call, executed and restored (the file hashes back to `af56d315…`) | under `Path` replaced by the Windows flavour (a scratch harness: POSIX-rooted literals mapped to `PureWindowsPath`), that case FAILs with the job's own `AssertionError: False is not true` |
| a stubbed report's printed line is rendered, not hand-spelled | `conversion_report_of`'s two `codex_invocation(...)` calls → the pre-round `f"CODEX_HOME={home} …"` strings, executed and restored | the driven run records `the printed resume lines name this run's own home and session` FAIL — the very check the job's third failure block quotes — beside the four checks that fail only because the scratch harness's backslash path cannot be opened on this host (a Windows host opens it) |

Both repaired cases now drive `PATH_FLAVOURS` and spell their lines through
`scripts/tests/test_opencode_native_check.py`'s `codex_invocation` (the quoting `shellQuote` applies), so the
Windows arm is pinned on this host rather than left to the job: the case that failed in CI exercises
`PureWindowsPath` on every run, including this one.

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 423 tests in 7.740s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5718 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored byte…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

(The `citations` row that failed in CI verifies here — `TestContinueFromOpenCodeWritesTheEstablishedReport`
is the test this repair fixed, and the manifest's 13 rows are unchanged.) The `selftest` count stays 423
before and after: the repair changes two cases' inputs, it adds none, and the module's own count stays 95.
`AGENTS.md` **does** move with this round: the repair is conformance to a rule that file only half stated,
so its harness-tests bullet now names the two measured shapes (a reported path is asserted through the rule
the CLI resolves it by; a rendered line is asserted in the renderer's quoting, with a shell-unsafe home),
`docs/conventions.md`'s platform paragraph states them for the Go suites and `scripts/tests/` alike, and
`docs/harness.md`'s Platform boundary section — its Windows-answers bullets for a backslash a POSIX shell
reads as syntax, and for a path the platform that reads it spells — names this head's two instances beside
the earlier ones (`37c0a10`, `1a476e0`, `524a20f`, `4cd0567`). No
goal file moves: 0008 is still the increment on its branch, awaiting acceptance. No new OPEN-QUESTIONS
entry: both shapes are rules the docs already bound in spirit (a case must hold on every platform), so no
owner-only default was taken.

### The second review round's findings (`r1`, on `7ab7ab2` + working tree), fixed and re-measured

`review-findings-r1.md` carried two P2 findings, both about `scripts/opencode-native-check.py`'s source
direction. Each was reproduced before anything was changed — in-process against the shipped module, and, for
the first one, against the route's own *writer* through a throw-away Go probe
(`internal/ccc/zz_r1_probe_test.go`, deleted again before the gate) so the expectation the comparison now
holds is a measurement rather than a deduction. Both fixes are pinned by cases in
`scripts/tests/test_opencode_native_check.py` that fail on the pre-fix file.

**R1-1 — a record that carries a tool call: the expectation held a text no target holds.** The reading a
document records was the reading the *installed rollout* was compared with, but the two are not the same for
one shape: an assistant record with a text part beside a tool part. The probe (the committed fixture with one
such record appended, read by the inherited provider and written by `ConvertToCodexRollout`) measured both
halves:

```
--- PASS: TestZZR1ProbeMixedDialogue/text_and_tool
    read: ordinal 5 role assistant content "Let me check the policy file too." thinking "" renders=true projected=false dialogue="" calls=1
    item: kind tool_call text "" tool "read"
    item: kind tool_result text "38: // policy.go\n40: const maxAttempts = 4" tool ""
    loss: unresolved_dialogue = 1 (omitted)
--- PASS: TestZZR1ProbeMixedDialogue/reasoning,_text_and_tool
    read: ordinal 5 role assistant content "[Thinking]\nThe policy file holds the attempt cap.\n[/Thinking]\nLet me check the policy file too." thinking "" renders=true projected=false dialogue="" calls=1
```
with the second record's own text (a record carrying no call) written as an `assistant_message` and counted
`unprojected_dialogue` — i.e. the text of a *call-bearing* record is what no target holds, for both part
orders. In-process, the comparison as it stood reported a difference a correct conversion cannot avoid:

```
finding1 expectation: {"user_texts": ["prompt"], "call_names": ["read"], "call_inputs": ["{\"path\":\"/x\"}"],
                       "call_results": ["38: // policy.go"], "assistant_texts": ["Let me check the policy file too."]}
finding1 rollout    : {"user_texts": [], "call_names": ["read"], "call_inputs": ["{\"path\":\"/x\"}"],
                       "call_results": ["38: // policy.go"], "assistant_texts": []}
finding1 differences: ['the user text(s) are [], the source recorded ["prompt"]', 'the assistant text(s) are [],
                       the source recorded ["Let me check the policy file too."]']
```

The fix is a second reading: `opencode_written_evidence` is what the installed rollout is compared with
(every member as before, and **no** assistant text for a record that carries a call), while the planting's
document-against-document comparison keeps `opencode_document_evidence` and its stricter reading (a store
whose assistant record lost a part stays a difference). Equality still *pins* the omission — a target that
wrote the rendering holds an assistant text the read never established, and that is a difference — and the
check's own line is renamed to say what it compares (`the installed rollout holds what a conversion writes of
the fixture's own prompts, tool call, result text and texts`; §1's prose above names the move). The
disposition (model, not refuse) is recorded as Q-0008-07. Pinned by
`SourceEvidence.test_a_record_carrying_a_call_is_expected_with_no_text` and, through the whole flow, by
`SourceDirectionRun.test_a_document_with_a_mixed_record_converts_without_a_false_difference` — measured
pre-fix as `RESULT: FAIL — 1 check(s) failed: the installed rollout holds the fixture's own prompts, tool
call, result text and texts`, post-fix `exit 0` with no failed check.

**R1-2 — the load-only run accepted resume vectors naming another program.** `record_codex_invocation`
checked each printed line against the report's *own* vector and the session id's membership, and the vectors
themselves against this run's harness only inside `run_source_turn` — which a run without `--resume-prompt`
never reaches. Reproduced in-process and through the flow: a pair naming `/usr/bin/other-program` (and its own
rendered lines, so the line checks agree with it) was recorded `PASS`, and a driven load-only run ended
`RESULT: PASS — 18 check(s), none failed`. The fix reads both vectors on every run
(`read_codex_resume_vector` for the interactive form, `read_codex_turn_vector` for the exec form, both
against the harness this run probed and the session it converted), records the broken rule in the check's own
detail, and the driven case now ends `RESULT: FAIL — 1 check(s) failed: the printed resume lines name this
run's own home and session` naming `'/usr/bin/other-program'`. Pinned by
`SourceTurnVector.test_the_invocation_record_reads_both_vectors_without_a_turn`,
`test_the_interactive_vector_…`/`test_an_interactive_vector_naming_another_program_session_or_shape_is_refused`
and `SourceDirectionRun.test_a_load_only_run_reads_the_printed_vectors_and_fails_on_another_program`.

Re-measured: the check's own suite 95 → 101 cases and the `selftest` step 423 → 429 (`Ran 429 tests`,
`rc=0`); the source direction re-run on this host against OpenCode `2.2.5` and Codex `0.170.0` (load level,
the invocation check now reading both vectors of a real report):

```console
$ python3 scripts/opencode-native-check.py --direction opencode-to-codex \
    --opencode /home/azureuser/.npm-global/bin/opencode
…
PASS: the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result text and texts — 14 record(s); 2 prompt(s), 1 call(s) and their recorded results compared with the fixture's own bytes
PASS: the printed resume lines name this run's own home and session — CODEX_HOME=/tmp/ccc-opencode-source-check-2_5piu6o/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex resume 01a1f3b7-6336-7280-8d77-094dad7eea7e | CODEX_HOME=/tmp/ccc-opencode-source-check-2_5piu6o/codex-home /home/azureuser/.local/share/mise/installs/codex/latest/bin/code …[the rest of this output was dropped]
…
RESULT: PASS — 18 check(s), none failed
```

and with a turn requested (the same command plus `--resume-prompt "Reply with exactly: OPENCODE-SOURCE-OK"
--expect OPENCODE-SOURCE-OK`) the arm §2 discloses is unchanged: `FAIL: a real turn continues from the
installed session — exit 1, session 3804 -> 109186 bytes` (the harness's own `401 Unauthorized` in a scratch
home), `FAIL: the turn's answer contains 'OPENCODE-SOURCE-OK'`, and every other line passing — including
`the turn was appended to the installed session — 14 record(s) before the turn, 28 after` — with
`RESULT: FAIL — 2 check(s) failed` and exit 1. Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 429 tests in 7.642s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5718 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`AGENTS.md` does not move with this round: it changes no layout, gate step, command, read order or rule an
agent follows — the check's own registration in `docs/harness.md` carries the two rules this round sharpened
(the comparison's mixed-record reading and the load-only reading of both vectors), and the plan's §11 gained
its fifth departure note.

### The third review round's findings (`r2`, on `45be503` + working tree), fixed and re-measured

`review-findings-r2.md` carried two P2 findings, both about `scripts/opencode-native-check.py`'s
source-direction comparison and both established by the reviewer's static trace. Each was reproduced here
before anything was changed, in-process against the shipped module and over the check's *own* extractors
rather than over hand-built dicts (the two-call document below through `opencode_written_evidence`, the
records a conversion writes for it through `rollout_evidence`). Four cases were added to
`scripts/tests/test_opencode_native_check.py`; run against the pre-fix script (a scratch copy of `scripts/`
with `git show HEAD:scripts/opencode-native-check.py` in place of the fixed one, the new test file kept),
seven cases fail — the list and the behaviours below — and the check's own suite passes 105/105 on this
tree.

**R2-1 — the comparison read detached lists, so no call/result association was compared (P2).**
`_opencode_evidence` carried every call's own `id` in no member and `rollout_evidence` dropped every
`call_id`, so a same-count reassignment of the recorded outputs to the wrong calls left every compared
list identical. Reproduced: a two-call document (a `read` part and a `grep` part, each with one recorded
result) whose rollout writes both calls and both outputs with the two outputs' `call_id`s swapped — the
count, the order and the texts are the conversion's own:

```
pre-fix  expected: {"assistant_texts": [], "call_inputs": ["{\"path\":\"/a\"}", "{\"pattern\":\"x\"}"], "call_names": ["read", "grep"], "call_results": ["A's lines", "B's lines"], "user_texts": []}
pre-fix  got     : {"assistant_texts": [], "call_inputs": ["{\"path\":\"/a\"}", "{\"pattern\":\"x\"}"], "call_names": ["read", "grep"], "call_results": ["A's lines", "B's lines"], "user_texts": []}
pre-fix  differences: []
fixed    expected: {"assistant_texts": [], "call_inputs": ["{\"path\":\"/a\"}", "{\"pattern\":\"x\"}"], "call_names": ["read", "grep"], "call_result_owners": ["0", "1"], "call_results": ["A's lines", "B's lines"], "user_texts": []}
fixed    got     : {"assistant_texts": [], "call_inputs": ["{\"path\":\"/a\"}", "{\"pattern\":\"x\"}"], "call_names": ["read", "grep"], "call_result_owners": ["no call", "0"], "call_results": ["A's lines", "B's lines"], "user_texts": []}
fixed    differences: ['the recorded tool result owner(s) are ["no call", "0"], the source recorded ["0", "1"]']
```

Every result is now carried under the call it answers (`call_result_owners`): on the source side the
call's place in the walk (the read answers each result on the part that recorded its call), on the rollout
side the file's own pairing — an output's recorded `call_id` names its call, the next unmatched call
record under that id where several carry one, and `no call` where no unmatched call record answers it
(measured on the swapped file above: the first output names `call_b` before that call's record is written,
so it answers no call yet; the second output claims the first call, whose own output it is not). The
recorded id itself is deliberately not compared: the source's ids are not the wire's to preserve (a
conversion trims a recorded one and generates one for a call that recorded none, so an id-less part's
expectation cannot know the generated spelling), while the association is what "one output per call" means.
Pinned by `SourceEvidence.test_an_output_reassigned_to_another_call_is_a_difference` (both mutations —
both outputs claimed by the first call, and the owners swapped — with the detached members asserted
unchanged so the pairing member is what catches them),
`test_a_target_that_mangled_the_call_or_a_prompt_is_a_difference`'s reassigned-result arm,
`test_a_record_carrying_a_call_is_expected_with_no_text`'s member loop, and
`SourceDirectionFraming.test_the_committed_fixture_is_the_source_document`'s owner assertion.

**R2-2 — the membership predicate was Python's, not the reader's (P2).** The assistant-text and
result-text membership tests used `if rendering:` and `text.strip()`, where the rule is the reader's own
`strings.TrimSpace`: a whitespace-only rendering is not a message a conversion writes (the reader's filter
drops the record where nothing else keeps it, and where token usage keeps it the writer's whitespace rule
still leaves the message out, because the target's own reader would read a blank message as no message),
and a text of the information separators `\x1c`-`\x1f` *is* written (Go keeps them where Python's
`str.strip` removes them). Reproduced:

```
pre-fix  blank assistant_texts : ['   ']      (the writer writes no message for it)
pre-fix  separator call_results: []           (the writer writes "\x1c")
fixed    blank assistant_texts : []
fixed    separator call_results: ['\x1c']
```

Both memberships now ask `opencode_trimmed` (the reader's own set, already spelled for the user-text
member). Pinned by `test_a_whitespace_only_assistant_text_is_expected_with_no_message`,
`test_a_result_whose_text_is_only_the_separator_is_expected`, and, through the whole flow,
`SourceDirectionRun.test_a_whitespace_only_assistant_record_converts_without_a_false_difference` — whose
fabricated rollout is the measured writer's rule (`rollout_records_of`, which now reads every membership
through the same functions) and which fails on the pre-fix script with the check's own false difference
(`the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result
text and texts`).

The pre-fix run of the new test file against the old script (`FAILED (failures=4, errors=3)`):

```
FAIL: test_a_result_whose_text_is_only_the_separator_is_expected (test_opencode_native_check.SourceEvidence.test_a_result_whose_text_is_only_the_separator_is_expected)
FAIL: test_a_target_that_mangled_the_call_or_a_prompt_is_a_difference (test_opencode_native_check.SourceEvidence.test_a_target_that_mangled_the_call_or_a_prompt_is_a_difference) (case='a reassigned result')
FAIL: test_a_whitespace_only_assistant_record_converts_without_a_false_difference (test_opencode_native_check.SourceDirectionRun.test_a_whitespace_only_assistant_record_converts_without_a_false_difference)
FAIL: test_a_whitespace_only_assistant_text_is_expected_with_no_message (test_opencode_native_check.SourceEvidence.test_a_whitespace_only_assistant_text_is_expected_with_no_message)
ERROR: test_a_record_carrying_a_call_is_expected_with_no_text (test_opencode_native_check.SourceEvidence.test_a_record_carrying_a_call_is_expected_with_no_text)
ERROR: test_an_output_reassigned_to_another_call_is_a_difference (test_opencode_native_check.SourceEvidence.test_an_output_reassigned_to_another_call_is_a_difference)
ERROR: test_the_committed_fixture_is_the_source_document (test_opencode_native_check.SourceDirectionFraming.test_the_committed_fixture_is_the_source_document)
```

(The `ERROR`s are the cases whose *members* the pre-fix extractors do not carry at all; the `FAIL`s are
the behavioural ones, the reassigned-result arm — the finding's own shape — and the two whitespace
memberships among them.)

Re-measured: the check's own suite 101 -> 105 cases (`Ran 105 tests`, `OK`) and the `selftest` step
429 -> 433 (`Ran 433 tests`, `rc=0`); the source direction re-run on this host against OpenCode `2.2.5`
and Codex `0.170.0`, at the load level — the strengthened rollout comparison passes over the file the
real writer installs:

```console
$ python3 scripts/opencode-native-check.py --direction opencode-to-codex \
    --opencode /home/azureuser/.npm-global/bin/opencode
…
PASS: the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result text and texts — 14 record(s); 2 prompt(s), 1 call(s) and their recorded results compared with the fixture's own bytes
PASS: the target harness's own loader verified the installed session — the harness's loader answered with this session's own id and path; opening a session makes the harness record its own settings in it (measured: one thread_settings_applied record), and no session other than the one it opened is touched
…
RESULT: PASS — 18 check(s), none failed
```

and with a turn requested, the arm §2 discloses is unchanged — every other line passing, the installed
rollout's comparison included, and the turn failing on the same boundary:

and with a turn requested (the same command plus `--resume-prompt "Reply with exactly:
OPENCODE-SOURCE-OK" --expect OPENCODE-SOURCE-OK`), the arm §2 discloses is unchanged — every other line
passing, the installed rollout's comparison included, and the turn failing on the same boundary:

```console
…
FAIL: a real turn continues from the installed session — exit 1, session 3804 -> 109185 bytes
FAIL: the turn's answer contains 'OPENCODE-SOURCE-OK'
PASS: the turn was appended to the installed session — 14 record(s) before the turn, 28 after; the records the conversion wrote are unchanged; the prompt is in the records it now holds
RESULT: FAIL — 2 check(s) failed: a real turn continues from the installed session, the turn's answer contains 'OPENCODE-SOURCE-OK'
```

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 433 tests in 7.887s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5718 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`AGENTS.md` does not move with this round: it changes no layout, gate step, command, read order or rule an
agent follows — the check's own registration in `docs/harness.md` carries the two rules this round added
(each result read under the call it answers; every text membership read through the reader's own trimming),
and the plan's §11 gained its sixth departure note. No new OPEN-QUESTIONS entry: both findings are the
comparison's own already-claimed rules ("one output per call" and the reader's filter), so no owner-only
default was taken. No goal file moves: 0008 is still the increment on its branch, awaiting operator
acceptance.

### The fourth review round's findings (`r3`, on `f56be51` + working tree), fixed and re-measured

`review-findings-r3.md` carried one P2 finding, about `scripts/opencode-native-check.py`'s source direction
and established by the reviewer's static trace. It was **reproduced before anything was changed**, end to
end, with both real harnesses on this host — OpenCode `2.2.5`
(`/home/azureuser/.npm-global/bin/opencode`) and Codex `0.170.0` — by planting a document of the finding's
shape through the harness's own importer (`--source /tmp/ccc-fileblock-doc.json`, a copy of the committed
fixture whose first tool part's `state.content` is a `file` block beside a `text` block, plus two further
file-bearing parts described below). The pre-fix run's rollout comparison, in the check's own words (exit 1):

```
PASS: the store holds the fixture document's own conversation — 6 message(s), 3 tool call(s) and their recorded results compared with the fixture's own bytes
FAIL: the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result text and texts — the recorded tool result(s) are ["[{\"mime\":\"text/plain\",\"name\":\"config.go\",\"type\":\"file\",\"uri\":\"file:///home/user/code/example-api/internal/retry/config.go\"},{\"text\":\"Read f …[the rest of this output was dropped], the source recorded []; the recorded tool result owner(s) are ["0", "1", "2"], the source recorded []
RESULT: FAIL — 1 check(s) failed: the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result text and texts
```

**R3-1 — a file-bearing result: the expectation held no text where the conversion writes the parser's own
encoding (P2).** The finding's trace is exact: for a part whose `state.content` carries a `file` block the
carried parser moves the part's text into *blocks* and marshals them
(`internal/parser/opencode_v2.go`: `json.Marshal(blocks, json.Deterministic(true))`), the event it answers
carries that encoding as its content, the read hands it on as decoded text
(`internal/ccc/reader.go`'s result events, `ContentDecoded: true`) and `codex_target.go`'s `resultText`
therefore writes it verbatim, counting no `unrepresentable_result_blocks` — while the expectation's own
builder returned `""` for the shape, so a faithful conversion was reported as a difference and the case
`test_a_part_carrying_a_file_block_is_expected_with_no_text` pinned the wrong reading.

Measured cross-language before the fix, through a throw-away Go probe (removed afterwards; one document
planted through `internal/ccc/opencodefixture`, read through `NewSourceReader` and written by both target
writers, on four file-bearing shapes) — the Go conversion's own bytes for each part, which the fixed
expectation now answers identically (the Python mirror was compared against these lines byte for byte):

```
part 1 (file + text):       [{"mime":"text/plain","name":"config.go","type":"file","uri":"file:///home/user/code/example-api/internal/retry/config.go"},{"text":"Read file /home/user/code/example-api/internal/retry/config.go, lines 38-42\n38: // policy.go","type":"text"}]
part 2 (inline image):      [{"image_url":"DATA:IMAGE/png;base64,AAAB","mime":"image/png","name":"shot.png","type":"input_image"}]
part 3 (control chars):     [{"mime":"application/octet-stream","type":"file","uri":"file:///ctl"},{"text":"ctl\u0001\u001c<&>é中😀\n\tend","type":"text"}]
part 4 (structured+error):  [{"mime":"text/plain","name":"policy.go","type":"file","uri":"file:///policy"},{"text":"38: // policy.go","type":"text"},{"text":"boom","type":"text"}]
```

(The same four lines are what the Claude route's `tool_result` block holds — the encoding is the *read's*
answer, and both writers write it verbatim; measured on the same probe. Part 4's shape cannot be planted:
this harness's importer refuses a document carrying an errored tool part — `Failed to import session: Bad
Request`, measured on 2.2.5 — which is why the ordering it pins is a case at the expectation's level, and
why the planted document above carries parts 1–3.)

The encoder's own rules were measured, not assumed: through a throw-away Go probe over every code point
below `0x20`, plus `0x7f`, `<>&'`, `U+2028`/`U+2029`, `U+FFFD` and non-ASCII,
`json.Marshal(map[string]string{"k": …}, json.Deterministic(true))` answered the short escapes
`\b \t \n \f \r` for those five, `\u00xx` for the other twenty-seven control characters
(`01:{"k":"\u0001"} … 1f:{"k":"\u001f"}`), and every other character written literally (`20:{"k":" "}`,
`7f` raw — its own byte between the quotes, DEL unescaped — and `U+2028`, `<`, `&`, `é`, `中`, `😀` raw:
no HTML escaping), with map members in byte-lexicographic name order
(measured: `{"":"e","A":"a","a":"b","a-":"c","a1":"d",…}`). Python's own `json.dumps` differs on exactly
those bytes (`\b`→`\u0008`, `\f`→`\u000c`, HTML escaping), so the expectation carries its own encoder
mirror (`go_json_string`/`opencode_go_json`).

The fix follows the source-read path rather than refusing the shape (the disposition Q-0008-08 records, on
Q-0008-07's reasoning): `opencode_result_block_json` builds the blocks exactly as the parser does — every
`text` block, a `file` block with `uri`/`mime` and a `name` only where recorded, an inline `data:image/…`
file rewritten to `input_image`/`image_url` with `uri` dropped and `mime`/`name` kept (the parser rewrites
those two members and nothing else), the `read` tool's structured content and an errored part's
`error.message` appended as text blocks after the content's own blocks — and the same member is what both
readings carry, so the planting's document-against-document comparison reads the encoding too (a store whose
part lost its block is a difference rather than two empty members).

**Every rule pinned by an executed mutation.** Each rule of the new surface was reverted, in a scratch copy
of `scripts/` (the pre-fix line, and one reversed line per encoder member rule), the case that claims the
rule was run, and the line restored — the observed failures:

| Rule | Mutation (one line, reverted and restored) | Observed |
| --- | --- | --- |
| a file-bearing part answers the parser's encoding | `return opencode_result_block_json(part, state)` → `return ""` (the pre-fix line) | both new cases FAIL: `Lists differ: [] != ['[{"mime":"text/plain","type":"file","uri…` and `[] is not true : a text inside the encoding was not reported as a difference` |
| control characters escape as `\u00xx` | `out.append(f"\\u{ord(character):04x}")` → `out.append(character)` | FAIL: `'"\x00"' != '"\\u0000"'` |
| `\b` keeps its short escape | the `"\b": "\\b"` table entry dropped | FAIL: `'"\\u0008"' != '"\\b"'` |
| `0x20` is not escaped | `character < " "` → `character <= " "` | FAIL: `'"\\u0020"' != '" "'` |
| an image block drops its `uri` | `del entry["uri"]` → (removed) | FAIL: `…,"type":"input_image","uri":"DATA:IMAGE/png;base64,AAAA"}]' != …` |
| the image prefix is compared case-insensitively | `.lower() == IMAGE_URI_PREFIX` → `== IMAGE_URI_PREFIX` | FAIL: `'[{"mime":"image/png","name":"shot.png","type…' != '[{"image_url":"DATA:IMAGE/png;base64,AAAA",…` |
| members are written in name order | `for name in sorted(block)` → `for name in block` | FAIL: `'[{"uri":"u","type":"file","mime":"m","name":"n"}]' != '[{"mime":"m","name":"n","type":"file","uri":"u"}]'` |
| the structured content is appended as a text block | `append_text(structured["content"])` → `append_text("WRONG")` | FAIL: `…"xt":"WRONG","type":"text"},{"text":"boom","type":"text"}]' != …` |
| an errored part's message is appended | the `append_text(message)` guard negated | FAIL: the expected `{"text":"boom","type":"text"}` block is missing |

(One measurement caveat, recorded because it bit this round: the case module imports the shipped script by
path, and `importlib`'s bytecode cache keys on the source's mtime and size, so two scratch mutations within
one second that happen to share a size can serve stale bytecode — the first pass reported two rules as
unpinned and re-measuring with the cache cleared showed both failing. The shipped cases are unaffected: the
module is not rewritten under a test run.)

Re-measured on this tree: the check's own suite 105 → 109 cases (`Ran 109 tests`, `OK`) — the pre-fix case
replaced by five (`…is_expected_as_the_parsers_own_encoding`, `…planting_comparison_reads_the_encoding_too`,
`…encoding_mirrors_the_parsers_own_json_bytes`, `…inline_image_block_is_encoded_in_the_readers_own_members`,
`…structured_content_and_error_message_follow_as_text_blocks`) — and the `selftest` step 433 → 437
(`Ran 437 tests`, `rc=0`); the source direction re-run on this host against OpenCode `2.2.5` and Codex
`0.170.0`, at the load level, over the three-shape planted document:

```console
$ python3 scripts/opencode-native-check.py --direction opencode-to-codex \
    --opencode /home/azureuser/.npm-global/bin/opencode \
    --codex /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex \
    --source /tmp/ccc-fileblock-doc.json
…
PASS: the store holds the fixture document's own conversation — 6 message(s), 3 tool call(s) and their recorded results compared with the fixture's own bytes
PASS: the installed rollout holds what a conversion writes of the fixture's own prompts, tool call, result text and texts — 18 record(s); 2 prompt(s), 3 call(s) and their recorded results compared with the fixture's own bytes
…
RESULT: PASS — 18 check(s), none failed
```

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 437 tests in 8.099s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5718 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 174 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`AGENTS.md` does not move with this round: it changes no layout, gate step, command, read order or rule an
agent follows — its OpenCode-source bullet's wording ("the terminal event's text is what the target holds")
is what the fixed expectation now honours, and the check's registration in `docs/harness.md` carries the
rule this round added (the parser's own block encoding for a file-bearing result, mirrored byte for byte).
The plan's §11 gained its seventh departure note and the decision record a new entry (Q-0008-08, the
disposition between mirroring and refusing). No goal file moves: 0008 is still the increment on its branch,
awaiting operator acceptance.

### The fifth review round's findings (`r4`, on `262704b` + working tree), fixed and re-measured

`review-findings-r4.md` carried one P2 finding, about the *suite itself* and established by static
inspection: `scripts/tests/test_opencode_native_check.py` held a backslash inside an f-string expression
(`f'"{short.get(code, f"\\u{code:04x}")}"'`, line 1917 — added by the previous round's Go-encoder mirror),
a form PEP 701 relaxed at 3.12 and 3.11's grammar rejects. `scripts/gate.py` runs discovery with the
gate's own interpreter and `selftest` is a required step, so the documented gate could not import the suite
on a 3.11 host. **Reproduced before anything was changed**, on this host's CPython 3.11.14
(`/home/azureuser/.local/bin/python3.11`), the finding's own two traces:

```console
$ /home/azureuser/.local/bin/python3.11 -c "compile(open('scripts/tests/test_opencode_native_check.py','rb').read(), 'x', 'exec')"
SyntaxError: f-string expression part cannot include a backslash        # line 1917
$ /home/azureuser/.local/bin/python3.11 -m unittest discover -s scripts/tests -p 'test_*.py'
Ran 329 tests in 8.601s
FAILED (failures=2, errors=1)          # broken: test_opencode_native_check (the import error)
```

(The two failures are `HostCase`'s nested runs, which re-run the whole suite and report the broken child;
the module discovery could not import is the one error, which is why the count reads 329 where the
repaired suite reads 442.)

**R4-1 — the required selftest must be importable on every minor the docs name (P2).** Fixed as the
finding asked: the escaped fallback is computed outside the outer f-string
(`fallback = f"\\u{code:04x}"  # outside the f-string: 3.11 forbids a backslash in one`), so the case's
semantics are unchanged and the module parses at 3.11. The round-4 close-out asked for a *pin* rather than
another one-line fix, and the class had none: no case could fail when a shipped source needed grammar above
the floor, so the same defect could arrive again in any file the gate or the suite loads.

**The pin: `scripts/tests/test_python_floor.py`** (5 cases; `scripts/**/*.py` — the Python the manifest
marks `owned`; the carried upstream tree is out of scope and untouchable). Its docstring carries the
claim, the measured facts and its own stated limit; two arms answer the floor, neither skipping, so the
claim holds on every host:

- the **grammar arm**, `ast.parse(source, feature_version=(3, 11))` — the running interpreter's own parser
  constrained to the floor. Measured on CPython 3.15.2 it *rejects* PEP 695 (`type X = int`,
  `def f[T]()`, `class C[T]`), 3.13's type-parameter defaults, and 3.14's t-strings and
  parenthesized-free `except`, while it *accepts* every f-string form PEP 701 relaxed — the reported shape
  among them. On a 3.11 host the arm is the floor parser itself, so it answers the whole question there.
- the **f-string arm** reads each literal's field structure from the 3.12 tokenizer
  (`FSTRING_START`/`FSTRING_END`, field braces as real `{`/`}` operators) and reports the four relaxations
  the grammar arm cannot see.

**f-strings and `feature_version`, measured.** Each `FSTRING_CORPUS` entry was compiled by CPython 3.11.14
*and* CPython 3.15.2 on 2026-11-19; the table below is the 3.11 half re-derived against the shipped corpus
(mismatches against the committed expectation: **0**), and every rejection is 3.11's own words:

| Corpus entry | 3.11.14 | 3.15.2 |
| --- | --- | --- |
| the reviewed defect: `x = f"{d.get(1, f"\u{1:04x}")}"` | `unexpected character after line continuation character` (the literal is truncated at its own quote) | accepted |
| the same quote inside a field: `x = f"{d["k"]}"` | `f-string: unmatched '['` | accepted |
| a same-quote nested literal: `x = f"{f"{a}"}"` | `f-string: expecting '}'` | accepted |
| the own triple-quote run in a field: `x = f"""{d["""k"""]}"""` | `f-string: unmatched '['` | accepted |
| a comment in a field (two lines, the field closed on the second) | `f-string expression part cannot include '#'` | accepted |
| a backslash in a field after a non-ASCII literal: `x = f"é{'\n'}{a}"` | `f-string expression part cannot include a backslash` | accepted |
| a field inside two format specs: `x = f"{a:{b:{c}}}"` | `f-string: expressions nested too deeply` | accepted |
| *floor-valid, reported by neither arm*: a backslash in the literal part (`f"A\nB{a}"`), a backslash in a spec (`f"{a:\n}"`), `#` in a spec (`f"{a:#x}"`), one spec deep (`f"{a:{w}}"`), other quotes in a field (`f"{'a' + 'b'}"`, `f"{f'{a}'}"`), `#` inside a string in a field (`f"{ '#' }"`), a same quote in a triple (`f'''{ 'a' }'''`), a two-quote run in a triple (`f"""{ "" }"""`), a slice colon (`f"{a[1:2]}"`) | accepted | accepted |

Two further measurements shaped the arms rather than the corpus: `feature_version=(3, 11)` is a *no-op*
for f-strings on a newer interpreter (it accepted the reviewed defect's exact line), which is why the
second arm exists at all; and a UTF-8 BOM is stripped by the importer but rejected by `ast.parse` of the
raw *string*, so the pin reads every source `utf-8-sig` and a case pins that reading (and its failure
mode) on a throw-away copy.

**Every rule proven by an executed mutation, on this tree, applied and reverted.** The observed failure is
the pin's own message (the live-tree case names path, line and rule):

| Rule | Mutation (the first row restores the finding's own line; the middle three append one shape each to `scripts/opencode-native-check.py`) | Observed |
| --- | --- | --- |
| a backslash in a field's expression (the reviewed defect) | `scripts/tests/test_opencode_native_check.py:1917` reverted to `f'"{short.get(code, f"\\u{code:04x}")}"'` | `FAIL: scripts/tests/test_opencode_native_check.py:1917: a backslash stands in a replacement field's expression, which the floor rejects` |
| a comment in a field | `COMMENT_PROBE = f"""{1 # a comment in a field`⏎`}"""` (the comment runs to the line's end, so the field closes on the second line) | `FAIL: …:2203: a comment stands in a replacement field's expression, which the floor rejects` |
| the literal's own quote inside it | `OWN_QUOTE_PROBE = f"{d["k"]}"` | `FAIL: …:2205: the literal's own quote stands inside it, where the floor's tokenizer ends the literal` |
| a field inside two format specs | `TWO_SPECS_PROBE = f"{a:{b:{c}}}"` | `FAIL: …:2206: a replacement field stands inside two format specs, deeper than the floor allows` |
| grammar above the floor (the grammar arm) | `type FloorProbe = int` | `FAIL: …:2207: Type statement is only supported in Python 3.12 and greater` |
| the BOM reading | `read_source`'s `utf-8-sig` → `utf-8` | `FAIL: Lists differ: [(1, 'invalid non-printable character U+FEFF')] != []` |

The mutant script's own floor verdict, for the record (the shapes are floor-rejected, not merely
newer-valid): 3.11's compile of `scripts/opencode-native-check.py` with the four probes answered
`f-string expression part cannot include '#'` at line 2204. Each corpus entry additionally pins its rule's
*reason string* by name, and the live-tree case is what the mutations above exercise.

**Re-measured on this tree.** The `selftest` step 437 → 442 cases (`Ran 442 tests`, `rc=0`), and the suite
now holds on the floor interpreter the docs name (where the pre-fix tree read `Ran 329 … FAILED`):

```console
$ /home/azureuser/.local/bin/python3.11 -m unittest discover -s scripts/tests -p 'test_*.py'
Ran 442 tests in 8.143s

OK
```

Gate, re-run on this tree (`GATE: PASS`, exit 0):

```console
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.639s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5719 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored byte…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 188 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The finding's own scenario — *the documented gate run under 3.11* — is re-derived whole on the same tree,
not inferred from the `selftest` step alone (the Go steps are the same commands on the same tree):

```console
$ /home/azureuser/.local/bin/python3.11 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3624 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 442 tests in 8.167s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5719 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored byte…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets  no leaks found in 188 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`AGENTS.md` moves with this round: the rule review round 4 found unenforced is now named where the file
already lists the suite's cross-platform and cross-minor rules — "no grammar above the oldest minor the
docs name (`scripts/tests/test_python_floor.py` holds every `scripts/**/*.py` to it: the running parser
constrained to 3.11, plus the four PEP 701 f-string relaxations `feature_version` cannot see)" — and the
two documents that state the rule gained the pin's description (`docs/conventions.md`'s Python paragraph,
`docs/harness.md`'s `selftest` section). The plan's §11 gained its eighth departure note and
`docs/decisions/OPEN-QUESTIONS.md` a ninth 0008 entry (Q-0008-09: the floor is pinned in the suite; whether
CI also runs a floor leg is the operator's call, because the CI matrix and `docs/harness.md`'s CI section
are one statement held together by `scripts/tests/test_ci_matrix_claim.py`, which refuses a matrix key it
does not read). No goal file moves: 0008 is still the increment on its branch, awaiting operator
acceptance, and none of the other goal files under `docs/goals/` is accepted (0009–0013 provisional,
0014–0017 ready).

## 7. Residual limits and boundaries named rather than hidden

- **The real turn on this host.** §2 and §4: a Codex turn in a scratch home answers `401 Unauthorized`; the
  load half (Codex's own `thread/resume`) is what is verified, and the turn is recorded as a failed check
  with the harness's own words. Q-0008-05 states the default and its reversal.
- **The Claude half's native load** needs the Claude Code binary, which this host does not have; §3 shows
  the conversion, the installation and the inherited reader's structural read-back, and 0003's accepted
  check remains the native half on a host that has the binary.
- **The boundary for a container member** is `not-taken` by design (no file holds one member's own bytes;
  the store's own writes move the database file), so two conversions of one store are told apart by the
  read's own fingerprint and by the run's target identity rather than by a digest of the file.
- **The dialogue arm** for a reader that renders a trace without recording it is the shipped one (Q-0008-01):
  the rendering travels and is counted; the mechanism arm Q-0007-12 deferred here is not implemented, with
  the reasons recorded there.
- **No parity/oracle case** for the OpenCode source (Q-0008-04): the oracle is file-shaped; `parity` proves
  the carried reader's bytes and the native run reads a store the harness itself wrote.
