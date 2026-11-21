#!/usr/bin/env python3
"""Carry a pinned AgentsView revision into this tree and write the carry manifest.

The carried content comes from the upstream *git objects*, not from a working tree, so a checkout
with an unusual ``core.autocrlf`` cannot smuggle a rewritten file into the tree. Everything upstream
tracks is carried byte for byte, except the paths this tree's own layout declares as deviations: in
the root layout that is ``SKIP_REASONS`` (this repository owns those paths) and ``.github/**``
(Actions would run upstream's workflows here), and in the layout where the carry has a directory of
its own it is only ``NORMALISED`` -- the paths carried with the bytes git stores for them after
upstream's own attribute file rewrites their line endings, whose record pins both digests. Which
layout this tree has is read from its own ``go.mod`` files (:func:`upstream_manifest.detect_layout`),
never from a flag, so a tree that has been moved is imported at the shape it now has. Each generated
build input is recorded with its hash instead of being copied from upstream's checkout.

    python3 scripts/upstream-import.py --upstream /path/to/agentsview [--pin <rev>]
                                       [--restore-snapshot] [--prune] [--dry-run]

The tree is left for review; this script never commits, pushes or deletes anything it did not write.
``--prune`` removes only files a previous manifest recorded as carried, refuses if one of them has been
modified here, and runs *before* anything is written -- a path can change shape between revisions (a
file becomes a directory), and writing first would stop half-way through the tree on a path the prune
is about to remove. Every refusal names the path it stopped on instead of printing a traceback.
"""

from __future__ import annotations

import argparse
import hashlib
import os
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import (  # noqa: E402  (path set up above)
    Manifest,
    Record,
    blob_sha1,
    detect_layout,
    hashed_path,
    load_manifest,
    pinned_snapshot_sha256,
)

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "scripts" / "upstream-manifest.txt"
REMOTE = "https://github.com/kenn-io/agentsview"
PRICING_TOOL = "internal/pricing/cmd/litellm-snapshot/main.go"

#: Build inputs upstream does not track but every Go build embeds. They are copied from the upstream
#: checkout (``--restore-snapshot`` fetches the pinned artifact there first) and recorded with their
#: SHA-256, so the tree compiles offline and the input is still verifiable against upstream's pin.
GENERATED_INPUTS = ("internal/pricing/snapshot/litellm_snapshot.json.gz",)

#: Upstream paths this repository owns instead of carrying. Each entry is a reason the checker
#: prints verbatim, so the deviation is identified rather than folded into a count. These records
#: exist only in the root layout: once the carry has a directory of its own, upstream's `.gitignore`,
#: `AGENTS.md`, `README.md` and `docs/README.md` are carried inside it while this repository keeps its
#: own files at the root -- each side with one home, and the boundary (not a list) is what says which
#: is which.
SKIP_REASONS = {
    ".gitignore": "project-owned: call/ccc keeps its own ignore rules at this path",
    "AGENTS.md": "project-owned: call/ccc's control file lives at this path",
    "README.md": "project-owned: call/ccc's project README lives at this path",
    "docs/README.md": "project-owned: call/ccc's docs index lives at this path",
    # Upstream's own `.gitattributes` normalises text files to LF, and this 1.4 MB frontend fixture
    # holds one stray CRLF that `git add` therefore rewrites. The file is kept with the byte git
    # stores (identical otherwise); carrying upstream's blob verbatim would need an attribute file
    # inside a carried directory, which is project content in upstream-owned space.
    "frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json":
        "carried with one byte normalised: upstream's blob holds a single CRLF pair that this "
        "repository's carried .gitattributes turns into LF, so git stores the file one byte shorter",
}

#: Upstream CI definitions are not carried in the root layout: Actions reads `.github/workflows/` at
#: the repository root, so carrying them would run upstream's CI (its runners, secrets and desktop
#: builds) on every call/ccc pull request. Inside a carry directory of their own they are carried
#: verbatim and inert: GitHub reads the repository root's `.github/` and nothing below a subdirectory.
SKIP_PREFIXES = {
    ".github/": "upstream CI definitions: Actions would run upstream's workflows on call/ccc pull requests",
}

