import { AppWindow, Building2, Globe, Link2, type LucideIcon, Radio, Server, Waypoints } from "lucide-react";

import { t } from "@/i18n";
import type { CoverageGraphEdge, CoverageGraphNode } from "@/lib/types";

// Default number of children shown per kind under each parent; the rest fold away, and "Show more" pulls in this many each time.
export const FOLD_LIMIT = 20;
export const FOLD_STEP = 20;

export type Kind = CoverageGraphNode["kind"];

export type KindMeta = { label: string; icon: LucideIcon; iconBg: string; hex: string; size: number };

export const kindMeta: Record<Kind, KindMeta> = {
  company: { label: t("Company"), icon: Building2, iconBg: "bg-slate-500", hex: "#64748b", size: 46 },
  root_domain: { label: t("Root domain"), icon: Globe, iconBg: "bg-indigo-500", hex: "#6366f1", size: 38 },
  subdomain: { label: t("Subdomain"), icon: Waypoints, iconBg: "bg-blue-500", hex: "#3b82f6", size: 30 },
  ip: { label: "IP", icon: Server, iconBg: "bg-cyan-600", hex: "#0891b2", size: 28 },
  service: { label: t("Service"), icon: Radio, iconBg: "bg-amber-500", hex: "#f59e0b", size: 26 },
  app: { label: t("App"), icon: AppWindow, iconBg: "bg-fuchsia-500", hex: "#d946ef", size: 26 },
  endpoint: { label: t("Endpoint"), icon: Link2, iconBg: "bg-rose-500", hex: "#f43f5e", size: 20 },
};

