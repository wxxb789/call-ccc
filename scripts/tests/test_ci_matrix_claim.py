#!/usr/bin/env python3
"""The CI matrix and the platform claim in ``docs/harness.md`` are one statement.

The workflow is the acceptance surface for what CI verifies: every workflow file GitHub reads from
``.github/workflows/`` and ``docs/harness.md``'s two live copies of the claim — the §Platform boundary
paragraph that says where each answer is verified, and §CI — describe one state of the world: the whole
gate runs on the runners the matrix names, and macOS has no leg (the owner decision of 2026-11-18), so a
macOS-shaped rule is answered only by the suite's own executed host simulations. This case reads the
shipped files and fails when either side moves alone: a matrix that grows or drops a leg (its ``os:``
list, an ``include:`` or ``exclude:`` entry), a job that runs on a literal runner instead of the matrix,
a section naming a runner the matrix does not hold, a trigger or a step that narrows or skips what §CI
claims, or a section that stops stating the gap.

The reader fails closed: it reads a workflow structurally — the trigger block, the one matrix (its
``os:`` list and every ``include:``/``exclude:`` entry, combined as ``os ∪ include − exclude`` whatever
their order), each job's ``runs-on: ${{ matrix.os }}``, each job's steps, and every step's own ``run:``
value — and refuses, as a failing case naming the shape, everything it does not read exactly: another
spelling or value of ``runs-on``, a block scalar, a quoted or block label list, a step-level
``if:``/``continue-on-error:``, a trigger block outside the shape §CI states, a second ``matrix:``, a
job, step or matrix key it does not know. Reading past such a shape would pass this pin without holding
the claim it exists for.

Two limits are the pin's own, stated rather than hidden. It reads raw text, so it cannot see Markdown
rendering: wrapping the claim paragraphs in an HTML comment satisfies every case while the rendered
document loses them. And nothing pins this module's own existence — the ``selftest`` step discovers
``test_*.py``, so deleting or renaming this file leaves the gate green; the increment's record says why
no honest home for that check was found. It reads files only — no host probe, no tool, no platform
answer — so it holds on every host the suite runs on.
"""

from __future__ import annotations

import re
import unittest
from dataclasses import dataclass
from pathlib import Path
from typing import NoReturn

ROOT = Path(__file__).resolve().parents[2]

#: The workflows directory: every file GitHub reads from it is read here too, so a job cannot run in a
#: file this pin never opened.
WORKFLOW_DIR = ROOT / ".github" / "workflows"

#: The gate workflow: the file §CI names, whose matrix is the claim's own.
WORKFLOW = WORKFLOW_DIR / "gate.yml"

HARNESS = ROOT / "docs" / "harness.md"

#: The job's own command: the same file the local gate runs (``docs/harness.md``, its first line). The
#: workflow spells the interpreter ``python``, the name every runner it runs on has. A step must run it
#: as that step's whole ``run:`` value: a line that merely contains the text is not a step that runs it.
GATE_COMMAND = "python scripts/gate.py"

#: The matrix's legs: every leg runs the whole gate. A change here is a change to the platform claim,
#: and both live copies of the claim (the platform rules and §CI) must move with it: this pin reads all
#: of them.
RUNNERS = ("ubuntu-latest", "windows-latest")

#: The value a job's ``runs-on:`` may hold: the matrix reference. Every other value — a literal label, an
#: empty value, a block sequence — is refused: it would make a job's runner a fact outside the matrix the
#: claim is read from.
MATRIX_REFERENCE = "${{ matrix.os }}"

#: The label the matrix no longer holds. The platform rules' paragraph may still name it — in a sentence
#: recording what the leg's failures forced — and this pin reads that mention as the history it is while
#: refusing every other runner outside the matrix.
RETIRED_RUNNER = "macos-latest"

#: The claim the two sections must hold while the matrix has no macOS leg: the sentence a reader needs
#: in order not to read a green run as macOS coverage. The sections state it in bold, so the phrase is
#: not their line.
NO_MACOS_RUNNER = "macOS is not verified on a runner"

#: A runner label, as GitHub's own hosted labels are spelled: a family (``ubuntu``/``windows``/
#: ``macos``) with a version or ``latest``. Both families a matrix could name — ``macos-14`` as much as
#: ``macos-latest`` — are read, so a document sentence naming one the matrix does not hold is caught.
_RUNNER_LABEL = re.compile(r"\b(?:ubuntu|windows|macos)-(?:latest|[0-9][0-9.]*)\b")

#: The markers a §Platform sentence naming a runner the matrix does not hold must carry: the mention is
#: then the history it is (a leg's own red at a past head, the repair it forced), not a present-tense
#: claim about CI. Presence alone is not enough — ``macos-latest is green again`` must fail.
_HISTORY_MARKER = re.compile(r"while the leg ran|as recorded history|head `[0-9a-f]{7,}`")

#: A ``key: value`` line of a workflow, and a list item's body (``- key: value``).
_KEY = re.compile(r"^(?P<key>[A-Za-z_][A-Za-z0-9_.-]*):[ \t]*(?P<value>.*?)[ \t]*$")
_ITEM = re.compile(r"^[ \t]*-[ \t]*(?P<body>.*)$")

#: A ``key: value``-shaped line that the strict ``_KEY`` spelling does not accept (a quoted key, a space
#: before the colon). A line spelling a key this pin reads in such a form is refused by name instead of
#: slipping past the walk, which is how ``'runs-on':`` and ``runs-on :`` used to be invisible.
_LOOSE_KEY = re.compile(r"""^["']?(?P<key>[A-Za-z_][A-Za-z0-9_.-]*)["']?[ \t]*:""")

#: The keys this pin reads (at their own positions): a loose spelling of any of them is refused by name.
_READ_KEYS = frozenset({
    "name", "on", "permissions", "jobs", "uses", "if", "continue-on-error", "runs-on", "strategy",
    "fail-fast", "matrix", "os", "include", "exclude", "steps", "run", "with", "pull_request", "push",
    "branches",
})

#: A YAML block scalar's indicator (``|``, ``|-``, ``>+``, ``|2-`` …): a value this pin refuses wherever
#: it meets one, because the text under it is not the one-line value every check here reads.
_BLOCK_SCALAR = re.compile(r"[|>][0-9]*[+-]?")

