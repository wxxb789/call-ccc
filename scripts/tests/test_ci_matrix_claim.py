#!/usr/bin/env python3
"""The CI matrix and the platform claim in ``docs/harness.md`` are one statement.

The workflow is the acceptance surface for what CI verifies: every workflow file GitHub reads from
``.github/workflows/`` and ``docs/harness.md``'s two live copies of the claim — the §Platform boundary
paragraph that says where each answer is verified, and §CI — describe one state of the world: the whole
gate runs on the runners the matrix names, and macOS has no leg (the owner decision of 2026-11-18), so a
macOS-shaped rule is answered only by the suite's own executed host simulations. This case reads the
shipped files and fails when either side moves alone: a matrix that grows or drops a leg (its ``os:``
list, an ``include:`` or ``exclude:`` entry), a second matrix, a ``uses:`` job (a called or remote
workflow is not read here), a job that runs on a literal runner instead of the matrix, a section naming
a runner the matrix does not hold, a gate step that no longer runs the gate file, or a section that
stops stating the gap. A shape the reader does not understand is refused rather than read past, so a
file the pin cannot certify fails it instead of passing. It reads files only — no host probe, no tool,
no platform answer — so it holds on every host the suite runs on.
"""

from __future__ import annotations

import re
import unittest
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

#: The workflows directory: every file GitHub reads from it is read here too, so a job cannot run in a
#: file this pin never opened.
WORKFLOW_DIR = ROOT / ".github" / "workflows"

#: The gate workflow: the file §CI names, whose matrix is the claim's own.
WORKFLOW = WORKFLOW_DIR / "gate.yml"

HARNESS = ROOT / "docs" / "harness.md"

#: The job's own command: the same file the local gate runs (``docs/harness.md``, its first line). The
#: workflow spells the interpreter ``python``, the name every runner it runs on has.
GATE_COMMAND = "python scripts/gate.py"

#: The matrix's legs: every leg runs the whole gate. A change here is a change to the platform claim,
#: and both live copies of the claim (the platform rules and §CI) must move with it: this pin reads all
#: of them.
RUNNERS = ("ubuntu-latest", "windows-latest")

#: The value a job's ``runs-on:`` may hold: the matrix reference. A literal label would make the job's
#: runner a fact outside the matrix the claim is read from.
MATRIX_REFERENCE = "${{ matrix.os }}"

#: The label the matrix no longer holds. The platform rules' paragraph may still name it — in the
#: sentence recording what the leg's failures forced — and this pin reads that mention as the history
#: it is while refusing every other runner outside the matrix.
RETIRED_RUNNER = "macos-latest"

#: The claim the two sections must hold while the matrix has no macOS leg: the sentence a reader needs
#: in order not to read a green run as macOS coverage. The sections state it in bold, so the phrase is
#: not their line.
NO_MACOS_RUNNER = "macOS is not verified on a runner"

#: A runner label, as GitHub's own hosted labels are spelled.
_RUNNER_LABEL = re.compile(r"\b[a-z0-9][a-z0-9-]*-latest\b")

#: A ``runs-on:`` value: the matrix reference, or a literal label that would bypass the matrix.
_RUNS_ON = re.compile(r"^[ \t]*runs-on:[ \t]*(\S.*)$", re.MULTILINE)

#: A ``run:`` value: the step's own command (a commented-out command is not a run).
_RUN = re.compile(r"^[ \t]*run:[ \t]*(\S.*)$", re.MULTILINE)

#: A ``key: value`` line of a workflow, and a list item's body (``- key: value``).
_KEY = re.compile(r"^(?P<key>[A-Za-z_][A-Za-z0-9_.-]*):[ \t]*(?P<value>.*?)[ \t]*$")
_ITEM = re.compile(r"^[ \t]*-[ \t]*(?P<body>.*)$")


def _key(body: str) -> tuple[str, str] | None:
    """``(key, value)`` when a line's body is a ``key: value`` line, ``None`` otherwise."""
    match = _KEY.match(body)
    return (match.group("key"), match.group("value")) if match else None


class UnreadableWorkflow(AssertionError):
    """A workflow shape this pin refuses to read around.

    Refusing fails the pin; guessing would pass it without holding the claim it exists for. The shapes
    refused are the ones the reader cannot enumerate: a second ``matrix:``, an ``os:`` it cannot place
    or parse, an ``include:``/``exclude:`` entry that is not ``- os: <label>``, and a ``uses:`` job —
    a called or remote workflow is not read here.
    """


@dataclass(frozen=True)
class Workflow:
    """What this pin reads out of one workflow file."""

    path: Path
    legs: tuple[str, ...] | None      # the matrix's effective legs, or None when it declares none
    literal_runs_on: tuple[str, ...]  # every ``runs-on:`` value that is not the matrix reference


