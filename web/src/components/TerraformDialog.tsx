import { useState } from "react";
import { engine } from "../engine";

const wanted = /(\.tf|\.tfvars|\.tfvars\.json|modules\.json)$/;

function readTree(list: FileList): Promise<Record<string, string>> {
  const reads = [...list].filter((f) => wanted.test(f.name)).map((f) => f.text().then((text) => [f.webkitRelativePath || f.name, text] as const));
  return Promise.all(reads).then((pairs) => Object.fromEntries(pairs));
}

/** Prefer a directory with a provider block that is not a module. */
function guessRoot(roots: string[], files: Record<string, string>): string {
  const withProvider = roots.filter((r) => Object.entries(files).some(([p, src]) => p.startsWith(`${r}/`) && !p.slice(r.length + 1).includes("/") && /provider\s+"aws"/.test(src)));
  return withProvider.find((r) => !r.includes("/modules/")) ?? roots[0] ?? "";
}

function parseVars(text: string): Record<string, string> {
  const vars: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const i = line.indexOf("=");
    if (i > 0) vars[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return vars;
}

export interface ImportRequest {
  files: Record<string, string>;
  root: string;
  vars: Record<string, string>;
  merge: boolean;
}

export function TerraformDialog({ canMerge, onImport, onClose }: { canMerge: boolean; onImport: (r: ImportRequest) => void; onClose: () => void }) {
  const [files, setFiles] = useState<Record<string, string>>({});
  const [roots, setRoots] = useState<string[]>([]);
  const [root, setRoot] = useState("");
  const [vars, setVars] = useState("");
  const [merge, setMerge] = useState(canMerge);
  const [error, setError] = useState<string>();
  const pick = (list: FileList | null) => {
    if (!list) return;
    readTree(list)
      .then((tree) => {
        const found = engine.roots(tree);
        setFiles(tree);
        setRoots(found);
        setRoot(guessRoot(found, tree));
        setError(found.length === 0 ? "No .tf files in that folder." : undefined);
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };
  return (
    <div className="dialog-backdrop" role="dialog" aria-modal="true" aria-label="Import Terraform">
      <div className="dialog">
        <h2>Import Terraform</h2>
        <p className="muted">
          Pick the folder that holds your Terraform, including the modules it uses. The files are read in this browser; nothing is uploaded.
        </p>
        <label className="field">
          <span className="field-label">Folder</span>
          <input type="file" multiple {...{ webkitdirectory: "" }} onChange={(e) => pick(e.target.files)} />
          <span className={`field-hint${error ? " tone-bad" : ""}`}>{error ?? `${Object.keys(files).length} Terraform files read`}</span>
        </label>
        {roots.length > 0 && (
          <label className="field">
            <span className="field-label">Root module</span>
            <select value={root} onChange={(e) => setRoot(e.target.value)}>
              {roots.map((r) => (
                <option key={r}>{r}</option>
              ))}
            </select>
          </label>
        )}
        <label className="field">
          <span className="field-label">Variables</span>
          <textarea rows={3} placeholder={"env=prod\nenable_cleanup=true"} value={vars} onChange={(e) => setVars(e.target.value)} />
          <span className="field-hint">One name=value per line, for variables without a default or tfvars.</span>
        </label>
        {canMerge && (
          <label className="check">
            <input type="checkbox" checked={merge} onChange={(e) => setMerge(e.target.checked)} />
            Merge into the current declaration, keeping its assumptions, load and edges
          </label>
        )}
        <div className="dialog-actions">
          <button onClick={onClose}>Cancel</button>
          <button className="primary" disabled={!root} onClick={() => onImport({ files, root, vars: parseVars(vars), merge })}>
            Import
          </button>
        </div>
      </div>
    </div>
  );
}
