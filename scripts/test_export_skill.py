"""Synthetic fixtures are retained under project GOTMPDIR/.cache/tmp; no cleanup."""
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

import export_skill as exporter

ROOT = Path(__file__).resolve().parents[1]
# Keep fixtures inside this project even if a caller has a global GOTMPDIR.
DEFAULT_TMP = ROOT / ".cache" / "tmp"
TMP = Path(os.environ.get("GOTMPDIR", str(DEFAULT_TMP)))
if not TMP.is_absolute() or ROOT not in TMP.parents:
    TMP = DEFAULT_TMP


class ExportSkillTests(unittest.TestCase):
    def setUp(self):
        TMP.mkdir(parents=True, exist_ok=True)
        self.work = Path(tempfile.mkdtemp(prefix="skill-export-unit-", dir=TMP))
        self.note = {
            "id": "0123456789abcdef0123456789abcdef", "revision": 7,
            "kind": "skill", "status": "active", "archived": False,
            "title": "Synthetic Review Procedure", "content": "Use synthetic data only.\n",
        }
        self.input = self.work / "note.json"
        self.output = self.work / "export"
        self.counter = 0
        self.write_input(self.note)

    def private_file(self, path, data):
        with open(path, "xb") as stream:
            stream.write(data)
        os.chmod(path, 0o600)

    def write_input(self, note):
        # Revisions for negative cases use new files; never overwrite fixtures.
        if self.input.exists():
            self.counter += 1
            self.input = self.work / f"note-{self.counter}.json"
        self.private_file(self.input, json.dumps(note, ensure_ascii=True).encode())

    def export(self, **kwargs):
        return exporter.export_skill(self.input, self.output, reviewed=True, **kwargs)

    def assert_refused(self):
        with self.assertRaises((exporter.ExportError, OSError)):
            self.export()
        self.assertFalse(self.output.exists())

    def test_success_checksum_permissions_source_revision_and_input_unchanged(self):
        before = self.input.read_bytes()
        before_stat = self.input.stat()
        manifest = self.export()
        self.assertEqual(self.input.read_bytes(), before)
        self.assertEqual(exporter.fingerprint(self.input.stat()), exporter.fingerprint(before_stat))
        self.assertEqual(set(p.name for p in self.output.iterdir()), {"SKILL.md", "manifest.json"})
        self.assertEqual(manifest, json.loads((self.output / "manifest.json").read_bytes()))
        skill = (self.output / "SKILL.md").read_bytes()
        self.assertEqual(manifest["source"]["id"], self.note["id"])
        self.assertEqual(manifest["source"]["revision"], 7)
        self.assertEqual(manifest["source"]["input_sha256"], hashlib.sha256(before).hexdigest())
        self.assertEqual(manifest["files"]["SKILL.md"], {"bytes": len(skill), "sha256": hashlib.sha256(skill).hexdigest()})
        self.assertFalse(manifest["native_discovery_verified"])
        self.assertEqual(stat.S_IMODE(self.output.stat().st_mode), 0o700)
        for path in self.output.iterdir():
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
        self.assertTrue(skill.endswith(self.note["content"].encode()))

    def test_declared_metadata_uses_safe_trigger_and_manifest(self):
        self.note["skill_metadata"] = {"trigger": 'Use when reviewing "quoted" code', "version": "1.0", "compatibility": ["OpenCode"], "test_evidence": "Synthetic test"}
        self.write_input(self.note)
        manifest = self.export()
        self.assertEqual(manifest["declared_skill_metadata"], self.note["skill_metadata"])
        skill = (self.output / "SKILL.md").read_text()
        self.assertIn('description: ' + json.dumps(self.note["skill_metadata"]["trigger"], ensure_ascii=True), skill)
        self.assertFalse(manifest["native_discovery_verified"])

    def test_invalid_metadata_refused_before_output(self):
        for metadata in ({"trigger": "bad\nvalue"}, {"version": "x" * 101}, {"trigger": "bad\u0085value"}, {"compatibility": ["x"] * 9}, {"unknown": "value"}):
            self.note["skill_metadata"] = metadata
            self.write_input(self.note)
            self.assert_refused()

    def test_acknowledgement_required_before_read_or_create(self):
        with patch.object(exporter, "read_input", side_effect=AssertionError("must not read")):
            for reviewed in (False, None, 1, "yes"):
                with self.subTest(reviewed=reviewed), self.assertRaises(exporter.ExportError):
                    exporter.export_skill(self.input, self.output, reviewed=reviewed)
        self.assertFalse(self.output.exists())

    def test_existing_output_directory_file_or_symlink_not_overwritten(self):
        self.output.mkdir(mode=0o700)
        self.private_file(self.output / "marker", b"preserve")
        with self.assertRaises(FileExistsError):
            self.export()
        self.assertEqual((self.output / "marker").read_bytes(), b"preserve")
        for name, link in (("existing-file", False), ("existing-link", True)):
            self.output = self.work / name
            if link:
                self.output.symlink_to(self.work / "nonexistent")
            else:
                self.private_file(self.output, b"preserve")
            with self.assertRaises(FileExistsError):
                self.export()
            if not link:
                self.assertEqual(self.output.read_bytes(), b"preserve")

    def test_traversal_rejected_before_normalization(self):
        for field in ("source", "output"):
            with self.subTest(field=field), self.assertRaises(exporter.ExportError):
                source = self.work / ".." / self.work.name / "note.json" if field == "source" else self.input
                output = self.work / ".." / self.work.name / "export" if field == "output" else self.output
                exporter.export_skill(source, output, reviewed=True)
        self.assertFalse(self.output.exists())

    def test_symlink_input_and_symlink_ancestors(self):
        link = self.work / "source-link"
        link.symlink_to(self.input)
        with self.assertRaises(OSError):
            exporter.export_skill(link, self.output, reviewed=True)
        parent_link = self.work / "parent-link"
        parent_link.symlink_to(self.work, target_is_directory=True)
        for source, output in ((parent_link / self.input.name, self.output),
                               (self.input, parent_link / "export")):
            with self.subTest(source=source, output=output), self.assertRaises(OSError):
                exporter.export_skill(source, output, reviewed=True)
        self.assertFalse(self.output.exists())

    def test_hardlink_input_denied(self):
        os.link(self.input, self.work / "hardlink")
        self.assert_refused()

    def test_nonregular_input_denied_without_blocking(self):
        for source in (self.work, self.work / "fifo"):
            if source.name == "fifo":
                os.mkfifo(source, 0o600)
            with self.subTest(source=source), self.assertRaises(exporter.ExportError):
                exporter.export_skill(source, self.output, reviewed=True)
        self.assertFalse(self.output.exists())

    def test_nonprivate_input_and_wrong_owner_denied(self):
        os.chmod(self.input, 0o640)
        self.assert_refused()
        os.chmod(self.input, 0o600)
        with patch.object(exporter.os, "geteuid", return_value=os.geteuid() + 1):
            self.assert_refused()

    def test_untrusted_output_parent_denied(self):
        parent = self.work / "shared"
        parent.mkdir(mode=0o777)
        os.chmod(parent, 0o777)
        self.output = parent / "export"
        self.assert_refused()

    def test_note_validation_and_unsupported_bundles(self):
        cases = {
            "kind": ["knowledge", "Skill", None],
            "status": ["draft", "verified", "unverified", "Active", None],
            "archived": [True, "false", 0, None],
            "id": ["../escape", "a" * 31, "A" * 32, "g" * 32, 42, None],
            "revision": [True, 0, -1, 1000000, "7", 1.5, None],
            "title": ["", "   ", "unsafe\n---\nname: injected", "x\rname: bad", "bad\x00", "é" * 101, None],
            "content": ["", "   ", "bad\x00", "bad\x1b", None],
        }
        for field, values in cases.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    self.write_input(dict(self.note, **{field: value}))
                    self.assert_refused()
            missing = dict(self.note)
            del missing[field]
            self.write_input(missing)
            self.assert_refused()
        for key in ("references", "scripts", "files", "assets"):
            self.write_input(dict(self.note, **{key: []}))
            self.assert_refused()

    def test_malformed_utf8_markdown_envelope_duplicates_and_nan_denied(self):
        for index, raw in enumerate((b"\xff", b"---\nname: x\n---", b"[]", b"{", b"NaN",
                                      b'{"status":"active","status":"draft"}',
                                      json.dumps({"result": self.note}).encode(),
                                      ('{"extra":NaN,' + json.dumps(self.note)[1:]).encode(),
                                      b"[" * 2000 + b"]" * 2000)):
            source = self.work / f"malformed-{index}"
            self.private_file(source, raw)
            with self.subTest(index=index), self.assertRaises(exporter.ExportError):
                exporter.export_skill(source, self.output, reviewed=True)
        self.assertFalse(self.output.exists())

    def test_size_limits_exact_and_multibyte(self):
        for content in ("x" * (exporter.MAX_CONTENT + 1), "é" * (exporter.MAX_CONTENT // 2 + 1)):
            self.write_input(dict(self.note, content=content))
            self.assert_refused()
        source = self.work / "too-large.json"
        self.private_file(source, b" " * (exporter.MAX_INPUT + 1))
        with self.assertRaises(exporter.ExportError):
            exporter.export_skill(source, self.output, reviewed=True)
        self.write_input(dict(self.note, content="x" * exporter.MAX_CONTENT))
        self.export()
        # Valid JSON padded to exactly 128 KiB must be accepted, one extra byte refused.
        source = self.work / "exact-input.json"
        raw = json.dumps(self.note).encode()
        self.private_file(source, raw + b" " * (exporter.MAX_INPUT - len(raw)))
        exporter.export_skill(source, self.work / "exact-export", reviewed=True)

    def test_frontmatter_quoting_slug_and_no_content_execution(self):
        marker = self.work / "must-not-exist"
        title = '日本語 "quotes": [x] # comment ---'
        body = f'---\nname: not-frontmatter\n---\n$(touch {marker})\n```python\nraise RuntimeError("never run")\n```\n'
        note = dict(self.note, title=title, content=body)
        self.write_input(note)
        self.export()
        skill = (self.output / "SKILL.md").read_text()
        frontmatter, exported_body = skill[4:].split("\n---\n\n", 1)
        lines = frontmatter.splitlines()
        self.assertEqual(len(lines), 2)
        name = json.loads(lines[0].removeprefix("name: "))
        description = json.loads(lines[1].removeprefix("description: "))
        self.assertRegex(name, r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
        self.assertLessEqual(len(name), 64)
        self.assertIn(title, description)
        self.assertEqual(exported_body, body)
        self.assertFalse(marker.exists())
        self.assertEqual(exporter.render_skill(note), exporter.render_skill(note))
        self.assertNotEqual(exporter.render_skill(dict(note, title="A B")), exporter.render_skill(dict(note, title="A-B")))
        self.assertTrue(exporter.render_skill(dict(note, title="日本語")).startswith(b'---\nname: "skill-'))

    def test_changed_input_during_read_denied(self):
        original = exporter.fingerprint
        calls = []
        def changed(info):
            calls.append(1)
            return original(info) + (len(calls),)
        with patch.object(exporter, "fingerprint", side_effect=changed):
            self.assert_refused()

    def test_partial_output_retained_on_failure(self):
        original = exporter.write_new
        def fail_manifest(parent, name, data):
            if name == "manifest.json":
                raise OSError("synthetic write failure")
            return original(parent, name, data)
        with patch.object(exporter, "write_new", side_effect=fail_manifest), self.assertRaises(OSError):
            self.export()
        self.assertTrue((self.output / "SKILL.md").is_file())
        self.assertFalse((self.output / "manifest.json").exists())
        with self.assertRaises(FileExistsError):
            self.export()

    def test_cli_ack_required_and_sanitized_error(self):
        command = [sys.executable, str(ROOT / "scripts" / "export_skill.py"),
                   "--input", str(self.input), "--output", str(self.output)]
        denied = subprocess.run(command, capture_output=True, text=True, timeout=10)
        self.assertEqual(denied.returncode, 1)
        self.assertNotIn(str(self.input), denied.stderr)
        self.assertFalse(self.output.exists())
        result = subprocess.run(command + ["--reviewed"], capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("native installation/discovery is not verified", result.stdout)


if __name__ == "__main__":
    unittest.main()
