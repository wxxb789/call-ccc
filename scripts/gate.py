#!/usr/bin/env python3
"""call/ccc project gate — one command, real exit codes, no network for its required steps (the
disclosed ``portability`` step fetches the pin).

Steps, in order: gofmt, build, vet, test, portability, selftest, parity, citations, secrets.
Exit 0 only when every step that RAN passed. A step that could not run prints
SKIP/BOUNDARY with its reason, a step whose hook is not in the tree prints N/A,
and none of the three is counted as a pass: the verdict line names every one of
them, so a run that never checked something cannot read as a full ``GATE: PASS``
-- it says ``GATE: PASS (partial: ...)``. A step named in
scripts/gate-required.txt must be PASS: any other status -- FAIL, SKIP, BOUNDARY
or N/A -- is rewritten to FAIL before the verdict, so it exits 1.

The Go steps are handed a *module directory*: this tree's own Go module, whose
directory is read from the tree's own ``go.mod`` files (``upstream_manifest``'s
``detect_layout``), so the same targets mean the same packages in either layout
this repository has had -- the module at the root beside the carry, or the carry
in ``agentsview/`` beside the module in ``ccc/``. ``gofmt`` stays one step over
every tracked ``*.go`` file: formatting has no module boundary, and splitting it
would only narrow the check. The ``portability`` step is the one that uses the
network (``scripts/portability.py``: build and test this module against a
pristine checkout of the pin); it is not required, and a host that cannot fetch
one is disclosed in the verdict line rather than passed.

Knobs (in-repo, auditable):
  scripts/gate-targets.txt    Go package patterns for build/vet/test; absent = ./...
  scripts/gate-goflags.txt    extra Go flags for build/vet/test (e.g. -tags fts5)
  scripts/upstream-manifest.txt  the carry manifest, which also says which files the secrets step
                              scans (this repository's own; the carried tree is the parity step's)
  scripts/gate-required.txt   steps that MUST be PASS; a listed step that is
                              skipped, unavailable or a platform boundary fails
  scripts/upstream-parity.py  source-side parity check, run when present
  scripts/portability.py      this module against a pristine pinned carry, run when present
  scripts/obligation-citations.py  the obligation manifests under docs/evidence/, run when present
  scripts/tests/              stdlib-unittest regression tests for this gate,
                              run by the selftest step
  CALLCCC_GATE_ROOT           run against another tree (used by
                              scripts/tests/test_gate.py); defaults to this repo
  CCC_UPSTREAM_DIR / CCC_UPSTREAM_CACHE  where the portability step takes its pinned checkout from
                              (read by scripts/portability.py; see docs/harness.md)

The platform boundary and the reason behind each status are stated in
docs/harness.md; this file implements them.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import (  # noqa: E402  (path set up above)
    ManifestError,
    detect_layout,
    load_manifest,
)

ROOT = Path(os.environ.get("CALLCCC_GATE_ROOT") or Path(__file__).resolve().parent.parent).resolve()
PASS, FAIL, SKIP, BOUNDARY, NA = "PASS", "FAIL", "SKIP", "BOUNDARY", "N/A"
KNOWN_STEPS = ("gofmt", "build", "vet", "test", "portability", "selftest", "parity", "citations",
               "secrets")

#: How many characters of ``*.go`` paths one ``gofmt`` invocation may carry. Windows caps a process
#: command line at 32 767 characters, and the carried tree's file list is far past that, so the step
#: runs bounded batches that stay well inside the cap on every platform -- see ``gofmt_batches``.
GOFMT_BATCH_CHARS = 8000

#: The citations step's budget. The checker bounds its own whole run (``TOTAL_TIMEOUT`` in
#: ``scripts/obligation-citations.py``: two per-command deadlines, 3 600 s) and answers a bound it
#: reaches as a FAIL verdict, so the step is handed that bound plus a margin -- a bound the checker can
#: reach is one it still has time to answer on, and this budget is only the backstop for a checker that
#: could not report at all (its ``TimeoutExpired`` is caught and answered as FAIL, below).
CITATIONS_TIMEOUT = 3600 + 300

#: The portability step's budget. The checker fetches a pinned checkout when it has none cached
#: (network) and then builds and tests this module against it, which on a cold runner is minutes; this
#: is the backstop for a run that ends without a verdict, answered as FAIL below -- a bound the checker
#: reaches is not a pass either.
PORTABILITY_TIMEOUT = 3600


def read_list(name: str) -> list[str]:
    path = ROOT / "scripts" / name
    if not path.exists():
        return []
    items = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.split("#", 1)[0].strip()
        if line:
            items.append(line)
    return items


def run(cmd: list[str], env: dict[str, str] | None = None, timeout: int = 1800,
        cwd: Path | None = None):
    merged = os.environ.copy()
    merged.update(env or {})
    try:
        return subprocess.run(
            cmd, cwd=str(cwd or ROOT), env=merged, capture_output=True, text=True, timeout=timeout
        )
    except OSError as exc:
        # A missing binary is a step result, not a crash: report it as rc=127. Every other OSError --
        # a command line the platform refuses (Windows caps one at 32 767 characters), a binary it
        # cannot execute -- is the same kind of answer, so a step reports it instead of the gate
        # ending in a traceback with no verdict at all.
        #
        # The command is named here rather than left to the platform's message, because that message
        # does not always hold it: Windows' ``FileNotFoundError`` comes from ``CreateProcess`` and
        # carries a WinError with no filename ("[WinError 2] The system cannot find the file
        # specified"), so a step that could not start would otherwise say neither which tool was
        # missing nor what it tried to run. ``cmd[0]`` and not the whole argv: the gofmt step hands
        # this hundreds of paths, and the step's own line already names the rest.
        return subprocess.CompletedProcess(
            cmd, 127 if isinstance(exc, FileNotFoundError) else 126, "", f"{cmd[0]}: {exc}\n"
        )


def tail(text: str, limit: int = 2500) -> str:
    text = (text or "").strip()
    return text if len(text) <= limit else "…\n" + text[-limit:]


def tracked_go_files() -> list[str]:
    """The ``*.go`` files gofmt looks at: the tracked ones, minus a top-level ``vendor/``.

    Both branches answer in one spelling: POSIX-form relative paths
    (``deep/vendor/nested.go``), the form ``git ls-files`` prints on every
    platform. The walk below normalises with ``as_posix()`` instead of letting
    ``str()`` drift with the host separator, so the list handed to ``gofmt`` --
    and the list a test can pin -- is the same one on Windows as here.

    A tree without git (a ``CALLCCC_GATE_ROOT`` copy, a throw-away test tree) has
    no notion of "tracked", so the walk below answers with everything it can see
    while dropping the same two things the git branch drops: ``.git/`` and a
    top-level ``vendor/``. docs/harness.md states both halves of the contract.
    """
    proc = run(["git", "ls-files", "*.go"])
    if proc.returncode != 0:
        files = []
        for path in ROOT.rglob("*.go"):
            relative = path.relative_to(ROOT)
            if ".git" not in relative.parts and relative.parts[0] != "vendor":
                files.append(relative.as_posix())
        return sorted(files)
    return [line for line in proc.stdout.splitlines() if line and not line.startswith("vendor/")]


def go_targets() -> list[str]:
    return read_list("gate-targets.txt") or ["./..."]


def module_dir() -> Path:
    """The directory of this repository's own Go module, as this tree's ``go.mod`` files state it.

    The layout is read off the tree (``upstream_manifest.detect_layout``) rather than configured: in
    the layout this repository had first, that module is the repository root and the carried tree sits
    beside our packages; in the layout it moved to, the carried tree has a directory of its own
    (``agentsview/``) and this module is ``ccc/`` with its own ``go.mod``. A throw-away tree with one
    ``go.mod`` at its root answers the root layout, which is what the gate's own cases build.
    """
    return ROOT / detect_layout(ROOT).module


def module_spelling(directory: Path) -> str:
    """``directory`` relative to the tree, or ``.`` when it *is* the tree."""
    if directory == ROOT:
        return "."
    return directory.relative_to(ROOT).as_posix()


def go_flags() -> list[str]:
    """Extra Go flags from ``scripts/gate-goflags.txt``, folded into build/vet/test.

    The inherited AgentsView build passes ``-tags fts5`` on every Go command it ships, and the flag is
    not cosmetic: without it the SQLite driver has no FTS5 module, so a binary built here could not
    load an archive that has a full-text index. The knob stays in the tree rather than in a shell
    history so the flags a gate run used are auditable next to the targets it used.
    """
    flags: list[str] = []
    for line in read_list("gate-goflags.txt"):
        flags.extend(line.split())
    return flags


def go_env() -> tuple[dict[str, str], str | None]:
    """Return (environment, boundary_reason). boundary_reason means 'cannot build here'."""
    env = {"GOFLAGS": "-mod=mod", "CGO_ENABLED": "1"}
    if sys.platform.startswith("win") and not shutil.which("gcc"):
        return env, "no C compiler on PATH (CGO/SQLite steps cannot run on this host)"
    return env, None


def gofmt_batches(files: list[str], limit: int = GOFMT_BATCH_CHARS) -> list[list[str]]:
    """Split the ``*.go`` list into invocations that fit a process command line.

    Windows caps a process command line at 32 767 characters, and the carried AgentsView tree holds
    thousands of ``*.go`` files, so one ``gofmt -l <every path>`` invocation cannot be started there at
    all. The batches together are still the whole list, in the same order, and a path longer than the
    limit is a batch of its own rather than a dropped one.
    """
    batches: list[list[str]] = []
    batch: list[str] = []
    used = 0
    for path in files:
        cost = len(path) + 1  # the separator the command line spends between arguments
        if batch and used + cost > limit:
            batches.append(batch)
            batch, used = [], 0
        batch.append(path)
        used += cost
    if batch:
        batches.append(batch)
    return batches


def step_gofmt(go_files: list[str]) -> tuple[str, str]:
    if not shutil.which("gofmt"):
        return FAIL, "gofmt not found (Go toolchain is required)"
    if not go_files:
        return SKIP, "no Go sources in the tree yet"
    batches = gofmt_batches(go_files)
    unformatted: list[str] = []
    failed: list[str] = []
    for batch in batches:
        proc = run(["gofmt", "-l", *batch])
        if proc.returncode != 0:
            failed.append(tail(proc.stderr, 1200))
            continue
        unformatted.extend(line for line in proc.stdout.splitlines() if line.strip())
    if failed:
        # One line, so the tool's own message reaches the step's printed line (and any assertion that
        # reads it) instead of only the detail's second line.
        return FAIL, (f"gofmt could not check {len(failed)} of {len(batches)} batch(es): "
                      + " | ".join(failed))
    if unformatted:
        more = f"\n… and {len(unformatted) - 40} more" if len(unformatted) > 40 else ""
        return FAIL, "not gofmt-clean:\n" + "\n".join(unformatted[:40]) + more
    return PASS, f"{len(go_files)} file(s) gofmt-clean"


def step_go(
    sub: str, go_files: list[str], boundary: str | None, targets: list[str], flags: list[str],
    module: Path,
) -> tuple[str, str]:
    name = {"build": "go build", "vet": "go vet", "test": "go test -count=1"}[sub]
    if not (module / "go.mod").exists():
        return SKIP, f"no go.mod in {module_spelling(module)} yet"
    if not go_files:
        return SKIP, "no Go sources in the tree yet"
    if boundary:
        return BOUNDARY, boundary
    if not shutil.which("go"):
        return FAIL, "go not found on PATH"
    env, _ = go_env()
    if sub == "test":
        cmd = ["go", "test", "-count=1", *flags, *targets]
    else:
        cmd = ["go", sub, *flags, *targets]
    proc = run(cmd, env=env, cwd=module)
    detail = tail((proc.stdout or "") + ("\n" + proc.stderr if proc.stderr else ""))
    # The command is run *in the module*, so a failure's paths are the module's own; the detail names
    # which module answered (the step's name cannot: the step names the tool, not the tree).
    where = "" if module == ROOT else f" (in {module_spelling(module)})"
    if proc.returncode != 0:
        return FAIL, f"{name} {' '.join(targets)}{where} -> rc={proc.returncode}\n{detail}"
    if sub == "test":
        passed = sum(1 for line in (proc.stdout or "").splitlines() if line.startswith("ok "))
        return PASS, f"{passed} package(s) ok{where}"
    return PASS, f"{name} {' '.join(targets)}{where} -> rc=0"


def failing_tests(output: str) -> list[str]:
    """Every test the run named as a failure or an error, in the order it printed them.

    ``unittest`` prints one ``FAIL: <test> (<module>.<class>.<test>)`` or
    ``ERROR: <test> (<module>.<class>.<test>)`` line per failing test, before that test's traceback;
    its summary line only counts them. A step's detail is a *tail* of the run (``step_selftest``),
    so on a run with several failures the names scroll out of it -- and a CI job's log is all a
    reader has left when the run is over, where GitHub's runner collapses the captions into a single
    annotation. The names are therefore read out of the whole output, whole, and printed beside the
    tail: a list short of its failures is a run nobody can enumerate.

    A line is a test only when it carries the runner's own case-line shape -- the
    ``(<module>.<class>.<test>)`` qualifier in parentheses after the name -- because the run's
    output also holds what its cases printed, and the shipped checks print ``FAIL: <name> — <detail>``
    records of their own during ordinary case execution: the prefix alone named those
    records as failing tests (measured on this suite's green run at this rule's fix: 40 of them).
    """
    named = []
    for line in output.splitlines():
        if not line.startswith(("FAIL: ", "ERROR: ")):
            continue
        _, sep, rest = line.partition(" (")
        qualifier, close, _ = rest.partition(")")
        if sep and close and "." in qualifier:
            named.append(line.strip())
    return named


def step_selftest() -> tuple[str, str]:
    """The harness's own tests: a broken gate is a defect like any other.

    A failing run's detail names every test it failed, then the tail of the run's own output: the
    names are what lets the next reader -- of a CI log as much as of a terminal -- enumerate the
    failures without a second run (``failing_tests``).
    """
    tests = ROOT / "scripts" / "tests"
    if not tests.is_dir() or not any(tests.rglob("test_*.py")):
        return SKIP, "no scripts/tests/test_*.py in this tree yet"
    proc = run([sys.executable, "-m", "unittest", "discover", "-s", "scripts/tests", "-p", "test_*.py"],
               env={"PYTHONDONTWRITEBYTECODE": "1"}, timeout=900)
    output = (proc.stdout or "") + (proc.stderr or "")
    detail = tail(output, 1200)
    if proc.returncode != 0:
        named = failing_tests(output)
        listing = (f"\n{len(named)} failing test(s):\n" + "\n".join(named)) if named else ""
        return FAIL, f"scripts/tests -> rc={proc.returncode}{listing}\n{detail}"
    ran = [line for line in (proc.stderr or "").splitlines() if line.startswith("Ran ")]
    return PASS, (ran[-1] if ran else "ok") + f" (rc={proc.returncode})"


def step_parity() -> tuple[str, str]:
    script = ROOT / "scripts" / "upstream-parity.py"
    if not script.exists():
        return NA, "no scripts/upstream-parity.py in this tree yet"
    proc = run([sys.executable, str(script)])
    if proc.returncode != 0:
        return FAIL, f"upstream-parity.py -> rc={proc.returncode}\n{tail((proc.stdout or '') + (proc.stderr or ''))}"
    return PASS, tail(proc.stdout, 800).replace("\n", " | ") or "rc=0"


def step_citations() -> tuple[str, str]:
    """The obligation manifests: every cited test runs and passes.

    ``scripts/obligation-citations.py`` answers the verdict in this gate's own vocabulary as the last
    line it prints, so the status is read from that line rather than from a second spelling of the
    checker's exit codes; the exit code is still cross-checked, because a run that claims PASS and
    exits non-zero is a defect either way. The step is required (``scripts/gate-required.txt``): the
    manifests are an increment's acceptance surface, so a missing manifest or a stale citation fails
    the gate instead of passing partially.

    The step's own budget (``CITATIONS_TIMEOUT``) sits past the total bound the checker puts on its
    whole run, so a bound the checker reaches is answered by the checker's own FAIL verdict first; this
    arm is the backstop for a checker that ends without a verdict anyway. ``run()`` has already killed
    and reaped the child by the time ``TimeoutExpired`` is raised, and a step that ended without a
    verdict is a failure -- not a traceback that leaves the gate with no verdict at all.
    """
    script = ROOT / "scripts" / "obligation-citations.py"
    if not script.exists():
        return NA, "no scripts/obligation-citations.py in this tree yet"
    try:
        proc = run([sys.executable, str(script)], timeout=CITATIONS_TIMEOUT)
    except subprocess.TimeoutExpired as expired:
        out = expired.stdout if isinstance(expired.stdout, str) else ""
        return FAIL, (
            f"obligation-citations.py -> no verdict within {CITATIONS_TIMEOUT} s: the checker's own "
            f"total bound fires first, so this backstop budget was reached by a run that could not "
            f"report at all\n" + tail(out, 400)
        )
    text = (proc.stdout or "") + (proc.stderr or "")
    verdicts = [
        line for line in (proc.stdout or "").splitlines()
        if re.match(r"^citations: (PASS|FAIL|SKIP|BOUNDARY|N/A)(?: — .*)?$", line)
    ]
    if len(verdicts) != 1:
        return FAIL, (
            f"obligation-citations.py -> rc={proc.returncode}, {len(verdicts)} verdict line(s)\n"
            + tail(text, 1500)
        )
    status = verdicts[0].split(":", 1)[1].split(" — ", 1)[0].strip()
    if status == PASS and proc.returncode != 0:
        return FAIL, f"obligation-citations.py answered PASS with rc={proc.returncode}\n" + tail(text, 1500)
    if status == PASS:
        lines = [line for line in (proc.stdout or "").splitlines() if not line.startswith("citations: ")]
        return PASS, " | ".join(lines) or "rc=0"
    if status in (SKIP, BOUNDARY):
        return status, verdicts[0].split(" — ", 1)[-1]
    return FAIL, tail(text, 1500)


def secrets_scope() -> tuple[Path | None, int, str]:
    """Stage this repository's own files for the leak scan and describe the scope.

    The carried AgentsView tree is not scanned here: it is upstream's content, byte-verified against
    the pinned revision by the parity step, and a credential shape inside upstream's own
    secret-detector corpora (``internal/secrets/rules_test.go`` and friends) is upstream's test data,
    not a credential call/ccc committed. What this step is for is the other half -- the files this
    repository owns -- and the manifest is what says which those are, so a new project file is scanned
    automatically and a carried one cannot be mistaken for ours.

    Returns ``(directory, file count, note)``. ``directory`` is None when the copy could not be made,
    which the caller reports instead of scanning something else.
    """
    manifest_path = ROOT / "scripts" / "upstream-manifest.txt"
    try:
        manifest = load_manifest(manifest_path)
    except (OSError, ManifestError) as broken:
        # A tree without a carry manifest is not this repository's shape (a throw-away test tree, a
        # copy): the honest fallback is to scan the whole tree and say so.
        return ROOT, -1, f"no readable carry manifest ({broken}), so the whole tree was scanned"
    stage_root: str | None = None
    try:
        stage_root = tempfile.mkdtemp(prefix="ccc-secrets-")
        staged = Path(stage_root)
        copied = 0
        for path in ROOT.rglob("*"):
            if not path.is_file() or ".git" in path.relative_to(ROOT).parts:
                continue
            relative = path.relative_to(ROOT).as_posix()
            if not manifest.is_owned(relative):
                continue
            target = staged / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, target)
            copied += 1
    except OSError as failed:
        # Staging needs disk space and permissions the scan itself does not: report the scope that
        # could not be prepared, so the caller names it instead of crashing on a traceback.
        if stage_root is not None:
            shutil.rmtree(stage_root, ignore_errors=True)
        return None, 0, f"could not stage this repository's own files for the leak scan ({failed})"
    return staged, copied, (
        f"{copied} file(s) this repository owns (the carried upstream tree is covered by the parity "
        f"step, not by this scan)")


def step_secrets() -> tuple[str, str]:
    """Scan this repository's own files for credentials.

    ``--no-git`` (no git history, current files only) has no gitignore switch, so a local ``.env`` is
    read here even though CI's clean checkout never has one; the PASS message states the scope
    instead of implying a tracked-tree one.
    """
    if not shutil.which("gitleaks"):
        return SKIP, "gitleaks not installed on this host (CI installs it)"
    scope, copied, note = secrets_scope()
    if scope is None:
        # The scan could not be prepared: like a gitleaks binary that is absent or a run that did not
        # complete, that is a disclosed SKIP named in the verdict line, not a leak result.
        return SKIP, note
    try:
        proc = run(
            ["gitleaks", "detect", "--no-git", "--source", str(scope), "--redact", "--no-banner",
             "--exit-code", "7"],
            timeout=600,
        )
    finally:
        if scope != ROOT:
            shutil.rmtree(scope, ignore_errors=True)
    if proc.returncode == 0:
        return PASS, f"no leaks found in {note}"
    if proc.returncode == 7:
        return FAIL, f"gitleaks reported leaks (redacted) in {note}:\n" + tail(proc.stdout, 1200)
    return SKIP, f"gitleaks did not complete (rc={proc.returncode}): " + tail(proc.stderr, 400)


def step_portability() -> tuple[str, str]:
    """This module, built and tested against a pristine checkout of the pin.

    ``scripts/portability.py`` is the checker (its own docstring states what it does and does not
    prove); it is the one step that needs the network -- to fetch the pinned checkout when the host
    has none cached -- and it is deliberately **not** in ``gate-required.txt``: the gate's subject is
    this tree, and a host that cannot reach the pin would then fail a tree with nothing wrong with it.
    What it must not do is pass quietly: an exit code of 2 is the checker saying it could not run, and
    that is reported as SKIP with the checker's own reason, so the verdict line names it and a run
    that never checked portability cannot read as a full pass. Any other non-zero code is a failure,
    and a run that ends without a verdict at all (the timeout) is one too.
    """
    script = ROOT / "scripts" / "portability.py"
    if not script.exists():
        return NA, "no scripts/portability.py in this tree yet"
    try:
        proc = run([sys.executable, str(script)], timeout=PORTABILITY_TIMEOUT)
    except subprocess.TimeoutExpired as expired:
        out = expired.stdout if isinstance(expired.stdout, str) else ""
        return FAIL, (
            f"portability.py -> no verdict within {PORTABILITY_TIMEOUT} s\n" + tail(out, 400)
        )
    text = (proc.stdout or "") + (proc.stderr or "")
    if proc.returncode == 0:
        return PASS, tail(proc.stdout, 800).replace("\n", " | ") or "rc=0"
    if proc.returncode == 2:
        verdicts = [
            line for line in (proc.stdout or "").splitlines()
            if line.startswith("portability: INCOMPLETE")
        ]
        reason = verdicts[-1] if verdicts else "portability.py could not run here"
        return SKIP, reason + " — " + tail(
            "\n".join(line for line in (proc.stdout or "").splitlines()
                      if line.startswith("  not checked:")), 300)
    return FAIL, tail(text, 1500)


def build_steps() -> list[tuple[str, str, str]]:
    go_files = tracked_go_files()
    targets = go_targets()
    flags = go_flags()
    module = module_dir()
    _, boundary = go_env()
    steps: list[tuple[str, str, str]] = []
    steps.append(("gofmt", *step_gofmt(go_files)))
    for sub in ("build", "vet", "test"):
        steps.append((sub, *step_go(sub, go_files, boundary, targets, flags, module)))
    steps.append(("portability", *step_portability()))
    steps.append(("selftest", *step_selftest()))
    steps.append(("parity", *step_parity()))
    steps.append(("citations", *step_citations()))
    steps.append(("secrets", *step_secrets()))
    return steps


def apply_required(
    steps: list[tuple[str, str, str]], required: set[str]
) -> list[tuple[str, str, str]]:
    """A required step that did not PASS is a failure of the gate, not a footnote.

    Printing an extra FAIL line and then computing the verdict from the original
    statuses is exactly how a mandatory check silently stops being mandatory.
    """
    out: list[tuple[str, str, str]] = []
    for name, status, detail in steps:
        if name in required and status != PASS:
            out.append((name, FAIL, f"REQUIRED but was {status}: {detail}"))
        else:
            out.append((name, status, detail))
    return out


def main() -> int:
    required = set(read_list("gate-required.txt"))
    steps = apply_required(build_steps(), required)
    for name in sorted(required - set(KNOWN_STEPS)):
        steps.append((f"required:{name}", FAIL, f"gate-required.txt names an unknown step: {name}"))

    print(f"gate: {ROOT}")
    print(f"gate: go={shutil.which('go') or 'missing'} module={module_spelling(module_dir())} "
          f"targets={' '.join(go_targets())}")
    for name, status, detail in steps:
        print(f"  {status:<8} {name:<11} {detail}")

    failed = [s[0] for s in steps if s[1] == FAIL]
    if failed:
        print("GATE: FAIL — " + ", ".join(failed))
        return 1
    partial = [s[0] for s in steps if s[1] in (SKIP, BOUNDARY, NA)]
    if partial:
        print("GATE: PASS (partial: " + ", ".join(partial) + " did not run — see reasons above)")
    else:
        print("GATE: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
