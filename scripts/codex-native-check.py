#!/usr/bin/env python3
"""Native verification for the Claude Code → Codex route (goal 0002).

This harness answers the half of the goal that the gate cannot: whether the installed target session is
readable by **Codex itself** and whether a **real turn** continues from it. It builds `ccc`, converts a
committed fixture into an isolated Codex data directory, asks that directory's own app server to load
the session (`thread/resume`), reads the harness's own view of the conversation (`thread/read`), checks
that the source session and every other session in the target home are untouched, and — when
`--resume-prompt` is given — runs one real turn through `codex exec resume` and reports what the target
answered.

It is **not** a gate step: it needs the Codex binary, and a turn needs model access. `scripts/gate.py`
decides the offline half (round-trip through the inherited reader, install rules, reported losses, CLI
shape); this script is the re-derivable native evidence, the way `scripts/upstream-compare.py` is for the
carried tree's behaviour.

    python3 scripts/codex-native-check.py
    python3 scripts/codex-native-check.py --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast \
        --resume-prompt "Which file did you read, and why?"
    python3 scripts/codex-native-check.py --json          # machine-readable summary on stdout

`--model-provider`/`--model` name what the throw-away Codex configuration should use, because the
target's loader checks a session against the provider *its* configuration names: the load check only
needs a configured provider, while `--resume-prompt` needs one that can actually answer on this host
(its key comes from the environment, never from a copied credential).

Exit codes: 0 every check that ran passed; 1 a check failed; 2 the harness could not run (no Codex, no
Go toolchain, a missing fixture).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

#: The provider directory, and the two committed Claude fixtures this harness can convert. The first
#: carries dialogue, a tool call with its result and a compaction boundary; the second (TOOLING_SESSION)
#: carries a persisted reasoning trace, a subagent turn and an errored tool result, and no compaction
#: boundary, so a resumed turn still sees its tool calls.
SOURCE_PROVIDER = "claude"
SOURCE_SESSION = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"
TOOLING_SESSION = "b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef"
FIXTURE_ROOT = Path("internal", "ccc", "testdata", "fixtures")
GO_TAGS = "fts5"
BINARY = Path("bin", "ccc-native-check")

#: The unrelated session the harness plants in the target home before converting: a conversion must not
#: touch a session another agent may already have continued.
UNRELATED_NAME = "rollout-2026-01-01T00-00-00-00000000-0000-7000-8000-000000000000.jsonl"

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


def with_top_level_overrides(config: str, overrides: dict[str, str]) -> str:
    """The host configuration with the named top-level keys replaced.

    TOML keys after a table header belong to that table, so the replacements go above every header and
    the host's own copies of those keys (which are only the top-level ones, before the first header) are
    dropped: a duplicate key is a configuration error the target refuses to start with, and appending the
    replacement at the end would silently change a section instead.
    """
    kept: list[str] = []
    in_table = False
    for line in config.splitlines():
        stripped = line.strip()
        if stripped.startswith("["):
            in_table = True
        if not in_table:
            key = stripped.split("=", 1)[0].strip() if "=" in stripped else ""
            if key in overrides:
                continue
        kept.append(line)
    replaced = [f'{key} = "{value}"' for key, value in overrides.items()]
    body = "\n".join(kept).strip("\n")
    return "\n".join(replaced) + "\n\n" + body + "\n"


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
        timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=cwd, env=merged, capture_output=True, text=True, timeout=timeout)


class AppServer:
    """A minimal client for the target's own app-server protocol over stdio."""

    def __init__(self, binary: str, home: Path):
        self.next_id = 0
        self.process = subprocess.Popen(
            [binary, "app-server", "--listen", "stdio://"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, bufsize=1, env={**os.environ, "CODEX_HOME": str(home)},
        )

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

    def call(self, method: str, params: dict) -> dict:
        self.next_id += 1
        request_id = self.next_id
        payload = json.dumps({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
        assert self.process.stdin is not None
        self.process.stdin.write(payload + "\n")
        self.process.stdin.flush()
        deadline = time.time() + 120
        while time.time() < deadline:
            line = self.process.stdout.readline() if self.process.stdout else ""
            if not line:
                raise RuntimeError(f"the app server closed its output while answering {method}")
            message = json.loads(line)
            if message.get("id") != request_id:
                continue  # a notification (a warning, a status change): not this call's answer
            if "error" in message:
                raise RuntimeError(f"{method} failed: {message['error'].get('message')}")
            return message.get("result") or {}
        raise RuntimeError(f"the app server did not answer {method} in time")

    def notify(self, method: str, params: dict) -> None:
        assert self.process.stdin is not None
        self.process.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method, "params": params}) + "\n")
        self.process.stdin.flush()

    def handshake(self) -> None:
        self.call("initialize", {"clientInfo": {"name": "ccc-native-check", "version": "0.1.0"}})
        self.notify("initialized", {})


