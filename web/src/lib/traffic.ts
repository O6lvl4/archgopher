import type { Field, Load, Spec, SpecNode, Traffic } from "./types";

// The ways a node's load can be said, and the fields each one asks for. The
// engine turns traffic into a load and explains the arithmetic; the UI only
// edits the declaration.

export type Shape = "load" | "rate" | "users" | "concurrent" | "schedule" | "batch";

export const shapes: { shape: Shape; label: string; hint: string }[] = [
  { shape: "load", label: "Volume and peak", hint: "A month's volume and the peak per second, as numbers" },
  { shape: "rate", label: "Rate", hint: "So many a second, minute, hour, day, week or month" },
  { shape: "users", label: "Users", hint: "People, and what each does a day, week or month" },
  { shape: "concurrent", label: "Concurrent users", hint: "People at work at the same time, one action every few seconds" },
  { shape: "schedule", label: "Schedule", hint: "rate(), cron(), Unix or Azure cron" },
  { shape: "batch", label: "Batch", hint: "So many items at once on a schedule, worked through in a set time" },
];

export function shapeOf(node: Pick<SpecNode, "load" | "traffic">): Shape {
  const t = node.traffic;
  if (!t) return "load";
  if (t.rate) return "rate";
  if (t.users) return "users";
  if (t.concurrent) return "concurrent";
  if (t.batch) return "batch";
  return "schedule";
}

type Switched = { load?: Load; traffic?: Traffic };
type When = Pick<Traffic, "hours" | "days">;

const fresh: Record<Shape, (node: Pick<SpecNode, "load">, when: When) => Switched> = {
  load: (node) => ({ load: node.load ?? { monthly: 0, peakPerSecond: 0 }, traffic: undefined }),
  rate: (_, when) => ({ load: undefined, traffic: { ...when, rate: { count: 0, per: "day" } } }),
  users: (_, when) => ({ load: undefined, traffic: { ...when, users: { count: 0, actions: 1, per: "day" } } }),
  concurrent: (_, when) => ({ load: undefined, traffic: { ...when, concurrent: { users: 0, everySeconds: 30 } } }),
  schedule: () => ({ load: undefined, traffic: { schedule: "rate(1 hour)" } }),
  batch: () => ({ load: undefined, traffic: { batch: { items: 0, every: "rate(1 day)", withinSeconds: 600 } } }),
};

/** The load and traffic of a node switched to a shape, keeping when the traffic comes. */
export function switchTo(shape: Shape, node: Pick<SpecNode, "load" | "traffic">): Switched {
  return fresh[shape](node, { hours: node.traffic?.hours, days: node.traffic?.days });
}

const periods = ["second", "minute", "hour", "day", "week", "month"];
const num = (key: string, label: string, hint?: string, unit?: string): Field => ({ key, label, type: "number", required: true, hint, unit });

/** The fields of each shape's part of the traffic. */
export const partFields: Record<"rate" | "users" | "concurrent" | "batch", Field[]> = {
  rate: [
    num("count", "How many"),
    { key: "per", label: "Per", type: "choice", options: periods, required: true, hint: "A second, minute or hour is the rate while active; a day, week or month is a total" },
  ],
  users: [
    num("count", "Users"),
    num("actions", "Actions each"),
    { key: "per", label: "Per", type: "choice", options: ["day", "week", "month"], required: true, hint: "A day counts only the active days" },
  ],
  concurrent: [num("users", "Users at a time"), num("everySeconds", "One action every", "Think time plus response time", "s")],
  batch: [
    num("items", "Items a run"),
    { key: "every", label: "Every", type: "string", required: true, hint: "rate(6 hours), cron(0 2 * * ? *), 0 */6 * * *" },
    num("withinSeconds", "Worked through within", "Sets the peak: items over this time", "s"),
  ],
};

/** When traffic of rate, users or concurrent comes. */
export const whenFields: Field[] = [
  { key: "hours", label: "Hours", type: "string", required: false, hint: "9-18 or 9-12,13-18; empty is all day" },
  { key: "days", label: "Days", type: "choice", options: ["all", "weekdays", "weekends"], required: false, default: "all" },
  { key: "peakFactor", label: "Peak factor", type: "number", required: false, hint: "Peak over the average while active: 2 for totals and users, 1 for rates and concurrent users" },
];

export const peakField: Field = { key: "peakPerSecond", label: "Peak", type: "number", unit: "/s", required: false, hint: "Set the peak outright" };

/**
 * What a field of the load moves: the monthly volume, which is the cost (the
 * peak follows it), or only the peak, which is the headroom. Every field moves
 * one of the two; this mirrors how traffic/traffic.go resolves a load.
 */
export type Effect = "volume" | "peak";

/** Fields of a part that set only the peak; the rest set the volume. */
export const peakOnlyParts: Partial<Record<keyof typeof partFields, string[]>> = { batch: ["withinSeconds"] };

/**
 * Hours and days set the volume of a rate while active (per second, minute or
 * hour) and of concurrent users, whose peak is their rate. Of a total, days set
 * the volume only per day, as a day counts only the active days; otherwise they
 * and the hours only spread the same volume, which moves the peak.
 */
export function whenEffect(key: string, t: Traffic): Effect {
  if (t.concurrent || (t.rate && t.rate.per in subDay)) return "volume";
  if (key === "days" && (t.rate ?? t.users)?.per === "day") return "volume";
  return "peak";
}

const subDay: Record<string, true> = { second: true, minute: true, hour: true };

/**
 * The declaration with every source of load at zero: entries, schedules,
 * batches and load put on any other node. What it costs is what the
 * architecture costs standing still, as declared; the rest of a total is
 * what the load brings. Mixed prices (an hourly charge plus one per GB) split
 * themselves this way, with no line classed as fixed or variable.
 */
export function withoutLoad(spec: Spec): Spec {
  const idle = { monthly: 0, peakPerSecond: 0 };
  return {
    ...spec,
    nodes: spec.nodes.map((n) => (n.type === "entry" || n.load || n.traffic ? { ...n, load: idle, traffic: undefined } : n)),
  };
}
