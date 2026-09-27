import { useEffect, useRef, useState } from "react";

/**
 * Notices a value changing after the first render, for a moment: `flash` is
 * true for `ms` after the last change and `from` holds the value before the
 * changes began. A value that comes back to where it started, or passes
 * through undefined on the way (a half-typed input), does not count. A new
 * `reset` (another document or node) takes the value as it is, without a flash.
 */
export function useFlash<T>(value: T, reset: unknown, ms = 2400): { flash: boolean; from: T | undefined; tick: number } {
  const last = useRef({ value, reset });
  const [state, setState] = useState<{ from: T | undefined; tick: number; on: boolean }>({ from: undefined, tick: 0, on: false });

  useEffect(() => {
    const prev = last.current;
    if (prev.reset !== reset) {
      last.current = { value, reset };
      setState((s) => ({ ...s, from: undefined, on: false }));
      return;
    }
    if (value === undefined || Object.is(prev.value, value)) return;
    last.current = { value, reset };
    setState((s) => {
      // While typing, compare with the value from before the first keystroke.
      const from = s.on ? s.from : prev.value;
      return Object.is(from, value) ? { ...s, on: false } : { from, tick: s.tick + 1, on: true };
    });
    const t = window.setTimeout(() => setState((s) => ({ ...s, on: false })), ms);
    return () => window.clearTimeout(t);
  }, [value, reset, ms]);

  return { flash: state.on, from: state.from, tick: state.tick };
}
