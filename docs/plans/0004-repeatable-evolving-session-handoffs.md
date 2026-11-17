# Plan — 0004 Repeat continuation as source history grows

Contract: [`docs/goals/0004-repeatable-evolving-session-handoffs.md`](../goals/0004-repeatable-evolving-session-handoffs.md).
Governing design: [`docs/core-design/README.md`](../core-design/README.md).
Shape references: increment 0002 ([`docs/plans/0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md)),
increment 0003 ([`docs/plans/0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md)).

Written before implementation. Every fact below about the inherited readers and the two accepted routes was
measured in this tree, on this host, before any code was written; every measurement is repeated in
`docs/evidence/0004-repeatable-continuation-handoffs.md`.

## 1. What this increment adds, and what already holds

`Done when` clause 1 asks for two explicit conversions of a growing source that are distinguishable and
correctly attributed. Measured **before** this increment, with the built CLI and a copy of the committed
Claude fixture (evidence §2):

| Fact | Measured |
| --- | --- |
| a second conversion writes a second session | two rollouts under the same home, ids `01a1e92e-3604-…` and `01a1e92e-4345-…` |
| the later conversion carries the appended turn | `conversation: 8 → 10 message(s)`, `converted: 5 → 7 message(s)`, `2 → 3 turn(s)` |
| the earlier target is untouched | its bytes and digest are the ones the first run wrote |
| the source is read again from its own file | both runs report `read from: source` and the grown file's digest |

So the *behaviour* the clause asks for mostly exists — a conversion is already a fresh-identity snapshot
that re-reads the source. What does not exist is the *information* that makes two results comparable and
each one attributed, and one rule the clause below it demands. This increment adds:

1. **A source boundary on every conversion result**: the source file's own bytes at the boundary instant
   (size, sha256), the offset at which the source's last complete line-recorded event ends, and the bytes
   after it that no snapshot publishes — structured in `--json`, printed in the human block.
