# 0009a (v2) — the native oracle, re-scoped — evidence

Increment: the re-scope of [goal 0009a](../goals/0009a-native-oracle.md), reactivated by this record's
state. Plan written before implementation: [`docs/plans/0009a-native-oracle-v2.md`](../plans/0009a-native-oracle-v2.md).
The superseded attempt (PR #19, closed unmerged, sixteen review rounds) is preserved at tag
`archive/0009a-pre-rescope` (`8787ae6484`); the deletion list in §2 is derived by reading that attempt's
diff, never from memory.

Every command below ran in the task worktree `/home/azureuser/claw/tasks/2026-11-14/call-ccc/wt-s2/call-ccc`
(branch `feat/0009a-native-oracle-v2`, cut from `origin/main` = `5c2b9c237451768fa7409870d3ded8eb023bd840`),
on 2026-11-23, with every edit uncommitted for the operator. Nothing carried is touched; no Go file, gate
step or CI workflow changes.

## 1. What shipped

**The offline oracle is now the container and the receipt, and nothing else.** Its registry
(`scripts/codex-native-check.py`'s `ORACLE_RULES`) holds four format-agnostic rules:

| id | subject |
| --- | --- |
| `oracle.utf8` | valid UTF-8 text, opening with no byte-order mark |
| `oracle.whole-lines` | whole lines: the empty artifact, or bytes whose final byte is a line feed |
| `oracle.jsonl-objects` | every line exactly one JSON object, member names unique, no unpaired surrogate escape |
| `oracle.receipt-integrity` | a receipt's byte count, whole-file SHA-256 and one anchor per line, against the artifact's bytes |

The reading is staged (the byte layer first: a violation there ends the reading), the receipt is a
committed document beside the frozen artifact (`scripts/tests/testdata/codex-oracle/artifact.jsonl` +
`artifact-receipt.json`), and `--selftest-mutations` proves the declared set equals the mutated set —
**4 rules, 16 fixture pairs, each with its three arms** (§3). The fixture models no agent's format on
purpose: it is the artifact a case proves the rules accept while the binary refuses it (§4).

**The Codex judgment is the binary's.** The native half is unchanged except for two additions: every run
records the resolved binary (the owner's answer — follow `latest`, no version re-pin — so the run
records `codex --version` and the binary's own SHA-256), and the **disclosure arm** plants the oracle's
frozen artifact in the scratch home and records the pair *offline rules accept / the harness refuses*,
with the harness's own words in the detail. The loader, resume and turn checks are the file's own,
unchanged; nothing in Python restates what they answer.

## 2. The deletion list

**How it was derived.** `git diff 8989d2c..archive/0009a-pre-rescope --stat` names the attempt's
increment (`scripts/codex-native-check.py` +3025, `scripts/tests/test_codex_native_check.py` +1827, five
fixture files, plus the docs of its own era). Its registry and its mutation corpus were read from the
tag's own bytes (`archive/0009a-pre-rescope:scripts/codex-native-check.py`) and enumerated
programmatically: **16 rules, 68 mutation pairs**. Each rule whose subject is a Codex member, event
vocabulary, record shape or replay ordering is deleted here, with its fixtures and mutations — 15 rules,
62 pairs.

**Deleted rules** (id — positive fixture(s) — mutations, by label):

- **`oracle.envelope`** — `session.jsonl` — 7: `a record without its timestamp`, `a payload that is not an object`, `a record without its type`, `an offset the format forbids`, `an instant at the leap second`, `an instant with a trailing line feed`, `an instant written with a non-ASCII digit`.
- **`oracle.record-type`** — `session.jsonl` — 1: `a record type the format does not define`.
- **`oracle.session-meta`** — `session.jsonl` — 4: `the metadata record deleted`, `a second metadata record`, `every record deleted`, `a record standing before the metadata`.
- **`oracle.session-meta-members`** — `session.jsonl` — 3: `an empty id`, `no cli_version`, `no originator`.
- **`oracle.clock`** — `session.jsonl` — 1: `two adjacent instants swapped`.
- **`oracle.message`** — `session.jsonl`, `session-media.jsonl` — 9: `a message with no content`, `a block type the native format does not define`, `a block typed with the reader's spelling the format lacks`, `a message whose text is blank`, `a message with no role`, `a block whose text is not a string beside a valid one`, `a media block without its reference member`, `a media block whose reference member is not a string`, `a media block whose detail is outside the version's spellings`.
- **`oracle.event-message`** — `session.jsonl` — 2: `an event message that is not a string`, `an event message that is blank`.
- **`oracle.call-pairing`** — `session.jsonl` — 5: `an output naming no call`, `a call id carried twice`, `an output written before its call`, `a call without a usable name`, `a named call whose call_id is empty`.
- **`oracle.call-members`** — `session.jsonl` — 6: `a call's recorded input renamed to the other family's member`, `a call output without its output member`, `a call's recorded input written as an object`, `a custom call's recorded input written as an object`, `a call output written as null`, `a call output written as an array of bare strings`.
- **`oracle.compacted`** — `session.jsonl`, `reduced.jsonl` — 2: `a boundary whose message member is deleted`, `a boundary whose message member is not a string`.
- **`oracle.receipt`** — `session.jsonl` + `session-receipt.json` — 4: `a disposition the vocabulary does not hold`, `a result entry with no call_id`, `a result entry with no recorded text`, `a receipt that declares no occurrences`. *(The manifest receipt's schema — occurrence kinds, identities, dispositions. The surviving `oracle.receipt-integrity` is a different subject with a new id; no manifest-receipt clause and no id carries over.)*
- **`oracle.plan-emitted`** — `session.jsonl` + `session-receipt.json` — 2: `a message the plan emits and the artifact lost`, `an emitted message whose turn-view event is gone`.
- **`oracle.plan-accounted`** — `session.jsonl` + `session-receipt.json`, `reduced.jsonl` + `reduced-receipt.json` — 13: `an occurrence the plan marks omitted and the artifact holds`, `an occurrence the plan names nowhere`, `a boundary the plan names nowhere`, `a boundary carrying the dialogue the plan omits`, `a boundary's retained checkpoint carrying the dialogue the plan omits`, `a retained_context record carrying the dialogue the plan omits`, `a message role the receipt's vocabulary cannot name`, `an omitted message that survives in the turn view`, `a call carrying both recorded-input members`, `a result's recorded text written as a typed content-item array`, `a message carrying a media block the receipt cannot name`, `an event that rolls a recorded turn back`, `a session_meta carrying explicit fork lineage`.
- **`oracle.plan-order`** — `session.jsonl` + `session-receipt.json` — 2: `a compaction boundary moved past the later turns`, `two prompts swapped between the turns' view events`.
- **`oracle.plan-pairs`** — `session.jsonl` + `session-receipt.json` — 1: `a call emitted while its recorded result is omitted`.

**Kept, re-narrowed.** `oracle.jsonl-objects` survives (its subject — JSON framing and duplicate
reader-answerable shapes — is not Codex's), with its five framing mutations carried onto the new fixture
(`a line that is not JSON`, `a line that parses as an array`, `a line holding a non-JSON constant`,
`a line nested past reading`, `a line holding a member name twice`); its sixth pair's old subject named
Codex call ids, so the pair is re-shaped onto the new fixture's own note string as `a line holding an
unpaired surrogate escape`.

**Added** (the premise's other two subjects): `oracle.utf8` (2 pairs: `a byte that is not UTF-8`,
`a byte-order mark`), `oracle.whole-lines` (1 pair: `a record left unterminated`),
`oracle.receipt-integrity` (7 pairs: `an artifact byte changed under an unchanged receipt`,
`a receipt that states another whole-file digest`, `a receipt whose byte count is wrong`,
`a receipt naming no lines`, `a receipt naming one line fewer`,
`a receipt whose last anchor is another line's`, `a receipt that is not an object`). Corpus now: 4 rules,
16 pairs.

**Fixtures.** Deleted with their subjects: `scripts/tests/testdata/codex-oracle/session.jsonl`,
`reduced.jsonl`, `session-media.jsonl`, `session-receipt.json`, `reduced-receipt.json` (three synthetic
sessions of 17/12/17 lines and two manifest receipts). Added in their place: `artifact.jsonl` (three
JSON objects, no agent's format) and the `artifact-receipt.json` that names its bytes.

**Test cases.** The attempt's oracle class held 37 cases over the deleted rules, its manifest receipts
and its reader model; the reduced file's oracle class holds 14, all of them over the survivors and the
premise's two negative spaces (D2's container-valid-but-not-Codex artifact, D2's vocabulary scan), the
D1 corpus read through the shipped command, and D4's two receipt cases. The rest of the suite
(`ProtectedArtifactsAfterNativeVerification` and the file's other classes) is the base's own; two of its
stubs gained the two host probes this increment's native half now makes (`codex --version`, and the
loader's refusal of a session it does not hold — the disclosure arm's own subject), and one existing
assertion ("1 pre-existing session file(s)") stands because the disclosure arm's planted file, like the
unrelated session, is the harness's own fabrication rather than the caller's.

**Documentation.** The attempt's `docs/harness.md` paragraph describing sixteen rules and the manifest
receipt is replaced by the reduced oracle's description plus the disclosure (§4); its AGENTS.md edits of
that era are not part of this base (the ownership rule is already there). The plan stub the decomposition
left for this goal (`docs/plans/0009a-native-oracle.md`, whose increment is the superseded attempt) is
reduced to a pointer at this increment's plan, so no plan file directs the superseded premise; its older
text is in git history and the attempt's own trees at the tag.

## 3. The runs

**The oracle's closure** (`--selftest-mutations`, exit 0; each arm driven through the shipped command's
own `--validate`/`--receipt`/`--disable-rule` path):

```
oracle.selftest: PASS — 4 rule(s), 16 fixture pair(s): positive PASS, negative FAIL (each citing its own
rule), --disable-rule PASS
```

**The verdict paths**, with real exit codes:

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate /tmp/mutated.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json       # one byte of line 3 changed
oracle: oracle.receipt-integrity — the receipt states sha256 855ccca11024…, the artifact's bytes are sha256 8cf233599e61…
oracle: oracle.receipt-integrity line 3 — the line's bytes are sha256 595a00578a9b…, the receipt's anchor is sha256 ac584578fe8b…
oracle: FAIL — 3 record(s), 2 violation(s)                                    # exit 1
$ python3 scripts/codex-native-check.py --validate /tmp/any.jsonl             # {"anything":"goes"} ×3
oracle: PASS — 2 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate /tmp/bad.jsonl             # a 0xFF first byte
oracle: oracle.utf8 — the artifact is not UTF-8 text: byte 0xff at offset 0
oracle: FAIL — 0 record(s), 1 violation(s)                                    # exit 1
$ python3 scripts/codex-native-check.py --validate /tmp/no-such-file.jsonl
oracle: INCOMPLETE — the artifact /tmp/no-such-file.jsonl cannot be read: …   # exit 2
```

**The suite** (`selftest`'s own command, narrowed to this file; the whole discovery runs in §5):

```
$ python3 -m unittest discover -s scripts/tests -p 'test_codex_native_check.py'
Ran 48 tests in 0.181s
OK
```

**The native run** (the Codex judgment is the binary's; `python3 scripts/codex-native-check.py --json`,
exit 0). The header it records — this run's resolved binary, which is the only place a version is named,
because the owner's answer is to follow `latest`:

```
codex:       /home/azureuser/.local/share/mise/installs/codex/latest/bin/codex (codex-cli 0.171.0, sha256 b76a027d6b210f9a8da7aaf321dd235c700f0ef7a794d4a5314b9af6c134f0f0)
```

its counts — `27 check(s) passed, 0 failed, 2 skipped` (the two skips are the real turns, which need
model access and no prompt flag was passed) — and the disclosure arm's own line, verbatim:

```
PASS: the harness refuses an artifact the offline rules accept — the offline rules: green; thread/resume answered: thread/resume failed: failed to read thread: thread-store internal error: failed to read session metadata …/sessions/2026/01/01/rollout-2026-01-01T00-00-01-00000000-0000-7000-8000-00000000a009.jsonl: rollout at … is empty
```

**The absent-binary arm** (D3's other half — the reason, and a non-zero exit, never a guess):

```
$ python3 scripts/codex-native-check.py --codex definitely-not-a-binary
definitely-not-a-binary is not on PATH: this harness verifies native behaviour, which needs the target harness installed
# exit 2
```

## 4. The disclosure, side by side

The two answers for one artifact's bytes are in §3: the offline rules hold the frozen artifact green, and
the binary that owns the format refuses it. That is the re-scope's stated limit, not a defect: the offline
half can no longer detect a Codex artifact the binary would reject, by construction — it holds no Codex
knowledge at all. It is stated where a reader meets it: the goal file's own "What this goal does not
claim" paragraph (under *Intended advance*) and its *Non-scope*, this
record, the checker's own section header and module docstring, and `docs/harness.md`'s native-check
section (which is also the CI note: the gate never runs the native half — `scripts/gate.py` decides the
offline half, and the native checks are deliberately not gate steps).

## 5. The debt re-read (D5)

`docs/core-design/README.md` §2's declared-debt entries were re-read against this increment's surface
(it deletes a Python oracle, adds container/receipt rules and two native-half report lines; no Go file,
no other native check). Each is **kept** with its condition unchanged:

| entry (condition) | site | why kept |
| --- | --- | --- |
| `Q-0005-18` | `ccc/internal/ccc/pi_source_leaf.go` | no Go change here; the leaf check's behaviours are untouched |
| `Q-0008-10` | `scripts/opencode-native-check.py` | that file is not touched by this increment |
| `Q-0007-13` | the same file's Claude arm | not touched |
| `Q-0005-20` | `scripts/pi-native-check.py` | not touched |
| `Q-0005-19` | `ccc/internal/ccc/pi_verify.go`, `omp_verify.go`, the Pi/OMP checks | not touched |
| `Q-0003-12` | `ccc/internal/ccc/route_shared.go` | not touched |

No new entry is owed: the surviving rules judge no foreign format, and the arm that asks the binary is a
read of the binary's own answer, which is what the ownership rule asks for. The defaults this increment
took where the brief was silent are recorded in `docs/decisions/OPEN-QUESTIONS.md` as `Q-0009a-01`–`03`.

## 6. The gate, on the state this increment leaves

`python3 scripts/gate.py`, exit 0 — every step ran, none skipped, bounded or narrowed:

```
gate: /home/azureuser/claw/tasks/2026-11-14/call-ccc/wt-s2/call-ccc
gate: go=/home/azureuser/.local/share/mise/installs/go/1.27.4/bin/go module=ccc targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt       3624 file(s) gofmt-clean
  PASS     build       go build ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     vet         go vet ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     test        3 package(s) ok (in ccc)
  PASS     portability pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) | module ccc built and tested against the pristine checkout at /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584 | ran with -modfile=portability.mod (replace -> /home/azureuser/.cache/ccc-upstream/83195c8a6861b8c74a4b6cb87512817fed6f8584) and portability.sum beside it | go build ./cmd/ccc/... ./internal/ccc/... -> rc=0 | 3 package(s) ok
  PASS     selftest    Ran 503 tests in 13.932s (rc=0)
  PASS     parity      … | : 83195c8a6861b8c74a4b6cb87512817fed6f8584 | layout: carry root agentsview/, this repository's module ccc | carried: 5579 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 1 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 0 project-owned prefix(es), plus the carry directory's own boundary; files seen by git: 5775 | inputs: agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by agentsview/internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared stored bytes verified: agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations   docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009a-citations.tsv: 2 row(s) verified
  PASS     secrets     no leaks found in 195 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The two steps this increment is about are `selftest` — whose discovery runs `CodexArtifactOracle` (the
corpus's 16 pairs through the shipped command, the negative space, the receipt cases) — and `citations`,
which verifies the goal's two rows against the Go tests they cite. The table is the run taken before this
§6 was written; it was repeated afterwards on the tree that holds this record, and printed the same
unqualified `GATE: PASS`.

## What this record does not claim

- That the offline rules can detect a Codex artifact the binary would reject. They cannot, by
  construction, and §4 states it where a reader meets it.
- That a gate run exercises the native half. It never has and does not now: the native checks need the
  Codex binary installed (and model access for a real turn), and they stay re-runnable evidence rather
  than gate steps. The two turns in the native half were therefore skipped in §3's run, and the run says
  so (`2 skipped`) instead of reading as a full one.
- That the offline rules are complete about JSONL in general. They state the container this route writes
  — whole UTF-8 lines, one JSON object per line, no duplicate members, no unpaired surrogate escapes —
  and a blank line is refused rather than skipped; a choice the later increments can revisit, which is
  what the oracle lock freezes until goal 0009 ends.
- That the recorded `0.171.0` is any kind of pin. The owner's answer is to follow `latest`; the version
  and digest in §3 are what this host resolved on 2026-11-23, and the next run records its own.
