#!/usr/bin/env python3
"""Regression tests for the checks in scripts/codex-native-check.py that need no Codex binary.

Two halves, both driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the check that reads the reported resume invocation back the way a POSIX shell reads it
(``shell_words``/``reported_invocation``), and the ordering rule that the source session and the planted
session are compared **after** the harness's own native stages (``main`` driven with its subprocess and
app-server seams replaced by stubs). A third subject is driven the same way: the app server's diagnostics
drainer, whose retained lines are bounded (``new_diagnostics``/``drain_diagnostics``).

The resume-invocation check is the harness's own rule about the line a caller pastes or an agent runs: it
must name the data directory the session was installed into, because Codex takes that directory from
CODEX_HOME alone and has no flag for it. The line is *rendered* for a POSIX shell, so a check written
against a literal
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

Every case drives **both** ``pathlib`` flavours a host can hand the shipped check: this host's ``Path``
and ``PureWindowsPath`` -- what ``Path`` *is* on Windows. That, and nothing about the host running this
suite, is what keeps the fixtures honest. An earlier version of these cases spelled their homes
``/tmp/…`` and literals to match, so they were green here and failed in the ``windows-latest`` job:
``Path('/tmp/codex home/.codex')`` reads back as ``\\tmp\\codex home\\.codex`` there, and the literal
rendering and the comparison therefore disagreed on that host alone. A home in ``RENDERINGS`` is
spelled so both flavours give it back unchanged (a Windows-shaped path, or a single-segment name),
because the rule under test is about the *characters* of a word -- a space, a backslash, an embedded
quote -- never about the separator this host happens to use.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import shlex
import shutil
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path, PureWindowsPath
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]

#: A session id and binary the cases use: the check compares words, so their values only have to be
#: spelled the same on both sides.
SESSION = "01a1e225-a986-7372-bf0f-2c51da684752"
OTHER_SESSION = "00000000-0000-0000-0000-000000000000"
CODEX = "codex"

RESUME_ARGUMENTS = [CODEX, "resume", SESSION]
EXEC_RESUME_ARGUMENTS = [CODEX, "exec", "resume", SESSION, "<prompt>"]

#: ``(home, the word a shell reads it back as, the assignment `internal/ccc/continue.go`'s `shellQuote`
#: renders for it)``. A word holding nothing a shell would act on stands bare, and one holding a space,
#: a backslash or a quote is single-quoted with an embedded quote closed and re-opened (``'it'\''s``).
#: ``codex-home`` and ``codex home`` are single-segment names so ``pathlib`` spells them the same on
#: every host; the Windows-shaped path is a path because that is the shape the quoting was written for.
RENDERINGS = (
    ("codex-home", "CODEX_HOME=codex-home", "CODEX_HOME=codex-home"),
    ("codex home", "CODEX_HOME=codex home", "CODEX_HOME='codex home'"),
    (r"C:\Users\a b\.codex", r"CODEX_HOME=C:\Users\a b\.codex", r"CODEX_HOME='C:\Users\a b\.codex'"),
    ("it's home", "CODEX_HOME=it's home", "CODEX_HOME='it'\\''s home'"),
)

#: The path flavours a case drives the check with: this host's own, and Windows' answer on any host.
PATH_FLAVOURS = (Path, PureWindowsPath)


def load_native_check():
    """The shipped harness, imported by path (its file name is not an identifier)."""
    spec = importlib.util.spec_from_file_location(
        "shipped_codex_native_check", SCRIPTS / "codex-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


class ResumeInvocationCheck(unittest.TestCase):
    def assert_rejected(self, line: str, home: str, arguments: list[str],
                        words: list[str]) -> None:
        """``line`` is not the invocation, and the shell's own reading says why.

        The reading is asserted first so a rejection cannot come from a spelling accident: the case
        names the words a shell reads (an unquoted home split in two, another home, another session, no
        assignment at all), and ``reported_invocation`` has to answer ``False`` for that reason under
        both path flavours.
        """
        self.assertEqual(NATIVE_CHECK.shell_words(line), words, line)
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, flavour(home), arguments), line)

    def test_accepts_the_rendering_the_renderer_emits(self) -> None:
        """Every rendering reads back as the assignment plus the argument vector, on both flavours."""
        for home, word, rendering in RENDERINGS:
            for flavour in PATH_FLAVOURS:
                with self.subTest(home=home, flavour=flavour.__name__):
                    resume = f"{rendering} {CODEX} resume {SESSION}"
                    exec_resume = f"{rendering} {CODEX} exec resume {SESSION} '<prompt>'"
                    self.assertEqual(NATIVE_CHECK.shell_words(resume), [word, *RESUME_ARGUMENTS], resume)
                    self.assertEqual(NATIVE_CHECK.shell_words(exec_resume),
                                     [word, *EXEC_RESUME_ARGUMENTS], exec_resume)
                    self.assertTrue(
                        NATIVE_CHECK.reported_invocation(resume, flavour(home), RESUME_ARGUMENTS),
                        f"a correct rendering was rejected: {resume!r}")
                    self.assertTrue(
                        NATIVE_CHECK.reported_invocation(exec_resume, flavour(home),
                                                         EXEC_RESUME_ARGUMENTS),
                        f"a correct rendering was rejected: {exec_resume!r}")

    def test_rejects_an_unquoted_home_a_shell_would_split(self) -> None:
        # The defect the quoted rendering prevents: a shell reads the space as the end of the assignment
        # word, so the line opens a home named `codex` instead of the one that was installed into.
        # The substring form of this check accepted exactly this line -- hence the reading asserted first.
        self.assert_rejected(
            f"CODEX_HOME=codex home {CODEX} resume {SESSION}", "codex home", RESUME_ARGUMENTS,
            ["CODEX_HOME=codex", "home", CODEX, "resume", SESSION])

    def test_rejects_a_command_that_names_another_home(self) -> None:
        self.assert_rejected(
            f"CODEX_HOME=codex-elsewhere {CODEX} resume {SESSION}", "codex-home", RESUME_ARGUMENTS,
            ["CODEX_HOME=codex-elsewhere", CODEX, "resume", SESSION])

    def test_rejects_a_command_without_the_assignment(self) -> None:
        self.assert_rejected(
            f"{CODEX} resume {SESSION}", "codex-home", RESUME_ARGUMENTS, [CODEX, "resume", SESSION])

    def test_rejects_a_command_for_another_session(self) -> None:
        self.assert_rejected(
            f"CODEX_HOME=codex-home {CODEX} resume {OTHER_SESSION}", "codex-home", RESUME_ARGUMENTS,
            ["CODEX_HOME=codex-home", CODEX, "resume", OTHER_SESSION])

    def test_rejects_a_line_a_shell_cannot_read(self) -> None:
        # An unbalanced quote is not a command line at all: a shell would keep reading, so the reported
        # invocation could not be the one this check is about. This reading does not meet a home, so
        # there is nothing for a path flavour to change about it.
        line = f"CODEX_HOME='codex-home {CODEX} resume {SESSION}"
        self.assertIsNone(NATIVE_CHECK.shell_words(line))
        self.assertFalse(NATIVE_CHECK.reported_invocation(line, Path("codex-home"), RESUME_ARGUMENTS))


class BoundedAppServerDiagnostics(unittest.TestCase):
    """The app server's diagnostics are a bounded tail, not a log this harness keeps for the whole run.

    The drainer has to keep reading the pipe -- a server whose diagnostics nobody reads blocks instead of
    answering -- but what it *retains* is a ring of the last lines: the failure reports quote that tail
    (``AppServer.stderr_tail``), and a runaway log must not become this harness's own memory. Both halves
    are driven through the shipped sink and the shipped drainer, so removing the bound fails here instead
    of in a review, and the cases keep the two properties the drainer had before it was bounded: a stream
    that fails mid-read keeps the lines that arrived, and the reported tail is the *end* of the stream.
    """

    def test_retains_only_the_bounded_tail(self) -> None:
        retained = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics((f"diagnostic {index}\n" for index in range(5000)), retained)
        self.assertLessEqual(len(retained), 1000,
                             f"the drainer retained {len(retained)} lines of a 5000-line stream: the "
                             f"diagnostics are a tail, not a log")
        self.assertEqual(len(retained), NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_MAX,
                         "the shipped bound is not what bounded the sink")
        self.assertEqual(list(retained)[-1], "diagnostic 4999",
                         "the retained lines are not the end of the stream")
        self.assertEqual(list(retained)[0],
                         f"diagnostic {5000 - NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_MAX}",
                         "the retained lines do not begin where the bound says they should")

    def test_keeps_the_lines_that_arrived_when_the_stream_ends_badly(self) -> None:
        def broken():
            yield "the server's last words\n"
            raise OSError("the pipe closed mid-line")

        retained = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics(broken(), retained)
        self.assertEqual(list(retained), ["the server's last words"],
                         "a stream that failed mid-read lost the diagnostics that did arrive")

    def test_the_reported_tail_is_the_end_of_what_was_read(self) -> None:
        # The instance is built without its process: the rule is about the sink the drainer fills, and
        # the tail is read out of a ring, which takes no slice of its own.
        server = NATIVE_CHECK.AppServer.__new__(NATIVE_CHECK.AppServer)
        server.diagnostics = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics([f"line {index}\n" for index in range(10)], server.diagnostics)
        self.assertEqual(server.stderr_tail(), "line 7 | line 8 | line 9")


#: A host configuration the seeding case drives the check with: one credential in each shape a real
#: configuration can hold it -- a provider's authorization header written out, a query parameter carrying
#: a token, another provider's definition, and an MCP server's environment table. The values are
#: sanitized fixtures (no real token, no prefix any secret scanner looks for), which is all the case
#: needs: the rule is about the *name* of the setting, so a copied value is a copied value whatever it
#: says.
HOST_TOKENS = {
    "header": "a-fixture-header-token",
    "query": "a-fixture-query-token",
    "other": "a-fixture-other-provider-token",
    "mcp": "a-fixture-mcp-environment-token",
    "url_user": "a-fixture-url-user",
    "url_password": "a-fixture-url-password",
}

HOST_CONFIGURATION = f"""\
model = "a-host-model"
model_provider = "a-proxy"
model_reasoning_effort = "high"

