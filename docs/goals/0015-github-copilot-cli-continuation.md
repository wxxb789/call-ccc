# 0015 — Continue conversations into and out of GitHub Copilot CLI

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](0002-claude-to-codex-continuation.md). Independent of 0003–0014 and 0016–0017.

## Intended advance

GitHub Copilot CLI becomes a natively resumable source and target in call/ccc. A coding agent can transfer a Copilot CLI session into Codex, or convert a supported source conversation into a **new Copilot CLI session**, using the same simple non-interactive interaction as other targets.

## Bounds

Use the existing AgentsView `copilot` provider for Copilot CLI's session JSONL, including `~/.copilot/session-state/` and supported `events.jsonl` layouts. Copilot CLI is a **different product and source format** from VS Code Copilot and Visual Studio Copilot; do not merge their capabilities or assume they share a target writer.

GitHub documents a session-state layout and a derived session index/store but does not publish a complete producer-side event serializer in the cited AgentsView source inventory. Therefore, incoming Copilot CLI support requires observed, version-scoped native import/load behavior. Prefer official supported restore/import routes where available; do not speculatively mutate Copilot's live derived SQLite store. Historical event metadata and tool evidence must not be fabricated to make a session appear legitimate.

Do not expand this goal into IDE Copilot integrations, model authentication management or a background session-sync daemon.

## Done when

- A real or sanitized native-shaped Copilot CLI conversation is read using unchanged AgentsView logic and continued in **Codex**, including an actual native target load/resume and explicit reporting of any missing tool or content semantics.
- A supported Claude/Codex-origin conversation is installed as a **new Copilot CLI-native session** and can be loaded and continued with a fresh turn by a tested Copilot CLI runtime. Show the exact verified runtime/version and native resume evidence, not only valid-looking JSONL.
- The converted history preserves supported turn order and tool-call/result relationships without mislabeling other Copilot products' histories; unsupported events are disclosed. Demonstrate at least one nontrivial tool-bearing case.
- A caller using only CLI help and structured output can select Copilot CLI, see the resulting native identity and resume instructions, and distinguish converted/installed/resumable status.
- The source and unrelated target sessions remain unchanged, earlier accepted continuation routes still work, and no source-side AgentsView or archive schema modifications are introduced.

## Stop and report

If no supported and verifiable native Copilot CLI import/install route exists for the available version, report the exact blocker; a source-only export is **not** evidence this two-way goal is complete. Stop for missing user-only access or irreversible operations, otherwise choose reversible defaults and report each criterion with its evidence.
