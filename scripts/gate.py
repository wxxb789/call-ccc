#!/usr/bin/env python3
"""call/ccc project gate — one command, real exit codes, no network.

Steps, in order: gofmt, build, vet, test, selftest, parity, secrets.
Exit 0 only when every step that RAN passed. A step that could not run prints
SKIP/BOUNDARY with its reason and is never counted as a pass; the verdict line
then says ``GATE: PASS (partial: ...)``. A step named in
scripts/gate-required.txt must be PASS: any other status -- FAIL, SKIP, BOUNDARY
or N/A -- is rewritten to FAIL before the verdict, so it exits 1.

Knobs (in-repo, auditable):
  scripts/gate-targets.txt    Go package patterns for build/vet/test; absent = ./...
  scripts/gate-required.txt   steps that MUST be PASS; a listed step that is
                              skipped, unavailable or a platform boundary fails
  scripts/upstream-parity.py  source-side parity check, run when present
  scripts/tests/              stdlib-unittest regression tests for this gate,
                              run by the selftest step
  CALLCCC_GATE_ROOT           run against another tree (used by
                              scripts/tests/test_gate.py); defaults to this repo

Platform boundary: on Linux/macOS a missing Go toolchain or C compiler is a
FAILURE; on Windows a missing C compiler is a disclosed BOUNDARY (the project's
platforms are "subject to native harness availability"). See docs/harness.md.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
from pathlib import Path

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
    proc = run(["git", "ls-files", "*.go"])
    if proc.returncode != 0:
        return sorted(
            str(p.relative_to(ROOT))
            for p in ROOT.rglob("*.go")
            if ".git" not in p.parts and "vendor" not in p.parts
        )
    return [line for line in proc.stdout.splitlines() if line and not line.startswith("vendor/")]


def go_targets() -> list[str]:
    return read_list("gate-targets.txt") or ["./..."]


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


def step_go(sub: str, go_files: list[str], boundary: str | None, targets: list[str]) -> tuple[str, str]:
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
    cmd = ["go", sub, *targets] if sub in ("build", "vet") else ["go", "test", "-count=1", *targets]
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


def step_secrets() -> tuple[str, str]:
    if not shutil.which("gitleaks"):
        return SKIP, "gitleaks not installed on this host (CI installs it)"
    proc = run(
        ["gitleaks", "detect", "--no-git", "--source", ".", "--redact", "--no-banner", "--exit-code", "7"],
        timeout=600,
    )
    if proc.returncode == 0:
        return PASS, "no leaks found in the tracked tree"
    if proc.returncode == 7:
        return FAIL, "gitleaks reported leaks (redacted):\n" + tail(proc.stdout, 1200)
    return SKIP, f"gitleaks did not complete (rc={proc.returncode}): " + tail(proc.stderr, 400)


def build_steps() -> list[tuple[str, str, str]]:
    go_files = tracked_go_files()
    targets = go_targets()
    _, boundary = go_env()
    steps: list[tuple[str, str, str]] = []
    steps.append(("gofmt", *step_gofmt(go_files)))
    for sub in ("build", "vet", "test"):
        steps.append((sub, *step_go(sub, go_files, boundary, targets)))
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
    partial = [s[0] for s in steps if s[1] in (SKIP, BOUNDARY)]
    if partial:
        print("GATE: PASS (partial: " + ", ".join(partial) + " did not run — see reasons above)")
    else:
        print("GATE: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
