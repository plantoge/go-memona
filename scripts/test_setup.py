import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("memona_setup", Path(__file__).with_name("setup.py"))
setup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(setup)


class SetupTest(unittest.TestCase):
    def test_first_setup_creates_private_config_and_passes_password_only_in_environment(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            (root / "config.example.json").write_text('{"data_dir":"./data"}')
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.getpass, "getpass", side_effect=["a-strong-admin-password", "a-strong-admin-password"]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                setup.setup()
                path, argv, env = execve.call_args.args
                self.assertEqual(path, binary)
                self.assertEqual(argv, [str(binary), "serve", "-config", str(root / "config.json")])
                self.assertEqual(env["MEMONA_ADMIN_PASSWORD"], "a-strong-admin-password")
                self.assertNotIn("a-strong-admin-password", " ".join(argv))
            self.assertEqual((root / "config.json").stat().st_mode & 0o777, 0o600)
            self.assertFalse((root / "data").exists())

    def test_existing_data_never_resets_admin(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            (root / "config.example.json").write_text('{"data_dir":"./data"}')
            (root / "data").mkdir()
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.os, "execve") as execve:
                with self.assertRaisesRegex(ValueError, "Data sudah ada"):
                    setup.setup()
                execve.assert_not_called()

    def test_noninteractive_never_executes_binary(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            (root / "config.example.json").write_text('{"data_dir":"./data"}')
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=False), patch.object(setup.os, "execve") as execve:
                with self.assertRaisesRegex(ValueError, "terminal interaktif"):
                    setup.setup()
                execve.assert_not_called()
            self.assertFalse((root / "data").exists())

    def test_mismatch_never_bootstraps(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            (root / "config.example.json").write_text('{"data_dir":"./data"}')
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.getpass, "getpass", side_effect=["password-cukup-panjang", "salah"]), patch.object(setup.os, "execve") as execve:
                with self.assertRaisesRegex(ValueError, "Password tidak cocok"):
                    setup.setup()
                execve.assert_not_called()


if __name__ == "__main__":
    unittest.main()
