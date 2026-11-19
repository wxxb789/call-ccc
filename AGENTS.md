# AGENTS.md — call/ccc

**call/ccc** (*call-with-current-conversation-continuation*) lets a conversation recorded by one
AI-agent harness continue as a new, natively resumable session of another harness.

Read in this order; do not skip: this file → [`docs/core-design/README.md`](docs/core-design/README.md)
→ the active goal in [`docs/goals/`](docs/goals/README.md) → [`docs/conventions.md`](docs/conventions.md)
→ [`docs/upstream.md`](docs/upstream.md) (the carried AgentsView pin) → [`docs/harness.md`](docs/harness.md).

An increment's own records live next to it: its plan in [`docs/plans/`](docs/plans/) (written before
the implementation) and its evidence in [`docs/evidence/`](docs/evidence/) (the commands that ran,
with their exit codes and counts). Every obligation a gate-runnable Go test discharges also gets a row
in that increment's `<goal>-citations.tsv`, which the required `citations` gate step runs
(`docs/conventions.md`).

## What is binding

- `docs/core-design/README.md` governs every increment. No increment may silently weaken it.
- The goal file is the completion contract for the increment you are implementing: its `Done when`
  clauses are the acceptance, its `Bounds` are hard, and its `Stop and report` section says when to
  stop instead of improvising.
- **AgentsView is the source-side authority.** Its reader, parser, sync, SQLite and migration code
  and their tests are carried at a traceable upstream revision and are never rewritten, simplified
  or forked. A locally maintained parser fork is a defect, not a shortcut. The pin, the declared
  deviations and the update procedure are in [`docs/upstream.md`](docs/upstream.md); the machine
  record is `scripts/upstream-manifest.txt`, and `scripts/upstream-parity.py` — a **required** gate
  step — proves the carried files are still the pin's. Change a carried file only upstream.
- **Precision over apparent completeness.** An explicitly partial translation beats a
  plausible-looking one. Never fabricate tool results, reasoning, roles, resume identities or a
  success result; a valid file is not evidence that anything can resume it. A target format that is
  itself a tree (Pi's session JSONL, a Claude Code transcript) is written as the *source's* own tree:
  each entry hangs off the entry of the record the source named as its parent, so the target's own
  loader walks the source's active path and an inactive branch stays off it as recorded history rather
  than being chained in as an active turn. A target that is one line of records with no parent relation
  (a Codex rollout) cannot hold a branch that way, so it writes the source's own line and omits the
  records the source's relations place off it, counted as `inactive_branch_records`; and a relation the
  read cannot answer is counted (`unresolved_source_parents`) instead of being presented as one the
  source recorded — on that route too, where the count is the line walk's own accounting of the places it
  chose, not only of the records it left out.
- **A target writer's claim is shown through the target harness's own loader.** Call `ccc continue …
  --to codex`, `--to claude`, `--to pi`, `--to omp` or `--to opencode` converts, installs and reports
  **converted / installed /
  natively resumable** as three separate states; "resumable" is only ever `verification.load` or
  `verification.resume` coming back `verified`, and every one of a conversion's omissions or narrowing
  is named in `conversion.losses[]`. The native half needs the target harness installed and, for a real
  turn, model access, so it is **not** a gate step: `scripts/codex-native-check.py`,
  `scripts/claude-native-check.py`, `scripts/pi-native-check.py`, `scripts/omp-native-check.py` and
  `scripts/opencode-native-check.py`
  re-derive it and report each check's
  own verdict (the gate keeps deciding the offline half — round-trip through the inherited reader,
  install rules, reported losses, CLI shape). A check whose target harness is absent exits 2 naming the
  harness it needs — the Pi, OMP and OpenCode checks end their run with `RESULT: INCOMPLETE` and the
  reason, the Codex and Claude checks print the reason on stderr — so a partial run is never quoted as a
  pass. The Pi and OMP
  checks drive their harness's own RPC loader against a **throw-away copy** of the converted bytes
  (measured on both: a load writes records of its own into the session it opens) and read the harness's
  own `get_state`/`get_messages` answers beside an unknown-id control arm; the OMP check's scratch data
  directory carries this run's own provider definition for the load (its `apiKey` value a placeholder this
  run writes) so a load needs no credential of the caller's. The Codex check also runs the repeat scenario goal 0004 added: it converts a *copy* of the
  fixture, loads it, appends the committed `internal/ccc/testdata/growth/` fragment to the copy,
  converts again, and shows Codex's own loader opening the second session with its own view holding the
  appended turn while the first session's bytes did not move (`--repeat-resume-prompt` adds one real
  turn in the second session; each prompt flag runs one turn).
