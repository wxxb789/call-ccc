# Harness — call/ccc

One command decides whether a tree is acceptable:

```bash
python3 scripts/gate.py     # must print GATE: PASS; CI runs this same file
```

## What the gate does

| Step | Tool | What it proves |
| --- | --- | --- |
| `gofmt` | `gofmt -l` over the tree's `*.go` (below) | formatting is not drifting |
| `build` | `go build <flags> <targets>` with `CGO_ENABLED=1` | the tree compiles with the inherited SQLite driver |
| `vet` | `go vet <flags> <targets>` | no obvious static defects |
| `test` | `go test -count=1 <flags> <targets>` | the tests actually run and pass |
| `selftest` | `python3 -m unittest discover -s scripts/tests` | the harness's own rules still hold |
| `parity` | `scripts/upstream-parity.py` (required) | every carried AgentsView file still matches the pinned revision |
| `secrets` | `gitleaks detect --no-git --source <scope>` when available | no credentials in the files this repository owns |

`gofmt` is the one step that also has to answer in a tree without git: it takes `git ls-files '*.go'`
when the tree is a repository (the normal case, CI included), and otherwise walks the tree skipping
`.git/` and a top-level `vendor/`. Both branches drop the same two things, and a nested `vendor/`
stays — a Go `vendor` directory is a module-root concept. Both branches also answer in one spelling:
POSIX-form relative paths (`deep/vendor/nested.go`) on every platform, which is what `git ls-files`
prints and what the walk normalises to with `as_posix()`. The list handed to `gofmt` — and any test
that pins it — therefore does not depend on the host's separator, and a regression to `str(relative)`
is what the suite's Windows-flavoured case exists to catch.

That list is handed over in **bounded batches**, not in one command: the carried tree holds thousands
of `*.go` files and Windows refuses a process command line past 32 767 characters — an `OSError`, which
`run()` answers as a step result (`rc=126`, never a traceback with no verdict). The batches together are
still the whole list, in the same order, and every batch's answer is aggregated: a batch that cannot be
checked and a file a later batch reports unformatted both fail the step. `gofmt_batches()` and
`GOFMT_BATCH_CHARS` (8 000 characters of paths per invocation) are the shipped rule, pinned by two cases
in `scripts/tests/test_gate.py` — one for the whole list staying whole and every command line staying
inside the budget, one for the answers of a later batch surviving.

`selftest` runs the suite with the interpreter that is running the gate (`sys.executable`), so the
suite must hold on every Python 3 the docs allow and not only on CI's pinned 3.13: it is stdlib-only
Python 3, and no case may depend on a `pathlib` behaviour that moved between minor versions —
`PurePath` parses its constructor arguments in `__new__` before 3.12 and in `__init__` from 3.12 on,
which is why the suite's Windows-flavoured root is a plain object holding `PureWindowsPath` children
instead of a `PureWindowsPath` subclass.

