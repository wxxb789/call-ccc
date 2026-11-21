#!/usr/bin/env python3
"""Regression tests for scripts/upstream-parity.py and scripts/upstream-import.py.

These two scripts are the only thing standing between "we carry AgentsView unchanged" and a claim
nobody checked, so their rules get tests like the gate's do: a carried file that drifts, one that goes
missing, one whose type changes, a generated input whose hash moves, a manifest that stops matching
the revision the CLI reports, and a declaration that has quietly become stale all have to fail, and a
clean tree has to pass.

Most cases build a throw-away tree and drive the shipped module in-process (the scripts take
``--root``), because a parity run must answer the same on a machine whose ``PATH`` holds no tools:
the two rules that need git are pinned by patching the module's own seam rather than by shelling out,
which is also how the suite pins the gate's ``run()``. Every case holds on every platform and every
Python 3 the repository documents.
"""

from __future__ import annotations

import hashlib
import importlib.util
import io
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path, PureWindowsPath
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]
REPO_ROOT = SCRIPTS.parent
SCRIPT_DIRS = (str(SCRIPTS),)

#: A revision-shaped pin; the exact digits do not matter to any rule under test.
PIN = "0123456789abcdef0123456789abcdef01234567"
REVISION = "fedcba9876543210fedcba9876543210fedcba98"
SNAPSHOT = "internal/pricing/snapshot/litellm_snapshot.json.gz"
TOOL = "internal/pricing/cmd/litellm-snapshot/main.go"
MANIFEST = "scripts/upstream-manifest.txt"


def platform_scratch_bases(platform: str) -> list[Path]:
    """The scratch roots ``platform`` itself offers, for a host whose env answer is unusable.

    ``tempfile.gettempdir()`` answers from ``TMPDIR``/``TEMP``/``TMP`` and caches the first answer in
    ``tempfile.tempdir``, so a case that poisons that attribute -- the two cases at the bottom of this
    module do exactly that, to pin the leak rule -- takes the environment's answer away from the whole
    process and nothing else is left. POSIX names its own two scratch roots (``/tmp``, ``/var/tmp``);
    Windows has no scratch root of its own to name (``Path("/tmp")`` is ``\\tmp`` on whichever drive
    the process happens to be on, so a list of those finds nothing there and this helper could not
    produce a tree at all), and its answer is the variables its own temp answer is made of, read again
    here. ``scripts/portability.py``'s ``platform_scratch_bases`` is the shipped half of this same
    rule (it reads ``os.name`` through a ``platform_name`` seam, the shape shipped code uses); this
    copy is the suite's, and takes the answer as a parameter so a case can pin both on either host --
    ``os.name`` itself cannot be patched for that on a POSIX host, because ``pathlib`` reads it when a
    ``Path`` is constructed and every ``Path`` call in the case would then fail.
    """
    if platform == "nt":
        return [Path(os.environ[name]) for name in ("TEMP", "TMP") if os.environ.get(name)]
    return [Path("/tmp"), Path("/var/tmp")]


def scratch_tree(prefix: str) -> Path:
    """A throw-away tree for one case, always absolute and always outside the repository.

    ``tempfile.gettempdir()`` answers whatever ``TMPDIR`` says, and a *relative* ``TMPDIR`` -- or one
    that points inside the repository -- would put the tree under the process's current directory
    (the repository root when the gate runs this suite), where a killed run leaves it behind: 81 empty
    ``parity-root-*`` directories were found in the repository root on 2026-11-21 for exactly that
    reason. The base is therefore resolved first, a base inside the repository is refused, and the
    platform's own scratch roots (:func:`platform_scratch_bases`) are the fallback; the patterns are
    named in the repository's own ``.gitignore`` as the second line of defence -- a stray tree must
    never be something ``git status`` offers to commit.
    """
    base = Path(tempfile.gettempdir())
    if not base.is_absolute():
        base = Path(os.path.abspath(base))
    attempts: list[str] = []
    # The environment's own answer is tried as it stands (that is what ``tempfile`` would use); the
    # platform's fallback roots are offered only where one is usable at all.
    candidates: list[Path] = [base]
    candidates.extend(root for root in platform_scratch_bases(os.name)
                      if root.is_dir() and root != base and root != REPO_ROOT
                      and REPO_ROOT not in root.parents)
    for candidate_base in candidates:
        try:
            candidate = Path(tempfile.mkdtemp(prefix=prefix, dir=str(candidate_base))).resolve()
        except OSError as exc:
            attempts.append(f"{candidate_base}: {exc}")
            continue
        if candidate != REPO_ROOT and REPO_ROOT not in candidate.parents:
            return candidate
        shutil.rmtree(candidate, ignore_errors=True)
        attempts.append(f"{candidate_base}: inside {REPO_ROOT}")
    raise AssertionError(
        f"no scratch directory outside {REPO_ROOT} is available ({'; '.join(attempts)}), and a tree "
        f"inside the repository is what this helper exists to prevent")


def tool_result(cmd, rc=0, out="", err=""):
    """A ``CompletedProcess`` shaped like the ones the shipped runners return."""
    return subprocess.CompletedProcess(cmd, rc, out, err)


def load_script(name: str):
    """Import a shipped script by path, with scripts/ on sys.path for its own imports."""
    for directory in SCRIPT_DIRS:
        if directory not in sys.path:
            sys.path.insert(0, directory)
    spec = importlib.util.spec_from_file_location(f"shipped_{name.replace('.', '_').replace('-', '_')}",
                                                  SCRIPTS / name)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def blob_sha1(data: bytes) -> str:
    """git's own object name, so a case can build a manifest without a git binary."""
    return hashlib.sha1(b"blob %d\x00" % len(data) + data).hexdigest()


