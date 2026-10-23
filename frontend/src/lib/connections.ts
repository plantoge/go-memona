import type { ConnectionInput } from './types';

export function relationIDs(text: string): string[] {
  const ids = text.trim().split(/[\s,]+/).filter(Boolean);
  if (ids.length > 20) throw new Error('Maksimal 20 ID per jenis relasi.');
  if (ids.some(id => !/^[a-f0-9]{32}$/.test(id))) throw new Error('Gunakan ID catatan 32 karakter heksadesimal, dipisahkan baris atau koma.');
  if (new Set(ids).size !== ids.length) throw new Error('ID relasi tidak boleh berulang.');
  return ids;
}
export function connectionInput(summary: boolean, related: string, supersedes: string, ownID: string): ConnectionInput {
  const related_to = relationIDs(related), replaced = relationIDs(supersedes);
  const all = [...related_to, ...replaced];
  if (all.includes(ownID)) throw new Error('Catatan tidak boleh merujuk dirinya sendiri.');
  if (new Set(all).size !== all.length) throw new Error('Satu ID hanya boleh digunakan pada satu jenis relasi.');
  return { is_project_summary: summary, related_to, supersedes: replaced };
}
