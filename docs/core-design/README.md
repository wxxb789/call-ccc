# Core Design — call/ccc

> **call/ccc** (*call-with-current-conversation-continuation*) lets a conversation recorded by one AI-agent harness continue in another harness, as a native, resumable session. Its default behavior preserves available history rather than rewriting it.

## 1. Product and taste

The smallest successful experience is one unambiguous command that takes a source session and a target harness, creates a **new native target session**, and returns its native resume identity and command. A coding agent should learn the tool from --help or a short machine-readable capability response; a skill is optional.

This is a **session and context tool**, not a project-state migration tool. It does not manage Git, synchronize worktrees, restore processes, transfer credentials, orchestrate agents, operate a remote fleet, or replicate an entire execution environment. A resumed agent can investigate its own environment. Do not introduce infrastructure to solve problems outside the session boundary.

Favor **precision over apparent completeness**, **small interfaces over frameworks**, and **observable continuity over plausible-looking files**. An explicitly partial translation is better than fabricated tool history, reasoning, roles or a misleading success result.

## 2. AgentsView is the source-side authority

The source discovery, native parsing, normalization, synchronization and SQLite archival machinery are **owned by AgentsView**. In this project, carry the actual upstream source tree at a traceable revision, without rewriting, reimplementing, simplifying or changing the behavior of its reader/parser/archive packages and schema.

The engineering definition of "unchanged" is testable: compare the inherited source-side files and their tests against the pinned upstream Git revision and report any difference. Upstream updates should preserve their ancestry and keep call/ccc changes outside upstream-owned code wherever possible. Preserve original license notices. If an upstream fix is genuinely necessary, contribute it upstream independently rather than maintaining an invisible local parser fork.

**Each half has one owner.** AgentsView's half is **n → 1**: n input formats into one intermediate conversation document. call/ccc's new half is **1 → n**: that document into each target harness's own native session format. Where the two can disagree about a foreign format's shape — a record's wire shape, a field's type, whether a loader accepts or skips a line — the referee is the upstream reader or the real binary that owns the format, never a re-implementation inside call/ccc. A private decoder, schema or replay state machine for a foreign format is a defect rather than a shortcut, and it is the failure mode this boundary exists to prevent. `AGENTS.md` carries this rule's operable form — what discharges an obligation about a foreign format — and the single authoritative list of the places that are **declared debt instead** is the one below.

**Declared debt: the places that still re-derive a foreign judgment.** Three exceptions stand today. Each is debt rather than licence: it is kept only where nothing else pins that behaviour, it may not grow a new local judgment, and it is replaced the moment an authoritative read answers its question. This list is the single enumeration — a new exception is added here, not in a second place.

