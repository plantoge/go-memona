<script lang="ts">
  import { tick } from 'svelte';
  import { threeGraphLayout } from '../three-graph-layout';
  import { graphQuality } from '../graph-quality';
  import { kindLabels, type GraphData } from '../types';
  import ThreeGraphCanvas from './ThreeGraphCanvas.svelte';
  import Icon from './Icon.svelte';

  let { data, projects, loading, projectLoading, suspended = false, scoped = false, onsearch, onopen }: {
    data: GraphData; projects: string[]; loading: boolean; projectLoading: boolean; suspended?: boolean; scoped?: boolean;
    onsearch: (query: string, project: string, archived: boolean) => void; onopen: (id: string) => void;
  } = $props();
  const uid = $props.id();
  let query = $state(''), project = $state(''), archived = $state(false);
  let kind = $state(''), relation = $state('all'), labels = $state(false), paused = $state(false);
  let filtersOpen = $state(false), searchOpen = $state(false);
  let canvas = $state<ThreeGraphCanvas>();
  let filterButton: HTMLButtonElement, filterArea: HTMLDivElement;
  let searchInput: HTMLInputElement, searchArea: HTMLDivElement;
  let resultsPanel = $state<HTMLDivElement>();
  let kindInput = $state<HTMLSelectElement>();

  const filtered = $derived({
    ...data,
    nodes: data.nodes.filter(n => !kind || n.kind === kind),
    edges: data.edges.filter(e => relation === 'all' || e.type === relation),
  });
  const layout = $derived(threeGraphLayout(filtered));
  const lightMode = $derived(graphQuality(layout.nodes.length, layout.edges.length).light);
  const term = $derived(query.trim().toLocaleLowerCase());
  // Search the loaded metadata, independently of the bounded visual layout.
  const matches = $derived(term ? filtered.nodes.filter(n => n.title.toLocaleLowerCase().includes(term)).slice(0, 20) : []);
  const suggestions = $derived(searchOpen && !!term);
  const filterCount = $derived(Number(!!kind) + Number(relation !== 'all') + Number(labels) + Number(archived));

  function search() {
    searchOpen = false;
    onsearch(query.trim(), scoped ? '' : project, scoped ? false : archived);
  }
  function open(id: string) {
    if (!id || loading) return;
    searchOpen = false;
    filtersOpen = false;
    onopen(id);
  }
  async function toggleFilters() {
    searchOpen = false;
    filtersOpen = !filtersOpen;
    if (filtersOpen) { await tick(); kindInput?.focus(); }
  }
  function closeFilters(restore = false) {
    filtersOpen = false;
    if (restore) filterButton?.focus();
  }
  function key(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    if (filtersOpen) { event.preventDefault(); closeFilters(true); }
    else if (suggestions) { event.preventDefault(); searchOpen = false; searchInput?.focus(); }
  }
  function outside(event: PointerEvent) {
    const target = event.target;
    if (!(target instanceof Node)) return;
    if (!filterArea?.contains(target)) filtersOpen = false;
    if (!searchArea?.contains(target)) searchOpen = false;
  }
  function leaveSearch(event: FocusEvent) {
    if (!searchArea?.contains(event.relatedTarget as Node | null)) searchOpen = false;
  }
  function leaveFilters(event: FocusEvent) {
    if (!filterArea?.contains(event.relatedTarget as Node | null)) filtersOpen = false;
  }
  function searchKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' && term) {
      event.preventDefault();
      searchOpen = true;
      void tick().then(() => resultsPanel?.querySelector<HTMLButtonElement>('button')?.focus());
    }
  }
  function resetFilters() {
    const reload = archived;
    kind = ''; relation = 'all'; labels = false; archived = false;
    if (reload) search();
  }
</script>

<svelte:window onkeydown={key} onpointerdown={outside} />

