import { assign, framed, type Rect } from "./frames";
import { autoLayout } from "./layout";
import type { Position, Spec, SpecEdge, SpecGroup, SpecNode, Values } from "./types";

/** What the user has selected on the canvas. */
export type Selection = { kind: "node"; id: string } | { kind: "edge"; index: number } | { kind: "group"; id: string } | undefined;

// The declaration is the only state. Every edit in the UI is one of these actions.

export type Action =
  | { type: "load"; spec: Spec }
  | { type: "meta"; name?: string; region?: string }
  | { type: "addNode"; node: SpecNode }
  | { type: "updateNode"; id: string; patch: Partial<SpecNode> }
  | { type: "renameNode"; id: string; to: string }
  | { type: "removeNode"; id: string }
  | { type: "move"; positions: Record<string, Position> }
  | { type: "drop"; positions: Record<string, Position> }
  | { type: "addEdge"; edge: SpecEdge }
  | { type: "updateEdge"; index: number; patch: Partial<SpecEdge> }
  | { type: "removeEdge"; index: number }
  | { type: "updateGroup"; id: string; patch: Partial<SpecGroup> }
  | { type: "addGroup"; group: SpecGroup }
  | { type: "removeGroup"; id: string }
  | { type: "frame"; id: string; rect: Rect; positions: Record<string, Position> };

export const emptySpec: Spec = { name: "Untitled", region: "us-east-1", nodes: [], edges: [] };

function mapNode(spec: Spec, id: string, f: (n: SpecNode) => SpecNode): Spec {
  return { ...spec, nodes: spec.nodes.map((n) => (n.id === id ? f(n) : n)) };
}

function rename(spec: Spec, id: string, to: string): Spec {
  if (!to || spec.nodes.some((n) => n.id === to)) return spec;
  const swap = (x: string) => (x === id ? to : x);
  return {
    ...spec,
    nodes: spec.nodes.map((n) => (n.id === id ? { ...n, id: to } : n)),
    edges: spec.edges.map((e) => ({ ...e, from: swap(e.from), to: swap(e.to) })),
  };
}

function move(spec: Spec, positions: Record<string, Position>): Spec {
  return { ...spec, nodes: spec.nodes.map((n) => (positions[n.id] ? { ...n, position: positions[n.id] } : n)) };
}

type Handlers = { [K in Action["type"]]: (spec: Spec, a: Extract<Action, { type: K }>) => Spec };

const handlers: Handlers = {
  load: (_, a) => ({ ...a.spec, nodes: a.spec.nodes ?? [], edges: a.spec.edges ?? [] }),
  meta: (spec, a) => ({ ...spec, name: a.name ?? spec.name, region: a.region ?? spec.region }),
  addNode: (spec, a) => ({ ...spec, nodes: [...spec.nodes, a.node] }),
  updateNode: (spec, a) => mapNode(spec, a.id, (n) => ({ ...n, ...a.patch })),
  renameNode: (spec, a) => rename(spec, a.id, a.to),
  removeNode: (spec, a) => ({
    ...spec,
    nodes: spec.nodes.filter((n) => n.id !== a.id),
    edges: spec.edges.filter((e) => e.from !== a.id && e.to !== a.id),
  }),
  move: (spec, a) => move(spec, a.positions),
  addEdge: (spec, a) => ({ ...spec, edges: [...spec.edges, a.edge] }),
  updateEdge: (spec, a) => ({ ...spec, edges: spec.edges.map((e, i) => (i === a.index ? { ...e, ...a.patch } : e)) }),
  removeEdge: (spec, a) => ({ ...spec, edges: spec.edges.filter((_, i) => i !== a.index) }),
  updateGroup: (spec, a) => ({ ...spec, groups: (spec.groups ?? []).map((g) => (g.id === a.id ? { ...g, ...a.patch } : g)) }),
  // A dropped card joins the frame its center is in, or leaves the one it was in.
  drop: (spec, a) => assign(move(spec, a.positions), Object.keys(a.positions)),
  addGroup: (spec, a) => ({ ...spec, groups: [...(spec.groups ?? []), a.group] }),
  removeGroup: (spec, a) => ({
    ...spec,
    groups: (spec.groups ?? []).filter((g) => g.id !== a.id),
    nodes: spec.nodes.map((n) => (n.group === a.id ? { ...n, group: undefined } : n)),
  }),
  // A frame moved or resized: its cards moved with it, and every card is placed again.
  frame: (spec, a) => {
    const moved = move(spec, a.positions);
    const groups = (moved.groups ?? []).map((g) =>
      g.id === a.id ? { ...g, position: { x: a.rect.x, y: a.rect.y }, size: { width: a.rect.width, height: a.rect.height } } : g,
    );
    const next = { ...moved, groups };
    return assign(next, next.nodes.map((n) => n.id));
  },
};

/** Lays everything out again: cards by dagre, frames fitted around their cards, empty frames given room. */
export function tidy(spec: Spec): Spec {
  const layout = autoLayout(spec);
  const laid = framed(move(spec, layout), true);
  return {
    ...laid,
    groups: laid.groups?.map((g) => {
      const p = layout[`group:${g.id}`];
      return p ? { ...g, position: p } : g;
    }),
  };
}

// Changes to the shape of the graph lay it out again; edits to numbers and
// small moves of a card do not.
const reshapes = new Set<Action["type"]>(["addNode", "removeNode", "addEdge", "removeEdge", "addGroup", "removeGroup"]);
const regroups = new Set<Action["type"]>(["drop", "frame", "updateNode"]);
const grouping = (s: Spec) => s.nodes.map((n) => `${n.id}@${n.group ?? ""}`).join("|");

export function reducer(spec: Spec, a: Action): Spec {
  const handle = handlers[a.type] as (spec: Spec, a: Action) => Spec;
  const next = handle(spec, a);
  if (next === spec) return next;
  const regrouped = regroups.has(a.type) && grouping(next) !== grouping(spec);
  return reshapes.has(a.type) || regrouped ? tidy(next) : next;
}

/** Returns values with key set, or removed when v is undefined. */
export function withValue(values: Values | undefined, key: string, v: unknown): Values | undefined {
  const next: Values = { ...values };
  if (v === undefined) delete next[key];
  else next[key] = v;
  return Object.keys(next).length > 0 ? next : undefined;
}

/** True when adding from -> to would close a cycle. */
export function closesCycle(spec: Spec, from: string, to: string): boolean {
  const out = new Map<string, string[]>();
  for (const e of spec.edges) out.set(e.from, [...(out.get(e.from) ?? []), e.to]);
  const stack = [to];
  const seen = new Set<string>();
  while (stack.length > 0) {
    const n = stack.pop() as string;
    if (n === from) return true;
    if (seen.has(n)) continue;
    seen.add(n);
    stack.push(...(out.get(n) ?? []));
  }
  return false;
}

function unused(taken: Set<string>, label: string, fallback: string): string {
  const base = label.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || fallback;
  if (!taken.has(base)) return base;
  let i = 2;
  while (taken.has(`${base}-${i}`)) i++;
  return `${base}-${i}`;
}

/** A fresh id from the display name: "agentcore-runtime", "agentcore-runtime-2", ... */
export function freshId(spec: Spec, label: string): string {
  return unused(new Set(spec.nodes.map((n) => n.id)), label, "node");
}

/** A fresh group id: "vpc", "vpc-2", ... */
export function freshGroupId(spec: Spec, label: string): string {
  return unused(new Set((spec.groups ?? []).map((g) => g.id)), label, "group");
}
