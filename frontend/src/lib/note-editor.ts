import type { Note, SkillMetadata } from './types';

/** Only replacement content fields: omitted metadata is preserved by Revise. */
export interface NoteInput {
  project: string; kind: string; title: string; content: string; source: string; status: string;
  is_testing?: boolean;
  skill_metadata?: SkillMetadata;
}
export type NoteEditorErrors = Partial<Record<keyof NoteInput, string>>;
const bytes = (value: string) => new TextEncoder().encode(value).length;
const controls = /\p{Cc}/u;

export function initialNoteInput(note?: Note | null, project = ''): NoteInput {
  return {
    project: note?.project ?? project, kind: note?.kind ?? 'knowledge',
    title: note?.title ?? '', content: note?.content ?? '', source: note?.source ?? '',
    status: note?.kind === 'skill' ? 'draft' : note?.status ?? 'unverified',
    ...(note?.is_testing ? { is_testing: true } : {}),
    ...(note?.skill_metadata ? { skill_metadata: { trigger: note.skill_metadata.trigger || '', version: note.skill_metadata.version || '', test_evidence: note.skill_metadata.test_evidence || '', compatibility: [...(note.skill_metadata.compatibility || [])] } } : {}),
  };
}

export function noteEditorErrors(input: NoteInput): NoteEditorErrors {
  const errors: NoteEditorErrors = {};
  if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(input.project)) errors.project = 'Gunakan slug proyek 1–64 karakter: huruf kecil, angka, atau tanda hubung.';
  if (!['fact', 'decision', 'knowledge', 'handoff', 'skill'].includes(input.kind)) errors.kind = 'Pilih jenis catatan yang tersedia.';
  if (!input.title.trim() || bytes(input.title) > 200 || controls.test(input.title)) errors.title = 'Judul wajib diisi, maksimal 200 byte, tanpa karakter kontrol.';
  if (bytes(input.content) > 65536 || input.content.includes('\0')) errors.content = 'Isi maksimal 65536 byte dan tidak boleh memuat karakter NUL.';
  if (bytes(input.source) > 2000 || controls.test(input.source)) errors.source = 'Sumber maksimal 2000 byte, tanpa karakter kontrol.';
  if (input.kind === 'skill' ? input.status !== 'draft' : !['unverified', 'verified'].includes(input.status)) errors.status = 'Skill harus berupa draf; catatan lain menggunakan belum diverifikasi atau terverifikasi.';
  if (input.status === 'verified' && !input.source.trim()) errors.source = 'Catatan terverifikasi memerlukan bukti di sumber.';
  if (input.skill_metadata) {
    const m = input.skill_metadata;
    const texts = [m.trigger, m.version, m.test_evidence, ...m.compatibility];
    if (input.kind !== 'skill' || bytes(m.trigger) > 1000 || bytes(m.version) > 100 || bytes(m.test_evidence) > 2000 || m.compatibility.length > 8 || texts.some(t => controls.test(t)) || texts.reduce((sum, t) => sum + bytes(t), 0) > 4000) errors.skill_metadata = 'Metadata hanya untuk skill: pemicu maks. 1000 byte, versi 100, bukti test 2000, kompatibilitas 8 item, total 4000 byte tanpa karakter kontrol.';
  }
  return errors;
}

export function noteEditorInput(input: NoteInput): NoteInput {
  if (Object.keys(noteEditorErrors(input)).length) throw new Error('Periksa isian catatan yang tidak valid.');
  // Do not spread a Note here: relation/overview flags must remain omitted.
  return { project: input.project, kind: input.kind, title: input.title, content: input.content, source: input.source, status: input.status, ...(typeof input.is_testing === 'boolean' ? { is_testing: input.is_testing } : {}), ...(input.skill_metadata ? { skill_metadata: { ...input.skill_metadata, compatibility: [...input.skill_metadata.compatibility] } } : {}) };
}

export interface ComparedLine { number: number; before: string | null; after: string | null; changed: boolean }
/** Positional, linear-time comparison, not an LCS. Text only; safe to interpolate. */
export function compareNoteLines(before: string, after: string): { lines: ComparedLine[]; truncated: boolean } {
  const maxChars = 20000, maxLines = 200;
  const left = before.slice(0, maxChars).split('\n');
  const right = after.slice(0, maxChars).split('\n');
  const count = Math.min(Math.max(left.length, right.length), maxLines);
  return {
    lines: Array.from({ length: count }, (_, index) => ({
      number: index + 1, before: left[index] ?? null, after: right[index] ?? null,
      changed: left[index] !== right[index],
    })),
    truncated: before.length > maxChars || after.length > maxChars || left.length > maxLines || right.length > maxLines,
  };
}
