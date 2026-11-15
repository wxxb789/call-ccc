#!/usr/bin/env python3
"""Compare this tree's inherited AgentsView behaviour with a pristine checkout at the pinned revision.

`scripts/upstream-parity.py` proves the carried files are byte-identical to the pin; this script
proves the bytes *behave* the same, by running one program — internal/ccc/parity/oracle, copied
verbatim into the pristine tree — over the same fixture bytes on both sides and diffing the two
canonical dumps: parse → sync into an isolated SQLite archive → archive read.

Both sides do the same work with the same flags, over separately copied fixture files and separately
created, throw-away archives; the only difference between them is which tree the inherited packages
come from. A run is a failure unless the dumps are byte-identical.

    python3 scripts/upstream-compare.py                 # fetch a pristine checkout, compare, clean up
    python3 scripts/upstream-compare.py --upstream DIR  # use an existing clean checkout at the pin
    python3 scripts/upstream-compare.py --record        # also record the dumps as the committed oracle
    python3 scripts/upstream-compare.py --keep          # keep the scratch directory for inspection

Needs network (a checkout has to come from somewhere) and the Go toolchain. It is deliberately not a
gate step: the gate is offline. Its job is to be re-runnable when someone asks "and how do you know
your copy still behaves like upstream?".
"""

from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import load_manifest  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "scripts" / "upstream-manifest.txt"
ORACLE_PACKAGE = Path("internal", "ccc")
FIXTURE_ROOT = Path("internal", "ccc", "testdata", "fixtures")
ORACLE_DIR = Path("internal", "ccc", "parity", "oracle")
ORACLE_RECORD_DIR = Path("internal", "ccc", "parity", "testdata", "oracle")
PRICING_SNAPSHOT = Path("internal", "pricing", "snapshot", "litellm_snapshot.json.gz")
PRICING_TOOL_PACKAGE = "./internal/pricing/cmd/litellm-snapshot"

#: The cases both sides run: the provider directory under the fixture root, and the session file the
#: case reads, relative to that directory. A provider may hold more than one fixture (a second
#: transcript shape), so every case names its file; this table mirrors internal/ccc/parity's Cases,
#: which is what decides what a case reads.
CASES = (
    ("claude", "-home-user-code-example-api/9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10.jsonl"),
    ("codex", "2026/05/04/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl"),
)

#: Build tags upstream ships with: without fts5 the inherited schema cannot load an archive that has
#: one, so both sides are built exactly the way an /AgentsView binary is built.
GO_TAGS = "fts5"


def run(cmd: list[str], cwd: Path | None = None, env: dict[str, str] | None = None) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    merged.setdefault("CGO_ENABLED", "1")
    if env:
        merged.update(env)
    return subprocess.run(cmd, cwd=str(cwd) if cwd else None, env=merged, capture_output=True, text=True, check=False)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def go_source_digest(root: Path, relative: Path) -> tuple[str, int]:
    """Hash the Go sources of a package tree, so both sides can be shown to run the same code."""
    hasher = hashlib.sha256()
    files = sorted(path for path in (root / relative).rglob("*.go") if "testdata" not in path.parts)
    for path in files:
        hasher.update(path.relative_to(root / relative).as_posix().encode("utf-8"))
        hasher.update(b"\0")
        hasher.update(path.read_bytes())
        hasher.update(b"\0")
    return hasher.hexdigest(), len(files)


def fetch_pristine(pin: str, remote: str, into: Path) -> None:
    """Clone exactly the pinned revision, without history that is not needed to build it."""
    steps = [
        ["git", "init", "--quiet"],
        ["git", "remote", "add", "origin", remote],
        ["git", "fetch", "--quiet", "--depth", "1", "origin", pin],
        ["git", "checkout", "--quiet", "FETCH_HEAD"],
    ]
    for step in steps:
        proc = run(step, cwd=into)
        if proc.returncode != 0:
            raise SystemExit(f"{' '.join(step)} failed in {into}:\n{proc.stderr.strip()}")


