#!/usr/bin/env python3
"""Regression tests for the checks in scripts/omp-native-check.py that need no OMP binary.

Five subjects, all driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the project directory OMP files a session under (``omp_project_dir``, the rule
``internal/ccc/omp_target.go`` computes and this harness re-derives), the seeding of the run's own data
directory from the host's own OMP home (``seed_omp_home`` -- which providers survive and what their
``apiKey`` becomes), the reading of a written session's own bytes (the assistant ``model``/``provider``
pair OMP's request builder indexes, the fixed-width first line, and the roles the loader builds from the
file's own ``parentId`` walk), the printed resume line read the way a POSIX shell reads it
(``ReportedInvocationCases`` -- OMP's line names both the data directory and the session directory inside
it), and the ``INCOMPLETE`` verdict a run reports when the OMP binary is not installed (``main`` driven
with an absent binary).

The spelling is the subject the whole route turns on, and it is **not** Pi's: measured on OMP 18.18.7, a
working directory under the OS temporary directory spells ``-tmp`` + its relative path (that branch wins
when both apply), one under the home directory spells ``-`` + its home-relative path, and only a path
outside both keeps Pi's absolute ``--…--`` form -- while OMP *moves* a session it finds under Pi's
spelling into its own directory itself. The table below is the one the Go case
(``internal/ccc/omp_target_test.go``) pins, so the two implementations of the same rule are held to the
same cases rather than each to its own.

The seeding cases are about a policy rather than a shape: OMP demands a literal key on a provider
(measured: without one it refuses to start at all, and ``$VAR``/``${VAR}`` in ``apiKey`` is **not**
expanded), so a run that wants a turn has to write *something*. What it writes is a placeholder it
invents, for a **loopback** endpoint only, with the host's own credential-bearing values dropped -- and
the case asserts the dropped values are not on disk, which is the property a future edit could break.

Nothing here needs a model, a network, or the harness: the cases drive the shipped module in-process, the
way ``scripts/tests/test_pi_native_check.py`` drives its script.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parents[1]

#: Values the cases use as if they were credentials: distinctive strings, so an assertion that one is
#: absent from a written file is about that value and not about a substring of something else.
LOOPBACK_SECRET = "sk-loopback-value-that-must-not-be-written"
REMOTE_SECRET = "sk-remote-value-that-must-not-be-written"
USERINFO_SECRET = "sk-userinfo-value-that-must-not-be-written"
HEADER_VALUE = "Bearer a-header-value-that-must-not-be-written"

#: The provider the host's own home is seeded with, and the one that must be left out whole.
LOOPBACK = "loopback-proxy"
REMOTE = "remote-endpoint"

TITLE_SLOT_BYTES = 256


def load_native_check():
    spec = importlib.util.spec_from_file_location(
        "shipped_omp_native_check", SCRIPTS / "omp-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


def session_file(directory: Path, name: str, text: str) -> Path:
    path = directory / name
    path.write_text(text, encoding="utf-8")
    return path


def entry(**fields) -> str:
    return json.dumps(fields) + "\n"


def title_slot_line(entry_text: str) -> str:
    """One title slot line padded to the harness's own width, line feed included."""
    padded = entry_text + " " * (TITLE_SLOT_BYTES - 1 - len(entry_text))
    assert len(padded) + 1 == TITLE_SLOT_BYTES
    return padded + "\n"


