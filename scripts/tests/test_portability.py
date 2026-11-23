#!/usr/bin/env python3
"""Regression tests for scripts/portability.py, the gate's ``portability`` step.

The step's subject is a *shape*: this repository's own module, built and tested against a pristine
checkout of the pin instead of against the carry in the tree. A step nobody can re-run is a claim, so
every rule it rests on gets a case here -- the checkout it is handed is clean at the pin, the module's
one edge is rewritten in a scratch file (never in the tree), the Go commands carry the gate's own
targets and flags and run in the module's own directory, a build or test failure is a failure, and a
tree or a host it cannot answer for is disclosed as INCOMPLETE rather than passed. A tree whose module
is the repository root (the layout this repository had before the carry moved into a directory) is that
last shape, and a case below pins it.

Every case builds a throw-away tree and drives the shipped module in-process through its own seams
(``run``, ``fetch_checkout``, ``default_cache_root``, ``gate_module``), so no case needs a git binary,
a Go toolchain, a network or a C compiler, and every case holds on every platform and every Python 3
the repository documents.
"""

from __future__ import annotations

import hashlib
import importlib.util
import io
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import ExitStack, redirect_stdout
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]
REPO_ROOT = SCRIPTS.parent
SCRIPT_DIRS = (str(SCRIPTS),)

#: A revision-shaped pin; the digits do not matter to any rule here.
PIN = "0123456789abcdef0123456789abcdef01234567"
REMOTE = "https://example.invalid/agentsview"
MANIFEST = "scripts/upstream-manifest.txt"
SNAPSHOT = "internal/pricing/snapshot/litellm_snapshot.json.gz"
SNAPSHOT_BYTES = b"snapshot-bytes\n"

CARRY_GO_MOD = "module go.kenn.io/agentsview\n\ngo 1.27.4\n"
OUR_GO_MOD = (
    "module go.kenn.io/agentsview/ccc\n"
    "\n"
    "go 1.27.4\n"
    "\n"
    "require go.kenn.io/agentsview v0.0.0-00010101000000-000000000000\n"
    "\n"
    "replace go.kenn.io/agentsview => ../agentsview\n"
)
#: What a ``go test`` run's stdout holds for one passing package -- what the checker counts.
GO_TEST_OK = "ok  \tgo.kenn.io/agentsview/ccc/internal/ccc\t0.01s\n"


def tool_result(cmd, rc=0, out="", err=""):
    """A ``CompletedProcess`` shaped like the ones the shipped runner returns."""
    return subprocess.CompletedProcess(cmd, rc, out, err)


def load_script(name: str):
    """Import a shipped script by path, with scripts/ on sys.path for its own imports."""
    for directory in SCRIPT_DIRS:
        if directory not in sys.path:
            sys.path.insert(0, directory)
    spec = importlib.util.spec_from_file_location(
        f"shipped_{name.replace('.', '_').replace('-', '_')}", SCRIPTS / name)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def scratch_tree() -> Path:
    """A throw-away tree, absolute and outside the repository: the shipped checker's own rule."""
    return load_script("portability.py").scratch_directory("portability-root-")


class GateStub:
    """``scripts/gate.py``'s three answers, as the portability checker reads them.

    The real module is what is read in a shipped run (``portability.gate_module``); a case that is not
    about those answers hands this in instead, so it needs neither a real ``gate.py`` in its tree nor a
    Go toolchain.
    """

    def __init__(self, targets=("./...",), flags=("-tags", "fts5"), boundary=None):
        self.targets = list(targets)
        self.flags = list(flags)
        self.boundary = boundary

    def go_targets(self):
        return list(self.targets)

    def go_flags(self):
        return list(self.flags)

    def go_env(self):
        return {"GOFLAGS": "-mod=mod", "CGO_ENABLED": "1"}, self.boundary


