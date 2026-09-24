import { useEffect, useState, type Dispatch } from "react";
import { withValue, type Action } from "../../lib/state";
import type { CatalogEntry, Field, NodeResult, SpecNode, Values } from "../../lib/types";
import { FieldInput } from "../../ui/FieldInput";
import { NodeReadings } from "../../composites/NodeReadings";

interface Props {
  node: SpecNode;
  entry: CatalogEntry | undefined;
  reading: NodeResult | undefined;
  dispatch: Dispatch<Action>;
}

function IdInput({ node, dispatch }: Pick<Props, "node" | "dispatch">) {
  const [draft, setDraft] = useState(node.id);
  useEffect(() => setDraft(node.id), [node.id]);
  const commit = () => {
    if (draft !== node.id) dispatch({ type: "renameNode", id: node.id, to: draft.trim() });
  };
  return (
    <label className="field">
      <span className="field-label">Id</span>
      <input value={draft} onChange={(e) => setDraft(e.target.value)} onBlur={commit} onKeyDown={(e) => e.key === "Enter" && commit()} />
    </label>
  );
}

export function Fields({ title, fields, values, onChange }: { title: string; fields: Field[]; values: Values | undefined; onChange: (key: string, v: unknown) => void }) {
  if (fields.length === 0) return null;
  return (
    <section className="panel-section">
      <h3>{title}</h3>
      {fields.map((f) => (
        <FieldInput key={f.key} field={f} value={values?.[f.key]} onChange={(v) => onChange(f.key, v)} />
      ))}
    </section>
  );
}

function LoadFields({ node, dispatch }: Pick<Props, "node" | "dispatch">) {
  const load = node.load ?? { monthly: 0, peakPerSecond: 0 };
  const set = (key: "monthly" | "peakPerSecond", v: unknown) =>
    dispatch({ type: "updateNode", id: node.id, patch: { load: { ...load, [key]: typeof v === "number" ? v : 0 } } });
  const fields: Field[] = [
    { key: "monthly", label: "Monthly volume", type: "number", required: true, hint: "Drives cost" },
    { key: "peakPerSecond", label: "Peak per second", type: "number", required: true, hint: "Drives headroom" },
  ];
  return (
    <section className="panel-section">
      <h3>Load</h3>
      {fields.map((f) => (
        <FieldInput key={f.key} field={f} value={node.load?.[f.key as "monthly"]} onChange={(v) => set(f.key as "monthly", v)} />
      ))}
      {node.load && node.type !== "entry" && (
        <button className="link" onClick={() => dispatch({ type: "updateNode", id: node.id, patch: { load: undefined } })}>
          Remove load
        </button>
      )}
    </section>
  );
}

export function Problem({ reading }: { reading: NodeResult | undefined }) {
  const text = reading?.error ?? reading?.skipped;
  if (!text) return null;
  return <p className={`notice ${reading?.error ? "tone-bad" : "tone-muted"}`}>{text}</p>;
}

export function NodeForm({ node, entry, reading, dispatch }: Props) {
  const update = (patch: Partial<SpecNode>) => dispatch({ type: "updateNode", id: node.id, patch });
  const showLoad = node.type === "entry" || node.load !== undefined;
  return (
    <div className="panel-body">
      <header className="panel-head">
        <div>
          <div className="panel-kind">{entry?.label ?? node.type}</div>
          {node.address && <code className="panel-address">{node.address}</code>}
        </div>
        <button className="danger" onClick={() => dispatch({ type: "removeNode", id: node.id })}>
          Delete
        </button>
      </header>
      {entry && <p className="panel-desc">{entry.description}</p>}
      <Problem reading={reading} />
      <IdInput node={node} dispatch={dispatch} />
      {showLoad && <LoadFields node={node} dispatch={dispatch} />}
      <Fields title="From Terraform" fields={entry?.attributes ?? []} values={node.attributes} onChange={(k, v) => update({ attributes: withValue(node.attributes, k, v) })} />
      <Fields title="Assumptions" fields={entry?.assumptions ?? []} values={node.assumptions} onChange={(k, v) => update({ assumptions: withValue(node.assumptions, k, v) })} />
      <NodeReadings reading={reading} />
      <label className="field">
        <span className="field-label">Note</span>
        <textarea rows={3} value={node.note ?? ""} onChange={(e) => update({ note: e.target.value || undefined })} />
      </label>
      {!showLoad && (
        <button className="link" onClick={() => update({ load: { monthly: 0, peakPerSecond: 0 } })}>
          Add load at this node
        </button>
      )}
    </div>
  );
}
