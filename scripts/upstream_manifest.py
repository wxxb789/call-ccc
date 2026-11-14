"""Shared format for the upstream carry manifest, used by the importer and the checker.

The manifest is the contract between ``scripts/upstream-import.py`` (writes it while carrying a
pinned AgentsView revision into this tree) and ``scripts/upstream-parity.py`` (reads it to prove the
carried files still match that revision, offline). Keeping one reader/writer here means the two
scripts cannot drift apart in how they spell a record.

Format -- header lines ``key value``, then tab-separated records, sorted by path::

    pin              <40-hex upstream revision>
    remote           <clone URL>
    describe         <git describe --tags output at the pin>
    commit-date      <committer date, RFC 3339>
    upstream-files   <tracked files at the pin>
    carried          <files carried byte for byte>
    deviations       <paths deliberately not carried or not byte-identical>

    file   <mode> <blob-sha1> <size> <path>
    skip   <mode> <blob-sha1> <size> <path> <reason> [<stored-blob-sha1> <stored-size>]
    input  <mode> <sha256>    <size> <path> <reason>

``mode`` is git's: ``100644``, ``100755`` or ``120000`` (symlink; the blob holds the target text).
``owned`` records list the path prefixes this repository owns -- anything else tracked in the tree
must be a ``file`` record, which is what makes an unexplained addition fail the check.

A ``skip`` record names an upstream path this repository does not carry as upstream's own blob: a
project-owned path, or one carried with a declared byte normalisation (upstream's ``.gitattributes``
rewrites line endings when git stores a text file). When the path *is* carried anyway, the record
also holds the digest and size of the bytes this tree stores for it, because upstream's digest could
never match those -- the normalisation is the reason the path is declared rather than carried. The
checker verifies the stored bytes (git's index where git is available, the file's own bytes where it
is not) and still refuses a deviation that has quietly become upstream's copy.

The blob hash is git's own (``sha1("blob <size>\\0" + bytes)``) over the file's committed content.
It is computed here rather than shelled out to ``git hash-object`` so the check works in a tree
without git and answers the same on every platform; ``.gitattributes`` (carried from upstream)
normalises line endings to LF on checkout, so the working tree bytes are the committed bytes.
"""

from __future__ import annotations

import hashlib
import os
import re
from pathlib import Path

HEADER_KEYS = ("pin", "remote", "describe", "commit-date", "upstream-files", "carried", "deviations")
RECORD_KINDS = ("file", "skip", "input", "owned")


class ManifestError(ValueError):
    """The manifest file is malformed or internally inconsistent."""


class Record:
    """One manifest record.

    ``stored_digest``/``stored_size`` are set only on a ``skip`` record whose path this tree carries
    anyway: they are the digest and size of the bytes this tree stores for it (see the module
    docstring), which upstream's own blob can never equal.
    """

    __slots__ = ("kind", "mode", "digest", "size", "path", "reason", "stored_digest", "stored_size")

    def __init__(self, kind: str, mode: str, digest: str, size: int, path: str, reason: str = "",
                 stored_digest: str = "", stored_size: int = 0):
        self.kind = kind
        self.mode = mode
        self.digest = digest
        self.size = size
        self.path = path
        self.reason = reason
        self.stored_digest = stored_digest
        self.stored_size = stored_size

    def __repr__(self) -> str:  # pragma: no cover - debugging aid
        return f"Record({self.kind!r}, {self.mode!r}, {self.digest!r}, {self.size}, {self.path!r})"

    @property
    def line(self) -> str:
        if self.kind == "owned":
            return f"owned\t{self.path}"
        fields = [self.kind, self.mode, self.digest, str(self.size), self.path]
        if self.kind in ("skip", "input"):
            fields.append(self.reason)
        if self.kind == "skip" and self.stored_digest:
            fields += [self.stored_digest, str(self.stored_size)]
        return "\t".join(fields)


