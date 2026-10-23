export interface GraphQuality {
  light: boolean; maxDPR: number; nodeResolution: number;
  cooldownTicks: number; cooldownTime: number; labelLimit: number; labelInterval: number;
}
/** Counts affect visual quality only, never graph membership or stored data. */
export function graphQuality(nodes: number, edges: number): GraphQuality {
  const light = nodes > 100 || edges > 300;
  return { light, maxDPR: light ? 1 : 1.5, nodeResolution: light ? 6 : 12,
    cooldownTicks: light ? 60 : 120, cooldownTime: light ? 1800 : 3000,
    labelLimit: light ? 24 : 48, labelInterval: light ? 80 : 50 };
}
export interface LabelCandidate { id: string; x: number; y: number; z: number }
/** Default: hover/selection only. Opt-in labels: nearest bounded set to camera. */
export function labelCandidates<T extends LabelCandidate>(nodes: T[], camera: {x:number;y:number;z:number}, all: boolean, active: string[], limit: number): T[] {
  const wanted = new Set(active.filter(Boolean));
  const priority = nodes.filter(n => wanted.has(n.id));
  if (!all) return priority;
  const distance = (n:T) => (n.x-camera.x)**2 + (n.y-camera.y)**2 + (n.z-camera.z)**2;
  return [...priority, ...nodes.filter(n => !wanted.has(n.id)).sort((a,b)=>distance(a)-distance(b) || a.id.localeCompare(b.id))].slice(0,Math.max(priority.length,limit));
}

/** Bounded trailing scheduler: no idle RAF polling, cancel all work at teardown. */
export function scheduleGraphLabels(run: () => void, interval: () => number, clock: {
  now(): number; delay(callback: () => void, ms: number): number; cancelDelay(id:number): void;
  frame(callback: () => void): number; cancelFrame(id:number): void;
}) {
  let timer: number | undefined, frame: number | undefined, last = -Infinity, disposed = false;
  function schedule() {
    if (disposed || timer !== undefined || frame !== undefined) return;
    const enqueue = () => {
      timer = undefined;
      if (disposed) return;
      frame = clock.frame(() => { frame = undefined; if (!disposed) { last = clock.now(); run(); } });
    };
    const wait = Math.max(0, interval() - (clock.now() - last));
    if (wait) timer = clock.delay(enqueue,wait); else enqueue();
  }
  return { schedule, dispose() {
    disposed = true;
    if (timer !== undefined) clock.cancelDelay(timer);
    if (frame !== undefined) clock.cancelFrame(frame);
    timer = frame = undefined;
  } };
}
