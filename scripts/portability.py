#!/usr/bin/env python3
"""Prove this repository's own module builds and tests against a *pristine* pinned carry. Needs network.

The gate's `build`/`vet`/`test` steps compile this repository's module against the carry as it sits
in the tree -- which the required `parity` step has proved to be the pin's bytes, but on *its* terms.
This check answers the other question the layout exists for: is the carried module a leaf the layout
can be re-aimed at? It takes a pristine checkout of the pinned revision, rewrites the module's one
edge (`replace go.kenn.io/agentsview => …`) to point at that checkout instead of the in-tree carry,
and then builds and tests *this tree's own* module against it:

    python3 scripts/portability.py                 # use the cache, fetching the pin if needed
    python3 scripts/portability.py --upstream DIR  # use an existing clean checkout at the pin
    python3 scripts/portability.py --keep          # keep the scratch directory for inspection

The module's own files are the ones tested -- nothing is copied -- through Go's `-modfile` switch: an
alternate `go.mod` (and its `.sum`) is written into a scratch directory *outside this tree* holding
the rewritten ``replace``, and `go build`/`go test` run in the module's own directory with
`-modfile=<scratch>`. The repository is never written to, the checkout is never written to unless this
run cloned it itself (a caller's `--upstream` is left exactly as it was found), and the scratch
directory is removed again unless `--keep` says otherwise.

The pristine checkout is taken, in this order, from `--upstream DIR`, `CCC_UPSTREAM_DIR`, or a cache
under `CCC_UPSTREAM_CACHE` (default: this platform's cache directory, `ccc-upstream/<pin>`), and
fetched with `git clone --filter=blob:none --no-checkout` + `git checkout --detach <pin>` when it is
not there yet. A checkout that is used is verified first: it is a git checkout, its `HEAD` is the pin,
and `git status --porcelain` is empty -- the same three answers
`scripts/upstream-compare.py`'s `verify_checkout` requires before it attributes a dump to the pin, and
they are the reason a result from this run may be attributed to that revision at all. A checkout a
caller handed over that fails those answers is a failure, not a skip: the run was asked to use it.

The generated build input upstream does not track (`internal/pricing/snapshot/litellm_snapshot.json.gz`,
restored by upstream's Makefile before every Go-compiling target) is written into a checkout this run
fetched itself, taken from this tree's own copy -- whose SHA-256 the carried snapshot tool pins and
the `parity` step verifies -- and a caller's checkout without it is refused with the command that
restores it, rather than written to.

Exit codes: 0 the module built and its tests passed against the pin, 1 a rule is violated (the tree's
module, a supplied checkout, or the build itself), 2 the check could not be run here (no pin checkout
to be had without network, no Go toolchain, the platform's C-compiler boundary, a tree whose own
module is the repository root) -- never a pass.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import Manifest, ManifestError, detect_layout, load_manifest  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
MANIFEST_RELATIVE = Path("scripts") / "upstream-manifest.txt"
GATE = Path("scripts") / "gate.py"
OUR_MODULE_DIR = "go.mod"
#: The one edge the layout is: the module's own ``go.mod`` declares the carried module a requirement
#: and points the replace at the carry beside it. This check rewrites exactly this line, and a module
#: whose file no longer holds it is a failure -- there would be no edge to aim.
REPLACE_LINE = "replace go.kenn.io/agentsview => ../agentsview"
REPLACE_PATTERN = re.compile(r"^replace\s+go\.kenn\.io/agentsview\s+=>\s+(\S+)\s*$", re.MULTILINE)
#: How much of a failing command's output the step's line carries.
OUTPUT_TAIL = 1500

class PortabilityError(Exception):
    """A rule this check enforces is violated, with the answer that names it."""


class PortabilityIncomplete(Exception):
    """This host cannot run the check, named so the gate can disclose it instead of passing."""

    def __init__(self, summary: str, skipped: list[str]):
        super().__init__(summary)
        self.summary = summary
        self.skipped = skipped


def gate_module(root: Path | None = None):
    """`scripts/gate.py` as a module: the Go targets, flags and platform boundary live there.

    The same seam `scripts/obligation-citations.py` reads, for the same reason: a second copy of
    "which packages, with which flags, on which platform" would be a second answer to one question.
    ``root`` is the tree the knobs are read from -- ``check`` hands over the tree it is checking, so a
    throw-away tree's own ``scripts/gate-targets.txt`` is what a case's run is handed.
    """
    base = root or ROOT
    spec = importlib.util.spec_from_file_location("shipped_gate_for_portability", base / GATE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.ROOT = base
    return module


def run(cmd: list[str], cwd: Path, env: dict[str, str] | None = None, timeout: int = 3600):
    """Run one command, answering its result; a missing binary is an rc=127 answer, not a crash."""
    merged = os.environ.copy()
    merged.update(env or {})
    try:
        return subprocess.run(
            cmd, cwd=str(cwd), env=merged, capture_output=True, text=True, timeout=timeout
        )
    except OSError as exc:
        return subprocess.CompletedProcess(cmd, 127, "", f"{cmd[0]}: {exc}\n")


def tail(text: str, limit: int = OUTPUT_TAIL) -> str:
    text = (text or "").strip()
    return text if len(text) <= limit else "…\n" + text[-limit:]


def scratch_directory(prefix: str) -> Path:
    """A fresh scratch directory, absolute, and **outside this tree**.

    ``tempfile.gettempdir()`` answers whatever ``TMPDIR`` says, and a caller's relative ``TMPDIR`` --
    or one inside the repository -- would otherwise put the scratch under the process's current
    directory (the repository root when the gate runs this), where the ``parity`` step would then see
    an unexplained path. Each candidate is therefore resolved and checked against the tree before it
    is used, and the platform's own scratch roots are the fallback; when none of them is usable the
    run says so instead of leaving a directory nobody asked for inside the repository.
    """
    attempts: list[str] = []
    for base in (None, Path("/tmp"), Path("/var/tmp")):
        if base is not None and (not base.is_dir() or base == ROOT or ROOT in base.parents):
            continue
        try:
            candidate = Path(
                tempfile.mkdtemp(prefix=prefix, dir=str(base) if base else None)).resolve()
        except OSError as exc:
            attempts.append(f"{base if base is not None else tempfile.gettempdir()}: {exc}")
            continue
        if candidate != ROOT and ROOT not in candidate.parents:
            return candidate
        shutil.rmtree(candidate, ignore_errors=True)
        attempts.append(f"{base if base is not None else tempfile.gettempdir()}: inside {ROOT}")
    raise PortabilityIncomplete(
        f"every scratch directory this host offers is inside the repository (or unusable): "
        f"{'; '.join(attempts)}, and a scratch tree inside the repository would be an unexplained "
        f"path to the parity step",
        ["a scratch directory outside this tree: set TMPDIR to an absolute path outside it"],
    )


def default_cache_root() -> Path:
    """Where fetched pin checkouts are cached: `CCC_UPSTREAM_CACHE`, else the platform's cache dir."""
    override = os.environ.get("CCC_UPSTREAM_CACHE")
    if override:
        return Path(override).expanduser()
    if os.name == "nt":
        base = os.environ.get("LOCALAPPDATA") or str(Path.home() / "AppData" / "Local")
    else:
        base = os.environ.get("XDG_CACHE_HOME") or str(Path.home() / ".cache")
    return Path(base) / "ccc-upstream"


