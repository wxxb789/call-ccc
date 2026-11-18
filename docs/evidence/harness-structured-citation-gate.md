# Evidence — harness: a machine-checked obligation-citation manifest

Plan (this increment carries no numbered goal, so the plan is its completion contract):
[`docs/plans/harness-structured-citation-gate.md`](../plans/harness-structured-citation-gate.md).
Branch `chore/structured-obligation-citations`, off `origin/main` (`338df94`); the tree is left in the
worktree for operator verification — nothing was committed, pushed or opened as a pull request.

What it adds: `scripts/obligation-citations.py` (the checker), the **required** `citations` gate step
(`scripts/gate.py`, `scripts/gate-required.txt`), `scripts/tests/test_obligation_citations.py` (16
cases), the seed manifest `docs/evidence/0002-citations.tsv` (36 rows), and the docs that state the
schema and the step (`docs/conventions.md`, `docs/harness.md`, `AGENTS.md`).

## 1. The gate, on the delivered bytes

`python3 scripts/gate.py` → `GATE: PASS`, exit `0` (35.1 s; the `selftest` step's own time moves between
runs — `6.801s` here and `7.526s` on the same tree a minute earlier — the count and the exit status are
the stable part):

```
gate: <repo>
gate: go=<go 1.27.4> targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 306 tests in 6.801s (rc=0)
  PASS     parity   <the line quoted as fragments below>
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified
  PASS     secrets  no leaks found in 147 file(s) this repository owns (the carried upstream tree is
                    covered by the parity step, not by this scan)
GATE: PASS
```

The `parity` step's line is the gate's own 800-character rendering of the checker's stdout (its leading
`…` is that rendering's cut), so it is quoted here as the fragments it holds rather than as one line:
`reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584`, `carried: 5553 file(s) identical to the
pinned revision (compared through git's own object store)`, `deviations: 27 upstream path(s) declared (1
of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared`,
`owned: 31 project-owned prefix(es); files seen by git: 5693`. The pin is unchanged and the carried half
is untouched.

The `citations` step on its own, same bytes — `python3 scripts/obligation-citations.py`:

```
docs/evidence/0002-citations.tsv: 36 row(s) verified
citations: 36 row(s) across 1 file(s) verified
citations: PASS
```

exit `0`; 4.5 s with the packages already compiled (38.4 s with a cold build cache). That is two `go
test` invocations — one per cited package (`./cmd/ccc`, `./internal/ccc`), never one per row — and each
cited test is required to appear as `--- PASS: <test>` in its package's output, which is what
`36 row(s) verified` reports.

## 2. The checker's FAIL path, on a planted broken row

One row's test name was renamed in the delivered manifest (`A1`'s `TestReadReportsAForkedPathAsAmbiguous`
→ `TestReadReportsAForkedPathAsAmbiguousRenamed`), then the checker and the whole gate were run, then the
manifest was restored byte-identically (sha256 `c26ab5fc…` before and after; `sha256sum -c` OK).

```
$ python3 scripts/obligation-citations.py
docs/evidence/0002-citations.tsv: 0 row(s) verified
  docs/evidence/0002-citations.tsv:24: rule A1: test TestReadReportsAForkedPathAsAmbiguousRenamed is not
  declared by any _test.go in ./internal/ccc
citations: 0 row(s) across 1 file(s) verified
citations: FAIL — 1 violation(s)
$ echo $?
1

$ python3 scripts/gate.py            # the same planted row, through the required step
  FAIL     citations REQUIRED but was FAIL: docs/evidence/0002-citations.tsv: 0 row(s) verified
  docs/evidence/0002-citations.tsv:24: rule A1: test TestReadReportsAForkedPathAsAmbiguousRenamed is not
  declared by any _test.go in ./internal/ccc
citations: 0 row(s) across 1 file(s) verified
citations: FAIL — 1 violation(s)
GATE: FAIL — citations
```

The failure names the file and the line, and the step's own name is the reason the gate refuses the tree.

## 3. Each selftest case fails when its rule is broken

`scripts/tests/test_obligation_citations.py` pins every rule of the plan's §3, its boundaries and the
gate wiring. Each case was shown to fail on its rule broken by a temporary pattern-addressed edit to the
shipped checker or the shipped gate, run once with the named case, and restored afterwards from a copy
taken before the edit — every restore compared byte-for-byte (`sha256sum -c`; checker `0c7cd18e…`, gate
`1472fb51…`, both unmodified at the end of the run). Every command below is
`python3 -m unittest scripts.tests.test_obligation_citations.<Case>.<case>` and exited `1`.

