# 0007 — Continue supported conversations in OpenCode: evidence

Contract: [`docs/goals/0007-opencode-target-continuation.md`](../goals/0007-opencode-target-continuation.md)
(this increment leaves it in `docs/goals/` with its row marked *Implemented … awaiting operator
acceptance*, per `AGENTS.md`: the goal moves to `docs/goals/archive/` in the increment that merges it).
Plan: [`docs/plans/0007-opencode-target-continuation.md`](../plans/0007-opencode-target-continuation.md).
Harness: OpenCode `2.2.5` (`opencode --version` prints `opencode v2.2.5`), the pinned AgentsView carry
`83195c8a6861b8c74a4b6cb87512817fed6f8584` (reported by every run's `carried:` line).

Every command below ran on this host (`linux x64`, the harness at
`/home/azureuser/.npm-global/bin/opencode`), in this repository, on branch
`feat/0007-opencode-target-continuation`. Output is quoted verbatim; `…[the rest of this output was
dropped]` is the harness's own elision of a bounded detail line, and nothing else was edited.

## 0. What was measured before anything was written

The plan's §2.1 holds the table; this is what it rests on, all measured against the installed harness
rather than read from a document. The measurements marked **new** are the ones this increment added to the
plan's, each found while making the route verify against the real importer.

| Question | Measured answer |
| --- | --- |
| the data directory | `$XDG_DATA_HOME/opencode` (the harness's own `debug paths` says so); there is **no** variable that names the directory itself, `OPENCODE_DB` overrides only the database file, and `XDG_STATE_HOME` holds service registration |
| the store | one SQLite database per data directory: `<data dir>/opencode.db`, with `session_v2`, `session_message` and `project` among its tables (no per-session file) |
| the import interface | `opencode session import <file> [--directory DIR] --standalone` — prints `Imported session: <id>` on stdout with exit 0, **preserves the document's `info.id`**, requires `--directory` to exist (`LocationNotFoundError` otherwise), prints `Session already exists` on **stderr with exit 0** for an id the store holds, and leaves the store unchanged for a refused import (`Bad Request` / `Internal Server Error`) |
| the import document | `{info, messages}` — the harness's own `session export` output. Required: `info.id`, `info.projectID` (value **ignored, recomputed**, `""` accepted), `info.cost`, `info.tokens{input,output,reasoning,cache{read,write}}`, `info.time.updated`, `info.location.directory`; assistant `agent`, `model{id,providerID}`, tool `state.input` + state body + its own `time`. Optional: `title`, `subpath`, `outcome`, `info.model`, `time.idle`, `finish`, `executed`, `metadata`, reasoning `native`/`time`, idle messages |
| **new**: the model reference's values | the member (`model{id,providerID}`) is required on every assistant message, its values need not be — measured on the installed 2.2.5 against documents the conversion itself wrote: `{"id":"","providerID":""}` imports (exit 0), is stored without a silent drop, and `session export` answers the member as written, exactly as it does for a non-empty id (`build`, the agent name, was measured the same way). Which value a writer puts there is a choice the harness leaves open, so this build writes the empty id where neither the source read nor the harness answered a model rather than naming one it cannot support |
| **new**: the assistant message's own completion instant | an assistant message whose `time` carries `created` alone is accepted by the importer and **silently dropped**: exit 0, no row in `session_message`, no entry in `session export` (all seven members bisected; only `time.completed` changed the outcome). This is what the structural read-back caught on the first real conversion (§1, and the fix is the completion instant `writeAssistantMessage` writes into the message's own `time`, disclosed as `assistant_completion_instant`) |
| **new**: the tool input's spelling | the harness stores a tool part's `state.input` as a parsed value: a document's own `{"limit": 5, "file_path": "…"}` (the recorded key order, with the source's whitespace) is held and read back as `{"limit":5,"file_path":"…"}`, so the store keeps the JSON value and the recorded key order while the recorded whitespace is not what it keeps. The structural comparison accordingly canonicalizes that one member's JSON on both sides -- the writer's item from the source's spelling, the read-back's call from the store's -- while every other member is compared as recorded (a value that does not parse is left as recorded and reported) |
| the tool states | `completed` (non-empty `state.content` text) and `error` (`error.type` + `error.message`); `pending` and `running` are refused |
| a compaction message | needs `summary` + `status` + `reason` (`auto`/`manual` accepted, others refused); an empty summary is accepted |
| the export interface | `opencode session export <id> --standalone` — the session's own document, or exit 1 with `Session not found: <id>`; **content-neutral** (identical rows, identical session row, identical exported document around it, while the database *file's* bytes move with WAL handling) |
| the listing | `opencode session list --standalone --format json` lists the sessions of the **current directory's** project (`[]` from a directory that is not the session's own); `--json` is not a flag (`Unrecognized flag`) |
| the delete | `opencode session delete <id> --standalone` prints `Session <id> deleted` and exits 0; an id the store does not hold is `Session not found: <id>` with exit 1 |
| a resume turn | `opencode run --standalone --session <id> -- <prompt>` answers from the installed history and appends its own records (a user message, an assistant message and an `idle` marker — measured: 3 records per turn); **an unknown id is created** rather than refused, which is why the load level uses `session export` |
| the session's model binding | `info.model` is the binding a turn runs under: with an empty configuration a turn answers `provider.no-route: Model unavailable: <document model>`, and without the member it ran under the configuration's default |
| the agent | `opencode debug agents --standalone` answers its own agent list as JSON: the first `mode=primary`, non-hidden entry is the agent a new session runs under — `build`, both with the owner's configuration and with an empty one — with `model{id,providerID,variant}` or `null` |
| the service trap | a stray `opencode.exe serve --service` process holds the managed port (`Managed service port … already in use`), and a flag placed *after* a `--` terminator leaves the run on that service: no invocation this build runs — and none the check (§2) runs — leaves `--standalone` past a `--` terminator. The check names it immediately after every subcommand; the build carries it last on the subcommands that take no terminator (`session import <file> --directory <dir> --standalone`, `session export <id> --standalone`, `debug agents --standalone`) and immediately after `run` on the resume turn, whose words follow the terminator (`run --standalone --session <id> -- <prompt>`) |

