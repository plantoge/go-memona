<script lang="ts">
  import { onMount } from 'svelte';
  import { date, kindLabels, statusLabel, type NoteDetail, type ConnectionInput } from '../types';
  import { connectionInput } from '../connections';
  import NoteEditor from './NoteEditor.svelte';
  import { compareNoteLines, type NoteInput } from '../note-editor';
  import { compareRevisionMetadata } from '../revision-metadata';
  let { detail, loading, saving, error, onclose, onchange, onopen, onsaveconnections, onsavecontent = undefined, onduplicates, onhistory, onrestoreskill }: {
    detail: NoteDetail | null; loading: boolean; saving: boolean; error: string;
    onclose: () => void; onchange: (action: 'archive' | 'restore' | 'publish') => void;
    onopen: (id: string) => void; onsaveconnections: (input: ConnectionInput) => Promise<boolean>;
    onsavecontent?: (input: NoteInput) => Promise<boolean>;
    onduplicates?: (input: NoteInput, excludeID: string) => Promise<import('../types').DuplicateCandidate[]>;
    onhistory?: (before: number) => Promise<void>;
    onrestoreskill?: (revision: number) => Promise<void>;
  } = $props();
  let dialog: HTMLDialogElement;
  let heading: HTMLHeadingElement;
  let errorBox = $state<HTMLParagraphElement>();
  let formErrorBox = $state<HTMLParagraphElement>();
  let editing = $state(false), summaryFlag = $state(false), related = $state(''), replaces = $state(''), formError = $state('');
  let editorID = $state('');
  let editingContent = $state(false);
  let fromRevision = $state(0), toRevision = $state(0);
  const comparison = $derived.by(() => {
    const before = detail?.history?.find(note => note.revision === Number(fromRevision));
    const after = detail?.history?.find(note => note.revision === Number(toRevision));
    return before && after ? { ...compareNoteLines(before.content, after.content), metadata: compareRevisionMetadata(before, after) } : null;
  });
  $effect(() => {
    if (detail?.note && editorID !== detail.note.id) {
      editorID = detail.note.id; editing = false; editingContent = false;
      toRevision = detail.note.revision;
      fromRevision = Math.max(1, detail.note.revision - 1);
    }
  });
  async function saveContent(input: NoteInput): Promise<boolean> {
    if (!onsavecontent) return false;
    const saved = await onsavecontent(input);
    if (saved) editingContent = false;
    return saved;
  }
  $effect(() => { if (detail && !loading) heading?.focus(); });
  $effect(() => { if (formError) formErrorBox?.focus(); });
  $effect(() => { if (error) errorBox?.focus(); });
  function editConnections() {
    if (!detail) return;
    summaryFlag = !!detail.note.is_project_summary;
    related = (detail.note.related_to || []).join('\n'); replaces = (detail.note.supersedes || []).join('\n');
    formError = ''; editing = true;
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (!detail || saving) return;
    formError = '';
    try {
      const input = connectionInput(summaryFlag, related, replaces, detail.note.id);
      if (input.supersedes.length && !window.confirm('Relasi menggantikan akan mengecualikan catatan target dari get_context. Lanjutkan?')) return;
      if (await onsaveconnections(input)) editing = false;
    } catch (e) { formError = e instanceof Error ? e.message : 'Relasi tidak valid.'; }
  }
  onMount(() => { dialog.showModal(); });
</script>

