# Evidence — 0004 Repeat continuation as source history grows

Contract: [`docs/goals/archive/0004-repeatable-evolving-session-handoffs.md`](../goals/archive/0004-repeatable-evolving-session-handoffs.md).
Plan: [`docs/plans/0004-repeatable-evolving-session-handoffs.md`](../plans/0004-repeatable-evolving-session-handoffs.md).
Route evidence this increment extends: [`0002-claude-to-codex-continuation.md`](0002-claude-to-codex-continuation.md)
(the accepted Claude Code → Codex route) and [`0003-codex-to-claude-continuation.md`](0003-codex-to-claude-continuation.md).

Every command below was run on this host, in this tree, with the exit code and the output it printed.
Nothing here is re-typed from memory; where a number appears it was counted by the run that names it.
Absolute paths are as the runs printed them, with this account's home spelled `~` and the repository root
spelled `<repo>`; scratch directories are under `/tmp/`. No real session content is quoted: every fixture
string is invented, and the model turns ran against the committed fixtures only.

Host facts this increment was measured against:

| Fact | Value |
| --- | --- |
| Codex CLI | `codex` resolved from `~/.local/share/mise/installs/codex/latest/bin/codex` (the installed binary `scripts/codex-native-check.py` probes and drives) |
| model access used by the real turns | the host's own provider configuration, reached through the throw-away home's seeded `config.toml` (`--model-provider tencent-litellm --model deepseek-v4.1-flash-fast`); no credential was copied or printed |
| data directories | every run used an isolated `--target-home`/`--codex-home`; no developer data directory was read or written |
| build | `go build -tags fts5 -o bin/ccc ./cmd/ccc` (`bin/` is git-ignored) |

## 0. What was measured before any code was written

| Measurement | Result |
| --- | --- |
| the accepted route already re-reads a grown source | converting a *copy* of the committed Claude fixture, appending a turn, and converting again produced two rollouts in one home (`01a1e92e-3604-…`, `01a1e92e-4345-…`), the later with `conversation: 8 → 10 message(s)` and `converted: 5 → 7 message(s)`, the earlier untouched, both reporting `read from: source` |
| what a result said about *where* its snapshot stopped | nothing structured: the source file's sha256 appeared only inside `installation.detail` ("sha256 …, taken before and after the conversion"), and the read's own boundary had to be inferred from `counts`/`ended_at`/`fidelity` |
| the inherited Claude reader on an unterminated trailing record | counts it as malformed **and** flags the session truncated (`internal/parser/claude.go`: `isTruncated := lastLineHasData && !lastLineValid && !fileEndsWithNewline`) |
| the inherited Codex reader on the same file | **counts nothing at all** (`internal/parser/codex.go`: *"A writer may leave its final JSON record incomplete while the session is live. Count terminated malformed records, and count an unterminated tail only when the caller owns a stable snapshot"*). Measured end to end: a conversion of a Codex source whose file ended in a half-written record reported `conversation: 6 message(s)` and **no fidelity line, no loss category and no warning** — the record was dropped silently |
| what the target harness records about a resumed session | a real turn appends the harness's own records to the session file (measured twice here: `3970 → 70068` and `4920 → 71478` bytes), so the earlier target is *expected* to move once its own harness resumes it — which is why the comparison this increment adds is taken around the second conversion, after that turn |

The gap the increment closes is therefore exact: the repeat *behaviour* mostly existed, the *information*
that makes two snapshots distinguishable did not, and for a Codex source captured mid-append the read
said nothing at all.

## 1. The source boundary on a real conversion

A conversion of the committed Claude fixture into an isolated Codex home (`<repo>/bin/ccc continue
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex --target-home /tmp/evidence-0004/r3/a3/home
--target-binary ccc-no-such-harness --verify none`, `CLAUDE_PROJECTS_DIR=<repo>/internal/ccc/testdata/fixtures/claude`,
exit code 0):

```console
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → codex
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2051 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2051: the file ends at a record boundary
  converted:    codex-rollout-jsonl, 3101 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: pre_compaction_history ×3
  omitted:      malformed_source_lines ×1
  installed:    /tmp/evidence-0004/r3/a3/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e98b-7682-7104-95a2-8641d4e883ee.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e98b-7682-7104-95a2-8641d4e883ee
  working dir:  /home/user/code/example-api
  resume:       CODEX_HOME=/tmp/evidence-0004/r3/a3/home ccc-no-such-harness resume 01a1e98b-7682-7104-95a2-8641d4e883ee
  non-interactive resume: CODEX_HOME=/tmp/evidence-0004/r3/a3/home ccc-no-such-harness exec resume 01a1e98b-7682-7104-95a2-8641d4e883ee '<prompt>'
  cli_version:  0.169.0 (writer-pin)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  not-requested (the target harness's own loader (codex app-server, thread/resume))
                verification was not requested (--verify none)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                verification was not requested (--verify none)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The same run with `--json` (an isolated second run, `/tmp/evidence-0004/r3/a4`), with the boundary, the
source comparison and the target identity quoted from the document itself:

```json
{
  "source": {
    "counts": {"messages": 8, "user_messages": 3, "assistant_messages": 5, "system_records": 1,
               "compact_boundaries": 1, "tool_calls": 1, "tool_results": 1},
    "fidelity": {"malformed_lines": 1, "source_version": "1.0.30", "read_from": "source"},
    "boundary": {
      "state": "taken",
      "detail": "the conversation's own source file, read after the session and before anything was written",
      "path": "<repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl",
      "bytes": 2051,
      "sha256": "2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6",
      "read_bytes": 2051,
      "read_coverage": "size-match",
      "read_coverage_detail": "the read's own snapshot of this file is 2051 byte(s) and the file holds 2051: the file is the size the read recorded, which is a size match and not a byte-identity check -- the read's own record holds no digest of the bytes it opened, so a same-length replacement of the file after it is not ruled out",
      "record_offsets": "claimed",
      "terminated_records_end_bytes": 2051
    }
  },
  "installation": {
    "state": "installed",
    "source_check_state": "unchanged",
    "detail": "sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion"
  },
  "target_session": {
    "session_id": "01a1e98b-76a3-7667-8465-9154b85cadb9",
    "path": "/tmp/evidence-0004/r3/a4/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e98b-76a3-7667-8465-9154b85cadb9.jsonl",
    "resume_command": "CODEX_HOME=/tmp/evidence-0004/r3/a4/home ccc-no-such-harness resume 01a1e98b-76a3-7667-8465-9154b85cadb9"
  }
}
```

*(Both blocks are this round's own re-run on the fixed tree — `/tmp/evidence-0004/r3/a3` and `a4`, exit code
0 each — re-taken because the read-coverage answer's words changed in this round (§12's finding 1: the state
is a size match, not a byte-identity claim). The earlier spellings of these two runs stood as
`read_coverage: verified` with "the conversion read these bytes"; §11's own records keep the wording of the
round that wrote them, and §12 names what changed.)*

`unterminated_tail_bytes` is absent because it is zero (omitempty), which is the field's own meaning: the
file's bytes end at a line feed, so it has no unterminated record. (`record_offsets_detail` is absent for
the mirror reason: the offsets *were* claimed, so there is no reason to state.) The fixture's last line *is*
malformed (invalid JSON) — a complete but unparseable record — and that is what `fidelity.malformed_lines`
and the `malformed_source_lines ×1` loss count, separately from the boundary: a terminated bad record is a
skipped record, not a record the file had not finished writing.

`ccc help` states the same rule where a caller learns the command (exit code 0):

```console
  A conversion is a snapshot of the source at one moment: it reads the source as it is and writes a new
  session, so converting the same source again after it has grown writes another new session -- never a
  merge into one that an agent may already have continued, and never a rewrite of the source. The result
  names the boundary each snapshot covers (the source file's sha256 and size, whether the file is still the
  size the read's own snapshot recorded -- a size match, which is what a size can show -- and, for a source
  whose records are the lines a harness appends, where its last line-feed-terminated record ends), so two
  continuations of one growing session can be told apart and the newer one chosen. A file that ends without
  a line feed holds a record the source may still be writing: the 'records:' line and the run's warnings
  state those bytes, and leave what the read made of them to the read's own fidelity, so nothing the read
  did not read is presented as history. A source whose own bytes are a compressed container of the records
  -- Codex reads its compressed rollouts -- is reported by its bytes and digest with no record offset claimed
  for them, because no offset into such a file is a record boundary.
```

*(The paragraph as the final tree prints it (this round's own `ccc help`, exit code 0): the apply stage's
accepted findings qualified its last sentence to a source whose records are appended lines — §7 — and this
round replaced the label it names for an unfinished record with the one the block prints it on, and the
coverage clause with the size match the boundary answers (§12's findings 1 and 3).)*

## 2. Two conversions of one growing source, by hand

A copy of the committed Claude fixture under `/tmp/evidence-0004/r3/b/src`, converted, grown by the committed
fragment `internal/ccc/testdata/growth/claude-complete-turn.jsonl`, then converted again into the same
home — the scenario the goal's first clause names (exit code 0 for each run). This block is one run's own
transcript, quoted as the evidence-bearing subset of the two conversion blocks it ran (§12's finding 2
re-took it, because an earlier revision quoted the first conversion's identity from one run beside the
second's from another): B1's `installed:`/`target id:` lines and B4's `first installed now:` name the same
file, `01a1e98b-76e1-…` (`rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl`), and
B4 digests it after the second conversion with the same `sha256=3971fbc722c190ee` it had when B1 wrote it;
the home B4 lists holds exactly the two sessions B4's two lines name, and B5's two counts are of those two
documents. The B0 and B2 lines join the size that run's `stat` printed with the digest its `sha256sum`
printed — the block's own long-standing shape — and every other quoted line is the run's, character for
character.

```console
### B0: the growing copy before the first conversion
bytes=2051 sha256=2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6

### B1: the first conversion
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2051 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2051: the file ends at a record boundary
  converted:    codex-rollout-jsonl, 3101 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  installed:    /tmp/evidence-0004/r3/b/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e98b-76e1-7475-ba0e-80c959cb5f5d
first installed: rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl sha256=3971fbc722c190ee

### B2: the source gains one turn (the committed growth fragment)
bytes=2540 sha256=e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f

### B3: the second conversion of the same session
  conversation: 10 message(s), 4 user, 6 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2540 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2540: the file ends at a record boundary
  converted:    codex-rollout-jsonl, 4051 bytes: 7 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 3 turn(s)
  installed:    /tmp/evidence-0004/r3/b/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e98b-7709-7ecf-9abe-2c984d6846d3.jsonl [hard-link]
  source file:  unchanged (sha256 e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f, taken before and after the conversion)
  target id:    01a1e98b-7709-7ecf-9abe-2c984d6846d3

### B4: the earlier target after the second conversion
first installed now: rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl sha256=3971fbc722c190ee
  rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl
  rollout-2026-05-04T09-15-00-01a1e98b-7709-7ecf-9abe-2c984d6846d3.jsonl

### B5: the appended turn's words in each installed document
  rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl: question=0 answer=0
  rollout-2026-05-04T09-15-00-01a1e98b-7709-7ecf-9abe-2c984d6846d3.jsonl: question=2 answer=2
```

B4's first line is the *same file* B1 installed — `01a1e98b-76e1-…`, the one B1's own `installed:` and
`target id:` lines name — digested again after the second conversion ran into the same home: the same
sha256, `3971fbc722c190ee`, it had when B1 wrote it. The home holds exactly the two sessions B4 lists.

Read as the four claims it is meant to support:

| Claim | Evidence in the block above |
| --- | --- |
| two conversions of one growing source are **distinguishable** | two sessions in one home, `01a1e98b-76e1-…` then `01a1e98b-7709-…`, each with its own path; two different boundaries (`2051` then `2540` bytes, `2bd51082…` then `e2921e99…`) |
| each is **correctly attributed** to its own bytes | B0's digest is B1's `boundary` digest and B1's `source file` digest; the copy's own digest after the growth (B2) is B3's `boundary` digest; the later read carries 10 messages against the earlier 8; both blocks state the read's own snapshot as a size match against the file they measured (`read_coverage: size-match`, which is what a size can show — §12's finding 1) |
| the later result **includes the eligible appended history** | B3's `conversation: 10 message(s)`/`converted: 7 message(s)`/`3 turn(s)` against B1's 8/5/2, and B5: the appended question and answer appear twice in the second document and not at all in the first |
| the earlier target **stays unchanged** | B4: `sha256=3971fbc722c190ee` for `01a1e98b-76e1-…`, the same digest that file had immediately after B1, after the second conversion ran into the same home |
| the source is read again and never written | each run's `source file: unchanged (sha256 …, taken before and after the conversion)` names the digest it compared; the copy's bytes changed only where B2 appended the fragment |

The same property is pinned in-process for **both** target routes by
`TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot` (`internal/ccc/continue_growth_test.go`),
whose two subtests convert into Codex and into Claude Code; it additionally asserts that the first
conversion's installed file's digest is unchanged after the second, that the two session ids and paths
differ, and that the appended turn's words are absent from the earlier document (§4).

## 3. The incomplete tail: a source captured while it was still being written

Two controlled cases, one per accepted source provider, with the *unterminated* fragment appended to a copy
of the committed fixture: `internal/ccc/testdata/growth/claude-unfinished-record.jsonl` (131 bytes) and
`internal/ccc/testdata/growth/codex-unfinished-record.jsonl` (165 bytes). Both fragments are byte prefixes
of a record the harness writes — the shape a capture mid-append has — and both end without a line feed.

```console
### C1: a Claude source whose writer had not finished a record
the copy: 2182 bytes (the fixture's 2051 and the fragment's 131)
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     2 malformed line(s), and the last record was still being written at read time
  boundary:     2182 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 2051: the last 131 byte(s) are a record the source had not terminated
  omitted:      malformed_source_lines ×2
  installed:    /tmp/evidence-0004/r3/c/claude/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e98b-773d-7d0f-afc9-cc08bdaf68c2.jsonl [hard-link]
  source file:  unchanged (sha256 95ce6c56aee411c05bd48dfbf966e63bd219a42295b6178ec83890a9482d9b3f, taken before and after the conversion)
  target id:    01a1e98b-773d-7d0f-afc9-cc08bdaf68c2
  warning:      the source file's last 131 byte(s) hold no line feed: its bytes end at byte 2182 while its last line-feed-terminated record ends at byte 2051, so the source may still be writing that record.
  the unfinished record's words in the installed document: 0

