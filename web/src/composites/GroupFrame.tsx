import type { NodeProps } from "@xyflow/react";
import { usd } from "../lib/format";
import type { FrameNode } from "../lib/flow";

/**
 * A boundary drawn around its nodes. Only the label takes clicks (it selects
 * the group); the rest passes them to what is under it.
 */
export function GroupFrame({ data, width, height }: NodeProps<FrameNode>) {
  const { group, reading } = data;
  return (
    <div className="frame" style={{ width, height }}>
      <span className="frame-label">
        <span className="frame-kind">{group.kind}</span>
        {group.label ?? group.id}
        {reading && <span className="frame-cost">{reading.error ? "needs input" : usd(reading.monthlyUsd)}</span>}
      </span>
    </div>
  );
}
