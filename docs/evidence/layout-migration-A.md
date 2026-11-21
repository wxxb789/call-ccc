# Layout migration A — evidence

**Increment:** the carried AgentsView tree moved into `agentsview/`, this repository's own module into
`ccc/` (owner decision: layout **A**, 2026-11-21). **Plan:**
[`docs/plans/layout-migration-A.md`](../plans/layout-migration-A.md). **Branch:**
`prep/layout-migration-A`, cut from `origin/main` = `21d43fe1a3c81bb59504bfaae23c8c618edf7ae2`.
Every command below ran in this worktree; the toolchain is the one on this host's `PATH` (Go 1.27.4
via its shim, Python 3.15.2 for the harness).

Nothing here is a native harness run: no target harness was driven, so no `verification.load` /
`verification.resume` state appears — this increment moves files and modules, and the checks that
matter are `parity` (the carry is still the pin's bytes), the Go steps (our module builds and its tests
pass), `portability` (it builds and tests against a *pristine* checkout of the pin) and `selftest` (the
harness's own rules still hold).

## 1. What moved

The move was driven by the committed ledger, never by hand: every carried path the manifest named went
to `agentsview/<path>`, and everything the manifest called project-owned stayed. Merged directories
(`docs/`, `scripts/`, `cmd/`, `internal/`) were split by that rule — whole subtrees moved as
directories where the ledger said every file under them was upstream's (`agentsview/internal/**`,
`agentsview/docs/{agents,internal,website,screenshots,scripts,stylesheets,javascripts,overrides,assets}/**`,
`agentsview/scripts/{memory-e2e,qwenpaw-fixtures}/**`, `agentsview/frontend/**`, …, 138 directory moves)
and the remaining files one by one (60 file moves). Our own half moved first
(`internal/ccc` → `ccc/internal/ccc`, `cmd/ccc` → `ccc/cmd/ccc`), so the mixed trees were clean when the
carried subtrees left them.

```
$ git status --short | grep -c "^R"      # renames staged by the move
5638
$ git ls-files | wc -l                   # tracked paths before and after the move
5763
$ ls -A                                  # the top level afterwards
.git-blame-ignore-revs .github .gitignore AGENTS.md README.md agentsview ccc docs scripts
```

Nine of the renamed files were also edited: the self-import rewrite
`go.kenn.io/agentsview/internal/ccc` → `go.kenn.io/agentsview/ccc/internal/ccc` (and
`…/cmd/ccc` → `…/ccc/cmd/ccc`) in `ccc/cmd/ccc/{main.go,main_test.go,omp_route_test.go,opencode_route_test.go,opencode_source_route_test.go}`,
`ccc/internal/ccc/{continue_opencode_source_test.go,opencode_install_test.go}`,
`ccc/internal/ccc/parity/dump.go` and `ccc/internal/ccc/parity/oracle/main.go`. The carried bytes are
not among them: the compiler is what catches an import the rewrite missed, and the `parity` step is
what catches a carried byte that moved.

## 2. The ledger re-derived at the new prefix

The importer reads the tree's own shape (its `go.mod` pair) and writes the carry into the directory
that shape names, so the ledger was re-derived by the tool that owns it — not hand-edited:

```
$ python3 scripts/upstream-import.py --upstream ~/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a, 2026-11-14T16:19:32+00:00)
layout: carry root agentsview/, this repository's module ccc
upstream tracked files: 5580
carried byte for byte:  5579
declared deviations:    2 (0 upstream paths not carried, 1 carried with a declared normalisation, 1 generated input)
project-owned prefixes: 0
rc=0
```

**Deviations 28 → 2**, and the 26 that stopped being deviations are exactly the ones the layout
answered — no reason was dropped, each became a carried file:

| Before (28) | After | Why it is no longer declared |
| --- | --- | --- |
| 4 × project-owned upstream paths: `.gitignore`, `AGENTS.md`, `README.md`, `docs/README.md` | carried as `agentsview/<path>` | each side has one home now: upstream's copies travel inside the carry, this repository's stay at the root |
| 22 × upstream `.github/**` skipped | carried as `agentsview/.github/**` | GitHub reads only the repository root's `.github/`, so upstream's CI definitions are inert inside the carry — and are carried verbatim like the rest of the tree |
| 1 × `frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json` carried with one byte normalised | unchanged, now at `agentsview/frontend/…` | the byte git stores still differs from upstream's blob (upstream's own `.gitattributes` rewrites the CRLF pair); the record still pins both digests |
| 1 × generated build input `internal/pricing/snapshot/litellm_snapshot.json.gz` | unchanged, now at `agentsview/internal/pricing/…` | upstream still does not track it; it is still verified against the SHA-256 the carried snapshot tool pins |