def workflow_files() -> list[Path]:
    """Every workflow file GitHub reads from ``.github/workflows/``: ``*.yml`` and ``*.yaml``."""
    return sorted({*WORKFLOW_DIR.glob("*.yml"), *WORKFLOW_DIR.glob("*.yaml")})


def structural_lines(text: str) -> list[tuple[int, int, str]]:
    """``(line number, indent, body)`` for every line holding workflow structure.

    Comments and blank lines are dropped — the shipped workflow comments the matrix and the steps, and
    a comment must not count as a key — with ``#`` opening a comment only at the line's start or after
    whitespace, the way YAML spells one.
    """
    lines = []
    for number, raw in enumerate(text.splitlines(), start=1):
        code = re.sub(r"^#.*$|(?<=[ \t])#.*$", "", raw).rstrip()
        if not code.strip():
            continue
        lines.append((number, len(code) - len(code.lstrip(" \t")), code.strip()))
    return lines


def run_commands(text: str) -> list[str]:
    """Every ``run:`` line's own command, in file order (a commented-out command is not a run)."""
    return [re.sub(r"(?<=[ \t])#.*$", "", value).strip().strip("'\"")
            for value in _RUN.findall(text)]


def runner_label(path: Path, number: int, value: str) -> str:
    """A runner label as the pin reads one: a bare scalar, quotes stripped, nothing else."""
    label = value.strip().strip("'\"")
    if not label or re.search(r"[\s\[\]{}#]", label):
        raise UnreadableWorkflow(
            f"{path}: line {number}: a runner label this pin cannot read ({value!r}) — it reads a "
            f"bare label like `{RETIRED_RUNNER}`")
    return label


def os_legs(path: Path, number: int, value: str) -> list[str]:
    """An ``os: [label, …]`` list's legs, deduplicated in order; refuses any other form."""
    match = re.fullmatch(r"\[(?P<labels>.*)\]", value.strip())
    if match is None:
        raise UnreadableWorkflow(
            f"{path}: line {number}: an `os:` this pin cannot read ({value!r}) — the pin reads the "
            f"inline list form (`os: [label, …]`)")
    legs: list[str] = []
    for part in match.group("labels").split(","):
        if not part.strip():
            continue
        label = runner_label(path, number, part)
        if label not in legs:
            legs.append(label)
    if not legs:
        raise UnreadableWorkflow(
            f"{path}: line {number}: an `os:` this pin cannot read ({value!r}) — the list holds no "
            f"label")
    return legs


