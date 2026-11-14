# 0004 — Repeat continuation as source history grows

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](0002-claude-to-codex-continuation.md). Can be pursued independently of 0003.

## Intended advance

A user or coding agent can transfer a conversation, let the **source** session gain new history, and explicitly transfer it again without corrupting, silently rewriting or confusing the earlier target. The tool stays a snapshot converter, not a bidirectional sync service.

## Bounds

Only complete recorded source events are eligible for a snapshot. An append that is still incomplete is not a reason to invent content or build a daemon. Repeated conversion does not merge a new source snapshot into a target session that may already have been resumed. No automatic background synchronization, conflict resolution across agents or process coordination.

## Done when

- Two explicit conversions of the same growing source produce distinguishable, correctly attributed results. Demonstrate that the later result includes eligible appended history and the earlier target stays unchanged.
- Reading an actively appended or partially written source does not publish malformed tail events as complete history. Show a controlled incomplete-tail case and the resulting boundary or clear failure.
- The result exposes enough source and target identity/boundary information for a user or agent to choose which continuation to resume, without a separate always-on session index.
- The accepted 0002 continuation route remains natively resumable and the source remains unchanged through the repeated-transfer scenario.

## Stop and report

Do not trade the snapshot model for hidden target mutation or a synchronization daemon. Stop for missing user-only authority or irreversible changes; otherwise choose reversible defaults and pair each accepted outcome with observable evidence.
