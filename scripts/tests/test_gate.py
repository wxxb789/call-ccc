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
produce here. Both must hold on every Python 3 the repository documents
(``docs/conventions.md``), which is why no case inherits from ``PurePath``: that
class' argument handling is not the same before and after 3.12.
"""

from __future__ import annotations

import importlib.util
import io
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path, PureWindowsPath
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


class WindowsSpelledTree:
    """A root whose ``rglob`` answers with Windows-spelled children, without Windows.

    The path flavour is the whole difference a Windows host makes to a relative
    path: ``PureWindowsPath`` spells a nested file ``deep\\vendor\\nested.go``
    where a POSIX host spells it ``deep/vendor/nested.go``. The children are
    supplied rather than walked, so a case can drive the shipped walk on this
    host, and they are ``PureWindowsPath`` values, so ``relative_to`` and
    ``parts`` have the Windows flavour the shipped code relies on.

    Deliberately *not* a ``PureWindowsPath`` subclass: a subclass taking
    ``(root, children)`` only answers on 3.12 and later, where ``PurePath`` moved
    argument parsing from ``__new__`` into ``__init__``. On 3.11 the inherited
    ``__new__`` parses the children list as a path segment and raises
    ``TypeError`` before the subclass' own ``__init__`` runs, so the selftest
    would fail on an interpreter ``docs/conventions.md`` allows. This object
    therefore carries no minor-version requirement: it answers the two calls the
    shipped walk makes -- ``rglob``, and being the argument of a child's
    ``relative_to`` through ``__fspath__``.
    """

    def __init__(self, root: str, children: list[str]) -> None:
        self._root = PureWindowsPath(root)
        self._children = [PureWindowsPath(child) for child in children]

    def __fspath__(self) -> str:
        return str(self._root)

    def rglob(self, pattern: str) -> list[PureWindowsPath]:
        return [child for child in self._children if child.match(pattern)]


class GateTree:
    """One throw-away tree, one stub bin and one pinned ``PATH`` per case.

    Fixtures and read-the-printed-verdict helpers, so both case classes below
    can use them without one class running the other's tests.
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

    def verdict(self, stdout: str) -> str:
        """The single ``GATE:`` line of a run."""
        lines = [line for line in stdout.splitlines() if line.startswith("GATE:")]
        self.assertEqual(len(lines), 1, stdout)
        return lines[0]

    def step_line(self, stdout: str, name: str) -> tuple[str, str]:
        """The ``(status, detail)`` the run printed for step ``name``.

        A case that asserts a *status* has to read the step's own line: an
        ``assertIn`` on the detail text alone cannot tell a SKIP from a FAIL,
        and the verdict line names only the steps that did not run at all.
        """
        rows = [line.split(None, 2) for line in stdout.splitlines() if line.startswith("  ")]
        rows = [row for row in rows if len(row) >= 2 and row[1] == name]
        self.assertEqual(len(rows), 1, stdout)
        return rows[0][0], (rows[0][2] if len(rows[0]) > 2 else "")

    def partial_steps(self, stdout: str) -> list[str]:
        """The step names a ``GATE: PASS (partial: …)`` verdict discloses, in order.

        The statuses are asserted through ``step_line``; this helper answers the
        other half of the rule -- that the verdict names *every* step that did
        not run, so no SKIP, BOUNDARY or N/A can be dropped from it.
        """
        verdict = self.verdict(stdout)
        prefix, sep, rest = verdict.partition("GATE: PASS (partial: ")
        self.assertEqual(prefix, "", verdict)
        self.assertTrue(sep, verdict)
        names, sep, _ = rest.partition(" did not run")
        self.assertTrue(sep, verdict)
        return names.split(", ")


