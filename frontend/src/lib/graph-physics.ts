import type { GraphEdge } from './types';
import type { KnowledgeNode } from './knowledge-graph';
import type { Camera } from './graph';

export interface Particle extends KnowledgeNode { vx: number; vy: number }

// Pointer placement must also discard momentum when animation is paused or
// reduced motion is enabled (there may be no physics step before release).
export function moveParticle(node: Particle, point: { x: number; y: number }) {
  node.x = point.x; node.y = point.y; node.vx = 0; node.vy = 0;
}

export function physicsStep(nodes: Particle[], edges: GraphEdge[], strength: number, distance: number, pinned = '') {
  const byID = new Map(nodes.map(n => [n.id, n]));
  for (let i = 0; i < nodes.length; i++) for (let j = i + 1; j < nodes.length; j++) {
    const a = nodes[i], b = nodes[j];
    const dx = a.x - b.x || .01, dy = a.y - b.y || .01, d = Math.max(8, Math.hypot(dx, dy));
    const f = Math.min(3, strength / (d * d));
    a.vx += dx / d * f; a.vy += dy / d * f; b.vx -= dx / d * f; b.vy -= dy / d * f;
  }
  for (const edge of edges) {
    const a = byID.get(edge.from), b = byID.get(edge.to); if (!a || !b || a === b) continue;
    const dx = b.x - a.x, dy = b.y - a.y, d = Math.max(1, Math.hypot(dx, dy));
    const f = (d - distance) * .007;
    a.vx += dx / d * f; a.vy += dy / d * f; b.vx -= dx / d * f; b.vy -= dy / d * f;
  }
  let energy = 0;
  for (const n of nodes) {
    if (n.id === pinned) { n.vx = 0; n.vy = 0; continue; }
    n.vx = (n.vx + (600 - n.x) * .0008) * .78;
    n.vy = (n.vy + (400 - n.y) * .0008) * .78;
    n.x += Math.max(-8, Math.min(8, n.vx)); n.y += Math.max(-8, Math.min(8, n.vy));
    energy += Math.abs(n.vx) + Math.abs(n.vy);
  }
  return energy;
}

interface FrameScheduler {
  request: (callback: () => void) => number;
  cancel: (id: number) => void;
}

// The caller owns one loop and calls the returned disposer on pause, filter
// change, reduced motion, focus or unmount. Read the dragged ID at frame time.
export function startSimulation(nodes: Particle[], edges: GraphEdge[], strength: number, distance: number, pinned: () => string, scheduler: FrameScheduler = { request: callback => requestAnimationFrame(callback), cancel: id => cancelAnimationFrame(id) }) {
  let frame = 0, ticks = 0, cancelled = false;
  const animate = () => {
    if (cancelled) return;
    const pinnedID = pinned();
    const energy = physicsStep(nodes, edges, strength, distance, pinnedID);
    ticks++;
    if (ticks < 600 && (ticks < 60 || energy > .15 || !!pinnedID)) frame = scheduler.request(animate);
  };
  frame = scheduler.request(animate);
  return () => { cancelled = true; scheduler.cancel(frame); };
}

// Preserve the world point under the cursor, including the existing pan offset.
export function zoomAt(camera: Camera, point: { x: number; y: number }, factor: number): Camera {
  const zoom = Math.max(.5, Math.min(8, camera.zoom * factor));
  return { x: camera.x + (point.x - 600 - camera.x) * (1 - camera.zoom / zoom), y: camera.y + (point.y - 400 - camera.y) * (1 - camera.zoom / zoom), zoom };
}