- **A record's dialogue is the source's own answer or the record's own bytes, never a reading of them.** Every
  target writer reads its message text through one shared rule (`internal/ccc/dialogue_provenance.go`'s
  `dialogueOf`). The source's own answer is the projection (`parser.ParsedMessage.DialogueText`, carried
  as `Message.DialogueProjected`), and where the read made one it is the whole answer — an explicitly
  empty projection included, which is the source's own statement that the record has no dialogue, and an
  exact conversion of a record with none. `Content` is never read beside a projection. Two semantic
  requirements stand on their own: a record whose `Content` is byte for byte the reader's own rendering of
  its whole trace (`[Thinking]\n<trace>\n[/Thinking]`) has no dialogue at all — the rendering is the trace
  the read carries as the record's reasoning — and a literal `[Thinking]…[/Thinking]` block standing in a
  record of a reader that records `Content` as the source wrote it (Vibe's independent `content` and
  `reasoning_content`) is the model's own words, which nothing may drop. What is *not* true, and what the
  rules below replace, is that a record's flattened bytes can be un-mixed: the readers write the trace's
  own text verbatim between their markers, so two different native part orders — the trace's text before
  the record's, and a text part that quotes the block a thinking part renders — produce the same `Content`,
  the same trace and the same reader label, and no inverse can say which occurrence was the model's.
  `internal/parser/zcode.go` renders a thinking part's text verbatim, so a closer written inside that text
  is not escaped and the first closer after an opener need not be the reader's own. The readings machinery
  that inferred a removal from those bytes (rounds 11–15 each found a shape it deleted, or a rendering it
  published, wrongly) is therefore **removed, not patched**: nothing is removed from a record's `Content`
  by inference any more. Where the read answered no projection, what happens to a rendering reader's
  `Content` follows the read's own answers: a record whose read recorded a trace has no text established
  (below), and a record whose read recorded none carries its `Content` as the record's own bytes — the
  rendering among them where the record has one — and the report counts it (`unprojected_dialogue`).
- **Where the read answered no projection, the outcome is disclosed, never guessed.** `dialogueOf`
  answers one of three states for a record, and every route acts on the state, never on a text alone.
  (1) The reader is not one `thinkingRenderingReaders` names, or the read answered no trace for the
  record, and the record carries no calls: `Content` is the record's own bytes, it travels as the
  record's text, and the conversion counts it (`unprojected_dialogue`, carried-with-caveat) — so a reader
  of the report can see which text rests on the read's `Content` rather than on an answer the source
  gave. The list names both the readers that render a trace and project none and those that render one
  *and* project (a projection answers first wherever the read made one; the entry is what covers a record
  *without* one that carries a trace — a legacy archive row, a record a reader path keeps as the source
  wrote it); every entry names its
  construction site in the carried reader, and an upstream update that brings in a reader rendering a
  trace into `Content` has to add it (docs/upstream.md's procedure), or that reader's records travel with
  the rendering carried as dialogue. The list names **labels, not readers**: a reader that serves several
  labels (the Pi reader serves six, the Codex reader four, the Kimi, Hermes and OpenCode readers two or
  three each) takes one entry per label, because the table is asked by the session's own stored label —
  a label left out leaves that label's records, a legacy archive row among them, travelling with the
  rendering carried as dialogue. The entry acts only where the read recorded the record's trace: a
  reader that renders a trace into `Content` without recording the trace's own text (`Message.Thinking`
  empty for every record it produces) takes this state too, so the text — the rendering among it —
  travels and is counted here, and the listing cannot act for that record. A label this build knows no
  reader for is treated the same way and counted in the same category. (2) `Content` is not the record's
  own bytes (the reader renders the trace into it and the record carries one, or the record carries calls
  whose renderings a reader may have inlined): **nothing
  is written and nothing is deleted**. The record's text is not published, its tool evidence still is, its
  recorded reasoning still travels where the target has a shape for it, and the conversion counts the
  record (`unresolved_dialogue`, omitted; the count is not a claim that the record wrote none). The source
  session keeps the bytes, and a later route with better provenance can read them. (3) `Content` is
  exactly the reader's rendering of the record's whole trace: the record has no dialogue, and the trace is
  not double-carried. Partial conversion — not refusal — is the chosen disposition, and each state is
  named on all three surfaces: the document carries the read's own per-record answers, the target holds
  the per-message shape the state produced (a message or part with its text, or none), and the report
  counts the category. A report that says `converted` without them would claim dialogue that cannot be
  traced to a source-owned answer, and "the file parsed" is not evidence that its dialogue is exact.