### C2: a Codex source whose writer had not finished a record
the copy: 1957 bytes (the fixture's 1792 and the fragment's 165)
  conversation: 6 message(s), 2 user, 4 assistant, 1 tool call(s), 1 compaction boundary(ies)
  boundary:     1957 byte(s) read; the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)
  records:      the source's records end at a line feed at byte 1792: the last 165 byte(s) are a record the source had not terminated
  omitted:      message_model_attribution ×5
  installed:    /tmp/evidence-0004/r3/c/codex/home/projects/-home-user-code-example-api/93ff4b0d-acfb-4504-ab86-321ad0ee93a1.jsonl [hard-link]
  source file:  unchanged (sha256 aa19c70a9c0042786070d0891f88f58c21fa678f7ccc73aa62bc48f6e553bbb3, taken before and after the conversion)
  target id:    93ff4b0d-acfb-4504-ab86-321ad0ee93a1
  warning:      the source file's last 165 byte(s) hold no line feed: its bytes end at byte 1957 while its last line-feed-terminated record ends at byte 1792, so the source may still be writing that record.
  the unfinished record's words in the installed document: 0
  the same source read by ccc inspect reports:
    {"fidelity": {"read_from": "source"}, "messages": 6, "file_size": 1957}
```

*(Both blocks are this round's own re-run on the fixed tree (`/tmp/evidence-0004/r3/c`, exit code 0 each),
re-taken because the boundary's read-coverage clause changed (§12's finding 1). Each `warning:` line is
quoted through its first sentence — the rest names the read's own answer and the remedy, and stands in full
in §11's F02 — the `the copy:` lines state the fixture's own size beside the appended fragment's, where the
run printed the grown copy's, and the `ccc inspect` line under C2 is that same copy read by a second command
of the same run (`ccc inspect codex:0c8e5f21-… --json`, exit code 0), quoted as it printed; no other line is
altered. The earlier revision quoted the pre-fix wording, `the read's own snapshot of this file is exactly
these bytes`.)*

| What the case shows | C1 (Claude source) | C2 (Codex source) |
| --- | --- | --- |
| the reader's own answer | `fidelity: 2 malformed line(s), and the last record was still being written at read time`, `malformed_source_lines ×2` | **nothing**: no fidelity line, no loss category, no warning — `ccc inspect` of the same file reports `"fidelity": {"read_from": "source"}` with 6 messages |
| the boundary's answer | `the source's records end at a line feed at byte 2051` of 2182, `the last 131 byte(s) are a record the source had not terminated` | `the source's records end at a line feed at byte 1792` of 1957, `the last 165 byte(s) are a record the source had not terminated` |
| did the unfinished record become history? | no: its words occur 0 times in the installed document, and the conversation's message counts are the complete file's (8/3/5/1/1) | no: its words occur 0 times in the installed document, and the conversation's message counts are the complete file's (6/2/4/1/1) |
| is the source written? | no: `source file: unchanged (sha256 95ce6c56…, taken before and after the conversion)` | no: `source file: unchanged (sha256 aa19c70a…, taken before and after the conversion)` |

The row that matters for the goal is the second one's *first* cell: the Codex source — the 0002 route's own
source side — says nothing about the tail it dropped, so the *boundary* is what discloses it there. That is
why the boundary is computed from the file's own bytes rather than from a provider's classification: the
same rule answers for both readers, and the read's own fidelity answers beside it wherever its provider
supplies one.

Both cases are also pinned in-process, per provider, by
`TestUnterminatedSourceTailIsBoundaryedAndNeverPublished`, which additionally compares the captured run's
carried evidence with a conversion of the same copy *before* the tail was appended: message, user,
assistant, tool-call and tool-result counts are identical, so the unfinished record contributed nothing —
and it asserts that no warning claims the record was left out of the conversion (§11's F02).

## 4. The boundary's own cases and the states it can be in

| Case (verbatim test name) | What it pins |
| --- | --- |
| `TestSourceBoundaryOfFileStatesTheRecordOffsets` | the pass itself: two complete lines (`terminated_records_end = 16`, tail 0), an unterminated last line (`terminated_records_end = 8` of 13, tail 5), a complete record without its line feed (8 of 15, tail 7), a file with no line feed at all (0, the whole 7 bytes unterminated), an empty file (both 0), a multi-byte rune the pass's own read buffer splits (still text: the offsets are claimed), a source whose records are *not* declared appended lines (the file's bytes and digest are still measured, `record_offsets: not-claimed` with the declaration as the reason), a compressed container (its zstd header is not text: `not-claimed` with the container as the reason, no offset, no unterminated record), and a path that names no file (the error itself — `os.ErrNotExist`, asserted by kind so the case holds on every platform — never a boundary with no bytes in it) |
| `TestTextStreamCarriesSplitRunes` | the validator the gate's second half rests on: a rune split across chunks is decided by the chunk that completes it (a two-byte and a four-byte rune, and the encoded rune error), while a stray continuation byte, a zstd frame header, and a rune no chunk completes are not text |
| `TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure` | `not-taken` with its own reason for an archive read (`the conversion read an archive row, so no source file was open at all`) and for a container-member identity (`…member member-1 inside the container…`), with no bytes claimed |
| `TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead` | the state a mid-read growth leaves, at the seam where it is visible (§11's F01): the read's own recorded snapshot is smaller than the file the pass measures, so the block reports `read_bytes`, `read_coverage: size-mismatch` with both sizes in its detail, the file's own record structure beside it, and a warning naming the remedy — and the pass writes nothing to the file |
| `TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity` | the limit of the read-coverage answer (§12's finding 1): a file replaced by *different bytes of the same length* after the read leaves the sizes equal, so the pass reports `read_coverage: size-match` with both sizes, the digest of the file as it stands, the detail's own limit (`not a byte-identity check`, `a same-length replacement … not ruled out`), no warning, and neither the pre-fix `verified` state nor the words "the conversion read these bytes" — measured red against the pre-fix code, which printed both |
| `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile` | the warning channel: no boundary, a boundary that was not taken, and a file with neither an unterminated record nor a move produce no warning; an unterminated record produces one naming `1835`, `1792`, `43 byte(s)` and `converting again` and claiming no omission; a size mismatch produces one naming both sizes (`1792 byte(s) read`, `1835 byte(s) now`) and the remedy; both answers together produce one each |
| `TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations` | the *declaration* half of the gate on the record offsets: `claude` and `codex` are declared line-recorded (`parser.SourceCapabilities.IncrementalAppend`), an unknown name and an empty one are not, and a reader-less call claims nothing. Three providers whose source is a whole-file JSON document or a compressed container (QwenPaw, CodeBuddy, DeepSeek Harness — each declaring only `ContentCapabilities.MalformedLineCount`) are not (§10: the review round found the gate accepting that count and superseding `Q-0004-01` with `Q-0004-07`) |
| `TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary` | end to end (added in the review round, §10): a complete QwenPaw session — one whole-file JSON document, no trailing line feed — converts with `record_offsets: not-claimed` (the provider declares no such source), no record offset, no unterminated record and no unterminated-record warning, and the boundary digest is the file's own |
| `TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets` | end to end (§11's F03): a runtime-compressed copy of the committed Codex rollout — Codex's own `.jsonl.zst` representation, the one its reader decodes — converts with the same conversation the plain rollout of the same bytes produces, while the boundary reports the compressed file's own size and digest, `record_offsets: not-claimed` with the container as the reason, no record offset, no unterminated record, and `read_coverage: size-match` |
| `TestContinueCarriesACompleteRecordThatHasNoLineFeed` | end to end (§11's F02), now in both directions (§13): each direction is one accepted source converted into the other harness — the Claude fragment into Codex, the Codex fragment into Claude — and each appends its provider's own committed growth fragment as committed and with its last byte (its line feed) removed. Both halves produce the same conversation and the same carried counts — the reader reads a complete unterminated record — while only the second boundary reports an unterminated record and its installed document holds the record's dialogue; the search text is checked to sit in the fragment's **last** record, the one the case leaves unterminated, so a case that searched a terminated record could not stand in for it |
| `TestConversionFromAnArchiveReportsNoBoundaryToTake` | end to end: a conversion that fell back to the archive reports `not-taken`, no bytes, `source_check_state: not-verified` with the archive reason, and no unfinished-record warning |
| `TestContinueJSONCarriesTheSourceBoundary` | the CLI's `--json` document: `state`, `path` (the file the conversion read), `bytes` (the file's own size), `sha256` (an independently computed digest), `read_bytes`/`read_coverage: size-match` (the file is the size the read's own snapshot recorded — a size match, not a byte-identity check), `record_offsets: claimed` with no reason, `terminated_records_end_bytes`, and a non-empty `detail` |
| `TestContinueHumanOutputStatesTheSourceBoundary` | the human block, on a real conversion and on the five shapes (complete, unterminated record, compressed container, moved file, not-taken, undeclared source), including that a boundary that claims no offsets prints no `records end at` line, that the unterminated case claims no omission, and that a report with no boundary prints no `boundary:` line at all |
| `TestFixturesMatchTheirGenerator` | the committed fixtures **and** the growth fragments are byte-for-byte what `internal/ccc/fixturegen` writes, so a fragment cannot drift from the shape the runs above appended |

All of them pass (`--count=1`, exit code 0):

```console
--- PASS: TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot (0.19s)
--- PASS: TestUnterminatedSourceTailIsBoundaryedAndNeverPublished (0.03s)
--- PASS: TestContinueCarriesACompleteRecordThatHasNoLineFeed (0.03s)
--- PASS: TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets (0.02s)
--- PASS: TestSourceBoundaryOfFileStatesTheRecordOffsets (0.00s)
--- PASS: TestTextStreamCarriesSplitRunes (0.00s)
--- PASS: TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure (0.00s)
--- PASS: TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead (0.00s)
--- PASS: TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity (0.00s)
--- PASS: TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile (0.00s)
--- PASS: TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations (0.00s)
--- PASS: TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary (0.01s)
--- PASS: TestConversionFromAnArchiveReportsNoBoundaryToTake (0.15s)
--- PASS: TestFixturesMatchTheirGenerator (0.10s)
PASS
ok  	go.kenn.io/agentsview/internal/ccc	0.561s
--- PASS: TestContinueJSONCarriesTheSourceBoundary (0.18s)
--- PASS: TestContinueHumanOutputStatesTheSourceBoundary (0.01s)
--- PASS: TestContinueHelpIsARequestNotAFailure (0.01s)
PASS
ok  	go.kenn.io/agentsview/cmd/ccc	0.235s
```

*(§13's re-take: this round's own run of the cases above, with `-run` naming them, exit code 0, its
per-subtest lines elided exactly as the round before elided its own. Timing varies per run; the names, the
counts and the verdicts do not.)*

## 5. What the boundary and the repeat carry, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| the source file's own bytes at the boundary (`2051` bytes, `sha256 2bd510821d4abf83…`) | `source.boundary` in the conversion's JSON and the `boundary:` line in its human block; the same digest in `installation.detail` (§1) |
| the appended turn (a committed growth fragment) | present twice in the second Codex rollout and absent from the first — §2's B5 counts two matches of the appended question in the later document and the same two for the words that replied to it, against none of either in the earlier one; in-process, the same holds for the Claude Code target (`TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot`). The turn exists once per accepted source provider: `growth/claude-complete-turn.jsonl` in every run here, and `growth/codex-complete-turn.jsonl` as the same question and reply in Codex's own record shape (§13), which is what lets the no-line-feed rule be pinned from a Codex source as well |
| the appended fact, as the **answer** of a real turn in the *later* target | the recorded turn's answer holds it: the second answer block quotes `120 seconds per session, held in` (§6) |
| the converted history before the growth, as the **answer** of a real turn in the *earlier* target | the recorded turn's answer names `maxBackoffSeconds` as the fact the converted history supplied, in the first answer block (§6) |
| an unterminated trailing record (`165` bytes with no line feed) | `boundary.unterminated_tail_bytes`, the `records:` line and the run's warning; no message of the conversation and no record of either target document holds its text (§3) — and where the record *is* a complete one, the conversion carries it while the boundary still states the byte fact (§11's F02) |
| an earlier conversion of the same source | unchanged by the second conversion (`sha256 3971fbc722c190ee` before and after, §2 B4, taken of the same file B1 installed) — and unchanged in the native run after the first session had itself been resumed by Codex (§6) |
| the source file during and after every run | `source file: unchanged (sha256 …, taken before and after the conversion)` in each CLI run (§1–§3); in the native check, the copy and the committed fixture tree are digested around each conversion, the copy is compared with the bytes it had before the first conversion **immediately before it grows** — so the baseline every later comparison measures from is the bytes that conversion read, across its loader and its optional turn (§13) — and the copy, the fixture tree and the growth fragment are compared once more at the end of the run, after the second loader and the optional turn (§6, §10, §13) |

The two "**answer**" rows are the ones this record cannot re-derive by re-running the *offline* half: they
are the target model's own words, recorded verbatim below and in
`scripts/codex-native-check.py`'s answer blocks, and the harness's case that keeps a record from quoting an
answer it did not make reads exactly those blocks
(`scripts/tests/test_evidence_claims.py`, which discovers this file by name and requires it to credit the
run).

## 6. The accepted 0002 route through the repeated scenario, natively

`scripts/codex-native-check.py` (the route's own native verification, **not** a gate step: it needs the
Codex binary and, for a turn, model access), run on this host three times: once with no prompt (the offline
half), and twice with both prompts. All three exit 0.

**The offline run** (`python3 scripts/codex-native-check.py`): `26 check(s) passed, 0 failed, 2 skipped`.
Its header and the checks that make the clause material:

```console
source:      internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
copy:        source/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl (the fixture's bytes; this is what grows)
growth:      internal/ccc/testdata/growth/claude-complete-turn.jsonl (489 bytes)
PASS: the conversion reports the boundary of the source copy it read — boundary={"state": "taken", … "bytes": 2051, "sha256": "2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6", "read_bytes": 2051, "read_coverage": "size-match", … "record_offsets": "claimed", "terminated_records_end_bytes": 2051}; 9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl is 2051 bytes, sha256 2bd510821d4abf83…
PASS: the source copy is unchanged by the conversion — sha256 2bd510821d4abf83…, 2051 bytes, taken before and after
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e9a1-b50f-7032-bed6-a5e2d91b2523 [legacy] cwd=/home/user/code/example-api
PASS: the source copy still holds the bytes the first conversion read — sha256 2bd510821d4abf83…, 2051 bytes, against the 2bd510821d4abf83…, 2051 bytes taken before the first conversion
PASS: the source copy grew by exactly the appended turn — 2051 -> 2540 bytes (+489 from internal/ccc/testdata/growth/claude-complete-turn.jsonl), sha256 e2921e995915855d…
PASS: the second conversion reports the boundary of the grown source copy — boundary={"state": "taken", … "bytes": 2540, "sha256": "e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f", "read_bytes": 2540, "read_coverage": "size-match", … "record_offsets": "claimed", "terminated_records_end_bytes": 2540}; …; the first conversion's boundary was {"state": "taken", …}
PASS: the second conversion's identity is new, not the source's and not the first session's — 01a1e9a1-b808-7055-be63-ffa200f2807b at sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e9a1-b808-7055-be63-ffa200f2807b.jsonl
PASS: the second conversion carries the history the source gained — carried user messages 2 -> 3, assistant messages 3 -> 4; the appended turn's own words are in the second document and not in the first
PASS: the first conversion's session is unchanged by the second — sha256 c633cfc95c369048… before and after the second conversion
PASS: the source copy is unchanged by the second conversion — sha256 e2921e995915855d…, 2540 bytes, taken before and after the second conversion
PASS: the target harness's own loader opens the second installed session — thread/resume -> 01a1e9a1-b808-7055-be63-ffa200f2807b [legacy] cwd=/home/user/code/example-api
PASS: the target's own view of the second session lists the appended turn — 7 item(s) in the reply's own turn list; the appended turn's own words are in its own view
PASS: the first session's own view does not hold the appended turn — the first session's own view holds 5 item(s), none of them the turn the source gained after it was converted
conversions: 2 session(s) installed (01a1e9a1-b50f-7032-bed6-a5e2d91b2523 then 01a1e9a1-b808-7055-be63-ffa200f2807b); real turn(s) run: 0
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical; the converted copy identical too: sha256 e2921e995915855d…, 2540 bytes)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
26 check(s) passed, 0 failed, 2 skipped
```

*(The block above is re-taken from this round's own re-run of the offline half on the tree §13 fixed: the
same checks §12's block quoted plus the comparison taken where the copy grows, `26 check(s) passed, 0
failed, 2 skipped`, the same committed fixture and fragment digests, and this run's own session ids. Each
boundary JSON is quoted with `…` where its own `detail`, `path` and `read_coverage_detail` fields (and, on
the second line, the per-file restatement that follows) are elided; every other character is what the run
printed. §10 records why the final comparison grew; §11 and §12 record what changed then, and §13 what
changed now.)*

Codex's own `thread/read` view of the *second* session, printed by the offline run re-taken above (its own
view of the later snapshot, with the turn the source gained as its last turn):

```console
the target harness's own view of the second installed session:
    thread 01a1e9a1-b808-7055-be63-ffa200f2807b [legacy] provider=openai
      turn rollout-2
        userMessage: Summarize the retry policy in internal/retry/policy.go.
        agentMessage: I will read the retry policy first.
        agentMessage: Four attempts with exponential backoff, capped at 30s between attempts.
      turn rollout-11
        userMessage: Now check where the backoff cap is configured.
        agentMessage: The cap is configured in internal/retry/config.go as maxBackoffSeconds.
      turn rollout-15
        userMessage: What budget do retries share per session?
        agentMessage: The retry budget is 120 seconds per session, held in internal/retry/budget.go.
