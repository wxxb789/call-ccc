# Evidence — 0001 Inherit AgentsView and inspect real sessions

Contract: [`docs/goals/0001-agentsview-parity-and-inspection.md`](../goals/0001-agentsview-parity-and-inspection.md).
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
    "counts": { "messages": 8, "user_messages": 3, "assistant_messages": 5, "system_records": 1,
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

## 2. A real source-to-archive parse and a subsequent archive read agree with the pinned implementation

The oracle (`internal/ccc/parity`) parses a fixture, writes it through the inherited sync engine into
an isolated SQLite archive, reads it back through the inherited read path, and emits a canonical dump:
content is hashed, host-dependent fields are excluded and named in the dump itself. The comparison
harness runs the *same* oracle source in this tree and in a pristine checkout of the pin, over
separately copied fixture bytes and separately created archives.

```
$ python3 scripts/upstream-compare.py --record --scratch <tmp>/ccc-compare-0001-r2
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) from https://github.com/kenn-io/agentsview
fetched the pinned revision into <tmp>/ccc-compare-0001-r2/upstream (clean, at the pin)
- restored internal/pricing/snapshot/litellm_snapshot.json.gz in the pristine tree from upstream's pinned artifact
- copied internal/ccc (10 Go source file(s)) from this tree into the pristine checkout: the oracle and its call/ccc wrapper are identical on both sides, so the only variable is which tree the inherited packages come from
oracle source: 10 file(s), sha256=0080eb0bffb8adb6bbf0d1bf36c70b2c02ddf89b9eeacbc84956a76ee01fbf55 (identical on both sides)
claude: fixture sha256=2bd510821d4abf83… ours=32244340553137fd… pristine=32244340553137fd… agreement=yes
  recorded internal/ccc/parity/testdata/oracle/claude.json
codex: fixture sha256=d9957a8d6f5731a3… ours=fe8d542a39799f8f… pristine=fe8d542a39799f8f… agreement=yes
  recorded internal/ccc/parity/testdata/oracle/codex.json
RESULT: PASS — 2 case(s) agree byte for byte with the pinned revision      # rc=0, 48 s
```

The same command against an existing clean checkout (`--upstream /path/to/agentsview`) reports the
same two dump hashes and removes the copy it made (`git status` clean afterwards); a checkout that is
not clean at the pin is refused before anything runs, because the dumps — and, with `--record`, the
committed oracle — are attributed to that revision.

The two dumps the pristine side produced are committed and re-checked offline by
`internal/ccc/parity/dump_test.go`, so the ordinary test suite — and so the gate — fails if this
tree's inherited code stops producing them:

```
$ sha256sum internal/ccc/parity/testdata/oracle/*.json
32244340553137fd9af173c84eb9c17c94d655c055af5e536dee0cc8d2daa712  internal/ccc/parity/testdata/oracle/claude.json
fe8d542a39799f8fe973cb74b9ca2df86665e5f348c6a00d0e18eebf0e5e9a91  internal/ccc/parity/testdata/oracle/codex.json
```

The Codex dump changed in this increment's fix pass, and only in its `parse` section: the wrapper now
carries a call's own result events into the conversation (`internal/ccc/reader.go`, the same mapping
`messageFromArchive` already used), so a Codex `function_call_output` reaches a source read instead of
being dropped. The pristine side reproduces the change byte for byte, which is the point of recording
from it: the adapter's correction is visible as agreement, not as a local edit to a dump.

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
working-tree comparison above is that fix, and it costs about 0.8 s and ~63 MB of file reads over this
tree's 5 553 carried files (measured by hashing every path in
[`scripts/upstream-manifest.txt`](../../scripts/upstream-manifest.txt)); three cases in
`scripts/tests/test_upstream_parity.py` script git's own answers with an empty `git status`, a pinned
index blob and a modified working tree (bytes, shape and executable bit), and each fails when the
working-tree comparison is removed from the shipped check. The same round pinned the rules that branch
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
$ python3 scripts/gate.py        # rc=0, 25.2 s wall clock, taken on the final tree
gate: <repo>
gate: go=<go>/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3574 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 134 tests in 6.057s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 25 project-owned prefix(es); files seen by git: 5613 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored byte…
  PASS     secrets  no leaks found in 60 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The gate truncates the middle of a long step detail, which is why the `parity` line above opens with `…`;
