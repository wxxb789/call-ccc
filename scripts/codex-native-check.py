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
    python3 scripts/codex-native-check.py --validate bin/rollout.jsonl --disable-rule oracle.clock --json
    python3 scripts/codex-native-check.py --selftest-mutations

`--disable-rule <RULE_ID>` (with `--validate`) turns exactly that one declared rule off in-process —
no file is edited and no shell surgery is needed — and `--selftest-mutations` executes the registry
against its mutation corpus instead of an artifact: for every rule id `ORACLE_RULES` declares, its
positive fixture passes, its negative fixture fails citing that rule alone, and the same fixture passes
with that rule disabled — each of the three arms read through this command's own
`--validate`/`--receipt`/`--disable-rule` path, on the mutated artifact and receipt written under a
scratch directory — so the declared ids and the tested ids are one closed set. Neither mode needs
Codex, a Go toolchain or a source session.

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
import datetime
import hashlib
import io
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
# the rule or the record type, and add the mutation that turns its fixture red — the bijection between
# `ORACLE_RULES` and `ORACLE_MUTATIONS` is what keeps that honest, because a rule without a red
# mutation, or a mutation without a rule, fails `--selftest-mutations` (the same corpus the required
# `selftest` gate step runs through the suite).
#
# The rules are statements about the *format*, measured on Codex 0.170.0's own sessions (`~/.codex/
# sessions`, 25 files) and on artifacts this build's writer installs; each rule's own mutation — the
# corpus in `ORACLE_MUTATIONS` below, driven by `--selftest-mutations` and by the suite — is what makes
# it a claim a case can break rather than an aspiration. They are the artifact's rules and never the
# source's: which records a source contributed, which a selection kept and what the target planned are
# the receipt's own statements, and the oracle only compares a receipt with the artifact it plans. The
# structural half is deliberately silent about a call
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
# summary — the occurrence vocabulary counts every one of them — so no boundary can stand outside the
# plan by carrying no `message`; and where a plan is compared, a boundary that carries a native
# `replacement_history` is refused as well, because dialogue that member holds is dialogue the plan
# cannot account for — the receipt's boundary identity is the summary text alone, and this route's
# writer emits only the summary. A boundary whose `message` member is *present* but not a string is
# refused where a plan is compared too: the member's own bytes state that a summary is there and is
# not text, and naming the boundary by the empty text it reads as would certify a summary the
# artifact does not hold (an **absent** member is the empty boundary the plan can name).
#
# **What the oracle decides, and what it does not (the mechanism repair).** The oracle's subject is the
# artifact's own structure: its JSONL framing, its envelope schema, its record vocabulary, its ordering,
# its call/result identities and their pairing, and the occurrence multiplicities its records state —
# plus, where a receipt is supplied, the receipt's own declarations read against exactly those
# occurrences, in both of the dialogue channels the artifact holds them in and in the receipt's own
# order (the paragraphs above). It does **not** model what an inherited read of the artifact would
# produce: the records a read drops, rewrites, dedupes, suppresses or expands are the *reader-visible
# projection*, and that projection is the **manifest receipt's own statement**, produced by goal
# 0009c/0009d (this goal's Non-scope), never something this oracle infers from the artifact's bytes. The
# repair removed the rules that mirrored reader behaviour — a Python shadow of `internal/parser/codex.go`
# which the artifact and a receipt alone cannot reproduce — after the increment's review budget was spent
# on their divergences: a rule that re-derives a read can always be found incomplete against the next
# reader shape, because the artifact does not state what a read answers. Every rule that remains is
# decidable from the artifact's own bytes; each removed check is routed to the goal's Non-scope and
# disclosed as a residual in this increment's evidence record.
#
# The refusals that remain where a plan is compared are the shapes whose receipt-facing identity the
# artifact's own bytes cannot state: a call whose recorded input is written as two members (`arguments`
# and `input`) or as a member that is not a string, and a result whose recorded text is written as a
# non-string member (the receipt's identity members are strings, and a decoded value cannot recover the
# member's own spelling); a `compacted` record whose `message` member is present but not a string (the
# receipt's boundary identity is the summary text, and such a member states none); a `response_item`
# `message` whose `role` is a non-empty string outside the receipt's vocabulary (`user`/`assistant`),
# because the receipt names a message by its role and a record in any other role carries dialogue no
# plan entry can account for — the role is the artifact's own bytes and not a projection of a read; and
# a `session_meta` carrying `forked_from_id`, whose own statement that its history continues another
# transcript's leaves unstated which of its records are this session's own. Each is silent without a
# plan — a native rollout may carry the shape (the host's own sessions hold 57 `developer`-role
# messages, §0 of the evidence record), and the structural rules say nothing about it.
#
# The structural half refuses an output written before its owner in either mode: this route's writer
# never emits one there.
# ===================================================================================================

#: Every rule the oracle states: rule id → the sentence it states. The ids are this report's own
#: vocabulary and the keys of this file's `ORACLE_MUTATIONS`, which `--selftest-mutations` asserts to be
#: in bijection with this table, so a rule cannot be added without a fixture that turns it red and vice
#: versa.
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
        "every record's `type` is one the rollout format defines — the record vocabulary Codex 0.170.0 "
        "persists (`session_meta`, `turn_context`, `response_item`, `event_msg`, `compacted`, "
        "`token_usage_record`, `world_state`, `retained_context`, `dynamic_tools_updated`, "
        "`inter_agent_communication`, `inter_agent_communication_metadata`, `security_risk_score`, "
        "`realtime_item`), the five this route's writer emits among them; payload types stay open",
    "oracle.session-meta":
        "the artifact's first record is its `session_meta`, and the artifact carries exactly one",
    "oracle.session-meta-members":
        "that record's `id`, `originator` and `cli_version` are non-empty strings",
    "oracle.clock":
        "the envelope instants never decrease: no written record is stamped before the record written "
        "before it",
    "oracle.message":
        "a `response_item` message's `role` is a non-empty string and its `content` is a non-empty list "
        "of blocks typed as text a native message carries (`input_text`, `output_text` — `text` is a "
        "reader spelling Codex 0.170.0's content-item enum does not define, so such a line fails to "
        "decode and its rollout reader skips it), at least one of them non-blank",
    "oracle.event-message":
        "a `user_message`/`agent_message` event carries a non-empty string `message` — the record the "
        "harness's own view of the turn is built from",
    "oracle.call-pairing":
        "every call's `call_id` is a non-empty string no other call of the artifact carries, the call "
        "carries a non-empty string `name` — a record stating no name states no invocation the "
        "occurrence vocabulary can carry, so an output naming it attaches to nothing — and every call "
        "output names a call the artifact carries before it",
    "oracle.call-members":
        "a tool call states the member its native type requires — `arguments` on `function_call`, "
        "`input` on `custom_tool_call` — and a `function_call_output`/`custom_tool_call_output` states "
        "its `output`: Codex 0.170.0's own decoder requires each member on that payload type and its "
        "rollout reader skips a line that fails to decode, so a record missing one is a record the "
        "target's read does not carry while the artifact's bytes and this oracle's occurrence "
        "vocabulary can still name it",
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
        "`omitted` or names nowhere; a compaction boundary's `replacement_history` or `retained_context` "
        "(the retained families store original user and assistant message text beside the checkpoint); a "
        "record whose `type` is one the format defines while its payload carries conversation text no "
        "occurrence kind can name (`retained_context`, `inter_agent_communication`, `realtime_item`); and "
        "the shapes whose receipt-facing identity the artifact's own bytes cannot state — a call whose "
        "recorded input is written as two members (`arguments` and `input`) or as a member that is not "
        "a string, a result whose recorded text is written as a member that is not a string, a "
        "`compacted` record whose `message` member is present but not a string, a `message` whose "
        "`role` is a non-empty string "
        "outside the receipt's vocabulary, and a `session_meta` carrying `forked_from_id` — each refused "
        "where a comparison runs and silent without one, because a native rollout may carry the shape "
        "and the structural rules say nothing about it; the plan is total, in both of the target's "
        "dialogue channels",
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

