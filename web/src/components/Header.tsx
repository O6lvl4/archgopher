import { usd } from "../format";
import type { Result } from "../types";

export interface HeaderActions {
  onExample: () => void;
  onNew: () => void;
  onOpen: (file: File) => void;
  onSave: () => void;
  onTerraform: () => void;
  onLayout: () => void;
}

function counts(result: Result | undefined) {
  const nodes = result?.nodes ?? [];
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
        <strong>arch-scouter</strong>
        <span className="muted">cost · headroom · latency · availability</span>
      </div>
      <div className="totals" aria-live="polite">
        <span className="total">{usd(result?.monthlyUsd ?? 0)}</span>
        <span className="muted">/ month</span>
        {c.over > 0 && <span className="chip tone-bad">{c.over} over limit</span>}
        {c.errors > 0 && <span className="chip tone-warn">{c.errors} need input</span>}
      </div>
      <div className="actions">
        <button onClick={actions.onTerraform}>Import Terraform</button>
        <FileButton onOpen={actions.onOpen} />
        <button onClick={actions.onSave}>Save YAML</button>
        <button onClick={actions.onLayout}>Tidy</button>
        <button onClick={actions.onExample}>Example</button>
        <button onClick={actions.onNew}>New</button>
      </div>
    </header>
  );
}
