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
- **A record's dialogue is the source's own, and a rendered reasoning trace is never it.** Every target
  writer reads its message text through one shared rule (`dialogueText` → `dialogueFromContent`): the
  source's own projection where it made one — an explicitly empty projection included — and, where it
  projected none and the record carries no calls, the record's `Content` minus the reader's own rendering
  of that record's persisted reasoning trace (`[Thinking]…[/Thinking]`, the block shape
  `internal/parser.IsThinkingOnlyContent` recognises). A rendering is removed only where the reader that
  wrote `Content` is one that renders the trace into it (`Message.ContentRendersThinking`, established by
  `internal/ccc/dialogue_provenance.go`'s `thinkingRenderingReaders` from the session's own agent label:
  each listed reader's construction site in the carried reader is named there, a reader that also projects
  a dialogue is answered by its projection instead, and a reader that records `Content` as the source
  wrote it — Vibe's independent `content` and `reasoning_content` — carries a marker block only as the
  model's own words, which nothing may drop), and where the record's own bytes and its own trace leave the
  block no choice: the readers write each part of the trace as one block in the record's own part order,
  so a removal is established by a *reading* of the record — an assignment of some of `Content`'s marker
  blocks to the trace's parts, in order, whose words open the trace, close it, and account for everything
  between them — and a block is taken only where every reading the bytes admit takes it. The blocks the
  readings weigh are enumerated at every opener — each paired with the first closer that follows it, an
  opener standing inside a pair read before it included — because a text part's own trailing opener
  otherwise pairs with the rendering's closer and hides the rendering whose words the trace holds, so the
  reading takes the record's own pair and deletes words it wrote (round 13). The feeds around a marker
  pair are not a part boundary: a part of the record's own text may carry feeds of its own around a pair
  whose words the trace holds, as a substring of it as readily as the whole (round 12), so where the
  readings disagree about a block the block keeps its place as the model's own words. What stands: the
  words are the record's own wherever the reader put them — a reader that keeps the record's part order
  (ZCode's) renders the trace after the text it followed as readily as before it — so a trace the read
  answered nothing for, a block no trace of that record accounts for, a marker pair inside a part the
  record wrote, and a block the record's own bytes leave ambiguous keep their `Content` byte for byte,
  while a record whose `Content` is nothing but a rendering of its own trace has no dialogue at all (the
  readers' own rule for that shape, which asks the block's words to *be* the trace, whole). The trace
  travels to the target beside that dialogue as the record's reasoning, or is counted where the target has
  no shape for it; a writer that fell back to the raw `Content` would carry the trace twice — once as
  reasoning and once as words the source never recorded the agent as saying.
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
  because the call's own instant stated twice turns a recorded span into one the source never recorded.
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
  native-shaped fixtures are `internal/ccc/testdata/fixtures/` and the growth fragments a run appends to
  a *copy* of one (`internal/ccc/testdata/growth/`, written by the same `internal/ccc/fixturegen` and
  pinned against it) are what makes one source session grow; `cmd/ccc` adds
  argument handling and presentation only, and a target-side native harness
  (`scripts/codex-native-check.py`, `scripts/claude-native-check.py`, `scripts/pi-native-check.py`,
  `scripts/omp-native-check.py`, `scripts/opencode-native-check.py`) is
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
