#!/usr/bin/env python3
"""Regression tests for the checks in scripts/opencode-native-check.py that need no OpenCode binary.

The subjects are the rules that decide what this harness *records*, all driven in-process with the shipped
module imported by path, no subprocess and no tool on ``PATH``:

* the verdict a run with no harness reports (``INCOMPLETE``, exit 2, zero checks) -- this route's
  installation *is* the harness's own importer, so there is no offline half to claim without it;
* the spelling of one harness invocation: ``--standalone`` stands immediately after the subcommand, which
  is where the route's own invocation puts it. A run whose flag lands after the ``--`` terminator leaves
  the flag on the managed background service and hands it to the harness as part of the prompt -- the
  defect this case exists for: it answered from a store the check's own session was not in, so the turn
  "succeeded" while the installed session did not grow;
* the resume stage's own rule: one real turn is a *resumed session* only when the harness's own reader
  reads the store differently afterwards (a turn appends its own records), and the answer printed under
  ``native_check_common.TURN_OUTPUT_MARKER`` is the harness's own bytes, not a summary of them. The
  ``--expect`` text is optional: without it the harness's own printed answer is what the appended
  assistant evidence must hold, so a successful turn is verified rather than failed for a flag nobody
  passed;
* the environment every harness command is run with: the check's own dictionary *is* the command's whole
  environment (``replace_env``), so the database override it removes cannot be restored from this
  process's own environment underneath it -- asserted on the environment that reaches ``subprocess.run``,
  not on the dictionary the check built;
* the printed invocation's spelling: the line is read the way a POSIX shell reads it, so it is built with
  the quoting ``renderInvocation``/``shellQuote`` apply and driven under both path flavours
  (``PATH_FLAVOURS``). A hand-spelled line without that quoting held only where ``str(Path(...))`` has no
  backslash in it, which is what the ``windows-latest`` job alone measured;
* the reading of the harness's own answers: a refusal that does not name the id it refused is not the
  control arm, a listing the harness failed to answer is not an empty listing, and a document the harness
  did not parse is not a document;
* the planted session: the document carries the members the harness's own importer was measured to
  require, ``already exists`` is not a planting, and the removal is recorded as its own check -- only when
  this run planted something.
"""

from __future__ import annotations

import contextlib
import importlib
import importlib.util
import io
import json
import os
import shlex
import subprocess
import tempfile
import unittest
from pathlib import Path, PureWindowsPath
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]


def load_native_check():
    """The shipped harness, imported by path (its file name is not an identifier)."""
    spec = importlib.util.spec_from_file_location(
        "shipped_opencode_native_check", SCRIPTS / "opencode-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()

#: The shared runner the check calls. The environment cases read the environment that reaches its own
#: ``subprocess.run`` rather than the dictionary the check built: an overlay restores a removed name from
#: this process's environment, which is exactly the shape those cases exist for.
NATIVE_COMMON = importlib.import_module("native_check_common")

#: One session id, one unrelated id and one answer, as the cases spell them.
SESSION = "ses_installed000000000000000"
UNKNOWN = "ses_unknown_control_0000000000"
ANSWER = "TURN-ANSWER-OK"

#: The marker the resume stage prints the target's answer under: part of the reporting contract, because
#: ``scripts/tests/test_evidence_claims.py`` reads a recorded run's answer from it.
TURN_OUTPUT_MARKER = "the turn's own output (the target's answer, as the harness printed it):"


@contextlib.contextmanager
def recorded():
    """The module's own result list, restored afterwards: every case asserts on what a check recorded
    rather than on a copy of the rule."""
    saved = list(NATIVE_CHECK.RESULTS)
    NATIVE_CHECK.RESULTS.clear()
    try:
        yield NATIVE_CHECK.RESULTS
    finally:
        NATIVE_CHECK.RESULTS[:] = saved


def completed(stdout: str = "", stderr: str = "", returncode: int = 0):
    """One harness invocation's answer, as the module's own ``run`` seam answers it."""
    return mock.Mock(stdout=stdout, stderr=stderr, returncode=returncode)


def document(messages: int, session_id: str = SESSION, answer: str | None = None) -> dict:
    """One exported session document with `messages` messages: as much of the harness's answer as the
    checks read. When `answer` is given, the messages beyond the third one are the turn's own records --
    a user message carrying the prompt and an assistant message carrying the harness's answer -- which is
    the shape a real turn appends."""
    stored = [{"id": f"msg_{index}", "type": "user", "text": f"turn {index}"} for index in range(min(messages, 3))]
    for index in range(3, messages, 2):
        stored.append({"id": f"msg_{index}", "type": "user", "text": "a turn"})
        if index + 1 < messages:
            stored.append({"id": f"msg_{index + 1}", "type": "assistant", "agent": "build",
                           "model": {"id": "m", "providerID": "p"},
                           "content": [{"type": "text", "text": answer or ""}]})
    return {"info": {"id": session_id}, "messages": stored[:messages]}


#: The harness path the report's own invocations name, as the cases hand it to the conversion: one
#: absolute word, so the program word of a report's vector and the harness this check probed are the same
#: word (the shipped check reads them with ``Path``, so a case can drive either flavour).
OPENCODE = "/usr/local/bin/opencode"

#: The path flavours a case drives the check with: this host's own, and Windows' answer on any host. The
#: printed line is read the way a POSIX shell reads it, and a Windows-spelled path holds a backslash, which
#: an unquoted word hands to the shell as an escape -- so the line is only readable because
#: ``internal/ccc/continue.go``'s ``shellQuote`` quotes it, and a case that spelled the line by hand for
#: one host alone held only on that host (the shape the ``windows-latest`` job failed on).
PATH_FLAVOURS = (Path, PureWindowsPath)

#: One report target as the shipped checks read it: the two argument vectors the route prints (the plain
#: form with the empty prompt, the exec form with the placeholder), and the environment the printed line
#: sets -- this run's own data root and the store's own database pinned beside it. A case overrides one
#: member to drive the rule it is about.
def turn_target(flavour=Path, session_id: str = SESSION, *, host: str = OPENCODE,
                data_root: str = "/data", store: str = "/data/opencode/opencode.db") -> dict:
    environment = {NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV: str(flavour(data_root)),
                   NATIVE_CHECK.OPENCODE_DB_ENV: str(flavour(store))}
    return {
        "session_id": session_id,
        "resume_arguments": [host, "run", "--standalone", "--session", session_id, "--", ""],
        "exec_resume_arguments": [host, "run", "--standalone", "--session", session_id, "--",
                                  NATIVE_CHECK.PROMPT_PLACEHOLDER],
        "resume_environment": environment,
    }


def vector_check(arguments_field: str, prompt_name: str) -> str:
    """The record name the shipped check's own vector reading produces for one form, so a case asserts on
    the record rather than on a copy of its wording."""
    return (f"the report's {arguments_field} names the harness this run probed, this session and "
            f"{prompt_name}")


def invocation(workspace, environment: dict[str, str], arguments: list[str],
               whole_quoted: str | None = None) -> str:
    """One printed invocation as ``internal/ccc/continue.go``'s renderer spells it.

    ``renderInvocation`` starts with ``cd <shellQuote(workingDir)> &&``, then ``renderResumeCommand``
    spells each assignment as its bare ``NAME=`` beside a quoted value -- the names sorted -- and every
    argument quoted where a POSIX shell would act on it, which is what ``shlex.quote`` does. The cases
    build the report's own ``resume_command`` with this, so the line they hand the check is one the shipped
    renderer really prints rather than a hand-spelling that happens to read back on this host.

    ``whole_quoted`` names one variable whose *whole* assignment is spelled as a single quoted word
    (``'NAME=value'``) instead: a spelling the renderer never emits, whose words are the invocation's while
    a shell reads that word as the program to run. It is the negative arm the cases drive, not a spelling
    any route writes."""
    pieces = [shlex.quote(NATIVE_COMMON.CD_WORD), shlex.quote(str(workspace)),
              NATIVE_COMMON.OPERATOR_WORD]
    pieces += ["'" + f"{name}={value}" + "'" if name == whole_quoted
               else f"{name}={shlex.quote(value)}" for name, value in sorted(environment.items())]
    pieces += [shlex.quote(word) for word in arguments]
    return " ".join(pieces)


class HarnessInvocationSpelling(unittest.TestCase):
    """``--standalone`` stands after the subcommand: the spelling the route's own invocation uses."""

    def test_the_flag_stands_after_the_subcommand(self) -> None:
        self.assertEqual(
            NATIVE_CHECK.opencode_arguments(["run", "--session", SESSION, "--", "hello"]),
            ["run", "--standalone", "--session", SESSION, "--", "hello"])
        self.assertEqual(NATIVE_CHECK.opencode_arguments(["session", "export", SESSION]),
                         ["session", "--standalone", "export", SESSION])
        self.assertEqual(NATIVE_CHECK.opencode_arguments(["debug", "agents"]),
                         ["debug", "--standalone", "agents"])

    def test_the_turn_does_not_carry_the_flag_past_the_terminator(self) -> None:
        """The defect this rule exists for, driven through the turn the resume stage runs: the turn is
        the report's own exec-form vector with the prompt substituted for its placeholder, the flag stands
        on the subcommand's own side of the ``--`` terminator, and the vector named the session this
        check's own reader resolved."""
        calls: list[list[str]] = []

        def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
            calls.append(list(command))
            return completed(stdout=ANSWER)

        args = mock.Mock(resume_prompt="say hello", expect=None)
        target = turn_target()
        with mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run), \
             mock.patch.object(NATIVE_CHECK, "export_document",
                               side_effect=[(document(3), "ok", 0),
                                            (document(6, answer=ANSWER), "ok", 0)]):
            with recorded():
                NATIVE_CHECK._run_turn(args, target, OPENCODE, Path("/data"),
                                       Path("/data/opencode/opencode.db"), Path("/workspace"))
        self.assertEqual(len(calls), 1)
        turn = calls[0]
        self.assertEqual(turn, [OPENCODE, "run", "--standalone", "--session", SESSION, "--", "say hello"],
                         "the turn is not the reported invocation with the prompt substituted")
        self.assertLess(turn.index("--standalone"), turn.index("--"),
                        f"the flag is on the prompt's side of the terminator: {turn}")
        self.assertLess(turn.index("--standalone"), turn.index("--session"))

    def test_a_turn_whose_reported_invocation_is_not_this_turns_never_runs(self) -> None:
        """A report whose exec vector names another session is not this conversion's turn: the check
        records the failure and runs nothing, instead of running a corrected command of its own."""
        calls: list[list[str]] = []

        def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
            calls.append(list(command))
            return completed(stdout=ANSWER)

        target = turn_target(session_id=UNKNOWN)
        target["session_id"] = SESSION
        with mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run):
            with recorded() as results:
                NATIVE_CHECK._run_turn(mock.Mock(resume_prompt="a turn", expect=None), target, OPENCODE,
                                       Path("/data"), Path("/data/opencode/opencode.db"),
                                       Path("/workspace"))
                states = {name: state for name, state, _ in results}
        self.assertEqual(calls, [], "a turn ran although the reported invocation named another session")
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "FAIL")


