# 0012 — Minimize unnecessary sensitive details in transferable context

**Status:** Provisional — refine before activation.  
**Prerequisite:** [0009 — Select which recorded context crosses a harness boundary](0009-selective-context.md). Independent of 0010 and 0011.

## Intended advance

A user can choose **narrow, inspectable edits** to sensitive or incidental content in the exported conversation, reducing unnecessary exposure and avoiding avoidable false positives in downstream services while retaining useful task history. The tool should minimize change, not turn redaction into indiscriminate censorship.

## Material bounds

Redaction is opt-in, never a covert automatic pass of the default converter. Do not promise to bypass service safety controls, falsify tool results, remove evidence of a consequential change, or guarantee that provider moderation will accept modified text. Original source history remains untouched. Reports must be useful without themselves leaking the redacted material.

## Provisional acceptance basis

- On representative false-positive or privacy examples, a user can request minimal edits and inspect which regions changed without disclosing the original sensitive bytes in normal logs.
- The resulting continuation is natively resumable; unaffected history retains its original meaning and important changes are clearly attributed as transformations.
- A reviewer can compare the modified target against the chosen original context and judge whether the intervention was minimal and useful.
- The unconfigured conversion continues to preserve the available content rather than silently applying sanitization.

## Refinement needed before activation

Identify the first supported redaction type, the user's control/approval model, and how minimality and semantic preservation will be judged. Select legitimate test cases and avoid target-provider pass rates as the sole success criterion. Refine the goal from real evidence before starting.
