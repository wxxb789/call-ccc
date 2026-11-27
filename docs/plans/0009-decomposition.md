# Goal 0009 — decomposition (fork A2, owner decision 2026-11-21)

**Decided.** Goal 0009 is decomposed into four ordered increments, the native oracle frozen first. Two blind
cross-family consults converged (`proposals/0009-consult-2-gpt62.md`, `…-carbon.md`).

## What the consults converged on

1. **Still the gain > 1 class, not a benign test tail.** Carbon's three *non-byte* signals: (a) direct
   self-excitation — round 1's fix added ~133 lines to `scripts/codex-native-check.py` and round 2 filed two
   findings **inside that script**; (b) **severity inversion** — 1 P1 → 2 P1s across a fix round; (c) reviewer
   saturation — `7 → 6` over 311 → 392 KiB is two readings at the detector's emission horizon, not a
   draining pool. GPT: enough to stop, and names the evidence that would settle it (replay the adjudicated
   findings against both revisions and classify each as self-inflicted / displaced / uncovered baseline).
2. **The new root cause both name: the verifier is co-mutated with the implementation.**
   `scripts/codex-native-check.py` is a **second consumer** that re-derives stream invariants in Python, and
   the fix round edited it **in the same increment** as the Go emitter — so each round moves *both sides* of
   `Impl ⊨ Oracle`. In carbon's words: *if the judge must move to pass, escalate rather than iterate.* GPT
   reaches it from the other side: *a writer and its read-back can agree about a rollout that omitted the
   wrong result.*
3. **GPT's sharper D6 gap:** the handoff back from the writer says which **kinds of items** a source record
   produced. *"This record produced history" cannot prove "this pair survived."* A record can yield dialogue
   while the exact result that made it necessary is omitted.
4. **Both: keep the breaker's verdict, replace its formula** — bytes alone both false-positives (a clean
   increment adding table tests) and false-negatives (production churn under a net threshold).
5. **Both: no new parser, index or framework.** GPT states it explicitly; carbon's sum-typed AST is a larger
   design than the evidence requires. Adopted instead: a typed per-occurrence triple carried in the existing
   transient boundary, plus a **manifest receipt** the external checkers consume.

## The decomposition (four ordered increments, each inside single-pass audit)

| # | Increment | What it proves that the others cannot | Touches |
|---|---|---|---|
| **A2-1** | **Freeze the native oracle** — `scripts/codex-native-check.py` + its tests + synthetic Codex fixtures, **zero Go changes** | The validator is sound on its own, and it is then **read-only for the rest of goal 0009** → the `Impl ⊨ Oracle` co-mutation loop is deleted as a class | `scripts/codex-native-check.py`, `scripts/tests/test_codex_native_check.py`, fixtures |
| **A2-2** | **Source binding + route isolation** — materialise the read's cross-record facts once, wire the route table and CLI gating under a keep-all pass | Binding extraction over arbitrary Claude logs is lossless and behaviour-preserving across all five routes; `targets.go` / `route_shared.go` / `main.go` / the other three targets leave every later diff | `internal/ccc/conversation.go`, `route_shared.go`, `targets.go`, `pi/omp/opencode` targets, `cmd/ccc/main.go` |
| **A2-3** | **Selection + closure, pure and offline** — the selector, its refusal surface, and **closure by construction** before any writer sees the stream | Every post-selection stream is closed and well-formed by construction; the closure obligation names **both occurrences** (`this call` + `this result`), not the records that contain them | `internal/ccc/selection.go`, `selection_test.go` |
| **A2-4** | **Codex lowering + native acceptance** — the writer lowers each closed unit locally, against the frozen A2-1 oracle | `codex_target.go` is a stateless local homomorphism from closed units to Codex records, and Codex really opens the intended artifact | `internal/ccc/codex_target.go`, `codex_install.go`, `cmd/ccc/main.go` |

