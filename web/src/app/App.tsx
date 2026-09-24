import { useReactFlow } from "@xyflow/react";
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
import { autoLayout, freeSpot, NODE_HEIGHT, NODE_WIDTH } from "../lib/layout";
import { freshId, type Selection } from "../lib/state";
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
  const flow = useReactFlow();
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

  const addNode = (type: string) => {
    const id = freshId(ws.spec, ws.catalogMap.get(type)?.label ?? type);
    const box = document.querySelector(".canvas")?.getBoundingClientRect();
    const center = flow.screenToFlowPosition({ x: (box?.left ?? 0) + (box?.width ?? 600) / 2, y: (box?.top ?? 0) + (box?.height ?? 400) / 2 });
    const position = freeSpot(ws.spec, { x: Math.round(center.x - NODE_WIDTH / 2), y: Math.round(center.y - NODE_HEIGHT / 2) });
    ws.dispatch({ type: "addNode", node: { id, type, position } });
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
    onLayout: () => {
      ws.dispatch({ type: "move", positions: autoLayout(ws.spec) });
      ws.setGeneration((g) => g + 1);
    },
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
