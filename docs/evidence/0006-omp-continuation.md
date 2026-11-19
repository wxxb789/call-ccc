# 0006 — Continue conversations into and out of Oh My Pi: evidence

Contract: [`docs/goals/archive/0006-omp-continuation.md`](../goals/archive/0006-omp-continuation.md)
(merged onto `main` as `338df94`; archived to `docs/goals/archive/`, per `AGENTS.md`).
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

### The third review round

The findings file for this review round (head `ef31bea89aab692b88ad6308809e15de22a4e95b`) named one P1
defect: **the seeder could copy a credential-bearing endpoint the credential filter never read**, in two
spellings, and asked for the class rather than the spellings -- "decode supported YAML values before
checking them and reject unsupported mapping/value forms before copying any provider block". The two named
spellings were executed on the pre-fix tree before any edit (the review had inspected them statically and
said so): `seed_omp_home` seeded the definition and wrote the credential-bearing line into the scratch
`models.yml` -- the file the loader arm copies again and a `--keep` tree retains -- for

- a model entry in flow form, `- {id: demo, baseUrl: "http://…/v1?api_key=…"}`: the textual reader read the
  line as an entry whose key is `{id`, so `block_has_credential_endpoint` never examined the nested
  `baseUrl` and `_seed_block` kept the line; and
- a double-quoted endpoint whose query delimiter is a YAML escape,
  `baseUrl: "http://127.0.0.1:4141/v1\u003fapi_key=…"`: `_scalar` removed the quotes without decoding them,
  so the raw value held no literal `?` for `credential_free_url` to find while the harness's own reader
  reads one.

The class was then enumerated against a YAML reader's own answers (`go.yaml.in/yaml/v3`, driven from `/tmp`,
outside this repository), each additional spelling executed against the pre-fix tree, and each shown to copy
its credential before the fix: a key under a tag, anchor or alias (`&a apiKey: …`, `!!str headers: …` --
YAML reads the field bare), a model-level `baseUrl: *u` whose anchor's credential-bearing URL sits under
another key (copied, because a line whose key is not `baseUrl` is not scanned as an endpoint), a block
scalar endpoint (`baseUrl: |-` plus a content line holding `?api_key=…` -- a content line is not an entry
this reader recognizes), a nested sequence marker (`- - apiKey: …` reads as the key `- apiKey`), a quoted
value continued onto the next line (its first line holds no `?`; YAML folds the value into the
credential-bearing URL), an empty key, and content after a quoted value's closing quote.

What changed (`scripts/omp-native-check.py`):

- `_scalar` now **decodes** a quoted value the way the harness's reader reads it before any check sees it:
  JSON's escape set for a double-quoted scalar (`\u003f` is the `?` the credential check must find; an
  escape outside the set is refused, never guessed at) and `''` for a single-quoted one; a quoted value
  that does not close on its line, or that carries content after its closing quote, is refused. A value
  that opens any other node form -- a flow collection, a tag/anchor/alias, a block scalar, a nested block
  structure -- is refused too: each can spell fields of its own.
