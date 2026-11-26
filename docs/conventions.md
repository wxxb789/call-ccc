# Conventions — call/ccc

Binding for every increment. When this file and a goal file disagree about acceptance, the goal file
wins; when they disagree about structure or method, this file wins unless the increment records why.

**How paths are written here.** The carried AgentsView tree lives in `agentsview/` and this
repository's own module in `ccc/` (`docs/upstream.md` states the layout and the old → new mapping). A
path written with an `agentsview/` prefix is a carried file; a path written as `internal/ccc/…`,
`cmd/ccc/…` or `scripts/gate.py` is this repository's own — the first two are relative to the module
(`ccc/`), the rest to the repository root.

## Stack

- **Go**, first and only shipping language for the CLI, the continuation engine and the inherited
  AgentsView code.
- The module path and ancestry are inherited from AgentsView (`go.kenn.io/agentsview`) for the carried
  half; this repository's own half is the subpath module `go.kenn.io/agentsview/ccc`, whose path
  prefix keeps the carried packages importable (Go's internal-package rule is decided on import paths,
  so `go.kenn.io/agentsview/ccc/...` may import `go.kenn.io/agentsview/internal/...`). Upstream
  imports are never bulk-renamed to invent an unrelated module path, and the two modules are wired by
  exactly one `replace` (`ccc/go.mod` → `../agentsview`) with no `go.work`.
- SQLite access goes through the inherited AgentsView path, including the `fts5` build tag and CGO
  where the inherited driver needs them. `call/ccc` never opens the archive with its own driver,
  schema migration, watcher or writer.
- No second language is introduced for scripting. Non-Go tooling is Python 3 (stdlib only) or the
  existing shell-free entry points under `scripts/`.

## Layout

Two directories, one sentence: inside `agentsview/` is upstream's (the carried tree, at its pin, in
`docs/upstream.md`), everything else is ours. This repository's own module is `ccc/`, and inside it:

```
cmd/ccc/            the standalone CLI; no continuation-core types leak into it
internal/ccc/       continuation core: conversation document, readers, archive access,
                    target writers and the route table (targets.go, codex_target.go,
                    codex_install.go, codex_verify.go, claude_target.go, claude_install.go,
                    claude_verify.go, pi_target.go, pi_install.go, pi_verify.go, omp_target.go,
                    omp_install.go, omp_verify.go, opencode_target.go, opencode_install.go,
                    opencode_verify.go, continue.go),
                    and the install/verify/probe machinery the routes share (route_shared.go)
internal/ccc/parity the canonical dump and the oracle a pristine checkout runs too
internal/ccc/opencodefixture  the store a test builds when it needs one the inherited OpenCode reader
                    can open (a fixture, not a writer of this build's: OpenCode sessions exist only
                    inside the harness's own SQLite store)
internal/ccc/testdata/fixtures  committed native-shaped sessions, written by internal/ccc/fixturegen
                    (fixtures/opencode/ holds the harness's own export document for one session, the
                    source direction's fixture: that provider's session is a row in a store only the
                    harness writes)
internal/ccc/testdata/growth    committed fragments a run appends to a *copy* of one fixture, so one
                    source session grows; the same generator writes and pins them
docs/core-design/   the long-lived product and engineering boundaries
docs/goals/         numbered outcome contracts (archive/ once accepted)
docs/plans/         one plan per increment, written before its implementation
docs/evidence/      one evidence record per increment: real commands, exit codes, counts, and
                    the increment's obligation-citation manifest (<goal>-citations.tsv)
docs/upstream.md    the carried AgentsView pin, its declared deviations and the update procedure
docs/decisions/     append-only record of decisions taken and defaults chosen
scripts/            harness entry points (gate, knobs, carry importer, parity, comparison,
                    codex-native-check, claude-native-check, pi-native-check, omp-native-check and
                    opencode-native-check, whose two directions are 0007's claude-to-opencode and 0008's
                    opencode-to-codex: the native verifications, deliberately not gate steps)
```

Inherited AgentsView packages keep their upstream paths and stay where upstream put them. The carry is
recorded in `scripts/upstream-manifest.txt` and checked by `scripts/upstream-parity.py`, which the
gate runs as a required step.

## The inherited tree

