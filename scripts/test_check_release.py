"""Synthetic fixtures only; preserve fixtures rather than deleting files automatically."""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import check_release as release


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp(prefix="memona-preflight-", dir="/tmp/opencode"))
        for relative in release.SOURCE_FILES:
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("synthetic\n", encoding="utf-8")
        (self.root / ".gitignore").write_text("\n".join(release.IGNORE_PATTERNS) + "\n")
        (self.root / "go.mod").write_text(
            "module memona\n\ngo 1.26.0\nrequire (\n"
            " github.com/modelcontextprotocol/go-sdk v1.8.0\n"
            " modernc.org/sqlite v1.61.0\n)\n")
        dependencies = {"svelte": "5.57.2", "vite": "8.3.5"}
        (self.root / "frontend/package.json").write_text(json.dumps({"devDependencies": dependencies}))
        (self.root / "frontend/package-lock.json").write_text(json.dumps({
            "packages": {"": {"devDependencies": dependencies}}}))
        (self.root / "bin").mkdir()
        (self.root / "bin/memona").write_bytes(b"synthetic-not-executable")
        # These sentinel contents must never be inspected or appear in results.
        (self.root / "data").mkdir()
        for relative in ("config.json", ".env", "data/private.md"):
            (self.root / relative).write_text("SENTINEL-PRIVATE-CONTENT")

    def result(self):
        return release.inspect_source(self.root)

    def test_fixed_allowlist_only_and_no_runtime_reads(self):
        original = release.digest_file
        accessed = []

        def track(path, *args, **kwargs):
            accessed.append(str(Path(path).relative_to(self.root)))
            return original(path, *args, **kwargs)

        with mock.patch.object(release, "digest_file", side_effect=track):
            result = self.result()
        self.assertTrue(result["ok"])
        self.assertEqual(accessed, list(release.SOURCE_FILES))
        rendered = json.dumps(result)
        self.assertNotIn(str(self.root), rendered)
        self.assertNotIn("SENTINEL", rendered)
        self.assertNotIn("synthetic-not-executable", rendered)
        self.assertFalse(any(c["check"] == "explicit_binary_sha256" for c in result["checks"]))

    def test_explicit_binary_hash_without_execution(self):
        result = release.inspect_source(self.root, self.root / "bin/memona")
        entry = next(c for c in result["checks"] if c["check"] == "explicit_binary_sha256")
        self.assertTrue(entry["ok"])
        self.assertEqual(entry["sha256"], release.hashlib.sha256(b"synthetic-not-executable").hexdigest())

    def test_missing_root_and_arguments_are_sanitized(self):
        result = release.inspect_source(self.root / "SENSITIVE-ROOT")
        self.assertFalse(result["ok"])
        self.assertNotIn("SENSITIVE-ROOT", json.dumps(result))
        output = io.StringIO()
        with contextlib.redirect_stdout(output), self.assertRaises(SystemExit) as error:
            release.main(["--unexpected=SENSITIVE-ARG"])
        self.assertEqual(error.exception.code, 2)
        self.assertNotIn("SENSITIVE-ARG", output.getvalue())
        self.assertFalse(json.loads(output.getvalue())["ok"])

    def test_missing_source_is_failure(self):
        # A separate empty root avoids deleting any fixture files.
        empty = Path(tempfile.mkdtemp(prefix="memona-preflight-empty-", dir="/tmp/opencode"))
        result = release.inspect_source(empty)
        self.assertFalse(result["ok"])
        self.assertEqual(result["checks"][0]["reason"], "missing")

    def test_required_ignore_and_pins_fail_closed(self):
        (self.root / ".gitignore").write_text("/bin/\n/internal/webui/dist/\n")
        (self.root / "frontend/package.json").write_text('{"devDependencies":{"vite":"^8.3.5"}}')
        result = self.result()
        failed = {c["check"] for c in result["checks"] if not c["ok"]}
        self.assertIn("ignore_required_patterns", failed)
        self.assertIn("ignore_embedded_assets_not_explicit", failed)
        self.assertIn("frontend_exact_pins_and_lock_root", failed)

    def test_symlink_binary_and_parent_are_rejected(self):
        (self.root / "linked-binary").symlink_to(self.root / "bin/memona")
        result = release.inspect_source(self.root, self.root / "linked-binary")
        self.assertEqual(result["checks"][-1]["reason"], "symlink_rejected")
        (self.root / "linked-source").symlink_to(self.root, target_is_directory=True)
        result = release.inspect_source(self.root / "linked-source")
        self.assertEqual(result["checks"][0]["reason"], "symlink_rejected")

    def test_source_symlink_and_os_error_are_sanitized(self):
        # Separate root: never remove/replace existing fixture files to make a link.
        linked = Path(tempfile.mkdtemp(prefix="memona-preflight-link-", dir="/tmp/opencode"))
        (linked / ".gitignore").symlink_to(self.root / ".env")
        result = release.inspect_source(linked)
        self.assertEqual(result["checks"][0]["reason"], "symlink_rejected")
        self.assertNotIn("SENTINEL", json.dumps(result))
        with mock.patch.object(release, "digest_file", side_effect=PermissionError("SENSITIVE-PATH")):
            result = self.result()
        self.assertFalse(result["ok"])
        self.assertNotIn("SENSITIVE-PATH", json.dumps(result))

    def test_nonregular_and_oversized_inputs_are_rejected(self):
        result = release.inspect_source(self.root, self.root / "data")
        self.assertEqual(result["checks"][-1]["reason"], "not_regular_file")
        with mock.patch.object(release, "MAX_SOURCE_BYTES", 1):
            result = self.result()
        self.assertFalse(result["ok"])
        self.assertEqual(result["checks"][0]["reason"], "size_limit")

    def test_malformed_manifest_is_sanitized(self):
        (self.root / "frontend/package.json").write_text("SENSITIVE-BAD-JSON")
        result = self.result()
        self.assertFalse(result["ok"])
        self.assertNotIn("SENSITIVE-BAD-JSON", json.dumps(result))

    def test_cli_json_exit_status(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            code = release.main(["--source-root", str(self.root)])
        self.assertEqual(code, 0)
        self.assertTrue(json.loads(output.getvalue())["ok"])


if __name__ == "__main__":
    unittest.main()