#: The record types the rollout format defines: Codex 0.170.0's persisted record vocabulary — the
#: `RolloutItemWire` enum its rollout payload decodes with (`codex-rs/history/src/rollout_payload.rs`,
#: tag `type`), which holds types this host's measured 25-session sample happened not to carry
#: (`retained_context`, persisted during ordinary operation —
#: `codex-rs/core/src/session/retained_context.rs` — as well as `dynamic_tools_updated`,
#: `inter_agent_communication`, `security_risk_score` and `realtime_item`). A type outside this set is
#: refused rather than skipped — an artifact holding one carries records this build cannot say it
#: understands. Payload types are *not* closed: the harness's own event and item vocabulary is open and
#: measured (`token_count`, `thread_settings_applied`, …).
ORACLE_RECORD_TYPES = (
    "session_meta", "turn_context", "response_item", "event_msg", "compacted",
    "token_usage_record", "world_state", "retained_context", "dynamic_tools_updated",
    "inter_agent_communication", "inter_agent_communication_metadata", "security_risk_score",
    "realtime_item",
)

#: The record types the vocabulary defines whose payloads carry conversation text outside the
#: occurrence vocabulary, so no receipt entry can account for them: the retained families keep original
#: user and assistant message text (`history/src/retained_context.rs`'s `RetainedUserMessage`), an
#: agent-to-agent communication carries its message in `content`
#: (`protocol/src/protocol.rs`'s `InterAgentCommunication`), and a realtime item's
#: `transcript_segment` variant carries recorded speech text (`protocol/src/realtime.rs`). Refused
#: where a plan is compared and silent without one: the shape is the format's own, and the structural
#: rules say nothing about it.
ORACLE_UNACCOUNTABLE_RECORD_TYPES = (
    "retained_context", "inter_agent_communication", "realtime_item",
)

#: The content-block types a native message's `content` may carry: Codex 0.170.0's `ContentItem` enum
#: defines exactly these two text spellings (`codex-rs/protocol/src/models.rs`), and a block of any
#: other type makes the record fail to decode — its rollout reader skips the line. `text` is **not**
#: one of them, however a read of the bytes answers it; that spelling is `ORACLE_TEXT_BLOCK_TYPES`'.
ORACLE_MESSAGE_BLOCK_TYPES = ("input_text", "output_text")

#: The block types this oracle reads a message's text from when it counts an occurrence: the native two
#: and `text`, the further spelling the carried reader reads text from (`internal/parser/codex.go`'s
#: `extractCodexTextBlocks`). The occurrence identity is the dialogue the artifact's own bytes write as
#: a read of them names it, which is what a receipt's `text` member is matched on, so a message
#: carrying its text in a spelling the native format does not define is still counted here while
#: `oracle.message` refuses the record: the receipt comparison is not the place that decides whether
#: the artifact is a rollout the target's own reader can read.
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
    """`oracle.message`: a message record is one the native format carries and a read of it can see.

    The block types are the native ones (`ORACLE_MESSAGE_BLOCK_TYPES`): a block of any other type makes
    the record fail to decode, so a record holding one is a record the target's own reader skips — while
    `oracle_message_text` still reads the further spelling a *carried reader* answers, which is why this
    rule, not the receipt comparison, is where the unsupported spelling is refused.
    """
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
            if block.get("type") not in ORACLE_MESSAGE_BLOCK_TYPES:
                violations.append(Violation("oracle.message", line,
                                            f"content[{index}].type is "
                                            f"{oracle_value_text(block.get('type'))}, not one a native "
                                            f"message's content blocks carry — the native decoder "
                                            f"refuses the record and its rollout reader skips the "
                                            f"line"))
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

    Ownership is checked in the record order, not against the artifact's whole set: an output stands
    after the call it names, so an artifact whose output is written first holds a result that attaches
    to nothing although every id it names exists later in the file. The other direction — a call whose
    recorded result was never written — is deliberately not a structural rule (see the section header):
    the writer retires such calls by design, so only a plan can say whether a result ever existed, and
    `oracle.plan-pairs` answers for that. A diagnostic names a `call_id` by its digest, never by its
    contents (`oracle_digest`).

    A call is registered only where it states the invocation the occurrence vocabulary carries: the
    record's `name` must be a non-empty string (`oracle_call_name`'s own empty-string test, so a
    whitespace-only name is a name), and a call whose id was already carried is refused rather than
    registered again — the two identities a receipt's call entry is matched on. A record stating no
    name states no invocation, so an output naming it would attach to nothing and is refused here.
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
                                            f"{oracle_value_text(payload.get('name'))} — a record "
                                            f"stating no non-empty string `name` states no invocation "
                                            f"the occurrence vocabulary can carry, so no output "
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


