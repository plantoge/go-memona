<script lang="ts">
  import { tick } from 'svelte';
  import { projectInput, type Project, type ProjectInput } from '../projects';
  import { errorMessage } from '../api';
  let { entries, knownSlugs, loading, saving, onsave }: {
    entries: Project[]; knownSlugs: string[]; loading: boolean; saving: boolean;
    onsave: (input: ProjectInput) => Promise<void>;
  } = $props();
  let dialog: HTMLDialogElement;
  let errorBox = $state<HTMLParagraphElement>();
  let heading: HTMLHeadingElement;
  let selected = $state<Project | null>(null);
  let slug = $state(''), name = $state(''), description = $state(''), aliases = $state('');
  let archived = $state(false), pending = $state(false), error = $state(''), query = $state(''), showArchived = $state(false);
  const busy = $derived(saving || pending);
  const visible = $derived(entries.filter(p => (showArchived || !p.archived) && `${p.slug} ${p.name} ${p.description} ${p.aliases.join(' ')}`.toLocaleLowerCase().includes(query.toLocaleLowerCase())));
  const unregistered = $derived(knownSlugs.filter(s => !entries.some(p => p.slug === s)));
  async function open(project: Project | null = null, initialSlug = '') {
    selected = project; slug = project?.slug || initialSlug; name = project?.name || ''; description = project?.description || '';
    aliases = (project?.aliases || []).join('\n'); archived = project?.archived || false; error = '';
    dialog.showModal(); await tick(); heading?.focus();
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); if (busy) return; error = '';
    try {
      const input = projectInput(slug, name, description, aliases, archived, selected?.revision || 0);
      if (selected && selected.archived !== archived && !window.confirm(archived ? 'Arsipkan metadata proyek? Catatan dan izin key tidak berubah.' : 'Pulihkan metadata proyek ini?')) return;
      pending = true; await onsave(input); dialog.close();
    } catch (e) { error = errorMessage(e); await tick(); errorBox?.focus(); }
    finally { pending = false; }
  }
</script>

<section aria-labelledby="projects-heading">
  <div class="section-heading"><div><h2 id="projects-heading">Proyek</h2><p class="muted">Satu slug yang stabil untuk beberapa mesin dan worktree.</p></div><button type="button" class="primary" disabled={busy || loading} onclick={() => open()}>Daftarkan proyek</button></div>
  <p class="hint">Registry hanya metadata. Alias tidak memperluas scope key; arsip proyek tidak mengarsipkan catatan atau mencabut key. Tidak ada pemindaian lokasi otomatis.</p>
  <div class="filters"><label>Cari proyek<input bind:value={query} maxlength="200" placeholder="Nama, slug atau alias"></label><label class="check-label"><input type="checkbox" bind:checked={showArchived}> Sertakan proyek diarsipkan</label></div>
  <div class="stack" aria-busy={loading}>
    {#if loading}<p role="status">Memuat proyek…</p>{:else}
      <p class="hint" role="status">{visible.length} dari {entries.length} proyek terdaftar</p>
      {#each visible as project (project.slug)}
        <article class="panel"><div class="section-heading"><h3>{project.name}</h3><span class="badge">{project.archived ? 'Diarsipkan' : 'Aktif'}</span></div><code>{project.slug}</code><p>{project.description || 'Belum ada deskripsi.'}</p>
          {#if project.aliases.length}<p class="muted">Alias: {project.aliases.join(' · ')}</p>{/if}
          <div class="actions"><span class="hint">Revisi {project.revision}</span><button type="button" class="secondary" disabled={busy} onclick={() => open(project)} aria-label={`Edit proyek ${project.name}`}>Edit metadata</button></div>
        </article>
      {:else}<p class="empty-state">Belum ada proyek yang cocok. Catatan dengan slug belum terdaftar tetap dapat dipakai.</p>{/each}
      {#if unregistered.length}<details><summary>Slug dari catatan yang belum terdaftar ({unregistered.length})</summary><div class="actions">{#each unregistered as value}<button type="button" class="secondary" disabled={busy} onclick={() => open(null, value)}>Daftarkan {value}</button>{/each}</div></details>{/if}
    {/if}
  </div>
</section>
<dialog bind:this={dialog!} aria-labelledby="project-editor-heading" oncancel={e => { if (busy) e.preventDefault(); }}>
  <div class="modal-header"><h2 id="project-editor-heading" tabindex="-1" bind:this={heading!}>{selected ? 'Edit metadata proyek' : 'Daftarkan proyek'}</h2><button type="button" class="secondary" disabled={busy} onclick={() => dialog.close()}>Tutup</button></div>
  {#if error}<p class="inline-error" role="alert" tabindex="-1" bind:this={errorBox!}>{error} Isian tetap disimpan. Jika konflik revisi, tutup lalu muat ulang sebelum mengedit kembali.</p>{/if}
  <form onsubmit={submit} aria-busy={busy}><fieldset disabled={busy}>
    <label>Slug (tetap setelah dibuat)<input bind:value={slug} readonly={!!selected} required maxlength="64" pattern={'[a-z0-9][a-z0-9-]{0,63}'} autocomplete="off"></label>
    <label>Nama<input bind:value={name} required maxlength="200"></label>
    <label>Deskripsi<textarea bind:value={description} rows="3" maxlength="2000"></textarea></label>
    <label>Alias / referensi lokasi (satu per baris)<textarea bind:value={aliases} rows="4" maxlength="4020" aria-describedby="alias-help"></textarea></label>
    <p id="alias-help" class="hint">Maksimal 20 referensi informasional. Jangan masukkan credential, URL bertoken, atau lokasi pribadi yang tidak perlu. Lokasi tidak diakses server.</p>
    <label class="check-label"><input type="checkbox" bind:checked={archived}> Arsipkan metadata proyek</label>
    <button type="submit" class="primary">{busy ? 'Menyimpan…' : 'Simpan proyek'}</button>
  </fieldset></form>
</dialog>
<style>
  fieldset { border: 0; margin: 0; padding: 0; display: grid; gap: 1rem; min-width: 0; }
  label { display: grid; gap: .4rem; } .check-label { display: flex; align-items: center; gap: .5rem; }
  input:not([type='checkbox']), textarea { width: 100%; box-sizing: border-box; } textarea { resize: vertical; }
  article { overflow-wrap: anywhere; } dialog { max-width: 42rem; } .actions { flex-wrap: wrap; }
</style>