#: A runner label's own spelling, after quotes are stripped: alphanumerics with ``.`` and ``-``. A token
#: holding a comma, a space, a bracket or any other character is refused — a quoted scalar that GitHub
#: would take for one unrecognized label must not be read as two.
_LABEL_TOKEN = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]*")


def _key(body: str) -> tuple[str, str] | None:
    """``(key, value)`` when a line's body is a ``key: value`` line, ``None`` otherwise."""
    match = _KEY.match(body)
    return (match.group("key"), match.group("value")) if match else None


class UnreadableWorkflow(AssertionError):
    """A workflow shape this pin refuses to read around.

    Refusing fails the pin; guessing would pass it without holding the claim it exists for. The reader
    refuses every shape it cannot enumerate exactly — a second job or matrix, an ``os:`` it cannot place
    or parse, a block scalar, a trigger block beyond the accepted shape, a job or step key it does not
    know, a ``runs-on`` spelled or valued otherwise, a quoted or block label list — and every refusal
    names the line and the shape, so a mutation that reaches one fails the case that covers it.
    """


@dataclass(frozen=True)
class Line:
    """One structural line: its file line number, its indent, and its body (comments stripped)."""

    number: int
    indent: int
    body: str


@dataclass(frozen=True)
class Child:
    """One mapping entry of a structural region: ``key: value``, or a ``- key: value`` list item.

    ``index``/``end`` are the entry's own line index and the first line index after its value region
    (the lines indented deeper than the entry): a scalar entry's region must be empty, and a block
    entry's region is what its reader walks.
    """

    number: int
    index: int
    end: int
    key: str
    value: str
    item: bool


@dataclass(frozen=True)
class Workflow:
    """What this pin reads out of one workflow file."""

    path: Path
    legs: tuple[str, ...]  # the matrix's effective legs: os ∪ include − exclude, in order
    runs: tuple[str, ...]  # every step's whole `run:` value, in the job's own order


def workflow_files() -> list[Path]:
    """Every workflow file GitHub reads from ``.github/workflows/``: ``*.yml`` and ``*.yaml``."""
    return sorted({*WORKFLOW_DIR.glob("*.yml"), *WORKFLOW_DIR.glob("*.yaml")})


def structural_lines(text: str) -> list[Line]:
    """The file's structural lines: comments and blank lines dropped, the rest stripped.

    With ``#`` opening a comment only at the line's start or after whitespace, the way YAML spells one,
    so the shipped workflow's comments — the matrix's own among them — never count as keys.
    """
    lines = []
    for number, raw in enumerate(text.splitlines(), start=1):
        code = re.sub(r"^#.*$|(?<=[ \t])#.*$", "", raw).rstrip()
        if not code.strip():
            continue
        lines.append(Line(number, len(code) - len(code.lstrip(" \t")), code.strip()))
    return lines


