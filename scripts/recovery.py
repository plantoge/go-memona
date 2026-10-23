#!/usr/bin/env python3
"""Offline backup/verify/restore. Standard-library only; never starts a service.

All destinations must be new. Failed/incomplete output is retained, not deleted.
Run as the data owner after a normal shutdown. No source files are changed.
"""
import argparse
from contextlib import contextmanager
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import stat
import sys

FORMAT = "memona-offline-backup-v1"
TOOL_VERSION = "1"
MAX_FILES = 100_000
MAX_BYTES = 16 * 1024**3
MAX_MANIFEST = 16 * 1024**2
MAX_DEPTH = 64


class RecoveryError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise RecoveryError(message)


def absolute(path):
    path = Path(path)
    require(".." not in path.parts, "Parent traversal is not allowed.")
    return Path(os.path.abspath(path))


@contextmanager
def directory(path):
    """Pin every component without following any symlink, even in ancestors."""
    path = absolute(path)
    fd = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for part in path.parts[1:]:
            next_fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        yield fd
    finally:
        os.close(fd)


def private(info):
    return info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) & 0o077 == 0


def fingerprint(info):
    return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


@contextmanager
def regular(parent, name, confidential=False):
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent)
    try:
        info = os.fstat(fd)
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1, "Expected a regular file with no hard links.")
        if confidential:
            require(private(info), "Sensitive files must be private and owned by the current user.")
        yield fd
    finally:
        os.close(fd)


def read_small(parent, name, limit=MAX_MANIFEST, confidential=False):
    with regular(parent, name, confidential) as fd:
        before = os.fstat(fd)
        require(before.st_size <= limit, "Metadata exceeds the size limit.")
        with os.fdopen(os.dup(fd), "rb") as stream:
            data = stream.read(limit + 1)
        require(len(data) <= limit, "Metadata exceeds the size limit.")
        require(fingerprint(before) == fingerprint(os.fstat(fd)), "Metadata changed during read.")
        return data


def json_bytes(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()


def write_new(parent, name, data, mode=0o600):
    fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, mode, dir_fd=parent)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


@contextmanager
def new_directory(path):
    path = absolute(path)
    with directory(path.parent) as parent:
        info = os.fstat(parent)
        require(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) & 0o022 == 0,
                "Destination parent must be owned by you and not writable by other users.")
        os.mkdir(path.name, 0o700, dir_fd=parent)
        os.fsync(parent)
        fd = os.open(path.name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
        try:
            yield fd
            os.fsync(fd)
        finally:
            os.close(fd)


def scan(root):
    """Return a bounded inventory, rejecting links/devices/nonprivate entries."""
    entries = {}
    total = 0

    def walk(fd, prefix, depth=0):
        nonlocal total
        require(depth <= MAX_DEPTH, "Directory nesting exceeds the depth limit.")
        require(private(os.fstat(fd)), "Backup/data directories must be private and owned by you.")
        for name in sorted(os.listdir(fd)):
            rel = f"{prefix}/{name}" if prefix else name
            info = os.stat(name, dir_fd=fd, follow_symlinks=False)
            require(private(info), "Backup/data entries must be private and owned by you.")
            require(len(entries) < MAX_FILES, "Too many backup entries.")
            if stat.S_ISDIR(info.st_mode):
                entries[rel] = {"type": "directory"}
                child = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=fd)
                try:
                    walk(child, rel, depth + 1)
                finally:
                    os.close(child)
            else:
                with regular(fd, name, confidential=True) as file_fd:
                    before = os.fstat(file_fd)
                    require(fingerprint(info) == fingerprint(before), "Source changed during inspection.")
                    total += before.st_size
                    require(total <= MAX_BYTES, "Backup exceeds the byte limit.")
                    digest = hashlib.sha256()
                    size = 0
                    with os.fdopen(os.dup(file_fd), "rb") as stream:
                        while chunk := stream.read(1024 * 1024):
                            size += len(chunk)
                            require(size <= before.st_size, "File grew during hashing.")
                            digest.update(chunk)
                    require(size == before.st_size, "File changed during hashing.")
                    require(fingerprint(before) == fingerprint(os.fstat(file_fd)), "Source changed during hashing.")
                    entries[rel] = {"type": "file", "size": before.st_size, "sha256": digest.hexdigest()}
    walk(root, "")
    return entries


