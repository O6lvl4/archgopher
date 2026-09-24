import type { CatalogEntry } from "../lib/types";

const providerLabels: Record<string, string> = { "": "General", aws: "AWS", azure: "Azure", gcp: "Google Cloud", cloudflare: "Cloudflare" };

function byKey<T>(items: T[], key: (t: T) => string): [string, T[]][] {
  const by = new Map<string, T[]>();
  for (const it of items) by.set(key(it), [...(by.get(key(it)) ?? []), it]);
  return [...by.entries()];
}

/** Providers in a fixed order (neutral nodes first), categories sorted inside each. */
function groups(catalog: CatalogEntry[]): [string, [string, CatalogEntry[]][]][] {
  const order = Object.keys(providerLabels);
  return byKey(catalog, (e) => e.provider ?? "")
    .sort(([a], [b]) => (order.indexOf(a) + 1 || order.length + 1) - (order.indexOf(b) + 1 || order.length + 1))
    .map(([p, entries]) => [p, byKey(entries, (e) => e.category).sort(([a], [b]) => a.localeCompare(b))]);
}

export function Catalog({ catalog, onAdd }: { catalog: CatalogEntry[]; onAdd: (type: string) => void }) {
  return (
    <nav className="catalog" aria-label="Scouters">
      {groups(catalog).map(([provider, categories]) => (
        <section key={provider} className="catalog-provider">
          <h2>{providerLabels[provider] ?? provider}</h2>
          {categories.map(([category, entries]) => (
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
        </section>
      ))}
    </nav>
  );
}
