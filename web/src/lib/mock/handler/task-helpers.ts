import {
  classifyCompanyScopeLine,
  companyScopeRuleError,
  isCompanyScopeKind,
  normalizeCompanyScopeValue,
} from "@/lib/company-scope";
import type {
  Activity,
  BatchControlItem,
  CompanyScopeRule,
  ScopeRow,
  Task,
  TaskCategory,
  TaskLLMResolution,
} from "@/lib/types";

import * as D from "../data";
import { mockActivity, mockCompanies, mockConversations, mockIntents, mockTaskCategories, mockTasks } from "./state";

export function mockTaskCategorySnapshot(): TaskCategory[] {
  return mockTaskCategories.map((category) => ({
    ...category,
    task_count: mockTasks.filter((task) => task.category_id === category.id).length,
  }));
}

export function mockScopeRows(
  companyID: number,
  input: unknown,
  existing: ScopeRow[] = [],
): { rows: ScopeRow[]; invalid: number; skipped: number } {
  if (!Array.isArray(input)) return { rows: [], invalid: 0, skipped: 0 };
  let nextID =
    mockCompanies.flatMap((company) => company.scope ?? []).reduce((max, row) => Math.max(max, row.id), 0) + 1;
  const rows: ScopeRow[] = [];
  let invalid = 0;
  let skipped = 0;
  const keys = new Set(
    existing.map((row) => `${row.kind}|${row.domain ?? row.net ?? row.value ?? row.raw.trim().toLowerCase()}`),
  );
  for (const [index, candidate] of input.entries()) {
    let rule: CompanyScopeRule | undefined;
    if (typeof candidate === "string") {
      rule = classifyCompanyScopeLine(candidate, index + 1).rule;
    } else if (candidate && typeof candidate === "object") {
      const item = candidate as { kind?: unknown; value?: unknown };
      const value = String(item.value ?? "").trim();
      if (item.kind === undefined || item.kind === "") rule = classifyCompanyScopeLine(value, index + 1).rule;
      else if (isCompanyScopeKind(item.kind)) rule = { kind: item.kind, value };
    }
    if (!rule || companyScopeRuleError(rule)) {
      invalid++;
      continue;
    }
    const normalized = normalizeCompanyScopeValue(rule);
    const row: ScopeRow = { id: nextID++, company_id: companyID, kind: rule.kind, raw: rule.value.trim() };
    if (rule.kind === "domain") row.domain = normalized;
    else if (rule.kind === "ip") row.net = `${normalized}/${normalized.includes(":") ? 128 : 32}`;
    else if (rule.kind === "cidr") row.net = normalized;
    else row.value = normalized;
    const key = `${row.kind}|${row.domain ?? row.net ?? row.value}`;
    if (keys.has(key)) {
      skipped++;
      continue;
    }
    keys.add(key);
    rows.push(row);
  }
  return { rows, invalid, skipped };
}

export function mockProfileResolution(
  profileID: number | undefined,
  source: TaskLLMResolution["source"],
): TaskLLMResolution {
  const profile = D.llmProfiles.find((item) => Number(item.id) === profileID);
  if (!profile) {
    return { name: "", format: "", model: "", source, available: false, reason: "LLM configuration not found" };
  }
  return {
    profile_id: Number(profile.id),
    name: profile.name,
    format: profile.format,
    model: profile.model,
    source,
    available: Boolean(profile.api_key_hint),
    reason: profile.api_key_hint ? undefined : "LLM configuration has no API key set",
  };
}

// Mirrors the backend precedence in server/task_resolution.go:
// Agent binding → task LLM configuration chain → global configuration → environment configuration.
export function mockRoleResolution(task: Task, agentKey: "mainagent" | "planner" | "worker"): TaskLLMResolution {
  const agent = D.agents.find((item) => item.key === agentKey);
  if (agent?.llm_profile_id) {
    const bound = mockProfileResolution(agent.llm_profile_id, "agent_binding");
    if (bound.available) return bound;
  }
  if (task.llm_profile_ids?.length) {
    if (task.active_llm_profile_id === undefined) {
      return {
        name: "",
        format: "",
        model: "",
        source: "task_chain",
        available: false,
        reason: "task LLM configuration chain quota exhausted",
      };
    }
    return mockProfileResolution(task.active_llm_profile_id, "task_chain");
  }
  const globalProfile = D.llmProfiles.find((item) => item.is_default);
  if (globalProfile) return mockProfileResolution(Number(globalProfile.id), "global_profile");
  return {
    name: "Global configuration",
    format: D.llmConfig.provider,
    model: D.llmConfig.model,
    source: "environment",
    available: true,
  };
}

export function bodyIDs(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const ids = [...new Set(value.map((item) => String(item)).filter(Boolean))];
  if (ids.length > 100) throw new Error("batch operations support at most 100 IDs");
  return ids;
}

export type MockIntentControlResult = { ok: boolean; state?: string; error?: string };
export type MockWorkerMessage = {
  intentId: string;
  message: string;
  state: "open" | "running";
  activitySeq: number;
};