[model_providers.a-proxy]
name = "A Proxy"
base_url = "https://proxy.example/v1"
wire_api = "responses"
env_key = "A_PROXY_KEY"
http_headers = {{ Authorization = "Bearer {HOST_TOKENS['header']}" }}
query_params = {{ token = "{HOST_TOKENS['query']}" }}

[model_providers.other]
name = "Other"
base_url = "https://other.example/v1"
http_headers = {{ Authorization = "Bearer {HOST_TOKENS['other']}" }}

[mcp_servers.a-server]
command = "a-server"
env = {{ A_TOKEN = "{HOST_TOKENS['mcp']}" }}

[projects."/a/project"]
trust_level = "trusted"
"""


class SeededHostConfiguration(unittest.TestCase):
    """The throw-away Codex home is seeded with settings, never with the host's configuration file.

    The rule this class pins: a setting whose value can hold a credential -- a provider's authorization
    header, a query parameter, another provider's definition, an MCP server's environment table -- never
    crosses into the throw-away home, and the report line says what was copied and what was left behind
    instead of claiming that nothing credential-bearing was copied while copying everything. Both halves
    are checked against the *bytes* the home receives (by reading them back as TOML) and against the
    report, because a sanitizer whose report is wrong states something untrue either way.
    """

    def seeded(self, text: str = HOST_CONFIGURATION, **overrides: str) -> tuple[dict, str, str]:
        """One seeding: the document the home receives, parsed back, its bytes, and the report line."""
        seeding = NATIVE_CHECK.seed_configuration(text, dict(overrides))
        document = tomllib.loads(seeding.text) if seeding.text else {}
        return document, seeding.text, seeding.report()

    def assert_no_credential(self, rendered: str, line: str) -> None:
        """No fixture token is in the bytes the home receives, or in the report about them."""
        for name, token in HOST_TOKENS.items():
            with self.subTest(credential=name):
                self.assertNotIn(token, rendered, f"the {name} credential was copied into the home")
                self.assertNotIn(token, line, f"the {name} credential is quoted in the report")

    def test_copies_only_the_settings_that_cannot_hold_a_credential(self) -> None:
        document, rendered, report = self.seeded()
        self.assertEqual(document, {
            "model": "a-host-model",
            "model_provider": "a-proxy",
            "model_providers": {"a-proxy": {
                "name": "A Proxy", "base_url": "https://proxy.example/v1",
                "wire_api": "responses", "env_key": "A_PROXY_KEY"}},
        }, f"the seeded document is not the settings that cannot hold a credential: {rendered!r}")
        self.assert_no_credential(rendered, report)
        # The report names both halves: what was copied, and every host setting that was left behind --
        # including the ones the case above removed from the document, by name and not by value.
        self.assertIn("copied model, model_provider, "
                      "[model_providers.a-proxy].name, [model_providers.a-proxy].base_url, "
                      "[model_providers.a-proxy].wire_api, [model_providers.a-proxy].env_key", report)
        for left in ("[model_providers.a-proxy].http_headers", "[model_providers.a-proxy].query_params",
                     "[model_providers.other]", "[mcp_servers]", "[projects]", "model_reasoning_effort"):
            with self.subTest(left_behind=left):
                self.assertIn(left, report, report)

    def test_does_not_copy_a_provider_whose_url_holds_a_credential(self) -> None:
        host = HOST_CONFIGURATION.replace(
            "https://proxy.example/v1",
            f"https://{HOST_TOKENS['url_user']}:{HOST_TOKENS['url_password']}@proxy.example/v1")
        document, rendered, report = self.seeded(host)
        self.assertNotIn("model_providers", document,
                         f"a provider definition holding a credential was copied: {rendered!r}")
        self.assert_no_credential(rendered, report)
        self.assertIn("[model_providers.a-proxy]", report,
                      "the report does not name the provider definition it left behind")

    def test_the_command_line_still_decides_what_the_home_answers_with(self) -> None:
        document, rendered, report = self.seeded(model_provider="other", model="a-named-model")
        self.assertEqual(document.get("model"), "a-named-model")
        self.assertEqual(document.get("model_provider"), "other")
        # The definition copied is the one the home now names, and its own credential-bearing settings
        # are still left behind.
        self.assertEqual(document.get("model_providers"),
                         {"other": {"name": "Other", "base_url": "https://other.example/v1"}},
                         f"the seeded document does not hold the named provider's definition: {rendered!r}")
        self.assert_no_credential(rendered, report)
        self.assertIn("[model_providers.other].http_headers", report, report)

    def test_a_configuration_it_cannot_read_is_not_copied(self) -> None:
        document, rendered, report = self.seeded("[this is not toml")
        self.assertEqual((document, rendered), ({}, ""),
                         "a configuration the check cannot read was copied anyway")
        self.assertIn("not readable as TOML", report, report)
        # The command line is still honoured: it is this check's own answer, not the host's.
        document, rendered, _ = self.seeded("[this is not toml", model="a-named-model")
        self.assertEqual(document, {"model": "a-named-model"})

    def test_a_configuration_with_nothing_to_seed_says_so(self) -> None:
        document, rendered, report = self.seeded("[mcp_servers.a-server]\ncommand = \"a-server\"\n")
        self.assertEqual((document, rendered), ({}, ""))
        self.assertTrue(report.startswith("nothing was copied"), report)
        self.assertIn("[mcp_servers]", report, "the report does not name the table it left behind")


#: The checks the ordering rule below is about, named as the harness records them.
PROTECTED_CHECKS = ("the source session is unchanged",
                    "the unrelated session in the target home is unchanged")


class ProtectedArtifactsAfterNativeVerification(unittest.TestCase):
    """The source session and the planted session are compared again after the harness's own native
    stages, not before them.

    The native half cannot run offline -- it needs the Codex binary and, for a turn, model access -- so
    these cases drive the shipped ``main`` with the seams that half stands on replaced: the subprocess
    runner, the app server and the harness's own view call, plus ``shutil.which`` for the two tools. The
    stub for ``ccc continue`` installs a real file into a throw-away home, and the stub for the resume
    turn answers like a turn that continued (the installed file grows) while it changes both protected
    artifacts. With the comparisons at the end of the run, the mutating case must FAIL exactly those two
    checks and PASS the turn; if the comparisons move back above the loader and the turn -- the defect
    this pins -- they answer on the pre-mutation state, so the mutating case below fails over an empty
    failure list while the intact control still passes: the ordering defect breaks the catching, not the
    passing. The intact run is the control for the other direction -- a comparison that answered
    FAIL for its own reasons could not pass it -- and the mutating run asserts the two protected checks
    are its **only** failures, so nothing else was already red.
    """

    def drive(self, mutate_during_resume: bool) -> tuple[int, dict[str, tuple[str, str]]]:
        """One run of the shipped ``main`` against stubs; answers its exit code and its recorded checks."""
        # The scratch root is resolved before anything is derived from it, because the shipped ``main``
        # resolves what it is handed (``--repo`` always, and ``--codex-home`` when the caller names one,
        # which this case does): a host whose temporary root is reached through a symlink, or whose
        # ``resolve()`` answers a canonical spelling of it -- macOS spells the temp root
        # ``/var/folders/…`` and reads it back as ``/private/var/folders/…`` -- would leave this case's
        # stub answering in a spelling the harness never uses, so the stubbed ``--repo``/``--target-home``
        # commands, the document's own home and the run's recorded checks would all disagree with it on
        # that host alone, and did: this is the fixture defect head ``4cd0567``'s ``macos-latest`` leg
        # broke on. Deriving the tree from the resolved spelling makes this case's paths the harness's
        # own on every host.
        scratch = Path(tempfile.mkdtemp(prefix="ccc-native-check-case-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        repo = scratch / "repo"
        fixture = (repo / NATIVE_CHECK.FIXTURE_ROOT / NATIVE_CHECK.SOURCE_PROVIDER
                   / "-fake-project" / f"{NATIVE_CHECK.SOURCE_SESSION}.jsonl")
        fixture.parent.mkdir(parents=True)
        fixture.write_text(json.dumps({"type": "user"}) + "\n")
        home = scratch / "codex-home"
        home.mkdir()
        # The two tools the harness resolves: names it only compares and runs through the stub below,
        # so no binary has to exist for this case.
        codex = str(scratch / "codex")
        go = str(scratch / "go")

        thread_id = "01a1e000-0000-7000-8000-000000000002"
        installed = (home / "sessions" / "2026" / "05" / "04"
                     / f"rollout-2026-05-04T09-15-00-{thread_id}.jsonl")
        planted = home / "sessions" / "2026" / "01" / "01" / NATIVE_CHECK.UNRELATED_NAME
        # The two reported command lines are rendered the way the conversion's own renderer spells one
        # (`internal/ccc/continue.go`'s ``renderResumeCommand``/``shellQuote``: the environment's value,
        # then every argument word, quoted where a POSIX shell would act on it), because the harness
        # reads them the way a shell does. Joining the argument words bare left a Windows binary path's
        # backslashes to that reading -- the host spells the stub ``C:\…\codex`` and the shell reads it
        # back as ``C:…codex`` -- so the check that answers PASS for a correctly rendered line answered
        # FAIL for this fixture: the ``windows-latest`` shape head ``1a476e0`` failed, with these two
        # cases breaking the same way the resolved-root spelling above did on ``macos-latest``.
        assignment = "CODEX_HOME=" + shlex.quote(str(home))

        def rendered(words: list[str]) -> str:
            """The words as the renderer under test spells them: each quoted where a shell acts."""
            return " ".join(shlex.quote(word) for word in words)

        resume_arguments = [codex, "resume", thread_id]
        exec_resume_arguments = [codex, "exec", "resume", thread_id, "<prompt>"]
        document = {
            "conversion": {
                "state": "converted", "format": "codex-rollout-jsonl",
                "carried": {"messages": 3, "user_messages": 1, "tool_calls": 1, "tool_results": 1,
                            "compaction_boundaries": 0},
                "losses": [{"category": "reasoning", "disposition": "omitted", "count": 1,
                            "detail": "the stub's one loss"}],
            },
            "installation": {"state": "installed", "path": str(installed), "mechanism": "hard-link",
                             "created": True, "source_check_state": "unchanged"},
            "target_session": {
                "session_id": thread_id, "path": str(installed), "binary_found": True,
                "resume_arguments": resume_arguments, "exec_resume_arguments": exec_resume_arguments,
                "resume_environment": {"CODEX_HOME": str(home)},
                "resume_command": f"{assignment} {rendered(resume_arguments)}",
                "exec_resume_command": f"{assignment} {rendered(exec_resume_arguments)}",
            },
            "verification": {"load": {"state": "verified"}, "resume": {"state": "verified"}},
        }

        def fake_run(cmd, cwd=None, env=None, timeout=600):
            command = [str(part) for part in cmd]
            if command[0] == go:
                return subprocess.CompletedProcess(cmd, 0, "", "")
            if command[0] == str(repo / NATIVE_CHECK.BINARY):
                if command[1] == "continue":
                    installed.parent.mkdir(parents=True, exist_ok=True)
                    installed.write_text(json.dumps({"type": "session_meta"}) + "\n")
                    return subprocess.CompletedProcess(cmd, 0, json.dumps(document), "")
                if command[1] == "inspect":
                    counts = {"user_messages": 1, "tool_calls": 1, "tool_results": 1,
                              "compact_boundaries": 0}
                    return subprocess.CompletedProcess(
                        cmd, 0, json.dumps({"source": {"counts": counts}}), "")
            if command[0] == codex and command[1] == "exec":
                # The resume stage: a turn that continued (the installed file grows) and, when this case
                # asks for it, the mutation both protected artifacts have to survive.
                installed.write_text(installed.read_text() + json.dumps({"type": "turn"}) + "\n")
                if mutate_during_resume:
                    fixture.write_text(fixture.read_text() + json.dumps({"type": "assistant"}) + "\n")
                    planted.write_text(planted.read_text() + json.dumps({"type": "session_meta"}) + "\n")
                return subprocess.CompletedProcess(cmd, 0, "the stub turn answered\n", "")
            raise AssertionError(f"the case's stub was asked for an unexpected command: {command}")

        class FakeAppServer:
            """The app server this case never starts: it answers the loader with the installed file."""

            def __init__(self, binary, home):
                self.binary, self.home = binary, home

            def handshake(self) -> None:
                pass

            def call(self, method: str, params: dict) -> dict:
                if method != "thread/resume":
                    raise AssertionError(f"the case's stub was asked for {method}")
                return {"thread": {"id": thread_id, "path": str(installed), "historyMode": "legacy",
                                   "cwd": "/tmp"}}

            def close(self) -> None:
                pass

        def fake_thread_view(server, thread):
            return [f"thread {thread} (the case's stub's own view)"], ["userMessage", "agentMessage"]

        def fake_which(name, *args, **kwargs):
            return codex if name == "codex" else go

        before = len(NATIVE_CHECK.RESULTS)
        with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                mock.patch.object(NATIVE_CHECK, "AppServer", FakeAppServer), \
                mock.patch.object(NATIVE_CHECK, "thread_view", fake_thread_view), \
                mock.patch("shutil.which", fake_which), \
                contextlib.redirect_stdout(io.StringIO()):
            code = NATIVE_CHECK.main(["--repo", str(repo), "--codex", "codex",
                                      "--codex-home", str(home),
                                      "--resume-prompt", "answer only from this conversation"])
        results = {name: (state, detail) for name, state, detail in NATIVE_CHECK.RESULTS[before:]}
        del NATIVE_CHECK.RESULTS[before:]
        return code, results

    def test_a_mutation_during_the_resume_stage_is_caught_by_the_last_comparison(self) -> None:
        code, results = self.drive(mutate_during_resume=True)
        self.assertEqual(results.get("a real turn continues from the installed session", ("", ""))[0],
                         "PASS",
                         f"the stub's turn did not run, so its mutation would not be what the "
                         f"comparisons catch: {results}")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, sorted(PROTECTED_CHECKS),
                         f"the mutating run's failures are not exactly the two protected-artifact "
                         f"comparisons: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with failed checks: {results}")
        self.assertIn("the fixture tree changed:", results[PROTECTED_CHECKS[0]][1])
        self.assertIn(" before, ", results[PROTECTED_CHECKS[1]][1])
        self.assertIn(" after", results[PROTECTED_CHECKS[1]][1])

    def test_an_intact_run_passes_the_comparisons(self) -> None:
        code, results = self.drive(mutate_during_resume=False)
        for name in PROTECTED_CHECKS:
            state, detail = results.get(name, ("missing", ""))
            self.assertEqual(state, "PASS", f"{name}: {state} — {detail}; all results: {results}")
        self.assertEqual(code, 0, f"exit {code} for an intact run: {results}")


if __name__ == "__main__":
    unittest.main()
