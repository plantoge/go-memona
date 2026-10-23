export type Harness = 'opencode' | 'hermes';
export function integrationSnippet(harness: Harness, endpoint: string): string {
  const url = new URL(endpoint);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || url.pathname !== '/mcp' || /[\s\u0000-\u001f\u007f]/.test(endpoint)) throw new Error('Endpoint MCP tidak valid.');
  if (harness === 'opencode') return JSON.stringify({ $schema: 'https://opencode.ai/config.json', mcp: { servers: { memona: { type: 'remote', url: endpoint, oauth: false, headers: { Authorization: 'Bearer {env:MEMONA_API_KEY}' } } } } }, null, 2);
  return `mcp_servers:\n  memona:\n    url: ${JSON.stringify(endpoint)}\n    headers:\n      Authorization: "Bearer \${MEMONA_API_KEY}"\n`;
}
export function rotationReady(oldID: string, replacementID: string, acknowledged: boolean, revoked: boolean): boolean {
  return !!oldID && !!replacementID && oldID !== replacementID && acknowledged && !revoked;
}
