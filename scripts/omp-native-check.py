#!/usr/bin/env python3
"""Verify the Oh My Pi route natively, both directions, against the installed OMP binary.

This is the target-side native verification for goal 0006 (OMP continuation), and it is deliberately
**not** a gate step, exactly like ``scripts/claude-native-check.py``, ``scripts/codex-native-check.py``
and ``scripts/pi-native-check.py``: it needs the OMP binary installed and a data directory for the target
that resolves a model -- the harness's own answer is the binding the conversion writes into the session,
and the load check needs that binding as much as a real turn does, so a host whose data directory names no
model this run can use ends its run with those checks failed, never passed. The gate keeps deciding the
offline half (the inherited reader's round trip, the install rules, the reported losses and the CLI
shape).

What it does, for the direction it is asked about:

1. copies the source fixture into the run's own scratch tree and rewrites **only** the working directory
   its own records carry to a directory inside that tree, so the session it converts belongs to a path
   this run owns and the target's own lookup can find what it installs;
2. makes the OMP data directory the conversion installs into: the run's own scratch one (seeded from the
   host's own OMP home, see below) or the caller's ``--target-home``, which this run seeds nothing into
   (the conversion installs the session there, and a requested turn grows it);
3. plants an unrelated session in the scratch home it made, and digests every session the home holds;
4. converts through the CLI (``ccc continue … --json``) and reads the conversion, the installation and the
   structural read-back out of the report rather than trusting them, then checks the installed file's own
   bytes (the OMP project-directory spelling the run computes, the 256-byte title slot line, the
   non-empty ``model``/``provider`` pair OMP's request builder requires on every assistant record);
5. runs OMP's **own loader** offline (``omp --mode rpc --no-tools --resume <id>`` with a
   ``get_state``/``get_messages`` request on stdin) against a **copy** of the converted bytes in a
   throw-away data directory that carries this run's own seeded configuration (or, when the caller named
   the home, the smallest credential-free home the file's own records name), with an unknown-id control
   arm, and compares what the harness answered with what the installed file's own bytes say;
6. with ``--resume-prompt``, runs one real turn in the installed session and prints the harness's own
   answer under ``native_check_common.TURN_OUTPUT_MARKER``: for the OMP target this run drives the
   **printed** invocation's own words with the report's ``resume_environment`` itself, while for the
   Claude Code target the conversion CLI's own resume check (``--verify resume``) runs the turn the
   printed exec form spells and this run reads its state and the answer out of the report -- a requested
   turn that does not verify fails the run either way. Whether the OMP turn can run at all is decided by
   the model the run's own data directory selects *and* by the harness's own ``get_state`` answer the
   conversion probed, so a caller's home whose configuration this run's textual reader cannot follow
   still drives a turn; a requested turn the run cannot attempt is recorded as a failed check, never a
   pass and never an unremarked skip;
7. re-digests the source tree, the planted session and every session the home held, so "nothing else
   moved" is a measurement rather than a claim.

The OMP data directory this run makes for itself is seeded from the host's own OMP home
(``$PI_CODING_AGENT_DIR``, else ``~/.omp/agent``) as a **source of provider definitions only**. Every
provider whose ``baseUrl`` is loopback (``localhost``, ``127.0.0.1``, ``::1``) is copied with its ``api``,
its model entries and its other non-credential fields, with the value of its ``apiKey`` replaced by a
placeholder this run invents and any ``headers`` it carries dropped -- each of those read the way YAML
itself spells a key rather than by a literal prefix, so a quoted key (``"apiKey"``) and a key with
whitespace before its colon (``headers :``) cannot carry a value past the replacement, and a key at
another casing is rewritten or dropped too (the conservative read: a value under a spelling some reader
might fold to the field must not pass unscrubbed); a provider whose endpoint is not loopback, or any of
whose endpoint-bearing lines carries a credential -- its own ``baseUrl``, or one a model entry writes,
which is a level OMP's own writer uses (``_first_field``) -- is left out whole, because OMP demands a
literal key and this run writes no credential into a file (measured: without a key it refuses to start at
all, and ``$VAR``/``${VAR}`` in ``apiKey`` is **not** expanded). A definition holding a form this run's
reader cannot decode is left out whole too, with that reason in the run's own output, rather than copied
unread: YAML's explicit ``? key``, a quoted key that does not close on its own line or whose escapes
cannot be unquoted, an entry or a kept field's value spelled in YAML's flow form (``- {id: …}``,
``models: [{…}]``), a value under a tag, anchor or alias, a block scalar, a nested sequence or an empty
key, a quoted value whose escapes cannot be decoded, a plain scalar continued on a later line (or a
sequence item's own scalar so continued -- a multi-line value this reader does not fold), a value a field
holds on a line of its own, and a line that is neither an entry nor a sequence item; the last three are
lines the copy cannot show a field owning, which is where an endpoint URL or a credential would ride
unscrubbed. A quoted value's escapes are decoded before any check reads it, so
``"…/v1\\u003fapi_key=…"`` carries the ``?`` the credential check sees, while a value the reader cannot
decode is refused rather than guessed at.
``modelRoles.default`` is ``--model`` when the host's own home defines its provider (or
``--provider-base-url`` defined it), else that home's own configured default when its provider survived,
else the first surviving provider's first model, else nothing (and the report says so, because a home
with no model role answers no model and the conversion then writes no binding -- OMP's own measured
behaviour, reported as a named check rather than a crash). Nothing in a credential store is read, copied
or written: a home the caller names with ``--target-home`` is not seeded and nothing is planted in it --
the converted session the conversion installs there is the only file this run leaves (and a requested
turn grows it) -- and its own configuration is read, never replaced.

Measured on this host against OMP 18.18.7 (``omp --version`` prints ``omp/18.18.7``): the data directory
comes from ``PI_CODING_AGENT_DIR`` (the same name Pi uses) and sessions live at
``<home>/sessions/<project spelling>/<timestamp>_<id>.jsonl``; ``PI_CODING_AGENT_SESSION_DIR`` names the
directory holding session files *directly*, so the project directory resolves a session and the
``sessions`` root above it answers "not found"; the loader *writes* into the session it opens (it appends
a ``custom``/``session_exit`` record), so the check drives a copy; an unknown id is answered
``Error: Session "<id>" not found.``; and a real turn appends to the session while its pre-existing bytes
stay byte-identical.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from urllib.parse import urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parent))

from native_check_common import (  # noqa: E402
    CONTINUE_TERMINATOR,
    PROMPT_PLACEHOLDER,
    RESULTS,
    bounded,
    credential_free_url,
    home_holds_id,
    installed_counts,
    jsonl_parse_error,
    print_answer,
    record,
    report,
    reported_invocation,
    rewrite_source_cwd,
    rpc_answer,
    run,
    session_records,
    session_tree,
    sha256,
    tree_digest,
)

#: Where this repository keeps its committed native-shaped fixtures. A Claude transcript and an OMP
#: session are each one file inside a provider directory; the OMP fixture sits one project-directory
#: level deeper, exactly as the harness files its own sessions.
FIXTURE_ROOT = Path("internal/ccc/testdata/fixtures")
SOURCE_PROVIDER = "claude"
SOURCE_SESSION = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"
OMP_TARGET = "omp"

#: The opposite direction's source: the committed OMP fixture, read as a *source* and converted into a
#: previously supported target harness through the same pipeline every other source uses.
OMP_BACK_SOURCE_PROVIDER = "omp"
OMP_BACK_SOURCE_SESSION = "01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4"
OMP_BACK_TARGET = "claude"

GO_TAGS = "fts5"

#: The binary this harness builds and drives. It lives under `bin/`, which `.gitignore` covers, so a run
#: leaves the repository as it found it; the name is its own, not a sibling harness's, so two harnesses
#: run side by side cannot overwrite each other's build mid-run.
BINARY = Path("bin", "ccc-native-check-omp")

#: The environment variables that name a source root for the conversion's reader, per provider, and the
#: variables each harness reads its data directory from. OMP reads the *same* data-directory name Pi does
#: (measured on 18.18.7), and neither harness has a flag for it, so it is set as an environment value.
SOURCE_ROOT_ENV = {"claude": "CLAUDE_PROJECTS_DIR", "omp": "OMP_DIR"}
OMP_HOME_ENV = "PI_CODING_AGENT_DIR"
OMP_SESSION_DIR_ENV = "PI_CODING_AGENT_SESSION_DIR"
CLAUDE_HOME_ENV = "CLAUDE_CONFIG_DIR"

OMP_SESSIONS_SUBDIR = "sessions"
CLAUDE_PROJECTS_SUBDIR = "projects"

#: The width of OMP's rewritable title slot line, line feed included (the harness's own
#: `SESSION_TITLE_SLOT_BYTES = 256`, and every one of 300 sampled first lines of a real OMP home is
#: exactly that long).
TITLE_SLOT_BYTES = 256

#: The placeholder this run writes in place of a provider's own `apiKey` value: a literal string it
#: invents, never a credential read from anywhere. OMP refuses to start without one.
SANDBOX_KEY = "ccc-native-check"

#: The hosts a seeded provider's endpoint may name: OMP demands a credential for a remote endpoint, and
#: this run writes none, so only a loopback endpoint can be exercised here.
LOOPBACK_HOSTS = ("localhost", "127.0.0.1", "::1")

#: The two credential-bearing names a seeded provider definition is rewritten for: `SEEDED_KEY_FIELD`'s
#: *value* is replaced by the placeholder this run invents (OMP refuses to run without the field, and
#: `$VAR`/`${VAR}` in it is not expanded -- measured), and every name in `SEEDED_DROPPED_FIELDS` is
#: dropped together with its whole sub-block. A provider whose own endpoint URL carries a credential is
#: left out whole instead -- the policy `scripts/pi-native-check.py` applies to `baseUrl`.
SEEDED_KEY_FIELD = "apiKey"
SEEDED_DROPPED_FIELDS = ("headers",)

#: The same names in the form a key is compared in. A key at another casing (`ApiKey`, `HEADERS`) is
#: rewritten or dropped too, and that is deliberate rather than a claim about the harness's own reader:
#: what a reader that folded a field's case would read as the field must not pass unscrubbed, and dropping
#: or replacing a value the harness would have ignored costs nothing.
_SEEDED_KEY_FIELD_LOWER = SEEDED_KEY_FIELD.lower()
_SEEDED_DROPPED_FIELDS = tuple(field.lower() for field in SEEDED_DROPPED_FIELDS)

#: The names the loader arm takes out of the environment before it starts the harness: the variables that
#: can point a run at a model or carry model credentials. They mirror `internal/ccc/omp_verify.go`'s
#: `ompLoadEnvDenied`, and removing them is what makes "the load made no model turn" a measurement on a
#: host that happens to hold a key.
OMP_LOAD_ENV_DENIED = (
    "PI_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_OAUTH_TOKEN", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
    "GOOGLE_API_KEY", "GEMINI_API_KEY", "XAI_API_KEY", "DEEPSEEK_API_KEY", "GROQ_API_KEY",
    "MISTRAL_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AZURE_OPENAI_API_KEY",
    "LITELLM_API_KEY", "LITELLM_BASE_URL",
)

#: The names the Claude Code loader arm takes out of the environment, mirroring
#: `internal/ccc/claude_verify.go`'s `claudeLoadEnvDenied`.
CLAUDE_LOAD_ENV_DENIED = (
    "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODEL",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
    "ANTHROPIC_SMALL_FAST_MODEL", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK",
    "CLAUDE_CODE_USE_VERTEX",
)

#: What OMP's own lookup answers when the id it was asked to resume is installed nowhere (measured), and
#: what Claude Code's loader answers for a session it does not have.
OMP_MISSING_SESSION_MARKER = "not found"
CLAUDE_CONTROL_ARM_TEXT = "No conversation found with session ID"

UNRELATED_PROJECT = "ccc-native-check-unrelated"
UNRELATED_SESSION = "00000000-0000-4000-8000-0000000000ff"
UNRELATED_TIMESTAMP = "2026-01-01T00-00-00-000Z"

#: The workspace the rewritten source session belongs to, relative to the scratch root.
WORKSPACE = "workspace"

#: The fabricated sessions this run planted, so a run that stops early still removes exactly the files
#: it created -- and never one that was already there.
PLANTED: list[Path] = []


def omp_separator_spelling(path: str) -> str:
    """Every separator and drive colon of `path` replaced by `-`, as each branch of OMP's spelling does."""
    return path.replace("/", "-").replace("\\", "-").replace(":", "-")


