import { useState } from "react";
import type { NodeChange } from "@xyflow/react";
import { groupOfFrame, type CardNode, type FlowNode } from "../../lib/flow";
import { groupAt, type Rect } from "../../lib/frames";
import { align, type Guides } from "../../lib/snap";
import type { Spec } from "../../lib/types";

const isCard = (n: FlowNode): n is CardNode => n.type === "scouter";

const boxOf = (n: FlowNode): Rect => ({
  x: n.position.x,
  y: n.position.y,
  width: n.measured?.width ?? n.width ?? 0,
  height: n.measured?.height ?? n.height ?? 0,
});

/** The frames as drawn now, by group id. */
function framesOf(nodes: FlowNode[]): Map<string, Rect> {
  return new Map(nodes.flatMap((n) => {
    const g = groupOfFrame(n.id);
    return g ? [[g, boxOf(n)] as const] : [];
  }));
}

/** A lone dragged card lines up with the others; returns the adjusted change and the guides. */
function snapCard(c: NodeChange<FlowNode>, nodes: FlowNode[]): { change: NodeChange<FlowNode>; guides: Guides } {
  const card = c.type === "position" && c.dragging && c.position ? nodes.find((n) => n.id === c.id && isCard(n)) : undefined;
  if (!card || c.type !== "position" || !c.position) return { change: c, guides: {} };
  const others = nodes.filter((n) => isCard(n) && n.id !== c.id).map(boxOf);
  const { position, guides } = align({ ...boxOf(card), ...c.position }, others);
  return { change: { ...c, position }, guides };
}

/** While a frame is dragged by its label, its cards move by the same amount. */
function followFrame(c: NodeChange<FlowNode>, nodes: FlowNode[], spec: Spec): NodeChange<FlowNode>[] {
  if (c.type !== "position" || !c.dragging || !c.position) return [];
  const group = groupOfFrame(c.id);
  const frame = nodes.find((n) => n.id === c.id);
  if (!group || !frame) return [];
  const dx = c.position.x - frame.position.x;
  const dy = c.position.y - frame.position.y;
  const members = new Set(spec.nodes.filter((n) => n.group === group).map((n) => n.id));
  return nodes
    .filter((n) => members.has(n.id))
    .map((n) => ({ type: "position" as const, id: n.id, position: { x: n.position.x + dx, y: n.position.y + dy }, dragging: true }));
}

export interface Drop {
  positions: Record<string, { x: number; y: number }>;
  /** For dropped cards: the frame each landed in, or undefined. */
  groups?: Record<string, string | undefined>;
  /** For a dragged empty frame: where it is now. */
  frames?: Record<string, { x: number; y: number }>;
}

/**
 * Dragging on the canvas: cards line up with each other (with guide lines),
 * frames hold still while a card moves and follow their label with their
 * cards, and a drop reports where everything went and which frame each card
 * landed in.
 */
export function useDrag(spec: Spec) {
  const [guides, setGuides] = useState<Guides>({});
  const [held, setHeld] = useState<Map<string, Rect>>();

  const adjust = (changes: NodeChange<FlowNode>[], nodes: FlowNode[]): NodeChange<FlowNode>[] => {
    const lone = changes.filter((c) => c.type === "position" && c.dragging).length === 1;
    let seen: Guides = {};
    const out = changes.flatMap((c) => {
      const snapped = lone ? snapCard(c, nodes) : { change: c, guides: {} };
      if (snapped.guides.x !== undefined || snapped.guides.y !== undefined) seen = snapped.guides;
      return [snapped.change, ...followFrame(c, nodes, spec)];
    });
    setGuides(seen);
    return out;
  };

  const start = (nodes: FlowNode[]) => setHeld(framesOf(nodes));

  const stop = (draggedNow: FlowNode[], nodes: FlowNode[]): Drop => {
    // React Flow reports where the pointer put them; what is drawn is where they snapped.
    const shown = new Map(nodes.map((n) => [n.id, n]));
    const dragged = draggedNow.map((n) => shown.get(n.id) ?? n);
    const frames = held ?? framesOf(nodes);
    setGuides({});
    setHeld(undefined);
    const frame = dragged.find((n) => groupOfFrame(n.id));
    const group = frame && groupOfFrame(frame.id);
    if (frame && group) {
      const members = new Set(spec.nodes.filter((n) => n.group === group).map((n) => n.id));
      const positions = Object.fromEntries(nodes.filter((n) => members.has(n.id)).map((n) => [n.id, n.position]));
      return { positions, frames: members.size === 0 ? { [group]: frame.position } : undefined };
    }
    const cards = dragged.filter(isCard);
    const center = (n: FlowNode) => {
      const b = boxOf(n);
      return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
    };
    return {
      positions: Object.fromEntries(cards.map((n) => [n.id, n.position])),
      groups: Object.fromEntries(cards.map((n) => [n.id, groupAt(frames, center(n))])),
    };
  };

  return { guides, held, adjust, start, stop };
}
