export interface Workspace { id: string; project: string; name: string; kind: 'personal' | 'team'; role: string }
export interface Invitation { id: string; email: string; workspace_name: string; status?: string }
export interface UserKey { id: string; name: string; workspace_id: string; prefix?: string; permissions: string[]; expires_at?: string; revoked: boolean; created_at?: string }

export function splitWorkspaces(items: Workspace[]): { personal: Workspace; teams: Workspace[] } {
  const personal = items.find(item => item.kind === 'personal' && item.id);
  if (!personal) throw new Error('Ruang pribadi tidak tersedia.');
  return { personal, teams: items.filter(item => item.kind === 'team') };
}

export function notePath(id: string, workspaceID: string): string {
  return `/api/user/notes/${encodeURIComponent(id)}?${new URLSearchParams({ workspace_id: workspaceID })}`;
}

export function expiryValue(day: string): string {
  return day ? new Date(`${day}T23:59:59`).toISOString() : '';
}
