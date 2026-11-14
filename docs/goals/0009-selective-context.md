# 0009 — Select which recorded context crosses a harness boundary

**Status:** Provisional — refine before activation.  
**Prerequisite:** [0003 — Continue a Codex session in Claude Code](0003-codex-to-claude-continuation.md).

## Intended advance

Users and coding agents can intentionally choose **which available parts of a recorded conversation** are carried into the new native session, rather than always transferring the complete selected snapshot. Selection is a transparent, optional transformation and does not affect existing default continuation behavior.

## Material bounds

Selection stays within Session/Conversation history; it does not infer project state, inspect Git, crawl linked sessions or call an LLM. Source and existing destination history remain unchanged. Structural dependencies between selected messages and tool events should not silently become false history. The original order and provenance must remain inspectable.

## Provisional acceptance basis

- A user can express a useful selection of available messages or ranges without editing native transcript files.
- The resulting target is natively resumable and contains **only the intended conversation selection** plus explicitly disclosed structural necessities.
- The conversion explains which source regions were included, excluded or altered; the same unconfigured command continues to preserve the earlier identity behavior.
- Another agent can use the feature without reading a long skill or performing interactive repair.

## Refinement needed before activation

Determine the smallest useful selection model using real user sessions: ordinal ranges, turn-level selections, message IDs, or a combination. Define the intended behavior when excluding one half of a tool-call/result pair. Validate the proposed interface against existing target writer capabilities, then resize to a reviewable outcome. This file is a direction, **not a ready implementation prompt**.