class PortabilityTree:
    """One throw-away tree in the layout this check is about, plus the checkout beside it.

    The tree is the directory layout: ``agentsview/go.mod`` declares the carried module and
    ``ccc/go.mod`` this repository's own, which is what makes ``detect_layout`` answer the shape this
    check is for. The checkout beside a case's tree is a plain directory with a ``.git`` and the
    carried module's own ``go.mod``: every git answer is scripted, so no case shells out.
    """

    def __init__(self):
        self.root = scratch_tree()
        self.manifest_script = load_script("upstream_manifest.py")
        self.write("agentsview/go.mod", CARRY_GO_MOD)
        self.write("ccc/go.mod", OUR_GO_MOD)
        self.write("ccc/go.sum", "github.com/example/dep v1.0.0 h1:AAAA\n")
        self.write("ccc/internal/ccc/x.go", "package ccc\n")
        # The generated build input lives where the manifest's layout says: inside the carry.
        self.write_bytes("agentsview/" + SNAPSHOT, SNAPSHOT_BYTES)
        self.write_manifest()
        self.checkout = self.root / "checkout"
        (self.checkout / ".git").mkdir(parents=True)
        self.write_in_checkout("go.mod", CARRY_GO_MOD)
        # The build input upstream does not track, restored by the checkout itself (upstream's
        # Makefile does that before every Go-compiling target): a caller's checkout is never written
        # to, so it has to hold it already -- and as bytes, because its digest is the value the
        # manifest pins.
        self.write_to(self.checkout, SNAPSHOT, SNAPSHOT_BYTES)

    def cleanup(self) -> None:
        shutil.rmtree(self.root, ignore_errors=True)

    def write(self, relative: str, text: str) -> None:
        self.write_to(self.root, relative, text.encode("utf-8"))

    def write_in_checkout(self, relative: str, text: str) -> None:
        self.write_to(self.checkout, relative, text.encode("utf-8"))

    def write_bytes(self, relative: str, data: bytes) -> None:
        """One file whose *bytes* are a value something pins: the generated build input.

        ``write_text`` is the wrong tool for this one path. Text mode translates ``\\n`` to the
        platform's own line separator on Windows, and this file's bytes are exactly what the manifest
        pins and what ``portability.restore_inputs`` re-hashes, so a tree whose input went through
        text mode is a tree whose digest stops matching its record on that host alone: measured by
        emulating the write side, seven acceptance cases turned into ``PortabilityError``s and four
        refusals asserted the wrong error, on this host, with nothing else changed.
        """
        self.write_to(self.root, relative, data)

    @staticmethod
    def write_to(root: Path, relative: str, data: bytes) -> None:
        target = root / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)

    def write_manifest(self) -> None:
        digest = hashlib.sha256(SNAPSHOT_BYTES).hexdigest()
        lines = [
            "# generated",
            f"pin {PIN}",
            f"remote {REMOTE}",
            "describe v0-test",
            "commit-date 2026-01-01T00:00:00+00:00",
            "upstream-files 1",
            "carried 1",
            "deviations 1",
            "layout agentsview/ ccc",
            "file\t100644\t" + "0" * 40 + "\t14\tinternal/parser/parser.go",
            f"input\t100644\t{digest}\t{len(SNAPSHOT_BYTES)}\t{SNAPSHOT}\tgenerated build input",
        ]
        self.write(MANIFEST, "\n".join(lines) + "\n")

    def scripted(self, *, head=PIN, status="", build_rc=0, test_rc=0,
                 build_out="", test_out=GO_TEST_OK):
        """A scripted ``run``: git answers the checkout's own questions, Go answers each subcommand.

        The build's answer and the test's are separate, so a case can watch a build fail without a
        test ever running, and the other way round. Every command, and the directory it ran in, is
        recorded, so a case can assert *where* a command ran as well as what it was.
        """
        seen: list[tuple[str, list[str]]] = []

        def run(cmd, cwd, env=None, timeout=3600):
            seen.append((str(cwd), list(cmd)))
            if cmd[0] == "git":
                if "rev-parse" in cmd:
                    return tool_result(cmd, 0, head + "\n", "")
                return tool_result(cmd, 0, status, "")
            if cmd[0] == "go":
                if cmd[1] == "test":
                    return tool_result(cmd, test_rc, test_out, "")
                return tool_result(cmd, build_rc, build_out, "")
            return tool_result(cmd, 0, "", "")

        return run, seen

    def patched(self, stack: ExitStack, module, run, *, supplied, keep=False,
                gate=None, fetch=None, which="/usr/bin/go", cache=None):
        """Patch the checker's seams for one case and hand back the call to make.

        ``which`` answers the Go toolchain probe (a host without ``go`` is one of the disclosed shapes
        and has its own case); ``cache`` is the directory the checker would fetch into.
        """
        stack.enter_context(mock.patch.object(module, "run", run))
        stack.enter_context(mock.patch.object(module, "ROOT", self.root))
        stack.enter_context(mock.patch.object(module, "default_cache_root",
                                            lambda: cache or (self.root / "cache")))
        stack.enter_context(mock.patch.object(module.shutil, "which", return_value=which))
        if gate is not None:
            stack.enter_context(mock.patch.object(module, "gate_module", lambda *args: gate))
        if fetch is not None:
            stack.enter_context(mock.patch.object(module, "fetch_checkout", fetch))
        return lambda: module.check(self.root, supplied, keep)


