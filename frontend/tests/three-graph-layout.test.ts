import test from 'node:test';
import assert from 'node:assert/strict';
import { threeGraphLayout, graphNeighborhood } from '../src/lib/three-graph-layout.ts';
import type { GraphData, GraphNode } from '../src/lib/types.ts';
const node = (id:string): GraphNode => ({id,project:'demo',kind:'knowledge',title:id,status:'unverified',archived:false,updated_at:'',revision:1});
test('3D layout is deterministic, finite and never adds inferred edges', () => {
  const data:GraphData = { nodes:[node('a'),node('b'),node('c')],edges:[{from:'a',to:'b',type:'related_to'},{from:'b',to:'a',type:'supersedes'}],truncated:false };
  const result = threeGraphLayout(data);
  assert.deepEqual(threeGraphLayout({...data,nodes:[...data.nodes].reverse()}), result);
  assert.equal(result.nodes.length,3);
  assert.equal(result.edges.length,2);
  assert.ok(result.nodes.every(n => [n.x,n.y,n.z].every(Number.isFinite)));
  assert.deepEqual([...graphNeighborhood(result,'a')].sort(),['a','b']);
  assert.deepEqual([...graphNeighborhood(result,'c')],['c']);
  assert.equal(data.nodes[0].id,'a');
});
test('3D map includes 500 finite nodes without changing input', () => {
  const data: GraphData = { nodes:Array.from({length:500},(_,i)=>node(String(i))), edges:[], truncated:false };
  const before = structuredClone(data);
  const result = threeGraphLayout(data);
  assert.equal(result.nodes.length,500); assert.equal(result.limited,false);
  assert.ok(result.nodes.every(n => [n.x,n.y,n.z].every(Number.isFinite)));
  assert.deepEqual(data,before);
});
test('3D map stays bounded, retains isolated nodes and truncation', () => {
  const result = threeGraphLayout({ nodes:Array.from({length:501},(_,i)=>node(String(i).padStart(3,'0'))),edges:[],truncated:false });
  assert.equal(result.nodes.length,500);assert.equal(result.limited,true);
  assert.equal(threeGraphLayout({nodes:[],edges:[],truncated:true}).limited,true);
});
