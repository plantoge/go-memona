import test from 'node:test';
import assert from 'node:assert/strict';
import { knowledgeLayout, knowledgeEdge } from '../src/lib/knowledge-graph.ts';
import type { GraphData, GraphNode } from '../src/lib/types.ts';
const node = (id: string): GraphNode => ({ id, title: id, project: 'demo', kind: 'fact', status: 'unverified', archived: false, revision: 1, updated_at: '' });
test('knowledge map is deterministic, bounded and prioritizes summary/hubs', () => {
  const nodes = Array.from({ length: 501 }, (_, i) => node(String(i)));
  nodes[500].is_project_summary = true;
  const data: GraphData = { nodes, edges: nodes.slice(1).map(n => ({ from: '0', to: n.id, type: 'related_to' })), truncated: false };
  const result = knowledgeLayout(data);
  assert.equal(result.nodes.length, 500); assert.equal(result.limited, true);
  assert.equal(result.nodes[0].id, '500'); assert.equal(result.nodes[1].id, '0');
  assert.deepEqual(knowledgeLayout({ ...data, nodes: [...nodes].reverse(), edges: [...data.edges].reverse() }), result);
  assert.ok(result.nodes.every(n => Number.isFinite(n.x) && n.x >= 65 && n.x <= 1135 && n.y >= 65 && n.y <= 735));
  const ids = new Set(result.nodes.map(n => n.id));
  assert.ok(result.edges.every(e => ids.has(e.from) && ids.has(e.to)));
});
test('500 interconnected nodes remain finite without mutating input or reporting truncation', () => {
  const nodes = Array.from({ length: 500 }, (_, i) => Object.freeze(node(String(i))));
  const edges = nodes.map((n, i) => Object.freeze({ from: n.id, to: nodes[(i + 1) % nodes.length].id, type: 'related_to' as const }));
  const data: GraphData = { nodes, edges, truncated: false };
  const before = structuredClone(data);
  Object.freeze(data.nodes); Object.freeze(data.edges); Object.freeze(data);
  const result = knowledgeLayout(data);
  assert.equal(result.nodes.length, 500); assert.equal(result.edges.length, 500);
  assert.equal(result.limited, false);
  assert.ok(result.nodes.every(n => [n.x, n.y, n.radius, n.degree].every(Number.isFinite)));
  assert.deepEqual(data, before);
});
test('isolated nodes, explicit direction and distinct edge geometry survive', () => {
  const result = knowledgeLayout({ nodes: [node('a'), node('b'), node('isolated')], edges: [{ from: 'a', to: 'b', type: 'supersedes' }], truncated: false });
  assert.equal(result.nodes.length, 3); assert.equal(result.edges.length, 1);
  const [a, b] = result.nodes;
  assert.notEqual(knowledgeEdge(a, b, 'related_to'), knowledgeEdge(a, b, 'supersedes'));
  assert.doesNotMatch(knowledgeEdge(a, a, 'related_to'), /NaN|Infinity/);
  assert.deepEqual(knowledgeLayout({ nodes: [], edges: [], truncated: false }).nodes, []);
});
