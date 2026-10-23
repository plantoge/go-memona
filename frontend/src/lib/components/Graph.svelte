<script lang="ts">
  import { NODE_WIDTH, NODE_HEIGHT, INITIAL_CAMERA, layoutGraph, edgePath, zoomCamera, cameraViewBox, relationshipLabel, type Camera } from '../graph';
  import { date, kindLabels, statusLabel, type GraphData } from '../types';
  import Icon from './Icon.svelte';

  let { data, projects, loading, projectLoading, onsearch, onopen }: {
    data: GraphData; projects: string[]; loading: boolean; projectLoading: boolean;
    onsearch: (query: string, project: string, archived: boolean) => void;
    onopen: (id: string) => void;
  } = $props();
  let query = $state('');
  let project = $state('');
  let archived = $state(false);
  let camera = $state<Camera>({ ...INITIAL_CAMERA });
  let svg = $state<SVGSVGElement>();
  let drag = $state<{ pointer: number; x: number; y: number; camera: Camera } | null>(null);
  const layout = $derived(layoutGraph(data));
  const nodesByID = $derived(new Map(layout.nodes.map(node => [node.id, node])));
  const connected = $derived(new Set(layout.edges.flatMap(edge => [edge.from, edge.to])));
  const viewBox = $derived(cameraViewBox(layout, camera));
  $effect(() => { void data; camera = { ...INITIAL_CAMERA }; drag = null; });

  function search() { onsearch(query.trim(), project, archived); }
  function pan(x: number, y: number) {
    camera = { ...camera, x: camera.x + x * layout.width / camera.zoom, y: camera.y + y * layout.height / camera.zoom };
  }
  function reset() { camera = { ...INITIAL_CAMERA }; }
  function revealNode(x: number, y: number) {
    const width = layout.width / camera.zoom, height = layout.height / camera.zoom;
    const left = (layout.width - width) / 2 + camera.x, top = (layout.height - height) / 2 + camera.y;
    if (x < left || y < top || x + NODE_WIDTH > left + width || y + NODE_HEIGHT > top + height) {
      camera = { ...camera, x: x + NODE_WIDTH / 2 - layout.width / 2, y: y + NODE_HEIGHT / 2 - layout.height / 2 };
    }
  }
  function keyboard(event: KeyboardEvent) {
    if (event.target !== event.currentTarget) return;
    const actions: Record<string, () => void> = {
      ArrowLeft: () => pan(-0.12, 0), ArrowRight: () => pan(0.12, 0),
      ArrowUp: () => pan(0, -0.12), ArrowDown: () => pan(0, 0.12),
      '+': () => camera = zoomCamera(camera, 1.3), '=': () => camera = zoomCamera(camera, 1.3),
      '-': () => camera = zoomCamera(camera, 1 / 1.3), '0': reset, Home: reset,
    };
    if (actions[event.key]) { event.preventDefault(); actions[event.key](); }
  }
  function startDrag(event: PointerEvent) {
    if (event.button !== 0 || (event.target as Element).closest('[role="button"]')) return;
    const point = new DOMPoint(event.clientX, event.clientY).matrixTransform(svg?.getScreenCTM()?.inverse());
    drag = { pointer: event.pointerId, x: point.x, y: point.y, camera: { ...camera } };
    svg?.setPointerCapture(event.pointerId);
  }
  function moveDrag(event: PointerEvent) {
    if (!drag || drag.pointer !== event.pointerId) return;
    const point = new DOMPoint(event.clientX, event.clientY).matrixTransform(svg?.getScreenCTM()?.inverse());
    camera = { ...drag.camera, x: camera.x + drag.x - point.x, y: camera.y + drag.y - point.y };
  }
  function stopDrag(event: PointerEvent) {
    if (drag?.pointer !== event.pointerId) return;
    if (svg?.hasPointerCapture(event.pointerId)) svg.releasePointerCapture(event.pointerId);
    drag = null;
  }
</script>

