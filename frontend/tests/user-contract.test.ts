import test from 'node:test';
import assert from 'node:assert/strict';
import { splitWorkspaces, notePath, expiryValue, type Workspace } from '../src/user-contract.ts';

test('workspace response is an array with personal and team kinds', () => {
  const items: Workspace[] = [
    { id: 'team-1', kind: 'team', name: 'Tim', role: 'member' },
    { id: 'personal-1', kind: 'personal', name: 'Pribadi', role: 'owner' },
  ];
  assert.deepEqual(splitWorkspaces(items), { personal: items[1], teams: [items[0]] });
  assert.throws(() => splitWorkspaces(items.slice(0, 1)), /Ruang pribadi/);
});

test('note detail and revision path carries the workspace scope', () => {
  assert.equal(notePath('note/with spaces', 'team & 1'), '/api/user/notes/note%2Fwith%20spaces?workspace_id=team+%26+1');
});

test('optional key expiry is an empty string, not null', () => {
  assert.equal(expiryValue(''), '');
  assert.match(expiryValue('2030-02-03'), /^2030-02-0[34]T/);
});
