#!/usr/bin/env python3
"""Prove the carried AgentsView tree still matches its pinned revision. Offline.

Every carried file must hash to the git blob the manifest recorded for it, every tracked file outside
the manifest must belong to a path prefix this repository owns, and every declared deviation must
still hold what the manifest declares: a project-owned path that has been replaced by upstream's copy
is a stale declaration, and a path carried with a declared normalisation must still store the bytes
its record pins -- both read from the working tree, and from git's index where git is available. The
generated build input is checked against the SHA-256 pinned inside the carried snapshot tool, so the
input is verified against upstream's own pin rather than a second hard-coded copy.

Run by the gate as the required ``parity`` step (docs/harness.md); also runnable by hand:

    python3 scripts/upstream-parity.py [--root DIR] [--quiet]
"""

from __future__ import annotations

import argparse
import hashlib
import os
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import (  # noqa: E402
    Manifest,
    ManifestError,
    hashed_path,
    load_manifest,
    pinned_snapshot_sha256,
)

ROOT = Path(__file__).resolve().parent.parent
MANIFEST_RELATIVE = Path("scripts") / "upstream-manifest.txt"
MANIFEST = ROOT / MANIFEST_RELATIVE
UPSTREAM_DOC = "docs/upstream.md"
PIN_SOURCE = "cmd/ccc/pin.go"
PIN_CONSTANT = re.compile(r'upstreamRevision\s*=\s*"([0-9a-f]{40})"')
PRICING_TOOL = "internal/pricing/cmd/litellm-snapshot/main.go"
PRICING_INPUT = "internal/pricing/snapshot/litellm_snapshot.json.gz"
REVISION = re.compile(r"\b[0-9a-f]{40}\b")

#: How many offending paths a failure prints before it stops listing them.
MAX_LISTED = 12


class Failure(Exception):
    """One parity violation, with the paths that caused it."""

    def __init__(self, summary: str, offenders: list[str]):
        super().__init__(summary)
        self.summary = summary
        self.offenders = offenders


def tree_files(root: Path) -> tuple[list[str] | None, list[str]]:
    """Paths git knows about: ``(tracked, present but untracked and not ignored)``.

    ``tracked`` is ``None`` in a tree without git (a copy, a throw-away test tree). The untracked
    list is what makes the "no unexplained file inside the carried tree" check answer the same
    before and after a commit: a tree that has not been committed yet still shows every file this
    repository would add, and an ignored path (build output, the staged SQLite header) is not one
    of them because the repository's own ignore rules already say so.
    """
    tracked = git_run(root, "ls-files", "-z")
    if tracked is None:
        return None, []
    others = git_run(root, "ls-files", "-z", "--others", "--exclude-standard")
    return decode_paths(tracked), decode_paths(others if others is not None else b"")


def ignored_paths(root: Path, paths: list[str]) -> list[str] | None:
    """The subset of ``paths`` git would skip when adding untracked files.

    ``None`` means the question could not be asked (no git, or git refused): a caller must report that
    rather than read it as "nothing is ignored", which is the one answer that would look like a pass.
    """
    if not paths:
        return []
    try:
        proc = subprocess.run(
            ["git", "-C", str(root), "check-ignore", "-z", "--stdin", "--no-index"],
            input="\0".join(paths).encode("utf-8", "surrogateescape"),
            capture_output=True,
            check=False,
        )
    except OSError:
        return None
    if proc.returncode not in (0, 1):  # 1 means "nothing ignored", which is the good answer
        return None
    return decode_paths(proc.stdout)


def git_run(root: Path, *args: str) -> bytes | None:
    """Run one git query in a tree, or answer None when git cannot be asked at all.

    A host without git is a state this check has to survive: the disclosure that the git-dependent
    rules were skipped is the answer, and a traceback is not. The Windows CI job and a stripped
    container are both real hosts of that shape.
    """
    try:
        proc = subprocess.run(["git", "-C", str(root), *args], capture_output=True, check=False)
    except OSError:
        return None
    if proc.returncode != 0:
        return None
    return proc.stdout


def decode_paths(raw: bytes) -> list[str]:
    return [entry for entry in raw.decode("utf-8", "surrogateescape").split("\0") if entry]


