#!/usr/bin/env python3
"""Prove the carried AgentsView tree still matches its pinned revision. Offline.

Every carried file must hash to the git blob the manifest recorded for it, every tracked file outside
the manifest must belong to a path prefix this repository owns, and every declared deviation must
still hold what the manifest declares: a project-owned path that has been replaced by upstream's copy
is a stale declaration, and a path carried with a declared normalisation must still store the bytes
its record pins -- both read from the working tree, and from git's index where git is available. The
working tree is hashed in both branches, with git and without it, so the one answer a carried path can
be configured to suppress -- git's own ``status`` report, silenced by an ``assume-unchanged`` or
``skip-worktree`` index flag -- is never the answer a carried file's content is checked through; git
adds what a stat cannot answer, namely what a fresh clone receives. A path carried with a declared
normalisation is a carried path for every rule but its bytes: its presence, shape and executable bit
are checked against the file this tree ships, in both branches, so the allowance can never pass for a
copy that was changed in a way the bytes do not show -- ``git status`` is as silent about a mode edit
as it is about an unflagged content edit when the checkout does not record one. The generated build
input is checked against the SHA-256 pinned inside the carried snapshot tool, so the input is verified
against upstream's own pin rather than a second hard-coded copy, and -- where git is available --
against git's index as well: that input is ignored by the carried ``snapshot/.gitignore``, so a copy
that is present on disk but absent from the index is missing from every fresh clone and the carried
``go:embed`` cannot compile there.

A carried symlink (a ``120000`` record) is a link, or the regular file holding the link's target text
that a checkout which does not create symlinks has at that path -- and only where that is what *this*
checkout does, which git's own ``core.symlinks`` states: the two shapes are the same git object, so
the shape is the only thing that can tell the pin's link from a file that replaced it, and a checkout
with symlinks enabled that replaced one would otherwise pass on bytes, index mode and a
``skip-worktree``-silenced ``git status`` alike. The run says how many such file-form links it
accepted, so the allowance is visible in its own answer.

Every git query this check makes is required: when git answers the file list but refuses the untracked
list, the index or the status report, the step fails naming the probe instead of skipping the rule it
answers for. "Nothing would be added" and "nothing is modified" are the answers that look like a pass,
and neither may come from a question git never answered.

A tree git cannot enumerate -- no git on this host, or a root that is a copy rather than a repository
-- is reported as ``INCOMPLETE`` and exits 2: the file-list and ignore rules cannot be asked, and the
gate runs this script as a required step that rejects any non-zero outcome, so an unverified tree is
never a pass. Everything answerable is still checked first, so real drift in such a tree is a failure
with its offenders.

Run by the gate as the required ``parity`` step (docs/harness.md); also runnable by hand:

    python3 scripts/upstream-parity.py [--root DIR] [--quiet]

Exit codes: 0 the tree is verified, 1 a rule is violated, 2 the check could not be completed here.
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
    Record,
    blob_sha1,
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


class Incomplete(Exception):
    """Verification this host cannot complete, which is not a pass.

    Distinct from ``Failure``: nothing was found wrong with the tree, the tree could not be *checked*
    -- a host without git cannot enumerate it. The gate runs this script as a required step and any
    non-zero outcome fails it, so an incomplete run can never be read as agreement with the pin; the
    checks that do not need a file list still run first, so real drift is still reported as a failure
    with its offenders instead of being hidden behind the incompleteness.
    """

    def __init__(self, summary: str, skipped: list[str]):
        super().__init__(summary)
        self.summary = summary
        self.skipped = skipped


def tree_files(root: Path) -> tuple[list[str] | None, list[str] | None]:
    """Paths git knows about: ``(tracked, present but untracked and not ignored)``.

    ``tracked`` is ``None`` in a tree without git (a copy, a throw-away test tree), and then the
    untracked list is ``None`` as well: the file list and every rule derived from it is what the
    caller's INCOMPLETE verdict covers. The untracked list is *also* ``None`` when git answered the
    file list but refused the untracked query, which the caller reports as a failure -- an empty list
    there would say "nothing would be added", the one answer that looks like a pass, for a question git
    never answered.

    The untracked list is what makes the "no unexplained file inside the carried tree" check answer the
    same before and after a commit: a tree that has not been committed yet still shows every file this
    repository would add, and an ignored path (build output, the staged SQLite header) is not one of
    them because the repository's own ignore rules already say so.
    """
    tracked = git_run(root, "ls-files", "-z")
    if tracked is None:
        return None, None
    others = git_run(root, "ls-files", "-z", "--others", "--exclude-standard")
    return decode_paths(tracked), (None if others is None else decode_paths(others))


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


def git_config_bool(root: Path, key: str) -> tuple[bool | None, bool]:
    """The boolean git records for ``key`` in a tree, and whether git answered at all.

    ``git config --type=bool --get <key>`` exits ``1`` with no output when the key is set nowhere --
    a real answer ("the platform's own default applies"), reported here as ``(None, True)`` -- and
    fails differently when the configuration itself cannot be read. That last shape is
    ``(None, False)``: a caller holding a git-backed answer for this tree must refuse it rather than
    read it as the default, or the rule it decides would rest on a question nobody answered.
    """
    try:
        proc = subprocess.run(
            ["git", "-C", str(root), "config", "--type=bool", "--get", key],
            capture_output=True,
            check=False,
        )
    except OSError:
        return None, False
    if proc.returncode == 0:
        answer = proc.stdout.strip().decode("utf-8", "surrogateescape").lower()
        if answer in ("true", "false"):
            return answer == "true", True
        return None, False
    if proc.returncode == 1 and not proc.stdout.strip():
        return None, True
    return None, False


def platform_name() -> str:
    """The platform this process runs on, as ``os.name`` spells it (``posix``, ``nt``).

    A named seam rather than a bare ``os.name`` read, because the rule that reads it
    (:func:`symlinks_unavailable`) has two answers and a case has to be able to pin both: a case that
    asserts one of them from the host it happens to run on pins nothing about the other, and the
    suite's own rule is that no case depends on which platform is running it.
    """
    return os.name


def symlinks_unavailable() -> bool:
    """Whether this platform's git checkouts cannot create a symlink at all.

    Windows is the platform where they cannot without a privilege and a git build that uses it, which
    is why Git for Windows records ``core.symlinks=false`` in its system configuration (and
    ``actions/checkout`` sets it for the same reason): such a checkout has a regular file holding the
    link's target text where the pin has the link. POSIX platforms create symlinks, so an unset
    ``core.symlinks`` there means the link is a link.
    """
    return platform_name() == "nt"


def link_paths_may_be_files(root: Path, git_tree: bool) -> bool:
    """Whether a carried ``120000`` record may be a regular file holding the link's target text here.

    git stores a symlink as the link's target text, so a checkout that does not create symlinks holds
    *that* text in a regular file at the path and its bytes are the pin's blob -- the same object. The
    allowance is for those checkouts only: where links *are* created, a file holding the target text
    is a shape the pin does not have, and it is indistinguishable from the pin by every other
    predicate this check makes -- its bytes hash to the recorded blob, its index entry can keep the
    pinned ``120000`` mode while ``git status`` is silenced by an ``assume-unchanged``/``skip-worktree``
    flag -- so accepting it unconditionally would pass for a carried path whose working-tree type, and
    behaviour, had changed.

    git's own answer for the checkout decides when it has one (``core.symlinks=false`` states the
    representation; an unset key is the platform's default, which :func:`symlinks_unavailable` knows).
    A tree git is answering for, where the query itself failed, is a failure naming the probe: that
    answer must not be guessed.
    """
    value, answered = git_config_bool(root, "core.symlinks")
    if not answered and git_tree:
        raise Failure(
            "could not ask git whether this checkout creates symlinks, so a carried link's shape "
            "cannot be verified",
            ["git config --type=bool --get core.symlinks"],
        )
    if value is not None:
        return value is False
    return symlinks_unavailable()


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


def carried_paths(manifest: Manifest) -> list[str]:
    """Every path this tree must hold: the carried files plus the deviations it carries anyway.

    A stored-byte deviation is a carried path like any other for discovery and for the rules about the
    working tree: only its *bytes* differ, because upstream's blob can never be what that path holds
    (that is why it is declared). Its digest is checked by :func:`check_deviations`, which is the one
    place that knows about the allowance; leaving it out of the ignore check (or of the presence check
    there) would let a normalised carried file be deleted or ignored and still pass.
    """
    return [record.path for record in manifest.files] + [
        record.path for record in manifest.stored_deviations
    ]


def worktree_mode(root: Path, record: Record) -> tuple[str, bool]:
    """Whether a carried path's *working tree* copy carries the record's executable bit.

    Answers ``(problem, mode_unverified)``: ``"mode"`` when the copy's executable bit is not the one
    the record names, ``""`` when it is, and ``True`` for ``mode_unverified`` when this platform has
    no POSIX mode bits to answer with at all (Windows answers every ``st_mode & 0o111`` with 0).

    This is the one answer a checkout can be configured not to have: with ``core.filemode=false`` --
    and always where the filesystem keeps no mode bits -- an executable bit flipped on the file this
    repository ships leaves its bytes, git's index mode and ``git status`` all saying "unchanged", so
    a check that read any of those as the mode's answer would pass for a modified carried path. It is
    therefore asked of the file itself, for every carried path, the same way in both branches.
    """
    if record.mode == "120000":
        # A link stores its target as its content and has no executable bit of its own to compare.
        return "", False
    if not executable_bit_supported():
        return "", True
    if bool((root / record.path).stat().st_mode & 0o111) != (record.mode == "100755"):
        return "mode", False
    return "", False


def worktree_state(root: Path, record: Record, link_files_allowed: bool) -> tuple[str, bool]:
    """How a carried file's *working tree* copy stands against the record the manifest keeps.

    Answers ``(problem, mode_unverified)``: ``problem`` is ``""`` when the copy is the pinned bytes in
    the shape and mode the record names, and otherwise the first rule it breaks -- ``"shape"`` for a
    path that is a directory, or a link where the pin carries a file (or the reverse), ``"drifted"``
    for other bytes than the pinned blob, ``"size"`` for another size, ``"mode"`` for another
    executable bit (see :func:`worktree_mode`). ``mode_unverified`` is True only when the copy is
    otherwise right and this platform has no POSIX mode bits to answer with.

    A ``120000`` record's copy is a link, or a file holding the link's target text where this checkout
    is known to write links that way (``link_files_allowed``, decided once per run by
    :func:`link_paths_may_be_files`); anything else at such a path is a shape the pin does not have.

    Every carried path is asked this with git as well as without it, because ``git status`` is the one
    answer a path can be configured to suppress: an ``assume-unchanged`` or ``skip-worktree`` index
    flag keeps the edit out of git's own report while the blob in the index stays pinned, and a check
    that read "git reports nothing modified" as "the file is the pin's copy" would pass for modified
    carried source. The comparison is the one the bytes-only branch always made, so both branches
    answer alike; only the index adds what a stat cannot answer, namely what a fresh clone receives.
    """
    target = root / record.path
    is_link = target.is_symlink()
    if record.mode == "120000":
        # git stores a link as its target text; only a checkout that does not create symlinks holds
        # that text in a file, which is the one other shape the pin's link can have.
        if not is_link and not (link_files_allowed and target.is_file()):
            return "shape", False
    elif is_link or not target.is_file():
        return "shape", False
    digest, size, _ = hashed_path(root, record.path)
    if digest != record.digest:
        return "drifted", False
    if size != record.size:
        return "size", False
    return worktree_mode(root, record)


def check_files(
    root: Path, manifest: Manifest, index: dict[str, tuple[str, str]] | None,
    modified: set[str] | None, link_files_allowed: bool,
) -> tuple[int, int, int, int]:
    """Verify every carried file against the pin.

    The working tree is verified first and in both branches, by hashing the file this repository
    ships (see :func:`worktree_state`), so an index flag that silences ``git status`` cannot hide a
    modified carried file. With git available the check then reads the *index* -- what a fresh clone
    would receive, filters and all -- and asks git which paths differ from it in the working tree
    (``modified``, probed once by the caller and shared with the deviation rule below). Without git
    the bytes already hashed are the whole answer, which is the same answer for a tree whose files
    were written by the importer.

    Returns ``(checked, mode_unverified, symlinks_hashed, link_files_seen)``, where
    ``link_files_seen`` counts the carried links this checkout holds as a file with the link's target
    text: the shapes :func:`link_paths_may_be_files` allowed, which the run discloses.
    """
    missing, drifted, wrong_shape, wrong_mode = [], [], [], []
    size_mismatch: list[str] = []
    mode_unverified = 0
    symlinks_hashed = 0
    link_files_seen = 0
    dirty: list[str] = []
    for record in manifest.files:
        if not present(root, record.path):
            missing.append(record.path)
            continue
        problem, mode_unverified_here = worktree_state(root, record, link_files_allowed)
        if problem == "shape":
            wrong_shape.append(record.path)
            continue
        if problem == "drifted":
            drifted.append(record.path)
            continue
        if problem == "size":
            size_mismatch.append(record.path)
            continue
        if problem == "mode":
            wrong_mode.append(record.path)
            continue
        if mode_unverified_here:
            mode_unverified += 1
        if record.mode == "120000":
            symlinks_hashed += 1
            if not (root / record.path).is_symlink():
                link_files_seen += 1
        if index is None:
            continue
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
    return len(manifest.files), mode_unverified, symlinks_hashed, link_files_seen


def check_untracked(root: Path, manifest: Manifest, tracked: list[str] | None,
                    others: list[str] | None) -> int:
    """Nothing git would add may sit outside the manifest unless this repository owns it.

    A tree git could not enumerate answers ``0`` here -- this function cannot tell "nothing is
    unexplained" from "nothing was listed" -- and the caller turns that into an incomplete run, which
    the required gate step rejects. An untracked list the caller could not obtain is refused there
    too, so an empty ``others`` here is always a real answer from git.
    """
    if tracked is None:
        return 0
    # Every path the manifest names is accounted for, whichever record kind names it: a carried file,
    # a declared deviation (present or absent) or a generated build input.
    named = {record.path for record in manifest.records if record.kind != "owned"}
    seen = sorted(set(tracked) | set(others or []))
    unexplained = [path for path in seen if path not in named and not manifest.is_owned(path)]
    if unexplained:
        raise Failure(
            f"{len(unexplained)} file(s) are neither carried nor project-owned",
            unexplained,
        )
    return len(seen)


def check_not_ignored(root: Path, manifest: Manifest, tracked: list[str] | None) -> None:
    """A carried file the ignore rules would skip is not carried by a fresh clone either.

    Every path this tree carries is asked about, a declared deviation carried anyway included: that
    path is in the tree for the same reason as any other, and an ignore rule that matches it would
    keep it out of a fresh clone. A tree git could not enumerate is answered by the caller's
    incomplete verdict; this function says nothing about it, because "nothing is ignored" is the one
    answer that would look like a pass.
    """
    if tracked is None:
        return
    ignored = ignored_paths(root, carried_paths(manifest))
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
    root: Path, manifest: Manifest, index: dict[str, tuple[str, str]] | None,
    modified: set[str] | None,
) -> tuple[list[str], list[str], list[str], int]:
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
      clean, and what a clone receives is the stored form. That path is a carried path in every other
      respect, so the rules that need no byte comparison apply to it too -- its shape and mode from
      git's index, its presence in the working tree, its *working-tree executable bit* (asked of the
      file itself by :func:`worktree_mode`, since neither the bytes nor a checkout with
      ``core.filemode=false`` answers it), and a working tree git reports as modified.
      :func:`check_not_ignored` asks the ignore rules about the same list of paths. The declaration is
      an allowance for its bytes, never for its absence or its mode.

    Returns ``(still_differing, stored, pre_normalisation_on_disk, mode_unverified)``.
    """
    stale: list[str] = []
    notes: list[str] = []
    stored_notes: list[str] = []
    on_disk_only: list[str] = []
    dirty: list[str] = []
    wrong_mode: list[str] = []
    mode_unverified = 0
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
                if stored[0] != record.mode:
                    stale.append(
                        f"{record.path}: git stores mode {stored[0]}, the manifest records "
                        f"{record.mode}")
                    continue
                if on_disk is None:
                    # git's index holding the path is not the working tree holding it, and it is the
                    # working tree this repository ships: a checkout that lost the file (or turned it
                    # into a directory) is a carried path that is gone, not a declaration that holds.
                    stale.append(
                        f"{record.path}: it is carried, but the working tree holds no file at it")
                    continue
                if modified is not None and record.path in modified:
                    dirty.append(record.path)
                    continue
            elif on_disk[0] not in (record.stored_digest, record.digest):
                stale.append(
                    f"{record.path}: the file hashes to {on_disk[0]}, neither the bytes this tree "
                    f"stores ({record.stored_digest}) nor upstream's own ({record.digest})")
                continue
            # The bytes are this path's own, but the copy this tree ships is a carried path in every
            # other respect, so its executable bit is asked of the file itself -- the one answer a
            # checkout with core.filemode=false, and a git index that never saw the edit, cannot give.
            problem, mode_unverified_here = worktree_mode(root, record)
            if problem == "mode":
                wrong_mode.append(record.path)
                continue
            if mode_unverified_here:
                mode_unverified += 1
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
    if wrong_mode:
        raise Failure(
            f"{len(wrong_mode)} carried path(s) changed mode (the executable bit)", wrong_mode
        )
    if dirty:
        raise Failure(
            f"{len(dirty)} carried path(s) differ from what git stores for them", dirty
        )
    if stale:
        raise Failure(
            f"{len(stale)} declared deviation(s) do not hold what the manifest declares",
            stale,
        )
    return notes, stored_notes, on_disk_only, mode_unverified


