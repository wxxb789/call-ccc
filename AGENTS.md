# AGENTS.md — call/ccc

**call/ccc** (*call-with-current-conversation-continuation*) lets a conversation recorded by one
AI-agent harness continue as a new, natively resumable session of another harness.

Read in this order; do not skip: this file → [`docs/core-design/README.md`](docs/core-design/README.md)
→ the active goal in [`docs/goals/`](docs/goals/README.md) → [`docs/conventions.md`](docs/conventions.md)
→ [`docs/upstream.md`](docs/upstream.md) (the carried AgentsView pin) → [`docs/harness.md`](docs/harness.md).

An increment's own records live next to it: its plan in [`docs/plans/`](docs/plans/) (written before
the implementation) and its evidence in [`docs/evidence/`](docs/evidence/) (the commands that ran,
with their exit codes and counts).

## What is binding

- `docs/core-design/README.md` governs every increment. No increment may silently weaken it.
- The goal file is the completion contract for the increment you are implementing: its `Done when`
  clauses are the acceptance, its `Bounds` are hard, and its `Stop and report` section says when to
  stop instead of improvising.
- **AgentsView is the source-side authority.** Its reader, parser, sync, SQLite and migration code
  and their tests are carried at a traceable upstream revision and are never rewritten, simplified
  or forked. A locally maintained parser fork is a defect, not a shortcut. The pin, the declared
  deviations and the update procedure are in [`docs/upstream.md`](docs/upstream.md); the machine
  record is `scripts/upstream-manifest.txt`, and `scripts/upstream-parity.py` — a **required** gate
  step — proves the carried files are still the pin's. Change a carried file only upstream.
- **Precision over apparent completeness.** An explicitly partial translation beats a
  plausible-looking one. Never fabricate tool results, reasoning, roles, resume identities or a
  success result; a valid file is not evidence that anything can resume it.
- Out of scope by design: Git/worktree management, process or environment migration, credentials,
  fleet orchestration, and any second session index for the existing archive.

## How to work here

- Gate: `python3 scripts/gate.py`, which must print `GATE: PASS`; it runs `parity` and `selftest` as
  required steps, and its Go steps are narrowed to `./cmd/ccc/... ./internal/ccc/...` with the measured
  reasons in [`docs/harness.md`](docs/harness.md#why-the-go-steps-target-a-subset) — that narrowing is
  a disclosure, not a shortcut; a run in which a step was skipped,
  bounded or absent prints `GATE: PASS (partial: …)` naming it, so a partial run can never be quoted
  as a full one. It is the same file CI runs (the runners name the interpreter `python`); never narrow
  or skip a step to get green — a step that cannot run is disclosed, not silenced. The gate also runs
  its own regression tests from `scripts/tests/` (the `selftest` step, stdlib only).
- The harness's own tests are part of that contract: an assertion under `scripts/tests/` must fail
  when the rule it names is broken, and must hold on every platform and every Python 3 the docs name
  — no hard-coded separators, no POSIX-only tool behaviour, no assumption that a tool is installed,
  no reliance on a `pathlib` behaviour that differs between Python minors, and no expectation that
  only holds on the platform or the `PATH` the case happens to run on: a case's stub bin answers the
  host probes it is not about, and the suite re-runs itself in a child whose `PATH` holds no tools and,
  once, with the platform probe answering Windows-without-a-compiler, so a host-dependent case fails
  on this host ([`docs/conventions.md`](docs/conventions.md)).
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
