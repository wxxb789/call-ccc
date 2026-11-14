# Harness — call/ccc

One command decides whether a tree is acceptable:

```bash
python3 scripts/gate.py     # must print GATE: PASS; CI runs this same file
```

## What the gate does

| Step | Tool | What it proves |
| --- | --- | --- |
| `gofmt` | `gofmt -l` over the tree's `*.go` (below) | formatting is not drifting |
| `build` | `go build <flags> <targets>` with `CGO_ENABLED=1` | the tree compiles with the inherited SQLite driver |
| `vet` | `go vet <flags> <targets>` | no obvious static defects |
| `test` | `go test -count=1 <flags> <targets>` | the tests actually run and pass |
| `selftest` | `python3 -m unittest discover -s scripts/tests` | the harness's own rules still hold |
| `parity` | `scripts/upstream-parity.py` (required) | every carried AgentsView file still matches the pinned revision |
| `secrets` | `gitleaks detect --no-git --source <scope>` when available | no credentials in the files this repository owns |

`gofmt` is the one step that also has to answer in a tree without git: it takes `git ls-files '*.go'`
when the tree is a repository (the normal case, CI included), and otherwise walks the tree skipping
`.git/` and a top-level `vendor/`. Both branches drop the same two things, and a nested `vendor/`
stays — a Go `vendor` directory is a module-root concept. Both branches also answer in one spelling:
POSIX-form relative paths (`deep/vendor/nested.go`) on every platform, which is what `git ls-files`
prints and what the walk normalises to with `as_posix()`. The list handed to `gofmt` — and any test
that pins it — therefore does not depend on the host's separator, and a regression to `str(relative)`
is what the suite's Windows-flavoured case exists to catch.

`selftest` runs the suite with the interpreter that is running the gate (`sys.executable`), so the
suite must hold on every Python 3 the docs allow and not only on CI's pinned 3.13: it is stdlib-only
Python 3, and no case may depend on a `pathlib` behaviour that moved between minor versions —
`PurePath` parses its constructor arguments in `__new__` before 3.12 and in `__init__` from 3.12 on,
which is why the suite's Windows-flavoured root is a plain object holding `PureWindowsPath` children
instead of a `PureWindowsPath` subclass.

