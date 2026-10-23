#!/usr/bin/env python3
"""Offline, stdlib-only export of one explicitly reviewed JSON skill note.

Never reads the live Markdown store, installs a skill, or executes note contents.
POSIX/Linux only. Failed partial output is retained; nothing is deleted.
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

MAX_INPUT = 128 * 1024
MAX_CONTENT = 64 * 1024
FORMAT = "memona-portable-skill-v1"


class ExportError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise ExportError(message)


def absolute(path):
    path = Path(path)
    require(".." not in path.parts, "Parent traversal is not allowed.")
    return Path(os.path.abspath(path))


@contextmanager
def directory(path):
    """Pin each ancestor using directory descriptors; never follow symlinks."""
    path = absolute(path)
    fd = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for part in path.parts[1:]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=fd)
            os.close(fd)
            fd = child
        yield fd
    finally:
        os.close(fd)


def fingerprint(info):
    return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns,
            info.st_uid, info.st_mode, info.st_nlink)


def read_input(path):
    path = absolute(path)
    with directory(path.parent) as parent:
        fd = os.open(path.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent)
        try:
            before = os.fstat(fd)
            require(stat.S_ISREG(before.st_mode) and before.st_nlink == 1,
                    "Input must be a regular file with a single link.")
            require(before.st_uid == os.geteuid() and stat.S_IMODE(before.st_mode) & 0o077 == 0,
                    "Input must be private and owned by the current user.")
            require(before.st_size <= MAX_INPUT, "Input exceeds 128 KiB.")
            with os.fdopen(os.dup(fd), "rb") as stream:
                raw = stream.read(MAX_INPUT + 1)
            require(len(raw) <= MAX_INPUT, "Input exceeds 128 KiB.")
            require(fingerprint(before) == fingerprint(os.fstat(fd)), "Input changed during read.")
            return raw
        finally:
            os.close(fd)


def unique_object(pairs):
    obj = {}
    for key, value in pairs:
        require(key not in obj, "Duplicate JSON keys are not allowed.")
        obj[key] = value
    return obj


def invalid_constant(value):
    raise ExportError("Nonstandard JSON constants are not allowed.")


def text(value, limit, multiline=False):
    if not isinstance(value, str) or not value.strip():
        return False
    try:
        size = len(value.encode("utf-8"))
    except UnicodeEncodeError:
        return False
    return size <= limit and all(
        (ord(char) >= 32 and ord(char) != 127) or (multiline and char in "\n\t")
        for char in value
    )


def parse_note(raw):
    try:
        note = json.loads(raw.decode("utf-8"), object_pairs_hook=unique_object,
                          parse_constant=invalid_constant)
    except (ValueError, UnicodeError, RecursionError):
        raise ExportError("Input must be bounded UTF-8 JSON, not Markdown or an API envelope.") from None
    require(isinstance(note, dict), "Input must be one JSON note object.")
    require(note.get("kind") == "skill" and note.get("status") == "active"
            and note.get("archived") is False,
            "Only explicitly active, nonarchived skill notes can be exported.")
    require(isinstance(note.get("id"), str) and re.fullmatch(r"[0-9a-f]{32}", note["id"]),
            "Invalid Memona note ID.")
    require(type(note.get("revision")) is int and 1 <= note["revision"] <= 999999,
            "Invalid source revision.")
    require(text(note.get("title"), 200), "Title must be nonempty single-line text, at most 200 UTF-8 bytes.")
    require(text(note.get("content"), MAX_CONTENT, multiline=True),
            "Content must be nonempty text, at most 64 KiB, without unsafe control characters.")
    require(not any(key in note for key in ("scripts", "references", "files", "assets")),
            "References, scripts, assets and file bundles are unsupported; supply a text-only note.")
    metadata = note.get("skill_metadata")
    if metadata is not None:
        require(isinstance(metadata, dict) and not set(metadata) - {"trigger", "version", "compatibility", "test_evidence"},
                "Invalid skill metadata fields.")
        for key, limit in (("trigger", 1000), ("version", 100), ("test_evidence", 2000)):
            value = metadata.get(key, "")
            require(isinstance(value, str) and (value == "" or text(value, limit))
                    and not any(unicodedata.category(char) == "Cc" for char in value), "Invalid skill metadata text.")
        compatibility = metadata.get("compatibility", [])
        require(isinstance(compatibility, list) and len(compatibility) <= 8
                and all(text(item, 200) and not any(unicodedata.category(char) == "Cc" for char in item)
                        for item in compatibility), "Invalid skill compatibility list.")
        require(sum(len(metadata.get(key, "").encode("utf-8")) for key in ("trigger", "version", "test_evidence"))
                + sum(len(item.encode("utf-8")) for item in compatibility) <= 4000, "Skill metadata exceeds total bound.")
    return note


def render_skill(note):
    title = note["title"].strip()
    ascii_title = unicodedata.normalize("NFKD", title).encode("ascii", "ignore").decode("ascii").lower()
    slug = re.sub(r"[^a-z0-9]+", "-", ascii_title).strip("-")[:48].rstrip("-") or "skill"
    suffix = hashlib.sha256(title.encode("utf-8")).hexdigest()[:10]
    name = f"{slug}-{suffix}"
    # JSON double-quoted strings are YAML-compatible scalars. No raw title is
    # interpolated into frontmatter; note content is copied only after its end.
    trigger = (note.get("skill_metadata") or {}).get("trigger", "").strip()
    description = json.dumps(trigger or f"Prosedur yang ditinjau operator: {title}", ensure_ascii=True)
    header = f'---\nname: "{name}"\ndescription: {description}\n---\n\n'
    return (header + note["content"]).encode("utf-8")


def write_new(parent, name, data):
    fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC,
                 0o600, dir_fd=parent)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def export_skill(source, output, reviewed=False):
    require(reviewed is True, "Explicit --reviewed acknowledgement is required.")
    source, output = absolute(source), absolute(output)
    raw = read_input(source)
    note = parse_note(raw)
    skill = render_skill(note)
    manifest = {
        "format": FORMAT,
        "exporter_version": "1",
        "reviewed_by_operator": True,
        "native_discovery_verified": False,
        "source": {"id": note["id"], "revision": note["revision"],
                   "input_sha256": hashlib.sha256(raw).hexdigest()},
        "files": {"SKILL.md": {"bytes": len(skill), "sha256": hashlib.sha256(skill).hexdigest()}},
        "unsupported": ["references", "scripts", "assets"],
    }
    if note.get("skill_metadata"):
        manifest["declared_skill_metadata"] = note["skill_metadata"]
    data = (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode("utf-8")
    with directory(output.parent) as parent:
        info = os.fstat(parent)
        require(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) & 0o022 == 0,
                "Output parent must be owned by you and not writable by other users.")
        # mkdir fails even for an existing empty directory or dangling symlink.
        os.mkdir(output.name, 0o700, dir_fd=parent)
        os.fsync(parent)
        fd = os.open(output.name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
        try:
            require(stat.S_IMODE(os.fstat(fd).st_mode) & 0o077 == 0, "Output directory must be private.")
            write_new(fd, "SKILL.md", skill)
            # Manifest is last: failed/incomplete exports are retained for review.
            write_new(fd, "manifest.json", data)
            os.fsync(fd)
        finally:
            os.close(fd)
    return manifest


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, help="Explicit private JSON note file; never a live store path")
    parser.add_argument("--output", required=True, help="New export directory under an existing trusted parent")
    parser.add_argument("--reviewed", action="store_true", help="Acknowledge review of this exact note/revision and its untrusted body")
    args = parser.parse_args(argv)
    try:
        manifest = export_skill(args.input, args.output, args.reviewed)
    except (ExportError, OSError):
        # Never echo arbitrary paths, titles, bodies or OS error filenames.
        print("Export refused or incomplete. Check review, note validity, limits and private nonlinked paths; any partial output is retained.", file=sys.stderr)
        return 1
    print("Portable export created; native installation/discovery is not verified. SKILL.md sha256: "
          + manifest["files"]["SKILL.md"]["sha256"])
    return 0


if __name__ == "__main__":
    sys.exit(main())
