import type { NodeProps } from "@xyflow/react";
import type { FrameNode } from "../lib/flow";

/** A boundary drawn around its nodes; clicks pass through to what is under it. */
export function GroupFrame({ data, width, height }: NodeProps<FrameNode>) {
  const { kind, label } = data.group;
  return (
    <div className="frame" style={{ width, height }}>
      <span className="frame-label">
        <span className="frame-kind">{kind}</span>
        {label}
      </span>
    </div>
  );
}
