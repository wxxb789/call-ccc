#!/usr/bin/env python3
"""Native verification for the Claude Code → Codex route (goals 0002 and 0004).

This harness answers the half of the goal that the gate cannot: whether the installed target session is
readable by **Codex itself** and whether a **real turn** continues from it. It builds `ccc`, converts a
committed fixture into an isolated Codex data directory, re-reads the installed file with its own second
`ccc` invocation (the inherited reader, so the conversion's own verdict about itself decides nothing
here), checks the resume invocation the conversion reported (its arguments name the target binary, the
resume subcommand and the session, its environment selects the directory the session is installed into,
and the line is spelled so a POSIX shell reads it as that assignment rather than as a command word),
asks that directory's own app server to load the session (`thread/resume`), reads the harness's
own view of the conversation (`thread/read`), and — when `--resume-prompt` is given — runs one real turn
from the reported invocation and reports what the target answered. Only then — after the loader and the
turn, and before the cleanup that removes the planted session — does it compare the source session and
every other session in the target home (the session it plants there and any the home already held)
against the bytes they had before the conversion, so a mutation during either native stage is answered by
the comparison that runs last rather than by an earlier verdict standing in for it. The planted session is
removed again when the run ends, so a `--codex-home` the caller named is left as it was found apart from
the converted session.

**The source grows, and is converted again (goal 0004).** The fixture is converted from a *copy* this run
makes under its scratch directory, never from the committed bytes: the copy is what grows, by the
committed growth fragment below, and it is then converted a second time into the same home. That second
half answers the repeatable-handoff clauses: the second conversion's reported boundary is the grown
copy's own bytes (the check hashes the file itself rather than trusting that report), the appended turn's
words are in the second installed document and not in the first, the first session's bytes are unchanged
by the second conversion, and Codex's own loader opens the second session with its own view holding the
turn the source gained — all of it after the first session was loaded (and, with `--resume-prompt`,
resumed by its harness). Immediately before it grows, the copy is compared with the bytes it had before the
first conversion, because the loader and that turn run between the two: without that comparison a
*same-length* change made during the first native stage keeps the growth record's size delta true and its
digest different, and becomes the baseline every later comparison measures from. `--repeat-resume-prompt`
runs one further real turn in the *second* session, so the later snapshot's model-visible history is
answered by the target itself; each prompt flag runs one turn, and the run's report line says how many ran.

It is **not** a gate step: it needs the Codex binary, and a turn needs model access. `scripts/gate.py`
decides the offline half (round-trip through the inherited reader, install rules, reported losses, CLI
shape); this script is the re-derivable native evidence, the way `scripts/upstream-compare.py` is for the
carried tree's behaviour. Every app-server request it makes is bounded by `APP_SERVER_TIMEOUT`, and a
server that stops answering is reported as a failed check with its own diagnostics rather than blocking
the run; those diagnostics are a bounded tail of bounded lines, read in bounded pieces, so a server that
floods that pipe cannot grow this harness's memory either.

    python3 scripts/codex-native-check.py
    python3 scripts/codex-native-check.py --validate artifact.jsonl --receipt artifact-receipt.json --json
    python3 scripts/codex-native-check.py --selftest-mutations
    python3 scripts/codex-native-check.py --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
        --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --repeat-resume-prompt "What budget do retries share per session?"
    python3 scripts/codex-native-check.py --json          # machine-readable summary on stdout

`--model-provider`/`--model` name what the throw-away Codex configuration should use, because the
target's loader checks a session against the provider *its* configuration names: the load check only
needs a configured provider, while `--resume-prompt` and `--repeat-resume-prompt` need one that can
actually answer on this host (its key comes from the environment, never from a copied credential). That
throw-away configuration is built from the settings of the host's own `config.toml`, which is never
copied as a document: it is seeded with that file's model settings and the named provider's definition,
and never with a setting whose value can hold a credential -- a provider's authorization headers, an MCP
server's environment table, a credential written into a URL -- because a configuration is exactly where
those live. The report line names the settings that were copied and the ones that were left behind, so a
provider whose credential is written into the configuration has to take it from the environment instead.

Exit codes: 0 every check that ran passed; 1 a check failed; 2 the harness could not run (no Codex, no
Go toolchain, a missing fixture).
"""

from __future__ import annotations

import argparse
import collections
import contextlib
import hashlib
import io
import json
import os
import queue
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import tomllib
from pathlib import Path
from typing import Callable, Iterable, NamedTuple

# The shared module lives beside this script, and the tests load both of them by file location (neither
# file name is an identifier). Python does not put a script's own directory on `sys.path` when it is
# loaded that way, so the directory is added here the way `scripts/gate.py` adds its own.
sys.path.insert(0, str(Path(__file__).resolve().parent))

from native_check_common import (  # noqa: E402  (path set up above)
    PROMPT_PLACEHOLDER,
    RESULTS,
    SHELL_SYNTAX_CHARACTERS,
    TURN_OUTPUT_MARKER,
    credential_free_url,
    jsonl_parse_error,
    record,
    run,
    sha256,
    shell_words,
    tree_digest,
    unquoted_text,
)

#: The provider directory, and the two committed Claude fixtures this harness can convert. The first
#: carries dialogue, a tool call with its result and a compaction boundary; the second (TOOLING_SESSION)
#: carries a persisted reasoning trace, a subagent turn, an errored tool result and a tool result the
#: provider recorded on the same line as a system reminder, and no compaction boundary, so a resumed
#: turn still sees its tool calls.
SOURCE_PROVIDER = "claude"
SOURCE_SESSION = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"
TOOLING_SESSION = "b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef"
FIXTURE_ROOT = Path("ccc", "internal", "ccc", "testdata", "fixtures")
GO_TAGS = "fts5"
BINARY = Path("bin", "ccc-native-check")

#: The committed growth fragment: the bytes this harness appends to its *copy* of the fixture between the
#: two conversions, so the source session gains a turn without the committed fixture tree (which this run
#: hashes) moving. It is generated and pinned by `internal/ccc/fixturegen`, the same way the fixtures are.
GROWTH_ROOT = Path("ccc", "internal", "ccc", "testdata", "growth")
GROWTH_TURN = "claude-complete-turn.jsonl"

#: The provider root the copy is written under: a Claude Code transcript is resolved through the directory
#: its project-dir spelling names, and this run drives the conversions with `CLAUDE_PROJECTS_DIR` pointing
#: at its scratch root rather than at the committed fixtures.
COPY_ROOT = Path("source")

#: The unrelated session the harness plants in the target home before converting: a conversion must not
#: touch a session another agent may already have continued.
UNRELATED_NAME = "rollout-2026-01-01T00-00-00-00000000-0000-7000-8000-000000000000.jsonl"

#: The fabricated session the disclosure arm plants: the offline oracle's frozen artifact under a name of
#: its own, so the harness's own loader answers for bytes the offline rules accept and the two answers
#: stand side by side (the oracle holds no Codex knowledge, and this is where a caller sees what that
#: costs: the binary refuses an artifact the container rules hold green).
ORACLE_PROBE_ID = "00000000-0000-7000-8000-00000000a009"
ORACLE_PROBE_NAME = f"rollout-2026-01-01T00-00-01-{ORACLE_PROBE_ID}.jsonl"

#: One app-server request's deadline. Every call this harness makes is bounded by it: a server that keeps
#: its output open and stops answering is reported as a failed check rather than blocking the run.
APP_SERVER_TIMEOUT = 120

#: How many of the app server's diagnostic lines are retained, and how many characters of one line. The
#: drainer has to keep reading the pipe (a full one blocks the server instead of answering) but what it
#: *keeps* is a ring of the last lines, each cut to its own bound: the failure reports quote the tail of
#: it, and neither many lines nor one endless line can make this harness's own memory a function of the
#: target's output -- a runaway log would be retained for the whole run. Retaining the ring also bounds
#: the *bytes* kept: at most ``APP_SERVER_DIAGNOSTICS_MAX * (APP_SERVER_DIAGNOSTICS_LINE_MAX +
#: len(DIAGNOSTICS_TRUNCATED))`` characters, because a line the bound cuts is kept as its head while the
#: rest of it is read and dropped. Both bounds are pinned by ``scripts/tests/test_codex_native_check.py``.
APP_SERVER_DIAGNOSTICS_MAX = 200
APP_SERVER_DIAGNOSTICS_LINE_MAX = 2000

#: What a retained line says where the per-line bound cut it: a report quotes the head of a line that
#: was longer, and a reader of the report is told the line continued instead of being shown a shortened
#: line as though it were the server's whole line.
DIAGNOSTICS_TRUNCATED = " …[the rest of this line was dropped]"

#: The top-level settings the throw-away home's configuration is seeded with, and nothing else. The
#: target's loader compares a session against the provider its *own* configuration names, so these two
#: answer the question the seed exists for; the host configuration's other top-level settings (approval
#: policy, reasoning effort, project trust, the TUI) are not that question, and the tables below them --
#: `[mcp_servers.*]` with its `env` table, `[model_providers.*]` with its `http_headers` -- are where a
#: configuration holds credentials.
SEEDED_TOP_LEVEL = ("model", "model_provider")

#: The provider-definition settings that are copied: a display name, an endpoint, a wire protocol, and
#: the *names* of the environment variables a credential is read from (`env_key`, `env_http_headers` --
#: this harness requires environment-backed authentication). A provider setting whose value can be a
#: credential itself (`http_headers`, `http_headers_helper`, `query_params`, a provider's `auth` block) is
#: deliberately not in this list, so it is never copied; what is not copied is named in the report.
SEEDED_PROVIDER_SETTINGS = ("name", "base_url", "wire_api", "env_key", "env_http_headers")

#: How many setting names the seeded-configuration report lists before it counts the rest: the report is
#: a diagnostic line, and a host configuration can hold more settings than a line can usefully name.
REPORTED_NAME_LIMIT = 8

#: The target harness's own startup chatter, which every invocation prints on this platform. It is not
#: a verdict, so it is kept out of the one-line summaries this harness reports.
NOISE = ("PATH aliases", "bubblewrap", "is deprecated")


def last_meaningful_line(text: str) -> str:
    lines = [line.strip() for line in (text or "").splitlines()
             if line.strip() and not any(needle in line for needle in NOISE)]
    return lines[-1][:300] if lines else "(no diagnostics)"


#: The control characters TOML's basic strings have a named escape for. Every other character below
#: space is written as `\uXXXX`.
NAMED_ESCAPES = {"\n": "\\n", "\r": "\\r", "\t": "\\t"}


def toml_string(value: str) -> str:
    """One TOML basic string holding `value` unchanged.

    The values are the host's own settings, copied without interpretation, so a quote, a backslash or a
    control character in one of them must not change the document's shape. TOML's basic strings accept
    every escape written here and need no other, so what this returns is the same string when read back.
    """
    escaped: list[str] = []
    for char in value:
        if char in ('"', "\\"):
            escaped.append("\\" + char)
        elif char in NAMED_ESCAPES:
            escaped.append(NAMED_ESCAPES[char])
        elif char < " " or char == "\x7f":
            escaped.append(f"\\u{ord(char):04x}")
        else:
            escaped.append(char)
    return '"' + "".join(escaped) + '"'