def git_index(root: Path) -> dict[str, tuple[str, str]] | None:
    """``path -> (mode, blob)`` for everything git has staged or committed, or None without git.

    This is the half a fresh clone sees: ``git add`` applies ``.gitattributes`` filters, so a file
    whose blob contains CRLF is stored normalised while the checkout may keep its own line endings.
    Comparing the *index* therefore answers "will the next clone have the pinned bytes?", which
    hashing the working tree cannot answer on its own.
    """
    raw = git_run(root, "ls-files", "-s", "-z")
    if raw is None:
        return None
    index: dict[str, tuple[str, str]] = {}
    for entry in raw.decode("utf-8", "surrogateescape").split("\0"):
        if not entry or "\t" not in entry:
            continue
        meta, _, path = entry.partition("\t")
        parts = meta.split()
        if len(parts) != 3 or parts[2] != "0":
            continue
        index[path] = (parts[0], parts[1])
    return index


def git_modified(root: Path) -> set[str] | None:
    """Paths whose *working-tree* content differs from the index, or None without git.

    Porcelain prints two columns: the first is the index against ``HEAD``, the second is the working
    tree against the index. Only the second means "what is on disk is not what git stores", which is
    the difference this check is about -- every carried file reads ``A`` in the first column until the
    increment is committed, and none of them is modified on disk.
    """
    raw = git_run(root, "status", "--porcelain", "-z", "--untracked-files=no")
    if raw is None:
        return None
    modified: set[str] = set()
    for token in raw.decode("utf-8", "surrogateescape").split("\0"):
        if len(token) < 4:
            continue
        status, path = token[:2], token[3:]
        if status[1] != " ":
            modified.add(path)
    return modified


def executable_bit_supported() -> bool:
    """Whether this filesystem reports POSIX mode bits at all.

    Windows answers every ``st_mode & 0o111`` with 0, so checking an executable bit there would fail
    every carried ``100755`` file; the check is therefore answered where the bits exist, and the run
    says so when they do not.
    """
    return os.name != "nt"


def present(root: Path, path: str) -> bool:
    target = root / path
    return target.exists() or target.is_symlink()


def check_files(root: Path, manifest: Manifest, index: dict[str, tuple[str, str]] | None) -> tuple[int, int, int]:
    """Verify every carried file against the pin.

    With git available the check reads the *index* -- what a fresh clone would receive, filters and
    all -- and asks git which paths differ from it in the working tree. Without git it falls back to
    hashing the working tree's bytes directly, which is the same answer for a tree whose files were
    written by the importer.

    Returns ``(checked, mode_unverified, symlinks, modified_in_worktree)``.
    """
    missing, drifted, wrong_shape, wrong_mode = [], [], [], []
    size_mismatch: list[str] = []
    mode_unverified = 0
    symlinks_hashed = 0
    dirty: list[str] = []
    modified = git_modified(root) if index is not None else None
    if index is not None and modified is None:
        # A probe that could not answer must not read as "nothing is modified": that is the one answer
        # which would look like a pass. The ignore probe below fails for the same reason.
        raise Failure(
            "could not ask git which carried paths differ from what it stores for them, so the check "
            "cannot be completed",
            ["git status --porcelain"],
        )
    for record in manifest.files:
        if not present(root, record.path):
            missing.append(record.path)
            continue
        target = root / record.path
        if index is not None:
            mode, blob = index.get(record.path, ("", ""))
            if not mode:
                # Present on disk but absent from the index: it would not exist in a fresh clone.
                missing.append(record.path)
                continue
            if blob != record.digest:
                drifted.append(record.path)
                continue
            if mode != record.mode:
                wrong_mode.append(record.path)
                continue
            if modified is not None and record.path in modified:
                dirty.append(record.path)
                continue
            if record.mode == "120000":
                symlinks_hashed += 1
            continue
        is_link = target.is_symlink()
        if record.mode == "120000" and not is_link and not target.is_file():
            wrong_shape.append(record.path)
            continue
        if record.mode != "120000" and (is_link or not target.is_file()):
            wrong_shape.append(record.path)
            continue
        digest, size, _ = hashed_path(root, record.path)
        if digest != record.digest:
            drifted.append(record.path)
            continue
        if size != record.size:
            size_mismatch.append(record.path)
            continue
        if record.mode == "120000":
            symlinks_hashed += 1
            continue
        if not executable_bit_supported():
            mode_unverified += 1
            continue
        # Only a platform with POSIX modes can answer this; the digest above already proves the
        # committed *content*, and git records the mode in the index, so a mismatch here means the
        # working tree disagrees with the pin about whether the file is executable.
        executable = bool(target.stat().st_mode & 0o111)
        if executable != (record.mode == "100755"):
            wrong_mode.append(record.path)
    if missing:
        raise Failure(f"{len(missing)} carried file(s) are absent", missing)
    if wrong_shape:
        raise Failure(f"{len(wrong_shape)} carried path(s) changed file type", wrong_shape)
    if drifted:
        raise Failure(
            f"{len(drifted)} carried file(s) differ from the pinned revision", drifted
        )
    if size_mismatch:
        raise Failure(f"{len(size_mismatch)} carried file(s) changed size", size_mismatch)
    if wrong_mode:
        raise Failure(
            f"{len(wrong_mode)} carried file(s) changed mode (executable bit or file type)",
            wrong_mode,
        )
    if dirty:
        raise Failure(
            f"{len(dirty)} carried file(s) differ from what git stores for them", dirty
        )
    return len(manifest.files), mode_unverified, symlinks_hashed