the same run's full summary is quoted in §3. The `selftest` count in that run is 127, counting both
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
`test_gate.py` unchanged at 33).

Wall clock for the whole gate on this host: 19–21 s with warm caches across the increment's earlier
runs, 25 s to 156 s across the round-2 fix pass's runs (the 156 s run followed a cold Go build cache, so
`build`/`vet`/`test` recompiled the inherited packages), 21–23 s across the round-3 runs, 22 s, 26 s and
53 s across the three round-4 runs, and 21.8–25.0 s across the round-5 runs. The six round-6 runs
measured 26.5 s, 25.2 s, 45.8 s, 28.3 s, 33.9 s and 31.1 s, and the round-7 runs 26.1 s, 28.8 s, 25.2 s
and 55 s. The block above quotes the 25.2 s run, taken after every code and test edit the round made;
the 28.8 s run beside it measured the same tree, the 26.1 s one preceded the test-helper refactor, and
the 55 s one was taken last, on the finished tree — the only write after it is the sentence that quotes
its own number, and its `selftest` step took 12.0 s against 5.5–6.1 s in the three runs before it, so
that wall clock is host load and not a slower gate. With `selftest` steps
between 4.76 s and 6.18 s across them, the wall-clock spread is host load rather than a slower gate (the
28.3 s run's step was the slowest at 6.18 s, the 45.8 s run's among the fastest at 4.96 s).
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
  reports that row as a difference by its key.
- **The reader mirrors the inherited machine lookup instead of importing it.** `internal/sync` resolves a
  source's machine with an unexported `configuredMachineForPath` plus `pathWithinRoot`, so
  `internal/ccc/reader.go` carries its own copy of those two rules (longest configured root containing the
  path, both sides through `pathutil.LocalComparisonKey`) and names the original in a comment. An upstream
  change to that resolution has to be mirrored here deliberately; the alternative — exporting the helper —
  is an upstream contribution, not a local edit to carried code.
- **A path reference that parses to more than one session reports the parsed sessions, not the path's
  first.** A transcript holding a DAG fork parses to several sessions; a path names a source rather than
  one of them, so the reader reports their identities as candidates (`AmbiguousError`, the design's
  answer for an ambiguous identifier) instead of reporting the reference as absent. The alternative
  reading, "the file's main branch", is a choice this increment does not make — a caller passes one of
  the identities back. A fork identity the parse produced (`<stem>-<entry-uuid>`) names the session the
  archive stores, but names no file: the inherited source lookup resolves an id to `<id>.jsonl`, so an
  inspection of that identity reaches the archive instead of a source file (`TestReadReportsAForkedPathAsAmbiguous`
  pins the ambiguity, the candidate identities, and the not-found answer an id-only reference gets).
- **A provider operation failure is reported as a failure, not as a missing session.** The reader
  classifies an outcome that carries operational `SourceErrors` and no result as `ErrProviderFailed` (the
  inherited Cursor provider's retryable store-enrichment shape) and keeps the provider's own diagnostics
  in the error; an inspection reports them as a warning and still offers an archived session, and the
  CLI prints an inspection's warnings when *neither* side answers (no source report and no archive
  report), so an operational failure is not collapsed into "no session found" (`TestNoResultExitKeepsTheDiagnostics`,
  `TestInspectKeepsAProviderFailureWhenNoArchiveHoldsTheSession`). A reference
  that resolves to no source at all, and a parse that produced no session without source errors, remain
  `ErrNotFound`. The classification covers the selected session's provider on the read path; a write-side
  retry policy is not part of this increment.
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