def toml_key(name: str) -> str:
    """One TOML key: bare where the key's own characters allow it, quoted otherwise."""
    if name and all(char.isascii() and (char.isalnum() or char in "_-") for char in name):
        return name
    return toml_string(name)


def bounded_names(names: list[str]) -> str:
    """The setting names, comma-separated, with the ones past the report's limit counted rather than named."""
    if len(names) <= REPORTED_NAME_LIMIT:
        return ", ".join(names)
    return ", ".join(names[:REPORTED_NAME_LIMIT]) + f" and {len(names) - REPORTED_NAME_LIMIT} more"


class SeededConfiguration(NamedTuple):
    """What the throw-away Codex home's configuration was seeded with, and what it was not.

    `text` is the document the home is given -- empty when the host configuration contributed nothing of
    its own -- `copied` and `not_copied` name the settings the host configuration holds, in document
    order, and `problem` is non-empty when the host configuration exists but could not be read at all.
    """

    text: str
    copied: list[str]
    not_copied: list[str]
    problem: str

    def report(self) -> str:
        """What was seeded, as one line, naming settings and never a setting's value."""
        if self.problem:
            return (f"nothing was copied from it ({self.problem}); the throw-away home holds only the "
                    f"settings named on the command line, if any")
        head = ("copied " + ", ".join(self.copied)) if self.copied else \
            "nothing was copied: it holds none of the settings this check seeds"
        if self.not_copied:
            head += ("; not copied (the throw-away home is not a copy of the host's configuration: it is "
                     "seeded with the settings that answer the loader's question, never with one that can "
                     "hold a credential, so a provider whose credential is written there must take it "
                     "from the environment): " + bounded_names(self.not_copied))
        return head


def seed_configuration(host_text: str, overrides: dict[str, str]) -> SeededConfiguration:
    """The throw-away home's configuration, built from the settings of the host's own configuration.

    The host's `config.toml` is *not* copied: a configuration is where credentials live -- provider
    authorization headers, an MCP server's `env` table, a token in a query parameter -- and this run
    writes that file into a directory it reports about and, with `--keep`, leaves on disk. Only settings
    whose value cannot be a credential cross over, and the report names both what was copied and what was
    left behind. The host configuration's own `model`/`model_provider` are replaced by `overrides` when
    the caller names them, so `--model-provider`/`--model` still decide what the home answers with.

    A document this function cannot read as TOML is not one it copies from: it contributes nothing and
    the caller is told, rather than having its bytes written out unexamined.
    """
    problem = ""
    document: dict[str, object] = {}
    if host_text.strip():
        try:
            document = tomllib.loads(host_text)
        except tomllib.TOMLDecodeError as failure:
            problem = f"it is not readable as TOML: {failure}"

    values: dict[str, str] = {}
    copied: list[str] = []
    not_copied: list[str] = []
    for key in SEEDED_TOP_LEVEL:
        value = overrides.get(key, document.get(key))
        if isinstance(value, str) and value.strip():
            values[key] = value
            copied.append(key)
    provider_name = values.get("model_provider", "")

    definitions = document.get("model_providers")
    provider_table: dict[str, object] = {}
    if provider_name and isinstance(definitions, dict):
        named = definitions.get(provider_name)
        if isinstance(named, dict):
            provider_table = named
    provider_settings: dict[str, str] = {}
    env_headers: dict[str, str] = {}
    base_url = provider_table.get("base_url")
    if provider_table and isinstance(base_url, str) and not credential_free_url(base_url):
        # The definition stays whole rather than losing only its endpoint: half a provider definition
        # would send the harness to the provider's default address instead of the one the host named.
        not_copied.append(f"[model_providers.{provider_name}]")
    elif provider_table:
        for key in SEEDED_PROVIDER_SETTINGS:
            value = provider_table.get(key)
            if key == "env_http_headers" and isinstance(value, dict):
                env_headers = {header: target for header, target in value.items() if isinstance(target, str)}
                if env_headers:
                    copied.append(f"[model_providers.{provider_name}].env_http_headers")
            elif isinstance(value, str) and value:
                provider_settings[key] = value
                copied.append(f"[model_providers.{provider_name}].{key}")
        for key in provider_table:
            if key in provider_settings or (key == "env_http_headers" and env_headers):
                continue
            not_copied.append(f"[model_providers.{provider_name}].{key}")

    if isinstance(definitions, dict):
        for name in definitions:
            if name != provider_name or not provider_table:
                not_copied.append(f"[model_providers.{name}]")
    for key in document:
        if key in SEEDED_TOP_LEVEL or key == "model_providers":
            continue
        not_copied.append(f"[{key}]" if isinstance(document[key], dict) else key)

    lines: list[str] = []
    for key in SEEDED_TOP_LEVEL:
        if key in values:
            lines.append(f"{key} = {toml_string(values[key])}")
    if provider_settings or env_headers:
        lines.append("")
        lines.append(f"[model_providers.{toml_key(provider_name)}]")
        for key in SEEDED_PROVIDER_SETTINGS:
            if key in provider_settings:
                lines.append(f"{key} = {toml_string(provider_settings[key])}")
        if env_headers:
            lines.append("")
            lines.append(f"[model_providers.{toml_key(provider_name)}.env_http_headers]")
            for header, target in env_headers.items():
                lines.append(f"{toml_key(header)} = {toml_string(target)}")
    text = "\n".join(lines) + "\n" if lines else ""
    return SeededConfiguration(text, copied, not_copied, problem)


def reads_as_assignment(command: str, name: str) -> bool:
    """Whether a POSIX shell reads `command` as the `name=…` assignment plus inert word text.

    Two spellings that read back as the same words as the right one and are not it: the assignment
    quoted -- `'name=…' codex resume …` is a *program* by that name to a shell, so nothing sets the
    variable and no session resumes -- and a character a shell acts on left unquoted -- `name=a;b codex
    resume …` sets `name=a` and runs `b`. Both are answered from the spelling, before the quoting is
    removed: the assignment has to be the line's opening word, with nothing quoted before its `=`, and
    every character a shell would act on has to be quoted (which `shellQuote` does).
    """
    text = unquoted_text(command)
    if text is None or not text.startswith(f"{name}="):
        return False
    return not set(text).intersection(SHELL_SYNTAX_CHARACTERS)


def reported_invocation(command: str, home: Path, arguments: list[str]) -> bool:
    """Whether `command` is the invocation as a POSIX shell reads it: the assignment, then `arguments`.

    Codex takes its data directory from CODEX_HOME alone -- it has no flag for it -- so the assignment
    being first and exact is what makes a pasted line resume the session this run installed. The words
    alone do not answer that: a quoted assignment reads back as the very same words while a shell takes
    it as the command word, so the spelling is read too (`reads_as_assignment`).
    """
    return (shell_words(command) == [f"CODEX_HOME={home}", *arguments]
            and reads_as_assignment(command, "CODEX_HOME"))


def new_diagnostics() -> collections.deque[str]:
    """The bounded sink the stderr drainer writes into: the last lines, not a log.

    A ``deque`` with a ``maxlen`` drops the oldest line as each new one arrives, and the drainer cuts
    every line it keeps to the per-line bound, so what a failure report can quote is bounded by count
    *and* by bytes while the drainer keeps the pipe empty.
    """
    return collections.deque(maxlen=APP_SERVER_DIAGNOSTICS_MAX)


def retained_line(line: str, cut: bool = False) -> str:
    """One diagnostic line as the sink keeps it: at most ``APP_SERVER_DIAGNOSTICS_LINE_MAX`` characters.

    ``cut`` says the line continues past what was read, so a shortened line is marked as shortened
    rather than presented as the server's whole line.
    """
    if len(line) > APP_SERVER_DIAGNOSTICS_LINE_MAX:
        line, cut = line[:APP_SERVER_DIAGNOSTICS_LINE_MAX], True
    return line + DIAGNOSTICS_TRUNCATED if cut else line


def discard_line_remainder(read: Callable[[int], str]) -> bool:
    """Read and drop the rest of a line the bound cut short; True when the line ended, not the stream.

    The remainder is read rather than left in the pipe -- a full pipe blocks the server instead of
    answering -- but it is read in bounded pieces and thrown away, so an enormous line is kept out of
    this reader's own buffer. A stream that fails inside the remainder reports the line as *not* ended:
    what the server said before the failure is still worth keeping, and it is a cut line either way.
    """
    while True:
        try:
            chunk = read(APP_SERVER_DIAGNOSTICS_LINE_MAX)
        except (OSError, ValueError):
            return True
        if chunk == "":
            return False
        if chunk.endswith("\n"):
            return True


def drain_diagnostics(stream: Iterable[str], retained: collections.deque[str]) -> None:
    """Read a diagnostics stream to its end into ``retained``, bounded on both of its axes.

    ``retained``'s own ``maxlen`` bounds how many lines are kept, and every kept line is cut to
    ``APP_SERVER_DIAGNOSTICS_LINE_MAX`` characters, so the tail a failure report can quote is bounded by
    bytes as well as by count. What one read may accumulate is bounded too: a stream is read in bounded
    pieces (``readline(size)``, as a text pipe bounds it), and a line that does not end inside that bound
    has its head kept while the rest of it is read and dropped -- a server that writes one enormous line,
    or never writes a newline at all, must not be able to make this reader's memory a function of its own
    output. The stream ends when the server closes it, and a read that fails mid-stream keeps the lines
    that did arrive: a diagnostics tail that ends at the failure is worth more than an empty one.
    """
    try:
        read = getattr(stream, "readline", None)
        if read is None:
            # A sequence of already-split lines (the test seam): each element is one line, and it is
            # still cut to the per-line bound before it is kept.
            for line in stream:
                retained.append(retained_line(line.rstrip("\n")))
            return
        while True:
            line = read(APP_SERVER_DIAGNOSTICS_LINE_MAX)
            if line == "":
                return
            if line.endswith("\n"):
                retained.append(retained_line(line.rstrip("\n")))
                continue
            # The bound cut this line (or ended the stream): keep its head, drain the remainder.
            cut = discard_line_remainder(read)
            retained.append(retained_line(line, cut))
    except (OSError, ValueError):
        pass


class AppServerErrorResponse(RuntimeError):
    """The app server answered one call with a JSON-RPC error: the harness refused that call, which is a
    different fact from a handshake, timeout or close failure, where no answer was obtained."""


