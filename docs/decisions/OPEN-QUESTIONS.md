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

- **Status:** resolved (owner, 2026-11-19 — the default taken stands). This entry takes a
  **new** id because `Q-0002-01` is already the provider-binding entry above; nothing about this entry
  rewrites or supersedes it, and `Q-0000-01` (merge authority) is untouched.
- **Confirmed (owner, 2026-11-19):** no change to the existing behaviour. The ordinary budget's exhaustion
  behaviour is unchanged (stop → cross-family consult → design fork to the owner), and the Stop condition
  below governs as written; this confirmation neither amends it nor endorses any particular count of past
  audit rounds.
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

- **Status:** resolved (owner, 2026-11-19 — the default taken stands).
- **Confirmed (owner, 2026-11-19):** no code change; the measured limitation stands as recorded.
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

- **Status:** resolved (owner, 2026-11-19 — the default taken stands).
- **Confirmed (owner, 2026-11-19):** the claim stays corrected to what the checks enforce; the residual
  stands.
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
  the second boundary reporting an unterminated record; §13's round made the case a table over both accepted
  source directions and added the Codex growth fragment it needs, Q-0004-13),
  `TestUnterminatedSourceTailIsBoundaryedAndNeverPublished`
  (which now also asserts the warning claims no omission), `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile`,
  and `TestSourceBoundaryOfFileStatesTheRecordOffsets`'s "a complete record without its line feed" row.

## Q-0004-11 — The read-coverage answer is a size match, not byte identity (Q-0004-08's wording superseded)

- **Status:** resolved (default taken)
- **Question:** Q-0004-08 answered the read-coverage question with `read_coverage: verified` — "the
  conversion read these bytes" — when the file's size equals the size the read's own snapshot recorded. The
  review round measured that the equality does not support the claim: the read's own record
  (`ParsedSession.File.Size`) is a size and no digest of the bytes it opened, so a file replaced by a
  *different* transcript of the same length after the read leaves every size equal while the conversation
  came from bytes that are gone. Nothing else in the report can catch it either: the boundary's digest is of
  the file as it stands, and the source comparison's two digests are both taken after the read — so the
  state and its words were the whole of the claim, and both were stronger than the measurement.
- **Default taken:** the states say what was compared. They are `size-match` and `size-mismatch` (not
  `verified`/`unverified`), the match state's detail names the comparison and its limit ("the file is the
  size the read recorded, which is a size match and not a byte-identity check -- the read's own record holds
  no digest of the bytes it opened, so a same-length replacement of the file after it is not ruled out"), the
  human `boundary:` line says the same in one clause, and the mismatch state keeps its own words (both
  sizes, the file moved while it was being read, coverage not established) and its warning. The byte
  identity the earlier wording claimed is not obtainable at this seam without a second full read before the
  parse (Q-0004-08's own rejected reversal) and is not what the design asks a caller to compare two
  snapshots by: the digest beside the state is, and it is the file's own.
- **Reversal:** a provider-level content digest could restore a "these bytes" claim for a source whose read
  records one (the inherited `Fingerprint` is the provider's own artifact — a parse's or a store member's —
  so it is not comparable with one file's bytes), or the boundary pass could be moved before the read to
  bound the snapshot from the other side.
- **Settled by:** whether the owner wants the boundary to take a second full read of the source before the
  parse to compare digests (one more pass over a source that can be hundreds of megabytes), or a
  provider-declared per-source digest added upstream.
- **Regression:** `TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity` (an equal-size replacement at the
  pass's seam: `size-match`, both sizes reported, the digest of the file as it stands, the detail's limit
  named, no warning, and neither `verified` nor "the conversion read these bytes" anywhere in the answer —
  measured red against the pre-fix code, which printed both), and the repeated-conversion case's
  `read_bytes == bytes`/`read_coverage: size-match` assertions, which are the end-to-end half.

## Q-0004-12 — The copy's preservation is one comparison per window, and the growth baseline is one of them

- **Status:** resolved (default taken)
- **Question:** the native check's closing verdict, "the source session is unchanged", is measured against
  the *copy* this run converts. The review round measured that the baseline it compared from was adopted
  from whatever bytes the copy had when the fragment was appended: the first conversion's own comparison and
  the append are separated by that session's native loader and its optional real turn, and a *same-length*
  change made in between leaves the growth record's own claim true (its size delta still holds and its digest
  still differs from the pre-conversion digest), so the changed bytes became the baseline every later
  comparison — the closing verdict included — measured from. The record therefore said the source had
  survived the whole repeated transfer without having checked the bytes its later comparisons stood on.
- **Default taken:** the copy is compared with the bytes taken before the first conversion **immediately
  before it grows** (`the source copy still holds the bytes the first conversion read`), recorded as its own
  check rather than folded into the growth record, whose size claim is true either way. Every window a copy
  must survive now has a case of its own, each driving one *same-length* swap and asserting its exact failure
  set (`drive`'s `copy_mutation` table in `scripts/tests/test_codex_native_check.py`): a change while a
  conversion runs fails that conversion's own comparison and (for the first) the growth baseline; a change
  while the first session's loader runs fails the growth baseline alone; a change while the second conversion
  runs fails the second copy comparison and the closing verdict; a change while the second loader runs fails
  the closing verdict alone.
- **Reversal:** the growth record could compare a *prefix* digest of the grown file with the pre-conversion
  digest instead of taking a separate comparison before the append (one check covering the same interval);
  or the harness could stop converting a copy and grow the committed fixture's own path, which the goal's
  "the source is never opened for writing" rule forbids.
- **Settled by:** nothing owner-only — which intervals a comparison covers, and that each has a case, is the
  record's own business; the reversal is a cleanup, not a question.
- **Regression:** the four `test_a_copy_mutated_…` cases, each executing its own mutation on every run; with
  the shipped comparison removed, the first-load case reports an empty failure list — measured
  (`Lists differ: [] != ['the source copy still holds the bytes the first conversion read']`).

## Q-0004-13 — The no-line-feed rule is pinned per source direction, with a Codex growth fragment

- **Status:** resolved (default taken)
- **Question:** §11's F02 fix left this record claiming `TestContinueCarriesACompleteRecordThatHasNoLineFeed`
  pinned the rule "in both directions" while the case converted a Claude source only: the Codex source's
  incomplete-tail case is about malformed JSON, not about a *complete* record whose final line feed is
  missing. The review round measured the claim false and asked for the missing direction or a narrowed claim.
- **Default taken:** the direction was added rather than the claim narrowed, because the rule is about a
  reader reading a complete unterminated record and the accepted routes have two source readers. A committed
  `growth/codex-complete-turn.jsonl` — the same question and answer as the Claude fragment, in Codex's own
  `response_item` shape, generated and pinned by `internal/ccc/fixturegen` like every other fixture — drives
  the case's Codex → Claude half, and the case is now a table over the two accepted sources. Its own search
  text is asserted to sit in the fragment's **last** record, the one its second half leaves unterminated, so
  a case that searched a terminated record cannot stand in for it.
- **Reversal:** narrowing the record's claim to the Claude direction was the smaller fix and remains
  available; it would leave the Codex reader's whole-record-at-EOF behavior pinned only at the parser level
  (`internal/parser`'s `codex_cursor_test.go`), not end to end through the boundary and the conversion.
- **Settled by:** nothing owner-only.
- **Regression:** both halves of the case. Measured red by cutting the no-feed half's last record mid-JSON
  (which is what makes it incomplete): the Claude half reported `read as 9 message(s) and carried 6, against
  10 and 7 with it` and the Codex half `read as 7 message(s) and carried 5, against 8 and 6 with it`, each
  also failing the search for the last record's dialogue; and by reading the fragment's *first* text field
  instead of its last, which failed the Codex half alone with `the case searches for "What budget do retries
  share per session?", which is not in the fragment's last record …`.

## Q-0005-01 — A Pi assistant entry always carries a `usage` object, with zero counts

- **Status:** resolved (default taken)
- **Question:** Pi's assistant records carry a `usage` object whose counts are the harness's own
  accounting. A converted session's turns were produced by the source's provider, so what counts should
  the record carry — the source's, a computed estimate, or none?
- **Default taken:** always write a `usage` object, its counts zero, and name it in the report
  (`unrecorded_token_usage`, carried-with-caveat, with what it costs). Measured on 1.5.2: the loader
  indexes `message.usage.input` for every assistant message it loads, and a written message without a
  usage object stops a resumed run with `undefined is not an object (evaluating 'message.usage.input')`
  — so "none" is not available. Zero is the answer that attributes nothing: the source's provider billed
  the source's session, and no number this build could write would be that provider's rate or this
  session's spend.
- **Reversal:** carry the source's recorded per-message counts (the inherited reader holds them per
  message: `parser.ParsedMessage.TokenUsage`), which would make the target's own totals look like this
  session's spending; or copy the source's cost table, which this build has no rates to justify.
- **Settled by:** nothing owner-only. The caveat names the cost of the default: a long carried history
  reads to Pi's own context estimate as cheaper than it is until the first resumed turn reports its own
  usage.

## Q-0005-02 — `firstKeptEntryId` is the boundary's own id, and `systemMessage` is null

- **Status:** resolved (default taken)
- **Question:** Pi's compaction entry names the first entry the boundary kept and may carry the prompt
  checkpoint the context is rebuilt from. A source's compaction record (a Codex `compacted` record, a
  Claude `compact_boundary` summary) names the summary and the boundary but not which entries were kept,
  and no checkpoint at all.
- **Default taken:** write `firstKeptEntryId` as the compaction entry's own id — Pi's own retain-none
  shape, measured in the harness's `buildContextEntries`, where a boundary whose kept id is its own
  leaves nothing before it retained — and `systemMessage` as an explicit null, because Pi installs its
  own prompt and tool loadout at request time and a reconstructed checkpoint would be a prompt nobody
  recorded. Measured on the converted fixture: the loader then reports the boundary's summary as the
  leading `compactionSummary` entry of its model-visible history, with the turns after the boundary
  following it.
- **Reversal:** claim the entries after the source's boundary as the kept range (inventing a boundary
  position the source never recorded), or reconstruct a checkpoint from the target's own prompt.
- **Settled by:** nothing owner-only.

## Q-0005-03 — A Pi native check drives the loader against a copy, from the session's recorded directory

- **Status:** resolved (default taken)
- **Question:** Pi's loader is driven offline by its RPC mode, and it resolves a session id through the
  project directory spelled from the process's own working directory. Should the check run the loader
  against the installed session, and from where?
- **Default taken:** against a throw-away copy in a scratch agent directory, from the session's own
  recorded working directory. Two measurements settled it: (1) an RPC run **appends** its own records (a
  `thinking_level_change` among them) to the session it opens, so driving the loader against the
  installed file alters what the conversion published — the installed file's digest is compared around
  the whole stage, which is what makes the copy an assertion rather than a preference; and (2) a process
  whose working directory spells another project is answered with `Session found in different project:
  <recorded>` and a *fork* offer rather than a load, which is not the conversation's own resume command —
  so the check runs from the recorded directory and reports a recorded directory that does not exist on
  this machine as "the load could not be verified" rather than reading the fork offer as a pass. The
  scratch copy is installed under the same project spelling as the real one, which is what makes the
  lookup find it.
- **Reversal:** run the loader against the installed file (mutating it), or recreate a stand-in path
  inside the scratch tree (whose project spelling is a different project's, which is the fork offer
  again).
- **Settled by:** nothing owner-only.

## Q-0005-04 — The Pi resume command prints no `--provider` and no `--model`

- **Status:** resolved (default taken)
- **Question:** the source's own last message records a provider and a model. Should the printed Pi
  resume command carry them, so the resumed turn runs under the instruction the session records?
- **Default taken:** neither is printed. The source's provider is a *source-side* label — AgentsView
  spells the agent that wrote the words (`claude`, `codex`, `pi`) — and Pi's own provider ids are the
  endpoints the caller's configuration defines, so the label names an endpoint the target home need not
  have. Measured twice on 1.5.2, both times before the session was even opened: `--provider claude`
  against a Pi home whose `models.json` defines one endpoint stopped with `Unknown provider "claude"`,
  and `--model claude-sonnet-4-5` stopped with `ambiguous across providers … No matching provider is
  authenticated`. A resume command that cannot open the session is not a resume command, and the earlier
  candidate flags also failed goal 0005's own "resumed by Pi" evidence. The attribution is not lost: the
  assistant entries carry the source's provider and model verbatim, and the report says so
  (`message_model_attribution`, carried-with-caveat) beside the `model_provider_binding` format limit.
- **Reversal:** print the model (risk: the target cannot open the session when the id is ambiguous or
  unauthenticated), or print `provider/model` from the source's label (which Pi cannot resolve at all).
- **Settled by:** nothing owner-only; a caller who wants a specific model passes Pi's own flags.

## Q-0005-05 — `ccc continue --to pi` accepts a session whose recorded directory is not on this machine

