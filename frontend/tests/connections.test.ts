import test from 'node:test';
import assert from 'node:assert/strict';
import { relationIDs, connectionInput } from '../src/lib/connections.ts';

const a = 'a'.repeat(32), b = 'b'.repeat(32), c = 'c'.repeat(32);
test('relation input accepts separated IDs and explicit empty clears', () => {
  assert.deepEqual(relationIDs(` ${a},\n${b} `), [a, b]);
  assert.deepEqual(connectionInput(false, '', '', c), { is_project_summary: false, related_to: [], supersedes: [] });
  assert.deepEqual(connectionInput(true, a, b, c), { is_project_summary: true, related_to: [a], supersedes: [b] });
});
test('relation input rejects malformed, duplicate, overlapping, self and excessive IDs', () => {
  for (const ids of ['../path', 'A'.repeat(32), `${a},${a}`, Array(21).fill(a).join('\n')]) assert.throws(() => relationIDs(ids));
  assert.throws(() => connectionInput(false, a, '', a));
  assert.throws(() => connectionInput(false, a, a, c));
});