def verify_checkout(directory: Path, pin: str) -> str | None:
    """Why this directory is not a clean checkout of ``pin``, or ``None`` when it is.

    The three answers `scripts/upstream-compare.py` also requires before it attributes anything to the
    pinned revision: a git checkout, its ``HEAD`` at the pin, and nothing modified -- restated here
    (rather than imported from a script module) so a caller can be told exactly which one failed.
    """
    if not (directory / ".git").exists():
        return f"{directory} is not a git checkout"
    head = run(["git", "-C", str(directory), "rev-parse", "HEAD"], cwd=directory)
    if head.returncode != 0:
        return f"git rev-parse HEAD failed in {directory}: {tail(head.stderr, 400)}"
    if head.stdout.strip() != pin:
        return f"{directory} is at {head.stdout.strip() or 'an unreadable revision'}, not the pinned {pin}"
    status = run(["git", "-C", str(directory), "status", "--porcelain"], cwd=directory)
    if status.returncode != 0:
        return f"git status failed in {directory}: {tail(status.stderr, 400)}"
    if status.stdout.strip():
        return f"{directory} has local modifications, so nothing taken from it is the pin's:\n" + \
            tail(status.stdout, 600)
    return None


def fetch_checkout(directory: Path, remote: str, pin: str) -> str | None:
    """Fetch the pin into ``directory``, answering why it could not be done, or ``None``.

    ``--filter=blob:none`` keeps the fetch small (the commit graph and tags, which `git describe`
    needs, without every blob) and the checkout then reads the blobs it needs; the recipe is the one
    measured on this repository's own host, and it needs no support for fetching an unadvertised
    commit by name.
    """
    directory.parent.mkdir(parents=True, exist_ok=True)
    clone = run(["git", "clone", "--no-checkout", "--filter=blob:none", remote, str(directory)],
                cwd=directory.parent)
    if clone.returncode != 0:
        return f"git clone of {remote} failed (rc={clone.returncode}): {tail(clone.stderr, 600)}"
    checkout = run(["git", "-C", str(directory), "checkout", "--detach", pin], cwd=directory)
    if checkout.returncode != 0:
        return (f"git checkout --detach {pin} failed (rc={checkout.returncode}): "
                f"{tail(checkout.stderr, 600)}")
    problem = verify_checkout(directory, pin)
    if problem:
        return f"the fetched checkout is not the pin: {problem}"
    return None


