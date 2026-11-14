#!/usr/bin/env python3
"""Carry a pinned AgentsView revision into this tree and write the carry manifest.

The carried content comes from the upstream *git objects*, not from a working tree, so a checkout
with an unusual ``core.autocrlf`` cannot smuggle a rewritten file into the tree. Everything upstream
tracks is carried byte for byte except the paths listed in ``SKIP_REASONS`` (this repository owns
them) and ``.github/**`` (Actions would run upstream's workflows here), and each generated build
input is recorded with its hash instead of being copied from upstream's checkout.

    python3 scripts/upstream-import.py --upstream /path/to/agentsview [--pin <rev>]
                                       [--restore-snapshot] [--prune] [--dry-run]

The tree is left for review; this script never commits, pushes or deletes anything it did not write
(``--prune`` removes only files a previous manifest recorded as carried, and refuses if one of them
has been modified here).
"""

from __future__ import annotations

import argparse
import hashlib
import os
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from upstream_manifest import (  # noqa: E402  (path set up above)
    Manifest,
    Record,
    blob_sha1,
    hashed_path,
    load_manifest,
)

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / "scripts" / "upstream-manifest.txt"
REMOTE = "https://github.com/kenn-io/agentsview"
PRICING_TOOL = "internal/pricing/cmd/litellm-snapshot/main.go"

#: Build inputs upstream does not track but every Go build embeds. They are copied from the upstream
#: checkout (``--restore-snapshot`` fetches the pinned artifact there first) and recorded with their
#: SHA-256, so the tree compiles offline and the input is still verifiable against upstream's pin.
GENERATED_INPUTS = ("internal/pricing/snapshot/litellm_snapshot.json.gz",)

#: Upstream paths this repository owns instead of carrying. Each entry is a reason the checker
#: prints verbatim, so the deviation is identified rather than folded into a count.
SKIP_REASONS = {
    ".gitignore": "project-owned: call/ccc keeps its own ignore rules at this path",
    "AGENTS.md": "project-owned: call/ccc's control file lives at this path",
    "README.md": "project-owned: call/ccc's project README lives at this path",
    "docs/README.md": "project-owned: call/ccc's docs index lives at this path",
    # Upstream's own `.gitattributes` normalises text files to LF, and this 1.4 MB frontend fixture
    # holds one stray CRLF that `git add` therefore rewrites. The file is kept with the byte git
    # stores (identical otherwise); carrying upstream's blob verbatim would need an attribute file
    # inside a carried directory, which is project content in upstream-owned space.
    "frontend/src/lib/utils/__fixtures__/retained-tool-image-1735.json":
        "carried with one byte normalised: upstream's blob holds a single CRLF pair that this "
        "repository's carried .gitattributes turns into LF, so git stores the file one byte shorter",
}

#: Upstream CI definitions are not carried: Actions reads `.github/workflows/` at the repository
#: root, so carrying them would run upstream's CI (its runners, secrets and desktop builds) on every
#: call/ccc pull request.
SKIP_PREFIXES = {
    ".github/": "upstream CI definitions: Actions would run upstream's workflows on call/ccc pull requests",
}

#: Paths this repository owns. A tracked file outside the manifest must match one of these; a
#: trailing slash means "everything below".
OWNED = (
    ".github/",
    ".gitignore",
    "AGENTS.md",
    "README.md",
    "cmd/ccc/",
    "docs/README.md",
    "docs/conventions.md",
    "docs/core-design/",
    "docs/decisions/",
    "docs/evidence/",
    "docs/goals/",
    "docs/harness.md",
    "docs/plans/",
    "docs/upstream.md",
    "internal/ccc/",
    "scripts/gate-goflags.txt",
    "scripts/gate-required.txt",
    "scripts/gate-targets.txt",
    "scripts/gate.py",
    "scripts/tests/",
    "scripts/upstream-compare.py",
    "scripts/upstream-import.py",
    "scripts/upstream_manifest.py",
    "scripts/upstream-manifest.txt",
    "scripts/upstream-parity.py",
)


def git(upstream: Path, *args: str, text: bool = True) -> subprocess.CompletedProcess:
    """Ask git about a checkout; a missing git is an rc=127 answer, not a crash."""
    try:
        return subprocess.run(
            ["git", "-C", str(upstream), *args],
            capture_output=True,
            text=text,
            check=False,
        )
    except OSError as exc:
        empty = "" if text else b""
        return subprocess.CompletedProcess(["git", *args], 127, empty, str(exc))


