#!/usr/bin/env python3
"""The suite's host-contract lint, and the host contract's own probe answers.

``scripts/tests/_host_contract.py`` measures this host's own filesystem answers once at import and
offers the canonical constructors for the fixtures that depend on them; a *pure* path rule is a
question about a dialect and is driven over both dialects. This lint is that contract's tripwire: it
scans every ``scripts/tests/**/*.py`` (the helper itself and this file excepted) and fails on the
idioms that were each one red CI leg before the contract existed --

- a fixture anchored on a bare separator (``os.sep``, ``os.path.join(os.sep, ...)``, a join anchored
  on a leading-slash literal): the host's dialect spelled by hand where ``make_abs_path``/
  ``make_rooted_path`` or a ``pathmod`` parameter is the answer;
- an errno subclass pinned for a create collision (``assertRaises(FileExistsError)`` and family):
  the product's refusal is asserted through ``assert_create_refused``, and a case that deliberately
  injects an errno through a seam marks the line ``# host-contract: synthetic-errno``;
- a raw ``os.symlink``/``Path.symlink_to``: links are staged through ``make_symlink``, which says
  whether the host can make one;
- a ``":"`` literal building or splitting ``PATH``: the split and the join go through
  ``os.pathsep``, and the lint reads the statement the call stands in for the ``PATH`` it names;
- an ``os.access(..., os.X_OK)`` assertion whose enclosing function does not consult
  ``HOST_SEMANTICS.exec_lookup_follows_mode_bits``: Windows answers ``X_OK`` for any existing file
  and a POSIX scratch mount can be mounted ``noexec``, so an executability assertion without the
  measured answer pins nothing there.

The lint is a tripwire, not a prover: a premise smuggled through a helper of the case's own is out of
its reach. Its own bite is pinned case by case below, and the gate's ``selftest`` step runs this file
(the discovery imports it) on every host.
"""

from __future__ import annotations

import ast
import dataclasses
import io
import os
import pathlib
import sys
import tempfile
import tokenize
import unittest
from unittest import mock

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import _host_contract as HOST_CONTRACT  # noqa: E402  (the suite's host contract, beside this file)

EXCLUDED = {HERE / "_host_contract.py", HERE / "test_host_contract.py"}

MARKER = "host-contract: synthetic-errno"

ERRNO_CLASSES = frozenset({
    "OSError", "IOError", "EnvironmentError", "BlockingIOError", "ChildProcessError",
    "ConnectionError", "BrokenPipeError", "ConnectionAbortedError", "ConnectionRefusedError",
    "ConnectionResetError", "FileExistsError", "FileNotFoundError", "InterruptedError",
    "IsADirectoryError", "NotADirectoryError", "PermissionError", "ProcessLookupError",
    "TimeoutError",
})

SPLIT_JOIN_NAMES = frozenset({"split", "rsplit", "join"})
ASSERT_RAISES_NAMES = frozenset({"assertRaises", "assertRaisesRegex"})

_SEPARATOR_DETAIL = ("a fixture anchored on a bare separator spells one dialect's spelling; build it "
                     "through _host_contract.make_abs_path/make_rooted_path, or drive the path "
                     "dialects")
_EXEC_BIT_DETAIL = ("an executability assertion does not consult "
                    "HOST_SEMANTICS.exec_lookup_follows_mode_bits, which says whether this host's "
                    "executable lookup answers for a mode-bit executable")


def _comment_text_per_line(source: str) -> dict[int, str]:
    text: dict[int, str] = {}
    for token in tokenize.generate_tokens(io.StringIO(source).readline):
        if token.type == tokenize.COMMENT:
            text[token.start[0]] = token.string
    return text


def _dotted(node: ast.AST) -> str:
    """The dotted name an expression spells, ``os.path.sep`` included, or ``""``."""
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        prefix = _dotted(node.value)
        return f"{prefix}.{node.attr}" if prefix else node.attr
    return ""


