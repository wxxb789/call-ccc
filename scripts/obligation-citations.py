#!/usr/bin/env python3
"""call/ccc obligation citations — the acceptance surface, machine-checked.

A goal's obligations live in prose (``docs/evidence/<goal>-obligation-matrix.md``), and prose audited by
a reader does not converge: a claim stronger than the evidence it cites lands inside the previous
repair's own wording. So every obligation a gate-runnable Go test discharges also gets a row in
``docs/evidence/<goal>-citations.tsv`` — the rule id, the test that pins it, the package ``go test`` is
handed, and the matrix section the pair comes from — and this script is the arbiter: it parses every
discovered manifest and runs the tests the rows cite.

Rules (``docs/conventions.md`` states the schema, ``docs/harness.md`` the gate step):

- discovery: ``docs/evidence/*-citations.tsv``, sorted by file name; the order is pinned and printed;
- parse, per file: the first non-comment line is the header and names exactly ``rule_id``, ``test``,
  ``package``, ``artifact`` in that order; every data row holds exactly four non-empty cells; no
  ``rule_id`` repeats within a file. A violation fails the run, named by file and line;
- verify, per row: ``package`` is one package ``scripts/gate-targets.txt`` covers, ``test`` is a legal
  Go test name that a ``_test.go`` under that package declares, and the test must **run and pass** —
  one ``go test -count=1 -v -run '^(…|…)$' <package>`` per package (the same ``scripts/gate-goflags.txt``
  flags the gate's own steps use), whose output must hold ``--- PASS: <test>`` for every cited test. A
  cited test that is absent, that fails or that the run reports ``--- SKIP`` is a failure naming the row;
- print: one line per file (``<relative path>: <n> row(s) verified``), the summary
  (``citations: <rows> row(s) across <files> file(s) verified``) and the verdict line
  (``citations: PASS``, ``citations: FAIL — …``, ``citations: SKIP — …``, ``citations: BOUNDARY — …``),
  which is the last line. On a failure the violation lines are printed above it.

Exit codes: ``0`` PASS, ``1`` FAIL, ``2`` SKIP, ``3`` BOUNDARY.

``SKIP`` means no manifest was found anywhere: the manifest *is* an increment's acceptance surface, so
the ``citations`` step is required (``scripts/gate-required.txt``) and the gate rewrites that SKIP to a
failure — a tree that lost its acceptance surface must not read as passing. ``BOUNDARY`` is the gate's
own platform boundary, read from ``scripts/gate.py``'s ``go_env()`` rather than restated here: on
Windows without a C compiler the Go steps cannot run, and neither can this one.

The static half — the identifier, the package and the ``_test.go`` declaration — runs before any tool:
a violation there is answered without invoking ``go`` at all.
"""

from __future__ import annotations

import importlib.util
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple

ROOT = Path(__file__).resolve().parent.parent
PASS, FAIL, SKIP, BOUNDARY = "PASS", "FAIL", "SKIP", "BOUNDARY"
EXIT = {PASS: 0, FAIL: 1, SKIP: 2, BOUNDARY: 3}

#: The four columns, in the order the header must name them.
COLUMNS = ("rule_id", "test", "package", "artifact")

#: Where the manifests live, and what one is called.
MANIFEST_DIR = Path("docs") / "evidence"
MANIFEST_GLOB = "*-citations.tsv"

#: How many characters of cited-test names one ``go test -run`` may carry: the bound
#: ``scripts/gate.py``'s ``gofmt_batches`` uses for its file list, for the same reason (Windows refuses
#: a process command line past 32 767 characters). A package's cited tests are one invocation unless
#: the alternation alone would outgrow this.
BATCH_CHARS = 8000

#: One Go command's deadline: the gate's own default, so a stalled run is a result rather than a hang.
GO_TIMEOUT = 1800

#: At most this many violation lines are printed (the gofmt step's own bound); the verdict line counts
#: them all.
VIOLATIONS_PRINTED = 40

#: A Go identifier, and a name ``go test`` runs at top level: ``Test`` then a character that is not a
#: lowercase letter (``testing/match.go``'s own ``isTest``).
GO_IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")

#: The package column: one package in the tree's own ``./`` spelling (``./internal/ccc``), which is what
#: ``go test`` is handed — not a pattern, not a module path.
PACKAGE_SHAPE = re.compile(r"^\./[A-Za-z0-9_.][A-Za-z0-9_./-]*$")