The ledger's `layout` line is not decoration: the checker refuses a ledger that disagrees with the
tree's own `go.mod` files, which is what the state *between* the move and the re-derivation looked like:

```
$ python3 scripts/upstream-parity.py     # (ledger still said 'layout . .', the tree was already moved)
parity: FAIL — the manifest's layout line says '. .', but this tree's own go.mod files are 'agentsview/ ccc' -- the ledger and the tree disagree about where the carried tree sits
  scripts/upstream-manifest.txt
rc=1
```

And the answer at the new prefix, in full:

```
$ python3 scripts/upstream-parity.py
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a)
reported by ccc/cmd/ccc/pin.go: 83195c8a6861b8c74a4b6cb87512817fed6f8584
layout: carry root agentsview/, this repository's module ccc
carried: 5579 file(s) identical to the pinned revision (compared through git's own object store)
deviations: 1 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared
owned: 0 project-owned prefix(es), plus the carry directory's own boundary; files seen by git: 5763
inputs: agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by agentsview/internal/pricing/cmd/litellm-snapshot/main.go)
symlinks hashed by their link target: 4
declared stored bytes verified: agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
rc=0
```

For comparison, the same step on the pre-move tree (one module, the carry at the root) printed
`carried: 5553 file(s) identical to the pinned revision … 27 upstream path(s) declared … + 1 generated
input(s) … owned: 32 project-owned prefix(es); files seen by git: 5733` with rc=0. `5579 = 5553 + 22 +
4`: the carried set grew by exactly the deviations that stopped being deviations, and the 5553 paths
that were carried before are carried now.

## 3. The carried bytes did not move: a pre/post spot check

`sha256` of three paths that existed before the move and after it — one carried Go source, one of our
own sources, one carried fixture (the 1.4 MB one whose record pins a normalisation) — taken before the
move and again after it:

```
# before (as internal/parser/claude.go, internal/ccc/continue.go, frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json)
94f0c8e6a6b8788f11e51d985c621db7ac41d100d10717b09746f3519412e381  internal/parser/claude.go
99ba84c07b438a2671cde5eac528395fbfacee85a879a4ff3cc29a331bd1a14b  internal/ccc/continue.go
e2a390c7c046dd3ffce54c4a6fcd6ada0e5d4028f212390d5eb805708de47df3  frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json

# after (their new paths)
94f0c8e6a6b8788f11e51d985c621db7ac41d100d10717b09746f3519412e381  agentsview/internal/parser/claude.go
99ba84c07b438a2671cde5eac528395fbfacee85a879a4ff3cc29a331bd1a14b  ccc/internal/ccc/continue.go
e2a390c7c046dd3ffce54c4a6fcd6ada0e5d4028f212390d5eb805708de47df3  agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
```

Identical, and the engine's own bytes are unchanged by the module split (`continue.go` is our file and
it was not edited — only its import block's siblings were). The stronger statement is `parity`'s: 5579
carried files compared through git's own object store, plus the 120000-mode links hashed by their
target and the stored-byte deviation verified.

## 4. Our module, built and tested (and the gate's new step)

```
$ cd ccc && GOFLAGS=-mod=mod CGO_ENABLED=1 go vet -tags fts5 ./cmd/ccc/... ./internal/ccc/...
VET rc=0
$ cd ccc && GOFLAGS=-mod=mod CGO_ENABLED=1 go test -count=1 -tags fts5 ./cmd/ccc/... ./internal/ccc/...
ok  	go.kenn.io/agentsview/ccc/cmd/ccc	6.204s
ok  	go.kenn.io/agentsview/ccc/internal/ccc	20.408s
?   	go.kenn.io/agentsview/ccc/internal/ccc/fixturegen	[no test files]
?   	go.kenn.io/agentsview/ccc/internal/ccc/opencodefixture	[no test files]
ok  	go.kenn.io/agentsview/ccc/internal/ccc/parity	1.125s
?   	go.kenn.io/agentsview/ccc/internal/ccc/parity/oracle	[no test files]
rc=0
```

The new `portability` step is the property the two-module shape exists for, so its own run is quoted in
full — once by hand (below, on the cold cache, which is what fetched the checkout) and once through the
gate's step line (§7, which reused the fetched checkout):