def omp_within(base: str, path: str) -> str | None:
    """`path`'s spelling relative to `base`, or None when it is not inside it.

    It mirrors the Go function ``ompWithin`` (internal/ccc/omp_target.go): an empty relative path (the
    path *is* the base, which Go spells `.`) and a relative path that neither starts with `..` nor is
    absolute are inside; anything else is not, and two paths with no relative spelling at all (different
    volumes on Windows) are the same answer.
    """
    if not base.strip():
        return None
    try:
        relative = os.path.relpath(path, base)
    except ValueError:
        return None
    if relative in ("", "."):
        return ""
    if relative == ".." or relative.startswith(".." + os.sep) or relative.startswith("../"):
        return None
    if os.path.isabs(relative):
        return None
    return relative


def omp_join_spelling(prefix: str, relative: str) -> str:
    """The spelling's own prefix joined to a relative path the way the harness does (``ompJoinSpelling``)."""
    spelled = omp_separator_spelling(relative)
    if spelled == "":
        return prefix
    if prefix.endswith("-"):
        return prefix + spelled
    return prefix + "-" + spelled


def omp_project_dir(cwd, home=None, tmp=None) -> str:
    """The directory OMP files one working directory's sessions under.

    It mirrors the Go function ``OMPProjectDir`` (internal/ccc/omp_target.go), which is the harness's own
    three-branch rule: a working directory under the OS temporary directory spells `-tmp` + its path
    relative to it (and that branch wins when both apply); otherwise under the home directory `-` + its
    home-relative path; otherwise the whole absolute path with its leading separator removed, every
    separator spelled `-` and the result wrapped in `--`. This is *not* Pi's spelling in the first two
    branches: measured, a session written for a directory under `/tmp` is filed under `-tmp…`.
    """
    resolved = os.path.abspath(str(cwd))
    base_tmp = str(tmp) if tmp else tempfile.gettempdir()
    base_home = str(home) if home else str(Path.home())
    relative = omp_within(base_tmp, resolved)
    if relative is not None:
        return omp_join_spelling("-tmp", relative)
    relative = omp_within(base_home, resolved)
    if relative is not None:
        return omp_join_spelling("-", relative)
    return "--" + omp_separator_spelling(resolved.lstrip("/\\")) + "--"


def claude_project_dir(cwd) -> str:
    """The directory Claude Code files a session under for `cwd`: every character outside [A-Za-z0-9]
    replaced by `-`. It mirrors the Go function ``ClaudeProjectDir`` (internal/ccc/claude_target.go)."""
    return re.sub(r"[^A-Za-z0-9]", "-", str(cwd))


def host_omp_home() -> Path:
    """The host's own OMP data directory, read as a source of provider definitions and never written to."""
    configured = os.environ.get(OMP_HOME_ENV)
    if configured:
        return Path(configured)
    return Path.home() / ".omp" / "agent"


def yaml_scalar(value: str) -> str:
    """One value spelled for the check's own YAML as a double-quoted scalar -- JSON's string encoding,
    which is also a valid YAML scalar. Every value is quoted, not only the ones holding a character YAML
    acts on, because a *plain* scalar is typed by its own spelling: YAML reads ``123`` as a number,
    ``null`` as a null, ``yes`` as a boolean and ``2026-11-18`` as a timestamp, so a provider, a model or
    a ``provider/model`` role a home defines as that string would reach the harness as one of those
    instead of as the identity the session binds (these are the caller's own configuration answers, so a
    spelling this run did not anticipate is ordinary). It mirrors the Go function ``ompYAMLScalar``
    (internal/ccc/omp_verify.go), whose schema is the same small one."""
    return json.dumps(value)


def _scalar(text: str) -> str:
    """One YAML scalar's value, decoded the way the harness's own reader reads it, or a refusal.

    A plain scalar is its own text -- YAML gives a plain scalar no escapes, so the bytes are the value.
    A quoted scalar is decoded: a single-quoted one folds ``''`` to ``'``, and a double-quoted one is
    decoded with JSON's escape set -- a subset of YAML's own double-quoted escapes; an escape outside it
    is refused, never guessed -- so ``"…/v1\\u003fapi_key=…"`` carries the ``?`` any YAML reader sees.
    Reading a quoted value by removing its quotes is exactly how a credential-bearing endpoint would pass
    the ``baseUrl`` credential check: the raw value holds no literal query delimiter for
    `credential_free_url` to find while the harness reads one. A quoted value that does not close on its
    own line (a multi-line scalar, whose folded value this reader does not reconstruct) or that carries
    content after its closing quote raises `_UnseedableBlock`.

    A value that opens any other node form -- YAML's flow collections (``{…}``/``[…]``), a node property
    (a tag, an anchor or an alias), a block scalar (``|``/``>``) or a nested block mapping/sequence -- is
    a value this reader did not read: each spells fields of its own (a flow mapping's keys, a block
    scalar's content lines) that the credential policy has to see, so the definition is refused rather
    than copied unread.
    """
    value = text.strip()
    if not value:
        # YAML reads an empty value as null; there is nothing to decode or refuse.
        return value
    if value[:1] in "'\"":
        token = _QUOTED_SCALAR.match(value)
        if token is None:
            raise _UnseedableBlock(
                "it spells a quoted value that does not close on its own line, so this seeder's reader "
                "cannot decode it")
        if value[token.end():].strip():
            raise _UnseedableBlock(
                "it spells a quoted value with content after its closing quote, which this seeder's "
                "reader cannot follow")
        quoted = token.group(0)
        if quoted[0] == "'":
            return quoted[1:-1].replace("''", "'")
        try:
            return json.loads(quoted)
        except json.JSONDecodeError:
            raise _UnseedableBlock(
                "it spells a quoted value with escapes this seeder's reader cannot decode")
    if value[:1] in "{[&*!|>" or (value[:1] in "?-" and value[1:2] in ("", " ", "\t")):
        raise _UnseedableBlock(
            "it spells a value in a form (a flow collection, a tagged or anchored node, a block scalar "
            "or a nested block structure) this seeder's reader cannot follow")
    return value


def _require_readable_value(value: str) -> None:
    """Refuse one entry's or sequence item's value unless its form is one this seeder reads, by decoding
    it.

    Called for every value a block is copied with -- `_seed_block`'s fields other than the two names
    whose whole value it replaces or drops, and every sequence item's own text -- so the copy carries no
    value the seeder did not read. A flow collection's keys, a block scalar's content lines and an
    escaped quoted scalar are each a place a field -- an endpoint URL among them -- can be spelled
    without this reader seeing it; `_scalar` is the same reading the credential checks use, so a
    supported value costs one decode and an unsupported one is refused with the reason every unread form
    raises.
    """
    _scalar(value)


class _UnseedableBlock(Exception):
    """One provider definition holds a line this reader will not guess at, with the reason it names.

    Raised for a form YAML itself could read and this seeder's textual reader cannot: a key spelled in
    YAML's explicit ``?`` form, a quoted key that does not close on its own line or whose escapes this
    reader cannot unquote, an entry or value spelled in flow form (``- {id: …, baseUrl: …}``), a value
    that is a tagged or anchored node, a block scalar or a nested block structure, a quoted value whose
    escapes this reader cannot decode, a scalar a field or sequence item holds on a later line (a plain
    scalar continued, or one spelled under its own field's name), and a line that is neither an entry nor
    a sequence item and so cannot be attributed to a field at all. A definition carrying one is left out
    of the seed whole, with this reason in the run's own output, because copying a line the seeder did
    not read is exactly how a credential-bearing ``headers``/``apiKey``/``baseUrl`` spelling would reach
    the scratch home, the loader home and a ``--keep`` tree unscrubbed.
    """


#: One quoted scalar as YAML opens it: the double-quoted form (JSON's own escapes) or the single-quoted
#: one (where a doubled quote is the escaped quote). Used to read a quoted *key*; a token this pattern
#: cannot close is refused rather than read.
_QUOTED_SCALAR = re.compile(r"\"(?:[^\"\\]|\\.)*\"|'(?:[^']|'')*'")


