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

**The frozen artifact oracle (goal 0009a).** The same file carries a second, offline half: a
well-formedness oracle over a Codex session **artifact** — `validate_artifact` below, reachable as
`--validate <artifact> [--receipt <document>]` — which needs no harness, no source transcript and no
selection semantics, because its input is the artifact and, at most, an explicit manifest receipt. It is
what goal 0009's later increments read an installed rollout with, and its rule table, its fixtures'
mutations and this mode are **read-only for the rest of goal 0009**: a round that moves this checker
alongside production code moves both sides of "the implementation satisfies the oracle" at once, which is
the oracle lock (goal 0009a, D4; stated in full at the section itself). The native checks above are
untouched by it — they are the acceptance path this increment must leave reporting the same verdict.

    python3 scripts/codex-native-check.py
    python3 scripts/codex-native-check.py --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
        --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --repeat-resume-prompt "What budget do retries share per session?"
    python3 scripts/codex-native-check.py --json          # machine-readable summary on stdout
    python3 scripts/codex-native-check.py --validate bin/rollout.jsonl --receipt plan.json --json

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
import datetime
import hashlib
import json
import os
import queue
import re
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
FIXTURE_ROOT = Path("internal", "ccc", "testdata", "fixtures")
GO_TAGS = "fts5"
BINARY = Path("bin", "ccc-native-check")

#: The committed growth fragment: the bytes this harness appends to its *copy* of the fixture between the
#: two conversions, so the source session gains a turn without the committed fixture tree (which this run
#: hashes) moving. It is generated and pinned by `internal/ccc/fixturegen`, the same way the fixtures are.
GROWTH_ROOT = Path("internal", "ccc", "testdata", "growth")
GROWTH_TURN = "claude-complete-turn.jsonl"

#: The provider root the copy is written under: a Claude Code transcript is resolved through the directory
#: its project-dir spelling names, and this run drives the conversions with `CLAUDE_PROJECTS_DIR` pointing
#: at its scratch root rather than at the committed fixtures.
COPY_ROOT = Path("source")

#: The unrelated session the harness plants in the target home before converting: a conversion must not
#: touch a session another agent may already have continued.
UNRELATED_NAME = "rollout-2026-01-01T00-00-00-00000000-0000-7000-8000-000000000000.jsonl"

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
                raise RuntimeError(f"{method} failed: {message['error'].get('message')}")
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


# ===================================================================================================
# The frozen Codex artifact oracle (goal 0009a).
#
# The native checks above ask the target harness itself. This section answers the other, offline half:
# whether a **Codex session artifact** is well-formed — including one a selection reduced — judged from
# the artifact's own bytes and, at most, an explicit **manifest receipt** (the plan's per-occurrence
# dispositions). It never reads a source transcript, never re-derives Claude Code structure or selection
# semantics, and needs no harness: `validate_artifact` takes the artifact's text and nothing else, so "a
# reduced artifact is validated with the source absent from the process" is a property of what it *can*
# read rather than a promise about its behaviour.
#
# **Freeze and oracle lock (goal 0009a, D4).** This section, the rule table and `--validate` are read-only
# for the rest of goal 0009, and the reason is mechanical rather than moral: this checker is the verifier
# a later lane's implementation is judged by, so a fix round that moves it *alongside* production code
# moves both sides of "the implementation satisfies the oracle" at once and buys a reviewer round instead
# of a convergence — measured on goal 0009's superseded attempt, where that co-mutation is the root cause
# both cross-family consults named. The conductor reads that shape as the **oracle lock** and blocks the
# series; the rule set, its fixtures and its mutations are the contract the later lanes read, not a draft
# they may tune. After goal 0009, a format change (a new record type, say) is an ordinary increment: add
# the rule or the record type, and add the mutation that turns its fixture red — the suite's bijection
# between `ORACLE_RULES` and its mutation table is what keeps that honest, because a rule without a red
# mutation, or a mutation without a rule, fails the required `selftest` step.
#
# The rules are statements about the *format*, measured on Codex 0.170.0's own sessions (`~/.codex/
# sessions`, 25 files) and on artifacts this build's writer installs; each rule's own mutation, in
# `scripts/tests/test_codex_native_check.py`, is what makes it a claim a case can break rather than an
# aspiration. They are the artifact's rules and never the source's: which records a source contributed,
# which a selection kept and what the target planned are the receipt's own statements, and the oracle only
# compares a receipt with the artifact it plans. The structural half is deliberately silent about a call
# whose recorded result was never written — the writer retires those by design
# (`TestCodexRolloutRetiresACallWhenItsResultIsOmitted`), so the artifact alone cannot tell "the source
# recorded no result" from "the result was left out": that answer is the receipt's own, and it is the pair
# rule below that refuses a plan which splits a pair. Where a receipt *is* supplied, the artifact is read
# against it in both of the channels a dialogue occurrence lives in — the model-visible `response_item`
# and the turn-view `event_msg` — and in the receipt's own order: a message the plan omits may not survive
# in the view, one it emits has to stand in both, a call or one of its outputs substituted under an
# unchanged `call_id` is not the occurrence the plan named, the turn view's own dialogue order is the
# receipt's order too (the harness builds its view of the turns from those events), and a compaction
# boundary moved past later turns is refused although every count is unchanged. The receipt's vocabulary
# names no turn, so which `turn_context` record a record stands after is outside what the two documents
# can state and is not compared: a `turn_context` moved across records — the read's active turn id, and
# so the turn each later record reads under, changes with it (`internal/parser/codex.go`) — leaves every
# checked sequence and count unchanged, and the rule states that scope rather than claiming a comparison
# it cannot make (review round r7's finding). A receipt that declares
# no occurrences at all is refused rather than compared, because both comparisons would pass vacuously on
# an artifact that holds none. A `compacted` record is a boundary occurrence whether or not it carries a
# summary, because the carried reader appends a boundary for every one of them
# (`internal/parser/codex.go`'s `handleCompacted`), so no boundary can stand outside the plan by carrying
# no `message`; and where a plan is compared, a boundary that carries a native `replacement_history` is
# refused as well, because dialogue that member holds is dialogue the plan — whose boundary identity is
# the summary alone — cannot account for, and this route's writer emits only the summary. The same holds
# for every `response_item` a read turns into an occurrence the receipt's vocabulary cannot name: an
# artifact's `session_meta` naming an agent path plus an inter-agent `agent_message` addressed to it is a
# user message to the carried reader (`internal/parser/codex.go`'s `handleAgentMessage`), a payload type
# the reader's switch does not take but which carries a `user`/`assistant` role and readable text is a
# message to its default branch, an `image_generation_call` is a tool call whose identity lives in
# members the receipt does not have, and a `user` message whose content the read consumes as a subagent
# notification (`handleSubagentNotification`) never travels as a message — the read attributes its
# delivered text to a result event the artifact states in no record the receipt can name — so each is
# refused where a plan is compared rather than certified (review rounds r4 and r9's findings). Before
# that refusal, an omitted prompt written into one of those shapes travelled through receipt validation
# unnoticed (review round r4's finding). A second class is the shape
# whose own read or identity this oracle does not model: a call carrying **both** an `arguments` and an
# `input` member can be named `"{}"` by a receipt while the carried reader's `extractCodexInputJSON` skips
# that value and continues to `input`; a call whose recorded input or result whose recorded text is
# written as a member that is **not a string** is matched by the identity's JSON spelling while the
# reader answers the member's own raw bytes (a decoded spelling cannot be recovered, and the receipt's
# identity members are strings); and an object-valued `content` is iterated by the reader's own `ForEach`
# over its **member values** — `{"block":{"type":"input_text","text":…}}` is read by the carried reader and
# empty to this oracle's text readers — so each is refused where a plan is compared (review rounds r5
# and r7's findings): the identity this oracle computes is not the answer a read gives, and no receipt
# can account for the difference. A third class is the result whose read the receipt cannot establish at
# all: an output whose call the read answers elsewhere — `spawn_agent`'s output links a subagent session
# and appends no result, `wait`/`wait_agent` answer per-agent results from the output's own `status`
# object (the exec-cell poll, which the read routes as an ordinary result, excepted) — and a record that
# carries no output a read appends a result from are refused the same way, because a receipt marking such
# a result emitted would certify evidence no read of the artifact returns (review round r7's finding),
# and so is a duplicate the read suppresses rather than appends: `hasEquivalentCallResultEvent` compares
# an event's status and its `strings.TrimSpace` content, so two result records under one call id whose
# recorded texts trim to one are one result to every read, and a receipt naming both certifies a
# multiplicity no read of the artifact answers (review round r8's finding); and two shapes the read's
# own bookkeeping places outside what this oracle models are refused beside them — a repeated output
# under a call whose written name is `apply_patch`, which the read expands into one call per file the
# patch names (`codexApplyPatchFiles`) and attaches by pending position and fallback, so whether a
# repetition is suppressed or kept is not answered, and an output whose written `call_id` is the
# Go-trimmed spelling of two distinct calls the artifact carries, which the read's pending memory
# (`rememberToolCall` keys the entry by the Go-trimmed id and replaces it; `forgetToolCall` clears one
# by the written id it is handed) leaves without an established routing (review round r11's findings). A
# `reasoning` item is not one of these: the reader answers it as a thinking row with an
# explicitly empty dialogue projection, so it carries no dialogue. The structural half refuses an output
# written before its owner in either mode: the target's own reader keeps such an event as an unattached
# update rather than a result on its call (`internal/parser/codex_collecting_sink.go`), and this route's
# writer never emits one there.
# ===================================================================================================

#: Every rule the oracle states: rule id → the sentence it states. The ids are this report's own
#: vocabulary and the keys of the tests' mutation table, which is asserted to be in bijection with this
#: table, so a rule cannot be added without a fixture that turns it red and vice versa.
ORACLE_RULES: dict[str, str] = {
    "oracle.jsonl-objects":
        "every non-empty line of the artifact is exactly one JSON object, every member name written once "
        "inside it, and no string it holds carrying an unpaired surrogate escape: a repeated name (this "
        "oracle's decoder keeps the last value, the carried reader's `gjson` the first) and a "
        "`\\ud800`-style escape (this oracle's decoder answers the code point, `gjson` U+FFFD, measured "
        "on the carried v1.20.0) each make the line one two JSON readers answer differently for, so such "
        "a line is refused rather than read as either answer",
    "oracle.envelope":
        "every record carries a parseable `timestamp`, a non-empty `type` and an object `payload`",
    "oracle.record-type":
        "every record's `type` is one the rollout format defines — the five this route's writer emits "
        "and the further ones Codex 0.170.0 writes; payload types stay open",
    "oracle.session-meta":
        "the artifact's first record is its `session_meta`, and the artifact carries exactly one",
    "oracle.session-meta-members":
        "that record's `id`, `originator` and `cli_version` are non-empty strings",
    "oracle.clock":
        "the envelope instants never decrease: no written record is stamped before the record written "
        "before it",
    "oracle.message":
        "a `response_item` message's `role` is a non-empty string and its `content` is a non-empty list "
        "of blocks typed as text the carried reader reads (`input_text`, `output_text`, `text`), at "
        "least one of them non-blank",
    "oracle.event-message":
        "a `user_message`/`agent_message` event carries a non-empty string `message` — the record the "
        "harness's own view of the turn is built from",
    "oracle.call-pairing":
        "every call's `call_id` is a non-empty string no other call of the artifact carries, the call "
        "carries the non-empty `name` the carried reader registers a call for — a record whose name is "
        "not a non-empty string is no call of the artifact, so an output naming it attaches to nothing "
        "— and every call output names a call the artifact carries before it",
    "oracle.receipt":
        "a manifest receipt is an object whose `occurrences` are entries with a `kind` of this route's "
        "occurrence vocabulary, the identity that kind needs — a tool call's `call_id`, `name` and "
        "recorded input (the receipt's `arguments` member, whichever of the artifact's `arguments`/`input` "
        "members held it), a tool result's `call_id` and recorded text — and a `disposition` of `emitted` "
        "or `omitted`; a receipt that declares no occurrences is refused rather than compared, and an "
        "unreadable receipt is reported and not compared with the artifact",
    "oracle.plan-emitted":
        "every occurrence the receipt marks `emitted` is in the artifact as many times as it is marked "
        "— a dialogue message in both of the target's channels for it: the model-visible "
        "`response_item` and the turn-view `event_msg`",
    "oracle.plan-accounted":
        "the artifact holds nothing the receipt does not mark `emitted`: an occurrence the receipt marks "
        "`omitted`, one it names nowhere, a compaction boundary's `replacement_history`, a "
        "`response_item` a read turns into an occurrence the receipt's vocabulary cannot name — the "
        "inter-agent `agent_message` addressed to the session's own agent path, any other payload type "
        "whose `user`/`assistant` role and readable text a read appends as a message, an "
        "`image_generation_call`, a `message` in a role outside the writer's two carrying readable text, "
        "a `user` message a read consumes as a subagent notification (the record never travels as a "
        "message; the read attributes its text to a result the artifact states in no record), a "
        "message a read's own text extraction does not answer as the record's written bytes — a `user` "
        "message the user-text preprocessing drops as a composed system envelope, the "
        "subagent-notification tag among them, its body parsing or not, a block whose envelope prefix "
        "it strips, and an empty string the read drops from the join — and a `user` message whose "
        "first block carries the plugin-discovery envelope the read strips only on the session's first "
        "user record, a state this oracle does not model (each refused rather than read under either "
        "answer), and a "
        "`custom_tool_call_output` whose carried read answers the result's outcome — and a shape "
        "whose own read or identity this oracle does not model — a call "
        "carrying both `arguments` and `input` (a read skips an empty or `{}`/`[]` `arguments` and "
        "continues to `input`), a call whose recorded input or result whose recorded text is written as "
        "a member that is not a string (the receipt's identity member is a string, and a non-string "
        "member's own bytes are what the carried read answers), and an object-valued `content` a read "
        "iterates the member values of "
        "(`ForEach`) — are equally refused, and so is a result the carried read does not establish: one "
        "whose call the read answers elsewhere (`spawn_agent`'s output links a subagent session and "
        "appends no result; `wait`/`wait_agent` answer per-agent results from the output's own `status` "
        "object — the exec-cell poll reads as an ordinary result and is exempt, and the family is "
        "decided by the name the read registers for the call, Go-trimmed, so a padded `wait_agent` is "
        "answered by its trimmed spelling) or one that carries no "
        "output a read appends a result from, or one the carried read suppresses as equivalent — the "
        "same status and recorded text after its own trim — to the result the artifact writes before "
        "it, and a repetition under a call whose written name is `apply_patch` is refused beside them, "
        "because the read expands that call into one call per file the patch names "
        "(`codexApplyPatchFiles`) and this oracle does not model which of those calls an output "
        "attaches to, so whether the repetition is suppressed or kept is not answered; and so is an "
        "output whose written `call_id` is already the Go-trimmed spelling of two distinct calls the "
        "artifact carries — `rememberToolCall` remembers calls by the trimmed id and replaces the "
        "pending entry `toolCallNameForOutput` routes the output through, so which call, or which arm, "
        "a read answers for the output is not established (the trimmed spelling alone: a padded "
        "output id is answered by the name written beside its own call) — each refused where a "
        "comparison runs and silent without one; the plan is total, in both of the target's dialogue "
        "channels",
    "oracle.plan-order":
        "the occurrences the receipt marks `emitted` stand in the artifact in the receipt's own order, "
        "and so do its dialogue messages in the artifact's turn view — a compaction boundary moved past "
        "later turns, or two prompts in each other's turns, compacts a different history even though "
        "every count is unchanged; the receipt's vocabulary names no turn, so which `turn_context` "
        "record a record stands after is outside what a receipt can state and not compared",
    "oracle.plan-pairs":
        "a call and its recorded result carry one disposition: a `call_id` whose call and result the "
        "receipt splits is the pair the closure rule forbids",
}

