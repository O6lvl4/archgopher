import type { CatalogEntry } from "../lib/types";

function groups(catalog: CatalogEntry[]): [string, CatalogEntry[]][] {
  const by = new Map<string, CatalogEntry[]>();
  for (const e of catalog) by.set(e.category, [...(by.get(e.category) ?? []), e]);
  return [...by.entries()].sort(([a], [b]) => a.localeCompare(b));
}

export function Catalog({ catalog, onAdd }: { catalog: CatalogEntry[]; onAdd: (type: string) => void }) {
  return (
    <nav className="catalog" aria-label="Scouters">
      {groups(catalog).map(([category, entries]) => (
        <section key={category}>
          <h3>{category}</h3>
          {entries.map((e) => (
            <button key={e.type} className="catalog-item" title={e.description} onClick={() => onAdd(e.type)}>
              <span className={`dot cat-${category.toLowerCase().replace(/[^a-z]/g, "")}`} />
              {e.label}
            </button>
          ))}
        </section>
      ))}
    </nav>
  );
}
