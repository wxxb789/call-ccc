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
produce here. ``HostCase`` closes that class of defect from the outside: it
re-runs this whole suite in a child whose ``PATH`` holds no tools at all and,
once, with the host answering the way Windows does without a C compiler -- so a
case that leans on a tool this machine happens to have, or on the platform it
happens to run, fails here instead of in a CI job nobody can watch. Every case
must hold on every Python 3 the repository documents (``docs/conventions.md``),
which is why no case inherits from ``PurePath``: that class' argument handling
is not the same before and after 3.12.
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
REPO_ROOT = GATE.parents[1]

#: Set for a nested run of this suite, so the child's ``HostCase`` copies skip
#: instead of nesting a run of their own; nothing else in the suite reads it.
SELFCHECK = "CALLCCC_SUITE_SELFCHECK"

#: A tool lookup answered the way a Windows host answers it, for the one
#: question the gate asks the host about itself: ``go_env()`` asks whether a C
#: compiler is reachable, because CGO depends on one being installed
#: separately. Off Windows the shipped lookup is ``shutil.which``; this stand-in
#: reproduces the two things Windows' answer does differently -- a PATH split on
#: ``os.pathsep`` and the PATHEXT suffixes -- so a case can put the answer
#: Windows would give in front of the shipped probe instead of asserting from
#: memory what a Windows machine does.
REAL_WHICH = shutil.which


def windows_like_which(name, *args, **kwargs):
    """Resolve ``name`` the way Windows resolves it, over the PATH in force."""
    if os.name == "nt" or not isinstance(name, str):
        return REAL_WHICH(name, *args, **kwargs)
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        if not directory:
            continue
        for candidate in (name, name + ".bat", name + ".exe"):
            path = os.path.join(directory, candidate)
            if os.path.isfile(path):
                return path
    return None


