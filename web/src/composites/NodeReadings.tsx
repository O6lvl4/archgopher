import { num, pct, tone, usd } from "../lib/format";
import type { NodeResult } from "../lib/types";

export function NodeReadings({ reading }: { reading: NodeResult | undefined }) {
  const costs = reading?.costs ?? [];
  const limits = reading?.limits ?? [];
  if (costs.length === 0 && limits.length === 0) return null;
  return (
    <section className="panel-section">
      <h3>Readings</h3>
      <table className="mini">
        <tbody>
          {costs.map((c) => (
            <tr key={`c-${c.name}`}>
              <td title={c.pool ? "Billed on the whole account's usage: this line pays the pool's average price" : undefined}>
                {c.pool ? `${c.name} (shared)` : c.name}
              </td>
              <td className="num">
                {num(c.quantity)} {c.unit}
              </td>
              <td className="num">{usd(c.monthlyUsd)}</td>
            </tr>
          ))}
          {limits.map((l) => (
            <tr key={`l-${l.name}`}>
              <td>{l.name}</td>
              <td className="num">
                {num(l.demand)} / {num(l.capacity)}
              </td>
              <td className={`num tone-${tone(l.headroom)}`}>{pct(l.headroom)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
