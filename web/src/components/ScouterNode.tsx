import { Handle, Position, type NodeProps } from "@xyflow/react";
import { ms, num, pct, tone, usd } from "../format";
import type { CardNode } from "../flow";
import type { NodeResult } from "../types";

function minHeadroom(r: NodeResult | undefined): number | null {
  const known = (r?.limits ?? []).map((l) => l.headroom).filter((h): h is number => h !== null);
  return known.length > 0 ? Math.min(...known) : null;
}

function status(r: NodeResult | undefined, stale: boolean | undefined): { label: string; tone: string } | undefined {
  if (r?.error) return { label: "error", tone: "bad" };
  if (r?.skipped) return { label: "not read", tone: "muted" };
  if (stale) return { label: "stale", tone: "warn" };
  const h = minHeadroom(r);
  if (h !== null && h < 0) return { label: "over limit", tone: "bad" };
  return undefined;
}

function Metric({ label, value, toneName }: { label: string; value: string; toneName?: string }) {
  return (
    <div className="metric">
      <span className="metric-label">{label}</span>
      <span className={`metric-value tone-${toneName ?? "none"}`}>{value}</span>
    </div>
  );
}

function Readings({ reading }: { reading: NodeResult | undefined }) {
  const headroom = minHeadroom(reading);
  const sla = reading?.sla?.value;
  return (
    <div className="card-metrics">
      <Metric label="Monthly" value={usd(reading?.monthlyUsd ?? 0)} />
      <Metric label="Headroom" value={pct(headroom, 0)} toneName={tone(headroom)} />
      <Metric label="p99" value={ms(reading?.latency?.p99Ms)} />
      <Metric label="SLA" value={sla === undefined || sla === null ? "–" : pct(sla, 2)} />
    </div>
  );
}

/** An entry has nothing to read; it shows the load it sends instead. */
function EntryLoad({ node }: { node: CardNode["data"]["node"] }) {
  return (
    <div className="card-metrics">
      <Metric label="Per month" value={node.load ? num(node.load.monthly) : "not set"} toneName={node.load ? undefined : "bad"} />
      <Metric label="Peak / s" value={node.load ? num(node.load.peakPerSecond) : "not set"} toneName={node.load ? undefined : "bad"} />
    </div>
  );
}

export function ScouterNode({ data, selected }: NodeProps<CardNode>) {
  const { node, reading, entry } = data;
  const s = status(reading, node.stale);
  return (
    <div className={`card cat-${(entry?.category ?? "other").toLowerCase().replace(/[^a-z]/g, "")}${selected ? " selected" : ""}`}>
      <Handle type="target" position={Position.Left} />
      <div className="card-head">
        <span className="card-kind">{entry?.label ?? node.type}</span>
        {s && <span className={`chip tone-${s.tone}`}>{s.label}</span>}
      </div>
      <div className="card-id" title={node.address ?? node.id}>
        {node.id}
      </div>
      {node.type === "entry" ? <EntryLoad node={node} /> : <Readings reading={reading} />}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