class AppServer:
    """A minimal client for the target's own app-server protocol over stdio.

    stdout is read by a thread of its own, so a call's deadline can end it: a server that keeps its
    stdout open and stops answering would otherwise block a synchronous read for ever, and this harness
    promises a bounded verdict for every check. stderr is drained by a second thread for the same
    reason -- a piped stream nobody reads fills up, and a server that blocks writing a diagnostic stops
    answering -- and the lines it holds are reported with a failure instead of being thrown away. What is
    reported is the *tail* of them: the drainer retains ``APP_SERVER_DIAGNOSTICS_MAX`` lines, each cut to
    ``APP_SERVER_DIAGNOSTICS_LINE_MAX`` characters and read in bounded pieces, so a server that logs for
    the whole run -- or writes one line it never ends -- cannot grow this harness's memory with it.
    """

    def __init__(self, binary: str, home: Path):
        self.next_id = 0
        self.process = subprocess.Popen(
            [binary, "app-server", "--listen", "stdio://"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, bufsize=1, env={**os.environ, "CODEX_HOME": str(home)},
        )
        self.responses: queue.Queue[str | None] = queue.Queue()
        self.diagnostics: collections.deque[str] = new_diagnostics()
        self.reader = threading.Thread(target=self._read_stdout, daemon=True)
        self.drainer = threading.Thread(target=self._drain_stderr, daemon=True)
        self.reader.start()
        self.drainer.start()

    def _read_stdout(self) -> None:
        stream = self.process.stdout
        if stream is None:
            self.responses.put(None)
            return
        try:
            for line in stream:
                self.responses.put(line)
        except (OSError, ValueError):
            pass
        self.responses.put(None)

    def _drain_stderr(self) -> None:
        stream = self.process.stderr
        if stream is None:
            return
        drain_diagnostics(stream, self.diagnostics)

    def close(self) -> None:
        try:
            if self.process.stdin:
                self.process.stdin.close()
        finally:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()

    def stderr_tail(self, limit: int = 3) -> str:
        # The sink is a deque (its maxlen is the bound), and a deque takes no slice: the last `limit`
        # lines are copied out of a ring that is already bounded.
        return " | ".join(list(self.diagnostics)[-limit:])[:400]

    def call(self, method: str, params: dict) -> dict:
        self.next_id += 1
        request_id = self.next_id
        payload = json.dumps({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
        assert self.process.stdin is not None
        self.process.stdin.write(payload + "\n")
        self.process.stdin.flush()
        deadline = time.monotonic() + APP_SERVER_TIMEOUT
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RuntimeError(
                    f"the app server did not answer {method} within {APP_SERVER_TIMEOUT}s"
                    f"{'; its diagnostics: ' + self.stderr_tail() if self.diagnostics else ''}")
            try:
                line = self.responses.get(timeout=remaining)
            except queue.Empty:
                continue
            if line is None:
                raise RuntimeError(f"the app server closed its output while answering {method}")
            message = json.loads(line)
            if message.get("id") != request_id:
                continue  # a notification (a warning, a status change): not this call's answer
            if "error" in message:
                raise AppServerErrorResponse(f"{method} failed: {message['error'].get('message')}")
            return message.get("result") or {}

    def notify(self, method: str, params: dict) -> None:
        assert self.process.stdin is not None
        self.process.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method, "params": params}) + "\n")
        self.process.stdin.flush()

    def handshake(self) -> None:
        self.call("initialize", {"clientInfo": {"name": "ccc-native-check", "version": "0.1.0"}})
        self.notify("initialized", {})


def append_fragment(path: Path, fragment: Path) -> int:
    """Append one fragment's bytes to a session file, answering the file's size afterwards.

    This is how the run makes its *copy* of the fixture grow: the copy is written under the scratch
    directory (``copy_of_fixture``), so the committed bytes this run also hashes never move.
    """
    data = fragment.read_bytes()
    with path.open("ab") as handle:
        handle.write(data)
    return path.stat().st_size


def appended_texts(fragment: Path) -> list[str]:
    """The dialogue texts one growth fragment records, read from the fragment's own bytes.

    They are what a run has to find in the *later* target's document and not in the earlier one, so the
    expectation is derived from the committed fragment rather than re-typed here. A record whose message
    content is a string holds one text; a record whose content is a block list holds the text of each of
    its text blocks.
    """
    texts: list[str] = []
    for line in fragment.read_text().splitlines():
        if not line.strip():
            continue
        content = (json.loads(line).get("message") or {}).get("content")
        if isinstance(content, str):
            texts.append(content)
        elif isinstance(content, list):
            texts.extend(str(block.get("text", "")) for block in content
                         if isinstance(block, dict) and block.get("type") == "text")
    return [text for text in texts if text]


def boundary_of(document: dict) -> dict:
    """The source boundary one conversion's JSON document reports (an empty mapping when it reports none)."""
    return ((document.get("source") or {}).get("boundary") or {})


def boundary_matches(document: dict, path: Path) -> tuple[bool, str]:
    """Whether a conversion's reported boundary is the bytes of ``path``, and that answer in words.

    The size and the digest are measured here, from the file itself: a boundary taken from another file,
    one that never ran, or one whose record offsets do not describe the file at all is a failure rather
    than something this harness takes the report's word for. The path the report names is compared with
    the same spelling this run handed the conversion, because that is the file the two must agree about.
    The read's own coverage answer is checked the same way: the conversion has to report that the file is
    still the size the snapshot it opened recorded (a size match, which is what the read's own record can
    show), which is what makes the two conversions of one growing copy attributable to their own bytes at
    all.
    """
    boundary = boundary_of(document)
    size, digest = path.stat().st_size, sha256(path)
    ok = (boundary.get("state") == "taken"
          and boundary.get("path") == str(path)
          and boundary.get("bytes") == size
          and boundary.get("sha256") == digest
          and boundary.get("read_bytes") == size
          and boundary.get("read_coverage") == "size-match"
          and boundary.get("record_offsets") == "claimed"
          and boundary.get("terminated_records_end_bytes") == size
          and not boundary.get("unterminated_tail_bytes"))
    detail = (f"boundary={json.dumps(boundary)}; {path.name} is {size} bytes, "
              f"sha256 {digest[:16]}…")
    return ok, detail


def copy_of_fixture(fixture: Path, root: Path) -> Path:
    """Copy one committed fixture under `root`, in the project directory the fixture itself is filed in.

    The copy — never the committed file — is what a run grows and converts: a Claude Code transcript is
    resolved through the project directory its own path spells, so the copy keeps that directory's name.
    """
    target = root / fixture.parent.name / fixture.name
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(fixture, target)
    return target


def thread_view(server: AppServer, thread_id: str) -> tuple[list[str], list[str]]:
    """The target harness's own view of a thread: the lines it shows, and the item types it lists.

    The item types come from the reply's own ``thread.turns[].items[].type`` fields, so the check that
    the converted turns are listed reads the structured answer rather than the lines this script
    rendered from it (a message body could contain the word the lines are searched for).
    """
    result = server.call("thread/read", {"threadId": thread_id, "includeTurns": True})
    thread = result.get("thread") or {}
    lines = [f"thread {thread.get('id')} [{thread.get('historyMode')}] provider={thread.get('modelProvider')}"]
    kinds: list[str] = []
    for turn in thread.get("turns") or []:
        lines.append(f"  turn {turn.get('id')}")
        for item in turn.get("items") or []:
            kind = str(item.get("type"))
            kinds.append(kind)
            if kind == "userMessage":
                text = "".join(part.get("text", "") for part in item.get("content") or [])
            else:
                text = item.get("text") or ""
            lines.append(f"    {kind}: {text}")
    return lines, kinds


def record_installed_jsonl(path: Path, check_name: str, note: str = "") -> None:
    """One installed file is one JSON object per line, under the check's own name.

    ``note`` is what the first conversion's file earns for its passing detail -- it is also the file the
    load check below reads -- while a file that is not that shape fails the check with the parse error
    itself, so the failure names the line rather than restating the check's name.
    """
    parsed = jsonl_parse_error(path.read_text())
    record(check_name, not parsed, parsed or f"{path.stat().st_size} bytes{note}")


def inspect_counts(repo: Path, home: Path, thread_id: str) -> tuple[dict, subprocess.CompletedProcess]:
    """The inherited reader's own read of one installed session: its counts, and the run that read them.

    The file is read back by this script's own invocation of the reader, not by the conversion's own
    structure block: `ccc inspect` is a second process reading the same isolated Codex root, and the
    counts it reports are compared with what the conversion says it carried. A broken round-trip check
    inside ccc -- or a CLI that mapped it to "verified" -- cannot make that comparison green.
    """
    readback = run([str(repo / BINARY), "inspect", f"codex:{thread_id}", "--json"],
                   cwd=repo, env={"CODEX_HOME": str(home)})
    try:
        counts = json.loads(readback.stdout).get("source", {}).get("counts", {})
    except json.JSONDecodeError:
        counts = {}
    return counts, readback


def readback_verdict(thread_id: str, counts: dict, carried: dict) -> tuple[bool, str]:
    """Whether the reader's counts agree with what one conversion carried, and that answer in words.

    The comparison is over the counts that name evidence, because the reader normalises records it reads
    (a standalone function_call becomes an assistant message of its own, for one), so a comparison of
    whole message counts would compare the writer's shape with the reader's.
    """
    agreed = (counts.get("user_messages") == carried["user_messages"]
              and counts.get("tool_calls") == carried["tool_calls"]
              and counts.get("tool_results") == carried["tool_results"]
              and counts.get("compact_boundaries") == carried["compaction_boundaries"])
    detail = (f"ccc inspect codex:{thread_id}: {json.dumps(counts)}; conversion carried: "
              f"{carried['user_messages']} user message(s), {carried['tool_calls']} tool call(s), "
              f"{carried['tool_results']} tool result(s), "
              f"{carried['compaction_boundaries']} boundary(ies)")
    return agreed, detail


def load_and_view(codex: str, home: Path, thread_id: str, installed: Path,
                  loader_name: str, view_label: str) -> tuple[list[str], list[str]]:
    """One installed session as the target harness itself answers for it: its own view, or the failure.

    The strings a run records about a load are the run's own -- each conversion's checks have their own
    names and the second session's own view has its own heading -- and the loader's answer is the
    harness's reply itself: a server that stopped answering is a failed check carrying its diagnostics,
    not a traceback, because the protocol is bounded and so is this run's verdict.
    """
    view: list[str] = []
    kinds: list[str] = []
    server = AppServer(codex, home)
    try:
        server.handshake()
        loaded = server.call("thread/resume", {"threadId": thread_id})
        loaded_thread = loaded.get("thread") or {}
        record(loader_name,
               loaded_thread.get("id") == thread_id and str(loaded_thread.get("path")) == str(installed),
               f"thread/resume -> {loaded_thread.get('id')} [{loaded_thread.get('historyMode')}] "
               f"cwd={loaded_thread.get('cwd')}")
        view, kinds = thread_view(server, thread_id)
    except RuntimeError as failure:
        record(loader_name, False, str(failure))
    finally:
        server.close()
    if view:
        print()
        print(view_label)
        for line in view:
            print("   ", line)
    return view, kinds


def resume_answer(codex: str, home: Path, thread_id: str, planted: Path) -> tuple[bool, str]:
    """The target harness's own answer for one artifact: whether it refused the session, and its own
    words either way.

    A JSON-RPC error answer to `thread/resume` counts as the harness's refusal of the artifact only
    where its own words name the planted session or its file: any error answer can be protocol drift or
    a thread-store failure, so an answer that names neither is recorded as a failure in its own words
    rather than as the refusal this check's name claims. A handshake, timeout or close failure is no
    answer at all, and is likewise *not* a refusal, because a run that never obtained an answer may not
    record the refusal its name claims.
    """
    server = AppServer(codex, home)
    try:
        try:
            server.handshake()
        except RuntimeError as failure:
            return False, f"the harness never answered: {failure}"
        try:
            loaded = server.call("thread/resume", {"threadId": thread_id})
        except AppServerErrorResponse as answer:
            if thread_id in str(answer) or planted.name in str(answer):
                return True, f"thread/resume answered: {answer}"
            return False, (f"the harness answered an error that names neither the planted session "
                           f"{thread_id} nor its file {planted.name}: {answer}")
        except RuntimeError as failure:
            return False, f"the harness never answered: {failure}"
    finally:
        server.close()
    thread = loaded.get("thread") or {}
    return False, (f"thread/resume answered {thread.get('id')} [{thread.get('historyMode')}] "
                   f"path={thread.get('path')}")


