# Evidence — 0003 obligation matrix (Codex → Claude Code)

Contract: [`docs/goals/0003-codex-to-claude-continuation.md`](../goals/0003-codex-to-claude-continuation.md).
Evidence record: [`0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md) (cited below
as *record §N*). Plan: [`docs/plans/0003-codex-to-claude-continuation.md`](../plans/0003-codex-to-claude-continuation.md).

## How the rows below were produced

Every `Done when` clause and every `Bounds` sentence of the goal file has a row. A row is discharged only
by something this tree can re-run: a command with its exit code (the gate, the native check, a `go test`
selector), a file whose bytes hold the claim (a fixture, the installed transcript), or a harness's own
output quoted in the record. A row that names a test names it by its full name, and the test's own rule
is what the row claims — not a summary of it.

This matrix was produced in the **implement** stage of the increment: it records clause-by-clause
discharge, and it does *not* include the per-clause mutation sweep that 0002's matrix accumulated during
its acceptance rounds (each row there names an executed mutation and its restore). Where a rule here is
asserted by a test rather than by a mutation, the row says so; the mutation sweep belongs to the review
stage that follows this one, and it began in the fix round that followed review round 2 — that round's
executed mutations and restores are recorded at the end of this file.

## `Done when` clauses

| # | Clause | Discharged by | Evidence | Pinned by |
| --- | --- | --- | --- | --- |
| D1 | *A single non-interactive command selects a Codex session and produces a new Claude Code-native resume identity and usable command. Demonstrate it on a representative session.* | The CLI converts the committed Codex fixture in one invocation and prints the new identity and both resume forms | record §1 (the run, its exit code and its output, including `resume:` and `non-interactive resume:`); record §2 (the `--json` document) | `cmd/ccc/claude_route_test.go`'s `TestContinueToClaudeConvertsInstallsAndReportsTheRoute`, `TestContinueToClaudeHumanOutputCarriesTheResumeLine`; `internal/ccc/continue_claude_test.go`'s `TestContinueIntoClaudeConvertsInstallsAndReadsBack`, `TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation`, `TestClaudeInvocationNamesTheExecutableItsProbeFound` (the printed forms name the program the probe found, resolved against the caller's directory, else the spelling given) |
| D2 | *Claude Code's native loader recognizes the installed session and accepts a continuing turn. Provide loader-level evidence.* | Four independent observations: the loader's own reconstructed request (loopback proxy), the offline `load` level with its unknown-id control arm, one real turn through the printed invocation, and the native-check script | record §3(a)–(d); record §6 (the script's run, check by check) | `internal/ccc/claude_verify_test.go`'s `TestVerifyClaudeLoadResolvesTheInstalledSessionAndItsControl`, `TestVerifyClaudeLoadRefusesAnAnswerAboutAnotherSession`, `TestVerifyClaudeResumeReportsTheTurnAndTheGrowth`, `TestVerifyClaudeResumeRefusesAPromptItWouldHaveToInvent`, `TestVerifyClaudeResumeRefusesWithoutABinary`; `scripts/tests/test_claude_native_check.py` |
| D3 | *Structured source messages and tool events translate through the shared conversation representation, with preserved ordering and explicit disclosure of material loss. Show an input/output semantic comparison, including at least one nontrivial tool interaction.* | The document (`Conversation`) feeds both writers through the same `Loss`/`Carried`/`TargetItem` vocabulary; the installed file is read back through the inherited reader and compared item by item; the loss list names every category with its count | record §5 (the source↔target table, the tool pair and the boundary included); record §2 (the carried counts and the non-zero categories of a real run) | `internal/ccc/claude_target_test.go`'s `TestConvertToClaudeTranscriptWritesTheMeasuredShape`, `TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers`, `TestClaudeWriterPairsAnIdlessResultOnlyWithSoleOutstandingCall`, `TestClaudeWriterCountsADiscardedSubagentSessionLink`, `TestConvertToClaudeTranscriptRefusesASourceWithoutAWorkingDirectory`; `claude_install_test.go`'s `TestVerifyClaudeTranscriptStructureRoundTripsAConversion`, `TestVerifyClaudeTranscriptStructureReportsEvidenceTheFileDoesNotHold` and `TestVerifyClaudeTranscriptStructureNamesTheClaudeReaderWhenItProjectsAway` |
| D4 | *The 0002 Claude-to-Codex behavior still passes its accepted verification, and the new route does not duplicate or diverge from AgentsView's source-side parsing implementation.* | 0002's behaviour re-verified by its tests inside the gate's `test` step (the route table refactor kept every pinned string, state and command; two lines that *named the newly supported target* were updated — the help case's `--to` spelling and the refusal case's target — and no assertion was weakened; record §6.2); the source side is still only the inherited reader — the new code reads no source format and the carry is untouched | record §6 (the gate's `test` step and the whole-suite counts); `parity` passes with no carried file changed (record §6) | every 0002 test in `internal/ccc/continue_test.go`, `codex_*_test.go`, `cmd/ccc/main_test.go`; `scripts/upstream-parity.py` (`parity` step); `internal/ccc/parity`'s oracle dump test |
| D5 | *The command's JSON and error behavior can be consumed without a skill or interactive prompt; neither original history nor unrelated target sessions are modified.* | `--json` writes one object whose three states are separate; the failures exit 1/2 with the reason on stderr and the report still on stdout; the source digest is compared around every run and the native check hashes the source copy, the planted unrelated session and every session the home already held **last** | record §2 (states and exit codes), §4 (the source check and the exclusive install), §3/§6 (the native check's comparisons) | `cmd/ccc/main_test.go` (the exit-code and document cases), `internal/ccc/claude_install_test.go`'s `TestInstallClaudeTranscriptIsExclusiveAndPrivate`, `internal/ccc/continue_claude_test.go`'s `TestContinueIntoClaudeDryRunWritesNothing` (the home is byte-identical afterwards) |

## `Bounds` sentences

| # | Bound | How it is held | Evidence |
| --- | --- | --- | --- |
| B1 | *Read Codex through the inherited AgentsView parser/archive semantics.* | The only source read is `SourceReader`/`Archive` over inherited `internal/parser`/`internal/db`; the new files import no source-format code and add no parser | `git diff --stat` of this increment touches no carried path; `parity` passes (record §6); the Carry manifest declares this increment's new project-owned scripts (`scripts/claude-native-check.py`, and the shared-mechanism module `scripts/native_check_common.py` the apply round added) |
| B2 | *Respect available Codex turn boundaries and tool evidence without treating stored model output as a new Claude system instruction or executing tools.* | A `compacted` boundary becomes the target's own boundary pair (measured to truncate the model-visible history); `turn_context` has no target record and is stated as a format limit; tool calls/results travel as recorded evidence in the target's own block shapes and a resumed turn reads them as history (nothing is replayed); no source text is written as a target system record | record §0 (the boundary measurement), §5 (the table's boundary and tool rows); `internal/ccc/claude_target.go`'s `claudeFormatLimits`; `TestConvertToClaudeTranscriptWritesTheMeasuredShape` |
| B3 | *Keep the original source and existing target sessions untouched.* | The source file is opened read-only and digested around the run; installation is exclusive (temp file + hard link, `ErrTargetExists`/`ErrNoExclusiveInstall`); the loader verification runs against a throw-away copy so the default level cannot write into the installed session | record §1 (the source check line), §4; `TestInstallClaudeTranscriptIsExclusiveAndPrivate`, `TestInstallClaudeTranscriptRefusesAFileSystemWithoutHardLinks`, `TestVerifyClaudeLoadResolvesTheInstalledSessionAndItsControl` (the installed file's digest is compared) |
| B4 | *Any unsupported source roles, content blocks or message relationships must be reflected in the conversion evidence, not silently presented as native-equivalent.* | Every category the writer knows is reported on every run with its count and disposition; format limits state what the pair cannot express at all; the round-trip check fails the `structure` state when the file does not hold a claimed item; a message *relationship* the target has no field for (a spawned child session's id on a carried call) is counted as its own category, `subagent_session_links` | record §2, §5; record §6.5 (finding 4); `TestClaudeWriterReportsEveryCategoryAndOmitsTheOthers` (the category set and each count), `TestClaudeWriterCountsADiscardedSubagentSessionLink`, `TestVerifyClaudeTranscriptStructureReportsEvidenceTheFileDoesNotHold` |

## Out of scope by the goal's own bounds (stated, not silently omitted)

- Generic prompt engineering, Git restoration, automatic context compaction and full context
  re-discovery: none is implemented; the writer's only transform is identity.
- A second source parser, a second session index, a competing writer, a daemon: none is added; the
  increment's diff contains no new database, watcher or schema code.

## Residual limits (named, not hidden)

- The `resume` level and the native check's turn need model access; on a host without it they report
  not-verified with the harness's own reason (record §3(b) shows the offline `load` level still running).
- Claude Code's app-server-era protocol is not used here: `load` drives the CLI's own loader with
  `--bare`, so a version that changes that flag's meaning would be reported as not-verified rather than
  silently passing.
- The parentUuid chain discipline is asserted by the writer's own round trip and by the loader's
  acceptance; the loader's *warning* about unlinked records (record §0) is the harness's own channel and
  is not parsed by this build.

## Mutation sweep — the fix round that followed review round 2

Four rules this round changed or added were each broken on this tree, the named case run, and the tree
restored (the round's own byte-for-byte backup of the two Go files it mutated; the restore is the reason
the round's diff-stat is the same before and after). The clause each rule sits under is named on the
left, the mutation that broke it next, then the case that fired — the record's §6.5 quotes the failure
lines:

| Clause | Mutation (broken rule) | Case that fired |
| --- | --- | --- |
| D1, D5 | the writer trims the recorded working directory again (`strings.TrimSpace(conv.Source.Cwd)`) | `TestConvertToClaudeTranscriptKeepsAWorkingDirectoryEndingInWhitespace` (the recorded cwd, the project spelling, the record bytes) and `TestContinueIntoClaudeKeepsAWorkingDirectoryEndingInWhitespace` (the reported cwd, the installed path) — exit 1 |
| D1 | the invocation ignores the probe's own answer (`binary := in.Binary`) | `TestClaudeInvocationNamesTheExecutableItsProbeFound` — `arguments name "claude"/"claude", want the probed executable "/opt/ccc tools/claude"` — exit 1 |
| D1, D2 | the printed exec form drops the prompt's option terminator | `TestContinueIntoClaudeConvertsInstallsAndReadsBack`, `TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation` and `cmd/ccc`'s `TestContinueToClaudeConvertsInstallsAndReportsTheRoute` — exit 1; the same shape is driven offline through the shipped native check by its `test_the_reported_exec_form_must_carry_the_prompt_terminator`, which fails that one check |
| D3, B4 | the writer stops counting a discarded subagent-session link | `TestClaudeWriterCountsADiscardedSubagentSessionLink` — `subagent_session_links = {… Count:0 …}, want the one written call's link counted as omitted` — exit 1 |

The sweep is **not** exhaustive, and says so: it covers the four rules this round changed or added, and
every other row above still names a case rather than a mutation. The per-clause sweep remains the
acceptance stage's work.
