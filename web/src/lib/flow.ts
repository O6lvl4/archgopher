import type { Edge, Node } from "@xyflow/react";
import { num } from "./format";
import type { CatalogEntry, NodeResult, Result, Spec, SpecNode } from "./types";

export interface CardData extends Record<string, unknown> {
  node: SpecNode;
  reading?: NodeResult;
  entry?: CatalogEntry;
}

export type CardNode = Node<CardData, "scouter">;

export function readingsById(result: Result | undefined): Map<string, NodeResult> {
  return new Map((result?.nodes ?? []).map((r) => [r.id, r]));
}

/** Builds the flow nodes, keeping what React Flow measured on the previous ones. */
export function toFlowNodes(spec: Spec, result: Result | undefined, catalog: Map<string, CatalogEntry>, prev: CardNode[]): CardNode[] {
  const before = new Map(prev.map((n) => [n.id, n]));
  const readings = readingsById(result);
  return spec.nodes.map((node) => {
    const old = before.get(node.id);
    return {
      ...old,
      id: node.id,
      type: "scouter",
      position: node.position ?? old?.position ?? { x: 0, y: 0 },
      data: { node, reading: readings.get(node.id), entry: catalog.get(node.type) },
    };
  });
}

function throughput(r: NodeResult | undefined): number {
  return Object.values(r?.demand ?? {}).reduce((sum, l) => sum + l.monthly, 0);
}

export function edgeId(index: number): string {
  return `e${index}`;
}

/** Edges carry the monthly volume they move, so the flow is readable on the canvas. */
export function toFlowEdges(spec: Spec, result: Result | undefined, selected: number | undefined): Edge[] {
  const readings = readingsById(result);
  return spec.edges.map((e, i) => {
    const volume = throughput(readings.get(e.from)) * (e.perUnit ?? 1);
    const parts = [e.kind, result ? `${num(volume)}/mo` : undefined].filter(Boolean);
    return {
      id: edgeId(i),
      source: e.from,
      target: e.to,
      label: parts.join(" · "),
      selected: i === selected,
      className: "flow-edge",
    };
  });
}