#: The record types the rollout format defines: the five this build's writer emits and the three more
#: that Codex 0.170.0 writes of its own (measured: `token_usage_record`, `world_state`,
#: `inter_agent_communication_metadata`). A type outside this set is refused rather than skipped — an
#: artifact holding one carries records this build cannot say it understands. Payload types are *not*
#: closed: the harness's own event and item vocabulary is open and measured (`token_count`,
#: `thread_settings_applied`, …).
ORACLE_RECORD_TYPES = (
    "session_meta", "turn_context", "response_item", "event_msg", "compacted",
    "token_usage_record", "world_state", "inter_agent_communication_metadata",
)

#: The content-block types the inherited reader reads text from (`internal/parser/codex.go`'s
#: `extractCodexTextBlocks`): a block of any other type is skipped, so a message holding only such blocks
#: is one no read of it sees at all.
ORACLE_TEXT_BLOCK_TYPES = ("input_text", "output_text", "text")

#: The message roles this route's occurrence vocabulary names: the two its writer writes.
ORACLE_MESSAGE_ROLES = ("user", "assistant")

#: The two ways a manifest receipt can dispose of one occurrence.
ORACLE_DISPOSITIONS = ("emitted", "omitted")

#: The occurrence kinds the receipt statements are about — what this route's writer expresses as
#: evidence. A record of another kind is not an occurrence of this vocabulary rather than a silently
#: accepted one.
ORACLE_OCCURRENCE_KINDS = ("message", "tool_call", "tool_result", "compaction_boundary")

#: The `response_item` payload types that carry a call and that carry a call output. Both families Codex
#: itself writes are named, because an artifact may hold either and a pairing defect is the same defect.
ORACLE_CALL_ITEMS = ("function_call", "custom_tool_call")
ORACLE_RESULT_ITEMS = ("function_call_output", "custom_tool_call_output")

#: The call names whose output the carried read answers through one of `handleFunctionCallOutput`'s
#: special arms rather than the ordinary one — `spawn_agent`'s output links the call to the subagent
#: session it names and appends **no** result, and `wait`/`wait_agent` read the output's own `status`
#: object and append one result per agent entry (none for an output that carries no status object) — so
#: the receipt's `call_id`/recorded-text identity is not the result a read answers for them where a plan
#: is compared (`oracle_result_read_violations`). One `wait` shape is exempt: the exec-cell poll, which
#: `handleFunctionCall` routes as `code_mode_wait` and the output switch answers through the ordinary
#: arm; `oracle_exec_cell_wait` mirrors that condition. The set is asked of the name the read
#: **remembers** the call under (`oracle_call_routing_name`: `rememberToolCall` Go-trims the emitted
#: name, so `" wait_agent "` is routed as `wait_agent`), while the written name stays the occurrence
#: identity.
ORACLE_SPECIAL_RESULT_CALLS = ("spawn_agent", "wait", "wait_agent")

#: The `event_msg` payload types that carry one message's text for the harness's own view.
ORACLE_TURN_EVENTS = ("user_message", "agent_message")

#: One RFC3339 instant, as strict as the format is: `T` (or the lowercase `t`, or the space the format
#: allows by agreement), a fraction of up to nanosecond precision — Go's own `RFC3339Nano` writes one —
#: and `Z` or a numeric offset whose own hour and minute `oracle_instant` bounds the way the format
#: bounds them (`00`–`23` hours, `00`–`59` minutes; `+99:99` is no offset). The second is bounded at
#: `59`: the instant subset this oracle reads is what the target's own writer emits, and Go's clock
#: never writes the leap second `60` — an instant reader that mapped one to the next minute would order
#: a fractional `:60` after the midnight that follows it, so `:60` is refused rather than mistranslated.
#: The match is the **whole** string (`\Z`; Python's `$` also matches just before a trailing newline, so
#: `…Z\n` must not pass as the instant) and the digits are ASCII (`[0-9]`; Python's `\d` matches every
#: Unicode decimal digit, and `int` parses those too, so a `\d` matcher would read an instant no
#: RFC3339 reader can).
ORACLE_INSTANT = re.compile(
    r"^([0-9]{4})-([0-9]{2})-([0-9]{2})[Tt ]([0-9]{2}):([0-9]{2}):([0-9]{2})"
    r"(?:\.([0-9]{1,9}))?([Zz]|[+-][0-9]{2}:[0-9]{2})\Z")


def oracle_instant(value: object) -> tuple[int, int] | None:
    """One record's `timestamp` as a comparable ``(second since the epoch, nanosecond)`` pair, or None.

    The comparison is on the tuple, never on a float: two records one nanosecond apart must order the way
    the file does, and a fraction is compared as the digits it holds rather than as the microseconds a
    host clock happens to keep. An instant outside the subset this writer emits — the leap second `60`
    among them; see `ORACLE_INSTANT` — is answered `None` and refused by `oracle.envelope`, never
    remapped onto a nearby instant the file does not hold.
    """
    if not isinstance(value, str):
        return None
    match = ORACLE_INSTANT.match(value)
    if match is None:
        return None
    year, month, day, hour, minute, second = (int(match.group(index)) for index in range(1, 7))
    if not (hour <= 23 and minute <= 59 and second <= 59):
        return None
    try:
        base = datetime.datetime(year, month, day, hour, minute, second,
                                 tzinfo=datetime.timezone.utc)
    except ValueError:
        return None  # no such calendar day
    epoch = int(base.timestamp())
    zone = match.group(8)
    if zone not in ("Z", "z"):
        offset_hour, offset_minute = int(zone[1:3]), int(zone[4:6])
        if offset_hour > 23 or offset_minute > 59:
            return None  # the format bounds an offset's own clock as it bounds the time's
        offset = offset_hour * 3600 + offset_minute * 60
        epoch -= offset if zone[0] == "+" else -offset
    return epoch, int((match.group(7) or "").ljust(9, "0"))


class Violation(NamedTuple):
    """One rule an artifact breaks: the rule's id, the record's 1-based line (0 for a receipt-side or
    whole-artifact violation) and what was found."""

    rule: str
    line: int
    detail: str


def oracle_payload(record: dict) -> dict:
    """A record's payload, or an empty mapping when it is not an object (which `oracle.envelope` names).

    Returning a mapping rather than raising is what keeps one defect to one report: a record whose
    payload is not an object is an envelope violation, not also a message or call violation about
    members that were never there.
    """
    payload = record.get("payload")
    return payload if isinstance(payload, dict) else {}


def _reject_json_constant(name: str) -> None:
    """Refuse a constant JSON does not define (`NaN`, `Infinity`, `-Infinity`), which Python accepts.

    The artifact is JSON, and the target's own reader reads JSON: a line Python's decoder would turn into
    a float the format has no spelling for is a line this oracle refuses, the way it refuses the line
    that does not parse at all.
    """
    raise ValueError(f"{name} is not a JSON constant")


class _OracleMemberNameRepeated(ValueError):
    """One JSON document's object writes a member name twice — no reader's single answer to compare.

    The message is fixed and names neither the member nor its value: a member name is a string an
    artifact or receipt controls, and this report is printed by default, so the refusal is one line the
    document cannot put text into (the boundary `oracle_value_text` and `oracle_digest` hold elsewhere).
    """

    def __init__(self) -> None:
        super().__init__("one member name is written twice")


def _oracle_unique_members(pairs: list[tuple[str, object]]) -> dict:
    """One JSON object, refused where a member name is written twice — anywhere in the document.

    Python's decoder keeps the **last** value for a repeated name and the carried reader's `gjson`
    answers the **first** (measured on `tidwall/gjson` v1.20.0: `{"text":"a","text":"b"}` reads `a`
    there), so a line whose object repeats a name is one the oracle and the read answer differently for
    — a receipt comparison over Python's reading could certify dialogue a read of the same bytes does not
    hold (review round r5's finding). The oracle therefore refuses such a line rather than comparing
    either reading, the same fail-closed stance the non-object and non-JSON arms of `oracle_records`
    take; `oracle_report` reads the receipt through this same hook, because a plan whose members collide
    is no more one plan than such a line is one record.
    """
    members: dict[str, object] = {}
    for name, value in pairs:
        if name in members:
            raise _OracleMemberNameRepeated()
        members[name] = value
    return members


def _oracle_first_members(pairs: list[tuple[str, object]]) -> dict:
    """One JSON object with a repeated member name read the way the carried reader's `gjson` reads it:
    the **first** value, where Python's default hook keeps the last.

    Used only where a *string member's own JSON* is read the way a read of the artifact reads it — the
    body of a subagent notification (`oracle_read_consumes_subagent_notification`), whose text sits
    inside a message's `content` block and reached this oracle as a decoded string, so the outer line's
    own repeated names were already refused by `_oracle_unique_members` before either reader compared
    anything. Mirroring the first-value answer here is what keeps the notification gate's reading equal
    to the read's own: a body whose object repeats a member name is not a shape this mirror may answer
    differently from `gjson` (measured on the carried v1.20.0: the first value is what a repeated name
    reads as there).
    """
    members: dict[str, object] = {}
    for name, value in pairs:
        members.setdefault(name, value)
    return members


def _oracle_lone_surrogate(value: object) -> bool:
    """Whether one parsed line holds an unpaired surrogate code point in any of its strings — member
    names included.

    JSON's `\\ud800`-style escape decodes to a lone surrogate, which this oracle's decoder answers as the
    code point itself and the carried reader's `gjson` answers as U+FFFD — the replacement
    `utf8.EncodeRune` writes for an unpaired surrogate (measured on the carried v1.20.0: `\\ud800` and
    `\\ufffd` are the same string to a read of the bytes, and a valid surrogate *pair* decodes to its
    astral code point in both readers). So a line holding one is a line two JSON readers answer
    differently for, and its strings may not be compared by either answer (`oracle_records` drops it the
    way it drops a repeated-member line).

    The walk is iterative rather than recursive (review round r10's finding): a recursive scan holds a
    frame per container level — two beside the generator a comprehension adds — so an object
    `json.loads` accepted could exhaust this interpreter's stack inside the scan and end the run with a
    `RecursionError` instead of a verdict. A stack of the values still to visit reads any depth the
    decoder answered.
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


#: The Latin-1 characters the writer's own blank test strips: `strings.TrimSpace` reads Go's
#: `unicode.IsSpace`, which holds the ASCII whitespace, NEL (U+0085) and NBSP (U+00A0). Python's
#: `str.isspace` is a wider set in Latin-1 alone — measured: the two agree on every code point outside
#: the information separators U+001C–U+001F, which Python counts as space and Go does not — and beyond
#: Latin-1 both read Unicode's `White_Space` property. Spelled out because the oracle has to ask the
#: writer's own question about a message's text (`internal/ccc/codex_target.go`: `text != "" &&
#: strings.TrimSpace(text) == ""` is what makes the writer drop a blank one), and a rule written with
#: Python's trimming would refuse an artifact this writer emits: a text of U+001C alone is dialogue.
ORACLE_LATIN1_SPACE = "\t\n\v\f\r \x85\xa0"


def oracle_is_space(character: str) -> bool:
    """Whether the writer's own `strings.TrimSpace` reads one character as space (Go's `unicode.IsSpace`:
    the Latin-1 set `ORACLE_LATIN1_SPACE` names, Unicode's `White_Space` property beyond it)."""
    return character in ORACLE_LATIN1_SPACE if ord(character) <= 0xFF else character.isspace()


def oracle_blank(text: str) -> bool:
    """What the writer's own blank test answers: `strings.TrimSpace(text) == ""`. Python's `str.strip`
    is not that predicate — it also strips U+001C–U+001F, which Go's space set does not hold."""
    return all(oracle_is_space(character) for character in text)


def oracle_go_trim(text: str) -> str:
    """What Go's own `strings.TrimSpace` answers for one string: the text without the space set
    `oracle_is_space` names at either end.

    Not `str.strip`: the carried reader trims an inter-agent `recipient` and the session's `agent_path`
    with `strings.TrimSpace` (`internal/parser/codex.go`'s `extractCodexInboundAgentMessage` and
    `handleSessionMeta`), and Python's predicate strips the information separators U+001C–U+001F Go's
    does not (the same difference `oracle_blank` states), so a trimming written with `str.strip` would
    compare two values the reader compares differently.
    """
    start, end = 0, len(text)
    while start < end and oracle_is_space(text[start]):
        start += 1
    while end > start and oracle_is_space(text[end - 1]):
        end -= 1
    return text[start:end]


