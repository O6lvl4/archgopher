import type { Dispatch } from "react";
import { num, usd } from "../../lib/format";
import type { Action } from "../../lib/state";
import { partFields, peakField, peakOnlyParts, shapeOf, shapes, switchTo, whenEffect, whenFields, type Effect, type Shape } from "../../lib/traffic";
import type { Field, NodeResult, SpecNode, Traffic } from "../../lib/types";
import { FieldInput } from "../../ui/FieldInput";
import { useFlash } from "../../ui/useFlash";

/** The declaration's monthly total and what of it comes with no load at all. */
export interface Split {
  total: number;
  idle: number;
}

interface Props {
  node: SpecNode;
  reading: NodeResult | undefined;
  split?: Split;
  dispatch: Dispatch<Action>;
}

/** Why a big change of load can barely move the total: most of it may not follow the load. */
function LoadShare({ split }: { split: Split | undefined }) {
  if (!split || split.total <= 0 || split.total - split.idle < -0.005) return null;
  const fromLoad = Math.max(split.total - split.idle, 0);
  return (
    <p className="load-share">
      Of {usd(split.total)} a month, {usd(split.idle)} comes with no load at all; the load moves the other {usd(fromLoad)}.
    </p>
  );
}

type Part = "rate" | "users" | "concurrent" | "batch";

/** Sets one key of the traffic, or of one of its parts; undefined removes it. */
function withKey(t: Traffic, part: Part | undefined, key: string, v: unknown): Traffic {
  if (!part) {
    const next = { ...t, [key]: v === "" ? undefined : v };
    if (v === undefined || v === "") delete (next as Record<string, unknown>)[key];
    return next;
  }
  return { ...t, [part]: { ...(t[part] as object), [key]: v } };
}

/** A field with where its value comes from and goes to. */
interface Row {
  field: Field;
  value: unknown;
  onChange: (v: unknown) => void;
}

const groups: Record<Effect, { title: string; says: string }> = {
  volume: { title: "Volume", says: "Sets the monthly cost; the peak follows it" },
  peak: { title: "Peak", says: "Moves only the peak and headroom, not the cost" },
};

/** The fields that move one thing, under a heading that says which and what it is now. */
function Group({ effect, rows, now, reset }: { effect: Effect; rows: Row[]; now: string | undefined; reset: string }) {
  const { flash, tick } = useFlash(now, reset);
  if (rows.length === 0) return null;
  const g = groups[effect];
  return (
    <div className={`load-group effect-${effect}`}>
      <div className="load-group-head">
        <span className="load-group-title">
          {g.title}
          {now && (
            <span key={tick} className={`load-group-now${flash ? " flash" : ""}`}>
              {now}
            </span>
          )}
        </span>
        <span className="load-group-says">{g.says}</span>
      </div>
      {rows.map((r) => (
        <FieldInput key={r.field.key} field={r.field} value={r.value} onChange={r.onChange} />
      ))}
    </div>
  );
}

function volumeRows(node: SpecNode, dispatch: Dispatch<Action>): Record<Effect, Row[]> {
  const load = node.load ?? { monthly: 0, peakPerSecond: 0 };
  const row = (field: Field): Row => ({
    field,
    value: load[field.key as keyof typeof load],
    onChange: (v) => dispatch({ type: "updateNode", id: node.id, patch: { load: { ...load, [field.key]: typeof v === "number" ? v : 0 } } }),
  });
  return {
    volume: [row({ key: "monthly", label: "Monthly volume", type: "number", required: true })],
    peak: [row({ key: "peakPerSecond", label: "Peak per second", type: "number", required: true })],
  };
}

function trafficRows(shape: Exclude<Shape, "load">, traffic: Traffic, onChange: (t: Traffic) => void): Record<Effect, Row[]> {
  const rows: Record<Effect, Row[]> = { volume: [], peak: [] };
  const add = (effect: Effect, part: Part | undefined, field: Field) => {
    const values = (part ? traffic[part] : traffic) as Record<string, unknown> | undefined;
    rows[effect].push({ field, value: values?.[field.key], onChange: (v) => onChange(withKey(traffic, part, field.key, v)) });
  };
  if (shape === "schedule") {
    add("volume", undefined, { key: "schedule", label: "Schedule", type: "string", required: true, hint: "rate(1 hour), cron(0 2 * * ? *), */15 9-17 * * 1-5" });
  } else {
    for (const f of partFields[shape]) add(peakOnlyParts[shape]?.includes(f.key) ? "peak" : "volume", shape, f);
  }
  if (shape === "rate" || shape === "users" || shape === "concurrent") {
    for (const f of whenFields) add(f.key === "peakFactor" ? "peak" : whenEffect(f.key, traffic), undefined, f);
  }
  add("peak", undefined, peakField);
  return rows;
}

/** What the engine made of it: the load and the arithmetic, or why it could not. */
function Resolved({ node, reading }: { node: SpecNode; reading: NodeResult | undefined }) {
  const load = reading?.load;
  const said = load ? `${num(load.monthly)} a month · peak ${num(load.peakPerSecond)}/s` : undefined;
  const { flash, tick } = useFlash(said, node.id);
  if (!load) return null;
  return (
    <p key={tick} className={`traffic-result${flash ? " flash" : ""}`} aria-live="polite">
      <strong>{said}</strong>
      {reading.loadBasis && <span className="traffic-basis">{reading.loadBasis}</span>}
    </p>
  );
}

/** The load a node brings in, said the way that fits: a volume, a rate, users, a schedule or batches. */
export function TrafficForm({ node, reading, split, dispatch }: Props) {
  const shape = shapeOf(node);
  const update = (patch: Partial<SpecNode>) => dispatch({ type: "updateNode", id: node.id, patch });
  const rows = shape === "load" || !node.traffic ? volumeRows(node, dispatch) : trafficRows(shape, node.traffic, (traffic) => update({ traffic }));
  const load = reading?.load;
  return (
    <section className="panel-section">
      <h3>Load</h3>
      <label className="field">
        <span className="field-label">Said as</span>
        <select value={shape} onChange={(e) => update(switchTo(e.target.value as Shape, node))}>
          {shapes.map((s) => (
            <option key={s.shape} value={s.shape}>
              {s.label}
            </option>
          ))}
        </select>
        <span className="field-hint">{shapes.find((s) => s.shape === shape)?.hint}</span>
      </label>
      <Resolved node={node} reading={reading} />
      <LoadShare split={split} />
      <Group effect="volume" rows={rows.volume} now={load && `${num(load.monthly)} a month`} reset={node.id} />
      <Group effect="peak" rows={rows.peak} now={load && `${num(load.peakPerSecond)}/s`} reset={node.id} />
      {node.type !== "entry" && (
        <button className="link" onClick={() => update({ load: undefined, traffic: undefined })}>
          Remove load
        </button>
      )}
    </section>
  );
}
