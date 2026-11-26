"""The host's own filesystem answers, measured once, and the fixtures cases build from them.

The suite runs on POSIX hosts and on ``windows-latest``, and the two kernels answer a case's questions
differently: POSIX resolves ``link/..`` through the link where win32 removes the component lexically
first, POSIX's exclusive create refuses a dangling link where win32 follows it and creates the target,
and POSIX enforces mode bits where the Windows filesystem keeps none. A case that asserts either answer
against whichever host it happens to run on passes here and reddens one CI leg at a time -- the failure
class the rounds before this module's head were each one instance of. This module measures those answers
at import, once, in a scratch directory of its own that is disposed of immediately, and offers the
canonical constructors for the fixtures that depend on them. Its sanity checks make a broken probe a
loud failure instead of a silent downgrade of the other platform's coverage.

Path questions that are *pure syntax* -- separator spelling, a drive-less root, absoluteness -- are not
host facts and are not answered here: a case drives the dialects themselves (``posixpath``/
``PurePosixPath`` and ``ntpath``/``PureWindowsPath``), and ``scripts/tests/test_host_contract.py``'s
lint fails a test that announces a path premise by hand instead: a separator-built fixture, a raw
symlink, an errno subclass on a create collision, or a PATH built from a literal ``":"``.
"""

from __future__ import annotations

import dataclasses
import ntpath
import os
import pathlib
import posixpath
import tempfile


@dataclasses.dataclass(frozen=True)
class HostSemantics:
    """What the host's own kernel answered in ``_probe``; every field measured, none inferred.

    ``abs_anchor`` -- the drive-qualified root of the process's own directory (``/`` on POSIX,
    ``C:\\`` on win32). ``fs_case_sensitive`` -- whether ``A`` and ``a`` name one file or two here.
    ``can_symlink`` -- whether ``os.symlink`` succeeded in the probe's scratch directory.
    ``symlink_dotdot_follows_link`` -- whether ``link/..`` follows the link (POSIX) or the component
    is removed before the link is followed (win32). ``o_excl_follows_dangling_symlink`` -- whether an
    exclusive create at a dangling link follows it and creates the target (win32) or refuses (POSIX).
    ``posix_exec_bits_enforced`` -- whether a mode change controls ``os.access(..., X_OK)``.
    """

    abs_anchor: str
    fs_case_sensitive: bool
    can_symlink: bool
    symlink_dotdot_follows_link: bool
    o_excl_follows_dangling_symlink: bool
    posix_exec_bits_enforced: bool


def _lexical_dotdot_platform() -> bool:
    """Whether this platform removes a ``..`` component lexically before a link is followed (win32)."""
    return os.name == "nt"


def _probe_case_sensitivity(scratch: pathlib.Path) -> bool:
    lower = scratch / "fs-case-probe"
    upper = scratch / "FS-CASE-PROBE"
    lower.write_text("lower", encoding="utf-8")
    return not (upper.exists() and upper.read_text(encoding="utf-8") == "lower")


