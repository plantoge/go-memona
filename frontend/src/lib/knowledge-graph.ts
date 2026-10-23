import type { GraphData, GraphNode } from './types';

export interface KnowledgeNode extends GraphNode { x: number; y: number; radius: number; degree: number }
export const KNOWLEDGE_LIMIT = 500;
// Bounded deterministic force relaxation, no animation loop or background worker.
// Project anchors affect placement only; they never introduce inferred edges.
export function knowledgeLayout(data: GraphData) {
  const degree = new Map<string, number>();
  for (const edge of data.edges) { degree.set(edge.from, (degree.get(edge.from) || 0) + 1); degree.set(edge.to, (degree.get(edge.to) || 0) + 1); }
  const ordered = [...data.nodes].sort((a, b) => Number(!!b.is_project_summary) - Number(!!a.is_project_summary) || (degree.get(b.id) || 0) - (degree.get(a.id) || 0) || a.id.localeCompare(b.id));
  const projects = [...new Set(ordered.map(n => n.project))].sort();
  const nodes: KnowledgeNode[] = ordered.slice(0, KNOWLEDGE_LIMIT).map((n, i) => {
    const a = i * 2.399963229728653, r = 45 * Math.sqrt(i);
    return { ...n, x: 600 + Math.cos(a) * r, y: 400 + Math.sin(a) * r, degree: degree.get(n.id) || 0, radius: n.is_project_summary ? 20 : 10 + Math.min(7, Math.sqrt(degree.get(n.id) || 0) * 2) };
  });
  const index = new Map(nodes.map((n, i) => [n.id, i]));
  const edges = data.edges.filter(e => index.has(e.from) && index.has(e.to)).sort((a, b) => `${a.from}:${a.to}:${a.type}`.localeCompare(`${b.from}:${b.to}:${b.type}`));
  // Preserve the legacy 180 passes through 100 nodes; larger graphs use at
  // most about two million repulsion pairs (500 nodes × 16 passes).
  const iterations = Math.max(16, Math.floor(180 * Math.min(1, 100 / Math.max(1, nodes.length)) ** 2));
  for (let step = 0; step < iterations; step++) {
    const force = nodes.map(() => ({ x: 0, y: 0 }));
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const dx = nodes[i].x - nodes[j].x, dy = nodes[i].y - nodes[j].y;
        const d = Math.max(1, Math.hypot(dx, dy));
        const f = Math.min(12, 2400 / (d * d) + (d < 80 ? (80 - d) * .06 : 0));
        force[i].x += dx / d * f; force[i].y += dy / d * f;
        force[j].x -= dx / d * f; force[j].y -= dy / d * f;
      }
    }
    for (const e of edges) {
      const i = index.get(e.from)!, j = index.get(e.to)!;
      const dx = nodes[j].x - nodes[i].x, dy = nodes[j].y - nodes[i].y, d = Math.max(1, Math.hypot(dx, dy));
      const f = (d - 135) * .018;
      force[i].x += dx / d * f; force[i].y += dy / d * f; force[j].x -= dx / d * f; force[j].y -= dy / d * f;
    }
    nodes.forEach((n, i) => {
      const a = projects.indexOf(n.project) / Math.max(1, projects.length) * Math.PI * 2;
      const r = projects.length > 1 ? 220 : 0;
      force[i].x += (600 + Math.cos(a) * r - n.x) * .009;
      force[i].y += (400 + Math.sin(a) * r - n.y) * .009;
      n.x = Math.max(65, Math.min(1135, n.x + Math.max(-12, Math.min(12, force[i].x))));
      n.y = Math.max(65, Math.min(735, n.y + Math.max(-12, Math.min(12, force[i].y))));
    });
  }
  return { nodes, edges, limited: data.nodes.length > KNOWLEDGE_LIMIT || data.truncated };
}

export function knowledgeEdge(from: KnowledgeNode, to: KnowledgeNode, type: string): string {
  if (from.id === to.id) return `M ${from.x} ${from.y - from.radius} C ${from.x + 65} ${from.y - 85}, ${from.x - 65} ${from.y - 85}, ${from.x - 5} ${from.y - from.radius}`;
  const dx = to.x - from.x, dy = to.y - from.y, d = Math.max(1, Math.hypot(dx, dy));
  const bend = type === 'supersedes' ? 24 : 10;
  return `M ${from.x + dx / d * (from.radius + 3)} ${from.y + dy / d * (from.radius + 3)} Q ${(from.x + to.x) / 2 - dy / d * bend} ${(from.y + to.y) / 2 + dx / d * bend} ${to.x - dx / d * (to.radius + 9)} ${to.y - dy / d * (to.radius + 9)}`;
}
