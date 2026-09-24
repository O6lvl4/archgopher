import dagre from "@dagrejs/dagre";
import type { Position, Spec, SpecGroup } from "./types";

export const NODE_WIDTH = 236;
// The height of a card with readings (.card in styles.css); the layout spaces rows by it.
export const NODE_HEIGHT = 140;

type Graph = InstanceType<typeof dagre.graphlib.Graph>;
type Box = { x: number; y: number; width: number; height: number };

const groupKey = (id: string) => `group:${id}`;

/** The dagre graph: cards, their edges, a cluster per filled group and a box per empty one. */
function graphOf(spec: Spec): { g: Graph; empty: SpecGroup[] } {
  const g = new dagre.graphlib.Graph({ compound: true });
  g.setGraph({ rankdir: "LR", nodesep: 40, ranksep: 64, marginx: 20, marginy: 20 });
  g.setDefaultEdgeLabel(() => ({}));
  const filled = new Set(spec.nodes.flatMap((n) => (n.group ? [n.group] : [])));
  const empty = (spec.groups ?? []).filter((gr) => !filled.has(gr.id));
  for (const id of filled) g.setNode(groupKey(id), {});
  for (const gr of empty) g.setNode(groupKey(gr.id), { width: gr.size?.width ?? 560, height: gr.size?.height ?? 360 });
  for (const n of spec.nodes) {
    g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
    if (n.group) g.setParent(n.id, groupKey(n.group));
  }
  for (const e of spec.edges) g.setEdge(e.from, e.to);
  return { g, empty };
}

const corner = (b: Box) => ({ x: Math.round(b.x - b.width / 2), y: Math.round(b.y - b.height / 2) });

/**
 * Lays the graph out left to right, callers on the left, keeping each group's
 * nodes together. An empty group takes the room of its frame, returned under
 * "group:<id>" as the frame's top-left corner.
 */
export function autoLayout(spec: Spec): Record<string, Position> {
  const { g, empty } = graphOf(spec);
  dagre.layout(g);
  const out: Record<string, Position> = {};
  const place = (key: string) => {
    const b = g.node(key) as Box | undefined;
    if (b) out[key] = corner(b);
  };
  for (const n of spec.nodes) place(n.id);
  for (const gr of empty) place(groupKey(gr.id));
  return out;
}
