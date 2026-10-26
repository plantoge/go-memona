import importlib.util
import json
from pathlib import Path
import sqlite3
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
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch("builtins.input", return_value=""), patch.object(setup.getpass, "getpass", side_effect=["a-strong-admin-password", "a-strong-admin-password"]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                setup.setup()
                path, argv, env = execve.call_args.args
                self.assertEqual(path, binary)
                self.assertEqual(argv, [str(binary), "serve", "-config", str(root / "config.json")])
                self.assertEqual(env["MEMONA_ADMIN_PASSWORD"], "a-strong-admin-password")
                self.assertNotIn("a-strong-admin-password", " ".join(argv))
            self.assertEqual((root / "config.json").stat().st_mode & 0o777, 0o600)
            self.assertEqual(json.loads((root / "config.json").read_text())["listen"], "127.0.0.1:8765")
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

    def test_failed_bootstrap_with_empty_admin_can_retry_without_deleting_data(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            data = root / "data"
            data.mkdir(mode=0o700)
            database = data / "memona.db"
            with sqlite3.connect(database) as connection:
                connection.execute("CREATE TABLE admin (id INTEGER PRIMARY KEY, salt TEXT, hash TEXT)")
            database.chmod(0o600)
            (data / ".writer.lock").write_bytes(b"")
            config = root / "config.json"
            original = '{"data_dir":"./data","listen":"192.168.1.6:8765","public_url":"http://192.168.1.6:8765","allow_private_http":true}'
            config.write_text(original)
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.getpass, "getpass", side_effect=["1234567890", "1234567890"]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                setup.setup()
                execve.assert_called_once()
                self.assertEqual(execve.call_args.args[2]["MEMONA_ADMIN_PASSWORD"], "1234567890")
            self.assertEqual(config.read_text(), original)
            self.assertTrue(database.exists())

    def test_existing_admin_or_unknown_database_blocks_retry(self):
        for condition in ("admin", "missing database", "missing table", "busy"):
            with self.subTest(condition=condition), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "bin").mkdir()
                binary = root / "bin" / "memona"
                binary.write_bytes(b"fixture")
                binary.chmod(0o700)
                data = root / "data"
                data.mkdir(mode=0o700)
                if condition != "missing database":
                    database = data / "memona.db"
                    with sqlite3.connect(database) as connection:
                        if condition != "missing table":
                            connection.execute("CREATE TABLE admin (id INTEGER PRIMARY KEY)")
                            if condition == "admin":
                                connection.execute("INSERT INTO admin VALUES (1)")
                    database.chmod(0o600)
                (data / ".writer.lock").write_bytes(b"")
                with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.local, "writer_busy", return_value=condition == "busy"), patch.object(setup.getpass, "getpass") as password, patch.object(setup.os, "execve") as execve:
                    with self.assertRaisesRegex(ValueError, "Data sudah ada"):
                        setup.setup()
                    password.assert_not_called()
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
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch("builtins.input", return_value=""), patch.object(setup.getpass, "getpass", side_effect=["password-cukup-panjang", "salah"]), patch.object(setup.os, "execve") as execve:
                with self.assertRaisesRegex(ValueError, "Password tidak cocok"):
                    setup.setup()
                execve.assert_not_called()
            self.assertFalse((root / "config.json").exists())

    def test_password_length_matches_go_bootstrap(self):
        for password, valid in [("123456789", False), ("1234567890", True),
                                ("x" * 1024, True), ("x" * 1025, False)]:
            with self.subTest(length=len(password)), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "bin").mkdir()
                binary = root / "bin" / "memona"
                binary.write_bytes(b"fixture")
                binary.chmod(0o700)
                with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch("builtins.input", return_value=""), patch.object(setup.getpass, "getpass", side_effect=[password, password]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                    if valid:
                        setup.setup()
                        execve.assert_called_once()
                    else:
                        with self.assertRaisesRegex(ValueError, "10–1024 byte"):
                            setup.setup()
                        execve.assert_not_called()
                        self.assertFalse((root / "config.json").exists())

    def test_network_options_and_rejection(self):
        cases = [
            (["2", "192.168.1.10"], "http://192.168.1.10:8765", False),
            (["2", "100.76.35.38"], "http://100.76.35.38:8765", False),
            (["3", "https://memona.example.com"], "https://memona.example.com", True),
        ]
        for answers, expected, secure in cases:
            with self.subTest(answers=answers), patch("builtins.input", side_effect=answers):
                config = setup.network_config()
                self.assertEqual(config["public_url"], expected)
                self.assertEqual(config["secure_cookies"], secure)
                self.assertEqual(config["listen"], "127.0.0.1:8765" if secure else expected.removeprefix("http://"))
        for answers in (["2", "8.8.8.8"], ["2", "127.0.0.1"], ["2", "240.0.0.1"], ["2", "198.18.0.1"],
                        ["3", "http://example.com"], ["3", "https://example.com/mcp"],
                        ["3", "https://example.com:wrong"], ["3", "https://user@example.com"], ["4"]):
            with self.subTest(rejected=answers), patch("builtins.input", side_effect=answers):
                with self.assertRaises(ValueError):
                    setup.network_config()

    def test_setup_writes_network_choice_to_new_config(self):
        for answers, expected in [(["2", "192.168.1.10"], "http://192.168.1.10:8765"),
                                  (["3", "https://memona.example.com"], "https://memona.example.com")]:
            with self.subTest(answers=answers), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "bin").mkdir()
                binary = root / "bin" / "memona"
                binary.write_bytes(b"fixture")
                binary.chmod(0o700)
                with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch("builtins.input", side_effect=answers), patch.object(setup.getpass, "getpass", side_effect=["a-strong-admin-password", "a-strong-admin-password"]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                    setup.setup()
                    execve.assert_called_once()
                config = json.loads((root / "config.json").read_text())
                self.assertEqual(config["public_url"], expected)
                self.assertEqual(config["secure_cookies"], expected.startswith("https://"))
                self.assertEqual(config.get("allow_private_http", False), expected.startswith("http://"))

    def test_existing_config_is_not_changed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "bin").mkdir()
            binary = root / "bin" / "memona"
            binary.write_bytes(b"fixture")
            binary.chmod(0o700)
            config = root / "config.json"
            original = '{"data_dir":"./private","listen":"127.0.0.1:8765","public_url":"http://127.0.0.1:8765"}'
            config.write_text(original)
            with patch.object(setup.local, "ROOT", root), patch.object(setup.sys.stdin, "isatty", return_value=True), patch.object(setup.getpass, "getpass", side_effect=["a-strong-admin-password", "a-strong-admin-password"]), patch.object(setup.os, "chdir"), patch.object(setup.os, "execve") as execve:
                setup.setup()
                execve.assert_called_once()
            self.assertEqual(config.read_text(), original)


if __name__ == "__main__":
    unittest.main()
