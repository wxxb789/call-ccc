# Open questions and decisions taken — call/ccc

Append-only. Each entry: the question, the options, the default that was actually taken by the
unattended increment, how to reverse it, and what would settle it. Never edit or delete an entry;
supersede it with a newer one and say which it supersedes.

## Q-0000-01 — Merge authority during the unattended series

- **Status:** resolved (default taken)
- **Question:** may an unattended run squash-merge its own pull requests into `main`?
- **Default taken:** yes, and only through the mechanical merge gate: CI green on the head that was
  verified, a completed clean machine-review evidence comment naming that exact head, zero unresolved
  review threads, and an operator re-run of `scripts/gate.py` with rc 0. Any other condition stops the
  run and reports.
- **Reversal:** pause the run and merge by hand; nothing about the increments depends on who merged.
- **Settled by:** the owner's instruction to drive the series to completion.

## Q-0000-02 — Where binding rules for agents live

- **Status:** resolved (default taken)
- **Question:** `AGENTS.md` at the repository root, or a separate conventions document?
- **Default taken:** `AGENTS.md` is the entry point and is expected to grow with the project;
  `docs/conventions.md` and `docs/harness.md` carry the detail it points to. An increment that changes
  layout, commands or rules updates `AGENTS.md` in the same increment.
- **Reversal:** fold `AGENTS.md` back into `docs/conventions.md` and leave a pointer; agents that read
  `AGENTS.md` by convention would then need the pointer passed explicitly.
- **Settled by:** the owner's instruction that this repository's `AGENTS.md` grows as the project does.

## Q-0000-03 — Module identity and ancestry for the inherited source side

- **Status:** resolved by goal 0001 (see Q-0001-01 for what "ancestry" means once a squash merge is
  the delivery mechanism)
- **Question:** carry AgentsView as the module identity itself (`go.kenn.io/agentsview` at a pinned
  revision, with `cmd/ccc` inside that tree), or import it as a dependency and keep a separate module?
- **Default taken:** the core design's stated default — preserve the upstream module identity and
  ancestry in this repository so upstream-owned packages stay importable without renames.
- **Reversal:** a separate module importing a pinned upstream version; the cost is the Go `internal/`
  import restriction, which would then be solved with thin adapter packages rather than by copying
  parser code.
- **Settled by:** goal 0001's parity evidence — whichever route lets a source-side diff against the
  pinned revision stay empty.

## Q-0001-01 — What "Git ancestry" means for a squash-merged carry

- **Status:** resolved (default taken)
- **Question:** the design asks the carried source side to preserve its upstream Git ancestry; this
  repository delivers every increment as a squash merge onto a linear `main`, which cannot preserve a
  foreign commit lineage (and importing 5 580 files' history would put upstream's whole log inside
  call/ccc).
- **Default taken:** ancestry is preserved as **identity, not lineage**: the carried tree is recorded
  by revision, remote, tag distance and a per-path blob manifest
  (`scripts/upstream-manifest.txt`), and `scripts/upstream-parity.py` proves on every gate run that
  the tree still is that revision. `scripts/upstream-import.py` re-derives the tree and the manifest
  from any checkout of a revision, so the pin is reproducible without local history.
- **Reversal:** fetch upstream into this repository and keep it as a second root (real lineage, at the
  cost of a repository whose history is mostly another project's), or go back to a separate module
  that imports the pin as a dependency.
- **Settled by:** goal 0001's parity evidence; the operator can ask for real lineage instead.

## Q-0001-02 — Which AgentsView revision to pin first

- **Status:** resolved (default taken)
- **Question:** pin the newest release tag (`v0.47.0`) or the tip of upstream `main`
  (`83195c8a`, `v0.47.0-181-g83195c8a`) at import time?
- **Default taken:** the `main` tip, because 181 commits and 605 files of parser/sync/DB change sit
  between the tag and it, and a user's live sessions come from the current format versions. The exact
  revision is recorded in the manifest, in `docs/upstream.md` and in `cmd/ccc/pin.go` (which the
  parity step keeps in agreement).
- **Reversal:** `python3 scripts/upstream-import.py --upstream <checkout> --restore-snapshot --prune`
  at `v0.47.0`, then `python3 scripts/upstream-parity.py`; one commit, no code changes.
- **Settled by:** the owner's preference between "newest formats" and "newest release".

## Q-0001-03 — The leak scan does not read the carried tree

- **Status:** resolved (default taken)
- **Question:** the gate's `secrets` step used to scan the whole working tree. Carrying AgentsView
  brings upstream's own credential-shape corpora (`internal/secrets/rules_test.go`,
  `internal/db/secret_findings_test.go`, …) into the tree: 175 findings, all upstream test data.
  Allowlist them, keep failing, or scan a scope?
- **Default taken:** the step stages and scans the paths the carry manifest calls project-owned and
  says so in its PASS line; the carried tree is covered by the byte-level `parity` step instead. A
  tree with no manifest is scanned whole and discloses that.
- **Reversal:** restore the whole-tree scan and add a `.gitleaks.toml` whose allowlist names the
  upstream corpora by path (maintenance: refresh it on every pin bump).
- **Settled by:** the owner's appetite for a maintained allowlist versus a scoped scan.

## Q-0001-04 — One carried file differs by a line ending

- **Status:** resolved (default taken)
- **Question:** `frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json` holds a single
  CRLF pair in 1.4 MB; this repository's carried `.gitattributes` (`* text=auto eol=lf`, upstream's
  own file) makes `git add` store it one byte shorter, so it cannot be both carried verbatim and
  stored unchanged.
- **Default taken:** keep the file with the byte git stores and declare the difference in the
  manifest and [`docs/upstream.md`](../upstream.md); the parity step verifies the *stored* bytes.
- **Reversal:** not carry the file at all (a gap in upstream's frontend fixtures), or add a
  `.gitattributes` inside the carried `frontend/` directory (project content in upstream-owned space).
- **Settled by:** the owner's preference if upstream's frontend fixtures ever need to run here.

## Q-0001-05 — CLI parsing without a framework

- **Status:** resolved (default taken)
- **Question:** build `cmd/ccc` on Cobra (already an inherited dependency) or on the standard library
  with a small option/operand split?
- **Default taken:** the standard library, for one command with two flags; the split exists because
  Go's `flag` stops at the first operand and `ccc inspect <ref> --json` is the natural order for a
  reader. No new module dependency is introduced, so `go.mod`/`go.sum` stay the pin's.
- **Reversal:** adopt Cobra when the command surface grows (targets, `formats`, `continue`), which is
  cheap because the CLI holds no logic the core depends on.
- **Settled by:** goal 0002's command surface.

## Q-0001-06 — The two choices the accepted simplify findings left open

- **Status:** resolved (default taken)
- **Question:** two accepted simplify findings each named two ways out, and the increment had to pick
  one. `Message.StopReason`, `Message.HasContextTokens`, `Message.ContextTokens` and
  `Message.OutputTokens` are write-only — nothing in `cmd/ccc` or `internal/ccc` reads them — so they
  can be deleted until goal 0002's writer needs them, or carried in the parity dump so a parser
  regression that stops filling them is visible. And `Fidelity.Projection` was a constant
  (`db.ArchiveContent()` on a handle `OpenReadOnly` never tightens), so the projection can be derived
  from the rows an archive read returns, or the field and its warnings can be deleted.
- **Default taken:** delete the four write-only fields (re-adding them is a small change against the
  inherited parser's own fields, which the deleted assignments named), and derive the projection from
  the read's rows — that is what makes the narrowing warning the plan promises fire, and it leaves
  the committed oracle records unchanged. `ArchiveStatus.Projection` moved onto `ArchiveReport` with
  it, because a session-row status cannot see how narrow the archive is.
- **Reversal:** re-add the four fields and dump them in `internal/ccc/parity`; or delete `Projection`
  and `projectionWarnings` and drop the label from the CLI — both then need
  `python3 scripts/upstream-compare.py --record` (network) to re-record the oracle.
- **Settled by:** goal 0002's writer needing the fields, or the operator's preference between a
  richer dump and a smaller document.

## Q-0001-07 — What an archive read may say about the tool payloads it does not show

- **Status:** resolved (default taken). Supersedes the `Fidelity.Projection` half of Q-0001-06; that
  entry's other half (deleting the four write-only `Message` fields) stands.
- **Question:** the review of increment 0001 (finding A02) measured a session with two text messages
  and no tool call, ingested under the **default** policy, reading back as projection `"transcripts"`
  with the warning `archive stores the transcripts projection: tool inputs and results are not in it`.
  The writer's `archive_content` policy lives on the writer's own handle
  (`internal/db.SetArchiveContent`; `OpenReadOnly` never tightens one), so naming a policy is an
  inference from row shape — and in that measurement it reported an omission that never happened.
  Keep the policy vocabulary with a caveat, or report only what the rows show?
- **Default taken:** report only what the rows show. `Fidelity.Projection` is
  `Fidelity.ContentShown` with the values `tool-payloads`, `message-text` and `none`; the CLI prints
  that, and the warnings are worded as what the read could not show (`the archive read shows no tool
  inputs or results: whether the archive dropped them or the session recorded none is not visible to a
  read`), naming the message counts when the session row records more messages than the read returned.
  The oracle dumps were re-recorded (`python3 scripts/upstream-compare.py --record`, network), which
  moved the dump tool id to `call-ccc-parity-oracle/2`; `docs/evidence/0001-…md` carries the new
  hashes.
- **Reversal:** put the policy words back — one `switch` in `internal/ccc/archive.go` — and re-record
  the oracle again, since the dumped value changes with them.
- **Settled by:** the owner's preference between a label that names the writer's policy as an
  inference and one that states what the read shows; goal 0002's writer needing a policy name it
  cannot have.

## Q-0001-08 — How a tool call's own result events reach the conversation

- **Status:** resolved (default taken)
- **Question:** the round-2 review (finding 1) measured that a call's own result events never reached a
  source read: the inherited parser attaches a Codex `function_call_output` to the call
  (`ParsedToolCall.ResultEvents`), `messageFromParsed` carried the calls but not their events, and so an
  inspection reported 0 tool results for a session whose archive read reported 1 with the shell output
  in it. Carrying them has two shapes: map each event onto the message's existing `ToolResult` rows —
  which is exactly what `messageFromArchive` does for the stored events, so both reads describe the same
  session the same way — or add an event-shaped `ToolCall.ResultEvents` field that also keeps the
  event's `AgentID`, `SubagentSessionID` and timestamp.
- **Default taken:** the first. `ToolResult` already holds the id, source, status, content and size, and
  `Source` says which kind of row it is (`function_call_output`, a Claude tool-result block); the
  re-recorded Codex dump shows both reads reporting the same 51-byte shell output under the same digest,
  which is the agreement the round-trip evidence exists to show. The event's subagent attribution stays
  on the call (`ToolCall.SubagentSessionID`) where the inherited parser already puts a spawned call's
  session id; `docs/evidence/0001-…md` §8 names that as a stated limit rather than a silent loss.
- **Reversal:** add the event shape as a fourth field and dump it (`ToolResultRow` in
  `internal/ccc/parity/dump.go`), which needs `python3 scripts/upstream-compare.py --record` (network)
  again because the dump tool id moves with the shape; the CLI needs no change either way.
- **Settled by:** goal 0002's writer needing per-event attribution (a subagent timeline in the target
  format) instead of the single result row it gets today.

## Q-0001-09 — What a parity run that cannot enumerate the tree reports

- **Status:** resolved (default taken)
- **Question:** the round-2 review (finding 5) measured that without git the file-list and ignore rules
  were skipped while the script still exited 0, so the required gate step printed an unqualified
  `GATE: PASS` for a tree whose carried files were never enumerated. The verdict could be folded into
  the existing `FAIL` (exit 1, "the tree violates parity"), or reported as its own status the gate
  rejects.
- **Default taken:** its own status — `parity: INCOMPLETE`, exit `2`, naming the two rules that did not
  run; 1 keeps its meaning ("a rule is violated"), 0 keeps "verified", and the gate's required-step
  machinery already fails any non-zero outcome, so an unverified tree cannot be quoted as agreement.
  Everything answerable without a file list still runs first, so real drift in such a tree is still
  reported as `FAIL` with its offenders rather than hidden behind the incompleteness. `docs/harness.md`,
  `docs/upstream.md` and the script's own docstring state the three exit codes, and
  `scripts/tests/test_upstream_parity.py` pins the verdict.
- **Reversal:** return 1 from the same branch and drop the status — one `except` clause in
  `scripts/upstream-parity.py`; the gate's verdict is identical either way, only a reader's telling of
  "unverifiable" from "violated" changes.
- **Settled by:** the owner's preference for a distinct incomplete verdict over a failure that also
  covers trees nobody inspected closely.

## Q-0001-10 — What a relative filesystem reference means

- **Status:** resolved (default taken)
- **Question:** the round-3 review (finding 4) measured that `claude:./project/session.jsonl` reached
  both lookups with only `filepath.Clean` applied: the inherited providers compare a candidate with
  their own absolute roots (`claudeProjectHintFromPath` resolves the pair with `filepath.Rel`) and
  reject the mixed form, and the archive stores the absolute path `sessions.file_path` was written
  with, so an existing session was reported as missing. A relative path can be read as relative to the
  provider's configured roots or as relative to the process's working directory — the shell's own
  meaning — and the two differ whenever the command is not run from a root.
- **Default taken:** the configured roots. A reference is offered to the provider and to the archive
  under `filepath.Join(root, reference)` for each configured root (deduplicated), the source reader
  reports two roots that both hold it as candidates rather than picking one, and an archive opened
  without a configuration resolves only absolute references — because the roots are a fact of the
  configuration, and a lookup whose answer depends on where the process happens to stand is a different
  product than an inspection of "this provider's sessions". `reader.go`'s `referencePaths` is the one
  place that spells it; `docs/evidence/…-parity-and-inspection.md` §1 and §8 state it as proven and as
  a limit.
- **Reversal:** resolve the relative reference against the process working directory instead (or in
  addition, with the ambiguity path already there to report a reference both readings find), one
  function in `internal/ccc/reader.go`; nothing persists a resolved path, so no data outlives the
  choice.
- **Settled by:** the operator's reading of `ccc inspect <provider>:<relative path>` — the roots it
  lives under, or the directory it was typed in.

## Q-0001-11 — How a host-qualified reference is resolved

- **Status:** resolved (default taken)
- **Question:** the round-3 review (finding 2) measured that `codex:host-a~codex:<native id>` had the
  host stripped before the lookup, so it resolved `codex:<native id>` — this machine's session with the
  same native id — and an inspection reported another conversation. The archive's id for a session
  synced from another machine is `<host>~<the id it had there>` (`internal/remotesync` rebuilds ids that
  way), and a caller may write the host qualifier around the stored id or around the bare native id.
  The lookup could be exact-only, or exact with a narrowed fallback.
- **Default taken:** exact first (`GetSession` on the spelling as given, i.e. the archive's own id),
  then the inherited raw-suffix lookup filtered to rows whose id carries the requested host *and* the
  requested agent — never the local row and never another host's, and two rows of the requested host are
  reported as candidates rather than picked. The source side refuses the reference entirely with
  `ErrNoLocalSource`: a remote session has no local file (upstream's own `FindSourceFile` answers `""`
  for one), and answering from a local session that shares the native id is the defect the finding
  named. An inspection therefore reports the archive's host-qualified row and says why the source is
  unreadable.
- **Reversal:** exact-only (drop the fallback, one helper in `internal/ccc/archive.go`) or let the
  source side fall back to the local session again (delete the guard in `reader.go`) — the second is the
  behaviour the finding rejected, so it would need the reviewer's agreement, not just a preference.
- **Settled by:** whether a caller's `host~<native id>` spelling (no agent prefix, which is how upstream
  documents remote ids) must resolve without the caller knowing the stored prefix; today it does.

## Q-0001-12 — How a carried file's working tree is verified

- **Status:** resolved (default taken)
- **Question:** the round-3 review (finding 1) measured that the git-backed branch trusted `git status`
  for the working tree, so a carried file marked `assume-unchanged` (or `skip-worktree`) could be edited
  while the pinned blob stayed in the index and every predicate still agreed. The hole can be closed by
  hashing the working tree independently, by rejecting the index flags that suppress git's answer, or by
  both.
- **Default taken:** hash it — `worktree_state` compares shape, bytes and (where the platform has POSIX
  mode bits) the executable bit of the file this repository ships against the manifest record, in both
  branches, with git and without it; git's index still answers what a stat cannot (what a fresh clone
  receives) and a `git status` that reports the working tree as modified is still a failure. No index
  flag inspection was added: the flags only silence `git status`, whose answer is no longer what the
  bytes are checked through, so rejecting them would add a failure mode without adding verification.
  The cost is one hash pass over the carried tree — 5 553 files, 63 MB, ~0.8 s on the import host,
  measured — and `docs/harness.md`/`docs/upstream.md` state the rule.
