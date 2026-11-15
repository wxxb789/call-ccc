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
$ python3 scripts/gate.py        # rc=0
gate: <repo>
gate: go=<go>/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3574 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 110 tests in 4.818s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 25 project-owned prefix(es); files seen by git: 5613 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 60 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The gate truncates the middle of a long step detail, which is why the `parity` line above opens with `…`;
the same run's full summary is quoted in §3. The `selftest` count is 110 in this run: the round-2 fix
pass added the four carried-path cases for a declared line-ending normalisation (deleted from the
working tree, modified in it, mode changed in git's index, matched by an ignore rule) and the case that
pins the incomplete verdict for a tree git cannot enumerate, bringing it to 104; the round-3 fix pass
added six more in `scripts/tests/test_upstream_parity.py` — three for a working tree `git status`
cannot report (bytes, shape and executable bit, each removed-fix-mutation-tested), one for a carried
path present on disk but absent from git's index, one for a record whose pinned size is not its bytes'
size, and one for a symlink record whose path became a directory (74 → 80 parity cases). The Go side
moved with it: `internal/ccc` went from 17 to 24 test functions (`archive_test.go` 13 → 16,
`fixtures_test.go` 4 → 8), and `cmd/ccc`'s tests are unchanged.

Wall clock for the whole gate on this host: 19–21 s with warm caches across the increment's earlier
runs, 25 s to 156 s across the round-2 fix pass's runs (the 156 s run followed a cold Go build cache, so
`build`/`vet`/`test` recompiled the inherited packages), and 21–23 s across the round-3 runs, whose
`selftest` step took 4.3 s to 5.3 s. The `secrets` count is the files under the manifest's
project-owned prefixes, which includes the harness's own `scripts/tests/__pycache__/*.pyc` once the
suite has run here (58 without them, 60 with the two this run's selftest left behind). `gofmt` covers
the carried tree as well (3574 Go files); `build`/`vet`/`test` cover the three call/ccc packages, which
compile the inherited packages they depend on.

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
- **`parity` cannot verify a tree git cannot enumerate.** No `git` on the host, or a root that is a
  copy rather than a repository, leaves the file-list and ignore rules unasked, so the step now reports
  `INCOMPLETE` and exits 2 instead of the pass with a disclosure inside its detail line it used to
  print; the required-step machinery in `scripts/gate.py` rejects any non-zero outcome. A host that
  needs a full pass therefore needs git — CI has it on all three runners — and the report a run prints
  names the two rules it did not check.
- **The ambiguity branch is exercised, and what it still does not cover.** `AmbiguousError` and the
  CLI's "pass one of those session ids instead of a guess" line are implemented and pinned by tests —
  the source reader reports several candidates for a relative path two configured roots hold, and the
  archive reports every row of the requested host whose id ends in the requested native id — but the
  *committed fixtures* hold one session each, so nothing in this record makes an identifier match more
  than one source through them; the ambiguity cases build their own second root or their own second
  archive row.
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
  picked (decision recorded as Q-0001-11).

## 9. Re-deriving this record

```bash
python3 scripts/upstream-import.py --upstream <checkout at the pin> --restore-snapshot --prune
python3 scripts/upstream-parity.py
go run ./internal/ccc/fixturegen                     # rewrites the fixtures; the test requires no diff
python3 scripts/upstream-compare.py --record         # needs network; rewrites the oracle records
python3 scripts/gate.py
```
