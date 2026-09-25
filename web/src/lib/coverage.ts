import type { Coverage } from "./types";

const total = (m: Record<string, number>) => Object.values(m).reduce((a, b) => a + b, 0);

/** One line for an import: how many resources are read, free and not priced yet, naming the last. */
export function coverageLine(c: Coverage): { text: string; unpriced: boolean } {
  const unpriced = Object.entries(c.unpriced)
    .map(([t, n]) => (n > 1 ? `${t} ×${n}` : t))
    .sort();
  let text = `${total(c.read)} resources read, ${total(c.free)} free, ${unpriced.length ? total(c.unpriced) : 0} without a price yet`;
  if (unpriced.length) text += `: ${unpriced.join(", ")}`;
  return { text, unpriced: unpriced.length > 0 };
}