def _mapping_entry(line: str) -> tuple[str, str, int, int] | None:
    """One line that opens a YAML mapping entry as ``(key, value, colon, key_column)``: the key's own
    text, the text after the colon that closes it, that colon's index in `line`, and the column the key
    itself starts at.

    The seeder's credential policy rests on reading a key the way YAML itself spells it rather than by a
    literal prefix: a key may be quoted (``"headers"``), whitespace may stand between it and its colon
    (``headers :``), and each of those spellings is the same field to any YAML reader. Entries are read at
    any depth and after a sequence marker, because ``headers`` and ``apiKey`` are fields a model entry may
    carry as well (Pi, the sibling harness whose schema OMP shares, documents header values at every
    level). A colon at the end of the line or before whitespace is what YAML itself ends a plain key at,
    so the read follows the same rule -- and a ``key:value`` with no space is no entry at all, exactly as
    YAML reads it (an implicit key of the whole scalar, not a field). Callers compare the returned key
    case-insensitively, which is the conservative direction: a value under a case-variant spelling must not
    pass unscrubbed.

    The key's column is what a plain scalar value's own continuation lines are indented deeper than
    (YAML: a continuation line must be more indented than the innermost enclosing block collection, whose
    indentation a block mapping's keys stand at), which is why `_seed_block` reads it: a line indented
    past it is a second line of the value, never a field.

    Returns None for a line that opens no entry -- a value or content line, a comment, a sequence of plain
    scalars -- and raises `_UnseedableBlock` for a line whose own start says it opens an entry in a form
    this reader cannot decode: YAML's explicit ``?`` key, a key spelled in YAML's flow form (``{id: …}``,
    which YAML reads as a whole mapping) or under a node property (a tag, an anchor or an alias), a nested
    sequence, or a quoted key that does not close or whose escapes cannot be decoded.
    """
    offset = len(line) - len(line.lstrip(" \t"))
    rest = line[offset:]
    if rest[:1] == "-" and rest[1:2] in ("", " ", "\t"):
        step = 1 if rest[1:2] == "" else 2
        offset += step
        rest = rest[step:]
        leading = len(rest) - len(rest.lstrip(" \t"))
        offset += leading
        rest = rest[leading:]
    if not rest or rest.startswith("#"):
        return None
    if rest[:1] == "?" and rest[1:2] in ("", " ", "\t"):
        raise _UnseedableBlock(
            "it spells a key with YAML's explicit `?` form, which this seeder's reader cannot follow")
    if rest[:1] in "{[":
        raise _UnseedableBlock(
            "it spells an entry in YAML's flow form, whose own keys this seeder's reader cannot follow")
    if rest[:1] in "&*!":
        raise _UnseedableBlock(
            "it spells a node with a tag, an anchor or an alias, which this seeder's reader cannot "
            "follow")
    if rest[:1] == "-" and rest[1:2] in ("", " ", "\t"):
        raise _UnseedableBlock(
            "it opens a nested sequence, whose own entries this seeder's reader cannot follow")
    if rest[:1] in "\"'":
        token = _QUOTED_SCALAR.match(rest)
        if token is None:
            raise _UnseedableBlock(
                "it opens a quoted key that does not close on its own line, so this seeder's reader "
                "cannot tell a key from a value")
        after = rest[token.end():]
        separator = re.match(r"[ \t]*:", after)
        if separator is None:
            # A quoted scalar that opens no entry: a quoted value or block-scalar content line.
            return None
        quoted = token.group(0)
        try:
            key = json.loads(quoted) if quoted[0] == '"' else quoted[1:-1].replace("''", "'")
        except json.JSONDecodeError:
            raise _UnseedableBlock(
                "it spells a quoted key with escapes this seeder's reader cannot decode")
        return key, after[separator.end():], offset + token.end() + separator.end() - 1, offset
    for index, character in enumerate(rest):
        if character == ":" and (index + 1 == len(rest) or rest[index + 1] in " \t"):
            key = rest[:index].strip()
            if not key:
                raise _UnseedableBlock(
                    "it opens a mapping entry with an empty key, which this seeder's reader cannot "
                    "name")
            return key, rest[index + 1:], offset + index, offset
    return None


def _sequence_item(line: str) -> str | None:
    """The text one line's block-sequence item carries after its own ``- `` marker, or None when the
    line opens no item.

    ``- `` is what YAML opens a block-sequence entry with, and nothing else is: ``-text`` is one plain
    scalar, and a line whose marker is followed by a mapping entry (``- id: demo``) is read as that
    entry instead (`_mapping_entry` takes the marker into account). That marker's own column is what an
    item's plain scalar continues past -- YAML indents the continuation lines of a sequence item's
    scalar deeper than the sequence itself, and the sequence's indentation is its markers' -- so the
    caller sets its continuation threshold from this line's indentation.
    """
    offset = len(line) - len(line.lstrip(" \t"))
    rest = line[offset:]
    if not rest.startswith("-") or rest[1:2] not in ("", " ", "\t"):
        return None
    return rest[1:].strip()


def provider_blocks(text: str) -> dict[str, list[str]]:
    """The provider definitions inside one OMP ``models.yml``, keyed by name, as their own lines.

    The document is read **textually** and each provider's block is kept verbatim: Pi's and OMP's
    ``models.yml`` schemas are nested (a model entry carries its own list members), and a run that
    re-serialised a definition it only partly understood would write a home the harness might refuse.
    The block boundary is YAML's own indentation rule -- a line at the provider-name indentation starts a
    provider, anything deeper belongs to it -- and the markdown-shaped list members under ``models:``
    (``- id: …``) stay inside their provider because they are indented deeper than its name.
    """
    lines = text.splitlines()
    start = None
    for index, line in enumerate(lines):
        if line.strip().rstrip(":") == "providers" and not line[:1].isspace():
            start = index + 1
            break
    if start is None:
        return {}
    blocks: dict[str, list[str]] = {}
    name_indent: int | None = None
    current: str | None = None
    for line in lines[start:]:
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            if current is not None:
                blocks[current].append(line)
            continue
        indent = len(line) - len(line.lstrip(" "))
        if name_indent is None:
            if indent == 0:
                break
            name_indent = indent
        if indent < name_indent:
            break
        if indent == name_indent:
            if not stripped.endswith(":"):
                current = None
                continue
            try:
                current = _scalar(stripped[:-1])
            except _UnseedableBlock:
                # The name is a label for this dict; the block's own lines are what the caller reads
                # (and refuses), and its first line is re-read there. Grouping the block under the text
                # as written keeps one unreadable name from dropping every definition in the file.
                current = stripped[:-1]
            blocks[current] = [line]
        elif current is not None:
            blocks[current].append(line)
    return blocks


def _first_field(lines: list[str], key: str) -> str:
    """The value of the first `key` entry inside one provider block, or "".

    The *first* one is the provider's own: a definition writes its own fields before the model entries
    that may carry the same key (``baseUrl`` is one OMP writes at both levels). The key is read the way
    every other key this seeder reads is read (`_mapping_entry`): quoted, spaced or at another casing,
    it names the same field.
    """
    wanted = key.strip().lower()
    for line in lines:
        entry = _mapping_entry(line)
        if entry is not None and entry[0].strip().lower() == wanted:
            return _scalar(entry[1])
    return ""


def _model_ids(lines: list[str]) -> list[str]:
    """Every model id one provider block lists, in order, read from the entries that open with a
    sequence marker (`- id: …`, the shape OMP's own writer produces)."""
    ids: list[str] = []
    in_models = False
    models_indent = 0
    for line in lines:
        stripped = line.strip()
        if not stripped:
            continue
        indent = len(line) - len(line.lstrip(" "))
        entry = _mapping_entry(line)
        if not in_models:
            if (entry is not None and entry[0].strip().lower() == "models"
                    and not entry[1].strip()):
                in_models = True
                models_indent = indent
            continue
        if indent <= models_indent and not stripped.startswith("-"):
            break
        if (stripped.startswith("-") and entry is not None
                and entry[0].strip().lower() == "id"):
            ids.append(_scalar(entry[1]))
    return ids


def loopback_url(url: str) -> bool:
    """Whether one endpoint URL names a loopback host (and carries no userinfo, query or fragment)."""
    if not url or not credential_free_url(url):
        return False
    try:
        parts = urlsplit(url)
    except ValueError:
        return False
    return (parts.hostname or "").lower() in LOOPBACK_HOSTS


def block_has_credential_endpoint(lines: list[str]) -> bool:
    """Whether any endpoint-bearing line of one provider block carries a credential.

    The block is read textually like the rest of this seeder, and the walk is by field name at any depth
    rather than by a fixed set of paths: a model entry may write its own ``baseUrl`` (OMP's own writer
    does -- ``_first_field``), so a filter that looked only at the provider's own line would copy such a
    URL verbatim into the scratch home, the loader home and a ``--keep`` tree, and a credential hides in
    an endpoint URL as userinfo, a query or a fragment (``credential_free_url``). The field name is read
    the way every other key this seeder reads is read (`_mapping_entry`), so a quoted or spaced spelling
    is the same field here too. The reason this run records names the provider, never the line: the line
    is where the credential is.
    """
    for line in lines:
        entry = _mapping_entry(line)
        if entry is None:
            continue
        if entry[0].strip().lower() == "baseurl" and not credential_free_url(_scalar(entry[1])):
            return True
    return False