// G6 node icons use the platform's lucide icons: render lucide SVG paths (v1.22) as white-stroked
// data URIs for node iconSrc (white reads well on both solid and gray fills). Hand-inlined to avoid react-dom/server
// issues in the React19/Next client bundle.
export function svgUri(inner: string, filled = false): string {
  const attrs = filled
    ? 'fill="#fff" stroke="none"'
    : 'fill="none" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"';
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" ${attrs}>${inner}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

export const KIND_ICON: Record<Kind, string> = {
  company: svgUri(
    '<path d="M10 12h4"/><path d="M10 8h4"/><path d="M14 21v-3a2 2 0 0 0-4 0v3"/><path d="M6 10H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-2"/><path d="M6 21V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v16"/>',
  ),
  root_domain: svgUri(
    '<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>',
  ),
  subdomain: svgUri(
    '<path d="m10.586 5.414-5.172 5.172"/><path d="m18.586 13.414-5.172 5.172"/><path d="M6 12h12"/><circle cx="12" cy="20" r="2"/><circle cx="12" cy="4" r="2"/><circle cx="20" cy="12" r="2"/><circle cx="4" cy="12" r="2"/>',
  ),
  ip: svgUri(
    '<rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>',
  ),
  service: svgUri(
    '<path d="M16.247 7.761a6 6 0 0 1 0 8.478"/><path d="M19.075 4.933a10 10 0 0 1 0 14.134"/><path d="M4.925 19.067a10 10 0 0 1 0-14.134"/><path d="M7.753 16.239a6 6 0 0 1 0-8.478"/><circle cx="12" cy="12" r="2"/>',
  ),
  app: svgUri(
    '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="M10 4v4"/><path d="M2 8h20"/><path d="M6 4v4"/>',
  ),
  endpoint: svgUri(
    '<path d="M9 17H7A5 5 0 0 1 7 7h2"/><path d="M15 7h2a5 5 0 1 1 0 10h-2"/><line x1="8" x2="16" y1="12" y2="12"/>',
  ),
};

export const FOLD_ICON = svgUri(
  '<circle cx="5" cy="12" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="19" cy="12" r="1.6"/>',
  true,
);

// ---------------------------------------------------------------------------
// Folding: full graph → currently-visible node/edge set (top-down cascading fold).
// ---------------------------------------------------------------------------
export type FoldNode = {
  fold: true;
  key: string;
  groupId: string;
  parentKey: string;
  kind: Kind;
  hidden: CoverageGraphNode[];
};
export type AssetRenderNode = { fold: false; key: string; kind: Kind; node: CoverageGraphNode };
export type RenderNode = AssetRenderNode | FoldNode;

export function sortChildren(a: CoverageGraphNode, b: CoverageGraphNode): number {
  if (a.tested !== b.tested) return a.tested ? -1 : 1;
  return (a.label || "").localeCompare(b.label || "");
}

export function computeVisible(
  nodes: CoverageGraphNode[],
  edges: CoverageGraphEdge[],
  expanded: Map<string, number>,
): { renderNodes: RenderNode[]; renderEdges: CoverageGraphEdge[] } {
  const byKey = new Map(nodes.map((n) => [n.key, n]));
  const parentOf = new Map<string, string>();
  const childrenOf = new Map<string, string[]>();
  for (const e of edges) {
    if (!byKey.has(e.src) || !byKey.has(e.dst)) continue;
    if (!parentOf.has(e.src)) parentOf.set(e.src, e.dst);
    const arr = childrenOf.get(e.dst);
    if (arr) arr.push(e.src);
    else childrenOf.set(e.dst, [e.src]);
  }

  const renderNodes: RenderNode[] = [];
  const visible = new Set<string>();
  const foldNodes: FoldNode[] = [];

  const queue: string[] = [];
  for (const n of nodes) {
    if (!parentOf.has(n.key)) {
      queue.push(n.key);
      visible.add(n.key);
    }
  }

  for (let qi = 0; qi < queue.length; qi++) {
    const parent = queue[qi];
    const kids = childrenOf.get(parent) ?? [];
    if (kids.length === 0) continue;
    const groups = new Map<Kind, CoverageGraphNode[]>();
    for (const ck of kids) {
      const cn = byKey.get(ck);
      if (!cn) continue;
      const g = groups.get(cn.kind);
      if (g) g.push(cn);
      else groups.set(cn.kind, [cn]);
    }
    for (const [kind, arr] of groups) {
      arr.sort(sortChildren);
      const groupId = `${parent}::${kind}`;
      const shown = expanded.get(groupId) ?? FOLD_LIMIT;
      const visibleChildren = arr.slice(0, shown);
      const hidden = arr.slice(shown);
      for (const c of visibleChildren) {
        if (!visible.has(c.key)) {
          visible.add(c.key);
          queue.push(c.key);
        }
      }
      if (hidden.length > 0) {
        foldNodes.push({ fold: true, key: `fold:${groupId}`, groupId, parentKey: parent, kind, hidden });
      }
    }
  }

  for (const n of nodes) {
    if (visible.has(n.key)) renderNodes.push({ fold: false, key: n.key, kind: n.kind, node: n });
  }
  const renderEdges: CoverageGraphEdge[] = edges.filter((e) => visible.has(e.src) && visible.has(e.dst));
  for (const f of foldNodes) {
    renderNodes.push(f);
    renderEdges.push({ src: f.key, dst: f.parentKey });
  }
  return { renderNodes, renderEdges };
}

// ---------------------------------------------------------------------------
// G6 data mapping. Custom fields go at the node top level (G6 v5 official force example convention: style/layout callbacks read d.<field> directly).
// ---------------------------------------------------------------------------
export type G6NodeDatum = {
  id: string;
  kind: Kind;
  fold: boolean;
  tested: boolean;
  inScope: boolean;
  lbl: string;
  size: number;
};

export function trunc(s: string, n = 26): string {
  return s.length > n ? `${s.slice(0, n - 1)}…` : s;
}

// Node text in the graph: services omit the full URL and show only port·title·status code; endpoints show only the path.
// Other kinds reuse the backend-provided label.
export function graphLabel(n: CoverageGraphNode): string {
  if (n.kind === "service") {
    const parts: string[] = [];
    if (n.port) parts.push(`:${n.port}`);
    if (n.page_title) parts.push(n.page_title);
    if (n.status_code) parts.push(String(n.status_code));
    // biome-ignore lint/nursery/useNullishCoalescing: empty domain/ip strings must fall through to the next label.
    return parts.length ? parts.join(" · ") : n.domain || n.ip || n.label;
  }
  if (n.kind === "endpoint" && n.url) {
    try {
      return new URL(n.url).pathname || "/";
    } catch {
      /* invalid URL: fall back to the full label */
    }
  }
  return n.label;
}

export function toG6Nodes(renderNodes: RenderNode[]): G6NodeDatum[] {
  return renderNodes.map((rn) => {
    if (rn.fold) {
      return {
        id: rn.key,
        kind: rn.kind,
        fold: true,
        tested: false,
        inScope: false,
        lbl: t("{count} more {kind}", { count: rn.hidden.length, kind: kindMeta[rn.kind].label }),
        size: 24,
      };
    }
    return {
      id: rn.key,
      kind: rn.kind,
      fold: false,
      tested: rn.node.tested,
      inScope: rn.node.in_scope,
      lbl: trunc(graphLabel(rn.node) || kindMeta[rn.kind].label),
      size: kindMeta[rn.kind].size,
    };
  });
}

// G6's types put custom fields under data, but the official force examples (and the runtime) read d.<field> at the top level.
// Callback parameters use unknown to satisfy the G6 signature; nd() casts back to our top-level shape internally.
export const nd = (d: unknown) => d as G6NodeDatum;

// Tested = solid highlight; in scope but untested = gray; out of scope / folded = lighter gray + dashed stroke.
export function nodeFill(d: G6NodeDatum): string {
  if (d.fold) return "#f1f5f9";
  if (!d.inScope) return "#e2e8f0";
  if (d.tested) return kindMeta[d.kind].hex;
  return "#94a3b8";
}
export function nodeStroke(d: G6NodeDatum): string {
  if (d.fold || !d.inScope) return "#94a3b8";
  if (d.tested) return "#0f766e";
  return "#64748b";
}

// ---------------------------------------------------------------------------
// Drawers: asset nodes show details; fold nodes show the hidden list + "Show more".
