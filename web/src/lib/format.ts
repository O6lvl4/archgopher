const usdFormat = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 });
const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 });
const plain = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 });

/** Dollars to the cent; an amount above zero that rounds to $0.00 shows as <$0.01, so it never reads as free. */
export function usd(v: number | null | undefined): string {
  if (v === null || v === undefined) return "unknown";
  if (v > 0 && v < 0.005) return "<$0.01";
  return usdFormat.format(v);
}

export function num(v: number | null | undefined): string {
  if (v === null || v === undefined) return "unknown";
  if (Math.abs(v) >= 10000) return compact.format(v);
  if (v !== 0 && Math.abs(v) < 0.01) return v.toPrecision(2);
  return plain.format(v);
}

export function pct(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return "–";
  return `${(v * 100).toFixed(digits)}%`;
}

export function ms(v: number | undefined): string {
  if (v === undefined) return "–";
  return `${plain.format(v)} ms`;
}

/** Severity of a headroom: over the limit, tight, or fine. */
export function tone(headroom: number | null | undefined): "bad" | "warn" | "ok" | "none" {
  if (headroom === null || headroom === undefined) return "none";
  if (headroom < 0) return "bad";
  if (headroom < 0.2) return "warn";
  return "ok";
}