- **OpenCode is a target whose store only the harness writes.** A session is a row in
  `<XDG data home>/opencode/opencode.db`, not a file, so `--to opencode` installs **through the harness's
  own importer** (`opencode session import … --directory <the session's recorded directory> --standalone`)
  and never writes SQL into that store: a host without the binary has no installation, and the report says
  so instead of working around it. Three members the importer requires are rules a writer here must keep,
  all measured on 2.2.5: an assistant message's `time.completed` (a message whose `time` carries
  `created` alone is accepted and then **silently dropped** — no row, no export entry), a tool part's
  `state.input`, which the store holds with the recorded key order and without the recorded whitespace,
  so the structural comparison canonicalizes that one member's JSON on both sides and compares every
  other member as recorded, and an assistant message's `model` member, which must be present although
  its values need not be: a record neither the source read nor the harness answered a model for is
  written with the member's id empty — measured: an empty id is imported, stored and exported as
  written — rather than a name this build cannot support. A tool part's `time.completed` is the instant of
  the record the carried result was recorded on, which the read answers per result
  (`ToolResult.Timestamp`: a result event's own envelope instant, which may be later than the record that
  carried its call) and only where the read holds none is the containing record's instant written — the
  report counts that substitution (`result_completion_fallbacks`) rather than presenting it as recorded,
  because the call's own instant stated twice turns a recorded span into one the source never recorded; and
  where the instant the read answered — the result's own record, or that containing-record fallback —
  precedes the part's own `created` (an out-of-order source, whose result record stands before the record
  that carried the call), `completed` rises to the call's recorded start and the part is counted
  (`result_completion_adjustments`): a row of its own, because an absent instant and an out-of-order one
  are different reads and a fallback that was raised too takes both. Where a route's record for a result
  states the containing record's instant instead of the read's answer for the result's own record, the
  same two reads are counted beside the rows it already has — `result_completion_fallbacks` (the read
  answered none) and `result_record_instants` (the read answered one and the file states another: the
  Codex, Claude and Pi/OMP writers) — and every route counts a written result's recorded channel label
  (`tool_result_sources`) and a carried call's recorded child-session link (`subagent_session_links`)
  where its own shape has no member for them, so no recorded value is dropped unnamed. The floor a
  message's own `created` is taken against is the messages' own records and nothing else: a part's
  completion raises only the instant the session's `time.updated` is read for, so an earlier call's
  result does not move a later,
  overlapping call's recorded start up to itself (round 15's finding). A message whose own recorded
  instant precedes the instant already written is written at that instant instead — the document's message
  order is the read's own — and each such written instant is counted (`substituted_record_instants`),
  because every tool part inside that message takes the substituted instant as its `created` and a recorded
  call→result span is shortened by it. The Codex, Claude, Pi and OMP writers share that clock rule
  (`internal/ccc/route_shared.go`'s `entryClock`) and report its substitution under the same category:
  each written timestamp the clock stamped with the previous written record's instant is counted there,
  and the instant written in a record's place is always another instant the read recorded — never one this
  build invented — so a recorded instant is not silently replaced on any route.
  The load level asks the harness's own
  `session export`, never `run`: `run` *creates* a session for an id the store does not hold. The
  directory a session is resolved from is the one its own document records, and the model
  binding it runs under is the target home's own, asked of the harness (`debug agents`). Every OpenCode
  subcommand this build prints or runs carries `--standalone`, and none leaves the flag past a `--`
  terminator, where it hands the run to the managed service and that service answers from a store this
  run did not grow (measured); the printed resume line names it immediately after its subcommand. The
  printed resume line and the resume check name the data root and, beside it, the store's own database
  file (`OPENCODE_DB`): a line a shell reads can set a variable but cannot remove one, and a caller's
  inherited override would otherwise send the pasted command to another store, where `run --session <id>`
  *creates* a session for an id the store does not hold (measured). An import whose process fails is
  **reconciled, never read as an absence**: the harness's own `Imported session: <id>` line in the
  captured output is its answer, the harness's own reader (`session export`, asked past the deadline that
  stopped the import) is the next, and the report says `unresolved` where neither answers — a run may
  report the installation absent only when the store itself answered that the id does not resolve, because
  a caller who reads "nothing was published" and converts again leaves a second snapshot of one
  conversation (measured: the harness commits the session and then exits). The route's native check keeps
  that discipline over its own planting, in a caller's store as much as a scratch one: the id it plants is
  read first, a session whose own document is that check's fabricated fixture is an earlier run's leftover
  and is removed through the harness's own delete, a session holding anything else is left exactly as
  found and the run stops there, and a planting whose import process failed or overran its deadline after
  committing is reconciled the same way — acknowledgment, then the store's own reader — and removed again
  as often as it is published, because a check that left its fixture behind would make every later run's
  planting meet `Session already exists` (measured: the pre-fix check ended `RESULT: FAIL` there and the
  fixture stayed).
