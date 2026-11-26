"use client";

import * as React from "react";

import type { Graph as G6Graph } from "@antv/g6";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { CoverageGraphEdge, CoverageGraphNode } from "@/lib/types";
import { cn } from "@/lib/utils";

import {
  computeVisible,
  FOLD_ICON,
  FOLD_LIMIT,
  FOLD_STEP,
  type FoldNode,
  type G6NodeDatum,
  KIND_ICON,
  type Kind,
  kindMeta,
  nd,
  nodeFill,
  nodeStroke,
  type RenderNode,
  toG6Nodes,
} from "./coverage-graph-model";
import { AssetSheet, FoldSheet } from "./coverage-graph-sheets";

function GraphInner({ taskId, coverageEnabled = true }: { taskId: string; coverageEnabled?: boolean }) {
  const [data, setData] = React.useState<{
    nodes: CoverageGraphNode[];
    edges: CoverageGraphEdge[];
  } | null>(null);
  const [expanded, setExpanded] = React.useState<Map<string, number>>(new Map());
  const [selectedAsset, setSelectedAsset] = React.useState<CoverageGraphNode | null>(null);
  const [selectedFoldId, setSelectedFoldId] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);

  const containerRef = React.useRef<HTMLDivElement>(null);
  const graphRef = React.useRef<G6Graph | null>(null);
  // The click handler needs the latest key→RenderNode map (G6 event callbacks read it via ref).
  const renderMapRef = React.useRef<Map<string, RenderNode>>(new Map());
  const gDataRef = React.useRef<{ nodes: G6NodeDatum[]; edges: { source: string; target: string }[] }>({
    nodes: [],
    edges: [],
  });

  const fetchGraph = React.useCallback(() => {
    setLoading(true);
    api
      .taskCoverageGraph(taskId)
      .then((g) => {
        // Asset coverage disabled (B1): the graph still renders (in-scope assets/company links remain visible), but
        // tested state is flattened away — no test progress and no tested highlight.
        const nodes = coverageEnabled ? (g.nodes ?? []) : (g.nodes ?? []).map((n) => ({ ...n, tested: false }));
        setData({ nodes, edges: g.edges ?? [] });
      })
      .catch(() => {
        /* keep the previous data */
      })
      .finally(() => setLoading(false));
  }, [taskId, coverageEnabled]);

  React.useEffect(() => {
    fetchGraph();
  }, [fetchGraph]);

  const { renderNodes, renderEdges } = React.useMemo(
    () =>
      data
        ? computeVisible(data.nodes, data.edges, expanded)
        : { renderNodes: [] as RenderNode[], renderEdges: [] as CoverageGraphEdge[] },
    [data, expanded],
  );

  // Structure signature: rebuild the graph and re-run layout only when the visible node/edge set changes, to avoid needless jitter.
  const sig = React.useMemo(
    () =>
      `${renderNodes
        .map((n) => `${n.key}:${n.fold ? "f" : n.node.tested ? "t" : "u"}`)
        .sort()
        .join(",")}|${renderEdges.length}`,
    [renderNodes, renderEdges],
  );

  // Maintain gDataRef + renderMapRef (read by events and graph data application).
  gDataRef.current = {
    nodes: toG6Nodes(renderNodes),
    edges: renderEdges.map((e) => ({ source: e.src, target: e.dst })),
  };
  const rmap = new Map<string, RenderNode>();
  for (const rn of renderNodes) rmap.set(rn.key, rn);
  renderMapRef.current = rmap;

  const applyData = React.useCallback(() => {
    const graph = graphRef.current;
    if (!graph || graph.destroyed) return;
    graph.setData(gDataRef.current);
    // render() runs the d3-force layout asynchronously; if the component unmounts/destroys before
    // the layout lands, g6 throws accessing transform on the cleared context (see runtime/layout transformDataAfterLayout). This is a pure
    // teardown race — swallow it, functionality is unaffected; real render errors (graph not destroyed) are still logged.
    void graph.render().catch((err) => {
      if (!graph.destroyed) console.error("[coverage-graph] render:", err);
    });
  }, []);

  // Build the graph (once). Dynamic import avoids the window dependency during SSR/static export.
  React.useEffect(() => {
    let destroyed = false;
    let graph: G6Graph | null = null;
    void (async () => {
      const { Graph } = await import("@antv/g6");
      if (destroyed || !containerRef.current) return;
      graph = new Graph({
        container: containerRef.current,
        autoResize: true,
        autoFit: "view",
        background: "#f0f2f7",
        node: {
          style: {
            size: (d: unknown) => nd(d).size,
            fill: (d: unknown) => nodeFill(nd(d)),
            stroke: (d: unknown) => nodeStroke(nd(d)),
            lineWidth: (d: unknown) => (nd(d).fold || !nd(d).inScope ? 1 : 1.5),
            lineDash: (d: unknown) => (nd(d).fold || !nd(d).inScope ? [3, 3] : [0]),
            iconSrc: (d: unknown) => (nd(d).fold ? FOLD_ICON : KIND_ICON[nd(d).kind]),
            iconWidth: (d: unknown) => Math.max(12, nd(d).size * 0.55),
            iconHeight: (d: unknown) => Math.max(12, nd(d).size * 0.55),
            labelText: (d: unknown) => nd(d).lbl,
            labelFontSize: 10,
            labelPlacement: "bottom",
            labelFill: "#475569",
            labelBackground: true,
            labelBackgroundFill: "rgba(255,255,255,0.75)",
            labelBackgroundRadius: 3,
            labelPadding: [1, 3],
          },
        },
        edge: {
          style: { stroke: "#cbd5e1", lineWidth: 1, endArrow: false },
        },
        layout: {
          type: "d3-force",
          collide: { radius: (d: unknown) => (nd(d).size ? nd(d).size : 20) + 8 },
          link: {
            distance: (edge: unknown) => {
              // Top levels (company/root domain) sit farther from child nodes, leaves closer. edge.source may be an id or an already-resolved node.
              const s = (edge as { source: string | { id?: string } }).source;
              const srcId = typeof s === "string" ? s : (s?.id ?? "");
              const src = renderMapRef.current.get(srcId);
              const k = src && !src.fold ? src.kind : "endpoint";
              return k === "company" || k === "root_domain" ? 120 : 60;
            },
          },
          manyBody: {
            strength: (d: unknown) => (nd(d).kind === "endpoint" || nd(d).fold ? -60 : -200),
          },
        },
        behaviors: ["drag-element-force", "drag-canvas", "zoom-canvas"],
      });
      graph.on("node:click", (evt: unknown) => {
        const id = (evt as { target?: { id?: string } }).target?.id;
        if (!id) return;
        const rn = renderMapRef.current.get(id);
        if (!rn) return;
        if (rn.fold) setSelectedFoldId(rn.key);
        else setSelectedAsset(rn.node);
      });
      graphRef.current = graph;
      applyData();
    })();
    return () => {
      destroyed = true;
      // Stop the layout before destroying: shrink the race window between "layout in flight" and "context cleared".
      try {
        graph?.stopLayout();
      } catch {
        /* the graph may not be built yet or may have no layout context */
      }
      graph?.destroy();
      graphRef.current = null;
    };
  }, [applyData]);

  // Visible set changed → re-feed data + layout. sig is only a re-layout trigger (applyData reads gDataRef).
  // biome-ignore lint/correctness/useExhaustiveDependencies: sig is a deliberate re-layout trigger dependency
  React.useEffect(() => {
    applyData();
  }, [sig, applyData]);

  const showMore = React.useCallback((groupId: string) => {
    setExpanded((prev) => {
      const m = new Map(prev);
      m.set(groupId, (m.get(groupId) ?? FOLD_LIMIT) + FOLD_STEP);
      return m;
    });
  }, []);

  const selectedFold =
    selectedFoldId != null
      ? ((renderNodes.find((n) => n.fold && n.key === selectedFoldId) as FoldNode | undefined) ?? null)
      : null;
  React.useEffect(() => {
    if (selectedFoldId != null && !selectedFold) setSelectedFoldId(null);
  }, [selectedFoldId, selectedFold]);

  const total = data?.nodes.length ?? 0;
  const tested = data?.nodes.filter((n) => n.in_scope && n.tested).length ?? 0;
  const inScope = data?.nodes.filter((n) => n.in_scope).length ?? 0;

  return (
    <div className="relative h-full w-full">
      <div ref={containerRef} className="h-full w-full" />

      {/* Legend + stats + refresh (overlay) */}
      <div className="bg-card/95 pointer-events-auto absolute top-3 left-3 flex max-w-[340px] flex-col gap-2.5 rounded-lg border p-3 text-xs shadow-sm backdrop-blur">
        <div className="flex items-center justify-between gap-3">
          {total > 0 ? (
            <span className="text-muted-foreground">
              {t("In scope")} <span className="text-foreground font-semibold tabular-nums">{inScope}</span>
              {coverageEnabled && (
                <>
                  {" "}
                  · {t("Tested")}{" "}
                  <span className="font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">{tested}</span>
                </>
              )}
            </span>
          ) : (
            <span className="text-muted-foreground">
              {loading ? t("Loading…") : t("No in-scope assets yet (anchor the task scope first)")}
            </span>
          )}
          <Button variant="ghost" size="icon" className="size-6" onClick={fetchGraph} title={t("Refresh")}>
            <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          </Button>
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-1.5">
          {(Object.keys(kindMeta) as Kind[]).map((k) => {
            const m = kindMeta[k];
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
          {coverageEnabled && (
            <>
              <span className="inline-flex items-center gap-1.5">
                <span className="size-3 rounded-full bg-emerald-500" /> {t("Tested (highlighted)")}
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="size-3 rounded-full bg-neutral-400" /> {t("Untested")}
              </span>
            </>
          )}
          <span className="inline-flex items-center gap-1.5">
            <span className="size-3 rounded-full border border-dashed border-neutral-400 bg-neutral-200" />{" "}
            {t("Out of scope")}
          </span>
        </div>
        <p className="text-muted-foreground/80 border-border/60 border-t pt-2 leading-relaxed">
          {t(
            'Force-directed layout; drag nodes and scroll to zoom. The gray "⋯" is a fold node — click it to expand more.',
          )}
        </p>
      </div>

      <AssetSheet node={selectedAsset} taskId={taskId} onOpenChange={(o) => !o && setSelectedAsset(null)} />
      <FoldSheet
        fold={selectedFold}
        onOpenChange={(o) => !o && setSelectedFoldId(null)}
        onShowMore={showMore}
        onPick={(n) => {
          setSelectedFoldId(null);
          setSelectedAsset(n);
        }}
      />
    </div>
  );
}

export function CoverageGraphTab({ taskId, coverageEnabled = true }: { taskId: string; coverageEnabled?: boolean }) {
  return (
    <Card>
      <CardContent className="p-0">
        <div className="h-[72vh] w-full overflow-hidden rounded-xl">
          <GraphInner taskId={taskId} coverageEnabled={coverageEnabled} />
        </div>
      </CardContent>
    </Card>
  );
}