class GateCase(GateTree, unittest.TestCase):
    """Subprocess cases: the shipped gate runs for real against a throw-away tree."""

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
        """Every non-PASS status, SKIP and N/A alike, is named in the verdict line.

        A tree with no Go sources, no tests and no parity hook reaches SKIP on
        six steps and N/A on the seventh. This is therefore the case that makes
        a verdict naming only one of those two statuses fail: dropping SKIP from
        the partial selection, naming only the first such step, or hiding the
        whole line behind a plain ``GATE: PASS`` changes either an assertion
        below or the verdict itself.
        """
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        for step, status in (
            ("gofmt", "SKIP"),
            ("build", "SKIP"),
            ("vet", "SKIP"),
            ("test", "SKIP"),
            ("selftest", "SKIP"),
            ("parity", "N/A"),
            ("secrets", "SKIP"),
        ):
            self.assertEqual(self.step_line(proc.stdout, step)[0], status, proc.stdout)
        self.assertEqual(
            self.partial_steps(proc.stdout),
            ["gofmt", "build", "vet", "test", "selftest", "parity", "secrets"],
            proc.stdout,
        )

    def test_every_step_runs_once_in_the_documented_order(self) -> None:
        """The printed steps are the whole documented set, in the documented order.

        The order is the one ``docs/harness.md``'s table and this gate's module
        docstring state; the set is what ``KNOWN_STEPS`` recognises, which is
        what makes an unknown name in ``gate-required.txt`` detectable. A step
        dropped from ``build_steps()``, printed twice, or swapped with its
        neighbour changes the printed line, and a step missing from
        ``KNOWN_STEPS`` would make a ``gate-required.txt`` entry for it read as
        an unknown name instead of being enforced.
        """
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        names = [
            line.split(None, 2)[1] for line in proc.stdout.splitlines() if line.startswith("  ")
        ]
        self.assertEqual(
            names,
            ["gofmt", "build", "vet", "test", "selftest", "parity", "secrets"],
            proc.stdout,
        )
        gate = load_shipped_gate()
        self.assertEqual(list(gate.KNOWN_STEPS), names, proc.stdout)

    def test_comments_and_blank_lines_are_ignored(self) -> None:
        self.write("scripts/gate-required.txt", "# nothing is required yet\n\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_runs_the_trees_own_tests(self) -> None:
        self.write_sample_test()
        proc = self.gate()
        status, detail = self.step_line(proc.stdout, "selftest")
        self.assertEqual(status, "PASS", proc.stdout + proc.stderr)
        self.assertIn("Ran 1 test", detail)
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)

    def test_selftest_step_fails_on_a_failing_test(self) -> None:
        self.write_sample_test(passes=False)
        proc = self.gate()
        status, _ = self.step_line(proc.stdout, "selftest")
        self.assertEqual(status, "FAIL", proc.stdout + proc.stderr)
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)

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
        status, detail = self.step_line(proc.stdout, "gofmt")
        self.assertEqual(status, "FAIL", proc.stdout + proc.stderr)
        self.assertIn("gofmt not found (Go toolchain is required)", detail)
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_missing_go_fails_the_gate(self) -> None:
        """The same rule for the steps that need ``go`` itself, not just ``gofmt``."""
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        proc = self.gate(env={"PATH": str(self.no_go_tools())})
        for step in ("build", "vet", "test"):
            status, detail = self.step_line(proc.stdout, step)
            self.assertEqual(status, "FAIL", proc.stdout + proc.stderr)
            self.assertIn("go not found on PATH", detail)
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
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
        """Call the shipped ``main()`` with ``PATH`` pinned and ``probes`` patched.

        Sockets are blocked for the duration, so the module docstring's "no
        network" is something a case checks instead of something a case assumes:
        a call the gate itself made would have to go through ``socket.socket``.
        """
        gate = load_shipped_gate()
        gate.ROOT = self.root
        out = io.StringIO()
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}), \
                mock.patch.object(
                    socket, "socket", side_effect=AssertionError("the gate opened a socket")
                ), \
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

    def test_unrequired_boundary_steps_are_disclosed_in_the_partial_verdict(self) -> None:
        """BOUNDARY is named in the verdict line too, not only when required.

        The required-boundary case above takes the failure branch before the
        partial verdict is built, so without this case a partial selection that
        kept SKIP and N/A but dropped BOUNDARY (``(SKIP, N/A)``) would leave the
        suite green and the verdict silently short of a host limitation.
        """
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        rc, text = self.run_in_process(
            tracked_go_files=lambda: ["main.go"],
            run=lambda cmd, env=None, timeout=1800: tool_result(cmd),
            go_env=lambda: ({}, "no C compiler on PATH (stubbed)"),
        )
        self.assertEqual(rc, 0, text)
        for step in ("build", "vet", "test"):
            status, detail = self.step_line(text, step)
            self.assertEqual(status, "BOUNDARY", text)
            self.assertIn("no C compiler on PATH (stubbed)", detail)
        self.assertEqual(self.step_line(text, "gofmt")[0], "PASS", text)
        self.assertEqual(
            self.partial_steps(text),
            ["build", "vet", "test", "selftest", "parity", "secrets"],
            text,
        )

    def test_gofmt_failure_results_are_failures_not_skips(self) -> None:
        """``gofmt -l`` naming a file, or ``gofmt`` itself failing, is a FAIL.

        Neither result is reachable through the stub bin, which always answers
        rc=0, so the shipped ``step_gofmt`` is driven directly: the whole point
        of the step is that a real formatting problem cannot degrade into the
        "no Go sources yet" SKIP.
        """
        self.write("main.go", "package main\n")
        _, text = self.run_in_process(
            tracked_go_files=lambda: ["main.go"],
            run=lambda cmd, env=None, timeout=1800: tool_result(cmd, 0, "main.go\n"),
        )
        status, detail = self.step_line(text, "gofmt")
        self.assertEqual(status, "FAIL", text)
        self.assertIn("not gofmt-clean", detail)
        self.assertIn("main.go", text)  # the file list is the detail's second line
        self.assertIn("GATE: FAIL", text)

        rc, text = self.run_in_process(
            tracked_go_files=lambda: ["main.go"],
            run=lambda cmd, env=None, timeout=1800: tool_result(cmd, 2, "", "syntax error\n"),
        )
        status, detail = self.step_line(text, "gofmt")
        self.assertEqual(status, "FAIL", text)
        self.assertIn("syntax error", detail)
        self.assertEqual(rc, 1, text)

    def test_go_step_results_are_failures_when_the_tool_fails(self) -> None:
        """A non-zero ``go build``/``vet``/``test`` is a FAIL, with the shipped argv.

        The recorded argv also pins the default target: no
        ``scripts/gate-targets.txt`` in this tree means ``./...``, one package
        pattern per command, with ``-count=1`` on the test run.
        """
        self.stub("go", exit_code=1)
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        seen: list[list[str]] = []

        def refusing(cmd, env=None, timeout=1800):
            seen.append(list(cmd))
            return tool_result(cmd, 1, "", "boom\n")

        rc, text = self.run_in_process(tracked_go_files=lambda: ["main.go"], run=refusing)
        self.assertEqual(rc, 1, text)
        for step in ("build", "vet", "test"):
            status, detail = self.step_line(text, step)
            self.assertEqual(status, "FAIL", text)
            self.assertIn("rc=1", detail)
        self.assertIn("boom", text)  # the tool's stderr is the detail's second line
        self.assertEqual(
            [cmd for cmd in seen if cmd[0] == "go"],
            [
                ["go", "build", "./..."],
                ["go", "vet", "./..."],
                ["go", "test", "-count=1", "./..."],
            ],
            text,
        )

    def test_gate_targets_file_replaces_the_default_pattern(self) -> None:
        """``scripts/gate-targets.txt`` is what build/vet/test are handed, line for line."""
        self.stub("go")
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        self.write(
            "scripts/gate-targets.txt",
            "# narrowed on purpose\nexample.com/throwaway/cmd/ccc\nexample.com/throwaway/internal/...\n",
        )
        seen: list[list[str]] = []

        def accepting(cmd, env=None, timeout=1800):
            if cmd[0] == "go":
                seen.append(list(cmd))
            return tool_result(cmd)

        rc, text = self.run_in_process(tracked_go_files=lambda: ["main.go"], run=accepting)
        self.assertEqual(rc, 0, text)
        self.assertEqual(
            seen,
            [
                ["go", "build", "example.com/throwaway/cmd/ccc", "example.com/throwaway/internal/..."],
                ["go", "vet", "example.com/throwaway/cmd/ccc", "example.com/throwaway/internal/..."],
                ["go", "test", "-count=1", "example.com/throwaway/cmd/ccc", "example.com/throwaway/internal/..."],
            ],
            text,
        )
        for step in ("build", "vet", "test"):
            self.assertEqual(self.step_line(text, step)[0], "PASS", text)

    def test_go_env_sets_cgo_and_the_windows_c_compiler_boundary(self) -> None:
        """The environment every Go step is handed, and the one excused host.

        ``CGO_ENABLED=1`` is what the inherited SQLite driver needs
        (docs/harness.md). The boundary exists only on Windows without a C
        compiler, which is why a POSIX host missing ``gcc`` is not excused and a
        Windows host that has one is not excused either.
        """
        gate = load_shipped_gate()
        with mock.patch("sys.platform", "win32"), \
                mock.patch.object(gate.shutil, "which", return_value=None):
            env, boundary = gate.go_env()
        self.assertEqual(env, {"GOFLAGS": "-mod=mod", "CGO_ENABLED": "1"})
        self.assertIsInstance(boundary, str)
        self.assertIn("C compiler", boundary)
        with mock.patch("sys.platform", "win32"), \
                mock.patch.object(gate.shutil, "which", return_value="/usr/bin/gcc"):
            _, with_compiler = gate.go_env()
        self.assertIsNone(with_compiler)
        with mock.patch("sys.platform", "linux"), \
                mock.patch.object(gate.shutil, "which", return_value=None):
            _, on_posix = gate.go_env()
        self.assertIsNone(on_posix)

    def test_missing_binary_is_a_step_result_not_a_crash(self) -> None:
        """``run()`` answers rc=127 when the binary is not on PATH at all."""
        gate = load_shipped_gate()
        gate.ROOT = self.root
        missing = "ccc-no-such-binary-3f9a2b"
        proc = gate.run([missing])
        self.assertEqual(proc.returncode, 127, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn(missing, proc.stderr)

    def test_gitleaks_run_that_did_not_complete_is_skipped_not_failed(self) -> None:
        """Any rc other than 0 and the leak code 7 leaves the step a disclosed SKIP."""
        self.stub("gitleaks", exit_code=1)
        rc, text = self.run_in_process(
            run=lambda cmd, env=None, timeout=1800: (
                tool_result(cmd, 1, "", "indexing failed\n")
                if cmd[0] == "gitleaks"
                else tool_result(cmd)
            )
        )
        self.assertEqual(rc, 0, text)
        status, detail = self.step_line(text, "secrets")
        self.assertEqual(status, "SKIP", text)
        self.assertIn("did not complete (rc=1)", detail)
        self.assertIn("secrets", self.partial_steps(text))

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
        """The git branch: tracked ``*.go``, minus a top-level ``vendor/``.

        Both non-``main.go`` files have to stay out of ``found`` for a different
        reason: ``untracked.go`` because ``git ls-files`` never lists it, and
        ``vendor/vendored.go`` because the vendor filter drops it *after* git
        listed it -- so the vendor file is staged, and deleting that filter
        changes the assertions.
        """
        git = shutil.which("git")
        if git is None:
            self.skipTest("git is not on PATH: this case exercises the git branch")
        self.write("main.go", "package main\n")
        self.write("untracked.go", "package main\n")
        self.write("vendor/vendored.go", "package main\n")
        for args in (["init", "-q"], ["add", "main.go", "vendor/vendored.go"]):
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
        ``CALLCCC_GATE_ROOT`` copy of the tree takes -- and a throw-away tree is
        not a repository, so the same branch is taken even where ``git`` is
        reachable without ``PATH`` (Windows resolves a subprocess through more
        than ``PATH``): a nested ``vendor/`` is part of the module and stays, a
        top-level one is not and goes, and ``.git/`` goes with it (a ``.git``
        directory that is not a working repository is exactly what a
        ``CALLCCC_GATE_ROOT`` copy has). The literal below is also a spelling
        claim -- POSIX-form relative paths on every platform, what
        ``git ls-files`` prints (docs/harness.md) -- which
        ``test_walk_branch_spells_paths_the_same_on_a_windows_root`` pins with a
        Windows path flavour, the half of the claim this host cannot produce.
        """
        self.write("main.go", "package main\n")
        self.write("vendor/vendored.go", "package main\n")
        self.write("deep/vendor/nested.go", "package main\n")
        self.write(".git/hooks/sample.go", "package main\n")
        gate = load_shipped_gate()
        gate.ROOT = self.root
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}):
            found = gate.tracked_go_files()
        self.assertEqual(found, ["deep/vendor/nested.go", "main.go"])

    def test_walk_branch_spells_paths_the_same_on_a_windows_root(self) -> None:
        """The walk answers with POSIX-form paths on a Windows-spelled root too.

        ``tracked_go_files()`` promises one spelling on every platform -- the
        one ``git ls-files`` prints -- and on Windows the walk's ``str(relative)``
        used to break that promise by spelling ``deep/vendor/nested.go`` with
        backslashes. This case drives the shipped walk against a root whose paths
        are ``PureWindowsPath`` values, which is the difference Windows makes, so
        a regression fails on this host instead of only in the Windows CI job.
        """
        gate = load_shipped_gate()
        gate.ROOT = WindowsSpelledTree(
            "D:/tree",
            ["D:/tree/main.go", "D:/tree/vendor/vendored.go", "D:/tree/deep/vendor/nested.go"],
        )

        def no_git(cmd, env=None, timeout=1800):
            return tool_result(cmd, 127, "", "git: not found\n")

        with mock.patch.object(gate, "run", side_effect=no_git):
            found = gate.tracked_go_files()
        self.assertEqual(found, ["deep/vendor/nested.go", "main.go"])


if __name__ == "__main__":
    unittest.main()