#: The verdict line's own spelling (the gate reads its status from this vocabulary, in these words).
VERDICT = re.compile(r"^citations: (PASS|FAIL|SKIP|BOUNDARY|N/A)(?: — .*)?$")


class Citation(NamedTuple):
    """One manifest row: the rule it cites, the test that pins it, the package, and the prose."""

    line: int
    rule_id: str
    test: str
    package: str
    artifact: str


def run(cmd: list[str], env: dict[str, str] | None = None, timeout: int = GO_TIMEOUT):
    """Run one command in the tree under test, answering the shapes the gate's own ``run()`` does.

    A tool that could not be started is a result, not a crash (``rc=127`` for a missing binary, ``126``
    for any other launch failure, the command named in the detail because Windows' own message does not
    hold it), and one that outran its deadline is ``rc=124``.
    """
    merged = os.environ.copy()
    merged.update(env or {})
    try:
        return subprocess.run(
            cmd, cwd=str(ROOT), env=merged, capture_output=True, text=True, timeout=timeout
        )
    except OSError as exc:
        return subprocess.CompletedProcess(
            cmd, 127 if isinstance(exc, FileNotFoundError) else 126, "", f"{cmd[0]}: {exc}\n"
        )
    except subprocess.TimeoutExpired as expired:
        return subprocess.CompletedProcess(
            cmd,
            124,
            expired.stdout if isinstance(expired.stdout, str) else "",
            f"{cmd[0]}: no answer within {timeout} s\n",
        )


def read_list(path: Path) -> list[str]:
    """One entry per line, ``#`` comments and blanks dropped (the shape ``gate.py`` reads knobs with)."""
    if not path.exists():
        return []
    items: list[str] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.split("#", 1)[0].strip()
        if line:
            items.append(line)
    return items


def go_flags() -> list[str]:
    """``scripts/gate-goflags.txt`` folded into one flag per token (the gate's own ``go_flags()`` rule).

    The knob file holds one flag per line (``-tags fts5``), and the two tokens are two arguments to
    ``go test``: handing the line over whole would ask the tool for a flag named ``-tags fts5``.
    """
    return [token for line in read_list(ROOT / "scripts" / "gate-goflags.txt") for token in line.split()]


