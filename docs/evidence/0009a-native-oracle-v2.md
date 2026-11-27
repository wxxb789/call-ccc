# 0009a (v2) — the native oracle, re-scoped — evidence

Increment: the re-scope of [goal 0009a](../goals/0009a-native-oracle.md), reactivated by this record's
state, **re-cut 2026-11-26 (owner option C)**. Plan written before implementation:
[`docs/plans/0009a-native-oracle-v2.md`](../plans/0009a-native-oracle-v2.md).
The superseded attempt (PR #19, closed unmerged, sixteen review rounds) is preserved at tag
`archive/0009a-pre-rescope` (`8787ae6484`); the deletion list in §2 is derived by reading that attempt's
diff, never from memory.

**The re-cut, and what this record describes.** Five cross-family review rounds (lane rounds r1–r5) on
this increment's pre-re-cut heads returned defects in the same three classes, severity did not decay (a
P1 every round), and the reviewed diff grew 171 KB → 364 KB. The owner took the re-decomposition
decision (option C): the **disclosure arm** and the **binary fingerprint** — the two features whose
footprint kept moving — move out into [goal 0009e](../goals/0009e-native-check-hardening.md), whose
reviewed starting point is tag `archive/0009a-r16-native-arm` (head `768df19`). The round history in
§1–§3 below is **the pre-re-cut tree's** — kept because it is where the moved features' fixes and the
oracle's own rounds were measured — while every count and quoted run the re-cut changes is
**re-measured on the re-cut tree** and marked as such. 0009a now ships: the offline oracle (unchanged by
the re-cut; the 2026-11-26 r7 fix round's two reading refinements — the receipt document's stated bound,
superseded by the r11 round's unmaskable refusal (§3, `Q-0009a-15`) — and the corpus's own verdict
assertions — and the r8 fix round's receipt-statement refinement are §1 and §3), the native load and
real-turn checks (the base's own, minus the two moved additions; the r8 round
splits the resume verdict from the read failure), and the three round-5 findings that live in the shipped
half — the corpus's declared unjudged rules, the receipt's three-member read, and the turn count. Every diff
that fixes a reproducible defect of a surviving path is kept: the dangling-symlink crash in a caller's
`--codex-home`, the portable held-name rule for the exclusive create, and the launch rule for a shim —
the path is never `resolve()`d, no component is normalized since the r7 round, and since the r11 round
the spelling answered is the one the host dialect's own join onto the process's directory leaves alone,
so a drive-less rooted path is anchored to this process's drive on every CPython minor (§3,
`Q-0009a-16`). The moved decisions are
`Q-0009a-05`, `-07` and `-09`, and the re-cut itself is `Q-0009a-10`
(`docs/decisions/OPEN-QUESTIONS.md`).

Every command below ran in the task worktree `<worktree>`
(branch `feat/0009a-native-oracle-v2`, cut from `origin/main` = `5c2b9c237451768fa7409870d3ded8eb023bd840`),
with every edit uncommitted for the operator. Nothing carried is touched; no Go file, gate
step or CI workflow changes.

## 1. What shipped