class _Scanner:
    def __init__(self, source: str):
        self.source = source
        self.comments = _comment_text_per_line(source)
        self.findings: list[tuple[int, str, str]] = []
        self._functions: list[ast.AST] = []
        self._consulting: set[ast.AST | None] = set()
        self._access_calls: list[tuple[int, ast.AST | None]] = []

    def scan(self, tree: ast.AST) -> list[tuple[int, str, str]]:
        self._walk(tree, None)
        for lineno, function in self._access_calls:
            if function not in self._consulting:
                self.findings.append((lineno, "exec-bit-premise", _EXEC_BIT_DETAIL))
        return sorted(self.findings)

    def _flag(self, node: ast.AST, rule: str, detail: str) -> None:
        self.findings.append((node.lineno, rule, detail))

    def _exempted(self, node: ast.AST) -> bool:
        return any(MARKER in self.comments.get(line, "")
                   for line in range(max(node.lineno - 1, 1), node.end_lineno + 1))

    def _walk(self, node: ast.AST, statement: ast.stmt | None) -> None:
        pushed = isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))
        if pushed:
            self._functions.append(node)
        function = self._functions[-1] if self._functions else None
        if isinstance(node, ast.stmt):
            statement = node
        self._check(node, function)
        if isinstance(node, ast.Call):
            self._check_call(node, statement, function)
        for child in ast.iter_child_nodes(node):
            self._walk(child, statement)
        if pushed:
            self._functions.pop()

    def _check(self, node: ast.AST, function: ast.AST | None) -> None:
        if isinstance(node, ast.Attribute):
            if node.attr == "sep" and _dotted(node.value) in ("os", "os.path"):
                self._flag(node, "bare-separator-fixture", _SEPARATOR_DETAIL)
            if node.attr == "exec_lookup_follows_mode_bits":
                self._consulting.add(function)

    def _check_call(self, node: ast.Call, statement: ast.stmt | None, function: ast.AST | None) -> None:
        name = node.func.attr if isinstance(node.func, ast.Attribute) else (
            node.func.id if isinstance(node.func, ast.Name) else "")
        if name == "join":
            receiver = node.func.value
            if isinstance(receiver, ast.Constant) and receiver.value in ("/", "\\"):
                self._flag(node, "bare-separator-fixture", _SEPARATOR_DETAIL)
            first = node.args[0] if node.args else None
            if (isinstance(first, ast.Constant) and isinstance(first.value, str)
                    and first.value.startswith("/")):
                self._flag(node, "bare-separator-fixture",
                           f"a join anchored on {first.value!r} spells a drive-less root outside this "
                           f"host's dialect; build the fixture through "
                           f"_host_contract.make_abs_path/make_rooted_path")
        if name in ASSERT_RAISES_NAMES and node.args:
            first = node.args[0]
            if isinstance(first, ast.Name) and first.id in ERRNO_CLASSES and not self._exempted(node):
                self._flag(node, "errno-class-on-a-create",
                           f"{first.id} is this host's errno answer, not the product's refusal; assert "
                           f"the refusal through assert_create_refused, or mark a deliberate "
                           f"injection with '# {MARKER}'")
        if name == "symlink_to" or (name == "symlink" and _dotted(node.func) == "os.symlink"):
            self._flag(node, "raw-symlink",
                       "stage a link through _host_contract.make_symlink, which answers whether this "
                       "host can make one")
        if name in SPLIT_JOIN_NAMES and statement is not None:
            if name in ("split", "rsplit"):
                candidates = list(node.args[:1])
            elif isinstance(node.func, ast.Attribute):
                candidates = [node.func.value]
            else:
                candidates = list(node.args[:1])
            separator = next((s for s in candidates
                              if isinstance(s, ast.Constant) and s.value == ":"), None)
            if (separator is not None
                    and "PATH" in (ast.get_source_segment(self.source, statement) or "")):
                self._flag(separator, "path-separator-literal",
                           "PATH is split and joined on os.pathsep, never a literal ':'; the "
                           "statement stands in for one, and that entry separator is the platform's")
        if name == "access" and _dotted(node.func) == "os.access":
            modes = list(node.args[1:2]) + [kw.value for kw in node.keywords if kw.arg == "mode"]
            if any(isinstance(mode, ast.Attribute) and mode.attr == "X_OK" for mode in modes):
                self._access_calls.append((node.lineno, function))


def scan_source(source: str, filename: str = "<source>") -> list[tuple[int, str, str]]:
    return _Scanner(source).scan(ast.parse(source, filename))