**The model reference's values, measured in the increment's apply round.** A document the §1 conversion
itself wrote was imported into a scratch data root with one assistant `model` member — then every one —
rewritten to `{"id":"","providerID":""}`: each import answered `Imported session: <id>` (exit 0),
`session export` held every message, and the member came back exactly as written. The same document with
`{"id":"build","providerID":""}` imported and exported the same way, so which value a writer puts in the
member is a choice the importer leaves to it, and the empty id is the one that names no model this build
cannot support.

## 1. Claude Code → OpenCode through the pipeline, with the harness's own importer

The source is a **copy** of the committed Claude fixture whose recorded working directory is rewritten to
a scratch workspace, because the harness's importer refuses a session whose directory does not exist
(measured: `LocationNotFoundError` for the fixture's own `/home/user/code/example-api`). The data root is a
scratch one, and the harness's own `opencode` binary is named as `--target-binary` so the conversion
drives the installation exactly as a caller would:

```
$ CLAUDE_PROJECTS_DIR=/tmp/ocx/ws/claude /tmp/ocx/ccc continue \
    claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to opencode \
    --target-home /tmp/ocx/data12 --target-binary /home/azureuser/.npm-global/bin/opencode \
    --verify resume --resume-prompt "Reply with exactly: CLI-RESUME-OK"
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → opencode
  source:       /tmp/ocx/ws/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2122 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2122: the file ends at a record boundary
  converted:    opencode-import-document, 2312 bytes: 7 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  omitted:      empty_user_messages ×1
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: message_model_attribution ×4
  carried-with-caveat: target_model_binding ×1
  carried-with-caveat: usage_zeroed ×1
  carried-with-caveat: target_model_id_attribution ×2
  carried-with-caveat: compaction_reason ×1
  carried-with-caveat: assistant_completion_instant ×4
  carried-with-caveat: unresolved_source_parents ×1
  carried-with-caveat: pre_compaction_history ×4
  omitted:      malformed_source_lines ×1
  installed:    /tmp/ocx/data12/opencode/opencode.db [the target harness's own session importer (opencode session import)]
  source file:  unchanged (sha256 fc21f865d9aa91fdaca51c0415c437742a22f5580ac6bc4466956d89a281bc3f, taken before and after the conversion)
  target id:    ses_7MGLdAA8SS6sj0hEVhP4VTmakr
  working dir:  /tmp/ocx/ws
  resume:       cd /tmp/ocx/ws && XDG_DATA_HOME=/tmp/ocx/data12 /home/azureuser/.npm-global/bin/opencode run --standalone --session ses_7MGLdAA8SS6sj0hEVhP4VTmakr -- ''
  non-interactive resume: cd /tmp/ocx/ws && XDG_DATA_HOME=/tmp/ocx/data12 /home/azureuser/.npm-global/bin/opencode run --standalone --session ses_7MGLdAA8SS6sj0hEVhP4VTmakr -- '<prompt>'
  cli_version:  2.2.5 (probe)
  structure:    verified (the inherited AgentsView OpenCode reader over the installed session)
  native load:  verified (the target harness's own reader (opencode session export, with a scratch configuration and no model turn, plus an unknown-id control))
                the harness's own reader resolved the installed session and answered with its own id and 7 message(s); the same command with an unknown session id refused it, naming ses_unknown_control_0000000000. The check ran with a scratch configuration and state directory and without the credential names the harness could read, and it billed no model turn
  native resume: verified (one turn in the installed session through the target harness's non-interactive resume (opencode run --session))
                the harness answered from the installed session in one turn and wrote its own records into it (the harness's own reader read 7 message(s) before the turn and 10 after)
  answer:       CLI-RESUME-OK
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The three states the core design keeps apart are three lines of that block: `converted` (the writer's
document), `installed` (the harness's own importer's answer — the mechanism is named, and the store is
where the harness reads sessions), and `native load`/`native resume` (the harness's own reader, and then
one real turn).

## 2. The OpenCode native check, with a real turn

`scripts/opencode-native-check.py` (deliberately **not** a gate step: it needs the harness installed and,
for a turn, model access). It plants an unrelated session **through the harness's own importer**, converts
through the CLI, asks the harness's own reader and listing, runs one real turn through the printed
invocation's own words, and removes the planted session through the harness's own `session delete`:

```
$ python3 scripts/opencode-native-check.py --opencode /home/azureuser/.npm-global/bin/opencode \
    --resume-prompt "Reply with exactly: NATIVE-TURN-OK" --expect NATIVE-TURN-OK
target:      /home/azureuser/.npm-global/bin/opencode (/home/azureuser/.npm-global/bin/opencode; opencode v2.2.5)
the source fixture's recorded working directory (/home/user/code/example-api) is rewritten to the workspace: the harness's own importer refuses a session whose directory does not exist (measured: LocationNotFoundError)
PASS: the unrelated session is planted through the harness's own importer — Imported session: ses_nativecheckunrelated0
PASS: the harness's own reader answers the planted session's document — exit 0
PASS: ccc builds with the flags upstream ships
PASS: ccc continue writes one JSON result document
PASS: the conversion reports the conversation it carried and every loss category — carried 7 message(s), 1 call(s), 1 result(s); loss categories named: 35
PASS: the installation names the harness's own importer — installed — the target harness's own session importer (opencode session import)
PASS: ccc continue exits 0 — exit 0; stderr Session not found: ses_ZjrlIZ1tSPyXFq2E6RVtPcVT2D
PASS: the structural read-back verified the installed session
PASS: the route's own load check verified the installed session natively — the harness's own reader resolved the installed session and answered with its own id and 7 message(s); the same command with an unknown session id refused it, naming ses_unknown_control_0000000000. The check ran with a scratch configuration and state directory and without the credential names the harness could read, and it billed no model turn
PASS: the report names the harness's own version and the model a new session of it would run under — cli_version 2.2.5 (probe), model tencent-litellm/deepseek-v4.1-flash-fast (the harness's own debug agents answer)
PASS: the installed session has the identity the report names — ses_ZjrlIZ1tSPyXFq2E6RVtPcVT2D
PASS: the report names the store the harness reads as the installed path — '/tmp/ccc-opencode-native-check-xistf009/opencode-data/opencode/opencode.db', want /tmp/ccc-opencode-native-check-xistf009/opencode-data/opencode/opencode.db
PASS: the harness's own reader answers the installed session's document — exit 0; id 'ses_ZjrlIZ1tSPyXFq2E6RVtPcVT2D', 10 message(s)
PASS: the harness's own reader holds every message the conversion wrote — the store's own document holds 10 message(s); the conversion carried 7, and the conversion's own resume check ran one turn in it
PASS: the installed session holds the source's own first prompt — 'Summarize the retry policy in internal/retry/policy.go.'
PASS: the installed session holds the source's tool call and its recorded result — [('toolu_01Example0000000000000000', 'Read', 'completed')]
PASS: the installed session holds the compaction boundary with the harness's own members — {"type": "compaction", "id": "msg_mF9rswDoomWJdnnWMMqsBpg777", "time": {"created": 1777886160000}, "status": "completed", "reason": "auto", "summary": "Summary: the retry policy caps attempts at four 
PASS: the unknown-id control arm reports no such session, naming the id — exit 1; Session not found: ses_unknown_control_0000000000
PASS: the harness's own listing names the installed session from its own directory — ['ses_ZjrlIZ1tSPyXFq2E6RVtPcVT2D']
PASS: the same listing from another directory does not name it — 1 session(s) listed from /tmp
PASS: the printed resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-opencode-native-check-xistf009/workspace && XDG_DATA_HOME=/tmp/ccc-opencode-native-check-xistf009/opencode-data /home/azureuser/.npm-global/bin/opencode run --standalone --session ses_ZjrlIZ1tSPyXFq2E6RVtPcVT …[the rest of this output was dropped]
PASS: the printed exec_resume_command reads as a POSIX shell line naming the session — cd /tmp/ccc-opencode-native-check-xistf009/workspace && XDG_DATA_HOME=/tmp/ccc-opencode-native-check-xistf009/opencode-data /home/azureuser/.npm-global/bin/opencode run --standalone --session ses_ZjrlIZ1tSPyXFq2E6RVtPcVT …[the rest of this output was dropped]
PASS: the turn ran through the printed invocation and exited 0 — exit 0; stderr [0m
the turn's own output (the target's answer, as the harness printed it):
    NATIVE-TURN-OK