def verify_checkout(path: Path, pin: str) -> None:
    """Refuse a side that is not a clean checkout of the pin.

    The comparison attributes its dumps to the pinned revision, and ``--record`` rewrites the committed
    oracle with them, so a checkout holding local edits would attribute this tree's own work to
    upstream -- the sibling importer refuses a dirty checkout for the same reason.
    """
    if not (path / ".git").exists():
        raise SystemExit(f"{path} is not a git checkout")
    head = run(["git", "rev-parse", "HEAD"], cwd=path)
    if head.returncode != 0:
        raise SystemExit(f"git rev-parse failed in {path}: {head.stderr.strip()}")
    revision = head.stdout.strip()
    if revision != pin:
        raise SystemExit(f"{path} is at {revision}, not the pinned {pin}")
    status = run(["git", "status", "--porcelain"], cwd=path)
    if status.returncode != 0:
        raise SystemExit(f"git status failed in {path}: {status.stderr.strip()}")
    if status.stdout.strip():
        raise SystemExit(
            f"{path} has local modifications; compare against a clean checkout of the pin:\n"
            f"{status.stdout.strip()}")


def stage_pristine_tree(clone: Path) -> list[str]:
    """Prepare a pristine checkout to build the oracle: its own build inputs, plus our wrapper code."""
    notes = []
    snapshot = clone / PRICING_SNAPSHOT
    if not snapshot.exists():
        proc = run(["go", "run", PRICING_TOOL_PACKAGE, "-restore"], cwd=clone)
        if proc.returncode != 0:
            raise SystemExit(
                "restoring the pinned pricing snapshot in the pristine tree failed "
                f"(rc={proc.returncode}):\n{proc.stdout.strip()}\n{proc.stderr.strip()}")
        notes.append(f"restored {PRICING_SNAPSHOT} in the pristine tree from upstream's pinned artifact")

    target = clone / ORACLE_PACKAGE
    if target.exists():
        raise SystemExit(f"{target} already exists in {clone}; it was not pristine after all")
    shutil.copytree(
        ROOT / ORACLE_PACKAGE,
        target,
        ignore=shutil.ignore_patterns("testdata"),
    )
    notes.append(
        f"copied {ORACLE_PACKAGE} ({go_source_digest(ROOT, ORACLE_PACKAGE)[1]} Go source file(s)) "
        f"from this tree into the pristine checkout: the oracle and its call/ccc wrapper are identical "
        f"on both sides, so the only variable is which tree the inherited packages come from")
    return notes


def isolated_fixtures(destination: Path) -> Path:
    """Copy the committed fixtures so each side reads its own bytes and its own file metadata."""
    if destination.exists():
        shutil.rmtree(destination)
    shutil.copytree(ROOT / FIXTURE_ROOT, destination)
    return destination


def unstage_pristine_tree(clone: Path, digest_before: str) -> str:
    """Remove the copy this script made in the pristine checkout, and say what happened.

    A checkout the caller supplied is theirs: the copy is removed only when it still hashes to what
    was copied, so a checkout whose copy somebody edited is left alone and named instead of deleted.
    """
    target = clone / ORACLE_PACKAGE
    if not target.exists():
        return f"nothing to remove at {target}"
    digest_after, _ = go_source_digest(clone, ORACLE_PACKAGE)
    if digest_after != digest_before:
        return (f"left {target} in place: its contents changed during the run, so this script will "
                f"not delete it")
    shutil.rmtree(target)
    return f"removed {target} from the checkout again"


