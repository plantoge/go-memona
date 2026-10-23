"""Synthetic bundle fixtures are retained in the project cache; no cleanup."""
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

import export_skill_bundle as bundle

ROOT = Path(__file__).resolve().parents[1]
TMP = ROOT / '.cache' / 'tmp'


class SkillBundleTests(unittest.TestCase):
    def setUp(self):
        TMP.mkdir(parents=True, exist_ok=True)
        self.work = Path(tempfile.mkdtemp(prefix='skill-bundle-unit-', dir=TMP))
        self.source = self.work / 'bundle.json'
        self.output = self.work / 'export'
        self.note = {'id': '0123456789abcdef0123456789abcdef', 'revision': 2,
                     'kind': 'skill', 'status': 'active', 'archived': False,
                     'title': 'Synthetic review', 'content': 'Read references/guide.md; do not execute code.\n'}
        self.spec = {'format': bundle.INPUT_FORMAT, 'note': 'note.json', 'files': []}
        self.put('note.json', json.dumps(self.note).encode())
        self.save_spec()

    def put(self, name, data, mode=0o600):
        path = self.work / name
        current = self.work
        for part in path.relative_to(self.work).parts[:-1]:
            current = current / part
            if not current.exists():
                current.mkdir(mode=0o700)
        path.write_bytes(data)
        path.chmod(mode)
        return path

    def save_spec(self):
        self.put('bundle.json', json.dumps(self.spec).encode())

    def export(self):
        return bundle.export_bundle(self.source, self.output, reviewed=True)

    def refused(self):
        with self.assertRaises((bundle.ExportError, OSError)):
            self.export()
        self.assertFalse(self.output.exists())

    def test_nested_bundle_checksums_private_modes_and_no_execution(self):
        self.spec['files'] = ['references/nested/guide.md', 'assets/chart.png']
        self.save_spec()
        self.put(self.spec['files'][0], b'# Synthetic reference\n```sh\necho never executed\n```\n')
        self.put(self.spec['files'][1], b'\x89PNG\r\n\x1a\nsynthetic header-only fixture')
        manifest = self.export()
        self.assertEqual(manifest, json.loads((self.output / 'manifest.json').read_bytes()))
        self.assertFalse(manifest['native_discovery_verified'])
        self.assertEqual(manifest['source']['revision'], 2)
        self.assertEqual(len(manifest['files']), 3)
        for name, entry in manifest['files'].items():
            path = self.output / name
            data = path.read_bytes()
            self.assertEqual(entry, {'bytes': len(data), 'sha256': hashlib.sha256(data).hexdigest()})
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
        for directory in (self.output, self.output / 'references', self.output / 'references/nested', self.output / 'assets'):
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE((self.output / 'manifest.json').stat().st_mode), 0o600)

    def test_acknowledgement_precedes_any_read(self):
        with patch.object(bundle, 'staging_path', side_effect=AssertionError('path accessed')):
            with self.assertRaises(bundle.ExportError):
                bundle.export_bundle(self.source, self.output)
        self.assertFalse(self.output.exists())

    def test_path_and_schema_validation(self):
        bad_paths = ['../escape.md', '/absolute.md', 'references/../escape.md',
                     'references//a.md', 'references/a\\b.md', 'references/.hidden.md',
                     'references/a.md/', 'scripts/a.py', 'references/exec.sh',
                     'assets/image.svg', 'references/a\n.md', 'references/' + 'a' * 101 + '.md']
        for path in bad_paths:
            with self.subTest(path=path):
                self.spec['files'] = [path]
                self.save_spec()
                self.refused()
        for note in ('../note.json', '/note.json', 'manifest.json', 'nested/note.json'):
            self.spec.update(note=note, files=[])
            self.save_spec()
            self.refused()
        for raw in (b'{"format":"x","format":"y"}', b'{"x":NaN}', b'[]', b'\xff'):
            self.put('bundle.json', raw)
            self.refused()

    def test_duplicate_case_and_prefix_collisions(self):
        for names in (['references/a.md'] * 2,
                      ['references/A.md', 'references/a.md'],
                      ['references/Dir/a.md', 'references/dir/b.md'],
                      ['references/a.md', 'references/a.md/b.md']):
            self.spec['files'] = names
            self.save_spec()
            self.refused()

    def test_exact_file_count_limit_and_one_more(self):
        self.spec['files'] = [f'references/f{i}.txt' for i in range(30)]
        for name in self.spec['files']:
            self.put(name, b'synthetic')
        self.save_spec()
        self.assertEqual(len(self.export()['files']), 31)
        self.output = self.work / 'too-many'
        self.spec['files'].append('references/extra.txt')
        self.save_spec()
        self.refused()

    def test_per_file_and_aggregate_limits(self):
        self.spec['files'] = ['references/big.txt']
        self.put(self.spec['files'][0], b'x' * (bundle.MAX_FILE + 1))
        self.save_spec()
        self.refused()
        self.put(self.spec['files'][0], b'x' * bundle.MAX_FILE)
        self.export()
        self.output = self.work / 'aggregate'
        self.spec['files'] = [f'references/{i}.txt' for i in range(8)]
        for name in self.spec['files']:
            self.put(name, b'x' * bundle.MAX_FILE)
        self.save_spec()
        self.refused()

    def test_oversized_json_input(self):
        self.put('bundle.json', b' ' * (bundle.exporter.MAX_INPUT + 1))
        self.refused()

    def test_reject_symlink_and_hardlink_inputs(self):
        # No deletion: every case receives a new supporting filename.
        target = self.put('target.txt', b'synthetic')
        os.symlink(target, self.work / 'references-link')
        self.spec['files'] = ['references-link/x.txt']
        self.save_spec()
        self.refused()  # schema rejects unsupported root
        (self.work / 'references').mkdir(mode=0o700)
        os.symlink(target, self.work / 'references/link.txt')
        self.spec['files'] = ['references/link.txt']
        self.save_spec()
        self.refused()
        os.link(target, self.work / 'references/hard.txt')
        self.spec['files'] = ['references/hard.txt']
        self.save_spec()
        self.refused()
        os.symlink(self.work / 'references', self.work / 'assets')
        self.spec['files'] = ['assets/link.png']
        self.save_spec()
        self.refused()

    def test_note_and_manifest_symlinks_refused(self):
        os.symlink(self.work / 'note.json', self.work / 'linked.json')
        self.spec['note'] = 'linked.json'
        self.save_spec()
        self.refused()
        os.symlink(self.source, self.work / 'linked-manifest.json')
        self.source = self.work / 'linked-manifest.json'
        self.refused()

    def test_nonprivate_executable_nonregular_and_wrong_owner(self):
        self.spec['files'] = ['references/a.txt']
        self.save_spec()
        path = self.put(self.spec['files'][0], b'synthetic', 0o644)
        self.refused()
        path.chmod(0o700)
        self.refused()
        path.chmod(0o600)
        with patch.object(bundle.os, 'geteuid', return_value=os.geteuid() + 1):
            self.refused()
        os.mkfifo(self.work / 'references/fifo.txt', 0o600)
        self.spec['files'] = ['references/fifo.txt']
        self.save_spec()
        self.refused()  # O_NONBLOCK prevents hanging on a FIFO.
        self.work.chmod(0o755)
        self.refused()

    def test_control_utf8_executable_and_asset_signature(self):
        for raw in (b'#!/bin/sh\ntrue', b'\x7fELFbinary', b'MZbinary', b'\xff', b'bad\x00text', 'bad\u0085text'.encode()):
            self.spec['files'] = ['references/a.txt']
            self.save_spec()
            self.put(self.spec['files'][0], raw)
            self.refused()
        self.spec['files'] = ['assets/image.png']
        self.save_spec()
        self.put(self.spec['files'][0], b'<svg>not a png</svg>')
        self.refused()

    def test_no_overwrite_even_empty_directory_or_symlink(self):
        self.output.mkdir(mode=0o700)
        sentinel = self.output / 'sentinel'
        sentinel.write_bytes(b'preserve')
        with self.assertRaises(FileExistsError):
            self.export()
        self.assertEqual(sentinel.read_bytes(), b'preserve')
        self.output = self.work / 'dangling'
        os.symlink(self.work / 'absent', self.output)
        with self.assertRaises(FileExistsError):
            self.export()
        self.assertTrue(self.output.is_symlink())

    def test_partial_output_retained_manifest_last(self):
        real_write = bundle.write_new
        def interrupted(parent, name, data):
            if name == 'manifest.json':
                raise OSError('synthetic failure')
            return real_write(parent, name, data)
        with patch.object(bundle, 'write_new', side_effect=interrupted):
            with self.assertRaises(OSError):
                self.export()
        self.assertTrue((self.output / 'SKILL.md').exists())
        self.assertFalse((self.output / 'manifest.json').exists())
        with self.assertRaises(FileExistsError):
            self.export()

    def test_staging_rename_keeps_all_reads_on_same_descriptor(self):
        real_read = bundle.read_private
        moved = self.work.with_name(self.work.name + '-moved')
        original = self.work
        def rename_after_manifest(parent, name, limit):
            raw = real_read(parent, name, limit)
            if name == 'bundle.json':
                os.rename(original, moved)
                original.mkdir(mode=0o700)
                (original / 'note.json').write_text('NOT THE REVIEWED NOTE')
            return raw
        with patch.object(bundle, 'read_private', side_effect=rename_after_manifest):
            manifest = self.export()
        self.assertEqual(manifest['source']['id'], self.note['id'])
        self.assertTrue((self.output / 'manifest.json').exists())
        self.assertTrue((moved / 'note.json').exists())

    def test_mutation_during_read_refused(self):
        real_fingerprint = bundle.exporter.fingerprint
        calls = 0
        def changed(info):
            nonlocal calls
            calls += 1
            return real_fingerprint(info) + (calls,)
        with patch.object(bundle.exporter, 'fingerprint', side_effect=changed):
            self.refused()

    def test_cli_redacts_paths_and_errors(self):
        secret_path = self.work / 'SYNTHETIC_PRIVATE_NAME.json'
        result = subprocess.run([sys.executable, '-B', str(ROOT / 'scripts/export_skill_bundle.py'),
                                 '--manifest', str(secret_path), '--output', str(self.output), '--reviewed'],
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('SYNTHETIC_PRIVATE_NAME', result.stdout + result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
