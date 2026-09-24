import { Background, Controls, MiniMap, Panel, ReactFlow, applyNodeChanges, type Connection, type EdgeChange, type NodeChange } from "@xyflow/react";
import { useEffect, useMemo, useState, type Dispatch } from "react";
import { groupOfFrame, toFlowEdges, toFlowNodes, toFrames, type CardNode, type FlowNode } from "../../lib/flow";
import type { Rect } from "../../lib/frames";
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

const isCard = (n: FlowNode): n is CardNode => n.type === "scouter";

/** While a frame is dragged, the cards in it move by the same amount. */
function followFrames(changes: NodeChange<FlowNode>[], nodes: FlowNode[], spec: Spec): NodeChange<FlowNode>[] {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  return changes.flatMap((c) => {
    const group = c.type === "position" && c.dragging && c.position ? groupOfFrame(c.id) : undefined;
    const frame = c.type === "position" ? byId.get(c.id) : undefined;
    if (!group || !frame || c.type !== "position" || !c.position) return [];
    const dx = c.position.x - frame.position.x;
    const dy = c.position.y - frame.position.y;
    return spec.nodes.flatMap((n) => {
      const card = n.group === group ? byId.get(n.id) : undefined;
      return card ? [{ type: "position" as const, id: card.id, position: { x: card.position.x + dx, y: card.position.y + dy }, dragging: true }] : [];
    });
  });
}

function frameRect(n: FlowNode): Rect {
  return { x: n.position.x, y: n.position.y, width: n.width ?? n.measured?.width ?? 0, height: n.height ?? n.measured?.height ?? 0 };
}

export function Canvas({ spec, result, catalog, selection, dispatch, onSelect, onNotice }: Props) {
  const [nodes, setNodes] = useState<FlowNode[]>([]);
  const selectedNode = selection?.kind === "node" ? selection.id : undefined;
  const selectedGroup = selection?.kind === "group" ? selection.id : undefined;
  const selectedEdge = selection?.kind === "edge" ? selection.index : undefined;
  useEffect(() => {
    const onResized = (id: string, rect: Rect) => dispatch({ type: "frame", id, rect, positions: {} });
    setNodes((prev) => [
      ...toFrames(spec, result, selectedGroup, onResized),
      ...toFlowNodes(spec, result, catalog, prev.filter(isCard)).map((n) => ({ ...n, selected: n.id === selectedNode })),
    ]);
  }, [spec, result, catalog, selectedNode, selectedGroup, dispatch]);
  const edges = useMemo(() => toFlowEdges(spec, result, selectedEdge), [spec, result, selectedEdge]);

  const onNodesChange = (changes: NodeChange<FlowNode>[]) => {
    setNodes((ns) => applyNodeChanges([...changes, ...followFrames(changes, ns, spec)], ns));
    for (const c of changes) if (c.type === "remove" && !groupOfFrame(c.id)) dispatch({ type: "removeNode", id: c.id });
  };
  const onDragStop = (dragged: FlowNode[]) => {
    const frame = dragged.find((n) => groupOfFrame(n.id));
    const group = frame && groupOfFrame(frame.id);
    if (frame && group) {
      const members = new Set(spec.nodes.filter((n) => n.group === group).map((n) => n.id));
      const positions = Object.fromEntries(nodes.filter((n) => members.has(n.id)).map((n) => [n.id, n.position]));
      return dispatch({ type: "frame", id: group, rect: frameRect(frame), positions });
    }
    dispatch({ type: "drop", positions: Object.fromEntries(dragged.filter(isCard).map((n) => [n.id, n.position])) });
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
      onNodeClick={(_, n) => {
        const group = groupOfFrame(n.id);
        onSelect(group ? { kind: "group", id: group } : { kind: "node", id: n.id });
      }}
      onEdgeClick={(_, e) => onSelect({ kind: "edge", index: edgeIndex(e.id) })}
      onPaneClick={() => onSelect(undefined)}
      onNodeDragStop={(_, __, dragged) => onDragStop(dragged)}
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
