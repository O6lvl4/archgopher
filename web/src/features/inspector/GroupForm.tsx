import type { Dispatch } from "react";
import { withValue, type Action } from "../../lib/state";
import type { CatalogEntry, NodeResult, SpecGroup } from "../../lib/types";
import { NodeReadings } from "../../composites/NodeReadings";
import { Fields, Problem } from "./NodeForm";

interface Props {
  group: SpecGroup;
  entry: CatalogEntry | undefined;
  reading: NodeResult | undefined;
  dispatch: Dispatch<Action>;
}

/** Per-hop latency belongs to nodes on a path; a boundary is not a hop. */
const hopFields = new Set(["latencyP50Ms", "latencyP99Ms"]);

/** A boundary: its numbers and what it reads from the traffic between its nodes. */
export function GroupForm({ group, entry, reading, dispatch }: Props) {
  return (
    <div className="panel-body">
      <header className="panel-head">
        <div>
          <div className="panel-kind">{group.kind}</div>
          <code className="panel-address">{group.label ?? group.id}</code>
        </div>
      </header>
      {entry && <p className="panel-desc">{entry.description}</p>}
      <Problem reading={reading} />
      <Fields
        title="Assumptions"
        fields={(entry?.assumptions ?? []).filter((f) => !hopFields.has(f.key))}
        values={group.assumptions}
        onChange={(k, v) => dispatch({ type: "updateGroup", id: group.id, patch: { assumptions: withValue(group.assumptions, k, v) } })}
      />
      {reading ? <NodeReadings reading={reading} /> : <p className="panel-desc">No edge between two nodes inside carries kb yet, so nothing is read.</p>}
    </div>
  );
}