2. **The incomplete-tail rule, stated rather than silent.** Measured: the inherited Codex reader drops an
   unterminated trailing record *and counts nothing* (`internal/parser/codex.go`: *"A writer may leave its
   final JSON record incomplete while the session is live. Count terminated malformed records, and count an
   unterminated tail only when the caller owns a stable snapshot"*), while the inherited Claude reader
   counts it as a malformed line and flags the session truncated. A Codex conversion of a source captured
   mid-append therefore reported **nothing at all** about the tail it left out (evidence §3). The boundary
   is the uniform answer: it is computed from the file's own bytes, not from any provider's classification.
3. **Repeat semantics pinned by tests**: two conversions of one growing source, both targets, the earlier
   session unchanged, both attributed to their own bytes.
4. **Native evidence for the accepted 0002 route through the repeated scenario**: `scripts/codex-native-check.py`
   converts a *copy* of the fixture, loads it, lets the source grow by one committed turn fragment,
   converts again, loads the second session and shows the target's own view of it holding the appended turn
   — plus one optional real turn in the second session (`--repeat-resume-prompt`) whose answer quotes the
   appended fact.
5. **The rules, in the files that carry them**: `AGENTS.md`, `docs/conventions.md`, `docs/harness.md`,
   `cmd/ccc`'s help text, `README.md`.

## 2. The boundary: what is claimed, and what is deliberately not

### 2.1 The claim

`SourceBoundary` (new, `internal/ccc/continue.go`) is attached to the conversion's source report as
`source.boundary` and carries:

| Field | Meaning | How it is answered |
| --- | --- | --- |
| `state` | `taken` or `not-taken` | `taken` when the conversion read the session from its own file at a path that names one file; `not-taken` with the reason for an archive read, a container-member identity, or a read that named no path |
| `detail` | the state's own words | e.g. *"the conversation's own source file, read after the session and before anything was written"*, or the reason it was not taken |
| `path`, `bytes`, `sha256` | the source file, its size and the digest of exactly those bytes | one streamed pass, the same pass that already computed the pre-conversion digest |
| `records_are_lines` | whether the record offsets beside it are claimed | the **provider's own declaration** (`parser.SourceCapabilities.IncrementalAppend` or `parser.ContentCapabilities.MalformedLineCount` = `CapabilitySupported`), never this build's guess about a format |
| `records_end_bytes` | the offset at which the source's last complete line-recorded event ends | the offset just after the file's last line feed, read in that same pass |
| `unpublished_tail_bytes` | the bytes after it: a record the file had not finished writing | `bytes − records_end_bytes` |

The block is attached to the conversion report only. `ccc inspect` does not take one: an inspection is the
cheap discovery command, and a boundary pass is another full read of a source that can be hundreds of
megabytes — the design's own rule ("Do not duplicate expensive source fingerprints solely for bookkeeping").
The field is documented as the conversion's, so its absence in an inspection is not a claim about a file.

### 2.2 What the boundary is not

- **It is not a claim that the parser consumed exactly those bytes.** The pass runs after the read and
  before anything is written; a file that grows between the two makes the boundary newer than the parse,
  and no comparison of the two is attempted (the parse's own `File.Size` is a composite for providers whose
  sources are a layout, so comparing it would produce a "the file grew" statement that is sometimes false —
  the defect this repository forbids). The `detail` states the ordering in words instead.
- **It is not a loss category.** An unterminated tail is not a recorded event: the goal's bounds say
  "Only complete recorded source events are eligible for a snapshot", and upstream's Codex reader treats a
  live session's tail the same way. The boundary states it, and the report's warning channel names it in
  words; `conversion.losses[]` keeps counting the categories the writers reason over (and for a Claude
  source the inherited reader's own `malformed_source_lines` count still travels beside it — a count of what
  *the parser* skipped, which the boundary does not duplicate).
- **It is not taken from a provider fingerprint.** `identity.fingerprint` is the provider's own answer about
  its source (for a layout, a composite); the boundary's digest is this build's sha256 of the file's bytes,
  which is what `installation.source_check_state` already compares before and after a conversion.

### 2.3 The incomplete tail, case by case

| Source file's tail | Read's own fidelity | Boundary | Published? |
| --- | --- | --- | --- |
| last byte is a line feed, every line parseable | `malformed_lines = 0` | `records_end_bytes = bytes`, `unpublished_tail_bytes = 0` | everything |
| last line is invalid but line-feed terminated (the committed Claude fixture) | `malformed_lines ≥ 1` | `records_end_bytes = bytes`, `unpublished_tail_bytes = 0` | the malformed line never becomes a message |
| trailing bytes hold no line feed (a record still being written) | Claude: `malformed_lines` +1 and `truncated = true`; Codex: **nothing** | `unpublished_tail_bytes > 0`, and a warning names it | nothing of the tail |
| the file holds no line feed at all | as above | `records_end_bytes = 0`, `unpublished_tail_bytes = bytes` | nothing (no complete line record) |
| the boundary pass cannot read the file | — | the run stops, the way a source check that cannot run already does | nothing written before the failure |

A source whose tail is unfinished is **not** a failure: it is a live session, the conversion is real, and
the report's own words say how to get the rest ("convert again after the source is written").

## 3. Public shape

### 3.1 `internal/ccc` (the engine)

```go
// SourceBoundary is where one conversion's snapshot of the source stops ...
type SourceBoundary struct {
	State  string `json:"state"`             // taken | not-taken
	Detail string `json:"detail,omitempty"`
	Path   string `json:"path,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	RecordsAreLines      bool  `json:"records_are_lines"`
	RecordsEndBytes      int64 `json:"records_end_bytes,omitempty"`
	UnpublishedTailBytes int64 `json:"unpublished_tail_bytes,omitempty"`
}
```

`SessionReport` gains `Boundary *SourceBoundary \`json:"boundary,omitempty"\``. `Continue` fills it;
`Inspect` leaves it absent (§2.1). No other public type changes: `ContinueOptions` gains nothing, and the
existing `installation.source_check_state` / `detail` keep their meaning (the boundary's sha256 *is* the
digest they compare, taken in one pass).

### 3.2 The CLI

No new command and no new flag. The human block gains up to two lines, and the same facts are already in
`--json`:

```text
  boundary:     2884 byte(s) read, the source's records end at byte 2884; nothing in the file is unpublished
  unpublished:  151 byte(s) after the last record terminator: the record the source was writing is not part of this conversion
  boundary:     not taken (the conversion read an archive row, so no source file was open at all)
  boundary:     3626 byte(s) read; this source's records are not lines the harness appends, so no record boundary is claimed
```

