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
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]
REPO_ROOT = SCRIPTS.parent
SCRIPT_DIRS = (str(SCRIPTS),)

#: A revision-shaped pin; the exact digits do not matter to any rule under test.
PIN = "0123456789abcdef0123456789abcdef01234567"
SNAPSHOT = "internal/pricing/snapshot/litellm_snapshot.json.gz"
TOOL = "internal/pricing/cmd/litellm-snapshot/main.go"
MANIFEST = "scripts/upstream-manifest.txt"


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
        self.root = Path(tempfile.mkdtemp(prefix="parity-root-"))
        self.manifest_module = load_script("upstream_manifest.py")
        self.parity = load_script("upstream-parity.py")
        self.import_script = load_script("upstream-import.py")
        self.carried: dict[str, tuple[str, bytes]] = {
            "src/a.txt": ("100644", b"alpha\n"),
            "src/b.sh": ("100755", b"#!/bin/sh\necho beta\n"),
            "docs/note.md": ("100644", b"note\n"),
        }
        # A deviation records the *upstream* blob and holds this tree's own bytes: the two differing
        # is what makes the declaration meaningful.
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
        for path, (mode, data) in sorted(self.deviations.items()):
            lines.append(f"skip\t{mode}\t{blob_sha1(data)}\t{len(data)}\t{path}\tproject-owned: this tree keeps its own file here")
        digest = hashlib.sha256(self.snapshot_bytes).hexdigest()
        lines.append(f"input\t100644\t{digest}\t{len(self.snapshot_bytes)}\t{SNAPSHOT}\tgenerated build input")
        (self.root / MANIFEST).parent.mkdir(parents=True, exist_ok=True)
        (self.root / MANIFEST).write_text("\n".join(lines) + "\n", encoding="utf-8")

    def run_parity(self, **patches) -> tuple[int, str]:
        """Run the shipped check against this tree, with optional module seams patched.

        A case that replaces ``tree_files`` is pretending git knows this throw-away tree, so it also
        gets a git ignore answer unless it supplies one of its own: the two halves are one seam --
        "what does git say about this tree" -- and a case about something else must not fall into the
        unanswerable-probe failure by accident. The cases that *are* about the ignore answer or about
        an unanswerable probe pass ``ignored_paths`` explicitly.
        """
        if "tree_files" in patches and "ignored_paths" not in patches:
            patches["ignored_paths"] = lambda root, paths: []
        if "git_index" not in patches and "tree_files" not in patches:
            # A throw-away tree is not a repository: the check falls back to hashing its bytes.
            patches["git_index"] = lambda root: None
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


