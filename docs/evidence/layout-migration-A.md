# Layout migration A — evidence

**Increment:** the carried AgentsView tree moved into `agentsview/`, this repository's own module into
`ccc/` (owner decision: layout **A**, 2026-11-21). **Plan:**
[`docs/plans/layout-migration-A.md`](../plans/layout-migration-A.md). **Branch:**
`prep/layout-migration-A`, cut from `origin/main` = `21d43fe1a3c81bb59504bfaae23c8c618edf7ae2` and later
**rebased onto `8989d2c`** (PR #20 merged) — the rebase, its one conflict and its resolution are §10.
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

- `scripts/tests/test_upstream_parity.py`'s `scratch_tree()` (the helper behind every `parity-root-*`
  and `import-root-*` tree and the comparison base) resolves the base to an absolute path, refuses one
  inside the repository, and falls back to the platform's own scratch root; a killed run can no longer
  leave anything in the tree, and `ScratchTreeCase` pins the rule for an inside-the-repository `TMPDIR`
  and for a relative one.
- `scripts/portability.py`'s `scratch_directory()` applies the same rule to the shipped checker (its
  scratch holds the alternate `go.mod`), with its own cases in `scripts/tests/test_portability.py` —
  and that suite's own trees now go through the shipped helper, with cases pinning the
  inside-the-repository and relative `TMPDIR` answers for the usage.
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

The check above has its case now (`ScratchTreeCase.test_a_scratch_tree_left_in_the_root_is_ignored`):
with git it makes the four directories and requires `git check-ignore` to name the matching pattern;
on a host or a tree without git it asserts the file's own four lines instead.

`upstream-parity.py` itself never created a scratch root: it is offline and hashes the tree in place,
which is why the leak's carrier was the *suite's* trees and the fix belongs to their helper. The
importer and the comparison script take the checkout they are handed (`--upstream`) or a scratch
directory outside the tree.

## 9. The `windows-latest` leg, and the repair it forced

`gate (ubuntu-latest)` passed this increment's head (`4bab390`); `gate (windows-latest)` ran the same
gate file, failed four steps and ended `GATE: FAIL — gofmt, test, selftest, citations` (`portability`
was disclosed as `SKIP` by design: that runner's checkout refuses one of the pin's long fixture paths,
which is the boundary the step states rather than a failure). This section is the repair. §9.1 is what
the leg printed; §9.2–§9.4 are the four classes it turned out to be, each with the evidence that
pinned it and the case that now holds it; §9.5 is the gate on the repaired state. The increment's plan
is **not** edited — no step of it changed — and §1–§8 above stand as written.

### 9.1 What the leg answered

The leg's step lines, exactly as the runner logged them (long cells elided where marked (`…`) and
re-wrapped for the page; the gate's own `… and 23 more` is its list bound, not an elision of this
record):

```
  FAIL     gofmt       not gofmt-clean:
ccc/cmd/ccc/claude_route_test.go
… (this record's elision: the gate printed 40 paths, every one under ccc/) …
… and 23 more
  PASS     build       go build ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     vet         go vet ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  FAIL     test        go test -count=1 ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=1
    --- FAIL: TestDumpMatchesTheRecordedUpstreamAgreement/codex (0.41s)
        dump_test.go:60: the dump for codex differs from the recorded upstream dump; …
            --- recorded: 9901 bytes
            --- now:      9658 bytes
    --- FAIL: TestDumpMatchesTheRecordedUpstreamAgreement/pi (0.30s)
            --- recorded: 7779 bytes
            --- now:      7582 bytes
  SKIP     portability portability: INCOMPLETE — no pinned checkout is available: git checkout --detach
           83195c8a… failed (rc=128): error: unable to create file
           internal/parser/testdata/data-version-corpus/…: Filename too long — not checked: fetch …
           by hand, or pass --upstream DIR with a clean checkout of it
  FAIL     selftest    REQUIRED but was FAIL: scripts/tests -> rc=1
Ran 477 tests in 24.031s
FAILED (failures=28, errors=9)
  PASS     parity      … | carried: 5579 file(s) identical to the pinned revision (compared through
           git's own object store) | … | symlinks hashed by their link target: 4 | carried link(s)
           matched in this platform's own spelling: 3 (its symlink layer stores a link's target with
           backslashes, …) | owner-executable bits not verified for 5576 file(s) …
  FAIL     citations   REQUIRED but was FAIL: … docs/evidence/0008-citations.tsv: 12 row(s) verified
  docs/evidence/0008-citations.tsv:51: rule S6: test TestFixturesMatchTheirGenerator in ./internal/ccc
  did not pass — go test reported `--- FAIL: TestFixturesMatchTheirGenerator`
  PASS     secrets     no leaks found in 187 file(s) this repository owns …
GATE: FAIL — gofmt, test, selftest, citations
```