- **Status:** resolved (default taken)
- **Question:** Pi's lookup is path-scoped, so a session installed under a working directory this host
  does not have cannot be loaded here. Should the conversion be refused (as a missing recorded directory
  is, `ErrNoTargetCwd`), or written and reported?
- **Default taken:** written and reported. The design's rule is that a conversion is a snapshot and that
  the native half needs the target installed; the target home is the caller's, and the session is
  installed exactly where the target's own discovery will find it on a machine that has the directory —
  which is the machine a cross-machine handoff is for. The load check reports `not-verified` naming the
  missing directory, the run's warnings carry it, and the exit code is the one a verification that could
  not run earns.
- **Reversal:** refuse the conversion when the recorded directory is absent here (which would make a
  cross-machine continuation impossible), or install into an invented directory (which is what
  `ErrNoTargetCwd` exists to prevent).
- **Settled by:** nothing owner-only.

## Q-0005-06 — Which fixture the Pi parity oracle records

- **Status:** resolved (default taken)
- **Question:** goal 0005 adds two Pi fixtures. Which one gets a recorded upstream-parity dump, given
  that `scripts/upstream-compare.py` needs network and a second checkout?
- **Default taken:** the ordinary session (`pi/01a1c335-…`), which exercises the reader's identity,
  ordering, tool-call/result pairing and per-message model fields in both trees. The branch/compaction
  fixture has no recorded dump in this increment; its own properties are pinned offline by this tree's
  tests (`TestPiHistoryFixtureBranchIsNotOnTheActivePath` walks the fixture's tree, and the continuation
  cases pin what a conversion does with it), and the case can be added the next time the comparison is
  run with `--record`.
- **Reversal:** run `python3 scripts/upstream-compare.py --record` with the second fixture added to both
  `Cases` and `CASES`; nothing depends on the omission except that the reader's projection of *that*
  fixture is not proven against the pin.
- **Settled by:** nothing owner-only.

## Q-0005-07 — A Pi target reproduces the source's own parent relation, rather than being flattened or cut

- **Status:** resolved (default taken; the review named the choice as an operator decision and this is the
  reversible default it landed on)
- **Question:** the target's session format is a tree (`id`/`parentId`), and the inherited readers answer
  each record's own identity and the record it hangs off (`Message.SourceUUID` / `Message.SourceParentUUID`;
  a Pi session's `parentId`, a Claude transcript's `parentUuid`). 0005's first writer chained every written
  entry to the one before it and dropped those two fields. That was measured wrong: for the committed
  branch fixture the pre-fix conversion made Pi's own loader read
  `[compactionSummary, user, assistant, assistant]` — the abandoned prompt "And the retry cap?" standing as
  an active user turn — where the source file answers `[user, assistant, assistant]`. The goal forbids
  exactly that (`Bounds`: "do not treat unrelated inactive branches as additional user turns"). Should the
  writer reproduce the source's tree, or write only the source's active path and count the rest as omitted?
- **Default taken:** reproduce the source's tree. Each entry is chained to the entry of the record the
  source named as its parent, so the target's loader walks the source's own active path from the file's
  last entry and an abandoned branch stays off it as *recorded history* — which is where the source's own
  file keeps it, so nothing is dropped either. Three consequences are part of the rule: a record whose
  named parent the target omitted is chained to the nearest ancestor that was written; a record that
  follows a tool result is chained to the *result* (the reader answers a Pi tool result as not a visible
  ancestor, while the source's file runs call → result → what follows, and the other reading would leave a
  resumed session with a call and no result); and a parent the read cannot resolve at all follows the entry
  before it and is counted (`unresolved_source_parents`), because that relation is this conversion's and
  not the source's. Re-derived natively (offline, Pi's own RPC loader on the source and on the conversion):
  both answer `messageCount 3`, `[user, assistant, assistant]`.
- **Reversal:** write only the source's active path, omitting and counting the off-path records (a smaller
  file, at the cost of the recorded history the source's own compaction leaves in place), or write a
  `branch_summary` entry for an abandoned path (which would require inventing a summary the source never
  recorded). Neither is needed to satisfy the goal's clause; the default keeps every record the read
  produced.
- **Settled by:** nothing owner-only; the review's own alternative (omit-and-count) is the reversal above.

## Q-0005-08 — A converted Pi assistant entry records the model and no provider (Q-0005-04's attribution clause superseded)

- **Status:** resolved (default taken; 0005's review named the substitution as a defect)
- **Question:** Pi's `message` record has a `provider` field beside its `model`, and the read reports a
  session-level provider too. 0005's first revision wrote that session-level provider — the *harness*
  label, `claude` / `codex` / `pi` as AgentsView spells the agent that wrote the words — into the field,
  and reported it as `model_provider_source: source-messages` with a detail saying the source's own
  provider travelled verbatim. That is false twice over: the committed ordinary Pi fixture records
  `example-proxy` while the conversion wrote `pi`, and the read's provider is not an endpoint Pi's own
  configuration defines. Should the entry name the harness, omit the field, or invent an endpoint?
- **Default taken:** the model travels verbatim and **no provider is written**. The read answers the model
  each source message recorded (`Message.Model`, which the inherited Pi reader projects: reading the
  committed fixture through it answers `example-model-2` per assistant message) and no endpoint
  attribution at all — the Pi reader does not project a message's `provider` field — while the session-level
  provider names the harness. The report records none (`model_provider_source: unrecorded`) and states why
  in `model_provider_detail`, and `message_model_attribution`'s detail says the endpoint attribution is
  what the read cannot answer. Q-0005-04's own conclusion is unaffected (the printed resume command still
  names no provider or model); only its closing claim that "the assistant entries carry the source's
  provider and model verbatim" was wrong and is superseded here.
- **Reversal:** write the harness label as before (measured wrong: it states an endpoint the caller's
  configuration need not define), or add a per-message provider to the interchange document and fill it
  from the parse where the reader answers one (`ParsedMessage.ProviderID`: Codex's service tier, Cline's,
  Posit's). That would be a real option for a later increment, but a billing identity is not necessarily
  an endpoint either, and for this route the Pi reader answers none — the reversal changes nothing here.
- **Settled by:** nothing owner-only.

## Q-0005-09 — A target whose format is one line writes the source's own line and counts the rest (Q-0005-07 extended)

- **Status:** resolved (default taken; 0005's second review round named the flattening as the same defect
  Q-0005-07 had fixed for the Pi target)
- **Question:** the outbound routes write a source's records into targets whose formats differ. Pi's
  session JSONL and a Claude Code transcript are trees — a record names its parent — while a Codex rollout
  is one sequence of records with no parent relation. Both routes wrote *every* record of the branch
  fixture as ordinary history, so the compaction boundary and the abandoned retry exchange became part of
  the target's model-visible conversation (measured: the installed Claude transcript's `parentUuid` walk
  held the summary, the abandoned prompt and its answer, where the source file's own walk holds three
  turns). Should those routes reproduce the source's tree too, or drop what the source's own line does not
  reach?
- **Default taken:** the tree-shaped targets are written as the source's own tree, exactly as Q-0005-07
  decided for Pi: each record hangs off the record the source named as its parent (a record's *final*
  entry, when it writes more than one, and the tool result standing between a call and what follows),
  so the target's loader walks the source's active path and the branch stays in the file beside it as
  recorded history — the shape Claude Code's own transcripts have when a conversation is rewound. A
  Codex rollout cannot hold one: a response item written into it becomes model-visible history (measured:
  the same fixture converted to a rollout read back as `6` messages where the source's own line holds
  `3`), so that route writes the source's own line and omits the records the source's relations place off
  it, counted as `inactive_branch_records`; the boundary that stood on the branch is not written at all,
  because a `compacted` record there would cut a line it was never on. Every omission is named in
  `conversion.losses[]`, and the branch itself is still in the source session, convertible on demand.
- **Reversal:** write the branch into the Codex rollout as well (measured wrong: an abandoned turn
  becomes active history), or refuse a branched source for that target (which would trade a disclosed
  omission for an unavailable conversion).
- **Settled by:** nothing owner-only.

## Q-0005-10 — A projected parent is read through the tool result standing between the records, and the reading is gated on the source's own reader

- **Status:** resolved (default taken; 0005's second review round named the projection as a defect: a
  record the Pi reader answers as hanging off the message that made a call may in the source's file have
  hung off a result of it, and treating the answer as exact left the result off the target's walk — and,
  for the Codex target, omitted it as an inactive branch)
- **Question:** the inherited Pi reader resolves a record under a tool result to its visible ancestor, so
  a record whose real parent is a tool result answers with the *call's* message. When another record (a
  branch's own turn) was written between the result and that record, the writers' immediate-predecessor
  rule no longer sees the result: the entry is chained straight to the call and the result stands off the
  walk (or is omitted by the Codex target as inactive). Should the writers undo the projection by walking
  back to the nearest result of the call the record names, and what should the report say about a relation
  the read's projection cannot settle?
- **Default taken:** where the read may answer projections, a record naming a call message is chained
  through the nearest tool result of that call written before it — the reading the source's own file order
  supports (`call → result → what follows`) — and the relation is counted
  (`unresolved_source_parents`) because the read's answer cannot settle whether the record hung off the
  call or off the result; the count's detail says exactly that. The reading is gated on the source's own
  reader: only the Pi parser and the Pi-derived readers (the agents `internal/parser/provider.go` builds
  with `newPiProviderFactory` and `newTauProviderFactory`) resolve descendants of a tool result to their
  visible ancestor, and for a read whose parent answers are exact (a Claude transcript's `parentUuid`
  names a record) the narrower immediate-predecessor reading is kept, so no relation is invented for a
  read that did not project. (The gate's first revision took the step for an exact read too, where the
  result was the record immediately before the one naming the call; 0005's third review round measured that
  as the same defect in a narrower shape — an exact read's answer names the record it means, so the result
  is that record's *sibling* — and Q-0005-12 records the correction: for an exact read the step is not
  taken at all.) The gate is a small name list in `internal/ccc/route_shared.go`
  (`projectedParentProviders`) because the inherited registry exposes no format-family predicate and the
  carried parser is not edited here.
- **Reversal:** apply the reading to every reader (which invents a relation for an exact read: measured
  against the Claude fixture's shape, a record naming a call while a result of that call stands between
  them would be chained through the result and counted, although the read answered the relation exactly),
  or leave the answer exact everywhere (measured wrong for Pi: the Codex target omitted the active result
  as `inactive_branch_records` and the tree targets left it off the walk — the defect this entry answers).
- **Settled by:** an upstream registry predicate naming the readers that project (the list would then be
  read from it), or a reader that keeps the record's own parent answer beside the projected one. A
  Pi-family agent this list does not name keeps the narrower reading, which under-discloses rather than
  inventing a relation.

## Q-0005-11 — A source whose selected leaf writes no entry is refused rather than converted

- **Status:** resolved (default taken; 0005's second review round measured that an omitted final record can
  make the target's loader select an abandoned branch's entry as its leaf)
