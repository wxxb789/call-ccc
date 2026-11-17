# 0016 — Continue VS Code Copilot conversations in another harness

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](archive/0002-claude-to-codex-continuation.md). Independent of 0003–0015 and 0017.

## Intended advance

A user or coding agent can select an existing **VS Code Copilot** chat session and continue its recorded conversation in a new Codex-native session. This makes the IDE's existing conversations portable without assuming that VS Code exposes a stable way for call/ccc to install foreign chat sessions back into the editor.

## Bounds

Read exclusively through AgentsView's existing `vscode-copilot` provider, which understands VS Code's `chatSessions/<uuid>.json` snapshots and JSONL operation logs. Distinguish this source from the separate GitHub Copilot CLI and Visual Studio Copilot formats. The destination is an **already supported, natively resumable** harness; **VS Code Copilot as a native target is deliberately out of scope** until such a path is verified.

The source may contain changing response arrays, repeated tool IDs, hidden invocations, and mixed text/tool references. Preserve available order and evidence, or report loss; do not blindly promote intermediate duplicate operations to authoritative finished turns. No modifications to VS Code storage, extension data, editor state, Git or workspace files.

## Done when

- A real or sanitized VS Code Copilot session with more than a single user turn can be selected via inherited AgentsView discovery/archive semantics and continued in a **new Codex-native session**, demonstrated by Codex's actual load/resume and one further turn.
- The transferred history reflects the available recorded messages and any supported completed tool evidence. A representative fixture or native sample covers a growing response, repeated tool identity or hidden invocation; its treatment and any losses are demonstrable.
- The original VS Code chat remains intact. The output accurately labels its source as VS Code Copilot rather than Copilot CLI, and the user/agent sees an unambiguous source ID, target ID, native resume command and fidelity warnings without requiring a skill.
- Existing continuation routes and the inherited AgentsView parser/schema remain unchanged. No purported VS Code native importer is added without evidence of editor-side load/resume compatibility.

## Stop and report

Stop if only user-provided access or an irreversible operation could enable validation. If the specific source lacks useful recorded conversation text, report unavailability rather than fabricating turns or claiming the target can resume the missing context. Otherwise use reversible defaults and pair every completion condition with concrete evidence.
