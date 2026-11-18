#!/usr/bin/env python3
"""Regression tests for the checks in scripts/omp-native-check.py that need no OMP binary.

The subjects, all driven in-process with the shipped module imported by path, no subprocess and no tool
on ``PATH``: the project directory OMP files a session under (``omp_project_dir``, the rule
``internal/ccc/omp_target.go`` computes and this harness re-derives), the seeding of the run's own data
directory from the host's own OMP home (``seed_omp_home`` -- which providers survive, what their
``apiKey`` becomes, and the key spellings the read must treat as the same field), the model role one
home's ``config.yml`` names (``configured_default_role``, in both the block and the flow spelling), the
answers that decide whether a requested real turn can run (``requested_turn_model`` and the failed check
an unattempted turn records), the reading of a written session's own bytes (the assistant
``model``/``provider`` pair OMP's request builder indexes, the fixed-width first line, and the roles the
loader builds from the file's own ``parentId`` walk), the printed resume line read the way a POSIX shell
reads it (``ReportedInvocationCases`` -- OMP's line names both the data directory and the session
directory inside it), and the ``INCOMPLETE`` verdict a run reports when the OMP binary is not installed
(``main`` driven with an absent binary).

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
the case asserts the dropped values are not on disk, which is the property a future edit could break. The
policy is about the *field*, not about a prefix: a quoted key (``"apiKey"``) and a key spelled with
whitespace before its colon (``apiKey :``) are the same field to any YAML reader, and a key at another
casing (``ApiKey``) is treated the same way on purpose, so each of them must be replaced or dropped too,
and a key form this seeder's reader cannot decode (YAML's explicit ``? key``, a quoted key it cannot
unquote) leaves the whole definition out rather than copy a key the seeder did not read.

Nothing here needs a model, a network, or the harness: the cases drive the shipped module in-process, the
way ``scripts/tests/test_pi_native_check.py`` drives its script.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import re
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


def abs_host_path(path: str) -> str:
    """One spelled path resolved the way the rule resolves a working directory: ``os.path.abspath``.

    The rule resolves the working directory (and, inside ``omp_within``, each base) before it compares
    them, so a base or a case left as a POSIX literal pins the wrong answer on a platform whose rooted
    paths carry a volume (Windows): a base spelled ``/tmp`` there has no relative spelling to the
    resolved path at all, both shortened branches become unreachable, and the absolute branch of the
    literal spells a name that host's rule never produces (its own drive letter is part of the spelling).
    On this host ``os.path.abspath`` is the identity of the literals below, which keeps this table the
    same cases its Go twin pins (``internal/ccc/omp_target_test.go``, ``absHostPath``).
    """
    return os.path.abspath(path)


def outside_spelling(path: str) -> str:
    """The ``--…--`` branch's own spelling of an absolute path: its leading separators removed, every
    separator and drive colon spelled ``-``, wrapped in ``--`` on both sides. Which is Pi's spelling too:
    that branch is the one case where the two harnesses agree.

    Spelled here rather than through the shipped ``omp_separator_spelling``, so the expectation is the
    rule's own words rather than the code under test's.
    """
    return "--" + re.sub(r"[/\\:]", "-", path.lstrip("/\\")) + "--"


def load_native_check():
    spec = importlib.util.spec_from_file_location(
        "shipped_omp_native_check", SCRIPTS / "omp-native-check.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


NATIVE_CHECK = load_native_check()


def session_file(directory: Path, name: str, text: str) -> Path:
    # Bytes rather than text: a text-mode write translates "\n" into the platform's own line separator
    # (CRLF on Windows), and the cases below measure the file's own bytes -- the fixed-width title slot's
    # length included -- so a translated line feed would break the reading they are about.
    path = directory / name
    path.write_bytes(text.encode("utf-8"))
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

    #: The base directories, and the one working directory outside both, resolved the way the rule
    #: resolves a working directory (``abs_host_path``). The bases have to be the host's own spellings
    #: for the temporary and home branches to be reachable on every platform, and the outside case's
    #: expectation depends on the whole resolved path, so its own path does too.
    HOME = abs_host_path("/home/example")
    TMP = abs_host_path("/tmp")
    OUTSIDE = abs_host_path("/srv/work/project")

    #: ``(cwd, want)``, the same cases ``internal/ccc/omp_target_test.go`` pins. Only the outside row's
    #: expectation depends on the whole path: the two shortened branches strip their base, so their
    #: spellings are the same words on every host.
    CASES = (
        ("a directory under the temporary directory",
         abs_host_path("/tmp/work/project"), "-tmp-work-project"),
        ("the temporary directory itself", TMP, "-tmp"),
        ("a directory under the home directory",
         abs_host_path("/home/example/repos/call-ccc"), "-repos-call-ccc"),
        ("the home directory itself", HOME, "-"),
        ("a directory under both, where the temporary one wins",
         abs_host_path("/tmp/home-example/work"), "-tmp-home-example-work"),
        ("a directory outside both", OUTSIDE, outside_spelling(OUTSIDE)),
    )

    def test_the_three_branches(self):
        for name, cwd, want in self.CASES:
            with self.subTest(name):
                self.assertEqual(
                    NATIVE_CHECK.omp_project_dir(cwd, home=self.HOME, tmp=self.TMP), want,
                    f"omp_project_dir({cwd!r})")

    def test_the_spelling_is_not_pis_where_it_differs(self):
        # For a working directory under the home or the temporary directory the two harnesses spell it
        # differently, and OMP moves a session it finds under Pi's spelling into its own directory.
        under_home_path = abs_host_path("/home/example/repos/call-ccc")
        under_home = NATIVE_CHECK.omp_project_dir(under_home_path, home=self.HOME, tmp=self.TMP)
        self.assertEqual(under_home, "-repos-call-ccc")
        self.assertNotEqual(under_home, outside_spelling(under_home_path))
        # A path outside both is the one branch the two harnesses share, and Pi's spelling there is
        # `--<path with separators replaced>--`.
        outside = NATIVE_CHECK.omp_project_dir(self.OUTSIDE, home=self.HOME, tmp=self.TMP)
        self.assertEqual(outside, outside_spelling(self.OUTSIDE))

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

    def test_a_multiline_key_value_is_removed_whole(self):
        # A provider whose apiKey is a block scalar -- a shape its YAML configuration vocabulary allows --
        # writes its value on the lines *below* the field. Replacing only the opening line would copy the
        # credential's continuation lines into the scratch home, the loader home and a --keep tree, so the
        # whole value is removed: the field keeps the one-line placeholder and every line deeper than it is
        # dropped. The folded and plain-continuation spellings of the same shape are covered for the same
        # reason.
        block_secret = "sk-block-scalar-value-that-must-not-be-written"
        folded_secret = "sk-folded-value-that-must-not-be-written"
        plain_secret = "sk-plain-continuation-that-must-not-be-written"
        second_line = "a-second-line-that-must-not-be-written"
        plain_continuation = "a-continuation-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  block-key-proxy:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: |-
      {block_secret}
      {second_line}
    models:
    - id: block-model
      name: block-model
  folded-key-proxy:
    baseUrl: http://127.0.0.1:4142/v1
    api: openai-responses
    apiKey: >-
      {folded_secret}
    models:
    - id: folded-model
  plain-key-proxy:
    baseUrl: http://127.0.0.1:4143/v1
    api: openai-responses
    apiKey: {plain_secret}
      {plain_continuation}
    models:
    - id: plain-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(
                seeded, ["block-key-proxy", "folded-key-proxy", "plain-key-proxy"],
                f"left out: {left_out}")
            self.assertEqual(role, "block-key-proxy/block-model")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (block_secret, folded_secret, plain_secret, second_line, plain_continuation):
                self.assertNotIn(value, written, "a credential line was written into a data directory")
            self.assertEqual(written.count(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}"), 3,
                             "each provider must keep the field with the one-line placeholder")
            for indicator in ("|-", ">-"):
                self.assertNotIn(indicator, written, "a block indicator survived the replacement")

    def test_a_plain_continuation_of_a_field_is_refused_whole(self):
        # A plain scalar can continue on a later, deeper line (checked against a YAML reader: the lines
        # fold into one value -- `http://…/v1 ?api_key=…`, and even a continuation line that opens with
        # `- ` folds, `a-name - ?api_key=…`), so a credential riding on the continuation line is a value
        # this seeder's line reader never attributed to a field -- and a definition carrying one was
        # copied, that line included, into the scratch home, the loader home and a --keep tree. Every
        # place a value can continue is refused whole, with the reason naming the continuation: the
        # provider's own field, a model entry's field, a kept field of any name, and a sequence item's
        # own scalar (a continuation line that itself looks like a sequence marker is caught the same
        # way, because it is one value to YAML). Three more spellings are refused on the same ground:
        # a value spelled under its field's name on the next line (the same single value, one line
        # lower), a line YAML reads as one plain scalar and not as a sequence item (`-text` opens no
        # item -- only `- ` does), and a sequence item whose own text is a form this reader cannot
        # follow (a block scalar, whose content lines are the item's value). The canonical provider
        # beside them is still seeded, so the refusal is about the spelling and not about the file.
        provider_secret = "sk-provider-continuation-that-must-not-be-written"
        model_secret = "sk-model-continuation-that-must-not-be-written"
        field_secret = "sk-field-continuation-that-must-not-be-written"
        marker_secret = "sk-marker-continuation-that-must-not-be-written"
        item_secret = "sk-item-continuation-that-must-not-be-written"
        marker_item_secret = "sk-item-marker-continuation-that-must-not-be-written"
        next_line_secret = "sk-next-line-plain-value-that-must-not-be-written"
        next_line_quoted_secret = "sk-next-line-quoted-value-that-must-not-be-written"
        block_item_secret = "sk-item-block-scalar-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  provider-continuation:
    baseUrl: http://127.0.0.1:4141/v1
      ?api_key={provider_secret}
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: provider-continuation-model
  model-continuation:
    baseUrl: http://127.0.0.1:4142/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: model-continuation-model
      baseUrl: http://127.0.0.1:4143/v1
        ?api_key={model_secret}
  field-continuation:
    baseUrl: http://127.0.0.1:4144/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    name: a-name
      ?api_key={field_secret}
    models:
    - id: field-continuation-model
  marker-continuation:
    baseUrl: http://127.0.0.1:4145/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    name: a-name
     - ?api_key={marker_secret}
    models:
    - id: marker-continuation-model
  item-continuation:
    baseUrl: http://127.0.0.1:4146/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: item-continuation-model
      input:
      - text
        ?api_key={item_secret}
  item-marker-continuation:
    baseUrl: http://127.0.0.1:4147/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: item-marker-continuation-model
      input:
      - text
        - ?api_key={marker_item_secret}
  next-line-plain-value:
    baseUrl: http://127.0.0.1:4148/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: next-line-plain-model
      baseUrl:
        http://127.0.0.1:4149/v1?api_key={next_line_secret}
  next-line-quoted-value:
    baseUrl: http://127.0.0.1:4150/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: next-line-quoted-model
      baseUrl:
        "http://127.0.0.1:4151/v1?api_key={next_line_quoted_secret}"
  empty-block-item:
    baseUrl: http://127.0.0.1:4152/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: empty-block-item-model
      input:
      - |
  block-item-with-content:
    baseUrl: http://127.0.0.1:4153/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: block-item-model
      input:
      - |
        {block_item_secret}
  bare-scalar-line:
    baseUrl: http://127.0.0.1:4155/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: bare-scalar-model
      input:
        -text
  canonical-continuation:
    # a comment line the copy keeps
    baseUrl: http://127.0.0.1:4154/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: canonical-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["canonical-continuation"], f"left out: {left_out}")
            self.assertEqual(role, "canonical-continuation/canonical-model")
            continuations = ("provider-continuation", "model-continuation", "field-continuation",
                             "marker-continuation", "item-continuation", "item-marker-continuation")
            attributed_by_name = ("next-line-plain-value", "next-line-quoted-value", "bare-scalar-line")
            refused = continuations + attributed_by_name + ("empty-block-item", "block-item-with-content")
            for name in refused:
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")
            for name in continuations:
                reason = next(reason for reason in left_out if reason.startswith(name))
                self.assertIn("continues a plain scalar", reason,
                              f"{name}'s refusal should name the continuation: {reason}")
            for name in attributed_by_name:
                reason = next(reason for reason in left_out if reason.startswith(name))
                self.assertIn("cannot attribute to a field", reason,
                              f"{name}'s refusal should say why the line has no field: {reason}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            self.assertIn("# a comment line the copy keeps", written,
                          "a comment line inside the definition is kept, as every other line is")
            for value in (provider_secret, model_secret, field_secret, marker_secret, item_secret,
                          marker_item_secret, next_line_secret, next_line_quoted_secret,
                          block_item_secret):
                self.assertNotIn(value, written, "a value line was written into a data directory")
            for name in refused:
                self.assertNotIn(name, written, "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)

    def test_a_provider_with_no_key_gains_the_runs_own_placeholder(self):
        # OMP refuses to start in a data directory whose provider carries no literal key (measured), so a
        # surviving provider that defines no `apiKey` at all keeps its definition and gains this run's
        # placeholder: a seeder that only rewrote an existing field would write a home the harness will
        # not start in.
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text("""\
providers:
  keyless-proxy:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    models:
    - id: keyless-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["keyless-proxy"], f"left out: {left_out}")
            self.assertEqual(role, "keyless-proxy/keyless-model")
            written = (home / "models.yml").read_text(encoding="utf-8")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)
            self.assertIn("keyless-model", written)

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

    def test_a_model_level_endpoint_with_a_credential_is_refused_whole(self):
        # The provider's own baseUrl is loopback; the credential rides on a model entry's -- the other
        # level OMP's own writer writes one at. The provider must be left out whole: `_seed_block`
        # rewrites key values and drops headers, not URLs, so a filter that read only the provider's own
        # line would copy this URL, credential and all, into the run's own home.
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  model-level-endpoint:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: model-level-model
      baseUrl: https://user:{USERINFO_SECRET}@api.example.com/v1
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, [], f"seeded: {seeded}")
            self.assertTrue(any(reason.startswith("model-level-endpoint") for reason in left_out), left_out)
            self.assertEqual(role, "")
            self.assertFalse((home / "models.yml").exists(),
                             "a refused provider's definition was written anyway")
            self.assertFalse((home / "config.yml").read_text(encoding="utf-8").find("default") >= 0,
                             "a model role was written for a refused provider")

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

    def test_quoted_and_spaced_key_spellings_name_the_same_fields(self):
        # A key is read as YAML spells it, not by a literal prefix: a quoted key and a key spelled with
        # whitespace before its colon are the same field to any YAML reader, and a key at another casing is
        # treated as the field on purpose (the conservative read). A seeder that recognized only `apiKey:`
        # would copy the quoted credential and the spaced header block -- values and all -- into the scratch
        # home, the loader home and a --keep tree, which is exactly what the no-credential-copy contract
        # forbids. The canonical spelling is in the same file, so the case also pins that the semantic read
        # did not stop recognizing it.
        double_secret = "sk-double-quoted-key-that-must-not-be-written"
        single_secret = "sk-single-quoted-key-that-must-not-be-written"
        spaced_secret = "sk-spaced-key-that-must-not-be-written"
        canonical_secret = "sk-canonical-key-that-must-not-be-written"
        quoted_header = "Bearer a-quoted-header-value-that-must-not-be-written"
        spaced_header = "Bearer a-spaced-header-value-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  double-quoted-key:
    "baseUrl": http://127.0.0.1:4141/v1
    api: openai-responses
    "apiKey": {double_secret}
    "headers":
      Authorization: {quoted_header}
    models:
    - id: double-model
  single-quoted-key:
    'baseUrl' : http://127.0.0.1:4142/v1
    api: openai-responses
    'apiKey' : {single_secret}
    models:
    - id: single-model
  spaced-and-cased-key:
    baseUrl : http://127.0.0.1:4143/v1
    api: openai-responses
    ApiKey: {spaced_secret}
    HEADERS:
      Authorization: {spaced_header}
    models:
    - id: spaced-model
  canonical-key:
    baseUrl: http://127.0.0.1:4144/v1
    api: openai-responses
    apiKey: {canonical_secret}
    headers:
      Authorization: {quoted_header}
    models:
    - id: canonical-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(
                seeded,
                ["canonical-key", "double-quoted-key", "single-quoted-key", "spaced-and-cased-key"],
                f"left out: {left_out}")
            self.assertEqual(role, "double-quoted-key/double-model")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (double_secret, single_secret, spaced_secret, canonical_secret, quoted_header,
                          spaced_header):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            self.assertNotIn("headers", written.lower())
            self.assertEqual(written.count(NATIVE_CHECK.SANDBOX_KEY), 4,
                             "every spelling of the field must keep the one-line placeholder")
            for name in ("double-quoted-key", "single-quoted-key", "spaced-and-cased-key",
                         "canonical-key"):
                self.assertIn(name, written)
            # The endpoint a quoted `baseUrl` names is read too: every provider above is loopback, so a
            # reader that missed a quoted endpoint key would have left that provider out whole.
            self.assertNotIn("canonical-key (it names no baseUrl)", left_out)

    def test_a_key_form_the_reader_cannot_decode_is_refused_whole(self):
        # YAML's explicit `? key` form and a quoted key whose escapes this seeder's reader cannot unquote
        # are keys the harness's own YAML reader *can* read -- so a definition carrying either must not be
        # copied at all: a copy would carry a key the seeder did not read, and a `headers`/`apiKey` spelled
        # that way would reach the scratch home, the loader home and a --keep tree unscrubbed. The provider
        # whose line opens with a quote that never closes on its own line is the same answer for the same
        # reason. The canonical provider in the same file must still be seeded, so the refusal is about the
        # definition that carries the form, not about the file.
        explicit_secret = "Bearer an-explicit-key-header-that-must-not-be-written"
        escaped_secret = "Bearer an-escaped-key-header-that-must-not-be-written"
        split_secret = "Bearer a-split-key-header-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  explicit-key:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    ? headers
    : {{Authorization: {explicit_secret}}}
    models:
    - id: explicit-model
  escaped-key:
    baseUrl: http://127.0.0.1:4142/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    "\\x68eaders": Bearer {escaped_secret}
    models:
    - id: escaped-model
  split-quoted-key:
    baseUrl: http://127.0.0.1:4143/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    "head
    ers": Bearer {split_secret}
    models:
    - id: split-model
  canonical-key:
    baseUrl: http://127.0.0.1:4144/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: canonical-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["canonical-key"], f"left out: {left_out}")
            self.assertEqual(role, "canonical-key/canonical-model")
            for name in ("explicit-key", "escaped-key", "split-quoted-key"):
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")
            # The explicit-key definition's own reason must name YAML's `?` form: its second line (`: …`)
            # is itself a form this reader refuses, so a run whose reader stopped recognizing `?` would
            # still leave the definition out -- while telling the operator about the wrong line.
            self.assertTrue(any("explicit `?` form" in reason for reason in left_out),
                            f"the explicit-key refusal should name YAML's `?` form: {left_out}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (explicit_secret, escaped_secret, split_secret):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            for name in ("explicit-key", "escaped-key", "split-quoted-key"):
                self.assertNotIn(name, written, "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)

    def test_a_flow_spelled_entry_is_refused_before_anything_is_copied(self):
        # YAML reads `- {id: demo, baseUrl: …}` as one model entry carrying both fields (checked against
        # a YAML reader), so a textual seeder that walked the line as a plain `id:` entry would never
        # examine the nested `baseUrl` and would copy the line verbatim -- a credential-bearing endpoint
        # going into the scratch home, the loader home and a --keep tree. The definition is refused whole
        # with the reason in the run's own output; the provider written the block way in the same file is
        # still seeded, so the refusal is about the spelling and not about the file.
        flow_secret = "sk-flow-spelled-endpoint-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  flow-spelled-endpoint:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - {{id: demo, baseUrl: "http://127.0.0.1:4142/v1?api_key={flow_secret}"}}
  block-spelled-endpoint:
    baseUrl: http://127.0.0.1:4143/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: block-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["block-spelled-endpoint"], f"left out: {left_out}")
            self.assertEqual(role, "block-spelled-endpoint/block-model")
            self.assertTrue(any(reason.startswith("flow-spelled-endpoint") for reason in left_out),
                            f"the flow-spelled provider is not named among {left_out}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            self.assertNotIn(flow_secret, written, "a credential value was written into a data directory")
            self.assertNotIn("flow-spelled-endpoint", written,
                             "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)

    def test_a_quoted_endpoint_is_decoded_before_the_credential_check(self):
        # A double-quoted scalar's escapes are YAML's, not decoration: `\u003f` is `?` to the harness's
        # own reader, so a filter that only removed the quotes would see no query delimiter in the raw
        # value while the harness reads a credential-bearing endpoint -- and would copy it. Both levels
        # are covered, the provider's own `baseUrl` and one a model entry writes. The accepting direction
        # is pinned in the same file: a host spelled `"http\u003a//127.0.0.1:4141/v1"` is loopback only
        # once its escapes are decoded, so a reader that never decoded would leave that provider out.
        escaped_provider_secret = "sk-escaped-provider-query-that-must-not-be-written"
        escaped_model_secret = "sk-escaped-model-endpoint-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  escaped-provider-endpoint:
    baseUrl: "http://127.0.0.1:4141/v1\\u003fapi_key={escaped_provider_secret}"
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: escaped-provider-model
  escaped-model-endpoint:
    baseUrl: http://127.0.0.1:4142/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: escaped-model
      baseUrl: "http://127.0.0.1:4143/v1\\u003fapi_key={escaped_model_secret}"
  decoded-endpoint:
    baseUrl: "http\\u003a//127.0.0.1:4144/v1"
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: decoded-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["decoded-endpoint"], f"left out: {left_out}")
            self.assertEqual(role, "decoded-endpoint/decoded-model")
            for name in ("escaped-provider-endpoint", "escaped-model-endpoint"):
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (escaped_provider_secret, escaped_model_secret):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            self.assertNotIn("escaped", written.lower(),
                             "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)

    def test_a_key_form_the_reader_mangles_is_refused_whole(self):
        # A key can be spelled so this textual reader reads a *different* key -- and YAML's own reader
        # reads the field the filter must scrub: an anchor or a tag on the key node (`&a apiKey:`,
        # `!!str headers:`), which YAML reads as `apiKey`/`headers` (checked against a YAML reader), and
        # a nested sequence marker in front of it (`- - apiKey:`), which this reader would read as the
        # key `- apiKey` and keep, credential and all. A line whose entry opens with no key at all (`: …`)
        # is the fourth spelling: the YAML reader this round checked refuses the document carrying it
        # (`did not find expected key`), and this seeder's reader will not name the line, so the
        # definition is refused rather than copied unread. Each definition is refused whole; the canonical
        # provider beside them is still seeded.
        anchored_secret = "sk-anchored-key-that-must-not-be-written"
        tagged_secret = "Bearer a-tagged-header-that-must-not-be-written"
        empty_key_secret = "sk-empty-key-that-must-not-be-written"
        nested_secret = "sk-nested-marker-key-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  anchored-key:
    baseUrl: http://127.0.0.1:4141/v1
    &a apiKey: {anchored_secret}
    models:
    - id: anchored-model
  tagged-header-key:
    baseUrl: http://127.0.0.1:4142/v1
    !!str headers:
      Authorization: {tagged_secret}
    models:
    - id: tagged-model
  empty-key:
    baseUrl: http://127.0.0.1:4143/v1
    : {{headers: {empty_key_secret}}}
    models:
    - id: empty-key-model
  nested-marker-key:
    baseUrl: http://127.0.0.1:4144/v1
    models:
    - - apiKey: {nested_secret}
  canonical-endpoint:
    baseUrl: http://127.0.0.1:4145/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: canonical-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["canonical-endpoint"], f"left out: {left_out}")
            self.assertEqual(role, "canonical-endpoint/canonical-model")
            for name in ("anchored-key", "tagged-header-key", "empty-key", "nested-marker-key"):
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (anchored_secret, tagged_secret, empty_key_secret, nested_secret):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            for name in ("anchored-key", "tagged-header-key", "nested-marker-key"):
                self.assertNotIn(name, written, "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)

    def test_a_value_form_the_reader_cannot_follow_is_refused_whole(self):
        # The value side of the same class: a block scalar's content lines and a flow collection's own
        # keys are fields to YAML's reader (`baseUrl` among them) that this textual reader never walks; a
        # double-quoted value that continues onto the next line can carry a credential on a line this
        # reader does not read as an entry (checked against a YAML reader: the continued value decodes to
        # `…/v1?api_key=…`, while its first line holds no `?` for the raw check to see); a quoted value
        # with an escape JSON does not know is a value it cannot decode; and content after a quoted
        # value's closing quote is a shape it does not read either. Each may carry a credential the filter
        # never read, so each definition is refused whole before anything is copied; the canonical
        # provider in the same file is still seeded.
        block_secret = "sk-block-scalar-endpoint-that-must-not-be-written"
        flow_value_secret = "sk-flow-value-endpoint-that-must-not-be-written"
        continued_secret = "sk-continued-quote-endpoint-that-must-not-be-written"
        undecodable_secret = "sk-undecodable-quoted-value-that-must-not-be-written"
        alias_secret = "sk-aliased-endpoint-that-must-not-be-written"
        with tempfile.TemporaryDirectory() as scratch:
            host = Path(scratch) / "host"
            home = Path(scratch) / "home"
            host.mkdir()
            home.mkdir()
            (host / "models.yml").write_text(f"""\
providers:
  block-scalar-endpoint:
    baseUrl: http://127.0.0.1:4141/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: block-scalar-model
      baseUrl: |-
        http://127.0.0.1:4142/v1?api_key={block_secret}
  flow-value-endpoint:
    baseUrl: http://127.0.0.1:4143/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models: [{{id: flow-value-model, baseUrl: "http://127.0.0.1:4144/v1?api_key={flow_value_secret}"}}]
  continued-quote-endpoint:
    baseUrl: http://127.0.0.1:4145/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: continued-quote-model
      baseUrl: "http://127.0.0.1:4146/v1\\
        ?api_key={continued_secret}"
  undecodable-quoted-value:
    baseUrl: http://127.0.0.1:4147/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: undecodable-model
      name: "a-\\x41-value-{undecodable_secret}"
  trailing-content-endpoint:
    baseUrl: "http://127.0.0.1:4148/v1" # a comment after the closing quote
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: trailing-model
  aliased-endpoint:
    baseUrl: http://127.0.0.1:4149/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    x-upstream: &u https://user:{alias_secret}@api.example.com/v1
    models:
    - id: aliased-model
      baseUrl: *u
  canonical-endpoint:
    baseUrl: http://127.0.0.1:4150/v1
    api: openai-responses
    apiKey: {LOOPBACK_SECRET}
    models:
    - id: canonical-model
""", encoding="utf-8")
            seeded, left_out, role = self.seed(home, host)
            self.assertEqual(seeded, ["canonical-endpoint"], f"left out: {left_out}")
            self.assertEqual(role, "canonical-endpoint/canonical-model")
            for name in ("block-scalar-endpoint", "flow-value-endpoint", "continued-quote-endpoint",
                         "undecodable-quoted-value", "trailing-content-endpoint", "aliased-endpoint"):
                self.assertTrue(any(reason.startswith(name) for reason in left_out),
                                f"{name} is not named among {left_out}")
            written = (home / "models.yml").read_text(encoding="utf-8")
            for value in (block_secret, flow_value_secret, continued_secret, undecodable_secret,
                          alias_secret):
                self.assertNotIn(value, written, "a credential value was written into a data directory")
            for name in ("block-scalar-endpoint", "flow-value-endpoint", "continued-quote-endpoint",
                         "undecodable-quoted-value", "trailing-content-endpoint", "aliased-endpoint"):
                self.assertNotIn(name, written, "a refused provider's definition was copied")
            self.assertIn(f"apiKey: {NATIVE_CHECK.SANDBOX_KEY}", written)


class DefaultRoleCases(unittest.TestCase):
    """The role `configured_default_role` reads out of one home's own `config.yml`.

    A caller's home is *read* to decide whether a real turn is possible, so a valid spelling this read
    misses would say the home names no model while the harness resolves one: the harness answers
    authoritatively (the report's own probe), but the reading must not be wrong about a spelling its own
    YAML writer produces. The field names are read semantically like the seeder's, so a quoted key or
    whitespace before a colon names the same field.
    """

    def role(self, text: str) -> str:
        with tempfile.TemporaryDirectory() as scratch:
            home = Path(scratch)
            (home / "config.yml").write_text(text, encoding="utf-8")
            return NATIVE_CHECK.configured_default_role(home)

    def test_the_block_mapping_is_read(self):
        self.assertEqual(self.role("modelRoles:\n  default: proxy/model\n"), "proxy/model")

    def test_the_flow_mapping_is_read(self):
        # The finding's own spelling: a home whose config.yml is one flow mapping resolves a model like
        # any other, so the read must answer it rather than fall back to "the home names none".
        self.assertEqual(self.role("modelRoles: {default: proxy/model}\n"), "proxy/model")

    def test_a_flow_mapping_beside_other_roles_and_quoting_is_read(self):
        self.assertEqual(
            self.role('modelRoles: { smol: other/small, "default": "proxy/model" }\n'),
            "proxy/model")

    def test_a_single_quoted_role_is_decoded_the_way_yaml_reads_it(self):
        # YAML's only escape inside a single-quoted scalar is a doubled quote, so `''` *is* one quote to
        # the harness's own reader; a read that kept the two characters would answer a role no home
        # defines. The double-quoted spelling beside it decodes JSON's escapes for the same reason.
        self.assertEqual(self.role("modelRoles:\n  default: 'proxy/model''s'\n"), "proxy/model's")
        self.assertEqual(self.role('modelRoles:\n  default: "proxy/model\\u0027s"\n'), "proxy/model's")

    def test_quoted_and_spaced_keys_name_the_same_fields(self):
        self.assertEqual(self.role('"modelRoles" :\n  "default" : proxy/model\n'), 'proxy/model')

    def test_a_spelling_the_reader_cannot_follow_answers_nothing(self):
        # Never a guess: an answer this read cannot make is "", and the harness's own get_state answer,
        # which the conversion probes and reports, is what decides whether a turn can run.
        self.assertEqual(self.role("modelRoles: [a, b]\n"), "")
        self.assertEqual(self.role("modelRoles: {smol: other/small}\n"), "")
        self.assertEqual(self.role('modelRoles: {default: "proxy/model}\n'), "")


class RequestedTurnCases(unittest.TestCase):
    """Whether a requested real turn can run, and what a run records when it cannot.

    `--resume-prompt` is a request for one real turn through the harness: a run that answers it with a
    verdict must have attempted the turn, and a turn that could not be attempted is a failed check, never
    a pass. The model that decides it comes from three answers, and the harness's own get_state answer --
    read out of the conversion's report, where the probe's detail names it -- outranks this run's textual
    reading of a caller's home.
    """

    def test_the_harnesss_own_answer_outranks_an_unreadable_configuration(self):
        target = {"model_provider": "proxy", "model": "model-one"}
        self.assertEqual(NATIVE_CHECK.resolved_report_model(target), "proxy/model-one")
        # The reader answered "" for this home (a spelling it could not follow): the turn must still run.
        self.assertEqual(NATIVE_CHECK.requested_turn_model("", "", target), "proxy/model-one")

    def test_a_half_answer_is_no_answer(self):
        self.assertEqual(NATIVE_CHECK.resolved_report_model({"model_provider": "proxy"}), "")
        self.assertEqual(NATIVE_CHECK.resolved_report_model({"model": "model-one"}), "")
        self.assertEqual(NATIVE_CHECK.requested_turn_model("", "", {}), "")

    def test_the_seeded_and_read_roles_are_answers_too(self):
        self.assertEqual(NATIVE_CHECK.requested_turn_model("proxy/model-one", "", {}), "proxy/model-one")
        self.assertEqual(NATIVE_CHECK.requested_turn_model("", "proxy/model-two", {}), "proxy/model-two")

    def test_an_unattempted_turn_is_a_failed_check_not_a_pass(self):
        saved = list(NATIVE_CHECK.RESULTS)
        self.addCleanup(NATIVE_CHECK.RESULTS.__setitem__, slice(None), saved)
        NATIVE_CHECK.RESULTS.clear()
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            NATIVE_CHECK.record_unavailable_requested_turn("EXPECTED")
        self.assertEqual([state for _, state, _ in NATIVE_CHECK.RESULTS], ["FAIL"],
                         "an unattempted requested turn must not be recorded as a skip a verdict ignores")
        self.assertIn("not attempted", output.getvalue())
        self.assertIn("--expect 'EXPECTED' was left unchecked", output.getvalue())
        verdict_output = io.StringIO()
        with contextlib.redirect_stdout(verdict_output):
            code = NATIVE_CHECK.verdict("")
        self.assertEqual(code, 1, "a run with an unattempted requested turn must not exit 0")
        self.assertIn("RESULT: FAIL", verdict_output.getvalue())


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
        # Both assignments are quoted: with only the home one quoted the line also names no sessions
        # tree, and the case could not tell the spelling rule from the missing word. This line carries
        # every word of the right invocation, so the spelling is the only thing that refuses it.
        quoted = (f"cd '{self.WORKSPACE}' && '{NATIVE_CHECK.OMP_HOME_ENV}={home}' "
                  f"'{NATIVE_CHECK.OMP_SESSION_DIR_ENV}={tree}' omp --resume {self.SESSION}")
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