- **Question:** the target's own loader walks from the last entry a conversion wrote, while the source's
  loader walks from the read's last record. When the read's last record writes no entry (an empty
  assistant turn, a system record, a subagent's own row) and the entry written last stands for a record
  the read's last record does not reach, the target would resume from a branch the source's conversation
  left — a context substitution no per-record loss count discloses. Should the conversion write an entry
  for the leaf anyway, silently keep the branch's entry as the last one (what it did), or refuse?
- **Default taken:** refuse (`ErrUnrepresentableLeaf`, naming the record the last entry stands for and
  that it is off the line the read's last record answers). The check runs after the walk, on the same
  line computation the writers use, so it is the read's own answer that decides; a source whose last
  record writes no entry *on* that line is unaffected (the last written entry is then an ancestor of the
  leaf, and the walk the target makes is the written part of the source's line), and so is a read with no
  relations at all, whose order is linear. The refusal is narrow by construction: it fires only when the
  last written record is off the leaf's line, which requires an omitted tail *and* a branch written after
  the last on-line entry.
- **Reversal:** write a minimal entry for the omitted leaf so the target's last entry is the leaf's own
  (narrowed by the omission's reason: a system record's text, a subagent's own words and a whitespace-only
  dialogue must not be written as the conversation's own turn, so no single rule covers every reason), or
  keep the silent substitution (measured wrong: the target resumes the abandoned branch).
- **Settled by:** nothing owner-only. A target format that records its own selected leaf (a `leafId`-like
  field) would make the refusal unnecessary where the format can name one.

## Q-0005-12 — The projection step is taken only where the read projects, and the Codex line walk's own places are counted

- **Status:** resolved (default taken; 0005's third review round named both halves as defects: the ungated
  immediate tool-result step revived an exact read's sibling result, and the Codex route disclosed only
  `inactive_branch_records` while its walk placed records through relations the read never answered)
- **Question:** two halves of one rule. (a) `sourceTree.parentFor` chained a record through the tool result
  immediately before it whenever that result named the same record, and `sourceLineParent` did the same for
  the Codex walk — without asking whether the read may answer a projection at all. For an exact read (a
  Claude transcript's `parentUuid`) that result is a *sibling*: in the shape `C, R(parent=C), A(parent=C)`
  the source's relations place `R` off the line the conversation runs, and chaining `A` through `R` revives
  `R`'s own turn — a user record can hold dialogue beside its result — as an ancestor of the active path,
  while the report counted nothing. (b) The tree writers count the relations the read could not place
  (`unresolved_source_parents`); the Codex route consumed only the boolean of `inactiveSourceRecords`, so a
  record the walk kept on the line through a projected parent — or through the record before it when the
  read answered no parent, or none it holds — travelled as "the source's own line" with no count.
- **Default taken:** (a) the tool-result step is taken only where `projectedParentProvider` says the read
  may project, on both the tree writers and the Codex walk; an exact read resolves every named parent by
  its own answer, and a result standing beside the record it names is that record's sibling (kept in the
  file for the tree targets, off the walk). `sourceToolResultBefore` lost its `projected` parameter: where
  the read does not project the step is not taken at all. (b) `inactiveSourceRecords` answers the count
  beside the flags — the places its walk chose that the read did not answer: the record chained through a
  result standing between it and the call it named (a result immediately before it is the file's own
  continuation, `call → result → what follows`, and is not counted, exactly as the tree writers do not
  count it); a record the read gave an identity but no parent, that is not the conversation's first; and a
  record naming a parent this read does not hold. The count is reported as `unresolved_source_parents`
  (carried-with-caveat) with a detail that names the three relations, and every one of those records is
  still written: the count names the place, not an omission.
- **Reversal:** take the immediate step for every read (measured wrong: with the gate removed the adjacent
  exact case's assertion fails — the sibling result and its dialogue come back as an ancestor of the
  active path), or keep the Codex report to `inactive_branch_records` (measured under-disclosing: the
  projection case is the tree writers' count of 1 on the same document, and without it the rollout
  presents the line it built as the source's by right).
- **Settled by:** nothing owner-only; the gate follows the reader predicate Q-0005-10 already names, and a
  future upstream registry predicate would replace it there.

## Q-0005-13 — A written tool result is registered under the record a record hanging off it would answer, and the projection step's place is counted wherever it is taken (Q-0005-12's "(b)" clause superseded)

- **Status:** resolved (default taken; 0005's fourth review round named both halves as defects: the
  registration key was the *carrier's* own read-answered parent, so a call's outcome the archive folds onto
  the call's message was registered under the record the call's message hangs off and a sibling of the call
  naming that record was chained through the abandoned call's result — reviving the call branch on the tree
  writers and on the Codex walk alike — and both writers read a result *immediately* before the record as
  settling the projection, which the read cannot do)
- **Question:** two halves of one place. (a) Which source record is a written tool result registered under,
  so that a later record naming that record is chained through the result rather than off the walk the
  target's loader makes? The registration used the walking record's read-answered parent, which is the
  call's message only for a result the read holds as a record of its own: an archive read folds a
  result-only row onto the call's own message and drops the row (`ingest.PairAndFilterContext` feeding
  `archive.messageFromArchive`), so the walking record *is* the call's message and its read-answered parent
  is the message above the call — a record naming that one is no candidate for the outcome at all, and
  registering it there chained such a record through the abandoned call's result. (b) Is the projection
  step counted when the result stands immediately before the record that names the call, or is adjacency
  evidence that the record hung off the result (the file's own continuation, `call → result → what
  follows`)? The two source files — `C, R(parent=C), A(parent=C)` and `C, R(parent=C), A(parent=R)` — are
  the same answer in the read (the Pi-family readers resolve a result's descendants to the visible
  ancestor), so adjacency cannot settle which one the file held: the review's measurement showed the same
  read reactivating an inactive result while reporting zero uncertainty.
- **Default taken:** (a) the registration key is the record a record *hanging off the outcome* would read
  as its parent (toolResultCarrierRecord): the message's own identity where that message carries a call at
  all (the folded shape the archive produces — a result record of its own carries no call, because the
  reader pairs its result with a call of another record) and the result record's read-answered parent
  otherwise. One function answers it for all three routes — the Pi writer per written result, the Claude
  writer once per outcome-holding record, and the Codex walk's `sourceToolResultBefore` — so the routes
  cannot drift; the registration itself stays gated on the read that may project (Q-0005-12 (a)). (b) the
  step is counted wherever it is taken: the reading taken is the file's own order, and the count names the
  place as this conversion's rather than the read's. The consequence is disclosed rather than hidden: an
  ordinary *linear* Pi conversation reports one counted place per record that follows a tool result (the
  committed Pi fixture's own `pi:… → --to claude` conversion now reports `unresolved_source_parents ×1`
  where the earlier revision reported nothing, with the converted bytes unchanged), and the folded-outcome
  cases (a record naming the parent of a call whose outcome is folded onto the call's message) report zero
  counted places, because no result of that parent stands anywhere for the read to have projected through.
- **Reversal:** (a) register under the walking record's read-answered parent (measured wrong in this
  round: the folded shape chains a sibling of the call through the abandoned call's result — the
  regression fails on the Pi tree writer, the Claude tree writer, the Codex line walk and the pipeline's
  archive read, in both spellings of the folded outcome), or always under the message's own identity
  (measured wrong: the standalone result record's descendants then answer the result's own id, which no
  record in the read names, and an intervening-branch record is chained to the call instead of through the
  result — the source-shape cases fail); (b) keep adjacency uncounted (measured wrong: the two source trees
  above convert identically and report zero uncertainty, so the conversion presents its own reading as the
  read's answer).
- **Settled by:** nothing owner-only; a reader that answers a record's own parent beside the projected one
  (Q-0005-10's own settlement) would make both halves answerable rather than counted.

## Q-0005-14 — Which provider definitions the Pi native check seeds into its scratch home (Q-0002-21's policy, on Pi's own schema)

- **Status:** resolved (default taken; 0005's third and fourth review rounds named the seed's credential
  surface: a nested `models`/`modelOverrides` `headers` field was copied while only the provider's own
  levels were scrubbed, and a selected provider's `baseUrl` could carry URL userinfo or a credential in
  its query, which the field scrubber preserved)
- **Question:** the Pi check seeds the scratch home it makes for itself with the host's provider
  definitions so a real turn can reach a model. Pi resolves a literal secret wherever its schema allows one
  (a provider's `apiKey` and `headers`, a `models` entry's `headers`, a `modelOverrides` entry's
  `headers`), and a credential can also be written into the endpoint URL itself — as userinfo
  (`https://user:token@host`) or as a query (`?token=…`). What may cross into the seeded `models.json`?
- **Default taken:** every credential-bearing *field* is dropped at every level (`scrub_credentials`), the
  chosen provider's key is re-spelled as an `$ENV` reference the host's environment resolves, and a
  selected provider whose endpoint URL is not credential-free (`credential_free_url`: no userinfo, query or
  fragment) is **not copied at all** — the provider is left out whole rather than copied with its address
  rewritten, because that address is the endpoint the harness would call and half a definition would send
  it to Pi's own default; the run's `pi models:` line names it, and a run that seeded nothing because of it
  records the named `SKIP` (`a Pi provider is configured for the chosen model`) rather than failing later
  in a turn. A provider the run does not need is left out by name. The policy and the URL helper are shared
  with the Codex check through `scripts/native_check_common.py` (Q-0002-21 is the Codex-side entry), so the
  two definitions' rules cannot drift.
- **Reversal:** strip only the credential *fields* and copy the endpoint as spelled (measured wrong: a
  second copy of the credential lands in a home `--keep` prints and leaves on disk), or strip the
  credential out of the endpoint (measured wrong: the address is rewritten, sending the harness somewhere
  the host did not name).
- **Settled by:** nothing owner-only.

## Q-0005-15 — An omitted record keeps the place its own walk resolved (Q-0005-07's first consequence, in composition with Q-0005-10's projection)

- **Status:** resolved (default taken; 0005's fifth review round measured that the consequence and the
  projection step did not compose)
- **Question:** Q-0005-07 chains a record whose named parent the target omitted to "the nearest ancestor
  that was written", and Q-0005-10 chains a record whose read-answered parent is a call through the tool
  result standing between them. Where the two meet — a Pi chain `U → C → R → E → A` in which `C` makes a
  call, `R` answers it, `E` is an empty assistant record and `A` names `E` — the omitted record resolved
  *its* place through `R`'s entry at its own walk and was then omitted, and a child re-resolving `E`'s
  **raw** relation reached `C`'s entry instead: the converted path became `U → C → A`, with the written
  result off the walk the target's loader makes although the report counted it as carried. Should an
  omitted record's resolved place be kept, or should a child of it be re-resolved from the raw relation?
- **Default taken:** the place an omitted record's own walk resolved is kept under the record's identity
  (`sourceTree.omittedAncestorEntry`), and a later record naming it follows *that* place (the result's own
  entry in the shape above). A record whose own walk resolved nothing — its recorded parent was read later,
  with nothing yet written to fall back to — keeps no place, and a child of it follows the document's own
  relations to the nearest written ancestor as before. Both tree writers write this through the shared
  tree, and the relation is not counted again on the child: the projection's place was counted once, where
  the omitted record's own walk took it (`unresolved_source_parents`).
- **Reversal:** re-resolve the child from the raw relation (measured wrong in this round: the new
  conversion-level cases fail on both tree writers — the record after the omitted one hangs off the call's
  own entry and the result stands off the model-visible walk), or keep a place for every record including
  the ones that wrote entries (the entry written for the record is already the place its children follow,
  so the map would only duplicate it).
- **Settled by:** nothing owner-only.

## Q-0005-16 — The source's own file answers the Pi-family leaf, and a read with no file says so instead of judging

- **Status:** resolved (default taken; 0005's sixth review round found the read's own last record standing in
  for the entry the source's file selects)
- **Question:** the carried Pi reader skips the tree entries the harness's own loader builds context from
  (`branch_summary`, `custom_message`), so a session switched back to an earlier branch with `/tree` ends
  with a `branch_summary` entry while the read's last record is still the abandoned branch's own last
  message. A target written from that record would resume the branch the source's conversation left, and the
  native load check could not tell: it compares the harness's answer with the *converted* file's own
  history. The source's own file answers the leaf — Pi's loader walks from the file's last entry back through
  `parentId` — but only where the file was read: an archive row is a projection with no file open, and a file
  that moved while it was read is not shown to be the bytes the read covered. Should a read with no file
  standing behind it be refused, or converted with the limit stated?
- **Default taken:** the file's own answer decides where the file stands behind the read (the read came from
  the source, and the file's size is the size the read recorded), and the tree-shaped and line-shaped targets
  share the refusal (`ErrUnrepresentableLeaf`) through the source-side check in
  `internal/ccc/pi_source_leaf.go`. Where no file stands behind the read the leaf is *stated* in the run's
  warnings and the conversion proceeds — the same shape the boundary uses for a source file it could not
  measure (`source_boundary.state: not-taken`) — because the archive projection is a documented route with
  its own recorded evidence (the archive read answers the same relations as the source read, §3 of the
  evidence record), and refusing it would remove a capability rather than a false claim. The refusal's own
  rules are the mutation-pinned ones the evidence record lists: the deepest record the read holds on the
  file's leaf path must be its last record, and every entry on that path the read does not carry must be a
  kind the harness's loader keeps out of its context.
- **Reversal:** refuse an archive-backed Pi-family conversion too (a smaller rule, at the cost of sessions
  whose file is gone: `ccc continue` would report "the source's own loader selects a leaf this conversion
  cannot write" for a session an archive legitimately holds), or convert silently as before (measured wrong
  in this round: the harness's own loader answers the branch fixture's copy with
  `[user, assistant, assistant, branchSummary]` while the read's last record is the abandoned branch's own
  turn).
- **Settled by:** nothing owner-only.

## Q-0005-17 — A Pi system record is an accounted omission, not a refusal (Q-0005-16's second rule refined)

- **Status:** resolved (default taken; 0005's eighth review round found that every session Pi itself has run
  a turn in was unconvertible)
- **Question:** Pi's own format document places a system record at the head of every session that has run a
  request and writes further ones as its prompt and tool loadout changes (`docs/session-format.md`,
  "message", shipped with 1.5.2), and Pi's own loader builds a message from it — so a session Pi itself has
  extended holds an uncarried record of a kind the loader's context holds, and Q-0005-16's second rule
  (every uncarried entry on the leaf path must be a kind the loader keeps out of its context) refused it
  with `ErrUnrepresentableLeaf`. The source direction then worked only for hand-written fixtures: the
  recorded native probe on this host answered exit 1 for a session a real turn had extended, naming the
  system record. Should such a record be refused, carried, or omitted with the omission reported?
- **Default taken:** omitted and *counted*, as `conversion.losses[]`'s `source_system_records`, whose count
  comes from the source file's own entries (the differentiator the refusal was built on,
  `piAccountedSourceOmission`): the record is the harness's own prompt and tool-loadout state rather than a
  turn, the inherited reader does not carry the role (and a second parser in this tree is what the design forbids),
  and no target has that state to hold — a target harness resumes under its own prompt, and Pi rebuilds a
  session's prompt from its configuration when a session records no system message at all. The refusal
  stays for every *other* uncarried context record (`custom_message`, `branch_summary`, an unknown kind or
  an unknown message role), so the reader's own limits are still refused rather than disclosed. The count
  is reported where a source file stood behind the read: an archive-backed read cannot count the records
  (the row holds the read's own answer, which never carried them), and that is stated in the run's warnings
  through Q-0005-16's existing leaf statement.
- **Reversal:** refuse the shape and disclose the restriction instead (the smaller rule the eighth round's
  finding allowed; measured to remove the capability for every native Pi session, which is the route's
  point), or carry the record by re-reading the file into conversation records in this tree (forbidden: a
  second source parser), or leave the omission uncounted and rely on the loss table's other categories
  (would present a dropped record as a carried conversation).
- **Settled by:** nothing owner-only.

## Q-0006-01 — The Oh My Pi title slot is written empty and counted

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** every session Oh My Pi writes begins with a fixed-width first line — measured on 18.18.7, a
  256-byte `{"type":"title",…}` record including its line feed, which the harness rewrites in place when it
  titles a session, and `SESSION_TITLE_SLOT_BYTES` in its own bundle names the width. A conversion has no
  title of the source's to carry: OMP's title is the harness's own generated session name, and the source
  (a Claude Code transcript, a Codex rollout, a Pi session) holds no such field — its first prompt is not
  the harness's title. Should the converted file carry a title slot at all, and if so with what in it?
- **Default taken:** written, with an empty title, and counted as the `title_slot_written` caveat in
  `conversion.losses[]`. The slot is the shape the harness's own writer starts from (a file without one
  still loads, measured, but one the harness extends keeps the width it found), and an empty title is the
  one value that claims nothing about the source: writing the source's first prompt would present it as
  the harness's own generated name.
- **Reversal:** omit the line and rely on the harness's own migration of a slot-less file (measured to
  load), or write a title derived from the source (a claim the read cannot support).
- **Settled by:** nothing owner-only.

## Q-0006-02 — Every converted assistant record states the target home's own provider beside the source's model

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** OMP's request builder reads a `model` **and** a `provider` off every assistant record, and
  measured on 18.18.7, dropping either from one record of an otherwise loadable session makes the next turn
  fail while building its request (`undefined is not an object (evaluating 'e.length')`) — while a foreign
  provider label is inert once the session's own binding resolves. The inherited read answers the model each
  source message recorded (and no endpoint: the session-level provider names the harness that wrote the
  words). What does a converted record state in that field?
- **Default taken:** the *target home's* own configured provider, asked of the harness itself, on every
  converted assistant record, with the source's model beside it and the pair counted as the
  `message_model_attribution` caveat. This is the same policy the Codex route applies to its provider
  (Q-0002-21's line: the target's own configured endpoint where the harness was asked), extended to the
  per-record field OMP requires; the report states which answer it carries
  (`target_model_binding`'s detail names the harness's own answer, or the reason there is none — a dry
  run, a harness that is missing, or a harness that answered nothing).
- **Reversal:** write no provider and let the next turn fail (measured), or write the source's provider
  label (a source-side name the target home need not define, and false as the endpoint that served the
  record).
- **Settled by:** nothing owner-only.

## Q-0006-03 — The converted session's model binding is the harness's own answer, not the source's

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** OMP restores a session's model from its `model_change` record, whose `model` is one
  `<provider>/<id>` string that must resolve in the home, else from the session's own recorded message
  pairs — and a pair the home cannot resolve is refused (`SessionModelRestoreError: Could not restore model
  <provider>/<id>`, exit 1). A converted session that records the source's model therefore opens only in a
  home that defines that exact provider and model. What should the binding be?
- **Default taken:** the target home's own configured `<provider>/<model>`, asked of the harness (`ompFacts`
  → `OMPModelProbe`, the same seam the Codex route asks its provider through), written as the
  `model_change` record the first source record hangs off; omitted when the harness answered none. The
  source's own model travels on each assistant record instead (Q-0006-02), so the history still says which
  model wrote it. This differs from both siblings deliberately: Codex records its provider and no model,
  Pi binds nothing at all.
- **Reversal:** bind the source's model and refuse the route wherever the home does not define it (measured
  to make the route unusable for the common case), or bind nothing and accept the harness's own default
  (a session whose records name a model it then does not use, and no disclosure of the substitution).
- **Settled by:** nothing owner-only.

## Q-0006-04 — A recorded working directory this host does not have is answered by the load check, not by refusing the conversion

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** OMP resolves a session id from any process's working directory (it scans its own sessions
  tree by id), but measured on 18.18.7 it refuses one whose *recorded* working directory it cannot find
  (`Error: Session "<id>" belongs to a directory that no longer exists (<dir>); run interactively to move
  it into the current project.`) — and its format holds one directory field, the one its loader resolves
  the session by (taken from the file, not from a project-directory spelling), so unlike the Codex route,
  whose `session_meta` carries a directory no target lookup needs, there is no second field a conversion
  could fill. A source read whose
  recorded working directory does not exist on the machine running the conversion is therefore a session
  this host can file but this host's harness will not open. Should the conversion refuse it?
- **Default taken:** the conversion is written and installed — the recorded directory is the *source's* own
  fact and is written unchanged (its spelling is OMP's own rule for that path) — and the claim stops at the
  load verification: `--verify load` (the default) reports `not-verified` with the harness's own refusal
  sentence and the run exits non-zero, so no run reports such a session as natively resumable. Measured on
  this host: a Claude Code fixture whose recorded directory is `/home/user/code/example-api` converts,
  installs under `sessions/--home-user-code-example-api--/` (the one branch of OMP's spelling that does not
  depend on the machine), and the load line names that directory and the harness's sentence. This is what
  every sibling route does with a host-specific limitation — the check is where the claim stops — and a host
  that has the directory verifies the same file.
- **Reversal:** refuse the conversion outright when this host lacks the recorded directory (a conversion
  that would verify on the operator's own machine is refused because a *check* host cannot drive it), or
  rewrite the recorded directory to the caller's own (writing a false record of where the conversation
  happened, and one the harness's own `--resume` would then report as moved).
- **Settled by:** nothing owner-only.

## Q-0006-05 — The printed resume line names the sessions tree inside the data directory

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** OMP resolves a session's id through `PI_CODING_AGENT_SESSION_DIR` when it is set, and
  measured on 18.18.7 that variable names the directory that holds session **files directly**: the
  session's own project directory under `<home>/sessions` resolves the id, while the `sessions` root
  above it answers `Error: Session "<id>" not found.`, and a caller's leftover value (a Pi user's
  `settings.json`/environment, most obviously) sends the lookup to a tree this session is not in. The
  data directory alone resolves an id too (OMP scans its own sessions tree by id), so does the printed
  line have to name the sessions tree as well, and which spelling?
- **Default taken:** both halves are named explicitly — `PI_CODING_AGENT_DIR` and
  `PI_CODING_AGENT_SESSION_DIR=<home>/sessions/<the project directory the installer filed the session
  under>` — by the printed `resume`/`non-interactive resume` lines and by both checks that drive the
  route (the Go resume check and the native check's printed-invocation case). The directory named is the
  one that holds the file directly, not the `sessions` root above it: measured, the root answers "not
  found", and naming the right tree is also what keeps a caller's leftover setting from hiding the
  session. This is the same shape the Pi route's printed line takes (its `PI_CODING_AGENT_SESSION_DIR`
  names the sessions tree too, `docs/conventions.md`'s install rule), and the reason the evidence
  record's §0 table carries the measurement.
- **Reversal:** name the data directory alone and rely on OMP's own scan (measured to resolve an id, but
  a caller's leftover `PI_CODING_AGENT_SESSION_DIR` then overrides the lookup), or name the `sessions`
  root (measured to answer that no such session is installed).
- **Settled by:** nothing owner-only.

## Q-0006-06 — The alias directories OMP's own writer links beside the canonical one are not written

- **Status:** resolved (default taken; 0006's implementation)
- **Question:** an OMP home's project directory is not the only name OMP's own writer leaves behind for a
  session — it also links alias directories beside the canonical one (`shadowedHomeDirName`,
  `hashedDirName`, for a working directory whose name is ambiguous or long). Should a conversion create
  them too, so every name the harness's own writer produces exists?
- **Default taken:** not written. The aliases are lookup aliases for a directory the harness itself
  derived, and the canonical directory is what the harness's own lookup reads: measured on 18.18.7, a
  session filed in the canonical project directory resolves by id with no alias present, and the install
  is exclusive — a conversion creates exactly one file and invents no second directory it cannot attest
  (`OMPProjectDir` computes one spelling; `shadowedHomeDirName`/`hashedDirName` are not part of the
  measured rule). The path a conversion reports is therefore the file's own directory, and the report's
  install line names the mechanism.
- **Reversal:** write the aliases too, if a harness version is measured to need one of them for a
  session's own lookup (the file would then be linked under names this build derived rather than
  measured).
- **Settled by:** nothing owner-only.

## Q-0007-01 — The OpenCode installation is the harness's own importer, so a missing binary is a boundary

- **Status:** resolved (default taken; 0007's implementation)
- **Question:** an OpenCode session is a row in the harness's own SQLite store, and the harness ships an
  importer (`opencode session import`). Should a host without that binary still get an installation — by
  writing the store's rows from this build, or by writing a second store the harness might read?
- **Default taken:** no. The install runs the harness's own importer with the document staged outside the
  home (a temporary file removed once the importer has it) and reports installed only when the harness
  answers `Imported session: <id>` for this conversion's id; a host whose binary is absent fails the run
  with the boundary named (`… is not on PATH, and this target's installation is the harness's own
  importer: …`), and the native check ends `RESULT: INCOMPLETE` (exit 2) before any check. Writing the
  store's rows here would be a second writer of a private store — exactly what the goal's Bounds refuse —
  and a store this build invented is one no harness was shown to read. An `already exists` answer (which
  the harness prints on stderr with exit 0) is a failure rather than a success, because this run did not
  install that session.
- **Reversal:** teach the installer a store writer if a harness version ships no import path at all (then
  the check would have to show the harness reading what this build wrote, and the report would say which
  writer ran).
- **Settled by:** an OpenCode release that documents a stable on-disk session format, or an operator
  decision that a missing-binary run should install anyway.

## Q-0007-02 — The data root the report names is the harness's own, taken from XDG_DATA_HOME

- **Status:** resolved (default taken; 0007's implementation)
- **Question:** `--target-home DIR` on every other route names the directory the harness keeps its data
  in. OpenCode has no variable that names its own directory: it reads `$XDG_DATA_HOME/opencode`. Should
  `--target-home` name the data root (`…/opencode`'s parent) or the data directory itself?
- **Default taken:** both spellings are accepted and resolved to the same directory. `ResolveOpenCodeHome`
  treats a value whose base name is `opencode` as the data directory itself (and names its parent as the
  data root) and any other value as the data root; the report, the `XDG_DATA_HOME` every command this
  route runs is given, and the store path all agree; `OPENCODE_DB` is removed from the child environment
  (it would point the harness at another store), and the printed resume line and the resume check pin this
  home's own store in it instead -- a shell line can set a variable but cannot remove one, so a caller's
  inherited override is answered by naming the database beside the root (measured: `debug paths db`
  answers the pinned file, `session export` resolves the session there, and the same export against
  another store refuses it, where `run --session <id>` would *create* the session) -- and a caller's own
  `XDG_DATA_HOME` is the fallback when no flag is given. A demo therefore cannot silently split the store
  the report names from the store the harness reads.
- **Reversal:** accept only one spelling (the data root), which would make a caller who names
  `…/opencode` install into a sibling directory.
- **Settled by:** nothing owner-only.

## Q-0007-03 — The load level asks `session export`, never `run`

- **Status:** resolved (default taken; 0007's implementation)
- **Question:** OpenCode's non-interactive turn command (`run --session <id>`) is the command a caller
  resumes with. Can the load level use it, as a harness whose loader reads a file might?
- **Default taken:** no. Measured on 2.2.5: `run --session <id>` **creates** a session for an id the store
  does not hold and appends its own records even when the turn fails, so a `run`-based load would answer
  about a session it had just made rather than about the installed one. The load level therefore asks
  `session export <id> --standalone` (the harness's own reader, content-neutral for the session's own
  records — measured: identical rows and an identical document around it) with an unknown-id control arm
  that must refuse naming that id, and the answer is accepted only when it names the installed session's
  own id. The real turn belongs to the resume level, which runs it explicitly
  (`--verify resume --resume-prompt TEXT`) and measures the growth through the harness's own reader.
- **Reversal:** use `run` for the load level, at the cost of the check being about a session the check
  itself created.
- **Settled by:** a harness version whose `run` refuses an unknown id.

## Q-0007-04 — An assistant record's completion instant is written from the record's own instant

- **Status:** resolved (default taken; 0007's implementation)
- **Question:** the harness's importer requires an assistant message's `time.completed` — measured: a
  message whose `time` carries `created` alone is accepted and then **silently dropped** (no row, no
  export entry; every other member was bisected). The source read answers one instant per record, not a
  span. What should the member hold?
- **Default taken:** the record's own instant, so the message reads as a zero-length turn; the choice is
  disclosed on every run as `assistant_completion_instant ×<n>` (carried with caveat) with the
  measurement in its detail. The alternatives were a span derived from the *next* record's instant (an
  interval the source never recorded) or leaving the member out (the harness would drop the message, and
  the dialogue and tool evidence with it).
- **Reversal:** write the next record's instant as the completion if a future harness version is measured
  to need a positive span (the report's detail sentence would then say which instant it wrote).
- **Settled by:** an OpenCode release whose importer keeps a message without `time.completed`, or one that
  documents what it does with the member.

## Q-0007-05 — The structural comparison canonicalizes a tool part's input JSON

- **Status:** resolved (default taken; 0007's implementation)
- **Question:** the inherited reader renders a tool call's input with Go's own JSON encoding (compact,
  keys sorted), while the document a conversion writes carries the source's own spelling
  (`{"file_path": "…"}`). Should the read-back comparison treat the two spellings as a difference?
- **Default taken:** no, for that one member. Measured on 2.2.5: the importer stores `state.input` as a
  parsed value and re-encodes it, so the store's own row holds the compact spelling and the document's
  whitespace is not evidence the store keeps. The comparison therefore canonicalizes the *intended* side's
  input JSON through `canonicalJSONValue` (a value that does not parse is left exactly as recorded, so the
  walk reports it) while every other member — dialogue, call id and name, the result's text, the boundary
  summary — is compared as recorded, and a changed value (or a changed tool name) is still a difference.
- **Reversal:** compare the raw spelling, which would report every real conversion as a difference over a
  member the harness itself normalizes.
- **Settled by:** a harness version that stores the input's original spelling.

## Q-0007-06 — A failed importer process is reconciled through the harness's own reader

- **Status:** resolved (default taken; 0007's review round 7)
- **Question:** this route's installation is another process's write (Q-0007-01), and — measured on 2.2.5
  — the harness's own importer commits the session and then exits, so a deadline or a cancellation between
  those two moments leaves the store holding the session while this run observed only a process error.
  Should a failed process answer "installed?" with the only thing the run saw — a failure — and report
  nothing published?
- **Default taken:** no, and three answers are kept apart. The harness's own `Imported session: <id>` line
  in the captured output is its answer that the store holds this session; where the process failed without
  acknowledging anything, the harness's own reader is asked (`session export <id> --standalone`, the
  interface Q-0007-03 already reads this store through, measured to change none of a session's own
  records); and where neither answers — an answered refusal being its own answer that the id does not
  resolve — the run reports Q-0007-07's third state. The question is asked with a context that **outlives
  the caller's own deadline** (`context.WithoutCancel` plus the route's own question deadline): the
  deadline that stopped the import is exactly what would make the question unable to answer, and asking it
  is a bounded, read-only act, where not asking it is what would leave the report asserting an absence
  nobody observed. A published answer keeps the installer's own convention — the path beside the error
  (`continue.go`) — so the report still carries the identity the session is resumed by.
- **Reversal:** read every process failure as "nothing was published". The cost is the one the goal's own
  clause names: a caller who acts on that line and converts the same conversation again leaves a second
  snapshot of one conversation in the store, with nothing recording either attempt.
- **Settled by:** a harness version whose importer answers only after committing (then the process error
  itself would be the absence), or an operator who prefers the cheapest report over a reconciled one.

## Q-0007-07 — The installation block has a third state, `unresolved`

- **Status:** resolved (default taken; 0007's review round 7; amends Q-0002-15's two-state vocabulary and
  its `(proposed; nothing was installed)` marker)
- **Question:** `installation.state` has carried `installed`/`not-installed` since 0002 (Q-0002-15), and the
  human block renders everything that is not `installed` as `not-installed (nothing was published under
  <home>)` with the identity lines marked `(proposed; nothing was installed)`. A run whose installer cannot
  establish whether it published anything (Q-0007-06's unanswered case) has neither answer. Should the
  third answer be a wire value of its own, or should such a run borrow `not-installed`'s words?
- **Default taken:** a wire value of its own: `installation.state` is `unresolved`, the human block prints
  `installed:    unresolved (whether the store under <home> holds this session could not be established)`,
  and the three identity lines are marked `(proposed; installation unresolved)`. The other fields are
  unchanged, the run still exits failure and still carries the failure in its error, detail and warnings,
  and an installer that handed back a published path reports `installed` however its process ended — the
  path is the answer. Borrowing `not-installed`'s words would put an absence nobody observed in the one
  line a reader scans for that question.
- **Reversal:** fold the state into `not-installed` and carry the difference in `installation.detail`
  alone: the same information for a reader of the detail, at the cost of the one line a reader scans for
  "is it installed?" answering with a claim the run does not have.
- **Settled by:** an operator who prefers the two-state vocabulary for a consumer already reading it, or a
  harness version that cannot leave this question unanswered.

## Q-0007-08 — A reader's rendering of a record's own trace is removed from that record's text

- **Status:** resolved (default taken; 0007's review round 8; a rule of the shared `dialogueText` every
  target writer reads its text through, not of this route alone)
- **Question:** the readers that render a persisted reasoning trace into `Content` instead of projecting a
  dialogue (ZCode's thinking parts, Cline's reasoning records, Copilot's reasoning blocks and the generic
  block reader) write it as `[Thinking]…[/Thinking]` *beside* the record's own text; only the shape where
  the rendering is the whole `Content` was recognised (the round-7 rule, `thinkingRenderingOnly`). A record
  carrying both therefore travelled with its trace twice — once as the record's own reasoning and once as
  words the reader rendered into the dialogue. Should such a record's `Content` be carried whole (the
  duplication), be dropped whole and counted, or have the rendering removed from it?
- **Default taken:** the rendering is removed from the record's text, and only where the record's own trace
  stands behind it: the block's own words must be part of the message's trace (so a record the read
  answered no trace for, a block no trace of that record holds — a model writing the markers itself —, a
  marker with no closer and an empty block keep their `Content` byte for byte), the removal happens where
  the block stands (a reader that keeps the record's part order may render the trace *after* the text it
  followed), and the line feeds that joined the rendering to its neighbours go with it. The alternative
  arms each lose something this one keeps: carrying the `Content` whole presents the trace as dialogue the
  source never recorded the agent as saying, and dropping it whole discards the record's own words, which
  nothing shows to be reasoning. (Round 10 narrowed it to the reader the words test cannot see: the removal
  is authorised only for a record whose *reader* renders the trace into `Content` —
  `Message.ContentRendersThinking`, established from the session's agent label by
  `internal/ccc/dialogue_provenance.go`'s `thinkingRenderingReaders`, whose thirteen entries each name their
  reader's construction site in the carried tree. A reader that records `Content` as the source wrote it —
  Vibe's independent `content` and `reasoning_content` among them — keeps its `Content` byte for byte *even
  where its words are the record's trace*, the coincidence the round's finding measured: a reader the table
  does not name is treated as a non-renderer, and the table is maintained with the carried readers. Round
  12 replaced the words test's own criterion with the readings: a block is removed only where *every*
  reading of the record's own bytes against its own trace takes it — an assignment of some of `Content`'s
  marker blocks, in order, to the trace's parts, whose words open the trace, close it and account for
  everything between them — because the feeds around a marker pair establish no part boundary (a part of
  the record's own text may carry feeds of its own around a pair whose words the trace holds, as a
  substring of it as readily as the whole); where the readings disagree about the dialogue, every block
  they disagree about stays in the dialogue as text the record carries, and the whole-content shape — a
  record whose `Content` is nothing but a rendering — is the one reading the readers' own rule states: the
  block's words must *be* the trace, whole.)
- **Reversal:** make the writer refuse such a record (or drop its `Content` and count it under a loss
  category), which is stricter about provenance and loses the record's own text with it.
- **Settled by:** a reader that renders a trace into `Content` in a shape where the block's own words are
  not recoverable from the message's trace field, or an operator who prefers the strict arm.

## Q-0007-09 — The native check removes, in a caller's store, only what it shows to be its own

- **Status:** resolved (default taken; 0007's review round 8)
- **Question:** `--target-home` points the OpenCode native check at a store the caller owns, and the check
  plants a session of its own in it (a fixed id, a fabricated document) to measure that "nothing else
  moved". Measured failure: an earlier run whose *planting import* committed and whose process then failed
  or overran its deadline left that fixture behind, so every later run met `Session already exists` and
  ended `RESULT: FAIL` — with the fixture still there. May the check remove what blocks it?
- **Default taken:** only what the store's own document shows to be that check's own fixture, and only
  after asking. The id the check plants is read through the harness's own reader first (the store's own
  `Session not found: <id>` refusal, naming that id, is the only answer that makes it free — a store that
  could not answer is not read as empty); a session the store holds whose *whole* document is the check's
  own fixture — compared member for member, with only the two members the harness's own importer is
  measured to rewrite (`info.projectID`, recomputed from the session's recorded directory, and
  `info.time.updated`, the import's own instant) normalized away — is an earlier run's leftover and is
  removed through the harness's own `session delete` (and shown gone through its own reader); a session
  holding anything else is left exactly as found, with the run recording the refusal and stopping. (Round 9
  narrowed the recognition to the whole document: the earlier rule — the id, the recorded directory and a
  matching message text — authorised deleting a leftover a caller had since continued, and the measured
  pre-fix run deleted one, while the fixed one leaves that session resolving with the caller's own turn and
  reports the refusal.) The planting itself is reconciled the way the route reconciles its own import —
  the harness's own acknowledgment line, kept
  even when the import overran its deadline, then the harness's own reader — so an import whose process
  failed after committing is still a publication this run removes again.
- **Reversal:** refuse to touch a store that already holds the id, leaving the operator to remove the
  leftover by hand (the state the measured failure produced, and the reason this default was taken).
- **Settled by:** an operator who prefers a check that never deletes in a named store, or a harness whose
  export cannot show which session a store holds.

## Q-0002-28 — Which obligation ids a citation manifest cannot hold

- **Status:** resolved (default taken; the harness increment — *a machine-checked obligation-citation
  manifest*, 2026-11-18 — which ships `docs/evidence/0002-citations.tsv` as the mechanism's seed)
- **Question:** the manifest's citation form is one row per rule id, verified by running that row's Go
  test in the package it names (`docs/conventions.md`; the `citations` gate step in `docs/harness.md`).
  Of the matrix's forty rule ids, four have no such test. D4 (the source tree and every pre-existing
  session compared *after* the native stages) and F1 (no credential-bearing setting copied into the
  throw-away home) take their own positive evidence from the **native** check
  `scripts/codex-native-check.py` — a script that is deliberately not a gate step — and are re-derived in
  the gate by the python cases `ProtectedArtifactsAfterNativeVerification` and `SeededHostConfiguration`
  in `scripts/tests/test_codex_native_check.py`, which the `selftest` step runs; F4 (the harness's cases
  hold on every host) and F5 (a required gate step cannot pass by not running) are rules of the harness
  itself, pinned by cases in the same suite. Which ids does the file leave out, and on what grounds?
- **Default taken:** all four are left out for the one form-based reason this file's citation form has
  for any id: a row is one Go test function in a package `scripts/gate-targets.txt` covers, and each of
  the four is re-derived by the gate through a python case under `scripts/tests/` that the `selftest`
  step runs — `ProtectedArtifactsAfterNativeVerification` and `SeededHostConfiguration` for D4/F1 (their
  native run stays their own positive evidence), the suite's own cases for F4/F5. All four ids are named
  in the manifest's own header comment, one line per reason, so nothing is dropped silently; a new
  obligation of either class joins that list the same way.
- **Reversal:** extend the row form with a second citation kind (a python case under `scripts/tests/`,
  verified through the `selftest` step) — the schema, the checker and the step then carry two kinds of
  citation — or leave every id whose re-derivation is a python case to `selftest` alone, which is the
  present state.
- **Settled by:** nothing owner-only; the owner may ask for the python-case row form when a goal's
  acceptance surface carries enough harness rules to justify it.

## Q-0007-10 — A record's dialogue the read cannot establish is not guessed at (Q-0007-08's mechanism superseded)

- **Status:** resolved (the owner's decision of 2026-11-19, letter A: the root fix for this face lands
  inside 0007, on the third consult's answer — root cause, what fixing the root means, and how to tell the
  root is fixed)
- **Question:** the carried readers write a record's persisted reasoning trace into `Content` verbatim
  between their own markers, so two different native part orders — a text part followed by a thinking
  part, and a text part that quotes the block the thinking part renders — produce the same `Content`, the
  same trace and the same reader label, and no inverse can say which occurrence was the model's words
  (`internal/parser/zcode.go` escapes nothing, so even the first closer need not be the reader's own; the
  consult's arm 2 is the collision with no trimming in the shape at all). Three review rounds (13, 14, 15)
  each found a deletion defect inside the readings machinery that inferred a removal from those bytes, and
  two cross-family consults each improved the inverse rather than removing the dependency on it. What
  happens to a record whose reader renders the trace into `Content` and whose read answered no dialogue
  projection — keep the removal, carry `Content` whole, drop `Content` whole and count it, or refuse the
  conversion?
- **Default taken:** **no removal by inference survives.** `dialogueOf`
  (`internal/ccc/dialogue_provenance.go`) answers the source's own projection (byte for byte, an
  explicitly empty one included) or one of three states — the record's own bytes where the reader is not
  one `thinkingRenderingReaders` names and the record carries no calls (carried, counted as
  `unprojected_dialogue`); no dialogue where `Content` is byte for byte the reader's rendering of the
  record's whole trace; nothing established where the reader renders the trace and the record carries one,
  or the record carries calls (no text written, counted as `unresolved_dialogue`, tool evidence still
  written, the source session keeping the bytes). The readings machinery, its candidate scan, its
  first-closer pairing and `thinkingRenderingOnly` are deleted rather than repaired, and the list is
  completed against the carried readers (the eight non-projecting renderers that were missing, and the
  projecting renderers, whose entry covers a record without a projection such as a legacy archive row).
  Partial conversion, not refusal, is the disposition: the loss is per record and disclosed, and refusing
  a conversation over one record's text would deny every record the source did answer for. Every run's
  report names both categories with their counts and dispositions.
- **Reversal:** restore a removal for a reader whose own construction proves the text — the durable route
  is upstream (a reader that projects its dialogue, docs/upstream.md's procedure), not a local recogniser
  — or take the stricter arm: refuse the records whose dialogue cannot be established instead of omitting
  their text and converting the rest.
- **Settled by:** an upstream projection for the readers that render a trace and project none (then their
  records convert exactly and both categories fall to zero), or an operator who prefers the strict arm.

## Q-0007-11 — OpenCode counts a record's dialogue state after its role gate (a count moves on one route)

- **Status:** resolved (the simplify round's accepted PD-2, applied by the unattended increment)
- **Question:** the dialogue categories (`unprojected_dialogue`, `unresolved_dialogue`) describe what
  happened to a record's *dialogue*. OpenCode asked `dialogueOf` and counted those categories for every
  record, before its role gate, and then discarded the text of a record whose role is neither the user's
  nor the assistant's under that role's own category — so a `tool`-role record's `Content` was reported as
  carried-with-caveat text ("the text written for them is the reader's own Content for the record, byte for
  byte") although no document and no report item held it, and one source record was counted differently by
  this route than by Codex, Claude and Pi. Count before the gate (one route's current answer, with the
  category's own sentence then false for it) or after it (the other three routes' answer, and a count that
  moves on one route)?
- **Default taken:** **after the gate.** `openCodeConverter.writeMessage` returns a record of any other
  role under that role's own category (still writing its tool evidence) before `dialogueOf` is asked, so a
  dialogue category is only ever counted for a record the writer went on to consider as dialogue. Measured
  on one hand-built document (a projected user record, a `tool`-role record whose `Content` is text, a
  plain assistant record): before, codex/claude/pi reported `unprojected_dialogue=1` and OpenCode `2`;
  after, all four report `tool_role_messages=1` and `unprojected_dialogue=1`. This is the round's one
  behavioural change — a count, for documents holding a non-dialogue role's text, that no run recorded in
  the increment's evidence ever stated — and the evidence record names it as such
  (`docs/evidence/0007-opencode-target-continuation.md`, the simplify round's section, PD-2). The round's
  other two defects are not behaviour changes: PD-1 is a measured restoration of the categories `fa8da89`
  reported (per route, both roles), and PD-3 is two report clauses corrected.
- **Reversal:** count the dialogue categories before the role gate again — nothing in a converted document
  depends on the count, so reversing it is one statement's position in `writeMessage`.
- **Settled by:** nothing owner-only; an operator who wants a non-dialogue role's `Content` counted under
  the dialogue categories (with those categories' sentences restated so they are true there, and the other
  three routes taught the same) may ask for that split instead.

## Q-0007-12 — A rendering reader whose read records no trace for the record: carry `Content` counted, or ask the read's own flag and omit

- **Status:** resolved (**owner, 2026-11-19 — letter A**, the wording arm; applied by the unattended increment as
  round 17's STD-A01, arm (a). The mechanism arm is **deferred to goal 0008**, where OpenCode as a *source*
  makes it live)
- **Question:** `dialogueOf`'s rendering arms ask the reader's name **and** a non-blank `Message.Thinking`
  (`ContentRendersThinking && TrimSpace(Thinking) != ""`). A reader that renders the trace into `Content`
  without the read recording the trace's own text — `Message.Thinking` empty for every record it produces
  — therefore never reaches `dialogueUnestablished`: its records land in `dialogueFromContent`, the text
  (the rendered block inside it) travels, and the conversion counts them `unprojected_dialogue`. That is
  every record of the eight block-writing readers this increment added to `thinkingRenderingReaders`
  (ChatGPT, Claude.ai, Copilot, Gemini, Hermes, OpenCode, Reasonix, Zencoder — none of their nine carried
  reader files sets `ThinkingText`, re-derived by grep), measured on the shape they produce:
  `dialogueOf` answers the record's-own-bytes state and the Codex document holds the rendering's own text
  (`secret trace`), with `unresolved_dialogue = 0`. Two arms: carry `Content` and count it, so the entries
  cannot act for those records and the report says so (the shipped arm); or carry the read's own existence
  flag — `parser.ParsedMessage.HasThinking`, not carried as a `Message` field today — and treat (named
  reader ∧ a trace was recorded as existing ∧ no trace text) as `dialogueUnestablished`, writing nothing
  and counting the record, the partial conversion the three states already define.
- **Default taken:** the shipped arm. Where the read answered no trace, the listing cannot act, and the
  record's text (a rendering among it) travels and is counted — the condition every statement of the rule
  now carries (AGENTS.md's disclosed-outcome rule (1), the `dialogueFromContent` state doc, the
  `unprojected_dialogue` row and this increment's evidence record), so the report discloses the ambiguity
  per run instead of claiming an exact answer for those readers. The mechanism arm cannot be a blanket
  rule without per-reader verification: `internal/parser`'s Goose reader sets `HasThinking` from
  `redactedThinking` with no block and no text, so "a trace was recorded as existing but its text is
  empty" is not by itself evidence that `Content` holds a rendering, and the blanket form would omit real
  text. Only a reader-by-reader answer (the list split by which readers render without recording, or the
  flag carrying that distinction) can say which readers the arm may act for.
- **Reversal:** carry `ParsedMessage.HasThinking` onto `Message` and ask the rendering arm as (reader
  named ∧ `HasThinking` ∧ `Thinking == ""`), after verifying per reader that its empty-`HasThinking` shape
  is the rendering one — or keep the shipped arm and revisit when an upstream reader records the trace's
  own text (the boundary Q-0007-10's "settled by" names). Either reversal is one clause in `dialogueOf`.
- **Settled by:** the owner's scope call — whether eight readers' renderings being carried and counted
  (the shipped arm, disclosed per run) is acceptable for this increment, with the mechanism arm a separate
  change verified reader by reader — or an upstream reader that records the trace's own text.
- **Resolution:** (owner, 2026-11-19) option (**A**) confirmed on the operator's re-sent form — 0007 states the
  condition the code implements and counts what it carries; the mechanism arm (carry `ParsedMessage.HasThinking`
  onto `Message`; named reader ∧ a trace was recorded as existing ∧ no trace text → `dialogueUnestablished`,
  verified reader by reader) is **not** authorized by this letter and belongs to 0008. Options (B) (do it inside
  0007) and (C) (a new upstream pin first) were offered in the same form and not taken.

## Q-0008-01 — The dialogue arm for a reader that renders a trace without recording it (Q-0007-12's deferred mechanism arm)

- **Status:** resolved (unattended default taken; Q-0007-12's status names the mechanism arm as deferred
  to goal 0008)
- **Question:** Q-0007-12's resolution deferred its *mechanism arm* to goal 0008: carry
  `parser.ParsedMessage.HasThinking` onto `Message` and treat (named rendering reader ∧ a trace was recorded
  as existing ∧ no trace text) as `dialogueUnestablished` — writing nothing for the record and counting it
  `unresolved_dialogue` — verified reader by reader. Goal 0008 is the increment where that becomes live,
  because OpenCode is read as a *source*: the OpenCode v2 reader renders a `reasoning` part into the
  record's `Content` and records no trace text, so every one of its records carries a rendering.
- **Default taken:** the shipped arm, unchanged for this increment. The rendering travels as the record's
  text and is counted `unprojected_dialogue` (carried-with-caveat) on both target routes — measured on the
  committed fixture: 4 of 4 carried messages, and the target's own records hold the rendering byte for byte.
  Two reasons, one of them measured: the mechanism arm cannot be a blanket rule (Q-0007-12's own finding:
  `internal/parser`'s Goose reader sets `HasThinking` from `redactedThinking` with no block and no text, so
  "a trace was recorded as existing but its text is empty" is not evidence that `Content` holds a rendering),
  and for this route it would *omit* the record's own words — the answer text a reader of a resumed session
  needs — where the shipped arm carries them with the caveat disclosed per run. Goal 0008's own clauses ask
  that available content survive where supported and that unsupported content be reported rather than
  silently omitted; the shipped arm does both, and the report names the state on every run.
- **Reversal:** carry `HasThinking` onto `Message` and ask the rendering arm as (reader named ∧ trace
  recorded as existing ∧ `Thinking == ""`), after verifying per reader that its empty-`HasThinking` shape is
  the rendering one — one clause in `dialogueOf` plus the per-reader evidence Q-0007-12 requires.
- **Settled by:** an owner letter authorizing the mechanism arm (with the per-reader verification), or an
  upstream reader change that records the trace's own text for these readers.

## Q-0008-02 — The native direction verifies Codex, and the Claude half rests on the accepted route

- **Status:** resolved (unattended default taken)
- **Question:** goal 0008's first clause allows a continued OpenCode session to be verified against **Claude
  Code or Codex**. Which target does the native direction drive?
- **Default taken:** Codex (`scripts/opencode-native-check.py --direction opencode-to-codex`). Measured on
  this host: Codex 0.170.0 is installed and its own loader (`codex app-server`'s `thread/resume`) opens the
  converted session offline — an actual target-native **load** result — while no Claude Code binary exists
  here at all (a direction that drove it would end `RESULT: INCOMPLETE` on every run of this host). The
  Claude half is covered where it already is: both targets are exercised offline by the Go cases (conversion,
  installation, the inherited reader's structural read-back over the installed transcript), and 0003's
  accepted native check drives Claude Code's own loader on a host that has the binary.
- **Reversal:** add the symmetric `opencode-to-claude` direction (the pipeline is identical; only the target
  binary and the printed line differ) when a host has Claude Code installed.
- **Settled by:** the operator's own host inventory or an instruction to prefer the Claude direction.

## Q-0008-03 — The OpenCode fixture is the harness's own export document, not a store

- **Status:** resolved (unattended default taken)
- **Question:** an OpenCode session is a row in the harness's SQLite store, so what does a committed fixture
  look like?
- **Default taken:** the harness's own **export document**
  (`internal/ccc/testdata/fixtures/opencode/<session>.json`, written by `internal/ccc/fixturegen` from the
  shape a real `session export` writes on 2.2.5, one canonical compact encoding). A committed *store* is not
  an option — only the harness writes one — and the document is the harness's own interchange format: the
  Go cases and `cmd/ccc`'s route cases materialise it through `internal/ccc/opencodefixture`, and the native
  check plants it through the harness's own `session import`, which also validates the shape against the
  real importer on every native run (measured: the import accepts it, and the store's own reader returns the
  document member for member apart from `info.projectID`, `info.location.directory` and
  `info.time.updated`).
- **Reversal:** none needed; the fixture is one file and its consumers are two constructors.
- **Settled by:** nothing pending; recorded because it changes the rule `docs/conventions.md` stated when
  OpenCode was a target only ("no fixture directory of its own").

## Q-0008-04 — No parity/oracle case is added for the OpenCode source

- **Status:** resolved (unattended default taken)
- **Question:** `internal/ccc/parity`'s oracle compares a read against a pristine checkout at the pin over
  *file*-shaped fixture roots. Should this increment add an OpenCode case?
- **Default taken:** no. An OpenCode source is a store, and the oracle's fixture mechanism reads a file per
  case; making it materialise a store would extend the oracle (a separate change with its own evidence).
  What covers the goal's third clause instead: the gate's required `parity` step proves the carried reader,
  provider and schema are the pin's bytes, and the native direction reads a store the harness itself wrote —
  the strongest available statement that the inherited reader reads *that* harness's rows.
- **Reversal:** teach the oracle to materialise a store for a case (`internal/ccc/opencodefixture` exists and
  could be borrowed) and record a dump from a pristine checkout.
- **Settled by:** a later increment that wants byte-level cross-tree evidence for the OpenCode reader (0001's
  argument for Pi applied to this provider).

## Q-0008-05 — A requested turn on a host whose Codex store holds no credential is recorded, not worked around

- **Status:** resolved (unattended default taken)
- **Question:** `--resume-prompt` asks for one real turn in the installed session. Measured on this host: a
  Codex turn under a **scratch** `CODEX_HOME` answers `401 Unauthorized` — the credential lives in the
  caller's own `~/.codex/auth.json`, and this build never copies a credential or login store — while the same
  boundary is already disclosed by `scripts/codex-native-check.py`'s own turn (0007's record quotes it). May
  the check seed the scratch home from the caller's, or drive the turn in the caller's own home, to obtain a
  turn?
- **Default taken:** no. The check runs its one turn through the report's own vector with its own isolation
  flags and records what the harness answered; on this host that is a **failed** check naming the harness's
  own words (never a pass, never a skip the verdict ignores), and the load half — Codex's own `thread/resume`
  through the app server, offline — is what the PASS run verifies. Seeding a credential into a scratch home
  is forbidden by the design's own rule; installing into a caller-named home to reach their credential is
  what the repository's rules reserve for the caller to do deliberately (`--target-home`).
- **Reversal:** run the check with `--target-home` naming a home that holds the caller's own credential (the
  printed resume line then names exactly that home), or seed a provider whose authentication is
  environment-backed (`env_key`) into the scratch home once such a provider is configured.
- **Settled by:** an operator instruction to spend a turn in their own Codex home, or a host whose scratch
  home can authenticate without a copied credential.

## Q-0008-06 — The source-direction comparison refuses a `shell` record rather than compare a subset

- **Status:** resolved (unattended default taken)
- **Question:** `scripts/opencode-native-check.py`'s source direction compares a source document's members
  (user texts, tool call names, recorded inputs, recorded result texts, assistant renderings) with a
  target's own records. The inherited reader answers a `shell` message as **both** a user record (the
  command) and tool evidence (a call and the recorded output), and the comparison holds no member for
  either — should it model the kind (mirroring the reader's `shell` arm, including `gjson`'s `.Str` reads
  of the recorded `output` member) or refuse a document that carries one? (The review round that found the
  comparison's other unread rules — empty `text` blocks, a result's error message, a user record's
  attachments — named both dispositions as acceptable.)
- **Default taken:** **refuse**, by name, before the harness is asked to import anything (exit 2). The
  modelling arm needs the reader's own `gjson` semantics for a member no fixture here records
  (`internal/ccc/testdata/growth/` has no OpenCode fragment and no store-shaped fixture can carry one),
  so it cannot be measured on this host, while a refusal is measured, disclosed and reversible; comparing a
  document by dropping a member a target holds would be exactly the class of claim the review was about.
  The committed fixture carries no `shell` record, so the default run is unaffected.
- **Reversal:** model the `shell` arm as a `user` text (the command), a call (`bash` with the recorded
  `callID`, or `shell` with the recorded `shellID`) and one result (the recorded `output`, its `output`
  member where `shellID` is set, the status raised by a nonzero `exit`), then lift the refusal and pin the
  arm with cases; or, if a host with the OpenCode binary measures the importer's own treatment of the kind,
  decide from that measurement.
- **Settled by:** an operator instruction, or a measurement against an installed harness that makes the
  modelling arm's rules measurable.

## Q-0008-07 — The mixed-record disposition in the source-direction comparison: modelled, not refused

- **Status:** resolved (unattended default taken)
- **Question:** `scripts/opencode-native-check.py`'s source direction compares an OpenCode document with
  the installed Codex rollout over members both sides are read into. An assistant record that carries a
  text part **beside** a tool part (a shape a caller's `--source` document may hold; the committed fixture
  does not) contributes text to the document's own reading, while the read establishes no dialogue for such
  a record — the OpenCode reader renders a persisted reasoning trace into `Content` and records no trace
  text and no projection, so nothing separates the rendering from the record's own words — every writer
  omits the text and counts it `unresolved_dialogue`, and the call and its recorded result travel on their
  own. The second review round's first finding (P2) named both dispositions as acceptable: model the
  conversion's disposition in the target expectation, or explicitly refuse the shape (as the comparison
  already refuses a `shell` record, Q-0008-06). Which one?
- **Default taken:** **model** it, in a second reading (`opencode_written_evidence`) beside the document's
  own (`opencode_document_evidence`). Measured on this host through the carried reader and the route's own
  writer (a throw-away Go probe, deleted again): the read answers `Content` = the text, `Thinking` empty,
  no projection and one call; the rollout holds the `function_call` and its `function_call_output` and **no**
  assistant message for the record, and the loss table names `unresolved_dialogue = 1` (the record with a
  reasoning part and no call beside it keeps its rendering as its own text, counted `unprojected_dialogue`).
  The expectation therefore holds no text for such a record, and because the comparison is by equality the
  omission is *pinned* rather than excused: a target that wrote the rendering holds an assistant text the
  source's read never established and is a difference. The planting's own comparison — document against
  document — keeps the stricter reading, so a store whose assistant record lost a part stays a difference.
  A refusal was rejected because mixed records are the common shape in a real session: refusing would make
  `--source` unusable on the documents this direction exists to compare, while the modelled reading keeps
  every member the read does establish under comparison.
- **Reversal:** refuse the shape by name before anything is planted — the finding's other named disposition
  (the `OPENCODE_UNMODELLED_MESSAGE_KINDS` arm's shape), at the cost of refusing a real session's common
  shape; or drop the assistant-text member for such a document, which is reversible but no longer pins the
  omission.
- **Settled by:** an operator instruction, or a measurement against an installed harness whose reader
  records the record's trace text (or projects a dialogue) for a call-bearing record — that would make the
  text established and the reading here too narrow; the reader is carried (`internal/parser`), so the
  measurement belongs to an upstream revision bump.

## Q-0008-08 — The file-bearing-result disposition in the source-direction comparison: mirrored, not refused

- **Status:** resolved (unattended default taken)
- **Question:** `scripts/opencode-native-check.py`'s source direction compares a caller's document (or the
  committed fixture) with the installed Codex rollout. A completed tool part whose `state.content` carries a
  `file` block makes the carried parser answer the part's text as its own JSON **block array**
  (`internal/parser`'s `openCodeV2ToolCall` marshals its blocks there), each writer writes those bytes
  verbatim (the read hands the encoding on as decoded text), and an expectation that held no text for such
  a part reported the rollout's own output as a difference — the fourth review round's finding (P2,
  `review-findings-r3.md`). The finding named both dispositions: make the expectation follow the source-read
  path, or explicitly refuse the shape before planting it (as the comparison refuses a `shell` record,
  Q-0008-06). Which one?
- **Default taken:** **mirror** it — `opencode_result_block_json` plus a mirror of the carried encoder
  (`go_json_string`/`opencode_go_json`). The encoding is fully determined by the document (the blocks in the
  content's own order, each object's members in their names' byte order, the parser's one member rewrite for
  an inline `data:image/…` file, the structured text and error message appended as text blocks), and every
  rule was measured rather than assumed: the encoder's spellings over every code point below `0x20` and over
  `0x7f`/HTML characters/`U+2028`/`U+2029`/non-ASCII through a throw-away Go probe, and the finished
  comparison end to end on this host (OpenCode 2.2.5 + Codex 0.170.0): a planted document holding three
  file-bearing results converts and the run ends `RESULT: PASS — 18 check(s), none failed`, where the
  pre-fix expectation ended `RESULT: FAIL — 1 check(s) failed` naming the rollout's encoded output as the
  source's own empty reading. Q-0008-07's reasoning applies to this shape too (such results are a real
  session's shape, and refusing would make `--source` unusable on the documents the direction exists for),
  and the mirror keeps every member the read establishes under comparison instead of skipping it — the
  planting's document-against-document reading included, so a store whose part lost its block is a
  difference.
- **Reversal:** refuse the shape by name before anything is planted (the `OPENCODE_UNMODELLED_MESSAGE_KINDS`
  arm's shape), at the cost of refusing a real session's shape; or hold no text for such a part again, which
  is reversible but reports a faithful conversion as a difference.
- **Settled by:** an operator instruction, or an upstream revision that changes the carried parser's answer
  for block-bearing results (the encoding, the member rewrite, or whether the read hands it on as decoded
  text) — the mirror belongs to the carried revision's own rules, so the measurement belongs to an upstream
  revision bump, and an operator who prefers refusal for this shape can say so.

## Q-0008-09 — The Python-minor floor is pinned in the suite; whether CI also runs the gate under it is the operator's call

- **Status:** open (default taken: the floor is pinned in the suite and the floor run is this increment's
  evidence on the authoring host; the CI matrix keeps its two 3.13 legs).
- **Question:** the docs bind the suite -- and, from this round, the shipped `scripts/**/*.py` -- to the
  oldest Python they name (3.11), and the fourth review round found that binding unenforced: a 3.12-only
  f-string in the suite made `selftest` unimportable under 3.11 (measured: `python3.11 -m unittest discover`
  reported the module broken before any case ran) while CI's pinned 3.13 stayed green. The fix adds
  `scripts/tests/test_python_floor.py` (the running parser constrained to the floor via `feature_version`,
  plus the four PEP 701 f-string relaxations that constraint cannot see). That pin runs wherever the suite
  runs and needs no host tool -- but CI's matrix never runs a floor interpreter, and the matrix and the CI
  section of `docs/harness.md` are one statement held together by `scripts/tests/test_ci_matrix_claim.py`
  (which refuses a matrix key it does not read), so adding a floor leg is a deliberate change to a pinned
  artifact, not a step this round could take inside its own scope.
- **Default taken:** no CI change. The residual gap is disclosed rather than hidden: the pin's four rules
  cover the measured PEP 701 set, and a relaxation outside it (PEP 701's implicit parentheses around a
  field's expression, say) is caught only where the interpreter at the floor itself parses the file -- which
  is why the floor run is recorded as evidence: `python3.11 -m unittest discover -s scripts/tests
  -p 'test_*.py'` answers `Ran 442 tests ... OK` on the authoring host (and `python3.11 scripts/gate.py`
  was run for the same record).
- **Reversal:** add a `python-version` dimension (or one `include:` leg) to `.github/workflows/gate.yml`,
  extend `scripts/tests/test_ci_matrix_claim.py`'s reader and the §CI/§Platform-boundary statement with it,
  and let CI hold the floor; the cost is one more job per run, the benefit is the *behavioural* half of the
  3.11 claim (the suite's `pathlib`-style differences, not only its parses) checked where nobody has to
  remember to run it.
- **Settled by:** an operator instruction, or a CI change that adds (or deliberately refuses) a floor leg --
  the pin itself needs no decision.

## Q-0009-01 — The increment is narrowed to one reference route, and the review loop is contractively bounded

- **Status:** resolved (owner, 2026-11-20 — fork A taken).

**Question (operator, 2026-11-20).** The wider goal-0009 attempt (PR #14) ran 19 machine-review rounds with
zero clean findings and the reviewed diff grew 191 KB → 1.47 MB. Two blind cross-family consults
(`gpt-6.2-sol` and `gemini-4-carbon`) converged: selection is a lossy pass over a stateful event log, so a
disposition the read→selection seam does not carry is re-derived by each target writer from the punctured
stream (an M×N matrix the per-round patches only widen); the obligation-citation ledger is an existential
traceability index, not a universal soundness proof, so an attentive audit always finds an unexercised
dimension; and the fix operator's gain is > 1, so a zero-finding fixed point is unreachable at any cap. Both
rejected all three pure forks (shrink / accept a disclosed residual / raise the bound).

**Decision (owner, 2026-11-20).** Fork **A**: supersede PR #14; **narrow the increment to one reference route
— Claude Code → Codex**; make the seam's carried dispositions the contract (D6); add a **contractive
breaker** to the review loop. After it lands, the other four routes fan out as their own increments before
0010–0013.
**Alternatives put to the owner:** B — keep PR #14 and refactor it in place (rejected: a 1.47 MB reactive
branch reviewed as one PR); C — accept a disclosed residual and merge (rejected by both consults: the
residual contains an active P1 semantic falsification); D — raise the round cap (rejected: gain ≥ 1).

**Why the seam, not a new IR layer.** The seam's shape was verified on the superseded attempt's tree:
`Conversation` there *already* carried the seam's answers for several dispositions
(`internal/ccc/conversation.go` on `archive/0009-pr14-pre-forkA`, added by PR #14's round 4 —
`sourceResultPairs` "It is what keeps a writer from re-reading the pairing over the window",
`sourceKeptAncestor`, `sourceLineInactive`/`sourceLineHolds`, `sourceFileLine`). This increment's base tree
carries none of them (`Conversation` is `Source`, `Messages` and `Fidelity` only), so the increment **adds** —
drawing on the tag where it fits — the set the reference route needs. The root cause is therefore the
**coverage** of the carried set, not the seam's absence — so the increment completes the set for one route
instead of introducing a second parser or a persistent index.

**Reversal:** re-open the wider scope by adding a carrying disposition set for all five routes at once (the
cost is the M×N matrix this decision exists to avoid), or retire the breaker by raising
`CONTRACTIVE_MAX_ORDINARY_ROUNDS` — the review conductor's own knob, outside this tree — above the series'
historical maximum (the cost is the 19-round/1.47 MB failure mode returning silently).
**Settled by:** an operator instruction; nothing downstream needs to re-decide it.

## Q-0009-02 — Which four routes "the other four target routes" are

- **Status:** open (default taken: the recorded wording stands, and the four later increments are unchanged
  in count).
- **Question:** the goal's non-scope and `AGENTS.md` list the four later increments as "the other four target
  routes (Pi, OMP, OpenCode, Codex-as-source)". Three entries name targets and the fourth names a source, so
  the list reads either as the four remaining target formats (Claude, Pi, OMP and OpenCode — the target
  writers the superseded attempt touched beside the reference route's Codex target) or as route families
  named from mixed sides. Which enumeration does the deferral mean?
- **Options:** (a) name the four target formats; (b) name four source→target pairs; (c) leave the wording as
  recorded.
- **Default taken:** (c). The apply stage's findings pass flagged the mixing and left the contract text
  untouched: which routes the non-scope defers is the owner's to state, and rewording it would change scope
  text on a guess.
- **Reversal:** one line in `docs/goals/0009-selective-context.md`, `AGENTS.md` and
  `docs/plans/0009-selective-context.md` replaces the parenthesis with the chosen enumeration; nothing
  downstream changes.
- **Settled by:** an owner sentence naming the four routes/increments.

## Q-0005-18 — A local re-derivation that the ownership rule now forbids is declared debt

- **Status:** open (default taken: the guard stays as **declared debt** — not removed, not extended, and
  replaced as soon as an authoritative read answers the same question).
- **Question:** the binding ownership rule ("each half has one owner … never a re-implementation of a foreign
  format's shape": `AGENTS.md` §What is binding, `docs/core-design/README.md` §2) forbids this repository from
  deciding what a foreign loader accepts or skips — yet the same contract still **requires** one such decision:
  the Pi-family source leaf check (`internal/ccc/pi_source_leaf.go`, invoked before a conversion by
  `internal/ccc/continue.go`) walks the source file's own entry path and classifies the entries the carried Pi
  reader did not return, because that reader skips the tree entries Pi's own loader builds context from. May a
  mandated check be kept where the newer rule forbids the judgment it makes, and if so, under what condition
  does it move?
- **Options:** (a) keep it as declared debt with a named replacement condition and a bar on extending it;
  (b) remove the guard now and let a leaf be established only where the read establishes it — conversions that
  resume a branch the conversation left become possible again, and the refusal cases 0005's rounds added go
  red; (c) exempt the Pi route in writing (refused: that is the exact defect the rule names).
- **Default taken:** (a). The guard is preserved because a conversion that ignored it could write a target
  whose loader walks a branch the source's own file had left; it **may not grow a new local judgment**; and it
  moves only to a read that preserves **all three** behaviours the guard carries — reconciling the deepest
  carried record with the read's last message, refusing an uncarried active-path entry the harness would use
  as context (a `branch_summary` or `custom_message`), and counting the source's own system records it omits
  (`source_system_records`) — from Pi's own loader (a load/`get_state`-shaped read of the source file) or an
  authoritative source-side read. A replacement that answered only the leaf question would weaken the guard
  rather than retire it. Until then the contradiction is disclosed here rather than silently resolved in
  either direction.
- **Reversal:** delete the check and its tests in one increment; the branch-switch refusal it pins goes with
  them (the fixture covers three targets), so a continuation on an abandoned branch becomes possible again —
  the cost is a **weakened** refusal, not a larger one — and the ownership rule then reads without exception.
- **Settled by:** an owner sentence, or a Pi-side read that answers the leaf question natively **and** preserves
  the guard's other tested outcomes — the refusal of an uncarried active-path entry, the
  `source_system_records` count, and the warning-with-no-leaf result where no source file stands behind the
  read — in which case the debt is discharged by **replacing** the walk, not by dropping it.

## Q-0008-10 — The OpenCode source oracle's expectations are a Python reconstruction of the carried reader

- **Status:** open (default taken: the check is **declared debt** — kept, not extended, and its expectations
  move to the carried reader's own output).
- **Question:** `scripts/opencode-native-check.py` computes the dialogue and tool results it expects from a
  Python reconstruction of the carried OpenCode reader, and it already answers differently from the reader it
  models: the check refuses a `shell` record before the import, while the carried reader
  (`internal/parser/opencode_v2.go`) reads that record as tool-bearing history. The native Codex load check
  does not resolve that source-side disagreement. May a native check keep its own model of a carried reader
  where the binding ownership rule gives that judgment to the reader itself?
- **Options:** (a) keep it as declared debt with a replacement condition — the expectations read from the
  carried reader's own answer; (b) replace the expectations now with the reader's output — not in this
  increment, which changes no script, and a change to a native check belongs to its own increment with its own
  two-stage review; (c) leave the divergence undisclosed — refused.
- **Default taken:** (a). Recorded here rather than silently, so the divergence between the check's refusal
  (`Q-0008-06`'s default taken) and the reader's own reading of the same record is visible to the next lane
  that touches the OpenCode source direction.
- **Reversal:** an increment that takes those expectations from an **independent AgentsView read of the
  OpenCode source** — not from the conversion's own read-back, which opens the *installed* artifact
  (`internal/ccc/route_shared.go`) and therefore cannot show what the source supplied — and keeps that
  read-back on the **observed** side of the comparison, re-pinning the three refusal cases; the debt then
  becomes a rule of the owned side.
- **Settled by:** the increment that next touches `scripts/opencode-native-check.py`, or an owner sentence.

## Q-0005-19 — The Pi/OMP target-side replay prediction is a local model of the target's own loader

- **Status:** open (default taken: declared debt — kept, not extended, replaced the moment the loader answers
  the same question itself).
- **Question:** `internal/ccc/pi_verify.go` walks the converted file's `parentId` chain, chooses the active
  compaction boundary and **predicts** the roles the native loader should return; the Pi and OMP verifiers then
  accept or reject a native load by that prediction (`pi_verify.go`'s load verdict, `omp_verify.go`'s), and the
  two Python native checks mirror it (`scripts/pi-native-check.py`, `scripts/omp-native-check.py`). That is a
  local model of the target harness's own replay — the judgment the binding ownership rule gives to the
  harness. May a verifier keep predicting what the loader will return?
- **Options:** (a) keep it as declared debt with a replacement condition; (b) replace the prediction now with
  the loader's own answer — not in this increment, which changes no verifier and no script, and a change to a
  verifier belongs to its own increment with its own two-stage review; (c) keep predicting without disclosing
  it — refused.
- **Default taken:** (a). The prediction is what lets the check **refuse** a load that looks natively
  loadable: the loader's own answer is already read by both verifiers (`get_messages`/`get_state` against a
  throw-away copy) and compared against an **independent expected sequence**, and that comparison is the only
  thing that can reject a wrong replay — the same-count role swap that `pi_verify_test.go` pins is rejected
  solely because an expectation exists beside the observed answer. So the debt is **where the expectation
  comes from**, not whether to have one: replacement means the sequence is derived from the harness's own
  documented replay semantics or an authoritative read *instead of* a local walk, **with the mismatch
  regression preserved** — a replacement that deletes the walk and keeps only the loader's answer would
  weaken the check, not retire the debt.
- **Reversal:** replace the walk in one increment with an expectation derived from the harness's own
  documented replay semantics or an authoritative read, keep a case that fails when the expectation is
  removed (the role-swap regression, or an equivalent), and only then delete `pi_source_leaf`-style local
  reasoning from the verifiers; the refusal surface narrows to what the retained expectation covers — a
  **weakened** refusal, not a larger one.
- **Settled by:** the increment that next touches `internal/ccc/pi_verify.go` / `omp_verify.go`, or an owner
  sentence.

## Q-0007-13 — The Claude → OpenCode oracle's expectations are a Python reconstruction of the carried Claude reader

- **Status:** open (default taken: declared debt — kept, not extended, replaced by an independent AgentsView
  read of the Claude source).
- **Question:** `scripts/opencode-native-check.py`'s `--direction claude-to-opencode` arm re-derives the carried
  Claude reader's tool-result decoding and pairing in Python and uses that reconstruction as the **expected**
  result, while OpenCode's own export supplies only the observed side. The two already disagree on a faithfully
  carried result: the Python oracle drops a result that consists of a lone U+001C — `.strip()` treats it as
  whitespace — while the writer's `strings.TrimSpace` does not, so a **correct** carry can fail the oracle. May
  a native check hold its own model of a carried reader where the binding ownership rule gives that judgment to
  the reader itself?
- **Options:** (a) keep it as declared debt with a replacement condition; (b) replace the expectations now —
  not in this increment, which changes no script, and a change to a native check belongs to its own increment
  with its own two-stage review; (c) leave the divergence undisclosed — refused.
- **Default taken:** (a). The whitespace divergence is named here so it is not rediscovered as a mystery
  failure: it is a **false negative of the check**, not a defect in the writer.
- **Reversal:** take those expectations from an independent AgentsView read of the Claude source (the observed
  side stays OpenCode's export), re-pin the arm's cases, and add a case whose recorded result is a lone
  U+001C, so the divergence cannot return silently.
- **Settled by:** the increment that next touches `scripts/opencode-native-check.py`'s Claude arm, or an owner
  sentence.

## Q-0005-20 — The Claude → Pi oracle expects the first raw user record, which the carried reader skips

- **Status:** open (default taken: declared debt — kept, not extended, replaced by an independent AgentsView
  read of the Claude source).
- **Question:** `scripts/pi-native-check.py` takes the **first raw user record** of a Claude transcript as the
  prompt it expects in the converted Pi session, while the carried reader skips a record carrying `isMeta` and
  selects the next one — a regression the carried suite pins
  (`internal/parser/claude_parser_test.go`). The check's expectation and the reader's answer therefore disagree
  by inspection: on a transcript whose first user record is metadata, the check expects a prompt the read never
  produced. May a native check compute its expected prompt from the raw file instead of from the read?
- **Options:** (a) keep it as declared debt with a replacement condition; (b) replace the expectation now — not
  in this increment, which changes no script, and a change to a native check belongs to its own increment with
  its own two-stage review; (c) leave the disagreement undisclosed — refused.
- **Default taken:** (a). **Not yet reproduced natively** — it is recorded as an *inspection* finding of the
  machine review, so a later increment that touches this arm should reproduce it before repairing, and either
  way the expectation should come from the read.
- **Reversal:** take the expected prompt from an independent AgentsView read of the Claude source, add a
  fixture whose first user record carries `isMeta`, and re-pin the arm's cases.
- **Settled by:** the increment that next touches `scripts/pi-native-check.py`'s Claude arm, or an owner
  sentence.

## Q-0003-12 — The Codex read-back pairing attributes carried text to written records by a local heuristic

- **Status:** open (default taken: declared debt — kept, not extended, replaced by the carried reader's own
  answer or an authoritative read).
- **Question:** `internal/ccc/route_shared.go`'s `pairReadBackMessage` matches the Codex read-back's text to the
  records this build wrote by a **same-role subsequence** heuristic. That is a local rule about how the carried
  reader renders a record — the judgment the ownership rule gives to the reader — used where a projection cannot
  establish the pairing (both prompts projected away). It is what the swapped-prompt regression depends on.
- **Options:** (a) keep it as declared debt with a replacement condition; (b) replace it now — not in this
  increment, which changes no Go file; (c) treat it as exempt because the compared bytes are ours — refused: the
  *comparison* is ours, the *attribution* is a claim about the reader.
- **Default taken:** (a). Named separately so the emitted-byte exemption above stays narrow, and so the pairing
  heuristic is visible to whoever next touches the installed-dialogue verification.
- **Reversal:** take the pairing from an authoritative read (the read's own per-record answers) instead of a
  subsequence match, keep a case that fails when the attribution is wrong (the swapped-prompt regression or an
  equivalent), and only then delete the heuristic.
- **Settled by:** the increment that next touches `pairReadBackMessage`, or an owner sentence.
