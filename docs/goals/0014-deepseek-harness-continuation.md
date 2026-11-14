# 0014 — Continue conversations into and out of DeepSeek Harness

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](0002-claude-to-codex-continuation.md). Independent of 0003–0013 and 0015–0017.

## Intended advance

DeepSeek Harness (DSH) becomes a natively resumable **source and target** for call/ccc. Users and coding agents can carry an existing DSH conversation into Codex and continue an existing supported conversation in a new DSH session, without any DSH-specific instructions beyond the normal call/ccc interface.

## Bounds

AgentsView already reads DSH's default JSONL source, including plain and zstd-compressed released generations v0–v4. Continue to use its actual inherited provider and SQLite archive path unchanged. **This does not imply support for DSH's optional SQLite persistence backend**, which AgentsView currently does not read.

A DSH target must be a genuine, verifiable native session for a clearly identified supported DSH runtime/version, not an arbitrary file shaped like role/content JSONL. Native event identity, sequence, header and tool evidence matter to the extent the target actually requires them. Do not synthesize execution, rewrite past tool results, or manage workspace/Git/process state. An upstream source variant lacking transferable content must produce an explicit loss or unavailable result.

Do not make support for all historical DSH write generations a condition of producing one valid current-format target session. No changes to upstream-owned source parsers or archive schema are authorized by this goal.

## Done when

- A representative DSH-origin session containing user/assistant dialogue and tool evidence is read through the unchanged AgentsView integration and continued as a **new Codex-native session**. Show Codex's actual load/resume evidence and an input/output semantic comparison.
- A representative Claude Code- or Codex-origin session is converted to a **new DSH-native session**, using a documented tested DSH format/runtime. Show DSH itself loading and continuing the session with a fresh turn; parsing the artifact with a different tool is not enough.
- Source identity, selected history boundary, conversation order, supported tool relationships and meaningful losses are observable in both directions. At least one DSH version-specific native fixture or test demonstrates that writer assumptions do not conflate generation families.
- Both conversions are non-interactive and expose the ordinary resume identity, verification level and loss data through the established CLI/JSON contract. The original source, existing destination sessions and inherited AgentsView source-side code remain unchanged.
- Earlier accepted Claude/Codex continuation behavior still works; DSH introduces no DSH-to-every-agent special-pair converter requirement.

## Stop and report

Stop immediately if only the user can supply missing authorization or a required irreversible decision. If native DSH resume or a safe tested target format cannot be established, report the exact incompatibility and **do not claim two-way support**. Otherwise choose reversible defaults, keep scope to one supported target generation, and pair every completion condition with observable evidence.
