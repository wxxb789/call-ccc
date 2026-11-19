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
  this run planted something;
* the leftover recognition: a store's session is this check's own only when its *whole* document matches
  the planted fixture, apart from the two members the importer rewrites (``info.projectID`` and
  ``info.time.updated``). A session a caller has continued, or whose recorded payload was changed, holds
  records this check never wrote and is never deleted -- the case this rule exists for.

Goal 0008's **source direction** (``--direction opencode-to-codex``) adds its own subjects, all driven the
same way:

* the fixture document this direction plants and compares against, and its refusal of a document that
  names no session;
* the comparison itself: a mangled tool name, input or result, a dropped assistant record, or a target that
  wrote a record's own words where the source's read carries the reader's rendering of a persisted
  reasoning trace, are each a difference -- and the rendering, built the way the inherited reader builds it,
  is what a faithful target holds byte for byte;
* the planting through the harness's own importer, including the reconciliation of an import whose process
  failed after committing;
* the turn vector read against the harness this run probed and the session it converted, and a vector that
  is not this run's running nothing;
* the printed Codex line, read the way a shell reads it (a quoted assignment is the *program* to a shell);
* one real turn recorded as what the harness answered -- a refused turn is a failed check, never a skip --
  with the rollout's own records showing the growth and the conversion's own records surviving it;
