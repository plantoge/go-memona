<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { date, type APIKey, type KeyUsage } from '../types';
  import { keyInput } from '../key-input';
  import { errorMessage } from '../api';
  import IntegrationGuide from './IntegrationGuide.svelte';
  import { rotationReady } from '../integration-guide';

  let { keys, keyUsage = {}, loading, saving, token, endpoint, error: serverError = '', latestKeyID = '', projectOptions = [], oncreate, onrevoke, ondismiss, onstatus }: {
    keys: APIKey[]; loading: boolean; saving: boolean; token: string; endpoint: string; error?: string;
    latestKeyID?: string; projectOptions?: string[];
    keyUsage?: Record<string, KeyUsage>;
    oncreate: (input: ReturnType<typeof keyInput>) => Promise<boolean>;
    // Resolve only after the server confirms revocation; reject on failure.
    onrevoke: (key: APIKey) => Promise<void>;
    ondismiss: () => void; onstatus: (message: string) => void;
  } = $props();
  let dialog = $state<HTMLDialogElement>();
  let mode = $state<'create' | 'details' | 'revoke' | null>(null);
  let selected = $state<APIKey | null>(null);
  let name = $state('');
  let projects = $state('');
  let expiry = $state('');
  let permissions = $state(['read', 'write']);
  let error = $state('');
  let feedback = $state('');
  let endpointFeedback = $state('');
  let closingToken = $state(false);
  let pending = $state(false);
  let rotationOld = $state<APIKey | null>(null);
  let replacementID = $state('');
  let tested = $state(false);
  let errorField = $state<HTMLParagraphElement>();
  let tokenField = $state<HTMLTextAreaElement>();
  let endpointField = $state<HTMLInputElement>();
  let nameField = $state<HTMLInputElement>();
  let modalHeading = $state<HTMLHeadingElement>();
  const busy = $derived(saving || pending);
  const modalError = $derived(error || serverError);

  function keyStatus(key: APIKey) {
    return key.revoked ? 'Dicabut' : key.expires_at && new Date(key.expires_at).getTime() < Date.now() ? 'Kedaluwarsa' : 'Aktif';
  }
  async function open(next: 'create' | 'details' | 'revoke', key: APIKey | null = null, initialName = '') {
    if (busy) return;
    error = ''; feedback = ''; closingToken = false; selected = key; mode = next;
    if (next === 'create') {
      name = key ? `${key.name.slice(0, 90)} pengganti` : initialName;
      projects = key ? (key.projects || []).join(', ') : '';
      permissions = key ? [...(key.permissions || [])] : ['read', 'write'];
      expiry = '';
    }
    await tick();
    if (dialog && !dialog.open) dialog.showModal();
    if (next === 'create') nameField?.focus();
    else modalHeading?.focus();
  }
  // A token retained by the parent must never appear outside the modal.
  $effect(() => {
    if (token && dialog && !dialog.open) {
      mode = 'create';
      dialog.showModal();
      void tick().then(() => tokenField?.focus());
    }
  });
  function close() {
    if (busy) return;
    if (token) ondismiss();
    dialog?.close(); mode = null; selected = null;
    error = ''; feedback = ''; closingToken = false;
  }
  function requestClose() {
    if (busy) return;
    if (token) { closingToken = true; return; }
    close();
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy || token) return;
    error = ''; feedback = '';
    try {
      const input = keyInput(name, permissions, projects, expiry);
      pending = true;
      if (await oncreate(input)) {
        if (selected) { rotationOld = selected; replacementID = latestKeyID; tested = false; }
        name = ''; projects = ''; expiry = ''; permissions = ['read', 'write'];
        await tick(); tokenField?.focus();
      } else {
        error = serverError || 'Kunci belum berhasil dibuat. Periksa pengaturan lalu coba lagi.';
      }
    } catch (e) { error = errorMessage(e); }
    finally { pending = false; if (error) { await tick(); errorField?.focus(); } }
  }
  async function revoke() {
    if (busy || !selected) return;
    pending = true; error = ''; feedback = '';
    try {
      await onrevoke(selected);
      if (serverError) { error = serverError; return; }
      pending = false;
      if (rotationOld?.id === selected.id) { rotationOld = null; replacementID = ''; tested = false; }
      close();
    } catch (e) { error = errorMessage(e); }
    finally { pending = false; }
  }
  async function copyToken() {
    const previous = token;
    error = ''; feedback = '';
    try {
      if (!navigator.clipboard) throw new Error('Clipboard tidak tersedia.');
      await navigator.clipboard.writeText(previous);
      if (token !== previous) return;
      feedback = 'Token disalin. Simpan di tempat aman sebelum menutup dialog.';
    } catch {
      if (token !== previous) return;
      tokenField?.focus(); tokenField?.select();
      error = 'Token tidak dapat disalin otomatis. Pilih teks lalu gunakan Ctrl+C, Command+C, atau menu Salin perangkat.';
    }
  }
  async function copyEndpoint() {
    if (!endpoint) return;
    try {
      if (!navigator.clipboard) throw new Error('Clipboard tidak tersedia.');
      await navigator.clipboard.writeText(endpoint);
      endpointFeedback = 'Endpoint disalin.';
      onstatus(endpointFeedback);
    } catch {
      endpointField?.focus(); endpointField?.select();
      endpointFeedback = 'Salin otomatis tidak tersedia. Teks endpoint dipilih; gunakan Ctrl+C, Command+C, atau menu Salin perangkat.';
    }
  }
