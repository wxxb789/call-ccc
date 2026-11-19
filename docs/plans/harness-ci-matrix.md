# Plan — harness: the CI matrix is ubuntu and windows, and the platform claim says so

Status: **harness increment** (no numbered goal). Owner decision 2026-11-18 — the operator's ledger,
`~/claw/tasks/2026-11-14/call-ccc/DECISIONS.md`, entry 34 question Q3: the macOS CI leg is removed, and
the platform claim is downgraded to what is actually verified. That decision is *made*, not open, so it
is recorded here with its reason rather than as an `OPEN-QUESTIONS.md` entry.

Branch `chore/remove-macos-ci-leg`, off `origin/main` (`e68f18a`). One goal ≈ one branch ≈ one PR;
squash-merged onto a linear `main`. The operator owns the merge.

## 1. The decision, and why

The `macos-latest` job was repeatedly starved of a runner. A job that fails **slowly with zero executed
steps** is infrastructure, not a verdict — `gh pr checks` reports that shape as `fail`, so the leg cost
red runs that said nothing about the tree, and its occasional green said only that the same gate passed
one more time. Removed rather than tolerated: the project's claim should say what is verified, and a
green CI run should mean what a reader thinks it means.

## 2. The change

1. `.github/workflows/gate.yml` — the matrix loses its macOS leg: `os: [ubuntu-latest, windows-latest]`.
   Same job shape (one job per leg, `runs-on: ${{ matrix.os }}`), same command
   (`run: python scripts/gate.py`), same triggers (`pull_request`, `push` to `main`).
2. `docs/harness.md` — §CI states what now holds: ubuntu and windows run the whole gate on real
   runners; **macOS is not verified on a runner** (no CI leg); a macOS-shaped rule is answered in CI
   only by the suite's own executed host simulations, which run wherever the `selftest` step runs; a
   green run is evidence about Linux and Windows only. §Platform boundary gains the same qualification
   beside the rules themselves ("where each answer is *verified* is narrower than where it applies"),
   and keeps those rules: a macOS host running the gate still treats a missing Go toolchain or C
   compiler as a failure, and the `macos-latest` failures the section records stay as history.
3. `scripts/tests/test_ci_matrix_claim.py` — **the pin** (run by the `selftest` step). Before this
   increment the platform claim had no check at all, which is what this increment exists to fix. Seven
   cases ship on this branch — four in the plan's first cut, extended by the apply round (the
   simplification review's accepted findings) and by the fix round (the code review's findings), both
   recorded in `docs/evidence/harness-ci-matrix.md` — and they read the two shipped files and fail when
   either moves alone: the matrix's legs are exactly `ubuntu-latest` + `windows-latest` (its `os:` list
   and every `include:`/`exclude:` entry, combined order-independently, read from every workflow file);
   every job of the gate workflow runs the gate file itself as a whole step's `run:` value; §CI names
   exactly the matrix's runner labels; §CI states `macOS is not verified on a runner` while the matrix
   stays two-legged; the §Platform boundary paragraph is the claim's other live copy (a retired-runner
   mention needs a history marker); and the reader's contract — the shapes it accepts, and every shape
   it refuses, one named case per branch. The reader reads the workflow structurally and fails closed: a
   shape it does not read exactly (a `runs-on` spelled or valued otherwise, a block scalar, a quoted or
   block label list, a step-level or job-level `if:`, a trigger block outside the accepted shape, a
   second matrix) is refused by a failing case naming it rather than read past.
4. `docs/evidence/0001-agentsview-parity-and-inspection.md` — the two present-tense CI claims in that
   record's residual-risk section ("CI runs the same gate on all three", "CI has it on all three
   runners") now state what holds. Historical rows are untouched: what a past round ran, and the
   `macos-latest` reds and reproductions the 0002/0003 records and `docs/harness.md` quote, stay as
   they are.
5. `AGENTS.md` — **no change**: no sentence there states or implies the CI matrix or a platform
   coverage claim ("the runners name the interpreter `python`" stays true for both remaining runners),
   and this increment changes neither the layout, the gate, the commands nor the read order, so the
   growth rule asks for no edit.
6. Records: this plan and `docs/evidence/harness-ci-matrix.md`.

## 3. What this does not authorize

- No second gate script, no new or second workflow, no branch-protection change, and no change to what
  `scripts/gate.py` runs or to `scripts/gate-required.txt`.
- No rewriting of history: the evidence rows that record the `macos-latest` legs' reds and repairs (and
  the harness notes quoting them) are unedited; only present-tense claims about CI move.
- No claim that macOS is unsupported or that the gate no longer *works* there: macOS stays an intended
  platform in the core design, a macOS host running the gate still gets the answers the platform
  contract states, and the suite still pins the macOS-shaped rules it can exercise — what is withdrawn
  is the claim that a runner verifies any of it.
- No change to the carried tree, the upstream pin, `parity`, the Go targets or the native checks.

## 4. Acceptance

- The workflow's matrix reads ubuntu + windows only, and an operator can see that on the file itself;
  the job, command and triggers are the shape above.
- The docs' claim and the matrix are one statement, and the new case is its pin: §CI names exactly the
  matrix's runners, states the macOS gap, and the matrix cannot regain a leg without the section (and
  the decision record) moving with it.
- `python3 scripts/gate.py` → `GATE: PASS`, with the `selftest` and `citations` lines reported verbatim
  with their counts (the `selftest` count grows by this increment's own cases — four in the plan's first
  cut, seven as shipped, the extensions recorded in `docs/evidence/harness-ci-matrix.md`).
- Each of the pin's rules shown to fail when broken, by a temporary mutation and a byte-identical
  restore (recorded in `docs/evidence/harness-ci-matrix.md`).
