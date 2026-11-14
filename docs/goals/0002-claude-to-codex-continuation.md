# 0002 — Continue a Claude Code session in Codex

**Status:** Ready  
**Prerequisite:** [0001 — Inherit AgentsView and inspect real sessions](0001-agentsview-parity-and-inspection.md).

## Intended advance

From a chosen Claude Code session, call/ccc produces a **new Codex-native session** that can be resumed in Codex. This is the first complete, useful cross-harness continuation and establishes the behavior expected from future target adapters. One non-interactive CLI invocation should be sufficient for a user or another agent.

## Bounds

The source is read through the inherited AgentsView source/archive path; use original native bytes through the existing source identity when a verified Codex importer makes that preferable. Prefer an official target importer that actually creates a persistent Codex conversation. Never change the source transcript, invent a tool result or overwrite an existing Codex session. Do not manage code files, worktrees or source-agent processes.

The normal conversion preserves available history by default; summarization, selective compaction, redaction and LLM calls are not part of this goal.

## Done when

- A user or coding agent can choose a Claude Code session and target Codex with a single documented, non-interactive command. Show the successful invocation and its native Codex resume identity/command.
- Codex can load the **installed** target session and continue with a new turn. Show the actual native loader/resume evidence, not merely well-formed JSON.
- A representative transcript containing ordinary dialogue and tool-call/result evidence retains supported semantics; unsupported or transformed content is reported explicitly. Show the comparison of source evidence and the target view.
- The source transcript is unchanged, the target identity is new and an unrelated existing Codex session is not replaced. Show repeatable, isolated verification of these properties.
- Machine-readable output clearly distinguishes conversion, installation and native-resume verification; help alone explains the common invocation.

## Stop and report

If required user-only access, an unsupported native Codex version or a potentially destructive operation blocks safe completion, stop and report the evidence rather than faking success. Otherwise choose reversible defaults and finish by pairing every Done when item with observable evidence. The first importer route is not evidence that all other format pairs work.
