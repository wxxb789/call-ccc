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
import hashlib
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
              repeat_turn: bool = False,
              seed_probe: bool = False,
              handshake_fails: bool = False,
              version_fails: bool = False
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

        ``seed_probe`` writes a caller's own file under the disclosure arm's probe name before ``main``
        runs — the collision its refusal path is about; ``handshake_fails`` makes the stub app server
        never complete the handshake, so the arm's subject becomes an answer that was never obtained;
        and ``version_fails`` makes the stub's ``codex --version`` exit non-zero. The run's own stdout
        and stderr are kept as ``self.last_stdout``/``self.last_stderr``, and the home it drove as
        ``self.last_home``, so a case can read what the run reported and what it left on disk after
        ``main`` returned.
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
        # The caller's own file under the probe's name: the run must refuse to overwrite it, plant
        # nothing of its own, and leave this file exactly as it found it.
        probe = home / "sessions" / "2026" / "01" / "01" / NATIVE_CHECK.ORACLE_PROBE_NAME
        if seed_probe:
            probe.parent.mkdir(parents=True, exist_ok=True)
            probe.write_text("the caller's own file under the probe's name\n")
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
            if command[0] == codex and command[1:] == ["--version"]:
                if version_fails:
                    return subprocess.CompletedProcess(cmd, 1, "", "codex-cli 0.test is broken\n")
                return subprocess.CompletedProcess(cmd, 0, "codex-cli 0.test\n", "")
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
                if handshake_fails:
                    raise RuntimeError("the case's stub never answered initialize")

            def call(self, method: str, params: dict) -> dict:
                if method != "thread/resume":
                    raise AssertionError(f"the case's stub was asked for {method}")
                session = str(params.get("threadId"))
                if session not in (thread_id, second_thread_id):
                    # The disclosure arm's own subject: the loader holds no session for the artifact the
                    # offline rules accept, and the harness's answer for it is its refusal -- an error
                    # response to thread/resume, the way the real AppServer reports one.
                    raise NATIVE_CHECK.AppServerErrorResponse(
                        f"thread/resume failed: the case's stub holds no session {session}")
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
        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                mock.patch.object(NATIVE_CHECK, "AppServer", FakeAppServer), \
                mock.patch.object(NATIVE_CHECK, "thread_view", fake_thread_view), \
                mock.patch("shutil.which", fake_which), \
                contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            code = NATIVE_CHECK.main(["--repo", str(repo), "--codex", "codex",
                                      "--codex-home", str(home)] + prompts)
        results = {name: (state, detail) for name, state, detail in NATIVE_CHECK.RESULTS[before:]}
        del NATIVE_CHECK.RESULTS[before:]
        self.last_stdout, self.last_stderr, self.last_home = stdout.getvalue(), stderr.getvalue(), home
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

    def test_the_disclosure_arm_is_a_recorded_pass_naming_the_harnesss_refusal(self) -> None:
        """The D3 side-by-side is a check of its own, and this pins its record: the arm runs, its PASS
        rests on the harness's own refusal, and its detail quotes that refusal — a run that stopped
        recording the arm, or one that recorded it unconditionally, fails here."""
        _, results = self.drive(mutate_during_resume=False)
        state, detail = results.get("the harness refuses an artifact the offline rules accept",
                                    ("missing", ""))
        self.assertEqual(state, "PASS",
                         f"the disclosure arm is not a recorded PASS: {state} — {detail}")
        self.assertIn("the offline rules: green", detail, detail)
        self.assertIn("the case's stub holds no session", detail,
                      f"the arm's detail does not name the harness's own refusal: {detail}")

    def test_a_handshake_that_never_answered_is_not_read_as_a_refusal(self) -> None:
        """No answer is not a refusal: with the handshake never completing, the disclosure arm records
        its FAIL in the failure's own words — a run that read the missing answer as the refusal its
        name claims fails here."""
        _, results = self.drive(mutate_during_resume=False, handshake_fails=True)
        state, detail = results.get("the harness refuses an artifact the offline rules accept",
                                    ("missing", ""))
        self.assertEqual(state, "FAIL",
                         f"a handshake that never answered was recorded as {state}: {detail}")
        self.assertIn("the case's stub never answered initialize", detail, detail)

    def test_a_run_records_the_resolved_binary_and_its_digest(self) -> None:
        """D3's own line is a record: every run names the binary it resolved — the `--version` answer
        and the binary's own digest — and answers "unreadable" where either cannot be measured."""
        self.drive(mutate_during_resume=False)
        self.assertIn("codex-cli 0.test, sha256 unreadable", self.last_stdout,
                      "the run did not record the resolved binary's version and its own digest")
        self.drive(mutate_during_resume=False, version_fails=True)
        self.assertIn("(unreadable, sha256 unreadable)", self.last_stdout,
                      "a run whose `codex --version` failed did not name the version unreadable")

    def test_a_caller_file_under_the_probes_name_is_refused_without_leaving_a_fabrication(self) -> None:
        """A probe collision plants nothing: the run refuses (exit 2), the caller's own file is left
        exactly as found, and no fabricated session is written into the home — the leak this pins was
        measured when the check ran after the unrelated fabric had already been planted."""
        code, _ = self.drive(mutate_during_resume=False, seed_probe=True)
        self.assertEqual(code, 2, f"exit {code} for a probe collision: {self.last_stderr}")
        sessions = self.last_home / "sessions" / "2026" / "01" / "01"
        self.assertEqual((sessions / NATIVE_CHECK.ORACLE_PROBE_NAME).read_text(),
                         "the caller's own file under the probe's name\n",
                         "the run did not leave the caller's probe file as it found it")
        self.assertFalse((sessions / NATIVE_CHECK.UNRELATED_NAME).exists(),
                         "the refused run left the fabricated unrelated session in the home")

    def test_the_planted_probe_is_removed_when_the_run_ends(self) -> None:
        """The disclosure arm's planted artifact belongs to the run: a normal run removes it from the
        home, so neither a caller's home nor this case's own keeps a fabricated file."""
        code, _ = self.drive(mutate_during_resume=False)
        self.assertEqual(code, 0, f"exit {code} for an intact run: {self.last_stderr}")
        probe = (self.last_home / "sessions" / "2026" / "01" / "01"
                 / NATIVE_CHECK.ORACLE_PROBE_NAME)
        self.assertFalse(probe.exists(),
                         "the run ended with the disclosure arm's planted artifact still in the home")


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