PASS: the turn's answer contains 'NATIVE-TURN-OK' — NATIVE-TURN-OK
PASS: the turn grew the installed session, through the harness's own reader — 10 message(s) before the turn, 13 after
PASS: the session's own records were not rewritten by the turn — the first 10 message(s) are unchanged
PASS: the turn's own answer is in the session the harness reads — the turn appended 3 message(s)
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged — exit 0; its document is byte-identical before and after
PASS: the planted session is removed through the harness's own session delete — Session ses_nativecheckunrelated0 deleted
PASS: the removed session no longer resolves — exit 1; Session not found: ses_nativecheckunrelated0
RESULT: PASS — 31 check(s), none failed
```

A host without the harness ends the run before any check, naming the boundary:

```
$ python3 scripts/opencode-native-check.py --opencode ccc-no-such-harness
RESULT: INCOMPLETE — 0 check(s) ran; ccc-no-such-harness is not on PATH, so the native half of the opencode route could not run (this route's installation *is* the harness's own importer, so there is nothing to verify without it)
$ echo $?
2
```

The run's offline half (`--json`, no turn: the turn is recorded as a named `SKIP`) is
`RESULT: PASS — 27 check(s), none failed`.

## 3. The accepted routes still verify

The shared machinery this increment changed (`verifyStructure`'s installed-bytes seam,
`roundTripReader`'s dialogue seam, the install input's shape) is exercised by the routes that were already
accepted. The Oh My Pi route, with a real turn, on this host:

```
$ python3 scripts/omp-native-check.py --direction claude-to-omp --resume-prompt "Reply with exactly: ACCEPTED-ROUTE-OK" --expect ACCEPTED-ROUTE-OK
the turn's own output (the target's answer, as the harness printed it):
    ACCEPTED-ROUTE-OK

