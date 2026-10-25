import importlib.util
import json
from pathlib import Path
import socket
import tempfile
import threading
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("memona_local", Path(__file__).with_name("local.py"))
local = importlib.util.module_from_spec(spec)
spec.loader.exec_module(local)


class LocalTest(unittest.TestCase):
    def test_status_absent_does_not_create_data(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            config = root / "config.json"
            config.write_text(json.dumps({"data_dir": str(root / "missing")}))
            self.assertEqual(local.main(["status", "--config", str(config)]), 1)
            self.assertFalse((root / "missing").exists())

    def test_status_reports_lock_and_health_without_claiming_readiness(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data = root / "data"
            data.mkdir()
            config = root / "config.json"
            config.write_text(json.dumps({"data_dir": str(data)}))
            with patch.object(local, "writer_busy", return_value=True), patch.object(local, "local_health", return_value=False):
                self.assertEqual(local.main(["status", "--config", str(config)]), 1)
            with patch.object(local, "writer_busy", return_value=True), patch.object(local, "local_health", return_value=True):
                self.assertEqual(local.main(["status", "--config", str(config)]), 0)

    def test_start_never_execs_if_writer_is_busy(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data = root / "data"
            data.mkdir()
            config = root / "config.json"
            config.write_text(json.dumps({"data_dir": str(data)}))
            with patch.object(local, "ROOT", root), patch.object(local, "writer_busy", return_value=True), patch.object(local.os, "execv") as execv:
                (root / "bin").mkdir()
                binary = root / "bin" / "memona"
                binary.write_text("fake")
                binary.chmod(0o700)
                self.assertEqual(local.main(["start", "--config", str(config)]), 1)
                execv.assert_not_called()

    def test_health_uses_loopback_and_configured_host(self):
        server = socket.socket()
        server.bind(("127.0.0.1", 0))
        server.listen(1)
        port = server.getsockname()[1]
        seen = []

        def respond():
            connection, _ = server.accept()
            with connection:
                seen.append(connection.recv(512))
                connection.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\n{}")
            server.close()

        thread = threading.Thread(target=respond)
        thread.start()
        try:
            self.assertTrue(local.local_health(f"0.0.0.0:{port}", f"http://100.76.35.38:{port}"))
        finally:
            thread.join(timeout=3)
            server.close()
        self.assertIn(f"Host: 100.76.35.38:{port}\r\n".encode(), seen[0])
        self.assertIsNone(local.local_health("192.168.1.1:8765", "http://192.168.1.1:8765"))


if __name__ == "__main__":
    unittest.main()
