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
   by the harness, which is this route's install rule as much as its check);
4. converts through the CLI (``ccc continue … --json``) and reads the conversion, the installation and the
   structural read-back out of the report rather than trusting them;
5. asks the harness's **own reader** about the installed session (``opencode session export <id>``) and
   compares its answer with the report and the source's own evidence -- the source's first prompt, the tool
   call with its recorded result, the compaction boundary -- with an unknown-id control arm: the same
   command must refuse, naming the id it could not find;
6. asks the harness's own project-scoped listing (``opencode session list --format json``) **from the
   session's own recorded directory**, where it must name the installed session, and from a directory that
   is not that one, where it must not: a session the harness does not resolve from where the report says to
   resume it is a session a caller cannot resume;
7. with ``--resume-prompt``, runs one real turn in the installed session through the **printed** invocation
   (``opencode run --standalone --session <id> -- <prompt>`` with the report's own ``resume_environment``
   and working directory), prints the harness's own answer under
   ``native_check_common.TURN_OUTPUT_MARKER``, and measures the growth through the harness's own reader;
8. re-digests the source tree and re-reads the planted session's document, so "nothing else moved" is
   measured after the run as well as before it, and removes the planted session through the harness's own
   ``session delete``.

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

#: The environment variable that names the source root for the conversion's reader, and the one the
#: harness reads its data root from (mirroring `internal/ccc/opencode_install.go`'s `openCodeDataRootEnv`).
#: The harness has no flag for its data directory, so it is set as an environment value -- which is also
#: what the printed resume line does.
SOURCE_ROOT_ENV = {"claude": "CLAUDE_PROJECTS_DIR"}
OPENCODE_DATA_ROOT_ENV = "XDG_DATA_HOME"

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

#: The bound on one harness invocation: a local command that answers in about a second, given a deadline of
#: its own so a harness that hangs is a failed check rather than a hung run.
HARNESS_TIMEOUT = 300

#: The bound on one real resume turn: a model call in the caller's own session.
TURN_TIMEOUT = 600


def opencode_environment(data_root: Path) -> dict[str, str]:
    """The environment one OpenCode command this check runs is given: this process's own, with the data
    root named and the database override removed (it would point the harness at another store -- the same
    rule ``internal/ccc/opencode_install.go``'s own environment applies)."""
    environment = dict(os.environ)
    environment[OPENCODE_DATA_ROOT_ENV] = str(data_root)
    environment.pop("OPENCODE_DB", None)
    return environment


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
    replace, so a case can answer the harness's own protocol without the harness installed."""
    return run([opencode, *opencode_arguments(verb)], cwd=cwd, env=opencode_environment(data_root),
               timeout=timeout)


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


def plant_unrelated(opencode: str, data_root: Path, scratch: Path) -> tuple[bool, str]:
    """Plant the unrelated session **through the harness's own importer**, which is the only way a store is
    written here: the document is staged in the run's scratch tree and handed over exactly as the route's
    own installer hands a converted document over."""
    staged = scratch / "unrelated-import.json"
    staged.write_text(json.dumps(unrelated_document()), encoding="utf-8")
    completed = ask_opencode(opencode, ["session", "import", str(staged), "--directory",
                                        str(UNRELATED_DIRECTORY)], data_root)
    answer = (completed.stdout or "") + (completed.stderr or "")
    ok = completed.returncode == 0 and f"Imported session: {UNRELATED_SESSION}" in answer
    return ok, bounded(answer, 300)


def remove_planted(opencode: str, data_root: Path, planted: bool) -> None:
    """Remove the planted session through the harness's own `session delete`, and show that it is gone.

    This runs on every path that planted one -- after the last check, and before the report removes the
    scratch tree the store lives in (measured: a delete after the store's directory is gone resolves a
    fresh empty store and answers "Session not found", which is not a removal) -- so a caller-named
    ``--target-home`` is left as it was found apart from the converted session this run installed. The
    removal is a check of its own: a fabricated session this harness leaves behind is a session the
    caller's own listing would show."""
    if not planted:
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


