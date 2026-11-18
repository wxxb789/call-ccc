# 0006 — Continue conversations into and out of Oh My Pi: evidence

Contract: [`docs/goals/0006-omp-continuation.md`](../goals/0006-omp-continuation.md) (archived to
`docs/goals/archive/` in the increment that merges it, per `AGENTS.md`).
Plan: [`docs/plans/0006-omp-continuation.md`](../plans/0006-omp-continuation.md).
Harness: Oh My Pi `omp/18.18.7` (`omp --version`), the pinned AgentsView carry
`83195c8a6861b8c74a4b6cb87512817fed6f8584` (reported by every run's `carried:` line).

Every command below ran on the host named by each block, in this repository, on branch
`feat/0006-omp-continuation`. Output is quoted verbatim; `…[the rest of this output was dropped]` is the
harness's own elision of a bounded detail line, and a block that stops above the CLI's trailing
`carried:` footer (and above any `warning:` lines a run prints) says so in place; nothing else was
edited.

## 0. What was measured before anything was written

The plan's §2.1 holds the table; this is what it rests on, all measured on this host against the
installed harness rather than read from a document:

| Question | Measured answer |
| --- | --- |
| the data directory | `PI_CODING_AGENT_DIR` (the same name Pi uses), default `~/.omp/agent`; sessions at `<home>/sessions/<project spelling>/<timestamp>_<id>.jsonl` |
| `PI_CODING_AGENT_SESSION_DIR` | names the directory that holds session **files** directly: the project directory resolves an id, the `sessions` root above it answers `Error: Session "<id>" not found.` |
| the project spelling | three branches, read out of the shipped bundle (`getDefaultSessionDirPath`) and confirmed by observation: under the OS temporary directory `-tmp` + its relative path (that branch wins when both apply), under the home `-` + its home-relative path, else Pi's absolute `--…--` form; a session filed under Pi's spelling is **moved** by OMP itself |
| the first line | every session OMP writes starts with a fixed-width title slot: 256 bytes including its line feed (`SESSION_TITLE_SLOT_BYTES` in the harness's own bundle); a file without one still loads |
| the model binding | restored from the session's own `model_change` record (one `<provider>/<id>` string) when it resolves in the home, else from the pairs its records carry; an unresolvable one is refused (`SessionModelRestoreError`, exit 1) |
| the per-record pair | **every** assistant record needs a non-empty `model` **and** `provider`: dropping either from one record of an otherwise loadable session makes the next turn fail while it builds its request (`undefined is not an object (evaluating 'e.length')`); `api`, `usage`, `stopReason` and `runtime` are all optional |
| a foreign provider label | inert at load and at a real turn once the session's own binding resolves |
| a load | **writes** into the session it opens (it appends a `custom`/`session_exit` record), so any check must drive a copy |
| a provider's key | must be literal: `apiKey: $VAR`/`${VAR}` is sent as those characters (`401 … Received=${TE****KEY}`); the built-in providers read the environment (`LITELLM_BASE_URL`/`LITELLM_API_KEY`), and a provider defined with `auth: none` starts the harness with no credential anywhere |
| a home with no model | the harness refuses to start (`No default model selected … Or create <home>/models.yml`), which is why a scratch home must be seeded |

## 1. Claude Code → OMP through the pipeline, with OMP's own loader and a real turn

The source is a **copy** of the committed Claude fixture whose recorded working directory
(`/home/user/code/example-api`, a path no host has) was rewritten mechanically to `/tmp/omp-ev/ws`
(`native_check_common.rewrite_source_cwd`, through the session's own JSON field), and the target home is
a scratch OMP data directory whose provider definitions were seeded from this host's own OMP home
(loopback only, with the key's value replaced by a placeholder). Exit code **0**:

```console
$ CLAUDE_PROJECTS_DIR=/tmp/omp-ev/claude-src bin/ccc continue \
    claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to omp --target-home /tmp/omp-ev/ev1 \
    --target-binary /home/azureuser/.local/share/mise/installs/oh-my-pi/latest/omp \
    --verify resume --resume-prompt 'Reply with exactly: OMP-RESUME-OK'
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → omp
  source:       /tmp/omp-ev/claude-src/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2128 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2128: the file ends at a record boundary
  converted:    omp-session-jsonl, 3595 bytes: 6 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 0 turn(s)
  carried-with-caveat: unrecorded_token_usage ×4
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: message_model_attribution ×4
  carried-with-caveat: unresolved_source_parents ×1
  carried-with-caveat: pre_compaction_history ×4
  omitted:      malformed_source_lines ×1
  carried-with-caveat: target_model_binding ×1
  carried-with-caveat: title_slot_written ×1
  installed:    /tmp/omp-ev/ev1/sessions/-tmp-omp-ev-ws/2026-05-04T09-15-00Z_01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48.jsonl [hard-link]
  source file:  unchanged (sha256 272dd4520eb6565a2435d052833e67719e2027adbd78482864fefdf0826eccbf, taken before and after the conversion)
  target id:    01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48
  working dir:  /tmp/omp-ev/ws
  resume:       cd /tmp/omp-ev/ws && PI_CODING_AGENT_DIR=/tmp/omp-ev/ev1 PI_CODING_AGENT_SESSION_DIR=/tmp/omp-ev/ev1/sessions/-tmp-omp-ev-ws /home/azureuser/.local/share/mise/installs/oh-my-pi/latest/omp --resume 01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48
  non-interactive resume: cd /tmp/omp-ev/ws && PI_CODING_AGENT_DIR=/tmp/omp-ev/ev1 PI_CODING_AGENT_SESSION_DIR=/tmp/omp-ev/ev1/sessions/-tmp-omp-ev-ws /home/azureuser/.local/share/mise/installs/oh-my-pi/latest/omp -p --resume 01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48 -- '<prompt>'
  cli_version:  18.18.7 (probe)
  structure:    verified (the inherited AgentsView OMP reader over the installed session)
  native load:  verified (the target harness's own loader (omp --mode rpc, get_state/get_messages, in a scratch data directory whose provider needs no credential, plus an unknown-id control))
                the harness's own loader read the session and answered about it (sessionId "01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48", messageCount 3); the same question with an unknown session id was answered "not found". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-omp-load-385962790/sessions/-tmp-omp-ev-ws/2026-05-04T09-15-00Z_01a1ebb0-8b94-78fc-8b36-4f5ad7dbfe48.jsonl) in a scratch data directory whose provider (ghc-proxy/gpt-5.6-sol) needs no credential, with PI_API_KEY, ANTHROPIC_API_KEY, ANTHROPIC_OAUTH_TOKEN, OPENAI_API_KEY, OPENROUTER_API_KEY, GOOGLE_API_KEY, GEMINI_API_KEY, XAI_API_KEY, DEEPSEEK_API_KEY, GROQ_API_KEY, MISTRAL_API_KEY, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AZURE_OPENAI_API_KEY, LITELLM_API_KEY, LITELLM… [the rest of this output was dropped]
  native resume: verified (one turn in the installed session through the target harness's non-interactive resume (omp -p --resume))
                the harness answered from the installed session in one turn and wrote its own records into it (the session file went from 3595 to 7097 bytes)
  answer:       OMP-RESUME-OK
```

(The block stops above the CLI's own trailing `carried:` footer — §3 below quotes that line verbatim:
`carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified
(scripts/upstream-parity.py)` — and above any `warning:` lines a run prints. Nothing else was edited
out.)

The CLI's own `answer:` line is the answer the harness returned from the installed session; the same
words, as the harness's own printed output, are:

the turn's own output (the target's answer, as the harness printed it):
    answer:       OMP-RESUME-OK

The three states the contract separates are all here and all separate: **converted** (`6 message(s)`, one
tool call and its result, one compaction boundary, 31 loss categories named), **installed** (a hard link
at the path OMP's own spelling of `/tmp/omp-ev/ws` computes — `-tmp-omp-ev-ws` — exclusive, the source
file's digest unchanged before and after), and **natively resumable** (`verification.load` and
`verification.resume` both `verified`, the latter by one real turn the harness answered).

## 2. The OMP native check, `claude-to-omp`, with a real turn

`python3 scripts/omp-native-check.py --resume-prompt 'Reply with exactly: OMP-NATIVE-CHECK-OK' --expect
OMP-NATIVE-CHECK-OK`, exit code **0**. It re-derives §1's states from the file's own bytes and asks OMP's
own loader independently of the Go check:

```console
repo:        /home/azureuser/repos/call-ccc
direction:   claude-to-omp (claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → omp)
omp:         /home/azureuser/.local/share/mise/installs/oh-my-pi/latest/omp (omp/18.18.7)
source:      internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl (copied to /tmp/ccc-omp-native-check-u5bozm9a/claude-source/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl)
workspace:   /tmp/ccc-omp-native-check-u5bozm9a/workspace -> OMP project dir -tmp-ccc-omp-native-check-u5bozm9a-workspace
target home: /tmp/ccc-omp-native-check-u5bozm9a/omp-home
omp models:  seeded provider(s) ghc-proxy, ghc-proxy-chat from /home/azureuser/.omp/agent; left out: tencent-litellm (its endpoint https://litellm-proxy-hk.graydune-4a6be1e2.eastasia.azurecontainerapps.io/v1 is not loopback: this run writes no credential for a remote endpoint into a file); apiKey values replaced by 'ccc-native-check', headers dropped, modelRoles.default ghc-proxy/gpt-5.6-sol
the source fixture's recorded working directory (/home/user/code/example-api) is rewritten to the workspace, so the session belongs to a path this run owns and the target's own lookup can find what it installs

PASS: an OMP provider definition was seeded into the run's own scratch home — seeded ghc-proxy, ghc-proxy-chat; modelRoles.default ghc-proxy/gpt-5.6-sol; apiKey values replaced by 'ccc-native-check'
PASS: the seeded home selects a model role a real turn can use — modelRoles.default ghc-proxy/gpt-5.6-sol
PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 6 message(s), 1 call(s), 1 result(s); loss categories named: 31
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr Error: Session "ceab4666-2e40-40fe-b08f-6883bfda4147" not found.
Run `omp --resume` without an argument to pick from recent sessions, or `omp` to start a new one.
PASS: the installed session exists at the reported path — sessions/-tmp-ccc-omp-native-check-u5bozm9a-workspace/2026-05-04T09-15-00Z_01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d.jsonl [hard-link]
PASS: the installed session is filed under this run's own OMP spelling of the workspace — sessions/-tmp-ccc-omp-native-check-u5bozm9a-workspace/2026-05-04T09-15-00Z_01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d.jsonl; this run's OMP spelling of /tmp/ccc-omp-native-check-u5bozm9a/workspace is -tmp-ccc-omp-native-check-u5bozm9a-workspace
PASS: the installed file is an OMP session — entries {'compaction': 1, 'message': 7, 'model_change': 1, 'role:assistant': 4, 'role:toolResult': 1, 'role:user': 2, 'session': 1, 'title': 1}
PASS: the installed file opens with a 256-byte title slot record — first line 256 byte(s) including its line feed, type title
PASS: every assistant record carries a non-empty model and provider — 4 assistant message(s), 0 without the pair
PASS: the load check verified the installed session natively — the harness's own loader read the session and answered about it (sessionId "01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d", messageCount 3); the same question with an unknown session id was answered "not found". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-omp-load-2402448392/sessions/-tmp-ccc-omp-native-check-u5bozm9a-workspace/2026-05-04T09-15-00Z_01a1ebb0-1d2d-7a8d-a79d-8a5ca …[the rest of this output was dropped]
PASS: the harness's own loader answers about the installed session — sessionId '01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d', messageCount 3 (the file's own history holds 3), sessionFile 'the copy this check installed'
PASS: the loader's own message list is the file's model-visible history — 3 message(s): ['compactionSummary', 'user', 'assistant']
PASS: the unknown-id control arm reports no such session — Error: Session "01a1c338-dead-beef-0000-000000000000" not found.
Run `omp --resume` without an argument to pick from recent sessions, or `omp` to start a new one.
PASS: the load ran against a copy and left the installed file alone
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-omp-native-check-u5bozm9a/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-omp-native-check-u5bozm9a/omp-home PI_CODING_AGENT_SESSION_DIR=/tmp/ccc-omp-native-check-u5bozm9a/omp-home/sessions/-tmp …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-omp-native-check-u5bozm9a/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-omp-native-check-u5bozm9a/omp-home PI_CODING_AGENT_SESSION_DIR=/tmp/ccc-omp-native-check-u5bozm9a/omp-home/sessions/-tmp …[the rest of this output was dropped]
PASS: a real turn answered from the installed session and only appended — exit 0, session 3625 -> 7176 bytes; ran /home/azureuser/.local/share/mise/installs/oh-my-pi/latest/omp -p --resume 01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d -- Reply with exactly: OMP-NATIVE-CHECK-OK
the turn's own output (the target's answer, as the harness printed it):
    OMP-NATIVE-CHECK-OK

PASS: the turn's answer contains 'OMP-NATIVE-CHECK-OK' — OMP-NATIVE-CHECK-OK
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — 01a1ebb0-1d2d-7a8d-a79d-8a5cabdce49d is new among the session id(s) the home's own header lines record
scratch removed (/tmp/ccc-omp-native-check-u5bozm9a)
RESULT: PASS — 25 check(s), none failed
```

The control arm's own two lines are the harness's answer for an id nothing installed (`Error: Session
"…" not found.`), which is what makes the first arm an answer about *this* session rather than about a
process that started. The load ran against a copy in a data directory holding only the run's own seeded
configuration, and the installed file's digest is unchanged after both arms.

## 3. OMP → Claude Code through the pipeline and through Claude Code's own loader

The source is a copy of the committed OMP fixture
(`internal/ccc/testdata/fixtures/omp/--home-user-code-example-api--/01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4.jsonl`)
with its recorded working directory rewritten the same way. Exit code **0**:

```console
$ OMP_DIR=/tmp/omp-ev/omp-src bin/ccc continue \
    omp:01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4 --to claude --target-home /tmp/omp-ev/ev2 \
    --target-binary /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude
omp:01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4 → claude
  source:       /tmp/omp-ev/omp-src/--home-user-code-example-api--/01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4.jsonl
  read from:    source
  conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)
  boundary:     4114 byte(s) read
  records:      no line-record offsets claimed: the provider's own declarations do not make this source a transcript of appended line records
  converted:    claude-code-transcript-jsonl, 2695 bytes: 4 message(s), 1 tool call(s), 1 tool result(s), 0 boundary(ies), 0 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  omitted:      message_model_attribution ×3
  carried-with-caveat: unresolved_source_parents ×1
  installed:    /tmp/omp-ev/ev2/projects/-tmp-omp-ev-ws/97798bb9-c5bc-43a3-a75b-209cb8f35dfa.jsonl [hard-link]
  source file:  unchanged (sha256 907281aa10773d7f2ab9d49e4de85978dbcc7387982f7c853b1f960b1a9c7919, taken before and after the conversion)
  target id:    97798bb9-c5bc-43a3-a75b-209cb8f35dfa
  working dir:  /tmp/omp-ev/ws
  resume:       cd /tmp/omp-ev/ws && CLAUDE_CONFIG_DIR=/tmp/omp-ev/ev2 /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume 97798bb9-c5bc-43a3-a75b-209cb8f35dfa
  non-interactive resume: cd /tmp/omp-ev/ws && CLAUDE_CONFIG_DIR=/tmp/omp-ev/ev2 /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude -p --output-format json --resume 97798bb9-c5bc-43a3-a75b-209cb8f35dfa -- '<prompt>'
  cli_version:  2.1.326 (probe)
  structure:    verified (the inherited AgentsView Claude reader over the installed session)
  native load:  verified (the target harness's own loader (claude --bare --resume, with no model credentials in its environment, plus an unknown-id control))
                the harness's loader resolved the installed session and answered about it (subtype "success", terminal reason "api_error", result "Not logged in · Please run /login"); the same command with an unknown session id answered "No conversation found with session ID: 0e3f91b7-5d22-4420-8041-ac977258e11f". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-claude-load-2069736916/projects/-tmp-omp-ev-ws/97798bb9-c5bc-43a3-a75b-209cb8f35dfa.jsonl) with --bare and with ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL, ANTHROPIC_MODEL, ANTHROPIC_DEFAULT_HAIKU_MODEL, ANTHROPIC_DEFAULT_OPUS_MODEL, ANTHROPIC_DEFAULT_SONNET_MODEL, ANTHROPIC_SMALL_FAST_MODEL, CLAUDE_CODE_OAUTH_TOKEN, CLAUDE_CODE_USE_BEDROCK, CLAUDE_CODE_US… [the rest of this output was dropped]
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume (claude -p --resume))
                not requested (--verify load)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

Claude Code's loader **resolved the installed session** (it reached the model call and answered about the
session, failing only at model access: `terminal reason "api_error"`, `result "Not logged in · Please run
/login"` — this host holds no Claude credential, which is the load level's own boundary), and the
control arm reports no conversation. The same direction through the route's own native harness,
`python3 scripts/omp-native-check.py --direction omp-to-claude --claude
~/.local/share/ccc-native/claude-cli/node_modules/.bin/claude`, exit code **0**:

```console
repo:        /home/azureuser/repos/call-ccc
direction:   omp-to-claude (omp:01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4 → claude)
claude:      /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude (/home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude)
source:      internal/ccc/testdata/fixtures/omp/--home-user-code-example-api--/01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4.jsonl (copied to /tmp/ccc-omp-native-check-_3yep86x/omp-source/--home-user-code-example-api--/01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4.jsonl)
workspace:   /tmp/ccc-omp-native-check-_3yep86x/workspace -> Claude project dir -tmp-ccc-omp-native-check--3yep86x-workspace
target home: /tmp/ccc-omp-native-check-_3yep86x/claude-home
the source fixture's recorded working directory (/home/user/code/example-api) is rewritten to the workspace, so the session belongs to a path this run owns and the target's own loader can find what it installs

PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 27
PASS: the structural read-back verified the installed session
PASS: ccc continue exits 0 for a conversion whose verification ran — exit 0; stderr No conversation found with session ID: c89cabcd-40b8-4fa5-bcdc-3c3e5ff2653d
PASS: the installed session exists at the reported path — projects/-tmp-ccc-omp-native-check--3yep86x-workspace/f7604bca-6259-49af-813c-cfa1e0c617a5.jsonl [hard-link]
PASS: the installed transcript is filed under this run's own Claude project spelling of the workspace — projects/-tmp-ccc-omp-native-check--3yep86x-workspace/f7604bca-6259-49af-813c-cfa1e0c617a5.jsonl; this run's Claude spelling of /tmp/ccc-omp-native-check-_3yep86x/workspace is -tmp-ccc-omp-native-check--3yep86x-workspace
PASS: the installed file is one JSON object per line — 2875 bytes, 6 record(s)
PASS: the load check verified the installed session natively — the harness's loader resolved the installed session and answered about it (subtype "success", terminal reason "api_error", result "Not logged in · Please run /login"); the same command with an unknown session id answered "No conversation found with session ID: c89cabcd-40b8-4fa5-bcdc-3c3e5ff2653d". The check ran against a throw-away copy of the converted bytes (/tmp/ccc-claude-load-1824847430/proj …[the rest of this output was dropped]
PASS: Claude Code's own loader resolves the installed session — exit 1; session_id 'f7604bca-6259-49af-813c-cfa1e0c617a5'
PASS: the unknown-id control arm reports no conversation — No conversation found with session ID: 01a1c338-dead-beef-0000-000000000000
PASS: the load ran against a copy and left the installed file alone
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-omp-native-check-_3yep86x/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-omp-native-check-_3yep86x/claude-home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-omp-native-check-_3yep86x/workspace && CLAUDE_CONFIG_DIR=/tmp/ccc-omp-native-check-_3yep86x/claude-home /home/azureuser/.local/share/ccc-native/claude-cli/node_modules/.bin/claude -p --out …[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — f7604bca-6259-49af-813c-cfa1e0c617a5, not one of the 1 id(s) already in the home
scratch removed (/tmp/ccc-omp-native-check-_3yep86x)
RESULT: PASS — 18 check(s), none failed
```

`--direction omp-to-claude` needs no OMP binary at all: its source is a committed fixture read by the
inherited reader and its native half is Claude Code's loader. Run on a PATH without `claude`, the same
direction reports `SKIP: Claude Code's own loader resolves the installed session (claude on PATH) — claude
is not on PATH …` and ends `RESULT: INCOMPLETE — 15 check(s) ran, none failed`, exit code 2 — a partial
run is never quoted as a pass.

## 4. The accepted Pi route still verifies natively

The writer, the tree walk, the projected-parent rule and the installer are shared with increment 0005's Pi
route, so 0005's own check was re-run against this tree:
`python3 scripts/pi-native-check.py`, exit code **0**:

```console
PASS: the harness's own loader answers about the installed session — sessionId '01a1ebaf-519b-7b48-ae13-c9ca797bd963', messageCount 3
PASS: the loader's own message list is the file's model-visible history — loader ['compactionSummary', 'user', 'assistant'] vs file ['compactionSummary', 'user', 'assistant']
PASS: the unknown-id control arm reports no such session — No session found matching '01a1c335-dead-beef-0000-000000000000'
PASS: the load check ran against a copy and left the installed file alone
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-pi-native-check-bh21q1fq/workspace && PI_CODING_AGENT_DIR=/tmp/ccc-pi-native-check-bh21q1fq/pi-home PI_CODING_AGENT_SESSION_DIR=/tmp/ccc-pi-native-check-bh21q1fq/pi-home/sessions/--tmp-ccc …[the rest of this output was dropped]
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — 01a1ebaf-519b-7b48-ae13-c9ca797bd963 is new among the session id(s) the home's own header lines record
RESULT: PASS — 21 check(s), none failed
```

## 5. What travels, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| a Claude Code conversation with a tool call, its result, recorded reasoning and a compaction boundary | converted into an OMP session of `6 message(s)` with the call and its result (`carried 6 message(s), 1 call(s), 1 result(s)`, §1, §2), the family's tree walk from the file's own `parentId` values, and one compaction entry — the same `entries {'compaction': 1, 'message': 7, 'model_change': 1, 'role:assistant': 4, 'role:toolResult': 1, 'role:user': 2, 'session': 1, 'title': 1}` an OMP file the loader reads holds (§2) |
| the source read's own omissions | named, counted and per-category: `message_model_attribution ×4`, `pre_compaction_history ×4`, `unrecorded_token_usage ×4`, `tool_name_vocabulary ×1`, `unresolved_source_parents ×1`, `malformed_source_lines ×1`, and the two OMP-only caveats `target_model_binding ×1` and `title_slot_written ×1` (§1) |
| the model binding the format needs | the *target home's* own configured `<provider>/<model>`, asked of the harness itself: the report names that source (`model_provider_source`) and the pair the harness resolved for a new session of it (`model_provider_detail`), and the conversion writes it as the `model_change` record the first source record hangs off — never the source's model (Q-0006-03) |
| the per-assistant pair OMP's request builder indexes | `4 assistant message(s), 0 without the pair` in the installed file's own bytes (§2): each carries the source's model and the target home's own provider (Q-0006-02) |
| the file's first line | `first line 256 byte(s) including its line feed, type title` (§2) — the harness's own fixed-width slot, written with an empty title (Q-0006-01) |
| the directory the session is filed under | OMP's own spelling of the session's recorded working directory, computed by the check and compared with the file's real location: `-tmp-ccc-omp-native-check-u5bozm9a-workspace` for a workspace under `/tmp`, `--home-user-code-example-api--` for the fixture's `/home/user/…` path (§1, §2, §3) |
| an OMP conversation as a **source** | converted into a Claude Code transcript (`4 message(s), 1 call(s), 1 result(s)`, 27 loss categories, §3), read back `verified` by the inherited Claude reader, and opened by **Claude Code's own loader** (`session_id 'f7604bca-…'`, control arm answered) |
| the converted history as the **answer** of a real turn in the installed OMP session | the recorded turn's answer holds the words the run asked for — `OMP-NATIVE-CHECK-OK` (§2), and the CLI's own resume verification of the same route recorded `OMP-RESUME-OK` (§1) |
| the source file during and after every run | `source file: unchanged (sha256 …, taken before and after the conversion)` in both CLI runs (§1, §3) and `PASS: the source fixture tree is unchanged — 1 file(s) digested before and after` in both native runs (§2, §3) |
| a session the target home already held | untouched: `PASS: the planted unrelated session is unchanged` and `PASS: every session the home already held is unchanged` (§2, §3), with `PASS: the conversion wrote no id the home already held` |
| the harness's own loader against a **copy** | `the load ran against a copy and left the installed file alone` in both directions (§2, §3) — measured: a load appends its own `custom`/`session_exit` record, so the installed bytes are never the ones a loader opens |
| the accepted Pi route after the shared change | `RESULT: PASS — 21 check(s), none failed` (§4), the same loader answers 0005's record holds |

The row that rests on the target model's own words is the `OMP-NATIVE-CHECK-OK` one: it is the block the
harness printed under `the turn's own output (the target's answer, as the harness printed it):`, quoted
verbatim in §2, and the case that keeps a record from quoting an answer it did not make reads exactly
that block (`scripts/tests/test_evidence_claims.py`, which this record is registered with).

## 6. Gate, tests, docs and bookkeeping

`python3 scripts/gate.py`, exit code 0:

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 268 tests in 5.888s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 30 project-owned prefix(es); files seen by git: 5688 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 141 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

After the simplify pass's accepted edits — the native checks' shared mechanisms moved into
`scripts/native_check_common.py`, with no behaviour change intended — this block was re-derived by
re-running the gate on the applied tree (`gofmt 3611 file(s)`, the tree's tracked `*.go` count; the
recorded run's `3602` was made before this increment's nine `.go` files were staged), and the harness
runs this record quotes were re-derived too, with the same verdicts: `python3 scripts/pi-native-check.py`
→ `RESULT: PASS — 21 check(s), none failed` (exit 0); `python3 scripts/omp-native-check.py --direction
omp-to-claude --claude ~/.local/share/ccc-native/claude-cli/node_modules/.bin/claude` → `RESULT: PASS —
18 check(s), none failed` (exit 0); the same direction with no `claude` on `PATH` → `RESULT: INCOMPLETE —
15 check(s) ran, none failed` (exit 2); and `python3 scripts/omp-native-check.py --resume-prompt 'Reply
with exactly: OMP-NATIVE-CHECK-OK' --expect OMP-NATIVE-CHECK-OK` → `RESULT: PASS — 25 check(s), none
failed` (exit 0), whose own turn grew the session `3625 -> 7179 bytes` in that run (the harness's
appended bytes differ per run; §2's `7176` is its own run's number). Every check count and verdict the
record holds is unchanged.

The fix round that followed the review re-derived this block too (the gate run above; its `selftest` count
is that round's `268` — its own new Python case — with every other line unchanged) and re-ran all four of
those on its own tree, with the same commands and the same verdicts (21 / 18 / 15 / 25; exits 0 / 0 / 2 /
0), and added the measurement that round's own finding asked for: `python3 scripts/omp-native-check.py --direction omp-to-claude --claude
~/.local/share/ccc-native/claude-cli/node_modules/.bin/claude --resume-prompt 'Reply with exactly:
OMP-NATIVE-CHECK-PROBE' --expect OMP-NATIVE-CHECK-PROBE` → `RESULT: FAIL — 3 check(s) failed: ccc continue
exits 0 for a conversion whose verification ran, the resume check verified one real turn through the
printed invocation, the turn's answer contains 'OMP-NATIVE-CHECK-PROBE'` (exit 1). In that direction
`--resume-prompt` now drives the conversion CLI's own resume check (`--verify resume`) rather than
recording a skip, and this host cannot answer the turn (`Not logged in · Please run /login`, §7), so a
requested turn that does not verify fails the run instead of leaving it a pass.

### The CI repair round

The hosted `gate (windows-latest)` job went red on this branch's head (`f0322e3`, run `44203425094`):
`FAIL     selftest REQUIRED but was FAIL: scripts/tests -> rc=1`, `FAILED (failures=4)` — the two rows of
`scripts/tests/test_omp_native_check.py`'s `ProjectDirCases` that name a whole resolved path
(`omp_project_dir('/srv/work/project', …)` answered `--D--srv-work-project--` where the table pinned
`--srv-work-project--`) and, as their consequence, `test_gate.py`'s two nested-run cases (the child run
they assert green ended with the same two failures and exit 1; a failing case in this suite therefore
fails the gate twice on that host, by design).

The cause was the table's own spelling, not the rule: `omp_project_dir` resolves the working directory
(`os.path.abspath`) and each base inside `omp_within` the same way, so on a platform whose rooted paths
carry a volume the CI's own process answers (Windows, `GetFullPathName`) a base left as `/tmp` has no
relative spelling to the resolved path at all and the absolute branch spells the drive as part of its
name. The table's bases, its working directories and the outside row's expectation are now the host's own
resolved spellings (`abs_host_path` and `outside_spelling`, mirroring the Go twin's `absHostPath` and its
`outsideWant`), so the three branches are reachable and the outside row pins the spelling that host's rule
actually produces; the shortened rows' expectations are unchanged words. Nothing shipped changed
(`scripts/omp-native-check.py` is byte-identical to the fix round's), and the suite's case count stays
`268` — the same 17 cases in that file, three of them with host-shaped rows.

The mutation that names the repaired rule, executed and restored (the file hashed before and after; the
restore matched `305a903f…`): `abs_host_path` made to answer its argument unchanged, i.e. the pre-fix
shape the hosted job ran —

| Rule | Line disabled | Observed |
| --- | --- | --- |
| the case table is the host's own resolution of the rule's inputs (`scripts/tests/test_omp_native_check.py`) | `abs_host_path` returns `path` | under a host whose `os.path` is `ntpath` with a drive for a rooted path (`GetFullPathName`'s own answer: `abspath('/srv/work/project')` = `D:\srv\work\project`), `test_the_three_branches [a directory outside both]` FAIL — `--D--srv-work-project-- != --srv-work-project--` — and `test_the_spelling_is_not_pis_where_it_differs` FAIL: the two failures the hosted job reported, reproduced in-process |
| the outside row pins the drive colon on the platform that has one | `outside_spelling`'s replacement drops `:` (`re.sub(r"[/\\]", …)`) | same two cases FAIL under that host; on this host both pass, because a POSIX literal carries no colon to spell — the colon's own row is the host-independent `omp_separator_spelling` case, which fails here too |

`python3 scripts/gate.py`, exit code 0:

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 268 tests in 6.065s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 30 project-owned prefix(es); files seen by git: 5688 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 141 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

Every other number this record pins is unchanged by the round, which is the expected outcome of a repair
confined to one case table: the native checks do not read it, and no converted byte, count, loss category
or loader answer depends on it. `AGENTS.md` is not edited: the round changes no layout, gate step,
command, read order or agent rule — the rule it repairs is the one `AGENTS.md` and `docs/conventions.md`
already state (no case may hold only on the host or `PATH` it happens to run on). No goal file is moved:
0006 is still the increment under review.

### The review round that followed the CI repair

The next review round (review head `12be17c`) named three defects; all three were fixed on the same tree,
and each fix's own case was shown to redden without it (the mutation executed and restored, each file
hashed after its restore; the gate and the native run below ran on those bytes).

- **A multiline `apiKey` value was copied whole** (`scripts/omp-native-check.py`). `_seed_block` replaced
  only the line a value opened on, so a provider whose key is a block scalar (`apiKey: |-`) had its
  credential's continuation lines written into the scratch home, the loader home and a `--keep` tree. The
  replacement now drops every line deeper than the field -- the whole value -- until the next line the
  block owns. The new case `SeedCases.test_a_multiline_key_value_is_removed_whole` covers the block,
  folded and plain-continuation spellings and asserts none of the values reaches a written file.
- **The model probe asked a differently configured harness than the printed resume command**
  (`internal/ccc/omp_verify.go`). `ProbeOMPModel` ran through the load check's runner, which removes the
  provider variables -- `LITELLM_BASE_URL`/`LITELLM_API_KEY` among them -- while the increment's own
  measured format table makes exactly that configuration the way an environment-configured home resolves
  a model. The probe now builds its own environment (`ompProbeEnvironment`: the caller's own, with the
  data directory set, nothing removed) and drives a seam that takes it, so a case can pin the policy
  (`TestProbeOMPModelAsksTheHarnessItself`).
- **An all-fallback conversion explained nothing about the models it supplied**
  (`internal/ccc/pi_target.go`). `losses()` selected the OMP `message_model_attribution` explanation by the
  travelled-model count alone, so a conversion whose every assistant model came from the target home
  reported only "the source recorded no model on any message" beside records stating that home's ids. The
  explanation is now selected by the count the entry carries (`modelAttributionCount`), and
  `TestOMPSessionExplainsTheModelsItSuppliedForEveryRecord` pins the all-fallback output, its count and its
  detail; the OMP detail's own sentence now says the model travels verbatim *where the source recorded
  one*, which is what is true when none did.

The mutations that name the three repaired rules, executed and restored (each file hashed after its
restore; the gate and the native run below ran on those bytes):

| Rule | Line disabled | Observed |
| --- | --- | --- |
| a seeded provider's `apiKey` value is removed whole (`scripts/omp-native-check.py`, sha256 `c025030b…` after the restore) | the drop after the field's replacement removed, i.e. `_seed_block` keeps the value's continuation lines | `test_a_multiline_key_value_is_removed_whole` FAIL — `'sk-block-scalar-value-that-must-not-be-written' unexpectedly found in` the written `models.yml`, its continuation lines printed under `apiKey: ccc-native-check` |
| the model probe asks under the caller's own environment (`internal/ccc/omp_verify.go`, sha256 `2ec1ce57…` after the restore) | `ompProbeEnvironment` built with the load check's denylist (`ompEnvironment(home, "", ompLoadEnvDenied)`) | `TestProbeOMPModelAsksTheHarnessItself` FAIL — `the probe asked without "LITELLM_BASE_URL=http://127.0.0.1:4141/v1"` and the same for `LITELLM_API_KEY`, the captured environment naming neither |
| the attribution caveat is selected by the count it carries (`internal/ccc/pi_target.go`, sha256 `5c3ec11d…` after the restore) | the condition restored to `b.lossModels > 0` | `TestOMPSessionExplainsTheModelsItSuppliedForEveryRecord` FAIL — `detail = "the source recorded no model on any message", want it to explain the substituted ids` |

`python3 scripts/gate.py`, exit code 0:

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 269 tests in 5.993s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 30 project-owned prefix(es); files seen by git: 5688 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 141 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The load half of the OMP native check was re-run on the same tree (`python3 scripts/omp-native-check.py`,
no `--resume-prompt`, so no model turn): `RESULT: PASS — 23 check(s), none failed` (exit 0). Its seeded line
is the one §2 records -- `seeded provider(s) ghc-proxy, ghc-proxy-chat from /home/azureuser/.omp/agent; …
apiKey values replaced by 'ccc-native-check', headers dropped, modelRoles.default ghc-proxy/gpt-5.6-sol`
-- and the harness's own loader verified the installed session
(`sessionId "01a1ec05-c625-7ccc-aad7-3d8b0763ad1d"`, `messageCount 3`) beside the unknown-id control arm,
with every digest check around it passing. The direction's source is the committed Claude fixture, so the
run needed no Claude CLI installed. A repeat of that run with `--keep` read the probe's own answer out of
the installed session's bytes: its `model_change` record states `ghc-proxy/gpt-5.6-sol`, the seeded home's
own `modelRoles.default` -- the binding is the harness's answer, asked under the caller's own environment.

No owner-only default was taken by the round: it re-applies rules the record already states (no credential
is copied; the probe asks the harness the printed command resumes; a substituted model is disclosed), so
`docs/decisions/OPEN-QUESTIONS.md` gains no entry. `AGENTS.md` is not edited: the round changes no layout,
gate step, command, read order or agent rule. No goal file is moved: 0006 is still the increment under
review.

The counts the increment is pinned by:

| Number | Where it comes from |
| --- | --- |
| `25 check(s), none failed` / `18 check(s), none failed` | the two native runs' verdicts (§2, §3), each exit 0 |
| `21 check(s), none failed` | the Pi route's check, re-run after the shared change (§4) |
| `carried 6 message(s), 1 call(s), 1 result(s); loss categories named: 31` (Claude → OMP) and `carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 27` (OMP → Claude) | the two native runs' own conversion lines (§2, §3); the CLI's §1/§3 runs report the same carried counts |
| `entries {'compaction': 1, 'message': 7, 'model_change': 1, 'role:assistant': 4, 'role:toolResult': 1, 'role:user': 2, 'session': 1, 'title': 1}` | the installed OMP file's own bytes (§2) |
| `messageCount 3` with `['compactionSummary', 'user', 'assistant']` | OMP's own loader's answers, twice (§1, §2), against a file holding seven message entries — the loader builds context from the walk the file's own `parentId` values make, so the pre-compaction records are not in it |
| `session 3625 -> 7176 bytes` / `3595 to 7097 bytes` | the two real turns: the session grew and its pre-existing bytes stayed a prefix (§1, §2) |
| `269` | the harness's own cases (`python3 -m unittest discover -s scripts/tests`), up from 251: `scripts/tests/test_omp_native_check.py` holds the 18 new ones — its 16 at implementation, the fix round's model-level credential case and the review round's multiline-key case — and the gate's own `selftest` step ran the same suite (`Ran 269 tests in 5.993s (rc=0)`) |

Documentation and bookkeeping this increment carries: `AGENTS.md` (the route list, the writer/installer/
verifier files, the OMP native harness, and the OMP rules — its own project spelling, the 256-byte title
slot, the required per-assistant `model`+`provider`, the session binding being the harness's own answer);
`docs/conventions.md` (the layout, the install/discoverability rule's OMP branch, the fixture layout);
`docs/harness.md` (this script's entry and why it is not a gate step); `README.md` (the quickstart's
`--to omp` and its printed resume line); `scripts/upstream-import.py` and `scripts/upstream-manifest.txt`
(the new owned script); `docs/decisions/OPEN-QUESTIONS.md` (Q-0006-01 … Q-0006-04); goal 0005 archived to
`docs/goals/archive/` with its index row and every link to it updated.

## 7. Limits, and what this record does not claim

- **The Claude half of §3 is a load, not a turn.** This host holds no Claude credential, so Claude Code's
  loader resolved the session and failed at the model call (`result "Not logged in · Please run /login"`).
  The contract's clause is "native load/resume"; the load is verified, and no turn is claimed. Asking for
  one in that direction (`--resume-prompt`) now runs the conversion CLI's own resume check and reports it
  not-verified with the harness's own words, ending the run non-zero (§6's fix-round run) rather than
  passing with the turn skipped.
- **The OMP half's turn ran under the host's own loopback proxy**, in a data directory the check seeded
  from the host's own OMP home with the key's *value* replaced by a placeholder. A host with no loopback
  provider (and no `LITELLM_BASE_URL`/`--provider-base-url`) reports a named skip for the turn; the load
  half runs regardless, since its loader data directory carries this run's own provider definition (the
  placeholder `apiKey` value this run writes, never a credential of the caller's).
- **`messageCount` is the loader's own count**, not the file's `message` entry count (§6): seven message
  entries, three messages on the file's own walk after its latest compaction entry. The check compares the
  loader's answer with the roles it computes from the file's bytes, which is the strongest comparison two
  independent readings of the same file allow.
- **The recorded working directory must exist on the host that verifies the load** (Q-0006-04). The check
  rewrites the fixture's directory to its own workspace for exactly that reason, and a run against a file
  naming a directory this host does not have reports not-verified with the harness's own sentence rather
  than running the loader somewhere else.
- **A generated filename is not compatibility** (the goal's own `Stop and report`): every "resumable"
  claim above is `verification.load` or `verification.resume` coming back `verified`, from the harness's
  own loader and, for the two OMP turns, the harness's own printed answer.
- **The repaired table's Windows answer is pinned by construction here, not by a run on Windows.** This
  host is POSIX, so the repair round re-ran the file against a host whose `os.path` is `ntpath` with a
  drive for a rooted path (`GetFullPathName`'s own answer) — a throw-away harness outside the tree, never
  committed — and read the same two failures the hosted job reported out of the pre-fix table and no
  failure out of the repaired one. The hosted `windows-latest` job is the real Windows host; its next run
  on that tree is what closes the round.
