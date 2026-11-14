# AGENTS.md — call/ccc

**call/ccc** (*call-with-current-conversation-continuation*) lets a conversation recorded by one
AI-agent harness continue as a new, natively resumable session of another harness.

Read in this order; do not skip: this file → [`docs/core-design/README.md`](docs/core-design/README.md)
→ the active goal in [`docs/goals/`](docs/goals/README.md) → [`docs/conventions.md`](docs/conventions.md)
→ [`docs/harness.md`](docs/harness.md).

## What is binding

- `docs/core-design/README.md` governs every increment. No increment may silently weaken it.
- The goal file is the completion contract for the increment you are implementing: its `Done when`
  clauses are the acceptance, its `Bounds` are hard, and its `Stop and report` section says when to
  stop instead of improvising.
- **AgentsView is the source-side authority.** Its reader, parser, sync, SQLite and migration code
  and their tests are carried at a traceable upstream revision and are never rewritten, simplified
  or forked. A locally maintained parser fork is a defect, not a shortcut.
- **Precision over apparent completeness.** An explicitly partial translation beats a
  plausible-looking one. Never fabricate tool results, reasoning, roles, resume identities or a
  success result; a valid file is not evidence that anything can resume it.
- Out of scope by design: Git/worktree management, process or environment migration, credentials,
  fleet orchestration, and any second session index for the existing archive.

## How to work here

- Gate: `python scripts/gate.py`, which must end in `GATE: PASS`. It is the same command CI runs;
  never narrow or skip a step to get green — a step that cannot run is disclosed, not silenced. The
  gate also runs its own regression tests from `scripts/tests/` (the `selftest` step, stdlib only).
- Stage order for an increment: implement → verify (gate) → simplify (report only) → apply (accepted
  findings only) → review (report only) → fix → gate → PR. Report stages must not edit the tree.
- One goal ≈ one branch ≈ one pull request, squash-merged onto a linear `main`. Do not commit, push
  or open a PR unless the dispatched stage explicitly asks for it; the operator owns the merge.
- Owner-only decisions never block an unattended increment: take the most reversible default,
  disclose it in the report, and append an entry to
  [`docs/decisions/OPEN-QUESTIONS.md`](docs/decisions/OPEN-QUESTIONS.md).

## This file grows

`AGENTS.md` is the project's control file and is expected to change as the project does. When an
increment changes the repository layout, the gate, the commands, the read order, or any rule an agent
must follow, update this file **in the same increment**. When it changes none of those, say so in the
increment's report instead of editing the file for its own sake. Keep it short: binding rules and
pointers, not a tour of the code.
