import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { engine, EngineError, loadEngine } from "../lib/engine";
import { emptySpec, reducer } from "../lib/state";
import { withoutLoad } from "../lib/traffic";
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

/**
 * What the declaration costs with no load at all (see withoutLoad). A second
 * reading takes as long as the first, so it waits until editing pauses, and
 * edits that only change the load (the entry's users, a rate) reuse it.
 */
function useIdleCost(spec: Spec, ready: boolean): number | undefined {
  // Where cards and frames are drawn does not change a reading, so a drag keeps it too.
  const key = useMemo(() => {
    const idle = withoutLoad(spec);
    return JSON.stringify({
      ...idle,
      nodes: idle.nodes.map((n) => ({ ...n, position: undefined })),
      groups: idle.groups?.map((g) => ({ ...g, position: undefined, size: undefined })),
    });
  }, [spec]);
  const [idle, setIdle] = useState<{ key: string; usd: number | undefined }>();
  useEffect(() => {
    if (!ready || idle?.key === key) return;
    const t = window.setTimeout(() => {
      let usd: number | undefined;
      try {
        usd = engine.scout(JSON.parse(key) as Spec).monthlyUsd;
      } catch {
        usd = undefined;
      }
      setIdle({ key, usd });
    }, 250);
    return () => window.clearTimeout(t);
  }, [key, ready, idle?.key]);
  return idle?.key === key ? idle.usd : undefined;
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

  const idleUsd = useIdleCost(spec, ready);

  const catalogMap = useMemo(() => new Map(catalog.map((c) => [c.type, c])), [catalog]);
  return {
    spec,
    dispatch,
    ready,
    fatal,
    catalog,
    catalogMap,
    regions,
    result: result ?? last.current,
    // Only against readings of this very declaration, never the last good ones.
    idleUsd: result ? idleUsd : undefined,
    error,
    load,
    generation,
    setGeneration,
  };
}

export { message };