| case | rule it pins | mutation | observed |
| --- | --- | --- | --- |
| `…CitationsCase.test_a_well_formed_manifest_verifies_every_row` | the test runs (positive control) | checker: `for group in batches(names):` → `for group in []:` | `AssertionError: 0 != 1` — no invocation recorded while the verdict still read PASS |
| the same case | the printed per-file line | checker: `print(f"{name}: {n} row(s) verified")` → `print(f"{name}: verified")` | `AssertionError: Lists differ: [] != ['docs/evidence/0002-citations.tsv: 2 row(s) verified']` |
| the same case (its second half) | one test cited twice is requested once and verifies both rows | checker: `row.test not in names` dropped from the per-package name list | `AssertionError: '^(TestAlpha\|TestAlpha)$' != '^(TestAlpha)$'` |
| `test_a_header_that_is_not_the_four_columns_fails` | the header names exactly the four columns, in order | checker: the header comparison → `if False:` | `AssertionError: 0 != 1` for the missing and the renamed header (both verified), the reordered one beside them |
| `test_a_row_that_is_not_four_non_empty_cells_fails` | four *non-empty* cells | checker: the `any(not cell …)` half removed | `AssertionError: '…:4: a row must hold exactly four non-empty cells' not found` (the empty-cell row was accepted) |
| the same case | exactly four cells | checker: `len(cells) != len(COLUMNS)` → `len(cells) > len(COLUMNS)` | `ValueError: not enough values to unpack (expected 4, got 3)` on the missing-cell row; the empty-cell row fails beside it |
| `test_a_rule_cited_twice_in_one_file_fails` | no repeated rule id in a file | checker: the duplicate check → `if False:` | `AssertionError: 0 != 1 : …: 2 row(s) verified` (both rows verified) |
| `test_a_cited_test_no_test_file_declares_fails_before_any_tool_runs` | the package's `_test.go` declares it | checker: the declaration check → `if False:` | `AssertionError: 0 != 1` (the row verified) |
| `test_a_name_go_test_would_not_run_fails_before_any_tool_runs` | a legal Go test name | checker: the identifier check → `if False:` | `AssertionError: 0 != 1` for `Testalpha`, `Tests` and `Test-Alpha` |
| `test_a_package_the_gate_does_not_test_fails_before_any_tool_runs` | coverage from `gate-targets.txt`, one package per row | checker: `problem = package_problem(…)` → `problem = None` | `AssertionError: 'not covered by scripts/gate-targets.txt' not found`, the same for `must name one package`, and the narrowed-knob half beside them |
| the same case | no tool runs before a static verdict | checker: the static short-circuit → `if violations and False:` | `AssertionError: Lists differ: [['go', 'test', '-count=1', …]] != []` (an invocation was recorded) |
| `test_a_cited_test_that_does_not_pass_fails` | `--- PASS` is required; `--- FAIL`/`--- SKIP`/none are failures | checker: `if name in passed:` → `if True:` | `AssertionError: 0 != 1 : …: 2 row(s) verified` (the failing run was reported as a pass) |
| `test_no_manifest_anywhere_is_a_skip_naming_where_it_looked` | no manifest → SKIP | checker: the `if not found:` branch → `if False:` | `AssertionError: 0 != 2 : citations: 0 row(s) across 0 file(s) verified` (an empty run read as a pass) |
| `test_the_manifests_are_discovered_and_printed_in_sorted_order` | discovery order is the file-name order | checker: `sorted(…, key=name)` → `reverse=True` | `AssertionError: Lists differ: ['…b-second…', '…a-first…'] != ['…a-first…', '…b-second…']` |
| `test_one_invocation_per_package_hands_every_cited_test_to_it` | one invocation per package, the knobs' flags | checker: `for group in batches(names):` → `for group in [[name] for name in names]:` | `AssertionError: Lists differ:` one three-row invocation `!=` three one-test invocations |
| the same case | the flags come from `gate-goflags.txt` | checker: `go_flags()` → `return []` | `AssertionError: Lists differ:` the argv holds no `-tags fts5` |
| `test_the_run_is_split_only_when_the_alternation_would_outgrow_the_bound` | the command line stays inside the bound | checker: `for group in batches(names):` → `for group in [names]:` | `AssertionError: 1 not greater than 1 : …: 300 row(s) verified` (one invocation for 300 names) |
| `test_the_missing_go_toolchain_is_a_failure_and_the_windows_answer_is_the_boundary` | no `go` is a failure | checker: the missing-tool branch → `if False:` | `AssertionError: 0 != 1` (the run proceeded and verified) |
| the same case | the host boundary is the gate's `go_env()` answer | checker: the boundary branch → `if boundary and False:` | `AssertionError: 1 != 3` (FAIL where the boundary answers BOUNDARY) |
| `…GateWiringCase.test_the_gate_step_reads_the_verdict_line_the_checker_printed` | the status comes from the verdict line | gate: `status = verdicts[0]…` → `status = PASS` | `AssertionError: 'FAIL' != 'SKIP'` and `'FAIL' != 'BOUNDARY'` |
| `test_the_gate_discloses_a_skipped_citations_step_in_a_partial_verdict` | an unrequired SKIP stays a disclosed partial run | gate: the `status in (SKIP, BOUNDARY)` branch → `if False:` | `AssertionError: 1 != 0` (the gate refused a tree it should have disclosed) |
| `test_the_gate_rewrites_a_skipped_citations_step_when_it_is_required` | a required step that skipped fails the gate | gate: `apply_required`'s rewrite → `if False:` | `AssertionError: 0 != 1` (a tree with no manifest passed) |

