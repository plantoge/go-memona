<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { kindLabels, statusLabel, type Note, type DuplicateCandidate } from '../types';
  import { initialNoteInput, noteEditorErrors, noteEditorInput, type NoteInput, type NoteEditorErrors } from '../note-editor';

  let { note = null, project = '', saving = false, onsubmit, oncancel, onduplicates }: {
    note?: Note | null; project?: string; saving?: boolean;
    onsubmit: (input: NoteInput) => Promise<boolean>; oncancel: () => void;
    onduplicates?: (input: NoteInput, excludeID: string) => Promise<DuplicateCandidate[]>;
  } = $props();
  const uid = $props.id();
  // A form is a snapshot of the revision being edited, never reset by a failed save.
  let input = $state(untrack(() => initialNoteInput(note, project)));
  let compatibilityText = $state(untrack(() => (note?.skill_metadata?.compatibility || []).join(', ')));
  function syncCompatibility() { if (input.skill_metadata) input.skill_metadata.compatibility = compatibilityText.split(',').map(s => s.trim()).filter(Boolean); }
  let errors = $state<NoteEditorErrors>({});
  let failure = $state('');
  let pending = $state(false);
  let checking = $state(false), checkedText = $state('');
  let candidates = $state<DuplicateCandidate[]>([]);
  const currentText = $derived(JSON.stringify([input.project, input.title, input.content]));
  async function checkDuplicates() {
    if (!onduplicates || checking) return;
    errors = noteEditorErrors(input); failure = '';
    if (Object.keys(errors).length) { await tick(); errorBox?.focus(); return; }
    const snapshot = currentText;
    checking = true; candidates = []; checkedText = '';
    try { const result = await onduplicates(noteEditorInput(input), note?.id || ''); if (snapshot === currentText) { candidates = result; checkedText = snapshot; } }
    catch { failure = 'Pemeriksaan kemiripan gagal. Tidak ada perubahan disimpan; Anda tetap dapat menyimpan tanpa pemeriksaan.'; }
    finally { checking = false; }
  }
  let errorBox = $state<HTMLDivElement>();
  let editorHeading: HTMLHeadingElement;
  onMount(() => { editorHeading?.focus(); });
  const busy = $derived(saving || pending);
  const fields: ('project' | 'kind' | 'title' | 'content' | 'source' | 'status')[] = ['project', 'kind', 'title', 'content', 'source', 'status'];
  const labels: Record<keyof NoteInput, string> = { project: 'Proyek', kind: 'Jenis', title: 'Judul', content: 'Isi Markdown', source: 'Sumber / bukti', status: 'Status', is_testing: 'Catatan testing', skill_metadata: 'Metadata skill' };
  const fieldID = (field: string) => `${uid}-${field}`;
  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (busy) return;
    errors = noteEditorErrors(input); failure = '';
    if (Object.keys(errors).length) { await tick(); errorBox?.focus(); return; }
    pending = true;
    try {
      if (!await onsubmit(noteEditorInput(input))) failure = 'Catatan belum disimpan. Periksa pesan server; isian Anda tetap tersedia. Jika revisi berubah, muat ulang sebelum mencoba kembali.';
    } catch { failure = 'Penyimpanan gagal. Isian Anda tetap tersedia; periksa keadaan catatan sebelum mencoba kembali.'; }
    finally { pending = false; }
    if (failure) { await tick(); errorBox?.focus(); }
  }
</script>