class HarnessEnvironment(unittest.TestCase):
    """Every harness command this check runs gets the check's own complete environment: the data root
    named and (for the setup commands) the database override removed, so an inherited ``OPENCODE_DB``
    cannot send the command to a store this run did not choose -- and the Go side, which genuinely removes
    the name there, and the setup the check's own commands run against are then the same store. The resume
    turn is the exception, and only in shape: it runs the environment the *report* names, which pins the
    store's own database file (a printed line can set a variable but not remove one), so this check's
    case asserts the reported pin reaches the child over the caller's inherited value. What these cases
    read is the environment that reaches ``subprocess.run``, not the dictionary the check built: the
    pre-fix shape merged that dictionary onto this process's own environment, which restores the removed
    name.

    Each case drives both path flavours (``PATH_FLAVOURS``) and compares against ``str`` of the object the
    check was handed: the check writes that object's own spelling, and a spelled-out ``/data`` holds only
    on a host where a path written with a leading slash spells back the same -- not on Windows, where
    ``str(Path("/data"))`` is ``\\data``, the answer the ``windows-latest`` job gave this pair."""

    #: The data root and store handed to the check, as text: each case spells them with the path flavour it
    #: drives the check with, so no separator is hard-coded for one host.
    DATA_ROOT = "/data"
    STORE = "/data/opencode/opencode.db"

    def test_one_harness_command_removes_an_inherited_database_override(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                data_root = flavour(self.DATA_ROOT)
                captured: dict = {}

                def fake_subprocess_run(command, **kwargs):
                    captured["command"] = list(command)
                    captured["env"] = dict(kwargs.get("env") or {})
                    return completed(stdout="{}")

                with mock.patch.dict(os.environ, {"OPENCODE_DB": "/tmp/caller-elsewhere.db"}), \
                     mock.patch.object(NATIVE_COMMON.subprocess, "run", side_effect=fake_subprocess_run):
                    NATIVE_CHECK.ask_opencode("opencode", ["session", "export", SESSION], data_root)
                    # The removal is the child's: this process still carries the caller's own value.
                    self.assertEqual(os.environ.get("OPENCODE_DB"), "/tmp/caller-elsewhere.db")
                self.assertIn("env", captured, "no harness command reached subprocess.run")
                environment = captured["env"]
                self.assertNotIn("OPENCODE_DB", environment,
                                 "an inherited database override reached the harness command")
                self.assertEqual(environment.get(NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV), str(data_root),
                                 "the check's own data root did not reach the harness command")

    def test_the_resume_turn_runs_the_reported_store_over_an_inherited_override(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                data_root, store = flavour(self.DATA_ROOT), flavour(self.STORE)
                captured: list[dict[str, str]] = []

                def fake_subprocess_run(command, **kwargs):
                    captured.append(dict(kwargs.get("env") or {}))
                    return completed(stdout="{}")

                target = turn_target(flavour)
                with mock.patch.dict(os.environ, {"OPENCODE_DB": "/tmp/caller-elsewhere.db"}), \
                     mock.patch.object(NATIVE_COMMON.subprocess, "run", side_effect=fake_subprocess_run), \
                     mock.patch.object(NATIVE_CHECK, "export_document",
                                       side_effect=[(document(3), "ok", 0),
                                                    (document(6, answer=ANSWER), "ok", 0)]):
                    with recorded():
                        NATIVE_CHECK._run_turn(mock.Mock(resume_prompt="a turn", expect=ANSWER),
                                               target, OPENCODE, data_root, store, flavour("/workspace"))
                    # The override is the child's to drop: this process still carries the caller's own value.
                    self.assertEqual(os.environ.get("OPENCODE_DB"), "/tmp/caller-elsewhere.db")
                self.assertEqual(len(captured), 1, "the turn did not reach subprocess.run")
                environment = captured[0]
                self.assertEqual(environment.get(NATIVE_CHECK.OPENCODE_DB_ENV), str(store),
                                 "the turn did not run the store the report pinned")
                self.assertEqual(environment.get(NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV), str(data_root))


class PrintedInvocation(unittest.TestCase):
    """The printed line is read the way a POSIX shell reads it: the ``cd``, the data root, the store's own
    database pinned beside it, then the arguments -- and the reported ``resume_environment`` is the same
    pair. A line that names the root but not the store is another invocation: a caller's inherited
    ``OPENCODE_DB`` would decide which store it reads, and ``run --session <id>`` *creates* a session for
    an id the store does not hold (measured), so the pasted line could run in empty history under the
    converted identity instead of refusing it.

    Each case builds its line with ``invocation`` -- the quoting ``renderInvocation`` itself applies -- and
    drives every path flavour in ``PATH_FLAVOURS``, so the Windows spelling is exercised on this host too:
    the hand-spelling without that quoting read back as the invocation only where ``str(Path(...))`` holds
    no backslash, which is the ``windows-latest`` job's answer and not this one's."""

    #: The paths the report's own two halves name, as text: each case spells them with its path flavour, so
    #: no separator or drive letter is hard-coded for one host.
    DATA_ROOT = "/data"
    STORE = "/data/opencode/opencode.db"
    WORKSPACE = "/workspace"

    def target(self, flavour=Path, environment: dict | None = None) -> dict:
        # The report's own two halves: each argument vector the report prints begins with the program
        # word, and each command is that vector beside the environment assignments the renderer sorts.
        target = turn_target(flavour)
        if environment is not None:
            target["resume_environment"] = environment
        for field, arguments_field in (("resume_command", "resume_arguments"),
                                       ("exec_resume_command", "exec_resume_arguments")):
            target[field] = invocation(flavour(self.WORKSPACE), target["resume_environment"],
                                       target[arguments_field])
        return target

    def check(self, target: dict, flavour=Path) -> dict[str, str]:
        with recorded() as records:
            NATIVE_CHECK._check_invocation(target, OPENCODE, flavour(self.WORKSPACE),
                                           flavour(self.DATA_ROOT), flavour(self.STORE))
            return {name: state for name, state, _ in records}

    def test_a_line_pinning_the_store_reads_as_the_invocation(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                states = self.check(self.target(flavour), flavour)
                self.assertEqual(
                    states["the report names the environment the printed invocation sets"], "PASS")
                self.assertEqual(
                    states["the printed resume_command reads as a POSIX shell line naming the session"],
                    "PASS")
                self.assertEqual(
                    states["the printed exec_resume_command reads as a POSIX shell line naming the session"],
                    "PASS")
                self.assertEqual(states[vector_check("resume_arguments", "the empty prompt")], "PASS")
                self.assertEqual(
                    states[vector_check("exec_resume_arguments", f"the {NATIVE_CHECK.PROMPT_PLACEHOLDER!r} placeholder")],
                    "PASS")

    def test_a_line_naming_only_the_root_is_not_the_invocation(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                environment = {NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV: str(flavour(self.DATA_ROOT))}
                states = self.check(self.target(flavour, environment=environment), flavour)
                self.assertEqual(
                    states["the report names the environment the printed invocation sets"], "FAIL")
                self.assertEqual(
                    states["the printed resume_command reads as a POSIX shell line naming the session"],
                    "FAIL")

    def test_a_quoted_second_assignment_is_not_the_invocation(self) -> None:
        """The line's words are the invocation's to the last one, and the assignment the renderer writes
        *second* is spelled as one quoted word: a shell reads `'XDG_DATA_HOME=/data'` as the *program* to
        run, so the data root is never set and the harness opens the caller's own -- while the word vector,
        which throws the quoting away, reads back as the invocation. The spelling rule therefore requires
        every assignment the vector carries, and not only the one the line opens with (which is
        ``OPENCODE_DB`` here, the renderer's sorted names putting the database first)."""
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                target = self.target(flavour)
                environment = target["resume_environment"]
                for field, arguments_field in (("resume_command", "resume_arguments"),
                                               ("exec_resume_command", "exec_resume_arguments")):
                    target[field] = invocation(flavour(self.WORKSPACE), environment,
                                               target[arguments_field],
                                               whole_quoted=NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV)
                self.assertEqual(
                    NATIVE_COMMON.shell_words(target["resume_command"]),
                    [NATIVE_COMMON.CD_WORD, str(flavour(self.WORKSPACE)), NATIVE_COMMON.OPERATOR_WORD,
                     *[f"{name}={environment[name]}" for name in sorted(environment)],
                     *target["resume_arguments"]],
                    "the line's own words are not the invocation's, so the case would not be about the "
                    "spelling")
                states = self.check(target, flavour)
                self.assertEqual(
                    states["the printed resume_command reads as a POSIX shell line naming the session"],
                    "FAIL")
                self.assertEqual(
                    states["the printed exec_resume_command reads as a POSIX shell line naming the session"],
                    "FAIL")

    def test_a_pair_agreeing_with_itself_but_naming_another_session_is_not_this_turn(self) -> None:
        """The rule the review round's fifth finding is about: the line checks compare each rendered
        command with the report's own vector, so a consistently wrong pair -- another session, no
        ``--standalone``, the wrong prompt word -- can agree with itself. The vectors are read against this
        check's own harness and this conversion's session, so those pairs fail on the vector's own record
        while the line records still pass."""
        cases = (
            ("another session", lambda target: target.update({
                "resume_arguments": [OPENCODE, "run", "--standalone", "--session", UNKNOWN, "--", ""],
                "exec_resume_arguments": [OPENCODE, "run", "--standalone", "--session", UNKNOWN, "--",
                                          NATIVE_CHECK.PROMPT_PLACEHOLDER]})),
            ("no --standalone", lambda target: target.update({
                "resume_arguments": [OPENCODE, "run", "--session", SESSION, "--", ""],
                "exec_resume_arguments": [OPENCODE, "run", "--session", SESSION, "--",
                                          NATIVE_CHECK.PROMPT_PLACEHOLDER]})),
            ("the wrong prompt word", lambda target: target.update({
                "resume_arguments": [OPENCODE, "run", "--standalone", "--session", SESSION, "--", "hello"],
                "exec_resume_arguments": [OPENCODE, "run", "--standalone", "--session", SESSION, "--",
                                          "hello"]})),
        )
        for name, mutate in cases:
            with self.subTest(case=name):
                target = self.target(Path)
                mutate(target)
                for field, arguments_field in (("resume_command", "resume_arguments"),
                                               ("exec_resume_command", "exec_resume_arguments")):
                    target[field] = invocation(Path(self.WORKSPACE), target["resume_environment"],
                                               target[arguments_field])
                states = self.check(target, Path)
                # The line and the vector agree word for word; what fails is the vector's own reading.
                self.assertEqual(
                    states["the printed exec_resume_command reads as a POSIX shell line naming the session"],
                    "PASS")
                self.assertEqual(states[vector_check("exec_resume_arguments",
                                                     f"the {NATIVE_CHECK.PROMPT_PLACEHOLDER!r} placeholder")],
                                 "FAIL")


class ResumeStage(unittest.TestCase):
    """One real turn is a resumed session only when the harness's own reader reads the store differently
    afterwards, and the answer printed is the harness's own."""

    def drive(self, before: dict, after: dict, stdout: str = ANSWER,
              expect: str | None = ANSWER) -> list[tuple[str, str, str]]:
        args = mock.Mock(resume_prompt="a turn", expect=expect)
        target = turn_target()
        with mock.patch.object(NATIVE_CHECK, "run", return_value=completed(stdout=stdout)), \
             mock.patch.object(NATIVE_CHECK, "export_document",
                               side_effect=[(before, "ok", 0), (after, "ok", 0)]):
            with recorded() as results:
                NATIVE_CHECK._run_turn(args, target, OPENCODE, Path("/data"),
                                       Path("/data/opencode/opencode.db"), Path("/workspace"))
                return list(results)

    def test_a_grown_session_is_a_resumed_session(self) -> None:
        results = self.drive(document(3), document(6, answer=ANSWER))
        states = {name: state for name, state, _ in results}
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "PASS")
        self.assertEqual(states["the turn grew the installed session, through the harness's own reader"],
                         "PASS")
        self.assertEqual(states["the session's own records were not rewritten by the turn"], "PASS")
        self.assertEqual(states["the turn's own answer is in the session the harness reads"], "PASS")
        self.assertIn(f"the turn's answer contains '{ANSWER}'", states)

    def test_a_turn_without_expected_text_is_verified_by_its_own_answer(self) -> None:
        """``--expect`` is optional: without it the needle is the harness's own printed answer, so a turn
        that answered and appended its records is a resumed session rather than a failure for a flag the
        caller did not pass."""
        results = self.drive(document(3), document(6, answer=ANSWER), expect=None)
        states = {name: state for name, state, _ in results}
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "PASS")
        self.assertEqual(states["the turn's own answer is in the session the harness reads"], "PASS")
        self.assertNotIn(f"the turn's answer contains '{ANSWER}'", states,
                         "a run without --expect recorded an expected-substring check")

    def test_a_turn_whose_answer_is_not_in_the_session_is_not_a_resumed_session(self) -> None:
        results = self.drive(document(3), document(6, answer="some other words"), expect=None)
        states = {name: state for name, state, _ in results}
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "PASS")
        self.assertEqual(states["the turn's own answer is in the session the harness reads"], "FAIL",
                         "the printed answer was not compared with the appended assistant evidence")

    def test_a_turn_that_left_the_store_reading_the_same_is_not_a_resumed_session(self) -> None:
        results = self.drive(document(3), document(3))
        states = {name: state for name, state, _ in results}
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "PASS")
        self.assertEqual(states["the turn grew the installed session, through the harness's own reader"],
                         "FAIL")
        self.assertEqual(states["the turn's own answer is in the session the harness reads"], "FAIL")

    def test_the_answer_is_printed_under_the_marker(self) -> None:
        stdout = io.StringIO()
        with contextlib.redirect_stdout(stdout):
            self.drive(document(3), document(6))
        printed = stdout.getvalue()
        self.assertIn(TURN_OUTPUT_MARKER, printed)
        marker_line, _, block = printed.partition(TURN_OUTPUT_MARKER)
        self.assertIn(ANSWER, block, f"the answer is not printed under the marker: {printed!r}")

    def test_a_session_rewritten_by_the_turn_is_reported(self) -> None:
        """The turn's own records are appended; a harness that rewrote the conversation it was resumed
        from is the case the prefix rule is about."""
        before = document(3)
        after = document(6, answer=ANSWER)
        after["messages"][0] = {"id": "msg_0", "type": "user", "text": "edited"}
        results = self.drive(before, after)
        states = {name: state for name, state, _ in results}
        self.assertEqual(states["the session's own records were not rewritten by the turn"], "FAIL")
        self.assertEqual(states["the turn grew the installed session, through the harness's own reader"],
                         "PASS")


