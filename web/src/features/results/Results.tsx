import { useState } from "react";
import { ms, num, pct, tone, usd } from "../../lib/format";
import type { Cost, Limit, NodeResult, PathResult, RefUse, Result } from "../../lib/types";
import { DataTable, type Column } from "../../ui/DataTable";

type Tab = "problems" | "costs" | "limits" | "paths" | "unverified";

interface Props {
  result: Result | undefined;
  error: string | undefined;
  warnings: string[];
  onSelect: (id: string) => void;
}

interface Problem {
  id?: string;
  text: string;
  tone: string;
}

/** Leaf results only: a pattern's rolled-up row repeats its members' lines. */
function leaves(r: Result | undefined): NodeResult[] {
  return (r?.nodes ?? []).filter((n) => !n.members);
}

function problems({ result, error, warnings }: Props): Problem[] {
  const out: Problem[] = error ? [{ text: error, tone: "bad" }] : [];
  for (const n of leaves(result)) {
    if (n.error) out.push({ id: n.id, text: n.error, tone: "bad" });
    if (n.skipped) out.push({ id: n.id, text: n.skipped, tone: "muted" });
    if (n.stale) out.push({ id: n.id, text: `no longer in Terraform (${n.address ?? ""})`, tone: "warn" });
  }
  for (const w of [...(result?.warnings ?? []), ...warnings]) out.push({ text: w, tone: "warn" });
  return out;
}

/** Member ids look like "orders/fn"; selecting one selects the pattern node on the canvas. */
const owner = (id: string) => id.split("/")[0] ?? id;

type CostRow = Cost & { id: string };
type LimitRow = Limit & { id: string };

const costColumns: Column<CostRow>[] = [
  { label: "Node", cell: (r) => r.id },
  { label: "Component", cell: (r) => r.name },
  { label: "Quantity", cell: (r) => num(r.quantity), numeric: true },
  { label: "Unit", cell: (r) => r.unit },
  { label: "Monthly", cell: (r) => usd(r.monthlyUsd), numeric: true },
];

const limitColumns: Column<LimitRow>[] = [
  { label: "Node", cell: (r) => r.id },
  { label: "Limit", cell: (r) => r.name },
  { label: "Peak demand", cell: (r) => num(r.demand), numeric: true },
  { label: "Capacity", cell: (r) => num(r.capacity), numeric: true },
  { label: "Unit", cell: (r) => r.unit },
  { label: "Headroom", cell: (r) => pct(r.headroom), numeric: true, tone: (r) => tone(r.headroom) },
];

const pathColumns: Column<PathResult>[] = [
  { label: "Path", cell: (p) => p.nodes.join(" → ") },
  { label: "p50", cell: (p) => ms(p.p50Ms), numeric: true },
  { label: "p99 (upper bound)", cell: (p) => ms(p.p99Ms), numeric: true },
  { label: "Availability", cell: (p) => pct(p.availability, 3), numeric: true },
  { label: "Missing", cell: (p) => [...p.missingLatency.map((x) => `${x} latency`), ...p.missingSla.map((x) => `${x} SLA`)].join(", ") },
];

const refColumns: Column<RefUse>[] = [
  { label: "Book", cell: (u) => u.book },
  { label: "Id", cell: (u) => u.id },
  { label: "Region", cell: (u) => u.region },
  { label: "State", cell: (u) => (u.known ? "unverified" : "unknown") },
  { label: "Source", cell: (u) => <a href={u.source} target="_blank" rel="noreferrer">source</a> },
];

function costs(r: Result | undefined): CostRow[] {
  const rows = leaves(r).flatMap((n) => (n.costs ?? []).map((c) => ({ id: n.id, ...c })));
  return rows.sort((a, b) => (b.monthlyUsd ?? 0) - (a.monthlyUsd ?? 0));
}

function limits(r: Result | undefined): LimitRow[] {
  const rows = leaves(r).flatMap((n) => (n.limits ?? []).map((l) => ({ id: n.id, ...l })));
  return rows.sort((a, b) => (a.headroom ?? 2) - (b.headroom ?? 2));
}

function Problems(props: Props) {
  const list = problems(props);
  if (list.length === 0) return <p className="muted pad">Nothing to fix.</p>;
  return (
    <ul className="problems">
      {list.map((p, i) => (
        <li key={i} className={`tone-${p.tone}`}>
          {p.id && (
            <button className="link" onClick={() => p.id && props.onSelect(owner(p.id))}>
              {p.id}
            </button>
          )}
          <span>{p.text}</span>
        </li>
      ))}
    </ul>
  );
}

const views: Record<Tab, { count: (p: Props) => number; view: (p: Props) => React.ReactNode }> = {
  problems: { count: (p) => problems(p).length, view: (p) => <Problems {...p} /> },
  costs: { count: (p) => costs(p.result).length, view: (p) => <DataTable columns={costColumns} rows={costs(p.result)} onRow={(r) => p.onSelect(owner(r.id))} /> },
  limits: { count: (p) => limits(p.result).length, view: (p) => <DataTable columns={limitColumns} rows={limits(p.result)} onRow={(r) => p.onSelect(owner(r.id))} /> },
  paths: { count: (p) => (p.result?.paths ?? []).length, view: (p) => <DataTable columns={pathColumns} rows={p.result?.paths ?? []} /> },
  unverified: {
    count: (p) => (p.result?.unverified ?? []).length,
    view: (p) => <DataTable columns={refColumns} rows={p.result?.unverified ?? []} empty="Every value this graph reads has been checked against its source." />,
  },
};

export function Results(props: Props) {
  const [tab, setTab] = useState<Tab>("problems");
  return (
    <section className="results">
      <div className="tabs" role="tablist">
        {(Object.keys(views) as Tab[]).map((t) => (
          <button key={t} role="tab" aria-selected={tab === t} className={tab === t ? "active" : ""} onClick={() => setTab(t)}>
            {t} <span className="count">{views[t].count(props)}</span>
          </button>
        ))}
      </div>
      <div className="tab-body">{views[tab].view(props)}</div>
    </section>
  );
}
