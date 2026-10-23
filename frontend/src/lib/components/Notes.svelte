<script lang="ts">
  import { onDestroy } from 'svelte';
  import { kindLabels, statusLabel, date, type Note } from '../types';
  let { notes, projects, loading, projectLoading, skills, onsearch, onopen }: {
    notes: Note[]; projects: string[]; loading: boolean; projectLoading: boolean; skills: boolean;
    onsearch: (query: string, project: string, kind: string) => void;
    onopen: (id: string) => void;
  } = $props();
  let query = $state('');
  let project = $state('');
  let kind = $state('');
  let archive = $state('all');
  let statusFilter = $state('');
  let author = $state('');
  let after = $state('');
  let before = $state('');
  let hideTesting = $state(false);
  let visible = $derived(notes.filter(n =>
    (archive === 'all' || n.archived === (archive === 'archived')) &&
    (!statusFilter || n.status === statusFilter) && (!author || n.author.toLocaleLowerCase().includes(author.toLocaleLowerCase())) &&
    (!after || n.updated_at.slice(0, 10) >= after) && (!before || n.updated_at.slice(0, 10) <= before) &&
    (!hideTesting || !n.is_testing)
  ));
  let timer: ReturnType<typeof setTimeout>;
  function search() { clearTimeout(timer); onsearch(query.trim(), project, skills ? 'skill' : kind); }
  function scheduleSearch() { clearTimeout(timer); timer = setTimeout(search, 300); }
  onDestroy(() => clearTimeout(timer));
</script>

<section aria-label="Daftar memori">
  <div class="stats">
    <article class="stat"><p>Hasil memori</p><strong>{loading ? '—' : notes.length}</strong></article>
    <article class="stat"><p>Skill dalam hasil</p><strong>{loading ? '—' : notes.filter(n => n.kind === 'skill').length}</strong></article>
    <article class="stat"><p>Proyek</p><strong>{projectLoading ? '—' : projects.length}</strong></article>
  </div>
  <form class="filters" role="search" onsubmit={(event) => { event.preventDefault(); search(); }}>
    <div class="search-field"><label for="query">Cari memori</label><input id="query" type="search" bind:value={query} oninput={scheduleSearch} placeholder="Cari judul atau isi memori…" autocomplete="off"></div>
    <div><label for="project-filter">Proyek</label><select id="project-filter" bind:value={project} onchange={search}><option value="">Semua proyek</option>{#each projects as p}<option value={p}>{p}</option>{/each}</select></div>
    <div><label for="kind-filter">Jenis</label><select id="kind-filter" bind:value={kind} disabled={skills} onchange={search}><option value="">{skills ? 'Skill' : 'Semua jenis'}</option>{#each Object.entries(kindLabels) as [value, label]}<option {value}>{label}</option>{/each}</select></div>
    <button type="submit" class="primary">Cari</button>
  </form>
  <details><summary>Filter lanjutan pada hasil yang dimuat</summary>
    <div class="filters">
      <label>Arsip<select bind:value={archive}><option value="all">Semua</option><option value="active">Nonarsip</option><option value="archived">Diarsipkan</option></select></label>
      <label>Status<select bind:value={statusFilter}><option value="">Semua</option><option value="unverified">Belum diverifikasi</option><option value="verified">Terverifikasi</option><option value="draft">Draft</option><option value="active">Skill aktif</option></select></label>
      <label>Penulis<input bind:value={author} maxlength="200" placeholder="Nama/identitas penulis"></label>
      <label>Diperbarui mulai<input type="date" bind:value={after}></label>
      <label>Sampai<input type="date" bind:value={before}></label>
      <label><input type="checkbox" bind:checked={hideTesting}> Sembunyikan catatan berlabel testing</label>
    </div>
    <p class="hint">Filter ini hanya memeriksa maksimal 500 hasil yang dimuat. Label testing eksplisit dari penulis; catatan lama tidak diklasifikasikan otomatis.</p>
  </details>
  <div class="section-heading"><h2>{skills ? 'Skill tersimpan' : 'Memori tersimpan'}</h2><span class="muted">{visible.length} dari {notes.length} hasil dimuat · maks. 500</span></div>
  <div class="notes-grid" aria-busy={loading}>
    {#if loading}<p class="empty-state">Memuat memori…</p>
    {:else if !visible.length}<p class="empty-state">Belum ada memori yang cocok. Coba kata kunci atau filter lain.</p>
    {:else}{#each visible as note (note.id)}
      <article class="note-card">
        <div class="badges"><span class="badge">{note.project}</span><span class="badge neutral">{kindLabels[note.kind] || note.kind}</span>{#if note.is_project_summary}<span class="badge">Ringkasan proyek</span>{/if}{#if note.connections?.superseded_by.length}<span class="badge neutral">Sudah digantikan</span>{/if}{#if note.archived}<span class="badge neutral">Diarsipkan</span>{/if}{#if note.is_testing}<span class="badge neutral">Testing</span>{/if}</div>
        <h3><button type="button" class="note-link" onclick={() => onopen(note.id)}>{note.title || 'Tanpa judul'}</button></h3>
        <p class="note-excerpt">{note.content.slice(0, 180)}</p>
        <p class="card-meta">{statusLabel(note.status)} · Revisi {note.revision} · {date(note.updated_at)}</p>
      </article>
    {/each}{/if}
  </div>
</section>
