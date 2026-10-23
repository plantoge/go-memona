import { knowledgeLayout } from './knowledge-graph.ts';
import type { GraphData } from './types.ts';

/** Stable shallow depth, not a hierarchy or an inferred relationship. */
export function threeGraphLayout(data: GraphData) {
  const seed = knowledgeLayout(data);
  return { ...seed, nodes: seed.nodes.map(node => {
    let hash = 2166136261;
    for (const char of node.id) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0;
    return { ...node, x: node.x - 600, y: 400 - node.y, z: node.is_project_summary ? 45 : (hash % 161) - 80 };
  }) };
}

export function graphNeighborhood(data: Pick<GraphData, 'nodes' | 'edges'>, id: string): Set<string> {
  const ids = new Set<string>(id ? [id] : []);
  for (const edge of data.edges) if (edge.from === id || edge.to === id) { ids.add(edge.from); ids.add(edge.to); }
  return ids;
}