def copy_file(source, name, target, dest, mode=0o600, budget=None):
    with regular(source, name) as fd:
        before = os.fstat(fd)
        require(before.st_size <= MAX_BYTES, "File exceeds the byte limit.")
        out = os.open(dest, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, mode, dir_fd=target)
        digest, size = hashlib.sha256(), 0
        with os.fdopen(out, "wb") as writer, os.fdopen(os.dup(fd), "rb") as reader:
            while chunk := reader.read(1024 * 1024):
                size += len(chunk)
                require(size <= before.st_size, "File grew during copy.")
                if budget is not None:
                    budget["bytes"] += len(chunk)
                    require(budget["bytes"] <= MAX_BYTES, "Copy exceeds the byte limit.")
                writer.write(chunk)
                digest.update(chunk)
            writer.flush()
            os.fsync(writer.fileno())
        require(size == before.st_size and fingerprint(before) == fingerprint(os.fstat(fd)), "Source changed during copy.")
        return {"type": "file", "size": size, "sha256": digest.hexdigest()}


def copy_tree(source, target, budget=None, depth=0):
    if budget is None:
        budget = {"entries": 0, "bytes": 0}
    require(depth <= MAX_DEPTH, "Directory nesting exceeds the depth limit.")
    require(private(os.fstat(source)), "Source directory must be private and owned by you.")
    for name in sorted(os.listdir(source)):
        budget["entries"] += 1
        require(budget["entries"] <= MAX_FILES, "Copy exceeds the entry limit.")
        info = os.stat(name, dir_fd=source, follow_symlinks=False)
        require(private(info), "Source entries must be private and owned by you.")
        if stat.S_ISDIR(info.st_mode):
            os.mkdir(name, 0o700, dir_fd=target)
            src = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=source)
            dst = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=target)
            try:
                copy_tree(src, dst, budget, depth + 1)
            finally:
                os.close(src)
                os.close(dst)
        else:
            copy_file(source, name, target, name, budget=budget)
    os.fsync(target)


@contextmanager
def offline(data):
    # Open the application's EXISTING lock read-only: never create/chmod it.
    with regular(data, ".writer.lock", confidential=True) as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RecoveryError("Writer is active. Stop Memona normally before backup; do not remove its lock.") from None
        yield


def backup(config, binary, output, stopped=False):
    require(stopped, "Explicit --stopped acknowledgement is required after normal shutdown.")
    config, binary, output = map(absolute, (config, binary, output))
    with directory(config.parent) as config_parent:
        raw = read_small(config_parent, config.name, 1024 * 1024)
    cfg = json.loads(raw)
    require(isinstance(cfg, dict) and isinstance(cfg.get("data_dir"), str) and cfg["data_dir"], "Config must specify data_dir.")
    data_path = absolute(cfg["data_dir"])
    require(output != data_path and data_path not in output.parents, "Backup destination must be outside data_dir.")
    with directory(data_path) as data, offline(data):
        before = scan(data)
        require(before.get("memona.db", {}).get("type") == "file" and before.get("notes", {}).get("type") == "directory", "Not a complete Memona data directory.")
        with new_directory(output) as dest:
            os.mkdir("data", 0o700, dir_fd=dest)
            copied = os.open("data", os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=dest)
            try:
                copy_tree(data, copied)
                require(scan(copied) == before and scan(data) == before, "Data changed during backup; incomplete output retained.")
            finally:
                os.close(copied)
            write_new(dest, "config.original.json", raw)
            with directory(binary.parent) as parent:
                copy_file(parent, binary.name, dest, "memona")
            project = Path(__file__).resolve().parents[1]
            with directory(project) as parent:
                for name in ("go.mod", "go.sum"):
                    copy_file(parent, name, dest, name)
            entries = scan(dest)
            manifest = {"format": FORMAT, "tool_version": TOOL_VERSION,
                        "created_at": datetime.now(timezone.utc).isoformat(),
                        "platform": platform.system(), "machine": platform.machine(),
                        "data_format": "source-bytes-unmodified",
                        "consistency": "offline-exclusive-writer-lock",
                        "entries": entries}
            encoded = json_bytes(manifest)
            require(len(encoded) <= MAX_MANIFEST, "Manifest exceeds the size limit.")
            # Manifest is the completion marker, written last. It includes binary
            # and module hashes; do not confuse this with signed authenticity.
            write_new(dest, "manifest.json", encoded)
            os.fsync(dest)
            verified(dest)
    return hashlib.sha256(encoded).hexdigest()


