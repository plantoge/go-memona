#!/usr/bin/env python3
"""Read-only verification of an explicit private portable skill bundle.

Checks integrity and supported structure, not provenance, safety or native discovery.
"""
import argparse
import hashlib
import json
import os
import re
import stat
import sys

import export_skill_bundle as bundle

require = bundle.require
ExportError = bundle.ExportError


def parse_manifest(raw):
    try:
        manifest = json.loads(raw.decode('utf-8'), object_pairs_hook=bundle.exporter.unique_object,
                              parse_constant=bundle.exporter.invalid_constant)
    except (ValueError, UnicodeError, RecursionError):
        raise ExportError('Invalid manifest JSON.') from None
    require(isinstance(manifest, dict) and set(manifest) == {
        'format', 'exporter_version', 'reviewed_by_operator', 'native_discovery_verified',
        'source', 'files', 'unsupported'}, 'Invalid bundle manifest structure.')
    require(manifest['format'] == bundle.FORMAT and manifest['exporter_version'] == '1'
            and manifest['reviewed_by_operator'] is True
            and manifest['native_discovery_verified'] is False, 'Unsupported bundle declaration.')
    source = manifest['source']
    require(isinstance(source, dict) and set(source) == {'id', 'revision', 'input_sha256', 'bundle_input_sha256'},
            'Invalid source declaration.')
    require(isinstance(source['id'], str) and re.fullmatch(r'[0-9a-f]{32}', source['id'])
            and type(source['revision']) is int and 1 <= source['revision'] <= 999999,
            'Invalid source identity.')
    for key in ('input_sha256', 'bundle_input_sha256'):
        require(isinstance(source[key], str) and re.fullmatch(r'[0-9a-f]{64}', source[key]),
                'Invalid source digest.')
    require(manifest['unsupported'] == ['scripts', 'executable files', 'installation', 'native discovery verification'],
            'Unsupported capability declaration.')
    files = manifest['files']
    require(isinstance(files, dict) and 'SKILL.md' in files and 1 <= len(files) < bundle.MAX_FILES,
            'Invalid payload file count.')
    supporting = [name for name in files if name != 'SKILL.md']
    # Apply exactly the exporter's collision/path policy, without reading any path.
    bundle.parse_manifest(json.dumps({'format': bundle.INPUT_FORMAT, 'note': 'note.json',
                                     'files': supporting}).encode())
    for entry in files.values():
        require(isinstance(entry, dict) and set(entry) == {'bytes', 'sha256'}
                and type(entry['bytes']) is int and 0 <= entry['bytes'] <= bundle.MAX_FILE
                and isinstance(entry['sha256'], str) and re.fullmatch(r'[0-9a-f]{64}', entry['sha256']),
                'Invalid payload digest or size.')
    require(sum(entry['bytes'] for entry in files.values()) + len(raw) <= bundle.MAX_TOTAL,
            'Bundle exceeds aggregate limit.')
    return manifest


def inventory(root, allowed):
    """Check a bounded exact tree; do not follow, open or execute unexpected entries."""
    expected_directories = set()
    for name in allowed:
        parts = name.split('/')
        expected_directories.update('/'.join(parts[:index]) for index in range(1, len(parts)))
    seen_files, seen_directories = set(), set()
    count = 0

    def walk(fd, prefix=''):
        nonlocal count
        with os.scandir(fd) as entries:
            for entry in entries:
                count += 1
                require(count <= bundle.MAX_FILES * 8, 'Too many bundle entries.')
                name = prefix + entry.name
                info = entry.stat(follow_symlinks=False)
                if stat.S_ISDIR(info.st_mode):
                    require(name in expected_directories, 'Unexpected directory in bundle.')
                    with bundle.child_directory(fd, [entry.name]) as child:
                        walk(child, name + '/')
                    seen_directories.add(name)
                else:
                    require(name in allowed and stat.S_ISREG(info.st_mode),
                            'Unexpected or nonregular bundle file.')
                    seen_files.add(name)
    walk(root)
    require(seen_files == set(allowed) and seen_directories == expected_directories,
            'Incomplete bundle tree.')


def verify_bundle(path):
    path = bundle.staging_path(path)
    with bundle.exporter.directory(path) as root:
        bundle.private_directory(root)
        raw = bundle.read_private(root, 'manifest.json', bundle.MAX_FILE)
        manifest = parse_manifest(raw)
        allowed = set(manifest['files']) | {'manifest.json'}
        inventory(root, allowed)
        for name, expected in manifest['files'].items():
            if name == 'SKILL.md':
                data = bundle.read_private(root, name, bundle.MAX_FILE)
                try:
                    text = data.decode('utf-8')
                except UnicodeError:
                    raise ExportError('Invalid skill text.') from None
                require(text.startswith('---\n') and '\n---\n\n' in text,
                        'Missing portable skill frontmatter.')
            else:
                data = bundle.read_supporting(root, name)
            require(len(data) == expected['bytes'] and hashlib.sha256(data).hexdigest() == expected['sha256'],
                    'Bundle checksum mismatch.')
        # Detect accidental changes during verification; this is not a filesystem snapshot.
        require(bundle.read_private(root, 'manifest.json', bundle.MAX_FILE) == raw,
                'Manifest changed during verification.')
        inventory(root, allowed)
    return {'format': manifest['format'], 'payload_files': len(manifest['files']),
            'payload_bytes': sum(entry['bytes'] for entry in manifest['files'].values()),
            'native_discovery_verified': False}


def main(argv=None):
    parser = bundle.SafeParser(description=__doc__)
    parser.add_argument('--bundle', required=True, help='Explicit isolated private bundle directory')
    args = parser.parse_args(argv)
    try:
        result = verify_bundle(args.bundle)
    except (ExportError, OSError, ValueError):
        print('Bundle verification refused or failed. Check complete manifest, checksums, limits and private nonlinked paths. No repair performed.', file=sys.stderr)
        return 1
    print(f"PASS: {result['payload_files']} payload files, {result['payload_bytes']} bytes; integrity only, not provenance/safety/native discovery.")
    return 0


if __name__ == '__main__':
    sys.exit(main())