class CleanCheckoutCase(unittest.TestCase):
    """The pass: this module, the pin's own bytes beside it, and the scratch file that aims the edge."""

    def setUp(self) -> None:
        self.module = load_script("portability.py")
        self.tree = PortabilityTree()
        self.addCleanup(self.tree.cleanup)
        self.gate = GateStub()

    def check(self, stack, run, **kwargs):
        return self.tree.patched(stack, self.module, run, gate=self.gate, **kwargs)()

    def test_a_clean_checkout_at_the_pin_passes(self) -> None:
        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            lines = self.check(stack, run, supplied=self.tree.checkout)
        self.assertEqual([line for line in lines if line.startswith("pin")][0],
                         f"pin {PIN} (v0-test)")
        self.assertTrue(any("built and tested against" in line for line in lines), lines)
        self.assertEqual([line for line in lines if line.endswith("package(s) ok")],
                         ["1 package(s) ok"])

    def test_the_go_commands_carry_the_gate_s_own_targets_and_flags_and_run_in_the_module(self) -> None:
        """The gate's answers, the scratch modfile, and the module's own directory: whole argv, pinned."""
        targets = ("./cmd/ccc/...", "./internal/ccc/...")
        self.gate = GateStub(targets=targets, flags=("-tags", "fts5"))
        run, seen = self.tree.scripted()
        with ExitStack() as stack:
            lines = self.check(stack, run, supplied=self.tree.checkout)
        self.assertTrue(any(line.endswith("package(s) ok") for line in lines), lines)
        go = [(cwd, cmd) for cwd, cmd in seen if cmd[0] == "go"]
        self.assertEqual(len(go), 2, go)
        build, test = [cmd for _, cmd in go]
        modfile = build[3]
        self.assertTrue(modfile.endswith("portability.mod"), build)
        self.assertEqual(build, ["go", "build", "-modfile", modfile, "-tags", "fts5", *targets])
        self.assertEqual(test, ["go", "test", "-count=1", "-modfile", modfile, "-tags", "fts5",
                                *targets])
        for cwd, _ in go:
            self.assertEqual(Path(cwd), self.tree.root / "ccc")

    def test_the_go_commands_are_handed_the_gate_module_read_from_the_tree_under_check(self) -> None:
        """The knobs are the gate's own readers, pointed at the tree being checked -- not copies here."""
        for name in ("gate.py", "upstream_manifest.py"):
            target = self.tree.root / "scripts" / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(SCRIPTS / name, target)
        self.tree.write("scripts/gate-targets.txt", "./internal/ccc/...\n")
        run, seen = self.tree.scripted()
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(sys, "dont_write_bytecode", True))
            lines = self.tree.patched(stack, self.module, run, supplied=self.tree.checkout)()
        self.assertTrue(any(line.endswith("package(s) ok") for line in lines), lines)
        go = [cmd for _, cmd in seen if cmd[0] == "go"]
        self.assertEqual(len(go), 2, go)
        for cmd in go:
            self.assertIn("./internal/ccc/...", cmd)
            self.assertIn("-modfile", cmd)

    def test_the_repository_is_not_written_to_and_the_scratch_file_is_removed(self) -> None:
        """Nothing in the tree moves: the alternate ``go.mod`` lives in the scratch and goes away."""
        before = {path: path.read_bytes() for path in self.tree.root.rglob("*") if path.is_file()}
        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            self.check(stack, run, supplied=self.tree.checkout)
        after = {path: path.read_bytes() for path in self.tree.root.rglob("*") if path.is_file()}
        self.assertEqual(before, after)

    def test_the_alternate_go_mod_aims_the_edge_at_the_checkout(self) -> None:
        """The scratch file is the module's own with exactly one line rewritten -- the replace."""
        module = self.module
        seen: list[Path] = []
        original = module.rewrite_modfile

        def recording(module_file, target, scratch):
            result = original(module_file, target, scratch)
            seen.append(scratch)
            return result

        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(module, "rewrite_modfile", recording))
            lines = self.check(stack, run, supplied=self.tree.checkout, keep=True)
        self.assertEqual(len(seen), 1)
        scratch = seen[0]
        try:
            text = (scratch / "portability.mod").read_text(encoding="utf-8")
            self.assertIn(f"replace go.kenn.io/agentsview => {self.tree.checkout}", text)
            self.assertIn("module go.kenn.io/agentsview/ccc", text)
            self.assertEqual((scratch / "portability.sum").read_text(encoding="utf-8"),
                             (self.tree.root / "ccc" / "go.sum").read_text(encoding="utf-8"))
        finally:
            shutil.rmtree(scratch, ignore_errors=True)
        self.assertTrue(any("portability.mod" in line for line in lines), lines)
        self.assertTrue((self.tree.root / "ccc" / "go.mod").read_text(encoding="utf-8")
                        .endswith("replace go.kenn.io/agentsview => ../agentsview\n"))

    def test_the_scratch_file_is_removed_again_unless_keep_says_otherwise(self) -> None:
        module = self.module
        seen: list[Path] = []
        original = module.rewrite_modfile

        def recording(module_file, target, scratch):
            result = original(module_file, target, scratch)
            seen.append(scratch)
            return result

        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            stack.enter_context(mock.patch.object(module, "rewrite_modfile", recording))
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertEqual(len(seen), 1)
        self.assertFalse(seen[0].exists(), "the scratch directory outlived the run")

    def test_a_fetched_checkout_is_cached_and_gets_the_generated_input(self) -> None:
        """A checkout this run fetched itself is written to: the input upstream's Makefile restores."""
        module = self.module
        cache = self.tree.root / "cache"

        def fetch(directory, remote, pin):
            (directory / ".git").mkdir(parents=True)
            (directory / "go.mod").write_text(CARRY_GO_MOD, encoding="utf-8")
            return None

        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            lines = self.check(stack, run, supplied=None, fetch=fetch, cache=cache)
        self.assertTrue((cache / PIN / SNAPSHOT).exists(), "the fetched checkout got the input")
        self.assertTrue(any("wrote " + SNAPSHOT in line for line in lines), lines)
        self.assertTrue(any(line.startswith("fetched the pin into") for line in lines), lines)


