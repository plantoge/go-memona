import test from 'node:test';
import assert from 'node:assert/strict';
import { graphData, layoutGraph, edgePath, zoomCamera, cameraViewBox, INITIAL_CAMERA, NODE_WIDTH, NODE_HEIGHT } from '../src/lib/graph.ts';
import type { GraphData, GraphNode } from '../src/lib/types.ts';

const node = (id: string, project = 'demo'): GraphNode => ({ id, project, title: id, kind: 'fact', status: 'unverified', archived: false, updated_at: '2026-10-22T00:00:00Z', revision: 1 });
const data: GraphData = { nodes: [node('c'), node('b'), node('a'), node('isolated')], edges: [{ from: 'a', to: 'b', type: 'related_to' }, { from: 'b', to: 'a', type: 'supersedes' }], truncated: false };

test('graph normalization keeps isolated nodes and only explicit directed edges', () => {
  const result = graphData({ ...data, edges: [...data.edges, data.edges[0], { from: 'a', to: 'missing', type: 'supersedes' }, { from: 'b', to: 'c', type: 'inferred' }] });
  assert.equal(result.nodes.length, 4);
  assert.deepEqual(result.edges, data.edges);
  assert.equal(result.edges.some(edge => edge.from === 'b' && edge.to === 'a' && edge.type === 'related_to'), false);
  assert.deepEqual(graphData({ nodes: null, edges: null, truncated: false }), { nodes: [], edges: [], truncated: false });
});

test('graph normalization bounds server responses and retains truncation', () => {
  const nodes = Array.from({ length: 502 }, (_, index) => node(String(index)));
  const result = graphData({ nodes, edges: [{ from: '0', to: '501', type: 'related_to' }], truncated: false });
  assert.equal(result.nodes.length, 500); assert.equal(result.truncated, true); assert.deepEqual(result.edges, []);
  assert.equal(graphData({ ...data, truncated: true }).truncated, true);
  for (const invalid of [null, {}, { ...data, nodes: {} }, { ...data, nodes: [node('a'), node('a')] }, { ...data, nodes: [{ ...node('a'), title: null }] }]) assert.throws(() => graphData(invalid));
});

test('layout is deterministic regardless of response order, preserves cycles and isolated nodes', () => {
  const result = layoutGraph(data);
  assert.deepEqual(layoutGraph({ ...data, nodes: [...data.nodes].reverse(), edges: [...data.edges].reverse() }), result);
  assert.equal(result.nodes.length, 4); assert.equal(result.edges.length, 2);
  assert.ok(result.nodes.some(node => node.id === 'isolated'));
  for (let i = 0; i < result.nodes.length; i++) {
    const a = result.nodes[i];
    assert.ok(a.x >= 0 && a.y >= 0 && a.x + NODE_WIDTH <= result.width && a.y + NODE_HEIGHT <= result.height);
    for (const b of result.nodes.slice(i + 1)) assert.ok(a.x + NODE_WIDTH <= b.x || b.x + NODE_WIDTH <= a.x || a.y + NODE_HEIGHT <= b.y || b.y + NODE_HEIGHT <= a.y);
  }
  assert.deepEqual(layoutGraph({ nodes: [], edges: [], truncated: false }).nodes, []);
});

test('bounded dense graph layout is finite and retains all explicit relationships', () => {
  const nodes = Array.from({ length: 500 }, (_, index) => node(String(index)));
  const edges = nodes.slice(1).map(item => ({ from: '0', to: item.id, type: 'related_to' as const }));
  const result = layoutGraph({ nodes, edges, truncated: false });
  assert.equal(result.nodes.length, 500); assert.equal(result.edges.length, 499);
  assert.ok(result.nodes.every(node => Number.isFinite(node.x) && Number.isFinite(node.y)));
});

test('edge geometry handles reciprocal, multiple types, vertical and self edges', () => {
  const a = { ...node('a'), x: 40, y: 40 }, b = { ...node('b'), x: 280, y: 40 };
  const path = edgePath(a, b, 'related_to');
  assert.notEqual(path, edgePath(b, a, 'related_to'));
  assert.notEqual(path, edgePath(a, b, 'supersedes'));
  for (const value of [path, edgePath(a, a, 'related_to'), edgePath(a, { ...b, x: a.x, y: 200 }, 'related_to')]) assert.doesNotMatch(value, /NaN|Infinity/);
});

test('camera zoom is clamped and reset/viewbox are deterministic', () => {
  assert.deepEqual(zoomCamera(INITIAL_CAMERA, 100), { x: 0, y: 0, zoom: 8 });
  assert.deepEqual(zoomCamera(INITIAL_CAMERA, 0.001), { x: 0, y: 0, zoom: 0.5 });
  assert.equal(cameraViewBox({ width: 800, height: 400 }, INITIAL_CAMERA), '0 0 800 400');
  assert.equal(cameraViewBox({ width: 800, height: 400 }, { x: 10, y: -10, zoom: 2 }), '210 90 400 200');
  assert.deepEqual(INITIAL_CAMERA, { x: 0, y: 0, zoom: 1 });
});