class HarnessAnswers(unittest.TestCase):
    """What the harness's own answers mean: a refusal names its id, a failed listing is not an empty one,
    and a document the harness did not parse is no document."""

    def test_export_answers_the_document_and_the_refusal(self) -> None:
        answered = completed(stdout=json.dumps(document(2)))
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=answered):
            exported, words, status = NATIVE_CHECK.export_document("opencode", Path("/data"), SESSION)
        self.assertEqual((exported or {}).get("info", {}).get("id"), SESSION)
        self.assertEqual(status, 0)
        self.assertEqual(json.loads(words)["info"]["id"], SESSION)

        refused = completed(stderr=f"Session not found: {UNKNOWN}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=refused):
            exported, words, status = NATIVE_CHECK.export_document("opencode", Path("/data"), UNKNOWN)
        self.assertIsNone(exported)
        self.assertEqual(status, 1)
        self.assertIn(UNKNOWN, words)

    def test_an_unparsed_answer_is_no_document(self) -> None:
        with mock.patch.object(NATIVE_CHECK, "ask_opencode",
                               return_value=completed(stdout="not a document")):
            exported, _, status = NATIVE_CHECK.export_document("opencode", Path("/data"), SESSION)
        self.assertIsNone(exported, f"exit {status} with an unparsed answer reported a document")

    def test_a_failed_listing_is_not_an_empty_one(self) -> None:
        with mock.patch.object(NATIVE_CHECK, "ask_opencode",
                               return_value=completed(stderr="boom", returncode=1)):
            listing, words = NATIVE_CHECK.session_list("opencode", Path("/data"), Path("/workspace"))
        self.assertIsNone(listing, f"a failed listing reported {listing!r}")
        self.assertIn("boom", words)

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=completed(stdout="[]")):
            listing, _ = NATIVE_CHECK.session_list("opencode", Path("/data"), Path("/workspace"))
        self.assertEqual(listing, [])

    def test_a_listing_that_is_not_an_array_is_no_listing(self) -> None:
        with mock.patch.object(NATIVE_CHECK, "ask_opencode",
                               return_value=completed(stdout=json.dumps({"sessions": []}))):
            listing, _ = NATIVE_CHECK.session_list("opencode", Path("/data"), Path("/workspace"))
        self.assertIsNone(listing)