def check_untracked(root: Path, manifest: Manifest, tracked: list[str] | None,
                    others: list[str]) -> int:
    """Nothing git would add may sit outside the manifest unless this repository owns it."""
    if tracked is None:
        return 0
    # Every path the manifest names is accounted for, whichever record kind names it: a carried file,
    # a declared deviation (present or absent) or a generated build input.
    named = {record.path for record in manifest.records if record.kind != "owned"}
    seen = sorted(set(tracked) | set(others))
    unexplained = [path for path in seen if path not in named and not manifest.is_owned(path)]
    if unexplained:
        raise Failure(
            f"{len(unexplained)} file(s) are neither carried nor project-owned",
            unexplained,
        )
    return len(seen)


def check_not_ignored(root: Path, manifest: Manifest, tracked: list[str] | None) -> None:
    """A carried file the ignore rules would skip is not carried by a fresh clone either."""
    if tracked is None:
        return
    ignored = ignored_paths(root, [record.path for record in manifest.files])
    if ignored is None:
        raise Failure(
            "could not ask git which carried paths are ignored, so the check cannot be completed",
            ["git check-ignore"],
        )
    if ignored:
        raise Failure(
            f"{len(ignored)} carried path(s) match this tree's ignore rules and would be skipped "
            f"by git add",
            ignored,
        )


