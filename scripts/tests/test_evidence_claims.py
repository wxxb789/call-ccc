#!/usr/bin/env python3
"""The continuation evidence's comparison credits a run word for word, or not at all.

``docs/evidence/0002-claude-to-codex-continuation.md`` §5 maps each piece of source evidence to the
target side -- the harness's own ``thread/read`` view, the file the conversion installed, and the resumed
turn's own answer -- and the answer half is the one claim in that record a reader cannot re-derive by
re-running something: the resumed turn's words differ per run, which is why §3 records the run it quotes.
Round 18's audit found two cells crediting the recorded run's answer with words its recorded output does
not hold: a ``Read`` call's returned contents reported as the answer's "first return", and the source's
tool name ``Bash`` where the answer said ``exec_command``. Both had been carried over from an earlier
run's answer into the cell of a later one, and neither the gate nor any case could see it.

This case pins the rule that keeps that from recurring: an inline-code quotation in a §5 table cell that
credits the run's answer must occur verbatim in a recorded answer block -- the lines the harness prints
under ``the turn's own output (the target's answer, as the harness printed it):``, which are the run's
own bytes rather than a summary of them. What the case cannot check is stated rather than hidden: the
harness records the answer, not its reading of it, so a quotation credited to the wrong return of the
right run is invisible here (the audit's own limit).

The second case drives the two audited cells, as they stood, through the same reading, so the detection
is exercised on the finding itself and not only asserted about the shipped file.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

#: Every continuation evidence record: one file per increment that reports a converted route, each of
#: which may quote a recorded run's answer in its §5 comparison. The rule below is about the records, not
#: about one of them, so a new increment's record is covered by writing it the way the others are
#: (rather than by remembering to add its name here).
DOCUMENTS = sorted((ROOT / "docs" / "evidence").glob("*continuation*.md"))

#: The line the harness prints above a turn's own output (``scripts/codex-native-check.py``, its
#: ``record``/print pair for the resume stage). The block under it is that turn's answer, verbatim.
ANSWER_MARKER = "the turn's own output (the target's answer, as the harness printed it):"

#: The section whose table makes the claims; the rule is about that table's cells.
SECTION_HEADING = "## 5."

#: A cell "credits the answer" when it says so: the word, not an incidental substring.
_ANSWER_WORD = re.compile(r"\banswer\b", re.IGNORECASE)

#: An inline-code quotation: what a cell shows as the run's (or the file's) own spelling.
_QUOTED = re.compile(r"`([^`\n]+)`")

_TABLE_SEPARATOR = re.compile(r"^\|[\s:\-|]+\|$")


def recorded_answer_blocks(document: str) -> list[str]:
    """The answer blocks the document records: the harness's own lines under each marker line."""
    lines = document.splitlines()
    blocks = []
    for index, line in enumerate(lines):
        if ANSWER_MARKER not in line:
            continue
        block = []
        for following in lines[index + 1:]:
            if not following.startswith("    ") or not following.strip():
                break
            block.append(following.strip())
        blocks.append("\n".join(block))
    return blocks


def comparison_table(document: str) -> list[str]:
    """§5's table rows without its header and separator: the cells that make claims."""
    lines = document.splitlines()
    start = next((index for index, line in enumerate(lines)
                  if line.startswith(SECTION_HEADING)), None)
    if start is None:
        return []
    rows = []
    for line in lines[start + 1:]:
        if line.startswith("## "):
            break
        if line.startswith("|"):
            rows.append(line)
    if len(rows) >= 2 and _TABLE_SEPARATOR.match(rows[1].strip()):
        del rows[0]  # the header names the columns; it claims nothing about a source record
        del rows[0]  # ...and the separator under it is punctuation
    return rows


def cells(row: str) -> list[str]:
    """A table row's cells, stripped."""
    return [cell.strip() for cell in row.strip().strip("|").split("|")]


def answer_claims(document: str) -> list[tuple[str, str]]:
    """Every ``(quotation, cell)`` the table's cells credit to the run's answer, in reading order."""
    claims = []
    for row in comparison_table(document):
        for cell in cells(row):
            if not _ANSWER_WORD.search(cell):
                continue
            for quotation in _QUOTED.findall(cell):
                claims.append((quotation, cell))
    return claims


