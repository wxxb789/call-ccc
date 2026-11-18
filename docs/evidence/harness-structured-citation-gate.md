# Evidence — harness: a machine-checked obligation-citation manifest

Plan (this increment carries no numbered goal, so the plan is its completion contract):
[`docs/plans/harness-structured-citation-gate.md`](../plans/harness-structured-citation-gate.md).
Branch `chore/structured-obligation-citations`, off `origin/main` (`338df94`); the tree is left in the
worktree for operator verification — nothing was committed, pushed or opened as a pull request.

What it adds: `scripts/obligation-citations.py` (the checker), the **required** `citations` gate step
(`scripts/gate.py`, `scripts/gate-required.txt`), `scripts/tests/test_obligation_citations.py` (30
cases), the seed manifest `docs/evidence/0002-citations.tsv` (36 rows), and the docs that state the
schema and the step (`docs/conventions.md`, `docs/harness.md`, `AGENTS.md`). Beside those, it archives
goal 0006 (`docs/goals/archive/0006-omp-continuation.md`, merged onto `main` as `338df94`) with the links
that named it, and it changes assertions in three existing test modules: `scripts/tests/test_gate.py` (a
new step joins its expected lists), `test_evidence_claims.py` and `test_gate_transcript_claims.py` (each
was scoped by a continuation-name glob, which this increment's record falls outside). §6 names each of
those changes and why it was forced.

## 1. The gate, on the delivered bytes

`python3 scripts/gate.py` → `GATE: PASS`, exit `0` — 37.1 s on the fix pass's last run of this tree, whose
step lines are quoted below (`selftest` at `Ran 320 tests in 7.553s`), 69.7 s on its first run of the
fixed code (`Ran 320` there too), and 36.8 s and 39.6 s on the increment's pre-fix bytes. The `selftest`
step's own time moves between runs (`7.4s` on the applied suite's own run; `7.742s`, `7.988s`, `8.210s`,
`8.290s` and `7.553s` on five runs here, minutes apart) — the count and the exit status are the stable
part:

```
gate: <repo>
gate: go=<go 1.27.4> targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3611 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 320 tests in 7.553s (rc=0)
  PASS     parity   <the line quoted as fragments below>
  PASS     citations docs/evidence/0002-citations.tsv: 36 row(s) verified
  PASS     secrets  no leaks found in 148 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The `secrets` step's count is the files this repository owns — every file under an owned prefix of the
carry manifest, ignored scratch included, because that is what the step stages for the scan. This
worktree held ten `scripts/tests/__pycache__` entries when the run above printed `148`; the same tree
with that scratch removed counts `138`, and a re-run prints `138` plus however many ignored files an
owned prefix happens to hold. The number is a scope statement, not a claim about tracked files.

The `parity` step's line is the gate's own elision — the gate's 800-character rendering of the checker's
stdout (`tail(proc.stdout, 800)` with ` | ` where line feeds stood), so its leading `…` is the elision's
cut and its end is the checker's own — and it is quoted here as the fragments it holds rather than as one
line:
`reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584`, `carried: 5553 file(s) identical to the
pinned revision (compared through git's own object store)`, `deviations: 27 upstream path(s) declared (1
of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared`,
`owned: 31 project-owned prefix(es); files seen by git: 5693`. The pin is unchanged and the carried half
is untouched. (The increment's own runs counted 5694: the one file this branch's goal move adds was
untracked until the rename was staged — the number is this worktree's own answer, not a constant.)

The `citations` step on its own, same bytes — `python3 scripts/obligation-citations.py`:

```
docs/evidence/0002-citations.tsv: 36 row(s) verified
citations: 36 row(s) across 1 file(s) verified
citations: PASS
```

exit `0`; 4.6 s with the packages already compiled (4.2 s on the fix pass's own re-run) — that compile
is what the gate's own `test` step above has just paid for. That is two `go test` invocations — one per
cited package (`./cmd/ccc`, `./internal/ccc`), never one per row — and each cited test is required to
appear as `--- PASS: <test>` in its package's output, which is what `36 row(s) verified` reports.

## 2. The checker's FAIL path, on a planted broken row

One row's test name was renamed in the delivered manifest (`A1`'s `TestReadReportsAForkedPathAsAmbiguous`
→ `TestReadReportsAForkedPathAsAmbiguousRenamed`), then the checker and the whole gate were run, then the
manifest was restored byte-identically (sha256 `3a5fd4b5…` before and after; `sha256sum -c` OK). The fix
pass re-ran this scenario on the shipped bytes after the manifest's header comment changed; the comment
edit kept the planted row on line 24, so the answer below is re-observed on the shipped digest named
above, and the violation is quoted as the single line the checker prints.

```
$ python3 scripts/obligation-citations.py
docs/evidence/0002-citations.tsv: 0 row(s) verified
  docs/evidence/0002-citations.tsv:24: rule A1: test TestReadReportsAForkedPathAsAmbiguousRenamed is not declared by any _test.go in ./internal/ccc
citations: 0 row(s) across 1 file(s) verified
citations: FAIL — 1 violation(s)
$ echo $?
1

$ python3 scripts/gate.py            # the same planted row, through the required step
  FAIL     citations REQUIRED but was FAIL: docs/evidence/0002-citations.tsv: 0 row(s) verified
  docs/evidence/0002-citations.tsv:24: rule A1: test TestReadReportsAForkedPathAsAmbiguousRenamed is not declared by any _test.go in ./internal/ccc
citations: 0 row(s) across 1 file(s) verified
citations: FAIL — 1 violation(s)
GATE: FAIL — citations
```

The failure names the file and the line, and the step's own name is the reason the gate refuses the tree.

## 3. Each selftest case fails when its rule is broken

`scripts/tests/test_obligation_citations.py` pins every rule of the plan's §3, its boundaries and the
gate wiring. Each case was shown to fail on its rule broken by a temporary pattern-addressed edit to the
shipped checker, the shipped gate or (for the record's own row) the record, run once with the named
case, and restored afterwards from a copy taken before the edit — every restore compared byte-for-byte
(`sha256sum -c`). The rows down to `test_the_gate_rewrites_a_skipped_citations_step_when_it_is_required`
were recorded against the increment's pre-apply checker (`0c7cd18e…`) and gate (`1472fb51…`); the eight
rows after them were recorded by the apply pass on the applied checker (`49e88616…`) and the unchanged
gate — they are the simplify report's findings 6–11 and the two discovery-guard rows, and the four
earlier rows whose mutation anchor moved with the delegation (the three `batches` rows and the `go_flags`
row) were re-run there too rather than left as text about code that moved. The first fix pass added five
rows — its `F4` and `F5` cover — recorded on the checker of that pass (`1f6f6a71…`), whose parse gained
the no-data-rows violation. The second fix pass (the machine review of `e4a23c546`'s two findings) then
changed the run accounting (`accounted`, the batch rejection, the FAIL/SKIP precedence) and the run's own
budget, in the checker, and the step's budget in the gate; its seven rows are recorded on the shipped
checker (`9592a02d…`) and gate (`5032a3bb…`), and it re-ran every earlier row whose anchor or observation
its edit moved — the positive control, the three `batches` rows, the `--- PASS`-is-required row (whose
old anchor no longer exists), and the rc row (same anchor, a new observation) — so every mutation target
the rows above name is still a line of the shipped checker or gate, and their observations are about code
this record ships. Every command below is `python3 -m unittest <module>.<Case>.<case>` —
`scripts.tests.test_obligation_citations` unless the row names another module — and exited `1`.

| case | rule it pins | mutation | observed |
| --- | --- | --- | --- |
| `…CitationsCase.test_a_well_formed_manifest_verifies_every_row` | the test runs (positive control) | checker: `for group in gate.gofmt_batches(names):` → `for group in []:` | `AssertionError: 0 != 1` — no invocation recorded while the verdict still read PASS |
| the same case | the printed per-file line | checker: `print(f"{name}: {verified.get(name, 0)} row(s) verified")` → `print(f"{name}: verified")` | `AssertionError: Lists differ: [] != ['docs/evidence/0002-citations.tsv: 2 row(s) verified']` |
| the same case (its second half) | one test cited twice is requested once and verifies both rows | checker: `row.test not in names` dropped from the per-package name list | `AssertionError: '^(TestAlpha\|TestAlpha)$' != '^(TestAlpha)$'` |
| `test_a_header_that_is_not_the_four_columns_fails` | the header names exactly the four columns, in order | checker: the header comparison → `if False:` | `AssertionError: 0 != 1` for the missing and the renamed header (both verified), the reordered one beside them |
| `test_a_row_that_is_not_four_non_empty_cells_fails` | four *non-empty* cells | checker: the `any(not cell …)` half removed | `AssertionError: '…:4: a row must hold exactly four non-empty cells' not found` (the empty-cell row was accepted) |
| the same case | exactly four cells | checker: `len(cells) != len(COLUMNS)` → `len(cells) > len(COLUMNS)` | `ValueError: not enough values to unpack (expected 4, got 3)` on the missing-cell row; the empty-cell row fails beside it |
| `test_a_rule_cited_twice_in_one_file_fails` | no repeated rule id in a file | checker: the duplicate check → `if False:` | `AssertionError: 0 != 1 : …: 2 row(s) verified` (both rows verified) |
| `test_a_cited_test_no_test_file_declares_fails_before_any_tool_runs` | the package's `_test.go` declares it | checker: the declaration check → `if False:` | `AssertionError: 0 != 1` (the row verified) |
| `test_a_name_go_test_would_not_run_fails_before_any_tool_runs` | a legal Go test name | checker: the identifier check → `if False:` | `AssertionError: 0 != 1` for `Testalpha`, `Tests` and `Test-Alpha` |
| `test_a_package_the_gate_does_not_test_fails_before_any_tool_runs` | coverage from `gate-targets.txt`, one package per row | checker: `problem = package_problem(…)` → `problem = None` | `AssertionError: 'not covered by scripts/gate-targets.txt' not found`, the same for `must name one package`, and the narrowed-knob half beside them |
| the same case | no tool runs before a static verdict | checker: the static short-circuit → `if violations and False:` | `AssertionError: Lists differ: [['go', 'test', '-count=1', …]] != []` (an invocation was recorded) |
| `test_a_cited_test_that_does_not_pass_fails` | `--- PASS` is required; `--- FAIL`/`--- SKIP`/none are failures, and an explicit FAIL/SKIP precedes a PASS for the same name | checker: `if name in others:` → `if name in others and False:` | `AssertionError: 'rule A1: … go test reported `--- FAIL: TestAlpha`' not found in '…did not pass — the `go test` run for it exited rc=1…'` (the FAIL/SKIP arm removed, the rc fallback answered) |
| `test_no_manifest_anywhere_is_a_skip_naming_where_it_looked` | no manifest → SKIP | checker: the `if not found:` branch → `if False:` | `AssertionError: 0 != 2 : citations: 0 row(s) across 0 file(s) verified` (an empty run read as a pass) |
| `test_the_manifests_are_discovered_and_printed_in_sorted_order` | discovery order is the file-name order | checker: `sorted(…, key=name)` → `reverse=True` | `AssertionError: Lists differ: ['…b-second…', '…a-first…'] != ['…a-first…', '…b-second…']` |
| `test_one_invocation_per_package_hands_every_cited_test_to_it` | one invocation per package, the knobs' flags | checker: `for group in gate.gofmt_batches(names):` → `for group in [[name] for name in names]:` | `AssertionError: Lists differ:` one three-row invocation `!=` three one-test invocations |
| the same case | the flags come from `gate-goflags.txt` | gate: `go_flags()` → `return []` | `AssertionError: Lists differ:` the argv holds no `-tags fts5` |
| `test_the_run_is_split_only_when_the_alternation_would_outgrow_the_bound` | the command line stays inside the bound | checker: `for group in gate.gofmt_batches(names):` → `for group in [names]:` | `AssertionError: 1 not greater than 1 : …: 300 row(s) verified` (one invocation for 300 names) |
| `test_the_missing_go_toolchain_is_a_failure_and_the_windows_answer_is_the_boundary` | no `go` is a failure | checker: the missing-tool branch → `if False:` | `AssertionError: 0 != 1` (the run proceeded and verified) |
| the same case | the host boundary is the gate's `go_env()` answer | checker: the boundary branch → `if boundary and False:` | `AssertionError: 1 != 3` (FAIL where the boundary answers BOUNDARY) |
| `…GateWiringCase.test_the_gate_step_reads_the_verdict_line_the_checker_printed` | the status comes from the verdict line | gate: `status = verdicts[0]…` → `status = PASS` | `AssertionError: 'FAIL' != 'SKIP'` and `'FAIL' != 'BOUNDARY'` |
| `test_the_gate_discloses_a_skipped_citations_step_in_a_partial_verdict` | an unrequired SKIP stays a disclosed partial run | gate: the `status in (SKIP, BOUNDARY)` branch → `if False:` | `AssertionError: 1 != 0` (the gate refused a tree it should have disclosed) |
| `test_the_gate_rewrites_a_skipped_citations_step_when_it_is_required` | a required step that skipped fails the gate | gate: `apply_required`'s rewrite → `if False:` | `AssertionError: 0 != 1` (a tree with no manifest passed) |
| `…CitationsCase.test_a_run_that_exited_without_a_verdict_is_reported_by_its_rc` | a run that exited without a verdict is reported by its rc, after the per-test lines | checker: `elif proc.returncode != 0:` → `elif False:` | `AssertionError: '…the `go test` run for it exited rc=1' not found in '…exited rc=1 while none of the tests it was asked for reported `--- FAIL` or `--- SKIP`…go test printed no `--- PASS:` line for it…'` (the rc arm removed; the batch rejection and the fallback answered) |
| `…CitationsCase.test_a_run_that_outran_its_deadline_is_a_result_not_a_crash` | a stalled `go test` is `rc=124`, not an exception | checker: `except subprocess.TimeoutExpired as expired:` → `except ZeroDivisionError:` | `subprocess.TimeoutExpired: Command '[…]' timed out after 0.2 seconds` (the arm removed, the platform's exception reached the case) |
| `…CitationsCase.test_the_printed_violations_are_bounded_and_the_verdict_counts_them_all` | the printed violations stop at `VIOLATIONS_PRINTED`; the verdict counts every one | checker: `violations[:VIOLATIONS_PRINTED]` → `violations` | `AssertionError: 41 != 40` (41 violation lines printed) |
| `…CitationsCase.test_a_tree_whose_gate_cannot_be_loaded_fails_naming_it` | a tree without `scripts/gate.py` is a FAIL naming the sibling | checker: the `try`/`except` around `gate_module()` → the bare call | `FileNotFoundError: [Errno 2] No such file or directory: '…/scripts/gate.py'` |
| `…CitationsCase.test_a_name_longer_than_the_bound_is_a_batch_of_its_own_rather_than_dropped` | a name longer than the bound is a batch of its own, never a dropped one | gate: `batch.append(path)` → guarded by `if batch or len(path) <= limit:` | `AssertionError: 1 != 2` (both names went over in one invocation) |
| `…GateWiringCase.test_the_gate_step_reads_the_verdict_line_the_checker_printed` | the PASS detail is the per-file lines, the `citations: `-prefixed ones filtered out | gate: the `not line.startswith("citations: ")` filter dropped | `AssertionError: '…verified \| citations: 1 row(s) across 1 file(s) verified \| citations: PASS' != '…verified'` |
| `…GateTranscriptElisionClaims.test_every_rule_record_quotes_the_gate_s_own_rendering` | the harness increment's record states the rule and is read by this case | record: the phrase "the gate's own elision" dropped from §1 | `AssertionError: 'harness-structured-citation-gate.md' not found in {'0004-repeatable-continuation-handoffs.md'}` |
| `…ComparisonCreditsTheRecordedRun.test_every_shipped_comparison_only_quotes_its_recorded_run` | the harness increment's record is discovered by the widened `*.md` glob | test: `glob("*.md")` → `glob("*continuation*.md")` | `AssertionError: 'harness-structured-citation-gate.md' not found in {'0002-claude-to-codex-continuation.md', …}` |
| `…CitationsCase.test_a_manifest_whose_header_declares_no_rows_fails` | a discovered manifest that declares no data rows is a failure | checker: `elif not rows:` → `elif False:` | `AssertionError: 0 != 1 : docs/evidence/0002-citations.tsv: 0 row(s) verified` (the header-only manifest read as a pass) |
| `…CitationsCase.test_a_targets_file_naming_one_package_covers_exactly_that_package` | a pattern without `...` is exactly that package | checker: `elif pattern == package:` → `elif package.startswith(pattern):` | `AssertionError: 'package ./internal/cccx is not covered by scripts/gate-targets.txt' not found in '…rule A1: test TestAlpha is not declared by any _test.go in ./internal/cccx…'` (the prefix arm covered the neighbouring package) |
| `…CitationsCase.test_a_tree_without_the_targets_file_falls_back_to_the_whole_module` | no `gate-targets.txt` falls back to `./...`, the Go steps' own default | checker: `patterns = gate.read_list("gate-targets.txt") or ["./..."]` → `patterns = gate.read_list("gate-targets.txt")` | `AssertionError: 1 != 0 : docs/evidence/0002-citations.tsv: 0 row(s) verified` (the empty list refused the package) |
| `…CitationsCase.test_a_manifest_that_cannot_be_read_fails_naming_it` | an unreadable manifest is a FAIL naming the file | checker: `return [], [f"{name}: could not be read: {exc}"]` → `return [], []` | `AssertionError: 0 != 1 : docs/evidence/0002-citations.tsv: 0 row(s) verified` (the unreadable file read as a pass) |
| `…CitationsCase.test_a_manifest_of_comments_with_no_header_line_fails` | a file that never reaches a header is a FAIL naming the file | checker: the post-loop `if header is None:` → `if False:` | `AssertionError: 'docs/evidence/0002-citations.tsv: no header line' not found in '…the header declares no data rows…'` (the no-header violation dropped, the no-rows answer reached instead) |
| `…CitationsCase.test_a_batch_that_exited_nonzero_rejects_itself_however_its_markers_read` | a nonzero exit no reported outcome accounts for is a violation of the batch's own | checker: `if not accounted:` → `if False:` | `AssertionError: "the `go test` run for ./internal/ccc exited rc=1 while none of the tests it was asked for reported `--- FAIL` or `--- SKIP`: …" not found in '…rule A1: … did not pass — the `go test` run for it exited rc=1…rule B2: …'` (the rejection removed; its rows still failed through the rc fallback, which is what this row shows the rejection does not depend on) |
| the same case | none of that run's markers verifies a row | checker: `elif name in passed and accounted:` → `elif name in passed:` | `AssertionError: Lists differ: ['docs/evidence/0002-citations.tsv: 2 row(s) verified'] != ['docs/evidence/0002-citations.tsv: 0 row(s) verified']` |
| `…CitationsCase.test_an_explicit_fail_or_skip_overrides_a_pass_for_the_same_test` | an explicit FAIL/SKIP precedes a PASS for the same name | checker: `if name in others:` → `if name in others and False:` | `AssertionError: 0 != 1 : docs/evidence/0002-citations.tsv: 1 row(s) verified / citations: PASS` — both the `FAIL` and the `SKIP` subtest |
| `…CitationsCase.test_the_run_is_bounded_in_total_and_a_spent_bound_fails_the_batches_after_it` | a batch the total can no longer afford is not started | checker: `if remaining <= 0:` → `if False:` | `AssertionError: 0 != 1 : docs/evidence/0002-citations.tsv: 2 row(s) verified / citations: PASS` (the spent bound ran the batch anyway, on a one-second slice) |
| the same case | each batch's deadline is the smaller of GO_TIMEOUT and the time left of TOTAL_TIMEOUT | checker: `timeout=max(1, int(min(GO_TIMEOUT, remaining))))` → `timeout=GO_TIMEOUT)` | `AssertionError: Lists differ: [1800, 1800] != [1800, 1700]` |
| `…GateWiringCase.test_a_checker_outliving_the_step_s_own_budget_is_a_failure_never_a_crash` | the step's runner deadline is caught and answered FAIL | gate: the `try`/`except subprocess.TimeoutExpired` → the bare `proc = run(…)` line | `subprocess.TimeoutExpired: Command '[…]' timed out after 3900 seconds` (the arm removed, the platform's exception reached the case) |
| the same case | the step's budget sits past the checker's own total bound | gate: `CITATIONS_TIMEOUT = 3600 + 300` → `CITATIONS_TIMEOUT = 1800` | `AssertionError: 1800 not greater than 3600 : the step's budget must sit past the checker's total bound…` |

No mutation survived; every restore was compared byte-for-byte, and the digest each row's restore was
checked against is the digest of the file that row's pass ran on: the pre-apply rows on the checker
`0c7cd18e…` and the gate `1472fb51…`, the apply pass's eight on `49e88616…` (the unchanged gate), the
first fix pass's five on `1f6f6a71…` (and §2's plant against the manifest's `3a5fd4b5…`, untouched by
either fix pass), and the second fix pass's rows on the shipped checker's `9592a02d…` and gate's
`5032a3bb…`. The case module was re-run whole on the restored bytes (`Ran 30 tests … OK`, §1's
`selftest` step counting the same suite).

## 4. The seed manifest

`docs/evidence/0002-citations.tsv` holds 36 rows — one per rule id of
[`0002-obligation-matrix.md`](0002-obligation-matrix.md) that a gate-runnable Go test discharges (the
matrix has 40 rule ids; the four left out are named in the file's own header comment with their reason):
all four are re-derived by the gate through the `selftest` step — `D4`/`F1` through the python cases
beside the non-gate native check (`ProtectedArtifactsAfterNativeVerification`, `SeededHostConfiguration`
in `scripts/tests/test_codex_native_check.py`), `F4`/`F5` through the suite's own cases — and it is the
*citation form* that cannot hold them: one Go test function in a package `scripts/gate-targets.txt`
covers. The decision is recorded as `Q-0002-28`.

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
and the Go targets and flags are the bytes they were. Three existing test modules' assertions did change,
each because this increment forces it — named here rather than folded into the sentence above, because
this record is the audit surface for exactly this question:

- `scripts/tests/test_gate.py` — its expected step and status lists now include `citations` (the
  unrequired-steps case's status list, its partial-steps list, and the gofmt-only-tree probe's partial
  list). Unavoidable: what the increment adds is a *step*, and a list still expecting seven steps goes
  red the moment the eighth exists.
- `scripts/tests/test_evidence_claims.py` — the record glob widened from `*continuation*.md` to `*.md`,
  plus a check that this increment's record is discovered. Its subject was bounded by a filename glob,
  and this increment's record is the first whose name falls outside it: the glob had to widen, or the
  rule would retire for the new record quietly.
- `scripts/tests/test_gate_transcript_claims.py` — the same widened glob, and the requirement that both
  the record the round-4 finding was about and this increment's record state the elision rule.

One bookkeeping item outside that list was due in this increment too: goal 0006 was merged onto `main` as
`338df94` (this branch's base), so its file moved to `docs/goals/archive/` with its Status and the README
row set to `Accepted: merged onto \`main\` as \`338df94\``, and the `Contract:` links in
`docs/plans/0006-omp-continuation.md` and `docs/evidence/0006-omp-continuation.md` moved with it — the
pair `338df94` itself rewrote for 0005.

Every other evidence row and disclosed limit is untouched, and the `selftest` step's 290 cases from before
this increment all still pass, as part of the 316 in §1.
