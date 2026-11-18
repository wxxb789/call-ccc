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
runs on, therefore fails in the selftest step instead of only in the one CI job nobody can watch; a
child that breaks is named by the case that read it — the verbose runner leaves a case's own line
dangling, with no status after its `...`, when the case's only reports come from subtests, and that
line is not a case line: reading it as one raised `IndexError` in the parent and the broken child's
name never reached the gate's log (a case here runs that shape and pins that the case is named); the
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
   accepted. The shape answer itself is read through a named seam (`symlink_target`), so a case can put
   the file form at a link record and say what target the link holds on a host whose checkouts cannot
   create one; "is anything at this path?" and "what bytes does it hold?" stay with the filesystem,
   because git stores a link as its target text and the file form's bytes are that same content. A link
   whose target the *platform* re-spells is compared in that spelling: Git for
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
- `scripts/tests/` — regression tests for the gate, the parity checker, the importer, the native check
  and the continuation evidence's own claims (`test_evidence_claims.py`: a §5 comparison cell that
  credits a recorded run's answer may only quote words that run's recorded output holds;
  `test_gate_transcript_claims.py`: a record's quoted `parity` line is the gate's own rendering — its end
  is the checker's own — and the elision limit a record states is the gate's own call; and
  `test_codex_native_check.py`'s `RecordStatesTheChecksOwnOrder`: the record's claim about *when* the
  comparison taken where the copy grows runs is read against the positions of the native check's own
  calls, so a record that drifts back to a placement the script does not have fails there), run by the
  `selftest` step.
- `CALLCCC_GATE_ROOT` — run the gate against another tree. Most cases in `scripts/tests/` pin a rule
  against a throw-away tree instead of trusting the reader.
