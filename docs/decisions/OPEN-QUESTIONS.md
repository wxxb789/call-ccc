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
  answer the tree pins). The rendering rule is already the documented contract of the field, a caller
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