def oracle_value_text(value: object) -> str:
    """One member's value as a diagnostic names it: what kind of value it is and, for the kinds that can
    hold the conversation's own words, how much of it there is — never the value itself.

    This report is printed by default, and a refused member is exactly where a malformed record can hold
    the conversation's words (a prompt written into `timestamp`, a call id written as the object a plan
    was handed): naming the field's place and the value's type is what a reader needs to find it, while
    echoing the value is what the default-logging boundary this repository holds to forbids. The same
    choice `oracle_occurrence_text` makes for occurrence identities, and `oracle_entry_identity` for the
    receipt's own `text`.
    """
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "a boolean"
    if isinstance(value, str):
        return f"a string of {len(value)} character(s)" if value else "an empty string"
    if isinstance(value, list):
        return f"an array of {len(value)} entry(ies)"
    if isinstance(value, dict):
        return f"an object of {len(value)} member(s)"
    return f"a {type(value).__name__}"


def oracle_digest(text: str) -> str:
    """One string an artifact or receipt controls, as a diagnostic names it: by a digest, never its
    contents.

    Every string one of those documents carries can hold the conversation's own words — a prompt written
    into a `call_id` is a string as readily as an object — and this report is printed by default, so a
    value that identifies a record is named by the first 12 hex digits of its SHA-256: a reader can tell
    two identities apart, or match a record against a receipt entry, without the report echoing the value
    (review round r3's finding; the object shapes are `oracle_value_text`'s).

    The encoding is strict UTF-8 with `surrogatepass`: JSON's `\\ud800`-style escape decodes to a lone
    surrogate code point, which `str.encode("utf-8")` refuses — an unmatched output whose `call_id` was
    such a string ended the run in a `UnicodeEncodeError` traceback instead of a printed verdict (review
    round r4's finding) — while `surrogatepass` writes each surrogate as its own three-byte form, which
    is injective (no non-surrogate text encodes to those bytes), so two identities stay distinguishable.
    """
    return f"sha256:{hashlib.sha256(text.encode('utf-8', 'surrogatepass')).hexdigest()[:12]}"


def oracle_records(text: str) -> tuple[list[tuple[int, dict]], list[Violation]]:
    """One artifact's records: ``(line, record)`` for every non-empty line that is one JSON object, plus
    the `oracle.jsonl-objects` violations for the lines that are not.

    Blank lines are not records and not violations: a JSONL file's own readers skip them, and a trailing
    newline is the normal end of one. Blank is the writer's own test (`oracle_blank`: `strings.TrimSpace`,
    not Python's wider `str.strip`), because a line of U+001C alone is one the target's reader tries as a
    record. Everything else must be one object per line — a line that parses as
    an array, a number or a string is a violation exactly as a line that does not parse is, because the
    target's own reader reads lines as objects, and a line whose object writes a member name twice is one
    too, because the oracle's decoder and the carried reader's `gjson` answer a repeated name differently
    (`_oracle_unique_members`) — so no line is ever compared through one reader's reading of a name a
    read of the same bytes resolves another way. A line holding an unpaired surrogate escape is refused
    and dropped the same way (`_oracle_lone_surrogate`): this decoder answers the code point and the
    carried reader's `gjson` answers U+FFFD for it, so its string members — a `call_id` among them, where
    two distinct spellings would become one id to a read while the oracle's own identity comparison kept
    them apart (review round r9's finding) — may not be compared by either answer. The split is on the
    line feed alone, never on the further
    line boundaries `str.splitlines` treats as separators (`U+0085`, `U+2028`, …): a JSON string may hold
    one of those, and a reader that split there would misread a valid line as two broken ones. Parsing
    refuses the non-JSON constants Python's decoder would accept, for the same reason.
    """
    records: list[tuple[int, dict]] = []
    violations: list[Violation] = []
    for number, line in enumerate(text.split("\n"), start=1):
        if oracle_blank(line):
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
            # `_reject_json_constant`'s own refusal (`JSONDecodeError` is a `ValueError` and is
            # answered above, so this arm is the constant the format does not hold).
            violations.append(Violation("oracle.jsonl-objects", number, f"not JSON ({broken})"))
            continue
        except RecursionError:
            # A validator that raised here would answer about this machine's stack, not about the file:
            # a line nested past what this reader can parse is a line the oracle cannot read.
            violations.append(Violation("oracle.jsonl-objects", number,
                                        "not JSON: nested too deeply to read"))
            continue
        if not isinstance(value, dict):
            violations.append(Violation("oracle.jsonl-objects", number,
                                        f"one JSON object per line, found {type(value).__name__}"))
            continue
        if _oracle_lone_surrogate(value):
            violations.append(Violation("oracle.jsonl-objects", number,
                                        "the line holds an unpaired surrogate escape (`\\ud800`-style): "
                                        "this oracle's decoder answers the code point and the carried "
                                        "reader's `gjson` answers U+FFFD for it, so two JSON readers "
                                        "answer different strings for the line — refused rather than "
                                        "read as either answer"))
            continue
        records.append((number, value))
    return records, violations


def oracle_envelope_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.envelope`: the members every record's own envelope carries."""
    violations: list[Violation] = []
    for line, record in records:
        stamp = record.get("timestamp")
        if not isinstance(stamp, str) or oracle_instant(stamp) is None:
            violations.append(Violation("oracle.envelope", line,
                                        f"timestamp is {oracle_value_text(stamp)}, not an RFC3339 "
                                        f"instant this oracle reads"))
        kind = record.get("type")
        if not isinstance(kind, str) or oracle_blank(kind):
            violations.append(Violation("oracle.envelope", line,
                                        f"type is {oracle_value_text(kind)}, not a non-empty string"))
        if not isinstance(record.get("payload"), dict):
            violations.append(Violation("oracle.envelope", line,
                                        f"payload is {oracle_value_text(record.get('payload'))}, not "
                                        f"an object"))
    return violations


def oracle_record_type_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.record-type`: the record's own type is one the format defines.

    A record whose `type` is not a non-empty string is `oracle.envelope`'s alone: this rule answers for a
    type that was stated and is none of the format's.
    """
    violations: list[Violation] = []
    for line, record in records:
        kind = record.get("type")
        if isinstance(kind, str) and not oracle_blank(kind) and kind not in ORACLE_RECORD_TYPES:
            violations.append(Violation("oracle.record-type", line,
                                        f"the record's type is {oracle_value_text(kind)}, not one the "
                                        f"format defines"))
    return violations


def oracle_session_meta_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.session-meta`: the artifact opens with its one metadata record."""
    violations: list[Violation] = []
    metas = [line for line, record in records if record.get("type") == "session_meta"]
    if not records:
        violations.append(Violation("oracle.session-meta", 0, "the artifact holds no records"))
    elif not metas or metas[0] != records[0][0]:
        line, record = records[0]
        violations.append(Violation("oracle.session-meta", line,
                                    f"the first record's type is "
                                    f"{oracle_value_text(record.get('type'))}, not 'session_meta'"))
    if len(metas) > 1:
        violations.append(Violation("oracle.session-meta", metas[1],
                                    f"the artifact carries {len(metas)} session_meta records; a rollout "
                                    f"carries one"))
    return violations


def oracle_session_meta_members_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.session-meta-members`: the members the target's own reader needs from that record."""
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "session_meta":
            continue
        payload = oracle_payload(record)
        for member in ("id", "originator", "cli_version"):
            value = payload.get(member)
            if not isinstance(value, str) or oracle_blank(value):
                violations.append(Violation("oracle.session-meta-members", line,
                                            f"session_meta.{member} is {oracle_value_text(value)}, not "
                                            f"a non-empty string"))
    return violations


def oracle_clock_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.clock`: the written instants run one way — each against the record written before it.

    A record whose instant does not parse is `oracle.envelope`'s alone and is skipped here rather than
    compared, so one malformed stamp does not make every later record look out of order; every record
    whose instant did parse becomes the next comparison's baseline, violating or not, so a violation's
    detail names the last parseable record written before it rather than the last instant that held.
    """
    violations: list[Violation] = []
    previous: tuple[str, tuple[int, int]] | None = None
    for line, record in records:
        stamp = record.get("timestamp")
        instant = oracle_instant(stamp)
        if instant is None:
            continue
        if previous is not None and instant < previous[1]:
            violations.append(Violation("oracle.clock", line,
                                        f"timestamp {stamp} precedes {previous[0]} on the record "
                                        f"before it"))
        previous = (str(stamp), instant)
    return violations


def oracle_message_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.message`: a message record is one a read of the artifact can see."""
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        if payload.get("type") != "message":
            continue
        role = payload.get("role")
        if not isinstance(role, str) or oracle_blank(role):
            violations.append(Violation("oracle.message", line,
                                        f"role is {oracle_value_text(role)}, not a non-empty string"))
        content = payload.get("content")
        if not isinstance(content, list):
            violations.append(Violation("oracle.message", line,
                                        f"content is {oracle_value_text(content)}, not a non-empty "
                                        f"list"))
            continue
        if not content:
            violations.append(Violation("oracle.message", line, "content is an empty list"))
            continue
        texts: list[str] = []
        for index, block in enumerate(content):
            if not isinstance(block, dict):
                violations.append(Violation("oracle.message", line,
                                            f"content[{index}] is not an object"))
                continue
            if block.get("type") not in ORACLE_TEXT_BLOCK_TYPES:
                violations.append(Violation("oracle.message", line,
                                            f"content[{index}].type is "
                                            f"{oracle_value_text(block.get('type'))}, not one the reader "
                                            f"reads text from"))
            text = block.get("text")
            if not isinstance(text, str):
                violations.append(Violation("oracle.message", line,
                                            f"content[{index}].text is {oracle_value_text(text)}, not a "
                                            f"string"))
            else:
                texts.append(text)
        if texts and not any(not oracle_blank(text) for text in texts):
            violations.append(Violation("oracle.message", line,
                                        "every content block's text is blank, so no read of this record "
                                        "sees a message"))
    return violations


