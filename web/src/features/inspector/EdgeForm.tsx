import type { Dispatch } from "react";
import { operations, withOperations } from "../../lib/ops";
import type { Action } from "../../lib/state";
import type { CatalogEntry, EdgeOp, SpecEdge } from "../../lib/types";
import { FieldInput } from "../../ui/FieldInput";

interface Props {
  edge: SpecEdge;
  index: number;
  target: CatalogEntry | undefined;
  dispatch: Dispatch<Action>;
}

const numberOf = (v: unknown) => (typeof v === "number" ? v : undefined);

/** One operation: what kind of work, how many per upstream unit, and how big each is. */
function OpRow({ op, kinds, onChange, onRemove }: { op: EdgeOp; kinds: string[]; onChange: (op: EdgeOp) => void; onRemove?: () => void }) {
  return (
    <div className="op-row">
      <FieldInput
        field={{ key: "kind", label: "Kind", type: "choice", options: kinds, default: kinds[0], required: false }}
        value={op.kind}
        onChange={(v) => onChange({ ...op, kind: typeof v === "string" ? v : undefined })}
      />
      <FieldInput
        field={{ key: "perUnit", label: "Per unit", type: "number", default: 1, required: false, hint: "0.1 is one in ten" }}
        value={op.perUnit}
        onChange={(v) => onChange({ ...op, perUnit: numberOf(v) })}
      />
      <FieldInput
        field={{ key: "kb", label: "Size", type: "number", unit: "KB", required: false, hint: "Data one operation moves; billing units round up by it" }}
        value={op.kb}
        onChange={(v) => onChange({ ...op, kb: numberOf(v) })}
      />
      {onRemove && (
        <button className="link op-remove" onClick={onRemove}>
          Remove
        </button>
      )}
    </div>
  );
}

export function EdgeForm({ edge, index, target, dispatch }: Props) {
  const update = (patch: Partial<SpecEdge>) => dispatch({ type: "updateEdge", index, patch });
  const kinds = target?.kinds ?? [];
  const ops = operations(edge);
  const setOps = (next: EdgeOp[]) => update(withOperations(next));
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
      <p className="panel-desc">
        What each unit of work at {edge.from} does to {edge.to}. Give each operation its size: {edge.to} counts its billing units by it, and a VPC the data that crosses zones.
      </p>
      <section className="panel-section">
        <h3>Operations</h3>
        {ops.map((op, i) => (
          <OpRow
            key={i}
            op={op}
            kinds={kinds}
            onChange={(o) => setOps(ops.map((x, j) => (j === i ? o : x)))}
            onRemove={ops.length > 1 ? () => setOps(ops.filter((_, j) => j !== i)) : undefined}
          />
        ))}
        <button className="link" onClick={() => setOps([...ops, { kind: kinds.find((k) => !ops.some((o) => o.kind === k)) ?? kinds[0] }])}>
          Add an operation
        </button>
      </section>
      <label className="field">
        <span className="field-label">Note</span>
        <textarea rows={3} value={edge.note ?? ""} onChange={(e) => update({ note: e.target.value || undefined })} />
      </label>
    </div>
  );
}
