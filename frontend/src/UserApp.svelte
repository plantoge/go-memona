<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Icon from './lib/components/Icon.svelte';
  import { applyTheme, type Theme } from './lib/theme';
  import { createAPI, array, errorMessage, isStale } from './lib/api';
  import { date, kindLabels, statusLabel, type Note } from './lib/types';
  import { splitWorkspaces, notePath, expiryValue, type Workspace, type Invitation, type UserKey } from './user-contract';
  type View = 'notes' | 'keys' | 'teams';
  type NoteInput = { workspace_id: string; kind: string; title: string; content: string; status: string; source: string };
  const blankNote = (): NoteInput => ({ workspace_id: '', kind: 'knowledge', title: '', content: '', status: 'unverified', source: '' });

  let session = $state<'checking' | 'out' | 'in'>('checking');
  let user = $state<{ id: string; email: string; name: string } | null>(null);
  let personal = $state<Workspace | null>(null);
  let teams = $state<Workspace[]>([]);
  let workspaceID = $state('');
  let view = $state<View>('notes');
  let notes = $state<Note[]>([]);
  let keys = $state<UserKey[]>([]);
  let invitations = $state<Invitation[]>([]);
  let selected = $state<Note | null>(null);
  let editor = $state<NoteInput>(blankNote());
  let editing = $state(false);
  let teamName = $state('');
  let inviteEmail = $state('');
  let keyName = $state('');
  let keyPermissions = $state<string[]>(['read']);
  let keyExpiry = $state('');
  let token = $state('');
  let busy = $state(false);
  let loading = $state(false);
  let error = $state('');
  let message = $state('');
  let loadID = 0;
  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  let sidebarOpen = $state(true);
  let compactLayout = $state(false);
  let main = $state<HTMLElement>();
  function toggleTheme() { theme = theme === 'dark' ? 'light' : 'dark'; applyTheme(theme); }
  const api = createAPI(() => { clearSession(); message = 'Sesi berakhir. Silakan masuk kembali.'; });
  const workspaces = $derived(personal ? [personal, ...teams] : teams);
  const current = $derived(workspaces.find(w => w.id === workspaceID));
  const canInvite = $derived(teams.some(t => t.id === workspaceID && (t.role === 'owner' || t.role === 'admin')));

  function clearSession() {
    loadID++;
    api.reset(); session = 'out'; user = null; personal = null; teams = []; workspaceID = '';
    notes = []; keys = []; invitations = []; selected = null; editing = false; editor = blankNote(); token = '';
  }
  function fail(e: unknown) { if (!isStale(e)) { error = errorMessage(e); message = ''; } }
  async function initialize() {
    try {
      const result = await api.request<{ user: { id: string; email: string; name: string }; csrf: string }>('/api/user/session');
      if (!result?.user?.id || !result.csrf) throw new Error('Sesi tidak valid. Muat ulang halaman.');
      api.setSession(result.csrf); user = result.user; session = 'in';
      await loadWorkspaces();
    } catch (e) {
      if (!isStale(e)) { clearSession(); if (e instanceof Error && !('status' in e && e.status === 401)) fail(e); }
    }
  }
  onMount(() => {
    const layout = window.matchMedia('(max-width: 1024px)');
    function updateLayout() {
      if (compactLayout !== layout.matches) {
        compactLayout = layout.matches;
        sidebarOpen = !layout.matches;
      }
    }
    updateLayout();
    layout.addEventListener('change', updateLayout);
    void initialize();
    return () => { layout.removeEventListener('change', updateLayout); api.reset(); };
  });
  function closeMenuOnEscape(event: KeyboardEvent) {
    if (event.key === 'Escape' && compactLayout && sidebarOpen) {
      sidebarOpen = false;
      document.querySelector<HTMLButtonElement>('.sidebar-toggle')?.focus();
    }
  }

  async function loadWorkspaces(preferred = workspaceID) {
    const result = array(await api.request<Workspace[] | null>('/api/user/workspaces'));
    const split = splitWorkspaces(result);
    personal = split.personal; teams = split.teams;
    const ids = [split.personal.id, ...split.teams.map(t => t.id)];
    workspaceID = ids.includes(preferred) ? preferred : split.personal.id;
    await loadView();
  }
  async function loadView() {
    const id = ++loadID;
    loading = true; error = ''; selected = null; editing = false;
    notes = []; keys = []; invitations = [];
    try {
      if (view === 'notes') {
        const result = array(await api.request<Note[] | null>(`/api/user/notes?${new URLSearchParams({ workspace_id: workspaceID })}`));
        if (id === loadID) notes = result;
      } else if (view === 'keys') {
        const result = array(await api.request<UserKey[] | null>('/api/user/keys'));
        if (id === loadID) keys = result;
      } else {
        const result = array(await api.request<Invitation[] | null>('/api/user/invitations'));
        if (id === loadID) invitations = result;
      }
    } catch (e) { if (id === loadID) fail(e); }
    finally { if (id === loadID) loading = false; }
  }
  function changeWorkspace(event: Event) {
    const next = (event.currentTarget as HTMLSelectElement).value;
    if (token && !confirm('Token baru akan disembunyikan. Sudah menyimpannya dengan aman?')) {
      (event.currentTarget as HTMLSelectElement).value = workspaceID; return;
    }
    if (token) dismissToken();
    workspaceID = next;
    message = ''; void loadView();
  }
  function switchView(next: View) {
    if (token && next !== 'keys' && !confirm('Token baru akan disembunyikan. Sudah menyimpannya dengan aman?')) return;
    if (token && next !== 'keys') dismissToken();
    view = next; message = '';
    if (compactLayout) sidebarOpen = false;
    void tick().then(() => main?.focus());
    void loadView();
  }
  async function action(task: () => Promise<void>) {
    if (busy) return;
    busy = true; error = ''; message = '';
    try { await task(); } catch (e) { fail(e); }
    finally { busy = false; }
  }
  function dismissToken() { token = ''; }
  async function logout() {
    if (token && !confirm('Token yang baru dibuat belum disalin? Token tidak dapat ditampilkan lagi.')) return;
    await action(async () => { await api.request('/api/user/logout', 'POST', {}); clearSession(); message = 'Anda sudah keluar.'; });
  }
  async function createTeam(event: SubmitEvent) {
    event.preventDefault();
    await action(async () => {
      const created = await api.request<Workspace>('/api/user/teams', 'POST', { name: teamName.trim() });
      teamName = ''; await loadWorkspaces(created?.id); message = 'Tim dibuat.';
    });
  }
  async function invite(event: SubmitEvent) {
    event.preventDefault();
    await action(async () => {
      await api.request(`/api/user/teams/${encodeURIComponent(workspaceID)}/invite`, 'POST', { email: inviteEmail.trim() });
      inviteEmail = ''; message = 'Undangan tersedia di aplikasi untuk akun Google dengan email tersebut. Belum ada email otomatis.';
    });
  }
  async function accept(id: string) {
    await action(async () => {
      await api.request(`/api/user/invitations/${encodeURIComponent(id)}/accept`, 'POST', {});
      await loadWorkspaces(); message = 'Undangan diterima.';
    });
  }
  function togglePermission(permission: string) {
    keyPermissions = keyPermissions.includes(permission) ? keyPermissions.filter(p => p !== permission) : [...keyPermissions, permission];
  }
  async function createKey(event: SubmitEvent) {
    event.preventDefault();
    if (!keyPermissions.length) { error = 'Pilih setidaknya satu izin.'; return; }
    if (token && !confirm('Token sebelumnya akan disembunyikan. Sudah menyimpannya dengan aman?')) return;
    await action(async () => {
      const result = await api.request<{ token: string; key: UserKey }>('/api/user/keys', 'POST', {
        workspace_id: workspaceID, name: keyName.trim(), permissions: keyPermissions,
        expires_at: expiryValue(keyExpiry),
      });
      if (!result?.token) throw new Error('Token tidak tersedia. Periksa daftar kunci sebelum membuat lagi.');
      token = result.token; keyName = ''; keyExpiry = '';
      await loadView(); message = 'Kunci dibuat. Token hanya ditampilkan sekali.';
    });
  }
  async function revoke(key: UserKey) {
    if (!confirm(`Cabut kunci “${key.name}”? Akses MCP-nya akan berhenti.`)) return;
    await action(async () => {
      await api.request(`/api/user/keys/${encodeURIComponent(key.id)}/revoke`, 'POST', {});
      await loadView(); message = 'Kunci dicabut.';
    });
  }
  async function openNote(note: Note) {
    const id = ++loadID;
    loading = true; error = ''; selected = null; editing = false;
    try {
      const result = await api.request<Note | { note: Note }>(notePath(note.id, workspaceID));
      if (id === loadID) {
        const item = 'note' in result ? result.note : result;
        if (!item?.id) throw new Error('Detail memori tidak valid.');
        selected = item;
      }
    } catch (e) { if (id === loadID) fail(e); }
    finally { if (id === loadID) loading = false; }
  }
  function startNote(note?: Note) {
    selected = note || null;
    editor = note ? { workspace_id: workspaceID, kind: note.kind, title: note.title, content: note.content, status: note.kind === 'skill' ? 'draft' : note.status, source: note.source } : { ...blankNote(), workspace_id: workspaceID };
    editing = true; error = ''; message = '';
  }
  function noteKindChanged() {
    editor.status = editor.kind === 'skill' ? 'draft' : 'unverified';
  }
  async function saveNote(event: SubmitEvent) {
    event.preventDefault();
    await action(async () => {
      const previous = selected;
      const result = await api.request<Note>(previous ? notePath(previous.id, workspaceID) : '/api/user/notes', 'POST',
        previous ? { ...editor, expected_revision: previous.revision } : editor);
      editing = false; await loadView();
      if (result?.id) await openNote(result);
      message = previous ? 'Memori diperbarui.' : 'Memori dibuat.';
    });
  }
