#!/usr/bin/env python3
"""Regression tests for the checks in scripts/codex-native-check.py that need no Codex binary.

Two halves, both driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the check that reads the reported resume invocation back the way a POSIX shell reads it
(``shell_words`` -- what its words are -- with ``unquoted_text``/``reported_invocation`` for the spelling
that decides whether a shell reads the line as the assignment at all), and the ordering rule that the
source session and the planted session are compared **after** the harness's own native stages (``main``
driven with its subprocess and app-server seams replaced by stubs). A third subject is driven the same
way: the app server's diagnostics drainer, whose retained lines are bounded
(``new_diagnostics``/``drain_diagnostics``).

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

One case reads beyond this module's subject: the evidence record's claim about *when* the comparison taken
where the copy grows runs is read against the positions of the shipped script's own calls
(``RecordStatesTheChecksOwnOrder``) in the same file, so a record that drifts back to the placement the
review found fails here instead of being re-read as a claim nobody checks.

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
import os
import shlex
import shutil
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path, PureWindowsPath
from typing import Callable, NamedTuple
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
    """The shipped harness, imported by path (its file name is not an identifier).

    An imported-from-source module is served from bytecode when the cache validates, and this loader's
    cache is validated by the source's mtime in *whole seconds* and its size -- so a mutation study that
    preserves the file's size (which moving a block does) and rewrites it within the same second as the
    previous revision can have the previous revision's code executed. Measured, not assumed: the growth
    baseline moved up before the first turn, then the closing comparisons moved up before the second turn
    -- both within the same second -- and the second run answered the *first* mutation's failure set.
    Every mutation run quoted in `docs/evidence/0004-repeatable-continuation-handoffs.md` used a fresh
    ``PYTHONPYCACHEPREFIX`` per run.
    """
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

    def test_rejects_an_assignment_a_shell_reads_as_a_command(self) -> None:
        """The words read back right and the line is still not the invocation.

        A shell recognizes a leading assignment only while nothing in its name or its `=` is quoted, so
        each spelling below is the *program* to run -- one named `CODEX_HOME=codex-home`, or one whose
        name a shell would spell that way only after removing the quoting -- and nothing sets the data
        directory. The word reading is asserted equal to the very words a correct rendering reads as,
        which is why this case cannot be left to that reading: the quoting is the whole difference.
        """
        assignment = "CODEX_HOME=codex-home"
        for line in (
            f"'{assignment}' {CODEX} resume {SESSION}",
            f'"{assignment}" {CODEX} resume {SESSION}',
            f"CO'DEX_HOME=codex-home' {CODEX} resume {SESSION}",
            f"CODEX_HOME\\=codex-home {CODEX} resume {SESSION}",
        ):
            with self.subTest(line=line):
                self.assertEqual(NATIVE_CHECK.shell_words(line), [assignment, *RESUME_ARGUMENTS], line)
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertFalse(
                            NATIVE_CHECK.reported_invocation(line, flavour("codex-home"),
                                                             RESUME_ARGUMENTS), line)

    def test_rejects_syntax_the_value_leaves_unquoted(self) -> None:
        """A value the renderer quotes, spelled bare: the words read back right and the line is another
        command line -- `;` ends the assignment (`CODEX_HOME=a`) and starts a program named `b`, `$HOME`
        is substituted before the variable is set, a backquote is run, `~` expands. The rule is about
        spellings: a character a shell acts on has to be quoted, which `shellQuote` does for every one.
        (Each value is a single segment, so both path flavours spell it as the line does.)
        """
        for line, home in (
            (f"CODEX_HOME=a;b {CODEX} resume {SESSION}", "a;b"),
            (f"CODEX_HOME=$HOME {CODEX} resume {SESSION}", "$HOME"),
            (f"CODEX_HOME=`id` {CODEX} resume {SESSION}", "`id`"),
            (f"CODEX_HOME=~ {CODEX} resume {SESSION}", "~"),
        ):
            with self.subTest(line=line):
                self.assertEqual(NATIVE_CHECK.shell_words(line),
                                 [f"CODEX_HOME={home}", *RESUME_ARGUMENTS], line)
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertFalse(
                            NATIVE_CHECK.reported_invocation(line, flavour(home), RESUME_ARGUMENTS),
                            line)

    def test_accepts_a_value_spelled_with_the_other_quotes(self) -> None:
        """The check applies the shell's rule, not the renderer's spelling: an assignment whose value is
        double-quoted and holds nothing a shell acts on is the same invocation, and refusing it would
        refuse a line that does resume the session. (`shellQuote` spells it with single quotes; the
        shell's own reading does not require that spelling.)
        """
        for home, line in (
            ("codex-home", f'CODEX_HOME="codex-home" {CODEX} resume {SESSION}'),
            ("codex home", f'CODEX_HOME="codex home" {CODEX} resume {SESSION}'),
        ):
            with self.subTest(line=line):
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertTrue(
                            NATIVE_CHECK.reported_invocation(line, flavour(home), RESUME_ARGUMENTS),
                            line)

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


class PipeShapedStream:
    """A text pipe over fixed text: ``readline(size)`` answers at most ``size`` characters of one line.

    This is how a pipe hands lines to a reader (``io.TextIOWrapper.readline``): up to ``size``
    characters, stopping at a newline, and shorter than ``size`` only at the end of the text. The
    iterator protocol hands out what is left as one line, because that is what iterating a stream means:
    the drainer under test has to read a line to its end, so a case that drove only ``readline`` would
    not notice a reader that kept a whole enormous line. ``sizes`` records what the reader asked for, so
    "what one read may accumulate" is measured rather than assumed.
    """

    def __init__(self, text: str) -> None:
        self.text, self.at = text, 0
        self.sizes: list[int] = []

    def readline(self, size: int = -1) -> str:
        if size is None or size < 0:
            raise AssertionError("the drainer asked for a whole line: an unending one would be "
                                 "retained in full, so the read itself has to be bounded")
        self.sizes.append(size)
        chunk = self.text[self.at:self.at + size]
        newline = chunk.find("\n")
        if newline >= 0:
            chunk = chunk[:newline + 1]
        self.at += len(chunk)
        return chunk

    def __iter__(self):
        rest, self.at = self.text[self.at:], len(self.text)
        if rest:
            yield rest


class UnterminatedLine:
    """A diagnostics stream whose single line is never ended, and whose bytes arrive in pieces.

    The stream serves ``flood`` characters and then fails the way a pipe fails when the server behind it
    dies, so a case drives it to completion instead of hanging; what the case then measures is memory --
    the pieces the drainer asked for against what it kept. An unbounded read is refused rather than
    answered, because answering it would mean holding the whole line here instead of in the drainer.
    """

    def __init__(self, flood: int) -> None:
        self.remaining = flood
        self.sizes: list[int] = []

    def _piece(self, size: int) -> str:
        if self.remaining <= 0:
            raise OSError("the flooded diagnostics pipe broke")
        take = min(size, self.remaining)
        self.remaining -= take
        return "x" * take

    def readline(self, size: int = -1) -> str:
        if size is None or size < 0:
            raise AssertionError("the drainer asked for a whole line: an unending one would be "
                                 "retained in full, so the read itself has to be bounded")
        self.sizes.append(size)
        return self._piece(size)

    def __iter__(self):
        return self

    def __next__(self) -> str:
        return self._piece(1 << 20)


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

    def test_an_oversized_line_is_cut_and_its_remainder_read_away(self) -> None:
        # One line far past the per-line bound, and a line that fits after it: the bound must cut the
        # long one (keeping its head, not its tail), and the rest of it must still be read -- a drainer
        # that stopped at the cut would leave the pipe full, which is the other half of its job.
        stream = PipeShapedStream("a" * 1_000_000 + "\nthe line after the oversized one\n")
        retained = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics(stream, retained)
        kept = list(retained)
        self.assertEqual(len(kept), 2, f"the drainer kept {kept!r}")
        self.assertEqual(kept[0], "a" * NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX
                         + NATIVE_CHECK.DIAGNOSTICS_TRUNCATED,
                         "the oversized line was not kept as its head, marked as cut")
        self.assertEqual(kept[1], "the line after the oversized one",
                         "the line after the oversized one was not read")
        self.assertLessEqual(sum(len(line) for line in kept),
                             NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_MAX
                             * (NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX + 64),
                             f"the drainer kept {sum(len(line) for line in kept)} characters")
        self.assertEqual(stream.at, len(stream.text),
                         "the drainer left part of the stream in the pipe: a full pipe blocks the "
                         "server it is draining")
        self.assertLessEqual(max(stream.sizes or [0]), NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX,
                             "the drainer read a piece larger than the per-line bound")
        self.assertTrue(stream.sizes, "the drainer never asked for a bounded piece of the stream")

    def test_an_already_split_line_is_cut_to_the_bound_too(self) -> None:
        # The drainer's other reading: a sequence of lines someone else split. An element of it can be
        # far larger than the per-line bound (the pipe did not split this one), and what the sink keeps
        # is bounded by bytes there too.
        retained = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics(["a" * 1_000_000 + "\n", "the line after\n"], retained)
        kept = list(retained)
        self.assertEqual(len(kept), 2, f"the drainer kept {len(kept)} line(s)")
        self.assertEqual(kept[0], "a" * NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX
                         + NATIVE_CHECK.DIAGNOSTICS_TRUNCATED,
                         "an oversized element was not kept as its head, marked as cut")
        self.assertEqual(kept[1], "the line after")
        self.assertLessEqual(sum(len(line) for line in kept),
                             NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_MAX
                             * (NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX + 64),
                             f"the drainer kept {sum(len(line) for line in kept)} characters")

    def test_an_unterminated_stream_is_read_away_without_being_retained(self) -> None:
        # A server that never writes a newline, and then dies: the drainer must keep reading (the pipe
        # must stay empty) while what it retains stays inside the bound -- a single line a server never
        # ends is exactly the shape a count-only bound does not answer.
        stream = UnterminatedLine(flood=3 << 20)
        retained = NATIVE_CHECK.new_diagnostics()
        NATIVE_CHECK.drain_diagnostics(stream, retained)
        kept = list(retained)
        self.assertEqual(len(kept), 1, f"the drainer kept {len(kept)} line(s) of one unending line")
        self.assertEqual(kept[0], "x" * NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX
                         + NATIVE_CHECK.DIAGNOSTICS_TRUNCATED,
                         "the unending line was not kept as its head, marked as cut")
        self.assertLessEqual(sum(len(line) for line in kept),
                             NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_MAX
                             * (NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX + 64),
                             f"the drainer kept {sum(len(line) for line in kept)} characters of a "
                             f"stream the server never ended")
        self.assertLessEqual(max(stream.sizes or [0]), NATIVE_CHECK.APP_SERVER_DIAGNOSTICS_LINE_MAX,
                             "the drainer read a piece larger than the per-line bound")
        self.assertTrue(stream.sizes, "the drainer never asked for a bounded piece of the stream")


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
        # All three shapes a credential can take in a URL, because each one is a branch of the rule
        # (`credential_free_url`): userinfo, a query parameter, and a fragment. A case with only the
        # first would stay green with the other two branches removed.
        shapes = {
            "userinfo": f"https://{HOST_TOKENS['url_user']}:{HOST_TOKENS['url_password']}@proxy.example/v1",
            "query": f"https://proxy.example/v1?token={HOST_TOKENS['query']}",
            "fragment": f"https://proxy.example/v1#token={HOST_TOKENS['query']}",
        }
        for shape, url in shapes.items():
            with self.subTest(credential_in="the url's " + shape):
                document, rendered, report = self.seeded(
                    HOST_CONFIGURATION.replace("https://proxy.example/v1", url))
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
                    "the unrelated session in the target home is unchanged",
                    "no pre-existing session in the target home changed")

#: The session this case's home already holds before the run starts: the third comparison's subject.
#: It is kept as a *name* here, so the assertion on the run's own report holds whatever separator the
#: host spells paths with.
PRE_EXISTING_NAME = "rollout-2025-12-31T23-59-00-01a1e000-0000-7000-8000-000000000003.jsonl"

#: The words the case's own growth fragment records, so the run has something to find in the *later*
#: target's document and not in the earlier one. They are the case's, not the committed fixture's: what
#: this half of the suite pins is that the shipped check reads the fragment it is given and searches the
#: two documents for its words.
GROWTH_QUESTION = "What budget do retries share per session?"
GROWTH_ANSWER = "The retry budget is 120 seconds per session."

#: The comparisons a case that asserts "nothing else was red" names, in the order the shipped check
#: records them: the repeat half's own, plus the two the *first* conversion's stage records before the
#: growth (the source copy's boundary and its unchanged bytes), which the later stages are built on. A
#: comparison that stopped running -- or one that never runs because its stage was skipped -- is red here
#: rather than silently absent.
REPEAT_CHECKS = (
    "the conversion reports the boundary of the source copy it read",
    "the source copy is unchanged by the conversion",
    "the source copy still holds the bytes the first conversion read",
    "the source copy grew by exactly the appended turn",
    "the second conversion reports the boundary of the grown source copy",
    "the second conversion's identity is new, not the source's and not the first session's",
    "the second installed file is one JSON object per line",
    "the second conversion carries the history the source gained",
    "the first conversion's session is unchanged by the second",
    "the source copy is unchanged by the second conversion",
    "the inherited reader reads the second installed file back as what was written",
    "the target harness's own loader opens the second installed session",
    "the target's own view of the second session lists the appended turn",
    "the first session's own view does not hold the appended turn",
)


class ProtectedArtifactsAfterNativeVerification(unittest.TestCase):
    """The source session, the planted session and every session the home already held are compared
    again after the harness's own native stages, not before them.

    The native half cannot run offline -- it needs the Codex binary and, for a turn, model access -- so
    these cases drive the shipped ``main`` with the seams that half stands on replaced: the subprocess
    runner, the app server and the harness's own view call, plus ``shutil.which`` for the two tools. The
    stub for ``ccc continue`` installs a real file into a throw-away home, and the stub for the resume
    turn answers like a turn that continued (the installed file grows) while it changes all three
    protected artifacts. With the comparisons at the end of the run, the mutating case must FAIL exactly
    those three checks and PASS the turn; if the comparisons move back above the loader and the turn --
    the defect this pins -- they answer on the pre-mutation state, so the mutating case below fails over
    an empty failure list while the intact control still passes: the ordering defect breaks the catching,
    not the passing. The intact run is the control for the other direction -- a comparison that answered
    FAIL for its own reasons could not pass it -- and the mutating run asserts the three protected checks
    are its **only** failures, so nothing else was already red.

    The home carries a session of its own (``PRE_EXISTING_NAME``) besides the one the harness plants, and
    that is what makes the third comparison's evidence non-vacuous rather than a restatement: with an
    empty capture, "no pre-existing session in the target home changed" would iterate nothing, so
    neutering the capture altogether would keep it green. Here the intact run has to report the capture
    it compared (a count of one) and the mutating run has to name the file it caught changing, so a
    capture that is never taken, or a comparison that never runs, fails these cases instead of agreeing
    with itself.

    The copy the conversions read is the fourth subject, and every window it has to survive has a case of
    its own (`drive`'s `copy_mutation` table): a same-length change made while a conversion runs, while a
    session's own loader runs, while either real turn runs, or -- for the second load -- after the second
    conversion's comparison, must fail exactly the comparisons whose interval covers that moment, and each
    comparison that claims an interval is named by the case whose window it is. The two turn windows are
    what place the comparison taken where the copy grows in time: a change while the *first* turn runs is
    caught by that comparison alone -- so that comparison is taken after the first conversion's loader and
    its turn, and is the last check of the bytes the growth is measured from -- and a change while the
    second turn runs is caught by the closing verdict alone, so the baseline comparison, and every other
    comparison of the copy, is behind that turn. The change is a byte swap rather than an append, so a
    comparison that weighed only sizes and a growth record whose claim is a size delta both still hold over
    it: what catches such a change is a digest compared against the right earlier bytes. The second load's
    case is also what keeps the final "the source session is unchanged" check non-vacuous, and the intact
    run's detail has to name the copy's bytes it compared, so the clause cannot be green by not being there.
    """

    def drive(self, mutate_during_resume: bool = False,
              installed_extra_line: str | None = None,
              mutate_first_session_by_second: bool = False,
              second_boundary: str | None = None,
              include_growth_in_second: bool = True,
              copy_mutation: str | None = None,
              repeat_turn: bool = False
              ) -> tuple[int, dict[str, tuple[str, str]]]:
        """One run of the shipped ``main`` against stubs; answers its exit code and its recorded checks.

        ``installed_extra_line`` is a line the stub appends to the first installed file after its one
        object, so the JSONL check's own failure paths are exercised: the line may be unparseable JSON or
        a valid JSON value that is not an object, and the check has to name each as the failure its own
        name promises.

        ``mutate_first_session_by_second`` changes the asset the *second* conversion's comparisons are
        about: the first conversion's installed file is rewritten while the second conversion runs, so the
        comparison that says the earlier target is untouched -- and only it -- must fail. That is what
        pins the new comparison to the end of the run rather than to the first conversion's own stage.

        ``second_boundary`` replaces the boundary the second conversion's document reports (``sha256``,
        ``bytes``, or any other field), so the attribution comparison is exercised against a report that
        is not the grown copy's bytes: the check measures the file itself, so a document this off must be
        a failure rather than something the harness takes the report's word for.

        ``include_growth_in_second`` is the control for the *search* over the two documents: with it off,
        the second conversion's file and the target's own view of it omit the appended turn's words, so
        the comparisons that require them -- and only they -- must fail. A search that looked for nothing,
        or one that searched the wrong file, would stay green here.

        ``copy_mutation`` names the one window in which this case makes its *same-length* change to the
        copy the conversions read, and nothing else: ``"first conversion"``, ``"first load"``, ``"first
        turn"``, ``"second conversion"``, ``"second load"`` or ``"second turn"``. The change is a byte
        swap, so every comparison that weighs only sizes still holds, and each window is answered by the
        comparisons that can see it -- the table the cases below drive:

        =================== ==========================================================
        window              the checks that must fail, and only they
        =================== ==========================================================
        first conversion    "the source copy is unchanged by the conversion" (the
                            conversion's own stage) and "the source copy still holds
                            the bytes the first conversion read" (the growth's
                            baseline, which that stage also precedes)
        first load          "the source copy still holds the bytes the first conversion
                            read" alone: the conversion's own comparison is already
                            over, and the changed bytes are the baseline every later
                            comparison would measure from
        first turn          "the source copy still holds the bytes the first conversion
                            read" alone: that comparison is taken after this
                            conversion's loader *and* its real turn, so the turn's own
                            window is inside the interval it answers for
        second conversion   "the source copy is unchanged by the second conversion" and
                            "the source session is unchanged": both compare the copy
                            with the digest taken before that conversion
        second load         "the source session is unchanged" alone, the comparison that
                            runs last
        second turn         "the source session is unchanged" alone: the baseline
                            comparison is recorded before the growth, and so *before*
                            this turn, and every other comparison of the copy is
                            behind it too
        =================== ==========================================================

        A window whose mutation fails *nothing* is a comparison that does not cover the interval its name
        claims: that is what the first-load case measured (the growth record's size delta and its
        digest-differs clause both hold over a same-length change, so a run without the baseline
        comparison answered PASS while the changed bytes became every later comparison's baseline).

        ``repeat_turn`` asks the shipped run for the *second* session's turn as well
        (``--repeat-resume-prompt`` beside ``--resume-prompt``, one turn each), which is the shape §6's
        transcript quotes: both turns then run, and the copy the conversions read has two turn windows
        instead of one -- the first before the growth, the second after the second conversion and its own
        comparisons. The two ``* turn`` windows require it, because a turn that did not run has no window
        to put a mutation in; their cases assert both turns' own checks passed, so a window whose turn was
        skipped cannot read as one whose mutation nothing caught.
        """
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
        # The committed growth fragment this run appends to its *copy*: two records, as the shipped
        # fixturegen writes one, with the words the later target's document has to hold and the earlier
        # one must not.
        fragment = repo / NATIVE_CHECK.GROWTH_ROOT / NATIVE_CHECK.GROWTH_TURN
        fragment.parent.mkdir(parents=True)
        fragment.write_text(json.dumps({"type": "user", "message": {"content": GROWTH_QUESTION}}) + "\n"
                            + json.dumps({"type": "assistant",
                                          "message": {"content": [{"type": "text",
                                                                   "text": GROWTH_ANSWER}]}}) + "\n")
        home = scratch / "codex-home"
        home.mkdir()
        # The two tools the harness resolves: names it only compares and runs through the stub below,
        # so no binary has to exist for this case.
        codex = str(scratch / "codex")
        go = str(scratch / "go")

        thread_id = "01a1e000-0000-7000-8000-000000000002"
        second_thread_id = "01a1e000-0000-7000-8000-000000000004"
        installed = (home / "sessions" / "2026" / "05" / "04"
                     / f"rollout-2026-05-04T09-15-00-{thread_id}.jsonl")
        second_installed = (home / "sessions" / "2026" / "05" / "04"
                            / f"rollout-2026-05-04T09-15-00-{second_thread_id}.jsonl")
        planted = home / "sessions" / "2026" / "01" / "01" / NATIVE_CHECK.UNRELATED_NAME
        # A session the home already held before this run started, besides the one the harness plants:
        # the third comparison's subject. The harness only digests it, so a metadata line is all it needs
        # to be, and its id is neither the source's nor the conversion's.
        pre_existing = home / "sessions" / "2025" / "12" / "31" / PRE_EXISTING_NAME
        pre_existing.parent.mkdir(parents=True)
        pre_existing.write_text(json.dumps({"type": "session_meta"}) + "\n")
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

        def source_copy(env: dict | None) -> Path:
            """The copy of the fixture the conversion under the stub is reading.

            The harness makes the copy under its own scratch directory and hands the conversion its root
            through ``CLAUDE_PROJECTS_DIR``, so that variable is where this case finds the file whose
            bytes the conversion's reported boundary has to be -- the same file the *shipped* check
            measures for itself.
            """
            root = Path((env or {}).get("CLAUDE_PROJECTS_DIR", str(scratch / "source")))
            return root / "-fake-project" / f"{NATIVE_CHECK.SOURCE_SESSION}.jsonl"

        def document_for(which: str, env: dict | None) -> dict:
            """One conversion's stub document: its own session id, path and boundary of the copy's bytes."""
            copy = source_copy(env)
            boundary = {"state": "taken", "detail": "the case's stub boundary", "path": str(copy),
                        "bytes": copy.stat().st_size, "sha256": NATIVE_CHECK.sha256(copy),
                        "read_bytes": copy.stat().st_size, "read_coverage": "size-match",
                        "record_offsets": "claimed",
                        "terminated_records_end_bytes": copy.stat().st_size}
            if which == "second":
                boundary.update(second_boundary or {})
            session, path = ((thread_id, installed) if which == "first"
                             else (second_thread_id, second_installed))
            resume_arguments = [codex, "resume", session]
            exec_resume_arguments = [codex, "exec", "resume", session, "<prompt>"]
            user_messages, assistant_messages = (1, 1) if which == "first" else (2, 2)
            return {
                "conversion": {
                    "state": "converted", "format": "codex-rollout-jsonl",
                    "carried": {"messages": user_messages + assistant_messages,
                                "user_messages": user_messages,
                                "assistant_messages": assistant_messages,
                                "tool_calls": 1, "tool_results": 1, "compaction_boundaries": 0},
                    "losses": [{"category": "reasoning", "disposition": "omitted", "count": 1,
                                "detail": "the stub's one loss"}],
                },
                "installation": {"state": "installed", "path": str(path), "mechanism": "hard-link",
                                 "created": True, "source_check_state": "unchanged"},
                "target_session": {
                    "session_id": session, "path": str(path), "binary_found": True,
                    "resume_arguments": resume_arguments, "exec_resume_arguments": exec_resume_arguments,
                    "resume_environment": {"CODEX_HOME": str(home)},
                    "resume_command": f"{assignment} {rendered(resume_arguments)}",
                    "exec_resume_command": f"{assignment} {rendered(exec_resume_arguments)}",
                },
                "source": {"boundary": boundary},
                "verification": {"load": {"state": "verified"}, "resume": {"state": "verified"}},
            }

        conversions = {"made": 0}

        def same_length_swap(path: Path) -> None:
            """This case's own change to a copy: one field's value swapped for another of the same length.

            Same-length is the point: a comparison that weighed only sizes, and a growth record whose claim
            is a size delta, both still hold over this change, so only a digest compared against the right
            earlier bytes can catch it. The guard keeps a fixture change from turning a case's premise
            into nothing.
            """
            before = path.read_text()
            changed = before.replace('"user"', '"turn"')
            if changed == before or len(changed) != len(before):
                raise AssertionError(
                    "this case's own mutation has to be a same-length swap, and the copy it changes is "
                    f"{before!r} -> {changed!r}")
            path.write_text(changed)

        #: The copy each conversion read, as the harness named it in the command's own environment: the
        #: mutations below change one of them, and the harness's scratch directory is its own (this case
        #: cannot guess its name).
        converted_copy: dict[str, Path] = {}

        def fake_run(cmd, cwd=None, env=None, timeout=600):
            command = [str(part) for part in cmd]
            if command[0] == go:
                return subprocess.CompletedProcess(cmd, 0, "", "")
            if command[0] == str(repo / NATIVE_CHECK.BINARY):
                if command[1] == "continue":
                    conversions["made"] += 1
                    which = "first" if conversions["made"] == 1 else "second"
                    converted_copy[which] = source_copy(env)
                    if copy_mutation == f"{which} conversion":
                        # The window the conversion's own comparison is about: the change is made before
                        # the stub builds its document, so the document and the boundary it reports are the
                        # mutated file's own -- what fails is the comparison against the earlier bytes, not
                        # a report that disagrees with the file it describes.
                        same_length_swap(converted_copy[which])
                    document = document_for(which, env)
                    path = Path(document["target_session"]["path"])
                    path.parent.mkdir(parents=True, exist_ok=True)
                    body = json.dumps({"type": "session_meta"}) + "\n"
                    if which == "first" and installed_extra_line:
                        body += installed_extra_line
                    elif which == "second" and include_growth_in_second:
                        # The later document holds the turn the source gained; the earlier one is written
                        # without it, which is the contrast the check searches for.
                        body += fragment.read_text()
                    if which == "second" and mutate_first_session_by_second:
                        installed.write_text(installed.read_text() + json.dumps({"type": "turn"}) + "\n")
                    path.write_text(body)
                    return subprocess.CompletedProcess(cmd, 0, json.dumps(document), "")
                if command[1] == "inspect":
                    which = "second" if second_thread_id in command[2] else "first"
                    counts = {"user_messages": 2 if which == "second" else 1,
                              "assistant_messages": 2 if which == "second" else 1,
                              "tool_calls": 1, "tool_results": 1, "compact_boundaries": 0}
                    return subprocess.CompletedProcess(
                        cmd, 0, json.dumps({"source": {"counts": counts}}), "")
            if command[0] == codex and command[1] == "exec":
                # The resume stage: a turn that continued (the session it names grows) and, when this
                # case asks for it, the mutation both protected artifacts have to survive.
                second = second_thread_id in command
                turned = second_installed if second else installed
                turned.write_text(turned.read_text() + json.dumps({"type": "turn"}) + "\n")
                if copy_mutation == ("second turn" if second else "first turn"):
                    # The copy the conversions read, changed while a real *turn* runs: for the first turn
                    # the growth's baseline comparison is still ahead of it (so that comparison, and only
                    # it, answers for this window), while for the second every comparison of the copy is
                    # already recorded and only the comparison that runs last is left to answer.
                    same_length_swap(converted_copy["second" if second else "first"])
                if mutate_during_resume:
                    fixture.write_text(fixture.read_text() + json.dumps({"type": "assistant"}) + "\n")
                    planted.write_text(planted.read_text() + json.dumps({"type": "session_meta"}) + "\n")
                    pre_existing.write_text(pre_existing.read_text() + json.dumps({"type": "turn"}) + "\n")
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
                session = str(params.get("threadId"))
                second = session == second_thread_id
                path = second_installed if second else installed
                if copy_mutation == ("second load" if second else "first load"):
                    # The copy the conversions read, changed while the target harness's own loader runs:
                    # for the second session that is after the second conversion and its own comparison, so
                    # only the comparison that runs last can catch it; for the first it is before the
                    # growth, so the changed bytes would become every later comparison's baseline.
                    same_length_swap(converted_copy["second" if second else "first"])
                return {"thread": {"id": session, "path": str(path), "historyMode": "legacy",
                                   "cwd": "/tmp"}}

            def close(self) -> None:
                pass

        def fake_thread_view(server, thread):
            lines = [f"thread {thread} (the case's stub's own view)"]
            if str(thread) == second_thread_id and include_growth_in_second:
                # The target's own view of the later session: the turn the source gained, as the harness
                # itself lists it.
                lines += [f"    userMessage: {GROWTH_QUESTION}", f"    agentMessage: {GROWTH_ANSWER}"]
            return lines, ["userMessage", "agentMessage"]

        def fake_which(name, *args, **kwargs):
            return codex if name == "codex" else go

        prompts = ["--resume-prompt", "answer only from this conversation"]
        if repeat_turn:
            # The second session's turn as well: the shape §6's transcript quotes (both prompt flags, one
            # turn each), so a case can put its mutation in either turn's own window.
            prompts += ["--repeat-resume-prompt", "answer only from this conversation"]
        before = len(NATIVE_CHECK.RESULTS)
        with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                mock.patch.object(NATIVE_CHECK, "AppServer", FakeAppServer), \
                mock.patch.object(NATIVE_CHECK, "thread_view", fake_thread_view), \
                mock.patch("shutil.which", fake_which), \
                contextlib.redirect_stdout(io.StringIO()):
            code = NATIVE_CHECK.main(["--repo", str(repo), "--codex", "codex",
                                      "--codex-home", str(home)] + prompts)
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
                         f"the mutating run's failures are not exactly the three protected-artifact "
                         f"comparisons: {results}")
        # The repeat half's own comparisons still pass here: the mutation this case makes is of the
        # *committed* fixture, and the conversions read the copy. A check that read the fixture where it
        # meant the copy would be red on this run.
        for name in REPEAT_CHECKS:
            state, detail = results.get(name, ("missing", ""))
            self.assertEqual(state, "PASS", f"{name}: {state} — {detail}; all results: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with failed checks: {results}")
        self.assertIn("the fixture tree changed:", results[PROTECTED_CHECKS[0]][1])
        self.assertIn(" before, ", results[PROTECTED_CHECKS[1]][1])
        self.assertIn(" after", results[PROTECTED_CHECKS[1]][1])
        # The session the home already held is named as the one that changed: a comparison reading an
        # empty capture, or not running, could not report this file.
        self.assertIn("changed:", results[PROTECTED_CHECKS[2]][1])
        self.assertIn(PRE_EXISTING_NAME, results[PROTECTED_CHECKS[2]][1])

    def test_an_intact_run_passes_the_comparisons(self) -> None:
        code, results = self.drive(mutate_during_resume=False)
        for name in PROTECTED_CHECKS:
            state, detail = results.get(name, ("missing", ""))
            self.assertEqual(state, "PASS", f"{name}: {state} — {detail}; all results: {results}")
        # The third comparison's own evidence: it says what it compared, so a capture that was never
        # taken (an empty one would answer PASS for every mutation) is red here rather than green.
        self.assertIn("1 pre-existing session file(s)", results[PROTECTED_CHECKS[2]][1],
                      "the run's pre-existing-session comparison did not name the session the home "
                      "already held: a capture that never happened agrees with itself")
        # The first comparison's own evidence for its copy clause: it names the copy's bytes it compared,
        # so a run that dropped the clause would not read as one that answered it.
        self.assertIn("the converted copy identical too", results[PROTECTED_CHECKS[0]][1],
                      "the source comparison did not name the converted copy it compared")
        # Every comparison of the repeat half is present and passing, so none of them can be green by
        # never running, and the only checks that did not run are the two real turns nobody asked for.
        for name in REPEAT_CHECKS:
            state, detail = results.get(name, ("missing", ""))
            self.assertEqual(state, "PASS", f"{name}: {state} — {detail}; all results: {results}")
        self.assertEqual(sorted(name for name, (state, _) in results.items() if state == "SKIP"),
                         ["a real turn continues from the second installed session"],
                         f"the run skipped something other than the one turn nobody asked for: {results}")
        # The turn this run did ask for ran in the *first* session, which is what makes that session a
        # session its own harness has already resumed by the time the second conversion happens.
        self.assertEqual(results["a real turn continues from the installed session"][0], "PASS",
                         f"the first session's turn did not run: {results}")
        # The attribution comparisons say what they compared, so a comparison that read the wrong file
        # (or an empty one) is visible in the record rather than only in a verdict.
        self.assertIn("the first conversion's boundary was",
                      results["the second conversion reports the boundary of the grown source copy"][1])
        self.assertIn("before and after the second conversion",
                      results["the first conversion's session is unchanged by the second"][1])
        # The comparison taken where the copy grows names the pre-conversion bytes it compared, so a run
        # that dropped the comparison, or one that compared the file with itself, is visible here.
        self.assertIn("taken before the first conversion",
                      results["the source copy still holds the bytes the first conversion read"][1],
                      "the comparison taken where the copy grows did not name the bytes taken before the "
                      "first conversion")
        self.assertEqual(code, 0, f"exit {code} for an intact run: {results}")

    def test_a_malformed_codex_installed_line_is_a_named_failure(self) -> None:
        """An unparseable installed line is a FAIL the run reports, not a traceback out of ``main``.

        The JSONL check is a named check, and a raise would print no ``FAIL`` line and no summary -- the
        very reporting contract the harness promises never to break. The run must therefore return 1 with
        that check recorded as a FAIL whose detail names the line and the parse error.
        """
        code, results = self.drive(mutate_during_resume=False, installed_extra_line="{not json}\n")
        state, detail = results.get("the installed file is one JSON object per line", ("missing", ""))
        self.assertEqual(state, "FAIL",
                         f"the malformed installed line did not fail the JSONL check: {results}")
        self.assertIn("line 2 is not JSON", detail, detail)
        self.assertEqual(code, 1, f"exit {code} for a run whose JSONL check failed: {results}")

    def test_a_first_session_mutated_by_the_second_conversion_is_caught(self) -> None:
        """The comparison about the earlier target runs after the *second* conversion, not before it.

        The stub rewrites the first conversion's installed file while the second conversion runs -- the
        shape a second conversion that merged into, or overwrote, the session an agent may already have
        continued would have. Exactly that comparison must fail: the earlier target's bytes are the
        promise this half of the goal makes, and a comparison recorded before the second conversion would
        answer on the state it was about before that state could change.
        """
        code, results = self.drive(mutate_first_session_by_second=True)
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, ["the first conversion's session is unchanged by the second"],
                         f"the mutating run's failures are not exactly the comparison about the first "
                         f"session: {results}")
        detail = results["the first conversion's session is unchanged by the second"][1]
        self.assertIn(" before and after the second conversion", detail, detail)
        self.assertEqual(code, 1, f"exit {code} for a run with a failed comparison: {results}")

    def test_a_copy_mutated_during_the_second_load_is_caught_by_the_final_comparison(self) -> None:
        """The copy the conversions read is part of the final "the source is unchanged" answer.

        The stub changes the copy while the *second* session's own loader runs -- after the second
        conversion and after that conversion's own copy comparison, so only the comparison that runs last
        can answer for the copy. Its failure set must therefore be exactly that one check, and its detail
        must name the copy's own wanted bytes: a run whose final comparison hashed only the committed
        fixture tree and the growth fragment answers PASS here, which is the defect this pins.
        """
        code, results = self.drive(copy_mutation="second load")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, ["the source session is unchanged"],
                         f"the copy mutation's failures are not exactly the final comparison: {results}")
        detail = results["the source session is unchanged"][1]
        self.assertIn("the converted source copy changed after the second conversion", detail, detail)
        self.assertIn("want sha256", detail, detail)
        self.assertEqual(code, 1, f"exit {code} for a run with a failed comparison: {results}")

    def test_a_copy_mutated_during_the_first_conversion_is_caught_by_two_comparisons(self) -> None:
        """A conversion that wrote to its own source is caught by the comparison about its own stage.

        The stub changes the copy while the *first* conversion runs, before it builds the document it
        reports: the bytes the conversion's own report describes are the file as it stands, so the failure
        is the comparison against the earlier bytes rather than a report that disagrees with its file. Both
        comparisons whose interval covers that moment must fail -- "the source copy is unchanged by the
        conversion", whose name is the promise the conversion must not write its source, and "the source
        copy still holds the bytes the first conversion read", because the changed bytes are what the
        growth below measures from -- and nothing else, so the run still holds its other checks.
        """
        code, results = self.drive(copy_mutation="first conversion")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed,
                         sorted(["the source copy is unchanged by the conversion",
                                 "the source copy still holds the bytes the first conversion read"]),
                         f"a copy changed while the first conversion ran was not caught by exactly the two "
                         f"comparisons whose interval covers it: {results}")
        self.assertIn(" taken before and after",
                      results["the source copy is unchanged by the conversion"][1])
        self.assertIn(" taken before the first conversion",
                      results["the source copy still holds the bytes the first conversion read"][1])
        self.assertEqual(code, 1, f"exit {code} for a run with failed comparisons: {results}")

    def test_a_copy_mutated_during_the_second_conversion_is_caught_by_two_comparisons(self) -> None:
        """A second conversion that wrote to its own source is caught by the two comparisons about it.

        The stub changes the copy while the *second* conversion runs, before it builds its document. Both
        comparisons that hold the digest taken before that conversion must fail -- "the source copy is
        unchanged by the second conversion" and the final "the source session is unchanged" -- and nothing
        else: the attribution comparison still passes, because the document the conversion reported is the
        mutated file's own bytes, which is what tells this case apart from a lying boundary report.
        """
        code, results = self.drive(copy_mutation="second conversion")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed,
                         sorted(["the source copy is unchanged by the second conversion",
                                 "the source session is unchanged"]),
                         f"a copy changed while the second conversion ran was not caught by exactly the two "
                         f"comparisons whose interval covers it: {results}")
        self.assertEqual(results["the second conversion reports the boundary of the grown source copy"][0],
                         "PASS", f"the attribution comparison failed over a consistent report: {results}")
        self.assertIn("the converted source copy changed after the second conversion",
                      results["the source session is unchanged"][1])
        self.assertEqual(code, 1, f"exit {code} for a run with failed comparisons: {results}")

    def test_a_copy_mutated_during_the_first_load_is_caught_where_the_copy_grows(self) -> None:
        """The bytes the copy grows from are the bytes the first conversion read, re-checked.

        The stub makes a *same-length* change to the copy while the *first* session's own loader runs --
        after the first conversion's own comparison and before the growth fragment is appended -- so the
        growth record's size delta still holds and its digest still differs, and the changed bytes would
        become the baseline every later comparison measures from. Exactly the comparison taken where the
        copy grows must fail: a run without it answers PASS here while its final "the source session is
        unchanged" verdict speaks for an interval whose baseline it never checked.
        """
        code, results = self.drive(copy_mutation="first load")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, ["the source copy still holds the bytes the first conversion read"],
                         f"the first native stage's copy mutation was not caught by exactly the comparison "
                         f"taken where the copy grows: {results}")
        detail = results["the source copy still holds the bytes the first conversion read"][1]
        self.assertIn(" taken before the first conversion", detail, detail)
        # The other half of the same defect: the growth record's own size delta did hold (that is what
        # makes this case about the baseline rather than about a length change), so it must not be what
        # fails here -- if it did, this case would pass for a run that only compared sizes.
        self.assertEqual(results["the source copy grew by exactly the appended turn"][0], "PASS",
                         f"the growth record failed over a same-length mutation, so this case is no longer "
                         f"about the baseline it pins: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with a failed comparison: {results}")

    def test_a_copy_mutated_during_the_first_turn_is_caught_where_the_copy_grows(self) -> None:
        """The bytes the growth is measured from are re-checked *after* the first session's own turn.

        The stub makes a same-length change to the copy while the first session's real turn runs -- after
        that conversion's own comparison and after its loader, with the growth fragment not yet appended.
        The comparison taken where the copy grows is recorded after this turn and before the growth (hence
        before the second conversion, its loader and its turn), so exactly it must fail; a comparison
        recorded *before* the turn -- the interval this record once attributed to it -- answers on the
        pre-turn bytes, the changed copy becomes the baseline every later comparison measures from, and
        this case's failure list is empty instead.
        """
        code, results = self.drive(copy_mutation="first turn", repeat_turn=True)
        for name in ("a real turn continues from the installed session",
                     "a real turn continues from the second installed session"):
            self.assertEqual(results.get(name, ("", ""))[0], "PASS",
                             f"{name}: {results.get(name)} — a turn that did not run has no window to "
                             f"mutate in: {results}")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, ["the source copy still holds the bytes the first conversion read"],
                         f"the first turn's copy mutation was not caught by exactly the comparison taken "
                         f"where the copy grows: {results}")
        detail = results["the source copy still holds the bytes the first conversion read"][1]
        self.assertIn(" taken before the first conversion", detail, detail)
        # The other half of the same defect: the growth record's own size delta did hold (that is what
        # makes this case about the baseline rather than about a length change).
        self.assertEqual(results["the source copy grew by exactly the appended turn"][0], "PASS",
                         f"the growth record failed over a same-length mutation, so this case is no longer "
                         f"about the baseline it pins: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with a failed comparison: {results}")

    def test_a_copy_mutated_during_the_second_turn_is_caught_by_the_final_comparison(self) -> None:
        """The baseline comparison is behind the *second* turn, and the closing verdict is after it.

        The stub changes the copy while the second session's turn runs -- after the second conversion's
        own comparison and after the comparison taken where the copy grew, both recorded by then -- so
        only the comparison that runs last can answer for it. Exactly that one must fail, its detail must
        name the copy's wanted bytes, and the baseline comparison must have passed: that pair is what puts
        the growth baseline in front of this turn rather than behind it, so a run whose baseline
        comparison moved after the second turn would report two failures here, and a run that never
        re-compared the copy at the end would report none.
        """
        code, results = self.drive(copy_mutation="second turn", repeat_turn=True)
        self.assertEqual(results.get("a real turn continues from the second installed session",
                                     ("", ""))[0], "PASS",
                         f"the second turn did not run, so its mutation would not be what the comparison "
                         f"catches: {results}")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, ["the source session is unchanged"],
                         f"the second turn's copy mutation was not caught by exactly the comparison that "
                         f"runs last: {results}")
        detail = results["the source session is unchanged"][1]
        self.assertIn("the converted source copy changed after the second conversion", detail, detail)
        self.assertEqual(results["the source copy still holds the bytes the first conversion read"][0],
                         "PASS",
                         f"the comparison taken where the copy grows answered for a turn that had not run "
                         f"yet: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with a failed comparison: {results}")

    def test_a_second_boundary_that_is_not_the_grown_copy_fails(self) -> None:
        """The reported boundary is checked against the file, not taken on its word.

        The stub's second document claims a digest that is not the grown copy's. The check measures the
        file itself, so the attribution comparison must fail -- and each of the overrides below is a
        different way a report can be wrong: a digest from another file, a size that is not the file's, a
        boundary that was never taken, and a read whose own snapshot is not the file (`read_bytes` or
        `read_coverage` disagreeing), which is the answer that makes the two conversions of one growing
        copy attributable to their own bytes at all.
        """
        for override in ({"sha256": "0" * 64}, {"bytes": 1}, {"state": "not-taken"},
                         {"read_coverage": "size-mismatch"}, {"read_bytes": 1}):
            with self.subTest(override=override):
                code, results = self.drive(second_boundary=override)
                state, detail = results.get(
                    "the second conversion reports the boundary of the grown source copy", ("missing", ""))
                self.assertEqual(state, "FAIL", f"{override}: {state} — {detail}; all results: {results}")
                # The first conversion's own boundary comparison is not the one that failed: the failure
                # is attributed to the document that lied, not to the run.
                self.assertEqual(results["the conversion reports the boundary of the source copy it read"][0],
                                 "PASS", f"{override}: {results}")
                self.assertEqual(code, 1, f"{override}: exit {code} for a failed comparison: {results}")

    def test_a_second_document_without_the_appended_turn_fails_the_search(self) -> None:
        """The two comparisons that look for the turn the source gained are live in both directions.

        With the growth left out of the second conversion's document and out of the target's own view,
        those two comparisons -- and only they -- must fail. A search that compared nothing, or one that
        searched the first conversion's file, stays green here.
        """
        code, results = self.drive(include_growth_in_second=False)
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed,
                         sorted(["the second conversion carries the history the source gained",
                                 "the target's own view of the second session lists the appended turn"]),
                         f"the run's failures are not exactly the two comparisons that search for the "
                         f"appended turn: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with failed comparisons: {results}")

    def test_a_non_object_codex_installed_line_is_a_named_failure(self) -> None:
        """A line that parses but is not a JSON object is a FAIL the check names, not a pass.

        ``json.loads`` also accepts ``null``, ``false`` and ``[]``, so a reading that asked only whether
        the line parses -- the Codex check's own replaced predicate asked more -- would answer PASS on
        "one JSON object per line" for a file whose records are not records. Each kind is driven through
        the shipped run, which must record that check's FAIL with the line's number and the value's kind,
        and still exit 1 rather than raising out of ``main``.
        """
        for line, kind in (("null\n", "null"), ("false\n", "a boolean"), ("[]\n", "an array")):
            with self.subTest(installed_line=line):
                code, results = self.drive(mutate_during_resume=False, installed_extra_line=line)
                state, detail = results.get("the installed file is one JSON object per line",
                                            ("missing", ""))
                self.assertEqual(state, "FAIL",
                                 f"the non-object installed line {line!r} did not fail the JSONL "
                                 f"check: {results}")
                self.assertIn(f"line 2 is not a JSON object, but {kind}", detail, detail)
                self.assertEqual(code, 1,
                                 f"exit {code} for a run whose JSONL check failed: {results}")


#: This increment's evidence record: the file whose §6 and §13 passages state where the native check
#: takes the comparison the growth is measured from.
RECORD = SCRIPTS.parent / "docs" / "evidence" / "0004-repeatable-continuation-handoffs.md"

#: The order the shipped check runs in, read from the script's own source, one ``(label, needle)`` per
#: stage: the first session's real turn, the comparison taken where the copy grows, the growth append,
#: then the second session's real turn. Each needle is a call the script makes exactly once, and the
#: label is what a failure names instead of quoting the call.
SHIPPED_ORDER = (
    ("the first session's turn", "run_turn(repo, codex, home, installed,"),
    ("the comparison taken where the copy grows",
     'record("the source copy still holds the bytes the first conversion read"'),
    ("the growth append", "append_fragment(copy, fragment)"),
    ("the second session's turn", "run_turn(repo, codex, home, second_installed,"),
)

#: The two halves of that placement a record states: the comparison is taken after the first session's
#: real turn, and before the copy the second conversion reads grows (hence before the second turn).
STATED_ORDER = ("after the first conversion's loader and its own real turn",
                "before the copy the second conversion reads grows")

#: The reading this record carried before this round's correction, kept here as the shape the case
#: reports rather than counts as the rule: the comparison placed ahead of *both* real turns, where no
#: turn had yet run when it was taken.
WRONG_ORDER = "runs *before* either"

#: The fence a quoted run -- a transcript, a command, a failure message -- lives in.
FENCE = "```"


def prose(document: str) -> str:
    """A record's own prose: its text with each fenced block dropped, whitespace collapsed to one line.

    The two readings below are about what the record *states*, not about what it quotes: §15 quotes the
    case's own failure message, and that message repeats the reading the record corrected, so a reading
    that scanned the whole file would report the record's own evidence against it. Inside a fence is
    quoted material; outside it is the record speaking.
    """
    kept, inside = [], False
    for line in document.splitlines():
        if line.startswith(FENCE):
            inside = not inside
            continue
        if not inside:
            kept.append(line)
    return " ".join(" ".join(kept).split())


class RecordStatesTheChecksOwnOrder(unittest.TestCase):
    """The record's claim about when the growth baseline is taken is the shipped check's own order.

    ``scripts/codex-native-check.py`` runs the first session's turn, takes the comparison whose detail is
    "the source copy still holds the bytes the first conversion read", appends the growth fragment and
    then runs the second session's turn -- the order read here from the script's own source. The record's
    §6 and §13 passages must state both ends of it and must not carry the reading the review found (the
    comparison ahead of *both* turns): the behavioural pins of that order are the two turn-window cases in
    ``ProtectedArtifactsAfterNativeVerification``, and this case is what keeps the record's own words
    answerable to them. What it does not read: a passage that states no placement at all, a claim spelled
    in words neither half of ``STATED_ORDER`` holds, and anything inside a fence -- those are readable by a
    person and by the review, not by this reading.
    """

    def test_the_record_states_the_order_the_shipped_script_runs_in(self) -> None:
        source = (SCRIPTS / "codex-native-check.py").read_text(encoding="utf-8")
        positions = []
        for label, stage in SHIPPED_ORDER:
            self.assertEqual(source.count(stage), 1,
                             f"the shipped script no longer makes exactly one call that is {label} "
                             f"({stage!r}), so the order this case reads from its source would name "
                             f"another stage")
            positions.append(source.index(stage))
        order = ", ".join(f"{label} at {position}"
                          for (label, _), position in zip(SHIPPED_ORDER, positions))
        self.assertEqual(positions, sorted(positions),
                         "the shipped script no longer runs the first session's turn, the comparison "
                         "taken where the copy grows, the growth append and the second session's turn in "
                         f"the order this rule is about: {order}")
        document = prose(RECORD.read_text(encoding="utf-8"))
        for half in STATED_ORDER:
            self.assertIn(half, document,
                          f"{RECORD.name} does not state {half!r}: this case reads the record's own "
                          f"claim about where that comparison is taken")
        self.assertNotIn(WRONG_ORDER, document,
                         f"{RECORD.name} places the comparison ahead of both real turns: the shipped "
                         f"script takes it after the first session's turn and before the second's")


# ===================================================================================================
# The frozen artifact oracle (goal 0009a): its committed synthetic fixtures, the one-or-more mutations
# that remove one rule each and turn their fixture red, and the cases that hold
# `scripts/codex-native-check.py`'s `validate_artifact` and its `--validate` mode to both.
#
# The rule table is frozen for the rest of goal 0009 (the section header in the shipped script states
# the oracle lock and its reason), so this half is the contract's other end: the table and this mutation
# table are asserted to be in bijection -- a rule without a red mutation, or a mutation without a rule,
# fails the required `selftest` step -- and each mutation's own case asserts it turns *its* rule red and
# no other, which is what makes the report a reading rather than a net.
# ===================================================================================================

#: The committed synthetic Codex artifacts and manifest receipts the oracle is exercised with: one
#: positive fixture (a two-turn session with a call, its result and a compaction boundary, plus record
#: types this build's writer never writes), the same session's own records with the second turn taken
#: out, and a receipt for each. Nothing here is a converted source session: these are the oracle's own
#: hand-constructed inputs, which is what lets a rule's fixture be reasoned about without a source.
ORACLE_FIXTURES = SCRIPTS / "tests" / "testdata" / "codex-oracle"


def oracle_fixture(name: str) -> str:
    """One committed fixture's own text."""
    return (ORACLE_FIXTURES / name).read_text(encoding="utf-8")


def oracle_receipt(name: str) -> dict:
    """One committed manifest receipt, parsed."""
    return json.loads(oracle_fixture(name))


def oracle_receipt_edit(receipt: dict, match: Callable[[dict], bool],
                        change: Callable[[dict], None]) -> dict:
    """One parsed receipt with its first entry `match` answers for changed.

    A mutation whose entry the fixture no longer holds raises instead of editing nothing: a case that
    silently mutated nothing would stay green and read as a rule nothing can break.
    """
    for entry in receipt["occurrences"]:
        if match(entry):
            change(entry)
            return receipt
    raise AssertionError("the receipt holds no entry this mutation edits")


def oracle_entry_kind(kind: str) -> Callable[[dict], bool]:
    """A matcher for the receipt entry whose occurrence kind is `kind`."""
    return lambda entry: entry.get("kind") == kind


def _entry_disposition_omitted(entry: dict) -> None:
    entry["disposition"] = "omitted"


def _entry_disposition_kept(entry: dict) -> None:
    entry["disposition"] = "kept"


def _entry_call_id_dropped(entry: dict) -> None:
    entry.pop("call_id")


def _without_record(text: str, match: Callable[[dict], bool]) -> str:
    """One fixture's text with the first record `match` answers for deleted, and nothing else moved."""
    lines = text.splitlines()
    kept = [line for line in lines if not match(json.loads(line))]
    if len(kept) != len(lines) - 1:
        raise AssertionError("the fixture holds no single record this mutation deletes")
    return "\n".join(kept) + "\n"


def oracle_quote(record: dict) -> str:
    """One record as the fixture's own line: compact, the way the committed files are written."""
    return json.dumps(record, separators=(",", ":"))


def oracle_edit(text: str, match: Callable[[dict], bool], change: Callable[[dict], None] | None = None,
                replacement: str | None = None) -> str:
    """One fixture's text with the first record `match` answers for changed — or replaced whole.

    A fixture that no longer holds the record raises rather than editing nothing, for the same reason a
    receipt mutation does: a mutation that edited nothing would leave its case green.
    """
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if not line.strip() or not match(json.loads(line)):
            continue
        if replacement is not None:
            lines[index] = replacement
        else:
            assert change is not None
            record = json.loads(line)
            change(record)
            lines[index] = oracle_quote(record)
        return "\n".join(lines) + "\n"
    raise AssertionError("the fixture holds no record this mutation edits")


def oracle_record_is(kind: str) -> Callable[[dict], bool]:
    """A matcher for the record whose envelope `type` is `kind`."""
    return lambda record: record.get("type") == kind


def oracle_event_is(kind: str) -> Callable[[dict], bool]:
    """A matcher for the record whose `event_msg` payload type is `kind`."""
    def match(record: dict) -> bool:
        return record.get("type") == "event_msg" and (record.get("payload") or {}).get("type") == kind
    return match


def oracle_event_message_is(text: str) -> Callable[[dict], bool]:
    """A matcher for the turn-view event carrying exactly `text` as its message."""
    def match(record: dict) -> bool:
        payload = record.get("payload") or {}
        return (record.get("type") == "event_msg" and payload.get("type") in ("user_message",
                                                                             "agent_message")
                and payload.get("message") == text)
    return match


def oracle_item_is(kind: str, needle: str) -> Callable[[dict], bool]:
    """A matcher for the `response_item` whose payload type is `kind` and whose line holds `needle`."""
    def match(record: dict) -> bool:
        return (record.get("type") == "response_item"
                and (record.get("payload") or {}).get("type") == kind
                and needle in oracle_quote(record))
    return match


class OracleMutation(NamedTuple):
    """One negative fixture: the mutation that removes a rule, the fixture pair it is applied to, and
    what a failure names it by."""

    label: str
    artifact: str
    receipt: str | None
    mutate: Callable[[str, dict | None], tuple[str, dict | None]]


def _drop_timestamp(record: dict) -> None:
    record.pop("timestamp")


def _timestamp_zone_out_of_range(record: dict) -> None:
    stamp = record["timestamp"]
    if not stamp.endswith("Z"):
        raise AssertionError("the fixture's instant is no longer Z-spelled")
    record["timestamp"] = stamp[:-1] + "+99:99"


def _timestamp_leap_second(record: dict) -> None:
    """One record's instant moved to the leap second `60` of its own minute."""
    stamp = record["timestamp"]
    if len(stamp) < 20 or stamp[17:19] != "00":
        raise AssertionError("the fixture's instant no longer holds the second this mutation edits")
    record["timestamp"] = stamp[:17] + "60" + stamp[19:]


def _timestamp_trailing_newline(record: dict) -> None:
    """One record's instant with a line feed appended.

    The decoded string is no RFC3339 instant — an instant reader taking Python's `$` as the end of the
    string matches just before this line feed and reads the same instant — so the envelope rule is the
    only rule that can refuse it (`oracle.clock` skips a stamp it cannot parse).
    """
    record["timestamp"] += "\n"


def _timestamp_unicode_digit(record: dict) -> None:
    """One digit of a record's instant written as a non-ASCII decimal digit.

    Python's `\\d` matches every Unicode decimal digit and `int` parses them, so an instant matcher
    spelled with `\\d` read this string as the same instant where no RFC3339 reader would parse it.
    """
    stamp = record["timestamp"]
    if not stamp.startswith("2026"):
        raise AssertionError("the fixture's instant no longer opens with the year this mutation edits")
    record["timestamp"] = "\u0662" + stamp[1:]


def _output_text_substituted(record: dict) -> None:
    """One call output's text replaced with different text under the same `call_id`."""
    record["payload"]["output"] = "# notes, but not the ones the plan read\n"


def _call_arguments_substituted(record: dict) -> None:
    """One call's recorded input replaced with another file's path under the same `call_id`."""
    record["payload"]["arguments"] = '{"file_path":"/work/oracle-fixture/other.md"}'


def _call_name_substituted(record: dict) -> None:
    """One call's emitted tool name replaced under the same `call_id`."""
    record["payload"]["name"] = "Write"


def _payload_becomes_a_list(record: dict) -> None:
    record["payload"] = ["not", "an", "object"]


def _type_becomes_unknown(record: dict) -> None:
    record["type"] = "conversation_item"


def _id_becomes_empty(record: dict) -> None:
    record["payload"]["id"] = ""


def _cli_version_dropped(record: dict) -> None:
    record["payload"].pop("cli_version")


def _content_becomes_empty(record: dict) -> None:
    record["payload"]["content"] = []


def _content_block_type_is_not_read(record: dict) -> None:
    record["payload"]["content"][0]["type"] = "input_image"


def _content_text_becomes_blank(record: dict) -> None:
    record["payload"]["content"][0]["text"] = "   "


def _content_text_becomes_file_separator(record: dict) -> None:
    """One message's whole text replaced with U+001C, which Go's space set does not hold."""
    record["payload"]["content"][0]["text"] = "\x1c"


def _event_message_becomes_file_separator(record: dict) -> None:
    """One turn-view event's message replaced with U+001C, which Go's space set does not hold."""
    record["payload"]["message"] = "\x1c"


def _event_message_becomes_blank(record: dict) -> None:
    """One turn-view event's message replaced with a Go-blank string."""
    record["payload"]["message"] = " "


def _event_message_is_not_a_string(record: dict) -> None:
    record["payload"]["message"] = None


def _output_names_an_absent_call(record: dict) -> None:
    record["payload"]["call_id"] = "call_oracle_0002"


def _mutation_line_not_json(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("event_msg"), replacement="this line is not JSON"), receipt


def _mutation_line_is_an_array(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("world_state"), replacement='["world_state"]'), receipt


def _mutation_line_non_json_constant(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's number written as `NaN`, a constant JSON has no spelling for.

    Python's decoder accepts `NaN`/`Infinity`; the artifact is JSON and the target's own reader reads
    JSON, so a line this oracle's parser would have turned into a float the format cannot hold is
    refused exactly as a line that does not parse is.
    """
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "world_state":
            continue
        record = json.loads(line)
        record["payload"]["state"] = "__NaN__"
        lines[index] = oracle_quote(record).replace('"__NaN__"', "NaN")
        return "\n".join(lines) + "\n", receipt
    raise AssertionError("the fixture no longer holds its world_state record")


def _mutation_line_nested_past_reading(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One line nested past what this reader can parse.

    It is the rule's own subject either way a host answers it: the parse refuses the line (this
    interpreter raises `RecursionError` at 200 000 levels, measured) and the oracle names it unreadable,
    or the line reads as what it is — not an object. A validator that raised here would answer about its
    own stack rather than about the file.
    """
    return oracle_edit(text, oracle_record_is("event_msg"),
                       replacement="[" * 200000 + "]" * 200000), receipt


def _mutation_line_duplicate_member(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's line with a member name written twice, spliced into the JSON text itself.

    The duplication is a text edit and not a record edit on purpose: `json.dumps` of a parsed dict cannot
    write a repeated name, and the two readers disagree only about text no single dict holds — this
    oracle's decoder keeps the *last* value and the carried reader's `gjson` the *first* (review round
    r5's finding). The repeated pair's values are equal, so the record's own parsed value is unchanged
    and the line is refused by the uniqueness clause alone; the `world_state` record carries no rule but
    its own envelope, so dropping it from the reading leaves every other rule as it was.
    """
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "world_state":
            continue
        needle = '"full":true'
        if needle not in line:
            raise AssertionError("the fixture's world_state no longer holds the member this mutation copies")
        lines[index] = line.replace(needle, needle + "," + needle, 1)
        return "\n".join(lines) + "\n", receipt
    raise AssertionError("the fixture no longer holds its world_state record")


def _mutation_envelope_without_timestamp(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_event_is("user_message"), change=_drop_timestamp), receipt


def _mutation_envelope_payload_not_an_object(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("world_state"), change=_payload_becomes_a_list), receipt


def _mutation_envelope_offset_out_of_range(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's numeric offset leaves the range RFC 3339 bounds: `+99:99` dates no instant.

    The offset changes and the instant around it does not, so the record is `oracle.envelope`'s alone:
    `oracle.clock` skips a stamp it cannot read rather than comparing it, and no other rule reads the
    stamp — the two mechanisms that let an out-of-range offset through while every other case stayed
    green.
    """
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_zone_out_of_range), receipt


def _mutation_envelope_leap_second(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record stamped at the leap second `60`, which no clock this writer runs on emits.

    Go's `RFC3339Nano` never writes `:60`, and an instant reader that accepted it would have to decide
    what it means — mapping it to the next minute puts a fractional leap second after the midnight that
    follows it. The oracle reads the writer's own instant subset, so a `:60` stamp is refused by the
    rule every unreadable instant is refused by, and no other rule reads it: the clock skips a stamp it
    cannot parse instead of comparing it.
    """
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_leap_second), receipt


def _mutation_envelope_trailing_newline(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's instant with a line feed appended: the whole string is not an RFC3339 instant.

    Python's `re.match` with `$` accepts a match just before a final newline, so a matcher anchored that
    way read this stamp as the same instant — review round r3's finding. The rule is refused by the
    envelope alone, for the reason the leap-second mutation states: the clock skips a stamp it cannot
    parse.
    """
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_trailing_newline), receipt


def _mutation_envelope_unicode_digits(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's instant with a non-ASCII decimal digit, which RFC3339 does not write.

    Python's `\\d` reads every Unicode decimal digit and `int` parses them, so a matcher spelled that
    way read this stamp as the same instant — review round r3's finding, the trailing newline's other
    arm. The envelope refuses it; the clock skips it.
    """
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_unicode_digit), receipt


def _mutation_record_type_unknown(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("world_state"), change=_type_becomes_unknown), receipt


def _mutation_session_meta_deleted(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return _without_record(text, oracle_record_is("session_meta")), receipt


def _mutation_session_meta_duplicated(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    lines = text.splitlines()
    if json.loads(lines[0]).get("type") != "session_meta":
        raise AssertionError("the fixture no longer opens with its session_meta")
    return "\n".join([lines[0], lines[0]] + lines[1:]) + "\n", receipt


def _mutation_every_record_deleted(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The artifact holds no records at all, its metadata record among them: nothing to open with."""
    if not text.strip():
        raise AssertionError("the fixture is already empty")
    return "", receipt


def _mutation_session_meta_id_empty(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("session_meta"), change=_id_becomes_empty), receipt


def _mutation_session_meta_cli_version_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("session_meta"), change=_cli_version_dropped), receipt


def _mutation_clock_swapped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The compaction boundary and the turn context after it trade instants: one written backwards.

    The pair is adjacent and the swap touches no other rule: the boundary's own line and the turn
    context that follows it carry each other's instant, so exactly one record is stamped before the
    record written before it, which is the rule's own shape.
    """
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "compacted":
            continue
        following = json.loads(lines[index + 1])
        if following.get("type") != "turn_context":
            raise AssertionError("the fixture no longer puts a turn context after its boundary")
        record = json.loads(line)
        record["timestamp"], following["timestamp"] = following["timestamp"], record["timestamp"]
        lines[index], lines[index + 1] = oracle_quote(record), oracle_quote(following)
        return "\n".join(lines) + "\n", receipt
    raise AssertionError("the fixture no longer holds a compacted record")


def _mutation_message_content_empty(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("message", "The synthetic one."),
                       change=_content_becomes_empty), receipt


def _mutation_message_block_type_not_read(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("message", "Which fixture is this?"),
                       change=_content_block_type_is_not_read), receipt


def _mutation_message_text_blank(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("message", "And now?"),
                       change=_content_text_becomes_blank), receipt


def _mutation_event_message_not_a_string(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_event_is("agent_message"),
                       change=_event_message_is_not_a_string), receipt


def _mutation_event_message_blank(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One turn event's message is a Go-blank string: no view record a read of it keeps.

    Blank is the writer's own test (`strings.TrimSpace`), so the mutation writes a text that rule calls
    blank — three spaces — and not one of the information separators Python's wider `str.strip` would
    call blank while the writer preserves it.
    """
    return oracle_edit(text, oracle_event_is("agent_message"),
                       change=_event_message_becomes_blank), receipt


def _mutation_output_names_an_absent_call(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("function_call_output", "call_oracle_0001"),
                       change=_output_names_an_absent_call), receipt


def _mutation_output_before_its_call(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's own output written one slot earlier, its instant staying in the slot.

    The pair trades records while each keeps the instant of the slot it stands in, so the clock stays
    nondecreasing, every count is unchanged and the output still names an id the artifact carries — only
    the record order says the output stands before its owner. The target's own reader attaches an output
    seen before its owner to nothing (`internal/parser/codex_collecting_sink.go`), and this route's
    writer never writes one there (`internal/ccc/codex_target.go`), so the slot the output took holds a
    result that does not attach to its call.
    """
    lines = text.splitlines()
    marker = "call_oracle_0001"
    calls = next(index for index, line in enumerate(lines)
                 if oracle_item_is("function_call", marker)(json.loads(line)))
    outputs = next(index for index, line in enumerate(lines)
                   if oracle_item_is("function_call_output", marker)(json.loads(line)))
    call, output = json.loads(lines[calls]), json.loads(lines[outputs])
    call["timestamp"], output["timestamp"] = output["timestamp"], call["timestamp"]
    lines[calls], lines[outputs] = oracle_quote(output), oracle_quote(call)
    return "\n".join(lines) + "\n", receipt


def _mutation_receipt_declares_nothing(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The receipt left declaring no occurrences at all.

    A plan that declares nothing accounted for nothing, and on an artifact that holds no occurrence in
    either channel both accounting rules compare empty against empty and pass — so the receipt itself
    has to refuse a declaration that is not a plan (`docs/plans/0009a-native-oracle.md`'s edge case).
    """
    assert receipt is not None
    receipt["occurrences"] = []
    return text, receipt


def oracle_omitted_message(receipt: dict) -> dict:
    """One message entry the plan marks `omitted`, for the mutations that plant its text in a shape the
    plan's vocabulary cannot name. A receipt that marks none raises rather than mutating nothing."""
    omitted = [entry for entry in receipt["occurrences"]
               if entry.get("kind") == "message" and entry.get("disposition") == "omitted"]
    if not omitted:
        raise AssertionError("the fixture's plan marks no message omitted")
    return omitted[0]


def _mutation_plan_boundary_replacement_history(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The boundary's native `replacement_history` filled with the dialogue the plan marks omitted.

    A native `compacted` record carries the messages it replaced beside its summary
    (`internal/parser/codex_fidelity_test.go`), and this route's writer emits only the summary: the
    receipt's boundary identity is that text alone, so dialogue written into `replacement_history` is
    history the plan cannot account for although every identity, count and order it compares is
    unchanged. The mutation writes the plan's own omitted prompt into the member — the bytes the plan
    claims are not in the artifact.
    """
    assert receipt is not None
    omitted = oracle_omitted_message(receipt)

    def carry(record: dict) -> None:
        record["payload"]["replacement_history"] = [
            {"type": "message", "role": omitted.get("role", "user"),
             "content": [{"type": "input_text", "text": omitted["text"]}]}]

    return oracle_edit(text, oracle_record_is("compacted"), change=carry), receipt


def _agent_message_record(delivered: str, recipient: str) -> dict:
    """Codex's own inter-agent message shape, as `internal/testjsonl/testjsonl.go`'s
    `CodexAgentMessageJSON` writes it: the delivered task in the `encrypted_content` member of a block
    typed `encrypted_content`, addressed from one agent path to another. The instant is filled in by
    `_append_agent_message`, which stamps it from the fixture it is appended to."""
    return {"timestamp": "", "type": "response_item", "payload": {
        "type": "agent_message", "author": "/root/worker", "recipient": recipient,
        "content": [{"type": "input_text", "text": "Task received from parent."},
                    {"type": "encrypted_content", "encrypted_content": delivered}]}}


def _with_agent_path(text: str, path: str) -> str:
    """One fixture's text with `agent_path` written into its `session_meta` payload."""
    return oracle_edit(text, oracle_record_is("session_meta"),
                       change=lambda record: record["payload"].__setitem__("agent_path", path))


def _append_agent_message(text: str, delivered: str, recipient: str,
                          content: object | None = None) -> str:
    """One fixture's text with an inter-agent message appended, stamped at the last record's instant so
    the clock stays legal (a record equal to its predecessor is not after it). `content`, where given,
    replaces the record's own block list — the object-valued `content` shape review round r5's finding
    names is written through it."""
    lines = text.splitlines()
    record = _agent_message_record(delivered, recipient)
    if content is not None:
        record["payload"]["content"] = content
    record["timestamp"] = json.loads(lines[-1])["timestamp"]
    return "\n".join(lines + [oracle_quote(record)]) + "\n"


def _append_item(text: str, payload: dict) -> str:
    """One fixture's text with a `response_item` of `payload` appended, stamped at the last record's
    instant so the clock stays legal."""
    lines = text.splitlines()
    record = {"timestamp": json.loads(lines[-1])["timestamp"], "type": "response_item",
              "payload": payload}
    return "\n".join(lines + [oracle_quote(record)]) + "\n"


def _mutation_plan_omitted_agent_message(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The plan's own omitted prompt written into an appended inter-agent message's delivered content.

    Codex's `agent_message` item is a user message to the carried reader where the artifact's
    `session_meta` names an agent path and the record is addressed to it
    (`internal/parser/codex.go`'s `handleAgentMessage`/`extractCodexInboundAgentMessage`), and the
    receipt has no occurrence kind that can name the delivered text: the omitted prompt survives in the
    artifact while both accounting rules compare identities, counts and order that are unchanged (review
    round r4's finding).
    """
    assert receipt is not None
    omitted = oracle_omitted_message(receipt)
    planted = _with_agent_path(text, "/root")
    return _append_agent_message(planted, omitted["text"], "/root"), receipt


def _mutation_plan_omitted_unknown_item(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The plan's own omitted prompt written into an item type the vocabulary does not name.

    The carried reader's `handleResponseItem` falls through its switch for a payload type it does not
    take and reads such a record as a message where its `role` is `user`/`assistant` and its content
    carries readable text (`extractCodexContent` over `extractCodexTextBlocks`); the occurrence
    vocabulary names only the `message` item, so the prompt survives in the model-visible channel while
    every identity, count and order the accounting rules compare is unchanged (the same class review
    round r4's finding states, through one of its other shapes).
    """
    assert receipt is not None
    omitted = oracle_omitted_message(receipt)
    return _append_item(text, {"type": "compat_message", "role": omitted.get("role", "user"),
                               "content": [{"type": "input_text", "text": omitted["text"]}]}), receipt


def _mutation_plan_image_generation_call(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """An `image_generation_call` appended, which a read turns into a tool call the receipt cannot name.

    `handleImageGenerationCall` appends a tool call whose identity is read from the payload's `id` and
    raw text — members the receipt's `call_id`/`name`/`arguments` identity has no place for — so the
    artifact holds an occurrence outside the vocabulary while every count and order the accounting rules
    compare is unchanged.
    """
    assert receipt is not None
    return _append_item(text, {"type": "image_generation_call", "id": "call_oracle_0002",
                               "status": "generating"}), receipt


def _mutation_call_id_duplicated(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "response_item":
            continue
        if (json.loads(line).get("payload") or {}).get("type") != "function_call":
            continue
        return "\n".join(lines[:index + 1] + [line] + lines[index + 1:]) + "\n", receipt
    raise AssertionError("the fixture no longer holds a function_call")


def _mutation_plan_emitted_shortfall(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The artifact loses one dialogue message the plan still marks emitted (a shortfall, not a split).

    Both of the message's records go — the model-visible `response_item` and the turn-view `event_msg`
    — because the plan's `emitted` covers both of the artifact's channels: deleting one alone would
    leave the other as a surplus for `oracle.plan-accounted`, and the mutation would turn two rules red
    rather than its own.
    """
    text = _without_record(text, oracle_item_is("message", "And now?"))
    return _without_record(text, oracle_event_message_is("And now?")), receipt


def _mutation_plan_emitted_view_shortfall(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A message the plan emits keeps its model-visible record while its turn-view event is deleted.

    The artifact's own occurrence list does not move — an `event_msg` is no occurrence of that channel —
    so only the plan's `emitted` mark read against the turn view can see the message the harness's own
    view no longer holds.
    """
    return _without_record(text, oracle_event_message_is("Which fixture is this?")), receipt


def _mutation_plan_boundary_without_a_summary(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A `compacted` record appended with an empty payload, which the plan names nowhere.

    The carried reader records a boundary for every `compacted` record, summary or not
    (`internal/parser/codex.go`'s `handleCompacted`), so the artifact holds an occurrence the receipt
    never planned although every rule that reads only string summaries — and every count — stayed
    unchanged.
    """
    lines = text.splitlines()
    stamp = json.loads(lines[-1])["timestamp"]
    lines.append(oracle_quote({"timestamp": stamp, "type": "compacted", "payload": {}}))
    return "\n".join(lines) + "\n", receipt


def _mutation_plan_omitted_present(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The plan marks the compaction boundary omitted while the artifact still holds it."""
    assert receipt is not None
    return text, oracle_receipt_edit(receipt, oracle_entry_kind("compaction_boundary"),
                                     _entry_disposition_omitted)


def _mutation_plan_names_it_nowhere(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The plan names the boundary nowhere at all: the artifact holds an occurrence it never planned."""
    assert receipt is not None
    kept = [entry for entry in receipt["occurrences"]
            if entry.get("kind") != "compaction_boundary"]
    if len(kept) != len(receipt["occurrences"]) - 1:
        raise AssertionError("the fixture's plan no longer holds the boundary occurrence")
    receipt["occurrences"] = kept
    return text, receipt


def _mutation_plan_omitted_event_survives(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A message the plan marks omitted still stands in the artifact's turn view.

    The appended records are the full session's own events, taken by the texts the reduced plan marks
    omitted, without their `response_item` counterparts: the model-visible channel then holds exactly
    the plan's emitted set, so only the turn-view half of `oracle.plan-accounted` can refuse the
    artifact — which is exactly the defect the reduced plan claims not to have (excluded dialogue
    surviving in the channel the harness's own view of the turn is built from).
    """
    assert receipt is not None
    omitted = {entry.get("text") for entry in receipt["occurrences"]
               if entry.get("kind") == "message" and entry.get("disposition") == "omitted"}
    if not omitted:
        raise AssertionError("the fixture's plan marks no message omitted")
    events = [line for line in oracle_fixture("session.jsonl").splitlines()
              if (json.loads(line).get("type") == "event_msg"
                  and (json.loads(line).get("payload") or {}).get("message") in omitted)]
    if len(events) != len(omitted):
        raise AssertionError("the session fixture holds no event for every omitted message")
    return "\n".join(text.splitlines() + events) + "\n", receipt


def _mutation_plan_order_boundary_moved(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The compaction boundary moved past the final response, stamped with that record's instant.

    Every count is unchanged, the clock stays nondecreasing, and every structural rule still holds —
    but the boundary now stands after the second turn, so the target applies it to history that
    includes that turn where the plan's own order puts it between the two. Only the order comparison
    can see the move.
    """
    assert receipt is not None
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "compacted":
            continue
        boundary = json.loads(lines.pop(index))
        boundary["timestamp"] = json.loads(lines[-1])["timestamp"]
        lines.append(oracle_quote(boundary))
        return "\n".join(lines) + "\n", receipt
    raise AssertionError("the fixture no longer holds a compacted record")


def _mutation_plan_order_view_swapped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The two user turns' turn-view events trade their messages.

    Every count is unchanged, the model-visible channel and its order are unchanged, and the clock is
    untouched: only the turn view puts the prompts in each other's turns — the channel the harness's own
    view of the conversation is built from, so the two prompts swapped are a different conversation.
    """
    lines = text.splitlines()
    events = [index for index, line in enumerate(lines)
              if oracle_event_is("user_message")(json.loads(line))]
    if len(events) != 2:
        raise AssertionError("the fixture no longer holds two user_message events")
    first, second = (json.loads(lines[index]) for index in events)
    first["payload"]["message"], second["payload"]["message"] = (second["payload"]["message"],
                                                                 first["payload"]["message"])
    lines[events[0]], lines[events[1]] = oracle_quote(first), oracle_quote(second)
    return "\n".join(lines) + "\n", receipt


def _mutation_plan_pair_split(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A call the plan emits while its recorded result is marked omitted, and the result is not there.

    The artifact is made to hold the call without its output, which is exactly the shape the receipt
    then describes: the plan's own split is the defect, and the artifact cannot be asked about it (a call
    with no recorded result is legitimate, so no structural rule forbids a bare call).
    """
    assert receipt is not None
    return (_without_record(text, oracle_item_is("function_call_output", "call_oracle_0001")),
            oracle_receipt_edit(receipt, oracle_entry_kind("tool_result"), _entry_disposition_omitted))


def _mutation_receipt_disposition_unknown(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    assert receipt is not None
    return text, oracle_receipt_edit(receipt, oracle_entry_kind("message"),
                                     _entry_disposition_kept)


def _mutation_receipt_call_id_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    assert receipt is not None
    return text, oracle_receipt_edit(receipt, oracle_entry_kind("tool_result"),
                                     _entry_call_id_dropped)


def _mutation_receipt_result_text_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A result entry without the recorded text its identity needs: the result cannot be named."""
    assert receipt is not None
    return text, oracle_receipt_edit(receipt, oracle_entry_kind("tool_result"),
                                     lambda entry: entry.pop("text"))


#: Every rule, and the negative fixture (or fixtures) that removes it: each mutation is applied to the
#: named committed artifact (and plan, where it names one) and must turn **its own rule** red and no
#: other — which is what makes the oracle a reading of one defect at a time rather than a net.
ORACLE_MUTATIONS: dict[str, tuple[OracleMutation, ...]] = {
    "oracle.jsonl-objects": (
        OracleMutation("a line that is not JSON", "session.jsonl", None, _mutation_line_not_json),
        OracleMutation("a line that parses as an array", "session.jsonl", None, _mutation_line_is_an_array),
        OracleMutation("a line holding a non-JSON constant", "session.jsonl", None,
                       _mutation_line_non_json_constant),
        OracleMutation("a line nested past reading", "session.jsonl", None,
                       _mutation_line_nested_past_reading),
        OracleMutation("a line holding a member name twice", "session.jsonl", None,
                       _mutation_line_duplicate_member),
    ),
    "oracle.envelope": (
        OracleMutation("a record without its timestamp", "session.jsonl", None,
                       _mutation_envelope_without_timestamp),
        OracleMutation("a payload that is not an object", "session.jsonl", None,
                       _mutation_envelope_payload_not_an_object),
        OracleMutation("an offset the format forbids", "session.jsonl", None,
                       _mutation_envelope_offset_out_of_range),
        OracleMutation("an instant at the leap second", "session.jsonl", None,
                       _mutation_envelope_leap_second),
        OracleMutation("an instant with a trailing line feed", "session.jsonl", None,
                       _mutation_envelope_trailing_newline),
        OracleMutation("an instant written with a non-ASCII digit", "session.jsonl", None,
                       _mutation_envelope_unicode_digits),
    ),
    "oracle.record-type": (
        OracleMutation("a record type the format does not define", "session.jsonl", None,
                       _mutation_record_type_unknown),
    ),
    "oracle.session-meta": (
        OracleMutation("the metadata record deleted", "session.jsonl", None, _mutation_session_meta_deleted),
        OracleMutation("a second metadata record", "session.jsonl", None, _mutation_session_meta_duplicated),
        OracleMutation("every record deleted", "session.jsonl", None, _mutation_every_record_deleted),
    ),
    "oracle.session-meta-members": (
        OracleMutation("an empty id", "session.jsonl", None, _mutation_session_meta_id_empty),
        OracleMutation("no cli_version", "session.jsonl", None, _mutation_session_meta_cli_version_dropped),
    ),
    "oracle.clock": (
        OracleMutation("two adjacent instants swapped", "session.jsonl", None, _mutation_clock_swapped),
    ),
    "oracle.message": (
        OracleMutation("a message with no content", "session.jsonl", None, _mutation_message_content_empty),
        OracleMutation("a block the reader does not read", "session.jsonl", None,
                       _mutation_message_block_type_not_read),
        OracleMutation("a message whose text is blank", "session.jsonl", None, _mutation_message_text_blank),
    ),
    "oracle.event-message": (
        OracleMutation("an event message that is not a string", "session.jsonl", None,
                       _mutation_event_message_not_a_string),
        OracleMutation("an event message that is blank", "session.jsonl", None,
                       _mutation_event_message_blank),
    ),
    "oracle.call-pairing": (
        OracleMutation("an output naming no call", "session.jsonl", None,
                       _mutation_output_names_an_absent_call),
        OracleMutation("a call id carried twice", "session.jsonl", None, _mutation_call_id_duplicated),
        OracleMutation("an output written before its call", "session.jsonl", None,
                       _mutation_output_before_its_call),
    ),
    "oracle.receipt": (
        OracleMutation("a disposition the vocabulary does not hold", "session.jsonl",
                       "session-receipt.json", _mutation_receipt_disposition_unknown),
        OracleMutation("a result entry with no call_id", "session.jsonl", "session-receipt.json",
                       _mutation_receipt_call_id_dropped),
        OracleMutation("a result entry with no recorded text", "session.jsonl", "session-receipt.json",
                       _mutation_receipt_result_text_dropped),
        OracleMutation("a receipt that declares no occurrences", "session.jsonl", "session-receipt.json",
                       _mutation_receipt_declares_nothing),
    ),
    "oracle.plan-emitted": (
        OracleMutation("a message the plan emits and the artifact lost", "session.jsonl",
                       "session-receipt.json", _mutation_plan_emitted_shortfall),
        OracleMutation("an emitted message whose turn-view event is gone", "session.jsonl",
                       "session-receipt.json", _mutation_plan_emitted_view_shortfall),
    ),
    "oracle.plan-accounted": (
        OracleMutation("an occurrence the plan marks omitted and the artifact holds", "session.jsonl",
                       "session-receipt.json", _mutation_plan_omitted_present),
        OracleMutation("an occurrence the plan names nowhere", "session.jsonl", "session-receipt.json",
                       _mutation_plan_names_it_nowhere),
        OracleMutation("a boundary the plan names nowhere", "session.jsonl", "session-receipt.json",
                       _mutation_plan_boundary_without_a_summary),
        OracleMutation("a boundary carrying the dialogue the plan omits", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_boundary_replacement_history),
        OracleMutation("an omitted message written into an inter-agent message", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_omitted_agent_message),
        OracleMutation("an omitted message written into an item type the receipt cannot name",
                       "reduced.jsonl", "reduced-receipt.json", _mutation_plan_omitted_unknown_item),
        OracleMutation("an image-generation call the receipt cannot name", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_image_generation_call),
        OracleMutation("an omitted message that survives in the turn view", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_omitted_event_survives),
    ),
    "oracle.plan-order": (
        OracleMutation("a compaction boundary moved past the later turns", "session.jsonl",
                       "session-receipt.json", _mutation_plan_order_boundary_moved),
        OracleMutation("two prompts swapped between the turns' view events", "session.jsonl",
                       "session-receipt.json", _mutation_plan_order_view_swapped),
    ),
    "oracle.plan-pairs": (
        OracleMutation("a call emitted while its recorded result is omitted", "session.jsonl",
                       "session-receipt.json", _mutation_plan_pair_split),
    ),
}


class CodexArtifactOracle(unittest.TestCase):
    """The frozen oracle (goal 0009a): its committed fixtures, its mutations, and its `--validate` mode.

    Every case drives the shipped module in-process, imported by path, with no harness and no source
    session: the oracle's subject is an artifact's own bytes, so a case that needed the Codex binary or a
    Claude transcript would not be a gate step. The rule table's other end is `ORACLE_MUTATIONS` above:
    each rule has at least one mutation that removes it, each mutation turns its own rule red and no
    other, and the two tables are in bijection — which is the increment's D1 in the form the required
    `selftest` step can re-derive.
    """

    def test_the_committed_fixtures_validate(self) -> None:
        """The positive fixtures are green, with and without their plans, and the reduced one is the
        full session's own records with the turn its plan marks omitted taken out.

        Both readings of "reduced" are pinned here: the reduced artifact is the full fixture's lines as a
        subsequence (so it is the same session's records, in the source's own order), and its occurrence
        count is the full one's minus exactly the two messages its plan marks omitted — an artifact that
        had merely dropped a rule, or that still held the turn, could not satisfy both.
        """
        session = oracle_fixture("session.jsonl")
        reduced = oracle_fixture("reduced.jsonl")
        for name, text, receipt in (
            ("session.jsonl", session, None),
            ("session.jsonl + its plan", session, "session-receipt.json"),
            ("reduced.jsonl", reduced, None),
            ("reduced.jsonl + its plan", reduced, "reduced-receipt.json"),
        ):
            with self.subTest(fixture=name):
                violations = (NATIVE_CHECK.validate_artifact(text, receipt=oracle_receipt(receipt))
                              if receipt else NATIVE_CHECK.validate_artifact(text))
                self.assertEqual(violations, [], f"{name} is not green")
        lines = session.splitlines()
        cursor = 0
        for line in reduced.splitlines():
            while cursor < len(lines) and lines[cursor] != line:
                cursor += 1
            self.assertLess(cursor, len(lines),
                            f"the reduced fixture holds a line the full fixture does not, in order: {line}")
            cursor += 1
        counted = [len(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(text)[0]))
                   for text in (session, reduced)]
        self.assertEqual(counted, [7, 5],
                         f"the fixtures' own occurrences are {counted}: the reduced artifact is the "
                         f"session with the two messages its plan marks omitted left out")

    def test_a_reduced_artifact_is_validated_from_the_artifact_alone(self) -> None:
        """A reduced artifact and its plan are validated with nothing but the two documents in hand.

        The validator is driven with the three Python-level names a file read goes through —
        `builtins.open`, `io.open` (which `pathlib`'s `read_text`/`open` resolve at call time) and
        `os.open` — all refusing to hand out any file, and the case first shows each refusal fires, so a
        reading that reached for a source transcript, the fixture on disk or any other document through
        any of them could not have answered: the oracle's input is the artifact's text and, at most, a
        plan (goal 0009a, D2). The full artifact against the reduced plan is the other direction — each
        occurrence the plan marks omitted is present there, in both of the artifact's dialogue channels,
        and each channel names its own record — which is what keeps "reduced" a statement about the
        artifact rather than about an oracle that ignores plans.
        """
        reduced = oracle_fixture("reduced.jsonl")
        receipt = oracle_receipt("reduced-receipt.json")

        def refuse(*args, **kwargs):
            raise AssertionError("the oracle opened a file: its input is the artifact, not a path")

        with mock.patch("builtins.open", refuse), mock.patch("io.open", refuse), \
                mock.patch("os.open", refuse):
            for open_a_file in (lambda: open(__file__),
                                lambda: io.open(__file__),
                                lambda: os.open(__file__, os.O_RDONLY),
                                lambda: Path(__file__).read_text(encoding="utf-8")):
                with self.assertRaises(AssertionError, msg="a file read was not refused"):
                    open_a_file()
            self.assertEqual(NATIVE_CHECK.validate_artifact(reduced, receipt=receipt), [],
                             "the reduced artifact and its plan are not green from their own texts")
        present = NATIVE_CHECK.validate_artifact(oracle_fixture("session.jsonl"), receipt=receipt)
        self.assertEqual([violation.rule for violation in present], ["oracle.plan-accounted"] * 4,
                         f"the unreduced artifact against the reduced plan: {present}")
        self.assertEqual([violation.line for violation in present], [14, 15, 16, 17],
                         f"the plan's omitted occurrences are not the ones the artifact holds: {present}")

    def test_every_rule_has_a_mutation_that_turns_its_fixture_red(self) -> None:
        """The rule table and the mutation table are one contract, read from both ends.

        Each rule states something; each has at least one mutation; each mutation leaves its fixture
        changed (a mutation that edited nothing would pass for a rule nothing can break) and turns
        exactly its own rule red, with a detail that says what was found — no other rule, so a report is
        a reading of one defect rather than a net that catches everything at once.
        """
        self.assertEqual(set(ORACLE_MUTATIONS), set(NATIVE_CHECK.ORACLE_RULES),
                         "a rule has no mutation, or a mutation has no rule")
        for rule, sentence in NATIVE_CHECK.ORACLE_RULES.items():
            self.assertTrue(sentence.strip(), f"the rule {rule} states nothing")
        for rule, mutations in sorted(ORACLE_MUTATIONS.items()):
            self.assertTrue(mutations, f"{rule} has no mutation")
            for mutation in mutations:
                with self.subTest(rule=rule, mutation=mutation.label):
                    text = oracle_fixture(mutation.artifact)
                    receipt = oracle_receipt(mutation.receipt) if mutation.receipt else None

                    def validate(artifact_text, plan=receipt):
                        return (NATIVE_CHECK.validate_artifact(artifact_text, receipt=plan)
                                if plan is not None else NATIVE_CHECK.validate_artifact(artifact_text))

                    base = validate(text)
                    self.assertEqual(base, [],
                                     f"{mutation.artifact} is not green before {mutation.label!r}: {base}")
                    before = (text, json.dumps(receipt, sort_keys=True))
                    mutated_text, mutated_receipt = mutation.mutate(text, receipt)
                    after = (mutated_text, json.dumps(mutated_receipt, sort_keys=True))
                    self.assertNotEqual(after, before,
                                        f"the mutation {mutation.label!r} changed nothing")
                    violations = validate(mutated_text, mutated_receipt)
                    self.assertEqual({violation.rule for violation in violations}, {rule},
                                     f"{mutation.label!r} on {mutation.artifact} turned "
                                     f"{sorted({violation.rule for violation in violations})} red "
                                     f"instead of only {rule}: {violations}")
                    for violation in violations:
                        self.assertTrue(violation.detail.strip(),
                                        f"the {rule} violation says nothing: {violation}")

    def test_the_instant_reader_dates_an_offset_by_its_sign(self) -> None:
        """`oracle_instant` dates the format's numeric offsets by their sign, not only its `Z`.

        `oracle.envelope` admits every spelling `ORACLE_INSTANT` reads, and every committed fixture
        spells its instants with `Z`, so the offset branch is pinned only here: an offset read by adding
        where it must subtract — or the reverse — dates `10:15+01:00` an hour away from the `Z` spelling
        of the same instant, which the whole mutation table would pass while the envelope rule admitted
        the record. Equality, not ordering: a uniform misreading keeps any order the file has.
        """
        self.assertEqual(NATIVE_CHECK.oracle_instant("2026-05-04T10:15:00+01:00"),
                         NATIVE_CHECK.oracle_instant("2026-05-04T09:15:00Z"),
                         "an ahead-of-UTC offset did not subtract")
        self.assertEqual(NATIVE_CHECK.oracle_instant("2026-05-04T08:15:00-01:00"),
                         NATIVE_CHECK.oracle_instant("2026-05-04T09:15:00Z"),
                         "a behind-UTC offset did not add")

    def test_occurrence_identity_is_counted_not_deduplicated(self) -> None:
        """The same message written twice is two occurrences, and a plan that counts it once refuses it.

        Multiplicity is part of an occurrence's identity rather than a bookkeeping detail: the receipt's
        marks are counted, so `emitted` and `omitted` entries of one identity never silently cancel, and
        a dedup by identity — which the rest of this suite would pass — is refused here. The duplicated
        line repeats an instant, which keeps the clock legal (a record equal to its predecessor is not
        after it), so the refusal is the plan's own and not a structural rule's.
        """
        session = oracle_fixture("session.jsonl")
        lines = session.splitlines()
        for index, line in enumerate(lines):
            if not oracle_item_is("message", "And now?")(json.loads(line)):
                continue
            duplicated = "\n".join(lines[:index + 1] + [line] + lines[index + 1:]) + "\n"
            break
        else:
            self.fail("the fixture no longer holds the message this case duplicates")
        self.assertEqual(NATIVE_CHECK.validate_artifact(duplicated), [],
                         "the duplicated artifact is not structurally green")
        counted = len(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(duplicated)[0]))
        self.assertEqual(counted, 8, f"the duplicate is not counted: {counted} occurrences")
        refused = NATIVE_CHECK.validate_artifact(duplicated,
                                                 receipt=oracle_receipt("session-receipt.json"))
        self.assertEqual([violation.rule for violation in refused], ["oracle.plan-accounted"],
                         f"a plan counting the message once did not refuse the artifact: {refused}")
        self.assertIn("the artifact holds 2, the receipt marks 1 emitted", refused[0].detail,
                      f"the refusal does not count the duplicate: {refused[0]}")

    def test_a_result_substituted_under_one_call_id_is_refused(self) -> None:
        """Two results under one call id are distinguishable, and their recorded text is the identity.

        The defect this pins: with a result read by its `call_id` alone, an output record replaced by
        different text under the same call id left every count — records, occurrences, the pairing rule
        — unchanged, so a plan that named the original text could not tell the substitution from the
        recorded result (the writer itself records several results under one call id:
        `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults` in `internal/ccc`). The
        result's recorded text is part of its identity here, so the plan's emitted occurrence is missing
        from the artifact and the substituted one is unaccounted — two findings for the one
        substitution, which is what the artifact and the plan say when read against each other.
        """
        session = oracle_fixture("session.jsonl")
        substituted = oracle_edit(session,
                                  oracle_item_is("function_call_output", "call_oracle_0001"),
                                  change=_output_text_substituted)
        self.assertEqual(NATIVE_CHECK.validate_artifact(substituted), [],
                         "the substituted artifact is not structurally green")
        self.assertEqual([len(NATIVE_CHECK.oracle_records(text)[0]) for text in (session, substituted)],
                         [17, 17], "the substitution changed the record count it must preserve")
        self.assertEqual([len(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(text)[0]))
                          for text in (session, substituted)],
                         [7, 7], "the substitution changed the occurrence count it must preserve")
        refused = NATIVE_CHECK.validate_artifact(substituted,
                                                 receipt=oracle_receipt("session-receipt.json"))
        by_rule = {violation.rule: violation for violation in refused}
        self.assertEqual(sorted(by_rule), ["oracle.plan-accounted", "oracle.plan-emitted"],
                         f"the same-id output substitution was not refused: {refused}")
        self.assertIn("the receipt marks 1 emitted, the artifact holds 0",
                      by_rule["oracle.plan-emitted"].detail, f"{by_rule['oracle.plan-emitted']}")
        self.assertIn(f"tool result {NATIVE_CHECK.oracle_digest('call_oracle_0001')}",
                      by_rule["oracle.plan-accounted"].detail,
                      f"{by_rule['oracle.plan-accounted']}")

    def test_a_call_substituted_under_one_call_id_is_refused(self) -> None:
        """Two invocations under one call id are distinguishable, and name and input are the identity.

        The defect this pins (review round r2's finding 1): with a call read by its `call_id` alone, the
        same call id certified a different invocation — the recorded `arguments`, or the emitted tool
        `name`, replaced while the call id, the output, every count and the receipt's marks stayed the
        same. The call's own name and recorded input are part of its identity here, so the plan's
        emitted occurrence is missing from the artifact and the substituted one is unaccounted — two
        findings for the one substitution, the same shape a substituted result gives.
        """
        session = oracle_fixture("session.jsonl")
        for label, change in (("arguments", _call_arguments_substituted),
                              ("name", _call_name_substituted)):
            with self.subTest(substituted=label):
                substituted = oracle_edit(session, oracle_item_is("function_call", "call_oracle_0001"),
                                          change=change)
                self.assertEqual(NATIVE_CHECK.validate_artifact(substituted), [],
                                 "the substituted artifact is not structurally green")
                self.assertEqual(
                    [len(NATIVE_CHECK.oracle_records(text)[0]) for text in (session, substituted)],
                    [17, 17], "the substitution changed the record count it must preserve")
                self.assertEqual(
                    [len(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(text)[0]))
                     for text in (session, substituted)],
                    [7, 7], "the substitution changed the occurrence count it must preserve")
                refused = NATIVE_CHECK.validate_artifact(substituted,
                                                         receipt=oracle_receipt("session-receipt.json"))
                by_rule = {violation.rule: violation for violation in refused}
                self.assertEqual(sorted(by_rule), ["oracle.plan-accounted", "oracle.plan-emitted"],
                                 f"the same-id call substitution was not refused: {refused}")
                self.assertIn("the receipt marks 1 emitted, the artifact holds 0",
                              by_rule["oracle.plan-emitted"].detail,
                              f"{by_rule['oracle.plan-emitted']}")
                self.assertIn(f"tool call {NATIVE_CHECK.oracle_digest('call_oracle_0001')}",
                              by_rule["oracle.plan-accounted"].detail,
                              f"{by_rule['oracle.plan-accounted']}")

    def test_a_custom_calls_recorded_input_is_part_of_its_identity(self) -> None:
        """A `custom_tool_call`'s recorded input — the member Codex's own files write it in — is the
        identity's input, and a receipt names it in its `arguments` member.

        The carried reader reads a call's recorded input from `arguments` then `input`
        (`internal/parser/codex.go`'s `extractCodexInputJSON`), and Codex's own custom calls carry it in
        `input` (`internal/parser/codex_provider_test.go`'s fixture): reading only `arguments` let a
        receipt with an empty one certify a custom call whatever its input, and made a receipt naming
        the actual input a mismatch (review round r3's finding). The case replaces the fixture's
        `function_call` with an input-only `custom_tool_call`: structurally green, counted by its input
        (asserted on the occurrence identity itself), accepted when the receipt's `arguments` names that
        input, and refused — in both accounting rules — when it names nothing or another invocation.
        """
        session = oracle_fixture("session.jsonl")

        def to_custom(record: dict) -> None:
            record["payload"] = {"type": "custom_tool_call", "name": "apply_patch",
                                 "call_id": "call_oracle_0001",
                                 "input": "*** Begin Patch\n*** End Patch"}

        custom = oracle_edit(session, oracle_item_is("function_call", "call_oracle_0001"),
                             change=to_custom)
        self.assertEqual(NATIVE_CHECK.validate_artifact(custom), [],
                         "the custom call is not structurally green")
        keys = [key for key, _ in
                NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(custom)[0])]
        self.assertIn(("tool_call", "call_oracle_0001", "apply_patch",
                       "*** Begin Patch\n*** End Patch"), keys,
                      f"the custom call is not counted by its recorded input: {keys}")

        def receipt_for(arguments: str) -> dict:
            plan = oracle_receipt("session-receipt.json")
            for entry in plan["occurrences"]:
                if entry.get("kind") == "tool_call":
                    entry["name"] = "apply_patch"
                    entry["arguments"] = arguments
            return plan

        self.assertEqual(
            NATIVE_CHECK.validate_artifact(custom, receipt=receipt_for("*** Begin Patch\n*** End Patch")),
            [], "the receipt naming the recorded input did not match the custom call")
        for arguments in ("", '{"file_path":"/work/oracle-fixture/notes.md"}'):
            with self.subTest(arguments=arguments):
                refused = NATIVE_CHECK.validate_artifact(custom, receipt=receipt_for(arguments))
                self.assertEqual(sorted({violation.rule for violation in refused}),
                                 ["oracle.plan-accounted", "oracle.plan-emitted"],
                                 f"a receipt naming {arguments!r} certified the custom call: {refused}")

    def test_a_written_empty_object_input_is_part_of_the_identity(self) -> None:
        """The writer's own `arguments: "{}"` is the written input, and a receipt naming it matches.

        This route's writer emits `arguments: "{}"` for a call whose source recorded no input
        (`internal/ccc/codex_target.go`: `input = "{}"`, counted as `lossEmptyInputs`), and the carried
        reader's `extractCodexInputJSON` skips that value — so an identity written from the read made the
        written `"{}"` and a missing `arguments` member the same occurrence: a receipt naming the written
        `"{}"` was a mismatch against the correct artifact, while a receipt using the read's empty
        identity certified both that artifact and one a mutation left with no member at all (review round
        r4's finding). The identity is the written member here, so the receipt naming `"{}"` matches, the
        read's empty projection is a different occurrence (refused in both accounting rules), and the
        mutation the finding names — the member removed, the call id and its recorded result untouched —
        is refused too. A call carrying **both** members is refused where a plan is compared (review
        round r5's finding): the written identity stands, but the carried read skips it and continues to
        `input`, so the receipt's single identity is not a statement about the invocation a read answers.
        """
        session = oracle_fixture("session.jsonl")
        written = oracle_edit(session, oracle_item_is("function_call", "call_oracle_0001"),
                              change=lambda record: record["payload"].__setitem__("arguments", "{}"))
        dropped = oracle_edit(written, oracle_item_is("function_call", "call_oracle_0001"),
                              change=lambda record: record["payload"].pop("arguments"))
        keys = [key for key, _ in
                NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(written)[0])]
        self.assertIn(("tool_call", "call_oracle_0001", "Read", "{}"), keys,
                      f"the written empty object is not the call's recorded input: {keys}")
        keys = [key for key, _ in
                NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(dropped)[0])]
        self.assertIn(("tool_call", "call_oracle_0001", "Read", ""), keys,
                      f"the removed member is not a distinct occurrence: {keys}")

        def receipt_for(arguments: str) -> dict:
            plan = oracle_receipt("session-receipt.json")
            for entry in plan["occurrences"]:
                if entry.get("kind") == "tool_call":
                    entry["arguments"] = arguments
            return plan

        self.assertEqual(NATIVE_CHECK.validate_artifact(written), [],
                         "the call written with `arguments: \"{}\"` is not structurally green")
        # The two written forms are two occurrences: each artifact matches the receipt that names its own
        # written input, and each is refused — in both accounting rules — by the receipt that names the
        # other. Under the read's identity (`extractCodexInputJSON` skips `{}` and answers `""`) the
        # first arm was a mismatch and the third certified both artifacts, so this table is the finding.
        for label, artifact, receipt, expected in (
            ("the written empty object against a receipt naming it", written, receipt_for("{}"), []),
            ("the written empty object against the read's empty identity", written, receipt_for(""),
             ["oracle.plan-accounted", "oracle.plan-emitted"]),
            ("the removed member against the read's empty identity", dropped, receipt_for(""), []),
            ("the removed member against the written empty object", dropped, receipt_for("{}"),
             ["oracle.plan-accounted", "oracle.plan-emitted"]),
        ):
            with self.subTest(arm=label):
                refused = NATIVE_CHECK.validate_artifact(artifact, receipt=receipt)
                self.assertEqual(sorted({violation.rule for violation in refused}), expected,
                                 f"{label}: {refused}")
        for text in (session, written, dropped):
            self.assertEqual(
                len(NATIVE_CHECK.oracle_records(text)[0]), 17,
                "the case's own edits changed the record count they must preserve")
            self.assertEqual(
                len(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(text)[0])), 7,
                "the case's own edits changed the occurrence count they must preserve")
        # The mutation the finding names leaves the call's id and its recorded result untouched: only the
        # input member differs between the two artifacts.
        def call_payload(text: str) -> dict:
            for line in text.splitlines():
                record = json.loads(line)
                if oracle_item_is("function_call", "call_oracle_0001")(record):
                    return record["payload"]
            raise AssertionError("the case's artifact lost the call")

        written_payload, dropped_payload = call_payload(written), call_payload(dropped)
        self.assertEqual({key: value for key, value in dropped_payload.items() if key != "arguments"},
                         {key: value for key, value in written_payload.items() if key != "arguments"},
                         "the removed member changed more than the call's own input")
        self.assertEqual([line for line in dropped.splitlines() if '"function_call_output"' in line],
                         [line for line in written.splitlines() if '"function_call_output"' in line],
                         "the removed member changed the call's recorded result")

        # A call carrying **both** members (review round r5's finding): the written identity stays the
        # `"{}"` the arms above pin — that identity is what the receipt names — while the carried
        # reader's `extractCodexInputJSON` skips it and continues to `input`
        # (`TestParseCodexSession_InputJSON`'s `string-encoded empty JSON falls through to input` subtest
        # pins that read), so a receipt naming the written member is no longer a statement about the
        # invocation a read answers. Receipt mode refuses the shape rather than certifying either
        # member; without a plan the artifact is green, because a native rollout may carry the pair.
        both = oracle_edit(written, oracle_item_is("function_call", "call_oracle_0001"),
                           change=lambda record: record["payload"].__setitem__(
                               "input", '{"file_path":"/work/oracle-fixture/other.md"}'))
        self.assertEqual(NATIVE_CHECK.validate_artifact(both), [],
                         "the two-member call is not structurally green")
        keys = [key for key, _ in
                NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(both)[0])]
        self.assertIn(("tool_call", "call_oracle_0001", "Read", "{}"), keys,
                      f"the written member is no longer the call's identity: {keys}")
        refused = NATIVE_CHECK.validate_artifact(both, receipt=receipt_for("{}"))
        self.assertEqual([violation.rule for violation in refused], ["oracle.plan-accounted"],
                         f"a receipt naming the written `{{}}` certified the two-member call: {refused}")
        self.assertIn("both `arguments` and `input`", refused[0].detail, f"{refused}")
        self.assertNotIn("other.md", refused[0].detail,
                         f"the refusal echoed the second member's value: {refused}")

    def test_a_boundarys_replacement_history_is_refused_with_a_plan(self) -> None:
        """The omitted prompt written into a boundary's `replacement_history` is refused in receipt mode.

        A native `compacted` record carries the messages it replaced in `replacement_history` beside its
        summary (`internal/parser/codex_fidelity_test.go`), and this route's writer emits only the
        summary: the receipt's boundary identity is that text alone, so the plan cannot account for
        dialogue written in the member even though every identity, count and order it compares is
        unchanged (review round r3's finding, whose own attack wrote the reduced plan's omitted prompt
        into the reduced boundary). Refused only where a plan is compared — the same artifact is
        structurally green, because a native rollout this oracle is not asked to plan against may carry
        the member — and an empty `replacement_history` holds no dialogue and is not refused.
        """
        reduced = oracle_fixture("reduced.jsonl")
        armed, plan = _mutation_plan_boundary_replacement_history(
            reduced, oracle_receipt("reduced-receipt.json"))
        self.assertEqual(NATIVE_CHECK.validate_artifact(armed), [],
                         "the armed boundary is not structurally green")
        refused = NATIVE_CHECK.validate_artifact(armed, receipt=plan)
        self.assertEqual([violation.rule for violation in refused], ["oracle.plan-accounted"],
                         f"the boundary carrying the omitted dialogue was not refused: {refused}")
        self.assertIn("replacement_history", refused[0].detail, f"{refused[0]}")
        self.assertIn("And now?", armed,
                      "the armed artifact no longer holds the dialogue the attack plants")
        empty = oracle_edit(
            reduced, oracle_record_is("compacted"),
            change=lambda record: record["payload"].__setitem__("replacement_history", []))
        self.assertEqual(
            NATIVE_CHECK.validate_artifact(empty, receipt=oracle_receipt("reduced-receipt.json")), [],
            "an empty replacement_history holds no dialogue and was refused")

    def test_receipt_validation_refuses_the_occurrences_it_cannot_name(self) -> None:
        """The omitted prompt written into a shape the receipt's vocabulary cannot name is refused.

        The finding's own attack: a `response_item` whose payload type is `agent_message` — Codex's
        inter-agent message — is a **user** message to the carried reader where the artifact's
        `session_meta` names an agent path and the record is addressed to it
        (`internal/parser/codex.go`'s `handleAgentMessage` with `extractCodexInboundAgentMessage`;
        `internal/parser/codex_provider_test.go`'s inbound-message fixture pins that read), and the
        receipt has no occurrence kind for it, so the reduced plan's omitted prompt survived inside such a
        record while every identity, count and order the comparison read was unchanged (review round r4).
        The refusal is stated over the class, not the one shape: the reader also appends a message from a
        payload type its switch does not take when the record carries a `user`/`assistant` role and
        readable text, and appends a tool call from an `image_generation_call` whose identity lives in
        members the receipt's `call_id`/`name`/`arguments` identity does not have. A second class is the
        shape whose own read this oracle does not model (review round r5's finding): an object-valued
        `content` is iterated by the reader's own `ForEach` over its **member values**, so the omitted
        prompt written into one is delivered by a read while this oracle's text readers answer nothing —
        refused wherever the read would enter its loop (the fallback branch's role and the inter-agent
        address are the two gates pinned here), and silent where it would not. Each shape is refused
        here — asserted to turn exactly its own rule red — and silent without a plan: a native rollout may
        carry them, so the structural rules say nothing. The controls pin the read's own gates: an address
        that is not the session's own, delivered text Go's own trim calls blank, a session that names no
        agent path, a role outside the reader's two, blank text, and a `reasoning` item, which the reader
        answers as a thinking row with an explicitly empty dialogue projection.
        """
        reduced = oracle_fixture("reduced.jsonl")
        plan = oracle_receipt("reduced-receipt.json")
        planted = _append_agent_message(_with_agent_path(reduced, "/root"), "And now?", "/root")
        self.assertEqual(NATIVE_CHECK.validate_artifact(planted), [],
                         "the planted inter-agent message is not structurally green")
        refused = NATIVE_CHECK.validate_artifact(planted, receipt=plan)
        self.assertEqual([violation.rule for violation in refused], ["oracle.plan-accounted"],
                         f"the omitted prompt inside an inter-agent message certified the artifact: "
                         f"{refused}")
        self.assertIn("inter-agent", refused[0].detail, f"{refused[0]}")
        self.assertNotIn("And now?", refused[0].detail,
                         f"a diagnostic echoed the delivered text: {refused[0]}")
        self.assertIn("And now?", planted,
                      "the planted artifact no longer holds the dialogue the attack writes")

        # The positive arms: every shape a read turns into an occurrence the vocabulary cannot name, and
        # every shape whose own read this oracle does not model (review round r5's finding: an
        # object-valued `content` is iterated by the reader's `ForEach` over its member values, so a
        # prompt written into one is read while this oracle's text readers see nothing). Go's own
        # trimming on both sides and the fallback member a subagent session names its agent path in
        # (`handleSessionMeta`) are pinned with the inter-agent shape, because both are paths the read
        # really takes.
        trimmed = _with_agent_path(reduced, " /root ")
        nested = oracle_edit(
            reduced, oracle_record_is("session_meta"),
            change=lambda record: record["payload"].__setitem__(
                "source", {"subagent": {"thread_spawn": {"agent_path": "/root"}}}))
        for label, artifact in (
            ("a Go-trimmed agent path and address",
             _append_agent_message(trimmed, "And now?", " /root ")),
            ("the agent path's own fallback member",
             _append_agent_message(nested, "And now?", "/root")),
            ("an item type the receipt cannot name, object-valued content",
             _append_item(reduced, {"type": "compat_message", "role": "user",
                                    "content": {"block": {"type": "input_text",
                                                          "text": "And now?"}}})),
            ("an item type the receipt cannot name, assistant role",
             _append_item(reduced, {"type": "compat_message", "role": "assistant",
                                    "content": [{"type": "output_text",
                                                 "text": "Still the synthetic one."}]})),
            ("an inter-agent message whose delivered content is an object",
             _append_agent_message(_with_agent_path(reduced, "/root"), "And now?", "/root",
                                   content={"delivered": {"type": "encrypted_content",
                                                          "encrypted_content": "And now?"}})),
            ("an image-generation call",
             _append_item(reduced, {"type": "image_generation_call", "id": "call_oracle_0002",
                                    "status": "generating"})),
        ):
            with self.subTest(refused=label):
                self.assertEqual(NATIVE_CHECK.validate_artifact(artifact), [],
                                 f"{label} is not structurally green")
                refused = NATIVE_CHECK.validate_artifact(artifact, receipt=plan)
                self.assertEqual([violation.rule for violation in refused], ["oracle.plan-accounted"],
                                 f"{label} was not refused as an unnameable occurrence: {refused}")

        # The control arms: shapes no read of the artifact appends an occurrence from. Each stays green
        # against the reduced plan, and the delivered text is the plan's own omitted prompt in every one.
        controls = (
            ("an address that is not the session's own",
             _append_agent_message(_with_agent_path(reduced, "/root"), "And now?", "/root/other")),
            ("a delivered text Go's own trim calls blank",
             _append_agent_message(_with_agent_path(reduced, "/root"), " \n", "/root")),
            ("a session that names no agent path",
             _append_agent_message(reduced, "And now?", "/root")),
            ("an address Go's trim keeps apart from the path",
             _append_agent_message(_with_agent_path(reduced, "/root"), "And now?", "\x1c/root")),
            ("a role outside the reader's two",
             _append_item(reduced, {"type": "compat_message", "role": "developer",
                                    "content": [{"type": "input_text", "text": "And now?"}]})),
            ("a readable text that is blank",
             _append_item(reduced, {"type": "compat_message", "role": "user",
                                    "content": [{"type": "input_text", "text": "  "}]})),
            ("a reasoning item carrying a role and text",
             _append_item(reduced, {"type": "reasoning", "role": "user",
                                    "content": [{"type": "input_text", "text": "And now?"}]})),
            # The object-valued shapes are refused by the read's own gates, not by their being objects:
            # a role the reader's fallback branch does not take, and an address that is not the
            # session's own, both leave the reader's loop unentered and stay green.
            ("an object-valued content on a role outside the reader's two",
             _append_item(reduced, {"type": "compat_message", "role": "developer",
                                    "content": {"block": {"type": "input_text", "text": "And now?"}}})),
            ("an object-valued delivered content addressed elsewhere",
             _append_agent_message(_with_agent_path(reduced, "/root"), "And now?", "/root/other",
                                   content={"delivered": {"type": "encrypted_content",
                                                          "encrypted_content": "And now?"}})),
        )
        for label, artifact in controls:
            with self.subTest(control=label):
                self.assertEqual(NATIVE_CHECK.validate_artifact(artifact, receipt=plan), [],
                                 f"{label} was refused although no read of it carries an occurrence")

    def test_a_compacted_record_without_a_summary_is_counted(self) -> None:
        """Every `compacted` record is a boundary occurrence, with or without a summary.

        The carried reader appends a boundary for every `compacted` record — `handleCompacted` reads the
        summary where there is one and records an empty boundary where there is not — so an oracle that
        counted only records holding a string `message` let a boundary stand outside the plan: an empty
        payload appended to the artifact left every count it read unchanged and passed with a receipt
        that names no boundary at all (review round r2's finding 2). The boundary is counted here as the
        empty text it reads as, and a receipt that names it accounts for it; the mutation under
        `oracle.plan-accounted` pins the refusal when the plan names it nowhere.
        """
        appended = _mutation_plan_boundary_without_a_summary(oracle_fixture("session.jsonl"), None)[0]
        self.assertEqual(NATIVE_CHECK.validate_artifact(appended), [],
                         "the appended empty boundary is not structurally green")
        counted = NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(appended)[0])
        self.assertEqual(counted[-1][0], ("compaction_boundary", ""),
                         f"the appended compacted record is not an empty boundary: {counted}")
        self.assertEqual(len(counted), 8, f"the boundary is not counted: {counted}")
        receipt = oracle_receipt("session-receipt.json")
        receipt["occurrences"].append({"kind": "compaction_boundary", "text": "",
                                       "disposition": "emitted"})
        self.assertEqual(NATIVE_CHECK.validate_artifact(appended, receipt=receipt), [],
                         "a receipt naming the empty boundary did not account for it")

    def test_the_oracles_blank_test_is_the_writers(self) -> None:
        """U+001C is not space to the writer, so a text of U+001C alone is dialogue, in both channels.

        `strings.TrimSpace` reads Go's `unicode.IsSpace`, which does not hold the information separators
        U+001C–U+001F that Python's `str.strip` (and `str.isspace`) call whitespace; the writer's own
        gate on a message's text (`internal/ccc/codex_target.go`: `text != "" &&
        strings.TrimSpace(text) == ""`) keeps such a text and writes it into the model-visible record
        and its turn-view event, so a rule written with Python's trimming refuses an artifact this
        writer emits (review round r2's finding 4). The case writes the same text into both channels:
        structurally green, and the message still an occurrence whose identity is that text.
        """
        session = oracle_fixture("session.jsonl")
        preserved = oracle_edit(session, oracle_item_is("message", "And now?"),
                                change=_content_text_becomes_file_separator)
        preserved = oracle_edit(preserved, oracle_event_message_is("And now?"),
                                change=_event_message_becomes_file_separator)
        self.assertNotEqual(preserved, session, "the case's two edits changed nothing")
        self.assertEqual(NATIVE_CHECK.validate_artifact(preserved), [],
                         "a text the writer preserves was refused as blank")
        keys = [key for key, _ in
                NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(preserved)[0])]
        self.assertIn(("message", "user", "\x1c"), keys,
                      f"the preserved message is not an occurrence with its own text: {keys}")

    def test_a_malformed_member_is_named_without_echoing_its_text(self) -> None:
        """Default diagnostics name a malformed value by its place and kind, never by its contents.

        A malformed record can put the conversation's own words where any member belongs — an object
        holding a prompt written into `timestamp`, a `call_id` or a receipt entry's `disposition`; a
        *string* can hold them just as well (review round r3's finding: the unmatched-output diagnostic
        echoed a string `call_id` verbatim); and so can a member **name**, which the uniqueness refusal
        reads without echoing (review round r5's finding). The case plants one marker per shape and
        reads `--validate`'s own output for every document: the field is named, the value's kind or
        digest stands where the value would, and the marker is nowhere in either the default or the JSON
        report.
        """
        marker = "SECRET-PROMPT-ALPHA"

        def report(artifact: str, receipt: dict | None) -> str:
            with tempfile.TemporaryDirectory() as scratch:
                artifact_path = Path(scratch) / "planted.jsonl"
                artifact_path.write_text(artifact, encoding="utf-8")
                argv = ["--validate", str(artifact_path)]
                if receipt is not None:
                    plan_path = Path(scratch) / "planted-receipt.json"
                    plan_path.write_text(json.dumps(receipt), encoding="utf-8")
                    argv += ["--receipt", str(plan_path)]
                output = io.StringIO()
                with contextlib.redirect_stdout(output):
                    code = NATIVE_CHECK.main(argv)
            self.assertEqual(code, 1, f"exit {code} for the planted documents: {output.getvalue()}")
            return output.getvalue()

        session = oracle_fixture("session.jsonl")
        planted = oracle_edit(
            session, oracle_event_is("user_message"),
            change=lambda record: record.__setitem__("timestamp", {"prompt": marker}))
        planted = oracle_edit(
            planted, oracle_item_is("function_call_output", "call_oracle_0001"),
            change=lambda record: record["payload"].__setitem__("call_id", {"prompt": marker}))
        receipt = oracle_receipt("session-receipt.json")
        receipt["occurrences"][0]["disposition"] = {"prompt": marker}
        printed = report(planted, receipt)
        self.assertIn("oracle.envelope line 5 — timestamp is an object of 1 member(s)", printed)
        self.assertIn("oracle.call-pairing line 9 — call output's call_id is an object of 1 member(s)",
                      printed)
        self.assertIn("oracle.receipt — occurrences[0].disposition is an object of 1 member(s)",
                      printed)
        self.assertNotIn(marker, printed, f"a diagnostic echoed the planted text: {printed}")

        # A call id is a string an artifact controls, so the report names it by its digest — at the
        # output that names no call, and at the occurrence identity the plan comparison reports.
        carried = f"call {marker}"
        unmatched = oracle_edit(
            session, oracle_item_is("function_call_output", "call_oracle_0001"),
            change=lambda record: record["payload"].__setitem__("call_id", carried))
        printed = report(unmatched, oracle_receipt("session-receipt.json"))
        self.assertIn(f"call output names call_id {NATIVE_CHECK.oracle_digest(carried)}", printed)
        self.assertIn(f"tool result {NATIVE_CHECK.oracle_digest(carried)}", printed)
        self.assertNotIn(marker, printed, f"a diagnostic echoed the planted text: {printed}")

        # A duplicate call id and a pair the receipt splits name the call the same way — by digest.
        duplicated = oracle_edit(
            session, oracle_item_is("function_call", "call_oracle_0001"),
            change=lambda record: record["payload"].__setitem__("call_id", carried))
        duplicated = _mutation_call_id_duplicated(duplicated, None)[0]
        printed = report(duplicated, None)
        self.assertIn(f"call_id {NATIVE_CHECK.oracle_digest(carried)} is already carried", printed)
        self.assertNotIn(marker, printed, f"a diagnostic echoed the planted text: {printed}")

        split = oracle_receipt("session-receipt.json")
        for entry in split["occurrences"]:
            if entry.get("kind") in ("tool_call", "tool_result"):
                entry["call_id"] = carried
            if entry.get("kind") == "tool_result":
                entry["disposition"] = "omitted"
        without_result = _without_record(session,
                                         oracle_item_is("function_call_output", "call_oracle_0001"))
        printed = report(without_result, split)
        self.assertIn(f"call {NATIVE_CHECK.oracle_digest(carried)}: the receipt gives the call",
                      printed)
        self.assertNotIn(marker, printed, f"a diagnostic echoed the planted text: {printed}")

        # A member *name* is a string an artifact controls, too: the refusal of a repeated name (review
        # round r5's finding) must not echo the name it read any more than the other refusals echo their
        # values.
        lines = session.splitlines()
        for index, line in enumerate(lines):
            if json.loads(line).get("type") != "world_state":
                continue
            needle = '"full":true'
            self.assertIn(needle, line,
                          "the fixture's world_state no longer holds the member this arm duplicates")
            lines[index] = line.replace(needle, needle + f',"{marker}":1,"{marker}":1', 1)
            break
        else:
            self.fail("the fixture no longer holds its world_state record")
        printed = report("\n".join(lines) + "\n", None)
        self.assertIn("oracle.jsonl-objects", printed,
                      f"the repeated member name was not refused: {printed}")
        self.assertNotIn(marker, printed, f"a diagnostic echoed the planted name: {printed}")

    def test_the_oracle_reads_the_artifacts_own_bytes(self) -> None:
        """The JSONL framing is the line feed and the parsing is JSON's, not Python's lenient half.

        `str.splitlines` treats more characters than LF as line boundaries — `U+0085` among them — and a
        JSON string may hold one, so a reader that split there would misread one valid line as two
        broken ones; the committed fixtures hold none, so the reading is pinned here. The strictness
        runs the other way too: bytes that are not UTF-8 are reported as such by `--validate` instead of
        being repaired with replacement characters before the rules read them.
        """
        session = oracle_fixture("session.jsonl")
        embedded = session.replace("Which fixture is this?", "Which\u0085fixture is this?")
        self.assertNotEqual(embedded, session, "the fixture no longer holds the message this case edits")
        self.assertEqual(NATIVE_CHECK.validate_artifact(embedded), [],
                         "a line holding U+0085 inside a JSON string was not read as one record")

        with tempfile.TemporaryDirectory() as scratch:
            artifact = Path(scratch) / "not-utf8.jsonl"
            artifact.write_bytes(b'{"timestamp":"2026-05-04T09:15:00Z","type":"session_meta",'
                                 b'"payload":{"id":"\xff"}}\n')
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact)])
            self.assertEqual(code, 2, f"exit {code} for bytes that are not UTF-8")
            self.assertIn("oracle: INCOMPLETE", errors.getvalue(),
                          f"invalid encoding was not reported: {errors.getvalue()}")
            self.assertIn("is not UTF-8 text", errors.getvalue(), errors.getvalue())

    def test_a_line_with_a_duplicate_member_is_refused(self) -> None:
        """A line whose object writes a member name twice is refused before any comparison, in both modes.

        Python's decoder keeps the *last* value for a repeated name and the carried reader's `gjson` the
        *first* (measured on the carried v1.20.0): the reduced plan's omitted prompt planted before the
        first user block's own `text` member is the planned one to a Python read of the line and the
        omitted one to a `gjson` read of the same bytes, so a plan comparison over the former could
        certify dialogue a read of the artifact does not hold — review round r5's finding. The line is
        spliced as raw JSONL rather than written from a dict (`json.dumps` cannot write a repeated name),
        refused structurally and with a plan, and a receipt whose members collide is refused the same way
        through the CLI: a plan that does not read as one object is not compared, exactly as such a line
        is not compared.
        """
        reduced = oracle_fixture("reduced.jsonl")
        needle = '{"type":"input_text","text":"Which fixture is this?"}'
        self.assertIn(needle, reduced, "the fixture no longer holds the first user block this case edits")
        planted_block = '{"type":"input_text","text":"And now?","text":"Which fixture is this?"}'
        armed = reduced.replace(needle, planted_block, 1)
        self.assertNotEqual(armed, reduced, "the case's splice changed nothing")
        # The premise: Python's decoder answers the plan's own prompt for the armed line (the last
        # value), so nothing but the uniqueness clause can see the omitted prompt the carried reader's
        # `gjson` answers from the same bytes (the first value).
        decoy = [json.loads(line) for line in armed.splitlines() if planted_block in line]
        self.assertEqual(len(decoy), 1, "the armed line is not one record")
        self.assertEqual(decoy[0]["payload"]["content"][0]["text"], "Which fixture is this?",
                         "the armed line no longer reads as the plan's own prompt through Python")

        structural = NATIVE_CHECK.validate_artifact(armed)
        self.assertEqual([violation.rule for violation in structural], ["oracle.jsonl-objects"],
                         f"the armed line was not refused for its repeated member: {structural}")
        self.assertIn("written twice", structural[0].detail, f"{structural}")
        refused = NATIVE_CHECK.validate_artifact(armed, receipt=oracle_receipt("reduced-receipt.json"))
        # The refused line's own emitted message is gone from the model-visible channel alone (the
        # turn-view event stands and matches), so the only further rule is the dialogue the reading can
        # no longer see — the armed line never passes, with or without its plan.
        self.assertEqual(sorted({violation.rule for violation in refused}),
                         ["oracle.jsonl-objects", "oracle.plan-emitted"],
                         f"the armed line's plan reading: {refused}")

        with tempfile.TemporaryDirectory() as scratch:
            artifact = Path(scratch) / "armed.jsonl"
            artifact.write_text(armed, encoding="utf-8")
            plan = Path(scratch) / "collided-receipt.json"
            plan.write_text(oracle_fixture("reduced-receipt.json").replace(
                '"disposition": "emitted"', '"disposition": "emitted","disposition": "emitted"', 1),
                encoding="utf-8")
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 2,
                             f"exit {code} for a receipt with a repeated member: {errors.getvalue()}")
            self.assertIn("cannot be read: one member name is written twice", errors.getvalue(),
                          f"the collided receipt is not reported unreadable: {errors.getvalue()}")

    def test_a_receipt_that_declares_nothing_cannot_pass(self) -> None:
        """A plan that declares no occurrences, and one that cannot be read, both refuse the artifact.

        The plan is total or it is not a plan: a receipt whose `occurrences` list is empty declares
        nothing about any dialogue, so it is refused as a receipt *before* comparison — the comparison
        rules would otherwise pass vacuously on a metadata-only artifact that holds no occurrence in
        either channel, which is exactly the case the plan names (review round r3's finding; the
        receipt is then reported and not compared, the same disposition an unreadable receipt gets). A
        receipt that is not an object carrying that list is reported under `oracle.receipt` too. The
        artifact is structurally green and the verdict is FAIL in every arm, so a caller cannot hand in
        nothing and read a pass.
        """
        session = oracle_fixture("session.jsonl")
        metadata_only = "\n".join(
            line for line in session.splitlines()
            if json.loads(line)["type"] in ("session_meta", "world_state", "turn_context",
                                            "token_usage_record")) + "\n"
        self.assertEqual(NATIVE_CHECK.validate_artifact(metadata_only), [],
                         "the metadata-only artifact is not structurally green")
        self.assertEqual(NATIVE_CHECK.oracle_occurrences(NATIVE_CHECK.oracle_records(metadata_only)[0]),
                         [], "the metadata-only artifact holds an occurrence")
        for label, artifact in (("the metadata-only artifact", metadata_only),
                                ("the session fixture", session)):
            with self.subTest(artifact=label):
                refused = NATIVE_CHECK.validate_artifact(artifact, receipt={"occurrences": []})
                self.assertEqual([violation.rule for violation in refused], ["oracle.receipt"],
                                 f"an empty receipt was not refused as a receipt: {refused}")
                self.assertIn("declares no occurrences", refused[0].detail, f"{refused}")
        unreadable = NATIVE_CHECK.validate_artifact(session, receipt={})
        self.assertEqual([violation.rule for violation in unreadable], ["oracle.receipt"],
                         f"a receipt that is not a plan was not named as one: {unreadable}")

    def test_the_citations_manifest_accounts_for_every_rule(self) -> None:
        """Every oracle rule is either a row of this increment's citations manifest or named in it.

        The manifest is this increment's acceptance surface (`docs/conventions.md`), so a rule it neither
        rows nor names in its header is one the ledger silently dropped, and a row whose rule id no longer
        exists is one it kept past its subject. The rows' other end — that each cites a test the gate runs
        and that the test passes — is the required `citations` step's own subject; this case reads the ids
        and the header only.
        """
        manifest = (SCRIPTS.parent / "docs" / "evidence" / "0009a-citations.tsv").read_text(encoding="utf-8")
        lines = manifest.splitlines()
        rows = [line.split("\t")[0] for line in lines
                if line.strip() and not line.startswith("#") and not line.startswith("rule_id")]
        self.assertTrue(rows, "the citations manifest carries no rows")
        unknown = sorted(set(rows) - set(NATIVE_CHECK.ORACLE_RULES))
        self.assertEqual(unknown, [], f"the manifest cites ids that are not oracle rules: {unknown}")
        header = "\n".join(line for line in lines if line.startswith("#"))
        missing = [rule for rule in NATIVE_CHECK.ORACLE_RULES if rule not in rows and rule not in header]
        self.assertEqual(missing, [],
                         f"rules the manifest neither rows nor names as not covered: {missing}")

    def test_decoded_input_never_aborts_validation(self) -> None:
        """A lone surrogate in a decoded string and a string member nested past Python's parser both end
        in a printed verdict, not a traceback.

        Two paths through one defect class (review round r4's finding): `oracle_digest` encoded an
        artifact- or receipt-controlled string as strict UTF-8, and JSON's `\\ud800`-style escape decodes
        to a lone surrogate code point `str.encode("utf-8")` refuses — an unmatched output whose
        `call_id` was such a string raised `UnicodeEncodeError` inside the report; and a call's string
        input was parsed by `_oracle_valid_json`, whose `json.loads` raises `RecursionError` past the
        host's own recursion bound and whose arm caught only `ValueError` — an invocation whose recorded
        input is deeply nested JSON raised out of validation before any verdict. Neither may end a run:
        the digest writes a surrogate as its own three-byte form instead (injective — no other text
        encodes to those bytes — so two identities stay distinguishable; pinned below by the digest
        literal), and no string member is parsed any more, because the identity is the written input. Each
        arm is driven through the shipped CLI, so what is asserted is the documented verdict and exit
        code a caller sees, not an in-process return.
        """
        self.assertEqual(NATIVE_CHECK.oracle_digest("\ud800"), "sha256:91a681b99855",
                         "a lone surrogate is no longer digested as its own three-byte form")
        with tempfile.TemporaryDirectory() as scratch:
            session = oracle_fixture("session.jsonl")
            surrogate = Path(scratch) / "surrogate.jsonl"
            surrogate.write_text(
                oracle_edit(session, oracle_item_is("function_call_output", "call_oracle_0001"),
                            change=lambda record: record["payload"].__setitem__("call_id", "\ud800")),
                encoding="utf-8")
            plan = Path(scratch) / "plan.json"
            plan.write_text(oracle_fixture("session-receipt.json"), encoding="utf-8")
            for label, argv, verdict in (
                ("an unmatched output whose call id is a lone surrogate",
                 ["--validate", str(surrogate)],
                 "oracle: FAIL — 17 record(s), 7 occurrence(s), 1 violation(s)"),
                ("the same artifact against a plan",
                 ["--validate", str(surrogate), "--receipt", str(plan)],
                 "oracle: FAIL — 17 record(s), 7 occurrence(s), 3 violation(s)"),
            ):
                with self.subTest(arm=label):
                    output = io.StringIO()
                    with contextlib.redirect_stdout(output):
                        code = NATIVE_CHECK.main(argv)
                    printed = output.getvalue()
                    self.assertEqual(code, 1, f"exit {code} for {label}: {printed}")
                    self.assertNotIn("Traceback", printed, f"{label} ended in a traceback: {printed}")
                    self.assertEqual(printed.strip().splitlines()[-1], verdict,
                                     f"{label}: the verdict is not the run's last line: {printed}")
                    self.assertIn("sha256:91a681b99855", printed, f"{label}: {printed}")
            nested = Path(scratch) / "nested-input.jsonl"
            nested.write_text(
                oracle_edit(session, oracle_item_is("function_call", "call_oracle_0001"),
                            change=lambda record: record["payload"].__setitem__(
                                "arguments", "[" * 200000 + "]" * 200000)),
                encoding="utf-8")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(nested)])
            printed = output.getvalue()
            self.assertEqual(code, 0, f"exit {code} for a deeply nested input: {printed}")
            self.assertNotIn("Traceback", printed, f"a nested input ended in a traceback: {printed}")
            self.assertEqual(printed.strip().splitlines()[-1],
                             "oracle: PASS — 17 record(s), 7 occurrence(s)",
                             f"the verdict is not the run's last line: {printed}")

    def test_the_validate_mode_reports_a_verdict(self) -> None:
        """`--validate` answers for one artifact (and its plan): PASS, FAIL and INCOMPLETE, with the
        numbers it read and the verdict line last.

        The same three answers the rest of this repository's checks give, on the mode goal 0009's later
        increments (and a person) read an installed rollout with: a green fixture and its plan pass, the
        unreduced artifact against the reduced plan fails naming the rule and the record — in both of
        the artifact's dialogue channels — `--json` carries the same reading as a document in both
        verdicts, an artifact or a receipt that cannot be read is INCOMPLETE with exit 2 rather than a
        pass, a receipt document that is not a plan (a file holding `null`) is a supplied value that
        `oracle.receipt` refuses rather than an absent receipt, and `--receipt` without `--validate` is
        refused before any file is read.
        """
        with tempfile.TemporaryDirectory() as scratch:
            artifact = Path(scratch) / "session.jsonl"
            plan = Path(scratch) / "plan.json"
            artifact.write_text(oracle_fixture("session.jsonl"), encoding="utf-8")
            plan.write_text(oracle_fixture("session-receipt.json"), encoding="utf-8")

            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 0, f"exit {code} for a green artifact: {output.getvalue()}")
            self.assertEqual(output.getvalue().strip().splitlines()[-1],
                             "oracle: PASS — 17 record(s), 7 occurrence(s)",
                             f"the verdict line is not the run's last line: {output.getvalue()}")

            plan.write_text(oracle_fixture("reduced-receipt.json"), encoding="utf-8")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 1, f"exit {code} for a red artifact: {output.getvalue()}")
            self.assertIn("oracle: oracle.plan-accounted line 14 —", output.getvalue(),
                          f"the failing record is not named: {output.getvalue()}")
            self.assertIn("the artifact's turn view holds 1", output.getvalue(),
                          f"the turn-view half of the comparison is not reported: {output.getvalue()}")
            self.assertEqual(output.getvalue().strip().splitlines()[-1],
                             "oracle: FAIL — 17 record(s), 7 occurrence(s), 4 violation(s)",
                             f"the verdict line is not the run's last line: {output.getvalue()}")

            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan),
                                          "--json"])
            self.assertEqual(code, 1, f"exit {code} for a red artifact as JSON: {output.getvalue()}")
            document = json.loads(output.getvalue()[:output.getvalue().index("\noracle: ")])
            self.assertEqual((document["verdict"], len(document["violations"])), ("FAIL", 4),
                             f"the JSON document does not carry the verdict: {document}")
            self.assertEqual(output.getvalue().strip().splitlines()[-1],
                             "oracle: FAIL — 17 record(s), 7 occurrence(s), 4 violation(s)",
                             f"the verdict line is not the run's last line: {output.getvalue()}")

            plan.write_text("null", encoding="utf-8")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 1, f"exit {code} for a receipt holding null: {output.getvalue()}")
            self.assertIn("oracle: oracle.receipt — the receipt is not an object carrying an "
                          "`occurrences` list", output.getvalue(),
                          f"a null receipt document was not refused as a plan: {output.getvalue()}")
            self.assertEqual(output.getvalue().strip().splitlines()[-1],
                             "oracle: FAIL — 17 record(s), 7 occurrence(s), 1 violation(s)",
                             f"the supplied null receipt did not fail the artifact: {output.getvalue()}")

            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--json"])
            self.assertEqual(code, 0, f"exit {code} for a green artifact: {output.getvalue()}")
            document = json.loads(output.getvalue()[:output.getvalue().index("\noracle: ")])
            self.assertEqual((document["verdict"], document["records"], document["occurrences"],
                              document["violations"], document["receipt"]),
                             ("PASS", 17, 7, [], None),
                             f"the JSON document is not the run's own reading: {document}")

            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(Path(scratch) / "absent.jsonl")])
            self.assertEqual(code, 2, f"exit {code} for an artifact that is not there")
            self.assertIn("oracle: INCOMPLETE", errors.getvalue(),
                          f"an unreadable artifact is not INCOMPLETE: {errors.getvalue()}")

            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact),
                                          "--receipt", str(Path(scratch) / "absent-plan.json")])
            self.assertEqual(code, 2, f"exit {code} for a receipt that is not there")
            self.assertIn("oracle: INCOMPLETE", errors.getvalue(),
                          f"an unreadable receipt is not INCOMPLETE: {errors.getvalue()}")

            plan.write_text("{not JSON", encoding="utf-8")
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 2, f"exit {code} for a receipt that is not JSON")
            self.assertIn("oracle: INCOMPLETE", errors.getvalue(),
                          f"a receipt that is not JSON is not INCOMPLETE: {errors.getvalue()}")

            plan.write_text("[" * 200000 + "]" * 200000, encoding="utf-8")
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(plan)])
            self.assertEqual(code, 2,
                             f"exit {code} for a receipt nested past reading: {errors.getvalue()}")
            self.assertIn("oracle: INCOMPLETE", errors.getvalue(),
                          f"a receipt nested past reading is not INCOMPLETE: {errors.getvalue()}")
            self.assertIn("nested too deeply to read", errors.getvalue(), errors.getvalue())

            errors = io.StringIO()
            with contextlib.redirect_stderr(errors), self.assertRaises(SystemExit) as refusal:
                NATIVE_CHECK.main(["--receipt", str(plan)])
            self.assertEqual(refusal.exception.code, 2,
                             f"`--receipt` without `--validate` was not refused: {errors.getvalue()}")
            self.assertIn("--receipt", errors.getvalue(),
                          f"the refusal does not name the flag: {errors.getvalue()}")


if __name__ == "__main__":
    unittest.main()