def load_gate():
    """Import ``scripts/gate.py`` — the sibling that runs this checker.

    The host boundary and the Go environment come from that module's own ``go_env()`` rather than from a
    copy here, so the two cannot drift apart: a Windows host without a C compiler is the same disclosed
    boundary for this step as for the Go steps it excuses.
    """
    path = Path(__file__).resolve().parent / "gate.py"
    spec = importlib.util.spec_from_file_location("ccc_citation_gate", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def manifests() -> list[Path]:
    """Every ``*-citations.tsv`` under ``docs/evidence/``, in the pinned order (by file name)."""
    directory = ROOT / MANIFEST_DIR
    if not directory.is_dir():
        return []
    return sorted((p for p in directory.glob(MANIFEST_GLOB) if p.is_file()), key=lambda p: p.name)


def relative(path: Path) -> str:
    """The printed path's spelling: POSIX-form, relative to the tree (one spelling on every platform)."""
    return path.relative_to(ROOT).as_posix()


def parse_manifest(path: Path) -> tuple[list[Citation], list[str]]:
    """One manifest's rows, and the violations that keep it from being one.

    A violation names the file and the line, because a row is only a citation where a reader can follow
    the line number back to the text that failed.
    """
    name = relative(path)
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as exc:
        return [], [f"{name}: could not be read: {exc}"]
    rows: list[Citation] = []
    violations: list[str] = []
    header: list[str] | None = None
    cited: dict[str, int] = {}
    for number, line in enumerate(text.splitlines(), start=1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        cells = [cell.strip() for cell in line.split("\t")]
        if header is None:
            header = cells
            if cells != list(COLUMNS):
                violations.append(
                    f"{name}:{number}: the header must name exactly {', '.join(COLUMNS)} in that order, "
                    f"not {line.strip()!r}"
                )
            continue
        if len(cells) != len(COLUMNS) or any(not cell for cell in cells):
            violations.append(
                f"{name}:{number}: a row must hold exactly four non-empty cells ({len(cells)} found here)"
            )
            continue
        rule_id, test, package, artifact = cells
        if rule_id in cited:
            violations.append(
                f"{name}:{number}: rule id {rule_id!r} is already cited at line {cited[rule_id]}"
            )
            continue
        cited[rule_id] = number
        rows.append(Citation(number, rule_id, test, package, artifact))
    if header is None:
        violations.append(
            f"{name}: no header line (the first non-comment line must name {', '.join(COLUMNS)})"
        )
    return rows, violations


def legal_test_name(name: str) -> bool:
    """Whether ``go test`` would run a top-level function of that name (``testing``'s own rule)."""
    if not GO_IDENTIFIER.match(name) or not name.startswith("Test"):
        return False
    rest = name[len("Test"):]
    return not rest or not rest[0].islower()


def covered_by(package: str, patterns: list[str]) -> bool:
    """Whether ``scripts/gate-targets.txt`` hands ``go test`` this package.

    Go's own pattern rule for the spellings the knob file uses: ``./x/...`` covers ``./x`` and everything
    under it (which is also how ``./...`` covers the tree), a pattern without ``...`` is that package.
    """
    for pattern in patterns:
        if pattern.endswith("/..."):
            prefix = pattern[:-4]
            if package == prefix or package.startswith(prefix + "/"):
                return True
        elif pattern == package:
            return True
    return False


def package_problem(package: str, patterns: list[str]) -> str | None:
    """Why this package column cannot be cited (one package, covered by the gate's own targets), or None."""
    if not PACKAGE_SHAPE.match(package) or "..." in package or "//" in package or ".." in package.split("/"):
        return (
            f"the package column must name one package in the tree's own `./` spelling "
            f"(e.g. ./internal/ccc), not {package!r}"
        )
    if not covered_by(package, patterns):
        return (
            f"package {package} is not covered by scripts/gate-targets.txt "
            f"({', '.join(patterns) or './...'})"
        )
    return None


def declared_in(package: str, name: str) -> bool:
    """Whether a ``_test.go`` under that package declares ``func <name>(``.

    The package's own directory, never a subdirectory: ``go test`` on one package compiles that
    directory's files only, so a declaration somewhere below is a different package's test.
    """
    directory = ROOT / package[len("./"):]
    if not directory.is_dir():
        return False
    needle = f"func {name}("
    for path in sorted(directory.glob("*_test.go")):
        try:
            text = path.read_text(encoding="utf-8")
        except OSError:
            continue
        for line in text.splitlines():
            if line.startswith(needle):
                return True
    return False


def static_problems(files: list[tuple[str, list[Citation]]], patterns: list[str]) -> list[str]:
    """The violations that are answerable without running anything.

    A name ``go test`` would not run, a package the gate does not test, a test no ``_test.go`` under the
    package declares: each is a citation that cannot point at anything, and each is answered before any
    tool is invoked.
    """
    problems: list[str] = []
    for name, rows in files:
        for row in rows:
            if not legal_test_name(row.test):
                problems.append(
                    f"{name}:{row.line}: rule {row.rule_id}: {row.test!r} is not a legal Go test name "
                    f"(a top-level test function is `TestXxx`, Xxx not a lowercase letter)"
                )
                continue
            problem = package_problem(row.package, patterns)
            if problem:
                problems.append(f"{name}:{row.line}: rule {row.rule_id}: {problem}")
                continue
            if not declared_in(row.package, row.test):
                problems.append(
                    f"{name}:{row.line}: rule {row.rule_id}: test {row.test} is not declared by any "
                    f"_test.go in {row.package}"
                )
    return problems


def batches(names: list[str], limit: int = BATCH_CHARS) -> list[list[str]]:
    """Split cited test names into ``go test -run`` invocations that fit a process command line.

    The rule ``gate.py``'s ``gofmt_batches`` applies to its file list: the batches together are the whole
    list, in the same order, and a name longer than the bound is a batch of its own rather than a
    dropped one.
    """
    groups: list[list[str]] = []
    group: list[str] = []
    used = 0
    for name in names:
        cost = len(name) + 1  # the separator the alternation spends between names
        if group and used + cost > limit:
            groups.append(group)
            group, used = [], 0
        group.append(name)
        used += cost
    if group:
        groups.append(group)
    return groups


def go_test_command(package: str, names: list[str], flags: list[str]) -> list[str]:
    """One ``go test`` invocation for one package's cited tests (never one invocation per row)."""
    return ["go", "test", "-count=1", "-v", *flags, "-run", "^(" + "|".join(names) + ")$", package]


def outcomes(output: str) -> tuple[set[str], dict[str, str]]:
    """What a ``go test -v`` run reported: the tests that passed, and the others by their own status."""
    passed: set[str] = set()
    others: dict[str, str] = {}
    for line in output.splitlines():
        for marker, status in (("--- PASS: ", "PASS"), ("--- FAIL: ", "FAIL"), ("--- SKIP: ", "SKIP")):
            if line.startswith(marker):
                name = line[len(marker):].split(" (", 1)[0].strip()
                if status == PASS:
                    passed.add(name)
                else:
                    others.setdefault(name, status)
    return passed, others


def report(
    files: list[tuple[str, list[Citation]]],
    verified: dict[str, int],
    violations: list[str],
    status: str,
    reason: str = "",
) -> int:
    """Print the run and answer the exit code.

    One line per file, the summary, the violations (bounded), and the verdict — which is the last line,
    so the gate reads the status from the verdict's own vocabulary.
    """
    for name, _rows in files:
        print(f"{name}: {verified.get(name, 0)} row(s) verified")
    for violation in violations[:VIOLATIONS_PRINTED]:
        print(f"  {violation}")
    if len(violations) > VIOLATIONS_PRINTED:
        print(f"  … and {len(violations) - VIOLATIONS_PRINTED} more")
    print(f"citations: {sum(verified.values())} row(s) across {len(files)} file(s) verified")
    print(f"citations: {status}" + (f" — {reason}" if reason else ""))
    return EXIT[status]


def main() -> int:
    found = manifests()
    if not found:
        print(f"citations: {SKIP} — no {MANIFEST_GLOB} under {MANIFEST_DIR.as_posix()}/")
        return EXIT[SKIP]

    files: list[tuple[str, list[Citation]]] = []
    violations: list[str] = []
    for path in found:
        rows, problems = parse_manifest(path)
        files.append((relative(path), rows))
        violations.extend(problems)
    verified = {name: 0 for name, _rows in files}
    patterns = read_list(ROOT / "scripts" / "gate-targets.txt") or ["./..."]
    if not violations:
        violations.extend(static_problems(files, patterns))
    if violations:
        return report(files, verified, violations, FAIL, f"{len(violations)} violation(s)")

    try:
        env, boundary = load_gate().go_env()
    except Exception as exc:  # the boundary's own home could not be read: that is the answer
        return report(
            files,
            verified,
            [f"scripts/gate.py, whose `go_env()` this checker reads its host boundary from, could not be "
             f"loaded: {exc!r}"],
            FAIL,
            "1 violation(s)",
        )
    if boundary:
        return report(files, verified, [], BOUNDARY, boundary)
    if not shutil.which("go"):
        return report(files, verified, [], FAIL, "go not found on PATH")

    flags = go_flags()
    entries = [(name, row) for name, rows in files for row in rows]
    order: list[str] = []
    for _name, row in entries:
        if row.package not in order:
            order.append(row.package)
    for package in order:
        names: list[str] = []
        for _name, row in entries:
            if row.package == package and row.test not in names:
                names.append(row.test)
        unverified: dict[str, str] = {}
        for group in batches(names):
            proc = run(go_test_command(package, group, flags), env=env)
            passed, others = outcomes((proc.stdout or "") + (proc.stderr or ""))
            for name in group:
                if name in passed:
                    unverified.pop(name, None)
                elif name in others:
                    unverified[name] = f"go test reported `--- {others[name]}: {name}`"
                elif proc.returncode != 0:
                    unverified[name] = f"the `go test` run for it exited rc={proc.returncode}"
                else:
                    unverified[name] = "go test printed no `--- PASS:` line for it"
        for name, row in entries:
            if row.package != package:
                continue
            if row.test in unverified:
                violations.append(
                    f"{name}:{row.line}: rule {row.rule_id}: test {row.test} in {package} did not pass — "
                    f"{unverified[row.test]}"
                )
            else:
                verified[name] += 1
    if violations:
        return report(files, verified, violations, FAIL, f"{len(violations)} violation(s)")
    return report(files, verified, [], PASS)


if __name__ == "__main__":
    sys.exit(main())