- **Reversal:** drop the `worktree_state` call in `check_files` (the git branch returns to git's own
  answers, the bytes-only branch keeps them), or add the flag rejection beside it (`git ls-files -v`
  output parsed for lowercase flags and `S`).
- **Settled by:** whether a gate run's budget can afford the hash pass on the largest tree the project
  ever carries; today it is under a second against a gate that takes ~23 s.

## Q-0001-13 — What the archive answers when a native id has more candidate rows than one lookup reads

- **Status:** resolved (default taken)
- **Question:** the round-4 review (finding 4) measured that the inherited raw-suffix lookup is asked
  with `LIMIT 5` and *then* filtered by provider and host, so `codex:host-a~<native id>` — a reference
  the archive can answer — was reported as absent whenever the requested row was older than five other
  hosts' copies of that native id. The answer can be established by reading more of the query's answer,
  by reporting the truncated list as candidates, or by refusing to answer.
- **Default taken:** read more, then refuse if that is not enough. The lookup starts at the previous
  page size and re-asks four times wider until the inherited query returns fewer rows than it was
  allowed — the query's own way of saying it had nothing more — or reaches a cap of 500 rows, where the
  reference is answered with `ErrTruncatedCandidates` instead of "not found" or a single row. Any
  filtered answer is only an answer when the unfiltered one is complete: "not found" asserts absence and
  a single candidate asserts uniqueness, and a truncated list supports neither. Both call sites (the
  local suffix fallback and the host-qualified one) share the helper, and 500 duplicates of one native
  id in one archive is not a shape a reference is resolved against by reading a subset of them.
- **Reversal:** return the truncated rows as an `AmbiguousError`'s candidates with a note that the list
  is not complete (one `switch` at the two call sites), or drop the cap and read until the query is
  exhausted (one constant), which trades a bounded read for an answer on absurd archives.
- **Settled by:** whether the operator prefers a refusal on a >500-duplicate archive over a bounded read
  of it; the reference form the finding names (`codex:host-a~<native id>`) works either way for the
  realistic case of a handful of hosts.

## Q-0001-14 — Which provider answers count as a local source a reference can read

- **Status:** resolved (default taken)
- **Question:** the round-4 review (finding 2) measured that `SourcesForChangedPath` deliberately
  classifies a path that is *absent* (so the watcher does not drop a deletion), and that this reader
  took that classification for a source: a relative reference became ambiguous between the root that
  holds the session and one that does not, and a source deleted after its archive was written failed
  while fingerprinting instead of reporting the archive. The alternatives are to accept a
  classification as a source (today), to require the path the reference named to exist, or to require a
  file to stand behind the source the provider names.
- **Default taken:** require a file, either way round — a candidate is kept when the spelling the
  reference named is a regular file (`parser.IsRegularFile`, the same predicate the providers' own
  stored-path lookup applies) or when the candidate's own identity is one. The second route is what
  keeps a provider's mapping from a sidecar or container path to an existing transcript, and a
  provider's store member whose identity is virtual (`<container>#<session>`) is kept through the first,
  because the container file the reference names is there. An absent path therefore resolves to no local
  source, which is what lets an inspection warn and report the archive instead of losing it to a
  fingerprint error. `docs/evidence/0001-…md` §8 states the limit this leaves for a source that exists
  but cannot be stat'ed at all.
- **Reversal:** delete the `readableSources` call in `resolvePathReference` (one line: the provider's
  classification is taken as the source again, with the two measured consequences above), or tighten it
  to the candidate's identity alone (`spellingPresent ||` removed), which would drop a store member
  reference to a container file.
- **Settled by:** whether the operator wants `ccc inspect <provider>:<path>` to answer "not found" (and
  consult the archive) for a path that is not on this machine, or to keep reporting the provider's
  claim that such a path names a session.

## Q-0001-15 — How a caller reads one branch of a forked transcript

- **Status:** resolved (default taken)
- **Question:** the round-6 review (finding 3) measured that a Claude Code transcript holding a DAG fork
  parses to several sessions while the reader answered a reference to that path as *absent*. Reporting
  the candidates is what the design asks for; the open choice is what a caller then does with one. The
  parse's identity for a fork is `<stem>-<entry-uuid>`, which is the identity an archive stores, but the
  inherited source lookup resolves a session id to `<id>.jsonl` under a configured root, so no file
  exists for that identity: reading it by id reports "not found" at the source and reaches an archive if
  one holds the session. The alternatives are to teach the source lookup to scan transcripts for the
  fork identity, to report the path's main branch as *the* session (the reading this increment
  rejected), or to leave the candidates as the answer and let the archive carry the fork.
- **Default taken:** report the candidates and leave the source lookup alone. A path reference to a
  forked transcript returns the parsed identities (`AmbiguousError`); an id-only reference to a fork
  reaches the archive or reports "no readable source session", never the main branch. Resolving a fork
  id to a file is a change to inherited parser code (a DAG-metadata scan), which this increment may not
  make locally — it is an upstream contribution, not a call/ccc patch.
