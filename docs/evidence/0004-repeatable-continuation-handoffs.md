# Evidence — 0004 Repeat continuation as source history grows

Contract: [`docs/goals/0004-repeatable-evolving-session-handoffs.md`](../goals/0004-repeatable-evolving-session-handoffs.md).
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
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex --target-home /tmp/evidence-0004/a1/home
--target-binary ccc-no-such-harness --verify none`, `CLAUDE_PROJECTS_DIR=<repo>/internal/ccc/testdata/fixtures/claude`,
exit code 0):

```console
claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 → codex
  source:       <repo>/internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
  read from:    source
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2051 byte(s) read; the read's own snapshot of this file is exactly these bytes
  records:      the source's records end at a line feed at byte 2051: the file ends at a record boundary
  converted:    codex-rollout-jsonl, 3101 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  carried-with-caveat: tool_name_vocabulary ×1
  carried-with-caveat: pre_compaction_history ×3
  omitted:      malformed_source_lines ×1
  installed:    /tmp/evidence-0004/a1/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e978-f9bb-7766-b15d-7f3135eede36.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e978-f9bb-7766-b15d-7f3135eede36
  working dir:  /home/user/code/example-api
  resume:       CODEX_HOME=/tmp/evidence-0004/a1/home ccc-no-such-harness resume 01a1e978-f9bb-7766-b15d-7f3135eede36
  non-interactive resume: CODEX_HOME=/tmp/evidence-0004/a1/home ccc-no-such-harness exec resume 01a1e978-f9bb-7766-b15d-7f3135eede36 '<prompt>'
  cli_version:  0.169.0 (writer-pin)
  structure:    verified (the inherited AgentsView Codex reader over the installed session)
  native load:  not-requested (the target harness's own loader (codex app-server, thread/resume))
                verification was not requested (--verify none)
  native resume: not-requested (one turn in the installed session through the target harness's non-interactive resume)
                verification was not requested (--verify none)
  carried:      AgentsView 83195c8a6861b8c74a4b6cb87512817fed6f8584, unmodified (scripts/upstream-parity.py)
```

The same run with `--json` (an isolated second run, `/tmp/evidence-0004/a2`), with the boundary, the
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
      "read_coverage": "verified",
      "read_coverage_detail": "the read's own snapshot of this file is 2051 byte(s) and the file holds 2051: the conversion read these bytes",
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
    "session_id": "01a1e978-f9da-7872-8e93-f626ecd07661",
    "path": "/tmp/evidence-0004/a2/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e978-f9da-7872-8e93-f626ecd07661.jsonl",
    "resume_command": "CODEX_HOME=/tmp/evidence-0004/a2/home ccc-no-such-harness resume 01a1e978-f9da-7872-8e93-f626ecd07661"
  }
}
```

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
  names the boundary each snapshot covers (the source file's sha256 and size, whether those are the bytes
  the read itself opened, and, for a source whose records are the lines a harness appends, where its last
  line-feed-terminated record ends), so two continuations of one growing session can be told apart and the
  newer one chosen. A file that ends without a line feed holds a record the source may still be writing:
  the block says so under 'unterminated:' and leaves what the read made of it to the read's own fidelity,
  so nothing the read did not read is presented as history. A source whose own bytes are a compressed
  container of the records -- Codex reads its compressed rollouts -- is reported by its bytes and digest
  with no record offset claimed for them, because no offset into such a file is a record boundary.
```

*(The paragraph as the final tree prints it: the apply stage's accepted findings qualified its last
sentence to a source whose records are appended lines — §7.)*

## 2. Two conversions of one growing source, by hand

A copy of the committed Claude fixture under `/tmp/evidence-0004/b/src`, converted, grown by the committed
fragment `internal/ccc/testdata/growth/claude-complete-turn.jsonl`, then converted again into the same
home — the scenario the goal's first clause names (exit code 0 for each run):

```console
### B0: the growing copy before the first conversion
bytes=2051 sha256=2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6

### B1: the first conversion
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2051 byte(s) read; the read's own snapshot of this file is exactly these bytes
  records:      the source's records end at a line feed at byte 2051: the file ends at a record boundary
  converted:    codex-rollout-jsonl, 3101 bytes: 5 message(s), 1 tool call(s), 1 tool result(s), 1 boundary(ies), 2 turn(s)
  installed:    /tmp/evidence-0004/b/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e939-1b2b-7e15-b068-2913ad8b9d72.jsonl [hard-link]
  source file:  unchanged (sha256 2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6, taken before and after the conversion)
  target id:    01a1e939-1b2b-7e15-b068-2913ad8b9d72
first installed: rollout-2026-05-04T09-15-00-01a1e939-1b2b-7e15-b068-2913ad8b9d72.jsonl sha256=c945f80f6c93bb49

### B2: the source gains one turn (the committed growth fragment)
bytes=2540 sha256=e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f

### B3: the second conversion of the same session
  conversation: 10 message(s), 4 user, 6 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     1 malformed line(s)
  boundary:     2540 byte(s) read; the read's own snapshot of this file is exactly these bytes
  records:      the source's records end at a line feed at byte 2540: the file ends at a record boundary
  installed:    /tmp/evidence-0004/b/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e978-fa5b-715b-be9d-8b90c2484ce3.jsonl [hard-link]
  source file:  unchanged (sha256 e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f, taken before and after the conversion)
  target id:    01a1e978-fa5b-715b-be9d-8b90c2484ce3

### B4: the earlier target after the second conversion
first installed now: rollout-2026-05-04T09-15-00-01a1e978-fa33-75f0-83c3-b397149d2fc7.jsonl sha256=ba2870988be17a2f
  rollout-2026-05-04T09-15-00-01a1e978-fa33-75f0-83c3-b397149d2fc7.jsonl
  rollout-2026-05-04T09-15-00-01a1e978-fa5b-715b-be9d-8b90c2484ce3.jsonl

### B5: the appended turn's words in each installed document
  rollout-2026-05-04T09-15-00-01a1e978-fa33-75f0-83c3-b397149d2fc7.jsonl: question=0 answer=0
  rollout-2026-05-04T09-15-00-01a1e978-fa5b-715b-be9d-8b90c2484ce3.jsonl: question=2 answer=2
```

Read as the four claims it is meant to support:

| Claim | Evidence in the block above |
| --- | --- |
| two conversions of one growing source are **distinguishable** | two sessions in one home, `01a1e978-fa33-…` then `01a1e978-fa5b-…`, each with its own path; two different boundaries (`2051` then `2540` bytes, `2bd51082…` then `e2921e99…`) |
| each is **correctly attributed** to its own bytes | B0's digest is B1's `boundary` digest and B1's `source file` digest; the copy's own digest after the growth (B2) is B3's `boundary` digest; the later read carries 10 messages against the earlier 8; both blocks state the read's own snapshot as the file they measured (`read_coverage: verified`) |
| the later result **includes the eligible appended history** | B3's `conversation: 10 message(s)`/`converted: 7 message(s)`/`3 turn(s)` against B1's 8/5/2, and B5: the appended question and answer appear twice in the second document and not at all in the first |
| the earlier target **stays unchanged** | B4: `sha256=ba2870988be17a2f` — the same digest it had immediately after B1, after the second conversion ran into the same home |
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
the copy: 2051 bytes -> 2182 bytes; the last 131 bytes hold no record terminator
  conversation: 8 message(s), 3 user, 5 assistant, 1 tool call(s), 1 compaction boundary(ies)
  fidelity:     2 malformed line(s), and the last record was still being written at read time
  boundary:     2182 byte(s) read; the read's own snapshot of this file is exactly these bytes
  records:      the source's records end at a line feed at byte 2051: the last 131 byte(s) are a record the source had not terminated
  omitted:      malformed_source_lines ×2
  installed:    /tmp/evidence-0004/c/claude/home/sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e979-1c49-7934-ae2d-e6bbd70aedbf.jsonl [hard-link]
  source file:  unchanged (sha256 95ce6c56aee411c05bd48dfbf966e63bd219a42295b6178ec83890a9482d9b3f, taken before and after the conversion)
  target id:    01a1e979-1c49-7934-ae2d-e6bbd70aedbf
  the unfinished record's words in the installed document: 0

### C2: a Codex source whose writer had not finished a record
the copy: 1792 bytes -> 1957 bytes; the last 165 bytes hold no record terminator
  conversation: 6 message(s), 2 user, 4 assistant, 1 tool call(s), 1 compaction boundary(ies)
  boundary:     1957 byte(s) read; the read's own snapshot of this file is exactly these bytes
  records:      the source's records end at a line feed at byte 1792: the last 165 byte(s) are a record the source had not terminated
  omitted:      message_model_attribution ×5
  installed:    /tmp/evidence-0004/c/codex/home/projects/-home-user-code-example-api/a652661e-01db-49a5-8238-533fc42ad433.jsonl [hard-link]
  source file:  unchanged (sha256 aa19c70a9c0042786070d0891f88f58c21fa678f7ccc73aa62bc48f6e553bbb3, taken before and after the conversion)
  target id:    a652661e-01db-49a5-8238-533fc42ad433
  the unfinished record's words in the installed document: 0
  the same source read by ccc inspect reports no malformed line at all:
    {"fidelity": {"read_from": "source"}, "messages": 6, "file_size": 1957}
```

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
| `TestTakeSourceBoundaryReportsAFileThatMovedWhileItWasRead` | the state a mid-read growth leaves, at the seam where it is visible (§11's F01): the read's own recorded snapshot is smaller than the file the pass measures, so the block reports `read_bytes`, `read_coverage: unverified` with both sizes in its detail, the file's own record structure beside it, and a warning naming the remedy — and the pass writes nothing to the file |
| `TestBoundaryWarningsNameTheUnterminatedRecordAndAMovedFile` | the warning channel: no boundary, a boundary that was not taken, and a file with neither an unterminated record nor a move produce no warning; an unterminated record produces one naming `1835`, `1792`, `43 byte(s)` and `converting again` and claiming no omission; an unverified coverage produces one naming both sizes (`1792 byte(s) read`, `1835 byte(s) now`) and the remedy; both answers together produce one each |
| `TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations` | the *declaration* half of the gate on the record offsets: `claude` and `codex` are declared line-recorded (`parser.SourceCapabilities.IncrementalAppend`), an unknown name and an empty one are not, and a reader-less call claims nothing. Three providers whose source is a whole-file JSON document or a compressed container (QwenPaw, CodeBuddy, DeepSeek Harness — each declaring only `ContentCapabilities.MalformedLineCount`) are not (§10: the review round found the gate accepting that count and superseding `Q-0004-01` with `Q-0004-07`) |
| `TestConversionOfAWholeFileJSONSourceClaimsNoRecordBoundary` | end to end (added in the review round, §10): a complete QwenPaw session — one whole-file JSON document, no trailing line feed — converts with `record_offsets: not-claimed` (the provider declares no such source), no record offset, no unterminated record and no unterminated-record warning, and the boundary digest is the file's own |
| `TestContinueFromACompressedCodexSourceClaimsNoRecordOffsets` | end to end (§11's F03): a runtime-compressed copy of the committed Codex rollout — Codex's own `.jsonl.zst` representation, the one its reader decodes — converts with the same conversation the plain rollout of the same bytes produces, while the boundary reports the compressed file's own size and digest, `record_offsets: not-claimed` with the container as the reason, no record offset, no unterminated record, and `read_coverage: verified` |
| `TestContinueCarriesACompleteRecordThatHasNoLineFeed` | end to end (§11's F02), in both directions: the committed growth fragment appended as committed and with its last byte (its line feed) removed produce the same conversation and the same carried counts — the reader reads a complete unterminated record — while only the second boundary reports an unterminated record, and its installed document holds the record's dialogue |
| `TestConversionFromAnArchiveReportsNoBoundaryToTake` | end to end: a conversion that fell back to the archive reports `not-taken`, no bytes, `source_check_state: not-verified` with the archive reason, and no unfinished-record warning |
| `TestContinueJSONCarriesTheSourceBoundary` | the CLI's `--json` document: `state`, `path` (the file the conversion read), `bytes` (the file's own size), `sha256` (an independently computed digest), `read_bytes`/`read_coverage` (the read's own snapshot is that file), `record_offsets: claimed` with no reason, `terminated_records_end_bytes`, and a non-empty `detail` |
| `TestContinueHumanOutputStatesTheSourceBoundary` | the human block, on a real conversion and on the five shapes (complete, unterminated record, compressed container, moved file, not-taken, undeclared source), including that a boundary that claims no offsets prints no `records end at` line, that the unterminated case claims no omission, and that a report with no boundary prints no `boundary:` line at all |
| `TestFixturesMatchTheirGenerator` | the committed fixtures **and** the growth fragments are byte-for-byte what `internal/ccc/fixturegen` writes, so a fragment cannot drift from the shape the runs above appended |

All of them pass (`--count=1`, exit code 0):

```console
--- PASS: TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot (0.37s)
--- PASS: TestUnpublishedSourceTailIsBoundaryedAndNeverPublished (0.03s)
--- PASS: TestSourceBoundaryOfFileStatesTheRecordOffsets (0.00s)
--- PASS: TestTakeSourceBoundaryAnswersTheReadsThatHaveNoFileToMeasure (0.00s)
--- PASS: TestBoundaryWarningsNameAnUnfinishedRecord (0.00s)
--- PASS: TestSourceRecordsAreLinesAsksTheProvidersOwnDeclarations (0.00s)
--- PASS: TestConversionFromAnArchiveReportsNoBoundaryToTake (0.14s)
--- PASS: TestFixturesMatchTheirGenerator (0.18s)
PASS
ok  	go.kenn.io/agentsview/internal/ccc	0.782s
--- PASS: TestContinueJSONCarriesTheSourceBoundary (0.36s)
--- PASS: TestContinueHumanOutputStatesTheSourceBoundary (0.01s)
--- PASS: TestContinueHelpIsARequestNotAFailure (0.01s)
PASS
ok  	go.kenn.io/agentsview/cmd/ccc	0.428s
```

## 5. What the boundary and the repeat carry, source side and target side

| Source evidence | Target side, and the run that recorded it |
| --- | --- |
| the source file's own bytes at the boundary (`2051` bytes, `sha256 2bd510821d4abf83…`) | `source.boundary` in the conversion's JSON and the `boundary:` line in its human block; the same digest in `installation.detail` (§1) |
| the appended turn (the committed growth fragment) | present twice in the second Codex rollout and absent from the first — §2's B5 counts two matches of the appended question in the later document and the same two for the words that replied to it, against none of either in the earlier one; in-process, the same holds for the Claude Code target (`TestContinueRepeatedOnAGrowingSourceIsAttributedToItsOwnSnapshot`) |
| the appended fact, as the **answer** of a real turn in the *later* target | the recorded turn's answer holds it: the second answer block quotes `120 seconds per session, held in` (§6) |
| the converted history before the growth, as the **answer** of a real turn in the *earlier* target | the recorded turn's answer names `maxBackoffSeconds` as the fact the converted history supplied, in the first answer block (§6) |
| an unterminated trailing record (`165` bytes with no line feed) | `boundary.unterminated_tail_bytes`, the `records:` line and the run's warning; no message of the conversation and no record of either target document holds its text (§3) — and where the record *is* a complete one, the conversion carries it while the boundary still states the byte fact (§11's F02) |
| an earlier conversion of the same source | unchanged by the second conversion (`sha256 c945f80f6c93bb49` before and after, §2 B4) — and unchanged in the native run after the first session had itself been resumed by Codex (§6) |
| the source file during and after every run | `source file: unchanged (sha256 …, taken before and after the conversion)` in each CLI run (§1–§3); in the native check, the copy and the committed fixture tree are digested around each conversion and compared once more at the end of the run, after the second loader and the optional turn (§6, §10) |

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

**The offline run** (`python3 scripts/codex-native-check.py`): `25 check(s) passed, 0 failed, 2 skipped`.
Its header and the checks that make the clause material:

```console
source:      internal/ccc/testdata/fixtures/claude/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl
copy:        source/-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl (the fixture's bytes; this is what grows)
growth:      internal/ccc/testdata/growth/claude-complete-turn.jsonl (489 bytes)
PASS: the conversion reports the boundary of the source copy it read — boundary={"state": "taken", … "bytes": 2051, "sha256": "2bd510821d4abf83235752eff81e0e9824d0c41b3d2d4b84956d1130992467b6", "read_bytes": 2051, "read_coverage": "verified", … "record_offsets": "claimed", "terminated_records_end_bytes": 2051}; 9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl is 2051 bytes, sha256 2bd510821d4abf83…
PASS: the source copy is unchanged by the conversion — sha256 2bd510821d4abf83…, 2051 bytes, taken before and after
PASS: the target harness's own loader opens the installed session — thread/resume -> 01a1e97a-befb-7e96-baf4-9771095d86ca [legacy] cwd=/home/user/code/example-api
PASS: the source copy grew by exactly the appended turn — 2051 -> 2540 bytes (+489 from internal/ccc/testdata/growth/claude-complete-turn.jsonl), sha256 e2921e995915855d…
PASS: the second conversion reports the boundary of the grown source copy — boundary={"state": "taken", … "bytes": 2540, "sha256": "e2921e995915855d41b1b945e847ed751a8e92d9d053ae19e5c05ac19c81a02f", "read_bytes": 2540, "read_coverage": "verified", … "record_offsets": "claimed", "terminated_records_end_bytes": 2540}; …; the first conversion's boundary was {"state": "taken", …}
PASS: the second conversion's identity is new, not the source's and not the first session's — 01a1e97a-c116-718a-96b0-2af34635a77a at sessions/2026/05/04/rollout-2026-05-04T09-15-00-01a1e97a-c116-718a-96b0-2af34635a77a.jsonl
PASS: the second conversion carries the history the source gained — carried user messages 2 -> 3, assistant messages 3 -> 4; the appended turn's own words are in the second document and not in the first
PASS: the first conversion's session is unchanged by the second — sha256 9914968c479b4dea… before and after the second conversion
PASS: the source copy is unchanged by the second conversion — sha256 e2921e995915855d…, 2540 bytes, taken before and after the second conversion
PASS: the target harness's own loader opens the second installed session — thread/resume -> 01a1e97a-c116-718a-96b0-2af34635a77a [legacy] cwd=/home/user/code/example-api
PASS: the target's own view of the second session lists the appended turn — 7 item(s) in the reply's own turn list; the appended turn's own words are in its own view
PASS: the first session's own view does not hold the appended turn — the first session's own view holds 5 item(s), none of them the turn the source gained after it was converted
conversions: 2 session(s) installed (01a1e97a-befb-7e96-baf4-9771095d86ca then 01a1e97a-c116-718a-96b0-2af34635a77a); real turn(s) run: 0
PASS: the source session is unchanged — sha256 2bd510821d4abf83… (fixture tree identical, internal/ccc/testdata/growth/claude-complete-turn.jsonl identical; the converted copy identical too: sha256 e2921e995915855d…, 2540 bytes)
PASS: the unrelated session in the target home is unchanged — sha256 59ff49ca30baf622…
PASS: no pre-existing session in the target home changed — 0 pre-existing session file(s)
25 check(s) passed, 0 failed, 2 skipped
```

*(The block above is re-taken from the second review round's own re-run of the offline half on the fixed
tree (§11): the same `25 check(s) passed, 0 failed, 2 skipped` and the same checks, the same committed
fixture and fragment digests, and the boundary lines as the fixed tree prints them — `read_bytes`,
`read_coverage`, `record_offsets` and `terminated_records_end_bytes` in place of the pre-fix spellings —
with this run's own session ids. §10 records why the final comparison grew; §11 records what changed.)*

Codex's own `thread/read` view of the *second* session, printed by the run (its own view of the later
snapshot, with the turn the source gained as its last turn):

```console
the target harness's own view of the second installed session:
    thread 01a1e97a-c116-718a-96b0-2af34635a77a [legacy] provider=openai
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
`27 check(s) passed, 0 failed, 0 skipped`, exit 0, two turns billed (the run's own line says so). The turn
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
conversion, the committed fixture tree and the growth fragment are compared at the end
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
pin the four rules this half stands on:

| Case | What it pins |
| --- | --- |
| `test_an_intact_run_passes_the_comparisons` | every new comparison is present and `PASS` (13 names in `REPEAT_CHECKS`), the only `SKIP`s are the turns nobody asked for, and the attribution comparisons name what they compared (`the first conversion's boundary was`, `before and after the second conversion`) |
| `test_a_mutation_during_the_resume_stage_is_caught_by_the_last_comparison` | the three protected-artifact comparisons run last and fail exactly then; the repeat half still passes, because the conversions read the copy rather than the mutated fixture |
| `test_a_first_session_mutated_by_the_second_conversion_is_caught` | the comparison about the earlier session runs **after** the second conversion: a second conversion that touched it fails exactly that comparison |
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
    same numbers §6 records.
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

The leading `…` inside the `parity` line is the gate's own elision of a long step detail (its `tail()`
keeps the last 2 500 characters and marks the cut), not an edit of this record: every character below is
what the run printed. The block is one run's own output, quoted as it printed; a later run on this tree
prints the same steps, the same counts and the same verdict apart from two fields, each stated with its
cause: `selftest`'s wall-clock seconds (5.222s above; 12.579s and 6.584s on two re-runs), and the `gofmt`
step's file count, which is the count of *tracked* `*.go` files — the run above was made while this
increment's new `internal/ccc/continue_growth_test.go` was untracked, so it printed the pre-increment
tree's `3591`, where the committed tree prints `3592` (`git ls-files '*.go' | grep -v '^vendor/' | wc -l`).
Every other field of the re-run, including `parity`'s `files seen by git: 5650`, was byte-identical to the
block above.

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
- **The boundary answers the read-coverage question only where the recorded size may be read as one file's**
  (Q-0004-08, superseding Q-0004-05). The comparison is asked for the providers whose declarations make the
  read's recorded size this file's own, so a layout provider's composite size is never compared with one
  file's — and a source that grows while it is being read is reported as `read_coverage: unverified` with
  both sizes, instead of the report attributing the conversation to bytes the read is not shown to have
  covered. The window itself is not closed: a file that grows during the read is still read only as far as
  the reader's own snapshot, and the report names it rather than repairing it.
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
  the offline default (`25 check(s) passed, 0 failed, 2 skipped`) runs none.
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
gate: /home/azureuser/repos/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go targets=./cmd/ccc/... ./internal/ccc/...
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
