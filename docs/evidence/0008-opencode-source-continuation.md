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
The comparison the goal's second clause asks for is the `PASS: the installed rollout holds the fixture's own
prompts, tool call, result text and texts` line: the *installed rollout's own records* are read against the
*fixture document's own bytes*, so a writer that mangled a name, an argument or a result cannot pass by
agreeing with itself.

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
direction live in `scripts/tests/test_opencode_native_check.py` — 89 cases, run by the `selftest` step,
among them the six that pin the expectation's own rules (an open call's blanked input, a non-`read` part's
structured content, a part carrying a file block, an errored result, a textless one) and the refusal of a
default fixture whose identity drifted. The obligation-citation manifest is `docs/evidence/0008-citations.tsv`
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
moved to `docs/goals/archive/`; and `docs/decisions/OPEN-QUESTIONS.md` (Q-0008-01 … Q-0008-05).

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