def uncited(document: str) -> list[tuple[str, str]]:
    """The answer claims a recorded answer block does not hold: ``(quotation, cell)`` each."""
    answers = recorded_answer_blocks(document)
    return [(quotation, cell) for quotation, cell in answer_claims(document)
            if not any(quotation in block for block in answers)]


#: The two cells round 18's audit corrected, exactly as they stood: the first credits the answer with a
#: file read's contents as its "first return" (the words of an earlier run), the second names the
#: source's tool name where the recorded answer said ``exec_command`` and says the failure is unquoted.
#: Fed to the same reading below, so the rule is exercised on the finding rather than only on the fix.
AUDITED_ROWS = """\
| `Read` call + its result ("package retry … MaxAttempts = 4") | reached the resumed turn (the answer names `Read` and its first return: "the contents of … `package retry` followed by `const MaxAttempts = 4`") and the file holds the pair | carried (`tool_calls ×3`, `tool_results ×3`) |
| `Bash` call whose result the source marked `is_error` | the file holds the pair, and this run's answer names `Bash` among the tools that appear (its wording does not quote the failure) | carried; the recorded outcome has no target field (`tool_result_outcomes ×1`) |
"""

AUDITED_DOCUMENT = (
    "## 3. Codex loads the installed session and continues from it\n\n"
    "the target harness's own view of the installed session:\n"
    "    turn rollout-2\n\n"
    f"{ANSWER_MARKER}\n"
    "    Tools: `exec_command` (twice, as shell commands).\n"
    "    First return: `Error: no such file or directory` — the `grep -rn maxBackoffSeconds "
    "internal/retry` search found nothing.\n"
    '    Last return: `"maxBackoffSeconds"` — the Python/json check printed the constant\'s name.\n\n'
    "## 5. A representative transcript keeps its supported semantics; the rest is reported\n\n"
    "| Source evidence | Target side | Reported as |\n"
    "| --- | --- | --- |\n"
    f"{AUDITED_ROWS}"
)


class ComparisonCreditsTheRecordedRun(unittest.TestCase):
    """§5's tables may not credit a run's answer with words its recorded output does not hold."""

    def test_every_shipped_comparison_only_quotes_its_recorded_run(self) -> None:
        self.assertTrue(
            DOCUMENTS,
            f"no continuation evidence record under {ROOT / 'docs' / 'evidence'}: this case would "
            f"agree with itself")
        for document_path in DOCUMENTS:
            document = document_path.read_text(encoding="utf-8")
            blocks = recorded_answer_blocks(document)
            self.assertEqual(
                uncited(document), [],
                f"{document_path.name}: a §5 cell credits the run's answer with words no recorded "
                f"answer block holds — a cell may only quote the run it names")
            # Non-vacuity, the same reading's other direction: a record that claims nothing about an
            # answer is not this rule's subject, and one that claims something must have recorded the
            # run it credits. So a record with answer claims needs two recorded blocks (the conversion
            # run and the resumed turn), and one without them is reported rather than passed off as
            # checked.
            claims = answer_claims(document)
            if not claims:
                continue
            self.assertGreaterEqual(
                len(blocks), 2,
                f"{document_path.name} quotes the run's answer in {len(claims)} cell(s) but records "
                f"{len(blocks)} answer block(s) under {ANSWER_MARKER!r}: the quotations have no "
                f"recorded run to cite")

    def test_the_cells_the_audit_corrected_are_reported(self) -> None:
        flagged = {quotation for quotation, _ in uncited(AUDITED_DOCUMENT)}
        self.assertEqual(
            flagged, {"Read", "package retry", "const MaxAttempts = 4", "Bash"},
            "the audited cells' answer claims are not reported as the words of no recorded answer "
            "block: the rule would not have detected the finding it was written for")
        # The row's own file-side quotations are not answer claims, so they are not flagged: the rule
        # reads the cells that credit the answer, not every cell of the row.
        self.assertNotIn("tool_calls ×3", flagged)


if __name__ == "__main__":
    unittest.main()
