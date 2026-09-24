import type { Edge, Node } from "@xyflow/react";
import { num } from "./format";
import { NODE_HEIGHT, NODE_WIDTH } from "./layout";
import type { CatalogEntry, NodeResult, Result, Spec, SpecGroup, SpecNode } from "./types";

export interface CardData extends Record<string, unknown> {
  node: SpecNode;
  reading?: NodeResult;
  entry?: CatalogEntry;
}

export type CardNode = Node<CardData, "scouter">;

export interface FrameData extends Record<string, unknown> {
  group: SpecGroup;
}

export type FrameNode = Node<FrameData, "frame">;

const FRAME_PAD = 20;
const FRAME_HEAD = 26;

/**
 * Frames around the cards of each group, sized from where the cards are now
 * (so they follow a drag) and how big React Flow measured them.
 */
export function toFrames(spec: Spec, cards: CardNode[]): FrameNode[] {
  const byId = new Map(cards.map((c) => [c.id, c]));
  return (spec.groups ?? []).flatMap((group) => {
    const members = spec.nodes.flatMap((n) => {
      const c = n.group === group.id ? byId.get(n.id) : undefined;
      return c ? [c] : [];
    });
    if (members.length === 0) return [];
    const left = Math.min(...members.map((c) => c.position.x));
    const top = Math.min(...members.map((c) => c.position.y));
    const right = Math.max(...members.map((c) => c.position.x + (c.measured?.width ?? NODE_WIDTH)));
    const bottom = Math.max(...members.map((c) => c.position.y + (c.measured?.height ?? NODE_HEIGHT)));
    const frame: FrameNode = {
      id: `group:${group.id}`,
      type: "frame",
      position: { x: left - FRAME_PAD, y: top - FRAME_PAD - FRAME_HEAD },
      width: right - left + 2 * FRAME_PAD,
      height: bottom - top + 2 * FRAME_PAD + FRAME_HEAD,
      data: { group },
      selectable: false,
      draggable: false,
      focusable: false,
      deletable: false,
      zIndex: -1,
    };
    return [frame];
  });
}

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
