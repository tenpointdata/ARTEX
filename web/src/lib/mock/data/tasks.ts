import type { Finding, Stats, Task, TaskCategory, TaskTemplate, TokenTotal } from "@/lib/types";

import { assets } from "./assets";
import { findings } from "./findings";

const T = (iso: string) => iso; // readability helper for timestamps

// ── Tasks ──────────────────────────────────────────────────────────────────
export const ACTIVE_TASK = "t-acme-web";

export const tasks: Task[] = [
  {
    id: "t-acme-web",
    name: "Acme external web & admin",
    category_id: 1,
    category_name: "External assessment",
    description: "External penetration of the Acme official site and admin (acme.com)",
    goal: "Gain admin access to acme.com and confirm user sensitive data can be read.",
    status: "running",
    created_at: T("2026-11-12T09:12:00Z"),
    created_unix: 1795079520,
    last_activity: T("2026-11-14T03:58:22Z"),
    last_activity_unix: 1795233502,
    paused: false,
    active: true,
    in_flight: 3,
    findings: { critical: 1, high: 2, medium: 1, low: 0 },
    stalled: false,
    goals_total: 5,
    goals_met: 2,
    engine_mode: "exploring",
    llm_profile_id: 1,
    llm_profile_ids: [1, 2],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: [],
    tokens: { input_tokens: 1284500, output_tokens: 96320, cache_read_tokens: 890400, cache_write_tokens: 132000 },
  },
  {
    id: "t-acme-api",
    name: "Acme API authz & injection",
    category_id: 2,
    category_name: "API focus",
    description: "Authorization bypass and injection testing on api.acme.com",
    goal: "Assess authorization bypass (IDOR) and injection risks on the api.acme.com order/user APIs.",
    status: "running",
    created_at: T("2026-11-13T14:05:00Z"),
    created_unix: 1795182900,
    last_activity: T("2026-11-14T03:40:10Z"),
    last_activity_unix: 1795232410,
    paused: false,
    active: false,
    in_flight: 2,
    findings: { critical: 0, high: 1, medium: 1, low: 0 },
    stalled: false,
    goals_total: 4,
    goals_met: 1,
    engine_mode: "exploring",
    llm_profile_id: 1,
    llm_profile_ids: [1, 2],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: ["t-acme-web"],
    tokens: { input_tokens: 642300, output_tokens: 51200, cache_read_tokens: 401000, cache_write_tokens: 60000 },
  },
  {
    id: "t-shop-pay",
    name: "Shop payment flows",
    category_id: 2,
    category_name: "API focus",
    description: "shop.acme.com payment and order flows",
    goal: "Assess authorization bypass, amount tampering and race condition risks on the payment and order APIs.",
    status: "paused",
    created_at: T("2026-11-10T08:30:00Z"),
    created_unix: 1794904200,
    last_activity: T("2026-11-11T18:40:10Z"),
    last_activity_unix: 1795027210,
    paused: true,
    active: false,
    in_flight: 0,
    stalled: false,
    goals_total: 4,
    goals_met: 1,
    engine_mode: "paused",
    llm_profile_id: 2,
    llm_profile_ids: [2, 1],
    active_llm_profile_id: 2,
    llm_failover_state: "ready",
    source_task_ids: ["t-acme-web", "t-acme-api"],
    tokens: { input_tokens: 233000, output_tokens: 18700, cache_read_tokens: 120000, cache_write_tokens: 22000 },
  },
  {
    id: "t-vpn-edge",
    name: "Perimeter recon",
    category_id: 1,
    category_name: "External assessment",
    description: "External attack surface reconnaissance (VPN / perimeter services)",
    goal: "Identify exploitable perimeter services exposed by acme.com.",
    status: "done",
    created_at: T("2026-11-06T08:00:00Z"),
    created_unix: 1794556800,
    completed_at: T("2026-11-08T22:11:00Z"),
    completed_unix: 1794780660,
    last_activity: T("2026-11-08T22:11:00Z"),
    last_activity_unix: 1794780660,
    paused: false,
    active: false,
    in_flight: 0,
    stalled: false,
    goals_total: 3,
    goals_met: 3,
    engine_mode: "idle",
    llm_profile_id: 1,
    llm_profile_ids: [1],
    active_llm_profile_id: 1,
    llm_failover_state: "ready",
    source_task_ids: [],
    tokens: { input_tokens: 512000, output_tokens: 40100, cache_read_tokens: 300000, cache_write_tokens: 41000 },
  },
];

export const taskCategories: TaskCategory[] = [
  {
    id: 1,
    name: "External assessment",
    task_count: 2,
    created_at: T("2026-11-06T08:00:00Z"),
    updated_at: T("2026-11-13T08:00:00Z"),
  },
  {
    id: 2,
    name: "API focus",
    task_count: 2,
    created_at: T("2026-11-07T08:00:00Z"),
    updated_at: T("2026-11-12T08:00:00Z"),
  },
];

export const taskTemplates: TaskTemplate[] = [
  {
    id: 1,
    name: "External web penetration",
    description:
      "Black-box penetration testing of the target's internet-facing exposure, covering sites, APIs and common admin entry points.",
    goal: "Identify and verify security issues that could lead to unauthorized access, sensitive data exposure or server compromise.",
    created_at: T("2026-11-08T08:00:00Z"),
    updated_at: T("2026-11-13T08:00:00Z"),
  },
  {
    id: 2,
    name: "API authorization bypass focus",
    description: "Focused testing of the target API's authentication, object-level authorization and role boundaries.",
    goal: "Confirm whether IDOR, horizontal and vertical privilege escalation, and bulk data access risks exist.",
    created_at: T("2026-11-07T08:00:00Z"),
    updated_at: T("2026-11-12T08:00:00Z"),
  },
];

export const getTask = (id: string) => tasks.find((t) => t.id === id);

export const grandTotal: TokenTotal = tasks.reduce(
  (a, t) => ({
    input_tokens: a.input_tokens + (t.tokens?.input_tokens ?? 0),
    output_tokens: a.output_tokens + (t.tokens?.output_tokens ?? 0),
    cache_read_tokens: a.cache_read_tokens + (t.tokens?.cache_read_tokens ?? 0),
    cache_write_tokens: a.cache_write_tokens + (t.tokens?.cache_write_tokens ?? 0),
  }),
  { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 },
);

// ── Stats ──────────────────────────────────────────────────────────────────
export function stats(
  taskId?: string,
  state: { tasks?: readonly Task[]; findings?: readonly Finding[]; activeTask?: string } = {},
): Stats & { active_task?: unknown } {
  const taskList = state.tasks ?? tasks;
  const findingList = state.findings ?? findings;
  const base: Stats = {
    assets: assets.length,
    engine_mode: "exploring",
    llm_configured: true,
    roe_enabled: true,
    findings_confirmed: findingList.filter((f) => f.severity !== "low").length,
  };
  const selectedTaskID = taskId ?? state.activeTask ?? ACTIVE_TASK;
  const t = taskList.find((item) => item.id === selectedTaskID);
  if (!t) return base;
  return {
    ...base,
    engine_mode: t.engine_mode ?? "idle",
    findings_confirmed: findingList.filter((f) => f.task_id === t.id).length,
    active_task: {
      id: t.id,
      in_flight: t.in_flight,
      goals_total: t.goals_total,
      goals_met: t.goals_met,
      engine_mode: t.engine_mode,
      paused: t.paused,
    },
  };
}