```
$ python3 scripts/portability.py
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a)
module ccc built and tested against the pristine checkout at /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584
ran with -modfile=portability.mod (replace -> /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584) and portability.sum beside it
go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
3 package(s) ok
rc=0
```

The harness's own suite grew by the cases that hold the new tooling (31 for `scripts/portability.py`
and one for the two modules' `go` directive; the layout rules themselves are covered where they live):

```
$ PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -p 'test_*.py'
Ran 475 tests in 11.721s
OK
```

The five native checks are **not** gate steps and were not run (see "what this record does not
claim"); what was verified for them is that their suites still pass with the CLI built from `ccc/` and
their fixture roots under the module — `python3 -m unittest scripts.tests.test_{claude,codex,pi,omp,opencode}_native_check`
→ `Ran 250 tests … OK`.

## 5. The boundary probe, re-run in this repository

The two-module feasibility was proven before anything moved; the probe was re-run from this worktree
afterwards (it builds its own throw-away modules, so it needs no network and touches nothing here):

```
$ python3 ~/.hermes/skills/software-development/upstream-tree-carry/scripts/go-internal-boundary-probe.py
PASS: A carry in its own folder + subpath sibling module
      exit=0
PASS: B carry at the root + nested subpath module
      exit=0
PASS: C unrelated module path (must be refused)
      exit=1
      package example.com/unrelated
      	main.go:6:2: use of internal package go.kenn.io/agentsview/internal/db not allowed

PROBE: a module whose path is a SUBPATH of the carrying module's path may import its
       internal/ packages through a replace; an unrelated module path is refused.
       => the carry can live in its own folder with its own go.mod, bytes untouched.
rc=0
```

The shape this tree now has is exactly arm A: the carry in `agentsview/` with the pin's own `go.mod`,
and `ccc/` whose module path (`go.kenn.io/agentsview/ccc`) is a subpath of the carrying module's, wired
by one `replace` and no `go.work`.

## 6. Behaviour, not only bytes

The gate is offline, so `scripts/upstream-compare.py` (deliberately not a gate step) is the check that
the inherited code still *behaves* like the pin's. It was updated for the layout — the oracle is copied
into the pristine checkout at the same subpath (`ccc/internal/ccc`), which is what keeps its own import
paths resolvable there — and re-run against a clean checkout of the pin:

```
$ python3 scripts/upstream-compare.py --upstream ~/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) from https://github.com/kenn-io/agentsview
- copied ccc/internal/ccc (55 Go source file(s)) from this tree into the pristine checkout, where the same subpath keeps the oracle's own import paths (go.kenn.io/agentsview/ccc/...) resolvable: the oracle and its call/ccc wrapper are identical on both sides, so the only variable is which tree the inherited packages come from
oracle source: 55 file(s), sha256=86651a5972404caa483b1cd6ae8c7090f276cf09613df7f3e245d0de56233016 (identical on both sides)
claude: fixture sha256=2bd510821d4abf83… ours=6409a23c502469a3… pristine=6409a23c502469a3… agreement=yes
codex: fixture sha256=d9957a8d6f5731a3… ours=531e5c0fb19fc51a… pristine=531e5c0fb19fc51a… agreement=yes
pi: fixture sha256=7395c3b0309a0950… ours=6751fa8b209e9077… pristine=6751fa8b209e9077… agreement=yes
RESULT: PASS — 3 case(s) agree byte for byte with the pinned revision
- removed …/ccc/internal/ccc from the checkout again
- …/83195c8a6861b8c74a4b6cb87512817fed6f8584 is clean
rc=0
```

## 7. The gate, on the state this increment leaves

```
gate: /home/azureuser/claw/tasks/2026-11-14/call-ccc/wt-s1/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go module=ccc targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt       3624 file(s) gofmt-clean
  PASS     build       go build ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     vet         go vet ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     test        3 package(s) ok (in ccc)
  PASS     portability pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) | module ccc built and tested against the pristine checkout at /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584 | ran with -modfile=portability.mod (replace -> /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584) and portability.sum beside it | go build ./cmd/ccc/... ./internal/ccc/... -> rc=0 | 3 package(s) ok
  PASS     selftest    Ran 475 tests in 10.951s (rc=0)
  PASS     parity      … | : 83195c8a6861b8c74a4b6cb87512817fed6f8584 | layout: carry root agentsview/, this repository's module ccc | carried: 5579 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 1 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 0 project-owned prefix(es), plus the carry directory's own boundary; files seen by git: 5765 | inputs: agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by agentsview/internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared stored bytes verified…
  PASS     citations   docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets     no leaks found in 186 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
rc=0
```

That run was taken with this increment's files in place except for this record, the plan, and the
scratch-leak fix of §8 (three files of prose, tests and ignore rules staged afterwards). Those touch the
`parity` step's seen-file count, the `secrets` scan's file count and the selftest's case count, and
nothing else — no Go source, no carried byte and no gate knob's pattern. The record's own claims are
held by the `selftest` step it quotes: `test_evidence_claims.py` reads this file like every other
record. The gate was re-run on the final tree and printed the same nine statuses and `GATE: PASS`; the
increment's report quotes that run verbatim.

## 8. The `parity-root-*` scratch leak

Eighty-one empty `parity-root-*` directories were found in the repository root on 2026-11-21. The cause
is the shape `tempfile.gettempdir()` answers for a *relative* (or repository-relative) `TMPDIR`: every
throw-away tree is created relative to the process's current directory, which is the repository root
when the gate runs the suite, and a run that dies before its cleanup leaves the directory behind. Three
defences now exist, and each has a case:

- `scripts/tests/test_upstream_parity.py`'s `scratch_tree()` (used by every one of the suite's
  throw-away trees, including the comparison base) resolves the base to an absolute path, refuses one
  inside the repository, and falls back to the platform's own scratch root; a killed run can no longer
  leave anything in the tree, and `ScratchTreeCase` pins the rule for an inside-the-repository `TMPDIR`
  and for a relative one.