def turn_identity(codex: str, thread_id: str, reported_arguments: list[str]) -> list[str]:
    """The words one real turn runs: the conversion's reported arguments, or the identity itself.

    The turn runs the arguments the conversion reported -- binary, subcommand, identity -- rather than a
    command line spelled here, so the printed invocation is what the check exercises. A report that did
    not check out (the comparison above recorded that) leaves the identity spelled here, so the turn
    still answers whether a turn can continue.
    """
    if reported_arguments == [codex, "exec", "resume", thread_id, PROMPT_PLACEHOLDER]:
        return reported_arguments[1:-1]
    return ["exec", "resume", thread_id]


def run_turn(repo: Path, codex: str, home: Path, session_file: Path, identity: list[str],
             prompt: str, check_name: str) -> None:
    """One real turn in an installed session, recorded under ``check_name``, and its answer printed.

    The flags this harness adds are its own isolation (no git repository, a read-only sandbox, no
    approvals) and belong to the check, not to the reported command; what the check answers is whether
    the turn ran and the session it names grew.
    """
    session_before = session_file.stat().st_size
    turn = run([codex] + identity + [prompt] + [
        "--skip-git-repo-check", "-c", 'sandbox_mode="read-only"', "-c", 'approval_policy="never"',
    ], cwd=repo, env={"CODEX_HOME": str(home)}, timeout=900)
    session_after = session_file.stat().st_size
    record(check_name, turn.returncode == 0 and session_after > session_before,
           f"exit {turn.returncode}, session {session_before} -> {session_after} bytes")
    print()
    print(TURN_OUTPUT_MARKER)
    for line in (turn.stdout or turn.stderr).splitlines():
        print("   ", line)


#: The deepest `{`/`[` nesting one artifact line may reach before the decoder reads it: the bound
#: `ORACLE_RULES["oracle.jsonl-objects"]` states, so a line's verdict is its own bytes rather than the
#: reading interpreter's recursion limit (CPython's default limit covers this depth on either scanner).
ORACLE_NESTING_BOUND = 256

#: Every rule the offline oracle states: rule id → the sentence it states. The ids are this report's own
#: vocabulary and the keys of `ORACLE_MUTATIONS`, which `--selftest-mutations` asserts to be in
#: bijection with this table.
ORACLE_RULES: dict[str, str] = {
    "oracle.utf8":
        "the artifact's bytes are valid UTF-8 text and open with no byte-order mark",
    "oracle.whole-lines":
        "the artifact's bytes are whole lines — the empty artifact, or bytes whose final byte is a line "
        "feed; an artifact whose last bytes are an unterminated record is refused",
    "oracle.jsonl-objects":
        "every line of the artifact is exactly one JSON object, every member name written once inside "
        "it, and no string it holds carries an unpaired surrogate escape: a repeated name and a "
        "`\\ud800`-style escape are shapes JSON itself leaves two readers free to answer differently "
        "for, so such a line is refused rather than read as either answer; a line nested deeper than "
        f"{ORACLE_NESTING_BOUND} levels is refused as well, because that bound is this rule's own "
        "statement rather than the reading interpreter's recursion limit",
    "oracle.receipt-integrity":
        "a receipt states the frozen artifact's byte count, its whole-file SHA-256 and one anchor per "
        "line — the SHA-256 of that line's own bytes, line feed included, in order — and a receipt "
        "whose statements do not match the artifact's bytes is refused, naming the line a changed byte "
        "moved; a receipt covers an artifact holding at least one line, so the empty artifact is not "
        "receipt-identified by this rule",
}

#: The fixtures the oracle is frozen against: one artifact that models no agent's format, and the
#: receipt that names its bytes. A fixture shaped like a Codex session would carry the foreign-format
#: knowledge this re-scope deleted; a case in scripts/tests/test_codex_native_check.py fails one that
#: reappears, and the native check's disclosure arm shows the binary refusing this artifact's bytes.
ORACLE_FIXTURE_ROOT = Path(__file__).resolve().parent / "tests" / "testdata" / "codex-oracle"
ORACLE_ARTIFACT = "artifact.jsonl"
ORACLE_RECEIPT = "artifact-receipt.json"

#: The sentinel for "no receipt was supplied": a *supplied* document that reads `null` is a receipt that
#: is not an object, which `oracle.receipt-integrity` refuses — not an absent receipt.
_NO_RECEIPT = object()


class Violation(NamedTuple):
    rule: str
    line: int
    detail: str


def oracle_digest_stated(value: object) -> str | None:
    if isinstance(value, str) and len(value) == 64 and all(char in "0123456789abcdef" for char in value):
        return value
    return None


class _OracleMemberNameRepeated(ValueError):
    """One JSON document writes a member name twice: no single reading to answer with.

    The message names neither the member nor its value: this report is printed by default, and a member
    name is a string a document controls.
    """

    def __init__(self) -> None:
        super().__init__("one member name is written twice")


def _oracle_unique_members(pairs: list[tuple[str, object]]) -> dict:
    """One JSON object, refused where a member name is written twice.

    Python's decoder keeps the last value for a repeated name and the carried tree's `gjson` the first
    (measured on v1.20.0), so such a line is one two readers answer differently for: it is refused
    rather than compared through either reading. The artifact's lines and the receipt document are read
    through this same hook, because a document whose members collide is no more one document than such
    a line is one record.
    """
    members: dict[str, object] = {}
    for name, value in pairs:
        if name in members:
            raise _OracleMemberNameRepeated()
        members[name] = value
    return members


def _reject_json_constant(name: str) -> None:
    """Refuse a constant JSON does not define (`NaN`, `Infinity`, `-Infinity`), which Python accepts:
    the artifact is JSON, so a line this decoder would turn into a float the format has no spelling for
    is refused exactly as a line that does not parse is."""
    raise ValueError(f"{name} is not a JSON constant")


def _oracle_lone_surrogate(value: object) -> bool:
    """Whether one parsed line holds an unpaired surrogate code point in any of its strings — member
    names included.

    JSON's `\\ud800`-style escape decodes to one, which this decoder answers as the code point and a Go
    reader as U+FFFD, so the line is one two readers answer differently for and its strings may not be
    compared by either. The walk is iterative rather than recursive: a recursive scan holds a frame per
    container level, so an object `json.loads` accepted could exhaust the interpreter's stack instead of
    yielding a verdict.
    """
    stack = [value]
    while stack:
        item = stack.pop()
        if isinstance(item, str):
            if any(0xD800 <= ord(character) <= 0xDFFF for character in item):
                return True
        elif isinstance(item, dict):
            for name, member in item.items():
                stack.append(name)
                stack.append(member)
        elif isinstance(item, list):
            stack.extend(item)
    return False


def oracle_utf8(data: bytes) -> tuple[str | None, list[Violation]]:
    """The byte layer: the artifact's text, or the one violation that ends the reading.

    A byte-order mark is refused here rather than left to the line layer below: Python's `json.loads`
    and Go's `encoding/json` both refuse a document that opens with one, so without this rule the mark
    would be reported as the first line's own defect rather than as the framing byte it is.
    """
    if data.startswith(b"\xef\xbb\xbf"):
        return None, [Violation("oracle.utf8", 0, "the artifact opens with a UTF-8 byte-order mark")]
    try:
        return data.decode("utf-8"), []
    except UnicodeDecodeError as failure:
        return None, [Violation("oracle.utf8", 0,
                                f"the artifact is not UTF-8 text: byte 0x{data[failure.start]:02x} at "
                                f"offset {failure.start}")]


def oracle_lines(text: str) -> list[str]:
    """One artifact's lines, line feeds excluded; the split is on the line feed alone.

    `str.splitlines` is not this split — it also breaks on U+0085, U+2028 and the other boundaries a
    JSON string may hold — and the file's final line feed ends its last record rather than opening an
    empty one.
    """
    lines = text.split("\n")
    if lines and lines[-1] == "":
        lines.pop()
    return lines


def oracle_line_bytes(data: bytes) -> list[bytes]:
    """The same lines as their own bytes, each with its line feed where it has one — what a receipt's
    anchors are computed over — so the two splits cannot disagree about how many lines there are."""
    lines = data.split(b"\n")
    if lines and lines[-1] == b"":
        lines.pop()
        return [line + b"\n" for line in lines]
    return [line + b"\n" for line in lines[:-1]] + lines[-1:]


def oracle_whole_lines(text: str) -> list[Violation]:
    """`oracle.whole-lines`: the bytes after the artifact's last line feed, where there are any."""
    if not text or text.endswith("\n"):
        return []
    tail = len(text) - text.rfind("\n") - 1
    return [Violation("oracle.whole-lines", len(oracle_lines(text)),
                      f"the artifact's last {tail} byte(s) are a record without its line feed")]


def _oracle_line_nesting(line: str) -> int:
    """The deepest `{`/`[` nesting one line reaches, counted outside its string literals: a bracket
    inside a string is that string's text, so the count is what `ORACLE_NESTING_BOUND` is stated over."""
    depth = deepest = 0
    in_string = escaped = False
    for character in line:
        if in_string:
            if escaped:
                escaped = False
            elif character == "\\":
                escaped = True
            elif character == '"':
                in_string = False
        elif character == '"':
            in_string = True
        elif character in "{[":
            depth += 1
            deepest = max(deepest, depth)
        elif character in "}]":
            depth -= 1
    return deepest


def oracle_records(text: str) -> tuple[list[tuple[int, dict]], list[Violation]]:
    """One line per record: every line exactly one JSON object, read the way a strict JSON reader reads
    it — no `NaN`/`Infinity` constant, no repeated member name, no unpaired surrogate escape, and a line
    nested deeper than `ORACLE_NESTING_BOUND` levels refused by this rule's own bound rather than by the
    reading interpreter's recursion limit."""
    records: list[tuple[int, dict]] = []
    violations: list[Violation] = []
    for number, line in enumerate(oracle_lines(text), start=1):
        if _oracle_line_nesting(line) > ORACLE_NESTING_BOUND:
            violations.append(Violation("oracle.jsonl-objects", number,
                                        f"not JSON: nested deeper than {ORACLE_NESTING_BOUND} levels, "
                                        f"the bound this rule states"))
            continue
        try:
            value = json.loads(line, parse_constant=_reject_json_constant,
                               object_pairs_hook=_oracle_unique_members)
        except _OracleMemberNameRepeated:
            violations.append(Violation("oracle.jsonl-objects", number,
                                        "one member name is written twice in one object, so two JSON "
                                        "readers answer different records for the line"))
            continue
        except json.JSONDecodeError as broken:
            violations.append(Violation("oracle.jsonl-objects", number,
                                        f"not JSON ({broken.msg} at column {broken.colno})"))
            continue
        except ValueError as broken:
            # `_reject_json_constant`'s own refusal (`JSONDecodeError` is a `ValueError` and is answered
            # above, so this arm is a constant the format does not hold).
            violations.append(Violation("oracle.jsonl-objects", number, f"not JSON ({broken})"))
            continue
        if not isinstance(value, dict):
            violations.append(Violation("oracle.jsonl-objects", number,
                                        f"one JSON object per line, found {type(value).__name__}"))
            continue
        if _oracle_lone_surrogate(value):
            violations.append(Violation("oracle.jsonl-objects", number,
                                        "the line holds an unpaired surrogate escape (`\\ud800`-style): "
                                        "two JSON readers answer different strings for the line"))
            continue
        records.append((number, value))
    return records, violations