def oracle_event_message_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.event-message`: the turn events the harness's own view of a turn is built from."""
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "event_msg":
            continue
        payload = oracle_payload(record)
        if payload.get("type") not in ORACLE_TURN_EVENTS:
            continue
        message = payload.get("message")
        if not isinstance(message, str) or oracle_blank(message):
            found = ("a blank string" if isinstance(message, str)
                     else oracle_value_text(message))
            violations.append(Violation("oracle.event-message", line,
                                        f"{payload.get('type')} message is {found}, not a non-empty "
                                        f"string"))
    return violations


def oracle_call_pairing_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.call-pairing`: call identity is stated once, and every output belongs to a call the
    artifact already carries.

    Ownership is checked in the record order, not against the artifact's whole set: an output written
    before its owner is not a result on that call for the target's own reader — `CodexCollectingSink`
    keeps such an event as an unattached update rather than a result on the call
    (`internal/parser/codex_collecting_sink.go`) — and this route's writer emits an output only after its
    owner's record (`internal/ccc/codex_target.go`), so an artifact whose output stands first holds a
    result that does not attach, although every id it names exists later in the file. The other
    direction — a call whose recorded result the conversion did not write — is deliberately not a
    structural rule (see the section header): the writer retires such calls by design, so only a plan can
    say whether a result ever existed, and `oracle.plan-pairs` answers for that. A diagnostic names a
    `call_id` by its digest, never by its contents (`oracle_digest`).

    A call is registered only where the carried reader registers one: `handleFunctionCall` reads the
    payload's `name` first and returns on an empty answer (`internal/parser/codex.go`), before it reads
    the `call_id` or appends the call, so a record whose `name` is not a non-empty string (its own
    `gjson` `.Str` answer is empty for a missing or non-string member, which is why the gate is the
    empty-string test `oracle_call_name` makes and not a trimming one) owns no output — a result under
    its id would be kept as an unattached update — and is refused here rather than registered (review
    round r6's finding). A whitespace-only string name is such a non-empty string and is registered,
    exactly as the read registers it.
    """
    violations: list[Violation] = []
    calls: dict[str, int] = {}
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        call_id = payload.get("call_id")
        if item in ORACLE_CALL_ITEMS:
            if not isinstance(call_id, str) or oracle_blank(call_id):
                violations.append(Violation("oracle.call-pairing", line,
                                            f"call_id is {oracle_value_text(call_id)}, not a non-empty "
                                            f"string"))
                continue
            if oracle_call_name(payload) == "":
                violations.append(Violation("oracle.call-pairing", line,
                                            f"the call's `name` is "
                                            f"{oracle_value_text(payload.get('name'))} — the carried "
                                            f"reader registers no call for a record without a non-empty "
                                            f"string `name` (`handleFunctionCall`), so no output "
                                            f"attaches to it"))
                continue
            if call_id in calls:
                violations.append(Violation("oracle.call-pairing", line,
                                            f"call_id {oracle_digest(call_id)} is already carried by the "
                                            f"call on line {calls[call_id]}"))
                continue
            calls[call_id] = line
        elif item in ORACLE_RESULT_ITEMS:
            if not isinstance(call_id, str):
                violations.append(Violation("oracle.call-pairing", line,
                                            f"call output's call_id is "
                                            f"{oracle_value_text(call_id)}, not a non-empty string"))
            elif call_id not in calls:
                violations.append(Violation("oracle.call-pairing", line,
                                            f"call output names call_id {oracle_digest(call_id)}, which "
                                            f"no call written before it carries"))
    return violations


def oracle_message_text(payload: dict) -> str:
    """One message's own text: the string `text` of every readable block, joined in the record's order.

    The identity a receipt and an artifact are matched on is this text. The rule is the oracle's own and
    not the inherited reader's (`extractCodexContent`) in one respect: the reader drops an empty string
    from the join and this oracle keeps every string it reads, so an artifact whose readable blocks are
    an empty string beside `hi` joins a leading newline where a read of it answers `hi`. This build's
    writer emits one message as one non-empty block, so the two rules agree on every artifact this route
    produces; the **written** text stays the occurrence identity a receipt's counts are matched by, and
    where a plan is compared the difference between it and the read's own answer is refused rather than
    certified — `oracle_message_read_text` is that read's answer, and
    `oracle_unaccounted_occurrence_violations` refuses a `message` whose read answer is not this text
    (review round r10's finding). An object-valued `content` is not read here at all — the
    carried reader's `ForEach` iterates its member values — so that shape is refused where a plan is
    compared (`oracle_unmodelled_shape_violations`) instead of being read as empty.
    """
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return ""
    return "\n".join(
        block["text"] for block in blocks
        if isinstance(block, dict) and block.get("type") in ORACLE_TEXT_BLOCK_TYPES
        and isinstance(block.get("text"), str)
    )


def oracle_reader_message_text(payload: dict) -> str:
    """One message's content as the carried reader's own join answers it: `extractCodexContent` over
    `extractCodexTextBlocks` — the `text` of every readable block that is a **non-empty** string, joined
    in the record's order with a line feed.

    Not the occurrence identity (`oracle_message_text` is that, and it keeps every string an empty one
    included, because a receipt's counts are matched by it); the difference is exactly what the two
    readers of one record see, and it is this reading the reader tests a record against
    (`handleResponseItem`'s `content`), so the notification gate below asks with this one.
    """
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return ""
    return "\n".join(
        block["text"] for block in blocks
        if isinstance(block, dict) and block.get("type") in ORACLE_TEXT_BLOCK_TYPES
        and isinstance(block.get("text"), str) and block["text"] != ""
    )


def oracle_read_consumes_subagent_notification(payload: dict) -> bool:
    """Whether the carried reader consumes a record as a subagent notification — the question
    `handleSubagentNotification` answers `true` to, after which a read of the artifact returns **no
    message** for the record.

    `handleResponseItem` reads every `user` record its response-item switch does not take through
    `extractCodexContent`, and `handleSubagentNotification` consumes it exactly where
    `parseCodexSubagentNotification` parses that content: after Go's own trim a
    `<subagent_notification>` prefix; a body that is valid JSON; an `agent_id`/`agent_path` that is a
    non-empty string after `firstNonEmpty`; and a terminal status (`completed`, `errored` or `running`)
    whose own string text is non-empty after `strings.TrimSpace`. The gates are mirrored on the members'
    own types and spellings — `gjson`'s `.Str` is empty for every non-string member, and a repeated
    member name inside the body is read the first-value way `gjson` reads it (`_oracle_first_members`) —
    so a record this answers `true` for is one no receipt's message identity can name as the record a
    read answers (review round r9's finding). A body whose reading this mirror cannot reproduce exactly
    — nesting past Python's parser — is answered as consumed, the fail-closed side
    `oracle_call_input_value` takes, because the record's read-answer as a message cannot be established
    there either.
    """
    opening = "<subagent_notification>"
    closing = "</subagent_notification>"
    trimmed = oracle_go_trim(oracle_reader_message_text(payload))
    if not trimmed.startswith(opening):
        return False
    body = trimmed[len(opening):]
    if body.endswith(closing):
        body = body[:-len(closing)]
    body = oracle_go_trim(body)
    try:
        parsed = json.loads(body, parse_constant=_reject_json_constant,
                            object_pairs_hook=_oracle_first_members)
    except json.JSONDecodeError:
        return False  # no JSON body: `gjson.Valid` answers false and the message travels
    except ValueError:
        return False  # a constant JSON has no spelling for (`NaN`): `gjson.Valid` refuses it too
    except RecursionError:
        return True  # nesting past this mirror's parser: the read's own answer is not established
    if not isinstance(parsed, dict):
        return False
    agent = ""
    for member in ("agent_id", "agent_path"):
        value = parsed.get(member)
        if isinstance(value, str):
            agent = oracle_go_trim(value)
            if agent != "":
                break
    if agent == "":
        return False
    status = parsed.get("status")
    if not isinstance(status, dict):
        return False
    for member in ("completed", "errored", "running"):
        value = status.get(member)
        if isinstance(value, str) and oracle_go_trim(value) != "":
            return True
    return False


#: The composed envelopes `stripCodexSystemPrefix` removes from the start of a user text block, and the
#: close tag each is removed through (`internal/parser/codex.go`): `# AGENTS.md` and `<INSTRUCTIONS>`
#: share `</INSTRUCTIONS>`, and `<environment_context>` closes with its own tag. Spelled out because
#: the oracle mirrors that reader function, whose prefixes and close tags are exactly these.
ORACLE_SYSTEM_PREFIXES = (
    ("# AGENTS.md", "</INSTRUCTIONS>"),
    ("<environment_context>", "</environment_context>"),
    ("<INSTRUCTIONS>", "</INSTRUCTIONS>"),
)

#: The `source="goal"` attribute `isCodexGoalContext` looks for inside a `<codex_internal_context …>`
#: open tag — Go's `(?:^|\s)source="goal"(?:\s|/|$)`, the attribute bordered by Go's own whitespace set
#: `[\t\n\f\r ]`, `/` or the tag's own end — as this oracle's mirror searches it. The set is spelled out
#: rather than written `\s`: Python's `\s` also matches U+00A0 and the other Unicode spaces, while Go's
#: `\s` is ASCII alone, so a nonbreaking space before `source="goal"` is a goal envelope to a `\s`
#: mirror and ordinary text to the carried reader, whose records must keep travelling (review round
#: r11's finding: the mirror read the block as system content and refused a matching receipt the reader
#: preserves).
ORACLE_GOAL_SOURCE = re.compile(r'(?:^|[\t\n\f\r ])' + re.escape('source="goal"')
                                + r'(?:[\t\n\f\r ]|/|$)')


def oracle_strip_codex_system_prefix(text: str) -> str:
    """What the carried reader's `stripCodexSystemPrefix` answers for one user text block: the block
    without every complete composed envelope at its start — `# AGENTS.md` through `</INSTRUCTIONS>`,
    `<environment_context>` through its own close tag, `<INSTRUCTIONS>` through `</INSTRUCTIONS>` — and
    unchanged where the envelope's close tag does not follow.

    The loop and the trimming are the reader's own: `strings.TrimLeft(content, "\r\n")` before each
    comparison (carriage returns and line feeds only — not Python's wider whitespace), the first close
    tag after the prefix (`strings.Cut`), and the remainder trimmed the same way before the next pass.
    """
    while True:
        trimmed = text.lstrip("\r\n")
        close_tag = ""
        for prefix, tag in ORACLE_SYSTEM_PREFIXES:
            if trimmed.startswith(prefix):
                close_tag = tag
                break
        if close_tag == "":
            return text
        index = trimmed.find(close_tag)
        if index < 0:
            return text
        text = trimmed[index + len(close_tag):].lstrip("\r\n")


def oracle_is_goal_context(text: str) -> bool:
    """What `isCodexGoalContext` answers for one user text block: a `/goal` continuation envelope —
    `<goal_context>` at the start after Go's own trim, or a `<codex_internal_context …>` open tag
    naming `source="goal"` — which the user-text preprocessing drops as system content."""
    trimmed = oracle_go_trim(text)
    if trimmed.startswith("<goal_context>"):
        return True
    if trimmed.startswith("<codex_internal_context"):
        open_tag, separator, _ = trimmed.partition(">")
        return separator != "" and ORACLE_GOAL_SOURCE.search(open_tag) is not None
    return False


def oracle_is_codex_system_message(text: str) -> bool:
    """What `isCodexSystemMessage` answers for one stripped user text block: whether the user-text
    preprocessing drops it.

    The prefixes are the reader's own, spelling for spelling: `# AGENTS.md`, `<environment_context>`
    and `<INSTRUCTIONS>` are compared against the **untrimmed** text, `<turn_aborted>`, `<skill>` and
    the subagent-notification tag after Go's trim, and the goal envelopes through
    `oracle_is_goal_context`. The notification tag is a prefix gate whether or not the body after it
    parses as one — `parseCodexSubagentNotification` is the separate, consuming read
    `oracle_read_consumes_subagent_notification` mirrors — which is what makes a malformed
    notification's message, the record a receipt would otherwise name, a record no read answers (review
    round r10's finding).
    """
    trimmed = oracle_go_trim(text)
    return (text.startswith("# AGENTS.md")
            or text.startswith("<environment_context>")
            or text.startswith("<INSTRUCTIONS>")
            or trimmed.startswith("<turn_aborted>")
            or trimmed.startswith("<skill>")
            or trimmed.startswith("<subagent_notification>")
            or oracle_is_goal_context(text))


def oracle_user_message_read_text(payload: dict) -> str | None:
    """The text the carried read appends for one `user` `message` record, or None where it appends no
    message at all.

    Mirrors `handleResponseItem`'s user path over a record whose message shape `oracle.message` is
    green for: the consumption gate first (`handleSubagentNotification` — the record never travels as a
    message), then `preprocessCodexUserTextBlocks(extractCodexTextBlocks(payload), initial)`: every
    non-empty readable text block with the composed envelope at its start stripped
    (`oracle_strip_codex_system_prefix`), a block dropped where Go's own trim calls the stripped text
    blank or `oracle_is_codex_system_message` answers true, and the kept blocks joined with a line feed
    — None where the join is Go-blank. The `initial` flag that gates `stripCodexRecommendedPlugins` is
    not modelled here; a record carrying the plugin-discovery envelope is refused by its own gate
    (`oracle_user_message_plugin_envelope`) rather than read under either answer. Nor is the reader's
    replay suppression modelled (`observeUserPrompt`'s state: a re-emitted **first** prompt dropped only
    after a `turn_aborted` signal and before a later user turn): that is turn state a record alone
    cannot establish, and an artifact whose duplicate first prompt the read would drop stays certifiable
    here — the boundary the increment's evidence record states among what it does not claim.
    """
    if oracle_read_consumes_subagent_notification(payload):
        return None
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return None
    kept: list[str] = []
    for block in blocks:
        if not isinstance(block, dict) or not isinstance(block.get("text"), str):
            continue
        text = block["text"]
        if text == "":
            continue
        stripped = oracle_strip_codex_system_prefix(text)
        if oracle_blank(stripped) or oracle_is_codex_system_message(stripped):
            continue
        kept.append(stripped)
    joined = "\n".join(kept)
    return None if oracle_blank(joined) else joined


def oracle_user_message_plugin_envelope(payload: dict) -> bool:
    """Whether one `user` message's first readable block carries the plugin-discovery envelope
    `stripCodexRecommendedPlugins` removes: an open tag with a close tag after it.

    That rewrite is gated on the reader's first-user flag (`preprocessCodexUserTextBlocks`'s `initial`):
    on the session's first user record the envelope is rewritten away, on a later one the block travels
    as written. Which record is that first one is state (`observeUserPrompt`, which inter-agent messages
    and turn events also advance) this oracle does not model, so the shape is refused where a plan is
    compared rather than read under either answer (review round r10's finding).
    """
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return False
    for block in blocks:
        if not isinstance(block, dict) or not isinstance(block.get("text"), str):
            continue
        text = block["text"]
        if text == "":
            continue
        start = text.find("<recommended_plugins>")
        if start < 0:
            return False
        return text.find("</recommended_plugins>", start + len("<recommended_plugins>")) >= 0
    return False


def oracle_message_read_text(payload: dict, role: str) -> str | None:
    """The text the carried read appends for one `message` record, or None where it appends none — the
    answer `oracle_message_text` is the artifact's **written** identity, and this is the read's own.

    A `user` record goes through `oracle_user_message_read_text` (the consumption gate and the
    user-text preprocessing); an `assistant` record through `extractCodexContent` — the non-empty
    readable texts joined with a line feed, no preprocessing — answered None only where Go's own trim
    calls the join blank, which a message green under `oracle.message` never is (one of its texts is
    non-blank). The difference from `oracle_message_text`, which keeps every readable string the record
    writes, is one an empty string in a block makes: the read drops it from the join, so the text it
    answers is not the record's written bytes (review round r10's finding — refused where a plan is
    compared).
    """
    if role == "user":
        return oracle_user_message_read_text(payload)
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return None
    kept: list[str] = []
    for block in blocks:
        if not isinstance(block, dict) or not isinstance(block.get("text"), str):
            continue
        text = block["text"]
        if text == "":
            continue
        kept.append(text)
    joined = "\n".join(kept)
    return None if oracle_blank(joined) else joined


def oracle_result_text(payload: dict) -> str:
    """One tool result's own text, as the occurrence identity counts it.

    A string `output` is the recorded text itself — the shape this route's writer emits, so a receipt's
    result entry and the artifact's output record are matched on the bytes the writer wrote, and a
    substituted output under an unchanged `call_id` is a different occurrence rather than the same one.
    A non-string output (Codex 0.170.0's own files hold list-valued outputs; this build's writer writes
    none) is kept as its JSON spelling, so two different outputs under one `call_id` stay two
    occurrences; an **absent** member is the empty string, and a member written as `null` is the string
    `null` — the raw text the carried reader answers for it (`parseCodexFunctionOutput` reads the
    member's own bytes) — rather than the absent member's empty string. Where a **plan** is compared, a
    non-string output is refused instead (`oracle_unmodelled_shape_violations`): the receipt's recorded
    text is a string and the decoded value cannot recover the member's own bytes (review round r7's
    finding).
    """
    if "output" not in payload:
        return ""
    output = payload["output"]
    if isinstance(output, str):
        return output
    return json.dumps(output, separators=(",", ":"), ensure_ascii=False)


def oracle_call_name(payload: dict) -> str:
    """One call's own tool name, as the occurrence identity counts it: the string `name`, and the empty
    string where the record holds none (the carried reader reads a non-string name as no name at all).

    The name is part of the identity beside the `call_id`: two invocations under one call id — a
    different tool, or the same tool with different input — are different occurrences rather than one,
    so a receipt that named the call cannot certify an invocation it did not name.
    """
    name = payload.get("name")
    return name if isinstance(name, str) else ""


def oracle_call_routing_name(name: str) -> str:
    """The name the carried read routes a call's output by: the name it remembers the call under.

    `handleFunctionCall` registers the emitted name through `rememberToolCall`, which Go-trims it, and
    `toolCallNameForOutput` answers from that remembered name before the raw `callNames` entry written
    beside it — so a name written with surrounding whitespace is routed as its trimmed spelling, the
    `wait_agent` branch for `" wait_agent "` (review round r10's finding: the padded name left the
    oracle on the ordinary arm while the read answered the call's plain-text output with **no** result,
    the wait branch's `status` gate returning before recording one). Where Go's trim leaves nothing
    (`" "`), the registration stores no name and the read falls back to the raw spelling, which is what
    this answers. The **written** name stays the occurrence identity (`oracle_call_name`), not this
    routing spelling: the read answers the call itself with the name the record wrote.
    """
    registered = oracle_go_trim(name)
    return registered if registered != "" else name


def oracle_call_input_member(payload: dict) -> tuple[str, object] | None:
    """The member `oracle_call_arguments` reads a call's identity from: the first present
    `arguments`/`input` member, a blank string skipped, and None where the record carries neither.

    The preference and the blank skip are the read's own (`extractCodexInputJSON`), and naming the
    member lets the receipt-mode refusal say which one held a value no receipt can name.
    """
    for member in ("arguments", "input"):
        if member not in payload:
            continue
        value = payload[member]
        if isinstance(value, str) and oracle_blank(value):
            continue
        return member, value
    return None


def oracle_call_arguments(payload: dict) -> str:
    """One call's own recorded input, as the occurrence identity counts it: **what the record writes** in
    its `arguments` member — the shape this route's writer emits — or, where that member is absent or
    blank, the `input` member a `custom_tool_call` carries its input in, for either call family.

    The identity is the written member and not the carried reader's projection of it (review round r4's
    finding). `internal/parser/codex.go`'s `extractCodexInputJSON` skips an empty or `{}`/`[]` value and
    answers the trimmed text, and an identity written from that read made two different records one
    occurrence: this route's writer emits `arguments: "{}"` for a call whose source recorded no input
    (`internal/ccc/codex_target.go`: `input = "{}"`, counted as `lossEmptyInputs`), so a receipt naming
    the written `"{}"` was a mismatch against the correct artifact, while a receipt using the read's
    empty identity certified both that artifact and one a mutation left with no `arguments` member at
    all. The member is read exactly as written (a blank one falls through to `input`, the member
    preference the read shares), because only then is a substituted invocation a different occurrence.

    A non-string member is kept as its JSON spelling, the same convention `oracle_result_text` states: an
    absent one is the empty string, and a member written as `null` is the string `null` the reader
    answers for it — but where a **plan** is compared that member is refused instead
    (`oracle_unmodelled_shape_violations`), because the receipt's `arguments` identity is a string and
    the decoded value cannot recover the member's own bytes, which are what the carried read answers
    (review round r7's finding). A record carrying **both** members is refused the same way: the carried
    read skips an empty or `{}`/`[]` `arguments` and continues to `input`, so the written identity and
    the read's answer can be two different invocations and one receipt cannot account for both.
    """
    read = oracle_call_input_member(payload)
    if read is None:
        return ""
    _, value = read
    if isinstance(value, str):
        return value
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False)


def oracle_boundary_text(payload: dict) -> str:
    """One compaction boundary's own text, as the occurrence identity counts it: the string `message`
    where the record carries one, and the empty string where it holds none.

    The carried reader appends a boundary for **every** `compacted` record (internal/parser/codex.go's
    `handleCompacted` reads `message` and records the boundary whether or not a summary is there), so a
    record without one is an empty boundary the artifact holds, not a record outside the occurrence
    vocabulary; counting only records with a string `message` would let such a boundary stand outside the
    plan. The record's `replacement_history` — the messages a native compaction replaced, which this
    route's writer never emits (`internal/parser/codex_fidelity_test.go`) — is not part of this identity;
    a boundary that carries one is `oracle_boundary_history_violations`' refusal where a plan is
    compared, because the receipt's boundary identity is the summary text alone and nothing else can
    account for dialogue written in that member.
    """
    message = payload.get("message")
    return message if isinstance(message, str) else ""


def oracle_boundary_history_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.plan-accounted`, for the dialogue a boundary can carry beside its summary.

    A native `compacted` record can hold the messages it replaced in `replacement_history` beside its
    `message` (`internal/parser/codex_fidelity_test.go`), and this route's writer emits only the summary:
    the receipt's boundary identity is that text, so a boundary carrying a `replacement_history` holds
    dialogue the plan cannot account for even though every identity, count and order it compares is
    unchanged (review round r3's finding). Refused where a plan is compared and silent without one: a
    native rollout this oracle is not asked to plan against may legitimately carry the member. A value
    that holds nothing — an empty list — is not dialogue and is not refused; anything else (a non-empty
    list, a string, a malformed value) is.
    """
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "compacted":
            continue
        payload = oracle_payload(record)
        if "replacement_history" not in payload:
            continue
        history = payload["replacement_history"]
        if isinstance(history, list) and not history:
            continue
        violations.append(Violation("oracle.plan-accounted", line,
                                    f"the compaction boundary carries a `replacement_history` "
                                    f"({oracle_value_text(history)}); the receipt's boundary identity is "
                                    f"the summary alone, so the plan cannot account for dialogue written "
                                    f"in that member — this route's writer emits only the summary"))
    return violations


def oracle_agent_path(records: list[tuple[int, dict]]) -> str:
    """The session's own agent path, read from the artifact's `session_meta` the way the carried reader
    reads it: `payload.agent_path`, else the `payload.source.subagent.thread_spawn.agent_path` a subagent
    session carries it in (`internal/parser/codex.go`'s `handleSessionMeta`), each through Go's own
    trimming (`oracle_go_trim`). The empty string where the artifact names none — the artifact standing
    alone is all this reads; D2's subject is that no source session is consulted.
    """
    for _, record in records:
        if record.get("type") != "session_meta":
            continue
        payload = oracle_payload(record)
        direct = payload.get("agent_path")
        candidates = [direct] if isinstance(direct, str) else []
        source = payload.get("source")
        subagent = source.get("subagent") if isinstance(source, dict) else None
        spawn = subagent.get("thread_spawn") if isinstance(subagent, dict) else None
        if isinstance(spawn, dict) and isinstance(spawn.get("agent_path"), str):
            candidates.append(spawn["agent_path"])
        for candidate in candidates:
            trimmed = oracle_go_trim(candidate)
            if trimmed != "":
                return trimmed
        return ""
    return ""


def oracle_agent_message_text(payload: dict) -> str:
    """One inter-agent message's delivered text, as the carried reader collects it
    (`extractCodexInboundAgentMessage`): the `encrypted_content` string of every block typed
    `encrypted_content`, kept where it is not empty and joined in the record's order.

    The reader's own display test for an opaque Fernet envelope (`isCodexEncryptedToolContent`) is
    deliberately **not** mirrored here: it decides what that reader chooses to show, not what the
    record's bytes can carry, and a plan that names nothing for the record cannot account for its
    delivered text either way (see `oracle_unaccounted_occurrence_violations`). An object-valued
    `content` is not read here at all — the reader's `ForEach` iterates its member values — so that shape
    is refused where a plan is compared (`oracle_unmodelled_shape_violations`) instead of being read as
    empty.
    """
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return ""
    texts: list[str] = []
    for block in blocks:
        if not isinstance(block, dict) or block.get("type") != "encrypted_content":
            continue
        text = block.get("encrypted_content")
        if isinstance(text, str) and text != "":
            texts.append(text)
    return "\n".join(texts)


#: The `response_item` payload types the carried reader takes in its own switch before its `role` branch
#: (`internal/parser/codex.go`'s `handleResponseItem`): each is read as a call, its output, a reasoning
#: row or an inter-agent message, never through the role the branch below the switch reads. The three
#: families this route's vocabulary names are not in this list because they are occurrences the receipt
#: can account for; the rest are here so a record of one of them carrying a role and text is not read as
#: a message this oracle would then have to refuse.
ORACLE_INTERCEPTED_ITEMS = (
    "function_call", "custom_tool_call", "function_call_output", "custom_tool_call_output",
    "agent_message", "image_generation_call", "reasoning",
)


def oracle_unaccounted_occurrence_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.plan-accounted`, for the occurrences a read makes that the receipt cannot name.

    The receipt's vocabulary is this route's four evidence kinds, and every identity it states is read
    from the artifact's own members. A `response_item` outside that vocabulary can still be an occurrence
    to the carried reader, and then the plan cannot account for it in any spelling — review round r4's
    finding wrote the reduced plan's omitted prompt into a `message`-shaped record declared with the
    payload type `agent_message`, and receipt validation certified it because neither accounting rule saw
    the shape at all. The shapes are enumerated rather than the refusal narrowed to the one:

    - an inter-agent `agent_message` addressed to the session's own agent path, whose delivered text
      `handleAgentMessage` appends as a **user** message (`oracle_agent_message_text` states the one
      gate of that read this oracle does not mirror);
    - a payload type the reader's switch does not take, carrying a `user`/`assistant` role and readable
      text: the reader's default branch appends it as a message (`extractCodexContent` over
      `extractCodexTextBlocks`), and the vocabulary names only the `message` item;
    - an `image_generation_call`, which `handleImageGenerationCall` appends as a tool call whose
      identity is read from the payload's `id` and raw text — members the receipt's
      `call_id`/`name`/`arguments` identity has no place for;
    - a `message` item in a role outside the vocabulary's two carrying readable text: the format's own
      message shape, which this route's writer writes only in the roles the receipt names, while
      Codex's own files carry other roles in it (§0: 57 `developer` messages across the 25 measured
      sessions) and the structural `oracle.message` rule admits any non-empty role — refused where a
      plan is compared, never in the structural rules (review round r6's finding);
    - a `custom_tool_call_output` whose read appends a result: `handleFunctionCallOutput` answers that
      result's outcome from the record's own `status`, defaulting to `completed` where it holds none,
      while this route's writer's own result record, `function_call_output`, carries no outcome member
      at all (counted `tool_result_outcomes`) — so a read of the artifact answers an outcome the
      receipt's `call_id`/recorded-text identity cannot state (review round r6's finding);
    - a `user` `message` whose content the carried read consumes as a subagent notification
      (`oracle_read_consumes_subagent_notification` mirrors `handleSubagentNotification`'s gates): the
      record never travels as a message — the read consumes it and attributes its delivered text to a
      result event on the wait call that named the agent, or, attributed to no call, to an orphan
      tool-result message — and no record of the artifact states that occurrence, so a receipt marking
      the message emitted names dialogue no read of the artifact answers (review round r9's finding);
    - a `user`/`assistant` `message` whose written text the carried read does not answer as recorded:
      the read's own text extraction drops an empty string from the join, and for a `user` record the
      user-text preprocessing drops a block it reads as a composed system envelope — the
      subagent-notification tag among them, **whether or not its body parses as one** — or strips an
      envelope prefix from a block, so the text a read answers, or the absence of a message, is not the
      record's own bytes the receipt's identity names (review round r10's finding: the malformed
      notifications the round's controls marked emitted, which no read answers as messages);
    - a `user` `message` whose first block carries the plugin-discovery envelope
      `stripCodexRecommendedPlugins` rewrites only on the session's first user record: the read's
      answer depends on which record that is, a state this oracle does not model, so the shape is
      refused rather than read under either answer (review round r10's finding).

    Each is refused where a plan is compared and silent without one: a native rollout may legitimately
    carry these shapes, and the structural rules say nothing about them (measured: 19 inter-agent
    `agent_message` records across Codex 0.170.0's own 25 host sessions, §0 of the evidence record; 57
    `developer` messages and 174 `custom_tool_call_output` records, the same section; for the
    notification arm, 0 across the same 25 sessions — the shape is the read's own
    (`internal/parser/codex_parser_test.go` pins the attribution), so refusing it where a plan is
    compared cannot refuse a measured native file). A
    `reasoning` item is deliberately not one of them: the reader answers it as a thinking row with an
    explicitly empty dialogue projection (`handleReasoning`: `DialogueText: new("")`), so it carries no
    dialogue the plan's dialogue accounting could be total over.
    """
    agent_path = oracle_agent_path(records)
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        detail = None  # the message-read-answer arms carry their whole sentence
        if item == "message":
            # The vocabulary's own message item, counted by the accounting rules where its role is one
            # of the two they name; a stated role outside them carrying readable text is refused here,
            # because the plan states the writer's own roles and this shape is the format's message
            # carrier. A role `oracle.message` itself refuses (missing, non-string, blank) is left to
            # that rule, so one defect stays one finding. A `user` record the read consumes as a
            # subagent notification is refused before any of that: the read answers no message for it.
            role = payload.get("role")
            if role == "user" and oracle_read_consumes_subagent_notification(payload):
                found = ("the artifact holds a `user` `message` whose content the carried read consumes "
                         "as a subagent notification (`handleSubagentNotification`), so a read of the "
                         "artifact answers no message for the record — the delivered text is "
                         "attributed to a result the artifact states in no record the receipt's "
                         "identity can name")
            elif role in ORACLE_MESSAGE_ROLES:
                if oracle_message_violations([(line, record)]):
                    continue  # the structural rule's own finding: one defect, one report
                if role == "user" and oracle_user_message_plugin_envelope(payload):
                    detail = ("the artifact holds a `user` `message` whose first block carries the "
                              "plugin-discovery envelope the carried read strips only on the session's "
                              "first user record (`stripCodexRecommendedPlugins`, gated on the flag "
                              "`preprocessCodexUserTextBlocks` is called with): which record that is a "
                              "state this oracle does not model, so the receipt cannot name the text a "
                              "read answers it as under either answer")
                else:
                    read_text = oracle_message_read_text(payload, role)
                    if read_text == oracle_message_text(payload):
                        continue
                    if read_text is None:
                        detail = (f"the artifact holds a `{role}` `message` whose content the carried "
                                  f"read answers as no message at all — every block is dropped as "
                                  f"system content or blank by the user-text preprocessing (the "
                                  f"subagent-notification tag among the gates, its body parsing or "
                                  f"not) — so the message the receipt names is one no read of the "
                                  f"artifact answers")
                    else:
                        detail = (f"the artifact holds a `{role}` `message` whose written text the "
                                  f"carried read does not answer as recorded: its own text extraction "
                                  f"drops or rewrites a block (`extractCodexContent`, and for a `user` "
                                  f"record `preprocessCodexUserTextBlocks` — an empty string dropped "
                                  f"from the join, a composed envelope prefix stripped), so the "
                                  f"receipt's identity, the record's own bytes, names no text a read "
                                  f"answers")
            elif not isinstance(role, str) or oracle_blank(role):
                continue
            else:
                text = oracle_message_text(payload)
                if oracle_blank(text):
                    continue
                found = (f"the artifact holds a `message` in the role {oracle_value_text(role)} carrying "
                         f"readable text ({oracle_value_text(text)}) — the vocabulary names the writer's "
                         f"own two roles, not this one")
        elif item in ORACLE_CALL_ITEMS:
            continue  # the vocabulary's own shapes; the accounting rules read them
        elif item in ORACLE_RESULT_ITEMS:
            if item != "custom_tool_call_output":
                continue  # the writer's own result family; the accounting rules read it
            if oracle_blank(oracle_result_text(payload)):
                continue  # no result text: the read appends no result with an outcome
            found = ("the artifact holds a `custom_tool_call_output`, which the carried read answers "
                     "with the result's outcome (the record's own `status`, or `completed` where it "
                     "holds none), a member this route's writer's `function_call_output` results have "
                     "no place for (counted `tool_result_outcomes`) and the receipt's "
                     "`call_id`/recorded-text identity has none either")
        elif item == "agent_message":
            recipient = payload.get("recipient")
            delivered = oracle_agent_message_text(payload)
            if (agent_path == "" or not isinstance(recipient, str)
                    or oracle_go_trim(recipient) != agent_path or oracle_blank(delivered)):
                continue
            found = (f"the artifact holds an inter-agent `agent_message` addressed to the session's own "
                     f"agent path carrying delivered text ({oracle_value_text(delivered)})")
        elif item == "image_generation_call":
            found = ("the artifact holds an `image_generation_call`, which a read appends as a tool call "
                     "whose identity (the payload's `id` and raw text) the receipt's `call_id`/`name`/"
                     "`arguments` identity has no members for")
        elif item not in ORACLE_INTERCEPTED_ITEMS:
            role = payload.get("role")
            text = oracle_message_text(payload)
            if role not in ORACLE_MESSAGE_ROLES or oracle_blank(text):
                continue
            found = (f"the artifact holds a `response_item` whose payload type is "
                     f"{oracle_value_text(item)} carrying a `{role}` role and readable text "
                     f"({oracle_value_text(text)})")
        else:
            continue
        violations.append(Violation("oracle.plan-accounted", line,
                                    detail if detail is not None else
                                    f"{found}; the receipt's vocabulary cannot name that occurrence, so "
                                    f"the plan cannot account for it — this route's writer writes no such "
                                    f"record"))
    return violations


def oracle_unmodelled_shape_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.plan-accounted`, for the shapes whose own read or identity this oracle does not model.

    The occurrence identity is the member the artifact writes, and the carried reader's answer for these
    shapes need not be that member (the `gjson` behaviour is measured on `tidwall/gjson` v1.20.0, the
    version this tree carries):

    - a call carrying **both** an `arguments` and an `input` member: `extractCodexInputJSON` skips an
      empty or `{}`/`[]` `arguments` and continues to `input`
      (`TestParseCodexSession_InputJSON`'s `string-encoded empty JSON falls through to input` subtest
      pins that read), so a receipt naming the written `"{}"` identity of this route's writer
      (`lossEmptyInputs`) can certify a call whose recorded input a read answers from the other member;
    - a call whose recorded input, or a result whose recorded text, is written as a member that is not a
      string: the identity serializes such a member back to its JSON spelling, but the carried reader
      answers the member's **own bytes** — `extractCodexInputJSON` returns the raw JSON of a non-string
      `arguments`/`input`, and `parseCodexFunctionOutput` reads a non-string `output`'s raw text — and
      this oracle holds the decoded value, from which the member's own spelling (a space after a colon,
      a number's written form) cannot be recovered. The receipt's own identity members are strings, so
      no receipt can name the invocation or the output exactly (review round r7's finding);
    - an object-valued `content` on a record a read turns into a message: `extractCodexTextBlocks` and
      `extractCodexInboundAgentMessage` run their `ForEach` over `content`, which for an object yields
      its **member values** — `{"block":{"type":"input_text","text":…}}` is read by the carried reader
      and empty to this oracle's own text readers — so the text a read delivers is accounted for by no
      identity the receipt can state.

    Each is refused where a plan is compared and silent without one: a native rollout may legitimately
    carry them (Codex 0.170.0's own files hold twelve blank call outputs and 122 list-valued
    `custom_tool_call_output` outputs, §0 of the evidence record), and the structural rules say nothing
    about a shape this route's writer does not emit and the occurrence vocabulary does not name (review
    rounds r5 and r7's findings). The content arm mirrors the read's own gates, because a record the read
    never turns into an occurrence carries no dialogue to refuse: `handleResponseItem`'s default branch
    needs a `user`/`assistant` role, and `extractCodexInboundAgentMessage` needs the session's own agent
    path as the recipient. An object-valued `content` on a `message` item is `oracle.message`'s finding
    instead — that rule requires a list — and this one leaves it alone. A record a read appends **no**
    result from is `oracle_result_read_violations`' finding, not this one's.
    """
    agent_path = oracle_agent_path(records)
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        if item in ORACLE_CALL_ITEMS and "arguments" in payload and "input" in payload:
            violations.append(Violation("oracle.plan-accounted", line,
                                        "the artifact holds a tool call carrying both `arguments` and "
                                        "`input`: the carried reader skips an empty or `{}`/`[]` "
                                        "`arguments` and continues to `input` "
                                        "(`extractCodexInputJSON`), so the recorded input a read "
                                        "answers need not be the input the receipt's single identity "
                                        "names — the plan cannot account for that invocation"))
            continue
        if item in ORACLE_CALL_ITEMS:
            read = oracle_call_input_member(payload)
            if read is not None and not isinstance(read[1], str):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the call's recorded input is written as the non-string "
                                            f"`{read[0]}` member ({oracle_value_text(read[1])}): the "
                                            f"receipt's `arguments` identity is a string, and the "
                                            f"carried reader answers the member's own bytes raw "
                                            f"(`extractCodexInputJSON`), which the decoded value this "
                                            f"oracle holds cannot recover — no receipt can name this "
                                            f"invocation exactly"))
                continue
        if item in ORACLE_RESULT_ITEMS:
            output = payload.get("output")
            if "output" in payload and not isinstance(output, str):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the result's recorded text is written as the non-string "
                                            f"`output` member ({oracle_value_text(output)}): the "
                                            f"receipt's recorded-text identity is a string, and the "
                                            f"carried reader answers the member's own bytes raw "
                                            f"(`parseCodexFunctionOutput`), which the decoded value this "
                                            f"oracle holds cannot recover — no receipt can name this "
                                            f"output exactly"))
                continue
        content = payload.get("content")
        if not isinstance(content, dict):
            continue
        if item == "agent_message":
            recipient = payload.get("recipient")
            if (agent_path == "" or not isinstance(recipient, str)
                    or oracle_go_trim(recipient) != agent_path):
                continue
            violations.append(Violation("oracle.plan-accounted", line,
                                        f"the artifact holds an inter-agent `agent_message` whose "
                                        f"`content` is {oracle_value_text(content)}; the carried reader "
                                        f"iterates an object's **member values** "
                                        f"(`extractCodexInboundAgentMessage`'s `ForEach`), so the "
                                        f"delivered text a read answers is not one this oracle's "
                                        f"identity reads — the plan cannot account for it"))
        elif item != "message" and item not in ORACLE_INTERCEPTED_ITEMS:
            role = payload.get("role")
            if role not in ORACLE_MESSAGE_ROLES:
                continue
            violations.append(Violation("oracle.plan-accounted", line,
                                        f"the artifact holds a `response_item` whose payload type is "
                                        f"{oracle_value_text(item)} with a `{role}` role and a "
                                        f"`content` that is {oracle_value_text(content)}; the carried "
                                        f"reader iterates an object's **member values** "
                                        f"(`extractCodexTextBlocks`' `ForEach`), so the message text a "
                                        f"read answers is not one this oracle's identity reads — the "
                                        f"plan cannot account for it"))
    return violations


def oracle_call_input_value(payload: dict) -> object:
    """The value the carried read parses a call's recorded input into, for the one routing decision this
    oracle mirrors (`oracle_exec_cell_wait`): the first `arguments`/`input` member that is a non-blank
    string of valid JSON or a non-empty object/array, and None where the members hold nothing a read
    parses as args.

    The parse is the read's own (`parseCodexFunctionArgs`) and deliberately strict rather than
    approximate: a member whose decoded form this parser cannot read exactly — a string that is not
    JSON, or an object that writes a member name twice, which Python and the reader's `gjson` answer
    differently (`_oracle_unique_members`) — answers None, which reads as "no args" and so refuses the
    shape instead of allowing it on a reading the reader may not share. It is **not** the occurrence
    identity (`oracle_call_arguments` reads the member as written), so only the routing decision reads
    it, and a string nested past Python's parser answers None the same way rather than escaping as a
    `RecursionError` (`test_decoded_input_never_aborts_validation`).
    """
    for member in ("arguments", "input"):
        if member not in payload:
            continue
        value = payload[member]
        if isinstance(value, str):
            trimmed = oracle_go_trim(value)
            if trimmed == "":
                continue
            try:
                return json.loads(trimmed, parse_constant=_reject_json_constant,
                                  object_pairs_hook=_oracle_unique_members)
            except (ValueError, RecursionError):
                return None
        if isinstance(value, (dict, list)):
            if not value:
                continue
            return value
        return None
    return None


def oracle_exec_cell_wait(payload: dict) -> bool:
    """Whether the carried read routes this `wait` call as an exec-cell poll — `code_mode_wait`, the one
    `wait` shape whose output the ordinary result arm answers.

    `handleFunctionCall`'s inline condition is mirrored: the call's parsed args carry a non-blank
    **string** `cell_id` (`gjson`'s `.Str` is empty for every non-string value, measured) and neither an
    `ids` nor a `targets` member (a member written as `null` is present — `gjson`'s `Exists()` is true
    for it — and so keeps the call out of this route). Args this oracle cannot read exactly answer None
    and read as no `cell_id`, so the shape is refused rather than allowed.
    """
    args = oracle_call_input_value(payload)
    if not isinstance(args, dict):
        return False
    cell_id = args.get("cell_id")
    return (isinstance(cell_id, str) and oracle_go_trim(cell_id) != ""
            and "ids" not in args and "targets" not in args)


def oracle_result_read_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.plan-accounted`, for a result record whose read the receipt cannot establish.

    A result occurrence's identity is its `call_id` and recorded text, and where a plan is compared that
    identity is a statement about the result a read of the artifact answers. The carried read's
    `handleFunctionCallOutput`, and the pending memory it routes outputs through, leave several shapes
    that are not one ordinary result, and each is refused here (review round r7's finding: the fixture's
    call renamed `wait_agent` with its output left as plain text validated green against the receipt,
    although no read of it returns the result):

    - a record that carries no output a read appends a result from — no `output` member at all, or a
      string one Go's own trim calls blank: `parseCodexFunctionOutput` returns before recording a
      result. This route's writer refuses both by its own rule (`internal/ccc/codex_target.go`:
      `lossEmptyResults` for the empty string, `lossWhitespaceResults` for text every byte of which is
      whitespace — "a written record would be in the file and absent from every read of it, including
      this conversion's own round-trip check");
    - an output whose call the read answers from the output's own `status` object: `spawn_agent`'s
      output links the call to the subagent session it names and appends **no** result, and
      `wait`/`wait_agent` append one result per agent entry — for the fixture's plain-text output,
      none — so the receipt's `call_id`/recorded-text identity is not the result a read answers. One
      `wait` shape is exempt: the exec-cell poll the read routes as an ordinary result
      (`oracle_exec_cell_wait` mirrors `handleFunctionCall`'s condition). The family is decided by the
      name the read remembers the call under, not the name as written: `rememberToolCall` Go-trims it,
      so a padded `wait_agent` is routed by the read as one (review round r10's finding —
      `oracle_call_routing_name` mirrors that answer, while the written name stays the occurrence
      identity);
    - a duplicate the read suppresses rather than appends: `CodexCollectingSink.appendToolResultEvent`
      drops an event whose status and `strings.TrimSpace` content equal an event the call already
      answers (`hasEquivalentCallResultEvent`), so two result records under one call id whose recorded
      texts trim to one are one result to every read, and a receipt naming both certifies a
      multiplicity no read of the artifact answers (review round r8's finding: the fixture's output
      duplicated under its call id with the trailing line feed removed matched two receipt entries
      while the read answers one result);
    - an output whose written `call_id` is already the Go-trimmed spelling of two distinct calls the
      artifact carries: `rememberToolCall` Go-trims the id it remembers a call under and replaces the
      pending entry, and `toolCallNameForOutput` routes a trimmed output id through that entry first,
      so the name (and arm) a read answers the output by is the last writer's — a question this oracle
      does not model, since the entry is replaced and forgotten by the reader's own order of calls and
      outputs. Refused rather than read under either answer (review round r11's finding: a `Read` call
      with id `x`, a later `wait_agent` call with id `" x "`, and a plain-text output for `x` validated
      green while the read routes the output to `wait_agent` and appends no result). Only the trimmed
      spelling can be in this state — a padded output id is answered by the name written beside its own
      call (`callNames`), which the read consults before anything else the padded id could match — so
      the refusal is scoped to it;
    - a repeated output under a call whose **written** name is `apply_patch`: `handleFunctionCall`
      expands such a call into one call per file the patch names (`codexApplyPatchFiles`), the first
      output attaches where the pending position points — the first file's call — and every later one
      falls back to the last file's call (`CodexCollectingSink`), so a repetition the read trims to one
      text may be two kept results on two calls rather than the one `hasEquivalentCallResultEvent`
      suppresses. Which of those the read answers depends on the files the patch names, a projection
      this oracle does not model, so the repetition is refused rather than answered either way (review
      round r11's finding: a two-file `apply_patch` followed by two identical outputs keeps both, while
      the per-call-id comparison read them as one suppressed duplicate). The gate is the written
      spelling, because it is the read's own: a padded `" apply_patch "` is not expanded by
      `handleFunctionCall`, so its repetition is suppressed like any ordinary call's;

    Refused only where a plan is compared: a native rollout may legitimately carry these shapes
    (measured: twelve blank outputs across Codex 0.170.0's own 25 sessions, §0 of the evidence record;
    no native session holds a call id with two outputs at all, §0), and the structural rules say
    nothing about an output's text, about the arm a read takes for its call, or about how many outputs
    one call id carries. The calls are walked in record order and an output is read against the call
    written before it, the ownership `oracle.call-pairing` states: an output naming an absent call, or
    one written before it, is that rule's finding and is left alone here.
    """
    owners: dict[str, tuple[str, bool, bool]] = {}
    #: The written id the read has remembered under each Go-trimmed id, with its line, and the first
    #: collision under one: `toolCallNameForOutput` routes a trimmed output id through the pending memory
    #: `rememberToolCall` keys by the trimmed id, so once two distinct written ids trim to one key, no
    #: output under the trimmed spelling has a routing this oracle can establish.
    trimmed_ids: dict[str, tuple[str, int]] = {}
    collisions: dict[str, tuple[str, int, str, int]] = {}
    answered: dict[str, list[tuple[int, tuple[str, str]]]] = {}
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        if item in ORACLE_CALL_ITEMS:
            call_id = payload.get("call_id")
            name = oracle_call_name(payload)
            if not isinstance(call_id, str) or call_id == "" or name == "":
                continue  # no call the read registers: the pairing rule's own finding
            trimmed = oracle_go_trim(call_id)
            carried = trimmed_ids.get(trimmed)
            if carried is None:
                trimmed_ids[trimmed] = (call_id, line)
            elif carried[0] != call_id:
                collisions.setdefault(trimmed, (carried[0], carried[1], call_id, line))
            owners[call_id] = (oracle_call_routing_name(name),
                               name == "wait" and oracle_exec_cell_wait(payload),
                               name == "apply_patch")
            continue
        if item not in ORACLE_RESULT_ITEMS:
            continue
        call_id = payload.get("call_id")
        if not isinstance(call_id, str):
            continue  # an output whose call_id is not a string: the pairing rule's own finding
        collision = collisions.get(call_id) if call_id == oracle_go_trim(call_id) else None
        if collision is not None:
            first_id, first_line, second_id, second_line = collision
            violations.append(Violation("oracle.plan-accounted", line,
                                        f"this output's call_id trims to the id of more than one call "
                                        f"the artifact carries — the calls on lines {first_line} and "
                                        f"{second_line} ({oracle_digest(first_id)} and "
                                        f"{oracle_digest(second_id)}) differ only in whitespace the "
                                        f"carried read's own trim removes — and `rememberToolCall` "
                                        f"remembers calls by the trimmed id, replacing the pending "
                                        f"entry `toolCallNameForOutput` routes this output through: "
                                        f"which call, or which arm, a read answers for this output is "
                                        f"not established, so the receipt's identity cannot name the "
                                        f"result a read returns"))
            continue
        owner = owners.get(call_id)
        if owner is None:
            continue  # an output before its call, or with none: the pairing rule's own finding
        routed, exec_cell, expandable = owner
        output = payload.get("output")
        if "output" not in payload or (isinstance(output, str) and oracle_blank(output)):
            violations.append(Violation("oracle.plan-accounted", line,
                                        "the record carries no output a read appends a result from — "
                                        "`parseCodexFunctionOutput` returns before recording one for a "
                                        "missing or blank output, the same rule this route's writer "
                                        "counts as `lossEmptyResults`/`lossWhitespaceResults` — so the "
                                        "receipt's `call_id`/recorded-text identity names no result a "
                                        "read answers"))
            continue
        if routed not in ORACLE_SPECIAL_RESULT_CALLS or exec_cell:
            # The ordinary arm's own event: its `Content` is the recorded text after Go's trim
            # (`parseCodexFunctionOutput` reads a string member trimmed, and `TrimSpace` is applied
            # again), and its `Status` is empty for the `function_call_output` family or the custom
            # family's own string `status` (`completed` where it holds none) — the two members
            # `hasEquivalentCallResultEvent` compares beside an `AgentID` this arm never sets. A
            # non-string output is left to `oracle_unmodelled_shape_violations`: its raw bytes cannot
            # be recovered from the decoded value, so no signature is computable here.
            if not isinstance(output, str):
                continue
            status = ""
            if item == "custom_tool_call_output":
                declared = payload.get("status")
                status = (declared if isinstance(declared, str) and declared != ""
                          else "completed")
            signature = (status, oracle_go_trim(output))
            held = answered.setdefault(call_id, [])
            previous_line = next((earlier for earlier, earlier_signature in held
                                  if earlier_signature == signature), None)
            if previous_line is not None:
                if expandable:
                    violations.append(Violation("oracle.plan-accounted", line,
                                                f"the artifact holds a second result under call "
                                                f"{oracle_digest(call_id)} whose recorded text is the "
                                                f"text the result on line {previous_line} answers after "
                                                f"the read's own trim, and the call's written name is "
                                                f"`apply_patch`: the carried read expands such a call "
                                                f"into one call per file the patch names "
                                                f"(`codexApplyPatchFiles`), attaches the first output "
                                                f"by the call's pending position and each later one to "
                                                f"the last file's call, so whether a read drops this "
                                                f"event as equivalent or keeps it on a distinct call is "
                                                f"a question this oracle does not model — the shape is "
                                                f"refused rather than answered either way"))
                    continue
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the artifact holds a second result under call "
                                            f"{oracle_digest(call_id)} whose recorded text is the text "
                                            f"the result on line {previous_line} answers after the "
                                            f"read's own trim (`strings.TrimSpace`): "
                                            f"`hasEquivalentCallResultEvent` suppresses this event as "
                                            f"equivalent (the same status and content), so the "
                                            f"receipt's two result identities here certify a "
                                            f"multiplicity no read of the artifact answers"))
                continue
            held.append((line, signature))
            continue
        if routed == "spawn_agent":
            found = ("the output belongs to a `spawn_agent` call, and the carried read appends **no** "
                     "result for one — it links the call to the subagent session the output names "
                     "(`agent_id`) and reads nothing else")
        else:
            found = (f"the output belongs to a `{routed}` call — the name the carried read routes it "
                     f"by — and the read answers its output from the output's own `status` object: one "
                     f"result per agent entry, none for an output that carries none")
        violations.append(Violation("oracle.plan-accounted", line,
                                    f"{found}; the receipt's `call_id`/recorded-text identity is not the "
                                    f"result a read answers"))
    return violations


def oracle_occurrences(records: list[tuple[int, dict]]) -> list[tuple[tuple, int]]:
    """The artifact's own occurrences: ``((kind, identity…), line)`` for each one it holds.

    The vocabulary is this route's four evidence kinds — a message in one of the writer's two roles, a
    tool call, a tool result and a compaction boundary. Identity is the occurrence's own: role and text
    for a message, `call_id` and the emitted tool's `name` and recorded input for a call, `call_id` and
    the recorded text for a result (two results under one call id are two occurrences), the summary's
    text for a boundary (`""` where the record carries none — every `compacted` record is an occurrence).
    Multiplicity is part of the record: two identical messages are two occurrences, and the receipt is
    counted the same way.
    """
    occurrences: list[tuple[tuple, int]] = []
    for line, record in records:
        kind = record.get("type")
        payload = oracle_payload(record)
        if kind == "response_item":
            item = payload.get("type")
            if item == "message" and payload.get("role") in ORACLE_MESSAGE_ROLES:
                occurrences.append((("message", payload.get("role"), oracle_message_text(payload)), line))
            elif item in ORACLE_CALL_ITEMS and isinstance(payload.get("call_id"), str):
                occurrences.append((("tool_call", payload["call_id"], oracle_call_name(payload),
                                     oracle_call_arguments(payload)), line))
            elif item in ORACLE_RESULT_ITEMS and isinstance(payload.get("call_id"), str):
                occurrences.append((("tool_result", payload["call_id"], oracle_result_text(payload)),
                                    line))
        elif kind == "compacted":
            occurrences.append((("compaction_boundary", oracle_boundary_text(payload)), line))
    return occurrences


def oracle_view_occurrences(records: list[tuple[int, dict]]) -> list[tuple[tuple, int]]:
    """The artifact's dialogue as the harness's own turn view holds it.

    The harness's view of a turn is built from the `user_message`/`agent_message` events, so an
    occurrence of this channel is one of those events' text under the same message identity the receipt
    uses (its role is the one the event's kind stands for). A message that is not a string is
    `oracle.event-message`'s violation and is skipped here, so one malformed event is one rule's finding
    rather than two. This channel is read only where a receipt was supplied: a native rollout need not
    duplicate its dialogue between the two channels, and the structural rules say nothing about a
    correspondence the converter's own contract is what fixes.
    """
    occurrences: list[tuple[tuple, int]] = []
    for line, record in records:
        if record.get("type") != "event_msg":
            continue
        payload = oracle_payload(record)
        kind = payload.get("type")
        if kind not in ORACLE_TURN_EVENTS:
            continue
        message = payload.get("message")
        if not isinstance(message, str):
            continue
        role = "user" if kind == "user_message" else "assistant"
        occurrences.append((("message", role, message), line))
    return occurrences


def oracle_occurrence_text(key: tuple) -> str:
    """One occurrence identity, in the words a report names it by — never the text it holds.

    The default logging boundary this repository holds to (the core design: conversation bodies, tool
    arguments, prompts and reasoning are not logged by default) applies to this oracle's own diagnostics:
    an occurrence is named by its kind, how much text it carries, and — for a `call_id` the receipt and
    the artifact are matched on — its digest (`oracle_digest`), because a call id is a string an artifact
    controls and so may hold the conversation's own words (review round r3's finding). A role is the
    closed vocabulary this route writes (`user`/`assistant`), so it stands bare.
    """
    if key[0] == "message":
        return f"message {key[1]} ({len(key[2])} character(s))"
    if key[0] == "tool_result":
        return f"tool result {oracle_digest(key[1])} ({len(key[2])} character(s))"
    if key[0] == "compaction_boundary":
        return f"compaction boundary ({len(key[1])} character(s))"
    return f"tool call {oracle_digest(key[1])}"


def oracle_plan(receipt: object) -> tuple[list[tuple[tuple, str]], list[Violation]]:
    """One manifest receipt as its plan: ``(identity, disposition)`` per entry, and its own violations.

    The receipt is the plan's own statement — which occurrence travels and which does not — and an entry
    whose kind, identity or disposition cannot be read is a violation rather than an entry: a plan this
    oracle cannot read is reported and left uncompared (see `validate_artifact`), because comparing half
    of a plan with an artifact proves nothing about either half.
    """
    violations: list[Violation] = []
    if not isinstance(receipt, dict) or not isinstance(receipt.get("occurrences"), list):
        return [], [Violation("oracle.receipt", 0,
                              "the receipt is not an object carrying an `occurrences` list")]
    if not receipt["occurrences"]:
        # A plan that accounts for nothing states nothing: both accounting rules would compare empty
        # against empty and a metadata-only artifact — one holding no occurrence in either channel —
        # would pass with a receipt that planned nothing (review round r3's finding). Fail closed, the
        # way the plan states it: a receipt that declares nothing cannot pass.
        return [], [Violation("oracle.receipt", 0,
                              "the receipt declares no occurrences: a plan that accounts for nothing "
                              "cannot certify an artifact, so it is refused rather than compared")]
    plan: list[tuple[tuple, str]] = []
    for index, entry in enumerate(receipt["occurrences"]):
        where = f"occurrences[{index}]"
        if not isinstance(entry, dict):
            violations.append(Violation("oracle.receipt", 0, f"{where} is not an object"))
            continue
        kind, disposition = entry.get("kind"), entry.get("disposition")
        if disposition not in ORACLE_DISPOSITIONS:
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.disposition is {oracle_value_text(disposition)}, not "
                                        f"one of {', '.join(ORACLE_DISPOSITIONS)}"))
            continue
        if kind not in ORACLE_OCCURRENCE_KINDS:
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.kind is {oracle_value_text(kind)}, not one of "
                                        f"{', '.join(ORACLE_OCCURRENCE_KINDS)}"))
            continue
        identity = oracle_entry_identity(entry, where, violations)
        if identity is not None:
            plan.append((identity, disposition))
    return plan, violations


def oracle_entry_identity(entry: dict, where: str, violations: list[Violation]) -> tuple | None:
    """One receipt entry's own identity, appended to `violations` and answered as None when unreadable.

    A text member is read for its type alone and never echoed into the violation: this report is printed
    by default, and the receipt is where the plan's own conversation text sits. The same holds for every
    other member that cannot be read — a diagnostic names the entry's place and the value's type
    (`oracle_value_text`), never a value that could be the conversation's words.

    A tool call's identity is its `call_id` **and** the emitted tool's `name` and recorded input (the
    receipt's `arguments` member, matched against whichever of the artifact's `arguments`/`input`
    members held it): a plan that named a call and an artifact that holds a different invocation under
    the same `call_id` are two occurrences, which is what the pair's counterpart for a result already
    states.
    """
    kind = entry["kind"]
    if kind == "message":
        role, text = entry.get("role"), entry.get("text")
        if role not in ORACLE_MESSAGE_ROLES:
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.role is {oracle_value_text(role)}, not one of "
                                        f"{', '.join(ORACLE_MESSAGE_ROLES)}"))
            return None
        if not isinstance(text, str):
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.text is {oracle_value_text(text)}, not a string"))
            return None
        return ("message", role, text)
    if kind == "compaction_boundary":
        text = entry.get("text")
        if not isinstance(text, str):
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.text is {oracle_value_text(text)}, not a string"))
            return None
        return ("compaction_boundary", text)
    call_id = entry.get("call_id")
    if not isinstance(call_id, str) or oracle_blank(call_id):
        violations.append(Violation("oracle.receipt", 0,
                                    f"{where}.call_id is {oracle_value_text(call_id)}, not a non-empty "
                                    f"string"))
        return None
    if kind == "tool_result":
        # A result's identity is its call and its recorded text: two results under one call id, or one
        # output substituted for another, are different occurrences rather than one.
        text = entry.get("text")
        if not isinstance(text, str):
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.text is {oracle_value_text(text)}, not a string"))
            return None
        return (kind, call_id, text)
    if kind == "tool_call":
        # A call's identity is its call id and the invocation the plan states for it: the emitted tool's
        # name and its recorded arguments, the two members the artifact's own call record carries, so a
        # substitution under an unchanged `call_id` is a different occurrence rather than the same one.
        name, arguments = entry.get("name"), entry.get("arguments")
        if not isinstance(name, str):
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.name is {oracle_value_text(name)}, not a string"))
            return None
        if not isinstance(arguments, str):
            violations.append(Violation("oracle.receipt", 0,
                                        f"{where}.arguments is {oracle_value_text(arguments)}, not a "
                                        f"string"))
            return None
        return (kind, call_id, name, arguments)
    return (kind, call_id)