* the whole direction driven end to end with both harnesses stubbed, which is where the flow itself is
  exercised: the conversion must be asked for the **load** level (the one real turn is this check's own) and
  every check the run promises must be reached.
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


#: The Codex argument vector the source direction's reports print: the harness this run probed, the
#: resume subcommand, and the session a caller resumes.
CODEX_RESUME_ARGUMENTS = ["codex", "resume", "01a1f342-0000-7000-8000-000000000001"]


def codex_invocation(home, arguments: list[str]) -> str:
    """One printed **Codex** line as ``internal/ccc/continue.go``'s renderer spells it: the
    ``CODEX_HOME`` assignment -- its value quoted where a POSIX shell would act on it, which ``shellQuote``
    does for exactly those words -- then the argument vector, each word quoted the same way. (Codex takes
    its data directory from that variable alone, so the route prints no ``cd``; ``invocation`` below is the
    OpenCode target's own form, which pins the store beside the root.)

    A case builds the line it hands a reading with this, so the line is one the shipped renderer really
    prints: a hand-spelling holds only where ``str(Path(...))`` has no backslash in it, and a Windows path
    -- the spelling the ``windows-latest`` job has -- is quoted, so a case that spelled the line bare
    failed there against a report no renderer produced."""
    return " ".join([f"{NATIVE_CHECK.CODEX_HOME_ENV}={shlex.quote(str(home))}",
                     *(shlex.quote(word) for word in arguments)])


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

    def test_the_harnesss_own_lines_are_read_whole(self) -> None:
        """The readings mirror the route's own installer (``openCodeImportAnswer`` and
        ``openCodeRefusalNamesSession``): the words after the harness's marker must be this id and nothing
        else. A diagnostic that merely mentions the id -- measured: a timed-out command's recorded stderr
        echoes its own command line, which names the session it was asked about -- is not the harness's
        answer about it, and reading it as one would call a store free that nobody showed to be."""
        wanted = NATIVE_CHECK.UNRELATED_SESSION
        self.assertTrue(NATIVE_CHECK.harness_line_names(
            f"Session not found: {wanted}\n", NATIVE_CHECK.SESSION_NOT_FOUND, wanted))
        self.assertTrue(NATIVE_CHECK.harness_line_names(
            f"  Imported session: {wanted}  \n", NATIVE_CHECK.IMPORT_ACKNOWLEDGMENT, wanted))
        self.assertFalse(NATIVE_CHECK.harness_line_names(
            f"timed out after 300s: opencode session export {wanted}\n",
            NATIVE_CHECK.SESSION_NOT_FOUND, wanted))
        self.assertFalse(NATIVE_CHECK.harness_line_names(
            f"Session not found: {wanted}/sibling\n", NATIVE_CHECK.SESSION_NOT_FOUND, wanted))
        self.assertFalse(NATIVE_CHECK.harness_line_names(
            f"Session not found: {NATIVE_CHECK.UNKNOWN_SESSION}\n", NATIVE_CHECK.SESSION_NOT_FOUND, wanted))

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

    def test_an_import_that_failed_after_committing_is_reconciled_as_a_publication(self) -> None:
        """The window the route's own installer reconciles, in this check's own planting: measured on
        2.2.5 the harness commits the session and can still fail or be killed before it answers, so a run
        that read only the exit status would leave its own fixture in a caller-named store -- where its
        fixed id makes every later run's planting meet ``Session already exists``. The acknowledgment the
        timed-out import printed is its first answer; the harness's own reader is the next."""
        deadline = completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n",
                             stderr=f"timed out after {NATIVE_CHECK.HARNESS_TIMEOUT}s: opencode session "
                                    f"import …\n", returncode=-1)
        document = completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[deadline, document]):
            published, detail = NATIVE_CHECK.plant_unrelated("opencode", Path("/data"), Path(scratch))
        self.assertTrue(published, detail)
        self.assertIn(NATIVE_CHECK.UNRELATED_SESSION, detail)
        self.assertIn("reader", detail, "the reconciliation does not say which answer decided it")

    def test_an_import_that_failed_without_its_reader_confirming_is_unresolved(self) -> None:
        """An acknowledgment the harness's own reader does not confirm is neither: the run must not call
        it published (its cleanup would delete a session it cannot show is its own) and must not call the
        store empty either."""
        answered = completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[answered, refused]):
            published, detail = NATIVE_CHECK.plant_unrelated("opencode", Path("/data"), Path(scratch))
        self.assertFalse(published, detail)
        self.assertIn("unresolved", detail)

    def test_an_import_with_no_answer_at_all_is_not_a_publication(self) -> None:
        answered = completed(stderr="the importer refused this planting\n", returncode=1)
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[answered, refused]):
            published, detail = NATIVE_CHECK.plant_unrelated("opencode", Path("/data"), Path(scratch))
        self.assertFalse(published)
        self.assertIn("refused", detail)

    def test_a_store_holding_this_checks_own_leftover_has_it_removed_before_planting(self) -> None:
        """An id planted by an earlier run whose cleanup never ran would block this one (``Session already
        exists``). A store holding *this check's own fixture* under it -- recognised by the document its
        own reader answers, which nothing else writes -- has it removed through the harness's own delete,
        and only then is the id free to plant."""
        verbs: list[list[str]] = []
        present = {"session": True}

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "export":
                if present["session"]:
                    return completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))
                return completed(stderr=f"Session not found: {verb[2]}\n", returncode=1)
            if verb[1] == "delete":
                present["session"] = False
                return completed(stdout=f"Session {verb[2]} deleted\n")
            raise AssertionError(f"unexpected harness verb {verb}")

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertTrue(free)
        self.assertIn(["session", "delete", NATIVE_CHECK.UNRELATED_SESSION], verbs)
        self.assertEqual(states, [
            ("an earlier run's own unrelated fixture is removed before this run plants", "PASS"),
        ])

    def test_a_foreign_session_under_that_id_is_left_alone(self) -> None:
        """The id is fabricated, but a store is the caller's: a session under it whose own document is not
        this check's fixture is never deleted, and the run says so instead."""
        foreign = NATIVE_CHECK.unrelated_document()
        foreign["messages"][0]["text"] = "a session this check did not write"
        verbs: list[list[str]] = []

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "export":
                return completed(stdout=json.dumps(foreign))
            raise AssertionError(f"unexpected harness verb {verb}")

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertFalse(free)
        self.assertEqual(verbs, [["session", "export", NATIVE_CHECK.UNRELATED_SESSION]])
        self.assertEqual(states, [("the id this run plants is free in the store", "FAIL")])

    def test_a_leftover_the_importer_rewrote_is_still_recognised(self) -> None:
        """The store's own export of this check's fixture is not byte-identical to the planted document:
        the harness's importer recomputes ``info.projectID`` from the session's directory and rewrites
        ``info.time.updated`` with the import's own instant (measured on 2.2.5). Those two members are
        normalized on both sides, so the leftover is still recognised and removed -- a comparison that
        demanded the planted bytes would leave this check's own fixture blocking every later run."""
        verbs: list[list[str]] = []
        present = {"session": True}
        rewritten = NATIVE_CHECK.unrelated_document()
        rewritten["info"]["projectID"] = "470c8549ef01a2a9d3c85b22965766cefc874134"
        rewritten["info"]["time"]["updated"] = 1794990087265

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "export":
                if present["session"]:
                    return completed(stdout=json.dumps(rewritten))
                return completed(stderr=f"Session not found: {verb[2]}\n", returncode=1)
            if verb[1] == "delete":
                present["session"] = False
                return completed(stdout=f"Session {verb[2]} deleted\n")
            raise AssertionError(f"unexpected harness verb {verb}")

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertTrue(free)
        self.assertIn(["session", "delete", NATIVE_CHECK.UNRELATED_SESSION], verbs)
        self.assertEqual(states, [
            ("an earlier run's own unrelated fixture is removed before this run plants", "PASS"),
        ])

    def test_a_leftover_a_caller_continued_is_left_alone(self) -> None:
        """A leftover that has received the caller's own history since -- a turn appended by a run in that
        session -- holds records this check never wrote. Recognising the fixture by any single member of it
        would delete that history with it, so the recognition is the whole document, and the continued
        session is left exactly as found: no delete is attempted."""
        continued = NATIVE_CHECK.unrelated_document()
        continued["messages"].append({
            "id": "msg_continued_0000000000000",
            "type": "assistant",
            "time": {"created": 1760000001000, "completed": 1760000002000},
            "agent": "build",
            "model": {"id": "m", "providerID": "p"},
            "content": [{"type": "text", "text": "the caller's own turn"}],
        })
        verbs: list[list[str]] = []

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "export":
                return completed(stdout=json.dumps(continued))
            raise AssertionError(f"unexpected harness verb {verb}")

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertFalse(free)
        self.assertEqual(verbs, [["session", "export", NATIVE_CHECK.UNRELATED_SESSION]])
        self.assertEqual(states, [("the id this run plants is free in the store", "FAIL")])

    def test_a_leftover_whose_recorded_record_changed_is_left_alone(self) -> None:
        """A leftover whose recorded message was rewritten -- its ``files`` member moved, say -- is not the
        document this check planted, and the id and the original text alone must not authorise deleting
        it."""
        modified = NATIVE_CHECK.unrelated_document()
        modified["messages"][0]["files"] = ["/tmp/a-file-the-fixture-never-recorded"]
        verbs: list[list[str]] = []

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "export":
                return completed(stdout=json.dumps(modified))
            raise AssertionError(f"unexpected harness verb {verb}")

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertFalse(free)
        self.assertEqual(verbs, [["session", "export", NATIVE_CHECK.UNRELATED_SESSION]])
        self.assertEqual(states, [("the id this run plants is free in the store", "FAIL")])

    def test_a_store_holding_nothing_under_that_id_is_free(self) -> None:
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=refused):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertTrue(free)
        self.assertEqual(states, [("the id this run plants is free in the store", "PASS")])

    def test_a_store_that_could_not_answer_is_not_free(self) -> None:
        """A probe that could not answer is not the store's own refusal: the id is not shown to be free,
        so the run plants nothing into it -- which is also what keeps the reconciliation from calling a
        session this run never published its own and deleting it."""
        overran = completed(stderr=f"timed out after {NATIVE_CHECK.HARNESS_TIMEOUT}s: opencode session "
                                   f"export {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=-1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", return_value=overran):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertFalse(free)
        self.assertEqual(states, [("the id this run plants is free in the store", "FAIL")])

    def test_a_leftover_the_reader_still_holds_is_not_read_as_removed(self) -> None:
        """The removal is checked, not assumed: a delete that answered and a store that still holds the
        session is a leftover that is still there, so the id is not free and this run plants nothing --
        otherwise the import would meet ``Session already exists`` and the report would say the id was
        cleared."""
        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            if verb[1] == "delete":
                return completed(stdout=f"Session {verb[2]} deleted\n")
            return completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))

        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask):
            with recorded() as results:
                free = NATIVE_CHECK.clear_own_unrelated_leftover("opencode", Path("/data"))
                states = [(name, state) for name, state, _ in results]
        self.assertFalse(free)
        self.assertEqual(states, [
            ("an earlier run's own unrelated fixture is removed before this run plants", "FAIL"),
        ])

    def test_nothing_is_recorded_when_nothing_was_published(self) -> None:
        with recorded() as results:
            NATIVE_CHECK.remove_planted("opencode", Path("/data"), published=False)
            self.assertEqual(list(results), [])

    def test_the_removal_is_recorded_and_its_absence_is_not_a_pass(self) -> None:
        deleted = completed(stdout=f"Session {NATIVE_CHECK.UNRELATED_SESSION} deleted\n")
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[deleted, refused]):
            with recorded() as results:
                NATIVE_CHECK.remove_planted("opencode", Path("/data"), published=True)
                states = [(name, state) for name, state, _ in results]
        self.assertEqual(states, [
            ("the planted session is removed through the harness's own session delete", "PASS"),
            ("the removed session no longer resolves", "PASS"),
        ])

    def test_a_delete_that_answered_not_found_is_reported(self) -> None:
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.UNRELATED_SESSION}\n", returncode=1)
        with mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[refused, refused]):
            with recorded() as results:
                NATIVE_CHECK.remove_planted("opencode", Path("/data"), published=True)
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
            if verb[1] == "export":
                # The store's own answer for the id this run is about to plant: free, so the run reaches
                # its planting.
                return completed(stderr=f"Session not found: {verb[2]}\n", returncode=1)
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
        # The version probe is the first command a run makes through the harness word, and every stage
        # this run hands one -- its own read of the id it plants, the planting, and the planting's
        # reconciliation -- carries the resolved word, never the caller's relative spelling.
        self.assertEqual([command[0] for command in commands], [self.resolved()])
        self.assertTrue(seen, "no stage ran through the harness word at all")
        self.assertEqual({stage for stage, _ in seen}, {self.resolved()},
                         f"a stage ran the caller's relative spelling: {seen}")

    def test_a_rooted_word_without_a_drive_is_resolved_by_the_hosts_own_rule(self) -> None:
        """A word the host does *not* call absolute is resolved rather than passed on.

        Since Python 3.13 ``os.path.isabs`` answers False for a rooted path that carries no drive
        (``/usr/local/bin/opencode`` hangs off the *current* drive), so a Windows run's word for a harness
        installed outside a drive is the ``os.path.abspath`` answer -- the word the run then hands the
        conversion and reads the report's own vector against. This host's ``os.path`` calls that shape
        absolute, so the two answers a Windows host gives it are put in front of the shipped resolution
        here, as the gate's own cases put the Windows compiler answer in front of their probe: without the
        resolution the case fails, which is what the round that added it left unchecked -- an end-to-end
        case spelled its report with the literal, the run probed the resolved word, and `windows-latest`
        measured the mismatch (the turn never reached its deadline record; the case raised ``KeyError``).
        """
        rooted = "/usr/local/bin/opencode"
        resolved = "D:\\usr\\local\\bin\\opencode"
        with mock.patch.object(NATIVE_CHECK.shutil, "which", return_value=rooted), \
             mock.patch.object(NATIVE_CHECK.os.path, "isabs", return_value=False), \
             mock.patch.object(NATIVE_CHECK.os.path, "abspath", return_value=resolved):
            self.assertEqual(NATIVE_CHECK.harness_executable(rooted), resolved,
                             "a word the host does not call absolute was passed on unresolved")

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

    #: The harness word the end-to-end case hands the run: a path absolute in the *host's own* spelling,
    #: spelled for the run this case drives rather than for one platform. The check resolves the word it
    #: drives once (``harness_executable``) and reads the report's own vector against that answer, and a
    #: word that is rooted but carries no drive (``/usr/local/bin/opencode``) is not absolute on Windows --
    #: ``os.path.isabs`` answers False for exactly that shape since Python 3.13 -- so the resolution would
    #: rewrite the literal against the current drive and the report's vector would name another word than
    #: the run probed. Measured on ``windows-latest``: the turn never reached its deadline record and this
    #: case's own ``states[…]`` lookup raised the ``KeyError`` (the ``states`` lookups must not be spelled
    #: with a word the host resolves; the resolution's own rule has
    #: ``HarnessExecutable.test_a_rooted_word_without_a_drive_is_resolved_by_the_hosts_own_rule``).
    HARNESS = str(Path(tempfile.gettempdir()) / "bin" / "opencode")

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
        self.assertTrue(os.path.isabs(self.HARNESS),
                        "the case's own harness word is not absolute in the host's own spelling, so the "
                        "run's resolution would rewrite it and the report's vector would name another word "
                        f"than the run probed: {self.HARNESS!r}")
        session_id = "ses_installed000000000000000"
        verbs: list[list[str]] = []
        # The store's own model for this case's stub: the id this run plants is absent until its import
        # publishes it, and absent again after the delete. Without the presence half, this run's own read
        # of the id it is about to plant (the leftover probe) would be answered as if the store held it --
        # which is exactly the state the probe exists to tell apart.
        planted = {"present": False, "removed": False}

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "import":
                planted["present"] = True
                return completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n")
            if verb[1] == "export":
                wanted = verb[2]
                if wanted == NATIVE_CHECK.UNRELATED_SESSION:
                    if not planted["present"]:
                        return completed(stderr=f"Session not found: {wanted}\n", returncode=1)
                    return completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))
                if wanted == session_id:
                    return completed(stdout=json.dumps(document(3, session_id=session_id)))
                return completed(stderr=f"Session not found: {wanted}\n", returncode=1)
            if verb[1] == "list":
                return completed(stdout="[]")
            if verb[1] == "delete":
                planted["present"] = False
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
                    "resume_arguments": [self.HARNESS, "run", "--standalone", "--session", session_id,
                                         "--", ""],
                    "exec_resume_arguments": [self.HARNESS, "run", "--standalone", "--session", session_id,
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
                return self.HARNESS if name == "opencode" else "/usr/local/bin/go"

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
        self.assertEqual(states[f"{self.HARNESS} run answers within its {NATIVE_CHECK.TURN_TIMEOUT}s bound"],
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

    def test_a_timed_out_command_keeps_what_it_printed(self) -> None:
        """The output a command did print survives its deadline. ``subprocess`` buffers what a command's
        pipes carried and hands it to the exception as the *bytes* it read, even where the runner asked for
        text (measured on this host's Python 3.15: ``exc.stdout`` is ``b'...'`` for a ``text=True`` run),
        and a slow import that acknowledged the session it committed before it hung is exactly the shape
        the planting reconciliation reads -- dropping it would make the check read a publication as an
        absence of one."""
        cmd = ["opencode", "session", "import", "unrelated-import.json"]
        answered = f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n"
        overran = subprocess.TimeoutExpired(cmd, NATIVE_CHECK.HARNESS_TIMEOUT,
                                            output=answered.encode(), stderr=b"")
        with mock.patch.object(NATIVE_CHECK, "run", side_effect=overran):
            with recorded() as results:
                answer = NATIVE_CHECK.deadline_run(cmd, timeout=NATIVE_CHECK.HARNESS_TIMEOUT)
                snapshot = list(results)
        self.assertIn(answered.strip(), answer.stdout,
                      "the timed-out command's own acknowledgment was dropped with its deadline")
        self.assertIn("timed out after", answer.stderr)
        self.assertEqual([(name, state) for name, state, _ in snapshot],
                         [("opencode session answers within its 300s bound", "FAIL")])

    def test_a_planting_that_overran_its_deadline_after_committing_is_removed_again(self) -> None:
        """The end-to-end shape of the planting reconciliation, through ``main``: an import that printed
        its own acknowledgment and then overran its deadline leaves the session in the store (measured on
        2.2.5, the harness commits the session and can still fail or be killed before it answers). The run
        reads that as a publication -- the acknowledgment it kept beside the deadline is its first answer,
        the harness's own reader the next -- and its cleanup removes the session through the harness's own
        delete, so the fixed id does not stay behind to make every later run's planting meet ``Session
        already exists``."""
        verbs: list[list[str]] = []
        store = {"unrelated": False}

        def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
            verbs.append(list(verb))
            if verb[1] == "import":
                store["unrelated"] = True
                return completed(stdout=f"Imported session: {NATIVE_CHECK.UNRELATED_SESSION}\n",
                                 stderr=f"timed out after {NATIVE_CHECK.HARNESS_TIMEOUT}s: opencode "
                                        f"session import …\n", returncode=-1)
            if verb[1] == "export":
                if verb[2] == NATIVE_CHECK.UNRELATED_SESSION and store["unrelated"]:
                    return completed(stdout=json.dumps(NATIVE_CHECK.unrelated_document()))
                return completed(stderr=f"Session not found: {verb[2]}\n", returncode=1)
            if verb[1] == "delete":
                store["unrelated"] = False
                return completed(stdout=f"Session {verb[2]} deleted\n")
            raise AssertionError(f"unexpected harness verb {verb}")

        def fake_run(command, cwd=None, env=None, timeout=None, replace_env=False):
            if "--version" in command:
                return completed(stdout="opencode v2.2.5")
            # The build is the stage after the planting: a run that stops there is one whose planting was
            # reconciled and whose cleanup this case reads.
            return completed(stderr="this case runs no build\n", returncode=1)

        def which(name):
            return self.HARNESS if name == "opencode" else "/usr/local/bin/go"

        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which), \
             mock.patch.object(NATIVE_CHECK, "BINARY",
                               Path(scratch) / "bin" / "ccc-native-check-opencode"), \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask), \
             mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run):
            with recorded() as results:
                stdout = io.StringIO()
                with contextlib.redirect_stdout(stdout):
                    code = NATIVE_CHECK.main(["--opencode", "opencode"])
                states = {name: state for name, state, _ in results}
        printed = stdout.getvalue()
        self.assertIn("RESULT:", printed, f"the run printed no verdict: {printed[-600:]}")
        self.assertEqual(states["the unrelated session is planted through the harness's own importer"],
                         "PASS", printed)
        self.assertIn(["session", "delete", NATIVE_CHECK.UNRELATED_SESSION], verbs,
                      f"a planting whose process overran its deadline left its fixture in the store: {verbs}")
        self.assertEqual(states["the planted session is removed through the harness's own session delete"],
                         "PASS")
        self.assertFalse(store["unrelated"], "the session this run published is still in the store")
        self.assertEqual(code, 1, printed)


