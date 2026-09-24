import { Background, Controls, MiniMap, Panel, ReactFlow, applyNodeChanges, type Connection, type EdgeChange, type NodeChange } from "@xyflow/react";
import { useEffect, useMemo, useState, type Dispatch } from "react";
import { toFlowEdges, toFlowNodes, toFrames, type CardNode, type FrameNode } from "../../lib/flow";
import { closesCycle, type Action, type Selection } from "../../lib/state";
import type { CatalogEntry, Result, Spec } from "../../lib/types";
import { GroupFrame } from "../../composites/GroupFrame";
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

const nodeTypes = { scouter: ScouterNode, frame: GroupFrame };

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
  const shown = useMemo<(CardNode | FrameNode)[]>(() => [...toFrames(spec, nodes), ...nodes], [spec, nodes]);

  const onNodesChange = (changes: NodeChange<CardNode | FrameNode>[]) => {
    const cards = changes.filter((c) => !("id" in c && c.id.startsWith("group:"))) as NodeChange<CardNode>[];
    setNodes((ns) => applyNodeChanges(cards, ns));
    for (const c of cards) if (c.type === "remove") dispatch({ type: "removeNode", id: c.id });
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
      nodes={shown}
      edges={edges}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onNodeClick={(_, n) => n.type === "scouter" && onSelect({ kind: "node", id: n.id })}
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
      {spec.nodes.length === 0 && (
        <Panel position="top-center" className="empty-hint">
          Add a resource from the catalog, import a Terraform folder, or open an example.
        </Panel>
      )}
      <Background gap={24} />
      <Controls showInteractive={false} />
      {spec.nodes.length > 0 && <MiniMap pannable zoomable nodeClassName={(n) => (n.type === "frame" ? "minimap-frame" : "")} />}
    </ReactFlow>
  );
}
