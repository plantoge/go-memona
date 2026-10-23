import type { Note } from './types';

export interface MetadataChange { label: string; before: string; after: string; truncated: boolean }

/** Compare explicit declarations, never infer which revision is true or safe. */
export function compareRevisionMetadata(before: Note, after: Note): MetadataChange[] {
  const fields: [string, (note: Note) => string][] = [
    ['Judul', n => n.title], ['Sumber / bukti', n => n.source], ['Status', n => n.status],
    ['Arsip', n => String(n.archived)], ['Testing', n => String(!!n.is_testing)],
    ['Ringkasan proyek', n => String(!!n.is_project_summary)],
    ['Waktu verifikasi', n => n.verified_at || ''],
    ['Perlu ditinjau', n => String(!!n.needs_review)],
    ['Terkait dengan', n => [...(n.related_to || [])].sort().join(', ')],
    ['Menggantikan', n => [...(n.supersedes || [])].sort().join(', ')],
    ['Pemicu skill', n => n.skill_metadata?.trigger || ''],
    ['Versi skill', n => n.skill_metadata?.version || ''],
    ['Kompatibilitas skill', n => (n.skill_metadata?.compatibility || []).join(', ')],
    ['Bukti pengujian skill', n => n.skill_metadata?.test_evidence || ''],
  ];
  const changes: MetadataChange[] = [];
  for (const [label, get] of fields) {
    const left = get(before), right = get(after);
    if (left !== right) changes.push({ label, before: left.slice(0, 2000), after: right.slice(0, 2000), truncated: left.length > 2000 || right.length > 2000 });
  }
  return changes;
}