def read_workflow(path: Path, text: str) -> Workflow:
    """Read one workflow file: its matrix's effective legs and every literal ``runs-on:`` value.

    Refuses (``UnreadableWorkflow``) every shape it cannot enumerate instead of reading past it, so no
    runner reaches a job outside what this function answers: a second ``matrix:``, an ``os:`` outside
    the one matrix or in a form that is not the inline ``[label, …]`` list, an
    ``include:``/``exclude:`` entry that is not ``- os: <label>``, a key inside the matrix that is
    neither ``os:``, ``include:`` nor ``exclude:``, and a ``uses:`` whose target is a workflow file (a
    called or remote workflow is not read here).
    """
    lines = structural_lines(text)
    literal_runs_on = tuple(
        value.strip() for value in _RUNS_ON.findall(text) if value.strip() != MATRIX_REFERENCE)

    matrices: list[tuple[int, str]] = []
    for index, (_number, _indent, body) in enumerate(lines):
        parsed = _key(body)
        if parsed is not None and parsed[0] == "matrix":
            matrices.append((index, parsed[1]))
    if len(matrices) > 1:
        listed = ", ".join(str(lines[index][0]) for index, _ in matrices)
        raise UnreadableWorkflow(
            f"{path}: `matrix:` keys at lines {listed} — a second `strategy.matrix` is a shape this "
            f"pin refuses: it reads one matrix per workflow and cannot say which one a job's "
            f"`runs-on:` reaches")
    if matrices and matrices[0][1]:
        raise UnreadableWorkflow(
            f"{path}: line {lines[matrices[0][0]][0]}: an inline `matrix:` — the pin reads the block "
            f"form (`matrix:` then its `os:`/`include:`/`exclude:` lines)")

    if not matrices:
        matrix_index, matrix_indent, region_end = None, -1, 0
    else:
        matrix_index = matrices[0][0]
        matrix_indent = lines[matrix_index][1]
        region_end = matrix_index + 1
        while region_end < len(lines) and lines[region_end][1] > matrix_indent:
            region_end += 1

    # Every `os:`, `include:` and `exclude:` must be the one matrix's own: anywhere else the pin
    # cannot place it, and reading past one would let a runner outside the matrix go unnoticed.
    for index, (number, _indent, body) in enumerate(lines):
        parsed = _key(body)
        if parsed is None or parsed[0] not in ("os", "include", "exclude"):
            continue
        if matrix_index is None or not matrix_index < index < region_end:
            raise UnreadableWorkflow(
                f"{path}: line {number}: an `{parsed[0]}:` outside the matrix — the pin reads the "
                f"matrix's own `os:`, `include:` and `exclude:` and refuses one it cannot place")

    legs: list[str] = []
    os_line: int | None = None
    if matrix_index is not None:
        index = matrix_index + 1
        while index < region_end:
            number, indent, body = lines[index]
            parsed = _key(body)
            if parsed is None:
                raise UnreadableWorkflow(
                    f"{path}: line {number}: a list item inside the matrix outside an "
                    f"`include:`/`exclude:` block — the pin cannot place it")
            key, value = parsed
            if key == "os":
                legs = os_legs(path, number, value)
                os_line = number
                index += 1
            elif key in ("include", "exclude"):
                if value:
                    raise UnreadableWorkflow(
                        f"{path}: line {number}: an inline `{key}:` — the pin reads the block form "
                        f"(`{key}:` then its `- os: <label>` entries)")
                index += 1
                claimed = False
                while index < region_end and lines[index][1] > indent:
                    entry_number, _entry_indent, entry_body = lines[index]
                    entry = _ITEM.match(entry_body)
                    if entry is None:
                        if not claimed:
                            raise UnreadableWorkflow(
                                f"{path}: line {entry_number}: an `{key}:` block this pin cannot read "
                                f"— it reads `- os: <label>` entries")
                        # a continuation of the previous entry's own map; it names no os
                        index += 1
                        continue
                    claimed = True
                    entry_key = _key(entry.group("body").strip())
                    if entry_key is None or entry_key[0] != "os" or not entry_key[1].strip():
                        raise UnreadableWorkflow(
                            f"{path}: line {entry_number}: an `{key}:` entry this pin cannot read "
                            f"({entry_body!r}) — it reads `- os: <label>`")
                    label = runner_label(path, entry_number, entry_key[1])
                    if key == "include":
                        if label not in legs:
                            legs.append(label)
                    elif label in legs:
                        legs.remove(label)
                    index += 1
            else:
                raise UnreadableWorkflow(
                    f"{path}: line {number}: a key inside the matrix this pin does not read "
                    f"(`{key}:`) — the pin reads `os:`, `include:` and `exclude:`")
        if os_line is None:
            raise UnreadableWorkflow(
                f"{path}: line {lines[matrix_index][0]}: a `matrix:` with no `os:` — the pin reads "
                f"the legs from it and refuses one it cannot enumerate")

    for number, _indent, body in lines:
        parsed = _key(body)
        if parsed is None or parsed[0] != "uses":
            continue
        target = parsed[1].strip()
        if ".github/workflows/" in target:
            raise UnreadableWorkflow(
                f"{path}: line {number}: `uses: {target}` — a `uses:` job hands its run to a called or "
                f"remote workflow, which this pin cannot read: it refuses instead of certifying a run "
                f"it cannot see")

    return Workflow(path, tuple(legs) if matrix_index is not None else None, literal_runs_on)


def section_body(harness: str, title: str) -> str:
    """``docs/harness.md``'s ``## <title>`` body: from its heading to the next ``## `` heading."""
    heading = re.search(rf"^## {re.escape(title)}[ \t]*$", harness, re.MULTILINE)
    if heading is None:
        raise AssertionError(f"docs/harness.md no longer has a `## {title}` section")
    rest = harness[heading.end():]
    following = re.search(r"^## ", rest, re.MULTILINE)
    return rest[:following.start()] if following else rest


def ci_section(harness: str) -> str:
    """``docs/harness.md``'s §CI body: the section the CI claim lives in."""
    return section_body(harness, "CI")


def platform_rules_paragraph(harness: str) -> str:
    """The §Platform boundary paragraph that states the gap — the claim's other live copy.

    §CI is not the claim's only live copy: the paragraph that says where each answer is verified states
    the same gap and the same runners, so a matrix change cannot leave either copy stating a state CI
    no longer has.
    """
    for paragraph in section_body(harness, "Platform boundary").split("\n\n"):
        if NO_MACOS_RUNNER in paragraph:
            return paragraph
    raise AssertionError(
        f"docs/harness.md §Platform boundary no longer states the gap ({NO_MACOS_RUNNER!r}): the "
        f"platform rules and the matrix are one claim, and this pin cannot read the rules without it")


