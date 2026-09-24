import { NodeResizer, type NodeProps } from "@xyflow/react";
import { usd } from "../lib/format";
import type { FrameNode } from "../lib/flow";

/**
 * A boundary drawn behind its cards. The label selects it and drags it with
 * its cards; when selected, the corners resize it. Everything else passes
 * clicks to what is under it.
 */
export function GroupFrame({ data, selected }: NodeProps<FrameNode>) {
  const { group, reading, onResized } = data;
  return (
    <div className={`frame${selected ? " selected" : ""}`}>
      <NodeResizer
        isVisible={selected}
        minWidth={200}
        minHeight={120}
        handleClassName="frame-handle"
        lineClassName="frame-line"
        onResizeEnd={(_, p) => onResized({ x: p.x, y: p.y, width: p.width, height: p.height })}
      />
      <span className="frame-label">
        <span className="frame-kind">{group.kind}</span>
        {group.label ?? group.id}
        {reading && <span className="frame-cost">{reading.error ? "needs input" : usd(reading.monthlyUsd)}</span>}
      </span>
    </div>
  );
}
