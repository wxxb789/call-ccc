# Plan — harness: a machine-checked obligation-citation manifest

Status: **harness increment** (no numbered goal). Owner decision 2026-11-18 (option 1 of the
operator's status message); the mechanism decisions 27–28 filed as "the next harness change".

Branch `chore/structured-obligation-citations`, off `origin/main` (`338df94`). One goal ≈ one branch ≈
one PR; squash-merged onto a linear `main`. The operator owns the merge.

## 1. The problem this replaces

A goal's acceptance surface is a **prose** obligation matrix — `docs/evidence/0002-obligation-matrix.md`
and `docs/evidence/0003-obligation-matrix.md`, or an obligation section inside a goal's own evidence
record. An LLM acceptance audit reads it round after round and returns the same class of finding: a
claim stronger than the evidence it cites, each landing inside the previous repair's own prose. Goal
0002's audits returned 4·2·3·1·1·1·1; goal 0007's review rounds are 8·7·4·0·2·0·2·0·3·2·2·2·2 — flat at
2–3 from round 9. The cross-family consult of decision 27 diagnosed the **mechanism**, not the
increment: *prose audited by an LLM does not converge*. Its replacement: a structured mapping
`rule id → test name + executed artifact`, checked by a script.

## 2. What this increment adds

1. `docs/evidence/<goal>-citations.tsv` — one row per obligation that a gate-runnable test discharges.
   Tab-separated, a header row, `#` comment lines and blank lines ignored, columns exactly:
   `rule_id`, `test`, `package`, `artifact`.
2. `scripts/obligation-citations.py` — the checker (§3).
3. A gate step `citations` in `scripts/gate.py`, and `citations` in `scripts/gate-required.txt` (§4).
4. `scripts/tests/test_obligation_citations.py` — one case per rule, each failing when its rule breaks (§5).
5. Docs: `docs/harness.md` (the step's contract and boundaries), `docs/conventions.md` (the TSV schema
   and *which* obligations get a row), `AGENTS.md` (the gate paragraph, per its own growth rule).
6. A real seed manifest `docs/evidence/0002-citations.tsv` (§6) — the mechanism is proven on real data,
   not only on synthetic cases.

## 3. The checker — completion contract

`scripts/obligation-citations.py`, exit 0 only when every discovered manifest parses **and** every row
verifies:

- **Discovery**: `docs/evidence/*-citations.tsv`, sorted (the order is pinned and printed).
- **Parse, per file** (a violation is FAIL naming file and line): the first non-comment line is the
  header and names exactly the four columns in the order above; every data row has exactly four
  non-empty cells; no `rule_id` repeats within the file.
- **Verify, per row**: `test` must exist as a Go test function in the named `package` (a `_test.go`
  under that package declares `func <test>(`), and the test must **run and pass**. A cited test that is
  absent, or that fails (including one that ends `--- SKIP`), is FAIL naming the row.
- **Execution is batched, per package**: for each `package`, one
  `go test -count=1 -v -run '^(<test>|<test>|…)$' <package>` invocation (with the repo's own
  `scripts/gate-goflags.txt` flags), then each cited test must appear in the output as `--- PASS: <test>`.
  One invocation per package, not per row — the gate must not pay a compile per citation.
- **Print**: one line per file (`<relative path>: <n> row(s) verified`), a summary line
  (`citations: <rows> row(s) across <files> file(s) verified`), and the decided verdict string.
- **Boundaries, disclosed not hidden** (the gate's own vocabulary — PASS / FAIL / SKIP / BOUNDARY / N/A):
  - no `*-citations.tsv` anywhere → `SKIP`, naming that (and, being a **required** step, the gate
    rewrites that SKIP to FAIL — a tree that has lost its acceptance surface must not read as passing);
  - the Go toolchain absent → the same platform boundary the Go steps report (`go_env()`), not a crash;
  - a `package` that is not covered by `scripts/gate-targets.txt` → FAIL (a citation may not point
    outside what the gate itself tests);
  - a `test` name that is not a legal Go test identifier → FAIL, before any tool runs.

## 4. Gate wiring

- `scripts/gate.py`: `citations` joins `KNOWN_STEPS`; `build_steps()` runs it after `parity` and before
  `secrets` (it reads the tree and the manifests; it is not a leak scan).
- `scripts/gate-required.txt`: `citations` is listed — the manifest **is** the acceptance surface, so a
  missing manifest or a stale citation must fail the gate rather than pass partially. This increment
  ships the seed manifest, so the required step has something to pass on.

## 5. Selftests (`scripts/tests/test_obligation_citations.py`)

Each rule in §3 pinned by a case that **fails when the rule is broken** (not merely asserted about the
shipped tree). The suite must be host-independent, exactly as `AGENTS.md` states: no hard-coded
separators, no assumption that a tool is installed, a stub `go` on a pinned `PATH` answering the probes
the case is not about. Cases:

- a well-formed manifest over a throw-away tree verifies (positive control);
- header wrong (missing/renamed/reordered column) → FAIL;
- a data row with a missing or an extra cell → FAIL;
- a duplicate `rule_id` → FAIL;
- a cited test that does not exist in the package → FAIL naming the row;
- a cited test that exists and fails → FAIL naming the test;
- a cited test the run reports as SKIP → FAIL;
- a `package` outside `gate-targets.txt` → FAIL;
- no manifest at all → SKIP; and, with `citations` in `gate-required.txt`, that SKIP is rewritten to FAIL
  by the gate (drive this through `gate.py` on a throw-away root, as `test_gate.py` does).

## 6. Seed manifest — `docs/evidence/0002-citations.tsv`

Covers every rule id of `docs/evidence/0002-obligation-matrix.md` that a **gate-runnable Go or harness
test** discharges. A header comment names the rule ids deliberately **not** covered — obligations whose
evidence is a native harness run rather than a gate test — with the reason: out of this mechanism's
scope **by design** (the owner's option-1 choice: a machine gate cannot re-run a native harness; that
class keeps its prose record and is the separate "post-run evidence anchor" follow-up). Each row's
`artifact` cites the matrix section the pair comes from, so the row is traceable back to its prose.

Extraction is by reading the matrix's own rows. The checker is the arbiter: a wrong test name or package
comes back as a FAIL naming the row, so the manifest is iterated until `citations` passes on the real
tree.

## 7. Out of scope, with reasons

- **Parsing prose to find rule ids** — rejected (the owner's option 2): prose parsing is the fragility
  this replaces.
- **Replacing the prose matrix / generating it** — deferred (option 3).
- **Migrating 0003–0007's matrices** — follow-up increments; this one lands the schema and the step.
  0007's manifest is produced when 0007 resumes.
- **Any change to `parity`, the Go steps, the carried tree, `upstream-manifest.txt`, the workflow files,
  or any existing assertion / evidence row / disclosed limit** — none is touched; `parity` must still
  report the carried tree identical to its pin.

## 8. Evidence plan (the increment's own record)

- `python3 scripts/gate.py` → `GATE: PASS`, with the `citations` step's real line
  (`citations: <n> row(s) across <m> file(s) verified`) and `parity` still identical.
- Each selftest case shown to fail on its rule broken, by a recorded mutation of the checker (or its
  input) and a byte-identical restore.
- The seed manifest's rows: each cited test shown to run and pass; the checker's FAIL path shown on a
  planted broken row.
- `AGENTS.md` growth stated (the gate paragraph gains `citations`) or, if the file needs no change, that
  said explicitly.

## 9. Ambiguities, and the reversible default taken

- **Which obligations get a row**: only those a gate-runnable test discharges; the excluded class is
  named in the manifest header, never silently dropped. (Default taken; disclosed in `conventions.md`
  and the increment's record.)
- **Batch granularity**: one `go test` invocation per package (rod row count is the citation's, not the
  invocation's). If a package's cited-test set proves to exceed a command line, batch within the package
  by the same bound `gofmt_batches` uses.
- **`artifact` content**: free text naming the matrix section (and, where useful, the file:line of the
  test), enough for a reader to reach the prose. It is not parsed.
