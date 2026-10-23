"""Read-only verifier tests; all synthetic fixtures retained inside project cache."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import export_skill_bundle as exporter
import verify_skill_bundle as verifier

ROOT = Path(__file__).resolve().parents[1]


class VerifyBundleTests(unittest.TestCase):
    def setUp(self):
        self.work = Path(tempfile.mkdtemp(prefix='verify-skill-bundle-', dir=ROOT / '.cache/tmp'))
        self.output = self.work / 'export'
        note = {'id':'a'*32, 'revision':1, 'kind':'skill', 'status':'active', 'archived':False,
                'title':'Synthetic fixture', 'content':'Read references/guide.txt.'}
        self.put('note.json', json.dumps(note).encode())
        self.put('input.json', json.dumps({'format':exporter.INPUT_FORMAT,'note':'note.json','files':['references/guide.txt']}).encode())
        (self.work / 'references').mkdir(mode=0o700)
        self.put('references/guide.txt', b'Synthetic reference\n')
        exporter.export_bundle(self.work / 'input.json', self.output, True)

    def put(self, name, data):
        path = self.work / name
        path.write_bytes(data)
        path.chmod(0o600)

    def refused(self):
        with self.assertRaises((exporter.ExportError, OSError)):
            verifier.verify_bundle(self.output)

    def test_read_only_complete_bundle(self):
        paths = [self.output, *self.output.rglob('*')]
        before = {str(p): (p.lstat().st_mode,p.lstat().st_mtime_ns,p.lstat().st_ctime_ns,
                           hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else '') for p in paths}
        result = verifier.verify_bundle(self.output)
        after = {str(p): (p.lstat().st_mode,p.lstat().st_mtime_ns,p.lstat().st_ctime_ns,
                          hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else '') for p in paths}
        self.assertEqual(before, after)
        self.assertEqual(result['payload_files'], 2)
        self.assertFalse(result['native_discovery_verified'])

    def test_tampered_content_and_missing_payload(self):
        (self.output / 'references/guide.txt').write_bytes(b'tampered')
        self.refused()
        # Missing file simulated by rename, never deletion.
        os.rename(self.output / 'SKILL.md', self.work / 'saved-skill.txt')
        self.refused()

    def test_missing_or_partial_manifest(self):
        os.rename(self.output / 'manifest.json', self.work / 'saved-manifest.json')
        self.refused()
        self.put('export/manifest.json', b'{"partial":')
        self.refused()

    def test_extra_file_and_extra_directory_refused(self):
        self.put('export/unexpected.txt', b'not in manifest')
        self.refused()
        os.rename(self.output / 'unexpected.txt', self.work / 'retained.txt')
        (self.output / 'unexpected').mkdir(mode=0o700)
        self.refused()

    def test_link_and_permissions_refused(self):
        payload = self.output / 'references/guide.txt'
        os.rename(payload, self.work / 'original.txt')
        os.symlink(self.work / 'original.txt', payload)
        self.refused()
        os.rename(payload, self.work / 'retained-link')
        os.link(self.work / 'original.txt', payload)
        self.refused()
        (self.output / 'SKILL.md').chmod(0o644)
        self.refused()

    def test_manifest_schema_paths_counts_sizes_and_types(self):
        path = self.output / 'manifest.json'
        original = json.loads(path.read_bytes())
        for change in ('traversal', 'native', 'bool-size', 'large', 'digest', 'extra', 'collision'):
            manifest = json.loads(json.dumps(original))
            if change == 'traversal':
                manifest['files']['../outside.txt'] = manifest['files']['SKILL.md']
            elif change == 'native':
                manifest['native_discovery_verified'] = True
            elif change == 'bool-size':
                manifest['files']['SKILL.md']['bytes'] = True
            elif change == 'large':
                manifest['files']['SKILL.md']['bytes'] = exporter.MAX_FILE + 1
            elif change == 'digest':
                manifest['files']['SKILL.md']['sha256'] = 'not-a-digest'
            elif change == 'extra':
                manifest['unexpected'] = 1
            else:
                manifest['files']['references/GUIDE.txt'] = manifest['files']['references/guide.txt']
            path.write_text(json.dumps(manifest))
            with self.subTest(change=change):
                self.refused()
        path.write_bytes(b'{"format":1,"format":2}')
        self.refused()

    def test_payload_permissions_and_frontmatter_checked_independently(self):
        skill = self.output / 'SKILL.md'
        skill.chmod(0o644)
        self.refused()
        skill.chmod(0o700)
        self.refused()
        skill.chmod(0o600)
        # Even a matching digest does not allow a nonportable skill header.
        skill.write_bytes(b'not portable frontmatter')
        path = self.output / 'manifest.json'
        manifest = json.loads(path.read_bytes())
        manifest['files']['SKILL.md'] = {'bytes':skill.stat().st_size,
                                       'sha256':hashlib.sha256(skill.read_bytes()).hexdigest()}
        path.write_text(json.dumps(manifest))
        self.refused()

    def test_cli_success_reports_only_aggregate_integrity(self):
        result = subprocess.run([sys.executable,'-B',str(ROOT / 'scripts/verify_skill_bundle.py'),
                                 '--bundle',str(self.output)],capture_output=True,text=True,timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('PASS: 2 payload files', result.stdout)
        self.assertNotIn('a'*32, result.stdout)
        self.assertNotIn(str(self.output), result.stdout)

    def test_cli_generic_failure_without_private_name(self):
        result = subprocess.run([sys.executable,'-B',str(ROOT / 'scripts/verify_skill_bundle.py'),
                                 '--bundle',str(self.work / 'SYNTHETIC_PRIVATE_MISSING')],
                                capture_output=True,text=True,timeout=10)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('SYNTHETIC_PRIVATE_MISSING',result.stdout+result.stderr)
        self.assertNotIn('Traceback',result.stderr)


if __name__ == '__main__':
    unittest.main()
