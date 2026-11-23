# The carried AgentsView tree

call/ccc does not reimplement source discovery, native session parsing, normalization or the SQLite
archive: it carries the **actual AgentsView source tree** at a pinned revision, inside `agentsview/`,
and adds this repository's own module (`ccc/`: the `ccc` CLI and its continuation core) beside it.
[The Core Design](core-design/README.md) makes that the source-side authority; this file records the
pin, the shape the two halves live in, and every difference from the pin.

The shape is one sentence: **inside `agentsview/` is upstream's, everything else is ours.** The carried
tree keeps its own `go.mod` (module `go.kenn.io/agentsview`), byte-identical to the pin; this
repository's module declares `go.kenn.io/agentsview/ccc` and points its single edge at the carry
(`replace go.kenn.io/agentsview => ../agentsview`), with no `go.work`, so a build of either half is
explicit about which one it is.

| | |
| --- | --- |
| Remote | `https://github.com/kenn-io/agentsview` |
| Pinned revision | `83195c8a6861b8c74a4b6cb87512817fed6f8584` |
| `git describe` | `v0.47.0-181-g83195c8a` |
| Commit date | 2026-11-14T16:19:32+00:00 |
| Upstream files at the pin | 5580 |
| Carried byte for byte | 5579 |
| Declared deviations | 2 (1 normalisation, 1 generated build input) |
| Layout | carry root `agentsview/`, this repository's module `ccc/` (the manifest's `layout` line) |
| Machine-readable record | [`scripts/upstream-manifest.txt`](../scripts/upstream-manifest.txt) |
| Checked by | `python3 scripts/upstream-parity.py` (the gate's required `parity` step) |
| Edge checked by | `python3 scripts/portability.py` (the gate's `portability` step: this module against a pristine checkout of the pin) |

## Where the files are (and where they were)

The carried tree moved into a directory of its own on 2026-11-21 (owner decision 2026-11-21): every path
below `agentsview/` is an upstream path, and everything else in this repository is ours. Paths are
recorded as they were read -- the evidence records under `docs/evidence/`, the decision log and the
plans keep the spellings of the day they were written, and are not rewritten -- so a reader following
an older record translates one word at a time:

| Then | Now |
| --- | --- |
| `internal/<pkg>` (a carried package) | `agentsview/internal/<pkg>` |
| `cmd/agentsview`, `frontend/`, `tools/`, `desktop/`, `testdata/`, `support/`, `plugins/` | the same names under `agentsview/` |
| `Makefile`, `Dockerfile`, `LICENSE`, `openapi.yaml`, `docs/<upstream page>` | the same names under `agentsview/` |
| `go.mod`, `go.sum` | `agentsview/go.mod`, `agentsview/go.sum` (the pin's own files) |
| `internal/ccc` (ours), `cmd/ccc` (ours) | `ccc/internal/ccc`, `ccc/cmd/ccc` |
| `docs/upstream.md`, `docs/harness.md`, `docs/conventions.md`, `docs/{plans,goals,evidence,decisions,core-design}/**` | unchanged (ours, still at the root) |
| `scripts/**` | this repository's own scripts unchanged; upstream's own `scripts/**` moved to `agentsview/scripts/**` |
| `.github/workflows/gate.yml` | unchanged (GitHub reads the repository root's `.github/`; upstream's own `.github/**` is carried under `agentsview/.github/`, where Actions cannot see it) |

The root `LICENSE` is **ours**, not a carried file: it states the same MIT terms as
`agentsview/LICENSE`, with the upstream copyright notice preserved, so a reader at the root — GitHub's
licence view included — sees a licence for this repository's own half (`ccc/**`, `scripts/**`). It was
added by this increment under the owner's decision of 2026-11-23.

The `parity` step prints tree paths (a record's upstream path through the layout), so its failures can
be opened directly; the manifest itself keeps upstream paths, because that is what a pin update has to
be diffed against.

## What "unmodified" means here, and how it is checked

`scripts/upstream-parity.py` compares every carried path with the blob recorded for it in the
manifest, through git's own index where git is available (`git ls-files -s`: what a fresh clone
receives, `.gitattributes` filters applied) and by hashing bytes where it is not — so the answer is
the same on every platform, and a file whose line endings git would normalise cannot pass in one
checkout and fail in another. The working tree is *also* hashed in both branches, with git and without
it: an `assume-unchanged` or `skip-worktree` index flag keeps an edited path out of `git status` while
the pinned blob stays in the index, so a check that trusted git's own status for the working tree would
pass for modified carried source; the record's shape, bytes and owner-executable bit (the bit git's own
mode records, `S_IXUSR`) are compared directly with the file this repository ships, and git's index adds
what a stat cannot answer — what a fresh clone receives. A carried symlink's shape is part of that
comparison: the link and the regular file holding its target text are the same git object, so the file
form is accepted only where the checkout says it stores links that way (`core.symlinks=false`, or a
platform that cannot create symlinks), and the run reports how many it accepted. A link whose target
this platform's own symlink layer re-spells is compared in that spelling: Git for Windows cannot create
a link whose target holds `/` (its `mingw_create_symlink` rewrites each separator to `\`), so the link a
checkout there creates for the pin's `../../x` reads back as `..\..\x` — the same destination, hashing
to no blob the manifest holds, and what a correct checkout on that platform looks like. Only the
separators are folded, only for a link, and only on the platform whose symlink layer does it, so a POSIX
checkout and a target that changed by anything else are still drift; the run names how many links it
matched that way. It then fails if a
manifest path is missing or modified in the working tree, if a declared deviation has quietly turned
back into upstream's copy — staged *or* only
written on disk, since the gate reads the working tree — or stops holding the stored bytes its record
pins (that path is a carried path for everything but the byte comparison: it must also be present in
the working tree, unmodified relative to git's index, its working-tree owner-executable bit must be the
record's — checked on the file itself, because a checkout with `core.filemode=false` leaves that edit
out of git's index and status — and it must be matched by no ignore rule), if the generated input
stops hashing to the value the carried snapshot tool pins or is not what git's index stores for it
(presence, mode and the blob its own bytes hash to: the carried `snapshot/.gitignore` keeps that path
out of every untracked listing, so only the index says whether a clone receives it), if `docs/upstream.md`
or `ccc/cmd/ccc/pin.go` disagree with the manifest about the revision, if the manifest's own `layout`
line disagrees with the shape this tree's `go.mod` files have, or if anything git would add sits inside
the tree without a manifest record and outside the boundary that says what is ours. Every git query is required: an untracked-file, index or
`git status` answer the host could not give fails the check rather than dropping the rule it answers
for, because "git would add nothing" and "nothing is modified" are the answers that look like a pass. A
tree git cannot enumerate at all — no `git` on the host, or a root that is a copy rather than a
repository — is reported as `INCOMPLETE` and exits non-zero, naming the file-list and ignore rules that
did not run, so the required gate step cannot print a full pass for a tree nobody verified. The check is
offline; it needs no checkout of upstream to answer.

Behaviour is a separate question from bytes, and it is answered by evidence rather than by this file:
[`docs/evidence/0001-agentsview-parity-and-inspection.md`](evidence/0001-agentsview-parity-and-inspection.md)
records a parse → sync → archive-read comparison between this tree and a pristine checkout at the
pin, and `scripts/upstream-compare.py` re-derives it.

## Declared deviations

Two upstream paths do not travel as upstream's own bytes, and both keep their recorded reason:

| Path | Why |
| --- | --- |
| `agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json` | carried with exactly one byte normalised: upstream's blob holds a single CRLF pair in 1.4 MB that this repository's carried `.gitattributes` turns into LF, so git stores the file one byte shorter. Its record pins both: upstream's blob (`ebcc03dc…`, 1 441 138 B) and the bytes this tree stores (`48adf87e…`, 1 441 137 B), which the parity step verifies through git's index — or against the file's own bytes in a tree without git |
| `agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz` | the generated build input (below): upstream does not track it, so there is no upstream blob to be identical to |

The first row is the only path among upstream's 5580 tracked files whose bytes cannot be stored
unchanged: git normalises text line endings on `git add` (`* text=auto eol=lf`, upstream's own
attribute file). Carrying it verbatim would need an attribute file inside a carried directory —
project content in upstream-owned space — so the file is kept with the byte git stores and the
difference is declared instead of hidden. Because upstream's blob can never be what the tree holds for
that path, "still differs from upstream" proves nothing about it: the stored digest is recorded and
checked, and the importer writes those stored bytes so the working tree equals what a clone checks
out.

Nothing else differs, and **twenty-six paths that used to be declared deviations are not any more** — the
layout answered them. `.gitignore`, `AGENTS.md`, `README.md` and `docs/README.md` were paths where
this repository's file and upstream's both wanted one home; each side now has one (`agentsview/…` for
upstream's, the repository root for ours), so upstream's copies are carried verbatim. The 22
`.github/**` files were skipped because Actions reads `.github/workflows/` at the repository root and
carrying upstream's workflows would run upstream's CI — its runners, secrets and desktop builds — on
every call/ccc pull request; inside `agentsview/` they are inert, because GitHub reads the root's
`.github/` and nothing below a subdirectory, and they are carried verbatim there like the rest of the
tree. There is also no `owned` prefix list to keep in step any more: everything outside `agentsview/`
is this repository's own by construction, and that boundary is what the checker applies (the ledger's
`layout` line states the shape; the `owned` records a ledger of the earlier root layout spelled out
are still read and honoured).

One build input is carried although upstream does not track it:
`agentsview/internal/pricing/fallback.go` embeds
`agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz`, which upstream's `Makefile` restores
from a pinned artifact branch before every Go-compiling target (`make pricing-snapshot`). Without it
nothing in the parser → archive path compiles, so the file is committed here and verified against the
SHA-256 that the carried `internal/pricing/cmd/litellm-snapshot` pins in its own source. Three
consequences worth knowing:

- the carried `internal/pricing/snapshot/.gitignore` ignores that file, so a fresh `git add -A` skips
  it; add it with `git add -f agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz`;
- a deleted copy is restored with
  `go run ./internal/pricing/cmd/litellm-snapshot -restore` (needs network, run inside `agentsview/`),
  which is the same command upstream's Makefile runs;
- `python3 scripts/portability.py` writes this tree's copy into a checkout *it* fetched, because a
  pristine checkout cannot have the file either.

One carried path behaves slightly differently than it does upstream, without differing in content:
`CLAUDE.md` is a symlink to `AGENTS.md`, and inside the carry it resolves to upstream's own
`agentsview/AGENTS.md` (carried verbatim) — not to this repository's control file at the root.
Upstream's `docs/agents/*` guides are carried as upstream's rules for changing carried code, while
this repository's own rules live in [`AGENTS.md`](../AGENTS.md) and
[`docs/conventions.md`](conventions.md).

## Updating the pin

```bash
git clone https://github.com/kenn-io/agentsview /tmp/agentsview
git -C /tmp/agentsview checkout <new-rev>
python3 scripts/upstream-import.py --upstream /tmp/agentsview --restore-snapshot --prune
python3 scripts/upstream-parity.py
```

The importer reads upstream's **git objects**, not its working tree, so a checkout with unusual
`core.autocrlf` cannot smuggle a rewritten file in; it reads this tree's own shape from its `go.mod`
files and writes the carried files into the carry root the layout names (`agentsview/`), recording the
shape as the manifest's `layout` line; it refuses a dirty checkout, refuses to write a path this
repository owns, and with `--prune` deletes only files the previous manifest carried and
that this tree has not modified since — pruning *before* it writes, because a path can change shape
between revisions (a file becomes a directory). A path in the importer's `NORMALISED` list is carried
with the bytes git stores for it and recorded with both digests. A pin update is one commit that
changes the manifest and the carried files, and it should say in its message why the new revision was
taken (a parser fix for a format a user's sessions are in, a security fix, a released tag). If the
`parity` step then reports a carried file differing from the pin, either upstream really changed it or
the new pin normalises a path this repository has not declared in `NORMALISED`; the step names which
file, and the fix is one entry there (or an upstream contribution), not a hand-edited manifest.

## Limits

- Parity proves bytes, not behaviour: it cannot tell whether the pinned revision itself is correct.
- Parity does not prove that the carry is a *leaf* our module can be built against from outside this
  tree. `scripts/portability.py` (the gate's `portability` step) proves that, once per run, by
  re-aiming `ccc/go.mod`'s one edge at a pristine checkout of the pin and building and testing our
  module there.
- Carrying the tree is not a vendoring relationship. The carried files keep upstream's module path
  (`go.kenn.io/agentsview`) and their upstream-owned guides apply to them; if a carried file needs a
  fix, the fix belongs upstream, contributed separately, not as a local edit that parity then has to
  declare.
- The carried tree's own suites are upstream's to run: the local gate builds and tests *our* module
  against the carry (and, in the `portability` step, against a pristine copy of it), and upstream's
  PostgreSQL / ClickHouse / DuckDB lanes and its `internal/db` SQLite-header staging stay where
  `docs/harness.md` says they are.