def oracle_plan_channel_violations(plan: list[tuple[tuple, str]],
                                   occurrences: list[tuple[tuple, int]],
                                   where: str,
                                   line_fallback: dict[tuple, int] | None = None) -> list[Violation]:
    """`oracle.plan-emitted` and `oracle.plan-accounted`, for one channel of the artifact.

    ``plan`` is the part of the receipt this channel can account for, ``occurrences`` what the channel
    holds, and ``where`` the channel's own words (the artifact, or its turn view), so a violation says
    which of the two places was searched. The two rules are one comparison read from both ends — how
    many occurrences of one identity the plan marks emitted against how many the channel holds — so a
    shortfall is one rule and a surplus (an occurrence the plan marks omitted, or names nowhere) is the
    other. ``line_fallback`` locates an identity another channel holds, so a turn-view shortfall points
    at the dialogue record that stands there without its view record.
    """
    violations: list[Violation] = []
    present = collections.Counter(key for key, _ in occurrences)
    lines: dict[tuple, int] = {}
    for key, line in occurrences:
        lines.setdefault(key, line)
    emitted = collections.Counter(key for key, disposition in plan if disposition == "emitted")
    omitted = collections.Counter(key for key, disposition in plan if disposition == "omitted")
    for key in sorted(set(present) | set(emitted), key=oracle_occurrence_text):
        held, marked = present.get(key, 0), emitted.get(key, 0)
        if held < marked:
            violations.append(Violation("oracle.plan-emitted",
                                        lines.get(key, (line_fallback or {}).get(key, 0)),
                                        f"{oracle_occurrence_text(key)}: the receipt marks {marked} "
                                        f"emitted, {where} holds {held}"))
        elif held > marked:
            why = (f"the receipt marks {omitted.get(key, 0)} omitted"
                   if omitted.get(key, 0) else "the receipt names no occurrence with this identity")
            violations.append(Violation("oracle.plan-accounted", lines.get(key, 0),
                                        f"{oracle_occurrence_text(key)}: {where} holds {held}, the "
                                        f"receipt marks {marked} emitted ({why})"))
    return violations