def scan_file(path: pathlib.Path) -> list[tuple[pathlib.Path, int, str, str]]:
    source = path.read_text(encoding="utf-8-sig")
    return [(path, line, rule, detail) for line, rule, detail in scan_source(source, str(path))]


def scan_directory(directory: pathlib.Path) -> list[tuple[pathlib.Path, int, str, str]]:
    found: list[tuple[pathlib.Path, int, str, str]] = []
    for path in sorted(directory.rglob("*.py")):
        if path.resolve() in EXCLUDED or "__pycache__" in path.parts:
            continue
        found.extend(scan_file(path))
    return found


def _planted(text: str) -> list[tuple[int, str, str]]:
    with tempfile.TemporaryDirectory(prefix="host-contract-lint-") as scratch:
        path = pathlib.Path(scratch) / "test_planted.py"
        path.write_text(text, encoding="utf-8")
        return [(line, rule, detail) for _path, line, rule, detail in scan_file(path)]


class TheSuitesOwnFilesObeyTheHostContract(unittest.TestCase):
    def test_no_banned_host_premise_stands_in_scripts_tests(self) -> None:
        findings = scan_directory(HERE)
        self.assertEqual(findings, [],
                         "banned host-premise idioms stand in scripts/tests:\n" + "\n".join(
                             f"{path}:{line}: {rule}: {detail}"
                             for path, line, rule, detail in findings))


class TheLintBites(unittest.TestCase):
    """Each rule planted in a scratch file: the lint names it, and its controls stay silent."""

    def assert_flagged(self, text: str, rule: str, line: int) -> None:
        found = _planted(text)
        self.assertEqual([(number, found_rule) for number, found_rule, _ in found], [(line, rule)],
                         f"planted {rule} was not the only finding: {found}")

    def assert_not_flagged(self, text: str) -> None:
        self.assertEqual(_planted(text), [], "a control snippet was flagged")

    def test_a_bare_separator_fixture_is_named(self) -> None:
        for snippet, line in (
            ("import os\nqualified = os.path.join(os.sep, \"opt\", \"bin\")\n", 2),
            ("import os\nqualified = os.sep + \"opt\"\n", 2),
            ("import os\nqualified = os.path.join(\"/\", \"opt\", \"bin\")\n", 2),
            ("import os\nqualified = os.path.join(\"/opt\", \"bin\")\n", 2),
            ("joined = \"/\".join(parts)\n", 1),
        ):
            with self.subTest(snippet=snippet):
                self.assert_flagged(snippet, "bare-separator-fixture", line)
        self.assert_not_flagged("import os\njoined = os.path.join(root, \"child\")\n")

    def test_an_errno_subclass_on_a_create_is_named(self) -> None:
        self.assert_flagged(
            "import unittest\n\n\nclass T(unittest.TestCase):\n"
            "    def test_refusal(self):\n"
            "        with self.assertRaises(FileExistsError):\n"
            "            plant()\n",
            "errno-class-on-a-create", 6)
        self.assert_flagged(
            "import unittest\n\n\nclass T(unittest.TestCase):\n"
            "    def test_refusal(self):\n"
            "        self.assertRaises(OSError, plant)\n",
            "errno-class-on-a-create", 6)
        self.assert_not_flagged(
            "import unittest\n\n\nclass T(unittest.TestCase):\n"
            "    def test_exit(self):\n"
            "        with self.assertRaises(SystemExit):\n"
            "            main()\n")

    def test_a_marked_injection_is_exempt(self) -> None:
        marked = ("import unittest\n\n\nclass T(unittest.TestCase):\n"
                  "    def test_refusal(self):\n"
                  "        # host-contract: synthetic-errno: this seam injects the platform's answer\n"
                  "        with self.assertRaises(FileExistsError):\n"
                  "            plant()\n")
        self.assertEqual(_planted(marked), [], "the marker did not exempt the deliberate injection")

    def test_a_raw_symlink_is_named(self) -> None:
        self.assert_flagged(
            "import os\nos.symlink(\"target\", \"link\")\n", "raw-symlink", 2)
        self.assert_flagged(
            "from pathlib import Path\nPath(\"link\").symlink_to(\"target\")\n", "raw-symlink", 2)
        self.assert_not_flagged(
            "import os\nanswered = os.readlink(\"link\")\n")

    def test_a_literal_path_separator_is_named(self) -> None:
        self.assert_flagged(
            "import os\nentries = os.environ[\"PATH\"].split(\":\")\n", "path-separator-literal", 2)
        self.assert_flagged(
            "import os\nos.environ[\"PATH\"] = \":\".join(entries)\n", "path-separator-literal", 2)
        self.assert_not_flagged(
            "line = \"stderr: a detail\"\nname = line.split(\":\", 1)[1]\n")
        self.assert_not_flagged(
            "import os\nentries = os.environ[\"PATH\"].split(os.pathsep)\n")

    def test_an_exec_bit_assertion_without_the_measured_answer_is_named(self) -> None:
        self.assert_flagged(
            "import os\n\n\ndef executable(path):\n"
            "    os.chmod(path, 0o755)\n"
            "    return os.access(path, os.X_OK)\n",
            "exec-bit-premise", 6)
        self.assert_not_flagged(
            "import os\n\nfrom _host_contract import HOST_SEMANTICS\n\n\n"
            "def executable(path):\n"
            "    os.chmod(path, 0o755)\n"
            "    if HOST_SEMANTICS.exec_lookup_follows_mode_bits:\n"
            "        return os.access(path, os.X_OK)\n"
            "    return path.exists()\n")
        self.assert_not_flagged(
            "import os\n\n\ndef readable(path):\n"
            "    return os.access(path, os.R_OK)\n")
        self.assert_flagged(
            "import os\n\nfrom _host_contract import HOST_SEMANTICS\n\n\n"
            "def measured():\n"
            "    return HOST_SEMANTICS.exec_lookup_follows_mode_bits\n\n\n"
            "def executable(path):\n"
            "    return os.access(path, os.X_OK)\n",
            "exec-bit-premise", 11)


