# Evidence — 0001 Inherit AgentsView and inspect real sessions

Contract: [`docs/goals/archive/0001-agentsview-parity-and-inspection.md`](../goals/archive/0001-agentsview-parity-and-inspection.md).
Plan: [`docs/plans/0001-agentsview-parity-and-inspection.md`](../plans/0001-agentsview-parity-and-inspection.md).
Every command below was run on the branch, and every block is its real output, with the repository
root rewritten to `<repo>` so no host path is committed.

Host: linux/amd64, 4 cores, 7 GB RAM. Toolchain: `go1.27.4`, `python3 3.15.2`, `gitleaks 8.30.1`.
Pinned revision: `83195c8a6861b8c74a4b6cb87512817fed6f8584` (`v0.47.0-181-g83195c8a`).

## 1. A separately buildable CLI inspects a real session and reports identity, agent, source and conversation

Fixtures (sanitized, invented content, built by the pinned revision's own `internal/testjsonl`
builders and committed):

```
$ sha256sum internal/ccc/testdata/fixtures/claude/*/*.jsonl internal/ccc/testdata/fixtures/codex/2026/05/04/*.jsonl
2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6  internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
d9957a8d6f5731a3df9ce1f175ffe9e228c71f041cd765b03f743852aad1d68b  internal/ccc/testdata/fixtures/codex/2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl
```

The Claude fixture carries two turns, a `Read` tool call with its result, a compaction-summary
boundary, a billable assistant turn with usage, and a deliberately truncated final line; the Codex
fixture carries session metadata, a `shell` call with its output, a token-count event and a
compaction boundary. `TestFixturesMatchTheirGenerator` re-runs `go run ./internal/ccc/fixturegen` into
a temporary directory and requires the committed bytes to match, so the files cannot drift from the
shape they document.

The binary is built with the flags upstream ships (`CGO_ENABLED=1 -tags fts5`):

