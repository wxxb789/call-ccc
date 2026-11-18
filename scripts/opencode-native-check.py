#!/usr/bin/env python3
"""Verify the OpenCode route natively, against the installed OpenCode binary.

This is the target-side native verification for goal 0007 (OpenCode continuation), and it is deliberately
**not** a gate step, exactly like ``scripts/claude-native-check.py``, ``scripts/codex-native-check.py``,
``scripts/pi-native-check.py`` and ``scripts/omp-native-check.py``: it needs the OpenCode binary installed,
and a real turn (``--resume-prompt``) needs model access. The gate keeps deciding the offline half (the
inherited reader's round trip, the install rules, the reported losses and the CLI shape).

What it does:

1. copies the source Claude fixture into the run's own scratch tree and rewrites **only** the working
   directory its own records carry to a directory inside that tree, so the session it converts belongs to a
   path this run owns -- measured: the harness's own importer refuses a session whose ``location.directory``
   does not exist (``LocationNotFoundError``), so the fixture's recorded path could not be imported at all;
2. makes the OpenCode data directory the conversion installs into: the run's own scratch one (nothing is
   seeded into it, because this harness needs no provider definition to import or read -- only a turn needs
   the caller's own configuration) or the caller's ``--target-home``;
3. plants an unrelated session in it **through the harness's own importer** and reads that session's own
   document back, so "nothing else moved" is a measurement rather than a claim (a store can only be written
   by the harness, which is this route's install rule as much as its check). A store that already holds the
   id this run plants is read first: a session whose own document is this check's fixture is a leftover from
   an earlier run (whose planting import committed and whose process then died before its cleanup) and is
   removed through the harness's own delete, while a session under that id holding anything else is a
   caller's own and is left exactly as it was found, with the run stopping there. The planting itself is
   reconciled the way the route reconciles its own import -- the harness's acknowledgment, then the
   harness's own reader -- so an import process that failed or overran its deadline after committing is a
   publication this run removes again rather than a fixture left in a caller-named store;
4. converts through the CLI (``ccc continue … --verify load --json``) and reads the conversion, the
   installation and the structural read-back out of the report rather than trusting them. The conversion
   level is ``load`` even when a turn was requested: the one real turn is this check's own (step 7), and
   asking the CLI's resume verification as well would run and bill two turns for one ``--resume-prompt``;
5. asks the harness's **own reader** about the installed session (``opencode session export <id>``) and
   compares its answer with the report, the source's own evidence and the **source fixture's own bytes** --
   the source's first prompt, the compaction boundary, and the tool call with the arguments and result text
   that fixture records (read by ``source_tool_evidence``, not by the conversion's own items, so a writer
   that mangled either member cannot pass by agreeing with itself) -- with an unknown-id control arm: the
   same command must refuse, naming the id it could not find;
6. asks the harness's own project-scoped listing (``opencode session list --format json``) **from the
   session's own recorded directory**, where it must name the installed session, and from a directory that
   is not that one, where it must not: a session the harness does not resolve from where the report says to
   resume it is a session a caller cannot resume;
7. with ``--resume-prompt``, runs one real turn in the installed session through the report's **own**
   invocation: the exec-form argument vector it printed, validated against this run's own harness and this
   conversion's session (the ``--standalone`` flag on the subcommand's own side of the ``--`` terminator),
   with the prompt substituted for its placeholder, and run from the session's own recorded directory under
   the report's own ``resume_environment`` (that environment names the data root and pins the store's own
   database file beside it, so an inherited ``OPENCODE_DB`` cannot send the command to another store). The
   harness's own answer is printed under ``native_check_common.TURN_OUTPUT_MARKER``, and the growth is
   measured through the harness's own reader;
8. re-digests the source tree and re-reads the planted session's document, so "nothing else moved" is
   measured after the run as well as before it, and removes the session this run published through the
   harness's own ``session delete``.

Every command this check runs goes through ``deadline_run``: a command that overruns its own bound is
recorded as a failed check of its own and answered as a failed exit status, rather than raising out of the
run, with the output it did print kept -- a slow import that printed its own acknowledgment of the session
it committed before it hung is the shape the planting reconciliation reads. That is what keeps the cleanup
(step 8) and the final ``RESULT`` line reachable on every path -- an uncaught ``TimeoutExpired`` would leave
the publication in a caller-named home, and its fixed id would make every later run's planting meet it.

Measured on this host against OpenCode 2.2.5 (``opencode --version`` prints ``opencode v2.2.5``): the data
directory is ``$XDG_DATA_HOME/opencode`` and the store inside it is ``opencode.db`` (there is no variable
that names the directory itself; ``OPENCODE_DB`` overrides only the database file); ``session import <file>
--directory <dir> --standalone`` answers ``Imported session: <id>`` on stdout with exit 0 and preserves the
document's own id, refuses a directory that does not exist, and answers ``Session already exists`` on stderr
with exit 0 for an id the store already holds; ``session export <id> --standalone`` answers the session's
document and exits 1 with ``Session not found: <id>`` for an unknown id, and changes none of the session's
own records (measured: identical rows and identical exported documents around it, while the database
*file's* bytes move with its WAL handling); ``session list --format json`` lists the sessions of the
*current directory's* project; ``session delete <id> --standalone`` answers ``Session <id> deleted`` and
leaves the store without that session; and ``run --session <id>`` appends its own records while *creating* a
session for an id the store does not hold -- which is why no level of this check drives ``run`` as a load,
and why the control arm asks about an id nothing installed.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from native_check_common import (  # noqa: E402
    PROMPT_PLACEHOLDER,
    RESULTS,
    bounded,
    print_answer,
    record,
    report,
    reported_invocation as _shared_reported_invocation,
    rewrite_source_cwd,
    run,
    tree_digest,
)

#: Where this repository keeps its committed native-shaped fixtures.
FIXTURE_ROOT = Path("internal/ccc/testdata/fixtures")
SOURCE_PROVIDER = "claude"
SOURCE_SESSION = "9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10"
TARGET_PROVIDER = "opencode"

GO_TAGS = "fts5"

#: The binary this harness builds and drives. It lives under `bin/`, which `.gitignore` covers, so a run
#: leaves the repository as it found it; the name is its own, so two harnesses run side by side cannot
#: overwrite each other's build mid-run.
BINARY = Path("bin", "ccc-native-check-opencode")

#: The environment variable that names the source root for the conversion's reader, and the ones the
#: harness reads its store from (mirroring `internal/ccc/opencode_install.go`'s `openCodeDataRootEnv` and
#: `openCodeDBEnv`). The harness has no flag for its data directory, so it is set as an environment value --
#: which is also what the printed resume line does, and why that line names the database file inside the
#: root too: a printed shell line can only set variables, never remove an inherited `OPENCODE_DB`, and an
#: inherited override would send the pasted command to another store.
SOURCE_ROOT_ENV = {"claude": "CLAUDE_PROJECTS_DIR"}
OPENCODE_DATA_ROOT_ENV = "XDG_DATA_HOME"
OPENCODE_DB_ENV = "OPENCODE_DB"

#: The names the harness resolves its store under, below the data root.
OPENCODE_DATA_DIR = "opencode"
OPENCODE_STORE = "opencode.db"

#: The id of the session this harness plants, in the harness's own spelling (``ses_`` and 22 characters,
#: the shape the route's own writer mints). It is fabricated: nothing converts it, and no real session,
#: credential or configuration is read to build it.
UNRELATED_SESSION = "ses_nativecheckunrelated0"
UNRELATED_MESSAGE = "msg_unrelated_0000000000000"

#: The directory the planted session records, and which the harness therefore files it under: the
#: temporary directory, which is also what the workspace is not -- so a listing from the workspace must not
#: name the planted session, and a listing from the temporary directory must not name the converted one.
UNRELATED_DIRECTORY = Path(tempfile.gettempdir())

#: The id the control arm asks about: an id in the harness's own spelling that nothing installed. The
#: answer must name it, because "the harness refused" alone is not evidence that it refused *that* id.
UNKNOWN_SESSION = "ses_unknown_control_0000000000"

#: The harness's own words for a session its importer published, and for an id its store does not hold
#: (measured on 2.2.5: the import prints the first on stdout with exit 0, the export prints the second on
#: stderr with exit 1). Both are read beside the exit status rather than instead of it: the harness commits
#: a session and can still exit nonzero, and an import of an id the store holds exits 0 with neither line.
IMPORT_ACKNOWLEDGMENT = "Imported session:"
SESSION_NOT_FOUND = "Session not found:"

#: The bound on one harness invocation: a local command that answers in about a second, given a deadline of
#: its own so a harness that hangs is a failed check rather than a hung run.
HARNESS_TIMEOUT = 300

#: The bound on one real resume turn: a model call in the caller's own session.
TURN_TIMEOUT = 600

#: The bound one of this check's own commands runs under when its call site names none: the shared
#: runner's own default (``native_check_common.run``).
DEFAULT_COMMAND_TIMEOUT = 600


def harness_executable(binary: str) -> str | None:
    """The harness executable this run drives, as a word no working directory changes the meaning of: the
    lookup's own answer, made absolute against this process's own working directory -- the directory
    ``shutil.which`` itself resolves a spelling against.

    ``shutil.which`` returns the spelling it was handed whenever that spelling names a path (a word holding
    a separator is tried directly rather than searched for on ``PATH``), so ``--opencode ./tools/opencode``
    stays relative. Every command this check runs *from the session's own directory* -- the export the
    installation is read back through, its unknown-id control arm and the listing that must not name the
    session -- would then be evaluated against that directory: ``FileNotFoundError``, raised by
    ``subprocess`` rather than answered by any deadline, out of ``main`` before its cleanup and its verdict
    (measured: the planted session stayed in a caller-named home and no ``RESULT`` line was printed).
    Resolving once here makes one program do the probe, the planting, the reading and the turn, whichever
    directory each of those runs in.

    The conversion's own CLI runs from the repository root and resolves a relative ``--target-binary``
    against *its* directory (``internal/ccc/targets.go``'s ``invocationBinary``), so a spelling that means
    different programs in the two directories is refused rather than half-run: the report's argument vector
    is read against the word resolved here (``_read_turn_vector``), and that reading names both words
    instead of comparing the report with itself.
    """
    found = shutil.which(binary)
    if found is None:
        return None
    return found if os.path.isabs(found) else os.path.abspath(found)


def deadline_run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None,
                 timeout: int = DEFAULT_COMMAND_TIMEOUT,
                 replace_env: bool = False) -> subprocess.CompletedProcess:
    """Run one command under its own deadline, with a timeout as a recorded failure rather than an
    escaping exception.

    ``native_check_common.run`` answers a command that overruns its bound by raising
    ``subprocess.TimeoutExpired``; uncaught, that exception ends the run before its own cleanup and its
    verdict -- the published session would stay in a caller-named home (and its fixed id would then make
    every later run's planting meet it), and no final ``RESULT`` line would be printed. Every command
    this check runs therefore goes through here: a command that does not answer inside its bound is
    recorded as a failed check of its own, naming the command and the bound, and answered as a completed
    process with exit status -1 whose stderr carries the same words -- so each caller's own check reports
    the evidence it could not read, and the run still reaches the published session's removal and its
    verdict.

    What the command printed before it overran is kept (``captured_stream``): a slow import that answered
    ``Imported session: <id>`` and then hung is exactly the case the route's own installer reconciles, and
    the acknowledgment is what ``plant_unrelated`` reads -- dropping it would make the check read a
    publication as an absence of one."""
    try:
        return run(cmd, cwd=cwd, env=env, timeout=timeout, replace_env=replace_env)
    except subprocess.TimeoutExpired as exc:
        words = f"timed out after {timeout}s: {bounded(' '.join(cmd), 300)}"
        record(f"{' '.join(cmd[:2])} answers within its {timeout}s bound", False, words)
        captured = captured_stream(exc, "stderr")
        if captured and not captured.endswith("\n"):
            captured += "\n"
        return subprocess.CompletedProcess(cmd, -1, captured_stream(exc, "stdout"), captured + words)


def captured_stream(exc: subprocess.TimeoutExpired, name: str) -> str:
    """One stream of a command that overran its deadline, as text.

    ``subprocess`` buffers what a command's pipes carried while it runs and hands it to the exception as
    the *bytes* it read, even where the runner asked for text (measured on this host's Python 3.15:
    ``exc.stdout`` is ``b'...'`` for a ``text=True`` run), so both shapes are decoded here rather than
    the timed-out command's own words being dropped with its deadline."""
    value = getattr(exc, name, None)
    if isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    return value or ""


def opencode_environment(data_root: Path) -> dict[str, str]:
    """The environment one OpenCode command this check runs is given: this process's own, with the data
    root named and the database override removed (it would point the harness at another store -- the same
    rule ``internal/ccc/opencode_install.go``'s own environment applies). ``ask_opencode`` passes this
    dictionary as the command's whole environment, so the removal is what the harness sees rather than a
    name restored from this process's own environment underneath it."""
    environment = dict(os.environ)
    environment[OPENCODE_DATA_ROOT_ENV] = str(data_root)
    environment.pop(OPENCODE_DB_ENV, None)
    return environment


def opencode_data_root_of(target: Path) -> Path:
    """The environment root the harness's own rule computes for one directory a caller named, mirroring
    ``internal/ccc/opencode_install.go``'s ``openCodeDataRoot``: a directory whose own name is the data
    directory's name (``opencode``) *is* the data directory, so the root that ``$XDG_DATA_HOME`` must name
    for ``$XDG_DATA_HOME/opencode`` to be that directory is its parent. Any other directory is the root
    itself. Without the rule, ``--target-home scratch/opencode`` would plant and export sessions from
    ``scratch/opencode/opencode/opencode.db`` while the conversion installed into
    ``scratch/opencode/opencode.db``, and a successful conversion would be checked against the wrong
    database."""
    return target.parent if target.name == OPENCODE_DATA_DIR else target


def conversion_target_home(target_home: Path | None, data_root: Path) -> Path:
    """The directory the conversion's ``--target-home`` names: the caller's own argument (already resolved
    by the caller, which is the spelling the route's own ``openCodeDataRoot`` then normalizes exactly
    once), or this run's scratch root when the caller named none. This check's already-normalized root is
    never passed on: it applies its own rule (``opencode_data_root_of``) to the same argument, and a root
    that itself names a data directory would be normalized a second time by the route, which would install
    into another store than the one this check reads (measured shape:
    ``--target-home scratch/opencode/opencode`` resolves to ``scratch/opencode`` on both sides, where
    handing the conversion the normalized ``scratch/opencode`` installed into ``scratch/opencode/opencode.db``
    while this check read ``scratch/opencode/opencode/opencode.db``)."""
    return target_home if target_home is not None else data_root


def opencode_arguments(verb: list[str]) -> list[str]:
    """One harness invocation's arguments: this check always runs the harness standalone, so the managed
    background service (which holds a port of its own and can be left behind by an earlier run) is never
    involved. Mirrors ``--standalone`` in ``internal/ccc/opencode_verify.go``, and the flag stands
    immediately after the subcommand -- the spelling every verb this check runs was measured to accept --
    because a run whose flag lands *after* the ``--`` terminator leaves it on the managed service and makes
    the prompt carry it (measured: such a turn answered from a session the check's own store did not grow)."""
    return [verb[0], "--standalone", *verb[1:]]


def ask_opencode(opencode: str, verb: list[str], data_root: Path, cwd: Path | None = None,
                 timeout: int = HARNESS_TIMEOUT) -> subprocess.CompletedProcess:
    """Run one harness command and answer its completed process. This is the seam the offline cases
    replace, so a case can answer the harness's own protocol without the harness installed.

    The environment is passed as the command's *whole* environment (``replace_env``): the dictionary
    ``opencode_environment`` builds is this process's own with the data root named and the database
    override removed, and an overlay would restore the removed name from this process's environment --
    sending the command to a database this check did not choose."""
    return deadline_run([opencode, *opencode_arguments(verb)], cwd=cwd, env=opencode_environment(data_root),
                        timeout=timeout, replace_env=True)


def unrelated_document() -> dict:
    """The document this harness plants an unrelated session with: the members the harness's own importer
    requires (measured: `info.id`, `info.projectID`, `info.cost`, `info.tokens`, `info.time.updated` and
    `info.location.directory`, and a user message's own `time`/`text`/`files`), and nothing else. The
    session holds one turn this run must not touch."""
    return {
        "info": {
            "id": UNRELATED_SESSION,
            "projectID": "",
            "cost": 0,
            "tokens": {"input": 0, "output": 0, "reasoning": 0, "cache": {"read": 0, "write": 0}},
            "time": {"created": 1760000000000, "updated": 1760000000000},
            "location": {"directory": str(UNRELATED_DIRECTORY)},
        },
        "messages": [{
            "id": UNRELATED_MESSAGE,
            "type": "user",
            "time": {"created": 1760000000000},
            "text": "a session this run must not touch",
            "files": [],
        }],
    }


def harness_line_names(answer: str, marker: str, session_id: str) -> bool:
    """Whether one of a harness answer's own lines is its ``<marker> <id>`` line naming this id, read the way
    the route's own installer reads those answers (``internal/ccc/opencode_install.go``'s
    ``openCodeImportAnswer`` and ``openCodeRefusalNamesSession``): the words after the marker must be this
    id and nothing else, so a diagnostic that merely mentions the id is not read as the harness's answer
    about it -- measured: a timed-out command's own recorded stderr echoes its command line, which names the
    session id it was asked about."""
    for line in (answer or "").splitlines():
        rest = line.strip()
        if rest.startswith(marker) and rest[len(marker):].strip() == session_id:
            return True
    return False


def unrelated_is_this_checks(document: dict | None) -> bool:
    """Whether one document a store holds *is* this check's own fabricated unrelated fixture: the id it
    plants, the directory that session records, and the one message text it writes. Nothing else writes
    those three -- nothing converts the document, and no real session, credential or configuration is read
    to build it -- so a store holding one holds a leftover from an earlier run of this check, while a
    session under the same id with any other document is one this check must not touch."""
    if not isinstance(document, dict):
        return False
    info = document.get("info") or {}
    if info.get("id") != UNRELATED_SESSION:
        return False
    if (info.get("location") or {}).get("directory") != str(UNRELATED_DIRECTORY):
        return False
    planted = unrelated_document()["messages"][0]["text"]
    return any((message.get("text") if isinstance(message, dict) else None) == planted
               for message in (document.get("messages") or []))


def clear_own_unrelated_leftover(opencode: str, data_root: Path) -> bool:
    """Make the id this run plants free, and answer whether it now is.

    The id and the document are fabricated, but an earlier run of this check can still have published the
    session and died before its cleanup (the window ``plant_unrelated`` reconciles), and that leftover
    would make every later run's planting meet ``Session already exists``. Such a leftover is recognised
    by the document the store's own reader answers for it (``unrelated_is_this_checks``) and removed
    through the harness's own delete. A session under that id that is *not* this check's own fixture is a
    caller's own store and is left exactly as it was found: the run records the refusal and nothing here
    removes it.

    The id is free only when the harness's own reader answers that it holds nothing under it -- its own
    ``Session not found: <id>`` line, naming *this* id. A store that could not answer at all is not a
    store shown to be free: the import would publish into it, and the reconciliation below, reading a
    failure against a store that was never shown to be free, could then call a session this run did not
    publish its own and remove it."""
    document, words, status = export_document(opencode, data_root, UNRELATED_SESSION)
    if document is None and status != 0 and harness_line_names(words, SESSION_NOT_FOUND, UNRELATED_SESSION):
        record("the id this run plants is free in the store", True, bounded(words, 200))
        return True
    if document is None:
        record("the id this run plants is free in the store", False,
               f"the harness's own reader could not answer whether it holds {UNRELATED_SESSION} "
               f"(exit {status}; {bounded(words or 'no answer', 200)}), and a store that answered nothing "
               f"is not a store shown to be free")
        return False
    if not unrelated_is_this_checks(document):
        record("the id this run plants is free in the store", False,
               f"the store already holds {UNRELATED_SESSION} and its own document is not this check's "
               f"fixture, so this run may not remove it: {bounded(json.dumps(document), 300)}")
        return False
    completed = ask_opencode(opencode, ["session", "delete", UNRELATED_SESSION], data_root)
    answer = (completed.stdout or "") + (completed.stderr or "")
    after, after_words, after_status = export_document(opencode, data_root, UNRELATED_SESSION)
    removed = (completed.returncode == 0 and f"Session {UNRELATED_SESSION} deleted" in answer
               and after is None)
    record("an earlier run's own unrelated fixture is removed before this run plants", removed,
           bounded(answer if removed else
                   f"{answer}; the harness's own reader answers exit {after_status}: "
                   f"{after_words or 'nothing'}", 300))
    return removed


def plant_unrelated(opencode: str, data_root: Path, scratch: Path) -> tuple[bool, str]:
    """Plant the unrelated session **through the harness's own importer**, which is the only way a store is
    written here: the document is staged in the run's scratch tree and handed over exactly as the route's
    own installer hands a converted document over.

    It answers whether **this run published** the session, which is not the same question as whether the
    import process exited 0. Measured on 2.2.5, the harness commits the session and can still fail, or be
    killed, before it answers -- the window ``InstallOpenCodeSession`` reconciles -- and a run that read
    only the exit status would leave its own fixture in a caller-named store, where its fixed id makes
    every later run's planting meet ``Session already exists``. The evidence is read in the route's own
    order: the harness's own ``Imported session: <id>`` line in the captured output (kept even when the
    import overran its deadline), then the harness's own reader, whose answered document means the store
    holds the session. An acknowledgment the reader does not confirm, and a store that answers nothing,
    are reported as unresolved rather than as either outcome."""
    staged = scratch / "unrelated-import.json"
    staged.write_text(json.dumps(unrelated_document()), encoding="utf-8")
    completed = ask_opencode(opencode, ["session", "import", str(staged), "--directory",
                                        str(UNRELATED_DIRECTORY)], data_root)
    answer = (completed.stdout or "") + (completed.stderr or "")
    acknowledged = harness_line_names(answer, IMPORT_ACKNOWLEDGMENT, UNRELATED_SESSION)
    if completed.returncode == 0 and acknowledged:
        return True, bounded(answer, 300)
    document, words, status = export_document(opencode, data_root, UNRELATED_SESSION)
    if document is not None:
        return True, bounded(
            f"the import process exited {completed.returncode} and its own acknowledgment "
            f"{'was' if acknowledged else 'was not'} printed, while the harness's own reader holds "
            f"{UNRELATED_SESSION}: this run published it, and removes it again. {answer}", 300)
    if acknowledged:
        return False, bounded(
            f"the harness's own acknowledgment names {UNRELATED_SESSION} but its own reader does not "
            f"resolve it (exit {status}; {words or 'no answer'}): unresolved, and nothing here removes "
            f"what it could not show is there. {answer}", 300)
    return False, bounded(answer, 300)


def remove_planted(opencode: str, data_root: Path, published: bool) -> None:
    """Remove the session **this run published** through the harness's own `session delete`, and show that
    it is gone.

    This runs on every path that published one -- after the last check, and before the report removes the
    scratch tree the store lives in (measured: a delete after the store's directory is gone resolves a
    fresh empty store and answers "Session not found", which is not a removal) -- so a caller-named
    ``--target-home`` is left as it was found apart from the converted session this run installed. The
    removal is a check of its own: a fabricated session this harness leaves behind is a session the
    caller's own listing would show. A run that published nothing removes nothing, because the store's
    other sessions are not this check's to delete."""
    if not published:
        return
    completed = ask_opencode(opencode, ["session", "delete", UNRELATED_SESSION], data_root)
    answer = (completed.stdout or "") + (completed.stderr or "")
    record("the planted session is removed through the harness's own session delete",
           completed.returncode == 0 and f"Session {UNRELATED_SESSION} deleted" in answer,
           bounded(answer, 200))
    after, words, status = export_document(opencode, data_root, UNRELATED_SESSION)
    record("the removed session no longer resolves",
           after is None and status != 0 and UNRELATED_SESSION in words,
           f"exit {status}; {bounded(words, 200)}")


def export_document(opencode: str, data_root: Path, session_id: str, cwd: Path | None = None,
                    ) -> tuple[dict | None, str, int]:
    """Ask the harness's own reader for one session's document. It answers the parsed document (or None),
    the harness's own words and its exit status; a session the store does not hold is the harness's own
    refusal rather than an exception, so a case can assert on it.

    Measured on 2.2.5: `session export` answers from the store and changes none of the session's own records,
    so this is the reader a caller's own harness would use -- and the reason the route's load check uses it
    rather than `run`, which *creates* a session for an unknown id."""
    completed = ask_opencode(opencode, ["session", "export", session_id], data_root, cwd=cwd)
    words = (completed.stdout or "") if completed.returncode == 0 else (completed.stderr or "")
    document = None
    if completed.returncode == 0:
        try:
            document = json.loads(completed.stdout)
        except json.JSONDecodeError:
            document = None
    return document, words.strip(), completed.returncode


def session_list(opencode: str, data_root: Path, cwd: Path) -> tuple[list[dict] | None, str]:
    """The harness's own project-scoped listing of the sessions it holds, as the harness answers it from
    `cwd`: measured on 2.2.5, `session list --format json` lists the sessions of the *current directory's*
    project (and `[]` from a directory that is not the session's own). None means the harness did not
    answer a listing, which is a failed check rather than an empty one."""
    completed = ask_opencode(opencode, ["session", "list", "--format", "json"], data_root, cwd=cwd)
    if completed.returncode != 0:
        return None, bounded(completed.stderr or completed.stdout or "", 300)
    try:
        listing = json.loads(completed.stdout or "[]")
    except json.JSONDecodeError:
        return None, bounded(completed.stdout, 300)
    if not isinstance(listing, list):
        return None, bounded(completed.stdout, 300)
    return listing, ""


def document_texts(document: dict, kind: str) -> list[str]:
    """The text of every message of one type in a document, in the document's own order: a user message
    carries its words in `text`, and an assistant message carries them in its text parts."""
    texts: list[str] = []
    for message in document.get("messages") or []:
        if message.get("type") != kind:
            continue
        if isinstance(message.get("text"), str):
            texts.append(message["text"])
        for part in message.get("content") or []:
            if isinstance(part, dict) and part.get("type") == "text" and isinstance(part.get("text"), str):
                texts.append(part["text"])
    return texts


def tool_result_text(content: object) -> str:
    """The text the inherited parser reads out of one ``tool_result`` block's content
    (``internal/parser/content.go``'s ``DecodeContent``): a string is itself, and an array of blocks
    contributes each block's ``text``, or its ``result``, joined in the block's own order."""
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return ""
    parts: list[str] = []
    for block in content:
        if not isinstance(block, dict):
            continue
        if isinstance(block.get("text"), str) and block["text"]:
            parts.append(block["text"])
        elif isinstance(block.get("result"), str) and block["result"]:
            parts.append(block["result"])
    return "".join(parts)


def source_tool_evidence(source: Path) -> dict[str, dict]:
    """The tool evidence one Claude transcript records, from the transcript's own bytes: for every
    ``tool_use`` block, its id, name, recorded input value and the text of the ``tool_result`` block
    recorded for it.

    This is the native check's independent side: the store's own document is compared against the source
    fixture rather than against the conversion's report or its intended items, so a writer that recorded
    different arguments or different output cannot pass by agreeing with itself. The reading mirrors the
    inherited parser's own two rules in the small -- a result's text is ``tool_result_text``
    (``DecodeContent``), and ``is_error`` is the ``errored`` status the reader records, which this
    target's vocabulary writes as the ``error`` state. A call the transcript records with no result at
    all, or with one whose text is blank, is not expected from the store: the writer leaves such a call
    out and counts it (``unpaired_tool_calls``/``empty_tool_results``) rather than writing a state its
    importer refuses.
    """
    evidence: dict[str, dict] = {}
    for line in source.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        try:
            record_ = json.loads(stripped)
        except json.JSONDecodeError:
            continue
        if not isinstance(record_, dict):
            continue
        message = record_.get("message")
        if not isinstance(message, dict):
            continue
        content = message.get("content")
        if not isinstance(content, list):
            continue
        for block in content:
            if not isinstance(block, dict):
                continue
            if block.get("type") == "tool_use":
                evidence[str(block.get("id", ""))] = {
                    "name": str(block.get("name", "")),
                    "status": "completed",
                    "input": block.get("input"),
                    "result": None,
                }
            elif block.get("type") == "tool_result":
                entry = evidence.get(str(block.get("tool_use_id", "")))
                if entry is None:
                    continue
                entry["result"] = tool_result_text(block.get("content"))
                entry["status"] = "error" if block.get("is_error") is True else "completed"
    return {call_id: entry for call_id, entry in evidence.items()
            if entry["result"] is not None and entry["result"].strip() != ""}


def document_tool_evidence(document: dict) -> list[dict]:
    """Every tool part the harness's own document holds, with the members this check compares: the call's
    identity (id, name, status) and the store's own copy of the recorded evidence -- the ``state.input``
    value, and the completed state's text (its ``state.content`` blocks) or an error state's message.

    The identity alone is not evidence: a part whose id, name and ``completed`` status are right can
    still carry different arguments or different output, which is exactly the shape a writer that mangled
    either member produces -- and the structural round trip cannot see it when the conversion's own
    intended items hold the same mangled bytes. The recorded input is compared as the JSON *value* both
    sides parsed, because the store keeps the value and the recorded key order while the recorded
    whitespace is not what it keeps (measured on 2.2.5); the two sides are compared by
    ``tool_evidence_differences``."""
    evidence: list[dict] = []
    for message in document.get("messages") or []:
        for part in message.get("content") or []:
            if not isinstance(part, dict) or part.get("type") != "tool":
                continue
            state = part.get("state") or {}
            text = ""
            content = state.get("content")
            if isinstance(content, list):
                text = "".join(block.get("text", "") for block in content
                               if isinstance(block, dict) and isinstance(block.get("text"), str))
            error = state.get("error")
            if isinstance(error, dict) and isinstance(error.get("message"), str):
                text = error["message"]
            evidence.append({
                "id": str(part.get("id", "")),
                "name": str(part.get("name", "")),
                "status": str(state.get("status", "")),
                "input": state.get("input"),
                "result": text,
            })
    return evidence


def tool_evidence_differences(expected: dict[str, dict], exported: list[dict]) -> list[str]:
    """Every way the store's own tool evidence differs from the source fixture's, as sentences a FAIL line
    can carry: the call's identity, its recorded input value and its recorded result text are each
    compared, and a part the fixture records nothing about -- or one the store holds twice -- is a
    difference of its own. An empty answer is the store matching the fixture."""
    differences: list[str] = []
    seen: dict[str, dict] = {}
    for part in exported:
        if part["id"] in seen:
            differences.append(f"the store's document holds {part['id']} more than once")
            continue
        seen[part["id"]] = part
    for call_id, want in expected.items():
        part = seen.pop(call_id, None)
        if part is None:
            differences.append(f"the store's document holds no part for the source's call {call_id}")
            continue
        if part["name"] != want["name"]:
            differences.append(f"{call_id}'s name is {part['name']!r}, the source recorded "
                               f"{want['name']!r}")
        if part["status"] != want["status"]:
            differences.append(f"{call_id}'s status is {part['status']!r}, the source's result is "
                               f"{want['status']!r}")
        if part["input"] != want["input"]:
            differences.append(f"{call_id}'s input is {bounded(json.dumps(part['input']), 120)}, the "
                               f"source recorded {bounded(json.dumps(want['input']), 120)}")
        if part["result"] != want["result"]:
            differences.append(f"{call_id}'s result text is {bounded(part['result'], 120)!r}, the source "
                               f"recorded {bounded(want['result'], 120)!r}")
    for call_id in sorted(seen):
        differences.append(f"the store's document holds a part for {call_id}, which the source fixture "
                           f"records no carryable result for")
    return differences


def recorded_tool_evidence(evidence: list[dict]) -> str:
    """The detail a *passing* tool-evidence check records: every part's identity and what it carries, so
    the line shows the evidence the store's own document holds rather than only a count."""
    return "; ".join(
        f"{part['id']} {part['name']} [{part['status']}] input {bounded(json.dumps(part['input']), 80)} "
        f"result {bounded(part['result'], 60)!r}" for part in evidence)


def document_compactions(document: dict) -> list[dict]:
    """Every compaction message a document holds, whatever members it carries."""
    return [message for message in (document.get("messages") or [])
            if message.get("type") == "compaction"]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--direction", choices=(f"{SOURCE_PROVIDER}-to-{TARGET_PROVIDER}",),
                        default=f"{SOURCE_PROVIDER}-to-{TARGET_PROVIDER}",
                        help="the route this run verifies; this target has one direction, because the "
                             "harness's own reader is already the source side's (the inherited reader "
                             "reads its store, so an OpenCode source needs no second parser)")
    parser.add_argument("--opencode", default="opencode",
                        help="the OpenCode binary to verify against (default: opencode on PATH)")
    parser.add_argument("--source", default=None,
                        help=f"the source session to convert (default: the committed {SOURCE_SESSION} "
                             f"Claude fixture)")
    parser.add_argument("--target-home", type=Path,
                        help="the harness's data root (default: a scratch one). A directory whose own name "
                             "is `opencode` is the data directory itself -- the rule the route itself "
                             "applies, whose parent then becomes the root -- so the conversion and this "
                             "check read the same store: the conversion is handed this argument's own "
                             "spelling and each side applies the rule once. A caller's own root is used "
                             "exactly as it is found: nothing is installed into it but the converted "
                             "session, and the unrelated session this run plants is removed again -- an "
                             "id the store already holds is removed first only where its own document "
                             "shows it to be an earlier run's leftover of this same check, and a session "
                             "holding anything else stops the run untouched")
    parser.add_argument("--resume-prompt",
                        help="run one real turn in the installed session with this prompt")
    parser.add_argument("--expect", help="require the resume turn's answer to contain this text")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    opencode = harness_executable(args.opencode)
    if opencode is None:
        print(f"RESULT: INCOMPLETE — 0 check(s) ran; {args.opencode} is not on PATH, so the native half of "
              f"the {TARGET_PROVIDER} route could not run (this route's installation *is* the harness's own "
              f"importer, so there is nothing to verify without it)")
        return 2
    go = shutil.which("go")
    if go is None:
        print("go is not on PATH: this harness builds ccc with the Go toolchain", file=sys.stderr)
        return 2

    source_session = args.source or SOURCE_SESSION
    candidates = sorted((fixture_root / SOURCE_PROVIDER).rglob(f"*{source_session}.jsonl"))
    if len(candidates) != 1:
        print(f"expected exactly one {SOURCE_PROVIDER} fixture for {source_session} below "
              f"{fixture_root / SOURCE_PROVIDER}, found {len(candidates)}", file=sys.stderr)
        return 2
    source = candidates[0]

    scratch = Path(tempfile.mkdtemp(prefix="ccc-opencode-native-check-")).resolve()
    workspace = scratch / "workspace"
    workspace.mkdir(parents=True, exist_ok=True)
    source_root = scratch / f"{SOURCE_PROVIDER}-source"
    destination = source_root / source.relative_to(fixture_root / SOURCE_PROVIDER)
    recorded_cwd = rewrite_source_cwd(source, destination, str(workspace))

    named_home = args.target_home is not None
    # A named directory is read through the harness's own rule first (opencode_data_root_of): a directory
    # whose own name is `opencode` is the data directory itself, and the root this check's own environment
    # must name for the harness to read that directory is its parent.
    data_root = (opencode_data_root_of(args.target_home.resolve())
                 if args.target_home else scratch / "opencode-data")
    # The conversion is handed the caller's own spelling, never this check's normalized root: each side
    # applies the harness's rule exactly once to the same argument, so both resolve one store (without
    # it, a caller spelling that names a data directory split the two: the check read
    # scratch/opencode/opencode/opencode.db while the conversion installed into scratch/opencode/opencode.db).
    conversion_home = conversion_target_home(args.target_home.resolve() if named_home else None, data_root)
    data_dir = data_root / OPENCODE_DATA_DIR
    store = data_dir / OPENCODE_STORE
    data_root.mkdir(parents=True, exist_ok=True)
    data_dir.mkdir(parents=True, exist_ok=True)

    version = deadline_run([opencode, "--version"]).stdout.strip()
    print(f"repo:        {repo}")
    print(f"direction:   {SOURCE_PROVIDER}:{source_session} → {TARGET_PROVIDER}")
    print(f"target:      {args.opencode} ({opencode}; {version[:60] or 'no version line'})")
    print(f"source:      {source.relative_to(repo)} (copied to {destination})")
    print(f"workspace:   {workspace}")
    print(f"data root:   {data_root}"
          + ("" if named_home else " (a scratch one this run made)")
          + (f"; the named directory resolves to it through the harness's own rule, because its own name "
             f"is {OPENCODE_DATA_DIR}" if named_home and data_root != args.target_home.resolve() else "")
          + f"; the harness's own data directory is {data_dir}, its store {store.name}")
    print(f"the source fixture's recorded working directory ({recorded_cwd or 'none recorded'}) is rewritten "
          f"to the workspace: the harness's own importer refuses a session whose directory does not exist "
          f"(measured: LocationNotFoundError)")
    print()

    parsed: dict = {}
    session_id = ""
    published = False

    def stop() -> int:
        """End the run on whatever state it reached: the session this run published is removed through the
        harness's own verb while the store still exists, and the report -- which removes the scratch tree --
        comes after."""
        remove_planted(opencode, data_root, published)
        return finish(args, scratch, parsed, session_id)

    source_root_before = tree_digest(source_root)
    if not clear_own_unrelated_leftover(opencode, data_root):
        return stop()
    published, planted_detail = plant_unrelated(opencode, data_root, scratch)
    record("the unrelated session is planted through the harness's own importer", published, planted_detail)
    if not published:
        return stop()
    unrelated_before, unrelated_words, unrelated_status = export_document(opencode, data_root,
                                                                         UNRELATED_SESSION)
    record("the harness's own reader answers the planted session's document",
           unrelated_before is not None,
           bounded(unrelated_words, 200) if unrelated_before is None else f"exit {unrelated_status}")
    if unrelated_before is None:
        return stop()

    build_parent = (repo / BINARY).parent
    build_parent.mkdir(parents=True, exist_ok=True)
    build = deadline_run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
    if build.returncode != 0:
        record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
        return stop()
    record("ccc builds with the flags upstream ships", True)

    # The conversion is verified at the load level even when a turn was requested: with --resume-prompt
    # this check runs that one real turn itself, below, through the invocation the report printed -- so
    # asking the CLI's own resume verification as well would run and bill a second turn for one request
    # (the shape the round's reading measured: the conversion grew the session 7 -> 10 and the check's own
    # turn grew it 10 -> 13). The load level bills nothing and writes no records.
    command = [str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{source_session}", "--to",
               TARGET_PROVIDER, "--target-home", str(conversion_home), "--target-binary", opencode,
               "--verify", "load", "--json"]
    convert = deadline_run(command, cwd=repo, env={SOURCE_ROOT_ENV[SOURCE_PROVIDER]: str(source_root)})
    try:
        document = json.loads(convert.stdout)
    except json.JSONDecodeError:
        record("ccc continue writes one JSON result document", False,
               bounded(convert.stdout or convert.stderr, 400))
        return stop()
    record("ccc continue writes one JSON result document", True)
    parsed = document

    conversion = document.get("conversion") or {}
    installation = document.get("installation") or {}
    target = document.get("target_session") or {}
    verification = document.get("verification") or {}
    losses = {loss.get("category"): loss.get("count") for loss in conversion.get("losses") or []}
    carried = conversion.get("carried") or {}
    record("the conversion reports the conversation it carried and every loss category",
           conversion.get("state") == "converted" and carried.get("messages", 0) > 0 and bool(losses),
           f"carried {carried.get('messages')} message(s), {carried.get('tool_calls')} call(s), "
           f"{carried.get('tool_results')} result(s); loss categories named: {len(losses)}")
    record("the installation names the harness's own importer",
           installation.get("state") == "installed"
           and "opencode session import" in (installation.get("mechanism") or ""),
           f"{installation.get('state')} — {bounded(installation.get('mechanism') or '', 200)}")
    record("ccc continue exits 0", convert.returncode == 0,
           f"exit {convert.returncode}; stderr {bounded(convert.stderr, 300)}")
    structure = verification.get("structure") or {}
    record("the structural read-back verified the installed session",
           structure.get("state") == "verified", bounded(structure.get("detail") or "", 400))
    load = verification.get("load") or {}
    record("the route's own load check verified the installed session natively",
           load.get("state") == "verified", bounded(load.get("detail") or "", 400))
    record_model_binding(target, bool(args.resume_prompt))

    session_id = target.get("session_id") or ""
    record("the installed session has the identity the report names", bool(session_id), session_id)
    if not session_id:
        return stop()
    record("the report names the store the harness reads as the installed path",
           (installation.get("path") or "") == str(store),
           f"{installation.get('path')!r}, want {store}")

    # The harness's own reader, asked by this check rather than read out of the report: the document
    # the store holds is the conversation the writer published.
    exported, export_words, export_status = export_document(opencode, data_root, session_id,
                                                            cwd=workspace)
    if exported is None:
        record("the harness's own reader answers the installed session's document", False,
               f"exit {export_status}; {bounded(export_words, 300)}")
    else:
        record("the harness's own reader answers the installed session's document",
               exported.get("info", {}).get("id") == session_id,
               f"exit {export_status}; id {exported.get('info', {}).get('id')!r}, "
               f"{len(exported.get('messages') or [])} message(s)")
        stored = len(exported.get("messages") or [])
        expected = carried.get("messages", 0)
        # The conversion itself ran no turn: its verification level is load (the harness's own
        # `session export`, measured to write no records), and any real turn this run makes is run by this
        # check afterwards, through the printed invocation. So the store holds exactly the conversion's
        # own messages at this point, and the export above was taken before that turn.
        record("the harness's own reader holds every message the conversion wrote",
               bool(expected) and stored == expected,
               f"the store's own document holds {stored} message(s); the conversion carried {expected}" +
               (" (the conversion ran no turn; this check's own turn, if requested, comes after this read)"
                if args.resume_prompt else ""))
        user_texts = document_texts(exported, "user")
        record("the installed session holds the source's own first prompt",
               bool(user_texts) and user_texts[0].strip().startswith("Summarize the retry policy"),
               f"{user_texts[0][:80]!r}" if user_texts else "no user text")
        expected_evidence = source_tool_evidence(destination)
        evidence = document_tool_evidence(exported)
        evidence_differences = tool_evidence_differences(expected_evidence, evidence)
        record("the installed session holds the source's tool call and its recorded result",
               not evidence_differences,
               bounded("; ".join(evidence_differences) if evidence_differences
                       else recorded_tool_evidence(evidence), 400))
        compactions = document_compactions(exported)
        record("the installed session holds the compaction boundary with the harness's own members",
               len(compactions) == 1 and compactions[0].get("status") == "completed"
               and compactions[0].get("reason") == "auto" and bool(compactions[0].get("summary")),
               json.dumps(compactions[0])[:200] if compactions else "no compaction message")

    refusal, refusal_words, refusal_status = export_document(opencode, data_root, UNKNOWN_SESSION)
    record("the unknown-id control arm reports no such session, naming the id",
           refusal is None and refusal_status != 0 and UNKNOWN_SESSION in refusal_words,
           f"exit {refusal_status}; {bounded(refusal_words, 200)}")

    here, here_words = session_list(opencode, data_root, workspace)
    record("the harness's own listing names the installed session from its own directory",
           here is not None and any(entry.get("id") == session_id for entry in here),
           f"{[entry.get('id') for entry in here][:4]}" if here else bounded(here_words or "no listing", 200))
    elsewhere, elsewhere_words = session_list(opencode, data_root, UNRELATED_DIRECTORY)
    record("the same listing from another directory does not name it",
           elsewhere is not None and not any(entry.get("id") == session_id for entry in elsewhere),
           f"{len(elsewhere)} session(s) listed from {UNRELATED_DIRECTORY}"
           if elsewhere is not None else bounded(elsewhere_words, 200))

    # The printed invocation, read the way a POSIX shell reads it and checked against this run's own
    # harness and session, and then -- when a turn was asked for -- run in its own words.
    _check_invocation(target, opencode, workspace, data_root, store)
    if args.resume_prompt:
        _run_turn(args, target, opencode, data_root, store, workspace)
    else:
        record("a real turn was requested (--resume-prompt)", False,
               "not requested: this run verifies the offline half (the importer, the harness's own "
               "reader and listing, the printed invocation) and runs no model turn",
               skipped=True)

    source_root_after = tree_digest(source_root)
    record("the source fixture tree is unchanged", source_root_after == source_root_before,
           f"{len(source_root_before)} file(s) digested before and after")
    unrelated_after, unrelated_words_after, unrelated_status_after = export_document(
        opencode, data_root, UNRELATED_SESSION)
    record("the planted unrelated session is unchanged",
           unrelated_after is not None and unrelated_after == unrelated_before,
           f"exit {unrelated_status_after}; its document is "
           + ("byte-identical before and after" if unrelated_after == unrelated_before
              else bounded(unrelated_words_after, 200)))

    return stop()


def record_model_binding(target: dict, turn_requested: bool) -> None:
    """Record what the report answers about the harness's own version and the model a turn would run
    under.

    A real turn runs under the home's own resolved binding, so one needs a complete pair; the offline
    half does not. Measured: a home that configures no model answers `debug agents` with its primary agent
    carrying `model: null`, the writer omits `info.model`, and the importer, the harness's own reader and
    the load level all work without one -- a supported run whose store holds every message. The report
    names that absence in `model_provider_detail` (with `model_provider_source` naming where the answer,
    or its absence, came from), so an offline run accepts it instead of ending FAIL for a configuration
    the harness itself answers with."""
    model = str(target.get("model") or "")
    provider = str(target.get("model_provider") or "")
    detail = str(target.get("model_provider_detail") or "")
    binding = bool(model) and bool(provider)
    record("the report names the harness's own version", bool(target.get("cli_version")),
           f"cli_version {target.get('cli_version')} ({target.get('cli_version_source')})")
    record("the report names the model a turn would run under, or says why the harness resolved none",
           binding or (not turn_requested and bool(detail)),
           f"model {provider or '(none recorded)'}/{model or '(none recorded)'} "
           f"({target.get('model_provider_source') or 'no source named'})"
           + (f"; the harness's own answer: {detail}" if detail else "")
           + ("" if binding or not turn_requested
              else " — a real turn was requested, so the home must resolve a model to run it under"))


def _read_turn_vector(target: dict, opencode: str, arguments_field: str,
                      prompt_word: str) -> tuple[list[str], str]:
    """One reported argument vector read against what this check knows on its own, rather than against
    the report's other fields: the harness this run probed and handed to the conversion, this
    conversion's own session id, the ``--standalone`` flag on the subcommand's own side of the ``--``
    terminator (past it, the harness reads it as part of the prompt and answers from the managed service,
    measured), the terminator itself, and the single prompt word behind it (the placeholder in the exec
    form, the empty prompt in the plain one).

    It answers the vector and the first rule it breaks ("" when it holds). A vector that only agreed with
    the report's own words could name another session, another binary or no terminator at all while the
    printed-line checks compared it with nothing but itself."""
    session_id = str(target.get("session_id") or "")
    words = [str(word) for word in (target.get(arguments_field) or [])]
    if not words:
        return [], f"the report printed no {arguments_field}"
    if Path(words[0]) != Path(opencode):
        return [], f"its program word is {words[0]!r}, not the harness this run probed ({opencode})"
    if words[1:2] != ["run"]:
        return [], f"its subcommand is {words[1:2]!r}, not the turn subcommand `run`"
    rest = words[2:]
    if rest[:1] != ["--standalone"]:
        return [], ("`--standalone` does not stand immediately after the subcommand: without it the turn "
                    "answers from the managed service, and a flag past the `--` terminator is read as part "
                    "of the prompt (measured)")
    if "--" not in rest:
        return [], "it carries no `--` terminator between its options and the prompt"
    terminator = rest.index("--")
    options = rest[:terminator]
    if "--session" not in options:
        return [], "it names no `--session`: the turn would create a session instead of resuming one"
    session_at = options.index("--session")
    named = options[session_at + 1] if session_at + 1 < len(options) else None
    if named != session_id:
        return [], f"its `--session` names {named!r}, not this conversion's own session {session_id!r}"
    prompt = rest[terminator + 1:]
    if prompt != [prompt_word]:
        return [], (f"its prompt side is {prompt!r}, not the single {prompt_word!r} word this check "
                    "expects behind the terminator")
    return words, ""


def reported_turn(target: dict, opencode: str) -> tuple[list[str], str]:
    """The exec-form invocation the report printed, as the turn this check runs: validated by
    ``_read_turn_vector`` and answered with its prompt placeholder still in it, for the caller to
    substitute the prompt into."""
    return _read_turn_vector(target, opencode, "exec_resume_arguments", PROMPT_PLACEHOLDER)


def turn_environment(target: dict, data_root: Path, store: Path) -> tuple[dict[str, str], str]:
    """The environment the turn runs with: this process's own, with every name the report's own
    invocation sets set to the report's value -- after checking that the pair is this run's own data root
    and store. A report that named another store would send the turn somewhere this check does not read,
    and ``run --session <id>`` *creates* a session for an id the store does not hold (measured)."""
    reported = target.get("resume_environment") or {}
    if not isinstance(reported, dict):
        return {}, f"the report's resume_environment is {reported!r}, not an object of names"
    reported = {str(name): str(value) for name, value in reported.items()}
    expected = {OPENCODE_DATA_ROOT_ENV: str(data_root), OPENCODE_DB_ENV: str(store)}
    if reported != expected:
        return {}, f"the report names the environment {reported!r}, not this run's own {expected!r}"
    environment = dict(os.environ)
    environment.update(expected)
    return environment, ""


def _check_invocation(target: dict, opencode: str, workspace: Path, data_root: Path,
                      store: Path) -> None:
    """Check the printed resume commands by reading them the way a POSIX shell reads them: the working
    directory the harness is started from, the data root it reads and the database inside it the line
    pins -- all named -- against the argument vector the report itself printed, and the reported
    environment against the same pair.

    Each of the report's two vectors is also read against what this check knows on its own
    (``_read_turn_vector``), so a consistently wrong pair cannot pass by agreeing with itself: the line
    must name the harness this run probed and this conversion's session, run it standalone on the
    subcommand's own side of the terminator, and carry that form's own prompt word.

    Pinning the database is the rule the environment cases in ``scripts/tests/test_opencode_native_check.py``
    keep: a printed line can only set variables, so it names this home's own store instead of removing an
    inherited ``OPENCODE_DB``, which would otherwise send the pasted command to another store where
    ``run --session <id>`` *creates* a session for the id (measured)."""
    pinned = {OPENCODE_DB_ENV: str(store)}
    expected_environment = {OPENCODE_DATA_ROOT_ENV: str(data_root), **pinned}
    record("the report names the environment the printed invocation sets",
           (target.get("resume_environment") or {}) == expected_environment,
           f"{target.get('resume_environment')!r}, want {expected_environment!r}")
    forms = (("resume_command", "resume_arguments", "", "the empty prompt"),
             ("exec_resume_command", "exec_resume_arguments", PROMPT_PLACEHOLDER,
              f"the {PROMPT_PLACEHOLDER!r} placeholder"))
    for field, arguments_field, prompt_word, prompt_name in forms:
        command = target.get(field, "")
        arguments = target.get(arguments_field) or []
        ok = (bool(command) and bool(arguments)
              and _shared_reported_invocation(command, workspace, data_root, OPENCODE_DATA_ROOT_ENV,
                                              arguments, also=pinned))
        record(f"the printed {field} reads as a POSIX shell line naming the session", ok,
               bounded(command, 220))
        vector, problem = _read_turn_vector(target, opencode, arguments_field, prompt_word)
        record(f"the report's {arguments_field} names the harness this run probed, this session and "
               f"{prompt_name}",
               bool(vector) and not problem,
               problem or bounded(" ".join(vector), 220))


def _run_turn(args: argparse.Namespace, target: dict, opencode: str, data_root: Path, store: Path,
              workspace: Path) -> None:
    """Run one real turn in the installed session through the invocation the report printed.

    The turn is the reported exec-form vector -- validated against this run's own harness and this
    conversion's session, and with the prompt substituted for its placeholder -- run from this run's own
    workspace under the environment the report names (checked to be this run's own data root and store).
    It is what proves the session is one a caller can resume: the harness answers from the installed
    history and appends its own records to it. The growth is measured through the harness's own reader,
    before and after, and the answer is printed under the shared marker -- an answer from a session the
    harness's own reader does not read differently is not a resumed session."""
    arguments, problem = reported_turn(target, opencode)
    if problem:
        record("the turn ran through the printed invocation and exited 0", False,
               f"the report's own invocation is not the turn this check runs: {problem}")
        return
    environment, problem = turn_environment(target, data_root, store)
    if problem:
        record("the turn ran through the printed invocation and exited 0", False, problem)
        return
    session_id = target.get("session_id", "")
    before, before_words, before_status = export_document(opencode, data_root, session_id, cwd=workspace)
    if before is None:
        record("the installed session reads back before the turn", False,
               f"exit {before_status}; {bounded(before_words, 200)}")
        return
    turn = deadline_run([*arguments[:-1], args.resume_prompt], cwd=workspace, env=environment,
                        timeout=TURN_TIMEOUT, replace_env=True)
    record("the turn ran through the printed invocation and exited 0", turn.returncode == 0,
           f"exit {turn.returncode}; stderr {bounded(turn.stderr, 300)}")
    print_answer(turn.stdout)
    if args.expect:
        record(f"the turn's answer contains {args.expect!r}", args.expect in (turn.stdout or ""),
               bounded(turn.stdout, 200))
    after, after_words, after_status = export_document(opencode, data_root, session_id, cwd=workspace)
    if after is None:
        record("the session's own reader reads it back after the turn", False,
               f"exit {after_status}; {bounded(after_words, 200)}")
        return
    before_messages = before.get("messages") or []
    after_messages = after.get("messages") or []
    record("the turn grew the installed session, through the harness's own reader",
           len(after_messages) > len(before_messages),
           f"{len(before_messages)} message(s) before the turn, {len(after_messages)} after")
    record("the session's own records were not rewritten by the turn",
           after_messages[:len(before_messages)] == before_messages,
           f"the first {len(before_messages)} message(s) are unchanged")
    appended = after_messages[len(before_messages):]
    # What the harness printed is the answer the caller saw, and what the turn appended is what a later
    # read of the session shows. `--expect` makes that text the needle when it was given; without it the
    # needle is the harness's own printed answer, so a turn that answered and appended its records is
    # verified rather than recorded as a failure for the flag the caller did not pass.
    answer = (turn.stdout or "").strip()
    needle = args.expect or answer
    appended_answers = "\n".join(document_texts({"messages": appended}, "assistant"))
    record("the turn's own answer is in the session the harness reads",
           bool(needle) and needle in appended_answers,
           f"the turn appended {len(appended)} message(s); the appended assistant record(s) "
           + ("hold" if needle and needle in appended_answers else "do not hold")
           + f" {needle[:160]!r}")


def finish(args: argparse.Namespace, scratch: Path, document: dict, session_id: str) -> int:
    """The run's own end: the report, then the verdict computed from the one list every check recorded
    into. A failure is FAIL (exit 1); a run with no failures is PASS (exit 0). The absent harness is
    INCOMPLETE (exit 2), decided before any check runs, because this route's installation is the harness's
    own importer: there is no offline half to verify without it."""
    report(args, scratch, document, {"target_provider": TARGET_PROVIDER} if session_id else None)
    failures = [name for name, state, _ in RESULTS if state == "FAIL"]
    if failures:
        print(f"RESULT: FAIL — {len(failures)} check(s) failed: {', '.join(failures)}")
        return 1
    print(f"RESULT: PASS — {len(RESULTS)} check(s), none failed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
