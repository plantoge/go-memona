import type { GraphData, GraphEdge, GraphNode } from './types';

export const NODE_WIDTH = 176;
export const NODE_HEIGHT = 70;
export const EMPTY_GRAPH: GraphData = { nodes: [], edges: [], truncated: false };
export interface PositionedNode extends GraphNode { x: number; y: number }
export interface GraphLayout { nodes: PositionedNode[]; edges: GraphEdge[]; width: number; height: number }
export interface Camera { x: number; y: number; zoom: number }
export const INITIAL_CAMERA: Camera = { x: 0, y: 0, zoom: 1 };

const compare = (a: string, b: string) => a < b ? -1 : a > b ? 1 : 0;

// Bound rendering independently of the server, retaining only explicit edges whose ends are visible.
export function graphData(value: unknown): GraphData {
  if (!value || typeof value !== 'object') throw new Error('Respons graf tidak valid. Coba muat ulang data.');
  const input = value as GraphData;
  const nodes = input.nodes === null ? [] : input.nodes;
  const edges = input.edges === null ? [] : input.edges;
  if (!Array.isArray(nodes) || !Array.isArray(edges) || typeof input.truncated !== 'boolean') throw new Error('Respons graf tidak valid. Coba muat ulang data.');
  const seen = new Set<string>();
  const bounded = nodes.slice(0, 500);
  for (const node of bounded) {
    if (!node || typeof node.id !== 'string' || !node.id || seen.has(node.id) ||
      !['title', 'project', 'kind', 'status', 'updated_at'].every(key => typeof node[key as keyof GraphNode] === 'string') ||
      typeof node.archived !== 'boolean' || typeof node.revision !== 'number') throw new Error('Simpul graf tidak valid. Coba muat ulang data.');
    seen.add(node.id);
  }
  const edgeIDs = new Set<string>();
  const explicit = edges.filter(edge => {
    if (!edge || (edge.type !== 'related_to' && edge.type !== 'supersedes') || !seen.has(edge.from) || !seen.has(edge.to)) return false;
    const id = JSON.stringify([edge.from, edge.to, edge.type]);
    if (edgeIDs.has(id)) return false;
    edgeIDs.add(id); return true;
  });
  return { nodes: bounded, edges: explicit, truncated: input.truncated || nodes.length > 500 };
}

// Undirected adjacency is used only to group layout; it never creates reverse/inferred relationships.
export function layoutGraph(data: GraphData): GraphLayout {
  const ordered = [...data.nodes].sort((a, b) => compare(a.project, b.project) || compare(a.title, b.title) || compare(a.id, b.id));
  const byID = new Map(ordered.map(node => [node.id, node]));
  const adjacency = new Map(ordered.map(node => [node.id, new Set<string>()]));
  const edges = [...data.edges].filter(edge => byID.has(edge.from) && byID.has(edge.to))
    .sort((a, b) => compare(a.from, b.from) || compare(a.to, b.to) || compare(a.type, b.type));
  for (const edge of edges) { adjacency.get(edge.from)!.add(edge.to); adjacency.get(edge.to)!.add(edge.from); }
  const visited = new Set<string>();
  const components: GraphNode[][] = [];
  for (const node of ordered) {
    if (visited.has(node.id)) continue;
    const component: GraphNode[] = [], queue = [node.id];
    visited.add(node.id);
    for (let index = 0; index < queue.length; index++) {
      const id = queue[index]; component.push(byID.get(id)!);
      for (const next of [...adjacency.get(id)!].sort(compare)) {
        if (!visited.has(next)) { visited.add(next); queue.push(next); }
      }
    }
    components.push(component);
  }
  const positioned: PositionedNode[] = [];
  let left = 40, top = 40, rowHeight = 0, width = 800;
  for (const component of components) {
    const columns = Math.min(6, Math.ceil(Math.sqrt(component.length)));
    const blockWidth = columns * 240, blockHeight = Math.ceil(component.length / columns) * 144;
    if (left > 40 && left + blockWidth > 1480) { left = 40; top += rowHeight + 40; rowHeight = 0; }
    component.forEach((node, index) => positioned.push({ ...node, x: left + (index % columns) * 240, y: top + Math.floor(index / columns) * 144 }));
    width = Math.max(width, left + blockWidth); rowHeight = Math.max(rowHeight, blockHeight); left += blockWidth + 32;
  }
  return { nodes: positioned, edges, width: width + 40, height: Math.max(440, top + rowHeight + 40) };
}

export function edgePath(from: PositionedNode, to: PositionedNode, type: GraphEdge['type']): string {
  const x = from.x + NODE_WIDTH / 2, y = from.y + NODE_HEIGHT / 2;
  if (from.id === to.id) return `M ${x + 30} ${from.y} C ${x + 100} ${from.y - 48}, ${x - 100} ${from.y - 48}, ${x - 30} ${from.y}`;
  const tx = to.x + NODE_WIDTH / 2, ty = to.y + NODE_HEIGHT / 2;
  const dx = tx - x, dy = ty - y;
  const distance = Math.hypot(dx, dy) || 1;
  // Distinct bends preserve reciprocal arrows and the two explicit relationship types.
  const bend = type === 'supersedes' ? 30 : 14;
  const cx = (x + tx) / 2 - dy / distance * bend, cy = (y + ty) / 2 + dx / distance * bend;
  const startScale = Math.min((NODE_WIDTH / 2 + 3) / Math.abs(cx - x), (NODE_HEIGHT / 2 + 3) / Math.abs(cy - y));
  const endScale = Math.min((NODE_WIDTH / 2 + 8) / Math.abs(cx - tx), (NODE_HEIGHT / 2 + 8) / Math.abs(cy - ty));
  return `M ${x + (cx - x) * startScale} ${y + (cy - y) * startScale} Q ${cx} ${cy} ${tx + (cx - tx) * endScale} ${ty + (cy - ty) * endScale}`;
}

export function zoomCamera(camera: Camera, factor: number): Camera {
  return { ...camera, zoom: Math.min(8, Math.max(0.5, camera.zoom * factor)) };
}
export function cameraViewBox(layout: Pick<GraphLayout, 'width' | 'height'>, camera: Camera): string {
  const width = layout.width / camera.zoom, height = layout.height / camera.zoom;
  return `${(layout.width - width) / 2 + camera.x} ${(layout.height - height) / 2 + camera.y} ${width} ${height}`;
}
export const relationshipLabel = (type: GraphEdge['type']) => type === 'supersedes' ? 'menggantikan' : 'terkait ke';