```

**The run with both real turns** (`python3 scripts/codex-native-check.py --model-provider tencent-litellm
--model deepseek-v4.1-flash-fast --resume-prompt "Which file configures the backoff cap, and what is the
cap set to?" --repeat-resume-prompt "What budget do retries share per session, and which file holds it?"`):
`27 check(s) passed, 0 failed, 0 skipped`, exit 0, two turns billed (the run's own line says so). §13 did
not re-run the turned halves: each is two billed turns, so a turned run of this tree would print
`28 check(s) passed, 0 failed, 0 skipped` — arithmetic from the offline count §13 measured plus this
block's own two extra passes, stated as arithmetic rather than quoted as a run. The comparison §13 added
sits between the two turns: it is taken after the first conversion's loader and its own real turn, and
immediately before the copy the second conversion reads grows — hence ahead of the second conversion, its
loader and its turn (§15 re-derived that ordering from the shipped script and pinned both of its ends).
The turn
in the **first** session ran *before* the growth and the second conversion, so that session is one its own
harness had already resumed when the second conversion happened:

```console
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 70068 bytes

the turn's own output (the target's answer, as the harness printed it):
    The cap is configured in `internal/retry/config.go` as `maxBackoffSeconds`. I haven't read that file yet, so I can't state the value.
    
    Want me to open it and report the actual number?
```

and the turn in the **second** session, after the source grew and was converted again:

```console
PASS: the first conversion's session is unchanged by the second — sha256 9ee7ce8b54cb82b5… before and after the second conversion
PASS: the target harness's own loader opens the second installed session — thread/resume -> 01a1e937-b1bb-7036-983e-2f6878c65674 [legacy] cwd=/home/user/code/example-api

PASS: a real turn continues from the second installed session — exit 0, session 4920 -> 71478 bytes

the turn's own output (the target's answer, as the harness printed it):
    The retry budget is 120 seconds per session, held in `internal/retry/budget.go`.
    
    One flag: I have not actually read that file or `internal/retry/policy.go` in this session — those earlier answers were stated as fact without verification. If you want, I can check the repo now and confirm the actual cap, attempt limit, and budget.

conversions: 2 session(s) installed (01a1e937-a827-7890-a2e5-71a8e5660678 then 01a1e937-b1bb-7036-983e-2f6878c65674); real turn(s) run: 2
```

What that pair shows, in the goal's own terms: **the later result includes the eligible appended history**
— a fact that appears in no earlier turn (*who* appended it was the growth fragment: the appended turn is
in the target's model-visible history, which is why the second session's turn answers with it) — and the
**earlier target stayed unchanged** even though its own harness had resumed it (`9ee7ce8b54cb82b5…` before
and after the second conversion). **The source remained unchanged**: the copy is digested around each
conversion — and compared with the bytes it had before the first conversion immediately before it grows, so
the baseline the run's own closing verdict measures from is the bytes that conversion read (§13) — the
committed fixture tree and the growth fragment are compared at the end
(`fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical`), and the
planted unrelated session plus every session the home already held are compared last, after both loaders
and both turns (`an ordering the harness's own cases pin offline`).

**The second turned run** (the same command, after two follow-up edits that changed no line quoted above:
an unused type removed from the script and the first-session view check made to require an own view):
`27 check(s) passed, 0 failed, 0 skipped`, exit 0, two turns billed, `conversions: 2 session(s) installed
(01a1e93e-f79b-7131-911f-abfc221712cc then 01a1e93f-09ab-714d-b106-e3835fb7c7eb); real turn(s) run: 2`. Its
second session's turn answered the appended fact **again** — a different session, a different run, the same
fact that exists in no earlier turn:

```console
PASS: a real turn continues from the second installed session — exit 0, session 4920 -> 75465 bytes

the turn's own output (the target's answer, as the harness printed it):
    The retry budget is 120 seconds per session, held in `internal/retry/budget.go`.
…  (this record's elision: the block continues with the model's caveat about not having read the files in
    this session; no line above is altered)
```

while its **first** session's turn spent its turn on the workspace instead of the conversation, and said so:

```console
PASS: a real turn continues from the installed session — exit 0, session 3970 -> 86975 bytes

the turn's own output (the target's answer, as the harness printed it):
    Every shell command is failing with the same sandbox error before it can execute:
    bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted
…  (this record's elision: the block continues with the fenced error text and the model's report that it had
    no verified answer; no line above is altered)
```

That contrast is the limit 0002's record already states and this one repeats rather than hides: the check's
own verdict is about whether the target harness **loads** the session and whether its session **grew** — it
cannot judge an answer's words, and a model that reaches for a tool it cannot run will answer about that
instead. What the fixture facts show is answered *by this record*, from the recorded blocks above: the run
this record quotes is the one whose answers named the converted facts, and the re-run's second answer —
the one that matters for the repeat clause — named the appended fact too.

Its offline cases (`scripts/tests/test_codex_native_check.py`, run by the `selftest` step) drive the
shipped `main` with the app server, the subprocess runner and the tool lookup replaced by stubs, and they
pin the rules this half stands on:

| Case | What it pins |
| --- | --- |
| `test_an_intact_run_passes_the_comparisons` | every comparison is present and `PASS` (14 names in `REPEAT_CHECKS`), the only `SKIP`s are the turns nobody asked for, and the comparisons that measure a file name the bytes they compared (`the first conversion's boundary was`, `before and after the second conversion`, `taken before the first conversion`) |
| `test_a_mutation_during_the_resume_stage_is_caught_by_the_last_comparison` | the three protected-artifact comparisons run last and fail exactly then; the repeat half still passes, because the conversions read the copy rather than the mutated fixture |
| `test_a_first_session_mutated_by_the_second_conversion_is_caught` | the comparison about the earlier session runs **after** the second conversion: a second conversion that touched it fails exactly that comparison |
| `test_a_copy_mutated_during_the_first_conversion_is_caught_by_two_comparisons` | a conversion that wrote to its own source fails exactly the two comparisons whose interval covers that moment (its own stage's, and the growth baseline the changed bytes become) |
| `test_a_copy_mutated_during_the_first_load_is_caught_where_the_copy_grows` | the comparison taken where the copy grows, and only it (§13): a *same-length* change made between the first conversion's comparison and the growth leaves the growth record's own size delta and digest-differs clause green, so a run without that comparison adopted the changed bytes as every later comparison's baseline |
| `test_a_copy_mutated_during_the_second_conversion_is_caught_by_two_comparisons` | a second conversion that wrote to its own source fails exactly the two comparisons that hold the digest taken before it, while the attribution comparison stays green (the report describes the file as it stands) |
| `test_a_copy_mutated_during_the_second_load_is_caught_by_the_final_comparison` | the final `the source session is unchanged` covers the copy after the second conversion and after the second loader (§10) |
| `test_a_second_boundary_that_is_not_the_grown_copy_fails` | the boundary is checked against the file (a wrong `sha256`, a wrong `bytes` and a `not-taken` state each fail that comparison, and the *first* conversion's boundary comparison stays green) |
| `test_a_second_document_without_the_appended_turn_fails_the_search` | the search for the appended turn is live in both directions: leaving it out of the second document and out of the target's own view fails exactly the two comparisons that look for it |

## 7. Tests, gate, docs and bookkeeping

- **Gate: no new step, no new required step, no narrowing.** The Go cases live in `internal/ccc` and
  `cmd/ccc`, which `scripts/gate-targets.txt` already builds and runs; the native check stays a script;
  the new Python cases run inside the existing `selftest` step.
- The run of `python3 scripts/gate.py` and its verdict are quoted with their counts in
  [§7.1](#71-the-gates-own-output) below.
- **Docs changed in this increment**, each for the reason stated:
  - `AGENTS.md`: the binding bullet on fresh identities gained the repeat rule (a conversion is a snapshot
    of the source at one boundary; repeating it writes *another* fresh session and never merges into an
    existing one; the boundary every result states; an unfinished record is disclosed and never published)
    and the layout line now names `internal/ccc/testdata/growth/`; the target-writer bullet says the Codex
    native check also runs the repeat scenario. This is a change to rules an agent must follow and to the
    repository layout, so `AGENTS.md` had to move with it.
  - `docs/conventions.md`: the same rule as a rule of the work, plus the two new `testdata/` trees in the
    layout block.
  - `docs/harness.md`: the `scripts/codex-native-check.py` entry states the copy-not-fixture rule, the
    growth fragment, the repeat scenario's checks, `--repeat-resume-prompt`, and what the new offline cases
    pin.
  - `README.md`: one paragraph — a conversion is a snapshot, converting again writes another session, and
    an unfinished trailing record is reported rather than published.
  - `cmd/ccc`: the `ccc help` page gained the paragraph quoted in §1, pinned by
    `TestContinueHelpIsARequestNotAFailure`.
  - `docs/goals/README.md`: 0003 archived (merged onto `main` as `18d50b2`) and 0004 marked
    `Implemented on feat/0004-repeatable-evolving-session-handoffs; awaiting operator acceptance`. The goal
    file moved to `docs/goals/archive/0003-codex-to-claude-continuation.md` unchanged, and the three links
    that named its former location (0003's plan, its evidence record and its obligation matrix) were
    retargeted.
  - `docs/decisions/OPEN-QUESTIONS.md`: the defaults this run had to take alone are appended
    (`Q-0004-…`).
  - **Applied simplify findings (the stage after this increment's implement-and-verify round)**:
    `ccc help`, `README.md`, `AGENTS.md` and `docs/conventions.md` now state the unfinished-record
    disclosure's own condition — a source whose records are appended lines, which is the gate in the code
    (`boundaryWarnings`/`sourceRecordsAreLines`) and the narrowing `Q-0004-01` and §9 already disclose —
    and the §1 help quote is re-taken from that tree; `docs/harness.md` states what the native check's
    repeat half does **not** re-check (the second conversion's installation block and rendered resume
    line, which the same route code writes and `cmd/ccc`'s cases pin the renderer for); the goal file's
    status line now matches this increment's index row; `scripts/codex-native-check.py`'s repeat half
    shares the first half's stage helpers (`record_installed_jsonl`, `inspect_counts`,
    `readback_verdict`, `load_and_view`, `run_turn`, `turn_identity`) with every recorded check name,
    state and detail unchanged — re-measured on the final tree: `25 check(s) passed, 0 failed, 2 skipped`
    offline and `27 check(s) passed, 0 failed, 0 skipped` with both prompt flags (two real turns), the
    same numbers §6 recorded at the time — while §13's tree, which added one comparison to that half,
    prints `26 check(s) passed, 0 failed, 2 skipped` offline (§13's own run; the turned numbers were not
    re-measured there).
  - `docs/upstream.md` and the carried tree are untouched: no carried file changed, and the gate's `parity`
    step is what proves it.

### 7.1 The gate's own output

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3591 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 198 tests in 5.222s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5650 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 101 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The leading `…` inside the `parity` line is the gate's own elision of a long step detail, not an edit of this
record: the `parity` step renders the checker's own stdout as one line (`scripts/gate.py`'s `step_parity` —
`tail(proc.stdout, 800).replace("\n", " | ")`: it keeps the last 800 characters of that stdout, spells the
line feeds it kept as ` | `, and marks the cut with a leading `…`; the 2 500-character default is the gate's
other steps' limit, not this step's) — so the line ends where the checker's own output ends, and every other
character of the block is what the run printed. The block is one run's own output, quoted as it printed; a
later run on this tree prints the same steps, the same counts and the same verdict apart from two fields,
each stated with its cause: `selftest`'s wall-clock seconds (5.222s above; 12.579s and 6.584s on two re-runs),
and the `gofmt` step's file count, which is the count of *tracked* `*.go` files — the run above was made while
this increment's new `internal/ccc/continue_growth_test.go` was untracked, so it printed the pre-increment
tree's `3591`, where the committed tree prints `3592` (`git ls-files '*.go' | grep -v '^vendor/' | wc -l`).
Every other field of the re-run, including `parity`'s `files seen by git: 5650`, was byte-identical to the
block above. (§13's re-run of the same tree prints the same steps and the same verdict with three counts
moved, each by a cause it names: `selftest Ran 202 tests`, `parity … files seen by git: 5651` and
`secrets … in 102 file(s)` — three cases joined `scripts/tests/test_codex_native_check.py` and
`internal/ccc/testdata/growth/codex-complete-turn.jsonl` is one more file this repository owns.)

Exit code `0`, and the verdict line is `GATE: PASS` with no `(partial: …)` suffix: every step ran, including
the two required ones (`parity`, `selftest`). Nothing in this increment changed the gate, its steps or its
knobs — the new Go cases run inside the existing `test` step's packages and the new Python cases inside
`selftest` — so there is nothing to disclose and nothing to narrow. `parity` is the proof that the carried
AgentsView tree is untouched: 5 553 carried files identical to the pinned revision, and the four paths that
differ are the four project-owned ones this repository has always declared.

## 8. The round's own findings, and what was repaired

Three findings, each caught by a check rather than by reading, and each repaired before the gate run
recorded above:

1. **The generator's second root was left to its default by the fixture test.** `TestFixturesMatchTheirGenerator`
   generates both trees in one run, and the first revision of this increment's change named only the root it
   was comparing at that moment, so the *other* tree was written to the flag's default — a path relative to
   the working directory, which for a test is the package directory. The litter was a real directory tree
   (`internal/ccc/internal/ccc/testdata/{fixtures,growth}`) that `git status` showed and nothing had asked
   for. Repaired: the test names both roots always, both flags' help says they are relative to the working
   directory, the generator's own comment says that a caller naming one root gets the other at its default
   and that every path it wrote is printed, and the stray tree was removed. The repaired test passes and
   `git status` shows only the files this increment means to add.
2. **A helper no production path called.** Once the boundary pass owned "which file does this conversation
   live in, and what is its digest", `sourceFileDigest` was reachable only from tests — dead weight whose
   rule (a read with no file of its own has no digest) had moved into the pass. Removed, and the three cases
   that drove it now drive `takeSourceBoundary` and the boundary's own not-taken answers; **no assertion was
   weakened**: the stop a comparison makes for a read that named no path is still pinned by
   `TestSourceCheckStopsAReadThatNamesNoFileAtAll`, and the two not-taken shapes plus the no-path case are
   pinned by `TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure`.
3. **A check with no own view to compare.** The comparison "the first session's own view does not hold the
   appended turn" would have passed vacuously on a run whose first loader call failed (`view` empty means
   the words are absent). It now requires a non-empty own view, so a run that could not read the first
   session's view reports that check as failed rather than green.

## 9. Limits and residual risks

- **The record-boundary claim is gated on the provider's own declaration and the file's own bytes**
  (Q-0004-01, superseded by Q-0004-07 and then Q-0004-09 — §10, §11). The declaration half is
  `parser.SourceCapabilities.IncrementalAppend`, the claim that the source grows by appended lines: Claude
  and Codex make it, and no other carried provider does. A provider whose format is line-recorded but which
  declares nothing (Copilot's transcript is the measured example) gets the file's bytes and digest with
  `record_offsets: not-claimed` and no record offset — and so does a provider declaring only
  `parser.ContentCapabilities.MalformedLineCount`, a count of malformed *entries* inside one whole-file
  document or container (QwenPaw, CodeBuddy, DeepSeek Harness), because that count is not the claim the
  offsets rest on. The file half is text-ness: a representation whose own bytes cannot be a transcript of
  lines gets the same `not-claimed` with that reason, which is what closes Codex's compressed `.jsonl.zst`
  rollouts. Both narrowings are stated per run rather than silently guessed around. What the gate does *not*
  cover is a text-shaped container of records (a base64 or otherwise textual wrapper): its bytes would pass
  the file half, and no provider in this tree writes one.
- **The boundary answers the read-coverage question only where the recorded size may be read as one file's,
  and a size is all it can compare** (Q-0004-08, superseding Q-0004-05; Q-0004-11 for what a size proves).
  The comparison is asked for the providers whose declarations make the read's recorded size this file's
  own, so a layout provider's composite size is never compared with one file's — and a source that grows
  while it is being read is reported as `read_coverage: size-mismatch` with both sizes, instead of the
  report attributing the conversation to bytes the read is not shown to have covered. The window itself is
  not closed: a file that grows during the read is still read only as far as the reader's own snapshot, and
  the report names it rather than repairing it. And the answer is a size match, never byte identity: a file
  replaced by different bytes of the same length after the read is a state no size comparison can see, which
  is why the state and its detail say what was compared (§12's finding 1, `TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity`).
- **The boundary states an unterminated record, not its publication** (Q-0004-10, superseding Q-0004-03's
  wording). The block reports the bytes no line feed follows and leaves whether the read carried them to
  the read's own fidelity and the conversion's counts; what the goal forbids — publishing bytes the read did
  not read as history — is answered by those two, and another conversion once the source is finished is the
  remedy the report names.
- **`ccc inspect` does not carry a boundary** (Q-0004-02), so "is my continuation current with the source
  right now?" is answered by an inspection's counts/size plus the stored conversion's boundary, not by one
  command.
- **The repeat scenario is natively verified for the 0002 route only** (Q-0004-04). The property itself is
  pinned for both target routes by the Go test, and 0003's own native check keeps its accepted shape.
- **A native check cannot judge an answer.** Both turned runs here show it: the second session answered the
  appended fact in each, while one first-session turn answered about the workspace sandbox instead. The
  harness reports the loader's answer and the session's growth; what the facts show is read by this record
  from the recorded blocks.
- **Two real turns are two billed turns.** Each prompt flag runs one, the run's line says how many ran, and
  the offline default (`26 check(s) passed, 0 failed, 2 skipped`, §13) runs none.
- **What a provider's parser drops silently is disclosed, not repaired.** The boundary states the file's
  unterminated record — the bytes beyond its last line feed — and the record itself is not published where
  the reader could not read it, and nothing here tries to reconstruct it; the remedy the goal states is
  another conversion once the source has been written. Where the reader *can* read it (a complete record
  without its terminator), it is carried, and the boundary's byte fact stands beside it rather than
  contradicting it.

## 10. The review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..4ca30db` and reported four findings (two
`MUST_FIX`). This round fixed all four; nothing outside them changed. The round's own re-measured numbers
are at the end of this section.

### R01 (P2, must fix) — the record-boundary gate accepted a declaration that does not mean "line records"

The review measured the gate's premise false on a provider already in the carried tree. Reproduced here
before the fix: a complete QwenPaw session (`{"agent":{"memory":{"content":[[…],[…]]}}}`, 370 bytes, one
compact JSON document, no trailing line feed, two messages) converted with `records_are_lines: true`,
`records_end_bytes: 0` and the whole file as `unpublished_tail_bytes`, and printed the human line *"the
source's records end at byte 0 — the file holds more than this snapshot covers"*, an `unpublished:` line
and a warning telling the caller to convert again once the source was written — for a file whose read had
consumed every byte. The cause: the gate accepted `parser.ContentCapabilities.MalformedLineCount` as a
line-transcript declaration, but QwenPaw counts malformed *message entries* inside one whole-file JSON
document, CodeBuddy does the same for a manifest object, and DeepSeek Harness's source may be a compressed
container whose bytes hold no line feed at all.

The gate is now `parser.SourceCapabilities.IncrementalAppend` alone — the declaration that the source
grows by appended lines — and `Q-0004-01` is superseded by the appended `Q-0004-07`. Two regression cases
pin it, both measured red against the old gate and green against the new one:
`TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations` gained the `qwenpaw`/`codebuddy`/
`deepseek-harness` rows (`sourceRecordsAreLines("qwenpaw") = true, want false`, and likewise the other
two, before the fix), and the new `TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary` drives the
whole-file document end to end and pins `records_are_lines: false`, no record offset, no unpublished tail,
no unfinished-record warning, and the file's own digest.

### R02 (P3, must fix) — four links to the archived 0003 resolved to nothing

`docs/goals/0005-pi-continuation.md`, `0008-opencode-source-continuation.md` and `0009-selective-context.md`
named `0003-codex-to-claude-continuation.md`, and the archived file's own prerequisite named
`archive/0002-claude-to-codex-continuation.md` from inside `archive/` — four references that no longer
resolve. All four now name their real locations. Verified by a one-off scan of every relative `*.md` link
under `docs/goals/` (all resolve); the scan is not a gate step, so "nothing in the gate resolves relative
links" remains the untested class the finding named.

### R03 (P3) — the native check's final comparison did not cover the file the conversions read

`scripts/codex-native-check.py` digested the copy for the last time right after the second conversion, while
the second `thread/resume`, the target's own view read and the optional `--repeat-resume-prompt` turn all ran
after it — so a mutation of the copy during the second native stage left every check green, including "the
source session is unchanged". The final comparison now also measures the copy against the digest and size
taken before the second conversion; the check keeps its name (that name is what a reader takes as the
scenario's source having survived the whole repeated transfer) and its detail now names the copy it
compared. The offline case
`ProtectedArtifactsAfterNativeVerification.test_a_copy_mutated_during_the_second_load_is_caught_by_the_final_comparison`
changes the copy while the second session's own loader runs and requires exactly that check to fail — red
against the pre-fix comparison (the mutation was invisible: the run's failures were an empty list) and green
after it, with the intact run's assertions requiring the copy clause to be named so it cannot be green by not
being there.

### R04 (P4, optional) — the plan's CLI illustration was stale

`docs/plans/0004-repeatable-evolving-session-handoffs.md` §3.2 now quotes the wording the shipped CLI
prints (`cmd/ccc/main.go`'s `writeBoundary`/`boundaryTailSuffix`), with its illustrative numbers labelled as
such and §1 of this record cited for a real run's own lines.

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 199 tests in 5.531s (rc=0)
  PASS     parity   — this record's elision of the step's long detail: 5 553 carried file(s) identical to
                    the pinned revision, every declared deviation present, the same fields §7.1 quotes in full
  PASS     secrets  no leaks found in 101 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

`./cmd/ccc/... ./internal/ccc/...` held 219 `func Test…` functions over 14 test files before this round and
220 after it (the one new case above); the review round's Python case moved the `selftest` step's own count
from `Ran 198 tests` to `Ran 199 tests` (§7.1's 198 is that step's earlier run, §10's count is this one).
The seconds in that step's line are the wall clock, as §7.1 says of its own run: this round's runs of the
tree printed `selftest`'s line with different seconds each time and every other field — `gofmt`'s 3592
included — identical.

`python3 scripts/codex-native-check.py` (the offline half, exit code `0`, no turn billed), re-run to
re-derive the numbers §6 quotes:

```console
$ python3 scripts/codex-native-check.py
…
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical; the converted copy identical too: sha256 e2921e995915855d…, 2540 bytes)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
25 check(s) passed, 0 failed, 2 skipped
```

The count is unchanged (the copy clause joined an existing check rather than adding one), so §6's `25
… 2 skipped` offline and `27 … 0 skipped` turned numbers are the counts this tree prints as well; the one
line that differs from §6's quoted block is the final comparison, re-taken there with this run's own copy
digests. The turned runs were not re-run (two billed turns each, and the clause they would exercise is the
one this round's offline case pins), so §6's turn answers stand as the runs they were.

`python3 scripts/tests/test_codex_native_check.py` (inside `selftest` above, and run alone): `Ran 28 tests
… OK`, including the new copy-mutation case.

**`AGENTS.md` and bookkeeping:** this round changed no repository layout, no gate step, no command, no read
order and no rule an agent must follow — the gate's steps, its narrowing and its knobs are untouched, and the
native check remains deliberately not a gate step — so `AGENTS.md` is unchanged (its snapshot bullet's "the
record offsets are claimed only where the provider's own declarations say its source is a transcript of line
records" is now exactly true, which is what R01 was about). `docs/goals/` holds no goal file whose
implementation is merged-and-waiting: 0004 is this increment's own, still on its branch and awaiting operator
acceptance, so nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s status table is unchanged.

## 11. The second review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..745e5f3` — the head this increment's records had
reached — and reported three `P2` findings, all of them about what the boundary block *claimed*. All three
are fixed; nothing outside them changed. Two of the three were reproduced end to end on the pre-fix tree
before the fix (below), the third is a state only a racing read leaves. The field names the sections above
and §10 quote (`records_are_lines`, `records_end_bytes`, `unpublished_tail_bytes`) are the pre-fix
spellings of `record_offsets`, `terminated_records_end_bytes` and `unterminated_tail_bytes`; the append-only
records are left as they were written, and the questions they answer are superseded in
`docs/decisions/OPEN-QUESTIONS.md` (Q-0004-08…10). The re-measured numbers are at the end of this section.

### F01 (P2) — the reported boundary could include history the conversion never read

The conversation is read first and the boundary is taken afterwards, and the inherited Codex read is
*bounded by the size the file had when the parser opened it* (`internal/parser/codex_rollout_open.go`'s
`plainCodexRolloutSnapshot`, an `io.LimitReader` over the file's stat size). A complete turn appended
during that parse is therefore excluded from the conversation while the boundary — and
`installation.source_check_state`, which compares the boundary's digest with the file after the
installation — both describe the newer bytes and report the file `unchanged`: the block had **no field that
compared the read's own snapshot with the file**, so nothing in the report said the conversion covered less
than it described.

The race itself is not reproduced here, and cannot be on demand: the parse runs inside `Continue`, and a
case that appended to the file while it ran would be a flake rather than a case. What is reproduced is the
state it leaves, pinned at the pass's own seam
(`TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead`): a read that recorded `8` bytes for a file
that now holds `16`.

**Fix:** the boundary answers the question it can support, from the read's own record — `read_bytes` (the
size the read recorded for the file it opened), `read_coverage: verified` when the file is that size, and
`read_coverage: unverified` with both sizes in `read_coverage_detail` and in the run's warnings when it is
not (Q-0004-08, superseding Q-0004-05). The comparison is asked only where the provider's own declarations
make that size this file's own, which is the set Q-0004-05's composite-size objection was about excluding.

**Regressions:** `TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead` (the state above, its detail,
its warning, and that the pass writes nothing), `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile`
(both channels, including both answers at once), and the repeated-conversion case's end-to-end assertion
that a read which did keep up reports `read_bytes` equal to the file's size with `read_coverage: verified` —
which is what pins the wiring of the read's recorded size into the pass.

**Not closed:** the window itself. A source that grows while it is being read is still read only as far as
the reader's own snapshot; what changed is that the report names it instead of attributing the conversation
to bytes it is not shown to have covered (§9).

### F02 (P2) — a valid final record was reported as unpublished while being carried

Both inherited full readers accept a valid record at EOF without its line feed (Codex's own tests pin it:
`codex_cursor_test.go`'s "newline-less valid EOF is parsed but not seeded"; Claude's loop processes any
line it can parse). The block called every byte after the last line feed `unpublished_tail_bytes` and its
warning said those bytes "are not part of this conversion", which is false for exactly that record.
Reproduced on the pre-fix tree, with the committed growth fragment appended to a copy of the committed
Claude fixture *without its last byte* (a complete record the file has not terminated):

```console
### the old tree, a complete record whose line feed is missing
  conversation: 10 message(s), 4 user, 6 assistant, 1 tool call(s), 1 compaction boundary(ies)
  boundary:     2539 byte(s) read, the source's records end at byte 2254 — the file holds more than this snapshot covers
  unpublished:  285 byte(s) after that hold no record terminator: the record the source was writing is not part of this conversion
  installed:    /tmp/evidence-0004/r1/nofeed/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e979-e61c-7de0-8813-f74cdab256a1.jsonl [hard-link]
  the record's own words in the installed document the old run wrote: 2
```

The last two lines are the contradiction: the block says the record is not part of the conversion, and the
conversion wrote its dialogue into the installed document twice.

**Fix:** what the block states is the file's own record structure — `terminated_records_end_bytes` (where
its line-feed-terminated records end) and `unterminated_tail_bytes` (the bytes after them, which no line
feed follows and which the source may still be writing) — and it claims nothing about whether the read
carried that record: the read's own fidelity block and the conversion's own counts answer that, and the
warning names the bytes, the possibility that the source is still writing them, and the remedy without
asserting an omission (Q-0004-10, superseding Q-0004-03's wording).

**Regressions:** `TestContinueCarriesACompleteRecordThatHasNoLineFeed` (the fragment appended twice — as
committed and without its last byte: the same conversation and the same carried counts both times, and only
the second boundary reports an unterminated record), `TestUnterminatedSourceTailIsBoundaryedAndNeverPublished`
(which now also asserts no warning claims the omission), `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile`,
and `TestSourceBoundaryOfFileStatesTheRecordOffsets`'s "a complete record without its line feed" row.

### F03 (P2) — compressed Codex rollouts received meaningless raw-byte record offsets

Codex declares `IncrementalAppend` *and* reads its own zstd-compressed rollouts, which the inherited
implementation decodes before parsing and refuses any offset into
(`internal/parser/codex_rollout_open.go`; `codex_provider.go`'s `ParseIncremental` returns
`IncrementalNeedsFullParse` for them). The gate was the provider-wide declaration alone, so the block
scanned the *compressed* file for line feeds. Reproduced on the pre-fix tree, with a zstd-compressed copy of
the committed Codex rollout (624 bytes; the plain rollout is 1 792):

```console
### the old tree, a compressed Codex rollout
  boundary: {"state": "taken", …, "bytes": 624, "sha256": "7afc1614385660b3a7fd5934fd12ee921f3600c11e509a4bdcfd33a1769bae4e", "records_are_lines": true, "records_end_bytes": 534, "unpublished_tail_bytes": 90}
  conversation: 6 message(s); carried: 4
  warnings: ["the source file holds an unfinished record: its bytes end at byte 624 while its records end at byte 534, so the last 90 byte(s) hold no record terminator and are not part of this conversion; converting again once the source has been written carries them"]
```

and the same command on the fixed tree:

```console
### the fixed tree, the same compressed rollout
  boundary: {"state": "taken", …, "bytes": 624, "sha256": "7afc1614385660b3a7fd5934fd12ee921f3600c11e509a4bdcfd33a1769bae4e", "read_bytes": 624, "read_coverage": "verified", "read_coverage_detail": "the read's own snapshot of this file is 624 byte(s) and the file holds 624: the conversion read these bytes", "record_offsets": "not-claimed", "record_offsets_detail": "the file's own bytes are not a transcript of text lines (a compressed or otherwise binary container of the records, or a sequence that does not decode as text), so n…
  conversation: 6 message(s); carried: 4
```

(and no warning at all: the document carries no `warnings` array, which is what an empty list serializes
to). Both runs carried the same decoded history — 6 messages, 4 carried — which is the point: only the
claims about the *bytes* were wrong.

**Fix:** the gate has a second half. The offsets are claimed only when the provider's declaration holds *and*
the file's own bytes can be that transcript — measured in the same pass as valid UTF-8, chunk by chunk,
carrying a rune a chunk boundary splits to the chunk that completes it. A file whose bytes are a compressed
or otherwise binary container of the records is reported by its size, digest and read-coverage with
`record_offsets: not-claimed` and the reason beside it, and no offset or unterminated record is claimed
(Q-0004-09). The path name, a magic number and a per-source provider call were each rejected as the gate:
the first two duplicate a format fact in `internal/ccc`, and the inherited interface's only per-source
answer needs a checkpoint for providers like Claude and would narrow their own case wrongly.

**Regressions:** `TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets` (end to end, against the
plain rollout's conversation of the same bytes), `TestSourceBoundaryOfFileStatesTheRecordOffsets`'s
container row, `TestTextStreamCarriesSplitRunes` (the validator, including a rune split across chunks), and
the native check's own offline cases, whose boundary comparison now requires the coverage answer too
(`test_a_second_boundary_that_is_not_the_grown_copy_fails` gained `read_coverage` and `read_bytes`
overrides), so a report whose read is not shown to have covered the file fails there as well.

### What this round changed in the files that carry the rules

- `AGENTS.md`: the snapshot bullet now states the coverage answer, that the record offsets need the
  provider's declaration *and* the file's own bytes (with a compressed container named as the case), and
  that an unterminated record is disclosed without being called published or left out. This is a change to a
  rule an agent must follow, and to how the boundary's own language is used, so the file moved with it; the
  layout, gate, commands and read order are unchanged.
- `docs/conventions.md`, `README.md` and `cmd/ccc`'s help paragraph: the same rules in the words their own
  readers use, with the rename and the coverage answer; the help paragraph's assertions in
  `TestContinueHelpIsARequestNotAFailure` moved with it.
- `docs/plans/0004-repeatable-evolving-session-handoffs.md`: §2.1's field table, §2.2's superseded bullets,
  §2.3's case table (which now has a row for a complete unterminated record and one for a read that moved),
  §3.1's shape, §3.2's CLI illustration, §4's attribution clause and §7's question list.
- `docs/harness.md`: the native check's repeat half states that its attribution comparison includes the
  read's own coverage answer.
- `docs/decisions/OPEN-QUESTIONS.md`: Q-0004-08 (the read's own snapshot, superseding Q-0004-05), Q-0004-09
  (the gate's second half) and Q-0004-10 (the unterminated record as a byte fact, superseding Q-0004-03's
  wording) appended.
- §6's offline block: re-taken from this round's own run of the offline half on the fixed tree (`25
  check(s) passed, 0 failed, 2 skipped`), with that run's session ids and the fixed tree's field names. The
  turned runs were not re-run — the two prompts spend two billed turns each, their quotes carry no boundary
  field, and the clause they exercise is the one this round's offline half and its cases pin.
- `docs/upstream.md` and the carried tree: untouched — the gate's `parity` step is what proves it.

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 199 tests in 5.588s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5650 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 101 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The seconds in that step's line are the wall clock, as §7.1 and §10 say of their own runs: a re-run of the
frozen tree printed `selftest Ran 199 tests in 5.549s` and every other field of the block above
byte-identical, so the counts — not the clock — are what this record rests on.

`./cmd/ccc/... ./internal/ccc/...` held 220 `func Test…` functions over 14 test files before this round and
224 after it (`git grep -h '^func Test' HEAD -- cmd/ccc internal/ccc | wc -l` against the same count in the
working tree); the `selftest` step's own count is unchanged at `Ran 199 tests`, because the Python case that
covers the boundary comparison grew (two more overrides inside one case) rather than adding one.

`python3 scripts/codex-native-check.py` (the offline half, exit code `0`, no turn billed): `25 check(s)
passed, 0 failed, 2 skipped` — §6's block is that run.

**`AGENTS.md` and bookkeeping:** as stated above, this round changed a rule an agent must follow, so
`AGENTS.md` was updated and the repository layout, the gate and its steps, the commands and the read order
were not. `docs/goals/` holds no goal file whose implementation is merged-and-waiting: 0004 is this
increment's own, still on its branch and awaiting operator acceptance, so nothing moves to
`docs/goals/archive/` and `docs/goals/README.md`'s status table is unchanged.

## 12. The third review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..696e055` — the head this increment's records had
reached — and reported four findings, all four about something this record, the help or the code stated
more strongly than the run behind it supported. All four are fixed; nothing outside them changed. Two were
measured red against the pre-fix tree before the fix (findings 1 and 3, below); the other two are records
that named things which do not exist (finding 2: a target identity and a digest from two different runs;
finding 4: the account's own host paths in a transcript). The re-measured numbers are at the end of this
section.

### T01 (P2) — equal file sizes do not establish byte coverage

The boundary answered equal sizes with `read_coverage: verified` and the words "the conversion read these
bytes". The review measured what that claim rests on: the read's own record is *a size*
(`ParsedSession.File.Size` — `SourceIdentity.file_size`), not a digest of the bytes the read opened, so a
file replaced by a different transcript of the same length after the read leaves every size equal while the
conversation came from bytes that are gone. Nothing else in the report catches it: the boundary's digest is
of the file as it stands, `installation.source_check_state` compares two digests both taken after the read,
and the state and its detail were the whole of the claim.

**Fix (Q-0004-11):** the states say what was compared. `SourceBoundaryCoverageSizeMatch` (`size-match`) and
`SourceBoundaryCoverageSizeMismatch` (`size-mismatch`) replace `verified`/`unverified`; the match state's
detail names the comparison and its limit ("the file is the size the read recorded, which is a size match
and not a byte-identity check -- the read's own record holds no digest of the bytes it opened, so a
same-length replacement of the file after it is not ruled out"); the human `boundary:` clause says the same
("the file is the size the read's own snapshot recorded (a size match, not a byte-identity check)"); and the
mismatch state, its detail, its warning and the CLI clause it renders are unchanged apart from the constant
they name.

**Regression:** `TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity` — a file replaced by different bytes
of the same length after the read, at the pass's own seam (the replacement happens inside a conversion
between the read and the pass, so an end-to-end case would race `Continue`'s own read). It requires
`size-match` with both sizes, the digest of the file as it stands, the detail's limit named, no warning, and
— as literals, so this half is red whatever the states are named — neither `verified` nor "the conversion
read these bytes". Measured red against the pre-fix code (which printed both) and green after:

```console
### the pre-fix code, the same case (the two states' values and words restored for the measurement)
--- FAIL: TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity (0.00s)
    continue_growth_test.go:777: boundary = {… ReadCoverage:verified ReadCoverageDetail:the read's own snapshot of this file is 16 byte(s) and the file holds 16: the conversion read these bytes …}, want no byte-identity claim from a size comparison
### the fixed code
--- PASS: TestTakeSourceBoundaryStatesASizeMatchNotAByteIdentity (0.00s)
```

The states' names travel with the claim everywhere it is made: `cmd/ccc`'s `coverageSuffix`, the
repeated-conversion case's assertions, the `read_coverage` the native check requires
(`scripts/codex-native-check.py`) and the overrides its own cases drive it with
(`scripts/tests/test_codex_native_check.py`), the plan's field and case tables, `AGENTS.md`,
`docs/conventions.md`, `README.md`, `docs/harness.md` and §9 above.

### T02 (P3) — the §2 transcript named one run's sessions beside another's digests

§2's B block recorded B1 installing `01a1e939-1b2b-…` with digest `c945f80f6c93bb49`, and B4 — the block's
"the earlier target stays unchanged" evidence — listing `01a1e978-fa33-…` with digest `ba2870988be17a2f`.
§2's table called the second "the same digest it had immediately after B1", and §5 cited B4 as the evidence
for B1's digest — which B4 does not show, and cannot: the two excerpts are different runs (the home B4 listed
was not the home B1 converted into).

**Fix:** the whole B block is one run's own transcript, re-taken on the fixed tree and re-verified
line by line against it. B1's `installed:`/`target id:` lines and B4's `first installed now:` now name the
same file, `01a1e98b-76e1-…` (`rollout-2026-05-04T09-15-00-01a1e98b-76e1-7475-ba0e-80c959cb5f5d.jsonl`), and
B4 digests it after the second conversion with the same `sha256=3971fbc722c190ee` it had when B1 wrote it;
the home B4 lists holds exactly the two sessions B4's two lines name, and B5's two counts are of those two
documents. §5's row and §2's claims table were re-taken with the same identities, and §2 says which run the
block is.

### T03 (P3) — the help named an `unterminated:` line no block prints

`ccc help` said an unfinished record is reported "under 'unterminated:'", while the renderer prints it on
the `records:` line (`writeBoundary`/`recordsLine`) and repeats it in the run's `warning:` lines — and the
help case pinned the stale spelling rather than checking it against the renderer.

**Fix:** the help now says "the 'records:' line and the run's warnings state those bytes, and leave what the
read made of them to the read's own fidelity", and `TestContinueHelpIsARequestNotAFailure` reads the label
back out of the block itself: it renders an unterminated boundary through `writeBoundary`, takes the label
of the line that states the tail (`tailLabelOf`), and requires the help to name *that* label — so a rename
in the renderer and not in the help fails the case instead of being pinned twice. Measured red against the
stale wording and green after:

```console
### the help put back to the pre-fix wording
--- FAIL: TestContinueHelpIsARequestNotAFailure (0.01s)
    main_test.go:2091: help does not state the repeat rule ("'records:' line and the run's warnings state those bytes")
### the fixed help
--- PASS: TestContinueHelpIsARequestNotAFailure (0.01s)
```

§1's `ccc help` quote is re-taken from this tree (`ccc help`, exit code 0) with the paragraph as it prints.

### T04 (P3) — the §11 gate transcript kept the account's own host paths

The last gate transcript in §11 printed the account's literal repository root and Go toolchain path on its
two header lines, against this record's own stated scheme (the repository root spelled `<repo>`, the
account's home spelled `~`) and `docs/conventions.md`'s prohibition on absolute host paths in committed
evidence.

**Fix:** both header lines now carry `<repo>` and `~/.local/share/mise/…`; the transcript's other lines are
unchanged. This record's runs otherwise name the committed fixture paths and `/tmp/evidence-0004/…` scratch
paths, which its header states are the runs' own. (The accepted 0003 record carries host paths of its own
from that increment's runs; it is that increment's record and this round left it as it was.)

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`, the verdict line `GATE: PASS` with no `(partial: …)` suffix — every
step ran, including the two required ones:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 199 tests in 5.490s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5650 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 101 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The header lines are this record's placeholders, as its own scheme states (the run printed the account's
home); every other character is what the run printed, with the `parity` line cut exactly where the gate's own
`tail()` cut it. `gofmt`'s 3592 is the count of *tracked* `*.go` files, unchanged from §11 (this round changed
files rather than adding one); `selftest`'s `Ran 199 tests` is unchanged too (the Python cases this round
touched changed values inside existing cases rather than adding one), and its wall clock varies per run.

`./cmd/ccc/... ./internal/ccc/...` held 224 `func Test…` functions before this round and 225 after it
(`git grep -h '^func Test' HEAD -- cmd/ccc internal/ccc | wc -l` against the same count in the working tree) —
the one new case is T01's regression.

The evidence re-runs this round's blocks are taken from, all offline, no turn billed, exit code 0 each:
`/tmp/evidence-0004/r3/{a3,a4}` (§1's two runs), `/tmp/evidence-0004/r3/b` (§2's B block: the copy, the two
conversions into one home, the first installed file digested after the second), `/tmp/evidence-0004/r3/c`
(§3's two cases), and `python3 scripts/codex-native-check.py` (§6's offline block: `25 check(s) passed, 0
failed, 2 skipped`, which is also the count §10 and §11 record). The turned runs were not re-run: two billed
turns each, they quote no boundary field, and the clause they exercise is what the offline half and its cases
pin.

### What this round changed in the files that carry the rules

- `AGENTS.md`: the snapshot bullet now states the size match and its limit (the read's own record holds no
  digest of the bytes it opened, so a same-length replacement is not ruled out). This is a change to a rule an
  agent must follow, so the file moved with it; the repository layout, the gate and its steps, the commands
  and the read order are unchanged.
- `docs/conventions.md`, `README.md`, `cmd/ccc`'s help paragraph and `docs/harness.md`: the same rule in the
  words their own readers use, with the new state names and the help's own label corrected.
- `docs/plans/0004-repeatable-evolving-session-handoffs.md`: §2.1's `read_bytes`/`read_coverage` row, §2.2's
  bullets (a new "not a byte-identity claim" one), §2.3's case table (an equal-size replacement row) and
  §3.2's CLI illustration.
- `docs/decisions/OPEN-QUESTIONS.md`: Q-0004-11 appended (the states say what was compared; Q-0004-08's
  wording superseded), with the reversals and the regression named.
- §1–§6 above: the run blocks re-taken from this round's own re-runs (named above, each in its own note) and
  the tables and limits re-worded; §10 and §11 keep the wording of the rounds that wrote them, and the
  `read_coverage` values they quote are the pre-round spellings of T01.
- `docs/upstream.md` and the carried tree: untouched — the gate's `parity` step (above) is what proves it.

**`AGENTS.md` and bookkeeping:** as stated above, this round changed a rule an agent must follow, so
`AGENTS.md` was updated and the repository layout, the gate and its steps, the commands and the read order
were not. `docs/goals/` holds no goal file whose implementation is merged-and-waiting: 0004 is this
increment's own, still on its branch and awaiting operator acceptance, so nothing moves to
`docs/goals/archive/` and `docs/goals/README.md`'s status table is unchanged.

## 13. The fourth review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..c38189e` — the head this increment's records had
reached — and reported two findings, both `P3`, both about a claim the run or the record behind it could not
support: the native check's closing "the source session is unchanged" verdict measured from a baseline
nobody had re-checked across the first native stage, and this record's claim that the no-line-feed rule was
pinned "in both directions" while the case converted a Claude source only. Both are fixed; nothing outside
them changed. Both were measured red against the pre-fix tree before the fix, and the round closes the two
classes they belong to — the copy's preservation claims and the no-line-feed rule's coverage, each of which
had survived an earlier round — with one machine-checked pin per rule rather than an argument (the table
under "the close-out" below). The re-measured numbers are at the end of this section.

### U01 (P3) — the source copy's baseline was not re-checked across the first native stage

The check compared the copy with `copy_before` before the first native loader and the optional turn, and
then appended the growth fragment and adopted the grown file's digest as the baseline every later comparison
named — without comparing the copy against `copy_before` again. The review measured what that leaves: a
*same-length* change during the first native stage satisfies the growth record's own claim (the size delta
is exactly the fragment's bytes, and the digest differs from the pre-conversion digest as expected) and then
*becomes* the baseline, so the final `the source session is unchanged` spoke for an interval whose baseline
it had never checked.

**Fix (Q-0004-12):** the copy is compared with the bytes taken before the first conversion **immediately
before it grows**, as its own check — `the source copy still holds the bytes the first conversion read`,
whose detail names both digests and both sizes — and the growth record keeps its own size claim, which is
true either way. Every window a copy must survive now has a case of its own, each driving one same-length
swap and asserting the exact failure set it produces (`drive`'s `copy_mutation` table in
`scripts/tests/test_codex_native_check.py`). Measured red against the pre-fix script (the comparison
removed, everything else as shipped) and green after:

```console
### the comparison removed from the shipped check, the new case
AssertionError: Lists differ: [] != ['the source copy still holds the bytes the first conversion read']
…
Ran 1 test in 0.007s

FAILED (failures=1)
### the fixed check, the shipped script's own cases
Ran 31 tests in 0.095s

OK
```

The empty list is the defect itself: with the comparison gone, the mutating run recorded no failure at all —
the growth record passed, and every comparison after it measured from the changed bytes.

### U02 (P3) — "in both directions" was not the direction the case exercised

This record attributed two-direction end-to-end coverage to
`TestContinueCarriesACompleteRecordThatHasNoLineFeed` (§4's table row, and the case's own comment), while the
case's helper always converted the *Claude* fixture into Codex; the Codex-source case elsewhere in §3 is
about a tail of malformed JSON, not a complete record missing its final line feed. The cited case therefore
provided no Codex → Claude evidence for the rule it was credited with.

**Fix (Q-0004-13):** the claim's direction was added rather than the claim narrowed. The case is a table over
both accepted source providers now, and a committed `growth/codex-complete-turn.jsonl` — the same question
and the same reply as the Claude fragment, in Codex's own `response_item` shape, generated and pinned by
`internal/ccc/fixturegen` like every other fixture — drives the Codex → Claude half. The case also asserts
that its own search text sits in the fragment's **last** record, the record its second half leaves
unterminated: the fragment's earlier records are terminated, so a search that fell back to one of them would
stay green while the record the rule is about was dropped. Measured red by making the no-feed half's last
record *incomplete* — the one thing that separates "unterminated but complete" from "unfinished", and the
half both directions share — and by reading the fragment's first text field instead of its last:

```console
### the no-feed half's last record cut mid-JSON
--- FAIL: TestContinueCarriesACompleteRecordThatHasNoLineFeed (0.14s)
    --- FAIL: TestContinueCarriesACompleteRecordThatHasNoLineFeed/a_Claude_source_into_Codex (0.13s)
        continue_growth_test.go:404: the record without its line feed read as 9 message(s) and carried 6, against 10 and 7 with it: both readers read a complete unterminated record
        continue_growth_test.go:410: the installed session does not hold "The retry budget is 120 seconds per session, held in internal/retry/budget.go.", the dialogue of the record the file had not terminated; the read carries it
    --- FAIL: TestContinueCarriesACompleteRecordThatHasNoLineFeed/a_Codex_source_into_Claude (0.01s)
        continue_growth_test.go:404: the record without its line feed read as 7 message(s) and carried 5, against 8 and 6 with it: both readers read a complete unterminated record
        continue_growth_test.go:410: the installed session does not hold "The retry budget is 120 seconds per session, held in internal/retry/budget.go.", the dialogue of the record the file had not terminated; the read carries it
```

```console
### the fragment's first text field read instead of its last
--- FAIL: TestContinueCarriesACompleteRecordThatHasNoLineFeed (0.11s)
    --- FAIL: TestContinueCarriesACompleteRecordThatHasNoLineFeed/a_Codex_source_into_Claude (0.00s)
        continue_growth_test.go:358: the case searches for "What budget do retries share per session?", which is not in the fragment's last record "{…}": the no-line-feed rule is about that record alone
```

*(Each block is the run's own `--- FAIL:` lines; the Claude half of the first block and the Codex half of
the second are quoted in full, the run's per-half boundary line, the file path the case prints and — in the
second block — the fragment's last record (quoted as `{…}`) are elided, and no other character is altered.
The second block's failure is the Codex half alone, which is what makes the last-record read a per-direction
pin rather than a shared one.)*

### The close-out: every rule of the touched surface, with its pin

Round 3's instruction was to stop answering a surviving class with an argument. The two classes this round
touches are the source copy's preservation claims (opted in by U01) and the no-line-feed rule's coverage
(U02); every rule of either now names the case that fails when it is broken, and each was *executed* — the
mutation run, the failure observed, the line restored:

| Rule | Its pin | The mutation this round executed |
| --- | --- | --- |
| the copy is not written by the first conversion | `test_a_copy_mutated_during_the_first_conversion_is_caught_by_two_comparisons` | the case's own same-length swap, in every run of it: exactly that comparison and the growth baseline fail |
| the copy is not written by the first native stage, and the growth is measured from the pre-conversion bytes | `test_a_copy_mutated_during_the_first_load_is_caught_where_the_copy_grows` | the case's own swap (exactly that comparison fails, and the growth record passes); signed again by deleting the shipped comparison, which left the mutating run's failure list empty (U01's block) |
| the copy is not written by the second conversion | `test_a_copy_mutated_during_the_second_conversion_is_caught_by_two_comparisons` | the case's own swap: the second copy comparison and the closing verdict fail, the attribution comparison stays green |
| the copy is not written by the second native stage | `test_a_copy_mutated_during_the_second_load_is_caught_by_the_final_comparison` | the case's own swap (now same-length, so it exercises the digest clause rather than the size clause): the closing verdict and only it fails |
| the committed growth fragments are what `fixturegen` writes | `TestFixturesMatchTheirGenerator` | one byte swapped in the committed `codex-complete-turn.jsonl`: `growth fragments: codex-complete-turn.jsonl differs from what fixturegen writes; re-run go run ./internal/ccc/fixturegen` |
| a complete record whose line feed is missing is read and carried, per source direction | both halves of `TestContinueCarriesACompleteRecordThatHasNoLineFeed` | the last record cut mid-JSON (both halves fail, each with its own counts), and the first text field read instead of the last (the Codex half alone) |

The comparison the closing verdict is measured *from* is the first rows' subject too: `the source session is
unchanged` covers the copy after the second native stage, and the growth baseline it is measured against is
now a checked comparison rather than whatever bytes were there. What this round did **not** re-prove is the
three protected-artifact comparisons' ordering (§10's R03 case) and the two search comparisons (§12's
cases): both were pinned by the rounds that fixed them, and this round did not touch their code.

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`, the verdict line `GATE: PASS` with no `(partial: …)` suffix — every
step ran, including the two required ones:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 202 tests in 5.544s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5651 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 102 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The header lines are this record's placeholders, as its own scheme states (the run printed the account's
home); every other character is what the run printed. The `parity` line is the gate's own rendering whole:
its one cut is the gate's own leading `…` (`tail(proc.stdout, 800)`, §7.1's rule), so the line ends with the
checker's own last line (`declared stored bytes verified: …`) rather than at a cut made here. Three counts
moved from §12's run of the same steps, each by a cause this round names:
`selftest` from `Ran 199 tests` to `Ran 202` (three cases joined
`scripts/tests/test_codex_native_check.py`), `parity`'s `files seen by git` from `5650` to `5651` and
`secrets`' from `101 file(s)` to `102` (the committed `internal/ccc/testdata/growth/codex-complete-turn.jsonl`
is one more file this repository owns). `gofmt`'s 3592 is unchanged — this round added no `*.go` file — and
`selftest`'s wall clock varies per run.

`./cmd/ccc/... ./internal/ccc/...` held 225 `func Test…` functions before this round and 225 after it
(`git grep -h '^func Test' HEAD -- cmd/ccc internal/ccc | wc -l` against the same count in the working tree):
this round extended a case rather than adding one, and the three cases that did join are Python ones.

`python3 scripts/codex-native-check.py` (the offline half, exit code `0`, no turn billed) reports
`26 check(s) passed, 0 failed, 2 skipped` — one more than §12's 25, the comparison U01 added — and its
opening lines and the check that comparison records are quoted in §6 above (§6's block is this run's own).
The turned runs were not re-run: two billed turns each, which is why §6 states the turned count it would
print as arithmetic rather than quoting a run. The comparison this round added is taken after the first
conversion's loader and its own real turn, and immediately before the copy the second conversion reads
grows — ahead of the second conversion, its loader and its turn, not ahead of both turns (§6's passage
above and this one read the other way round until §15 corrected them).

The evidence re-runs this round's blocks are taken from, all offline, no turn billed, exit code `0` each:
`/tmp/evidence-0004/r4/gate.txt` (§13's gate transcript), `/tmp/evidence-0004/r4/native.txt` (§6's offline
block and view block), `/tmp/evidence-0004/r4/gen/{fixtures,growth}` (the generator run whose two trees are
byte-identical to the committed ones apart from the new fragment), and the mutation runs quoted above, each
executed on this tree and restored.

### What this round changed in the files that carry the rules

- `scripts/codex-native-check.py`: the growth baseline is compared with the pre-conversion bytes immediately
  before the copy grows, as its own recorded check, and the module docstring and the closing comparison's
  comment state the interval that comparison covers. The harness stays **not** a gate step; no gate step,
  no required step and no narrowing changed.
- `scripts/tests/test_codex_native_check.py`: the four copy-window cases (one of them U01's) drive one
  window each through a single `copy_mutation` seam, with the failure set each window must produce stated in
  `drive`'s table; the second-load case's mutation became the same-length swap the other three use.
- `internal/ccc/fixturegen/main.go` and `internal/ccc/testdata/growth/codex-complete-turn.jsonl`: the Codex
  growth fragment, generated by the same command and pinned by the same case as every other fixture (the
  generator's own run wrote both trees, and the committed fixtures are byte-identical to its output).
- `internal/ccc/continue_growth_test.go`: `TestContinueCarriesACompleteRecordThatHasNoLineFeed` is a table
  over both accepted source providers, asserts its search text is in the fragment's last record, and the
  fragment-text helper reads that last text field (its name no longer says "unfinished": the fragments it is
  asked about are complete turns in the case's two directions and a byte prefix of one in the tail case).
- `docs/harness.md`: the native check's paragraph states the growth-boundary comparison.
- `docs/plans/0004-repeatable-evolving-session-handoffs.md`: the exclusion list's second growth fragment is
  no longer excluded-and-absent — the bullet now says what the committed Codex fragment is *for* (the
  no-line-feed rule's per-direction pin) while a repeated Codex *source* and a repeat scenario for the 0003
  route stay out of scope.
- `docs/decisions/OPEN-QUESTIONS.md`: Q-0004-12 and Q-0004-13 appended (the per-window copy comparisons, and
  the per-direction pin with its fragment), and Q-0004-10's regression line now names both directions.
- §1–§6 above: §4's row and case list, §5's growth-fragment and source-file rows, §6's offline block, view
  block and case table, §7's native-count sentence and §7.1's note re-taken or re-worded from this round's
  own runs; §9's offline count follows §13. §10–§12 keep the wording of the rounds that wrote them, and the
  counts they quote are those rounds' own.
- `docs/upstream.md` and the carried tree: untouched — the gate's `parity` step (above) is what proves it.

**`AGENTS.md` and bookkeeping:** this round changed no repository layout, no gate step, no command, no read
order and no rule an agent must follow. `AGENTS.md` already describes `internal/ccc/testdata/growth/` as the
tree of fragments `fixturegen` writes and pins, and the new fragment is one more file in it; the native
check's added comparison changes what that non-gate harness verifies, not a rule the increment binds agents
to, and the two rules this round pinned were already stated — as claims the records then over- or
under-supported. `AGENTS.md` is therefore unchanged. `docs/goals/` holds no goal file whose implementation
is merged-and-waiting: 0004 is this increment's own, still on its branch and awaiting operator acceptance, so
nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s status table is unchanged.

## 14. The fifth review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..34845d1` — the head this increment's records had
reached — and reported one finding, `P3`, about a claim this record's own transcript could not support: §7.1
stated the gate's elision limit as a number the shipped gate does not use for that step, and the `parity`
line §13 quoted had lost the end the gate's own elision keeps. Both halves are fixed: the quoted line is the
run's own bytes again, and the limit is named from the gate's own call. Nothing outside them changed. The
class — what a quoted gate transcript may claim about its own edits — had survived two rounds (§12's T04 was
this same transcript stating something its own run could not support, the account's host paths, fixed by
re-spelling its header lines), so this round closes it with a machine-checked pin per rule rather than an
argument, and every pin was **executed** against this record's own bytes (the mutations below). The
re-measured numbers are at the end of this section.

### V01 (P3) — the transcript attributed the record's own cut to the gate

§7.1 put the gate's elision of the `parity` detail at that detail's last 2 500 characters, with the cut
marked — 2 500 is `tail()`'s default, which the Go steps' failed-output details use. The shipped gate does
no such thing for this step: `scripts/gate.py`'s `step_parity` renders the checker's output as
`tail(proc.stdout, 800).replace("\n", " | ")` — it keeps the last 800 characters, spells the kept line feeds
` | `, and marks the cut with a leading `…`. §13's quoted line made the second half visible: it ended
`… | declared stored byte…`, and the sentence beside it attributed that cut to the gate. A suffix-preserving
elision cannot remove a detail's end: this round's own run of the same tree prints the line whole — 840
characters, 816 after the mark, where 800 kept characters plus two for each of the 8 spelled line feeds is
exactly 816 — ending at the checker's own last line,
`declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json`
(the transcript is `/tmp/evidence-0004/r4/gate.txt`, and this round's is `/tmp/evidence-0004/r5/gate.txt`).
The quoted line held 740 characters after the mark with the same 8 separators: a 76-character cut only the
record could have made.

**Fix.** §13's line is the run's own 840-character line again (the transcript file's own bytes — it was that
block's only edited line; its header placeholders are this record's own scheme, stated beside it), and its
sentence now says the `parity` line is the gate's rendering whole, ending at the checker's own last line
rather than at a cut made here. §7.1's claim now names the step's own call (`tail(proc.stdout, 800)`, the
kept line feeds spelled ` | `, the leading `…`), says the line ends where the checker's own output ends, and
names 2 500 as the gate's *other* steps' limit. §11's excerpt — labelled inline as this record's elision and
carrying no gate mark — was already the shape the rule below pins, and is unchanged.

### The close-out: the rules of this surface, each with its executed pin

The surface is this record's gate-transcript claims. `scripts/tests/test_gate_transcript_claims.py` reads
them from the shipped record: a record joins the rule by naming the gate's elision, this increment's record
must be among those that do (so a rewording cannot retire the case quietly), and the case reads this
section's own line too. Each rule below was mutated on the shipped record, the failure observed, and the
line restored — the record's sha256 before and after every mutation matched:

| Rule | Its pin | The mutation this round executed, and the failure observed |
| --- | --- | --- |
| a quoted `parity` line with the gate's own mark is the gate's rendering whole: it does not end in an ellipsis, and its body is the kept suffix (the limit plus two per spelled line feed) | `test_every_rule_record_quotes_the_gate_s_own_rendering` | §13's line cut where the review found it (740 characters after the mark): `line 1247: the parity line carries the gate's own elision mark and ends in an ellipsis: tail() keeps the detail's suffix, so this cut is the record's own`, and `line 1247: the elided parity body is 740 characters after the mark, where the gate's rendering leaves 816 (800 kept characters plus two for each of the 8 spelled line feeds)` |
| a record's stated elision limit for that step is the gate's own call | the same case's stated-limit reading | §7.1's `800` put back to `2 500`: `line 570: the record states the gate's parity elision as '2 500' characters, where the gate's own call keeps the last 800` |
| a record that states the rule is read by the case | the same case's coverage assertion | the phrase `the gate's own elision` removed from this record: `0004-repeatable-continuation-handoffs.md is the record this case was written for and states the rule, but it is not among the records discovered as stating it (found: [])` |

The arithmetic's premise — the parity checker's stdout holds no ` | ` of its own, so every ` | ` a body holds
was a spelled line feed — is documented in the case rather than mutated: the checker's *output* is not an
artifact this tree holds, and a checker that ever printed one turns the rule into a visible false positive
rather than a silent pass. An older increment's record is not read: 0003's record quotes the same
2 500-character default for a *failed* `go test` step, which is that step's own limit, and its transcripts
follow the scheme its own round chose.

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`, the verdict line `GATE: PASS` with no `(partial: …)` suffix — every
step ran, including the two required ones:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 204 tests in 5.275s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5652 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 103 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The header lines are this record's placeholders, as §7.1's scheme states (the run printed the account's
home); every other character is what the run printed, and the `parity` line is the gate's own rendering
whole — its one cut is the gate's own leading `…` (§7.1's rule), so it ends at the checker's own last line,
which is the shape the case above now reads. Three counts moved from §13's run of the same steps, each by a
cause this round names: `selftest` from `Ran 202 tests` to `Ran 204` (this round's two Python cases joined
`scripts/tests/test_gate_transcript_claims.py`), `parity`'s `files seen by git` from `5651` to `5652` and
`secrets`' from `102 file(s)` to `103` (the new, staged Python case file is one more file this repository
owns). `gofmt`'s 3592 is unchanged — this round added no `*.go` file — and `selftest`'s wall clock varies per
run.

`python3 -m unittest scripts.tests.test_gate_transcript_claims` (and inside the `selftest` step above):
`Ran 2 tests … OK`, the mutation case included. Nothing native was re-run: this round touched no Go code and
no native path, so §6's turned runs and §13's offline count stand as those rounds recorded them.

The evidence re-runs this round's blocks are taken from: `/tmp/evidence-0004/r5/gate.txt` (§14's transcript,
offline, no turn billed, exit code `0`) and the three mutation runs quoted above, each executed on this tree
and restored.

### What this round changed in the files that carry the rules

- `docs/evidence/0004-repeatable-continuation-handoffs.md`: §7.1's elision sentence corrected, §13's quoted
  `parity` line restored to the run's own bytes and its sentence corrected (V01), and this section appended.
- `scripts/tests/test_gate_transcript_claims.py`: new — the machine-checked pin for the rules above, run by
  the `selftest` step. No gate step, no required step and no narrowing changed.
- `docs/harness.md`: the `scripts/tests/` bullet now names the new case beside `test_evidence_claims.py`.
- `docs/upstream.md` and the carried tree: untouched — the gate's `parity` step (above) is what proves it.

**`AGENTS.md` and bookkeeping:** this round changed no repository layout, no gate step, no command, no read
order and no rule an agent must follow — it corrected what this record's transcript claims and added a Python
case that reads the gate's own call — so `AGENTS.md` is unchanged. `docs/goals/` holds no goal file whose
implementation is merged-and-waiting: 0004 is this increment's own, still on its branch and awaiting operator
acceptance, so nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s status table is unchanged.

## 15. The sixth review round: findings, fixes, and the numbers re-measured

An independent review read the frozen range `18d50b2..956bf00` — the head this increment's records had
reached — and reported one finding, `P3`, about what this record says about the native check: §6's paragraph
and §13's numbers section both placed the comparison the fourth review round added ahead of *both* real
turns, where the shipped script takes it after the first conversion's loader and that session's real turn and
immediately before the copy grows. Both passages now state the shipped order, the order itself is re-derived
from the script's own calls, and each end of it is pinned by a case that drives the shipped harness — the
class (what this record claims about where that comparison is taken) had survived two rounds, so this round
closes it with executed mutations rather than an argument, and the two turn windows close a gap no earlier
case could see. The turned halves were not re-run, as both passages already disclose: two billed turns each.
The re-measured numbers are at the end of this section.

### W01 (P3) — the record placed the growth baseline ahead of both real turns

With both prompt flags, the shipped order is: the first conversion and its own comparison, that session's
native loader, the first session's real turn (`run_turn(repo, codex, home, installed, …)`), the comparison
this record calls the growth's baseline (`copy_at_growth = sha256(copy)`, then the check `the source copy
still holds the bytes the first conversion read`), `append_fragment(copy, fragment)`, and then the second
conversion, its loader, the second session's turn (`run_turn(repo, codex, home, second_installed, …)`) and
the closing comparisons of the fixture tree, the planted session and every session the home already held.
The two passages said the comparison runs ahead of the two turns, which misstates the interval its own
verdict answers for — the bytes the first conversion read, re-checked *after* that session was loaded and,
when `--resume-prompt` is given, resumed — and §6's justification for stating the turned count as arithmetic
rested on the same wrong reading. The offline half's own check order shows the placement on the tree this
round ran it on: the first session's turn check above the comparison, and the second session's below it
(lines 31–33 and 57 of `/tmp/evidence-0004/r6/native.txt`; the middle `…` is this record's own):

```console
SKIP: a real turn continues from the installed session — not attempted: pass --resume-prompt to run one
PASS: the source copy still holds the bytes the first conversion read — sha256 2bd510821d4abf83…, 2051 bytes, against the 2bd510821d4abf83…, 2051 bytes taken before the first conversion
PASS: the source copy grew by exactly the appended turn — 2051 -> 2540 bytes (+489 from internal/ccc/testdata/growth/claude-complete-turn.jsonl), sha256 e2921e995915855d…
…
SKIP: a real turn continues from the second installed session — not attempted: pass --repeat-resume-prompt to run one
```

**Fix.** §6's paragraph now says the comparison "sits between the two turns: it is taken after the first
conversion's loader and its own real turn, and immediately before the copy the second conversion reads grows
— hence ahead of the second conversion, its loader and its turn"; §13's numbers section states the same
placement and names the correction; both keep the disclosure that the turned halves were not re-run. The
claim is pinned from both sides:

- **behaviourally**, by two new windows in `scripts/tests/test_codex_native_check.py`'s `copy_mutation` table
  — `"first turn"` and `"second turn"`, driven by `drive`'s new `repeat_turn` flag (both prompt flags, the
  shape §6's transcript quotes) — each making its same-length swap while that turn runs and asserting the
  exact failure set the turn's own interval produces;
- **as a record claim**, by `RecordStatesTheChecksOwnOrder` in the same file: it reads the four stages'
  positions out of `scripts/codex-native-check.py` itself and fails when the script's order and this record's
  stated placement disagree.

### The close-out: the rules of this surface, each with its executed pin

The instruction is round 3's repeated: a class that survived two rounds leaves this round with a
machine-checked pin per rule, and every pin below was **executed** — the lines moved or reverted, the failure
observed, the file restored (this record's sha256 and the shipped script's matched before and after every
run). One property of the method was measured on the way and is documented at the loader that hit it
(`load_native_check`): a script imported by path is served from bytecode whose cache is keyed on the source's
mtime in *whole seconds* and its size, so two size-preserving mutations written within one second can make
the second run execute the first's code — the first run of the M2 mutation below did exactly that, and every
run quoted here was made with a fresh `PYTHONPYCACHEPREFIX`. Each run is a whole-module run of
`scripts/tests/test_codex_native_check.py` (34 cases), `Ran 34 tests`:

| rule | its pin | the mutation this round executed, and the failure observed |
| --- | --- | --- |
| the comparison taken where the copy grows runs after the first session's real turn | `test_a_copy_mutated_during_the_first_turn_is_caught_where_the_copy_grows` | the comparison moved up to just before `if args.resume_prompt:` — after the loader, before the turn: `AssertionError: Lists differ: [] != ['the source copy still holds the bytes the first conversion read']` is the run's only behavioural failure, while the first-load case stays `OK`, so the loader's window and the turn's are told apart |
| the same comparison, and every other comparison of the copy, is behind the second session's turn | `test_a_copy_mutated_during_the_second_turn_is_caught_by_the_final_comparison` | the closing block moved up to just before `if args.repeat_resume_prompt:`: `AssertionError: Lists differ: [] != ['the source session is unchanged']`, and that case is the run's only failure |
| this record's claim about that placement is the shipped script's own order | `RecordStatesTheChecksOwnOrder.test_the_record_states_the_order_the_shipped_script_runs_in` | §6's sentence reverted to the reading the review found, the placement re-worded away wherever the record states it (§6, §13 and this section's own restatement of the fix), and the script's order broken the other way (the comparison moved above the first turn): the case reports what it read and fails in all three — quoted below |

```console
### the comparison moved above the loader (and so above the turn): the first-load case, the first-turn case, the record case
AssertionError: Lists differ: [] != ['the source copy still holds the bytes the first conversion read']
AssertionError: Lists differ: [] != ['the source copy still holds the bytes the first conversion read']
AssertionError: Lists differ: [53524, 49742, 55447, 61030] != [49742, 53524, 55447, 61030]
 … the clause the run prints under that diff: the shipped script no longer runs the first session's turn,
 the comparison taken where the copy grows, the growth append and the second session's turn in the order this
 rule is about: the first session's turn at 53524, the comparison taken where the copy grows at 49742, the
 growth append at 55447, the second session's turn at 61030

### the comparison moved above the turn but left below the loader: the first-turn case and the record case
AssertionError: Lists differ: [] != ['the source copy still holds the bytes the first conversion read']
AssertionError: Lists differ: [53524, 53157, 55447, 61030] != [53157, 53524, 55447, 61030]
 … the same clause, with the comparison taken where the copy grows at 53157 and the first session's turn at
 53524 …

### the closing comparisons moved above the second turn: the second-turn case
AssertionError: Lists differ: [] != ['the source session is unchanged']

### §6's sentence put back to the reading the review found
AssertionError: 'runs *before* either' unexpectedly found in '# Evidence — 0004 Repeat continuation as
source history grows …

### the placement re-worded away wherever the record states it
AssertionError: "after the first conversion's loader and its own real turn" not found in '# Evidence — 0004
Repeat continuation as source history grows …
```

*(Each block is one run's own lines, in the order the run printed them: a heading naming the mutation, then
that run's failures. A failure is quoted by its own first `AssertionError:` line — the diff and the message
unittest prints below it are elided, as a line beginning `…` says — except the two record-case order failures,
whose appended clause is quoted from the run too; `…` also elides the document the two record readings echo.
The phrase the fourth block quotes stands inside a fence here on purpose: it is the reading this record
corrected, quoted from the run that found it, not the record speaking — the case reads the record's prose with
fenced runs dropped for exactly that reason.)*

The comparison's *interval* was the previous round's subject (§13's U01) and its two ends are this round's;
together, a copy the conversions read now has six windows, one case each, and the growth baseline is the one
comparison two of them answer (the first turn's and the first load's).

### The numbers this round re-measured

`python3 scripts/gate.py`, exit code `0`, the verdict line `GATE: PASS` with no `(partial: …)` suffix — every
step ran, including the two required ones:

```console
$ python3 scripts/gate.py
gate: <repo>
gate: go=~/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt    3592 file(s) gofmt-clean
  PASS     build    go build ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     vet      go vet ./cmd/ccc/... ./internal/ccc/... -> rc=0
  PASS     test     3 package(s) ok
  PASS     selftest Ran 207 tests in 5.327s (rc=0)
  PASS     parity   … | 47.0-181-g83195c8a) | reported by cmd/ccc: 83195c8a6861b8c74a4b6cb87512817fed6f8584 | carried: 5553 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 27 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 28 project-owned prefix(es); files seen by git: 5652 | inputs: internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared deviations still differ from upstream: .gitignore AGENTS.md README.md docs/README.md | declared stored bytes verified: frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     secrets  no leaks found in 103 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The block is the run's own lines from the command line through the verdict line: the two header lines are
this record's placeholders, as §7.1's scheme states (the run printed the account's home), the gate's own
trailing `Wall time:` line is not quoted, and the `parity` line is the gate's own rendering whole — its one
cut is the gate's own leading `…` — so every other character is what the run printed. This round ran the same
command repeatedly, the block being the first of its runs: every later transcript of it in
`/tmp/evidence-0004/r6/` — the `gate-*.txt` files there — printed the same steps and the same verdict, and each
of their details equals the block's except two fields — `selftest`'s wall clock (`5.327s` above; 5.40–5.64s
there) and `secrets`' count, which read `103` in every one of them except the two runs made while the case
cache the next paragraph names stood (`104`). One count moved from §14's run of the same steps: `selftest` from
`Ran 204 tests` to `Ran 207` (this round's two turn-window cases and the record-reading case joined
`scripts/tests/test_codex_native_check.py`); `gofmt`'s 3592, `parity`'s `files seen by git: 5652` and
`secrets … in 103 file(s)` are otherwise unchanged — this round added no file — and `selftest`'s wall clock
varies per run. `secrets` counts the ignored files this repository owns too (the `scripts/…/__pycache__/*.pyc`
an *ad-hoc* `python3 -m unittest` writes; the gate's own steps run Python with `PYTHONDONTWRITEBYTECODE=1`, so
they write none): this round's direct runs of `scripts.tests.test_gate_transcript_claims` wrote that case's
cache between the first two runs and the last one, and the newest run's `103` is the count once it was removed
again.

`scripts/tests/test_codex_native_check.py` on its own: `Ran 34 tests … OK` (31 before this round; the three
cases above joined it), and `Ran 34 tests … FAILED` under each of the five mutations the close-out table
names (the first-load window's case the only one that stays green under the third — the comparison below the
loader, above the turn — which is what tells that window from the turn's), with the failure sets quoted there.
No Go file changed this round, so the `test` step's packages and its `func Test…` count stand as §13 recorded
them.

`python3 scripts/codex-native-check.py` (the offline half, exit code `0`, no turn billed) reports
`26 check(s) passed, 0 failed, 2 skipped` — the same count §13 recorded, because this round changed no line
of the shipped check — and the placement its own order shows is quoted in W01 above. It was run twice —
`/tmp/evidence-0004/r6/native.txt` and, on the tree as it stands, `/tmp/evidence-0004/r6/native-final.txt` —
with that summary both times. The turned halves were not re-run, as §6 and §13 disclose.

The evidence re-runs this round's blocks are taken from: `/tmp/evidence-0004/r6/gate.txt` (§15's gate
transcript, with the later runs beside it), `/tmp/evidence-0004/r6/native.txt` (the offline half's check lines
and its summary), and `/tmp/evidence-0004/r6/mutations/` — `shipped.txt` and the transcripts of the five
mutations this section quotes (`m1_before_the_loader.txt`, `m2_before_the_closing.txt`, `m3_before_the_turn.txt`,
`n1_wrong_reading.txt`, `n2_placement_removed.txt`), plus `stale_cache_proof.txt`, the bytecode-staleness
demonstration the close-out describes — each executed on this tree and restored.

### What this round changed in the files that carry the rules

- `docs/evidence/0004-repeatable-continuation-handoffs.md`: §6's and §13's passages corrected (W01), and this
  section appended.
- `scripts/tests/test_codex_native_check.py`: the two turn windows (`copy_mutation`'s `"first turn"` and
  `"second turn"`, `drive`'s new `repeat_turn`), the record-reading case `RecordStatesTheChecksOwnOrder` with
  its `prose()` reading, and the measured bytecode hazard documented at `load_native_check`. No gate step, no
  required step and no narrowing changed.
- `docs/harness.md`: the `scripts/tests/` bullet now names the record-reading case beside the two that read
  the continuation evidence's claims.
- `docs/upstream.md` and the carried tree: untouched — the gate's `parity` step (above) is what proves it.

**`AGENTS.md` and bookkeeping:** this round changed no repository layout, no gate step, no command, no read
order and no rule an agent must follow — it corrected what this record says about the shipped check's own
order and pinned both ends of that order with Python cases — so `AGENTS.md` is unchanged. `docs/goals/` holds
no goal file whose implementation is merged-and-waiting: 0004 is this increment's own, still on its branch
and awaiting operator acceptance, so nothing moves to `docs/goals/archive/` and `docs/goals/README.md`'s
status table is unchanged.
