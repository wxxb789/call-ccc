#!/usr/bin/env python3
"""The CI matrix and the platform claim in ``docs/harness.md`` are one statement.

The workflow is the acceptance surface for what CI verifies: ``.github/workflows/gate.yml``'s matrix
and ``docs/harness.md``'s §CI describe one state of the world — the whole gate runs on the runners the
matrix names, and macOS has no leg (the owner decision of 2026-11-18), so a macOS-shaped rule is
answered only by the suite's own executed host simulations. Before this case existed that claim was
prose with no check; the case now reads both shipped files and fails when either moves alone: a matrix
that grows or drops a leg, a §CI that names a runner the matrix does not hold, a job that bypasses the
matrix, or a §CI that stops stating the gap. It reads files only — no host probe, no tool, no platform
answer — so it holds on every host the suite runs on.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

#: The two shipped files that make the one statement.
WORKFLOW = ROOT / ".github" / "workflows" / "gate.yml"
HARNESS = ROOT / "docs" / "harness.md"

#: The job's own command: the same file the local gate runs (``docs/harness.md``, its first line). The
#: workflow spells the interpreter ``python``, the name every runner it runs on has.
GATE_COMMAND = "python scripts/gate.py"

#: The matrix's legs: every leg runs the whole gate. A change here is a change to the platform claim,
#: and §CI must move with it (this case reads both files).
RUNNERS = ("ubuntu-latest", "windows-latest")

#: The claim §CI must hold while the matrix has no macOS leg: the sentence a reader needs in order not
#: to read a green run as macOS coverage. The section states it in bold, so the phrase is not its line.
NO_MACOS_RUNNER = "macOS is not verified on a runner"

#: The matrix's own declaration: ``os: [a, b]`` (the legs may be quoted; comments are not read).
_MATRIX = re.compile(r"^[ \t]*os:[ \t]*\[([^\]]*)\]", re.MULTILINE)

#: A runner label, as GitHub's own hosted labels are spelled.
_RUNNER_LABEL = re.compile(r"\b[a-z0-9][a-z0-9-]*-latest\b")

#: A ``runs-on:`` value: the matrix reference, or a literal label that would bypass the matrix.
_RUNS_ON = re.compile(r"^[ \t]*runs-on:[ \t]*(\S.*)$", re.MULTILINE)

#: §CI's own heading, exactly (no other ``## `` line opens with it).
_SECTION = re.compile(r"^## CI[ \t]*$", re.MULTILINE)


def matrix_runners(workflow: str) -> list[str] | None:
    """The workflow's matrix legs, in their own order, or ``None`` when it declares no matrix."""
    found = _MATRIX.search(workflow)
    if found is None:
        return None
    legs = [leg.strip().strip("'\"") for leg in found.group(1).split(",")]
    return [leg for leg in legs if leg]


def ci_section(harness: str) -> str:
    """``docs/harness.md``'s §CI body: from its heading to the next ``## `` heading."""
    heading = _SECTION.search(harness)
    if heading is None:
        raise AssertionError("docs/harness.md no longer has a `## CI` section")
    rest = harness[heading.end():]
    following = re.search(r"^## ", rest, re.MULTILINE)
    return rest[:following.start()] if following else rest


class CiMatrixAndThePlatformClaim(unittest.TestCase):
    """§CI's claim and the matrix are read from the shipped files, so one cannot move alone."""

    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")
        self.section = ci_section(HARNESS.read_text(encoding="utf-8"))
        self.legs = matrix_runners(self.workflow)

    def test_the_matrix_is_exactly_the_platforms_the_claim_names(self) -> None:
        """Every leg runs the whole gate; a leg added or dropped here is a changed platform claim."""
        self.assertEqual(
            self.legs, list(RUNNERS),
            "the matrix's legs are the platforms docs/harness.md §CI claims: the two files move in "
            "the same increment (owner decision 2026-11-18 removed the macOS leg), or this check and "
            "the docs/plans/harness-ci-matrix.md decision change together")
        self.assertIn(
            GATE_COMMAND, self.workflow,
            f"the job must run the gate file itself ({GATE_COMMAND!r}), which is what §CI claims")

    def test_no_job_bypasses_the_matrix(self) -> None:
        """A literal ``runs-on:`` label would run something §CI's claim does not name."""
        literal = sorted({value.strip() for value in _RUNS_ON.findall(self.workflow)
                          if value.strip() != "${{ matrix.os }}"})
        self.assertEqual(
            literal, [],
            "a job runs on a runner outside the matrix; §CI's claim is a statement about what the "
            "matrix's legs run, so a job beside it makes the claim untrue")

    def test_the_claim_names_exactly_the_matrix_s_runners(self) -> None:
        """§CI names the runners the matrix holds — no runner it does not run, none left out."""
        named = set(_RUNNER_LABEL.findall(self.section))
        self.assertEqual(
            named, set(RUNNERS),
            "docs/harness.md §CI must name exactly the runners the workflow's matrix holds; a section "
            "naming a runner the matrix lacks (or omitting one it has) is a claim CI does not verify")

    def test_the_section_states_the_platform_the_matrix_does_not_cover(self) -> None:
        """A two-legged matrix leaves macOS unverified, and §CI must say so where a reader looks."""
        self.assertIn(
            NO_MACOS_RUNNER, self.section,
            "docs/harness.md §CI must state the gap while the matrix has no macOS leg: a green run "
            f"covers the runners above only, and the section has to say so ({NO_MACOS_RUNNER!r})")


if __name__ == "__main__":
    unittest.main()