`ccc help` gains one paragraph: a conversion is a snapshot, converting again reads the source again and
writes another new session, and each result names the source boundary it covers.

## 4. Repeat semantics the tests pin

- Each conversion writes a **fresh identity** and its own session: two conversions of one source into one
  home leave two files, neither replaced, neither merged (both targets).
- A conversion never touches a session it did not write — including an earlier conversion's, and including
  one the target harness has since resumed (the native check resumes the first session before the second
  conversion, then re-compares its bytes).
- Each result is **attributed to its own bytes**: `boundary.sha256`/`bytes` equal the check's digest of the
  source copy as it was *before* that conversion, and the later boundary is the larger one; the appended
  turn's own text is in the later target document and absent from the earlier one.
- `installation.source_check_state` keeps its meaning per run: the source's digest before and after *that*
  conversion.

## 5. Native verification: `scripts/codex-native-check.py` (the 0002 route), extended

The check keeps its shape and gains a repeat scenario. Order and cost:

1. the committed fixture is **copied** into `<scratch>/src/<project-dir>/<id>.jsonl`; the committed tree is
   hashed before and after the run and must not move (the check never grows a committed fixture);
2. **conversion A** from the copy, with every check 0002 accepted: three states, install layout, JSONL
   shape, the inherited reader's read-back, `thread/resume`, the harness's own `thread/read` view, the two
   rendered command lines; the copy is digested before and after the conversion and must be unchanged;
   the conversion's reported boundary is compared with the check's own digest of the copy;