PASS: the turn's answer contains 'ACCEPTED-ROUTE-OK' — ACCEPTED-ROUTE-OK
PASS: the source fixture tree is unchanged — 1 file(s) digested before and after
PASS: the planted unrelated session is unchanged
PASS: every session the home already held is unchanged
PASS: the conversion wrote no id the home already held — 01a1ec98-7bcc-7389-88ee-e5a041b7d47f is new among the session id(s) the home's own header lines record
RESULT: PASS — 25 check(s), none failed
```

The Claude Code → Codex route's offline half is intact (its conversion, installation, the target's own
`thread/read` views of both converted sessions, the repeat scenario, and the source's own digest):

```
$ python3 scripts/codex-native-check.py --resume-prompt "Reply with exactly: CODEX-ROUTE-OK"
PASS: the target's own view lists the converted turns — 5 item(s) in the reply's own turn list
PASS: the target's own view of the second session lists the appended turn — 7 item(s) in the reply's own turn list; the appended turn's own words are in its own view
PASS: the first session's own view does not hold the appended turn — the first session's own view holds 5 item(s), none of them the turn the source gained after it was converted
conversions: 2 session(s) installed (01a1ec98-c725-7f40-905c-1507056d7997 then 01a1ec99-045f-70ef-b93a-93315627d222); real turn(s) run: 1
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical; the converted copy identical too: sha256 e2921e995915855d…, 2540 bytes)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
26 check(s) passed, 1 failed, 1 skipped
```

That run's one failure is the harness's own model endpoint, not the route: the turn fails with
`ERROR codex_api::endpoint::responses_websocket: failed to connect to Responses WebSocket stage=Unknown`
(quoted from the run's own output block), and the harness itself exits 1 with `session 3928 -> 133561
bytes`. This host could not reach Codex's endpoint at that moment (the same route's real turn passed in
0002/0004's own records on a host that could), so this run verifies the *offline* half and reports the
turn as failed rather than unrun — the boundary is named rather than the failure hidden. The OMP run above
is this increment's evidence that a real target-side turn still runs on this host, and this increment
changed nothing on the Codex invocation path.

## 4. Gate, tests and bookkeeping

The gate is `python3 scripts/gate.py`, run alone (never beside a native check: both are memory-heavy on
this host), and it must print `GATE: PASS`:

```
$ python3 scripts/gate.py
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 308 tests in 5.669s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 31 project-owned prefix(es); files seen by git: 5702 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 155 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

