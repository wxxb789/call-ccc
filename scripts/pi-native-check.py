#!/usr/bin/env python3
"""Verify the Pi route natively, both directions, against the installed Pi binary.

This is the target-side native verification for goal 0005 (Pi continuation), and it is deliberately
**not** a gate step, exactly like ``scripts/codex-native-check.py`` and ``scripts/claude-native-check.py``:
it needs the target harness installed and, for a real turn, model access. The gate keeps deciding the
offline half (the inherited reader's round trip, the install rules, the reported losses, the CLI shape).

What it does, for the direction it is asked about:

1. copies the source fixture into the run's own scratch tree and rewrites **only** the recorded working
   directory it names to a directory inside that tree, so the session it converts belongs to a path this
   run owns and the target's own lookup can find what it installs;
2. plants an unrelated session in the target home the run will write into;
3. converts through the CLI (``ccc continue … --json``), reading the conversion, installation and the
   structural read-back out of the report rather than trusting them;
4. runs Pi's **own loader** offline (``pi --mode rpc``: ``get_state``/``get_messages``) against a
   **throw-away copy** of the installed session, with an unknown-id control arm, and compares what the
   harness answered about the session with what the installed file's own bytes say;
5. with ``--resume-prompt``, runs one real turn through the printed invocation and prints the target's own
   answer under ``native_check_common.TURN_OUTPUT_MARKER``;
6. re-digests the source tree, the planted session and every session the home already held, so "nothing
   else moved" is a measurement rather than a claim.

The Pi home it uses is a scratch directory seeded with the **provider definitions** the host's own
``models.json`` holds, with every credential-bearing field dropped (``apiKey``, ``headers``) and the
chosen provider's key taken from the environment through a ``$NAME`` reference; nothing in a credential
store is read, copied or written. A host with no provider configured for the chosen model is reported as
a named skip with that reason, not as a pass.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from native_check_common import (  # noqa: E402
    PROMPT_PLACEHOLDER,
    REPORT_BLOCK_CHARS_MAX,
    REPORT_BLOCK_LINES_MAX,
    RESULTS,
    SHELL_SYNTAX_CHARACTERS,
    TURN_OUTPUT_MARKER,
    bounded_block,
    record,
    rewrite_source_cwd,
    run,
    sha256,
    shell_words,
    tree_digest,
    unquoted_text,
)

#: Where this repository keeps its committed native-shaped fixtures, and the provider directories under
#: it. A Pi fixture is one session file named by its own header id.
FIXTURE_ROOT = Path("internal/ccc/testdata/fixtures")
SOURCE_PROVIDER = "claude"
TARGET_PROVIDER = "pi"
SOURCE_SESSION = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"

#: The same session, read as a *source* by the opposite direction: the committed Pi fixture, converted
#: into a previously supported target harness through the same pipeline every other source uses.
PI_BACK_SOURCE_SESSION = "01a1c335-7098-71c2-a91b-b5295c0464f5"
PI_BACK_TARGET = "claude"

GO_TAGS = "fts5"

#: The binary this harness builds and drives. It lives under `bin/`, which `.gitignore` covers, so a run
#: leaves the repository exactly as it found it: `scripts/upstream-parity.py` refuses a tree holding a
#: file that is neither carried nor project-owned, and a build output written to the repository root
#: would be exactly that. It is a name of its own, not a sibling route's binary, so two harnesses run
#: side by side cannot overwrite each other's build mid-run.
BINARY = Path("bin", "ccc-native-check-pi")

#: The environment variables that name a source root for the conversion's reader, per provider, and the
#: variable that names the Pi data directory. They mirror the names the configuration resolves.
SOURCE_ROOT_ENV = {"claude": "CLAUDE_PROJECTS_DIR", "pi": "PI_DIR"}
PI_HOME_ENV = "PI_CODING_AGENT_DIR"

#: How each target harness's printed invocation names its data directory, so this harness reads the
#: assignment out of the route's own line rather than assuming one route's spelling twice. The home
#: variable is the one the harness reads its data directory from: none of them has a flag for it, so it
#: is set as a leading assignment. (The session word itself is not spelled here: the check compares the
#: printed line against the argument vector the report printed, which already carries the route's own
#: session flag.)
TARGET_INVOCATION = {
    "pi": {"home_env": PI_HOME_ENV},
    "claude": {"home_env": "CLAUDE_CONFIG_DIR"},
}

PI_SESSION_SUBDIR = "sessions"
UNRELATED_PROJECT = "unrelated-project"
UNRELATED_SESSION = "00000000-0000-4000-8000-0000000000ff"

#: The workspace the rewritten source session belongs to, relative to the scratch root. It is one segment
#: so the project directory Pi derives from it is a single readable name.
WORKSPACE = "workspace"

#: The words a rendered invocation begins with and the operator between them: the directory change, and
#: the `&&` a shell reads as its own operator. They mirror `internal/ccc/continue.go`'s `renderInvocation`.
CD_WORD = "cd"
OPERATOR_WORD = "&&"


def pi_project_dir(cwd: str) -> str:
    """The directory Pi files one working directory's sessions under: the leading separator removed,
    every `/`, `\\` and `:` replaced by `-`, wrapped in `--` on both sides. It mirrors the Go function
    ``PiProjectDir`` (internal/ccc/pi_target.go), which is itself the spelling the installed harness's own
    ``getDefaultSessionDirPath`` computes."""
    trimmed = cwd.replace("\\", "/").lstrip("/")
    for character in ("/", ":"):
        trimmed = trimmed.replace(character, "-")
    return f"--{trimmed}--"


def pi_session_file_name(timestamp: str, session_id: str) -> str:
    """The name Pi's own writer gives a session file, from an ISO 8601 instant: separators replaced by
    dashes and the session id appended."""
    stamp = timestamp.replace(":", "-").replace(".", "-")
    return f"{stamp}_{session_id}.jsonl"


def host_pi_providers(host_agent_dir: Path) -> dict:
    """The provider definitions the host's own Pi ``models.json`` holds, or an empty mapping when the
    host has none.

    "A host with no provider configured for the chosen model" is a state the module docstring promises to
    report as a named skip, not a crash: a missing file, or a file that holds no ``providers`` object (its
    own schema is all-or-nothing), is that state, and a run that meets one still has to reach its
    ``RESULT:`` line. The read is guarded here so every caller -- the seeding below and the run's own
    provider selection -- answers the same document rather than each re-reading the file.
    """
    models_path = host_agent_dir / "models.json"
    if not models_path.is_file():
        return {}
    try:
        document = json.loads(models_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    providers = document.get("providers") if isinstance(document, dict) else None
    return providers if isinstance(providers, dict) else {}


def seed_pi_home(home: Path, host_agent_dir: Path, providers: list[str]) -> tuple[list[str], list[str]]:
    """Seed a scratch Pi agent directory with the host's provider definitions, minus every credential.

    Pi's ``models.json`` is read as a whole document and its schema is all-or-nothing, so a provider the
    host holds that this run does not need is left out rather than copied and hoped about; a provider whose
    ``apiKey`` is dropped must still resolve a credential from the environment, which is why the key is
    written back as an ``$ENV`` reference. Answers the provider names seeded and the ones left out.
    """
    host_models = host_pi_providers(host_agent_dir)
    seeded: dict[str, object] = {}
    left_out: list[str] = []
    for name, provider in host_models.items():
        if name not in providers:
            left_out.append(name)
            continue
        kept = {k: v for k, v in provider.items() if k not in ("apiKey", "headers")}
        reference = provider.get("apiKey")
        if isinstance(reference, str) and reference.startswith("$"):
            kept["apiKey"] = reference
        else:
            # The host names its credential in the auth store instead; the environment variable Pi's own
            # provider-id convention derives is the one this run points at, and it is named in the report.
            kept["apiKey"] = "$" + name.upper().replace("-", "_") + "_API_KEY"
        seeded[name] = kept
    if not seeded:
        return [], left_out
    home.mkdir(parents=True, exist_ok=True)
    (home / "models.json").write_text(json.dumps({"providers": seeded}, indent=1), encoding="utf-8")
    return sorted(seeded), sorted(left_out)


def first_user_text(session: Path) -> str:
    """The first user prompt one session file records, whatever transcript shape wrote it.

    The two shapes this harness reads spell the role differently -- a Pi session writes a `message` entry
    whose payload carries `role: "user"`, a Claude transcript a record whose own `type` is `user` -- so
    the role is read from either place and the prompt's text is taken from the record's content. That is
    the whole reader a source fixture needs here; nothing else about either format is interpreted.
    """
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        message = entry.get("message")
        if not isinstance(message, dict):
            continue
        role = message.get("role") or entry.get("type")
        if role != "user":
            continue
        content = message.get("content")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            for block in content:
                if isinstance(block, dict) and block.get("type") == "text":
                    return block.get("text", "")
    return ""


def installed_entry_counts(session: Path) -> dict[str, int]:
    """What one installed Pi session file holds, from its own bytes: entries by record type, and the
    message roles."""
    counts: dict[str, int] = {}
    roles: dict[str, int] = {}
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        kind = entry.get("type", "")
        counts[kind] = counts.get(kind, 0) + 1
        if kind == "message":
            role = (entry.get("message") or {}).get("role", "")
            roles[role] = roles.get(role, 0) + 1
    counts.update({f"role:{role}": value for role, value in roles.items()})
    return counts


def assistant_messages_without_usage(session: Path) -> tuple[int, int]:
    """How many `message` records one session file holds whose role is `assistant`, and how many of them
    lack a `usage` object.

    Pi's own loader indexes `message.usage` on every assistant message it reads, so an assistant record
    without one is a file the target would not read as written. The count is taken from the parsed
    records, the same walk `installed_entry_counts` makes: a whole-file substring search for `usage` is
    satisfied by a *message text* that happens to spell the word, which is not a usage object and is not
    what the target's loader reads.
    """
    assistants = 0
    without = 0
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if entry.get("type") != "message":
            continue
        message = entry.get("message")
        if not isinstance(message, dict) or message.get("role") != "assistant":
            continue
        assistants += 1
        if not isinstance(message.get("usage"), dict):
            without += 1
    return assistants, without


def session_id_from_header(session: Path) -> str:
    """The session id one Pi session file's own header line records, or "" when it holds none.

    A Pi session file is named ``<timestamp>_<id>.jsonl`` and its first record is the `session` entry
    whose `id` field is the identity the harness's own writer gave it. The identity is read from that
    record, never inferred from the file's name: the name's stem carries the timestamp as well, so two
    files recording the *same* session id under different timestamps read back as two different names.
    """
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if entry.get("type") == "session":
            return str(entry.get("id", ""))
    return ""


def home_holds_id(home: Path, session_id: str, exclude: Path | None = None) -> bool:
    """Whether any session file below `home`, other than `exclude`, records `session_id` in its header.

    The ids are read from each file's own header (`session_id_from_header`) rather than from its name, so
    a conversion that reused an id the home already held under another timestamp is caught instead of
    passing on a differing file name. `exclude` is the file this run just installed, whose header holds
    the same id by construction.
    """
    if not session_id:
        return False
    for path in home.rglob("*.jsonl"):
        if exclude is not None and path == exclude:
            continue
        if session_id_from_header(path) == session_id:
            return True
    return False


def pi_tool_names(session: Path) -> list[str]:
    """Every tool name one session file's call blocks record, in order."""
    names: list[str] = []
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if entry.get("type") != "message":
            continue
        content = (entry.get("message") or {}).get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if isinstance(block, dict) and block.get("type") == "toolCall":
                names.append(block.get("name", ""))
    return names