def upstream_entries(upstream: Path, pin: str) -> list[tuple[str, str, str]]:
    """Return ``(mode, blob-sha1, path)`` for every tracked blob at ``pin``."""
    proc = git(upstream, "ls-tree", "-r", "-z", "--full-tree", pin)
    if proc.returncode != 0:
        raise SystemExit(f"git ls-tree failed at {pin}: {proc.stderr.strip()}")
    entries = []
    for raw in proc.stdout.split("\0"):
        if not raw:
            continue
        meta, _, path = raw.partition("\t")
        mode, kind, sha = meta.split(" ")
        if kind != "blob":
            raise SystemExit(f"{path}: tracked entry is a {kind}, not a blob")
        entries.append((mode, sha, path))
    return entries


def blob_contents(upstream: Path, shas: list[str]) -> dict[str, bytes]:
    """Read many blobs in one ``git cat-file --batch`` call, keyed by object name."""
    wanted = list(dict.fromkeys(shas))
    proc = subprocess.run(
        ["git", "-C", str(upstream), "cat-file", "--batch"],
        input=("\n".join(wanted) + "\n").encode("utf-8"),
        capture_output=True,
        check=False,
    )
    if proc.returncode != 0:
        raise SystemExit(f"git cat-file failed: {proc.stderr.decode('utf-8', 'replace').strip()}")
    out = proc.stdout
    contents: dict[str, bytes] = {}
    offset = 0
    while offset < len(out):
        newline = out.index(b"\n", offset)
        header = out[offset:newline].decode("utf-8", "replace")
        offset = newline + 1
        sha, kind, size = header.split(" ")
        if kind != "blob":
            raise SystemExit(f"{sha}: cat-file answered a {kind}")
        length = int(size)
        contents[sha] = out[offset:offset + length]
        offset += length + 1
    missing = [sha for sha in wanted if sha not in contents]
    if missing:
        raise SystemExit(f"cat-file did not answer for {len(missing)} blob(s), first {missing[0]}")
    return contents


def skip_reason(path: str) -> str:
    if path in SKIP_REASONS:
        return SKIP_REASONS[path]
    for prefix, reason in SKIP_PREFIXES.items():
        if path.startswith(prefix):
            return reason
    return ""


def write_entry(target: Path, mode: str, data: bytes, dry_run: bool) -> None:
    """Write one carried blob, replacing whatever is there (file or link)."""
    if dry_run:
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.is_symlink() or target.exists():
        target.unlink()
    if mode == "120000":
        try:
            os.symlink(data.decode("utf-8"), target)
        except (OSError, NotImplementedError):
            # A platform without symlink support checks the link out as a file holding the target.
            target.write_bytes(data)
        return
    target.write_bytes(data)
    if mode == "100755":
        os.chmod(target, 0o755)


