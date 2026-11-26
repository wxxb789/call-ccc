# 0009e — Native-check hardening: the refusal arm and the binary fingerprint

**Status:** Not started. Re-cut out of [0009a](0009a-native-oracle.md) by the owner (option C,
2026-11-26), after five cross-family review rounds returned defects in the same three classes (a P1
every round; the reviewed diff grew 171 KB → 364 KB), and re-decomposition — not another hardening
round — was the owner's answer. The moved features' **reviewed starting point** is the tag
`archive/0009a-r16-native-arm` (head `768df19`): the full disclosure arm and binary fingerprint as that
head left them, with their test cases. This goal starts from that tag; it never re-derives them.
**Context:** the umbrella contract is [`docs/goals/0009-selective-context.md`](0009-selective-context.md);
the re-cut is recorded in [`docs/plans/0009-decomposition.md`](../plans/0009-decomposition.md) (the
2026-11-26 section) and in [`docs/decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md)
(`Q-0009a-10`, with the moved items `Q-0009a-05`, `Q-0009a-07` and `Q-0009a-09`).

**Prerequisite:** [0009a](0009a-native-oracle.md).

## Intended advance

The two features 0009a's review could not bound move here to be hardened on their own, each on a
narrower surface:

- **the disclosure arm** — plant a fabricated artifact the offline rules accept into a home only the run
  can write, drive the target's own app server against it, and record the pair *the offline rules hold
  these bytes green / the binary refuses them*: 0009a's side-by-side demonstration that the offline
  oracle holds no Codex knowledge;
- **the binary fingerprint** — every run records the launched name's `--version` answer and the SHA-256
  of the file that name dispatches to, so every verdict below is attributed to one exact binary.

## Done when

- **D1 — The refusal is the binary's own answer about *this* artifact.** The arm's PASS requires the
  target's answer to name the planted artifact's **file name** (the owner's decided criterion, `Q1 = B`,
  2026-11-25; refined by `Q-0009a-07` to the refusal sentence about the exact planted path) — with an
  **id-only lookup miss as a failure control**, because a store answers it without ever opening the
  rollout. An I/O failure naming the path, and a refusal of a same-named file elsewhere, are each a
  failed arm in their own words. The planted artifact lives in a probe home of its own under the run's
  scratch directory (never the target home), and the arm judges the bytes **read back from the planted
  name** before reading the binary's answer for them.
- **D2 — The fingerprint pairs one executable's words with that executable's bytes.** The run resolves
  *which file the launched name dispatches to*: a plain symlink to the harness is followed for the
  digest only where the resolved file's own `--version` answer is the harness's, while a launcher's shim
  — the round-5 #2 shape (an official npm launcher forwarded through; on this host `codex` is a mise
  shim) — either dispatches to the real binary through the launcher's own answer or ends the run
  INCOMPLETE naming the launcher and the words it answered. The launched path itself is never
  `resolve()`d (a shim dispatches on `argv[0]`). A binary absent from `PATH` remains 0009a's named
  reason and non-zero exit.
- **D3 — Every root the harness can write is owned by this run or left untouched.** Anything the check
  fabricates in a home is created exclusively, recorded before its write, and removed by the run's own
  ownership object — and the roots a plant inherits from the environment are part of that ownership:
  the round-5 #1 shape (a P1), where the planted probe inherited the caller's `CODEX_SQLITE_HOME` and
  could open or migrate the caller's Codex database, is an acceptance item — a planted probe must not
  open, migrate or write a caller's database, at any root the harness can reach (`CODEX_SQLITE_HOME`
  beside `CODEX_HOME` among them).
- **D4 — The round-5 findings on the moved features are closed, each with its own case.** (a) finding
  #1 (P1): the planted probe's run environment does not inherit `CODEX_SQLITE_HOME` (or any other
  caller-named root) — the probe's writes are confined to a root this run owns; (b) finding #2 (P2):
  the words and bytes the fingerprint records belong to the same dispatched executable — an official
  npm launcher is never fingerprinted in place of the native binary it forwards to, or the run is
  INCOMPLETE.

## Bounds

`scripts/codex-native-check.py`, `scripts/tests/test_codex_native_check.py`, this goal's own
plan/evidence, and the documentation lines its own description occupies. **No Go change, no gate step,
no CI change, no `AGENTS.md` edit.** The offline oracle (0009a's shipped half) is read-only here: this
increment adds no oracle rule, moves no frozen fixture, and its changes to the check are confined to the
native half and the shared seams it stands on.

## Non-scope

The offline oracle, its rules, corpus and fixtures (0009a's, under the oracle lock); any other harness's
native check; any Python restatement of a foreign loader's judgment.

## Acceptance surface

This goal's own plan/evidence record — the arm's and the fingerprint's native runs quoted with their
exit codes and the harness's own words, the negative cases through the shipped check, and the mutations
that turn them red — plus the gate's `selftest` step, which runs the suite's cases; the native half
itself is never a gate step.

## Stop and report

If the conductor's convergence signal fires, or a source read cannot establish a region a caller named,
report the gap — do not invent a recovered region or a raised bound. Stop on missing user-only authority
or irreversible changes; otherwise use reversible defaults and end with evidence paired to every clause
above.
