#!/usr/bin/env python3
"""A record's gate transcript claims only the elisions the shipped gate can produce.

The gate renders a long step detail as one line, and the only cut it makes is at the *front*: its
``parity`` step calls ``tail(proc.stdout, 800).replace("\\n", " | ")``, and ``tail()`` keeps the detail's
last characters, marking the cut with a leading ``…``. Two claims follow, and both are rules a record's
transcript has to satisfy:

* the body between that mark and the line's end is the kept suffix itself -- so its end is the checker's
  own end (a suffix-preserving elision cannot remove it), and its length is the limit plus the two
  characters each spelled line feed adds;
* the limit a record states for that elision is the gate's own call, read from ``scripts/gate.py``.

Round 4's review found the 0004 record breaking both: §7.1 stated the gate's limit as ``tail()``'s
2 500-character default (which belongs to the gate's other steps), and §13 quoted a ``parity`` line whose
end the record itself had cut -- 740 characters after the mark, where the gate's rendering leaves
800 + 2 x 8 -- while its prose attributed that cut to the gate. The fix put the run's own bytes back and
named the real limit; the case below reads the finding's own two shapes, so the rule is exercised on the
finding and not only on the fix.

An older increment's record is not read here: 0003's record quotes the same 2 500-character default about a
*failed* ``go test`` step, which is that step's own limit, and its transcripts follow the scheme its own
round chose. This rule is checked on the records that state it, and this increment's record must be among
them.

The arithmetic below assumes the parity checker's stdout holds no `` | `` of its own (its source spells the
separator only in type hints and a set union), so every `` | `` a body holds was a spelled line feed; a
checker that ever prints one turns this rule into a visible false positive rather than a silent pass.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

#: Where the records live. A record "states the rule" by naming the gate's elision, and the files that do
#: are the ones whose ``parity`` lines this case reads.
EVIDENCE = ROOT / "docs" / "evidence"
RULE_CLAIM = "the gate's own elision"

#: This increment's record -- the one the round-4 finding was about. Required to be among the records that
#: state the rule, so a rewording or a rename cannot retire this case quietly.
RULE_RECORD = "0004-repeatable-continuation-handoffs.md"

#: The gate, its step that renders the one-line detail, and the checker whose output that detail holds.
GATE = ROOT / "scripts" / "gate.py"
PARITY_STEP = "step_parity"

#: A gate step line as ``gate.py``'s ``main`` prints it: the state, the step's name, then the detail.
GATE_STEP_LINE = re.compile(r"^\s*(?:PASS|FAIL|SKIP|BOUNDARY|N/A)\s+parity\b")

#: The gate's own elision mark: ``tail()``'s cut, then the first spelled line feed.
GATE_MARK = "… | "

#: How an excerpt of a detail the record cut itself declares itself (the 0004 record's §11 scheme).
RECORD_ELISION = "this record's elision"

#: A stated limit -- "keeps the last 800 characters". A sentence in a rule record states the gate's own
#: ``parity`` limit in this shape, so this is what a record's claim is read from.
STATED_LIMIT = re.compile(r"keeps the last ([0-9][0-9 ]*) characters")

#: What the gate's own call looks like inside ``step_parity``: the limit and the separator it spells with.
GATE_LIMIT = re.compile(r"tail\(\s*proc\.stdout\s*,\s*([0-9_]+)\s*\)")
GATE_SEPARATOR_CALL = 'replace("\\n", " | ")'
SEPARATOR = " | "


def rule_records() -> list[Path]:
    """The evidence records that state the rule: the ones naming the gate's elision."""
    return [path for path in sorted(EVIDENCE.glob("*continuation*.md"))
            if RULE_CLAIM in path.read_text(encoding="utf-8")]


def parity_step_source() -> str:
    """``scripts/gate.py``'s ``step_parity``: the code that renders the detail this rule reads."""
    source = GATE.read_text(encoding="utf-8")
    match = re.search(r"^def step_parity\(.*?(?=^def |\Z)", source, re.M | re.S)
    if match is None:
        raise AssertionError(
            f"{GATE.name} has no step_parity() to read the gate's own elision from: the rendering this "
            f"rule is about has moved")
    return match.group(0)


def gate_limit() -> int:
    """The character limit the gate's own ``parity`` detail keeps, read from its call."""
    call = GATE_LIMIT.search(parity_step_source())
    if call is None:
        raise AssertionError(
            f"step_parity no longer calls tail(proc.stdout, <limit>): the limit a record states cannot be "
            f"read from the gate's own call")
    return int(call.group(1).replace("_", ""))