class Manifest:
    """A parsed manifest: header values, records in file order, and lookups."""

    def __init__(self, header: dict[str, str], records: list[Record]):
        self.header = header
        self.records = records

    @property
    def files(self) -> list[Record]:
        return [r for r in self.records if r.kind == "file"]

    @property
    def skips(self) -> list[Record]:
        return [r for r in self.records if r.kind == "skip"]

    @property
    def inputs(self) -> list[Record]:
        return [r for r in self.records if r.kind == "input"]

    @property
    def owned(self) -> list[str]:
        return [r.path for r in self.records if r.kind == "owned"]

    @property
    def stored_deviations(self) -> list[Record]:
        """Declared deviations this tree carries anyway, with the bytes each record pins."""
        return [r for r in self.skips if r.stored_digest]

    def is_owned(self, path: str) -> bool:
        """True when ``path`` is inside a self-owned prefix (which may be a directory)."""
        for prefix in self.owned:
            if prefix.endswith("/"):
                if path.startswith(prefix):
                    return True
            elif path == prefix:
                return True
        return False

    def render(self) -> str:
        lines = ["# call/ccc upstream carry manifest -- generated by scripts/upstream-import.py.",
                 "# Do not edit by hand; re-run the importer at a new pin instead."]
        for key in HEADER_KEYS:
            if key in self.header:
                lines.append(f"{key} {self.header[key]}")
        lines.append("#")
        lines.append("# file\tshape\tblob-sha1\tsize\tpath  (mode 100644/100755, 120000 = symlink)")
        lines.append("# skip\tshape\tblob-sha1\tsize\tpath\treason  (upstream paths deliberately not "
                     "carried as upstream's bytes)")
        lines.append("# skip\t...\tstored-blob-sha1\tstored-size  (when the path is carried anyway: "
                     "the bytes this tree stores for it)")
        lines.append("# input\tshape\tsha256\tsize\tpath\treason  (generated build inputs, not tracked upstream)")
        lines.append("# owned\tpath-prefix  (this repository owns the path; a trailing / means a directory)")
        for record in self.records:
            lines.append(record.line)
        return "\n".join(lines) + "\n"


def parse_manifest(text: str) -> Manifest:
    header: dict[str, str] = {}
    records: list[Record] = []
    for number, raw in enumerate(text.splitlines(), start=1):
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        if "\t" not in raw:
            key, _, value = raw.partition(" ")
            if key not in HEADER_KEYS:
                raise ManifestError(f"line {number}: unknown header key {key!r}")
            if key in header:
                raise ManifestError(f"line {number}: repeated header key {key!r}")
            header[key] = value.strip()
            continue
        fields = raw.split("\t")
        kind = fields[0]
        if kind not in RECORD_KINDS:
            raise ManifestError(f"line {number}: unknown record kind {kind!r}")
        if kind == "owned":
            if len(fields) != 2:
                raise ManifestError(f"line {number}: owned takes exactly one field")
            records.append(Record(kind, "", "", 0, fields[1]))
            continue
        if kind == "skip" and len(fields) == 8:
            _, mode, digest, size, path, reason, stored_digest, stored_size = fields
        elif len(fields) in (5, 6):
            _, mode, digest, size, path, reason = fields + [""] * (6 - len(fields))
            stored_digest, stored_size = "", ""
        else:
            detail = " (or 8 for a declared deviation carried with the bytes this tree stores)" \
                if kind == "skip" else ""
            raise ManifestError(
                f"line {number}: {kind} takes 5 or 6 fields{detail}, got {len(fields)}")
        for name, value in (("size", size), ("stored size", stored_size)):
            if value and not value.isdigit():
                raise ManifestError(f"line {number}: {name} {value!r} is not a number")
        records.append(Record(kind, mode, digest, int(size), path, reason, stored_digest,
                              int(stored_size) if stored_size else 0))
    return Manifest(header, records)


def load_manifest(path: Path) -> Manifest:
    return parse_manifest(path.read_text(encoding="utf-8"))


def blob_sha1(data: bytes) -> str:
    """Git's object name for ``data``: sha1 over ``blob <size>\\0`` plus the bytes."""
    return hashlib.sha1(b"blob %d\x00" % len(data) + data).hexdigest()


#: The form the carried snapshot tool pins its artifact in: a quoted 64-hex SHA-256 constant. The
#: importer, the parity checker and the checker's tests all have to read the same value from the same
#: source, so the reader lives here instead of being spelled three ways.
SNAPSHOT_PIN = re.compile(r'defaultSnapshotSHA256\s*=\s*"([0-9a-f]{64})"')


def pinned_snapshot_sha256(tool_source: Path) -> str:
    """The snapshot SHA-256 compiled into the carried ``litellm-snapshot`` tool, or ``""``.

    ``internal/pricing/cmd/litellm-snapshot/main.go`` verifies the restored artifact against this
    constant, so reading it (rather than a second hard-coded copy) is what keeps the committed input
    verifiable against upstream's own pin.
    """
    if not tool_source.exists():
        return ""
    match = SNAPSHOT_PIN.search(tool_source.read_text(encoding="utf-8"))
    return match.group(1) if match else ""


def hashed_path(root: Path, path: str) -> tuple[str, int, bytes]:
    """Hash one carried path the way git stores it.

    Returns ``(blob-sha1, size, committed-bytes)``. A symlink is hashed by its target text (git's
    blob for a link is the target), and a platform that checked the link out as a regular file
    holding that text produces the same bytes, which is why callers can compare digests alone.
    """
    target = root / path
    if target.is_symlink():
        data = os.readlink(target).encode("utf-8")
    else:
        data = target.read_bytes()
    return blob_sha1(data), len(data), data
