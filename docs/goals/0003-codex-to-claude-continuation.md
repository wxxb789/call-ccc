# 0003 — Continue a Codex session in Claude Code

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](archive/0002-claude-to-codex-continuation.md).

## Intended advance

A Codex-origin conversation becomes a new Claude Code-native session that Claude Code can actually resume. Together with 0002, this establishes **two directions through one coherent continuation representation**, rather than two unrelated hard-coded pairwise converters.

## Bounds

Read Codex through the inherited AgentsView parser/archive semantics. Respect available Codex turn boundaries and tool evidence without treating stored model output as a new Claude system instruction or executing tools. Keep the original source and existing target sessions untouched. Any unsupported source roles, content blocks or message relationships must be reflected in the conversion evidence, not silently presented as native-equivalent.

Do not expand this goal into generic prompt engineering, Git restoration, automatic context compaction or full context re-discovery.

## Done when

- A single non-interactive command selects a Codex session and produces a new Claude Code-native resume identity and usable command. Demonstrate it on a representative session.
- Claude Code's native loader recognizes the installed session and accepts a continuing turn. Provide loader-level evidence.
- Structured source messages and tool events translate through the shared conversation representation, with preserved ordering and explicit disclosure of material loss. Show an input/output semantic comparison, including at least one nontrivial tool interaction.
- The 0002 Claude-to-Codex behavior still passes its accepted verification, and the new route does not duplicate or diverge from AgentsView's source-side parsing implementation.
- The command's JSON and error behavior can be consumed without a skill or interactive prompt; neither original history nor unrelated target sessions are modified.

## Stop and report

If an unverified Claude format makes native installation unsafe, stop with the format evidence and compatibility boundary rather than fabricating a successful session. Stop for missing user-only authorization or irreversible effects; otherwise use reversible defaults and report proof for every Done when item.