```
$ CGO_ENABLED=1 go build -tags fts5 -o /tmp/ccc-bin ./cmd/ccc     # rc=0
$ /tmp/ccc-bin inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
  agent:        Claude Code
  session id:   9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  project:      example_api
  working dir:  /home/user/code/example-api
  source version: 1.0.30
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  tools:        Read
  recorded:     2026-05-04T09:15:00Z .. 2026-05-04T09:16:36Z
  first prompt: Summarize the retry policy in internal/retry/policy.go.
  fidelity:     1 malformed line(s)
  archive:      none at <tmp>/av/sessions.db
  warning:      no archive at <tmp>/av/sessions.db; reporting source facts only
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

`inspect --json` (same run, head):

```json
{
  "upstream_revision": "83195c8a6861b8c74a4b6cb87512817fed6f8584",
  "ref": { "Provider": "claude", "ID": "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10" },
  "agent": "Claude Code",
  "source": {
    "identity": {
      "provider": "claude",
      "agent": "Claude Code",
      "session_id": "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10",
      "raw_session_id": "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10",
      "source_path": "<repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-…jsonl",
      "project": "example_api",
      "cwd": "/home/user/code/example-api",
      "source_version": "1.0.30",
      "fingerprint": "2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6",
      "file_size": 2051
    },
    "counts": { "messages": 8, "user_messages": 3, "assistant_messages": 5, "system_messages": 0,
                "tool_messages": 0, "system_records": 1, "sidechain_messages": 0,
                "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1 }
  }
}
```

Exit codes from the same binary: found `0`, `help` `0`, and `inspect -h`/`inspect --help` `0` as well
— the usage line is the answer to a question about the command line, so it goes to stdout and the exit
code is the success one — session not found `1` (with the provider list on stderr), unknown provider
`2`, unknown command `2`.

The tests that hold this down: `internal/ccc/fixtures_test.go` (identity, counts, tool call, boundary,
malformed-line count, tool names, time range, and that no fixture contains a home directory, this
checkout or a credential shape), `cmd/ccc/main_test.go` (both output shapes, the usage errors and that
they stay on stderr, the not-found path, help without an environment — including the `-h` forms — and
that `inspect` never creates an archive), and `internal/ccc/archive_test.go` for the archive read's own
report: `content_shown` as what the rows show (`tool-payloads`, `message-text`, `none`), the folded
tool-result row, and the archive-only inspection that reports its comparison as not run.

Three reference-shape rules join them in the round-3 fix pass, each pinned by a test that fails when
its fix is reverted (one executed mutation per rule):

- a **host-qualified** reference (`codex:host-a~codex:<native id>`) names the session that host
  recorded, not this machine's session of the same native id: the archive resolves the host-qualified
  row and reports another host's row (or the local one) as not found, two rows of the requested host as
  candidates rather than a pick, and the source reader refuses with `ErrNoLocalSource` — the fixtures
  hold a Codex session under the same native id, which a reader that dropped the host would report
  from the source side (`fixtures_test.go`, `archive_test.go`);
- a **relative filesystem reference** is resolved against the provider's configured roots, so the same
  reference reaches the session through the source reader and through the archive, which stores the
  absolute path the writer recorded; two roots that hold it are reported as candidates, the same root
  listed twice is one root, and an archive opened without a configuration does not resolve the relative
  spelling at all (`fixtures_test.go`, `archive_test.go`);
- a tool result whose **persisted output could not be read** keeps the inherited parser's
  parse-time answer: the result carries `PersistedOutputUnresolved`, its content stays the placeholder
  that names the side file, `Fidelity.UnresolvedToolOutputs` counts it and an inspection warns — while
  a result whose side file *is* readable carries the file's bytes and no flag (`fixtures_test.go`).

Four more rules join them in the round-4 fix pass, on the same surface and with the same evidence shape
(one executed mutation per rule, recorded in the stage report):

- a **path reference whose provider answer is a classification of an absent path** is not a source. The
  inherited classifier answers for a path that is not there on purpose — a watcher must not drop a
  deletion — and taking that answer for a readable source made a relative reference ambiguous between
  the root that holds the session and one that does not, and turned a source deleted after its archive
  was written into a fingerprint error that lost the whole inspection. A candidate is kept when a file
  stands behind it: the spelling the reference named, or the identity the provider maps that spelling to
  (`fixtures_test.go`: an absent second root next to the one that holds the session, and a
  `tool-results/` sidecar spelling that names no file resolving to its owning transcript;
  `archive_test.go`: the deleted-source inspection reports the archived session and warns about the
  source);
- an **archive that stopped before a tool result arrived** is not reported as holding it. The comparison
  counted every tool-result-only source row as folded into its call, so a stale archive was "agreeing
  with the source read" and an inspection claimed the result *is stored in the archive folded into their
  tool call* while the archive held only the call. The archived rows are searched for the result the
  source row carries — its tool-use id and content size, the two fields both reads spell the same way —
  before it is counted; a result the archive does not hold is a named difference
  (`archive_test.go`: `TestInspectDoesNotCallAStaleArchiveFolded`, over an archive written from the same
  session before the result arrived, and `TestCompareMessagesNamesEveryDifference`, which pins each
  verdict of the comparison directly);
- a **host-qualified reference is answered only by the provider it names**: the archive's own spelling
  `<host>~<the id it had there>` was returned for any provider, and Claude's stored ids carry no agent
  prefix, so `codex:host-a~<a claude native id>` resolved the Claude row and an inspection labelled
  another product's conversation "Codex". The exact answer is now filtered by the row's own `agent`
  column, and a row of another provider falls through to the provider-and-host-filtered lookup
  (`archive_test.go`: the cross-provider case, with the same row still answered for `claude:`);
- a **candidate list the inherited lookup cannot complete is refused, not read as an answer**: the
  suffix lookup answers a limited number of rows before this code filters them by provider and host, so
  a requested host's row older than the first page was reported absent although the archive held it. The
  lookup now grows its page until the inherited query reports it complete, and a list that is still full
  at the cap is answered with `ErrTruncatedCandidates` — neither "not found" nor one row is supportable
  from a subset (`archive_test.go`: the row beyond six other hosts' copies, and the saturated list;
  decision recorded as Q-0001-13, and Q-0001-14 for the absent-candidate rule above).

That pass was run as a systematic close-out of the functions it touched: every rule those functions state
was mutated (the rule's line disabled, the test that names it run, the line restored) to see whether the
test actually fails without it. Four rules survived their mutation and are now pinned by a case each —
the per-spelling ambiguity report, the acceptance route that keeps a store member behind a present
container path, the working-tree digest (a same-size edit), and a stored deviation git's index does not
hold — and one rule was found redundant rather than unpinned: in the bytes-only branch a hash that is
neither the stored nor upstream's bytes is caught twice, by the rule that names it there and by the one
that follows, so disabling either alone changes the failure's wording and nothing else (the stage report
carries the sweep and each observed failure).

Five more rules join them in the round-5 fix pass, on the read path and the inspection comparison (the
same evidence shape: one executed mutation per rule, the rule's line disabled, the test that names it
run, the line restored — fifteen rules were mutated in this pass and every one of them failed its named
test when reverted):

- a reference that names a **session** must be produced by the parse, even when the parse returns a
  single result. A provider that resolves a reference by normalising it — the Piebald provider looks a
  fork reference such as `42-7` up as its base chat 42 — parses that chat to the one result `piebald:42`,
  and the reader took the sole result as the requested session: an inspection reported the base
  conversation for a fork that does not exist, and a conversion would have carried the wrong history.
  The requested identity is now required in the parse outcome — the check the sync engine makes for the
  same reason (`internal/sync`'s `providerSessionSourceMtime`) — while a **path** reference keeps its own
  answer: the provider's resolution of the path is the session's identity, so the source's single result
  is that session whatever id the provider reports (`TestReadRejectsASingleResultThatIsNotTheReference`
  and `TestPickResultRequiresTheReferenceIdentity`, over every spelling a reference may use, and
  `TestReadAcceptsTheOnlyResultOfAPathReference` for the distinction the identity rule must not swallow);
- an **archive missing a call's outcome** is a named difference, not agreement. Codex records a call's
  output as an event on the call's own row, so an archive written before the output arrived has the same
  ordinals, roles and content as the later source read and differs only in the result the source records
  on that row; the comparison read role and content alone and reported the stale archive as holding the
  conversation. Each archived row is now asked for the results the source row records — wherever the
  archive keeps them: a result row of its own, an event it folded onto the call, or the call's own result
  content — and the bodies both reads expose are compared too
  (`TestInspectDoesNotCallAStaleArchiveCompleteAfterACallOutcomeArrived`, over a small Codex rollout
  written with and without its output event, whose second half is the control that an archive *with* the
  output still reads as agreement; `TestCompareMessagesNamesEveryDifference` pins the verdicts directly:
  the missing outcome, the result the archive holds on the same row, a body the two reads disagree about,
  and a body only the archive shows — which is not a difference, because the source read shows no body
  for it either);
- a source under a root the configuration labels as **another machine's** keeps that attribution: the
  reader builds its providers with the configured root-to-machine mapping and parses with the source's
  effective machine. Dropping both read a foreign root as a local one — the Antigravity provider marks a
  foreign root's working directory as *remote* metadata and disables local filesystem project discovery
  from that mapping, and its inherited tests pin that distinction for a provider that was configured with
  it — and recorded this installation's identity on a session another machine recorded
  (`internal/ccc/reader.go`). Both halves are pinned on real providers:
  `TestReadKeepsAForeignRootForeign` writes a small Antigravity brain transcript under a throw-away root
  and requires a foreign-labelled root to resolve as `SourceCwdRemote` and to parse to the foreign
  machine's session, while a locally labelled one does not; `TestReadUsesTheConfiguredSourceMachine`
  requires the Claude fixture under a labelled root to report that label, and the unlabelled and
  locally-labelled cases to report the installation's own.

The round-6 fix pass closed four review findings, three of them on the read path's and the comparison's
own claims (same evidence shape: one executed mutation per rule, the rule's line disabled, the test that
names it run, the line restored — eleven rules were mutated in this pass and every one of them failed
its named test when reverted; the stage report carries each observed failure):

- a **folded tool result's body** is compared, not only its key. The archive holds a result the source
  recorded as its own row by folding it onto the call, and the key — the call and the content length —
  cannot tell two same-length outcomes apart, so an archive recording `PASS` for a session whose
  transcript now says `FAIL` was reported as holding every source message. The comparison now reads the
  body wherever the archive keeps it for a folded row (a result row of its own, an event folded onto the
  call, or the call row's own result content), accepts either representation of the source's body (the
  raw JSON content field, and the decoded text `parser.DecodeContent` produces — the step both inherited
  write paths apply), treats one matching body among several under one key as agreement, and reports
  nothing when the archive shows no body at all (a blocked category keeps the length and no text)
  (`TestInspectReportsADifferentFoldedResultBody` holds an archive of a `Bash` result against a later
  transcript of the same session, once with the recorded outcome and once with a same-length different
  one, whose second half is the control that the unchanged outcome still reads as agreement;
  `TestCompareMessagesNamesEveryDifference` pins the verdicts directly: the differing body on the call,
  the differing body on an event, the narrowed body, the decoded spelling, two bodies under one key with
  one match, and the call's *summary* on a row both reads have — deliberately out of scope, because the
  source read never showed that text);
- a provider **operation failure is not absence**. The inherited Cursor provider reports a retryable
  store-enrichment failure as `ParseOutcome.SourceErrors` with a nil Go error and no result; the reader
  wrapped every result-less outcome in `ErrNotFound`, and an inspection then said "no readable source
  session" while discarding the only copy of the provider's diagnostics. The outcome's `SourceErrors`
  now classify the failure as its own answer (`ErrProviderFailed`, distinct from `ErrNotFound`) with the
  provider's diagnostics in the error, and an inspection reports them and still consults the archive
  (`TestReadReportsAProviderFailureInsteadOfAbsence` for the classification and the diagnostics;
  `TestInspectExposesAProviderFailureAndFallsBackToTheArchive` for the inspection's end — the failed
  source read, the archived session, and the provider's own message in the warnings, with the "no
  readable source session" wording absent);
- a **path whose parse produced several sessions returns them**. A Claude Code transcript whose DAG forks
  parses to the main branch and the fork — two sessions from one file, the shape the inherited parser's
  own `TestParseClaudeSession_MetadataOnForkSessions` pins — and the reader answered that path as
  *absent*: the CLI said "no session found" for a transcript that is there, with both parsed sessions
  discarded. The design's rule for an ambiguous identifier applies — return the candidates instead of
  guessing — and the candidates are the parse's own session identities, the identities the archive
  stores (`TestReadReportsAForkedPathAsAmbiguous` writes the forked transcript and requires the two
  identities; `TestPickResultRequiresTheReferenceIdentity` keeps the helper's own answer that several
  results name no single session for it, and `Read` turns that answer into the candidates);
- the documented **build output** lives under the ignored `bin/`. `README.md` wrote the executable to
  root-level `ccc`, which no ignore rule and no manifest record covers, so a checkout that followed the
  quickstart then failed the required `parity` step as an unexplained file. The command now writes
  `bin/ccc`, which `.gitignore`'s `/bin/` covers (`go build -o bin/ccc` creates the directory itself on
  this tree's Go; `go.mod` requires 1.27.4). Executed verification of both ends: the old spelling
  reproduces the finding — `go build -tags fts5 -o ccc ./cmd/ccc` then `python3 scripts/upstream-parity.py`
  prints `parity: FAIL — 1 file(s) are neither carried nor project-owned` naming `ccc`, rc=1 — while with
  the documented `bin/ccc` in place `parity` prints rc=0, and one of the round-6 gate runs below was
  taken with that artifact present (the 33.9 s one; the quoted run has no artifact, since only the built
  tree was removed).

The round-8 fix pass closed two review findings, one on the comparison's claim and one on the parity
check's mode rule, with the same evidence shape (every rule the touched code claims mutated, the test
that names it run, the line restored — ten mutations, each quoted in §6):

- a tool result's **recorded outcome** is compared, not only its key and body. The inherited parser maps
  a provider's own `is_error` marker to the result's status (`errored`, `internal/parser/content.go`),
  ingestion keeps it on the archived event (`internal/ingest/tool_results.go`), and both new adapters
  carry it (`internal/ccc/reader.go`, `messageFromArchive`) — but the comparison read the key and the
  body and never the status, so an archive recording a failed call beside a source read recording the
  same call as a success was reported as *holding every source message*, and the CLI printed "every
  archived message matches the source read". The comparison now compares the outcomes both reads record
  for a key, in both directions and as sets (one key can be held by several rows, and an empty status is
  a value: it is what a read records for a result its provider did not mark as an error), wherever the
  archive shows a status-bearing row for that key — a call row's own content is not one, which is the
  same allowance the body comparison makes, and what keeps a provider whose results the archive stored
  on the call from being reported as differing everywhere. The warning names both reads' statuses
  (`TestInspectReportsADifferentFoldedResultOutcome` writes two Claude transcripts that differ in the
  `is_error` field alone and holds them against one archive, whose first half is the control that the
  transcript the archive was written from still agrees; `TestCompareMessagesNamesEveryDifference` pins
  the verdicts directly: an outcome only the archive records, one only the source records, a matching
  outcome, an extra archived outcome under one key, a key the archive holds only on its call row, and
  the folded shape; the strictness the finding left open is the default recorded as Q-0001-19);
- the parity check's **mode rule** compares the owner's execute bit. Git's stored mode is derived from
  `S_IXUSR`, so a carried `100755` file chmod'ed to `0655` — group and other execute bits left set — is
  a `100644` file to git, and the check's `st_mode & 0o111` predicate accepted it while the record's
  bytes, the pinned index blob and a `git status` under `core.filemode=false` all agreed with the
  modified tree. The comparison now asks for `0o100`, which is the bit git's own mode records, and the
  messages, the disclosure line and `docs/harness.md`/`docs/upstream.md` say *owner-executable* where
  they said executable (`test_the_owner_executable_bit_is_the_one_compared_with_the_record` clears only
  that bit and requires the failure; the pre-existing cases for a `0644` file, the git-backed branch and
  the stored-byte deviation still pin the rule from the other side).

The round-9 fix pass closed four review findings on the two reads and the comparison, with the same
evidence shape (every rule the touched surface states mutated, the test that names it run, the line
restored — thirteen mutations, each quoted in §6):

- a result's **retry state travels with the messages the provider returned**. `pickResult` returned only
  `candidate.Result`, so the provider's own `ParseResultOutcome.DataVersion` and `RetryReason` were
  dropped even for a usable result: a Codex fork whose copied prefix comes from a page whose parent head
  is unreadable keeps the copied parent messages *and* says the result needs a retry
  (`parser.DataVersionNeedsRetry`, the state the inherited `TestCodexForkWithOnlyParentPageNeedsRetry`
  pins), so an inspection reported copied parent turns as this session's own recorded history with
  nothing saying its boundary was unresolved. `pickResult` now returns the whole outcome, the source
  adapter carries `Fidelity.NeedsRetry`/`RetryReason` (`internal/ccc/reader.go`), and an inspection warns
  with the provider's own reason (`internal/ccc/inspect.go`). `TestInspectReportsARetryableResultWithItsMessages`
  writes that missing-parent-page shape through the inherited `internal/testjsonl` builders and requires
  the state, the reason, the *copied messages* and the warning together; with the parent's head readable
  the same reference reports neither state nor warning and no copied prefix — the control the rule needs,
  since a flag that fires on every read would satisfy the first half alone;
- **archive-only tool results are a difference**. Every result comparison was driven by the source's own
  rows — presence, body and outcome alike — so an archive holding a call's outcome the transcript read no
  longer records was reported as *every archived message matching the source read*, the exact reverse of
  the stale-archive shape. Each archived row's own result rows are now held against the source read too
  (`unheldResults(message.ToolResults, heldSourceResults(...))` in `internal/ccc/inspect.go`), where the
  source side is the results at that ordinal plus those of the rows the archive keeps no message for —
  the folded tool-result-only rows, whose evidence has no ordinal of its own (`foldedSourceResults`).
  That second half is load-bearing rather than generous: dropping it fails four verdicts and both folded
  end-to-end controls, because a real Claude archive read materializes a folded result as an event on the
  call row while the source read records it on its own row. `TestInspectDoesNotCallAFresherArchiveAgreement`
  holds an archive written *with* a Codex call's output against the transcript read without it (its second
  half: the same archive against the transcript it was written from), and
  `TestCompareMessagesNamesEveryDifference` pins the verdicts directly — the archive's extra result, the
  folded archive result, and the control that a source row the archive folded still holds it;
- **retained tool-call inputs are compared**. The comparison read role, content and results and never the
  calls' inputs, while a Claude `Edit` renders as `[Edit: <file_path>]` (`internal/parser/content.go`),
  so two edits of one file with different arguments reached the comparison as identical text and *every
  archived message matches the source read* covered calls whose retained inputs differ. Calls are now
  paired by their provider tool-use id (by position among a message's id-less calls), a call only the
  archive holds is a difference, and a call both reads hold is compared by name and by input — the input
  only where both retain one, in the inherited writer's own spelling (`db.SanitizeUTF8`, the
  normalisation `internal/sync`'s parsediff applies when it decides whether a stored call still matches
  its parse), because the storage projection keeps call names and blanks inputs
  (`internal/db/archive_content.go`) and an input a read does not show is not one it disagrees about.
  `TestInspectReportsADifferentToolCallInput` writes two same-file edits, checks its own premise
  (identical message text, different retained inputs) before it starts, and holds one against the
  other's archive, with the unchanged edit as the control;
  `TestCompareMessagesNamesEveryDifference` pins the verdicts directly: the archive-only call, the
  renamed call, the differing input, and the input only the archive retains;
- a provider's **transcript classification is preserved through both reads**. Upstream says explicitly
  whether a read is the session's transcript or a degraded fallback standing in for one
  (`ParsedSession.TranscriptFidelity`: `full`, `summary`, or empty for "nothing signalled"), stores it on
  the session row (`sessions.transcript_fidelity`), and its own Grok and Antigravity CLI cases classify
  summary-only reads as `summary` *and* current — a statement of its own, not a retry. Both new adapters
  dropped it, so an inspection of a summary-only Grok session (whose source version says
  `grok-summary-v1` and whose only turn is the synthesized opening prompt) reported the same shape as a
  transcript read. Both adapters carry `Fidelity.TranscriptFidelity` now, `ArchiveStatus` exposes the
  stored value to the archive report, and an inspection warns on each side
  (`TestInspectReportsASummaryTranscriptClassification`: the source read's field, source version,
  non-retryable state and warning; the archive read's stored value and its warning; the Claude fixture as
  the control that a transcript read is not classified as a summary and warns about none).

The pass was run as a systematic close-out of the surfaces the findings name: every rule the touched
code states was enumerated and mutated — thirteen lines, the test that names each one run, the line
restored — and each mutation failed its named test (§6 quotes them). Three pre-existing comparison
fixtures spelled a call name the source read never had (`Name: "shell"`/`"Bash"` beside a source call
named `Read`); the comparison never read names before this pass, so the mismatch was invisible, and the
cases' assertions and subjects are unchanged — the name is now the source's own, so each still pins
exactly the body, outcome or call-row allowance it was written for. Two strictness choices the findings
left open are recorded as defaults in [`docs/decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md):
Q-0001-20 names which archive-side result evidence the reverse direction holds the source to, and
Q-0001-21 names what the call comparison does and does not compare.

The round-10 fix pass closed two review findings — the reader's state for a provider the configuration
disables, and the role a standalone tool output is counted under — and was run as a close-out of both
surfaces, like round 9: every rule the touched code states was enumerated and mutated (twenty-two
mutations, the test that names each one run, the line restored), and every one of them failed its named
test after two rules were found unpinned by their own mutation and pinned in this round rather than left
as arguments (§6 quotes them).

