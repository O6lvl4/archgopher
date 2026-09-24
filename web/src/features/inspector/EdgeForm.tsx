import type { Dispatch } from "react";
import type { Action } from "../../lib/state";
import type { CatalogEntry, SpecEdge } from "../../lib/types";
import { FieldInput } from "../../ui/FieldInput";

interface Props {
  edge: SpecEdge;
  index: number;
  target: CatalogEntry | undefined;
  dispatch: Dispatch<Action>;
}

export function EdgeForm({ edge, index, target, dispatch }: Props) {
  const update = (patch: Partial<SpecEdge>) => dispatch({ type: "updateEdge", index, patch });
  const kinds = target?.kinds ?? [];
  return (
    <div className="panel-body">
      <header className="panel-head">
        <div>
          <div className="panel-kind">Edge</div>
          <code className="panel-address">
            {edge.from} → {edge.to}
          </code>
        </div>
        <button className="danger" onClick={() => dispatch({ type: "removeEdge", index })}>
          Delete
        </button>
      </header>
      <p className="panel-desc">Each unit of work at {edge.from} sends this many units to {edge.to}.</p>
      <FieldInput
        field={{ key: "kind", label: "Kind of work", type: "choice", options: kinds, default: kinds[0], required: false }}
        value={edge.kind}
        onChange={(v) => update({ kind: typeof v === "string" ? v : undefined })}
      />
      <FieldInput
        field={{ key: "perUnit", label: "Per upstream unit", type: "number", default: 1, required: false, hint: "0.2 means one in five calls" }}
        value={edge.perUnit}
        onChange={(v) => update({ perUnit: typeof v === "number" ? v : undefined })}
      />
      <label className="field">
        <span className="field-label">Note</span>
        <textarea rows={3} value={edge.note ?? ""} onChange={(e) => update({ note: e.target.value || undefined })} />
      </label>
    </div>
  );
}