def check_inputs(
    root: Path, manifest: Manifest, index: dict[str, tuple[str, str]] | None
) -> list[str]:
    """Generated build inputs: hash them, cross-check the one upstream pins in its own tool, and -- with
    git -- verify that the index carries them.

    Hashing the working tree is not enough on its own. This tree's generated input is *ignored* by the
    carried ``internal/pricing/snapshot/.gitignore`` (upstream's Makefile restores it from a pinned
    artifact branch before every build), so git never lists it as untracked either: a copy that sits on
    disk but is absent from git's index is missing from every fresh clone, where the carried
    ``internal/pricing/fallback.go`` then has no ``go:embed`` input and the module does not compile. What
    a clone receives is what git stores, so the path's presence, its mode and the blob its own bytes hash
    to are checked in the index as well -- the same questions every carried file is asked there.
    """
    problems: list[str] = []
    for record in manifest.inputs:
        if not present(root, record.path):
            problems.append(f"{record.path}: absent")
            continue
        data = (root / record.path).read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest != record.digest or len(data) != record.size:
            problems.append(f"{record.path}: hashes to {digest}, manifest says {record.digest}")
            continue
        if index is None:
            continue
        stored = index.get(record.path)
        if stored is None:
            problems.append(
                f"{record.path}: absent from git's index, so a fresh clone would not receive the "
                f"build input the carried go:embed needs (git add -f it)")
            continue
        if stored[0] != record.mode:
            problems.append(
                f"{record.path}: git stores mode {stored[0]}, the manifest records {record.mode}")
            continue
        blob = blob_sha1(data)
        if stored[1] != blob:
            problems.append(f"{record.path}: git stores {stored[1]}, its bytes hash to {blob}")
    if problems:
        raise Failure(f"{len(problems)} generated input(s) are not what this tree declares", problems)

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
    if tracked is not None and others is None:
        # git answered the file list but refused the untracked query. Reading that as "git would add
        # nothing" is the one answer that looks like a pass, so the probe's silence fails the step.
        raise Failure(
            "could not ask git which untracked paths this tree would add, so the unexplained-file "
            "check cannot be completed",
            ["git ls-files --others --exclude-standard"],
        )
    index = git_index(root)
    if tracked is not None and index is None:
        # The same rule for the index: without it, neither what a fresh clone receives nor what git
        # reports as modified can be checked, and a run that skipped both would still have printed a
        # PASS. A tree git cannot enumerate at all is the other answer -- the caller's INCOMPLETE
        # verdict below -- and is not this case: `tracked` answered there.
        raise Failure(
            "could not read git's index, so what a fresh clone would receive cannot be verified",
            ["git ls-files -s"],
        )
    modified = git_modified(root) if index is not None else None
    if index is not None and modified is None:
        # A probe that could not answer must not read as "nothing is modified": that is the one answer
        # which would look like a pass. The ignore probe fails for the same reason.
        raise Failure(
            "could not ask git which carried paths differ from what it stores for them, so the check "
            "cannot be completed",
            ["git status --porcelain"],
        )
    # How a carried link may be held is a property of this checkout, decided once per run and only
    # when the manifest carries a link: a tree with no 120000 record has no shape question to answer.
    link_files_allowed = (
        link_paths_may_be_files(root, git_tree=tracked is not None)
        if any(record.mode == "120000" for record in manifest.files) else False
    )
    carried_checked, mode_unverified, symlinks_hashed, link_files_seen = check_files(
        root, manifest, index, modified, link_files_allowed)
    check_not_ignored(root, manifest, tracked)
    seen = check_untracked(root, manifest, tracked, others)
    deviations, stored, on_disk_only, stored_mode_unverified = check_deviations(
        root, manifest, index, modified)
    inputs = check_inputs(root, manifest, index)
    if tracked is None:
        # Everything answerable has been answered above, so a real violation is reported as a failure
        # with its paths; what is left over is verification this host cannot perform at all. Returning
        # zero here -- the shape this script used to have -- let the required gate step print an
        # unqualified PASS for a tree whose carried files were never even enumerated, and a disclosure
        # inside a PASS line is not something a required step can reject.
        raise Incomplete(
            "git could not enumerate this tree, so the checks that need a file list did not run",
            [
                "nothing git would add sits inside the tree unexplained: an unrecorded file inside "
                "the carried tree would not be listed, so it could not be caught",
                "no carried path matches this tree's ignore rules: a carried path git would skip on "
                "add would not be caught",
                "the generated build input is what git stores for it: with no index to read, the "
                "ignored snapshot's presence, mode and stored bytes were not checked",
            ],
        )

    return [
        f"pin {manifest.header['pin']} ({manifest.header.get('describe', '')})",
        f"reported by cmd/ccc: {reported_pin}",
        f"carried: {carried_checked} file(s) identical to the pinned revision"
        + ("" if index is None else " (compared through git's own object store)"),
        f"deviations: {len(manifest.skips)} upstream path(s) declared "
        f"({len(stored)} of them carried with the bytes this tree stores)"
        f" + {len(manifest.inputs)} generated input(s), all present as declared",
        f"owned: {len(manifest.owned)} project-owned prefix(es); files seen by git: {seen}",
        f"inputs: {'; '.join(inputs)}",
    ] + (
        [f"symlinks hashed by their link target: {symlinks_hashed}"] if symlinks_hashed else []
    ) + (
        [
            f"carried link(s) held as a file with the link's target text: {link_files_seen} "
            f"(this checkout does not create symlinks)"
        ]
        if link_files_seen else []
    ) + (
        [
            f"executable bits not verified for {mode_unverified + stored_mode_unverified} file(s): "
            f"this platform has no POSIX mode bits"
        ]
        if mode_unverified + stored_mode_unverified else []
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
    except Incomplete as incomplete:
        # Exit 1 would say "your tree violates parity" about a tree nobody looked at closely enough to
        # say that; exit 0 would say the opposite. The gate rejects any non-zero outcome of this
        # required step, so 2 is reported as its own verdict -- the tree is not verified.
        print(f"parity: INCOMPLETE — {incomplete.summary}")
        for skipped in incomplete.skipped:
            print(f"  not checked: {skipped}")
        print("  the gate's parity step is required and rejects this: an unverified tree is not a pass")
        return 2
    if not args.quiet:
        for line in lines:
            print(line)
    return 0


if __name__ == "__main__":
    sys.exit(main())
