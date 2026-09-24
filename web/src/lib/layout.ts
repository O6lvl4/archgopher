import dagre from "@dagrejs/dagre";
import type { Position, Spec } from "./types";

export const NODE_WIDTH = 236;
// The height of a card with readings (.card in styles.css); the layout spaces rows by it.
export const NODE_HEIGHT = 140;

/** Lays the graph out left to right, callers on the left, keeping each group's nodes together. */
export function autoLayout(spec: Spec): Record<string, Position> {
  const g = new dagre.graphlib.Graph({ compound: true });
  g.setGraph({ rankdir: "LR", nodesep: 20, ranksep: 56, marginx: 20, marginy: 20 });
  g.setDefaultEdgeLabel(() => ({}));
  const groups = new Set((spec.groups ?? []).map((gr) => `group:${gr.id}`));
  for (const id of groups) g.setNode(id, {});
  for (const n of spec.nodes) {
    g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
    if (n.group && groups.has(`group:${n.group}`)) g.setParent(n.id, `group:${n.group}`);
  }
  for (const e of spec.edges) g.setEdge(e.from, e.to);
  dagre.layout(g);
  const out: Record<string, Position> = {};
  for (const n of spec.nodes) {
    const p = g.node(n.id) as { x: number; y: number } | undefined;
    if (p) out[n.id] = { x: Math.round(p.x - NODE_WIDTH / 2), y: Math.round(p.y - NODE_HEIGHT / 2) };
  }
  return out;
}

/** True when some node has never been placed. */
export function needsLayout(spec: Spec): boolean {
  return spec.nodes.some((n) => !n.position);
}

const overlaps = (a: Position, b: Position) => Math.abs(a.x - b.x) < NODE_WIDTH + 12 && Math.abs(a.y - b.y) < NODE_HEIGHT + 12;

/** The first spot at or below want that no placed node covers. */
export function freeSpot(spec: Spec, want: Position): Position {
  const taken = spec.nodes.flatMap((n) => (n.position ? [n.position] : []));
  const spot = { ...want };
  while (taken.some((p) => overlaps(p, spot))) spot.y += NODE_HEIGHT + 16;
  return spot;
}
