<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { moveParticle, startSimulation, zoomAt, type Particle } from '../graph-physics';
  import { knowledgeLayout, knowledgeEdge } from '../knowledge-graph';
  import { INITIAL_CAMERA, cameraViewBox, zoomCamera, relationshipLabel, type Camera } from '../graph';
  import { kindLabels, statusLabel, date, type GraphData } from '../types';
  import Icon from './Icon.svelte';
  let { data, projects, loading, projectLoading, onsearch, onopen }: {
    data: GraphData; projects: string[]; loading: boolean; projectLoading: boolean;
    onsearch: (query: string, project: string, archived: boolean) => void; onopen: (id: string) => void;
  } = $props();
  const uid = $props.id();
  let query = $state(''), project = $state(''), archived = $state(false);
  let kind = $state(''), selectedID = $state(''), focusOnly = $state(false), labels = $state(false);
  let relation = $state('all');
  let settings = $state(false), hovered = $state(''), colored = $state(false), paused = $state(false);
  let repulsion = $state(1800), linkDistance = $state(110);
  let particles = $state<Particle[]>([]);
  let simulationVersion = $state(0);
  let reduceMotion = $state(true), mounted = $state(false), nodeFocused = $state(false);
  let nodeDrag: { id: string; pointer: number; x: number; y: number; moved: boolean } | null = null;
  let suppressClick = '';
  let capture: Element | null = null;
  let camera = $state<Camera>({ ...INITIAL_CAMERA });
  let svg = $state<SVGSVGElement>();
  let settingsButton = $state<HTMLButtonElement>(), inspector = $state<HTMLElement>();
  let drag = $state<{ id: number; x: number; y: number; camera: Camera; scale: number } | null>(null);
  const filtered = $derived({ ...data, nodes: data.nodes.filter(n => !kind || n.kind === kind), edges: data.edges.filter(e => relation === 'all' || e.type === relation) });
  const seed = $derived(knowledgeLayout(filtered));
  const layout = $derived({ ...seed, nodes: particles });
  const byID = $derived(new Map(layout.nodes.map(n => [n.id, n])));
  const selected = $derived(byID.get(selectedID));
  const inspectorOpen = $derived(settings || !!selected);
  const activeID = $derived(hovered || selectedID);
  const neighbors = $derived(new Set(layout.edges.filter(e => e.from === activeID || e.to === activeID).flatMap(e => [e.from, e.to])));
  const focusIDs = $derived(new Set(layout.edges.filter(e => e.from === selectedID || e.to === selectedID).flatMap(e => [e.from, e.to])));
  const visibleNodes = $derived(layout.nodes.filter(n => !focusOnly || !selected || n.id === selectedID || focusIDs.has(n.id)));
  const visibleIDs = $derived(new Set(visibleNodes.map(n => n.id)));
  const visibleEdges = $derived(layout.edges.filter(e => visibleIDs.has(e.from) && visibleIDs.has(e.to)));
  const connections = $derived(layout.edges.filter(e => e.from === selectedID || e.to === selectedID));
  const viewBox = $derived(cameraViewBox({ width: 1200, height: 800 }, camera));
  $effect(() => {
    void data;
    untrack(() => {
      if (inspector?.contains(document.activeElement)) settingsButton?.focus();
      clearSelection(); camera = { ...INITIAL_CAMERA };
    });
  });
  $effect(() => {
    const initial = seed;
    untrack(() => {
      cancelGesture();
      hovered = '';
      particles = initial.nodes.map(n => ({ ...n, radius: 3 + Math.min(5, Math.sqrt(n.degree)), vx: 0, vy: 0 }));
      const focusedID = svg?.contains(document.activeElement) ? document.activeElement?.getAttribute('data-node-id') : null;
      nodeFocused = initial.nodes.some(n => n.id === focusedID);
      restart();
    });
  });
  onMount(() => {
    const media = matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => { reduceMotion = media.matches; };
    update(); mounted = true;
    media.addEventListener('change', update);
    return () => { media.removeEventListener('change', update); cancelGesture(); };
  });
  // One owner for the animation frame. Particle mutations are intentionally not
  // effect dependencies; otherwise every physics step would reset the tick budget.
  $effect(() => {
    const element = svg, edges = seed.edges, strength = repulsion, distance = linkDistance;
    const stopped = !mounted || !element || loading || paused || reduceMotion || nodeFocused;
    void simulationVersion;
    if (stopped) return;
    const nodes = untrack(() => particles);
    return startSimulation(nodes, edges, strength, distance, () => nodeDrag?.id || '');
  });
  // Filters can close the inspector while a control inside it has focus.
  $effect.pre(() => {
    if (!inspectorOpen && untrack(() => inspector?.contains(document.activeElement))) {
      void tick().then(() => settingsButton?.focus());
    }
  });
  function restart() { simulationVersion++; }
  function world(x: number, y: number) {
    const matrix = svg?.getScreenCTM();
    return matrix ? new DOMPoint(x, y).matrixTransform(matrix.inverse()) : null;
  }
  function wheel(e: WheelEvent) {
    e.preventDefault(); const point = world(e.clientX, e.clientY);
    if (point) camera = zoomAt(camera, point, Math.exp(-Math.max(-100, Math.min(100, e.deltaY)) * .003));
  }
  $effect(() => {
    const element = svg;
    if (!element || loading) untrack(cancelGesture);
    element?.addEventListener('wheel', wheel, { passive: false });
    return () => element?.removeEventListener('wheel', wheel);
  });
  function dragNode(e: PointerEvent, id: string) {
    if (e.button !== 0 || nodeDrag || drag) return;
    e.stopPropagation(); suppressClick = '';
    nodeDrag = { id, pointer: e.pointerId, x: e.clientX, y: e.clientY, moved: false };
    // Capture on the node, not the SVG: a stationary tap must still click
    // the node after capture retargeting. Move/up events bubble to the SVG.
    capture = e.currentTarget as Element;
    capture.setPointerCapture(e.pointerId); restart();
  }
  function search() { onsearch(query.trim(), project, archived); }
  function pan(x: number, y: number) { camera = { ...camera, x: camera.x + x / camera.zoom, y: camera.y + y / camera.zoom }; }
  function select(id: string) { suppressClick = ''; selectedID = id; }
  function clickNode(e: MouseEvent, id: string) {
    const suppressed = suppressClick === id && e.detail > 0;
    suppressClick = '';
    if (!suppressed) select(id);
  }
  function clearSelection() {
    if (inspector?.contains(document.activeElement)) {
      const node = Array.from(svg?.querySelectorAll<SVGGElement>('[data-node-id]') || []).find(n => n.dataset.nodeId === selectedID);
      (node || settingsButton)?.focus();
    }
    selectedID = ''; focusOnly = false;
  }
  function focusNode(e: FocusEvent, node: Particle) {
    nodeFocused = true;
    if ((e.currentTarget as Element).matches(':focus-visible')) camera = { ...camera, x: node.x - 600, y: node.y - 400 };
  }
  function center() { if (selected) camera = { x: selected.x - 600, y: selected.y - 400, zoom: 2 }; }
  function keyboard(e: KeyboardEvent) {
    if (e.target !== e.currentTarget) return;
    const actions: Record<string, () => void> = { ArrowLeft: () => pan(-80, 0), ArrowRight: () => pan(80, 0), ArrowUp: () => pan(0, -80), ArrowDown: () => pan(0, 80), '+': () => camera = zoomCamera(camera, 1.25), '=': () => camera = zoomCamera(camera, 1.25), '-': () => camera = zoomCamera(camera, .8), '0': () => camera = { ...INITIAL_CAMERA }, Escape: clearSelection };
    if (actions[e.key]) { e.preventDefault(); actions[e.key](); }
  }
  function start(e: PointerEvent) {
    if (e.button !== 0 || nodeDrag || drag || (e.target as Element).closest('[role="button"]')) return;
    suppressClick = '';
    const matrix = svg?.getScreenCTM(); if (!matrix || !svg) return;
    drag = { id: e.pointerId, x: e.clientX, y: e.clientY, camera: { ...camera }, scale: 1 / matrix.a };
    capture = svg;
    capture.setPointerCapture(e.pointerId);
  }
  function move(e: PointerEvent) {
    if (nodeDrag?.pointer === e.pointerId) {
      const point = world(e.clientX, e.clientY), node = byID.get(nodeDrag.id);
      if (Math.hypot(e.clientX - nodeDrag.x, e.clientY - nodeDrag.y) > 4) nodeDrag.moved = true;
      if (node && point && nodeDrag.moved) moveParticle(node, point);
      return;
    }
    if (drag?.id !== e.pointerId) return;
    camera = { ...drag.camera, x: drag.camera.x - (e.clientX - drag.x) * drag.scale, y: drag.camera.y - (e.clientY - drag.y) * drag.scale };
  }
  function stop(e: PointerEvent) {
    if (nodeDrag?.pointer !== e.pointerId && drag?.id !== e.pointerId) return;
    // A normal pointerup is followed by lostpointercapture, then click. Do not
    // clear suppression in that second event; cancelled gestures produce no click.
    suppressClick = e.type === 'pointerup' && nodeDrag?.moved ? nodeDrag.id : '';
    nodeDrag = null; drag = null;
    const target = capture; capture = null;
    if (target?.hasPointerCapture(e.pointerId)) target.releasePointerCapture(e.pointerId);
    restart();
  }
  function cancelGesture() {
    const pointer = nodeDrag?.pointer ?? drag?.id;
    nodeDrag = null; drag = null; suppressClick = ''; nodeFocused = false;
    const target = capture; capture = null;
    if (pointer !== undefined && target?.hasPointerCapture(pointer)) target.releasePointerCapture(pointer);
  }