def target_binary_name(target_provider: str) -> str:
    """The executable a run's target harness is driven through, per target provider."""
    return {"pi": "pi", "claude": "claude", "codex": "codex"}[target_provider]


def ask_pi_loader(pi: str, home: Path, cwd: Path, session_id: str, timeout: int = 90) -> tuple[str, str]:
    """Ask Pi's own loader for one session's state and messages, offline, and answer its stdout and
    stderr. The session is resolved by id through the agent directory's default layout, which is where a
    conversion installs it, and the process runs from the session's own working directory.

    The caller points this at a **copy** of the session, never at the installed file: measured on 1.5.2,
    an RPC run appends its own records (a `thinking_level_change` among them) to the session it opens, so
    driving the loader against the installed file would alter what the conversion published. The
    installed file's own digest is compared around the whole load stage, which is what makes that
    measurement an assertion rather than a note."""
    commands = "\n".join([
        json.dumps({"id": "state", "type": "get_state"}),
        json.dumps({"id": "messages", "type": "get_messages"}),
    ]) + "\n"
    environment = dict(os.environ)
    environment[PI_HOME_ENV] = str(home)
    environment.pop("PI_CODING_AGENT_SESSION_DIR", None)
    completed = subprocess.run(
        [pi, "--mode", "rpc", "--session", session_id, "--no-tools", "--offline"],
        input=commands, cwd=str(cwd), env=environment, capture_output=True, text=True, timeout=timeout,
    )
    return completed.stdout, completed.stderr