- `scripts/portability.py`'s `scratch_directory()` applies the same rule to the shipped checker (its
  scratch holds the alternate `go.mod`), with its own cases in `scripts/tests/test_portability.py`.
- `.gitignore` names the four patterns (`/parity-root-*/`, `/import-root-*/`, `/compare-checkout-*/`,
  `/portability-root-*/`), so a directory that appears anyway is never something `git status` offers to
  commit. Measured:

```
$ mkdir -p parity-root-verify portability-root-verify && git check-ignore -v parity-root-verify portability-root-verify
.gitignore:28:/parity-root-*/	parity-root-verify
.gitignore:31:/portability-root-*/	portability-root-verify
$ git status --short | grep -c "root-verify"      # ignored directories are not offered to commit
0
```

`upstream-parity.py` itself never created a scratch root: it is offline and hashes the tree in place,
which is why the leak's carrier was the *suite's* trees and the fix belongs to their helper. The
importer and the comparison script take the checkout they are handed (`--upstream`) or a scratch
directory outside the tree.

## What this record does not claim

- **No native check ran.** No target harness (Codex, Claude Code, Pi, OMP, OpenCode) was installed and
  no turn was taken, so `verification.load` / `verification.resume` states are absent from everything
  above; the five native checks were not executed (their *suites* were, with the CLI built from the new
  module path — that is not the same thing, and is not quoted as it).
- **CI has not run on this head.** `gate (ubuntu-latest)` and `gate (windows-latest)` are the operator's
  next step; the workflow's Go toolchain now comes from `ccc/go.mod`, and a case in
  `scripts/tests/test_upstream_parity.py` holds that file's `go` directive to the carried tree's — but
  no runner has executed this commit, and the Windows leg in particular is where the layout's path
  lengths and case behaviour would show.
- **The `AGENTS.md` update is not in this tree.** It is held for owner approval as an intended patch
  (`S1-agents-md-intended.patch`: the path statements the migration invalidates, the gate's new step,
  and the carry's location). This increment's `AGENTS.md` is byte-identical to `main`'s.
- **`docs/decisions/OPEN-QUESTIONS.md` and the earlier evidence records are not rewritten.** Their
  dated entries keep the paths of the day they were written; `docs/upstream.md` gained the old → new
  mapping, and both are deliberate (historical records are not restated, they are translated).
- **The carried tree's own suites were not run** (they are upstream's CI's job, and `internal/db` needs
  a staged `sqlite3.h`); `parity` proves their bytes, `portability` proves our module builds against
  them, and `upstream-compare` proves the code paths it exercises behave identically.
- **No ledger row, no goal touched.** This increment has no numbered goal, so it writes no
  `<goal>-citations.tsv`, moves nothing into `docs/goals/archive/`, and changes no `Done when` clause.
- **`.git-blame-ignore-revs` names the rule, not a SHA.** The squash commit that lands this migration
  does not exist yet; the file says how the operator appends it.
