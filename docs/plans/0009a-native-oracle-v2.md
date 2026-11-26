# Plan — 0009a (v2) — the native oracle, re-scoped

Contract: [`docs/goals/0009a-native-oracle.md`](../goals/0009a-native-oracle.md) (reactivated by this
increment). The re-scope's authoring brief is `proposals/0009a-rescope-brief.md` in the task tree; the
superseded attempt's head is the tag `archive/0009a-pre-rescope` (`8787ae6484`, PR #19, closed).

Written before implementation. This increment implements the goal's **re-scoped** premise: the offline
oracle keeps only rules that are **format-agnostic** — the JSONL container, a frozen artifact's receipt
integrity, and the rule registry's own closure — and the **Codex well-formedness and replay judgment
belongs to the `codex` binary**, driven by the native check and reported as *its* answer.

## Why the re-scope

Goal 0009a's original premise froze a **standalone Python specification of Codex well-formedness** and
mutated it. `AGENTS.md`'s one-owner-per-half rule forbids exactly that — the referee for a foreign
format is the carried reader or **the real binary that owns the format**, never a private
re-implementation — and the attempt spent sixteen review rounds paying for it (PR #19: rounds r1–r12
before the mechanism repair, then four more; never closed). The re-scope keeps the goal's *purpose* —
an oracle that cannot be co-mutated with the implementation it judges (the oracle lock) — and deletes
the Codex knowledge from Python rather than repairing it.

## The premise this increment implements

1. **Survives** — only format-agnostic rules:
   - the container: one JSON object per line, JSONL line discipline (whole terminated lines, no
     byte-order mark), the file's own UTF-8 validity;
   - receipt integrity: a frozen artifact's byte count and whole-file SHA-256 plus one anchor per line
     (a receipt covers an artifact holding at least one line; the empty artifact is not
     receipt-identified by this rule), so the fixture is re-identified by its bytes in a later increment
     and a mutated byte is named by the line it moved;
   - the registry's own closure: the declared rule set equals the mutated rule set
     (`--selftest-mutations`).
2. **Deleted** — every rule whose subject is a Codex member, event vocabulary, record shape or replay
   ordering, with its fixtures and mutations. The list is derived by **reading PR #19's diff**
   (`git diff 8989d2c..archive/0009a-pre-rescope`) — one entry per rule, named by its id, its positive
   fixture and each of its mutations — and lands in the evidence record in full.
3. **The Codex judgment is the installed `codex` binary's** — the native check runs the binary against an
   artifact and reports *its* answer (load / resume / refusal); where the binary is absent the check
   prints the reason and exits non-zero (exit 2 today). No Python restatement of the loader anywhere.
   The owner's answer stands: follow `latest` (no version re-pin), and every native run records the
   resolved version and the binary's own SHA-256.
4. **Not claimed** — that the offline half can detect a Codex artifact the binary would reject. It
   cannot, by construction; the disclosure lands in the goal file, the evidence record and the harness
   documentation (the gate never runs the native half).

## Design

**Where it lives.** `scripts/codex-native-check.py` gains one oracle section and the modes
`--validate [--receipt] [--disable-rule]` and `--selftest-mutations`; the native half (goals 0002/0004)
is otherwise untouched (the re-cut below removed the two report additions the first draft carried).

**Registry (`ORACLE_RULES`), four ids, each a format-agnostic sentence:**

