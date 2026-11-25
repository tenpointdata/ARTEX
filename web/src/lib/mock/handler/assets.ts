import type { Asset } from "@/lib/types";

import { mockTaskAssetID } from "./archive";
import { mockAssets, mockCompanies, mockFindings } from "./state";

export function mockAssetCounts(taskID?: string | null): Record<string, number> {
  const numericTaskID = taskID ? mockTaskAssetID(taskID) : undefined;
  return mockAssets.reduce<Record<string, number>>((counts, asset) => {
    if (taskID && (numericTaskID === undefined || !asset.task_ids.includes(numericTaskID))) return counts;
    counts[asset.type] = (counts[asset.type] ?? 0) + 1;
    return counts;
  }, {});
}

export function mockAssetMatchesDSL(asset: Asset, dsl: string): boolean {
  const query = dsl
    .replaceAll(/[()"]/g, " ")
    .replaceAll(/\b(?:AND|OR)\b/gi, " ")
    .replaceAll(/\b[a-z_][a-z0-9_]*(?:==|!=|>=|<=|=|>|<)/gi, " ")
    .trim()
    .toLowerCase();
  if (!query) return true;
  const haystack = JSON.stringify(asset).toLowerCase();
  return query.split(/\s+/).every((term) => haystack.includes(term));
}

// mockFilterFindings applies the findings page's common filters (severity/status/type/task/keyword); asset
// subtree filtering goes through mockApplyAssetScope — the same division of labor as the backend FindingFilter.where().
export function mockFilterFindings(q: URLSearchParams): (typeof mockFindings)[number][] {
  let list = mockFindings.filter((finding) => mockFindingMatchesQuery(finding, q.get("q")));
  const severity = q.get("severity");
  const status = q.get("status");
  const vulnclass = q.get("vulnclass");
  const taskID = q.get("task_id");
  if (severity) list = list.filter((finding) => finding.severity === severity);
  if (status) list = list.filter((finding) => finding.status === status);
  const reportState = q.get("report_state");
  if (reportState) list = list.filter((finding) => (finding.report_state ?? "none") === reportState);
  if (vulnclass) list = list.filter((finding) => finding.vulnclass === vulnclass);
  if (taskID === "__unassigned__") list = list.filter((finding) => !finding.task_id);
  else if (taskID) list = list.filter((finding) => finding.task_id === taskID);
  return list;
}

export function mockFindingMatchesQuery(finding: (typeof mockFindings)[number], query: string | null): boolean {
  const needle = query?.trim().toLowerCase();
  if (!needle) return true;
  return [finding.name, finding.vulnclass, finding.summary, finding.evidence, finding.report].some((value) =>
    String(value ?? "")
      .toLowerCase()
      .includes(needle),
  );
}

// ── "By asset" view ────────────────────────────────────────────────────────────
// The backend builds the tree in db/finding_assets.go (only assets with findings + ancestors filled in level by level; counts are
// deduplicated and accumulated along ancestor chains). Here the same parent-child priority is replayed in memory so the demo's hierarchy,
// counts, and subtree filtering match the real backend.

export const UNASSIGNED_ASSET = "__none__";

export function mockAssetLabel(asset: (typeof mockAssets)[number]): string {
  // Services without a URL get a port appended; otherwise the label would be identical to the host IP/domain row (consistent with the backend).
  if (asset.type === "service" && !asset.url) {
    const host = asset.domain || asset.ip;
    if (host && asset.port) return `${host}:${asset.port}`;
  }
  return asset.url || asset.domain || asset.ip || asset.app_name || `#${asset.id}`;
}

// mockAssetHost matches the backend hostPortOf: domain first, then the host from the URL, then ip.
export function mockAssetHost(asset: (typeof mockAssets)[number]): { host: string; port: number } {
  let host = asset.domain ?? "";
  let port = asset.port ?? 0;
  if (!host && asset.url) {
    try {
      const url = new URL(asset.url);
      host = url.hostname.replace(/^\[|\]$/g, "");
      if (!port) port = Number(url.port) || (url.protocol === "https:" ? 443 : 80);
    } catch {
      // Fall back to ip for invalid URLs.
    }
  }
  if (!host) host = asset.ip ?? "";
  return { host, port };
}

export interface MockAssetTreeNode {
  key: string;
  parent?: string;
  kind: string;
  label: string;
  asset_id?: number;
  company_id?: number;
  self: number;
  total: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

// mockBuildAssetTree builds the asset tree from a set of findings (already filtered by other conditions).
export function mockBuildAssetTree(list: (typeof mockFindings)[number][]): MockAssetTreeNode[] {
  const hit = new Set<number>();
  for (const finding of list) {
    for (const ref of finding.assets ?? []) hit.add(Number(ref.id));
  }

  // Collect hit assets + fill in ancestors level by level (host service / subdomain / IP / root domain).
  const picked = new Map<number, (typeof mockAssets)[number]>();
  for (const asset of mockAssets) if (hit.has(asset.id)) picked.set(asset.id, asset);
  for (let round = 0; round < 4; round++) {
    const before = picked.size;
    for (const asset of [...picked.values()]) {
      const { host } = mockAssetHost(asset);
      const wanted = mockAssets.filter((candidate) => {
        if (picked.has(candidate.id)) return false;
        if (asset.type === "endpoint" && candidate.type === "service") {
          return mockAssetHost(candidate).host === host;
        }
        if (asset.type === "service" || asset.type === "endpoint") {
          return (
            (candidate.type === "subdomain" && candidate.domain === host) ||
            (candidate.type === "ip" && candidate.ip === host)
          );
        }
        if (asset.type === "subdomain") {
          return candidate.type === "root_domain" && candidate.domain === asset.root_domain;
        }
        return false;
      });
      for (const candidate of wanted) picked.set(candidate.id, candidate);
    }
    if (picked.size === before) break;
  }

  const nodes = new Map<string, MockAssetTreeNode>();
  const parentOf = new Map<string, string>();
  const key = (id: number) => `a:${id}`;
  for (const asset of picked.values()) {
    nodes.set(key(asset.id), {
      key: key(asset.id),
      kind: asset.type,
      label: mockAssetLabel(asset),
      asset_id: asset.id,
      company_id: asset.company_id,
      self: 0,
      total: 0,
      critical: 0,
      high: 0,
      medium: 0,
      low: 0,
      last_found_at: "",
    });
  }

  // Parent-child relation: same priority order as firstOf in db/finding_assets.go.
  const find = (predicate: (a: (typeof mockAssets)[number]) => boolean) => {
    const asset = [...picked.values()].find(predicate);
    return asset ? key(asset.id) : "";
  };
  for (const asset of picked.values()) {
    const { host, port } = mockAssetHost(asset);
    let parent = "";
    if (asset.type === "subdomain") {
      parent = find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    } else if (asset.type === "service") {
      parent =
        find((a) => a.type === "subdomain" && (a.domain === asset.domain || a.domain === host)) ||
        find((a) => a.type === "ip" && (a.ip === asset.ip || a.ip === host)) ||
        find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    } else if (asset.type === "endpoint") {
      parent =
        find((a) => {
          if (a.type !== "service") return false;
          const svc = mockAssetHost(a);
          return svc.host === host && svc.port === port;
        }) ||
        find((a) => a.type === "service" && mockAssetHost(a).host === host) ||
        find((a) => a.type === "subdomain" && (a.domain === host || a.domain === asset.domain)) ||
        find((a) => a.type === "ip" && (a.ip === host || a.ip === asset.ip)) ||
        find((a) => a.type === "root_domain" && a.domain === asset.root_domain);
    }
    if (parent && parent !== key(asset.id)) {
      parentOf.set(key(asset.id), parent);
      const node = nodes.get(key(asset.id));
      if (node) node.parent = parent;
    }
  }

  // Company level: only fill in for top-level assets that actually have an owner (root domain / IP / app); ownerless ones are top-level themselves.
  for (const node of [...nodes.values()]) {
    if (node.parent || !node.company_id) continue;
    if (!["root_domain", "ip", "app"].includes(node.kind)) continue;
    const company = mockCompanies.find((candidate) => candidate.id === node.company_id);
    if (!company) continue;
    const companyKey = `c:${company.id}`;
    if (!nodes.has(companyKey)) {
      nodes.set(companyKey, {
        key: companyKey,
        kind: "company",
        label: company.name,
        company_id: company.id,
        self: 0,
        total: 0,
        critical: 0,
        high: 0,
        medium: 0,
        low: 0,
        last_found_at: "",
      });
    }
    node.parent = companyKey;
    parentOf.set(node.key, companyKey);
  }

  // Counts: a finding walks up each asset's ancestor chain, collects a deduplicated key set, then +1 each.
  const unassigned: MockAssetTreeNode = {
    key: UNASSIGNED_ASSET,
    kind: "none",
    label: "Unlinked assets",
    self: 0,
    total: 0,
    critical: 0,
    high: 0,
    medium: 0,
    low: 0,
    last_found_at: "",
  };
  const bump = (node: MockAssetTreeNode, finding: (typeof mockFindings)[number]) => {
    node.total++;
    if (finding.severity === "critical") node.critical++;
    else if (finding.severity === "high") node.high++;
    else if (finding.severity === "medium") node.medium++;
    else if (finding.severity === "low") node.low++;
    if (finding.ts > node.last_found_at) node.last_found_at = finding.ts;
  };
  for (const finding of list) {
    const direct = (finding.assets ?? []).map((ref) => nodes.get(`a:${ref.id}`)).filter((n) => n !== undefined);
    if (direct.length === 0) {
      bump(unassigned, finding);
      unassigned.self++;
      continue;
    }
    const touched = new Set<string>();
    for (const node of direct) {
      node.self++;
      for (let cur: string | undefined = node.key; cur; cur = parentOf.get(cur)) touched.add(cur);
    }
    for (const k of touched) {
      const node = nodes.get(k);
      if (node) bump(node, finding);
    }
  }

  const out = [...nodes.values()].filter((node) => node.total > 0);
  if (unassigned.total > 0) out.push(unassigned);
  out.sort((left, right) => {
    if ((left.kind === "none") !== (right.kind === "none")) return left.kind === "none" ? 1 : -1;
    return right.total - left.total || left.label.localeCompare(right.label);
  });
  return out;
}

// mockAssetScopeIds expands an asset tree node key into the asset id set of the whole subtree,
// synonymous with the backend applyAssetScope. miss=true means the node does not exist under the current filters → the result is always empty.
export function mockAssetScopeIds(
  scope: string,
  list: (typeof mockFindings)[number][],
): { ids: Set<string>; none: boolean; miss: boolean } {
  if (scope === UNASSIGNED_ASSET) return { ids: new Set(), none: true, miss: false };
  const nodes = mockBuildAssetTree(list);
  if (!nodes.some((node) => node.key === scope)) return { ids: new Set(), none: false, miss: true };
  const children = new Map<string, MockAssetTreeNode[]>();
  for (const node of nodes) {
    if (!node.parent) continue;
    children.set(node.parent, [...(children.get(node.parent) ?? []), node]);
  }
  const ids = new Set<string>();
  const queue = [scope];
  const seen = new Set(queue);
  while (queue.length > 0) {
    const cur = queue.shift() as string;
    const node = nodes.find((candidate) => candidate.key === cur);
    if (node?.asset_id) ids.add(String(node.asset_id));
    for (const child of children.get(cur) ?? []) {
      if (seen.has(child.key)) continue;
      seen.add(child.key);
      queue.push(child.key);
    }
  }
  return { ids, none: false, miss: ids.size === 0 };
}

// mockApplyAssetScope narrows a set of findings by asset_scope. "Unlinked" covers both findings with empty assets
// and findings pointing to deleted assets, matching the bucket on the tree.
export function mockApplyAssetScope(
  list: (typeof mockFindings)[number][],
  scope: string | null,
): (typeof mockFindings)[number][] {
  if (!scope) return list;
  const { ids, none, miss } = mockAssetScopeIds(scope, list);
  if (none) {
    return list.filter((finding) => {
      const refs = finding.assets ?? [];
      return refs.length === 0 || refs.every((ref) => !mockAssets.some((a) => String(a.id) === ref.id));
    });
  }
  if (miss) return [];
  return list.filter((finding) => (finding.assets ?? []).some((ref) => ids.has(ref.id)));
}
