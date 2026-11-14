# Harness — call/ccc

One command decides whether a tree is acceptable:

```bash
python3 scripts/gate.py     # must print GATE: PASS; CI runs this same file
```

## What the gate does

| Step | Tool | What it proves |
| --- | --- | --- |
| `gofmt` | `gofmt -l` over the tree's `*.go` (below) | formatting is not drifting |
| `build` | `go build <targets>` with `CGO_ENABLED=1` | the tree compiles with the inherited SQLite driver |
| `vet` | `go vet <targets>` | no obvious static defects |
| `test` | `go test -count=1 <targets>` | the tests actually run and pass |
| `selftest` | `python3 -m unittest discover -s scripts/tests` | the harness's own rules still hold |
| `parity` | `scripts/upstream-parity.py` when present | inherited AgentsView source matches its pinned upstream revision |
| `secrets` | `gitleaks detect --no-git --source .` when available | no credentials in the working tree (below) |

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

`secrets` scans wider than the repository: `--no-git` means "current files only" and leaves no
gitignore switch, so the scan reads the whole working tree, untracked and gitignored files included. A
local `.env` therefore fails the gate locally where a clean CI checkout passes; the step is a
working-tree check, not proof that nothing was ever committed.

Exit code 0 means every step that **ran** passed. A step that could not run is printed as
`SKIP`/`BOUNDARY` with its reason, and a step whose hook is not in the tree as `N/A`; none of them is
silently counted as a pass, and the verdict line names every one of them — `GATE: PASS (partial: …)` —
so a partial run is visible in CI logs and in any transcript quoting it.

## Knobs (all in-repo, all auditable)

- `scripts/gate-targets.txt` — one Go package pattern per line (`#` comments allowed) for
  build/vet/test. Absent means `./...`. Narrowing it is a disclosure obligation: say why in this file
  and in the increment's report.
- `scripts/gate-required.txt` — one step name per line. A step listed here must be `PASS`: `SKIP`,
  `BOUNDARY`, `N/A` and `FAIL` all fail the gate, and so does an unknown step name — the verdict is
  computed from the effective status, so listing a step cannot silently become a footnote. This is how
  a check is promoted from "runs when it can" to "must run".
- `scripts/upstream-parity.py` — the source-side parity check. The gate runs it when it exists; the
  increment that introduces the pinned upstream revision is the one that adds it and lists `parity` in
  `scripts/gate-required.txt`.
- `CALLCCC_GATE_ROOT` — run the gate against another tree. Most cases in `scripts/tests/test_gate.py`
  pin the required-step rule against a throw-away tree instead of trusting the reader; the ones that
  need a status — or a git branch — no installed toolchain can produce import the shipped file and
  drive it in-process instead.

## Platform boundary

Linux, macOS and Windows are the intended platforms, subject to native harness availability. On Linux
and macOS a missing Go toolchain or a missing C compiler is a **failure**: the gate cannot silently
degrade where the project is expected to build fully. On Windows a missing C compiler is a disclosed
boundary — the gate runs what it can and says so — because CGO on that platform depends on the
toolchain being installed separately.

## CI

`.github/workflows/gate.yml` runs the same `scripts/gate.py` on `ubuntu-latest`, `macos-latest` and
`windows-latest` for every pull request and every push to `main`. Its step spells the interpreter
`python`, the name all three runners have — the Windows runner has no `python3` — while the file is
`#!`-tagged `python3` for the hosts where that is the only name. CI installs Go and gitleaks; a CI job
with fewer tools than another is the boundary above, not a different gate.

## Growth policy

The gate grows by **adding a step to `scripts/gate.py` and (when it must always run) a line to
`scripts/gate-required.txt`** — not by adding a second script that CI calls separately, and not by
adding a check that only runs on the author's machine. A check that exists in a shell history or a PR
description does not exist. When a new check is introduced, this file and `AGENTS.md` say so in the
same increment.