No mutation survived; the digest check after every restore is the one stated above, and the case module
was re-run whole on the restored bytes (`Ran 16 tests … OK`).

## 4. The seed manifest

`docs/evidence/0002-citations.tsv` holds 36 rows — one per rule id of
[`0002-obligation-matrix.md`](0002-obligation-matrix.md) that a gate-runnable Go test discharges (the
matrix has 40 rule ids; the four left out are named in the file's own header comment with their
reasons): `D4` and `F1`, whose evidence is the non-gate native check, and `F4` and `F5`, which are rules
of the harness itself, discharged by the python suite the `selftest` step runs — the citation form is one
Go test function in a package `scripts/gate-targets.txt` covers, so those two cannot be held by a row.
The decision is recorded as `Q-0002-28`.

Every one of the 36 rows was verified by the run in §1: each cited test exists in the package the row
names and appeared as `--- PASS: <test>` in that package's own `go test -count=1 -v` output. A row cites
its matrix section's primary pin (the first `Pinned by` that section lists); the section lists the row's
other pins and their executed mutations, so the row is traceable back to its prose, and the `artifact`
column names the section.

## 5. `AGENTS.md` growth (asked for by the plan's §2.5, per that file's own growth rule)

`AGENTS.md` changed, because this increment changes the gate and a rule an agent follows:

- its record sentence now says that every obligation a gate-runnable Go test discharges also gets a row
  in that increment's `<goal>-citations.tsv`, which the required `citations` step runs;
- the Gate bullet now lists `citations` among the required steps and states what it verifies.

`docs/harness.md` gained the step's table row and its own section (the checker's contract, the printed
lines, the exit codes, both boundaries, and the measured cost); `docs/conventions.md` gained the schema
and *which* obligations get a row, plus the `docs/evidence/` line of its layout block.

## 6. The plan's §7 list, and the one record this increment had to add to it

§7 says nothing in `parity`, the Go steps, the carried tree, `upstream-manifest.txt`, the workflow files
or any existing assertion / evidence row / disclosed limit is touched, and that `parity` must still
report the carried tree identical to its pin — which it does (§1). Two files on that list needed a
one-line bookkeeping change for a file this repository adds, following the mechanism and the precedent of
every earlier increment that added a script (0006 added the same pair for `scripts/omp-native-check.py`):

- `scripts/upstream-manifest.txt`: `owned	scripts/obligation-citations.py` — without it `parity` fails
  with `1 file(s) are neither carried nor project-owned` (measured: that is what the step printed before
  the line was added), because the manifest is also where the repository records which paths are its
  own. The **carried** records, the pin and the deviations are untouched.
- `scripts/upstream-import.py`: the same path added to its `OWNED` tuple, which
  `test_upstream_parity.ImportCase.test_the_owned_list_and_the_manifest_agree` requires to agree with the
  manifest's `owned` records.

Nothing else on §7's list changed: `scripts/upstream-parity.py`, the workflow files, every carried file,
the Go targets and flags, and every existing assertion and evidence row are the bytes they were (the
`selftest` step's 290 cases before this increment all still pass, as part of the 306 in §1).
