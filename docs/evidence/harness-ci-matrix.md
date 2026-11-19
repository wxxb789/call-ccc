# Evidence — harness: the CI matrix is ubuntu and windows, and the platform claim says so

Plan (this increment carries no numbered goal, so the plan is its completion contract):
[`docs/plans/harness-ci-matrix.md`](../plans/harness-ci-matrix.md). Branch `chore/remove-macos-ci-leg`,
off `origin/main` (`e68f18a`); the tree is left in the worktree for operator verification — nothing was
committed, pushed or opened as a pull request.

What it changes: `.github/workflows/gate.yml` (the matrix is `os: [ubuntu-latest, windows-latest]`),
`docs/harness.md` (§CI and §Platform boundary now state what is verified — the macOS gap and the
simulations that pin the platform rules — and the `scripts/tests/` knob names the new pin),
`scripts/tests/test_ci_matrix_claim.py` (the pin: four cases in the plan's first cut, six after the
apply round, seven after the fix round — both extensions recorded in §4), and
`docs/evidence/0001-agentsview-parity-and-inspection.md` (two present-tense CI claims in its
residual-risk section). Historical rows are untouched: the `macos-latest` reds and the repairs they
forced, in the 0002/0003 records and in `docs/harness.md`'s own notes, still read as what those rounds
ran.

## 1. The gate, on the delivered tree

