#!/usr/bin/env python3
"""call/ccc project gate — one command, real exit codes, no network.

Steps, in order: gofmt, build, vet, test, selftest, parity, secrets.
Exit 0 only when every step that RAN passed. A step that could not run prints
SKIP/BOUNDARY with its reason, a step whose hook is not in the tree prints N/A,
and none of the three is counted as a pass: the verdict line names every one of
them, so a run that never checked something cannot read as a full ``GATE: PASS``
-- it says ``GATE: PASS (partial: ...)``. A step named in
scripts/gate-required.txt must be PASS: any other status -- FAIL, SKIP, BOUNDARY
or N/A -- is rewritten to FAIL before the verdict, so it exits 1.

Knobs (in-repo, auditable):
  scripts/gate-targets.txt    Go package patterns for build/vet/test; absent = ./...
  scripts/gate-goflags.txt    extra Go flags for build/vet/test (e.g. -tags fts5)
  scripts/upstream-manifest.txt  the carry manifest, which also says which files the secrets step
                              scans (this repository's own; the carried tree is the parity step's)
  scripts/gate-required.txt   steps that MUST be PASS; a listed step that is
                              skipped, unavailable or a platform boundary fails
  scripts/upstream-parity.py  source-side parity check, run when present
  scripts/tests/              stdlib-unittest regression tests for this gate,
                              run by the selftest step
  CALLCCC_GATE_ROOT           run against another tree (used by
                              scripts/tests/test_gate.py); defaults to this repo

The platform boundary and the reason behind each status are stated in
docs/harness.md; this file implements them.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import ManifestError, load_manifest  # noqa: E402  (path set up above)

ROOT = Path(os.environ.get("CALLCCC_GATE_ROOT") or Path(__file__).resolve().parent.parent).resolve()
PASS, FAIL, SKIP, BOUNDARY, NA = "PASS", "FAIL", "SKIP", "BOUNDARY", "N/A"
KNOWN_STEPS = ("gofmt", "build", "vet", "test", "selftest", "parity", "secrets")


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


def run(cmd: list[str], env: dict[str, str] | None = None, timeout: int = 1800):
    merged = os.environ.copy()
    merged.update(env or {})
    try:
        return subprocess.run(
            cmd, cwd=str(ROOT), env=merged, capture_output=True, text=True, timeout=timeout
        )
    except FileNotFoundError as exc:
        # A missing binary is a step result, not a crash: report it as rc=127.
        return subprocess.CompletedProcess(cmd, 127, "", f"{exc}\n")


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


def step_gofmt(go_files: list[str]) -> tuple[str, str]:
    if not shutil.which("gofmt"):
        return FAIL, "gofmt not found (Go toolchain is required)"
    if not go_files:
        return SKIP, "no Go sources in the tree yet"
    proc = run(["gofmt", "-l", *go_files])
    if proc.returncode != 0:
        return FAIL, tail(proc.stderr)
    unformatted = [line for line in proc.stdout.splitlines() if line.strip()]
    if unformatted:
        return FAIL, "not gofmt-clean:\n" + "\n".join(unformatted[:40])
    return PASS, f"{len(go_files)} file(s) gofmt-clean"


def step_go(
    sub: str, go_files: list[str], boundary: str | None, targets: list[str], flags: list[str]
) -> tuple[str, str]:
    name = {"build": "go build", "vet": "go vet", "test": "go test -count=1"}[sub]
    if not (ROOT / "go.mod").exists():
        return SKIP, "no go.mod yet"
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
    proc = run(cmd, env=env)
    detail = tail((proc.stdout or "") + ("\n" + proc.stderr if proc.stderr else ""))
    if proc.returncode != 0:
        return FAIL, f"{name} {' '.join(targets)} -> rc={proc.returncode}\n{detail}"
    if sub == "test":
        passed = sum(1 for line in (proc.stdout or "").splitlines() if line.startswith("ok "))
        return PASS, f"{passed} package(s) ok"
    return PASS, f"{name} {' '.join(targets)} -> rc=0"


def step_selftest() -> tuple[str, str]:
    """The harness's own tests: a broken gate is a defect like any other."""
    tests = ROOT / "scripts" / "tests"
    if not tests.is_dir() or not any(tests.rglob("test_*.py")):
        return SKIP, "no scripts/tests/test_*.py in this tree yet"
    proc = run([sys.executable, "-m", "unittest", "discover", "-s", "scripts/tests", "-p", "test_*.py"],
               env={"PYTHONDONTWRITEBYTECODE": "1"}, timeout=900)
    detail = tail((proc.stdout or "") + (proc.stderr or ""), 1200)
    if proc.returncode != 0:
        return FAIL, f"scripts/tests -> rc={proc.returncode}\n{detail}"
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
        owned = manifest.owned
    except (OSError, ManifestError) as broken:
        # A tree without a carry manifest is not this repository's shape (a throw-away test tree, a
        # copy): the honest fallback is to scan the whole tree and say so.
        return ROOT, -1, f"no readable carry manifest ({broken}), so the whole tree was scanned"
    staged = Path(tempfile.mkdtemp(prefix="ccc-secrets-"))
    copied = 0
    for path in ROOT.rglob("*"):
        if not path.is_file() or ".git" in path.relative_to(ROOT).parts:
            continue
        relative = path.relative_to(ROOT).as_posix()
        if not any(relative == prefix or (prefix.endswith("/") and relative.startswith(prefix))
                   for prefix in owned):
            continue
        target = staged / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, target)
        copied += 1
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
        return FAIL, note
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


def build_steps() -> list[tuple[str, str, str]]:
    go_files = tracked_go_files()
    targets = go_targets()
    flags = go_flags()
    _, boundary = go_env()
    steps: list[tuple[str, str, str]] = []
    steps.append(("gofmt", *step_gofmt(go_files)))
    for sub in ("build", "vet", "test"):
        steps.append((sub, *step_go(sub, go_files, boundary, targets, flags)))
    steps.append(("selftest", *step_selftest()))
    steps.append(("parity", *step_parity()))
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
    print(f"gate: go={shutil.which('go') or 'missing'} targets={' '.join(go_targets())}")
    for name, status, detail in steps:
        print(f"  {status:<8} {name:<8} {detail}")

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