<form class="note-editor" onsubmit={save} aria-busy={busy}>
  <h3 tabindex="-1" bind:this={editorHeading!}>{note ? 'Edit isi memori' : 'Buat memori'}</h3>
  <p class="hint" id={`${uid}-help`}>{note ? 'Proyek dan jenis tidak dapat diubah. Ringkasan dan relasi dipertahankan; setiap simpan menambah revisi.' : 'Isi disimpan sebagai Markdown. Jangan masukkan secret atau data pribadi.'}</p>
  {#if note?.kind === 'skill' && note.status === 'active'}<p class="hint">Mengedit skill yang dipublikasikan akan mengembalikannya menjadi draf untuk ditinjau kembali.</p>{/if}
  {#if Object.keys(errors).length || failure}
    <div class="inline-error" role="alert" tabindex="-1" bind:this={errorBox!}>
      <strong>Catatan belum disimpan.</strong>
      {#if failure}<p>{failure}</p>{/if}
      <ul>{#each fields.filter(field => errors[field]) as field}<li><a href={`#${fieldID(field)}`}>{labels[field]}: {errors[field]}</a></li>{/each}{#if errors.skill_metadata}<li>Metadata skill: {errors.skill_metadata}</li>{/if}</ul>
    </div>
  {/if}
  <fieldset disabled={busy}>
    {#each fields as field}
      <div class="editor-field">
        <label for={fieldID(field)}>{labels[field]}{field === 'project' || field === 'kind' || field === 'title' || field === 'status' ? ' (wajib)' : ''}</label>
        {#if field === 'kind'}
          <select id={fieldID(field)} bind:value={input.kind} disabled={!!note} aria-invalid={errors[field] ? true : undefined} aria-describedby={errors[field] ? `${fieldID(field)}-error` : undefined} onchange={() => { input.status = input.kind === 'skill' ? 'draft' : 'unverified'; if (input.kind !== 'skill') delete input.skill_metadata; }}>
            {#each Object.entries(kindLabels) as [value, label]}<option {value}>{label}</option>{/each}
          </select>
        {:else if field === 'status'}
          <select id={fieldID(field)} bind:value={input.status} aria-invalid={errors[field] ? true : undefined} aria-describedby={errors[field] ? `${fieldID(field)}-error` : undefined}>
            {#each input.kind === 'skill' ? ['draft'] : ['unverified', 'verified'] as status}<option value={status}>{statusLabel(status)}</option>{/each}
          </select>
        {:else if field === 'content'}
          <textarea id={fieldID(field)} bind:value={input.content} rows="12" spellcheck="false" aria-invalid={errors[field] ? true : undefined} aria-describedby={errors[field] ? `${fieldID(field)}-error` : `${uid}-help`}></textarea>
        {:else if field === 'source'}
          <textarea id={fieldID(field)} bind:value={input.source} rows="3" aria-invalid={errors[field] ? true : undefined} aria-describedby={errors[field] ? `${fieldID(field)}-error` : undefined}></textarea>
        {:else}
          <input id={fieldID(field)} bind:value={input[field]} readonly={field === 'project' && !!note} aria-invalid={errors[field] ? true : undefined} aria-describedby={errors[field] ? `${fieldID(field)}-error` : undefined}>
        {/if}
        {#if errors[field]}<p id={`${fieldID(field)}-error`} class="inline-error">{errors[field]}</p>{/if}
      </div>
    {/each}
    {#if input.kind === 'skill'}<section aria-label="Metadata review skill"><h4>Metadata skill (opsional)</h4>
      <button type="button" class="secondary" onclick={() => { input.skill_metadata = { trigger: '', version: '', compatibility: [], test_evidence: '' }; compatibilityText = ''; }}>Isi / kosongkan metadata skill</button>
      {#if input.skill_metadata}
        <label>Pemicu penggunaan<input bind:value={input.skill_metadata.trigger} maxlength="1000"></label>
        <label>Versi<input bind:value={input.skill_metadata.version} maxlength="100"></label>
        <label>Kompatibilitas harness (pisahkan koma)<input value={compatibilityText} oninput={event => { compatibilityText = event.currentTarget.value; syncCompatibility(); }}></label>
        <label>Bukti pengujian / referensi<input bind:value={input.skill_metadata.test_evidence} maxlength="2000"></label>
      {/if}
      {#if errors.skill_metadata}<p class="inline-error" role="alert">{errors.skill_metadata}</p>{/if}
      <p class="hint">Metadata adalah pernyataan penulis, bukan hasil uji otomatis. Mengedit skill aktif selalu mengembalikannya ke draf.</p>
    </section>{/if}
    <label class="testing-check"><input type="checkbox" checked={!!input.is_testing} onchange={e => input.is_testing = e.currentTarget.checked}> Catatan testing / sintetis (dikecualikan dari context default)</label>
    <p class="hint">Terverifikasi memerlukan bukti pada sumber; ini bukan verifikasi otomatis oleh Memona.</p>
    {#if onduplicates}<section aria-label="Saran catatan serupa"><button type="button" class="secondary" disabled={checking} onclick={checkDuplicates}>{checking ? 'Memeriksa…' : 'Periksa catatan serupa'}</button>
      {#if checkedText === currentText}<p class="hint" role="status">{candidates.length ? `${candidates.length} kandidat serupa. Tinjau ID/revisi sebelum memilih memperbarui catatan lama. Simpan tetap membuat catatan/revisi sesuai editor ini.` : 'Tidak ditemukan kandidat di atas ambang kemiripan; bukan jaminan tidak ada duplikat.'}</p><ul>{#each candidates as candidate}<li><strong>{candidate.title}</strong> · revisi {candidate.revision} · skor leksikal {Math.round(candidate.score * 100)}%<p class="hint">{candidate.id} — {candidate.reason}</p></li>{/each}</ul>{/if}
      <p class="hint">Pemeriksaan opsional, hanya proyek yang sama; tidak menggabungkan atau menghapus catatan.</p></section>{/if}
    <div class="actions"><button type="submit" class="primary">{busy ? 'Menyimpan…' : note ? 'Simpan revisi isi' : 'Buat memori'}</button><button type="button" class="secondary" onclick={oncancel}>Batal</button></div>
  </fieldset>
</form>

<style>
  .note-editor { display: grid; gap: 1rem; min-width: 0; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; display: grid; gap: 1rem; }
  .editor-field { display: grid; gap: .4rem; min-width: 0; }
  textarea, input, select { width: 100%; box-sizing: border-box; }
  textarea { resize: vertical; }
  textarea[spellcheck='false'] { font-family: monospace; }
  input[readonly] { opacity: .8; }
  .testing-check { display: flex; align-items: center; gap: .5rem; }
  .testing-check input { width: auto; }
</style>