- Carried files are **upstream's**, not ours: a fix to a carried parser, sync path, schema or test
  belongs upstream, contributed separately, not as a local edit that the parity step then has to
  declare as a deviation.
- Four paths are this repository's own and are therefore not carried: `.gitignore`, `AGENTS.md`,
  `README.md`, `docs/README.md`. Upstream's `.github/**` is not carried either, because Actions reads
  `.github/workflows/` at the repository root and would run upstream's CI here. Everything else at
  the pin is present, byte for byte, apart from one declared line-ending normalisation and one
  generated build input; `docs/upstream.md` lists them.
- A new revision arrives as a pin bump: `scripts/upstream-import.py --pin <rev>` rewrites the carried
  files and the manifest, one increment says why the revision was taken, and `parity` must pass.
- Carried code is edited under upstream's own guides (`agentsview/docs/agents/*`), which are carried
  with it;
  this repository's rules (this file, `AGENTS.md`) govern everything else.
- The generated input `agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz` is committed
  although upstream does not track it (upstream's Makefile restores it before every Go build), is
  ignored by the carried `snapshot/.gitignore`, and therefore needs `git add -f` when it is first
  added.

## Obligation citations

A goal's acceptance surface is a **machine-checked manifest** beside its prose record:
`docs/evidence/<goal>-citations.tsv`, one row per obligation that a gate-runnable Go test discharges. The
gate's required `citations` step (`docs/harness.md`) parses every `docs/evidence/*-citations.tsv` and
runs each row's test, so "this obligation is covered" is re-derived by a command instead of re-read by a
reader.

Tab-separated, a header line naming these four columns in this order, `#` comment lines and blank lines
ignored, one row per rule id:

| column | what it holds |
| --- | --- |
| `rule_id` | the obligation's id in the increment's matrix (`A1`, `B3`, `E2`, …) |
| `test` | the Go test that pins it — a top-level test function's name (`TestXxx`) |
| `package` | the package `go test` is handed, in this repository's module's own `./` spelling (`./internal/ccc` — the checker runs `go test` in the module, `ccc/`) |
| `artifact` | free text that reaches the prose, e.g. the matrix section the pair comes from |

**Which obligations get a row:** only those a gate-runnable Go test discharges. An obligation whose
gate re-derivation is a python case under `scripts/tests/` — the `selftest` step runs them on every gate
run — cannot be held by this form, which is one Go test function in a package `gate-targets.txt` covers.
That is the reason for every id the seed manifest's header comment lists as left out: the native-check
obligations (the native run is their own positive evidence, and the case beside it is what re-derives the
rule in the gate) and the harness's own rules alike. The manifest's header comment names the rule ids
deliberately left out, one line per reason — nothing is dropped silently. A discovered manifest that
declares no data rows is a failure naming the file, not a pass: a goal with no Go-test-dischargeable
obligation ships no citations file, rather than an empty one.

## Rules of the work

- **Evidence over assertion.** Every `Done when` clause is paired with an artifact the operator can
  re-derive: a command with its real exit code, a file with its own bytes, a native loader's own
  output. A parser's acceptance of a file is not proof that a target harness can resume it.
- **A target writer claims only what the target harness answered.** A conversion reports
  **converted**, **installed** and **natively resumable** as separate states; the last one comes from
  the harness's own loader (and, at the level that runs one, from a real turn), never from the file
  parsing. The target's records are written the way the target writes them (the model-visible item
  *and* the harness's own view of the turn): where the target format is itself a tree, it is written as
  the *source's* own tree — each entry hangs off the entry of the record the source named as its parent —
  so the target's loader walks the source's active path instead of one this build invented, and a branch
  the conversation left stays in the file off that walk; where the target is one line of records with no
  parent relation (a Codex rollout), the source's own line is written and the records the source placed
  off it are omitted and counted (`inactive_branch_records`), because a record written there becomes
  model-visible history, and the walk's own accounting — the places it chose that the read did not answer
  (`unresolved_source_parents`) — is disclosed on that route as it is on the tree writers. A record that
  follows a tool result is chained through it only where the read itself projects (the Pi-family readers
  resolve a result's descendants to their visible ancestor); an exact answer names the record it means, so
  a result standing beside a record the read chains to the same message is that record's sibling and not
  its place. A record the target leaves out keeps the place its own walk resolved — the result's own entry
  where that place came through the projection — so a record naming the omitted one follows that place
  rather than the nearest ancestor of its raw relation, and the place is counted once, where the omitted
  record's own walk took it. Where the step is taken it is counted even when the result stands immediately
  before the record: the read answers a record the source placed under the call and one it placed under the
  call's result the same way, so adjacency cannot settle which of the two the file held, and the count
  names the reading as this conversion's rather than the read's. The provider a record states is the
  provider the read actually answered — the target's own configured endpoint where the harness was asked,
  and none where the read answers the model alone — never the source's, and never invented.
- **A native-verification harness is a re-derivable script, not a gate step**, exactly like
  `scripts/upstream-compare.py`: it needs the target harness installed (and model access for a real
  turn), so the gate keeps deciding the offline half. A check that cannot run says so. The default
  verification level never bills a model turn and never writes to the installed session: a level that
  runs one is asked for explicitly and reported as the level that ran.
- **Disclose loss by category.** Converted, installed and natively resumable are three different
  states; a report that conflates them is wrong even when the file is valid. Every category of
  omitted or narrowed evidence is named with its count on every run, and the format's own
  unrepresentable limits are stated as limits rather than as this run's counts.
- **Non-destructive by default.** Never modify the source transcript, never overwrite an existing
  target session, never copy credentials or login stores, and never log conversation bodies, tool
  arguments or secrets by default. Write into isolated data directories for tests and demos, and a
  native check seeds its provider configuration only into the scratch home it made itself: a home the
  caller named is read, never written to, because replacing a configuration there (and the credential
  references in it) is a change no session digest would show.
- **A conversion is a snapshot at a stated boundary, and repeating it is the same operation.** A source
  that grew is read again and written as *another* fresh target session: no run merges later source
  history into a session an agent may already have continued, and no run rewrites the source or an
  earlier target. Every conversion reports the boundary it covers (`source.boundary`: the source file,
  the digest of its bytes at the boundary instant, whether the file is still the size the read's own
  snapshot recorded — a size match, which is what that record can show, since it holds no digest of the
  bytes the read opened — and, for a source whose records are the lines a harness appends, where its
  line-feed-terminated records end and how many bytes of an unterminated record follow) and the same facts
  in the human block, so a caller — or an agent choosing which continuation to resume — can tell two
  snapshots of one growing session apart without any index. A read whose own snapshot recorded a size the
  file no longer has says so (`read_coverage: size-mismatch`) instead of attributing a conversation to
  bytes it is not shown to have read. An unterminated trailing record is *disclosed* as what it is — the
  file's last bytes with no line feed, which the source may still be writing — and never claimed to be
  published or left out: whether a read carried it is the read's own answer, and the read's fidelity and
  the conversion's counts state it. The boundary is computed from the file's own bytes rather than from a
  provider's classification, the record offsets are claimed only where the provider's own declarations say
  the source is a transcript of appended lines *and* the file's own bytes can be one, and a compressed
  container of those records is reported by its bytes and digest with no record offset claimed for it.
- **A target session is discoverable where the target's own loader looks.** A target that finds a
  session only from the working directory its process runs in — Claude Code resolves a session through
  the directory whose project-dir spelling matches the one it was installed under, and Pi resolves a
  session id through the project directory spelled from the process's own working directory — is
  installed under the directory the source read recorded, and the printed resume command carries that
  directory as well as the target data home (`cd <cwd> && CLAUDE_CONFIG_DIR=<home> claude --resume
  <id>`, `cd <cwd> && PI_CODING_AGENT_DIR=<home> pi --session <id>`). A source that recorded no working
  directory is refused (`ErrNoTargetCwd`) rather than given an invented one, since a session installed
  anywhere else is one no loader finds. A native check of such a target runs the loader from the
  session's own recorded directory and against a **copy** of the converted bytes: measured on Pi 1.5.2,
  an RPC run appends its own records (a `thinking_level_change` among them) to the session it opens, so
  driving the loader against the installed file would alter what the conversion published.
  **Oh My Pi is the third such target, with a rule of its own on each half.** Its loader resolves a
  session by id from any directory, but refuses a session whose *recorded* working directory it cannot
  find (`Error: Session "<id>" belongs to a directory that no longer exists (…)`), so the load check runs
  from that directory when this host has it and reports the harness's own refusal when it does not. The
  directory it files sessions under is its own spelling of the working directory — `-tmp…` for a
  directory under the temporary tree, `-…` for one under the home, and Pi's `--…--` form only for a path
  outside both (measured against 18.18.7's own `getDefaultSessionDirPath`) — and it *moves* a session it
  finds under Pi's spelling into that directory itself, so the installer asks `OMPProjectDir` rather than
  reusing Pi's. Its printed line names the data directory and the directory that holds the session's file
  (`PI_CODING_AGENT_DIR`, `PI_CODING_AGENT_SESSION_DIR`): that variable names a directory of session
  files directly (naming the `sessions` root above a session's project directory answers that no such
  session is installed), and naming it is what keeps a caller's leftover value from hiding the session the
  pasted line was verified against. A load writes into the session it opens (measured: it appends a
  `custom` `session_exit` record), so its check drives a copy too, in a scratch data directory that
  declares its own provider definition for the converted file's own records without a credential of the
  caller's — `auth: none` in the Go verifier's scratch home (`internal/ccc/omp_verify.go`), the
  placeholder `apiKey` value `ccc-native-check` in the native check's loader data directory — a home with
  no model refuses to start the harness at all, and this way a load needs no credential. Every value such
  a home's own YAML spells (its provider's name, a model's id and name, `modelRoles.default`) is written
  as a **quoted** scalar rather than left to YAML's implicit typing: the ids are the target home's own
  configuration answers, so a provider or model spelled `123`, `null` or `2026-11-18` is ordinary, and a
  bare spelling would reach the harness as a number, a null or a timestamp — measured, the harness refuses
  such a session (`SessionModelRestoreError`).
  **OpenCode is the fourth, and its rule is its store's.** A session is a row in the harness's own
  SQLite store (`<XDG data home>/opencode/opencode.db`), not a file, so nothing but the harness can write
  one: this route's installation **is** the harness's own importer (`opencode session import … --directory
  <the session's recorded directory> --standalone`, the directory required to exist — measured: a missing
  one is refused with `LocationNotFoundError`), and a host without the binary has no installation at all,
  which the report states as the boundary it is. The load level asks the harness's own reader
  (`session export <id>`), never `run`: measured, `run` *creates* a session for an id the store does not
  hold and appends its own records, so a `run`-based load would answer for a session it had just made —
  and the same command's answer for an unknown id is the control arm (`Session not found: <id>`). An
  export changes none of a session's own records (measured: identical rows and an identical document
  around it), while the database *file's* bytes move with its WAL handling, which is why the check claims
  the records and not the file. A session is looked up from the directory its own document records
  (`session list --format json` names it there and not from another directory) and its model binding is
  the target home's own configured `<provider>/<model>`, asked of the harness (`debug agents`). Three
  members this format's importer requires are worth naming where a writer meets it: an assistant
  message's `model` member must be present on every assistant message although its values need not be
  (`{"id":"","providerID":""}` imports, is stored and re-exports as written), an assistant message whose
  `time` carries `created` alone is accepted and then **silently dropped** (measured on 2.2.5 by
  bisecting the message's members), and a tool part's `state.input` is stored with the JSON value and the
  recorded key order and without the recorded whitespace, so the comparison canonicalizes that member's
  JSON on both sides while every other member is compared as recorded.
- **A source that is one member of a container states that boundary, and its records are what a check
  re-reads.** The same store read *as a source* (goal 0008) resolves through the inherited provider — by
  `opencode:<session id>` under its configured root, or by the provider's own virtual path
  `<database>#<session id>` — and the conversion reports the boundary it cannot take:
  `source.boundary.state` is `not-taken`, naming the store and the member, and
  `installation.source_check_state` is `not-verified`, because the container's bytes hold every member and
  move with the harness's WAL handling. "The source is unchanged" is therefore measured on the harness's
  own records (`session export` before and after, byte-identical), never on the database file, and a Go
  case digests the store's whole directory around a conversion (no write, no journal created). What the
  read answers is the reader's own and is never re-derived here: the OpenCode v2 reader renders a
  `reasoning` part into the record's `Content` and records no trace text and no dialogue projection (so
  every written message is `unprojected_dialogue`, carried-with-caveat, and nothing is removed by
  inference), a `completed` tool part yields a textless progress event beside its terminal one (left out of
  the target and counted), and the store's `idle`/switch records are session controls rather than messages
  (nothing is counted for them because the read never held them).
- **Fixtures are sanitized native shapes.** Small, version-identified, committed fixtures — one
  provider directory per source (`fixtures/claude/`, `fixtures/codex/`, `fixtures/pi/`,
  `fixtures/omp/`) — with no real
  personal session content, no secrets and no absolute host paths in committed evidence; a fixture that
  must be driven by a harness whose lookup is path-scoped is copied into the run's own scratch tree with
  its recorded working directory rewritten mechanically, never edited in place. An Oh My Pi fixture sits
  one project directory below its provider root, because that provider's own discovery requires that
  level for this agent (measured: a file at the root is not discovered for `omp`, while the Pi provider
  accepts one there); the directory name is the absolute `--…--` spelling, the one branch of OMP's own
  rule that does not depend on the machine the generator runs on. OpenCode has **no committed store and no
  store-shaped fixture directory**: a session is a row in one store rather than a file, so a fixture could
  only be a store the harness (not this build) writes. What is committed is the harness's own **export
  document** for one session (`internal/ccc/testdata/fixtures/opencode/<session>.json`, written by
  `internal/ccc/fixturegen` with the one canonical encoding every other fixture line goes through — compact,
  measured: the importer stores a tool input as the parsed value and drops the recorded whitespace, so a
  pretty-printed fixture would make a store built from it answer an input spelling no harness store holds).
  A consumer materialises the store: the Go cases and `cmd/ccc`'s route cases through
  `internal/ccc/opencodefixture`, the native check through the harness's own `session import`. The target
  direction's own shapes are pinned as the document bytes the writer produces
  (`internal/ccc/opencode_target_test.go`).
- **Comments and docs state what the code does, not what it was meant to do.** A comment or test name
  claiming a rule the code no longer implements is a correctness defect.
- **A test pins the rule it names, on every platform and every Python 3 these docs name.** An
  assertion under `scripts/tests/` must fail when the behaviour it names is broken, and must not
  depend on the host *or on a language minor version*: no hard-coded path separators, no POSIX-only
  tool behaviour, no assumption that a tool is installed, and no `pathlib` behaviour that moved
  between 3.11 and 3.12 (`PurePath` parses its arguments in `__new__` before 3.12 and in `__init__`
  from 3.12 on, which is why the suite's Windows-flavoured root is a plain object holding
  `PureWindowsPath` children rather than a `PureWindowsPath` subclass). The *shipped* Python is held
  to that floor mechanically, not only by review: `scripts/tests/test_python_floor.py` parses every
  `scripts/**/*.py` — the Python this repository owns — with the running parser constrained to the
  oldest minor the docs name (`ast.parse(..., feature_version=(3, 11))`) and, where the running
  tokenizer is new enough to expose a replacement field's structure, reads the four PEP 701 f-string
  relaxations that constraint cannot see (a backslash, a comment, the literal's own quote, a field
  inside two format specs), each pinned against CPython 3.11.14's own verdicts; the suite's own
  modules are inside that scope, because discovery imports them before any case runs.

  The host's own filesystem answers are a seam of the same kind: `scripts/tests/_host_contract.py`
  measures them once at import — case sensitivity, symlink capability, `link/..` traversal, `O_EXCL` on
  a dangling link, mode-bit enforcement — and offers the fixture constructors (`make_abs_path`,
  `make_rooted_path`, `make_symlink`, `make_executable_stub`, `assert_create_refused`) so no case
  hand-rolls a POSIX premise; a *pure* path rule is instead a question about a dialect and is driven
  over both (`posixpath`/`ntpath`, `PurePosixPath`/`PureWindowsPath` — e.g. the Codex check's
  `absolute_launch_path(pathmod=…, cwd=…)`). `scripts/tests/test_host_contract.py`, run by the
  `selftest` step, lints every `scripts/tests/**/*.py` for the idioms that were each one red leg — a
  fixture anchored on a bare separator, a raw `os.symlink`/`Path.symlink_to`, an errno subclass pinned
  for a create collision (a deliberate injection marks itself `# host-contract: synthetic-errno`), a
  literal `':'` splitting or joining `PATH`, and an `os.access(..., X_OK)` assertion that does not
  consult `HOST_SEMANTICS.posix_exec_bits_enforced` — so the next such case fails on this host rather
  than in a CI leg.

  Where a rule
  cannot be exercised host-independently through a subprocess, the case drives the shipped module
  directly — `PATH` pinned to a stub bin, one probe patched, or a root whose children carry the
  Windows path flavour — instead of leaving the rule untested here and hoping a Windows job notices.
  The same applies to the *host probes* the gate makes before it looks at a tree: a case's stub bin
  answers the Go toolchain and the C compiler Windows needs, so a case whose subject is something
  else cannot fall into the Windows compiler boundary by forgetting a stub — a case that *is* about a
  missing tool says so, by pinning `PATH` to a bin without it or by patching the probe. The suite
  enforces that from the outside: two of its cases re-run the *whole* suite in a child process whose
  `PATH` holds no tools, the second with the host answering Windows-without-a-compiler
  (`scripts/tests/test_gate.py`, `HostCase`), so a case that leans on this machine, or on the platform
  it runs on, fails on this machine rather than in a CI job nobody can watch. The Go suites the `test`
  step runs are held to the same rule: a case whose subject is what a check makes of a target
  harness's answer drives the harness through the check's own seam, and answers the executable lookup
  the same way, instead of needing the harness installed on the machine running the suite; a case that
  *is* about a missing binary pins a `PATH` that holds no harness.

  Two shapes of that rule were measured on the `windows-latest` leg and are binding, on the Go side and
  in `scripts/tests/` alike:
  - **A path the CLI reports is asserted through the rule the CLI resolves it by.** The configuration
    resolves every configured session source before discovery reads it
    (`agentsview/internal/parser.ResolveProviderRoot`'s
    `agentsview/internal/pathutil.ResolveAbsolute`: absolute, symlinks resolved), so a case builds its
    expectation with that same rule rather than with the spelling it happened to pass:
    `cmd/ccc/opencode_source_route_test.go`'s `openCodeSourceStore` resolves the throw-away root before
    any expectation is spelled from it. Measured: `t.TempDir()` on that runner spells the short name
    (`…\RUNNER~1\…`) and the read answers the long one (`…\runneradmin\…`), so both of that file's
    inspection-shaped cases failed there against a correct report.
  - **A rendered line is asserted in the renderer's own quoting, and a case that hands a line to a
    reading builds it the way the renderer does.** `internal/ccc/continue.go`'s `shellQuote` quotes a word
    exactly where a POSIX shell would act on it, so a temp path is bare on POSIX and quoted where it holds
    backslashes (Windows): an assertion spelled against the host's own temp spelling pins whichever arm
    that host produces. A Go case therefore takes a home a shell would act on — the space in
    `cmd/ccc/main_test.go`'s `shellUnsafeHome`, which the Claude, Pi and OMP route cases copy — and a
    python case spells its line with `shlex.quote` and drives both path flavours (`PATH_FLAVOURS`), as the
    Codex check's own cases and `scripts/tests/test_opencode_native_check.py`'s `codex_invocation` do.
    Measured: the windows-latest leg failed a hand-spelled `CODEX_HOME=<home>` line and a
    `CODEX_HOME=<home>` substring assertion, both against output the renderer had spelled correctly.

## Commits and branches

- `type(scope): imperative summary`, types `feat|fix|refactor|docs|test|chore`.
- One goal per branch and per pull request; the branch is cut from freshly fetched `origin/main` and
  squash-merged, so `main` stays linear.
- Never commit generated build output, local caches, editor state or credentials. `.gitignore` is the
  first place to look before adding a directory.

## Uncertainty

An unattended increment must not stall on a question only the owner can answer. Take the most
reversible default, disclose it in the report as an explicit assumption, and append an entry to
`docs/decisions/OPEN-QUESTIONS.md` with the format that file already uses. Stop only for an
irreversible or unauthorized action, or when a missing owner-only input makes the stated outcome
impossible — and then report the blocker instead of substituting an easier target.
