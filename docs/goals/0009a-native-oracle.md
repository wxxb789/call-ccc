# 0009a — Freeze the native oracle

**Status:** **Re-scoped 2026-11-23 — implemented on `feat/0009a-native-oracle-v2`; awaiting operator
acceptance.** The hold of 2026-11-21 is over, and its superseded sections (`Intended advance`,
`Done when`) were **replaced, not reactivated** — the held text is in git history, and the superseded
attempt (PR #19, sixteen review rounds, closed unmerged) is preserved at tag `archive/0009a-pre-rescope`.
The oracle this goal freezes is the **reduced** one below: read-only for the rest of goal 0009 (the
oracle lock), with the Codex judgment owned by the pinned `codex` binary.
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md);
the decomposition's rationale, the two cross-family consults it came from, and the per-increment proof
obligations are in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md). The re-scope's
premise and its acceptance draft are in the authoring brief `proposals/0009a-rescope-brief.md` (task
tree); this increment's plan and evidence are
[`docs/plans/0009a-native-oracle-v2.md`](../plans/0009a-native-oracle-v2.md) and
[`docs/evidence/0009a-native-oracle-v2.md`](../evidence/0009a-native-oracle-v2.md).

**Prerequisite:** [0003 — Continue a Codex session in Claude Code](archive/0003-codex-to-claude-continuation.md).

## Intended advance

The native oracle is frozen **with no foreign-format knowledge in Python**. The offline half keeps only
rules that are format-agnostic — **the container**: one JSON object per line, whole terminated UTF-8
lines, no byte-order mark; **receipt integrity**: a frozen artifact's byte count, whole-file SHA-256 and
one anchor per line, so a fixture is re-identified by its bytes in a later increment and a mutated byte
is named by the line it moved; and **the rule registry's own closure**: the declared rule set equals the
mutated rule set, executed by `--selftest-mutations`. **The Codex well-formedness and replay judgment
belongs to the `codex` binary itself**: a native check runs the binary against an artifact and reports
*its* answer (load / resume / refusal), and where the binary is absent the check prints the reason and
exits non-zero — never a guess, never a Python restatement of the loader. Once merged, the oracle file is
**read-only for the rest of goal 0009**; the conductor's oracle-lock signal enforces that over the
reduced file.

**What this goal does not claim.** The offline half **cannot** detect a Codex artifact the binary would
reject — by construction, because it holds no Codex knowledge at all. That limit is disclosed here, in
the increment's evidence record (§4) and in the harness documentation (the CI note: the gate never runs
the native half).

## Bounds

No Go change; nothing outside `scripts/codex-native-check.py`, its tests, the committed oracle fixtures
(`scripts/tests/testdata/codex-oracle/`), the goal's `docs/evidence/0009a-citations.tsv` and this
increment's own plan/evidence. The existing unflagged Codex acceptance keeps reporting the same verdict.
A cited Go test must already exist and pass.

## Done when

- **D1 — Every surviving offline rule has its own negative fixture.** One mutation per rule at minimum,
  and `--selftest-mutations` proves the declared rule set equals the mutated rule set **over the
  survivors only** — each pair's three arms: the positive fixture passes, the mutation fails citing that
  rule alone, and the same bytes pass with `--disable-rule` for it.
- **D2 — The offline oracle holds no foreign-format knowledge**, stated testably: the rule registry
  names no Codex member, event type, record kind or ordering, and an artifact that is well-formed as a
  **container** but wrong as a **Codex** artifact passes the offline rules (a case that fails if any
  schema knowledge creeps back).
- **D3 — The Codex judgment is the binary's**: the native check drives the resolved binary and reports
  its answer; every run records the resolved version and the binary's own SHA-256 (the owner follows
  `latest`; no version is re-pinned), and absence of the binary is an explicit reason and a non-zero
  exit; the offline half never stands in for it.
- **D4 — Receipts**: a frozen artifact's byte count, whole-file SHA-256 and per-line anchors verify, and
  a single mutated byte breaks the receipt — two cases: verify, and verify-after-mutation.
- **D5 — The deletions are declared**: the deleted rule list, with each rule's fixture and mutation,
  is in the increment's evidence record (§2, derived from PR #19's diff), and
  `docs/core-design/README.md` §2's declared-debt entries for this area (`Q-0005-18`, `Q-0005-20`,
  `Q-0003-12`, `Q-0008-10`, `Q-0005-19`) were re-read and kept with their conditions (this increment
  touches no Go file and no other native check).

## Non-scope

Any Go change; the other four target routes; the manifest receipt's *producer* (0009c/0009d); and, by
the re-scope, **every rule whose subject is a Codex member, event vocabulary, record shape or replay
ordering** — those were deleted with their fixtures and mutations (the list is the increment's evidence
record §2), and no offline rule may name one again.

## Acceptance surface

`docs/evidence/0009a-citations.tsv` — the surviving rules a gate-runnable Go test can ground — with the
increment's record [`docs/evidence/0009a-native-oracle-v2.md`](../evidence/0009a-native-oracle-v2.md)
(§3: the corpus's closure, the verdict paths with their exit codes, the native run and its disclosure
arm). The native-load check itself is re-runnable evidence, deliberately not a gate step.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority
or irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause
above.