def checkout_for_pin(pin: str, remote: str, supplied: Path | None) -> tuple[Path, list[str]]:
    """A clean checkout of the pin and the notes the run's answer should disclose.

    A supplied checkout (``--upstream``/``CCC_UPSTREAM_DIR``) is refused when it fails the three
    answers -- the caller asked for *it* -- while a cache that is not at the pin is only ever used
    when it is (a stale cache is not deleted: it may be somebody's working copy, and the answer names
    it instead). Only a checkout this run fetched itself may be written to, which is what the caller
    of this function learns from the second half of its answer.
    """
    notes: list[str] = []
    if supplied is not None:
        problem = verify_checkout(supplied, pin)
        if problem:
            raise PortabilityError(
                f"--upstream {supplied} cannot be attributed to the pin: {problem}")
        return supplied, notes
    cache = default_cache_root() / pin
    if cache.exists():
        problem = verify_checkout(cache, pin)
        if problem is None:
            return cache, notes
        raise PortabilityIncomplete(
            f"the cached checkout at {cache} is not a clean checkout of the pin, and this run does "
            f"not delete a directory it did not create: {problem}",
            [f"remove {cache} (or point CCC_UPSTREAM_CACHE elsewhere) and re-run"],
        )
    problem = fetch_checkout(cache, remote, pin)
    if problem:
        raise PortabilityIncomplete(
            f"no pinned checkout is available: {problem}",
            [f"fetch {pin} into {cache} by hand, or pass --upstream DIR with a clean checkout of it"],
        )
    notes.append(f"fetched the pin into {cache}")
    return cache, notes


def restore_inputs(manifest: Manifest, checkout: Path, ours: bool) -> list[str]:
    """Provide the generated build inputs the carried ``go:embed`` needs, from this tree's copies.

    Upstream does not track them, so a pristine checkout reads them from a pinned artifact restored by
    its Makefile before every Go-compiling target; this check writes this tree's own copies into a
    checkout *it* fetched (their SHA-256 is what the manifest pins and the `parity` step verifies).
    A caller's checkout is never written to: it is expected to have restored them itself, and a missing
    one is named with the command that restores it.
    """
    notes: list[str] = []
    for record in manifest.inputs:
        target = checkout / record.path
        source = ROOT / manifest.tree_path(record.path)
        if target.exists():
            data = target.read_bytes()
            if hashlib.sha256(data).hexdigest() != record.digest:
                raise PortabilityError(
                    f"{target} is not the input this tree pins: it hashes to "
                    f"{hashlib.sha256(data).hexdigest()}, the manifest records {record.digest}")
            continue
        if not ours:
            raise PortabilityError(
                f"{checkout} has no {record.path}, which the carried internal/pricing go:embed needs: "
                f"restore it in that checkout first "
                f"(go run ./internal/pricing/cmd/litellm-snapshot -restore)")
        if not source.exists():
            raise PortabilityError(f"{source} is missing, so the input cannot be provided")
        data = source.read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest != record.digest:
            raise PortabilityError(
                f"{source} hashes to {digest}, the manifest records {record.digest}: this tree's own "
                f"input is not the pinned artifact")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        notes.append(f"wrote {record.path} into the fetched checkout "
                     f"(sha256={digest}, the value the carried tool pins)")
    return notes


def rewrite_modfile(module_file: Path, target: Path, scratch: Path) -> tuple[Path, Path]:
    """Write the module's ``go.mod`` into ``scratch`` with its replace aimed at ``target``.

    The alternate file is what Go's ``-modfile`` switch reads instead of the module's own, and Go
    reads the sum file beside it under the same stem -- so both are written, and the module's own
    files are never touched. The line that is rewritten is asserted first: a module that no longer
    declares the layout's one edge has nothing this check could aim.
    """
    text = module_file.read_text(encoding="utf-8")
    match = REPLACE_PATTERN.search(text)
    if not match or match.group(0).strip() != REPLACE_LINE:
        raise PortabilityError(
            f"{module_file} does not declare the layout's one edge ({REPLACE_LINE!r}): the carried "
            f"module is not a requirement this module points at the carry beside it, so this check "
            f"has no edge to aim at the pin")
    rewritten = text[:match.start()] + f"replace go.kenn.io/agentsview => {target}" + text[match.end():]
    modfile = scratch / "portability.mod"
    modfile.write_text(rewritten, encoding="utf-8")
    sumfile = scratch / "portability.sum"
    module_sum = module_file.with_name(module_file.name.replace(".mod", ".sum"))
    if not module_sum.exists():
        raise PortabilityError(f"{module_file} has no {module_sum.name} beside it")
    sumfile.write_text(module_sum.read_text(encoding="utf-8"), encoding="utf-8")
    return modfile, sumfile