- **Reversal:** one function in the inherited provider (resolve a raw id the file-name lookup misses by
  scanning sibling transcripts' DAG metadata for it), which changes carried code and needs an upstream
  route; or map the fork identity onto the file in the reader, which would have to hand the provider a
  path and then still choose among the parse's sessions.
- **Settled by:** whether the operator wants `ccc inspect` to read a forked branch from the source tree —
  the archive route already reads it, so the cost of waiting for an upstream scan is a source read that
  answers only through the archive.

## Q-0001-16 — What a provider operation failure does to an inspection's exit status

- **Status:** resolved (default taken)
- **Question:** the round-6 review (finding 2) measured that an outcome carrying operational
  `SourceErrors` and no result was reported as absence, with the provider's diagnostics discarded. The
  classification is now its own answer (`ErrProviderFailed`); the open choice is whether an inspection
  whose source read failed that way is a success when an archive holds the session, or a failure the
  operator sees on the exit status.
- **Default taken:** the inspection reports the provider's diagnostics as a warning and, when an archive
  holds the session, returns it with exit status 0 — the shape `ErrNotFound` and `ErrNoLocalSource`
  already have, and what finding 2 asked for ("expose their diagnostics even if archive fallback remains
  available"). `--json` carries the warning in `warnings`, so a script can see it without reading the
  exit status; a hard error is reserved for outcomes whose read could not be attempted at all (a parse
  returning a Go error, an unreadable archive).
- **Reversal:** treat `ErrProviderFailed` as fatal in `Inspect` (one `case` in its switch: return the
  error instead of warning), which would make a retryable failure override an archive read that
  succeeded.
- **Settled by:** whether the operator wants an unattended pipeline to fail when the source side fails
  while an archive answers, or to succeed with the failure named in the report.

## Q-0001-17 — Whether a filesystem reference also reports a conversation whose source is gone

- **Status:** resolved (default taken)
- **Question:** the round-7 review (finding 1) measured that a path reference reported nothing for a
  session AgentsView had reconciled away (its row keeps `source_missing_at`, its conversation stays
  readable by id, and the path-keyed lookup it used excludes exactly those rows). Resolving the
  retained row is the fix; the open choice is what a path answers when it has *both* current members
  and retained rows — e.g. a source deleted and re-created at the same path, or a multi-session file
  whose superseded member was marked missing. The options: answer the current members only, answer the
  union of both as candidates, or keep the retired lookup's answer (current members) and lose the
  retained rows again.
- **Default taken:** the current members answer while the archive records any for the path — the rows a
  parse of that file emits, which is what the *source* side reports for the same reference, so the two
  sides cannot disagree about a file that is there — and the rows the archive retained under the path
  answer only when no spelling has a current member. The retained conversation is never lost: it stays
  the archive's answer to its own id, and the path answers it once the file it came from is gone. The
  union was rejected because it reports conversations the source side does not for a file that is
  present (and because the same conversation can arrive through both tiers, which the close-out's union
  mutation observes as a duplicate candidate).
- **Reversal:** one function (`internal/ccc/archive.go`'s `sessionIDsForPaths`): drop the
  `len(current) > 0` shortcut and return the union of both tiers, deduplicated, which makes a path with
  a live row and a retained row an ambiguous reference.
- **Settled by:** whether the operator wants `ccc inspect <provider>:<path>` to answer with every
  conversation that path ever produced, or with the members the file holds now — the archive route by
  id already reads the retained one either way.

## Q-0001-18 — Which checkouts may hold a carried symlink as a file

- **Status:** resolved (default taken)
- **Question:** the round-7 review (finding 3) measured that the parity check accepted a regular file
  holding the link's target text at *every* `120000` record, because a checkout that cannot create
  symlinks has exactly that file. On a symlink-enabled checkout that acceptance certifies a carried
  path whose working-tree type — and behaviour — has changed (the bytes are the pinned blob, git's
  index keeps the pinned `120000` entry, and `skip-worktree` keeps `git status` silent), which is what
  the step exists to establish. The open choice is which checkouts may hold the file form.
- **Default taken:** the file form is accepted where the checkout *states* the representation it uses —
  git's `core.symlinks=false` for this tree, and, where git cannot be asked at all (a tree without git,
  a copy), a platform whose checkouts cannot create symlinks. An unset `core.symlinks` on a POSIX host
  means the link is a link, and a `core.symlinks` query that cannot answer on a tree git *is* answering
  for fails the step naming the probe. A run that accepted the file form reports how many it accepted.
- **Reversal:** one function (`link_paths_may_be_files` in `scripts/upstream-parity.py`): return
  `False` unconditionally (a checkout that stores links as files then fails the step), or `True`
  unconditionally (back to the state the finding measured). The Windows case is why the platform
  fallback exists: Git for Windows records `core.symlinks=false` in its system configuration and
  `actions/checkout` sets it on that runner, so a Windows CI job answers through git rather than through
  the platform probe.
- **Settled by:** whether the operator wants a carried link's shape to be required to be a link
  everywhere (and Windows checkouts to be told to configure `core.symlinks=false`), or the current
  reading where the checkout's own answer decides.

## Q-0001-19 — How strictly the comparison compares two reads' recorded tool-result outcomes

- **Status:** resolved (default taken)
- **Question:** the round-8 review (finding 2) measured that `same_messages_as_source` ignored
  `ToolResult.Status`, so an archived Claude result and a source result with the same content but
  different `is_error` values were reported as agreeing. The requested change — "compare statuses
  wherever both reads retain them" — leaves the strictness open: compare only where both reads record a
  non-empty status (the reading where "retain" means "recorded a value"), or compare the outcomes the
  two reads record as a whole, including a read that records none.
- **Default taken:** the whole recorded outcome, as a set per result key, in both directions, wherever
  the archive shows a status-bearing row for that key. An empty status is a value, not an absence: it is
  what a read records for a result its provider did not mark as an error, so an archive saying
  `errored` beside a source saying nothing is a difference, and so is the reverse. A call row's own
  result content is not status-bearing (the archive stores no status on the call), so a key the archive
  holds only there is not compared — the same allowance the body comparison makes, and what keeps a
  provider whose results the archive kept on the call from being reported as differing everywhere.
  Statuses are retained metadata under every archive content policy (`internal/db/archive_content.go`),
  never a payload a policy narrows, which is why their absence is comparable at all.
- **Reversal:** one function (`differingResultOutcomeNames` in `internal/ccc/inspect.go`): return
  `nil` (back to the state the finding measured), require only that the source's own status appear among
  the archive's (drops the archive-records-an-extra-outcome case), or skip comparison whenever either
  side records none (the narrower reading of "wherever both reads retain them").
- **Settled by:** whether the operator wants an inspection to say "every archived message matches the
  source read" when the archive and the transcript disagree about whether a tool call failed; today it
  does not.

## Q-0001-20 — Which archive-side result evidence the comparison holds the source read to

- **Status:** resolved (default taken)
- **Question:** the round-9 review (finding 2) measured that `same_messages_as_source` never asked the
  reverse direction: every result check was driven by the source's own rows, so an archive holding a
  call's outcome the transcript read no longer recorded was reported as *every archived message matching
  the source read*. The requested change — "check archive-only results too, accounting for folded source
  rows" — leaves the archive-side scope open: every way an archive read can hold a result (a result row of
  its own, an event folded onto the call, or the call row's own `result_content`), or only the rows that
  record an individual result.
- **Default taken:** the archive message's **own result rows** — which, for an archive read, is where the
  inherited loader materializes a call's `ResultEvents` (`internal/db/messages.go`, `attachToolCalls`).
  A call row's own `result_content` stays out, exactly as it does for the body comparison: the inherited
  write path derives it from the call's events and may merge several agents' outputs into one summary
  (`internal/ingest/tool_results.go`), so its key is not evidence that the source recorded a result — and
  holding the source to it would report a difference the archive's own write path created. The source side
  of the check is the results at that ordinal plus those of the rows the archive keeps no message for (the
  folded tool-result-only rows), because folding is exactly where the two reads stop sharing an ordinal.
- **Reversal:** one call site (`compareMessages` in `internal/ccc/inspect.go`): pass `nil` (back to the
  state the finding measured), pass every key `archivedResultKeys` collects for the message (also holds
  the source to a call's derived summary), or drop `foldedSourceResults` from the source side (reports
  every real Claude archive as holding an extra result).
- **Settled by:** whether the operator wants an archive-only result to be reportable when the only trace
  the archive kept of it is the derived summary on the call; today "every archived message matches the
  source read" is not claimed for that shape, but neither is a difference reported for it.

## Q-0001-21 — What the comparison compares between two reads' tool calls

- **Status:** resolved (default taken)
- **Question:** the round-9 review (finding 3) measured that `same_messages_as_source` compared messages
  by role and rendered text and never by the calls' retained `input_json`: a Claude `Edit` renders as
  `[Edit: <file_path>]`, so two edits of one file with different arguments read as identical text while
  both adapters retain different inputs. The requested change — "compare call identity/name and inputs
  wherever both reads retain them" — leaves both the pairing and the directions open.
- **Default taken:** calls are paired by the provider's own tool-use id where a read recorded one and by
  position among a message's id-less calls otherwise; a call only the **archive** holds, a call whose name
  both reads spell differently, and a call whose input both retain differently are differences. The input
  comparison is skipped unless both reads retain a non-empty one (the storage projection keeps call names
  and blanks inputs, `internal/db/archive_content.go`), and it compares the inherited writer's own
  spelling (`db.SanitizeUTF8`, which is how `internal/sync`'s parsediff decides whether a stored call
  still matches its parse). A call only the **source** holds is deliberately not compared: a narrowed
  archive content policy may keep a row and drop its calls, and that narrowing is already reported as what
  the read shows (`content_shown`, `ArchiveContentUsage` drops payloads and whole rows alike).
- **Reversal:** one function (`differingCallProblems` in `internal/ccc/inspect.go`): return `nil` (back to
  the state the finding measured), compare inputs without the both-retain guard (reads a blanked input as
  a changed one), or pair calls by name and index instead of tool-use id.
- **Settled by:** whether the operator wants a source-only call reported too — it would make every
  usage-projection archive differ from its source read — and whether the name comparison should be as
  strict as the input one; today both name and input serve the same question ("is this the same call?").

## Q-0001-22 — What an incomplete parse of a path does to an inspection

- **Status:** resolved (default taken)
- **Question:** the round-11 review (finding 2) measured that a path reference accepted the sole
  successful parse result without looking at `ParseOutcome.ResultSetComplete` or the outcome's source
  errors: a multi-session source that returns one readable member beside members it could not read
  (internal/parser's `TestOpenClawSQLiteStoreMalformedMemberIsolation`) resolved that path to the
  readable member, so whether a malformed sibling parses decided which conversation a path named. The
  refusal is what the finding asks for; open is whether an inspection treats it as a failure or as one
  more source-side state whose archive is still consulted.
- **Default taken:** the same shape `ErrProviderFailed` has (Q-0001-16): the reader classifies it as
  its own answer (`ErrPartialSource`) carrying the provider's diagnostics and the identities the parse
  *did* read, and an inspection reports it as a warning, consults the archive, and exits 0 when the
  archive holds a session. A hard failure would take away the archived session a caller can still read,
  and the warning names the identity to pass back (`ccc inspect <provider>:<that id>`), which is what
  makes the refusal actionable; `--json` carries both in `warnings`.
- **Reversal:** one `case` in `Inspect`'s switch (return the error instead of warning), which makes a
  partial source override an archive read that succeeded.
- **Settled by:** whether the operator wants an unattended pipeline to fail when a source is only
  partly readable while an archive answers, or to succeed with the partial read named in the report.

## Q-0001-23 — Whether a stored agent label is a provider name a reference may use

- **Status:** resolved (default taken)
- **Question:** the round-11 review (finding 1) measured that a Freebuff session's archive row carries
  `agent = freebuff` and the id `freebuff:<project>:<timestamp>`, while the CLI's reference grammar
  accepted only registry providers: `ccc inspect freebuff:…` exited 2 as an "unknown provider" before
  the core looked anything up. Freebuff is deliberately not a provider in AgentsView (no registry
  entry, no config key, no factory -- `parser.AgentByPrefix` maps its ids onto the Codebuff def and
  `parser.ProviderStoredAgents` records that the Codebuff provider writes both labels). The alternatives
  are to accept the label as a name for the provider that writes it, or to require callers to respell it
  as `codebuff:<native id>`.
- **Default taken:** accept it, from the inherited mapping only. `internal/ccc` resolves a name that is
  not a registry entry through the provider whose `parser.ProviderStoredAgents` includes it
  (`storedAgentOwner`), the CLI asks the core (`ccc.KnownProvider`) instead of doing its own registry
  lookup, and the archive answers the `freebuff:` spelling of every lookup shape. Nothing is registered
  locally: the label list a caller sees is still upstream's, and a session's own recorded label is what
  the read reports for it.
- **Reversal:** drop the `storedAgentOwner` branch in `resolveFactory` and the `ccc.KnownProvider` call
  in `cmd/ccc`'s `parseRef` (two small edits): `freebuff:…` becomes an unknown provider again, and a
  caller has to respell the identity the archive reports before it resolves.
- **Settled by:** whether the operator wants the exact identifier an inspection prints (`archived id:
  freebuff:…`) to be usable as the next reference, or a grammar in which only registry provider names
  appear.

## Q-0002-01 — What a converted Codex session records as its model provider

- **Status:** resolved (default taken)
- **Question:** a converted session must say *something* about its provider, and every choice binds it.
  Measured on Codex 0.169.0: a rollout that records no provider loads under `thread/resume` in a home
  that has never migrated, and is refused (`This chat uses model provider "", which is no longer
  configured`) in a home whose state already holds migrated threads — which is what a real user's home
  looks like. Recording the *source's* provider would be false (Codex has no such provider
  configured). Recording the configuration's explicit `model_provider` alone is null on a host that
  relies on Codex's default. Recording the effective provider binds the session to it, so a later
  change of that configuration makes the harness refuse to resume with `Cannot change this thread's
  model provider from X to Y on resume`.
- **Default taken:** record the provider the harness itself says a **new** session of it would use,
  asked over its own protocol (`thread/start` on an *ephemeral* thread, which reports the effective
  provider including the harness's own default; `config/read` as the fallback when that leaves no
  answer), as seen from the source session's working directory. No model is recorded, so the target
  keeps choosing that. The run reports the value, where it came from
  (`model_provider_source`: `target-thread-defaults`/`target-config`/`unrecorded`) and the binding as a
  reported caveat (`provider_binding`).
- **Reversal:** stop passing `Provider` in `CodexTargetOptions` (one field, empty means "record none"),
  which restores the provider-less file and the refusal in a migrated home.
- **Settled by:** whether the operator prefers a session that resumes under the harness's defaults but
  is refused in a migrated home, or one that resumes now and is pinned to the provider that was
  configured when it was converted.

## Q-0002-02 — The provenance a converted rollout records for itself

- **Status:** resolved (default taken)
- **Question:** the rollout's `session_meta` carries `originator`, `source` and `thread_source`, and
  the target's UI and pickers read them. Writing `codex_cli_rs`/`exec` would be false provenance and
  would hide the session behind the picker's "non-interactive" filter; writing an originator the target
  has never seen risks a version that validates it.
- **Default taken:** `originator: "ccc"` (the program that wrote the file), `source: "cli"` (the session
  is meant to be resumed interactively), `thread_source: "user"`; measured to be accepted and reported
  as such by `thread/read`/`thread/resume`. The user-facing identity of the conversion lives in the
  CLI's output, not in a fabricated field.
- **Reversal:** one constant in `internal/ccc/codex_target.go`; nothing else reads the value.
- **Settled by:** whether the operator wants the target's own names there or call/ccc's, and whether a
  future Codex version rejects an unknown originator (the native check would fail loudly).

## Q-0002-03 — Which `cli_version` a converted rollout records

- **Status:** resolved (default taken)
- **Question:** the target's reader requires `session_meta.cli_version` (measured: without it,
  `thread/resume` says `rollout … does not start with session metadata`), but call/ccc did not write
  the conversation, so the value is a claim about the file's format rather than about its author.
- **Default taken:** the version the installed `codex --version` reports when it can be probed
  (`cli_version_source: "probe"`), otherwise the pinned constant `CodexFormatVersion = "0.169.0"` — the
  version whose reader this writer's output was verified against (`"writer-pin"`). Both are reported.
- **Reversal:** always write the constant, or refuse to convert when the target binary is absent.
- **Settled by:** whether an older Codex refuses a rollout whose `cli_version` is newer than itself; if
  it does, the probe should be replaced by the *oldest* verified version.

## Q-0002-04 — Pre-compaction history, and what the boundary means in the target

- **Status:** resolved (default taken)
- **Question:** the goal says both "preserve available history by default" and "do not revive
  deliberately inactive history as though it were active". A transcript that compacted holds both the
  summary and the turns it replaced.
- **Default taken:** carry the turns **and** the boundary, as the target's own `compacted` record.
  Measured: Codex applies that record to its model-visible history (a resumed turn answers from the
  summary and the post-boundary turns only), while the file keeps the recorded turns for readers, so
  the target continues from the source's *active* context without call/ccc having made a selection the
  goal does not authorize. The report counts the pre-boundary messages and states what the boundary
  does (`pre_compaction_history`, carried-with-caveat).
- **Reversal:** one branch in the writer that stops at the first boundary (then the pre-boundary turns
  are a reported omission instead of a reported caveat).
- **Settled by:** goal 0009 (selective context), which is where selection belongs if the operator wants
  it.

## Q-0002-05 — Omitting reasoning traces, sidechain turns and system records

- **Status:** resolved (default taken)
- **Question:** a target writer can carry a persisted reasoning trace as plaintext, a subagent's turn as
  an ordinary message, and a provider's system record as a `developer` message — or omit each and say
  so.
- **Default taken:** omit all three by default and report each with its count and reason. Codex records
  reasoning as its provider's own encrypted artifact (writing plaintext in its place would present
  source-private reasoning as a target-native trace); a sidechain turn has no subagent attribution in a
  flat message (so carrying it as an ordinary turn would state that the conversation's own agent said
  it); the target's `developer` channel carries the target's own instructions (so a provider's
  bookkeeping does not belong there).
- **Reversal:** one branch each in the writer's message loop; each category's count is already in the
  report, so the change is visible rather than silent.
- **Settled by:** goals 0009/0013, which own selection and persisted reasoning traces.

## Q-0002-06 — Verification levels, their default, and the exit codes

- **Status:** resolved (default taken)
- **Question:** `ccc continue` can verify nothing, ask the target's loader, or run a real turn — and a
  failure of each means something different for the caller's pipeline.