#: Upstream paths this repository carries with a declared byte normalisation instead of upstream's own
#: blob. Upstream's `.gitattributes` (`* text=auto eol=lf`) rewrites CRLF to LF when git stores a text
#: file, so these paths can never be carried byte for byte; the write below applies the same rewrite,
#: which keeps the working tree equal to what a clone checks out, and the manifest records both
#: digests so the checker can verify the stored one. The reason is in SKIP_REASONS.
NORMALISED = (
    "frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json",
)

#: Paths this repository owns in the root layout. A tracked file outside the manifest must match one
#: of these; a trailing slash means "everything below". The directory layout declares none: there the
#: carry's own directory is the boundary -- everything outside it is this repository's own -- so a
#: list would be a second spelling of a rule the layout already states.
OWNED = (
    ".github/",
    ".gitignore",
    "AGENTS.md",
    "README.md",
    "cmd/ccc/",
    "docs/README.md",
    "docs/conventions.md",
    "docs/core-design/",
    "docs/decisions/",
    "docs/evidence/",
    "docs/goals/",
    "docs/harness.md",
    "docs/plans/",
    "docs/upstream.md",
    "internal/ccc/",
    "scripts/claude-native-check.py",
    "scripts/codex-native-check.py",
    "scripts/gate-goflags.txt",
    "scripts/gate-required.txt",
    "scripts/gate-targets.txt",
    "scripts/gate.py",
    "scripts/native_check_common.py",
    "scripts/obligation-citations.py",
    "scripts/omp-native-check.py",
    "scripts/opencode-native-check.py",
    "scripts/pi-native-check.py",
    "scripts/portability.py",
    "scripts/tests/",
    "scripts/upstream-compare.py",
    "scripts/upstream-import.py",
    "scripts/upstream-manifest.txt",
    "scripts/upstream-parity.py",
    "scripts/upstream_manifest.py",
)


def skip_reasons(layout) -> dict[str, str]:
    """The declared deviations of one layout, keyed by upstream path.

    The directory layout declares one: the path upstream's own attribute file rewrites. Everything
    else in the root layout's list is a path conflict between two files that want one home, and the
    directory layout gives each side one.
    """
    if layout.carry:
        return {path: reason for path, reason in SKIP_REASONS.items() if path in NORMALISED}
    return dict(SKIP_REASONS)


def skip_prefixes(layout) -> dict[str, str]:
    """The declared deviation *prefixes* of one layout: upstream's CI definitions, in the root layout
    only (inside a carry directory they are inert, see :data:`SKIP_PREFIXES`)."""
    return {} if layout.carry else dict(SKIP_PREFIXES)


def owned_prefixes(layout) -> tuple[str, ...]:
    """The project-owned prefixes of one layout: the root layout's list, or none."""
    return () if layout.carry else OWNED


def git(upstream: Path, *args: str, text: bool = True) -> subprocess.CompletedProcess:
    """Ask git about a checkout; a missing git is an rc=127 answer, not a crash."""
    try:
        return subprocess.run(
            ["git", "-C", str(upstream), *args],
            capture_output=True,
            text=text,
            check=False,
        )
    except OSError as exc:
        empty = "" if text else b""
        return subprocess.CompletedProcess(["git", *args], 127, empty, str(exc))


def upstream_entries(upstream: Path, pin: str) -> list[tuple[str, str, str]]:
    """Return ``(mode, blob-sha1, path)`` for every tracked blob at ``pin``."""
    proc = git(upstream, "ls-tree", "-r", "-z", "--full-tree", pin)
    if proc.returncode != 0:
        raise SystemExit(f"git ls-tree failed at {pin}: {proc.stderr.strip()}")
    entries = []
    for raw in proc.stdout.split("\0"):
        if not raw:
            continue
        meta, _, path = raw.partition("\t")
        mode, kind, sha = meta.split(" ")
        if kind != "blob":
            raise SystemExit(f"{path}: tracked entry is a {kind}, not a blob")
        entries.append((mode, sha, path))
    return entries


