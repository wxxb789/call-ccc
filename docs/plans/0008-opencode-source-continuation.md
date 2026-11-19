# Plan — 0008 Continue OpenCode history in other harnesses

Contract: [`docs/goals/0008-opencode-source-continuation.md`](../goals/0008-opencode-source-continuation.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape reference: increment 0003 ([`docs/plans/0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md))
for a *source* direction over a previously accepted target, and 0007
([`docs/plans/0007-opencode-target-continuation.md`](0007-opencode-target-continuation.md)) for everything
OpenCode-shaped: the store, the harness's own importer, the measured document members, the native-check
discipline. 0002 supplies the route contract and 0004 the boundary rules (both unchanged here).

Written before implementation. Every OpenCode-side fact below was **measured on this host against the
installed OpenCode CLI (`2.2.5`)** and the carried AgentsView revision
(`83195c8a6861b8c74a4b6cb87512817fed6f8584`), in scratch data roots under `/tmp` (`XDG_DATA_HOME`
isolated, `OPENCODE_DIR` naming the root the reader resolves), through throw-away probe programs; the
commands and raw answers are repeated in `docs/evidence/0008-opencode-source-continuation.md` §0–§1.

## 1. What this increment adds, and what it must not

The goal's outcome is a **source direction**: an OpenCode session continues in a native target harness.
The source half is already the inherited AgentsView provider — 0007's own check already said so ("the
harness's own reader is already the source side's (the inherited reader reads its store, so an OpenCode
source needs no second parser)") — and the target halves (Codex, Claude Code) are accepted routes from
0002/0003. This increment therefore adds **no new parser, no new writer, no new target, and no new
persistence**: it

1. **pins the source route with tests** over a committed OpenCode fixture, at the reader, the pipeline and
   the CLI;
2. **commits that fixture** — the harness's own export document, written by `internal/ccc/fixturegen`
   (so `TestFixturesMatchTheirGenerator` pins it) and materialised into a store by
   `internal/ccc/opencodefixture`;
3. **adds the native direction `opencode-to-codex`** to `scripts/opencode-native-check.py`, which drives
   this host's installed OpenCode *and* Codex: the harness's own importer plants the fixture, the CLI
   converts, the target harness's own loader opens what was installed, and a requested turn runs in it;
4. names OpenCode as a **source** in the CLI's help and the README, and writes the increment's plan,
   evidence record, obligation-citation manifest and bookkeeping.

The goal's Bounds are the design's existing ones and stay unweakened: the session is read only through
inherited AgentsView code, no bespoke SQL reader is added, and the identity transform is untouched.

## 2. Measured: the source side is the inherited provider, and what it answers

### 2.1 Where a session lives, and how a reference resolves

| Observation (measured, OpenCode 2.2.5) | Consequence |
| --- | --- |
| a session is a row in `<data dir>/opencode.db` (`session_v2` + `session_message`), where the data dir is `$XDG_DATA_HOME/opencode` | the source is a **container member**, not a file: the read's identity is the inherited virtual path `<db path>#<session id>` |
| the provider's configured root for OpenCode is the data dir itself (`OPENCODE_DIR`, config key `opencode_dirs`, default `~/.local/share/opencode`); with `XDG_DATA_HOME=/d` the store is `/d/opencode/opencode.db`, so the root is `/d/opencode` | `OPENCODE_DIR` names the directory holding `opencode.db`; a reference resolves when that root is configured, exactly as `CLAUDE_PROJECTS_DIR`/`CODEX_HOME` do for the other sources |
| `ccc inspect opencode:<id>` with `OPENCODE_DIR` set resolves the session through the provider's own `FindSource` (raw id → `findOpenCodeFormatSourceFile` → `OpenCodeSQLiteSessionExists` → virtual path) | no reader work is needed; the route is a *configuration* the caller names |
| `ccc inspect opencode:<id>` resolved the session on the first attempt, reporting `source_path`/`display_path` as `/…/opencode.db#ses_…`, `project` from the store's worktree basename, `cwd` from the session row's directory | the identity the source read reports is the store's own |
| the store's own session row carries the harness version and the session's directory; the reader answers message records only (the `idle` record is a session control its own switch skips) | one `idle` record in a document is *not* a message in the read, and nothing is counted for it because the read never held it |

### 2.2 What one read of a session answers

Measured by reading the fixture document below through the inherited OpenCode v2 reader (both the store
the harness's own importer wrote and the store `internal/ccc/opencodefixture` writes read the same
conversation — verified member by member; the only difference is the *spelling* of a tool input object,
below):

| The store holds | The read answers |
| --- | --- |
| a `user` message with `text` | `Role: user`, `Content` = that text |
| an `assistant` message with a `tool` part whose `state.status` is `completed` | one `ToolCall` (id, name `read`, `InputJSON` = the part's `state.input` as the store spells it) and **two** result events: `started` (the part's own `time.ran`, no content, `source: tool_execution`) and the terminal `completed` event (the part's `state.content` text, `time.completed`) |
| an `assistant` message with `reasoning` and `text` parts | `Content` = `"[Thinking]\n" + trace + "\n[/Thinking]\n" + text` (the reader's own rendering, joined with newlines); `ContentRendersThinking` true, `Thinking` **empty**, no dialogue projection |
| the reader serves the `OpenCode` label, and `dialogue_provenance.go` lists it among the readers that render a trace into `Content` | such a record is state (1) of the AGENTS.md dialogue rule: the text travels as the record's own bytes *with the caveat counted* (`unprojected_dialogue`, carried-with-caveat). Nothing is removed from `Content` by inference |
| every record the reader returns | `DialogueProjected` false, `Dialogue` empty — the OpenCode reader answers **no** dialogue projection for any record, so *every* written message is counted `unprojected_dialogue` on both target routes (measured: 4 of 4 on the fixture) |
| the reader's `openCodeV2ToolCall` | the progress event is a real record shape of this source (the part's own `time.ran`), not an artifact: the writers leave it out (it holds no text) and count it (`empty_tool_results` on the Codex and Claude routes, measured 1 each), and the terminal event's text is what the target holds |

### 2.3 The boundary a container member can and cannot state

| Observation | Consequence |
| --- | --- |
| the read's `SourcePath` is `<db>#<id>`; `sourceFileOf` answers "a multi-session container is read through a member identity … no file holds the conversation's bytes" | `source.boundary.state` is `not-taken` with the container and the member named; `installation.source_check_state` is `not-verified` for the same reason (both measured in the fixture conversion) |
| the container's own bytes hold every member, and the harness's own writes move them with its WAL handling | the conversion must not claim the database file's digest as "the source is unchanged": the source's *records* are what a check re-reads (this check's own `session export` before/after), and the report says the file cannot answer the question |
| the fixture-store bytes and the harness-store bytes are different files with different row spellings | the boundary rule (not the digests) is what this increment pins; the store is never opened for writing by this build (a Go case digests the store tree around a conversion) |

### 2.4 The fixture, and why it is a *document*

| Observation | Consequence |
| --- | --- |
| a store can only be written by the harness (`opencode session import`) or by the fixture package the 0007 tests already use | a committed store is not an option; the committed artifact is the **document** the harness's own `session export` produces, and every consumer materialises a store from it |
| measured: `session import <document> --directory <dir> --standalone` preserves the document's messages byte-shape at the value level and rewrites exactly three members — `info.projectID` (recomputed from the directory), `info.location.directory` (set to `--directory`) and `info.time.updated` (the import instant) | the importing check needs **no** working-directory rewriting at all (unlike 0007's Claude-source copy): `--directory <workspace>` is what places the session, and the fixture's own invented directory stays invented |
| measured: the importer requires `--directory` to exist (a missing one is `LocationNotFoundError`) | the check passes its own scratch workspace |
| measured: the importer stores a tool part's `state.input` as a parsed value — the store keeps the JSON value and the key order and drops the recorded whitespace (0007's measurement, re-measured here) | the fixture is written **compact** (one canonical `json.Marshal` line, exactly what `fixturegen`'s `canonicalJSONL` produces), so the fixture store's own `InputJSON` is byte-identical to the harness store's. A pretty-printed document would have made the fixture store answer a *spelling* the harness's store never holds |
| measured: the harness's own export is pretty-printed by the exporter, its `text` parts carry no `time` member, its `reasoning` and `tool` parts do | the fixture carries the members a real export carries (no invented member, no member a real export never has), serialized with the one canonical encoding the generator uses for every other fixture |
| a `text` part with a `time` member is a member the importer drops (measured: the re-export loses it) | the fixture carries none, so the fixture is a *shape the harness itself produces* rather than one it silently normalizes |

## 3. The route this increment pins (unchanged machinery, new coverage)

```
OpenCode store (harness-written rows)
        │  inherited AgentsView OpenCode provider  (×0 new parser)
        ▼
   Conversation document  ── identity transform ──▶  Codex rollout writer   (0002/0003)
        │                                             Claude transcript writer(0002/0003)
        └─ read's own answers, counted losses ───────▶ report
```

Nothing in `internal/ccc`'s writers changes. What the increment pins is that this source's read *composes*
with them: the same document every other source produces, with this reader's own answers, and the same
counted losses — plus the container-member boundary state.

### 3.1 The losses this source produces (all pre-existing categories, measured on the fixture)

| Category | This source's count | Why |
| --- | --- | --- |
| `unprojected_dialogue` (carried-with-caveat) | 4 of 4 messages (Codex), 4 (Claude) | the reader answers no dialogue projection and records no trace text, so each written text is the reader's own `Content`, named as such |
| `empty_tool_results` (omitted) | 1 | the progress event (`started`) holds no text: it is not written, and the terminal event's text is |
| `tool_result_outcomes` / `tool_result_statuses` (carried-with-caveat) | 1 | the terminal event's recorded status word has no member on the target's output record |
| `tool_result_sources`, `result_record_instants` (omitted) | 1 each | the read's channel label and the result's own recorded instant have no target member |
| `tool_name_vocabulary` (carried-with-caveat) | 1 | `read` is the source's tool vocabulary |
| `unresolved_source_parents` (carried-with-caveat) | 4 | the OpenCode v2 reader answers no per-record parent relation, so each record's place on a flat target line is this conversion's reading |

**Format limits** (properties of the pair, in the report's `target_session.format_limits` and the docs):
the source's session controls (`idle`/outcome markers, agent and model switches) are not messages the
inherited reader returns; a tool part's richer `state` members (`metadata`, `providerCall`, `rawInput`)
are the harness's own bookkeeping, not evidence the read carries; an attachment (`file`) block in a tool
result travels as its blocks and is counted when the target cannot carry it; persisted reasoning keeps
its text inside the reader's rendering and no provider-native payload.

## 4. Verification: converted ≠ installed ≠ natively resumable

| Level | Mechanism (unchanged) | This increment's evidence |
| --- | --- | --- |
| structure | the installed session read back through the inherited **target** reader | Go cases over both targets; the CLI's own `verification.structure`; the native run's report |
| load | the target harness's own loader (Codex `app-server` `thread/resume`; Claude `--bare --resume` + unknown-id control) | the native direction `opencode-to-codex` runs it on this host (Codex 0.170.0 is installed); the Claude half is exercised by the conversion + structure read-back here and by 0003's accepted check where the binary is |
| resume | one real turn through the printed invocation | `--resume-prompt` in the native direction; the turn's own answer printed under `native_check_common.TURN_OUTPUT_MARKER` and the rollout's growth measured in its own records |

### 4.1 `scripts/opencode-native-check.py --direction opencode-to-codex`

New direction, new flags `--codex` (default `codex`) — the OpenCode side is the *source* here, so the
run's `--target-home` is the **Codex** home and the OpenCode data root is always this run's own scratch
one. Steps:

1. refuse to run (exit 2, `RESULT: INCOMPLETE`) when either binary is missing, naming which;
2. build `ccc` (`bin/ccc-native-check-opencode`), plant the committed fixture **through the harness's own
   importer** (`session import <fixture>.json --directory <workspace> --standalone`) into the scratch
   data root, and read it back with the harness's own `session export` (the planting's own
   acknowledgment, and the control arm answers `Session not found: <id>` for an id nothing installed);
3. convert through the CLI (`continue opencode:<fixture id> --to codex --target-home <codex home>
   --target-binary <codex> --verify load --json`, `OPENCODE_DIR` naming the scratch root), reading the
   report's three states rather than trusting them;
4. compare the **installed rollout's own records** with the **fixture document's own bytes** — the first
   prompt, the tool call's name and recorded input value, the result text, the second turn's question and
   answer, and the reader's rendering of the reasoning part — so a writer that mangled either member
   cannot pass by agreeing with itself;
5. record the boundary state the report gives a container member (`not-taken` + the container named), and
   that the source's own records did not move (the harness's own `session export` digested before/after —
   never the database file's bytes);
6. with `--resume-prompt`: one real turn, run by the check itself through the report's **own**
   `exec_resume_arguments` (plus the check's isolation flags), the rollout's growth counted in its own
   records and the conversion's own records shown to be unchanged, the answer printed under the marker —
   the design the implementation reached (see §11: the CLI's resume level would have put a turn's records
   in the rollout *before* the rollout-vs-fixture comparison, and would have billed the same turn twice for
   one request);
7. remove the planted session through the harness's own `session delete` and re-read its absence.

The offline cases for the new direction live in `scripts/tests/test_opencode_native_check.py` (the
`selftest` step runs them): the fixture document's required members, the evidence comparison (a mangled
name/input/result is a finding), the source-records-unchanged reading, the missing-target verdict, and
the direction's own argument rules.

## 5. CLI and machine-readable output

```
ccc continue opencode:<session-id> --to codex|claude [--json] [--dry-run] [--target-home DIR] …
ccc inspect opencode:<session-id> [--json]
```

No new flag, no new grammar: the source is named the way every other source is (`<provider>:<id>`), and
the JSON document keeps the established shape. What changes is discoverability:

* `ccc` help's example block gains `ccc continue opencode:<session-id> --to codex`, and its source
  paragraph names the roots a source is found through (`OPENCODE_DIR` for OpenCode, beside
  `CLAUDE_PROJECTS_DIR`/`CODEX_HOME`/…);
* `README.md` lists OpenCode among the sources and shows the source-side invocation and the resume line
  the target prints (Codex's is `CODEX_HOME=… codex resume <id>`).

`ccc formats --json` stays deferred, as in 0002–0007: the capability listing is the acceptance of a
later increment, not this one.

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| 1. an OpenCode-origin session with substantive messages continues as a new native Claude Code **or** Codex session, with an actual target native load/resume result | `scripts/opencode-native-check.py --direction opencode-to-codex` (runs in §1–§2 of the record): the harness's own importer plants it, `ccc continue … --to codex --verify load` reports `converted`/`installed`/`load: verified` through **Codex's own app server**, and the requested turn is the check's own — recorded as what the harness answered (on this host the scratch home holds no credential, so the harness's own `401` is the recorded answer, Q-0008-05); the Claude half is the same pipeline's second target, verified by the structural read-back on this host and by 0003's accepted native check on a host with the binary |
| 2. tool call/result and multi-part content semantics survive where supported; unsupported content has a specific reported loss, not silent omission | the fixture is tool-bearing and multi-part (reasoning + text + tool part, a progress event beside its terminal one): Go cases pin the read's own answers and each target document's tool evidence; the native run compares the installed rollout with the fixture's own bytes; every non-zero loss category is named in §3.1 of the record |
| 3. the accepted OpenCode-target route and the prior Claude/Codex routes remain correct; source-side AgentsView packages and SQLite schema unchanged | the gate's `parity` step on an unchanged carry; the existing suites (`continue_opencode_test.go`, the Claude/Codex route tests, `continue_growth_test.go`, `cmd/ccc`'s route tests) run by the gate's `test` step; §3 of the record re-runs the OpenCode-target native check and the Codex route check |
| 4. the same predictable non-interactive JSON and resume identity as other sources, without a new catalog or service | `cmd/ccc` cases (success JSON, unknown-session failure, `inspect`), the native run's report reading, and the printed invocation read as a POSIX line; the record's §0 lists what was *not* added (no catalog, no daemon, no new flag) |

`docs/evidence/0008-citations.tsv` holds one row per obligation a gate-runnable Go test discharges
(`D1`–`D4`, `B1`–`B2`, and the AGENTS.md OpenCode-source paragraph's rules `S1`–`S7`); the header comment
names the rows deliberately left out (the native run's own obligations and the parity half), with the
form-based reason.

## 7. Fixture plan

* `internal/ccc/testdata/fixtures/opencode/ses_0008opencodefixture00000001.json` — one line, one
  canonical JSON encoding, the invented conversation: two turns, a `read` tool part with its terminal
  state and its `time.ran` (so the read yields the progress + terminal event pair), a reasoning part
  beside a text part, an `idle` session control, and invented `info` members.
* written by `internal/ccc/fixturegen` (`opencodeSession()` → `fixtureFiles()`), so
  `TestFixturesMatchTheirGenerator` pins its bytes;
* the comments that say "each provider subdirectory is that provider's source root"
  (`fixtures_test.go`, `parity/dump_test.go`, `parity/dump.go`'s `Options.Fixtures`) are corrected: the
  `opencode/` entry is a **document**, not a source root, because this provider's session is a row in a
  store the harness writes — `internal/ccc/opencodefixture` materialises it for tests and the harness's
  own importer does for the native check.

## 8. Documentation and bookkeeping

* `AGENTS.md` — a new bullet for the OpenCode **source** side (the store as the unit, the root, the read's
  own answers and the counted caveat, the progress/terminal event pair, the container-member boundary, the
  fixture-document stance, the `opencode-to-codex` direction). This increment changes rules an agent must
  follow, so the control file moves with it.
* `docs/conventions.md` — the layout block (the fixture entry), the fixture rule's OpenCode branch (a
  committed *document*, the harness's own shape, planted by its own importer), the native-check note.
* `docs/harness.md` — the OpenCode check's entry gains the second direction, its flags and its verdicts.
* `README.md` — the capability sentence, the examples, the loader/mechanism paragraphs.
* `docs/goals/README.md` — goal **0007** moves to `docs/goals/archive/` with its row updated (its
  implementation is merged onto `main` as `e68f18a`), every link to it moves, and 0008's row reads
  *Implemented on `feat/0008-opencode-source-continuation`; awaiting operator acceptance*.
* `docs/decisions/OPEN-QUESTIONS.md` — the defaults this increment takes (the native direction chosen is
  Codex, because the Claude binary is absent on this host; the fixture is a committed document rather than
  a store; no second native direction).
* `scripts/tests/test_evidence_claims.py` — the new record joins `ROUTE_DOCUMENTS`.

## 9. Defaults taken where only the owner could decide, and out of scope

**Defaults** (each reversible, each appended to `docs/decisions/OPEN-QUESTIONS.md`):

1. *Which target the native direction verifies.* The goal allows Claude Code **or** Codex. This host has
   Codex installed (0.170.0, whose app server answers `thread/resume` offline) and no Claude Code binary;
   the direction therefore verifies Codex natively and covers Claude Code by the conversion, the structure
   read-back and 0003's accepted evidence where the binary exists. Reversal: add the symmetric direction
   when a host has the binary — the pipeline is identical.
2. *The fixture's form.* A committed **document** (the harness's own export shape) rather than a store:
   a store can only be written by the harness, and committing the harness's own interchange document keeps
   the fixture native-shaped and small. Reversal: none needed; the fixture is one file.
3. *No new parity (`oracle`) case for OpenCode.* The oracle compares a read against a pristine checkout at
   the pin over *file*-shaped fixture roots; an OpenCode source is a store, so a case would have to
   materialise one inside the oracle. The gate's `parity` step already proves the carried reader is
   upstream's bytes, and the native check reads a store the harness itself wrote. Reversal: extend the
   oracle's fixture mechanism in a later increment if a store-shaped case is wanted.
4. *No `ccc formats` entry.* Deferred as in every prior increment.

**Out of scope, with reasons**: a second OpenCode fixture (attachments/errored calls are shapes the target
writers already cover with other fixtures, and this increment's tool-bearing one is what the goal asks
for); a growth fragment for the OpenCode store (the goal's clauses do not ask for a growing source, and a
store's growth is a harness write the fragment mechanism cannot express as bytes appended to a file);
legacy `storage/…json` OpenCode sources (the provider reads them, this increment's route pins the
store-backed form the installed harness writes); any archive/index/daemon work (design §2).

## 10. Stage order and gate

implement → verify (`python3 scripts/gate.py`) → simplify (report only) → apply → review (report only) →
fix → gate → hand the tree over for operator verification. Never a native check beside the full suite on
this host. The gate must print `GATE: PASS` with the increment's Go suites and the new obligation rows
green; `parity`, `citations` and `selftest` are required steps and this increment adds rows/files to all
three.

## 11. Where the implementation departed from this plan, and why

Three changes were made while implementing, each recorded here rather than in a rewritten plan (0007 §6.1's
habit):

1. **The requested turn is the check's own, run through the report's own vector** — not the CLI's resume
   level (planned in §4.1 step 6). Two measured reasons: the CLI's resume level would have written a turn's
   own records into the rollout *before* this check compares the rollout with the fixture's bytes (a Codex
   turn appends an `environment_context` user message — measured), and asking both for the CLI's resume
   level and for the check's own turn would bill two turns for one `--resume-prompt`. The route's resume
   level stays covered by the CLI's own tests and by 0003's/0002's accepted checks.
2. **One comparison function, not two.** The first revision compared an OpenCode document with a Codex
   rollout by searching the target's texts for a rendering a *document* never holds; the driven run failed
   its own planting check on a correct store. The emitted shape is now the reader's own rendering on both
   sides (`opencode_reader_text`), so one member-for-member comparison serves both the planting and the
   installed file, and it is *stronger*: a target holding the record's bare words where the read carries the
   rendering is a difference.
3. **The container-member boundary assertions were split into their own Go case**
   (`TestOpenCodeSourceBoundaryStatesTheContainerItCannotMeasure`) so the obligation-citation manifest cites
   one test per rule rather than the same pipeline case twice.