class ParityTree:
    """One throw-away tree carrying a tiny, valid manifest, plus helpers to break it."""

    def __init__(self) -> None:
        self.root = scratch_tree("parity-root-")
        self.manifest_module = load_script("upstream_manifest.py")
        self.parity = load_script("upstream-parity.py")
        self.import_script = load_script("upstream-import.py")
        self.carried: dict[str, tuple[str, bytes]] = {
            "src/a.txt": ("100644", b"alpha\n"),
            "src/b.sh": ("100755", b"#!/bin/sh\necho beta\n"),
            "docs/note.md": ("100644", b"note\n"),
        }
        # A deviation records the *upstream* blob and holds this tree's own bytes: the two differing is
        # what makes the declaration meaningful. A third element is a declared normalisation -- bytes
        # this tree stores for a path it carries anyway -- and the record then pins those bytes beside
        # upstream's blob, because upstream's digest can never be what the tree holds.
        self.deviations = {"README.md": ("100644", b"upstream readme\n")}
        self.snapshot_bytes = b"snapshot-bytes"
        for path, (mode, data) in self.carried.items():
            self.write_bytes(path, data)
            self.apply_executable_bit(path, mode)
        for path in self.deviations:
            self.write_bytes(path, b"this tree's own file\n")
        self.write_bytes(SNAPSHOT, self.snapshot_bytes)
        self.write(TOOL, self.tool_source(hashlib.sha256(self.snapshot_bytes).hexdigest()))
        self.write("cmd/ccc/pin.go", f'package main\n\nconst upstreamRevision = "{PIN}"\n')
        self.write("docs/upstream.md", f"# Upstream\n\n| Pinned revision | `{PIN}` |\n")
        self.write_manifest()

    def cleanup(self) -> None:
        shutil.rmtree(self.root, ignore_errors=True)

    def tool_source(self, digest: str) -> str:
        return f'package main\n\nconst defaultSnapshotSHA256 = "{digest}"\n'

    def write(self, relative: str, text: str) -> None:
        self.write_bytes(relative, text.encode("utf-8"))

    def write_bytes(self, relative: str, data: bytes) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def remove(self, relative: str) -> None:
        (self.root / relative).unlink()

    def apply_executable_bit(self, relative: str, mode: str) -> None:
        """Match the file's mode to the manifest's, where the platform has POSIX mode bits."""
        if mode == "100755" and os.name != "nt":
            path = self.root / relative
            path.chmod(path.stat().st_mode | 0o111)

    def write_manifest(self) -> None:
        lines = [
            "# generated",
            f"pin {PIN}",
            "remote https://example.invalid/agentsview",
            "describe v0-test",
            "commit-date 2026-01-01T00:00:00+00:00",
            f"upstream-files {len(self.carried) + len(self.deviations)}",
            f"carried {len(self.carried)}",
            f"deviations {len(self.deviations) + 1}",
            "owned\tproject/",
        ]
        for path, (mode, data) in sorted(self.carried.items()):
            lines.append(f"file\t{mode}\t{blob_sha1(data)}\t{len(data)}\t{path}")
        for path, deviation in sorted(self.deviations.items()):
            mode, upstream = deviation[0], deviation[1]
            line = (f"skip\t{mode}\t{blob_sha1(upstream)}\t{len(upstream)}\t{path}"
                    f"\tproject-owned: this tree keeps its own file here")
            if len(deviation) > 2:
                stored = deviation[2]
                line += f"\t{blob_sha1(stored)}\t{len(stored)}"
            lines.append(line)
        digest = hashlib.sha256(self.snapshot_bytes).hexdigest()
        lines.append(f"input\t100644\t{digest}\t{len(self.snapshot_bytes)}\t{SNAPSHOT}\tgenerated build input")
        (self.root / MANIFEST).parent.mkdir(parents=True, exist_ok=True)
        (self.root / MANIFEST).write_text("\n".join(lines) + "\n", encoding="utf-8")

    def add_link_record(self, path: str, target: bytes) -> None:
        """Record a carried symlink at path, whose bytes are the link's target text.

        The caller writes the shape the case is about: the link itself, or a regular file holding the
        target text (which is what a checkout that does not create symlinks has at that path).
        """
        self.carried[path] = ("120000", target)
        self.write_manifest()

    def link_shape_runs(self, path: str, target: bytes):
        """Each way this host can hold ``target`` at ``path`` as a link, as a lazy ``(name, patches)``.

        The link rules -- the shape a ``120000`` record's copy must have, and the separator spelling
        this platform's symlink layer stores a link's target in -- are rules of the *link*, and a host
        whose checkouts cannot create one must still test them: the file form holds exactly the bytes an
        ``os.readlink`` of the link would answer (git stores a link as its target text), so the first
        shape writes that file and tells the module's ``symlink_target`` seam what target the link
        holds. Where this host can create a real symlink, that shape follows, so the seam is exercised
        against the real thing too. A case asserts the same rules for every shape it is given.

        A generator because the two shapes replace each other in one tree, and each is written only when
        the case asks for it: run the returned patches before asking for the next shape. The file
        *form*'s own rejection is a different rule with its own cases; these patches say "this path is
        the link" and must not be used for it.
        """
        self.write_bytes(path, target)
        yield "file form, with the link seam answering the target it holds", {
            "symlink_target": self.link_seam({path: target.decode("utf-8")})}
        link = self.root / path
        link.unlink()
        try:
            os.symlink(target.decode("utf-8"), link)
        except (OSError, NotImplementedError):
            # This host cannot create a link: the file form above is the whole of what it can hold, and
            # the case's seam answers the target that link holds.
            self.write_bytes(path, target)
            return
        yield "this host's own symlink", {}

    def link_seam(self, targets: dict[str, str]):
        """A link-reading seam answering ``targets``, deferring to the filesystem for everything else.

        The filesystem's own answer is read once, before the seam is patched in, so deferring cannot
        recurse into this seam.
        """
        filesystem = self.parity.symlink_target

        def seam(root: Path, path: str) -> str | None:
            if path in targets:
                return targets[path]
            return filesystem(root, path)

        return seam

    def add_stored_deviation(self, path: str, upstream: bytes, stored: bytes,
                             mode: str = "100644") -> None:
        """Declare a path this tree carries anyway, pinning the bytes it stores for it.

        The tree is written with the stored bytes, which is what the importer does: upstream's blob is
        what the record names, and the two can differ only by the declared normalisation.
        """
        self.deviations[path] = (mode, upstream, stored)
        self.write_bytes(path, stored)
        self.write_manifest()

    def git_known_paths(self) -> list[str]:
        """The file list a scripted git answers for this throw-away tree: the paths the manifest names.

        The tree is not a repository, so every git answer a case needs is scripted. This list is the
        manifest's own paths: the tree's remaining files (the pin constant, the carried tool, the
        upstream document) are scaffolding the checks read directly, and listing them would make each
        of them an unexplained file in a case that is about something else. A case that *is* about the
        file list, or about a tree nobody can enumerate, passes ``tree_files`` itself.
        """
        try:
            manifest = self.manifest_module.load_manifest(self.root / MANIFEST)
        except self.manifest_module.ManifestError:
            return []  # the case under test is the malformed manifest itself
        return sorted(record.path for record in manifest.records if record.kind != "owned")

    def index_state(self) -> dict[str, tuple[str, str]]:
        """What a scripted git stores for this tree: its index, keyed by path.

        A carried file's entry is the *pinned* blob its record names, so a case that breaks the file on
        disk is caught by the working-tree comparison rather than by an index that was built from the
        modified bytes; a declared deviation's entry is the blob of the file this tree ships, which is
        what an importer writes and what a clone checks out. The generated input is not a git object in
        the manifest's spelling (its record pins a SHA-256), so its entry is the blob its own bytes hash
        to -- the same value the check computes and compares against the index.
        """
        index: dict[str, tuple[str, str]] = {}
        try:
            manifest = self.manifest_module.load_manifest(self.root / MANIFEST)
        except self.manifest_module.ManifestError:
            return index
        for record in manifest.records:
            if record.kind == "owned":
                continue
            if record.kind == "file":
                index[record.path] = (record.mode, record.digest)
                continue
            if record.kind == "skip" and record.stored_digest:
                index[record.path] = (record.mode, record.stored_digest)
                continue
            path = self.root / record.path
            if path.is_file():
                index[record.path] = (record.mode, blob_sha1(path.read_bytes()))
        return index

    def scripted_git(self, **overrides):
        """A scripted repository whose answers describe this tree as a clean checkout of its manifest.

        The tracked list is the manifest's own paths, the index is ``index_state``, nothing is
        untracked, nothing is ignored and ``git status`` reports no modification: the shape a case about
        a *different* rule needs, so its subject cannot fall into the unenumerable-tree verdict or trip
        one of the probes that fails closed. A case that is about one of those answers overrides it.
        """
        return ScriptedGit(tracked=self.git_known_paths(), index=self.index_state(), **overrides)

    @staticmethod
    def gitless_answers() -> dict:
        """The seam answers of a tree git cannot enumerate: no file list, no index, no status.

        This is one coherent state, not four: ``tracked`` is ``None``, so the caller's INCOMPLETE verdict
        covers every rule that needs the list, and the checks that answer from the tree's own bytes still
        run -- and still fail on real drift, which a case below pins.
        """
        return {
            "tree_files": lambda root: (None, None),
            "git_index": lambda root: None,
            "git_modified": lambda root: None,
            "ignored_paths": lambda root, paths: [],
        }

    def run_parity(self, **patches) -> tuple[int, str]:
        """Run the shipped check against this tree, with optional module seams patched.

        The seams are the answers a git-backed run takes from git: the file list, the index, what
        ``git status`` reports, and the ignore answer. Each defaults to this tree's own clean scripted
        answer -- the manifest's paths, an index holding the manifest's bytes, nothing modified and
        nothing ignored -- so that a case about some other rule runs the whole check instead of falling
        into the unenumerable-tree verdict, and a case that *is* about one of those answers passes that
        seam itself. A case that wants the git-less branch passes ``gitless_answers()``, whose four
        answers are the one coherent state a tree without git has.
        """
        if "subprocess" not in patches:
            patches.setdefault("tree_files", lambda root: (self.git_known_paths(), []))
            patches.setdefault("git_index", lambda root: self.index_state())
            patches.setdefault("git_modified", lambda root: set())
            patches.setdefault("ignored_paths", lambda root, paths: [])
            #: ``core.symlinks`` unset, with git answering: the platform's own default decides how a
            #: link may be held, and no case shells out to a tool to hear it.
            patches.setdefault("git_config_bool", lambda root, key: (None, True))
        buffer = io.StringIO()
        with mock.patch.multiple(self.parity, **patches) if patches else _no_patch():
            with redirect_stdout(buffer):
                code = self.parity.main(["--root", str(self.root)])
        return code, buffer.getvalue()

    def break_carried(self, relative: str, data: bytes) -> None:
        self.write_bytes(relative, data)
        mode = self.carried[relative][0]
        self.apply_executable_bit(relative, mode)


class _no_patch:
    def __enter__(self) -> None: return None

    def __exit__(self, *exc) -> bool: return False


class ParityTreeCase:
    """The throw-away tree, its helpers and the two verdict assertions the case classes share.

    Deliberately not a ``unittest.TestCase``: a case class that inherited the *cases* would have every
    one of them discovered twice -- once on the base, once on the subclass -- and a subclass that
    means to exercise one branch would re-run all the other branches' cases too.
    """

    def setUp(self) -> None:
        self.tree = ParityTree()
        self.addCleanup(self.tree.cleanup)

    def assert_clean(self, code: int, output: str) -> None:
        self.assertEqual(code, 0, output)
        self.assertIn("reported by cmd/ccc/pin.go: " + PIN, output)
        self.assertIn(f"carried: {len(self.tree.carried)} file(s) identical to the pinned revision",
                      output)

    def assert_failed(self, code: int, output: str, needle: str) -> None:
        self.assertEqual(code, 1, output)
        self.assertIn("parity: FAIL", output)
        self.assertIn(needle, output)

    def assert_incomplete(self, code: int, output: str) -> None:
        """A tree git could not enumerate is not verified: its own verdict, and never a pass."""
        self.assertEqual(code, 2, output)
        self.assertIn("parity: INCOMPLETE", output)
        self.assertNotIn("parity: FAIL", output)