def _seed_block(lines: list[str], placeholder: str) -> list[str]:
    """One provider block with apiKey's *value* replaced by the placeholder and every `headers` entry and
    its sub-block dropped. The field itself is never removed: OMP refuses to start without a key.

    The *whole* value is replaced, not only the line it opens with: a value may be a block scalar
    (``apiKey: |-``) or any other multi-line scalar, whose own lines are indented deeper than the field
    and would otherwise be copied -- the credential among them -- into the scratch home, the loader home
    and a ``--keep`` tree. So the replacement also drops every line deeper than the field until the next
    line the block itself owns, exactly as a dropped ``headers`` sub-block is dropped.

    Every key is read through `_mapping_entry` -- the same reader `_first_field` and
    `block_has_credential_endpoint` use -- and compared case-insensitively, so a quoted key, a key spelled
    with whitespace before its colon and a key at another casing cannot carry a value past this
    replacement. A form that reader cannot decode raises `_UnseedableBlock`, which `seed_omp_home`
    turns into a named refusal: the definition is left out whole rather than copied with a key this run
    did not read. Every value of a field the block keeps is read and decoded too
    (`_require_readable_value`): a value whose form this reader cannot follow -- a flow collection, a
    tagged or anchored node, a block scalar -- may spell fields of its own, so it is refused before
    anything is copied rather than copied unread. The two names whose whole value is replaced or dropped
    above are not read that way: no part of either is copied.

    A scalar can also continue, or start, on a line of its own, and both spellings are values this
    line-at-a-time reader does not read as a field: a plain scalar's continuation lines are indented
    deeper than the key they belong to (or than a sequence item's own marker), and a field whose value
    begins on the next line stands under a name this reader saw but a value it did not. Neither line can
    be attributed to a field, so any line that is not a comment, an entry, or a sequence item whose own
    text this reader decodes is refused whole -- otherwise an endpoint or credential the YAML reader
    folds into ``baseUrl``/``apiKey`` would be copied into the scratch home, the loader home and a
    ``--keep`` tree unscrubbed. Blank and comment lines carry no field and are kept as they were.
    """
    kept: list[str] = []
    drop_deeper_than: int | None = None
    replaced = False
    insert_at: int | None = None
    #: The column past which a following line continues the last plain scalar this walk read: the key
    #: column of the entry that value belongs to, or the marker column of the sequence item. A line
    #: indented deeper than it is a second line of that value -- YAML folds the two into one -- and is
    #: refused rather than copied unread.
    scalar_above: int | None = None
    for line in lines:
        stripped = line.strip()
        indent = len(line) - len(line.lstrip(" "))
        if drop_deeper_than is not None:
            if not stripped or indent > drop_deeper_than:
                continue
            drop_deeper_than = None
        if not stripped or stripped.startswith("#"):
            kept.append(line)
            continue
        if scalar_above is not None:
            if indent > scalar_above:
                raise _UnseedableBlock(
                    "it continues a plain scalar on a later line -- a multi-line value this seeder's "
                    "reader does not reconstruct -- so the value is not one this reader can read")
            scalar_above = None
        entry = _mapping_entry(line)
        if entry is not None:
            key, value, colon, key_column = entry
            name = key.strip().lower()
            if name in _SEEDED_DROPPED_FIELDS:
                drop_deeper_than = indent
                continue
            if name == _SEEDED_KEY_FIELD_LOWER:
                kept.append(line[:colon + 1] + f" {placeholder}")
                replaced = True
                # Any line deeper than this field is a continuation of its value -- a block scalar's own
                # lines, a folded or quoted scalar continued, a plain scalar's continuation -- never a field
                # of the block, so dropping to the next line at this indent or shallower removes the original
                # value whole.
                drop_deeper_than = indent
                continue
            _require_readable_value(value)
            if value.strip():
                scalar_above = key_column
        else:
            item = _sequence_item(line)
            if item is None:
                raise _UnseedableBlock(
                    "it is a line this seeder's reader cannot attribute to a field: it opens no entry "
                    "and no sequence item, so a value it carries -- an endpoint URL among them -- would "
                    "be copied unread")
            if item:
                _require_readable_value(item)
                scalar_above = indent
        kept.append(line)
        if insert_at is None and entry is not None and entry[0].strip().lower() in ("baseurl", "api"):
            insert_at = len(kept)
    if not replaced:
        position = insert_at if insert_at is not None else 1
        kept.insert(min(position, len(kept)), " " * 4 + f"{SEEDED_KEY_FIELD}: {placeholder}")
    return kept


def _generated_block(name: str, base_url: str, model_id: str) -> list[str]:
    """A provider definition this run writes itself, in the shape the harness's own writer produces for
    a one-model provider: the endpoint, the api, the placeholder key, and one model entry. The provider's
    name and the model's id and name are spelled through `yaml_scalar` -- quoted scalars -- for the reason
    that function states: they are strings, and a YAML reader would type `123` or `null` by its own
    spelling and reach the harness as a number or a null rather than as the id the session binds."""
    return [
        f"  {yaml_scalar(name)}:",
        f"    baseUrl: {base_url}",
        "    api: openai-completions",
        f"    apiKey: {SANDBOX_KEY}",
        "    models:",
        f"    - id: {yaml_scalar(model_id)}",
        f"      name: {yaml_scalar(model_id)}",
        "      reasoning: false",
        "      input: [text]",
        "      contextWindow: 1000",
        "      maxTokens: 100",
    ]


#: The `default` entry of a flow-mapping spelling of `modelRoles`: an entry after the opening brace or a
#: comma, with the key quoted or bare. Its value is a scalar (a `provider/model` string), so the read
#: below stops at the first comma or closing brace rather than parsing the flow syntax whole.
_FLOW_DEFAULT = re.compile(r"(?:^|[,{]\s*)[\"']?default[\"']?\s*:")


def _flow_role(text: str) -> str:
    """The value of the `default` entry inside one flow-mapping spelling of `modelRoles`, or "".

    `modelRoles: {default: provider/model}` is a spelling OMP's own YAML reader accepts and this run's
    textual reader must read too: a home whose configuration is written that way resolves a model like
    any other, so a reader that answered "" for it would say the home names none. A spelling this read
    cannot follow answers "" -- never a guess -- and the harness's own `get_state` answer, which the
    conversion probes and reports, is what decides a turn then.
    """
    match = _FLOW_DEFAULT.search(text)
    if match is None:
        return ""
    rest = text[match.end():].lstrip()
    if not rest:
        return ""
    if rest[:1] in "\"'":
        token = _QUOTED_SCALAR.match(rest)
        return _scalar(token.group(0)) if token is not None else ""
    end = len(rest)
    for index, character in enumerate(rest):
        if character in ",}":
            end = index
            break
    return _scalar(rest[:end])


def configured_default_role(home: Path) -> str:
    """The `modelRoles.default` one OMP home's own `config.yml` names, or "".

    The read is textual and narrow on purpose: it answers only which model role a home selects, so a
    caller's home can be *read* to decide whether a real turn is possible without this run ever parsing
    (or copying) the rest of their configuration. Both spellings a home's own YAML writer produces are
    read -- the block mapping (``modelRoles:`` then a deeper ``default:``) and the flow mapping
    (``modelRoles: {default: provider/model}``) -- and the field names are read semantically like the
    seeder's (`_mapping_entry`), so a quoted key or whitespace before a colon names the same field. A
    spelling this reader cannot follow answers "" rather than a guess; the harness's own answer, which
    the conversion reports, is what decides the turn in that case.
    """
    config = home / "config.yml"
    if not config.is_file():
        return ""
    try:
        lines = config.read_text(encoding="utf-8").splitlines()
    except OSError:
        return ""
    in_roles = False
    roles_indent = 0
    try:
        for line in lines:
            stripped = line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            indent = len(line) - len(line.lstrip(" "))
            entry = _mapping_entry(line)
            if not in_roles:
                if entry is not None and entry[0].strip().lower() == "modelroles":
                    if entry[1].strip():
                        return _flow_role(entry[1])
                    in_roles = True
                    roles_indent = indent
                continue
            if indent <= roles_indent:
                break
            if entry is not None and entry[0].strip().lower() == "default":
                return _scalar(entry[1])
    except _UnseedableBlock:
        # A line this reader cannot follow -- an unreadable key or value form, a quoted value whose
        # escapes it cannot decode -- is answered as the home naming no role this read can vouch for,
        # never guessed at. The harness's own get_state answer is what decides the turn then.
        return ""
    return ""


def seed_omp_home(home: Path, host_home: Path, preferred_model: str, provider_base_url: str):
    """Seed one scratch OMP home from the host's own, and answer what it holds.

    Answers the seeded provider names, the ones left out with their reason, and the model role the
    seeded home selects ("" when no provider survived). Every seeded definition is the host's own with
    one *value* replaced -- never a credential: a `headers` entry is dropped whole, an `apiKey`'s value
    is the placeholder this run invents, and a provider whose endpoint is not loopback, or any of whose
    endpoint-bearing lines carries a credential -- its own `baseUrl`, or one of its model entries', which
    is a level OMP writes (`_first_field`) -- is not copied at all, because OMP demands a literal key and
    this run writes none into a file. A definition holding a form this seeder's reader cannot decode
    (`_UnseedableBlock`) -- an unreadable key or value form, or a quoted value whose escapes it cannot
    decode -- is left out whole for the same reason: it is refused with the reason named, never copied
    with a line the seeder did not read.
    """
    seeded: dict[str, list[str]] = {}
    left_out: list[str] = []
    text = ""
    models_path = host_home / "models.yml"
    if models_path.is_file():
        try:
            text = models_path.read_text(encoding="utf-8")
        except OSError:
            text = ""
    for name, lines in provider_blocks(text).items():
        try:
            base_url = _first_field(lines, "baseUrl")
            if not base_url:
                left_out.append(f"{name} (it names no baseUrl)")
                continue
            if block_has_credential_endpoint(lines):
                # Every endpoint in the definition, not only the provider's own first line: a model entry's
                # `baseUrl` would otherwise be copied verbatim, because `_seed_block` rewrites key values and
                # drops headers, not URLs.
                left_out.append(f"{name} (an endpoint URL in its definition carries a credential)")
                continue
            if not loopback_url(base_url):
                left_out.append(f"{name} (its endpoint {base_url} is not loopback: this run writes no "
                                f"credential for a remote endpoint into a file)")
                continue
            seeded[name] = _seed_block(lines, SANDBOX_KEY)
        except _UnseedableBlock as refusal:
            # A line whose key this seeder's reader cannot decode leaves the whole definition out: copying
            # it would mean copying a key the seeder did not read, which is how a credential-bearing
            # spelling would pass unscrubbed. The reason is named in the run's own output.
            left_out.append(f"{name} ({refusal})")
            continue

    caller_provider = preferred_model.partition("/")[0]
    if provider_base_url and caller_provider:
        if not credential_free_url(provider_base_url):
            left_out.append(f"{caller_provider} (--provider-base-url carries a credential)")
        elif not loopback_url(provider_base_url):
            left_out.append(f"{caller_provider} (--provider-base-url {provider_base_url} is not "
                            f"loopback: this run writes no credential for a remote endpoint into a file)")
        elif caller_provider not in seeded:
            seeded[caller_provider] = _generated_block(
                caller_provider, provider_base_url, preferred_model.partition("/")[2] or "ccc-model")

    if seeded:
        body = "\n".join("\n".join(lines) for lines in seeded.values())
        (home / "models.yml").write_text("providers:\n" + body + "\n", encoding="utf-8")

    candidates: list[str] = []
    if caller_provider and caller_provider in seeded and preferred_model:
        candidates.append(preferred_model)
    host_role = configured_default_role(host_home)
    if host_role and host_role.partition("/")[0] in seeded:
        candidates.append(host_role)
    for name, lines in seeded.items():
        ids = _model_ids(lines)
        if ids:
            candidates.append(f"{name}/{ids[0]}")
    default_role = candidates[0] if candidates else ""
    if default_role:
        (home / "config.yml").write_text(
            f"modelRoles:\n  default: {yaml_scalar(default_role)}\n", encoding="utf-8")
    else:
        (home / "config.yml").write_text("", encoding="utf-8")
    return sorted(seeded), left_out, default_role