def oracle_fixture(name: str) -> bytes:
    return (ORACLE_FIXTURE_ROOT / name).read_bytes()


def oracle_receipt(name: str) -> dict:
    return json.loads(oracle_fixture(name).decode("utf-8"))


def oracle_receipt_violations(data: bytes, receipt: object) -> list[Violation]:
    """`oracle.receipt-integrity`: the receipt's own statements, read against the artifact's bytes.

    Further members than the three the rule names are not read: a receipt is the three statements it is
    asked for, and a statement this oracle does not know is not one it may compare.
    """
    if not isinstance(receipt, dict):
        return [Violation("oracle.receipt-integrity", 0,
                          f"a receipt is a JSON object, found {type(receipt).__name__}")]
    violations: list[Violation] = []
    stated_bytes = receipt.get("bytes")
    if not isinstance(stated_bytes, int) or isinstance(stated_bytes, bool) or stated_bytes < 0:
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    "a receipt states its artifact's byte count as a non-negative "
                                    "integer"))
    elif stated_bytes != len(data):
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    f"the receipt states {stated_bytes} byte(s), the artifact holds "
                                    f"{len(data)}"))
    stated_digest = oracle_digest_stated(receipt.get("sha256"))
    if stated_digest is None:
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    "a receipt states its artifact's whole-file SHA-256 as 64 "
                                    "lowercase hex digits"))
    elif stated_digest != hashlib.sha256(data).hexdigest():
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    f"the receipt states sha256 {stated_digest[:12]}…, the artifact's "
                                    f"bytes are sha256 {hashlib.sha256(data).hexdigest()[:12]}…"))
    anchors = receipt.get("lines")
    spans = oracle_line_bytes(data)
    if not isinstance(anchors, list) or not anchors:
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    "a receipt names one anchor per line, as a non-empty array"))
    elif len(anchors) != len(spans):
        violations.append(Violation("oracle.receipt-integrity", 0,
                                    f"the receipt names {len(anchors)} line(s), the artifact holds "
                                    f"{len(spans)}"))
    else:
        for number, (anchor, span) in enumerate(zip(anchors, spans), start=1):
            stated = oracle_digest_stated(anchor.get("sha256") if isinstance(anchor, dict) else None)
            if stated is None:
                violations.append(Violation("oracle.receipt-integrity", number,
                                            "an anchor is 64 lowercase hex digits"))
            elif stated != hashlib.sha256(span).hexdigest():
                violations.append(Violation("oracle.receipt-integrity", number,
                                            f"the line's bytes are sha256 "
                                            f"{hashlib.sha256(span).hexdigest()[:12]}…, the receipt's "
                                            f"anchor is sha256 {stated[:12]}…"))
    return violations


def oracle_reading(data: bytes, receipt: object = _NO_RECEIPT,
                   disabled: frozenset[str] = frozenset()
                   ) -> tuple[list[tuple[int, dict]], list[Violation]]:
    """One artifact read once: its records and every rule's violations.

    The byte layer is read first and a violation there ends the reading — nothing below the bytes can be
    read, and a receipt is not compared against bytes whose text could not be read — so
    `--disable-rule oracle.utf8` hands back a reading with nothing else in it.
    """
    text, violations = oracle_utf8(data)
    records: list[tuple[int, dict]] = []
    if text is not None:
        violations += oracle_whole_lines(text)
        records, json_violations = oracle_records(text)
        violations += json_violations
        if receipt is not _NO_RECEIPT:
            violations += oracle_receipt_violations(data, receipt)
    return records, sorted((violation for violation in violations if violation.rule not in disabled),
                           key=lambda violation: (violation.line, violation.rule, violation.detail))


def validate_artifact(data: bytes, receipt: object = _NO_RECEIPT,
                      disabled: Iterable[str] = ()) -> list[Violation]:
    """Every rule this oracle states, over one artifact's own bytes and — where a receipt is handed in —
    its receipt: no path, no source session and no harness, which is what lets a reduced artifact be
    validated with the source absent from the process."""
    return oracle_reading(data, receipt, frozenset(disabled))[1]


def oracle_report(artifact: Path, receipt: Path | None, as_json: bool,
                  disabled: frozenset[str] = frozenset()) -> int:
    """The oracle's own command: one artifact, at most one receipt, a verdict and an exit code.

    It reads the artifact's bytes and, when one is named, the receipt's — nothing else, ever. An input
    it cannot read at all ends as `oracle: INCOMPLETE` (exit 2), never as a pass: a file that cannot be
    opened, a receipt document that is not UTF-8 or not JSON, and a document whose object writes a
    member name twice are all such inputs. The verdict line is the last one printed, and `--json` adds
    the machine-readable document above it.
    """
    try:
        data = artifact.read_bytes()
    except OSError as failure:
        print(f"oracle: INCOMPLETE — the artifact {artifact} cannot be read: {failure}",
              file=sys.stderr)
        return 2
    document: object = _NO_RECEIPT
    if receipt is not None:
        try:
            document = json.loads(receipt.read_bytes().decode("utf-8"),
                                  parse_constant=_reject_json_constant,
                                  object_pairs_hook=_oracle_unique_members)
        except UnicodeDecodeError as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} is not UTF-8 text: {failure}",
                  file=sys.stderr)
            return 2
        except OSError as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} cannot be read: {failure}",
                  file=sys.stderr)
            return 2
        except (ValueError, RecursionError) as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} cannot be read: {failure}",
                  file=sys.stderr)
            return 2
    records, violations = oracle_reading(data, receipt=document, disabled=disabled)
    if as_json:
        print(json.dumps({
            "artifact": str(artifact),
            "receipt": str(receipt) if receipt is not None else None,
            "disabled_rules": sorted(disabled),
            "records": len(records),
            "violations": [{"rule": rule, "line": line, "detail": detail}
                           for rule, line, detail in violations],
            "verdict": "PASS" if not violations else "FAIL",
        }, indent=2))
    else:
        for rule, line, detail in violations:
            print(f"oracle: {rule}" + (f" line {line}" if line else "") + f" — {detail}")
    print(f"oracle: {'PASS' if not violations else 'FAIL'} — {len(records)} record(s)"
          + (f", {len(violations)} violation(s)" if violations else ""))
    return 0 if not violations else 1


class OracleMutation(NamedTuple):
    label: str
    artifact: str
    receipt: str | None
    mutate: Callable[[bytes, dict | None], tuple[bytes, dict | None]]


def oracle_replace_line(data: bytes, number: int, replacement: bytes) -> bytes:
    lines = data.split(b"\n")
    lines[number - 1] = replacement
    return b"\n".join(lines)