class ParityCase(ParityTreeCase, unittest.TestCase):
    """Cases that assert the shipped check's verdicts."""

    def test_a_clean_tree_passes(self) -> None:
        self.assert_clean(*self.tree.run_parity())

    def test_the_pin_source_spelling_is_posix_on_every_platform(self) -> None:
        """A tree path this check names is spelled with forward slashes, whatever the host spells.

        The check prints the pin constant's own path (``reported by …``) and names it in every
        message about it; the manifest's record paths, the layout's lines and the docs all spell tree
        paths POSIX-form, and the migration that gave the line its layout prefix spelled it with a
        host ``str()`` instead -- ``cmd\\ccc\\pin.go`` on Windows alone, in the one job nobody can
        re-run on demand. On *this* host a ``str()`` and an ``as_posix()`` agree, so the shipped
        helper is driven with the path *values* a Windows host hands it (there both operands are
        Windows-flavoured): the same simulation the gate's suite runs for its own walk, and the one
        that makes the difference visible here rather than only in the Windows lane.
        """
        parity = self.tree.parity
        manifests = self.tree.manifest_module
        for header, want in (("", "cmd/ccc/pin.go"),
                             ("layout agentsview/ ccc\n", "ccc/cmd/ccc/pin.go")):
            with self.subTest(layout=want):
                manifest = manifests.parse_manifest(f"pin {PIN}\n{header}")
                self.assertEqual(parity.pin_source_path(manifest), want, header)
                with mock.patch.multiple(parity, Path=PureWindowsPath,
                                         PIN_SOURCE=PureWindowsPath("cmd", "ccc", "pin.go")):
                    self.assertEqual(parity.pin_source_path(manifest), want, header)

    def test_a_modified_carried_file_is_named(self) -> None:
        self.tree.break_carried("src/a.txt", b"alpha changed\n")
        code, output = self.tree.run_parity()
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("differ from the pinned revision", output)

    def test_a_byte_appended_to_a_carried_file_is_caught(self) -> None:
        self.tree.break_carried("docs/note.md", b"note\n\n")
        self.assert_failed(*self.tree.run_parity(), "docs/note.md")

    def test_a_missing_carried_file_is_named(self) -> None:
        (self.tree.root / "docs/note.md").unlink()
        self.assert_failed(*self.tree.run_parity(), "absent")

    def test_a_carried_file_that_became_a_directory_is_a_failure(self) -> None:
        path = self.tree.root / "docs/note.md"
        path.unlink()
        path.mkdir()
        self.assert_failed(*self.tree.run_parity(), "changed file type")

    def test_a_symlink_record_hashes_the_link_target(self) -> None:
        """git stores a link as its target text, so the same bytes in a file are the same object.

        Every shape this host can hold a link in is run: the file form the seam declares the link for,
        and the host's own real symlink where it can create one, so the rule holds on a host that
        cannot -- and neither shape is what the *file* cases below put at such a path, which is a file
        the run must refuse.
        """
        target = b"a.txt"
        self.tree.add_link_record("src/link", target)
        for name, patches in self.tree.link_shape_runs("src/link", target):
            with self.subTest(shape=name):
                code, output = self.tree.run_parity(**patches)
                self.assert_clean(code, output)
                self.assertIn("symlinks hashed by their link target: 1", output)
                self.assertNotIn("carried link(s) held as a file", output)

    def test_a_link_held_as_a_file_is_a_shape_failure_where_symlinks_are_created(self) -> None:
        """The link's bytes are also a file's bytes, so only the shape can tell the two apart.

        A regular file holding the link's target text is exactly what a checkout that does not create
        symlinks has, and it is *not* what a symlink-enabled checkout has: there the pin's link is a
        link, and this replacement keeps the recorded blob (its bytes are the target text), can keep
        the pinned ``120000`` mode in git's index while a ``skip-worktree`` flag silences
        ``git status`` -- the case below pins that through the git branch -- so a check that accepted
        any file here would certify a carried path whose working-tree type had changed.
        """
        self.tree.write_bytes("src/link", b"a.txt")
        self.tree.add_link_record("src/link", b"a.txt")
        code, output = self.tree.run_parity(git_config_bool=lambda root, key: (True, True))
        self.assert_failed(code, output, "changed file type")
        self.assertIn("src/link", output)

    def test_a_link_held_as_a_file_passes_where_the_checkout_stores_links_that_way(self) -> None:
        """``core.symlinks=false`` is git saying the file form *is* this checkout's representation.

        The allowance has to stay where it applies -- Git for Windows and a checkout made with
        ``core.symlinks=false`` hold exactly this file -- and the run says the shape it accepted instead
        of accepting it silently. The control for the case above: without this, a check that refused
        every file at a link record would pass the shape rule for the wrong reason.
        """
        self.tree.write_bytes("src/link", b"a.txt")
        self.tree.add_link_record("src/link", b"a.txt")
        code, output = self.tree.run_parity(git_config_bool=lambda root, key: (False, True))
        self.assert_clean(code, output)
        self.assertIn("carried link(s) held as a file with the link's target text: 1", output)

    def test_the_platform_answer_decides_a_link_file_form_when_git_cannot_be_asked(self) -> None:
        """With no git at all, the platform decides -- and only a platform that cannot link says yes.

        A tree git cannot enumerate still has the shape rule answered from the working tree, so this is
        where a checkout that cannot create symlinks (and cannot say so through git, having none) is
        read as holding the link's target in a file, while a host whose checkouts do create links reads
        the same file as the wrong shape.
        """
        self.tree.write_bytes("src/link", b"a.txt")
        self.tree.add_link_record("src/link", b"a.txt")
        code, output = self.tree.run_parity(
            symlinks_unavailable=lambda: True, **self.tree.gitless_answers())
        # The shape rule passed (the verdict is the missing file list, not a failed comparison); the
        # disclosure of the shape that was accepted belongs to the git-backed run above, because an
        # INCOMPLETE verdict prints what was *not* checked instead of the run's own summary lines.
        self.assert_incomplete(code, output)
        code, output = self.tree.run_parity(
            symlinks_unavailable=lambda: False, **self.tree.gitless_answers())
        self.assert_failed(code, output, "changed file type")

    def test_a_symlink_configuration_query_that_cannot_answer_fails_the_run(self) -> None:
        """A probe that could not answer must not read as the platform default: that would decide a rule."""
        self.tree.write_bytes("src/link", b"a.txt")
        self.tree.add_link_record("src/link", b"a.txt")
        code, output = self.tree.run_parity(git_config_bool=lambda root, key: (None, False))
        self.assert_failed(code, output, "could not ask git whether this checkout creates symlinks")

    def test_the_platform_answer_a_link_file_form_is_read_from(self) -> None:
        """Both answers of the platform probe, without depending on the host the case runs on.

        The probe is what decides a link's file form when git cannot be asked, and it has two answers:
        a checkout that cannot create symlinks holds the target text in a file, and one that can does
        not. Asserting the answer the *host* gives would pin only one of them, so the platform name is
        answered by the case instead.
        """
        parity = self.tree.parity
        with mock.patch.object(parity, "platform_name", lambda: "nt"):
            self.assertTrue(parity.symlinks_unavailable())
        with mock.patch.object(parity, "platform_name", lambda: "posix"):
            self.assertFalse(parity.symlinks_unavailable())

    def test_a_link_target_in_the_platforms_own_spelling_is_matched(self) -> None:
        """A platform that re-spells a link's target is compared in that spelling, and says so.

        Git for Windows cannot create a link whose target holds ``/``: ``mingw_create_symlink``
        rewrites every separator to ``\\``, so the link this checkout created for the pin's
        ``../../.agents/skills/x`` reads back as ``..\\..\\.agents\\skills\\x`` -- the same
        destination, hashing to no blob the manifest holds. Reading that as drift would fail a correct
        checkout on that platform alone; the fold is the whole allowance, and the run names the links
        it needed it for. Both link shapes this host can hold are run, so a host whose checkouts cannot
        create a symlink tests the fold too -- and a *file* at this path is a different rule, whose own
        cases below pin that the fold is not applied to it.
        """
        pinned, respelled = b"../../.agents/skills/x", b"..\\..\\.agents\\skills\\x"
        self.tree.add_link_record("src/link", pinned)
        for name, patches in self.tree.link_shape_runs("src/link", respelled):
            with self.subTest(shape=name):
                code, output = self.tree.run_parity(platform_name=lambda: "nt", **patches)
                self.assert_clean(code, output)
                self.assertIn("matched in this platform's own spelling: 1", output)

    def test_a_respelled_link_git_reports_changed_is_not_a_second_failure(self) -> None:
        """git on such a host spells that link the same way, so its own comparison decides nothing new.

        ``git status`` compares a symlink by its target's bytes, and on this platform the target the
        checkout *created* is the pin's target in that spelling: git therefore reports a checkout the
        pin itself produces there as differing from its index, and failing on that would fail a correct
        tree for the platform's spelling a second time. The link's content is still verified -- against
        the pin, in that spelling -- and its index entry is still required to be the pinned mode and
        blob, so the rule below is what keeps this narrow: every other path reported as changed is a
        failure, as the case that follows asserts.
        """
        pinned, respelled = b"../../.agents/skills/x", b"..\\..\\.agents\\skills\\x"
        self.tree.add_link_record("src/link", pinned)
        for name, patches in self.tree.link_shape_runs("src/link", respelled):
            with self.subTest(shape=name):
                code, output = self.tree.run_parity(
                    platform_name=lambda: "nt", git_modified=lambda root: {"src/link"}, **patches)
                self.assert_clean(code, output)
                self.assertIn("matched in this platform's own spelling: 1", output)

    def test_a_carried_file_git_reports_changed_is_still_a_failure_on_that_platform(self) -> None:
        """The control for the case above: the exemption is the respelled link's, not the platform's.

        The file's bytes are the pin's, so only git's own answer claims a difference -- which is a
        failure for every path but a link whose target was verified in the platform's spelling.
        """
        code, output = self.tree.run_parity(
            platform_name=lambda: "nt", git_modified=lambda root: {"src/a.txt"})
        self.assert_failed(code, output, "differ from what git stores for them")
        self.assertIn("src/a.txt", output)

    def test_a_link_target_that_platform_does_not_respell_is_drift(self) -> None:
        """The control: the very same target text is drift where the platform writes ``/``.

        A POSIX link whose target holds a backslash names a file with a backslash in its name, not a
        path -- so folding there would accept a target the pin never had. The allowance is asked of
        the platform for exactly this reason.
        """
        pinned, respelled = b"../../.agents/skills/x", b"..\\..\\.agents\\skills\\x"
        self.tree.add_link_record("src/link", pinned)
        for name, patches in self.tree.link_shape_runs("src/link", respelled):
            with self.subTest(shape=name):
                code, output = self.tree.run_parity(platform_name=lambda: "posix", **patches)
                self.assert_failed(code, output, "differ from the pinned revision")
                self.assertIn("src/link", output)

    def test_a_link_target_that_changed_more_than_its_separators_is_drift(self) -> None:
        """Folding separators is the whole allowance: another destination is still drift.

        The link below is the platform's own spelling of a *different* path -- one component changed --
        which is what a link repointed by hand looks like on that platform, so it must fail there as
        well as anywhere else.
        """
        pinned, elsewhere = b"../../.agents/skills/x", b"..\\..\\.agents\\skills\\other"
        self.tree.add_link_record("src/link", pinned)
        for name, patches in self.tree.link_shape_runs("src/link", elsewhere):
            with self.subTest(shape=name):
                code, output = self.tree.run_parity(platform_name=lambda: "nt", **patches)
                self.assert_failed(code, output, "differ from the pinned revision")
                self.assertIn("src/link", output)

    def test_a_file_holding_a_links_respelled_target_is_not_the_link_where_the_host_can_tell(
        self,
    ) -> None:
        """A regular file at a link record is the wrong type on that platform too.

        Its bytes are the target text the platform would store -- folded and all -- which is what a
        check that folded the *shape* question away with the spelling would accept: a checkout that
        creates links creates one here, so a file at this path is a carried path whose working-tree
        type changed.
        """
        self.tree.add_link_record("src/link", b"../../.agents/skills/x")
        self.tree.write_bytes("src/link", b"..\\..\\.agents\\skills\\x")
        code, output = self.tree.run_parity(
            platform_name=lambda: "nt", git_config_bool=lambda root, key: (True, True))
        self.assert_failed(code, output, "changed file type")
        self.assertIn("src/link", output)

    def test_a_file_holding_a_links_respelled_target_is_drift_where_links_are_written_as_files(
        self,
    ) -> None:
        """The file form is compared byte for byte: git writes the pin's bytes into it, not a spelling.

        ``write_file_entry`` copies the blob's contents into the file, so a backslash in a file at a
        ``120000`` record is the file's own content and not a separator this platform spelled for the
        pin's target -- folding it there would accept a link record whose stored bytes changed.
        """
        self.tree.add_link_record("src/link", b"../../.agents/skills/x")
        self.tree.write_bytes("src/link", b"..\\..\\.agents\\skills\\x")
        code, output = self.tree.run_parity(
            platform_name=lambda: "nt", git_config_bool=lambda root, key: (False, True))
        self.assert_failed(code, output, "differ from the pinned revision")
        self.assertIn("src/link", output)

    def test_the_platform_answer_a_links_target_spelling_is_read_from(self) -> None:
        """Both answers of the separator seam, on either host, as the shipped check asks it."""
        parity = self.tree.parity
        with mock.patch.object(parity, "platform_name", lambda: "nt"):
            self.assertTrue(parity.separators_respelled())
        with mock.patch.object(parity, "platform_name", lambda: "posix"):
            self.assertFalse(parity.separators_respelled())

    def test_a_symlink_record_whose_path_is_a_directory_is_a_failure(self) -> None:
        """A link's bytes are its target text, so a third shape at the path is not the pin's link."""
        self.tree.carried["src/link"] = ("120000", b"a.txt")
        (self.tree.root / "src/link").mkdir()
        self.tree.write_manifest()
        code, output = self.tree.run_parity()
        self.assert_failed(code, output, "changed file type")
        self.assertIn("src/link", output)

    def test_an_unowned_extra_file_is_named(self) -> None:
        code, output = self.tree.run_parity(
            tree_files=lambda root: (["src/a.txt", "src/b.sh", "docs/note.md", "stray/thing.txt"], []))
        self.assert_failed(code, output, "stray/thing.txt")
        self.assertIn("neither carried nor project-owned", output)

    def test_an_owned_addition_is_allowed(self) -> None:
        self.tree.write_bytes("project/notes.md", b"ours\n")
        code, output = self.tree.run_parity(
            tree_files=lambda root: (["src/a.txt", "src/b.sh", "docs/note.md", "project/notes.md"], []))
        self.assert_clean(code, output)

    def test_an_untracked_but_present_file_is_checked_too(self) -> None:
        """A tree that has not been committed yet still has to explain every file it would add."""
        code, output = self.tree.run_parity(
            tree_files=lambda root: (["src/a.txt", "src/b.sh", "docs/note.md"], ["scratch.go"]))
        self.assert_failed(code, output, "scratch.go")

    def test_an_index_blob_that_differs_from_the_pin_is_a_failure(self) -> None:
        """What git stores is what a clone receives, so that is what is compared."""
        index = {
            "src/a.txt": ("100644", "0" * 40),
            "src/b.sh": ("100755", blob_sha1(self.tree.carried["src/b.sh"][1])),
            "docs/note.md": ("100644", blob_sha1(self.tree.carried["docs/note.md"][1])),
        }
        code, output = self.tree.run_parity(git_index=lambda root: index, git_modified=lambda root: set())
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("differ from the pinned revision", output)

    def test_an_index_mode_that_differs_from_the_pin_is_a_failure(self) -> None:
        index = {
            "src/a.txt": ("100755", blob_sha1(self.tree.carried["src/a.txt"][1])),
            "src/b.sh": ("100755", blob_sha1(self.tree.carried["src/b.sh"][1])),
            "docs/note.md": ("100644", blob_sha1(self.tree.carried["docs/note.md"][1])),
        }
        code, output = self.tree.run_parity(git_index=lambda root: index, git_modified=lambda root: set())
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("changed mode", output)

    def test_a_worktree_that_differs_from_the_index_is_a_failure(self) -> None:
        """Staged-but-cleaned files are not dirty; a working tree git would report as modified is."""
        index = {
            path: (mode, blob_sha1(data)) for path, (mode, data) in self.tree.carried.items()
        }
        code, output = self.tree.run_parity(
            git_index=lambda root: index, git_modified=lambda root: {"docs/note.md"})
        self.assert_failed(code, output, "differ from what git stores")
        self.assertIn("docs/note.md", output)

    def test_a_tree_git_cannot_enumerate_is_incomplete_not_a_pass(self) -> None:
        """A file list nobody could produce is not verification, so the run is INCOMPLETE.

        Both rules that need the list -- an unrecorded file inside the carried tree, and a carried
        path the ignore rules would skip -- pass silently without it. The gate's parity step is
        required and rejects any non-zero outcome, so this verdict can never be quoted as agreement
        with the pin; the script used to exit zero here and the gate used to print a bare PASS.
        """
        code, output = self.tree.run_parity(**self.tree.gitless_answers())
        self.assert_incomplete(code, output)
        self.assertIn("did not run", output)
        self.assertIn("unexplained", output)
        self.assertIn("ignore rules", output)

    def test_an_unanswerable_ignore_probe_fails_instead_of_passing(self) -> None:
        """``ignored_paths`` returning None means "could not ask", which must not read as "fine"."""
        code, output = self.tree.run_parity(ignored_paths=lambda root, paths: None)
        self.assert_failed(code, output, "could not ask git which carried paths are ignored")

    def test_an_index_probe_that_cannot_answer_fails_instead_of_passing(self) -> None:
        """The same rule for the index: a query that failed is not "nothing to verify".

        ``git ls-files -s`` failing while the ordinary file list succeeded used to be read as "this tree
        has no index" -- which skipped both what a fresh clone receives and ``git status``, and still
        reached a verdict. The rule is not conditional on which probe failed: an answer the check needs
        and did not get fails the run, and the message names the query.
        """
        code, output = self.tree.run_parity(git_index=lambda root: None)
        self.assert_failed(code, output, "could not read git's index")
        self.assertIn("git ls-files -s", output)

    def test_an_untracked_probe_that_cannot_answer_fails_instead_of_passing(self) -> None:
        """A failed untracked-file query is not an empty list: that is the answer that looks like a pass.

        ``git ls-files --others --exclude-standard`` refusing used to become ``[]`` here, so every file
        git would add was silently reported as nothing to add and the run passed. The caller now refuses
        the answer by name.
        """
        code, output = self.tree.run_parity(
            tree_files=lambda root: (self.tree.git_known_paths(), None))
        self.assert_failed(code, output, "could not ask git which untracked paths")

    def test_an_ignored_carried_path_fails(self) -> None:
        code, output = self.tree.run_parity(
            ignored_paths=lambda root, paths: ["docs/note.md"])
        self.assert_failed(code, output, "docs/note.md")
        self.assertIn("ignore rules", output)

    def test_a_stale_deviation_fails(self) -> None:
        """A declared deviation whose file holds upstream's own bytes is a declaration out of date.

        The clean tree's ``README.md`` differs from the recorded upstream bytes, which is what a
        declaration means; restoring those bytes has to fail, or the checker would wave through a
        project-owned file that quietly became upstream's copy.
        """
        code, output = self.tree.run_parity()
        self.assert_clean(code, output)
        # Restoring upstream's own bytes at a declared-deviation path has to fail: the file would be
        # carried, not deviated, and the manifest would be lying about which of the two it is.
        self.tree.write_bytes("README.md", self.tree.deviations["README.md"][1])
        code, output = self.tree.run_parity()
        self.assert_failed(code, output, "holds upstream's own bytes")

    def test_a_generated_input_that_changed_fails(self) -> None:
        self.tree.write_bytes(SNAPSHOT, b"different snapshot")
        self.assert_failed(*self.tree.run_parity(), "generated input")

    def test_a_generated_input_is_cross_checked_against_the_carried_tool(self) -> None:
        self.tree.write(TOOL, self.tree.tool_source("0" * 64))
        code, output = self.tree.run_parity()
        self.assert_failed(code, output, "pins")
        self.tree.write(TOOL, "package main\n\n// no constant here\n")
        self.assert_failed(*self.tree.run_parity(), "no longer carries")

    def test_the_docs_must_record_the_pin(self) -> None:
        self.tree.write("docs/upstream.md", "# Upstream\n\nNo revision recorded.\n")
        self.assert_failed(*self.tree.run_parity(), "docs/upstream.md does not record")

    def test_the_cli_must_report_the_pinned_revision(self) -> None:
        self.tree.write("cmd/ccc/pin.go", f'package main\n\nconst upstreamRevision = "{"f" * 40}"\n')
        code, output = self.tree.run_parity()
        self.assert_failed(code, output, "cmd/ccc/pin.go reports")
        (self.tree.root / "cmd/ccc/pin.go").unlink()
        self.assert_failed(*self.tree.run_parity(), "is missing")

    def test_the_executable_bit_is_checked_where_the_platform_has_it(self) -> None:
        """A carried 100755 file that is not executable is a difference, where mode bits exist.

        Windows reports no POSIX mode bits at all, so the same rule answers there by declining to
        check (asserted by the case below) instead of failing every executable file in the tree.
        """
        if os.name == "nt":
            self.assertEqual(self.tree.parity.executable_bit_supported(), False)
            return
        os.chmod(self.tree.root / "src/b.sh", 0o644)
        code, output = self.tree.run_parity(executable_bit_supported=lambda: True)
        self.assert_failed(code, output, "executable bit")

    def test_the_owner_executable_bit_is_the_one_compared_with_the_record(self) -> None:
        """Git's executable mode *is* the owner's execute bit, so ``0655`` is ``100644`` to git.

        A carried ``100755`` file chmod'ed to ``0655`` keeps the group and other execute bits: a
        predicate that asked whether *any* execute bit was set still answers "executable", and with
        ``core.filemode=false`` the pinned index blob, ``git status`` and the recorded bytes all agree
        with the modified tree -- the modification git would record is exactly the owner bit that was
        cleared. The comparison therefore asks for ``S_IXUSR``, the bit git's own mode is derived from.
        """
        if os.name == "nt":
            self.assertEqual(self.tree.parity.executable_bit_supported(), False)
            return
        path = self.tree.root / "src/b.sh"
        os.chmod(path, 0o655)
        self.assertNotEqual(path.stat().st_mode & 0o111, 0, "the case needs the other bits left set")
        code, output = self.tree.run_parity(executable_bit_supported=lambda: True)
        self.assert_failed(code, output, "src/b.sh")
        self.assertIn("changed mode", output)
        self.assertIn("owner-executable bit", output)

    def test_a_platform_without_mode_bits_discloses_them(self) -> None:
        code, output = self.tree.run_parity(executable_bit_supported=lambda: False)
        self.assert_clean(code, output)
        self.assertIn("owner-executable bits not verified", output)

    def test_a_malformed_manifest_fails_with_a_message(self) -> None:
        self.tree.write(MANIFEST, "pin " + PIN + "\nfile\t100644\tdeadbeef\tnot-a-number\tx\n")
        code, output = self.tree.run_parity()
        self.assertEqual(code, 1, output)
        self.assertIn("malformed", output)

    def test_an_unknown_record_kind_is_rejected_by_the_parser(self) -> None:
        with self.assertRaises(self.tree.manifest_module.ManifestError):
            self.tree.manifest_module.parse_manifest("unknown\ta\tb\t1\tc\n")

    def test_the_manifest_round_trips(self) -> None:
        text = (self.tree.root / MANIFEST).read_text(encoding="utf-8")
        parsed = self.tree.manifest_module.parse_manifest(text)
        again = self.tree.manifest_module.parse_manifest(parsed.render())
        self.assertEqual(parsed.header, again.header)
        self.assertEqual([r.line for r in parsed.records], [r.line for r in again.records])
        self.assertTrue(again.is_owned("project/anything.md"))
        self.assertFalse(again.is_owned("src/a.txt"))

    def test_git_object_names_match_gits_own(self) -> None:
        module = self.tree.manifest_module
        self.assertEqual(module.blob_sha1(b""), "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391")
        self.assertEqual(module.blob_sha1(b"hello\n"), "ce013625030ba8dba906f756967f9e9ca394464a")


