# Layout migration A — the carry in its own directory, two modules

**Status:** implemented in the increment tracked by [`docs/evidence/layout-migration-A.md`](../evidence/layout-migration-A.md).
**Owner decision:** the layout question was put to the owner on 2026-11-21 as one fork, and the answer
was *A — carry in its own folder, two modules* (the answered sweep is recorded in the task's decision
log; this file restates the decision in-repo so the repository carries its own provenance, and the
machine record of the result is `scripts/upstream-manifest.txt`'s `layout` line).

## 1. The shape, and why this one

```
/                       ours, at the root:
  AGENTS.md  README.md  .gitignore  .github/   docs/   scripts/
  ccc/                  go.mod (module go.kenn.io/agentsview/ccc) + internal/ccc/** + cmd/ccc/**
  agentsview/           the whole carried tree, go.mod byte-identical to the pin
```

One sentence decides every question this layout raises: **inside `agentsview/` is upstream's,
everything else is ours.** The alternative shapes (the carry at the root with our code beside it, which
is what the repository had; a workspace instead of a module edge) each leave a reader asking which file
belongs to whom; here the directory answers it, and the ledger's `owned` prefix list stops being the
boundary — a list that had to be edited every time this repository added a file.

The second half of the shape is our module's *path*: `go.kenn.io/agentsview/ccc` is a subpath of
upstream's module path, which is what keeps the carried `internal/**` packages importable under Go's
internal-package rule (decided on import paths, not on directories) without renaming a single upstream
import. It also makes the eventual back-port literally "copy the `ccc/` folder in": every import path
inside it (`go.kenn.io/agentsview/ccc/…`) is already what it would be inside upstream's module. That
was proven on the installed toolchain before any file moved (three probes: the directory layout with a
subpath module, the root layout with the same, and an unrelated module path refused).

## 2. What moved, and what it cost

- **5579 carried files** (the whole pinned tree, its own `go.mod`/`go.sum` included) moved into
  `agentsview/`. The carried set is **byte-identical** to the pin before and after: the ledger was
  re-derived by `scripts/upstream-import.py` at the new prefix, and the `parity` step answers 5579
  files identical, 1 declared normalisation and 1 generated build input — **deviations 28 → 2**. The 26
  that stopped being deviations are the four paths where our file and upstream's both wanted one home
  (`.gitignore`, `AGENTS.md`, `README.md`, `docs/README.md` — each side now has one) and upstream's 22
  `.github/**` files (GitHub reads only the repository root's `.github/`, so upstream's CI definitions
  are inert where they are carried).
- **Our own half** (`internal/ccc/**`, `cmd/ccc/**`) moved into `ccc/`, and nine files' self-imports
  were rewritten (`go.kenn.io/agentsview/internal/ccc` → `go.kenn.io/agentsview/ccc/internal/ccc`).
  The compiler catches every miss: `go build`/`vet`/`test` in the module answer rc=0 and three packages
  `ok`.
- **The tools learned the shape** rather than being told it: `upstream_manifest.detect_layout` reads the
  tree's own `go.mod` pair, so the importer writes into the right prefix, the checker maps every
  recorded path to its tree path, the gate runs its Go steps in this repository's module directory, the
  citations checker runs `go test` there, and the native checks build from `ccc/`. Nothing in the tree
  forces one of the two shapes: the root layout (one module, the carry at the root) is still the
  answer for a tree that has it, which is what made the tooling change land safely before the files
  moved.
- **New gate step, `portability`** (`scripts/portability.py`): this module is built and tested against a
  *pristine* checkout of the pin, with `ccc/go.mod`'s single edge re-aimed at it through a scratch
  `-modfile`. It is the property the two-module shape exists for, checked instead of asserted. It is
  not a required step (it needs a second checkout), and a host that cannot get one gets a disclosed
  `SKIP` — never a quiet pass.
- **The scratch-root leak is closed.** `TMPDIR` that is relative or inside the repository is refused and
  a platform scratch root taken instead (`scripts/portability.py:scratch_directory`), the parity suite's
  throw-away trees resolves its base to an absolute path, and `.gitignore` names `parity-root-*/` — the
  pattern an interrupted run once left 81 copies of in the repository root.

## 3. Non-goals (explicitly not this increment)

- **S0's Top Rule** ("each half has one owner: theirs reads, ours writes") and the 0015–0017 deferral
  land in their own increment, held for owner approval.
- **S2's 0009a re-scope**, and the 0010–0013 refinements, come after this one; this increment authors no
  citations manifest for 0009a and touches no goal file.
- **No goal's `Done when` changes**, no goal moves to `docs/goals/archive/`, and the conductor's
  `GOALS` is untouched: this increment has no numbered goal, so it writes no ledger row.
- **The carried tree is not edited.** Upstream's own docs inside the carry keep their relative links as
  upstream wrote them (they resolve inside `agentsview/`, exactly as they would in a checkout of the
  pin). Two upstream links that used to dangle at the root (`agentsview/docs/agents/build.md`'s
  `../../README.md`, `agentsview/docs/screenshots/README.md`'s `../README.md`) now resolve, because
  upstream's `README.md` and `docs/README.md` are carried files instead of declared deviations.
- **Historical records are not rewritten.** `docs/evidence/*.md`, `docs/decisions/*` and the plans of
  earlier increments keep the paths of the day they were written; `docs/upstream.md` gains the old → new
  mapping that lets a reader translate one word at a time.

## 4. Where the plan was silent, and the choice taken

- `gofmt` stays **one step over every tracked `*.go`** (both modules) instead of being split per module:
  formatting has no module boundary, and splitting it would narrow a check that costs nothing. The
  per-module rule the impact list asks for applies to `build`/`vet`/`test`, which do depend on it.
- The ledger keeps its **records as upstream paths** plus a `layout` line, rather than prefixing every
  record: provenance stays legible (a pin update diffs upstream against upstream), the layout line states
  the shape once, and the checker is the one place that turns one into the other. The line is
  cross-checked against the tree's own `go.mod` files, so a half-migrated tree cannot be checked at the
  wrong prefix.
- The `citations` manifests' `package` column stays as it is (`./internal/ccc`): it is the package
  `go test` is handed *inside the module*, and the checker now runs `go test` there, so 86 historical
  rows did not have to be rewritten to mean the same thing.
- `docs/decisions/OPEN-QUESTIONS.md`'s dated entries keep their paths (historical record). The live
  contract docs (`docs/conventions.md`, `docs/harness.md`, `docs/core-design/README.md`) state the path
  convention once and are corrected where a statement about the tree itself was wrong.
- `.git-blame-ignore-revs` names the rename commit's *rule* and its subject, not a SHA: the squash
  commit does not exist until the operator makes it, and a file naming a commit nobody made would be a
  claim that cannot be checked. The operator appends the SHA when the merge lands (the file says how).

## 5. How it lands

One branch (`prep/layout-migration-A`, cut from `origin/main` = `21d43fe1a`) → one PR → squash merge onto
a linear `main`; the worktree leaves the edits uncommitted for the operator. The gate is green on the
final state (`python3 scripts/gate.py` → `GATE: PASS`), CI runs the same file on `ubuntu-latest` and
`windows-latest` (the workflow's Go toolchain now comes from `ccc/go.mod`, which declares the same `go`
directive as the carry), and the increment's evidence record
([`docs/evidence/layout-migration-A.md`](../evidence/layout-migration-A.md)) holds the commands, their
exit codes and the parity numbers, plus what the record does **not** claim.