def oracle_call_members_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.call-members`: a call or result states the member its own payload type requires.

    Codex 0.170.0's decoder requires `arguments` (a string) on `function_call`, `input` (a string) on
    `custom_tool_call`, and `output` on either `*_call_output`, and its rollout reader skips a line that
    fails to decode (`codex-rs/rollout/src/recorder.rs`): a record missing its member is one the
    target's read does not carry, although this oracle's occurrence vocabulary could still name it —
    from the other family's member, or from the empty text an absent member reads as. So the *presence*
    of the native member is this rule's own subject; which written member is the identity's is
    `oracle_call_arguments`' (a `function_call` carrying only `input` is natively undecodable while its
    occurrence identity is still the written string), and a member written as a non-string is refused
    where a plan is compared (`oracle_unnameable_shape_violations`), the receipt's identity members
    being strings.
    """
    violations: list[Violation] = []
    for line, record in records:
        if record.get("type") != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        if item in ORACLE_CALL_ITEMS:
            member = "arguments" if item == "function_call" else "input"
            if member not in payload:
                violations.append(Violation("oracle.call-members", line,
                                            f"the {item} states no `{member}` member: the native decoder "
                                            f"requires it on this payload type and the rollout reader "
                                            f"skips a line it cannot decode, so the artifact holds a "
                                            f"record the target's read does not carry while its "
                                            f"occurrence identity is still counted"))
        elif item in ORACLE_RESULT_ITEMS and "output" not in payload:
            violations.append(Violation("oracle.call-members", line,
                                        f"the {item} states no `output` member: the native decoder "
                                        f"requires it on this payload type and the rollout reader skips "
                                        f"a line it cannot decode, and an absent member is not the "
                                        f"written empty result the occurrence vocabulary reads it as"))
    return violations


def oracle_message_text(payload: dict) -> str:
    """One message's own text: the string `text` of every readable block, joined in the record's order.

    The identity a receipt and an artifact are matched on is this text, and it is the text the record's
    own bytes hold: every string the readable blocks carry is kept, an empty one included, so two
    records that differ only there are two occurrences. A message whose `content` is not a list is
    `oracle.message`'s finding and reads as no text here — this only computes the identity for a record
    the structural rule has already admitted. The set read here (`ORACLE_TEXT_BLOCK_TYPES`) is one
    spelling wider than the native set that rule requires: a `text` block is still the dialogue the
    artifact's bytes write as a read of them names it, so a receipt's `text` member matches it, while
    `oracle.message` refuses the record as one the native decoder cannot carry.
    """
    blocks = payload.get("content")
    if not isinstance(blocks, list):
        return ""
    return "\n".join(
        block["text"] for block in blocks
        if isinstance(block, dict) and block.get("type") in ORACLE_TEXT_BLOCK_TYPES
        and isinstance(block.get("text"), str)
    )


def oracle_result_text(payload: dict) -> str:
    """One tool result's own text, as the occurrence identity counts it.

    A string `output` is the recorded text itself — the shape this route's writer emits — so a receipt's
    result entry and the artifact's output record are matched on the bytes the writer wrote, and a
    substituted output under an unchanged `call_id` is a different occurrence rather than the same one.
    A non-string output (Codex 0.170.0's own files hold list-valued outputs; this build's writer writes
    none) is kept as its JSON spelling so two different outputs under one `call_id` stay two
    occurrences; an **absent** member is the empty string, and a member written as `null` is the string
    `null` — the member's own JSON text — rather than the absent member's empty string. Where a **plan**
    is compared, a non-string output is refused instead (`oracle_unnameable_shape_violations`): the
    receipt's recorded text is a string and a decoded value cannot recover the member's own bytes.
    """
    if "output" not in payload:
        return ""
    output = payload["output"]
    if isinstance(output, str):
        return output
    return json.dumps(output, separators=(",", ":"), ensure_ascii=False)


def oracle_call_name(payload: dict) -> str:
    """One call's own tool name, as the occurrence identity counts it: the string `name`, and the empty
    string where the record holds none (a non-string name is no name at all).

    The name is part of the identity beside the `call_id`: two invocations under one call id — a
    different tool, or the same tool with different input — are different occurrences rather than one,
    so a receipt that named the call cannot certify an invocation it did not name.
    """
    name = payload.get("name")
    return name if isinstance(name, str) else ""


def oracle_call_arguments(payload: dict) -> str:
    """One call's own recorded input, as the occurrence identity counts it: the string its `arguments`
    member writes, else the string its `input` member writes, and the empty string where it writes
    neither.

    The identity is the member the record writes, read exactly as written — the shape this route's
    writer emits is `arguments`, and the member Codex's own custom calls carry their input in is
    `input` — so a receipt naming the written member matches it, and a substituted invocation under an
    unchanged `call_id` is a different occurrence rather than the same one. A non-string member is kept
    as its JSON spelling so two different inputs stay two occurrences; where a **plan** is compared, a
    non-string member, or a record carrying both members, is refused instead
    (`oracle_unnameable_shape_violations`): the receipt's identity members are strings, and a decoded
    value cannot recover the member's own bytes.
    """
    for member in ("arguments", "input"):
        if member not in payload:
            continue
        value = payload[member]
        if isinstance(value, str):
            return value
        return json.dumps(value, separators=(",", ":"), ensure_ascii=False)
    return ""


def oracle_boundary_text(payload: dict) -> str:
    """One compaction boundary's own text, as the occurrence identity counts it: the string `message`
    where the record carries one, and the empty string where it holds none.

    The occurrence vocabulary counts **every** `compacted` record, so a record without a summary is an
    empty boundary the artifact holds, not a record outside the vocabulary; counting only records with a
    string `message` would let such a boundary stand outside the plan. A record's `replacement_history`
    is not part of this identity; a boundary carrying one is `oracle_unnameable_shape_violations`'
    refusal where a plan is compared, because the receipt's boundary identity is the summary text alone
    and the plan cannot account for what that member holds. A **present but non-string** `message` also
    reads as the empty identity here — the count stays total over every boundary — and is refused by
    that same rule where a plan is compared: the receipt naming the empty text would otherwise certify
    a summary the artifact's own bytes state is not text (an absent member is the empty boundary the
    plan can name).
    """
    message = payload.get("message")
    return message if isinstance(message, str) else ""


def oracle_unnameable_shape_violations(records: list[tuple[int, dict]]) -> list[Violation]:
    """`oracle.plan-accounted`, for the shapes whose receipt-facing identity the artifact's own bytes
    cannot state — refused where a plan is compared and silent without one.

    The occurrence vocabulary names a call by its written recorded input and a result by its written
    recorded text, and the receipt's own identity members are strings. The shapes outside what that
    identity can name, each decidable from the artifact's own bytes alone:

    - a call carrying **both** an `arguments` and an `input` member: the artifact writes two recorded
      inputs under one call id, and a receipt's single `arguments` member names one;
    - a call whose `arguments`/`input` member is written as a value that is not a string: the identity
      is the member's own bytes, and a decoded value cannot recover the spelling (a space after a
      colon, a number's written form) a receipt would have to name it by;
    - a result whose `output` member is written as a value that is not a string: the same, for the
      result's recorded text;
    - a `compacted` record carrying a `replacement_history` that holds something: the receipt's
      boundary identity is the summary text alone, so the plan cannot account for what the member
      holds (an empty list holds nothing and is not refused);
    - a `compacted` record carrying a `retained_context` that holds something: the same, for the
      member that keeps the retained families — original user and assistant message text persisted
      beside the checkpoint (`history/src/retained_context.rs`; an empty object holds nothing and is
      not refused);
    - a record whose `type` is one the format defines while its payload carries conversation text no
      occurrence kind can name (`ORACLE_UNACCOUNTABLE_RECORD_TYPES`: a `retained_context` record's
      retained message text, an `inter_agent_communication`'s `content`, a `realtime_item`'s
      `transcript_segment` text): the plan's vocabulary is this route's four occurrence kinds, so
      dialogue kept in such a record is dialogue no entry can account for;
    - a `compacted` record whose `message` member is **present but not a string**: the member's own
      bytes state that a summary is there and is not text, and the empty identity it reads as is the
      identity an absent summary has, so a receipt naming that empty text would certify a summary the
      artifact does not hold (an absent member is the empty boundary the plan can name);
    - a `response_item` `message` whose `role` is a non-empty string outside the receipt's vocabulary
      (`user`/`assistant`): the receipt names a message by its role and its text, so a record in any
      other role carries dialogue no plan entry can account for. The role is the artifact's own bytes
      rather than a projection of a read — the host's own sessions hold 57 `developer`-role messages
      (§0 of the evidence record), which is why the shape is silent without a plan;
    - a `session_meta` carrying a non-blank `forked_from_id`: the artifact's own statement that its
      history continues another transcript's leaves unstated which of its records are this session's
      own, so a receipt marking them emitted certifies a history the artifact does not establish.

    Each is refused rather than read under either answer: a native rollout may legitimately carry the
    shape (Codex 0.170.0's own files hold twelve blank call outputs and 122 list-valued
    `custom_tool_call_output` outputs, §0 of the evidence record), and the structural rules say nothing
    about a shape this route's writer does not emit. No diagnostic echoes the document's own text: a
    field is named by its place and the value's kind (`oracle_value_text`), never its contents.
    """
    violations: list[Violation] = []
    for line, record in records:
        kind = record.get("type")
        if kind == "compacted":
            payload = oracle_payload(record)
            if "message" in payload and not isinstance(payload["message"], str):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the compaction boundary's `message` is "
                                            f"{oracle_value_text(payload['message'])}, not a string: "
                                            f"the receipt's boundary identity is the summary text, and "
                                            f"naming this boundary by the empty text the member reads "
                                            f"as would certify a summary the artifact does not hold"))
            if "replacement_history" not in payload and "retained_context" not in payload:
                continue
            history = payload.get("replacement_history")
            if "replacement_history" in payload and not (isinstance(history, list) and not history):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the compaction boundary carries a `replacement_history` "
                                            f"({oracle_value_text(history)}); the receipt's boundary "
                                            f"identity is the summary alone, so the plan cannot account "
                                            f"for dialogue written in that member"))
            retained = payload.get("retained_context")
            if "retained_context" in payload and not (isinstance(retained, dict) and not retained):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the compaction boundary carries a `retained_context` "
                                            f"({oracle_value_text(retained)}); the retained families "
                                            f"keep original user and assistant message text beside the "
                                            f"checkpoint, and the receipt's boundary identity is the "
                                            f"summary alone, so the plan cannot account for dialogue "
                                            f"kept in that member"))
            continue
        if kind == "session_meta":
            parent = oracle_payload(record).get("forked_from_id")
            if isinstance(parent, str) and not oracle_blank(parent):
                violations.append(Violation("oracle.plan-accounted", line,
                                            "`session_meta.forked_from_id` names a fork parent: the "
                                            "artifact states that its history continues another "
                                            "transcript's, and which of its records are this "
                                            "session's own is not stated by the artifact alone, so a "
                                            "receipt cannot certify them"))
            continue
        if kind in ORACLE_UNACCOUNTABLE_RECORD_TYPES:
            violations.append(Violation("oracle.plan-accounted", line,
                                        f"the artifact holds a `{kind}` record: its payload carries "
                                        f"conversation text the receipt's occurrence vocabulary "
                                        f"({', '.join(ORACLE_OCCURRENCE_KINDS)}) has no kind for, so "
                                        f"the plan cannot account for it"))
            continue
        if kind != "response_item":
            continue
        payload = oracle_payload(record)
        item = payload.get("type")
        if item == "message":
            role = payload.get("role")
            if isinstance(role, str) and not oracle_blank(role) and role not in ORACLE_MESSAGE_ROLES:
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the message's `role` is {oracle_value_text(role)}, "
                                            f"outside the receipt's vocabulary "
                                            f"({', '.join(ORACLE_MESSAGE_ROLES)}): the receipt names "
                                            f"a message by its role, so this record's dialogue is one "
                                            f"no plan entry can account for"))
            continue
        if item in ORACLE_CALL_ITEMS:
            if "arguments" in payload and "input" in payload:
                violations.append(Violation("oracle.plan-accounted", line,
                                            "the artifact holds a tool call carrying both `arguments` "
                                            "and `input`: the receipt's single `arguments` identity "
                                            "names one recorded input while the record writes two, so "
                                            "the plan cannot account for the invocation"))
                continue
            for member in ("arguments", "input"):
                if member in payload and not isinstance(payload[member], str):
                    violations.append(Violation("oracle.plan-accounted", line,
                                                f"the call's recorded input is written as the "
                                                f"non-string `{member}` member "
                                                f"({oracle_value_text(payload[member])}): the receipt's "
                                                f"`arguments` identity is a string and a decoded value "
                                                f"cannot recover the member's own bytes, so no receipt "
                                                f"can name this invocation exactly"))
                    break
        elif item in ORACLE_RESULT_ITEMS:
            if "output" in payload and not isinstance(payload["output"], str):
                violations.append(Violation("oracle.plan-accounted", line,
                                            f"the result's recorded text is written as the non-string "
                                            f"`output` member "
                                            f"({oracle_value_text(payload['output'])}): the receipt's "
                                            f"recorded-text identity is a string and a decoded value "
                                            f"cannot recover the member's own bytes, so no receipt can "
                                            f"name this output exactly"))
    return violations


def oracle_occurrences(records: list[tuple[int, dict]]) -> list[tuple[tuple, int]]:
    """The artifact's own occurrences: ``((kind, identity…), line)`` for each one it holds.

    The vocabulary is this route's four evidence kinds — a message in one of the writer's two roles, a
    tool call, a tool result and a compaction boundary. Identity is the occurrence's own: role and text
    for a message, `call_id` and the emitted tool's `name` and recorded input for a call, `call_id` and
    the recorded text for a result (two results under one call id are two occurrences), the summary's
    text for a boundary (`""` where the record carries none — every `compacted` record is an occurrence).
    Multiplicity is part of the record: two identical messages are two occurrences, and the receipt is
    counted the same way. A `message` whose role is not one of the two is no occurrence of this
    vocabulary; where a plan is compared, `oracle_unnameable_shape_violations` refuses it, because the
    receipt names a message by its role and the record carries dialogue no entry can account for.
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


def oracle_reading(text: str, receipt: object = _NO_RECEIPT,
                   disabled: frozenset[str] = frozenset()
                   ) -> tuple[list[tuple[int, dict]], list[tuple[tuple, int]], list[Violation]]:
    """One artifact read once: its records, its own occurrences and every rule's violations.

    `validate_artifact` and `oracle_report` both answer from this one reading, so the counts a verdict
    line prints and the verdict itself cannot disagree about the same text — the numbers come from the
    reading that was judged, not from a second parse of the same bytes. `receipt` is the supplied
    document itself, and the default is the absence of one: a supplied document that is not a plan — a
    file that reads `null`, say — is `oracle.receipt`'s finding rather than an absent receipt.
    `disabled` holds rule ids whose violations are dropped from this reading — the in-process half of
    `--disable-rule`, which turns exactly the named rules off without editing a file — and it is how
    `--selftest-mutations` proves a negative fixture passes once its own rule is off.
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
    violations += oracle_call_members_violations(records)
    occurrences = oracle_occurrences(records)
    if receipt is not _NO_RECEIPT:
        plan, receipt_violations = oracle_plan(receipt)
        violations += receipt_violations
        if not receipt_violations:
            view_occurrences = oracle_view_occurrences(records)
            violations += oracle_plan_violations(plan, occurrences, view_occurrences)
            violations += oracle_plan_order_violations(plan, occurrences, view_occurrences)
            violations += oracle_unnameable_shape_violations(records)
    return records, occurrences, sorted(
        (violation for violation in violations if violation.rule not in disabled),
        key=lambda violation: (violation.line, violation.rule, violation.detail))


def validate_artifact(text: str, receipt: object = _NO_RECEIPT,
                      disabled: Iterable[str] = ()) -> list[Violation]:
    """Every rule this oracle states, over one artifact and — where a receipt is handed in — its plan.

    The artifact is this `text` and nothing else: no path, no source session, no selection and no
    harness, which is what lets a **reduced** artifact be validated with the source absent from the
    process. A receipt is compared only when every one of its entries could be read; an unreadable one is
    reported under `oracle.receipt` alone, because a plan nobody can read says nothing about an artifact.
    The default means no receipt was supplied; a supplied value that is not a plan is refused by that
    same rule. `disabled` names the rules to turn off for this reading (`oracle_reading`).
    """
    return oracle_reading(text, receipt, frozenset(disabled))[2]


def oracle_report(artifact: Path, receipt: Path | None, as_json: bool,
                  disabled: frozenset[str] = frozenset()) -> int:
    """The oracle's own command: one artifact, at most one receipt, a verdict and an exit code.

    It reads the artifact's bytes and, when one is named, the receipt's — nothing else, ever. An input
    it cannot read at all ends as `oracle: INCOMPLETE` (exit 2), never as a pass: nothing was validated.
    Bytes that are not UTF-8 text are such an input, not something to repair with replacement
    characters, because this oracle judges the artifact's own bytes. A receipt that parses to a
    non-object (a file holding `null`) is a *supplied* document that is not a plan and is refused by
    `oracle.receipt`, not silently read as an absent receipt; a receipt whose object writes a member name
    twice is not read as *one* plan at all (`_oracle_unique_members`), so it is an unreadable document
    like any other and ends `INCOMPLETE`. `disabled` names the rules to turn off for this reading, the
    in-process `--disable-rule` half: their violations are dropped exactly as if the rules were not
    stated, and every other rule is read as it always is. The verdict line is the last one printed,
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
    records, occurrences, violations = oracle_reading(text, receipt=document, disabled=disabled)
    if as_json:
        print(json.dumps({
            "artifact": str(artifact),
            "receipt": str(receipt) if receipt is not None else None,
            "disabled_rules": sorted(disabled),
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


# ===================================================================================================
# The oracle's mutation corpus (goal 0009a, D1).
#
# `ORACLE_MUTATIONS` pairs every rule id in `ORACLE_RULES` with at least one committed synthetic fixture
# — `scripts/tests/testdata/codex-oracle/`'s two artifacts and their manifest receipts — and a change
# that removes exactly that rule from it. It lives here, beside the registry, so `--selftest-mutations`
# can execute the closed equality D1 states without reading the test suite: for every declared id, the
# positive fixture passes, the negative fixture fails citing that rule alone, and the same negative
# fixture passes when that rule is disabled. Each arm is driven through this file's `main` — the shipped
# command's own `--validate`/`--receipt`/`--disable-rule` path — with the mutated artifact (and receipt)
# written under the run's scratch directory, so the flag the third arm names is the shipped one rather
# than the validator's Python default (`_oracle_selftest_one`, `_oracle_cli_reading`). A rule id with no
# fixture pair, a fixture pair naming an id
# the registry does not declare, a mutation that changes nothing, a second rule turned red beside the
# named one, or a negative fixture that stays red with its own rule off — each fails the command, and
# the required `selftest` gate step runs the same corpus through the suite's own cases. The corpus
# writes nothing anywhere: the selftest materializes each mutated pair under a scratch directory of its
# own, reads it back, and removes the directory when the run ends.
# ===================================================================================================

ORACLE_FIXTURE_ROOT = Path(__file__).resolve().parent / "tests" / "testdata" / "codex-oracle"


def oracle_fixture(name: str) -> str:
    """One committed fixture's own text."""
    return (ORACLE_FIXTURE_ROOT / name).read_text(encoding="utf-8")


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
    """One negative fixture: the change that removes a rule, the fixture pair it is applied to, and
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
    """One record's instant with a line feed appended: no RFC3339 instant, whatever a `$` anchor says."""
    record["timestamp"] += "\n"


def _timestamp_unicode_digit(record: dict) -> None:
    """One digit of a record's instant written as a non-ASCII decimal digit."""
    stamp = record["timestamp"]
    if not stamp.startswith("2026"):
        raise AssertionError("the fixture's instant no longer opens with the year this mutation edits")
    record["timestamp"] = "\u0662" + stamp[1:]


def _payload_becomes_a_list(record: dict) -> None:
    record["payload"] = ["not", "an", "object"]


def _type_dropped(record: dict) -> None:
    """One record's envelope `type` deleted: the rule's own non-empty-string arm."""
    record.pop("type")


def _type_becomes_unknown(record: dict) -> None:
    record["type"] = "conversation_item"


def _id_becomes_empty(record: dict) -> None:
    record["payload"]["id"] = ""


def _cli_version_dropped(record: dict) -> None:
    record["payload"].pop("cli_version")


def _originator_dropped(record: dict) -> None:
    """The metadata record's `originator` deleted: the third member the rule names."""
    record["payload"].pop("originator")


def _content_becomes_empty(record: dict) -> None:
    record["payload"]["content"] = []


def _role_dropped(record: dict) -> None:
    """One message's `role` deleted: the rule's own non-empty-string arm."""
    record["payload"].pop("role")


def _content_block_type_is_not_read(record: dict) -> None:
    record["payload"]["content"][0]["type"] = "input_image"


def _content_text_becomes_blank(record: dict) -> None:
    record["payload"]["content"][0]["text"] = "   "


def _content_text_is_not_a_string(record: dict) -> None:
    """One message given a second readable block whose `text` is not a string, the first left valid.

    The valid non-blank neighbour is what makes this the string-type arm's own fixture: the all-blank
    clause cannot see a record that still holds one readable string.
    """
    record["payload"]["content"].append({"type": "input_text", "text": 7})


def _event_message_becomes_blank(record: dict) -> None:
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

    Python's decoder accepts `NaN`/`Infinity`; the artifact is JSON, so a line this oracle's parser
    would have turned into a float the format cannot hold is refused exactly as a line that does not
    parse is.
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
    """One line nested past what this reader can parse: refused as unreadable, or read as no object."""
    return oracle_edit(text, oracle_record_is("event_msg"),
                       replacement="[" * 200000 + "]" * 200000), receipt


def _mutation_line_duplicate_member(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's line with a member name written twice, spliced into the JSON text itself.

    The duplication is a text edit and not a record edit on purpose: `json.dumps` of a parsed dict
    cannot write a repeated name, and the two JSON readers this rule is about disagree only about text
    no single dict holds. The repeated pair's values are equal, so the record's own parsed value is
    unchanged and the line is refused by the uniqueness clause alone.
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


#: The two call-id spellings that are one id to a read: JSON's `\ud800` escape decodes to a lone
#: surrogate to this oracle's decoder and to U+FFFD to the carried reader's `gjson` (measured on the
#: carried v1.20.0), so two ids this build can tell apart are one id to every read of the bytes.
SURROGATE_CALL_IDS = ("\ud800", "\ufffd")


def _mutation_call_id_lone_surrogate(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One call and its output rewritten to the lone-surrogate escape, twins under U+FFFD appended.

    Uniqueness, pairing and every receipt identity comparison keep the two spellings apart in this
    oracle — they are two Python strings — while a read of the artifact answers **one** call id for
    both, so the mutation is the collision `oracle.jsonl-objects` refuses before either reader's answer
    is compared; the twin pair is appended at the last record's instant so the clock stays legal.
    """
    lines = text.splitlines()
    call_index = next(index for index, line in enumerate(lines)
                      if oracle_item_is("function_call", "call_oracle_0001")(json.loads(line)))
    output_index = next(index for index, line in enumerate(lines)
                        if oracle_item_is("function_call_output", "call_oracle_0001")(json.loads(line)))
    call, output = json.loads(lines[call_index]), json.loads(lines[output_index])
    for record in (call, output):
        record["payload"]["call_id"] = SURROGATE_CALL_IDS[0]
    lines[call_index], lines[output_index] = oracle_quote(call), oracle_quote(output)
    stamp = json.loads(lines[-1])["timestamp"]
    twins = []
    for record in (call, output):
        twin = json.loads(oracle_quote(record))
        twin["timestamp"] = stamp
        twin["payload"]["call_id"] = SURROGATE_CALL_IDS[1]
        twins.append(twin)
    return "\n".join(lines + [oracle_quote(twin) for twin in twins]) + "\n", receipt


def _mutation_envelope_without_timestamp(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_event_is("user_message"), change=_drop_timestamp), receipt


def _mutation_envelope_payload_not_an_object(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("world_state"), change=_payload_becomes_a_list), receipt


def _mutation_envelope_without_type(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's envelope `type` deleted: the rule's non-empty-string arm.

    The other envelope mutations reach the timestamp and the payload arms alone, so this one is the arm
    that states a record must carry a type at all (`oracle.record-type` deliberately leaves a missing or
    blank one to this rule).
    """
    return oracle_edit(text, oracle_record_is("world_state"), change=_type_dropped), receipt


def _mutation_envelope_offset_out_of_range(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's numeric offset leaves the range RFC 3339 bounds: `+99:99` dates no instant."""
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_zone_out_of_range), receipt


def _mutation_envelope_leap_second(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record stamped at the leap second `60`, which no clock this writer runs on emits.

    An instant reader that accepted `:60` would have to decide what it means — mapping it to the next
    minute puts a fractional leap second after the midnight that follows it — so the oracle reads the
    writer's own instant subset and refuses a `:60` stamp.
    """
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_leap_second), receipt


def _mutation_envelope_trailing_newline(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's instant with a line feed appended: Python's `$` would read it as the same instant."""
    return oracle_edit(text, oracle_record_is("turn_context"),
                       change=_timestamp_trailing_newline), receipt


def _mutation_envelope_unicode_digits(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One record's instant with a non-ASCII decimal digit, which RFC3339 does not write."""
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


def _mutation_session_meta_not_first(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The artifact's one metadata record standing second, behind a record of another type.

    The first two records trade bodies while each slot keeps the instant it held, so the clock stays
    nondecreasing and every other rule still holds — the metadata record is present, it is the only one,
    and its own members are untouched.
    """
    lines = text.splitlines()
    if len(lines) < 2:
        raise AssertionError("the fixture holds fewer than two records")
    first, second = json.loads(lines[0]), json.loads(lines[1])
    if first.get("type") != "session_meta" or second.get("type") == "session_meta":
        raise AssertionError("the fixture no longer opens with a session_meta then another record")
    moved, kept = dict(second), dict(first)
    moved["timestamp"], kept["timestamp"] = first["timestamp"], second["timestamp"]
    lines[0], lines[1] = oracle_quote(moved), oracle_quote(kept)
    return "\n".join(lines) + "\n", receipt


def _mutation_every_record_deleted(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The artifact holds no records at all, its metadata record among them: nothing to open with."""
    if not text.strip():
        raise AssertionError("the fixture is already empty")
    return "", receipt


def _mutation_session_meta_id_empty(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("session_meta"), change=_id_becomes_empty), receipt


def _mutation_session_meta_cli_version_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_record_is("session_meta"), change=_cli_version_dropped), receipt


def _mutation_session_meta_originator_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The metadata record's `originator` deleted: the rule's third member arm."""
    return oracle_edit(text, oracle_record_is("session_meta"),
                       change=_originator_dropped), receipt


def _mutation_clock_swapped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The compaction boundary and the turn context after it trade instants: one written backwards.

    The pair is adjacent and the swap touches no other rule: the boundary's own line and the turn
    context that follows it carry each other's instant, so exactly one record is stamped before the
    record written before it.
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


def _mutation_message_block_type_is_text(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One message's readable block retyped `text`, the carried reader's spelling the format lacks.

    The text, the receipt and both channels' identities are unchanged — the occurrence vocabulary reads
    the reader's spelling too — so neither accounting rule nor the order comparison can see the change,
    and only the message rule can refuse the record: Codex 0.170.0's content-item enum defines no
    `text` variant, so the native decoder refuses the line and its rollout reader skips it.
    """
    return oracle_edit(
        text, oracle_item_is("message", "Which fixture is this?"),
        change=lambda record: record["payload"]["content"][0].__setitem__("type", "text")), receipt


def _mutation_message_text_blank(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("message", "And now?"),
                       change=_content_text_becomes_blank), receipt


def _mutation_message_text_not_a_string(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One message given a second readable block whose `text` is not a string, the first left valid."""
    return oracle_edit(text, oracle_item_is("message", "Which fixture is this?"),
                       change=_content_text_is_not_a_string), receipt


def _mutation_message_role_dropped(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One message's `role` deleted, its content untouched: the structural role arm's own fixture."""
    return oracle_edit(text, oracle_item_is("message", "The synthetic one."),
                       change=_role_dropped), receipt


def _mutation_event_message_not_a_string(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_event_is("agent_message"),
                       change=_event_message_is_not_a_string), receipt


def _mutation_event_message_blank(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One turn event's message replaced with a blank string: no view record a read of it keeps."""
    return oracle_edit(text, oracle_event_is("agent_message"),
                       change=_event_message_becomes_blank), receipt


def _mutation_output_names_an_absent_call(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    return oracle_edit(text, oracle_item_is("function_call_output", "call_oracle_0001"),
                       change=_output_names_an_absent_call), receipt


def _mutation_output_before_its_call(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's own output written one slot earlier, its instant staying in the slot.

    The pair trades records while each keeps the instant of the slot it stands in, so the clock stays
    nondecreasing, every count is unchanged and the output still names an id the artifact carries — only
    the record order says the output stands before its owner.
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


def _mutation_call_id_duplicated(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    lines = text.splitlines()
    for index, line in enumerate(lines):
        if json.loads(line).get("type") != "response_item":
            continue
        if (json.loads(line).get("payload") or {}).get("type") != "function_call":
            continue
        return "\n".join(lines[:index + 1] + [line] + lines[index + 1:]) + "\n", receipt
    raise AssertionError("the fixture no longer holds a function_call")


def _mutation_call_without_a_usable_name(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's `name` member deleted, its id, arguments and recorded output untouched.

    The record then states no invocation the occurrence vocabulary can carry, and the output under its
    id attaches to nothing — both findings are `oracle.call-pairing`'s own.
    """
    return oracle_edit(text, oracle_item_is("function_call", "call_oracle_0001"),
                       change=lambda record: record["payload"].pop("name")), receipt


def _mutation_call_without_a_call_id(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's `call_id` written as the empty string, and **no** output left to fail pairing on its
    own: the call keeps the `name` a receipt's identity needs, and its output record is deleted with it.
    """
    text = oracle_edit(text, oracle_item_is("function_call", "call_oracle_0001"),
                       change=lambda record: record["payload"].__setitem__("call_id", ""))
    return _without_record(text, oracle_item_is("function_call_output", "call_oracle_0001")), receipt


def _mutation_call_input_member_renamed(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's recorded input moved from `arguments` to the other family's `input` member.

    The invocation's written identity does not move — the occurrence vocabulary reads whichever member
    is written — so the unchanged receipt still names the call, its output still pairs, and every count
    and order the plan compares is unchanged; the record is one the native decoder cannot decode,
    though, because a `function_call` states its recorded input in `arguments`. The receipt is left as
    committed so only the native-member rule can refuse the artifact.
    """
    def rename(record: dict) -> None:
        record["payload"]["input"] = record["payload"].pop("arguments")

    return oracle_edit(text, oracle_item_is("function_call", "call_oracle_0001"), change=rename), receipt


def _mutation_result_without_its_output(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """One call output's `output` member deleted: an absent member, not a written empty result.

    The identity reads an absent member as the empty string, so every structural rule that reads the
    text sees the result a written empty output would give; the native decoder requires the member on
    the payload type, and the rollout reader skips a line it cannot decode.
    """
    return oracle_edit(text, oracle_item_is("function_call_output", "call_oracle_0001"),
                       change=lambda record: record["payload"].pop("output")), receipt


def _mutation_receipt_declares_nothing(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The receipt left declaring no occurrences at all: a plan that accounts for nothing."""
    assert receipt is not None
    receipt["occurrences"] = []
    return text, receipt


def _oracle_omitted_message(receipt: dict) -> dict:
    """One message entry the plan marks `omitted`, for the fixture that plants its text elsewhere."""
    omitted = [entry for entry in receipt["occurrences"]
               if entry.get("kind") == "message" and entry.get("disposition") == "omitted"]
    if not omitted:
        raise AssertionError("the fixture's plan marks no message omitted")
    return omitted[0]


def _mutation_plan_boundary_replacement_history(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The boundary's native `replacement_history` filled with the dialogue the plan marks omitted.

    A native `compacted` record carries the messages it replaced beside its summary, and the receipt's
    boundary identity is that text alone: dialogue written into `replacement_history` is history the
    plan cannot account for although every identity, count and order it compares is unchanged. The
    mutation writes the plan's own omitted prompt into the member — the bytes the plan claims are not in
    the artifact.
    """
    assert receipt is not None
    omitted = _oracle_omitted_message(receipt)

    def carry(record: dict) -> None:
        record["payload"]["replacement_history"] = [
            {"type": "message", "role": omitted.get("role", "user"),
             "content": [{"type": "input_text", "text": omitted["text"]}]}]

    return oracle_edit(text, oracle_record_is("compacted"), change=carry), receipt


def _mutation_plan_boundary_retained_context(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The boundary's native `retained_context` checkpoint filled with the plan's omitted prompt.

    The checkpoint member is the second place a boundary keeps original conversational text: the
    retained families live beside `replacement_history` in the native `CompactedItemWire`
    (`history/src/rollout_payload.rs`), which Codex serializes and restores. The plan marks the prompt
    omitted, every identity, count and order it compares is unchanged, and only the refusal can see the
    dialogue kept in the member.
    """
    assert receipt is not None
    omitted = _oracle_omitted_message(receipt)

    def carry(record: dict) -> None:
        record["payload"]["retained_context"] = {
            "verified_answers": [],
            "incomplete": False,
            "user_messages": [{"order": 1, "turn_id": "01a1f00d-5f2c-7000-8000-000000000003",
                               "message_id": None, "text": omitted["text"], "complete": True}],
            "user_messages_incomplete": False,
            "assistant_messages": [],
            "assistant_messages_incomplete": False,
            "next_order": 1,
        }

    return oracle_edit(text, oracle_record_is("compacted"), change=carry), receipt


def _mutation_plan_retained_context_record(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A `retained_context` record appended, carrying the prompt the plan marks omitted.

    The record type is one Codex 0.170.0 persists during ordinary operation (a retained event lands in
    the rollout as its own record — `core/src/session/retained_context.rs`), so the structural rule
    admits it and no structural rule reads its payload; the receipt's occurrence vocabulary has no kind
    for it, so only the refusal can refuse the dialogue the record keeps.
    """
    assert receipt is not None
    omitted = _oracle_omitted_message(receipt)
    lines = text.splitlines()
    stamp = json.loads(lines[-1])["timestamp"]
    planted = {"timestamp": stamp, "type": "retained_context",
               "payload": {"type": "delivered_assistant_message",
                           "turn_id": "01a1f00d-5f2c-7000-8000-000000000003",
                           "message_id": None, "text": omitted["text"], "complete": True,
                           "acceptance_order": 1}}
    return "\n".join(lines + [oracle_quote(planted)]) + "\n", receipt


def _mutation_plan_unnameable_message_role(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A well-formed message in a role the receipt's vocabulary cannot name, carrying the plan's
    omitted prompt, appended at the last record's instant.

    The record is one the structural rule admits — a non-empty `role`, a readable text block — so
    without the refusal it stands outside both compared channels (the occurrence vocabulary reads the
    writer's two roles only) while its bytes carry dialogue the plan marks omitted. The plan is
    untouched: the defect is an identity of the artifact's own that no receipt entry can state.
    """
    assert receipt is not None
    omitted = _oracle_omitted_message(receipt)
    lines = text.splitlines()
    stamp = json.loads(lines[-1])["timestamp"]
    planted = {"timestamp": stamp, "type": "response_item",
               "payload": {"type": "message", "role": "developer",
                           "content": [{"type": "input_text", "text": omitted["text"]}]}}
    return "\n".join(lines + [oracle_quote(planted)]) + "\n", receipt


def _mutation_plan_non_string_call_input(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call's recorded input written as a JSON object, spaces after its colons.

    The oracle's identity serializes a non-string member back to compact JSON and so reconstructs the
    receipt's own `arguments` string, while this route's writer's own calls write `arguments` as a
    string: a changed member would be certified as the unchanged invocation without the refusal. The
    receipt is untouched — it still names the compact spelling the mutation reconstructs.
    """
    needle = '"arguments":"{\\"file_path\\":\\"/work/oracle-fixture/notes.md\\"}"'
    replacement = '"arguments":{"file_path": "/work/oracle-fixture/notes.md"}'
    for line in text.splitlines():
        if not oracle_item_is("function_call", "call_oracle_0001")(json.loads(line)):
            continue
        if needle not in line:
            raise AssertionError("the fixture no longer writes the call's arguments this mutation edits")
        return text.replace(needle, replacement, 1), receipt
    raise AssertionError("the fixture no longer holds the call this mutation edits")


def _mutation_plan_two_input_members(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The call given a second recorded-input member beside its `arguments`.

    The record then writes two recorded inputs under one call id, and the receipt's single `arguments`
    member names one: the shape's receipt-facing identity is not stated by the artifact's own bytes.
    """
    def add(record: dict) -> None:
        record["payload"]["input"] = '{"file_path":"/work/oracle-fixture/other.md"}'

    return oracle_edit(text, oracle_item_is("function_call", "call_oracle_0001"), change=add), receipt


def _mutation_plan_forked_from_id(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """The session_meta carrying `forked_from_id`, naming a parent the oracle is not given.

    The artifact then states that its history continues another transcript's; which of its records are
    this session's own is not stated by the artifact alone, so a receipt cannot certify them. The member
    is written on the artifact alone: no receipt identity changes, so only the refusal can turn it red.
    """
    return oracle_edit(
        text, oracle_record_is("session_meta"),
        change=lambda record: record["payload"].__setitem__(
            "forked_from_id", "01a1f00d-5f2c-7000-8000-0000000000aa")), receipt


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

    The occurrence vocabulary counts every `compacted` record, summary or not, so the artifact holds an
    occurrence the receipt never planned although every rule that reads only string summaries — and
    every count — stayed unchanged.
    """
    lines = text.splitlines()
    stamp = json.loads(lines[-1])["timestamp"]
    lines.append(oracle_quote({"timestamp": stamp, "type": "compacted", "payload": {}}))
    return "\n".join(lines) + "\n", receipt


def _mutation_plan_boundary_non_string_summary(text: str, receipt: dict | None) -> tuple[str, dict | None]:
    """A boundary's summary rewritten as an object, with the plan naming the boundary's empty text.

    A non-string `message` reads as the empty occurrence identity — the same identity an absent summary
    has — so a receipt whose boundary entry was rewritten to `text: ""` compares green, by identity,
    count and order, against a boundary whose member the artifact states is not text. The receipt edit
    is part of the mutation because the plan names the summary it read: the artifact-only edit would
    turn `oracle.plan-emitted` and `oracle.plan-accounted` red over the count, not the member's type.
    """
    assert receipt is not None

    def corrupt(record: dict) -> None:
        if not isinstance(record["payload"].get("message"), str):
            raise AssertionError("the fixture's boundary no longer carries its string summary")
        record["payload"]["message"] = {"text": record["payload"]["message"]}

    text = oracle_edit(text, oracle_record_is("compacted"), change=corrupt)
    receipt = oracle_receipt_edit(receipt, oracle_entry_kind("compaction_boundary"),
                                  lambda entry: entry.__setitem__("text", ""))
    return text, receipt


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
    artifact — the defect the reduced plan claims not to have.
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

    Every count is unchanged, the clock stays nondecreasing, and every structural rule still holds — but
    the boundary now stands after the second turn, and only the order comparison can see the move.
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
    untouched: only the turn view puts the prompts in each other's turns.
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
    then describes: the plan's own split is the defect, and the artifact cannot be asked about it (a
    call with no recorded result is legitimate, so no structural rule forbids a bare call).
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
#: other — which is what makes the oracle a reading of one defect at a time rather than a net. The
#: table's keys and `ORACLE_RULES`' are asserted equal, from both ends, by `--selftest-mutations`.
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
        OracleMutation("two call ids distinct to Python and one to the read", "session.jsonl", None,
                       _mutation_call_id_lone_surrogate),
    ),
    "oracle.envelope": (
        OracleMutation("a record without its timestamp", "session.jsonl", None,
                       _mutation_envelope_without_timestamp),
        OracleMutation("a payload that is not an object", "session.jsonl", None,
                       _mutation_envelope_payload_not_an_object),
        OracleMutation("a record without its type", "session.jsonl", None,
                       _mutation_envelope_without_type),
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
        OracleMutation("the metadata record deleted", "session.jsonl", None,
                       _mutation_session_meta_deleted),
        OracleMutation("a second metadata record", "session.jsonl", None,
                       _mutation_session_meta_duplicated),
        OracleMutation("every record deleted", "session.jsonl", None,
                       _mutation_every_record_deleted),
        OracleMutation("a record standing before the metadata", "session.jsonl", None,
                       _mutation_session_meta_not_first),
    ),
    "oracle.session-meta-members": (
        OracleMutation("an empty id", "session.jsonl", None, _mutation_session_meta_id_empty),
        OracleMutation("no cli_version", "session.jsonl", None,
                       _mutation_session_meta_cli_version_dropped),
        OracleMutation("no originator", "session.jsonl", None,
                       _mutation_session_meta_originator_dropped),
    ),
    "oracle.clock": (
        OracleMutation("two adjacent instants swapped", "session.jsonl", None, _mutation_clock_swapped),
    ),
    "oracle.message": (
        OracleMutation("a message with no content", "session.jsonl", None,
                       _mutation_message_content_empty),
        OracleMutation("a block the oracle does not read", "session.jsonl", None,
                       _mutation_message_block_type_not_read),
        OracleMutation("a block typed with the reader's spelling the format lacks", "session.jsonl",
                       "session-receipt.json", _mutation_message_block_type_is_text),
        OracleMutation("a message whose text is blank", "session.jsonl", None,
                       _mutation_message_text_blank),
        OracleMutation("a message with no role", "session.jsonl", None, _mutation_message_role_dropped),
        OracleMutation("a block whose text is not a string beside a valid one", "session.jsonl", None,
                       _mutation_message_text_not_a_string),
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
        OracleMutation("a call without a usable name", "session.jsonl", None,
                       _mutation_call_without_a_usable_name),
        OracleMutation("a named call whose call_id is empty", "session.jsonl", None,
                       _mutation_call_without_a_call_id),
    ),
    "oracle.call-members": (
        OracleMutation("a call's recorded input renamed to the other family's member", "session.jsonl",
                       "session-receipt.json", _mutation_call_input_member_renamed),
        OracleMutation("a call output without its `output` member", "session.jsonl", None,
                       _mutation_result_without_its_output),
    ),
    "oracle.receipt": (
        OracleMutation("a disposition the vocabulary does not hold", "session.jsonl",
                       "session-receipt.json", _mutation_receipt_disposition_unknown),
        OracleMutation("a result entry with no call_id", "session.jsonl", "session-receipt.json",
                       _mutation_receipt_call_id_dropped),
        OracleMutation("a result entry with no recorded text", "session.jsonl", "session-receipt.json",
                       _mutation_receipt_result_text_dropped),
        OracleMutation("a receipt that declares no occurrences", "session.jsonl",
                       "session-receipt.json", _mutation_receipt_declares_nothing),
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
        OracleMutation("a boundary's retained checkpoint carrying the dialogue the plan omits",
                       "reduced.jsonl", "reduced-receipt.json",
                       _mutation_plan_boundary_retained_context),
        OracleMutation("a retained_context record carrying the dialogue the plan omits", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_retained_context_record),
        OracleMutation("a boundary whose summary member is not a string", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_boundary_non_string_summary),
        OracleMutation("a message role the receipt's vocabulary cannot name", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_unnameable_message_role),
        OracleMutation("an omitted message that survives in the turn view", "reduced.jsonl",
                       "reduced-receipt.json", _mutation_plan_omitted_event_survives),
        OracleMutation("a call whose recorded input is not a string", "session.jsonl",
                       "session-receipt.json", _mutation_plan_non_string_call_input),
        OracleMutation("a call carrying both recorded-input members", "session.jsonl",
                       "session-receipt.json", _mutation_plan_two_input_members),
        OracleMutation("a session_meta carrying explicit fork lineage", "session.jsonl",
                       "session-receipt.json", _mutation_plan_forked_from_id),
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


def _oracle_cli_reading(argv: list[str]) -> tuple[int, list[dict], str | None]:
    """One `--validate` invocation through `main` — the shipped command's own entry point.

    Returns ``(exit code, violations, None)`` where the command answered a readable JSON document, and
    ``(code, [], why)`` where it did not. The arms of `_oracle_selftest_one` are read through this
    function, never through `validate_artifact`'s Python `disabled` default: a regression that stopped
    `--disable-rule <RULE_ID>` from reaching the reading has to turn `--selftest-mutations` red rather
    than stay green behind an in-process call, and the artifact and receipt each arm judges are the
    bytes on disk a caller of the command would supply.
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

    (a) The positive fixture passes — with its plan where the pair names one — read through the
    command's own `--validate [--receipt]` path on the committed bytes. (b) The mutated pair differs
    from it and fails citing **only** `rule`. (c) The same mutated pair passes with `--disable-rule`
    for `rule`. The mutated artifact — and the mutated receipt where the pair names one — is
    materialized under `work`, and the negative and disabled arms are the same two paths read by the
    same command, so the flag the third arm names is the shipped one; `work` is the only place this
    run writes.
    """
    try:
        text = oracle_fixture(mutation.artifact)
        receipt = oracle_receipt(mutation.receipt) if mutation.receipt is not None else None
    except OSError as failure:
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
    before = (text, json.dumps(receipt, sort_keys=True) if receipt is not None else None)
    mutated_text, mutated_receipt = mutation.mutate(
        text, oracle_receipt(mutation.receipt) if mutation.receipt is not None else None)
    after = (mutated_text,
             json.dumps(mutated_receipt, sort_keys=True) if mutated_receipt is not None else None)
    if after == before:
        return "the mutation changed nothing"
    stem = rule.replace(".", "-")
    artifact_path = work / (stem + ".jsonl")
    artifact_path.write_bytes(mutated_text.encode("utf-8"))
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
        return (f"the negative fixture with `--disable-rule {rule}` is still not green "
                f"(exit {code}, citing {cited}): "
                + "; ".join(f"{violation['rule']} line {violation['line']}"
                            for violation in violations))
    return None


def oracle_mutation_selftest() -> int:
    """The D1 proof, executed: the rule registry against the mutation corpus, closed from both ends.

    Every declared rule id must have at least one fixture pair, every fixture pair must name a declared
    id, and each pair's three arms must hold (`_oracle_selftest_one`), each driven through the shipped
    command's own `--validate`/`--disable-rule` path. One line per pair says so, a failure names the
    rule and what disagreed, and the exit code is 0 only when every arm of every pair held — the shape
    a reviewer can re-run without editing anything, which is what makes the registry a finite, executed
    obligation rather than a claim about the checker's own code. The run creates one scratch directory
    and removes it again; it writes nothing outside it and touches no fixture.
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
                        help="validate a Codex session artifact against the frozen well-formedness "
                             "oracle (offline: no harness, no source session) and exit")
    parser.add_argument("--receipt", type=Path, metavar="DOCUMENT",
                        help="the artifact's manifest receipt, with --validate: the occurrences a "
                             "conversion planned, compared with the artifact")
    parser.add_argument("--disable-rule", action="append", default=[], metavar="RULE_ID",
                        help="with --validate: drop exactly this rule's violations from the reading, as "
                             "if the rule were not stated (repeatable); an id the registry does not "
                             "declare is refused")
    parser.add_argument("--selftest-mutations", action="store_true",
                        help="prove the rule registry against its mutation corpus and exit: for every "
                             "declared rule id its positive fixture passes, its negative fixture fails "
                             "citing that rule alone, and the same fixture passes with --disable-rule "
                             "for it — each arm through this command's --validate path, on the mutated "
                             "artifact and receipt written under a scratch directory")
    args = parser.parse_args(argv)

    if args.selftest_mutations:
        if args.validate is not None or args.receipt is not None or args.disable_rule:
            parser.error("--selftest-mutations runs the whole corpus and takes none of --validate, "
                         "--receipt or --disable-rule")
        return oracle_mutation_selftest()
    if args.receipt is not None and args.validate is None:
        parser.error("--receipt names the manifest receipt of the artifact --validate is given")
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
