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