def gate_rendering() -> tuple[int, str]:
    """The gate's own elision as ``step_parity`` spells it: ``(limit, separator)``."""
    if GATE_SEPARATOR_CALL not in parity_step_source():
        raise AssertionError(
            f"step_parity no longer spells the kept line feeds as {SEPARATOR!r}: the length this rule "
            f"expects would count a body's separators wrongly")
    return gate_limit(), SEPARATOR


def parity_lines(document: str) -> list[tuple[int, str]]:
    """The document's ``parity`` step lines, as ``(line number, line)``."""
    return [(number, line) for number, line in enumerate(document.splitlines(), 1)
            if GATE_STEP_LINE.match(line)]


def elision_violations(document: str) -> list[str]:
    """Every way a ``parity`` line breaks the rule, one message each.

    A line carrying the gate's own mark must be the gate's rendering whole: it cannot end in an ellipsis
    (the gate's elision preserves the suffix), and its body's length must be the gate's limit plus the two
    characters each spelled line feed adds. A line with no such mark that is elided anyway must name the
    record as the one that cut it (``RECORD_ELISION``), or a reader cannot tell the gate's rendering from
    an excerpt.
    """
    limit, separator = gate_rendering()
    found = []
    for number, line in parity_lines(document):
        if GATE_MARK in line:
            body = line.split(GATE_MARK, 1)[1]
            if line.rstrip().endswith("…"):
                found.append(
                    f"line {number}: the parity line carries the gate's own elision mark and ends in an "
                    f"ellipsis: tail() keeps the detail's suffix, so this cut is the record's own")
            expected = limit + 2 * body.count(separator)
            if len(body) != expected:
                found.append(
                    f"line {number}: the elided parity body is {len(body)} characters after the mark, where "
                    f"the gate's rendering leaves {expected} ({limit} kept characters plus two for each of "
                    f"the {body.count(separator)} spelled line feeds)")
        elif "…" in line and RECORD_ELISION not in line:
            found.append(
                f"line {number}: the parity line is elided without the gate's mark and without naming the "
                f"record as the one that cut it ({RECORD_ELISION!r})")
    return found


def stated_limit_violations(document: str) -> list[str]:
    """Every stated ``parity``-elision limit that is not the gate's own call, one message each."""
    limit = gate_limit()
    found = []
    for number, line in enumerate(document.splitlines(), 1):
        for stated in STATED_LIMIT.findall(line):
            if int(stated.replace(" ", "")) != limit:
                found.append(
                    f"line {number}: the record states the gate's parity elision as {stated!r} characters, "
                    f"where the gate's own call keeps the last {limit}")
    return found


class GateTranscriptElisionClaims(unittest.TestCase):
    """A record's quoted ``parity`` line is the gate's rendering, and the limit it states is the gate's."""

    def test_every_rule_record_quotes_the_gate_s_own_rendering(self) -> None:
        self.assertTrue(EVIDENCE.is_dir(), f"no {EVIDENCE} to read the records from")
        discovered = {path.name for path in rule_records()}
        self.assertIn(
            RULE_RECORD, discovered,
            f"{RULE_RECORD} is the record this case was written for and states the rule, but it is not "
            f"among the records discovered as stating it (found: {sorted(discovered)})")
        for record in rule_records():
            document = record.read_text(encoding="utf-8")
            found = elision_violations(document) + stated_limit_violations(document)
            self.assertEqual(found, [], f"{record.name}: " + "; ".join(found))

    def test_the_round_s_own_cut_and_stated_default_are_both_reported(self) -> None:
        """The finding's two shapes, driven through the same reading as the shipped record."""
        document = (EVIDENCE / RULE_RECORD).read_text(encoding="utf-8")
        line = next((line for _, line in parity_lines(document) if GATE_MARK in line), None)
        self.assertIsNotNone(
            line, f"{RULE_RECORD} no longer holds a parity line with the gate's own mark: the shape this "
                  f"case mutates is gone")
        # §13's line as the round-4 review found it: cut where the record cut it, at the checker's own
        # `declared stored bytes verified:` line, 740 characters after the mark.
        marker = "declared stored byte"
        cut = line[:line.index(marker) + len(marker)] + "…"
        self.assertTrue(cut.rstrip().endswith("…"))
        messages = elision_violations(cut + "\n")
        self.assertTrue(messages, "the reading does not report a parity line whose end the record cut")
        self.assertTrue(any("ends in an ellipsis" in message for message in messages), messages)
        self.assertTrue(any("characters after the mark" in message for message in messages), messages)
        stated = stated_limit_violations(
            "the `parity` step keeps the last 2 500 characters of the checker's own stdout\n")
        self.assertEqual(
            len(stated), 1,
            f"the reading accepts a record stating the gate's parity limit as tail()'s 2 500-character "
            f"default ({stated!r})")


if __name__ == "__main__":
    unittest.main()
