#!/usr/bin/env python3
"""Regression tests for scripts/obligation-citations.py and the gate step that runs it.

A manifest under ``docs/evidence/`` is an increment's acceptance surface, and the ``citations`` step is
required, so the checker's own rules need tests like the gate's: a header that is not the four columns,
a row with a missing or an extra cell, a rule cited twice, a test no ``_test.go`` declares, a name
``go test`` would not run, a package the gate does not test, a cited test that fails, skips or is not
reported at all, a run that exited non-zero with no verdict line, a run that outran its deadline, a
cited name longer than the batch bound, a flood of violations past the printed bound, a tree whose
manifest is gone, and a tree whose ``scripts/gate.py`` cannot be loaded all have to come back as the
verdict the contract names — and the positive control proves the reading is live rather than vacuous (it
asserts the row count the run verified, so a checker that verified nothing cannot pass it).

Every case drives the shipped checker in-process with ``ROOT`` pointed at a throw-away tree and the
checker's runner seam (``run``) answering the ``go test`` invocation, so no case depends on a Go
toolchain being installed, on the platform it runs on, or on the tree this file lives in. ``PATH`` is
pinned to a stub bin holding ``go``, ``gofmt`` and ``gcc``, which is what keeps the missing-tool answer
and the Windows C-compiler boundary out of a case whose subject is something else; a case that *is*
about the missing tool pins a bin without one. Two cases drive the shipped gate as a subprocess to pin
that a skipped citations step is a partial run when it is not required and a failure when it is, and one
drives the gate's step in-process with one probe patched to pin the mapping from the checker's verdict
line to the gate's statuses.
"""

from __future__ import annotations

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
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]
CHECKER = SCRIPTS / "obligation-citations.py"
GATE = SCRIPTS / "gate.py"

#: The header every case's manifest carries unless it is the case's subject.
HEADER = "rule_id\ttest\tpackage\tartifact"

#: The verdict line's own spelling, which the gate reads its status from.
VERDICT_LINE = re.compile(r"^citations: (PASS|FAIL|SKIP|BOUNDARY|N/A)(?: — .*)?$")


def shipped_scripts_on_path() -> None:
    """The shipped scripts import their siblings by name; this module loads them from their own tree."""
    if str(SCRIPTS) not in sys.path:
        sys.path.insert(0, str(SCRIPTS))


def load_module(path: Path, name: str):
    """Import a shipped script by path, with ``scripts/`` on ``sys.path`` for its own imports."""
    shipped_scripts_on_path()
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_checker():
    return load_module(CHECKER, "shipped_obligation_citations")


def load_gate():
    return load_module(GATE, "shipped_gate_for_citations")


def tool_result(cmd, rc=0, out="", err=""):
    """A ``CompletedProcess`` shaped like the ones the shipped runners return."""
    return subprocess.CompletedProcess(cmd, rc, out, err)


def requested(cmd):
    """The test names one recorded ``go test`` invocation asked for (its own ``-run`` alternation)."""
    pattern = cmd[cmd.index("-run") + 1]
    return [name for name in pattern[2:-2].split("|") if name]


def run_reporting(status, rc):
    """The answer a ``go test -v`` run gives when every requested test comes back ``status``."""

    def answer(cmd):
        lines = "".join(f"--- {status}: {name} (0.00s)\n" for name in requested(cmd))
        return tool_result(cmd, rc, lines)

    return answer


def run_passing(cmd):
    """The answer a real ``go test -v`` run gives for tests that exist and pass."""
    return run_reporting("PASS", 0)(cmd)


def run_silent(cmd):
    """A ``go test`` run that exited 0 without reporting any test (``no tests to run``)."""
    return tool_result(cmd, 0, f"ok  \t{cmd[-1]}\t0.01s\n")