class PlantedSession(unittest.TestCase):
    """The planted session is written through the harness's own importer and removed through its own
    delete, and both are recorded as checks."""

    def test_the_planted_document_carries_the_members_the_importer_requires(self) -> None:
        planted = NATIVE_CHECK.unrelated_document()
        self.assertEqual(sorted(planted["info"]),
                         ["cost", "id", "location", "projectID", "time", "tokens"])
        self.assertEqual(sorted(planted["info"]["tokens"]), ["cache", "input", "output", "reasoning"])
        self.assertEqual(sorted(planted["info"]["tokens"]["cache"]), ["read", "write"])
        self.assertIn("updated", planted["info"]["time"])
        self.assertEqual(planted["info"]["location"]["directory"],
                         str(NATIVE_CHECK.UNRELATED_DIRECTORY))
        message = planted["messages"][0]
        self.assertEqual(sorted(message), ["files", "id", "text", "time", "type"])
        self.assertEqual(message["type"], "user",
                         "an assistant message would need the members this check does not plant")

    def test_an_already_exists_answer_is_not_a_planting(self) -> None:
        """The harness answers ``Session already exists`` on stderr with exit 0: a run that read only the
        exit status would report a planting it did not make."""
        answered = completed(stderr="Session already exists\n")
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=answered):
            planted, detail = NATIVE_CHECK.plant_unrelated("opencode", Path("/data"), Path(scratch))
        self.assertFalse(planted, detail)

    def test_a_planting_names_the_id_the_harness_answered(self) -> None:
        answered = completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n")
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=answered):
            planted, detail = NATIVE_CHECK.plant_unrelated("opencode", Path("/data"), Path(scratch))
        self.assertTrue(planted, detail)

    def test_nothing_is_recorded_when_nothing_was_planted(self) -> None:
        with recorded() as results:
            NATIVE_CHECK.remove_planted("opencode", Path("/data"), planted=False)
            self.assertEqual(list(results), [])

    def test_the_removal_is_recorded_and_its_absence_is_not_a_pass(self) -> None:
        deleted = completed(stdout=f"Session {NATIVE_CHECK.UNRELATED_SESSION} deleted\n")
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[deleted, refused]):
            with recorded() as results:
                NATIVE_CHECK.remove_planted("opencode", Path("/data"), planted=True)
                states = [(name, state) for name, state, _ in results]
        self.assertEqual(states, [
            ("the planted session is removed through the harness's own session delete", "PASS"),
            ("the removed session no longer resolves", "PASS"),
        ])

    def test_a_delete_that_answered_not_found_is_reported(self) -> None:
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[refused, refused]):
            with recorded() as results:
                NATIVE_CHECK.remove_planted("opencode", Path("/data"), planted=True)
                states = {name: state for name, state, _ in results}
        self.assertEqual(states["the planted session is removed through the harness's own session delete"],
                         "FAIL")