class ProjectDirCases(unittest.TestCase):
    """The directory OMP files a session under, which is the rule the installer asks."""

    #: ``(cwd, want)``, the same cases ``internal/ccc/omp_target_test.go`` pins.
    CASES = (
        ("a directory under the temporary directory", "/tmp/work/project", "-tmp-work-project"),
        ("the temporary directory itself", "/tmp", "-tmp"),
        ("a directory under the home directory", "/home/example/repos/call-ccc", "-repos-call-ccc"),
        ("the home directory itself", "/home/example", "-"),
        ("a directory under both, where the temporary one wins",
         "/tmp/home-example/work", "-tmp-home-example-work"),
        ("a directory outside both", "/srv/work/project", "--srv-work-project--"),
    )

    def test_the_three_branches(self):
        for name, cwd, want in self.CASES:
            with self.subTest(name):
                self.assertEqual(
                    NATIVE_CHECK.omp_project_dir(cwd, home="/home/example", tmp="/tmp"), want,
                    f"omp_project_dir({cwd!r})")

    def test_the_spelling_is_not_pis_where_it_differs(self):
        # For a working directory under the home or the temporary directory the two harnesses spell it
        # differently, and OMP moves a session it finds under Pi's spelling into its own directory.
        under_home = NATIVE_CHECK.omp_project_dir("/home/example/repos/call-ccc",
                                                  home="/home/example", tmp="/tmp")
        self.assertEqual(under_home, "-repos-call-ccc")
        self.assertNotEqual(under_home, "--home-example-repos-call-ccc--")
        # A path outside both is the one branch the two harnesses share, and Pi's spelling there is
        # `--<path with separators replaced>--`.
        outside = NATIVE_CHECK.omp_project_dir("/srv/work/project", home="/home/example", tmp="/tmp")
        self.assertEqual(outside, "--srv-work-project--")

    def test_a_windows_shaped_path_keeps_its_drive_colon_spelled(self):
        # The harness's own spelling replaces every separator *and* a drive colon (`ompSeparatorSpelling`),
        # so the two are one rule here rather than two: a case that only replaced separators would leave
        # `C:` in the directory name.
        self.assertEqual(NATIVE_CHECK.omp_separator_spelling("C:\\work\\api"), "C--work-api")


class SeedCases(unittest.TestCase):
    """What the run's own data directory is seeded with, and what must not be written."""

    HOST_MODELS = f"""\
providers:
  {LOOPBACK}:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    headers:
      Authorization: {HEADER_VALUE}
    models:
    - id: model-one
      name: model-one
    - id: model-two
      name: model-two
  {REMOTE}:
    baseUrl: https://api.example.com/v1
    api: openai-completions
    apiKey: {REMOTE_SECRET}
    models:
    - id: remote-model
  with-userinfo:
    baseUrl: https://user:password@api.example.com/v1
    api: openai-completions
    apiKey: {USERINFO_SECRET}
    models:
    - id: userinfo-model
  without-an-endpoint:
    api: openai-completions
    apiKey: {USERINFO_SECRET}
    models:
    - id: no-endpoint-model
"""

    def seed(self, home: Path, host: Path, preferred: str = "", base_url: str = ""):
        return NATIVE_CHECK.seed_omp_home(home, host, preferred, base_url)

    def test_only_a_loopback_provider_survives_and_no_value_is_written(self):
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            (host / "models.yml").parent.mkdir(parents=True, exist_ok=True)
            (host / "models.yml").write_text(self.HOST_MODELS, encoding="utf-8")
            (host / "config.yml").write_text(f"modelRoles:\n  default: {REMOTE}/remote-model\n",
                                             encoding="utf-8")
            home.mkdir()
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, [LOOPBACK], "seeded providers")
            self.assertEqual(role, f"{LOOPBACK}/model-one",
                             "the role the seeded home selects: the host's own default names a provider "
                             "that did not survive, so the first surviving model is used")
            self.assertEqual(len(left_out), 3, f"left out: {left_out}")
            for name in (REMOTE, "with-userinfo", "without-an-endpoint"):
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")

            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (LOOPBACK_SECRET, REMOTE_SECRET, USERINFO_SECRET, HEADER_VALUE):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            self.assertNotIn("headers", written)
            for name in (REMOTE, "with-userinfo", "without-an-endpoint"):
                self.assertNotIn(name, written, "a provider that must be left out whole was copied")
            # ...while the definition that did survive is the host's own, with its key replaced.
            self.assertIn(LOOPBACK, written)
            self.assertIn(NATIVE_CHECK.SANDBOX_KEY, written)
            self.assertIn("openai-responses", written)
            self.assertIn("model-one", written)
            self.assertIn("model-two", written)
            self.assertEqual(NATIVE_CHECK.configured_default_role(home), f"{LOOPBACK}/model-one")

    def test_the_callers_own_model_wins_when_its_provider_survived(self):
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(self.HOST_MODELS, encoding="utf-8")
            seeded, _, role = self.seed(home, host, preferred=f"{LOOPBACK}/model-two")
            self.assertEqual(seeded, [LOOPBACK])
            self.assertEqual(role, f"{LOOPBACK}/model-two")

    def test_a_non_loopback_endpoint_is_never_defined_even_when_asked(self):
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(self.HOST_MODELS, encoding="utf-8")
            seeded, left_out, role = self.seed(home, host, preferred="litellm/some-model",
                                               base_url="https://litellm.example.com/v1")
            self.assertNotIn("litellm", seeded)
            self.assertTrue(any(reason.startswith("litellm") for reason in left_out),
                            f"--provider-base-url's provider is not named among {left_out}")
            self.assertEqual(role, f"{LOOPBACK}/model-one")
            written = (home / "models.yml").read_text(encoding="utf-8")
            self.assertNotIn("litellm.example.com", written)

    def test_an_ipv6_loopback_endpoint_is_usable_for_the_callers_own_model(self):
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(self.HOST_MODELS, encoding="utf-8")
            seeded, left_out, role = self.seed(home, host, preferred="litellm/some-model",
                                               base_url="http://[::1]:4141/v1")
            # IPv6 loopback is loopback: the endpoint may be used, and the key is still this run's own.
            self.assertIn("litellm", seeded, left_out)
            written = (home / "models.yml").read_text(encoding="utf-8")
            self.assertIn(NATIVE_CHECK.SANDBOX_KEY, written)
            self.assertNotIn(LOOPBACK_SECRET, written)
            self.assertEqual(role, "litellm/some-model")

    def test_no_surviving_provider_leaves_the_home_with_no_model_role(self):
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            # Only a remote provider and one whose URL carries a credential: neither can be seeded.
            (host / "models.yml").write_text(
                f"""\
providers:
  {REMOTE}:
    baseUrl: https://api.example.com/v1
    api: openai-completions
    apiKey: {REMOTE_SECRET}
    models:
    - id: remote-model
""", encoding="utf-8")
            home.joinpath("config.yml").write_text("", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, [])
            self.assertTrue(any(reason.startswith(REMOTE) for reason in left_out), left_out)
            self.assertEqual(role, "")
            # No models.yml is written at all, and config.yml names no role: the home answers no model,
            # which the conversion reports rather than hiding (OMP's own measured behaviour).
            self.assertFalse((home / "models.yml").exists())
            self.assertNotIn("default", (home / "config.yml").read_text(encoding="utf-8"))


