# 0007 — Continue supported conversations in OpenCode

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](archive/0002-claude-to-codex-continuation.md). Can be pursued independently of 0003–0006.

## Intended advance

OpenCode becomes a reliable **target harness**: a selected conversation from an already supported source is imported as a **new OpenCode-native session** and can be resumed from OpenCode. This proves the continuation engine can handle a native destination backed by a database, not only a JSONL file.

## Bounds

Do not write ad hoc SQL into a live OpenCode application database when an appropriately supported official import or session interface exists. Treat target format versions and importer capabilities as observable compatibility boundaries. Reuse the same conversation representation and CLI contract as earlier routes; do not create a second OpenCode source parser or a replacement agent data store.

## Done when

- A representative Claude-origin conversation becomes a newly identifiable OpenCode session, and a native OpenCode load/export/resume path verifies that the session is usable.
- Import succeeds without clobbering another session or modifying the source. A failed or partially completed import produces a truthful, recoverable result and cannot silently duplicate or overwrite unrelated content.
- The target preserves the supported dialogue and tool evidence expected by OpenCode's verified import format; meaningful omissions or rewrites are disclosed with source-related evidence.
- The existing Claude-to-Codex route and source-side AgentsView parity remain intact. The ordinary non-interactive command and JSON output describe OpenCode target support and its version limitations.

## Stop and report

If the installed OpenCode version lacks a verified safe import path, report that boundary rather than modifying its private database speculatively. Stop for irreversible operations or missing user-only authority; otherwise choose reversible defaults and provide evidence for each Done when item.
