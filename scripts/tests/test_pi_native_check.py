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


def invocation(cwd, home, home_env: str, arguments: list[str]) -> str:
    """One invocation as the shipped renderer spells it: the `cd`, then the assignment, then the words.

    ``internal/ccc/continue.go``'s ``renderInvocation`` starts with ``cd <shellQuote(workingDir)> &&`` and
    then ``renderResumeCommand``'s ``NAME=<shellQuote(value)>`` plus one ``shellQuote`` per argument: the
    assignment's *name* and its `=` stay bare -- quoting them would turn the assignment into a program's
    name to a shell -- while the value and every other word are single-quoted exactly where a POSIX shell
    would act on them, which is what ``shlex.quote`` does.
    """
    pieces = [shlex.quote(NATIVE_CHECK.CD_WORD), shlex.quote(str(cwd)), NATIVE_CHECK.OPERATOR_WORD,
              f"{home_env}={shlex.quote(str(home))}"]
    pieces += [shlex.quote(word) for word in arguments]
    return " ".join(pieces)


def invocation_states(target: dict, target_provider: str = "pi") -> dict[str, str]:
    """The check states ``_check_invocation`` records for ``target``, restored afterwards."""
    before = len(NATIVE_CHECK.RESULTS)
    with contextlib.redirect_stdout(io.StringIO()):
        NATIVE_CHECK._check_invocation(target, target_provider)
    states = {name: state for name, state, _ in NATIVE_CHECK.RESULTS[before:]}
    del NATIVE_CHECK.RESULTS[before:]
    return states


