import { Background, ControlButton, Controls, MiniMap, Panel, ReactFlow, ViewportPortal, applyNodeChanges, useReactFlow, type Connection, type EdgeChange, type NodeChange } from "@xyflow/react";
import { useEffect, useMemo, useState, type Dispatch } from "react";
import { groupOfFrame, toFlowEdges, toFlowNodes, toFrames, type CardNode, type FlowNode } from "../../lib/flow";
import { closesCycle, type Action, type Selection } from "../../lib/state";
import type { CatalogEntry, Result, Spec } from "../../lib/types";
import { GroupFrame } from "../../composites/GroupFrame";
import type { Guides } from "../../lib/snap";
import { useDrag } from "./useDrag";
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



/** Delete or Backspace removes the selected frame (its cards stay), unless typing in a field. */
function useDeleteGroup(group: string | undefined, dispatch: Dispatch<Action>) {
  useEffect(() => {
    if (!group) return;
    const onKey = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && e.target.closest("input, textarea, select, [contenteditable]");
      if ((e.key === "Delete" || e.key === "Backspace") && !typing) dispatch({ type: "removeGroup", id: group });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [group, dispatch]);
}

/** Brings everything into view when nodes or frames come or go. */
function useFitOnReshape(spec: Spec) {
  const flow = useReactFlow();
  const shape = `${spec.nodes.map((n) => n.id).join()}|${(spec.groups ?? []).map((g) => g.id).join()}`;
  useEffect(() => {
    const t = window.setTimeout(() => void flow.fitView({ padding: 0.08, maxZoom: 1, duration: 300 }), 50);
    return () => window.clearTimeout(t);
  }, [shape, flow]);
}

/** The lines a dragged card lines up with, drawn across the canvas. */
function GuideLines({ guides }: { guides: Guides }) {
  return (
    <ViewportPortal>
      {guides.x !== undefined && <div className="guide guide-x" style={{ transform: `translate(${guides.x}px, -5000px)` }} />}
      {guides.y !== undefined && <div className="guide guide-y" style={{ transform: `translate(-5000px, ${guides.y}px)` }} />}
    </ViewportPortal>
  );
}

export function Canvas({ spec, result, catalog, selection, dispatch, onSelect, onNotice }: Props) {
  const [nodes, setNodes] = useState<FlowNode[]>([]);
  const drag = useDrag(spec);
  const selectedNode = selection?.kind === "node" ? selection.id : undefined;
  const selectedGroup = selection?.kind === "group" ? selection.id : undefined;
  const selectedEdge = selection?.kind === "edge" ? selection.index : undefined;
  useEffect(() => {
    setNodes((prev) => [
      ...toFrames(spec, result, selectedGroup, drag.held),
      ...toFlowNodes(spec, result, catalog, prev.filter(isCard)).map((n) => ({ ...n, selected: n.id === selectedNode })),
    ]);
  }, [spec, result, catalog, selectedNode, selectedGroup, drag.held]);
  const edges = useMemo(() => toFlowEdges(spec, result, selectedEdge), [spec, result, selectedEdge]);
  useDeleteGroup(selectedGroup, dispatch);
  useFitOnReshape(spec);

  const onNodesChange = (changes: NodeChange<FlowNode>[]) => {
    setNodes((ns) => applyNodeChanges(drag.adjust(changes, ns), ns));
    for (const c of changes) if (c.type === "remove" && !groupOfFrame(c.id)) dispatch({ type: "removeNode", id: c.id });
  };
  const onDragStop = (dragged: FlowNode[]) => {
    const drop = drag.stop(dragged, nodes);
    dispatch(drop.groups ? { type: "drop", positions: drop.positions, groups: drop.groups } : { type: "move", positions: drop.positions, frames: drop.frames });
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
      onNodeDragStart={() => drag.start(nodes)}
      onNodeDragStop={(_, __, dragged) => onDragStop(dragged)}
      deleteKeyCode={["Backspace", "Delete"]}
      elevateNodesOnSelect={false}
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
      <GuideLines guides={drag.guides} />
      <Controls showInteractive={false}>
        <ControlButton onClick={() => dispatch({ type: "tidy" })} title="Lay everything out again" aria-label="Lay everything out again">
          ⇶
        </ControlButton>
      </Controls>
      {spec.nodes.length > 0 && <MiniMap pannable zoomable nodeClassName={(n) => (n.type === "frame" ? "minimap-frame" : "")} />}
    </ReactFlow>
  );
}
