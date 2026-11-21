# Plan — prep increment: the ownership rule and 0015–0017's deferral

**Status:** landed as PR #20 (`prep/top-rule-and-deferrals`). A **prep increment**: it carries no goal id, no
`Done when` and no goal-ledger row, and it is operator-merged.

**Why a prep increment exists here at all:** the two things it carries are decisions *about* the series rather
than increments *of* it — one changes what a rule binds, the other changes which goals the unattended chain
dispatches. Both had to land before the goals they govern are implemented, and neither could ride inside a goal
without making that goal's acceptance depend on text the owner had not approved.

## Scope

Three documents and this record pair; **no Go file, no script, no gate step, no CI workflow**.

1. `AGENTS.md` (What is binding) — the ownership rule: **each half has one owner.**
2. `docs/core-design/README.md` §2 — the same rule one level up, the **single authoritative list** of the
   places that are declared debt instead, and a pointer to the contract's operable form.
3. `docs/goals/README.md` — 0015, 0016, 0017 leave the unattended chain; 0009a is marked **Rescope pending**;
   the relationship paragraph states the chain's last increment and what changes that.
4. `docs/decisions/OPEN-QUESTIONS.md` — `Q-0005-18`, `Q-0008-10`, `Q-0005-19`: the three debt sites, each with
   its options, the default taken, its replacement condition and what settles it.

## The decisions, restated here so a fresh checkout does not need the task ledger

The owner answered a batch of questions on **2026-11-21**, one at a time, with options and their trade-offs.
The two that this increment lands:

- **The ownership rule (binding).** The session-reading half of this project is **AgentsView's** — **n → 1**,
  n input formats into one intermediate conversation document, carried verbatim at the pin. This repository's
  half is **1 → n** — that document into each target harness's own native session format. Where the two can
  disagree about a foreign format's shape, **the referee is the upstream reader or the real binary that owns
  the format, never a re-implementation inside `call/ccc`.** An obligation to know what a foreign tool accepts
  is discharged by running that tool against the artifact, or by citing the carried reader's own code. The rule
  is the owner's, given as a top rule; the two debts that predate it are enumerated in
  `docs/core-design/README.md` §2 rather than being hidden by it.
- **The deferral.** **0015** (GitHub Copilot CLI), **0016** (VS Code Copilot) and **0017** (Visual Studio
  Copilot) leave the unattended chain: 0015 waits for the owner's other machine — he does not want Copilot
  installed on this host — and 0016/0017 for an IDE environment this host does not have. Their goal texts and
  their `Done when` clauses are **unchanged**; this is a scheduling deferral, not a scope change, and no goal
  is closed on a discounted acceptance. The chain's **last** increment is therefore 0014.

Two further owner answers of the same batch are *recorded* here because other records cite them: the offline
Codex oracle keeps only container and receipt integrity while **the `codex` binary owns Codex
well-formedness** (which is why 0009a now reads *Rescope pending*), and the oracle's binary is **not pinned**
— the host tracks `latest`.

## What this increment does not do

- It does not rescope 0009a: its goal file is untouched, and the hold in the index is a dispatch instruction.
- It does not touch 0010–0013's provisional texts, or `docs/core-design/README.md`'s paragraph on the Go
  `internal/` import restriction — the layout change that supersedes that paragraph lands as its own increment,
  so no increment ships a document that disagrees with the tree.
- It changes no behaviour: no verifier, no native check, no writer. The three debt sites stay exactly as they
  were.

## How it lands

`prep/top-rule-and-deferrals` → PR #20 against `main` → `scripts/gate.py` green on the pushed head → CI green
on both matrix legs → the **two machine stages** (simplify report + apply; code review report + fix), findings
fixed to clean → squash merge, no `done` row. No citations manifest is owed: no acceptance surface here names
one. The evidence record is [`docs/evidence/prep-top-rule-and-deferrals.md`](../evidence/prep-top-rule-and-deferrals.md).