def snapshot_sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tool_constant(path: Path) -> str:
    """The pinned artifact SHA-256 compiled into the carried snapshot tool, if readable."""
    if not path.exists():
        return ""
    for line in path.read_text(encoding="utf-8").splitlines():
        if "defaultSnapshotSHA256" in line and "=" in line:
            _, _, value = line.partition("=")
            return value.strip().strip('"')
    return ""


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--upstream", required=True, type=Path, help="checkout to read from")
    parser.add_argument("--pin", default="", help="revision to carry (default: the checkout's HEAD)")
    parser.add_argument("--restore-snapshot", action="store_true",
                        help="run the carried snapshot tool in --upstream first (needs network)")
    parser.add_argument("--prune", action="store_true",
                        help="delete files a previous manifest carried but this revision no longer has")
    parser.add_argument("--dry-run", action="store_true", help="report what would change; write nothing")
    args = parser.parse_args(argv)

    upstream: Path = args.upstream.resolve()
    if not (upstream / ".git").exists():
        raise SystemExit(f"{upstream} is not a git checkout")

    revision = git(upstream, "rev-parse", "HEAD")
    if revision.returncode != 0:
        raise SystemExit(f"git rev-parse HEAD failed in {upstream}: {revision.stderr.strip()}")
    head = revision.stdout.strip()
    pin = args.pin or head
    if pin != head:
        raise SystemExit(f"--pin {pin} is not the checkout's HEAD ({head}); check out the pin first")
    status = git(upstream, "status", "--porcelain").stdout.strip()
    if status:
        raise SystemExit(f"{upstream} has local modifications; carry from a clean checkout:\n{status}")

    if args.restore_snapshot:
        proc = subprocess.run(
            ["go", "run", "./internal/pricing/cmd/litellm-snapshot", "-restore"],
            cwd=upstream,
            check=False,
        )
        if proc.returncode != 0:
            raise SystemExit(f"snapshot restore failed with rc={proc.returncode}")

    entries = upstream_entries(upstream, pin)
    carried = [(mode, sha, path) for mode, sha, path in entries if not skip_reason(path)]
    skipped = [(mode, sha, path) for mode, sha, path in entries if skip_reason(path)]
    overlap = [path for _, _, path in carried if any(
        path == owned or path.startswith(owned) for owned in OWNED
    )]
    if overlap:
        raise SystemExit(f"carried path is declared project-owned too: {overlap[0]}")

    contents = blob_contents(upstream, [sha for _, sha, _ in entries])

    previous = load_manifest(MANIFEST) if args.prune and MANIFEST.exists() else None

    for mode, sha, path in carried:
        data = contents[sha]
        if blob_sha1(data) != sha:
            raise SystemExit(f"{path}: content does not hash to {sha}")
        write_entry(ROOT / path, mode, data, args.dry_run)

    if args.prune and previous:
        keep = {path for _, _, path in carried}
        for record in previous.files:
            if record.path in keep:
                continue
            target = ROOT / record.path
            if not target.exists() and not target.is_symlink():
                continue
            digest, _, _ = hashed_path(ROOT, record.path)
            if digest != record.digest:
                raise SystemExit(f"{record.path}: modified here; refusing to prune it")
            if args.dry_run:
                continue
            target.unlink()

    input_records: list[Record] = []
    for path in GENERATED_INPUTS:
        source = upstream / path
        target = ROOT / path
        if not source.exists():
            raise SystemExit(
                f"{path} does not exist in {upstream}: re-run with --restore-snapshot "
                f"(needs network) so the pinned artifact is fetched first"
            )
        data = source.read_bytes()
        if not args.dry_run:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
        digest = hashlib.sha256(data).hexdigest()
        pinned = tool_constant(ROOT / PRICING_TOOL)
        if pinned and pinned != digest:
            raise SystemExit(f"{path} hashes to {digest}, but the carried tool pins {pinned}")
        input_records.append(Record(
            "input", "100644", digest, len(data), path,
            f"generated build input embedded by internal/pricing; upstream restores it from a pinned "
            f"artifact before every Go build (sha256 pinned in {PRICING_TOOL})",
        ))

    records: list[Record] = [Record("owned", "", "", 0, prefix) for prefix in OWNED]
    records += [Record("file", mode, sha, len(contents[sha]), path) for mode, sha, path in sorted(
        carried, key=lambda entry: entry[2]
    )]
    records += [
        Record("skip", mode, sha, len(contents[sha]) if sha in contents else -1, path, skip_reason(path))
        for mode, sha, path in sorted(skipped, key=lambda entry: entry[2])
    ]
    records += input_records

    describe = git(upstream, "describe", "--tags", pin).stdout.strip()
    commit_date = git(upstream, "show", "-s", "--format=%cI", pin).stdout.strip()
    manifest = Manifest(
        {
            "pin": pin,
            "remote": REMOTE,
            "describe": describe,
            "commit-date": commit_date,
            "upstream-files": str(len(entries)),
            "carried": str(len(carried)),
            "deviations": str(len(skipped) + len(input_records)),
        },
        records,
    )
    if not args.dry_run:
        MANIFEST.write_text(manifest.render(), encoding="utf-8")

    print(f"pin {pin} ({describe}, {commit_date})")
    print(f"upstream tracked files: {len(entries)}")
    print(f"carried byte for byte:  {len(carried)}")
    print(f"declared deviations:    {len(skipped) + len(input_records)}"
          f" ({len(skipped)} upstream paths not carried, {len(input_records)} generated input)")
    print(f"project-owned prefixes: {len(OWNED)}")
    if args.dry_run:
        print("dry run: nothing written")
    return 0


if __name__ == "__main__":
    sys.exit(main())