def rpc_answer(stdout: str, command: str) -> dict | None:
    """The last response record for one command out of an RPC run's stdout, or None."""
    answer: dict | None = None
    for line in stdout.splitlines():
        stripped = line.strip()
        if not stripped.startswith("{"):
            continue
        try:
            parsed = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if parsed.get("type") == "response" and parsed.get("command") == command:
            answer = parsed
    return answer


def model_visible_roles(session: Path) -> list[str]:
    """The roles the harness's own loader builds from one session file, in order: the latest compaction
    entry **on the path the loader walks** (reported as ``compactionSummary``), then every message entry
    after it on that path, and nothing off the path. It mirrors the Go function ``piModelVisibleRoles``
    (internal/ccc/pi_verify.go).

    The walk is the harness's own, re-derived from its answer rather than assumed (measured on 1.5.2): the
    loader starts at the file's **last** entry, follows ``parentId`` back to the tree's root and rebuilds
    the reversed path, so an entry the file holds but no later record hangs off is recorded history and
    not context, and a compaction entry standing off that path is not a boundary anything is rebuilt from.
    The path is read from the file's own ``parentId`` values, never from file order: a parentId the file
    does not hold, a record naming itself and a loop each end the walk where they are reached, which is the
    same partial path the loader would have. Reading the latest compaction in file order instead is only
    accidentally right for single-chain files -- the committed branch fixture's compaction entry and the
    turn after it stand off its last entry's path, and the loader answers ``[user, assistant, assistant]``
    (no ``compactionSummary``).
    """
    records: list[dict] = []
    for line in session.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if entry.get("type") in ("", "session"):
            continue
        records.append(entry)
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
    for index, entry in enumerate(path):
        if entry.get("type") == "compaction":
            boundary = index
    roles: list[str] = []
    if boundary >= 0:
        roles.append("compactionSummary")
    for entry in path[boundary + 1:]:
        if entry.get("type") != "message":
            continue
        message = entry.get("message")
        roles.append(message.get("role", "") if isinstance(message, dict) else "")
    return roles