class SessionBytesCases(unittest.TestCase):
    """What the check makes of a written session's own bytes."""

    def test_the_assistant_pair_is_required_on_every_assistant_record(self):
        with tempfile.TemporaryDirectory() as scratch:
            directory = Path(scratch)
            complete = session_file(directory, "complete.jsonl", "".join([
                entry(type="title", title=""),
                entry(type="session", version=3, id="s", cwd="/tmp/x"),
                entry(type="message", message={"role": "user", "content": []}),
                entry(type="message", message={"role": "assistant", "model": "m", "provider": "p"}),
            ]))
            assistants, without = NATIVE_CHECK.assistant_records_without_pair(complete)
            self.assertEqual((assistants, without), (1, 0))

            for name, message in (
                ("no-provider.jsonl", {"role": "assistant", "model": "m", "provider": ""}),
                ("no-model.jsonl", {"role": "assistant", "model": "", "provider": "p"}),
                ("neither.jsonl", {"role": "assistant"}),
            ):
                path = session_file(directory, name, entry(type="message", message=message))
                assistants, without = NATIVE_CHECK.assistant_records_without_pair(path)
                self.assertEqual((assistants, without), (1, 1), name)

    def test_the_first_line_must_be_the_fixed_width_title_slot(self):
        with tempfile.TemporaryDirectory() as scratch:
            directory = Path(scratch)
            good = session_file(directory, "good.jsonl", title_slot_line('{"type":"title","title":""}'))
            width, is_title = NATIVE_CHECK.title_slot_line(good)
            self.assertEqual((width, is_title), (TITLE_SLOT_BYTES, True))

            short = session_file(directory, "short.jsonl", '{"type":"title","title":""}\n')
            width, is_title = NATIVE_CHECK.title_slot_line(short)
            self.assertTrue(is_title)
            self.assertNotEqual(width, TITLE_SLOT_BYTES, "a short line must not pass the width check")

            header_first = session_file(directory, "header-first.jsonl",
                                        entry(type="session", version=3, id="s"))
            width, is_title = NATIVE_CHECK.title_slot_line(header_first)
            self.assertFalse(is_title, "a file whose first line is not the slot must not pass")

    def test_the_model_visible_roles_walk_the_files_own_path(self):
        # The file: a header, the binding, the conversation, a compaction boundary and a message after
        # it -- plus an assistant record nothing hangs off (the branch the conversation left) and a
        # custom record after the boundary. The loader rebuilds context from the *path* it walks from the
        # file's last entry back through `parentId`, so the abandoned record is recorded history and the
        # compaction entry on the path is the boundary.
        text = "".join([
            title_slot_line('{"type":"title","title":""}'),
            entry(type="session", version=3, id="s", cwd="/tmp/x"),
            entry(type="model_change", id="bind", parentId=None, model="p/m"),
            entry(type="message", id="u1", parentId="bind", message={"role": "user"}),
            entry(type="message", id="a1", parentId="u1", message={"role": "assistant"}),
            entry(type="message", id="abandoned", parentId="u1", message={"role": "assistant"}),
            entry(type="message", id="u2", parentId="a1", message={"role": "user"}),
            entry(type="compaction", id="c1", parentId="u2", summary="earlier turns"),
            entry(type="custom", id="x1", parentId="c1", customType="session_exit"),
            entry(type="message", id="a2", parentId="x1", message={"role": "assistant"}),
        ])
        self.assertEqual(NATIVE_CHECK.omp_model_visible_roles(text), ["compactionSummary", "assistant"])

        # With no compaction on the path the roles are every message the path holds, in order.
        plain = "".join([
            entry(type="session", version=3, id="s", cwd="/tmp/x"),
            entry(type="model_change", id="bind", parentId=None, model="p/m"),
            entry(type="message", id="u1", parentId="bind", message={"role": "user"}),
            entry(type="message", id="a1", parentId="u1", message={"role": "assistant"}),
        ])
        self.assertEqual(NATIVE_CHECK.omp_model_visible_roles(plain), ["user", "assistant"])

    def test_the_binding_is_read_the_way_the_harness_reads_it(self):
        with_binding = "".join([
            entry(type="session", version=3, id="s", cwd="/tmp/x"),
            entry(type="model_change", id="bind", parentId=None, model="home-provider/home-model"),
            entry(type="message", id="a1", parentId="bind",
                  message={"role": "assistant", "model": "source-model", "provider": "source-provider"}),
        ])
        self.assertEqual(NATIVE_CHECK.session_binding(with_binding),
                         ("home-provider", "home-model"))

        # Without a binding the pair the records carry is the answer, and a `model_change` that names no
        # `<provider>/<model>` is not one: the harness cannot resolve it either.
        without_binding = "".join([
            entry(type="session", version=3, id="s", cwd="/tmp/x"),
            entry(type="message", id="a1", parentId=None,
                  message={"role": "assistant", "model": "model-one", "provider": "provider-one"}),
            entry(type="message", id="a2", parentId="a1",
                  message={"role": "assistant", "model": "model-two", "provider": "provider-two"}),
        ])
        self.assertEqual(NATIVE_CHECK.session_binding(without_binding), ("provider-two", "model-two"))

        unusable = entry(type="model_change", id="bind", parentId=None, model="no-slash")
        self.assertEqual(NATIVE_CHECK.session_binding(unusable), ("", ""))


