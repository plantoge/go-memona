export interface SkillMetadata { trigger: string; version: string; compatibility: string[]; test_evidence: string }
export interface Note {
  id: string; project: string; kind: string; title: string; content: string;
  status: string; source: string; author: string; revision: number;
  created_at: string; updated_at: string; archived: boolean;
  is_project_summary?: boolean; is_testing?: boolean; related_to?: string[]; supersedes?: string[];
  connections?: Connections;
  skill_metadata?: SkillMetadata;
}
export interface NoteReference { id: string; title: string; project: string; kind: string; archived: boolean }
export interface Connections {
  related_to: NoteReference[]; supersedes: NoteReference[];
  referenced_by: NoteReference[]; superseded_by: NoteReference[]; truncated: boolean;
}
export interface ConnectionInput { is_project_summary: boolean; related_to: string[]; supersedes: string[] }
export interface NoteDetail { note: Note; html: string; history: Note[] | null; history_next_before?: number }
export interface APIKey {
  id: string; name: string; prefix: string; permissions: string[]; projects: string[];
  created_at: string; expires_at?: string; revoked: boolean;
}
export interface Activity { time: string; action: string; actor: string; target: string }
export interface KeyUsage { last_recorded_at: string; action: string }
export interface DuplicateCandidate { id: string; title: string; project: string; revision: number; score: number; reason: string }
export type GraphNode = Pick<Note, 'id' | 'project' | 'kind' | 'title' | 'status' | 'archived' | 'is_project_summary' | 'updated_at' | 'revision'>;
export interface GraphEdge { from: string; to: string; type: 'related_to' | 'supersedes' }
export interface GraphData { nodes: GraphNode[]; edges: GraphEdge[]; truncated: boolean }
export type View = 'notes' | 'skills' | 'keys' | 'activity' | 'storage' | 'graph' | 'projects';
export interface StorageHealth {
  checked_at: string; status: 'ok' | 'warning' | 'error';
  disk: { available: boolean; total_bytes: number; available_bytes: number; available_inodes: number; low: boolean };
  integrity: { complete: boolean; entries: number; notes: number; revisions: number; bytes: number; issues: Record<string, number> };
  staging_entries: number;
  write_failures: { count: number; last_at?: string; last_code?: string };
  write_failures_observed: boolean;
}
export const kindLabels: Record<string, string> = {
  fact: 'Fakta', decision: 'Keputusan', knowledge: 'Pengetahuan', handoff: 'Handoff', skill: 'Skill',
};
export function statusLabel(value: string): string {
  return ({ unverified: 'Belum diverifikasi', verified: 'Terverifikasi', draft: 'Draf', active: 'Dipublikasikan' } as Record<string, string>)[value] || value;
}
export function date(value?: string): string {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? 'Tanggal tidak valid' : parsed.toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' });
}