class GitlessCase(ParityTreeCase, unittest.TestCase):
    """The branch a tree without git takes: the answers that need no file list, then INCOMPLETE.

    Every case here passes ``gitless_answers()`` -- no file list, no index, no status -- because that is
    the one state a tree git cannot enumerate has. What the branch claims is that real drift is still
    *reported as a failure* with its offenders before the run says it could not verify the rest, so a
    tree nobody could enumerate is never read as a tree that holds the pin.
    """

    def test_a_tree_git_cannot_enumerate_still_fails_on_drift_it_can_see(self) -> None:
        """Drift the bytes can answer is a FAIL, not hidden behind the INCOMPLETE verdict."""
        self.tree.break_carried("src/a.txt", b"alpha changed\n")
        code, output = self.tree.run_parity(**self.tree.gitless_answers())
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("differ from the pinned revision", output)

    def test_stored_bytes_are_verified_against_the_files_own_bytes_without_git(self) -> None:
        """The declared normalisation is answered by the file where there is no index to read.

        The record's stored bytes are what a checkout writes, so the tree's own copy must hash to them;
        the same declaration is answered through the index in ``GitBranchCase``. A copy that holds
        neither the stored bytes nor upstream's own is a failure here too.
        """
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        code, output = self.tree.run_parity(**self.tree.gitless_answers())
        self.assert_incomplete(code, output)
        self.tree.write_bytes("README.md", b"locally rewritten\n")
        self.assert_failed(*self.tree.run_parity(**self.tree.gitless_answers()),
                           "neither the bytes this tree stores")

    def test_a_recorded_path_that_is_absent_fails_without_git(self) -> None:
        """The other answer the same rule gives where there is no index: the record is not honoured."""
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.tree.remove("README.md")
        self.assert_failed(*self.tree.run_parity(**self.tree.gitless_answers()),
                           "the manifest records the bytes this tree stores")

    def test_a_generated_input_that_changed_fails_without_git(self) -> None:
        """The input's hash and its cross-check against the pinned tool need no file list either."""
        self.tree.write_bytes(SNAPSHOT, b"different snapshot")
        self.assert_failed(*self.tree.run_parity(**self.tree.gitless_answers()), "generated input")