The same step must not depend on the *host* either, so two of its cases re-run the whole suite in a
child process — once with `PATH` pointing at a directory that holds no tools at all, once with the
platform probe answering the way Windows answers without a C compiler (`sys.platform`, and the
suite's own Windows-shaped tool lookup) — and assert that every case still ran and none flipped. The
child's copies of those two cases skip, keyed on `CALLCCC_SUITE_SELFCHECK`, so a nested run does not
nest a third. A case that reaches for a tool this host happens to have, or that reads the platform it
runs on, therefore fails in the selftest step instead of only in the one CI job nobody can watch; the
Windows answer is a simulation executed on the running host, never an assertion from memory about a
machine nobody in the loop has.

## `parity`: the carried tree against its pin

`scripts/upstream-parity.py` answers, offline, whether the AgentsView source this repository carries
is still the pinned revision: `docs/upstream.md` states the pin, the deviations and the update
procedure; `scripts/upstream-manifest.txt` is the machine-readable record. The step is **required**
(`scripts/gate-required.txt`), so a run in which it is skipped, bounded or absent fails.

It verifies, in this order:

1. `docs/upstream.md` records the pin, and `cmd/ccc/pin.go` reports it, so the revision the binary
   claims cannot drift from the revision the manifest describes;
2. every carried path is what git stores for it (`git ls-files -s`, i.e. what a fresh clone receives,
   with `.gitattributes` filters applied) — or, in a tree without git, what its bytes hash to;
3. every carried path's **working tree copy** holds the pinned bytes, shape and executable bit — hashed
   and compared to the manifest record in both branches, with git and without it, before git is asked
   anything. The bit compared is the **owner's** execute bit, because that is the bit git's own mode
   records (`100755` when `S_IXUSR` is set, `100644` otherwise): a copy carrying only the group's and
   another's execute bits (`0655`) *is* the `100644` file git stores, while a `100755` record's file
   whose owner bit was cleared is not executable to git whatever its other bits say. A carried symlink
   (a `120000` record) is a link, or the regular file holding the link's
   target text that a checkout which does not create symlinks has there — the two are the same git
   object, so the shape is the only thing that tells them apart — and the file form is accepted only
   where the checkout really stores links that way, which git's own `core.symlinks=false` (or a
   platform that cannot create symlinks) states; a broken `core.symlinks` query on a tree git is
   answering for fails the step naming the probe, and the run reports how many file-form links it
   accepted. A link whose target the *platform* re-spells is compared in that spelling: Git for
   Windows cannot create a symlink whose target holds `/` (its `mingw_create_symlink` rewrites each
   separator to `\`), so the link a checkout there creates for the pin's `../../x` reads back as
   `..\..\x` — the same destination, hashing to no blob the manifest holds. Only the separators are
   folded, only for a link, and only on the platform whose symlink layer does it, so a POSIX checkout
   and a target that changed by anything else are still failures; the run names how many links it
   matched that way. `git status` is the one answer a path can be configured to suppress: an `assume-unchanged`
   or `skip-worktree` index flag keeps an edited file out of git's report while the pinned blob stays in
   the index, so a check that read "git reports nothing modified" as "the file is the pin's copy" would
   pass for modified carried source. The record's *mode* is additionally compared with git's index
   (what a fresh clone receives), and a `git status` answer saying the file differs from the index is
   still a failure — except for a link whose target was matched in this platform's spelling: git there
   spells that link the same way and reports a checkout the pin itself creates, so its answer about that
   one link adds nothing to the shape and index rules around it — and a `git status` that cannot answer
   fails the step instead of dropping the rule;
4. no carried path matches the tree's ignore rules, which would make `git add` skip it in a fresh
   clone;
5. nothing git would add sits inside the tree unexplained: a tracked or untracked-but-not-ignored
   path is either named by the manifest or inside a prefix the manifest calls project-owned;
6. every declared deviation still holds what the manifest declares, read from the working tree (what
   the gate actually gated) and from git's index where git is available — a project-owned file that
   has quietly become upstream's copy is a stale declaration whether it was staged or only written on
   disk, and a path carried with a declared line-ending normalisation must still store the bytes its
   record pins, since upstream's own blob can never be what that path holds. A working-tree copy that
   still holds upstream's pre-normalisation bytes is *reported*, not failed: git reads that copy as
   clean and a checkout writes the declared bytes. Such a path is a carried path for the rules above
   as well — present in the working tree, unmodified relative to git's index, not matched by the
   ignore rules, and its *working-tree owner-executable bit* compared with the file this repository
   ships (`worktree_mode`, the same check a carried file's mode gets, in both branches) — because the
   declaration is an allowance for its *bytes*, never for its absence or its mode. The file's own bits
   are what answers it: a checkout with `core.filemode=false` records no mode change for `git status`
   or the index to report, so an executable bit flipped on that one path would leave every git-backed
   predicate agreeing with a modified tree;
7. the generated build input hashes to the SHA-256 that the carried snapshot tool pins in its own
   source, so the input is verified against upstream's pin rather than a second hard-coded copy — and,
   where git answers, that the **index** carries it: its presence there, the mode it stores and the blob
   its own bytes hash to. The carried `internal/pricing/snapshot/.gitignore` keeps the file out of every
   untracked listing, so a copy that is present on disk and ignored is compatible with every other rule
   here while a fresh clone receives nothing for the carried `go:embed` to compile.

Every git query the check makes is required. When git answers the file list but refuses the untracked
list, the index or the status report, the step **fails**, naming the probe: "git would add nothing" and
"nothing is modified" are the answers that look like a pass, and neither may come from a question nobody
answered. A tree git cannot enumerate at all is the separate INCOMPLETE verdict below — the difference
is that one is a tree nobody could list and the other a repository that answered some queries and not
others.

Hashing every carried file's working-tree bytes reads ~63 MB on the import host (5 553 carried files)
and costs well under a second there: round 3 measured about 0.8 s, round 8 re-measured the whole check
(hashes and git queries together, `time python3 scripts/upstream-parity.py`) at 0.40 s — the mode rule's
predicate changed no measured work, so the difference is page-cache state. The byte comparison is what
makes the `assume-unchanged` and `skip-worktree` flags irrelevant to it, so no index flag inspection is
needed.

A tree git cannot enumerate — no `git` on `PATH`, or a `--root` that is a copy rather than a
repository — is reported as **INCOMPLETE**, not as a pass: the unexplained-file rule, the ignore rule
and the generated input's index check cannot run there, so `upstream-parity.py` prints
`parity: INCOMPLETE — …`, names what it did not check, and exits `2`. The step is required and any
non-zero outcome fails the gate, so such a tree
can never be quoted as agreement with the pin. Everything that does not need a file list is still
checked first (carried bytes, deviations, the generated input's own bytes and its pin, the recorded
pin), so real drift in that tree is reported as `FAIL` with its offenders rather than as `INCOMPLETE`.

`scripts/upstream-compare.py` answers the other half — behaviour — and is deliberately **not** a gate
step, because it needs network and a second checkout: it fetches the pinned revision into a scratch
directory, copies `internal/ccc` into it so both sides run the *same* oracle program over the *same*
fixture bytes, runs parse → sync → archive read on both sides in their own throw-away archives, and
diffs the two canonical dumps. Each case **names the fixture it reads** (`internal/ccc/parity`'s
`Cases`, mirrored by the script's `CASES`), so a provider that grows a second transcript shape cannot
silently change what a recorded dump describes. A checkout it did not fetch itself (`--upstream`) must
be *clean* at the pin, exactly as the importer requires: the dumps are attributed to that revision, and
`--record` rewrites the committed oracle with them. `internal/ccc/parity/testdata/oracle/*.json` holds
the dumps a pristine checkout produced; the ordinary test suite re-checks this tree against them
offline. The increment's evidence record holds a run of it with its hashes.

## `secrets`: what it scans, and what it does not

The scan reads the files this repository **owns** — the prefixes the carry manifest records as
project-owned — staged into a throw-away directory so `--no-git` still means "current files only,
untracked and gitignored files included" within that scope. The carried AgentsView tree is not
scanned here: it is upstream's content, byte-verified by the `parity` step, and upstream's own
secret-detector corpora (`internal/secrets/rules_test.go` and friends) exist precisely to contain
credential shapes. The step's PASS line states the scope and the file count, so a run can never be
read as having scanned more than it did. A tree with no readable carry manifest is scanned whole, and
says so.

## Knobs (all in-repo, all auditable)

- `scripts/gate-targets.txt` — one Go package pattern per line (`#` comments allowed) for build/vet/
  test. Absent means `./...`.
- `scripts/gate-goflags.txt` — extra Go flags, one per line, folded into build/vet/test ahead of the
  targets. It carries `-tags fts5`, which is not cosmetic: without it the inherited SQLite driver has
  no FTS5 module, so a binary built here could not load an archive that has a full-text index, and
  the step would be quietly building a different product than upstream ships.
- `scripts/gate-required.txt` — one step name per line. A step listed here must be `PASS`: `SKIP`,
  `BOUNDARY`, `N/A` and `FAIL` all fail the gate, and so does an unknown step name. It lists `parity`
  and `selftest`. The Go steps are deliberately *not* listed: on Windows without a C compiler they are
  a disclosed platform boundary (below), not a failure.
- `scripts/upstream-parity.py` — the source-side parity check (above).
- `scripts/tests/` — the gate's, the parity checker's and the importer's regression tests, run by the
  `selftest` step.
- `CALLCCC_GATE_ROOT` — run the gate against another tree. Most cases in `scripts/tests/` pin a rule
  against a throw-away tree instead of trusting the reader.
- `scripts/codex-native-check.py` — the target-side native verification for the Claude Code → Codex
  route, and **deliberately not a gate step** for the same reason `scripts/upstream-compare.py` is
  not: it needs the Codex binary installed and, for a real turn, model access. It builds `ccc`,
  converts a committed fixture into an isolated Codex data directory (seeded with the host's own
  `config.toml` — settings, never credentials or sessions), plants an unrelated session there, runs
  `ccc continue … --json`, checks the resume invocation the conversion reported (its argument vector
  names the binary, `resume` and the session; its environment selects the data directory the session
  was installed into, which is the only way Codex is told where to look), re-reads the installed file
  with its own `ccc inspect` invocation and compares what the inherited reader made of it with what the
  conversion says it carried (a second process, so the conversion's own round-trip verdict cannot stand
  in for that read), asks Codex's own app server to load the session (`thread/resume`) and prints
  Codex's own view of it (`thread/read`), hashes the source, the planted session and every session the
  home already held before and after (removing the planted one again when the run ends), and — with
  `--resume-prompt` — runs one real turn from the reported invocation and prints the target's answer.
  Every app-server request is bounded by the script's own deadline (`APP_SERVER_TIMEOUT`), stdout is
  read by a thread and stderr drained by another, so a server that stops answering or fills its
  diagnostics pipe is reported as a failed check rather than hanging the run. Every check prints its
  own verdict and the script exits non-zero when one fails, so a run that could not do the native half
  says so instead of reporting a pass.

## Why the Go steps target a subset

`scripts/gate-targets.txt` is `./cmd/ccc/... ./internal/ccc/...`, and that is a disclosure, not a
convenience. Measured on the import host (4 cores, 7 GB, Ubuntu):

| Inherited suite | Result |
| --- | --- |
| `go test -tags fts5 ./internal/parser` | PASS, 156 s |
| `go test -tags fts5 ./internal/db` | build fails: the `sqlite-vec` cgo binding needs `sqlite3.h`, which upstream's `Makefile` stages from the module cache (`sqlite-vec-header`) |
| `go test -tags fts5 ./internal/sync` | exceeds a ten-minute default timeout |
| the module's PostgreSQL / ClickHouse / DuckDB lanes | start containers and are upstream's own CI's job |

So the local gate builds and tests what call/ccc builds and depends on — which compiles the inherited
`internal/parser`, `internal/db`, `internal/sync` and their dependencies anyway — and the inherited
*tests* are covered by `parity` (the files are the pin's, byte for byte) plus the oracle comparison
(those code paths run on fixtures, in both trees). Running more than that belongs to upstream's CI
and to `docs/upstream.md`'s update procedure, not to a gate that has to finish on a laptop.

## Platform boundary

Linux, macOS and Windows are the intended platforms, subject to native harness availability. On Linux
and macOS a missing Go toolchain or a missing C compiler is a **failure**: the gate cannot silently
degrade where the project is expected to build fully. On Windows a missing C compiler is a disclosed
boundary — the gate runs what it can and says so — because CGO on that platform depends on the
toolchain being installed separately. That boundary is the one host answer a step can turn on, so the
suite answers it from the case's own stub bin and exercises the boundary itself by forcing the
platform probe to its Windows answer (see the `selftest` section): no verdict in the suite depends on
which platform is running it.

Windows has no POSIX mode bits, so `parity` declines to check a carried file's — or a stored-byte
deviation's — working-tree owner-executable bit there and says so in its output instead of failing every
executable file; the *stored* mode still comes from git's own index, which records it on every
platform. For the same reason the Go test that pins an installed session's permissions asks for the
mode the installer *requests* (`codexSessionFileMode`) on Windows, and compares the installed file's
own bits with that request where the platform reports them: a widened mode must fail on the platform
where the answer is visible, so the two halves are asserted by one platform-aware branch.

Three more Windows answers the tree pins rather than tolerates, all because the platform's own answer is
not the one the rule is about:

- a missing binary raises `FileNotFoundError` **without a filename** on Windows — `CreateProcess`'s
  error carries a WinError only, where the POSIX error names the file it could not find — so the gate's
  step result names the command itself (`rc=127`, and the command in the detail) and the suite pins the
  Windows shape by raising exactly that error on any host, instead of reading the platform's message;
- a link's target is re-spelled: Git for Windows cannot create a symlink whose target holds `/`
  (`mingw_create_symlink` rewrites each separator to `\`), so the link a checkout there creates for a
  carried `120000` record reads back in that spelling and hashes to no blob the manifest holds. See the
  `parity` section above: the separators alone are folded, only for a link, only on the platform whose
  symlink layer does it, and the run names how many links needed it. The Windows-shaped tree is
  simulated on any host through the check's own seam (`platform_name`) and, where the host can create
  one, by writing the link's target in that spelling — the shapes are pinned by cases that run here.
- a Windows path holds backslashes, which a POSIX shell reads as syntax, so the resume line a
  continuation prints quotes the data directory it names there (`CODEX_HOME='C:\…\codex home' codex
  resume …`) where a POSIX path of the same shape stands bare. The rule is "quote where a shell would
  act on the word", not "print the path as this host spells it", so the two CLI cases that read that
  line back install into a directory whose name holds a space — a word the rule quotes on every
  platform — instead of asserting this host's own temp path, which is shell-safe here and shell-unsafe
  there. The Windows-shaped input is exercised on any host by pointing `TMPDIR` at a directory whose
  name holds a backslash: the same two cases then fail against a host-shaped assertion (executed) and
  pass against the rule.

## CI

`.github/workflows/gate.yml` runs the same `scripts/gate.py` on `ubuntu-latest`, `macos-latest` and
`windows-latest` for every pull request and every push to `main`. Its step spells the interpreter
`python`, the name all three runners have — the Windows runner has no `python3` — while the file is
`#!`-tagged `python3` for the hosts where that is the only name. CI installs Go and gitleaks; a CI job
with fewer tools than another is the boundary above, not a different gate. The Go toolchain comes
from `go.mod` (`go-version-file`), so the workflow and the module cannot disagree about the version
the tree builds with.

## Growth policy

The gate grows by **adding a step to `scripts/gate.py` and (when it must always run) a line to
`scripts/gate-required.txt`** — not by adding a second script that CI calls separately, and not by
adding a check that only runs on the author's machine. A check that exists in a shell history or a PR
description does not exist. When a new check is introduced, this file and `AGENTS.md` say so in the
same increment.
