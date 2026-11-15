# 0001 — Inherit AgentsView and inspect real sessions

**Status:** Accepted: merged onto `main` as `ef6a61c`; archived  
**Prerequisite:** None.

## Intended advance

Establish a standalone call/ccc CLI whose source discovery, native session parsing, normalization and SQLite archive behavior come from the **actual, unmodified** AgentsView implementation, not an independent reimplementation. A user or coding agent can identify and inspect an existing agent session without needing an AgentsView UI or a separate conversation index. This provides the first useful product capability and the trustworthy foundation for every conversion.

## Bounds

Preserve and identify the pinned AgentsView upstream revision and its source-side Git ancestry. The upstream parser, sync, DB schema, migration and their tests remain unchanged relative to that revision. Keep the new continuation core independent from the CLI. A second database schema, reimplemented source parser, private format reader, background daemon or read/write path that bypasses AgentsView's archive ownership rules is outside the goal.

Any behavior that depends on missing native source material must be reported, not invented. Support the native session formats and sources the inherited revision actually supports; do not claim unsupported versions are covered.

## Done when

- A separately buildable call/ccc CLI can inspect a session identified through AgentsView's existing discovery/archive semantics, and reports usable identity, agent, source and conversation information. Demonstrate this with a sanitized, non-empty session fixture.
- A real source-to-archive parse and a subsequent archive read use inherited AgentsView behavior and agree with the pinned upstream implementation on the same fixture; show comparative evidence rather than just claiming parity.
- A source-side diff against the pinned upstream revision shows **no project-owned modifications** to inherited parser/sync/DB/schema code and tests. Any existing upstream differences are identified rather than silently accepted.
- The new continuation functionality is callable without CLI-specific types. The CLI can work as a standalone executable without requiring an AgentsView Web UI or an independent session index.
- The original source session and any existing user archive remain intact; tests or demonstrations use isolated writable data when writing is required.

## Stop and report

If the goal requires user-only credentials or authorization, or an irreversible operation on live history, stop and report the missing decision before taking that action. Otherwise take reversible defaults, and report each Done when outcome with its concrete evidence. A parser rewrite or a new SQLite schema does not count as completing this goal.