<section aria-labelledby="graph-heading">
  <div class="section-heading"><h2 id="graph-heading">Graf memori</h2><span class="muted">Relasi eksplisit · arah sumber → tujuan</span></div>
  <form class="filters" role="search" aria-label="Filter graf" onsubmit={event => { event.preventDefault(); search(); }}>
    <div class="search-field"><label for="graph-query">Cari memori</label><input id="graph-query" type="search" bind:value={query} placeholder="Cari judul atau isi memori…" autocomplete="off"></div>
    <div><label for="graph-project">Proyek</label><select id="graph-project" bind:value={project} onchange={search} aria-busy={projectLoading}><option value="">Semua proyek</option>{#each projects as item}<option value={item}>{item}</option>{/each}</select></div>
    <label class="check-label"><input type="checkbox" bind:checked={archived} onchange={search}> Sertakan arsip</label>
    <button type="submit" class="primary">Cari</button>
  </form>
  <div aria-busy={loading}>
    {#if loading}<p class="empty-state" role="status">Memuat graf memori…</p>
    {:else if !data.nodes.length}<p class="empty-state" role="status">Tidak ada memori yang cocok. Coba kata kunci, proyek, atau sertakan arsip.</p>
    {:else}
      <p class="graph-summary" role="status">{layout.nodes.length} memori · {layout.edges.length} relasi · {layout.nodes.filter(node => !connected.has(node.id)).length} tanpa relasi dalam hasil ini</p>
      {#if data.truncated}<p class="graph-notice" role="status">Hasil graf dibatasi. Tidak semua memori atau relasi ditampilkan; persempit proyek atau pencarian (maks. 200 memori per permintaan).</p>{/if}
      <div class="graph-panel">
        <div class="graph-controls" role="group" aria-label="Kontrol tampilan graf">
          <button type="button" class="secondary" aria-label="Perbesar graf" disabled={camera.zoom >= 8} onclick={() => camera = zoomCamera(camera, 1.3)}><Icon name="zoom-in" /></button>
          <button type="button" class="secondary" aria-label="Perkecil graf" disabled={camera.zoom <= 0.5} onclick={() => camera = zoomCamera(camera, 1 / 1.3)}><Icon name="zoom-out" /></button>
          <button type="button" class="secondary" onclick={reset}><Icon name="refresh" /> Atur ulang</button>
          <span class="zoom-value">{Math.round(camera.zoom * 100)}%</span>
          <div class="pan-controls" role="group" aria-label="Geser tampilan">
            <button type="button" class="secondary" aria-label="Geser tampilan ke kiri" onclick={() => pan(-0.12, 0)}>←</button>
            <button type="button" class="secondary" aria-label="Geser tampilan ke atas" onclick={() => pan(0, -0.12)}>↑</button>
            <button type="button" class="secondary" aria-label="Geser tampilan ke bawah" onclick={() => pan(0, 0.12)}>↓</button>
            <button type="button" class="secondary" aria-label="Geser tampilan ke kanan" onclick={() => pan(0.12, 0)}>→</button>
          </div>
        </div>
        <p id="graph-help" class="hint">Pilih memori untuk membuka detail. Geser latar atau gunakan tombol. Saat graf berfokus: panah untuk menggeser, +/− untuk zoom, 0 untuk mengatur ulang. Daftar teks tersedia di bawah.</p>
        <!-- This named region is intentionally focusable for optional pan/zoom shortcuts; native buttons above provide the same operations. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div class="graph-viewport" role="region" aria-label="Tampilan graf interaktif" aria-describedby="graph-help" tabindex="0" onkeydown={keyboard}>
          <svg bind:this={svg!} class:dragging={!!drag} {viewBox} role="group" aria-label="Memori dan relasi berarah" onpointerdown={startDrag} onpointermove={moveDrag} onpointerup={stopDrag} onpointercancel={stopDrag} onlostpointercapture={() => drag = null}>
            <defs>
              <marker id="graph-related-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="userSpaceOnUse"><path d="M0 0 8 4 0 8Z" class="arrow-related" /></marker>
              <marker id="graph-supersedes-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="userSpaceOnUse"><path d="M0 0 8 4 0 8Z" class="arrow-supersedes" /></marker>
            </defs>
            <g aria-hidden="true">
              {#each layout.edges as edge}
                <path class="graph-edge" class:supersedes={edge.type === 'supersedes'} d={edgePath(nodesByID.get(edge.from)!, nodesByID.get(edge.to)!, edge.type)} marker-end={edge.type === 'supersedes' ? 'url(#graph-supersedes-arrow)' : 'url(#graph-related-arrow)'} />
              {/each}
            </g>
            {#each layout.nodes as node (node.id)}
              <g class="graph-node" class:archived={node.archived} role="button" tabindex="0" onfocus={() => revealNode(node.x, node.y)} aria-label={`Buka ${node.title || 'Tanpa judul'} · ${node.project} · ${kindLabels[node.kind] || node.kind}${node.archived ? ' · Diarsipkan' : ''}${node.is_project_summary ? ' · Ringkasan proyek' : ''}`} onclick={() => onopen(node.id)} onkeydown={event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); onopen(node.id); } }}>
                <title>{node.title || 'Tanpa judul'} — {node.project} — {statusLabel(node.status)}</title>
                <rect x={node.x} y={node.y} width={NODE_WIDTH} height={NODE_HEIGHT} rx="10" />
                <text x={node.x + 12} y={node.y + 25} class="node-title">{(node.title || 'Tanpa judul').slice(0, 19)}{node.title.length > 19 ? '…' : ''}</text>
                <text x={node.x + 12} y={node.y + 47} class="node-meta">{node.project.slice(0, 17)}{node.project.length > 17 ? '…' : ''}</text>
                {#if node.is_project_summary}<circle cx={node.x + NODE_WIDTH - 12} cy={node.y + 12} r="4" />{/if}
              </g>
            {/each}
          </svg>
        </div>
        <p class="graph-legend"><span><i class="legend-line"></i> terkait ke</span><span><i class="legend-line supersedes"></i> menggantikan</span><span>● ringkasan proyek</span><span>bingkai putus-putus: arsip</span></p>
      </div>
      <section class="graph-text" aria-labelledby="graph-relations-heading">
        <h3 id="graph-relations-heading">Daftar relasi berarah</h3>
        <p class="hint">Hanya relasi eksplisit dengan kedua memori dalam hasil ini. Tidak ada relasi balik atau penggantian yang disimpulkan.</p>
        {#if !layout.edges.length}<p class="muted">Tidak ada relasi dalam hasil ini. Memori tanpa relasi tetap ditampilkan.</p>
        {:else}<ul class="edge-list">{#each layout.edges as edge}<li><button type="button" class="note-link" onclick={() => onopen(edge.from)}>{nodesByID.get(edge.from)!.title || 'Tanpa judul'}</button><span class="relation-label">→ {relationshipLabel(edge.type)} →</span><button type="button" class="note-link" onclick={() => onopen(edge.to)}>{nodesByID.get(edge.to)!.title || 'Tanpa judul'}</button></li>{/each}</ul>{/if}
      </section>
      <details class="graph-text">
        <summary>Semua memori dalam graf ({layout.nodes.length})</summary>
        <ul class="node-list">{#each layout.nodes as node (node.id)}<li><button type="button" class="note-link" onclick={() => onopen(node.id)}>{node.title || 'Tanpa judul'}</button><p class="hint">{node.project} · {kindLabels[node.kind] || node.kind} · {statusLabel(node.status)} · Revisi {node.revision} · {date(node.updated_at)}{node.is_project_summary ? ' · Ringkasan proyek' : ''}{node.archived ? ' · Diarsipkan' : ''}{!connected.has(node.id) ? ' · Tanpa relasi dalam hasil' : ''}</p></li>{/each}</ul>
      </details>
    {/if}
  </div>
</section>

<style>
  .graph-summary { color: var(--muted); font-size: .86rem; }
  .graph-notice { padding: 14px; background: var(--pale); border: 1px solid var(--input-line); border-radius: 10px; }
  .graph-panel, .graph-text { min-width: 0; padding: 18px; border: 1px solid var(--line); border-radius: 16px; background: var(--glass-soft); box-shadow: var(--shadow); backdrop-filter: blur(16px); }
  .graph-text { margin-top: 18px; overflow-wrap: anywhere; }
  .graph-controls, .pan-controls, .graph-legend, .graph-legend span { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .graph-controls button { min-width: 44px; }
  .pan-controls { margin-left: auto; }
  .zoom-value { min-width: 48px; color: var(--muted); font-size: .8rem; }
  .graph-viewport { overflow: hidden; border: 1px solid var(--input-line); border-radius: 10px; background: var(--surface); }
  svg { display: block; width: 100%; height: clamp(320px, 54vh, 640px); cursor: grab; touch-action: none; user-select: none; }
  svg.dragging { cursor: grabbing; }
  .graph-edge { fill: none; stroke: var(--muted); stroke-width: 1.7; }
  .graph-edge.supersedes { stroke: var(--accent); stroke-dasharray: 7 4; }
  .arrow-related { fill: var(--muted); }.arrow-supersedes { fill: var(--accent); }
  .graph-node { cursor: pointer; outline: none; }
  .graph-node rect { fill: var(--pale); stroke: var(--input-line); stroke-width: 1.5; }
  .graph-node.archived rect { stroke-dasharray: 5 3; }
  .graph-node:hover rect, .graph-node:focus-visible rect { stroke: var(--ring); stroke-width: 4; }
  .node-title { fill: var(--ink); font-size: 12px; font-weight: 650; }.node-meta { fill: var(--muted); font-size: 11px; }
  .graph-node circle { fill: var(--accent); }
  .graph-legend { margin: 14px 0 0; color: var(--muted); font-size: .76rem; gap: 10px 20px; }
  .legend-line { width: 28px; border-top: 2px solid var(--muted); }.legend-line.supersedes { border-top: 2px dashed var(--accent); }
  .edge-list, .node-list { margin: 0; padding-left: 20px; }
  .edge-list li, .node-list li { padding: 8px 0; border-bottom: 1px solid var(--line); }
  .edge-list li:last-child, .node-list li:last-child { border-bottom: 0; }
  .edge-list .note-link { display: inline; font-size: .9rem; }.relation-label { font-size: .8rem; color: var(--muted); margin: 0 10px; }
  .node-list .hint { margin: 0; }
  @media (max-width: 760px) { .graph-panel, .graph-text { padding: 12px; }.pan-controls { width: 100%; margin-left: 0; }.relation-label { display: block; margin: 4px 0; } }
  @supports not (backdrop-filter: blur(1px)) { .graph-panel, .graph-text { background: var(--surface); } }
</style>