class DataRootRule(unittest.TestCase):
    """A target root whose own name is ``opencode`` is the data directory itself: the harness's own rule
    (mirrored from ``internal/ccc/opencode_install.go``'s ``openCodeDataRoot``) makes its parent the
    environment root, so the check's harness commands, the conversion's ``--target-home`` and the store
    this run reads are one directory -- not ``<argument>/opencode`` beside ``<argument>``."""

    def test_a_directory_named_opencode_is_the_data_directory_itself(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                named = flavour("/data") / NATIVE_CHECK.OPENCODE_DATA_DIR
                self.assertEqual(NATIVE_CHECK.opencode_data_root_of(named), flavour("/data"),
                                 "a named data directory did not resolve to its parent root")
                self.assertEqual(NATIVE_CHECK.opencode_data_root_of(flavour("/data")), flavour("/data"),
                                 "a named root was rewritten")

    def test_the_store_either_spelling_computes_is_the_same_file(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                named_directory = flavour("/data") / NATIVE_CHECK.OPENCODE_DATA_DIR
                root = NATIVE_CHECK.opencode_data_root_of(named_directory)
                store = root / NATIVE_CHECK.OPENCODE_DATA_DIR / NATIVE_CHECK.OPENCODE_STORE
                self.assertEqual(store, named_directory / NATIVE_CHECK.OPENCODE_STORE)

    def test_the_conversion_is_given_the_callers_own_spelling(self) -> None:
        """The route applies the same rule once to the ``--target-home`` it is handed, so the conversion
        must be handed the caller's own spelling: this check's already-normalized root, when it itself
        names a data directory, would be normalized a second time by the route and resolve to another
        store than the one this check plants, converts and reads."""
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                named = (flavour("/data") / NATIVE_CHECK.OPENCODE_DATA_DIR
                         / NATIVE_CHECK.OPENCODE_DATA_DIR)
                home = NATIVE_CHECK.conversion_target_home(named, flavour("/scratch"))
                self.assertEqual(home, named, "the conversion was handed a normalized spelling")
                self.assertEqual(NATIVE_CHECK.opencode_data_root_of(home),
                                 flavour("/data") / NATIVE_CHECK.OPENCODE_DATA_DIR,
                                 "the two sides resolved different roots")
                self.assertEqual(NATIVE_CHECK.opencode_data_root_of(home) / NATIVE_CHECK.OPENCODE_DATA_DIR
                                 / NATIVE_CHECK.OPENCODE_STORE,
                                 named / NATIVE_CHECK.OPENCODE_STORE,
                                 "the two sides resolved different stores")
                self.assertEqual(NATIVE_CHECK.conversion_target_home(None, flavour("/scratch")),
                                 flavour("/scratch"),
                                 "a run with no named home did not hand the conversion its own root")


class ModelBinding(unittest.TestCase):
    """The offline half accepts a home that resolves no model -- the report's own ``model_provider_detail``
    is its statement of that answer -- while a requested turn needs a usable binding, exactly as the
    fourth measured case has it: a primary agent with ``model: null`` is a home the importer, the reader
    and the load level all work under, and an offline run must not end FAIL for it."""

    VERSION = "the report names the harness's own version"
    MODEL = "the report names the model a turn would run under, or says why the harness resolved none"

    def states(self, target: dict, turn_requested: bool = False) -> dict[str, str]:
        with recorded() as results:
            NATIVE_CHECK.record_model_binding(target, turn_requested)
            return {name: state for name, state, _ in results}

    def test_an_offline_run_accepts_the_harnesss_own_no_model_answer(self) -> None:
        target = {
            "cli_version": "2.2.5", "cli_version_source": "probe",
            "model_provider_source": "the harness's own debug agents answer",
            "model_provider_detail": "the harness's own agent listing names `build` as its primary agent "
                                     "and resolves no model for it, so this home names no model a new "
                                     "session of it would run under",
        }
        states = self.states(target)
        self.assertEqual(states[self.VERSION], "PASS")
        self.assertEqual(states[self.MODEL], "PASS")

    def test_an_offline_absence_without_the_harnesss_words_is_not_accepted(self) -> None:
        """A report that names no model and no reason has lost the answer it was supposed to carry."""
        states = self.states({"cli_version": "2.2.5"})
        self.assertEqual(states[self.MODEL], "FAIL")

    def test_a_requested_turn_needs_a_usable_binding(self) -> None:
        target = {"cli_version": "2.2.5",
                  "model_provider_detail": "… resolves no model for it"}
        self.assertEqual(self.states(target, turn_requested=True)[self.MODEL], "FAIL")

    def test_a_named_binding_passes_the_offline_and_the_turn_arm(self) -> None:
        target = {"cli_version": "2.2.5", "model": "target-model", "model_provider": "target-provider",
                  "model_provider_source": "the harness's own debug agents answer"}
        for turn_requested in (False, True):
            with self.subTest(turn_requested=turn_requested):
                self.assertEqual(self.states(target, turn_requested)[self.MODEL], "PASS")

    def test_a_missing_version_is_recorded_apart_from_the_model(self) -> None:
        states = self.states({"model": "m", "model_provider": "p"})
        self.assertEqual(states[self.VERSION], "FAIL")
        self.assertEqual(states[self.MODEL], "PASS")


class MissingHarness(unittest.TestCase):
    """A host with no harness is an INCOMPLETE run: this route's installation is that harness's own
    importer, so the run has nothing to verify and must not claim a pass."""

    def test_a_missing_harness_is_incomplete_and_runs_no_check(self) -> None:
        def which(name):
            return "/usr/bin/go" if name == "go" else None

        with mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which):
            with recorded() as results:
                stdout = io.StringIO()
                with contextlib.redirect_stdout(stdout):
                    code = NATIVE_CHECK.main(["--opencode", "opencode"])
                recorded_states = list(results)
        printed = stdout.getvalue()
        self.assertEqual(code, 2, printed)
        self.assertIn("RESULT: INCOMPLETE", printed)
        self.assertIn("0 check(s) ran", printed)
        self.assertIn("opencode", printed)
        self.assertEqual(recorded_states, [])

    def test_a_missing_go_toolchain_is_reported_without_a_verdict(self) -> None:
        def which(name):
            return None if name == "go" else "/usr/local/bin/opencode"

        with mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which), \
             contextlib.redirect_stderr(io.StringIO()) as stderr:
            code = NATIVE_CHECK.main(["--opencode", "opencode"])
        self.assertEqual(code, 2)
        self.assertIn("go is not on PATH", stderr.getvalue())


