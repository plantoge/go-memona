import test from 'node:test';
import assert from 'node:assert/strict';
import { projectInput } from '../src/lib/projects.ts';

test('project editor normalizes display fields and preserves stable identity/revision', () => {
  assert.deepEqual(projectInput('demo', ' Demo ', ' Description ', 'one\n\n two ', true, 3), {
    slug: 'demo', name: 'Demo', description: 'Description', aliases: ['one', 'two'], archived: true, expected_revision: 3,
  });
  assert.equal(projectInput('demo', 'Demo', '', '', false).expected_revision, 0);
});
test('project editor rejects invalid slug, byte limits, duplicate aliases and revisions', () => {
  for (const slug of ['', '*', '../demo', 'UPPER', 'a'.repeat(65)]) assert.throws(() => projectInput(slug, 'Demo', '', '', false));
  assert.throws(() => projectInput('demo', 'é'.repeat(101), '', '', false));
  assert.throws(() => projectInput('demo', 'Demo', 'é'.repeat(1001), '', false));
  assert.throws(() => projectInput('demo', 'Demo', '', 'same\nsame', false));
  assert.throws(() => projectInput('demo', 'Demo', '', Array.from({ length: 21 }, (_, i) => `alias${i}`).join('\n'), false));
  assert.throws(() => projectInput('demo', 'Demo', '', '', false, -1));
});
