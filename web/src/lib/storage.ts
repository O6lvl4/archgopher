import type { Spec } from "./types";

// Browser storage keeps the declaration being edited for this viewer only.
// It may be unavailable (private windows, blocked storage), so every access is guarded.

const KEY = "archgopher:spec";

export function loadSaved(): Spec | undefined {
  try {
    const raw = window.localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as Spec) : undefined;
  } catch {
    return undefined;
  }
}

export function save(spec: Spec): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(spec));
  } catch {
    // Storage is a convenience; editing still works without it.
  }
}
