import test from 'node:test';
import assert from 'node:assert/strict';
import { integrationSnippet, rotationReady } from '../src/lib/integration-guide.ts';

test('harness snippets use distinct environment syntax and never a secret', () => {
  const endpoint = 'https://memona.example/mcp';
  const config = JSON.parse(integrationSnippet('opencode', endpoint));
  assert.equal(config.mcp.servers.memona.url, endpoint);
  assert.equal(config.mcp.servers.memona.oauth, false);
  assert.equal(config.mcp.servers.memona.headers.Authorization, 'Bearer {env:MEMONA_API_KEY}');
  const yaml = integrationSnippet('hermes', endpoint);
  assert.ok(yaml.startsWith('mcp_servers:\n'));
  assert.ok(yaml.includes('Bearer ${MEMONA_API_KEY}'));
  assert.ok(!yaml.includes('{env:'));
});
test('snippet rejects credential-bearing, injected or non-MCP endpoints', () => {
  for (const url of ['javascript:alert(1)', 'https://user:secret@example.com/mcp', 'https://example.com/mcp?token=secret', 'https://example.com/mcp#secret', 'https://example.com/dashboard', 'https://example.com/\nmcp', 'invalid']) {
    assert.throws(() => integrationSnippet('hermes', url));
  }
});
test('rotation requires distinct replacement, acknowledgment and nonrevoked original', () => {
  assert.equal(rotationReady('old', 'new', true, false), true);
  assert.equal(rotationReady('old', 'old', true, false), false);
  assert.equal(rotationReady('old', '', true, false), false);
  assert.equal(rotationReady('old', 'new', false, false), false);
  assert.equal(rotationReady('old', 'new', true, true), false);
});