class ReportedInvocationCases(unittest.TestCase):
    """The printed resume line, read the way a POSIX shell reads it.

    OMP's line names **two** assignments -- the data directory and the directory inside it that holds the
    session's file -- because measured on 18.18.7 the harness resolves a session from the second: naming
    the `sessions` root above it answers that no such session is installed. A line that lost the second
    one would still look like a resume line, which is why the whole word vector is compared.
    """

    HOME = "/tmp/omp check home"          # a space, so the renderer has to quote it
    WORKSPACE = "/tmp/the workspace"
    SESSION = "01a1eba9-6b83-7ef6-b318-efae1673c2db"
    PROJECT_DIR = "-tmp-the-workspace"

    def invocation(self, session_dir: str | None = None) -> str:
        home = f"{NATIVE_CHECK.OMP_HOME_ENV}='{self.HOME}'"
        tree = session_dir if session_dir is not None else (
            f"{NATIVE_CHECK.OMP_SESSION_DIR_ENV}="
            f"'{NATIVE_CHECK.omp_session_tree(self.HOME, self.PROJECT_DIR)}'")
        return f"cd '{self.WORKSPACE}' && {home} {tree} omp --resume {self.SESSION}"

    def test_the_printed_line_names_the_directory_that_holds_the_file(self):
        home = self.HOME
        tree = NATIVE_CHECK.omp_session_tree(home, self.PROJECT_DIR)
        self.assertEqual(tree, f"{home}/{NATIVE_CHECK.OMP_SESSIONS_SUBDIR}/{self.PROJECT_DIR}")
        self.assertTrue(NATIVE_CHECK.reported_invocation(
            self.invocation(), self.WORKSPACE, home, NATIVE_CHECK.OMP_HOME_ENV,
            ["omp", "--resume", self.SESSION],
            session_dir_env=NATIVE_CHECK.OMP_SESSION_DIR_ENV, session_dir=tree))

    def test_a_line_without_the_session_directory_is_not_the_invocation(self):
        home = self.HOME
        tree = NATIVE_CHECK.omp_session_tree(home, self.PROJECT_DIR)
        missing = f"cd '{self.WORKSPACE}' && {NATIVE_CHECK.OMP_HOME_ENV}='{home}' omp --resume {self.SESSION}"
        self.assertFalse(NATIVE_CHECK.reported_invocation(
            missing, self.WORKSPACE, home, NATIVE_CHECK.OMP_HOME_ENV,
            ["omp", "--resume", self.SESSION],
            session_dir_env=NATIVE_CHECK.OMP_SESSION_DIR_ENV, session_dir=tree))
        # The sessions root above the project directory is exactly what the harness answers "not found"
        # for, and it is not this invocation either.
        root = f"{home}/{NATIVE_CHECK.OMP_SESSIONS_SUBDIR}"
        self.assertFalse(NATIVE_CHECK.reported_invocation(
            self.invocation(session_dir=f"{NATIVE_CHECK.OMP_SESSION_DIR_ENV}='{root}'"),
            self.WORKSPACE, home, NATIVE_CHECK.OMP_HOME_ENV, ["omp", "--resume", self.SESSION],
            session_dir_env=NATIVE_CHECK.OMP_SESSION_DIR_ENV, session_dir=tree))

    def test_a_quoted_assignment_is_not_read_as_the_invocation(self):
        home = self.HOME
        tree = NATIVE_CHECK.omp_session_tree(home, self.PROJECT_DIR)
        quoted = (f"cd '{self.WORKSPACE}' && '{NATIVE_CHECK.OMP_HOME_ENV}={home}' omp --resume "
                  f"{self.SESSION}")
        self.assertFalse(NATIVE_CHECK.reported_invocation(
            quoted, self.WORKSPACE, home, NATIVE_CHECK.OMP_HOME_ENV, ["omp", "--resume", self.SESSION],
            session_dir_env=NATIVE_CHECK.OMP_SESSION_DIR_ENV, session_dir=tree))


class VerdictCases(unittest.TestCase):
    """The verdict a run reports, without a harness, a model or a scratch tree."""

    def test_a_missing_harness_is_incomplete_rather_than_a_pass(self):
        with tempfile.TemporaryDirectory() as scratch:
            absent = str(Path(scratch) / "omp-does-not-exist")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                code = NATIVE_CHECK.main(["--omp", absent])
            self.assertEqual(code, 2)
            printed = output.getvalue()
            self.assertIn("RESULT: INCOMPLETE", printed)
            self.assertIn("not on PATH", printed)


if __name__ == "__main__":
    unittest.main()