- a **disabled provider does not take an archived session away**. `disabled_agents` is inherited
  upstream's local-ingestion switch, and this reader dropped a disabled provider's factory, so
  `resolveFactory` answered an ordinary error and an inspection returned it *before* its archive branch:
  a session the archive held was reported as a failed read. The reader now answers that state with its
  own sentinel — `ErrSourceDisabled`, wrapped so the text is unchanged, `provider "claude" is disabled
  in this configuration` — and `Inspect` classifies it as a warning rather than an error, so the archive
  is still consulted and the state is distinguishable from an absent provider (the provider's label is
  still reported) and from an absent session (the warning carries the setting's own words, and an
  inspection must not say "no readable source session" about a session it never looked for). The
  restriction itself is unmoved: reading or resolving one still fails with the sentinel, and no source
  report is produced. `TestInspectArchiveOnlyWhenTheProviderIsDisabled` writes the Claude fixture into
  an archive first and disables the provider afterwards — the state the setting's own contract
  describes, "previously archived sessions from a disabled provider remain available and exportable"
  (`docs/configuration.md`, "Disabling Session Providers") — and requires the archive report, the absent
  source report, no comparison (no source read happened) and the warning together;
- a **tool-role message is not a system message**. A standalone tool-result record is preserved as
  `parser.RoleTool` by the inherited readers — the Devin reader maps a native `"tool"` record to that
  role (`internal/parser/devin.go:804-805`) and its own database-fallback test asserts it
  (`internal/parser/devin_test.go:527`) — and both adapters here carried the role unchanged, but
  `Summarize` grouped it with `RoleSystem`, so a session whose only extra record was tool output
  reported a *system* message no record ever wrote: the count reaches source reports, archive reports
  and parity dumps. `Counts.ToolMessages` counts that role now (`tool_messages` in the JSON, a field
  beside `system_messages` that the human output does not print, so nothing else relabels it), which
  moves the dump's shape and therefore its tool id to `call-ccc-parity-oracle/3`.
  `TestSummarizeCountsEachRoleSeparately` pins the whole classification in one message list: the four
  roles, the system *record* flag (a different rule from the system *role*), sidechain, boundary, and
  *two* tool calls against one tool result — the pair the committed fixtures cannot separate, because
  both of theirs hold exactly one of each, so a summariser that summed the wrong slice agreed with every
  fixture count there was (the mutation that swapped them survived its first run and is pinned by this
  case since) — plus a message whose role the core does not classify, whose rule is that it is counted
  in `Messages` and in no role field.

Observed through the built binary, with an archive written while the provider was enabled and the
configuration then disabling it — `disabled_agents = ["claude"]` in a throw-away `config.toml`, and
`--data-dir` naming that archive:

```
$ /tmp/ccc-bin inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --data-dir <dir>      # rc=0
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
  agent:        Claude Code
  source:       not readable here (see warnings)
  archive:      <dir>/sessions.db [tool-payloads]
  archived id:  9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
  archived:     7 message(s), 2 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  agreement:    not compared (the source was not readable)
  warning:      provider "claude" is disabled in this configuration; the setting restricts local discovery only, so an archive can still hold the session
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

Before this pass the same command exited 1 with `ccc: provider "claude" is disabled in this
configuration` on stderr and no archive report: the inspection returned the reader's error before its
archive branch (the sweep's `inspect-disabled-clause` mutation reproduces that state, and the case
fails on it).

The round-11 fix pass closed two review findings — the archive's stored-agent aliases, and a path whose
parse is incomplete — and was run as a close-out of both surfaces, like rounds 9 and 10: every rule the
touched code states was enumerated and mutated (twenty mutations, the test that names each one run,
the line restored, each file's bytes verified back before the next), and all twenty failed their named
test (§6 quotes them, including the two entries the first sweep could not use and the rule it had not yet
covered). The round-9 review and this one both named a `pickResult` line (round 9 the
retry state its result carries, round 11 the path branch), so this pass enumerated that helper's rules
in full rather than patching one branch again.

- the **inherited provider-to-stored-agent mapping decides which archive rows a reference names**. The
  Codebuff provider also writes free-tier sessions: the parser reports `AgentFreebuff` and a
  `freebuff:<project>:<timestamp>` id for one, while discovery, caches and `provider_freshness` stay
  keyed on Codebuff — `parser.ProviderStoredAgents` is where upstream records that both labels belong to
  it, and `internal/parser`'s `TestParseCodebuffSession_FreebuffClassification` pins the parser's own
  output. Every archive lookup compared the row's `agent` column with the provider's own name, so a
  Freebuff session read from its source by a `codebuff:<project>:<timestamp>` reference stopped being
  findable — by id, by host-qualified spelling and by path — the moment that source was gone. The
  comparison is the mapping now, in all three shapes (`providerStoresAgent` for the id and
  host-qualified filters, `providerStoredAgentNames` where a query takes agent names; `providerRoots`
  answers a stored label with the roots of the provider that writes it, since
  `FREEBUFF_CONFIG_DIR` re-roots the same projects path). The label is also a *name* a reference may use
  — `storedAgentOwner` resolves one that has no registry entry through the provider whose mapping
  includes it, the CLI asks the core (`ccc.KnownProvider`) instead of doing its own registry lookup, and
  a session's own recorded label travels into the source report and a missing top-level one falls back
  to it — so the identity an inspection prints is the identity that resolves next (Q-0001-23).
  `TestFreebuffSessionResolvesThroughItsCodebuffProvider` is the round trip: a real Freebuff session is
  written into an archive, then read back through four spellings (the provider's native id, the stored
  `freebuff:` id, the stored label, the transcript path — absolute, relative and after the source is
  deleted and reconciled), each with the row's own label, and a `codex:` reference against the same
  native id stays not found;
  `TestHostQualifiedFreebuffRowResolvesThroughItsCodebuffProvider` seeds host-qualified rows through the
  inherited database API — the local Codebuff row of the same project and timestamp is never offered for
  `host-a` — and `TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead` shows the exact spelling
  answering where the candidate read cannot be completed at all.
- a **path whose parse is incomplete names no session**. `pickResult` took the sole successful parse
  result of a *path* reference without looking at `ParseOutcome.ResultSetComplete` or the outcome's
  source errors, and `Read` then discarded the outcome's diagnostics: a multi-session source that
  returns one readable member beside members it could not read — the inherited shape
  `TestOpenClawSQLiteStoreMalformedMemberIsolation` pins — resolved that path to the readable member, so
  whether a malformed sibling parses decided which conversation the caller meant. A sole path result is
  that source's session only when the parse read the source in full now; otherwise the reader answers
  `ErrPartialSource` carrying the provider's diagnostics and the identities it did read, an inspection
  reports them as a warning beside the archive, and the identity in the warning is what reads next. The
  pass also measured which container shapes are reachable: Trae's stored-path lookup answers the
  container itself and its snapshot marks an unreadable entry without isolating it to a session
  (`TestReadRefusesAPartialTraeContainer` drives exactly that through the inherited provider on a real
  `state.vscdb`), while OpenClaw's stored-path lookup resolves *members*, so its container path reaches
  the changed-path enumeration instead and its members — malformed ones included — are the candidates
  (`TestReadReportsAnOpenClawContainersMembersInsteadOfPickingOne`);
  `TestReadRefusesAPathWhoseParseIsIncomplete` reproduces the inherited outcome field for field as a
  stand-in, including the control that an outcome with *no* result keeps its own classification and that
  a reference naming an identity is unaffected by incompleteness elsewhere in the source.

Observed through the built binary, with a Freebuff source under a throw-away `CODEBUFF_DIR` and an
archive the inherited ingestor wrote from it (`--data-dir` naming that archive):

```
$ /tmp/ccc-r11 inspect codebuff:example-project:2026-07-15T20-01-32.065Z --data-dir <dir>     # rc=0
codebuff:example-project:2026-07-15T20-01-32.065Z
  agent:        Codebuff
  session id:   freebuff:example-project:2026-07-15T20-01-32.065Z
  source:       <root>/example-project/chats/2026-07-15T20-01-32.065Z/chat-messages.json
  conversation: 2 message(s), 1 user, 1 assistant
  archive:      <dir>/sessions.db [message-text]
  archived id:  freebuff:example-project:2026-07-15T20-01-32.065Z
  agreement:    every archived message matches the source read

$ rm -rf <root>            # the finding's state: the source is gone, the archive still holds it
$ /tmp/ccc-r11 inspect codebuff:example-project:2026-07-15T20-01-32.065Z --data-dir <dir>     # rc=0
codebuff:example-project:2026-07-15T20-01-32.065Z
  agent:        Codebuff
  source:       not readable here (see warnings)
  archive:      <dir>/sessions.db [message-text]
  archived id:  freebuff:example-project:2026-07-15T20-01-32.065Z
  archived:     2 message(s), 1 user, 1 assistant
  agreement:    not compared (the source was not readable)
  warning:      no readable source session for codebuff:example-project:2026-07-15T20-01-32.065Z

$ /tmp/ccc-r11 inspect freebuff:example-project:2026-07-15T20-01-32.065Z --data-dir <dir> --json
{ … "agent": "Freebuff", "archive": { "status": { "session_id":
  "freebuff:example-project:2026-07-15T20-01-32.065Z", "agent": "freebuff", "agent_label": "Freebuff", … } } }

$ /tmp/ccc-r11 inspect no-such-agent:whatever --data-dir <dir>                               # rc=2
ccc: unknown provider "no-such-agent"
```

Before this pass the second command reported no session at all (the row was filtered out by the
exact-name comparison), the third exited 2 as an unknown provider before any lookup, and the fourth was
the only outcome the CLI could give for it. The sweep's `archive-provider-filter`,
`host-exact-predicate`, `path-current-labels`, `path-retained-labels`, `provider-roots-owner`,
`known-provider-mapping` and `resolve-factory-alias` each break a different part of that round trip, and
each failed a named test (§6 quotes all twenty observed failures).

## 2. A real source-to-archive parse and a subsequent archive read agree with the pinned implementation

The oracle (`internal/ccc/parity`) parses a fixture, writes it through the inherited sync engine into
an isolated SQLite archive, reads it back through the inherited read path, and emits a canonical dump:
content is hashed, host-dependent fields are excluded and named in the dump itself. The comparison
harness runs the *same* oracle source in this tree and in a pristine checkout of the pin, over
separately copied fixture bytes and separately created archives.

```
$ python3 scripts/upstream-compare.py --upstream /tmp/agentsview-probe --record --scratch /tmp/ccc-compare-0001-r11
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) from https://github.com/kenn-io/agentsview
using the existing checkout at /tmp/agentsview-probe (clean, at the pin)
- copied internal/ccc (10 Go source file(s)) from this tree into the pristine checkout: the oracle and its call/ccc wrapper are identical on both sides, so the only variable is which tree the inherited packages come from
oracle source: 10 file(s), sha256=68d5a5796b74f0ae252b4979033e5bb0428b41e7843af9b93ac5d2dd1d940f3b (identical on both sides)
claude: fixture sha256=2bd510821d4abf83… ours=6409a23c502469a3… pristine=6409a23c502469a3… agreement=yes
  recorded internal/ccc/parity/testdata/oracle/claude.json
codex: fixture sha256=d9957a8d6f5731a3… ours=531e5c0fb19fc51a… pristine=531e5c0fb19fc51a… agreement=yes
  recorded internal/ccc/parity/testdata/oracle/codex.json
