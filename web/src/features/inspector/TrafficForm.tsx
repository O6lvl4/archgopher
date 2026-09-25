import type { Dispatch } from "react";
import { num } from "../../lib/format";
import type { Action } from "../../lib/state";
import { partFields, peakField, shapeOf, shapes, switchTo, whenFields, type Shape } from "../../lib/traffic";
import type { Field, NodeResult, SpecNode, Traffic } from "../../lib/types";
import { FieldInput } from "../../ui/FieldInput";

interface Props {
  node: SpecNode;
  reading: NodeResult | undefined;
  dispatch: Dispatch<Action>;
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

function Fields({ fields, values, onChange }: { fields: Field[]; values: Record<string, unknown> | undefined; onChange: (key: string, v: unknown) => void }) {
  return (
    <>
      {fields.map((f) => (
        <FieldInput key={f.key} field={f} value={values?.[f.key]} onChange={(v) => onChange(f.key, v)} />
      ))}
    </>
  );
}

function VolumeFields({ node, dispatch }: Pick<Props, "node" | "dispatch">) {
  const load = node.load ?? { monthly: 0, peakPerSecond: 0 };
  const fields: Field[] = [
    { key: "monthly", label: "Monthly volume", type: "number", required: true, hint: "Drives cost" },
    { key: "peakPerSecond", label: "Peak per second", type: "number", required: true, hint: "Drives headroom" },
  ];
  return (
    <Fields
      fields={fields}
      values={load as unknown as Record<string, unknown>}
      onChange={(k, v) => dispatch({ type: "updateNode", id: node.id, patch: { load: { ...load, [k]: typeof v === "number" ? v : 0 } } })}
    />
  );
}

function TrafficFields({ shape, traffic, onChange }: { shape: Exclude<Shape, "load">; traffic: Traffic; onChange: (t: Traffic) => void }) {
  const set = (part: Part | undefined) => (key: string, v: unknown) => onChange(withKey(traffic, part, key, v));
  const scheduleField: Field = { key: "schedule", label: "Schedule", type: "string", required: true, hint: "rate(1 hour), cron(0 2 * * ? *), */15 9-17 * * 1-5" };
  const part = shape === "schedule" ? undefined : shape;
  const timed = shape === "rate" || shape === "users" || shape === "concurrent";
  return (
    <>
      {part ? (
        <Fields fields={partFields[part]} values={traffic[part] as Record<string, unknown>} onChange={set(part)} />
      ) : (
        <Fields fields={[scheduleField]} values={traffic as Record<string, unknown>} onChange={set(undefined)} />
      )}
      {timed && <Fields fields={whenFields} values={traffic as Record<string, unknown>} onChange={set(undefined)} />}
      <Fields fields={[peakField]} values={traffic as Record<string, unknown>} onChange={set(undefined)} />
    </>
  );
}

/** What the engine made of it: the load and the arithmetic, or why it could not. */
function Resolved({ reading }: { reading: NodeResult | undefined }) {
  const load = reading?.load;
  if (!load) return null;
  return (
    <p className="traffic-result">
      <strong>
        {num(load.monthly)} a month · peak {num(load.peakPerSecond)}/s
      </strong>
      {reading.loadBasis && <span className="traffic-basis">{reading.loadBasis}</span>}
    </p>
  );
}

/** The load a node brings in, said the way that fits: a volume, a rate, users, a schedule or batches. */
export function TrafficForm({ node, reading, dispatch }: Props) {
  const shape = shapeOf(node);
  const update = (patch: Partial<SpecNode>) => dispatch({ type: "updateNode", id: node.id, patch });
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
      <Resolved reading={reading} />
      {shape === "load" || !node.traffic ? (
        <VolumeFields node={node} dispatch={dispatch} />
      ) : (
        <TrafficFields shape={shape} traffic={node.traffic} onChange={(traffic) => update({ traffic })} />
      )}
      {node.type !== "entry" && (
        <button className="link" onClick={() => update({ load: undefined, traffic: undefined })}>
          Remove load
        </button>
      )}
    </section>
  );
}