- **OpenCode is a source whose session is a row in that harness's own store.** A reference
  (`opencode:<session id>` under the provider's configured root — `OPENCODE_DIR`, the configuration's
  `opencode_dirs`, else `~/.local/share/opencode`, the directory holding `opencode.db` — or the provider's
  own virtual path `<database>#<session id>`) is resolved by the inherited AgentsView OpenCode provider
  and read by its reader: this build adds no SQL reader, no second parser and no store writer of its own,
  and a caller who wants to convert an OpenCode session names the root it lives in, exactly as the other
  sources name `CLAUDE_PROJECTS_DIR`/`CODEX_SESSIONS_DIR`/… . What one read answers is the reader's own, and
  the routes report it as such: the OpenCode v2 reader renders a `reasoning` part into the record's
  `Content` (`[Thinking]\n<trace>\n[/Thinking]`, joined with the record's `text` parts) and records **no**
  trace text and **no** dialogue projection, so every written message is carried-with-caveat
  (`unprojected_dialogue`) and nothing is removed from `Content` by inference; a `completed` tool part
  yields the part's own `time.ran` as a textless **progress event** beside the terminal event that carries
  its recorded content, so the writers leave the progress event out and count it
  (`empty_tool_results` on the Codex and Claude routes) while the terminal event's text is what the target
  holds; and the store's session controls (an `idle` marker, agent and model switches) are not messages the
  reader returns, so nothing is counted for them — the read never held them. A member of that store is a
  **container** member: `source.boundary` is `not-taken` naming the store and the member,
  `installation.source_check_state` is `not-verified`, and "the source is unchanged" is measured on the
  harness's own *records* (`session export` before and after), never on the database file's bytes, which
  move with the harness's WAL handling. Its committed fixture is therefore a **document**, not a store
  (`internal/ccc/testdata/fixtures/opencode/<session>.json`, written by `internal/ccc/fixturegen` from the
  shape a real `session export` writes, one canonical encoding, compact so the tool input's JSON value is
  the one the store holds) — a store can only be written by the harness, so tests materialise one through
  `internal/ccc/opencodefixture` and the native check plants the same document through the harness's own
  importer. `scripts/opencode-native-check.py --direction opencode-to-codex` is that direction: it plants
  the fixture, converts through the CLI at the **load** level, reads the installed Codex rollout's own
  records against the fixture's own bytes (prompts, call name, recorded input value, recorded result text —
  one output per call — and the reader's rendering of the reasoning record), reads the printed Codex line as
  a POSIX invocation naming `CODEX_HOME` and the session, and — with `--resume-prompt` — runs one real turn
  through the report's own vector, recording what the harness answered (a refused turn is a failed check,
  never a skip).