class CodexArtifactOracle(unittest.TestCase):
    """The offline oracle (goal 0009a, re-scoped): the container and receipt rules, their corpus, and
    the `--validate` mode.

    Every case drives the shipped module in-process, imported by path, with no harness and no source
    session: the oracle's subject is an artifact's own bytes. The corpus lives in the shipped script
    beside the registry and this class drives the same objects — each rule has at least one mutation
    that removes it, each mutation turns its own rule red and no other, the two tables are in bijection,
    and every arm is read through the shipped command's own
    `--validate`/`--receipt`/`--disable-rule` path, so `--disable-rule` is exercised for every declared
    rule. What the rules can no longer say is part of the subject: the registry names no foreign
    format's vocabulary, and an artifact that is a container and no agent's session passes.
    """

    #: Spellings that belong to a foreign session format. A rule that names one is a rule about that
    #: format — the judgment this oracle deleted — so none may appear anywhere on its frozen surface
    #: (the ids, the sentences, the corpus's labels, the fixtures' names).
    FOREIGN_VOCABULARY = ("codex", "rollout", "session_meta", "response_item", "event_msg",
                          "turn_context", "compacted", "token_usage", "world_state", "call_id",
                          "tool_call", "tool_result", "function_call", "input_text", "output_text",
                          "input_image", "input_audio", "occurrence", "disposition", "manifest",
                          "emitted", "omitted", "payload", "turn", "replay")

    def reading(self, data: bytes, receipt: dict | None = None,
                disabled: tuple[str, ...] = ()) -> list:
        """One reading through the shipped validator, with or without a receipt."""
        if receipt is None:
            return NATIVE_CHECK.validate_artifact(data, disabled=disabled)
        return NATIVE_CHECK.validate_artifact(data, receipt=receipt, disabled=disabled)

    def test_the_frozen_artifact_validates(self) -> None:
        """The committed artifact is green alone and with its receipt (D4's verify case), and the
        receipt's own statements are the artifact's bytes: its size, its whole-file digest, and one
        anchor per line over that line's own bytes with its line feed included."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)
        receipt = NATIVE_CHECK.oracle_receipt(NATIVE_CHECK.ORACLE_RECEIPT)
        self.assertEqual(self.reading(data), [])
        self.assertEqual(self.reading(data, receipt), [])
        self.assertEqual(receipt["bytes"], len(data))
        self.assertEqual(receipt["sha256"], hashlib.sha256(data).hexdigest())
        spans = NATIVE_CHECK.oracle_line_bytes(data)
        self.assertEqual([entry["sha256"] for entry in receipt["lines"]],
                         [hashlib.sha256(span).hexdigest() for span in spans],
                         "the anchors are not the lines' own bytes, line feed included")

    def test_a_container_of_json_objects_that_is_no_agents_session_passes(self) -> None:
        """D2, behaviourally: an artifact no agent's reader would read — JSON objects carrying members
        no harness defines, no record vocabulary at all — passes the offline rules, so a schema rule
        that crept back fails here."""
        data = b"".join(json.dumps({"anything": f"goes {n}"}, separators=(",", ":")).encode() + b"\n"
                        for n in range(3))
        self.assertEqual(self.reading(data), [])
        with tempfile.TemporaryDirectory() as scratch:
            artifact = Path(scratch) / "artifact.jsonl"
            artifact.write_bytes(data)
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(artifact)])
            self.assertEqual(code, 0, output.getvalue())
            self.assertEqual(output.getvalue().strip().splitlines()[-1], "oracle: PASS — 3 record(s)")

    def test_the_registry_holds_no_foreign_format_vocabulary(self) -> None:
        """D2, over the frozen surface: the id set is the four container/receipt rules, and nothing the
        registry, its sentences, the corpus's labels or the fixtures' names spell is a foreign
        format's vocabulary."""
        self.assertEqual(sorted(NATIVE_CHECK.ORACLE_RULES),
                         ["oracle.jsonl-objects", "oracle.receipt-integrity", "oracle.utf8",
                          "oracle.whole-lines"])
        surface = " ".join(list(NATIVE_CHECK.ORACLE_RULES)
                           + list(NATIVE_CHECK.ORACLE_RULES.values())
                           + [f"{mutation.label} {mutation.artifact} {mutation.receipt or ''}"
                              for mutations in NATIVE_CHECK.ORACLE_MUTATIONS.values()
                              for mutation in mutations]
                           + [NATIVE_CHECK.ORACLE_ARTIFACT, NATIVE_CHECK.ORACLE_RECEIPT]).lower()
        found = sorted(token for token in self.FOREIGN_VOCABULARY if token in surface)
        self.assertEqual(found, [],
                         f"the oracle's own surface names a foreign format's vocabulary: {found}")

    def test_the_registry_and_its_corpus_are_one_closed_set(self) -> None:
        """D1's closure, read from both ends: every declared rule has at least one mutation and every
        mutation names a declared rule, each rule states something, and every fixture the corpus names
        is committed (a pair naming an absent fixture would fail the command, not this reading)."""
        self.assertEqual(set(NATIVE_CHECK.ORACLE_MUTATIONS), set(NATIVE_CHECK.ORACLE_RULES),
                         "a rule has no mutation, or a mutation has no rule")
        for rule, sentence in NATIVE_CHECK.ORACLE_RULES.items():
            self.assertTrue(sentence.strip(), f"the rule {rule} states nothing")
        for rule, mutations in sorted(NATIVE_CHECK.ORACLE_MUTATIONS.items()):
            self.assertTrue(mutations, f"{rule} has no mutation")
            for mutation in mutations:
                fixture = NATIVE_CHECK.ORACLE_FIXTURE_ROOT / mutation.artifact
                self.assertTrue(fixture.is_file(), f"{rule} names an absent fixture {fixture}")
                if mutation.receipt is not None:
                    receipt = NATIVE_CHECK.ORACLE_FIXTURE_ROOT / mutation.receipt
                    self.assertTrue(receipt.is_file(), f"{rule} names an absent receipt {receipt}")

    def test_every_rule_has_a_mutation_that_turns_its_fixture_red(self) -> None:
        """Every mutation changes its fixture and turns exactly its own rule red, with a detail that
        says what was found — no other rule, so a report is a reading of one defect rather than a net
        that catches everything at once — and with that rule disabled the same bytes are green again."""
        for rule, mutations in sorted(NATIVE_CHECK.ORACLE_MUTATIONS.items()):
            for mutation in mutations:
                with self.subTest(rule=rule, mutation=mutation.label):
                    data = NATIVE_CHECK.oracle_fixture(mutation.artifact)
                    receipt = (NATIVE_CHECK.oracle_receipt(mutation.receipt)
                               if mutation.receipt is not None else None)
                    self.assertEqual(self.reading(data, receipt), [],
                                     f"{mutation.artifact} is not green before {mutation.label!r}")
                    before = (data, json.dumps(receipt, sort_keys=True))
                    mutated, mutated_receipt = mutation.mutate(
                        data, NATIVE_CHECK.oracle_receipt(mutation.receipt)
                        if mutation.receipt is not None else None)
                    self.assertNotEqual((mutated, json.dumps(mutated_receipt, sort_keys=True)), before,
                                        f"the mutation {mutation.label!r} changed nothing")
                    violations = self.reading(mutated, mutated_receipt)
                    self.assertEqual({violation.rule for violation in violations}, {rule},
                                     f"{mutation.label!r} on {mutation.artifact} turned "
                                     f"{sorted({violation.rule for violation in violations})} red "
                                     f"instead of only {rule}: {violations}")
                    for violation in violations:
                        self.assertTrue(violation.detail.strip(),
                                        f"the {rule} violation says nothing: {violation}")
                    self.assertEqual(self.reading(mutated, mutated_receipt, (rule,)), [],
                                     f"the mutation {mutation.label!r} is still not green with "
                                     f"`{rule}` disabled")

    def test_the_corpus_arms_run_the_shipped_command_not_the_python_validator(self) -> None:
        """The corpus's arms are the command's own: the Python validator's entry point refuses for the
        duration, every arm judges a file on disk written under a scratch directory, and every declared
        rule appears as the `--disable-rule` argument of at least one invocation — a corpus that
        reached for the validator, or skipped a rule's flip, fails here rather than closing."""
        real_main = NATIVE_CHECK.main
        seen: list[list[str]] = []

        def spy(argv):
            seen.append(list(argv))
            judged = Path(argv[argv.index("--validate") + 1])
            self.assertTrue(judged.is_file(), f"an arm judged a path that is not a file on disk: {argv}")
            return real_main(argv)

        def refuse(*args, **kwargs):
            raise AssertionError("the corpus read through the Python validator instead of the command")

        output = io.StringIO()
        with mock.patch.object(NATIVE_CHECK, "main", spy), \
                mock.patch.object(NATIVE_CHECK, "validate_artifact", refuse), \
                contextlib.redirect_stdout(output):
            code = NATIVE_CHECK.oracle_mutation_selftest()
        self.assertEqual(code, 0, f"the corpus does not close through the command: {output.getvalue()}")
        disabled = [argv[argv.index("--disable-rule") + 1] for argv in seen
                    if "--disable-rule" in argv]
        self.assertEqual(sorted(set(disabled)), sorted(NATIVE_CHECK.ORACLE_RULES),
                         f"the corpus did not exercise --disable-rule for every rule: {sorted(disabled)}")

    def test_a_mutated_byte_breaks_the_receipt(self) -> None:
        """D4's second case: one line's own byte changed under the unchanged receipt fails, naming the
        line — and the mutated artifact is not otherwise defective, so the failure is the receipt's
        statement and not the container's; a receipt recomputed for those bytes passes, which is the
        verify arm's control."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)
        receipt = NATIVE_CHECK.oracle_receipt(NATIVE_CHECK.ORACLE_RECEIPT)
        mutated, _ = NATIVE_CHECK._mutation_artifact_byte_changed(data, None)
        self.assertNotEqual(mutated, data)
        self.assertEqual(self.reading(mutated), [],
                         "the mutated artifact is the container's own defect too, so this case could "
                         "not attribute the failure to the receipt")
        violations = self.reading(mutated, receipt)
        self.assertEqual({violation.rule for violation in violations}, {"oracle.receipt-integrity"},
                         f"the receipt did not answer the changed byte: {violations}")
        self.assertIn(3, {violation.line for violation in violations},
                      f"no violation names the line the byte moved on: {violations}")
        recomputed = {"bytes": len(mutated), "sha256": hashlib.sha256(mutated).hexdigest(),
                      "lines": [{"sha256": hashlib.sha256(span).hexdigest()}
                                for span in NATIVE_CHECK.oracle_line_bytes(mutated)]}
        self.assertEqual(self.reading(mutated, recomputed), [],
                         "a receipt recomputed for the mutated bytes is not green")

    def test_the_receipt_compares_the_lines_own_bytes(self) -> None:
        """The anchors are bytes, not the JSON value a reader makes of them: a carriage return the
        container rules read as whitespace is a byte the receipt sees, and the artifact is otherwise
        clean, so the comparison is what refuses it."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT).replace(b"\n", b"\r\n")
        receipt = NATIVE_CHECK.oracle_receipt(NATIVE_CHECK.ORACLE_RECEIPT)
        self.assertEqual(self.reading(data), [], "the CRLF artifact is the container's own defect")
        violations = self.reading(data, receipt)
        self.assertEqual({violation.rule for violation in violations}, {"oracle.receipt-integrity"},
                         f"the receipt did not answer the changed bytes: {violations}")

    def test_the_byte_layer_ends_the_reading(self) -> None:
        """A violation at the byte layer stops the reading — the BOM and a byte that is not UTF-8 are
        `oracle.utf8` alone, never also a line's JSON defect — and dropping that rule hands back a
        reading with nothing else in it, which is the closure's third arm."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)
        broken_byte, _ = NATIVE_CHECK._mutation_byte_not_utf8(data, None)
        for label, broken in (("a byte-order mark", b"\xef\xbb\xbf" + data),
                              ("a byte that is not UTF-8", broken_byte)):
            self.assertEqual([violation.rule for violation in self.reading(broken)], ["oracle.utf8"],
                             f"{label} is not the byte layer's own defect alone")
            self.assertEqual(self.reading(broken, disabled=("oracle.utf8",)), [],
                             f"{label} is not green with the rule disabled")

    def test_the_line_rule_is_the_whole_lines_own(self) -> None:
        """An artifact whose last record has no line feed is `oracle.whole-lines` alone — its last line
        is still one JSON object — the empty artifact holds no unterminated record, and a line that is
        no JSON document (a blank one included) is `oracle.jsonl-objects` alone."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)
        self.assertEqual([violation.rule for violation in self.reading(data[:-1])],
                         ["oracle.whole-lines"])
        self.assertEqual(self.reading(b""), [], "the empty artifact is not an unterminated record")
        self.assertEqual([violation.rule for violation in self.reading(data + b"\n")],
                         ["oracle.jsonl-objects"],
                         "a blank line was skipped rather than refused as no JSON object")

    def test_an_unreadable_input_is_incomplete_never_a_pass(self) -> None:
        """The mode's third outcome: an artifact that cannot be opened, a receipt that is not JSON, and
        a receipt document whose object writes a member name twice each exit 2 with the reason on
        stderr — nothing was validated, so nothing may read as a pass."""
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            artifact = root / "artifact.jsonl"
            artifact.write_bytes(NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT))
            not_json = root / "not-json.receipt.json"
            not_json.write_text("this is not JSON", encoding="utf-8")
            repeated = root / "repeated.receipt.json"
            repeated.write_text('{"bytes": 1, "bytes": 2}', encoding="utf-8")
            for label, argv in (
                ("an artifact that cannot be opened", ["--validate", str(root / "absent.jsonl")]),
                ("a receipt that is not JSON",
                 ["--validate", str(artifact), "--receipt", str(not_json)]),
                ("a receipt writing a member name twice",
                 ["--validate", str(artifact), "--receipt", str(repeated)]),
            ):
                with self.subTest(input=label):
                    errors = io.StringIO()
                    with contextlib.redirect_stderr(errors):
                        code = NATIVE_CHECK.main(argv)
                    self.assertEqual(code, 2, f"{label}: exit {code}, {errors.getvalue()}")
                    self.assertIn("oracle: INCOMPLETE", errors.getvalue(), label)

    def test_the_validate_mode_reports_a_verdict(self) -> None:
        """The mode's own three outcomes and its JSON document: PASS (exit 0), FAIL naming the
        violations (exit 1), and the receipt/flag refusals, with the verdict line last."""
        fixture = NATIVE_CHECK.ORACLE_FIXTURE_ROOT / NATIVE_CHECK.ORACLE_ARTIFACT
        receipt = NATIVE_CHECK.ORACLE_FIXTURE_ROOT / NATIVE_CHECK.ORACLE_RECEIPT
        with tempfile.TemporaryDirectory() as scratch:
            mutated = Path(scratch) / "mutated.jsonl"
            mutated.write_bytes(NATIVE_CHECK._mutation_artifact_byte_changed(
                NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT), None)[0])
            for label, argv, want_code, want_line in (
                ("the artifact alone", ["--validate", str(fixture)], 0, "oracle: PASS — 3 record(s)"),
                ("the artifact with its receipt", ["--validate", str(fixture), "--receipt", str(receipt)],
                 0, "oracle: PASS — 3 record(s)"),
                ("a changed byte under the receipt",
                 ["--validate", str(mutated), "--receipt", str(receipt)], 1,
                 "oracle: FAIL — 3 record(s), 2 violation(s)"),
            ):
                with self.subTest(invocation=label):
                    output = io.StringIO()
                    with contextlib.redirect_stdout(output):
                        code = NATIVE_CHECK.main(argv)
                    self.assertEqual(code, want_code, f"{label}: {output.getvalue()}")
                    self.assertEqual(output.getvalue().strip().splitlines()[-1], want_line,
                                     f"{label}: {output.getvalue()}")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(fixture), "--receipt", str(receipt), "--json"])
            self.assertEqual(code, 0, output.getvalue())
            document = json.loads(output.getvalue()[:output.getvalue().index("\noracle: ")])
            self.assertEqual((document["verdict"], document["records"], document["violations"],
                              document["disabled_rules"]), ("PASS", 3, [], []), document)
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors), self.assertRaises(SystemExit) as refusal:
                NATIVE_CHECK.main(["--receipt", str(receipt)])
            self.assertEqual(refusal.exception.code, 2,
                             f"--receipt without --validate was accepted: {errors.getvalue()}")

    def test_selftest_mutations_refuses_corpus_flags(self) -> None:
        """The corpus mode takes none of the single-artifact flags: an invocation that also names
        ``--validate``, ``--receipt`` or ``--disable-rule`` is refused (exit 2) rather than running the
        corpus under a flag it would ignore."""
        for flags in (["--validate", "any-artifact.jsonl"], ["--receipt", "any-receipt.json"],
                      ["--disable-rule", "oracle.utf8"]):
            with self.subTest(flags=flags):
                errors = io.StringIO()
                with contextlib.redirect_stderr(errors), self.assertRaises(SystemExit) as refusal:
                    NATIVE_CHECK.main(["--selftest-mutations", *flags])
                self.assertEqual(refusal.exception.code, 2, errors.getvalue())
                self.assertIn("--selftest-mutations runs the whole corpus", errors.getvalue())

    def test_disable_rule_without_validate_is_refused(self) -> None:
        """``--disable-rule`` turns a rule off for ``--validate``, which names the artifact the reading
        is about: without one there is no reading to modify, and the invocation is refused (exit 2)."""
        errors = io.StringIO()
        with contextlib.redirect_stderr(errors), self.assertRaises(SystemExit) as refusal:
            NATIVE_CHECK.main(["--disable-rule", "oracle.utf8"])
        self.assertEqual(refusal.exception.code, 2, errors.getvalue())
        self.assertIn("--disable-rule turns a rule off for --validate", errors.getvalue())

    def test_a_receipt_that_is_not_utf8_text_is_incomplete(self) -> None:
        """A receipt whose bytes are not UTF-8 text cannot be read as one: exit 2 with the reason,
        never a pass — the arm a receipt with a stray byte lands on."""
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            artifact = root / "artifact.jsonl"
            artifact.write_bytes(NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT))
            not_text = root / "not-text.receipt.json"
            not_text.write_bytes(b"\xff\xfe{}")
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact), "--receipt", str(not_text)])
            self.assertEqual(code, 2, errors.getvalue())
            self.assertIn("is not UTF-8 text", errors.getvalue())

    def test_a_receipt_that_cannot_be_read_is_incomplete(self) -> None:
        """A receipt path that cannot be opened is an input that cannot be read, the same way: exit 2
        with the reason on stderr, never a pass."""
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            artifact = root / "artifact.jsonl"
            artifact.write_bytes(NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT))
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(artifact),
                                          "--receipt", str(root / "absent-receipt.json")])
            self.assertEqual(code, 2, errors.getvalue())
            self.assertIn("cannot be read", errors.getvalue())

    def test_disable_rule_turns_exactly_one_rule_off(self) -> None:
        """`--disable-rule` drops exactly the named rule's violations, through the shipped command: the
        same bytes pass again without a file being edited, an id the registry does not declare is
        refused before any artifact is read, and the JSON document names the reading it was taken
        under."""
        fixture = NATIVE_CHECK.ORACLE_FIXTURE_ROOT / NATIVE_CHECK.ORACLE_ARTIFACT
        with tempfile.TemporaryDirectory() as scratch:
            unterminated = Path(scratch) / "unterminated.jsonl"
            unterminated.write_bytes(NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)[:-1])
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(unterminated)])
            self.assertEqual(code, 1, output.getvalue())
            self.assertIn("oracle: oracle.whole-lines line 3 —", output.getvalue())
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(unterminated),
                                          "--disable-rule", "oracle.whole-lines"])
            self.assertEqual(code, 0, output.getvalue())
            self.assertNotIn("oracle.whole-lines", output.getvalue())
            self.assertEqual(output.getvalue().strip().splitlines()[-1], "oracle: PASS — 3 record(s)")
            errors = io.StringIO()
            with contextlib.redirect_stderr(errors):
                code = NATIVE_CHECK.main(["--validate", str(fixture),
                                          "--disable-rule", "oracle.nope"])
            self.assertEqual(code, 2, f"exit {code} for an undeclared id: {errors.getvalue()}")
            self.assertIn("names no rule this oracle declares: oracle.nope", errors.getvalue())
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--validate", str(unterminated), "--json",
                                          "--disable-rule", "oracle.whole-lines"])
            document = json.loads(output.getvalue()[:output.getvalue().index("\noracle: ")])
            self.assertEqual((code, document["disabled_rules"], document["violations"]),
                             (0, ["oracle.whole-lines"], []), document)

    def test_the_validator_reads_no_file(self) -> None:
        """`validate_artifact`'s input is the artifact's bytes and nothing else: with every
        file-opening name refused it still answers, which is the property that lets a reduced artifact
        be validated with the source absent from the process (D2)."""
        data = NATIVE_CHECK.oracle_fixture(NATIVE_CHECK.ORACLE_ARTIFACT)
        receipt = NATIVE_CHECK.oracle_receipt(NATIVE_CHECK.ORACLE_RECEIPT)

        def refuse(*args, **kwargs):
            raise AssertionError("the validator opened a file")

        with mock.patch("builtins.open", refuse), mock.patch("io.open", refuse), \
                mock.patch("os.open", refuse):
            self.assertEqual(NATIVE_CHECK.validate_artifact(data, receipt=receipt), [])


if __name__ == "__main__":
    unittest.main()
