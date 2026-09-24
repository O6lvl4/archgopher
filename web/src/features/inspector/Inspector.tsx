import type { Dispatch } from "react";
import type { Action, Selection } from "../../lib/state";
import type { CatalogEntry, RegionGroup, Result, Spec } from "../../lib/types";
import { EdgeForm } from "./EdgeForm";
import { GroupForm } from "./GroupForm";
import { NodeForm } from "./NodeForm";
import { SpecForm } from "./SpecForm";

interface Props {
  spec: Spec;
  result: Result | undefined;
  catalog: Map<string, CatalogEntry>;
  regions: RegionGroup[];
  selection: Selection;
  dispatch: Dispatch<Action>;
}

type Panel = (p: Props) => React.ReactNode;

const nodePanel: Panel = ({ spec, result, catalog, selection, dispatch }) => {
  if (selection?.kind !== "node") return undefined;
  const node = spec.nodes.find((n) => n.id === selection.id);
  if (!node) return undefined;
  const reading = (result?.nodes ?? []).find((r) => r.id === node.id);
  return <NodeForm key={node.id} node={node} entry={catalog.get(node.type)} reading={reading} dispatch={dispatch} />;
};

const edgePanel: Panel = ({ spec, catalog, selection, dispatch }) => {
  if (selection?.kind !== "edge") return undefined;
  const edge = spec.edges[selection.index];
  if (!edge) return undefined;
  const target = spec.nodes.find((n) => n.id === edge.to);
  return <EdgeForm edge={edge} index={selection.index} target={target && catalog.get(target.type)} dispatch={dispatch} />;
};

const groupPanel: Panel = ({ spec, result, catalog, selection, dispatch }) => {
  if (selection?.kind !== "group") return undefined;
  const group = (spec.groups ?? []).find((g) => g.id === selection.id);
  if (!group) return undefined;
  const reading = (result?.groups ?? []).find((r) => r.id === group.id);
  return <GroupForm key={group.id} group={group} entry={group.type ? catalog.get(group.type) : undefined} reading={reading} dispatch={dispatch} />;
};

/** The form for what is selected, or the declaration's when nothing is. */
export function Inspector(props: Props) {
  return nodePanel(props) ?? edgePanel(props) ?? groupPanel(props) ?? <SpecForm spec={props.spec} regions={props.regions} dispatch={props.dispatch} />;
}