# --- reading a written session file, from its own bytes ---------------------------------------------


def omp_model_visible_roles(text: str) -> list[str]:
    """The roles OMP's own loader builds from one session file, in order: the latest compaction entry
    **on the path the loader walks** (reported by the harness as ``compactionSummary``), then every
    message entry after it on that path, and nothing off the path.

    It mirrors the Go function ``piModelVisibleRoles`` (internal/ccc/pi_verify.go) with OMP's own first
    line skipped: the walk starts at the file's last entry, follows ``parentId`` back to the tree's root
    and rebuilds the reversed path, so an entry the file holds but no later record hangs off is recorded
    history and not context. The title slot line (which carries no id) and the header are not entries of
    that tree.
    """
    records = [entry for entry in session_records(text) if entry.get("type") not in ("", "session", "title")]
    if not records:
        return []
    index_by_id = {entry.get("id"): index for index, entry in enumerate(records) if entry.get("id")}
    reversed_path: list[dict] = []
    visited: set[int] = set()
    index = len(records) - 1
    while index not in visited:
        visited.add(index)
        entry = records[index]
        reversed_path.append(entry)
        parent = entry.get("parentId")
        if not isinstance(parent, str) or parent not in index_by_id:
            break
        index = index_by_id[parent]
    path = list(reversed(reversed_path))
    boundary = -1
    for position, entry in enumerate(path):
        if entry.get("type") == "compaction":
            boundary = position
    roles: list[str] = []
    if boundary >= 0:
        roles.append("compactionSummary")
    for entry in path[boundary + 1:]:
        if entry.get("type") != "message":
            continue
        message = entry.get("message")
        roles.append(message.get("role", "") if isinstance(message, dict) else "")
    return roles


def assistant_records_without_pair(session: Path) -> tuple[int, int]:
    """How many assistant message records one file holds, and how many of them lack a non-empty `model`
    or `provider`. Measured: OMP's request builder indexes both on every assistant record, so one
    without either makes the session's next turn fail while it builds the request."""
    assistants = 0
    without = 0
    for entry in session_records(session.read_text(encoding="utf-8")):
        if entry.get("type") != "message":
            continue
        message = entry.get("message")
        if not isinstance(message, dict) or message.get("role") != "assistant":
            continue
        assistants += 1
        if not str(message.get("model") or "").strip() or not str(message.get("provider") or "").strip():
            without += 1
    return assistants, without


def title_slot_line(session: Path) -> tuple[int, bool]:
    """The first line of one session file as (its byte length including the line feed, whether it is a
    `title` record), measured on the file itself."""
    raw = session.read_bytes()
    first = raw.split(b"\n", 1)[0]
    try:
        entry = json.loads(first)
    except json.JSONDecodeError:
        return len(first) + 1, False
    return len(first) + 1, isinstance(entry, dict) and entry.get("type") == "title"


def session_binding(text: str) -> tuple[str, str]:
    """The provider and model the converted file's own records name: the session's `model_change`
    binding when it holds one, else the pair the last assistant record carries (the order OMP reads a
    session's model in, mirroring `ompSessionBinding`)."""
    provider, model = "", ""
    for entry in session_records(text):
        if entry.get("type") == "model_change":
            recorded = str(entry.get("model") or "").strip()
            slash = recorded.find("/")
            if 0 < slash < len(recorded) - 1:
                return recorded[:slash], recorded[slash + 1:]
            continue
        if entry.get("type") != "message":
            continue
        message = entry.get("message")
        if not isinstance(message, dict) or message.get("role") != "assistant":
            continue
        if str(message.get("provider") or "").strip() and str(message.get("model") or "").strip():
            provider = str(message["provider"]).strip()
            model = str(message["model"]).strip()
    return provider, model


# --- the two harnesses' own loaders -----------------------------------------------------------------


def omp_rpc_arguments(session_id: str) -> list[str]:
    """The offline RPC invocation OMP's own loader is asked through, mirroring
    `internal/ccc/omp_verify.go`'s `ompAskLoader`."""
    return ["--mode", "rpc", "--no-tools", "--resume", session_id]


def omp_rpc_environment(home, session_dir) -> dict[str, str]:
    """The environment `ompAskLoader` runs its own arm with: the data directory, the directory inside it
    that holds this session's file directly, and the model-reaching names removed."""
    environment = {name: value for name, value in os.environ.items()
                   if name not in OMP_LOAD_ENV_DENIED}
    environment[OMP_HOME_ENV] = str(home)
    environment[OMP_SESSION_DIR_ENV] = str(session_dir)
    return environment


def ask_omp_loader(omp: str, home, session_dir, cwd, session_id: str,
                   timeout: int = 120) -> subprocess.CompletedProcess:
    """Ask OMP's own loader for one session's state and messages, offline, with no model turn."""
    commands = "\n".join([
        json.dumps({"id": "state", "type": "get_state"}),
        json.dumps({"id": "messages", "type": "get_messages"}),
    ]) + "\n"
    return subprocess.run(
        [omp, *omp_rpc_arguments(session_id)], input=commands, cwd=str(cwd),
        env=omp_rpc_environment(home, session_dir), capture_output=True, text=True, timeout=timeout)


def last_json_line(output: str) -> dict | None:
    """The last line of one harness run's stdout that is one JSON object, or None."""
    for line in reversed((output or "").splitlines()):
        stripped = line.strip()
        if not stripped.startswith("{"):
            continue
        try:
            parsed = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            return parsed
    return None


def ask_claude_loader(claude: str, home, cwd, session_id: str,
                      timeout: int = 90) -> subprocess.CompletedProcess:
    """Ask Claude Code's own loader about one session, offline: `--bare` is what keeps the run from
    reading a credential store, and the model-reaching names are removed for the same reason
    `internal/ccc/claude_verify.go` removes them."""
    environment = {name: value for name, value in os.environ.items()
                   if name not in CLAUDE_LOAD_ENV_DENIED}
    environment[CLAUDE_HOME_ENV] = str(home)
    return subprocess.run(
        [claude, "--bare", "-p", "--output-format", "json", "--resume", session_id,
         CONTINUE_TERMINATOR, "load check"],
        cwd=str(cwd), env=environment, capture_output=True, text=True, timeout=timeout)


# --- reading a printed invocation -------------------------------------------------------------------


def omp_session_tree(home, project_dir: str) -> str:
    """The directory one OMP session is filed in and looked up through: the project directory below the
    data directory's own `sessions` root. It mirrors the Go function ``ompSessionTree``
    (internal/ccc/omp_verify.go), which joins those three names with ``filepath.Join``; the join itself
    is `session_tree`."""
    return session_tree(home, OMP_SESSIONS_SUBDIR, project_dir)


# --- reporting --------------------------------------------------------------------------------------


def _report(args: argparse.Namespace, scratch: Path, document: dict) -> None:
    """OMP's own `report` payload adds the installed session's path between `target_session` and
    `conversion`; the body it shares with the sibling harnesses is `native_check_common.report`."""
    target = document.get("target_session") or {}
    report(args, scratch, document, {"installed": target.get("path", "")})


def verdict(native_missing: str) -> int:
    """The `RESULT:` line every sibling harness ends with, and the exit code that matches it."""
    failures = [name for name, state, _ in RESULTS if state == "FAIL"]
    if failures:
        print(f"RESULT: FAIL — {len(failures)} check(s) failed: {', '.join(failures)}")
        return 1
    if native_missing:
        print(f"RESULT: INCOMPLETE — {len(RESULTS)} check(s) ran, none failed; {native_missing}")
        return 2
    print(f"RESULT: PASS — {len(RESULTS)} check(s), none failed")
    return 0


def finish(args: argparse.Namespace, scratch: Path, document: dict, native_missing: str = "") -> int:
    """The end of every path through this harness: say what became of the scratch tree, print the
    machine-readable summary when it was asked for, and print the one `RESULT:` line the checks earned
    -- including the paths that stop early, so no run ends without a verdict."""
    _report(args, scratch, document)
    return verdict(native_missing)


# --- the two directions -----------------------------------------------------------------------------


def _build_ccc(args: argparse.Namespace, repo: Path, scratch: Path) -> int | None:
    """Build the CCC binary this direction drives, recording the one named check both directions share.

    Returns the exit code to finish with when the build failed, or None when it built.
    """
    build_parent = (repo / BINARY).parent
    build_parent.mkdir(parents=True, exist_ok=True)
    build = run([shutil.which("go") or "go", "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY),
                 "./cmd/ccc"], cwd=repo, env={"CGO_ENABLED": "1"})
    if build.returncode != 0:
        record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
        return finish(args, scratch, {})
    record("ccc builds with the flags upstream ships", True)
    return None


def _plant_unrelated(args: argparse.Namespace, scratch: Path, unrelated: Path, body: str,
                     named_home: bool) -> int | None:
    """Plant this run's fabricated unrelated session, unless the caller named the home (which this run
    seeds nothing into, so the fabricated session goes to the scratch home alone). Returns the exit code
    to finish with when the file was already there, else None.
    """
    if named_home:
        return None
    if unrelated.exists():
        print(f"{unrelated} already exists: this harness plants its own fabricated session under that "
              f"name and will not overwrite one that is there", file=sys.stderr)
        return finish(args, scratch, {},
                      f"the fabricated session this run plants at {unrelated} was already there, and "
                      f"this harness never overwrites one that is")
    unrelated.parent.mkdir(parents=True, exist_ok=True)
    unrelated.write_text(body, encoding="utf-8")
    PLANTED.append(unrelated)
    return None


