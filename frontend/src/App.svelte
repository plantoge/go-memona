<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Notes from './lib/components/Notes.svelte';
  import Keys from './lib/components/Keys.svelte';
  import Projects from './lib/components/Projects.svelte';
  import type { Project, ProjectInput } from './lib/projects';
  import NoteDetail from './lib/components/NoteDetail.svelte';
  import NoteEditor from './lib/components/NoteEditor.svelte';
  import type { NoteInput } from './lib/note-editor';
  import Icon from './lib/components/Icon.svelte';
  import Storage from './lib/components/Storage.svelte';
  import Graph from './lib/components/KnowledgeGraph.svelte';
  import { EMPTY_GRAPH, graphData } from './lib/graph';
  import type { StorageHealth, GraphData, KeyUsage } from './lib/types';
  import { createAPI, array, errorMessage, isStale, APIError } from './lib/api';
  import { date, type Note, type NoteDetail as Detail, type APIKey, type Activity, type View, type ConnectionInput } from './lib/types';
  import type { keyInput } from './lib/key-input';
  import { applyTheme, type Theme } from './lib/theme';

  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  function toggleTheme() { theme = theme === 'dark' ? 'light' : 'dark'; applyTheme(theme); }
  let endpoint = $state('');

  let session = $state<'checking' | 'out' | 'in'>('checking');
  let password = $state('');
  let authBusy = $state(false);
  let view = $state<View>('notes');
  let sidebarOpen = $state(true);
  let status = $state('Memeriksa sesi…');
  let error = $state('');
  let notes = $state<Note[]>([]);
  let projects = $state<string[]>([]);
  let registry = $state<Project[]>([]);
  let registrySaving = $state(false);
  let latestKeyID = $state('');
  let keys = $state<APIKey[]>([]);
  let keyNext = $state('');
  let keyCursors = $state<string[]>([]);
  let keyCursor = '';
  let keyUsage = $state<Record<string, KeyUsage>>({});
  let activity = $state<Activity[]>([]);
  let storage = $state<StorageHealth | null>(null);
  let graph = $state<GraphData>(EMPTY_GRAPH);
  let graphFilters = { query: '', project: '', archived: false };
  let loading = $state(false);
  let projectLoading = $state(false);
  let saving = $state(false);
  let token = $state('');
  let detailOpen = $state(false);
  let detail = $state<Detail | null>(null);
  let detailLoading = $state(false);
  let detailSaving = $state(false);
  let detailError = $state('');
  let creating = $state(false);
  let contentSaving = $state(false);
  let passwordField = $state<HTMLInputElement>();
  let main = $state<HTMLElement>();
  let requestID = 0;
  let detailID = 0;
  let filters = $state({ query: '', project: '', kind: '' });
  const titles: Record<View, string> = {
    notes: 'Semua memori', skills: 'Pustaka skill', keys: 'MCP Server', activity: 'Aktivitas', storage: 'Penyimpanan', graph: 'Knowledge Graph', projects: 'Proyek',
  };
  const api = createAPI(() => { signedOut(); status = 'Sesi berakhir. Silakan masuk kembali.'; });
  function showError(e: unknown) { if (!isStale(e)) error = errorMessage(e); }
  function signedOut() {
    api.reset(); requestID++; detailID++;
    creating = false; contentSaving = false;
    registry = []; registrySaving = false; latestKeyID = '';
    keyNext = ''; keyCursors = []; keyCursor = '';
    session = 'out'; password = ''; token = ''; endpoint = ''; detailOpen = false; detail = null;
    detailError = ''; notes = []; keys = []; keyUsage = {}; activity = []; projects = []; storage = null;
    graph = EMPTY_GRAPH; graphFilters = { query: '', project: '', archived: false };
    filters = { query: '', project: '', kind: '' }; loading = false; saving = false;
    void tick().then(() => passwordField?.focus());
  }
  async function signedIn(data: { csrf: string }) {
    if (!data || typeof data.csrf !== 'string' || !data.csrf) throw new Error('Sesi tidak valid: token CSRF tidak tersedia. Silakan masuk kembali.');
    api.setSession(data.csrf); session = 'in'; password = ''; error = '';
    view = 'notes'; filters = { query: '', project: '', kind: '' };
    await Promise.all([loadView(), loadProjects()]);
  }
  async function login(event: SubmitEvent) {
    event.preventDefault(); authBusy = true; error = ''; status = 'Memverifikasi kata sandi…';
    try { await signedIn(await api.request('/api/login', 'POST', { password })); }
    catch (e) { showError(e); status = 'Masuk belum berhasil.'; await tick(); passwordField?.focus(); }
    finally { password = ''; authBusy = false; }
  }
  async function logout() {
    authBusy = true; error = ''; token = '';
    try { await api.request('/api/logout', 'POST', {}); signedOut(); status = 'Anda sudah keluar.'; }
    catch (e) { showError(e); status = 'Keluar belum terkonfirmasi oleh server. Coba lagi.'; }
    finally { authBusy = false; }
  }
  async function loadProjects() {
    projectLoading = true;
    try {
      const [slugs, entries] = await Promise.all([
        api.request<string[] | null>('/api/projects'), api.request<Project[] | null>('/api/project-registry'),
      ]);
      registry = array(entries);
      projects = [...new Set([...array(slugs), ...registry.filter(p => !p.archived).map(p => p.slug)])].sort();
    }
    catch (e) { showError(e); }
    finally { projectLoading = false; }
  }
  async function loadView(keyPageCursor = '') {
    const id = ++requestID;
    loading = true; error = ''; status = 'Memuat data…';
    // Never leave previous results presented as current after a failed request.
    notes = []; keys = []; keyNext = ''; keyUsage = {}; activity = []; storage = null; graph = EMPTY_GRAPH;
    if (!keyPageCursor) { keyCursors = []; keyCursor = ''; }
    try {
      if (view === 'projects') {
        const entries = await api.request<Project[] | null>('/api/project-registry');
        if (id !== requestID) return;
        registry = array(entries); status = `${registry.length} proyek terdaftar.`;
      } else if (view === 'graph') {
        const params = new URLSearchParams({ project: graphFilters.project, q: graphFilters.query, archived: String(graphFilters.archived), limit: '500' });
        const result = await api.request<unknown>(`/api/graph?${params}`);
        if (id !== requestID) return;
        graph = graphData(result); status = `${graph.nodes.length} memori dan ${graph.edges.length} relasi dimuat.${graph.truncated ? ' Hasil dibatasi.' : ''}`;
      } else if (view === 'storage') {
        const report = await api.request<StorageHealth>('/api/storage-health');
        if (id !== requestID) return;
        storage = report; status = `Pemeriksaan penyimpanan: ${report.status}.`;
      } else if (view === 'keys') {
        const [result, info] = await Promise.all([
          api.request<{ items: APIKey[]; next_cursor: string; usage: Record<string, KeyUsage> }>(`/api/keys-page?${new URLSearchParams({ limit: '50', cursor: keyPageCursor })}`),
          api.request<{ mcp_endpoint: string }>('/api/server-info'),
        ]);
        if (id !== requestID) return;
        keys = array(result.items); keyNext = result.next_cursor; keyCursor = keyPageCursor; keyUsage = result.usage; endpoint = info.mcp_endpoint; status = `${keys.length} kunci API pada halaman ini.${keyNext ? ' Masih ada halaman berikutnya.' : ''}`;
      } else if (view === 'activity') {
        const result = array(await api.request<Activity[] | null>('/api/activity'));
        if (id !== requestID) return;
        activity = result; status = `${activity.length} aktivitas dimuat.`;
      } else {
        const params = new URLSearchParams({ q: filters.query, project: filters.project, kind: view === 'skills' ? 'skill' : filters.kind });
        const result = array(await api.request<Note[] | null>(`/api/notes?${params}`));
        if (id !== requestID) return;
        notes = result; status = `${notes.length} memori ditemukan.`;
      }
    } catch (e) {
      if (id !== requestID || isStale(e)) return;
      showError(e); status = 'Data gagal dimuat. Gunakan Muat ulang data untuk mencoba kembali.';
    } finally { if (id === requestID) loading = false; }
  }
  function changeKeyPage(next: boolean) {
    if (loading || saving || token) return;
    if (next && keyNext) { const cursor = keyNext; keyCursors = [...keyCursors, keyCursor]; void loadView(cursor); }
    else if (!next && keyCursors.length) { const cursor = keyCursors[keyCursors.length - 1]; keyCursors = keyCursors.slice(0, -1); void loadView(cursor); }
  }
  async function switchView(next: View) {
    if (next === view) return;
    if (saving || registrySaving || contentSaving || detailSaving) { status = 'Tunggu sampai perubahan selesai sebelum berpindah menu.'; return; }
    if (token && next !== 'keys' && !window.confirm('Token baru akan ditutup. Sudah menyimpannya dengan aman?')) return;
    if (next !== 'keys') token = '';
    view = next; filters = { query: '', project: '', kind: '' };
    graphFilters = { query: '', project: '', archived: false };
    if (window.matchMedia('(max-width: 760px)').matches) sidebarOpen = false;
    await tick(); main?.focus();
    await loadView();
  }
  function search(query: string, project: string, kind: string) { filters = { query, project, kind }; void loadView(); }
  function searchGraph(query: string, project: string, archived: boolean) { graphFilters = { query, project, archived }; void loadView(); }
  async function openNote(id: string) {
    const request = ++detailID;
    detailOpen = true; detail = null; detailLoading = true; detailError = '';
    try {
      const result = await api.request<Detail>(`/api/notes/${encodeURIComponent(id)}?history=paged`);
      if (request !== detailID) return;
      if (!result || !result.note || typeof result.html !== 'string') throw new Error('Detail memori tidak valid. Tutup dan buka kembali.');
      result.history = array(result.history); detail = result;
    } catch (e) { if (request === detailID && !isStale(e)) detailError = errorMessage(e); }
    finally { if (request === detailID) detailLoading = false; }
  }
  function closeDetail() { detailID++; detailOpen = false; detail = null; }
  async function loadHistory(before: number) {
    if (!detail || detailLoading || detailSaving) return;
    const id = detail.note.id, request = detailID;
    detailSaving = true; detailError = '';
    try {
      const page = await api.request<{ items: Note[]; next_before: number }>(`/api/notes/${encodeURIComponent(id)}/history-page?${new URLSearchParams({ before: String(before), limit: '20' })}`);
      if (request !== detailID || !detail || detail.note.id !== id) return;
      if (!Array.isArray(page.items) || !Number.isInteger(page.next_before)) throw new Error('Halaman riwayat tidak valid.');
      detail = { ...detail, history: page.items, history_next_before: page.next_before };
    } catch (e) { if (request === detailID && !isStale(e)) detailError = errorMessage(e); }
    finally { if (request === detailID) detailSaving = false; }
  }
  async function restoreSkillContent(revision: number) {
    if (!detail || detailSaving || detailLoading || detail.note.kind !== 'skill') return;
    const note = detail.note, request = detailID;
    if (!window.confirm(`Salin judul, isi, dan sumber revisi ${revision} menjadi revisi skill baru berstatus draf? Relasi saat ini dipertahankan. Publikasi harus ditinjau terpisah.`)) return;
    detailSaving = true; detailError = '';
    try {
      await api.request(`/api/notes/${encodeURIComponent(note.id)}/restore-skill-content`, 'POST', { expected_revision: note.revision, source_revision: revision });
      if (request === detailID) await openNote(note.id);
      if (session === 'in') await loadView();
    } catch (e) { if (request === detailID && !isStale(e)) detailError = errorMessage(e); }
    finally { if (session === 'in') detailSaving = false; }
  }
  async function checkDuplicates(input: NoteInput, excludeID: string): Promise<import('./lib/types').DuplicateCandidate[]> {
    const result = await api.request<{ candidates: import('./lib/types').DuplicateCandidate[] }>('/api/duplicate-candidates', 'POST', { project: input.project, title: input.title, content: input.content, exclude_id: excludeID });
    if (!Array.isArray(result.candidates)) throw new Error('Respons kandidat tidak valid.');
    return result.candidates;
  }
  async function saveContent(input: NoteInput, create = false): Promise<boolean> {
    if (contentSaving || detailSaving || (!create && !detail)) return false;
    const note = detail?.note, request = detailID;
    contentSaving = true;
    if (!create) detailSaving = true;
    error = ''; detailError = '';
    try {
      const path = create ? '/api/notes' : `/api/notes/${encodeURIComponent(note!.id)}/revise`;
      const result = await api.request<Note>(path, 'POST', create ? input : { ...input, expected_revision: note!.revision });
      if (create) creating = false;
      if (session === 'in') {
        await Promise.all([loadView(), loadProjects()]);
        if (create || request === detailID) await openNote(result.id);
      }
      return true;
    } catch (e) {
      if (!isStale(e)) {
        if (create) showError(e);
        else if (request === detailID) detailError = errorMessage(e);
      }
      return false;
    } finally { contentSaving = false; if (!create) detailSaving = false; }
  }
  async function saveConnections(input: ConnectionInput): Promise<boolean> {
    if (!detail || detailSaving) return false;
    const note = detail.note, request = detailID;
    detailSaving = true; detailError = '';
    try {
      await api.request(`/api/notes/${encodeURIComponent(note.id)}/connections`, 'POST', { expected_revision: note.revision, ...input });
      if (request === detailID) await openNote(note.id);
      if (session === 'in') await loadView();
      return true;
    } catch (e) {
      if (request === detailID && !isStale(e)) detailError = errorMessage(e);
      return false;
    } finally { detailSaving = false; }
  }
  async function changeNote(action: 'publish' | 'archive' | 'restore') {
    if (!detail || detailSaving) return;
    const note = detail.note;
    const message = action === 'archive' ? `Arsipkan “${note.title}”? Data tetap disimpan dan dapat dipulihkan.` : action === 'restore' ? `Pulihkan “${note.title}” dari arsip?` : `Terbitkan “${note.title}” revisi ${note.revision} sebagai skill? Pastikan Anda sudah meninjau pemicu penggunaan, kompatibilitas harness, sumber dan bukti pengujian. Publikasi bukan bukti skill aman atau telah diuji otomatis.`;
    if (!window.confirm(message)) return;
    const request = detailID;
    detailSaving = true; detailError = '';
    try {
      await api.request(`/api/notes/${encodeURIComponent(note.id)}/${action}`, 'POST', { expected_revision: note.revision });
      if (request === detailID) await openNote(note.id);
      if (session === 'in') await loadView();
    } catch (e) {
      if (request === detailID && !isStale(e)) detailError = e instanceof APIError && e.status === 409 ? 'Revisi sudah berubah. Tutup dan buka kembali detail sebelum mencoba lagi.' : errorMessage(e);
    } finally { detailSaving = false; }
  }
  async function createKey(input: ReturnType<typeof keyInput>): Promise<boolean> {
    saving = true; error = '';
    try {
      const result = await api.request<{ token: string; key: APIKey }>('/api/keys', 'POST', input);
      if (!result || typeof result.token !== 'string' || !result.token) throw new Error('Server tidak mengembalikan token. Periksa daftar kunci sebelum mencoba lagi.');
      token = result.token;
      latestKeyID = result.key?.id || '';
      if (view === 'keys') await loadView();
      status = 'Kunci dibuat. Token tersedia sekali; simpan sekarang.'; return true;
    } catch (e) { showError(e); return false; }
    finally { saving = false; }
  }
  async function revokeKey(key: APIKey) {
    saving = true; error = '';
    try {
      await api.request(`/api/keys/${encodeURIComponent(key.id)}/revoke`, 'POST', {});
      await loadView();
    }
    catch (e) { showError(e); throw e; }
    finally { saving = false; }
  }
  async function saveProject(input: ProjectInput): Promise<void> {
    registrySaving = true; error = '';
    try {
      const result = await api.request<Project>('/api/project-registry', 'POST', input);
      registry = [...registry.filter(p => p.slug !== result.slug), result].sort((a, b) => a.slug.localeCompare(b.slug));
      await loadProjects(); status = `Metadata proyek ${result.slug} disimpan.`;
    } catch (e) { showError(e); throw e; }
    finally { registrySaving = false; }
  }
  onMount(() => {
    if (window.matchMedia('(max-width: 760px)').matches) sidebarOpen = false;
    void api.request<{ csrf: string }>('/api/session').then(signedIn).catch(e => {
      signedOut(); status = 'Silakan masuk untuk melanjutkan.';
      if (!(e instanceof APIError && e.status === 401)) showError(e);
    });
    return () => { api.reset(); };
  });
