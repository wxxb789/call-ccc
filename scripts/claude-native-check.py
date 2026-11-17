#!/usr/bin/env python3
"""Native verification for the Codex → Claude Code route (goal 0003).

This harness answers the half of the goal that the gate cannot: whether the installed target session is
readable by **Claude Code itself** and whether a **real turn** continues from it. It builds `ccc`,
converts a committed Codex fixture into an isolated Claude Code data directory, re-reads the installed
file with its own second `ccc` invocation (the inherited reader, so the conversion's own verdict about
itself decides nothing here), checks the resume invocation the conversion reported (its words name the
target binary, the resume subcommand, the session, the working directory the loader has to be started
from and the data directory the session was installed into, and the line is spelled so a POSIX shell
reads it as that `cd` and that assignment rather than as different words), asks the engine's own load
check to have the harness's loader resolve the session (`claude --bare --resume`, whose report line is
the engine's own words about the unknown-id control arm), and — when `--resume-prompt` is given — runs
the printed exec resume line with the caller's prompt as printed and reports what the target answered.
Only then — after the loader and the turn, and before the cleanup that removes the planted session —
does it compare the source fixture copy, the session it plants in the target home and every other
session the home already held against the bytes they had before the conversion, so a mutation during
either native stage is answered by the comparison that runs last rather than by an earlier verdict
standing in for it. The planted session is removed again when the run ends, so a `--claude-home` the
caller named is left as it was found apart from the converted session.

The target's loader finds a session **through the working directory it is run from**: measured on Claude
Code 2.1.326, the same session id answers `No conversation found with session ID: <id>` from any
directory whose project-dir spelling differs. The committed fixture records `/home/user/code/example-api`,
a path no run here could be started from, so this harness copies the fixture into a scratch Codex source
root and rewrites that one field to a scratch workspace directory it creates — mechanically, for that
reason, and the run header says so. Nothing is seeded into the target home: a Claude Code transcript
needs no configuration, and this harness never writes or copies a credential (the load check runs the
harness with the variables that could reach a model removed, which is the engine's own doing, and the
turn uses the caller's environment and nothing else).

It is **not** a gate step: it needs the Claude Code binary installed and, for a turn, model access.
`scripts/gate.py` decides the offline half (round-trip through the inherited reader, install rules,
reported losses, CLI shape); this script is the re-derivable native evidence, the way
`scripts/upstream-compare.py` is for the carried tree's behaviour and `scripts/codex-native-check.py` is
for the other route.

    python3 scripts/claude-native-check.py
    python3 scripts/claude-native-check.py --claude /path/to/claude \\
        --resume-prompt "Summarise in one sentence what this conversation records about retry attempts."
    python3 scripts/claude-native-check.py --json          # machine-readable summary on stdout

`--resume-prompt` needs a harness that can actually answer on this host (its model access comes from the
caller's environment, never from this script); the load check and every other check need no model access
at all. `--expect TEXT` makes the resume check's verdict depend on the answer containing that text.

Exit codes: 0 every check that ran passed; 1 a check failed; 2 the harness could not run (no Claude Code
binary, no Go toolchain, a missing fixture).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile
from pathlib import Path

#: The provider and the committed fixture this harness converts: a Codex rollout with 6 messages, one
#: tool call with its result and one `compacted` boundary.
SOURCE_PROVIDER = "codex"
SOURCE_SESSION = "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"
TARGET_PROVIDER = "claude"
FIXTURE_ROOT = Path("internal", "ccc", "testdata", "fixtures")
GO_TAGS = "fts5"

#: The binary this harness builds and drives. It is a name of its own, not the Codex route's
#: `bin/ccc-native-check`: the two harnesses are run side by side, and one overwriting the other's binary
#: mid-run would make a failure unreadable.
BINARY = Path("bin", "ccc-native-check-claude")

#: Where inside the scratch directory the scratch Codex source root and the working directory the
#: fixture's recorded `cwd` is rewritten to live. Named here so the offline cases in
#: ``scripts/tests/test_claude_native_check.py`` can derive the same paths the shipped ``main`` does.
CODEX_SOURCE_DIR = Path("codex-source")
WORKSPACE = Path("workspace", "example-api")

#: The subdirectory of a Claude Code data directory that holds its session transcripts, and the
#: unrelated session this harness plants there before converting: a conversion must not touch a session
#: another agent may already have continued, and must not reuse its identity.
CLAUDE_PROJECTS_SUBDIR = "projects"
UNRELATED_SESSION = "0f1e2d3c-4b5a-4978-8c9d-0e1f2a3b4c5d"
UNRELATED_PROJECT = "-ccc-native-check-unrelated"

#: The environment variable the target harness takes its data directory from -- it has no flag for it --
#: and the words the rendered invocation is made of: the directory change, its operator, the assignment
#: the harness reads the data directory from, and the placeholder the printed exec form carries where a
#: turn's prompt goes.
CLAUDE_HOME_ENV = "CLAUDE_CONFIG_DIR"
CD_WORD = "cd"
OPERATOR_WORD = "&&"
PROMPT_PLACEHOLDER = "<prompt>"

#: What the load check's control arm must say for the load check to be an answer about this session
#: rather than about the loader starting at all: measured on Claude Code 2.1.326, the same command with an
#: unknown session id exits non-zero with no result document and this line on stderr.
CONTROL_ARM_TEXT = "No conversation found with session ID"

#: The marker the resume check prints the target's own answer under, and the bound on that block. Both
#: are part of the reporting contract: `scripts/tests/test_evidence_claims.py` reads the answer a
#: recorded run credits from this marker, so the answer is printed under it, one answer line per printed
#: line, and cut to a bound that a runaway answer cannot turn into this harness's own memory.
TURN_OUTPUT_MARKER = "the turn's own output (the target's answer, as the harness printed it):"
TURN_OUTPUT_LINES_MAX = 200
TURN_OUTPUT_CHARS_MAX = 20000

#: The characters a POSIX shell acts on somewhere in an unquoted word: `;`, `&`, `|` and a newline end a
#: command, `<`, `>`, `(`, `)` redirect or group it, `$`, a backquote and `~` (at a word's start) expand
#: or substitute, `*`, `?`, `[`, `]`, `{`, `}` change the word's meaning, and `#` and `!` are read
#: specially at a word's start. `shellQuote` quotes every one of them, so a rendering that leaves one
#: bare is not a spelling this renderer permits -- and a word a shell acts on is a different command than
#: the word it looks like.
SHELL_SYNTAX_CHARACTERS = ";&|<>()$`~*?[]{}\n#!"

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


def claude_project_dir(cwd: str) -> str:
    """The directory the target files a session under for `cwd`: every character outside [A-Za-z0-9]
    becomes `-`, and nothing else changes.

    Measured on Claude Code 2.1.326, whose own session for `/tmp/probe3/WS.Upper Case_1+plus` was filed
    under `-tmp-probe3-WS-Upper-Case-1-plus`: the separator, the dot, the underscore, the space and the
    plus all become `-`, while case is preserved. `internal/ccc` spells the same rule
    (`ClaudeProjectDir`); it is repeated here because this harness has to say what directory it expects
    the installed session under -- and a check that asked the thing it is checking would answer nothing.
    """
    return re.sub(r"[^A-Za-z0-9]", "-", cwd)


def shell_words(command: str) -> list[str] | None:
    """The words a POSIX shell reads `command` as, or None when it cannot be read as a command line.

    The reported invocation is rendered for a POSIX shell (`internal/ccc/continue.go`'s
    `renderInvocation`/`shellQuote` quote a word exactly where a shell would act on it), so the
    documented reading of it is a shell's own: split on whitespace outside quotes, then remove the
    quoting. A check that looked for the literal substring `CLAUDE_CONFIG_DIR=<home>` would do the
    opposite of that rule: it rejects every home the renderer correctly quotes -- any home with a space,
    any Windows path with a backslash -- and accepts an unquoted home whose space a shell reads as two
    words, which is the defect quoting exists to prevent.
    """
    try:
        return shlex.split(command)
    except ValueError:
        # An unbalanced quote: a shell would not read this as the reported invocation at all.
        return None


def unquoted_text(command: str) -> str | None:
    """What a POSIX shell reads as syntax in `command`: the line with its quoting and escapes removed.

    Quoting is what makes a character inert, so the word reading above -- which throws the quoting away
    -- cannot tell an assignment from a program named like one, or the operator `&&` from two characters
    inside a word. This is that half, read before the quoting is discarded. Inside `'…'` nothing is
    syntax at all, inside `"…"` a shell still acts on `$` and a backquote (both kept here, which is what
    refuses that spelling) and `\\` consumes the character after it, outside quotes `\\` consumes the
    character after it, and everything else that is quoted is dropped. `None` means a shell would not
    stop reading the line as one command: a quote or an escape is never closed.
    """
    kept: list[str] = []
    quote = ""
    escaped = False
    for char in command:
        if escaped:
            escaped = False
        elif quote == "'":
            if char == "'":
                quote = ""
        elif char == "\\":
            escaped = True
        elif quote:
            if char == quote:
                quote = ""
            elif char in "$`":
                kept.append(char)
        elif char in "'\"":
            quote = char
        else:
            kept.append(char)
    if quote or escaped:
        return None
    return "".join(kept)


def spelled_as_invocation(command: str) -> bool:
    """Whether the line's own spelling makes a shell read it as `<cd> <dir> && NAME=<home> <words>`.

    `unquoted_text` keeps the characters a shell reads *without* quoting, so the line's structure is read
    off it: the opening `cd` word, then `&&` as an operator word of its own, then the assignment's own
    `CLAUDE_CONFIG_DIR=` prefix, with no other unquoted character a shell would act on. Three spellings
    that read back as the same words and are not that invocation are refused here: a quoted assignment
    (`… && 'CLAUDE_CONFIG_DIR=…' …` is a *program* by that name to a shell, so nothing sets the variable
    and the harness opens the caller's own data directory); a character a shell acts on left unquoted
    inside the value (`CLAUDE_CONFIG_DIR=a&&b` sets the variable to `a` and runs `b`); and an operator
    glued to a word (`cd /a&&b …` is a directory named `/a&&b` to a shell, not a change followed by an
    assignment) -- which the whitespace test below answers, because a `&&` a shell reads as its operator
    is a word of its own and therefore has whitespace on both sides.
    """
    text = unquoted_text(command)
    if text is None:
        return False
    head, operator, tail = text.partition(OPERATOR_WORD)
    if operator == "" or OPERATOR_WORD in tail:
        return False
    if head.split()[:1] != [CD_WORD] or not head[-1:].isspace() or not tail[:1].isspace():
        return False
    if not tail.lstrip().startswith(f"{CLAUDE_HOME_ENV}="):
        return False
    forbidden = set(SHELL_SYNTAX_CHARACTERS) - {"&"}
    return not (set(head) | set(tail)) & forbidden


def reported_invocation(command: str, cwd: Path, home: Path, arguments: list[str]) -> bool:
    """Whether `command` is the invocation as a POSIX shell reads it: the `cd`, then the assignment, then
    `arguments`.

    The target's loader finds a session through the working directory it is started in, and it takes its
    data directory from CLAUDE_CONFIG_DIR alone -- it has no flag for either -- so a pasted line that
    names neither resumes nothing, or resumes the caller's own session, rather than the one this run
    installed. The words alone do not answer that (`spelled_as_invocation` reads the spelling too).
    """
    return (shell_words(command) == [CD_WORD, str(cwd), OPERATOR_WORD,
                                     f"{CLAUDE_HOME_ENV}={home}", *arguments]
            and spelled_as_invocation(command))


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
        timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=cwd, env=merged, capture_output=True, text=True, timeout=timeout)


def run_shell(line: str, cwd: Path, env: dict[str, str],
              timeout: int = 900) -> subprocess.CompletedProcess:
    """Run one printed command line through a POSIX shell, from the session's working directory.

    The line is the report's own rendering (`cd <dir> && CLAUDE_CONFIG_DIR=<home> claude …`), and this is
    what a caller pasting it does: a shell reads the `cd` and the assignment, so the harness runs in the
    working directory and against the data directory the report named. This is a seam as well -- the
    offline cases drive `main` with it replaced, the way they replace `run`.
    """
    return subprocess.run(line, shell=True, cwd=cwd, env=env, capture_output=True, text=True,
                          timeout=timeout)


def rewrite_source_cwd(fixture: Path, destination: Path, cwd: str) -> str:
    """Write `fixture` to `destination` with its session_meta record's working directory rewritten.

    The target's loader finds a session through the working directory it is run from and files the
    transcript under that directory's own spelling, so the conversion needs a working directory that
    exists on this host: the committed fixture's (`/home/user/code/example-api`) is a path no run here
    could be started from. Only that one field is touched -- every other value of every record is the
    fixture's own -- so the copy answers the same conversation the committed file does. Answers the
    working directory the fixture recorded, or "" when it records none.
    """
    original = ""
    lines: list[str] = []
    for line in fixture.read_text().splitlines():
        if not line.strip():
            continue
        record_ = json.loads(line)
        payload = record_.get("payload")
        if (record_.get("type") == "session_meta" and isinstance(payload, dict)
                and isinstance(payload.get("cwd"), str)):
            original = original or payload["cwd"]
            payload["cwd"] = cwd
        lines.append(json.dumps(record_))
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text("\n".join(lines) + "\n")
    return original


def resume_line(command: str, prompt: str) -> tuple[str | None, str]:
    """The printed exec resume line with the caller's prompt in place of the placeholder word.

    The printed exec form carries the placeholder (`<prompt>`) where a turn's text goes, so the turn this
    check runs cannot be the line unaltered. The line is rebuilt from the printed text itself, replacing
    exactly that word and nothing else -- the renderer quotes it (`'<prompt>'`) -- so every other word of
    the command is the one the report printed and not a spelling this check chose. Answers the line to
    run and, when it cannot be built, why.
    """
    words = shell_words(command)
    if words is None or not words or words[-1] != PROMPT_PLACEHOLDER:
        return None, f"the printed exec resume line does not end with the placeholder word: {command!r}"
    for suffix in (f"'{PROMPT_PLACEHOLDER}'", f'"{PROMPT_PLACEHOLDER}"', PROMPT_PLACEHOLDER):
        if command.endswith(suffix):
            return command[: -len(suffix)] + shlex.quote(prompt), ""
    return None, f"the printed exec resume line does not spell the placeholder word: {command!r}"


def turn_answer(stdout: str, stderr: str) -> str:
    """The harness's own answer text from one non-interactive run.

    `--output-format json` makes the harness print one result document on stdout, and its `result` field
    is the answer a caller would have read. A run whose output is not that document (a harness that
    refused the session, and said why on stderr) is answered with its raw output, so the check reports
    what the harness actually said rather than hiding it.
    """
    for line in reversed((stdout or "").splitlines()):
        stripped = line.strip()
        if not stripped:
            continue
        try:
            document = json.loads(stripped)
        except json.JSONDecodeError:
            break
        if isinstance(document, dict):
            result = document.get("result")
            if isinstance(result, str) and result.strip():
                return result
            break
    return (stdout or "").strip() or (stderr or "").strip()


def bounded_block(text: str, lines_max: int, chars_max: int) -> list[str]:
    """`text` as printable lines, cut to a bound on both axes and marked where it was cut."""
    lines = (text or "").splitlines() or [""]
    kept: list[str] = []
    used = 0
    for line in lines[:lines_max]:
        if used + len(line) > chars_max:
            kept.append(line[: max(chars_max - used, 0)] + " …[the rest of this answer was dropped]")
            used = chars_max
            break
        kept.append(line)
        used += len(line)
    if len(lines) > len(kept):
        kept.append(f"…[{len(lines) - len(kept)} more line(s) of this answer were dropped]")
    return kept


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--claude", default="claude", help="the Claude Code binary to verify against")
    parser.add_argument("--session", default=SOURCE_SESSION,
                        help=f"the Codex fixture session id to convert (default: {SOURCE_SESSION})")
    parser.add_argument("--claude-home", type=Path,
                        help="Claude Code data directory to use (default: a temporary one)")
    parser.add_argument("--resume-prompt",
                        help="run one real turn in the installed session with this prompt")
    parser.add_argument("--expect",
                        help="require the resume turn's answer to contain this text")
    parser.add_argument("--keep", action="store_true",
                        help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true",
                        help="also print a machine-readable summary")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    # A Codex rollout is filed by day (`codex/2026/05/04/rollout-<timestamp>-<id>.jsonl`), not by project
    # directory, so the search is recursive and matches the id the file name ends with.
    candidates = sorted((fixture_root / SOURCE_PROVIDER).rglob(f"*{args.session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {SOURCE_PROVIDER} fixture whose name ends with "
              f"{args.session}.jsonl below {fixture_root / SOURCE_PROVIDER}, found {len(candidates)}",
              file=sys.stderr)
        return 2
    source = candidates[0]
    claude = shutil.which(args.claude)
    if claude is None:
        print(f"{args.claude} is not on PATH: this harness verifies native behaviour, which needs the "
              f"target harness installed", file=sys.stderr)
        return 2
    go = shutil.which("go")
    if go is None:
        print("go is not on PATH: this harness builds ccc with the Go toolchain", file=sys.stderr)
        return 2

    scratch = Path(tempfile.mkdtemp(prefix="ccc-claude-native-check-")).resolve()
    home = args.claude_home.resolve() if args.claude_home else scratch / "claude-home"
    home.mkdir(parents=True, exist_ok=True)
    codex_root = scratch / CODEX_SOURCE_DIR
    cwd = scratch / WORKSPACE
    cwd.mkdir(parents=True, exist_ok=True)
    project_dir = claude_project_dir(str(cwd))
    destination = codex_root / source.relative_to(fixture_root / SOURCE_PROVIDER)
    recorded_cwd = rewrite_source_cwd(source, destination, str(cwd))

    unrelated = home / CLAUDE_PROJECTS_SUBDIR / UNRELATED_PROJECT / f"{UNRELATED_SESSION}.jsonl"
    if unrelated.exists():
        print(f"{unrelated} already exists: this harness plants its own fabricated session under that "
              f"name and will not overwrite one that is there", file=sys.stderr)
        return 2
    unrelated.parent.mkdir(parents=True, exist_ok=True)
    unrelated.write_text(json.dumps({
        "type": "user", "uuid": UNRELATED_SESSION, "sessionId": UNRELATED_SESSION,
        "timestamp": "2026-01-01T00:00:00.000Z", "cwd": "/tmp",
        "message": {"role": "user", "content": "an unrelated session this run must not touch"},
    }) + "\n")

    print(f"repo:        {repo}")
    print(f"claude:      {claude}")
    print(f"claude home: {home}")
    print(f"source:      {source.relative_to(repo)}")
    print(f"codex root:  {codex_root} (a scratch copy; only its session_meta cwd was rewritten)")
    print(f"workspace:   {cwd} -> project dir {project_dir}")
    print(f"scratch:     {scratch}")
    print(f"the target's loader is resumed from a directory that exists and spells to the session's project "
          f"dir: the fixture's recorded cwd ({recorded_cwd or 'none recorded'}) is rewritten mechanically "
          f"to {cwd}, which spells to {project_dir}, so the installed session is filed there and the "
          f"loader can find it from that directory")
    print("the load check's state is read from this conversion run's own report (its default level is "
          "`load`); this harness runs no second conversion")
    print()

    source_before = tree_digest(codex_root)
    planted_before = sha256(unrelated)
    # Every session the home already held — the planted one and the caller's own, when --claude-home
    # names a real home — as the id its file name is, and its bytes: a conversion must not reuse an id
    # and must not touch a file it did not write.
    existing_ids = [path.stem for path in home.rglob("*.jsonl")]
    existing_sessions = {path: sha256(path) for path in home.rglob("*.jsonl") if path != unrelated}
    try:
        build = run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
        if build.returncode != 0:
            record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
            return 2
        record("ccc builds with the flags upstream ships", True)

        env = {"CODEX_SESSIONS_DIR": str(codex_root)}
        convert = run([
            str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{args.session}", "--to", TARGET_PROVIDER,
            "--target-home", str(home), "--target-binary", claude, "--json",
        ], cwd=repo, env=env)
        document: dict = {}
        if convert.returncode != 0:
            # The CLI writes its JSON document before it decides its exit code, so a failed verification
            # still says which state failed; the harness's own stderr is only noise around that.
            try:
                document = json.loads(convert.stdout)
            except json.JSONDecodeError:
                document = {}
            detail = (json.dumps(document.get("verification", {}))[:500] if document
                      else convert.stderr.strip()[:400])
            record("ccc continue converts, installs and verifies", False, detail)
            return 1
        document = json.loads(convert.stdout)

        conversion = document["conversion"]
        installation = document["installation"]
        target = document["target_session"]
        verification = document["verification"]
        thread_id = target["session_id"]
        installed = Path(target["path"])
        carried = conversion["carried"]

        # (b) The conversion: what it says it carried, what it lost, where the session went, and the
        # state of the structural read-back. The counts are printed here because the increment's evidence
        # record cites them; the loss categories are named with their counts for the same reason.
        non_zero = [(loss["category"], loss["count"]) for loss in conversion["losses"]
                    if loss["count"] > 0]
        record("the conversion reports carried evidence and a non-zero loss category",
               conversion["state"] == "converted"
               and conversion["format"] == "claude-code-transcript-jsonl"
               and carried["messages"] > 0 and bool(non_zero),
               f"carried {carried['messages']} message(s), {carried['user_messages']} user message(s), "
               f"{carried['assistant_messages']} assistant message(s), {carried['tool_calls']} tool "
               f"call(s), {carried['tool_results']} tool result(s), "
               f"{carried['compaction_boundaries']} compaction boundary(ies), {carried['bytes']} byte(s); "
               f"non-zero loss categories: "
               + ", ".join(f"{name}={count}" for name, count in non_zero))
        record("the session is installed under the project directory its working directory spells to",
               installation["state"] == "installed" and installed.is_file()
               and installed.parent == home / CLAUDE_PROJECTS_SUBDIR / project_dir
               and installed.name == f"{thread_id}.jsonl",
               f"{installed.relative_to(home)} [{installation['mechanism']}]")
        mode = stat.S_IMODE(installed.stat().st_mode)
        record("the installed session is readable and writable only by its owner",
               mode == 0o600, f"mode {mode:04o}")
        record("the target identity is new and is not the source's",
               thread_id != args.session and thread_id not in existing_ids,
               f"{thread_id}, not one of the {len(existing_ids)} id(s) already in the home")
        record("the installed file is one JSON object per line",
               all(json.loads(line) for line in installed.read_text().splitlines() if line.strip()),
               f"{installed.stat().st_size} bytes (the target's own read of it is the load check)")
        record("the structural read-back of the installed file is verified",
               verification["structure"]["state"] == "verified",
               f"{verification['structure']['state']} — {verification['structure']['method']}")

        # (c) The installed file is read back by this script's own invocation of the inherited reader,
        # not by the conversion's own structure block: `ccc inspect` is a second process reading the same
        # isolated Claude Code root, and the counts it reports are compared with what the conversion says
        # it carried. The reader counts records in its own shape (one record per content block, a
        # tool-result record among its user messages), so the comparison is over the counts that name
        # evidence; the reader's own message counts are printed beside them.
        readback = run([str(repo / BINARY), "inspect", f"{TARGET_PROVIDER}:{thread_id}", "--json"],
                       cwd=repo, env={CLAUDE_HOME_ENV: str(home)})
        try:
            reader = json.loads(readback.stdout).get("source", {}).get("counts", {})
        except json.JSONDecodeError:
            reader = {}
        structure_counts = verification["structure"].get("read_back", {})
        evidence_agreed = (reader.get("tool_calls") == carried["tool_calls"]
                           and reader.get("tool_results") == carried["tool_results"]
                           and reader.get("compact_boundaries") == carried["compaction_boundaries"]
                           and reader.get("messages", 0) >= carried["messages"])
        second_read_agreed = all(
            reader.get(name) == structure_counts.get(name)
            for name in ("messages", "user_messages", "assistant_messages", "tool_calls", "tool_results"))
        second_read_agreed = second_read_agreed and (
            reader.get("compact_boundaries") == structure_counts.get("compaction_boundaries"))
        detail = (f"ccc inspect {TARGET_PROVIDER}:{thread_id}: {json.dumps(reader)}; the conversion's own "
                  f"structure read-back: {json.dumps(structure_counts)}; carried: "
                  f"{carried['tool_calls']} tool call(s), {carried['tool_results']} tool result(s), "
                  f"{carried['compaction_boundaries']} compaction boundary(ies), "
                  f"{carried['messages']} message(s)")
        if readback.returncode != 0:
            detail += f"; inspect rc={readback.returncode}: {readback.stderr.strip()[:200]}"
        record("the inherited reader reads the installed file back as what was written",
               evidence_agreed and second_read_agreed, detail)

        # (d) The reported invocation, checked against the directory and data directory this run used:
        # the loader finds a session through the directory it is started in and takes its data directory
        # from CLAUDE_CONFIG_DIR alone, so a resume command that names neither opens whatever the
        # caller's shell happens to point at. Both rendered lines are read the way a shell reads them --
        # the words they are, and the spelling that decides whether a shell reads the opening `cd` and
        # the assignment as themselves (a quoted assignment reads back as the same words and is a program
        # to a shell) -- so a working directory or home the renderer quotes (a path with a space, a
        # Windows path) is accepted, while an unquoted one a shell would split is not.
        resume_arguments = target.get("resume_arguments") or []
        exec_resume_arguments = target.get("exec_resume_arguments") or []
        resume_environment = target.get("resume_environment") or {}
        resume_command = str(target.get("resume_command") or "")
        exec_resume_command = str(target.get("exec_resume_command") or "")
        want_resume = [claude, "--resume", thread_id]
        want_exec_resume = [claude, "-p", "--output-format", "json", "--resume", thread_id,
                            PROMPT_PLACEHOLDER]
        record("the reported resume command names the binary, the session, the working directory and the "
               "data directory",
               resume_arguments == want_resume and exec_resume_arguments == want_exec_resume
               and resume_environment == {CLAUDE_HOME_ENV: str(home)}
               and reported_invocation(resume_command, cwd, home, want_resume)
               and reported_invocation(exec_resume_command, cwd, home, want_exec_resume),
               f"resume_command={resume_command!r} exec_resume_command={exec_resume_command!r} "
               f"environment={resume_environment}")

        # (e) The engine's own load check: the target harness's loader resolving the installed session,
        # with a control arm that must answer that an unknown session does not exist. Its state and words
        # are taken from the conversion run above (whose default level is `load`), so this check is the
        # engine's report re-read rather than a second conversion.
        load = verification.get("load", {})
        load_detail = str(load.get("detail") or "")
        print()
        print("the target harness's own loader, as the conversion's own report words it:")
        for line in bounded_block(load_detail, TURN_OUTPUT_LINES_MAX, TURN_OUTPUT_CHARS_MAX):
            print("   ", line)
        record("the target harness's own loader resolves the installed session",
               load.get("state") == "verified" and CONTROL_ARM_TEXT in load_detail,
               f"{load.get('state')} — {load_detail[:600]}")

        # (f) One real turn, run from the printed exec resume line with the caller's prompt in the
        # placeholder's place: the line a caller would paste, through a shell, from the session's working
        # directory, with the caller's own environment (this script supplies no credential of its own).
        if args.resume_prompt:
            print()
            session_before = installed.stat().st_size
            line, problem = resume_line(exec_resume_command, args.resume_prompt)
            if line is None:
                record("a real turn continues from the installed session", False, problem)
            else:
                turn = run_shell(line, cwd=cwd, env=os.environ.copy(), timeout=900)
                session_after = installed.stat().st_size
                answer = turn_answer(turn.stdout, turn.stderr)
                expected = args.expect in answer if args.expect else True
                ok = (turn.returncode == 0 and bool(answer.strip())
                      and session_after > session_before and expected)
                detail = (f"exit {turn.returncode}, session {session_before} -> {session_after} bytes; "
                          f"ran: {line}")
                if args.expect:
                    detail += (f"; the answer {'holds' if expected else 'does not hold'} "
                               f"{args.expect!r}, and that comparison decides this check")
                record("a real turn continues from the installed session", ok, detail)
                print()
                print(TURN_OUTPUT_MARKER)
                for line_ in bounded_block(answer, TURN_OUTPUT_LINES_MAX, TURN_OUTPUT_CHARS_MAX):
                    print("   ", line_)
        else:
            record("a real turn continues from the installed session", False,
                   "not attempted: pass --resume-prompt to run one", skipped=True)

        # (g) Every promise about files this run must not change is answered here, after the loader and
        # — when it ran — the real turn: a comparison recorded beside the read-back above would let a
        # mutation during either native stage stand on an earlier verdict, and the planted session is
        # only deleted once this block has run.
        source_after = tree_digest(codex_root)
        moved = sorted([name for name, digest in source_before.items()
                        if source_after.get(name) != digest]
                       + [name for name in source_after if name not in source_before])
        record("the source session is unchanged",
               not moved,
               f"sha256 {sha256(destination)[:16]}… (the scratch source root is identical)" if not moved
               else f"the source root changed: {', '.join(moved)}")
        planted_unchanged = unrelated.is_file() and sha256(unrelated) == planted_before
        record("the unrelated session in the target home is unchanged",
               planted_unchanged,
               f"sha256 {planted_before[:16]}…" if planted_unchanged
               else f"sha256 {planted_before[:16]}… before, "
                    + (f"{sha256(unrelated)[:16]}… after" if unrelated.is_file() else "missing after"))
        # Every session file the home held before the conversion, compared again now: the planted
        # unrelated one is checked above, and this covers a real --claude-home's own sessions too.
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
                              "claude": claude, "claude_home": str(home),
                              "project_dir": project_dir, "cwd": str(cwd)}, indent=2))
        return 1 if failures else 0
    finally:
        # The fabricated unrelated session belongs to this harness and no one else: it is removed even
        # from a home the caller named, so --claude-home is left as it was found apart from the
        # converted session this run installed.
        unrelated.unlink(missing_ok=True)
        if args.keep:
            print(f"kept {scratch} and {home}")
        else:
            shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
