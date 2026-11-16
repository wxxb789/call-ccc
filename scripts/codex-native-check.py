#!/usr/bin/env python3
"""Native verification for the Claude Code → Codex route (goal 0002).

This harness answers the half of the goal that the gate cannot: whether the installed target session is
readable by **Codex itself** and whether a **real turn** continues from it. It builds `ccc`, converts a
committed fixture into an isolated Codex data directory, re-reads the installed file with its own second
`ccc` invocation (the inherited reader, so the conversion's own verdict about itself decides nothing
here), checks the resume invocation the conversion reported (its arguments name the target binary, the
resume subcommand and the session, and its environment selects the directory the session is installed
into), asks that directory's own app server to load the session (`thread/resume`), reads the harness's
own view of the conversation (`thread/read`), and — when `--resume-prompt` is given — runs one real turn
from the reported invocation and reports what the target answered. Only then — after the loader and the
turn, and before the cleanup that removes the planted session — does it compare the source session and
every other session in the target home (the session it plants there and any the home already held)
against the bytes they had before the conversion, so a mutation during either native stage is answered by
the comparison that runs last rather than by an earlier verdict standing in for it. The planted session is
removed again when the run ends, so a `--codex-home` the caller named is left as it was found apart from
the converted session.

It is **not** a gate step: it needs the Codex binary, and a turn needs model access. `scripts/gate.py`
decides the offline half (round-trip through the inherited reader, install rules, reported losses, CLI
shape); this script is the re-derivable native evidence, the way `scripts/upstream-compare.py` is for the
carried tree's behaviour. Every app-server request it makes is bounded by `APP_SERVER_TIMEOUT`, and a
server that stops answering is reported as a failed check with its own diagnostics rather than blocking
the run.

    python3 scripts/codex-native-check.py
    python3 scripts/codex-native-check.py --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
        --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --json          # machine-readable summary on stdout

`--model-provider`/`--model` name what the throw-away Codex configuration should use, because the
target's loader checks a session against the provider *its* configuration names: the load check only
needs a configured provider, while `--resume-prompt` needs one that can actually answer on this host
(its key comes from the environment, never from a copied credential). That throw-away configuration is
built from the settings of the host's own `config.toml`, which is never copied as a document: it is
seeded with that file's model settings and the named provider's definition, and never with a setting
whose value can hold a credential -- a provider's authorization headers, an MCP server's environment
table, a credential written into a URL -- because a configuration is exactly where those live. The
report line names the settings that were copied and the ones that were left behind, so a provider whose
credential is written into the configuration has to take it from the environment instead.

Exit codes: 0 every check that ran passed; 1 a check failed; 2 the harness could not run (no Codex, no
Go toolchain, a missing fixture).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import queue
import shlex
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import tomllib
from pathlib import Path
from typing import NamedTuple
from urllib.parse import urlsplit

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

#: The unrelated session the harness plants in the target home before converting: a conversion must not
#: touch a session another agent may already have continued.
UNRELATED_NAME = "rollout-2026-01-01T00-00-00-00000000-0000-7000-8000-000000000000.jsonl"

#: One app-server request's deadline. Every call this harness makes is bounded by it: a server that keeps
#: its output open and stops answering is reported as a failed check rather than blocking the run.
APP_SERVER_TIMEOUT = 120

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

#: One check's outcome, as the script reports it.
RESULTS: list[tuple[str, str, str]] = []


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    digest.update(path.read_bytes())
    return digest.hexdigest()


def tree_digest(root: Path) -> dict[str, str]:
    """Every file below root, keyed by its path relative to root."""
    digests: dict[str, str] = {}
    for path in sorted(root.rglob("*")):
        if path.is_file():
            digests[path.relative_to(root).as_posix()] = sha256(path)
    return digests


def record(name: str, ok: bool, detail: str = "", skipped: bool = False) -> None:
    state = "SKIP" if skipped else ("PASS" if ok else "FAIL")
    RESULTS.append((name, state, detail))
    print(f"{state}: {name}" + (f" — {detail}" if detail else ""))


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


def credential_free_url(url: str) -> bool:
    """Whether a base URL is one this check copies: an endpoint with no userinfo, query or fragment.

    A username or a password in a URL is a credential written into the configuration, and a query can
    hold one too (`?api_key=…` is what a provider's `query_params` setting would otherwise carry). A URL
    the standard library cannot read at all is not one this check can vouch for.
    """
    try:
        parts = urlsplit(url)
    except ValueError:
        return False
    return not (parts.username or parts.password or parts.query or parts.fragment)


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


def shell_words(command: str) -> list[str] | None:
    """The words a POSIX shell reads `command` as, or None when it cannot be read as a command line.

    The reported invocation is rendered for a POSIX shell (`internal/ccc/continue.go`'s
    `renderResumeCommand`/`shellQuote` quote a word exactly where a shell would act on it), so the
    documented reading of it is a shell's own: split on whitespace outside quotes, then remove the
    quoting. A check that looked for the literal substring `CODEX_HOME=<home>` would do the opposite of
    that rule: it rejects every home the renderer correctly quotes -- any home with a space, any Windows
    path with a backslash -- and accepts an unquoted home whose space a shell reads as two words, which
    is the defect quoting exists to prevent.
    """
    try:
        return shlex.split(command)
    except ValueError:
        # An unbalanced quote: a shell would not read this as the reported invocation at all.
        return None


def reported_invocation(command: str, home: Path, arguments: list[str]) -> bool:
    """Whether `command` is the shell's reading of the data-directory assignment plus `arguments`.

    Codex takes its data directory from CODEX_HOME alone -- it has no flag for it -- so the assignment
    being first and exact is what makes a pasted line resume the session this run installed.
    """
    return shell_words(command) == [f"CODEX_HOME={home}", *arguments]


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
        timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=cwd, env=merged, capture_output=True, text=True, timeout=timeout)


class AppServer:
    """A minimal client for the target's own app-server protocol over stdio.

    stdout is read by a thread of its own, so a call's deadline can end it: a server that keeps its
    stdout open and stops answering would otherwise block a synchronous read for ever, and this harness
    promises a bounded verdict for every check. stderr is drained by a second thread for the same
    reason -- a piped stream nobody reads fills up, and a server that blocks writing a diagnostic stops
    answering -- and the lines it holds are reported with a failure instead of being thrown away.
    """

    def __init__(self, binary: str, home: Path):
        self.next_id = 0
        self.process = subprocess.Popen(
            [binary, "app-server", "--listen", "stdio://"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, bufsize=1, env={**os.environ, "CODEX_HOME": str(home)},
        )
        self.responses: queue.Queue[str | None] = queue.Queue()
        self.diagnostics: list[str] = []
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
        try:
            for line in stream:
                self.diagnostics.append(line.rstrip("\n"))
        except (OSError, ValueError):
            pass

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
        return " | ".join(self.diagnostics[-limit:])[:400]

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
    parser.add_argument("--model-provider", help="provider the throw-away Codex config should name "
                        "(default: the host configuration's own answer)")
    parser.add_argument("--model", help="model the throw-away Codex config should name")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    candidates = sorted((fixture_root / SOURCE_PROVIDER).glob(f"*/{args.session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {SOURCE_PROVIDER} fixture named {args.session}.jsonl below "
              f"{fixture_root / SOURCE_PROVIDER}, found {len(candidates)}", file=sys.stderr)
        return 2
    source = candidates[0]
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
    print(f"scratch:     {scratch}")
    print()

    fixture_before = tree_digest(fixture_root)
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

        env = {"CLAUDE_PROJECTS_DIR": str(fixture_root / SOURCE_PROVIDER)}
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
        day_directory = installed.parent.relative_to(home / "sessions")
        record("the session is installed under the target's own sessions tree",
               installation["state"] == "installed" and installed.is_file()
               and len(day_directory.parts) == 3 and all(part.isdigit() for part in day_directory.parts),
               f"{installed.relative_to(home)} [{installation['mechanism']}]")
        record("the target identity is new and is not the source's",
               thread_id != args.session and thread_id not in existing_ids,
               f"{thread_id}, not one of the {len(existing_ids)} id(s) already in the home")
        record("the installed file is one JSON object per line",
               all(json.loads(line) for line in installed.read_text().splitlines() if line.strip()),
               f"{installed.stat().st_size} bytes (the target's own read of it is the load check)")
        # The installed file is read back by this script's own invocation of the inherited reader, not by
        # the conversion's own structure block: `ccc inspect` is a second process reading the same
        # isolated Codex root, and the counts it reports are compared with what the conversion says it
        # carried. A broken round-trip check inside ccc -- or a CLI that mapped it to "verified" -- cannot
        # make this check green.
        readback = run([str(repo / BINARY), "inspect", f"codex:{thread_id}", "--json"],
                       cwd=repo, env={"CODEX_HOME": str(home)})
        try:
            read_back = json.loads(readback.stdout).get("source", {}).get("counts", {})
        except json.JSONDecodeError:
            read_back = {}
        carried = conversion["carried"]
        # The comparison is over the counts that name evidence, because the reader normalises records it
        # reads (a standalone function_call becomes an assistant message of its own, for one), so a
        # comparison of whole message counts would compare the writer's shape with the reader's.
        agreed = (read_back.get("user_messages") == carried["user_messages"]
                  and read_back.get("tool_calls") == carried["tool_calls"]
                  and read_back.get("tool_results") == carried["tool_results"]
                  and read_back.get("compact_boundaries") == carried["compaction_boundaries"])
        detail = (f"ccc inspect codex:{thread_id}: {json.dumps(read_back)}; conversion carried: "
                  f"{carried['user_messages']} user message(s), {carried['tool_calls']} tool call(s), "
                  f"{carried['tool_results']} tool result(s), "
                  f"{carried['compaction_boundaries']} boundary(ies)")
        if readback.returncode != 0:
            detail += f"; inspect rc={readback.returncode}: {last_meaningful_line(readback.stderr)}"
        record("the inherited reader reads the installed file back as what was written", agreed, detail)

        server = AppServer(codex, home)
        view: list[str] = []
        item_kinds: list[str] = []
        try:
            server.handshake()
            loaded = server.call("thread/resume", {"threadId": thread_id})
            loaded_thread = loaded.get("thread") or {}
            record("the target harness's own loader opens the installed session",
                   loaded_thread.get("id") == thread_id and str(loaded_thread.get("path")) == str(installed),
                   f"thread/resume -> {loaded_thread.get('id')} [{loaded_thread.get('historyMode')}] "
                   f"cwd={loaded_thread.get('cwd')}")
            view, item_kinds = thread_view(server, thread_id)
        except RuntimeError as failure:
            # A server that stopped answering is a failed check with the harness's own diagnostics, not a
            # traceback: the protocol is bounded, and so is this run's verdict.
            record("the target harness's own loader opens the installed session", False, str(failure))
        finally:
            server.close()
        if view:
            print()
            print("the target harness's own view of the installed session:")
            for line in view:
                print("   ", line)
        record("the target's own view lists the converted turns",
               "userMessage" in item_kinds and "agentMessage" in item_kinds,
               f"{len(item_kinds)} item(s) in the reply's own turn list")

        # The reported invocation, checked against the directory this run installed into: Codex takes
        # its data directory from CODEX_HOME alone, so a resume command that does not name it opens
        # whichever home the caller's shell points at. Both rendered lines are read the way a shell
        # reads them (so a home the renderer quotes -- a path with a space, a Windows path -- is
        # accepted, while an unquoted home a shell would split is not).
        resume_arguments = target.get("resume_arguments") or []
        exec_resume_arguments = target.get("exec_resume_arguments") or []
        resume_environment = target.get("resume_environment") or {}
        resume_command = str(target.get("resume_command") or "")
        exec_resume_command = str(target.get("exec_resume_command") or "")
        want_resume = [codex, "resume", thread_id]
        want_exec_resume = [codex, "exec", "resume", thread_id, "<prompt>"]
        record("the reported resume command selects the installation home",
               resume_arguments == want_resume and exec_resume_arguments == want_exec_resume
               and resume_environment == {"CODEX_HOME": str(home)}
               and reported_invocation(resume_command, home, want_resume)
               and reported_invocation(exec_resume_command, home, want_exec_resume),
               f"resume_command={resume_command!r} exec_resume_command={exec_resume_command!r} "
               f"environment={resume_environment}")

        if args.resume_prompt:
            print()
            session_before = installed.stat().st_size
            # The turn runs the arguments the conversion reported -- binary, subcommand, identity --
            # rather than a command line spelled here, so the printed invocation is what this check
            # exercises. The flags this harness adds are its own isolation (no git repository, a
            # read-only sandbox, no approvals) and belong to the check, not to the reported command.
            # A report that did not check out (the check above recorded that) leaves the identity
            # spelled here, so this turn still answers whether a turn can continue.
            identity = exec_resume_arguments[1:-1] if exec_resume_arguments == want_exec_resume \
                else ["exec", "resume", thread_id]
            turn_command = [codex] + identity + [args.resume_prompt] + [
                "--skip-git-repo-check", "-c", 'sandbox_mode="read-only"', "-c", 'approval_policy="never"',
            ]
            turn = run(turn_command, cwd=repo, env={"CODEX_HOME": str(home)}, timeout=900)
            session_after = installed.stat().st_size
            record("a real turn continues from the installed session",
                   turn.returncode == 0 and session_after > session_before,
                   f"exit {turn.returncode}, session {session_before} -> {session_after} bytes")
            print()
            print("the turn's own output (the target's answer, as the harness printed it):")
            for line in (turn.stdout or turn.stderr).splitlines():
                print("   ", line)
        else:
            record("a real turn continues from the installed session", False,
                   "not attempted: pass --resume-prompt to run one", skipped=True)

        # Every promise about files this run must not change is answered here, after the loader and —
        # when it ran — the real turn: a comparison recorded beside the read-back above would let a
        # mutation during either native stage stand on an earlier verdict, and the planted session is
        # only deleted once this block has run.
        fixture_after = tree_digest(fixture_root)
        moved = sorted([name for name, digest in fixture_before.items()
                        if fixture_after.get(name) != digest]
                       + [name for name in fixture_after if name not in fixture_before])
        record("the source session is unchanged",
               not moved,
               f"sha256 {sha256(source)[:16]}… (fixture tree identical)" if not moved
               else f"the fixture tree changed: {', '.join(moved)}")
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