<section class="knowledge" aria-labelledby={`${uid}-heading`}>
  <header class="graph-header">
    <div class="heading">
      <h2 id={`${uid}-heading`}>Knowledge Graph</h2>
      <p class="counts" aria-live="polite"><strong>{layout.nodes.length}</strong> catatan <span aria-hidden="true">·</span> <strong>{layout.edges.length}</strong> relasi{#if lightMode}<span class="quality-badge" title="Kualitas render adaptif; seluruh node tetap ditampilkan">Ringan</span>{/if}</p>
    </div>
    <form class="toolbar" role="search" aria-label="Cari dan filter graph" onsubmit={event => { event.preventDefault(); search(); }}>
      <div class="search-area" bind:this={searchArea} onfocusout={leaveSearch}>
        <div class="search-field">
          <label class="sr-only" for={`${uid}-search`}>Cari judul catatan</label>
          <input id={`${uid}-search`} type="search" bind:this={searchInput} bind:value={query} maxlength="2000" autocomplete="off" placeholder="Cari catatan…" aria-describedby={`${uid}-search-hint`} aria-controls={suggestions ? `${uid}-results` : undefined} oninput={() => { searchOpen = true; filtersOpen = false; }} onfocus={() => { searchOpen = true; filtersOpen = false; }} onkeydown={searchKey} />
          <button type="submit" class="tool-button search-submit" aria-label="Cari di server" title="Cari di server (Enter)" disabled={loading}><Icon name="search" /></button>
        </div>
        <span class="sr-only" id={`${uid}-search-hint`}>Tekan Enter untuk mencari di server. Gunakan Tab atau panah bawah untuk membuka hasil judul catatan yang sudah dimuat.</span>
        {#if suggestions}
          <div class="search-results popover" id={`${uid}-results`} bind:this={resultsPanel} role="region" aria-label="Hasil judul catatan yang dimuat">
            <p class="popover-caption" role="status">{matches.length ? `${matches.length === 20 ? 'Maks. ' : ''}${matches.length} catatan yang dimuat` : 'Tidak ada judul yang cocok'}</p>
            <div class="result-items">
              {#each matches as node (node.id)}
                <button type="button" class="search-result" onclick={() => open(node.id)}>
                  <strong>{node.title}</strong>
                  <span>{kindLabels[node.kind] || node.kind} <span aria-hidden="true">·</span> {node.project}{node.archived ? ' · Arsip' : ''}</span>
                </button>
              {/each}
            </div>
            <p class="search-hint">Enter di kolom pencarian untuk mencari di server.</p>
          </div>
        {/if}
      </div>
      {#if !scoped}<label class="project-field">
        <span class="sr-only">Proyek</span>
        <select aria-label="Proyek" value={project} onchange={event => { project = event.currentTarget.value; search(); }} disabled={projectLoading}>
          <option value="">Semua proyek</option>
          {#each projects as p}<option value={p}>{p}</option>{/each}
        </select>
      </label>{/if}
      <div class="filter-area" bind:this={filterArea} onfocusout={leaveFilters}>
        <button type="button" class="tool-button filter-toggle" class:active={filterCount > 0} bind:this={filterButton} aria-label={`Filter tampilan${filterCount ? `, ${filterCount} aktif` : ''}`} title="Filter tampilan" aria-expanded={filtersOpen} aria-controls={`${uid}-filters`} onclick={toggleFilters}>
          <Icon name="filter" />{#if filterCount}<span class="filter-dot" aria-hidden="true"></span>{/if}
        </button>
        {#if filtersOpen}
          <div class="filter-popover popover" id={`${uid}-filters`} role="group" aria-labelledby={`${uid}-filter-heading`}>
            <div class="popover-top"><h3 id={`${uid}-filter-heading`}>Filter tampilan</h3><button type="button" class="tool-button" aria-label="Tutup filter" title="Tutup filter" onclick={() => closeFilters(true)}><Icon name="close" /></button></div>
            <label class="filter-field">Jenis catatan<select bind:this={kindInput} bind:value={kind}><option value="">Semua jenis</option>{#each Object.entries(kindLabels) as [value, label]}<option {value}>{label}</option>{/each}</select></label>
            <label class="filter-field">Hubungan<select bind:value={relation}><option value="all">Semua hubungan</option><option value="related_to">Terkait ke</option><option value="supersedes">Menggantikan</option></select></label>
            <label class="check"><input type="checkbox" bind:checked={labels}> Label terdekat (maks. 24–48)</label>
            {#if !scoped}<label class="check"><input type="checkbox" checked={archived} onchange={event => { archived = event.currentTarget.checked; search(); }}> Sertakan arsip</label>{/if}
            <button type="button" class="reset-filters" disabled={!filterCount} onclick={resetFilters}>Reset filter</button>
          </div>
        {/if}
      </div>
    </form>
  </header>

  <div class="graph-frame" aria-busy={loading}>
    <div class="canvas-wrap">
      {#if layout.nodes.length}
        <ThreeGraphCanvas bind:this={canvas} nodes={layout.nodes} edges={layout.edges} labels={labels} paused={paused || suspended || loading} onselect={open} onopen={open} />
      {:else if !loading}
        <div class="empty" role="status"><Icon name="graph" /><h3>Belum ada catatan yang cocok</h3><p>Ubah pencarian atau filter untuk melihat hubungan catatan.</p></div>
      {/if}
    </div>
    {#if loading}<div class="loading-state" role="status"><span class="loading-dot"></span>Memuat graph…</div>{/if}
    {#if layout.limited}<p class="cap-notice" role="status">Maks. 500 node · Persempit pencarian untuk hasil lainnya</p>{/if}
    <div class="legend" aria-label="Legenda hubungan"><span><i class="related"></i>Terkait</span><span><i class="supersedes"></i>Menggantikan</span></div>
    <div class="graph-controls" role="group" aria-label="Kontrol graph">
      <button type="button" class="tool-button" aria-label={paused ? 'Lanjutkan graph' : 'Jeda graph'} title={paused ? 'Lanjutkan graph' : 'Jeda graph'} aria-pressed={paused} disabled={!layout.nodes.length || loading} onclick={() => paused = !paused}><Icon name={paused ? 'play' : 'pause'} /></button>
      <span class="control-divider" aria-hidden="true"></span>
      <button type="button" class="tool-button" aria-label="Perkecil" title="Perkecil" disabled={!layout.nodes.length || loading} onclick={() => canvas?.zoom(.8)}><Icon name="zoom-out" /></button>
      <button type="button" class="tool-button" aria-label="Perbesar" title="Perbesar" disabled={!layout.nodes.length || loading} onclick={() => canvas?.zoom(1.25)}><Icon name="zoom-in" /></button>
      <button type="button" class="tool-button" aria-label="Reset tampilan graph" title="Reset tampilan graph" disabled={!layout.nodes.length || loading} onclick={() => canvas?.reset()}><Icon name="fit" /></button>
    </div>
  </div>
</section>

<style>
  .knowledge { display:flex; flex-direction:column; gap:12px; width:100%; height:100%; min-width:0; min-height:0; color:var(--ink); }
  .graph-header { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:12px 24px; flex-shrink:0; }
  .heading { display:flex; align-items:baseline; flex-wrap:wrap; gap:6px 14px; min-width:0; }
  h2 { margin:0; font-size:1.05rem; font-weight:650; line-height:1.5; letter-spacing:-.025em; }
  .counts { display:flex; align-items:center; gap:5px; margin:0; font-size:.7rem; color:var(--muted); white-space:nowrap; }
  .quality-badge { padding:3px 6px; border:1px solid var(--line); border-radius:5px; color:var(--muted); font-size:.6rem; }
  .counts strong { font-weight:600; color:var(--ink); }.counts > span { margin:0 3px; }
  .toolbar { display:flex; align-items:center; gap:8px; flex:1; max-width:600px; min-width:0; }
  .search-area { position:relative; flex:1; min-width:0; }
  .search-field { display:flex; align-items:center; border:1px solid var(--line); border-radius:10px; background:var(--surface); }
  .search-field:focus-within { outline:2px solid var(--accent); outline-offset:2px; }
  .search-field input { min-width:0; width:100%; height:44px; min-height:44px; padding:10px 0 10px 13px; border:0; background:transparent; border-radius:10px; font-size:.78rem; box-shadow:none; }
  .search-field input:focus-visible { outline:none; }
  .tool-button { display:inline-flex; align-items:center; justify-content:center; flex:0 0 44px; width:44px; height:44px; min-height:44px; padding:0; border:1px solid transparent; border-radius:8px; background:transparent; color:var(--ink); box-shadow:none; }
  .tool-button :global(.icon) { width:18px; height:18px; }
  .tool-button:hover:not(:disabled), .search-result:hover, .reset-filters:hover:not(:disabled) { background:color-mix(in srgb, var(--accent) 9%, var(--surface)); }
  button:focus-visible, select:focus-visible, .check:has(input:focus-visible) { outline:2px solid var(--accent); outline-offset:2px; }
  .project-field { flex:0 1 172px; min-width:0; }
  .project-field select { width:100%; height:46px; border-color:var(--line); font-size:.75rem; text-overflow:ellipsis; }
  .filter-area { position:relative; flex-shrink:0; }
  .filter-toggle { position:relative; width:46px; height:46px; background:var(--surface); border-color:var(--line); }
  .filter-toggle.active { color:var(--accent); border-color:var(--accent); }
  .filter-dot { position:absolute; top:8px; right:8px; width:5px; height:5px; border-radius:50%; background:var(--accent); }
  .popover { position:absolute; top:calc(100% + 8px); z-index:10; border:1px solid var(--line); border-radius:12px; background:var(--surface); box-shadow:0 12px 36px color-mix(in srgb, var(--ink) 13%, transparent); }
  .search-results { left:0; width:max(100%, 300px); max-width:calc(100vw - 40px); overflow:hidden; }
  .popover-caption { margin:0; padding:12px 14px 8px; color:var(--muted); font-size:.68rem; }
  .result-items { max-height:min(340px, 48dvh); overflow-y:auto; overscroll-behavior:contain; padding:3px 6px; }
  .search-result { display:flex; align-items:flex-start; justify-content:center; flex-direction:column; gap:4px; width:100%; min-height:54px; text-align:left; padding:9px 8px; border:0; border-radius:6px; background:transparent; }
  .search-result:focus-visible { outline-offset:-2px; }
  .search-result strong { font-size:.77rem; font-weight:550; line-height:1.45; overflow-wrap:anywhere; }
  .search-result span { color:var(--muted); font-size:.65rem; font-weight:400; line-height:1.45; overflow-wrap:anywhere; }
  .search-hint { margin:0; padding:10px 14px; border-top:1px solid var(--line); color:var(--muted); font-size:.65rem; line-height:1.5; }
  .filter-popover { right:0; width:280px; max-width:calc(100vw - 40px); max-height:calc(100dvh - 170px); overflow-y:auto; overscroll-behavior:contain; padding:8px 14px 10px; }
  .popover-top { display:flex; align-items:center; justify-content:space-between; gap:10px; margin-bottom:8px; }
  .popover-top h3 { margin:0; font-size:.8rem; font-weight:600; }.popover-top .tool-button { margin-right:-6px; }
  .filter-field { display:grid; gap:7px; margin-bottom:14px; color:var(--muted); font-size:.72rem; }
  .filter-field select { font-size:.77rem; border-color:var(--line); }
  .check { display:flex; align-items:center; gap:9px; min-height:44px; font-size:.75rem; border-radius:4px; }
  .check input { flex-shrink:0; }
  .reset-filters { width:100%; min-height:44px; padding:8px; margin-top:6px; border:1px solid var(--line); background:transparent; font-size:.72rem; font-weight:500; }
  .graph-frame { position:relative; isolation:isolate; flex:1 0 auto; height:calc(100dvh - 160px); min-height:500px; overflow:hidden; border:1px solid var(--line); border-radius:14px; background:var(--surface); }
  .canvas-wrap { position:absolute; inset:0; }
  .empty { position:absolute; inset:0; display:flex; flex-direction:column; justify-content:center; align-items:center; gap:10px; padding:28px; text-align:center; color:var(--muted); }
  .empty :global(.icon) { width:28px; height:28px; opacity:.7; }.empty h3 { margin:0; font-size:.9rem; font-weight:550; color:var(--ink); }.empty p { margin:0; max-width:290px; font-size:.76rem; line-height:1.6; }
  .loading-state { position:absolute; z-index:2; top:16px; left:50%; transform:translateX(-50%); display:flex; align-items:center; gap:8px; padding:9px 12px; border:1px solid var(--line); border-radius:8px; color:var(--muted); background:var(--surface); font-size:.72rem; white-space:nowrap; pointer-events:none; }
  .loading-dot { width:6px; height:6px; border-radius:50%; background:var(--accent); }
  .cap-notice { position:absolute; top:12px; left:14px; right:14px; margin:0; color:var(--muted); font-size:.64rem; line-height:1.5; pointer-events:none; }
  .graph-frame[aria-busy="true"] .cap-notice { top:60px; }
  .graph-controls { position:absolute; right:14px; bottom:14px; z-index:2; display:flex; align-items:center; gap:2px; padding:3px; border:1px solid var(--line); border-radius:11px; background:var(--surface); box-shadow:0 3px 14px color-mix(in srgb, var(--ink) 6%, transparent); }
  .graph-controls .tool-button[aria-pressed="true"] { color:var(--accent); background:color-mix(in srgb, var(--accent) 10%, var(--surface)); }
  .control-divider { width:1px; height:18px; margin:0 3px; background:var(--line); }
  .legend { position:absolute; left:16px; bottom:30px; display:flex; align-items:center; flex-wrap:wrap; gap:12px; max-width:calc(100% - 230px); color:var(--muted); font-size:.62rem; pointer-events:none; }
  .legend span { display:flex; align-items:center; gap:5px; }.legend i { width:14px; border-top:2px solid #568b9d; }.legend .supersedes { border-top:2px solid #c88745; }
  @media(max-width:1000px) { .toolbar { max-width:none; flex-basis:100%; }.graph-frame { height:clamp(380px, 65dvh, 820px); min-height:380px; } }
  @media(max-width:520px) {
    .knowledge { gap:10px; }.graph-header { gap:10px; }.heading { width:100%; justify-content:space-between; gap:8px; }h2 { font-size:.95rem; }.counts { font-size:.72rem; gap:3px; white-space:normal; }.counts > span { margin:0 1px; }
    .toolbar { gap:8px; flex-wrap:wrap; }.search-area { flex-basis:100%; }.project-field { flex:1; }.project-field select { font-size:.88rem; padding-left:9px; padding-right:22px; }.search-field input { font-size:16px; padding-left:10px; }.search-submit { flex-basis:44px; width:44px; }.filter-toggle { width:44px; }
    .graph-frame { height:clamp(380px, 58dvh, 680px); min-height:380px; border-radius:11px; }.graph-controls { right:10px; bottom:10px; gap:0; }.legend { left:12px; bottom:74px; max-width:calc(100% - 24px); gap:10px; }.cap-notice { left:12px; top:10px; font-size:.7rem; }
    .search-results { width:min(300px, calc(100vw - 44px)); max-width:100%; }.filter-popover { max-width:calc(100vw - 24px); }.filter-popover { max-height:calc(100dvh - 190px); }
  }
</style>