export const mockWorkerMessages = new Map<string, MockWorkerMessage>();
export let nextMockWorkerMessageActivitySeq =
  mockActivity.reduce((maximum, item) => Math.max(maximum, item.seq), 0) + 1;

export function controlMockIntent(id: string, action: "pause" | "resume"): MockIntentControlResult {
  const intent = mockIntents.find((item) => item.id === id);
  if (!intent) return { ok: false, error: "intent not found" };
  const requiredState = action === "pause" ? "running" : "paused";
  if (intent.inherited || intent.state !== requiredState) {
    return { ok: false, state: intent.state, error: "Worker state has changed" };
  }
  intent.state = action === "pause" ? "paused" : "open";
  return { ok: true, state: intent.state };
}

export function sendMockWorkerMessage(
  id: string,
  message: string,
  requestId: string,
): MockIntentControlResult & { activitySeq?: number; requestId?: string } {
  const normalizedMessage = message.trim();
  const normalizedRequestId = requestId.trim();
  if (!normalizedRequestId) return { ok: false, error: "request_id cannot be empty" };
  const previous = mockWorkerMessages.get(normalizedRequestId);
  if (previous) {
    if (previous.intentId !== id || previous.message !== normalizedMessage) {
      return { ok: false, error: "request_id already used by another Worker message" };
    }
    return {
      ok: true,
      state: previous.state,
      activitySeq: previous.activitySeq,
      requestId: normalizedRequestId,
    };
  }
  const intent = mockIntents.find((item) => item.id === id);
  if (!intent) return { ok: false, error: "intent not found" };
  if (intent.inherited || intent.state !== "paused") {
    return { ok: false, state: intent.state, error: "only a paused Worker can receive messages; pause it first" };
  }
  if (!normalizedMessage) return { ok: false, state: intent.state, error: "message cannot be empty" };
  if (Array.from(normalizedMessage).length > 4000) {
    return { ok: false, state: intent.state, error: "message cannot exceed 4000 characters" };
  }

  // The real endpoint transitions the intent paused->running, records the user turn,
  // and runs it in a dedicated goroutine outside the worker pool. Keep the mock in
  // sync: flip to running immediately and emit the visible user activity.
  intent.state = "running";
  const activitySeq = nextMockWorkerMessageActivitySeq++;
  const workerMessage: MockWorkerMessage = {
    intentId: id,
    message: normalizedMessage,
    state: "running",
    activitySeq,
  };
  mockWorkerMessages.set(normalizedRequestId, workerMessage);
  mockActivity.push({
    seq: activitySeq,
    intent_id: id,
    worker: "user",
    ts: new Date().toISOString(),
    kind: "user",
    summary: normalizedMessage,
    detail: normalizedMessage,
  } satisfies Activity);
  return {
    ok: true,
    state: workerMessage.state,
    activitySeq: workerMessage.activitySeq,
    requestId: normalizedRequestId,
  };
}

export function controlMockTask(id: string, action: "pause" | "resume"): BatchControlItem {
  const task = mockTasks.find((item) => item.id === id);
  if (!task) return { id, ok: false, error: "task not found" };
  if (task.status === "done" || task.status === "failed" || task.status === "timeout") {
    return { id, ok: false, status: task.status, error: "a task in a final state cannot be controlled" };
  }
  if (action === "pause") {
    if (task.paused || task.status === "paused") {
      return { id, ok: false, status: task.status, error: "task is already paused" };
    }
    task.paused = true;
    task.queued = false;
    task.status = "paused";
    task.engine_mode = "paused";
  } else {
    if (!task.paused && task.status !== "paused") {
      return { id, ok: false, status: task.status, error: "task is not paused" };
    }
    task.paused = false;
    task.queued = false;
    task.status = "running";
    task.engine_mode = "exploring";
  }
  return { id, ok: true, status: task.status, queued: false };
}

export function sortMockConversations() {
  mockConversations.sort((a, b) => {
    const aPinned = a.pinned_at ? 1 : 0;
    const bPinned = b.pinned_at ? 1 : 0;
    if (aPinned !== bPinned) return bPinned - aPinned;
    const aTime = a.pinned_at ?? a.updated_at;
    const bTime = b.pinned_at ?? b.updated_at;
    return bTime.localeCompare(aTime) || b.id - a.id;
  });
}

export function sortMockTasks() {
  mockTasks.sort((a, b) => {
    const aPinned = a.pinned_at ? 1 : 0;
    const bPinned = b.pinned_at ? 1 : 0;
    if (aPinned !== bPinned) return bPinned - aPinned;
    if (a.pinned_at !== b.pinned_at) return (b.pinned_at ?? "").localeCompare(a.pinned_at ?? "");
    return b.id.localeCompare(a.id, undefined, { numeric: true });
  });
}

export function normalizedTemplateName(value: unknown): string {
  return String(value ?? "")
    .trim()
    .split(/\s+/)
    .join(" ");
}

export function parseBody(body?: BodyInit | null): Record<string, unknown> {
  if (typeof body !== "string") return {};
  try {
    return JSON.parse(body) as Record<string, unknown>;
  } catch {
    return {};
  }
}