def document_tool_evidence(document: dict) -> list[tuple[str, str, str]]:
    """Every tool part a document holds, as `(call id, name, status)`: the evidence a read of the session
    shows for a call and its recorded result."""
    evidence: list[tuple[str, str, str]] = []
    for message in document.get("messages") or []:
        for part in message.get("content") or []:
            if not isinstance(part, dict) or part.get("type") != "tool":
                continue
            state = part.get("state") or {}
            evidence.append((str(part.get("id", "")), str(part.get("name", "")),
                             str(state.get("status", ""))))
    return evidence


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
                        help="the harness's data root (default: a scratch one). A caller's own root is used "
                             "exactly as it is found: nothing is installed into it but the converted "
                             "session, and the planted session this run adds is removed again")
    parser.add_argument("--resume-prompt",
                        help="run one real turn in the installed session with this prompt")
    parser.add_argument("--expect", help="require the resume turn's answer to contain this text")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory and say where it is")
    parser.add_argument("--json", action="store_true", help="also print a machine-readable summary")
    args = parser.parse_args(argv)

    repo = args.repo.resolve()
    fixture_root = repo / FIXTURE_ROOT
    opencode = shutil.which(args.opencode)
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
    data_root = args.target_home.resolve() if args.target_home else scratch / "opencode-data"
    data_root.mkdir(parents=True, exist_ok=True)
    store = data_root / OPENCODE_DATA_DIR / OPENCODE_STORE

    version = run([opencode, "--version"]).stdout.strip()
    print(f"repo:        {repo}")
    print(f"direction:   {SOURCE_PROVIDER}:{source_session} → {TARGET_PROVIDER}")
    print(f"target:      {args.opencode} ({opencode}; {version[:60] or 'no version line'})")
    print(f"source:      {source.relative_to(repo)} (copied to {destination})")
    print(f"workspace:   {workspace}")
    print(f"data root:   {data_root}"
          + ("" if named_home else " (a scratch one this run made)")
          + f"; the harness's own data directory is {data_root / OPENCODE_DATA_DIR}, its store {store.name}")
    print(f"the source fixture's recorded working directory ({recorded_cwd or 'none recorded'}) is rewritten "
          f"to the workspace: the harness's own importer refuses a session whose directory does not exist "
          f"(measured: LocationNotFoundError)")
    print()

    parsed: dict = {}
    session_id = ""

    def stop() -> int:
        """End the run on whatever state it reached: the planted session is removed through the harness's
        own verb while the store still exists, and the report -- which removes the scratch tree -- comes
        after."""
        remove_planted(opencode, data_root, planted)
        return finish(args, scratch, parsed, session_id)

    source_root_before = tree_digest(source_root)
    planted, planted_detail = plant_unrelated(opencode, data_root, scratch)
    record("the unrelated session is planted through the harness's own importer", planted, planted_detail)
    if not planted:
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
    build = run([go, "build", f"-tags={GO_TAGS}", "-o", str(repo / BINARY), "./cmd/ccc"], cwd=repo)
    if build.returncode != 0:
        record("ccc builds with the flags upstream ships", False, build.stderr.strip()[:400])
        return stop()
    record("ccc builds with the flags upstream ships", True)

    command = [str(repo / BINARY), "continue", f"{SOURCE_PROVIDER}:{source_session}", "--to",
               TARGET_PROVIDER, "--target-home", str(data_root), "--target-binary", opencode, "--json"]
    if args.resume_prompt:
        command += ["--verify", "resume", "--resume-prompt", args.resume_prompt]
    convert = run(command, cwd=repo, env={SOURCE_ROOT_ENV[SOURCE_PROVIDER]: str(source_root)})
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
    record("the report names the harness's own version and the model a new session of it would run "
           "under",
           bool(target.get("cli_version")) and bool(target.get("model"))
           and bool(target.get("model_provider")),
           f"cli_version {target.get('cli_version')} ({target.get('cli_version_source')}), model "
           f"{target.get('model_provider')}/{target.get('model')} "
           f"({target.get('model_provider_source')})")

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
        # A requested turn (--resume-prompt) runs inside the conversion too: the CLI's own resume check
        # drives one real turn, which appends records to the session it opens. So the store holds the
        # conversion's own messages *and* whatever that turn appended -- never fewer, and exactly the
        # conversion's own when no turn was asked for.
        record("the harness's own reader holds every message the conversion wrote",
               bool(expected) and stored >= expected and (bool(args.resume_prompt) or stored == expected),
               f"the store's own document holds {stored} message(s); the conversion carried {expected}"
               + (", and the conversion's own resume check ran one turn in it" if args.resume_prompt else ""))
        user_texts = document_texts(exported, "user")
        record("the installed session holds the source's own first prompt",
               bool(user_texts) and user_texts[0].strip().startswith("Summarize the retry policy"),
               f"{user_texts[0][:80]!r}" if user_texts else "no user text")
        evidence = document_tool_evidence(exported)
        record("the installed session holds the source's tool call and its recorded result",
               evidence == [("toolu_01Example0000000000000000", "Read", "completed")], f"{evidence}")
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

    # The printed invocation, read the way a POSIX shell reads it, and then -- when a turn was asked
    # for -- run in its own words.
    _check_invocation(target, workspace, data_root)
    if args.resume_prompt:
        _run_turn(args, target, opencode, data_root, workspace)
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


def _check_invocation(target: dict, workspace: Path, data_root: Path) -> None:
    """Check the printed resume commands by reading them the way a POSIX shell reads them: the working
    directory the harness is started from and the data root it reads, both named, against the argument
    vector the report itself printed."""
    for field, arguments_field in (("resume_command", "resume_arguments"),
                                   ("exec_resume_command", "exec_resume_arguments")):
        command = target.get(field, "")
        arguments = target.get(arguments_field) or []
        ok = (bool(command) and bool(arguments)
              and _shared_reported_invocation(command, workspace, data_root, OPENCODE_DATA_ROOT_ENV,
                                              arguments))
        record(f"the printed {field} reads as a POSIX shell line naming the session", ok,
               bounded(command, 220))


def _run_turn(args: argparse.Namespace, target: dict, opencode: str, data_root: Path,
              workspace: Path) -> None:
    """Run one real turn in the installed session through the printed invocation's own words.

    The turn is what proves the session is one a caller can resume: the harness answers from the installed
    history and appends its own records to it. The growth is measured through the harness's own reader,
    before and after, and the answer is printed under the shared marker -- an answer from a session the
    harness's own reader does not read differently is not a resumed session."""
    session_id = target.get("session_id", "")
    before, before_words, before_status = export_document(opencode, data_root, session_id, cwd=workspace)
    if before is None:
        record("the installed session reads back before the turn", False,
               f"exit {before_status}; {bounded(before_words, 200)}")
        return
    turn = run([opencode, *opencode_arguments(["run", "--session", session_id, "--", args.resume_prompt])],
               cwd=workspace, env=opencode_environment(data_root), timeout=TURN_TIMEOUT)
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
    record("the turn's own answer is in the session the harness reads",
           bool(args.expect) and any(args.expect in json.dumps(message) for message in appended),
           f"the turn appended {len(appended)} message(s)")


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
