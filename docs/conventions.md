# Conventions — call/ccc

Binding for every increment. When this file and a goal file disagree about acceptance, the goal file
wins; when they disagree about structure or method, this file wins unless the increment records why.

## Stack

- **Go**, first and only shipping language for the CLI, the continuation engine and the inherited
  AgentsView code.
- The module path and ancestry are inherited from AgentsView (`go.kenn.io/agentsview`) so that
  upstream-owned packages stay importable and diffable. `cmd/ccc` and the isolated continuation
  packages are added **inside the permitted import tree**; upstream imports are never bulk-renamed to
  invent a new module path, and Go's internal-package rule is a boundary to design around, not to
  work around.
- SQLite access goes through the inherited AgentsView path, including the `fts5` build tag and CGO
  where the inherited driver needs them. `call/ccc` never opens the archive with its own driver,
  schema migration, watcher or writer.
- No second language is introduced for scripting. Non-Go tooling is Python 3 (stdlib only) or the
  existing shell-free entry points under `scripts/`.

## Layout

```
cmd/ccc/            the standalone CLI; no continuation-core types leak into it
internal/ccc/       continuation core: conversation document, readers, archive access,
                    target writers and the route table (targets.go, codex_target.go,
                    codex_install.go, codex_verify.go, claude_target.go, claude_install.go,
                    claude_verify.go, pi_target.go, pi_install.go, pi_verify.go, continue.go),
                    and the install/verify/probe machinery the three routes share (route_shared.go)
internal/ccc/parity the canonical dump and the oracle a pristine checkout runs too
internal/ccc/testdata/fixtures  committed native-shaped sessions, written by internal/ccc/fixturegen
internal/ccc/testdata/growth    committed fragments a run appends to a *copy* of one fixture, so one
                    source session grows; the same generator writes and pins them
docs/core-design/   the long-lived product and engineering boundaries
docs/goals/         numbered outcome contracts (archive/ once accepted)
docs/plans/         one plan per increment, written before its implementation
docs/evidence/      one evidence record per increment: real commands, exit codes, counts
docs/upstream.md    the carried AgentsView pin, its declared deviations and the update procedure
docs/decisions/     append-only record of decisions taken and defaults chosen
scripts/            harness entry points (gate, knobs, carry importer, parity, comparison,
                    codex-native-check, claude-native-check and pi-native-check: the
                    target-side native verifications, deliberately not gate steps)
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
- Carried code is edited under upstream's own guides (`docs/agents/*`), which are carried with it;
  this repository's rules (this file, `AGENTS.md`) govern everything else.
- The generated input `internal/pricing/snapshot/litellm_snapshot.json.gz` is committed although
  upstream does not track it (upstream's Makefile restores it before every Go build), is ignored by
  the carried `snapshot/.gitignore`, and therefore needs `git add -f` when it is first added.

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
  model-visible history. The provider a record states is the provider the read actually answered — the
  target's own configured endpoint where the harness was asked, and none where the read answers the model
  alone — never the source's, and never invented.
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
- **Fixtures are sanitized native shapes.** Small, version-identified, committed fixtures — one
  provider directory per source (`fixtures/claude/`, `fixtures/codex/`, `fixtures/pi/`) — with no real
  personal session content, no secrets and no absolute host paths in committed evidence; a fixture that
  must be driven by a harness whose lookup is path-scoped is copied into the run's own scratch tree with
  its recorded working directory rewritten mechanically, never edited in place.
- **Comments and docs state what the code does, not what it was meant to do.** A comment or test name
  claiming a rule the code no longer implements is a correctness defect.
- **A test pins the rule it names, on every platform and every Python 3 these docs name.** An
  assertion under `scripts/tests/` must fail when the behaviour it names is broken, and must not
  depend on the host *or on a language minor version*: no hard-coded path separators, no POSIX-only
  tool behaviour, no assumption that a tool is installed, and no `pathlib` behaviour that moved
  between 3.11 and 3.12 (`PurePath` parses its arguments in `__new__` before 3.12 and in `__init__`
  from 3.12 on, which is why the suite's Windows-flavoured root is a plain object holding
  `PureWindowsPath` children rather than a `PureWindowsPath` subclass). Where a rule cannot be
  exercised host-independently through a subprocess, the case drives the shipped module directly —
  `PATH` pinned to a stub bin, one probe patched, or a root whose children carry the Windows path
  flavour — instead of leaving the rule untested here and hoping a Windows job notices. The same
  applies to the *host probes* the gate makes before it looks at a tree: a case's stub bin answers the
  Go toolchain and the C compiler Windows needs, so a case whose subject is something else cannot fall
  into the Windows compiler boundary by forgetting a stub — a case that *is* about a missing tool says
  so, by pinning `PATH` to a bin without it or by patching the probe. The suite enforces that from the
  outside: two of its cases re-run the *whole* suite in a child process whose `PATH` holds no tools,
  the second with the host answering Windows-without-a-compiler
  (`scripts/tests/test_gate.py`, `HostCase`), so a case that leans on this machine, or on the platform
  it runs on, fails on this machine rather than in a CI job nobody can watch. The Go suites the `test`
  step runs are held to the same rule: a case whose subject is what a check makes of a target
  harness's answer drives the harness through the check's own seam, and answers the executable lookup
  the same way, instead of needing the harness installed on the machine running the suite; a case that
  *is* about a missing binary pins a `PATH` that holds no harness.

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