class CiMatrixAndThePlatformClaim(unittest.TestCase):
    """§CI's claim and the matrix are read from the shipped files, so one cannot move alone."""

    def setUp(self) -> None:
        self.harness = HARNESS.read_text(encoding="utf-8")
        self.section = ci_section(self.harness)
        self.texts = {path: path.read_text(encoding="utf-8") for path in workflow_files()}
        self.workflows = {path: read_workflow(path, text) for path, text in self.texts.items()}
        self.assertIn(
            WORKFLOW, self.workflows,
            f"the gate workflow {WORKFLOW.relative_to(ROOT)} is not among the workflow files this pin "
            f"read (found: {sorted(str(path.relative_to(ROOT)) for path in self.workflows)})")

    def test_the_matrix_is_exactly_the_platforms_the_claim_names(self) -> None:
        """Every leg runs the whole gate; a leg added or dropped here is a changed platform claim."""
        gate = self.workflows[WORKFLOW]
        self.assertEqual(
            list(gate.legs or ()), list(RUNNERS),
            "the matrix's legs are the platforms docs/harness.md §CI claims: the `os:` list and every "
            "`include:`/`exclude:` entry are read, so the two files move in the same increment (owner "
            "decision 2026-11-18 removed the macOS leg), or this check and the "
            "docs/plans/harness-ci-matrix.md decision change together")
        for path, workflow in sorted(self.workflows.items()):
            if path == WORKFLOW:
                continue
            self.assertEqual(
                sorted(set(workflow.legs or ()) - set(RUNNERS)), [],
                f"{path.relative_to(ROOT)} runs a matrix leg outside the runners the claim names "
                f"({RUNNERS!r}): every workflow file is read, not only the gate's")
        self.assertIn(
            GATE_COMMAND, run_commands(self.texts[WORKFLOW]),
            f"a step of the gate job must run the gate file itself, as the whole command of a `run:` "
            f"line ({GATE_COMMAND!r}): §CI claims CI runs that file, and a commented-out or replaced "
            f"command is not a run")

    def test_no_job_bypasses_the_matrix(self) -> None:
        """A literal ``runs-on:`` label would run a job the matrix's legs do not account for."""
        bypassing = [(str(path.relative_to(ROOT)), value)
                     for path, workflow in sorted(self.workflows.items())
                     for value in workflow.literal_runs_on]
        self.assertEqual(
            bypassing, [],
            "a job runs on a runner outside the matrix; §CI's claim is a statement about what the "
            "matrix's legs run, so a job beside it — in any workflow file — makes the claim untrue")

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

    def test_the_platform_rules_state_the_gap_and_the_runners_too(self) -> None:
        """The platform rules' paragraph is the claim's other live copy, and it moves with the matrix."""
        paragraph = platform_rules_paragraph(self.harness)
        named = set(_RUNNER_LABEL.findall(paragraph))
        self.assertTrue(
            named - set(RUNNERS) <= {RETIRED_RUNNER},
            "docs/harness.md §Platform boundary's paragraph names a runner the matrix does not hold "
            f"(only the retired `{RETIRED_RUNNER}`, kept by its history sentence, may remain): a matrix "
            f"change cannot leave the platform rules stating a state CI no longer has")
        self.assertTrue(
            set(RUNNERS) <= named,
            "docs/harness.md §Platform boundary's paragraph must name every runner the matrix holds "
            "(the same list §CI names), so the claim's two live copies state one thing")

    def test_the_reader_refuses_a_shape_it_cannot_certify(self) -> None:
        """A shape the pin cannot read is refused, never read past: the reader's own contract."""
        shapes = {
            "a second `strategy.matrix`": (
                "jobs:\n"
                "  gate:\n"
                "    strategy:\n"
                "      matrix:\n"
                "        os: [ubuntu-latest, windows-latest]\n"
                "  other:\n"
                "    strategy:\n"
                "      matrix:\n"
                "        os: [macos-latest]\n"),
            "an `os:` outside the matrix": (
                "jobs:\n"
                "  gate:\n"
                "    runs-on: ${{ matrix.os }}\n"
                "    os: [macos-latest]\n"),
            "an `os:` this pin cannot read": (
                "jobs:\n"
                "  gate:\n"
                "    strategy:\n"
                "      matrix:\n"
                "        os:\n"
                "          - ubuntu-latest\n"),
            "a `matrix:` with no `os:`": (
                "jobs:\n"
                "  gate:\n"
                "    strategy:\n"
                "      matrix:\n"
                "        include:\n"
                "          - os: ubuntu-latest\n"),
            "a key inside the matrix this pin does not read": (
                "jobs:\n"
                "  gate:\n"
                "    strategy:\n"
                "      matrix:\n"
                "        os: [ubuntu-latest]\n"
                "        python: ['3.12']\n"),
            "a `uses:` job": (
                "jobs:\n"
                "  call:\n"
                "    uses: ./.github/workflows/gate.yml\n"),
        }
        for reason, text in shapes.items():
            with self.subTest(shape=reason):
                with self.assertRaises(UnreadableWorkflow) as refused:
                    read_workflow(WORKFLOW, text)
                self.assertIn(reason, str(refused.exception))


if __name__ == "__main__":
    unittest.main()
