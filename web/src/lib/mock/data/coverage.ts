import type { FindingAsset } from "@/lib/types";

import { assetRef, assets } from "./assets";

export const coverage = {
  enabled: true,
  scope_rows: 4,
  denominator: 26,
  tested: 11,
  pct: 11 / 26,
  by_type: [
    { type: "subdomain", total: 6, tested: 4 },
    { type: "service", total: 9, tested: 4 },
    { type: "endpoint", total: 8, tested: 2 },
    { type: "ip", total: 3, tested: 1 },
  ],
};

// ── Asset coverage graph (/tasks/{id}/coverage-graph) ──
export const coverageGraph = {
  nodes: [
    { key: "c:1", kind: "company", label: "Acme Corp", tested: false, in_scope: false, company_id: 1 },
    {
      key: "a:1",
      kind: "root_domain",
      label: "acme.com",
      tested: false,
      in_scope: false,
      asset_id: 1,
      domain: "acme.com",
      company_id: 1,
    },
    {
      key: "a:2",
      kind: "subdomain",
      label: "www.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 2,
      domain: "www.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:3",
      kind: "subdomain",
      label: "api.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 3,
      domain: "api.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:4",
      kind: "subdomain",
      label: "admin.acme.com",
      tested: false,
      in_scope: true,
      asset_id: 4,
      domain: "admin.acme.com",
      root_domain: "acme.com",
    },
    {
      key: "a:5",
      kind: "service",
      label: "www.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 5,
      domain: "www.acme.com",
      url: "https://www.acme.com",
      port: 443,
      page_title: "Acme home",
      status_code: 200,
    },
    {
      key: "a:6",
      kind: "endpoint",
      label: "https://www.acme.com/search",
      tested: true,
      in_scope: true,
      asset_id: 6,
      url: "https://www.acme.com/search?q=",
      domain: "www.acme.com",
    },
    {
      key: "a:7",
      kind: "service",
      label: "api.acme.com",
      tested: true,
      in_scope: true,
      asset_id: 7,
      domain: "api.acme.com",
      url: "https://api.acme.com",
      port: 443,
      page_title: "API Gateway",
      status_code: 401,
    },
    {
      key: "a:8",
      kind: "endpoint",
      label: "https://api.acme.com/v1/orders",
      tested: false,
      in_scope: true,
      asset_id: 8,
      url: "https://api.acme.com/v1/orders?id=",
      domain: "api.acme.com",
    },
    {
      key: "a:9",
      kind: "service",
      label: "admin.acme.com",
      tested: false,
      in_scope: true,
      asset_id: 9,
      domain: "admin.acme.com",
      url: "https://admin.acme.com",
      port: 443,
      page_title: "Admin login",
      status_code: 200,
    },
    {
      key: "a:10",
      kind: "ip",
      label: "203.0.113.10",
      tested: true,
      in_scope: true,
      asset_id: 10,
      ip: "203.0.113.10",
      company_id: 1,
    },
  ],
  edges: [
    { src: "a:1", dst: "c:1" },
    { src: "a:2", dst: "a:1" },
    { src: "a:3", dst: "a:1" },
    { src: "a:4", dst: "a:1" },
    { src: "a:5", dst: "a:2" },
    { src: "a:6", dst: "a:5" },
    { src: "a:7", dst: "a:3" },
    { src: "a:8", dst: "a:7" },
    { src: "a:9", dst: "a:4" },
    { src: "a:10", dst: "c:1" },
  ],
};

// ── Intents/facts/findings linked to the asset in this task (/tasks/{id}/asset-refs) ──
// Broadcast board demo: exploration nodes → their anchored assets. The real backend reads exploration_anchors; the mock provides a few static entries.
const NODE_ASSETS: Record<string, number[]> = {
  fi1: [3],
  fi2: [2, 4],
  fi3: [5],
  fiRce: [4],
  fiPriv: [19],
  fiJenkins: [21],
  i1: [2],
  i7: [4],
};

export function nodeAssetsFor(nodeId: string): FindingAsset[] {
  return (NODE_ASSETS[nodeId] ?? []).flatMap((id) => {
    const asset = assets.find((candidate) => candidate.id === id);
    return asset ? [assetRef(id)] : [];
  });
}

export function assetRefsFor(_assetId: number) {
  return {
    intents: [
      { id: 12, kind: "intent", state: "done", summary: "SQL injection probing against the www.acme.com search API" },
      {
        id: 18,
        kind: "intent",
        state: "running",
        summary: "Enumerating object-level authorization bypass (IDOR) on api.acme.com",
      },
    ],
    facts: [
      {
        id: 34,
        kind: "fact",
        state: "confirmed",
        summary: "The search?q= parameter is injectable; the error echoes a MySQL syntax error",
      },
    ],
    findings: [{ id: 41, kind: "finding", state: "confirmed", summary: "[High] SQL injection www.acme.com/search?q=" }],
  };
}

// ── Workspace file manager (/workspace/*, demo: static sample tree) ──