class _WorkflowReader:
    """A structural walk over one workflow, refusing every shape it cannot read exactly.

    The walk reads the mapping regions GitHub's own workflow shape spells — the top level, ``on:``,
    ``permissions:``, ``jobs:``, each job, ``strategy:``, ``matrix:`` and ``steps:`` — and hands each
    key's value region to the reader that owns it. A line that is not at the indent its region expects,
    a key its position does not read, an item outside a block sequence, a value region under a key the
    pin reads as a scalar: each is refused with its line number, never read past.
    """

    def __init__(self, path: Path, lines: list[Line]) -> None:
        self.path = path
        self.lines = lines

    def refuse(self, number: int, message: str) -> NoReturn:
        raise UnreadableWorkflow(f"{self.path}: line {number}: {message}")

    def _value_end(self, index: int, end: int) -> int:
        """The first line index after ``index``'s value region: its deeper-indented lines."""
        base = self.lines[index].indent
        following = index + 1
        while following < end and self.lines[following].indent > base:
            following += 1
        return following

    def child(self, line: Line, index: int, end: int, context: str) -> Child:
        """One region entry: a strict ``key: value`` line or a ``- key: value`` item, or a refusal.

        A line that spells a key this pin reads — ``runs-on``, ``matrix``, ``os``… — in a form the
        strict spelling does not accept is refused by that key's name: a quoted key or a space before
        the colon must not render a line invisible to the walk. A ``runs-on`` entry whose value is not
        exactly the matrix reference is refused here, wherever it stands.
        """
        parsed = _key(line.body)
        item = False
        if parsed is None:
            body = _ITEM.match(line.body)
            if body is not None:
                parsed = _key(body.group("body").strip())
                item = parsed is not None
        if parsed is not None:
            if parsed[0] == "runs-on" and parsed[1].strip() != MATRIX_REFERENCE:
                self.refuse(line.number, (
                    f"a `runs-on:` that is not exactly `{MATRIX_REFERENCE}` ({parsed[1].strip()!r}) — "
                    f"§CI's claim is about the runners the matrix's legs name, and a job whose runner is "
                    f"spelled anywhere else bypasses it"))
            return Child(line.number, index, self._value_end(index, end), parsed[0], parsed[1], item)
        loose = _LOOSE_KEY.match(line.body)
        if loose is not None and loose.group("key") in _READ_KEYS:
            self.refuse(line.number, (
                f"a `{loose.group('key')}` spelled in a form this pin cannot read ({line.body!r}) inside "
                f"{context} — a quoted key or a space before the colon is refused: the pin reads "
                f"`{loose.group('key')}: <value>`"))
        self.refuse(line.number, (
            f"a line inside {context} this pin cannot read as `key: value` or `- key: value` "
            f"({line.body!r})"))

    def children(self, start: int, end: int, indent: int, context: str) -> list[Child]:
        """A mapping region's entries: every entry at ``indent``, each skipping its value region."""
        entries = []
        index = start
        while index < end:
            line = self.lines[index]
            if line.indent != indent:
                self.refuse(line.number, (
                    f"a line inside {context} this pin cannot place (indent {line.indent}, want "
                    f"{indent}): {line.body!r}"))
            entries.append(self.child(line, index, end, context))
            index = self._value_end(index, end)
        return entries

    def block_children(self, parent: Child, context: str) -> list[Child]:
        """A key's block value: the entries of its value region (``[]`` when the region is empty)."""
        start, end = parent.index + 1, parent.end
        if start >= end:
            return []
        return self.children(start, end, self.lines[start].indent, context)

    def item_entries(self, item: Child, context: str) -> list[Child]:
        """A list item's mapping: the item's own key, then its region's entries as siblings."""
        first = Child(item.number, item.index, item.index + 1, item.key, item.value, False)
        return [first, *self.block_children(item, context)]

    def scalar(self, entry: Child, what: str) -> str:
        """A key this pin reads as one inline scalar; refuses a block scalar or a block value."""
        value = entry.value.strip()
        if _BLOCK_SCALAR.fullmatch(value):
            self.refuse(entry.number, (
                f"a `{entry.key}:` with a block scalar (`{value}`) — the pin reads one line, and the "
                f"text under a block scalar is not the value any check here can certify"))
        if entry.end > entry.index + 1:
            self.refuse(entry.number, (
                f"{what} with a block value — the pin reads an inline scalar (`{entry.key}: <value>`)"))
        if not value:
            self.refuse(entry.number, f"a `{entry.key}:` with no value — {what} must be a scalar")
        return value

    def inline_tokens(self, entry: Child, what: str) -> list[str]:
        """An inline ``[a, b]`` list's tokens, split respecting quotes; refuses any other form.

        Quotes are removed from each token, so a token holding a comma is the one token GitHub would
        read as one label — the caller validates its spelling — and a quoted scalar holding a comma is
        never silently split into two. A quote that is never closed is refused, as is any value that is
        not the inline list form.
        """
        value = entry.value.strip()
        match = re.fullmatch(r"\[(?P<items>.*)\]", value)
        if match is None:
            self.refuse(entry.number, (
                f"{what} this pin cannot read ({value!r}) — the pin reads the inline list form "
                f"(`[label, …]`), and refuses a block list or another spelling"))
        tokens, quote, buf = [], None, ""
        for char in match.group("items"):
            if quote is not None:
                if char == quote:
                    quote = None
                else:
                    buf += char
            elif char in "'\"":
                quote = char
            elif char == ",":
                tokens.append(buf)
                buf = ""
            else:
                buf += char
        if quote is not None:
            self.refuse(entry.number, f"{what} this pin cannot read ({value!r}) — a quote in it is never closed")
        tokens.append(buf)
        return [token.strip() for token in tokens if token.strip()]

    def label_token(self, entry: Child, token: str, what: str) -> str:
        """A list token as a runner label's own spelling, or a refusal naming the token."""
        if not _LABEL_TOKEN.fullmatch(token):
            self.refuse(entry.number, (
                f"{what} this pin cannot read ({token!r}) — a quoted scalar holding a comma or a space, "
                f"or any other spelling outside a label's own characters, is refused"))
        return token

    def read(self) -> Workflow:
        """Read the workflow whole: its trigger block, its matrix's legs, its job's steps."""
        top = self.children(0, len(self.lines), 0, "the workflow")
        if not top:
            self.refuse(1, "an empty workflow file — the pin reads the gate workflow, and a file with no keys is not one")
        seen: dict[str, Child] = {}
        for entry in top:
            if entry.item:
                self.refuse(entry.number, (
                    f"a list item at the top level of the workflow ({entry.key}: {entry.value}) — the pin "
                    f"reads the workflow as its `key: value` lines"))
            if entry.key in seen:
                self.refuse(entry.number, f"a second `{entry.key}:` at the top level of the workflow")
            seen[entry.key] = entry
        for required in ("on", "jobs"):
            if required not in seen:
                self.refuse(1, f"a workflow without `{required}:` — the pin reads the gate workflow's triggers and its job")
        run_values: tuple[str, ...] = ()
        legs: tuple[str, ...] = ()
        for entry in top:
            if entry.key == "name":
                self.scalar(entry, "the workflow's `name:`")
            elif entry.key == "on":
                self.read_triggers(entry)
            elif entry.key == "permissions":
                self.read_permissions(entry)
            elif entry.key == "jobs":
                legs, run_values = self.read_jobs(entry)
            else:
                self.refuse(entry.number, (
                    f"a top-level key this pin does not read (`{entry.key}:`) — the pin reads `name`, "
                    f"`on`, `permissions` and `jobs` and refuses one it cannot place"))
        return Workflow(self.path, legs, run_values)

    def read_triggers(self, on_key: Child) -> None:
        """``on:`` — the trigger shape §CI's sentence states: every pull request, every push to main.

        The pin reads the block form with exactly ``pull_request:`` (bare) and ``push:`` (whose
        ``branches:`` list must hold ``main``). A filter under either trigger — ``paths-ignore:``,
        ``branches-ignore:``, an inline value — narrows a claim §CI states as universal, so it is
        refused rather than read as if it changed nothing; any other trigger is a shape the pin does not
        read.
        """
        if on_key.value.strip():
            self.refuse(on_key.number, (
                f"an inline `on:` ({on_key.value!r}) — the pin reads the block form (`on:` then its "
                f"`pull_request:`/`push:` lines)"))
        entries = self.block_children(on_key, "the `on:` block")
        if not entries:
            self.refuse(on_key.number, "an `on:` with no trigger — §CI states what the workflow runs for, and the pin reads that statement")
        keys: dict[str, Child] = {}
        for entry in entries:
            if entry.item:
                self.refuse(entry.number, "a list item inside `on:` — the pin reads the trigger block's `key: value` lines")
            if entry.key in keys:
                self.refuse(entry.number, f"a second `{entry.key}:` trigger")
            keys[entry.key] = entry
            if entry.key not in ("pull_request", "push"):
                self.refuse(entry.number, (
                    f"a trigger this pin does not read (`{entry.key}:`) — §CI's sentence is about every "
                    f"pull request and every push to `main`, and the pin refuses a trigger it cannot "
                    f"place in that statement"))
        for required in ("pull_request", "push"):
            if required not in keys:
                self.refuse(on_key.number, (
                    f"an `on:` without `{required}:` — §CI says the gate runs for every pull request and "
                    f"every push to `main`, and this pin holds that sentence to the trigger block"))
        pull_request = keys["pull_request"]
        if pull_request.value.strip() or pull_request.end > pull_request.index + 1:
            self.refuse(pull_request.number, (
                "a `pull_request:` with a value or a block — a filter (`paths`, `paths-ignore`, "
                "`branches-ignore`, …) narrows §CI's 'every pull request' to something less, and the pin "
                "reads `pull_request:` bare rather than certifying the filter it cannot evaluate"))
        push = keys["push"]
        if push.value.strip():
            self.refuse(push.number, f"an inline `push:` ({push.value!r}) — the pin reads the block form with its `branches:` list")
        push_entries = self.block_children(push, "the `push:` block")
        if not push_entries:
            self.refuse(push.number, "a `push:` with no `branches:` — §CI says every push to `main`, and the pin reads that list")
        seen_branches = False
        for entry in push_entries:
            if entry.item or entry.key != "branches" or seen_branches:
                self.refuse(entry.number, (
                    f"a `push:` key this pin does not read (`{entry.key}:`) — the pin reads `branches: "
                    f"[main, …]` and nothing else under `push:`"))
            seen_branches = True
            branches = self.inline_tokens(entry, "a `push:` `branches:` list")
            if "main" not in branches:
                self.refuse(entry.number, (
                    f"a `push:` whose `branches:` list does not hold `main` ({branches!r}) — §CI says "
                    f"every push to `main` runs the gate, and this list does not cover it"))

    def read_permissions(self, permissions: Child) -> None:
        """``permissions:`` — read structurally, so its scopes cannot hide another key's shape."""
        if permissions.value.strip():
            self.refuse(permissions.number, (
                f"an inline `permissions:` ({permissions.value!r}) — the pin reads the block form (`permissions:` then its scopes)"))
        entries = self.block_children(permissions, "the `permissions:` block")
        if not entries:
            self.refuse(permissions.number, "a `permissions:` with no scopes — the pin reads the block form, and an empty one is refused")
        for entry in entries:
            if entry.item:
                self.refuse(entry.number, "a list item inside `permissions:` — the scopes are `key: value` lines")
            self.scalar(entry, f"a `permissions:` scope (`{entry.key}`)")

    def read_jobs(self, jobs: Child) -> tuple[tuple[str, ...], tuple[str, ...]]:
        """``jobs:`` — the one job that runs the matrix's legs, whose steps the pin reads."""
        if jobs.value.strip():
            self.refuse(jobs.number, f"an inline `jobs:` ({jobs.value!r}) — the pin reads the block form")
        entries = self.block_children(jobs, "the `jobs:` block")
        if not entries:
            self.refuse(jobs.number, "a `jobs:` block with no jobs")
        if len(entries) > 1:
            self.refuse(entries[1].number, (
                f"a second job (`{entries[1].key}:`) — a second `strategy.matrix` is a shape this pin "
                f"refuses: it reads one job and one matrix per workflow and cannot say which matrix a "
                f"job's `runs-on:` reaches"))
        if entries[0].item:
            self.refuse(entries[0].number, "a list item inside `jobs:` — a job is a `name: key: value` mapping")
        return self.read_job(entries[0])

    def read_job(self, job: Child) -> tuple[tuple[str, ...], tuple[str, ...]]:
        """One job: ``name``, ``runs-on: ${{ matrix.os }}``, ``strategy`` (with the matrix) and ``steps``."""
        if job.value.strip():
            self.refuse(job.number, (
                f"an inline job definition (`{job.key}: {job.value}`) — the pin reads a job's keys as its "
                f"own lines"))
        entries = self.block_children(job, f"the job `{job.key}`")
        keys: dict[str, Child] = {}
        for entry in entries:
            if entry.item:
                self.refuse(entry.number, f"a list item inside the job `{job.key}` this pin cannot place")
            if entry.key in keys:
                self.refuse(entry.number, f"a second `{entry.key}:` in the job `{job.key}`")
            keys[entry.key] = entry
            if entry.key == "uses":
                self.refuse(entry.number, (
                    f"a `uses:` job (`{job.key}: uses: {entry.value}`) — a called or remote workflow is "
                    f"not read here: the pin refuses instead of certifying a run it cannot see"))
            if entry.key == "if":
                self.refuse(entry.number, (
                    f"a job-level `if:` ({entry.value!r}) — §CI's sentence is about every pull request "
                    f"and every push to `main`, and a job-level condition can skip a leg: the pin "
                    f"refuses a shape that narrows the claim it holds"))
            if entry.key == "continue-on-error":
                self.refuse(entry.number, (
                    f"a job-level `continue-on-error:` ({entry.value!r}) — a leg that may fail without "
                    f"failing the run is not a leg that ran the gate"))
            if entry.key in ("os", "include", "exclude"):
                self.refuse(entry.number, (
                    f"an `{entry.key}:` outside the matrix — the pin reads the matrix's own `os:`, "
                    f"`include:` and `exclude:` and refuses one it cannot place"))
            if entry.key not in ("name", "runs-on", "strategy", "steps"):
                self.refuse(entry.number, (
                    f"a job key this pin does not read (`{entry.key}:`) in the job `{job.key}` — the pin "
                    f"reads `name`, `runs-on`, `strategy` and `steps`"))
        for required in ("runs-on", "strategy", "steps"):
            if required not in keys:
                self.refuse(job.number, (
                    f"a job without `{required}:` — the pin reads `runs-on: {MATRIX_REFERENCE}` and the "
                    f"`strategy.matrix` that answers it, and a job missing either runs on no leg this "
                    f"pin can certify"))
        if "name" in keys:
            self.scalar(keys["name"], f"the job `{job.key}`'s `name:`")
        return self.read_strategy(keys["strategy"]), self.read_steps(keys["steps"])

    def read_strategy(self, strategy: Child) -> tuple[str, ...]:
        """``strategy:`` — ``fail-fast`` and the one ``matrix`` whose legs are the claim's."""
        if strategy.value.strip():
            self.refuse(strategy.number, f"an inline `strategy:` ({strategy.value!r}) — the pin reads the block form")
        entries = self.block_children(strategy, "the `strategy:` block")
        keys: dict[str, Child] = {}
        for entry in entries:
            if entry.item:
                self.refuse(entry.number, "a list item inside `strategy:` — the pin reads its `key: value` lines")
            if entry.key in keys:
                self.refuse(entry.number, f"a second `{entry.key}:` in `strategy:`")
            keys[entry.key] = entry
            if entry.key == "fail-fast":
                self.scalar(entry, "`strategy.fail-fast:`")
            elif entry.key != "matrix":
                self.refuse(entry.number, (
                    f"a `strategy:` key this pin does not read (`{entry.key}:`) — the pin reads "
                    f"`fail-fast` and `matrix`"))
        if "matrix" not in keys:
            self.refuse(strategy.number, "a `strategy:` with no `matrix:` — the pin reads the legs from it")
        return self.read_matrix(keys["matrix"])

    def read_matrix(self, matrix: Child) -> tuple[str, ...]:
        """The matrix's legs: ``os:``, ``include:`` and ``exclude:`` combined order-independently.

        YAML mappings are unordered, so the three keys are collected wherever they stand and combined
        semantically — ``(os ∪ include) − exclude``, an exclude winning over an include of the same
        label — instead of applied in the order the lines happen to hold.
        """
        if matrix.value.strip():
            self.refuse(matrix.number, (
                f"an inline `matrix:` ({matrix.value!r}) — the pin reads the block form (`matrix:` then "
                f"its `os:`/`include:`/`exclude:` lines)"))
        entries = self.block_children(matrix, "the `matrix:` block")
        if not entries:
            self.refuse(matrix.number, "a `matrix:` with no `os:` — the pin reads the legs from it and refuses one it cannot enumerate")
        seen: dict[str, Child] = {}
        os_labels: list[str] = []
        includes: list[str] = []
        excludes: list[str] = []
        for entry in entries:
            if entry.item:
                self.refuse(entry.number, (
                    f"a list item inside the matrix outside an `include:`/`exclude:` block — the pin "
                    f"cannot place it and refuses the shape"))
            if entry.key in seen:
                self.refuse(entry.number, (
                    f"a second `{entry.key}:` inside the matrix — the pin reads one `os:`, one "
                    f"`include:` and one `exclude:`"))
            seen[entry.key] = entry
            if entry.key == "os":
                os_labels = self.read_os(entry)
            elif entry.key in ("include", "exclude"):
                labels = self.read_legs(entry)
                (includes if entry.key == "include" else excludes).extend(labels)
            else:
                self.refuse(entry.number, (
                    f"a key inside the matrix this pin does not read (`{entry.key}:`) — the pin reads "
                    f"`os:`, `include:` and `exclude:`"))
        if "os" not in seen:
            self.refuse(matrix.number, "a `matrix:` with no `os:` — the pin reads the legs from it and refuses one it cannot enumerate")
        legs: list[str] = []
        for label in [*os_labels, *includes]:
            if label not in legs:
                legs.append(label)
        for label in excludes:
            if label in legs:
                legs.remove(label)
        if not legs:
            self.refuse(matrix.number, (
                "a matrix whose effective legs are empty — an `exclude:` removed every leg the `os:` "
                "list and the `include:` entries name"))
        return tuple(legs)

    def read_os(self, os_key: Child) -> list[str]:
        """An ``os: [label, …]`` list's legs, deduplicated in order; refuses any other form."""
        if os_key.end > os_key.index + 1 or not os_key.value.strip():
            self.refuse(os_key.number, (
                f"an `os:` this pin cannot read ({os_key.value.strip()!r}) — the pin reads the inline "
                f"list form (`os: [label, …]`) and refuses a block label list"))
        labels = [self.label_token(os_key, token, "a runner label") for token in self.inline_tokens(os_key, "an `os:` label list")]
        if not labels:
            self.refuse(os_key.number, (
                f"an `os:` this pin cannot read ({os_key.value.strip()!r}) — the list holds no label"))
        return labels

    def read_legs(self, block: Child) -> list[str]:
        """An ``include:``/``exclude:`` block's ``- os: <label>`` entries; refuses any other entry."""
        if block.value.strip():
            self.refuse(block.number, (
                f"an inline `{block.key}:` ({block.value!r}) — the pin reads the block form (`{block.key}:` "
                f"then its `- os: <label>` entries)"))
        entries = self.block_children(block, f"the `{block.key}:` block")
        if not entries:
            self.refuse(block.number, f"an `{block.key}:` block with no entries — the pin reads `- os: <label>` entries")
        labels = []
        for entry in entries:
            if not (entry.item and entry.key == "os" and entry.value.strip()):
                self.refuse(entry.number, (
                    f"an `{block.key}:` entry this pin cannot read ({entry.key}: {entry.value}) — the pin "
                    f"reads `- os: <label>` entries"))
            if entry.end > entry.index + 1:
                self.refuse(entry.number, (
                    f"an `{block.key}:` entry with a nested value — the pin reads `- os: <label>` and "
                    f"nothing else in the entry"))
            labels.append(self.label_token(entry, entry.value.strip(), f"an `{block.key}:` label"))
        return labels

    def read_steps(self, steps: Child) -> tuple[str, ...]:
        """``steps:`` — every step's whole ``run:`` value, in order; each step read structurally."""
        if steps.value.strip():
            self.refuse(steps.number, f"an inline `steps:` ({steps.value!r}) — the pin reads the block form (`steps:` then its `- uses:`/`- run:` items)")
        entries = self.block_children(steps, "the `steps:` block")
        if not entries:
            self.refuse(steps.number, "a `steps:` block with no steps")
        runs: list[str] = []
        for entry in entries:
            if not entry.item:
                self.refuse(entry.number, (
                    f"a `steps:` entry this pin cannot read (`{entry.key}: {entry.value}`) — the pin "
                    f"reads list items (`- uses: …`, `- run: …`)"))
            runs.extend(self.read_step(entry))
        return tuple(runs)

    def read_step(self, item: Child) -> tuple[str, ...]:
        """One step: ``name``, and exactly one of ``uses`` (with its ``with`` inputs) and ``run``.

        A ``run`` step's value is kept whole — the command check compares it as a whole — and a step
        that could be skipped (``if:``) or ignored (``continue-on-error:``) is refused: §CI claims a run
        of the gate, and a step that may not run, or may fail without failing the run, is not one.
        """
        entries = self.item_entries(item, "a step")
        keys: dict[str, Child] = {}
        runs: list[str] = []
        for entry in entries:
            if entry.key in keys:
                self.refuse(entry.number, f"a second `{entry.key}:` in one step")
            keys[entry.key] = entry
            if entry.key in ("if", "continue-on-error"):
                self.refuse(entry.number, (
                    f"a step-level `{entry.key}:` ({entry.value!r}) — a step that can be skipped or "
                    f"ignored is not a step that runs the gate: the pin refuses the shape"))
            if entry.key == "uses":
                target = self.scalar(entry, "a step's `uses:`")
                if ".github/workflows/" in target:
                    self.refuse(entry.number, (
                        f"a step whose `uses:` names a workflow file ({target}) — the pin reads steps "
                        f"whose run it can see and refuses one that hands it to another file"))
            elif entry.key == "run":
                runs.append(self.scalar(entry, "a step's `run:`"))
            elif entry.key == "name":
                self.scalar(entry, "a step's `name:`")
            elif entry.key == "with":
                if entry.value.strip():
                    self.refuse(entry.number, f"an inline `with:` ({entry.value!r}) — the pin reads the block form")
                inputs = self.block_children(entry, "a step's `with:` block")
                if not inputs:
                    self.refuse(entry.number, "a `with:` with no inputs")
                for input_ in inputs:
                    if input_.item:
                        self.refuse(input_.number, "a list item inside `with:` — an input is a `key: value` line")
                    self.scalar(input_, f"a `with:` input (`{input_.key}`)")
            else:
                self.refuse(entry.number, (
                    f"a step key this pin does not read (`{entry.key}:`) — the pin reads `name`, `uses`, "
                    f"`run` and `with`"))
        has_uses, has_run = "uses" in keys, "run" in keys
        if has_uses == has_run:
            self.refuse(item.number, (
                f"a step that is {'both a `uses:` step and a `run:` step' if has_uses else 'neither a `uses:` nor a `run:` step'} "
                f"— a step is one or the other, and the pin refuses a shape whose command it cannot see"))
        return tuple(runs)