class RefusalCase(unittest.TestCase):
    """What the checker refuses: a checkout it cannot attribute, and a failure it must not hide."""

    def setUp(self) -> None:
        self.module = load_script("portability.py")
        self.tree = PortabilityTree()
        self.addCleanup(self.tree.cleanup)

    def check(self, stack, run, **kwargs):
        return self.tree.patched(stack, self.module, run, gate=GateStub(), **kwargs)()

    def test_a_supplied_checkout_at_another_revision_is_a_failure(self) -> None:
        run, _ = self.tree.scripted(head="f" * 40)
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("not the pinned", str(raised.exception))

    def test_a_supplied_checkout_with_local_modifications_is_a_failure(self) -> None:
        run, _ = self.tree.scripted(status=" M internal/parser/parser.go\n")
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("local modifications", str(raised.exception))

    def test_a_supplied_directory_that_is_not_a_checkout_is_a_failure(self) -> None:
        plain = self.tree.root / "not-a-checkout"
        plain.mkdir()
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=plain)
        self.assertIn("not a git checkout", str(raised.exception))

    def test_a_build_failure_is_a_failure_naming_the_command(self) -> None:
        run, _ = self.tree.scripted(build_rc=1, build_out="cannot find package\n")
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        message = str(raised.exception)
        self.assertIn("go build", message)
        self.assertIn("rc=1", message)
        self.assertIn("cannot find package", message)

    def test_a_test_failure_is_a_failure_naming_the_command(self) -> None:
        """A ``--- FAIL`` reaches the checker as the run's own rc, and rc is what is read."""
        run, _ = self.tree.scripted(test_rc=1, test_out="--- FAIL: TestX (0.01s)\n")
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("go test", str(raised.exception))

    def test_a_module_without_the_layouts_edge_is_a_failure(self) -> None:
        """Nothing to aim: the module no longer points the carried module at a directory beside it."""
        self.tree.write("ccc/go.mod", "module go.kenn.io/agentsview/ccc\n\ngo 1.27.4\n")
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("replace go.kenn.io/agentsview => ../agentsview", str(raised.exception))

    def test_a_module_without_a_sum_file_is_a_failure(self) -> None:
        (self.tree.root / "ccc" / "go.sum").unlink()
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("go.sum", str(raised.exception))

    def test_a_caller_checkout_without_the_generated_input_is_refused_not_written_to(self) -> None:
        """A checkout this run did not fetch is never written to; the restore command is named."""
        (self.tree.checkout / SNAPSHOT).unlink()
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("litellm-snapshot -restore", str(raised.exception))
        self.assertFalse((self.tree.checkout / SNAPSHOT).exists(), "the caller's tree was written to")

    def test_an_input_that_is_not_the_pinned_artifact_is_a_failure(self) -> None:
        """The copy this tree provides is checked against the manifest's own digest."""
        (self.tree.root / "agentsview" / SNAPSHOT).write_bytes(b"not the pinned bytes\n")
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityError) as raised:
            self.check(stack, run, supplied=None, fetch=lambda *args: None, cache=None)
        self.assertIn("the manifest records", str(raised.exception))


