# call/ccc — numbered outcome goals

This series follows the **define-goal** convention: a goal states a user-visible or independently verifiable outcome, not an implementation checklist. All goals inherit [the Core Design](../core-design/README.md). The sequence is an evolving set of commitments, not a scheduler, build plan or promise that every distant idea will ship.

The normal feedback window is approximately 12 hours of agent work. If a goal proves larger, preserve its intended acceptance and split it into smaller useful outcomes rather than claiming an incomplete feature is done.

| File | Outcome | Prerequisite | Status |
| --- | --- | --- | --- |
| [0001](0001-agentsview-parity-and-inspection.md) | Unmodified upstream source/archive, usable session inspection | None | Ready |
| [0002](0002-claude-to-codex-continuation.md) | Claude Code history continues in Codex | 0001 | Ready |
| [0003](0003-codex-to-claude-continuation.md) | Codex history continues in Claude Code | 0002 | Ready |
| [0004](0004-repeatable-evolving-session-handoffs.md) | Repeat conversion as a source session grows | 0002 | Ready; independent of 0003 |
| [0005](0005-pi-continuation.md) | Native two-way Pi continuation | 0003 | Ready |
| [0006](0006-omp-continuation.md) | Native two-way Oh My Pi continuation | 0005 | Ready |
| [0007](0007-opencode-target-continuation.md) | Continue existing conversations in OpenCode | 0002 | Ready; may run separately |
| [0008](0008-opencode-source-continuation.md) | Continue OpenCode history in other agents | 0007 and 0003 | Ready |
| [0009](0009-selective-context.md) | Select which source history travels | 0003 | Provisional |
| [0010](0010-selective-compaction.md) | Compress selected regions while protecting others | 0009 | Provisional |
| [0011](0011-context-summaries.md) | Produce explicit reusable summaries | 0009 | Provisional; independent of 0010 |
| [0012](0012-minimal-content-redaction.md) | Minimal opt-in redaction with visible changes | 0009 | Provisional; independent of 0010/0011 |
| [0013](0013-persisted-reasoning-traces.md) | Safe editing of exposed, persisted reasoning traces | 0009 | Provisional; independent of 0010–0012 |

**Relationship rules:** 0004 and 0007 extend a previously verified route without replacing it. 0005/0006 share a format family but may not collapse product-specific resume semantics. 0008 supplies the opposite direction of 0007. 0009–0013 are mutually compatible *possibilities*, not preauthorized defaults: no future transformation may alter the ordinary identity-conversion behavior. Their ordering is provisional and should be revisited as real user evidence appears.

A **ready** goal can start once its prerequisite outcome is accepted. A goal with no prerequisite may start now. A **provisional** goal must be refined before activation against accepted prior results and current user priorities. Archive completed goals under docs/goals/archive/ when they are accepted; leave uncompleted goals in this numbered series. Creating these files does **not** start their execution.

Upstream's decision to accept a future AgentsView PR is external and is not treated as an acceptance gate for this project's own goals.
