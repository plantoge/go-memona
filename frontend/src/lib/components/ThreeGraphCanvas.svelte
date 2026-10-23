<script lang="ts">
  import { onMount } from 'svelte';
  import type { GraphEdge } from '../types';
  import type { GraphSceneNode, GraphLabel, ThreeGraphController } from '../three-graph-renderer';
  let { nodes, edges, selected = '', labels = false, paused = false, onselect, onopen }: {
    nodes: GraphSceneNode[]; edges: GraphEdge[]; selected?: string; labels?: boolean; paused?: boolean;
    onselect: (id: string) => void; onopen: (id: string) => void;
  } = $props();
  let host: HTMLDivElement;
  let controller = $state<ThreeGraphController>();
  let projected = $state<GraphLabel[]>([]), hovered = $state(''), failed = $state(false);
  const names = $derived(new Map(nodes.map(n => [n.id, n.title])));
  let hidden = $state(false);
  onMount(() => {
    let closed = false;
    let observer: MutationObserver | undefined;
    const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
    const reduced = () => controller?.setReducedMotion(motion.matches);
    const visibility = () => { hidden = document.hidden; };
    visibility(); document.addEventListener('visibilitychange', visibility);
    motion.addEventListener('change', reduced);
    void import('../three-graph-renderer').then(({ createThreeGraph }) => {
      if (closed) return;
      try {
        const scene = createThreeGraph(host, {
          onSelect: onselect, onOpen: onopen, onHover: id => { hovered = id; },
          onLabels: next => { projected = next; },
          onFailure: () => { failed = true; },
        });
        if (closed) { scene.dispose(); return; }
        controller = scene;
        reduced(); scene.setPaused(paused || hidden);
        const theme = () => scene.setTheme(document.documentElement.dataset.theme === 'dark');
        theme(); observer = new MutationObserver(theme);
        observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
      } catch { failed = true; }
    }).catch(() => { if (!closed) failed = true; });
    return () => { closed = true; observer?.disconnect(); document.removeEventListener('visibilitychange', visibility); motion.removeEventListener('change', reduced); controller?.dispose(); };
  });
  $effect(() => { controller?.setData(nodes, edges); });
  $effect(() => { controller?.setSelection(selected, hovered); });
  $effect(() => { controller?.setLabels(labels); });
  $effect(() => { if (failed) controller?.dispose(); });
  $effect(() => { controller?.setPaused(paused || hidden); });
  export function zoom(factor: number) { controller?.zoom(factor); }
  export function reset() { controller?.reset(); }
  export function focus(id: string) { controller?.focus(id); }
  export function pan(x: number, y: number) { controller?.pan(x, y); }
  export function rotate(x: number, y: number) { controller?.rotate(x, y); }
  function key(event: KeyboardEvent) {
    if (event.target !== event.currentTarget) return;
    if (event.shiftKey && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) {
      event.preventDefault();
      rotate(event.key === 'ArrowLeft' ? -40 : event.key === 'ArrowRight' ? 40 : 0, event.key === 'ArrowUp' ? -40 : event.key === 'ArrowDown' ? 40 : 0);
      return;
    }
    const actions: Record<string, () => void> = {
      ArrowLeft: () => pan(-60, 0), ArrowRight: () => pan(60, 0), ArrowUp: () => pan(0, -60), ArrowDown: () => pan(0, 60),
      '+': () => zoom(1.2), '=': () => zoom(1.2), '-': () => zoom(.8), '0': reset, Escape: () => onselect(''),
    };
    if (actions[event.key]) { event.preventDefault(); actions[event.key](); }
  }
</script>

<!-- Keyboard camera controls supplement the compact search-based note picker. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div class="scene" role="region" aria-label="Peta relasi Three.js interaktif" tabindex="0" onkeydown={key}>
  <div class="webgl" bind:this={host} aria-hidden="true"></div>
  {#if failed}<div class="fallback"><strong role="status">Visual 3D tidak tersedia.</strong><p>WebGL tidak didukung atau konteks grafis terputus. Pilih memori di bawah untuk membaca isinya; tidak ada data yang hilang.</p><label class="sr-only" for="fallback-note">Pilih memori</label><select id="fallback-note" onchange={event => { if (event.currentTarget.value) onopen(event.currentTarget.value); }}><option value="">Pilih memori untuk dibuka…</option>{#each nodes as node (node.id)}<option value={node.id}>{node.title}</option>{/each}</select></div>
  {:else}
    <div class="labels" aria-hidden="true">{#each projected.filter(p => p.visible) as point (point.id)}<span class:active={point.id === selected || point.id === hovered} style:left={`${point.x}px`} style:top={`${point.y}px`}>{(names.get(point.id) || '').slice(0, 55)}</span>{/each}</div>
    {#if !controller}<p class="initializing" role="status">Menyiapkan visual 3D…</p>{/if}
  {/if}
</div>

<style>
  .scene { position: relative; height: 100%; min-height: 420px; overflow: hidden; outline-offset: -3px; }
  .scene:focus-visible { outline: 2px solid var(--ring); }
  .webgl { position: absolute; inset: 0; touch-action: none; }
  .webgl :global(canvas) { width: 100%; height: 100%; display: block; cursor: grab; }
  .webgl :global(canvas:active) { cursor: grabbing; }
  /* Local stylesheet: dependency-injected style tags are not trusted by CSP. */
  .webgl :global(.graph-info-msg) { position: absolute; top: 50%; width: 100%; text-align: center; color: var(--muted); font: inherit; }
  .webgl :global(.scene-nav-info), .webgl :global(.float-tooltip) { display: none; }
  .webgl :global(.clickable) { cursor: pointer; }
  .webgl :global(.grabbable) { cursor: grab; }
  .webgl :global(.grabbable:active) { cursor: grabbing; }
  .labels { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
  .labels span { position: absolute; transform: translate(-50%, 18px); max-width: 210px; padding: 3px 7px; border-radius: 5px; color: var(--muted); background: var(--surface); font-size: .68rem; white-space: nowrap; text-overflow: ellipsis; overflow: hidden; }
  .labels .active { color: var(--ink); border: 1px solid var(--line); font-weight: 600; }
  .fallback { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; align-items: center; padding: 36px; text-align: center; background: var(--surface); }
  .fallback p { color: var(--muted); max-width: 420px; line-height: 1.7; }
  .fallback select { max-width: 100%; width: 360px; }
  @media (max-height: 560px) { .scene { min-height: 320px; } }
  .initializing { position: absolute; inset: 40% 10% auto; text-align: center; color: var(--muted); }
</style>