def oracle_plan_order_violations(plan: list[tuple[tuple, str]],
                                 occurrences: list[tuple[tuple, int]],
                                 view_occurrences: list[tuple[tuple, int]]) -> list[Violation]:
    """`oracle.plan-order`: the emitted occurrences stand in the receipt's own order — in the artifact
    and in the artifact's turn view.

    Counts alone cannot see a moved record, and the compaction boundary is the record whose place is its
    meaning: the target applies a boundary to the history written before it, so the same records with
    the boundary moved past later turns compact a different conversation. The dialogue's own order in the
    turn view is a second place the same defect hides: the harness's view of the turns is built from the
    `user_message`/`agent_message` events, so the same prompts read against the same model-visible
    history but written into each other's turns are a different conversation although every count is
    unchanged. Each comparison runs only where the two multisets agree — a differing multiset is the two
    accounting rules' finding, and it has no order to compare.
    """
    emitted = [key for key, disposition in plan if disposition == "emitted"]
    held = [key for key, _ in occurrences]
    if collections.Counter(emitted) != collections.Counter(held):
        return []
    for index, (planned, found) in enumerate(zip(emitted, held)):
        if planned != found:
            return [Violation("oracle.plan-order", 0,
                              f"occurrence {index + 1} of {len(emitted)}: the receipt's own order has "
                              f"{oracle_occurrence_text(planned)} here, the artifact holds "
                              f"{oracle_occurrence_text(found)}")]
    dialogue = [key for key in emitted if key[0] == "message"]
    viewed = [key for key, _ in view_occurrences]
    if collections.Counter(dialogue) != collections.Counter(viewed):
        return []
    for index, (planned, found) in enumerate(zip(dialogue, viewed)):
        if planned != found:
            return [Violation("oracle.plan-order", 0,
                              f"turn-view occurrence {index + 1} of {len(dialogue)}: the receipt's own "
                              f"order has {oracle_occurrence_text(planned)} here, the artifact's turn "
                              f"view holds {oracle_occurrence_text(found)}")]
    return []


