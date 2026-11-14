#!/usr/bin/env python3
"""Regression tests for scripts/gate.py's verdict logic.

The gate is the only thing standing between an increment and `main`, so its own
rules need a test: a required step that did not run must fail the gate, a step
that merely could not run must be named in the verdict instead of reading as a
pass, and a tool step that found a real problem (a failing parity hook, a leak
report) must stay a FAIL instead of degrading into the disclosed SKIP.

Most cases build a throw-away tree and run the shipped gate as a subprocess with
``CALLCCC_GATE_ROOT`` pointed at it and ``PATH`` pinned to a stub bin, so no
verdict depends on which tools this machine happens to have installed.
``GateProbeCase`` is the exception: it drives the shipped file in-process with
one probe patched, because it asserts statuses an installed toolchain cannot
produce here.
"""

from __future__ import annotations

import importlib.util
import io
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest import mock

GATE = Path(__file__).resolve().parents[1] / "gate.py"


def load_shipped_gate():
    """Import scripts/gate.py as a module so main() can be driven in-process."""
    spec = importlib.util.spec_from_file_location("gate_under_test", GATE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def tool_result(cmd, rc=0, out="", err=""):
    """A ``CompletedProcess`` shaped like the ones the gate's own ``run()`` returns."""
    return subprocess.CompletedProcess(cmd, rc, out, err)


class GateTree:
    """One throw-away tree, one stub bin and one pinned ``PATH`` per case.

    Fixtures only, so both case classes below can use them without one class
    running the other's tests.
    """

    def setUp(self) -> None:
        self.root = Path(tempfile.mkdtemp(prefix="gate-root-"))
        (self.root / "scripts").mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.stub_bin = self.root / "stub-bin"
        self.stub("gofmt")  # so gofmt's own absence is never the failing reason

    def stub(self, name: str, exit_code: int = 0, where: Path | None = None) -> None:
        """Make ``name`` findable on a pinned ``PATH`` without installing a tool.

        No case in this suite executes a stub: the cases that need a tool to
        answer with a specific exit code patch the gate's own ``run`` instead.
        A stub therefore only has to be *found*, which is why it is written
        twice -- as a POSIX script and as a Windows ``.bat`` (``shutil.which``
        resolves that one through ``PATHEXT``) -- and why no stub is a binary.
        """
        directory = where or self.stub_bin
        directory.mkdir(parents=True, exist_ok=True)
        script = directory / name
        script.write_text(f"#!/bin/sh\nexit {exit_code}\n", encoding="utf-8")
        script.chmod(0o755)
        (directory / f"{name}.bat").write_text(f"@exit /b {exit_code}\r\n", encoding="utf-8")

    def write(self, relative: str, text: str) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def write_sample_test(self, passes: bool = True) -> None:
        """One unittest file for the selftest step to discover and run."""
        self.write(
            "scripts/tests/test_sample.py",
            "import unittest\n\n"
            "class T(unittest.TestCase):\n"
            f"    def test_sample(self):\n        self.assertTrue({passes})\n",
        )

    def no_go_tools(self) -> Path:
        """A bin dir holding no Go toolchain.

        The ``gcc`` stub keeps the Windows-only "no C compiler" boundary out of
        the way: the rule under test is the missing Go toolchain, which is a
        failure on every platform.
        """
        directory = self.root / "no-go-tools"
        self.stub("gcc", where=directory)
        return directory

    def gate(self, env: dict[str, str] | None = None) -> subprocess.CompletedProcess:
        """Run the shipped gate against this tree with ``PATH`` pinned to the stub bin."""
        merged = os.environ.copy()
        merged["PATH"] = str(self.stub_bin)
        merged["CALLCCC_GATE_ROOT"] = str(self.root)
        merged.update(env or {})
        return subprocess.run(
            [sys.executable, str(GATE)],
            capture_output=True, text=True, env=merged, cwd=str(self.root), timeout=600,
        )


class GateCase(GateTree, unittest.TestCase):
    """Subprocess cases: the shipped gate runs for real against a throw-away tree."""

    def verdict(self, stdout: str) -> str:
        """The single ``GATE:`` line of a run."""
        lines = [line for line in stdout.splitlines() if line.startswith("GATE:")]
        self.assertEqual(len(lines), 1, stdout)
        return lines[0]

    def test_required_step_that_skips_fails_the_gate(self) -> None:
        """A required step reported as N/A/SKIP/BOUNDARY must fail, not footnote."""
        self.write("scripts/gate-required.txt", "parity\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("GATE: FAIL", proc.stdout)
        self.assertIn("parity", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_required_go_step_without_sources_fails_the_gate(self) -> None:
        self.write("scripts/gate-required.txt", "gofmt\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("REQUIRED but was SKIP", proc.stdout)

    def test_required_secrets_step_when_gitleaks_is_absent_fails(self) -> None:
        """SKIP must count as not-PASS for a required step, whatever the cause.

        The stub bin supplies ``gofmt`` and nothing else, so ``gitleaks`` is
        unreachable even on a host that has it installed.
        """
        self.write("scripts/gate-required.txt", "secrets\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("REQUIRED but was SKIP", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_unknown_required_step_name_fails_the_gate(self) -> None:
        self.write("scripts/gate-required.txt", "paritty\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("unknown step", proc.stdout)

    def test_unrequired_steps_that_cannot_run_are_disclosed_not_failed(self) -> None:
        """Every non-PASS status, N/A included, is named in the verdict line."""
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn("GATE: PASS (partial:", proc.stdout)
        self.assertIn("parity", self.verdict(proc.stdout))

    def test_comments_and_blank_lines_are_ignored(self) -> None:
        self.write("scripts/gate-required.txt", "# nothing is required yet\n\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_runs_the_trees_own_tests(self) -> None:
        self.write_sample_test()
        proc = self.gate()
        self.assertIn("PASS", proc.stdout)
        self.assertIn("selftest", proc.stdout)
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_fails_on_a_failing_test(self) -> None:
        self.write_sample_test(passes=False)
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("selftest", proc.stdout)

    def test_parity_hook_that_fails_fails_the_gate(self) -> None:
        """A hook that exists and exits non-zero is a FAIL, never the absent-hook N/A."""
        self.write("scripts/upstream-parity.py", "import sys\n\nsys.exit(1)\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("upstream-parity.py -> rc=1", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_missing_gofmt_fails_the_gate(self) -> None:
        """No toolchain on PATH fails on Linux and macOS, naming the tool it wanted."""
        proc = self.gate(env={"PATH": str(self.no_go_tools())})
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("gofmt not found (Go toolchain is required)", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_missing_go_fails_the_gate(self) -> None:
        """The same rule for the steps that need ``go`` itself, not just ``gofmt``."""
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        proc = self.gate(env={"PATH": str(self.no_go_tools())})
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("go not found on PATH", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)


class GateProbeCase(GateTree, unittest.TestCase):
    """Cases that drive the shipped module in-process with one host probe patched.

    A build boundary, a scanner that reports leaks, a tree without git and the
    exact argv a file step is handed are answers this machine cannot give, so
    these cases import the shipped file and call it directly: ``main()`` and
    every status rule behind it are the shipped ones, and ``PATH`` stays pinned
    to the stub bin, so a patched probe is the only reason the gate believes a
    tool exists.
    """

    def run_in_process(self, **probes) -> tuple[int, str]:
        """Call the shipped ``main()`` with ``PATH`` pinned and ``probes`` patched."""
        gate = load_shipped_gate()
        gate.ROOT = self.root
        out = io.StringIO()
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}), \
                mock.patch.multiple(gate, **probes), \
                redirect_stdout(out):
            rc = gate.main()
        return rc, out.getvalue()

    def test_required_step_reported_as_boundary_fails_the_gate(self) -> None:
        """BOUNDARY is a host limitation, not a pass, when the step is required."""
        self.write("scripts/gate-required.txt", "build\n")
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        rc, text = self.run_in_process(
            tracked_go_files=lambda: ["main.go"],
            run=lambda cmd, env=None, timeout=1800: tool_result(cmd),
            go_env=lambda: ({}, "no C compiler on PATH (stubbed)"),
        )
        self.assertEqual(rc, 1, text)
        self.assertIn("REQUIRED but was BOUNDARY", text)
        self.assertIn("GATE: FAIL", text)
        self.assertNotIn("GATE: PASS", text)

    def test_gitleaks_leak_report_fails_the_gate(self) -> None:
        """rc=7 is the leak result and a FAIL; any other non-zero rc stays the SKIP."""
        self.stub("gitleaks", exit_code=7)
        rc, text = self.run_in_process(
            run=lambda cmd, env=None, timeout=1800: (
                tool_result(cmd, 7, "Finding: ghp_redacted\n")
                if cmd[0] == "gitleaks"
                else tool_result(cmd)
            )
        )
        self.assertEqual(rc, 1, text)
        self.assertIn("gitleaks reported leaks", text)
        self.assertNotIn("GATE: PASS", text)

    def test_only_tracked_go_files_outside_vendor_reach_gofmt(self) -> None:
        """The git branch: tracked ``*.go``, minus a top-level ``vendor/``."""
        git = shutil.which("git")
        if git is None:
            self.skipTest("git is not on PATH: this case exercises the git branch")
        self.write("main.go", "package main\n")
        self.write("untracked.go", "package main\n")
        self.write("vendor/vendored.go", "package main\n")
        for args in (["init", "-q"], ["add", "main.go"]):
            done = subprocess.run([git, *args], cwd=self.root, capture_output=True, text=True)
            self.assertEqual(done.returncode, 0, f"git {args[0]}: {done.stdout}{done.stderr}")

        gate = load_shipped_gate()
        gate.ROOT = self.root
        recorded: list[list[str]] = []
        real_run = gate.run

        def recording(cmd, env=None, timeout=1800):
            if cmd[0] == "gofmt":
                recorded.append(list(cmd))
                return tool_result(cmd)
            return real_run(cmd, env, timeout)

        path = os.pathsep.join([str(self.stub_bin), str(Path(git).parent)])
        with mock.patch.dict(os.environ, {"PATH": path}), \
                mock.patch.object(gate, "run", side_effect=recording):
            found = gate.tracked_go_files()
            status, detail = gate.step_gofmt(found)

        self.assertEqual(found, ["main.go"])
        self.assertEqual(status, gate.PASS, detail)
        self.assertEqual(recorded, [["gofmt", "-l", "main.go"]])

    def test_tree_without_git_walks_go_files_minus_a_top_level_vendor(self) -> None:
        """The no-git branch drops the same things the git branch drops.

        Nothing in the stub bin answers to ``git``, so this is the walk a
        ``CALLCCC_GATE_ROOT`` copy of the tree takes: a nested ``vendor/`` is
        part of the module and stays, a top-level one is not and goes.
        """
        self.write("main.go", "package main\n")
        self.write("vendor/vendored.go", "package main\n")
        self.write("deep/vendor/nested.go", "package main\n")
        gate = load_shipped_gate()
        gate.ROOT = self.root
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}):
            found = gate.tracked_go_files()
        self.assertEqual(found, ["deep/vendor/nested.go", "main.go"])


if __name__ == "__main__":
    unittest.main()