# --- goal 0008's source direction (opencode-to-codex) ------------------------------------------------


class SourceDirectionFraming(unittest.TestCase):
    """The source direction needs *both* harnesses: the OpenCode binary writes the source store (only its
    own importer can plant a session) and Codex is the target whose loader answers. A host missing either
    is an INCOMPLETE run naming which, never a pass."""

    def test_a_missing_codex_binary_is_incomplete_and_runs_no_check(self) -> None:
        def which(name):
            return None if name == "codex" else ("/usr/local/bin/go" if name == "go" else "/bin/opencode")

        with mock.patch.object(NATIVE_CHECK.shutil, "which", side_effect=which):
            with recorded() as results:
                stdout = io.StringIO()
                with contextlib.redirect_stdout(stdout):
                    code = NATIVE_CHECK.main(["--direction", NATIVE_CHECK.SOURCE_DIRECTION,
                                              "--opencode", "opencode"])
        printed = stdout.getvalue()
        self.assertEqual(code, 2, printed)
        self.assertIn("RESULT: INCOMPLETE", printed)
        self.assertIn("0 check(s) ran", printed)
        self.assertIn("codex", printed)
        self.assertEqual(list(results), [])

    def test_an_unknown_direction_is_refused(self) -> None:
        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as raised:
                NATIVE_CHECK.main(["--direction", "opencode-to-claude"])
        self.assertEqual(raised.exception.code, 2,
                         "a direction this check does not verify must not be accepted")

    def test_the_committed_fixture_is_the_source_document(self) -> None:
        """The fixture's own bytes are what this direction plants and compares against, so a fixture edit
        that dropped a member the comparison asks about has to fail a case here rather than silently
        narrow what the native run checks."""
        root = Path(__file__).resolve().parents[2]
        document = NATIVE_CHECK.fixture_document(root / NATIVE_CHECK.FIXTURE_ROOT
                                                 / NATIVE_CHECK.OPENCODE_SOURCE_FIXTURE)
        self.assertEqual((document.get("info") or {}).get("id"), NATIVE_CHECK.OPENCODE_SOURCE_SESSION)
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_names"], ["read"])
        self.assertEqual(len(evidence["call_results"]), 1)
        self.assertEqual(evidence["call_result_owners"], ["0"],
                         "the fixture's one result is not read under its own call")
        self.assertEqual(len(evidence["user_texts"]), 2)
        # The assistant side is the reader's own rendering of each record, byte for byte: a fixture edit
        # that dropped the reasoning part, or an extractor that stopped building the rendering, fails here.
        self.assertEqual(evidence["assistant_texts"], [
            "[Thinking]\nThe grep hit is maxBackoffSeconds = 30 in internal/retry/config.go.\n"
            "[/Thinking]\nThe cap is `maxBackoffSeconds = 30` in internal/retry/config.go.",
            "Four attempts, and thirty seconds of backoff between them.",
        ])

    def test_a_default_fixture_naming_another_session_is_refused(self) -> None:
        """The committed fixture is the identity this direction knows: a regenerated fixture whose id
        drifted would plant a session the direction line names wrongly, so the run refuses before the
        harness is asked to import anything (``--source`` names a document of the caller's own, and is not
        held to this identity)."""
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root / "opencode").mkdir()
            (root / "opencode" / "other.json").write_text(json.dumps({
                "info": {"id": "ses_someone_elses_fixture", "time": {"created": 1, "updated": 1}},
                "messages": []}), encoding="utf-8")
            stdout, stderr = io.StringIO(), io.StringIO()
            with mock.patch.object(NATIVE_CHECK, "FIXTURE_ROOT", "."), \
                 mock.patch.object(NATIVE_CHECK, "OPENCODE_SOURCE_FIXTURE", "opencode/other.json"), \
                 recorded() as results, \
                 contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                code = NATIVE_CHECK.run_source_direction(
                    mock.Mock(direction=NATIVE_CHECK.SOURCE_DIRECTION, source=None, target_home=None,
                              resume_prompt=None, expect=None),
                    root, "/usr/local/bin/go", "opencode", "codex", root / "scratch")
            self.assertEqual(code, 2, stderr.getvalue())
            self.assertIn("drifted apart", stderr.getvalue())
            self.assertEqual(list(results), [],
                             "a fixture whose identity drifted must be refused before any check runs")

    def test_a_document_without_an_id_is_refused(self) -> None:
        with tempfile.TemporaryDirectory() as scratch:
            path = Path(scratch) / "no-id.json"
            path.write_text('{"info": {}, "messages": []}', encoding="utf-8")
            with self.assertRaises(SystemExit):
                NATIVE_CHECK.fixture_document(path)

    def test_a_document_with_an_unmodelled_message_kind_is_refused_before_planting(self) -> None:
        """The comparison's own refusal, driven through the direction: a document carrying a ``shell``
        record -- a kind whose command and tool evidence the comparison holds no member for -- stops the run
        at exit 2 naming the kind, with the harness never asked to import anything (so no session this run
        would have to remove is ever published)."""
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root / "opencode").mkdir()
            (root / "opencode" / "shell.json").write_text(json.dumps({
                "info": {"id": NATIVE_CHECK.OPENCODE_SOURCE_SESSION, "time": {"created": 1, "updated": 1}},
                "messages": [{"id": "msg_1", "type": "shell", "command": "ls",
                              "callID": "call_1", "time": {"completed": 1}}]}), encoding="utf-8")
            stdout, stderr = io.StringIO(), io.StringIO()
            with mock.patch.object(NATIVE_CHECK, "FIXTURE_ROOT", "."), \
                 mock.patch.object(NATIVE_CHECK, "OPENCODE_SOURCE_FIXTURE", "opencode/shell.json"), \
                 mock.patch.object(NATIVE_CHECK, "ask_opencode") as ask, \
                 recorded() as results, \
                 contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                code = NATIVE_CHECK.run_source_direction(
                    mock.Mock(direction=NATIVE_CHECK.SOURCE_DIRECTION, source=None, target_home=None,
                              resume_prompt=None, expect=None),
                    root, "/usr/local/bin/go", "opencode", "codex", root / "scratch")
            self.assertEqual(code, 2, stderr.getvalue())
            self.assertIn("'shell'", stderr.getvalue())
            self.assertEqual(list(results), [],
                             "a document the comparison refuses must be refused before any check runs")
            ask.assert_not_called()

    def test_a_planting_that_the_readers_own_answer_does_not_confirm_is_not_published(self) -> None:
        refused = completed(stderr=f"Session not found: {NATIVE_CHECK.OPENCODE_SOURCE_SESSION}\n",
                            returncode=1)
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[completed(returncode=1), refused]):
            published, detail = NATIVE_CHECK.plant_fixture(
                "opencode", Path("/data"), Path(scratch) / "fixture.json", Path(scratch),
                NATIVE_CHECK.OPENCODE_SOURCE_SESSION)
        self.assertFalse(published, detail)

    def test_a_planting_whose_process_failed_after_committing_is_reconciled(self) -> None:
        """The route's own reconciliation, in this direction's planting: an import whose process overran
        its deadline after the harness committed the session leaves the harness's own reader as the next
        answer, and the run may convert what that reader holds."""
        overran = completed(stderr="timed out after 300s: opencode session import …\n", returncode=-1)
        answered = completed(stdout=json.dumps({"info": {"id": NATIVE_CHECK.OPENCODE_SOURCE_SESSION},
                                                "messages": []}))
        with tempfile.TemporaryDirectory() as scratch, \
             mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=[overran, answered]):
            published, detail = NATIVE_CHECK.plant_fixture(
                "opencode", Path("/data"), Path(scratch) / "fixture.json", Path(scratch),
                NATIVE_CHECK.OPENCODE_SOURCE_SESSION)
        self.assertTrue(published, detail)
        self.assertIn("reader", detail)


