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

Exit codes from the same binary: found `0`, session not found `1` (with the provider list on stderr),
unknown provider `2`, `help` `0`.

The tests that hold this down: `internal/ccc/fixtures_test.go` (identity, counts, tool call, boundary,
malformed-line count, tool names, time range, and that no fixture contains a home directory, this
checkout or a credential shape), `cmd/ccc/main_test.go` (both output shapes, the usage errors, the
not-found path, help without an environment, and that `inspect` never creates an archive).

## 2. A real source-to-archive parse and a subsequent archive read agree with the pinned implementation

The oracle (`internal/ccc/parity`) parses a fixture, writes it through the inherited sync engine into
an isolated SQLite archive, reads it back through the inherited read path, and emits a canonical dump:
content is hashed, host-dependent fields are excluded and named in the dump itself. The comparison
harness runs the *same* oracle source in this tree and in a pristine checkout of the pin, over
separately copied fixture bytes and separately created archives.

```
$ python3 scripts/upstream-compare.py                      # fetch mode: clones the pin itself
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) from https://github.com/kenn-io/agentsview
fetched the pinned revision into <tmp>/upstream
- restored internal/pricing/snapshot/litellm_snapshot.json.gz in the pristine tree from upstream's pinned artifact
- copied internal/ccc (10 Go source file(s)) from this tree into the pristine checkout: the oracle and its call/ccc wrapper are identical on both sides, so the only variable is which tree the inherited packages come from
oracle source: 10 file(s), sha256=294041862d332d7ae2c4078356cf8c4a40c3f6d832a98b32139f4fdb0ec41684 (identical on both sides)
claude: fixture sha256=2bd510821d4abf83… ours=83a32277577fc32e… pristine=83a32277577fc32e… agreement=yes
codex: fixture sha256=d9957a8d6f5731a3… ours=db0295aba8d0eec4… pristine=db0295aba8d0eec4… agreement=yes
RESULT: PASS — 2 case(s) agree byte for byte with the pinned revision      # rc=0, 37 s
```

The same command against an existing clean checkout (`--upstream /path/to/agentsview`) reports the
same two dump hashes and removes the copy it made (`git status` clean afterwards).

The two dumps the pristine side produced are committed and re-checked offline by
`internal/ccc/parity/dump_test.go`, so the ordinary test suite — and so the gate — fails if this
tree's inherited code stops producing them:

```
$ sha256sum internal/ccc/parity/testdata/oracle/*.json
83a32277577fc32ee2c11cd4d5a900ae4945d14c2590bcaefb416d5f4a2f61f8  internal/ccc/parity/testdata/oracle/claude.json
db0295aba8d0eec436d691944a9d2ecf309e9add33dfa78ee07103caa3053c23  internal/ccc/parity/testdata/oracle/codex.json
```

The dumps carry the archive round trip's shape: for the Claude fixture, parse reports 8 messages with
1 tool call, 1 tool result and 1 compaction boundary; the archive read reports the same tool call and
boundary at the same ordinals, the tool-result-only row the archive folds into its call (and the
result's length, because the inherited default configuration blocks result *bodies* for the `Read`
category), and one malformed line on both sides — the truncated tail is counted, never turned into a
message. For the Codex fixture the shell result body survives the round trip intact.

## 3. No project-owned modification of the inherited parser/sync/DB/schema code and tests

```
$ python3 scripts/upstream-parity.py        # rc=0
pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a)
reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584
carried: 5553 file(s) identical to the pinned revision (compared through git's own object store)
deviations: 27 upstream path(s) not carried + 1 generated input(s), all present as declared
owned: 25 project-owned prefix(es); files seen by git: 5613
inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go)
symlinks hashed by their link target: 4
declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
```

Every declaration, and nothing else:

| Kind | Count | What |
| --- | --- | --- |
| project-owned `.gitignore`, `AGENTS.md`, `README.md`, `docs/README.md` | 4 | call/ccc's own files live at these paths |
| upstream CI definitions `.github/**` | 22 | would run upstream's CI here |
| line-ending normalisation of one frontend fixture | 1 | upstream's own `.gitattributes` rewrites its one CRLF pair on `git add` |
| generated build input | 1 | upstream does not track the LiteLLM snapshot; its Makefile restores it before every Go build |

The check reads git's own index (`git ls-files -s`) — the bytes a fresh clone receives, filters
applied — and fails if a carried file is modified in the working tree relative to it, if a declaration
has gone stale, if the generated input stops matching the SHA-256 the carried snapshot tool pins, if
`docs/upstream.md` or `cmd/ccc/pin.go` disagrees about the revision, or if anything git would add is
neither recorded nor inside a project-owned prefix. It needs no network and no upstream checkout.

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
  PASS     selftest Ran 69 tests in 4.967s (rc=0)
  PASS     parity   pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) not carried + 1 generated input(s), all present as declared | owned: 25 project-owned prefix(es); files seen by git: 5613 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md frontend/src/lib/utils/__fix…
  PASS     secrets  no leaks found in 58 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

Wall clock for the whole gate on this host: 18 s with warm caches in this run (53 s cold, measured earlier
in this increment). `gofmt` covers the carried tree as well (3574 Go files); `build`/`vet`/`test` cover
the three call/ccc packages, which compile the inherited packages they depend on.

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
- **The ambiguity branch is not exercised by a fixture.** `AmbiguousError` and the CLI's "pass one of
  those session ids instead of a guess" line are implemented — the source reader reports several
  candidate sources for a changed path, and the archive reports every session row that names one
  source file, instead of taking the first — but the committed fixtures hold one session each, so
  nothing in this record makes an identifier match more than one source.

## 9. Re-deriving this record

```bash
python3 scripts/upstream-import.py --upstream <checkout at the pin> --restore-snapshot --prune
python3 scripts/upstream-parity.py
go run ./internal/ccc/fixturegen                     # rewrites the fixtures; the test requires no diff
python3 scripts/upstream-compare.py --record         # needs network; rewrites the oracle records
python3 scripts/gate.py
```
