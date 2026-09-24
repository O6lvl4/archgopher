import type { NodeProps } from "@xyflow/react";
import { usd } from "../lib/format";
import type { FrameNode } from "../lib/flow";

/**
 * A boundary drawn behind its cards, sized by the layout. The label selects
 * it; everything else passes clicks to what is under it.
 */
export function GroupFrame({ data, selected }: NodeProps<FrameNode>) {
  const { group, reading } = data;
  return (
    <div className={`frame${selected ? " selected" : ""}`}>
      <span className="frame-label">
        <span className="frame-kind">{group.kind}</span>
        {group.label ?? group.id}
        {reading && <span className="frame-cost">{reading.error ? "needs input" : usd(reading.monthlyUsd)}</span>}
      </span>
    </div>
  );
}