class HarnessExecutable(unittest.TestCase):
    """The harness word this run drives is resolved once, before any command runs, so no stage's own
    working directory decides which program a command names.

    ``shutil.which`` answers a spelling that names a path unchanged, so a relative ``--opencode`` stayed
    relative and the stages that run *from the session's own directory* -- the export the installation is
    read back through, its unknown-id control arm and the listing that must not name the session -- were
    evaluated there. Measured: ``FileNotFoundError`` out of ``main``, no cleanup, no ``RESULT`` line, the
    planted session left in a caller-named home. A case that read only a stage running from the caller's
    own directory would not see it, which is where the relative word means what the caller meant."""

    #: The caller's spelling, and the word it means: a path the lookup answers unchanged, made absolute
    #: against the directory it was resolved in.
    RELATIVE = "tools/opencode"

    def resolved(self) -> str:
        return str(Path(os.getcwd()) / "tools" / "opencode")

    def test_a_relative_harness_argument_is_absolute_before_any_command_runs(self) -> None:
        seen: list[tuple[str, Path | None]] = []
        commands: list[list[str]] = []

        def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
            commands.append(list(command))
            return completed(stdout="opencode v2.2.5")

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            seen.append((opencode, cwd))
            # The planting is where this case stops: an import that failed leaves a run that reports its
            # own failure and reaches its verdict, which is all this case needs of the run.
            return completed(stderr="the importer refused this case's planting\n", returncode=1)

        def which(name):
            return self.RELATIVE if name == self.RELATIVE else "/usr/local/bin/go"

        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which), \
             mock.patch.object(NATIVE_CHECK, "BINARY",
                               Path(scratch) / "bin" / "ccc-native-check-opencode"), \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask), \
             mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run):
            with recorded():
                stdout = io.StringIO()
                with contextlib.redirect_stdout(stdout):
                    code = NATIVE_CHECK.main(["--opencode", self.RELATIVE])
        printed = stdout.getvalue()
        self.assertIn("RESULT:", printed, f"the run printed no verdict: {printed[-400:]}")
        self.assertEqual(code, 1, printed)
        # The version probe is the first command a run makes through the harness word, and the planting is
        # the first stage handed one: neither carries the caller's relative spelling.
        self.assertEqual([command[0] for command in commands], [self.resolved()])
        self.assertEqual(seen, [(self.resolved(), None)],
                         "a stage ran the caller's relative spelling")

    def test_a_directory_changing_stage_is_handed_an_absolute_word(self) -> None:
        """The stage the defect was reached through: the harness's own reader, asked from the session's
        own working directory, whose command would be looked up in *that* directory rather than the
        caller's."""
        seen: list[tuple[list[str], Path | None]] = []

        def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
            seen.append((list(command), cwd))
            return completed(stdout=json.dumps(document(2)))

        with mock.patch.object(NATIVE_CHECK.shutil, "which", return_value=self.RELATIVE):
            opencode = NATIVE_CHECK.harness_executable(self.RELATIVE)
        workspace = Path(os.getcwd()) / "a-workspace-that-is-not-this-one"
        with mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run):
            NATIVE_CHECK.export_document(opencode, Path("/data"), SESSION, cwd=workspace)
        self.assertEqual(len(seen), 1, "the export did not reach a command")
        command, cwd = seen[0]
        self.assertEqual(Path(cwd), workspace, "the stage did not change the directory it runs in")
        self.assertEqual(Path(command[0]), Path(self.resolved()),
                         "the command a directory-changing stage runs names the spelling it was handed")
        self.assertTrue(os.path.isabs(command[0]), command)


