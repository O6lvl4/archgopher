import type { Rect } from "./frames";
import type { Position } from "./types";

// While a card is dragged it lines up with the others: an edge or the center
// that comes within a few pixels of another card's snaps to it, and the line
// it snapped to is shown. Otherwise it snaps to the grid.

export const GRID = 16;
const PULL = 8;

export interface Guides {
  x?: number;
  y?: number;
}

/** The candidate lines of a box along one axis: start, center, end. */
const lines = (start: number, size: number) => [start, start + size / 2, start + size];

/** The nearest pull of the moving box's lines onto the others' lines, within PULL. */
function nearest(moving: number[], others: number[][]): { shift: number; at: number } | undefined {
  let best: { shift: number; at: number } | undefined;
  for (const o of others) {
    for (const m of moving) {
      for (const t of o) {
        const shift = t - m;
        if (Math.abs(shift) <= PULL && (!best || Math.abs(shift) < Math.abs(best.shift))) best = { shift, at: t };
      }
    }
  }
  return best;
}

const toGrid = (v: number) => Math.round(v / GRID) * GRID;

/** Where a box dragged to `box` lands, and the guide lines it lines up with. */
export function align(box: Rect, others: Rect[]): { position: Position; guides: Guides } {
  const x = nearest(lines(box.x, box.width), others.map((o) => lines(o.x, o.width)));
  const y = nearest(lines(box.y, box.height), others.map((o) => lines(o.y, o.height)));
  return {
    position: { x: x ? box.x + x.shift : toGrid(box.x), y: y ? box.y + y.shift : toGrid(box.y) },
    guides: { x: x?.at, y: y?.at },
  };
}
