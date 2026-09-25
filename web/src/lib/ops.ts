import type { EdgeOp, SpecEdge } from "./types";

// An edge does one or more operations per upstream unit. One operation is
// kept in the short form (kind, perUnit, kb) so a simple edge stays simple in
// YAML; two or more go under ops.

export function operations(e: SpecEdge): EdgeOp[] {
  return e.ops && e.ops.length > 0 ? e.ops : [{ kind: e.kind, perUnit: e.perUnit, kb: e.kb }];
}

/** The edge fields that say these operations. */
export function withOperations(ops: EdgeOp[]): Pick<SpecEdge, "kind" | "perUnit" | "kb" | "ops"> {
  if (ops.length === 1) return { kind: ops[0]?.kind, perUnit: ops[0]?.perUnit, kb: ops[0]?.kb, ops: undefined };
  return { kind: undefined, perUnit: undefined, kb: undefined, ops };
}

/** Kinds and sizes for an edge's label: "read 25 KB · write". */
export function opsLabel(e: SpecEdge): string {
  return operations(e)
    .map((o) => [o.kind, o.kb !== undefined ? `${o.kb} KB` : undefined].filter(Boolean).join(" "))
    .filter(Boolean)
    .join(" · ");
}