class ParityCase(unittest.TestCase):
    """Cases that assert the shipped check's verdicts."""

    def setUp(self) -> None:
        self.tree = ParityTree()
        self.addCleanup(self.tree.cleanup)

    def assert_clean(self, code: int, output: str) -> None:
        self.assertEqual(code, 0, output)
        self.assertIn("reported by cmd/ccc: " + PIN, output)
        self.assertIn(f"carried: {len(self.tree.carried)} file(s) identical to the pinned revision",
                      output)

    def assert_failed(self, code: int, output: str, needle: str) -> None:
        self.assertEqual(code, 1, output)
        self.assertIn("parity: FAIL", output)
        self.assertIn(needle, output)

    def test_a_clean_tree_passes(self) -> None:
        self.assert_clean(*self.tree.run_parity())

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
        """git stores a link as its target text, so the same bytes in a file are the same object."""
        try:
            os.symlink("a.txt", self.tree.root / "src/link")
        except (OSError, NotImplementedError):
            # A platform without symlink support checks the link out as a file holding the target.
            self.tree.write_bytes("src/link", b"a.txt")
        target = b"a.txt"
        self.tree.carried["src/link"] = ("120000", target)
        self.tree.write_manifest()
        code, output = self.tree.run_parity()
        self.assert_clean(code, output)
        self.assertIn("symlinks hashed by their link target: 1", output)

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

    def test_a_gitless_tree_discloses_the_skipped_discovery_checks(self) -> None:
        code, output = self.tree.run_parity(tree_files=lambda root: (None, []))
        self.assert_clean(code, output)
        self.assertIn("git-less tree: file-discovery checks skipped", output)

    def test_an_unanswerable_ignore_probe_fails_instead_of_passing(self) -> None:
        """``ignored_paths`` returning None means "could not ask", which must not read as "fine"."""
        code, output = self.tree.run_parity(
            tree_files=lambda root: (["src/a.txt", "src/b.sh", "docs/note.md"], []),
            ignored_paths=lambda root, paths: None)
        self.assert_failed(code, output, "could not ask git which carried paths are ignored")

    def test_an_ignored_carried_path_fails(self) -> None:
        code, output = self.tree.run_parity(
            tree_files=lambda root: (["src/a.txt", "src/b.sh", "docs/note.md"], []),
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
        self.assert_failed(code, output, "hold upstream's own bytes")

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

    def test_a_platform_without_mode_bits_discloses_them(self) -> None:
        code, output = self.tree.run_parity(executable_bit_supported=lambda: False)
        self.assert_clean(code, output)
        self.assertIn("executable bits not verified", output)

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


class ImportCase(unittest.TestCase):
    """Cases that assert the importer's declarations, which the checker trusts."""

    def setUp(self) -> None:
        self.import_script = load_script("upstream-import.py")

    def test_every_owned_prefix_exists_in_this_repository(self) -> None:
        """A typo in the owned list would make the checker accept an unexplained file."""
        missing = [entry for entry in self.import_script.OWNED if not (REPO_ROOT / entry).exists()]
        self.assertEqual(missing, [], f"owned entries with no path in the tree: {missing}")

    def test_the_owned_list_and_the_manifest_agree(self) -> None:
        manifest = load_script("upstream_manifest.py").load_manifest(REPO_ROOT / MANIFEST)
        self.assertEqual(sorted(manifest.owned), sorted(self.import_script.OWNED))

    def test_every_carried_path_is_skipped_for_a_stated_reason(self) -> None:
        for path in (".gitignore", "AGENTS.md", "README.md", "docs/README.md"):
            self.assertIn("project-owned", self.import_script.skip_reason(path))
        self.assertIn("CI definitions", self.import_script.skip_reason(".github/workflows/ci.yml"))
        self.assertEqual(self.import_script.skip_reason("internal/parser/claude.go"), "")

    def test_no_carried_path_is_declared_project_owned(self) -> None:
        """The importer must not carry a path this repository claims to own, or vice versa."""
        manifest = load_script("upstream_manifest.py").load_manifest(REPO_ROOT / MANIFEST)
        for record in manifest.files:
            self.assertFalse(manifest.is_owned(record.path), record.path)
            self.assertEqual(self.import_script.skip_reason(record.path), "", record.path)

    def test_the_importer_refuses_a_pin_that_is_not_the_checkouts_head(self) -> None:
        """Without a git checkout the revision cannot be confirmed, so the importer must stop."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".git").mkdir()
            with self.assertRaises(SystemExit) as raised:
                self.import_script.main(["--upstream", str(root), "--pin", PIN, "--dry-run"])
            # Either git is missing or it answered "not a repository": both mean the revision cannot
            # be confirmed, and both must stop the import rather than carry an unverified revision.
            self.assertIn("HEAD", str(raised.exception))

    def test_the_importer_refuses_a_directory_that_is_not_a_checkout(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(SystemExit) as raised:
                self.import_script.main(["--upstream", directory, "--dry-run"])
            self.assertIn("not a git checkout", str(raised.exception))


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
        tool = (REPO_ROOT / TOOL).read_text(encoding="utf-8")
        pinned = re.search(r'defaultSnapshotSHA256\s*=\s*"([0-9a-f]{64})"', tool)
        self.assertIsNotNone(pinned, "the carried snapshot tool no longer pins a SHA-256")
        recorded = next(record.digest for record in manifest.inputs if record.path == SNAPSHOT)
        self.assertEqual(recorded, pinned.group(1))
        for path in ("cmd/ccc/", "internal/ccc/", "docs/plans/", "scripts/tests/"):
            self.assertTrue(manifest.is_owned(path), path)


if __name__ == "__main__":
    unittest.main()
