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

/** Lucide glyphs (ISC), drawn in the current text color. Notice: licenses/lucide.txt */
const glyphs = {
  "file-plus": [
    "M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2z",
    "M14 2v5a1 1 0 0 0 1 1h5",
    "M9 15h6",
    "M12 18v-6",
  ],
  "folder-open": [
    "m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2",
  ],
  save: [
    "M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z",
    "M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7",
    "M7 3v4a1 1 0 0 0 1 1h7",
  ],
  import: ["M12 3v12", "m8 11 4 4 4-4", "M8 5H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-4"],
  "chevron-down": ["m6 9 6 6 6-6"],
};

function Glyph({ name }: { name: keyof typeof glyphs }) {
  return (
    <svg className="glyph" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      {glyphs[name].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}

function FileButton({ onOpen }: { onOpen: (file: File) => void }) {
  return (
    <label className="button" title="Open a .scouter.yaml file">
      <Glyph name="folder-open" />
      Open
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

function ExamplePicker({ onExample }: { onExample: (name: string) => void }) {
  return (
    <label className="button picker">
      Examples
      <Glyph name="chevron-down" />
      <select value="" aria-label="Open an example" onChange={(e) => e.target.value && onExample(e.target.value)}>
        <option value="" disabled>
          Open an example
        </option>
        {exampleNames.map((n) => (
          <option key={n}>{n}</option>
        ))}
      </select>
    </label>
  );
}

export function Header({ result, actions }: { result: Result | undefined; actions: HeaderActions }) {
  const c = counts(result);
  return (
    <header className="topbar">
      <div className="brand">
        <img className="brand-mark" src={`${import.meta.env.BASE_URL}mark.png`} alt="" width="32" height="32" />
        <div className="brand-text">
          <strong>archgopher</strong>
          <span className="muted">cost · headroom · latency · availability</span>
        </div>
      </div>
      <div className="totals" aria-live="polite">
        <span className="totals-label">Monthly estimate</span>
        <div className="totals-row">
          <span className="total">{usd(result?.monthlyUsd ?? 0)}</span>
          <span className="muted">/ mo</span>
          {c.over > 0 && <Chip tone="bad">{c.over} over limit</Chip>}
          {c.errors > 0 && <Chip tone="warn">{c.errors} need input</Chip>}
        </div>
      </div>
      <nav className="actions" aria-label="Document">
        <div className="button-group" role="group" aria-label="File">
          <button onClick={actions.onNew} title="Start an empty declaration">
            <Glyph name="file-plus" />
            New
          </button>
          <FileButton onOpen={actions.onOpen} />
          <button onClick={actions.onSave} title="Download as .scouter.yaml">
            <Glyph name="save" />
            Save
          </button>
        </div>
        <ExamplePicker onExample={actions.onExample} />
        <span className="actions-divider" aria-hidden="true" />
        <button className="primary" onClick={actions.onTerraform} title="Read .tf files from a folder">
          <Glyph name="import" />
          Import Terraform
        </button>
      </nav>
    </header>
  );
}
