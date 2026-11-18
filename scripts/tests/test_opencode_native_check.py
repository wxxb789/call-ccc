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
  ``native_check_common.TURN_OUTPUT_MARKER`` is the harness's own bytes, not a summary of them;
* the reading of the harness's own answers: a refusal that does not name the id it refused is not the
  control arm, a listing the harness failed to answer is not an empty listing, and a document the harness
  did not parse is not a document;
* the planted session: the document carries the members the harness's own importer was measured to
  require, ``already exists`` is not a planting, and the removal is recorded as its own check -- only when
  this run planted something.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path
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
        """The defect this rule exists for, driven through the turn the resume stage runs: the flag after
        ``--`` is the prompt's, and a turn whose records a service holds elsewhere is not this run's."""
        calls: list[list[str]] = []

        def fake_run(command, cwd=None, env=None, timeout=None):
            calls.append(list(command))
            return completed(stdout=ANSWER)

        args = mock.Mock(resume_prompt="say hello", expect=None)
        target = {"session_id": SESSION}
        with mock.patch.object(NATIVE_CHECK, "run", side_effect=fake_run), \
             mock.patch.object(NATIVE_CHECK, "export_document",
                               side_effect=[(document(3), "ok", 0),
                                            (document(6, answer=ANSWER), "ok", 0)]):
            with recorded():
                NATIVE_CHECK._run_turn(args, target, "opencode", Path("/data"), Path("/workspace"))
        self.assertEqual(len(calls), 1)
        turn = calls[0]
        self.assertIn("--standalone", turn)
        self.assertLess(turn.index("--standalone"), turn.index("--"),
                        f"the flag is on the prompt's side of the terminator: {turn}")
        self.assertLess(turn.index("--standalone"), turn.index("--session"))
        self.assertEqual(turn[-1], "say hello")


class ResumeStage(unittest.TestCase):
    """One real turn is a resumed session only when the harness's own reader reads the store differently
    afterwards, and the answer printed is the harness's own."""

    def drive(self, before: dict, after: dict, stdout: str = ANSWER) -> list[tuple[str, str, str]]:
        args = mock.Mock(resume_prompt="a turn", expect=ANSWER)
        target = {"session_id": SESSION}
        with mock.patch.object(NATIVE_CHECK, "run", return_value=completed(stdout=stdout)), \
             mock.patch.object(NATIVE_CHECK, "export_document",
                               side_effect=[(before, "ok", 0), (after, "ok", 0)]):
            with recorded() as results:
                NATIVE_CHECK._run_turn(args, target, "opencode", Path("/data"), Path("/workspace"))
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


if __name__ == "__main__":
    unittest.main()