def bounded(text: str, limit: int) -> str:
    text = (text or "").strip()
    if len(text) <= limit:
        return text
    return text[:limit] + " …[the rest of this output was dropped]"


def print_answer(output: str) -> None:
    print(TURN_OUTPUT_MARKER)
    for line in bounded_block(output, REPORT_BLOCK_LINES_MAX, REPORT_BLOCK_CHARS_MAX):
        print("    " + line)
    print()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--pi", default="pi", help="the Pi binary to verify against")
    parser.add_argument("--claude", default="claude",
                        help="the Claude Code binary to verify against for the pi-to-claude direction "
                             "(default: claude on PATH)")
    parser.add_argument("--direction", choices=("claude-to-pi", "pi-to-claude"), default="claude-to-pi",
                        help="which route to verify (default: the Pi target)")
    parser.add_argument("--source", default=None,
                        help=f"the source session to convert (default: the committed "
                             f"{SOURCE_SESSION} Claude fixture, or the Pi fixture for pi-to-claude)")
    parser.add_argument("--target-home", type=Path,
                        help="the target harness's data directory (default: a scratch one)")
    parser.add_argument("--resume-prompt",
                        help="run one real turn in the installed session with this prompt")
    parser.add_argument("--expect", help="require the resume turn's answer to contain this text")
    parser.add_argument("--pi-dir", type=Path,
                        help="the host's Pi agent directory to seed provider definitions from "
                             "(default: $PI_CODING_AGENT_DIR, else ~/.pi/agent)")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    pi = shutil.which(args.pi)
    if pi is None:
        print(f"{args.pi} is not on PATH: this harness verifies native behaviour, which needs the target "
              f"harness installed", file=sys.stderr)
        return 2
    go = shutil.which("go")
    if go is None:
        print("go is not on PATH: this harness builds ccc with the Go toolchain", file=sys.stderr)
        return 2

    host_pi_dir = args.pi_dir or (
        Path(os.environ[PI_HOME_ENV]) if os.environ.get(PI_HOME_ENV) else Path.home() / ".pi" / "agent")

    scratch = Path(tempfile.mkdtemp(prefix="ccc-pi-native-check-")).resolve()
    workspace = scratch / WORKSPACE
    workspace.mkdir(parents=True, exist_ok=True)

    if args.direction == "claude-to-pi":
        source_provider, target_provider = SOURCE_PROVIDER, TARGET_PROVIDER
        source_session = args.source or SOURCE_SESSION
        candidates = sorted((fixture_root / source_provider).rglob(f"*{source_session}.jsonl"))
    else:
        source_provider, target_provider = "pi", PI_BACK_TARGET
        source_session = args.source or PI_BACK_SOURCE_SESSION
        candidates = sorted((fixture_root / source_provider).glob(f"{source_session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {source_provider} fixture for {source_session} below "
              f"{fixture_root / source_provider}, found {len(candidates)}", file=sys.stderr)
        return 2
    source = candidates[0]

    source_root = scratch / f"{source_provider}-source"
    destination = source_root / source.relative_to(fixture_root / source_provider)
    recorded_cwd = rewrite_source_cwd(source, destination, workspace)

    home = args.target_home.resolve() if args.target_home else scratch / f"{target_provider}-home"
    home.mkdir(parents=True, exist_ok=True)
    seeded_providers: list[str] = []
    left_out: list[str] = []
    if target_provider == TARGET_PROVIDER:
        wanted = [reference for reference in [os.environ.get("CCC_PI_NATIVE_PROVIDER")] if reference]
        if not wanted:
            wanted = sorted(host_pi_providers(host_pi_dir))
        seeded_providers, left_out = seed_pi_home(home, host_pi_dir, wanted)
        if not wanted:
            # The module docstring's promise: a host with no provider configured for the chosen model is
            # a named skip with that reason, not a crash and not a pass. The conversion can still run --
            # the provider is what a *turn* needs -- so the run continues and records the skip beside its
            # other checks rather than stopping at the first state this host cannot satisfy.
            record("a Pi provider is configured for the chosen model", False,
                   f"no provider configured: {host_pi_dir / 'models.json'} names no provider and "
                   f"$CCC_PI_NATIVE_PROVIDER is unset, so no provider definition was seeded for this run",
                   skipped=True)

    unrelated = home / PI_SESSION_SUBDIR / f"--{UNRELATED_PROJECT}--" / \
        pi_session_file_name("2026-01-01T00-00-00-000Z", UNRELATED_SESSION)
    if unrelated.exists():
        print(f"{unrelated} already exists: this harness plants its own fabricated session under that name "
              f"and will not overwrite one that is there", file=sys.stderr)
        return 2
    unrelated.parent.mkdir(parents=True, exist_ok=True)
    unrelated.write_text(json.dumps({
        "type": "session", "version": 3, "id": UNRELATED_SESSION,
        "timestamp": "2026-01-01T00:00:00.000Z", "cwd": f"/home/user/{UNRELATED_PROJECT}",
        "records": "a session this run must not touch",
    }) + "\n", encoding="utf-8")

    # The pi-to-claude direction's target harness is named by `--claude` (the sibling's own flag), so a
    # Claude Code binary that is not on PATH can be named directly; every other direction drives the Pi
    # binary the `--pi` flag resolves. The same word is passed to the CLI as `--target-binary` and read
    # back out of the report, so the invocation this run checks is the one it drove.
    target_binary = args.claude if target_provider == PI_BACK_TARGET else target_binary_name(target_provider)
    target_path = shutil.which(target_binary)

    print(f"repo:        {repo}")
    print(f"direction:   {args.direction} ({source_provider}:{source_session} → {target_provider})")
    print(f"target:      {target_binary} ({target_path or 'not on PATH'})")
    if target_provider == PI_BACK_TARGET:
        print(f"claude:      {target_binary}")
    if target_provider == TARGET_PROVIDER:
        print(f"pi:          {pi}" + (f" ({run([pi, '--version']).stdout.strip()[:40]})" if pi else ""))
    print(f"source:      {source.relative_to(repo)} (copied to {destination})")
    if target_provider == TARGET_PROVIDER:
        print(f"workspace:   {workspace} -> project dir {pi_project_dir(str(workspace))}")
    else:
        # The installed path here is the target's own spelling, not Pi's, so naming Pi's project
        # directory for this direction would name a directory that does not exist.
        print(f"workspace:   {workspace}")
    print(f"target home: {home}")
    if target_provider == TARGET_PROVIDER:
        print(f"pi models:   seeded provider(s) {', '.join(seeded_providers) or '(none)'} from {host_pi_dir}"
              + (f"; left out (not needed here): {', '.join(left_out)}" if left_out else "")
              + "; every credential field dropped, keys resolved from the environment")
    print(f"the source fixture's recorded working directory ({recorded_cwd or 'none recorded'}) is rewritten "
          f"to the workspace, so the session belongs to a path this run owns and the target's own lookup "
          f"can find what it installs")
    print()

    source_root_before = tree_digest(source_root)
    planted_before = sha256(unrelated)
    existing_sessions = {path: sha256(path) for path in home.rglob("*.jsonl") if path != unrelated}

    # Everything below runs inside a `finally` that removes the fabricated unrelated session this harness
    # planted: it is removed however the run ends -- after every check, or from any of the branches that
    # return early -- so a caller-named `--target-home` is left as it was found apart from the converted
    # session this run installed, and a second run against the same home plants its own session instead of
    # meeting its own leftover.
    try:
        build_parent = (repo / BINARY).parent
        build_parent.mkdir(parents=True, exist_ok=True)
        build = run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
        if build.returncode != 0:
            record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
            return 2
        record("ccc builds with the flags upstream ships", True)

        env = {SOURCE_ROOT_ENV[source_provider]: str(source_root)}
        command = [str(repo / BINARY), "continue", f"{source_provider}:{source_session}", "--to",
                   target_provider, "--target-home", str(home), "--target-binary", target_binary, "--json"]
        if args.resume_prompt:
            command += ["--verify", "resume", "--resume-prompt", args.resume_prompt]
        convert = run(command, cwd=repo, env=env)

        document: dict = {}
        try:
            document = json.loads(convert.stdout)
        except json.JSONDecodeError:
            record("ccc continue writes one JSON result document", False,
                   bounded(convert.stdout or convert.stderr, 400))
            return 1
        record("ccc continue writes one JSON result document", True)

        conversion = document.get("conversion") or {}
        installation = document.get("installation") or {}
        target = document.get("target_session") or {}
        verification = document.get("verification") or {}
        losses = {loss.get("category"): loss.get("count") for loss in conversion.get("losses") or []}
        carried = conversion.get("carried") or {}

        record("the conversion reports the conversation it carried and every loss category",
               conversion.get("state") == "converted" and carried.get("messages", 0) > 0 and bool(losses),
               f"carried {carried.get('messages')} message(s), {carried.get('tool_calls')} call(s), "
               f"{carried.get('tool_results')} result(s); loss categories named: {len(losses)}")
        record("the structural read-back verified the installed session",
               (verification.get("structure") or {}).get("state") == "verified",
               bounded((verification.get("structure") or {}).get("detail", ""), 300))
        load_state = (verification.get("load") or {}).get("state", "")
        load_detail = (verification.get("load") or {}).get("detail", "")
        if target_path is None:
            # The target harness is not installed: its own loader cannot run, and this harness says so
            # with the CLI's own words instead of reading the absent verification as a failed conversion.
            record(f"the target harness ({target_binary}) is installed", False,
                   f"not on PATH; the CLI reported the load check as {load_state or 'absent'}"
                   + (f": {bounded(load_detail, 200)}" if load_detail else ""))
            record("ccc continue exits 0 for a conversion whose conversion and read-back ran",
                   convert.returncode == 0,
                   f"exit {convert.returncode}; stderr {bounded(convert.stderr, 300)}")
        else:
            record("ccc continue exits 0 for a conversion whose verification ran", convert.returncode == 0,
                   f"exit {convert.returncode}; stderr {bounded(convert.stderr, 300)}")
        session_id = target.get("session_id", "")
        installed = Path(target.get("path", ""))
        if not session_id or not installed.is_file():
            record("the installed session exists at the reported path", False,
                   f"session_id={session_id!r} path={target.get('path')!r}")
            _report(args, scratch, document)
            return 1
        record("the installed session exists at the reported path", True,
               f"{installed.relative_to(home)} [{(installation or {}).get('mechanism')}]")

        counts = installed_entry_counts(installed)
        if target_provider == TARGET_PROVIDER:
            record("the installed file is a Pi session, installed where its working directory spells",
                   installed.parent == home / PI_SESSION_SUBDIR / pi_project_dir(str(workspace))
                   and counts.get("session") == 1,
                   f"{installed.relative_to(home)}; entries {dict(sorted(counts.items()))}")
            prompt = first_user_text(destination)
            record("the installed file carries the source's own first prompt",
                   bool(prompt) and prompt.strip() in installed.read_text(encoding="utf-8"),
                   f"{prompt[:60]!r}")
            tools = pi_tool_names(installed)
            record("the installed file holds the source's tool call and its result",
                   bool(tools) and counts.get("role:toolResult", 0) >= 1
                   and carried.get("tool_calls", 0) == len(tools),
                   f"call block(s) {tools}, toolResult record(s) {counts.get('role:toolResult', 0)}")
            assistants, without_usage = assistant_messages_without_usage(installed)
            record("the installed file's assistant records carry a usage object",
                   assistants > 0 and without_usage == 0,
                   f"{assistants} assistant message(s), {without_usage} without a usage object")

        # The report's own load state, and then Pi's loader asked again by this harness: the report is one
        # read of the harness's answer, and this is another, against the file rather than against the
        # report.
        if target_provider == TARGET_PROVIDER:
            report = verification.get("load") or {}
            record("the load check verified the installed session natively",
                   report.get("state") == "verified", bounded(report.get("detail", ""), 400))
            _check_pi_loader(pi, home, workspace, session_id, installed)
        else:
            # The target here is not Pi: the CLI's own verification ran that harness's loader
            # (`verification.load`), and its state is recorded so a PASS for this direction rests on the
            # loader's own answer and not only on the CLI's exit code. When the harness is absent there is
            # no loader answer to verify, and the dedicated check above already says why -- so this one is
            # recorded only when the harness was there to answer.
            if target_path is not None:
                record("the load check verified the installed session natively",
                       load_state == "verified", bounded(load_detail, 400))

        resume = verification.get("resume") or {}
        if args.resume_prompt:
            record("the resume check verified one real turn through the printed invocation",
                   resume.get("state") == "verified", bounded(resume.get("detail", ""), 400))
            print_answer(resume.get("output", ""))
            if args.expect:
                record(f"the turn's answer contains {args.expect!r}",
                       args.expect in (resume.get("output") or ""),
                       bounded(resume.get("output", ""), 160))

        _check_invocation(target, target_provider, workspace, home)

        source_root_after = tree_digest(source_root)
        record("the source fixture tree is unchanged", source_root_after == source_root_before,
               f"{len(source_root_before)} file(s) digested before and after")
        record("the planted unrelated session is unchanged", sha256(unrelated) == planted_before)
        moved = [str(path) for path, digest in existing_sessions.items()
                 if not path.exists() or sha256(path) != digest]
        record("every session the home already held is unchanged", not moved, "; ".join(moved[:3]))
        if target_provider == TARGET_PROVIDER:
            record("the conversion wrote no id the home already held",
                   not home_holds_id(home, session_id, exclude=installed),
                   f"{session_id} is new among the session id(s) the home's own header lines record")

        _report(args, scratch, document)
        failures = [name for name, state, _ in RESULTS if state == "FAIL"]
        missing_target = f"the target harness ({target_binary}) is installed"
        if failures == [missing_target]:
            # Everything this harness can check without the target harness's own loader passed, and the
            # one check that needs it says why it could not run. That is an INCOMPLETE run, not a failed
            # one: it is the same distinction scripts/upstream-parity.py draws for a tree it cannot
            # enumerate, and a reader of the verdict must not take it for either a pass or a defect.
            print(f"RESULT: INCOMPLETE — {len(RESULTS) - 1} check(s) ran, none failed; "
                  f"{target_binary} is not on PATH, so the native half of {args.direction} could not run")
            return 2
        if failures:
            print(f"RESULT: FAIL — {len(failures)} check(s) failed: {', '.join(failures)}")
            return 1
        print(f"RESULT: PASS — {len(RESULTS)} check(s), none failed")
        return 0
    finally:
        # The fabricated session belongs to this harness and no one else: it is removed even from a home
        # the caller named with `--target-home`, so the home is left as it was found apart from the
        # converted session this run installed, and a second run does not meet this harness's own
        # leftover.
        unrelated.unlink(missing_ok=True)


def _check_pi_loader(pi: str, home: Path, workspace: Path, session_id: str, installed: Path) -> None:
    """Ask Pi's own loader about the installed session and about an unknown id, and compare its answers
    with the installed file's own bytes."""
    before = sha256(installed)
    loader_home = Path(tempfile.mkdtemp(prefix="ccc-pi-loader-home-"))
    try:
        relative = installed.relative_to(home)
        copy = loader_home / relative
        copy.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(installed, copy)
        for name in ("models.json", "settings.json"):
            source = home / name
            if source.is_file():
                shutil.copyfile(source, loader_home / name)
        try:
            stdout, stderr = ask_pi_loader(pi, loader_home, workspace, session_id)
        except subprocess.TimeoutExpired:
            record("the harness's own loader answers about the installed session", False,
                   "the loader did not answer inside its deadline")
            return
    finally:
        shutil.rmtree(loader_home, ignore_errors=True)
    state = rpc_answer(stdout, "get_state")
    messages = rpc_answer(stdout, "get_messages")
    if state is None or not state.get("success"):
        record("the harness's own loader answers about the installed session", False,
               bounded(stderr or stdout, 300))
        return
    data = state.get("data") or {}
    record("the harness's own loader answers about the installed session",
           data.get("sessionId") == session_id,
           f"sessionId {data.get('sessionId')!r}, messageCount {data.get('messageCount')}")
    roles = [(message or {}).get("role", "") for message in ((messages or {}).get("data") or {}).get("messages", [])]
    record("the loader's own message list is the file's model-visible history",
           roles == model_visible_roles(installed),
           f"loader {roles} vs file {model_visible_roles(installed)}")

    unknown = "01a1c335-dead-beef-0000-000000000000"
    control_home = Path(tempfile.mkdtemp(prefix="ccc-pi-loader-control-"))
    try:
        control_out, control_err = ask_pi_loader(pi, control_home, workspace, unknown)
    finally:
        shutil.rmtree(control_home, ignore_errors=True)
    record("the unknown-id control arm reports no such session",
           "No session found matching" in (control_err or "") and rpc_answer(control_out, "get_state") is None,
           bounded(control_err or control_out, 200))
    # Measured: an RPC run appends its own records to the session it opens, so this comparison is what
    # makes the check's own copy the mechanism rather than a preference.
    record("the load check ran against a copy and left the installed file alone",
           sha256(installed) == before)


def spelled_as_invocation(command: str, home_env: str) -> bool:
    """Whether the line's own spelling makes a shell read it as `<cd> <dir> && NAME=<home> <words>`.

    `unquoted_text` keeps the characters a shell reads *without* quoting, so the line's structure is read
    off it: the opening `cd` word, then `&&` as an operator word of its own, then the assignment's own
    `<home_env>=` prefix, with no other unquoted character a shell would act on. The same three spellings
    the sibling ``scripts/claude-native-check.py``'s `spelled_as_invocation` refuses are refused here: a
    quoted assignment (`… && 'NAME=…' …` is a *program* by that name to a shell, so nothing sets the
    variable and the harness opens the caller's own data directory); a character a shell acts on left
    unquoted inside the value (`NAME=a;b` sets the variable to `a` and runs `b`); and an operator glued to
    a word (`cd /a&&b …` is a directory named `/a&&b` to a shell, not a change followed by an assignment)
    -- which the whitespace test below answers, because a `&&` a shell reads as its operator is a word of
    its own and therefore has whitespace on both sides.
    """
    text = unquoted_text(command)
    if text is None:
        return False
    head, operator, tail = text.partition(OPERATOR_WORD)
    if operator == "" or OPERATOR_WORD in tail:
        return False
    if head.split()[:1] != [CD_WORD] or not head[-1:].isspace() or not tail[:1].isspace():
        return False
    if not tail.lstrip().startswith(f"{home_env}="):
        return False
    forbidden = set(SHELL_SYNTAX_CHARACTERS)
    return not (set(head) | set(tail)) & forbidden


def reported_invocation(command: str, workspace, home, home_env: str, arguments: list[str]) -> bool:
    """Whether `command` is the invocation as a POSIX shell reads it: the `cd`, then the assignment, then
    `arguments`.

    The target's loader finds a session through the working directory it is started in (Pi resolves a
    session id through the project directory of the process's own working directory), and it takes its
    agent directory from the environment alone -- it has no flag for either -- so a pasted line that names
    neither resumes nothing, or resumes the caller's own session, rather than the one this run installed.
    The full word vector is compared, exactly as the sibling ``scripts/claude-native-check.py``'s
    ``reported_invocation`` does: any `cd <anything>` beside any `NAME=<anything>` beside any session id
    reads back as the right *shape* and is still another invocation, so a substring or a shape test would
    accept a line that resumes somewhere else. The words alone do not answer it either
    (`spelled_as_invocation` reads the spelling too), which is why both halves decide this rule.
    """
    return (shell_words(command) == [CD_WORD, str(workspace), OPERATOR_WORD,
                                     f"{home_env}={home}", *arguments]
            and spelled_as_invocation(command, home_env))


def _check_invocation(target: dict, target_provider: str, workspace, home) -> None:
    """Check the printed resume commands by reading them the way a POSIX shell reads them: word by word,
    against the working directory and the data directory this run used and the argument vector the report
    itself printed, with nothing a shell would split or act on left bare (the spelling
    `spelled_as_invocation` reads, the same rule the sibling harness applies)."""
    spec = TARGET_INVOCATION[target_provider]
    for field, arguments_field in (("resume_command", "resume_arguments"),
                                   ("exec_resume_command", "exec_resume_arguments")):
        command = target.get(field, "")
        arguments = target.get(arguments_field) or []
        ok = (bool(command) and bool(arguments)
              and reported_invocation(command, workspace, home, spec["home_env"], arguments))
        record(f"the printed {field} reads as a POSIX shell line naming the session", ok,
               bounded(command, 200))


def _report(args: argparse.Namespace, scratch: Path, document: dict) -> None:
    if args.keep:
        print(f"scratch kept at {scratch}")
    else:
        shutil.rmtree(scratch, ignore_errors=True)
        print(f"scratch removed ({scratch})")
    if args.json:
        print(json.dumps({
            "direction": args.direction,
            "checks": [{"name": name, "state": state, "detail": detail} for name, state, detail in RESULTS],
            "target_session": (document.get("target_session") or {}).get("session_id", ""),
            "conversion": (document.get("conversion") or {}).get("state", ""),
        }, indent=2))


if __name__ == "__main__":
    sys.exit(main())