class TheProbeKeepsTheTwoExecBitQuestionsApart(unittest.TestCase):
    """The probe answers mode-bit storage from ``stat`` and executable lookup from ``access``.

    A POSIX scratch mount mounted ``noexec`` refuses ``os.access(..., X_OK)`` for a file whose stored
    owner-execute bit is set; the pre-fix probe folded that refusal into its single answer, and
    ``_assert_sane`` then raised at import, before the suite could run (the r8 finding). Storage is
    the answer the parity checker's own read (``stat``) depends on, so it must not move with the
    mount; the lookup answer is measured beside it and never asserted.
    """

    def probe_with_lookup_refused(self) -> tuple[bool, bool]:
        real_access = os.access

        def refusing(path, mode, **kwargs):
            if mode == os.X_OK:
                return False
            return real_access(path, mode, **kwargs)

        with tempfile.TemporaryDirectory(prefix="host-contract-exec-") as scratch:
            with mock.patch.object(HOST_CONTRACT.os, "access", refusing):
                return HOST_CONTRACT._probe_exec_bits(pathlib.Path(scratch))

    def test_a_refused_exec_lookup_leaves_the_storage_answer_standing(self) -> None:
        stored, follows = self.probe_with_lookup_refused()
        self.assertEqual(stored, HOST_CONTRACT.HOST_SEMANTICS.mode_bits_stored,
                         "the mount's exec policy moved the probe's mode-bit storage answer")
        self.assertFalse(follows, "a refused X_OK was answered as a lookup that followed the bits")
        HOST_CONTRACT._assert_sane(dataclasses.replace(
            HOST_CONTRACT.HOST_SEMANTICS, mode_bits_stored=stored,
            exec_lookup_follows_mode_bits=follows))

    def test_a_storage_answer_the_platform_contradicts_is_still_loud(self) -> None:
        """This case is the POSIX arm, where storage is a documented rule rather than a measurement
        that may legitimately answer ``False`` (win32 stores no POSIX mode bits)."""
        if os.name != "posix":
            self.skipTest("the assertion it pins is the POSIX arm of _assert_sane")
        with self.assertRaises(AssertionError):
            HOST_CONTRACT._assert_sane(dataclasses.replace(
                HOST_CONTRACT.HOST_SEMANTICS, mode_bits_stored=False))


if __name__ == "__main__":
    unittest.main()