class StoredDeviationCase(ParityTreeCase, unittest.TestCase):
    """Cases for a declared deviation this tree carries anyway, with the bytes it stores pinned.

    Such a record is the only carried path whose bytes can never equal upstream's blob (upstream's own
    attribute file rewrites its line endings), so the manifest has to pin the stored digest for the
    checker to verify anything at all. Only its *bytes* are allowed to differ: the rules that need no
    byte comparison treat it as the carried path it is -- the ignore rule here, and the index-backed
    presence, working-tree and mode rules in ``GitBranchCase``.
    """

    upstream = b"upstream\r\nreadme\n"
    stored = b"upstream\nreadme\n"

    def test_the_stored_bytes_are_verified_against_the_record(self) -> None:
        """The bytes this tree stores are checked, and the answer names the path it verified.

        The scaffolding here is a scripted checkout (``index_state``), so this is the index-backed
        answer; the same rule where git cannot answer at all -- verified against the file's own bytes
        -- is pinned in ``GitlessCase``.
        """
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        code, output = self.tree.run_parity()
        self.assert_clean(code, output)
        self.assertIn("declared stored bytes verified: README.md", output)

    def test_a_working_tree_copy_holding_upstreams_bytes_is_disclosed_not_failed(self) -> None:
        """A copy of a worktree the importer wrote holds upstream's bytes; git normalises them away."""
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        self.tree.write_bytes("README.md", self.upstream)
        code, output = self.tree.run_parity()
        self.assert_clean(code, output)
        self.assertIn("working tree still holds upstream's pre-normalisation bytes for: README.md",
                      output)

    def test_stored_bytes_that_drifted_fail(self) -> None:
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        self.tree.write_bytes("README.md", b"locally rewritten\n")
        self.assert_failed(*self.tree.run_parity(), "neither the bytes this tree stores")

    def test_a_recorded_path_that_is_absent_fails(self) -> None:
        """The record pins bytes this tree stores; a checkout that lost the file is not carrying them.

        With git available the index still holds those bytes (which is what a fresh clone would
        receive) and the working tree holds no file -- the git-backed answer this run reports. The
        git-less branch reports the same rule by its record, pinned in ``GitlessCase``.
        """
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        self.tree.remove("README.md")
        self.assert_failed(*self.tree.run_parity(), "holds no file at it")

    def test_a_record_whose_stored_bytes_are_upstreams_own_fails(self) -> None:
        """A declaration whose stored byte *is* upstream's blob declares nothing: it is carried."""
        self.tree.deviations["README.md"] = ("100644", self.upstream, self.upstream)
        self.tree.write_bytes("README.md", self.upstream)
        self.tree.write_manifest()
        self.assert_failed(*self.tree.run_parity(), "carried, not deviated")

    def test_an_ignored_carried_deviation_fails(self) -> None:
        """The normalised path is a carried path, so the ignore rule is asked about it too.

        The answer here is a filter over the paths the check asks about, so this case fails when the
        stored-byte deviation is left out of that list -- which is exactly how an ignore rule that
        would keep the file out of a fresh clone used to pass.
        """
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        code, output = self.tree.run_parity(
            ignored_paths=lambda root, paths: [path for path in paths if path == "README.md"])
        self.assert_failed(code, output, "README.md")
        self.assertIn("ignore rules", output)

    def test_a_stored_deviation_whose_executable_bit_changed_fails(self) -> None:
        """The bytes it stores are verified, but the copy this tree ships is checked for its mode too.

        A declared normalisation is a carried path for every rule but the byte comparison, and the
        bytes cannot answer this: the file holds exactly the declared digest while its mode has been
        changed, and no git-backed answer has to disagree for that to be a difference (a checkout with
        ``core.filemode=false`` never records one). The case below reaches the same rule through the
        git-backed branch, where the index mode and a clean status would both pass it.
        """
        if os.name == "nt":
            self.assertEqual(self.tree.parity.executable_bit_supported(), False)
            return
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        path = self.tree.root / "README.md"
        path.chmod(path.stat().st_mode | 0o111)
        code, output = self.tree.run_parity(executable_bit_supported=lambda: True)
        self.assert_failed(code, output, "README.md")
        self.assertIn("changed mode", output)
        self.assertIn("executable bit", output)

    def test_a_platform_without_mode_bits_discloses_a_stored_deviations_mode_too(self) -> None:
        """The disclosure covers every carried path, not only the ones ``check_files`` iterates.

        The tree carries three files and one stored deviation; a run that only counted the files would
        report three here, and the deviated path's own bit would be silently unchecked rather than
        disclosed as unverifiable.
        """
        self.tree.add_stored_deviation("README.md", self.upstream, self.stored)
        code, output = self.tree.run_parity(executable_bit_supported=lambda: False)
        self.assert_clean(code, output)
        expected = len(self.tree.carried) + len(self.tree.deviations)
        self.assertIn(f"owner-executable bits not verified for {expected} file(s)", output)


class ScriptedGit:
    """The git queries the checker makes, answered from a scripted repository state.

    No case in this suite runs a tool: this stand-in is installed as the parity module's
    ``subprocess``, and answers in git's own wire formats (``ls-files -s -z``,
    ``status --porcelain -z``, ``check-ignore -z``), so the shipped parsers and the index branch of
    the deviation rules run for real on any host.
    """

    def __init__(self, tracked=(), others=(), index=None, porcelain=b"", ignored=(), symlinks=None):
        self.tracked = list(tracked)
        self.others = list(others)
        self.index = dict(index or {})
        self.porcelain = porcelain
        self.ignored = list(ignored)
        #: git's ``core.symlinks`` for this scripted checkout: ``True``/``False`` as git records them,
        #: or ``None`` for the key being unset (git's own "the platform default applies" answer).
        self.symlinks = symlinks

    def run(self, cmd, input=None, **kwargs):  # noqa: A002 - the signature subprocess.run has
        args = list(cmd[3:])  # drop "git", "-C" and the tree the query runs in
        if args[:1] == ["ls-files"] and "-s" in args:
            stdout = "\0".join(
                f"{mode} {blob} 0\t{path}" for path, (mode, blob) in sorted(self.index.items()))
        elif args[:1] == ["ls-files"] and "--others" in args:
            stdout = "\0".join(self.others)
        elif args[:1] == ["ls-files"]:
            stdout = "\0".join(self.tracked)
        elif args[:1] == ["status"]:
            stdout = self.porcelain.decode("utf-8")
        elif args[:1] == ["check-ignore"]:
            stdout = "\0".join(self.ignored)
        elif args[:1] == ["config"]:
            if self.symlinks is None:
                # git exits 1 with no output for a key that is set nowhere: the answer the shipped
                # reader takes as "the platform's default applies", not as an unanswered question.
                return tool_result(cmd, 1, b"", b"")
            stdout = "true" if self.symlinks else "false"
        else:
            raise AssertionError(f"the case does not script this git query: {args}")
        # Bytes, like ``subprocess.run(capture_output=True)`` without ``text=True``: the shipped
        # readers decode the output themselves, which is part of what these cases exercise.
        return tool_result(cmd, 0, stdout.encode("utf-8"), b"")


