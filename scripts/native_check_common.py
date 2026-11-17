"""The mechanisms both native-check harnesses share.

``scripts/claude-native-check.py`` and ``scripts/codex-native-check.py`` verify the two continuation
routes the same way: each digests the trees it must not change, records one named check at a time, runs a
command with this process's environment, and reads a rendered invocation back the way a POSIX shell reads
it. Those mechanisms are one implementation here, so a fix or a stricter rule is not a fix to one route
and a silent drift for the other -- the divergence between the two spellings of the invocation rule is
what disclosed this. Each script imports the names it needs into its own namespace (its module-level
``run`` is the seam its offline cases replace, and it slices its own import of ``RESULTS``), so the run's
exit code is computed from the one list both scripts report into.
"""

from __future__ import annotations

import hashlib
import json
import os
import shlex
import subprocess
from pathlib import Path

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


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
        timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=cwd, env=merged, capture_output=True, text=True, timeout=timeout)
