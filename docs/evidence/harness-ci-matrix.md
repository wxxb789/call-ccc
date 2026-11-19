# Evidence — harness: the CI matrix is ubuntu and windows, and the platform claim says so

Plan (this increment carries no numbered goal, so the plan is its completion contract):
[`docs/plans/harness-ci-matrix.md`](../plans/harness-ci-matrix.md). Branch `chore/remove-macos-ci-leg`,
off `origin/main` (`e68f18a`); the tree is left in the worktree for operator verification — nothing was
committed, pushed or opened as a pull request.

What it changes: `.github/workflows/gate.yml` (the matrix is `os: [ubuntu-latest, windows-latest]`),
`docs/harness.md` (§CI and §Platform boundary now state what is verified — the macOS gap and the
simulations that pin the platform rules — and the `scripts/tests/` knob names the new pin),
`scripts/tests/test_ci_matrix_claim.py` (the pin: four cases, six after the apply round recorded in §4),
and `docs/evidence/0001-agentsview-parity-and-inspection.md` (two present-tense CI claims in its
residual-risk section). Historical rows are untouched: the `macos-latest` reds and the repairs they
forced, in the 0002/0003 records and in `docs/harness.md`'s own notes, still read as what those rounds
ran.

## 1. The gate, on the delivered tree

`python3 scripts/gate.py` → `GATE: PASS`, exit `0`, 45.8 s on this host, and re-runs on the delivered
tree — the `selftest` step discovers `docs/evidence/*.md`, so this record's own file is part of the tree
a run reads — print the same verdicts and the same `citations` counts (`36`/`37` rows), moving only the
suite's own time and case count (the pin's, §4) and the tree's own file counts, which this record's file
joins (`secrets` `166` → `167`, files seen by git `5711` → `5712`). The `selftest` step is the shipped
suite: 384 cases before this increment, 388 when this record was first written (this module's four; the
first block below quotes that run), 390 on the delivered tree (the pin's two apply-round cases; the
second block quotes the re-run). The `parity` step's detail is the gate's own rendering of the checker's
stdout — `tail(proc.stdout, 800)`, its line feeds joined with ` | `, which is why the line opens on a
cut:

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

`scripts/gate.py` re-run after §2's mutation round was recorded — the delivered tree, exit `0`, 44.8 s
on this host — quoted as the gate printed it:

