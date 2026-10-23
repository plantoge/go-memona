import test from 'node:test';
import assert from 'node:assert/strict';
import { moveParticle, physicsStep, startSimulation, zoomAt, type Particle } from '../src/lib/graph-physics.ts';
const node = (id: string, x: number): Particle => ({ id, x, y: 400, vx: 0, vy: 0, radius: 4, degree: 1, project: 'demo', title: id, kind: 'fact', status: 'unverified', archived: false, revision: 1, updated_at: '' });
test('physics preserves pinned node and settles finite even with overlap and self edges', () => {
  const nodes = [node('a', 600), node('b', 600), node('c', 610)];
  const edges = [{ from: 'a', to: 'b', type: 'related_to' as const }, { from: 'a', to: 'a', type: 'related_to' as const }];
  for (let i = 0; i < 600; i++) physicsStep(nodes, edges, 1800, 110, 'a');
  assert.equal(nodes[0].x, 600); assert.equal(nodes[0].y, 400);
  assert.ok(nodes.every(n => Number.isFinite(n.x) && Number.isFinite(n.y)));
  assert.notEqual(nodes[1].x, 600);
});
test('drag placement clears both velocity axes without requiring a simulation frame', () => {
  const particle = { ...node('a', 600), vx: 7, vy: -5 };
  moveParticle(particle, { x: 700, y: 300 });
  assert.deepEqual([particle.x, particle.y, particle.vx, particle.vy], [700, 300, 0, 0]);
});

function frames() {
  let next = 0;
  const pending = new Map<number, () => void>();
  return {
    pending,
    request(callback: () => void) { pending.set(++next, callback); return next; },
    cancel(id: number) { pending.delete(id); },
    run() {
      const entry = pending.entries().next().value;
      assert.ok(entry, 'one frame should be pending');
      const [id, callback] = entry;
      pending.delete(id); callback();
    },
  };
}

test('simulation owns one frame and disposal guards even a queued callback', () => {
  const scheduler = frames(), particles = [node('a', 800)];
  const stop = startSimulation(particles, [], 1800, 110, () => '', scheduler);
  assert.equal(scheduler.pending.size, 1);
  scheduler.run();
  assert.equal(scheduler.pending.size, 1);
  const queued = [...scheduler.pending.values()][0];
  const before = { ...particles[0] };
  stop(); stop(); queued();
  assert.equal(scheduler.pending.size, 0);
  assert.deepEqual(particles[0], before);
});

test('simulation settles, reads current pin and remains bounded during dragging', () => {
  const scheduler = frames(), particles = [node('a', 600)];
  let pin = '';
  const stop = startSimulation(particles, [], 1800, 110, () => pin, scheduler);
  for (let i = 0; i < 59; i++) scheduler.run();
  assert.equal(scheduler.pending.size, 1);
  scheduler.run();
  assert.equal(scheduler.pending.size, 0, 'quiet layout stops after 60 frames');
  stop();
  const stopPinned = startSimulation(particles, [], 1800, 110, () => pin, scheduler);
  pin = 'a';
  moveParticle(particles[0], { x: 850, y: 350 });
  particles[0].vx = 5; particles[0].vy = -3;
  for (let i = 0; i < 600; i++) scheduler.run();
  assert.deepEqual([particles[0].x, particles[0].y, particles[0].vx, particles[0].vy], [850, 350, 0, 0]);
  assert.equal(scheduler.pending.size, 0, 'drag cannot run past the tick budget');
  stopPinned();
});

test('cursor zoom preserves cursor world point and clamps zoom', () => {
  const camera = { x: 100, y: -80, zoom: 2 }, point = { x: 900, y: 100 };
  const next = zoomAt(camera, point, 1.5);
  assert.ok(Math.abs((point.x - 600 - camera.x) * camera.zoom - (point.x - 600 - next.x) * next.zoom) < 1e-8);
  assert.ok(Math.abs((point.y - 400 - camera.y) * camera.zoom - (point.y - 400 - next.y) * next.zoom) < 1e-8);
  assert.equal(zoomAt(camera, point, 100).zoom, 8);
  assert.equal(zoomAt(camera, point, .001).zoom, .5);
});