</script>

<svelte:head><title>Memona — Ruang kerja saya</title></svelte:head>
<svelte:window onkeydown={closeMenuOnEscape} />
<a class="skip-link" href="#main">Lewati ke konten</a>
<div class="utility-controls">
  <button type="button" class="icon-button" aria-label={theme === 'dark' ? 'Gunakan mode terang' : 'Gunakan mode gelap'} title={theme === 'dark' ? 'Mode terang' : 'Mode gelap'} onclick={toggleTheme}><Icon name={theme === 'dark' ? 'sun' : 'moon'} /></button>
  {#if session === 'in'}<button type="button" class="icon-button" aria-label="Muat ulang data" title="Muat ulang data" onclick={() => loadView()} disabled={loading || busy}><Icon name="refresh" /></button>{/if}
</div>
<div class="user-dashboard">
  {#if session === 'checking'}
    <main id="main" class="center-state"><p role="status">Memeriksa sesi…</p></main>
  {:else if session === 'out'}
    <main id="main" class="login-wrap"><section class="login-card" aria-labelledby="login-title">
      <div class="brand"><span class="brand-mark" aria-hidden="true">m</span> memona<span class="brand-dot" aria-hidden="true">.</span></div>
      <p class="eyebrow">RUANG KERJA / MEMONA</p>
      <h1 id="login-title">Masuk</h1>
      <p>Kelola memori pribadi dan tim dengan akun Google Anda.</p>
      {#if error}<p class="inline-error" role="alert">{error}</p>{/if}
      {#if message}<p class="notice" role="status">{message}</p>{/if}
      <a class="user-primary-link primary full" href="/auth/google/start">Masuk dengan Google</a>
      <p class="hint">Jika login Google belum tersedia, hubungi administrator.</p>
      <p class="hint">Administrator? <a href="/admin">Buka dashboard admin</a></p>
    </section></main>
  {:else}
    <div class="shell" class:sidebar-closed={!sidebarOpen} class:compact-layout={compactLayout}>
      {#if compactLayout && sidebarOpen}<button type="button" class="sidebar-scrim" aria-label="Tutup menu navigasi" tabindex="-1" onclick={() => { sidebarOpen = false; }}></button>{/if}
      <button type="button" class="sidebar-toggle" aria-expanded={sidebarOpen} aria-controls="sidebar-content" aria-label={compactLayout ? sidebarOpen ? 'Tutup menu navigasi' : 'Buka menu navigasi' : sidebarOpen ? 'Ciutkan menu navigasi' : 'Perluas menu navigasi'} title={sidebarOpen ? 'Tutup menu' : 'Buka menu'} onclick={() => { sidebarOpen = !sidebarOpen; }}>
        <Icon name="panel" /><span class="toggle-label">{compactLayout ? sidebarOpen ? 'Tutup menu' : 'Menu' : 'Memona'}</span><span class="toggle-chevron"><Icon name="chevron" /></span>
      </button>
      <aside id="sidebar-content" class="sidebar" aria-label="Ruang kerja" aria-hidden={compactLayout && !sidebarOpen} inert={compactLayout && !sidebarOpen}>
        <div class="sidebar-inner">
          <div class="sidebar-brand"><span class="brand-mark" aria-hidden="true">m</span><span class="nav-text">memona</span></div>
          <div class="workspace-picker"><label for="workspace">Ruang kerja</label>
            <select id="workspace" value={workspaceID} onchange={changeWorkspace} disabled={busy}>
              {#each workspaces as workspace (workspace.id)}<option value={workspace.id}>{workspace.name}{workspace.id === personal?.id ? ' · Pribadi' : ''}</option>{/each}
            </select>
            {#if current?.role}<p class="hint">Peran: {current.role}</p>{/if}
          </div>
          <nav aria-label="Navigasi utama" class={`active-${view}`}>
            <span class="nav-indicator" aria-hidden="true"></span>
            <button type="button" aria-label="Memori" title={!sidebarOpen ? 'Memori' : undefined} aria-current={view === 'notes' ? 'page' : undefined} onclick={() => switchView('notes')} disabled={busy}><Icon name="notes" /><span class="nav-text">Memori</span></button>
            <button type="button" aria-label="Kunci MCP" title={!sidebarOpen ? 'Kunci MCP' : undefined} aria-current={view === 'keys' ? 'page' : undefined} onclick={() => switchView('keys')} disabled={busy}><Icon name="keys" /><span class="nav-text">Kunci MCP</span></button>
            <button type="button" aria-label="Tim & undangan" title={!sidebarOpen ? 'Tim & undangan' : undefined} aria-current={view === 'teams' ? 'page' : undefined} onclick={() => switchView('teams')} disabled={busy}><Icon name="projects" /><span class="nav-text">Tim & undangan</span></button>
          </nav>
          <div class="sidebar-bottom"><p class="sidebar-account nav-text">{user?.name || user?.email}</p><button type="button" class="secondary sidebar-logout" aria-label="Keluar" title={!sidebarOpen ? 'Keluar' : undefined} disabled={busy} onclick={logout}><Icon name="logout" /><span class="nav-text">Keluar</span></button></div>
        </div>
      </aside>
      <main id="main" tabindex="-1" bind:this={main!} inert={compactLayout && sidebarOpen}>
        <header class="workspace-heading"><span class="eyebrow">RUANG KERJA / {current?.name || 'MEMONA'}</span><h1>{view === 'notes' ? 'Memori' : view === 'keys' ? 'Kunci MCP' : 'Tim & undangan'}</h1>
          {#if !sidebarOpen && !compactLayout}<div class="collapsed-workspace"><label for="workspace-collapsed">Ruang kerja</label><select id="workspace-collapsed" value={workspaceID} onchange={changeWorkspace} disabled={busy}>{#each workspaces as workspace (workspace.id)}<option value={workspace.id}>{workspace.name}{workspace.id === personal?.id ? ' · Pribadi' : ''}</option>{/each}</select></div>{/if}
        </header>
        {#if error}<p class="alert" role="alert">{error}</p>{/if}
        {#if message}<p class="notice" role="status">{message}</p>{/if}
        {#if loading}<p role="status">Memuat data…</p>{/if}
        {#if view === 'notes'}
          <div class="section-head"><h2>Catatan ruang kerja</h2><button class="button primary" type="button" onclick={() => startNote()} disabled={busy}>Memori baru</button></div>
          {#if editing}
            <form class="panel form-grid" onsubmit={saveNote}>
              <h3>{selected ? 'Sunting memori' : 'Memori baru'}</h3>
              <label>Judul<input required maxlength="200" bind:value={editor.title}></label>
              <label>Jenis<select bind:value={editor.kind} onchange={noteKindChanged} disabled={!!selected}><option value="knowledge">Pengetahuan</option><option value="fact">Fakta</option><option value="decision">Keputusan</option><option value="handoff">Handoff</option><option value="skill">Skill</option></select></label>
              <label>Status<select bind:value={editor.status}>{#if editor.kind === 'skill'}<option value="draft">Draf</option>{:else}<option value="unverified">Belum diverifikasi</option><option value="verified">Terverifikasi</option>{/if}</select></label>
              <label>Sumber<input required bind:value={editor.source} placeholder="Asal informasi"></label>
              <label class="wide">Isi<textarea required rows="10" bind:value={editor.content}></textarea></label>
              <div class="form-actions"><button type="button" class="secondary" onclick={() => editing = false} disabled={busy}>Batal</button><button class="primary" disabled={busy} type="submit">{busy ? 'Menyimpan…' : 'Simpan memori'}</button></div>
            </form>
          {:else if selected}
            <article class="panel note-detail"><div class="section-head"><div><p class="eyebrow">{kindLabels[selected.kind] || selected.kind} · Revisi {selected.revision}</p><h2>{selected.title}</h2></div><button type="button" class="secondary" onclick={() => selected = null}>Tutup</button></div><p class="hint">{statusLabel(selected.status)} · {date(selected.updated_at)} · Sumber: {selected.source}</p><pre class="note-body">{selected.content}</pre><button class="secondary" type="button" onclick={() => { if (selected) startNote(selected); }} disabled={busy}>Sunting</button></article>
          {/if}
          {#if !loading && notes.length === 0}<p class="empty">Belum ada memori di ruang kerja ini.</p>{/if}
          <ul class="item-list">{#each notes as note (note.id)}<li><button type="button" class="item-button" onclick={() => openNote(note)}><strong>{note.title}</strong><span>{kindLabels[note.kind] || note.kind} · {statusLabel(note.status)} · {date(note.updated_at)}</span></button></li>{/each}</ul>
        {:else if view === 'keys'}
          <p class="lead">Buat kunci untuk menghubungkan MCP ke ruang kerja yang dipilih. Token baru hanya ditampilkan sekali.</p><p class="hint">Endpoint MCP: <code>{window.location.origin}/mcp</code>. Project untuk tool MCP: <code>{current?.project || '—'}</code>. Gunakan project ini saat menyimpan atau mencari memori di ruang kerja terpilih.</p><p class="hint">Batas awal: 100 kunci per akun, termasuk kunci yang dicabut.</p>
          {#if token}<section class="panel token-panel" aria-label="Token kunci baru"><h2>Salin token sekarang</h2><p>Jangan bagikan token ini. Setelah ditutup, token tidak dapat ditampilkan kembali.</p><code>{token}</code><button type="button" class="quiet" onclick={dismissToken}>Saya sudah menyimpan token</button></section>{/if}
          <form class="panel form-grid" onsubmit={createKey}><h2>Kunci baru</h2><label>Nama kunci<input required maxlength="100" bind:value={keyName}></label><label>Kedaluwarsa (opsional)<input type="date" bind:value={keyExpiry}></label><fieldset class="wide"><legend>Izin</legend>{#each ['read', 'write'] as permission}<label class="check"><input type="checkbox" checked={keyPermissions.includes(permission)} onchange={() => togglePermission(permission)}>{permission}</label>{/each}</fieldset><div class="form-actions"><button class="button primary" type="submit" disabled={busy || !keyPermissions.length}>Buat kunci</button></div></form>
          <h2>Kunci Anda</h2>{#if !loading && keys.length === 0}<p class="empty">Belum ada kunci.</p>{/if}
          <ul class="item-list">{#each keys as key (key.id)}<li class="item-row"><div><strong>{key.name}</strong><span>{key.prefix || key.id} · {key.permissions.join(', ')} · {key.revoked ? 'Dicabut' : 'Aktif'}{key.expires_at ? ` · Berlaku hingga ${date(key.expires_at)}` : ''}</span><span>Ruang kerja: {workspaces.find(w => w.id === key.workspace_id)?.name || key.workspace_id}</span></div>{#if !key.revoked}<button type="button" class="danger" onclick={() => revoke(key)} disabled={busy}>Cabut</button>{/if}</li>{/each}</ul>
        {:else}
          <div class="two-columns"><section class="panel"><h2>Buat tim</h2><p class="hint">Pisahkan pengetahuan bersama dari ruang pribadi. Batas awal: 10 tim yang dibuat per akun.</p><form class="form-grid" onsubmit={createTeam}><label>Nama tim<input required maxlength="100" bind:value={teamName}></label><button type="submit" class="button primary" disabled={busy}>Buat tim</button></form></section>
          <section class="panel"><h2>Undang anggota</h2>{#if canInvite}<p class="hint">Tambahkan undangan dalam aplikasi untuk alamat Google anggota tim {current?.name}; email otomatis belum dikirim.</p><form class="form-grid" onsubmit={invite}><label>Alamat email<input type="email" required autocomplete="email" bind:value={inviteEmail}></label><button type="submit" class="button primary" disabled={busy}>Buat undangan</button></form>{:else}<p class="hint">Pilih tim yang Anda kelola untuk mengundang anggota.</p>{/if}</section></div>
          <h2>Undangan untuk Anda</h2>{#if !loading && invitations.length === 0}<p class="empty">Tidak ada undangan.</p>{/if}
          <ul class="item-list">{#each invitations as invitation (invitation.id)}<li class="item-row"><div><strong>{invitation.workspace_name}</strong><span>{invitation.email}{invitation.status ? ` · ${invitation.status}` : ''}</span></div>{#if !invitation.status || invitation.status === 'pending'}<button type="button" class="button" onclick={() => accept(invitation.id)} disabled={busy}>Terima</button>{/if}</li>{/each}</ul>
        {/if}
      </main>
    </div>
  {/if}
</div>