def _convert_and_read_report(args: argparse.Namespace, repo: Path, scratch: Path, home: Path,
                             command: list[str], env: dict[str, str]):
    """Run the conversion CLI and read the checks both directions share out of its result document: the
    document parses, the conversation it carried and every loss category, the structural read-back, the
    exit code, and the installed session's own existence.

    Returns either the exit code to finish with (a document that would not parse, or an install this
    check could not find) or the tuple `(document, convert, target, verification, session_id, installed)`.
    """
    convert = run(command, cwd=repo, env=env)
    try:
        document = json.loads(convert.stdout)
    except json.JSONDecodeError:
        record("ccc continue writes one JSON result document", False,
               bounded(convert.stdout or convert.stderr, 400))
        return finish(args, scratch, {})
    record("ccc continue writes one JSON result document", True)

    conversion = document.get("conversion") or {}
    installation = document.get("installation") or {}
    target = document.get("target_session") or {}
    verification = document.get("verification") or {}
    carried = conversion.get("carried") or {}
    losses = {loss.get("category"): loss.get("count") for loss in conversion.get("losses") or []}
    record("the conversion reports the conversation it carried and every loss category",
           conversion.get("state") == "converted" and carried.get("messages", 0) > 0 and bool(losses),
           f"carried {carried.get('messages')} message(s), {carried.get('tool_calls')} call(s), "
           f"{carried.get('tool_results')} result(s); loss categories named: {len(losses)}")
    record("the structural read-back verified the installed session",
           (verification.get("structure") or {}).get("state") == "verified",
           bounded((verification.get("structure") or {}).get("detail", ""), 300))
    record("ccc continue exits 0 for a conversion whose verification ran", convert.returncode == 0,
           f"exit {convert.returncode}; stderr {bounded(convert.stderr, 300)}")

    session_id = target.get("session_id", "")
    installed = Path(target.get("path", ""))
    if not session_id or not installed.is_file():
        record("the installed session exists at the reported path", False,
               f"session_id={session_id!r} path={target.get('path')!r}")
        return finish(args, scratch, document)
    record("the installed session exists at the reported path", True,
           f"{installed.relative_to(home)} [{(installation or {}).get('mechanism')}]")
    return document, convert, target, verification, session_id, installed


def _record_unchanged_evidence(source_root: Path, source_root_before: dict[str, str], named_home: bool,
                               unrelated: Path, planted_before: str,
                               existing_sessions: dict[Path, str]) -> None:
    """Re-digest the source tree, the planted session and every session the home already held, so
    "nothing else moved" is a measurement rather than a claim."""
    source_root_after = tree_digest(source_root)
    record("the source fixture tree is unchanged", source_root_after == source_root_before,
           f"{len(source_root_before)} file(s) digested before and after")
    if named_home:
        record("the planted unrelated session is unchanged", False,
               "not planted: this run was given the caller's home with --target-home and seeds nothing "
               "into it (the converted session the conversion installs there is the file it leaves); "
               "every session that home holds is digested instead", skipped=True)
    else:
        record("the planted unrelated session is unchanged", sha256(unrelated) == planted_before)
    moved = [str(path) for path, digest in existing_sessions.items()
             if not path.is_file() or sha256(path) != digest]
    record("every session the home already held is unchanged", not moved, "; ".join(moved[:3]))


def resolved_report_model(target: dict) -> str:
    """The model role the conversion's own probe read out of the harness's own answer, or "".

    The conversion asks the target harness which model a new session of the home it installs into
    resolves (``ProbeOMPModel``) and reports the answer on ``target_session``. A home whose own
    configuration this run's textual reader cannot follow still answers there, and the harness's answer is
    what the printed resume invocation runs under; the route writes both fields together, so an answer is
    the pair.
    """
    provider = str(target.get("model_provider") or "").strip()
    model = str(target.get("model") or "").strip()
    return f"{provider}/{model}" if provider and model else ""


def requested_turn_model(default_role: str, caller_role: str, target: dict) -> str:
    """The model role a requested real turn can run under, or "" when there is none.

    Three answers are tried, nearest the harness the turn runs against first: the role this run's own
    seed wrote into the scratch home, the role its textual read found in a caller's home, and the
    harness's own ``get_state`` answer the conversion probed and reported. The last outranks a reading: a
    home whose ``config.yml`` spells its role in a way this run cannot follow is still a home the harness
    resolves a model in, and this run's empty answer is not evidence that the harness has none.
    """
    for candidate in (default_role, caller_role, resolved_report_model(target)):
        if candidate:
            return candidate
    return ""


def record_unavailable_requested_turn(expect: str) -> None:
    """The record a run makes when the turn the caller asked for could not be attempted.

    A requested turn that was not run is a failed check, never a pass and never an unremarked skip: the
    caller asked this run to verify one real turn through the harness, and a verdict that ignored the
    omission would report a conversion whose turn was never tried as if it had been.
    """
    record("a real turn answered from the installed session and only appended", False,
           "not attempted: this run's data directory selects no model role and the harness's own answer "
           "names no model either, so the requested turn has no model to run under"
           + (f"; --expect {expect!r} was left unchecked" if expect else ""))


def _claude_to_omp(args: argparse.Namespace, repo: Path, scratch: Path, workspace: Path,
                   source_root: Path, destination: Path, recorded_cwd: str, source: Path) -> int:
    """Convert a Claude transcript into OMP and verify the installed session with OMP's own loader."""
    omp = shutil.which(args.omp)
    assert omp is not None
    named_home = args.target_home is not None
    home = args.target_home.resolve() if args.target_home else scratch / "omp-home"
    home.mkdir(parents=True, exist_ok=True)

    seeded: list[str] = []
    left_out: list[str] = []
    default_role = ""
    caller_role = ""
    if named_home:
        caller_role = configured_default_role(home)
    else:
        seeded, left_out, default_role = seed_omp_home(home, host_omp_home(), args.model,
                                                       args.provider_base_url)
    spelling = omp_project_dir(str(workspace))

    unrelated = home / OMP_SESSIONS_SUBDIR / f"-{UNRELATED_PROJECT}" / \
        f"{UNRELATED_TIMESTAMP}_{UNRELATED_SESSION}.jsonl"
    planted = _plant_unrelated(args, scratch, unrelated, json.dumps({
        "type": "title", "v": 1, "title": "", "updatedAt": "2026-01-01T00:00:00.000Z", "pad": "",
    }) + "\n" + json.dumps({
        "type": "session", "version": 3, "id": UNRELATED_SESSION,
        "timestamp": "2026-01-01T00:00:00.000Z", "cwd": f"/tmp/{UNRELATED_PROJECT}",
        "records": "a session this run must not touch",
    }) + "\n", named_home)
    if planted is not None:
        return planted

    print(f"repo:        {repo}")
    print(f"direction:   {args.direction} ({SOURCE_PROVIDER}:{args.source or SOURCE_SESSION} → omp)")
    print(f"omp:         {omp} ({run([omp, '--version']).stdout.strip()[:40]})")
    print(f"source:      {source.relative_to(repo)} (copied to {destination})")
    print(f"workspace:   {workspace} -> OMP project dir {spelling}")
    print(f"target home: {home}")
    if named_home:
        print(f"omp models:  the caller's home is not seeded: this run reads its modelRoles.default as "
              f"{caller_role or '(none)'}, and the harness's own answer is reported below; the conversion "
              f"installs the session into it")
    else:
        print(f"omp models:  seeded provider(s) {', '.join(seeded) or '(none)'} from {host_omp_home()}"
              + (f"; left out: {'; '.join(left_out)}" if left_out else "")
              + f"; apiKey values replaced by {SANDBOX_KEY!r}, headers dropped, "
              + f"modelRoles.default {default_role or '(none written)'}")
    print(f"the source fixture's recorded working directory ({recorded_cwd or 'none recorded'}) is rewritten "
          f"to the workspace, so the session belongs to a path this run owns and the target's own lookup "
          f"can find what it installs")
    print()

    # The caller-home record is made after the conversion, where the harness's own answer is known: this
    # run's textual read of the home's configuration is one answer, and the harness's get_state answer --
    # which outranks it -- is the other. A record made before the conversion could only restate the
    # reading, and a reading that missed a valid spelling would then say (with a skip) that the home names
    # no model while the run's own turn runs under one. The seeded branch keeps its records here: what the
    # seed wrote is known before anything runs.
    if not named_home:
        host_home = host_omp_home()
        if seeded:
            seed_detail = (f"seeded {', '.join(seeded)}; modelRoles.default {default_role or '(none)'}; "
                           f"apiKey values replaced by {SANDBOX_KEY!r}")
        else:
            reasons = "; ".join(left_out) if left_out else f"{host_home / 'models.yml'} lists none"
            seed_detail = f"no provider survived the seed: {reasons}"
        record("an OMP provider definition was seeded into the run's own scratch home", bool(seeded),
               seed_detail, skipped=not seeded)
        record("the seeded home selects a model role a real turn can use", bool(default_role),
               f"modelRoles.default {default_role}" if default_role else
               f"no model role was written, so this home answers no model: a turn (--resume-prompt) has "
               f"nothing to run under",
               skipped=not default_role)

    source_root_before = tree_digest(source_root)
    planted_before = sha256(unrelated) if unrelated.is_file() else ""
    existing_sessions = {path: sha256(path) for path in home.rglob("*.jsonl") if path != unrelated}

    built = _build_ccc(args, repo, scratch)
    if built is not None:
        return built

    command = [str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{args.source or SOURCE_SESSION}",
               "--to", OMP_TARGET, "--target-home", str(home), "--target-binary", omp, "--json"]
    read = _convert_and_read_report(args, repo, scratch, home, command,
                                    {SOURCE_ROOT_ENV[SOURCE_PROVIDER]: str(source_root)})
    if isinstance(read, int):
        return read
    document, convert, target, verification, session_id, installed = read

    if named_home:
        resolved = resolved_report_model(target)
        answered = resolved or caller_role
        record("the caller's home names a model role its loader can start from", bool(answered),
               f"the harness's own get_state answer resolved {resolved}" if resolved else
               (f"this run reads modelRoles.default as {caller_role}" if caller_role else
                f"{home / 'config.yml'} names no modelRoles.default this run can read and the harness "
                f"answered no model for this home, so the conversion writes no binding (its own measured "
                f"behaviour)"),
               skipped=not answered)

    record("the installed session is filed under this run's own OMP spelling of the workspace",
           installed.parent == home / OMP_SESSIONS_SUBDIR / spelling,
           f"{installed.relative_to(home)}; this run's OMP spelling of {workspace} is {spelling}")
    counts = installed_counts(installed)
    record("the installed file is an OMP session", counts.get("session") == 1 and counts.get("message", 0) > 0,
           f"entries {dict(sorted(counts.items()))}")
    width, is_title = title_slot_line(installed)
    record(f"the installed file opens with a {TITLE_SLOT_BYTES}-byte title slot record",
           is_title and width == TITLE_SLOT_BYTES,
           f"first line {width} byte(s) including its line feed, type {'title' if is_title else 'other'}")
    assistants, without_pair = assistant_records_without_pair(installed)
    record("every assistant record carries a non-empty model and provider",
           assistants > 0 and without_pair == 0,
           f"{assistants} assistant message(s), {without_pair} without the pair")
    report_load = verification.get("load") or {}
    record("the load check verified the installed session natively",
           report_load.get("state") == "verified", bounded(report_load.get("detail", ""), 400))

    data = installed.read_text(encoding="utf-8")
    file_roles = omp_model_visible_roles(data)
    _check_omp_loader(omp, home, workspace, installed, session_id, file_roles, named_home)

    _check_omp_invocation(target, workspace, home, omp)

    if args.resume_prompt:
        if not requested_turn_model(default_role, caller_role, target):
            record_unavailable_requested_turn(args.expect)
        else:
            _run_omp_turn(args, target, workspace, installed)

    _record_unchanged_evidence(source_root, source_root_before, named_home, unrelated, planted_before,
                               existing_sessions)
    record("the conversion wrote no id the home already held",
           not home_holds_id(home, session_id, exclude=installed),
           f"{session_id} is new among the session id(s) the home's own header lines record")

    return finish(args, scratch, document)