class ToolEvidenceComparison(unittest.TestCase):
    """The store's tool part is compared with the *source fixture's* own bytes -- the committed Claude
    transcript the check converts -- and not with the conversion's report or its intended items: the
    call's identity, its recorded arguments and its recorded result text are each compared, because a
    writer that mangled the arguments or the output produces a report whose own items hold the same
    mangled bytes, and a comparison that read only the identity would pass it."""

    @classmethod
    def setUpClass(cls) -> None:
        candidates = sorted((SCRIPTS.parent / "internal/ccc/testdata/fixtures" / NATIVE_CHECK.SOURCE_PROVIDER)
                            .rglob(f"*{NATIVE_CHECK.SOURCE_SESSION}.jsonl"))
        cls.source = candidates[0]
        cls.expected = NATIVE_CHECK.source_tool_evidence(cls.source)

    def document(self, mutate=None) -> dict:
        """One exported document as the harness answers it, carrying what the fixture records: one tool
        part per recorded call, in the shape ``session export`` prints (``state.input`` and the completed
        state's text). ``mutate`` drives one member away from the fixture's own evidence."""
        parts = []
        for call_id, entry in self.expected.items():
            state: dict = {"status": entry["status"], "input": entry["input"]}
            if entry["status"] == "error":
                state["error"] = {"type": "tool", "message": entry["result"]}
            else:
                state["content"] = [{"type": "text", "text": entry["result"]}]
            parts.append({"type": "tool", "id": call_id, "name": entry["name"], "state": state})
        document = {"info": {"id": SESSION},
                    "messages": [{"id": "msg_1", "type": "assistant", "content": parts}]}
        if mutate is not None:
            mutate(document)
        return document

    def differences(self, mutate=None) -> list[str]:
        return NATIVE_CHECK.tool_evidence_differences(
            self.expected, NATIVE_CHECK.document_tool_evidence(self.document(mutate)))

    def part(self, document: dict) -> dict:
        return document["messages"][0]["content"][0]

    def test_the_fixture_reader_reads_the_recorded_call(self) -> None:
        """The independent side is the fixture's own bytes: its call id, name, arguments and result text
        are read from the transcript rather than from anything the conversion wrote."""
        self.assertEqual(len(self.expected), 1, "the committed fixture records one call")
        call_id, entry = next(iter(self.expected.items()))
        self.assertEqual(call_id, "toolu_01Example0000000000000000")
        self.assertEqual(entry["name"], "Read")
        self.assertEqual(entry["status"], "completed")
        self.assertEqual(entry["input"], {"file_path": "/home/user/code/example-api/internal/retry/policy.go"})
        self.assertEqual(entry["result"], "package retry\n\nconst MaxAttempts = 4\n")

    def test_the_fixtures_own_evidence_matches(self) -> None:
        self.assertEqual(self.differences(), [])

    def test_changed_arguments_are_a_difference(self) -> None:
        """The identity and the status stay right while the recorded input changes: the shape a writer
        that mangled the arguments produces, which an identity-only comparison passed."""
        def mutate(document: dict) -> None:
            self.part(document)["state"]["input"] = {"file_path": "/etc/passwd"}
        differences = self.differences(mutate)
        self.assertTrue(any("input" in difference for difference in differences), differences)

    def test_changed_result_text_is_a_difference(self) -> None:
        def mutate(document: dict) -> None:
            self.part(document)["state"]["content"][0]["text"] = "a different output"
        differences = self.differences(mutate)
        self.assertTrue(any("result text" in difference for difference in differences), differences)

    def test_changed_name_or_status_are_differences(self) -> None:
        def rename(document: dict) -> None:
            self.part(document)["name"] = "Bash"

        def restatus(document: dict) -> None:
            part = self.part(document)
            part["state"]["status"] = "error"
            part["state"].pop("content")
            part["state"]["error"] = {"type": "tool", "message": "x"}

        self.assertTrue(any("name" in difference for difference in self.differences(rename)))
        self.assertTrue(any("status" in difference for difference in self.differences(restatus)))

    def test_a_missing_or_extra_part_is_a_difference(self) -> None:
        def remove(document: dict) -> None:
            document["messages"][0]["content"] = []

        def duplicate(document: dict) -> None:
            content = document["messages"][0]["content"]
            content.append(dict(content[0]))

        def extra(document: dict) -> None:
            part = {"type": "tool", "id": "toolu_other", "name": "Read",
                    "state": {"status": "completed", "input": {},
                              "content": [{"type": "text", "text": "x"}]}}
            document["messages"][0]["content"].append(part)

        self.assertTrue(any("holds no part" in difference for difference in self.differences(remove)))
        self.assertTrue(any("more than once" in difference for difference in self.differences(duplicate)))
        self.assertTrue(any("records no carryable result" in difference
                            for difference in self.differences(extra)))

    def test_an_errored_result_is_expected_as_the_error_state(self) -> None:
        """``is_error`` is the reader's own ``errored`` status, which this target writes as the ``error``
        state: the fixture-driven expectation follows the flag rather than assuming every result completed.
        """
        with tempfile.TemporaryDirectory() as scratch:
            transcript = Path(scratch) / "session.jsonl"
            transcript.write_text("\n".join([
                json.dumps({"type": "assistant", "message": {"content": [
                    {"type": "tool_use", "id": "toolu_e", "name": "Bash", "input": {"command": "false"}}]}}),
                json.dumps({"type": "user", "message": {"content": [
                    {"type": "tool_result", "tool_use_id": "toolu_e", "content": "boom", "is_error": True}]}}),
            ]) + "\n", encoding="utf-8")
            expected = NATIVE_CHECK.source_tool_evidence(transcript)
        self.assertEqual(expected["toolu_e"]["status"], "error")
        self.assertEqual(expected["toolu_e"]["result"], "boom")