#: The nested run's program, run by the interpreter running this suite with the
#: repository root as its working directory. It discovers this suite exactly as
#: the gate's selftest step does, and -- when asked -- makes the host answer the
#: way Windows does without a C compiler *before* the cases run: ``sys.platform``
#: for the branch, and the suite's own ``windows_like_which`` for the compiler
#: probe, so the nested run and the case that pins the fixture's stub bin cannot
#: answer differently. ``descriptions=False`` keeps one line per case, which is
#: what ``HostCase.nested_statuses`` reads.
NESTED_RUNNER = '''\
import os, shutil, sys, unittest

tests_dir = os.path.join("scripts", "tests")
sys.path.insert(0, os.path.abspath(tests_dir))
suite = unittest.TestLoader().discover(tests_dir, pattern="test_*.py")
import test_gate  # the module discovery just imported: this suite's own helpers

if "--windows-without-compiler" in sys.argv:
    sys.platform = "win32"
    shutil.which = test_gate.windows_like_which
# The marker is the *shipped* probe's own answer under the host this child just
# set up -- its PATH holds no compiler, so the answer is the boundary exactly
# when the Windows answer reached it -- and not a restatement of the flag.
_, boundary = test_gate.load_shipped_gate().go_env()
print("nested run: host answer =", "windows-without-compiler" if boundary else "this host")
result = unittest.TextTestRunner(verbosity=2, descriptions=False).run(suite)
sys.exit(0 if result.wasSuccessful() else 1)
'''


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

    Fixtures and read-the-printed-verdict helpers, so the case classes below can
    use them without one class running the other's tests.

    The stub bin holds the two tools the *shipped gate* asks the host about
    before it looks at the tree at all: ``gofmt`` (a missing one is a FAIL) and
    ``gcc`` (a missing one is the Windows C-compiler boundary). Both answers
    belong to the host, not to any case's subject, so a case gets them out of
    the way by construction -- review round 7 found two cases that stubbed ``go``
    but not ``gcc``, and so observed BOUNDARY on Windows where they expected a
    FAIL and three recorded commands. A case that *is* about a missing tool
    pins ``PATH`` to a bin that lacks it (``no_go_tools()``) or patches the
    probe (``go_env``), and never by forgetting the stub.
    """

    def setUp(self) -> None:
        self.root = Path(tempfile.mkdtemp(prefix="gate-root-"))
        (self.root / "scripts").mkdir(parents=True)
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.stub_bin = self.root / "stub-bin"
        self.stub("gofmt")  # so gofmt's own absence is never the failing reason
        self.stub("gcc")  # so the Windows C-compiler boundary is never the failing reason

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
        """A bin dir holding no Go toolchain, replacing the default stub bin.

        It is a bin of its own, so it carries its own ``gcc`` stub: on Windows a
        bin without one answers with the C-compiler boundary before the missing
        Go toolchain is ever looked for, and the rule under test here is the
        missing Go toolchain, which is a failure on every platform.
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

    def test_gofmt_checks_the_whole_list_in_bounded_batches(self) -> None:
        """The file list reaches ``gofmt`` whole, split so no command line can outgrow the platform.

        The carried tree holds thousands of ``*.go`` files, and Windows refuses a process command line
        past 32 767 characters -- an ``OSError`` the gate used to raise with no verdict at all. The list
        is therefore unchanged in scope, only divided: this case asserts every path is handed to
        ``gofmt`` exactly once, in order, and that each invocation's own command line stays inside the
        shipped budget, so a regression to one invocation fails here rather than in the Windows lane.
        """
        files = [f"internal/ccc/fixture_{index:04d}/deeply/nested/name.go" for index in range(400)]
        seen: list[list[str]] = []

        def recording(cmd, env=None, timeout=1800):
            if cmd[0] == "gofmt":
                seen.append(list(cmd))
            return tool_result(cmd)

        rc, text = self.run_in_process(tracked_go_files=lambda: files, run=recording)
        self.assertEqual(rc, 0, text)
        self.assertEqual(self.step_line(text, "gofmt")[0], "PASS", text)
        self.assertGreater(len(seen), 1, "the whole list went to one invocation")
        self.assertEqual([path for cmd in seen for path in cmd[2:]], files)
        for cmd in seen:
            budget = load_shipped_gate().GOFMT_BATCH_CHARS + len("gofmt -l")
            self.assertLessEqual(len(" ".join(cmd)), budget, cmd[:3])

    def test_every_gofmt_batch_is_checked_and_every_answer_is_reported(self) -> None:
        """A later batch's answer is not dropped: unformatted files and a failed run both survive.

        Batching is only a division of labour if the answers are aggregated: the first case here puts
        the unformatted file in the *last* batch, the second makes an earlier batch exit non-zero, and
        the other batches must still run -- a division that stopped at the first non-zero rc, or that
        reported only the last batch's ``stdout``, would pass a run it did not complete.
        """
        files = [f"internal/ccc/pkg_{index:04d}/deeply/nested/generated_source_file.go"
                 for index in range(200)]
        batches = load_shipped_gate().gofmt_batches(files)
        self.assertGreater(len(batches), 1, "no case here would exercise a second batch")
        last_file = files[-1]

        def unformatted_in_the_last_batch(cmd, env=None, timeout=1800):
            out = last_file + "\n" if cmd[2:] == batches[-1] else ""
            return tool_result(cmd, 0, out, "")

        rc, text = self.run_in_process(
            tracked_go_files=lambda: files, run=unformatted_in_the_last_batch)
        status, detail = self.step_line(text, "gofmt")
        self.assertEqual(status, "FAIL", text)
        self.assertIn("not gofmt-clean", detail)
        self.assertIn(last_file, text)
        self.assertEqual(rc, 1, text)

        seen: list[list[str]] = []

        def failing_batch(cmd, env=None, timeout=1800):
            if cmd[0] == "gofmt":
                seen.append(list(cmd))
                if cmd[2:] == batches[0]:
                    return tool_result(cmd, 2, "", "syntax error in batch one\n")
            return tool_result(cmd)

        rc, text = self.run_in_process(tracked_go_files=lambda: files, run=failing_batch)
        status, detail = self.step_line(text, "gofmt")
        self.assertEqual(status, "FAIL", text)
        self.assertIn("syntax error in batch one", detail)
        self.assertIn(f"1 of {len(batches)} batch(es)", detail)
        self.assertEqual([cmd[2:] for cmd in seen], batches, text)
        self.assertEqual(rc, 1, text)

    def test_a_command_line_the_platform_refuses_is_a_step_result_not_a_crash(self) -> None:
        """``run()`` answers rc=126 for any ``OSError`` that is not a missing binary.

        Windows refusing an over-long command line raises ``OSError`` (not ``FileNotFoundError``), and
        the catch used to let it out of ``run()`` -- so a step that could not be started ended the gate
        in a traceback with no ``GATE:`` verdict for the lane to read.
        """
        gate = load_shipped_gate()
        gate.ROOT = self.root
        with mock.patch.object(gate.subprocess, "run",
                               side_effect=OSError("argument list too long")):
            proc = gate.run(["gofmt", "-l", "many.go"])
        self.assertEqual(proc.returncode, 126, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn("argument list too long", proc.stderr)

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

    def test_gate_goflags_file_is_folded_into_the_go_commands(self) -> None:
        """``scripts/gate-goflags.txt`` reaches build/vet/test, ahead of the targets.

        The inherited AgentsView module builds with ``-tags fts5`` on every Go command, and a binary
        built without it cannot load an archive that has a full-text index, so the flags a gate run
        used have to come from the tree rather than from whoever typed the command.
        """
        self.stub("go")
        self.write("go.mod", "module example.com/throwaway\n\ngo 1.27\n")
        self.write("main.go", "package main\n")
        self.write("scripts/gate-goflags.txt", "# the inherited build tag\n-tags fts5\n")
        self.write("scripts/gate-targets.txt", "./cmd/ccc/...\n")
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
                ["go", "build", "-tags", "fts5", "./cmd/ccc/..."],
                ["go", "vet", "-tags", "fts5", "./cmd/ccc/..."],
                ["go", "test", "-count=1", "-tags", "fts5", "./cmd/ccc/..."],
            ],
            text,
        )

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
        """``run()`` answers rc=127 when the binary is not on PATH at all, and names it.

        The launch is the host's own, so this is the platform's error shape for a missing executable
        -- and the platform's message does not hold the program on every platform (see the case below
        for the Windows one), which is why the step result names it and this asserts it here.
        """
        gate = load_shipped_gate()
        gate.ROOT = self.root
        missing = "ccc-no-such-binary-3f9a2b"
        proc = gate.run([missing])
        self.assertEqual(proc.returncode, 127, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn(missing, proc.stderr)

    def test_a_windows_shaped_missing_binary_is_a_named_step_result(self) -> None:
        """The Windows error carries no filename, so ``run()`` has to name the command itself.

        A Windows host raises this error through ``CreateProcess``: ``PyErr_SetFromWindowsErr`` gives
        ``FileNotFoundError`` a ``winerror`` and no filename, so ``str(exc)`` is "[WinError 2] The
        system cannot find the file specified" -- no program in it -- while the POSIX error names the
        file it could not find. A step result that only echoed the platform's message therefore lost
        the tool's name on Windows alone, which is the failure this case pins: the raising is the
        Windows shape, so the case runs it on any host instead of asserting from memory what Windows
        does, and it launches nothing, so no stub has to exist for it.
        """
        gate = load_shipped_gate()
        gate.ROOT = self.root
        missing = "ccc-no-such-binary-3f9a2b"
        with mock.patch.object(gate.subprocess, "run",
                               side_effect=FileNotFoundError(
                                   2, "The system cannot find the file specified")):
            proc = gate.run([missing])
        self.assertEqual(proc.returncode, 127, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn(missing, proc.stderr)
        self.assertIn("The system cannot find the file specified", proc.stderr)

    def test_secrets_step_scans_this_repositorys_own_files(self) -> None:
        """The carried tree is the parity step's to verify; the leak scan reads what is ours.

        ``secrets_scope`` stages exactly the paths the carry manifest calls project-owned, so an
        upstream credential shape in upstream's own detector corpora cannot fail a gate run here and a
        credential this repository commits cannot hide behind that.
        """
        self.stub("gitleaks")
        self.write("scripts/upstream-manifest.txt",
                   "pin " + "a" * 40 + "\nowned\tproject/\nfile\t100644\tabc\t3\tcarried.txt\n")
        self.write("project/ours.txt", "ours\n")
        self.write("carried.txt", "theirs\n")
        seen: list[list[str]] = []
        scopes: list[Path] = []

        def accepting(cmd, env=None, timeout=1800):
            if cmd[0] == "gitleaks":
                seen.append(list(cmd))
                source = Path(cmd[cmd.index("--source") + 1])
                scopes.append(source)
                staged = sorted(p.name for p in source.rglob("*") if p.is_file())
                self.assertEqual(staged, ["ours.txt"], f"scanned {staged}")
            return tool_result(cmd)

        rc, text = self.run_in_process(run=accepting)
        self.assertEqual(rc, 0, text)
        self.assertEqual(len(seen), 1, text)
        self.assertNotEqual(scopes[0], self.root, text)
        status, detail = self.step_line(text, "secrets")
        self.assertEqual(status, "PASS", text)
        self.assertIn("this repository owns", detail)

    def test_secrets_step_without_a_manifest_says_what_it_scanned(self) -> None:
        """A tree with no carry manifest cannot narrow the scan, and must not pretend it did."""
        self.stub("gitleaks")
        self.write("loose.txt", "no manifest here\n")
        seen: list[list[str]] = []

        def accepting(cmd, env=None, timeout=1800):
            if cmd[0] == "gitleaks":
                seen.append(list(cmd))
            return tool_result(cmd)

        rc, text = self.run_in_process(run=accepting)
        self.assertEqual(rc, 0, text)
        self.assertEqual(len(seen), 1, text)
        self.assertEqual(seen[0][seen[0].index("--source") + 1], str(self.root), text)
        status, detail = self.step_line(text, "secrets")
        self.assertEqual(status, "PASS", text)
        self.assertIn("no readable carry manifest", detail)
        self.assertIn("whole tree", detail)

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

        The case depends on no installed tool: the runner answers ``git ls-files`` with the shape git's
        own output has (one POSIX-form relative path per line, ``docs/harness.md``), and the stub bin
        answers ``gofmt``, so the rule under test -- the gate's file selection -- is exercised with no
        host toolchain. The vendor file has to be dropped *after* the listing for ``found`` to be the
        one tracked file: ``untracked.go`` is not in the listing, ``vendor/vendored.go`` is and the
        vendor filter drops it, so deleting that filter changes the assertions. The first recorded
        command is asserted as well, so a case that quietly took the walk branch instead of the git
        branch fails here rather than reading as green.
        """
        self.write("main.go", "package main\n")
        self.write("untracked.go", "package main\n")
        self.write("vendor/vendored.go", "package main\n")
        gate = load_shipped_gate()
        gate.ROOT = self.root
        recorded: list[list[str]] = []
        real_run = gate.run

        def recording(cmd, env=None, timeout=1800):
            if cmd[:2] == ["git", "ls-files"]:
                recorded.append(list(cmd))
                return tool_result(cmd, 0, "main.go\nvendor/vendored.go\n", "")
            if cmd[0] == "gofmt":
                recorded.append(list(cmd))
                return tool_result(cmd)
            return real_run(cmd, env, timeout)

        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}), \
                mock.patch.object(gate, "run", side_effect=recording):
            found = gate.tracked_go_files()
            status, detail = gate.step_gofmt(found)

        self.assertEqual(recorded, [["git", "ls-files", "*.go"], ["gofmt", "-l", "main.go"]])
        self.assertEqual(found, ["main.go"])
        self.assertEqual(status, gate.PASS, detail)

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


class HostCase(GateTree, unittest.TestCase):
    """Cases about the host this suite runs on, not about a gate verdict.

    Three review rounds in a row found the same defect shape: a case whose
    expectations only hold on the platform, the ``PATH`` or the interpreter its
    author happened to have. These cases close the class from the outside --
    each runs *this whole suite* again in a child process and reads the child's
    result -- so the next case of that shape fails here, on this host, in this
    window, instead of in the one CI job nobody can watch.

    The child's ``PATH`` is a directory holding nothing, and one of the two runs
    puts the Windows answer in front of the platform probe. Every tool a case
    needs therefore has to come from the stub bin the case itself builds.
    ``CALLCCC_SUITE_SELFCHECK`` is set in the child's environment so the child's
    own copies of these cases skip instead of nesting a third run; those two
    skips are the only thing the guard suppresses.
    """

    #: This class' nested-run cases, by name: the ones the guard makes skip
    #: inside a child. Any *other* case skipping under an isolated ``PATH`` or a
    #: forced platform answer is the defect these cases exist to catch.
    GUARDED = (
        "test_suite_holds_in_a_child_whose_path_holds_no_tools",
        "test_suite_holds_when_the_host_answers_windows_without_a_compiler",
    )

    def nested_run(self, windows_without_compiler: bool = False) -> subprocess.CompletedProcess:
        """Run this suite in a child process: no host tools, optionally Windows.

        The child is started the way the gate starts the selftest step -- the
        interpreter running this case, the repository root as working directory
        -- with ``PATH`` pointing at one empty directory,
        ``PYTHONDONTWRITEBYTECODE`` keeping it from writing bytecode into the
        repository, and ``CALLCCC_GATE_ROOT`` dropped so it resolves the tree it
        lives in, as a fresh gate run would.
        """
        tools = self.root / "no-host-tools"
        tools.mkdir(parents=True, exist_ok=True)
        env = os.environ.copy()
        env["PATH"] = str(tools)
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        env[SELFCHECK] = "1"
        env.pop("CALLCCC_GATE_ROOT", None)
        argv = [sys.executable, "-c", NESTED_RUNNER]
        if windows_without_compiler:
            argv.append("--windows-without-compiler")
        return subprocess.run(
            argv, cwd=str(REPO_ROOT), capture_output=True, text=True, env=env, timeout=900
        )

    def suite_size(self) -> int:
        """How many cases this suite defines: what the gate's own discovery finds.

        Counted from the *discovery* the selftest step performs over
        ``scripts/tests/test_*.py``, not from one module, because a suite that
        grew a second file would otherwise be counted short: a nested run that
        discovered a subset -- one file it could not import, one case it dropped
        -- must fail here rather than read as green over the cases it never ran.
        """
        loader = unittest.TestLoader()
        return loader.discover(str(REPO_ROOT / "scripts" / "tests"), pattern="test_*.py").countTestCases()

    def nested_statuses(self, text: str) -> dict[str, str]:
        """The status the nested run printed per case, keyed by case name.

        The nested runner switches docstrings off, so a case is one line of the
        shape ``name (test_gate.Case.name) ... ok`` and the field after the last
        ``...`` is its status -- ``ok``, ``FAIL``, or ``skipped '<reason>'``.
        A case that the child never printed is absent from the mapping, which is
        what makes a dropped case fail the count below rather than pass.

        A line with nothing after its ``...`` is not a case line: the verbose
        runner leaves the case's own line dangling before the indented line of a
        subtest failure, and the case's status is on those indented lines. Reading
        the dangling line as a status raised ``IndexError`` in the parent for
        exactly the child that had a subtest failure to report, so the name never
        reached the message -- see the case that pins it below.
        """
        statuses: dict[str, str] = {}
        for line in text.splitlines():
            name, sep, rest = line.partition(" (")
            if not sep or " ... " not in rest:
                continue
            # The qualifier is ``<module>.<class>.<case>`` for every case the
            # discovering runner prints, whichever file defines it.
            qualifier, close, _ = rest.partition(")")
            if not close or "." not in qualifier:
                continue
            status = rest.rsplit(" ... ", 1)[1].split()
            if not status:
                continue
            statuses[name.strip()] = status[0]
        return statuses

    def nested_summary(self, proc: subprocess.CompletedProcess) -> str:
        """A one-line summary of a nested run, filtered to be safe in a message.

        The child's per-case lines name ``(test_gate.…)`` exactly as this run's
        do, so pasting its raw output into an assertion would make a nested
        break read as a break of *this* run to anything parsing the outer output
        (the suite's mutation driver does). Only the child's summary line, its
        verdict line and the names of its broken cases go in.
        """
        text = proc.stdout + proc.stderr
        ran = next(
            (line for line in text.splitlines() if line.startswith("Ran ")),
            "no 'Ran N tests' line",
        )
        verdict = next(
            (line for line in text.splitlines() if line.startswith(("OK", "FAILED"))),
            "no verdict line",
        )
        broken = [
            line.split(": ", 1)[1].split(" (")[0]
            for line in text.splitlines()
            if line.startswith(("FAIL: ", "ERROR: "))
        ]
        summary = f"nested run rc={proc.returncode}: {ran} | {verdict}"
        return summary + (f" | broken: {', '.join(broken)}" if broken else "")

    def assert_nested_green(self, proc: subprocess.CompletedProcess) -> dict[str, str]:
        """Assert the nested run ran every case and broke none; return its statuses."""
        text = proc.stdout + proc.stderr
        summary = self.nested_summary(proc)
        statuses = self.nested_statuses(text)
        self.assertEqual(proc.returncode, 0, summary)
        self.assertEqual(len(statuses), self.suite_size(), summary)
        ran = [line for line in text.splitlines() if line.startswith("Ran ")]
        self.assertEqual(len(ran), 1, summary)
        self.assertEqual(ran[0].split()[1], str(self.suite_size()), summary)
        return statuses

    def assert_only_expected_skips(
        self, statuses: dict[str, str], message: str
    ) -> None:
        """The nested run's non-``ok`` cases must be exactly the expected skips.

        A skipped case proves nothing, so the only legitimate skips are named and everything else is a
        failure of this assertion: the child's own copies of this class skip because the guard told
        them to, and no case may skip for any other reason -- a case that needs a host tool the child's
        ``PATH`` does not hold is the defect these cases exist to catch, and every stub a case needs has
        to live in the bin inside its own throw-away tree. That the guard *did* fire is asserted too: if
        it stopped being set, a child that nested a third run would be the symptom, and this is the case
        that would notice first.
        """
        expected = sorted(self.GUARDED)
        self.assertEqual(
            sorted(name for name, status in statuses.items() if status != "ok"),
            expected,
            message,
        )
        for name in expected:
            self.assertEqual(statuses.get(name), "skipped", f"{name}: {message}")

    def test_suite_holds_in_a_child_whose_path_holds_no_tools(self) -> None:
        """No case may need a tool this machine happens to have installed.

        Every stub a case needs lives in the bin inside its own throw-away
        tree, so an empty ambient ``PATH`` changes nothing for a case that
        stages what it uses -- and fails a case that quietly reaches for this
        host's ``go``, ``gofmt``, ``gcc``, ``gitleaks`` or ``git``.
        """
        if os.environ.get(SELFCHECK):
            self.skipTest(f"already a nested run ({SELFCHECK} is set)")
        proc = self.nested_run()
        statuses = self.assert_nested_green(proc)
        self.assert_only_expected_skips(statuses, self.nested_summary(proc))

    def test_suite_holds_when_the_host_answers_windows_without_a_compiler(self) -> None:
        """The Go-command cases must not flip when the platform probe says Windows.

        This is the defect review round 7 found: a fixture that stubs ``go`` but
        no C compiler, under a host answering "Windows, no compiler", so
        ``step_go()`` returns BOUNDARY before the mocked runner is ever called --
        the failure-result case sees BOUNDARY where it expects a FAIL, and the
        target-pattern case records no command where it expects three. The child
        forces exactly that answer and re-runs every case, so the two are
        asserted by name here as well as by the child's exit code: a case that
        forgets its compiler stub fails here, on this host.

        The marker it asserts is the *shipped* probe's answer inside the child,
        read after the forcing, so this case fails if the forcing stops reaching
        ``go_env()`` -- quietly re-running the suite on the ordinary host answer
        would leave every assertion above about the wrong host. On a Windows
        runner the answer is the host's own and the marker is expected there for
        a different reason; off Windows it is only true if the forcing worked.
        """
        if os.environ.get(SELFCHECK):
            self.skipTest(f"already a nested run ({SELFCHECK} is set)")
        proc = self.nested_run(windows_without_compiler=True)
        message = self.nested_summary(proc)
        self.assertIn("nested run: host answer = windows-without-compiler", proc.stdout, message)
        statuses = self.assert_nested_green(proc)
        for case in (
            "test_go_step_results_are_failures_when_the_tool_fails",
            "test_gate_targets_file_replaces_the_default_pattern",
        ):
            self.assertEqual(statuses.get(case), "ok", f"{case}: {message}")
        self.assert_only_expected_skips(statuses, message)

    def broken_subtest_child(self) -> subprocess.CompletedProcess:
        """A child run whose one case fails inside a ``subTest``, executed here.

        The shape is produced by running it, not asserted from memory: the verbose
        runner prints the case's own line, then -- for a failure that came from a
        subtest -- an indented line per subtest, so the case's line is left dangling
        with no status. The runner is started the way ``nested_run`` starts the real
        one (the interpreter running this case, a tree of this case's own, no host
        tool needed), and the suite it discovers is written here rather than
        committed: no case of this suite may fail on purpose to produce the shape.
        """
        suite = self.root / "subtest-suite"
        suite.mkdir()
        (suite / "test_subtests.py").write_text(
            "import unittest\n"
            "\n"
            "class Case(unittest.TestCase):\n"
            "    def test_only_reports_come_from_subtests(self):\n"
            "        for home in ('one', 'two'):\n"
            "            with self.subTest(home=home):\n"
            "                self.assertTrue(False, f'{home} broke')\n",
            encoding="utf-8",
        )
        return subprocess.run(
            [sys.executable, "-m", "unittest", "discover", "-v", "-s", str(suite),
             "-p", "test_*.py"],
            capture_output=True, text=True, cwd=str(self.root), timeout=300,
        )

    def test_a_child_broken_inside_a_subtest_is_reported_by_name(self) -> None:
        """A child broken inside a subtest fails this suite *naming* the case.

        What a nested run is for is saying which case broke on the host this run
        cannot watch. The reader used to answer with an ``IndexError`` in the parent
        instead, for exactly the shape a Windows child had at the head this repair
        started from: the dangling line the verbose runner leaves when a case's only
        reports come from subtests. The case is named in the mapping *and* in the
        message that fails the parent, so the next such child fails the suite with a
        name to look at rather than an error block with nothing in it.
        """
        proc = self.broken_subtest_child()
        text = proc.stdout + proc.stderr
        case = "test_only_reports_come_from_subtests"
        self.assertEqual(proc.returncode, 1, text)
        statuses = self.nested_statuses(text)
        self.assertEqual(statuses.get(case), "FAIL", text)
        self.assertIn(f"broken: {case}", self.nested_summary(proc))
        with self.assertRaises(AssertionError) as caught:
            self.assert_nested_green(proc)
        self.assertIn(f"broken: {case}", str(caught.exception))

    def test_the_stub_bin_answers_the_windows_compiler_probe(self) -> None:
        """``go_env()`` under the Windows answer, in both directions, as shipped.

        The bin ``setUp`` hands every case holds a ``gcc`` stub, which is what
        keeps the C-compiler boundary out of a case whose subject is something
        else -- so this is the case that fails if that stub goes away. The
        second half drops the stub and lets the shipped ``go_env()`` answer: it
        must be the boundary, or the first half would be pinning a probe that no
        longer asks the host anything.
        """
        gate = load_shipped_gate()
        without_compiler = self.root / "no-compiler"
        self.stub("gofmt", where=without_compiler)  # a Windows host with Go and no C compiler
        with mock.patch("sys.platform", "win32"), \
                mock.patch.object(shutil, "which", windows_like_which):
            with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}):
                env, boundary = gate.go_env()
            self.assertEqual(env, {"GOFLAGS": "-mod=mod", "CGO_ENABLED": "1"})
            self.assertIsNone(boundary)
            with mock.patch.dict(os.environ, {"PATH": str(without_compiler)}):
                _, missing = gate.go_env()
        self.assertIsInstance(missing, str)
        self.assertIn("C compiler", missing)


if __name__ == "__main__":
    unittest.main()
