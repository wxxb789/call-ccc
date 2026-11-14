# call/ccc design and goals

**call/ccc** means *call-with-current-conversation-continuation*.

- [Core Design](core-design/README.md) defines the long-lived product and engineering boundaries.
- [Numbered Goals](goals/README.md) contain inspectable outcomes for goal-driven development.
- [Upstream](upstream.md) records the pinned AgentsView revision, every declared difference from it,
  and how to move the pin.
- [Conventions](conventions.md) and [Harness](harness.md) are binding for every increment: layout,
  stack, gate, knobs, and the rules the harness's own tests enforce.
- [Decisions](decisions/OPEN-QUESTIONS.md) is the append-only record of decisions taken and defaults
  chosen where an owner's preference was missing.
- [Plans](plans/) holds one plan per increment, written before its implementation;
  [Evidence](evidence/) holds one record per increment with the commands that ran and their real
  output.

The Core Design is deliberately stable. Goals may be refined after evidence from earlier work;
implementing a goal must not silently weaken the Core Design.

This repository keeps the AgentsView reader, parser and SQLite archive **source-identical** to the
pinned upstream revision — verified on every gate run by `scripts/upstream-parity.py` — and adds a
separately reusable continuation engine plus a thin CLI beside it. Upstream's own guides are carried
with its code (`docs/agents/*`) and govern changes to carried files; this repository's rules govern
everything else.
