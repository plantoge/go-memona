#!/usr/bin/env python3
"""Offline reviewed skill bundles; no discovery, execution, installation or cleanup.

POSIX/Linux, Python standard library only. Inputs must be explicit private staging
files, never a live store or harness directory. See docs/skill-bundles.md.
"""
import argparse
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import unicodedata

import export_skill as exporter

ExportError = exporter.ExportError
require = exporter.require
INPUT_FORMAT = "memona-skill-bundle-input-v1"
FORMAT = "memona-portable-skill-bundle-v1"
MAX_FILES = 32  # All output files, including SKILL.md and manifest.json.
MAX_FILE = 256 * 1024
MAX_TOTAL = 2 * 1024 * 1024  # Rendered SKILL.md plus supporting files.
REFERENCE_SUFFIXES = {".md", ".txt", ".json", ".csv", ".tsv"}
ASSET_SUFFIXES = {".png", ".jpg", ".jpeg", ".gif", ".webp"}
# Lexical guard, not live configuration discovery. Never inspect these trees.
BLOCKED_PARTS = {"data", "notes", ".opencode", ".claude", ".hermes", ".codex",
                 ".config", ".local", ".ssh", ".gnupg"}


def staging_path(path):
    path = exporter.absolute(path)
    require(not any(part.casefold() in BLOCKED_PARTS for part in path.parts),
            "Live data, credential and harness paths are forbidden; use isolated staging.")
    require(path.parts[1:2] not in (("proc",), ("sys",), ("dev",)),
            "System pseudo-filesystems are forbidden.")
    require(not any(unicodedata.category(char) == "Cc" or char == "\\" for char in str(path)), 
            "Unsafe staging path.")
    return path


def private_directory(fd):
    info = os.fstat(fd)
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == os.geteuid()
            and stat.S_IMODE(info.st_mode) & 0o7077 == 0,
            "Staging directories must be private and owned by the current user.")


def component(value):
    return (isinstance(value, str) and len(value) <= 100
            and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", value) is not None
            and not value.endswith(".") and value.casefold() not in BLOCKED_PARTS)


def supporting_path(value):
    require(isinstance(value, str) and len(value) <= 240, "Invalid supporting path.")
    parts = value.split("/")
    require(2 <= len(parts) <= 8 and parts[0] in ("references", "assets")
            and all(component(part) for part in parts), "Unsafe supporting path.")
    suffix = Path(parts[-1]).suffix.lower()
    allowed = REFERENCE_SUFFIXES if parts[0] == "references" else ASSET_SUFFIXES
    require(suffix in allowed, "Only supported reference text and raster assets are allowed.")
    return parts


def parse_manifest(raw):
    try:
        manifest = json.loads(raw.decode("utf-8"), object_pairs_hook=exporter.unique_object,
                              parse_constant=exporter.invalid_constant)
    except (ValueError, UnicodeError, RecursionError):
        raise ExportError("Manifest must be bounded UTF-8 JSON.") from None
    require(isinstance(manifest, dict) and set(manifest) == {"format", "note", "files"}
            and manifest["format"] == INPUT_FORMAT, "Invalid bundle manifest schema.")
    note = manifest["note"]
    require(component(note) and note.lower().endswith(".json")
            and note.casefold() != "manifest.json", "Note must be an explicit staging JSON filename.")
    files = manifest["files"]
    require(isinstance(files, list) and len(files) + 2 <= MAX_FILES,
            "Bundle exceeds the output file count limit.")
    # Track both directories and files so differently cased shared ancestors,
    # prefix conflicts, and duplicate files fail before any supporting read.
    entries = {}
    for value in files:
        parts = supporting_path(value)
        for index in range(1, len(parts) + 1):
            name = "/".join(parts[:index])
            key = name.casefold()
            kind = "file" if index == len(parts) else "directory"
            if key in entries:
                previous, previous_kind = entries[key]
                require(name == previous and kind == previous_kind == "directory",
                        "Duplicate, case collision or file/directory conflict.")
            else:
                entries[key] = (name, kind)
    return manifest


@contextmanager
def child_directory(parent, parts):
    """Pin every child relative to the already pinned private staging root."""
    fd = os.dup(parent)
    try:
        for part in parts:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                            dir_fd=fd)
            os.close(fd)
            fd = child
            private_directory(fd)
        yield fd
    finally:
        os.close(fd)


def read_private(parent, name, limit):
    """Read a basename relative to the pinned staging descriptor, never reopen its path."""
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC,
                 dir_fd=parent)
    try:
        before = os.fstat(fd)
        require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1,
                "Input must be regular with one link.")
        require(before.st_uid == os.geteuid() and stat.S_IMODE(before.st_mode) & 0o7177 == 0,
                "Input must be private, nonexecutable and owned by you.")
        require(before.st_size <= limit, "Input exceeds its byte limit.")
        with os.fdopen(os.dup(fd), "rb") as stream:
            raw = stream.read(limit + 1)
        require(len(raw) <= limit, "Input exceeds its byte limit.")
        require(exporter.fingerprint(before) == exporter.fingerprint(os.fstat(fd)),
                "Input changed during read.")
        return raw
    finally:
        os.close(fd)


def read_supporting(root, name):
    parts = supporting_path(name)
    with child_directory(root, parts[:-1]) as parent:
        raw = read_private(parent, parts[-1], MAX_FILE)
    validate_content(name, raw)
    return raw