def check_deviations(
    root: Path, manifest: Manifest, index: dict[str, tuple[str, str]] | None
) -> tuple[list[str], list[str], list[str]]:
    """Each declared deviation must still differ from upstream, or be carried as it declares.

    Two kinds of declaration exist, and both are checked against the *working tree* as well as git's
    index, because the gate reads the working tree (`docs/harness.md`) and a file that quietly became
    upstream's copy is the same stale declaration whether it was staged or only written on disk.

    - a record with no stored digest declares "upstream's copy is not carried here": the bytes git
      stores for the path and the bytes on disk must both differ from upstream's blob;
    - a record with a stored digest declares a path this tree carries anyway, in bytes upstream's blob
      can never equal (a declared normalisation): the stored bytes are what must hold -- the index
      where git is available, the file's own bytes where it is not. A working-tree copy that still
      holds upstream's pre-normalisation bytes is named, not failed: git itself reads that copy as
      clean, and what a clone receives is the stored form.

    Returns ``(still_differing, stored, pre_normalisation_on_disk)``.
    """
    stale: list[str] = []
    notes: list[str] = []
    stored_notes: list[str] = []
    on_disk_only: list[str] = []
    for record in manifest.skips:
        target = root / record.path
        # Only a file or a link has bytes to compare. A directory where the manifest declares a file
        # reads as "not carried": that is the answer the git branch already gives for it (a directory
        # cannot be in the index), and hashing it would raise instead of reporting.
        on_disk = hashed_path(root, record.path) if target.is_file() or target.is_symlink() else None
        stored = index.get(record.path) if index is not None else None
        if stored is None and on_disk is None:
            if not record.stored_digest:
                continue  # not carried at all: the declaration is "absent", which is not stale
            stale.append(f"{record.path}: absent, but the manifest records the bytes this tree stores")
            continue
        if record.stored_digest:
            if record.stored_digest == record.digest:
                stale.append(
                    f"{record.path}: the stored bytes are recorded as upstream's own, so the path is "
                    f"carried, not deviated")
                continue
            if index is not None:
                if stored is None:
                    stale.append(
                        f"{record.path}: absent from git's index, but the manifest records the bytes "
                        f"this tree stores")
                    continue
                if stored[1] != record.stored_digest:
                    stale.append(
                        f"{record.path}: git stores {stored[1]}, the manifest records "
                        f"{record.stored_digest}")
                    continue
            elif on_disk[0] not in (record.stored_digest, record.digest):
                stale.append(
                    f"{record.path}: the file hashes to {on_disk[0]}, neither the bytes this tree "
                    f"stores ({record.stored_digest}) nor upstream's own ({record.digest})")
                continue
            if on_disk is not None and on_disk[0] == record.digest:
                # A working-tree copy still holding the pre-normalisation bytes: git reads that copy
                # as clean (its attribute file normalises it on comparison) and a clone checks out the
                # stored form, so the declaration holds -- but the copy is named, not passed over.
                on_disk_only.append(record.path)
            elif on_disk is not None and on_disk[0] != record.stored_digest:
                stale.append(
                    f"{record.path}: the working tree hashes to {on_disk[0]}, neither the bytes this "
                    f"tree stores ({record.stored_digest}) nor upstream's own ({record.digest})")
                continue
            stored_notes.append(record.path)
            continue
        if stored is not None and stored[1] == record.digest:
            stale.append(f"{record.path}: it holds upstream's own bytes, so the declaration is stale")
            continue
        if on_disk is not None and on_disk[0] == record.digest:
            stale.append(
                f"{record.path}: the working tree holds upstream's own bytes, so the declaration "
                f"is stale")
            continue
        notes.append(record.path)
    if stale:
        raise Failure(
            f"{len(stale)} declared deviation(s) do not hold what the manifest declares",
            stale,
        )
    return notes, stored_notes, on_disk_only


def check_inputs(root: Path, manifest: Manifest) -> list[str]:
    """Generated build inputs: hash them, and cross-check the one upstream pins in its own tool."""
    problems: list[str] = []
    for record in manifest.inputs:
        if not present(root, record.path):
            problems.append(f"{record.path}: absent")
            continue
        data = (root / record.path).read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest != record.digest or len(data) != record.size:
            problems.append(f"{record.path}: hashes to {digest}, manifest says {record.digest}")
    if problems:
        raise Failure(f"{len(problems)} generated input(s) changed", problems)

    pinned = pinned_snapshot_sha256(root / PRICING_TOOL)
    recorded = next((r.digest for r in manifest.inputs if r.path == PRICING_INPUT), "")
    if not pinned:
        raise Failure(
            f"{PRICING_TOOL} no longer carries the pinned snapshot SHA-256 the input is verified against",
            [PRICING_TOOL],
        )
    if recorded != pinned:
        raise Failure(
            f"{PRICING_INPUT} is recorded as {recorded} but {PRICING_TOOL} pins {pinned}",
            [PRICING_INPUT],
        )
    return [f"{PRICING_INPUT} sha256={recorded} (pinned by {PRICING_TOOL})"]


def check_pin_documented(root: Path, manifest: Manifest) -> None:
    pin = manifest.header.get("pin", "")
    if not REVISION.fullmatch(pin or ""):
        raise Failure("the manifest does not record a 40-hex revision", [MANIFEST.name])
    doc = root / UPSTREAM_DOC
    if not doc.exists() or pin not in doc.read_text(encoding="utf-8"):
        raise Failure(f"{UPSTREAM_DOC} does not record the pinned revision {pin}", [UPSTREAM_DOC])