class IncompleteCase(unittest.TestCase):
    """The shapes the checker discloses instead of passing: no checkout, no toolchain, a boundary."""

    def setUp(self) -> None:
        self.module = load_script("portability.py")
        self.tree = PortabilityTree()
        self.addCleanup(self.tree.cleanup)

    def check(self, stack, run, **kwargs):
        return self.tree.patched(stack, self.module, run, gate=GateStub(), **kwargs)()

    def test_a_tree_whose_module_is_the_repository_root_is_disclosed(self) -> None:
        """The layout this repository had before the move has no ``ccc/`` module to re-aim."""
        root = scratch_tree()
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        (root / MANIFEST).parent.mkdir(parents=True, exist_ok=True)
        (root / MANIFEST).write_text(f"pin {PIN}\nremote {REMOTE}\n", encoding="utf-8")
        (root / "go.mod").write_text(CARRY_GO_MOD, encoding="utf-8")
        run, _ = self.tree.scripted()
        with mock.patch.object(self.module, "run", run), \
                mock.patch.object(self.module, "gate_module", lambda *args: GateStub()):
            with self.assertRaises(self.module.PortabilityIncomplete) as raised:
                self.module.check(root, None, False)
        self.assertIn("repository root", raised.exception.summary)

    def test_a_missing_go_toolchain_is_disclosed(self) -> None:
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityIncomplete) as raised:
            self.check(stack, run, supplied=self.tree.checkout, which=None)
        self.assertIn("PATH", raised.exception.summary)

    def test_the_platform_boundary_is_disclosed_with_the_gate_s_own_reason(self) -> None:
        """On Windows without a C compiler the Go steps are excused there too, with that reason."""
        boundary = "no C compiler on PATH (CGO/SQLite steps cannot run on this host)"
        run, _ = self.tree.scripted()
        with ExitStack() as stack:
            call = self.tree.patched(stack, self.module, run, supplied=self.tree.checkout,
                                     gate=GateStub(boundary=boundary))
            with self.assertRaises(self.module.PortabilityIncomplete) as raised:
                call()
        self.assertEqual(raised.exception.summary, boundary)

    def test_no_readable_manifest_is_disclosed(self) -> None:
        """The pin to check against is unknowable without the ledger; that is not a pass."""
        (self.tree.root / MANIFEST).unlink()
        run, _ = self.tree.scripted()
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityIncomplete) as raised:
            self.check(stack, run, supplied=self.tree.checkout)
        self.assertIn("carry manifest", raised.exception.summary)

    def test_a_cache_that_is_not_at_the_pin_is_named_and_never_deleted(self) -> None:
        """A stale cache may be somebody's working copy: the answer names it instead of removing it."""
        cache = self.tree.root / "cache" / PIN
        (cache / ".git").mkdir(parents=True)
        (cache / "go.mod").write_text(CARRY_GO_MOD, encoding="utf-8")
        run, _ = self.tree.scripted(head="e" * 40)
        with ExitStack() as stack, self.assertRaises(self.module.PortabilityIncomplete) as raised:
            self.check(stack, run, supplied=None)
        self.assertIn(str(cache), raised.exception.summary)
        self.assertTrue(cache.exists(), "the cache was deleted")

    def test_a_fetch_that_fails_is_disclosed_with_what_it_tried(self) -> None:
        def failing(cmd, cwd, env=None, timeout=3600):
            if cmd[0] == "git" and cmd[1] == "clone":
                return tool_result(cmd, 128, "", "fatal: could not read from remote\n")
            return tool_result(cmd, 0, "", "")

        with ExitStack() as stack, self.assertRaises(self.module.PortabilityIncomplete) as raised:
            self.check(stack, failing, supplied=None)
        self.assertIn("git clone", raised.exception.summary)
        self.assertIn("could not read from remote", raised.exception.summary)

    def test_a_cached_checkout_at_the_pin_is_used_without_fetching(self) -> None:
        cache = self.tree.root / "cache" / PIN
        (cache / ".git").mkdir(parents=True)
        (cache / "go.mod").write_text(CARRY_GO_MOD, encoding="utf-8")
        (cache / SNAPSHOT).parent.mkdir(parents=True, exist_ok=True)
        (cache / SNAPSHOT).write_bytes(SNAPSHOT_BYTES)
        run, seen = self.tree.scripted()
        with ExitStack() as stack:
            lines = self.check(stack, run, supplied=None)
        self.assertFalse([cmd for _, cmd in seen if cmd[:2] == ["git", "clone"]], "it fetched anyway")
        self.assertTrue(any(line.endswith("package(s) ok") for line in lines), lines)

    def test_main_prints_the_incomplete_verdict_and_exits_2(self) -> None:
        """The gate reads this exit code as a disclosed SKIP, so it must be 2 and it must print."""
        root = scratch_tree()
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        module = load_script("portability.py")
        out = io.StringIO()
        with mock.patch.object(module, "ROOT", root), redirect_stdout(out):
            code = module.main([])
        self.assertEqual(code, 2, out.getvalue())
        self.assertIn("portability: INCOMPLETE", out.getvalue())


