import ForceGraph3D, { type ForceGraph3DInstance, type LinkObject, type NodeObject } from '3d-force-graph';
import * as THREE from 'three';
import type { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { forceGraphData } from './force-graph-data.ts';
import { KNOWLEDGE_LIMIT } from './knowledge-graph.ts';
import { graphQuality, labelCandidates, scheduleGraphLabels } from './graph-quality.ts';
import type { GraphEdge, GraphNode } from './types.ts';

export type GraphSceneNode = GraphNode & { x: number; y: number; z: number; degree: number };
export interface GraphLabel { id: string; x: number; y: number; visible: boolean }
interface GraphCallbacks {
  onSelect(id: string): void;
  onOpen(id: string): void;
  onHover(id: string): void;
  onLabels(labels: GraphLabel[]): void;
  onFailure(): void;
}
export interface ThreeGraphController {
  setData(nodes: GraphSceneNode[], edges: GraphEdge[]): void;
  setTheme(dark: boolean): void;
  setSelection(id: string, hover: string): void;
  setPaused(paused: boolean): void;
  setReducedMotion(reduced: boolean): void;
  setLabels(all: boolean): void;
  zoom(factor: number): void;
  reset(): void;
  focus(id: string): void;
  /** Screen-space pixels, positive right/down. */
  pan(x: number, y: number): void;
  /** Orbit by screen-space pixels, positive right/down. */
  rotate(x: number, y: number): void;
  dispose(): void;
}

type LocalNode = GraphSceneNode & NodeObject;
type LocalLink = GraphEdge & LinkObject<LocalNode>;
const palettes = {
  light: { background: '#f6f3ec', node: ['#528b89', '#7289a2', '#a58d61', '#8c7b9d', '#79926e'], selected: '#245f64', edge: '#a0aaa7', supersedes: '#c78045' },
  dark: { background: '#1c2225', node: ['#79aaa5', '#8c9fb6', '#b9a379', '#aa94b7', '#97ae8c'], selected: '#c2e6df', edge: '#687d80', supersedes: '#d89a62' }
};
const finite = (...values: number[]) => values.every(Number.isFinite);
function kindIndex(kind: string) {
  let hash = 0;
  for (const char of kind) hash = (hash * 31 + char.charCodeAt(0)) | 0;
  return (hash >>> 0) % 5;
}
function radius(node: LocalNode) {
  return 3.5 + Math.min(5, Math.sqrt(Math.max(0, node.degree))) + (node.is_project_summary ? 2 : 0);
}
function alpha(color: string, opacity: number) {
  const value = new THREE.Color(color).convertLinearToSRGB();
  return `rgba(${Math.round(value.r * 255)},${Math.round(value.g * 255)},${Math.round(value.b * 255)},${opacity})`;
}

/** Real ForceGraph3D/d3 view, with local cloned simulation data only.
 * The library owns its render loop (including idle pointer picking). pauseAnimation
 * stops that loop; we also disable interactions while hidden/manually paused.
 * Parent labels use a coalesced event-driven RAF, never a second continuous loop.
 */
export function createThreeGraph(host: HTMLElement, callbacks: GraphCallbacks): ThreeGraphController {
  const mount = document.createElement('div');
  const canvas = document.createElement('canvas');
  canvas.setAttribute('aria-hidden', 'true');
  canvas.style.cssText = 'display:block;touch-action:none;';
  host.appendChild(mount);
  let graph: ForceGraph3DInstance<LocalNode, LocalLink> | undefined;
  let controls: OrbitControls | undefined;
  let observer: ResizeObserver | undefined;
  let disposed = false, failed = false, released = false, failureReported = false;
  let paused = false, reduced = false, dark = false, selected = '', hovered = '';
  let width = 1, height = 1, fitted = false, fitPending = false, allLabels = false, staticPending = false;
  let quality = graphQuality(0, 0);
  let nodes: LocalNode[] = [], links: LocalLink[] = [];
  const positions = new Map<string, { x: number; y: number; z: number }>();
  const projected = new THREE.Vector3();
  // The dependency exposes a debugging scene on window. Do not retain our disposed scene.
  const debugWindow = window as Window & { scene?: THREE.Scene };
  const previousScene = debugWindow.scene;
  const alive = () => !disposed && !failed;

  function guard(action: () => void) {
    if (!alive()) return;
    try { action(); } catch { fail(); }
  }
  function fail() {
    if (!alive()) return;
    failed = true;
    releaseResources();
    // Caller must have received its inert controller before fallback changes state.
    queueMicrotask(() => {
      if (!disposed && !failureReported) {
        failureReported = true;
        try { callbacks.onFailure(); } catch { /* A parent failure must not restart rendering. */ }
      }
    });
  }
  const labelWork = scheduleGraphLabels(updateLabels, () => quality.labelInterval, {
    now: () => performance.now(), delay: (fn, ms) => window.setTimeout(fn, ms), cancelDelay: id => window.clearTimeout(id),
    frame: fn => requestAnimationFrame(fn), cancelFrame: id => cancelAnimationFrame(id),
  });
  function invalidate() { if (alive()) labelWork.schedule(); }
  function rememberPositions() {
    for (const node of nodes) {
      if (finite(node.x, node.y, node.z)) positions.set(node.id, { x: node.x, y: node.y, z: node.z });
    }
    // Bound the session-only coordinate cache as well as the visible graph.
    while (positions.size > KNOWLEDGE_LIMIT) positions.delete(positions.keys().next().value!);
  }
  function updateLabels() {
    if (document.hidden) return;
    guard(() => {
      if (!graph) return;
      if (staticPending) {
        staticPending = false;
        // ForceGraph3D omits tickFrame from its facade, but exposes its scene.
        // Synchronize the inner force object's mesh positions once, without a
        // physics step or resuming the library's continuous render loop.
        graph.scene().traverse(object => {
          const force = object as THREE.Object3D & { tickFrame?: () => void; cooldownTicks?: (value: number) => unknown };
          if (typeof force.tickFrame === 'function' && typeof force.cooldownTicks === 'function') {
            force.cooldownTicks(0); force.tickFrame();
            force.cooldownTicks(reduced ? 0 : quality.cooldownTicks);
          }
        });
      }
      if (fitPending && nodes.length) {
        fitPending = false;
        fitted = true;
        fit();
      }
      const camera = graph.camera() as THREE.PerspectiveCamera;
      camera.updateMatrixWorld();
      // Paused updates still need one static render for theme/keyboard/resize changes.
      if (paused || document.hidden) graph.renderer().render(graph.scene(), camera);
      const ordered = labelCandidates(nodes, camera.position, allLabels, [hovered, selected], quality.labelLimit);
      const labels = ordered.map(node => {
        const point = graph!.graph2ScreenCoords(node.x, node.y, node.z);
        projected.set(node.x, node.y, node.z).project(camera);
        const distance = camera.position.distanceTo(new THREE.Vector3(node.x, node.y, node.z));
        const offset = Math.min(35, radius(node) * height / (2 * Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)) * Math.max(distance, .1))) + 8;
        return { id: node.id, x: point.x, y: point.y - offset, visible: finite(point.x, point.y, projected.z) && projected.z >= -1 && projected.z <= 1 && point.x >= 0 && point.x <= width && point.y >= 0 && point.y <= height };
      });
      callbacks.onLabels(labels);
    });
  }
  function applyColors() {
    if (!graph) return;
    const palette = dark ? palettes.dark : palettes.light;
    const active = hovered || selected;
    const neighbors = new Set(active ? [active] : []);
    for (const link of links) {
      if (link.from === active || link.to === active) { neighbors.add(link.from); neighbors.add(link.to); }
    }
    const linkColor = (link: LocalLink) => alpha(link.type === 'supersedes' ? palette.supersedes : palette.edge,
      active && link.from !== active && link.to !== active ? .16 : .85);
    graph.backgroundColor(palette.background)
      .nodeColor(node => alpha(node.id === selected || node.id === hovered ? palette.selected : palette.node[kindIndex(node.kind)], active && !neighbors.has(node.id) ? .36 : 1))
      .linkColor(linkColor).linkDirectionalArrowColor(linkColor)
      .linkDirectionalArrowLength(link => active && (link.from === active || link.to === active) ? 4 : 0);
    invalidate();
  }
  function applyQuality() {
    if (!graph) return;
    graph.nodeResolution(quality.nodeResolution)
      .warmupTicks(0).cooldownTicks(reduced ? 0 : quality.cooldownTicks)
      .cooldownTime(quality.cooldownTime).enableNodeDrag(!reduced);
    resize();
  }
  function fit() {
    if (!graph || !controls || !nodes.length) return;
    // Zero duration: neither reduced-motion mode nor normal updates animate the camera.
    const bounds = graph.getGraphBbox();
    if (bounds) controls.target.set((bounds.x[0]+bounds.x[1])/2, (bounds.y[0]+bounds.y[1])/2, (bounds.z[0]+bounds.z[1])/2);
    graph.zoomToFit(0, 40);
    controls.update();
    invalidate();
  }
  function resize() {
    guard(() => {
      if (!graph) return;
      width = Math.max(1, host.clientWidth);
      height = Math.max(1, host.clientHeight);
      graph.width(width).height(height);
      graph.renderer().setPixelRatio(Math.min(quality.maxDPR, window.devicePixelRatio || 1));
      invalidate();
    });
  }
  function syncPause() {
    if (!graph || !controls) return;
    const stopped = paused || document.hidden;
    // pauseAnimation alone stops rendering, not OrbitControls/drag event listeners.
    canvas.style.pointerEvents = stopped ? 'none' : '';
    graph.enableNavigationControls(!stopped).enablePointerInteraction(!stopped);
    controls.enabled = !stopped;
    if (stopped) { graph.pauseAnimation(); staticPending = true; }
    else graph.resumeAnimation();
    invalidate();
  }
  function visibilityChanged() { guard(syncPause); }
  function contextLost(event: Event) { event.preventDefault(); fail(); }
  function releaseResources() {
    if (released) return;
    released = true;
    labelWork.dispose();
    observer?.disconnect();
    window.removeEventListener('resize', resize);
    document.removeEventListener('visibilitychange', visibilityChanged);
    controls?.removeEventListener('change', invalidate);
    canvas.removeEventListener('webglcontextlost', contextLost);
    const renderer = graph?.renderer();
    const scene = graph?.scene();
    // _destructor disposes scene geometries/materials, controls, renderer and composer.
    // Context loss is additional: the library does not explicitly release the context.
    try { graph?._destructor(); } catch { /* Continue releasing even after context loss. */ }
    // If context loss happened inside the library's current frame, that frame may
    // schedule its next RAF after _destructor returns. Cancel that final request.
    queueMicrotask(() => { graph?.pauseAnimation(); });
    try { controls?.dispose(); renderer?.renderLists.dispose(); renderer?.dispose(); renderer?.forceContextLoss(); } catch { /* Already lost/disposed. */ }
    if (!renderer) {
      // A constructor may obtain a GPU context before throwing; own canvas permits cleanup.
      try { canvas.getContext('webgl2')?.getExtension('WEBGL_lose_context')?.loseContext(); } catch { /* No usable WebGL context. */ }
    }
    if (scene && debugWindow.scene === scene) debugWindow.scene = previousScene;
    else if (!graph && debugWindow.scene !== previousScene) debugWindow.scene = previousScene;
    mount.remove();
    nodes = []; links = []; positions.clear();
  }
  function cameraAction(action: (camera: THREE.PerspectiveCamera, orbit: OrbitControls) => void) {
    guard(() => {
      if (!graph || !controls) return;
      action(graph.camera() as THREE.PerspectiveCamera, controls);
      controls.update();
      invalidate();
    });
  }

  const controller: ThreeGraphController = {
    setData(inputNodes, inputEdges) {
      guard(() => {
        if (!graph) return;
        rememberPositions();
        const seen = new Set<string>();
        const valid = inputNodes.filter(node => {
          if (!node || typeof node.id !== 'string' || !node.id.trim() || seen.has(node.id) || !finite(node.x, node.y, node.z, node.degree) || Math.max(Math.abs(node.x), Math.abs(node.y), Math.abs(node.z)) > 1e7) return false;
          seen.add(node.id); return true;
        });
        const data = forceGraphData(valid, inputEdges.filter(edge => edge && (edge.type === 'related_to' || edge.type === 'supersedes')));
        nodes = data.nodes;
        links = data.links;
        for (const node of nodes) {
          const old = positions.get(node.id);
          if (old) Object.assign(node, old);
        }
        // Only cloned objects go into d3, which replaces link IDs with node objects.
        // No camera refit on filtering or normal data updates.
        quality = graphQuality(nodes.length, links.length);
        applyQuality();
        graph.graphData({ nodes, links });
        staticPending = paused || document.hidden;
        if (staticPending && !fitted) fitPending = true;
        applyColors();
        invalidate();
      });
    },
    setTheme(value) { guard(() => { dark = value; applyColors(); }); },
    setSelection(id, hover) { guard(() => { if (selected === id && hovered === hover) return; selected = id; hovered = hover; applyColors(); }); },
    setLabels(value) { guard(() => { allLabels = value; invalidate(); }); },
    setPaused(value) { guard(() => { paused = value; syncPause(); }); },
    setReducedMotion(value) {
      guard(() => {
        reduced = value;
        if (!graph) return;
        // The already-computed deterministic seed is the static reduced-motion
        // layout. No synchronous 100-tick d3 warmup or animated camera is needed.
        applyQuality();
        if (!reduced) graph.d3ReheatSimulation();
        if (controls) { controls.enableDamping = false; controls.autoRotate = false; }
        invalidate();
      });
    },
    zoom(factor) {
      if (!Number.isFinite(factor) || factor <= 0) return;
      cameraAction((camera, orbit) => {
        const offset = camera.position.clone().sub(orbit.target);
        offset.setLength(THREE.MathUtils.clamp(offset.length() / factor, orbit.minDistance, orbit.maxDistance));
        camera.position.copy(orbit.target).add(offset);
      });
    },
    reset() { guard(fit); },
    focus(id) {
      cameraAction((camera, orbit) => {
        const node = nodes.find(node => node.id === id);
        if (!node) return;
        const offset = camera.position.clone().sub(orbit.target);
        offset.setLength(THREE.MathUtils.clamp(Math.max(radius(node) * 16, offset.length() * .65), orbit.minDistance, orbit.maxDistance));
        orbit.target.set(node.x, node.y, node.z);
        camera.position.copy(orbit.target).add(offset);
      });
    },
    pan(x, y) {
      if (!finite(x, y)) return;
      cameraAction((camera, orbit) => {
        camera.updateMatrixWorld();
        const scale = 2 * camera.position.distanceTo(orbit.target) * Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)) / height;
        const shift = new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, 0).multiplyScalar(x * scale)
          .addScaledVector(new THREE.Vector3().setFromMatrixColumn(camera.matrixWorld, 1), -y * scale);
        camera.position.add(shift); orbit.target.add(shift);
      });
    },
    rotate(x, y) {
      if (!finite(x, y)) return;
      cameraAction((camera, orbit) => {
        const spherical = new THREE.Spherical().setFromVector3(camera.position.clone().sub(orbit.target));
        spherical.theta -= x * .008;
        spherical.phi = THREE.MathUtils.clamp(spherical.phi + y * .008, .02, Math.PI - .02);
        camera.position.copy(orbit.target).add(new THREE.Vector3().setFromSpherical(spherical));
      });
    },
    dispose() { if (!disposed) { disposed = true; releaseResources(); } }
  };

  try {
    canvas.addEventListener('webglcontextlost', contextLost);
    graph = new ForceGraph3D(mount, { controlType: 'orbit', rendererConfig: { canvas, antialias: true, alpha: false, powerPreference: 'low-power' } }) as unknown as ForceGraph3DInstance<LocalNode, LocalLink>;
    controls = graph.controls() as OrbitControls;
    controls.enableDamping = false;
    controls.autoRotate = false;
    controls.minDistance = 3;
    controls.maxDistance = 100000;
    controls.addEventListener('change', invalidate);
    graph.forceEngine('d3').cooldownTicks(quality.cooldownTicks).cooldownTime(quality.cooldownTime)
      .showNavInfo(false).nodeLabel(() => '').linkLabel(() => '')
      .nodeRelSize(1).nodeVal(node => radius(node) ** 3).nodeOpacity(1)
      .linkOpacity(.55).linkWidth(0)
      .linkCurvature(link => link.from === link.to ? .65 : 0)
      .linkDirectionalArrowLength(0).linkDirectionalArrowRelPos(1)
      .linkDirectionalParticles(0).enableNodeDrag(true)
      .onNodeClick(node => guard(() => callbacks.onSelect(node.id)))
      .onBackgroundClick(() => guard(() => callbacks.onSelect('')))
      .onNodeHover(node => guard(() => callbacks.onHover(node?.id || '')))
      .onNodeDrag(() => { if (alive()) invalidate(); })
      .onNodeDragEnd(() => { if (alive()) { rememberPositions(); invalidate(); } })
      .onEngineTick(() => { if (alive()) invalidate(); })
      .onEngineStop(() => {
        if (!alive()) return;
        rememberPositions();
        if (!fitted && nodes.length) fitPending = true;
        invalidate();
      });
    // Avoid the library's default auto-reaim on graphData changes; fit once at rest.
    graph.cameraPosition({ x: .01, y: 40, z: 300 });
    observer = new ResizeObserver(resize);
    observer.observe(host);
    window.addEventListener('resize', resize);
    document.addEventListener('visibilitychange', visibilityChanged);
    resize(); applyColors(); syncPause();
  } catch { fail(); }
  return controller;
}
