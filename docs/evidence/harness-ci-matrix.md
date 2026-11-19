# Evidence — harness: the CI matrix is ubuntu and windows, and the platform claim says so

Plan (this increment carries no numbered goal, so the plan is its completion contract):
[`docs/plans/harness-ci-matrix.md`](../plans/harness-ci-matrix.md). Branch `chore/remove-macos-ci-leg`,
off `origin/main` (`e68f18a`); the tree is left in the worktree for operator verification — nothing was
committed, pushed or opened as a pull request.

What it changes: `.github/workflows/gate.yml` (the matrix is `os: [ubuntu-latest, windows-latest]`),
`docs/harness.md` (§CI and §Platform boundary now state what is verified — the macOS gap and the
simulations that pin the platform rules — and the `scripts/tests/` knob names the new pin),
`scripts/tests/test_ci_matrix_claim.py` (the pin, 4 cases), and
`docs/evidence/0001-agentsview-parity-and-inspection.md` (two present-tense CI claims in its
residual-risk section). Historical rows are untouched: the `macos-latest` reds and the repairs they
forced, in the 0002/0003 records and in `docs/harness.md`'s own notes, still read as what those rounds
ran.

## 1. The gate, on the delivered tree

`python3 scripts/gate.py` → `GATE: PASS`, exit `0`, 45.8 s on this host, and re-runs on the delivered
tree — the `selftest` step discovers `docs/evidence/*.md`, so this record's own file is part of the
tree a run reads — print the same verdicts and the same `selftest`/`citations` counts (`388` tests;
`36`/`37` rows), moving only the suite's own time and the tree's own file counts, which this record's
file joins (`secrets` `166` → `167`, files seen by git `5711` → `5712`). The `selftest` step is the
shipped suite: 384 cases before this increment, 388 after (this module's four); the block below is one
of those runs, quoted as the gate printed it. The `parity` step's detail is the gate's own rendering
of the checker's stdout — `tail(proc.stdout, 800)`, its line feeds joined with ` | `, which is why the
line opens on a cut:

```
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 388 tests in 9.388s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5711 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified
  PASS     secrets  no leaks found in 166 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The `parity` step reports the carried tree identical to the pin (5 553 files), the generated input's
pinned digest, and the declared deviations as declared — this increment touches nothing the pin covers.
The `secrets` line's 166 files are this repository's owned scope with this worktree's own scratch
included, as the step's own wording says.

## 2. Each of the pin's rules fails when it is broken

`scripts/tests/test_ci_matrix_claim.py`'s four cases were each shown to fail on its rule broken: a
temporary edit to the shipped workflow or to `docs/harness.md`, the module run once
(`python3 -m unittest scripts.tests.test_ci_matrix_claim`), then the file restored from a copy taken
before the edit and compared byte-for-byte afterwards (`sha256sum`: `gate.yml` `260923d147…` and
`harness.md` `2913c305a8…` held before the first mutation and again after the last restore). Both runs
exited `1`.

| case | rule it pins | mutation | observed |
| --- | --- | --- | --- |
| `test_the_matrix_is_exactly_the_platforms_the_claim_names` | the matrix's legs are exactly the two the claim names | `os: [ubuntu-latest, windows-latest]` → `os: [ubuntu-latest, macos-latest, windows-latest]` | `FAILED (failures=3)`; this case: `AssertionError: Lists differ: ['ubuntu-latest', 'macos-latest', 'windows-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | the job runs the gate file itself | `run: python scripts/gate.py` → `run: python3 scripts/gate.py` (the spelling the Windows runner cannot run) | this case: `AssertionError: 'python scripts/gate.py' not found in 'name: gate\n\non:\n…'` |
| `test_no_job_bypasses_the_matrix` | no job takes a literal runner beside the matrix | `runs-on: ${{ matrix.os }}` → `runs-on: ubuntu-latest` | `FAILED (failures=2)`; this case: `AssertionError: Lists differ: ['ubuntu-latest'] != []` |
| `test_the_claim_names_exactly_the_matrix_s_runners` | §CI names exactly the matrix's runners | §CI's runner list → `on \`ubuntu-latest\`, \`macos-latest\` and \`windows-latest\`` | `FAILED (failures=3)`; this case: `AssertionError: Items in the first set but not the second:` — the extra element is `'macos-latest'` |
| `test_the_section_states_the_platform_the_matrix_does_not_cover` | §CI states the macOS gap while the matrix stays two-legged | `**macOS is not verified on a runner**` → `**macOS has no CI leg**` | this case: `AssertionError: 'macOS is not verified on a runner' not found in "…"` |

The first, fourth and fifth rows were observed in one run (three failures, rc `1`); the second and
third in a second run (two failures, rc `1`). No mutation survived.

## 3. `AGENTS.md` growth: nothing moves

`AGENTS.md` is unchanged. Its one CI-adjacent sentence — "It is the same file CI runs (the runners
name the interpreter `python`)" — stays true, because both remaining runners have `python`; no sentence
there states or implies the matrix or a platform coverage claim, and the read order it fixes already
points at `docs/harness.md`, where the claim lives. This increment changes neither the repository
layout, the gate, the commands, the read order nor a rule an agent follows (the new pin is a
`scripts/tests/` case, which the file's existing harness-tests bullet already covers), so per the
file's growth rule that is said here instead of editing the file for its own sake.

## 4. What else moved, and what did not

- `docs/evidence/0001-agentsview-parity-and-inspection.md`: two sentences of its residual-risk
  section — present-tense claims about CI, not rows recording what a past round ran — moved with the
  claim ("CI runs the same gate on all three" and "CI has it on all three runners" now name
  `ubuntu-latest`/`windows-latest` and state the macOS gap). Every other evidence row and every
  disclosed limit is untouched.
- `docs/harness.md`'s `scripts/tests/` knob gained this module and its claim, in the list that already
  names the suite's doc-claim cases. The `macos-latest` history in §Platform boundary (the reds at
  heads `4cd0567`, `1a476e0` and `524a20f`, and the reproductions they left behind) is unedited —
  those sentences record what past rounds ran and were repaired against.
- Nothing else moved: no gate step (`scripts/gate.py` and `scripts/gate-required.txt` are untouched),
  no carried file, no manifest, no native check, no other workflow line beyond the matrix and the
  comment above it, and no branch-protection or CI setting outside this file.
- The decision implemented here is the owner's (2026-11-18, the operator's ledger entry 34 Q3), not a
  default taken alone, so no `OPEN-QUESTIONS.md` entry accompanies it: nothing is open. Its reversal is
  the plan's — re-add a leg only together with the two files this pin holds in one statement.
- This record's own bytes: it has no §5 comparison table and makes no claim about the `parity` step's
  elision, so the suite's two record-reading cases (`test_evidence_claims.py`,
  `test_gate_transcript_claims.py`) are not exercised by it beyond discovery — stated because both read
  `docs/evidence/*.md`, and a new record is exactly what could trip them.