class CitationsTree(unittest.TestCase):
    """One throw-away tree, one stub bin and one pinned ``PATH`` per case.

    The stub bin answers the host probes the shipped code makes before it looks at the tree: ``go`` and
    the C compiler the Windows boundary asks about (``gofmt`` too, for the gate's own first step when a
    case drives the gate). No case executes a stub — every case that reaches a ``go test`` invocation
    patches the checker's runner seam — so a stub only has to be *found*, which is why it is written
    twice, as a POSIX script and as a Windows ``.bat``.
    """

    PACKAGE = "./internal/ccc"

    def setUp(self) -> None:
        self.root = Path(tempfile.mkdtemp(prefix="citations-root-"))
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.stub_bin = self.root / "stub-bin"
        for name in ("go", "gofmt", "gcc"):
            self.stub(name)
        self.write("scripts/gate-targets.txt", "./cmd/ccc/...\n./internal/ccc/...\n")
        self.write("scripts/gate-goflags.txt", "-tags fts5\n")

    def stub(self, name: str, where: Path | None = None) -> None:
        """Make ``name`` findable on a pinned ``PATH`` without installing a tool."""
        directory = where or self.stub_bin
        directory.mkdir(parents=True, exist_ok=True)
        script = directory / name
        script.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        script.chmod(0o755)
        (directory / f"{name}.bat").write_text("@exit /b 0\r\n", encoding="utf-8")

    def write(self, relative: str, text: str) -> None:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def declare(self, *tests: str, package: str | None = None) -> None:
        """Write the ``_test.go`` that declares each cited test, the way the static check reads it."""
        package = package or self.PACKAGE
        directory = self.root / package[len("./"):]
        directory.mkdir(parents=True, exist_ok=True)
        lines = ["package ccc", "", 'import "testing"', ""]
        for name in tests:
            lines += [f"func {name}(t *testing.T) {{}}", ""]
        (directory / "cited_test.go").write_text("\n".join(lines), encoding="utf-8")

    def manifest(
        self,
        rows: list[str],
        name: str = "0002-citations.tsv",
        header: str = HEADER,
        preamble: tuple[str, ...] = ("# a comment line", ""),
    ) -> None:
        self.write(f"docs/evidence/{name}", "\n".join([*preamble, header, *rows]) + "\n")

    def row(self, rule_id: str, test: str, package: str | None = None,
            artifact: str = "0002-obligation-matrix.md §X") -> str:
        return "\t".join([rule_id, test, package or self.PACKAGE, artifact])

    def install_checker(self) -> None:
        """Put the shipped checker where a subprocess run of the gate finds it."""
        target = self.root / "scripts" / "obligation-citations.py"
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(CHECKER, target)

    def drive(self, answer=None, env: dict[str, str] | None = None):
        """Run the shipped checker over this tree: ``(exit code, printed text, commands)``.

        ``ROOT`` is pointed at the throw-away tree and the runner seam is patched, so no host tool is
        executed; ``PATH`` stays pinned to the stub bin, so the shipped host probes answer the stubs.
        """
        checker = load_checker()
        checker.ROOT = self.root
        commands: list[list[str]] = []

        def recording(cmd, env=None, timeout=None):
            commands.append(list(cmd))
            return (answer or run_passing)(cmd)

        out = io.StringIO()
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin), **(env or {})}), \
                mock.patch.object(checker, "run", side_effect=recording), \
                redirect_stdout(out):
            rc = checker.main()
        return rc, out.getvalue(), commands

    def verdict(self, text: str) -> str:
        """The checker's own verdict line (exactly one, whatever else it printed)."""
        lines = [line for line in text.splitlines() if VERDICT_LINE.match(line)]
        self.assertEqual(len(lines), 1, text)
        return lines[0]

    def status_of(self, text: str) -> str:
        return self.verdict(text).split(":", 1)[1].split(" — ", 1)[0].strip()

    def file_lines(self, text: str) -> list[str]:
        """The per-file lines (``<path>: <n> row(s) verified``), in the order they were printed."""
        return [line for line in text.splitlines() if line.endswith("row(s) verified")]


