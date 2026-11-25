import type { FindingRetest, TrafficEvidenceRole } from "@/lib/types";

import * as D from "../data";
import { mockApplyAssetScope, mockBuildAssetTree, mockFilterFindings } from "./assets";
import {
  type MockBindingSeed,
  mockBuildSnapshot,
  mockEvidenceBodies,
  mockEvidencePreview,
  mockFindingTrafficVersion,
  mockTrafficBindings,
  mockTrafficCount,
  mockTrafficSummary,
} from "./evidence";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import {
  mockActivity,
  mockConversations,
  mockFindings,
  mockIntents,
  mockRetestMessages,
  mockRetests,
  mockState,
  mockTasks,
} from "./state";

export function routeExploration({ m, path, seg, q, b }: RouteCtx): unknown {
  const task = q.get("task") ?? undefined;
  // ── exploration ──
  if (path === "/exploration/frontier") return D.frontier;
  if (path === "/exploration/findings/stats") {
    const vulnclasses = Array.from(new Set(mockFindings.map((f) => f.vulnclass))).sort();
    // "By task" dropdown: tasks with findings + description + count (mock task ids are strings, used directly as ids).
    const taskMap = new Map<string, { name: string; description: string; count: number }>();
    for (const f of mockFindings) {
      if (!f.task_id) continue;
      const owner = mockTasks.find((candidate) => candidate.id === f.task_id);
      const cur = taskMap.get(f.task_id) ?? {
        name: owner?.name ?? "",
        description: f.task_description ?? "",
        count: 0,
      };
      cur.count++;
      taskMap.set(f.task_id, cur);
    }
    const tasks = Array.from(taskMap, ([id, v]) => ({ id, name: v.name, description: v.description, count: v.count }));
    return {
      total: mockFindings.length,
      pending: mockFindings.filter((f) => f.status === "pending").length,
      submitted: mockFindings.filter((f) => (f.report_state ?? "none") === "submitted").length,
      accepted: mockFindings.filter((f) => f.report_state === "accepted").length,
      critical: mockFindings.filter((f) => f.severity === "critical").length,
      high: mockFindings.filter((f) => f.severity === "high").length,
      medium: mockFindings.filter((f) => f.severity === "medium").length,
      low: mockFindings.filter((f) => f.severity === "low").length,
      vulnclasses,
      tasks,
    };
  }
  if (path === "/exploration/findings/bulk-report-state" && m === "POST") {
    const ids = Array.isArray(b.ids) ? b.ids : [];
    const state = typeof b.report_state === "string" ? b.report_state : "";
    let updated = 0;
    let missing = 0;
    const results = ids.map((id) => {
      const key = String(id);
      const f = mockFindings.find((item) => item.id === key);
      if (!f) {
        missing++;
        return { id, found: false, changed: false };
      }
      const from = f.report_state ?? "none";
      const changed = from !== state;
      if (changed) {
        f.report_state = state as typeof f.report_state;
        updated++;
      }
      return { id, found: true, changed, from };
    });
    return { updated, missing, results };
  }
  if (path === "/exploration/findings/asset-tree") {
    const list = mockApplyAssetScope(mockFilterFindings(q), null);
    return { nodes: mockBuildAssetTree(list), finding_total: list.length, truncated: false };
  }
  if (path === "/exploration/findings/groups") {
    const severityOrder = { critical: 4, high: 3, medium: 2, low: 1 } as const;
    const list = mockApplyAssetScope(mockFilterFindings(q), q.get("asset_scope"));

    const grouped = new Map<string, typeof list>();
    for (const finding of list) {
      const key = finding.task_id ?? "__unassigned__";
      grouped.set(key, [...(grouped.get(key) ?? []), finding]);
    }
    const groups = Array.from(grouped, ([key, items]) => {
      const owner = mockTasks.find((candidate) => candidate.id === key);
      return {
        task_id: key === "__unassigned__" ? null : key,
        task_name: owner?.name ?? "",
        task_description: owner?.description ?? items[0]?.task_description ?? "",
        task_status: owner?.status ?? "",
        count: items.length,
        critical: items.filter((finding) => finding.severity === "critical").length,
        high: items.filter((finding) => finding.severity === "high").length,
        medium: items.filter((finding) => finding.severity === "medium").length,
        low: items.filter((finding) => finding.severity === "low").length,
        last_found_at: items.reduce((latest, finding) => (finding.ts > latest ? finding.ts : latest), ""),
        max_severity: Math.max(...items.map((finding) => severityOrder[finding.severity])),
      };
    });
    groups.sort((left, right) =>
      q.get("sort") === "severity"
        ? right.max_severity - left.max_severity || right.last_found_at.localeCompare(left.last_found_at)
        : right.last_found_at.localeCompare(left.last_found_at),
    );
    const rawPage = Number(q.get("page") ?? 1);
    const rawPageSize = Number(q.get("limit") ?? 10);
    const page = Number.isFinite(rawPage) && rawPage > 0 ? Math.floor(rawPage) : 1;
    const pageSize = Number.isFinite(rawPageSize) && rawPageSize > 0 ? Math.min(100, Math.floor(rawPageSize)) : 10;
    return {
      items: groups.slice((page - 1) * pageSize, page * pageSize),
      total: groups.length,
      finding_total: list.length,
      page,
      page_size: pageSize,
    };
  }
  if (path === "/exploration/findings/retests/active" && m === "GET") {
    return {
      retests: mockRetests
        .filter((item) => ["pending", "running"].includes(item.status) && item.conversation_id != null)
        .map((item) => ({
          id: item.id,
          finding_id: D.findings[item.finding_id - 1].id,
          conversation_id: item.conversation_id,
          status: item.status,
        })),
    };
  }
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "retests") {
    const finding = mockFindings.find((item) => item.id === seg[2]);
    if (!finding) throw new Error("finding not found");
    const findingID = D.findings.findIndex((item) => item.id === finding.id) + 1;
    if (m === "GET") return { retests: structuredClone(mockRetests.filter((item) => item.finding_id === findingID)) };
    if (m === "POST") {
      const existing = mockRetests.find(
        (item) => item.finding_id === findingID && ["pending", "running"].includes(item.status),
      );
      if (existing) return { retest: structuredClone(existing), created: false };
      const now = new Date().toISOString();
      const conversationID = mockConversations.reduce((max, item) => Math.max(max, item.id), 0) + 1;
      mockConversations.unshift({
        id: conversationID,
        agent_key: "retester",
        title: `Retest #${finding.id} · ${finding.name || finding.vulnclass}`,
        pinned: false,
        created_at: now,
        updated_at: now,
      });
      const retest: FindingRetest = {
        id: mockRetests.length + 1,
        finding_id: findingID,
        conversation_id: conversationID,
        status: "running",
        verdict: "",
        notes: String(b.notes ?? ""),
        summary: "",
        evidence: "",
        error: "",
        created_at: now,
        started_at: now,
        finished_at: null,
      };
      mockRetests.unshift(retest);
      mockRetestMessages[conversationID] = [
        {
          seq: 1,
          worker: "retester",
          ts: now,
          kind: "user",
          summary: `Please retest finding #${finding.id}`,
          detail: retest.notes,
        },
        {
          seq: 2,
          worker: "retester",
          ts: now,
          kind: "text",
          summary: "Demo retest in progress (no requests sent to the target)",
          detail: "Demo retest in progress (no requests sent to the target)",
        },
      ];
      return { retest: structuredClone(retest), created: true };
    }
  }
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "deepen" && m === "POST") {
    const finding = mockFindings.find((candidate) => candidate.id === seg[2]);
    const description = String(b.description ?? "").trim();
    if (!description) throw new Error("description is required");
    if ([...description].length > 4000) throw new Error("description must be at most 4000 characters");
    if (!finding) throw new Error("finding not found");
    if (!finding.task_id || !mockTasks.some((candidate) => candidate.id === finding.task_id)) {
      throw new Error("finding origin task or node is no longer available");
    }
    return {
      task_id: finding.task_id,
      intent_id: `mock-deepen-${Date.now()}`,
      state: "open",
      queued: false,
    };
  }
  // Related traffic evidence: list / bind / edit / unbind / reorder / single message detail (in-memory in the demo).
  // seg = ["exploration","findings",<id>,"traffic", ...]
  if (seg[0] === "exploration" && seg[1] === "findings" && seg[3] === "traffic") {
    const findingID = seg[2];
    if (!mockFindings.some((item) => item.id === findingID)) throw new Error("finding not found");
    const bindings = mockTrafficBindings(findingID);
    const bumpVersion = () => {
      mockFindingTrafficVersion[findingID] = (mockFindingTrafficVersion[findingID] ?? 1) + 1;
    };

    // Single message detail: GET /traffic/{binding_id}
    if (seg.length === 5 && seg[4] !== "order" && m === "GET") {
      const binding = bindings.find((item) => item.id === seg[4]);
      if (!binding) throw new Error("evidence not found");
      const body = mockEvidenceBodies[binding.snapshot.source_traffic_id] ?? { req: "", resp: "" };
      return {
        binding: structuredClone(binding),
        request: mockEvidencePreview(body.req),
        response: mockEvidencePreview(body.resp),
      };
    }
    // Message body pagination: GET /traffic/{binding_id}/body — demo bodies are not truncated; return an empty continuation page.
    if (seg.length === 6 && seg[5] === "body" && m === "GET") {
      const binding = bindings.find((item) => item.id === seg[4]);
      if (!binding) throw new Error("evidence not found");
      const body = mockEvidenceBodies[binding.snapshot.source_traffic_id] ?? { req: "", resp: "" };
      const side = q.get("side") === "request" ? body.req : body.resp;
      return mockEvidencePreview(side);
    }
    // Bind traffic: POST /traffic
    if (seg.length === 4 && m === "POST") {
      const refs = Array.isArray(b.traffic_refs) ? (b.traffic_refs as MockBindingSeed[]) : [];
      for (const ref of refs) {
        if (!ref.traffic_id || bindings.some((item) => item.snapshot.source_traffic_id === ref.traffic_id)) continue;
        bindings.push({
          id: String(++mockState.bindingSeq),
          finding_id: findingID,
          snapshot_id: `snap-${ref.traffic_id}`,
          role: ref.role ?? "supporting",
          note: ref.note ?? "",
          position: bindings.length,
          created_at: new Date().toISOString(),
          snapshot: mockBuildSnapshot(ref.traffic_id),
        });
      }
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // Reorder: PUT /traffic/order
    if (seg.length === 5 && seg[4] === "order" && m === "PUT") {
      const order = Array.isArray(b.binding_ids) ? (b.binding_ids as string[]) : [];
      bindings.sort((left, right) => order.indexOf(left.id) - order.indexOf(right.id));
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // Edit note / unbind: PATCH|DELETE /traffic/{binding_id}
    if (seg.length === 5 && (m === "PATCH" || m === "DELETE")) {
      const index = bindings.findIndex((item) => item.id === seg[4]);
      if (index < 0) throw new Error("evidence not found");
      if (m === "DELETE") {
        bindings.splice(index, 1);
      } else {
        if (typeof b.role === "string") bindings[index].role = b.role as TrafficEvidenceRole;
        if (typeof b.note === "string") bindings[index].note = b.note;
      }
      bumpVersion();
      return mockTrafficSummary(findingID);
    }
    // List: GET /traffic
    return mockTrafficSummary(findingID);
  }
  // Single finding: GET detail / PATCH status/severity/name/class (the demo mutates the in-memory object directly).
  if (seg[0] === "exploration" && seg[1] === "findings" && seg.length === 3 && seg[2] !== "stats") {
    const f = mockFindings.find((x) => x.id === seg[2]);
    if (!f) return {};
    if (m === "PATCH") {
      if (typeof b.status === "string") f.status = b.status as typeof f.status;
      if (typeof b.report_state === "string") f.report_state = b.report_state as typeof f.report_state;
      if (typeof b.severity === "string") f.severity = b.severity as typeof f.severity;
      if (typeof b.name === "string") f.name = b.name;
      if (typeof b.vulnclass === "string") f.vulnclass = b.vulnclass;
    }
    const contextTaskId = q.get("context_task");
    const contextTask = contextTaskId ? mockTasks.find((item) => item.id === contextTaskId) : undefined;
    const inherited = !!(
      contextTask &&
      f.task_id &&
      f.task_id !== contextTask.id &&
      contextTask.source_task_ids?.includes(f.task_id)
    );
    return {
      ...f,
      finding_id: f.id,
      traffic_count: mockTrafficCount(f.id),
      ...(inherited ? { inherited: true, source_task_id: f.task_id } : {}),
    };
  }
  if (path === "/exploration/findings") {
    // finding_id=id: the real backend uses the separate table row id as the status/detail handle; the mock uses its own id instead.
    // report is only returned by the detail endpoint and stripped from lists (consistent with the backend).
    const withFid = (f: (typeof mockFindings)[number]) => ({
      ...f,
      report: undefined,
      finding_id: f.id,
      traffic_count: mockTrafficCount(f.id),
    });
    if (task) {
      const owner = mockTasks.find((item) => item.id === task);
      const sources = new Set(owner?.source_task_ids ?? []);
      return mockFindings
        .filter((f) => f.task_id === task || (!!f.task_id && sources.has(f.task_id)))
        .map((f) => ({
          ...withFid(f),
          ...(f.task_id !== task ? { inherited: true, source_task_id: f.task_id } : {}),
        }));
    }
    // Global: with page/limit → a pagination object; otherwise a bare array (dashboard).
    if (!q.has("page") && !q.has("limit")) return mockFindings.map(withFid);
    const sev = { critical: 4, high: 3, medium: 2, low: 1 } as const;
    const list = mockApplyAssetScope(mockFilterFindings(q), q.get("asset_scope"));
    list.sort((a, b) =>
      q.get("sort") === "severity"
        ? sev[b.severity] - sev[a.severity] || +new Date(b.ts) - +new Date(a.ts)
        : +new Date(b.ts) - +new Date(a.ts),
    );
    const rawPage = Number(q.get("page") ?? 1);
    const rawPageSize = Number(q.get("limit") ?? 20);
    const page = Number.isFinite(rawPage) && rawPage > 0 ? Math.floor(rawPage) : 1;
    const pageSize = Number.isFinite(rawPageSize) && rawPageSize > 0 ? Math.min(200, Math.floor(rawPageSize)) : 20;
    return {
      items: list.slice((page - 1) * pageSize, page * pageSize).map(withFid),
      total: list.length,
      page,
      page_size: pageSize,
    };
  }
  if (path === "/exploration/intents") {
    if (q.has("page")) {
      const before = Number(q.get("before") ?? 0);
      const limit = Math.max(1, Number(q.get("limit") ?? 300));
      let list = mockIntents;
      if (before > 0) list = list.filter((intent) => Number(intent.id.replace(/\D/g, "") || intent.id) < before);
      return { items: list.slice(0, limit), has_more: list.length > limit };
    }
    return mockIntents;
  }
  if (path === "/exploration/tokens") {
    const selectedTask = task ? mockTasks.find((item) => item.id === task) : undefined;
    return {
      workers: D.tokenWorkers,
      sessions: D.tokenSessions,
      total: selectedTask?.tokens ?? D.tokenTotal,
    };
  }
  if (path === "/exploration/graph") return D.explorationGraph;
  // Broadcast board: same semantics as the backend /exploration/nodes — paginated by creation order (ts + id in the mock),
  // also returning the edges involved on this page plus the nodes at the other ends.
  if (path === "/exploration/nodes") {
    const all = D.explorationGraph.nodes;
    const kinds = new Set((q.get("kind") ?? "").split(",").filter(Boolean));
    const states = new Set((q.get("state") ?? "").split(",").filter(Boolean));
    const needle = (q.get("q") ?? "").trim().toLowerCase();
    const asc = q.get("order") === "asc";
    const page = Math.max(1, Number(q.get("page") ?? 1));
    const size = Math.min(200, Math.max(1, Number(q.get("size") ?? 20)));
    const rank = (id: string) => all.findIndex((n) => n.id === id);
    const matched = all
      .filter((n) => (kinds.size === 0 || kinds.has(n.type)) && (states.size === 0 || states.has(n.state)))
      .filter(
        (n) =>
          !needle ||
          // Matching content / source / node id is enough (the id accepts the "#41" form).
          `${n.payload ?? ""} ${n.origin} ${n.id}`.toLowerCase().includes(needle.replace(/^#/, "")),
      )
      .sort((a, b) => {
        const d = Date.parse(a.ts) - Date.parse(b.ts) || rank(a.id) - rank(b.id);
        return asc ? d : -d;
      });
    const items = matched.slice((page - 1) * size, page * size);
    const onPage = new Set(items.map((n) => n.id));
    const edges = D.explorationGraph.edges.filter((e) => onPage.has(e.src) || onPage.has(e.dst));
    const refs: Record<string, (typeof all)[number]> = {};
    for (const e of edges) {
      for (const id of [e.src, e.dst]) {
        if (onPage.has(id) || refs[id]) continue;
        const node = all.find((n) => n.id === id);
        if (node) refs[id] = node;
      }
    }
    // Also return the assets anchored by this page's nodes (including neighbors) for the expansion view.
    const assets: Record<string, ReturnType<typeof D.nodeAssetsFor>> = {};
    for (const id of new Set([...onPage, ...Object.keys(refs)])) {
      const anchored = D.nodeAssetsFor(id);
      if (anchored.length > 0) assets[id] = anchored;
    }
    return { items, total: matched.length, page, size, edges, refs, assets };
  }
  if (path === "/exploration/activity" && seg.length === 2) {
    const since = Number(q.get("since") ?? 0);
    const limit = Math.max(1, Number(q.get("limit") ?? 300));
    const items = mockActivity.filter((item) => item.seq > since).slice(0, limit);
    return { items, cursor: items.length ? items[items.length - 1].seq : since };
  }
  if (seg[0] === "exploration" && seg[1] === "activity" && seg.length === 3) {
    const a = mockActivity.find((x) => x.seq === Number(seg[2]));
    return { detail: a?.detail ?? a?.summary ?? "" };
  }
  return NO_ROUTE;
}
