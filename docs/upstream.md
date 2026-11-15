# The carried AgentsView tree

call/ccc does not reimplement source discovery, native session parsing, normalization or the SQLite
archive: it carries the **actual AgentsView source tree** at a pinned revision and adds `cmd/ccc` and
`internal/ccc` beside it. [The Core Design](core-design/README.md) makes that the source-side
authority; this file records the pin and every difference from it.

| | |
| --- | --- |
| Remote | `https://github.com/kenn-io/agentsview` |
| Pinned revision | `83195c8a6861b8c74a4b6cb87512817fed6f8584` |
| `git describe` | `v0.47.0-181-g83195c8a` |
| Commit date | 2026-11-14T16:19:32+00:00 |
| Upstream files at the pin | 5580 |
| Carried byte for byte | 5553 |
| Declared deviations | 28 (27 upstream paths not carried or normalised, 1 generated build input) |
| Machine-readable record | [`scripts/upstream-manifest.txt`](../scripts/upstream-manifest.txt) |
| Checked by | `python3 scripts/upstream-parity.py` (the gate's required `parity` step) |

## What "unmodified" means here, and how it is checked

`scripts/upstream-parity.py` compares every carried path with the blob recorded for it in the
manifest, through git's own index where git is available (`git ls-files -s`: what a fresh clone
receives, `.gitattributes` filters applied) and by hashing bytes where it is not — so the answer is
the same on every platform, and a file whose line endings git would normalise cannot pass in one
checkout and fail in another. It then fails if a manifest path is missing or modified in the working
tree, if a declared deviation has quietly turned back into upstream's copy — staged *or* only written
on disk, since the gate reads the working tree — or stops holding the stored bytes its record pins
(that path is a carried path for everything but the byte comparison: it must also be present in the
working tree, unmodified relative to git's index, and matched by no ignore rule), if the generated
input stops hashing to the value the carried snapshot tool pins, if `docs/upstream.md` or
`cmd/ccc/pin.go` disagree with the manifest about the revision, or if anything git would add sits
inside the tree without a manifest record or a project-owned prefix. A `git status` or ignore probe
that cannot answer fails the check: "nothing is modified" is the one answer that would look like a
pass. A tree git cannot enumerate at all — no `git` on the host, or a root that is a copy rather than
a repository — is reported as `INCOMPLETE` and exits non-zero, naming the file-list and ignore rules
that did not run, so the required gate step cannot print a full pass for a tree nobody verified. The
check is offline; it needs no checkout of upstream to answer.

Behaviour is a separate question from bytes, and it is answered by evidence rather than by this file:
[`docs/evidence/0001-agentsview-parity-and-inspection.md`](evidence/0001-agentsview-parity-and-inspection.md)
records a parse → sync → archive-read comparison between this tree and a pristine checkout at the
pin, and `scripts/upstream-compare.py` re-derives it.

## Declared deviations

Nothing else differs. The four paths below are this repository's own files, so upstream's copies of
them are not carried; `.github/**` is not carried because Actions reads `.github/workflows/` at the
repository root, and carrying upstream's workflows would run upstream's CI — its runners, secrets and
desktop builds — on every call/ccc pull request.

| Path | Why |
| --- | --- |
| `.gitignore` | call/ccc keeps its own ignore rules at this path |
| `AGENTS.md` | call/ccc's control file lives at this path |
| `README.md` | call/ccc's project README lives at this path |
| `docs/README.md` | call/ccc's docs index lives at this path |
| `.github/**` (22 files) | upstream CI definitions would run upstream's CI here |
| `frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json` | carried with exactly one byte normalised: upstream's blob holds a single CRLF pair in 1.4 MB that this repository's carried `.gitattributes` turns into LF, so git stores the file one byte shorter. Its record pins both: upstream's blob (`ebcc03dc…`, 1 441 138 B) and the bytes this tree stores (`48adf87e…`, 1 441 137 B), which the parity step verifies through git's index — or against the file's own bytes in a tree without git |

The last row is the only path among upstream's 5580 tracked files whose bytes cannot be stored
unchanged: git normalises text line endings on `git add` (`* text=auto eol=lf`, upstream's own
attribute file). Carrying it verbatim would need an attribute file inside a carried directory —
project content in upstream-owned space — so the file is kept with the byte git stores and the
difference is declared instead of hidden. Because upstream's blob can never be what the tree holds for
that path, "still differs from upstream" proves nothing about it: the stored digest is recorded and
checked, and the importer writes those stored bytes so the working tree equals what a clone checks
out.

One build input is carried although upstream does not track it: `internal/pricing/fallback.go`
embeds `internal/pricing/snapshot/litellm_snapshot.json.gz`, which upstream's `Makefile` restores
from a pinned artifact branch before every Go-compiling target (`make pricing-snapshot`). Without it
nothing in the parser → archive path compiles, so the file is committed here and verified against the
SHA-256 that the carried `internal/pricing/cmd/litellm-snapshot` pins in its own source. Two
consequences worth knowing:

- the carried `internal/pricing/snapshot/.gitignore` ignores that file, so a fresh `git add -A` skips
  it; add it with `git add -f internal/pricing/snapshot/litellm_snapshot.json.gz`;
- a deleted copy is restored with
  `go run ./internal/pricing/cmd/litellm-snapshot -restore` (needs network), which is the same
  command upstream's Makefile runs.

Two carried paths behave slightly differently than they do upstream, without differing in content:
`CLAUDE.md` is a symlink to `AGENTS.md`, so in this repository it resolves to call/ccc's control file;
and upstream's `docs/agents/*` guides are carried as upstream's rules for changing carried code,
while this repository's own rules live in [`AGENTS.md`](../AGENTS.md) and
[`docs/conventions.md`](conventions.md).

## Updating the pin

```bash
git clone https://github.com/kenn-io/agentsview /tmp/agentsview
git -C /tmp/agentsview checkout <new-rev>
python3 scripts/upstream-import.py --upstream /tmp/agentsview --restore-snapshot --prune
python3 scripts/upstream-parity.py
```

The importer reads upstream's **git objects**, not its working tree, so a checkout with unusual
`core.autocrlf` cannot smuggle a rewritten file in; it refuses a dirty checkout, refuses to write a
path this repository owns, and with `--prune` deletes only files the previous manifest carried and
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
- Carrying the tree is not a vendoring relationship. The carried files keep upstream's module path
  (`go.kenn.io/agentsview`) and their upstream-owned guides apply to them; if a carried file needs a
  fix, the fix belongs upstream, contributed separately, not as a local edit that parity then has to
  declare.
