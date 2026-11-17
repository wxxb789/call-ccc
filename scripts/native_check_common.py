"""The mechanisms the native-check harnesses share.

``scripts/claude-native-check.py``, ``scripts/codex-native-check.py`` and ``scripts/pi-native-check.py``
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

import hashlib
import json
import os
import shlex
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
