#!/usr/bin/env python3
"""The repository's own Python stays inside the grammar floor the docs name (3.11).

``docs/conventions.md`` binds every assertion under ``scripts/tests/`` to "every Python 3 these docs
name" and forbids depending "on the host *or on a language minor version*"; ``docs/harness.md`` states
why: the gate's ``selftest`` step runs the suite with the interpreter running the gate, so the suite
"must hold on every Python 3 the docs allow and not only on CI's pinned 3.13". The minors the docs
name are 3.11 (the oldest a behaviour difference is named for), 3.12 and 3.13 (the CI pin). This
module is that rule's pin for the Python this repository *ships* -- every ``scripts/**/*.py`` (the
manifest marks ``scripts/`` ``owned``), which is what the gate, the checks and this suite load.

Review round 4 of goal 0008 found the rule unpinned: one case in ``test_opencode_native_check.py``
held a backslash inside an f-string expression, which 3.11's grammar rejects and 3.12's PEP 701
relaxed -- the module could not be imported by a 3.11 discovery, so the required ``selftest`` step
failed before any case ran. No case noticed, because the interpreter running the suite was newer.

Two arms are needed, because no single standard-library call answers the whole question:

* the **grammar arm** parses each source with the running interpreter's own parser constrained to the
  floor -- ``ast.parse(source, feature_version=(3, 11))``. Measured on CPython 3.15.2: it rejects
  PEP 695 (``type X = int``, ``def f[T]()``, ``class C[T]``), 3.13's type-parameter defaults and
  3.14's t-strings and parenthesized-free ``except``, and it *accepts* every f-string form PEP 701
  relaxed, the reported shape among them (``f-strings and feature_version`` in the increment's
  evidence). On a 3.11 host this arm *is* the floor parser, so a 3.12-only f-string is a plain
  ``SyntaxError`` there (measured on CPython 3.11.14: ``f-string expression part cannot include a
  backslash``) and the second arm has nothing left to add -- which is why it needs no skip of its
  own when the running interpreter is the floor.
* the **f-string arm** reads every f-string literal's field structure. 3.12's tokenizer is the first
  to split a literal into ``FSTRING_START``/``FSTRING_END`` and turn replacement-field braces into
  real ``{``/``}`` tokens, so on a host at or above 3.12 this arm reads what 3.11's tokenizer -- which
  keeps a literal whole -- never exposed. Its rules are the four relaxations measured against
  CPython 3.11.14 (each entry of the corpus below was run through that interpreter on 2026-11-19):

  1. a backslash stands in a replacement field's expression part (3.11: "f-string expression part
     cannot include a backslash", inside parentheses, strings or not -- including inside a nested
     literal standing in the field -- while a backslash in the *literal* part, or in a *format spec*,
     is floor-valid and stays reported as no violation),
  2. a comment stands in a replacement field's expression part (3.11: "f-string expression part cannot
     include '#'"; a ``#`` inside a string in the field, or in a format spec, is floor-valid),
  3. the literal's own quote run stands inside the literal (3.11's tokenizer ends the literal
     there, and the truncated literal then fails; the corpus' single-quoted, nested and
     triple-quoted shapes are the measured ones, while one quote of the *other* kind, a two-quote
     run in a triple-quoted literal, and an *escaped* own quote in the literal part are
     floor-valid),
  4. a replacement field nested inside two format specs (3.11: "expressions nested too deeply";
     ``f"{a:{b}}"`` is floor-valid and ``f"{a:{b:{c}}}"`` is not).

  The arm is deliberately the enumerated rules and not a 3.11 parser: a relaxation outside this list
  (PEP 701's implicit parentheses around a field's expression, say) is *not* reported here -- on a
  floor host the grammar arm catches it, because the running parser is the floor, and on a newer host
  it passes this pin. That limit is stated, not hidden: the corpus below is the measured set.

Both arms run on every host and neither skips: on a floor host the grammar arm is exact, above it
both are, and every corpus case asserts what the *combination* reports, so the case's claim does not
depend on which minor runs it. The corpus' expected verdicts were measured, not reasoned: each
``FSTRING_CORPUS`` entry was compiled by CPython 3.11.14 (and by 3.15.2, to show the shapes are
newer-valid), and the increment's evidence records the run.
"""

from __future__ import annotations

import ast
import bisect
import io
import tempfile
import tokenize
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / "scripts"

#: The oldest Python the docs name: the grammar floor every shipped source must stay inside.
FLOOR = (3, 11)