<dialog bind:this={dialog!} aria-labelledby="detail-title" onclose={onclose} oncancel={(event) => { if (saving) event.preventDefault(); }}>
  <div class="dialog-top"><span class="eyebrow">DETAIL MEMORI</span><button type="button" class="secondary" disabled={saving} onclick={() => dialog.close()}>Tutup</button></div>
  <h2 id="detail-title" tabindex="-1" bind:this={heading!}>{detail?.note.title || 'Detail memori'}</h2>
  <p id="detail-status" role="status">{loading ? 'Memuat detail…' : saving ? 'Menyimpan perubahan…' : detail ? `Revisi ${detail.note.revision} dimuat.` : 'Detail gagal dimuat.'}</p>
  {#if error}<p id="detail-error" role="alert" tabindex="-1" bind:this={errorBox}>{error}</p>{/if}
  {#if detail && !loading}
    {@const note = detail.note}
    {#if note.is_project_summary}<p class="badge">Ringkasan proyek · diprioritaskan dalam konteks</p>{/if}
    {#if note.is_testing}<p class="badge">Testing / sintetis · tidak dimasukkan ke context default</p>{/if}
    {#if note.connections?.superseded_by.length}<p class="inline-error" role="status">Catatan ini sudah digantikan. Lihat penggantinya di bawah; catatan ini tidak dimasukkan ke get_context.</p>{/if}
    <dl class="metadata">
      {#each Object.entries({ ID: note.id, Proyek: note.project, Jenis: kindLabels[note.kind] || note.kind, [note.kind === 'skill' ? 'Publikasi skill' : 'Verifikasi']: statusLabel(note.status), Pengarsipan: note.archived ? 'Diarsipkan' : 'Tidak diarsipkan', Sumber: note.source, Penulis: note.author, Revisi: note.revision, Dibuat: date(note.created_at), Diperbarui: date(note.updated_at) }) as [label, value]}
        <div><dt>{label}</dt><dd>{value || '—'}</dd></div>
      {/each}
    </dl>
    {#if note.status === 'verified'}<p class="hint">Terverifikasi berarti sumber dicatat oleh penulis; bukan jaminan pemeriksaan otomatis oleh Memona.</p>{/if}
    <div class="actions">
      {#if onsavecontent && !note.archived && !editingContent}<button type="button" class="secondary" disabled={saving || editing} onclick={() => { editingContent = true; }}>Edit isi memori</button>{/if}
      {#if !note.archived && note.kind === 'skill' && note.status === 'draft'}<button type="button" class="primary" disabled={saving} onclick={() => onchange('publish')}>Terbitkan sebagai skill</button>{/if}
      {#if note.archived}<button type="button" class="secondary" disabled={saving} onclick={() => onchange('restore')}>Pulihkan dari arsip</button>
      {:else}<button type="button" class="danger" disabled={saving} onclick={() => onchange('archive')}>Arsipkan memori</button>{/if}
    </div>
    {#if editingContent && onsavecontent}
      {#key note.id}<NoteEditor {note} {saving} {onduplicates} onsubmit={saveContent} oncancel={() => { editingContent = false; }} />{/key}
    {/if}
    {#if note.skill_metadata}<section aria-label="Metadata skill"><h3>Metadata skill</h3><dl class="metadata">{#each Object.entries({ Pemicu: note.skill_metadata.trigger, Versi: note.skill_metadata.version, Kompatibilitas: (note.skill_metadata.compatibility || []).join(', '), 'Bukti pengujian': note.skill_metadata.test_evidence }) as [label, value]}<div><dt>{label}</dt><dd>{value || '—'}</dd></div>{/each}</dl><p class="hint">Pernyataan penulis, bukan pemeriksaan otomatis Memona.</p></section>{/if}
    <article class="prose" aria-label="Isi memori">
      <!-- Only Go goldmark's safe-mode Markdown response from our authenticated same-origin API. Never raw note.content. -->
      {#if detail.html.trim()}{@html detail.html}{:else}<p>Memori ini belum memiliki isi.</p>{/if}
    </article>
    <section class="note-connections" aria-labelledby="connections-title">
      <div class="section-heading"><h3 id="connections-title">Hubungan catatan</h3>{#if !note.archived && !editing}<button type="button" class="secondary" disabled={saving || editingContent} onclick={editConnections}>Atur ringkasan & relasi</button>{/if}</div>
      {#if note.connections}
        {#each [{ label: 'Terkait dengan', items: note.connections.related_to }, { label: 'Menggantikan', items: note.connections.supersedes }, { label: 'Dirujuk oleh', items: note.connections.referenced_by }, { label: 'Digantikan oleh', items: note.connections.superseded_by }] as group}
          <h4>{group.label}</h4>
          <ul class="relation-list">{#each group.items as ref (ref.id)}<li><button type="button" class="note-link" disabled={saving} onclick={() => onopen(ref.id)}>{ref.title}{ref.archived ? ' (diarsipkan)' : ''}</button><code>{ref.id}</code></li>{:else}<li class="muted">Tidak ada.</li>{/each}</ul>
        {/each}
        {#if note.connections.truncated}<p class="hint">Maksimal 20 referensi per jenis ditampilkan. Hasil terpotong.</p>{/if}
      {/if}
      {#if editing}
        <form class="connection-form" onsubmit={save} aria-busy={saving}>
          <p id="relation-help" class="hint">ID catatan dalam proyek {note.project}, dipisahkan baris/koma. Maks. 20 per jenis. Mengosongkan daftar akan menghapus relasi pada revisi baru, bukan menghapus catatan.</p>
          {#if formError}<p id="relation-error" role="alert" class="inline-error" tabindex="-1" bind:this={formErrorBox}>{formError} <a href="#related-ids">Periksa terkait</a> · <a href="#supersedes-ids">Periksa menggantikan</a></p>{/if}
          <label class="checkbox-row"><input type="checkbox" bind:checked={summaryFlag} disabled={saving || note.kind !== 'knowledge'}> Ringkasan proyek</label>
          <p class="hint">Hanya satu ringkasan nonarsip per proyek; jenis pengetahuan, isi maks. 8000 byte. Ubah isi melalui editor atau MCP. Perubahan metadata skill aktif mengembalikannya ke draf.</p>
          <label for="related-ids">Terkait dengan (ID)</label><textarea id="related-ids" bind:value={related} disabled={saving} rows="3" aria-describedby={formError ? 'relation-help relation-error' : 'relation-help'} aria-invalid={formError ? true : undefined}></textarea>
          <label for="supersedes-ids">Menggantikan (ID)</label><textarea id="supersedes-ids" bind:value={replaces} disabled={saving} rows="3" aria-describedby={formError ? 'relation-help relation-error' : 'relation-help'} aria-invalid={formError ? true : undefined}></textarea>
          <div class="actions"><button type="submit" class="primary" disabled={saving}>{saving ? 'Menyimpan…' : 'Simpan revisi metadata'}</button><button type="button" class="secondary" disabled={saving} onclick={() => { editing = false; }}>Batal</button></div>
        </form>
      {/if}
    </section>
    <details><summary>Bandingkan isi & metadata revisi</summary>
      <p class="hint">Perbandingan revisi pada halaman yang dimuat, berdasarkan posisi baris. Maksimal 200 baris / 20000 karakter per sisi. Pilih ulang revisi setelah berpindah halaman.</p>
      <div class="revision-selectors">
        <label>Dari revisi <select bind:value={fromRevision}>{#each detail.history || [] as revision}<option value={revision.revision}>Revisi {revision.revision}</option>{/each}</select></label>
        <label>Ke revisi <select bind:value={toRevision}>{#each detail.history || [] as revision}<option value={revision.revision}>Revisi {revision.revision}</option>{/each}</select></label>
      </div>
      {#if comparison}
        <h4>Perubahan metadata</h4>
        <p class="hint">Perbedaan deklarasi penulis, bukan keputusan otomatis tentang kebenaran atau keamanan skill. Maks. 2000 karakter per nilai.</p>
        {#each comparison.metadata as change (change.label)}<article class="history-item"><h4>{change.label}</h4><dl class="metadata"><div><dt>Dari revisi {fromRevision}</dt><dd>{change.before || '—'}</dd></div><div><dt>Ke revisi {toRevision}</dt><dd>{change.after || '—'}</dd></div></dl>{#if change.truncated}<p class="hint">Nilai terpotong; periksa detail revisi.</p>{/if}</article>{:else}<p class="hint">Tidak ada perubahan metadata yang dibandingkan.</p>{/each}
        <h4>Perubahan isi</h4>
        {#if comparison.truncated}<p class="hint" role="status">Perbandingan terpotong. Lihat riwayat revisi untuk isi lengkap.</p>{/if}
        <!-- Keyboard focus enables arrow-key scrolling of this bounded text region. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div class="revision-comparison" role="region" aria-label="Perbandingan isi revisi" tabindex="0">
          <table><thead><tr><th scope="col">Baris</th><th scope="col">Dari revisi {fromRevision}</th><th scope="col">Ke revisi {toRevision}</th></tr></thead><tbody>
            {#each comparison.lines as line}<tr class:changed={line.changed}><th scope="row">{line.number}{#if line.changed}<span class="change-label"> · Berubah</span>{/if}</th><td><pre>{line.before ?? '—'}</pre></td><td><pre>{line.after ?? '—'}</pre></td></tr>{/each}
          </tbody></table>
        </div>
      {:else}<p class="empty-state">Pilih dua revisi untuk dibandingkan.</p>{/if}
    </details>
    <details><summary>Riwayat revisi</summary><div class="stack">
      {#if onhistory}<div class="actions"><button type="button" class="secondary" disabled={saving} onclick={() => onhistory?.(0)}>Revisi terbaru</button><button type="button" class="secondary" disabled={saving || !detail.history_next_before} onclick={() => onhistory?.(detail!.history_next_before!)}>Revisi lebih lama</button></div><p class="hint">Maksimal 20 revisi per halaman; halaman sebelumnya diganti, bukan dikumpulkan.</p>{/if}
      {#each detail.history || [] as revision (revision.revision)}
        {#if onrestoreskill && note.kind === 'skill' && !note.archived && revision.revision !== note.revision}<button type="button" class="secondary" disabled={saving} onclick={() => onrestoreskill?.(revision.revision)}>Salin isi revisi {revision.revision} ke draf baru</button>{/if}
        <article class="history-item"><h3>Revisi {revision.revision} · {revision.title}</h3><p class="muted">{revision.author} · {date(revision.updated_at)}</p><p class="hint">Status: {statusLabel(revision.status)} · Testing: {revision.is_testing ? 'Ya' : 'Tidak'} · Ringkasan: {revision.is_project_summary ? 'Ya' : 'Tidak'} · Terkait: {(revision.related_to || []).join(', ') || '—'} · Menggantikan: {(revision.supersedes || []).join(', ') || '—'}</p><pre class="history-content">{revision.content || '(Tanpa isi)'}</pre></article>
      {:else}<p class="empty-state">Belum ada riwayat revisi.</p>{/each}
    </div></details>
  {/if}
</dialog>

<style>
  .revision-selectors { display: flex; flex-wrap: wrap; gap: 1rem; margin: 1rem 0; }
  .revision-selectors label { display: grid; gap: .4rem; }
  .revision-comparison { max-height: 32rem; overflow: auto; }
  table { width: 100%; border-collapse: collapse; table-layout: fixed; }
  th, td { text-align: left; vertical-align: top; padding: .5rem; border: 1px solid var(--line); }
  th:first-child { width: 6rem; }
  td pre { white-space: pre-wrap; overflow-wrap: anywhere; margin: 0; }
  .changed td { border-inline-start-width: 3px; }
  .change-label { font-size: .8rem; }
</style>
