# Evidence — prep increment: the ownership rule and 0015–0017's deferral

The commands that ran, with their verdicts. Scope: the documents this increment changes (the plan's scope
list) plus its own records; **no Go file, no script, no gate step, no workflow** — so the Go steps below are the gate proving the
tree it already proved, and the steps that *read* this increment are `parity`, `selftest` and `secrets`.

## Gate, per reviewed head

`python3 scripts/gate.py` — the **required** set is `scripts/gate-required.txt`. A required step that cannot
verify **fails** the gate; an unavailable **non-required** step can instead produce
`GATE: PASS (partial: …)` with exit 0. **This increment's evidence requires the unqualified `GATE: PASS`**: a
`(partial: …)` run is not what the plan's "gate green" means, whatever its exit code, and no row below is one.

| head | verdict | notes |
|---|---|---|
| `e2faae6` | `GATE: PASS` | gofmt 3 624 files clean · build/vet rc=0 · test 3 packages · selftest 442 tests · parity 5 553 carried files identical to `83195c8a`, 27 declared upstream-path deviations + 1 generated input, all present as declared, 32 owned prefixes · citations 0002: 36, 0007: 37, 0008: 13 · secrets clean in 178 owned files |
| `5c4e79c` | `GATE: PASS` | same eight steps, same counts — the simplify round's edits are documentation |
| `670161b` | `GATE: PASS` | same eight steps, same counts — the declared-debt clause is `AGENTS.md`, a declared deviation from upstream |
| `4b081f6` | `GATE: PASS` | same eight steps, same counts. **Sequencing disclosed:** the commit message for this head quotes the gate *before* the run above it, which belonged to `670161b`; the gate was then run on this head and passed. The claim is true of the head it names; the order was wrong, and it is recorded rather than amended away (amending a pushed commit needs a force-push) |
| `46edf2a` | `GATE: PASS` | same eight steps, same counts — the two-site debt clause |
| `43d1386` | `GATE: PASS` | same eight steps — `AGENTS.md` keeps the rule and points at §2; `files seen by git` 5 735; secrets clean in 180 owned files |

## CI

`.github/workflows/gate.yml`, matrix `[ubuntu-latest, windows-latest]`, `python scripts/gate.py` — the same
file this record's gate runs use:

| head | ubuntu | windows |
|---|---|---|
| `e2faae6` | pass | pass |
| `670161b` | pass 1m07s | pass 2m48s |
| `4b081f6` | pass 51s | pass |
| `46edf2a` | pass 1m11s | pass 3m06s |

## Carried-tree parity

`parity` is a **required** gate step and is the step that owns the copy: it re-derives the carried set from
`scripts/upstream-manifest.txt` (`pin 83195c8a6861b8c74a4b6cb87512817fed6f8584`, remote
`https://github.com/kenn-io/agentsview`, `v0.47.0-181-g83195c8a`) and compares index blobs, modes and
working-tree bytes. Every head above reads **5 553 carried files identical**, `deviations: 27 upstream path(s)
declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared`,
`owned: 32 project-owned prefix(es); files seen by git: 5733` — 5 735 once this record pair lands, since the
gate counts the project's own files too. The machine reviewer independently re-audited
the same set in each round (all 5 553 `file` records, the normalized fixture, the generated input, the ignore
rules via `git check-ignore --no-index`) and found no mismatch; it notes it verifies the checkout against the
**unchanged manifest**, not against a fresh upstream checkout, whose object is not present locally.

## Machine stages on PR #20 (owner's rule: both stages, no exemption for a small change)

**Simplify (report-only)** — `artifacts/pr20/simplify-report.md` in the task tree; three findings, all applied
before the first merge-ready head: (1) the rule appeared twice and the two copies had drifted in term and in
strength — the core-design copy now uses the contract's term and points at the contract's operable form;
(2) the deferral was recorded twice — the rows keep the reasons, the paragraph keeps the scheduling
consequence; (3) its own *possible defect*: the paragraph claimed the unattended series ended at 0014, which
the status table denies (0018–0021 are ready and come before it) — reworded.

**Code review** — one fresh read-only session per head, scoped to the project-owned delta
(`carried-excluded.txt` empty: this diff touches no carried path). Each round runs on the head it names; every
finding is a defect in what this increment wrote, and each was fixed rather than argued:

