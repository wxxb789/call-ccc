#!/usr/bin/env python3
"""Regression tests for the checks in scripts/claude-native-check.py that need no Claude Code binary.

Two subjects, both driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the check that reads the reported resume invocation back the way a POSIX shell reads it
(``shell_words`` -- what its words are -- with ``unquoted_text``/``spelled_as_invocation`` for the
spelling that decides whether a shell reads the line as that `cd` and that assignment at all), and the
ordering rule that the source fixture copy and both planted sessions are compared **after** the
harness's own native stages (``main`` driven with its subprocess and shell seams replaced by stubs). A
third subject is driven the same way: the discovery rule the run header and the installation check depend
on (``claude_project_dir``, the target's own spelling of a working directory into a project directory).

The resume-invocation check is the harness's own rule about the line a caller pastes or an agent runs: it
must name the working directory the target's loader is started from and the data directory the session
was installed into, because the loader finds a session through the directory it is run from and takes its
data directory from CLAUDE_CONFIG_DIR alone -- it has no flag for either. The line is *rendered* for a
POSIX shell, so a check written against a literal substring answers the opposite of the rule in both
directions: it accepts an unquoted working directory or home whose space a shell reads as two words --
the defect the quoting exists to prevent -- and it rejects every value the renderer correctly quotes,
which is every path with a space and every Windows path with a backslash.

The cases drive the shipped module in-process, imported by path, with no subprocess and no tool on
``PATH`` -- the way ``scripts/tests/test_upstream_parity.py`` drives its scripts: the reading of a rendered
line is pure Python, and a case that needed the Claude Code binary would not be a gate step. The
renderer's own half of the contract is pinned by the Go tests that assert the quoted spelling
(``cmd/ccc/main_test.go``, ``internal/ccc/continue_test.go``); these cases pin the half that reads that
line back.

Every case drives **both** ``pathlib`` flavours a host can hand the shipped check: this host's ``Path``
and ``PureWindowsPath`` -- what ``Path`` *is* on Windows. That, and nothing about the host running this
suite, is what keeps the fixtures honest: a home spelled ``/tmp/…`` with literals to match is green here
and fails in the ``windows-latest`` job alone, because ``Path('/tmp/codex home')`` reads back as
``\\tmp\\codex home`` there and the literal rendering and the comparison therefore disagree on that host
alone. A path in ``RENDERINGS`` is spelled so both flavours give it back unchanged (a Windows-shaped
path, or a single-segment name), because the rule under test is about the *characters* of a word -- a
space, a backslash, an embedded quote -- never about the separator this host happens to use.
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
CLAUDE = "claude"

RESUME_ARGUMENTS = [CLAUDE, "--resume", SESSION]
EXEC_RESUME_ARGUMENTS = [CLAUDE, "-p", "--output-format", "json", "--resume", SESSION, "--", "<prompt>"]

#: ``(the working directory, the data directory)``. Each is a single-segment name, a Windows-shaped path
#: or a name holding a quote, so both ``pathlib`` flavours spell it exactly as the case does. The word a
#: shell reads an assignment of is pinned beside the line by the renderer below, so only the *values*
#: are listed here.
RENDERINGS = (
    ("codex-home", "claude-home"),
    ("codex home", "claude home"),
    (r"C:\code\a b", r"C:\Users\a b\.claude"),
    ("it's dir", "it's home"),
)

#: The path flavours a case drives the check with: this host's own, and Windows' answer on any host.
PATH_FLAVOURS = (Path, PureWindowsPath)

#: The marker the resume check prints the target's answer under: part of the reporting contract, because
#: ``scripts/tests/test_evidence_claims.py`` reads a recorded answer from it.
TURN_OUTPUT_MARKER = ("the turn's own output (the target's answer, as the harness printed it):")


def load_native_check():
    """The shipped harness, imported by path (its file name is not an identifier)."""
    spec = importlib.util.spec_from_file_location(
        "shipped_claude_native_check", SCRIPTS / "claude-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


def rendering(cwd, home, arguments: list[str]) -> str:
    """One invocation as the shipped renderer spells it: the `cd`, then the assignment, then the words.

    ``internal/ccc/continue.go``'s ``renderInvocation`` starts with ``cd <shellQuote(workingDir)> &&``
    and then ``renderResumeCommand``'s ``NAME=<shellQuote(value)>`` plus one ``shellQuote`` per argument:
    the assignment's *name* and its `=` stay bare -- quoting them would turn the assignment into a
    program's name to a shell -- while the value and every other word are single-quoted exactly where a
    POSIX shell would act on them, which is what ``shlex.quote`` does. Spelling the fixtures this way --
    rather than joining words bare, or quoting the whole assignment -- is what keeps them honest on a
    host whose paths hold a backslash or a space.
    """
    pieces = [shlex.quote(NATIVE_CHECK.CD_WORD), shlex.quote(str(cwd)), NATIVE_CHECK.OPERATOR_WORD,
              f"{NATIVE_CHECK.CLAUDE_HOME_ENV}={shlex.quote(str(home))}"]
    pieces += [shlex.quote(word) for word in arguments]
    return " ".join(pieces)


class ResumeInvocationCheck(unittest.TestCase):
    def assert_rejected(self, line: str, cwd, home, arguments: list[str], words: list[str]) -> None:
        """``line`` is not the invocation, and the shell's own reading says why.

        The reading is asserted first so a rejection cannot come from a spelling accident: the case names
        the words a shell reads (a working directory split in two, another home, another session, no
        `cd` at all), and ``reported_invocation`` has to answer ``False`` for that reason under both path
        flavours.
        """
        self.assertEqual(NATIVE_CHECK.shell_words(line), words, line)
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, flavour(cwd), flavour(home), arguments), line)

    def test_accepts_the_claude_rendering_the_renderer_emits(self) -> None:
        """Every rendering reads back as the `cd`, the assignment and the argument vector, on both
        flavours."""
        for cwd, home in RENDERINGS:
            for flavour in PATH_FLAVOURS:
                with self.subTest(cwd=cwd, home=home, flavour=flavour.__name__):
                    resume = rendering(flavour(cwd), flavour(home), RESUME_ARGUMENTS)
                    exec_resume = rendering(flavour(cwd), flavour(home), EXEC_RESUME_ARGUMENTS)
                    words = [NATIVE_CHECK.CD_WORD, str(flavour(cwd)), NATIVE_CHECK.OPERATOR_WORD,
                             f"{NATIVE_CHECK.CLAUDE_HOME_ENV}={flavour(home)}"]
                    self.assertEqual(NATIVE_CHECK.shell_words(resume), [*words, *RESUME_ARGUMENTS],
                                     resume)
                    self.assertEqual(NATIVE_CHECK.shell_words(exec_resume),
                                     [*words, *EXEC_RESUME_ARGUMENTS], exec_resume)
                    self.assertTrue(
                        NATIVE_CHECK.reported_invocation(resume, flavour(cwd), flavour(home),
                                                         RESUME_ARGUMENTS),
                        f"a correct rendering was rejected: {resume!r}")
                    self.assertTrue(
                        NATIVE_CHECK.reported_invocation(exec_resume, flavour(cwd), flavour(home),
                                                         EXEC_RESUME_ARGUMENTS),
                        f"a correct rendering was rejected: {exec_resume!r}")

    def test_rejects_a_claude_assignment_a_shell_reads_as_a_command(self) -> None:
        """The words read back right and the line is still not the invocation.

        A shell recognizes an assignment only while nothing in its name or its `=` is quoted, so each
        spelling below is the *program* to run -- one named `CLAUDE_CONFIG_DIR=claude-home`, or one whose
        name a shell would spell that way only after removing the quoting -- and nothing sets the data
        directory, so the harness would open the caller's own directory instead of the installed one.
        """
        assignment = f"{NATIVE_CHECK.CLAUDE_HOME_ENV}=claude-home"
        for line in (
            f"cd codex-home && '{assignment}' {CLAUDE} --resume {SESSION}",
            f'cd codex-home && "{assignment}" {CLAUDE} --resume {SESSION}',
            f"cd codex-home && CLAUDE_CONFIG_'DIR=claude-home' {CLAUDE} --resume {SESSION}",
            f"cd codex-home && {NATIVE_CHECK.CLAUDE_HOME_ENV}\\=claude-home {CLAUDE} --resume {SESSION}",
        ):
            with self.subTest(line=line):
                self.assertEqual(NATIVE_CHECK.shell_words(line),
                                 ["cd", "codex-home", "&&", assignment, CLAUDE, "--resume", SESSION],
                                 line)
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertFalse(
                            NATIVE_CHECK.reported_invocation(line, flavour("codex-home"),
                                                             flavour("claude-home"), RESUME_ARGUMENTS),
                            line)

    def test_rejects_claude_syntax_the_value_leaves_unquoted(self) -> None:
        """A value the renderer quotes, spelled bare: the words read back right and the line is another
        command line -- `;` ends the assignment (`CLAUDE_CONFIG_DIR=a`) and starts a program named `b`,
        a bare `&` does the same (`a&b` sets the variable to `a` and runs `b`), `$HOME` is substituted
        before the variable is set, and a second `&&` ends the invocation and runs whatever follows. The
        rule is about spellings: a character a shell acts on has to be quoted, which `shellQuote` does for
        every one. (Each value is a single segment, so both path flavours spell it as the line does.)
        """
        for line, home in (
            (f"cd codex-home && CLAUDE_CONFIG_DIR=a;b {CLAUDE} --resume {SESSION}", "a;b"),
            (f"cd codex-home && CLAUDE_CONFIG_DIR=a&b {CLAUDE} --resume {SESSION}", "a&b"),
            (f"cd codex-home && CLAUDE_CONFIG_DIR=$HOME {CLAUDE} --resume {SESSION}", "$HOME"),
            (f"cd codex-home && CLAUDE_CONFIG_DIR=`id` {CLAUDE} --resume {SESSION}", "`id`"),
            (f"cd codex-home && CLAUDE_CONFIG_DIR=~ {CLAUDE} --resume {SESSION}", "~"),
        ):
            with self.subTest(line=line):
                self.assertEqual(NATIVE_CHECK.shell_words(line),
                                 ["cd", "codex-home", "&&", f"CLAUDE_CONFIG_DIR={home}",
                                  CLAUDE, "--resume", SESSION], line)
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertFalse(
                            NATIVE_CHECK.reported_invocation(line, flavour("codex-home"),
                                                             flavour(home), RESUME_ARGUMENTS), line)

    def test_rejects_an_operator_glued_to_a_word(self) -> None:
        """`CLAUDE_CONFIG_DIR=a&&b` is one word to the shell's reading and two commands to a shell.

        A `&&` a shell reads as its operator is a word of its own, with whitespace on both sides; glued to
        a value it is part of that word, so the line sets the variable to `a` and then runs `b`. The word
        reading cannot tell the two apart (it answers the same words), which is exactly why the spelling
        is read.
        """
        line = f"cd codex-home && CLAUDE_CONFIG_DIR=a&&b {CLAUDE} --resume {SESSION}"
        self.assertEqual(NATIVE_CHECK.shell_words(line),
                         ["cd", "codex-home", "&&", "CLAUDE_CONFIG_DIR=a&&b", CLAUDE, "--resume",
                          SESSION], line)
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, flavour("codex-home"), flavour("a&&b"),
                                                     RESUME_ARGUMENTS), line)

    def test_rejects_an_operator_a_shell_would_read_as_a_word(self) -> None:
        """A quoted `&&` reads back as the same words and is not the operator.

        A shell reads `'&&'` as a word, so the `cd` is followed by an argument spelled `&&` and nothing
        separates the change from the assignment -- which is not the invocation this run reported.
        """
        line = f"cd codex-home '&&' CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {SESSION}"
        self.assertEqual(NATIVE_CHECK.shell_words(line),
                         ["cd", "codex-home", "&&", "CLAUDE_CONFIG_DIR=claude-home", CLAUDE, "--resume",
                          SESSION], line)
        for flavour in PATH_FLAVOURS:
            with self.subTest(flavour=flavour.__name__):
                self.assertFalse(
                    NATIVE_CHECK.reported_invocation(line, flavour("codex-home"),
                                                     flavour("claude-home"), RESUME_ARGUMENTS), line)

    def test_accepts_a_claude_value_spelled_with_the_other_quotes(self) -> None:
        """The check applies the shell's rule, not the renderer's spelling: an assignment whose value is
        double-quoted and holds nothing a shell acts on is the same invocation, and refusing it would
        refuse a line that does set the data directory. (`shellQuote` spells it with single quotes; the
        shell's own reading does not require that spelling.)
        """
        for cwd, home, line in (
            ("codex-home", "claude-home",
             f'cd codex-home && CLAUDE_CONFIG_DIR="claude-home" {CLAUDE} --resume {SESSION}'),
            ("codex home", "claude home",
             f'cd "codex home" && CLAUDE_CONFIG_DIR="claude home" {CLAUDE} --resume {SESSION}'),
        ):
            with self.subTest(line=line):
                for flavour in PATH_FLAVOURS:
                    with self.subTest(flavour=flavour.__name__):
                        self.assertTrue(
                            NATIVE_CHECK.reported_invocation(line, flavour(cwd), flavour(home),
                                                             RESUME_ARGUMENTS), line)

    def test_rejects_an_unquoted_working_directory_a_shell_would_split(self) -> None:
        # The defect the quoted rendering prevents: a shell reads the space as the end of the directory
        # word, so the loader is started somewhere else and answers "No conversation found" -- or finds
        # another session entirely.
        self.assert_rejected(
            f"cd codex home && CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {SESSION}",
            "codex home", "claude-home", RESUME_ARGUMENTS,
            ["cd", "codex", "home", "&&", "CLAUDE_CONFIG_DIR=claude-home", CLAUDE, "--resume",
             SESSION])

    def test_rejects_a_line_without_the_directory_change(self) -> None:
        self.assert_rejected(
            f"CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {SESSION}",
            "codex-home", "claude-home", RESUME_ARGUMENTS,
            ["CLAUDE_CONFIG_DIR=claude-home", CLAUDE, "--resume", SESSION])

    def test_rejects_a_line_for_another_working_directory(self) -> None:
        self.assert_rejected(
            f"cd codex-elsewhere && CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {SESSION}",
            "codex-home", "claude-home", RESUME_ARGUMENTS,
            ["cd", "codex-elsewhere", "&&", "CLAUDE_CONFIG_DIR=claude-home", CLAUDE, "--resume",
             SESSION])

    def test_rejects_a_line_that_names_another_home(self) -> None:
        self.assert_rejected(
            f"cd codex-home && CLAUDE_CONFIG_DIR=claude-elsewhere {CLAUDE} --resume {SESSION}",
            "codex-home", "claude-home", RESUME_ARGUMENTS,
            ["cd", "codex-home", "&&", "CLAUDE_CONFIG_DIR=claude-elsewhere", CLAUDE, "--resume",
             SESSION])

    def test_rejects_a_line_for_another_session(self) -> None:
        self.assert_rejected(
            f"cd codex-home && CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {OTHER_SESSION}",
            "codex-home", "claude-home", RESUME_ARGUMENTS,
            ["cd", "codex-home", "&&", "CLAUDE_CONFIG_DIR=claude-home", CLAUDE, "--resume",
             OTHER_SESSION])

    def test_rejects_a_claude_line_a_shell_cannot_read(self) -> None:
        # An unbalanced quote is not a command line at all: a shell would keep reading, so the reported
        # invocation could not be the one this check is about. This reading does not meet a home, so
        # there is nothing for a path flavour to change about it.
        line = f"cd 'codex-home && CLAUDE_CONFIG_DIR=claude-home {CLAUDE} --resume {SESSION}"
        self.assertIsNone(NATIVE_CHECK.shell_words(line))
        self.assertFalse(
            NATIVE_CHECK.reported_invocation(line, Path("codex-home"), Path("claude-home"),
                                             RESUME_ARGUMENTS))


class ProjectDirectorySpelling(unittest.TestCase):
    """The target files a session under its working directory's own spelling, and the run's header and
    installation check depend on that rule rather than asking the thing they check.

    Measured on Claude Code 2.1.326: its own session for `/tmp/probe3/WS.Upper Case_1+plus` was filed
    under `-tmp-probe3-WS-Upper-Case-1-plus`, so every character outside `[A-Za-z0-9]` becomes `-` and
    case is preserved. The cases pin the shape the loader has to be started from and the directory the
    transcript is installed under, which is the whole reason this harness rewrites the fixture's recorded
    working directory to a scratch directory it creates.
    """

    def test_spells_the_measured_examples(self) -> None:
        for cwd, expected in (
            ("/tmp/probe3/WS.Upper Case_1+plus", "-tmp-probe3-WS-Upper-Case-1-plus"),
            ("/home/user/code/example-api", "-home-user-code-example-api"),
            (r"C:\Users\a b\code", r"C--Users-a-b-code"),
            ("", ""),
        ):
            with self.subTest(cwd=cwd):
                self.assertEqual(NATIVE_CHECK.claude_project_dir(cwd), expected)

    def test_a_directory_the_loader_can_be_started_from_keeps_case(self) -> None:
        # The literal pins the documented spelling (every character outside [A-Za-z0-9] becomes `-`,
        # case preserved), so this case answers the rule rather than comparing a call with itself.
        self.assertEqual(NATIVE_CHECK.claude_project_dir("/tmp/ExampleApi"), "-tmp-ExampleApi")
        self.assertNotEqual(NATIVE_CHECK.claude_project_dir("/tmp/ExampleApi"),
                            NATIVE_CHECK.claude_project_dir("/tmp/exampleapi"))


#: The checks the ordering rule below is about, named as the harness records them.
PROTECTED_CHECKS = ("the source session is unchanged",
                    "the unrelated session in the target home is unchanged",
                    "no pre-existing session in the target home changed")

#: The check that reads the reported invocation back, named as the harness records it: the case that
#: changes the reported exec form drives this one, and nothing else.
REPORTED_INVOCATION_CHECK = ("the reported resume command names the binary, the session, the working "
                             "directory and the data directory")

#: The session this case's home already holds before the run starts: the third comparison's subject. It
#: is kept as a *name* here, so the assertion on the run's own report holds whatever separator the host
#: spells paths with.
PRE_EXISTING_NAME = "01a1e000-0000-7000-8000-000000000003.jsonl"

#: What the shell seam answers in every case that runs a turn. The cases drive the seam rather than
#: reading this host's own answer, for the same reason they drive the platform probe: the suite also runs
#: in a child whose ``PATH`` holds no tool, where ``posix_shell`` would answer differently (or nothing at
#: all) than it does on the host the case happens to run on. It is a value no host is asked to have: it is
#: only handed to the stub and compared.
SHELL_SEAM_ANSWER = "/the-shell-the-seam-answered/sh"


class ProtectedArtifactsAfterNativeVerification(unittest.TestCase):
    """The source fixture copy, the planted session and every session the home already held are compared
    again after the harness's own native stages, not before them.

    The native half cannot run offline -- it needs the Claude Code binary and, for a turn, model access --
    so these cases drive the shipped ``main`` with the seams that half stands on replaced: the subprocess
    runner, the shell runner the turn goes through with the shell the run resolved for it, the shell
    probe that answers which POSIX shell that is, and ``shutil.which`` for the two tools. The stub for
    ``ccc continue`` installs a real file into a throw-away home (at the path the shipped ``main``
    computes from the scratch source root it spawned, so the installation check is answered honestly),
    and the stub for the resume turn answers like a turn that continued (the installed file grows) while
    it changes all three protected artifacts. With the comparisons at the end of the run, the mutating
    case must FAIL exactly those three checks and PASS the turn; if the comparisons move back above the
    loader and the turn -- the defect this pins -- they answer on the pre-mutation state, so the mutating
    case below fails over an empty failure list while the intact control still passes: the ordering
    defect breaks the catching, not the passing. The intact run is the control for the other direction --
    a comparison that answered FAIL for its own reasons could not pass it -- and the mutating run asserts
    the three protected checks are its **only** failures, so nothing else was already red.

    The home carries a session of its own (``PRE_EXISTING_NAME``) besides the one the harness plants, and
    that is what makes the third comparison's evidence non-vacuous rather than a restatement: with an
    empty capture, "no pre-existing session in the target home changed" would iterate nothing, so
    neutering the capture altogether would keep it green. Here the intact run has to report the capture it
    compared (a count of one) and the mutating run has to name the file it caught changing.
    """

    def drive(self, mutate_during_resume: bool, posix_mode_bits: bool | None = None,
              malformed_installed: bool = False, exec_terminator: bool = True,
              posix_shell: str | None = SHELL_SEAM_ANSWER) -> tuple[int, dict[str, tuple[str, str]], str, str | None]:
        """One run of the shipped ``main`` against stubs; answers its exit code, its recorded checks, its
        printed output and the shell the stub turn was asked to run through (``None`` when no turn ran).

        ``posix_mode_bits`` is what the platform seam answers; ``None`` (the default) means *this* host's
        own answer, so a case that does not name it holds on a host that reports POSIX mode bits and on
        one that does not (``False`` drives the Windows answer explicitly), and ``malformed_installed``
        makes the stub's installed file carry a line that is not JSON, so the JSONL check's own failure
        path is exercised. ``exec_terminator`` is whether the stub's reported exec form carries the option
        terminator before its placeholder: ``False`` is the shape a printed command without one has, which
        the check's reading of the reported invocation must refuse. ``posix_shell`` is what the shell seam
        answers, defaulted to ``SHELL_SEAM_ANSWER`` rather than to this host's own so that a case holds
        under an isolated ``PATH`` (the suite's child runs hold no tool) and on every platform; ``None``
        drives the host with no POSIX shell, whose turn the run must report as a boundary instead of
        running it through the wrong shell.
        """
        # The scratch root is resolved before anything is derived from it, because the shipped ``main``
        # resolves what it is handed (``--repo`` and ``--claude-home``) and resolves its own scratch root
        # as well: a host whose temporary root is reached through a symlink, or whose ``resolve()``
        # answers a canonical spelling of it -- macOS spells the temp root ``/var/folders/…`` and reads it
        # back as ``/private/var/folders/…`` -- would leave this case's stub answering in a spelling the
        # harness never uses, so the stubbed commands, the document's own paths and the run's recorded
        # checks would all disagree with it on that host alone.
        scratch = Path(tempfile.mkdtemp(prefix="ccc-claude-native-check-case-")).resolve()
        self.addCleanup(shutil.rmtree, scratch, ignore_errors=True)
        repo = scratch / "repo"
        fixture = (repo / NATIVE_CHECK.FIXTURE_ROOT / NATIVE_CHECK.SOURCE_PROVIDER
                   / "-fake-project" / f"{NATIVE_CHECK.SOURCE_SESSION}.jsonl")
        fixture.parent.mkdir(parents=True)
        fixture.write_text(json.dumps({"type": "session_meta",
                                       "payload": {"id": NATIVE_CHECK.SOURCE_SESSION,
                                                   "cwd": "/home/user/code/example-api"}}) + "\n")
        home = scratch / "claude-home"
        home.mkdir()
        # The two tools the harness resolves: names it only compares and runs through the stubs below, so
        # no binary has to exist for this case.
        claude = str(scratch / "claude")
        go = str(scratch / "go")

        thread_id = "01a1e000-0000-7000-8000-000000000002"
        planted = (home / NATIVE_CHECK.CLAUDE_PROJECTS_SUBDIR / NATIVE_CHECK.UNRELATED_PROJECT
                   / f"{NATIVE_CHECK.UNRELATED_SESSION}.jsonl")
        # A session the home already held before this run started, besides the one the harness plants:
        # the third comparison's subject. The harness only digests it, so a metadata line is all it needs
        # to be, and its id is neither the source's nor the conversion's.
        pre_existing = (home / NATIVE_CHECK.CLAUDE_PROJECTS_SUBDIR / "-pre-existing"
                        / PRE_EXISTING_NAME)
        pre_existing.parent.mkdir(parents=True)
        pre_existing.write_text(json.dumps({"type": "user", "uuid": PRE_EXISTING_NAME[:-6]}) + "\n")

        state: dict[str, Path] = {}

        def fake_run(cmd, cwd=None, env=None, timeout=600):
            command = [str(part) for part in cmd]
            if command[0] == go:
                return subprocess.CompletedProcess(cmd, 0, "", "")
            if command[0] == str(repo / NATIVE_CHECK.BINARY):
                if command[1] == "continue":
                    # The shipped ``main`` spawns its own scratch root and names it in the conversion's
                    # environment; the project directory a session is filed under is that root's
                    # workspace directory's spelling, which is the rule this case reproduces rather than
                    # guesses at.
                    codex_root = Path(env["CODEX_SESSIONS_DIR"])
                    cwd_path = codex_root.parent / NATIVE_CHECK.WORKSPACE
                    target_home = Path(command[command.index("--target-home") + 1])
                    project_dir = NATIVE_CHECK.claude_project_dir(str(cwd_path))
                    installed = target_home / NATIVE_CHECK.CLAUDE_PROJECTS_SUBDIR / project_dir / (
                        f"{thread_id}.jsonl")
                    installed.parent.mkdir(parents=True, exist_ok=True)
                    installed.write_text(
                        json.dumps({"type": "user", "sessionId": thread_id}) + "\n"
                        + ("{not json}\n" if malformed_installed else ""))
                    os.chmod(installed, 0o600)
                    state["installed"] = installed
                    state["cwd"] = cwd_path
                    state["source"] = codex_root / fixture.relative_to(
                        repo / NATIVE_CHECK.FIXTURE_ROOT / NATIVE_CHECK.SOURCE_PROVIDER)
                    resume_arguments = [claude, "--resume", thread_id]
                    # The exec form as the route prints it, the option terminator included;
                    # ``exec_terminator=False`` is the pre-fix spelling (the prompt bare), which the
                    # check's reading of the reported invocation must refuse.
                    exec_resume_arguments = [claude, "-p", "--output-format", "json", "--resume",
                                             thread_id,
                                             *((NATIVE_CHECK.CONTINUE_TERMINATOR,)
                                               if exec_terminator else ()),
                                             NATIVE_CHECK.PROMPT_PLACEHOLDER]
                    document = {
                        "conversion": {
                            "state": "converted", "format": "claude-code-transcript-jsonl",
                            "carried": {"messages": 3, "user_messages": 2, "assistant_messages": 1,
                                        "tool_calls": 1, "tool_results": 1,
                                        "compaction_boundaries": 1, "bytes": 1024},
                            "losses": [{"category": "message_model_attribution",
                                        "disposition": "omitted", "count": 2,
                                        "detail": "the stub's one non-zero loss"}],
                        },
                        "installation": {"state": "installed", "path": str(installed),
                                         "mechanism": "hard-link", "created": True,
                                         "source_check_state": "unchanged"},
                        "target_session": {
                            "session_id": thread_id, "path": str(installed), "binary_found": True,
                            "cwd": str(cwd_path),
                            "resume_arguments": resume_arguments,
                            "exec_resume_arguments": exec_resume_arguments,
                            "resume_environment": {"CLAUDE_CONFIG_DIR": str(target_home)},
                            "resume_command": rendering(cwd_path, target_home, resume_arguments),
                            "exec_resume_command": rendering(cwd_path, target_home,
                                                              exec_resume_arguments),
                        },
                        "verification": {
                            "structure": {"state": "verified",
                                          "method": "the stub's own read-back",
                                          "read_back": {"session_id": thread_id, "messages": 7,
                                                        "user_messages": 3, "assistant_messages": 4,
                                                        "tool_calls": 1, "tool_results": 1,
                                                        "compaction_boundaries": 1}},
                            "load": {"state": "verified",
                                     "detail": "the loader resolved the installed session; the control "
                                               "arm answered \"No conversation found with session ID: "
                                               "00000000-0000-4000-8000-000000000000\""},
                            "resume": {"state": "not-requested"},
                        },
                    }
                    return subprocess.CompletedProcess(cmd, 0, json.dumps(document), "")
                if command[1] == "inspect":
                    counts = {"messages": 7, "user_messages": 3, "assistant_messages": 4,
                              "system_messages": 0, "tool_messages": 0, "system_records": 1,
                              "sidechain_messages": 0, "compact_boundaries": 1,
                              "tool_calls": 1, "tool_results": 1}
                    return subprocess.CompletedProcess(
                        cmd, 0, json.dumps({"source": {"counts": counts}}), "")
            raise AssertionError(f"the case's stub was asked for an unexpected command: {command}")

        def fake_shell(line, cwd, env, shell, timeout=900):
            # The resume stage: a turn that continued (the installed file grows) and, when this case asks
            # for it, the mutation all three protected artifacts have to survive. The shell the shipped
            # ``main`` resolved is recorded here, so a case can assert which shell the line was read by.
            state["shell"] = shell
            installed = state["installed"]
            installed.write_text(installed.read_text() + json.dumps({"type": "assistant"}) + "\n")
            if mutate_during_resume:
                state["source"].write_text(state["source"].read_text()
                                           + json.dumps({"type": "assistant"}) + "\n")
                planted.write_text(planted.read_text() + json.dumps({"type": "user"}) + "\n")
                pre_existing.write_text(pre_existing.read_text() + json.dumps({"type": "assistant"}) + "\n")
            return subprocess.CompletedProcess(line, 0, json.dumps({"result": "the stub turn answered"}),
                                               "")

        def fake_which(name, *args, **kwargs):
            return claude if name == "claude" else go

        before = len(NATIVE_CHECK.RESULTS)
        printed = io.StringIO()
        host_posix_mode_bits = NATIVE_CHECK.reports_posix_mode_bits() if posix_mode_bits is None \
            else posix_mode_bits
        with mock.patch.object(NATIVE_CHECK, "run", fake_run), \
                mock.patch.object(NATIVE_CHECK, "run_shell", fake_shell), \
                mock.patch.object(NATIVE_CHECK, "reports_posix_mode_bits",
                                  lambda: host_posix_mode_bits), \
                mock.patch.object(NATIVE_CHECK, "posix_shell", lambda: posix_shell), \
                mock.patch("shutil.which", fake_which), \
                contextlib.redirect_stdout(printed):
            code = NATIVE_CHECK.main(["--repo", str(repo), "--claude", "claude",
                                      "--claude-home", str(home),
                                      "--resume-prompt", "Summarise this conversation."])
        results = {name: (state_, detail) for name, state_, detail in NATIVE_CHECK.RESULTS[before:]}
        del NATIVE_CHECK.RESULTS[before:]
        return code, results, printed.getvalue(), state.get("shell")

    def test_a_mutation_during_the_claude_resume_stage_is_caught_by_the_last_comparison(self) -> None:
        code, results, printed, shell = self.drive(mutate_during_resume=True)
        self.assertEqual(shell, SHELL_SEAM_ANSWER,
                         f"the turn was handed {shell!r} rather than the shell the seam answered, so "
                         f"this case is not about the turn it stubs: {results}")
        self.assertEqual(results.get("a real turn continues from the installed session", ("", ""))[0],
                         "PASS",
                         f"the stub's turn did not run, so its mutation would not be what the "
                         f"comparisons catch: {results}")
        failed = sorted(name for name, (state, _) in results.items() if state == "FAIL")
        self.assertEqual(failed, sorted(PROTECTED_CHECKS),
                         f"the mutating run's failures are not exactly the three protected-artifact "
                         f"comparisons: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run with failed checks: {results}")
        self.assertIn("the source root changed:", results[PROTECTED_CHECKS[0]][1])
        self.assertIn(" before, ", results[PROTECTED_CHECKS[1]][1])
        self.assertIn(" after", results[PROTECTED_CHECKS[1]][1])
        # The session the home already held is named as the one that changed: a comparison reading an
        # empty capture, or not running, could not report this file.
        self.assertIn("changed:", results[PROTECTED_CHECKS[2]][1])
        self.assertIn(PRE_EXISTING_NAME, results[PROTECTED_CHECKS[2]][1])
        # The answer is printed under the marker `scripts/tests/test_evidence_claims.py` reads a recorded
        # answer from, indented by four spaces and bounded.
        self.assertIn(f"\n{TURN_OUTPUT_MARKER}\n", printed, printed)
        self.assertIn("\n    the stub turn answered\n", printed, printed)

    def test_an_intact_claude_run_passes_the_comparisons(self) -> None:
        code, results, _, shell = self.drive(mutate_during_resume=False)
        self.assertEqual(shell, SHELL_SEAM_ANSWER,
                         f"the turn was handed {shell!r}, so the run did not read the line through the "
                         f"shell the seam answered")
        for name in PROTECTED_CHECKS:
            state, detail = results.get(name, ("missing", ""))
            self.assertEqual(state, "PASS", f"{name}: {state} — {detail}; all results: {results}")
        # The third comparison's own evidence: it says what it compared, so a capture that was never
        # taken (an empty one would answer PASS for every mutation) is red here rather than green.
        self.assertIn("1 pre-existing session file(s)", results[PROTECTED_CHECKS[2]][1],
                      "the run's pre-existing-session comparison did not name the session the home "
                      "already held: a capture that never happened agrees with itself")
        # The owner-only check's state is the platform's own answer, taken from the seam the script
        # reads: the very comparison on a host that reports POSIX mode bits, a named SKIP on one that
        # does not. Asserting PASS unconditionally would fail this case on Windows for the platform's
        # answer rather than for the harness's behaviour.
        self.assertEqual(results["the installed session is readable and writable only by its owner"][0],
                         "PASS" if NATIVE_CHECK.reports_posix_mode_bits() else "SKIP", results)
        self.assertEqual(code, 0, f"exit {code} for an intact run: {results}")

    def test_a_host_without_posix_mode_bits_skips_the_owner_only_check(self) -> None:
        """The owner-only check is a named SKIP where the platform reports no POSIX mode bits.

        Windows answers every ``stat`` with the all-bits mode regardless of the installer's `0600`
        request, so the *request* is the checkable half there and this harness, which reads the installed
        file's mode back, cannot answer the rule. The check must not FAIL -- a FAIL would make the intact
        run exit 1 on that leg, contradicting the harness's own exit-code contract -- so it is recorded as
        a SKIP that names the platform reason, and the intact run still exits 0.
        """
        code, results, printed, _ = self.drive(mutate_during_resume=False, posix_mode_bits=False)
        state, detail = results.get("the installed session is readable and writable only by its owner",
                                    ("missing", ""))
        self.assertEqual(state, "SKIP",
                         f"a host without POSIX mode bits did not skip the owner-only check: {results}")
        self.assertIn("no POSIX mode bits", detail, detail)
        self.assertIn("SKIP: the installed session is readable and writable only by its owner", printed)
        self.assertEqual(code, 0,
                         f"exit {code} for an intact run on a host without POSIX mode bits: {results}")

    def test_a_host_with_no_posix_shell_reports_the_turn_as_a_boundary(self) -> None:
        """The turn is a POSIX line, so a host with no POSIX shell reports the boundary, not a failure.

        The reported line is `cd <dir> && CLAUDE_CONFIG_DIR=<home> claude …`, each word quoted where a
        POSIX shell would act on it: `cmd.exe` cannot read that at all, and running it there would fail
        before the harness was reached. Leaving the shell to `subprocess`'s `shell=True` therefore made
        the real-turn check answer about the platform's command shell on Windows rather than about the
        conversion -- a failure this host alone could produce, which is why the shell is named instead.
        With the seam answering that there is none, the run must record the turn as a named SKIP with the
        reason, must not call the shell runner at all, and must still exit 0: a boundary is not a failure.
        """
        code, results, printed, shell = self.drive(mutate_during_resume=False, posix_shell=None)
        state, detail = results.get("a real turn continues from the installed session", ("missing", ""))
        self.assertEqual(state, "SKIP",
                         f"a host with no POSIX shell did not report the turn as a boundary: {results}")
        self.assertIn("no POSIX shell", detail, detail)
        self.assertEqual(shell, None,
                         "the shell runner was called on a host the seam answered has no POSIX shell, so "
                         "the line was read by whatever the platform's own command shell is")
        self.assertNotIn(TURN_OUTPUT_MARKER, printed,
                         f"the run printed a turn's output on a host where no turn ran: {printed}")
        self.assertIn("SKIP: a real turn continues from the installed session", printed)
        self.assertEqual(code, 0, f"exit {code} for a run whose turn is a boundary: {results}")

    def test_a_malformed_claude_installed_line_is_a_named_failure(self) -> None:
        """An unparseable installed line is a FAIL the run reports, not a traceback out of ``main``.

        The JSONL check is a named check, and a raise would print no ``FAIL`` line and no summary -- the
        very reporting contract the harness promises never to break. The run must therefore return 1 with
        that check recorded as a FAIL whose detail names the line and the parse error.
        """
        code, results, printed, _ = self.drive(mutate_during_resume=False, malformed_installed=True)
        state, detail = results.get("the installed file is one JSON object per line", ("missing", ""))
        self.assertEqual(state, "FAIL",
                         f"the malformed installed line did not fail the JSONL check: {results}")
        self.assertIn("line 2 is not JSON", detail, detail)
        self.assertEqual(code, 1, f"exit {code} for a run whose JSONL check failed: {results}")
        self.assertIn("FAIL: the installed file is one JSON object per line", printed)

    def test_the_reported_exec_form_must_carry_the_prompt_terminator(self) -> None:
        """The check refuses a printed exec form whose prompt is not behind the option terminator.

        The route runs the resume verification's prompt behind `--` (`internal/ccc/claude_verify.go`), and
        the printed exec form is that command with the placeholder in the prompt's place: without the
        terminator a prompt whose own first word is an option (`--help`) is read as one by the harness, so
        the printed line would run something other than the turn the verification ran. The check's reading
        of the reported invocation is where that rule is answerable with no harness installed, so this case
        drives both shapes against the stub document: the intact vector passes, and the pre-fix vector --
        the placeholder where the prompt goes with no terminator before it -- fails exactly that check.
        """
        code, results, _, _ = self.drive(mutate_during_resume=False)
        state, detail = results.get(REPORTED_INVOCATION_CHECK, ("missing", ""))
        self.assertEqual(state, "PASS",
                         f"the intact exec form did not pass the invocation check: {state} — {detail}")
        self.assertEqual(code, 0, f"exit {code} for an intact run: {results}")

        code, results, _, _ = self.drive(mutate_during_resume=False, exec_terminator=False)
        state, detail = results.get(REPORTED_INVOCATION_CHECK, ("missing", ""))
        self.assertEqual(state, "FAIL",
                         f"a printed exec form without the terminator was accepted: {results}")
        self.assertNotIn(f" {NATIVE_CHECK.CONTINUE_TERMINATOR} '{NATIVE_CHECK.PROMPT_PLACEHOLDER}'",
                         detail,
                         f"the failure's own evidence shows the terminator it is about: {detail}")
        self.assertEqual(sorted(name for name, (state_, _) in results.items() if state_ == "FAIL"),
                         [REPORTED_INVOCATION_CHECK],
                         f"the pre-fix shape failed more than the check that reads it: {results}")
        self.assertEqual(code, 1, f"exit {code} for a run whose reported form was refused: {results}")


if __name__ == "__main__":
    unittest.main()