def run_go(sub: str, module_dir: Path, modfile: Path, targets: list[str], flags: list[str],
           env: dict[str, str]) -> subprocess.CompletedProcess:
    if sub == "test":
        cmd = ["go", "test", "-count=1", "-modfile", str(modfile), *flags, *targets]
    else:
        cmd = ["go", sub, "-modfile", str(modfile), *flags, *targets]
    return run(cmd, cwd=module_dir, env=env)


def check(root: Path, supplied: Path | None, keep: bool) -> list[str]:
    """Build and test this repository's module against a pristine checkout of the pin."""
    manifest_path = root / MANIFEST_RELATIVE
    try:
        manifest = load_manifest(manifest_path)
    except (OSError, ManifestError) as broken:
        raise PortabilityIncomplete(
            f"no readable carry manifest at {manifest_path} ({broken}), so the pin to check against "
            f"is unknown",
            ["scripts/upstream-manifest.txt"],
        )
    pin = manifest.header.get("pin", "")
    remote = manifest.header.get("remote", "")
    if not pin or not remote:
        raise PortabilityError(f"{manifest_path} does not record both a pin and a remote")

    layout = detect_layout(root)
    if not layout.carry:
        raise PortabilityIncomplete(
            "this tree's own module is the repository root, so there is no separate module to build "
            "against a scratch carry (the layout whose carry has its own directory is the shape this "
            "check is about)",
            ["the ccc/ module beside agentsview/ (the layout this repository moved to)"],
        )
    module_dir = root / layout.module
    module_file = module_dir / OUR_MODULE_DIR
    if not module_file.exists():
        raise PortabilityError(f"{module_file} is missing, so there is no module to check")

    gate = gate_module(root)
    targets = gate.go_targets()
    flags = gate.go_flags()
    env, boundary = gate.go_env()
    if boundary:
        raise PortabilityIncomplete(boundary, ["go build/test on this host"])
    if not shutil.which("go"):
        raise PortabilityIncomplete("go is not on this host's PATH", ["the Go toolchain"])

    checkout, notes = checkout_for_pin(pin, remote, supplied)
    ours = supplied is None
    notes += restore_inputs(manifest, checkout, ours)

    scratch = scratch_directory("ccc-portability-")
    try:
        modfile, sumfile = rewrite_modfile(module_file, checkout, scratch)
        results: list[str] = []
        for sub in ("build", "test"):
            proc = run_go(sub, module_dir, modfile, targets, flags, env)
            output = (proc.stdout or "") + ("\n" + proc.stderr if proc.stderr else "")
            if proc.returncode != 0:
                raise PortabilityError(
                    f"go {sub} {' '.join(targets)} against the pin -> rc={proc.returncode}\n"
                    + tail(output))
            if sub == "test":
                passed = sum(1 for line in (proc.stdout or "").splitlines()
                             if line.startswith("ok "))
                results.append(f"{passed} package(s) ok")
            else:
                results.append(f"go build {' '.join(targets)} -> rc=0")
        notes.append(f"ran with -modfile={modfile.name} (replace -> {checkout}) and "
                     f"{sumfile.name} beside it")
        return [
            f"pin {pin} ({manifest.header.get('describe', '')})",
            f"module {layout.module} built and tested against the pristine checkout at {checkout}",
            *notes,
            *results,
        ]
    finally:
        if not keep:
            shutil.rmtree(scratch, ignore_errors=True)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--upstream", type=Path, default=None,
                        help="an existing clean checkout of the pin (default: fetch/use the cache)")
    parser.add_argument("--keep", action="store_true",
                        help="keep the scratch directory holding the alternate go.mod")
    parser.add_argument("--quiet", action="store_true", help="print nothing when the check passes")
    args = parser.parse_args(argv)

    supplied = args.upstream
    if supplied is None:
        env_dir = os.environ.get("CCC_UPSTREAM_DIR", "")
        supplied = Path(env_dir).expanduser() if env_dir else None

    try:
        lines = check(ROOT, supplied.resolve() if supplied else None, args.keep)
    except ManifestError as broken:
        print(f"portability: FAIL — the carry manifest is malformed: {broken}")
        return 1
    except PortabilityError as failure:
        print(f"portability: FAIL — {failure}")
        return 1
    except PortabilityIncomplete as incomplete:
        print(f"portability: INCOMPLETE — {incomplete.summary}")
        for skipped in incomplete.skipped:
            print(f"  not checked: {skipped}")
        return 2
    if not args.quiet:
        for line in lines:
            print(line)
    return 0


if __name__ == "__main__":
    sys.exit(main())
