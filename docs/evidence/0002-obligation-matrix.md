# Evidence — 0002 obligation matrix (systematic close-out before one acceptance audit)

Contract: [`docs/goals/0002-claude-to-codex-continuation.md`](../goals/0002-claude-to-codex-continuation.md)
and [`docs/core-design/README.md`](../core-design/README.md).
Plan: [`docs/plans/0002-claude-to-codex-continuation.md`](../plans/0002-claude-to-codex-continuation.md).
Round-1..12 findings: [`0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md) §8–§17.

Candidate head: `3ee6cb2` (`fix(0002): address review findings (round 12)`), branch
`feat/0002-claude-to-codex-continuation`. Every command below was run on this tree, after the
close-out's own repairs; the head this matrix freezes is the head the acceptance audit reads, and this
file is its scope.

Purpose: enumerate the obligations this increment must discharge — from the core design, the goal's
`Done when` clauses, the public output claims and the behavioral claims of every shipped check and test,
including all 33 findings of the twelve ordinary rounds — and map each to its implementation, its
positive evidence, and **one executed mutation that detects its violation**. No obligation was weakened
to match the implementation; where the implementation did not discharge one, the implementation was
repaired and the pin added (§ "Repairs", surface C2/E1/E2/E4).

## How the evidence below was produced

- Positive evidence, every obligation: `python3 scripts/gate.py` on this tree printed
  `GATE: PASS` — `gofmt 3582 file(s) gofmt-clean`; `build`/`vet` rc=0; `test` `3 package(s) ok`;
  `selftest Ran 158 tests in 5.600s (rc=0)`; `parity … carried: 5553 file(s) identical to the pinned
  revision`; `secrets no leaks found in 74 file(s) this repository owns`. The Go steps cover the offline
  rules (writer, pipeline, installer, structural check, CLI); the `selftest` step covers the harness's own
  rules (native check, parity, gate). Where an obligation's own test is named, that test also ran alone
  and passed on the unmutated tree before the mutation was applied.
- Native positive evidence (surface E, and the C2 claim): one real run of the non-gate harness on this
  host, exit 0 —
  `python3 scripts/codex-native-check.py --model-provider tencent-litellm --model deepseek-v4.1-flash-fast --resume-prompt "Answer only from this conversation, one line each: which file did you read, and what was the attempt limit?"`
  → `13 check(s) passed, 0 failed, 0 skipped`; the loader answered
  `thread/resume -> 01a1e365-0b98-7043-9cfd-fff24cca0081 [legacy] cwd=/home/user/code/example-api`; the
  target's own view listed `5 item(s)` (`userMessage`/`agentMessage`); a real turn ran
  `exit 0, session 3970 -> 67991 bytes`; the source, the planted session and every pre-existing session
  were unchanged. That harness is deliberately **not** a gate step (`docs/harness.md`), which is why the
  offline pins below matter.
- Mutations: each was applied to the working tree as a temporary pattern-addressed edit (spec given
  verbatim below), the named test was run once with `GOFLAGS=-mod=mod CGO_ENABLED=1 go test -tags fts5
  -count=1 -run <Test> ./…` (or `python3 -m unittest …` for the harness rules), the observed failure and
  the test's exit status are recorded, and the file was restored from a byte-identical copy whose sha256
  was compared after the restore (`restored (sha256 …)` in every row). No mutation survived: the tree this
  file ships with is the unmutated one, and the final gate run above is on it.
- One obligation (F1) also names a **probe that did not detect**: recorded as a measured limit rather
  than dropped, because a mutation that cannot fail is not evidence.
- Host paths, the repository root and the throw-away directories are rewritten as `<repo>`, `<home>`,
  `<tmp>`; no credential, token or absolute host path is committed.

---

## Audit repair (2026-11-16) — the four acceptance findings

One authorized acceptance audit read **this file** at candidate head `6fe0d7a` and returned exactly four
P2 findings; the run stopped for replanning as designed. The repair is on the same branch
(`feat/0002-claude-to-codex-continuation`, head `6fe0d7a` plus these uncommitted edits) and **only the
rows it names changed** — A2, A3, C2, E1, E2, E4, E6 and F4. Every other row, and every measurement a row
records, is the record of the head it was taken on: the `Restored …` digests of untouched files are the
same at this head, and the two files whose repair is a source change have their new digests stated in
their rows (`internal/ccc/reader.go` `95e68873…`, `cmd/ccc/main_test.go` `0ea5446a…`).

- Gate on the repaired tree, run by this repair: `python3 scripts/gate.py` → `GATE: PASS` —
  `gofmt 3582 file(s) gofmt-clean`; `build`/`vet` rc=0; `test 3 package(s) ok`; `selftest Ran 161 tests …
  (rc=0)`; `parity … carried: 5553 file(s) identical to the pinned revision (compared through
  git's own object store)`; `secrets no leaks found in 75 file(s) this repository owns`. The two Go
  packages the repairs touch also ran alone (`go test -tags fts5 -count=1 ./cmd/ccc/... ./internal/ccc/...`
  → `ok`), and each named test ran alone and passed before its mutation was applied.
- The four findings, and the obligation each one narrowed:
  1. `internal/ccc/continue.go:677` — a **source I/O failure** (a fingerprint or a parse that could not
     read the file) escaped `readForConversion` as a bare error, before the archive was consulted; the
     A2 row's cited test covers a *deleted* source only → A2, A3.
  2. `cmd/ccc/main_test.go:1771` — the CLI verification tests handed `continueDocumentOf` the constant
     `exitOK`, and the resume case left the stub loader's mode unset, so its positive case claimed a
     success the command had not produced → E1, E2, E4.
  3. `scripts/codex-native-check.py:388` — the stderr drainer appended every line to an unbounded list;
     excerpt truncation bounds presentation only, and the cited mutation covers the separate Go buffer →
     E6.
  4. `internal/ccc/codex_target_test.go:1113` — the event-envelope assertion compared through this
     writer's own constant (`codexRecordEventMsg`), so writer and assertion could drift together away
     from the native `event_msg` spelling → C2.
- Every repair carries **executed mutations**: finding 1 one per half (the fingerprint wrap and the parse
  wrap reverted one at a time), finding 2 the detecting run plus the pre-repair shape re-run (which passed
  while the CLI exited `1`), findings 3 and 4 one each (finding 4 also re-running the pre-repair
  assertion). Each was applied as a temporary pattern-addressed edit, run with the named test, observed,
  and restored from a byte-identical copy whose sha256 was compared after the restore.
  No mutation survived; the tree this file ships with is the unmutated one, and the gate above is on it.
- No gate step, gate knob, required-step list or carried upstream file was touched:
  `scripts/gate.py`, `scripts/gate-required.txt`, `scripts/gate-goflags.txt`, `scripts/upstream-parity.py`
  and `.github/workflows/` are as they were, `parity` still reports 5553 carried files identical to the
  pinned revision, and no existing assertion, evidence row or disclosed limit was deleted or weakened.
  `docs/harness.md` gained the sentence that states the new retention bound.

---

## Surface A — source selection and fidelity

### A1 — A reference resolves only to the source it names; an ambiguity is reported with candidates

- **Clause:** reading `claude:<id-or-path>` answers with the one session the reference names; a reference
  a provider answers with several candidates for is reported as ambiguous with those candidates, never
  guessed.
- **Discharged:** `internal/ccc/reader.go:308` (classification), `:386-413` (path and multi-source
  ambiguity), `:605-613` (`ambiguousPathResults`), `internal/ccc/inspect.go` (the report's candidates).
- **Positive evidence:** gate `test` step, `3 package(s) ok`; `TestReadReportsAForkedPathAsAmbiguous`,
  `TestReadReportsAForkedPathAsAmbiguous`'s sibling `TestMissingSessionIsAFailureWithACandidateList` pass.
- **Pinned by:** `TestReadReportsAForkedPathAsAmbiguous` (a forked transcript read by path is an
  ambiguity carrying both parsed identities, not a session).
- **Executed mutation:** `reader.go:613` `return &AmbiguousError{Ref: ref, Candidates: candidates}` →
  `return nil` (a path ambiguity resolves the way an unambiguous reference does). Command: the test above.
  **Observed:** exit `1`; `FAIL: TestReadReportsAForkedPathAsAmbiguous`;
  `fixtures_test.go:1005: reading a forked transcript by path answered session not found: … , want an
  ambiguity report`. Restored `ad2a15ce85f7a3549b7da800917798ca57788d7f1a31d61c6a0806fdbf18932b`.

### A2 — A source that cannot be read falls back to the archive, and the report says so

- **Clause:** a source-side read failure falls back to the archive, and the conversion reports
  `read_from: archive` with a warning naming the source's own failure — "converted" never silently stands
  for a different source than the file.
- **Discharged:** `internal/ccc/continue.go:670-736` (`readForConversion`), `:683` (the fallback guard),
  `:722-725` (the warning naming the archive and the source error).
- **Positive evidence:** `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` passes; the run's JSON in
  the round-1 evidence record (`read_from: "archive"`) is the same shape.
- **Pinned by:** `TestContinueFallsBackToTheArchiveWhenTheSourceIsGone` (the source is gone, the archive
  answers, and the report says which side answered and why).
- **Executed mutation:** `continue.go:683` `if strings.TrimSpace(archivePath) == "" {` → `if true {`
  (the fallback never runs). **Observed:** exit `1`; `FAIL` at
  `continue_test.go:704: continuing from the archive: session not found: …`. Restored `e5be4e8a…`.
- **Audit repair (2026-11-16, finding 1 of the acceptance audit).** The fallback was reachable only for
  the classifications `readForConversion` lists, and that list did not cover a source read that failed at
  the **I/O level**: `internal/ccc/reader.go`'s `Read` wrapped a failed `Fingerprint` or `Parse` as a bare
  error, the switch fell to its `default`, and the conversion returned before the archive was opened — the
  cited test above covers a *deleted* source only, which is `ErrNotFound`. Both shapes now travel as
  `ErrProviderFailed` (the sentinel the accepted set already holds, whose documented meaning is "a
  provider resolved a source but could not produce a session from it"), the reader's own message and the
  underlying failure stay in the chain (`reader.go:483-498`), and the set's own comment
  (`continue.go:677-688`) states why it is the gate. The report is unchanged and now says which answer
  came from where for this shape too: `read_from: archive` plus a warning naming the archive path and the
  source's own failure.
- **Pinned by (audit repair):** `TestContinueFallsBackToTheArchiveWhenTheSourceCannotBeRead`
  (`internal/ccc/continue_test.go:736`) — three cases: a fingerprint I/O failure, a parse I/O failure, and
  a source I/O failure whose archive refuses with an ambiguity. They drive the shipped `Read` through a
  provider that fails at the I/O level (the test-only seam `fixedSourceProvider.fingerprintErr`/`parseErr`
  in `fixtures_test.go`) and assert the archived read, `source_check_state: not-verified` with its reason,
  and the warnings naming the archive **and** the source's own failure. The same sentinel is what `Inspect`
  switches on, so its own documented rule — a source the reader cannot read is a warning rather than an
  error whenever an archive could still answer the reference (`inspect.go:92-98`) — now covers this shape
  as well instead of ending the inspection at the reader's error: pinned by
  `TestUnreadableSourceFallsBackToTheArchive` (`internal/ccc/archive_test.go:748-794`), the sibling of the
  deletion rule that row already had, executed under the same mutation.
- **Executed mutations (audit repair, one per half, each run on the repaired test):**
  1. `reader.go:488` `fmt.Errorf("%w: fingerprinting %s: %w", ErrProviderFailed, source.DisplayPath, err)`
     → the pre-repair `fmt.Errorf("fingerprinting %s: %w", source.DisplayPath, err)`. Command: the test
     above. **Observed:** exit `1`; `FAIL` with `the_fingerprint_could_not_read_the_file`
     (`continue_test.go:782: continuing after the source read failed: fingerprinting <…>/9f2c1d47-….jsonl:
     input/output error`) and `the_archive's_own_refusal_is_the_reported_failure` failing while the parse
     case passed; the same mutation and the same edit, run against
     `TestUnreadableSourceFallsBackToTheArchive`, fails it too
     (`archive_test.go:778: inspecting a source that could not be read: fingerprinting <…>: input/output
     error`). Restored `95e68873…` (this file's own digest at the repaired head).
  2. `reader.go:497` `fmt.Errorf("%w: parsing %s: %w", ErrProviderFailed, source.DisplayPath, err)` → the
     pre-repair `fmt.Errorf("parsing %s: %w", source.DisplayPath, err)`. **Observed:** exit `1`; `FAIL`
     with `the_parse_could_not_read_the_file` (`continue_test.go:782: … parsing …: permission denied`)
     failing while the fingerprint case passed. Restored `95e68873…`.

### A3 — When both sides refuse, the archive's own answer is the reported failure

- **Clause:** if the source read fails and the archive also refuses the reference, the failure reported is
  the archive's own (an ambiguity with its candidates, a truncated lookup, a query failure) — never the
  source's "not found" standing in for all of them.
- **Discharged:** `internal/ccc/continue.go:695-714` (`readForConversion`'s archive branch, `:702`).
- **Positive evidence:** `TestContinueReportsTheArchivesAmbiguityInsteadOfAbsence` passes.
- **Pinned by:** `TestContinueReportsTheArchivesAmbiguityInsteadOfAbsence` (a reference two archive rows
  answer is an ambiguity, not absence).
- **Executed mutation:** `continue.go:702` `if !errors.Is(readErr, ErrNotFound) {` → `if false {`.
  **Observed:** exit `1`; `FAIL`; `continue_test.go:778: a reference two archive rows answer returned
  session not found … want an ambiguity report`. Restored `e5be4e8a…`.
- **Audit repair (2026-11-16, finding 1 of the acceptance audit).** "The source read fails" now covers a
  read that failed at the I/O level as well, and the rule that the archive's own answer is the one
  reported therefore holds for that shape too — executed, not inferred: the third case of
  `TestContinueFallsBackToTheArchiveWhenTheSourceCannotBeRead`
  (`internal/ccc/continue_test.go:812-864`) fails the source at the I/O level, seeds two `host~<id>` rows,
  and asserts the reported failure is the **archive's** ambiguity with both candidate identities (and that
  it is not `ErrNotFound`). Before the repair the source's bare error was returned before `archive.Read`
  ran, so no archive answer could be reported at all.
- **Executed mutation (audit repair):** the mutation recorded under A2 (the fingerprint wrap reverted to
  its pre-repair form) fails this case by name —
  `the_archive's_own_refusal_is_the_reported_failure` — as the A2 row's first observation states. The
  archive half of the rule above keeps its own mutation, unchanged: `continue.go:702` → `if false {`.

### A4 — The read's own completeness and the provider's own classification reach the report

- **Clause:** lines the provider's reader could not turn into a message are counted in
  `conversion.losses[]` from the read's own count (with the unfinished tail named), and a read the
  provider classified as a summary fallback or as needing a retry is reported in the losses *and* in the
  run's warnings — a consumer that read only "converted" must not believe it holds the session's recorded
  transcript.
- **Discharged:** `internal/ccc/codex_target.go:818-836` (the read-completeness reason, the truncated
  suffix, the two classification reasons), `:1008-1027` (the three entries), `internal/ccc/continue.go:279`
  (`fidelityWarnings` into the run's warnings), `:562-583` (their wording).
- **Positive evidence:** gate `test` step; `TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce`,
  `TestCodexRolloutReportsEveryCategoryItKnows`, `TestContinueDisclosesASummaryTranscriptRead`,
  `TestContinueDisclosesARetryableRead` all pass.
- **Pinned by:** `TestCodexRolloutReportsTheSourceLinesItsReadCouldNotProduce` (the read's own malformed
  count, and the truncated tail named in the read's own words); `TestContinueDisclosesARetryableRead` /
  `TestContinueDisclosesASummaryTranscriptRead` (classification in the warnings *and* the losses).
- **Executed mutations (two, one per half):**
  1. `codex_target.go:1011` `Count: b.sourceFidelity.MalformedLines,` → `Count: 0,`. **Observed:** exit
     `1`; `FAIL`; `codex_target_test.go:999: malformed_source_lines = {… Count:0 …}, want the read's own
     count as an omission`. Restored `f43564af…`.
  2. `codex_target.go:1023` `Count: flagCount(b.sourceFidelity.NeedsRetry),` → `Count: 0,`. **Observed:**
     exit `1`; `FAIL`; `continue_test.go:2109: source_needs_retry = {… Count:0 …}, want the read's state
     counted once, carried with a caveat`. Restored `f43564af…`.

### A5 — A subagent (sidechain) record never stands for the main conversation

- **Clause:** a message the source marked `isSidechain` is omitted and counted, never written as the
  conversation's own turn — including a subagent record that also carries the source's compaction marker,
  whose summary must not be applied as the main conversation's `compacted` boundary.
- **Discharged:** `internal/ccc/codex_target.go:421-434` (the sidechain guard *before* the boundary rule),
  `:884-893` (`sidechain_messages`).
- **Positive evidence:** `TestCodexRolloutDoesNotApplyASidechainCompactionBoundary`,
  `TestCodexRolloutReportsEveryCategoryItKnows` pass.
- **Pinned by:** `TestCodexRolloutDoesNotApplyASidechainCompactionBoundary` (round 3, finding 1's rule: the
  subagent's summary is not the target's boundary).
- **Executed mutation:** `codex_target.go:421` `if message.IsSidechain {` →
  `if message.IsSidechain && !message.IsCompactBoundary {` (the pre-round-3 condition).
  **Observed:** exit `1`; `FAIL`; `codex_target_test.go:276: the conversion wrote the target's compacted
  record for a subagent's summary: {"message":"SUBAGENT-SUMMARY"}`. Restored `f43564af…`.

### A6 — A compaction boundary is the target's own record, and the history before it stays in the file

- **Clause:** a compaction boundary travels as the target's `compacted` record with the recorded summary,
  and the turns before it remain in the written file while being counted as carried-with-caveat
  (`pre_compaction_history`), because the target applies the boundary to its model-visible history.
- **Discharged:** `internal/ccc/codex_target.go:435-450` (the boundary record, the pre-boundary count),
  `:1030-1037` (`pre_compaction_history` and its reason).
- **Positive evidence:** `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt` passes; the
  native run's target view showed the post-boundary turns and the resumed turn continued from the summary.
- **Pinned by:** `TestCodexRolloutCarriesACompactionBoundaryAndKeepsTheHistoryBeforeIt`.
- **Executed mutation:** `codex_target.go:437` `b.lossPreBoundary = b.carried.Messages` →
  `b.lossPreBoundary = 0`. **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:246: pre_compaction_history = {… Count:0 …}, want 2 carried-with-caveat`.
  Restored `f43564af…`.

### A7 — The source check has three answered states, and claims only the comparison it ran

- **Clause:** `installation.source_check_state` is `unchanged`, `changed` or `not-verified`; an archive
  read, a container-member identity and a run that stopped answer `not-verified` with that answer's own
  words, and only a read that names no file at all (or a digest that was never taken) stops the run.
- **Discharged:** `internal/ccc/continue.go:749-759` (`sourceFileDigest`), `:780-807`
  (`sourceCheckAfter`), `:736-748` (`sourceFileOf`, the member identity), `:810-819`
  (`sourceChangedWarnings`).
- **Positive evidence:** `TestSourceCheckComparesTheFileBeforeAndAfter`,
  `TestSourceCheckSaysWhenThereWasNoFileToCompare`, `TestContinueFromAContainerMemberReportsNoFileToCompare`,
  `TestSourceCheckStopsAReadThatNamesNoFileAtAll`, `TestSourceChangedWarningsNameTheMove` pass.
- **Pinned by:** `TestSourceCheckSaysWhenThereWasNoFileToCompare` (a comparison that did not run is
  `not-verified` with the archive's reason, not a claim about the file).
- **Executed mutation:** `continue.go:781` `if readFrom != "source" {` → `if false {` (an archive read
  proceeds as though it had a file). **Observed:** exit `1`; `FAIL`;
  `continue_test.go:1749: state = "", err = the source digest before the conversion was not taken; want a
  comparison that did not run`. Restored `e5be4e8a…`.

## Surface B — semantic translation

### B1 — Only user and assistant roles are dialogue; a tool-role record travels only as its output

- **Clause:** a record whose role is not the user's or the assistant's is never written as either; a
  tool-role record's text is carried only as the `function_call_output` paired with its call, and a record
  with nothing but its role is counted under its own category.
- **Discharged:** `internal/ccc/codex_target.go:466-487` (the role gate), `:895-902` (`tool_role_messages`
  / `unclassified_role_messages`), `:952-960` (`non_dialogue_content`).
- **Positive evidence:** gate `test` step; `TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult`,
  `TestCodexRolloutReportsEveryCategoryItKnows` pass.
- **Pinned by:** `TestCodexRolloutCarriesAToolRoleMessageOnlyThroughItsResult` (the output is carried once,
  paired; the role is counted as no message).
- **Executed mutation:** `codex_target.go:477` `if role != parser.RoleUser && role != parser.RoleAssistant {`
  → `if false {`. **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:543: carried = {Messages:2 …}, want the output carried once and the tool record
  counted as no message`. Restored `f43564af…`.

### B2 — A persisted reasoning trace is omitted and counted, and never written as dialogue

- **Clause:** `Thinking` is omitted and counted (`reasoning_text`); a record the provider projected as
  having no dialogue but whose `Content` renders its reasoning is never written as the agent's words, and
  the writer never falls back to the rendering when the source's own projection says the message has none.
- **Discharged:** `internal/ccc/codex_target.go:451-465` (the reasoning-only guard), `:1066-1085`
  (`dialogueText`, the explicit-empty-projection rule), `:879-885` (`reasoning_text`).
- **Positive evidence:** `TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue`,
  `TestToolingFixtureCarriesTheShapesTheWriterMustNotMisread` pass; in the native run the trace's words
  appear nowhere in the installed file.
- **Pinned by:** `TestCodexRolloutKeepsAReasoningOnlyRecordOutOfTheDialogue` (the trace is counted and is
  not in the file as dialogue).
- **Executed mutation:** `codex_target.go:1067` `if strings.TrimSpace(message.Dialogue) != "" ||
  message.DialogueProjected {` → `if strings.TrimSpace(message.Dialogue) != "" {` (an explicitly empty
  projection falls back to `Content`). **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:787: the rollout holds the reasoning trace the report says is omitted, as the
  agent's dialogue`. Restored `f43564af…`.

### B3 — Tool calls and results pair by recorded identity, or are reported unpaired

- **Clause:** a call is written with its recorded tool-use id; a result is written under the call it
  names; an id-less result pairs only with exactly one outstanding call (two or more make the pairing a
  guess, none leaves nothing to pair with); a result naming a call this conversion did not write is
  omitted and counted.
- **Discharged:** `internal/ccc/codex_target.go:702-711` (`solePendingCall`), `:712-778` (`toolResult`,
  both omission guards), `:906-913` (`unpaired_tool_results`), `:945-951` (`generated_call_ids`).
- **Positive evidence:** `TestCodexRolloutPairsAnIdLessResultWithItsCall` (one, two and three outstanding
  calls) passes.
- **Pinned by:** `TestCodexRolloutPairsAnIdLessResultWithItsCall`.
- **Executed mutation:** `codex_target.go:703` `if len(b.pendingCalls) != 1 {` →
  `if len(b.pendingCalls) == 0 {`. **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:635: carried 1 result(s) for two id-less calls, which is a pairing the source did
  not record` and `:639: unpaired_tool_results = {… Count:0 …}, want the ambiguous result counted`.
  Restored `f43564af…`.

### B4 — Records are stamped in one ordered run

- **Clause:** the envelope timestamps never go backwards, so a source whose messages are unordered (a DAG,
  a replayed fork) cannot produce a file whose order contradicts its own times.
- **Discharged:** `internal/ccc/codex_target.go:323-334` (`codexClock.next`).
- **Positive evidence:** `TestCodexRolloutStampsRecordsInOneOrderedRun` passes.
- **Pinned by:** `TestCodexRolloutStampsRecordsInOneOrderedRun`.
- **Executed mutation:** `codex_target.go:325` `if at.IsZero() || at.Before(c.last) {` → `if false {`.
  **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:1042: line 5 goes back in time: 2026-05-05T09:00:00Z after 2026-05-05 10:00:10
  +0000 UTC`. Restored `f43564af…`.

### B5 — Duplicate / aggregate evidence is written once

- **Clause:** an archive read holds a call's outcome both as its recorded result events and as the derived
  summary in `ResultContent`; the events are written once each and the summary only for a call with no
  recorded result of its own (where it is the only evidence the read holds).
- **Discharged:** `internal/ccc/codex_target.go:538-577` (`writeToolEvidence`), `:578-588`
  (`callHasRecordedResult`).
- **Positive evidence:** `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults`,
  `TestContinueFromTheArchiveWritesEachRecordedOutcomeOnce` pass.
- **Pinned by:** `TestCodexRolloutWritesAnArchiveSummaryOnlyForACallWithoutResults` (round 3, finding 2's
  rule).
- **Executed mutation:** `codex_target.go:579` (first line of `callHasRecordedResult`'s body) prepended
  `return true`. **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:977: function_call_output records = ["toolu_1: the recorded output" …] (carried 3
  results), want … ["toolu_3: summary-only outcome"]`. Restored `f43564af…`.

### B6 — An encoded content field is decoded exactly once

- **Clause:** text the read already decoded (a result event, an archive row) is written as it stands; an
  encoded content field goes through the reader's decode once — a second decode strips the quotation marks
  a tool really printed.
- **Discharged:** `internal/ccc/codex_target.go:1086-1108` (`resultText`), `:1109-1137`
  (`unrepresentedResultBlocks`).
- **Positive evidence:** `TestCodexRolloutDecodesEncodedContentExactlyOnce` passes; the native run's
  resumed turn quoted the third tool's `"maxBackoffSeconds"` with its quotation marks.
- **Pinned by:** `TestCodexRolloutDecodesEncodedContentExactlyOnce`.
- **Executed mutation:** `codex_target.go:1087` `if result.ContentDecoded {` → `if false {`.
  **Observed:** exit `1`; `FAIL`; `codex_target_test.go:862: already-decoded content written as "alpha",
  want it unchanged: a second decode strips the quotation marks the tool really printed`.
  Restored `f43564af…`.

### B7 — Unsupported content is named by category, count and kind; nothing omitted is reported as carried

- **Clause:** every loss category the writer knows is reported on every run including its zero counts, the
  counts are truthful (text the file does not hold is not called carried), the target's
  `function_call_output` cannot carry a content block that only holds an image payload (counted as blocks),
  and text the inherited reader reads as *nothing at all* (whitespace-only output, whitespace-only
  dialogue) is never written and is named under its own category.
- **Discharged:** `internal/ccc/codex_target.go:792-1030` (the fixed category table), `:712-778` (the
  whitespace and empty guards before the caveat counters), `:1109-1137` (block counting), `:488-500` and
  `:930-944` (whitespace categories).
- **Positive evidence:** `TestCodexRolloutReportsEveryCategoryItKnows` (all 22 categories present and
  truthful), `TestCodexRolloutDoesNotReportOmittedResultsAsCarried`,
  `TestCodexRolloutReportsToolResultBlocksTheTargetCannotCarry`,
  `TestContinueOmitsOutputTheInheritedReaderCannotReadBack`, `TestContinueOmitsDialogueTheInheritedReader
  ReadsAsNothing` all pass.
- **Pinned by:** those four tests, each naming the rule in its own name.
- **Executed mutations (three):**
  1. `codex_target.go:891` `Category: "unclassified_role_messages",` →
     `Category: "unclassified_roles_renamed",`. **Observed:** exit `1`; `FAIL`;
     `codex_target_test.go:422: the report does not name the "unclassified_role_messages" category`.
  2. `codex_target.go:1130` `if holds && !carried {` → `if holds && carried {`. **Observed:** exit `1`;
     `FAIL`; `codex_target_test.go:916: unrepresentable_result_blocks = {… Count:0 …}, want the two image
     blocks counted`.
  3. `codex_target.go:734` `if strings.TrimSpace(text) == "" {` → `if text == "" {`. **Observed:** exit
     `1`; `FAIL`; `continue_test.go:1454: carried = {… ToolResults:1 …}, want no tool result: the recorded
     output holds only whitespace`.
  Restored `f43564af…` after each.

### B8 — A system-injected record contributes no text, but its tool evidence travels once, paired

- **Clause:** a record the provider flagged as system-injected has its own text omitted and counted; a tool
  result recorded on that same record is real evidence of a call this conversion carried, so it is written
  once, paired with its call — dropping it would leave the call unanswered while the report named only the
  record's text.
- **Discharged:** `internal/ccc/codex_target.go:452-461` (the `IsSystem` branch) and `:955-971`
  (`system_records`' own wording).
- **Positive evidence:** `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText`,
  `TestToolingFixtureCarriesASystemRemindersToolResult` pass; the tooling fixture's third result (recorded
  on a system reminder) reached the native run's answer.
- **Pinned by:** `TestCodexRolloutWritesASystemRecordsToolEvidenceWithoutItsText` (round 4's rule, through a
  hand-built document and the committed fixture's real parse).
- **Executed mutation:** `codex_target.go:459` (the branch's `b.writeToolEvidence(message)`) → `_ = message`.
  **Observed:** exit `1`; `FAIL`; `codex_target_test.go:331: the recorded output appears 0 time(s) in the
  rollout, want exactly once, paired with its call`. Restored `f43564af…`.

### B9 — Dialogue is the reader's own projection, never the rendered structured payloads

- **Clause:** a message that carries tool calls contributes only its dialogue projection as text: the
  reader's `Content` renders the calls (`[Read: <path>]`), and writing both would duplicate the call in the
  target's history.
- **Discharged:** `internal/ccc/codex_target.go:1066-1085` (`dialogueText`, the tool-call rule).
- **Positive evidence:** `TestCodexRolloutKeepsTheDialogueProjection` passes.
- **Pinned by:** `TestCodexRolloutKeepsTheDialogueProjection`.
- **Executed mutation:** `codex_target.go:1070` `if len(message.ToolCalls) > 0 {` → `if false {`.
  **Observed:** exit `1`; `FAIL`; `codex_target_test.go:205: dialogue items = ["Ask" "And now the answer."
  "[Bash: list files]"], want the two recorded texts and no rendering of the calls`. Restored `f43564af…`.

## Surface C — structural verification

### C1 — The structural check reads the file it was handed, never a same-id copy

- **Clause:** the round-trip read-back is the exact installed path the check was given, read through the
  inherited reader's path-reference resolution; it never resolves the session id through the target's
  sessions tree, whose lookup walks every dated directory and prefers the lexically first copy of an id
  (round 12, finding 2).
- **Discharged:** `internal/ccc/codex_install.go:205-232` (`VerifyCodexRolloutStructure`'s `TargetPath` and
  `Ref{ID: verification.TargetPath}`), `:266-275` (the path check of the installed bytes).
- **Positive evidence:** `TestStructureVerificationReadsTheInstalledPathItWasGiven` passes; in the native
  run the reported structural check was `verified (the inherited AgentsView Codex reader over the installed
  session)`.
- **Pinned by:** `TestStructureVerificationReadsTheInstalledPathItWasGiven` (a same-id copy elsewhere in
  the home must not decide what is compared).
- **Executed mutation:** `codex_install.go:226` `Ref{Provider: string(parser.AgentCodex), ID:
  verification.TargetPath})` → `ID: rollout.SessionID})`. **Observed:** exit `1`; `FAIL`;
  `continue_test.go:1053: the check read something other than the installed file: {State:not-verified …
  dialogue 2 is in the file but was not written …}`. Restored `0a26ebd5…`.

### C2 — The installed file holds both the model-visible record and the harness's own view record

- **Clause:** every written dialogue message is written twice: a `response_item` (the record the target's
  model history is built from) and an `event_msg` of the target's own kind carrying the same text (the
  record its own view of the turn is built from). Measured on the tested version: a rollout holding only
  the response items loads and resumes, while the harness's own view of that session shows no turn — so
  "installed" would be a claim about a file rather than about a session a caller can open.
- **Discharged:** `internal/ccc/codex_target.go:606-657` (`messageItem` writes both kinds, per role),
  `:206-209` (the record and event type names).
- **Positive evidence:** the native run's `the target's own view lists the converted turns — 5 item(s)` and
  its `thread/read` printout naming each `userMessage`/`agentMessage`; `TestCodexRolloutRecordShape` and
  the round-trip tests pass.
- **Pinned by (repaired — see § Repairs):**
  `TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage`, new in this close-out: for every written
  dialogue item the file holds exactly one model-visible record under that role's own block type *and*
  exactly one event record of the target's own kind with the same text, with the literal kind names
  (`user_message` / `agent_message`) asked of the file's own bytes rather than of this writer's constants.
- **Executed mutation:** `codex_target.go:208` `codexEventUserMessage = "user_message"` →
  `= "item_started"`. **Observed:** exit `1`; `FAIL`;
  `codex_target_test.go:1123: the file holds 0 user_message event(s) for "Ask", want exactly one: the
  harness's own view of the turn is built from that record …`. Restored `f43564af…`.
- **Audit repair (2026-11-16, finding 4 of the acceptance audit).** The assertion above asked the file's
  own bytes for the literal `user_message`/`agent_message` kinds but took the **envelope** kinds from this
  writer's own constants (`codexRecordResponseItem`, `codexRecordEventMsg`), so a rename on the writer's
  side moved the assertion with it — the drift the literals exist to catch, one level up. Both envelopes
  are now literals, taken from the native format as the *carried* reader spells it
  (`internal/parser/codex.go`'s `codexTypeResponseItem`/`codexTypeEventMsg`): `"response_item"` and
  `"event_msg"` (`internal/ccc/codex_target_test.go:1108` and `:1116`). The inherited dialogue read-back
  cannot answer this either way: it projects message items and ignores the event records, so nothing but
  this assertion holds the harness's own view to the native spelling.
- **Executed mutation (audit repair):** `codex_target.go:206` `codexRecordEventMsg = "event_msg"` →
  `= "event_msg_drifted"` (the writer's spelling drifts, so the file holds no event record the target
  reads while every constant-driven comparison still agrees). Command:
  `CGO_ENABLED=1 go test -tags fts5 -count=1 -run
  TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage ./internal/ccc/...`. **Observed:** exit `1`;
  `FAIL`; `codex_target_test.go:1126: the file holds 0 user_message event(s) for "Ask", want exactly one …`
  and the same for `agent_message`. Restored `f43564af…`.
- **Executed demonstration of the pre-repair shape (audit repair):** with the same drifted writer spelling
  *and* the assertion put back on `codexRecordEventMsg` — writer and assertion moved together, which is
  what the finding names — the case **passed** (`exit 0`): the drift was invisible to it. Restored
  `f43564af…` (writer) and `a0c02e63…` (the repaired test file).

### C3 — Ordered one-to-one matching, including repeated text

- **Clause:** the file's own dialogue records are read in the file's own order; one record answers for one
  written message; a record the file holds elsewhere is an out-of-order difference; a record no written
  message resolves is dialogue the document does not write; and a text the document writes twice needs the
  read-back to hold it twice.
- **Discharged:** `internal/ccc/codex_install.go:396-530` (`installedDialogueRecords`, `dialogueRecords`'
  queue and `consume`), `:595-706` (`compareDialogue`, the walk), `:707-763` (`readBackTexts`, the count).
- **Positive evidence:** `TestCodexRoundTripCountsARecordOncePerWrittenMessage`,
  `TestCodexRoundTripReportsDialogueRecordsOutOfOrder`, `TestCodexRoundTripAsksTheFileForItsOwnDialogue
  Record`, `TestStructureVerificationReportsDialogueRecordsOutOfOrder`,
  `TestStructureVerificationCountsAnOmittedEnvelopeBesideItsOwnTextOnce` pass.
- **Pinned by:** `TestCodexRoundTripCountsARecordOncePerWrittenMessage` (round 11, finding 2's rule).
- **Executed mutation:** `codex_install.go:468` `return len(records.pending[key]) > 0` → `return true` (a
  record answers any number of times). **Observed:** exit `1`; `FAIL:
  TestCodexRoundTripCountsARecordOncePerWrittenMessage`. Restored `0a26ebd5…`.

### C4 — The check compares rather than agrees

- **Clause:** a file that does not hold the evidence the document says it wrote is reported as a
  difference; the check's verdict is never `verified` for a report the file does not support.
- **Discharged:** `internal/ccc/codex_install.go:236-249` (the differences branch of
  `VerifyCodexRolloutStructure`), `:808-830` (`compareSequences`), `:841-865` (`describeItem` /
  `describeText`).
- **Positive evidence:** `TestStructureVerificationNamesWhatTheFileDoesNotHold` (mutated file →
  `not-verified`, differences named) and `TestContinueConvertsInstallsAndReadsBackTheFixture` (intact file
  → `verified`) both pass, which is the two-direction evidence this rule needs.
- **Pinned by:** `TestStructureVerificationNamesWhatTheFileDoesNotHold` (round: the check compares rather
  than agrees) and `TestCodexRoundTripReportNamesDifferencesWithoutTheirText`.
- **Executed mutation:** `codex_install.go:242` `if len(roundTrip.differences) > 0 {` → `if false {`.
  **Observed:** exit `1`; `FAIL`;
  `continue_test.go:939: the structure check accepted a report the file does not support`.
  Restored `0a26ebd5…`.

### C5 — A legitimate parser projection is counted as such, not as a loss

- **Clause:** text the inherited reader projects away although the file really holds it (a prompt opening
  with an injected-context envelope, in both its shapes) is counted in
  `verification.structure.not_read_back_verbatim` and warned about; a message the file does not hold stays
  the missing message it is; and the count is read from the read-back's own texts, one claim per message.
- **Discharged:** `internal/ccc/codex_install.go:531-594` (`dialogueMessage`), `:595-634` (the gated count
  in `compareDialogue`), `:670-706` (`pairReadBackMessage`), `internal/ccc/continue.go:562-583`
  (`structureWarnings`).
- **Positive evidence:** `TestCodexRoundTripCountsWhatTheReaderProjectsAway`,
  `TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage`,
  `TestContinueInstallsPromptsTheReaderReadsDifferently` (both projection shapes through native Claude
  transcripts) pass.
- **Pinned by:** `TestCodexRoundTripReportsAMessageTheFileOnlyHoldsAsNoMessage` (round 8's rule) and
  `TestCodexRoundTripCountsWhatTheReaderProjectsAway`.
- **Executed mutation:** `codex_install.go:617` (the `account` closure's final `return false`) → `return
  true`. **Observed:** exit `1`; `FAIL`;
  `codex_install_test.go:405: notReadBackVerbatim = 1, want 0: an empty block is no message`.
  Restored `0a26ebd5…`.

## Surface D — installation and outcomes

### D1 — A conversion writes a fresh identity that no id in the home holds

- **Clause:** the target session id is a fresh UUID v7, never the source's session id and never an id
  already present in the target home; the file name and the reported identity agree.
- **Discharged:** `internal/ccc/codex_target.go:230-250` (`ConvertToCodexRollout`'s identity),
  `:1149-1157` (the file name and dated directory), `internal/ccc/continue.go` (the rounded report).
- **Positive evidence:** `TestCodexRolloutGeneratesAFreshIdentity`,
  `TestContinueConvertsInstallsAndReadsBackTheFixture` pass; the native run reported
  `the target identity is new and is not the source's — 01a1e365-…, not one of the 1 id(s) already in
  the home`.
- **Pinned by:** `TestCodexRolloutGeneratesAFreshIdentity` (two conversions cannot share an id) and the
  native check's own identity comparison.
- **Executed mutation:** `codex_target.go:241` `if sessionID == "" {` → `if false {`. **Observed:** exit
  `1`; `FAIL`; `codex_target_test.go:744: two conversions produced the same id ""`. Restored `f43564af…`.

### D2 — Publication is exclusive: nothing existing is ever replaced

- **Clause:** a taken target name is refused with `ErrTargetExists` and the existing bytes keep their
  digest; publication is the exclusive hard link, and a filesystem without hard links is refused with
  `ErrNoExclusiveInstall` rather than published by a rename that could replace another installer's session.
- **Discharged:** `internal/ccc/codex_install.go:101-190` (the guard, the temp file, `linkSession`, the
  `ErrNoExclusiveInstall` branch), `:51` (`linkSession = os.Link`), `:57` (`removeFile`).
- **Positive evidence:** `TestInstallCodexRolloutNeverReplacesASession`,
  `TestInstallCodexRolloutRefusesAFilesystemWithoutHardLinks` pass; the native run reported
  `[…jsonl] [hard-link]` and `no pre-existing session in the target home changed`.
- **Pinned by:** those two tests (round 1..12: the exclusive-publication rule and its no-hard-link
  reversal, `Q-0002-12`).
- **Executed mutation (one, three edits — the installer made to publish by a replacing mechanism):**
  `codex_install.go:51` `var linkSession = os.Link` → `os.Rename`; `:57` `var removeFile = os.Remove` →
  a wrapper that ignores the missing temp file; `:107`/`:109` guard and else-branch neutralized
  (`os.Lstat(target); false {` / `} else if false {`). **Observed:** exit `1`; `FAIL`;
  `continue_test.go:451: installing over an existing session returned <nil>, want ErrTargetExists` (the
  second install replaced the first write). Restored `0a26ebd5…`.

### D3 — A source that moved while it was converted is reported as changed

- **Clause:** the source file's own digest is taken before the conversion and compared after it; different
  bytes answer `changed` with both digests, and the run warns in its own channel as well.
- **Discharged:** `internal/ccc/continue.go:749-766` (`sourceFileDigest`), `:796-807` (the comparison),
  `:810-819` (`sourceChangedWarnings`), `internal/ccc/codex_install.go` (the report field).
- **Positive evidence:** `TestSourceCheckComparesTheFileBeforeAndAfter`,
  `TestSourceChangedWarningsNameTheMove` pass.
- **Pinned by:** `TestSourceCheckComparesTheFileBeforeAndAfter`.
- **Executed mutation:** `continue.go:799` `if after != before {` → `if false {`. **Observed:** exit `1`;
  `FAIL`; `continue_test.go:1640: a source that changed during the conversion was reported as "unchanged",
  want "changed"`. Restored `e5be4e8a…`.

### D4 — The source tree and every pre-existing session are compared *after* the native stages

- **Clause:** the fixture tree, the planted unrelated session and every session the home already held are
  digested before the run and compared after the loader and the real turn have run, so a mutation during
  either native stage is caught by the comparison that runs last; the planted file is removed only once
  those comparisons have run.
- **Discharged:** `scripts/codex-native-check.py:669-690` (the captures), `:697-742` (the comparisons and
  their placement after the load/resume blocks), `:748-753` (the planted file removed in `finally`).
- **Positive evidence:** the native run's own three lines —
  `the source session is unchanged — sha256 2bd510821d4abf83…`, `the unrelated session in the target home is
  unchanged — sha256 59ff49ca30baf622…`, `no pre-existing session in the target home changed — 0
  pre-existing session file(s)` — and the round 6 evidence record's mutation runs.
- **Pinned by:** `scripts/tests/test_codex_native_check.py`'s `ProtectedArtifactsAfterNativeVerification`
  (both directions: an intact run passes both comparisons, a run whose source tree is mutated during the
  resume stage fails exactly those two).
- **Executed mutation:** `scripts/codex-native-check.py:716` `planted_unchanged = unrelated.is_file() and
  sha256(unrelated) == unrelated_before` → `planted_unchanged = unrelated.is_file()`. Command:
  `python3 -m unittest scripts.tests.test_codex_native_check.ProtectedArtifactsAfterNativeVerification`.
  **Observed:** exit `1`; `FAIL:
  test_a_mutation_during_the_resume_stage_is_caught_by_the_last_comparison`;
  `AssertionError: Lists differ: ['the…changed'] != ['the…changed', 'the unrelated session in the target
  home is unchanged']`. Restored `7cf87ee9…`.

### D5 — A failure after publication keeps the report it reached

- **Clause:** a run that fails after the session is published still answers with the conversion it
  completed, the installation and the target identity, and every check that did not run is reported
  `not-verified` with the reason; the error accompanies the report instead of replacing it.
- **Discharged:** `internal/ccc/continue.go:598-641` (`reportAfterFailure`), `:390-402` (the installation
  block that exists from publication on), and the installer's two-answer contract
  (`internal/ccc/codex_install.go:101-190`).
- **Positive evidence:** `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall`,
  `TestContinueKeepsTheReportWhenTheTemporaryFileCannotBeRemoved`,
  `TestContinueKeepsTheConversionReportWhenTheHomeCannotBeCreated`,
  `TestContinueKeepsTheDryRunReportWhenItsSourceCheckCannotRun` (round 5) pass.
- **Pinned by:** `TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall` (round 5, finding 1).
- **Executed mutation:** `continue.go:636` `return result, err` → `return ContinueResult{}, err`.
  **Observed:** exit `1`; `FAIL`;
  `continue_test.go:1838: conversion = {State: … Carried:{Messages:0 …}}, want the conversion the run
  completed, with its counts`. Restored `e5be4e8a…`.

### D6 — Converted / installed / verified are separate states, and the exit code follows them

- **Clause:** the three states are reported separately and machine-readably; a requested verification that
  ran and failed exits `1` (with its document written), a verification that could not run does not fail the
  conversion, usage errors exit `2`, and the human block names the resume command.
- **Discharged:** `cmd/ccc/main.go:505-535` (`continuationExit`), `:482-494` (the document and the
  JSON/human split), `internal/ccc/continue.go:63-100` (`CheckVerifyOptions`) and `:412-556` (the stage
  states).
- **Positive evidence:** `TestContinueJSONDistinguishesConversionInstallationAndVerification`,
  `TestContinueReportsAFailedVerificationAsAFailure`, `TestContinueUsageErrors`,
  `TestContinueHumanOutputNamesTheResumeCommand`, `TestContinueWritesTheReportOfAFailureAfterPublication`,
  `TestContinueHumanOutputReportsAPrePublicationFailure` pass.
- **Pinned by:** `TestContinueReportsAFailedVerificationAsAFailure` (a verification that ran and failed is
  a failure, while an absent harness is not).
- **Executed mutation:** `cmd/ccc/main.go:506` (first statement of `continuationExit`) → inserted
  `return exitOK`. **Observed:** exit `1`; `FAIL`;
  `main_test.go:1305: exit 0, want 1 for a verification that ran and failed (stderr "")`. Restored
  `76dbd342…`.

### D7 — The target data directory is resolved, never invented

- **Clause:** `--target-home` wins and may be created; `$CODEX_HOME` is used when the caller names nothing,
  and a missing directory nobody named is an error (`ErrTargetHomeMissing`) rather than a directory
  call/ccc creates for the caller; the printed resume command selects that directory.
- **Discharged:** `internal/ccc/codex_install.go:62-100` (`ResolveCodexHome`), `internal/ccc/continue.go
  :250-264` (the shape check and the create-only-when-explicit rule).
- **Positive evidence:** `TestContinueTargetHomeResolution`, `TestContinueRefusesAMissingDataDirectory
  NobodyNamed`, `TestContinuePrintsAResumeCommandThatSelectsTheInstallationHome` pass.
- **Pinned by:** `TestContinueTargetHomeResolution`.
- **Executed mutation:** `codex_install.go:70` `if fromEnv := strings.TrimSpace(os.Getenv("CODEX_HOME"));
  fromEnv != "" {` → `if fromEnv := ""; fromEnv != "" {`. **Observed:** exit `1`; `FAIL`;
  `continue_test.go:611: error = <nil>, want ErrTargetHomeMissing for a data directory nobody named`.
  Restored `0a26ebd5…`.

### D8 — A dry run touches no target directory and reports its own states

- **Clause:** `--dry-run` converts and verifies against a throw-away copy, creates no directory for a home
  the caller named but that is not there, never starts the target harness against its data directory, and
  reports the levels that could not run as `not-verified` with the dry-run reason.
- **Discharged:** `internal/ccc/continue.go:388-420` (the dry-run branch: scratch install, structure over
  the copy, the level states), `:300-320` (the provider probe skipped with its reason, `Q-0002-11`).
- **Positive evidence:** `TestContinueDryRunWritesNothing`, `TestContinueDryRunReportsWhatItVerified`,
  `TestContinueDryRunInstallsNothingThroughTheCLI` pass (the third reads the stub harness's marker file to
  prove it was never started).
- **Pinned by:** `TestContinueDryRunWritesNothing`.
- **Executed mutation:** `continue.go:404` `InstallCodexRollout(rollout, scratch)` →
  `InstallCodexRollout(rollout, home)`. **Observed:** exit `1`; `FAIL`;
  `continue_test.go:325: a dry run wrote into the target home: 0 entries before, 1 after`. Restored
  `e5be4e8a…`.

## Surface E — native verification

### E1 — The load check is the target's own loader, and the answer must be this session's

- **Clause:** `load` runs the target harness's own loader (`codex app-server` → `thread/resume`, the
  request the interactive resume path uses) and requires the answered thread's id **and** path to be this
  conversion's session; a different id, a missing path or another file is `not-verified` with the harness's
  own reason — never a pass, and never a pass for a loader that merely echoed an identity.
- **Discharged:** `internal/ccc/codex_verify.go:218-300` (`VerifyCodexLoad`: `:256` the id check, `:262-270`
  the path checks), `:469-560` (the bounded app-server client).
- **Positive evidence:** the native run — `PASS: the target harness's own loader opens the installed
  session — thread/resume -> 01a1e365-… [legacy] cwd=/home/user/code/example-api` — and the same run's
  `PASS: the target's own view lists the converted turns — 5 item(s)`.
- **Pinned by (repaired — see § Repairs):** `TestCodexLoadVerificationRequiresTheSessionsOwnRecord`, new in
  this close-out, three cases: a loader answering with the session's own id and path verifies (exit 0, the
  reported path is the installed file); one answering with another id is `not-verified` + exit 1; one
  answering with this id but another file is `not-verified` + exit 1. Before this, the rule's only exercise
  was the non-gate native harness.
- **Executed mutations (two):**
  1. `codex_verify.go:256` `if loadedID != opts.SessionID {` → `if false {`. **Observed:** exit `1`;
     `FAIL`; `main_test.go:1725: exit 0, want 1 for a loader that loaded another session (stderr "")`.
  2. `codex_verify.go:270` `if opts.ExpectedPath != "" && !sameFile(loadedPath, opts.ExpectedPath) {` →
     `if false {`. **Observed:** exit `1`; `FAIL`;
     `main_test.go:1746: exit 0, want 1 for a loader that named another file (stderr "")`.
  Restored `b79031c0…` after each.
- **Audit repair (2026-11-16, finding 2 of the acceptance audit).** The positive case's exit status is now
  the run's own: `code, stdout, stderr := runCLI(…)` and `continueDocumentOf(t, code, stdout, stderr)`
  (the constant `exitOK` is what the helper compares against, never what it is handed). Before the repair
  the helper was passed `exitOK` itself, so "the run exited 0" agreed with the case's own argument — a
  tautology a failing command could not break. Re-run on the repaired tree:
  `TestCodexLoadVerificationRequiresTheSessionsOwnRecord` and `TestProviderBindingIsTheTargetsOwnAnswer`
  (E4) both PASS with the run's own exit code in the assertion, and the mutation that detects the old
  shape is recorded in E2's row (one temporary edit, its own case).

### E2 — "Natively resumable" is a real turn that appended to the installed session

- **Clause:** `resume` runs one non-interactive turn in the installed session and reports its command, exit
  code and the session file's growth; a turn that answered without appending the session is
  `not-verified` (a harness that answered a question has not shown the session can continue), and no prompt
  is ever invented (`--verify resume` requires `--resume-prompt`).
- **Discharged:** `internal/ccc/codex_verify.go:314-384` (`VerifyCodexResume`: `:371` the growth rule),
  `:402-438` (`runCodexTurn`), `internal/ccc/continue.go:63-100` (the prompt requirement).
- **Positive evidence:** the native run — `PASS: a real turn continues from the installed session — exit 0,
  session 3970 -> 67991 bytes` — plus `TestContinueRequiresAPromptBeforeItRunsATurn` for the prompt rule.
- **Pinned by (repaired — see § Repairs):**
  `TestResumeVerificationRequiresTheTurnToAppendToTheSession`, new in this close-out, two cases: a turn that
  appends verifies (exit 0, `session_bytes_after > session_bytes_before`); a turn that answers without
  appending is `not-verified` + exit 1 with "did not grow" in the detail.
- **Executed mutation:** `codex_verify.go:371` `if after <= before {` → `if false {`. **Observed:** exit
  `1`; `FAIL`; `main_test.go:1798: resume = map[… session_bytes_after:3099 session_bytes_before:3099
  state:verified], want not-verified when the session did not grow`. Restored `b79031c0…`.
- **Audit repair (2026-11-16, finding 2 of the acceptance audit).** Two defects in this case, one of which
  hid the other. (a) The positive case handed `continueDocumentOf` the constant `exitOK` instead of the
  status `runCLI` answered, so the helper's "the run exited 0" check compared the case's own argument with
  itself. (b) The case left the stub loader's mode unset, and a resume verification loads the session
  *before* it runs the turn — so the run it called a success had in fact failed: the loader refused
  `thread/resume`, the load block came back `not-verified`, and `continuationExit` answered `1`. The
  positive case now reads the run's own exit code and sets the loader's mode it needs
  (`stubLoadModeEnv = stubLoadSelfMode`, `main_test.go:1774`), which makes it the genuinely successful run
  it claims; the negative half sets the same mode, so it can fail only on the turn that appended nothing.
- **Executed mutation (audit repair):** the repaired case with the loader's mode removed again (the
  pre-repair state) — `t.Setenv(stubLoadModeEnv, stubLoadSelfMode)` deleted. Command:
  `CGO_ENABLED=1 go test -tags fts5 -count=1 -run TestResumeVerificationRequiresTheTurnToAppendToTheSession
  ./cmd/ccc/...`. **Observed:** exit `1`; `FAIL`;
  `main_test.go:1778: exit 1, want 0 (stderr "")` — the run's real status, which the pre-repair case could
  not see because it was not looking at it. Restored `0ea5446a…`.
- **Executed demonstration of the pre-repair shape (audit repair):** with the same missing loader mode
  *and* the assertion put back on the constant (`continueDocumentOf(t, exitOK, …)`, plus a temporary
  `t.Logf` of the status that was being discarded), the case **passed** (`exit 0`) while it logged
  `the CLI's real exit code: 1`. That is the tautology the finding names, measured: the command failed and
  the test agreed with itself. Restored `0ea5446a…`.

### E3 — The turn runs the executable the caller's path resolved to

- **Clause:** a relative `--target-binary` is resolved before the turn changes into the session's working
  directory, so the harness the probe asked for its version is the harness that runs (round 3, finding 3).
- **Discharged:** `internal/ccc/codex_verify.go:439-467` (`resolveCodexExecutable`), `:402-438`
  (`runCodexTurn` sets `Dir` only after resolving).
- **Positive evidence:** `TestRunCodexTurnResolvesARelativeExecutableBeforeChangingDirectory` passes.
- **Pinned by:** that test.
- **Executed mutation:** `codex_verify.go:444` `if filepath.IsAbs(path) {` → `if true {` and `:447`
  `return filepath.Abs(path)` → `return path, nil`. **Observed:** exit `1`; `FAIL`;
  `codex_verify_test.go:41: the turn did not run the harness the caller's relative path names (fork/exec
  ./ccc.test: no such file or directory)`. Restored `b79031c0…`.

### E4 — The recorded provider is the target's own answer, asked of the harness

- **Clause:** the provider written into `session_meta` is what the target harness itself answers a new
  session would use (`thread/start` on an ephemeral thread, whose answer names the *effective* provider;
  `config/read` as the fallback), never the source's provider; the report names which answer it was
  (`target-thread-defaults` / `target-config` / `unrecorded`), and a dry run records none with that reason.
- **Discharged:** `internal/ccc/codex_verify.go:100-165` (`ProbeCodexConfig`), `internal/ccc/continue.go
  :296-318` (the probe call and `providerSource`), `internal/ccc/codex_target.go:390-412`
  (`sessionMeta`'s `ModelProvider`), `:1018-1027` (`provider_binding`).
- **Positive evidence:** the native run — `provider=tencent-litellm` from a session whose recorded provider
  came from the harness's own answer (`model_provider "tencent-litellm" (source "target-thread-defaults")`
  in the round-2 record) — and the offline `TestCodexRolloutRecordsTheProviderBindingItIsGiven` for the
  writer's half.
- **Pinned by (repaired — see § Repairs):** `TestProviderBindingIsTheTargetsOwnAnswer`, new in this
  close-out, two cases: a harness answering the thread defaults writes `a-stub-provider` into
  `target_session.model_provider`, `model_provider_source: target-thread-defaults` **and** into the
  installed `session_meta`; a harness that refuses `thread/start` but names the provider in its
  configuration answers `target-config`.
- **Executed mutation:** `codex_verify.go:140` `…; err == nil {` → `…; err == nil && false {` (the thread
  defaults' answer is never used). **Observed:** exit `1`; `FAIL`;
  `main_test.go:1830: model_provider = <nil>, want the harness's own answer`. Restored `b79031c0…`.
- **Audit repair (2026-11-16, finding 2 of the acceptance audit).** Both of this row's cases now pass the
  run's own exit code to `continueDocumentOf` (the constant `exitOK` is what the helper compares against),
  so the "exit 0" half of each positive case is the command's answer rather than the case's own argument.
  Re-run on the repaired tree: `TestProviderBindingIsTheTargetsOwnAnswer` PASSes with `code` wired into the
  assertion; the mutation that detects the old shape (one temporary edit) is recorded in E2's row, which is
  the case the audit cited.

### E5 — The reported invocation is the shell's reading of the command that ran

- **Clause:** the printed resume/exec-resume commands select the installation's data directory
  (`CODEX_HOME=…`), quote every word a POSIX shell would act on, and the resume verification reports the
  command it actually ran the same way; the native check reads those lines the way a shell does, so a home
  holding a space (or a Windows path with backslashes) is accepted while an unquoted home a shell would
  split is rejected.
- **Discharged:** `internal/ccc/continue.go:853-880` (`renderResumeCommand`, `shellQuote`),
  `internal/ccc/codex_verify.go:385-401` (`resumeCommandLine`), `scripts/codex-native-check.py:310-334`
  (`shell_words`, `reported_invocation`).
- **Positive evidence:** the native run's `PASS: the reported resume command selects the installation home`
  (a home whose rendered line was read back as words); `TestContinuePrintsAResumeCommandThatSelectsTheInstal
  lationHome`, `TestResumeVerificationCommandNamesTheHomeItRanAgainst`,
  `TestContinueHumanOutputNamesTheResumeCommand` pass; `ResumeInvocationCheck` (both `pathlib` flavours)
  passes.
- **Pinned by:** `TestResumeVerificationCommandNamesTheHomeItRanAgainst` (the renderer) and
  `ResumeInvocationCheck` (the reader, both directions: correct renderings accepted, split/absent/other-home
  renderings rejected).
- **Executed mutations (two, one per side):**
  1. `continue.go:873` `if word != "" && !strings.ContainsAny(word, shellUnsafeCharacters) {` → `if true {`
     (never quote). **Observed:** exit `1`; `FAIL`;
     `continue_test.go:895: resume command line = …`. Restored `e5be4e8a…`.
  2. `codex-native-check.py:334` `return shell_words(command) == [f"CODEX_HOME={home}", *arguments]` →
     `return shell_words(command) is not None`. **Observed:** exit `1`;
     `FAIL: test_rejects_a_command_for_another_session` for both `Path` and `PureWindowsPath`;
     `AssertionError: True is not false : CODEX_HOME=codex-home codex resume 00000000-…`. Restored
     `7cf87ee9…`.

### E6 — Subprocesses are bounded: no hang, no unbounded diagnostics

- **Clause:** one app-server request is bounded by its own deadline and the server's diagnostics are kept
  in a bounded, synchronized buffer, so a protocol that stalls or floods is reported as a failed check
  rather than blocking or exhausting the run; the native check bounds its own requests the same way
  (`APP_SERVER_TIMEOUT`).
- **Discharged:** `internal/ccc/codex_verify.go:554-611` (`call` with the per-request timeout),
  `:612-633` (`stderrExcerpt`), `:634-654` (`boundedBuffer`), `scripts/codex-native-check.py:85-142`
  (`APP_SERVER_TIMEOUT`, `last_meaningful_line`).
- **Positive evidence:** `TestBoundedBufferKeepsTheFirstBytes`,
  `TestBoundedBufferIsSafeForConcurrentWritesAndSnapshots` (the case `go test -race` diagnoses) pass; the
  native run completed inside its own deadlines on a host whose app server prints startup chatter.
- **Pinned by:** `TestBoundedBufferKeepsTheFirstBytes` (the buffer's bound) with the round-7 evidence
  record's stalled-stub-server measurement for the timeout half.
- **Executed mutation:** `codex_verify.go:639` `if remaining := b.max - b.data.Len(); remaining > 0 {` →
  `if remaining := b.max; remaining > 0 {`. **Observed:** exit `1`; `FAIL`;
  `codex_verify_test.go:59: snapshot = "abcdefgh", want the first 4 bytes`. Restored `b79031c0…`.
- **Audit repair (2026-11-16, finding 3 of the acceptance audit).** This clause's second half was
  discharged for the Go client's `boundedBuffer` only. `scripts/codex-native-check.py`'s stderr drainer
  appended every line the app server wrote to an unbounded `list`, and the `[:400]` in `stderr_tail` bounds
  what a *report* shows, never what the harness retains — a server that logs for the whole run (or floods
  its diagnostics pipe, which is the drainer's other job) would grow the harness's memory for the length of
  the run. The drainer now writes into a ring: `APP_SERVER_DIAGNOSTICS_MAX` (`:87-92`),
  `new_diagnostics`/`drain_diagnostics` (`:354-372`), the sink and the drainer (`:396`, `:413-418`), with
  `stderr_tail` reading the ring's tail (`:431-434`). Presentation is unchanged (the last `limit` lines,
  cut to 400 characters), and no credential or conversation-content rule is touched: the sink holds the
  target server's own diagnostics, which F1's seeding rule never writes a credential into.
- **Pinned by (audit repair):** `scripts/tests/test_codex_native_check.py`'s `BoundedAppServerDiagnostics`
  (`:156-198`), three cases through the shipped sink and drainer: a 5000-line stream leaves exactly
  `APP_SERVER_DIAGNOSTICS_MAX` lines and they are the stream's **last** ones; a stream that fails mid-read
  keeps the lines that arrived; the reported tail is the end of what was read.
- **Executed mutation (audit repair):** `codex-native-check.py:360`
  `return collections.deque(maxlen=APP_SERVER_DIAGNOSTICS_MAX)` → `return collections.deque()` (the bound
  removed). Command: `python3 -m unittest
  scripts.tests.test_codex_native_check.BoundedAppServerDiagnostics`. **Observed:** exit `1`; `FAIL:
  test_retains_only_the_bounded_tail`; `AssertionError: 5000 not less than or equal to 1000 : the drainer
  retained 5000 lines of a 5000-line stream: the diagnostics are a tail, not a log`. Restored `7d759175…`.

## Surface F — privacy and test integrity

### F1 — No credential-bearing setting is copied into, or quoted by, the throw-away home

- **Clause:** the native check's throw-away Codex home is seeded setting by setting from the host's
  configuration — never by copying the configuration document — with only settings whose values cannot hold
  a credential (the model settings and a provider's name/endpoint/wire protocol/*names* of the environment
  variables a credential comes from); a provider whose base URL holds userinfo, a query or a fragment is
  left behind whole; the run's `seeded from:` line names both halves without quoting a value.
- **Discharged:** `scripts/codex-native-check.py:93-107` (`SEEDED_TOP_LEVEL`, `SEEDED_PROVIDER_SETTINGS`),
  `:174-186` (`credential_free_url`), `:223-307` (`seed_configuration`), `:508-527` (the report line).
- **Positive evidence:** the native run's own line, which names what was copied and what was left behind
  (`copied model, model_provider, [model_providers.tencent-litellm].name, …; not copied …:
  model_reasoning_effort, …, [projects], [features], [tui]`), and
  `SeededHostConfiguration` (five cases) passing.
- **Pinned by:** `scripts/tests/test_codex_native_check.py`'s `SeededHostConfiguration` (round 12,
  finding 1: the throw-away home is seeded, not cloned).
- **Executed mutation:** `scripts/codex-native-check.py:175` (first line of `credential_free_url`'s body)
  prepended `return True`. **Observed:** exit `1`;
  `FAIL: test_does_not_copy_a_provider_whose_url_holds_a_credential`;
  `AssertionError: 'model_providers' unexpectedly found in {… 'base_url':
  'https://a-fixture-url-user:a-fixture-url-password@proxy.example/v1' …}`. Restored `7cf87ee9…`.
- **Measured limit (a probe that did **not** detect, recorded rather than dropped):** adding
  `"http_headers"` to `SEEDED_PROVIDER_SETTINGS` left `SeededHostConfiguration` green, because the
  fixture's `http_headers` is a TOML table and the seeding loop copies only string values while naming the
  table in `not_copied`. The real guard for a credential written into a configuration remains
  `credential_free_url` (a query parameter is a credential too) plus the exclusion of the
  header/`env`/`auth` tables; the string-shaped `http_headers` case is not a shape the target's own
  configuration schema defines.

### F2 — No report or CLI document carries source conversation content

- **Clause:** the conversion report and the CLI's wire document (human and `--json`, installed and dry run)
  carry counts, states and digests — never dialogue, tool arguments or tool output; a difference report
  identifies an item by size and SHA-256 instead of quoting it.
- **Discharged:** `internal/ccc/codex_install.go:841-865` (`describeItem`/`describeText`),
  `internal/ccc/continue.go:219-231` (`conversionSourceReport`), `cmd/ccc/main.go:482-494` (the document),
  `:536-612` (the human block).
- **Positive evidence:** `TestContinueReportCarriesNoSourceContent`, `TestContinueJSONCarriesNoSourceContent`
  (round 7, finding 3), `TestCodexRoundTripReportNamesDifferencesWithoutTheirText` (round 12),
  `TestStructureVerificationAsksTheFileForTheMessageRecord`'s last assertion (the difference report does not
  contain the text it could not match) pass.
- **Pinned by:** `TestContinueJSONCarriesNoSourceContent` and
  `TestCodexRoundTripReportNamesDifferencesWithoutTheirText`.
- **Executed mutation:** `codex_install.go:845` `fmt.Sprintf("%s (%s)", identity, describeText(rest))` →
  `fmt.Sprintf("%s (%s)", identity, rest)`. **Observed:** exit `1`; `FAIL`;
  `codex_install_test.go:55: the difference report carries the source content it compared:
  "summarize the retry policy in internal/retry/policy.go"`. Restored `0a26ebd5…`.

### F3 — The committed fixtures carry no host path and no credential

- **Clause:** every committed fixture is sanitized: no absolute host path, no credential shape; the check
  fails naming the file and the offending string.
- **Discharged:** `internal/ccc/testdata/fixtures/**` (the committed bytes) and the scan in
  `internal/ccc/fixtures_test.go` (`TestFixturesCarryNoHostPathsOrSecrets`), with the generator that
  produced them (`internal/ccc/fixturegen`).
- **Positive evidence:** `TestFixturesCarryNoHostPathsOrSecrets` and `TestFixturesMatchTheirGenerator` pass;
  the gate's `secrets` step found no leaks in the 74 project-owned files.
- **Pinned by:** `TestFixturesCarryNoHostPathsOrSecrets`.
- **Executed mutation (altered fixture):** appended one line to
  `internal/ccc/testdata/fixtures/claude/-home-user-code-tooling/b6d93f07-2f5c-4a11-8e0d-3c7a91d4b2ef.jsonl`
  whose content named the repository root as an absolute path below the builder's home (written `<home>` in
  this file's own convention, as every host path here is). **Observed:** exit `1`; `FAIL`;
  `fixtures_test.go:651: claude/-home-user-code-tooling/b6d93f07-…jsonl contains "/home/<user>"` (the
  builder's home directory's first segment, as the scan itself spells it).
  Restored `415bbc31a6cd541f9bc1ff2b99dc4532abe81f218e14dc5c97c7399f3b212873` (the digest the evidence
  record also reports for that fixture).

### F4 — The harness's tests hold on every host, and the suite proves it from outside

- **Clause:** an assertion under `scripts/tests/` names a rule and fails when the rule is broken, on every
  platform and Python the docs name: a case's stubs answer the host probes the case is not about, no case
  depends on a tool being installed, and the suite re-runs itself in a child whose `PATH` holds nothing and
  once with the platform probe answering Windows-without-a-compiler — so a host-dependent case fails on
  this host rather than in an unwatched CI job.
- **Discharged:** `scripts/tests/test_gate.py:150-232` (the per-case stub bin and pinned `PATH`),
  `:913-1113` (`HostCase`: the nested runs, the statuses, the allowed skips),
  `scripts/gate.py:139-160` (`go_env`'s Windows/C-compiler boundary).
- **Positive evidence:** the gate's `selftest` step — `Ran 158 tests in 5.600s (rc=0)`, which includes both
  nested runs on this host.
- **Pinned by:** `HostCase.test_suite_holds_in_a_child_whose_path_holds_no_tools` and
  `test_suite_holds_when_the_host_answers_windows_without_a_compiler` (with
  `test_the_stub_bin_answers_the_windows_compiler_probe` for the stub rule itself).
- **Executed mutation:** `scripts/tests/test_gate.py:824` (first line of
  `test_only_tracked_go_files_outside_vendor_reach_gofmt`'s body) prepended
  `subprocess.run(["git", "--version"], capture_output=True)` — a host tool a case did not stub.
  **Observed:** exit `1`;
  `FAIL: test_suite_holds_in_a_child_whose_path_holds_no_tools`;
  `AssertionError: 1 != 0 : nested run rc=1: Ran 158 tests in 1.829s | FAILED (errors=1, skipped=2) |
  broken: test_only_tracked_go_files_outside_vendor_reach_gofmt`. Restored `978c7a0a…`.
- **Audit repair (2026-11-16):** the suite now holds **161** cases — the three added with E6's repair
  (`BoundedAppServerDiagnostics`) — and both nested runs still hold: the gate's `selftest` step on the
  repaired tree printed `Ran 161 tests … (rc=0)`, which is the same two child runs (no tools on
  `PATH`, and the platform probe answering Windows-without-a-compiler) over the larger suite. The earlier
  `Ran 158 tests in 5.600s (rc=0)` line above and in the header is the close-out head's own measurement.

### F5 — A required gate step cannot pass by not running

- **Clause:** a step named in `scripts/gate-required.txt` (`parity`, `selftest`) must be PASS: any other
  status — FAIL, SKIP, BOUNDARY, N/A — is a failure of the gate, an unknown required name is a failure,
  and a run in which an optional step did not run says so in its verdict (`GATE: PASS (partial: …)`) so a
  partial run can never be quoted as a full one; `parity` itself exits `2` with `parity: INCOMPLETE` when
  git cannot enumerate the tree (round 2, finding 5's rule).
- **Discharged:** `scripts/gate.py:335-351` (`apply_required`), `:353-356` (unknown required names),
  `:365-373` (the verdict and partial disclosure), `scripts/upstream-parity.py`
  (the INCOMPLETE status, `Q-0001-09`).
- **Positive evidence:** the gate runs above: `parity` and `selftest` PASS in the verdict line with no
  `(partial: …)` qualifier; `test_gate.GateCase`/`GateProbeCase` and
  `test_upstream_parity`'s INCOMPLETE cases pass inside the `selftest` step.
- **Pinned by:** `GateCase.test_required_step_that_skips_fails_the_gate`,
  `test_required_step_reported_as_boundary_fails_the_gate`,
  `test_unknown_required_step_name_fails_the_gate`, and the parity script's own INCOMPLETE case.
- **Executed mutation:** `scripts/gate.py:345` `if name in required and status != PASS:` → `if name in
  required and status == PASS:`. **Observed:** exit `1`;
  `FAIL: test_required_step_that_skips_fails_the_gate`;
  `AssertionError: 0 != 1 : gate: <tmp>/gate-root-…` (the gate passed with a skipped required step).
  Restored `6b495a57…`.

---

## Repairs made by this close-out (Deliverable 2)

Three obligations were implemented but had **no gate-level pin**: their only exercise was the non-gate
native harness, so an audit of the offline half could not have proved them — the exact shape the close-out
exists to find. Each was repaired by adding the pin, and each new pin has its own executed mutation above
(C2, E1, E2, E4).

| Obligation | Was unpinned because | Pin added |
| --- | --- | --- |
| C2 — both record kinds (model-visible + the harness's own view) | the record-shape test pinned the response item only; the *view* was read only by `scripts/codex-native-check.py` (not a gate step) | `TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage` (internal/ccc), asking the file's own bytes for the literal `user_message`/`agent_message` kinds |
| E1 — the loader's answered id **and** path must be this session's | no offline test ran an app server at all; the only stub refused every request | the mode-aware stub harness in `cmd/ccc/main_test.go` + `TestCodexLoadVerificationRequiresTheSessionsOwnRecord` (self / other identity / other file) |
| E2 — "resumable" requires the turn to append to the session | no offline test ran a turn | `stubTurn` (grow / silent modes) + `TestResumeVerificationRequiresTheTurnToAppendToTheSession` |
| E4 — the provider is the harness's own answer, and it is recorded | `ProbeCodexConfig`'s answer had no test; only the writer's half was pinned | `thread/start` + `config/read` stub modes + `TestProviderBindingIsTheTargetsOwnAnswer` |

The repair is additive: no existing assertion, evidence row, disclosed limit or gate rule was weakened,
removed or narrowed, and no carried AgentsView file was touched (`parity` proves it).

## Repairs made by the acceptance audit's repair (2026-11-16)

Four findings, four repairs; each row above carries its own mutation and each repair's named test ran
alone and passed before that mutation was applied.

| Finding | Row | Repair | Pin (and its mutation) |
| --- | --- | --- | --- |
| 1 — `continue.go:677`, a source I/O failure bypassed the archive fallback | A2, A3 | `reader.go`: a failed `Fingerprint`/`Parse` travels as `ErrProviderFailed`, the sentinel the accepted set already holds; `continue.go`'s set documents why it is the gate | `TestContinueFallsBackToTheArchiveWhenTheSourceCannotBeRead` (three cases) + `TestUnreadableSourceFallsBackToTheArchive` (the inspection's same sentinel) — both wraps reverted, one at a time, each run failing |
| 2 — `main_test.go:1771`, the positive cases passed a constant `exitOK`, and the resume case's load was unset | E1, E2, E4 | every positive case reads the run's own exit code; the resume case sets the loader's mode it needs, and its negative half sets it too | `TestResumeVerificationRequiresTheTurnToAppendToTheSession` — the mode removed again fails as `exit 1, want 0`; the pre-repair shape passed while the CLI exited `1` |
| 3 — `codex-native-check.py:388`, diagnostics retained without a memory bound | E6 | the drainer writes into a ring (`APP_SERVER_DIAGNOSTICS_MAX`, `new_diagnostics`/`drain_diagnostics`), presentation unchanged | `BoundedAppServerDiagnostics` — the `maxlen` removed retains 5000 of 5000 lines |
| 4 — `codex_target_test.go:1113`, the envelope assertion shared the writer's constant | C2 | both envelope spellings are literals taken from the carried reader (`response_item`, `event_msg`) | `TestCodexRolloutWritesTheTargetsOwnViewRecordForEachMessage` — a drifted writer spelling fails it; sharing the constant again hides the same drift |
| — (same class, disclosed not found) `codex_target_test.go:128-135`, the record-*kind* list also read the writer's constants | C2 | the list is now the carried reader's own spellings as literals, with the source named in a comment; the operator repaired this one itself | `TestCodexRolloutRecordShape` — a drifted `codexRecordEventMsg` fails it with `record types = […want…]`; the pre-repair form moved with the writer and stayed green (the lane's disclosure, verified by an executed drift mutation and a byte-identical restore) |

## What this matrix does not claim

- The interactive `codex resume` TUI is still unverified on this host (no TTY); its loader
  (`thread/resume`) is what `load` exercises (round-1 limit, unchanged).
- The app-server protocol is experimental and version-bound: the verified version is recorded on every run
  and in `CodexFormatVersion`; a protocol change turns `load` into `not-verified` with the harness's own
  message rather than a pass.
- A filesystem without hard links cannot receive a converted session (`ErrNoExclusiveInstall`); recorded as
  `Q-0002-12`.
- The recorded provider binds the session to the target's configuration at conversion time (`Q-0002-01`).
- The native half of surface E (a real turn through the target harness) needs the harness and model access
  and is therefore not a gate step: the evidence for it in this file is one real run, not a gate verdict,
  and the offline pins above are what the gate decides.
- **Residual, disclosed by the audit repair (2026-11-16).** `internal/ccc/codex_target_test.go`'s
  record-*order* assertion (`:129-136`) still lists the file's record kinds through this writer's constants
  (`codexRecordSessionMeta`, `codexRecordResponseItem`, `codexRecordEventMsg`, …), which the audit did not
  name; it is about the order and the pairing of those records, and the spellings it shares with the writer
  are now pinned independently by C2's repaired assertion, so a writer rename fails C2 rather than passing
  both. It was left as it stands rather than widened beyond the finding.
