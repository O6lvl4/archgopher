import { usd } from "../../lib/format";
import { exampleNames } from "../../lib/examples";
import type { Result } from "../../lib/types";
import { Chip } from "../../ui/Chip";

export interface HeaderActions {
  onExample: (name: string) => void;
  onNew: () => void;
  onOpen: (file: File) => void;
  onSave: () => void;
  onTerraform: () => void;
}

function counts(result: Result | undefined) {
  const nodes = (result?.nodes ?? []).filter((n) => !n.members);
  const over = nodes.filter((n) => (n.limits ?? []).some((l) => l.headroom !== null && l.headroom < 0)).length;
  const errors = nodes.filter((n) => n.error).length;
  return { over, errors };
}

function FileButton({ onOpen }: { onOpen: (file: File) => void }) {
  return (
    <label className="button">
      Open YAML
      <input
        type="file"
        accept=".yaml,.yml"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) onOpen(f);
          e.target.value = "";
        }}
      />
    </label>
  );
}

export function Header({ result, actions }: { result: Result | undefined; actions: HeaderActions }) {
  const c = counts(result);
  return (
    <header className="topbar">
      <div className="brand">
        <strong>archgopher</strong>
        <span className="muted">cost · headroom · latency · availability</span>
      </div>
      <div className="totals" aria-live="polite">
        <span className="total">{usd(result?.monthlyUsd ?? 0)}</span>
        <span className="muted">/ month</span>
        {c.over > 0 && <Chip tone="bad">{c.over} over limit</Chip>}
        {c.errors > 0 && <Chip tone="warn">{c.errors} need input</Chip>}
      </div>
      <div className="actions">
        <button onClick={actions.onTerraform}>Import Terraform</button>
        <FileButton onOpen={actions.onOpen} />
        <button onClick={actions.onSave}>Save YAML</button>
        <select className="button" value="" aria-label="Open an example" onChange={(e) => e.target.value && actions.onExample(e.target.value)}>
          <option value="">Examples…</option>
          {exampleNames.map((n) => (
            <option key={n}>{n}</option>
          ))}
        </select>
        <button onClick={actions.onNew}>New</button>
      </div>
    </header>
  );
}
