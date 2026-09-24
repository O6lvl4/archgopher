/** A short status label. */
export function Chip({ tone, children }: { tone: string; children: React.ReactNode }) {
  return <span className={`chip tone-${tone}`}>{children}</span>;
}
