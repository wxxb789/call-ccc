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
internal/           continuation core: conversation document, transform, target writers
docs/core-design/   the long-lived product and engineering boundaries
docs/goals/         numbered outcome contracts (archive/ once accepted)
docs/decisions/     append-only record of decisions taken and defaults chosen
scripts/            harness entry points (gate and its hooks)
```

Inherited AgentsView packages keep their upstream paths and stay where upstream put them.

## Rules of the work

- **Evidence over assertion.** Every `Done when` clause is paired with an artifact the operator can
  re-derive: a command with its real exit code, a file with its own bytes, a native loader's own
  output. A parser's acceptance of a file is not proof that a target harness can resume it.
- **Disclose loss by category.** Converted, installed and natively resumable are three different
  states; a report that conflates them is wrong even when the file is valid.
- **Non-destructive by default.** Never modify the source transcript, never overwrite an existing
  target session, never copy credentials or login stores, and never log conversation bodies, tool
  arguments or secrets by default. Write into isolated data directories for tests and demos.
- **Fixtures are sanitized native shapes.** Small, version-identified, committed fixtures; no real
  personal session content, no secrets, no absolute host paths in committed evidence.
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
  it runs on, fails on this machine rather than in a CI job nobody can watch.

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