def _mutation_byte_not_utf8(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    """One byte inside the first line's string replaced with 0xFF: still one whole line, no longer
    UTF-8 text."""
    lines = data.split(b"\n")
    assert b"frozen artifact" in lines[0], "the fixture no longer holds the text this mutation edits"
    lines[0] = lines[0].replace(b"frozen", b"froz\xffen", 1)
    return b"\n".join(lines), receipt


def _mutation_byte_order_mark(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    return b"\xef\xbb\xbf" + data, receipt


def _mutation_final_line_feed_dropped(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    assert data.endswith(b"\n"), "the fixture no longer ends at a line feed"
    return data[:-1], receipt


def _mutation_line_not_json(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    return oracle_replace_line(data, 2, b"this line is not JSON"), receipt


def _mutation_line_is_an_array(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    return oracle_replace_line(data, 2, b"[1, 2, 3]"), receipt


def _mutation_line_non_json_constant(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    """One value written as `NaN`, a constant JSON has no spelling for: Python's decoder accepts it, and
    a reader that refuses it would answer differently."""
    return oracle_replace_line(data, 2, b'{"n": NaN}'), receipt


def _mutation_line_nested_past_reading(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    depth = 200000
    return oracle_replace_line(data, 2, b'{"n": ' + b"[" * depth + b"]" * depth + b"}"), receipt


def _mutation_line_duplicate_member(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    return oracle_replace_line(data, 2, b'{"note": "one", "note": "two"}'), receipt


def _mutation_line_lone_surrogate(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    """One string written with a `\\ud800`-style escape: valid JSON the decoder answers with a code
    point that is not text, where a Go reader writes U+FFFD."""
    return oracle_replace_line(data, 2, b'{"note": "\\ud800"}'), receipt


def _mutation_artifact_byte_changed(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    """One value inside the third line changed under the committed receipt: the same length, still one
    whole JSON object per line — and no longer the frozen bytes."""
    lines = data.split(b"\n")
    assert b'"n":3' in lines[2], "the fixture no longer holds the value this mutation edits"
    lines[2] = lines[2].replace(b'"n":3', b'"n":9')
    return b"\n".join(lines), receipt


def _mutation_receipt_other_digest(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    assert receipt is not None
    receipt["sha256"] = "0" * 64
    return data, receipt


def _mutation_receipt_byte_count(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    assert receipt is not None
    receipt["bytes"] = receipt["bytes"] + 1
    return data, receipt


def _mutation_receipt_no_lines(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    assert receipt is not None
    receipt["lines"] = []
    return data, receipt


def _mutation_receipt_one_line_fewer(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    assert receipt is not None
    receipt["lines"] = receipt["lines"][:-1]
    return data, receipt


def _mutation_receipt_last_anchor_another_line(data: bytes, receipt: dict | None
                                               ) -> tuple[bytes, dict | None]:
    assert receipt is not None
    assert receipt["lines"][-1] != receipt["lines"][0], "the fixture's anchors no longer differ"
    receipt["lines"][-1] = dict(receipt["lines"][0])
    return data, receipt


def _mutation_receipt_not_an_object(data: bytes, receipt: dict | None) -> tuple[bytes, dict | None]:
    return data, []


ORACLE_MUTATIONS: dict[str, tuple[OracleMutation, ...]] = {
    "oracle.utf8": (
        OracleMutation("a byte that is not UTF-8", ORACLE_ARTIFACT, None, _mutation_byte_not_utf8),
        OracleMutation("a byte-order mark", ORACLE_ARTIFACT, None, _mutation_byte_order_mark),
    ),
    "oracle.whole-lines": (
        OracleMutation("a record left unterminated", ORACLE_ARTIFACT, None,
                       _mutation_final_line_feed_dropped),
    ),
    "oracle.jsonl-objects": (
        OracleMutation("a line that is not JSON", ORACLE_ARTIFACT, None, _mutation_line_not_json),
        OracleMutation("a line that parses as an array", ORACLE_ARTIFACT, None,
                       _mutation_line_is_an_array),
        OracleMutation("a line holding a non-JSON constant", ORACLE_ARTIFACT, None,
                       _mutation_line_non_json_constant),
        OracleMutation("a line nested past reading", ORACLE_ARTIFACT, None,
                       _mutation_line_nested_past_reading),
        OracleMutation("a line holding a member name twice", ORACLE_ARTIFACT, None,
                       _mutation_line_duplicate_member),
        OracleMutation("a line holding an unpaired surrogate escape", ORACLE_ARTIFACT, None,
                       _mutation_line_lone_surrogate),
    ),
    "oracle.receipt-integrity": (
        OracleMutation("an artifact byte changed under an unchanged receipt", ORACLE_ARTIFACT,
                       ORACLE_RECEIPT, _mutation_artifact_byte_changed),
        OracleMutation("a receipt that states another whole-file digest", ORACLE_ARTIFACT,
                       ORACLE_RECEIPT, _mutation_receipt_other_digest),
        OracleMutation("a receipt whose byte count is wrong", ORACLE_ARTIFACT, ORACLE_RECEIPT,
                       _mutation_receipt_byte_count),
        OracleMutation("a receipt naming no lines", ORACLE_ARTIFACT, ORACLE_RECEIPT,
                       _mutation_receipt_no_lines),
        OracleMutation("a receipt naming one line fewer", ORACLE_ARTIFACT, ORACLE_RECEIPT,
                       _mutation_receipt_one_line_fewer),
        OracleMutation("a receipt whose last anchor is another line's", ORACLE_ARTIFACT,
                       ORACLE_RECEIPT, _mutation_receipt_last_anchor_another_line),
        OracleMutation("a receipt that is not an object", ORACLE_ARTIFACT, ORACLE_RECEIPT,
                       _mutation_receipt_not_an_object),
    ),
}


def _oracle_cli_reading(argv: list[str]) -> tuple[int, list[dict], str | None]:
    """One `--validate` invocation through `main`, the shipped command's own entry point.

    Returns ``(exit code, violations, None)`` where the command answered a readable JSON document, and
    ``(code, [], why)`` where it did not. The arms of `_oracle_selftest_one` are read through this
    function rather than `validate_artifact`'s Python `disabled` default: a regression that stopped
    `--disable-rule <RULE_ID>` from reaching the reading turns `--selftest-mutations` red instead of
    staying green behind an in-process call.
    """
    output, errors = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
        try:
            code = main(argv)
        except SystemExit as refusal:  # argparse refused the invocation itself
            return int(refusal.code or 0), [], ("the command refused the invocation: "
                                                + errors.getvalue().strip())
    stdout = output.getvalue()
    marker = stdout.find("\noracle: ")
    if marker < 0 or not stdout.startswith("{"):
        answer = (stdout.strip() or errors.getvalue().strip() or "no output")
        return code, [], f"the command printed no JSON document (exit {code}): {answer[:200]}"
    try:
        document = json.loads(stdout[:marker])
    except ValueError as failure:
        return code, [], f"the command's JSON document cannot be read ({failure})"
    return code, document["violations"], None


def _oracle_selftest_one(rule: str, mutation: OracleMutation, work: Path) -> str | None:
    """One fixture pair's three arms, each through the shipped command: None where all three hold.

    (a) the positive fixture passes — with its receipt where the pair names one; (b) the mutated pair
    differs from it and fails citing **only** `rule`; (c) the same mutated pair passes with
    `--disable-rule` for `rule`. The mutated bytes are materialized under `work`, the only place this
    run writes, so both negative arms judge the bytes a caller of the command would supply.
    """
    try:
        data = oracle_fixture(mutation.artifact)
        receipt = oracle_receipt(mutation.receipt) if mutation.receipt is not None else None
    except (OSError, ValueError) as failure:
        return f"the committed fixture cannot be read ({failure})"
    positive_argv = ["--validate", str(ORACLE_FIXTURE_ROOT / mutation.artifact), "--json"]
    if mutation.receipt is not None:
        positive_argv += ["--receipt", str(ORACLE_FIXTURE_ROOT / mutation.receipt)]
    code, violations, problem = _oracle_cli_reading(positive_argv)
    if problem is not None:
        return f"the positive fixture: {problem}"
    if code != 0 or violations:
        hit = sorted({violation["rule"] for violation in violations})
        return (f"the positive fixture is not green (exit {code}, citing {hit or ['no rule']}): "
                + "; ".join(f"{violation['rule']} line {violation['line']} — {violation['detail']}"
                            for violation in violations))
    before = (data, json.dumps(receipt, sort_keys=True) if receipt is not None else None)
    mutated_data, mutated_receipt = mutation.mutate(
        data, oracle_receipt(mutation.receipt) if mutation.receipt is not None else None)
    after = (mutated_data,
             json.dumps(mutated_receipt, sort_keys=True) if mutated_receipt is not None else None)
    if after == before:
        return "the mutation changed nothing"
    stem = rule.replace(".", "-")
    artifact_path = work / (stem + ".jsonl")
    artifact_path.write_bytes(mutated_data)
    negative_argv = ["--validate", str(artifact_path), "--json"]
    if mutated_receipt is not None:
        receipt_path = work / (stem + ".receipt.json")
        receipt_path.write_bytes(json.dumps(mutated_receipt, sort_keys=True).encode("utf-8"))
        negative_argv += ["--receipt", str(receipt_path)]
    code, violations, problem = _oracle_cli_reading(negative_argv)
    if problem is not None:
        return f"the negative fixture: {problem}"
    cited = sorted({violation["rule"] for violation in violations})
    if code != 1 or cited != [rule]:
        return (f"the negative fixture turned {cited} red instead of only {rule} (exit {code}): "
                + "; ".join(f"{violation['rule']} line {violation['line']} — {violation['detail']}"
                            for violation in violations))
    code, violations, problem = _oracle_cli_reading(negative_argv + ["--disable-rule", rule])
    if problem is not None:
        return f"the negative fixture with `--disable-rule {rule}`: {problem}"
    if code != 0 or violations:
        cited = sorted({violation["rule"] for violation in violations})
        return (f"the negative fixture with `--disable-rule {rule}` is still not green (exit {code}, "
                f"citing {cited}): "
                + "; ".join(f"{violation['rule']} line {violation['line']}"
                            for violation in violations))
    return None


def oracle_mutation_selftest() -> int:
    """The registry's closure, executed: the declared rule set against the mutation corpus.

    One line per fixture pair says its three arms held; a failure names the rule and what disagreed, and
    the exit code is 0 only when every arm of every pair held. The run creates one scratch directory and
    removes it again; it writes nothing outside it and touches no fixture. A mutation whose subject the
    fixture no longer holds raises rather than editing nothing — a case that silently mutated nothing
    would read as a rule nothing can break.
    """
    failures: list[str] = []
    declared, mutated = set(ORACLE_RULES), set(ORACLE_MUTATIONS)
    for rule in sorted(declared - mutated):
        failures.append(f"{rule}: declared in ORACLE_RULES with no fixture pair in ORACLE_MUTATIONS")
    for rule in sorted(mutated - declared):
        failures.append(f"{rule}: a fixture pair names an id ORACLE_RULES does not declare")
    checked = 0
    work = Path(tempfile.mkdtemp(prefix="ccc-oracle-selftest-"))
    try:
        for rule in sorted(declared & mutated):
            mutations = ORACLE_MUTATIONS[rule]
            if not mutations:
                failures.append(f"{rule}: declared with an empty fixture list")
                continue
            for mutation in mutations:
                checked += 1
                problem = _oracle_selftest_one(rule, mutation, work)
                if problem is None:
                    print(f"oracle.selftest: {rule} — {mutation.label}: positive PASS, negative FAIL "
                          f"({rule} cited), --disable-rule PASS")
                else:
                    failures.append(f"{rule}: {mutation.label}: {problem}")
    finally:
        shutil.rmtree(work, ignore_errors=True)
    if failures:
        for failure in failures:
            print(f"oracle.selftest: FAIL — {failure}")
        print(f"oracle.selftest: FAIL — {len(failures)} failure(s), {checked} fixture pair(s) run")
        return 1
    print(f"oracle.selftest: PASS — {len(declared)} rule(s), {checked} fixture pair(s): positive "
          f"PASS, negative FAIL (each citing its own rule), --disable-rule PASS")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--codex", default="codex", help="the Codex binary to verify against")
    parser.add_argument("--session", default=SOURCE_SESSION,
                        help=f"the Claude fixture session id to convert (default: {SOURCE_SESSION}, the "
                             f"one with a compaction boundary; {TOOLING_SESSION} has none, so a resumed "
                             f"turn sees its tool calls)")
    parser.add_argument("--codex-home", type=Path, help="Codex data directory to use (default: a temporary one)")
    parser.add_argument("--resume-prompt", help="run one real turn in the installed session with this prompt")
    parser.add_argument("--repeat-resume-prompt", help="run one real turn in the SECOND session — the one "
                        "converted after the source copy grew — with this prompt")
    parser.add_argument("--model-provider", help="provider the throw-away Codex config should name "
                        "(default: the host configuration's own answer)")
    parser.add_argument("--model", help="model the throw-away Codex config should name")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    parser.add_argument("--validate", type=Path, metavar="ARTIFACT",
                        help="validate one session artifact against the frozen offline oracle — the "
                             "container rules and, with --receipt, the artifact's receipt — and exit "
                             "(no harness, no source session)")
    parser.add_argument("--receipt", type=Path, metavar="DOCUMENT",
                        help="with --validate: the frozen artifact's receipt, stating its byte count, "
                             "whole-file SHA-256 and one anchor per line")
    parser.add_argument("--disable-rule", action="append", default=[], metavar="RULE_ID",
                        help="with --validate: drop exactly this rule's violations from the reading, as "
                             "if the rule were not stated (repeatable); an id the registry does not "
                             "declare is refused")
    parser.add_argument("--selftest-mutations", action="store_true",
                        help="prove the rule registry against its mutation corpus and exit: for every "
                             "declared rule id its positive fixture passes, its mutation fails citing "
                             "that rule alone, and the same bytes pass with --disable-rule for it — "
                             "each arm through this command's own --validate path, under a scratch "
                             "directory")
    args = parser.parse_args(argv)

    if args.selftest_mutations:
        if args.validate is not None or args.receipt is not None or args.disable_rule:
            parser.error("--selftest-mutations runs the whole corpus and takes none of --validate, "
                         "--receipt or --disable-rule")
        return oracle_mutation_selftest()
    if args.receipt is not None and args.validate is None:
        parser.error("--receipt names the receipt of the artifact --validate is given")
    if args.disable_rule and args.validate is None:
        parser.error("--disable-rule turns a rule off for --validate, which names the artifact")
    if args.validate is not None:
        disabled = frozenset(args.disable_rule)
        unknown = sorted(disabled - set(ORACLE_RULES))
        if unknown:
            print("oracle: INCOMPLETE — --disable-rule names no rule this oracle declares: "
                  + ", ".join(unknown), file=sys.stderr)
            return 2
        return oracle_report(args.validate, args.receipt, args.json, disabled)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    candidates = sorted((fixture_root / SOURCE_PROVIDER).glob(f"*/{args.session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {SOURCE_PROVIDER} fixture named {args.session}.jsonl below "
              f"{fixture_root / SOURCE_PROVIDER}, found {len(candidates)}", file=sys.stderr)
        return 2
    source = candidates[0]
    fragment = repo / GROWTH_ROOT / GROWTH_TURN
    if not fragment.is_file():
        print(f"the committed growth fragment {fragment} is missing: this harness needs the bytes the "
              f"source copy grows by between the two conversions", file=sys.stderr)
        return 2
    codex = shutil.which(args.codex)
    if codex is None:
        print(f"{args.codex} is not on PATH: this harness verifies native behaviour, which needs the "
              f"target harness installed", file=sys.stderr)
        return 2
    # The Codex judgment is this binary's, and the owner's answer is to follow `latest`: no version is
    # pinned anywhere, so each run records what it resolved — the binary's own words and its digest.
    codex_digest = sha256(Path(codex)) if Path(codex).is_file() else "unreadable"
    version = run([codex, "--version"], cwd=repo)
    codex_version = (last_meaningful_line(version.stdout or version.stderr)
                     if version.returncode == 0 else "unreadable")
    oracle_fixture_path = ORACLE_FIXTURE_ROOT / ORACLE_ARTIFACT
    if not oracle_fixture_path.is_file():
        print(f"the committed oracle fixture {oracle_fixture_path} is missing: the disclosure arm "
              f"needs the bytes the harness answers for", file=sys.stderr)
        return 2
    go = shutil.which("go")
    if go is None:
        print("go is not on PATH: this harness builds ccc with the Go toolchain", file=sys.stderr)
        return 2

    scratch = Path(tempfile.mkdtemp(prefix="ccc-native-check-"))
    # The source session this run converts is a copy under the scratch directory, never the committed
    # fixture: the second conversion needs a source that grew, and a committed file is not this run's to
    # write. The copy keeps the project directory's name, because that is how a Claude Code transcript is
    # spelled on disk and how the provider resolves it.
    source_root = scratch / COPY_ROOT
    copy = copy_of_fixture(source, source_root)
    home = args.codex_home.resolve() if args.codex_home else scratch / "codex-home"
    home.mkdir(parents=True, exist_ok=True)
    seeded_report = ""
    if not args.codex_home:
        # The target's loader checks a session against the provider its *configuration* names, so a bare
        # home would be answered by the harness's own default and the load check would be about a home the
        # caller does not have. The throw-away home is therefore seeded with the settings that answer that
        # question and nothing else: the host's `config.toml` itself is never copied, because a
        # configuration is where credentials live (provider authorization headers, an MCP server's env
        # table, a token in a URL), and the report says what was copied and what was left behind.
        host_home = Path(os.environ.get("CODEX_HOME") or (Path.home() / ".codex"))
        host_config = host_home / "config.toml"
        overrides = {}
        if args.model_provider:
            overrides["model_provider"] = args.model_provider
        if args.model:
            overrides["model"] = args.model
        seeding = seed_configuration(host_config.read_text() if host_config.is_file() else "", overrides)
        if seeding.text:
            (home / "config.toml").write_text(seeding.text)
        if host_config.is_file():
            seeded_report = f"seeded from: {host_config} — {seeding.report()}"
        elif seeding.text:
            seeded_report = (f"seeded from: the command line (no configuration at {host_config}) — "
                             f"{seeding.report()}")
    sessions = home / "sessions" / "2026" / "01" / "01"
    sessions.mkdir(parents=True, exist_ok=True)
    unrelated = sessions / UNRELATED_NAME
    probe = sessions / ORACLE_PROBE_NAME
    if unrelated.exists():
        print(f"{unrelated} already exists: this harness plants its own fabricated session under that "
              f"name and will not overwrite one that is there", file=sys.stderr)
        return 2
    if probe.exists():
        print(f"{probe} already exists: this harness plants the oracle's frozen artifact under that "
              f"name and will not overwrite one that is there", file=sys.stderr)
        return 2
    try:
        unrelated.write_text(json.dumps({"timestamp": "2026-01-01T00:00:00Z", "type": "session_meta",
                                         "payload": {"id": "unrelated", "cwd": "/tmp"}}) + "\n")
        probe.write_bytes(oracle_fixture(ORACLE_ARTIFACT))

        print(f"repo:        {repo}")
        print(f"codex:       {codex} ({codex_version}, sha256 {codex_digest})")
        print(f"codex home:  {home}")
        if seeded_report:
            print(seeded_report)
        print(f"source:      {source.relative_to(repo)}")
        print(f"copy:        {copy.relative_to(scratch)} (the fixture's bytes; this is what grows)")
        print(f"growth:      {fragment.relative_to(repo)} ({fragment.stat().st_size} bytes)")
        print(f"scratch:     {scratch}")
        print()

        fixture_before = tree_digest(fixture_root)
        fragment_before = sha256(fragment)
        copy_before = sha256(copy)
        copy_size_before = copy.stat().st_size
        unrelated_before = sha256(unrelated)
        # Every session the home already held — the caller's own, when --codex-home names a real home — as
        # the dashed id its file name ends with, and its bytes: a conversion must not reuse an id and must
        # not touch a file it did not write. The two fabrications this harness planted are in the id list
        # (a conversion must not reuse their ids either) but not in the byte comparison, which is about the
        # files this run did not write.
        existing_ids = ["-".join(path.stem.rsplit("-", 5)[-5:]) for path in home.rglob("rollout-*.jsonl")]
        existing_sessions = {path: sha256(path) for path in home.rglob("rollout-*.jsonl")
                             if path not in (unrelated, probe)}
        build = run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"],
                    cwd=repo / "ccc")
        if build.returncode != 0:
            record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
            return 2
        record("ccc builds with the flags upstream ships", True)

        # The disclosure arm, side by side: the offline rules accept these bytes — they are a container,
        # not a Codex session — and the harness's own loader answers for them. `record` says the pair
        # held only where the harness refused: a binary that accepted the artifact turns this line red,
        # which is the signal that the disclosure below the fold needs re-reading.
        offline = validate_artifact(oracle_fixture(ORACLE_ARTIFACT))
        refused, answer = resume_answer(codex, home, ORACLE_PROBE_ID, probe)
        record("the harness refuses an artifact the offline rules accept",
               not offline and refused,
               f"the offline rules: {'green' if not offline else sorted({v.rule for v in offline})}; "
               f"{answer}")

        env = {"CLAUDE_PROJECTS_DIR": str(source_root)}
        convert = run([
            str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{args.session}", "--to", "codex",
            "--target-home", str(home), "--target-binary", codex, "--json",
        ], cwd=repo, env=env)
        document: dict = {}
        if convert.returncode != 0:
            # The CLI writes its JSON document before it decides its exit code, so a failed verification
            # still says which state failed; the harness's own stderr is only noise around that.
            try:
                document = json.loads(convert.stdout)
            except json.JSONDecodeError:
                document = {}
            detail = json.dumps(document.get("verification", {}))[:500] if document else \
                last_meaningful_line(convert.stderr)
            record("ccc continue converts, installs and verifies", False, detail)
            return 1
        document = json.loads(convert.stdout)

        conversion = document["conversion"]
        installation = document["installation"]
        target = document["target_session"]
        verification = document["verification"]
        thread_id = target["session_id"]
        installed = Path(target["path"])

        record("the conversion reports carried evidence and a non-zero loss category",
               conversion["state"] == "converted" and conversion["carried"]["messages"] > 0
               and any(loss["count"] > 0 for loss in conversion["losses"]),
               f"{conversion['carried']['messages']} message(s), {conversion['carried']['tool_calls']} "
               f"tool call(s), {len(conversion['losses'])} loss categories")
        # The source boundary, against this harness's own reading of the file: the conversion's report is
        # the claim, and the bytes here are what it is checked against.
        boundary_ok, boundary_detail = boundary_matches(document, copy)
        record("the conversion reports the boundary of the source copy it read", boundary_ok, boundary_detail)
        record("the source copy is unchanged by the conversion",
               sha256(copy) == copy_before and copy.stat().st_size == copy_size_before,
               f"sha256 {copy_before[:16]}…, {copy_size_before} bytes, taken before and after")
        day_directory = installed.parent.relative_to(home / "sessions")
        record("the session is installed under the target's own sessions tree",
               installation["state"] == "installed" and installed.is_file()
               and len(day_directory.parts) == 3 and all(part.isdigit() for part in day_directory.parts),
               f"{installed.relative_to(home)} [{installation['mechanism']}]")
        record("the target identity is new and is not the source's",
               thread_id != args.session and thread_id not in existing_ids,
               f"{thread_id}, not one of the {len(existing_ids)} id(s) already in the home")
        record_installed_jsonl(installed, "the installed file is one JSON object per line",
                               " (the target's own read of it is the load check)")
        read_back, readback = inspect_counts(repo, home, thread_id)
        carried = conversion["carried"]
        agreed, detail = readback_verdict(thread_id, read_back, carried)
        if readback.returncode != 0:
            detail += f"; inspect rc={readback.returncode}: {last_meaningful_line(readback.stderr)}"
        record("the inherited reader reads the installed file back as what was written", agreed, detail)

        view, item_kinds = load_and_view(
            codex, home, thread_id, installed,
            "the target harness's own loader opens the installed session",
            "the target harness's own view of the installed session:")
        record("the target's own view lists the converted turns",
               "userMessage" in item_kinds and "agentMessage" in item_kinds,
               f"{len(item_kinds)} item(s) in the reply's own turn list")

        # The reported invocation, checked against the directory this run installed into: Codex takes
        # its data directory from CODEX_HOME alone, so a resume command that does not name it opens
        # whichever home the caller's shell points at. Both rendered lines are read the way a shell
        # reads them -- the words they are, and the spelling that decides whether a shell reads the
        # opening word as that assignment at all (a quoted assignment reads back as the same words and
        # is a program to a shell) -- so a home the renderer quotes (a path with a space, a Windows
        # path) is accepted, while an unquoted home a shell would split is not.
        resume_arguments = target.get("resume_arguments") or []
        exec_resume_arguments = target.get("exec_resume_arguments") or []
        resume_environment = target.get("resume_environment") or {}
        resume_command = str(target.get("resume_command") or "")
        exec_resume_command = str(target.get("exec_resume_command") or "")
        want_resume = [codex, "resume", thread_id]
        want_exec_resume = [codex, "exec", "resume", thread_id, PROMPT_PLACEHOLDER]
        record("the reported resume command selects the installation home",
               resume_arguments == want_resume and exec_resume_arguments == want_exec_resume
               and resume_environment == {"CODEX_HOME": str(home)}
               and reported_invocation(resume_command, home, want_resume)
               and reported_invocation(exec_resume_command, home, want_exec_resume),
               f"resume_command={resume_command!r} exec_resume_command={exec_resume_command!r} "
               f"environment={resume_environment}")

        if args.resume_prompt:
            print()
            run_turn(repo, codex, home, installed,
                     turn_identity(codex, thread_id, exec_resume_arguments),
                     args.resume_prompt, "a real turn continues from the installed session")
        else:
            record("a real turn continues from the installed session", False,
                   "not attempted: pass --resume-prompt to run one", skipped=True)

        # ---------------------------------------------------------------- the source grows, and is
        # converted again. Everything above is the first conversion's own answer; what follows is the
        # goal's repeatable-handoff half: the source gains one turn, the same session is converted a
        # second time into the same home, and both results are held to their own bytes.
        first_installed_digest = sha256(installed)
        appended = appended_texts(fragment)
        if not appended:
            record("the growth fragment records dialogue to look for", False,
                   f"{fragment.relative_to(repo)} holds no message text")
            return 2
        # The baseline the growth is measured from, compared against the bytes taken *before* the first
        # conversion and therefore answering for the conversion, the loader and -- when it ran -- the real
        # turn: without this the growth record's size delta would accept a same-length change made during
        # the first native stage (the size still matches and the digest still differs) and adopt the
        # changed bytes as the baseline every comparison below uses, so the run's own "the source session
        # is unchanged" verdict could not answer for that interval. A conversion's read is over by now
        # either way, so the comparison is recorded and the run continues: what it catches is stated as
        # its own failure rather than folding into the growth record, whose size claim is still true.
        copy_at_growth = sha256(copy)
        copy_grew_from_baseline = copy_at_growth == copy_before \
            and copy.stat().st_size == copy_size_before
        record("the source copy still holds the bytes the first conversion read",
               copy_grew_from_baseline,
               f"sha256 {copy_at_growth[:16]}…, {copy.stat().st_size} bytes, against the "
               f"{copy_before[:16]}…, {copy_size_before} bytes taken before the first conversion")
        grown_size = append_fragment(copy, fragment)
        grown_digest = sha256(copy)
        record("the source copy grew by exactly the appended turn",
               grown_size == copy_size_before + fragment.stat().st_size and grown_digest != copy_before,
               f"{copy_size_before} -> {grown_size} bytes (+{fragment.stat().st_size} from "
               f"{fragment.relative_to(repo)}), sha256 {grown_digest[:16]}…")

        grown = run([
            str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{args.session}", "--to", "codex",
            "--target-home", str(home), "--target-binary", codex, "--json",
        ], cwd=repo, env=env)
        second_document: dict = {}
        if grown.returncode != 0:
            try:
                second_document = json.loads(grown.stdout)
            except json.JSONDecodeError:
                second_document = {}
            record("ccc continue converts the grown source, installs and verifies", False,
                   json.dumps(second_document.get("verification", {}))[:500] if second_document
                   else last_meaningful_line(grown.stderr))
            return 1
        second_document = json.loads(grown.stdout)
        second_conversion = second_document["conversion"]
        second_target = second_document["target_session"]
        second_thread_id = second_target["session_id"]
        second_installed = Path(second_target["path"])
        second_boundary_ok, second_boundary_detail = boundary_matches(second_document, copy)
        record("the second conversion reports the boundary of the grown source copy",
               second_boundary_ok
               and boundary_of(second_document).get("sha256") != boundary_of(document).get("sha256"),
               f"{second_boundary_detail}; the first conversion's boundary was "
               f"{json.dumps(boundary_of(document))}")
        record("the second conversion's identity is new, not the source's and not the first session's",
               second_thread_id not in (args.session, thread_id) and second_thread_id not in existing_ids
               and second_installed != installed,
               f"{second_thread_id} at {second_installed.relative_to(home)}")
        record_installed_jsonl(second_installed,
                               "the second installed file is one JSON object per line")
        gained = (second_conversion["carried"]["user_messages"]
                  > conversion["carried"]["user_messages"]
                  and second_conversion["carried"]["assistant_messages"]
                  > conversion["carried"]["assistant_messages"])
        first_text, second_text = installed.read_text(), second_installed.read_text()
        carried_texts = all(text in second_text for text in appended) \
            and not any(text in first_text for text in appended)
        record("the second conversion carries the history the source gained",
               gained and carried_texts,
               f"carried user messages {conversion['carried']['user_messages']} -> "
               f"{second_conversion['carried']['user_messages']}, assistant messages "
               f"{conversion['carried']['assistant_messages']} -> "
               f"{second_conversion['carried']['assistant_messages']}; the appended turn's own words are "
               f"in the second document and not in the first")
        record("the first conversion's session is unchanged by the second",
               installed.is_file() and sha256(installed) == first_installed_digest,
               f"sha256 {first_installed_digest[:16]}… before and after the second conversion")
        record("the source copy is unchanged by the second conversion",
               sha256(copy) == grown_digest and copy.stat().st_size == grown_size,
               f"sha256 {grown_digest[:16]}…, {grown_size} bytes, taken before and after the second "
               f"conversion")

        # The inherited reader's own second read, and the target harness's own second load: this run's
        # own stub-free answers about the later snapshot.
        second_counts, second_readback = inspect_counts(repo, home, second_thread_id)
        second_carried = second_conversion["carried"]
        second_agreed, second_detail = readback_verdict(second_thread_id, second_counts, second_carried)
        record("the inherited reader reads the second installed file back as what was written",
               second_readback.returncode == 0 and second_agreed, second_detail)

        second_view, second_kinds = load_and_view(
            codex, home, second_thread_id, second_installed,
            "the target harness's own loader opens the second installed session",
            "the target harness's own view of the second installed session:")
        second_view_text = "\n".join(second_view)
        record("the target's own view of the second session lists the appended turn",
               all(text in second_view_text for text in appended)
               and "userMessage" in second_kinds and "agentMessage" in second_kinds,
               f"{len(second_kinds)} item(s) in the reply's own turn list; the appended turn's own words "
               f"are in its own view")
        record("the first session's own view does not hold the appended turn",
               bool(item_kinds) and not any(text in "\n".join(view) for text in appended),
               f"the first session's own view holds {len(item_kinds)} item(s), none of them the turn the "
               f"source gained after it was converted")

        if args.repeat_resume_prompt:
            print()
            run_turn(repo, codex, home, second_installed,
                     turn_identity(codex, second_thread_id,
                                   second_target.get("exec_resume_arguments") or []),
                     args.repeat_resume_prompt, "a real turn continues from the second installed session")
        else:
            record("a real turn continues from the second installed session", False,
                   "not attempted: pass --repeat-resume-prompt to run one", skipped=True)
        print()
        print(f"conversions: 2 session(s) installed ({thread_id} then {second_thread_id}); real turn(s) "
              f"run: {int(bool(args.resume_prompt)) + int(bool(args.repeat_resume_prompt))}")

        # Every promise about files this run must not change is answered here, after the loader and —
        # when it ran — the real turn: a comparison recorded beside the read-back above would let a
        # mutation during either native stage stand on an earlier verdict, and the planted session is
        # only deleted once this block has run.
        #
        # The check below is the scenario's "the source is unchanged" promise, and the source this run
        # actually converted is the *copy*: its digest was measured before the second conversion ("the
        # source copy is unchanged by the second conversion" answers for that stage) and is measured once
        # more here, after the second loader and the optional turn — otherwise a mutation of the copy
        # during the second native stage would leave every check green while this check's own name said
        # the source had survived the whole repeated transfer. The other end of that interval is the
        # comparison taken where the copy grew ("the source copy still holds the bytes the first
        # conversion read"): it is what makes the state this check compares with — grown_digest — the
        # baseline the first conversion's own read produced rather than whatever the bytes had become
        # while that conversion's session was loaded and, with --resume-prompt, resumed.
        fixture_after = tree_digest(fixture_root)
        moved = sorted([name for name, digest in fixture_before.items()
                        if fixture_after.get(name) != digest]
                       + [name for name in fixture_after if name not in fixture_before])
        fragment_unchanged = sha256(fragment) == fragment_before
        copy_unchanged = (copy.is_file() and sha256(copy) == grown_digest
                          and copy.stat().st_size == grown_size)
        if not moved and fragment_unchanged and copy_unchanged:
            detail = (f"sha256 {sha256(source)[:16]}… (fixture tree identical, "
                      f"{fragment.relative_to(repo)} identical; the converted copy identical too: "
                      f"sha256 {grown_digest[:16]}…, {grown_size} bytes)")
        elif moved:
            detail = f"the fixture tree changed: {', '.join(moved)}"
        elif not fragment_unchanged:
            detail = f"{fragment.relative_to(repo)} changed"
        elif copy.is_file():
            detail = (f"the converted source copy changed after the second conversion: want sha256 "
                      f"{grown_digest[:16]}…, {grown_size} bytes, found sha256 {sha256(copy)[:16]}…, "
                      f"{copy.stat().st_size} bytes")
        else:
            detail = (f"the converted source copy is gone after the second conversion: it was "
                      f"sha256 {grown_digest[:16]}…, {grown_size} bytes")
        record("the source session is unchanged",
               not moved and fragment_unchanged and copy_unchanged, detail)
        planted_unchanged = unrelated.is_file() and sha256(unrelated) == unrelated_before
        record("the unrelated session in the target home is unchanged",
               planted_unchanged,
               f"sha256 {unrelated_before[:16]}…" if planted_unchanged
               else f"sha256 {unrelated_before[:16]}… before, "
                    + (f"{sha256(unrelated)[:16]}… after" if unrelated.is_file() else "missing after"))
        # Every session file the home held before the conversion, compared again now: the planted
        # unrelated one is checked above, and this covers a real --codex-home's own sessions too.
        changed = [str(path.relative_to(home)) for path, digest in existing_sessions.items()
                   if not path.is_file() or sha256(path) != digest]
        record("no pre-existing session in the target home changed",
               not changed,
               f"{len(existing_sessions)} pre-existing session file(s)"
               + (f"; changed: {', '.join(sorted(changed))}" if changed else ""))

        failures = [name for name, state, _ in RESULTS if state == "FAIL"]
        print()
        print(f"{len([r for r in RESULTS if r[1] == 'PASS'])} check(s) passed, "
              f"{len([r for r in RESULTS if r[1] == 'FAIL'])} failed, "
              f"{len([r for r in RESULTS if r[1] == 'SKIP'])} skipped")
        if args.json:
            print(json.dumps({"results": [{"check": n, "state": s, "detail": d} for n, s, d in RESULTS],
                              "target_session": thread_id, "installed": str(installed),
                              "second_session": second_thread_id, "second_installed": str(second_installed),
                              "codex": codex, "codex_home": str(home),
                              "codex_version": codex_version, "codex_sha256": codex_digest}, indent=2))
        return 1 if failures else 0
    finally:
        # The fabricated unrelated session and the disclosure arm's planted artifact belong to this
        # harness and no one else: they are removed even from a home the caller named, so --codex-home
        # is left as it was found apart from the converted session this run installed.
        unrelated.unlink(missing_ok=True)
        probe.unlink(missing_ok=True)
        if args.keep:
            print(f"kept {scratch} and {home}")
        else:
            shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