3. `--resume-prompt`, when given, runs **one real turn in A** (0002's own level, unchanged) — which also
   makes A a session its harness has already resumed;
4. the copy **grows** by the committed fragment `growth/claude-complete-turn.jsonl`; the check asserts the
   delta is exactly that fragment's bytes;
5. **conversion B** into the same home, and the checks that make the clause material: B's identity is new,
   B's reported boundary is the grown copy's own bytes (≠ A's), B's carried counts grew, the appended text
   is in B's installed file and not in A's, **A's installed file digest is unchanged**, the copy is
   unchanged by B's run, and Codex's own loader opens B (`thread/resume`) with its own `thread/read` view
   listing the appended turn;
6. `--repeat-resume-prompt`, when given, runs **one real turn in B** through B's reported invocation — the
   answer is printed under the harness's answer marker and is what the evidence record quotes as proof that
   the later snapshot's model-visible history holds the appended turn;
7. the report line says how many sessions were installed and how many turns ran, so a run that ran none (or
   two) never reads as the other.

Its offline cases (`scripts/tests/test_codex_native_check.py`) drive the shipped `main` with the existing
stubs, extended so the second conversion and the second loader call answer with their own document, thread
id and installed file: the intact run passes every new comparison; a mutation of **A's installed file**
during the second conversion fails exactly the new comparison (the ordering rule — the comparison must run
after the stage it is about); a second document whose boundary digest is not the grown copy's fails the
attribution comparison (the check verifies rather than trusts).

## 6. Evidence plan

| `Done when` clause | Artifact |
| --- | --- |
| two conversions of a growing source are distinguishable and correctly attributed; the later includes the appended history; the earlier target stays unchanged | the by-hand run in evidence §2 (two `ccc continue` runs on a grown copy: both boundaries, both ids, both digests, the appended turn's text in B's file and not A's, A's digest after B); the Go test for both targets; the native check's own report (§5) |
| an actively appended or partially written source publishes no malformed tail event as complete history; a controlled incomplete-tail case with its boundary or a clear failure | evidence §3 for a Claude and a Codex source (the fragment's text absent from every message and from the installed file; `boundary.unpublished_tail_bytes` = the fragment's size; the warning; and the Codex case where the read itself counts nothing); the Go test; the pass's own unit cases (empty file, no line feed at all, a terminated invalid line) |
| the result exposes enough source and target identity/boundary information to choose which continuation to resume, without an always-on index | the `--json` documents of §2/§3 with `source.boundary`, `source.counts`, `target_session.session_id`/`path`/`resume_command`; the human block of §1/§2; the CLI test that pins both lines; `ccc help`'s paragraph |
| the accepted 0002 route stays natively resumable and the source stays unchanged through the repeated scenario | `scripts/codex-native-check.py`'s run on this host: both `thread/resume` answers, both `thread/read` views, the two real turns' own output, the source copy's and A's digests before and after |
| the rules reach the files that carry them | the diff's hunks in `AGENTS.md`, `docs/conventions.md`, `docs/harness.md`, `README.md`, `cmd/ccc`'s usage text; `docs/goals/README.md`'s rows (0003 archived in this increment, 0004 marked implemented) |

## 7. Gate, docs, bookkeeping

- **Gate: no new step, no new required step, no narrowing.** The new Go tests live in `internal/ccc` and
  `cmd/ccc`, which `scripts/gate-targets.txt` already builds and runs; the native check stays a script;
  `scripts/tests/test_codex_native_check.py` and `test_evidence_claims.py` grow cases inside the existing
  `selftest` step. The evidence record joins the records that must credit a recorded turn's answer
  (`ROUTE_DOCUMENTS`), so its §5 cannot quote a run it did not make.
- `AGENTS.md`: the binding bullet on fresh identities gains the repeat rule (a growing source is read again
  and written as another fresh session; the boundary every result names; never a merge into an existing
  target session), and the layout/harness lines name the growth fragments and the check's repeat scenario.
- `docs/conventions.md`: the rule of the work — a conversion is a snapshot of the source *at its boundary*,
  the boundary is reported, an unfinished tail is disclosed and never repaired, and repeated conversions
  are separate sessions.
- `docs/harness.md`: the codex check's entry gains the repeat scenario and the second prompt flag; the
  offline-case requirements it already states apply to the new cases.
- `README.md`: one paragraph — converting again after a session grows.
- `docs/goals/README.md`: 0003 accepted (merged as `18d50b2`) and archived in this increment; 0004 marked
  `Implemented on feat/0004-repeatable-evolving-session-handoffs; awaiting operator acceptance`. The goal
  file for 0003 moves to `docs/goals/archive/` unchanged, and every link that named it is retargeted.
- `docs/decisions/OPEN-QUESTIONS.md`: entries for the defaults taken alone — the record-boundary gate
  (provider declaration vs. this build's inference), the boundary's absence from `ccc inspect`, the
  unfinished tail as a boundary statement rather than a loss category, and the native repeat scenario's
  scope (the 0002 route's check, with `--repeat-resume-prompt` as a second, separately asked-for turn).
- `docs/upstream.md` and the carried tree are untouched: no carried file changes, and the parity step must
  still pass.

## 8. Out of scope, with reasons

- **Detecting earlier conversions of the same source** (to offer "--update"): the design forbids a second
  index and says a target writer's records are the target's, which for both routes record no provenance
  this build could trust; the boundary plus the fresh identity is the reversible half. Converting again is
  the documented remedy.
- **Merging later source history into an existing target session**: explicitly out of the goal's bounds and
  the design's ("not live synchronization; do not promise cross-session merging").
- **A boundary in `ccc inspect`**: §2.1 (cost).
- **Widening the record-boundary gate to providers that declare neither capability** (Copilot's JSONL, for
  one): the claim would rest on this build's inference about a format instead of the provider's own
  declaration; the plan takes the conservative gate and records the question.
- **A repeat scenario in `scripts/claude-native-check.py`**: the contract names the 0002 route, whose check
  is extended; the repeat property is pinned for both target routes by the Go tests, and the Claude route's
  check keeps its accepted shape. Recorded as a limit, not as an omission nobody noticed.
- **A second (Codex-source) growth fragment**: the growth property is about the source *file* being re-read,
  which the Claude-source case exercises for both targets; a Codex-source growth belongs to the increment
  that extends 0003's route, and the tail case (where the two providers genuinely differ) *is* covered for
  both.
- **Automatic re-conversion, watchers, daemons, process coordination**: the goal's bounds.
- **A target-side digest in the report**: a target session is expected to grow when its harness resumes it,
  so a digest recorded at install time would invite a false "unchanged" question; what a caller needs is
  the resume identity, which the report already carries.