</script>

<div class="mcp-layout">
  <section class="panel endpoint-card" aria-labelledby="endpoint-heading">
    <div class="section-heading"><div class="heading-with-icon"><Icon name="server" /><h2 id="endpoint-heading">Endpoint MCP</h2></div><span class="badge">Streamable HTTP</span></div>
    <label for="mcp-endpoint">URL server</label>
    <div class="endpoint-field"><input id="mcp-endpoint" bind:this={endpointField!} value={endpoint} readonly spellcheck="false" aria-describedby="endpoint-hint" placeholder="Endpoint belum tersedia"><button type="button" class="secondary" disabled={!endpoint} onclick={copyEndpoint}><Icon name="copy" /><span>Salin endpoint</span></button></div>
    <p id="endpoint-hint" class="hint">Gunakan transport Streamable HTTP di klien MCP dan header Authorization: Bearer &lt;token&gt;. Endpoint bukan halaman web.</p>
    {#if endpointFeedback}<p class="hint" role="status">{endpointFeedback}</p>{/if}
  </section>

  <IntegrationGuide {endpoint} {busy} oncreate={initialName => { void open('create', null, initialName); }} />

  {#if rotationOld && replacementID}
    <section class="panel" aria-labelledby="rotation-heading"><h2 id="rotation-heading">Rotasi key: {rotationOld.name}</h2>
      <p>Key pengganti sudah dibuat (ID <code>{replacementID}</code>). Key lama tetap berlaku. Perbarui setiap klien yang memakai key lama lalu uji discovery, baca/tulis sesuai izin, dan penolakan scope lain.</p>
      <label class="check-label"><input type="checkbox" bind:checked={tested}> Saya sudah menguji key pengganti pada semua klien yang akan dipindahkan.</label>
      <p class="hint">Ini konfirmasi operator, bukan deteksi otomatis. Rencana rotasi hanya tersimpan selama halaman ini terbuka.</p>
      <div class="actions"><button type="button" class="danger" disabled={busy || token !== '' || !rotationReady(rotationOld.id, replacementID, tested, rotationOld.revoked || !!keys.find(k => k.id === rotationOld?.id)?.revoked) || !keys.some(k => k.id === replacementID && keyStatus(k) === 'Aktif')} onclick={() => open('revoke', rotationOld)}>Tinjau pencabutan key lama</button><button type="button" class="secondary" disabled={busy} onclick={() => { rotationOld = null; replacementID = ''; tested = false; }}>Tutup panduan tanpa mencabut</button></div>
    </section>
  {/if}

  <section class="panel key-manager" aria-labelledby="keys-heading">
    <div class="section-heading"><h2 id="keys-heading">Kunci akses</h2><button type="button" class="primary" disabled={busy} onclick={() => open('create')}><Icon name="plus" /><span>Buat kunci</span></button></div>
    <div class="key-grid" aria-busy={loading}>
      {#if loading}<p class="empty-state">Memuat kunci akses…</p>
      {:else}{#each keys as key (key.id)}
        <article class="key-card">
          <div class="section-heading"><div class="heading-with-icon"><Icon name="key" /><h3>{key.name}</h3></div><span class="badge" class:revoked={key.revoked}>{keyStatus(key)}</span></div>
          <code class="key-prefix">{key.prefix}…</code>
          <p class="muted">Proyek: {(key.projects || []).join(', ') || '—'}</p>
          <div class="permission-list" aria-label="Izin akses">{#each key.permissions || [] as permission}<span class="badge">{permission}</span>{/each}</div>
          <p class="hint">Dibuat {date(key.created_at)}</p>
          <p class="hint">Identitas audit: <code>key:{key.id}</code></p>
          <p class="hint">{keyUsage[key.id] ? `Operasi terakhir tercatat: ${date(keyUsage[key.id].last_recorded_at)} · ${keyUsage[key.id].action}` : 'Tidak ditemukan operasi dalam 200 event audit terbaru.'} Bukan status online.</p>
          <div class="key-card-actions"><button type="button" class="secondary" disabled={busy} onclick={() => open('details', key)} aria-label={`Lihat detail kunci ${key.name}`}>Lihat detail<Icon name="chevron" /></button>{#if !key.revoked}<button type="button" class="icon-button danger" disabled={busy} onclick={() => open('revoke', key)} aria-label={`Cabut akses kunci ${key.name}`} title="Cabut akses"><Icon name="trash" /></button>{/if}</div>
        </article>
      {:else}<div class="empty-state"><Icon name="key" /><h3>Belum ada kunci akses</h3><p>Buat kunci pertama untuk menghubungkan klien MCP Anda.</p></div>{/each}{/if}
    </div>
  </section>
</div>

<dialog class="key-dialog" bind:this={dialog!} aria-labelledby="key-modal-heading" oncancel={event => { event.preventDefault(); requestClose(); }}>
  <div class="modal-content" aria-busy={busy}>
    <div class="modal-header"><h2 id="key-modal-heading" tabindex="-1" bind:this={modalHeading!}>{token ? 'Simpan token rahasia' : mode === 'create' ? selected ? 'Buat kunci pengganti' : 'Buat kunci akses' : mode === 'revoke' ? 'Cabut akses kunci?' : 'Detail kunci akses'}</h2><button type="button" class="icon-button" aria-label="Tutup dialog" title="Tutup dialog" disabled={busy} onclick={requestClose}><Icon name="close" /></button></div>
    {#if modalError}<p class="inline-error" role="alert" tabindex="-1" bind:this={errorField!}>{modalError}</p>{/if}
    {#if feedback}<p class="hint" role="status">{feedback}</p>{/if}
    {#if token}
      <div class="secret-panel"><p>Token ini hanya ditampilkan sekali. Simpan di tempat aman; token tidak dapat dilihat kembali setelah dialog ditutup.</p><label for="new-token">Token rahasia</label><textarea id="new-token" bind:this={tokenField!} value={token} readonly rows="3" spellcheck="false" autocomplete="off"></textarea><button type="button" class="primary" disabled={busy} onclick={copyToken}><Icon name="copy" /><span>Salin token</span></button></div>
      {#if closingToken}<div class="token-confirmation" role="group" aria-labelledby="token-close-heading"><h3 id="token-close-heading">Sudah menyimpan token?</h3><p>Menutup dialog akan menghilangkan token dari tampilan. Pastikan token sudah disimpan dengan aman.</p><div class="modal-actions"><button type="button" class="secondary" disabled={busy} onclick={() => { closingToken = false; }}>Kembali ke token</button><button type="button" class="primary" disabled={busy} onclick={close}>Sudah disimpan, tutup</button></div></div>
      {:else}<div class="modal-actions"><button type="button" class="secondary" onclick={requestClose}>Tutup setelah menyimpan</button></div>{/if}
    {:else if mode === 'create'}
      {#if selected}<p class="hint">Izin dan proyek disalin dari “{selected.name}”. Kunci asli tetap berlaku dan tidak dicabut otomatis.</p>{/if}
      <form class="modal-form" onsubmit={submit}>
        <fieldset disabled={busy} class="form-fields">
          <label for="key-name">Nama kunci</label><input id="key-name" bind:this={nameField!} bind:value={name} required maxlength="100" placeholder="Contoh: Asisten proyek" autocomplete="off">
          <label for="key-projects">Proyek yang diizinkan</label><input id="key-projects" bind:value={projects} required placeholder="proyek-a, proyek-b" aria-describedby="projects-hint" autocomplete="off"><p id="projects-hint" class="hint">Slug huruf kecil, angka, dan tanda hubung; pisahkan dengan koma. Gunakan * untuk semua proyek, termasuk proyek baru.</p>
          {#if projectOptions.length}<label for="key-project-choice">Tambahkan dari daftar proyek</label><select id="key-project-choice" value="" onchange={event => { const slug = event.currentTarget.value; if (slug && projects.trim() !== '*') projects = [...new Set([...projects.split(',').map(p => p.trim()).filter(Boolean), slug])].join(', '); event.currentTarget.value = ''; }}><option value="">Pilih proyek…</option>{#each projectOptions as slug}<option value={slug}>{slug}</option>{/each}</select>{/if}
          <fieldset><legend>Izin akses</legend><label class="check-label"><input type="checkbox" bind:group={permissions} value="read"> Baca (read)</label><label class="check-label"><input type="checkbox" bind:group={permissions} value="write"> Tulis (write)</label><label class="check-label"><input type="checkbox" bind:group={permissions} value="archive"> Arsipkan (tanpa hapus permanen)</label></fieldset>
          <label for="key-expiry">Tanggal kedaluwarsa (opsional)</label><input id="key-expiry" type="date" bind:value={expiry} aria-describedby="expiry-hint"><p id="expiry-hint" class="hint">Berlaku sampai akhir tanggal ini, sesuai zona waktu perangkat.</p>
        </fieldset>
        <div class="modal-actions"><button type="button" class="secondary" disabled={busy} onclick={requestClose}>Batal</button><button type="submit" class="primary" disabled={busy}>{busy ? 'Menyimpan…' : 'Buat kunci'}</button></div>
      </form>
    {:else if selected && mode === 'revoke'}
      <p>Cabut kunci <strong>{selected.name}</strong> ({selected.prefix}…)? Semua integrasi yang memakai kunci ini akan kehilangan akses.</p><p class="hint">Tindakan ini tidak dapat dibatalkan. Riwayat kunci tetap disimpan; tidak ada penghapusan permanen.</p><div class="modal-actions"><button type="button" class="secondary" disabled={busy} onclick={requestClose}>Batal</button><button type="button" class="danger" disabled={busy} onclick={revoke}>{busy ? 'Mencabut…' : 'Ya, cabut akses'}</button></div>
    {:else if selected}
      <dl class="key-details"><dt>Nama</dt><dd>{selected.name}</dd><dt>Status</dt><dd>{keyStatus(selected)}</dd><dt>Awalan token</dt><dd><code>{selected.prefix}…</code></dd><dt>Izin</dt><dd>{(selected.permissions || []).join(', ') || '—'}</dd><dt>Proyek</dt><dd>{(selected.projects || []).join(', ') || '—'}</dd><dt>Dibuat</dt><dd>{date(selected.created_at)}</dd><dt>Kedaluwarsa</dt><dd>{selected.expires_at ? date(selected.expires_at) : 'Tidak ditentukan'}</dd></dl>
      <p class="hint">Identitas audit: <code>key:{selected.id}</code>. {keyUsage[selected.id] ? `Operasi terakhir dalam jendela audit: ${date(keyUsage[selected.id].last_recorded_at)} (${keyUsage[selected.id].action}).` : 'Tidak ditemukan operasi dalam 200 event audit terbaru; bukan berarti belum pernah digunakan.'} Identitas aplikasi/perangkat tidak diverifikasi; status aktif bukan berarti online.</p>
      <p class="hint">Token dan izin kunci yang sudah dibuat tidak dapat diubah. Buat pengganti untuk konfigurasi baru; kunci asli tidak dicabut otomatis.</p><div class="modal-actions"><button type="button" class="secondary" disabled={!!rotationOld} onclick={() => open('create', selected)}><Icon name="refresh" /><span>Buat pengganti</span></button>{#if !selected.revoked}<button type="button" class="danger" onclick={() => open('revoke', selected)}>Cabut akses</button>{/if}</div>
    {/if}
  </div>
</dialog>