class GitBranchCase(ParityTreeCase, unittest.TestCase):
    """Cases for the shipped git-backed branches: the index and git's own status output.

    ``ParityTree.run_parity`` patches the two seams by default, so these cases hand it a scripted git
    instead -- the branch a real repository takes, which nothing else in the suite reaches.
    """

    def setUp(self) -> None:
        super().setUp()
        # The scripted repository starts as this tree's own clean state: the index holds the manifest's
        # bytes for every path it names (a carried file's pinned blob, a deviation's own copy, the
        # generated input's bytes), and git's file list is the same set of paths.
        self.index = self.tree.index_state()
        self.tracked = self.tree.git_known_paths()

    def stage(self, path: str, data: bytes, mode: str = "100644") -> None:
        self.index[path] = (mode, blob_sha1(data))
        if path not in self.tracked:
            self.tracked.append(path)

    def run_with_git(self, **scripted) -> tuple[int, str]:
        git = ScriptedGit(tracked=self.tracked, index=self.index, **scripted)
        return self.tree.run_parity(subprocess=git)

    def test_a_clean_tree_passes_through_the_git_branch(self) -> None:
        code, output = self.run_with_git()
        self.assert_clean(code, output)
        self.assertIn("compared through git's own object store", output)

    def test_a_generated_input_absent_from_the_index_fails(self) -> None:
        """Correct bytes on disk are not a build input a clone receives: the index has to hold it too.

        The carried ``snapshot/.gitignore`` ignores this file, so git would never list it as untracked
        and nothing but the index says whether a clone gets it -- which is why ``check_untracked``
        accepts the manifest's naming of the path. A working-tree copy that is present, ignored and
        correct is therefore compatible with every other rule here, while a fresh clone has no file at
        all and the carried ``internal/pricing/fallback.go`` has nothing to embed.
        """
        index = dict(self.index)
        del index[SNAPSHOT]
        tracked = [path for path in self.tracked if path != SNAPSHOT]
        # git's untracked list is empty here because the carried snapshot .gitignore matches the path,
        # and the manifest names it -- so the "unexplained file" rule is satisfied while no clone would
        # receive it.
        code, output = self.tree.run_parity(
            subprocess=ScriptedGit(tracked=tracked, index=index))
        self.assert_failed(code, output, "absent from git's index")
        self.assertIn(SNAPSHOT, output)

    def test_a_generated_input_whose_indexed_bytes_differ_fails(self) -> None:
        """The index's blob is compared with the bytes the file must hold, not merely with itself."""
        index = dict(self.index)
        index[SNAPSHOT] = (index[SNAPSHOT][0], "0" * 40)
        code, output = self.tree.run_parity(subprocess=ScriptedGit(self.tracked, index=index))
        self.assert_failed(code, output, SNAPSHOT)
        self.assertIn("git stores", output)

    def test_a_generated_input_whose_indexed_mode_changed_fails(self) -> None:
        """The type git stores the input as is part of what a clone receives."""
        index = dict(self.index)
        index[SNAPSHOT] = ("100755", index[SNAPSHOT][1])
        code, output = self.tree.run_parity(subprocess=ScriptedGit(self.tracked, index=index))
        self.assert_failed(code, output, "git stores mode 100755")

    def test_a_staged_deviation_that_became_upstreams_copy_is_stale(self) -> None:
        """The index is what a clone receives, so upstream's blob there is a stale declaration."""
        self.stage("README.md", self.tree.deviations["README.md"][1])
        code, output = self.run_with_git()
        self.assert_failed(code, output, "it holds upstream's own bytes")

    def test_an_unstaged_deviation_that_became_upstreams_copy_is_stale(self) -> None:
        """The gate reads the working tree: an unstaged copy is the same stale declaration."""
        self.tree.write_bytes("README.md", self.tree.deviations["README.md"][1])
        code, output = self.run_with_git()
        self.assert_failed(code, output, "the working tree holds upstream's own bytes")

    def test_a_worktree_modified_carried_file_is_reported_through_git_status(self) -> None:
        code, output = self.run_with_git(porcelain=b" M docs/note.md\0")
        self.assert_failed(code, output, "differ from what git stores for them")

    def test_a_worktree_edit_git_status_cannot_report_is_still_caught(self) -> None:
        """The bytes are what a carried file *is*, and no index flag may stop that being checked.

        ``git update-index --assume-unchanged`` (or ``skip-worktree``) keeps an edited path out of
        ``git status`` while the pinned blob stays in the index, so every git-backed predicate here
        still agrees: no porcelain entry to report it as dirty, the pinned digest in the index, the
        record's mode. A check that trusted git's own status for the working tree therefore passed for
        modified carried source. The working tree is hashed independently, so the scripted status
        answering "nothing modified" is exactly the case this pins.
        """
        self.tree.break_carried("src/a.txt", b"alpha, locally rewritten\n")
        code, output = self.run_with_git(porcelain=b"")
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("differ from the pinned revision", output)

    def test_a_same_size_worktree_edit_is_caught_by_its_bytes(self) -> None:
        """The digest decides, not the size: an edit that keeps the length has to be caught too.

        An edited file usually changes length, so a check that compared sizes (and trusted git) would
        still fail those cases. This one is the same size as the pinned bytes and is visible only to
        the hash of the copy this repository ships, with git's index and status both scripted clean.
        """
        self.tree.break_carried("src/a.txt", b"omega\n")  # six bytes, like the pinned b"alpha\n"
        code, output = self.run_with_git(porcelain=b"")
        self.assert_failed(code, output, "src/a.txt")
        self.assertIn("differ from the pinned revision", output)

    def test_a_link_replaced_by_a_file_under_a_silent_status_is_still_caught(self) -> None:
        """A link can be replaced without changing a byte of what git stores, so the shape is the answer.

        ``CLAUDE.md`` is a carried symlink whose blob *is* its target text, so replacing the link with a
        regular file of exactly that text leaves git's index entry (mode ``120000``, the pinned blob)
        and every other predicate here agreeing -- and with ``skip-worktree`` set on the path,
        ``git status`` reports nothing at all. ``core.symlinks=true`` is git saying this checkout
        creates the link, which is the state the pin is about: a check that accepted a file here would
        certify a carried path whose working-tree type, and behaviour, had changed.
        """
        target = b"a.txt"
        self.tree.write_bytes("src/link", target)
        self.tree.add_link_record("src/link", target)
        self.index = self.tree.index_state()
        self.tracked = self.tree.git_known_paths()
        code, output = self.run_with_git(symlinks=True, porcelain=b"")
        self.assert_failed(code, output, "changed file type")
        self.assertIn("src/link", output)

    def test_a_link_held_as_a_file_where_git_says_so_is_the_same_file_accepted(self) -> None:
        """The control for the case above: the same file and the same clean status, one git answer apart.

        A checkout that does not create symlinks holds the link's target text in a file, and git says so
        with ``core.symlinks=false``; there the file *is* the pin's working-tree copy. Without this
        control the shape rule could pass by refusing a file at a link record everywhere, including
        where that is what the checkout is configured to hold.
        """
        target = b"a.txt"
        self.tree.write_bytes("src/link", target)
        self.tree.add_link_record("src/link", target)
        self.index = self.tree.index_state()
        self.tracked = self.tree.git_known_paths()
        code, output = self.run_with_git(symlinks=False, porcelain=b"")
        self.assert_clean(code, output)
        self.assertIn("carried link(s) held as a file with the link's target text: 1", output)

    def test_a_worktree_shape_change_git_status_cannot_report_is_still_caught(self) -> None:
        path = self.tree.root / "docs/note.md"
        path.unlink()
        path.mkdir()
        self.assert_failed(*self.run_with_git(porcelain=b""), "changed file type")

    def test_a_worktree_executable_bit_git_status_cannot_report_is_still_caught(self) -> None:
        """The mode is checked on the file itself, the way the bytes-only branch always checked it."""
        if os.name == "nt":
            self.assertEqual(self.tree.parity.executable_bit_supported(), False)
            return
        os.chmod(self.tree.root / "src/b.sh", 0o644)
        code, output = self.run_with_git(porcelain=b"")
        self.assert_failed(code, output, "src/b.sh")
        self.assertIn("changed mode", output)

    def test_a_carried_path_git_does_not_store_is_absent_from_a_clone(self) -> None:
        """Present on disk is not carried: a fresh clone receives what git's index holds."""
        del self.index["docs/note.md"]
        self.assert_failed(*self.run_with_git(), "absent")

    def test_a_record_whose_size_is_not_the_bytes_size_is_a_failure(self) -> None:
        """The size is checked beside the digest, so a record whose two halves disagree is caught."""
        manifest = self.tree.root / MANIFEST
        text = manifest.read_text(encoding="utf-8")
        line = next(line for line in text.splitlines() if line.endswith("\tsrc/a.txt"))
        manifest.write_text(text.replace(line, line.replace("\t6\t", "\t5\t")), encoding="utf-8")
        self.assert_failed(*self.tree.run_parity(), "changed size")

    def test_a_staged_but_uncommitted_tree_is_not_dirty(self) -> None:
        """The second porcelain column is the working tree; only that one means "modified on disk"."""
        code, output = self.run_with_git(porcelain=b"M  src/a.txt\0")
        self.assert_clean(code, output)

    def test_a_git_status_that_cannot_answer_fails_instead_of_passing(self) -> None:
        """A probe with no answer must not read as "nothing is modified" -- that would look like a pass."""
        code, output = self.tree.run_parity(
            git_index=lambda root: dict(self.index), git_modified=lambda root: None)
        self.assert_failed(code, output, "could not ask git which carried paths differ")

    def test_a_carried_normalisation_is_verified_through_the_index(self) -> None:
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        code, output = self.run_with_git()
        self.assert_clean(code, output)
        self.assertIn("declared stored bytes verified: README.md", output)

    def test_a_carried_normalisation_whose_stored_digest_drifted_fails(self) -> None:
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", b"not what the record pins\n")
        self.assert_failed(*self.run_with_git(), "git stores")

    def test_a_carried_normalisation_whose_working_tree_holds_upstreams_bytes_is_disclosed(self) -> None:
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        self.tree.write_bytes("README.md", StoredDeviationCase.upstream)
        code, output = self.run_with_git()
        self.assert_clean(code, output)
        self.assertIn("working tree still holds upstream's pre-normalisation bytes for: README.md",
                      output)

    def test_a_carried_normalisation_whose_working_tree_holds_neither_fails(self) -> None:
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        self.tree.write_bytes("README.md", b"locally rewritten\n")
        self.assert_failed(*self.run_with_git(), "neither the bytes this tree stores")

    def test_a_carried_normalisation_deleted_from_the_worktree_fails(self) -> None:
        """git's index holding the path is not the working tree holding it: the file is carried.

        The stored bytes are verified through the index, so a record alone cannot tell a checkout that
        lost the file from one that has it -- the working tree is what this repository ships, and an
        absent file there is an absent carried path.
        """
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        self.tree.remove("README.md")
        self.assert_failed(*self.run_with_git(), "holds no file at it")

    def test_a_carried_normalisation_modified_in_the_worktree_fails(self) -> None:
        """A working tree git reports as modified is the same failure as for a carried file."""
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        self.tree.write_bytes("README.md", b"locally rewritten\n")
        code, output = self.run_with_git(porcelain=b" M README.md\0")
        self.assert_failed(code, output, "differ from what git stores for them")
        self.assertIn("README.md", output)

    def test_a_carried_normalisation_whose_index_mode_changed_fails(self) -> None:
        """Its mode is checked where git answers, exactly as a carried file's mode is."""
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored, mode="100755")
        self.assert_failed(*self.run_with_git(), "git stores mode 100755")

    def test_a_carried_normalisation_absent_from_the_index_fails(self) -> None:
        """A path this tree carries has to be what git stores, or a fresh clone does not receive it.

        The working tree holds the declared bytes and the record pins them, but the path is not in the
        index -- an unstaged addition, or one removed from it -- so the clone that receives this tree
        would receive no file there at all.
        """
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        del self.index["README.md"]
        self.assert_failed(*self.run_with_git(), "absent from git's index")

    def test_a_carried_normalisation_whose_worktree_mode_changed_is_still_caught(self) -> None:
        """The finding's scenario: git's index and status both keep answering as if nothing changed.

        On a POSIX checkout with ``core.filemode=false`` -- and wherever the filesystem keeps no mode
        bits -- flipping the executable bit on the normalised carried path leaves every git-backed
        predicate agreeing: the file hashes to the declared stored digest, the index still records the
        record's mode, and ``git status`` reports nothing. The working tree's own mode is therefore
        asked of the file itself for that path too, which is what makes the declared allowance cover
        its bytes and nothing else.
        """
        if os.name == "nt":
            self.assertEqual(self.tree.parity.executable_bit_supported(), False)
            return
        self.tree.add_stored_deviation("README.md", StoredDeviationCase.upstream,
                                       StoredDeviationCase.stored)
        self.stage("README.md", StoredDeviationCase.stored)
        path = self.tree.root / "README.md"
        path.chmod(path.stat().st_mode | 0o111)
        # The mode check is forced on, like the other mode cases: the rule under test is the
        # comparison, not what this host's filesystem reports.
        git = ScriptedGit(tracked=self.tracked, index=self.index, porcelain=b"")
        code, output = self.tree.run_parity(subprocess=git, executable_bit_supported=lambda: True)
        self.assert_failed(code, output, "README.md")
        self.assertIn("changed mode", output)