def thread_view(server: AppServer, thread_id: str) -> list[str]:
    """The target harness's own view of a thread: one line per item it shows for it."""
    result = server.call("thread/read", {"threadId": thread_id, "includeTurns": True})
    thread = result.get("thread") or {}
    lines = [f"thread {thread.get('id')} [{thread.get('historyMode')}] provider={thread.get('modelProvider')}"]
    for turn in thread.get("turns") or []:
        lines.append(f"  turn {turn.get('id')}")
        for item in turn.get("items") or []:
            kind = item.get("type")
            if kind == "userMessage":
                text = "".join(part.get("text", "") for part in item.get("content") or [])
            else:
                text = item.get("text") or ""
            lines.append(f"    {kind}: {text}")
    return lines


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

    scratch = Path(tempfile.mkdtemp(prefix="ccc-native-check-"))
    home = args.codex_home.resolve() if args.codex_home else scratch / "codex-home"
    home.mkdir(parents=True, exist_ok=True)
    seeded = ""
    if not args.codex_home:
        # The target's loader checks a session against the provider its *configuration* names, so the
        # throw-away home needs that configuration: the host's own config.toml is copied (settings, not
        # credentials — auth.json and every session file stay where they are), and the script says so
        # rather than presenting a bare home as the user's.
        host_home = Path(os.environ.get("CODEX_HOME") or (Path.home() / ".codex"))
        host_config = host_home / "config.toml"
        if host_config.is_file():
            seeded = str(host_config)
            home_config = host_config.read_text()
        else:
            home_config = ""
        overrides = {}
        if args.model_provider:
            overrides["model_provider"] = args.model_provider
        if args.model:
            overrides["model"] = args.model
        if overrides:
            home_config = with_top_level_overrides(home_config, overrides)
        if home_config:
            (home / "config.toml").write_text(home_config)
    sessions = home / "sessions" / "2026" / "01" / "01"
    sessions.mkdir(parents=True, exist_ok=True)
    unrelated = sessions / UNRELATED_NAME
    unrelated.write_text(json.dumps({"timestamp": "2026-01-01T00:00:00Z", "type": "session_meta",
                                     "payload": {"id": "unrelated", "cwd": "/tmp"}}) + "\n")

    print(f"repo:        {repo}")
    print(f"codex:       {codex}")
    print(f"codex home:  {home}")
    if seeded:
        print(f"seeded from: {seeded} (config.toml only; no credentials and no sessions were copied)")
    print(f"source:      {source.relative_to(repo)}")
    print(f"scratch:     {scratch}")
    print()

    fixture_before = tree_digest(fixture_root)
    unrelated_before = sha256(unrelated)
    existing_ids = [path.stem.rsplit("-", 5)[-5:] for path in home.rglob("rollout-*.jsonl")]
    try:
        build = run(["go", "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
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

        record("the conversion reports carried evidence and its limits",
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
               thread_id != args.session and all(thread_id not in str(entry) for entry in existing_ids),
               thread_id)
        record("the installed file is JSONL the target reads",
               all(json.loads(line) for line in installed.read_text().splitlines() if line.strip()),
               f"{installed.stat().st_size} bytes")
        record("the inherited reader reads the installed file back as what was written",
               verification["structure"]["state"] == "verified",
               json.dumps(verification["structure"].get("read_back", {})))
        record("the source session is unchanged",
               tree_digest(fixture_root) == fixture_before,
               f"sha256 {sha256(source)[:16]}… (fixture tree identical)")
        record("the unrelated session in the target home is unchanged",
               sha256(unrelated) == unrelated_before,
               f"sha256 {unrelated_before[:16]}…")

        server = AppServer(codex, home)
        try:
            server.handshake()
            loaded = server.call("thread/resume", {"threadId": thread_id})
            loaded_thread = loaded.get("thread") or {}
            record("the target harness's own loader opens the installed session",
                   loaded_thread.get("id") == thread_id and str(loaded_thread.get("path")) == str(installed),
                   f"thread/resume -> {loaded_thread.get('id')} [{loaded_thread.get('historyMode')}] "
                   f"cwd={loaded_thread.get('cwd')}")
            view = thread_view(server, thread_id)
        finally:
            server.close()
        print()
        print("the target harness's own view of the installed session:")
        for line in view:
            print("   ", line)
        record("the target's own view lists the converted turns",
               any("userMessage" in line for line in view) and any("agentMessage" in line for line in view),
               f"{len(view)} line(s)")

        if args.resume_prompt:
            print()
            session_before = installed.stat().st_size
            turn = run([
                codex, "exec", "resume", thread_id, args.resume_prompt,
                "--skip-git-repo-check", "-c", 'sandbox_mode="read-only"', "-c", 'approval_policy="never"',
            ], cwd=repo, env={"CODEX_HOME": str(home)}, timeout=900)
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
        if args.keep:
            print(f"kept {scratch} and {home}")
        else:
            shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