#: 3.12's tokenizer is the first that splits an f-string into these, which is what lets the field arm
#: read a field's expression part at all; absent, the running parser is the floor and the grammar arm
#: already answers the whole question.
FSTRING_START = getattr(tokenize, "FSTRING_START", None)
FSTRING_END = getattr(tokenize, "FSTRING_END", None)

BACKSLASH = "a backslash stands in a replacement field's expression, which the floor rejects"
COMMENT = "a comment stands in a replacement field's expression, which the floor rejects"
OWN_QUOTE = "the literal's own quote stands inside it, where the floor's tokenizer ends the literal"
TWO_SPECS = "a replacement field stands inside two format specs, deeper than the floor allows"


def owned_sources() -> list[Path]:
    """The Python this repository owns and ships: every ``scripts/**/*.py``, no build droppings."""
    return sorted(path for path in SCRIPTS.rglob("*.py") if "__pycache__" not in path.parts)


def read_source(path: Path) -> str:
    """The source text the way the interpreter that imports the file reads it.

    A UTF-8 BOM is stripped by the importer before tokenizing; ``ast.parse`` on the same *string*
    rejects it (``invalid non-printable character U+FEFF``), so reading plain UTF-8 would report a
    violation for a file Python imports without complaint -- the case below pins that this reading
    is deliberate.
    """
    return path.read_text(encoding="utf-8-sig")


def grammar_violations(source: str, filename: str = "<source>") -> list[tuple[int, str]]:
    """``(line, message)`` if ``source`` needs grammar above the floor: the parser's own answer.

    ``feature_version`` asks the running interpreter's parser for the floor's grammar, so on a host
    at the floor this is simply "does this interpreter parse it", and above the floor it is the
    floor's answer for everything except the f-string relaxations the arm above reads (measured).
    """
    try:
        ast.parse(source, filename=filename, feature_version=FLOOR)
    except SyntaxError as exc:
        return [(exc.lineno or 0, exc.msg)]
    return []


def fstring_violations(source: str) -> list[tuple[int, str]]:
    """``(line, reason)`` for every f-string shape the floor rejects and a newer parser accepts.

    The walk is per literal: ``FSTRING_START`` opens a context, the field braces arrive as ``{``/``}``
    operators, a nested literal opens its own context (its fields are *its* regions), and a
    replacement field inside a format spec opens a field whose own format spec is one level deeper.
    """
    if FSTRING_START is None:  # a floor tokenizer: fields are not tokens, so there is nothing to read
        return []
    found: list[tuple[int, str]] = []
    starts = _line_starts(source)
    contexts: list[dict] = []
    for token in tokenize.generate_tokens(io.StringIO(source).readline):
        start = _offset(starts, token.start)
        end = _offset(starts, token.end)
        if token.type == FSTRING_START:
            quote = token.string[-1]
            run = quote * 3 if token.string.endswith(quote * 3) else quote
            contexts.append({"run": run, "body": end, "fields": []})
            continue
        if token.type == FSTRING_END:
            if contexts:
                context = contexts.pop()
                found.extend(_own_quote_violation(source, starts, context, start))
            continue
        if not contexts:
            continue
        fields = contexts[-1]["fields"]
        if token.type == tokenize.OP and token.string == "{":
            if fields and fields[-1]["spec"] is None:
                fields[-1]["depth"] += 1  # a brace of the expression itself: a dict, a set
            else:
                parent = fields[-1]["spec_depth"] if fields else -1
                fields.append({"start": end, "spec": None, "spec_depth": parent + 1, "depth": 0})
        elif token.type == tokenize.OP and token.string == "}":
            if fields and fields[-1]["depth"] > 0:
                fields[-1]["depth"] -= 1
            elif fields:
                found.extend(_field_violations(source, starts, fields.pop(), end))
        elif token.type == tokenize.OP and token.string in "([":
            if fields and fields[-1]["spec"] is None:
                fields[-1]["depth"] += 1
        elif token.type == tokenize.OP and token.string in ")]":
            if fields and fields[-1]["spec"] is None and fields[-1]["depth"] > 0:
                fields[-1]["depth"] -= 1
        elif token.type == tokenize.OP and token.string == ":":
            if fields and fields[-1]["spec"] is None and fields[-1]["depth"] == 0:
                fields[-1]["spec"] = end  # the field's expression ends here; a format spec follows
        elif token.type == tokenize.COMMENT:
            if fields and fields[-1]["spec"] is None:
                found.append((token.start[0], COMMENT))
    return sorted(found)