class ImportCase(unittest.TestCase):
    """Cases that assert the importer's declarations, which the checker trusts."""

    def setUp(self) -> None:
        self.import_script = load_script("upstream-import.py")

    def layout(self):
        """The shape this tree has, which decides which declarations are the tree's own."""
        return self.import_script.detect_layout(REPO_ROOT)

    def test_every_owned_prefix_exists_in_this_repository(self) -> None:
        """A typo in an owned list would make the checker accept an unexplained file.

        Which list is the tree's own is the layout's answer: in the root layout the prefixes the
        importer spells out, and in the directory layout none at all -- the carry's own directory is
        the boundary there, and a list would be a second spelling of it.
        """
        owned = self.import_script.owned_prefixes(self.layout())
        if self.layout().carry:
            self.assertEqual(owned, (), "the directory layout declares no owned prefix")
        missing = [entry for entry in owned if not (REPO_ROOT / entry).exists()]
        self.assertEqual(missing, [], f"owned entries with no path in the tree: {missing}")

    def test_the_owned_list_and_the_manifest_agree(self) -> None:
        manifest = load_script("upstream_manifest.py").load_manifest(REPO_ROOT / MANIFEST)
        self.assertEqual(sorted(manifest.owned),
                         sorted(self.import_script.owned_prefixes(self.layout())))

    def test_every_path_this_shape_declares_has_a_stated_reason(self) -> None:
        layout = self.layout()
        for path in self.import_script.skip_reasons(layout):
            self.assertTrue(self.import_script.skip_reason(path, layout).strip(), path)
        if not layout.carry:
            for path in (".gitignore", "AGENTS.md", "README.md", "docs/README.md"):
                self.assertIn("project-owned", self.import_script.skip_reason(path, layout))
            self.assertIn("CI definitions",
                          self.import_script.skip_reason(".github/workflows/ci.yml", layout))
        self.assertEqual(self.import_script.skip_reason("internal/parser/claude.go", layout), "")

    def test_no_carried_path_is_declared_project_owned(self) -> None:
        """The importer must not carry a path this repository claims to own, or vice versa.

        The two halves are read in the shape's own terms: at the root layout a carried record's path
        must not match an owned prefix, and at the directory layout -- where nothing outside the carry
        directory is a record at all -- no carried record may be declared a skip. The record paths are
        *upstream* paths in both, which is why the owned question is asked about the tree path.
        """
        manifest = load_script("upstream_manifest.py").load_manifest(REPO_ROOT / MANIFEST)
        layout = self.layout()
        for record in manifest.files:
            if not layout.carry:
                self.assertFalse(manifest.is_owned(record.path), record.path)
            self.assertEqual(self.import_script.skip_reason(record.path, layout), "", record.path)

    def test_the_importer_refuses_a_pin_that_is_not_the_checkouts_head(self) -> None:
        """A pin that is not the checkout's HEAD must stop the import, named as such.

        The rule has its own answer -- the revision reported back differs from ``--pin`` -- and it is
        reached here by the module's own ``git`` seam, because a real two-commit checkout would need a
        host git this suite must not depend on. The answer is asserted by its own words, not by the
        word "HEAD" that the guard above it also prints.
        """
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".git").mkdir()
            answer = tool_result(["git"], 0, REVISION + "\n", "")
            with mock.patch.object(self.import_script, "git", lambda *args, **kwargs: answer):
                with self.assertRaises(SystemExit) as raised:
                    self.import_script.main(["--upstream", str(root), "--pin", PIN, "--dry-run"])
            self.assertIn("is not the checkout's HEAD", str(raised.exception))

    def test_the_importer_stops_when_the_revision_cannot_be_confirmed(self) -> None:
        """Without a git answer the revision cannot be confirmed, so the importer must stop."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".git").mkdir()
            with self.assertRaises(SystemExit) as raised:
                self.import_script.main(["--upstream", str(root), "--pin", PIN, "--dry-run"])
            # Either git is missing or it answered "not a repository": both mean the revision cannot
            # be confirmed, and both must stop the import rather than carry an unverified revision.
            self.assertIn("git rev-parse HEAD failed", str(raised.exception))

    def test_the_importer_refuses_a_directory_that_is_not_a_checkout(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(SystemExit) as raised:
                self.import_script.main(["--upstream", directory, "--dry-run"])
            self.assertIn("not a git checkout", str(raised.exception))

    def test_every_declared_normalisation_is_a_skip_path_with_a_reason(self) -> None:
        """A declared normalisation is carried by the write path, so it must be declared as skipped."""
        for path in self.import_script.NORMALISED:
            self.assertIn("normalised", self.import_script.skip_reason(path), path)


class ImportWriteCase(unittest.TestCase):
    """Cases for the importer's write path, driven without git: what it prunes and what it refuses.

    The importer reads a checkout's git objects, which no case here may need; the step that turns
    those objects into files is a function of its own, so a case can hand it entries and watch the
    tree -- including the shape change that used to crash half-way through a pin update.
    """

    def setUp(self) -> None:
        self.import_script = load_script("upstream-import.py")
        self.manifest_module = load_script("upstream_manifest.py")
        self.root = scratch_tree("import-root-")
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)

    def entry(self, path: str, data: bytes, mode: str = "100644"):
        """One entry as the importer builds it: ``(mode, blob, path)`` plus the blob's contents."""
        return (mode, blob_sha1(data), path), {blob_sha1(data): data}

    def previous(self, path: str, data: bytes):
        return self.manifest_module.Manifest(
            {"pin": PIN},
            [self.manifest_module.Record("file", "100644", blob_sha1(data), len(data), path)],
        )

    def write(self, entries, contents, previous=None, prune=False) -> None:
        self.import_script.carry_entries(self.root, entries, contents, previous, prune, False)

    def test_a_file_that_became_a_directory_is_pruned_before_the_write(self) -> None:
        """The revision that carried ``sub`` as a file may carry ``sub/alpha.txt``: prune, then write.

        Written the other way round, the write hits the file the prune is about to remove and stops
        half-way through the tree with a FileExistsError.
        """
        (self.root / "sub").write_bytes(b"carried as a file\n")
        entry, contents = self.entry("sub/alpha.txt", b"alpha\n")
        self.write([entry], contents, self.previous("sub", b"carried as a file\n"), prune=True)
        self.assertEqual((self.root / "sub" / "alpha.txt").read_bytes(), b"alpha\n")

    def test_a_collision_without_prune_is_named_not_a_traceback(self) -> None:
        (self.root / "sub").write_bytes(b"carried as a file\n")
        entry, contents = self.entry("sub/alpha.txt", b"alpha\n")
        with self.assertRaises(SystemExit) as raised:
            self.write([entry], contents)
        message = str(raised.exception)
        self.assertIn("sub is a file", message)
        self.assertIn("sub/alpha.txt", message)
        self.assertNotIsInstance(raised.exception, OSError)

    def test_a_directory_where_the_pin_carries_a_file_is_named(self) -> None:
        (self.root / "note.md").mkdir()
        entry, contents = self.entry("note.md", b"note\n")
        with self.assertRaises(SystemExit) as raised:
            self.write([entry], contents)
        self.assertIn("note.md is a directory", str(raised.exception))

    def test_prune_refuses_a_file_modified_here(self) -> None:
        (self.root / "gone.txt").write_bytes(b"edited here\n")
        with self.assertRaises(SystemExit) as raised:
            self.write([], {}, self.previous("gone.txt", b"carried\n"), prune=True)
        self.assertIn("refusing to prune", str(raised.exception))
        self.assertTrue((self.root / "gone.txt").exists())

    def test_a_declared_normalisation_is_written_with_the_bytes_git_stores(self) -> None:
        """The worktree must equal what a clone checks out, so the write applies the normalisation."""
        path = self.import_script.NORMALISED[0]
        upstream = b"upstream\r\nreadme\n"
        entry, contents = self.entry(path, upstream)
        self.write([entry], contents)
        self.assertEqual((self.root / path).read_bytes(), b"upstream\nreadme\n")

        layout = self.import_script.detect_layout(REPO_ROOT)
        record = self.import_script.deviation_record("100644", blob_sha1(upstream), upstream, path,
                                                     layout)
        self.assertEqual(record.stored_digest, blob_sha1(b"upstream\nreadme\n"))
        self.assertEqual(record.stored_size, len(b"upstream\nreadme\n"))

    def test_a_normalisation_that_rewrites_nothing_is_refused(self) -> None:
        """A pin that already stores LF makes the declaration wrong: the path is carried, not deviated."""
        path = self.import_script.NORMALISED[0]
        already_stored = b"upstream\nreadme\n"
        entry, contents = self.entry(path, already_stored)
        with self.assertRaises(SystemExit) as raised:
            self.write([entry], contents)
        self.assertIn("carry it as a file instead", str(raised.exception))