**The offline oracle is now the container and the receipt, and nothing else.** Its registry
(`scripts/codex-native-check.py`'s `ORACLE_RULES`) holds four format-agnostic rules:

| id | subject |
| --- | --- |
| `oracle.utf8` | valid UTF-8 text, opening with no byte-order mark |
| `oracle.whole-lines` | whole lines: the empty artifact, or bytes whose final byte is a line feed |
| `oracle.jsonl-objects` | every line exactly one JSON object, member names unique, no unpaired surrogate escape; a line nested deeper than 256 levels is refused by the rule's own stated bound, not by the reading interpreter's recursion limit; integers are read by their written digits, never converted, so the interpreter's integer-conversion limit decides no verdict |
| `oracle.receipt-integrity` | a receipt's byte count, whole-file SHA-256 and one anchor per line, against the artifact's bytes — the anchors the two sides share are compared even where the counts differ, so a moved delimiter byte still names the line it diverged on; the rule judges the statements of a receipt the reading could read, and a receipt document nested deeper than 256 levels is an unreadable *input* — `oracle: INCOMPLETE` (exit 2) before any member is read, which no `--disable-rule` can mask (`Q-0009a-15`) (a receipt covers an artifact holding at least one line; the empty artifact is not receipt-identified by this rule) |

The reading: **every rule is evaluated, or explicitly `NotEvaluated`.** `ORACLE_RULE_INPUTS` declares
what each rule reads — `oracle.utf8`, `oracle.whole-lines` and `oracle.receipt-integrity` read the
artifact's own bytes (the line rule included: the pre-r3 reading gated it on the decoded text, so a
masked byte in an unterminated record hid the line rule's own violation), the receipt wherever one was
given (the r2 round's refinement) — while `oracle.jsonl-objects` is `needs_text` and is reported as *not
evaluated* (naming `oracle.utf8`), never as an absent violation, where the byte layer answered no text.
`--disable-rule` is a set subtraction *after* evaluation, never a reason to skip one: a verdict is PASS
only where every rule was evaluated and no violation remains, FAIL where violations remain, and
INCOMPLETE (exit 2) where none remains but a rule was left unjudged. The corpus is part of the seam —
for every pair of byte rules one mutation violates both, declared in each rule's own arm, so disabling
either still cites the other. The receipt is a
committed document beside the frozen artifact (`scripts/tests/testdata/codex-oracle/artifact.jsonl` +
`artifact-receipt.json`), and `--selftest-mutations` proves the declared set equals the mutated set —
**4 rules, 22 fixture pairs, each with its three arms** (a pair whose bytes are known to break a second
rule names it, the pair's `also_cited` set, and still cites exactly it with the pair's own rule
disabled; §3). The fixture models no agent's format on
purpose: it is the artifact a case proves the rules accept, and the moved disclosure arm (goal 0009e) is
where the binary's refusal of these same bytes was measured (§4).

**The Codex judgment is the binary's.** On the pre-re-cut tree the native half carried two additions —
the resolved-binary fingerprint and the **disclosure arm** (it plants the oracle's frozen artifact in
the scratch home and records the pair *offline rules accept / the harness refuses*); both were re-cut
into goal 0009e on 2026-11-26. The loader, resume and turn checks are the file's own, unchanged; nothing
in Python restates what they answer. The 2026-11-24 cross-family review round
tightened three of the native half's own readings: a plant path holding anything — a caller's *dangling*
symlink included — is a collision, never written through or removed; the refusal is the harness's only
where its own words identify the planted rollout; and a binary fingerprint the run cannot record ends it
INCOMPLETE instead of a pass (§3, §6). The 2026-11-25 cross-family **lane** round (the r2 fix) closed
what remained open in those three families (the refusal-criterion and fingerprint fixes are the
pre-re-cut tree's; they are preserved at tag `archive/0009a-r16-native-arm` and carried by 0009e), each
as one systematic pass: every plant is created
*exclusively* — the create is the reservation — and only a file this run's create made is ever removed,
so two overlapping runs against one home can neither clobber nor delete each other's file; the refusal
must be the harness's measured sentence about the *exact* planted path, so an id-only lookup miss, an
I/O failure naming the path, and a refusal of a same-named file elsewhere each fail the arm; the
fingerprint resolves the executable before hashing or launching it and requires a real `--version`
answer (the `(no diagnostics)` placeholder is not one); and every read of the caller's home bytes and
every harness invocation answers by name — INCOMPLETE where the run cannot proceed without it, the
check's own FAIL where it can — instead of an exception out of the run (§3, §6). The 2026-11-25 **r3
fix round** (the same lane's third pass, under a root-cause consult's mandate: three invariants that
lived only in comments are now typed seams) replaced the plant/cleanup flag pair with one `ForeignHome`
ownership object — a path is recorded the instant its exclusive create succeeds and removed whatever
happens next, a *failed write after the reservation* no longer strangles the next run, a removal that
fails is INCOMPLETE by name, and the directories this run created to hold its fabrications are recorded
and removed too; gave every harness interaction one result vocabulary (`Thread` / `OtherError` /
`NoAnswer` — whose `Refusal` member and one-sentence matcher moved to 0009e with the arm — and a
pipe that dropped under the client folded at the client object so a caller's `finally` cannot replace
an answer already in hand); and re-shaped the oracle reading per §1's paragraph above. The tests moved
to the same seams: the drive's app-server stub now answers `thread/read` *replies* the shipped
extraction reads, the ownership object's record-before-write and named-removal contract has its own
unit cases, and `call`/`notify`/`close` are driven against a real broken pipe; six whole-suite
mutations (one per seam shape) each turn the suite red, where every one of them stayed green at the
head this round started from (§3). The ownership object's one declared residual is stated on it: a kill
between the exclusive create and process exit leaves a file no `finally` runs for, answered by the next
run's collision refusal rather than swept.

The 2026-11-26 **r4 round** (the same lane's fourth pass) re-read the three seams' sites — the plant
ownership object, the refusal's one sentence matcher and the fingerprint's launching rule — and
corrected the fingerprint correction itself (§3's operator blockquote: the launched path is made
absolute with `os.path.abspath`, **never** `resolve()`d, because this host's `codex` is a mise shim).
The **r5 round** closes the four findings the cross-family lane left on that head
(`review-lane-r4-fix-20261126.md`), each at the seam the finding names (the first two bullets below are
the moved features': their fixes are preserved with the moved work at tag `archive/0009a-r16-native-arm`
and carried by 0009e — the last two stay with the shipped half):

- **The disclosure arm's artifact lives where only this run can write.** It is planted into a probe home
  of its own under the run's scratch directory, never the target home, so no cleanup of this run can
  unlink a file another writer put under the planted name; and the arm judges the bytes *read back* from
  the planted name — requiring them equal to the bytes it planted — so a substitution of the planted file
  is named by the arm's own comparison instead of passing on the committed fixture.
- **The fingerprint pairs one executable's words with that executable's bytes.** Where the resolved name
  is a symlink, the run asks the resolved file to identify itself: a plain symlink to the harness answers
  its own version and its target's bytes are the digest, while a launcher's shim — answering its own
  words — ends the run INCOMPLETE rather than printing Codex's version beside the launcher's digest. The
  launched path itself is still never `resolve()`d.
- **The `--selftest-mutations` help states the three-arm contract the corpus measures** (a pair cites the
  rules its bytes break beside its own; with its own rule disabled it still cites those others, or is
  INCOMPLETE where a `needs_text` rule was left unjudged) — the corpus is unchanged.
- **The `oracle.jsonl-objects` citation is narrowed to what its Go case proves**: every line it decodes,
  the artifact's surrounding blank lines trimmed first, is exactly one JSON object; the rule's statement
  over every physical line belongs to the offline rule, whose blank line's refusal
  `test_the_line_rule_is_the_whole_lines_own` pins.

The suite moved with the seams on the pre-re-cut tree (94 → 96): the drive hands `main` a fixed scratch
directory of the case's own name (patched `mkdtemp`), so a case can address the run's own tree — the
probe home above all — before the run starts; `seed_probe` staged the held name at the plant's own site
while `probe_in_caller_home` pinned the caller's file under the probe's *name*; `probe_replaced` pinned
the read-back comparison; and the symlink case split into the dispatched-target arm and the
launcher-shim INCOMPLETE arm (§2, §3). **The re-cut removes every case whose only subject is a moved
feature** — sixteen of them (§2's "The re-cut's own test movement") — **and narrows the shared ones** to
the one plant site that survives (the unrelated session's in the caller's home), adding the three pins
for the shipped half's round-5 findings; the surviving suite is **82 tests, re-measured in §3**.

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
- **`oracle.message`** — `session.jsonl`, `session-media.jsonl`, `session-receipt.json` for one mutation — 9: `a message with no content`, `a block type the native format does not define`, `a block typed with the reader's spelling the format lacks`, `a message whose text is blank`, `a message with no role`, `a block whose text is not a string beside a valid one`, `a media block without its reference member`, `a media block whose reference member is not a string`, `a media block whose detail is outside the version's spellings`.
- **`oracle.event-message`** — `session.jsonl` — 2: `an event message that is not a string`, `an event message that is blank`.
- **`oracle.call-pairing`** — `session.jsonl` — 5: `an output naming no call`, `a call id carried twice`, `an output written before its call`, `a call without a usable name`, `a named call whose call_id is empty`.
- **`oracle.call-members`** — `session.jsonl`, `session-receipt.json` for one mutation — 6: `a call's recorded input renamed to the other family's member`, `a call output without its output member`, `a call's recorded input written as an object`, `a custom call's recorded input written as an object`, `a call output written as null`, `a call output written as an array of bare strings`.
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

**Added** (the premise's other two subjects): `oracle.utf8` (4 pairs: `a byte that is not UTF-8`,
`a byte-order mark`, and — added by the 2026-11-25 r2 round — `a byte that is not UTF-8 under the
unchanged receipt`, a *same-length* masked byte whose receipt disagreement (`also_cited`:
`oracle.receipt-integrity`) survives dropping `oracle.utf8`; plus, added by the r3 round,
`a byte that is not UTF-8 in an unterminated record`, whose `also_cited` is `oracle.whole-lines` — the
pair the r3 finding measured a reading answering PASS for), `oracle.whole-lines` (3 pairs: `a record
left unterminated`, the r3 round's `a record left unterminated beside a masked byte` (`also_cited`:
`oracle.utf8`) and `a record left unterminated under the unchanged receipt` (`also_cited`:
`oracle.receipt-integrity`)),
`oracle.receipt-integrity` (9 pairs: `an artifact byte changed under an unchanged receipt`,
`a receipt that states another whole-file digest`, `a receipt whose byte count is wrong`,
`a receipt naming no lines`, `a receipt naming one line fewer`,
`a receipt whose last anchor is another line's`, `a receipt that is not an object`, and the r3 round's
`a receipt under a masked byte` (`also_cited`: `oracle.utf8`) and `a receipt under an unterminated
record` (`also_cited`: `oracle.whole-lines`)). Corpus now: 4 rules, 22 pairs, and every pair of
byte-reading rules is co-cited in both directions.

**Fixtures.** Deleted with their subjects: `scripts/tests/testdata/codex-oracle/session.jsonl`,
`reduced.jsonl`, `session-media.jsonl`, `session-receipt.json`, `reduced-receipt.json` (three synthetic
sessions of 17/12/17 lines and two manifest receipts). Added in their place: `artifact.jsonl` (three
JSON objects, no agent's format) and the `artifact-receipt.json` that names its bytes.

**Test cases.** The attempt's oracle class held 37 cases over the deleted rules, its manifest receipts
and its reader model; the reduced file's oracle class held 21 at the head the second fix pass left — 18
through the simplify-apply round, one more from the review-fix round (the nesting bound's
at-or-under/past-it case), and the two the second fix pass added (the string-literal skip's own case and
the byte-counted tail) —
all of them over the survivors and the premise's two negative spaces (D2's container-valid-but-not-Codex
artifact, D2's vocabulary scan), the
D1 corpus read through the shipped command, and D4's two receipt cases. The rest of the suite
(`ProtectedArtifactsAfterNativeVerification` and the file's other classes) is the base's own; two of its
stubs gained the two host probes this increment's native half now makes (`codex --version`, and the
loader's refusal of a session it does not hold — the disclosure arm's own subject), and one existing
assertion ("1 pre-existing session file(s)") stands because the disclosure arm's planted file, like the
unrelated session, is the harness's own fabrication rather than the caller's (the 2026-11-26 r5 round
moved the arm's artifact out of the target home into the run's own probe home, so the capture never sees
it at all and the count is unchanged) — the `probe_in_caller_home` case drives the other reading, where
a file under the probe's *name* in the caller's home is compared as one of the home's own 2 sessions.
The 2026-11-24
simplify-apply round added four refusal-arm cases to the oracle class (its count moves 14 → 18) and five
to `ProtectedArtifactsAfterNativeVerification` (the disclosure arm's own record, the handshake that must
not read as a refusal, the resolved binary's version and digest, the planted probe's refusal and its
removal), so the narrowed suite's count moves 48 → 57. The review-fix round of the same day added two
more (the nesting bound's at-or-under/past-it pair and the error that names no planted artifact), moving
it 57 → 59. The second fix pass added three: two in the oracle class — the nesting pre-scan's
string-literal skip (`test_a_bracket_inside_a_string_is_not_nesting`) and the byte-counted tail
(`test_the_unterminated_tail_is_counted_in_the_artifact_s_own_bytes`) — and, in
`ProtectedArtifactsAfterNativeVerification`, `test_a_refused_collision_leaves_no_scratch_tree` (which the
third pass mirrored for the unrelated-name guard, alongside
`test_a_keep_run_that_refuses_keeps_and_names_its_scratch_tree`), so the narrowed suite's count moved
59 → 62 → 64 across the second and third passes. The cross-family review round of 2026-11-24
added five more: in the oracle class, the big-integer host-independence case
(`test_a_line_holding_a_big_integer_is_one_json_object_whatever_the_limit`) and the moved-delimiter case
(`test_a_line_feed_moved_is_named_by_the_line_whose_bytes_diverged`), its count moving 21 → 23, and, in
`ProtectedArtifactsAfterNativeVerification`, the dangling-symlink collision case, the id-only
lookup-miss case and the other-artifact case, its count moving 23 → 25 — beside the resolved-binary case
rewritten to pin the digest of the executable this run drove and two INCOMPLETE arms — so the oracle
class held 23 at that head and the narrowed suite's count was 69. The 2026-11-25 r2 round adds thirteen
(the narrowed suite moves 69 → 82): in `ProtectedArtifactsAfterNativeVerification`, the plant-race case
(both plant names), the two unreadable-home-entry cases (the directory shape, host-independent, and the
dangling-link integration arm), the unreadable-installed-session case, the command-that-cannot-be-started
case (four arms: build, conversion, read-back, turn), the empty-`--version` case, the relative-`--codex`
pairing case, and the two new refusal shapes (an open error naming the artifact; a same-named refusal
elsewhere); the two seam classes `ExclusivePlantClaim` (the exclusive-create contract and the held-name
collision) and `HarnessThatCannotBeStarted` (a binary that cannot be started records a named answer);
and, in the oracle class, `test_a_receipt_still_answers_a_byte_the_text_layer_cannot_read`. Two existing
cases gained arms with it: the corpus case now reads every pair's `also_cited` set, and the closed-set
case asserts each co-citation is declared and is not the pair's own rule. The 2026-11-25 **r3 round**
adds eleven (the narrowed suite moves 82 → 93): in `ProtectedArtifactsAfterNativeVerification`, the
failed-write case (the reservation survives the failure, the partial file is removed, and a retry's own
plant finds the name free), the refused-removal case (INCOMPLETE by name, exit 2, reported once, the
file reported rather than pretended away), the dropped-pipe disclosure arm (`NoAnswer` at the arm's
resume is a FAIL in the transport's own words, never the refusal the check's name claims), and the
text-only view-reply case — whose stub now answers `thread/read` *replies* and no longer stands in for
`thread_view`, so the shipped extraction runs inside every drive case (the removed stub was a finding of
its own); the seam unit classes `ForeignHomeOwnership` (the exclusive-create contract, the held-name
collision, the record-before-write rollback and the named removal; renamed from `ExclusivePlantClaim`)
and `BrokenPipeAtTheClientSeam` (a
real pipe whose read end is closed: `call`, `notify` and `close` fold it, and `close` raises nothing a
caller's `finally` would lose an answer to); `ThreadViewExtraction` (the extraction's own cases: the
kinds are the items' types, a kind word in message text is not a kind); and, in the oracle class,
`test_a_disabled_utf8_rule_still_cites_the_line_rule` (R3-2's own bytes, through the command) and
`test_every_pair_of_byte_rules_is_co_cited_by_a_mutation`, beside the byte-layer case rewritten for the
not-evaluated contract and the closed-set case's input-kind assertions. The 2026-11-26 r4 round added
one (93 → 94: `test_a_symlinked_shim_is_launched_through_its_own_path`, the shim correction's own pin),
and the r5 round moves it 94 → 96 while one case's subject moves: `test_a_callers_file_under_the_probes_name_is_refused_without_leaving_a_fabrication`
becomes `test_a_callers_file_under_the_probes_name_is_left_as_it_was_found` (the artifact now lives in
the run's own probe home, so a caller's file under that name is a session the home already held), and
two cases join it — `test_a_probe_replaced_while_the_run_goes_is_named_not_passed_on_the_fixture` (the
read-back comparison) and `test_a_launcher_shim_that_cannot_be_dispatched_ends_the_run_incomplete` —
while `test_a_symlinked_shim_is_launched_through_its_own_path` becomes
`test_a_symlinked_name_is_launched_through_its_own_path_and_fingerprinted_by_its_target` and the seam
cases' staged paths move into the run's own tree (§1).

**The re-cut's own test movement (2026-11-26).** Sixteen cases were removed with their subjects —
`test_the_disclosure_arm_is_a_recorded_pass_naming_the_harnesss_refusal`,
`test_a_handshake_that_never_answered_is_not_read_as_a_refusal`,
`test_an_error_that_names_no_planted_artifact_is_not_read_as_a_refusal`,
`test_an_id_only_lookup_miss_is_not_read_as_the_refusal`,
`test_an_error_naming_another_artifact_is_not_read_as_the_refusal`,
`test_a_run_records_the_resolved_binary_and_its_digest`,
`test_a_callers_file_under_the_probes_name_is_left_as_it_was_found`,
`test_a_probe_replaced_while_the_run_goes_is_named_not_passed_on_the_fixture`,
`test_the_planted_probe_is_removed_when_the_run_ends`,
`test_a_dropped_pipe_on_the_probe_resume_is_not_read_as_a_refusal`,
`test_a_version_command_with_no_answer_ends_the_run_incomplete`,
`test_a_symlinked_name_is_launched_through_its_own_path_and_fingerprinted_by_its_target`,
`test_a_launcher_shim_that_cannot_be_dispatched_ends_the_run_incomplete`,
`test_the_fingerprint_pairs_one_executable_with_its_own_version`,
`test_an_open_error_naming_the_artifact_is_not_read_as_the_refusal` and
`test_a_refusal_of_a_same_name_elsewhere_is_not_read_as_the_refusal` — the two refusal-path collision
cases collapsed into one for the surviving plant site (`test_a_refused_collision_leaves_no_scratch_tree`,
which also carries the caller's-colliding-file clause), and six shared cases were narrowed to the
unrelated session's plant site: `test_a_callers_dangling_symlink_under_a_plants_name_is_a_collision`,
`test_a_keep_run_that_refuses_keeps_and_names_its_scratch_tree`,
`test_a_write_that_fails_after_the_reservation_leaves_no_leftover`,
`test_a_fabrication_the_run_cannot_remove_is_incomplete_by_name`,
`test_a_file_racing_the_plant_is_neither_clobbered_nor_deleted` and the unreadable-home-entry case
(the drive's `race_plant`/`plant_write_fails`/`unlink_fails`/`dangling_symlink` knobs are the booleans
they now are, since one plant site remains). Three cases were added for the shipped half's round-5
findings: `test_the_corpus_notices_unjudged_reporting_that_disappeared` (F-1),
`test_a_receipt_member_the_rule_does_not_read_is_not_converted` (F-2) and
`test_a_turn_that_never_answered_is_not_counted_as_run` (F-3); the closed-set case gained the
`unjudged`-declaration assertions. The suite is **82 tests on the re-cut tree** (§3, re-measured).

**Documentation.** The attempt's `docs/harness.md` paragraph describing sixteen rules and the manifest
receipt is replaced by the reduced oracle's description and the disclosure's limit (§4; the arm and
fingerprint paragraphs the first draft added were removed again by the re-cut, and live with 0009e); its
AGENTS.md edits of
that era are not part of this base (the ownership rule is already there). The plan stub the decomposition
left for this goal (`docs/plans/0009a-native-oracle.md`, whose increment is the superseded attempt) is
reduced to a pointer at this increment's plan, so no plan file directs the superseded premise; its older
text is in git history and the attempt's own trees at the tag.

## 3. The runs

**The oracle's closure** (`--selftest-mutations`, exit 0; each arm driven through the shipped command's
own `--validate`/`--receipt`/`--disable-rule` path):

```
oracle.selftest: PASS — 4 rule(s), 22 fixture pair(s): positive PASS, negative FAIL (each citing its own
rule; a pair whose bytes break another rule names it too), --disable-rule clean of the pair's own rule
(still citing those others where the bytes break them, INCOMPLETE where the disabled rule left a
needs_text rule unjudged)
```

**The verdict paths**, with real exit codes:

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate <scratch>/mutated.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json       # one byte of line 3 changed
oracle: oracle.receipt-integrity — the receipt states sha256 855ccca11024…, the artifact's bytes are sha256 8cf233599e61…
oracle: oracle.receipt-integrity line 3 — the line's bytes are sha256 595a00578a9b…, the receipt's anchor is sha256 ac584578fe8b…
oracle: FAIL — 3 record(s), 2 violation(s)                                    # exit 1
$ python3 scripts/codex-native-check.py --validate <scratch>/any.jsonl             # {"anything":"goes"} ×3
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate <scratch>/bad.jsonl             # a 0xFF first byte
oracle: oracle.utf8 — the artifact is not UTF-8 text: byte 0xff at offset 0
oracle: oracle.jsonl-objects — not evaluated: oracle.utf8 answered no text to judge
oracle: FAIL — 0 record(s), 1 violation(s), 1 rule(s) not evaluated           # exit 1
$ python3 scripts/codex-native-check.py --validate <scratch>/no-such-file.jsonl
oracle: INCOMPLETE — the artifact <scratch>/no-such-file.jsonl cannot be read: …   # exit 2
$ python3 scripts/codex-native-check.py --validate <scratch>/big.jsonl             # {"n": <5,000 nines>}
oracle: PASS — 3 record(s)                                                    # exit 0
$ python3 scripts/codex-native-check.py --validate <scratch>/moved-lf.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json       # the first LF became a space
oracle: oracle.receipt-integrity — the receipt states sha256 855ccca11024…, the artifact's bytes are sha256 7f78a8778a84…
oracle: oracle.jsonl-objects line 1 — not JSON (Extra data at column 63)
oracle: oracle.receipt-integrity line 1 — the line's bytes are sha256 f4c89b83a897…, the receipt's anchor is sha256 402041d785c7…
oracle: oracle.receipt-integrity line 2 — the line's bytes are sha256 ac584578fe8b…, the receipt's anchor is sha256 9371e97c2450…
oracle: oracle.receipt-integrity line 3 — the receipt names 3 line(s), the artifact holds 2; anchor 3 is the first the artifact gives no line for
oracle: FAIL — 1 record(s), 5 violation(s)                                    # exit 1
```

The last two commands are the 2026-11-24 round's reading fixes, re-measured: a 5,000-digit integer is one
JSON object on an interpreter whose conversion limit is 4,300 digits, and a moved delimiter byte is
refused with the line whose bytes diverged rather than only with the file's digest and the count. The
2026-11-25 r2 round's own reading fix is one more block of paths over the same bytes — a *same-length*
0xFF byte masked into the frozen artifact's first line, under the receipt the frozen bytes earned (the
outputs below are the r3 round's own, re-measured on this record's tree):

```
$ python3 scripts/codex-native-check.py --validate <scratch>/masked.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json        # one same-length byte masked
oracle: oracle.receipt-integrity — the receipt states sha256 855ccca11024…, the artifact's bytes are sha256 bd31b20dd855…
oracle: oracle.utf8 — the artifact is not UTF-8 text: byte 0xff at offset 17
oracle: oracle.receipt-integrity line 1 — the line's bytes are sha256 57b328a054d7…, the receipt's anchor is sha256 402041d785c7…
oracle: oracle.jsonl-objects — not evaluated: oracle.utf8 answered no text to judge
oracle: FAIL — 0 record(s), 3 violation(s), 1 rule(s) not evaluated           # exit 1
$ python3 scripts/codex-native-check.py --validate <scratch>/masked.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json --disable-rule oracle.utf8
oracle: oracle.receipt-integrity — the receipt states sha256 855ccca11024…, the artifact's bytes are sha256 bd31b20dd855…
oracle: oracle.receipt-integrity line 1 — the line's bytes are sha256 57b328a054d7…, the receipt's anchor is sha256 402041d785c7…
oracle: oracle.jsonl-objects — not evaluated: oracle.utf8 answered no text to judge
oracle: FAIL — 0 record(s), 2 violation(s), 1 rule(s) not evaluated           # exit 1
$ python3 scripts/codex-native-check.py --validate <scratch>/masked.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json \
    --disable-rule oracle.utf8 --disable-rule oracle.receipt-integrity
oracle: oracle.jsonl-objects — not evaluated: oracle.utf8 answered no text to judge
oracle: INCOMPLETE — 0 record(s), 1 rule(s) not evaluated                     # exit 2
```

The middle command is the r2 fix: before it, the byte-layer violation skipped the receipt comparison
entirely, the filter then dropped the only surviving violation, and these bytes **passed** with only
`oracle.utf8` disabled (measured: `exit 0`); now the receipt's own two statements stand. The r3 round
changes the third arm: with both byte rules dropped the reading no longer answers PASS — the
text-dependent rule was never judged, and a verdict is PASS only where every rule was evaluated — so it
names the unjudged rule and exits INCOMPLETE (2), which is the seam the r3 finding was about (the
pre-r3 reading answered PASS over bytes whose final byte was not a line feed; §1's paragraph and
`--disable-rule oracle.utf8`'s middle arm above hold the line rule's violation standing).

**The suite** (`selftest`'s own command, narrowed to this file; the whole discovery runs in §6). At the
r5 (pre-re-cut) head it ran 96 tests; **re-measured on the r8 round's tree**:

```
$ python3 -m unittest discover -s scripts/tests -p 'test_codex_native_check.py'
Ran 93 tests in 1.620s
OK
```

> **Operator correction (2026-11-26 ~01:10 CST, after this stage's report).** The fingerprint fix above
> `resolve()`d the `--codex` path. On this host `codex` is a **mise shim** — a symlink to the `mise`
> binary that dispatches on `argv[0]` — so resolving it launched `mise` itself, whose `--version` is
> mise's and whose `app-server` is not Codex's: the native run exited **1** and the disclosure arm was
> answered by nothing. The path is now made absolute with `os.path.abspath` (**never** `resolve()`d),
> pinned by `test_a_symlinked_shim_is_launched_through_its_own_path` (measured: the case fails with
> `resolve()` restored and passes without it). Re-measured on the corrected tree: `GATE: PASS`,
> `selftest` 549, the narrowed suite 94, native `--json` exit 0 (27 passed / 0 failed / 2 skipped,
> `codex-cli 0.172.0`).
>
> The **r5 round** renamed that case and split its subject:
> `test_a_symlinked_name_is_launched_through_its_own_path_and_fingerprinted_by_its_target` (the
> `abspath`, never `resolve()`, rule is unchanged) and
> `test_a_launcher_shim_that_cannot_be_dispatched_ends_the_run_incomplete` (the fingerprint's dispatch
> rule, §1). The suite run above is the r5 round's own.

The 48 → 57 movement is the simplify-apply round's nine added cases, 57 → 59 the review-fix round's two,
59 → 62 the second fix pass's three and 62 → 64 the third pass's two (the unrelated-name guard's own
refusal path and the `--keep` refusal), 64 → 69 the cross-family review round's five, 69 → 82 the
2026-11-25 r2 round's thirteen, 82 → 93 the 2026-11-25 r3 round's eleven (§2 names them), and 93 → 94
the 2026-11-26 r4 round's one; the r5 round moves it 94 → 96 (§1 names the two and the case whose
subject moved with the artifact), and the run above is the r5 round's own, on the tree this record
describes. **The re-cut moves it 96 → 82** — sixteen cases removed with the moved features, the two
refusal-path collision cases collapsed into one, six cases narrowed to the surviving plant site, and
three pins added for the shipped half's round-5 findings (§2's "The re-cut's own test movement") — and
the whole discovery in §6 is correspondingly **537** tests against 551 at the r5 head. The **r7 fix
round** moves it 82 → 90 — its eight cases: R6-1's launch-path pins (three unit cases and one
drive case), R6-2's past-bound receipt, R6-3's verdict-lying corpus mutant, R6-4's non-zero turn and
R6-5's offline-mode refusal — and the whole discovery in §6 is correspondingly **545** tests. The
**r8 fix round** moves it 90 → 93 — R7-1's rooted-path predicate case, R7-2's read-failure arm and
R7-3's appended-LF receipt case — and the whole discovery in §6 is correspondingly **548** tests.

**The r7 fix round's arms** (2026-11-26, on this record's tree; each finding's case was measured red
before its fix):

```
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl \
    --receipt <scratch>/deep.receipt.json      # the frozen receipt + a 2,000-level *ignored* member
oracle: oracle.receipt-integrity — the receipt document is nested deeper than 256 levels, the bound this rule states
oracle: FAIL — 3 record(s), 1 violation(s)                                     # exit 1
# pre-fix the same document answered PASS (exit 0) on Python 3.15 and INCOMPLETE (exit 2) on 3.11:
# the member the rule does not read decided the verdict with the reading interpreter (R6-2).
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl \
    --resume-prompt "a question"               # an offline mode handed a native-turn flag
codex-native-check.py: error: --validate reads one artifact and runs no harness turn: it takes no --resume-prompt or --repeat-resume-prompt
                                                                               # exit 2 (R6-5; pre-fix: PASS, no turn, no diagnostic)
```

`--selftest-mutations` refuses the same combination by name (exit 2). The suite's own halves: the corpus
rewritten to say `verdict: PASS` on every emitted document closes **red** (R6-3; pre-fix it closed
green), a turn whose process answered non-zero without growing its session prints
`real turn(s) run: 0` (R6-4; pre-fix: 1), and the run hands the stub the `--codex` path it was given —
`<root>/alias/../codex`, never its lexical collapse (R6-1; pre-fix the stub saw `<root>/codex`, a name
the traversal does not reach). The lock-boundary finding (R6-6) is a documentation change: the two goal
files now state the same boundary (the offline oracle, not the whole file), in the form `Q-0009a-13`
records.

**The r8 fix round's arms** (2026-11-26, on this record's tree; each finding's case was measured red
before its fix, and every probe edit was restored):

```
$ python3 -m unittest ... -k test_a_read_failure_does_not_speak_for_the_resume          # R7-2
# pre-fix the loaded session's record was FAIL in the read's own words and the resume verdict was gone;
# now the loader's check is `thread/resume`'s and the view check names the read:
PASS: the target harness's own loader opens the installed session — thread/resume -> <id> [legacy] cwd=/tmp
FAIL: the target's own view lists the converted turns — the app server closed its output while answering thread/read
$ python3 scripts/codex-native-check.py --validate scripts/tests/testdata/codex-oracle/artifact.jsonl \
    --resume-prompt ""                         # an *empty* native-turn flag on an offline mode (R7-4)
codex-native-check.py: error: --validate reads one artifact and runs no harness turn: it takes no --resume-prompt or --repeat-resume-prompt
                                                                    # exit 2; pre-fix: PASS, no turn, no diagnostic
```

The appended-LF artifact's receipt violations read `{0, 4}` — line 4 named as the first the receipt
gives no anchor for — where pre-fix they read `{0}` with the count statement naming no line (R7-3); the
absolute-path case's fixtures are built from the process directory's own anchor and the drive-less
shape has its own case, because `ntpath.isabs('\opt\bin\codex')` answers False on CPython 3.13 (True on
3.11 and 3.12) while the helper's contract anchors it to the process directory — the current drive —
which is the platform's own resolution (R7-1; the Windows CI leg is that arm's oracle).

**The r11 fix round's arms** (2026-11-27, on this record's tree, the five r8 findings; each pin was
measured red before its fix and green after, every probe edit restored, and no case deleted, narrowed
or skip-widened):

```
$ python3 scripts/codex-native-check.py --validate …/artifact.jsonl --receipt <scratch>/deep.receipt.json \
    --disable-rule oracle.receipt-integrity              # 257 openers and no closers
oracle: INCOMPLETE — the receipt <scratch>/deep.receipt.json cannot be read: nested deeper than 256 levels, the bound this reading states
                                                                               # exit 2
# pre-fix the same bytes answered `oracle: PASS — 3 record(s)` (exit 0): the depth refusal was
# oracle.receipt-integrity's own violation, and --disable-rule masked it (r8 finding #1; Q-0009a-15).
$ python3.11 -m unittest … -k test_a_rooted_path_is_answered_by_the_dialects_own_join
# pre-fix on 3.11/3.12 the drive-less rooted spelling came back unchanged ('\opt\bin\codex', no drive):
# ntpath.isabs calls it absolute there, so one run's two consumers could resolve it against different
# drives; now the dialect's own join supplies the drive on every minor (r8 finding #2; Q-0009a-16).
$ python3 -m unittest … -k test_an_installed_session_that_is_not_utf8_is_a_named_failure
# pre-fix the same run raised `UnicodeDecodeError` out of main on a UTF-8 locale; now the JSONL check
# and the appended-turn search each record `cannot be read as UTF-8` and the same file's digest
# comparison still passes (r8 finding #3; Q-0009a-17).
$ python3 -m unittest … -k a_refused_exec_lookup   # and -k staged_executable_bit
# pre-fix the probe answered one bool (False under a noexec mount, because it asked os.access) and
# _assert_sane raised at import, before the required selftest step; now storage is measured with stat,
# the lookup answer is separate and non-fatal, and parity staging sets the bit storage answers for
# (r8 finding #4; Q-0009a-18).
```

**The r12 fix round's arms** (2026-11-28, on this record's tree; Part A's `windows-latest` red — CI run
`45949278359` — and the five r9 findings; each pin was measured red before its fix and green after,
every probe edit restored, and no case deleted, narrowed or skip-widened):

```
$ python3 -m unittest … -k test_the_expected_skip_inventory_follows_the_host_s_measured_answer
# pre-fix the nested run's expected-skip inventory was the literal two-name tuple, so on a host whose
# measured `mode_bits_stored` is false the child's storage-premise skip failed the inventory assertion
# while the child run itself was green (the CI red). Now the inventory is `GUARDED` plus that one
# case's name iff the host's own measured answer says storage is absent, and the pin drives the
# assertion with fabricated statuses under both answers — accepted without storage, refused with it —
# and refuses a skip of the staged-bit case's name under either answer: its disposition moved
# skip -> assert, its no-storage arm pinned through the patched control (`st_mode & 0o100 == 0`).
$ python3 -m unittest … -k test_the_seeded_configuration_is_written_as_utf8_whatever_the_locale_answers
# pre-fix the scratch TOML was written with no encoding: under a process whose locale cannot encode the
# host configuration's provider name the write raised `UnicodeEncodeError: 'ascii' codec …` out of main
# before the cleanup block — no launch, no verdict (r9 finding #1).
$ python3 -m unittest … -k test_a_turn_that_overran_its_deadline_names_the_duration_not_the_command
# pre-fix the timeout handler formatted `TimeoutExpired` itself, whose `str` carries the whole argument
# vector — the prompt among its words — into the recorded detail and so into the printed report and,
# under --json, the machine-readable one (r9 finding #3).
$ python3 -m unittest … -k test_a_drive_relative_word_is_refused_and_never_returned \
    -k test_a_drive_relative_executable_is_refused_by_name_before_anything_runs
# pre-fix `absolute_launch_path` returned a join-unchanged drive-relative word (`D:tools\codex.exe`)
# unresolved: `ntpath.join` answers a different-drive word unchanged (it discards the accumulated
# path) while the word still resolves against that drive's own current directory — process state this
# run cannot spell for the processes it starts, the spelling Q-0009a-16's own "Settled by" line names
# (r9 finding #2, the class of r11's #2). Now the rule refuses the word (`ValueError`), the call site
# answers exit 2 by name with nothing launched and no scratch tree made, and `windows-latest` is the
# default-path arm's oracle.
$ python3 scripts/codex-native-check.py --validate <scratch>/moved-lf.jsonl \
    --receipt scripts/tests/testdata/codex-oracle/artifact-receipt.json
# the refreshed transcript above: the shipped reading puts the unmatched anchor on line 3 and sorts it
# after the lines 1-2 violations (pre-fix this record printed the count violation unnumbered and
# first), and the r7 block's fence now closes after its last transcript (r9 finding #4).
```

`docs/conventions.md`'s binding lint instruction now names the field the lint reads
(`HOST_SEMANTICS.exec_lookup_follows_mode_bits`) and distinguishes it from `mode_bits_stored`, the
parity staging answer (r9 finding #5; the stale name raised `AttributeError` and would have failed the
lint). The two taken readings are `Q-0009a-19` (the inventory) and `Q-0009a-20` (the refusal).

The plan statements the r8 findings #1/#2/#5 falsified are updated where they are stated
(`docs/plans/0009a-native-oracle-v2.md`, `docs/plans/0009-decomposition.md`, `docs/harness.md`'s
native-check section), and the superseded readings are `Q-0009a-15`–`-18`.

**The native run** (the Codex judgment is the binary's; `python3 scripts/codex-native-check.py --json`,
**re-measured on the r11 round's tree, the r8 round's answer unchanged**, exit 0). The header names the launched binary alone — the version and
digest pairing moved out with the fingerprint — and the run's own count is the base's minus the moved
check line:

```
codex:       <codex>
…
26 check(s) passed, 0 failed, 2 skipped
```

(`26` passed + `2` skipped = 28 recorded checks, against 29 at the pre-re-cut head — the moved arm's own
line; the two skips are the real turns, which need model access and no prompt flag was passed.)

The pre-re-cut tree's own native runs — the disclosure arm's PASS with its r3/r5 re-measurements, the
fingerprint's header pair, the r2 round's strictness arms
(`test_an_id_only_lookup_miss_is_not_read_as_the_refusal`,
`test_an_open_error_naming_the_artifact_is_not_read_as_the_refusal`,
`test_a_refusal_of_a_same_name_elsewhere_is_not_read_as_the_refusal`) and the fingerprint's preconditions
(`test_a_run_records_the_resolved_binary_and_its_digest`,
`test_a_version_command_with_no_answer_ends_the_run_incomplete`,
`test_the_fingerprint_pairs_one_executable_with_its_own_version`) — all moved with their features to
[goal 0009e](../goals/0009e-native-check-hardening.md); their fixes and cases are preserved at tag
`archive/0009a-r16-native-arm`, and the quotes that measured them are in this record's earlier revisions
(this branch's pre-re-cut heads, in git history). What the re-measured run above records is the loader's
own answers and the base's own comparisons — plus the absent-binary arm below — and nothing in Python
restates what the binary answers.

**The absent-binary arm** (D3's other half — the reason, and a non-zero exit, never a guess):

```
$ python3 scripts/codex-native-check.py --codex definitely-not-a-binary
definitely-not-a-binary is not on PATH: this harness verifies native behaviour, which needs the target harness installed
# exit 2
```

## 4. The disclosure, side by side

The two answers for one artifact's bytes were measured on the pre-re-cut tree (those runs are §3's, now
moved with the arm to [goal 0009e](../goals/0009e-native-check-hardening.md)): the offline rules hold
the frozen artifact green, and the binary that owns the format refuses it. That is the re-scope's stated
limit, not a defect: the offline
half can no longer detect a Codex artifact the binary would reject, by construction — it holds no Codex
knowledge at all. It is stated where a reader meets it: the goal file's own "What this goal does not
claim" paragraph (under *Intended advance*) and its *Non-scope*, this
record, and `docs/harness.md`'s native-check
section (which is also the CI note: the gate never runs the native half — `scripts/gate.py` decides the
offline half, and the native checks are deliberately not gate steps). The demonstration that keeps that
limit visible — planting a fabricated artifact and quoting the binary's refusal — is 0009e's acceptance.

## 5. The debt re-read (D5)

`docs/core-design/README.md` §2's declared-debt entries were re-read against this increment's surface
(it deletes a Python oracle, adds container/receipt rules; the two native-half additions were re-cut to
0009e, and no Go file or other native check is touched). Each is **kept** with its condition unchanged:

| entry (condition) | site | why kept |
| --- | --- | --- |
| `Q-0005-18` | `ccc/internal/ccc/pi_source_leaf.go` | no Go change here; the leaf check's behaviours are untouched |
| `Q-0008-10` | `scripts/opencode-native-check.py` | that file is not touched by this increment |
| `Q-0007-13` | the same file's Claude arm | not touched |
| `Q-0005-20` | `scripts/pi-native-check.py` | not touched |
| `Q-0005-19` | `ccc/internal/ccc/pi_verify.go`, `omp_verify.go`, the Pi/OMP checks | not touched |
| `Q-0003-12` | `ccc/internal/ccc/codex_install.go` | not touched |

No new entry is owed: the surviving rules judge no foreign format, and the native half reads the
binary's own answers, which is what the ownership rule asks for. The defaults this increment
took where the brief was silent are recorded in `docs/decisions/OPEN-QUESTIONS.md` as `Q-0009a-01`–`05`,
with the 2026-11-25 r2 round's two refinements appended as `Q-0009a-06` (the staged reading's receipt
clause, superseding `Q-0009a-02`'s) and `Q-0009a-07` (the refusal criterion, superseding `Q-0009a-05`),
and the r3 round's reading supersession as `Q-0009a-08` (every rule evaluated or explicitly
`NotEvaluated`; `--disable-rule` a subtraction after evaluation, superseding `Q-0009a-02`'s evaluation
clause), and the r5 round appends `Q-0009a-09` (the launcher-shim dispatch: the run ends INCOMPLETE
rather than printing a pair it cannot support, which is the finding's own allowed arm). **The
2026-11-26 re-cut appends `Q-0009a-10` (the re-cut itself) and marks `Q-0009a-05`, `-07` and `-09`
moved to [goal 0009e](../goals/0009e-native-check-hardening.md) with the arm and the fingerprint;
`Q-0009a-01`–`-04`, `-06` and `-08` stay with the frozen oracle.**

## 6. The gate, on the state this increment leaves

`python3 scripts/gate.py`, exit 0 — every step ran, none skipped, bounded or narrowed:

```
gate: <worktree>
gate: go=<go 1.27.4> module=ccc targets=./cmd/ccc/... ./internal/ccc/...
  PASS     gofmt       3624 file(s) gofmt-clean
  PASS     build       go build ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     vet         go vet ./cmd/ccc/... ./internal/ccc/... (in ccc) -> rc=0
  PASS     test        3 package(s) ok (in ccc)
  PASS     portability pin 83195c8a6861b8c74a4b6cb87512817fed6f8584 (v0.47.0-181-g83195c8a) | module ccc built and tested against the pristine checkout at <upstream-cache>/83195c8a6861b8c74a4b6cb87512817fed6f8584 | ran with -modfile=portability.mod (replace -> <upstream-cache>/83195c8a6861b8c74a4b6cb87512817fed6f8584) and portability.sum beside it | go build ./cmd/ccc/... ./internal/ccc/... -> rc=0 | 3 package(s) ok
  PASS     selftest    Ran 564 tests in 41.868s (rc=0)
  PASS     parity      … | : 83195c8a6861b8c74a4b6cb87512817fed6f8584 | layout: carry root agentsview/, this repository's module ccc | carried: 5579 file(s) identical to the pinned revision (compared through git's own object store) | deviations: 1 upstream path(s) declared (1 of them carried with the bytes this tree stores) + 1 generated input(s), all present as declared | owned: 0 project-owned prefix(es), plus the carry directory's own boundary; files seen by git: 5778 | inputs: agentsview/internal/pricing/snapshot/litellm_snapshot.json.gz sha256=75097ea1199a3f53590ccbf6ec61aa05a0c471ffc2f44018925d4a895ab5a3e8 (pinned by agentsview/internal/pricing/cmd/litellm-snapshot/main.go) | symlinks hashed by their link target: 4 | declared stored bytes verified: agentsview/frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json
  PASS     citations   docs/evidence/0002-citations.tsv: 36 row(s) verified | docs/evidence/0007-citations.tsv: 37 row(s) verified | docs/evidence/0008-citations.tsv: 13 row(s) verified | docs/evidence/0009a-citations.tsv: 1 row(s) verified
  PASS     secrets     no leaks found in 198 file(s) this repository owns (the carried upstream tree is covered by the parity step, not by this scan)
GATE: PASS
```

The two steps this increment is about are `selftest` — whose discovery runs `CodexArtifactOracle` (the
corpus's 22 pairs through the shipped command, the negative space, the receipt cases) — and `citations`,
which verifies the goal's row against the Go test it cites. The table is **re-measured on the r12 round's
tree** (2026-11-28), whose `selftest` count is 564 against 551 at the r5 head (§2/§3 name the moved
cases; the whole discovery's counts moved 503 → 519 across the four rounds, → 524 in the cross-family
review round, → 537 in the r2 round, → 548 in the r3 round, → 549 in the r4 round, → 551 in the r5
round, whose 96-test narrowed suite the re-cut moves to 82, the r7 fix round to 90, the r8 fix
round to 93, the r11 round to 94 and the r12 round to 98; the whole discovery moves 537 → 545 → 548,
→ 555 in the r9+r10 rounds (the Windows-leg cases and the host contract's own), → 559 in the r11
round (the undecodable-install case, the two probe-answer cases and the parity-staging case) and
→ 564 in the r12 round, whose five are the seeding-encoding case, the timeout case, the two
drive-relative refusal cases and the skip-inventory case). Its `secrets`
count (198 files) is
this worktree's own scope — the
count includes untracked `__pycache__`/`bin` files — not a change the re-cut made.

## What this record does not claim

- That the offline rules can detect a Codex artifact the binary would reject. They cannot, by
  construction, and §4 states it where a reader meets it; the demonstration that keeps it visible (the
  binary's refusal of the frozen artifact) is [goal 0009e](../goals/0009e-native-check-hardening.md)'s.
- That a gate run exercises the native half. It never has and does not now: the native checks need the
  Codex binary installed (and model access for a real turn), and they stay re-runnable evidence rather
  than gate steps. The two turns in the native half were therefore skipped in §3's run, and the run says
  so (`2 skipped`) instead of reading as a full one.
- That the offline rules are complete about JSONL in general. They state the container this route writes
  — whole UTF-8 lines, one JSON object per line, no duplicate members, no unpaired surrogate escapes —
  and a blank line is refused rather than skipped; a choice the later increments can revisit, which is
  what the oracle lock freezes until goal 0009 ends.
- That the native run's recorded binary is any kind of pin. The owner's answer is to follow `latest`;
  the pre-re-cut record's `0.172.0` and digest were what this host resolved on 2026-11-25 (the
  2026-11-23 run of this increment built on 0.171.0), the re-cut's header names the launched path alone
  (the fingerprint is 0009e's), and the next run records its own.