def _check_omp_loader(omp: str, home: Path, workspace: Path, installed: Path, session_id: str,
                      file_roles: list[str], named_home: bool) -> None:
    """Drive OMP's own loader against a **copy** of the converted bytes and compare its answers with the
    file's own bytes.

    The copy lives in a throw-away data directory that carries this run's own seeded configuration (the
    provider definitions the conversion's binding was asked of, credential-free by construction). A home
    the caller named is never copied -- its configuration may hold literal keys or headers -- so when the
    copy carries no `models.yml` this check writes its own provider definition instead, in the shape
    `_generated_block` writes: the provider and model the file's own records name, with the placeholder
    `apiKey` value `SANDBOX_KEY` (`ccc-native-check`) and no `auth:` key anywhere. A file whose own
    records name no provider and model has no definition to build for it, and the harness's own refusal to
    start in a home that holds none -- the same absence the conversion's own load check reports -- is then
    this arm's answer: a failed check that names the absence, never a pass.
    """
    before = sha256(installed)
    loader_home = Path(tempfile.mkdtemp(prefix="ccc-omp-native-loader-"))
    try:
        for name in ("models.yml", "config.yml"):
            source = home / name
            if not named_home and source.is_file():
                shutil.copyfile(source, loader_home / name)
        if not (loader_home / "models.yml").is_file():
            provider, model = session_binding(installed.read_text(encoding="utf-8"))
            if provider and model:
                (loader_home / "models.yml").write_text(
                    "providers:\n" + "\n".join(_generated_block(provider, "https://example.invalid/v1",
                                                               model)) + "\n", encoding="utf-8")
                (loader_home / "config.yml").write_text(
                    f"modelRoles:\n  default: {yaml_scalar(f'{provider}/{model}')}\n", encoding="utf-8")
        copy = loader_home / OMP_SESSIONS_SUBDIR / installed.parent.name / installed.name
        copy.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(installed, copy)
        session_dir = loader_home / OMP_SESSIONS_SUBDIR / installed.parent.name
        try:
            completed = ask_omp_loader(omp, loader_home, session_dir, workspace, session_id)
        except subprocess.TimeoutExpired:
            record("the harness's own loader answers about the installed session", False,
                   "the loader did not answer inside its deadline")
            return
        state = rpc_answer(completed.stdout, "get_state")
        messages = rpc_answer(completed.stdout, "get_messages")
        if state is None or not state.get("success"):
            record("the harness's own loader answers about the installed session", False,
                   bounded(completed.stderr or completed.stdout, 300))
        else:
            data = state.get("data") or {}
            answered_file = str(data.get("sessionFile") or "")
            same_copy = bool(answered_file) and Path(answered_file).resolve() == copy.resolve()
            record("the harness's own loader answers about the installed session",
                   data.get("sessionId") == session_id
                   and data.get("messageCount") == len(file_roles) and same_copy,
                   f"sessionId {data.get('sessionId')!r}, messageCount {data.get('messageCount')} "
                   f"(the file's own history holds {len(file_roles)}), sessionFile "
                   f"{'the copy this check installed' if same_copy else answered_file!r}")
        roles = [(message or {}).get("role", "")
                 for message in ((messages or {}).get("data") or {}).get("messages", [])]
        record("the loader's own message list is the file's model-visible history",
               bool(roles) and roles == file_roles,
               f"loader {roles} vs file {file_roles}" if roles != file_roles
               else f"{len(roles)} message(s): {roles}")

        unknown = "01a1c338-dead-beef-0000-000000000000"
        try:
            control = ask_omp_loader(omp, loader_home, session_dir, workspace, unknown)
        except subprocess.TimeoutExpired:
            record("the unknown-id control arm reports no such session", False,
                   "the control arm did not answer inside its deadline")
        else:
            record("the unknown-id control arm reports no such session",
                   OMP_MISSING_SESSION_MARKER in (control.stderr or "") + (control.stdout or "")
                   and rpc_answer(control.stdout, "get_state") is None,
                   bounded(control.stderr or control.stdout, 200))
    finally:
        shutil.rmtree(loader_home, ignore_errors=True)
    record("the load ran against a copy and left the installed file alone", sha256(installed) == before)


def _run_omp_turn(args: argparse.Namespace, target: dict, workspace: Path, installed: Path) -> None:
    """Run one real turn through the printed invocation's own words, with the report's own environment."""
    words = list(target.get("exec_resume_arguments") or [])
    environment = os.environ.copy()
    environment.update(target.get("resume_environment") or {})
    if not words or words[-1] != PROMPT_PLACEHOLDER:
        record("a real turn answered from the installed session and only appended", False,
               f"the printed exec invocation does not end with the placeholder word: {words!r}")
        return
    words[-1] = args.resume_prompt
    before = installed.read_bytes()
    try:
        completed = subprocess.run(words, cwd=str(workspace), env=environment, capture_output=True,
                                   text=True, timeout=900)
    except subprocess.TimeoutExpired:
        record("a real turn answered from the installed session and only appended", False,
               "the turn did not answer inside its deadline")
        return
    after = installed.read_bytes()
    answer = (completed.stdout or "").strip() or (completed.stderr or "").strip()
    grew = len(after) > len(before)
    appended = after.startswith(before)
    record("a real turn answered from the installed session and only appended",
           completed.returncode == 0 and bool(answer) and grew and appended,
           f"exit {completed.returncode}, session {len(before)} -> {len(after)} bytes"
           + ("" if appended else "; the pre-existing bytes are not a prefix of the new file")
           + f"; ran {' '.join(words)}")
    print_answer(completed.stdout or completed.stderr or "")
    if args.expect:
        record(f"the turn's answer contains {args.expect!r}", args.expect in (completed.stdout or ""),
               bounded(completed.stdout, 160))


def _check_omp_invocation(target: dict, workspace, home, omp: str) -> None:
    """Check the printed resume commands by reading them the way a POSIX shell reads them: word by word,
    against the working directory and the data directory this run used, with nothing a shell would split
    or act on left bare."""
    binary = target.get("binary") or omp
    project_dir = Path(target.get("path", "")).parent.name
    session_id = target.get("session_id", "")
    for field, arguments_field, arguments in (
            ("resume_command", "resume_arguments", [binary, "--resume", session_id]),
            ("exec_resume_command", "exec_resume_arguments",
             [binary, "-p", "--resume", session_id, CONTINUE_TERMINATOR, PROMPT_PLACEHOLDER])):
        command = target.get(field, "")
        printed = target.get(arguments_field) or []
        session_dir = omp_session_tree(home, project_dir)
        expected_environment = {OMP_HOME_ENV: str(home), OMP_SESSION_DIR_ENV: session_dir}
        ok = (bool(command) and printed == arguments
              and (target.get("resume_environment") or {}) == expected_environment
              and reported_invocation(command, workspace, home, OMP_HOME_ENV, arguments,
                                      OMP_SESSION_DIR_ENV, session_dir))
        record(f"the printed {field} reads as a POSIX shell line naming the session", ok,
               bounded(command, 200))


