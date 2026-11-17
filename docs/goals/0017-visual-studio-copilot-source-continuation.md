# 0017 — Continue available Visual Studio Copilot conversation history elsewhere

**Status:** Ready  
**Prerequisite:** [0002 — Continue a Claude Code session in Codex](archive/0002-claude-to-codex-continuation.md). Independent of 0003–0016.

## Intended advance

When **Visual Studio Copilot** traces actually contain chat prompts and responses, a user or coding agent can select the corresponding recorded conversation and continue the available history in a new Codex-native session. When traces contain only telemetry or incomplete history, call/ccc should instead provide a precise, trustworthy explanation of what can and cannot be continued.

## Bounds

AgentsView's `visualstudio-copilot` source currently consumes OpenTelemetry JSONL spans from Visual Studio's Copilot integration, distinct from VS Code's chat snapshots and from GitHub Copilot CLI. The spans may include `gen_ai.input.messages` and `gen_ai.output.messages`, but are **not evidence of a complete native Visual Studio chat archive**. Use the inherited AgentsView source parser and archive untouched. Preserve only actually observed text/tool evidence; do not synthesize missing turns from usage metrics, token counts or inferred intent.

This is deliberately a **source-only** portability goal. A Visual Studio-native session writer, IDE integration, credential transfer or reconstruction of unrecorded conversation state is outside the scope. Do not make enabling telemetry globally or altering existing Visual Studio settings an implicit prerequisite of conversion.

## Done when

- Using a representative trace with recorded conversation content, a source conversation identified by AgentsView can be continued as a **new Codex-native session**, demonstrated by Codex's actual load/resume with a fresh turn.
- Transferred prompts, responses and tool evidence are traceable to observed span content, and any gaps caused by missing spans, partial recordings or absent tool results are reported. A contrasting usage-only or otherwise non-conversational trace yields a clear **no transferable conversation** result instead of invented history.
- VS Code Copilot, GitHub Copilot CLI and Visual Studio Copilot remain distinct source identities and capability claims. No existing IDE data or user archives are overwritten, and no unsupported Visual Studio-native resume claim appears in the output.
- The CLI remains non-interactive and machine-readable; earlier accepted routes and inherited AgentsView source-side code remain unchanged. Present concrete proof for both the transferable and insufficient-source cases.

## Stop and report

If only user-supplied authorization or an irreversible action could establish the needed trace evidence, stop and report it. If no legitimate trace with actual conversation bodies is available for validation, report the evidence gap rather than treating telemetry-only conversion as completion. Otherwise use reversible defaults and pair each Done when criterion with direct evidence.