def read_workflow(path: Path, text: str) -> Workflow:
    """Read one workflow file: its matrix's effective legs and its steps' whole ``run:`` values."""
    return _WorkflowReader(path, structural_lines(text)).read()


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


def workflow_text(*, triggers: str | None = None, matrix: str | None = None,
                  steps: str | None = None) -> str:
    """A minimal workflow holding the shapes a case drives the reader with."""
    return (
        "name: gate\n"
        "on:\n"
        + ("  pull_request:\n  push:\n    branches: [main]\n" if triggers is None else triggers)
        + "permissions:\n"
        "  contents: read\n"
        "jobs:\n"
        "  gate:\n"
        "    name: gate (${{ matrix.os }})\n"
        "    runs-on: ${{ matrix.os }}\n"
        "    strategy:\n"
        "      fail-fast: false\n"
        "      matrix:\n"
        + ("        os: [ubuntu-latest, windows-latest]\n" if matrix is None else matrix)
        + "    steps:\n"
        + ("      - name: Gate\n        run: python scripts/gate.py\n" if steps is None else steps)
    )


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
            list(gate.legs), list(RUNNERS),
            "the matrix's legs are the platforms docs/harness.md §CI claims: the `os:` list and every "
            "`include:`/`exclude:` entry are read and combined order-independently (`os ∪ include − "
            "exclude`), so the two files move in the same increment (owner decision 2026-11-18 removed "
            "the macOS leg), or this check and the docs/plans/harness-ci-matrix.md decision change "
            "together")
        for path, workflow in sorted(self.workflows.items()):
            if path == WORKFLOW:
                continue
            self.assertEqual(
                sorted(set(workflow.legs) - set(RUNNERS)), [],
                f"{path.relative_to(ROOT)} runs a matrix leg outside the runners the claim names "
                f"({RUNNERS!r}): every workflow file is read, not only the gate's")

    def test_the_gate_job_runs_the_gate_file_itself(self) -> None:
        """§CI claims CI runs the gate file: a step of the gate job must be that command, whole."""
        runs = self.workflows[WORKFLOW].runs
        self.assertIn(
            GATE_COMMAND, runs,
            f"the gate job has no step whose whole `run:` value is {GATE_COMMAND!r} (steps' run values: "
            f"{list(runs)!r}): §CI claims CI runs that file, and a commented-out command, a spelling the "
            f"Windows runner cannot run, a block scalar or a longer line that merely contains the text "
            f"is not a step that runs it")

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
        """The platform rules' paragraph is the claim's other live copy, and it moves with the matrix.

        A runner outside the matrix may stay in that paragraph only in a sentence carrying a history
        marker — the leg's own red at a past head, the repair it forced: a present-tense mention is a
        claim about CI the matrix does not support, and presence alone must not pass.
        """
        paragraph = platform_rules_paragraph(self.harness)
        named = set(_RUNNER_LABEL.findall(paragraph))
        self.assertTrue(
            named - set(RUNNERS) <= {RETIRED_RUNNER},
            "docs/harness.md §Platform boundary's paragraph names a runner the matrix does not hold "
            f"(only the retired `{RETIRED_RUNNER}`, kept by a history sentence, may remain): a matrix "
            f"change cannot leave the platform rules stating a state CI no longer has")
        self.assertTrue(
            set(RUNNERS) <= named,
            "docs/harness.md §Platform boundary's paragraph must name every runner the matrix holds "
            "(the same list §CI names), so the claim's two live copies state one thing")
        for sentence in re.split(r"(?<=[.;])\s+", paragraph):
            foreign = set(_RUNNER_LABEL.findall(sentence)) - set(RUNNERS)
            if not foreign:
                continue
            self.assertTrue(
                _HISTORY_MARKER.search(sentence),
                f"docs/harness.md §Platform boundary names {sorted(foreign)} in a sentence with no "
                f"history marker ({sentence!r}): only a mention recording what a past run did (its red "
                f"at a past head, `while the leg ran`) may stay; a present-tense mention is a claim CI "
                f"does not verify")

    def test_the_reader_reads_the_shapes_it_accepts(self) -> None:
        """The accepted shapes are exercised here, not only the refused ones: a case that only drives
        refusals would pass with its accepting branch disabled."""
        accepted = {
            "an `include:`/`exclude:` block written before `os:`": (
                workflow_text(matrix=(
                    "        include:\n          - os: macos-latest\n"
                    "        exclude:\n          - os: ubuntu-latest\n"
                    "        os: [ubuntu-latest, windows-latest]\n")),
                ("windows-latest", "macos-latest")),
            "the same blocks written after `os:`": (
                workflow_text(matrix=(
                    "        os: [ubuntu-latest, windows-latest]\n"
                    "        include:\n          - os: macos-latest\n"
                    "        exclude:\n          - os: ubuntu-latest\n")),
                ("windows-latest", "macos-latest")),
            "a quoted label list": (
                workflow_text(matrix="        os: ['ubuntu-latest', \"windows-latest\"]\n"),
                RUNNERS),
            "a `push:` trigger whose `branches:` list holds `main` beside another branch": (
                workflow_text(triggers="  pull_request:\n  push:\n    branches: [release/x, main]\n"),
                RUNNERS),
            "a `uses:` step with its `with:` block, beside the gate step": (
                workflow_text(steps=(
                    "      - uses: actions/setup-go@v5\n"
                    "        with:\n"
                    "          go-version-file: go.mod\n"
                    "      - name: Gate\n"
                    "        run: python scripts/gate.py\n")),
                RUNNERS),
        }
        for shape, (text, legs) in accepted.items():
            with self.subTest(shape=shape):
                workflow = read_workflow(WORKFLOW, text)
                self.assertEqual(list(workflow.legs), list(legs),
                                 f"{shape}: legs read as {list(workflow.legs)!r}, want {list(legs)!r}")
                self.assertEqual(list(workflow.runs), [GATE_COMMAND],
                                 f"{shape}: the step's whole `run:` value must be read as the command")

    def test_the_reader_refuses_a_shape_it_cannot_certify(self) -> None:
        """A shape the pin cannot read is refused, never read past: the reader's own contract.

        One entry per refusing branch of the walk — the shapes the docstrings name among them — so a
        mutation that disables a branch leaves a named case red here instead of passing quietly.
        """
        shapes = {
            "a second job (and with it a second `strategy.matrix`)": (
                workflow_text() + "  other:\n    runs-on: ${{ matrix.os }}\n",
                "a second job"),
            "a top-level key this pin does not read": (
                workflow_text() + "env:\n  FOO: bar\n",
                "a top-level key this pin does not read (`env:`)"),
            "a list item at the top level": (
                "- name: stray\n" + workflow_text(),
                "a list item at the top level"),
            "a second top-level key": (
                workflow_text() + "on: push\n",
                "a second `on:` at the top level"),
            "a workflow without `jobs:`": (
                "name: gate\non:\n  pull_request:\n  push:\n    branches: [main]\n",
                "a workflow without `jobs:`"),
            "an inline `on:`": (
                workflow_text().replace("on:\n  pull_request:\n  push:\n    branches: [main]\n",
                                        "on: [pull_request, push]\n"),
                "an inline `on:`"),
            "an `on:` with no trigger": (
                workflow_text(triggers=""),
                "an `on:` with no trigger"),
            "an `on:` without `push:`": (
                workflow_text(triggers="  pull_request:\n"),
                "an `on:` without `push:`"),
            "a trigger this pin does not read": (
                workflow_text(triggers="  pull_request:\n  push:\n    branches: [main]\n  schedule:\n    - cron: '0 0 * * *'\n"),
                "a trigger this pin does not read (`schedule:`)"),
            "a `pull_request:` with a block (a filter narrows 'every pull request')": (
                workflow_text(triggers="  pull_request:\n    paths-ignore: ['docs/**']\n  push:\n    branches: [main]\n"),
                "a `pull_request:` with a value or a block"),
            "a `pull_request:` with an inline value": (
                workflow_text(triggers="  pull_request: {paths-ignore: ['docs/**']}\n  push:\n    branches: [main]\n"),
                "a `pull_request:` with a value or a block"),
            "a `push:` with no `branches:`": (
                workflow_text(triggers="  pull_request:\n  push:\n"),
                "a `push:` with no `branches:`"),
            "a `push:` whose `branches:` list does not hold `main`": (
                workflow_text(triggers="  pull_request:\n  push:\n    branches: [dev]\n"),
                "does not hold `main`"),
            "a `push:` key this pin does not read": (
                workflow_text(triggers="  pull_request:\n  push:\n    branches: [main]\n    tags: ['v*']\n"),
                "a `push:` key this pin does not read"),
            "an inline `permissions:`": (
                workflow_text().replace("permissions:\n  contents: read\n", "permissions: read-all\n"),
                "an inline `permissions:`"),
            "a job key this pin does not read": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n",
                                        "    runs-on: ${{ matrix.os }}\n    env:\n      FOO: bar\n"),
                "a job key this pin does not read (`env:`)"),
            "a job-level `if:`": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n",
                                        "    runs-on: ${{ matrix.os }}\n    if: github.event_name == 'push'\n"),
                "a job-level `if:`"),
            "a job-level `continue-on-error:`": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n",
                                        "    runs-on: ${{ matrix.os }}\n    continue-on-error: true\n"),
                "a job-level `continue-on-error:`"),
            "a `uses:` job": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n",
                                        "    uses: ./.github/workflows/gate.yml\n"),
                "a `uses:` job"),
            "a job without `runs-on:`": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n", ""),
                "a job without `runs-on:`"),
            "a job without `strategy:`": (
                workflow_text().replace(
                    "    strategy:\n      fail-fast: false\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n", ""),
                "a job without `strategy:`"),
            "a job without `steps:`": (
                workflow_text().replace("    steps:\n      - name: Gate\n        run: python scripts/gate.py\n", ""),
                "a job without `steps:`"),
            "an inline job definition": (
                workflow_text().replace(
                    "  gate:\n    name: gate (${{ matrix.os }})\n    runs-on: ${{ matrix.os }}\n",
                    "  gate: {runs-on: ${{ matrix.os }}}\n"),
                "an inline job definition"),
            "a `runs-on:` that is not exactly the matrix reference": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n", "    runs-on: macos-latest\n"),
                "a `runs-on:` that is not exactly"),
            "a `runs-on:` with a block value": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n", "    runs-on:\n      - macos-latest\n"),
                "a `runs-on:` that is not exactly"),
            "a quoted `runs-on` key": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n", "    'runs-on': ${{ matrix.os }}\n"),
                "a `runs-on` spelled in a form this pin cannot read"),
            "a space before a key's colon": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n", "    runs-on : ${{ matrix.os }}\n"),
                "a `runs-on` spelled in a form this pin cannot read"),
            "an inline `strategy:`": (
                workflow_text().replace(
                    "    strategy:\n      fail-fast: false\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n",
                    "    strategy: {matrix: {os: [ubuntu-latest]}}\n"),
                "an inline `strategy:`"),
            "a `strategy:` key this pin does not read": (
                workflow_text().replace("      fail-fast: false\n", "      fail-fast: false\n      max-parallel: 2\n"),
                "a `strategy:` key this pin does not read (`max-parallel:`)"),
            "a `strategy:` with no `matrix:`": (
                workflow_text().replace("      matrix:\n        os: [ubuntu-latest, windows-latest]\n", ""),
                "a `strategy:` with no `matrix:`"),
            "an inline `matrix:`": (
                workflow_text().replace("      matrix:\n        os: [ubuntu-latest, windows-latest]\n",
                                        "      matrix: {os: [ubuntu-latest, windows-latest]}\n"),
                "an inline `matrix:`"),
            "a matrix with no `os:`": (
                workflow_text(matrix="        include:\n          - os: ubuntu-latest\n"),
                "a `matrix:` with no `os:`"),
            "a key inside the matrix this pin does not read": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        python: ['3.12']\n"),
                "a key inside the matrix this pin does not read (`python:`)"),
            "a list item inside the matrix outside `include:`/`exclude:`": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        - os: macos-latest\n"),
                "a list item inside the matrix outside an `include:`/`exclude:` block"),
            "a second `os:` inside the matrix": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        os: [windows-latest]\n"),
                "a second `os:` inside the matrix"),
            "an `os:` outside the matrix": (
                workflow_text().replace("    runs-on: ${{ matrix.os }}\n",
                                        "    runs-on: ${{ matrix.os }}\n    os: [macos-latest]\n"),
                "an `os:` outside the matrix"),
            "an `os:` this pin cannot read (a block label list)": (
                workflow_text(matrix="        os:\n          - ubuntu-latest\n"),
                "an `os:` this pin cannot read"),
            "an `os:` list holding no label": (
                workflow_text(matrix="        os: []\n"),
                "the list holds no label"),
            "a quoted scalar holding a comma read as one label": (
                workflow_text(matrix="        os: ['ubuntu-latest, windows-latest']\n"),
                "this pin cannot read ('ubuntu-latest, windows-latest')"),
            "a label spelling outside a label's own characters": (
                workflow_text(matrix="        os: [ubuntu latest]\n"),
                "this pin cannot read ('ubuntu latest')"),
            "an inline `include:`": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        include: [{os: macos-latest}]\n"),
                "an inline `include:`"),
            "an `include:` block with no entries": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        include:\n"),
                "an `include:` block with no entries"),
            "an `include:` entry that is not `- os:`": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        include:\n          - python: '3.12'\n"),
                "an `include:` entry this pin cannot read"),
            "an `include:` entry with a nested value": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        include:\n          - os: macos-latest\n            python: '3.12'\n"),
                "an `include:` entry with a nested value"),
            "a matrix whose effective legs are empty": (
                workflow_text(matrix="        os: [ubuntu-latest]\n        exclude:\n          - os: ubuntu-latest\n"),
                "a matrix whose effective legs are empty"),
            "an inline `steps:`": (
                workflow_text().replace("    steps:\n      - name: Gate\n        run: python scripts/gate.py\n",
                                        "    steps: [a]\n"),
                "an inline `steps:`"),
            "a `steps:` entry that is not a list item": (
                workflow_text(steps="      name: Gate\n"),
                "a `steps:` entry this pin cannot read"),
            "a step key this pin does not read": (
                workflow_text().replace("        run: python scripts/gate.py\n",
                                        "        run: python scripts/gate.py\n        shell: bash\n"),
                "a step key this pin does not read (`shell:`)"),
            "a step-level `if:`": (
                workflow_text().replace("        run: python scripts/gate.py\n",
                                        "        if: runner.os != 'Windows'\n        run: python scripts/gate.py\n"),
                "a step-level `if:`"),
            "a step-level `continue-on-error:`": (
                workflow_text().replace("        run: python scripts/gate.py\n",
                                        "        continue-on-error: true\n        run: python scripts/gate.py\n"),
                "a step-level `continue-on-error:`"),
            "a block scalar `run:`": (
                workflow_text(steps="      - name: Gate\n        run: >-\n          python scripts/gate.py\n"),
                "a `run:` with a block scalar"),
            "a step with both `uses:` and `run:`": (
                workflow_text(steps="      - uses: actions/checkout@v4\n        run: python scripts/gate.py\n"),
                "both a `uses:` step and a `run:` step"),
            "a step with neither `uses:` nor `run:`": (
                workflow_text(steps="      - name: Gate\n"),
                "neither a `uses:` nor a `run:` step"),
            "a step whose `uses:` names a workflow file": (
                workflow_text(steps="      - uses: ./.github/workflows/gate.yml\n"),
                "a step whose `uses:` names a workflow file"),
            "an inline `with:`": (
                workflow_text(steps="      - uses: actions/setup-go@v5\n        with: {go-version-file: go.mod}\n"),
                "an inline `with:`"),
            "a `with:` with no inputs": (
                workflow_text(steps="      - uses: actions/setup-go@v5\n        with:\n"),
                "a `with:` with no inputs"),
            "a `with:` input with a block value": (
                workflow_text(steps="      - uses: actions/setup-go@v5\n        with:\n          go-version-file:\n            file: go.mod\n"),
                "with a block value"),
        }
        for shape, (text, reason) in shapes.items():
            with self.subTest(shape=shape):
                with self.assertRaises(UnreadableWorkflow) as refused:
                    read_workflow(WORKFLOW, text)
                self.assertIn(reason, str(refused.exception),
                              f"{shape}: refusal was {str(refused.exception)!r}")


if __name__ == "__main__":
    unittest.main()