- **A conversion writes a fresh native identity and replaces nothing.** The install is exclusive, the
  source is never opened for writing, no credential or login store is copied, and a target data
  directory nobody named is an error rather than a directory invented here. Tests and demos use
  isolated data directories. A written Codex session records the target harness's own configured model
  provider — asked of the harness itself — and no model, never the source's: recording nothing is
  refused by a home that has migrated to paginated threads, and recording the source's provider would
  be false. A Claude Code transcript has no provider field at all, so `--to claude` states that as the
  `model_provider_binding` format limit and binds the session to no provider, and no route ever writes the
  source's provider as the session's own. A Pi session binds no provider either — its records carry a
  model and a provider per assistant message, and the read answers the model each source message recorded
  but not the endpoint that served it, so a converted entry carries that model and no provider (writing
  the read's session-level harness label in the endpoint's place was measured wrong) and the report names
  that as the attribution it cannot answer — and its printed resume command names no provider or model,
  because the source's provider label is a source-side name the target home need not define and a flagged
  model can leave the target unable to open the session at all. An Oh My Pi session **requires** a model binding its loader can
  resolve (`SessionModelRestoreError` otherwise), so `--to omp` records the target home's own configured
  `<provider>/<model>`, asked of the harness — never the source's — and every converted assistant record
  states a non-empty `model` beside a non-empty `provider`, because the harness's request builder reads
  both on every assistant record and fails to build the next request when either is missing: the model is
  the source's own where the read answered one and the target home's configured id where it did not, the
  provider is the harness's own answer, and a run that asked the harness nothing (a dry run, a missing
  binary) writes no provider — and no model for a record the read answered none for — with the report
  naming every such absence. The file opens with the harness's own fixed-width (256-byte) title-slot
  line, written with an empty title, which is the shape its own writer starts from and the line it
  rewrites in place when it titles a session.