class CommandDeadlines(unittest.TestCase):
    """Every command this check runs goes through the one deadlined runner: a command that overruns its
    bound is recorded as a failed check of its own and answered as a failed exit status, so no timeout
    raises out of the run. Without that, a timeout would end the run before its cleanup and its verdict --
    the planted session would stay in a caller-named home, and its fixed id would make every later run's
    planting meet it."""

    def test_a_timed_out_command_is_recorded_and_answered(self) -> None:
        cmd = ["opencode", "session", "export", SESSION]
        with mock.patch.object(NATIVE_CHECK, "run",
                               side_effect=subprocess.TimeoutExpired(cmd, NATIVE_CHECK.HARNESS_TIMEOUT)):
            with recorded() as results:
                answer = NATIVE_CHECK.deadline_run(cmd, timeout=NATIVE_CHECK.HARNESS_TIMEOUT)
                snapshot = list(results)
        self.assertNotEqual(answer.returncode, 0, "a timed-out command answered as a success")
        self.assertIn("timed out after", answer.stderr)
        self.assertEqual([(name, state) for name, state, _ in snapshot],
                         [("opencode session answers within its 300s bound", "FAIL")])
        self.assertIn("timed out after 300s", snapshot[0][2])

    def test_a_timed_out_harness_command_reaches_the_callers_own_answer(self) -> None:
        with mock.patch.object(NATIVE_CHECK, "run",
                               side_effect=subprocess.TimeoutExpired("opencode", NATIVE_CHECK.HARNESS_TIMEOUT)):
            with recorded() as results:
                answer = NATIVE_CHECK.ask_opencode("opencode", ["session", "export", SESSION], Path("/data"))
                snapshot = list(results)
        self.assertNotEqual(answer.returncode, 0, "the timeout raised out of ask_opencode")
        self.assertEqual([state for _, state, _ in snapshot], ["FAIL"])

    def test_a_timed_out_turn_is_recorded_and_the_stage_returns(self) -> None:
        target = turn_target()
        with mock.patch.object(NATIVE_CHECK, "run",
                               side_effect=subprocess.TimeoutExpired("opencode", NATIVE_CHECK.TURN_TIMEOUT)), \
             mock.patch.object(NATIVE_CHECK, "export_document",
                               side_effect=[(document(3), "ok", 0), (document(3), "ok", 0)]):
            with recorded() as results:
                NATIVE_CHECK._run_turn(mock.Mock(resume_prompt="a turn", expect=None), target, OPENCODE,
                                       Path("/data"), Path("/data/opencode/opencode.db"),
                                       Path("/workspace"))
                states = {name: state for name, state, _ in results}
        self.assertEqual(states[f"{OPENCODE} run answers within its {NATIVE_CHECK.TURN_TIMEOUT}s bound"],
                         "FAIL")
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "FAIL")

    def test_a_timed_out_turn_still_removes_the_planted_session_and_prints_a_verdict(self) -> None:
        """The end-to-end guarantee: a run whose own turn times out still reaches ``remove_planted`` --
        the planted session is deleted through the harness's own verb -- and ends with a ``RESULT`` line
        and its exit code, because the timeout is answered rather than raised."""
        session_id = "ses_installed000000000000000"
        verbs: list[list[str]] = []
        planted = {"removed": False}

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "import":
                return completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n")
            if verb[1] == "export":
                wanted = verb[2]
                if wanted == NATIVE_CHECK.UNRELATED_SESSION:
                    if planted["removed"]:
                        return completed(stderr=f"Session not found: {wanted}\n", returncode=1)
                    return completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))
                if wanted == session_id:
                    return completed(stdout=json.dumps(document(3, session_id=session_id)))
                return completed(stderr=f"Session not found: {wanted}\n", returncode=1)
            if verb[1] == "list":
                return completed(stdout="[]")
            if verb[1] == "delete":
                planted["removed"] = True
                return completed(stdout=f"Session {verb[2]} deleted\n")
            raise AssertionError(f"unexpected harness verb {verb}")

        with tempfile.TemporaryDirectory() as scratch:
            scratch_path = Path(scratch)
            # The caller's spelling names a data directory itself (`…/opencode/opencode`): the check's own
            # root is its parent, the store it plants, exports and deletes below is the store inside that
            # named directory, and the conversion must be handed the caller's own spelling -- not the
            # normalized root -- or the route would normalize a second time and install into
            # `<root>/opencode.db`, a different store than the one this check reads.
            target_argument = (scratch_path / NATIVE_CHECK.OPENCODE_DATA_DIR
                               / NATIVE_CHECK.OPENCODE_DATA_DIR).resolve()
            data_root = NATIVE_CHECK.opencode_data_root_of(target_argument)
            store = data_root / NATIVE_CHECK.OPENCODE_DATA_DIR / NATIVE_CHECK.OPENCODE_STORE
            report = {
                "conversion": {"state": "converted", "carried": {"messages": 7, "tool_calls": 1,
                                                                 "tool_results": 1},
                               "losses": [{"category": "usage_zeroed", "count": 1}]},
                "installation": {"state": "installed",
                                 "mechanism": "the target harness's own session importer "
                                              "(opencode session import)",
                                 "path": str(store)},
                "verification": {"structure": {"state": "verified"},
                                 "load": {"state": "verified", "detail": "…"}},
                "target_session": {
                    "session_id": session_id, "cli_version": "2.2.5",
                    "model": "target-model", "model_provider": "target-provider",
                    "model_provider_source": "the harness's own debug agents answer",
                    "resume_environment": {NATIVE_CHECK.OPENCODE_DATA_ROOT_ENV: str(data_root),
                                           NATIVE_CHECK.OPENCODE_DB_ENV: str(store)},
                    "resume_arguments": [OPENCODE, "run", "--standalone", "--session", session_id, "--", ""],
                    "exec_resume_arguments": [OPENCODE, "run", "--standalone", "--session", session_id,
                                              "--", NATIVE_CHECK.PROMPT_PLACEHOLDER],
                },
            }

            commands: list[list[str]] = []

            def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
                commands.append(list(command))
                if "--version" in command:
                    return completed(stdout="opencode v2.2.5")
                if "build" in command:
                    return completed()
                if "continue" in command:
                    return completed(stdout=json.dumps(report))
                if "run" in command:
                    raise subprocess.TimeoutExpired(command, NATIVE_CHECK.TURN_TIMEOUT)
                raise AssertionError(f"unexpected command {command}")

            def which(name):
                return "/usr/local/bin/opencode" if name == "opencode" else "/usr/local/bin/go"

            with mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which), \
                 mock.patch.object(NATIVE_CHECK, "BINARY",
                                   scratch_path / "bin" / "ccc-native-check-opencode"), \
                 mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask), \
                 mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run):
                with recorded() as results:
                    stdout = io.StringIO()
                    with contextlib.redirect_stdout(stdout):
                        code = NATIVE_CHECK.main(["--opencode", "opencode",
                                                  "--target-home", str(target_argument),
                                                  "--resume-prompt", "a turn"])
                    states = {name: state for name, state, _ in results}
        printed = stdout.getvalue()
        self.assertIn("RESULT:", printed, f"the run printed no verdict: {printed[-800:]}")
        self.assertEqual(code, 1, printed)
        self.assertEqual(states[f"{OPENCODE} run answers within its {NATIVE_CHECK.TURN_TIMEOUT}s bound"],
                         "FAIL")
        self.assertEqual(states["the turn ran through the printed invocation and exited 0"], "FAIL")
        self.assertIn(["session", "delete", NATIVE_CHECK.UNRELATED_SESSION], verbs,
                      "a timeout left the planted session in the home")
        self.assertEqual(states["the planted session is removed through the harness's own session delete"],
                         "PASS")
        conversion = next(command for command in commands if "continue" in command)
        self.assertEqual(conversion[conversion.index("--target-home") + 1], str(target_argument),
                         "the conversion was handed a normalized spelling: the route would normalize "
                         "again and install into another store than the one this check reads")
        self.assertEqual(states["the report names the store the harness reads as the installed path"],
                         "PASS")


if __name__ == "__main__":
    unittest.main()
