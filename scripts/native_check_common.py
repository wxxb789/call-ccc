"""The mechanisms the native-check harnesses share.

``scripts/claude-native-check.py``, ``scripts/codex-native-check.py``, ``scripts/pi-native-check.py`` and
``scripts/omp-native-check.py``
verify their routes the same way: each digests the trees it must not change, records one named check at a
time, runs a command with this process's environment, and reads a rendered invocation back the way a POSIX
shell reads it. The Claude and Pi harnesses also drive a **copy** of the fixture whose recorded working
directory this module's record-field rewrite replaces, because a Claude Code transcript and a Pi session are
each filed under the working directory's own spelling; ``scripts/codex-native-check.py`` does not, because a
Codex rollout is filed by day and this module's rewrite is not part of that route. Those mechanisms are one
implementation here, so a fix or a stricter rule is not a fix to one route and a silent drift for the
others -- the divergence between the two spellings of the invocation rule is what disclosed this. Each
script imports the names it needs into its own namespace (its module-level ``run`` is the seam its offline
cases replace, and it slices its own import of ``RESULTS``), so the run's exit code is computed from the one
list every script reports into.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shlex
import shutil
import subprocess
from pathlib import Path
from urllib.parse import urlsplit

#: The placeholder the printed exec form carries where a turn's prompt goes. It mirrors the Go constant
#: ``resumePromptPlaceholder`` (``internal/ccc/continue.go``), which renders both routes' exec forms.
PROMPT_PLACEHOLDER = "<prompt>"

#: The option terminator the Claude route's printed exec form carries before that placeholder. It mirrors
#: the Go constant ``promptTerminator`` (``internal/ccc/continue.go``), which is what the route's resume
#: verification runs its own prompt behind: a prompt whose first word is an option (`--help`) is an option
#: to the harness unless the command says where its own flags end.
CONTINUE_TERMINATOR = "--"

#: The marker each harness prints the target's own answer under. It is part of the reporting contract:
#: ``scripts/tests/test_evidence_claims.py`` reads the answer a recorded run credits from this marker, so
#: the answer is printed under it, one answer line per printed line.
TURN_OUTPUT_MARKER = "the turn's own output (the target's answer, as the harness printed it):"

#: The bound on a block a harness reports: the target loader's own words, and -- when a turn runs -- the
#: target's own answer printed under `TURN_OUTPUT_MARKER`. Both are printed line by line and cut to a
#: bound on both axes, so a runaway detail or answer cannot turn into a harness's own memory or make the
#: report it prints unbounded.
REPORT_BLOCK_LINES_MAX = 200
REPORT_BLOCK_CHARS_MAX = 20000

#: The characters a POSIX shell acts on somewhere in an unquoted word: `;`, `&`, `|` and a newline end a
#: command, `<`, `>`, `(`, `)` redirect or group it, `$`, a backquote and `~` (at a word's start) expand
#: or substitute, `*`, `?`, `[`, `]`, `{`, `}` change the word's meaning, and `#` and `!` are read
#: specially at a word's start. `shellQuote` quotes every one of them, so a rendering that leaves one
#: bare is not a spelling this renderer permits -- and a word a shell acts on is a different command than
#: the word it looks like.
SHELL_SYNTAX_CHARACTERS = ";&|<>()$`~*?[]{}\n#!"

#: One check's outcome, as the script reports it.
RESULTS: list[tuple[str, str, str]] = []


def credential_free_url(url: str) -> bool:
    """Whether a base URL is one a seeding check copies: an endpoint with no userinfo, query or fragment.

    A username or a password in a URL is a credential written into the configuration, and a query can
    hold one too (`?api_key=…` is what a provider's `query_params` setting would otherwise carry). A URL
    the standard library cannot read at all is not one this check can vouch for. The Codex checker's
    provider definitions and the Pi checker's are seeded by different code and answer hosts with
    different schemas, but the policy is one rule and lives here: a fix or a stricter reading of a URL
    is not a fix to one route and a silent drift for the others.
    """
    try:
        parts = urlsplit(url)
    except ValueError:
        return False
    return not (parts.username or parts.password or parts.query or parts.fragment)


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


def rewrite_source_cwd(source: Path, destination: Path, cwd) -> str:
    """Copy one session fixture to `destination`, rewriting the working directory its own records carry.

    A target's loader finds a session through the working directory it is run from -- a Claude Code
    transcript and a Pi session each file the session under a project directory spelled from it, and both
    the Claude route's and the Pi route's printed line is resumed from it -- so the conversion needs a
    working directory that exists on this host: a committed fixture's (`/home/user/code/example-api`) is a
    path no run here could be started from.

    The rewrite is through the session's own **JSON record field** and never through its raw bytes: every
    record is parsed, the `cwd` field it carries is rewritten wherever the record spells one -- at the
    record's top level for a Claude transcript or a Pi session, under `session_meta.payload` for a Codex
    rollout -- and the record is written back, so a value a message happens to spell as a byte run is left
    alone rather than rewritten as if it were the working directory. The recorded directory is the first
    `cwd` field any record carries, and every `cwd` field spelling it is rewritten. A line that is not one
    JSON record -- a session a crash left torn, which the target's own reader tolerates -- is carried
    through verbatim, because nothing about it can be read as a field. Answers the working directory the
    fixture recorded, or "" when it records none.
    """
    replacement = str(cwd)
    records: list = []
    for line in source.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        try:
            records.append(json.loads(line))
        except json.JSONDecodeError:
            records.append(line)
    recorded = ""
    for record_ in records:
        found = _first_cwd(record_)
        if found:
            recorded = found
            break
    if recorded:
        for record_ in records:
            _rewrite_cwd(record_, recorded, replacement)
    destination.parent.mkdir(parents=True, exist_ok=True)
    preserved = [record_ if isinstance(record_, str) else json.dumps(record_) for record_ in records]
    destination.write_text("\n".join(preserved) + "\n", encoding="utf-8")
    return recorded


def _first_cwd(value) -> str:
    """The first `cwd` field's string value inside one parsed record, depth first, or ""."""
    if isinstance(value, dict):
        for key, item in value.items():
            if key == "cwd" and isinstance(item, str):
                return item
            found = _first_cwd(item)
            if found:
                return found
    elif isinstance(value, list):
        for item in value:
            found = _first_cwd(item)
            if found:
                return found
    return ""


def _rewrite_cwd(value, recorded: str, replacement: str) -> None:
    """Rewrite every `cwd` field spelling `recorded` to `replacement`, in place, depth first."""
    if isinstance(value, dict):
        for key, item in value.items():
            if key == "cwd" and item == recorded:
                value[key] = replacement
            else:
                _rewrite_cwd(item, recorded, replacement)
    elif isinstance(value, list):
        for item in value:
            _rewrite_cwd(item, recorded, replacement)


def record(name: str, ok: bool, detail: str = "", skipped: bool = False) -> None:
    state = "SKIP" if skipped else ("PASS" if ok else "FAIL")
    RESULTS.append((name, state, detail))
    print(f"{state}: {name}" + (f" — {detail}" if detail else ""))


#: How a valid JSON value that is not an object is named in a check's own reason: both harnesses' records
#: are objects, so a line holding anything else is refused, and a reader of the FAIL line should meet the
#: value's kind rather than Python's own type name (``NoneType`` for ``null``, for instance). The lookup is
#: by exact type, so ``bool`` needs its own entry even though it is an ``int`` subclass.
JSON_VALUE_KINDS = {type(None): "null", bool: "a boolean", int: "a number", float: "a number",
                    str: "a string", list: "an array"}


def jsonl_parse_error(text: str) -> str:
    """The first non-blank line of `text` that is not one JSON object, as a named reason, or "" when every one is.

    Both harnesses answer "the installed file is one JSON object per line" with this reading, and a
    malformed line has to be that check's FAIL with its own reason: ``json.loads`` raises, and an
    exception out of the run would print no FAIL line and no summary, which is the reporting contract
    that check exists to keep. The reason names the line's number so a reader can find it.

    The rule is the *object* rule, and not merely the parse rule: ``json.loads`` also accepts ``null``,
    ``false``, a number, a string and an array, and a line holding one of those is not a record either
    harness writes. A check that asked only whether the line parses would report such a file as one JSON
    object per line -- evidence of an install the target cannot read the way the check claims.
    """
    for number, line in enumerate(text.splitlines(), start=1):
        if not line.strip():
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError as error:
            return f"line {number} is not JSON: {error}"
        if not isinstance(value, dict):
            return f"line {number} is not a JSON object, but {JSON_VALUE_KINDS.get(type(value), 'a value')}"
    return ""


def shell_words(command: str) -> list[str] | None:
    """The words a POSIX shell reads `command` as, or None when it cannot be read as a command line.

    The reported invocation is rendered for a POSIX shell (`internal/ccc/continue.go`'s
    `renderInvocation`/`renderResumeCommand` and `shellQuote` quote a word exactly where a shell would act
    on it), so the documented reading of it is a shell's own: split on whitespace outside quotes, then
    remove the quoting. A check that looked for the literal assignment substring
    (`CLAUDE_CONFIG_DIR=<home>` on the Claude route, `CODEX_HOME=<home>` on the Codex one) would do the
    opposite of that rule: it rejects every home the renderer correctly quotes -- any home with a space,
    any Windows path with a backslash -- and accepts an unquoted home whose space a shell reads as two
    words, which is the defect quoting exists to prevent.

    This answers what the *words* are and nothing else: the quoting is removed here, so what a shell reads
    as syntax is `unquoted_text`'s half of the rule, and each route's `reported_invocation` needs both.
    """
    try:
        return shlex.split(command)
    except ValueError:
        # An unbalanced quote: a shell would not read this as the reported invocation at all.
        return None


def unquoted_text(command: str) -> str | None:
    """What a POSIX shell reads as syntax in `command`: the line with its quoting and escapes removed.

    Quoting is what makes a character inert, so the word reading above -- which throws the quoting away
    -- cannot tell an assignment from a program that is named like one, or the operator `&&` from two
    characters inside a word, or a value from a value a `;` turns into two commands. This is that half,
    read before the quoting is discarded. Inside `'…'` nothing is syntax at all, inside `"…"` a shell
    still acts on `$` and a backquote (both kept here, which is what refuses that spelling) and `\\`
    consumes the character after it, outside quotes `\\` consumes the character after it, and everything
    else that is quoted is dropped. `None` means a shell would not stop reading the line as one command:
    a quote or an escape is never closed.
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


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
        timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=cwd, env=merged, capture_output=True, text=True, timeout=timeout)


# --- the mechanisms the native checks share beyond the reader's fixtures -----------------------------

#: The words a rendered invocation begins with and the operator between them: the directory change, and
#: the `&&` a shell reads as its own operator. They mirror `internal/ccc/continue.go`'s `renderInvocation`.
CD_WORD = "cd"
OPERATOR_WORD = "&&"


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


def bounded(text: str, limit: int) -> str:
    """`text` stripped and cut to `limit`, marked where the tail was dropped."""
    text = (text or "").strip()
    if len(text) <= limit:
        return text
    return text[:limit] + " …[the rest of this output was dropped]"


def print_answer(output: str) -> None:
    """Print one turn's answer under `TURN_OUTPUT_MARKER`, one indented line at a time, bounded on both
    axes -- the reporting half of the answer contract `TURN_OUTPUT_MARKER` names."""
    print(TURN_OUTPUT_MARKER)
    for line in bounded_block(output, REPORT_BLOCK_LINES_MAX, REPORT_BLOCK_CHARS_MAX):
        print("    " + line)
    print()


def spelled_as_invocation(command: str, home_env: str) -> bool:
    """Whether the line's own spelling makes a shell read it as `<cd> <dir> && NAME=<home> <words>`.

    `unquoted_text` keeps the characters a shell reads *without* quoting, so the line's structure is read
    off it: the opening `cd` word, then `&&` as an operator word of its own, then the assignment's own
    `<home_env>=` prefix, with no other unquoted character a shell would act on. Three spellings are
    refused, and each is a different command than the line looks like: a quoted assignment
    (`… && 'NAME=…' …` is a *program* by that name to a shell, so nothing sets the variable and the
    harness opens the caller's own data directory); a character a shell acts on left unquoted inside the
    value (`NAME=a;b` sets the variable to `a` and runs `b`); and an operator glued to a word
    (`cd /a&&b …` is a directory named `/a&&b` to a shell, not a change followed by an assignment) --
    which the whitespace test below answers, because a `&&` a shell reads as its operator is a word of
    its own and therefore has whitespace on both sides. Every sibling harness shares this one
    implementation, so a fix to the rule is not a fix to one route and a silent drift for the others.
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


def reported_invocation(command: str, workspace, home, home_env: str, arguments: list[str],
                        session_dir_env: str | None = None, session_dir: str = "") -> bool:
    """Whether `command` is the invocation as a POSIX shell reads it: the `cd`, the environment
    assignments, then `arguments`.

    The whole word vector is compared, exactly as every sibling harness's `reported_invocation` does: any
    `cd <anything>` beside any assignments beside any session id reads back as the right *shape* and is
    still another invocation, so a substring or a shape test would accept a line that resumes somewhere
    else. The spelling is read too (`spelled_as_invocation`), which is why both halves decide this rule.
    A route whose printed line names the sessions directory directly passes that directory in
    `session_dir`, with `session_dir_env` naming its variable: it is compared against the directory the
    check itself looked the session up in, not against a spelling computed twice. A route whose sessions
    tree is its own spelling computes that tree in its own wrapper and passes the result here.
    """
    words = [f"{home_env}={home}"]
    if session_dir_env:
        words.append(f"{session_dir_env}={session_dir}")
    return (shell_words(command) == [CD_WORD, str(workspace), OPERATOR_WORD, *words, *arguments]
            and spelled_as_invocation(command, home_env))


def session_records(text: str) -> list[dict]:
    """Every line of one session file that is one JSON object, in the file's own order."""
    records: list[dict] = []
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            entry = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if isinstance(entry, dict):
            records.append(entry)
    return records


def session_id_from_header(session: Path) -> str:
    """The session id one session file's own header line records, or "" when it holds none.

    The identity is read from the `session` record's own `id` field (through `session_records`, so a line
    that is not one JSON object is skipped), and never inferred from the file's name: the name's stem
    carries a timestamp as well, so two files recording the *same* session id under different timestamps
    read back as two different names.
    """
    for entry in session_records(session.read_text(encoding="utf-8")):
        if entry.get("type") == "session":
            return str(entry.get("id", ""))
    return ""


def home_holds_id(home: Path, session_id: str, exclude: Path | None = None) -> bool:
    """Whether any session file below `home`, other than `exclude`, records `session_id` in its header.

    The ids are read from each file's own header (`session_id_from_header`) rather than from its name, so
    a conversion that reused an id the home already held under another timestamp is caught instead of
    passing on a differing file name. `exclude` is the file a run just installed, whose header holds the
    same id by construction.
    """
    if not session_id:
        return False
    for path in home.rglob("*.jsonl"):
        if exclude is not None and path == exclude:
            continue
        if session_id_from_header(path) == session_id:
            return True
    return False


def installed_counts(session: Path) -> dict[str, int]:
    """What one installed session file holds, from its own bytes: entries by record type, and the roles
    of its message entries. A `message` entry whose own `message` value is not an object counts under the
    empty role, the same reading the loader makes of a record it cannot index."""
    counts: dict[str, int] = {}
    roles: dict[str, int] = {}
    for entry in session_records(session.read_text(encoding="utf-8")):
        kind = entry.get("type", "")
        counts[kind] = counts.get(kind, 0) + 1
        if kind == "message":
            message = entry.get("message")
            role = message.get("role", "") if isinstance(message, dict) else ""
            roles[role] = roles.get(role, 0) + 1
    counts.update({f"role:{role}": value for role, value in roles.items()})
    return counts


def session_tree(home, subdir: str, project_dir: str) -> str:
    """The directory one session is filed in and looked up through, below a data directory: the project
    directory under the sessions root `subdir`.

    It mirrors `filepath.Join` on the Go side: the separator is spelled the way the paths are -- a
    Windows-shaped data directory (backslashes, no forward slashes) joins with ``\\`` and every other
    spelling joins with ``/``. The sessions root and the project directory are each route's own spelling,
    so a route wraps this with its own subdir constant and its own project-directory rule.
    """
    home_text = str(home)
    separator = "\\" if "\\" in home_text and "/" not in home_text else "/"
    return separator.join([home_text.rstrip("/\\"), subdir, project_dir])


def report(args: argparse.Namespace, scratch: Path, document: dict, extra: dict | None = None) -> None:
    """The end of a run's report, shared by the native checks: say what became of the scratch tree and,
    when it was asked for, print the machine-readable summary.

    `extra` carries any additional keys a route's payload holds; it is inserted after `target_session`
    and before `conversion`, so each route's own key order is preserved -- a route with none passes
    nothing.
    """
    if args.keep:
        print(f"scratch kept at {scratch}")
    else:
        shutil.rmtree(scratch, ignore_errors=True)
        print(f"scratch removed ({scratch})")
    if args.json:
        target = document.get("target_session") or {}
        payload = {
            "direction": args.direction,
            "checks": [{"name": name, "state": state, "detail": detail} for name, state, detail in RESULTS],
            "target_session": target.get("session_id", ""),
        }
        payload.update(extra or {})
        payload["conversion"] = (document.get("conversion") or {}).get("state", "")
        print(json.dumps(payload, indent=2))