- **A conversion is a snapshot of the source at one boundary, and repeating it is the same operation.**
  The source is read again and written as *another* fresh target session; nothing ever merges later
  source history into an existing target session, and nothing rewrites the source. Every conversion's
  result states the boundary it covers — the source file's sha256 and size, whether the file is still the
  size the read's own snapshot recorded (`read_coverage`: a size match, which is what the read's own record
  can show — it holds no digest of the bytes it opened, so a same-length replacement of the file after the
  read is not ruled out), and, for a source whose records are appended lines, where its
  line-feed-terminated records end and how many bytes of an unterminated record follow — so two
  continuations of one growing session can be told apart and each is attributed to the bytes it read; a
  read whose own recorded snapshot is not the size the file has at the boundary says so instead of
  attributing its conversation to bytes it is not shown to have read. A file whose last bytes hold no line
  feed holds a record the source had not terminated, which it may still be writing: that is disclosed
  (`boundary.unterminated_tail_bytes`, the run's warnings, the human `records:` line) and never called
  published or left out, because whether a read carried an unterminated record is the read's own answer —
  what is forbidden is presenting bytes the read did not read as history, and another conversion once the
  file is written reads a finished record. The boundary is taken from the file's own bytes, never inferred
  from a provider's silence, and the record offsets are claimed only where the provider's own declarations
  say its source is a transcript of appended lines *and* the file's own bytes can be one — a compressed
  container of records is reported by its digest and size with no offset claimed for it.
- Out of scope by design: Git/worktree management, process or environment migration, credentials,
  fleet orchestration, and any second session index for the existing archive.

## How to work here

- The continuation core and its per-harness target writers live in `internal/ccc` — the route table
  `targets.go`, the install/verify/probe machinery the routes share (`route_shared.go`), and the
  writers/installers/verifiers `codex_target.go`, `codex_install.go`,
  `codex_verify.go`, `claude_target.go`, `claude_install.go`, `claude_verify.go`, `pi_target.go`,
  `pi_install.go`, `pi_verify.go`, `omp_target.go`, `omp_install.go`,
  `omp_verify.go` (the last three are the Pi family's second format, whose shared writer is Pi's own
  parameterized by `piFamilyShape`), and the OpenCode target's `opencode_target.go`, `opencode_install.go`
  and `opencode_verify.go` (whose store a test builds through `internal/ccc/opencodefixture` — a fixture
  package, because a real OpenCode session exists only inside the harness's own store); its committed
  native-shaped fixtures are `internal/ccc/testdata/fixtures/` — one provider directory per source, except
  `fixtures/opencode/`, which holds the harness's own **export document** for one session rather than a
  source root, because that provider's session is a row in a store only the harness writes — and the growth
  fragments a run appends to
  a *copy* of one (`internal/ccc/testdata/growth/`, written by the same `internal/ccc/fixturegen` and
  pinned against it) are what makes one source session grow; `cmd/ccc` adds
  argument handling and presentation only, and a target-side native harness
  (`scripts/codex-native-check.py`, `scripts/claude-native-check.py`, `scripts/pi-native-check.py`,
  `scripts/omp-native-check.py`, `scripts/opencode-native-check.py`, whose two directions are
  0007's `claude-to-opencode` and 0008's `opencode-to-codex`) is
  registered in `docs/harness.md`, re-runnable, and deliberately not a gate step. A target whose own
  loader resolves a session id through the process's working directory — Claude Code, and Pi — is
  installed under the project spelling of the source read's recorded working directory, and a source that
  recorded none is refused (`ErrNoTargetCwd`) rather than installed where no lookup finds it. Oh My Pi
  files a session under the project directory **its own** spelling computes, which is not Pi's for a
  working directory under the home or the temporary directory (it shortens those to `-…` and `-tmp…` and
  keeps Pi's `--…--` form only for a path outside both), and it moves a session it finds under Pi's
  spelling into its own directory itself — so the installer asks OMP's rule (`OMPProjectDir`), and a
  conversion's reported path is where the file stays. The source's
  own relation is reproduced as the read answered it, with the one step the read itself projects: a record
  the Pi-family readers answer as hanging off the message that made a call is chained through the tool
  result standing between them, and counted (`unresolved_source_parents`) where the projection cannot
  settle which of the two the record named. The step is taken only where the read may project, because it
  exists to undo a projection: an exact answer — a Claude transcript's `parentUuid` — names the record it
  means, and a result standing beside a record the read chains to the same message is that record's
  sibling, not its place. A record the target leaves out keeps the place its own walk resolved — the
  result's own entry where that place came through the projection — so a record naming the omitted one
  follows that place rather than the nearest ancestor of its raw relation. A source whose *selected leaf*
  the target's loader would not reach — the entry written last stands for a record off the line the read's
  last record answers — is refused
  (`ErrUnrepresentableLeaf`) rather than written as a file that resumes a branch the conversation left, and
  for a Pi-family source the *source-side* half of that rule is checked too: the carried Pi reader skips the
  tree entries the harness's own loader builds context from (`branch_summary`, `custom_message`), so the
  read's last record is not by itself the entry the source's file selects as its leaf — the file's own last
  entry is. `internal/ccc/pi_source_leaf.go` walks that file's own path (the same entry reader the load
  check reads a converted file with) and refuses a conversion whose read cannot be reconciled with it: the
  deepest record the read holds on that path must be the record the read's last message stands for, and
  every entry on the path the read does not carry must be a kind the harness's own loader keeps out of the
  conversation (an unknown kind is not assumed to be outside it). Where no source file stands behind the
  read — an archive row, a file that moved while it was read — the entry is not established, and that is
  stated in the run's warnings instead of being presented as the read's own answer. A
  Pi or Oh My Pi invocation the report prints and the check runs names the sessions tree inside the data
  directory explicitly (`PI_CODING_AGENT_SESSION_DIR`), so the printed line, the verified lookup and the
  installation agree even when the caller's shell, or the home's own `settings.json`, names another tree.
- Gate: `python3 scripts/gate.py`, which must print `GATE: PASS`; it runs `parity`, `citations` and
  `selftest` as required steps (`citations` verifies every `docs/evidence/*-citations.tsv` row: the
  cited Go test must exist in the package the row names and run and pass), and its Go steps are
  narrowed to `./cmd/ccc/... ./internal/ccc/...` with the measured reasons in
  [`docs/harness.md`](docs/harness.md#why-the-go-steps-target-a-subset) — that narrowing is
  a disclosure, not a shortcut; a run in which a step was skipped,
  bounded or absent prints `GATE: PASS (partial: …)` naming it, so a partial run can never be quoted
  as a full one. It is the same file CI runs (the runners name the interpreter `python`); never narrow
  or skip a step to get green — a step that cannot run is disclosed, not silenced, and a required step
  that cannot *verify* fails: `parity` prints `parity: INCOMPLETE` and exits 2 when git cannot
  enumerate the tree, instead of a PASS with the disclosure inside its detail line. The gate also runs
  its own regression tests from `scripts/tests/` (the `selftest` step, stdlib only).
- The harness's own tests are part of that contract: an assertion under `scripts/tests/` must fail
  when the rule it names is broken, and must hold on every platform and every Python 3 the docs name
  — no hard-coded separators, no POSIX-only tool behaviour, no assumption that a tool is installed,
  no reliance on a `pathlib` behaviour that differs between Python minors, and no expectation that
  only holds on the platform or the `PATH` the case happens to run on: a case's stub bin answers the
  host probes it is not about, and the suite re-runs itself in a child whose `PATH` holds no tools and,
  once, with the platform probe answering Windows-without-a-compiler, so a host-dependent case fails
  on this host ([`docs/conventions.md`](docs/conventions.md)). The Go suites the gate's `test` step
  runs are held to the same rule: a case whose subject is what a check makes of a target harness's
  answer drives that harness through the check's own seam and answers the executable lookup the same
  way, instead of needing the harness installed on the machine running the suite — a case that *is*
  about a missing binary pins a `PATH` that holds no harness.
- Stage order for an increment: implement → verify (gate) → simplify (report only) → apply (accepted
  findings only) → review (report only) → fix → gate → PR. Report stages must not edit the tree.
- One goal ≈ one branch ≈ one pull request, squash-merged onto a linear `main`. Do not commit, push
  or open a PR unless the dispatched stage explicitly asks for it; the operator owns the merge. A goal
  whose implementation is merged moves to [`docs/goals/archive/`](docs/goals/archive/) with its index
  row updated in the same increment, and the links other files make to it move with it.
- Owner-only decisions never block an unattended increment: take the most reversible default,
  disclose it in the report, and append an entry to
  [`docs/decisions/OPEN-QUESTIONS.md`](docs/decisions/OPEN-QUESTIONS.md).

## This file grows

`AGENTS.md` is the project's control file and is expected to change as the project does. When an
increment changes the repository layout, the gate, the commands, the read order, or any rule an agent
must follow, update this file **in the same increment**. When it changes none of those, say so in the
increment's report instead of editing the file for its own sake. Keep it short: binding rules and
pointers, not a tour of the code.
