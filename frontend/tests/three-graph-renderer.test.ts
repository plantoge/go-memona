import test from 'node:test';
import assert from 'node:assert/strict';
import { forceGraphData, type PositionedGraphNode } from '../src/lib/force-graph-data.ts';
import type { GraphEdge } from '../src/lib/types.ts';
const node = (id:string): PositionedGraphNode => ({id,project:'demo',kind:'knowledge',title:'<script>fixture</script>',status:'unverified',revision:1,updated_at:'',archived:false,x:0,y:0,z:0,degree:1});
test('force engine gets copies: positions and endpoint mutations never change API state', () => {
  const nodes = Object.freeze([Object.freeze(node('a')),Object.freeze(node('b'))]);
  const edges = Object.freeze([Object.freeze({from:'a',to:'b',type:'related_to' as const})]);
  const data = forceGraphData([...nodes],[...edges]);
  assert.equal(data.nodes.length,2); assert.equal(data.links.length,1);
  data.nodes[0].x = 99;
  // The force engine resolves IDs into objects in its private graph data.
  Object.assign(data.links[0],{source:data.nodes[0],target:data.nodes[1]});
  assert.equal(nodes[0].x,0);assert.equal(edges[0].from,'a');
  assert.equal(data.nodes[0].title,'<script>fixture</script>','data stays plain text; tooltips must not use it as HTML');
});
test('force data keeps all 500 nodes with finite independent coordinates', () => {
  const nodes = Array.from({length:500},(_,i)=>Object.freeze(node(String(i))));
  const before = structuredClone(nodes);
  const data = forceGraphData(nodes,[]);
  assert.equal(data.nodes.length,500);
  assert.ok(data.nodes.every(n => [n.x,n.y,n.z].every(Number.isFinite)));
  data.nodes[499].x = 123;
  assert.deepEqual(nodes,before);
});
test('force data preserves directions/types/cycles, drops dangling links and caps nodes', () => {
  const nodes = Array.from({length:501},(_,i)=>node(String(i)));
  const edges:GraphEdge[] = [{from:'0',to:'1',type:'related_to'},{from:'1',to:'0',type:'supersedes'},{from:'0',to:'0',type:'related_to'},{from:'0',to:'500',type:'related_to'}];
  const data=forceGraphData(nodes,edges);
  assert.equal(data.nodes.length,500);
  assert.deepEqual(data.links.map(e=>[e.source,e.target,e.type]),[['0','1','related_to'],['1','0','supersedes'],['0','0','related_to']]);
  assert.deepEqual(forceGraphData([],edges),{nodes:[],links:[]});
});