The log is also the whole of what could be *read out of it*: GitHub's runner collapses the python
child's captions into one annotation, and the `selftest` step's detail was a 1 200-character **tail**
— so the 28 failures and 9 errors it counted could not be enumerated from the log at all. The one
annotation it printed is the last of them, and it is a §9.3.1 case (the runner's own line, its inner
line feeds shown as the log's own `\n` and wrapped here):

```
##[error]AssertionError: 'reported by cmd/ccc/pin.go: 0123456789abcdef0123456789abcdef01234567' not found in
"pin 0123456789abcdef0123456789abcdef01234567 (v0-test)\nreported by cmd\ccc\pin.go: 0123456789abcdef0123456789abcdef01234567\n…"
```

Two consequences are part of this repair. The step now names every failing test in its own detail
(§9.4), so the next Windows round — if there is one — enumerates itself. And the classes below were
identified by **simulating the host axes this machine can simulate** — CRLF text writes, an absent
`/tmp` and `/var/tmp`, the `nt` platform seams, a host-spelled relative path — rather than by reading
the log, because the log does not hold them; every simulation below states which axis it fakes, and
§9.6 states what that does not prove.

### 9.2 Class 1 — the checkout's line endings (`gofmt`, `test`, `citations`)

The migration moved upstream's root `.gitattributes` into the carry and this repository did not get one
of its own, so this repository's own files were left with no attributes at all:

```
# before the fix, at 4bab390
$ git check-attr text eol -- ccc/internal/ccc/continue.go
ccc/internal/ccc/continue.go: text: unspecified
ccc/internal/ccc/continue.go: eol: unspecified
$ git check-attr text eol -- agentsview/internal/parser/claude.go
agentsview/internal/parser/claude.go: text: auto
agentsview/internal/parser/claude.go: eol: lf
```

The Windows runner checks out with `core.autocrlf=true`, so the carried tree — which keeps upstream's
own `.gitattributes` at `agentsview/.gitattributes` — was written with LF while every file this
repository owns was written with CRLF. That is the whole of the three Go-visible failures: `gofmt`
reads a CRLF `*.go` as unformatted, and the two recorded dumps and the fixture-generator pin compare
the working tree's bytes against recorded artifacts.

**Fix.** A root `.gitattributes` carrying the base file's own first rule, scoped to this repository's
half (`* text=auto eol=lf`); the carried tree keeps its own copy, and attributes resolve from the
deepest file, so its `binary`/`linguist-generated` rules still win for the paths they name.
`git check-attr` after the fix: `ccc/internal/ccc/continue.go` and `scripts/gate.py` read
`text: auto / eol: lf`; `agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz` still reads
`text: unset` (the carried `*.gz binary` rule) and the frontend generated paths still take the
carried `linguist-generated` rule. No blob needed renormalising: `git ls-files --eol` answers 5 657
`i/lf`, 71 `i/-text` (a carried `binary` rule), 34 `i/none` (content git reads as binary) and 4 empty
entries (the carried 120000 symlinks) — and **no `i/crlf` at all**, which is the state a byte-preserving
move leaves.

**Evidence, both arms measured on this host** (the checkout recipe, not a Windows box): clone the head
twice, `core.autocrlf=true`, the second clone carrying a one-commit probe with the root
`.gitattributes`:

```
# negative arm — a fresh clone of 4bab390 with core.autocrlf=true
$ grep -c $'\r' ccc/internal/ccc/continue.go          # ours: CRLF
1494
$ grep -c $'\r' scripts/gate.py
524
$ grep -c $'\r' agentsview/internal/parser/claude.go  # carried: LF (its own attributes)
0
$ (cd ccc && gofmt -l ./cmd/ccc ./internal/ccc | wc -l)
63                                                    # the gate's own list: 40 + "and 23 more"
$ (cd ccc && go test -count=1 -tags fts5 -run 'TestDumpMatchesTheRecordedUpstreamAgreement' ./internal/ccc/parity/...)
    --- FAIL: TestDumpMatchesTheRecordedUpstreamAgreement/pi (0.12s)
            --- recorded: 7779 bytes
            --- now:      7582 bytes      # the leg's own numbers
$ (cd ccc && go test -count=1 -tags fts5 -run 'TestFixturesMatchTheirGenerator' ./internal/ccc)
    fixtures_test.go:208: growth fragments: … differs from what fixturegen writes; …
FAIL

# positive arm — the same clone with the root .gitattributes committed
$ grep -c $'\r' ccc/internal/ccc/continue.go; grep -c $'\r' scripts/gate.py
0
0
$ (cd ccc && gofmt -l ./cmd/ccc ./internal/ccc | wc -l)
0
$ (cd ccc && go test … 'TestDumpMatchesTheRecordedUpstreamAgreement' ./internal/ccc/parity/...)
ok  	go.kenn.io/agentsview/ccc/internal/ccc/parity	0.746s
$ (cd ccc && go test … 'TestFixturesMatchTheirGenerator' ./internal/ccc)
ok  	go.kenn.io/agentsview/ccc/internal/ccc	1.131s
```

A third measurement, the fresh-checkout question itself (a temporary index + `checkout-index` under
`core.autocrlf=true`, the real working tree's attributes in force): every file this repository owns
comes out LF (`grep -c $'\r'` = 0 for `ccc/internal/ccc/continue.go` and `scripts/gate.py`), the
carried file stays LF, and the binary input is byte-identical (`cmp` clean).

### 9.3 Class 2 — the host's own answers (the `28` failures and `9` errors)

Each class below was reproduced on this host by simulating one axis of a Windows host, and each was
then fixed. The classes' arithmetic is the leg's own counts — 28 failures = 17 + 3 (the parity line
below) + 2 + 4 + 2, 9 errors = 7 + 2 — and §9.6 states what a simulation of one axis does not prove.

**9.3.1 The parity line's separator (17 failures + 3 subtest records).** The migration spelled the
pin constant's path through `Path(...)` and printed it with `str()`, so the leg's Windows run wrote
`reported by cmd\ccc\pin.go: …` where every other surface of this tree spells tree paths POSIX-form —
and the `parity` suite asserts that line (16 `assert_clean` cases). The same spelling reached the
failure messages, which one more case asserts (`test_the_cli_must_report_the_pinned_revision`). The
three link-shape cases run two shapes each where the host can create symlinks — and it can there: the
leg's own `parity` line discloses `carried link(s) matched in this platform's own spelling: 3`, which
is Git for Windows having *created* the pin's links, so `os.symlink` works on that runner and the
second shape ran too, each of its two subtests failing on the same line (3 more failure records).

*Fix:* `scripts/upstream-parity.py`'s `pin_source_path()` — the layout's module through a `Path` and
back, `as_posix()`'d, the same rule `scripts/gate.py`'s `module_spelling()` states. *Case:*
`ParityCase.test_the_pin_source_spelling_is_posix_on_every_platform` drives the helper with the path
*values* a Windows host hands it (`PureWindowsPath` for both operands, as they are there) for both
layouts, so the difference is visible on this host rather than only in the Windows lane; reverting the
helper to `str()` fails it (measured).

**9.3.2 The scratch fallback list (2 failures + 2 errors).** The `parity-root-*` leak's fix gave both
scratch helpers a fallback list of `/tmp` and `/var/tmp`. On Windows those are `\tmp` and `\var\tmp` on
whichever drive the process is on — not directories — so a case that poisons the environment's temp
answer (the two `ScratchTreeCase` cases, deliberately) or a relative one
(`ScratchDirectoryCase`) had *no* candidate left: `AssertionError: no scratch directory outside … is
available` and `PortabilityIncomplete: every scratch directory this host offers is inside the
repository` — 2 failures and 2 errors, and the helper's whole purpose unmet on that platform.

*Fix:* `platform_scratch_bases()` in `scripts/portability.py` (POSIX: `/tmp`, `/var/tmp`; Windows:
`TEMP`/`TMP`, the variables that platform's own temp answer is made of) and the same rule in
`scripts/tests/test_upstream_parity.py`'s helper (taking the platform as a parameter, because `os.name`
cannot be patched while `pathlib` reads it at `Path()` call time). A base either list answers that is
not *absolute* is refused before anything is created: a relative `TEMP` names no root of its own, and
the scratch would land under whatever the process's current directory happens to be — the 81-directory
leak's own shape, one layer down.
*Cases:* `test_the_platforms_own_scratch_bases_are_pinned_for_both_answers` in both suites pins both
answers; the four poisoning cases were re-measured with the Windows answers in the seams (`TEMP`/`TMP`
naming the real temporary root, `/tmp` and `/var/tmp` made absent, the `nt` answers forced through the
shipped seam and the helper's parameter) and all four pass, each having failed the same way before the
fix. The same repair noticed, and removes, one more piece of litter this increment's scratch work
introduced: `CompareCheckoutCase.checkout()` gives its throw-away checkout a base of its own, and
`TemporaryDirectory` removes only the directory *inside* that base — so the base itself was left in
the temp root, one per case. It is registered for removal with the tree it carries now.

**9.3.3 The portability fixture's text-written build input (7 errors + 4 failures).** The suite's
`PortabilityTree` wrote the pinned `litellm_snapshot.json.gz` with `write_text`. Text mode translates
`\n` to the platform's separator on Windows, so those bytes stopped hashing to the digest the manifest
pins and `portability.restore_inputs()` refuses them — for the *checkout's* copy with the input error
(7 acceptance cases error out: `PortabilityError: … is not the input this tree pins …`) and for the
tree's own copy after the refusal paths had been reached (4 refusal cases assert the wrong error:
`'go build' not found in '… is not the input this tree pins …'`).

*Fix:* the fixture writes that file as **bytes** (a `write_bytes`/`write_to` pair in the tree helper;
the shipped checker is right — the digest is over bytes). *Evidence:* emulating the write side alone
(`Path.write_text` translating `\n` to `\r\n`, reads untouched, as on Windows) reproduced exactly that
set before the fix — `ran 479 failures 4 errors 7`, every one in `test_portability` — and the same
emulation over the repaired suite reports `Ran 488 tests … OK`; the acceptance cases are their own
witness (they fail when the input's digest is not the pinned one).

**9.3.4 The nested-run cases (2 failures), and the harness defect the repair exposed.** Both
`HostCase` nested-run cases fail whenever the child suite fails (§9.3.1–9.3.3 are the child's own
failures), and the repair of the suite is the repair of those two. The repair exposed two defects in
the harness itself — the first when the count assertion met the second same-named case this repair
adds (the two modules' platform-fallback cases), the second when the record-line case below was
written: `nested_statuses()` keyed the child's cases by
their **bare name**, so the second case to join a name read as a
count that does not add up — `AssertionError: 485 != 488` — instead of as two cases (keys are the
runner's own `<module>.<class>.<case>` qualifier now, read back by name through `status_of()`, which
refuses a lookup that matches no case or more than one); and `nested_summary()`'s "broken:" list
scanned for the `FAIL:`/`ERROR:` prefix alone, so a case's *printed records* — the shipped checks
record `FAIL: <name> — <detail>` lines, and the opencode cases print those — were named as broken
cases in the one message whose job is naming the case that broke (only the runner's own case-line
shape is read now). Both are held the same way: reverting the qualifier keys fails
`test_suite_holds_in_a_child_whose_path_holds_no_tools` (`485 != 488`), and reverting the filter fails
`test_a_record_line_in_the_child_s_output_is_not_a_broken_case`.

### 9.4 The step now enumerates its failures

`scripts/gate.py`'s `step_selftest()` prints every `FAIL:`/`ERROR:` caption the child printed, ahead of
the detail's tail (`failing_tests()`): a tail holds only the last failure's traceback, and a CI log is
what a reader has left when the run is over. Output only — no step is narrowed, skipped or silenced,
and no gate rule changes. *Case:* `test_the_selftest_failure_names_every_failing_test` writes two
failing tests (one failure, one error) and asserts both captions and the summary behind them;
suppressing the listing fails it (measured). `docs/harness.md`'s `selftest` section states the rule.

### 9.5 The gate on the repaired state

```
$ python3 scripts/gate.py
gate: /home/azureuser/claw/tasks/2026-11-14/call-ccc/wt-s1/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go module=ccc targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt       3624 file(s) gofmt-clean
  PASS     build       go build ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     vet         go vet ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     test        3 package(s) ok (in ccc)
  PASS     portability pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) | module ccc built and tested against the pristine checkout at /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584 | ran with -modfile=portability.mod (replace -> /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584) and portability.sum beside it | go build ./cmd/ccc/... ./internal/ccc/... -> rc=0 | 3 package(s) ok
  PASS     selftest    Ran 488 tests in 12.791s (rc=0)
  PASS     parity      … | : 83195c8a6861b8c74a4b6cb87512817fed6f8584 | layout: carry root agentsview/, this repository's module ccc | carried: 5579 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 1 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 0 project-owned prefix(es), plus the carry directory's own boundary; files seen by git: 5769 | inputs: agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by agentsview/internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared stored bytes verified: agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations   docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified
  PASS     secrets     no leaks found in 190 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
rc=0
```

The run was taken on the state this record leaves except for this record's own text (the record is a
tracked file already counted by the `parity` and `secrets` steps, so their numbers do not move), and
the suite's own count is 488 — the migration's 477, the five cases this repair adds (the spelling
case, the two fallback-list cases, the gate's failing-test listing case, and the record-line case), and
the six the simplify apply pass adds (the two portability exit-code cases, the root `.gitattributes`
case, the `.gitignore` scratch-pattern case, and the two `test_portability.py` scratch-tree cases).

### 9.6 What this repair does not claim

- **No Windows runner executed this head.** Every Windows answer above is a *simulation of one axis* on
  this Linux host — CRLF text writes, an absent `/tmp`, the `nt` platform seams, a host-spelled
  relative path — never Windows itself. The simulations reproduce the leg's failure *classes* (and the
  byte counts the Go tests printed), but the operator's re-run of `windows-latest` is the verification.
- **The 28 + 9 enumeration is derived, not read.** The log holds the summary counts and exactly one
  caption (the assertion quoted in §9.1); the classes above reproduce on this host and their sum is the
  leg's own count, but no log line confirms the distribution — and the fixed `selftest` step is what
  makes the next round readable rather than inferred.
- **The three-subtest arithmetic of §9.3.1 is an inference from the leg's own disclosure** that Git for
  Windows created the pin's links (`carried link(s) matched in this platform's own spelling: 3`), i.e.
  that the runner can create symlinks and the suite's second link shape ran there.

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
- **The `AGENTS.md` update is in this tree now** (`fe68e35`, owner-authorised 2026-11-22 after two
  unattended prompts timed out): the intended patch's nine hunks — the path statements the migration
  invalidates, the gate's new `portability` step, the carry's location — plus the owner's comment rule
  as one `## How to work here` bullet; §10 records how the bytes were verified. What this bullet used to
  say (held for approval, byte-identical to `main`'s) no longer holds.
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
- **The repair of §9 is not verified on Windows.** No runner executed the repaired head, and every
  Windows answer §9 rests on is a simulation of one host axis on this Linux machine; what is verified
  *here* is the gate on the repaired tree (§9.5), the suite (488 tests), and each emulation's
  reproduction of the failing set it stands for. The next `windows-latest` run is the verification —
  and the `selftest` step now names its failures, so that run enumerates itself.

## 10. After the base moved: the rebase, the contract's own update, and the heads this record covers

PR #20 (`prep/top-rule-and-deferrals`, S0) merged to `main` as **`8989d2c`** while this increment waited, so
the branch was **rebased** onto it — `0671bda`, `4e45995`, then the `AGENTS.md` commit below — rather than
redone. The increment's own content is unchanged by the rebase except where the two increments edited the
same paragraph.

- **One conflict, one resolution — `docs/core-design/README.md`.** PR #20 added §2 (the ownership rule and the
  declared-debt list) at the paragraph this increment rewrites (the Go `internal/` import restriction), and
  #20's new text names our half's packages by their pre-migration paths. The resolution keeps **every
  sentence** of §2 whole and translates only the spellings the migration moved (`ccc/internal/ccc/…`,
  `agentsview/internal/parser/…`); the import-restriction paragraph is this increment's. Everything else
  #20 changed is taken as it stands — its §2 debt list, the 0015–0017 deferral, the `docs/goals/` rows, and
  its own evidence record.
- **`AGENTS.md` is in this tree as `fe68e35`.** The intended patch's nine hunks plus the owner's comment rule
  of the same day, as one new `## How to work here` bullet. The write went through Hermes's protected-file
  gate as the owner's own click, after two prompts had timed out unattended; the bytes were verified *before*
  the click and after it — the same patch text was applied to a byte-identical scratch copy first and the
  result compared against the deterministic reference (`AGENTS.pre.md` + `patch -p1 <` the intended patch +
  the bullet): **identical, 40 292 bytes, `adaabe9f7a1fe1cc…`**, and the committed file hashes the same. One
  judgement is worth naming: the comment rule is an *owner instruction* riding a layout increment because it
  governs every later increment's code and this is the increment in flight; it is stated as its own bullet,
  not woven into the migration's statements.
- **The gate on the rebased tree** prints the unqualified `GATE: PASS` — nine steps, `portability` included,
  against the pristine pin checkout at `~/.cache/ccc-upstream/83195c8a…` (log
  `artifacts/s1/gate-rebased-20261122T1245.log` in the task tree): parity reads **5 579** carried files
  identical to the pin, deviations 1 + 1 generated input, `files seen by git` 5 769; the suite is 482 cases
  at that head, and the simplify apply pass's six cases take it to 488 (§9.5).
- **What the rebase did not do.** It rewrote no history anyone else holds — the branch is this increment's own
  and is re-pushed with `--force-with-lease`; it touched no carried byte (`parity` is the proof); it changed
  no plan statement beyond the scope line that now names `AGENTS.md` and the comment rule; and it left the
  earlier increments' dated records exactly as §"What this record does not claim" says.