def oracle_plan_violations(plan: list[tuple[tuple, str]],
                           occurrences: list[tuple[tuple, int]],
                           view_occurrences: list[tuple[tuple, int]]) -> list[Violation]:
    """The receipt rules over both of the artifact's dialogue channels, and the pair rule.

    The plan states one disposition per occurrence, and the artifact expresses a dialogue occurrence
    twice — the model-visible `response_item` this route's reader builds history from, and the
    `event_msg` the harness's own view of the turn is built from (`internal/ccc/codex_target.go`
    writes both for every message) — so each channel is read against the plan: an emitted message has to
    stand in both, an omitted (or unnamed) one in neither. A native rollout that duplicates no dialogue
    keeps passing, because this comparison exists only where a receipt was supplied. The pair rule is
    the plan's own statement, because only a plan can say whether a recorded result ever existed, and it
    reads every result under its call: a call and its recorded results travel together.
    """
    violations = oracle_plan_channel_violations(plan, occurrences, "the artifact")
    model_lines: dict[tuple, int] = {}
    for key, line in occurrences:
        model_lines.setdefault(key, line)
    dialogue = [entry for entry in plan if entry[0][0] == "message"]
    violations += oracle_plan_channel_violations(dialogue, view_occurrences,
                                                 "the artifact's turn view", model_lines)
    dispositions: dict[str, set[str]] = collections.defaultdict(set)
    calls: set[str] = set()
    for key, disposition in plan:
        if key[0] == "tool_call":
            calls.add(key[1])
        if key[0] in ("tool_call", "tool_result"):
            dispositions[key[1]].add(disposition)
    for call_id in sorted(calls):
        if len(dispositions[call_id]) > 1:
            violations.append(Violation("oracle.plan-pairs", 0,
                                        f"call {oracle_digest(call_id)}: the receipt gives the call and "
                                        f"its recorded result different dispositions — a call and its "
                                        f"recorded result travel together"))
    return violations


