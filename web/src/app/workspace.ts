import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { engine, EngineError, loadEngine } from "../lib/engine";
import { emptySpec, reducer } from "../lib/state";
import { loadSaved, save } from "../lib/storage";
import type { CatalogEntry, RegionGroup, Result, Spec } from "../lib/types";

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
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
  const [regions, setRegions] = useState<RegionGroup[]>([]);
  const [generation, setGeneration] = useState(0);

  const load = (next: Spec) => {
    dispatch({ type: "load", spec: next });
    setGeneration((g) => g + 1);
  };

  useEffect(() => {
    loadEngine()
      .then(() => {
        setCatalog(engine.catalog());
        setRegions(engine.regions());
        setReady(true);
        load(loadSaved() ?? emptySpec);
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
