import type { Dispatch } from "react";
import type { Action } from "../../lib/state";
import type { Spec } from "../../lib/types";

export function SpecForm({ spec, regions, dispatch }: { spec: Spec; regions: string[]; dispatch: Dispatch<Action> }) {
  return (
    <div className="panel-body">
      <header className="panel-head">
        <div className="panel-kind">Declaration</div>
      </header>
      <label className="field">
        <span className="field-label">Name</span>
        <input value={spec.name} onChange={(e) => dispatch({ type: "meta", name: e.target.value })} />
      </label>
      <label className="field">
        <span className="field-label">Region</span>
        <select value={spec.region} onChange={(e) => dispatch({ type: "meta", region: e.target.value })}>
          {regions.map((r) => (
            <option key={r}>{r}</option>
          ))}
        </select>
        <span className="field-hint">Prices and quotas are read for this region.</span>
      </label>
      <section className="panel-section">
        <h3>How to use</h3>
        <ul className="howto">
          <li>Import a Terraform folder, or add nodes from the catalog on the left.</li>
          <li>Drag from a node's right handle to another node to send load.</li>
          <li>Give the entry a load, then fill the assumptions marked required.</li>
          <li>Select a node or an edge to edit it. Delete removes the selection.</li>
        </ul>
      </section>
    </div>
  );
}