</script>

<a class="skip-link" href="#main">Lewati ke konten</a>
<p class="sr-only" role="status" aria-live="polite">{status}</p>
<div class="utility-controls">
  <button type="button" class="icon-button" aria-label={theme === 'dark' ? 'Gunakan mode terang' : 'Gunakan mode gelap'} title={theme === 'dark' ? 'Mode terang' : 'Mode gelap'} onclick={toggleTheme}><Icon name={theme === 'dark' ? 'sun' : 'moon'} /></button>
  {#if session === 'in'}<button type="button" class="icon-button" aria-label="Muat ulang data" title="Muat ulang data" onclick={() => { void loadView(); void loadProjects(); }}><Icon name="refresh" /></button>{/if}
</div>
{#if session === 'out'}
  <section class="login-wrap" aria-labelledby="login-title">
    <div class="login-card">
      <div class="brand"><span class="brand-mark" aria-hidden="true">m</span> memona<span class="brand-dot" aria-hidden="true">.</span></div>
      <h1 id="login-title">Masuk</h1>
      {#if error}<p class="inline-error" role="alert">{error}</p>{/if}
      <form onsubmit={login}><label for="password">Kata sandi administrator</label><input id="password" bind:this={passwordField!} bind:value={password} type="password" autocomplete="current-password" required><button type="submit" class="primary full" disabled={authBusy}>{authBusy ? 'Memverifikasi…' : 'Masuk ke ruang memori'}</button></form>
      <p class="hint">Administrator Memona</p>
    </div>
  </section>
{:else if session === 'in'}
  <div class="shell" class:sidebar-closed={!sidebarOpen}>
    <button type="button" class="sidebar-toggle" aria-expanded={sidebarOpen} aria-controls="sidebar-content" aria-label={sidebarOpen ? 'Ciutkan menu navigasi' : 'Perluas menu navigasi'} title={sidebarOpen ? 'Ciutkan menu' : 'Perluas menu'} onclick={() => { sidebarOpen = !sidebarOpen; }}>
      <Icon name="panel" /><span class="toggle-label">Memona</span><span class="toggle-chevron"><Icon name="chevron" /></span>
    </button>
    <aside id="sidebar-content" class="sidebar" aria-label="Ruang kerja">
      <div class="sidebar-inner">
        <div class="sidebar-brand"><span class="brand-mark" aria-hidden="true">m</span><span class="nav-text">memona</span></div>
        <nav aria-label="Navigasi utama" class={`active-${view}`}>
          <span class="nav-indicator" style:transform={`translateY(${Object.keys(titles).indexOf(view) * 56}px)`} aria-hidden="true"></span>
          {#each Object.entries(titles) as [name, title]}
            <button type="button" aria-label={title} title={!sidebarOpen ? title : undefined} aria-current={view === name ? 'page' : undefined} onclick={() => switchView(name as View)}><Icon name={name as View} /><span class="nav-text">{title}</span></button>
          {/each}
        </nav>
        <div class="sidebar-bottom"><button type="button" class="secondary sidebar-logout" aria-label="Keluar" title={!sidebarOpen ? 'Keluar' : undefined} disabled={authBusy} onclick={logout}><Icon name="logout" /><span class="nav-text">Keluar</span></button></div>
      </div>
    </aside>
    <main id="main" class:graph-workspace={view === 'graph'} tabindex="-1" bind:this={main!}>
      <h1 class="sr-only">{titles[view]}</h1>
      {#if error}<p id="error" role="alert">{error}</p>{/if}
      {#if view === 'notes' || view === 'skills'}
        <div class="actions"><button type="button" class="primary" disabled={contentSaving} onclick={() => { creating = !creating; }}>{creating ? 'Tutup editor baru' : 'Buat memori'}</button></div>
         {#if creating}<section aria-label="Memori baru"><NoteEditor saving={contentSaving} project={filters.project} onduplicates={checkDuplicates} onsubmit={input => saveContent(input, true)} oncancel={() => { creating = false; }} /></section>{/if}
        {#key view}<Notes {notes} {projects} {loading} {projectLoading} skills={view === 'skills'} onsearch={search} onopen={openNote} />{/key}
      {:else if view === 'graph'}
        <Graph data={graph} {projects} {loading} {projectLoading} suspended={detailOpen} onsearch={searchGraph} onopen={openNote} />
      {:else if view === 'keys'}
        <Keys {keys} {keyUsage} {loading} {saving} {token} {endpoint} {error} {latestKeyID} projectOptions={projects} oncreate={createKey} onrevoke={revokeKey} ondismiss={() => { token = ''; status = 'Token ditutup dan tidak dapat ditampilkan kembali.'; }} onstatus={message => status = message} />
        <nav aria-label="Halaman kunci API" class="actions"><button type="button" class="secondary" disabled={loading || saving || !!token || !keyCursors.length} onclick={() => changeKeyPage(false)}>Sebelumnya</button><button type="button" class="secondary" disabled={loading || saving || !!token || !keyNext} onclick={() => changeKeyPage(true)}>Berikutnya</button></nav>
        <p class="hint">Maksimal 50 key per halaman. Muat ulang untuk melihat key baru. Pemilihan pengganti rotasi memerlukan key pengganti pada halaman yang dimuat; key yang tidak terlihat bukan berarti dicabut.</p>
      {:else if view === 'projects'}
        <Projects entries={registry} knownSlugs={projects} {loading} saving={registrySaving} onsave={saveProject} />
      {:else if view === 'storage'}
        <Storage report={storage} {loading} onrefresh={() => { void loadView(); }} />
      {:else}
        <section aria-labelledby="activity-heading"><h2 id="activity-heading">Aktivitas</h2><div class="stack" aria-busy={loading}>
          {#if loading}<p class="empty-state">Memuat aktivitas…</p>{:else}{#each activity as entry}<article class="activity-item"><p class="activity-time">{date(entry.time)}</p><h3>{entry.action}</h3><p class="muted">Pelaku: {entry.actor || '—'} · Target: {entry.target || '—'}</p></article>{:else}<p class="empty-state">Belum ada aktivitas yang tercatat.</p>{/each}{/if}
        </div></section>
      {/if}
      <!-- No decorative footer: keep the workspace focused on content. -->
    </main>
  </div>
{/if}
{#if detailOpen && session === 'in'}<NoteDetail {detail} contentFirst={view === 'graph'} loading={detailLoading} saving={detailSaving} error={detailError} onclose={closeDetail} onchange={changeNote} onopen={openNote} onhistory={loadHistory} onrestoreskill={restoreSkillContent} onduplicates={checkDuplicates} onsaveconnections={saveConnections} onsavecontent={input => saveContent(input)} />{/if}