**Carried through all four:** the unflagged baseline stays a separate regression obligation; the flags are not
exposed before A2-4; `--select`/`--strict-selection` remain refused by name on every pair but the reference one.

## The instrument change (both consults; operator-owned, reported not asked)

Replace the single byte rule with four measurements, keeping the **verdict semantics** the owner chose
(one convergence rule that blocks a non-contractive loop):

1. **Round-0 non-test entry ceiling** — refuse to enter the review loop when the initial **non-test** diff
   exceeds single-pass audit capacity (a saturated reviewer spends rounds, it does not converge).
2. **Non-test churn breaker** — growth and round-over-round churn computed on **non-test executable files
   only**; test/fixture growth gets its own, much higher runaway ceiling.
3. **Oracle lock** — block immediately when a fix round moves a gate/checker script **alongside** production
   code: if the judge must change to pass, that is an escalation, not an iteration.
4. **Severity monotonicity** — block on **any rise in P0/P1 count** (this catches PR #16 with zero dependence
   on bytes) and on a hunk-overlap defect yield ≥ findings retired.

## What the owner decided, and what superseded what

- **PR #14** (the wide attempt) was closed unmerged after 19 zero-clean rounds; its head is tag
  `archive/0009-pr14-pre-forkA` and its review log is the negative-case corpus.
- **PR #16** (the first narrowing) was closed unmerged after the breaker tripped at round 2; its head is tag
  `archive/0009-pr16-pre-forkA2`. The oracle work it contains is the starting point for **A2-1**.
- **Instrument**: the convergence formula was replaced (not the verdict semantics) and calibrated against this
  series' own 11 recorded review histories — see `probes/contractive-instrument-v2-probe.py`, whose assertions
  separate the two non-contractive runs from the nine that converged.
- Goal **0009** itself is no longer a runnable id; it survives as the umbrella contract. The route fan-out
  (0018–0021) now depends on **0009d**.

## The 2026-11-26 re-cut (owner option C): four increments become five

Five cross-family review rounds on 0009a's implementation (lane rounds r1–r5, a different model family
per round) returned defects in the **same three classes**, severity did not decay (a P1 every round)
and the reviewed diff grew 171 KB → 194 KB → 338 KB → 364 KB. The root-cause consult named the
boundary: A2-1's increment carried two features — the **disclosure arm** and the **binary
fingerprint** — that each round hardened at one site while the next round found the next inherited
environment variable or the next launch indirection ("if the judge must move to pass, escalate rather
than iterate", this document's own instrument rule). The owner re-decomposed rather than hardened
again:

- **0009a ships the offline oracle plus the native check without the two features** (the increment this
  repository's branch `feat/0009a-native-oracle-v2` now carries), keeping every diff that fixes a
  reproducible defect of a surviving path — the dangling-symlink crash in a caller's `--codex-home`,
  the portable held-name rule for the exclusive create, and the launching rule: no component
  normalized, nothing `resolve()`d, and a spelling whose anchor only a consumer's working directory
  would supply (a drive-less rooted path) answered anchored to this process's own (`Q-0009a-12`,
  `Q-0009a-16`).
- **0009e** is the moved work's home, with the reviewed starting point at tag
  `archive/0009a-r16-native-arm` (head `768df19`): the disclosure arm (owner criterion `Q1 = B`, an
  id-only lookup miss as the failure control), the binary fingerprint (which file the launched name
  dispatches to), the ownership seam for every root the harness can write (the round-5 #1 P1: the
  planted probe inheriting a caller's `CODEX_SQLITE_HOME`), and the round-5 #2 P2 (an official npm
  launcher fingerprinted instead of the native binary it forwards to).

The route fan-out (0018–0021) still depends on **0009d**; 0009e is independent of it. The re-cut is
also recorded in `docs/decisions/OPEN-QUESTIONS.md` (`Q-0009a-10`, with the moved items `Q-0009a-05`,
`-07` and `-09`).
