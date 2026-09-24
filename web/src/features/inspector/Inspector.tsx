import type { Dispatch } from "react";
import type { Action, Selection } from "../../lib/state";
import type { CatalogEntry, Result, Spec } from "../../lib/types";
import { EdgeForm } from "./EdgeForm";
import { NodeForm } from "./NodeForm";
import { SpecForm } from "./SpecForm";

interface Props {
  spec: Spec;
  result: Result | undefined;
  catalog: Map<string, CatalogEntry>;
  regions: string[];
  selection: Selection;
  dispatch: Dispatch<Action>;
}

export function Inspector({ spec, result, catalog, regions, selection, dispatch }: Props) {
  if (selection?.kind === "node") {
    const node = spec.nodes.find((n) => n.id === selection.id);
    if (node) {
      const reading = (result?.nodes ?? []).find((r) => r.id === node.id);
      return <NodeForm key={node.id} node={node} entry={catalog.get(node.type)} reading={reading} dispatch={dispatch} />;
    }
  }
  if (selection?.kind === "edge") {
    const edge = spec.edges[selection.index];
    if (edge) {
      const target = spec.nodes.find((n) => n.id === edge.to);
      return <EdgeForm edge={edge} index={selection.index} target={target && catalog.get(target.type)} dispatch={dispatch} />;
    }
  }
  return <SpecForm spec={spec} regions={regions} dispatch={dispatch} />;
}