</script>

<section class="knowledge" class:colored aria-labelledby="kg-heading">
  <header class="kg-heading"><div><h2 id="kg-heading">Graph view</h2></div><div class="totals"><span>{data.nodes.length} catatan · {data.edges.length} relasi</span></div></header>
  <form class="kg-toolbar" role="search" aria-label="Filter knowledge graph" onsubmit={e => { e.preventDefault(); search(); }}>
    <label class="query"><span class="sr-only">Cari pengetahuan</span><input type="search" bind:value={query} maxlength="2000" placeholder="Cari pengetahuan, keputusan, atau ide…"></label>
    <label><span class="sr-only">Proyek</span><select bind:value={project} onchange={search} disabled={projectLoading}><option value="">Semua proyek</option>{#each projects as p}<option value={p}>{p}</option>{/each}</select></label>
    <button type="submit" class="primary" disabled={loading}>Telusuri</button>
    <label class="archive"><input type="checkbox" bind:checked={archived} onchange={search}> Arsip</label>
    <button bind:this={settingsButton} type="button" class="secondary" aria-expanded={settings} aria-controls={`${uid}-inspector`} onclick={() => settings = !settings}>Pengaturan</button>
  </form>
  <div class="kg-workspace" aria-busy={loading}>
    <div class="canvas-shell">
      <div class="canvas-top"><span class="canvas-title"><Icon name="graph" /> {visibleNodes.length} node</span><button type="button" class="secondary" aria-pressed={paused || reduceMotion} disabled={reduceMotion} onclick={() => paused = !paused}>{reduceMotion ? 'Gerakan dikurangi' : paused ? 'Lanjutkan simulasi' : 'Jeda simulasi'}</button></div>
      {#if loading}<div class="canvas-empty" role="status"><Icon name="graph" /><h3>Memetakan pengetahuan…</h3></div>
      {:else if !visibleNodes.length}<div class="canvas-empty" role="status"><Icon name="graph" /><h3>Belum ada node yang cocok</h3><p>Ubah filter atau tambahkan memori dan relasi eksplisit.</p></div>
      {:else}
        <!-- Named, focusable region provides optional map shortcuts; all operations also have buttons. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div class="map" role="region" aria-label="Peta knowledge graph interaktif" aria-describedby={`${uid}-help`} tabindex="0" onkeydown={keyboard}>
          <svg bind:this={svg!} {viewBox} class:dragging={!!drag} role="group" aria-label="Entitas dan relasi eksplisit" onpointerdown={start} onpointermove={move} onpointerup={stop} onpointercancel={stop} onlostpointercapture={stop}>
            <defs><marker id={`${uid}-arrow`} markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto" markerUnits="userSpaceOnUse"><path d="M0 0 7 3.5 0 7Z" class="arrow" /></marker></defs>
            <g aria-hidden="true">{#each visibleEdges as edge}<path class="edge" class:replacement={edge.type === 'supersedes'} class:highlight={!!activeID && (edge.from === activeID || edge.to === activeID)} class:dimmed={!!activeID && edge.from !== activeID && edge.to !== activeID} d={edge.from === edge.to ? knowledgeEdge(byID.get(edge.from)!, byID.get(edge.to)!, edge.type) : `M ${byID.get(edge.from)!.x} ${byID.get(edge.from)!.y} L ${byID.get(edge.to)!.x} ${byID.get(edge.to)!.y}`} />{/each}</g>
            {#each visibleNodes as node (node.id)}
              <g class={`node kind-${node.kind}`} class:chosen={node.id === activeID} class:dimmed={!!activeID && node.id !== activeID && !neighbors.has(node.id)} class:archived={node.archived} role="button" tabindex="0" aria-pressed={node.id === selectedID} aria-label={`${node.title} · ${kindLabels[node.kind] || node.kind} · ${node.project}`} onpointerenter={() => hovered = node.id} onpointerleave={() => hovered = ''} onpointerdown={e => dragNode(e, node.id)} ondblclick={() => onopen(node.id)} onclick={e => clickNode(e, node.id)} onkeydown={e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); select(node.id); } else if (e.key === 'Escape') { e.preventDefault(); clearSelection(); } }} onfocus={e => focusNode(e, node)} onblur={() => nodeFocused = false} data-node-id={node.id}>
                <title>{node.title} · {node.project} · {node.degree} relasi</title>
                <circle class="hit" cx={node.x} cy={node.y} r={Math.max(24, node.radius + 10)} />
                <circle class="halo" cx={node.x} cy={node.y} r={node.radius + 8} />
                <circle class="core" cx={node.x} cy={node.y} r={node.radius} />
                {#if labels || camera.zoom >= 1.6 || visibleNodes.length <= 15 || node.id === activeID || (!!activeID && neighbors.has(node.id))}<text class="node-label" x={node.x} y={node.y + node.radius + 15 / camera.zoom} text-anchor="middle" style:font-size={`${11 / Math.sqrt(camera.zoom)}px`}>{node.title.length > 42 ? `${node.title.slice(0, 41)}…` : node.title}</text>{/if}
              </g>
            {/each}
          </svg>
        </div>
      {/if}
      <div class="canvas-bottom"><div class="legend">{#each Object.entries(kindLabels) as [value, label]}<span class={`kind-${value}`}><i></i>{label}</span>{/each}</div><div class="zoom-controls"><button type="button" class="icon-button" aria-label="Perkecil" onclick={() => camera = zoomCamera(camera, .8)}><Icon name="zoom-out" /></button><span>{Math.round(camera.zoom * 100)}%</span><button type="button" class="icon-button" aria-label="Perbesar" onclick={() => camera = zoomCamera(camera, 1.25)}><Icon name="zoom-in" /></button><button type="button" class="icon-button" aria-label="Atur ulang tampilan" onclick={() => camera = { ...INITIAL_CAMERA }}><Icon name="refresh" /></button></div></div>
    </div>
    <aside bind:this={inspector} id={`${uid}-inspector`} class="inspector" class:closed={!settings && !selected} aria-label="Penjelajah entitas">
      <div class="inspector-title"><span class="eyebrow">GRAPH EXPLORER</span><Icon name="graph" /></div>
      <div class="inspector-filters"><label>Jenis entitas<select bind:value={kind}><option value="">Semua jenis</option>{#each Object.entries(kindLabels) as [v, label]}<option value={v}>{label}</option>{/each}</select></label><label>Relasi<select bind:value={relation}><option value="all">Semua relasi</option><option value="related_to">Terkait ke</option><option value="supersedes">Menggantikan</option></select></label></div>
      {#if settings}<div class="physics-settings"><label>Gaya tolak <input type="range" min="300" max="5000" step="100" bind:value={repulsion}></label><label>Jarak hubungan <input type="range" min="40" max="220" step="5" bind:value={linkDistance}></label><label class="check"><input type="checkbox" bind:checked={colored}> Warna berdasarkan jenis</label><p class="hint">Garis menunjukkan relasi; arah dan jenis tersedia pada detail node dan daftar teks.</p></div>{/if}
      {#if selected}
        <div class="selected-heading"><span class={`type-badge kind-${selected.kind}`}>{kindLabels[selected.kind] || selected.kind}</span><button class="icon-button" type="button" aria-label="Batalkan pilihan node" onclick={clearSelection}><Icon name="close" /></button></div>
        <h3>{selected.title}</h3><p class="muted">{selected.project}</p>
        <dl><div><dt>Status</dt><dd>{statusLabel(selected.status)}</dd></div><div><dt>Revisi</dt><dd>{selected.revision}</dd></div><div><dt>Diperbarui</dt><dd>{date(selected.updated_at)}</dd></div></dl>
        {#if selected.archived}<p class="hint">Entitas diarsipkan</p>{/if}{#if selected.is_project_summary}<p class="hint">★ Ringkasan proyek</p>{/if}
        <div class="selection-actions"><button type="button" class="primary" onclick={() => onopen(selected.id)}>Buka memori <Icon name="chevron" /></button><button type="button" class="secondary" onclick={center}>Pusatkan node</button></div>
        <label class="check"><input type="checkbox" bind:checked={focusOnly}> Fokus lingkungan satu-hop</label>
        <h4>Relasi langsung <span class="badge">{connections.length}</span></h4>
        <div class="connections">{#each connections as edge}{@const outgoing = edge.from === selectedID}{@const other = byID.get(outgoing ? edge.to : edge.from)!}<button type="button" class="connection" onclick={() => select(other.id)}><span class="connection-type">{outgoing ? 'Keluar →' : '← Masuk'} · {relationshipLabel(edge.type)}</span><strong>{other.title}</strong><span>{other.project}</span></button>{:else}<p class="hint">Tidak ada relasi eksplisit pada hasil yang ditampilkan.</p>{/each}</div>
      {:else}<div class="inspector-empty"><span class="orb"><Icon name="graph" /></span><h3>Setiap ide punya koneksi.</h3><p>Pilih sebuah node untuk menjelajahi konteks dan relasinya, tanpa meninggalkan peta.</p><p class="hint">Lingkaran: entitas · Kotak: keputusan<br>★ ringkasan · Garis putus-putus: menggantikan</p></div>{/if}
    </aside>
  </div>
  <div class="map-meta"><p id={`${uid}-help`}>Tarik node atau latar · Scroll untuk zoom · Klik dua kali untuk membuka. Keyboard pada peta: panah geser, +/− zoom, 0 reset. Pada node: Enter/Spasi pilih, Escape batalkan. Simulasi berhenti selama node mendapat fokus dan saat preferensi gerakan dikurangi aktif.</p><div class="pan"><button type="button" class="secondary" aria-label="Geser kiri" onclick={() => pan(-80, 0)}>←</button><button type="button" class="secondary" aria-label="Geser atas" onclick={() => pan(0, -80)}>↑</button><button type="button" class="secondary" aria-label="Geser bawah" onclick={() => pan(0, 80)}>↓</button><button type="button" class="secondary" aria-label="Geser kanan" onclick={() => pan(80, 0)}>→</button><label class="check"><input type="checkbox" bind:checked={labels}> Semua label</label></div></div>
  {#if layout.limited}<p class="notice" role="status">Peta menampilkan maksimal 100 entitas, memprioritaskan ringkasan dan node paling terhubung. Hasil server juga dapat dibatasi. Persempit filter untuk melihat bagian lain; relasi tidak pernah disimpulkan.</p>{/if}
  <details class="text-view"><summary>Jelajahi sebagai daftar teks ({filtered.nodes.length} entitas)</summary><div class="entity-list">{#each filtered.nodes as node (node.id)}<article><button type="button" class="note-link" onclick={() => onopen(node.id)}>{node.title}</button><p class="hint">{node.project} · {kindLabels[node.kind] || node.kind}</p><ul>{#each filtered.edges.filter(e => e.from === node.id && filtered.nodes.some(n => n.id === e.to)) as edge}<li>{relationshipLabel(edge.type)} → <button type="button" class="note-link" onclick={() => onopen(edge.to)}>{filtered.nodes.find(n => n.id === edge.to)?.title}</button></li>{/each}</ul></article>{/each}</div></details>
</section>

<style>
  .knowledge { --kg-knowledge: #7c6ee6; --kg-fact: #248b97; --kg-decision: #bd7419; --kg-handoff: #ba568a; --kg-skill: #4e8c52; }
  .kind-knowledge { --node-color: var(--kg-knowledge); }.kind-fact { --node-color: var(--kg-fact); }.kind-decision { --node-color: var(--kg-decision); }.kind-handoff { --node-color: var(--kg-handoff); }.kind-skill { --node-color: var(--kg-skill); }
  .kg-heading { display: flex; justify-content: space-between; gap: 24px; align-items: center; margin-bottom: 24px; }.kg-heading h2 { font-size: clamp(1.7rem, 3vw, 2.4rem); letter-spacing: -.055em; margin: 5px 0 8px; }.eyebrow { font-size: .65rem; letter-spacing: .15em; }
  .totals { display: flex; gap: 24px; color: var(--muted); font-size: .75rem; }.totals span { display: grid; gap: 5px; }
  .kg-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-bottom: 16px; }.query { flex: 1; min-width: 180px; }.query input { width: 100%; }.archive, .check { display: flex; align-items: center; gap: 8px; font-size: .8rem; }.archive { margin-left: 8px; }
  .kg-workspace { display: grid; grid-template-columns: minmax(0, 1fr) 290px; border: 1px solid var(--line); border-radius: 20px; overflow: hidden; background: var(--glass); box-shadow: var(--shadow); }
  .canvas-shell { min-width: 0; display: flex; flex-direction: column; background-color: var(--surface); background-image: radial-gradient(var(--input-line) .8px, transparent .8px); background-size: 22px 22px; }.canvas-top { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 18px 20px; font-size: .7rem; color: var(--muted); }.canvas-title { display: flex; gap: 8px; align-items: center; font-size: .78rem; color: var(--ink); }.canvas-title :global(.icon) { width: 17px; }
  .map { min-width: 0; flex: 1; }.map:focus-visible { outline: 2px solid var(--ring); outline-offset: -3px; }svg { width: 100%; height: clamp(430px, 65vh, 760px); display: block; cursor: grab; touch-action: none; user-select: none; }svg.dragging { cursor: grabbing; }
  .edge { fill: none; stroke: var(--muted); stroke-opacity: .35; stroke-width: 1.2; }.edge.replacement { stroke-dasharray: 5 5; }.edge.highlight { stroke: var(--accent); stroke-width: 2; stroke-opacity: .85; }.arrow { fill: var(--muted); }.dimmed { opacity: .16; }.node { cursor: pointer; outline: none; }.hit { fill: transparent; }.halo { fill: var(--node-color, var(--accent)); opacity: .09; }.core { fill: var(--node-color, var(--accent)); stroke: var(--surface); stroke-width: 3; }.node.archived .core { stroke: var(--muted); stroke-dasharray: 3 3; }.node.chosen .halo, .node:hover .halo, .node:focus-visible .halo { opacity: .27; }.node.chosen .core, .node:focus-visible .core { stroke: var(--ink); stroke-width: 2.5; }.node-label { fill: var(--ink); stroke: var(--surface); stroke-width: 4px; paint-order: stroke; font-size: 12px; font-weight: 550; pointer-events: none; }.summary-star { fill: #fff; font-size: 15px; pointer-events: none; }
  .canvas-bottom { padding: 14px 18px; display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; border-top: 1px solid var(--line); background: var(--glass); }.legend { display: flex; flex-wrap: wrap; gap: 12px; font-size: .65rem; color: var(--muted); }.legend span { display: flex; align-items: center; gap: 5px; }.legend i { width: 7px; height: 7px; border-radius: 50%; background: var(--node-color); }.zoom-controls { display: flex; gap: 2px; align-items: center; }.zoom-controls span { font-size: .7rem; min-width: 38px; text-align: center; }.zoom-controls button { background: transparent; }
  .inspector { border-left: 1px solid var(--line); padding: 22px; min-width: 0; max-height: 860px; overflow-y: auto; background: var(--glass-soft); }.inspector-title { display: flex; justify-content: space-between; color: var(--muted); align-items: center; margin-bottom: 22px; }.inspector-filters { display: grid; gap: 12px; padding-bottom: 20px; border-bottom: 1px solid var(--line); }.inspector-filters label { display: grid; gap: 6px; font-size: .7rem; color: var(--muted); }.inspector-filters select { width: 100%; }.inspector h3 { line-height: 1.4; font-size: 1.15rem; overflow-wrap: anywhere; }.selected-heading { display: flex; justify-content: space-between; align-items: center; margin-top: 20px; }.type-badge { color: var(--node-color); font-size: .7rem; font-weight: 700; }dl { font-size: .75rem; }dl div { display: flex; justify-content: space-between; gap: 8px; padding: 8px 0; }dt { color: var(--muted); }dd { margin: 0; text-align: right; }.selection-actions { display: grid; gap: 8px; margin: 18px 0; }.selection-actions :global(.icon) { transform: rotate(180deg); }.inspector h4 { font-size: .8rem; margin: 24px 0 12px; }.connections { display: grid; gap: 8px; }.connection { display: grid; text-align: left; gap: 6px; padding: 12px; border: 1px solid var(--line); background: var(--surface); color: var(--ink); font-size: .75rem; width: 100%; overflow-wrap: anywhere; }.connection-type, .connection > span:last-child { font-size: .65rem; color: var(--muted); }.connection:hover { border-color: var(--ring); }
  .inspector-empty { padding: 36px 0; }.inspector-empty p { font-size: .8rem; line-height: 1.8; color: var(--muted); }.orb { width: 52px; height: 52px; display: grid; place-items: center; border-radius: 18px; background: var(--pale); color: var(--accent); }.canvas-empty { min-height: 480px; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 24px; text-align: center; color: var(--muted); }.canvas-empty :global(.icon) { width: 40px; height: 40px; }.map-meta { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 12px 0; }.map-meta p { font-size: .7rem; color: var(--muted); }.pan { display: flex; align-items: center; gap: 5px; flex-wrap: wrap; }.pan button { min-width: 44px; }.notice { padding: 12px; background: var(--pale); font-size: .8rem; border-radius: 10px; }.text-view { padding: 16px; border: 1px solid var(--line); border-radius: 12px; margin-top: 10px; font-size: .8rem; }.entity-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; padding-top: 16px; }.entity-list article { min-width: 0; overflow-wrap: anywhere; }.entity-list ul { padding-left: 18px; }
  @media (max-width: 1100px) { .kg-workspace { grid-template-columns: minmax(0, 1fr) 250px; }.inspector { padding: 16px; }.totals { gap: 14px; }.map-meta { flex-wrap: wrap; } }
  @media (max-width: 800px) { .kg-workspace { grid-template-columns: 1fr; }.inspector { border-left: 0; border-top: 1px solid var(--line); max-height: none; }.inspector-filters { grid-template-columns: 1fr 1fr; }.inspector-empty { padding: 12px 0; }.kg-heading { align-items: flex-start; flex-direction: column; gap: 16px; }.totals { gap: 28px; }svg { height: 460px; }.canvas-top { padding: 14px; }.kg-toolbar > label:not(.query) { flex: 1; }.kg-toolbar select { width: 100%; } }
  .kg-heading { margin-bottom: 12px; }.kg-heading h2 { font-size: 1.1rem; letter-spacing: -.02em; margin: 0; }.totals { font-size: .75rem; }
  .kg-workspace { position: relative; display: block; border-radius: 10px; box-shadow: none; }.canvas-shell { background-image: none; background-color: var(--surface); }.map svg { height: max(560px, calc(100dvh - 270px)); }
  .canvas-top { position: absolute; top: 0; left: 0; right: 0; z-index: 1; pointer-events: none; padding: 12px 16px; }.canvas-top button { pointer-events: auto; font-size: .7rem; background: var(--surface); }
  .inspector { position: absolute; top: 60px; right: 14px; width: 270px; max-height: calc(100% - 140px); border: 1px solid var(--line); border-radius: 10px; background: var(--surface); box-shadow: var(--shadow); padding: 18px; }.inspector.closed { display: none; }.inspector-title { margin-bottom: 14px; }.inspector-empty { display: none; }.physics-settings { display: grid; gap: 15px; padding-top: 18px; }.physics-settings > label:not(.check) { display: grid; gap: 8px; font-size: .75rem; }.physics-settings input[type="range"] { width: 100%; }
  .node .core { fill: var(--muted); stroke: none; }.colored .node .core { fill: var(--node-color); }.node .halo { opacity: 0; }.node.chosen .core, .node:hover .core { fill: var(--accent); stroke: none; }.node.chosen .halo { fill: var(--accent); opacity: .08; }.node:focus-visible .core { stroke: var(--ink); stroke-width: 2; }.edge { stroke-width: .7; stroke-opacity: .28; }.edge.highlight { stroke-width: 1.2; stroke-opacity: .8; }.dimmed { opacity: .1; }.node-label { font-weight: 400; fill: var(--muted); stroke-width: 3px; }.node.chosen .node-label { fill: var(--ink); }.canvas-bottom { border-top: none; background: transparent; padding: 10px 16px; }.legend { display: none; }.colored .legend { display: flex; }.zoom-controls { margin-left: auto; }.map-meta { padding-top: 4px; }
  @media (max-width: 800px) { .map svg { height: 65dvh; min-height: 440px; }.inspector { width: min(270px, calc(100% - 28px)); max-height: 55%; top: 60px; }.kg-heading { flex-direction: row; align-items: center; }.inspector-filters { grid-template-columns: 1fr; } }
</style>
