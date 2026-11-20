# 0011 — Produce reusable, explicit conversation summaries

**Status:** Provisional — refine before activation.  
**Prerequisite:** [0009 — Select which recorded context crosses a harness boundary](0009-selective-context.md), then the four follow-up route increments that carry 0009's selection seam to its other routes (the 2026-11-20 fork-A sequencing; see [the index](README.md)). Independent of 0010.

## Intended advance

Users and agents can obtain a **deliberate summary of recorded conversation context** that can be included in a target continuation, inspected separately, or used as an explicitly reduced view of a historical session. This complements selective compaction but does not require a general-purpose agent memory service.

## Material bounds

Summaries must be distinguishable from original turns and historical tool evidence. The system does not fabricate remembered conclusions, replace original history in AgentsView, chase unselected earlier sessions, or silently use an external model. It remains a conversation transform, not a workspace investigator or automatic orchestration engine.

## Provisional acceptance basis

- A representative long session yields a readable, inspectable summary whose relationship to the source messages is clear.
- A supported native target can resume with the requested summary context while the user can still access unaltered source history.
- Significant omissions, unsupported content and model-generated statements are not misrepresented as exact original messages.
- The default continuation path performs no summary generation and has no new mandatory model or network dependency.

## Refinement needed before activation

Determine the useful summary product (brief handoff, evidence-preserving recap, structured decisions, or more than one) from concrete user cases. Decide whether summaries should be standalone artifacts or target-native content and set the permitted model, privacy and cost behavior. Then refine a narrow first outcome and credible review evidence.
