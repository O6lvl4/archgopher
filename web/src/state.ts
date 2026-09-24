import type { Position, Spec, SpecEdge, SpecNode, Values } from "./types";

// The declaration is the only state. Every edit in the UI is one of these actions.

export type Action =
  | { type: "load"; spec: Spec }
  | { type: "meta"; name?: string; region?: string }
  | { type: "addNode"; node: SpecNode }
  | { type: "updateNode"; id: string; patch: Partial<SpecNode> }
  | { type: "renameNode"; id: string; to: string }
  | { type: "removeNode"; id: string }
  | { type: "move"; positions: Record<string, Position> }
  | { type: "addEdge"; edge: SpecEdge }
  | { type: "updateEdge"; index: number; patch: Partial<SpecEdge> }
  | { type: "removeEdge"; index: number };

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
};

export function reducer(spec: Spec, a: Action): Spec {
  const handle = handlers[a.type] as (spec: Spec, a: Action) => Spec;
  return handle(spec, a);
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

/** A fresh id based on the type: "lambda", "lambda-2", ... */
export function freshId(spec: Spec, type: string): string {
  const base = type.replace(/^aws_/, "").split("_")[0] ?? "node";
  const taken = new Set(spec.nodes.map((n) => n.id));
  if (!taken.has(base)) return base;
  let i = 2;
  while (taken.has(`${base}-${i}`)) i++;
  return `${base}-${i}`;
}
