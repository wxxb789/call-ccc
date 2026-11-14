#!/usr/bin/env python3
"""Regression tests for scripts/gate.py's verdict logic.

The gate is the only thing standing between an increment and `main`, so its own
rules need a test: a required step that did not run must fail the gate, and a
step that merely could not run must not be reported as a pass.

Each case builds a throw-away tree and runs the real gate with
``CALLCCC_GATE_ROOT`` pointed at it, so the code under test is the shipped file,
not a copy.
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


class GateCase(unittest.TestCase):
    def setUp(self) -> None:
        self.root = Path(tempfile.mkdtemp(prefix="gate-root-"))
        (self.root / "scripts").mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)

    def write(self, relative: str, text: str) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def gate(self, *args: str) -> subprocess.CompletedProcess:
        env = os.environ.copy()
        env["CALLCCC_GATE_ROOT"] = str(self.root)
        return subprocess.run(
            [sys.executable, str(GATE), *args],
            capture_output=True, text=True, env=env, cwd=str(self.root), timeout=600,
        )

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

        PATH keeps a working ``gofmt`` so the failing verdict can only come from
        the required-step rule, not from a missing Go toolchain.
        """
        self.write("scripts/gate-required.txt", "secrets\n")
        fake_bin = self.root / "fake-bin"
        fake_bin.mkdir()
        gofmt = fake_bin / "gofmt"
        gofmt.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        gofmt.chmod(0o755)
        env = os.environ.copy()
        env["PATH"] = str(fake_bin) + os.pathsep + "/usr/bin" + os.pathsep + "/bin"
        env["CALLCCC_GATE_ROOT"] = str(self.root)
        proc = subprocess.run(
            [sys.executable, str(GATE)], capture_output=True, text=True,
            env=env, cwd=str(self.root), timeout=600,
        )
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("REQUIRED but was SKIP", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_unknown_required_step_name_fails_the_gate(self) -> None:
        self.write("scripts/gate-required.txt", "paritty\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("unknown step", proc.stdout)

    def test_unrequired_steps_that_cannot_run_are_disclosed_not_failed(self) -> None:
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        self.assertIn("GATE: PASS (partial:", proc.stdout)

    def test_comments_and_blank_lines_are_ignored(self) -> None:
        self.write("scripts/gate-required.txt", "# nothing is required yet\n\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_runs_the_trees_own_tests(self) -> None:
        self.write("scripts/tests/test_sample.py", "import unittest\n\n"
                    "class T(unittest.TestCase):\n"
                    "    def test_ok(self):\n        self.assertTrue(True)\n")
        proc = self.gate()
        self.assertIn("PASS", proc.stdout)
        self.assertIn("selftest", proc.stdout)
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_fails_on_a_failing_test(self) -> None:
        self.write("scripts/tests/test_sample.py", "import unittest\n\n"
                    "class T(unittest.TestCase):\n"
                    "    def test_bad(self):\n        self.assertTrue(False)\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertIn("selftest", proc.stdout)


class GateBoundaryCase(unittest.TestCase):
    """A required step reported as a platform boundary must fail too.

    BOUNDARY is only produced on a host that cannot run the step (Windows
    without a C compiler), so the host probe is the one thing stubbed here; the
    verdict math under test is still the shipped file's.
    """

    def test_required_step_reported_as_boundary_fails_the_gate(self) -> None:
        with tempfile.TemporaryDirectory(prefix="gate-boundary-") as tmp:
            root = Path(tmp)
            (root / "scripts").mkdir(parents=True)
            (root / "scripts" / "gate-required.txt").write_text("build\n", encoding="utf-8")
            (root / "go.mod").write_text("module example.com/throwaway\n\ngo 1.27\n", encoding="utf-8")
            (root / "main.go").write_text("package main\n", encoding="utf-8")

            gate = load_shipped_gate()
            gate.ROOT = root
            out = io.StringIO()
            with mock.patch.object(
                gate, "go_env", return_value=({}, "no C compiler on PATH (stubbed)")
            ), redirect_stdout(out):
                rc = gate.main()

        text = out.getvalue()
        self.assertEqual(rc, 1, text)
        self.assertIn("REQUIRED but was BOUNDARY", text)
        self.assertIn("GATE: FAIL", text)
        self.assertNotIn("GATE: PASS", text)


if __name__ == "__main__":
    unittest.main()