| id | states |
| --- | --- |
| `oracle.utf8` | the artifact's bytes are valid UTF-8 text and its first bytes are no byte-order mark |
| `oracle.whole-lines` | the artifact's bytes are whole lines: the empty artifact, or bytes whose final byte is a line feed — an artifact whose last bytes are an unterminated record is refused |
| `oracle.jsonl-objects` | every line is exactly one JSON object, every member name written once inside it, and no string it holds carries an unpaired surrogate escape — a repeated name and a `\ud800`-style escape are shapes JSON itself leaves two readers free to answer differently for, so such a line is refused rather than read as either answer; a line nested deeper than 256 levels is refused as well — the bound is the rule's own statement rather than the reading interpreter's recursion limit |
| `oracle.receipt-integrity` | a receipt states the frozen artifact's byte count, its whole-file SHA-256 and one anchor per line — the SHA-256 of that line's own bytes, line feed included, in order — and a receipt whose statements do not match the artifact's bytes is refused, naming the line a changed byte moved; a receipt covers an artifact holding at least one line, so the empty artifact is not receipt-identified by this rule; a receipt document nested deeper than 256 levels is refused as well — the bound is the rule's own statement rather than the reading interpreter's recursion limit |

**Every rule is evaluated, or explicitly `NotEvaluated`.** `ORACLE_RULE_INPUTS` declares what each rule
reads: `oracle.utf8`, `oracle.whole-lines` and `oracle.receipt-integrity` are `on_bytes` — evaluated
against the artifact's own bytes, the line rule included (a decode result cannot gate a byte predicate),
and the receipt wherever one was handed in (the r2 round's refinement: its disagreement survives the
text layer's failure) — while `oracle.jsonl-objects` is `needs_text`: where the byte layer answered no
text it is `NotEvaluated(blocked_by="oracle.utf8")`, a result the report shows by name rather than an
absent violation. `--disable-rule` is a set subtraction from the violation list *after* every rule was
evaluated, never a reason to skip one; a verdict is `PASS` only where every rule was evaluated and no
violation remains, `FAIL` where violations remain, and `INCOMPLETE` (exit 2) where none remains but a
rule was left unjudged (the r3 round's seam: the pre-fix reading answered PASS over an unterminated
record whose byte rule the decode failure had skipped). The corpus is part of the seam: for every pair
of byte rules one mutation violates both, declared in each rule's arm, so disabling either still cites
the other (`test_every_pair_of_byte_rules_is_co_cited_by_a_mutation`).

**The artifact fixture is not a Codex session.** `scripts/tests/testdata/codex-oracle/artifact.jsonl` is
three JSON objects that model no agent's format, committed with its receipt
(`artifact-receipt.json`); the superseded fixtures (`session.jsonl`, `reduced.jsonl`,
`session-media.jsonl` and the two manifest receipts) are **deleted with their rules**. A committed
fixture that *were* a Codex session would re-enter the Codex knowledge the deletion removes; a case that
fails if schema knowledge creeps back (`CodexArtifactOracle`) asserts both the id set and that this
artifact passes, and the disclosure arm in the native check shows the binary refusing the same bytes.

**Receipt document shape.** One JSON object per artifact:
`{"bytes": <n>, "sha256": "<64 hex>", "lines": [{"sha256": "<64 hex>"}, …]}` — one entry per line, in
order, each over the line's own bytes **including its line feed**; further members are not read. A
document that does not parse, or repeats a member name, is an unreadable *input* (`INCOMPLETE`, exit 2),
never a pass.

**CLI.** `--validate <artifact> [--receipt <document>] [--disable-rule <RULE_ID>] [--json]` prints
`oracle: PASS` / `oracle: FAIL — <n> violation(s)` / `oracle: INCOMPLETE — <reason>` with exit 0/1/2;
`--selftest-mutations` runs the corpus (three arms per mutation: the positive fixture passes, the
mutation cites its own rule beside every other rule its bytes are known to break, and `--disable-rule`
turns its own rule off — clean of that rule, green where every rule was still evaluated, INCOMPLETE
where the disabled rule left a `needs_text` rule unjudged, or still failing citing exactly those others)
through the shipped command's own path, on mutated bytes written under one scratch directory.

**The 2026-11-26 re-cut (owner option C).** Five cross-family review rounds returned defects in the same
three classes (a P1 every round; the reviewed diff grew 171 KB → 364 KB), and the root-cause consult
named the two features whose footprint kept moving: the **disclosure arm** and the **binary
fingerprint**. Both are removed from this increment and moved to [goal
0009e](../goals/0009e-native-check-hardening.md), whose reviewed starting point is tag
`archive/0009a-r16-native-arm` (head `768df19`). What this increment now ships under D3 is the native
check **without** them: the loader's own answers and the real-turn checks the base carried, with the
`os.path.abspath` (never `resolve()`) launching rule kept (the r7 round refines the absolutization: no
component is normalized, `Q-0009a-12`), and the absent-binary reason and non-zero
exit kept. The three round-5 findings that stay in the shipped half are fixed here: the
`--selftest-mutations` corpus declares each pair's expected unjudged rules and asserts them
independently of the reading's reply (D1); the receipt rule reads only its three members, so an ignored
member holding a five-thousand-digit integer cannot turn an unchanged receipt INCOMPLETE (D4); and the
run's own "real turn(s) run" count counts turns that ran, never the prompt flags a caller passed
(D3's run line). Every diff that fixes a reproducible defect of a surviving path is kept: the
dangling-symlink crash in a caller's `--codex-home`, the portable held-name rule for the exclusive
create, and the launch-path rule above.

## Acceptance (the goal file's `Done when`)

- **D1** — every surviving offline rule has its own negative fixture: one mutation per rule at minimum,
  and `--selftest-mutations` proves the declared set equals the mutated set over the survivors only,
  each pair's expected unjudged rules declared with it and asserted against the reading the command
  gets (never derived from the reply).
- **D2** — the offline oracle holds no foreign-format knowledge, stated testably: the registry names no
  Codex member, event type, record kind or ordering, and an artifact that is well-formed as a container
  but wrong as a Codex artifact passes the offline rules.
- **D3** — the Codex judgment is the binary's: the native check drives the resolved binary and reports
  its answer (the loader's own calls, and a real turn where a prompt flag asks for one); absence of the
  binary is an explicit reason and a non-zero exit; the offline half never stands in for it. The
  refusal arm and the version+SHA-256 fingerprint are 0009e's acceptance.
- **D4** — receipts: the frozen artifact's SHA-256 and anchors verify, and a single mutated byte breaks
  the receipt (two cases: verify, and verify-after-mutation); the receipt rule reads only its three
  members, so an ignored extra member is never converted.
- **D5** — the deletions are declared: the deleted rule list, with each rule's fixture and mutation, is
  in this increment's evidence; and `docs/core-design/README.md` §2's declared-debt entries
  (`Q-0005-18`, `Q-0005-20`, `Q-0003-12`, `Q-0008-10`, `Q-0005-19`, `Q-0007-13`) are re-read and either kept with
  their conditions or retired. This increment touches none of those arms (no Go change, no other
  native check), so each is **kept**, and the re-read is recorded.

## Bounds

`scripts/codex-native-check.py`, `scripts/tests/test_codex_native_check.py`,
`scripts/tests/testdata/codex-oracle/**`, `docs/evidence/0009a-citations.tsv`, the goal file and this
increment's own plan/evidence — plus the documentation lines the deleted oracle's description occupies
(`docs/harness.md`'s native-check section, the goals index row), the superseded plan stub
(`docs/plans/0009a-native-oracle.md`, reduced to a pointer in the same increment) and
`docs/decisions/OPEN-QUESTIONS.md`.
**No Go change, no gate step added, no CI change, no `AGENTS.md` edit** (this increment changes no
layout, no gate step, no command an agent runs and no rule an agent follows, so the growth policy says
no `AGENTS.md` edit is owed). Nothing carried is touched. A cited Go test must already exist and pass.

## Rollback

Revert the working tree to `feat/0009a-native-oracle-v2`'s base: the superseded content is preserved at
tag `archive/0009a-pre-rescope`, the goal file's held text is in git history, and this increment leaves
every edit uncommitted for the operator. Nothing in the increment writes outside the repository, and
`--selftest-mutations` writes only under one scratch directory it removes.