```
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 390 tests in 8.763s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5712 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified
  PASS     secrets  no leaks found in 167 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The `parity` step reports the carried tree identical to the pin (5 553 files), the generated input's
pinned digest, and the declared deviations as declared — this increment touches nothing the pin covers.
The `secrets` line's file count is this repository's owned scope with this worktree's own scratch
included, as the step's own wording says (166 before this record's file existed at a run; 167 after).

## 2. Each of the pin's rules fails when it is broken

The apply round extended the pin (§4) and re-ran every mutation the simplification review's four
accepted findings had demonstrated. The battery runs against a throw-away copy of the tree
(`/tmp/ccc-ci-matrix-apply/tree` holds the three files the module reads — `.github/workflows/gate.yml`,
`docs/harness.md`, `scripts/tests/test_ci_matrix_claim.py` — and the module resolves its root from its
own path, so the copy is enough): each mutation edits one of the copy's files, the module runs once
(`python3 -m unittest scripts.tests.test_ci_matrix_claim`), and the copy's unmutated control run prints
`Ran 6 tests … OK` (exit `0`) while every mutation below exits `1`. The shipped files' digests held
before and after the battery — `sha256sum`: `gate.yml` `260923d147…`, `harness.md` `2913c305a8…` (the
same two digests the first round recorded) — the battery never wrote into the worktree.

| case | rule it pins | mutation | observed |
| --- | --- | --- | --- |
| `test_the_matrix_is_exactly_the_platforms_the_claim_names` | the matrix's legs are exactly the two the claim names | `os: [ubuntu-latest, windows-latest]` → `os: [ubuntu-latest, macos-latest, windows-latest]` | `FAILED (failures=1)`; this case: `AssertionError: Lists differ: ['ubuntu-latest', 'macos-latest', 'windows-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `include:` entry is a leg (review finding 2) | `include:` + `- os: macos-latest` under the matrix | `FAILED (failures=1)`; this case: `Lists differ: ['ubuntu-latest', 'windows-latest', 'macos-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `exclude:` entry is a leg (review finding 2) | `exclude:` + `- os: windows-latest` under the matrix | `FAILED (failures=1)`; this case: `Lists differ: ['ubuntu-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | the job runs the gate file itself (review finding 4) | `run: python scripts/gate.py` → `# run: python scripts/gate.py` + `run: python -m gate`; and → `run: python3 scripts/gate.py` (the spelling the Windows runner cannot run) | `FAILED (failures=1)`; this case both times: `AssertionError: 'python scripts/gate.py' not found in ['go install …', 'python -m gate']` / `… ['go install …', 'python3 scripts/gate.py']` |
| the same case | every workflow file is read (review finding 3) | `.github/workflows/other.yml` with its own matrix `os: [ubuntu-latest, macos-latest]` and `runs-on: ${{ matrix.os }}` | `FAILED (failures=1)`; this case: `Lists differ: ['macos-latest'] != []` — `other.yml runs a matrix leg outside the runners the claim names` |
| `test_no_job_bypasses_the_matrix` | no job takes a literal runner | `runs-on: ${{ matrix.os }}` → `runs-on: ubuntu-latest` | `FAILED (failures=1)`; this case: `Lists differ: [('.github/workflows/gate.yml', 'ubuntu-latest')] != []` |
| the same case | the same, in a second workflow file (the review's own demonstration) | `.github/workflows/other.yml` with `runs-on: macos-latest` and `run: python scripts/gate.py` | `FAILED (failures=1)`; this case: `Lists differ: [('.github/workflows/other.yml', 'macos-latest')] != []` |
| `test_the_claim_names_exactly_the_matrix_s_runners` | §CI names exactly the matrix's runners | §CI's runner list → `on \`ubuntu-latest\`, \`macos-latest\` and \`windows-latest\`` | `FAILED (failures=1)`; this case: `Items in the first set but not the second:` — the extra element is `'macos-latest'` |
| `test_the_section_states_the_platform_the_matrix_does_not_cover` | §CI states the macOS gap | §CI's `**macOS is not verified on a runner**` → `**macOS has no CI leg**` | `FAILED (failures=1)`; this case: `AssertionError: 'macOS is not verified on a runner' not found in "…"` |
| `test_the_platform_rules_state_the_gap_and_the_runners_too` | the platform rules' paragraph is the claim's other live copy (review finding 1) | the paragraph's `**macOS is not verified on a runner**` → `**macOS has no CI leg**` | `FAILED (failures=1)`; this case: `AssertionError: docs/harness.md §Platform boundary no longer states the gap ('macOS is not verified on a runner')` |
| the reader (`read_workflow`; so all six cases) | one matrix per workflow; a second is refused | a second job with `strategy.matrix` `os: [ubuntu-latest, macos-latest]` + `runs-on: ${{ matrix.os }}` | `FAILED (failures=6)`; `UnreadableWorkflow: …gate.yml: \`matrix:\` keys at lines 17, 58 — a second \`strategy.matrix\` is a shape this pin refuses: it reads one matrix per workflow…` |
| the reader | an `os:` it cannot place is refused | a second job with a bare `os: [ubuntu-latest, macos-latest]` + `runs-on: ${{ matrix.os }}` (no second `matrix:`) | `FAILED (failures=6)`; `UnreadableWorkflow: …gate.yml: line 25: an \`os:\` outside the matrix — the pin reads the matrix's own \`os:\`, \`include:\` and \`exclude:\`…` |
| the reader | an `os:` it cannot parse is refused | `os: [ubuntu-latest, windows-latest]` → the block form (`os:` then `- ubuntu-latest`, `- windows-latest`) | `FAILED (failures=6)`; `UnreadableWorkflow: …gate.yml: line 22: an \`os:\` this pin cannot read ('') — the pin reads the inline list form…` |
| the reader | a `uses:` job is refused (review finding 3) | `.github/workflows/other.yml` with `call: uses: ./.github/workflows/gate.yml`; and the same job appended to `gate.yml` itself | `FAILED (failures=6)`; `UnreadableWorkflow: …other.yml: line 8: … a \`uses:\` job hands its run to a called or remote workflow, which this pin cannot read…` (the appended one refuses at `gate.yml: line 54`) |
| the reader | a form it does not read is refused, not guessed | an `include:` entry written at `include:`'s own indent (valid YAML, a form the reader does not model) | `FAILED (failures=6)`; `UnreadableWorkflow: …gate.yml: line 24: a list item inside the matrix outside an \`include:\`/\`exclude:\` block — the pin cannot place it` |

No mutation survived. The reader's refusal contract — second matrix, `os:` outside or in an unparsed
form, a matrix with no `os:`, an unread key inside the matrix, a `uses:` job — is held on the shipped
tree by `test_the_reader_refuses_a_shape_it_cannot_certify` (six crafted shapes, each's refusal message
asserted); the rows above are the same reader driven on the real files.

## 3. `AGENTS.md` growth: nothing moves

`AGENTS.md` is unchanged. Its one CI-adjacent sentence — "It is the same file CI runs (the runners name
the interpreter `python`)" — stays true, because both remaining runners have `python`; no sentence there
states or implies the matrix or a platform coverage claim, and the read order it fixes already points at
`docs/harness.md`, where the claim lives. This increment changes neither the repository layout, the
gate, the commands, the read order nor a rule an agent follows (the pin is a `scripts/tests/` case,
which the file's existing harness-tests bullet already covers), so per the file's growth rule that is
said here instead of editing the file for its own sake. The apply round re-checked this against its own
change: the pin's extension lives entirely inside `scripts/tests/` — cases and a reader — so the same
bullet covers it and the conclusion is unchanged.

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
- The apply round (2026-11-19) extended `scripts/tests/test_ci_matrix_claim.py` for the simplification
  review's four accepted findings and touched nothing else. The reader now globs every workflow file
  (`*.yml` and `*.yaml`), reads `include:`/`exclude:` entries as legs (include adds, exclude removes)
  and refuses the shapes it cannot certify; case 1 also holds the gate command as a whole `run:` line
  and every other workflow's legs inside the claim's runners; case 2 reads every workflow file; case 4's
  §CI sentence check is joined by case 5 (the platform rules' paragraph, review finding 1) and case 6
  (the reader's refusal contract). `.github/workflows/gate.yml` and `docs/harness.md` bytes did not
  move (digests in §2), the gate is unchanged, no file was added or removed, and the only count that
  moved is the suite's own (`388` → `390`, §1). The review's third live copy of the claim
  (`docs/evidence/0001-…md`) is a record and stays a snapshot.
- Nothing else moved: no gate step (`scripts/gate.py` and `scripts/gate-required.txt` are untouched),
  no carried file, no manifest, no native check, no other workflow line beyond the matrix and the
  comment above it, and no branch-protection or CI setting outside this file.
- The decision implemented here is the owner's (2026-11-18, the operator's ledger entry 34 Q3), not a
  default taken alone, so no `OPEN-QUESTIONS.md` entry accompanies it: nothing is open. Its reversal is
  the plan's — re-add a leg only together with the two files this pin holds in one statement.
- This record's own bytes: it has no §5 comparison table and makes no claim about the `parity` step's
  elision, so the suite's two record-reading cases (`test_evidence_claims.py`,
  `test_gate_transcript_claims.py`) are not exercised by it beyond discovery — stated because both read
  `docs/evidence/*.md`, and this record (edited by the apply round) is exactly what could trip them.
