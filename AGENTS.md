# AGENTS.md — call/ccc

**call/ccc** (*call-with-current-conversation-continuation*) lets a conversation recorded by one
AI-agent harness continue as a new, natively resumable session of another harness.

Read in this order; do not skip: this file → [`docs/core-design/README.md`](docs/core-design/README.md)
→ the active goal in [`docs/goals/`](docs/goals/README.md) → [`docs/conventions.md`](docs/conventions.md)
→ [`docs/upstream.md`](docs/upstream.md) (the carried AgentsView pin) → [`docs/harness.md`](docs/harness.md).

An increment's own records live next to it: its plan in [`docs/plans/`](docs/plans/) (written before
the implementation) and its evidence in [`docs/evidence/`](docs/evidence/) (the commands that ran,
with their exit codes and counts).

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
  records the source's relations place off it, counted as `inactive_branch_records`. A relation the read
  cannot answer is counted (`unresolved_source_parents`) instead of being presented as one the source
  recorded.
- **A target writer's claim is shown through the target harness's own loader.** Call `ccc continue …
  --to codex`, `--to claude` or `--to pi` converts, installs and reports **converted / installed /
  natively resumable** as three separate states; "resumable" is only ever `verification.load` or
  `verification.resume` coming back `verified`, and every one of a conversion's omissions or narrowing
  is named in `conversion.losses[]`. The native half needs the target harness installed and, for a real
  turn, model access, so it is **not** a gate step: `scripts/codex-native-check.py`,
  `scripts/claude-native-check.py` and `scripts/pi-native-check.py` re-derive it and report each check's
  own verdict (the gate keeps deciding the offline half — round-trip through the inherited reader,
  install rules, reported losses, CLI shape). A check whose target harness is absent exits 2 naming the
  harness it needs — the Pi check ends its run with `RESULT: INCOMPLETE` and the reason, the Codex and
  Claude checks print the reason on stderr — so a partial run is never quoted as a pass. The Codex check also runs the repeat scenario goal 0004 added: it converts a *copy* of the
  fixture, loads it, appends the committed `internal/ccc/testdata/growth/` fragment to the copy,
  converts again, and shows Codex's own loader opening the second session with its own view holding the
  appended turn while the first session's bytes did not move (`--repeat-resume-prompt` adds one real
  turn in the second session; each prompt flag runs one turn).
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
  model can leave the target unable to open the session at all.
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
  `targets.go`, the install/verify/probe machinery the three routes share (`route_shared.go`), and the
  writers/installers/verifiers `codex_target.go`, `codex_install.go`,
  `codex_verify.go`, `claude_target.go`, `claude_install.go`, `claude_verify.go`, `pi_target.go`,
  `pi_install.go`, `pi_verify.go`; its committed
  native-shaped fixtures are `internal/ccc/testdata/fixtures/` and the growth fragments a run appends to
  a *copy* of one (`internal/ccc/testdata/growth/`, written by the same `internal/ccc/fixturegen` and
  pinned against it) are what makes one source session grow; `cmd/ccc` adds
  argument handling and presentation only, and a target-side native harness
  (`scripts/codex-native-check.py`, `scripts/claude-native-check.py`, `scripts/pi-native-check.py`) is
  registered in `docs/harness.md`, re-runnable, and deliberately not a gate step. A target whose own
  loader resolves a session id through the process's working directory — Claude Code, and Pi — is
  installed under the project spelling of the source read's recorded working directory, and a source that
  recorded none is refused (`ErrNoTargetCwd`) rather than installed where no lookup finds it. The source's
  own relation is reproduced as the read answered it, with the one step the read itself projects: a record
  the Pi-family readers answer as hanging off the message that made a call is chained through the tool
  result standing between them, and counted (`unresolved_source_parents`) where the projection cannot
  settle which of the two the record named — a read whose parent answers are exact keeps the narrower
  reading. A source whose *selected leaf* the target's loader would not reach — the entry written last
  stands for a record off the line the read's last record answers — is refused
  (`ErrUnrepresentableLeaf`) rather than written as a file that resumes a branch the conversation left. A
  Pi invocation the report prints and the check runs names the sessions tree inside the data directory
  explicitly (`PI_CODING_AGENT_SESSION_DIR`), so the printed line, the verified lookup and the
  installation agree even when the caller's shell, or the home's own `settings.json`, names another tree.
- Gate: `python3 scripts/gate.py`, which must print `GATE: PASS`; it runs `parity` and `selftest` as
  required steps, and its Go steps are narrowed to `./cmd/ccc/... ./internal/ccc/...` with the measured
  reasons in [`docs/harness.md`](docs/harness.md#why-the-go-steps-target-a-subset) — that narrowing is
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
