# call/ccc design and goals

**call/ccc** means *call-with-current-conversation-continuation*.

- [Core Design](core-design/README.md) defines the long-lived product and engineering boundaries.
- [Numbered Goals](goals/README.md) contain inspectable outcomes for goal-driven development.

The Core Design is deliberately stable. Goals may be refined after evidence from earlier work; implementing a goal must not silently weaken the Core Design.

The standalone repository is intended to keep the AgentsView reader, parser and SQLite archive **source-identical** to the pinned upstream revision. The novel code is a separately reusable continuation engine and a thin CLI. Neither this documentation PR nor the goal files implement that engine.