class CitationsCase(CitationsTree):
    """The checker's own rules, each pinned by a case that fails when the rule breaks."""

    def test_a_well_formed_manifest_verifies_every_row(self) -> None:
        """The positive control: two rows, two cited tests, one package, one invocation.

        The row count the run *verified* is asserted, so a checker that stopped running anything (or
        that verified nothing) cannot pass this by printing the right verdict; the second half pins
        that one test cited by two rows is requested once and verifies both.
        """
        self.declare("TestAlpha", "TestBeta")
        self.manifest([self.row("A1", "TestAlpha"), self.row("B2", "TestBeta")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(self.verdict(text), "citations: PASS", text)
        self.assertEqual(
            self.file_lines(text), ["docs/evidence/0002-citations.tsv: 2 row(s) verified"], text
        )
        self.assertIn("citations: 2 row(s) across 1 file(s) verified", text)
        self.assertEqual(len(commands), 1, text)
        self.assertEqual(requested(commands[0]), ["TestAlpha", "TestBeta"], text)

        # One test can pin two obligations: both rows verify from the same invocation, and the
        # alternation names it once (asking for it twice would be asking the same run twice).
        self.manifest([self.row("A1", "TestAlpha"), self.row("B2", "TestAlpha")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(
            self.file_lines(text), ["docs/evidence/0002-citations.tsv: 2 row(s) verified"], text
        )
        self.assertEqual(commands[0][-2], "^(TestAlpha)$", text)

    def test_a_header_that_is_not_the_four_columns_fails(self) -> None:
        """A missing, renamed or reordered column is refused, naming the file and the line.

        The header is the first *non-comment* line — a comment and a blank line stand above it — which
        is why the line named here is line 3 and not line 1.
        """
        for header in (
            "rule_id\ttest\tpackage",
            "rule_id\ttest\tpackage\tnote",
            "test\trule_id\tpackage\tartifact",
        ):
            with self.subTest(header=header):
                self.declare("TestAlpha")
                self.manifest([self.row("A1", "TestAlpha")], header=header)
                rc, text, commands = self.drive()
                self.assertEqual(rc, 1, text)
                self.assertEqual(self.status_of(text), "FAIL", text)
                self.assertIn("docs/evidence/0002-citations.tsv:3: the header must name exactly", text)
                self.assertEqual(commands, [], text)

    def test_a_row_that_is_not_four_non_empty_cells_fails(self) -> None:
        for label, row in (
            ("missing", "A1\tTestAlpha\t./internal/ccc"),
            ("extra", "A1\tTestAlpha\t./internal/ccc\tart\tmore"),
            ("empty", "A1\tTestAlpha\t\tart"),
        ):
            with self.subTest(row=label):
                self.declare("TestAlpha")
                self.manifest([row])
                rc, text, commands = self.drive()
                self.assertEqual(rc, 1, text)
                self.assertIn(
                    "docs/evidence/0002-citations.tsv:4: a row must hold exactly four non-empty cells",
                    text,
                )
                self.assertEqual(commands, [], text)

    def test_a_rule_cited_twice_in_one_file_fails(self) -> None:
        self.declare("TestAlpha", "TestBeta")
        self.manifest([self.row("A1", "TestAlpha"), self.row("A1", "TestBeta")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 1, text)
        self.assertIn(
            "docs/evidence/0002-citations.tsv:5: rule id 'A1' is already cited at line 4", text
        )
        self.assertEqual(commands, [], text)

    def test_a_cited_test_no_test_file_declares_fails_before_any_tool_runs(self) -> None:
        """The package's own directory is what declares it: the tree here declares another test."""
        self.declare("TestOther")
        self.manifest([self.row("A1", "TestAlpha")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 1, text)
        self.assertIn("rule A1: test TestAlpha is not declared by any _test.go in ./internal/ccc", text)
        self.assertEqual(commands, [], "the static half must be answered without running go")

    def test_a_name_go_test_would_not_run_fails_before_any_tool_runs(self) -> None:
        """``TestXxx`` with a lowercase first letter, or a name that is not a Go identifier, is refused.

        Each name here *is* declared by a ``_test.go`` in the tree, so the declaration rule cannot be
        what fails it — and no invocation may be recorded for any of them.
        """
        for name in ("Testalpha", "Tests", "Test-Alpha"):
            with self.subTest(name=name):
                self.declare(name)
                self.manifest([self.row("A1", name)])
                rc, text, commands = self.drive()
                self.assertEqual(rc, 1, text)
                self.assertIn(f"{name!r} is not a legal Go test name", text)
                self.assertEqual(commands, [], text)

    def test_a_package_the_gate_does_not_test_fails_before_any_tool_runs(self) -> None:
        """The coverage answer comes from ``scripts/gate-targets.txt``, and a pattern is not a package."""
        self.declare("TestAlpha")
        self.manifest([self.row("A1", "TestAlpha")])
        for named, package in (
            ("not covered by scripts/gate-targets.txt", "./internal/parser"),
            ("must name one package", "./internal/ccc/..."),
        ):
            with self.subTest(package=package):
                self.manifest([self.row("A1", "TestAlpha", package=package)])
                rc, text, commands = self.drive()
                self.assertEqual(rc, 1, text)
                self.assertIn(named, text)
                self.assertEqual(commands, [], text)

        # The knob file, not a list this checker holds: narrowed to ./cmd/ccc/..., a citation of
        # ./internal/ccc is no longer covered even though the package exists on disk.
        self.write("scripts/gate-targets.txt", "./cmd/ccc/...\n")
        self.manifest([self.row("A1", "TestAlpha")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 1, text)
        self.assertIn("package ./internal/ccc is not covered by scripts/gate-targets.txt", text)

    def test_a_cited_test_that_does_not_pass_fails(self) -> None:
        """Failing, skipping and never-reported are all failures naming the row.

        The first half's two rows share one invocation, so it also pins that a run's own failure does
        not erase the rows that did verify — the per-file count stays truthful.
        """
        self.declare("TestAlpha", "TestBeta")
        self.manifest([self.row("A1", "TestAlpha"), self.row("B2", "TestBeta")])

        def half_failing(cmd):
            lines = "".join(
                f"--- {'PASS' if name == 'TestBeta' else 'FAIL'}: {name} (0.00s)\n"
                for name in requested(cmd)
            )
            return tool_result(cmd, 1, lines)

        rc, text, commands = self.drive(answer=half_failing)
        self.assertEqual(rc, 1, text)
        self.assertIn(
            "rule A1: test TestAlpha in ./internal/ccc did not pass — go test reported "
            "`--- FAIL: TestAlpha`",
            text,
        )
        self.assertNotIn("rule B2", text)
        self.assertEqual(
            self.file_lines(text), ["docs/evidence/0002-citations.tsv: 1 row(s) verified"], text
        )

        for status, rc_of_run in (("SKIP", 0), ("FAIL", 1)):
            with self.subTest(status=status):
                rc, text, _commands = self.drive(answer=run_reporting(status, rc_of_run))
                self.assertEqual(rc, 1, text)
                self.assertIn(f"did not pass — go test reported `--- {status}: TestAlpha`", text)

        rc, text, _commands = self.drive(answer=run_silent)
        self.assertEqual(rc, 1, text)
        self.assertIn("go test printed no `--- PASS:` line for it", text)

    def test_a_run_that_exited_without_a_verdict_is_reported_by_its_rc(self) -> None:
        """A package that does not build prints no per-test line: the run's own rc is the message.

        The rc arm is the last fallback, so a run that failed *and* printed a per-test line reports
        that line instead — the second half pins the precedence beside the first.
        """
        self.declare("TestAlpha")
        self.manifest([self.row("A1", "TestAlpha")])

        rc, text, _commands = self.drive(
            answer=lambda cmd: tool_result(cmd, 1, "", "build failed\n")
        )
        self.assertEqual(rc, 1, text)
        self.assertIn(
            "rule A1: test TestAlpha in ./internal/ccc did not pass — "
            "the `go test` run for it exited rc=1",
            text,
        )

        def failing_but_reported(cmd):
            return tool_result(cmd, 1, "--- FAIL: TestAlpha (0.00s)\n", "")

        rc, text, _commands = self.drive(answer=failing_but_reported)
        self.assertEqual(rc, 1, text)
        self.assertIn("did not pass — go test reported `--- FAIL: TestAlpha`", text)
        self.assertNotIn("exited rc=", text)

    def test_a_run_that_outran_its_deadline_is_a_result_not_a_crash(self) -> None:
        """A stalled ``go test`` answers ``rc=124`` with its deadline named, rather than raising.

        This arm is this checker's own — the gate's ``run()`` lets ``TimeoutExpired`` through — and it
        is what turns a `go test` that never answers into a citation's own ``did not pass`` instead of
        a traceback with no verdict at all. Both the platform's own raising (a real child outliving a
        short deadline) and the raise shape itself are driven here.
        """
        checker = load_checker()
        checker.ROOT = self.root

        stalling = [sys.executable, "-c", "import time; time.sleep(30)"]
        proc = checker.run(stalling, timeout=0.2)
        self.assertEqual(proc.returncode, 124, proc.stderr)
        self.assertEqual(proc.stdout, "")
        self.assertIn(sys.executable, proc.stderr)
        self.assertIn("no answer within 0.2 s", proc.stderr)

        # The platform's shape: the exception carries the output captured so far, and the answer
        # passes it on rather than dropping it.
        with mock.patch.object(
            checker.subprocess, "run",
            side_effect=subprocess.TimeoutExpired(["go", "test"], 7, output="ok  \t./internal/ccc\n"),
        ):
            proc = checker.run(["go", "test"], timeout=7)
        self.assertEqual(proc.returncode, 124, proc.stderr)
        self.assertEqual(proc.stdout, "ok  \t./internal/ccc\n")
        self.assertEqual(proc.stderr, "go: no answer within 7 s\n")

    def test_the_printed_violations_are_bounded_and_the_verdict_counts_them_all(self) -> None:
        """A flood of violations prints the bound and names how many it held back.

        The bound is ``VIOLATIONS_PRINTED`` (the gofmt step's own), and the verdict's count is the
        whole number — so a printout that stops at the bound still tells a reader the size of the
        run's answer.
        """
        bound = load_checker().VIOLATIONS_PRINTED
        names = [f"TestCitedObligationNumber{index:04d}" for index in range(bound + 1)]
        self.manifest([self.row(f"A{index:04d}", name) for index, name in enumerate(names)])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 1, text)
        printed = [line for line in text.splitlines() if line.startswith("  docs/evidence/")]
        self.assertEqual(len(printed), bound, text)
        self.assertIn(f"  … and {len(names) - bound} more", text)
        self.assertEqual(self.verdict(text), f"citations: FAIL — {len(names)} violation(s)", text)
        self.assertEqual(commands, [], text)

    def test_no_manifest_anywhere_is_a_skip_naming_where_it_looked(self) -> None:
        self.write("docs/evidence/0002-obligation-matrix.md", "prose evidence, no citation rows\n")
        rc, text, commands = self.drive()
        self.assertEqual(rc, 2, text)
        self.assertEqual(
            self.verdict(text), "citations: SKIP — no *-citations.tsv under docs/evidence/", text
        )
        self.assertEqual(commands, [], text)

    def test_the_manifests_are_discovered_and_printed_in_sorted_order(self) -> None:
        """The pinned order is the file-name order, and only ``*-citations.tsv`` is discovered."""
        self.declare("TestAlpha", "TestBeta")
        self.manifest([self.row("A1", "TestAlpha")], name="b-second-citations.tsv")
        self.manifest([self.row("B1", "TestBeta")], name="a-first-citations.tsv")
        self.write("docs/evidence/0002-obligation-matrix.tsv", HEADER + "\n")
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(
            self.file_lines(text),
            [
                "docs/evidence/a-first-citations.tsv: 1 row(s) verified",
                "docs/evidence/b-second-citations.tsv: 1 row(s) verified",
            ],
            text,
        )
        self.assertIn("citations: 2 row(s) across 2 file(s) verified", text)

    def test_one_invocation_per_package_hands_every_cited_test_to_it(self) -> None:
        """Three rows in one package are one ``go test`` run, with the knobs' own flags and ``-run``.

        One invocation per *row* would put three commands here, and a flag this checker held itself
        rather than read would survive the knob file's second spelling below.
        """
        self.declare("TestAlpha", "TestBeta", "TestGamma")
        self.manifest(
            [self.row("A1", "TestAlpha"), self.row("A2", "TestBeta"), self.row("A3", "TestGamma")]
        )
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(
            commands,
            [
                [
                    "go", "test", "-count=1", "-v", "-tags", "fts5",
                    "-run", "^(TestAlpha|TestBeta|TestGamma)$", "./internal/ccc",
                ]
            ],
            text,
        )

        self.write("scripts/gate-goflags.txt", "-tags fts5\n-short\n")
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(commands[0][4:6], ["-tags", "fts5"], text)
        self.assertIn("-short", commands[0], text)

    def test_the_run_is_split_only_when_the_alternation_would_outgrow_the_bound(self) -> None:
        """A command line cannot outgrow the platform: the cited names go in bounded batches.

        The bound is the one ``gate.py``'s ``gofmt_batches`` uses for its file list, and the batches
        together are the whole list in the same order — a split that dropped a name, ran one twice, or
        kept one invocation for a list Windows would refuse changes an assertion below.
        """
        names = [f"TestCitedObligationNumber{index:04d}" for index in range(300)]
        self.declare(*names)
        self.manifest([self.row(f"A{index:04d}", name) for index, name in enumerate(names)])
        bound = load_gate().GOFMT_BATCH_CHARS
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertGreater(len(commands), 1, text)
        handed = [name for cmd in commands for name in requested(cmd)]
        self.assertEqual(handed, names, text)
        for cmd in commands:
            alternation = cmd[cmd.index("-run") + 1]
            self.assertLessEqual(len(alternation) - len("^()"), bound, cmd[:1])

    def test_a_name_longer_than_the_bound_is_a_batch_of_its_own_rather_than_dropped(self) -> None:
        """One name past the bound cannot be split, so it goes alone: nothing is dropped.

        This is the gate's own ``gofmt_batches`` clause, and it is what keeps an over-long cited name
        from being left out of every batch — a row verified by a run that never asked for it.
        """
        bound = load_gate().GOFMT_BATCH_CHARS
        long_name = "Test" + "A" * bound
        self.declare(long_name, "TestAlpha")
        self.manifest([self.row("A1", long_name), self.row("B2", "TestAlpha")])
        rc, text, commands = self.drive()
        self.assertEqual(rc, 0, text)
        self.assertEqual(len(commands), 2, text)
        self.assertEqual(requested(commands[0]), [long_name], text)
        self.assertEqual(requested(commands[1]), ["TestAlpha"], text)
        self.assertEqual(
            [name for cmd in commands for name in requested(cmd)], [long_name, "TestAlpha"], text
        )

    def test_the_missing_go_toolchain_is_a_failure_and_the_windows_answer_is_the_boundary(self) -> None:
        """The host answer is the gate's own: no ``go`` fails, Windows without a compiler is BOUNDARY."""
        self.declare("TestAlpha")
        self.manifest([self.row("A1", "TestAlpha")])

        without_go = self.root / "without-go"
        self.stub("gcc", where=without_go)  # so the C-compiler boundary is not the failing reason
        rc, text, commands = self.drive(env={"PATH": str(without_go)})
        self.assertEqual(rc, 1, text)
        self.assertEqual(self.verdict(text), "citations: FAIL — go not found on PATH", text)
        self.assertEqual(commands, [], text)

        without_compiler = self.root / "without-compiler"
        self.stub("go", where=without_compiler)
        # Both halves of the Windows question are put in front of the shipped probe: the platform
        # answer alone changes ``shutil.which``'s own code path on this host (a Windows host resolves
        # a bare name through ``PATHEXT``), so the lookup's answer is patched with it -- the same pair
        # the gate's own suite puts in front of ``go_env()``.
        with mock.patch("sys.platform", "win32"), mock.patch("shutil.which", return_value=None):
            rc, text, commands = self.drive(env={"PATH": str(without_compiler)})
        self.assertEqual(rc, 3, text)
        self.assertEqual(self.status_of(text), "BOUNDARY", text)
        self.assertIn("C compiler", text)
        self.assertEqual(commands, [], text)

    def test_a_tree_whose_gate_cannot_be_loaded_fails_naming_it(self) -> None:
        """Without ``scripts/gate.py`` nothing can be read the way the gate reads it: a named FAIL.

        The checker's knob readers, batching rule and host boundary are the gate module's, so a tree
        that holds a copied checker but no gate is the shape where the load fails — and the answer is
        a FAIL naming the sibling, not a crash with no verdict line at all.
        """
        self.declare("TestAlpha")
        self.manifest([self.row("A1", "TestAlpha")])
        self.install_checker()
        checker = load_module(
            self.root / "scripts" / "obligation-citations.py", "copied_obligation_citations"
        )
        checker.ROOT = self.root
        out = io.StringIO()
        with mock.patch.dict(os.environ, {"PATH": str(self.stub_bin)}), redirect_stdout(out):
            rc = checker.main()
        text = out.getvalue()
        self.assertEqual(rc, 1, text)
        self.assertEqual(self.status_of(text), "FAIL", text)
        self.assertIn("scripts/gate.py", text)
        self.assertIn("could not be loaded", text)


class GateWiringCase(CitationsTree):
    """The gate step: the checklist's verdict line is the status, and a skip is required to fail."""

    def gate(self, env: dict[str, str] | None = None):
        """Run the shipped gate against this tree with ``PATH`` pinned to the stub bin."""
        merged = os.environ.copy()
        merged["PATH"] = str(self.stub_bin)
        merged["CALLCCC_GATE_ROOT"] = str(self.root)
        merged.update(env or {})
        return subprocess.run(
            [sys.executable, str(GATE)],
            capture_output=True, text=True, env=merged, cwd=str(self.root), timeout=600,
        )

    def step_line(self, stdout: str, name: str) -> tuple[str, str]:
        """The ``(status, detail)`` the run printed for step ``name``."""
        rows = [line.split(None, 2) for line in stdout.splitlines() if line.startswith("  ")]
        rows = [row for row in rows if len(row) >= 2 and row[1] == name]
        self.assertEqual(len(rows), 1, stdout)
        return rows[0][0], (rows[0][2] if len(rows[0]) > 2 else "")

    def test_the_gate_rewrites_a_skipped_citations_step_when_it_is_required(self) -> None:
        """A tree that lost its manifest must fail the gate, not pass without the step running."""
        self.install_checker()
        self.write("scripts/gate-required.txt", "citations\n")
        proc = self.gate()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        status, detail = self.step_line(proc.stdout, "citations")
        self.assertEqual(status, "FAIL", proc.stdout)
        self.assertIn("REQUIRED but was SKIP", detail)
        self.assertIn("no *-citations.tsv under docs/evidence/", detail)
        self.assertIn("GATE: FAIL", proc.stdout)
        self.assertNotIn("GATE: PASS", proc.stdout)

    def test_the_gate_discloses_a_skipped_citations_step_in_a_partial_verdict(self) -> None:
        """Unrequired, the same SKIP is a disclosed partial run rather than a failure."""
        self.install_checker()
        proc = self.gate()
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        status, detail = self.step_line(proc.stdout, "citations")
        self.assertEqual(status, "SKIP", proc.stdout)
        self.assertIn("no *-citations.tsv under docs/evidence/", detail)
        verdict = [line for line in proc.stdout.splitlines() if line.startswith("GATE:")]
        self.assertEqual(len(verdict), 1, proc.stdout)
        self.assertIn("GATE: PASS (partial:", verdict[0])
        self.assertIn("citations", verdict[0])

    def test_the_gate_step_reads_the_verdict_line_the_checker_printed(self) -> None:
        """Every status maps to the gate's own, the PASS detail is the per-file lines alone, and a
        run that printed no single verdict fails."""
        self.install_checker()
        gate = load_gate()
        gate.ROOT = self.root
        for stdout, rc, expected in (
            ("docs/evidence/0002-citations.tsv: 1 row(s) verified\ncitations: PASS\n", 0, "PASS"),
            ("  x.tsv:4: rule A1: a violation\ncitations: FAIL — 1 violation(s)\n", 1, "FAIL"),
            ("citations: SKIP — no *-citations.tsv under docs/evidence/\n", 2, "SKIP"),
            ("citations: BOUNDARY — no C compiler on PATH (stubbed)\n", 3, "BOUNDARY"),
            ("citations: PASS\n", 1, "FAIL"),  # a PASS that exited non-zero is a defect either way
            ("no verdict line at all\n", 1, "FAIL"),  # a run whose output holds no verdict
            ("citations: PASS\ncitations: FAIL — two of them\n", 0, "FAIL"),  # ambiguous
        ):
            with self.subTest(expected=expected, stdout=stdout):
                with mock.patch.object(
                    gate, "run",
                    side_effect=lambda cmd, env=None, timeout=1800: tool_result(cmd, rc, stdout),
                ):
                    status, detail = gate.step_citations()
                self.assertEqual(status, expected, f"{detail}\n{stdout}")

        # The PASS detail is the per-file lines, and only those: the `citations: `-prefixed lines
        # (the summary and the verdict) are the checker's own accounting. This string is
        # consumer-visible -- the increment's record quotes it -- so it is asserted, not implied.
        stdout = (
            "docs/evidence/0002-citations.tsv: 1 row(s) verified\n"
            "citations: 1 row(s) across 1 file(s) verified\n"
            "citations: PASS\n"
        )
        with mock.patch.object(
            gate, "run",
            side_effect=lambda cmd, env=None, timeout=1800: tool_result(cmd, 0, stdout),
        ):
            status, detail = gate.step_citations()
        self.assertEqual(status, "PASS", stdout)
        self.assertEqual(detail, "docs/evidence/0002-citations.tsv: 1 row(s) verified", stdout)


if __name__ == "__main__":
    unittest.main()
