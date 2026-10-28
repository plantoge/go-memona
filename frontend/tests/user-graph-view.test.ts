import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// Compile/contract checks only; browser rendering and real Google login need manual acceptance.
const source = (name: string) => readFileSync(new URL(`../src/${name}`, import.meta.url), 'utf8');

test('user graph fetch and node detail stay on workspace-scoped user endpoints', () => {
  const app = source('UserApp.svelte');
  assert.match(app, /\/api\/user\/graph\?\$\{new URLSearchParams\(\{ workspace_id: requestedWorkspace, q: graphQuery \}\)\}/);
  assert.match(app, /notePath\(note\.id, requestedWorkspace\)/);
  assert.match(app, /if \(id === loadID && requestedWorkspace === workspaceID\) graph = graphData\(result\)/);
  assert.match(app, /graph = EMPTY_GRAPH; graphQuery = ''/);
  assert.match(app, /\{#key workspaceID\}<KnowledgeGraph[^>]*scoped suspended=\{!!selected\}/);
  assert.doesNotMatch(app, /['"]\/api\/graph/);
  assert.equal(compile(app, { filename: 'UserApp.svelte', generate: 'client' }).warnings.length, 0);
});

test('scoped graph hides admin project and archive filters', () => {
  const graph = source('lib/components/KnowledgeGraph.svelte');
  assert.match(graph, /\{#if !scoped\}<label class="project-field">/);
  assert.match(graph, /\{#if !scoped\}<label class="check"><input type="checkbox" checked=\{archived\}/);
  assert.match(graph, /onsearch\(query\.trim\(\), scoped \? '' : project, scoped \? false : archived\)/);
  assert.equal(compile(graph, { filename: 'KnowledgeGraph.svelte', generate: 'client' }).warnings.length, 0);
});