class ScratchTreeCase(unittest.TestCase):
    """The suite's own scratch trees stay outside the repository: the ``parity-root-*`` leak's rule."""

    def setUp(self) -> None:
        self.addCleanup(setattr, tempfile, "tempdir", tempfile.tempdir)

    def test_a_temp_root_inside_the_repository_is_refused(self) -> None:
        inside = REPO_ROOT / "scratch-tmp-for-parity-cases"
        inside.mkdir(exist_ok=True)
        try:
            tempfile.tempdir = str(inside)
            root = scratch_tree("parity-root-")
        finally:
            shutil.rmtree(inside, ignore_errors=True)
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        self.assertTrue(root.is_absolute(), root)
        self.assertNotEqual(root, REPO_ROOT)
        self.assertNotIn(REPO_ROOT, root.parents)

    def test_a_relative_temp_root_does_not_land_under_the_current_directory(self) -> None:
        tempfile.tempdir = "relative-parity-scratch"
        root = scratch_tree("parity-root-")
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        self.assertTrue(root.is_absolute(), root)
        self.assertNotIn(REPO_ROOT, root.parents)

    def test_the_platforms_own_scratch_bases_are_pinned_for_both_answers(self) -> None:
        """The fallback the two cases above rely on is pinned for both answers, not for this host's.

        Those cases take the environment's own answer away, so what lets them produce a tree at all is
        this list; on Windows the list they need is not ``/tmp`` (which hangs off the current drive
        there) but the variables ``tempfile`` itself reads. Both answers are driven here, with the
        variables set for the Windows one, so the rule's answer is pinned on either host rather than
        read off the machine the suite happens to run on.
        """
        windows_temp = Path(tempfile.gettempdir()) / "platform-temp"
        with mock.patch.dict(os.environ, {"TEMP": str(windows_temp),
                                          "TMP": str(windows_temp / "alt")}):
            self.assertEqual(platform_scratch_bases("nt"), [windows_temp, windows_temp / "alt"])
            self.assertEqual(platform_scratch_bases("posix"), [Path("/tmp"), Path("/var/tmp")])


class ManifestCase(unittest.TestCase):
    """Cases for the shared manifest format."""

    def setUp(self) -> None:
        self.module = load_script("upstream_manifest.py")

    def test_an_owned_record_renders_as_one_field(self) -> None:
        record = self.module.Record("owned", "", "", 0, "cmd/ccc/")
        self.assertEqual(record.line, "owned\tcmd/ccc/")

    def test_a_skip_record_carries_its_reason(self) -> None:
        record = self.module.Record("skip", "100644", "abc", 3, "README.md", "project-owned")
        self.assertEqual(record.line, "skip\t100644\tabc\t3\tREADME.md\tproject-owned")

    def test_a_stored_deviation_record_round_trips(self) -> None:
        """The bytes a carried deviation stores are two more fields: digest and size."""
        record = self.module.Record("skip", "100644", "abc", 3, "README.md", "normalised", "def", 4)
        self.assertEqual(record.line, "skip\t100644\tabc\t3\tREADME.md\tnormalised\tdef\t4")
        parsed = self.module.parse_manifest(record.line + "\n")
        self.assertEqual([r.line for r in parsed.stored_deviations], [record.line])
        self.assertEqual((parsed.skips[0].stored_digest, parsed.skips[0].stored_size), ("def", 4))

    def test_a_half_declared_stored_deviation_is_rejected(self) -> None:
        with self.assertRaises(self.module.ManifestError):
            self.module.parse_manifest("skip\t100644\tabc\t3\tREADME.md\treason\tdef\n")

    def test_only_a_skip_record_may_pin_stored_bytes(self) -> None:
        with self.assertRaises(self.module.ManifestError):
            self.module.parse_manifest("file\t100644\tabc\t3\tREADME.md\treason\tdef\t4\n")

    def test_a_stored_size_that_is_not_a_number_is_rejected(self) -> None:
        with self.assertRaises(self.module.ManifestError):
            self.module.parse_manifest("skip\t100644\tabc\t3\tREADME.md\treason\tdef\tbig\n")

    def test_a_manifest_without_a_pin_is_not_a_manifest(self) -> None:
        manifest = self.module.parse_manifest("file\t100644\tabc\t3\tx\n")
        self.assertNotIn("pin", manifest.header)

    def test_a_repeated_header_key_is_rejected(self) -> None:
        with self.assertRaises(self.module.ManifestError):
            self.module.parse_manifest(f"pin {PIN}\npin {PIN}\n")

    def test_an_unknown_header_key_is_rejected(self) -> None:
        with self.assertRaises(self.module.ManifestError):
            self.module.parse_manifest("nonsense value\n")

    def test_the_default_owned_answer_is_false(self) -> None:
        manifest = self.module.parse_manifest("")
        self.assertFalse(manifest.is_owned("anything"))
        self.assertEqual(manifest.files, [])


class RepositoryCase(unittest.TestCase):
    """One case that reads this repository's own manifest: the committed record must hold."""

    def test_the_committed_manifest_describes_this_tree(self) -> None:
        module = load_script("upstream_manifest.py")
        manifest = module.load_manifest(REPO_ROOT / MANIFEST)
        self.assertRegex(manifest.header["pin"], re.compile(r"^[0-9a-f]{40}$"))
        self.assertEqual(manifest.header["carried"], str(len(manifest.files)))
        self.assertTrue(any(record.path == SNAPSHOT for record in manifest.inputs))
        pinned = module.pinned_snapshot_sha256(REPO_ROOT / manifest.tree_path(TOOL))
        self.assertTrue(pinned, "the carried snapshot tool no longer pins a SHA-256")
        recorded = next(record.digest for record in manifest.inputs if record.path == SNAPSHOT)
        self.assertEqual(recorded, pinned)
        for path in ("ccc/cmd/ccc/", "ccc/internal/ccc/", "docs/plans/", "scripts/tests/"):
            self.assertTrue(manifest.is_owned(path), path)
        self.assertFalse(manifest.is_owned(manifest.tree_path("internal/parser/parser.go")),
                         "a carried record's own path must not read as project-owned")

    def test_the_two_modules_declare_one_go_directive(self) -> None:
        """The workflow builds with ``ccc/go.mod``; a carry needing a newer Go must be noticed here.

        ``docs/harness.md`` §CI states the claim -- the workflow, this repository's module and the
        carried tree cannot disagree about the toolchain -- and this is the half a case can hold: in
        the directory layout the two ``go.mod`` files state one ``go`` directive, and this module's
        file declares the layout's one edge (the module path, the requirement, the replace). A pin
        update that raised the carried module's directive without raising ours would leave CI building
        our module with a toolchain the carried code does not accept.
        """
        layout = load_script("upstream_manifest.py").detect_layout(REPO_ROOT)
        if not layout.carry:
            self.skipTest("the root layout has one module: no second directive to compare")
        ours = (REPO_ROOT / layout.module / "go.mod").read_text(encoding="utf-8")
        carried = (REPO_ROOT / layout.carry / "go.mod").read_text(encoding="utf-8")
        self.assertEqual(re.search(r"^go (\S+)$", ours, re.M).group(1),
                         re.search(r"^go (\S+)$", carried, re.M).group(1))
        self.assertIn("module go.kenn.io/agentsview/ccc", ours)
        self.assertIn("replace go.kenn.io/agentsview => ../agentsview", ours)
        self.assertNotIn("go.work", {p.name for p in REPO_ROOT.iterdir()})

    def test_the_declared_normalisation_records_the_bytes_this_tree_stores(self) -> None:
        """The one path upstream's attribute file rewrites must pin the bytes this tree stores.

        Upstream's blob can never be what this tree holds for that path, so a record without a stored
        digest would leave 1.4 MB verified against nothing. The case reads the file as it sits in the
        tree and requires it to be one of the two forms the record names -- the stored bytes a checkout
        writes, or upstream's pre-normalisation bytes that git normalises away -- at that form's size.
        """
        module = load_script("upstream_manifest.py")
        manifest = module.load_manifest(REPO_ROOT / MANIFEST)
        normalised = {record.path: record for record in manifest.stored_deviations}
        self.assertEqual(sorted(normalised),
                         sorted(load_script("upstream-import.py").NORMALISED))
        for path, record in normalised.items():
            self.assertNotEqual(record.stored_digest, record.digest, path)
            digest, size, _ = module.hashed_path(REPO_ROOT, manifest.tree_path(path))
            forms = {record.stored_digest: record.stored_size, record.digest: record.size}
            self.assertIn(digest, forms, f"{path} holds bytes the manifest does not name")
            self.assertEqual(forms[digest], size, path)


class CompareCheckoutCase(unittest.TestCase):
    """The comparison harness must refuse a side that is not a clean checkout of the pin.

    It attributes the dumps it compares -- and, with ``--record``, the oracle it rewrites -- to the
    pinned revision, so a checkout holding local edits would credit this tree's own work to upstream.
    The probe runs through the module's own ``run`` seam: no case here may need a host git.
    """

    def setUp(self) -> None:
        self.compare = load_script("upstream-compare.py")

    def probe(self, status_output: str):
        def run(cmd, cwd=None, env=None):
            if "rev-parse" in cmd:
                return tool_result(cmd, 0, REVISION + "\n", "")
            return tool_result(cmd, 0, status_output, "")
        return run

    def checkout(self, status_output: str) -> Path:
        base = scratch_tree("compare-base-")
        # The base holds the throw-away checkout and nothing else, so it goes with it: registered
        # here, because ``TemporaryDirectory`` removes the directory *inside* the base and the base
        # itself would otherwise be left in the temp root, one per case (measured: the shape the
        # migration's ``dir=`` argument introduced).
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        directory = tempfile.TemporaryDirectory(prefix="compare-checkout-", dir=str(base))
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        (root / ".git").mkdir()
        return root

    def test_a_clean_checkout_passes(self) -> None:
        with mock.patch.object(self.compare, "run", self.probe("")):
            self.compare.verify_checkout(self.checkout(""), REVISION)

    def test_a_dirty_checkout_is_refused(self) -> None:
        with mock.patch.object(self.compare, "run", self.probe(" M README.md\n")):
            with self.assertRaises(SystemExit) as raised:
                self.compare.verify_checkout(self.checkout(" M README.md\n"), REVISION)
        self.assertIn("local modifications", str(raised.exception))
        self.assertIn("README.md", str(raised.exception))

    def test_a_checkout_at_another_revision_is_refused(self) -> None:
        with mock.patch.object(self.compare, "run", self.probe("")):
            with self.assertRaises(SystemExit) as raised:
                self.compare.verify_checkout(self.checkout(""), PIN)
        self.assertIn("not the pinned", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
