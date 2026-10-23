import type { GraphEdge, GraphNode } from './types.ts';
import { KNOWLEDGE_LIMIT } from './knowledge-graph.ts';
export type PositionedGraphNode = GraphNode & { x: number; y: number; z: number; degree: number };
/** d3 mutates nodes and link endpoints. Never hand it Svelte state/API objects. */
export function forceGraphData(nodes: PositionedGraphNode[], edges: GraphEdge[]) {
  const copies = nodes.slice(0, KNOWLEDGE_LIMIT).map(n => ({ ...n }));
  const ids = new Set(copies.map(n => n.id));
  return {
    nodes: copies,
    links: edges.filter(e => ids.has(e.from) && ids.has(e.to)).map(e => ({ ...e, source: e.from, target: e.to })),
  };
}