def check_reported_pin(root: Path, manifest: Manifest) -> str:
    """The revision cmd/ccc reports must be the pin, or the binary would lie about itself."""
    pin = manifest.header["pin"]
    source = root / PIN_SOURCE
    if not source.exists():
        raise Failure(f"{PIN_SOURCE} is missing, so the revision the CLI reports cannot be checked",
                      [PIN_SOURCE])
    reported = PIN_CONSTANT.search(source.read_text(encoding="utf-8"))
    if not reported:
        raise Failure(f"{PIN_SOURCE} no longer carries a literal upstreamRevision constant",
                      [PIN_SOURCE])
    if reported.group(1) != pin:
        raise Failure(
            f"{PIN_SOURCE} reports {reported.group(1)} but the carried revision is {pin}",
            [PIN_SOURCE])
    return reported.group(1)


def run(root: Path) -> list[str]:
    manifest = load_manifest(root / MANIFEST_RELATIVE)
    check_pin_documented(root, manifest)
    reported_pin = check_reported_pin(root, manifest)
    tracked, others = tree_files(root)
    index = git_index(root)
    carried_checked, mode_unverified, symlinks_hashed = check_files(root, manifest, index)
    check_not_ignored(root, manifest, tracked)
    seen = check_untracked(root, manifest, tracked, others)
    deviations, stored, on_disk_only = check_deviations(root, manifest, index)
    inputs = check_inputs(root, manifest)

    scope = "(git-less tree: file-discovery checks skipped)" if tracked is None else ""
    return [
        f"pin {manifest.header['pin']} ({manifest.header.get('describe', '')})",
        f"reported by cmd/ccc: {reported_pin}",
        f"carried: {carried_checked} file(s) identical to the pinned revision"
        + ("" if index is None else " (compared through git's own object store)")
        + f" {scope}".rstrip(),
        f"deviations: {len(manifest.skips)} upstream path(s) declared "
        f"({len(stored)} of them carried with the bytes this tree stores)"
        f" + {len(manifest.inputs)} generated input(s), all present as declared",
        f"owned: {len(manifest.owned)} project-owned prefix(es); files seen by git: {seen}",
        f"inputs: {'; '.join(inputs)}",
    ] + (
        [f"symlinks hashed by their link target: {symlinks_hashed}"] if symlinks_hashed else []
    ) + (
        [f"executable bits not verified for {mode_unverified} file(s): this platform has no POSIX "
         f"mode bits"] if mode_unverified else []
    ) + (
        [
            "declared deviations still differ from upstream: {listed}{more}".format(
                listed=" ".join(deviations[:MAX_LISTED]),
                more=" …" if len(deviations) > MAX_LISTED else "",
            )
        ]
        if deviations else []
    ) + (
        [
            "declared stored bytes verified: {listed}{more}".format(
                listed=" ".join(stored[:MAX_LISTED]),
                more=" …" if len(stored) > MAX_LISTED else "",
            )
        ]
        if stored else []
    ) + (
        [
            "working tree still holds upstream's pre-normalisation bytes for: {listed}{more} "
            "(git stores the declared bytes; a checkout writes those)".format(
                listed=" ".join(on_disk_only[:MAX_LISTED]),
                more=" …" if len(on_disk_only) > MAX_LISTED else "",
            )
        ]
        if on_disk_only else []
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--root", type=Path, default=ROOT, help="tree to check (default: this repo)")
    parser.add_argument("--quiet", action="store_true", help="print nothing when the check passes")
    args = parser.parse_args(argv)

    try:
        lines = run(args.root.resolve())
    except ManifestError as broken:
        print(f"parity: FAIL — {MANIFEST.name} is malformed: {broken}")
        return 1
    except Failure as failure:
        print(f"parity: FAIL — {failure.summary}")
        for offender in failure.offenders[:MAX_LISTED]:
            print(f"  {offender}")
        if len(failure.offenders) > MAX_LISTED:
            print(f"  … and {len(failure.offenders) - MAX_LISTED} more")
        return 1
    if not args.quiet:
        for line in lines:
            print(line)
    return 0


if __name__ == "__main__":
    sys.exit(main())
