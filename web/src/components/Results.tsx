import { useState } from "react";
import { ms, num, pct, tone, usd } from "../format";
import type { Result } from "../types";

type Tab = "problems" | "costs" | "limits" | "paths" | "unverified";

interface Props {
  result: Result | undefined;
  error: string | undefined;
  warnings: string[];
  onSelect: (id: string) => void;
}

function problems(result: Result | undefined, error: string | undefined, warnings: string[]) {
  const out: { id?: string; text: string; tone: string }[] = [];
  if (error) out.push({ text: error, tone: "bad" });
  for (const n of result?.nodes ?? []) {
    if (n.error) out.push({ id: n.id, text: n.error, tone: "bad" });
    if (n.skipped) out.push({ id: n.id, text: n.skipped, tone: "muted" });
    if (n.stale) out.push({ id: n.id, text: `no longer in Terraform (${n.address ?? ""})`, tone: "warn" });
  }
  for (const w of [...(result?.warnings ?? []), ...warnings]) out.push({ text: w, tone: "warn" });
  return out;
}

function Problems(props: Props) {
  const list = problems(props.result, props.error, props.warnings);
  if (list.length === 0) return <p className="muted pad">Nothing to fix.</p>;
  return (
    <ul className="problems">
      {list.map((p, i) => (
        <li key={i} className={`tone-${p.tone}`}>
          {p.id && (
            <button className="link" onClick={() => p.id && props.onSelect(p.id)}>
              {p.id}
            </button>
          )}
          <span>{p.text}</span>
        </li>
      ))}
    </ul>
  );
}

function Costs({ result, onSelect }: Props) {
  const rows = (result?.nodes ?? []).flatMap((n) => (n.costs ?? []).map((c) => ({ id: n.id, ...c })));
  rows.sort((a, b) => (b.monthlyUsd ?? 0) - (a.monthlyUsd ?? 0));
  return (
    <table className="grid">
      <thead>
        <tr><th>Node</th><th>Component</th><th className="num">Quantity</th><th>Unit</th><th className="num">Monthly</th></tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i} onClick={() => onSelect(r.id)}>
            <td>{r.id}</td><td>{r.name}</td><td className="num">{num(r.quantity)}</td><td>{r.unit}</td><td className="num">{usd(r.monthlyUsd)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function Limits({ result, onSelect }: Props) {
  const rows = (result?.nodes ?? []).flatMap((n) => (n.limits ?? []).map((l) => ({ id: n.id, ...l })));
  rows.sort((a, b) => (a.headroom ?? 2) - (b.headroom ?? 2));
  return (
    <table className="grid">
      <thead>
        <tr><th>Node</th><th>Limit</th><th className="num">Peak demand</th><th className="num">Capacity</th><th>Unit</th><th className="num">Headroom</th></tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i} onClick={() => onSelect(r.id)}>
            <td>{r.id}</td><td>{r.name}</td><td className="num">{num(r.demand)}</td><td className="num">{num(r.capacity)}</td><td>{r.unit}</td>
            <td className={`num tone-${tone(r.headroom)}`}>{pct(r.headroom)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function Paths({ result }: Props) {
  return (
    <table className="grid">
      <thead>
        <tr><th>Path</th><th className="num">p50</th><th className="num">p99 (upper bound)</th><th className="num">Availability</th><th>Missing</th></tr>
      </thead>
      <tbody>
        {(result?.paths ?? []).map((p, i) => (
          <tr key={i}>
            <td>{p.nodes.join(" → ")}</td><td className="num">{ms(p.p50Ms)}</td><td className="num">{ms(p.p99Ms)}</td><td className="num">{pct(p.availability, 3)}</td>
            <td className="muted">{[...p.missingLatency.map((x) => `${x} latency`), ...p.missingSla.map((x) => `${x} SLA`)].join(", ")}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function Unverified({ result }: Props) {
  const rows = result?.unverified ?? [];
  if (rows.length === 0) return <p className="muted pad">Every value this graph reads has been checked against its source.</p>;
  return (
    <table className="grid">
      <thead>
        <tr><th>Book</th><th>Id</th><th>Region</th><th>State</th><th>Source</th></tr>
      </thead>
      <tbody>
        {rows.map((u) => (
          <tr key={`${u.book}-${u.id}`}>
            <td>{u.book}</td><td>{u.id}</td><td>{u.region}</td><td>{u.known ? "unverified" : "unknown"}</td>
            <td><a href={u.source} target="_blank" rel="noreferrer">source</a></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

const views: Record<Tab, (p: Props) => React.ReactNode> = { problems: Problems, costs: Costs, limits: Limits, paths: Paths, unverified: Unverified };

function tabCount(tab: Tab, p: Props): number {
  const r = p.result;
  const counts: Record<Tab, number> = {
    problems: problems(r, p.error, p.warnings).length,
    costs: (r?.nodes ?? []).reduce((s, n) => s + (n.costs ?? []).length, 0),
    limits: (r?.nodes ?? []).reduce((s, n) => s + (n.limits ?? []).length, 0),
    paths: (r?.paths ?? []).length,
    unverified: (r?.unverified ?? []).length,
  };
  return counts[tab];
}

export function Results(props: Props) {
  const [tab, setTab] = useState<Tab>("problems");
  const View = views[tab];
  return (
    <section className="results">
      <div className="tabs" role="tablist">
        {(Object.keys(views) as Tab[]).map((t) => (
          <button key={t} role="tab" aria-selected={tab === t} className={tab === t ? "active" : ""} onClick={() => setTab(t)}>
            {t} <span className="count">{tabCount(t, props)}</span>
          </button>
        ))}
      </div>
      <div className="tab-body">
        <View {...props} />
      </div>
    </section>
  );
}