| round | head | findings | disposition |
<!-- counts inside these rows are what that round saw; the declared-debt inventory is §2's, not this table's -->
|---|---|---|---|
| 1 | `e2faae6` | P2: the ownership rule contradicts the required Pi-family source leaf check | guard kept as **declared debt** (`Q-0005-18`), clause added to the contract with the owner's approval |
| 2 | `670161b` | P2: the OpenCode source oracle sits outside the rule's only declared exception · P2: 0009a still reads *Ready* under the superseded contract · P3: documented order ≠ dispatch order · P3: `Q-0005-18`'s reversal described the opposite effect | four fixed: the debt clause names both sites, the index row reads *Rescope pending … not dispatchable*, the order paragraph names the registry as it stands, the reversal now says the cost is a **weakened** refusal |
| 3 | `46edf2a` | P2: the Pi/OMP **target-side replay** prediction is the same class and was missing from the debt · P2: `Q-0008-10`'s replacement proposed the conversion's own read-back, which is the *installed target* and cannot show what the source supplied · P3: the increment had no repository-local plan or evidence record | all three: the debt list is now a **single enumeration in `docs/core-design/README.md` §2** with a third entry (`Q-0005-19`), `Q-0008-10` requires an independent AgentsView source read with the read-back on the observed side, and this record pair was written |
| 4 | `43d1386` | P2: a fourth debt site — the Claude → OpenCode arm's expectations, whose `.strip()` drops a result that is a lone U+001C while the writer's `TrimSpace` keeps it, so a faithful carry can fail the oracle · P2: `Q-0005-19` mistook the loader's **observed** answer for an expectation (the same-count role swap is detected only *because* an expectation stands beside it) · P2: the 0009a hold lived only in the index, while the dispatcher reads prerequisites, not status · P3: this record overstated the gate's fail-closed rule (a non-required step can leave `GATE: PASS (partial: …)`, exit 0) | all four: §2 gains its fourth entry (`Q-0007-13`), `Q-0005-19` now requires an expectation **and** the mismatch regression before the walk goes, `docs/goals/0009a-native-oracle.md` and the index both read **Held** and both say the hold is **operator-enforced**, and this record distinguishes a required-step failure from a partial pass and requires the unqualified `GATE: PASS` |

| 5 | `8237606` | P2: a **fifth** debt site — the Claude → Pi arm of `scripts/pi-native-check.py` expects the first raw user record as the prompt, while the carried reader skips an `isMeta` record (recorded as an inspection finding, not yet reproduced natively) · P2: the blanket decoder ban would cover a *narrow emitted-byte* check (`codex_install.go`'s installed-dialogue walk, which pins the swapped-prompt regression) though no carried reader is re-derived · P3: the plan and the records had gone stale against each other ("goal file untouched" vs the new **Held** line; three-vs-four debt entries; three-vs-four rounds) | all three: §2 gains its fifth entry plus an explicit **non-completeness** clause (declare, don't enumerate) and a clause separating narrow emitted-byte structural checks from foreign decoders; the plan, the index and this record were reconciled, and the counts were removed in favour of "declared so far" / "each round on the head it names" |

| 6 | `bfa007d` | P2: the held goal's clauses read as **reactivated** by the re-scope, though the rescope must **replace** them · P3: two declaration procedures (the contract required a §2 *and* a Q entry, §2 allowed evidence + a Q entry) · P3: the plan's scope said three documents where the delta carries five, and counts survived in the plan and this record | all three: the hold now says the clauses are on record and the re-scope replaces them, with both superseded sections marked; §2's escape hatch is gone, so one procedure stands; the plan lists all five documents and refers to §2 for the inventory |

## What this record does not claim

- It does not claim the declared debt sites are correct today, only that they are **declared**, bounded, and
  each carries the condition under which it is replaced.
- It does not re-verify upstream provenance: the pin's own commit object is not available on this host, so
  parity here proves the tree matches the manifest, and the manifest's provenance is checked where the
  importer runs against a checkout of the pin.
- It does not run any native check: this increment changes no writer, verifier or native check, and the
  native tier needs the target harnesses installed.
