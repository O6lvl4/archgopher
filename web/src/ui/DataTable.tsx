import type { ReactNode } from "react";

export interface Column<T> {
  label: string;
  cell: (row: T) => ReactNode;
  numeric?: boolean;
  tone?: (row: T) => string;
}

interface Props<T> {
  columns: Column<T>[];
  rows: T[];
  onRow?: (row: T) => void;
  empty?: string;
}

function className(c: Column<unknown>, row: unknown): string | undefined {
  const parts = [c.numeric ? "num" : "", c.tone ? `tone-${c.tone(row)}` : ""].filter(Boolean);
  return parts.length > 0 ? parts.join(" ") : undefined;
}

/** A sticky-header table; clicking a row calls onRow. */
export function DataTable<T>({ columns, rows, onRow, empty }: Props<T>) {
  if (rows.length === 0 && empty) return <p className="muted pad">{empty}</p>;
  const cols = columns as Column<unknown>[];
  return (
    <table className="grid">
      <thead>
        <tr>
          {cols.map((c) => (
            <th key={c.label} className={c.numeric ? "num" : undefined}>
              {c.label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, i) => (
          <tr key={i} onClick={onRow ? () => onRow(row) : undefined}>
            {cols.map((c) => (
              <td key={c.label} className={className(c, row)}>
                {c.cell(row)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