def blob_contents(upstream: Path, shas: list[str]) -> dict[str, bytes]:
    """Read many blobs in one ``git cat-file --batch`` call, keyed by object name."""
    wanted = list(dict.fromkeys(shas))
    proc = subprocess.run(
        ["git", "-C", str(upstream), "cat-file", "--batch"],
        input=("\n".join(wanted) + "\n").encode("utf-8"),
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0:
        raise SystemExit(f"git cat-file failed: {proc.stderr.decode('utf-8', 'replace').strip()}")
    out = proc.stdout
    contents: dict[str, bytes] = {}
    offset = 0
    while offset < len(out):
        newline = out.index(b"\n", offset)
        header = out[offset:newline].decode("utf-8", "replace")
        offset = newline + 1
        sha, kind, size = header.split(" ")
        if kind != "blob":
            raise SystemExit(f"{sha}: cat-file answered a {kind}")
        length = int(size)
        contents[sha] = out[offset:offset + length]
        offset += length + 1
    missing = [sha for sha in wanted if sha not in contents]
    if missing:
        raise SystemExit(f"cat-file did not answer for {len(missing)} blob(s), first {missing[0]}")
    return contents


def skip_reason(path: str, layout=None) -> str:
    """Why this upstream path is declared a deviation in this layout, or "" when it is carried.

    ``path`` is an upstream path; ``layout`` is the tree's own shape (:func:`detect_layout`) and
    defaults to the shape of the tree this module lives in, which is what a one-argument call from a
    test or a REPL is asking about.
    """
    if layout is None:
        layout = detect_layout(ROOT)
    reasons = skip_reasons(layout)
    if path in reasons:
        return reasons[path]
    for prefix, reason in skip_prefixes(layout).items():
        if path.startswith(prefix):
            return reason
    return ""


def deviation_record(mode: str, sha: str, data: bytes, path: str, layout) -> Record:
    """One declared deviation, pinning the bytes this tree stores for it when it carries the path.

    A project-owned path or an upstream CI definition is not carried at all, so upstream's digest alone
    is the whole record. A declared normalisation *is* carried, in bytes upstream's blob can never
    equal, so the record also holds the stored digest and size -- what the checker verifies, since
    "still differs from upstream" is true of those bytes by construction.
    """
    if path in NORMALISED:
        stored = stored_bytes(path, data)
        return Record("skip", mode, sha, len(data), path, skip_reason(path, layout),
                      blob_sha1(stored), len(stored))
    return Record("skip", mode, sha, len(data), path, skip_reason(path, layout))


def write_entry(target: Path, mode: str, data: bytes, dry_run: bool) -> None:
    """Write one carried blob, replacing whatever is there (file or link)."""
    if dry_run:
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.is_symlink() or target.exists():
        target.unlink()
    if mode == "120000":
        try:
            os.symlink(data.decode("utf-8"), target)
        except (OSError, NotImplementedError):
            # A platform without symlink support checks the link out as a file holding the target.
            target.write_bytes(data)
        return
    target.write_bytes(data)
    if mode == "100755":
        os.chmod(target, 0o755)


def stored_bytes(path: str, data: bytes) -> bytes:
    """The bytes git stores for one upstream blob: declared normalisations applied.

    Only a path listed in ``NORMALISED`` is rewritten, and the rewrite is the one git's own carried
    attribute file performs on it (``text=auto eol=lf``: CRLF becomes LF when the file is stored).
    Everything else is carried byte for byte.
    """
    if path in NORMALISED:
        return data.replace(b"\r\n", b"\n")
    return data


def write_collision(root: Path, path: str) -> str:
    """Why writing ``path`` would have to replace something it must not, or "" when the way is clear.

    A pin update can change a path's shape: the revision that carried ``sub`` as a file may carry
    ``sub/alpha.txt``. Writing through a file, or over a directory, raises half-way through the tree,
    so the collision is named before anything is written.
    """
    parts = path.split("/")
    for depth in range(1, len(parts)):
        ancestor = root.joinpath(*parts[:depth])
        if ancestor.exists() and not ancestor.is_dir():
            return (f"{'/'.join(parts[:depth])} is a file, and the pin carries "
                    f"{'/'.join(parts[:depth + 1])} below it")
    target = root.joinpath(*parts)
    if target.is_dir() and not target.is_symlink():
        return f"{path} is a directory, and the pin carries a file"
    return ""


def prune_previously_carried(root: Path, previous: Manifest, keep: set[str], dry_run: bool) -> None:
    """Delete files a previous manifest carried that this revision no longer has.

    A file modified here since the previous manifest was written is not deleted: the import stops and
    names it, because that file is somebody's work and not this script's to remove.
    """
    for record in previous.files:
        if record.path in keep:
            continue
        target = root / record.path
        if not target.exists() and not target.is_symlink():
            continue
        digest, _, _ = hashed_path(root, record.path)
        if digest != record.digest:
            raise SystemExit(f"{record.path}: modified here; refusing to prune it")
        if dry_run:
            continue
        target.unlink()


def carry_entries(root: Path, entries, contents, previous: Manifest | None, prune: bool,
                  dry_run: bool) -> None:
    """Prune what this revision no longer carries, then write the pin's files.

    The prune runs before any write because a path can change shape between revisions: writing first
    would hit a path the prune is about to remove and stop half-way through the tree. Every refusal
    here is a named ``SystemExit`` -- an import that stops must say which path stopped it, not print a
    traceback with the tree already partly rewritten.
    """
    if prune and previous is not None:
        prune_previously_carried(root, previous, {path for _, _, path in entries}, dry_run)
    for mode, sha, path in entries:
        upstream = contents[sha]
        if blob_sha1(upstream) != sha:
            raise SystemExit(f"{path}: content does not hash to {sha}")
        if path in NORMALISED and stored_bytes(path, upstream) == upstream:
            raise SystemExit(
                f"{path} is declared carried with a normalisation, but nothing in the pin's bytes is "
                f"rewritten; carry it as a file instead")
        collision = write_collision(root, path)
        if collision:
            raise SystemExit(f"{collision}; remove it by hand or re-run with --prune")
        try:
            write_entry(root / path, mode, stored_bytes(path, upstream), dry_run)
        except OSError as error:
            raise SystemExit(f"{path}: writing it failed: {error}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--upstream", required=True, type=Path, help="checkout to read from")
    parser.add_argument("--pin", default="", help="revision to carry (default: the checkout's HEAD)")
    parser.add_argument("--restore-snapshot", action="store_true",
                        help="run the carried snapshot tool in --upstream first (needs network)")
    parser.add_argument("--prune", action="store_true",
                        help="delete files a previous manifest carried but this revision no longer has")
    parser.add_argument("--dry-run", action="store_true", help="report what would change; write nothing")
    args = parser.parse_args(argv)

    # The shape of *this* tree decides where the carried files are written, which paths are declared
    # deviations and what the ledger's layout line says. It is read from the tree's own go.mod files,
    # so a tree that has been moved but whose ledger has not is imported at the shape it now has.
    layout = detect_layout(ROOT)
    carry_root = ROOT / layout.carry if layout.carry else ROOT

    upstream: Path = args.upstream.resolve()
    if not (upstream / ".git").exists():
        raise SystemExit(f"{upstream} is not a git checkout")

    revision = git(upstream, "rev-parse", "HEAD")
    if revision.returncode != 0:
        raise SystemExit(f"git rev-parse HEAD failed in {upstream}: {revision.stderr.strip()}")
    head = revision.stdout.strip()
    pin = args.pin or head
    if pin != head:
        raise SystemExit(f"--pin {pin} is not the checkout's HEAD ({head}); check out the pin first")
    status = git(upstream, "status", "--porcelain").stdout.strip()
    if status:
        raise SystemExit(f"{upstream} has local modifications; carry from a clean checkout:\n{status}")

    if args.restore_snapshot:
        proc = subprocess.run(
            ["go", "run", "./internal/pricing/cmd/litellm-snapshot", "-restore"],
            cwd=upstream,
            check=False,
        )
        if proc.returncode != 0:
            raise SystemExit(f"snapshot restore failed with rc={proc.returncode}")

    entries = upstream_entries(upstream, pin)
    carried = [(mode, sha, path) for mode, sha, path in entries if not skip_reason(path, layout)]
    skipped = [(mode, sha, path) for mode, sha, path in entries if skip_reason(path, layout)]
    owned = owned_prefixes(layout)
    overlap = [path for _, _, path in carried if any(
        path == owned_path or path.startswith(owned_path) for owned_path in owned
    )]
    if overlap:
        raise SystemExit(f"carried path is declared project-owned too: {overlap[0]}")

    contents = blob_contents(upstream, [sha for _, sha, _ in entries])

    previous = load_manifest(MANIFEST) if args.prune and MANIFEST.exists() else None

    # A declared normalisation is carried, so its path is written as well -- with the bytes git will
    # store for it, not upstream's blob, so the working tree equals what a clone checks out.
    written = carried + [entry for entry in skipped if entry[2] in NORMALISED]
    carry_entries(carry_root, written, contents, previous, args.prune, args.dry_run)

    input_records: list[Record] = []
    for path in GENERATED_INPUTS:
        source = upstream / path
        target = carry_root / path
        if not source.exists():
            raise SystemExit(
                f"{path} does not exist in {upstream}: re-run with --restore-snapshot "
                f"(needs network) so the pinned artifact is fetched first"
            )
        data = source.read_bytes()
        if not args.dry_run:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
        digest = hashlib.sha256(data).hexdigest()
        pinned = pinned_snapshot_sha256(carry_root / PRICING_TOOL)
        if pinned and pinned != digest:
            raise SystemExit(f"{path} hashes to {digest}, but the carried tool pins {pinned}")
        input_records.append(Record(
            "input", "100644", digest, len(data), path,
            f"generated build input embedded by internal/pricing; upstream restores it from a pinned "
            f"artifact before every Go build (sha256 pinned in {PRICING_TOOL})",
        ))

    # Sorted, like every other record list here: the ledger is a diffable file, and an owned
    # prefix's position in the tuple above is not a fact about the tree.
    records: list[Record] = [Record("owned", "", "", 0, prefix) for prefix in sorted(owned)]
    records += [Record("file", mode, sha, len(contents[sha]), path) for mode, sha, path in sorted(
        carried, key=lambda entry: entry[2]
    )]
    records += [
        deviation_record(mode, sha, contents[sha], path, layout)
        for mode, sha, path in sorted(skipped, key=lambda entry: entry[2])
    ]
    records += input_records

    describe = git(upstream, "describe", "--tags", pin).stdout.strip()
    commit_date = git(upstream, "show", "-s", "--format=%cI", pin).stdout.strip()
    manifest = Manifest(
        {
            "pin": pin,
            "remote": REMOTE,
            "describe": describe,
            "commit-date": commit_date,
            "upstream-files": str(len(entries)),
            "carried": str(len(carried)),
            "deviations": str(len(skipped) + len(input_records)),
            "layout": layout.spelling,
        },
        records,
    )
    if not args.dry_run:
        MANIFEST.write_text(manifest.render(), encoding="utf-8")

    normalised = [path for _, _, path in skipped if path in NORMALISED]
    print(f"pin {pin} ({describe}, {commit_date})")
    print(f"layout: carry root {layout.carry or '.'}, this repository's module {layout.module}")
    print(f"upstream tracked files: {len(entries)}")
    print(f"carried byte for byte:  {len(carried)}")
    print(f"declared deviations:    {len(skipped) + len(input_records)}"
          f" ({len(skipped) - len(normalised)} upstream paths not carried, "
          f"{len(normalised)} carried with a declared normalisation, "
          f"{len(input_records)} generated input)")
    print(f"project-owned prefixes: {len(owned)}")
    if args.dry_run:
        print("dry run: nothing written")
    return 0


if __name__ == "__main__":
    sys.exit(main())