def _probe_links(scratch: pathlib.Path) -> tuple[bool, bool, bool]:
    """``(can symlink, link/.. follows the link, O_EXCL follows a dangling link)``, each measured."""
    root = scratch / "links"
    (root / "real" / "sub").mkdir(parents=True)
    (root / "real" / "codex").write_text("the executable's own bytes\n", encoding="utf-8")
    (root / "codex").write_text("the file the lexical collapse names\n", encoding="utf-8")
    dangling = scratch / "excl-link"
    try:
        os.symlink(root / "real" / "sub", root / "alias", target_is_directory=True)
        os.symlink(str(scratch / "excl-target"), dangling, target_is_directory=False)
    except (OSError, NotImplementedError):
        follows = not _lexical_dotdot_platform()
        return False, follows, not follows
    follows = (root / "alias" / ".." / "codex").read_bytes() == (root / "real" / "codex").read_bytes()
    try:
        descriptor = os.open(dangling, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
    except FileExistsError:
        o_excl_follows = False
    else:
        os.close(descriptor)
        o_excl_follows = True
    return True, follows, o_excl_follows


def _probe_exec_bits(scratch: pathlib.Path) -> bool:
    # win32 answers X_OK for any existing file, so a one-sided ask cannot tell enforcement from its
    # absence (measured against CPython 3.13's os_access_impl); clearing the bits and setting them
    # again can.
    probe = scratch / "exec-bits-probe"
    probe.write_text("#!/bin/sh\n", encoding="utf-8")
    os.chmod(probe, 0o644)
    cleared = not os.access(probe, os.X_OK)
    os.chmod(probe, 0o755)
    return cleared and os.access(probe, os.X_OK)


def _assert_sane(semantics: HostSemantics) -> None:
    """The probe's verdicts against the two platforms' documented rules: a broken probe is noise."""
    if os.name == "posix":
        assert semantics.symlink_dotdot_follows_link, (
            "POSIX resolves `link/..` through the link; the probe answered otherwise and is broken")
        assert semantics.posix_exec_bits_enforced, (
            "POSIX enforces its mode bits; the probe answered otherwise and is broken")
    if os.name == "nt":
        assert not semantics.symlink_dotdot_follows_link, (
            "win32 removes a `..` component lexically before the reparse point is followed; the "
            "probe answered otherwise and is broken")


def _probe() -> HostSemantics:
    with tempfile.TemporaryDirectory(prefix="ccc-host-contract-") as scratch_text:
        scratch = pathlib.Path(scratch_text)
        can_symlink, follows_dotdot, excl_follows = _probe_links(scratch)
        semantics = HostSemantics(
            abs_anchor=os.path.splitdrive(os.getcwd())[0] + os.sep,
            fs_case_sensitive=_probe_case_sensitivity(scratch),
            can_symlink=can_symlink,
            symlink_dotdot_follows_link=follows_dotdot,
            o_excl_follows_dangling_symlink=excl_follows,
            posix_exec_bits_enforced=_probe_exec_bits(scratch),
        )
    _assert_sane(semantics)
    return semantics


HOST_SEMANTICS = _probe()

#: The dialect anchors ``make_abs_path`` builds under when it is asked for a dialect this host is not.
_DIALECT_ANCHORS = {posixpath: "/", ntpath: "C:\\"}


def make_abs_path(*parts: str, pathmod=os.path) -> str:
    """A fully qualified path under the dialect's own anchor: ``/...``, ``C:\\...``, or this host's.

    A case builds an absolute fixture through this, never by joining a bare separator: on win32 a
    lone-separator join is a *drive-less rooted* path, which the platform's own ``isabs`` has not
    called absolute since CPython 3.13, so such a fixture's premise would hold on POSIX alone.
    """
    if pathmod is os.path:
        return pathmod.join(HOST_SEMANTICS.abs_anchor, *parts)
    return pathmod.join(_DIALECT_ANCHORS[pathmod], *parts)


def make_rooted_path(*parts: str, pathmod=os.path) -> str:
    """The dialect's root-anchored spelling, *without* a drive: absolute on POSIX, rooted on win32.

    The shape's absoluteness is the platform's own answer, so a case that builds one asserts that
    predicate's answer (``pathmod.isabs``, ``PureWindowsPath.is_absolute``) and never treats it as an
    absolute fixture.
    """
    return pathmod.join(pathmod.sep, *parts)


def make_symlink(target, link, *, is_dir: bool) -> bool:
    """Create ``link`` pointing at ``target`` where this host can; answer whether it now exists.

    ``target_is_directory`` is the argument a bare ``os.symlink`` omits: on win32 a link to a
    directory -- or to a target that does not exist yet -- made without it is a *file* reparse point,
    which the platform then resolves as one. A case that stages a link goes through this and skips
    when the host cannot make one.
    """
    try:
        os.symlink(os.fspath(target), os.fspath(link), target_is_directory=is_dir)
    except (OSError, NotImplementedError):
        return False
    assert os.path.islink(link), f"the host answered that it linked {link}, and nothing links there"
    return True


def make_executable_stub(directory, name: str, *, exit_code: int = 0) -> pathlib.Path:
    """A stub this host's own tool lookup resolves as ``name`` on ``PATH``.

    Executability is the platform's own: POSIX mode bits on one (``name``, ``0o755``), a suffix its
    ``shutil.which`` resolves through ``PATHEXT`` on the other (``name.bat``). No case in this suite
    executes a stub -- every case that needs a tool to answer patches the runner seam instead -- so
    the body is the exit code alone.
    """
    directory = pathlib.Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    if os.name == "nt":
        stub = directory / f"{name}.bat"
        stub.write_text(f"@exit /b {exit_code}\r\n", encoding="utf-8")
    else:
        stub = directory / name
        stub.write_text(f"#!/bin/sh\nexit {exit_code}\n", encoding="utf-8")
        stub.chmod(0o755)
    return stub


def assert_create_refused(plant, target, *, dangling_dest=None) -> None:
    """The product's collision refusal at ``target``, and the state the refusal must leave behind.

    ``plant`` is the product's own create-under-a-held-name call, answering ``(False, "")`` for a
    collision; ``dangling_dest`` is what a dangling link at ``target`` points at, which the refusal
    must leave uncreated. The platform's errno subclass is deliberately not pinned: POSIX answers
    ``FileExistsError`` and win32's exclusive create answers differently for the same held name, and
    neither subclass is the product's contract. A case that deliberately *injects* an errno through a
    seam is a different subject and is not routed through this.
    """
    planted, failure = plant(target, b"a refused create must not write these bytes\n")
    if (planted, failure) != (False, ""):
        raise AssertionError(
            f"the create under {target} did not answer as the product's collision refusal: "
            f"{(planted, failure)!r}")
    if not os.path.lexists(target):
        raise AssertionError(f"the refusal did not leave {target} as it found it")
    if dangling_dest is not None and os.path.exists(dangling_dest):
        raise AssertionError(f"the refusal created the dangling link's target {dangling_dest}")
