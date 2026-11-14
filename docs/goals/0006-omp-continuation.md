# 0006 — Continue conversations into and out of Oh My Pi

**Status:** Ready  
**Prerequisite:** [0005 — Continue conversations into and out of Pi](0005-pi-continuation.md).

## Intended advance

Oh My Pi (OMP) becomes a natively resumable source and target while **sharing genuinely common Pi-family semantics**. A user can transfer a selected OMP conversation to another supported harness, or continue an existing conversation in a new OMP session, without studying OMP's storage internals.

## Bounds

The AgentsView OMP reader and archive path remain untouched. Use the existing Pi-family relationship as a means of avoiding duplication, but do not assume OMP's session paths, title behavior, metadata or resume expectations are identical to Pi's. Preserve available history evidence and report transformations; do not build an OMP orchestrator or session synchronizer.

## Done when

- A nontrivial OMP session can be continued in another already supported harness and a nontrivial supported source can be continued in OMP. Both destinations are confirmed by their **native load/resume mechanisms**.
- Observed OMP-specific history and installation requirements are distinguished from Pi's rather than accidentally accepted only because their JSONL shapes overlap; show at least one meaningful OMP-specific regression case.
- Shared Pi-family conversion logic does not break the accepted Pi routes, and no OMP-specific source parser forks away from AgentsView's inherited implementation.
- Loss and compatibility warnings remain actionable, the source and unrelated target sessions are unchanged, and both directions work through the ordinary CLI/JSON contract.

## Stop and report

Do not claim compatibility from a generated filename alone. If a native-version or format uncertainty prevents a safe install, report it explicitly. Stop for user-only authorization or irreversible effects; otherwise use reversible defaults and pair completion claims with direct evidence.