def valid_manifest(raw):
    manifest = json.loads(raw)
    require(isinstance(manifest, dict) and manifest.get("format") == FORMAT, "Unknown backup format.")
    entries = manifest.get("entries")
    require(isinstance(entries, dict) and len(entries) <= MAX_FILES, "Invalid manifest inventory.")
    for name in ("data", "data/notes", "data/.writer.lock", "data/memona.db", "config.original.json", "memona", "go.mod", "go.sum"):
        require(name in entries, "Backup is missing a required entry.")
    for name, info in entries.items():
        require(isinstance(name, str) and isinstance(info, dict), "Invalid manifest entry.")
        path = PurePosixPath(name)
        require(name == str(path) and not path.is_absolute() and ".." not in path.parts and name not in (".", "manifest.json"), "Unsafe manifest path.")
        require(name in ("data", "config.original.json", "memona", "go.mod", "go.sum") or name.startswith("data/"), "Unexpected backup entry.")
        expected_type = "directory" if name in ("data", "data/notes") else None
        require(info.get("type") in ("directory", "file") and (expected_type is None or info["type"] == expected_type), "Invalid manifest entry type.")
        if info["type"] == "file":
            size, digest = info.get("size"), info.get("sha256")
            require(type(size) is int and 0 <= size <= MAX_BYTES and isinstance(digest, str) and len(digest) == 64 and all(c in "0123456789abcdef" for c in digest), "Invalid file size or checksum.")
    for name in ("data/.writer.lock", "data/memona.db", "config.original.json", "memona", "go.mod", "go.sum"):
        require(entries[name].get("type") == "file", "Required file has an invalid type.")
    return manifest


def verified(root, expected_hash=None):
    raw = read_small(root, "manifest.json", confidential=True)
    if expected_hash is not None:
        require(hashlib.sha256(raw).hexdigest() == expected_hash, "Manifest checksum mismatch.")
    manifest = valid_manifest(raw)
    inventory = scan(root)
    inventory.pop("manifest.json", None)
    require(inventory == manifest["entries"], "Backup checksum/inventory mismatch; restore refused.")
    return manifest


def verify(bundle, expected_hash=None):
    with directory(bundle) as root:
        verified(root, expected_hash)


def restore(bundle, output, port, expected_hash=None):
    bundle, output = absolute(bundle), absolute(output)
    require(1024 <= port <= 65535, "Use an unprivileged loopback port (1024–65535).")
    require(output != bundle and bundle not in output.parents, "Restore destination must be outside the backup.")
    with directory(bundle) as source:
        # Verify every checksum/path/type before creating any restore output.
        manifest = verified(source, expected_hash)
        with new_directory(output) as dest:
            copy_tree(source, dest)
            verified(dest, expected_hash)
            require(verified(source, expected_hash) == manifest, "Backup changed during restore.")
            # Do not reuse the original config: it may point at production data.
            # Never execute the copied binary automatically. Owner grants execute
            # permission separately only after reviewing the binary provenance.
            cfg = {"listen": f"127.0.0.1:{port}", "public_url": f"http://127.0.0.1:{port}",
                   "data_dir": str(output / "data"), "secure_cookies": False}
            write_new(dest, "config.restore.json", json_bytes(cfg))
            os.fsync(dest)
    return output / "config.restore.json"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    create = sub.add_parser("backup", help="Offline backup to a NEW private directory")
    create.add_argument("--config", required=True)
    create.add_argument("--binary", required=True, help="Exact reviewed binary corresponding to this data format")
    create.add_argument("--output", required=True)
    create.add_argument("--stopped", action="store_true", help="Acknowledge normal shutdown; writer lock is still checked")
    for command in ("verify", "restore"):
        p = sub.add_parser(command)
        p.add_argument("--backup", required=True)
        p.add_argument("--manifest-sha256", help="Trusted digest recorded separately at backup time")
        if command == "restore":
            p.add_argument("--output", required=True, help="NEW directory only; existing paths are refused")
            p.add_argument("--port", required=True, type=int, help="Alternative local port for the isolated restore")
    args = parser.parse_args()
    try:
        if args.command == "backup":
            digest = backup(args.config, args.binary, args.output, args.stopped)
            print("Backup complete. Keep this manifest SHA-256 separately:", digest)
        elif args.command == "verify":
            verify(args.backup, args.manifest_sha256)
            print("PASS: backup inventory and SHA-256 checksums verified (not a signature or semantic integrity check).")
        else:
            restore(args.backup, args.output, args.port, args.manifest_sha256)
            print("Restore copy verified. Review config.restore.json; no binary executed or service started.")
            print("WARNING: credentials are restored to backup time. Reconcile later revocations before any cutover.")
    except (RecoveryError, OSError, ValueError, RecursionError):
        # Do not print source content, config, filenames controlled by data, or
        # secrets embedded in JSON decoder/OS exceptions.
        print("Recovery refused or failed. Check paths, private permissions, writer shutdown, inventory/checksums and disk space. Any partial destination is retained; never reuse or auto-delete it.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
