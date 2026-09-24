import { Background, Controls, MiniMap, ReactFlow, applyNodeChanges, type Connection, type EdgeChange, type NodeChange } from "@xyflow/react";
import { useEffect, useMemo, useState, type Dispatch } from "react";
import { toFlowEdges, toFlowNodes, type CardNode } from "../../lib/flow";
import { closesCycle, type Action, type Selection } from "../../lib/state";
import type { CatalogEntry, Result, Spec } from "../../lib/types";
import { ScouterNode } from "../../composites/ScouterNode";

interface Props {
  spec: Spec;
  result: Result | undefined;
  catalog: Map<string, CatalogEntry>;
  selection: Selection;
  dispatch: Dispatch<Action>;
  onSelect: (s: Selection) => void;
  onNotice: (text: string) => void;
}

const nodeTypes = { scouter: ScouterNode };

function edgeIndex(id: string): number {
  return Number(id.slice(1));
}

/** Why a connection is refused, or undefined when it is fine. */
function refusal(spec: Spec, c: Connection): string | undefined {
  if (!c.source || !c.target || c.source === c.target) return "An edge needs two different nodes.";
  if (spec.edges.some((e) => e.from === c.source && e.to === c.target && !e.kind)) return "These nodes are already connected.";
  if (closesCycle(spec, c.source, c.target)) return "That edge would close a cycle; load has to flow one way.";
  return undefined;
}

export function Canvas({ spec, result, catalog, selection, dispatch, onSelect, onNotice }: Props) {
  const [nodes, setNodes] = useState<CardNode[]>([]);
  const selectedNode = selection?.kind === "node" ? selection.id : undefined;
  const selectedEdge = selection?.kind === "edge" ? selection.index : undefined;
  useEffect(() => {
    setNodes((prev) => toFlowNodes(spec, result, catalog, prev).map((n) => ({ ...n, selected: n.id === selectedNode })));
  }, [spec, result, catalog, selectedNode]);
  const edges = useMemo(() => toFlowEdges(spec, result, selectedEdge), [spec, result, selectedEdge]);

  const onNodesChange = (changes: NodeChange<CardNode>[]) => {
    setNodes((ns) => applyNodeChanges(changes, ns));
    for (const c of changes) if (c.type === "remove") dispatch({ type: "removeNode", id: c.id });
  };
  const onEdgesChange = (changes: EdgeChange[]) => {
    const removed = changes.filter((c) => c.type === "remove").map((c) => edgeIndex(c.id));
    for (const i of removed.sort((a, b) => b - a)) dispatch({ type: "removeEdge", index: i });
    if (removed.length > 0) onSelect(undefined);
  };
  const onConnect = (c: Connection) => {
    const why = refusal(spec, c);
    if (why) return onNotice(why);
    dispatch({ type: "addEdge", edge: { from: c.source, to: c.target } });
    onSelect({ kind: "edge", index: spec.edges.length });
  };

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onNodeClick={(_, n) => onSelect({ kind: "node", id: n.id })}
      onEdgeClick={(_, e) => onSelect({ kind: "edge", index: edgeIndex(e.id) })}
      onPaneClick={() => onSelect(undefined)}
      onNodeDragStop={(_, __, dragged) => dispatch({ type: "move", positions: Object.fromEntries(dragged.map((n) => [n.id, n.position])) })}
      deleteKeyCode={["Backspace", "Delete"]}
      colorMode="system"
      fitView
      fitViewOptions={{ padding: 0.08, maxZoom: 1 }}
      minZoom={0.2}
      proOptions={{ hideAttribution: true }}
      defaultEdgeOptions={{ type: "smoothstep" }}
    >
      <Background gap={24} />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable />
    </ReactFlow>
  );
}