def validate_content(name, raw):
    # Reject executable signatures even with an allowed extension. This is not
    # antivirus, a raster decoder or a sanitizer; review remains mandatory.
    require(not raw.startswith((b"#!", b"\x7fELF", b"MZ", b"\xfe\xed\xfa\xce", b"\xce\xfa\xed\xfe",
                                b"\xfe\xed\xfa\xcf", b"\xcf\xfa\xed\xfe", b"\xca\xfe\xba\xbe")),
            "Executable input is forbidden.")
    if name.startswith("references/"):
        try:
            text = raw.decode("utf-8")
        except UnicodeError:
            raise ExportError("References must be UTF-8 text.") from None
        require(all(unicodedata.category(char) != "Cc" or char in "\n\r\t" for char in text),
                "Unsafe control character in reference text.")
    else:
        suffix = Path(name).suffix.lower()
        valid = ((suffix == ".png" and raw.startswith(b"\x89PNG\r\n\x1a\n"))
                 or (suffix in (".jpg", ".jpeg") and raw.startswith(b"\xff\xd8\xff"))
                 or (suffix == ".gif" and raw.startswith((b"GIF87a", b"GIF89a")))
                 or (suffix == ".webp" and raw.startswith(b"RIFF") and raw[8:12] == b"WEBP"))
        require(valid, "Asset signature does not match an allowed raster format.")


def write_new(parent, name, data):
    fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
                 0o600, dir_fd=parent)
    with os.fdopen(fd, "wb") as stream:
        # Only a descriptor just created with O_EXCL is chmodded, never existing files.
        os.fchmod(stream.fileno(), 0o600)
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def make_directory(parent, name):
    os.mkdir(name, 0o700, dir_fd=parent)  # Never reuse any existing child.
    fd = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC,
                 dir_fd=parent)
    try:
        private_directory(fd)
        require(stat.S_IMODE(os.fstat(fd).st_mode) == 0o700,
                "Umask must permit owner access to newly created directories.")
        os.fsync(parent)
        return fd
    except BaseException:
        os.close(fd)
        raise


def export_bundle(source, output, reviewed=False):
    require(reviewed is True, "Explicit --reviewed acknowledgement is required before reads.")
    source, output = staging_path(source), staging_path(output)
    with exporter.directory(source.parent) as root:
        private_directory(root)
        raw_manifest = read_private(root, source.name, exporter.MAX_INPUT)
        spec = parse_manifest(raw_manifest)
        note_path = staging_path(source.parent / spec["note"])
        require(note_path != source, "Manifest and note must be distinct inputs.")
        # Reuse note validation/rendering while keeping all reads on the pinned root.
        raw_note = read_private(root, spec["note"], exporter.MAX_INPUT)
        note = exporter.parse_note(raw_note)
        skill = exporter.render_skill(note)
        require(len(skill) <= MAX_FILE, "Rendered skill exceeds per-file limit.")
        payload = {"SKILL.md": skill}
        total = len(skill)
        for name in spec["files"]:
            data = read_supporting(root, name)
            total += len(data)
            require(total <= MAX_TOTAL, "Bundle exceeds 2 MiB payload limit.")
            payload[name] = data
    manifest = {
        "format": FORMAT, "exporter_version": "1", "reviewed_by_operator": True,
        "native_discovery_verified": False,
        "source": {"id": note["id"], "revision": note["revision"],
                   "input_sha256": hashlib.sha256(raw_note).hexdigest(),
                   "bundle_input_sha256": hashlib.sha256(raw_manifest).hexdigest()},
        "files": {name: {"bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}
                  for name, data in payload.items()},
        "unsupported": ["scripts", "executable files", "installation", "native discovery verification"],
    }
    manifest_data = (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode("utf-8")
    require(len(manifest_data) <= MAX_FILE, "Generated manifest exceeds per-file limit.")
    # The accounting bound includes generated provenance too, not only copied files.
    require(total + len(manifest_data) <= MAX_TOTAL, "Bundle including manifest exceeds 2 MiB.")
    with exporter.directory(output.parent) as parent:
        info = os.fstat(parent)
        require(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) & 0o022 == 0,
                "Output parent must be owned by you and not writable by other users.")
        root = make_directory(parent, output.name)
        directories = {(): root}
        try:
            for name, data in payload.items():
                parts = name.split("/")
                for index in range(1, len(parts)):
                    key = tuple(parts[:index])
                    if key not in directories:
                        directories[key] = make_directory(directories[key[:-1]], key[-1])
                write_new(directories[tuple(parts[:-1])], parts[-1], data)
            for fd in directories.values():
                os.fsync(fd)
            # Last operation on file contents: absence means incomplete; never resume.
            write_new(root, "manifest.json", manifest_data)
            os.fsync(root)
        finally:
            for fd in reversed(list(directories.values())):
                os.close(fd)
    return manifest


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, "Invalid bundle exporter arguments; use --help.\n")


def main(argv=None):
    parser = SafeParser(description=__doc__)
    parser.add_argument("--manifest", required=True, help="Explicit private bundle input JSON in isolated staging")
    parser.add_argument("--output", required=True, help="New directory under an existing trusted parent")
    parser.add_argument("--reviewed", action="store_true", help="Acknowledge review of this exact note, manifest and every file")
    args = parser.parse_args(argv)
    try:
        manifest = export_bundle(args.manifest, args.output, args.reviewed)
    except (ExportError, OSError, ValueError):
        print("Bundle export refused or incomplete. Check review, schema, limits and private nonlinked paths; any partial output is retained.", file=sys.stderr)
        return 1
    print("Portable bundle created; native installation/discovery is not verified. SKILL.md sha256: "
          + manifest["files"]["SKILL.md"]["sha256"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
