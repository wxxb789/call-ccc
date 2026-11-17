#!/usr/bin/env python3
"""Regression tests for the checks in scripts/pi-native-check.py that need no Pi binary.

Three subjects, all driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the reading of the reported resume invocation (``shell_words`` -- what its words are -- with
``unquoted_text``/``spelled_as_invocation`` for the spelling that decides whether a shell reads the line
as that `cd` and that assignment at all), the route selection ``--direction`` makes and the ``INCOMPLETE``
verdict a run reports when the target harness is not installed (``main`` driven with its subprocess and
``which`` seams replaced by stubs), and the answer block's printed shape. A fourth subject is the
fixture-copy mechanism the Pi and Claude harnesses share: the working directory is rewritten through the
session's own **JSON record field**, never through its raw bytes.

The resume-invocation check is the harness's own rule about the line a caller pastes or an agent runs: it
must name the working directory the target's loader is started from and the data directory the session was
installed into, because Pi resolves a session id through the project directory of the process's own
working directory and takes its agent directory from the environment alone -- it has no flag for either.
The line is *rendered* for a POSIX shell, so a check written against a literal substring answers the
opposite of the rule in both directions: it accepts an unquoted value whose character a shell would act on
-- the defect the quoting exists to prevent -- and it rejects every value the renderer correctly quotes,
which is every path with a space and every Windows path with a backslash. The Pi harness applies the same
rule its Claude sibling does, spelled by ``spelled_as_invocation``.

The cases drive the shipped module in-process, imported by path, with no subprocess and no tool on
``PATH`` -- the way ``scripts/tests/test_claude_native_check.py`` drives its script: the reading of a
rendered line is pure Python, and a case that needed the Pi binary would not be a gate step. The renderer's
own half of the contract is pinned by the Go tests that assert the quoted spelling
(``cmd/ccc/main_test.go``, ``internal/ccc/continue_test.go``); these cases pin the half that reads that
line back.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import shlex
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path, PureWindowsPath
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]

#: A session id and binary the cases use: the check compares words, so their values only have to be
#: spelled the same on both sides.
SESSION = "01a1e225-a986-7372-bf0f-2c51da684752"
OTHER_SESSION = "00000000-0000-0000-0000-000000000000"
PI = "pi"
PI_HOME_ENV = "PI_CODING_AGENT_DIR"

RESUME_ARGUMENTS = [PI, "--session", SESSION]
EXEC_RESUME_ARGUMENTS = [PI, "--session", SESSION, "--", "<prompt>"]

#: ``(the working directory, the data directory)``. Each is a single-segment name, a Windows-shaped path
#: or a name holding a quote, so both ``pathlib`` flavours spell it exactly as the case does.
RENDERINGS = (
    ("workspace", "pi-home"),
    ("work space", "pi home"),
    (r"C:\code\a b", r"C:\Users\a b\.pi"),
    ("it's dir", "it's home"),
)

#: The path flavours a case drives the check with: this host's own, and Windows' answer on any host.
PATH_FLAVOURS = (Path, PureWindowsPath)

#: The marker the resume check prints the target's answer under: part of the reporting contract, because
#: ``scripts/tests/test_evidence_claims.py`` reads a recorded answer from it.
TURN_OUTPUT_MARKER = ("the turn's own output (the target's answer, as the harness printed it):")

#: The checks ``_check_invocation`` records for the two printed commands, as the shipped module names
#: them.
RESUME_CHECK = "the printed resume_command reads as a POSIX shell line naming the session"
EXEC_RESUME_CHECK = "the printed exec_resume_command reads as a POSIX shell line naming the session"

#: The one check a run records FAIL for when the target harness is not installed, as the shipped module
#: names it.
MISSING_TARGET_CHECK = "the target harness (claude) is installed"


def load_native_check():
    """The shipped harness, imported by path (its file name is not an identifier)."""
    spec = importlib.util.spec_from_file_location(
        "shipped_pi_native_check", SCRIPTS / "pi-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


def invocation(cwd, home, home_env: str, arguments: list[str],
               session_dir_env: str | None = None, session_dir: str | None = None) -> str:
    """One invocation as the shipped renderer spells it: the `cd`, then the assignments, then the words.

    ``internal/ccc/continue.go``'s ``renderInvocation`` starts with ``cd <shellQuote(workingDir)> &&`` and
    then ``renderResumeCommand``'s ``NAME=<shellQuote(value)>`` plus one ``shellQuote`` per argument: the
    assignment's *name* and its `=` stay bare -- quoting them would turn the assignment into a program's
    name to a shell -- while the value and every other word are single-quoted exactly where a POSIX shell
    would act on them, which is what ``shlex.quote`` does. The assignments are sorted by name, which is
    what puts the agent directory before the sessions tree.
    """
    assignments = [(home_env, str(home))]
    if session_dir_env:
        assignments.append((session_dir_env, str(session_dir)))
    pieces = [shlex.quote(NATIVE_CHECK.CD_WORD), shlex.quote(str(cwd)), NATIVE_CHECK.OPERATOR_WORD]
    pieces += [f"{name}={shlex.quote(value)}" for name, value in sorted(assignments)]
    pieces += [shlex.quote(word) for word in arguments]
    return " ".join(pieces)


def invocation_states(target: dict, target_provider: str = "pi", *, workspace="workspace",
                      home="pi-home") -> dict[str, str]:
    """The check states ``_check_invocation`` records for ``target``, restored afterwards.

    ``workspace`` and ``home`` are the working directory and the data directory the check is told this run
    used: the printed line is compared against them (and against the argument vector the report printed),
    so a case that hands the check other values is a case about the full word-vector rule.
    """
    before = len(NATIVE_CHECK.RESULTS)
    with contextlib.redirect_stdout(io.StringIO()):
        NATIVE_CHECK._check_invocation(target, target_provider, workspace, home)
    states = {name: state for name, state, _ in NATIVE_CHECK.RESULTS[before:]}
    del NATIVE_CHECK.RESULTS[before:]
    return states


class ReportedInvocationReading(unittest.TestCase):
    """The printed resume commands are read the way a POSIX shell reads them.

    The rule is the sibling ``scripts/claude-native-check.py``'s, and it reads two halves: the *words* a
    shell splits the line into -- compared with the `cd`, the working directory the loader is started from,
    the data directory the session was installed into and the exact argument vector the report itself
    printed -- and the line's own *spelling* (``spelled_as_invocation``), what a shell reads once the
    quoting is taken into account. A check with only the spelling half would accept any `cd <anything>`
    beside any `NAME=<anything>` beside any session id, because those are the right words in the right
    order; a check with only the word half would accept an unquoted value a shell acts on, because that
    reads back as the same word. Both refusal families are below, so a check that dropped either half
    would accept a line this run did not print.
    """

    def assert_rejected(self, line: str, workspace, home, arguments: list[str],
                        words: list[str]) -> None:
        """``line`` is not the invocation, and the shell's own reading says why.

        The words are asserted first, so a refusal cannot come from a spelling accident: the case names the
        vector a shell splits the line into (another directory, another home, another session, a directory
        the shell splits), and then both halves of the rule refuse it.
        """
        self.assertEqual(NATIVE_CHECK.shell_words(line), words, line)
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, flavour(workspace), flavour(home),
                                                     PI_HOME_ENV, arguments,
                                                     NATIVE_CHECK.PI_SESSION_DIR_ENV), line)
        target = {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
                  "resume_arguments": arguments, "exec_resume_arguments": arguments}
        states = invocation_states(target, "pi", workspace=workspace, home=home)
        self.assertNotIn("PASS", states.values(), f"{line!r} was accepted: {states}")

    def test_accepts_a_pi_rendering_the_renderer_emits(self) -> None:
        """Both printed lines read back as the `cd`, the assignments and the argument vector."""
        for cwd, home in RENDERINGS:
            for flavour in PATH_FLAVOURS:
                with self.subTest(cwd=cwd, home=home, flavour=flavour.__name__):
                    session_dir = NATIVE_CHECK.pi_session_tree(flavour(home), flavour(cwd))
                    resume = invocation(flavour(cwd), flavour(home), PI_HOME_ENV, RESUME_ARGUMENTS,
                                        NATIVE_CHECK.PI_SESSION_DIR_ENV, session_dir)
                    exec_resume = invocation(flavour(cwd), flavour(home), PI_HOME_ENV,
                                             EXEC_RESUME_ARGUMENTS, NATIVE_CHECK.PI_SESSION_DIR_ENV,
                                             session_dir)
                    words = [NATIVE_CHECK.CD_WORD, str(flavour(cwd)), NATIVE_CHECK.OPERATOR_WORD,
                             f"{PI_HOME_ENV}={flavour(home)}",
                             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}={session_dir}"]
                    self.assertEqual(NATIVE_CHECK.shell_words(resume), [*words, *RESUME_ARGUMENTS],
                                     resume)
                    self.assertEqual(NATIVE_CHECK.shell_words(exec_resume),
                                     [*words, *EXEC_RESUME_ARGUMENTS], exec_resume)
                    target = {"session_id": SESSION, "resume_command": resume,
                              "exec_resume_command": exec_resume,
                              "resume_arguments": RESUME_ARGUMENTS,
                              "exec_resume_arguments": EXEC_RESUME_ARGUMENTS}
                    states = invocation_states(target, "pi", workspace=flavour(cwd), home=flavour(home))
                    self.assertEqual(states.get(RESUME_CHECK), "PASS",
                                     f"a correct rendering was rejected: {resume!r} — {states}")
                    self.assertEqual(states.get(EXEC_RESUME_CHECK), "PASS",
                                     f"a correct rendering was rejected: {exec_resume!r} — {states}")
                    self.assertTrue(NATIVE_CHECK.spelled_as_invocation(resume, PI_HOME_ENV), resume)

    def test_rejects_a_pi_line_without_the_sessions_tree(self) -> None:
        """A line naming only the data directory is not this run's invocation.

        Pi reads ``PI_CODING_AGENT_SESSION_DIR`` ahead of the agent directory's own default, so a caller
        whose shell holds the variable, or a data directory whose ``settings.json`` names another tree,
        would send this line looking in that tree while the conversion installed the session under the
        data directory's own sessions root. The route prints the tree for that reason, and a line without
        it is another invocation however plausible it reads.
        """
        line = invocation("workspace", "pi-home", PI_HOME_ENV, RESUME_ARGUMENTS)
        self.assert_rejected(
            line, "workspace", "pi-home", RESUME_ARGUMENTS,
            ["cd", "workspace", "&&", f"{PI_HOME_ENV}=pi-home", PI, "--session", SESSION])

    def test_rejects_a_pi_line_that_names_another_sessions_tree(self) -> None:
        self.assert_rejected(
            invocation("workspace", "pi-home", PI_HOME_ENV, RESUME_ARGUMENTS,
                       NATIVE_CHECK.PI_SESSION_DIR_ENV, "pi-home/sessions/--elsewhere--"),
            "workspace", "pi-home", RESUME_ARGUMENTS,
            ["cd", "workspace", "&&", f"{PI_HOME_ENV}=pi-home",
             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}=pi-home/sessions/--elsewhere--",
             PI, "--session", SESSION])

    def test_rejects_a_pi_line_for_another_working_directory(self) -> None:
        self.assert_rejected(
            invocation("workspace-elsewhere", "pi-home", PI_HOME_ENV, RESUME_ARGUMENTS,
                       NATIVE_CHECK.PI_SESSION_DIR_ENV,
                       NATIVE_CHECK.pi_session_tree("pi-home", "workspace-elsewhere")),
            "workspace", "pi-home", RESUME_ARGUMENTS,
            ["cd", "workspace-elsewhere", "&&", f"{PI_HOME_ENV}=pi-home",
             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}="
             + NATIVE_CHECK.pi_session_tree("pi-home", "workspace-elsewhere"),
             PI, "--session", SESSION])

    def test_rejects_a_pi_line_that_names_another_home(self) -> None:
        self.assert_rejected(
            invocation("workspace", "pi-elsewhere", PI_HOME_ENV, RESUME_ARGUMENTS,
                       NATIVE_CHECK.PI_SESSION_DIR_ENV,
                       NATIVE_CHECK.pi_session_tree("pi-elsewhere", "workspace")),
            "workspace", "pi-home", RESUME_ARGUMENTS,
            ["cd", "workspace", "&&", f"{PI_HOME_ENV}=pi-elsewhere",
             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}="
             + NATIVE_CHECK.pi_session_tree("pi-elsewhere", "workspace"),
             PI, "--session", SESSION])

    def test_rejects_a_pi_line_for_another_session(self) -> None:
        self.assert_rejected(
            invocation("workspace", "pi-home", PI_HOME_ENV,
                       [PI, "--session", OTHER_SESSION], NATIVE_CHECK.PI_SESSION_DIR_ENV,
                       NATIVE_CHECK.pi_session_tree("pi-home", "workspace")),
            "workspace", "pi-home", RESUME_ARGUMENTS,
            ["cd", "workspace", "&&", f"{PI_HOME_ENV}=pi-home",
             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}="
             + NATIVE_CHECK.pi_session_tree("pi-home", "workspace"),
             PI, "--session", OTHER_SESSION])

    def test_rejects_a_pi_working_directory_a_shell_would_split(self) -> None:
        # The defect the quoted rendering prevents: a shell reads the space as the end of the directory
        # word, so the loader is started somewhere else and finds another session, or none at all. The
        # sessions tree is spelled as the renderer would quote it, so the split is the cwd's own.
        sessions_tree = NATIVE_CHECK.pi_session_tree("pi-home", "work space")
        self.assert_rejected(
            f"cd work space && {PI_HOME_ENV}=pi-home "
            f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}={shlex.quote(sessions_tree)} {PI} --session {SESSION}",
            "work space", "pi-home", RESUME_ARGUMENTS,
            ["cd", "work", "space", "&&", f"{PI_HOME_ENV}=pi-home",
             f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}={sessions_tree}", PI, "--session", SESSION])

    def test_rejects_a_pi_assignment_a_shell_reads_as_a_command(self) -> None:
        """The words read back right and the line is still not the invocation.

        A shell recognizes an assignment only while nothing in its name or its `=` is quoted, so each
        spelling below is the *program* to run -- one named ``PI_CODING_AGENT_DIR=pi-home``, or one whose
        name a shell would spell that way only after removing the quoting -- and nothing sets the agent
        directory.
        """
        assignment = f"{PI_HOME_ENV}=pi-home"
        for line in (
            f"cd workspace && '{assignment}' {PI} --session {SESSION}",
            f'cd workspace && "{assignment}" {PI} --session {SESSION}',
            f"cd workspace && PI_CODING_AGENT_'DIR=pi-home' {PI} --session {SESSION}",
            f"cd workspace && {PI_HOME_ENV}\\=pi-home {PI} --session {SESSION}",
        ):
            with self.subTest(line=line):
                self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
                states = invocation_states(
                    {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
                     "resume_arguments": RESUME_ARGUMENTS,
                     "exec_resume_arguments": RESUME_ARGUMENTS}, "pi")
                self.assertNotIn("PASS", states.values(), f"{line!r} was accepted: {states}")

    def test_rejects_pi_syntax_the_value_leaves_unquoted(self) -> None:
        """A value the renderer quotes, spelled bare: the words read back right and the line is another
        command line -- `;` ends the assignment and starts a program, a bare `&` does the same, `$HOME` is
        substituted before the variable is set, and a backquote substitutes a command. The rule is about
        spellings: a character a shell acts on has to be quoted, which `shellQuote` does for every one."""
        for home in ("a;b", "a&b", "$HOME", "`id`", "~"):
            line = (f"cd workspace && {PI_HOME_ENV}={home} "
                    f"{NATIVE_CHECK.PI_SESSION_DIR_ENV}="
                    f"{NATIVE_CHECK.pi_session_tree(home, 'workspace')} {PI} --session {SESSION}")
            with self.subTest(line=line):
                self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
                # The line's own value is the home this case names, so the word vector matches and only
                # the spelling refuses it: that is what makes the case about the spelling rule. The
                # sessions-tree value is the tree under that same home, spelled bare the same way.
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, Path("workspace"), Path(home), PI_HOME_ENV,
                                                     RESUME_ARGUMENTS,
                                                     NATIVE_CHECK.PI_SESSION_DIR_ENV), line)
                states = invocation_states(
                    {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
                     "resume_arguments": RESUME_ARGUMENTS,
                     "exec_resume_arguments": RESUME_ARGUMENTS}, "pi", home=home)
                self.assertNotIn("PASS", states.values(), f"{line!r} was accepted: {states}")

    def test_rejects_a_pi_operator_glued_to_a_word(self) -> None:
        """``PI_CODING_AGENT_DIR=a&&b`` is one word to the shell's reading and two commands to a shell.

        A `&&` a shell reads as its operator is a word of its own, with whitespace on both sides; glued to
        a value it is part of that word, so the line sets the variable to `a` and then runs `b`.
        """
        line = f"cd workspace && {PI_HOME_ENV}=a&&b {PI} --session {SESSION}"
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states(
            {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
             "resume_arguments": RESUME_ARGUMENTS, "exec_resume_arguments": RESUME_ARGUMENTS},
            "pi", home="a&&b")
        self.assertNotIn("PASS", states.values(), states)

    def test_rejects_a_quoted_operator_a_shell_would_read_as_a_word(self) -> None:
        """A quoted `&&` reads back as the same words and is not the operator.

        A shell reads ``'&&'`` as a word, so the `cd` is followed by an argument spelled `&&` and nothing
        separates the change from the assignment.
        """
        line = f"cd workspace '&&' {PI_HOME_ENV}=pi-home {PI} --session {SESSION}"
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states(
            {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
             "resume_arguments": RESUME_ARGUMENTS, "exec_resume_arguments": RESUME_ARGUMENTS}, "pi")
        self.assertNotIn("PASS", states.values(), states)

    def test_rejects_a_pi_line_a_shell_cannot_read(self) -> None:
        # An unbalanced quote is not a command line at all: a shell would keep reading. The word reading
        # answers None, and the check records its refusal for that reason.
        line = f"cd 'workspace && {PI_HOME_ENV}=pi-home {PI} --session {SESSION}"
        self.assertIsNone(NATIVE_CHECK.shell_words(line))
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states(
            {"session_id": SESSION, "resume_command": line, "exec_resume_command": line,
             "resume_arguments": RESUME_ARGUMENTS, "exec_resume_arguments": RESUME_ARGUMENTS}, "pi")
        self.assertNotIn("PASS", states.values(), states)


class AnswerBlockShape(unittest.TestCase):
    """The target's answer is printed under its marker, indented by four spaces, one line per line.

    ``scripts/tests/test_evidence_claims.py`` reads a recorded answer from the lines a run prints under
    ``TURN_OUTPUT_MARKER`` that start with four spaces, and both sibling harnesses indent by four; a run
    whose answer block is printed bare would leave that reading with zero recorded answer lines, so the
    record's §5 quotation would read as uncited.
    """

    def test_the_answer_is_printed_four_space_indented_under_the_marker(self) -> None:
        printed = io.StringIO()
        with contextlib.redirect_stdout(printed):
            NATIVE_CHECK.print_answer("PROBE-TURN-OK\n")
        self.assertIn(f"{TURN_OUTPUT_MARKER}\n    PROBE-TURN-OK\n", printed.getvalue(),
                      f"the answer block is not indented under its marker: {printed.getvalue()!r}")

    def test_a_long_answer_is_bounded_and_marked(self) -> None:
        # The bound is the shared one (lines and characters), and what it drops is named rather than
        # silently truncated.
        printed = io.StringIO()
        with contextlib.redirect_stdout(printed):
            NATIVE_CHECK.print_answer("\n".join(f"line {number}" for number in range(1000)) + "\n")
        lines = printed.getvalue().splitlines()
        self.assertIn(TURN_OUTPUT_MARKER, lines)
        self.assertTrue(any("were dropped" in line for line in lines),
                        "a bounded answer was not marked where it was cut")
        self.assertLess(len(lines), 300, "the printed answer block was not bounded by its line count")


class SourceCopyRewritesTheRecordField(unittest.TestCase):
    """The fixture copy is rewritten through the session's own JSON record field, not its raw bytes.

    Both the Pi and the Claude harnesses share ``rewrite_source_cwd``: it parses each record and rewrites
    the `cwd` field, so a value a message happens to spell as a byte run -- a topic the conversation is
    about -- is left alone, while a raw byte replace would rewrite it too. The recorded directory is what
    the copy answers back, so the run's header can name it.
    """

    def test_rewrites_the_cwd_field_and_not_a_value_a_message_spells(self) -> None:
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-rewrite-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        source = scratch / "source.jsonl"
        source.write_text(json.dumps({
            "type": "user", "cwd": "/home/user/code/example-api",
            "message": {"role": "user", "content": "I edited a file under /home/user/code/example-api"},
        }) + "\n", encoding="utf-8")
        destination = scratch / "copy.jsonl"
        recorded = NATIVE_CHECK.rewrite_source_cwd(source, destination, "/scratch/workspace")
        self.assertEqual(recorded, "/home/user/code/example-api")
        copied = json.loads(destination.read_text(encoding="utf-8").splitlines()[0])
        self.assertEqual(copied["cwd"], "/scratch/workspace")
        self.assertEqual(copied["message"]["content"],
                         "I edited a file under /home/user/code/example-api",
                         "the copy rewrote a value inside a message rather than the record's own field")

    def test_a_torn_line_is_carried_through_verbatim(self) -> None:
        # A session a crash left torn is not one JSON record; the target's own reader tolerates it, so the
        # copy carries the bytes through rather than failing to parse them.
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-rewrite-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        source = scratch / "source.jsonl"
        torn = '{"type":"assistant","message":{"text":"Reading the ret'
        source.write_text(json.dumps({"type": "session", "cwd": "/home/user/code/example-api"}) + "\n"
                          + torn + "\n", encoding="utf-8")
        destination = scratch / "copy.jsonl"
        recorded = NATIVE_CHECK.rewrite_source_cwd(source, destination, "/scratch/workspace")
        self.assertEqual(recorded, "/home/user/code/example-api")
        self.assertEqual(destination.read_text(encoding="utf-8").splitlines()[-1], torn)


class AssistantUsageRule(unittest.TestCase):
    """An assistant record without a ``usage`` object is caught by the parsed records, not a substring.

    Pi's own loader indexes ``message.usage`` on every assistant message it reads, so the check walks the
    file's records the way ``installed_entry_counts`` does and requires every assistant *message* to carry
    the object. A whole-file substring search for ``usage`` would be satisfied by a message whose text
    merely spells the word -- which is not a usage object, and a revert to one must fail this case.
    """

    def test_a_message_text_spelling_usage_is_not_a_usage_object(self) -> None:
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-usage-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        session = scratch / "session.jsonl"
        session.write_text("\n".join([
            json.dumps({"type": "session", "id": SESSION}),
            json.dumps({"type": "message",
                        "message": {"role": "assistant",
                                    "content": "the field is spelled usage in this prose"}}),
            json.dumps({"type": "message", "message": {"role": "assistant", "usage": {"input": 1}}}),
            json.dumps({"type": "message", "message": {"role": "user", "content": "hi"}}),
        ]) + "\n", encoding="utf-8")
        assistants, without = NATIVE_CHECK.assistant_messages_without_usage(session)
        self.assertEqual((assistants, without), (2, 1),
                         "the count did not read the records: a prose `usage` must not satisfy it, and a "
                         "user message must not be counted")


class SessionIdentityRule(unittest.TestCase):
    """A session id is read from each file's header, never from its name.

    A Pi session file is named ``<timestamp>_<id>.jsonl``; the stem carries the timestamp too, so a
    conversion that reused an id the home already held under *another* timestamp would compare as a
    different name. The rule reads the `session` record's own `id`, so the collision is caught.
    """

    def test_an_id_colliding_under_another_timestamp_is_already_held(self) -> None:
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-identity-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        home = scratch / "home" / "--x--"
        home.mkdir(parents=True)
        first = home / "2026-01-01T00-00-00-000Z_11111111-1111-4111-8111-111111111111.jsonl"
        installed = home / "2026-02-02T00-00-00-000Z_11111111-1111-4111-8111-111111111111.jsonl"
        collision = "11111111-1111-4111-8111-111111111111"
        for path in (first, installed):
            path.write_text(json.dumps({"type": "session", "id": collision}) + "\n", encoding="utf-8")
        self.assertNotEqual(first.stem, installed.stem,
                            "the case is vacuous unless the two names differ")
        self.assertEqual(NATIVE_CHECK.session_id_from_header(first), collision)
        self.assertTrue(NATIVE_CHECK.home_holds_id(scratch / "home", collision, exclude=installed),
                        "a colliding id under another timestamp was not seen as already held")


class HostProvidersReadIsGuarded(unittest.TestCase):
    """A host with no usable ``models.json`` reads as no providers rather than crashing.

    The module docstring promises "a host with no provider configured for the chosen model is reported as
    a named skip with that reason, not as a pass"; the guarded read is what keeps a missing file, an empty
    document and a ``providers`` value that is not an object from raising out of the run before its
    ``RESULT:`` line.
    """

    def test_a_missing_empty_or_malformed_models_file_reads_as_no_providers(self) -> None:
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-providers-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        self.assertEqual(NATIVE_CHECK.host_pi_providers(scratch / "missing"), {})
        models = scratch / "models.json"
        models.write_text("{}", encoding="utf-8")
        self.assertEqual(NATIVE_CHECK.host_pi_providers(scratch), {})
        models.write_text('{"providers": [1, 2]}', encoding="utf-8")
        self.assertEqual(NATIVE_CHECK.host_pi_providers(scratch), {})
        models.write_text("not json", encoding="utf-8")
        self.assertEqual(NATIVE_CHECK.host_pi_providers(scratch), {})


class SeedPiHomeDropsEveryCredentialLevel(unittest.TestCase):
    """``seed_pi_home`` copies the host's provider definitions with no credential, at every level.

    Pi resolves a literal secret wherever its schema allows one: a provider's own ``apiKey`` and
    ``headers``, a ``models`` entry's ``headers``, and a ``modelOverrides`` entry's ``headers`` (Pi's own
    reference says a header's values "resolve like apiKey"). A filter that looked only at the provider's
    own level would write a model-level ``Authorization`` header into the scratch ``models.json`` -- a
    second copy of a secret, in a home ``--keep`` prints and leaves on disk.
    """

    def test_nested_definitions_are_scrubbed_and_the_key_is_an_env_reference(self) -> None:
        host = Path(tempfile.mkdtemp(prefix="ccc-pi-seed-host-")).resolve()
        self.addCleanup(shutil.rmtree, host, ignore_errors=True)
        home = Path(tempfile.mkdtemp(prefix="ccc-pi-seed-home-")).resolve()
        self.addCleanup(shutil.rmtree, home, ignore_errors=True)
        (host / "models.json").write_text(json.dumps({"providers": {
            "probe": {
                "apiKey": "sk-literal-provider-secret",
                "baseUrl": "https://example.invalid",
                "headers": {"Authorization": "Bearer a-provider-header-secret"},
                "models": [{"id": "probe-model", "baseUrl": "https://example.invalid",
                            "headers": {"Authorization": "Bearer a-model-header-secret"}}],
                "modelOverrides": {"probe-model": {"headers": {"x-api-key": "an-override-secret"}}},
            },
            "unneeded": {"apiKey": "an-unneeded-secret"},
        }}), encoding="utf-8")

        seeded, left_out, refused = NATIVE_CHECK.seed_pi_home(home, host, ["probe"])

        self.assertEqual(seeded, ["probe"], seeded)
        self.assertEqual(left_out, ["unneeded"], left_out)
        self.assertEqual(refused, [], refused)
        document = json.loads((home / "models.json").read_text(encoding="utf-8"))
        probe = document["providers"]["probe"]
        self.assertEqual(probe["apiKey"], "$PROBE_API_KEY", probe)
        self.assertEqual(probe["models"],
                         [{"id": "probe-model", "baseUrl": "https://example.invalid"}], probe)
        self.assertEqual(probe["modelOverrides"], {"probe-model": {}}, probe)
        # A provider the run does not need is left out entirely: its definitions (and every credential
        # in them) never reach the scratch document.
        self.assertEqual(sorted(document["providers"]), ["probe"], document)
        for secret in ("sk-literal-provider-secret", "a-provider-header-secret",
                       "a-model-header-secret", "an-override-secret", "an-unneeded-secret"):
            self.assertNotIn(secret, json.dumps(document), secret)

    def test_a_key_reference_survives_and_a_header_reference_does_not(self) -> None:
        """The two fields' rules are different on purpose, and both are the shipped ones: the document's
        ``apiKey`` is re-spelled as the ``$ENV`` reference the seeded home resolves its credential from
        (dropping the host's literal or ``!command`` value either way), while a ``headers`` object is
        dropped at every level, reference or not, exactly as the provider's own ``headers`` have been."""
        host = Path(tempfile.mkdtemp(prefix="ccc-pi-seed-host-")).resolve()
        self.addCleanup(shutil.rmtree, host, ignore_errors=True)
        home = Path(tempfile.mkdtemp(prefix="ccc-pi-seed-home-")).resolve()
        self.addCleanup(shutil.rmtree, home, ignore_errors=True)
        (host / "models.json").write_text(json.dumps({"providers": {"probe": {
            "apiKey": "$HOST_NAMED_KEY",
            "headers": {"Authorization": "$A_HOST_HEADER_REFERENCE"},
            "models": [{"id": "probe-model", "headers": {"Authorization": "$HOST_NAMED_HEADER"}}],
        }}}), encoding="utf-8")

        NATIVE_CHECK.seed_pi_home(home, host, ["probe"])

        document = json.loads((home / "models.json").read_text(encoding="utf-8"))
        probe = document["providers"]["probe"]
        self.assertEqual(probe["apiKey"], "$HOST_NAMED_KEY", document)
        self.assertNotIn("headers", probe, document)
        self.assertEqual(probe["models"], [{"id": "probe-model"}], document)
        for reference in ("A_HOST_HEADER_REFERENCE", "HOST_NAMED_HEADER"):
            self.assertNotIn(reference, json.dumps(document), reference)


class SeedPiHomeRefusesACredentialBearingEndpoint(unittest.TestCase):
    """A provider whose endpoint URL carries a credential is not copied, at any depth.

    The scrubber drops a definition's credential *fields*; an endpoint is not one -- it is the address the
    harness itself would call -- so rewriting it would send the run somewhere the host did not name, and
    copying it as it stands would write a second copy of a secret into the scratch ``models.json`` (and
    into the directory ``--keep`` prints and leaves on disk). The provider is left out whole, the same
    policy the Codex checker applies to a provider's ``base_url``, and the run's own report names it.
    """

    SHAPES = {
        "userinfo": "https://user:sk-endpoint-secret@example.invalid/v1",
        "query": "https://example.invalid/v1?token=sk-endpoint-secret",
        "fragment": "https://example.invalid/v1#token=sk-endpoint-secret",
    }

    def seed(self, provider: dict) -> tuple[list[str], list[str], list[str], Path]:
        host = Path(tempfile.mkdtemp(prefix="ccc-pi-endpoint-host-")).resolve()
        self.addCleanup(shutil.rmtree, host, ignore_errors=True)
        home = Path(tempfile.mkdtemp(prefix="ccc-pi-endpoint-home-")).resolve()
        self.addCleanup(shutil.rmtree, home, ignore_errors=True)
        (host / "models.json").write_text(json.dumps({"providers": {"probe": provider}}),
                                          encoding="utf-8")
        seeded, left_out, refused = NATIVE_CHECK.seed_pi_home(home, host, ["probe"])
        return seeded, left_out, refused, home

    def test_every_url_shape_and_level_is_refused_whole(self) -> None:
        # Every branch of `credential_free_url` (userinfo, query, fragment) at both levels a definition
        # can name an endpoint at, because either half alone would stay green with the other removed.
        for shape, url in self.SHAPES.items():
            for level in ("the provider's own baseUrl", "a models entry's baseUrl"):
                with self.subTest(credential_in=shape, at=level):
                    provider = {"baseUrl": "https://example.invalid", "apiKey": "$PROBE_API_KEY",
                                "models": [{"id": "probe-model"}]}
                    if level == "the provider's own baseUrl":
                        provider["baseUrl"] = url
                    else:
                        provider["models"] = [{"id": "probe-model", "baseUrl": url}]
                    seeded, left_out, refused, home = self.seed(provider)
                    self.assertEqual(seeded, [], f"a provider holding a credential was copied: {seeded}")
                    self.assertEqual(left_out, [], left_out)
                    self.assertEqual(refused, ["probe"], refused)
                    self.assertFalse((home / "models.json").exists(),
                                     "the refused provider still reached a seeded models.json")
                    self.assertNotIn("sk-endpoint-secret", json.dumps(seeded), seeded)

    def test_a_credential_free_endpoint_is_copied(self) -> None:
        """The other side of the rule, so the cases above cannot pass by refusing every provider: an
        endpoint with no userinfo, query or fragment is copied as it stands."""
        seeded, left_out, refused, home = self.seed({"baseUrl": "https://example.invalid/v1",
                                                     "apiKey": "$PROBE_API_KEY"})
        self.assertEqual(seeded, ["probe"], seeded)
        self.assertEqual((left_out, refused), ([], []), (left_out, refused))
        provider = json.loads((home / "models.json").read_text(encoding="utf-8"))["providers"]["probe"]
        self.assertEqual(provider["baseUrl"], "https://example.invalid/v1", provider)


class ModelVisibleHistoryWalk(unittest.TestCase):
    """``model_visible_roles`` walks the file's own ``parentId`` chain, not its file order.

    Measured on the harness's own loader: it starts at the file's last entry, follows ``parentId`` back to
    the root and rebuilds the reversed path, so an entry no later record hangs off is recorded history and
    not context. The flat reading -- take the latest compaction in file order, then every message after it,
    off the path or not -- is only accidentally right for a single-chain file, and both halves of the walk
    are pinned here: a compaction off the last entry's path must not report a ``compactionSummary``, and a
    branch hanging off a path entry must not be folded into the context.
    """

    def test_a_compaction_and_a_branch_off_the_last_entrys_path_are_not_context(self) -> None:
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-walk-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        session = scratch / "session.jsonl"
        # The last entry's path is root -> user -> assistant -> last: the compaction entry, the turn after
        # it and the branch entry all hang off that path's assistant, so no later record names them.
        session.write_text("\n".join([
            json.dumps({"type": "session", "id": "header"}),
            json.dumps({"type": "message", "id": "u", "parentId": None,
                        "message": {"role": "user"}}),
            json.dumps({"type": "message", "id": "a1", "parentId": "u",
                        "message": {"role": "assistant"}}),
            json.dumps({"type": "compaction", "id": "c", "parentId": "a1"}),
            json.dumps({"type": "message", "id": "u2", "parentId": "c",
                        "message": {"role": "user"}}),
            json.dumps({"type": "message", "id": "a2", "parentId": "u2",
                        "message": {"role": "assistant"}}),
            json.dumps({"type": "message", "id": "branch", "parentId": "a1",
                        "message": {"role": "assistant"}}),
            json.dumps({"type": "message", "id": "last", "parentId": "a1",
                        "message": {"role": "assistant"}}),
        ]) + "\n", encoding="utf-8")
        self.assertEqual(NATIVE_CHECK.model_visible_roles(session), ["user", "assistant", "assistant"])

    def test_a_compaction_on_the_path_is_still_a_boundary(self) -> None:
        # The other half, so the walk is not merely "never report a compaction": when the last entry does
        # hang off the compaction, the summary is reported and only the messages after it on the path.
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-walk-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        session = scratch / "session.jsonl"
        session.write_text("\n".join([
            json.dumps({"type": "session", "id": "header"}),
            json.dumps({"type": "message", "id": "u", "parentId": None,
                        "message": {"role": "user"}}),
            json.dumps({"type": "message", "id": "a1", "parentId": "u",
                        "message": {"role": "assistant"}}),
            json.dumps({"type": "compaction", "id": "c", "parentId": "a1"}),
            json.dumps({"type": "message", "id": "u2", "parentId": "c",
                        "message": {"role": "user"}}),
            json.dumps({"type": "message", "id": "a2", "parentId": "u2",
                        "message": {"role": "assistant"}}),
        ]) + "\n", encoding="utf-8")
        self.assertEqual(NATIVE_CHECK.model_visible_roles(session),
                         ["compactionSummary", "user", "assistant"])


class NativeCheckDriver(unittest.TestCase):
    """One run of the shipped ``main`` against stubs, shared by the cases below.

    The native half cannot run offline -- it needs the target harness installed and, for a turn, model
    access -- so a case drives ``main`` with the seams that half stands on replaced: the subprocess
    runner and ``shutil.which`` for the tools. The stub for ``ccc continue`` answers a conversion
    document whose target session this case can choose to have installed or not, which is what lets a run
    be driven to its route checks and verdict without a target harness. This class holds no ``test_``
    method of its own -- it is the driver -- so it adds no case to the suite.
    """

    def drive(self, direction: str = "claude-to-pi", *, claude_on_path: bool = False,
              write_installed: bool = True, pi_provider: str | None = "probe",
              load_state: str = "verified", claude: str | None = None,
              pi: str | None = None, pi_on_path: bool = True, give_home: bool = True,
              host_models: dict | None = None,
              pi_loader_answer: tuple[str, str] | None = None
              ) -> tuple[int, dict[str, tuple[str, str]], str, dict]:
        """One run of the shipped ``main`` against stubs; answers its exit code, its recorded checks, its
        printed output and the facts the stub captured (the conversion's ``--to`` target, the
        ``--target-binary`` it was given, whether the target home held a seeded ``models.json`` while the
        stub ran, among them).

        ``pi_provider`` is what ``$CCC_PI_NATIVE_PROVIDER`` holds for the run (``None`` sets it empty, so
        the run takes the "no provider configured" path); ``load_state`` is what the stub's conversion
        report says its load verification answered; ``claude`` names the binary for the pi-to-claude
        direction, which the ``--claude`` flag carries; ``pi`` is what the ``--pi`` flag names (the
        default is left off the command line, which is the shipped default ``pi``), and ``pi_on_path``
        says whether the executable lookup answers it -- ``False`` is the host without the harness.
        ``give_home`` says whether the run is handed ``--target-home`` (``False`` leaves it the scratch
        home it makes itself); ``host_models`` is what the host agent directory's ``models.json`` holds,
        which is what the scratch home's seeding copies. ``pi_loader_answer`` is what the Pi loader seam
        answers when a case drives a Pi-target run far enough to reach it -- the default is to refuse,
        because a case that does not name it is not a case about the loader.
        """
        scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-native-check-case-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        repo = scratch / "repo"
        fixture_root = repo / NATIVE_CHECK.FIXTURE_ROOT
        claude_fixture = (fixture_root / "claude" / "-fake-project"
                          / f"{NATIVE_CHECK.SOURCE_SESSION}.jsonl")
        claude_fixture.parent.mkdir(parents=True)
        claude_fixture.write_text(json.dumps({"type": "user", "cwd": "/home/user/code/example-api",
                                              "message": {"role": "user", "content": "hello"}}) + "\n")
        pi_fixture = fixture_root / "pi" / f"{NATIVE_CHECK.PI_BACK_SOURCE_SESSION}.jsonl"
        pi_fixture.parent.mkdir(parents=True)
        pi_fixture.write_text(json.dumps({"type": "session",
                                          "id": NATIVE_CHECK.PI_BACK_SOURCE_SESSION,
                                          "cwd": "/home/user/code/example-api"}) + "\n")
        home = scratch / "target-home"
        home.mkdir()
        go = str(scratch / "go")
        claude_binary = str(scratch / "claude") if claude is None else claude
        thread_id = "01a1e000-0000-7000-8000-000000000002"
        captured: dict = {}
        if host_models is not None:
            host_agent_dir = scratch / "pi-agent"
            host_agent_dir.mkdir(parents=True, exist_ok=True)
            (host_agent_dir / "models.json").write_text(json.dumps({"providers": host_models}),
                                                        encoding="utf-8")

        # The Pi spelling this run resolves: the ``--pi`` flag's own value, or the shipped default. The
        # lookup is answered under that spelling so a case can read back which executable ``--pi``
        # produced -- a name the case spelled as a path is answered as that path, and a bare name as one
        # inside this case's scratch tree.
        pi_spelling = pi or "pi"

        def fake_which(name, *args, **kwargs):
            # The Claude Code binary may be named by the ``--claude`` flag, so the name can be a path;
            # its basename is what says it is the Claude binary this case stands up.
            spelled = str(name)
            if os.path.basename(spelled) == "claude":
                return claude_binary if claude_on_path else None
            if spelled == pi_spelling:
                captured.setdefault("which", {})[spelled] = None if not pi_on_path else (
                    spelled if spelled != Path(spelled).name else str(scratch / spelled))
                return captured["which"][spelled]
            return str(scratch / spelled)

        def fake_ask_pi_loader(*args, **kwargs):
            if pi_loader_answer is None:
                raise AssertionError("this case's run reached the Pi loader seam it did not stub")
            return pi_loader_answer

        def fake_run(cmd, cwd=None, env=None, timeout=600):
            command = [str(part) for part in cmd]
            if command[0] == go:
                captured["build"] = command
                return subprocess.CompletedProcess(cmd, 0, "", "")
            if "--version" in command:
                # The one version query a run makes is the Pi binary's own, and the executable it names is
                # what a case reads back: a case that names one with `--pi` must reach its own assertions,
                # not the stub's refusal, and the query is one of the stages `--pi` has to reach.
                captured["version_query"] = command
                return subprocess.CompletedProcess(cmd, 0, "1.5.2\n", "")
            if command[0] == str(repo / NATIVE_CHECK.BINARY):
                env = env or {}
                to_provider = command[command.index("--to") + 1]
                captured["to"] = to_provider
                captured["target_binary"] = command[command.index("--target-binary") + 1]
                target_home = Path(command[command.index("--target-home") + 1])
                captured["target_home"] = str(target_home)
                # What the target home held **while the conversion ran**: a seeded `models.json` is the
                # only way a real turn reaches a provider here, and whether one is there is what tells a
                # seeded scratch home from a caller's home this harness must not write to.
                captured["home_models"] = (target_home / "models.json").is_file()
                if captured["home_models"]:
                    captured["home_models_seeded"] = json.loads(
                        (target_home / "models.json").read_text(encoding="utf-8")).get("providers")
                source_root = Path(env.get("CLAUDE_PROJECTS_DIR") or env.get("PI_DIR"))
                cwd_path = source_root.parent / NATIVE_CHECK.WORKSPACE
                installed = target_home / "installed" / f"{thread_id}.jsonl"
                if to_provider == "claude":
                    home_env = "CLAUDE_CONFIG_DIR"
                    session_dir_env = None
                    session_dir = None
                    resume_arguments = ["claude", "--resume", thread_id]
                    exec_arguments = ["claude", "-p", "--output-format", "json", "--resume", thread_id,
                                      "--", NATIVE_CHECK.PROMPT_PLACEHOLDER]
                else:
                    home_env = PI_HOME_ENV
                    session_dir_env = NATIVE_CHECK.PI_SESSION_DIR_ENV
                    session_dir = NATIVE_CHECK.pi_session_tree(target_home, cwd_path)
                    resume_arguments = ["pi", "--session", thread_id]
                    exec_arguments = ["pi", "-p", "--session", thread_id, "--",
                                      NATIVE_CHECK.PROMPT_PLACEHOLDER]
                if write_installed:
                    installed.parent.mkdir(parents=True, exist_ok=True)
                    installed.write_text(json.dumps({"type": "user"}) + "\n")
                document = {
                    "conversion": {
                        "state": "converted",
                        "carried": {"messages": 3, "tool_calls": 1, "tool_results": 1},
                        "losses": [{"category": "message_model_attribution", "disposition": "omitted",
                                    "count": 1, "detail": "the stub's one loss"}],
                    },
                    "installation": {"state": "installed", "path": str(installed),
                                     "mechanism": "hard-link"},
                    "target_session": {
                        "session_id": thread_id, "path": str(installed),
                        "resume_arguments": resume_arguments,
                        "exec_resume_arguments": exec_arguments,
                        "resume_command": invocation(cwd_path, target_home, home_env, resume_arguments,
                                                     session_dir_env, session_dir),
                        "exec_resume_command": invocation(cwd_path, target_home, home_env,
                                                          exec_arguments, session_dir_env,
                                                          session_dir),
                    },
                    "verification": {
                        "structure": {"state": "verified", "method": "the stub's read-back"},
                        "load": {"state": load_state, "detail": "the stub's own loader answered"},
                        "resume": {"state": "not-requested"},
                    },
                }
                return subprocess.CompletedProcess(cmd, 0, json.dumps(document), "")
            raise AssertionError(f"the case's stub was asked for an unexpected command: {command}")

        before = len(NATIVE_CHECK.RESULTS)
        printed = io.StringIO()
        argv = ["--repo", str(repo), "--direction", direction, "--pi-dir", str(scratch / "pi-agent")]
        if give_home:
            argv += ["--target-home", str(home)]
        if claude is not None:
            argv += ["--claude", claude]
        if pi is not None:
            argv += ["--pi", pi]
        try:
            with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                    mock.patch.object(NATIVE_CHECK, "ask_pi_loader", fake_ask_pi_loader), \
                    mock.patch.dict(os.environ, {"CCC_PI_NATIVE_PROVIDER": pi_provider or ""}), \
                    mock.patch("shutil.which", fake_which), \
                    contextlib.redirect_stdout(printed):
                code = NATIVE_CHECK.main(argv)
        finally:
            results = {name: (state, detail)
                       for name, state, detail in NATIVE_CHECK.RESULTS[before:]}
            del NATIVE_CHECK.RESULTS[before:]
        return code, results, printed.getvalue(), captured


class DirectionSelection(NativeCheckDriver):
    """``--direction`` selects the source provider, the source session and the target provider.

    The two directions are the two routes this script verifies, and each names a different committed
    fixture and a different target; the header and the ``--to`` the conversion is asked for are where a
    run says which one it is.
    """

    def test_claude_to_pi_converts_the_claude_fixture_into_a_pi_session(self) -> None:
        _, _, printed, captured = self.drive("claude-to-pi", write_installed=False)
        self.assertIn(f"direction:   claude-to-pi (claude:{NATIVE_CHECK.SOURCE_SESSION} → pi)",
                      printed, printed)
        self.assertEqual(captured.get("to"), "pi", captured)

    def test_pi_to_claude_converts_the_pi_fixture_into_a_claude_session(self) -> None:
        _, _, printed, captured = self.drive("pi-to-claude", write_installed=False)
        self.assertIn(f"direction:   pi-to-claude (pi:{NATIVE_CHECK.PI_BACK_SOURCE_SESSION} → claude)",
                      printed, printed)
        self.assertEqual(captured.get("to"), "claude", captured)

    def test_a_pi_to_claude_run_does_not_name_the_pi_project_directory(self) -> None:
        # The installed path in this direction is the target's own spelling (Claude's), so naming Pi's
        # project directory for it would name a directory that does not exist.
        _, _, printed, _ = self.drive("pi-to-claude", write_installed=False)
        header = printed.split("target home:", 1)[0]
        self.assertIn("workspace:", header, printed)
        self.assertNotIn("project dir", header, printed)


class IncompleteWhenTheTargetHarnessIsAbsent(NativeCheckDriver):
    """A run whose one failure is the absent target harness reports ``INCOMPLETE``, not a pass or a
    defect.

    Every check this script can run without the target harness's own loader passed, and the one check that
    needs it says why it could not run: that is an INCOMPLETE run, named with what could not run, and it
    exits 2 -- the distinction a reader of the verdict must not take for either a pass or a failure.
    """

    def test_reports_incomplete_naming_what_could_not_run(self) -> None:
        code, results, printed, _ = self.drive("pi-to-claude", claude_on_path=False)
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, [MISSING_TARGET_CHECK],
                         f"the absent harness was not the run's only failure: {results}")
        self.assertIn("RESULT: INCOMPLETE", printed, printed)
        self.assertIn("claude is not on PATH", printed, printed)
        self.assertIn("native half of pi-to-claude could not run", printed, printed)
        self.assertEqual(code, 2, f"exit {code} for an INCOMPLETE run: {results}")

    def test_the_other_checks_pass_so_incomplete_is_not_hiding_a_defect(self) -> None:
        # A run that failed some other check would print FAIL, not INCOMPLETE: the verdict is only
        # reachable when the absent harness is the sole failure, so the same drive with the harness on
        # PATH runs every check that arm can and fails none.
        code, results, printed, _ = self.drive("pi-to-claude", claude_on_path=True)
        failed = [name for name, (state, _) in results.items() if state == "FAIL"]
        self.assertEqual(failed, [], f"a run with the harness present failed a check: {results}")
        self.assertEqual(code, 0, f"exit {code} for a run whose checks all passed: {results}")
        self.assertNotIn("INCOMPLETE", printed, printed)

    def test_the_non_pi_direction_records_the_load_state(self) -> None:
        # The loader's own answer has to appear for a non-Pi target too, so a PASS rests on it rather
        # than only on the CLI's exit code.
        _, results, _, _ = self.drive("pi-to-claude", claude_on_path=True)
        state = results.get("the load check verified the installed session natively")
        self.assertIsNotNone(state, f"the non-Pi load check was not recorded: {results}")
        self.assertEqual(state[0], "PASS", results)

    def test_a_non_verified_load_state_fails(self) -> None:
        code, results, printed, _ = self.drive("pi-to-claude", claude_on_path=True,
                                               load_state="not-run")
        self.assertEqual(results.get("the load check verified the installed session natively", (None,))[0],
                         "FAIL", results)
        self.assertIn("RESULT: FAIL", printed, printed)
        self.assertEqual(code, 1, results)


class NoProviderConfiguredIsANamedSkip(NativeCheckDriver):
    """A host with no provider configured is reported as a named SKIP, not a crash and not a pass.

    The docstring's promise is the check's own words: the run reaches its ``RESULT:`` line (the gate's
    offline run has no provider configured) and the check that needs one says so, so the run does not
    traceback out of ``models.json``.
    """

    def test_a_host_with_no_provider_records_a_named_skip(self) -> None:
        # The loader seam answers so the run reaches its verdict: the skip is what this case is about,
        # and a run that stopped before its RESULT line would not show that the skip did not stop it.
        loader_answer = "\n".join([
            json.dumps({"type": "response", "command": "get_state", "success": True,
                        "data": {"sessionId": "the-stub", "messageCount": 0}}),
            json.dumps({"type": "response", "command": "get_messages", "data": {"messages": []}}),
        ]) + "\n"
        # `give_home=False` is what puts this case on the seeding path: a run that makes its own scratch
        # home seeds it from the host's configuration, and *that* is the path whose skip names the host's
        # models.json and $CCC_PI_NATIVE_PROVIDER (the caller's-home path has its own case below).
        _, results, printed, _ = self.drive("claude-to-pi", pi_provider=None, give_home=False,
                                            pi_loader_answer=(loader_answer, ""))
        state = results.get("a Pi provider is configured for the chosen model")
        self.assertIsNotNone(state, f"no skip was recorded for a host without a provider: {results}")
        self.assertEqual(state[0], "SKIP", results)
        self.assertIn("no provider configured", state[1], state[1])
        self.assertIn("RESULT:", printed, printed)


class ClaudeBinaryFlag(NativeCheckDriver):
    """``--claude PATH`` names the Claude Code binary for the pi-to-claude direction.

    The flag is the sibling's own spelling, and it does what the sibling's does: the named path is printed
    in the header and handed to the CLI as ``--target-binary``, so a Claude Code binary that is not on
    ``PATH`` can be named directly instead of by a ``PATH`` prefix.
    """

    def test_the_claude_flag_names_the_target_binary_and_is_printed(self) -> None:
        _, _, printed, captured = self.drive("pi-to-claude", claude_on_path=True,
                                             claude="/opt/named/claude")
        self.assertIn("claude:      /opt/named/claude", printed, printed)
        self.assertEqual(captured.get("target_binary"), "/opt/named/claude", captured)


class PiBinaryFlag(NativeCheckDriver):
    """``--pi PATH`` names the executable the *conversion* is driven through, not only the one this
    harness queries.

    The resolved path is what the CLI is handed as ``--target-binary`` and what the version query names
    beside it, so a caller with Pi outside ``PATH`` -- or with a second installation on ``PATH`` -- has
    the installation they named verified instead of whichever one a bare name finds.
    """

    def test_the_pi_flag_resolves_the_executable_the_conversion_is_driven_through(self) -> None:
        for named in ("pi-named", "/opt/pi/bin/pi"):
            with self.subTest(named=named):
                _, _, printed, captured = self.drive("claude-to-pi", pi=named, write_installed=False)
                resolved = captured.get("which", {}).get(named)
                self.assertIsNotNone(resolved, captured)
                self.assertIn(f"pi:          {resolved}", printed, printed)
                self.assertEqual(captured.get("target_binary"), resolved,
                                 "the conversion was driven through a different executable than the one "
                                 "--pi resolved")
                self.assertEqual(captured.get("version_query", [None])[0], resolved,
                                 "the version query and the conversion named different installations")


class MissingPiEndsInIncomplete(NativeCheckDriver):
    """A host without Pi gets the script's own INCOMPLETE verdict, not a bare exit code.

    The Pi target is the claude-to-pi direction's target harness: with no binary there is no loader to
    drive and no version to record, so its native half cannot run, and that is the verdict the other
    native checks already report for an absent harness -- exit 2 with the reason on its own RESULT line.
    The reverse direction does not need Pi at all: its source is a committed fixture read by the inherited
    reader and its native half is Claude's loader, so a host with no Pi still verifies it.
    """

    def test_the_pi_target_direction_reports_incomplete(self) -> None:
        code, _, printed, _ = self.drive("claude-to-pi", pi_on_path=False)
        self.assertIn("RESULT: INCOMPLETE", printed, printed)
        self.assertIn("pi is not on PATH", printed, printed)
        self.assertIn("native half of claude-to-pi", printed, printed)
        self.assertEqual(code, 2, printed)

    def test_the_reverse_direction_does_not_need_a_pi_binary(self) -> None:
        code, results, printed, _ = self.drive("pi-to-claude", pi_on_path=False, claude_on_path=True)
        self.assertNotIn("INCOMPLETE", printed, printed)
        self.assertEqual(code, 0, f"exit {code} for a run whose checks all passed: {results}")


class TheRunsOwnHomeIsSeededAndACallersIsNot(NativeCheckDriver):
    """The scratch home a run makes is seeded; a home the caller names is only read.

    Seeding is what lets a real turn reach a model: the provider definitions are the host's own, with
    every credential-bearing field dropped and the key left as an ``$ENV`` reference. Writing that
    document into a home the caller named would replace whatever configuration the caller keeps there --
    provider definitions and credential references among it -- and the comparisons a run makes cover
    session files only, so nothing it reports would have seen the loss.
    """

    HOST_PROVIDERS = {
        "probe": {"apiKey": "$PROBE_API_KEY", "baseUrl": "https://example.invalid",
                  "headers": {"x-api-key": "a-host-secret"},
                  "models": [{"id": "probe-model", "baseUrl": "https://example.invalid",
                              "headers": {"Authorization": "Bearer a-model-secret"}}],
                  "modelOverrides": {"probe-model": {"headers": {"x-api-key": "an-override-secret"}}}},
        "unused": {"apiKey": "a-host-secret"},
    }

    def test_the_scratch_home_is_seeded_without_credentials(self) -> None:
        _, _, printed, captured = self.drive("claude-to-pi", write_installed=False, give_home=False,
                                             host_models=self.HOST_PROVIDERS)
        seeded = captured.get("home_models_seeded")
        self.assertIsNotNone(seeded, f"the run's own scratch home was not seeded: {captured}")
        self.assertEqual(sorted(seeded), ["probe"], seeded)
        self.assertEqual(seeded["probe"].get("apiKey"), "$PROBE_API_KEY", seeded)
        self.assertEqual(seeded["probe"].get("models"),
                         [{"id": "probe-model", "baseUrl": "https://example.invalid"}], seeded)
        self.assertEqual(seeded["probe"].get("modelOverrides"), {"probe-model": {}}, seeded)
        for secret in ("a-host-secret", "a-model-secret", "an-override-secret"):
            self.assertNotIn(secret, json.dumps(seeded), seeded)
        self.assertIn("seeded provider(s) probe", printed, printed)

    def test_a_provider_whose_endpoint_carries_a_credential_is_not_seeded(self) -> None:
        """A refused provider is a named skip, not a silent one: the run seeds nothing, writes nothing,
        and says which provider it left out and why, so a turn that cannot reach a model has a reason in
        the report rather than a failure the caller has to decode -- and the credential is not quoted."""
        _, results, printed, captured = self.drive(
            "claude-to-pi", write_installed=False, give_home=False, pi_provider="probe",
            host_models={"probe": {"apiKey": "$PROBE_API_KEY",
                                   "baseUrl": "https://probe-user:an-endpoint-secret@example.invalid/v1"}})
        self.assertFalse(captured.get("home_models"),
                         "a provider whose endpoint carries a credential was seeded into the scratch home")
        self.assertNotIn("an-endpoint-secret", printed, printed)
        self.assertIn("left out (their endpoint URL carries a credential): probe", printed, printed)
        state = results.get("a Pi provider is configured for the chosen model")
        self.assertIsNotNone(state, f"no provider check was recorded: {results}")
        self.assertEqual(state[0], "SKIP", results)
        self.assertIn("carrying a credential", state[1], state[1])
        self.assertIn("probe", state[1], state[1])

    def test_a_callers_home_is_never_written_to(self) -> None:
        _, results, printed, captured = self.drive("claude-to-pi", write_installed=False,
                                                   host_models=self.HOST_PROVIDERS)
        self.assertFalse(captured.get("home_models"),
                         "the run seeded models.json into a home the caller named")
        self.assertFalse(Path(captured["target_home"], "models.json").exists(),
                         "the run left a models.json in the caller's home")
        self.assertIn("nothing was seeded into it", printed, printed)
        state = results.get("a Pi provider is configured for the chosen model")
        self.assertIsNotNone(state, f"no provider check was recorded: {results}")
        self.assertEqual(state[0], "SKIP", results)
        self.assertIn("a home the caller named is never written to", state[1], state[1])


class LoaderCopyCarriesNoCredentials(unittest.TestCase):
    """The loader half's throw-away copy holds the session and none of the caller's configuration.

    A caller-named ``--target-home`` may hold literal API keys in its ``models.json`` (or authorization
    headers, in that document or in ``settings.json``), and copying either document into the loader home
    would copy a credential the conversion contract copies nowhere -- the original home staying unchanged
    does not make a second copy of a secret acceptable. Measured on 1.5.2: with the sessions tree named
    through ``PI_CODING_AGENT_SESSION_DIR``, the loader answers ``get_state``/``get_messages`` from a home
    holding the session alone, because an RPC run is started with ``--offline --no-tools``; nothing about
    the copy's configuration is needed for the question the check asks.
    """

    SESSION = "01a1e225-a986-7372-bf0f-2c51da684752"

    def setUp(self) -> None:
        self.home = Path(tempfile.mkdtemp(prefix="ccc-loader-case-home-"))
        self.addCleanup(shutil.rmtree, self.home, ignore_errors=True)
        self.workspace = Path("workspace")
        self.installed = (self.home / NATIVE_CHECK.PI_SESSION_SUBDIR
                          / NATIVE_CHECK.pi_project_dir(str(self.workspace))
                          / f"2026-01-01T00-00-00-000Z_{self.SESSION}.jsonl")
        self.installed.parent.mkdir(parents=True)
        self.installed.write_text(json.dumps({"type": "session", "version": 3,
                                              "id": self.SESSION}) + "\n", encoding="utf-8")
        (self.home / "models.json").write_text(json.dumps({"providers": {"probe": {
            "baseUrl": "https://example.invalid", "apiKey": "sk-literal-secret",
            "headers": {"Authorization": "Bearer header-secret"}}}}), encoding="utf-8")
        (self.home / "settings.json").write_text(json.dumps({"sessionDir": "/somewhere/else",
                                                            "apiKey": "sk-settings-secret"}),
                                                 encoding="utf-8")

    def test_the_loader_home_holds_the_session_copy_and_no_configuration(self) -> None:
        captured: dict = {}

        def fake_ask_pi_loader(pi, loader_home, cwd, session_id, timeout=90):
            loader_home = Path(loader_home)
            if session_id == self.SESSION:
                # The arm under test: the check's copy of the installed session. The control arm below
                # asks about an id nothing holds, from an empty home of its own.
                captured["home"] = loader_home
                captured["cwd"] = str(cwd)
                captured["files"] = {str(path.relative_to(loader_home)): path.read_text(encoding="utf-8")
                                     for path in loader_home.rglob("*") if path.is_file()}
                state = json.dumps({"type": "response", "command": "get_state", "success": True,
                                    "data": {"sessionId": self.SESSION, "messageCount": 1}})
                messages = json.dumps({"type": "response", "command": "get_messages", "success": True,
                                       "data": {"messages": []}})
                return state + "\n" + messages + "\n", ""
            return "", f"No session found matching '{session_id}'"

        before = len(NATIVE_CHECK.RESULTS)
        try:
            with mock.patch.object(NATIVE_CHECK, "ask_pi_loader", fake_ask_pi_loader), \
                    contextlib.redirect_stdout(io.StringIO()):
                NATIVE_CHECK._check_pi_loader("pi", self.home, self.workspace, self.SESSION,
                                              self.installed)
            states = {name: state for name, state, _ in NATIVE_CHECK.RESULTS[before:]}
        finally:
            del NATIVE_CHECK.RESULTS[before:]

        joined = json.dumps(captured.get("files", {}))
        for secret in ("sk-literal-secret", "header-secret", "sk-settings-secret"):
            self.assertNotIn(secret, joined,
                             f"the loader copy holds the caller's credential: {joined}")
        names = {PureWindowsPath(name).name for name in captured.get("files", {})}
        self.assertNotIn("models.json", names, f"the loader home holds the caller's models.json: {names}")
        self.assertNotIn("settings.json", names,
                         f"the loader home holds the caller's settings.json: {names}")
        # The session itself is what the copy is for, at the path the installed layout gives it.
        self.assertIn(str(self.installed.relative_to(self.home)), captured.get("files", {}))
        self.assertEqual(captured.get("cwd"), str(self.workspace))
        self.assertIn("the harness's own loader answers about the installed session", states)

    def test_the_loader_run_names_the_sessions_tree_the_printed_line_names(self) -> None:
        """The check runs the printed environment: the agent directory, and the sessions tree inside it.

        Pi reads ``PI_CODING_AGENT_SESSION_DIR`` ahead of the agent directory's own default, so a check
        that removed the variable would verify a lookup the printed command does not make whenever the
        caller's shell holds it, or the data directory's ``settings.json`` names another tree. The home is
        driven under this host's own flavour and under ``PureWindowsPath`` -- what ``Path`` is on Windows,
        whose ``str`` folds a rooted value to a backslash spelling -- and the expected value is the home
        the check was given, so the case pins the value itself on every host rather than one host's
        spelling of it. The expected sessions tree is the same ``pi_session_tree`` the printed line is
        built from, which is what makes the two the same directory rather than two spellings of one.
        """
        for home in (Path("/loader-home"), PureWindowsPath(r"C:\loader-home")):
            with self.subTest(home=str(home)):
                captured: dict = {}

                def fake_run(command, cwd=None, env=None, timeout=None, **kwargs):
                    captured["command"] = [str(part) for part in command]
                    captured["env"] = dict(env or {})
                    captured["cwd"] = str(cwd)
                    return subprocess.CompletedProcess(command, 0, "", "")

                with mock.patch.object(NATIVE_CHECK.subprocess, "run", fake_run):
                    NATIVE_CHECK.ask_pi_loader("pi", home, self.workspace, self.SESSION)
                self.assertEqual(captured["env"].get(PI_HOME_ENV), str(home))
                self.assertEqual(captured["env"].get(NATIVE_CHECK.PI_SESSION_DIR_ENV),
                                 NATIVE_CHECK.pi_session_tree(home, self.workspace))
                self.assertEqual(captured["cwd"], str(self.workspace))


class BuildOutputStaysOutOfTheRepositoryRoot(NativeCheckDriver):
    """The built binary lives under ``bin/``, which the ignore rules cover.

    A build output written into the repository root is a file that is neither carried nor
    project-owned, which is exactly what ``scripts/upstream-parity.py`` refuses; the sibling harnesses
    build under ``bin/`` and so does this one.
    """

    def test_the_binary_constant_is_a_bin_path(self) -> None:
        self.assertEqual(NATIVE_CHECK.BINARY, Path("bin", "ccc-native-check-pi"))

    def test_the_build_writes_into_bin(self) -> None:
        _, _, _, captured = self.drive("pi-to-claude", write_installed=False)
        build = captured.get("build")
        self.assertIsNotNone(build, f"the run did not build: {captured}")
        output = Path(build[build.index("-o") + 1])
        self.assertEqual(output.parent.name, "bin", build)
        self.assertEqual(output.name, "ccc-native-check-pi", build)


if __name__ == "__main__":
    unittest.main()