Its steps for this increment: `parity` (the carried AgentsView files are still the pin's), `selftest` (the
harness's own Python cases, `scripts/tests/`, which this increment extends with
`test_opencode_native_check.py` and with this record's registration in `test_evidence_claims.py`), `gofmt`,
`go vet`, the `test` step over `./cmd/ccc/... ./internal/ccc/...` with `-tags fts5`, and `secrets`. It
refused the tree once, before the new check was registered: `FAIL parity REQUIRED but was FAIL:
upstream-parity.py -> rc=1` / `parity: FAIL — 1 file(s) are neither carried nor project-owned` /
`scripts/opencode-native-check.py` — the step that keeps a project-owned file from being mistaken for
upstream's, and the fix is this increment's own registration of the script in
`scripts/upstream-import.py`'s owned list and in `scripts/upstream-manifest.txt` (the manifest is what the
checker reads; the list is what regenerates it).

The Go suites this increment's own tests extend:

```
$ go test -count=1 -tags fts5 ./internal/ccc/ ./cmd/ccc/...
ok  	go.kenn.io/agentsview/internal/ccc
ok  	go.kenn.io/agentsview/cmd/ccc
```

What the new cases pin, each a rule rather than a restatement of the code: the document's required members
(the assistant message's own completion instant among them — the rule whose absence the harness silently
drops a message over), the importer's re-encoding of a tool input as evidence-preserving while a changed
value is a difference, the three states of the route's own report, the flat target's counted omission of an
inactive branch, the CLI's shape (`--to opencode`, the printed invocation, the refusal that names the
harness), and the offline native-check rules (the `--standalone` position, a turn that did not grow the
store, a refusal that must name its id, a failed listing that is not an empty one).

