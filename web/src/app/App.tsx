import { useEffect, useState } from "react";
import { Canvas } from "../features/canvas/Canvas";
import { Catalog } from "../composites/Catalog";
import { Header } from "../features/header/Header";
import { Inspector } from "../features/inspector/Inspector";
import { Results } from "../features/results/Results";
import { TerraformDialog, type ImportRequest } from "../features/terraform/TerraformDialog";
import { download, slug } from "../lib/download";
import { engine } from "../lib/engine";
import { examples } from "../lib/examples";
import { NEW_FRAME } from "../lib/frames";
import { freshGroupId, freshId, type Selection } from "../lib/state";
import type { CatalogEntry } from "../lib/types";
import { message, useWorkspace } from "./workspace";

function useNotice() {
  const [notice, setNotice] = useState<string>();
  useEffect(() => {
    if (!notice) return;
    const t = window.setTimeout(() => setNotice(undefined), 5000);
    return () => window.clearTimeout(t);
  }, [notice]);
  return [notice, setNotice] as const;
}

export function App() {
  const ws = useWorkspace();
  const [selection, setSelection] = useState<Selection>();
  const [importing, setImporting] = useState(false);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [notice, setNotice] = useNotice();

  const replace = (run: () => void) => {
    try {
      run();
      setSelection(undefined);
    } catch (e) {
      setNotice(message(e));
    }
  };

  /** A boundary from the catalog is an empty frame; cards dropped in it join it. The layout places both. */
  const addGroup = (entry: CatalogEntry) => {
    const id = freshGroupId(ws.spec, entry.label);
    ws.dispatch({ type: "addGroup", group: { id, kind: entry.label, label: id, type: entry.type, size: NEW_FRAME } });
    setSelection({ kind: "group", id });
  };

  const addNode = (type: string) => {
    const entry = ws.catalogMap.get(type);
    if (entry?.boundary) return addGroup(entry);
    const id = freshId(ws.spec, entry?.label ?? type);
    ws.dispatch({ type: "addNode", node: { id, type } });
    setSelection({ kind: "node", id });
  };

  const onImport = (r: ImportRequest) =>
    replace(() => {
      const res = engine.terraform({ files: r.files, root: r.root, vars: r.vars, merge: r.merge ? ws.spec : undefined });
      ws.load(res.spec);
      setWarnings(res.warnings);
      setImporting(false);
    });

  const actions = {
    onExample: (name: string) => {
      const yaml = examples[name];
      if (yaml) replace(() => ws.load(engine.parseYaml(yaml)));
    },
    onNew: () => replace(() => ws.load({ name: "Untitled", region: ws.spec.region, nodes: [{ id: "users", type: "entry" }], edges: [] })),
    onOpen: (f: File) => {
      f.text()
        .then((text) => replace(() => ws.load(engine.parseYaml(text))))
        .catch((e: unknown) => setNotice(message(e)));
    },
    onSave: () => replace(() => download(`${slug(ws.spec.name)}.scouter.yaml`, engine.toYaml(ws.spec))),
    onTerraform: () => setImporting(true),
  };

  if (ws.fatal) return <p className="fatal">The engine did not load: {ws.fatal}</p>;
  if (!ws.ready) return <p className="loading">Loading the engine…</p>;
  return (
    <div className="app">
      <Header result={ws.result} actions={actions} />
      <Catalog catalog={ws.catalog} onAdd={addNode} />
      <main className="canvas">
        <Canvas
          key={ws.generation}
          spec={ws.spec}
          result={ws.result}
          catalog={ws.catalogMap}
          selection={selection}
          dispatch={ws.dispatch}
          onSelect={setSelection}
          onNotice={setNotice}
        />
        {notice && <div className="toast" role="status">{notice}</div>}
      </main>
      <aside className="panel">
        <Inspector spec={ws.spec} result={ws.result} catalog={ws.catalogMap} regions={ws.regions} selection={selection} dispatch={ws.dispatch} />
      </aside>
      <Results result={ws.result} error={ws.error} warnings={warnings} onSelect={(id) => setSelection((ws.spec.groups ?? []).some((g) => g.id === id) ? { kind: "group", id } : { kind: "node", id })} />
      {importing && <TerraformDialog canMerge={ws.spec.nodes.length > 0} onImport={onImport} onClose={() => setImporting(false)} />}
    </div>
  );
}
