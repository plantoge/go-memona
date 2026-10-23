#!/usr/bin/env python3
"""Read-only, bounded source preflight. Never execute a binary or inspect runtime state."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys

# Deliberately not discovery/glob/rglob. Runtime config, environment and data are absent.
SOURCE_FILES = (
    ".gitignore", "go.mod", "go.sum", "Makefile", "frontend/package.json",
    "frontend/package-lock.json", "cmd/memona/main.go", "deploy/memona.service.example",
)
IGNORE_PATTERNS = (
    "/.cache/", "/bin/", "/data/", "/backups/", "/restore-*/", "/config.json",
    "/.env", "/.env.*", "!/.env.example", "*.log", "/frontend/node_modules/",
    "/frontend/.vite/",
)
MAX_SOURCE_BYTES = 4 * 1024 * 1024
MAX_BINARY_BYTES = 512 * 1024 * 1024


class CheckError(Exception):
    pass


def absolute_without_links(path):
    """Reject symlink components without following them, including parent directories."""
    path = Path(os.path.abspath(path))
    current = Path(path.anchor)
    for part in path.parts[1:]:
        current = current / part
        if stat.S_ISLNK(current.lstat().st_mode):
            raise CheckError("symlink_rejected")
    return path


def regular_stat(path):
    path = absolute_without_links(path)
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode):
        raise CheckError("not_regular_file")
    return path, info


def digest_file(path, limit, collect=False):
    path, info = regular_stat(path)
    if info.st_size > limit:
        raise CheckError("size_limit")
    # Linux O_NOFOLLOW plus identity check also rejects replacement of the leaf.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        opened = os.fstat(stream.fileno())
        if not stat.S_ISREG(opened.st_mode) or (info.st_dev, info.st_ino) != (opened.st_dev, opened.st_ino):
            raise CheckError("file_changed")
        digest = hashlib.sha256()
        chunks = []
        total = 0
        while True:
            chunk = stream.read(min(65536, limit + 1 - total))
            if not chunk:
                break
            total += len(chunk)
            if total > limit:
                raise CheckError("size_limit")
            digest.update(chunk)
            if collect:
                chunks.append(chunk)
        end = os.fstat(stream.fileno())
        if (opened.st_size, opened.st_mtime_ns) != (end.st_size, end.st_mtime_ns):
            raise CheckError("file_changed")
    return digest.hexdigest(), b"".join(chunks)


def failure_code(error):
    if isinstance(error, CheckError):
        return error.args[0]
    if isinstance(error, FileNotFoundError):
        return "missing"
    return "unreadable_or_invalid"


def inspect_source(source_root, binary=None):
    checks = []

    def add(name, ok, **metadata):
        checks.append({"check": name, "ok": bool(ok), **metadata})

    try:
        root = absolute_without_links(source_root)
        if not root.is_dir():
            raise CheckError("not_directory")
    except (OSError, CheckError, ValueError) as error:
        add("source_root", False, reason=failure_code(error))
        return {"schema_version": 1, "ok": False, "checks": checks}

    content = {}
    for index, relative in enumerate(SOURCE_FILES):
        # Stable labels, never user-supplied paths or file contents in output.
        label = "source_file_" + str(index + 1)
        try:
            digest, data = digest_file(root / relative, MAX_SOURCE_BYTES, collect=True)
            content[relative] = data.decode("utf-8")
            add(label, True, sha256=digest)
        except (OSError, CheckError, ValueError, UnicodeError) as error:
            add(label, False, reason=failure_code(error))

    lines = set(content.get(".gitignore", "").splitlines())
    add("ignore_required_patterns", all(pattern in lines for pattern in IGNORE_PATTERNS))
    # Exact policy entries only: this is NOT a full Git ignore interpreter or secret scan.
    add("ignore_embedded_assets_not_explicit", not any(
        line.strip() in ("/internal/webui/dist/", "internal/webui/dist/", "/internal/", "*")
        for line in lines))
    go_mod = content.get("go.mod", "")
    add("go_language_directive", bool(re.search(r"(?m)^go 1\.26\.0$", go_mod)))
    # A language directive does not establish the actual Go patch toolchain used.
    add("go_dependency_pins", all(
        re.search(r"(?m)^\s*" + re.escape(module) + r"\s+" + re.escape(version) + r"\s*$", go_mod)
        for module, version in (("github.com/modelcontextprotocol/go-sdk", "v1.8.0"),
                                ("modernc.org/sqlite", "v1.61.0"))))
    try:
        package = json.loads(content.get("frontend/package.json", ""))
        lock = json.loads(content.get("frontend/package-lock.json", ""))
        dependencies = package.get("devDependencies", {})
        pinned = bool(dependencies) and all(
            isinstance(value, str) and re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", value)
            for value in dependencies.values())
        locked = lock.get("packages", {}).get("", {}).get("devDependencies") == dependencies
        add("frontend_exact_pins_and_lock_root", pinned and locked)
    except (ValueError, TypeError, AttributeError, RecursionError):
        add("frontend_exact_pins_and_lock_root", False)
    try:
        _, info = regular_stat(root / "bin/memona")
        add("default_binary_present", info.st_size > 0, executable=bool(info.st_mode & 0o111))
    except (OSError, CheckError, ValueError) as error:
        add("default_binary_present", False, reason=failure_code(error))
    if binary is not None:
        try:
            digest, _ = digest_file(binary, MAX_BINARY_BYTES)
            add("explicit_binary_sha256", True, sha256=digest)
        except (OSError, CheckError, ValueError) as error:
            add("explicit_binary_sha256", False, reason=failure_code(error))
    return {"schema_version": 1, "ok": all(check["ok"] for check in checks), "checks": checks}


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        # argparse's default includes untrusted arguments. Do not echo them.
        print(json.dumps({"schema_version": 1, "ok": False, "error": "invalid_arguments"}))
        raise SystemExit(2)


def main(argv=None):
    parser = SafeParser(description=__doc__)
    parser.add_argument("--source-root", required=True, help="explicit project source root")
    parser.add_argument("--binary", help="optional explicit binary to hash; never executed")
    args = parser.parse_args(argv)
    result = inspect_source(args.source_root, args.binary)
    print(json.dumps(result, sort_keys=True))
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    sys.exit(main())