`python3 scripts/gate.py` → `GATE: PASS`, exit `0`, 45.8 s on this host, and re-runs on the delivered
tree — the `selftest` step discovers `docs/evidence/*.md`, so this record's own file is part of the tree
a run reads — print the same verdicts and the same `citations` counts (`36`/`37` rows), moving only the
suite's own time and case count and the tree's own file counts, which this record's file joins
(`secrets` `166` → `167`, files seen by git `5711` → `5712`). The `selftest` step is the shipped
suite: 384 cases before this increment, 388 when this record was first written (this module's four), 390
after the apply round (the pin's two extra cases; the second block below quotes that re-run), 391 as
delivered by the fix round (the pin's rewrite: one case dropped into the reader's refusals, two added;
§4). The `parity` step's detail is the gate's own rendering of the checker's stdout — `tail(proc.stdout,
800)`, its line feeds joined with ` | `, which is why the line opens on a cut. The first two blocks are
the earlier rounds' deliveries, quoted as the gate printed them; the third is the fix round's, the run
this record's counts come from.

```
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 388 tests in 9.388s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5711 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes ver…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified
  PASS     secrets  no leaks found in 166 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`scripts/gate.py` re-run after §2's mutation round was recorded — the apply round's delivered tree, exit
`0`, 44.8 s on this host — quoted as the gate printed it:

```
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 390 tests in 8.763s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 32 project-owned prefix(es); files seen by git: 5712 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes ver…
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified
  PASS     secrets  no leaks found in 167 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The fix round's run — the tree this record's rewrite was in, exit `0`, 74.4 s on this host; pasting this
block in is the record's own only later edit, and the gate re-run on the delivered tree prints the same
lines with its own timing — quoted as the gate printed it, its `parity` line whole (the first two blocks
cut that line shorter, as those rounds wrote them):

```
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3622 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 391 tests in 13.896s (rc=0)
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

The fix round rewrote the pin (§4) and re-ran, against it, every mutation the code review executed and
every mutation the earlier rounds had recorded — STD-1's two orders, STD-2's three spellings, STD-3's
three shapes, STD-4…STD-7's shapes, and the apply round's rows among them: the battery in
`/tmp/ccc-ci-matrix-fix/mutate.py` holds the 38 mutations below. It runs against a throw-away copy of
the tree (`/tmp/ccc-ci-matrix-fix/tree` holds the three files the module reads —
`.github/workflows/gate.yml`, `docs/harness.md`, `scripts/tests/test_ci_matrix_claim.py` — plus the
second workflow file two rows add; the module resolves its root from its own path, so the copy is
enough): each mutation edits one of the copy's files, the module runs once (`python3 -m unittest
scripts.tests.test_ci_matrix_claim` from the copy's root), and the copy's unmutated control run prints
`Ran 7 tests … OK` (exit `0`) while all 38 mutations below exit `1` — the count the battery's own last
line prints (`38 mutation(s), 0 that did not fail`). The shipped files' digests held before and after
the battery — `sha256sum`: `gate.yml` `260923d147…` (unchanged from the first round's reading),
`harness.md` `bc675ce2fe04…` and the pin `1aac68cb4d69…` (both moved this round, §4) — the battery
never wrote into the worktree. In the table, the failures the reader refuses are reported the way the
run reports them: the refusal is raised in the module's `setUp`, so every case errors and the run ends
`FAILED (failures=7)`; the messages are quoted to their first clause where `…` shows it, as the run
printed them around the tree's own path.

| case | mutation | observed |
| --- | --- | --- |
| `test_the_matrix_is_exactly_the_platforms_the_claim_names` | `os:` gains `macos-latest` | `FAILED (failures=1)`; `Lists differ: ['ubuntu-latest', 'macos-latest', 'windows-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `include:` entry after `os:` adds `macos-latest` | `FAILED (failures=1)`; `Lists differ: ['ubuntu-latest', 'windows-latest', 'macos-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `exclude:` entry after `os:` removes `windows-latest` | `FAILED (failures=1)`; `Lists differ: ['ubuntu-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `include:` entry **before** `os:` adds `macos-latest` (STD-1) | `FAILED (failures=1)`; `Lists differ: ['ubuntu-latest', 'windows-latest', 'macos-latest'] != ['ubuntu-latest', 'windows-latest']` — the fix round's proof the composition no longer depends on key order |
| the same case | an `exclude:` entry **before** `os:` removes `windows-latest` (STD-1) | `FAILED (failures=1)`; `Lists differ: ['ubuntu-latest'] != ['ubuntu-latest', 'windows-latest']` |
| the same case | an `other.yml` whose own matrix holds a macos leg | `FAILED (failures=1)`; `Lists differ: ['macos-latest'] != []` — `other.yml runs a matrix leg outside the runners the claim names` |
| `test_the_gate_job_runs_the_gate_file_itself` | the command commented out and replaced by `python -m gate` | `FAILED (failures=1)`; `'python scripts/gate.py' not found in ('go install …', 'python -m gate')` |
| the same case | the command spelled `python3` (a spelling the Windows runner cannot run) | `FAILED (failures=1)`; `'python scripts/gate.py' not found in ('go install …', 'python3 scripts/gate.py')` |
| the same case | `run: echo python scripts/gate.py` (the whole value, not a substring) | `FAILED (failures=1)`; `'python scripts/gate.py' not found in ('go install …', 'echo python scripts/gate.py')` |
| all seven cases (the reader's refusal, in `setUp`) | the Gate step gains `if: runner.os != 'Windows'` (STD-3) | `FAILED (failures=7)`; `a step-level `if:` ("runner.os != 'Windows'") — a step that can be skipped or ignored is not a step …` |
| the same | the Gate step gains `continue-on-error: true` (STD-3) | `FAILED (failures=7)`; `a step-level `continue-on-error:` ('true') — …` |
| the same | the Gate step's `run: >-` folds the command's text inside itself (STD-3) | `FAILED (failures=7)`; `a `run:` with a block scalar (`>-`) — the pin reads one line …` |
| the same | `runs-on: macos-latest` (a literal) | `FAILED (failures=7)`; `a `runs-on:` that is not exactly `${{ matrix.os }}` ('macos-latest') — …` |
| the same | `'runs-on': …` (a quoted key, STD-2) | `FAILED (failures=7)`; `a `runs-on` spelled in a form this pin cannot read ("'runs-on': ${{ matrix.os }}") …` |
| the same | `runs-on : …` (a space before the colon, STD-2) | `FAILED (failures=7)`; `a `runs-on` spelled in a form this pin cannot read ('runs-on : ${{ matrix.os }}') …` |
| the same | `runs-on:` with a block sequence (STD-2) | `FAILED (failures=7)`; `a `runs-on:` that is not exactly `${{ matrix.os }}` ('') — …` |
| the same | an `other.yml` whose job runs on `runs-on: macos-latest` | `FAILED (failures=7)`; the same `runs-on` refusal, naming `other.yml: line 10` |
| the same | an `other.yml` with a `uses:` job (a called workflow) | `FAILED (failures=7)`; `a `uses:` job (`call: uses: ./.github/workflows/gate.yml`) — …` |
| the same | `paths-ignore: ['docs/**']` under `pull_request:` (STD-7) | `FAILED (failures=7)`; `a `pull_request:` with a value or a block — a filter … narrows §CI's 'every pull request' …` |
| the same | a job-level `if:` (STD-7) | `FAILED (failures=7)`; `a job-level `if:` ("github.event_name == 'push'") — …` |
| the same | a `schedule:` trigger beside the accepted ones | `FAILED (failures=7)`; `a trigger this pin does not read (`schedule:`) — …` |
| the same | `os: ['ubuntu-latest, windows-latest']` (one quoted scalar, STD-6) | `FAILED (failures=7)`; `a runner label this pin cannot read ('ubuntu-latest, windows-latest') — …` |
| `test_the_claim_names_exactly_the_matrix_s_runners` | §CI gains `The `macos-14` runner also runs the gate nightly.` (STD-4) | `FAILED (failures=1)`; `Items in the first set but not the second: 'macos-14'` |
| the same case | §CI's `windows-latest` renamed `windows-2022` (STD-4) | `FAILED (failures=1)`; the set diff both ways, `'windows-2022'` / `'windows-latest'` |
| the same case | §CI's runner list names `macos-latest` | `FAILED (failures=1)`; `Items in the first set but not the second: 'macos-latest'` |
| `test_the_platform_rules_state_the_gap_and_the_runners_too` | §Platform gains a present-tense `` The `macos-latest` runner is green again since the repair. `` (STD-5) | `FAILED (failures=1)`; `§Platform boundary names ['macos-latest'] in a sentence with no history marker (…)` |
| `test_the_section_states_the_platform_the_matrix_does_not_cover` | §CI's `**macOS is not verified on a runner**` renamed | `FAILED (failures=1)`; `'macOS is not verified on a runner' not found in …` |
| `test_the_platform_rules_state_the_gap_and_the_runners_too` | §Platform's gap sentence renamed | `FAILED (failures=1)`; `§Platform boundary no longer states the gap (…)` |
| all seven cases (the reader's refusal) | a second job with its own `strategy.matrix` | `FAILED (failures=7)`; `a second job (`other:`) — a second `strategy.matrix` is a shape this pin refuses …` |
| the same | a bare `os:` outside the matrix | `FAILED (failures=7)`; `an `os:` outside the matrix — …` |
| the same | `os:` in the block form | `FAILED (failures=7)`; `an `os:` this pin cannot read ('') — the pin reads the inline list form …` |
| the same | a `uses:` job appended to `gate.yml` | `FAILED (failures=7)`; `a second job (`call:`) — …` (the appended job is refused before its `uses:` is read) |
| the same | an `include:` entry at `include:`'s own indent | `FAILED (failures=7)`; `an `include:` block with no entries — …` |
| the same | `os: []` (the list holds no label, STD-10) | `FAILED (failures=7)`; `an `os:` this pin cannot read ('[]') — the list holds no label` |
| the same | an inline `matrix:` (STD-10) | `FAILED (failures=7)`; `an inline `matrix:` ('{os: [ubuntu-latest, windows-latest]}') — …` |
| the same | an `include:` entry that is not `- os:` (STD-10) | `FAILED (failures=7)`; `an `include:` entry this pin cannot read (python: '3.12') — …` |
| the same | a `matrix:` with no `os:` | `FAILED (failures=7)`; `a `matrix:` with no `os:` — the pin reads the legs from it …` |
| the same | an inline `steps:` | `FAILED (failures=7)`; `an inline `steps:` ('[a]') — …` |

No mutation survived. The reader's refusal contract is pinned on the shipped tree per branch by
`test_the_reader_refuses_a_shape_it_cannot_certify` — 57 crafted shapes, one per refusing branch of the
walk, each's refusal message asserted (the fix round's STD-10 closure: a mutation that disables one
branch leaves that shape's named subtest red) — and the accepting branch has its own case,
`test_the_reader_reads_the_shapes_it_accepts`, over five crafted shapes (an ordered matrix with
`include:`/`exclude:` on each side of `os:`, a quoted label list, a `branches:` list holding `main`
beside another branch, and a `uses:` step with its `with:` block beside the gate step), so a mutation
that disables an accepting branch cannot pass vacuously. The rows above are the same reader driven on
the real files.

## 3. `AGENTS.md` growth: nothing moves

`AGENTS.md` is unchanged. Its one CI-adjacent sentence — "It is the same file CI runs (the runners name
the interpreter `python`)" — stays true, because both remaining runners have `python`; no sentence there
states or implies the matrix or a platform coverage claim, and the read order it fixes already points at
`docs/harness.md`, where the claim lives. This increment changes neither the repository layout, the
gate, the commands, the read order nor a rule an agent follows (the pin is a `scripts/tests/` case,
which the file's existing harness-tests bullet already covers), so per the file's growth rule that is
said here instead of editing the file for its own sake. The apply and fix rounds re-checked this against
their own changes: both live entirely inside `scripts/tests/` and the claim's own docs — cases, a
reader, a knob sentence and §CI's own paragraph — so the same bullet covers them and the conclusion is
unchanged.

## 4. What else moved, and what did not

- `docs/evidence/0001-agentsview-parity-and-inspection.md`: two sentences of its residual-risk
  section — present-tense claims about CI, not rows recording what a past round ran — moved with the
  claim ("CI runs the same gate on all three" and "CI has it on all three runners" now name
  `ubuntu-latest`/`windows-latest` and state the macOS gap). Every other evidence row and every
  disclosed limit is untouched.
- `docs/harness.md`: the `scripts/tests/` knob gained this module and its claim in the first round; the
  fix round corrected that bullet's pointer (`the CI section above` → `below`) and restated the
  contract it names, and rewrote §CI's pin paragraph to the shipped reader (structural, order-independent
  legs, whole-command step, triggers read, refusals) and added the paragraph that states the pin's two
  limits and the constraint it puts on future workflows (§5). Those are the only bytes that moved in the
  file this round — the knob bullet and §CI; `sha256sum` `2913c305a8…` → `bc675ce2fe04…` — and every
  other section, the `macos-latest` history in §Platform boundary among them, is unedited.
- `.github/workflows/gate.yml` did not move this round either: `sha256sum` `260923d147…`, the same
  digest the first round's record holds.
- The apply round (2026-11-19) extended `scripts/tests/test_ci_matrix_claim.py` for the simplification
  review's four accepted findings and touched nothing else. The reader now globs every workflow file
  (`*.yml` and `*.yaml`), reads `include:`/`exclude:` entries as legs (include adds, exclude removes)
  and refuses the shapes it cannot certify; case 1 also holds the gate command as a whole `run:` line
  and every other workflow's legs inside the claim's runners; case 2 reads every workflow file; case 4's
  §CI sentence check is joined by case 5 (the platform rules' paragraph, review finding 1) and case 6
  (the reader's refusal contract). The only count that moved is the suite's own (`388` → `390`, §1).
- The fix round (2026-11-19) rewrote the pin for the code review's findings and touched nothing else:
  the reader is now the structural walk §2's rows drive (order-independent leg composition, `runs-on`
  read at its own position and refused outside the matrix reference, triggers read, gate command held as
  a step's whole `run:` value, block scalars and skippable/ignorable steps refused), the label reading
  covers the hosted families and refuses a quoted scalar holding a comma, the §Platform paragraph's
  retired-runner allowance requires a history marker, the refusal contract is pinned per branch (§2), and
  the case inventory moved from six to seven (the no-bypass case's rule moved into the reader's own
  refusals; the command check and the accepting-shapes case are new). `docs/harness.md` moved exactly
  the two places the bullets above name; `docs/plans/harness-ci-matrix.md`'s case count and its
  "grows by four cases" sentence were updated to the shipped number (seven; §4's extensions named). The
  suite is `391` as delivered (§1). No file was added or removed.
- Nothing else moved: no gate step (`scripts/gate.py` and `scripts/gate-required.txt` are untouched),
  no carried file, no manifest, no native check, no other workflow line beyond the matrix and the
  comment above it, and no branch-protection or CI setting outside this file.
- The decision implemented here is the owner's (2026-11-18, the operator's ledger entry 34 Q3), not a
  default taken alone, so no `OPEN-QUESTIONS.md` entry accompanies it: nothing is open. Its reversal is
  the plan's — re-add a leg only together with the two files this pin holds in one statement.
- This record's own bytes: its §5 is prose — no comparison table — and it makes no claim about the
  `parity` step's elision, so the suite's two record-reading cases (`test_evidence_claims.py`,
  `test_gate_transcript_claims.py`) are not exercised by it beyond discovery — stated because both read
  `docs/evidence/*.md`, and this record is exactly what could trip them.

## 5. The pin's stated limits, and the residue left open

- **Raw text cannot see Markdown rendering.** The pin reads the section's bytes, not the rendered page:
  wrapping the §CI claim paragraph and the §Platform gap paragraph in `<!-- … -->` satisfies every case
  while the rendered document loses them — executed this round on the battery's copy (`Ran 7 tests … OK`,
  exit `0`, the same reader every other row drives). Stated in the pin's docstring and in §CI, not
  hidden.
- **The pin's own existence is not pinned.** The `selftest` step discovers `scripts/tests/test_*.py`, so
  deleting the pin — or renaming it outside the glob — leaves the gate green. Executed this round on the
  worktree: with the file moved out, `python3 -m unittest discover -s scripts/tests -p 'test_*.py'`
  printed `Ran 384 tests … OK` (exit `0`), and the restore was byte-identical (`sha256sum`
  `1aac68cb4d69…` before and after; the same result for the renamed file).
- **Why no closing case.** The mandate was to close it if the repo has a natural home, and none was
  found: `test_evidence_claims.py`'s named-file mechanism covers *evidence records* (its subject is §5
  answer claims, not the suite's own modules), `test_gate.py`'s nested runs count the suite from
  discovery but name no module, and no case's subject is the suite's own module inventory; a case
  asserting this pin's existence would live off-subject in another module, or invent a new inventory
  mechanism, rather than have a home. Recorded here, and named in §CI, instead of added as a decorative
  check.
- **The constraint the pin puts on future workflows is now stated where it binds.** Every file under
  `.github/workflows/` must keep its job on `${{ matrix.os }}` with the matrix's legs inside the runners
  §CI names, or the pin refuses the file (a second job, a literal `runs-on`, a `uses:` job or another
  matrix is refused loudly, not read past). §CI now says so; before this round that constraint lived
  only in the pin's own messages.
