#!/usr/bin/env python3
"""Regression tests for the resume-invocation check in scripts/codex-native-check.py.

That check is the harness's own rule about the line a caller pastes or an agent runs: it must name the
data directory the session was installed into, because Codex takes that directory from CODEX_HOME alone
and has no flag for it. The line is *rendered* for a POSIX shell, so a check written against a literal
``CODEX_HOME=<home>`` substring answers the opposite of the rule in both directions: it accepts an
unquoted home whose space a shell reads as two words -- the defect the quoting exists to prevent -- and
it rejects every home the renderer correctly quotes, which is every home with a space and every Windows
path with a backslash (the shapes CI's windows-latest leg produces, where the substring form was
measured failing a run whose conversion, environment and native load were all correct).

The cases drive the shipped module in-process, imported by path, with no subprocess and no tool on
``PATH`` -- the way ``scripts/tests/test_upstream_parity.py`` drives its scripts: the reading of a
rendered line is pure Python, and a case that needed the Codex binary would not be a gate step. The
renderer's own half of the contract is pinned by the Go tests that assert the quoted spelling
(``cmd/ccc/main_test.go``); these cases pin the half that reads that line back.
"""

from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1]

#: A session id and binary the cases use: the check compares words, so their values only have to be
#: spelled the same on both sides.
SESSION = "01a1e225-a986-7372-bf0f-2c51da684752"
CODEX = "codex"

RESUME_ARGUMENTS = [CODEX, "resume", SESSION]
EXEC_RESUME_ARGUMENTS = [CODEX, "exec", "resume", SESSION, "<prompt>"]

#: One home and the assignment `internal/ccc/continue.go`'s `shellQuote` renders for it: a word holding
#: nothing a shell would act on stands bare, and one holding a space, a backslash or a quote is
#: single-quoted with an embedded quote closed and re-opened (`'it'\''s'`). The space case is the exact
#: spelling the CLI test asserts for a home with a space in it.
RENDERINGS = (
    ("/home/user/.codex", "CODEX_HOME=/home/user/.codex"),
    ("/tmp/codex home/.codex", "CODEX_HOME='/tmp/codex home/.codex'"),
    (r"C:\Users\a b\.codex", r"CODEX_HOME='C:\Users\a b\.codex'"),
    ("/tmp/it's home/.codex", "CODEX_HOME='/tmp/it'\\''s home/.codex'"),
)


def load_native_check():
    """The shipped harness, imported by path (its file name is not an identifier)."""
    spec = importlib.util.spec_from_file_location(
        "shipped_codex_native_check", SCRIPTS / "codex-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


class ResumeInvocationCheck(unittest.TestCase):
    def test_accepts_the_rendering_the_renderer_emits(self) -> None:
        for home, assignment in RENDERINGS:
            with self.subTest(home=home):
                resume = f"{assignment} {CODEX} resume {SESSION}"
                exec_resume = f"{assignment} {CODEX} exec resume {SESSION} '<prompt>'"
                self.assertTrue(NATIVE_CHECK.reported_invocation(resume, Path(home), RESUME_ARGUMENTS),
                                f"a correct rendering was rejected: {resume!r}")
                self.assertTrue(
                    NATIVE_CHECK.reported_invocation(exec_resume, Path(home), EXEC_RESUME_ARGUMENTS),
                    f"a correct rendering was rejected: {exec_resume!r}")

    def test_rejects_an_unquoted_home_a_shell_would_split(self) -> None:
        # The defect the quoted rendering prevents: a shell reads the space as the end of the assignment
        # word, so the line opens a home named `/tmp/codex` instead of the one that was installed into.
        # The substring form of this check accepted exactly this line.
        line = f"CODEX_HOME=/tmp/codex home/.codex {CODEX} resume {SESSION}"
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("/tmp/codex home/.codex"),
                                                         RESUME_ARGUMENTS))

    def test_rejects_a_command_that_names_another_home(self) -> None:
        line = f"CODEX_HOME=/elsewhere/.codex {CODEX} resume {SESSION}"
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("/tmp/codex home/.codex"),
                                                         RESUME_ARGUMENTS))

    def test_rejects_a_command_without_the_assignment(self) -> None:
        line = f"{CODEX} resume {SESSION}"
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("/tmp/.codex"), RESUME_ARGUMENTS))

    def test_rejects_a_command_for_another_session(self) -> None:
        line = f"CODEX_HOME=/tmp/.codex {CODEX} resume 00000000-0000-0000-0000-000000000000"
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("/tmp/.codex"), RESUME_ARGUMENTS))

    def test_rejects_a_line_a_shell_cannot_read(self) -> None:
        # An unbalanced quote is not a command line at all: a shell would keep reading, so the reported
        # invocation could not be the one this check is about.
        line = f"CODEX_HOME='/tmp/.codex {CODEX} resume {SESSION}"
        self.assertIsNone(NATIVE_CHECK.shell_words(line))
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("/tmp/.codex"), RESUME_ARGUMENTS))


if __name__ == "__main__":
    unittest.main()