- `_mapping_entry` refuses a line whose own start opens a flow collection (`- {id: …}`), a node property
  (`&a apiKey:`), a nested sequence (`- - apiKey:`), an empty key (`: …`), or a quoted key it cannot
  decode (the rules for YAML's explicit `?` form and a quoted key that does not close are unchanged).
- `_seed_block` reads every value of a field the block keeps (`_require_readable_value`) before copying it;
  the two names whose whole value it replaces or drops are not read that way, because no part of either is
  copied.
- Each refusal reaches the run's own output through the existing `_UnseedableBlock` path, so the report
  names the provider and the form it could not read.

Five new cases in `scripts/tests/test_omp_native_check.py` pin these rules -- `SeedCases
.test_a_flow_spelled_entry_is_refused_before_anything_is_copied`,
`.test_a_quoted_endpoint_is_decoded_before_the_credential_check`,
`.test_a_key_form_the_reader_mangles_is_refused_whole`,
`.test_a_value_form_the_reader_cannot_follow_is_refused_whole` and `DefaultRoleCases
.test_a_single_quoted_role_is_decoded_the_way_yaml_reads_it` -- and two round-2 fixtures whose key rules a
later value rule could mask now carry their credential as a plain value, so each rule reddens on its own
when reverted.

The rules this surface claims, each proved by one executed mutation (the line reverted, the pinning case
run, the bytes restored -- `scripts/omp-native-check.py` sha256
`815832c8e2123d6aac56c5d2c39ff02fd4f5b7397386f61489050c58813c7e06` after every restore; no mutation
survived its case; the `(new)` rows are this round's rules, the others the seeder's pre-existing ones):

| Rule | Line disabled | Observed |
| --- | --- | --- |
| a flow-spelled entry is refused (new) | the `{[` branch of `_mapping_entry` | `test_a_flow_spelled_entry…` FAIL -- `Lists differ: ['block-spelled-endpoint', 'flow-spelled-endpoint'] != ['block-spelled-endpoint']` |
| a key under a tag/anchor/alias is refused (new) | the `&*!` branch of `_mapping_entry` | `test_a_key_form_the_reader_mangles…` FAIL -- `Lists differ: ['anchored-key', 'canonical-endpoint', 'tagged-header-key'] != ['canonical-endpoint']` |
| a nested sequence marker is refused (new) | the nested-`-` branch of `_mapping_entry` | same case FAIL -- `Lists differ: ['canonical-endpoint', 'nested-marker-key'] != ['canonical-endpoint']` |
| an empty key is refused (new) | the empty-key branch of `_mapping_entry` | same case FAIL -- `Lists differ: ['canonical-endpoint', 'empty-key'] != ['canonical-endpoint']` |
| a quoted value's escapes are decoded (new) | `json.loads` replaced by the old quote-strip | `test_a_quoted_endpoint_is_decoded…` FAIL -- `Lists differ: ['escaped-model-endpoint', 'escaped-provider-endpoint'] != ['decoded-endpoint']` |
| a quoted value's undecodable escapes are refused (new) | `except json.JSONDecodeError` falling back to the raw text | `test_a_value_form_the_reader_cannot_follow…` FAIL -- `Lists differ: ['canonical-endpoint', 'undecodable-quoted-value'] != ['canonical-endpoint']` |
| a quoted value that does not close on its line is refused (new) | the `token is None` branch returning the raw text | same case FAIL -- `Lists differ: ['canonical-endpoint', 'continued-quote-endpoint'] != ['canonical-endpoint']` |
| content after a quoted value's closing quote is refused (new) | the trailing-content branch | same case FAIL -- `Lists differ: ['canonical-endpoint', 'trailing-content-endpoint'] != ['canonical-endpoint']` |
| a non-scalar value form is refused (new) | the value-form branch of `_scalar` | same case FAIL -- `Lists differ: ['aliased-endpoint', 'block-scalar-endpoint…'] != ['canonical-endpoint']` |
| every value a block keeps is read before copying (new) | the `_require_readable_value(value)` call | same case FAIL -- `Lists differ: ['canonical-endpoint', 'flow-value-endpoint', 'undecodable-quoted-value'] != ['canonical-endpoint']` |
| a single-quoted value's doubled quote is decoded (new) | the `''` -> `'` fold | `test_a_single_quoted_role…` FAIL -- `"proxy/model''s" != "proxy/model's"` |
| an `apiKey` value is replaced by the placeholder | the replacement branch of `_seed_block` | `test_only_a_loopback_provider_survives…` FAIL -- `'sk-loopback-value-that-must-not-be-written' unexpectedly found in` the written `models.yml` |
| every `headers` entry is dropped whole | the drop branch of `_seed_block` | `test_quoted_and_spaced_key_spellings…` FAIL -- `'Bearer a-quoted-header-value-that-must-not-be-written' unexpectedly found in` the written `models.yml`, under a kept `"headers":` line |
| a host provider whose endpoint is not loopback is never seeded | the `loopback_url(base_url)` branch | `test_only_a_loopback_provider_survives…` FAIL -- `Lists differ: ['loopback-proxy', 'remote-endpoint'] != ['loopback-proxy']` |
| `--provider-base-url` must be credential-free and loopback | both branches of the caller-provider check | `test_a_non_loopback_endpoint_is_never_defined…` FAIL -- `'litellm' unexpectedly found in ['litellm', 'loopback-proxy']` |
| an endpoint URL carrying a credential is refused whole | the `block_has_credential_endpoint` branch | `test_a_model_level_endpoint_with_a_credential…` FAIL -- `Lists differ: ['model-level-endpoint'] != []` |
| keys are compared case-insensitively | `key.strip().lower()` -> `key.strip()` | `test_quoted_and_spaced_key_spellings…` FAIL -- `'sk-double-quoted-key-that-must-not-be-written' unexpectedly found in` the written `models.yml`, under a kept `"apiKey":` line |
| YAML's explicit `?` key form is refused, and said so | the explicit-`?` branch of `_mapping_entry` | `test_a_key_form_the_reader_cannot_decode…` FAIL -- `False is not true : the explicit-key refusal should name YAML's '?' form: ["explicit-key (it opens a mapping entry with an empty key…)"]` |
| a multiline `apiKey` value is removed whole | the drop after the field's replacement | `test_a_multiline_key_value_is_removed_whole` FAIL -- `'sk-block-scalar-value-that-must-not-be-written' unexpectedly found in` the written `models.yml`, under `apiKey: ccc-native-check` |
| a quoted key whose escapes cannot be decoded is refused | the key-decode `except json.JSONDecodeError` falling back to the raw text | `test_a_key_form_the_reader_cannot_decode…` FAIL -- `Lists differ: ['canonical-key', 'escaped-key'] != ['canonical-key']` |

`python3 scripts/gate.py`, exit code 0:

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 285 tests in 5.941s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 30 project-owned prefix(es); files seen by git: 5688 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 141 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The OMP native check was re-run on the same tree (`python3 scripts/omp-native-check.py`, no
`--resume-prompt`, so no model turn): `RESULT: PASS — 23 check(s), none failed` (exit 0). Its seeded line is
unchanged from §2 -- `seeded provider(s) ghc-proxy, ghc-proxy-chat from /home/azureuser/.omp/agent; left out:
tencent-litellm (its endpoint … is not loopback…); apiKey values replaced by 'ccc-native-check', headers
dropped, modelRoles.default ghc-proxy/gpt-5.6-sol` -- which is also the measurement that this host's own
home (block-style lists, plain scalars) still seeds through the stricter reader; the harness's own loader
verified the installed session (`sessionId "01a1ec2a-7e98-777b-96cc-ed5be4ffc281"`, `messageCount 3`, the
loader's list `['compactionSummary', 'user', 'assistant']`) beside the unknown-id control arm, and every
digest check around it passed.

Bookkeeping: no carried file changed (the fix touches an owned script, its tests and docs only), so
`docs/upstream.md`, `scripts/upstream-manifest.txt` and the parity step's inputs are untouched; `AGENTS.md`
is not edited (no layout, gate step, command, read order or agent rule changes -- the seeder's refusal rule
is stated in `docs/harness.md` and the script's own docstrings, where it already lived); no owner-only
question arose, so `docs/decisions/OPEN-QUESTIONS.md` gains no entry; no goal file is moved (0006 is still
the increment under review).

### The fourth review round

Round 4's behavioural finding was again the seeder's credential policy, in the shape a plain scalar takes
when it continues on a later line: `baseUrl: http://localhost:4141/v1` followed by an indented
`?api_key=…` is one folded value to any YAML reader, while the seeder's line reader saw only the first
line's endpoint and copied the continuation verbatim into its scratch home, its loader copy and a
`--keep` tree. The folded and next-line shapes were checked against a YAML reader before the fix
(`gopkg.in/yaml.v3` from the module graph the inherited tree carries, run against the shapes the new
cases seed): the continuation of a field, of a sequence item's own scalar, a continuation line that
itself opens with `- ` (which folds into the value too), a value spelled under its field's name on the
next line, and a block-scalar sequence item. `_seed_block` now carries continuation state per line --
after any plain scalar value it read, a later line indented past the key column that value belongs to (or
past a sequence item's own marker) is refused as a second line of that value -- and refuses any line that
is neither an entry nor a sequence item, while a sequence item's own text is decoded like every entry
value. The refusal rule in the script's docstrings and `docs/harness.md` now names those forms; the two
new cases in `scripts/tests/test_omp_native_check.py` pin them beside a canonical provider that must
still seed, and the 11 seeder rules this round touched were each reverted once, every revert making its
pinning case fail (`11 caught`), with the script restored byte for byte (sha256 `d304c451…` before and
after).

The second finding was this record's own limit, not the check's behaviour: §7 had promised that a host
with no loopback provider "reports a named skip for the turn" and that "the load half runs regardless".
Measured on this host with `PI_CODING_AGENT_DIR` pointed at a home holding one remote provider
(`python3 scripts/omp-native-check.py`, exit `1`):

```console
SKIP: an OMP provider definition was seeded into the run's own scratch home — no provider survived the seed: remote-endpoint (its endpoint https://api.example.com/v1 is not loopback: …)
SKIP: the seeded home selects a model role a real turn can use — no model role was written, so this home answers no model: a turn (--resume-prompt) has nothing to run under
FAIL: ccc continue exits 0 for a conversion whose verification ran — exit 1; stderr No default model selected. Use /login, set an API key environment variable, or select a local model with /model or --model.
FAIL: every assistant record carries a non-empty model and provider — 4 assistant message(s), 4 without the pair
FAIL: the load check verified the installed session natively — the converted session names no model and no provider in its own records, and this harness refuses to start in a data directory that defines no model: the load could not be verified against a home built for it
FAIL: the harness's own loader answers about the installed session — … SessionModelRestoreError: Could not res…
FAIL: the loader's own message list is the file's model-visible history — loader [] vs file ['compactionSummary', 'user', 'assistant']
RESULT: FAIL — 5 check(s) failed: ccc continue exits 0 for a conversion whose verification ran, every assistant record carries a non-empty model and provider, the load check verified the installed session natively, the harness's own loader answers about the installed session, the loader's own message list is the file's model-visible history
```

so §7 now states that limit as it was measured: a data directory that resolves a model is the
prerequisite the conversion's binding (and so the load half) needs, and the run ends failed -- a requested
turn through `record_unavailable_requested_turn()`, never a skip. The check was re-run on the host's own
home on the fixed tree (no `--resume-prompt`): `RESULT: PASS — 23 check(s), none failed` (exit 0), with the
seeded line unchanged from §2 (`seeded provider(s) ghc-proxy, ghc-proxy-chat from /home/azureuser/.omp/agent;
left out: tencent-litellm (its endpoint … is not loopback…)`, `modelRoles.default ghc-proxy/gpt-5.6-sol`),
the load check verified, and the harness's own loader answering `messageCount 3` beside the unknown-id
control arm.

bookkeeping for round 4: no carried file changed (an owned script, its tests and docs), so
`docs/upstream.md`, `scripts/upstream-manifest.txt` and the parity step's inputs are untouched; `AGENTS.md`
is not edited (no layout, gate step, command, read order or agent rule changes -- the refusal rule lives in
`docs/harness.md` and the script's docstrings); no owner-only question arose, so
`docs/decisions/OPEN-QUESTIONS.md` gains no entry; no goal file is moved (0006 is still the increment under
review); the gate's own steps and counts are in the table below.

### The fifth review round

Round 5's finding was a model-attribution hole on the writer's carried-call path
(`internal/ccc/pi_target.go`): `writeMessage`'s whitespace branch sends an assistant record whose dialogue
is whitespace alone to `writeCarriedCalls`, and that function built its entry's model from
`entryModel(Message{})` -- discarding the record -- so an OMP conversion stated the target home's
configured id on a record whose own model the read had answered (counting it as a substitution), while the
Pi route omitted the member entirely. The entry now states `entryModel(message)`, the answer
`writeAssistantMessage` already states: the model the read answered for the record whose calls these are,
with the family's fallback only where it answered none. The function's own doc and the
`synthesized_tool_call_messages` detail now name the shapes this path actually serves -- the record's
*role* having no target message was the old claim, which the whitespace branch and the user-with-calls path
both contradict -- and the report's line for that count names the whitespace-only dialogue among the
records it comes from. `TestOMPSessionKeepsTheSourcesModelOnAWhitespaceOnlyCallRecord` pins the two model
arms on two whitespace-only records (one with a model, one without) in both families' files, beside the
call-travel, no-dialogue and count/detail assertions on the same entries.

The mutations that name the rules this round's case pins (each disabled once, the case watched fail,
`internal/ccc/pi_target.go` restored byte for byte; sha256
`3655d6a3bd3b20566fa9bef0ee46f12cbd42da3a3632dfb1f4c62db58ba6106b` and `internal/ccc/omp_target_test.go`
sha256 `726889eb50a2ddf0a173a57b6b1279d43db54ffbc6f3532cc9f8a8c3e20e2a9f` after the restores):

| Rule | Line disabled | Observed |
| --- | --- | --- |
| the entry states the model the read answered for its record | `b.entryModel(message)` reverted to the reviewed `b.entryModel(Message{})` | `TestOMPSessionKeepsTheSourcesModelOnAWhitespaceOnlyCallRecord` FAIL -- `assistant models = 0 carried from the source, 2 supplied by the target home; want one of each`, and the Pi file holds `0 model member(s)` where the source recorded one |
| the family's fallback stands in only where the read answered none | the model field replaced by `b.shape.assistantModelFallback` unconditionally | the same case FAIL -- `assistant models = 0 carried from the source, 2 supplied by the target home`, `message_model_attribution count = 0, want 2`, and the Pi file holds `0 model member(s)` |
| the entry carries the record's calls | the call-block loop emptied (the entry written with no blocks) | the same case FAIL -- `the converted session does not carry the call the whitespace-only record made`, the file's entries printed |
| the entry holds no dialogue | the record's own dialogue written as a leading text block | the same case FAIL -- `line 4's assistant record holds the record's dialogue text "  \n  "` (and line 5's `"\n"`) |

Beside the pins, a throwaway harness (not committed) converted all ten committed fixtures with both
writers and compared each converted file with the writer's random identities held fixed (the session id
and every entry id are fresh per conversion, so the raw bytes differ between any two runs): every fixture's
digest under the fixed writer equals its digest with the changed line reverted to HEAD's, and every
conversion reports `synthesized_tool_call_messages` = 0 -- the changed line is not reached by any committed
fixture, so the recorded runs' own numbers stand. The OMP native check was re-run on the host's own home as
well (`python3 scripts/omp-native-check.py`, exit 0, `RESULT: PASS — 23 check(s), none failed`), its
conversion line and installed-file shape the ones §2 records (`carried 6 message(s), 1 call(s), 1
result(s); loss categories named: 31`; `entries {'compaction': 1, 'message': 7, 'model_change': 1,
'role:assistant': 4, 'role:toolResult': 1, 'role:user': 2, 'session': 1, 'title': 1}`; `4 assistant
message(s), 0 without the pair`; `messageCount 3`).

`python3 scripts/gate.py` was re-run on this round's tree and printed `GATE: PASS` (exit 0), its steps
reporting the same verdicts as §6's block above (`gofmt 3611 file(s)`, `build`/`vet` rc=0, `test` `3
package(s) ok`, `selftest Ran 287 tests`, `parity` carried 5553 files identical, `secrets` no leaks in 141
files); this round adds one Go case (`internal/ccc` 298 -> 299 test functions by `go test -list`; none
removed) and no Python case, so every recorded number below and above is unchanged.

The counts the increment is pinned by:

| Number | Where it comes from |
| --- | --- |
| `25 check(s), none failed` / `18 check(s), none failed` | the two native runs' verdicts (§2, §3), each exit 0 |
| `21 check(s), none failed` | the Pi route's check, re-run after the shared change (§4) |
| `carried 6 message(s), 1 call(s), 1 result(s); loss categories named: 31` (Claude → OMP) and `carried 4 message(s), 1 call(s), 1 result(s); loss categories named: 27` (OMP → Claude) | the two native runs' own conversion lines (§2, §3); the CLI's §1/§3 runs report the same carried counts |
| `entries {'compaction': 1, 'message': 7, 'model_change': 1, 'role:assistant': 4, 'role:toolResult': 1, 'role:user': 2, 'session': 1, 'title': 1}` | the installed OMP file's own bytes (§2) |
| `messageCount 3` with `['compactionSummary', 'user', 'assistant']` | OMP's own loader's answers, twice (§1, §2), against a file holding seven message entries — the loader builds context from the walk the file's own `parentId` values make, so the pre-compaction records are not in it |
| `session 3625 -> 7176 bytes` / `3595 to 7097 bytes` | the two real turns: the session grew and its pre-existing bytes stayed a prefix (§1, §2) |
| `287` | the harness's own cases (`python3 -m unittest discover -s scripts/tests`), up from `267` at implementation: `scripts/tests/test_omp_native_check.py` holds 36 of them — 16 at implementation, the fix round's model-level credential case, the review round's eleven (that round's own counts row and console kept `269`, the count of its earlier state; measured on its tree, `ef31bea`, the suite is `280`), the third round's five, and the fourth round's two (the continuation and keyless-provider cases; measured per round on the branch, that file holds `17`, `18`, `29`, `34` methods at `12be17c`/`d3e91ff`/`ef31bea`/`a4acf53`) — and the gate's own `selftest` step ran the same suite (`Ran 287 tests in 5.933s (rc=0)`; the fifth round adds a Go case and no Python case) |
| `23 check(s), none failed` (exit 0) | the OMP native check re-run on the host's own home, once per later review round (§6); no `--resume-prompt`, so the two turn checks do not run |
| `RESULT: FAIL — 5 check(s) failed: … every assistant record carries a non-empty model and provider, the load check verified the installed session natively, …` (exit 1) | the fourth round's no-provider measurement (§6): the failure a host with no provider this check can use must see instead of a pass |
| `11 caught` | the fourth round's mutation check: each seeder rule the round touched, reverted once and its pinning case watched fail, script restored byte for byte (`sha256 d304c451…` before and after) |
| `4 caught` | the fifth round's mutation check: the four rules its new case pins -- the entry states the read's model, the fallback only where it answered none, the calls travel, the entry holds no dialogue -- each disabled once and the case watched fail, `internal/ccc/pi_target.go` restored byte for byte (`sha256 3655d6a3…` after the restores) |
| `290` | the harness's own cases after the sixth round (`python3 -m unittest discover -s scripts/tests`, `Ran 290 tests in 5.984s`), up from `287`: that round's three cases are `ScalarSpellingCases`' two and `SeedCases.test_the_runs_own_definition_spells_ambiguous_ids_as_strings`, taking `scripts/tests/test_omp_native_check.py` from 36 to 39 methods |
| `302 -> 303` (`cmd/ccc` unchanged at `41`) | the sixth round's Go test functions, counted as `grep -h '^func Test'` over the gate's own packages' `_test.go` files (`internal/ccc` and its subpackages): the new `TestOMPScratchHomeDeclaresTheSessionsIdAsStrings` and the rule test renamed `TestOMPYAMLScalarQuotesExoticValues` -> `TestOMPYAMLScalarQuotesEveryValue` with its rows extended; none removed |
| `23 check(s), none failed` (exit 0) with `--model 123/123 --provider-base-url http://127.0.0.1:4141/v1` | the sixth round's ambiguous-id measurement (§6): a home whose provider and model are the *string* `123`, verified by the harness's own loader through both writers |
| `RESULT: FAIL — 2 check(s) failed: …` with `SessionModelRestoreError: Could not restore model 123/123` (exit 1) | the same measurement with the old conditional quoting restored (§6): the failure a non-string-valued scratch home produces for a correctly bound session |
| `6 caught` | the sixth round's mutation check: the helper's rule, its three call sites (models.yml, config.yml) and the Python twin's two writers, each disabled once and its case watched fail -- plus the harness-level re-run -- every file restored (`sha256 16c6571b…`/`da406bec…`/`0266e142…`/`9c11d873…` for the delivered files, taken after the round's last edit) |

Documentation and bookkeeping this increment carries: `AGENTS.md` (the route list, the writer/installer/
verifier files, the OMP native harness, and the OMP rules — its own project spelling, the 256-byte title
slot, the required per-assistant `model`+`provider`, the session binding being the harness's own answer);
`docs/conventions.md` (the layout, the install/discoverability rule's OMP branch, the fixture layout);
`docs/harness.md` (this script's entry and why it is not a gate step); `README.md` (the quickstart's
`--to omp` and its printed resume line); `scripts/upstream-import.py` and `scripts/upstream-manifest.txt`
(the new owned script); `docs/decisions/OPEN-QUESTIONS.md` (Q-0006-01 … Q-0006-04); goal 0005 archived to
`docs/goals/archive/` with its index row and every link to it updated.

### The sixth review round

Round 6's finding was a **type**, not a shape. `ompYAMLScalar` (`internal/ccc/omp_verify.go`) and its twin
`yaml_scalar` (`scripts/omp-native-check.py`) wrote a value as a *plain* scalar whenever it held none of a
fixed set of characters — and a plain scalar is typed by its own spelling, so a target home whose
configured model id is the string `123` (or `null`, `yes`, `2026-11-18`) reached the harness as a number,
a null, a boolean or a timestamp. Both functions now emit every value as a **double-quoted JSON string**
(which is a valid YAML scalar), and `_generated_block` — the one provider definition the Python check
writes itself — spells the provider's name and the model's id and name through the same function instead
of interpolating them bare. `writeOMPScratchHome`'s three call sites are unchanged: they already went
through the helper, which is why one rule now covers every value both homes hold.

Proven against the harness rather than argued, with a home whose only provider and model are spelled
`123` (`python3 scripts/omp-native-check.py --model 123/123 --provider-base-url
http://127.0.0.1:4141/v1`): the run seeds its own home with `  "123":` / `- id: "123"` /
`modelRoles:\n  default: "123/123"\n`, the conversion writes the binding the harness answered for that home
(`"model":"123/123"` in the session's `model_change`, `"model":"123","provider":"123"` on its assistant
records), and OMP's own loader opens the installed session — `RESULT: PASS — 23 check(s), none failed`,
exit **0**. With the old conditional quoting restored (one mutation, `bin/ccc` rebuilt, reverted after the
measurement), the *same* command ends exit **1**:

```console
FAIL: the load check verified the installed session natively — the harness's own loader did not answer for session 01a1ec5f-e9ec-723f-90fa-8ae12a8e406e: exit status 1; stderr: … throw new Aee(Xn[0]); ^ SessionModelRestoreError: Could not restore model 123/123 at VQ (/$bunfs/root/chunks/cli-k …
RESULT: FAIL — 2 check(s) failed: ccc continue exits 0 for a conversion whose verification ran, the load check verified the installed session natively
scratch removed (/tmp/ccc-omp-native-check-tlfqdfig)
```

That is the finding's own impact, measured: the default load verification of a correctly bound session
failing because the verifier's scratch home defined a *different model* (`123` as a number, the model's
name as `null`) than the session binds. The default invocation was re-run on this tree as well
(`python3 scripts/omp-native-check.py`, no `--model`), printing the same `RESULT: PASS — 23 check(s), none
failed` with §2's seeded line unchanged (`seeded provider(s) ghc-proxy, ghc-proxy-chat …;
modelRoles.default ghc-proxy/gpt-5.6-sol`): quoting every value is accepted by the harness for its
ordinary provider names too, and the load arm — the Go verifier's own scratch home — ran there as well.

The mutations that name the rules this round's cases pin, each line disabled once and its case watched
fail, every file restored (these are the delivered files' digests, taken after the round's last edit):
`internal/ccc/omp_verify.go` sha256 `16c6571b…`, `internal/ccc/omp_verify_test.go` `da406bec…`,
`scripts/omp-native-check.py` `0266e142…`, `scripts/tests/test_omp_native_check.py` `9c11d873…`:

| Rule | Line disabled | Observed |
| --- | --- | --- |
| every value the scratch home writes is a quoted scalar | `ompYAMLScalar`'s body reverted to the reviewed conditional | `TestOMPYAMLScalarQuotesEveryValue` FAIL (14 rows: `example-proxy`, `123`, `-1`, `1e3`, `0x1f`, `1_000`, `null`, `~`, `true`, `yes`, `no`, `on`, `.inf`, `2026-11-18` — `ompYAMLScalar("123") = "123", want "\"123\""`), and `TestOMPScratchHomeDeclaresTheSessionsIdAsStrings` FAIL -- `the scratch home's model id decoded as <nil> (<nil>), want the string "null"` (a YAML reader's own answer) beside `config.yml = "modelRoles:\n  default: 123/null\n"` |
| the same rule spells `models.yml`'s own fields (three call sites) | the three calls replaced by bare `%s` interpolation | `TestOMPScratchHomeDeclaresTheSessionsIdAsStrings` FAIL -- `the scratch home's model id decoded as <nil> (<nil>), want the string "null"` and the same for the model's name (the unit case above stays green: it pins the helper, and the home's fields are what this one pins) |
| the same rule spells `config.yml`'s default role | the role's call site replaced by `provider+"/"+model` | the same case FAIL -- `the scratch home's config.yml = "modelRoles:\n  default: 123/null\n", want "modelRoles:\n  default: \"123/null\"\n"`; a `provider/model` value is not a spelling YAML types by itself (its `/` rules that out), so the byte expectation is what pins this site |
| the Python twin quotes every value it is handed | `yaml_scalar`'s body reverted to the reviewed conditional | `ScalarSpellingCases.test_every_value_is_a_quoted_scalar` FAIL — 14 subtests, `AssertionError: '2026-11-18' != '"2026-11-18"'`; the round-trip case beside it stays green, which is the measured shape of that rule (`configured_default_role` answers the same role either way — the role has no scalar type to be re-read as) |
| the run's own generated definition spells its ids that way | `_generated_block` reverted to the bare f-strings | `SeedCases.test_the_runs_own_definition_spells_ambiguous_ids_as_strings` FAIL — 13 subtests, `AssertionError: '  "2026-11-18":' not found in 'providers:\n  2026-11-18:\n …'` |
| (at the harness, not at a case) | `ompYAMLScalar`'s conditional restored, `bin/ccc` rebuilt | the ambiguous-id native check FAIL — `RESULT: FAIL — 2 check(s) failed: …`, the harness's own `SessionModelRestoreError: Could not restore model 123/123` above |

The cases themselves, one per rule above: `TestOMPYAMLScalarQuotesEveryValue`
(`internal/ccc/omp_verify_test.go`) pins the spelling of 19 values, the punctuation-free type spellings
among them, and `TestOMPScratchHomeDeclaresTheSessionsIdAsStrings` writes the home for `123`/`null` and
reads both files back **with an independent YAML reader** (`go.yaml.in/yaml/v3`): the provider's key, the
model's id and name and the default role must decode as the strings the session binds, and the home's
role line is asserted literally beside them (a `provider/model` value has no type to be misread as, so its
spelling is what is left to pin); on the Python side
`ScalarSpellingCases.test_every_value_is_a_quoted_scalar` pins the same table against `json.dumps`,
`ScalarSpellingCases.test_a_role_this_run_writes_is_read_back_as_the_same_role` pins the write→read round
trip through `configured_default_role`, and
`SeedCases.test_the_runs_own_definition_spells_ambiguous_ids_as_strings` drives `seed_omp_home`'s
generated block with 13 ambiguous ids, asserting each quoted spelling and the role the run's own reader
reads back.

`python3 scripts/gate.py` was re-run on this round's tree and printed `GATE: PASS` (exit 0):

```console
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 290 tests in 5.984s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 30 project-owned prefix(es); files seen by git: 5688 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 141 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

This round adds one Go case and three Python cases, and removes none: `internal/ccc` + its subpackages
holds 302 -> 303 test functions (`grep -h '^func Test'` over the package's `_test.go` files; `cmd/ccc`
stays 41), and `scripts/tests/test_omp_native_check.py` 36 -> 39 methods, taking the harness's own suite
`287 -> 290` (`Ran 290 tests in 5.984s` in the block above).

Documentation this round carries: `docs/conventions.md`'s OMP scratch-home clause now states the rule
(every value such a home's own YAML spells is a quoted scalar, with the measured `SessionModelRestoreError`
as the reason). `AGENTS.md` is unchanged: the round changes neither the layout, the gate or its steps, the
commands, the read order, nor a rule an agent must follow. Nothing is archived: goal 0006 is still
`Implemented on feat/0006-omp-continuation; awaiting operator acceptance`, so it stays under `docs/goals/`,
and no owner-only question arose (the finding itself named the direction: quoted strings).

## 7. Limits, and what this record does not claim

- **The Claude half of §3 is a load, not a turn.** This host holds no Claude credential, so Claude Code's
  loader resolved the session and failed at the model call (`result "Not logged in · Please run /login"`).
  The contract's clause is "native load/resume"; the load is verified, and no turn is claimed. Asking for
  one in that direction (`--resume-prompt`) now runs the conversion CLI's own resume check and reports it
  not-verified with the harness's own words, ending the run non-zero (§6's fix-round run) rather than
  passing with the turn skipped.
- **The OMP half's turn ran under the host's own loopback proxy**, in a data directory the check seeded
  from the host's own OMP home with the key's *value* replaced by a placeholder. A data directory that
  resolves a model, though, is a prerequisite this check cannot supply itself: the conversion asks the
  harness for the `<provider>/<model>` binding the session records, and the load half needs that binding
  as much as a real turn does. On a host with no loopback provider and no
  `LITELLM_BASE_URL`/`--provider-base-url`, the seed records are skips, the harness itself answers
  `No default model selected…` so the conversion exits non-zero, and the checks that read the binding
  fail -- the per-record `model`+`provider` pair, the conversion's own load verification, and the script's
  loader arm, where the harness raises `SessionModelRestoreError` -- so the run ends
  `RESULT: FAIL` (exit 1; the measurement is §6's). A requested turn there is a failed check through
  `record_unavailable_requested_turn()`, never a skip. Re-deriving the numbers in §1–§5 needs the loopback
  provider the recorded runs had; a host without one can still run the check and will see the failure
  derivation above, never a pass.
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
