/** A small label over a value, colored by tone. */
export function Metric({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="metric">
      <span className="metric-label">{label}</span>
      <span className={`metric-value tone-${tone ?? "none"}`}>{value}</span>
    </div>
  );
}