def floor_violations(source: str, filename: str = "<source>") -> list[tuple[int, str]]:
    """Everything both arms report for one source: the pin's whole verdict."""
    return sorted(grammar_violations(source, filename) + fstring_violations(source))


def _line_starts(source: str) -> list[int]:
    starts = [0]
    for line in source.splitlines(keepends=True):
        starts.append(starts[-1] + len(line))
    return starts


def _offset(starts: list[int], position: tuple[int, int]) -> int:
    row, column = position
    return starts[min(row - 1, len(starts) - 1)] + column


def _line(starts: list[int], offset: int) -> int:
    return bisect.bisect_right(starts, offset)


def _field_violations(source: str, starts: list[int], field: dict, close: int) -> list[tuple[int, str]]:
    """The checks one replacement field answers at its closing brace."""
    found: list[tuple[int, str]] = []
    if field["spec_depth"] >= 2:
        found.append((_line(starts, field["start"]), TWO_SPECS))
    expression_end = field["spec"] if field["spec"] is not None else close
    expression = source[field["start"]:expression_end]
    index = expression.find("\\")
    if index >= 0:
        found.append((_line(starts, field["start"] + index), BACKSLASH))
    return found


def _own_quote_violation(source: str, starts: list[int], context: dict, body_end: int) -> list[tuple[int, str]]:
    """A 3.11 tokenizer ends the literal at the first occurrence of its own quote run in its body."""
    index = context["body"]
    run = context["run"]
    while index < body_end:
        if source[index] == "\\":
            index += 2  # an escape holds the closing quote off in a raw literal too
            continue
        if source.startswith(run, index):
            return [(_line(starts, index), OWN_QUOTE)]
        index += 1
    return []