class ScratchDirectoryCase(unittest.TestCase):
    """The scratch tree must stay outside the repository -- the ``parity-root-*`` leak's own rule."""

    def setUp(self) -> None:
        self.module = load_script("portability.py")
        self.tree = PortabilityTree()
        self.addCleanup(self.tree.cleanup)
        self.addCleanup(setattr, tempfile, "tempdir", tempfile.tempdir)

    def test_a_temp_root_inside_the_tree_is_refused_and_a_platform_one_taken(self) -> None:
        inside = self.tree.root / "scratch-tmp"
        inside.mkdir()
        tempfile.tempdir = str(inside)
        with mock.patch.object(self.module, "ROOT", self.tree.root):
            path = self.module.scratch_directory("ccc-portability-")
        self.addCleanup(shutil.rmtree, path, ignore_errors=True)
        self.assertNotEqual(path, self.tree.root)
        self.assertNotIn(self.tree.root, path.parents)
        self.assertFalse(str(path).startswith(str(inside)))

    def test_a_relative_temp_root_does_not_put_the_scratch_under_the_current_directory(self) -> None:
        """``TMPDIR=scratch`` is what put 81 ``parity-root-*`` directories in the repository root."""
        tempfile.tempdir = "relative-scratch"
        with mock.patch.object(self.module, "ROOT", self.tree.root):
            path = self.module.scratch_directory("ccc-portability-")
        self.addCleanup(shutil.rmtree, path, ignore_errors=True)
        self.assertTrue(path.is_absolute(), path)
        self.assertNotIn(self.tree.root, path.parents)

    def test_the_platforms_own_scratch_bases_are_pinned_for_both_answers(self) -> None:
        """The fallback list has two answers, and both are pinned here rather than read off this host.

        POSIX names its scratch roots; Windows has none of its own to name (``Path("/tmp")`` hangs off
        whichever drive the process is on, so a list of those is empty there and the two cases above
        -- which take the environment's own answer away -- could not produce a scratch at all), and
        its answer is therefore the variables ``tempfile`` itself reads, asked again. Driving the
        platform seam for both answers is what keeps this case the rule's answer rather than this
        machine's: a fallback that only worked on the host the suite happens to run on is exactly the
        defect the two cases above exist to catch, one layer down.
        """
        module = self.module
        windows_temp = Path(tempfile.gettempdir()) / "platform-temp"
        with mock.patch.dict(os.environ, {"TEMP": str(windows_temp),
                                          "TMP": str(windows_temp / "alt")}):
            with mock.patch.object(module, "platform_name", lambda: "nt"):
                self.assertEqual(module.platform_scratch_bases(),
                                 [windows_temp, windows_temp / "alt"])
            with mock.patch.object(module, "platform_name", lambda: "posix"):
                self.assertEqual(module.platform_scratch_bases(), [Path("/tmp"), Path("/var/tmp")])


