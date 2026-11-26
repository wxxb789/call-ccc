# 0009a — Freeze the native oracle

**Status:** **Re-scoped 2026-11-23, re-cut 2026-11-26 (owner option C) — implemented on
`feat/0009a-native-oracle-v2`; awaiting operator acceptance.** The hold of 2026-11-21 is over, and its
superseded sections (`Intended advance`, `Done when`) were **replaced, not reactivated** — the held text
is in git history, and the superseded attempt (PR #19, sixteen review rounds, closed unmerged) is
preserved at tag `archive/0009a-pre-rescope`. The 2026-11-26 re-cut moves the **disclosure arm** and the
**binary fingerprint** out of this increment into [goal 0009e](0009e-native-check-hardening.md) — after
five cross-family review rounds returned defects in the same three classes, the owner re-decomposed
rather than hardened again; the reviewed starting point of the moved work is tag
`archive/0009a-r16-native-arm` (`768df19`), and the re-cut is recorded in
[`docs/decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md) (`Q-0009a-10`). The oracle this
goal freezes is the **reduced** one below — the offline half alone, read-only for the rest of goal 0009
(the oracle lock, whose boundary is stated under *Intended advance*), with the Codex judgment owned by
the installed `codex` binary.
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
one anchor per line (a receipt covers an artifact holding at least one line; the empty artifact is not
receipt-identified by this rule), so a fixture is re-identified by its bytes in a later increment and a
mutated byte is named by the line it moved; and **the rule registry's own closure**: the declared rule
set equals the mutated rule set, executed by `--selftest-mutations`. **The Codex well-formedness and
replay judgment belongs to the `codex` binary itself**: a native check runs the binary against an
artifact and reports *its* answer (the loader's own `thread/resume`/`thread/read`, and a real turn where
a prompt flag asks for one), and where the binary is absent the check prints the reason and
exits non-zero — never a guess, never a Python restatement of the loader. Once merged, the **offline
oracle** — the rule table and its corpus (`ORACLE_RULES`/`ORACLE_MUTATIONS`), the committed fixtures
under `scripts/tests/testdata/codex-oracle/`, and the `--validate` reading — is **read-only for the rest
of goal 0009** (the oracle lock); the same file's native half stays the native half, and the conductor's
oracle-lock signal is the mechanism: it blocks a fix round that moves this checker alongside production
code, the co-mutation that would move both sides of "the implementation satisfies the oracle" at once.
After goal 0009, a change to the oracle is an ordinary increment — add the rule and the mutation that
turns its fixture red.

**What this goal does not claim.** The offline half **cannot** detect a Codex artifact the binary would
reject — by construction, because it holds no Codex knowledge at all. That limit is disclosed here, in
the increment's evidence record (§4) and in the harness documentation (the CI note: the gate never runs
the native half). The side-by-side *demonstration* of that limit — planting a fabricated artifact and
recording the binary's refusal — is [goal 0009e](0009e-native-check-hardening.md)'s, not this one's.

## Bounds

No Go change; nothing outside `scripts/codex-native-check.py`, its tests, the committed oracle fixtures
(`scripts/tests/testdata/codex-oracle/`), the goal's `docs/evidence/0009a-citations.tsv`, this
increment's own plan/evidence, and the documentation surfaces this increment's own description occupies
— this goal file, `docs/goals/README.md`'s index row, `docs/harness.md`'s native-check section, the
superseded plan stub `docs/plans/0009a-native-oracle.md`, `docs/decisions/OPEN-QUESTIONS.md` and the
moved work's own goal file `docs/goals/0009e-native-check-hardening.md` (whose lock statement this
goal's own must agree with). The existing unflagged Codex acceptance keeps reporting the same verdict. A
cited Go test must already exist and pass.

## Done when

- **D1 — Every surviving offline rule has its own negative fixture.** One mutation per rule at minimum,
  and `--selftest-mutations` proves the declared rule set equals the mutated rule set **over the
  survivors only** — each pair's three arms: the positive fixture passes; the mutation fails citing that
  rule (and, where its bytes are known to break another rule too, exactly those, the pair's `also_cited`
  set); and with `--disable-rule` for the pair's own rule the same bytes carry no violation of that rule
  — a pass where every rule was still evaluated, INCOMPLETE naming the rule the disabled one left
  unjudged where it was not, or still fail citing exactly the other rules its bytes break. The expected
  unjudged rules are **declared with each pair and asserted against the reading the command gets** — the
  command's own reply never supplies the expectation — so a reading that stopped carrying the
  not-evaluated rules turns the corpus red instead of closing green. The corpus
  holds that independence for every pair of byte-reading rules: one mutation violates both, declared in
  each rule's own arm, so disabling either still cites the other.
- **D2 — The offline oracle holds no foreign-format knowledge**, stated testably: the rule registry
  names no Codex member, event type, record kind or ordering, and an artifact that is well-formed as a
  **container** but wrong as a **Codex** artifact passes the offline rules (a case that fails if any
  schema knowledge creeps back).
- **D3 — The Codex judgment is the binary's**: the native check drives the resolved binary and reports
  its answer (the loader's own `thread/resume`/`thread/read`, and a real turn where a prompt flag asks
  for one); absence of the binary is an explicit reason and a non-zero exit; the offline half never
  stands in for it. The **refusal arm** (planting a fabricated artifact and requiring the binary's
  refusal of it) and the **version + SHA-256 fingerprint** of the launched binary are
  [goal 0009e](0009e-native-check-hardening.md)'s acceptance, not this increment's.
- **D4 — Receipts**: a frozen artifact's byte count, whole-file SHA-256 and per-line anchors verify, and
  a single mutated byte breaks the receipt — two cases: verify, and verify-after-mutation.
- **D5 — The deletions are declared**: the deleted rule list, with each rule's fixture and mutation,
  is in the increment's evidence record (§2, derived from PR #19's diff), and
  `docs/core-design/README.md` §2's declared-debt entries for this area (`Q-0005-18`, `Q-0005-20`,
  `Q-0003-12`, `Q-0008-10`, `Q-0005-19`, `Q-0007-13`) were re-read and kept with their conditions (this
  increment touches no Go file and no other native check).

## Non-scope

Any Go change; the other four target routes; the manifest receipt's *producer* (0009c/0009d); the
**disclosure arm** and the **binary fingerprint**, re-cut to [goal 0009e](0009e-native-check-hardening.md)
(2026-11-26, owner option C); and, by
the re-scope, **every rule whose subject is a Codex member, event vocabulary, record shape or replay
ordering** — those were deleted with their fixtures and mutations (the list is the increment's evidence
record §2), and no offline rule may name one again.

## Acceptance surface

`docs/evidence/0009a-citations.tsv` — the surviving rules a gate-runnable Go test can ground — with the
increment's record [`docs/evidence/0009a-native-oracle-v2.md`](../evidence/0009a-native-oracle-v2.md)
(§3: the corpus's closure, the verdict paths with their exit codes, the native run). The native-load
check itself is re-runnable evidence, deliberately not a gate step.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority
or irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause
above.