def _omp_to_claude(args: argparse.Namespace, repo: Path, scratch: Path, workspace: Path,
                   source_root: Path, destination: Path, recorded_cwd: str, source: Path) -> int:
    """Convert an OMP session into a Claude Code transcript and verify it with Claude Code's own loader."""
    claude = shutil.which(args.claude)
    named_home = args.target_home is not None
    home = args.target_home.resolve() if args.target_home else scratch / "claude-home"
    home.mkdir(parents=True, exist_ok=True)
    spelling = claude_project_dir(str(workspace))

    unrelated = home / CLAUDE_PROJECTS_SUBDIR / f"-{UNRELATED_PROJECT}" / f"{UNRELATED_SESSION}.jsonl"
    planted = _plant_unrelated(args, scratch, unrelated, json.dumps({
        "type": "user", "uuid": UNRELATED_SESSION, "sessionId": UNRELATED_SESSION,
        "timestamp": "2026-01-01T00:00:00.000Z", "cwd": f"/tmp/{UNRELATED_PROJECT}",
        "message": {"role": "user", "content": "an unrelated session this run must not touch"},
    }) + "\n", named_home)
    if planted is not None:
        return planted

    print(f"repo:        {repo}")
    print(f"direction:   {args.direction} (omp:{args.source or OMP_BACK_SOURCE_SESSION} → claude)")
    print(f"claude:      {args.claude}" + (f" ({claude})" if claude else " (not on PATH)"))
    print(f"source:      {source.relative_to(repo)} (copied to {destination})")
    print(f"workspace:   {workspace} -> Claude project dir {spelling}")
    print(f"target home: {home}")
    print(f"the source fixture's recorded working directory ({recorded_cwd or 'none recorded'}) is rewritten "
          f"to the workspace, so the session belongs to a path this run owns and the target's own loader "
          f"can find what it installs")
    print()

    source_root_before = tree_digest(source_root)
    planted_before = sha256(unrelated) if unrelated.is_file() else ""
    existing_sessions = {path: sha256(path) for path in home.rglob("*.jsonl") if path != unrelated}
    existing_ids = [path.stem for path in home.rglob("*.jsonl")]

    built = _build_ccc(args, repo, scratch)
    if built is not None:
        return built

    command = [str(repo / BINARY), "continue", f"{OMP_BACK_SOURCE_PROVIDER}:"
               f"{args.source or OMP_BACK_SOURCE_SESSION}", "--to", OMP_BACK_TARGET,
               "--target-home", str(home), "--target-binary", args.claude, "--json"]
    if args.resume_prompt:
        # The flag is the request for one real turn, and this direction runs it at the level the route
        # has for it: the conversion CLI's own resume check (`--verify resume`), which runs the
        # invocation the printed exec form spells and reports the harness's answer and the session's
        # growth in the report read below. A turn that does not verify is a failed check, never a pass.
        command += ["--verify", "resume", "--resume-prompt", args.resume_prompt]
    read = _convert_and_read_report(args, repo, scratch, home, command,
                                    {SOURCE_ROOT_ENV[OMP_BACK_SOURCE_PROVIDER]: str(source_root)})
    if isinstance(read, int):
        return read
    document, convert, target, verification, session_id, installed = read

    record("the installed transcript is filed under this run's own Claude project spelling of the workspace",
           installed.parent == home / CLAUDE_PROJECTS_SUBDIR / spelling,
           f"{installed.relative_to(home)}; this run's Claude spelling of {workspace} is {spelling}")
    counts = installed_counts(installed)
    parse_error = jsonl_parse_error(installed.read_text(encoding="utf-8"))
    record("the installed file is one JSON object per line", not parse_error,
           parse_error or f"{installed.stat().st_size} bytes, {sum(counts.values())} record(s)")

    if claude is None:
        record(f"Claude Code's own loader resolves the installed session ({args.claude} on PATH)", False,
               f"{args.claude} is not on PATH, so the native half of {args.direction} could not run",
               skipped=True)
    else:
        report_load = verification.get("load") or {}
        record("the load check verified the installed session natively",
               report_load.get("state") == "verified", bounded(report_load.get("detail", ""), 400))
        _check_claude_loader(claude, installed, workspace, session_id)

    _check_claude_invocation(target, workspace, home, args.claude)

    if args.resume_prompt:
        resume = verification.get("resume") or {}
        record("the resume check verified one real turn through the printed invocation",
               resume.get("state") == "verified", bounded(resume.get("detail", ""), 400))
        print_answer(resume.get("output", ""))
        if args.expect:
            record(f"the turn's answer contains {args.expect!r}",
                   args.expect in (resume.get("output") or ""),
                   bounded(resume.get("output", ""), 160))

    _record_unchanged_evidence(source_root, source_root_before, named_home, unrelated, planted_before,
                               existing_sessions)
    record("the conversion wrote no id the home already held",
           session_id not in existing_ids, f"{session_id}, not one of the {len(existing_ids)} id(s) already "
                                           f"in the home")

    missing = "" if claude is not None else (
        f"{args.claude} is not on PATH, so the native half of {args.direction} could not run")
    return finish(args, scratch, document, missing)


def _check_claude_loader(claude: str, installed: Path, workspace: Path, session_id: str) -> None:
    """Drive Claude Code's own loader against a **copy** of the converted bytes, the way
    `internal/ccc/claude_verify.go` does: `--bare -p --output-format json --resume <id>` in a
    throw-away data directory, plus an unknown-id control arm."""
    before = sha256(installed)
    loader_home = Path(tempfile.mkdtemp(prefix="ccc-claude-native-loader-"))
    try:
        copy = loader_home / CLAUDE_PROJECTS_SUBDIR / installed.parent.name / installed.name
        copy.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(installed, copy)
        try:
            completed = ask_claude_loader(claude, loader_home, workspace, session_id)
        except subprocess.TimeoutExpired:
            record("Claude Code's own loader resolves the installed session", False,
                   "the loader did not answer inside its deadline")
            return
        answer = last_json_line(completed.stdout)
        record("Claude Code's own loader resolves the installed session",
               answer is not None and answer.get("session_id") == session_id,
               f"exit {completed.returncode}; session_id "
               f"{(answer or {}).get('session_id')!r}"
               + ("" if answer is not None else f"; {bounded(completed.stderr or completed.stdout, 200)}"))

        unknown = "01a1c338-dead-beef-0000-000000000000"
        try:
            control = ask_claude_loader(claude, loader_home, workspace, unknown)
        except subprocess.TimeoutExpired:
            record("the unknown-id control arm reports no conversation", False,
                   "the control arm did not answer inside its deadline")
        else:
            record("the unknown-id control arm reports no conversation",
                   last_json_line(control.stdout) is None
                   and CLAUDE_CONTROL_ARM_TEXT in ((control.stdout or "") + (control.stderr or "")),
                   bounded(control.stderr or control.stdout, 200))
    finally:
        shutil.rmtree(loader_home, ignore_errors=True)
    record("the load ran against a copy and left the installed file alone", sha256(installed) == before)


def _check_claude_invocation(target: dict, workspace, home, claude: str) -> None:
    """Check Claude Code's printed resume commands the way a POSIX shell reads them."""
    binary = target.get("binary") or claude
    session_id = target.get("session_id", "")
    for field, arguments_field, arguments in (
            ("resume_command", "resume_arguments", [binary, "--resume", session_id]),
            ("exec_resume_command", "exec_resume_arguments",
             [binary, "-p", "--output-format", "json", "--resume", session_id,
              CONTINUE_TERMINATOR, PROMPT_PLACEHOLDER])):
        command = target.get(field, "")
        printed = target.get(arguments_field) or []
        ok = (bool(command) and printed == arguments
              and (target.get("resume_environment") or {}) == {CLAUDE_HOME_ENV: str(home)}
              and reported_invocation(command, workspace, home, CLAUDE_HOME_ENV, arguments))
        record(f"the printed {field} reads as a POSIX shell line naming the session", ok,
               bounded(command, 200))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--omp", default="omp", help="the OMP binary to verify against")
    parser.add_argument("--claude", default="claude",
                        help="the Claude Code binary the omp-to-claude direction verifies against "
                             "(default: claude on PATH)")
    parser.add_argument("--direction", choices=("claude-to-omp", "omp-to-claude"), default="claude-to-omp",
                        help="which route to verify (default: the OMP target)")
    parser.add_argument("--source", default=None,
                        help="the source session to convert (default: the committed Claude fixture, or the "
                             "committed OMP fixture for omp-to-claude)")
    parser.add_argument("--target-home", type=Path,
                        help="the target harness's data directory (default: a scratch one this run makes "
                             "and seeds from the host's own OMP home)")
    parser.add_argument("--resume-prompt",
                        help="run one real turn in the installed session with this prompt")
    parser.add_argument("--expect", help="require the resume turn's answer to contain this text")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    parser.add_argument("--model", default=os.environ.get("CCC_OMP_NATIVE_MODEL", ""),
                        help="the model the run's own scratch home prefers as its default role, when the "
                             "host's own home defines (or --provider-base-url defines) its provider; "
                             "empty (the default) uses the host home's own configured default, and no "
                             "model id is baked into this script")
    parser.add_argument("--provider-base-url", default=os.environ.get("LITELLM_BASE_URL", ""),
                        help="an endpoint for --model's provider, used only when the host's own home does "
                             "not define it (default: $LITELLM_BASE_URL)")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    if args.direction == "claude-to-omp":
        source_provider = SOURCE_PROVIDER
        source_session = args.source or SOURCE_SESSION
        omp = shutil.which(args.omp)
        if omp is None:
            print(f"RESULT: INCOMPLETE — 0 check(s) ran; {args.omp} is not on PATH, so the native half of "
                  f"{args.direction} (whose target harness is OMP) could not run")
            return 2
    else:
        source_provider = OMP_BACK_SOURCE_PROVIDER
        source_session = args.source or OMP_BACK_SOURCE_SESSION
    candidates = sorted((fixture_root / source_provider).rglob(f"*{source_session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {source_provider} fixture for {source_session} below "
              f"{fixture_root / source_provider}, found {len(candidates)}", file=sys.stderr)
        print(f"RESULT: INCOMPLETE — 0 check(s) ran; the {source_provider} fixture for {source_session} "
              f"was not found exactly once below {fixture_root / source_provider}")
        return 2
    source = candidates[0]
    if shutil.which("go") is None:
        print("go is not on PATH: this harness builds ccc with the Go toolchain", file=sys.stderr)
        print("RESULT: INCOMPLETE — 0 check(s) ran; the Go toolchain this harness builds ccc with is not "
              "on PATH")
        return 2

    scratch = Path(tempfile.mkdtemp(prefix="ccc-omp-native-check-")).resolve()
    workspace = scratch / WORKSPACE
    workspace.mkdir(parents=True, exist_ok=True)
    source_root = scratch / f"{source_provider}-source"
    destination = source_root / source.relative_to(fixture_root / source_provider)
    recorded_cwd = rewrite_source_cwd(source, destination, workspace)

    try:
        if args.direction == "claude-to-omp":
            return _claude_to_omp(args, repo, scratch, workspace, source_root, destination, recorded_cwd,
                                  source)
        return _omp_to_claude(args, repo, scratch, workspace, source_root, destination, recorded_cwd,
                              source)
    finally:
        # The fabricated session belongs to this harness and no one else: it is removed however the run
        # ends, so a second run against the same named home plants its own instead of meeting this one's
        # leftover -- and a file this run refused to overwrite is never one of them.
        for planted in PLANTED:
            planted.unlink(missing_ok=True)


if __name__ == "__main__":
    sys.exit(main())
