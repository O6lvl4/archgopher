import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { engine, EngineError, loadEngine } from "./engine";
import { exampleYaml } from "./examples";
import { autoLayout, needsLayout } from "./layout";
import { emptySpec, reducer } from "./state";
import { loadSaved, save } from "./storage";
import type { CatalogEntry, Result, Spec } from "./types";

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** Places every node that has no position yet, keeping the others where they are. */
export function placed(spec: Spec): Spec {
  if (!needsLayout(spec)) return spec;
  const layout = autoLayout(spec);
  return { ...spec, nodes: spec.nodes.map((n) => (n.position ? n : { ...n, position: layout[n.id] })) };
}

function read(spec: Spec, ready: boolean): { result?: Result; error?: string } {
  if (!ready) return {};
  try {
    return { result: engine.scout(spec) };
  } catch (e) {
    if (e instanceof EngineError) return { error: e.message };
    throw e;
  }
}

/** The engine, the declaration being edited, and its readings. */
export function useWorkspace() {
  const [spec, dispatch] = useReducer(reducer, emptySpec);
  const [ready, setReady] = useState(false);
  const [fatal, setFatal] = useState<string>();
  const [catalog, setCatalog] = useState<CatalogEntry[]>([]);
  const [regions, setRegions] = useState<string[]>([]);
  const [generation, setGeneration] = useState(0);

  const load = (next: Spec) => {
    dispatch({ type: "load", spec: placed(next) });
    setGeneration((g) => g + 1);
  };

  useEffect(() => {
    loadEngine()
      .then(() => {
        setCatalog(engine.catalog());
        setRegions(engine.regions());
        setReady(true);
        load(loadSaved() ?? engine.parseYaml(exampleYaml));
      })
      .catch((e: unknown) => setFatal(message(e)));
  }, []);

  useEffect(() => {
    if (ready) save(spec);
  }, [spec, ready]);

  // A structural error (a cycle, a dangling edge) keeps the last good readings on screen.
  const last = useRef<Result>(undefined);
  const { result, error } = useMemo(() => read(spec, ready), [spec, ready]);
  if (result) last.current = result;

  const catalogMap = useMemo(() => new Map(catalog.map((c) => [c.type, c])), [catalog]);
  return { spec, dispatch, ready, fatal, catalog, catalogMap, regions, result: result ?? last.current, error, load, generation, setGeneration };
}

export { message };
