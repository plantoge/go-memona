import test from 'node:test';
import assert from 'node:assert/strict';
import { bytes, issueLabel } from '../src/lib/storage-health.ts';

test('storage capacity format distinguishes unavailable and zero', () => {
  assert.equal(bytes(0), '0 B');
  assert.equal(bytes(1024), '1 KiB');
  assert.equal(bytes(256 * 1024 ** 2), '256 MiB');
  assert.equal(bytes(NaN), 'Tidak tersedia');
  assert.equal(bytes(-1), 'Tidak tersedia');
});
test('integrity labels retain unknown issue codes without claiming success', () => {
  assert.match(issueLabel('entry_limit'), /20.000/);
  assert.match(issueLabel('corrupt_revision'), /rusak/);
  assert.equal(issueLabel('future_code'), 'future_code');
});