def run_oracle(tree: Path, fixtures: Path, work_dir: Path, case: tuple[str, str], out: Path) -> None:
    provider, fixture = case
    work_dir.mkdir(parents=True, exist_ok=True)
    proc = run(
        ["go", "run", f"-tags={GO_TAGS}", "./" + ORACLE_DIR.as_posix(),
         "-fixtures", str(fixtures), "-work-dir", str(work_dir),
         "-provider", provider, "-fixture", fixture, "-out", str(out)],
        cwd=tree,
    )
    if proc.returncode != 0:
        raise SystemExit(
            f"the oracle failed in {tree} for {provider} (rc={proc.returncode}):\n"
            f"{proc.stdout.strip()}\n{proc.stderr.strip()}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--upstream", type=Path, help="existing clean checkout of the pin to compare against")
    parser.add_argument("--record", action="store_true",
                        help="write the compared dumps as the committed oracle under "
                             "internal/ccc/parity/testdata/oracle")
    parser.add_argument("--keep", action="store_true", help="keep the scratch directory")
    parser.add_argument("--scratch", type=Path, help="scratch directory to use (implies --keep)")
    args = parser.parse_args(argv)

    staged_clone: Path | None = None
    copied_digest = ""
    manifest = load_manifest(MANIFEST)
    pin = manifest.header["pin"]
    remote = manifest.header["remote"]
    print(f"pin {pin} ({manifest.header.get('describe', '')}) from {remote}")

    scratch = args.scratch or Path(tempfile.mkdtemp(prefix="ccc-upstream-compare-"))
    scratch.mkdir(parents=True, exist_ok=True)
    if args.scratch:
        args.keep = True

    try:
        if args.upstream:
            clone = args.upstream.resolve()
            verify_checkout(clone, pin)
            staged_clone = clone
            print(f"using the existing checkout at {clone} (clean, at the pin)")
        else:
            clone = scratch / "upstream"
            clone.mkdir(parents=True, exist_ok=True)
            fetch_pristine(pin, remote, clone)
            verify_checkout(clone, pin)
            print(f"fetched the pinned revision into {clone} (clean, at the pin)")

        for note in stage_pristine_tree(clone):
            print("-", note)
        copied_digest, _ = go_source_digest(clone, ORACLE_PACKAGE)

        wrapper_digest, wrapper_files = go_source_digest(ROOT, ORACLE_PACKAGE)
        print(f"oracle source: {wrapper_files} file(s), sha256={wrapper_digest} (identical on both sides)")

        failures = 0
        for case in CASES:
            provider, fixture = case
            our_fixtures = isolated_fixtures(scratch / provider / "our-fixtures")
            their_fixtures = isolated_fixtures(scratch / provider / "their-fixtures")
            our_dump = scratch / provider / "our.json"
            their_dump = scratch / provider / "their.json"
            run_oracle(ROOT, our_fixtures, scratch / provider / "our-work", case, our_dump)
            run_oracle(clone, their_fixtures, scratch / provider / "their-work", case, their_dump)

            ours, theirs = our_dump.read_bytes(), their_dump.read_bytes()
            same = ours == theirs
            print(f"{provider}: fixture sha256="
                  f"{digest(our_fixtures / provider / Path(fixture))[:16]}… "
                  f"ours={hashlib.sha256(ours).hexdigest()[:16]}… "
                  f"pristine={hashlib.sha256(theirs).hexdigest()[:16]}… "
                  f"agreement={'yes' if same else 'NO'}")
            if same:
                if args.record:
                    target = ROOT / ORACLE_RECORD_DIR / f"{provider}.json"
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_bytes(theirs)
                    print(f"  recorded {target.relative_to(ROOT)}")
                continue

            failures += 1
            diff = run(["diff", "-u", str(our_dump), str(their_dump)])
            print("  the two dumps differ:")
            for line in diff.stdout.splitlines()[:40]:
                print("   ", line)

        if failures:
            print(f"RESULT: FAIL — {failures} of {len(CASES)} case(s) disagree with the pinned revision")
            return 1
        print(f"RESULT: PASS — {len(CASES)} case(s) agree byte for byte with the pinned revision")
        return 0
    finally:
        if staged_clone is not None:
            print("-", unstage_pristine_tree(staged_clone, copied_digest))
            residue = run(["git", "status", "--porcelain"], cwd=staged_clone)
            print(f"- {staged_clone} is {'clean' if not residue.stdout.strip() else 'dirty: ' + residue.stdout.strip()}")
        if not args.keep and scratch.exists():
            shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
