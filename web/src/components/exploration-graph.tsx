"use client";
"use no memo";

import * as React from "react";

import {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  ReactFlowProvider,
  type Edge as RFEdge,
  useEdgesState,
  useNodesState,
  useReactFlow,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { t } from "@/i18n";
import type { Edge, ExploreKind, TaskNode } from "@/lib/types";
import { cn } from "@/lib/utils";

import {
  collapseDigestGraph,
  computeLayout,
  type ExploreNodeData,
  type ExploreRFNode,
  relColor,
  relLabel,
  typeMeta,
  viewKind,
} from "./exploration-graph/model";
import { NodeDetailSheet, nodeTypes } from "./exploration-graph/nodes";

export { relColor, relLabel } from "./exploration-graph/model";

function ExplorationGraphInner({
  nodes,
  edges,
  emptyHint = t("No exploration data"),
  focusNodeId,
}: {
  nodes: TaskNode[];
  edges: Edge[];
  emptyHint?: string;
  focusNodeId?: string;
}) {
  const { fitView } = useReactFlow();
  // The focus id already applied, so polling refreshes do not re-open the drawer the user closed.
  const focusedRef = React.useRef("");
  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<ExploreRFNode>([]);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<RFEdge>([]);
  const [selected, setSelected] = React.useState<TaskNode | null>(null);
  // Full node index (incl. folded members) so the digest drawer can resolve
  // covered members even though they are not rendered on the canvas.
  const nodesById = React.useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  // Collapsed view: folded members removed, their edges rewired onto the digest.
  const { viewNodes, viewEdges } = React.useMemo(() => collapseDigestGraph(nodes, edges), [nodes, edges]);
  const isEmpty = viewNodes.length === 0;

  React.useEffect(() => {
    setSelected((cur) => (cur ? (nodesById.get(cur.id) ?? cur) : cur));
    const liveTargets = new Set(
      viewNodes.filter((n) => !n.inherited && n.type === "intent" && n.state === "running").map((n) => n.id),
    );
    const layout = computeLayout(viewNodes, viewEdges);
    setRfNodes((prev) => {
      const prevPos = new Map(prev.map((p) => [p.id, p.position]));
      return viewNodes.map((n) => ({
        id: n.id,
        type: "explore" as const,
        position: prevPos.get(n.id) ?? layout.get(n.id) ?? { x: 0, y: 0 },
        data: { node: n },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
      }));
    });
    setRfEdges(
      viewEdges.map((e, i) => {
        const color = relColor[e.rel] ?? "#94a3b8";
        const animated = liveTargets.has(e.dst);
        return {
          id: `e${i}-${e.src}-${e.dst}`,
          source: e.src,
          target: e.dst,
          label: relLabel[e.rel] ?? e.rel,
          type: "default",
          animated,
          labelShowBg: false,
          labelStyle: { fontSize: 10, fontWeight: 600, fill: color },
          style: { stroke: color, strokeWidth: 2, strokeOpacity: animated ? 0.9 : 0.55 },
          markerEnd: { type: MarkerType.ArrowClosed, color, width: 14, height: 14 },
        } satisfies RFEdge;
      }),
    );
  }, [viewNodes, viewEdges, nodesById, setRfNodes, setRfEdges]);

  // Deep link focus (?node=<id>): once the node loads, open its drawer and, when it is on the
  // canvas (not folded into a digest), select and center on it.
  React.useEffect(() => {
    if (!focusNodeId || focusedRef.current === focusNodeId) return;
    const target = nodesById.get(focusNodeId);
    if (!target || rfNodes.length === 0) return;
    focusedRef.current = focusNodeId;
    setSelected(target);
    if (!rfNodes.some((n) => n.id === focusNodeId)) return;
    setRfNodes((prev) => prev.map((n) => ({ ...n, selected: n.id === focusNodeId })));
    window.requestAnimationFrame(() => {
      void fitView({ nodes: [{ id: focusNodeId }], padding: 0.6, maxZoom: 1.2, duration: 400 });
    });
  }, [focusNodeId, nodesById, rfNodes, setRfNodes, fitView]);

  return (
    <>
      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={(_, node) => setSelected(node.data.node)}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
        onlyRenderVisibleElements
        minZoom={0.1}
        className="!bg-[#f0f2f7] dark:!bg-neutral-950"
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={16}
          size={1}
          className="text-neutral-400/50 dark:text-neutral-700/60"
        />
        <Controls
          showInteractive={false}
          className="!rounded-lg !border !shadow-sm [&>button]:!border-border [&>button]:!bg-card [&>button:hover]:!bg-accent [&_svg]:!fill-foreground"
        />
        <MiniMap
          pannable
          zoomable
          className="!bg-card !rounded-lg !border !shadow-sm"
          maskColor="rgb(148 163 184 / 0.18)"
          nodeColor={(node) => {
            const data = node.data as ExploreNodeData | undefined;
            const k = data?.node ? viewKind(data.node) : "intent";
            return typeMeta[k]?.hex ?? "#94a3b8";
          }}
          nodeStrokeWidth={0}
          nodeBorderRadius={4}
        />
        <Panel position="top-left">
          <div className="bg-card/95 flex flex-col gap-2.5 rounded-lg border p-3 text-xs shadow-sm backdrop-blur">
            {isEmpty && <span className="text-muted-foreground">{emptyHint}</span>}
            <div className="flex flex-wrap gap-x-3 gap-y-1.5">
              {(["begin", "goal", "intent", "fact", "finding", "hint", "digest"] as ExploreKind[]).map((k) => {
                const m = typeMeta[k];
                const Icon = m.icon;
                return (
                  <span key={k} className="text-foreground inline-flex items-center gap-1.5">
                    <span className={cn("flex size-4 items-center justify-center rounded", m.iconBg)}>
                      <Icon className="size-2.5 text-white" />
                    </span>
                    {m.label}
                  </span>
                );
              })}
            </div>
            <div className="border-border/60 text-muted-foreground flex flex-wrap gap-x-3 gap-y-1.5 border-t pt-2">
              {Object.entries(relLabel).map(([k, v]) => (
                <span key={k} className="inline-flex items-center gap-1.5">
                  <span className="h-0.5 w-4 rounded-full" style={{ backgroundColor: relColor[k] }} />
                  {v}
                </span>
              ))}
            </div>
          </div>
        </Panel>
      </ReactFlow>
      <NodeDetailSheet
        node={selected}
        nodesById={nodesById}
        onSelect={setSelected}
        onOpenChange={(o) => !o && setSelected(null)}
      />
    </>
  );
}

// ExplorationGraph is the shared exploration graph / attack-path graph canvas — used by the task graph
// tab (full graph, polled) and the finding lineage view (origin→finding subgraph).
export function ExplorationGraph({
  nodes,
  edges,
  className,
  emptyHint,
  focusNodeId,
}: {
  nodes: TaskNode[];
  edges: Edge[];
  className?: string;
  emptyHint?: string;
  /** Node to select and center on once it loads (the task page's ?node= deep link). */
  focusNodeId?: string;
}) {
  return (
    <div className={cn("w-full overflow-hidden rounded-xl", className)}>
      <ReactFlowProvider>
        <ExplorationGraphInner nodes={nodes} edges={edges} emptyHint={emptyHint} focusNodeId={focusNodeId} />
      </ReactFlowProvider>
    </div>
  );
}