class SourceEvidence(unittest.TestCase):
    """The comparison this direction rests on: the fixture document's own bytes against the target's own
    records, both read into the same members. Every case here breaks one member and requires it to be
    reported."""

    def expected(self) -> dict:
        return {"user_texts": ["a prompt"], "call_names": ["read"],
                "call_inputs": ['{"path":"/x"}'], "call_results": ["the file's lines"],
                "call_result_owners": ["0"],
                "assistant_texts": ["[Thinking]\nthe cap is thirty\n[/Thinking]\nthe cap is thirty",
                                    "four attempts"]}

    def rollout(self) -> dict:
        return dict(self.expected())

    def differences(self, **broken) -> list[str]:
        got = self.rollout()
        got.update(broken)
        return NATIVE_CHECK.evidence_differences(self.expected(), got)

    def test_a_target_that_mangled_the_call_or_a_prompt_is_a_difference(self) -> None:
        for label, broken in {
            "a tool name": {"call_names": ["grep"]},
            "a recorded input": {"call_inputs": ['{"path":"/y"}']},
            "a recorded result": {"call_results": ["something else"]},
            "a reassigned result": {"call_result_owners": ["1"]},
            "a user text": {"user_texts": ["another prompt"]},
            "an assistant text": {"assistant_texts": ["four attempts"]},
            "a dropped assistant record": {"assistant_texts": []},
        }.items():
            with self.subTest(case=label):
                self.assertTrue(self.differences(**broken), f"{label} difference was not reported")

    def test_the_rendering_is_what_a_target_must_hold(self) -> None:
        """The reader carries a persisted reasoning trace by rendering it into the record's text, so the
        target's own records hold the rendering -- measured on the installed rollout. A target that wrote
        the record's own words instead has dropped the trace, and nothing is compared by containment: the
        bare text is a difference, not a match."""
        self.assertEqual(self.differences(), [], "a correctly rendered record was reported as a difference")
        self.assertTrue(self.differences(assistant_texts=["the cap is thirty", "four attempts"]),
                        "a target that dropped the reader's rendering was reported as identical")

    def test_a_record_carrying_a_call_is_expected_with_no_text(self) -> None:
        """A record that carries a tool call has no text the source's read establishes: the reader renders a
        trace into ``Content`` and records no trace text and no projection, so nothing separates the
        rendering from the record's own words, and every writer omits the text and counts it
        (``unresolved_dialogue``) while the call and its result travel. Measured on the writer (one text part
        beside one completed tool part): the rollout holds the ``function_call`` and its
        ``function_call_output`` and no assistant message for the record.

        The expectation holds no text for it, which *pins* the omission: a target that wrote the rendering
        anyway holds an assistant text the read never established and is a difference. The planting's own
        comparison is document against document and keeps the stricter reading -- there the record's text is
        recorded (and a store whose assistant record lost a part must stay a difference)."""
        def mixed(with_text: bool) -> dict:
            parts = ([{"type": "text", "text": "Let me check the policy file too."}] if with_text else []) + [
                {"type": "tool", "name": "read",
                 "state": {"status": "completed", "input": {"path": "/x"},
                           "content": [{"type": "text", "text": "the file's lines"}]}}]
            return {"type": "assistant", "content": parts}

        document = {"messages": [mixed(with_text=True),
                                 {"type": "assistant", "content": [
                                     {"type": "reasoning", "text": "the cap is thirty"},
                                     {"type": "text", "text": "the cap is thirty"}]}]}
        recorded = NATIVE_CHECK.opencode_document_evidence(document)
        written = NATIVE_CHECK.opencode_written_evidence(document)
        rendering = "[Thinking]\nthe cap is thirty\n[/Thinking]\nthe cap is thirty"
        self.assertEqual(recorded["assistant_texts"], ["Let me check the policy file too.", rendering],
                         "the planting's own reading dropped a text the document records")
        self.assertEqual(written["assistant_texts"], [rendering],
                         "the written reading kept the text of a record that carries a call")
        for member in ("call_names", "call_inputs", "call_results", "call_result_owners"):
            self.assertEqual(written[member], recorded[member],
                             f"the two readings differ in {member}, which they must not")

        # A conversion's own answer for the document: the call, its recorded result, and the second
        # record's text -- and no message for the mixed record's text.
        rollout = {"user_texts": [], "call_names": ["read"], "call_inputs": ['{"path":"/x"}'],
                   "call_results": ["the file's lines"], "call_result_owners": ["0"],
                   "assistant_texts": [rendering]}
        self.assertEqual(NATIVE_CHECK.evidence_differences(written, rollout), [],
                         "a conversion that omits the unestablished text was reported as a difference")
        self.assertTrue(NATIVE_CHECK.evidence_differences(
            written, {**rollout, "assistant_texts": ["Let me check the policy file too.", rendering]}),
            "a target that wrote the mixed record's text was reported as identical")
        self.assertTrue(NATIVE_CHECK.evidence_differences(
            written, {**rollout, "call_names": []}),
            "a target that dropped the call the record carries was reported as identical")

    def test_a_second_output_for_one_call_is_a_difference(self) -> None:
        """A completed OpenCode tool part yields the part's own ``time.ran`` as a second, textless event;
        a target that wrote it as another output holds a tool output the source never recorded."""
        self.assertTrue(self.differences(call_results=["the file's lines", ""]),
                        "a textless progress event written as a second output was not reported")

    def test_an_output_reassigned_to_another_call_is_a_difference(self) -> None:
        """The finding the detached result list could not see: a two-call document whose target reassigns
        the recorded outputs to the wrong calls. **Same count, same order, same texts** -- only the
        ``call_id`` each output is written under moves -- so a comparison of detached result lists answers
        "identical", and the pairing member (``call_result_owners``) is what reports it. Both extractors are
        driven, not a hand-built dict: the document through ``opencode_written_evidence`` and the rollout
        through ``rollout_records_of``/``rollout_evidence``, which is the pairing a reader of the file
        makes."""
        document = {"messages": [
            {"type": "user", "text": "a prompt", "files": []},
            {"type": "assistant", "content": [
                {"type": "tool", "id": "call_a", "name": "read",
                 "state": {"status": "completed", "input": {"path": "/a"},
                           "content": [{"type": "text", "text": "A's lines"}]}},
                {"type": "tool", "id": "call_b", "name": "grep",
                 "state": {"status": "completed", "input": {"pattern": "x"},
                           "content": [{"type": "text", "text": "B's lines"}]}}]}]}
        expected = NATIVE_CHECK.opencode_written_evidence(document)
        records = rollout_records_of(document)
        self.assertEqual(expected["call_result_owners"], ["0", "1"],
                         "the source's own reading does not pair each result with its call")
        self.assertEqual(NATIVE_CHECK.evidence_differences(expected, NATIVE_CHECK.rollout_evidence(records)),
                         [], "the case's own rollout is not what a conversion writes")

        def with_owners(owners: list[str]) -> list[dict]:
            """The same records, each output written under the call ``owners`` names at its place -- the
            order, the texts and the count are the ones the conversion wrote."""
            mutated: list[dict] = []
            index = 0
            for record in records:
                payload = dict(record["payload"])
                if payload.get("type") == "function_call_output":
                    payload["call_id"] = owners[index]
                    index += 1
                mutated.append({**record, "payload": payload})
            return mutated

        for case, owners in {"both outputs claimed by the first call": ["call_a", "call_a"],
                             "the two outputs' owners swapped": ["call_b", "call_a"]}.items():
            with self.subTest(case=case):
                got = NATIVE_CHECK.rollout_evidence(with_owners(owners))
                self.assertEqual(got["call_results"], expected["call_results"],
                                 "the case moved a text, not only the ownership")
                self.assertEqual(got["call_names"], expected["call_names"])
                differences = NATIVE_CHECK.evidence_differences(expected, got)
                self.assertTrue(any("owner" in difference for difference in differences),
                                f"{case} was not reported: {differences}")
        self.assertEqual(
            NATIVE_CHECK.rollout_evidence(with_owners(["call_a", "call_a"]))["call_result_owners"],
            ["0", NATIVE_CHECK.CALL_RESULT_UNOWNED],
            "an output no unmatched call record answers is not read as unowned")

    def test_a_document_that_lost_the_reasoning_part_is_a_difference(self) -> None:
        """The planting's own comparison is document against document: a store whose assistant record lost
        the reasoning part renders a different text, and that is a difference even where the record's own
        words survived intact."""
        document = {"messages": [{"type": "assistant", "content": [
            {"type": "reasoning", "text": "the cap is thirty"},
            {"type": "text", "text": "the cap is thirty"}]}]}
        without = {"messages": [{"type": "assistant", "content": [
            {"type": "text", "text": "the cap is thirty"}]}]}
        self.assertTrue(NATIVE_CHECK.evidence_differences(
            NATIVE_CHECK.opencode_document_evidence(document),
            NATIVE_CHECK.opencode_document_evidence(without)))

    def test_the_tool_input_is_compared_as_a_value(self) -> None:
        """Measured: a Codex rollout writes a call's arguments as a JSON string and the harness's own store
        keeps the parsed value, so the comparison reads the value -- the key order and the whitespace are
        who wrote it, not what it says."""
        self.assertEqual(NATIVE_CHECK.canonical_json_value('{"a": 1, "b": 2}'),
                         NATIVE_CHECK.canonical_json_value({"b": 2, "a": 1}))

    def test_the_expectation_holds_what_a_conversion_writes_for_an_open_call(self) -> None:
        """A ``pending``/``streaming`` part's input is blanked by the read and written as ``{}`` by each
        writer (counted ``empty_tool_inputs``), and no result is written while the part is open: an
        expectation holding the document's own member would report a difference that is not one."""
        document = {"messages": [{"type": "assistant", "content": [{
            "type": "tool", "name": "read",
            "state": {"status": "streaming", "input": {"path": "/x"}}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_inputs"], ["{}"])
        self.assertEqual(evidence["call_results"], [])
        settled = {"user_texts": [], "call_names": ["read"], "call_inputs": ["{}"],
                   "call_results": [], "call_result_owners": [], "assistant_texts": []}
        self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [],
                         "the empty object a writer holds for an open call was reported as a difference")

    def test_the_structured_attachment_is_expected_only_for_the_read_tool(self) -> None:
        """The reader appends a part's structured UTF-8 content only where the part's own name is ``read``
        (``internal/parser``'s ``openCodeV2ToolCall``), so the expectation has to gate on the name too: a
        target that wrote exactly what the reader answered for a ``grep`` part is not a difference."""
        def part(name: str) -> dict:
            return {"type": "tool", "name": name, "state": {
                "status": "completed", "input": {"path": "/x"},
                "content": [{"type": "text", "text": "the file's lines"}],
                "structured": {"encoding": "utf8", "content": "38: // policy.go"}}}

        for name, expected_text in (("read", "the file's lines\n38: // policy.go"),
                                    ("grep", "the file's lines")):
            with self.subTest(tool=name):
                document = {"messages": [{"type": "assistant", "content": [part(name)]}]}
                evidence = NATIVE_CHECK.opencode_document_evidence(document)
                self.assertEqual(evidence["call_results"], [expected_text])
                settled = {"user_texts": [], "call_names": [name], "call_inputs": ['{"path":"/x"}'],
                           "call_results": [expected_text], "call_result_owners": ["0"],
                           "assistant_texts": []}
                self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [])

    def test_a_part_carrying_a_file_block_is_expected_with_no_text(self) -> None:
        """The reader turns a part's text into result *blocks* when its content carries a ``file`` block,
        and the target writers hold no field for those (counted ``unrepresentable_result_blocks``): the
        text never reaches the target, so the expectation holds none and a target without an output for
        that call is not a difference."""
        document = {"messages": [{"type": "assistant", "content": [{
            "type": "tool", "name": "read",
            "state": {"status": "completed", "input": {"path": "/x"},
                      "content": [{"type": "file", "uri": "file:///x", "mime": "text/plain"},
                                  {"type": "text", "text": "the file's lines"}]}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], [])
        settled = {"user_texts": [], "call_names": ["read"], "call_inputs": ['{"path":"/x"}'],
                   "call_results": [], "call_result_owners": [], "assistant_texts": []}
        self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [])

    def test_an_errored_result_is_expected_because_the_writers_write_its_text(self) -> None:
        """The reader answers a result for an errored part as well as a completed one (its own switch) and
        the writers write its text; only the *outcome* is a counted loss (``tool_result_outcomes``)."""
        document = {"messages": [{"type": "assistant", "content": [{
            "type": "tool", "name": "shell",
            "state": {"status": "error", "input": {"command": "false"},
                      "content": [{"type": "text", "text": "exit status 1"}]}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], ["exit status 1"])

    def test_an_empty_text_block_is_part_of_the_recorded_result(self) -> None:
        """The reader appends *every* ``text`` block of a part's terminal content, an empty one included
        (``internal/parser``'s ``openCodeV2ToolCall`` calls its own ``appendText`` unconditionally), and a
        conversion writes that joined text verbatim: measured on this host through the carried reader, a
        document whose part records ``[{"type":"text","text":""},{"type":"text","text":"read 12 lines"}]``
        makes the reader answer ``"\\nread 12 lines"`` (the reviewer measured the same shape end to end
        through the installed harness). An expectation that dropped the empty block would report a
        faithful conversion as a difference."""
        document = {"messages": [{"type": "assistant", "content": [{
            "type": "tool", "name": "read",
            "state": {"status": "completed", "input": {"path": "/x"},
                      "content": [{"type": "text", "text": ""},
                                  {"type": "text", "text": "read 12 lines"}]}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], ["\nread 12 lines"])
        settled = {"user_texts": [], "call_names": ["read"], "call_inputs": ['{"path":"/x"}'],
                   "call_results": ["\nread 12 lines"], "call_result_owners": ["0"],
                   "assistant_texts": []}
        self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [])

    def test_the_error_message_is_part_of_an_errored_result(self) -> None:
        """The reader's own errored branch appends the part's recorded ``state.error.message`` to the
        result text, and a ``shell``/``bash`` part whose recorded exit is nonzero takes that branch even
        when its status is ``completed`` (the reader's own ``shellFailed`` arm): both are text a target
        holds, so the expectation holds them; an exit that reads as zero cites no message."""
        def part(**state: object) -> dict:
            state.setdefault("input", {"command": "false"})
            return {"type": "tool", "name": "bash", "state": state}

        document = {"messages": [{"type": "assistant", "content": [
            part(status="error", content=[{"type": "text", "text": "exit 1"}], error={"message": "boom"}),
            part(status="completed", metadata={"exit": 1}, error={"message": "still boom"}),
            part(status="completed", metadata={"exit": 0}, error={"message": "not cited"}),
            part(status="completed", metadata={"exit": 1}),
            part(status="completed", structured={"exit": "2"}, error={"message": "from structured"})]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], ["exit 1\nboom", "still boom", "from structured"])

    def test_a_user_message_with_files_is_expected_as_the_reader_renders_it(self) -> None:
        """The reader renders a user record's ``files`` into its own text -- ``[Attachment: <name>]`` with
        the name falling back to the recorded MIME type, and a ``text/*`` payload's decoded text under it
        where the member decodes to non-empty valid UTF-8 -- and a conversion carries that rendering as the
        prompt (measured: ``aGVsZA==`` decodes to ``held``; invalid base64 and an empty payload add
        nothing): an expectation holding only the message's own ``text`` would report a faithful conversion
        as a difference."""
        document = {"messages": [
            {"type": "user", "text": "look at these", "files": [
                {"name": "notes.txt", "mime": "text/plain", "data": "aGVsZA=="},
                {"name": "", "mime": "application/pdf", "data": "AAAA"},
                {"name": "broken.txt", "mime": "text/plain", "data": "not base64!!"}]},
            {"type": "user", "text": "", "files": [
                {"name": "solo", "mime": "text/plain", "data": ""}]},
        ]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["user_texts"], [
            "look at these\n[Attachment: notes.txt]\nheld\n[Attachment: application/pdf]\n"
            "[Attachment: broken.txt]",
            "[Attachment: solo]"])

    def test_a_whitespace_only_user_text_is_expected_with_no_prompt(self) -> None:
        """The reader's own filter drops a record whose content ``strings.TrimSpace`` empties and that
        carries neither tool use nor a compaction boundary -- a user record carries neither -- so the read
        answers no record and a conversion writes no prompt. The trimming is the reader's own and not
        Python's wider ``str.strip``: measured over every code point, Go's space set is Python's
        ``str.isspace`` set minus the information separators ``\\x1c``-``\\x1f``, so a record whose whole
        text is ``"\\x1c"`` *is* a record the read keeps, and the prompt a target holds."""
        document = {"messages": [{"type": "user", "text": "   ", "files": []},
                                 {"type": "user", "text": "\x1c", "files": []}]}
        self.assertEqual(NATIVE_CHECK.opencode_document_evidence(document)["user_texts"], ["\x1c"])

    def test_a_whitespace_only_assistant_text_is_expected_with_no_message(self) -> None:
        """An assistant record whose rendered text is only whitespace is not a message a conversion writes:
        the reader's own ``strings.TrimSpace`` filter drops the record where nothing else keeps it, and
        where token usage keeps it the writer's own whitespace rule still leaves the message out
        (``whitespace_only_dialogue``) -- because the target's own reader would read a blank-after-trimming
        message as no message at all. The membership is that same trimming, Go's own set and not Python's
        wider ``str.strip``: a record whose whole text is the separator ``\x1c`` *is* one the read carries
        and the writer writes, so it is a member."""
        document = {"messages": [
            {"type": "assistant", "content": [{"type": "text", "text": "   "}]},
            {"type": "assistant", "content": [{"type": "text", "text": "\x1c"}]}]}
        self.assertEqual(NATIVE_CHECK.opencode_written_evidence(document)["assistant_texts"], ["\x1c"])

    def test_a_document_with_an_unmodelled_message_kind_is_refused(self) -> None:
        """A ``shell`` record is the kind the reader answers as both a user record (the command) and tool
        evidence (the call and its recorded output) while this comparison holds no member for either: it is
        refused by name rather than compared as a subset."""
        document = {"messages": [{"id": "msg_1", "type": "shell", "command": "ls",
                                  "callID": "call_1", "time": {"completed": 1}}]}
        with self.assertRaises(SystemExit) as raised:
            NATIVE_CHECK.opencode_document_evidence(document)
        self.assertIn("'shell'", str(raised.exception))

    def test_a_textless_or_whitespace_only_result_is_expected_with_no_output(self) -> None:
        """A result whose text is empty or whitespace only is counted and left out of the target
        (``empty_tool_results``, ``whitespace_only_tool_results``), so the expectation holds no member for
        it -- an empty string there would be a difference the target cannot avoid."""
        document = {"messages": [{"type": "assistant", "content": [
            {"type": "tool", "name": "read", "state": {"status": "completed", "input": {}, "content": []}},
            {"type": "tool", "name": "read", "state": {"status": "completed", "input": {},
                                                       "content": [{"type": "text", "text": "   "}]}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], [])
        settled = {"user_texts": [], "call_names": ["read", "read"], "call_inputs": ["{}", "{}"],
                   "call_results": [], "call_result_owners": [], "assistant_texts": []}
        self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [])

    def test_a_result_whose_text_is_only_the_separator_is_expected(self) -> None:
        """The result membership asks the same Go-compatible predicate: ``\\x1c`` is not space to the
        reader's ``strings.TrimSpace`` (Go keeps the information separators Python's ``str.strip`` removes),
        so a result whose text is that one byte is one the writer writes -- dropping it, as the pre-fix
        Python trimming did, reports a faithful conversion as a difference."""
        document = {"messages": [{"type": "assistant", "content": [
            {"type": "tool", "name": "read", "state": {
                "status": "completed", "input": {},
                "content": [{"type": "text", "text": "\x1c"}]}}]}]}
        evidence = NATIVE_CHECK.opencode_document_evidence(document)
        self.assertEqual(evidence["call_results"], ["\x1c"])
        settled = {"user_texts": [], "call_names": ["read"], "call_inputs": ["{}"],
                   "call_results": ["\x1c"], "call_result_owners": ["0"], "assistant_texts": []}
        self.assertEqual(NATIVE_CHECK.evidence_differences(evidence, settled), [])


class SourceTurnVector(unittest.TestCase):
    """The report's two argument vectors are read against what this check knows on its own: the harness it
    probed and the session it converted. The exec form (``read_codex_turn_vector``) is the words one real
    turn runs; the interactive form (``read_codex_resume_vector``) is the line printed beside it. Both are
    read on *every* run -- ``record_codex_invocation`` asks for both, so a load-only run, which runs no
    turn, cannot accept a consistently rendered pair naming another program (finding 2 of the round's
    review)."""

    def target(self, **overrides) -> dict:
        target = {"session_id": "01a1f342-0000-7000-8000-000000000001",
                  "resume_arguments": ["/bin/codex", "resume",
                                       "01a1f342-0000-7000-8000-000000000001"],
                  "exec_resume_arguments": ["/bin/codex", "exec", "resume",
                                            "01a1f342-0000-7000-8000-000000000001",
                                            NATIVE_CHECK.PROMPT_PLACEHOLDER]}
        target.update(overrides)
        return target

    def test_the_vector_is_read_against_this_runs_harness_and_session(self) -> None:
        identity, problem = NATIVE_CHECK.read_codex_turn_vector(self.target(), "/bin/codex")
        self.assertEqual(problem, "")
        self.assertEqual(identity, ["exec", "resume", "01a1f342-0000-7000-8000-000000000001"])

    def test_a_vector_naming_another_program_session_or_shape_is_refused(self) -> None:
        cases = {
            "another program": self.target(exec_resume_arguments=["/bin/other", "exec", "resume",
                                                                  "01a1f342-0000-7000-8000-000000000001",
                                                                  NATIVE_CHECK.PROMPT_PLACEHOLDER]),
            "another session": self.target(exec_resume_arguments=["/bin/codex", "exec", "resume",
                                                                  "01a1f342-0000-7000-8000-000000000002",
                                                                  NATIVE_CHECK.PROMPT_PLACEHOLDER]),
            "no placeholder": self.target(exec_resume_arguments=["/bin/codex", "exec", "resume",
                                                                 "01a1f342-0000-7000-8000-000000000001",
                                                                 "a prompt spelled here"]),
            "another subcommand": self.target(exec_resume_arguments=["/bin/codex", "resume",
                                                                     "01a1f342-0000-7000-8000-000000000001"]),
            "no vector": self.target(exec_resume_arguments=[]),
        }
        for label, target in cases.items():
            with self.subTest(case=label):
                identity, problem = NATIVE_CHECK.read_codex_turn_vector(target, "/bin/codex")
                self.assertEqual(identity, [])
                self.assertTrue(problem, f"{label} was accepted")

    def test_the_interactive_vector_is_read_against_this_runs_harness_and_session(self) -> None:
        words, problem = NATIVE_CHECK.read_codex_resume_vector(self.target(), "/bin/codex")
        self.assertEqual(problem, "")
        self.assertEqual(words, ["/bin/codex", "resume", "01a1f342-0000-7000-8000-000000000001"])

    def test_an_interactive_vector_naming_another_program_session_or_shape_is_refused(self) -> None:
        cases = {
            "another program": self.target(resume_arguments=["/bin/other", "resume",
                                                             "01a1f342-0000-7000-8000-000000000001"]),
            "another session": self.target(resume_arguments=["/bin/codex", "resume",
                                                             "01a1f342-0000-7000-8000-000000000002"]),
            "an extra word": self.target(resume_arguments=["/bin/codex", "resume",
                                                           "01a1f342-0000-7000-8000-000000000001",
                                                           "--last"]),
            "the exec subcommand": self.target(resume_arguments=["/bin/codex", "exec", "resume",
                                                                 "01a1f342-0000-7000-8000-000000000001"]),
            "no vector": self.target(resume_arguments=[]),
        }
        for label, target in cases.items():
            with self.subTest(case=label):
                words, problem = NATIVE_CHECK.read_codex_resume_vector(target, "/bin/codex")
                self.assertEqual(words, [])
                self.assertTrue(problem, f"{label} was accepted")

    def test_the_invocation_record_reads_both_vectors_without_a_turn(self) -> None:
        """The finding-2 rule: the run that asks for no turn reads the vectors here, so a pair that agrees
        with its own rendered lines but names another program is a failed check rather than a passed one.
        The lines' own reading still passes -- they are exactly the words the report's vectors carry --
        which is what makes the vectors' own reading the rule that catches this shape."""
        home = Path("/tmp/home")

        def report_of(program: str) -> dict:
            target = self.target(resume_arguments=[program, "resume",
                                                   "01a1f342-0000-7000-8000-000000000001"],
                                 exec_resume_arguments=[program, "exec", "resume",
                                                        "01a1f342-0000-7000-8000-000000000001",
                                                        NATIVE_CHECK.PROMPT_PLACEHOLDER])
            target["resume_environment"] = {NATIVE_CHECK.CODEX_HOME_ENV: str(home)}
            target["resume_command"] = codex_invocation(home, target["resume_arguments"])
            target["exec_resume_command"] = codex_invocation(home, target["exec_resume_arguments"])
            return target

        with recorded() as results:
            NATIVE_CHECK.record_codex_invocation(report_of("/bin/codex"), "/bin/codex", home)
            self.assertEqual({name: state for name, state, _ in results},
                             {"the printed resume lines name this run's own home and session": "PASS"})
        with recorded() as results:
            NATIVE_CHECK.record_codex_invocation(report_of("/bin/other"), "/bin/codex", home)
            recorded_states = {name: (state, detail) for name, state, detail in results}
        state, detail = recorded_states["the printed resume lines name this run's own home and session"]
        self.assertEqual(state, "FAIL", "a pair naming another program was accepted without a turn")
        self.assertIn("'/bin/other'", detail)


class SourceInvocationReading(unittest.TestCase):
    """The printed Codex line is read the way a shell reads it: the assignment first and unquoted, then the
    words -- a quoted assignment is the *program* to a shell, so nothing sets the variable and the pasted
    command resumes the caller's own home.

    Each case drives both path flavours (``PATH_FLAVOURS``) and builds its line with ``codex_invocation``,
    the quoting the renderer itself applies, so the Windows spelling is exercised on this host too: a
    hand-spelled ``CODEX_HOME=/home/x`` beside ``Path('/home/x')`` reads back as the invocation only where
    ``str(Path(...))`` holds no backslash, which is the ``windows-latest`` job's answer and not this
    one's."""

    #: The home the line resumes, as text: each case spells it with the flavour it drives, so no separator
    #: or drive letter is hard-coded for one host.
    HOME = "/home/x"

    def test_the_right_line_is_read_as_the_invocation(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                home = flavour(self.HOME)
                line = codex_invocation(home, CODEX_RESUME_ARGUMENTS)
                self.assertEqual(NATIVE_CHECK.shell_words(line),
                                 [f"{NATIVE_CHECK.CODEX_HOME_ENV}={home}", *CODEX_RESUME_ARGUMENTS],
                                 "the case's own line is not the invocation's words")
                self.assertTrue(NATIVE_CHECK.reads_as_a_codex_invocation(
                    line, home, CODEX_RESUME_ARGUMENTS), line)

    def test_a_quoted_or_shell_acting_line_is_not_the_invocation(self) -> None:
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                home = flavour(self.HOME)
                # The whole assignment as one single-quoted word, its value spelled bare inside the quotes
                # (a shell reads backslashes literally there, so this is a spelling of the *value*, not a
                # second quoting of it): a shell reads the word as the *program* to run, which is why the
                # word reading below is asserted -- it reads back exactly as the invocation's words do.
                quoted = (f"'{NATIVE_CHECK.CODEX_HOME_ENV}={home}' "
                          + " ".join(shlex.quote(word) for word in CODEX_RESUME_ARGUMENTS))
                refusals = {
                    # A shell reads a quoted assignment as the *program* to run: nothing sets the
                    # variable, whatever the words read back as (which is why the reading is asserted
                    # first). The renderer never emits this spelling; it is the defect the rule is about.
                    "a quoted assignment": (
                        quoted, [f"{NATIVE_CHECK.CODEX_HOME_ENV}={home}", *CODEX_RESUME_ARGUMENTS]),
                    # `;` ends the assignment and starts another command, so the variable keeps only the
                    # part before it and a program of the rest is run.
                    "an unquoted separator": (
                        f"{NATIVE_CHECK.CODEX_HOME_ENV}={home};" + " ".join(CODEX_RESUME_ARGUMENTS), None),
                    # Another home is another store, however the line is spelled.
                    "another home": (codex_invocation(flavour("/home/y"), CODEX_RESUME_ARGUMENTS), None),
                }
                for label, (line, words) in refusals.items():
                    with self.subTest(case=label):
                        if words is not None:
                            self.assertEqual(NATIVE_CHECK.shell_words(line), words, line)
                        self.assertFalse(NATIVE_CHECK.reads_as_a_codex_invocation(
                            line, home, CODEX_RESUME_ARGUMENTS), line)


class SourceTurn(unittest.TestCase):
    """One real turn, run through the report's own vector, recorded as what the harness answered."""

    def setUp(self) -> None:
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.rollout = Path(self.scratch.name) / "rollout.jsonl"
        self.before = [{"type": "session_meta", "payload": {"id": "x"}},
                       {"type": "response_item", "payload": {"type": "message", "role": "user",
                                                             "content": [{"text": "a prompt"}]}}]
        self.rollout.write_text("\n".join(json.dumps(record) for record in self.before) + "\n",
                                encoding="utf-8")
        self.target = {"session_id": "01a1f342-0000-7000-8000-000000000001",
                       "cwd": self.scratch.name,
                       "exec_resume_arguments": ["/bin/codex", "exec", "resume",
                                                 "01a1f342-0000-7000-8000-000000000001",
                                                 NATIVE_CHECK.PROMPT_PLACEHOLDER]}

    def turn(self, answer: str, *, exit_code: int = 0, grow: bool = True) -> None:
        """Drive one turn in-process: the stub harness appends the records a real turn appends (the prompt
        and its answer) and answers with the words the check prints."""
        def fake_deadline(cmd, cwd=None, env=None, timeout=None, replace_env=False):
            if grow:
                with self.rollout.open("a", encoding="utf-8") as handle:
                    handle.write(json.dumps({"type": "response_item",
                                             "payload": {"type": "message", "role": "user",
                                                         "content": [{"text": "a turn"}]}}) + "\n")
                    handle.write(json.dumps({"type": "response_item",
                                             "payload": {"type": "message", "role": "assistant",
                                                         "content": [{"text": answer}]}}) + "\n")
            return completed(stdout=(answer + "\n") if answer else "",
                             stderr="the harness's own diagnostics\n", returncode=exit_code)
        args = mock.Mock(resume_prompt="a turn", expect=("TURN-OK" if answer == "TURN-OK" else None))
        with recorded() as results, \
             mock.patch.object(NATIVE_CHECK, "deadline_run", side_effect=fake_deadline):
            stdout = io.StringIO()
            with contextlib.redirect_stdout(stdout):
                NATIVE_CHECK.run_source_turn(args, self.target, "/bin/codex", Path("/home/x"), self.rollout,
                                             self.before)
            # Read inside the context: the recorded() helper restores the shared list when it leaves.
            self.results = {name: (state, detail) for name, state, detail in results}
            self.states = [state for _, state, _ in results]
        self.printed = stdout.getvalue()

    def test_a_turn_that_answered_is_a_pass_with_the_growth_read(self) -> None:
        self.turn("TURN-OK")
        state, detail = self.results["a real turn continues from the installed session"]
        self.assertEqual(state, "PASS", detail)
        appended, appended_detail = self.results["the turn was appended to the installed session"]
        self.assertEqual(appended, "PASS", appended_detail)
        self.assertIn("2 record(s) before the turn, 4 after", appended_detail)
        self.assertIn("the records the conversion wrote are unchanged", appended_detail)
        self.assertIn(NATIVE_CHECK.TURN_OUTPUT_MARKER, self.printed)
        self.assertIn("TURN-OK", self.printed)

    def test_a_refused_turn_is_a_failed_check_and_never_a_skip(self) -> None:
        """Measured on this host: a Codex turn in a scratch home answers 401 (its credential is the
        caller's own store, and this build never copies one). The harness's own words are printed and the
        check fails -- a requested turn is never a pass and never a skip the verdict ignores."""
        self.turn("", exit_code=1, grow=True)
        state, detail = self.results["a real turn continues from the installed session"]
        self.assertEqual(state, "FAIL", detail)
        self.assertIn("exit 1", detail)
        self.assertIn("the harness's own diagnostics", self.printed)

    def test_a_turn_that_left_the_rollout_the_same_is_not_a_turn(self) -> None:
        self.turn("TURN-OK", grow=False)
        state, _ = self.results["a real turn continues from the installed session"]
        self.assertEqual(state, "FAIL")
        self.assertNotIn("SKIP", self.states,
                         "a requested turn may never be a skip the verdict ignores")

    def test_a_vector_that_is_not_this_turns_runs_nothing(self) -> None:
        self.target["exec_resume_arguments"] = ["/bin/other", "exec", "resume",
                                                "01a1f342-0000-7000-8000-000000000001",
                                                NATIVE_CHECK.PROMPT_PLACEHOLDER]
        called: list[list[str]] = []
        args = mock.Mock(resume_prompt="a turn", expect=None)
        with recorded() as results, \
             mock.patch.object(NATIVE_CHECK, "deadline_run", side_effect=lambda *a, **k: called.append(a) or
                               completed()):
            with contextlib.redirect_stdout(io.StringIO()):
                NATIVE_CHECK.run_source_turn(args, self.target, "/bin/codex", Path("/home/x"), self.rollout,
                                             self.before)
            states = [state for _, state, _ in results]
        self.assertEqual(called, [], "a turn ran on a vector that was not this run's")
        self.assertEqual(states, ["FAIL"])


class SourceDirectionRun(unittest.TestCase):
    """The whole direction driven in-process: the harness's own importer, the CLI conversion, the target's
    loader and the turn are all stubbed, so *the flow itself* is exercised without either harness -- which
    level the conversion is asked for, which commands run in which order, and whether every check the run
    promises is reached.

    ``drive`` returns everything each case asserts on: the exit code, the recorded states, the recorded
    detail lines, the printed output, the commands the run issued and the harness verbs it asked for.
    ``source`` is the document the CLI is pointed at (``--source``): the committed fixture by default, a
    case's own document when it drives a shape the fixture does not hold."""

    def drive(self, document: dict, *, document_path: Path | None = None, reported_codex: str = "/usr/bin/codex",
              probed_codex: str = "/usr/bin/codex", resume_prompt: str | None = "a turn",
              expect: str = "TURN-OK",
              ) -> tuple[int, dict[str, str], dict[str, str], str, list[list[str]], list[list[str]]]:
        """Drive one source-direction run against the stubbed harnesses. ``reported_codex`` and
        ``probed_codex`` are the program word the report's own vectors carry and the harness this check
        probed: they differ for the case about a report naming another program."""
        root = Path(__file__).resolve().parents[2]
        session_id = document["info"]["id"]
        with tempfile.TemporaryDirectory() as scratch_dir:
            scratch = Path(scratch_dir)
            rollout = scratch / "codex-home" / "sessions" / "2026" / "05" / "08" / "rollout-x.jsonl"
            rollout.parent.mkdir(parents=True)
            rollout.write_text("\n".join(json.dumps(record) for record in rollout_records_of(document))
                               + "\n", encoding="utf-8")
            codex_home = scratch / "codex-home"
            conversion = conversion_report_of(document, rollout, codex_home, reported_codex)
            commands: list[list[str]] = []
            verbs: list[list[str]] = []
            store = {"holds": False}

            def fake_ask(opencode, verb, data_root, cwd=None, timeout=None):
                verbs.append(list(verb))
                if verb[1] == "import":
                    store["holds"] = True
                    return completed(stdout=f"Imported session: {session_id}\n")
                if verb[1] == "export":
                    if store["holds"] and verb[2] == session_id:
                        return completed(stdout=json.dumps(document))
                    return completed(stderr=f"Session not found: {verb[2]}\n", returncode=1)
                if verb[1] == "delete":
                    store["holds"] = False
                    return completed(stdout=f"Session {session_id} deleted\n")
                return completed()

            def fake_deadline(cmd, cwd=None, env=None, timeout=None, replace_env=False):
                commands.append(list(cmd))
                if cmd[-1] == "--version":
                    return completed(stdout=cmd[0] + " 9.9.9\n")
                if cmd[0].endswith("opencode") and cmd[1] == "session":
                    return fake_ask(cmd[0], cmd[1:], Path("/data"), cwd=cwd)
                if len(cmd) > 1 and cmd[1] == "build":
                    return completed(stdout="built\n")
                if len(cmd) > 1 and cmd[1] == "continue":
                    return completed(stdout=json.dumps(conversion))
                if len(cmd) > 1 and cmd[1] == "exec":
                    # A real turn appends the prompt it was given and the answer it produced.
                    with rollout.open("a", encoding="utf-8") as handle:
                        handle.write(json.dumps({"type": "response_item",
                                                 "payload": {"type": "message", "role": "user",
                                                             "content": [{"text": resume_prompt}]}})
                                     + "\n")
                        handle.write(json.dumps({"type": "response_item",
                                                 "payload": {"type": "message", "role": "assistant",
                                                             "content": [{"text": "TURN-OK"}]}}) + "\n")
                    return completed(stdout="TURN-OK\n")
                return completed(stdout="\n")

            args = mock.Mock(direction=NATIVE_CHECK.SOURCE_DIRECTION,
                             source=str(document_path) if document_path else None,
                             target_home=None, resume_prompt=resume_prompt, expect=expect,
                             keep=False, json=False)
            with mock.patch.object(NATIVE_CHECK, "BINARY",
                                   scratch / "bin" / "ccc-native-check-opencode"), \
                 mock.patch.object(NATIVE_CHECK, "ask_opencode", side_effect=fake_ask), \
                 mock.patch.object(NATIVE_CHECK, "deadline_run", side_effect=fake_deadline):
                with recorded() as results:
                    stdout = io.StringIO()
                    with contextlib.redirect_stdout(stdout):
                        code = NATIVE_CHECK.run_source_direction(args, root, "go", "/usr/bin/opencode",
                                                                 probed_codex, scratch)
                    states = {name: state for name, state, _ in results}
                    details = {name: detail for name, _, detail in results}
            printed = stdout.getvalue()
        return code, states, details, printed, commands, verbs

    def test_an_opencode_source_converts_through_the_flow_and_asks_for_the_load_level(self) -> None:
        root = Path(__file__).resolve().parents[2]
        document = json.loads((root / NATIVE_CHECK.FIXTURE_ROOT
                               / NATIVE_CHECK.OPENCODE_SOURCE_FIXTURE).read_text(encoding="utf-8"))
        session_id = document["info"]["id"]
        code, states, _, printed, commands, verbs = self.drive(document)

        converted = [command for command in commands if len(command) > 1 and command[1] == "continue"]
        self.assertEqual(len(converted), 1, f"the flow ran {len(converted)} conversions: {commands}")
        self.assertIn("--verify", converted[0])
        self.assertEqual(converted[0][converted[0].index("--verify") + 1], "load",
                         "the conversion must run no turn: the one this check runs is its own")
        self.assertNotIn("--resume-prompt", converted[0])
        self.assertIn(["session", "delete", session_id], verbs,
                      "the run did not remove the session it planted")
        self.assertEqual([state for state in states.values() if state == "FAIL"], [],
                         f"a check failed: {printed[-800:]}")
        self.assertEqual(code, 0, printed[-800:])
        self.assertIn("a real turn continues from the installed session", states)
        self.assertIn(NATIVE_CHECK.TURN_OUTPUT_MARKER, printed)

    def test_a_document_with_a_mixed_record_converts_without_a_false_difference(self) -> None:
        """Finding 1 of the round's review, driven through the flow: an assistant record carrying a text
        part beside a completed tool part is a shape a caller's own ``--source`` document may hold (the
        committed fixture does not). Its text is not established for the read, so a conversion writes the
        call and its result and no message for the text -- and the run has to report that as the conversion
        it is, not as a difference the conversion cannot avoid. The rollout here is what the measured
        writer writes (``rollout_records_of``), so a run that went back to expecting the document's own
        reading for the installed rollout is a failed check, in this case."""
        root = Path(__file__).resolve().parents[2]
        fixture = json.loads((root / NATIVE_CHECK.FIXTURE_ROOT
                              / NATIVE_CHECK.OPENCODE_SOURCE_FIXTURE).read_text(encoding="utf-8"))
        mixed = {"id": "msg_mixed0000000000000000000001", "type": "assistant", "agent": "build",
                 "model": {"id": "example-model", "providerID": "example-proxy"},
                 "time": {"created": 1778231710500, "completed": 1778231710800},
                 "content": [
                     {"type": "text", "text": "Let me check the policy file too."},
                     {"type": "tool", "id": "call_mixed0000000000000000000001", "name": "read",
                      "state": {"status": "completed",
                                "input": {"path": "/home/user/code/example-api/internal/retry/policy.go"},
                                "content": [{"type": "text", "text": "40: const maxAttempts = 4"}]}}]}
        document = {**fixture, "messages": [*(fixture["messages"][:5]), mixed]}
        with tempfile.TemporaryDirectory() as directory:
            document_path = Path(directory) / "mixed-document.json"
            document_path.write_text(json.dumps(document), encoding="utf-8")
            code, states, _, printed, _, _ = self.drive(document, document_path=document_path)

        difference = [name for name, state in states.items() if state == "FAIL"]
        self.assertEqual(difference, [], f"a check failed on a correctly disclosed conversion: {printed[-800:]}")
        self.assertEqual(code, 0, printed[-800:])
        self.assertEqual(
            states["the installed rollout holds what a conversion writes of the fixture's own prompts, "
                   "tool call, result text and texts"], "PASS")

    def test_a_whitespace_only_assistant_record_converts_without_a_false_difference(self) -> None:
        """The r2 round's whitespace finding, driven through the flow: a caller's ``--source`` document may
        hold an assistant record whose text is only whitespace. The reader's own filter drops it, and where
        token usage keeps it the writer's whitespace rule still writes no message -- so the run must expect
        no message for it and report the conversion as the conversion it is. The fabricated rollout here is
        the measured writer's own rule (``rollout_records_of``: no message for the blank record), which is
        what the pre-fix expectation (truthy on any non-empty rendering) would have reported as a missing
        assistant text."""
        root = Path(__file__).resolve().parents[2]
        fixture = json.loads((root / NATIVE_CHECK.FIXTURE_ROOT
                              / NATIVE_CHECK.OPENCODE_SOURCE_FIXTURE).read_text(encoding="utf-8"))
        blank = {"id": "msg_blank00000000000000000000001", "type": "assistant", "agent": "build",
                 "model": {"id": "example-model", "providerID": "example-proxy"},
                 "time": {"created": 1778231710900, "completed": 1778231711000},
                 "content": [{"type": "text", "text": "   "}]}
        document = {**fixture, "messages": [*fixture["messages"], blank]}
        records = rollout_records_of(document)
        self.assertNotIn("   ", [block.get("text") for record in records
                                 for block in (record.get("payload") or {}).get("content") or []
                                 if isinstance(block, dict)],
                         "the fabricated rollout holds a message the writer writes none of")
        with tempfile.TemporaryDirectory() as directory:
            document_path = Path(directory) / "blank-document.json"
            document_path.write_text(json.dumps(document), encoding="utf-8")
            code, states, _, printed, _, _ = self.drive(document, document_path=document_path)

        difference = [name for name, state in states.items() if state == "FAIL"]
        self.assertEqual(difference, [], f"a check failed on a correctly disclosed conversion: {printed[-800:]}")
        self.assertEqual(code, 0, printed[-800:])
        self.assertEqual(
            states["the installed rollout holds what a conversion writes of the fixture's own prompts, "
                   "tool call, result text and texts"], "PASS")

    def test_a_load_only_run_reads_the_printed_vectors_and_fails_on_another_program(self) -> None:
        """Finding 2 of the round's review, driven through the flow: with no ``--resume-prompt`` no turn
        runs, so the report's argument vectors are read by ``record_codex_invocation`` alone -- the
        printed-line checks compare each rendered line with the report's own vector, and a consistently
        wrong pair (another program here) agrees with itself. The run must report that as a failed check and
        end FAIL, not PASS."""
        root = Path(__file__).resolve().parents[2]
        document = json.loads((root / NATIVE_CHECK.FIXTURE_ROOT
                              / NATIVE_CHECK.OPENCODE_SOURCE_FIXTURE).read_text(encoding="utf-8"))
        code, states, details, printed, _, _ = self.drive(document, reported_codex="/usr/bin/other-program",
                                                         resume_prompt=None, expect=None)

        self.assertEqual(
            states["the printed resume lines name this run's own home and session"], "FAIL",
            f"a load-only run accepted vectors naming another program: {printed[-800:]}")
        self.assertIn("'/usr/bin/other-program'",
                      details["the printed resume lines name this run's own home and session"])
        self.assertEqual(code, 1, printed[-800:])
        self.assertIn("a real turn was requested (--resume-prompt)", states)


def rollout_records_of(document: dict) -> list[dict]:
    """A Codex rollout's records for one OpenCode document, in the shapes the comparison reads: what a
    conversion writes (``opencode_written_evidence``). Used by the driven cases above, which need a target
    file without a conversion.

    Every membership is the comparison's own rule, read through the same functions (``opencode_user_text``
    and ``opencode_trimmed``, ``opencode_reader_text``, ``opencode_tool_state_text``): a user record's
    rendered text and a tool result contribute a record only where the text survives the reader's own
    ``strings.TrimSpace``, and a whitespace-only assistant rendering is not written (the writer's
    ``whitespace_only_dialogue``), because the target's own reader would read a blank message as no message.

    A record that carries a tool call contributes its ``function_call`` and, where the recorded text is one
    the writers write, its ``function_call_output`` -- and *no* assistant message, whatever text parts the
    document gives it: the read establishes no dialogue for a record carrying calls, so no writer writes
    its text (measured on the writer; see the comparison's own case)."""
    records = [{"type": "session_meta", "payload": {"id": "x"}}]
    for message in document.get("messages") or []:
        if message.get("type") == "user":
            text = NATIVE_CHECK.opencode_user_text(message)
            if NATIVE_CHECK.opencode_trimmed(text):
                records.append({"type": "response_item", "payload": {
                    "type": "message", "role": "user", "content": [{"text": text}]}})
        elif message.get("type") == "assistant":
            carries_call = False
            for part in message.get("content") or []:
                if part.get("type") == "tool":
                    carries_call = True
                    records.append({"type": "response_item", "payload": {
                        "type": "function_call", "name": part.get("name"),
                        "arguments": NATIVE_CHECK.opencode_tool_call_input(part),
                        "call_id": part.get("id")}})
                    text = NATIVE_CHECK.opencode_tool_state_text(part)
                    if NATIVE_CHECK.opencode_trimmed(text):
                        records.append({"type": "response_item", "payload": {
                            "type": "function_call_output", "call_id": part.get("id"),
                            "output": text}})
            rendering = NATIVE_CHECK.opencode_reader_text(message)
            if NATIVE_CHECK.opencode_trimmed(rendering) and not carries_call:
                records.append({"type": "response_item", "payload": {
                    "type": "message", "role": "assistant", "content": [{"text": rendering}]}})
    return records


def conversion_report_of(document: dict, rollout: Path, codex_home: Path, codex: str) -> dict:
    """The report a `ccc continue … --json` run writes for one OpenCode source, as the driven case's stub:
    the states and the vectors the flow reads, with the target path pointing at the case's own file. The
    vectors name this run's own harness and home, because the flow reads them against exactly that, and the
    two printed lines are rendered the way the route's own renderer spells them (``codex_invocation``):
    the case's own home is a real directory, whose spelling a POSIX shell would act on wherever it holds a
    backslash (Windows), so a hand-spelled line would hand the flow's own reading a command line the route
    never prints."""
    session_id = document["info"]["id"]
    home = str(codex_home)
    resume_arguments = [codex, "resume", "01a1f342-0000-7000-8000-000000000001"]
    exec_resume_arguments = [codex, "exec", "resume", "01a1f342-0000-7000-8000-000000000001",
                             NATIVE_CHECK.PROMPT_PLACEHOLDER]
    return {
        "ref": {"Provider": "opencode", "ID": session_id},
        "target": "codex",
        "source": {"identity": {"provider": "opencode", "agent": "OpenCode", "session_id":
                                f"opencode:{session_id}", "source_path": f"<store>#{session_id}"},
                   "counts": {"messages": 4, "tool_calls": 1, "tool_results": 1},
                   "boundary": {"state": "not-taken",
                                "detail": f"the source read resolved member {session_id} inside the "
                                          f"container <store>/opencode.db, and no file holds this member's "
                                          f"own bytes to compare"}},
        "conversion": {"state": "converted", "format": "codex-rollout-jsonl", "read_from": "source",
                       "carried": {"messages": 4, "tool_calls": 1, "tool_results": 1},
                       "losses": [{"category": "unprojected_dialogue", "disposition":
                                   "carried-with-caveat", "count": 4, "detail": "…"}]},
        "installation": {"state": "installed", "path": str(rollout), "mechanism": "hard-link",
                         "source_check_state": "not-verified", "detail": "no file holds this member"},
        "target_session": {"provider": "codex", "session_id": "01a1f342-0000-7000-8000-000000000001",
                           "path": str(rollout), "cwd": str(rollout.parent),
                           "resume_arguments": resume_arguments,
                           "exec_resume_arguments": exec_resume_arguments,
                           "resume_environment": {NATIVE_CHECK.CODEX_HOME_ENV: home},
                           "resume_command": codex_invocation(codex_home, resume_arguments),
                           "exec_resume_command": codex_invocation(codex_home, exec_resume_arguments)},
        "verification": {"structure": {"state": "verified", "detail": "read back"},
                         "load": {"state": "verified", "detail": "the harness's loader answered"}},
    }


if __name__ == "__main__":
    unittest.main()