class ShippedPythonFloor(unittest.TestCase):
    """The pin: the shipped Python parses inside the documented floor, on every host."""

    #: Each shape measured by CPython 3.11.14 and CPython 3.15.2 on 2026-11-19: ``(label, source,
    #: reason)``, where ``reason`` is the field arm's own text for a shape the floor rejects and
    #: ``None`` for a shape the floor accepts. A rejected entry needs the *combination* to report it
    #: (a floor host answers through the running parser, a newer host through the field arm); a
    #: floor-valid entry needs *both* arms silent.
    FSTRING_CORPUS = (
        ("the reviewed defect: a nested call with an escaped fallback in the field",
         'x = f"{d.get(1, f"\\u{1:04x}")}"', BACKSLASH),
        ("the same quote inside a field", 'x = f"{d["k"]}"', OWN_QUOTE),
        ("a same-quote nested literal inside a field", 'x = f"{f"{a}"}"', OWN_QUOTE),
        ("the literal's own triple-quote run inside a field", 'x = f"""{d["""k"""]}"""', OWN_QUOTE),
        ("a comment standing in a field", 'x = f"""{a # c\n}"""', COMMENT),
        ("a field inside two format specs", 'x = f"{a:{b:{c}}}"', TWO_SPECS),
        ("a backslash inside a field after a non-ASCII literal character",
         'x = f"\u00e9{\'\\n\'}{a}"', BACKSLASH),
        ("a backslash in the literal part", 'x = f"A\\nB{a}"', None),
        ("a backslash in a format spec", 'x = f"{a:\\n}"', None),
        ("a hash flag in a format spec", 'x = f"{a:#x}"', None),
        ("a field inside one format spec", 'x = f"{a:{w}}"', None),
        ("other quotes inside a field", 'x = f"{\'a\' + \'b\'}"', None),
        ("a hash inside a string in a field", 'x = f"{ \'#\' }"', None),
        ("the same quote character in a triple-quoted literal",
         "x = f'''{ 'a' }'''", None),
        ("a two-quote run in a triple-quoted literal's field", 'x = f"""{ "" }"""', None),
        ("a nested literal with other quotes inside a field", 'x = f"{f\'{a}\'}"', None),
        ("a slice's colon inside a field", 'x = f"{a[1:2]}"', None),
    )

    #: Grammar above the floor the grammar arm must name, and floor-valid shapes it must accept.
    ABOVE_FLOOR = (
        ("a type alias statement", "type Alias = int\n"),
        ("type parameters on a function", "def f[T](x):\n    return x\n"),
        ("type parameters on a class", "class C[T]:\n    pass\n"),
        ("a 3.13 type-parameter default", "class C[T = int]:\n    pass\n"),
        ("a t-string", 'x = t"hi"\n'),
        ("a parenthesized-free except", "try:\n    pass\nexcept A, B:\n    pass\n"),
    )
    AT_FLOOR = (
        ("an f-string with an escaped quote", 'x = f"a\\"b{a}"\n'),
        ("a walrus and a match", "match x:\n    case 1 if (n := 1):\n        pass\n"),
        ("an except* clause", "try:\n    pass\nexcept* ValueError:\n    pass\n"),
    )

    def test_every_shipped_script_parses_inside_the_documented_floor(self) -> None:
        """Every ``scripts/**/*.py`` source answers the floor the way this pin's arms read it.

        A source that fails here is not merely untidy: the required ``selftest`` step runs discovery
        with the gate's interpreter, so a source above the floor breaks the gate on every host whose
        interpreter is at the floor -- exactly the round-4 defect -- and a file the gate *runs* breaks
        it there too.
        """
        violations = []
        for path in owned_sources():
            source = read_source(path)
            name = path.relative_to(ROOT).as_posix()
            for line, reason in floor_violations(source, name):
                violations.append(f"{name}:{line}: {reason}")
        self.assertEqual(violations, [], "\n".join(violations))

    def test_the_arms_read_the_floor_the_way_the_measured_shapes_do(self) -> None:
        """The corpus: the combination reports every floor-rejected shape and leaves the rest alone.

        On a 3.12+ host the rejected entries are the field arm's own verdict, and that arm's reason
        is asserted by name; on a 3.11 host the running parser answers them (and the field arm cannot
        read fields at all), so what is asserted there is the reported/not-reported verdict, which is
        the rule itself.
        """
        for label, source, reason in self.FSTRING_CORPUS:
            with self.subTest(shape=label):
                reported = floor_violations(source)
                if reason is None:
                    self.assertEqual(reported, [], f"{label}: reported {reported}")
                    continue
                self.assertTrue(reported, f"{label} was not reported")
                if FSTRING_START is not None:
                    reasons = [text for _, text in fstring_violations(source)]
                    self.assertIn(reason, reasons, f"{label}: the field arm reported {reasons}")

    def test_the_grammar_arm_names_grammar_above_the_floor(self) -> None:
        """``feature_version`` is the floor grammar: above-floor syntax is reported, floor syntax is not.

        Measured: the arm reports PEP 695, 3.13's defaults and 3.14's t-strings and parenthesized-free
        ``except`` -- and accepts, on every host, the floor's own shapes. The corpus above carries the
        one class it cannot answer (PEP 701's f-string relaxations).
        """
        for label, source in self.ABOVE_FLOOR:
            with self.subTest(shape=label):
                self.assertTrue(grammar_violations(source), f"{label} was not reported")
        for label, source in self.AT_FLOOR:
            with self.subTest(shape=label):
                self.assertEqual(grammar_violations(source), [], label)

    def test_a_byte_order_mark_is_read_the_way_the_importer_reads_it(self) -> None:
        """A UTF-8 BOM is not grammar, though ``ast.parse`` on the raw string says it is.

        Measured: a file that *starts* with a BOM imports without complaint (CPython strips it
        before tokenizing), while ``ast.parse`` of the same text read as plain UTF-8 raises
        ``SyntaxError: invalid non-printable character U+FEFF`` -- so a pin reading UTF-8 would fail
        the gate on a correct tree. The reader above answers ``utf-8-sig``; this case pins both that
        reading and the failure mode it avoids, so a later "tidy-up" of the encoding is caught here.
        """
        scratch = self.enterContext(tempfile.TemporaryDirectory())
        path = Path(scratch) / "bommed.py"
        path.write_text('x = f"{a}"\n', encoding="utf-8-sig")
        self.assertEqual(floor_violations(read_source(path)), [])
        self.assertTrue(grammar_violations(path.read_text(encoding="utf-8")),
                        "plain UTF-8 reading was expected to report the BOM")

    def test_the_violation_line_is_the_line_the_shape_stands_on(self) -> None:
        """The reported line is read from the source's own offsets, non-ASCII characters included.

        On a 3.12+ host that offset arithmetic is the field arm's own; on a floor host the shape is
        the running parser's ``SyntaxError``, which carries its own line -- either way the line the
        shape stands on, never the first line of the source.
        """
        source = "first = 1\n" + 'second = f"\u00e9{\'\\n\'}"\n'
        self.assertEqual([line for line, _ in floor_violations(source)], [2])
        if FSTRING_START is not None:
            self.assertEqual([line for line, _ in fstring_violations(source)], [2])
