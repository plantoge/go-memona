export interface Project { slug: string; name: string; description: string; aliases: string[]; archived: boolean; revision: number }
export type ProjectInput = Omit<Project, 'revision'> & { expected_revision: number };
const bytes = (value: string) => new TextEncoder().encode(value).length;
export function projectInput(slug: string, name: string, description: string, rawAliases: string, archived: boolean, revision = 0): ProjectInput {
  name = name.trim(); description = description.trim();
  const aliases = rawAliases.split('\n').map(v => v.trim()).filter(Boolean);
  if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(slug)) throw new Error('Slug wajib 1–64 karakter huruf kecil, angka atau tanda hubung.');
  if (!name || bytes(name) > 200) throw new Error('Nama wajib diisi, maksimal 200 byte.');
  if (bytes(description) > 2000) throw new Error('Deskripsi maksimal 2000 byte.');
  if (aliases.length > 20 || new Set(aliases).size !== aliases.length || aliases.some(v => bytes(v) > 200)) throw new Error('Maksimal 20 alias unik, masing-masing maksimal 200 byte.');
  if (!Number.isSafeInteger(revision) || revision < 0) throw new Error('Revisi proyek tidak valid.');
  return { slug, name, description, aliases, archived, expected_revision: revision };
}
