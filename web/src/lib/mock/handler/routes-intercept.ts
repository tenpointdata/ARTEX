import * as D from "../data";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import { mockInterceptDetails, mockInterceptHistory, mockInterceptPending } from "./state";

export function routeIntercept({ m, path, seg, q, b }: RouteCtx): unknown {
  const _task = q.get("task") ?? undefined;
  // ── intercept ──
  if (path === "/intercept/rules" && m === "GET") return { rules: D.interceptRules };
  if (seg[0] === "intercept" && seg[1] === "rules" && seg[3] === "toggle")
    return { ok: true, enabled: b.enabled ?? true };
  if (path === "/intercept/pending" && m === "GET")
    return { pending: mockInterceptHistory.filter((r) => r.status === "pending") };
  if (seg[0] === "intercept" && seg[1] === "pending" && seg[3] === "decide") {
    const id = Number(seg[2]);
    const row = mockInterceptHistory.find((r) => r.id === id) ?? mockInterceptPending.find((r) => r.id === id);
    if (row?.status !== "pending") throw new Error("approval already processed or not found; refresh the records");
    if (b.decision !== "allowed" && b.decision !== "denied") throw new Error("invalid approval action");
    row.status = b.decision;
    row.decided_at = new Date().toISOString();
    const detail = mockInterceptDetails[id];
    if (detail) {
      detail.effective_action = b.decision === "allowed" ? "allow" : "deny";
      detail.decision_reason = b.decision === "allowed" ? "Manually allowed" : "Manually denied";
      detail.execution_status = b.decision === "allowed" ? "unknown" : "not_executed";
      detail.output = b.decision === "allowed" ? "Demo mode did not execute the tool." : "";
    }
    return { ok: true };
  }
  if (seg[0] === "intercept" && seg[1] === "history" && seg.length === 3) {
    const row = mockInterceptHistory.find((r) => r.id === Number(seg[2]));
    if (!row) throw new Error("approval record not found");
    return { ...row, audit: mockInterceptDetails[row.id] ?? null };
  }
  if (seg[0] === "intercept" && seg[1] === "pending" && seg.length === 3 && m === "GET")
    return mockInterceptPending.find((p) => p.id === Number(seg[2])) ?? null;
  if (path === "/intercept/history" || (seg[0] === "intercept" && seg[1] === "task")) {
    const status = q.get("status") || "";
    const decisionSource = q.get("decision_source") || "";
    if (status && !["pending", "allowed", "denied", "timeout"].includes(status))
      throw new Error("invalid approval status");
    if (decisionSource && !["model", "rule", "unknown"].includes(decisionSource))
      throw new Error("invalid decision source");
    const decidedBy = q.get("decided_by") || "";
    if (decidedBy && !["human", "rule", "model", "system"].includes(decidedBy)) throw new Error("invalid decided_by");
    const filtered = mockInterceptHistory.filter((row) => {
      const source =
        row.decision_source || (row.rule_id ? "rule" : row.reason?.startsWith("[model]") ? "model" : "unknown");
      return (
        (seg[1] !== "task" || row.task_id === decodeURIComponent(seg[2])) &&
        (!status || row.status === status) &&
        (!decisionSource || source === decisionSource) &&
        (!decidedBy || row.decided_by === decidedBy)
      );
    });
    if (!q.has("page") && !q.has("size") && !status && !decisionSource && !decidedBy)
      return { items: filtered, total: filtered.length };
    const page = Math.max(1, Number(q.get("page")) || 1);
    const size = Math.min(100, Math.max(1, Number(q.get("size")) || 20));
    const offset = (page - 1) * size;
    return {
      items: filtered.slice(offset, offset + size),
      total: filtered.length,
      page,
      page_size: size,
    };
  }
  if (path === "/intercept/tool-config") return { enabled_tools: ["bash"] };
  if (path === "/intercept/judge" && m === "GET")
    return {
      enabled: false,
      profile_id: 0,
      prompt: "",
      timeout_seconds: 15,
      fail_action: "allow",
      ask_timeout_seconds: 300,
      ask_timeout_action: "deny",
    };
  if (path === "/intercept/judge" && m === "PUT") return { ok: true };
  if (path === "/intercept/judge/usage" && m === "GET")
    return {
      calls: 0,
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      daily: [],
    };

  // ── Side questions (/btw): no side-question sessions in the demo ──
  // Must match explicitly: a path ending in s is treated as a collection by the read fallback below and returns [], leaving items undefined.
  if (seg.at(-1) === "side-questions") {
    if (m === "GET") return { items: [], current: null, next_cursor: 0, snapshot: null };
    if (m === "POST") throw new Error("demo mode does not support side questions");
  }

  return NO_ROUTE;
}
