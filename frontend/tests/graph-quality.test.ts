import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { graphQuality, labelCandidates, scheduleGraphLabels } from '../src/lib/graph-quality.ts';

test('renderer retains navigation after cooldown and uses cheap links plus paused static synchronization', () => {
  const source=readFileSync(new URL('../src/lib/three-graph-renderer.ts',import.meta.url),'utf8');
  assert.match(source,/\.linkWidth\(0\)/);
  assert.match(source,/labelCandidates\(nodes, camera.position, allLabels/);
  assert.match(source,/force.cooldownTicks\(0\); force.tickFrame\(\)/);
  assert.ok(source.indexOf('controls.target.set((bounds.x') < source.indexOf('graph.zoomToFit(0, 40)'));
  const stop=source.slice(source.indexOf('.onEngineStop('),source.indexOf('// Avoid the library'));
  assert.doesNotMatch(stop,/pauseAnimation/,'cooling physics must not disable camera and picking');
});
test('quality adapts by nodes OR links without reducing graph capacity', () => {
  assert.equal(graphQuality(100,300).light,false);
  assert.equal(graphQuality(101,0).light,true);
  assert.equal(graphQuality(10,301).light,true);
  const big=graphQuality(500,1536);
  assert.equal(big.maxDPR,1);assert.equal(big.nodeResolution,6);
  assert.equal(big.cooldownTicks,60);assert.equal(big.cooldownTime,1800);
  assert.equal(big.labelLimit,24);assert.equal(big.labelInterval,80);
  assert.ok(graphQuality(20,20).maxDPR>big.maxDPR);
});
test('labels default to active nodes; opt-in nearest set is capped and does not mutate data', () => {
  const nodes=Array.from({length:500},(_,i)=>Object.freeze({id:String(i),x:i,y:0,z:0}));
  const camera={x:0,y:0,z:0};
  assert.deepEqual(labelCandidates(nodes,camera,false,[],24),[]);
  assert.deepEqual(labelCandidates(nodes,camera,false,['499','499','bad'],24).map(n=>n.id),['499']);
  const selected=labelCandidates(nodes,camera,true,['499'],24);
  assert.equal(selected.length,24);assert.equal(selected[0].id,'499');assert.equal(selected[1].id,'0');
  assert.equal(nodes[0].id,'0');assert.equal(nodes[499].x,499);
});
test('label scheduler coalesces bursts, delivers trailing update and cancels timers/frames', () => {
  let now=0,id=0,runs=0;const timers=new Map<number,{fn:()=>void;due:number}>(),frames=new Map<number,()=>void>();
  const clock={now:()=>now,delay:(fn:()=>void,ms:number)=>{timers.set(++id,{fn,due:now+ms});return id;},cancelDelay:(key:number)=>{timers.delete(key);},frame:(fn:()=>void)=>{frames.set(++id,fn);return id;},cancelFrame:(key:number)=>{frames.delete(key);}};
  const flush=()=>{const work=[...frames.values()];frames.clear();work.forEach(fn=>fn());};
  const scheduler=scheduleGraphLabels(()=>runs++,()=>80,clock);
  for(let i=0;i<500;i++)scheduler.schedule();
  assert.equal(frames.size,1);flush();assert.equal(runs,1);
  now=10;for(let i=0;i<500;i++)scheduler.schedule();
  assert.equal(timers.size,1);assert.equal([...timers.values()][0].due,80);
  now=80;const trailing=[...timers.values()][0];timers.clear();trailing.fn();flush();assert.equal(runs,2);
  now=90;scheduler.schedule();const late=[...timers.values()][0].fn;
  scheduler.dispose();late();scheduler.schedule();flush();assert.equal(runs,2);assert.equal(timers.size,0);
  const another=scheduleGraphLabels(()=>runs++,()=>80,clock);another.schedule();const stale=[...frames.values()][0];another.dispose();stale();assert.equal(runs,2);assert.equal(frames.size,0);
});