- **The Pi-family source leaf check** (`internal/ccc/pi_source_leaf.go`, required by the leaf rule in `AGENTS.md`): it walks a source file's own entry path to establish which entry the file selects as its leaf, because the carried Pi reader skips the tree entries Pi's own loader builds context from. *Replaced by* Pi's own loader answering for the file (a load/`get_state`-shaped read) or an authoritative source-side read — condition in `docs/decisions/OPEN-QUESTIONS.md`'s `Q-0005-18`.
- **The OpenCode source oracle's expectations** (`scripts/opencode-native-check.py`): it computes the dialogue and tool results it expects from a Python reconstruction of the carried OpenCode reader, and it already answers differently — it refuses a `shell` record the reader reads as tool-bearing history. *Replaced by* expectations taken from an independent AgentsView read of the OpenCode source, with the installed target's read-back kept on the observed side — condition in `Q-0008-10`.
- **The Pi/OMP target-side replay prediction** (`internal/ccc/pi_verify.go`'s parent-chain walk and role prediction, which both the Pi and OMP verifiers use to accept or reject a native load, mirrored in `scripts/pi-native-check.py` and `scripts/omp-native-check.py`): it predicts the roles the target's own loader should return. *Replaced by* the loader's own answer to that question where the check already drives it — condition in `Q-0005-19`.

Go's internal-package import restriction matters: a foreign module named github.com/wxxb789/call-ccc cannot directly import go.kenn.io/agentsview/internal/parser. The default direction is to preserve the upstream Go module identity and ancestry in this repository, add a separate cmd/ccc binary and isolated continuation packages **within the permitted import tree**, and distribute the standalone CLI as a binary. Do not rename all upstream imports just to create a new module path.

AgentsView remains the owner of sessions.db, its existing tables, WAL, indexes, data-version upgrades, parser checkpoints and writer-ownership rules. call/ccc must not add a competing session index, schema migration, file watcher or database writer. Access to a live archive must respect the existing AgentsView daemon/lock semantics. Reuse the upstream query and sync paths rather than issuing independent writes to its database.

**The archive is a normalized projection, not a lossless session container.** AgentsView archive_content can be full, transcripts or usage. Some native input structures are not reconstructible from the stored view. A continuation adapter may use the original source bytes or an authoritative native export *located through the upstream source identity* where the target's importer or fidelity demands it. That is not permission to implement a second source parser or silently reconstruct missing evidence.

## 3. One new capability: continuation

The only novel core pipeline is:

~~~text
AgentsView source/SQLite → Conversation document → Transform → Target writer
                                    |               (identity by default)
                                    +→ loss/provenance evidence
~~~

**Conversation document** is a typed, transient interchange representation, not a second persistent database schema. Capture ordered user and assistant messages, tool calls and results with their pairing, relevant content blocks/attachments, and source/branch metadata when available. Keep provider-specific identity/provenance so target writers can make informed decisions; do not force all providers into a lowest-common-denominator text transcript.

Readers are adapted from the **existing** AgentsView output. Target writers are new, format-family-aware components. The composition should scale by source and target adapter, rather than maintain a special converter for every directed agent pair. Reuse common native format families where justified, while treating products with different resume paths or metadata as distinct installations.

Prefer a supported **native target import** interface when one exists and satisfies the required semantics; otherwise use a narrowly scoped native writer backed by observed target-format evidence. A native import route and an IR route may coexist. Internal interfaces describe capabilities and conversion outcomes; they must not depend on Cobra, terminal prompts, HTTP handlers or JSON presentation.

The public engine is a Go API callable independently of cmd/ccc. No CLI types or process-global state enter the continuation core. Future embedding in AgentsView should require a thin adapter/command, not a copy of the engine.

## 4. Continuation semantics

A conversion is a **one-way snapshot**, not live synchronization or process migration. It captures history through the latest complete, usable recorded event at the chosen source boundary. A source that grows while being read may leave later events out; the user or agent may explicitly convert again. Do not invent trailing tool results or consume malformed partial writes as complete messages.

The unit is the **selected session**, not an inferred whole project. Do not automatically follow prior compactions, external session links, ancestor runs, subagents or files in order to fabricate a perfect context window. Preserve available compaction/branch boundaries when the source representation supplies them, and report meaningful omissions; do not revive deliberately inactive history as though it were active. Other history can be converted separately on demand.

Create a fresh target identity. Never overwrite the source or silently modify a target session that a different agent may have already continued. For repeated conversions, make the selected source boundary and resulting target session distinguishable; do not promise cross-session merging or automatic catch-up.

A valid JSONL file is not by itself success. Distinguish **converted**, **installed** and **natively resumable**. Validate the target structure and, where supported, exercise the installed target's native load/resume interface. The same holds for a target whose artifact is not a file but a row inside the harness's own store: what was published is what that harness's own interface accepted, and that harness's own reader is what shows it, so an installation that needs the harness is reported as the boundary it is rather than worked around. A semantic continuation test can provide stronger evidence but must not be falsely reported merely from successful parsing. Disclose lost or transformed meaning by category and provenance; do not quietly claim losslessness.

Keep model-private state, credentials, approval state, executable tool activity and runtime processes out of scope. A historical tool result is **evidence**, not an invitation to replay the tool. An unrepresentable tool role, image, tool result or reasoning trace must not be forged into equivalent-looking authoritative data.

## 5. User and agent interface

**Human-friendly and LLM-friendly are the same design requirement.** The common case requires little discovery and no mandatory skill. A stable help page and a machine-readable format/capability listing explain available source and target formats and their limits.

A minimal direction is:

~~~text
ccc continue claude:SESSION_ID --to codex
ccc continue claude:SESSION_ID --to codex --json
ccc continue claude:SESSION_ID --to codex --dry-run
ccc inspect claude:SESSION_ID
ccc formats --json
~~~

These are desired user experiences, not an immutable CLI grammar. The core operates without interactive prompts. Interactive launch of a target agent is not an implicit part of conversion. If an identifier is ambiguous, return the candidates instead of guessing. A target result supplies an exact native session ID, working directory if known, and resume arguments; avoid asking an agent to scrape human-readable prose.

When JSON is requested, stdout contains a stable structured result; progress and diagnostics use stderr. Failure classes, fidelity warnings and whether native resume was actually verified are explicit. Avoid flags whose combinations require agent-specific procedural knowledge.

## 6. Evolution through an identity transform

The initial Transform is **identity**: preserve selected available evidence and let the target writer express it as faithfully as its native format allows. Do not call an LLM, summarize, compact, redact or rewrite content by default.

Later, opt-in and user-controlled transformations can include message/range selection, selective/high-density compaction, summarization, minimally scoped redaction and editing of **actually exposed, persisted** reasoning traces. Transformations act on the conversation document, never on the source archive. Their inputs, outputs and semantic losses must be inspectable. A context edit is not a replay of the agent's hidden cognition.

Privacy-oriented redaction may reduce unnecessary sensitive detail and accidental service-filter false positives, but it is not a promise to defeat a provider's safety checks. Never invent a clean tool result or obscure a material change to the conversation's meaning. Do not build a transform plugin framework, policy DSL or model integration before a real user-facing transform needs one.

## 7. Performance and operational discipline

Local use should feel immediate for normal sessions. Cost should scale with the chosen source and generated target, not with all archived conversations, all agent providers, or the size of the workspace. Preserve AgentsView's mature incremental readers and SQLite architecture; avoid a second always-on daemon, index, WAL manager or full-archive scan per conversion.

Large transcripts, attachments and tool outputs are legitimate inputs. Keep reads and writes bounded where practical; stream or stage larger artifacts rather than requiring unbounded in-memory copies or huge terminal output. Do not duplicate expensive source fingerprints solely for bookkeeping. Measure meaningful regressions using representative native sessions, rather than setting arbitrary speed or memory numbers.

Native installation must be non-destructive and as atomic as the target supports. Keep file permissions restrictive, never copy login stores or tokens, and do not log conversation bodies, tool arguments, secrets, full prompts or reasoning content by default. Version drift across target harnesses is a compatibility boundary: validate with native format fixtures and real resume oracles on tested versions.

Prefer Go standard-library mechanisms, narrowly scoped dependencies, explicit context propagation and deterministic error reporting. Match upstream test style, format-family reuse, source-format provenance and focused review practices. Prioritize durable tests of behavior and failure modes over generated wrappers.

## 8. What proves the design

Every supported directed route needs evidence that the source history was obtained through inherited AgentsView behavior, the target is readable by its native harness, the original source remains unchanged, and any known semantic loss is reported. Include realistic cases involving tool calls/results, branching or compaction markers, missing payloads, and a growing source where applicable.

Use small, sanitized native-shaped fixtures plus version-aware native load/resume checks. Do not claim that one successful import proves arbitrary agent or model equivalence. Keep verification proportional: syntax and structure tests are necessary but distinct from native-resume and contextual-quality claims.

The source-side upstream parity check is a separate invariant from route-level tests. Upstream compatibility and the stand-alone CLI must both remain credible after changes. If a feature forces pervasive edits to AgentsView's source side, reconsider the boundary before proceeding.

## 9. Scope and release posture

Go CLI first; Linux, macOS and Windows are intended platforms subject to native harness availability. Source-side upstream parity and target resume correctness take priority over supporting many formats. Early coverage should prove Claude Code ↔ Codex, then grow through other genuinely needed format families, including Pi/OMP, OpenCode, DeepSeek Harness and GitHub Copilot CLI; extensibility means not accumulating an N-by-N mesh of pair-specific converters. Native source and target support are **directional capabilities**: inheriting an AgentsView reader does not imply a supported writer. VS Code Copilot and Visual Studio Copilot may initially contribute source-only conversation history; telemetry-only traces can omit substantial context. Do not fabricate an editor-native resume path or claim full-fidelity conversion when the source did not record the history.

Do not prematurely stabilize pre-1.0 internal formats or invent backward-compatibility obligations for unused APIs. Keep the long-term Context Management ambition visible, but defer transformation and multi-agent integration until ordinary continuation is trustworthy. Future AgentsView upstream acceptance is desirable, not guaranteed; each new module should be useful independently and straightforward to transplant.

For execution order and evidence, see [the goal series](../goals/README.md). This design governs the series, not a prescriptive implementation plan.
