import test from 'node:test';
import assert from 'node:assert/strict';
import { createAPI, APIError, array } from '../src/lib/api.ts';
import { keyInput } from '../src/lib/key-input.ts';

test('API sends cookie and CSRF, never bearer secret', async () => {
  const original = globalThis.fetch;
  let seen: RequestInit | undefined;
  globalThis.fetch = async (_url, options) => { seen = options; return Response.json({ ok: true }); };
  try {
    const api = createAPI(() => {}); api.setSession('synthetic-csrf');
    await api.request('/api/keys', 'POST', { name: 'test' });
    assert.equal(seen?.credentials, 'same-origin');
    assert.equal(seen?.cache, 'no-store');
    assert.equal((seen?.headers as Record<string, string>)['X-CSRF-Token'], 'synthetic-csrf');
    assert.equal((seen?.headers as Record<string, string>).Authorization, undefined);
    assert.equal(seen?.body, JSON.stringify({ name: 'test' }));
  } finally { globalThis.fetch = original; }
});

test('late responses cannot restore private state after logout', async () => {
  const original = globalThis.fetch;
  let resolve!: (response: Response) => void;
  globalThis.fetch = () => new Promise<Response>(done => { resolve = done; });
  try {
    const api = createAPI(() => {}); api.setSession('test');
    const pending = api.request('/api/keys'); api.reset();
    resolve(Response.json({ token: 'synthetic' }));
    await assert.rejects(pending, (error: unknown) => error instanceof APIError && error.stale);
  } finally { globalThis.fetch = original; }
});

test('expired session notifies caller and returns meaningful error', async () => {
  const original = globalThis.fetch; let expired = false;
  globalThis.fetch = async () => Response.json({ error: 'Sesi berakhir' }, { status: 401 });
  try {
    const api = createAPI(() => { expired = true; api.reset(); }); api.setSession('test');
    await assert.rejects(api.request('/api/session'), (error: unknown) => error instanceof APIError && error.status === 401);
    assert.equal(expired, true);
  } finally { globalThis.fetch = original; }
});

test('nullable lists and invalid responses', () => {
  assert.deepEqual(array(null), []);
  assert.throws(() => array({} as never), /Format respons/);
});

test('key input scopes, permissions and expiry', () => {
  assert.deepEqual(keyInput(' Test ', ['read'], 'project-a, project-a', ''), { name: 'Test', permissions: ['read'], projects: ['project-a'], expires_at: '' });
  assert.deepEqual(keyInput('All', ['read'], '*', '').projects, ['*']);
  for (const scope of ['', '*, project-a', '../private', 'Project A']) assert.throws(() => keyInput('Test', ['read'], scope, ''));
  assert.throws(() => keyInput('Test', [], 'a', ''));
  assert.throws(() => keyInput('Test', ['read'], 'a', '2000-01-01'));
});
