# 0005 — Continue conversations into and out of Pi

**Status:** Implemented on `feat/0005-pi-continuation`; awaiting operator acceptance  
**Prerequisite:** [0003 — Continue a Codex session in Claude Code](archive/0003-codex-to-claude-continuation.md).

## Intended advance

Pi becomes a usable participant in call/ccc's cross-harness continuation: a conversation from a supported existing harness can be resumed as a **new native Pi session**, and a Pi session can be continued in a different supported harness. This establishes a second target format family without copying or weakening the previous routes.

## Bounds

Reuse AgentsView's existing Pi discovery, parser and SQLite projection unchanged. Account for Pi's native session identity, message sequence and available branch/parent semantics at the selected conversation boundary; do not treat unrelated inactive branches as additional user turns. The project remains a one-shot converter, not a Pi session manager or agent runtime.

Do not silently translate historical tool results into new tool executions. Do not automatically follow external sessions, reconstruct context dropped by an earlier compact, or edit workspace files.

## Done when

- A representative Claude-origin conversation can be converted to a new Pi-native session and **resumed by Pi**. Show the actual native load/resume result, not only a parser round-trip.
- A representative Pi-origin conversation can continue in a previously supported target harness. Show native load/resume evidence for that direction, demonstrating the common intermediate model rather than a special Pi-to-one-agent shortcut.
- Tests or native-shaped fixtures show ordered dialogue and available tool-call/result relationships remain coherent, and that a branched or compacted Pi history is not accidentally flattened into an invented active conversation. Any material loss is explicit.
- The source and existing target sessions remain intact. The accepted Claude/Codex behaviors are not regressed, and inherited AgentsView source-side code remains unchanged.
- The CLI exposes both directions with the same help, JSON result and error conventions as earlier goals.

## Stop and report

If native Pi behavior cannot be validated at an available version, report the precise limitation rather than claiming broad compatibility. Stop for irreversible operations or missing user-only authority; otherwise use reversible defaults and report evidence against each Done when item.