- `scripts/codex-native-check.py` — the target-side native verification for the Claude Code → Codex
  route, and **deliberately not a gate step** for the same reason `scripts/upstream-compare.py` is
  not: it needs the Codex binary installed and, for a real turn, model access. It builds `ccc`,
  converts a committed fixture into an isolated Codex data directory whose `config.toml` is **seeded
  from the host's own configuration, setting by setting, never copied as a document**: the model
  settings that answer the loader's question and the named provider's definition, and never a setting
  whose value can hold a credential (a provider's authorization headers, an MCP server's environment
  table, a credential written into a URL). Its report line names what was copied and what was left
  behind, and a provider whose credential is written into the host configuration must take it from the
  environment. The conversion reads a **copy** of the fixture under the run's own scratch directory,
  never the committed bytes: the copy is what grows, by the committed
  `internal/ccc/testdata/growth/claude-complete-turn.jsonl` fragment, and the copy — not the fixture —
  is digested around each conversion, compared with the bytes it had before the first conversion immediately
  before it grows (so the baseline every later comparison measures from is the bytes that conversion read,
  across its loader and its optional turn), and digested once more at the end of the run, after the second
  loader and the repeat turn: the final comparison that answers "the source session is unchanged" covers the
  copy as well as the committed fixture tree and the growth fragment, so a mutation of the copy during the
  second native stage is caught by the comparison that runs last. The script plants an unrelated session in
  that home, runs `ccc continue … --json`, checks the resume invocation the conversion reported (its
  argument vector names the binary, `resume` and the session; its environment selects the data directory the
  session was installed into, which is the only way Codex is told where to look), re-reads the installed
  file with its own `ccc inspect` invocation and compares what the inherited reader made of it with what the
  conversion says it carried (a second process, so the conversion's own round-trip verdict cannot stand
  in for that read), asks Codex's own app server to load the session (`thread/resume`), prints Codex's
  own view of it (`thread/read`), runs — with `--resume-prompt` — one real turn from the reported
  invocation and prints the target's answer, and only then hashes the source, the planted session and
  every session the home already held against their bytes before the conversion (the planted session is
  removed once every comparison has run), so a mutation during the loader or the turn is answered by the
  comparison that runs last rather than by an earlier verdict — an ordering the native check's own cases
  pin offline.
  **The repeat scenario (goal 0004) runs in the same script, after the first conversion's own checks:
  the copy grows by the committed fragment, the same session is converted a second time into the same
  home, and the script then checks what the goal asks a caller to be able to see — the second
  conversion's reported boundary is the grown copy's own bytes (measured by the script, not taken from
  the report), including the read's own coverage answer (`read_bytes`/`read_coverage`: the file the
  conversion read is still the size that read's own snapshot recorded — a size match, which is what that
  record can show), the appended turn's words are in the second installed document and not in the first,
  the first session's bytes are unchanged by the second conversion, and Codex's own loader opens the
  second session with its own `thread/read` view holding the turn the source gained, while the first
  session's own view does not.** `--repeat-resume-prompt` adds one real turn in that second session (one
  turn per prompt flag, and the report line says how many ran), which is what answers the later
  snapshot's model-visible history with the target's own words; the script's own cases pin the ordering
  (the comparison about the first session runs after the second conversion, so a second conversion that
  touched it is caught; and a copy changed while the second session's own loader runs is caught by the
  final comparison, which covers it), that the attribution comparison is checked against the file rather
  than trusted from the document, and that the search for the appended turn is live in both directions.
  **What the second half does not re-check:** the second conversion's installation block and its
  rendered resume line are not held to the two checks the first conversion's own are (the session is
  installed under the target's sessions tree; the reported command selects the installation home). The
  same route code writes both, and the renderer's spelling is pinned by `cmd/ccc`'s own cases, so the
  second document is consumed for its identity, its path, what it carried and its boundary.
  The two rendered command lines (`resume_command`, `exec_resume_command`) are checked by **reading
  them the way a POSIX shell reads them**: word by word — a home the renderer quotes, any path holding
  a space or a Windows backslash, is accepted, while an unquoted home a shell would split into two words
  is not — and on the spelling that decides whether the shell reads the line's opening word as the
  assignment at all: a quoted `'CODEX_HOME=…'` reads back as the very same words while a shell takes it
  as the *program* to run, and a character a shell acts on left unquoted (`;`, `$`, a backquote, `~`) is
  refused for the same reason. That half is pinned offline, without the Codex binary, by
  `scripts/tests/test_codex_native_check.py` in the `selftest` step (the renderer's own spelling is
  pinned by `cmd/ccc`'s tests); the substring form it replaced accepted exactly the line the quoting
  exists to prevent and rejected correct renderings on Windows. Its fixtures spell every home so both
  `pathlib` flavours give it back unchanged, and each case drives both of them — the constraint the
  last bullet under Platform boundary below states — so a fixture that holds on one platform alone
  fails the suite on any host. The cases that drive this script's `main` derive their whole scratch
  tree from one **resolved** root, because `main` resolves the repository and the home it is handed and
  compares against that spelling: a case spelled through its own temporary root disagrees with the run
  it drives wherever `resolve()` re-spells that root — macOS's `/var/folders/…`, which reads back as
  `/private/var/folders/…`, the shape that failed `macos-latest` at head `4cd0567` with the first
  stubbed command (`--repo`, `--target-home`) filed under a spelling the case's own stub did not
  recognise. They resolve the root before deriving anything from it — the same rule the homes above
  follow, taking the platform's own answer instead of assuming this host's — and they spell the two
  reported command lines the way the renderer does, every word quoted where a shell would act on it;
  the shape they got wrong there, and the `windows-latest` failure at head `1a476e0` it caused, is the
  third Windows answer under Platform boundary below.
  Every app-server request is bounded by the script's own deadline (`APP_SERVER_TIMEOUT`), stdout is
  read by a thread and stderr drained by another, so a server that stops answering or fills its
  diagnostics pipe is reported as a failed check rather than hanging the run. The lines that drainer
  retains are a bounded tail — the last `APP_SERVER_DIAGNOSTICS_MAX` (200) of them, each cut to
  `APP_SERVER_DIAGNOSTICS_LINE_MAX` (2000) characters, so at most
  `200 × (2000 + len(" …[the rest of this line was dropped]"))` characters stay in memory — and it reads
  in bounded pieces: a line that does not end inside that bound is kept as its head while its remainder
  is read and dropped, so neither many lines, nor one enormous line, nor a stream the server never ends
  can grow this harness's memory with it, while the pipe stays empty so the server cannot block on it.
  Every check prints its own verdict and the script exits non-zero when one fails, so a run that could
  not do the native half says so instead of reporting a pass.
- `scripts/claude-native-check.py` — the target-side native verification for the Codex → Claude Code
  route, and **deliberately not a gate step** for the same reason: it needs the Claude Code binary
  installed, and its `resume` half also needs model access. The **`load` half needs no model access**: it
  drives the installed binary (`claude --bare -p --output-format json --resume <id>`) against a
  **throw-away copy** of the converted bytes in a scratch `CLAUDE_CONFIG_DIR`, with the environment
  variables that could reach a model removed, and adds a **control arm** running the same command on a
  fresh unknown session id, which must answer `No conversation found with session ID: <id>` — that
  contrast is what makes the check an answer about *this* session rather than about any file, and no turn
  is billed. The **`resume` half runs one real turn** through the invocation the conversion printed,
  which carries the session's working directory as well as its home (`cd <cwd> && CLAUDE_CONFIG_DIR=<home>
  claude --resume <id>`), because Claude Code resolves a session only from the directory whose project-dir
  spelling matches the one it was installed under. That line is a POSIX one (`cd … && CLAUDE_CONFIG_DIR=…
  claude …`, each word quoted where a POSIX shell would act on a character in it), so the shell that reads
  it is **named by the script** rather than left to whatever `subprocess`'s `shell=True` selects: on a
  POSIX host the platform's own `/bin/sh`, on any other host the first `sh`/`bash` on `PATH` (`posix_shell`),
  and a host with neither is reported as a named **skip** with that reason — the way a host without POSIX
  mode bits skips the owner-only check — because `cmd.exe` cannot read that line at all and running it
  there would fail before the harness was reached, answering about the host's command shell instead of the
  conversion. The check's own line says which shell read it (`ran through /bin/sh: …`). The host's model
  access is **inherited** by that turn, never copied into an isolated home; the run hashes the source
  fixture, a planted unrelated session and every session the home already held before and after, so a
  mutation during the loader or the turn is answered by the comparison that runs last.
- `scripts/pi-native-check.py` — the target-side native verification for goal 0005, both directions, and
  **deliberately not a gate step** for the same reason the other two are not: it needs the target harness
  installed and, for a real turn, model access. It converts a **copy** of a committed fixture whose
  recorded working directory has been rewritten mechanically to a directory inside the run's own scratch
  tree (Pi resolves a session id through the project directory spelled from the process's own working
  directory, so a session filed under a path no host has can never be loaded by it), plants an unrelated
  session in the target home, and reads the conversion's own report rather than trusting it: the
  structural read-back's state, the loss categories, and the installed file's own record shapes (one
  `session` header, the message roles, the tool-call blocks and their `toolResult`, and the `usage` object
  Pi's loader indexes into). It then asks **Pi's own loader**, offline, in its RPC mode
  (`pi --mode rpc --session <id>`, then `get_state`/`get_messages`) — against a **throw-away copy** of the
  converted bytes in a scratch agent directory, because measured on 1.5.2 an RPC run appends its own
  records (a `thinking_level_change` among them) to the session it opens, so driving the loader against
  the installed file would alter what the conversion published; the installed file's digest is compared
  around that whole stage, which is what makes the copy an assertion rather than a preference. The loader
  must answer with this session's id and with the file's own model-visible history — the walk the loader
  itself makes from the file's last entry back through `parentId`: the latest compaction entry *on that
  path*, rendered as `compactionSummary`, then every message entry after it on the path. The count is
  *not* the file's `message` count, measured, and not every entry in the file either: an entry the file
  holds but no later record hangs off is recorded history the loader does not build context from, and a
  compaction entry that stands off the path is not a boundary it rebuilds from (the Pi provider's own
  reader answers a record after a tool result as hanging off the message that made the call, so the path
  is read from the file's own `parentId` values and never from a document this build holds). An
  unknown-id control arm must answer `No session
  found matching '…'`, which is what makes the first arm an answer about *this* session rather than about
  a process that started. With `--resume-prompt` the run also drives one real turn through the invocation
  the report printed, prints the target's own answer under `TURN_OUTPUT_MARKER`, and requires the session
  file to have grown. Both the printed line and the loader stage name the **sessions tree inside the data
  directory** explicitly (`PI_CODING_AGENT_SESSION_DIR`, the project directory the installer filed the
  session under, `piSessionTree`), because Pi resolves a session directory as `--session-dir` first, then
  that variable, then the data directory's own `settings.json` `sessionDir`, and only then its default
  tree: measured on 1.5.2, a caller's leftover setting, or a `settings.json` naming another directory,
  otherwise sends the printed line looking where the session is not, while the check — which used to
  remove the variable — found it. The loader home is a throw-away directory holding the check's copy of
  the session and **none of the caller's configuration**: a home the caller names with `--target-home`
  may hold a `models.json` with literal API keys or authorization headers, and copying that document
  would copy a secret the conversion contract copies nowhere (measured: with the sessions tree named, the
  loader answers `get_state`/`get_messages` from a home holding the session alone, since an RPC run is
  started with `--offline --no-tools`).
  The scratch home a run makes for itself holds a `models.json` seeded from the
  host's provider definitions with every credential-bearing field dropped — a provider's `apiKey` and
  `headers`, a `models` entry's `headers` and a `modelOverrides` entry's `headers`, every level Pi's own
  schema allows one — and the chosen provider's key left as an `$ENV` reference, so nothing is read from
  or written to a credential store; a **selected provider whose endpoint URL itself carries a credential
  (userinfo, a query) is left out whole** rather than copied with its address rewritten, the same policy
  the Codex check applies to a provider's `base_url`, and the run's report names it and records the
  named skip; a home the caller names with `--target-home` is **never seeded** — the seed would replace a
  configuration the caller keeps there, and the comparisons this run makes cover session files only — so
  the provider check reports what that home already defines, and the converted session the run installs
  there is the file it leaves (a requested turn grows it). A host whose harness is
  absent ends as `RESULT: INCOMPLETE` (exit 2) naming what could not run, never as a pass: for
  `--direction claude-to-pi` a missing Pi binary is that verdict before any check runs, while the
  `pi-to-claude` direction needs no Pi at all (its source is a committed fixture read by the inherited
  reader and its native half is Claude's loader). It builds `ccc` into `bin/`, which `.gitignore` covers,
  so a run leaves no untracked file for the `parity` step to report; `--pi PATH` names the Pi binary the
  *conversion* is driven through — the resolved executable is what the CLI is handed as
  `--target-binary` and what the version query and the loader stage name, so a binary outside `PATH` is
  the one verified rather than a second installation a bare name would find — and, for the
  `--direction pi-to-claude` half, `--claude PATH` names the Claude Code binary to drive (its absence
  from `PATH` is exactly the case that flag exists for). The fabricated unrelated session it plants is
  removed again in a `finally`, so a home the caller named is left as it was found apart from the
  converted session (and its own configuration, which the run never touched).
- `scripts/omp-native-check.py` — the target-side native verification for goal 0006, both directions, and
  **deliberately not a gate step** for the same reason the others are not: it needs the OMP binary
  installed and, for a real turn, a model its data directory resolves. It shares the Pi check's shape —
  a **copy** of a committed fixture with only its recorded working directory rewritten mechanically to a
  directory the run owns, a planted unrelated session, digests taken before and after, the conversion's
  own report read rather than trusted, the harness's own loader asked against a **throw-away copy** of the
  converted bytes (a load appends a `custom`/`session_exit` record to the session it opens, measured on
  18.18.7), and an unknown-id control arm — and adds what is OMP's own: the project directory the session
  is filed under is asked of `OMPProjectDir`'s rule and compared with the file's real location (measured:
  the harness shortens a working directory under the temporary tree to `-tmp…` and one under the home to
  `-…`, keeps Pi's `--…--` form only outside both, and *moves* a session it finds under Pi's spelling);
  the file's first line must be the harness's own fixed-width (256-byte, line feed included) title slot;
  every assistant record must carry a non-empty `model` **and** `provider` (measured: dropping either from
  one record of an otherwise loadable session makes the next turn fail while it builds its request); and
  the loader's own answers are compared with the roles the file's own `parentId` walk builds, the latest
  compaction entry **on that path** rendered as `compactionSummary`. The loader arm runs in a throw-away
  copy the run makes (`ccc-omp-native-loader-`) of its own seeded data directory, with the
  credential-bearing variables (`ompLoadEnvDenied`) taken out of the environment, which is what makes "the
  load made no model turn" a measurement on a host that holds a key.
  The data directory a run makes for itself is seeded from the host's own OMP home (`PI_CODING_AGENT_DIR`,
  else `~/.omp/agent`) as a **source of provider definitions only**: every provider whose `baseUrl` is
  loopback is copied with its `api`, its models and its other non-credential fields, its `apiKey` **value**
  replaced by a placeholder this run invents and its `headers` dropped, while a provider whose endpoint is
  not loopback — or any of whose endpoint-bearing lines carries a credential (its own `baseUrl`, or one a
  model entry writes) — is left out whole and named in the report,
  because OMP demands a literal key (measured: without one it refuses to start at all, and `$VAR`/`${VAR}`
  in `apiKey` is **not** expanded, which is where OMP differs from Pi's `models.json` and why the collision
  policy is "the key's value is this run's own placeholder" rather than Pi's `$ENV` reference). Those
  fields are read as YAML itself spells a key, not by a literal prefix — a quoted key (`"apiKey"`) and a
  key with whitespace before its colon (`headers :`) are the same field to any YAML reader, and a key at
  another casing (`Headers`) is rewritten or dropped too (the conservative read: a spelling some reader
  might fold to the field must not pass unscrubbed, and dropping one the harness would ignore costs
  nothing), so none of those spellings can carry a value past the replacement or the drop, and a quoted
  value's escapes are decoded the way YAML reads them before any check sees the value (`"…\u003f…"` is the
  `?` the credential check must find, and a quoted value whose escapes this run's reader cannot decode is
  refused rather than guessed at). A definition holding a form this run's reader cannot follow is left out
  whole with that reason rather than copied unread, because each such form can spell fields the filter
  never read: YAML's explicit `? key`, a quoted key it cannot unquote, an entry — or a value of a field the
  block keeps — spelled in flow form (`- {id: …, baseUrl: …}`, `models: [{…}]`), a value under a tag,
  anchor or alias, a block scalar (whose content lines this reader does not read as fields), a nested
  sequence, an empty key, a quoted value that does not close on its line, and content after a quoted
  value's closing quote. With no surviving provider the home names no model role, the conversion writes no
  binding, and the run reports that as a named check instead of crashing — OMP's own measured behaviour for
  a home with no model. A home the caller names with `--target-home` is never seeded and nothing is
  planted in it — the converted session the conversion installs there is the file the run leaves, and a
  requested turn grows it — and the run reports both the role its own textual read finds in that home's
  `config.yml` and, when the conversion's own probe answered, the model the harness's own `get_state`
  resolved for it (the answer that
  decides the turn when the reading cannot follow the home's spelling) beside its every-session digests.
  `--resume-prompt` is a request for one real turn: the run attempts it whenever any of the seed's role,
  that reading and the harness's own answer names a model, and a requested turn the run could not attempt
  is recorded as a **failed** check — never a pass and never a skip the verdict ignores.
  The `--direction omp-to-claude` half needs no OMP at all (its source is a committed OMP fixture read by
  the inherited reader and its native half is Claude Code's loader, driven the way 0003's check drives it);
  a host without the harness that direction needs ends as `RESULT: INCOMPLETE` (exit 2) naming it. The
  script builds `ccc` into `bin/ccc-native-check-omp` (`.gitignore` covers `bin/`), takes the same
  `--resume-prompt`/`--expect`/`--keep`/`--json` flags as its siblings, and prints the turn's own answer
  under `TURN_OUTPUT_MARKER`.
- `scripts/native_check_common.py` — the mechanism the native checks share, so none keeps its own
  copy of it: the file digests (`sha256`, `tree_digest`), the check recorder (`record`, `RESULTS`), the
  subprocess runner (`run`), the POSIX-shell reading of a rendered command (`shell_words`,
  `unquoted_text` and the character set a shell acts on), the JSONL record-shape test the checks report
  through (a line may be blank or one JSON **object**; anything else — unparseable, or a value that is not
  an object — is that check's FAIL with the line's number), the two strings the record's own rules
  read (`TURN_OUTPUT_MARKER`, `PROMPT_PLACEHOLDER`), the fixture-copy mechanism (`rewrite_source_cwd`:
  the working directory rewritten through the session's own JSON `cwd` record field, never its raw
  bytes, with a torn line carried through verbatim), the printed-block bound (`bounded_block` with
  `REPORT_BLOCK_LINES_MAX`/`REPORT_BLOCK_CHARS_MAX`), and the endpoint policy both seeding checks apply
  (`credential_free_url`: a URL whose userinfo, query or fragment would write a credential into a
  configuration the check copies, so the Codex check leaves that provider's table behind whole and the Pi
  check leaves the provider out of its seeded document).
  It is project-owned (`OWNED` and the manifest), imports nothing, and each check stays runnable on its
  own; what the shared module does **not** absorb is the checks' independent restatements of the
  contracts they check (the spelling rules their case files re-implement, the marker their case files
  declare for themselves, the project-directory rule their own test carries), because a reading that
  imported those from the thing it checks would answer nothing.

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

Five more Windows answers the tree pins rather than tolerates, all because the platform's own answer is
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
  simulated on any host through the check's own seams (`platform_name`, and `symlink_target` for the
  shape of the link itself): the cases run both shapes a host can hold a link in — the file form, with
  `symlink_target` answering the target the link holds, and the host's own real symlink where it can
  create one — so a host whose checkouts cannot create a symlink pins the fold and the exemption
  instead of falling back to asserting the file form's own failure.
- a Windows path holds backslashes, which a POSIX shell reads as syntax, so the resume line a
  continuation prints quotes the data directory it names there (`CODEX_HOME='C:\…\codex home' codex
  resume …`) where a POSIX path of the same shape stands bare. The rule is "quote where a shell would
  act on the word", not "print the path as this host spells it", so the CLI cases that read that
  line back install into a directory whose name holds a space — a word the rule quotes on every
  platform — instead of asserting this host's own temp path, which is shell-safe here and shell-unsafe
  there. The Windows-shaped input is exercised on any host by pointing `TMPDIR` at a directory whose
  name holds a backslash: the two cases that first did this then fail against a host-shaped assertion
  (executed) and pass against the rule. The third case to read the line back — the resume verification's
  own command assertion the sweep added — is exercised with a temp root whose name holds a **space**, for
  the same reason and with the same result: `gate (windows-latest)` failed the `test` step at head
  `37c0a10` because that assertion spelled this host's temp path, and it now installs into the same
  shell-unsafe home. A backslash-shaped root cannot stand in for that case on this host (measured: the
  stub harness finds the installed session with `filepath.Glob` over the home, and a backslash in the
  pattern is an escape there, so the case fails with `the stub harness found no installed session` for a
  reason that is not this rule); the quoted-temp-root runs are in `docs/evidence/`. The rule is per word,
  and it binds the offline half's *fixture* too: the two
  `ProtectedArtifactsAfterNativeVerification` cases fabricate the document a conversion reports and
  spelled those two command lines by joining their argument words bare — which is the renderer's own
  line only while every word is shell-safe. The stubbed binary is a full path, so on Windows it holds
  backslashes, the harness's own reading of the line ate them, and the check answered FAIL for a line
  the fixture had mis-rendered: `gate (windows-latest)` failed the `selftest` step at head `1a476e0`
  (`FAILED (failures=4)`, its child naming the same two cases the `macos-latest` leg had named at
  `4cd0567` for its own defect, which that leg's repair fixed). The cases now quote every word the way
  `renderResumeCommand`/`shellQuote` does, and the shape is reproducible with the same trick:
  `mkdir -p '/tmp/ccc-win\temp' && TMPDIR='/tmp/ccc-win\temp' python3 -m unittest discover -s
  scripts/tests` (executed; head `1a476e0`'s own file prints `FAILED (failures=4)` there — the two cases
  plus the two self-check cases that read the red child, over the same two child names CI printed — and
  the repaired one `OK`, 153 cases).
- a path is spelled by the platform that reads it: `pathlib` answers `\tmp\x` for `Path('/tmp/x')` on
  Windows where POSIX answers `/tmp/x`, so a rule whose two sides are "the word a shell reads a rendered
  line back as" and "the home *this host* spells" cannot be pinned by a fixture spelled for one of them.
  The native check's offline half (`scripts/tests/test_codex_native_check.py`) therefore spells every
  home so both flavours give it back unchanged — a Windows-shaped path, or a single-segment name — and
  drives both, this host's `Path` and `PureWindowsPath` (what `Path` *is* on Windows), on every host.
  Fixtures spelled `/tmp/…` passed the `ubuntu-latest` and `macos-latest` legs and failed
  `windows-latest` at head `524a20f`: the literal rendering and the comparison disagreed there alone,
  and the three failing subtests reported it as `a correct rendering was rejected`.
  The same boundary applies to a *path the run itself re-spells*: `scripts/codex-native-check.py`'s
  `main` resolves what it is handed (`--repo` always, `--codex-home` when the caller names one) and
  compares the reported home against the resolved one, so a case that drives `main` and spells its own
  throw-away tree through `tempfile.mkdtemp()` disagrees with the run it drives wherever `resolve()`
  re-spells that root — macOS's temp root is `/var/folders/…`, which `resolve()` reads back as
  `/private/var/folders/…`. Unresolved, the two `ProtectedArtifactsAfterNativeVerification` cases failed
  the `macos-latest` leg at head `4cd0567` — `FAILED (failures=4)`, the four being the two cases plus
  the two self-check cases that read the red child — because the stub recognised only the binary path
  and the home it had spelled itself, while the run filed the first stubbed command under the resolved
  spelling: measured on this host below, where both cases fail there, before their own comparisons run.
  (That day's `windows-latest` failure named the same two cases for the other fixture defect, the
  unquoted words above; resolving the root left it red at head `1a476e0` until that one was repaired
  too.) They now resolve the scratch root once, before deriving anything from it, which is this rule
  applied to the run rather than to a fixture — and the shape is reproducible on any host that can make
  a symlink, by pointing `TMPDIR` at one: `mkdir -p /tmp/ccc-aliased/private-var/tmp &&
  ln -s private-var /tmp/ccc-aliased/var && TMPDIR=/tmp/ccc-aliased/var/tmp python3 -m unittest
  discover -s scripts/tests` (executed; the unresolved spelling is `FAILED (failures=4)` there — the two
  cases plus the two self-check cases that read the red child, CI's own number — and the repaired one
  `OK`, 153 cases).
- a path is absolute by the **platform's** own answer, not by its spelling: on Windows `filepath.IsAbs`
  answers false for a POSIX-absolute word like `/opt/tools/claude`, because its volume test wants a drive
  letter, a device prefix or a `\\host\share` UNC form (in the toolchain this tree builds with,
  `src/internal/filepathlite/path_windows.go`'s own `IsAbs`, and the table at
  `src/path/filepath/path_test.go` that turns every POSIX entry false there). A case that hands such a
  word to code which re-spells a word it does not call absolute pins that leg's *working directory*
  instead of the rule: `gate (windows-latest)` failed the `test` step at head `a2670d1` because the case
  the increment's evidence record §6.5 added for the probed executable spelled its probe answer that way,
  while the probe's answer for a bare name is a `PATH` hit, which both platforms spell the way the host
  reads it — `os/exec.LookPath` refuses a hit that is not absolute (`ErrDot`) rather than answering a
  relative one. The case now takes its probe answer from the host's own temp root, with a component holding
  a space so the quoting rule is still exercised everywhere, which is the shape the CLI cases above already
  use; the code under it did not move, because the probe's word is handed over as it stands whenever the
  host calls it absolute. The branch that fires is reproducible on any host by handing the route a probe
  answer the running host does not call absolute (evidence record §6.6).

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