Documentation and bookkeeping this increment carries: `AGENTS.md` **had to move with this round** — it
changed the route table (`--to opencode` with the writer/installer/verifier files it names), the commands a
caller runs (the harness's own importer, the `XDG_DATA_HOME` data root, `session export`), the repository
layout (`internal/ccc/opencodefixture/`, `scripts/opencode-native-check.py`) and rules an agent must
follow (an OpenCode session exists only inside the harness's store, so that harness's own importer is the
only install path and a host without the binary has no installation; the two document members the store
requires; the load level asks `session export`, never `run`; the session's own recorded directory; the
target home's own model binding; and the `--standalone` rule every subcommand keeps) — so its
OpenCode bullet, the route list and the native-check name were written in the same increment, and the file
is not edited as decoration. Also carried: `docs/conventions.md` (the layout block's three files and the
fixture package, the install rule's OpenCode branch, why this route adds no fixture directory);
`docs/harness.md` (the new script's entry and why it is not a gate step); `README.md` (the capability
sentence, the quickstart's `--to opencode` line and its printed resume form, the "what `continue` writes"
block, the loader list, `--target-home`/`--target-binary`); `docs/core-design/README.md` (one sentence: a
target whose artifact is a row inside the harness's own store); `docs/goals/README.md` (goal 0006 archived
with its row, 0007's row); goal `0006-omp-continuation.md` moved to `docs/goals/archive/` with every link
to it updated; and `docs/decisions/OPEN-QUESTIONS.md` (Q-0007-01 … Q-0007-05). Goal 0007's own file stays
in `docs/goals/` with its row marked *Implemented … awaiting operator acceptance*: it moves to the archive
in the increment that merges it.

### The control-file review round's re-run of the gate

This round changed no Go file and no case: its fix is `AGENTS.md`'s `--standalone` rule, the "Documentation
and bookkeeping" paragraph above, and two claims this increment's own edits had left stale in `README.md`
(the directory-scoped route count, `all three` → `all four`, and the further-targets list, which still named
`OpenCode` although this increment ships it as a target). It is also the round that gate-checked the
increment's **tracked** bytes: the run quoted above is the stage's own, taken while its output was still
untracked, and `gofmt` walks git's tracked `*.go` set (`scripts/gate.py`), so that line's 3611 is the tree's
count without this increment's nine added `*.go` files — `git ls-tree 338df94` counted 3611 tracked `*.go`
files and `git ls-files '*.go'` counts 3620 now (`git diff --name-status 338df94..HEAD -- '*.go'` → 9 added,
12 modified) — and the nine went into this round's re-run. That run, `python3 scripts/gate.py` on the bytes
this record ships, prints `GATE: PASS`, exit 0, with the same step set and the same contract counts as the
block above: `gofmt` 3620 file(s) gofmt-clean (the line that moved), `build`/`vet` rc=0, `test` 3 package(s)
ok, `selftest` Ran 308 tests (rc=0), `parity` 5553 carried file(s) identical to the pinned revision / 31
project-owned prefix(es) / `files seen by git` 5701 (that line counts tracked ∪ untracked, and the earlier
block's 5702 is its own moment's set, when this increment's output was still untracked), and `secrets` no
leaks in 155 file(s).

### The code-review round's fixes, re-measured

This round's fixes came from the review of the whole increment (`338df94..f5668f5`). The one that changes
behaviour this record rests on is the structural comparison's rule for a tool part's `state.input`: only the
writer's side was canonicalized, so a conversation whose call recorded its keys in an order Go's encoder does
not write was reported not-installed although the store held the same value — the review's measurement, and
the scenario below. Both sides are canonicalized now (the recorded key order and the recorded whitespace are
not evidence the walk asks about). Re-measured against the installed harness with a copy of the committed
Claude fixture whose recorded working directory is rewritten to a scratch tree (as in §1) and whose tool
input is recorded with the two keys out of order:

```
$ CLAUDE_PROJECTS_DIR=/tmp/ocfix/ws/claude /tmp/ocfix/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 \
    --to opencode --target-home /tmp/ocfix/data --target-binary /home/azureuser/.npm-global/bin/opencode --json
$ echo $?
0                       # stderr empty
  installation:            installed — the harness's own session importer (opencode session import)
  verification:            structure verified, load verified
  target_session.file_name: absent
  the source records:      "input":{"limit": 5, "file_path": "/tmp/ocfix/ws/internal/retry/policy.go"}
  the store holds:         "input":{"limit":5,"file_path":"/tmp/ocfix/ws/internal/retry/policy.go"}
                           (opencode session export answers the same bytes)
```

The Go case this round adds fails when the read-back side is left as the store spells it (the canonicalization
reverted for one run, then restored): `tool call 1 differs (written … sha256 bfd7b7e2…, read back … sha256
747b0542…)` — the same pair the review measured. The round's other fixes changed no measurement: the dry-run
warning names the harness's own agent listing as what a dry run leaves unasked (the importer does run for a
dry run, against the throw-away copy the structure check reads — the new route case and the recorded
`calls.Roots` case), the expected refusals of the exclusivity probe and of the load check's control arm no
longer print their `Session not found` lines (the re-run's stderr is empty, where §2's own log recorded
`PASS: ccc continue exits 0 — exit 0; stderr Session not found: ses_ZjrlIZ1tSPyXFq2E6RVtPcVT2D`),
`target_session.file_name` is absent for this route (its artifact is a store row, not a file), and the
measured spelling is stated the same way in the evidence table, the plan, `docs/conventions.md` and
`AGENTS.md`.

```
$ go test -count=1 -tags fts5 ./internal/ccc/ ./cmd/ccc/
ok  	go.kenn.io/agentsview/internal/ccc
ok  	go.kenn.io/agentsview/cmd/ccc
$ python3 scripts/opencode-native-check.py --opencode /home/azureuser/.npm-global/bin/opencode
… RESULT: PASS — 27 check(s), none failed     # the offline half, the same step set §2 lists
$ python3 scripts/gate.py
GATE: PASS
```

## 5. What travels, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| a Claude Code conversation with a tool call, its result, recorded reasoning and a compaction boundary | converted into a 7-message OpenCode document (`converted: opencode-import-document, 2312 bytes: 7 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)`, §1) and materialised by the harness's own importer as a store session (§2: `the store's own document holds 10 message(s); the conversion carried 7`, the extra three being the turn's own records) |
| the source read's own omissions and rewrites | named, counted and per-category: `empty_user_messages ×1`, `tool_name_vocabulary ×1`, `message_model_attribution ×4`, `usage_zeroed ×1`, `target_model_id_attribution ×2`, `compaction_reason ×1`, `assistant_completion_instant ×4`, `unresolved_source_parents ×1`, `pre_compaction_history ×4`, `malformed_source_lines ×1` (§1), out of the 35 categories the report names on every run (§2) |
| the assistant message's own completion instant the importer requires | written for every assistant message from the record's own instant, `×4` in §1 — the member whose absence the harness accepts and then drops the message over (§0), disclosed as `assistant_completion_instant`; the read-back is the check that caught it |
| the tool call and its recorded result | `[('toolu_01Example0000000000000000', 'Read', 'completed')]` in the harness's own document (§2) — the same call id and name the source recorded, with the result's own text as the part's completed state |
| the compaction boundary | `{"type": "compaction", … "status": "completed", "reason": "auto", "summary": "Summary: the retry policy caps attempts at four…` (§2) — the source's own summary, with the harness's own `reason` vocabulary (`auto`) disclosed as `compaction_reason` |
| the model binding a resumed turn runs under | the **target home's** own configured pair, asked of the harness's own `debug agents` listing — `tencent-litellm/deepseek-v4.1-flash-fast` (§2), written as the document's `info.model` and named by `target_model_binding` (§1) — never the source's model |
| the directory a session is filed under | the session's recorded working directory, and the harness's own lookup resolves the session from it: `['ses_7utczOHDIjpwHab8vubyl0UDVt']` listed from that directory and not from another (§2), which is why the printed line changes into it |
| the converted history as the **answer** of a real turn in the installed session | the recorded turn's answer holds the words the run asked for — `NATIVE-TURN-OK` (§2) |
| a real turn run by the route's own resume check through the CLI | the run printed its own turn's words, `CLI-RESUME-OK` (§1), and the harness's own reader read 7 message(s) before the turn and 10 after (`native resume: verified`, §1) |
| the source file during and after the run | `source file:  unchanged (sha256 fc21f865…, taken before and after the conversion)` (§1) and `PASS: the source fixture tree is unchanged — 1 file(s) digested before and after` (§2) |
| a session the target store already held | untouched: planted through the harness's own importer and `PASS: the planted unrelated session is unchanged — exit 0; its document is byte-identical before and after` (§2), then removed through the harness's own `session delete` and shown gone |
| the invocation a caller resumes with | `cd /tmp/ocx/ws && XDG_DATA_HOME=/tmp/ocx/data10 … opencode run --standalone --session ses_0w9mtmfCnO1M21H3eT4694rAOl -- ''` (§1), read back as a POSIX shell line along with the exec form (§2) |
| the accepted OMP route after the shared change | `RESULT: PASS — 25 check(s), none failed` with a real turn (§3) |
| the accepted Claude Code → Codex route after the shared change | its offline half passes in full (§3); its real turn failed on this host with the harness's own `failed to connect to Responses WebSocket`, named there rather than hidden |

The row that rests on the target model's own words is the `NATIVE-TURN-OK` one: it is the block the
harness printed under `the turn's own output (the target's answer, as the harness printed it):`, quoted
verbatim in §2, and the case that keeps a record from quoting an answer it did not make reads exactly that
block (`scripts/tests/test_evidence_claims.py`, which this record is registered with).

## 6. Limits, and what this record does not claim

- **The installation needs the harness.** This target's install is the harness's own importer, so a host
  without OpenCode cannot install a converted session at all — the route reports that boundary
  (`… is not on PATH, and this target's installation is the harness's own importer: …`) rather than writing
  the store itself, and the native check reports `INCOMPLETE` with exit 2.
- **A turn was measured on this host, not on a model's best behaviour.** The two real turns recorded here
  (`NATIVE-TURN-OK`, `CLI-RESUME-OK`) answered the prompts they were given; the record claims that the
  harness answered *from the installed session* (the session grew through the harness's own reader) and not
  that any particular model answered well.
- **The store's own bytes are not claimed to be unchanged by a read.** `session export` is content-neutral
  in the session's *records* (measured), while the database file's bytes move with its WAL handling; the
  record says which of the two it measured.
- **The user count of a read-back includes the compaction row.** The inherited reader reads a compaction row as
  a system record under the user role, so a read-back of a 7-message document reports
  `user_messages: 3`; the boundary count names the same row, and the comparison itself is itemwise.
- **The 35 loss categories are the writer's own vocabulary, not a claim about the harness's behaviour.** A
  category whose count is zero is still named (that is what makes a missing category a defect), and each
  non-zero one is disclosed with its own sentence.
- **Not measured here:** a `--to opencode` conversion on a source whose recorded directory does not exist
  (refused by the harness, and reported as the failure it is), a Windows host, and the real pilot-time
  behaviour of a converted session inside OpenCode's TUI (this increment verifies the loader, the reader,
  the listing and one real turn, not the TUI's own rendering).