class ScratchTreeCase(unittest.TestCase):
    """The suite's own throw-away trees stay outside the repository: the same rule, as it is used."""

    def setUp(self) -> None:
        self.addCleanup(setattr, tempfile, "tempdir", tempfile.tempdir)

    def test_a_temp_root_inside_the_repository_is_refused(self) -> None:
        inside = REPO_ROOT / "scratch-tmp-for-portability-cases"
        inside.mkdir(exist_ok=True)
        try:
            tempfile.tempdir = str(inside)
            root = scratch_tree()
        finally:
            shutil.rmtree(inside, ignore_errors=True)
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        self.assertTrue(root.is_absolute(), root)
        self.assertNotEqual(root, REPO_ROOT)
        self.assertNotIn(REPO_ROOT, root.parents)
        self.assertNotEqual(root.parent, inside)

    def test_a_relative_temp_root_does_not_land_under_the_current_directory(self) -> None:
        """``TMPDIR=scratch`` is what put 81 ``parity-root-*`` directories in the repository root."""
        tempfile.tempdir = "relative-portability-scratch"
        root = scratch_tree()
        self.addCleanup(shutil.rmtree, root, ignore_errors=True)
        self.assertTrue(root.is_absolute(), root)
        self.assertNotIn(REPO_ROOT, root.parents)


class ManifestLayoutCase(unittest.TestCase):
    """The shared layout answer: what makes the directory shape detectable at all."""

    def setUp(self) -> None:
        self.tree = PortabilityTree()
        self.addCleanup(self.tree.cleanup)
        self.manifest = self.tree.manifest_script

    def test_the_two_go_mod_files_are_what_detects_the_directory_layout(self) -> None:
        layout = self.manifest.detect_layout(self.tree.root)
        self.assertEqual((layout.carry, layout.module), ("agentsview/", "ccc"))
        self.assertEqual(layout.spelling, "agentsview/ ccc")
        self.assertEqual(layout.tree_path("internal/parser/x.go"), "agentsview/internal/parser/x.go")
        self.assertTrue(layout.owns("ccc/cmd/ccc/main.go"))
        self.assertFalse(layout.owns("agentsview/internal/parser/x.go"))

    def test_a_tree_with_neither_shape_answers_the_root_layout(self) -> None:
        empty = scratch_tree()
        self.addCleanup(shutil.rmtree, empty, ignore_errors=True)
        layout = self.manifest.detect_layout(empty)
        self.assertEqual((layout.carry, layout.module), ("", "."))
        self.assertEqual(layout.tree_path("internal/parser/x.go"), "internal/parser/x.go")

    def test_a_layout_line_that_is_not_two_tokens_is_a_malformed_manifest(self) -> None:
        with self.assertRaises(self.manifest.ManifestError):
            self.manifest.parse_manifest(f"pin {PIN}\nlayout agentsview\n")
        with self.assertRaises(self.manifest.ManifestError):
            self.manifest.parse_manifest(f"pin {PIN}\nlayout /absolute ccc\n")

    def test_the_layout_line_round_trips_through_the_rendered_manifest(self) -> None:
        text = (f"pin {PIN}\nlayout agentsview/ ccc\n"
                "file\t100644\t" + "0" * 40 + "\t14\tinternal/parser/parser.go\n")
        again = self.manifest.parse_manifest(self.manifest.parse_manifest(text).render())
        self.assertEqual((again.layout.carry, again.layout.module), ("agentsview/", "ccc"))
        self.assertEqual(again.tree_path("internal/parser/parser.go"),
                         "agentsview/internal/parser/parser.go")

    def test_owned_records_still_work_inside_the_directory_layout(self) -> None:
        """A ledger of the directory shape may still name an owned prefix, and the boundary adds more."""
        text = (f"pin {PIN}\nlayout agentsview/ ccc\nowned\tccc/cmd/ccc/\n"
                "file\t100644\t" + "0" * 40 + "\t14\tinternal/parser/parser.go\n")
        manifest = self.manifest.parse_manifest(text)
        self.assertTrue(manifest.is_owned("ccc/cmd/ccc/main.go"))
        self.assertTrue(manifest.is_owned("scripts/gate.py"))       # outside the carry: ours
        self.assertFalse(manifest.is_owned("agentsview/docs/harness.md"))


if __name__ == "__main__":
    unittest.main()
