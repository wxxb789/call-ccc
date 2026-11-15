# Plan — 0001 Inherit AgentsView and inspect real sessions

Contract: [`docs/goals/0001-agentsview-parity-and-inspection.md`](../goals/0001-agentsview-parity-and-inspection.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Written before implementation; the numbers below were measured first (see `docs/evidence/`).

## 1. What this increment adds

1. The **actual AgentsView source tree**, at a pinned revision, carried inside this repository
   unmodified, with a reproducible importer and an offline parity check.
2. A **continuation core** under `internal/ccc/` that reads a source session into a typed,
   transient Conversation document and reads an archive through AgentsView's own read path. No CLI,
   terminal, HTTP or process-global state enters it.
3. A standalone **`cmd/ccc`** CLI whose `inspect` command reports identity, agent, source and
   conversation information for `provider:session-id` (human and `--json`), read-only by default.
4. **Evidence machinery**: sanitized native fixtures, an oracle program that drives parse →
   sync → archive read, a committed oracle dump recorded from a pristine upstream checkout, an
   offline test asserting our tree reproduces it, and a network harness that re-derives it.

## 2. Carry: what exactly is inherited, and how it is proven

### 2.1 Pin

| | |
| --- | --- |
| Remote | `https://github.com/kenn-io/agentsview` |
| Revision | `83195c8a6861b8c74a4b6cb87512817fed6f8584` |
| `git describe` | `v0.47.0-181-g83195c8a` (181 commits past the newest tag) |
| Commit date | 2026-11-14T16:19:32Z |
| Module | `go.kenn.io/agentsview` (unchanged), `go 1.27.4` |
| Tracked files | 5580 |

The tip of upstream `main` is pinned rather than the newest release tag because the parser, sync and
DB code moved 605 files / ~43k lines between them, and a user's live sessions come from the current
format versions. The pin is immutable and recorded everywhere it matters; moving it is a re-run of
`scripts/upstream-import.py --pin <rev>`.

### 2.2 Declared deviations (the whole list)

The tree is carried byte-for-byte (5553 of 5580 tracked files), except:

| Deviation | Paths | Reason |
| --- | --- | --- |
| Project-owned files already here | `.gitignore`, `AGENTS.md`, `README.md`, `docs/README.md` | call/ccc's identity and control files; the upstream copies at these four paths are not carried |
| Upstream CI definitions | `.github/**` (22 files) | Actions reads `.github/workflows/` at the repository root: carrying upstream's workflows would run upstream's CI (its runners, secrets and desktop builds) on every call/ccc pull request |
| Line-ending normalisation | `frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json` | upstream's own `.gitattributes` (`* text=auto eol=lf`) makes `git add` rewrite the single CRLF pair the 1.4 MB blob holds, so this one path is carried with the byte git stores instead of being skipped or carried verbatim |
| Generated build input | `internal/pricing/snapshot/litellm_snapshot.json.gz` | upstream does not track it (`snapshot/.gitignore`); every Go-compiling Make target restores it first, because `internal/pricing/fallback.go` embeds it. Restored from upstream's own pinned artifact, verified against the SHA-256 constant compiled into the carried `internal/pricing/cmd/litellm-snapshot` |

Nothing else is added, removed or renamed. In particular `frontend/`, `desktop/`, `docs/`,
`plugins/`, `testdata/` and the root tooling configs are carried: carried upstream tests read
`../../frontend/src/hold-first-paint.js`, `../../desktop/...`, `../../plugins/...` and
`../docs/screenshots`, so excluding any of them would leave an inherited test that cannot pass.

### 2.3 Manifest and the gate's `parity` step

`scripts/upstream-manifest.txt` (generated, committed) records, per carried path: mode, git blob
SHA-1 and size, plus the pin header and the declared deviations above. The blob hash is git's own
(computed with `git hash-object`, i.e. filters applied), so `.gitattributes` line-ending
normalisation cannot turn a clean tree into a false failure.

`scripts/upstream-parity.py` (offline, no network) verifies in this order:

1. every carried path is what git stores for it (`git ls-files -s`: filters applied, i.e. what a
   fresh clone receives) — and, in a tree without git, what its bytes hash to — and no carried file is
   modified in the working tree: the working-tree copy is hashed, shaped and mode-checked against the
   manifest record in *both* branches, because `git status` is an answer a path can be configured to
   suppress (`assume-unchanged`, `skip-worktree`) while the pinned blob stays in the index. git's own
   status and index answers are still consulted (the index mode, and a worktree `git status` reports as
   modified);
2. each declared deviation still differs from upstream, so a project-owned file that quietly became
   upstream's copy is a stale declaration rather than a pass;
3. the generated snapshot's SHA-256 equals the constant compiled into the carried snapshot tool, so
   the build input is verified against upstream's own pin instead of a second hard-coded copy;
4. `docs/upstream.md` records the pin and `cmd/ccc/pin.go` reports it, so the revision the binary
   claims cannot drift from the revision the manifest describes;
5. nothing git would add is unexplained: every tracked or untracked-not-ignored path is named by the
   manifest (carried, declared or generated) or sits inside a project-owned prefix.

`parity` becomes a **required** gate step (`scripts/gate-required.txt`): SKIP/BOUNDARY/`N/A` fail it.

`docs/upstream.md` states the pin, the deviations, the update procedure (`scripts/upstream-import.py
--pin <rev>`), and the one thing parity cannot prove (it checks bytes, not behaviour — §4 covers
that).

### 2.4 Build inputs and flags inherited from upstream's Makefile

- `CGO_ENABLED=1` (the inherited SQLite driver needs CGO) — already the gate's environment.
- `-tags fts5`: upstream builds every shipped binary with it, `internal/db` refuses to *write* an
  archive whose `messages_fts` cannot load, and a binary built without the tag cannot read an
  archive that has one. The gate therefore needs to pass tags: a new knob
  `scripts/gate-goflags.txt` (one flag per line) folds into build/vet/test.
- `sqlite3.h` staging (`<tree>/.sqlite-include`, Makefile target `sqlite-vec-header`) is *not* done:
  it is needed only when `internal/vector`'s cgo binding compiles, which happens for `internal/db`'s
  test binary, and the gate's targets exclude that package's tests (measured reasons in
  `docs/harness.md`). Leaving it out keeps the gate free of a build step that depends on the host.

## 3. Engine and CLI shape

### 3.1 `internal/ccc` — core, no CLI types

```go
type Ref struct{ Provider, ID string }              // "claude:9f2…", or a native path
type SourceIdentity struct{ … }                     // provider, ids, paths, project, cwd, branch,
                                                    // session kind, provenance version, parent/relationship
type Message struct{ Ordinal int; Role string; Content, Thinking, Dialogue string; Timestamp time.Time
                     Model string; ToolCalls []ToolCall; ToolResults []ToolResult
                     IsSidechain, IsCompactBoundary bool; ContextTokens, OutputTokens int64 }
type Conversation struct{ Source SourceIdentity; Messages []Message; Fidelity Fidelity }

type SourceReader struct{ … }                       // inherited provider discovery + parse
func (r *SourceReader) Read(ctx context.Context, ref Ref) (Conversation, error)
func (r *SourceReader) Resolve(ctx context.Context, ref Ref) (SourceIdentity, bool, error)

type Archive struct{ … }                            // inherited db read path (read-only, no lock)
func OpenArchive(ctx context.Context, path string) (*Archive, error)
func (a *Archive) Status(ctx context.Context, id string) (ArchiveStatus, error)
func (a *Archive) Read(ctx context.Context, id string) (Conversation, error)

type Ingestor struct{ … }                           // inherited sync engine into an explicit DB path
func NewIngestor(cfg IngestConfig) (*Ingestor, error)   // DBPath required; never a default data dir
func (i *Ingestor) IngestPaths(ctx context.Context, paths ...string) (IngestResult, error)

func Inspect(ctx context.Context, opts InspectOptions) (Inspection, error)
```

Rules the code keeps: the core imports no CLI/HTTP/terminal package; no process-global state;
`Ingestor` takes an explicit absolute database path and holds the data directory's write-owner
`db.write.lock` (upstream's own lock, taken with the same `gofrs/flock` dependency) for the duration
of the write, and the CLI exposes no ingest at all, so nothing call/ccc ships can aim a write at a
live AgentsView data directory by accident; `Archive` opens read-only, so it needs no writer lock and
cannot fence the user's daemon. The one boundary this leaves is stated rather than hidden: call/ccc
cannot perform upstream's live-daemon check (it lives in a `package main` it may not import), so a
daemon that is running but not currently holding the lock is not detected by it.

### 3.2 `cmd/ccc`

```
ccc inspect <provider>:<id-or-path> [--json] [--data-dir DIR]
ccc help
```

Human output: identity block, archive status when an archive is present, conversation summary
(counts, roles, tool calls, time range, first user message), upstream-parity provenance line
(pin + `unchanged` when the parity check agrees). `--json` prints one stable object on stdout with
diagnostics on stderr. Writing to an archive is deliberately not a CLI command in this increment:
a write path that a human can point at a live data dir is not needed for goal 0001, and the
archive writes the contract asks for happen through `Ingestor` in tests and in the oracle.

### 3.3 Interpretation of the contract's clauses

- *"identified through AgentsView's existing discovery/archive semantics"* — `inspect` resolves the
  reference with the inherited provider (`FindSource`), and consults the archive with the inherited
  read path when one is present. A session that exists only in an archive is reported as such
  instead of being re-parsed from a guess.
- *"callable without CLI-specific types"* — `Inspect`, `SourceReader`, `Archive` and `Ingestor` are
  the callable surface; `cmd/ccc` contains argument handling and presentation only.
- Nothing is claimed for a provider without a fixture: the two proven routes are the fixture set
  actually parsed in the tests (§4).

## 4. Evidence plan

| Clause | Artifact |
| --- | --- |
| CLI inspects a real session, non-empty fixture | `internal/ccc/testdata/fixtures/{claude,codex}/…` (sanitized, version-identified, built with the inherited `internal/testjsonl` builders), `ccc inspect … --json` output recorded in the evidence record; `internal/ccc/*_test.go` assert identity and conversation content |
| parse → archive → read agrees with pinned upstream | `internal/ccc/parity` oracle (imports inherited packages only, so the same file compiles in a pristine checkout), committed `oracle` dumps, an offline test comparing our tree's dump to the committed one, and `scripts/upstream-compare.py`, which re-derives the dump from a fresh checkout at the pin and diffs it |
| no project-owned modification of inherited parser/sync/DB/schema/tests | `scripts/upstream-parity.py` output; manifest counts |
| core callable without CLI types; standalone CLI | package boundaries + a test that drives `Inspect` directly; `go build ./cmd/ccc` and a real `--json` run |
| source and user archive intact | tests hash the fixtures before/after and write only inside `t.TempDir()`; the oracle runs against copied fixtures and isolated DB paths; `Archive` is read-only; `Ingestor` takes upstream's write-owner lock and is not reachable from the CLI |
| no credential call/ccc commits | the gate's `secrets` step scans the paths the manifest calls project-owned (decided in `open-questions` Q-0001-03: upstream's credential-shape corpora are upstream's test data) |

The comparison is fair by construction: both sides compile the same oracle source over their own
inherited tree, parse the same fixture bytes, sync into their own isolated SQLite archive with the
same engine configuration, and emit a canonical dump that hashes content and excludes host-dependent
fields (file mtime, absolute paths, database path) — the exclusions are named in the dump itself.

## 5. Gate, docs, bookkeeping

- `scripts/gate.py`: `parity` already runs when the hook exists; this increment adds the
  `gate-goflags.txt` knob (and, if needed, `sqlite3.h` staging), plus selftest cases in
  `scripts/tests/test_gate.py`; `scripts/gate-required.txt` gains `parity`.
- `scripts/gate-targets.txt`: narrows build/vet/test to the packages call/ccc builds and depends on
  (the full upstream module needs Docker-backed PostgreSQL/ClickHouse suites, the web UI build and
  far more memory than this gate budgets). The narrowing, its reason and the command that runs more
  are stated in `docs/harness.md` and in the increment report.
- `AGENTS.md` grows: layout (`cmd/ccc`, `internal/ccc`, `docs/plans`, `docs/evidence`,
  `docs/upstream.md`), the carry rule and its parity step, the new knob, and the read order.
- `docs/conventions.md` (layout + inherited-tree rule), `docs/harness.md` (knobs, steps, narrowed
  targets), `docs/goals/README.md` (status), `docs/decisions/OPEN-QUESTIONS.md` (Q-0000-03 settled
  by this increment's evidence; new entries for the defaults this run had to take alone),
  `README.md` (what the tool is now), `.github/workflows/gate.yml` (its own TODO: switch to
  `go-version-file: go.mod` once the module lands).
- Bookkeeping: no goal file under `docs/goals/` is accepted yet, so nothing moves to
  `docs/goals/archive/` in this increment; the index status for 0001 is updated to say the
  implementation is on the branch and awaiting acceptance.

## 6. Out of scope, with reasons

- Target writers, `continue`, `formats --json`, capability tables: goals 0002+. Anything published
  now would have to claim writers that do not exist.
- Writing into a live AgentsView data directory: the write-lock owner lives in `cmd/agentsview`
  (`package main`) and cannot be imported; a second writer implementation is exactly what the design
  forbids. This increment therefore takes upstream's existing lock rather than inventing one, writes
  only where the caller names the database, and exposes no write command.
- Re-implementing anything the inherited tree already does (discovery, parsing, schema, migration,
  FTS, projections): the carry *is* the implementation.
- Coverage claims for providers without a fixture.

## 7. Ambiguities and how they are being handled

1. **"Agree with the pinned upstream implementation"** — read as a controlled comparison against a
   pristine checkout at the pin, not as a restatement of byte parity. Recorded in §4.
2. **Pin choice** (release tag vs `main` tip) is arguably owner-only; the reversible default taken
   here is the `main` tip with its exact revision recorded, and the alternative is one command away.
   Disclosed in `OPEN-QUESTIONS.md`.
3. **Excluding upstream's `.github/`** is a deviation from "carry the tree"; it is declared,
   enumerated and justified rather than left implicit.