- **Default taken:** `--verify load` (the target's own loader over `thread/resume`, offline); `resume`
  on request and only with `--resume-prompt` (the turn becomes part of the user's session, so its text
  is the caller's); `none` writes no claim. A verification that could not run because the target binary
  is absent is reported as not-verified and does **not** fail the run (the conversion and installation
  really happened); a verification that ran and failed exits 1. The three states are separate fields in
  `--json`, and the human output prints one line each.
- **Reversal:** make `load` required (refuse to convert without the target binary), or make an absent
  binary a failure — one branch in `continuationExit` plus one in `Continue`.
- **Settled by:** whether the operator wants an unattended pipeline to fail when the target harness is
  not installed, or to convert, install and say the native half is unverified.

## Q-0002-07 — The parity oracle's cases now name their fixture

- **Status:** resolved (default taken)
- **Question:** the comparison cases (`internal/ccc/parity`, `scripts/upstream-compare.py`) discovered
  "the single fixture below the provider root". This increment adds a second Claude transcript (the
  tool-heavy shape a conversion reports), so that discovery would fail — or, worse, silently read a
  different file if it had been written to pick the first.
- **Default taken:** every case names the session file it reads (`parity.Cases`, mirrored by the
  script's `CASES`), a name that is not there is an error, and the previous "exactly one" rule remains
  only as a convenience for a caller with a tree of its own. The recorded dumps were **not**
  re-recorded: the cases still read the same bytes, and the offline test compares them byte for byte
  (it passes unchanged). The new fixture is a new file, not a replacement.
- **Reversal:** delete the second fixture and the fixture field, restoring discovery by convention.
- **Settled by:** whether the operator wants the oracle to keep discovering fixtures or to name them;
  naming is what makes an added shape unable to change an existing record.

## Q-0002-08 — Archiving goal 0001 in this increment

- **Status:** resolved (default taken)
- **Question:** the index's rule says a goal moves to `docs/goals/archive/` once it is merged, while its
  row still read "awaiting operator acceptance". Goal 0001's implementation is on `main` (squash-merged
  as `ef6a61c`, PR #3), so is it accepted?
- **Default taken:** read "merged onto `main`" as the acceptance its own rule names: the goal file moved
  to `docs/goals/archive/`, its index row now reads "Accepted: merged onto `main` as `ef6a61c`;
  archived", and the links in `docs/evidence/0001-…`, `docs/plans/0001-…` and the goal series index were
  moved with it. Nothing in the goal's content changed.
- **Reversal:** `git mv docs/goals/archive/0001-… docs/goals/` and restore the index row — the file's
  bytes are untouched.
- **Settled by:** whether the operator considers a squash merge the acceptance, or wants to keep the
  contract under `docs/goals/` until they say so explicitly.

## Q-0002-09 — What a dry run's requested verification level reports

- **Status:** resolved (default taken)
- **Question:** `--dry-run --verify resume --resume-prompt X` is accepted, and the run returns before the
  level switch: nothing was installed, so no level could run, and the report used to call a level the
  caller had asked for `not-requested` — while the help's exit-0 clause only had wording for "the target
  harness is not installed", not for "asked, but a dry run installed nothing". A fourth state would say
  the difference in one word; it would also add a wire value no document names yet.
- **Default taken:** the requested level reports `not-verified` with the dry-run reason ("a dry run
  installs nothing, so the target harness has nothing to load"), which is that state's documented
  meaning — the target harness could not answer the question, and the detail says why; a level nobody
  asked for stays `not-requested`; the exit code stays 0 (the conversion and its structural check are
  what the run asked for), and the help's exit-0 clause now names the dry-run case too. The three states
  stay exactly what the plan documents, so no consumer sees a new spelling.
- **Reversal:** one arm of the dry-run switch in `Continue` plus the `result.Conversion.DryRun` return in
  `continuationExit`: the states go back to `not-requested` everywhere.
- **Settled by:** whether the operator wants a dry run to distinguish "asked, but nothing was installed"
  from "the loader refused the session" in the state alone — a fourth state (for example `not-run`)
  would say it in one word, at the price of a new wire value in `verification.*.state`.

## Q-0002-10 — A source that changes while it is converted

- **Status:** resolved (default taken)
- **Question:** the conversion reads the source file, takes its digest and compares it again after
  installing. When the file is edited in between, the conversion and the installation are real, but the
  file no longer holds the bytes the conversion read. The report used to set
  `installation.source_unchanged: false` with a detail and nothing else: no warning, nothing in the
  human output, and exit 0 — so the fact was visible only inside `--json`.
- **Default taken:** exit 0 stays (the conversion and the installation completed; failing would report
  them as failed when they are not), and the fact is made loud instead: the run appends a warning, prints
  `source file: changed while it was converted (…)` in the human block, and keeps the boolean and the
  detail in the JSON. A dry run makes the same before/after comparison, so `source_unchanged: true` is
  never reported without one having run.
- **Reversal:** drop the two `sourceChangedWarnings` appends and the human `source file:` line — the
  boolean and its detail stay in the JSON exactly as before.
- **Settled by:** whether the operator wants a source that moved mid-conversion to fail the run (exit 1).
  The race is one the conversion cannot prevent, and its consequence — the installed session holds the
  earlier bytes — is now stated rather than implied.

## Q-0002-11 — What a dry run records as the session's model provider

- **Status:** resolved (default taken)
- **Question:** the written session's provider binding comes from the target harness's own answer, which
  `Continue` asks for by starting the harness's app server against the target data directory
  (`ProbeCodexConfig`, Q-0002-01). Measured: a started Codex 0.169.0 writes its own state into that home
  (databases, logs, skills), even for the ephemeral `thread/start` the probe uses. A dry run promises it
  writes nothing into that directory, and it also used to create a home the caller named before
  returning, so the probe and the directory creation both made the flag's own printed line false.
- **Default taken:** a dry run neither creates the home nor starts the harness against it, so it records
  no provider: the probe is skipped, `model_provider` is absent from the dry run's conversion,
  `model_provider_source` is `unrecorded` with the reason in `model_provider_detail`, and — when there is
  a harness to ask, whose answer an installed run would record — the run appends a warning saying so. An
  installed run is unchanged: it asks the harness (the home is its own, and the harness's own bookkeeping
  belongs there) and records the answer. Home creation moved to the installation step, so a run that ends
  earlier — a refused reference, an unreadable source, a dry run — leaves the filesystem as it found it.
- **Reversal:** run the probe in a dry run again (one branch in `Continue`) and drop the warning, or move
  the `os.MkdirAll` back above the dry-run branch — the first restores the write into a live home, the
  second restores the false line, so either needs the reviewer's agreement, not just a preference.
- **Settled by:** whether the operator prefers a dry run whose reported conversion is byte-for-byte what an
  installed run would write (at the cost of touching the home it says it does not touch), or a dry run
  that touches nothing and says which one field differs.

## Q-0002-12 — What a conversion does on a filesystem without hard links

- **Status:** resolved (default taken)
- **Question:** the installer publishes a converted session by taking its final name with a hard link,
  which is atomic and exclusive. A filesystem without hard links (some Windows volumes, some network
  mounts) offers no mechanism here that takes a name *without replacing* whatever is at it: `os.Rename`
  replaces, so the previous fallback — check that the name is free, then rename — is exclusive only until
  another installer creates the session in that window, and the rename then replaces that session. The
  review found this while auditing the unconditional "a conversion never replaces a target session" rule:
  may such a filesystem be supported by a non-exclusive fallback, or must the installation be refused?
- **Default taken:** refuse. `InstallCodexRollout` has one publication mechanism, the exclusive hard link;
  when the filesystem cannot take it the installation fails with `ErrNoExclusiveInstall`, naming the
  directory and the link error, and leaves no session file, no placeholder and no temporary file behind
  (`internal/ccc/codex_install.go`). The cost is disclosed rather than hidden: a target data directory on
  such a filesystem cannot receive a converted session at all until a real no-replace primitive is added.
- **Reversal:** restore the existence-check-then-rename fallback (one branch in `InstallCodexRollout` and
  its test), or add a per-platform no-replace primitive — Linux `renameat2(RENAME_NOREPLACE)`, Windows
  `MoveFileEx` without `REPLACE_EXISTING`, or an `O_EXCL` claim of the name followed by a rename of that
  own placeholder, which reintroduces a window in which a reader (including the target's own discovery)
  sees a zero-byte rollout.
- **Settled by:** whether the operator wants filesystems without hard links supported, and if so by which
  primitive. Each alternative trades the unconditional exclusive-publication rule for reach or for a
  visible placeholder window, so the choice is the owner's rather than an unattended default.

## Q-0002-13 — Which shell dialect the printed resume line speaks on Windows

- **Status:** resolved (default taken)
- **Question:** a continuation prints the invocation that resumes the installed session as one line a
  reader may paste (`resume:` in the human block, `resume_command` in the JSON), rendered as a POSIX
  shell line: a leading `CODEX_HOME=<dir>` assignment, then the words, each quoted where a shell would
  act on it (`renderResumeCommand`, `internal/ccc/continue.go`). That spelling is not what PowerShell or
  `cmd` accept — there the same directory is set with `$env:CODEX_HOME='…'` before the command — so on
  Windows the printed line names the right directory in a form that shell cannot paste. CI's
  `gate (windows-latest)` found the consequence: a Windows temp path holds backslashes, which a POSIX
  shell reads as syntax, so the line quotes the directory there while the CLI test asserted this host's
  bare temp path and failed. Should the line be rendered per platform (`$env:` on Windows, POSIX
  elsewhere), be accompanied by a platform note, or stay one POSIX line?
- **Default taken:** it stays one POSIX shell line on every platform, and the test moved instead: the two
  CLI cases that read the line back now install into a directory whose name holds a space, which the rule
  quotes on every host, so one assertion holds everywhere instead of a platform branch
  (`shellUnsafeHome` in `cmd/ccc/main_test.go`; `docs/harness.md` states the quoting as the third Windows
  answer the tree pins). A third case joined them when the sweep added the resume verification's own
  command assertion and spelled this host's temp path: `gate (windows-latest)` failed the `test` step at
  head `37c0a10` for it, and it now uses the same home and the same quoted spelling (the fifth CI red,
  `docs/evidence/0002-claude-to-codex-continuation.md` §6) — a case added to this rule, not a new answer
  to the question above. A fourth case joined the rule in increment 0003, which found the same shape on
  its own route: `TestContinueResolvesARelativeTargetBinaryForTheDirectoryChangingInvocation` asserted the
  bare spelling of the resolved `--target-binary` executable in both printed lines, which every Windows
  temp path's backslashes put in the quoted arm; the caller's directory now holds a space and the two
  assertions spell the quoted word (`docs/evidence/0003-codex-to-claude-continuation.md` §6.4). The
  rendering rule is already the documented contract of the field, a caller
  that runs the harness directly uses the `resume_arguments`/`resume_environment` fields beside it, and a
  per-platform dialect would make the same field mean two things; changing product behaviour to fit a
  test would be the larger move.
- **Reversal:** render by platform in `renderResumeCommand` (a dialect argument from the target
  resolver), or leave the line alone and add a `format_limits`/warning entry saying which shell it is for
  — the second is compatible with every consumer that reads the fields rather than the line.
- **Settled by:** whether the operator wants the printed line to be pasteable on Windows, at the cost of
  a field whose meaning depends on the host that wrote it. Nothing in the conversion, the installation or
  the verification depends on the answer.

## Q-0002-14 — How the source check reports a comparison that did not run

- **Status:** resolved (default taken); supersedes Q-0002-10's field and states, not its default
- **Question:** the source-file comparison answered with a boolean (`installation.source_unchanged`, its
  detail beside it, per Q-0002-10). That shape has two answers for three outcomes: a comparison that never
  ran — an archive read has no file of its own, and a run can stop before comparing — was reported as
  `true` with a detail saying so, so "the file moved" and "nobody compared the file" were different words
  under one value, and the human block printed `changed while it was converted` for a run that had
  observed no change at all (round 5's review read exactly that as a claim). Should the third answer be a
  wire value of its own, or should the boolean stay with its detail carrying the difference?
- **Default taken:** the field is `installation.source_check_state` with the states the rest of the
  document already uses — `unchanged`, `changed`, `not-verified` (the comparison did not run) — with
  `detail` carrying that state's own words; the human block renders the third as `not verified (…)`
  (`sourceCheckWord`, `cmd/ccc/main.go`). The plan's §3/§5, the evidence record's §2 note/§4/§6/§10 and
  `cmd/ccc`'s own test that pins the document's spellings name it. Q-0002-10's decisions stay: a moved
  source exits 0, is warned about, and is printed.
- **Reversal:** rename the field back to the boolean and derive it from the state
  (`source_check_state == "unchanged"`): the three states hold the same information, and only consumers
  that read the installation block see the difference.
- **Settled by:** whether the operator prefers the boolean spelling for a consumer already reading it, at
  the cost of making "did the file move?" unanswerable for a comparison that never ran.

## Q-0002-15 — How the human block spells a report that published nothing

- **Status:** resolved (default taken)
- **Question:** the JSON document has always carried `installation.state` (`installed` / `not-installed`),
  but the human block branched only on `--dry-run` (round 6's review, finding 1): a run that converted and
  then failed before publication — a target home that could not be created is the reproducible shape —
  printed `installed: []` with a target id and two resume commands under it, so the one line a reader
  scans for "is it installed?" answered with an empty path where a published session's path belongs, and
  the identity beneath it read as a session a harness could open. Should the block print the document's
  own state for that case, and should the identity it did generate be marked as provisional?
- **Default taken:** the block renders the report's state — `installed:    not-installed (nothing was
  published under <home>)` whenever `installation.state` is not `installed` and the run was not a dry run —
  and each of the three identity lines (`target id`, `resume`, `non-interactive resume`) carries
  `(proposed; nothing was installed)` whenever the state is not `installed`, a dry run included. The JSON
  is unchanged, and `TestContinueHumanOutputReportsAPrePublicationFailure` pins the block and the document
  together; the evidence record's §11 records it.
- **Reversal:** drop the marker and/or print the state only in the JSON: both are presentation-only, and a
  reader of the JSON sees the same facts either way.
- **Settled by:** whether the operator prefers the shortest block (the state word alone, or nothing) over
  one that names, on every identity line, that no session was published — at the cost of a reader who
  copies a resume command from a failed run having to notice the installation line above it.

## Q-0002-16 — How a conversion reports text the target's reader reads as nothing

- **Status:** resolved (default taken)
- **Question:** round 7's review found a Claude tool output that holds only whitespace (`"\n"`) written as
  a `function_call_output`, counted as carried and expected by the round-trip comparison — while the
  inherited Codex reader reads an output that is only whitespace as no result at all, so the installed
  session failed its own structural check and the run exited nonzero even with `--verify none`. An
  executed probe found the same class twice more on the same surface: a whitespace-only dialogue (a
  message the reader reads as no message) and a compact summary whose trailing whitespace the reader
  trims, which the comparison compared untrimmed. Should such text be written anyway — it is what the
  source recorded, and the target's file format can hold it — with the check disclosing that it cannot be
  read back, or should it be omitted and named?
- **Default taken:** text the target's *own* reader reads as nothing is not written and is named: two
  omitted categories (`whitespace_only_tool_results`, `whitespace_only_dialogue`, both counted only for a
  result or a record the conversion does not write), and a compaction boundary compared by its trimmed
  summary exactly as a result is compared by its trimmed output. The alternative leaves the run's "the
  installed file holds what was written" verdict resting on a comparison that has to skip part of what was
  written, so the report would count evidence as carried that no read of the file can find — the shape
  round 5's review already rejected for omitted results.
- **Reversal:** write the text and report a `carried-with-caveat` category instead ("in the file, absent
  from every read of it"), with the round-trip comparison excluding those records from the evidence it
  expects; the writer's counts and the target file change, the states do not.
- **Settled by:** whether the operator prefers the source's bytes in the file even where the format's own
  reader cannot see them, at the cost of a structural check whose `verified` no longer covers every record
  the report counts as carried.

## Q-0002-17 — How a conversion reports a message the target's reader reads as injected context

- **Status:** resolved (default taken); narrows Q-0002-16's reach, not its default
- **Question:** round 8's review found a valid Claude prompt whose text opens with `# AGENTS.md`. The
  writer carries it verbatim, but the inherited Codex reader reads such a user text block as the context
  Codex itself injects and so reads *no message* for it (`internal/parser/codex.go`'s
  `preprocessCodexUserTextBlocks`): the read-back showed the message missing from the file that holds it,
  and the run exited nonzero even with `--verify none`, because the structural check always runs. Should
  such a message be dropped the way Q-0002-16 drops text the reader reads as nothing, or carried with the
  check disclosing what it cannot read back?
- **Default taken:** carried, counted and disclosed. The writer cannot make this call without
  reimplementing the reader's own classification (a privately maintained predicate over the text, which
  the carry rule forbids) and without discarding user-authored evidence; the reader's rule is
  *content-dependent* — meaningful words start it — where Q-0002-16's blank text is content-independent
  and can never be a message. The check therefore asks the installed file's own bytes whether it holds the
  text (`fileHoldsText`, matching the JSON string a record carries it in): the file holds it → the item is
  counted in `verification.structure.not_read_back_as_dialogue` and warned about; the file does not → the
  item stays in the ordered comparison and is reported as the missing message it is. `conversion.losses[]`
  is unchanged, because nothing was omitted, and the target harness's own view of a session may still show
  the message (`event_msg`/`user_message` is the record that view is built from).
- **Reversal:** treat it like Q-0002-16's blank text — omit the message and count it under its own
  `omitted` category — or accept the read-back's silence without asking the file, which would accept a
  message the conversion never wrote.
- **Settled by:** whether the operator prefers the source's words in the installed file, with the
  structural check's `verified` clearly counting (and naming) the messages its reader cannot show, or a
  conversion that discards a real user turn because the target's own reader classifies its prefix as
  injected context.

## Q-0002-18 — What the structural check asks the installed file about a message the reader reads differently

- **Status:** resolved (default taken); supersedes the *mechanism* half of Q-0002-17 (what the check asks
  the file), not its default (such a message is carried, counted and warned about) and not Q-0002-16's
  (text the reader reads as nothing at all is still omitted and named).
- **Question:** round 9's review found two defects in the question round 8's fix asked. `fileHoldsText`
  accepted a message's text *anywhere* in the installed file, so a session whose model-visible
  `response_item` had been dropped verified against the copy the target's own `event_msg` view keeps (and a
  text quoted by a tool output, or one occurrence answering for several written messages, did the same);
  and it resolved a written message away whenever the file held its text, so a prompt carrying text *after*
  an injected-context envelope — read back by the inherited reader with the envelope stripped — left the
  reader's shorter reading in the ordered comparison and failed a correct run, with `--verify none` and
  `--dry-run` too, because the structural check always runs. Should the check keep asking the file for the
  text, and how does a partial projection resolve?
- **Default taken:** the check asks the file for the message's own dialogue record: a `response_item` whose
  payload is a message, whose role is the message's, and whose content holds a block of the type this writer
  writes for that role carrying the message's own text — counted, so one record answers for one written
  message, and with the other record types, payload types, block types and an empty text all rejected. A
  written message the read-back does not show with its own text is counted
  (`verification.structure.not_read_back_verbatim`, renamed from `not_read_back_as_dialogue` because a
  partial projection *is* read back, just not verbatim) where that record is in the file and stays a missing
  message where it is not; a read-back message is accounted for by a written message only when it is that
  message's own text or the reader's reading of it — a run of the message's characters in their own order,
  under the message's own kind, and only where the file still holds the message's record — and is otherwise
  reported as dialogue the document does not hold. The walk pairs in the file's own order, so a read-back
  (or install) whose order contradicts the document is reported rather than folded away. Nothing here
  re-implements `preprocessCodexUserTextBlocks`: the relation is the structural property every
  deletion-based reading has (a run of the record's own characters in the record's own order), not a copy
  of the reader's envelope list, which the carry rule forbids.
- **Reversal:** ask the file for the text again (a search over its bytes) and let a resolved message be
  compared positionally against the read-back; both defects come back, and the two regressions this
  increment adds — the file whose message record was dropped while its event stayed, and the prompt that
  carries text after an envelope — fail (measured on the pre-fix head in
  `docs/evidence/0002-claude-to-codex-continuation.md` §14).
- **Settled by:** whether the operator prefers the check to hold a conversion to the target's model-visible
  records or only to the words somewhere in the file; a text the reader projects and a text the file does
  not hold are the same bytes to a search, and a partial projection is only distinguishable from a loss by
  which records the file holds.

## Q-0002-19 — How a continuation discloses the provider's own classification of the read

- **Status:** resolved (default taken).
- **Question:** round 10's review found that `Continue` accepted every known source provider but kept the
  provider's own fidelity classifications — a summary transcript, a result the provider says is not
  current for its data version — only in the JSON fidelity block: no warning, no loss category, and the
  human block's fidelity line named only malformed lines and truncation. An accepted invocation could
  therefore present a degraded projection (the Grok session whose chat history is missing reads as one
  synthesized fallback prompt) as an ordinary successful conversion. Should such a read be refused, or
  disclosed?
- **Default taken:** disclosed on every channel, and not refused. `Fidelity`'s classifications are turned
  into words once (`fidelityWarnings`, in `internal/ccc/conversation.go`), so an `Inspect` and a
  `Continue` say the same thing; `Continue` appends them to `warnings[]` (and the human block prints
  warnings), while the conversion's `losses[]` gains `summary_transcript` and `source_needs_retry` —
  both `carried-with-caveat`, count 1 when the classification holds and 0 otherwise, with the provider's
  own retry reason in the detail — mirroring `malformed_source_lines`, which had already put the read's
  own completeness in the loss list. The route is not refused: the messages the provider did return are
  real evidence, the design's rule is to disclose loss by category rather than to hide a usable read, and
  the exit code answers conversion, installation and native verification — not source fidelity.
- **Reversal:** refuse a conversion whose read is classified as a summary or as not current (a new
  error beside `ErrUnsupportedTarget`), or keep the disclosure in the fidelity block alone; the
  regressions (`TestContinueDisclosesASummaryTranscriptRead`, `TestContinueDisclosesARetryableRead`)
  fail either way.
- **Settled by:** whether the operator wants a continuation from a degraded or not-current read to be an
  error, or a disclosed conversion the caller decides about — the same choice Q-0002-16 and Q-0002-17
  took for content the target's reader reads differently.

## Q-0002-20 — How the source check answers a read whose identity is a container member

- **Status:** resolved (default taken)
- **Question:** a multi-session container is read through a *member* identity — the inherited providers
  spell it `parser.VirtualSourcePath(container, member)` (`<container>#<member>`), and the parse records
  that spelling as the session's own path. Round 11's review found that the source check opened that
  spelling as a filesystem path, so continuing a readable SQLite-backed session (an OpenClaw container's
  member is the fixture) failed before the conversion with `open …#main:good: no such file or directory`,
  whatever `--verify` said. Should the check digest the physical container the member lives in, or report
  the comparison as one that did not run?
- **Default taken:** report it as a comparison that did not run — `installation.source_check_state` is
  `not-verified`, with a `detail` naming the member and its container — and so never stop a conversion
  whose source was read. The container's own bytes are not the conversation's: they hold every member the
  container carries, so a digest of them answers a different question than the report asks (did *this*
  conversation's bytes move?), it would report `changed` for a sibling member's write, and it would
  sometimes report `unchanged` for a change that did not reach the file's main bytes (a database's
  write-ahead log, a buffered writer). `sourceFileOf` (`internal/ccc/continue.go`) decides once, from the
  identity, and both halves of the check use it: an identity that names no file is a state of its own, and
  only a read that reports no path at all, a digest that was never taken or a file that cannot be re-read
  stops the run. The rule reaches the plan's §3 bullet and the evidence record's §16.
- **Reversal:** digest the physical container (`parser.ParseVirtualSourcePath` gives it) and report
  `unchanged`/`changed` for the container's bytes, naming the member in the detail; or stop the run as
  before. `TestContinueFromAContainerMemberReportsNoFileToCompare` fails on the second and the third arms
  the same way, and on the first arm only if the container's bytes move, so the decision is one field of
  one report for this source family.
- **Settled by:** whether the operator prefers the third state (no comparison ran, the reason named) or a
  container-level digest as the nearest available file evidence for a member read.

## Q-0002-21 — Which settings the native check seeds into its throw-away Codex home

- **Status:** resolved (default taken)
- **Question:** `scripts/codex-native-check.py` wrote the host's whole `config.toml` into the throw-away
  home the native verification runs against, changing only the two top-level keys
  `--model-provider`/`--model` name, while its report line said no credentials were copied. A real
  configuration is where credentials live — provider authorization headers (`http_headers`,
  `http_headers_helper`), a provider's `query_params` or `auth` block, an MCP server's `env` table, a
  credential written into a URL — and the home is a directory the run reports about and, with `--keep`,
  leaves on disk. Should the check seed an allowlist of settings, copy the document with the
  credential-bearing settings stripped, or refuse to seed a credential-bearing configuration at all?
- **Default taken:** an allowlist, so safety is the default for a setting the target adds tomorrow rather
  than something a strip-list has to notice. `seed_configuration` (`scripts/codex-native-check.py`) reads the
  host configuration as TOML and copies only `model`, `model_provider` and the *named* provider's definition
  through `name`/`base_url`/`wire_api`/`env_key`/`env_http_headers`; the last two hold the *names* of
  environment variables, which is the environment-backed authentication the boundary asks for. Everything
  else — the host's other top-level settings and tables, every other provider definition, the named
  provider's credential-bearing settings — is left behind and named in the run's `seeded from:` line. A
  provider whose `base_url` carries a username, a password, a query or a fragment is left behind *whole*
  (half a definition would send the harness to the provider's default endpoint instead of the one the host
  named), and a configuration that cannot be read as TOML contributes nothing and says so. The rule reaches
  `docs/harness.md` and the plan's §8 bullet; the evidence record's §17 carries the mutations that pin it.
- **Reversal:** copy the document with a denylist of credential-capable keys removed, or require
  `--model-provider`/`--model` and seed nothing from the host configuration at all.
  `SeededHostConfiguration`'s cases fail on the first arm (its fixture's `http_headers` token and its
  URL's own credential are in the bytes, and the document-equality assertion holds extra settings) and on
  the second (the seeding cases assert the copied settings and the report's own names).
- **Settled by:** whether the operator wants the throw-away verification home to carry more of the host
  configuration than the loader's question needs — a denylist, at the price of copying settings whose
  values nothing vetted — or only the settings this check names.

## Q-0002-22 — The ordinary review budget, and the one authorized acceptance audit

- **Status:** open (one-time authorization taken; the audit's result is not yet known). This entry takes a
  **new** id because `Q-0002-01` is already the provider-binding entry above; nothing about this entry
  rewrites or supersedes it, and `Q-0000-01` (merge authority) is untouched.
- **Question:** twelve ordinary machine-review rounds have run on goal 0002's pull request
  (`feat/0002-claude-to-codex-continuation`) and **every one of them returned findings** —
  8 · 3 · 4 · 1 · 2 · 2 · 2 · 3 · 2 · 2 · 2 · 2 = **33 findings**, all closed, **no clean ordinary
  round** — so the ordinary review budget is exhausted. Is the next move a fourteenth round of the same
  kind, a larger review budget, or something else?
- **Default taken:** exhausting the budget is a signal, not a number to raise. The consulted cross-family
  decision was to perform **one systematic close-out of the complete fidelity and verification surface**,
  and then **exactly one fresh, read-only acceptance audit** whose scope is a *frozen rule list* — never a
  further automatic review cycle. The close-out enumerated the obligations derived from the core design,
  the goal's `Done when` clauses, the public output claims and the behavioral claims of every shipped check
  and test — including all 33 earlier findings — and mapped each to its implementation, its positive
  evidence and **one executed mutation that detects its violation**, without weakening an obligation to
  match the implementation. Its record is
  [`docs/evidence/0002-obligation-matrix.md`](../evidence/0002-obligation-matrix.md); the rule list it
  froze is that file, and the head it froze is `3ee6cb2` plus the close-out's own repairs (three
  obligations had no gate-level pin and gained one: the file holds both record kinds, the loader's
  answered id *and* path must be this session's, "resumable" requires the turn to append to the session,
  and the provider is the harness's own answer). Two alternative readings were rejected as the consulted
  decision instructed: code introduced by earlier fix rounds is **not** out of contract (a
  credential-copying native check or a falsely-positive structural check is a real obligation), and native
  verification is **not** optional decoration to be cut — it is central to the goal.
- **Stop condition (binding for the unattended series):** if the acceptance audit returns even one finding,
  the run **stops for decomposition/replanning** — no further automatic cycle, no re-frozen list, no
  "one more round". An audit that returns no finding is what ends the ordinary review for this goal, and
  the operator still owns the merge (`Q-0000-01`'s conditions are unchanged: CI green on the verified
  head, a clean machine-review evidence comment naming that head, zero unresolved threads, and the
  operator's own gate run).
- **Reversal:** delete the audit's authorization and return to ordinary rounds — the close-out is
  additive (a new evidence file, four new tests, no assertion or gate rule weakened), so nothing has to be
  unwound first; the cost is that the same twelve-round shape (a finding every round, none of them the same
  one) continues, which is the signal this entry records.
- **Settled by:** the acceptance audit's own verdict on the frozen list — which is why the list was frozen
  **before** the audit runs and is a file rather than a summary in a pull-request comment.

## Q-0002-23 — Whether the response stream is bounded too, and by what

- **Status:** open (default taken: no change to the response path; a measured limitation is recorded).
- **Question:** the sweep of 2026-11-16 (see
  [`docs/evidence/0002-obligation-matrix.md`](../evidence/0002-obligation-matrix.md), E6) bounded the
  *diagnostics* both harnesses keep — the Go client's app-server stderr and the resume turn's two pipes,
  the native check's stderr ring — but the **response** stream is bounded by count only, or not at all:
  `scripts/codex-native-check.py`'s `_read_stdout` appends every line it reads to an unbounded
  `queue.Queue` (measured: 100 001 retained items for a 100 000-line flood), and
  `internal/ccc/codex_verify.go`'s client bounds the number of decoded messages (`lines chan []byte`,
  64 slots) but not the size of one message. Should either harness bound the response path as well, and by
  count, by bytes, or both? The clause in the matrix says "a protocol that stalls or floods is reported as
  a failed check rather than blocking or exhausting the run"; for responses, only the *stall* half is
  enforced today by the per-request deadline.
- **Default taken:** no code change, and the limitation declared with its own measurement. A bound on the
  response path is not a free repair: a bounded queue must either block the reader (which stops the reader
  draining the pipe and can stall a server that is answering) or drop messages (which drops the answer a
  call is waiting for), turning a flood into a *timeout* — a behaviour choice about which failure a caller
  prefers, and one an operator should make rather than an unattended lane. The two harnesses also differ
  in consequence: the Go client is gate-covered and serves the shipped CLI, while the Python harness is
  the re-derivable native evidence and runs against the operator's own target binary.
- **Reversal:** bound the queue (a `maxsize` with a blocking put keeps every message and stops the reader
  at the bound) and bound a single line (`readline(size)` with the remainder dropped, the same shape the
  diagnostics drainer now uses), then pin both with a case that drives the reader over a flood; in Go,
  either cap one decoded message or decode through a bounded reader and answer `not-verified` with the
  harness's own message when the bound is hit.
- **Settled by:** whether the operator wants a flooding (or merely enormous) target answer to be a
  bounded failure rather than this process's memory, and which of "block the reader", "drop the answer" or
  "cap the message size" is the acceptable failure shape for the CLI they will run.

## Q-0002-24 — Whether a conversion reserves its new identity against the target home's own ids

- **Status:** open (default taken: the claim was corrected to what is enforced; no code change).
- **Question:** obligation D1's frozen sentence read "the target session id is a fresh UUID v7, never the
  source's session id and never an id already present in the target home"
  ([`docs/evidence/0002-obligation-matrix.md`](../evidence/0002-obligation-matrix.md), D1). The shipped
  conversion never consults the home: `ConvertToCodexRollout` mints a fresh v7 when it is handed no id
  (`internal/ccc/codex_target.go`), `Continue` hands it none, and the exclusive install refuses a taken
  *file name* (`ErrTargetExists`) — which does not cover an id the home already holds under another day or
  timestamp. The ids the home held are compared *after* the install by the native harness's own check
  (`scripts/codex-native-check.py`), not by the conversion. Should a conversion reserve its identity
  against the ids the home holds (a second walk of the target sessions tree, plus an injectable id source
  so a collision can be tested), or is "a fresh v7 plus an exclusive install" the guarantee the project
  wants — with a v7 collision treated as unrepairable-and-unreviewed?
- **Default taken:** the claim was corrected to what the three checks enforce, and the residual declared.
  The goal's own clause asks for "the target identity is new and an unrelated existing Codex session is not
  replaced", both of which hold; reserving against the home is new behaviour, on the conversion path the
  gate covers, for a collision whose probability is that of a v7 collision (and whose detection the native
  harness already performs on the home it runs against).
- **Reversal:** enumerate the home's `rollout-*.jsonl` ids before converting, pass them to the writer, and
  regenerate while the drawn id is in the set; then assert it through an id-source seam, with the mutation
  "the reservation removed" and a home that already holds the id the seam draws first.
- **Settled by:** whether the operator wants the conversion to be the place that guarantees id uniqueness
  across the home (paying a home walk on every conversion), or wants the guarantee to stay "fresh id +
  exclusive install", with the home's ids checked by the native verification and the id-uniqueness
  obligation stated at that strength in the matrix.

## Q-0002-25 — Which conditions stop the source check's run, and what the three states answer

- **Status:** resolved (default taken: the row's sentence was corrected to the behaviour the design asks
  for; no code change).
- **Question:** obligation A7's frozen sentence read "only a read that names no file at all (or a digest
  that was never taken) stops the run"
  ([`docs/evidence/0002-obligation-matrix.md`](../evidence/0002-obligation-matrix.md), A7). The shipped
  check stops the run on **three** conditions, not two: a read that reported no path at all
  (`internal/ccc/continue.go`'s `sourceCheckAfter`), a digest that was never taken, and a named file that
  cannot be **re-read** after the conversion (`sourceCheckAfter`'s `fileDigest` failure, reached through
  `Continue`'s post-install comparison, whose answer `reportAfterFailure` reports as a run that stopped).
  Is the third condition a defect — should a file that cannot be re-read instead answer `not-verified`
  like an archive read or a container member — or is stopping what the design asks for?
- **Default taken:** the third condition is the design's, and the sentence was the imprecise part. The
  three `SourceCheck*` states answer questions about the *source's shape* (an archive row has no file, a
  container member's identity is not a file's name), which the read knows before the conversion and which
  the report answers with the member's or the archive's own words. A file that was named and then cannot
  be re-read is the opposite: the read did have a file, the comparison the report exists to make did not
  happen, and the report may not claim either "unchanged" or "this shape has no file" — the run stops,
  the installed session's identity stays reported (`reportAfterFailure`), and the failure names the
  re-read. The check's own doc comment states this reading, and the case that pins it
  (`TestContinueKeepsTheReportWhenTheSourceVanishesAfterTheInstall`) asserts the report carries
  `not-verified` with the re-read failure named and does **not** warn that the source moved. The corrected
  sentence enumerates the three states and the three stop conditions, cites the case for the re-read
  condition and `reportAfterFailure` for the stopped run's answer, and the never-taken-digest branch is
  now pinned by `TestSourceCheckStopsAComparisonWhoseDigestWasNeverTaken` (before: no positive case).
- **Reversal:** make a failed re-read a `not-verified` answer instead — one `switch` in
  `sourceCheckAfter` (answer `SourceCheckNotVerified` with the re-read failure) and the vanished-source
  case changes from "the run fails" to "the run succeeds with not-verified" — which would state that a
  conversion whose source could not be compared is complete rather than unproven.
- **Settled by:** whether the operator prefers a conversion whose source comparison could not run to fail
  (today) or to succeed with `not-verified` and the reason (the archive read's shape).

## Q-0002-26 — Which checks a stopped run reports `not-verified`

- **Status:** resolved (default taken: the row's sentence was corrected to the report's actual answers; no
  code change).
- **Question:** obligation D5's frozen sentence said "every check that did not run is reported
  `not-verified` with the reason"
  ([`docs/evidence/0002-obligation-matrix.md`](../evidence/0002-obligation-matrix.md), D5). The report
  answers two different things, and deliberately: a check the run's **level asked for** and the run never
  reached is `not-verified` with "the run stopped before this check ran: …", while a check **no level
  asked for** stays `not-requested` with its level named (`internal/ccc/continue.go`'s
  `reportAfterFailure`: the `VerifyNone` and default arms). Folding the second into `not-verified` would
  make "could not run" and "nobody asked" the same answer, which is exactly what the three verification
  states exist to keep apart, and the cited case's helper (`stoppedRunChecks`) asserts both answers.
- **Default taken:** the sentence was too broad, and the behaviour is right. The corrected clause reads:
  a check the run's level asked for and the run never reached is `not-verified` with the reason; a check
  no level asked for is `not-requested` with its level named — and the row now cites the helper that
  asserts both, and the two `reportAfterFailure` arms that produce them.
- **Reversal:** set `State = not-verified` in the `VerifyNone`/default arms too (two lines in
  `reportAfterFailure`), then the helper's `Resume.State == not-requested` assertion changes with it and
  the human block prints "not verified" for a level nobody asked for.
- **Settled by:** whether the operator wants a stopped run to blur "nobody asked" into "could not run" for
  the levels the caller did not request; the three states' own documentation says it should not.

## Q-0002-27 — Whether the native check's own `run()` captures are bounded too

- **Status:** resolved (default taken: no code change to `run()`; the residual is declared with its own
  measurement, and the clause's subject is the target's diagnostics).
- **Question:** the E6 sweep note recorded a second residual beside the response-stream one (Q-0002-23):
  `scripts/codex-native-check.py`'s `run()` — the helper the native harness uses for `go build`,
  `ccc continue`, `ccc inspect` and the resume turn — captures each subprocess's whole stdout and stderr
  in memory (`subprocess.run(..., capture_output=True)`), so a subprocess that floods either stream is
  retained in full until it exits. Is that a violation of "Subprocesses are bounded" (E6), or is the
  clause's subject the *target harness's* diagnostics — the app-server stderr ring and the Go turn's two
  pipes, both now bounded — while `run()`'s captures are this harness's own call outputs?
- **Default taken:** the latter, and the residual declared. The clause's word *floods* is about the
  protocol's diagnostics (both harnesses bound those by bytes now); `run()` is not part of the app-server
  protocol at all, it is the harness's own subprocess plumbing, and every consumer of its captures reads a
  bounded piece of them (`build.stderr[:-400]`, `last_meaningful_line(...)`, the parsed JSON documents,
  the turn's answer lines through the report's own truncation). What bounds the exposure today is the
  subprocess's own deadline (`run()`'s `timeout`: 600 s for the build/convert/inspect calls, 900 s for the
  turn) plus the smallness of what those calls answer — measured, not assumed: a throwaway probe over the
  shipped `run()` driving `python3 -c 'import sys; sys.stdout.write("x" * (64 << 20))'` printed
  `captured=67108864 byte(s)` — the helper does retain what the child wrote, and the bound is the child's
  own output and deadline rather than a cap. Bounding it in the helper would have to choose between killing a subprocess whose answer the
  harness is waiting for and truncating a JSON document the check parses, the same behaviour choice
  Q-0002-23 records for the response path.
- **Reversal:** give `run()` a byte bound per stream (read both pipes through bounded readers, mark the
  capture truncated, and let the callers' own truncation report it), then pin it with a case that drives a
  stub subprocess over the bound; the build/convert/inspect calls would report a truncated answer instead
  of an unparsable one only if the parse of a truncated document is made an explicit failure.
- **Settled by:** whether the operator wants a flooding (or merely enormous) *helper* subprocess answer to
  be a bounded failure too, and which of "kill it at the bound", "truncate the document" or "leave the
  subprocess's own deadline as the bound" is the acceptable failure shape.

## Q-0003-01 — A source that records no working directory

- **Status:** resolved (default taken)
- **Question:** Claude Code's loader resolves a session only from the directory whose project-dir
  spelling matches the one it was installed under (`claude -p --resume <id>` run elsewhere answers
  `No conversation found with session ID`), so a source read that recorded no working directory cannot
  be installed where any loader would find it. Invent a directory (the caller's, or a fixed one), or
  refuse?
- **Default taken:** refuse the conversion with `ErrNoTargetCwd`
  (`internal/ccc/claude_target.go`'s `ConvertToClaudeTranscript`) before anything is written, and report
  the conversion block without an installation, rather than invent a directory that would make the
  session undiscoverable while the report called it installed. `claude_target_test.go` and
  `continue_claude_test.go` pin the refusal.
- **Reversal:** fall back to the caller's working directory (one branch in `claude_target.go`),
  disclosed in the report as an invented location.
- **Settled by:** whether the operator prefers a refused conversion over one installed at a disclosed
  guessed directory.

## Q-0003-02 — One record per source message, with a fresh `message.id`

- **Status:** resolved (default taken)
- **Question:** the inherited Claude reader merges *consecutive assistant records that share a
  `message.id`* as streaming chunks of one response, and a message can carry several content blocks.
  Write one record per message, or one record per block, and where does the id come from?
- **Default taken:** one record per content block of a source message — the message's dialogue as its own
  record (a plain string for a single text block, an array otherwise), each tool call as its own
  assistant record, and a message's tool results together on one user record — with one fresh unique
  `message.id` per source assistant message, shared by that message's records only; sharing an id across
  two messages would merge them (measured), while a single multi-block record was measured to
  reconstruct to exactly the request the harness's own block-per-record shape produces. The new UUID v4
  is both the file stem and every record's `sessionId`, because the inherited reader reads a record
  whose `sessionId` differs from the stem as a *child* session.
- **Reversal:** derive ids from the source's own message identity where it has one, which needs a
  per-source mapping and a source identity the read does not always carry.
- **Settled by:** whether the operator wants the target's message identity traceable to the source's.

## Q-0003-03 — `is_error` only where the source recorded a failure

- **Status:** resolved (default taken)
- **Question:** `tool_result.is_error` is the target's own field (measured to survive resume), and a
  source status has richer meanings than "failed". Write a flag for every non-success status, or only
  where the source said the call failed?
- **Default taken:** `is_error: true` is written **only** where the source's own status says the call
  failed (`internal/ccc/claude_target.go`); every other status is carried as text and counted under
  `tool_result_statuses`, because a status with no error meaning has no target field and inventing one
  would assert a failure the source did not record.
- **Reversal:** write no flag at all (0002's `tool_result_outcomes` caveat), or map every non-success
  status to `is_error`.
- **Settled by:** whether the operator wants the target's error flag to track the source's status
  taxonomy or only its recorded failures.

## Q-0003-04 — The model field, and the provider a Claude transcript records

- **Status:** resolved (default taken)
- **Question:** the source read carries a per-message model, and Claude Code's transcript has a
  `message.model` field but no provider field. Carry the source's model — the only field-shaped place
  for it — or omit it and record no provider binding?
- **Default taken:** omit the source's per-message model and count it (`message_model_attribution`), and
  record no provider binding anywhere in the transcript, because `message.model` states which model
  produced the words and the conversation was not produced by one of the caller's target models; the
  absence is disclosed as a loss category and as a format limit.
- **Reversal:** write the source's model into `message.model`, which would attribute the conversation to
  a model that did not produce it.
- **Settled by:** whether the operator prefers a transcript bound to no model over one that names the
  source's.

## Q-0003-05 — The `load` check is offline, with an unknown-id control arm

- **Status:** resolved (default taken)
- **Question:** the default verification level could prove the loader by running a real turn against the
  caller's own endpoint, or by driving the loader without model access and reading its own resolution of
  the session. Which is the default?
- **Default taken:** the offline one. `--verify load` (the default) runs the installed `claude` binary
  with `claude --bare -p --output-format json --resume <id>` against a **throw-away copy** of the
  converted bytes in a scratch `CLAUDE_CONFIG_DIR`, with the variables that could reach a model removed,
  plus a **control arm** on a fresh unknown id that must answer `No conversation found with session ID`
  (`internal/ccc/claude_verify.go`). The default level therefore bills no turn and alters no installed
  session, and the contrast with the control arm is what makes it an answer about *this* session rather
  than about any file.
- **Reversal:** an online load check against the caller's own endpoint (one branch in `claude_verify.go`),
  which costs a turn and cannot run without model access.
- **Settled by:** whether the operator wants the default verification to prove the loader with a real
  turn instead of the offline resolution.

## Q-0003-06 — The resume command carries the working directory

- **Status:** resolved (default taken)
- **Question:** the session is discoverable only from a particular directory (`Q-0003-01`), so the
  printed resume command could carry the directory itself or state the requirement in prose and leave the
  caller to `cd`.
- **Default taken:** carry it — `cd <cwd> && CLAUDE_CONFIG_DIR=<home> claude --resume <id>`, rendered the
  way 0002 renders its commands (every word quoted where a POSIX shell would act on it), because without
  the `cd` the loader's project-directory lookup cannot see the session and without the home it looks in
  the caller's own (`internal/ccc/continue.go`'s `renderInvocation`).
- **Reversal:** print the invocation without the `cd` and document the requirement instead.
- **Settled by:** whether the operator wants the pasted command to carry the directory or the
  documentation to.

## Q-0003-07 — Pre-boundary history, and the boundary's own pair

- **Status:** resolved (default taken)
- **Question:** a source `compacted` record could truncate the written transcript, or be written as the
  marker the target itself writes so the target applies its own compaction semantics while the file
  keeps the history.
- **Default taken:** carry the pre-boundary turns and count them (`pre_compaction_history`, with its
  caveat), and write the boundary as the pair the harness itself writes —
  `{"type":"system","subtype":"compact_boundary"}` followed by the `isCompactSummary` summary record.
  Measured: that pair truncates the target's model-visible history at the boundary (the loader's request
  began with the summary and a pre-boundary fact answered `NOT-IN-CONTEXT`), while the `isCompactSummary`
  flag alone changed nothing.
- **Reversal:** stop the file at the boundary.
- **Settled by:** whether the operator wants pre-boundary turns in the file — the history the boundary
  record is what withholds from the model — or omitted.

## Q-0003-08 — `isSidechain` records are omitted

- **Status:** resolved (default taken)
- **Question:** the source read marks some messages `IsSidechain`; the transcript could carry them (with
  the flag) or omit them and count the omission.
- **Default taken:** omit them, counted (`sidechain_messages`), because the loader was measured to drop
  `isSidechain` records from the model-visible history: carrying them would put evidence in the file that
  no read of it — including the conversion's own round-trip check — could show, and the target has no
  subagent attribution for a flat message.
- **Reversal:** write them with the flag, and report that the target's own reads do not surface them.
- **Settled by:** whether the operator wants a subagent's turns visible in the file even though no target
  read shows them.

## Q-0003-09 — The archived goal file's own `Status` line

- **Status:** resolved (default taken)
- **Question:** 0002's merge onto `main` (`8e1c502`) makes it accepted, so the bookkeeping fix round
  moved its goal file to `docs/goals/archive/` and the index row now reads `Accepted: merged onto `main`
  as `8e1c502`; archived`; the file's own `**Status:**` line still reads `Implemented on
  `feat/0002-claude-to-codex-continuation`; awaiting operator acceptance`. 0001's archived file was
  re-spelled to `Accepted: merged onto `main` as `ef6a61c`; archived` in the commit that archived it,
  while this round's finding told the round not to touch the goal text beyond its location and the
  README row.
- **Default taken:** the finding's own limit — the file's text stays as reviewed, and the README row
  carries the accepted state.
- **Reversal:** one line in `docs/goals/archive/0002-claude-to-codex-continuation.md`: the `**Status:**`
  line re-spelled to `Accepted: merged onto `main` as `8e1c502`; archived`, matching 0001's archive.
- **Settled by:** whether the owner wants an archived goal's own status line re-spelled to the accepted
  state (0001's practice) or frozen as the text the operator accepted.

## Q-0003-10 — The active goal file's own `Status` line

- **Status:** resolved (default taken)
- **Question:** `docs/goals/0003-codex-to-claude-continuation.md` still opens with `**Status:** Ready`
  while the index row at `docs/goals/README.md` reads `Implemented on
  `feat/0003-codex-to-claude-continuation`; awaiting operator acceptance`. The apply round's own finding
  named this and left it alone: the goal file is the owner's artifact, the index declares itself the
  status authority, and 0003 has not been accepted yet. Q-0003-09 records the same question for the
  *archived* 0002 file.
- **Default taken:** the goal file's line stays as it is; the README row carries the state, and the
  question is recorded here rather than answered by an edit to a file the owner is still reviewing.
- **Reversal:** one line in `docs/goals/0003-codex-to-claude-continuation.md`: `**Status:** Ready` →
  `Implemented on `feat/0003-codex-to-claude-continuation`; awaiting operator acceptance`.
- **Settled by:** whether the owner wants an active goal's own status line to track the index row while
  the increment awaits acceptance, or to stay the text the increment was dispatched against.

## Q-0003-11 — The printed invocation names the probed path, not the caller's word

- **Status:** resolved (default taken)
- **Question:** the Claude route's invocation runs from the session's own working directory (Q-0003-06),
  so a bare `--target-binary claude` printed as `cd <session dir> && … claude --resume <id>` is read by a
  shell that searches `PATH` *again* from that directory. Review round 2 asked for the probe's own answer
  to be named instead; the alternative is that a pasted line can run a different program than the one the
  conversion asked and the native checks ran — and, on a host whose `PATH` holds a relative entry, one
  beside the session rather than beside the caller.
- **Default taken:** name the path the probe found (`invocationBinary(in.Binary, in.Probe)`,
  `internal/ccc/targets.go`), keeping the caller's spelling only when the probe found nothing (a path-like
  spelling still resolved against the caller's directory, a bare name left as written). The report's own
  `target_session.binary` / `binary_found` fields already carry that same answer, so the printed line and
  the report agree. The cost is that a default `--target-binary` now prints an absolute path — a line that
  names an installed program rather than the word the caller typed, and that may need editing before it is
  pasted on another host.
- **Reversal:** one branch in `invocationBinary`: return the caller's spelling whenever it is the bare
  name the route defaults to (or unconditionally), and keep the probe's answer only in the report's
  `binary` fields.
- **Scope:** the Codex route's printed invocation is *not* changed here: it runs from the caller's own
  directory, where a bare name is the PATH lookup the probe itself made, and 0002's printed form is
  accepted evidence this increment does not rewrite. The same shell-versus-probe question can be asked of
  its `codex resume <id>` line, and answering it belongs to a 0002-scoped change.
- **Settled by:** whether the operator wants the printed resume command to name exactly the program this
  run probed and verified, or to stay the spelling a caller typed.

## Q-0004-01 — Which providers get a byte-level record boundary

- **Status:** resolved (default taken)
- **Question:** the boundary's `records_end_bytes`/`unpublished_tail_bytes` describe where a *line-recorded*
  source's last complete record ends, and this build can only justify that claim where the provider says so
  itself. Two of the inherited declarations are candidates — `parser.SourceCapabilities.IncrementalAppend`
  (this source grows by appended lines, which its read can only re-answer by parsing the whole file again)
  and `parser.ContentCapabilities.MalformedLineCount` (its read counts records it could not parse) — and the
  set of providers declaring either is not the set of providers whose format is line-recorded: Claude and
  Codex declare them, while Copilot's JSONL transcript declares neither, so a Copilot source gets no
  record boundary while an identical file shape does under another provider.
- **Default taken:** claim the offsets when either capability is `CapabilitySupported`, and otherwise report
  the file's bytes (path/size/digest) with `records_are_lines: false` and no record claim — the conservative
  side, because the alternative is this build inferring a format from its own reading of the bytes. The
  provider this reader cannot resolve (a disabled provider, a stored agent label with no factory) also
  answers false, so the boundary states bytes and claims nothing.
- **Reversal:** widen the predicate in `sourceRecordsAreLines` (`internal/ccc/continue.go`) — e.g. also
  treat a source whose file ends in a line feed as line-recorded, or carry an explicit list of agents.
- **Settled by:** whether the owner wants the record boundary for providers whose format is line-recorded
  but which declare neither capability (Copilot's transcript is the measured example).

## Q-0004-02 — The boundary is not taken by `ccc inspect`

- **Status:** resolved (default taken)
- **Question:** `ccc inspect` reports the same `SessionReport` shape a conversion does, so it could carry the
  boundary too — a user comparing a stored conversion against the source's *current* size would not have to
  read the file themselves. Taking one costs another full read of a source that can be hundreds of megabytes
  (the design forbids duplicating an expensive source fingerprint solely for bookkeeping), and the
  inspection already pays for a provider fingerprint and a parse.
- **Default taken:** the boundary belongs to a conversion (`source.boundary` is absent from an inspection,
  and the type's own comment says why). What an inspection already reports — the read's counts, timestamps,
  fidelity, and the parse's own recorded file size — is what a caller compares against.
- **Reversal:** call the boundary pass from `Inspect` too (one more full read per inspection) and reuse the
  same block.
- **Settled by:** whether the operator wants `ccc inspect` to answer "is my continuation current with the
  source right now?" from one command, at the cost of a second full read of the file.

## Q-0004-03 — An unfinished source record is a boundary fact, not a loss category

- **Status:** resolved (default taken)
- **Question:** the goal says only complete recorded events are eligible for a snapshot, so an unterminated
  trailing record is not evidence that was left out — it was never a record. It could still be named in
  `conversion.losses[]` (whose own rule is that every omission is counted there), beside the categories the
  writers reason over, or it could live in the boundary and the warning channel alone.
- **Default taken:** the boundary (`unpublished_tail_bytes`), the run's `warnings[]` and the human
  `unpublished:` line. The loss list is unchanged: for a Claude source the inherited reader's own
  `malformed_source_lines` count still travels in it (it counts what *the parser* skipped, including an
  unterminated tail), so nothing is hidden — and for a Codex source, whose parser deliberately counts
  nothing for a live session's tail, the boundary is the disclosure. Adding a category would have made two
  different statements (bytes of an unfinished record; records the parser skipped) look like one count.
- **Reversal:** add a loss category (e.g. `unpublished_tail`) to both writers' loss tables, driven from the
  boundary, and pin it in each writer's own list.
- **Settled by:** whether a consumer that reads only `conversion.losses[]` needs the tail named there.

## Q-0004-04 — The repeat scenario is native-verified for the 0002 route, with a second prompt flag

- **Status:** resolved (default taken)
- **Question:** the contract names the accepted 0002 route (Claude Code → Codex) for the repeated-transfer
  evidence, and one real turn is what the route's check has always asked for explicitly. Running the repeat
  scenario also for `scripts/claude-native-check.py` (0003's route) would double the native work; running
  *two* real turns in the Codex check (one before the growth, one after) is two billed turns where the
  caller's `--resume-prompt` used to be one.
- **Default taken:** the repeat scenario lives in `scripts/codex-native-check.py`, and each real turn has its
  own flag: `--resume-prompt` (the first conversion's session, run *before* the growth, which is also what
  makes that session one its harness has already resumed) and `--repeat-resume-prompt` (the second, later
  session). Both are optional; the offline default loads both sessions through Codex's own app server and
  runs no turn, and the run's report line says how many turns ran. The repeat property itself (two
  conversions of one growing source, the earlier target untouched) is pinned for **both** target routes by
  the Go test, so 0003's route is not left unpinned — only without a native repeat run.
- **Reversal:** add the same scenario to `scripts/claude-native-check.py` (its own copy-then-grow step and a
  second prompt flag), and/or fold the two prompt flags into one that runs both turns.
- **Settled by:** whether the operator wants the repeat scenario re-derived natively for the Claude Code
  target as well, and whether one flag should mean two turns.

## Q-0004-05 — When the boundary is taken, and what it therefore does not claim

- **Status:** resolved (default taken)
- **Question:** a conversion reads the source and then measures it. A file that grows between the two makes
  the boundary *newer* than the bytes the parser consumed, and nothing in the report says so. The alternative
  was to compare the boundary's size with the size the parser recorded (`source.identity.file_size`), but for
  a provider whose source is a layout that number is a composite (`internal/parser`'s sidecar handling), so
  the comparison would sometimes report "the file grew" when it did not.
- **Default taken:** the boundary is taken once, after the read and before anything is written, and the
  block's `detail` states exactly that ordering; no comparison with the parse's own stat is attempted. A file
  that moved *during the conversion* is still answered, by the existing source check
  (`installation.source_check_state`), which compares the same digest after the installation.
- **Reversal:** take the boundary before the read as well and compare the two digests (one more full read),
  reporting "the file grew while it was being read" when they differ.
- **Settled by:** whether the operator wants the mid-read growth window closed by measurement, at the cost of
  a second full read of every source.

## Q-0004-06 — 0003's goal file, its status line, and this record's name

- **Status:** resolved (default taken)
- **Question:** 0003 was merged onto `main` as `18d50b2`, so this increment archives it (Q-0003-10 asked
  whether the file's own `**Status:** Ready` line should track the index row first; Q-0003-09 asked the same
  for 0002's archived file) — and the increment's own evidence record's name decides whether
  `scripts/tests/test_evidence_claims.py` discovers it, since that case discovers `*continuation*.md` and
  requires the records it names to credit a recorded turn's answer.
- **Default taken:** the goal file moved to `docs/goals/archive/0003-codex-to-claude-continuation.md`
  **unchanged**; the index row carries `Accepted: merged onto main as 18d50b2; archived`, and the three links
  that named its former path (0003's plan, evidence record and obligation matrix) were retargeted. The two
  append-only `OPEN-QUESTIONS.md` entries that name the former path were left as written, because the file is
  append-only and never edited. This increment's evidence record is
  `docs/evidence/0004-repeatable-continuation-handoffs.md` — a name holding "continuation", so the
  answer-crediting rule discovers it — and it was added to that case's `ROUTE_DOCUMENTS`.
- **Reversal:** re-spell the archived file's `**Status:**` line (as 0001's archive did) in one line, and
  rename the evidence record to mirror the goal file's own stem if the discovery rule is widened instead.
- **Settled by:** whether the owner wants an archived goal's own status line re-spelled (0001's practice) or
  frozen as the text the operator accepted (0002's and 0003's), and whether the evidence-record naming should
  follow the goal file's stem rather than the discovery rule.

## Q-0004-07 — The record-boundary gate is `IncrementalAppend` alone (Q-0004-01 superseded)

- **Status:** resolved (default taken)
- **Question:** Q-0004-01 gated the boundary's record offsets on "either of the provider's two candidate
  declarations", believing that conservative. The review round measured the premise false: in the carried
  tree, `parser.ContentCapabilities.MalformedLineCount` is declared by providers whose source is not a line
  transcript at all — QwenPaw counts malformed *message entries* inside one whole-file JSON document
  (`internal/parser/qwenpaw_provider.go`), CodeBuddy counts them inside a manifest object, and DeepSeek
  Harness's source may be a compressed container whose raw bytes hold no line feed. A complete QwenPaw file
  converted with `records_are_lines: true`, `records_end_bytes: 0`, the whole file as
  `unpublished_tail_bytes`, and a "converting again carries them" warning — a user-visible false statement,
  and one `AGENTS.md`, `docs/conventions.md`, the plan, the evidence record and the code comment all rested
  on. The gate's own premise ("both are declarations only a line-recorded source has") was simply wrong.
- **Default taken:** claim the offsets only when `parser.SourceCapabilities.IncrementalAppend ==
  CapabilitySupported` — the declaration that the source grows by appended lines, which is exactly what makes
  the offset after the last line feed a *record* boundary. Claude and Codex declare it; no other carried
  provider does. Every other provider (including one that declares only `MalformedLineCount`) gets the file's
  bytes and digest with `records_are_lines: false`, no record offset and no unfinished-record warning. The
  regression cases are `TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations` (QwenPaw, CodeBuddy,
  DeepSeek Harness rows, each measured failing against the old gate) and
  `TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary` (a complete QwenPaw document converted end to
  end, which reproduced the false statement before the fix).
- **Reversal:** widen the gate again for a provider the owner names explicitly, but pair the widening with
  the two claims it needs — stop calling the offsets "records" and warn of an "unfinished record" only where
  `IncrementalAppend` is declared — so the words and the bytes agree whatever the gate admits.
- **Settled by:** the same question Q-0004-01 asked (does a line-recorded provider that declares only a
  malformed-entry count get the offsets?), now answered by measurement rather than by preference; a reversal
  needs either that provider to declare `IncrementalAppend` or the owner to accept the renamed claims.

## Q-0004-08 — The boundary reports the read's own snapshot, and only where it may read that as one file's

- **Status:** resolved (default taken)
- **Question:** Q-0004-05 declined to compare the boundary's size with the size the parser recorded
  (`source.identity.file_size`), because for a provider whose source is a layout that number is a
  composite and the comparison would sometimes report a file that moved when it did not. The review round
  measured what that left open: the conversation is read first and the boundary is taken afterwards, and
  for Codex the inherited parse is *bounded by the size it saw when it opened the file*
  (`internal/parser/codex_rollout_open.go`'s `plainCodexRolloutSnapshot`), so a complete turn appended
  during the parse is excluded from the conversation while the boundary — and the subsequent
  `installation.source_check_state` — describe the newer bytes and report the file `unchanged`. The result
  attributed an older conversation to newer bytes with nothing in the report to notice it by.
- **Default taken:** the boundary answers the question it can support, from the read's own record: the
  pass compares the file's size with the size the read recorded for it, and reports `read_bytes` (or
  nothing, when the read recorded none) with `read_coverage: verified` when they are the same — the
  conversion read these bytes — or `read_coverage: unverified` with both sizes in `read_coverage_detail`
  and in the run's warnings when they are not. The comparison is asked only where the provider's own
  declarations make the recorded size this file's own (`sourceRecordsAreLines`: the two providers whose
  source is a transcript of appended lines are the two whose parse records the transcript's own size —
  Claude's `info.Size()`, Codex's `snapshot.Source.Size()`, compressed or not), which is exactly the set
  Q-0004-05's objection was about excluding. The offsets are still reported when the coverage is
  unverified: they are byte facts about the file this block names, and the coverage answer beside them is
  what says the conversion is not shown to have covered them.
- **Reversal:** compare a provider whose source is a layout with the file anyway (the objection Q-0004-05
  raised), or take a second full read before the parse and compare that digest (one more pass over a source
  that can be hundreds of megabytes, and the same answer for the readers in this tree).
- **Settled by:** whether the owner wants a layout provider's composite size compared with one file's
  (Q-0004-05's objection restated for the narrowed gate), and whether the read-coverage answer belongs on
  the boundary block or in its own report section.
- **Regression:** `TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead` (the state a mid-read growth
  leaves, at the pass's own seam — a genuine end-to-end race cannot be made deterministic, since the parse
  runs inside `Continue`), `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile` (both channels),
  and the repeated-conversion case's `read_bytes == bytes`/`read_coverage: verified` assertions, which are
  the end-to-end half.

## Q-0004-09 — The record-offset gate has a second half: the file's own bytes

- **Status:** resolved (default taken)
- **Question:** Q-0004-07 narrowed the gate to the provider's declaration
  (`parser.SourceCapabilities.IncrementalAppend`), which is a statement about a *format*. The review round
  measured that a provider-wide declaration is not a statement about one representation of it: Codex both
  declares that capability and reads its own zstd-compressed rollouts (`.jsonl.zst`), which the inherited
  implementation decodes before parsing and refuses any byte offset into
  (`internal/parser/codex_rollout_open.go`, `codex_provider.go`'s `ParseIncremental`). The boundary scanned
  the compressed file for line feeds, so such a source could report an arbitrary "record boundary", an
  "unfinished record" and unpublished bytes while the decoded history had been carried in full.
- **Default taken:** the offsets are claimed only when *both* halves hold — the provider's declaration, and
  the file's own bytes being able to be that transcript. The second half is measured in the same pass
  (valid UTF-8, chunk by chunk, with the bytes of a rune a chunk boundary splits carried to the chunk that
  completes it): a transcript of lines is text, and a compressed or otherwise binary container of one is
  not — a zstd frame's own header is not valid UTF-8, so every zstd container is caught, whatever suffix it
  carries. The withheld claim is stated (`record_offsets: not-claimed` with `record_offsets_detail` and a
  human `records:` line naming which half failed), the file's size and digest are still reported, and
  `read_bytes`/`read_coverage` still answer the attribution question for it.
- **Reversal:** gate on the path's own name (the suffix the reader dispatches on) or on a magic number,
  both of which duplicate a format fact in `internal/ccc`; or ask the provider per source, which the
  inherited interface does not offer cheaply (its only per-source answer, `ParseIncremental`, needs a
  checkpoint for providers like Claude and would narrow their own case wrongly).
- **Settled by:** whether the owner wants the container's *decoded* records described too (the boundary
  has no seam for the decoded bytes: the file's own digest is what a caller compares two conversions by),
  and whether "not text" is the right floor for the second half or a provider-declared per-source
  capability should be added upstream.
- **Regression:** `TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets` (end to end: a
  runtime-compressed copy of the committed Codex fixture, whose decoded history must match the plain
  rollout's conversation while the boundary claims no offsets and warns of nothing),
  `TestSourceBoundaryOfFileStatesTheRecordOffsets`'s container row, and `TestTextStreamCarriesSplitRunes`.

## Q-0004-10 — An unterminated record is a byte fact, not a publication claim

- **Status:** resolved (default taken)
- **Question:** Q-0004-03 put the unfinished tail in the boundary rather than in the loss list, phrased as
  `unpublished_tail_bytes` with a warning saying those bytes "are not part of this conversion". The review
  round measured that claim false: both inherited full readers accept a *valid* record at EOF without its
  line feed (Codex's own tests pin it — `codex_cursor_test.go`'s "newline-less valid EOF is parsed but not
  seeded" — and Claude's loop processes any line it can parse), so the pipeline carries such a record while
  the boundary and the human `unpublished:` line asserted its omission. The old phrasing also named the
  offset after the last line feed "where the records end", which is untrue for the same file.
- **Default taken:** what the boundary states is the file's own record structure — the offset its
  line-feed-terminated records end at (`terminated_records_end_bytes`) and the bytes after it
  (`unterminated_tail_bytes`), which no line feed follows and which the source may still be writing — and
  it claims nothing about whether the read carried it. That answer stays where it is answered: the read's
  own fidelity block (Claude flags such a tail truncated; Codex counts nothing for a live tail) and the
  conversion's own counts against a conversion of the same copy without the tail. The warning names both
  offsets, says the source may still be writing the record, and keeps the goal's remedy ("converting again
  once the source has been written reads the file again"), with no claim of omission.
- **Reversal:** judge the tail's completeness in `internal/ccc` (a JSON validity test would make this build
  read a format, and would be wrong for a provider whose records are not JSON), or keep the omission claim
  and require it true by construction (providers would have to report the tail's own disposition, which the
  inherited interface has no seam for).
- **Settled by:** whether the owner wants the boundary to state a publication verdict where the provider
  supplies one (Claude's `truncated` flag is such a case) rather than leaving every verdict to the fidelity
  block and the counts.
- **Regression:** `TestContinueCarriesACompleteRecordThatHasNoLineFeed` (the committed Claude growth
  fragment, appended twice — with and without its last byte: the same conversation both times, with only
  the second boundary reporting an unterminated record), `TestUnterminatedSourceTailIsBoundaryedAndNeverPublished`
  (which now also asserts the warning claims no omission), `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile`,
  and `TestSourceBoundaryOfFileStatesTheRecordOffsets`'s "a complete record without its line feed" row.
