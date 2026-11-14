# 0008 — Continue OpenCode history in other harnesses

**Status:** Ready  
**Prerequisites:** [0007 — Continue supported conversations in OpenCode](0007-opencode-target-continuation.md) and [0003 — Continue a Codex session in Claude Code](0003-codex-to-claude-continuation.md).

## Intended advance

OpenCode-origin conversations can now be resumed in another native harness, completing OpenCode's two-way participation without inventing a new SQLite reader. Together with prior goals, the system demonstrates modular composition among JSONL and database-backed agent session formats.

## Bounds

Read OpenCode sessions only through AgentsView's existing source discovery/parser/archive behavior or an authoritative native export found through that upstream source identity. Do not bypass AgentsView with a bespoke OpenCode SQL parser. Context transformations are identity by default; do not rewrite user content merely to make a format appear to match.

## Done when

- An OpenCode-origin session with substantive messages can be chosen and continued as a new native Claude Code or Codex session. Provide an actual target native load/resume result.
- Available tool call/result and multi-part content semantics survive where supported; unsupported content has a specific reported loss instead of silent omission. Show semantic comparison evidence for at least one tool-bearing fixture.
- The accepted OpenCode-target route and prior Claude/Codex routes remain correct. Source-side AgentsView packages and SQLite schema are unchanged.
- A user or agent obtains the same predictable, non-interactive success/error JSON and native resume identity as for other source harnesses, without a new catalog or background service.

## Stop and report

If a source archive lacks content necessary for a claimed fidelity level, report the gap and do not invent a recovered original. Stop on missing user-only authority or irreversible changes; otherwise use reversible defaults and end with evidence paired to every completion condition.
