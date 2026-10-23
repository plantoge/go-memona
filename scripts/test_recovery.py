"""Synthetic tests; fixture directories are retained under project .cache/tmp."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import recovery

ROOT = Path(__file__).resolve().parents[1]
TMP = ROOT / ".cache" / "tmp"


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        TMP.mkdir(parents=True, exist_ok=True)
        self.work = Path(tempfile.mkdtemp(prefix="recovery-unit-", dir=TMP))
        self.data = self.work / "source"
        self.data.mkdir(mode=0o700)
        (self.data / "notes").mkdir(mode=0o700)
        (self.data / ".staging").mkdir(mode=0o700)
        for name, content in {".writer.lock": b"", "memona.db": b"synthetic-db", "memona.db-wal": b"wal", "memona.db-shm": b"shm", ".staging/retained": b"partial"}.items():
            self.file(self.data / name, content)
        self.config = self.work / "config.json"
        self.file(self.config, json.dumps({"data_dir": str(self.data), "public_url": "https://example.invalid"}).encode())
        self.binary = self.work / "binary"
        self.file(self.binary, b"not-executable-synthetic-fixture")
        self.bundle = self.work / "backup"

    def file(self, path, content):
        with open(path, "xb") as stream:
            stream.write(content)
        os.chmod(path, 0o600)

    def backup(self):
        return recovery.backup(self.config, self.binary, self.bundle, stopped=True)

    def test_backup_verify_restore_preserves_all_and_isolates_config(self):
        digest = self.backup()
        recovery.verify(self.bundle, digest)
        dest = self.work / "restored"
        cfg_path = recovery.restore(self.bundle, dest, 9876, digest)
        cfg = json.loads(cfg_path.read_bytes())
        self.assertEqual(cfg["data_dir"], str(dest / "data"))
        self.assertEqual(cfg["public_url"], "http://127.0.0.1:9876")
        self.assertEqual(cfg["listen"], "127.0.0.1:9876")
        self.assertFalse(cfg["secure_cookies"])
        with recovery.directory(self.data) as src, recovery.directory(dest / "data") as target:
            self.assertEqual(recovery.scan(src), recovery.scan(target))
        self.assertEqual((dest / "config.original.json").read_bytes(), self.config.read_bytes())
        self.assertEqual(stat.S_IMODE((dest / "memona").stat().st_mode), 0o600)
        for p in [dest, *dest.rglob("*")]:
            self.assertEqual(stat.S_IMODE(p.stat().st_mode) & 0o077, 0)
        manifest = json.loads((self.bundle / "manifest.json").read_bytes())
        self.assertIn("data/memona.db-wal", manifest["entries"])
        self.assertIn("data/memona.db-shm", manifest["entries"])
        self.assertIn("data/.staging/retained", manifest["entries"])
        self.assertEqual(manifest["entries"]["memona"]["sha256"], hashlib.sha256(self.binary.read_bytes()).hexdigest())

    def test_stopped_ack_and_active_writer(self):
        with self.assertRaises(recovery.RecoveryError):
            recovery.backup(self.config, self.binary, self.bundle)
        with open(self.data / ".writer.lock", "rb") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaises(recovery.RecoveryError):
                self.backup()
        self.assertFalse(self.bundle.exists())

    def test_existing_paths_never_overwritten(self):
        digest = self.backup()
        original = (self.bundle / "manifest.json").read_bytes()
        with self.assertRaises(FileExistsError):
            self.backup()
        self.assertEqual((self.bundle / "manifest.json").read_bytes(), original)
        marker = self.data / "user-work"
        self.file(marker, b"keep")
        with self.assertRaises(FileExistsError):
            recovery.restore(self.bundle, self.data, 9876, digest)
        self.assertEqual(marker.read_bytes(), b"keep")

    def test_tamper_refuses_before_restore_creation(self):
        digest = self.backup()
        (self.bundle / "data" / "memona.db").write_bytes(b"changed")
        with self.assertRaises(recovery.RecoveryError):
            recovery.verify(self.bundle, digest)
        dest = self.work / "bad-restore"
        with self.assertRaises(recovery.RecoveryError):
            recovery.restore(self.bundle, dest, 9876, digest)
        self.assertFalse(dest.exists())

    def test_manifest_checksum_detects_rewritten_inventory(self):
        digest = self.backup()
        manifest = self.bundle / "manifest.json"
        data = json.loads(manifest.read_bytes())
        data["created_at"] = "altered"
        manifest.write_bytes(recovery.json_bytes(data))
        with self.assertRaises(recovery.RecoveryError):
            recovery.verify(self.bundle, digest)

    def test_unlisted_file(self):
        self.backup()
        self.file(self.bundle / "extra", b"unexpected")
        with self.assertRaises(recovery.RecoveryError):
            recovery.verify(self.bundle)

    def test_missing_file_refused_without_cleanup(self):
        self.backup()
        os.rename(self.bundle / "data" / "memona.db", self.work / "retained-db")
        with self.assertRaises(recovery.RecoveryError):
            recovery.verify(self.bundle)
        self.assertTrue((self.work / "retained-db").exists())

    def test_cli_reports_no_secret_and_retains_partial_output(self):
        secret = "SYNTHETIC-DO-NOT-PRINT"
        malformed = self.work / "malformed.json"
        self.file(malformed, ('{"data_dir": "' + secret).encode())
        cmd = [sys.executable, "-B", str(ROOT / "scripts" / "recovery.py"), "backup", "--stopped",
               "--config", str(malformed), "--binary", str(self.binary), "--output", str(self.bundle)]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(secret, result.stdout + result.stderr)
        self.assertFalse(self.bundle.exists())
        missing_binary = self.work / "not-existing"
        with self.assertRaises(FileNotFoundError):
            recovery.backup(self.config, missing_binary, self.bundle, True)
        self.assertTrue((self.bundle / "data" / "memona.db").exists())
        self.assertFalse((self.bundle / "manifest.json").exists())
        with self.assertRaises(FileNotFoundError):
            recovery.verify(self.bundle)

    def test_symlink_refused(self):
        (self.data / "symlink").symlink_to(self.binary)
        with self.assertRaises((recovery.RecoveryError, OSError)):
            self.backup()
        self.assertFalse(self.bundle.exists())

    def test_hardlink_refused(self):
        os.link(self.binary, self.data / "hardlink")
        with self.assertRaises(recovery.RecoveryError):
            self.backup()

    def test_fifo_refused_without_blocking(self):
        os.mkfifo(self.data / "pipe", 0o600)
        with self.assertRaises(recovery.RecoveryError):
            self.backup()

    def test_symlink_parent_and_traversal_refused(self):
        alias = self.work / "alias"
        alias.symlink_to(self.data, target_is_directory=True)
        with self.assertRaises(OSError):
            with recovery.directory(alias / "notes"):
                pass
        with self.assertRaises(recovery.RecoveryError):
            recovery.absolute(self.work / "x" / ".." / "source")

    def test_public_data_refused_without_chmod(self):
        os.chmod(self.data / "memona.db", 0o644)
        with self.assertRaises(recovery.RecoveryError):
            self.backup()
        self.assertEqual(stat.S_IMODE((self.data / "memona.db").stat().st_mode), 0o644)

    def test_backup_inside_source_and_privileged_port_refused(self):
        with self.assertRaises(recovery.RecoveryError):
            recovery.backup(self.config, self.binary, self.data / "backup", True)
        digest = self.backup()
        with self.assertRaises(recovery.RecoveryError):
            recovery.restore(self.bundle, self.work / "restore", 80, digest)

    def test_malicious_manifest_paths(self):
        self.backup()
        original = json.loads((self.bundle / "manifest.json").read_bytes())
        for name in ("../escape", "/absolute", "data/../escape", "data//bad", "manifest.json"):
            data = dict(original)
            data["entries"] = dict(original["entries"])
            data["entries"][name] = {"type": "file", "size": 0, "sha256": "0" * 64}
            with self.assertRaises(recovery.RecoveryError):
                recovery.valid_manifest(recovery.json_bytes(data))

    def test_missing_lock_not_created(self):
        other = self.work / "missing-lock"
        other.mkdir(mode=0o700)
        with recovery.directory(other) as fd:
            with self.assertRaises(FileNotFoundError):
                with recovery.offline(fd):
                    pass
        self.assertFalse((other / ".writer.lock").exists())

    def test_cli_round_trip_and_tamper_exit_status(self):
        base = [sys.executable, "-B", str(ROOT / "scripts" / "recovery.py")]
        result = subprocess.run([*base, "backup", "--stopped", "--config", str(self.config),
                                 "--binary", str(self.binary), "--output", str(self.bundle)],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        digest = result.stdout.strip().split()[-1]
        self.assertEqual(len(digest), 64)
        result = subprocess.run([*base, "verify", "--backup", str(self.bundle), "--manifest-sha256", digest],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        target = self.work / "cli-restore"
        result = subprocess.run([*base, "restore", "--backup", str(self.bundle), "--manifest-sha256", digest,
                                 "--output", str(target), "--port", "9876"], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((target / "config.restore.json").exists())
        self.assertIn("credentials", result.stdout)
        (self.bundle / "data" / "memona.db").write_bytes(b"tamper")
        result = subprocess.run([*base, "verify", "--backup", str(self.bundle)], capture_output=True, text=True, timeout=10)
        self.assertNotEqual(result.returncode, 0)

    def test_inventory_and_depth_limits(self):
        with recovery.directory(self.data) as fd:
            for name, value in (("MAX_BYTES", 1), ("MAX_FILES", 1), ("MAX_DEPTH", 0)):
                with self.subTest(name=name), patch.object(recovery, name, value):
                    with self.assertRaises(recovery.RecoveryError):
                        recovery.scan(fd)
        self.assertFalse(self.bundle.exists())

    def test_manifest_required_file_types(self):
        self.backup()
        manifest = json.loads((self.bundle / "manifest.json").read_bytes())
        for name in ("data", "data/notes", "memona", "data/memona.db"):
            data = dict(manifest)
            data["entries"] = dict(manifest["entries"])
            data["entries"][name] = {"type": "file", "size": 0, "sha256": "0" * 64} if name in ("data", "data/notes") else {"type": "directory"}
            with self.assertRaises(recovery.RecoveryError):
                recovery.valid_manifest(recovery.json_bytes(data))


if __name__ == "__main__":
    unittest.main()