RESULT: PASS — 2 case(s) agree byte for byte with the pinned revision      # rc=0, 1.5 s warm
- removed /tmp/agentsview-probe/internal/ccc from the checkout again
- /tmp/agentsview-probe is clean
```

The oracle source digest moved with the round-9 fix pass and the two dumps did not: the copied sources
are the four call/ccc files, and the fields that pass added to `Fidelity` and `ArchiveStatus` are not
part of the dumped shape, so `--record` re-derived both committed dumps byte for byte
(`git status` shows no change under `internal/ccc/parity/testdata/oracle/`) — the adapters' *dumped*
behaviour is unmoved by a pass that only stops them dropping state.

The round-11 pass is the same shape of evidence: the digest moved again (`aa8e8f4a…` → `68d5a579…`,
three adapters touched) and both dumps did not — `--record` from the pristine side rewrote the two
committed files with the same bytes, and `git status` reports nothing under the oracle directory. That
run was taken warm (the first run of the same command on this host took 136 s building the pristine
checkout's packages; the recorded one re-ran the two cases against the already-built checkout).

The round-10 pass did move the dumps, because it moved the dumped *shape*: the counts object gained the
tool-role count, so each of the four counts blocks in each dump gained one line (`"tool_messages": 0`)
and the tool id went to `call-ccc-parity-oracle/3` — four added lines per dump, and nothing else
(`git diff --stat` reports `4 +++-` for each). The zeroes are the honest half of that: no committed
fixture records a standalone tool row, so the *classification* this round fixed is invisible in these
bytes, and the change a reader can check here is the shape that carries it. Recording from the pristine
side again is what keeps the agreement a fact rather than a local edit: both sides reproduced the dump
byte for byte, the pristine side included.

The same command against an existing clean checkout (`--upstream /path/to/agentsview`) reports the
same two dump hashes and removes the copy it made (`git status` clean afterwards); a checkout that is
not clean at the pin is refused before anything runs, because the dumps — and, with `--record`, the
committed oracle — are attributed to that revision.

The two dumps the pristine side produced are committed and re-checked offline by
`internal/ccc/parity/dump_test.go`, so the ordinary test suite — and so the gate — fails if this
tree's inherited code stops producing them:

```
$ sha256sum internal/ccc/parity/testdata/oracle/*.json
6409a23c502469a3211e86e74d2b7b53761d7739713a0cb11bc8d5fe0333b46b  internal/ccc/parity/testdata/oracle/claude.json
531e5c0fb19fc51a158c4a393481c5b63a1a6f5bed148fc0a6d996375b8353c3  internal/ccc/parity/testdata/oracle/codex.json
```

The Codex dump changed in this increment's round-2 fix pass, and only in its `parse` section: the
wrapper now carries a call's own result events into the conversation (`internal/ccc/reader.go`, the same
mapping `messageFromArchive` already used), so a Codex `function_call_output` reaches a source read
instead of being dropped. The pristine side reproduces the change byte for byte, which is the point of
recording from it: the adapter's correction is visible as agreement, not as a local edit to a dump.

The dumps carry the archive round trip's shape: for the Claude fixture, parse reports 8 messages with
1 tool call, 1 tool result and 1 compaction boundary; the archive read reports the same tool call and
boundary at the same ordinals, the tool-result-only row the archive folds into its call (and the
result's length, because the inherited default configuration blocks result *bodies* for the `Read`
category), `"content_shown": "tool-payloads"` (what that read shows, in the dump's own vocabulary:
`internal/ccc` derives it from the rows, since a read-only handle cannot see the writer's storage
policy), and one malformed line on both sides — the truncated tail is counted, never turned into a
message. For the Codex fixture the shell result body survives both reads: the parser attaches
`function_call_output` to the call rather than as a message of its own, and each side of the dump now
reports 1 tool result with the same content digest and the same 51-byte size — the source read
reported 0 of them before this fix pass, so an inspection undercounted the session's results while an
archive read of the same session kept the body.

## 3. No project-owned modification of the inherited parser/sync/DB/schema code and tests

```
$ python3 scripts/upstream-parity.py        # rc=0
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a)
reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584
carried: 5553 file(s) identical to the pinned revision (compared through git's own object store)
deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared
owned: 25 project-owned prefix(es); files seen by git: 5613
inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go)
symlinks hashed by their link target: 4
declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md
declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
```

Every declaration, and nothing else:

| Kind | Count | What |
| --- | --- | --- |
| project-owned `.gitignore`, `AGENTS.md`, `README.md`, `docs/README.md` | 4 | call/ccc's own files live at these paths |
| upstream CI definitions `.github/**` | 22 | would run upstream's CI here |
| line-ending normalisation of one frontend fixture | 1 | upstream's own `.gitattributes` rewrites its one CRLF pair on `git add` |
| generated build input | 1 | upstream does not track the LiteLLM snapshot; its Makefile restores it before every Go build |

The check reads git's own index (`git ls-files -s`) — the bytes a fresh clone receives, filters
applied — and hashes every carried path's **working-tree** copy against the manifest record in *both*
branches, with git and without it, before git is asked anything: shape, bytes and executable bit are
compared with the file this repository ships, and the index adds what a stat cannot answer (what a
fresh clone receives). It fails if a carried file is missing, changed in shape, mode or bytes, if a
`git status` answer says the working tree differs from the index, if a declaration has gone stale — a
project-owned path that turned back into upstream's copy, staged or only written on disk, or a
normalised path that stopped holding the bytes its record pins — if a probe (`git status`, ignore
rules) cannot answer, if the generated input stops matching the SHA-256 the carried snapshot tool
pins, if `docs/upstream.md` or `cmd/ccc/pin.go` disagrees about the revision, or if anything git would
add is neither recorded nor inside a project-owned prefix. The line-ending path is the one carried path
upstream's blob can never equal, so its record pins the stored digest beside upstream's and that digest
is what the check verifies; a working-tree copy still holding the pre-normalisation bytes is reported
rather than failed, because git reads that copy as clean and a checkout writes the declared bytes. It
needs no network and no upstream checkout.

The round-3 review (finding 1) measured that the git-backed branch trusted `git status` for the working
tree, and `git status` is exactly the answer a path can be configured to suppress: with a carried file
marked `assume-unchanged` (or `skip-worktree`), an edit stays out of git's report while the pinned blob
stays in the index, so every predicate the check made could agree with modified carried source. The
working-tree comparison above is that fix, and the check costs about 0.4 s and reads ~63 MB of file data
on this host — 5 553 carried files, 63 268 220 bytes (round 3 measured the hash pass at about 0.8 s,
round 8 re-measured the whole check after the mode rule's predicate changed at 0.407 s and 0.403 s wall
clock, `time python3 scripts/upstream-parity.py`, the difference being page-cache state: the rule does
one `stat` per carried file either way); three cases in
`scripts/tests/test_upstream_parity.py` script git's own answers with an empty `git status`, a pinned
index blob and a modified working tree (bytes, shape and the owner-executable bit), and each fails when
the working-tree comparison is removed from the shipped check. The same round pinned the rules that branch
already claimed and nothing exercised: a carried path present on disk but absent from git's index
(absent from a clone), a record whose pinned size is not its bytes' size, and a symlink record whose
path became a directory.

The round-4 fix pass closed the one gap that rule left: the path carried with a declared line-ending
normalisation was checked for its stored bytes (the index where git answers, its own bytes where it
does not) and for its index mode, but not for the executable bit of the file this repository ships. On a
POSIX checkout with `core.filemode=false` — and on any filesystem that keeps no mode bits — flipping
that bit leaves the declared digest, git's index mode and `git status` all saying "unchanged", so the
check accepted a modified carried path while its own documentation named it covered. The bit is now
asked of the file itself by the same helper a carried file's mode goes through (`worktree_mode`,
`scripts/upstream-parity.py`), in both branches; a run on a platform without mode bits counts that path
in its disclosure too. Three cases in `scripts/tests/test_upstream_parity.py` pin the rule — the
bytes-only branch, the git-backed branch with a pinned index mode and a clean status (the finding's own
scenario), and the disclosure — and each fails when the mode check is removed. The same pass pinned the
working-tree *digest* rule with a same-size edit (`test_a_same_size_worktree_edit_is_caught_by_its_bytes`):
every earlier edited-file case changed the file's length, so the size rule alone failed them and the
hash could be removed without a case noticing.

The round-5 fix pass closed two ways in which a run could look verified without the question having been
answered, and extended the generated input's check to what a clone actually receives:

- the generated build input is verified against **git's index** as well: its presence there, the mode it
  stores, and the blob its own bytes hash to. The carried
  `internal/pricing/snapshot/.gitignore` keeps that path out of every untracked listing — which
  `check_untracked` accepts, because the manifest names it — so a copy that stayed on disk and left the
  index was compatible with every other rule while a fresh clone received no file for the carried
  `internal/pricing/fallback.go` to embed and the module did not compile there (`check_inputs`). Three
  cases pin the rule: the finding's own shape (the working copy correct and ignored, the index without
  it), an index blob that differs from the file's bytes, and an index mode that is not the record's.
- a required git query that **cannot answer fails the step**. `git ls-files -s` refusing while the
  ordinary file list succeeded was read as "this tree has no index", which skipped both what a fresh
  clone receives and `git status` and still reached a verdict; a refused `git ls-files --others` became
  an empty list, which says "git would add nothing" — the answer that looks like a pass — for a question
  git never answered. Both fail now, naming the probe. The suite's own scaffolding moved with the rule: its
  default seams describe one coherent state (a clean scripted checkout, or none at all —
  `gitless_answers()`), because a tree that answered the file list and not the index is exactly the state
  the rule rejects, and `GitlessCase` pins what the git-less branch still answers: real drift is a
  `FAIL` with its offenders, before the `INCOMPLETE` verdict that says the rest could not be checked.

The round-7 fix pass closed the one shape a carried link can be changed into without changing a byte of
what git stores. `CLAUDE.md` and the three `.claude/skills/*` paths are `120000` records whose blob *is*
the link's target text, and the check accepted a regular file holding that text at such a path on every
checkout — the reasoning being that a checkout which cannot create symlinks has exactly that file. On a
checkout that *does* create symlinks that reasoning certifies a carried path whose working-tree type,
and behaviour, has changed: the file's bytes hash to the pinned blob, git's index keeps the pinned
`120000` entry, and with `skip-worktree` set on the path `git status` reports nothing, so every other
predicate agrees. The allowance is now limited to checkouts that state the representation — git's own
`core.symlinks=false` for this tree, or, where git cannot be asked, a platform whose checkouts cannot
create symlinks — a `core.symlinks` query that cannot answer on a tree git *is* answering for fails the
step naming the probe, and a run that accepted the file form says so in its own summary
(`carried link(s) held as a file with the link's target text: N`). Reproduced on this checkout with the
finding's own silencing flag, before and after the fix:

```
$ cp -a CLAUDE.md /tmp/CLAUDE.md.link && git update-index --skip-worktree CLAUDE.md
$ rm CLAUDE.md && printf 'AGENTS.md' > CLAUDE.md        # a file where the pin has a link
$ git status --porcelain                                # reports nothing (the flag)
$ python3 /tmp/oldparity/upstream-parity.py --root <repo>   # rc=0 — the pre-fix check accepted it
$ python3 scripts/upstream-parity.py                        # rc=1
parity: FAIL — 1 carried path(s) changed file type
  CLAUDE.md
$ cp -a /tmp/CLAUDE.md.link CLAUDE.md && git update-index --no-skip-worktree CLAUDE.md
$ python3 scripts/upstream-parity.py                        # rc=0
```

The pre-fix run is the previous commit's own script (`git show HEAD:scripts/upstream-parity.py`), given
`--root <repo>`. Seven cases in `scripts/tests/test_upstream_parity.py` pin the rule set: the finding's
scenario through the git-backed branch (a clean scripted index — mode `120000`, the pinned blob — and a
clean status, with the file in place of the link), the same scripted tree with `core.symlinks=false` as
the control that keeps the allowance where it applies, the file form refused where git says this
checkout creates links (git's answer `true`), the file form accepted where git says it stores links as
files, with the disclosure line the run prints, the platform probe's own answer both ways (through the
`platform_name` seam, so no case depends on the host it runs on), and the `core.symlinks` query that
cannot answer failing the run. Each fails when the rule it names is mutated away (below).

The round-8 fix pass closed the mode rule's own edge. `worktree_mode` asked whether *any* execute bit
was set (`st_mode & 0o111`), while git's stored mode is derived from the owner's execute bit
(`S_IXUSR`): a carried `100755` file chmod'ed to `0655` — group and other execute bits left set — is a
`100644` file to git, and the predicate still answered "executable", so the record's bytes, the pinned
index blob and a `git status` under `core.filemode=false` all agreed with a modified carried file. The
comparison now asks for `0o100`; the failure and disclosure lines say *owner-executable*, and so do
`docs/harness.md` and `docs/upstream.md`. The new case clears only that bit
(`test_the_owner_executable_bit_is_the_one_compared_with_the_record`, `0655` on the carried `100755`
file) and requires the failure; the cases that remove every execute bit, reach the rule through the
git-backed branch, and check a stored-byte deviation's mode still pin it from the other side, and each
fails when the rule it names is mutated away (below).

## 4. The continuation functionality is callable without CLI types, and the CLI is standalone

Engine packages: `internal/ccc` (Conversation document, `SourceReader`, `Archive`/`Ingestor`,
`Inspect`) and `internal/ccc/parity`. They import `internal/parser`, `internal/db`, `internal/sync`,
`internal/config` and stdlib — no Cobra, no terminal, no HTTP, no process-global state, no CLI type.
`cmd/ccc` contains argument handling and presentation only; `cmd/ccc/main_test.go` drives `run()` and
`internal/ccc`'s tests drive `Inspect` directly. Neither an AgentsView Web UI nor a second session
index is required: the archive is the inherited `sessions.db`, opened read-only.

## 5. The original source session and any user archive stay intact

- `TestIngestThenArchiveRead` hashes the whole fixture tree before and after a parse → sync → read and
  requires every byte to be unchanged.
- Every test that writes uses `t.TempDir()` (`fixtureConfig` builds its own data directory); no test
  reads or writes a real data directory, and `AGENTSVIEW_DATA_DIR`/`HOME` are pinned in the CLI tests.
- `TestArchiveReadDoesNotChangeTheArchive` requires `sessions.db` to be byte-identical after a status
  read and a message read, and allows no new file beyond SQLite's own WAL sidecars.
- `Ingestor` takes upstream's write-owner lock (`db.write.lock`, `gofrs/flock`) for the duration of a
  write and refuses when another writer holds it; it requires an explicit absolute database path; the
  CLI exposes no ingest at all, so nothing call/ccc ships can point a write at a live archive.

## 6. The gate

```
$ python3 scripts/gate.py        # rc=0, 34.5 s wall clock, taken on the final tree
gate: <repo>
gate: go=<go>/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3574 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 135 tests in 5.138s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 25 project-owned prefix(es); files seen by git: 5613 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored byte…
  PASS     secrets  no leaks found in 60 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The gate truncates the middle of a long step detail, which is why the `parity` line above opens with `…`;
the same run's full summary is quoted in §3. The `selftest` count in that run is 135, counting both
harness modules. The round-2 fix pass brought the suite to 104 (74 parity cases plus the gate module's 30): the four carried-path cases for a
declared line-ending normalisation, and the case that pins the incomplete verdict for a tree git cannot
enumerate. Round 3 added six `scripts/tests/test_upstream_parity.py` cases — three for a working tree
`git status` cannot report, one for a carried path present on disk but absent from git's index, one for a
record whose pinned size is not its bytes' size, and one for a symlink record whose path became a
directory — taking the suite to 110; round 4 added five (the stored-byte deviation's working-tree
executable bit in the bytes-only branch, the same rule through the git-backed branch with a pinned index
mode and a clean status, the no-mode-bits disclosure counting that path, a same-size working-tree edit
only the digest can catch, and a stored deviation the index does not hold), reaching 115. The round-5 fix
pass added twelve: three in `scripts/tests/test_gate.py` (the whole `*.go` list reaching `gofmt` in
bounded batches, a later batch's answer surviving, and a command line the platform refuses answered as a
step result) and nine in `scripts/tests/test_upstream_parity.py` (three for the generated input's index
entry, two for a required git probe that cannot answer, four for the git-less branch's own answers —
parity 85 → 94, gate 30 → 33). The Go side moved with each pass: `internal/ccc` went from 17 to 24 test
functions in round 3 (`archive_test.go` 13 → 16, `fixtures_test.go` 4 → 8), to 33 in round 4
(`archive_test.go` 16 → 22, `fixtures_test.go` 8 → 11), and to 39 in round 5 (`archive_test.go` 22 → 23,
`fixtures_test.go` 11 → 16: the reader's identity rule for identifiers and the path-reference
distinction, the picker's own verdicts, the configured source machine, and the foreign-root rule with a
real Antigravity transcript), and to 43 in round 6 (`archive_test.go` 23 → 25, `fixtures_test.go`
16 → 18: the forked-path ambiguity, the provider-failure classification, the folded-body comparison
end to end, and the inspection's provider-failure fallback). The round-7 fix pass moved both sides:
`internal/ccc` went 43 → 50 test functions (`archive_test.go` 25 → 32: the path lookup's selection and
order on its records, every configured root asked in both tiers, the provider scope of a path answer,
the trashed-row refusal, the retained-row tier with its current-member preference, the deleted and
reconciled source path, and the provider failure with no archive to fall back to), `cmd/ccc` went
9 → 10 (the no-result exit keeping the inspection's diagnostics, in both spellings of the command
line), and the harness's own count went 127 → 134 (`scripts/tests/test_upstream_parity.py` 94 → 101,
`test_gate.py` unchanged at 33). The round-8 fix pass moved the comparison's and the parity check's own
cases: `internal/ccc` went 50 → 51 test functions (`archive_test.go` 32 → 33: the folded tool result's
recorded outcome end to end; the six new verdicts live as subtests of the comparison case that already
pins that surface), `cmd/ccc` unchanged at 10, and the harness's own count went 134 → 135
(`scripts/tests/test_upstream_parity.py` 101 → 102: the owner-executable bit that git's mode records).
The round-9 fix pass moved `internal/ccc` 51 → 55 test functions (`archive_test.go` 33 → 36: the
fresher-archive direction end to end, the retryable result with its messages, and the differing tool-call
input; `fixtures_test.go` 18 → 19: the summary transcript classification through both reads, with the
round-9 comparison verdicts added as subtests of the comparison case that already pins that surface),
`cmd/ccc` unchanged at 10, and the harness's own count unchanged at 135 (no `scripts/tests/` case
changed). The round-10 fix pass moved `internal/ccc` 55 → 58 test functions (`archive_test.go`
36 → 38: the disabled-provider archive-only inspection and the unclassified-read-error case;
`fixtures_test.go` 19 → 20: the role-count case), `cmd/ccc` unchanged at 10, and the harness's own count
unchanged at 135. The round-11 fix pass moved `internal/ccc` 58 → 66 test functions (`archive_test.go`
38 → 42: the Freebuff round trip through its Codebuff provider, the host-qualified alias case, the
exact-spelling case against an uncompletable candidate read, and the partial container the inspection
names an identity for; `fixtures_test.go` 20 → 24: the provider-name list and `KnownProvider`, the
inherited partial-container outcome on a stand-in, the same rule on a real Trae container, and the
OpenClaw container's members-as-candidates measurement), `cmd/ccc` 10 → 11 (the stored agent label the
CLI now accepts), and the harness's own count unchanged at 135 (`scripts/tests/` untouched: no
`scripts/` file changed this round, so the `selftest` count is the same and the gate's own numbers below
are directly comparable).

Wall clock for the whole gate on this host: 19–21 s with warm caches across the increment's earlier
runs, 25 s to 156 s across the round-2 fix pass's runs (the 156 s run followed a cold Go build cache, so
`build`/`vet`/`test` recompiled the inherited packages), 21–23 s across the round-3 runs, 22 s, 26 s and
53 s across the three round-4 runs, and 21.8–25.0 s across the round-5 runs. The six round-6 runs
measured 26.5 s, 25.2 s, 45.8 s, 28.3 s, 33.9 s and 31.1 s, the round-7 runs 26.1 s, 28.8 s, 25.2 s and
55 s, the round-8 runs 33.3 s, 22.7 s, 23.4 s and 24.4 s, the round-9 runs 27.6 s and 26.1 s, and the
round-10 runs 26.9 s, 25.5 s and 27.4 s (the quoted one is the last, whose `selftest` step took 5.358 s,
inside the 4.76–6.18 s band this paragraph closes on), and the round-11 runs 102.5 s, 36.3 s, 89.4 s and
34.5 s (the first three ran beside this round's own work — a test-fixture refactor, a mutation sweep —
and the quoted one is the last, taken on the finished tree with the host otherwise idle; all four printed
the same verdict, and their `selftest` steps took 5.266 s, 5.490 s, 7.525 s and 5.138 s). The round-11
runs are slower than round 10's because this round's cases do more work — three containers are created on disk with the sqlite driver, and one
case seeds 502 archive rows — not because a step became slow: `test` reports the same three packages and
`selftest` stays inside the band.
The block above quotes the round-11 run taken after every code and test edit this round made; the only
writes after it are the documentation this round added about the run — the block itself, this paragraph,
and one count in §1's paragraph that the sweep's twentieth entry corrected. The code and test bytes it
gated are the ones shipped. The round-7 runs' 55 s one was taken last on that round's finished tree — the only write
after it is the sentence that quotes its own number, and its `selftest` step took 12.0 s against
5.5–6.1 s in the three runs before it, so that wall clock is host load and not a slower gate. With
`selftest` steps between 4.76 s and 6.18 s across them, the wall-clock spread is host load rather than a
slower gate (the 28.3 s run's step was the slowest at 6.18 s, the 45.8 s run's among the fastest at
4.96 s).
The 53 s round-4 run
ran while the host was busy, which is visible in its `selftest` step (23.6 s against 5.1 s and 5.2 s);
the suite on its own measures 5.4–6.4 s over these runs, so the difference is load and not a test that
grew slow. The `secrets` count is the files under the manifest's project-owned prefixes, which includes
the harness's own `scripts/tests/__pycache__/*.pyc` once the suite has run here (58 without them, 60
with the two this run's selftest left behind). `gofmt` covers the carried tree as well (3574 Go files);
`build`/`vet`/`test` cover the three call/ccc packages, which compile the inherited packages they depend
on.

The round-5 pass was run as a systematic close-out of the three surfaces it touched, like round 4's: every
rule those functions state was mutated (the rule's line disabled, the test that names it run, the line
restored — 15 mutations) and every one of them failed its named test. One rule's first mutation displaced
the wrong occurrence of a two-line pattern shared with `check_files`, so it was re-run pinned to the
`check_inputs` call site; the recorded failure is from that run.

The round-6 pass did the same for the read path's and the comparison's surfaces (11 mutations: the
provider-failure sentinel, the inspection's branch for it, the forked-path candidates and the identity
each candidate names, the folded-body comparison as such, the decoded spelling it accepts, the rule that
one matching body among several under a key is agreement, the call-row bodies it reads, the result rows
it reads, "no body shown is not a difference", and the same-row scope that keeps a call's summary out of
the comparison). Every mutation failed its named test; the fold-body sweep's observed failures include
the real fixture path (`TestIngestThenArchiveRead`, whose Claude `Read` result is blocked and must not
be reported as a different body). Two intermediate edit states were corrected before their mutation ran
(one left an unused import, one restore dropped a brace); no recorded failure comes from a state that
did not compile.

The round-7 pass closed three findings with the same rule, one surface at a time: **every rule the
touched code, document or test claims was mutated, and every mutation failed the test that names it**
(15 mutations in total, plus one live reproduction of the parity defect on this checkout).

- the path lookup (`internal/ccc/archive.go`, 7 mutations): reverting the retained tier to the
  inherited current-members-only lookup (`TestDeletedSourcePathFallsBackToTheArchive`,
  `TestPathReferencePrefersTheSourcesCurrentMembers`), widening the listing's agent scope
  (`TestPathReferenceDoesNotCrossProviders`), dropping the trashed-row filter (`TestPathRecordIDsSelectsAndOrdersTheRowsOfAPath`,
  `TestPathReferenceIgnoresATrashedSession`), asking one spelling instead of all of them — in the
  retained tier and in the current tier separately
  (`TestPathRecordIDsSelectsAndOrdersTheRowsOfAPath`, `TestRelativePathReferenceAsksEveryConfiguredRoot`,
  `TestRelativePathReferenceAsksEveryRootBeforeFallingBack`), dropping the sort that keeps a candidate
  list independent of the query plan (`TestPathRecordIDsSelectsAndOrdersTheRowsOfAPath`), and making
  the two tiers a union (`TestPathReferencePrefersTheSourcesCurrentMembers`, which observes the union
  reporting one conversation twice);
- the CLI's no-result exit (`cmd/ccc/main.go`, 1 mutation): dropping the warnings loop
  (`TestNoResultExitKeepsTheDiagnostics`);
- the parity check's link shape (`scripts/upstream-parity.py`, 7 mutations): restoring the
  unconditional file acceptance (`test_a_link_held_as_a_file_is_a_shape_failure_where_symlinks_are_created`,
  `test_a_link_replaced_by_a_file_under_a_silent_status_is_still_caught`,
  `test_the_platform_answer_decides_a_link_file_form_when_git_cannot_be_asked`), accepting the file
  form unconditionally (`test_a_symlink_configuration_query_that_cannot_answer_fails_the_run`), fixing
  the platform answer either way (`test_the_platform_answer_a_link_file_form_is_read_from`), ignoring
  git's own `core.symlinks` answer (`test_a_link_held_as_a_file_passes_where_the_checkout_stores_links_that_way`,
  `test_a_link_held_as_a_file_where_git_says_so_is_the_same_file_accepted`), and dropping the
  disclosure line the allowance prints (the same two cases).

One mutation's first form was a no-op rather than a mutation — widening the listing's agent scope with
the provider the reference already named — so it was re-run widened by the provider that recorded the
row instead, and that run is the failure quoted above. Two rules were then found unpinned by their own
mutation and pinned in this round rather than left as arguments. The candidate list's order could not be observed through SQLite (the shipped query's plan
answers one path's rows in id order anyway), so the selection and the ordering moved into a named
function (`pathRecordIDs`) the suite drives directly with records in scrambled order; and the platform
probe's *own* answer could not be asserted without depending on the host running the case, so it now
reads a named seam (`platform_name`) whose both answers a case patches. Every recorded failure came
from a compiling tree; the one mutation that did not (dropping `sort.Strings` and leaving the unused
import) was re-run with the import removed too, which is the failure quoted above.

The round-8 pass closed two findings with the same rule, one surface at a time: **every rule the
touched code or test claims was mutated, and every mutation failed the test that names it** (10
mutations). The observed failures, in the order they were run:

- the parity check's mode rule (`scripts/upstream-parity.py`, 4 mutations). Comparing *any* execute bit
  again (`& 0o100` → `& 0o111`) made
  `test_the_owner_executable_bit_is_the_one_compared_with_the_record` fail with `AssertionError: 0 != 1`
  — the `0655` file passed the run, which is the finding's own hole. Disabling the comparison
  (`if False:`) failed five cases: `test_the_executable_bit_is_checked_where_the_platform_has_it` and
  the owner-bit case above, both `GitBranchCase` mode cases
  (`test_a_worktree_executable_bit_git_status_cannot_report_is_still_caught` and
  `test_a_carried_normalisation_whose_worktree_mode_changed_is_still_caught`), and
  `test_a_stored_deviation_whose_executable_bit_changed_fails`; disabling
  only the stored-deviation path's call (`problem, mode_unverified_here = "", False`) failed
  `test_a_stored_deviation_whose_executable_bit_changed_fails` alone; and dropping the no-mode-bits
  disclosure (`if False else []`) failed `test_a_platform_without_mode_bits_discloses_them` and
  `test_a_platform_without_mode_bits_discloses_a_stored_deviations_mode_too`.
- the comparison's outcome rule (`internal/ccc/inspect.go`, 6 mutations). Dropping the held-row
  comparison failed the two attached-outcome verdicts and the extra-archived-outcome verdict; dropping
  the folded-row comparison failed `a_folded_result_whose_outcome_the_two_reads_disagree_about` and the
  end-to-end `TestInspectReportsADifferentFoldedResultOutcome` (its second half, the transcript the
  archive was written from, still agreed); dropping the "the archive shows no status row for this key"
  skip failed the call-row-hold verdict and the body-scope verdict that shares it; dropping the
  archive-side half of the set comparison failed `an_extra_archived_outcome_under_one_key`; and making
  an empty status a skipped value rather than a value — once on the archive side, once on the source
  side — failed, respectively, `an_attached_result_whose_outcome_only_the_source_records` (plus the
  extra-outcome verdict) and `an_attached_result_whose_outcome_only_the_archive_records` (plus the
  folded verdict and the end-to-end case).

Two mutations needed a second line removed to compile — the folded-row mutation left `foldedStatuses`
unused, and the skip-rule mutation left `ok` unused — and both were re-run from the compiling state;
every failure above came from a tree that built.

The round-9 pass closed four findings with the same rule, one rule at a time: **every rule the touched
code states was mutated, and every mutation failed the test that names it** (13 mutations). The observed
failures, in the order they were run, each line restored before the next:

- the retry state (`internal/ccc/reader.go`, 2 mutations). Carrying none (`NeedsRetry: false`) failed
  `TestInspectReportsARetryableResultWithItsMessages`, whose failure prints the read's own fidelity
  (`NeedsRetry:false RetryReason:could not read the Codex session it was forked from (…)`) and an empty
  warnings list; carrying the state without the provider's reason (`RetryReason: ""`) failed the same
  test's reason assertion and its warning check, the warning printed without the parenthetical reason.
- the retry disclosure (`internal/ccc/inspect.go`, 1 mutation). Disabling the branch failed that test's
  warning assertion alone — the state, the messages and the reason are asserted separately, so this is
  the line that pins the disclosure rather than the state.
- the transcript classification (`internal/ccc/reader.go`, `internal/ccc/archive.go`,
  `internal/ccc/inspect.go`, 4 mutations). Dropping the source read's field failed
  `TestInspectReportsASummaryTranscriptClassification` on the field and on the source warning; dropping
  the stored value (`ArchiveStatus.TranscriptFidelity`) failed the same test on the archive's field and
  its warning while the source half still passed; disabling either warning failed exactly its own
  warning assertion, with the other side's warning still in the failure's output.
- the reverse result direction (`internal/ccc/inspect.go`, 2 mutations). Handing the check no
  archive-side results (`unheldResults(nil, …)`) failed `a_result_the_archive_holds_and_the_source_read_does_not`,
  `a_result_the_archive_folded_and_the_source_has_no_row_for` and, end to end,
  `TestInspectDoesNotCallAFresherArchiveAgreement` — whose failure is the finding's own measurement
  (`same=true warnings=[]`). Emptying `foldedSourceResults` failed four comparison verdicts
  (`a_result_row_the_archive_folded_onto_its_call`,
  `a_folded_result_whose_archive_body_is_an_event_on_the_call`,
  `a_folded_result_whose_outcome_the_two_reads_disagree_about`,
  `an_archived_result_the_source_holds_on_a_folded_row`) and both folded end-to-end controls
  (`TestInspectReportsADifferentFoldedResultBody/unchanged_transcript`,
  `TestInspectReportsADifferentFoldedResultOutcome/the_transcript_the_archive_was_written_from`), which is
  what shows the folded keys are required by real Claude archives rather than a generosity.
- the call comparison (`internal/ccc/inspect.go`, 4 mutations). Comparing a spelling neither read
  retained (the input rule made a no-op) failed `a_call_whose_retained_input_the_two_reads_disagree_about`
  and, end to end, `TestInspectReportsADifferentToolCallInput` with `same=true warnings=[]` — the
  finding's own measurement; dropping the "both reads retain one" guard failed
  `a_call_whose_input_only_the_archive_retains`; dropping the archive-only-call report failed
  `a_tool_call_only_the_archive_holds`; and disabling the name comparison failed
  `a_tool_call_the_two_reads_name_differently`.

Two mutation attempts were corrected before their run and are not among the thirteen: one that kept `db`
imported by comparing zero-length slices (a form that disabled the rule only by accident), and one that
passed an empty key map at the call site, leaving `foldedSources` unused so the tree did not compile; the
second was re-run as a mutation of `foldedSourceResults` itself, and that is the failure quoted above.
Every recorded failure came from a tree that built.

The round-10 pass closed two findings with the same rule, one rule at a time: **every rule the touched
code claims was mutated, and every mutation failed the test that names it** (22 mutations, the whole
sweep re-run once after two rules were found unpinned by it). The observed failures, grouped by surface:

- the counts a summariser reports (`internal/ccc/conversation.go`, 12 mutations). Dropping each role's
  increment in turn failed `TestFixtureParseReportsIdentityAndConversation` (user, assistant, system
  records, compaction boundaries — the last through the CLI's JSON case as well) or
  `TestSummarizeCountsEachRoleSeparately` (the two roles no fixture reaches: system and tool). The
  finding's own defect — counting a tool-role message as a system one — failed the focused case, as did
  counting it nowhere, and so did a `default:` clause folding an unrecognised role into the system count,
  which is the rule that case's last message exists for. Dropping the sidechain or boundary increment
  failed the focused case; summing the *results* slice into `tool_calls` (and the calls slice into
  `tool_results`) failed it too — **the first sweep found both of those unpinned**, because each fixture
  holds exactly one call and one result, and the case now holds two calls against one result; and
  reporting no message total failed four cases, `TestIngestThenArchiveRead`'s status comparison among
  them.
- the disabled-provider state (`internal/ccc/reader.go`, 3 mutations). Answering a disabled provider with
  the plain error again — no sentinel — failed `TestInspectArchiveOnlyWhenTheProviderIsDisabled` on its
  `errors.Is` check, because the inspection returned the error before its archive branch: the finding's
  own measurement. Re-wording the sentinel failed the same case's check on the setting's own words, and
  building the reader's factory map from the full registry (`parser.ProviderFactories()`, the
  disabled-agents filter removed) failed it on the source report that must not exist.
- the reader-error classification (`internal/ccc/inspect.go`, 7 mutations). Disabling the
  disabled-provider clause (so the reference falls through to the default) failed the same case — the
  branch the finding named; shortening its warning failed that case's "why the archive is still
  consulted" assertion; making the provider-failure, host-qualified and not-found clauses fatal failed,
  respectively, `TestInspectExposesAProviderFailureAndFallsBackToTheArchive` and
  `TestInspectKeepsAProviderFailureWhenNoArchiveHoldsTheSession`; `TestHostQualifiedReferenceIsReportedWithoutASource`
  plus the CLI's `TestNoResultExitKeepsTheDiagnostics` in both spellings; and
  `TestInspectReportsAnArchiveOnlySession`, `TestDeletedSourcePathFallsBackToTheArchive` plus that CLI
  case's other diagnostic. Turning the default clause into a warning failed the case this pass added for
  it (`TestInspectReturnsAnUnclassifiedReadError`), which is what pins "a failure none of those
  classifications covers is an error" rather than an argument, and dropping the source report failed
  `TestInspectJSONReportsIdentityAndConversation`.

The two survivors of the first sweep are that paragraph's point rather than an aside: the one pair of
rules no fixture could tell apart (`tool_calls` against `tool_results`) was unpinned on *every* existing
count, and the focused case above now separates them. The sweep was then re-run in full on the finished
tree, where all 22 fail their named test; the driver verifies each file's bytes are back to what they
were before every next mutation, and every recorded failure came from a tree that built.

The round-11 pass closed two findings with the same rule, one rule at a time: **every rule the touched
code states was mutated, and every mutation failed the test that names it** (20 mutations). The first
complete sweep found two of them unusable — one rule survived it and one mutation did not compile — both
were addressed and the whole sweep was re-run, where all 20 failed their named test. Driver:
`/tmp/mutate-r11.py` (throwaway; hashes each file before and after, restores in a `finally`, refuses to
continue on a restore that is not byte-exact). The observed failures, in the order they were run, each
line restored before the next:

| Mutation | Where | Test that caught it | Observed failure |
| --- | --- | --- | --- |
| `archive-provider-filter` | `archive.go` `sessionIDsForProvider` | `TestFreebuffSessionResolvesThroughItsCodebuffProvider`, `TestHostQualifiedFreebuffRow…` | `while the source is there: the archive does not resolve codebuff:…` |
| `host-exact-predicate` | `archive.go` `resolveHostQualified` | `TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead` | `status of the spelled-out Freebuff row: more sessions end in this identity than a lookup can read` |
| `path-current-labels` | `archive.go` `sessionIDsForPaths` | `TestFreebuffSessionResolvesThroughItsCodebuffProvider` | `the archive does not resolve codebuff:<the transcript path>` |
| `path-retained-labels` | `archive.go` `sessionIDsForPaths` (retained listing) | same | `after the vanished source is reconciled: the archive does not resolve codebuff:<path>` |
| `provider-roots-owner` | `archive.go` `providerRoots` | same | `the relative freebuff path resolved found=false id="", want "freebuff:…"` |
| `providers-list-labels` | `reader.go` `Providers` | `TestProvidersNamesStoredAgentLabels` | `the provider list is missing "freebuff": [aider amp …]` |
| `known-provider-mapping` | `reader.go` `KnownProvider` | same | `KnownProvider("freebuff") = false, want true` |
| `known-provider-in-cli` | `reader.go` `KnownProvider` | `TestInspectAcceptsAStoredAgentLabel` | `exit 2, want 1 (stderr "ccc: unknown provider \"freebuff\"\n")` |
| `resolve-factory-alias` | `reader.go` `resolveFactory` | `TestFreebuffSessionResolvesThroughItsCodebuffProvider` | `reading the source under its stored agent label: unknown provider "freebuff" (known: aider, amp, …)` |
| `parse-label-carry-through` | `reader.go` `conversationFromParse` | same | `the by-label source read reports no agent label, want the one the parser recorded` |
| `inspect-agent-fallback` | `inspect.go` `Inspect` | same | `the inspection reports agent "", want the archived row's label "Freebuff"` |
| `path-pick-requires-complete` | `reader.go` `pickResult` | `TestReadRefusesAPathWhoseParseIsIncomplete`, `TestReadRefusesAPartialTraeContainer`, `TestPickResultRequiresTheReferenceIdentity/path_reference,_one_result,_incomplete_result_set` | the pick is back, so the sole result answers the path again |
| `read-reports-partial` | `reader.go` `Read` | `TestReadRefusesAPartialTraeContainer`, `TestReadRefusesAPathWhoseParseIsIncomplete` | `err = provider could not read the source: … produced no session` (absence again) |
| `partial-only-for-paths` | `reader.go` `partialPathParse` | `TestReadRejectsASingleResultThatIsNotTheReference` | `reading a fork that does not exist: err = provider could not read every session in the source: parsing 42-7 …` |
| `partial-keeps-empty-outcome` | `reader.go` `partialPathParse` | `TestReadRefusesAPathWhoseParseIsIncomplete` | `a path whose parse produced no result: err = provider could not read every session …` |
| `partial-names-identities` | `reader.go` `partialPathParse` | `TestReadRefusesAPartialTraeContainer`, `TestReadRefusesAPathWhoseParseIsIncomplete` | `the refusal does not carry "openclaw:main:good"` |
| `partial-carries-diagnostics` | `reader.go` `partialPathParse` | `TestReadRefusesAPathWhoseParseIsIncomplete` | `the refusal does not carry "openclaw:main:bad"` |
| `inspect-partial-clause` | `inspect.go` `Inspect` | `TestInspectNamesTheIdentityOfAPartialContainer` | `inspecting the container path: provider could not read every session in the source: …` (returned as an error) |
| `path-pick-still-answers` | `reader.go` `pickResult` (the positive half) | `TestReadAcceptsTheOnlyResultOfAPathReference`, `TestPickResultRequiresTheReferenceIdentity/path_reference,_one_result,_complete_result_set` | `reading by path: session not found: … produced no session` |
| `parse-ref-rejects-unknown` | `cmd/ccc/main.go` `parseRef` | `TestUsageErrors` | `ccc [inspect no-such-agent:whatever]: exit 1, want 2` |

Two mutations the first sweep could not use, one rule it did not cover, and what this round did about
that:

- `host-exact-predicate` survived its first form (reverting the predicate to a name comparison) because
  the suffix fallback reaches the same row whenever the reference spells the stored id — so the *rule*
  ("the archive's own spelling of a row is answered directly, and the provider predicate is what decides
  whether it answers") had no machine check. It is pinned now by
  `TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead`: 502 rows end in the same native id, so the
  candidate read cannot be completed at all and only the exact spelling can answer — the mutation makes
  that reference refuse with `ErrTruncatedCandidates` (the failure quoted above).
- the first form of `partial-names-identities` (`read = append(read, "?")`) did not compile — the loop
  variable became unused, so the run was a `[build failed]` rather than a verdict — and was re-run as
  `candidate.Result.Session.SourceSessionID` (the native id instead of the stored one), which is the
  rule's real alternative and fails the case.
- the sweep's first run had nineteen entries because its twentieth was added afterwards, when the pass
  noticed it had edited two existing cases' outcomes to state the completeness their stand-in provider
  must report: the *original* rule those cases pin — a complete sole result is the path's session — had
  to be shown pinned too (`path-pick-still-answers`), and dropping it fails both of them (the failure
  quoted in the table). The addition ran from the same driver with no other change, so the table above
  is one sweep's twenty entries, not two sweeps' results.
- one mutation attempt was a no-op *state*, not a mutation: the first run of the driver crashed midway
  (a missing `go` on the `PATH` it was given) leaving `archive-provider-filter` applied, so the restored
  file was the mutated one and the next sweep's first entry matched nothing. The driver now restores in a
  `finally`, and the sweep quoted above is the run taken from the corrected file (`git diff` shows the
  shipped predicate in place).

## 7. Measurements that shaped the harness

Run in a byte-identical tree (the pin, in a scratch clone) before the gate's targets were narrowed:

| Command | Result |
| --- | --- |
| `go test -count=1 -tags fts5 ./internal/parser` | rc=0, 156 s |
| `go test -count=1 -tags fts5 ./internal/db` | rc=1, build failure: `internal/vector`'s cgo binding needs `sqlite3.h`, which upstream's `Makefile` stages from the module cache |
| `go test -count=1 -tags fts5 ./internal/sync` | rc=1, exceeded a ten-minute default timeout |
| `go build ./internal/sync` | rc=0 (the module compiles without `-tags fts5`, but a binary built that way cannot load an archive that has an FTS5 index — hence the tag in `scripts/gate-goflags.txt`) |

The gate therefore targets `./cmd/ccc/... ./internal/ccc/...` and leaves upstream's own suites to
upstream's CI; `docs/harness.md` states this, and `parity` covers those files at the byte level.

The round-7 path lookup was measured the same way, on this host, with a throw-away test (since removed)
that seeded 20 000 sessions for one provider through `dbtest`, opened the archive read-only and timed
20 iterations of each query:

| Query | Mean per call |
| --- | --- |
| the inherited path-keyed lookup, `ListSessionIDsByFilePath` | 20.9 µs |
| the archive's per-provider row listing, `ListSessionPathRecordsForAgents` | 27.5 ms |
| one lookup answered from the listing (the listing plus its in-memory filter) | 27.9 ms |

The second query reads one row per session that provider has ever recorded, which is why the shipped
lookup asks it only when no spelling records a current member — the file is gone — and answers from the
indexed query while a source is there.

## 8. What this does not claim

- **Two providers, two fixtures.** Claude Code and Codex are the routes proven here; nothing about the
  other providers the pin supports is claimed, and no real user session was read.
- **Bytes, not behaviour, for the rest of the tree.** `parity` proves the carried files are the pin's;
  behaviour is covered for the parse → sync → archive-read path by the oracle comparison and the
  call/ccc tests, not for every inherited package.
- **Windows and macOS were not exercised on this host.** CI runs the same gate on all three; the
  Windows C-compiler boundary and the absent POSIX mode bits are disclosed in `docs/harness.md`.
- **Upstream's own test suites do not run in this gate** (measurements above).
- **The leak scan does not read the carried tree** (decision recorded in
  [`docs/decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md), Q-0001-03).
- **The leak scan can produce false positives.** One run flagged a Go constant in
  `cmd/ccc/main_test.go` under the `generic-api-key` rule — an identifier ending in an id-shaped name
  followed by a quoted file suffix, with no credential in it. It was fixed in the test (the fixture's
  session id is now read from the fixture file's own name instead of repeated in the test) rather than
  allowlisted, so the scan keeps its meaning; a future finding of this shape is worth the same
  treatment, and `gitleaks` exits non-zero and writes no report when a flag is wrong, which is worth
  checking when a scan looks clean.
- **No ingest command.** The write path exists in the engine for tests and the oracle; exposing it
  would need upstream's live-daemon check, which lives in a `package main` call/ccc may not import
  ([the plan](../plans/0001-agentsview-parity-and-inspection.md), §3.1, states that boundary and §6
  records the decision to ship no write command).
- **A result event's subagent attribution stays on the call.** A tool result a provider attaches to its
  call reaches a source read as the message's `ToolResult` (id, source, status, content and size) —
  the same mapping an archive read already used — but neither read carries the event's `AgentID` or
  `SubagentSessionID`, because `internal/ccc`'s `ToolResult` has no field for them; a spawned call's
  own `SubagentSessionID` is what the inherited parser sets on the call. The archive read additionally
  folds the result's length onto the call (`result_size` in the recorded dump, where the source read
  states it on the result row only), which is a difference in how the two reads describe the same
  evidence, not in whether they hold it.
- **A carried symlink is a link, or the file a checkout that does not create symlinks has at that
  path — and only where the checkout states that.** The two shapes are the same git object, so the
  shape is the only predicate that can tell the pin's link from a file that replaced it; the file form
  is accepted where git answers `core.symlinks=false` for the tree, or, when git cannot be asked, where
  the platform cannot create symlinks (Windows), and the run reports how many it accepted. A checkout
  that creates links and holds the file form is a failure (`parity: FAIL — … changed file type`), which
  is what the pre-fix check accepted on this tree's own `CLAUDE.md` with `skip-worktree` set (§3).
- **`parity` cannot verify a tree git cannot enumerate.** No `git` on the host, or a root that is a
  copy rather than a repository, leaves the file-list, ignore and generated-input index rules unasked, so
  the step reports `INCOMPLETE` and exits 2 instead of the pass with a disclosure inside its detail line
  it used to print; the required-step machinery in `scripts/gate.py` rejects any non-zero outcome. A host
  that needs a full pass therefore needs git — CI has it on all three runners — and the report a run
  prints names the rules it did not check.
- **The ambiguity branch is exercised, and what it still does not cover.** `AmbiguousError` and the
  CLI's "pass one of those session ids instead of a guess" line are implemented and pinned by tests —
  the source reader reports several candidates for a relative path two configured roots hold and for a
  transcript whose parse produced several sessions, and the archive reports every row of the requested
  host whose id ends in the requested native id — but the *committed fixtures* hold one session each, so
  nothing in this record makes an identifier match more than one source through them; the ambiguity cases
  build their own second root, their own second archive row, or their own forked transcript.
- **A relative filesystem reference is relative to the provider's configured roots, not to the working
  directory.** `claude:-home-user-code-example-api/<id>.jsonl` means that path under a configured Claude
  root; a process whose working directory happens to hold the same relative path is not what it names.
  The alternative reading (the shell's own, relative to where the command was run) is a one-function
  change in `internal/ccc/reader.go`, recorded in
  [`docs/decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md), Q-0001-10.
- **An archive opened without a configuration cannot resolve a relative reference.** The roots a
  relative path is relative to come from `config.Config`; `OpenArchive(path)` has none, so a relative
  spelling is only asked about as written and finds nothing, while `Inspect` (and the CLI) open the
  archive with the configuration they gave the reader. Absolute paths and ids are unaffected, and a
  test pins the refusal rather than leaving it to a comment.
- **A host-qualified reference is resolved by an exact lookup, then by a host-filtered one.** The
  archive's own spelling (`<host>~<the id it had there>`) answers first; a miss falls back to the
  inherited raw-suffix lookup narrowed to rows that carry the requested host *and* the requested
  provider, which is what lets `host~<native id>` reach a row stored as `host~<agent>:<native id>`. A
  host the archive has nothing from resolves nothing — never another host's row and never the local
  row of that native id — and two rows of the requested host are reported as candidates rather than
  picked (decision recorded as Q-0001-11). The exact answer is answered only for the provider the
  reference names, and the fallback reads its candidate list in full before filtering it: a list that is
  still full at 500 rows is refused with `ErrTruncatedCandidates` instead of being read as absence or
  uniqueness (Q-0001-13), so an archive holding more than 500 rows that end in one native id has no
  answer for such a reference — a shape a real archive would only reach by syncing hundreds of copies of
  one session.
- **A source path this process cannot stat is treated as a source that is not there.** The changed-path
  classification is kept only when a file stands behind it (the spelling the reference named, or the
  identity the provider maps it to — Q-0001-14), so a path whose permissions hide it from this process
  answers "no readable source session" and the archive, if it holds the session, is reported instead of
  a read failing part-way. The predicate is `parser.IsRegularFile`, the same one the inherited providers
  apply to a stored path.
- **A filesystem reference answers the source's current members, and the rows the archive retained
  when the source was reconciled away are the fallback.** A path is looked up through the archive's
  path-keyed query while any row records the source as a current member — the rows a parse of that file
  emits, which is what the *source* side answers for the same reference, so the two sides cannot
  disagree about a file that is there — and through the archive's per-provider row listing only when no
  spelling has one: the file is gone (or its rows are trashed), and the retained conversation is still
  the archive's answer to its own id, so it has to be its answer to the path too
  (`TestDeletedSourcePathFallsBackToTheArchive`, `TestRelativePathReferenceAsksEveryRootBeforeFallingBack`).
  The retired lookup (`ListSessionIDsByFilePath`) required `source_missing_at IS NULL`, which is right
  for its own callers — parsers tracking a live file — and wrong here. A union of the two tiers was
  rejected: it reports conversations the source side does not for a file that is present, and the same
  conversation can arrive through both tiers (the union mutation of this round's close-out fails
  `TestPathReferencePrefersTheSourcesCurrentMembers` by observing one conversation twice in one
  candidate list). The fallback is a per-provider listing rather than an indexed lookup (§7 measures
  the two), which is why it is the fallback and not the only lookup.
- **A tool result the archive stores only as a call row with neither content nor length cannot be
  recognized as held.** The comparison looks for the result the source row carries by tool-use id and
  content size, in the message's result rows, in the events the archive folded onto the call, and in the
  call row's own result content; a result stored as a call with an empty body *and* a zero length is
  indistinguishable from a call with no result at all, so an inspection reports that row as a
  difference. Every fixture and every provider route measured here records a length, which is what makes
  the answer exact for them.

- **The comparison matches by call id and content length, and compares bodies only where both reads show
  one.** For a row both reads have, the archived bodies come from that message's own result rows: a
  *summarized* call row holds text the source read never showed, so comparing the two would report a
  difference the archive's own read cannot support. For a **folded** row — the source's result-only
  message the archive stores on the call — the body is compared wherever the archive shows it, because
  the write path stored that very result there; either representation of the source's body (its raw
  content field, or the decoded text `parser.DecodeContent` produces) is accepted, one matching body
  among several under one key is agreement, and a body the archive does not show (a blocked category
  keeps the length and no text) is not a difference. A call stored with neither content nor a length
  stays indistinguishable from a call with no result at all (the bullet above), so the comparison
  reports that row as a difference by its key. The round-9 pass added the two directions the rule was
  missing, and they carry limits of their own. The archive's **own** result rows must now be held by the
  source read too (Q-0001-20), where the archive-side scope is those rows and not a call's own result
  content: that content is the writer's derived summary of a call's events — it merges several agents'
  outputs (`internal/ingest/tool_results.go`) — so its key is not evidence that the source recorded a
  result, and a result the archive kept *only* as a call's own content therefore still cannot be reported
  as archive-only. Tool **calls** are compared by identity, name and retained input (Q-0001-21), which
  means a call the *source* holds and the archive does not is not reported as a difference: a narrowed
  archive policy may keep a row and drop its calls, and that narrowing is already disclosed as what the
  read shows (`content_shown`). The comparison says nothing about a field neither read carries (a
  subagent attribution, a token count), by construction.
- **The reader mirrors the inherited machine lookup instead of importing it.** `internal/sync` resolves a
  source's machine with an unexported `configuredMachineForPath` plus `pathWithinRoot`, so
  `internal/ccc/reader.go` carries its own copy of those two rules (longest configured root containing the
  path, both sides through `pathutil.LocalComparisonKey`) and names the original in a comment. An upstream
  change to that resolution has to be mirrored here deliberately; the alternative — exporting the helper —
  is an upstream contribution, not a local edit to carried code.
- **A path reference that parses to more than one session reports the parsed sessions, not the path's
  first — and a path whose parse could not read the source in full reports neither.** A transcript
  holding a DAG fork parses to several sessions; a path names a source rather than
  one of them, so the reader reports their identities as candidates (`AmbiguousError`, the design's
  answer for an ambiguous identifier) instead of reporting the reference as absent. The alternative
  reading, "the file's main branch", is a choice this increment does not make — a caller passes one of
  the identities back. A fork identity the parse produced (`<stem>-<entry-uuid>`) names the session the
  archive stores, but names no file: the inherited source lookup resolves an id to `<id>.jsonl`, so an
  inspection of that identity reaches the archive instead of a source file (`TestReadReportsAForkedPathAsAmbiguous`
  pins the ambiguity, the candidate identities, and the not-found answer an id-only reference gets).
  A source a provider read only in part — some members returned, others recorded as source errors with
  `ParseOutcome.ResultSetComplete` false, the inherited container shape — names no single session
  either: the reader refuses to answer a *path* with the members it happened to read and reports
  `ErrPartialSource` with the provider's diagnostics plus the identities it did read
  (`TestReadRefusesAPathWhoseParseIsIncomplete` reproduces the inherited outcome field for field,
  `TestReadRefusesAPartialTraeContainer` drives it through the inherited Trae provider on a real
  container, and `TestReadReportsAnOpenClawContainersMembersInsteadOfPickingOne` records what the other
  container shape answers — OpenClaw's own stored-path lookup resolves members, so a container path
  reaches the changed-path enumeration and its members are the candidates). A reference that names a
  session id is unaffected by incompleteness elsewhere in the source: the identity it asked for is in
  the outcome or it is not.
- **A provider operation failure is reported as a failure, not as a missing session.** The reader
  classifies an outcome that carries operational `SourceErrors` and no result as `ErrProviderFailed` (the
  inherited Cursor provider's retryable store-enrichment shape) and keeps the provider's own diagnostics
  in the error; an inspection reports them as a warning and still offers an archived session, and the
  CLI prints an inspection's warnings when *neither* side answers (no source report and no archive
  report), so an operational failure is not collapsed into "no session found" (`TestNoResultExitKeepsTheDiagnostics`,
  `TestInspectKeepsAProviderFailureWhenNoArchiveHoldsTheSession`). A reference
  that resolves to no source at all, and a parse that produced no session without source errors, remain
  `ErrNotFound`. The classification covers the selected session's provider on the read path; a write-side
  retry policy is not part of this increment. A provider the configuration **disables for local
  ingestion** is a fourth state: the reader answers it with `ErrSourceDisabled` (the setting's own words,
  `provider "claude" is disabled in this configuration`) and an inspection reports the archive with a
  warning rather than the error, while reading or resolving that provider still fails — the setting
  restricts local discovery, and `docs/configuration.md` keeps previously archived sessions available
  (`TestInspectArchiveOnlyWhenTheProviderIsDisabled`). A source the provider could not read **in full**
  is the fifth: `ErrPartialSource` (Q-0001-22), refused for a path reference as the bullet above
  describes, reported as a warning beside the archive like the others.
- **The inherited provider-to-stored-agent mapping decides which archive rows a reference names, and a
  stored label is a name a reference may use.** The Codebuff provider also writes free-tier sessions:
  the parser reports `AgentFreebuff` and a `freebuff:<project>:<timestamp>` id for them, while
  discovery, caches and freshness stay keyed on Codebuff, and `parser.ProviderStoredAgents` is where
  upstream records that both labels belong to it. Every archive lookup that asks "is this row the
  requested provider's session" answers through that mapping (`providerStoresAgent` in `internal/ccc`
  for the id, host-qualified and path shapes, and `providerStoredAgentNames` where a query takes agent
  names) instead of comparing the row's `agent` column with the provider's own name — the comparison
  that dropped exactly those rows, so a Freebuff session read from its source disappeared from the
  archive once that source was gone. The label is also a *name* a caller can write: the reader resolves
  a name that is not a registry entry through the provider whose mapping includes it
  (`storedAgentOwner`), the CLI asks the core for that answer (`ccc.KnownProvider`) rather than doing
  its own registry lookup, and a relative path under such a name resolves against that provider's
  configured roots (Q-0001-23). The identity the read reports for such a session is the one its parser
  or archive row recorded, so an inspection of `freebuff:…` names the agent it belongs to instead of
  inventing a display name for a label the registry does not know
  (`TestFreebuffSessionResolvesThroughItsCodebuffProvider`,
  `TestHostQualifiedFreebuffRowResolvesThroughItsCodebuffProvider`,
  `TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead`, `TestProvidersNamesStoredAgentLabels`,
  `TestInspectAcceptsAStoredAgentLabel`). The cost of a second label is the shape §7 measured, not a new
  one: one more *indexed* query (`ListSessionIDsByFilePath`, ~21 µs) in the current tier, and a listing
  whose `agent IN (…)` covers the same rows the provider ever wrote, which is the ~27 ms query the
  listing always was.
- **The generated input's index check compares git's blob with the one the file's bytes hash to.** That is
  exact for the snapshot (a binary `.gz`, which git stores as it is) and would report a false difference
  for a future input that the tree's attribute file rewrites on `git add`: such an input would need the
  stored-bytes allowance a declared line-ending normalisation gets, which the manifest's `input` record
  does not have.

Observed through the built binary (`CGO_ENABLED=1 go build -tags fts5 -o /tmp/ccc-bin ./cmd/ccc`), the
no-result exit answers like this — exit 1, stdout empty, the inspection's own diagnostics on stderr
ahead of the summary line:

```
$ /tmp/ccc-bin inspect claude:00000000-0000-4000-8000-000000000000 --data-dir <dir>
ccc: no readable source session for claude:00000000-0000-4000-8000-000000000000
ccc: no archive at <dir>/sessions.db; reporting source facts only
ccc: no session found for claude:00000000-0000-4000-8000-000000000000 (providers: aider, amp, …)
```

## 9. Re-deriving this record

```bash
python3 scripts/upstream-import.py --upstream <checkout at the pin> --restore-snapshot --prune
python3 scripts/upstream-parity.py
go run ./internal/ccc/fixturegen                     # rewrites the fixtures; the test requires no diff
python3 scripts/upstream-compare.py --record         # needs network; rewrites the oracle records
python3 scripts/gate.py
```
