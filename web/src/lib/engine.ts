import type { CatalogEntry, Result, Spec, TerraformResponse } from "./types";

// The engine is the Go package api compiled to WebAssembly. It registers a
// global archGopher(name, input) that returns {"ok": ...} or {"error": ...}.

declare global {
  interface Window {
    Go?: new () => { importObject: WebAssembly.Imports; run(instance: WebAssembly.Instance): Promise<void> };
    archGopher?: (name: string, input: string) => string;
  }
}

export class EngineError extends Error {}

let loading: Promise<void> | undefined;

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new EngineError(`cannot load ${src}`));
    document.head.appendChild(s);
  });
}

async function start(): Promise<void> {
  const base = import.meta.env.BASE_URL;
  await loadScript(`${base}wasm_exec.js`);
  const GoRuntime = window.Go;
  if (!GoRuntime) throw new EngineError("wasm_exec.js did not define Go");
  const go = new GoRuntime();
  const ready = new Promise<void>((resolve) => window.addEventListener("archgopher-ready", () => resolve(), { once: true }));
  const { instance } = await WebAssembly.instantiateStreaming(fetch(`${base}archgopher.wasm`), go.importObject);
  void go.run(instance);
  await ready;
}

/** Loads the engine once; later calls share the same promise. */
export function loadEngine(): Promise<void> {
  loading ??= start();
  return loading;
}

function call<T>(name: string, input = ""): T {
  const fn = window.archGopher;
  if (!fn) throw new EngineError("the engine is not loaded");
  const reply = JSON.parse(fn(name, input)) as { ok?: T; error?: string };
  if (reply.error !== undefined) throw new EngineError(reply.error);
  return reply.ok as T;
}

export const engine = {
  catalog: () => call<CatalogEntry[]>("catalog"),
  regions: () => call<string[]>("regions"),
  scout: (spec: Spec) => call<Result>("scout", JSON.stringify(spec)),
  parseYaml: (text: string) => call<Spec>("parseYaml", text),
  toYaml: (spec: Spec) => call<string>("toYaml", JSON.stringify(spec)),
  roots: (files: Record<string, string>) => call<string[]>("roots", JSON.stringify(files)),
  terraform: (req: { files: Record<string, string>; root: string; vars?: Record<string, string>; merge?: Spec }) =>
    call<TerraformResponse>("terraform", JSON.stringify(req)),
};