class ReportedInvocationReading(unittest.TestCase):
    """The printed resume commands are read the way a POSIX shell reads them.

    The rule is the sibling ``scripts/claude-native-check.py``'s: the words a shell splits the line into
    are one half, and the line's own spelling -- what a shell reads as *syntax* once the quoting is taken
    into account -- is the other. A check that applied only the word reading would accept an unquoted
    value a shell acts on, so the refusal cases below are the ones that make this rule answer for the Pi
    route and not only the Claude one.
    """

    def test_accepts_a_pi_rendering_the_renderer_emits(self) -> None:
        """Both printed lines read back as the `cd`, the assignment and the argument vector."""
        for cwd, home in RENDERINGS:
            for flavour in PATH_FLAVOURS:
                with self.subTest(cwd=cwd, home=home, flavour=flavour.__name__):
                    resume = invocation(flavour(cwd), flavour(home), PI_HOME_ENV, RESUME_ARGUMENTS)
                    exec_resume = invocation(flavour(cwd), flavour(home), PI_HOME_ENV,
                                             EXEC_RESUME_ARGUMENTS)
                    target = {"session_id": SESSION, "resume_command": resume,
                              "exec_resume_command": exec_resume}
                    states = invocation_states(target, "pi")
                    self.assertEqual(states.get(RESUME_CHECK), "PASS",
                                     f"a correct rendering was rejected: {resume!r} — {states}")
                    self.assertEqual(states.get(EXEC_RESUME_CHECK), "PASS",
                                     f"a correct rendering was rejected: {exec_resume!r} — {states}")
                    self.assertTrue(NATIVE_CHECK.spelled_as_invocation(resume, PI_HOME_ENV), resume)

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
                states = invocation_states({"session_id": SESSION, "resume_command": line,
                                            "exec_resume_command": line}, "pi")
                self.assertNotIn("PASS", states, f"{line!r} was accepted: {states}")

    def test_rejects_pi_syntax_the_value_leaves_unquoted(self) -> None:
        """A value the renderer quotes, spelled bare: the words read back right and the line is another
        command line -- `;` ends the assignment and starts a program, a bare `&` does the same, `$HOME` is
        substituted before the variable is set, and a backquote substitutes a command. The rule is about
        spellings: a character a shell acts on has to be quoted, which `shellQuote` does for every one."""
        for line in (
            f"cd workspace && {PI_HOME_ENV}=a;b {PI} --session {SESSION}",
            f"cd workspace && {PI_HOME_ENV}=a&b {PI} --session {SESSION}",
            f"cd workspace && {PI_HOME_ENV}=$HOME {PI} --session {SESSION}",
            f"cd workspace && {PI_HOME_ENV}=`id` {PI} --session {SESSION}",
            f"cd workspace && {PI_HOME_ENV}=~ {PI} --session {SESSION}",
        ):
            with self.subTest(line=line):
                self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
                states = invocation_states({"session_id": SESSION, "resume_command": line,
                                            "exec_resume_command": line}, "pi")
                self.assertNotIn("PASS", states, f"{line!r} was accepted: {states}")

    def test_rejects_a_pi_operator_glued_to_a_word(self) -> None:
        """``PI_CODING_AGENT_DIR=a&&b`` is one word to the shell's reading and two commands to a shell.

        A `&&` a shell reads as its operator is a word of its own, with whitespace on both sides; glued to
        a value it is part of that word, so the line sets the variable to `a` and then runs `b`.
        """
        line = f"cd workspace && {PI_HOME_ENV}=a&&b {PI} --session {SESSION}"
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states({"session_id": SESSION, "resume_command": line,
                                    "exec_resume_command": line}, "pi")
        self.assertNotIn("PASS", states, states)

    def test_rejects_a_quoted_operator_a_shell_would_read_as_a_word(self) -> None:
        """A quoted `&&` reads back as the same words and is not the operator.

        A shell reads ``'&&'`` as a word, so the `cd` is followed by an argument spelled `&&` and nothing
        separates the change from the assignment.
        """
        line = f"cd workspace '&&' {PI_HOME_ENV}=pi-home {PI} --session {SESSION}"
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states({"session_id": SESSION, "resume_command": line,
                                    "exec_resume_command": line}, "pi")
        self.assertNotIn("PASS", states, states)

    def test_rejects_a_pi_line_a_shell_cannot_read(self) -> None:
        # An unbalanced quote is not a command line at all: a shell would keep reading. The word reading
        # answers None, and the check records its refusal for that reason.
        line = f"cd 'workspace && {PI_HOME_ENV}=pi-home {PI} --session {SESSION}"
        self.assertIsNone(NATIVE_CHECK.shell_words(line))
        self.assertFalse(NATIVE_CHECK.spelled_as_invocation(line, PI_HOME_ENV), line)
        states = invocation_states({"session_id": SESSION, "resume_command": line,
                                    "exec_resume_command": line}, "pi")
        self.assertNotIn("PASS", states, states)


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
              write_installed: bool = True) -> tuple[int, dict[str, tuple[str, str]], str, dict]:
        """One run of the shipped ``main`` against stubs; answers its exit code, its recorded checks, its
        printed output and the facts the stub captured (the conversion's ``--to`` target among them)."""
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
        pi = str(scratch / "pi")
        go = str(scratch / "go")
        claude = str(scratch / "claude")
        thread_id = "01a1e000-0000-7000-8000-000000000002"
        captured: dict = {}

        def fake_which(name, *args, **kwargs):
            if name == "claude":
                return claude if claude_on_path else None
            return str(scratch / name)

        def fake_ask_pi_loader(*args, **kwargs):
            raise AssertionError("this case's run reached the Pi loader seam it did not stub")

        def fake_run(cmd, cwd=None, env=None, timeout=600):
            command = [str(part) for part in cmd]
            if command[0] == go:
                return subprocess.CompletedProcess(cmd, 0, "", "")
            if command[0] == pi and "--version" in command:
                return subprocess.CompletedProcess(cmd, 0, "1.5.2\n", "")
            if command[0] == str(repo / NATIVE_CHECK.BINARY):
                env = env or {}
                to_provider = command[command.index("--to") + 1]
                captured["to"] = to_provider
                target_home = Path(command[command.index("--target-home") + 1])
                source_root = Path(env.get("CLAUDE_PROJECTS_DIR") or env.get("PI_DIR"))
                cwd_path = source_root.parent / NATIVE_CHECK.WORKSPACE
                installed = target_home / "installed" / f"{thread_id}.jsonl"
                if to_provider == "claude":
                    home_env = "CLAUDE_CONFIG_DIR"
                    resume_arguments = ["claude", "--resume", thread_id]
                    exec_arguments = ["claude", "-p", "--output-format", "json", "--resume", thread_id,
                                      "--", NATIVE_CHECK.PROMPT_PLACEHOLDER]
                else:
                    home_env = PI_HOME_ENV
                    resume_arguments = ["pi", "--session", thread_id]
                    exec_arguments = ["pi", "--session", thread_id, "--",
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
                        "resume_command": invocation(cwd_path, target_home, home_env, resume_arguments),
                        "exec_resume_command": invocation(cwd_path, target_home, home_env,
                                                          exec_arguments),
                    },
                    "verification": {
                        "structure": {"state": "verified", "method": "the stub's read-back"},
                        "load": {"state": "not-run", "detail": "the stub ran no loader"},
                        "resume": {"state": "not-requested"},
                    },
                }
                return subprocess.CompletedProcess(cmd, 0, json.dumps(document), "")
            raise AssertionError(f"the case's stub was asked for an unexpected command: {command}")

        before = len(NATIVE_CHECK.RESULTS)
        printed = io.StringIO()
        argv = ["--repo", str(repo), "--target-home", str(home), "--direction", direction,
                "--pi-dir", str(scratch / "pi-agent")]
        try:
            with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                    mock.patch.object(NATIVE_CHECK, "ask_pi_loader", fake_ask_pi_loader), \
                    mock.patch.dict(os.environ, {"CCC_PI_NATIVE_PROVIDER": "probe"}), \
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


if __name__ == "__main__":
    unittest.main()
