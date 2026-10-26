import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("memona_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class RunTest(unittest.TestCase):
    def test_new_install_builds_before_interactive_setup(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            calls = []
            with patch.object(runner.local, "ROOT", root), patch.object(runner.sys.stdin, "isatty", return_value=True), patch.object(runner.subprocess, "run", side_effect=lambda *a, **kw: calls.append("build")), patch.object(runner.setup, "setup", side_effect=lambda *a: calls.append("setup")), patch.object(runner.local, "main") as start:
                self.assertEqual(runner.run(), 0)
            self.assertEqual(calls, ["build", "setup"])
            start.assert_not_called()

    def test_noninteractive_new_install_does_not_build(self):
        with tempfile.TemporaryDirectory() as tmp:
            with patch.object(runner.local, "ROOT", Path(tmp)), patch.object(runner.sys.stdin, "isatty", return_value=False), patch.object(runner.subprocess, "run") as build:
                with self.assertRaisesRegex(ValueError, "terminal interaktif"):
                    runner.run()
                build.assert_not_called()

    def test_interrupted_bootstrap_builds_and_prompts_instead_of_start(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data").mkdir()
            (root / "config.json").write_text(json.dumps({"data_dir": "./data"}))
            with patch.object(runner.local, "ROOT", root), patch.object(runner.local, "writer_busy", return_value=False), patch.object(runner.setup, "interrupted_bootstrap", return_value=True), patch.object(runner.sys.stdin, "isatty", return_value=True), patch.object(runner.subprocess, "run") as build, patch.object(runner.setup, "setup") as bootstrap, patch.object(runner.local, "main") as start:
                self.assertEqual(runner.run(), 0)
                build.assert_called_once()
                bootstrap.assert_called_once_with("config.json")
                start.assert_not_called()

    def test_existing_data_uses_existing_account_and_config(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data").mkdir()
            config = root / "config.json"
            original = json.dumps({"data_dir": "./data", "listen": "127.0.0.1:8765", "public_url": "http://127.0.0.1:8765"})
            config.write_text(original)
            with patch.object(runner.local, "ROOT", root), patch.object(runner.local, "writer_busy", return_value=False), patch.object(runner.subprocess, "run") as build, patch.object(runner.local, "main", return_value=0) as start, patch.object(runner.setup, "setup") as bootstrap:
                self.assertEqual(runner.run(), 0)
                build.assert_called_once_with(["make", "build"], cwd=root, check=True)
                start.assert_called_once_with(["start", "--config", "config.json"])
                bootstrap.assert_not_called()
            self.assertEqual(config.read_text(), original)

    def test_existing_writer_blocks_build_and_second_start(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data").mkdir()
            (root / "config.json").write_text(json.dumps({"data_dir": "./data"}))
            with patch.object(runner.local, "ROOT", root), patch.object(runner.local, "writer_busy", return_value=True), patch.object(runner.subprocess, "run") as build:
                with self.assertRaisesRegex(ValueError, "Writer sedang aktif"):
                    runner.run()
                build.assert_not_called()

    def test_missing_config_with_data_blocks_reinitialization(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data").mkdir()
            with patch.object(runner.local, "ROOT", root), patch.object(runner.subprocess, "run") as build:
                with self.assertRaisesRegex(ValueError, "Config belum ada tetapi data sudah ada"):
                    runner.run()
                build.assert_not_called()


if __name__ == "__main__":
    unittest.main()