The same step must not depend on the *host* either, so two of its cases re-run the whole suite in a
child process — once with `PATH` pointing at a directory that holds no tools at all, once with the
platform probe answering the way Windows answers without a C compiler (`sys.platform`, and the
suite's own Windows-shaped tool lookup) — and assert that every case still ran and none flipped. The
child's copies of those two cases skip, keyed on `CALLCCC_SUITE_SELFCHECK`, so a nested run does not
nest a third. A case that reaches for a tool this host happens to have, or that reads the platform it
runs on, therefore fails in the selftest step instead of only in the one CI job nobody can watch; the
Windows answer is a simulation executed on the running host, never an assertion from memory about a
machine nobody in the loop has.

## `parity`: the carried tree against its pin

`scripts/upstream-parity.py` answers, offline, whether the AgentsView source this repository carries
is still the pinned revision: `docs/upstream.md` states the pin, the deviations and the update
procedure; `scripts/upstream-manifest.txt` is the machine-readable record. The step is **required**
(`scripts/gate-required.txt`), so a run in which it is skipped, bounded or absent fails.

It verifies, in this order:

1. every carried path is what git stores for it (`git ls-files -s`, i.e. what a fresh clone receives,
   with `.gitattributes` filters applied) — or, in a tree without git, what its bytes hash to;
2. no carried file is modified in the working tree relative to what git stores (`git status`
   porcelain, worktree column only: a staged-but-uncommitted tree is the normal state here);
3. every declared deviation still differs from upstream (a project-owned file that has quietly
   become upstream's copy is a stale declaration, not a pass);
4. the generated build input hashes to the SHA-256 that the carried snapshot tool pins in its own
   source, so the input is verified against upstream's pin rather than a second hard-coded copy;
5. `docs/upstream.md` records the pin, and `cmd/ccc/pin.go` reports it, so the revision the binary
   claims cannot drift from the revision the manifest describes;
6. nothing git would add sits inside the tree unexplained: a tracked or untracked-but-not-ignored
   path is either named by the manifest or inside a prefix the manifest calls project-owned.

`scripts/upstream-compare.py` answers the other half — behaviour — and is deliberately **not** a gate
step, because it needs network and a second checkout: it fetches the pinned revision into a scratch
directory, copies `internal/ccc` into it so both sides run the *same* oracle program over the *same*
fixture bytes, runs parse → sync → archive read on both sides in their own throw-away archives, and
diffs the two canonical dumps. `internal/ccc/parity/testdata/oracle/*.json` holds the dumps a
pristine checkout produced; the ordinary test suite re-checks this tree against them offline. The
increment's evidence record holds a run of it with its hashes.

## `secrets`: what it scans, and what it does not

The scan reads the files this repository **owns** — the prefixes the carry manifest records as
project-owned — staged into a throw-away directory so `--no-git` still means "current files only,
untracked and gitignored files included" within that scope. The carried AgentsView tree is not
scanned here: it is upstream's content, byte-verified by the `parity` step, and upstream's own
secret-detector corpora (`internal/secrets/rules_test.go` and friends) exist precisely to contain
credential shapes. The step's PASS line states the scope and the file count, so a run can never be
read as having scanned more than it did. A tree with no readable carry manifest is scanned whole, and
says so.

## Knobs (all in-repo, all auditable)

- `scripts/gate-targets.txt` — one Go package pattern per line (`#` comments allowed) for build/vet/
  test. Absent means `./...`.
- `scripts/gate-goflags.txt` — extra Go flags, one per line, folded into build/vet/test ahead of the
  targets. It carries `-tags fts5`, which is not cosmetic: without it the inherited SQLite driver has
  no FTS5 module, so a binary built here could not load an archive that has a full-text index, and
  the step would be quietly building a different product than upstream ships.
- `scripts/gate-required.txt` — one step name per line. A step listed here must be `PASS`: `SKIP`,
  `BOUNDARY`, `N/A` and `FAIL` all fail the gate, and so does an unknown step name. It lists `parity`
  and `selftest`. The Go steps are deliberately *not* listed: on Windows without a C compiler they are
  a disclosed platform boundary (below), not a failure.
- `scripts/upstream-parity.py` — the source-side parity check (above).
- `scripts/tests/` — the gate's, the parity checker's and the importer's regression tests, run by the
  `selftest` step.
- `CALLCCC_GATE_ROOT` — run the gate against another tree. Most cases in `scripts/tests/` pin a rule
  against a throw-away tree instead of trusting the reader.

## Why the Go steps target a subset

`scripts/gate-targets.txt` is `./cmd/ccc/... ./internal/ccc/...`, and that is a disclosure, not a
convenience. Measured on the import host (4 cores, 7 GB, Ubuntu):

| Inherited suite | Result |
| --- | --- |
| `go test -tags fts5 ./internal/parser` | PASS, 156 s |
| `go test -tags fts5 ./internal/db` | build fails: the `sqlite-vec` cgo binding needs `sqlite3.h`, which upstream's `Makefile` stages from the module cache (`sqlite-vec-header`) |
| `go test -tags fts5 ./internal/sync` | exceeds a ten-minute default timeout |
| the module's PostgreSQL / ClickHouse / DuckDB lanes | start containers and are upstream's own CI's job |

So the local gate builds and tests what call/ccc builds and depends on — which compiles the inherited
`internal/parser`, `internal/db`, `internal/sync` and their dependencies anyway — and the inherited
*tests* are covered by `parity` (the files are the pin's, byte for byte) plus the oracle comparison
(those code paths run on fixtures, in both trees). Running more than that belongs to upstream's CI
and to `docs/upstream.md`'s update procedure, not to a gate that has to finish on a laptop.

## Platform boundary

Linux, macOS and Windows are the intended platforms, subject to native harness availability. On Linux
and macOS a missing Go toolchain or a missing C compiler is a **failure**: the gate cannot silently
degrade where the project is expected to build fully. On Windows a missing C compiler is a disclosed
boundary — the gate runs what it can and says so — because CGO on that platform depends on the
toolchain being installed separately. That boundary is the one host answer a step can turn on, so the
suite answers it from the case's own stub bin and exercises the boundary itself by forcing the
platform probe to its Windows answer (see the `selftest` section): no verdict in the suite depends on
which platform is running it.

Windows has no POSIX mode bits, so `parity` declines to check a carried file's executable bit there
and says so in its output instead of failing every executable file; the *stored* mode still comes
from git's own index, which records it on every platform.

## CI

`.github/workflows/gate.yml` runs the same `scripts/gate.py` on `ubuntu-latest`, `macos-latest` and
`windows-latest` for every pull request and every push to `main`. Its step spells the interpreter
`python`, the name all three runners have — the Windows runner has no `python3` — while the file is
`#!`-tagged `python3` for the hosts where that is the only name. CI installs Go and gitleaks; a CI job
with fewer tools than another is the boundary above, not a different gate. The Go toolchain comes
from `go.mod` (`go-version-file`), so the workflow and the module cannot disagree about the version
the tree builds with.

## Growth policy

The gate grows by **adding a step to `scripts/gate.py` and (when it must always run) a line to
`scripts/gate-required.txt`** — not by adding a second script that CI calls separately, and not by
adding a check that only runs on the author's machine. A check that exists in a shell history or a PR
description does not exist. When a new check is introduced, this file and `AGENTS.md` say so in the
same increment.
