import { NODE_HEIGHT, NODE_WIDTH } from "./layout";
import type { Position, Spec, SpecGroup } from "./types";

// A group is drawn as a frame around its cards. An empty one keeps a place and
// a size of its own so cards can be dropped in it. A card belongs to the frame
// its center is in when it is dropped; frames hold still during a drag.

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export const FRAME_PAD = 20;
export const FRAME_HEAD = 26;
export const NEW_FRAME = { width: 560, height: 360 };

export function rectOf(g: SpecGroup): Rect | undefined {
  if (!g.position || !g.size) return undefined;
  return { ...g.position, ...g.size };
}

/** The frame that fits around the group's cards, or undefined when it has none placed. */
export function fitAround(spec: Spec, id: string): Rect | undefined {
  const cards = spec.nodes.flatMap((n) => (n.group === id && n.position ? [n.position] : []));
  if (cards.length === 0) return undefined;
  const left = Math.min(...cards.map((p) => p.x));
  const top = Math.min(...cards.map((p) => p.y));
  const right = Math.max(...cards.map((p) => p.x + NODE_WIDTH));
  const bottom = Math.max(...cards.map((p) => p.y + NODE_HEIGHT));
  return { x: left - FRAME_PAD, y: top - FRAME_PAD - FRAME_HEAD, width: right - left + 2 * FRAME_PAD, height: bottom - top + 2 * FRAME_PAD + FRAME_HEAD };
}

function withRect(g: SpecGroup, r: Rect | undefined): SpecGroup {
  return r ? { ...g, position: { x: r.x, y: r.y }, size: { width: r.width, height: r.height } } : g;
}

/** Gives every group without a frame one that fits around its cards; refit redoes all of them. */
export function framed(spec: Spec, refit = false): Spec {
  if (!spec.groups?.some((g) => refit || !rectOf(g))) return spec;
  return { ...spec, groups: spec.groups.map((g) => (refit || !rectOf(g) ? withRect(g, fitAround(spec, g.id)) : g)) };
}

const contains = (r: Rect, p: Position) => p.x >= r.x && p.x <= r.x + r.width && p.y >= r.y && p.y <= r.y + r.height;

/** The smallest of the frames the point is in. */
export function groupAt(frames: Map<string, Rect>, p: Position): string | undefined {
  let best: { id: string; area: number } | undefined;
  for (const [id, r] of frames) {
    if (contains(r, p) && (!best || r.width * r.height < best.area)) best = { id, area: r.width * r.height };
  }
  return best?.id;
}

/** Where a frame is drawn: around its cards, or where it was put when it has none. */
export function frameRect(spec: Spec, g: SpecGroup): Rect | undefined {
  return fitAround(spec, g.id) ?? rectOf(g);
}