_NO_RECEIPT = object()


def oracle_reading(text: str, receipt: object = _NO_RECEIPT
                   ) -> tuple[list[tuple[int, dict]], list[tuple[tuple, int]], list[Violation]]:
    """One artifact read once: its records, its own occurrences and every rule's violations.

    `validate_artifact` and `oracle_report` both answer from this one reading, so the counts a verdict
    line prints and the verdict itself cannot disagree about the same text — the numbers come from the
    reading that was judged, not from a second parse of the same bytes. `receipt` is the supplied
    document itself, and the default is the absence of one: a supplied document that is not a plan — a
    file that reads `null`, say — is `oracle.receipt`'s finding rather than an absent receipt.
    """
    records, violations = oracle_records(text)
    violations += oracle_envelope_violations(records)
    violations += oracle_record_type_violations(records)
    violations += oracle_session_meta_violations(records)
    violations += oracle_session_meta_members_violations(records)
    violations += oracle_clock_violations(records)
    violations += oracle_message_violations(records)
    violations += oracle_event_message_violations(records)
    violations += oracle_call_pairing_violations(records)
    occurrences = oracle_occurrences(records)
    if receipt is not _NO_RECEIPT:
        plan, receipt_violations = oracle_plan(receipt)
        violations += receipt_violations
        if not receipt_violations:
            view_occurrences = oracle_view_occurrences(records)
            violations += oracle_plan_violations(plan, occurrences, view_occurrences)
            violations += oracle_plan_order_violations(plan, occurrences, view_occurrences)
            violations += oracle_boundary_history_violations(records)
            violations += oracle_unaccounted_occurrence_violations(records)
            violations += oracle_unmodelled_shape_violations(records)
            violations += oracle_result_read_violations(records)
    return records, occurrences, sorted(
        violations, key=lambda violation: (violation.line, violation.rule, violation.detail))


def validate_artifact(text: str, receipt: object = _NO_RECEIPT) -> list[Violation]:
    """Every rule this oracle states, over one artifact and — where a receipt is handed in — its plan.

    The artifact is this `text` and nothing else: no path, no source session, no selection and no
    harness, which is what lets a **reduced** artifact be validated with the source absent from the
    process. A receipt is compared only when every one of its entries could be read; an unreadable one is
    reported under `oracle.receipt` alone, because a plan nobody can read says nothing about an artifact.
    The default means no receipt was supplied; a supplied value that is not a plan is refused by that
    same rule.
    """
    return oracle_reading(text, receipt)[2]


def oracle_report(artifact: Path, receipt: Path | None, as_json: bool) -> int:
    """The oracle's own command: one artifact, at most one receipt, a verdict and an exit code.

    It reads the artifact's bytes and, when one is named, the receipt's — nothing else, ever. An input
    it cannot read at all ends as `oracle: INCOMPLETE` (exit 2), never as a pass: nothing was validated.
    Bytes that are not UTF-8 text are such an input, not something to repair with replacement
    characters, because this oracle judges the artifact's own bytes. A receipt that parses to a
    non-object (a file holding `null`) is a *supplied* document that is not a plan and is refused by
    `oracle.receipt`, not silently read as an absent receipt; a receipt whose object writes a member name
    twice is not read as *one* plan at all (`_oracle_unique_members`), so it is an unreadable document
    like any other and ends `INCOMPLETE`. The verdict line is the last one printed,
    `oracle: PASS` or `oracle: FAIL — <n> violation(s)`, and each violation is one line above it
    (`--json` adds the machine-readable document instead of them).
    """
    try:
        raw = artifact.read_bytes()
    except OSError as failure:
        print(f"oracle: INCOMPLETE — the artifact {artifact} cannot be read: {failure}", file=sys.stderr)
        return 2
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as failure:
        print(f"oracle: INCOMPLETE — the artifact {artifact} is not UTF-8 text: {failure}",
              file=sys.stderr)
        return 2
    document: object = _NO_RECEIPT
    if receipt is not None:
        try:
            raw = receipt.read_bytes()
        except OSError as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} cannot be read: {failure}",
                  file=sys.stderr)
            return 2
        try:
            document = json.loads(raw.decode("utf-8"), parse_constant=_reject_json_constant,
                                  object_pairs_hook=_oracle_unique_members)
        except UnicodeDecodeError as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} is not UTF-8 text: {failure}",
                  file=sys.stderr)
            return 2
        except ValueError as failure:
            print(f"oracle: INCOMPLETE — the receipt {receipt} cannot be read: {failure}",
                  file=sys.stderr)
            return 2
        except RecursionError:
            # The same answer an artifact line nested past reading gets (`oracle_records`): a validator
            # that raised here would answer about this machine's stack, not about the document, and an
            # exception escaping before the verdict would read as a crash rather than an unreadable
            # receipt.
            print(f"oracle: INCOMPLETE — the receipt {receipt} cannot be read: nested too deeply to read",
                  file=sys.stderr)
            return 2
    records, occurrences, violations = oracle_reading(text, receipt=document)
    if as_json:
        print(json.dumps({
            "artifact": str(artifact),
            "receipt": str(receipt) if receipt is not None else None,
            "records": len(records),
            "occurrences": len(occurrences),
            "violations": [{"rule": rule, "line": line, "detail": detail}
                           for rule, line, detail in violations],
            "verdict": "PASS" if not violations else "FAIL",
        }, indent=2))
    else:
        for rule, line, detail in violations:
            print(f"oracle: {rule}" + (f" line {line}" if line else "") + f" — {detail}")
    print(f"oracle: {'PASS' if not violations else 'FAIL'} — {len(records)} record(s), "
          f"{len(occurrences)} occurrence(s)"
          + (f", {len(violations)} violation(s)" if violations else ""))
    return 0 if not violations else 1


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
                        help="validate a Codex session artifact against the frozen well-formedness "
                             "oracle (offline: no harness, no source session) and exit")
    parser.add_argument("--receipt", type=Path, metavar="DOCUMENT",
                        help="the artifact's manifest receipt, with --validate: the occurrences a "
                             "conversion planned, compared with the artifact")
    args = parser.parse_args(argv)

    if args.receipt is not None and args.validate is None:
        parser.error("--receipt names the manifest receipt of the artifact --validate is given")
    if args.validate is not None:
        return oracle_report(args.validate, args.receipt, args.json)

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
    if unrelated.exists():
        print(f"{unrelated} already exists: this harness plants its own fabricated session under that "
              f"name and will not overwrite one that is there", file=sys.stderr)
        return 2
    unrelated.write_text(json.dumps({"timestamp": "2026-01-01T00:00:00Z", "type": "session_meta",
                                     "payload": {"id": "unrelated", "cwd": "/tmp"}}) + "\n")

    print(f"repo:        {repo}")
    print(f"codex:       {codex}")
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
    # Every session the home already held — the planted one and the caller's own, when --codex-home
    # names a real home — as the dashed id its file name ends with, and its bytes: a conversion must
    # not reuse an id and must not touch a file it did not write.
    existing_ids = ["-".join(path.stem.rsplit("-", 5)[-5:]) for path in home.rglob("rollout-*.jsonl")]
    existing_sessions = {path: sha256(path) for path in home.rglob("rollout-*.jsonl") if path != unrelated}
    try:
        build = run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
        if build.returncode != 0:
            record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
            return 2
        record("ccc builds with the flags upstream ships", True)

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
                              "codex": codex, "codex_home": str(home)}, indent=2))
        return 1 if failures else 0
    finally:
        # The fabricated unrelated session belongs to this harness and no one else: it is removed even
        # from a home the caller named, so --codex-home is left as it was found apart from the
        # converted session this run installed.
        unrelated.unlink(missing_ok=True)
        if args.keep:
            print(f"kept {scratch} and {home}")
        else:
            shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
