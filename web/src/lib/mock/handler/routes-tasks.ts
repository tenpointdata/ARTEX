import type { Task, TaskCategory, TaskTemplate } from "@/lib/types";

import { deleteMockTaskAssetSources, mockTaskAssetID, publicMockTask, setMockTaskAssetSource } from "./archive";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import {
  mockActivity,
  mockAssets,
  mockCompanies,
  mockFindings,
  mockIntents,
  mockLLMRecords,
  mockState,
  mockTaskAssetIDs,
  mockTaskCategories,
  mockTasks,
  mockTaskTemplates,
} from "./state";
import {
  bodyIDs,
  controlMockIntent,
  controlMockTask,
  mockRoleResolution,
  mockTaskCategorySnapshot,
  normalizedTemplateName,
  sendMockWorkerMessage,
  sortMockTasks,
} from "./task-helpers";

export function routeTasks({ m, path, seg, b, q }: RouteCtx): unknown {
  const _task = q.get("task") ?? undefined;
  // ── tasks ──
  if (path === "/tasks" && m === "GET") {
    sortMockTasks();
    return { tasks: mockTasks.map(publicMockTask), active: mockState.activeTask };
  }
  if (path === "/tasks" && m === "POST") {
    let suffix = 1;
    while (mockTasks.some((item) => item.id === `t-new-${suffix}`)) suffix++;
    const id = `t-new-${suffix}`;
    const now = new Date();
    const profileIDs = [...((b.llm_profile_ids as number[] | undefined) ?? [])];
    const sourceTaskIDs = [...((b.source_task_ids as string[] | undefined) ?? [])];
    const companyIDs = [...new Set((b.company_ids as number[] | undefined) ?? [])];
    if (companyIDs.some((companyID) => !mockCompanies.some((company) => company.id === companyID))) {
      throw new Error("linked company not found or invalid");
    }
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("task category not found");
    const created: Task = {
      id,
      name: String(b.name ?? ""),
      category_id: category?.id,
      category_name: category?.name,
      description: String(b.description ?? "New task"),
      goal: String(b.goal ?? ""),
      status: "created",
      created_at: now.toISOString(),
      created_unix: Math.floor(now.getTime() / 1000),
      paused: false,
      active: true,
      in_flight: 0,
      findings: { critical: 0, high: 0, medium: 0, low: 0 },
      stalled: false,
      goals_total: 0,
      goals_met: 0,
      engine_mode: "idle",
      tokens: { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 },
      llm_profile_id: profileIDs[0],
      llm_profile_ids: profileIDs,
      llm_rotate: b.llm_rotate === true,
      active_llm_profile_id: profileIDs[0],
      llm_failover_state: profileIDs.length ? "ready" : "default",
      source_task_ids: sourceTaskIDs,
      company_ids: companyIDs,
    };
    const numericTaskID = mockState.nextTaskAssetID++;
    mockTaskAssetIDs.set(id, numericTaskID);
    const selectedCompanies = new Set(companyIDs);
    for (const asset of mockAssets) {
      if (asset.company_id === undefined || !selectedCompanies.has(asset.company_id)) continue;
      if (!asset.task_ids.includes(numericTaskID)) asset.task_ids.push(numericTaskID);
      const company = mockCompanies.find((candidate) => candidate.id === asset.company_id);
      setMockTaskAssetSource(id, asset.id, {
        task_source: "company",
        task_source_summary: `Company linked at task creation: ${company?.name ?? `#${asset.company_id}`}`,
        task_source_node_id: undefined,
      });
    }
    for (const item of mockTasks) item.active = false;
    mockTasks.unshift(created);
    mockState.activeTask = id;
    return created;
  }
  if (path === "/task-categories" && m === "GET") return { categories: mockTaskCategorySnapshot() };
  if (path === "/task-categories" && m === "POST") {
    const name = normalizedTemplateName(b.name);
    if (!name) throw new Error("category name cannot be empty");
    if (mockTaskCategories.some((category) => category.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("category name already exists");
    }
    const now = new Date().toISOString();
    const category: TaskCategory = {
      id: mockTaskCategories.reduce((maximum, item) => Math.max(maximum, item.id), 0) + 1,
      name,
      task_count: 0,
      created_at: now,
      updated_at: now,
    };
    mockTaskCategories.push(category);
    return category;
  }
  if (seg[0] === "task-categories" && seg.length === 2 && m === "PATCH") {
    const category = mockTaskCategories.find((item) => item.id === Number(seg[1]));
    if (!category) throw new Error("task category not found");
    const name = normalizedTemplateName(b.name);
    if (!name) throw new Error("category name cannot be empty");
    if (mockTaskCategories.some((item) => item.id !== category.id && item.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("category name already exists");
    }
    category.name = name;
    category.updated_at = new Date().toISOString();
    for (const task of mockTasks) {
      if (task.category_id === category.id) task.category_name = name;
    }
    return { ...category, task_count: mockTasks.filter((task) => task.category_id === category.id).length };
  }
  if (seg[0] === "task-categories" && seg.length === 2 && m === "DELETE") {
    const categoryID = Number(seg[1]);
    const index = mockTaskCategories.findIndex((item) => item.id === categoryID);
    if (index < 0) throw new Error("task category not found");
    mockTaskCategories.splice(index, 1);
    for (const task of mockTasks) {
      if (task.category_id !== categoryID) continue;
      task.category_id = undefined;
      task.category_name = undefined;
    }
    return { deleted: categoryID };
  }
  if (path === "/tasks/category/batch" && m === "POST") {
    const requested = Array.isArray(b.task_ids) ? b.task_ids.map(String) : [];
    const taskIDs = [...new Set(requested)];
    if (taskIDs.length === 0 || taskIDs.length > 100) throw new Error("task_ids must contain 1-100 items");
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("task category not found");
    const items = taskIDs.map((id) => {
      const task = mockTasks.find((item) => item.id === id);
      if (!task) return { id, ok: false, error: "task not found" };
      task.category_id = category?.id;
      task.category_name = category?.name;
      return { id, ok: true };
    });
    return {
      items,
      category: category ? mockTaskCategorySnapshot().find((item) => item.id === category.id) : null,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "category" && seg.length === 3 && m === "PATCH") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("task not found");
    const categoryID = typeof b.category_id === "number" ? b.category_id : undefined;
    const category = categoryID === undefined ? undefined : mockTaskCategories.find((item) => item.id === categoryID);
    if (categoryID !== undefined && !category) throw new Error("task category not found");
    task.category_id = category?.id;
    task.category_name = category?.name;
    return task;
  }
  if (path === "/task-templates" && m === "GET") return { templates: mockTaskTemplates };
  if (path === "/task-templates" && m === "POST") {
    const now = new Date().toISOString();
    const name = normalizedTemplateName(b.name);
    const description = String(b.description ?? "").trim();
    const goal = String(b.goal ?? "").trim();
    if (!name || !description || !goal) throw new Error("fill in the template name, description and goal");
    if (
      mockTaskTemplates.some((template) => normalizedTemplateName(template.name).toLowerCase() === name.toLowerCase())
    ) {
      throw new Error("template name already exists");
    }
    const nextID = mockTaskTemplates.reduce((max, template) => Math.max(max, template.id), 0) + 1;
    const created: TaskTemplate = {
      id: nextID,
      name,
      description,
      goal,
      created_at: now,
      updated_at: now,
    };
    mockTaskTemplates.unshift(created);
    return created;
  }
  if (seg[0] === "task-templates" && seg.length === 2 && m === "PATCH") {
    const template = mockTaskTemplates.find((item) => item.id === Number(seg[1]));
    if (!template) return {};
    const name = typeof b.name === "string" ? normalizedTemplateName(b.name) : template.name;
    const description = typeof b.description === "string" ? b.description.trim() : template.description;
    const goal = typeof b.goal === "string" ? b.goal.trim() : template.goal;
    if (!name || !description || !goal) throw new Error("fill in the template name, description and goal");
    if (
      mockTaskTemplates.some(
        (item) => item.id !== template.id && normalizedTemplateName(item.name).toLowerCase() === name.toLowerCase(),
      )
    ) {
      throw new Error("template name already exists");
    }
    template.name = name;
    template.description = description;
    template.goal = goal;
    template.updated_at = new Date().toISOString();
    mockTaskTemplates.sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id - a.id);
    return template;
  }
  if (seg[0] === "task-templates" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    const index = mockTaskTemplates.findIndex((item) => item.id === id);
    if (index >= 0) mockTaskTemplates.splice(index, 1);
    return { deleted: id };
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "GET") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("task not found");
    return publicMockTask(task);
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "PATCH") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("task not found");
    if (typeof b.name === "string") task.name = b.name.trim();
    if (typeof b.pinned === "boolean") {
      task.pinned = b.pinned;
      task.pinned_at = b.pinned ? (task.pinned_at ?? new Date().toISOString()) : null;
    }
    sortMockTasks();
    return structuredClone(task);
  }
  if (seg[0] === "tasks" && seg.length === 2 && m === "DELETE") {
    const id = seg[1];
    const numericTaskID = mockTaskAssetID(id);
    const index = mockTasks.findIndex((item) => item.id === id);
    if (index >= 0) mockTasks.splice(index, 1);
    mockTaskAssetIDs.delete(id);
    deleteMockTaskAssetSources(id);
    if (numericTaskID !== undefined) {
      for (const asset of mockAssets) asset.task_ids = asset.task_ids.filter((taskID) => taskID !== numericTaskID);
    }

    let findingsDeleted = 0;
    if (b.delete_findings) {
      for (let i = mockFindings.length - 1; i >= 0; i--) {
        if (mockFindings[i].task_id !== id) continue;
        mockFindings.splice(i, 1);
        findingsDeleted++;
      }
    } else {
      // PostgreSQL uses ON DELETE SET NULL for retained findings. Keep the mock
      // grouped view consistent by moving them into the unassigned/deleted bucket.
      for (const finding of mockFindings) {
        if (finding.task_id !== id) continue;
        finding.task_id = undefined;
        finding.task_description = "";
      }
    }

    let llmRecordsDeleted = 0;
    if (b.delete_llm_records) {
      for (let i = mockLLMRecords.length - 1; i >= 0; i--) {
        if (mockLLMRecords[i].task_id !== id) continue;
        mockLLMRecords.splice(i, 1);
        llmRecordsDeleted++;
      }
    }

    if (mockState.activeTask === id) {
      const nextActive = mockTasks[0]?.id ?? "";
      mockState.activeTask = nextActive;
      for (const item of mockTasks) item.active = item.id === nextActive;
    }
    return {
      deleted: id,
      assets_deleted: b.delete_assets ? 1 : 0,
      assets_detached: 0,
      traffic_deleted: b.delete_traffic ? 1 : 0,
      files_deleted: Boolean(b.delete_files),
      findings_deleted: findingsDeleted,
      llm_records_deleted: llmRecordsDeleted,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "llm" && m === "PUT") {
    const ids = [...((b.llm_profile_ids as number[] | undefined) ?? [])];
    const activeID = typeof b.active_llm_profile_id === "number" ? b.active_llm_profile_id : ids[0];
    const target = mockTasks.find((item) => item.id === seg[1]);
    if (target) {
      target.llm_profile_ids = ids;
      if (typeof b.llm_rotate === "boolean") target.llm_rotate = b.llm_rotate;
      target.llm_profile_id = ids[0];
      target.active_llm_profile_id = activeID;
      target.llm_failover_state = ids.length ? "ready" : "default";
      target.llm_failover_reason = undefined;
    }
    return {
      id: seg[1],
      llm_profile_ids: ids,
      active_llm_profile_id: activeID,
      llm_failover_state: ids.length ? "ready" : "default",
      reopened_intents: 0,
    };
  }
  if (seg[0] === "tasks" && seg[2] === "llm" && seg[3] === "resolution" && m === "GET") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    if (!task) throw new Error("task not found");
    return {
      mainagent: mockRoleResolution(task, "mainagent"),
      planner: mockRoleResolution(task, "planner"),
      worker: mockRoleResolution(task, "worker"),
    };
  }
  if (path === "/tasks/control/batch" && m === "POST") {
    const action = b.action === "resume" ? "resume" : "pause";
    return { items: bodyIDs(b.task_ids).map((id) => controlMockTask(id, action)) };
  }
  if (seg[0] === "tasks" && seg[2] === "intents" && seg[4] === "control" && m === "POST") {
    const id = seg[3];
    if (b.action === "cancel") {
      const index = mockIntents.findIndex((item) => item.id === id);
      const intent = mockIntents[index];
      if (
        !intent ||
        intent.inherited ||
        (intent.state !== "running" && intent.state !== "paused" && intent.state !== "open")
      ) {
        throw new Error("Worker state has changed");
      }
      const numId = Number(id.replace(/\D/g, "")) || 0;
      if (b.mode === "hard") {
        // Hard delete: remove from the list and return the cascading delete count.
        mockIntents.splice(index, 1);
        return {
          id: numId,
          state: "",
          deleted: {
            intents: 1,
            facts: 1,
            findings: 1,
            activities: mockActivity.filter((item) => item.intent_id === id).length,
          },
        };
      }
      // Soft delete (default): set deleted + record the delete reason, keep the node.
      intent.state = "deleted";
      intent.delete_reason = String(b.reason ?? "");
      return { id: numId, state: "deleted" };
    }
    const action = b.action === "resume" ? "resume" : "pause";
    const result = controlMockIntent(id, action);
    if (!result.ok) throw new Error(result.error ?? "Worker state has changed");
    return { id: Number(id.replace(/\D/g, "")) || 0, state: result.state };
  }
  if (seg[0] === "tasks" && seg[2] === "intents" && seg[4] === "messages" && m === "POST") {
    const id = seg[3];
    const result = sendMockWorkerMessage(id, String(b.message ?? ""), String(b.request_id ?? ""));
    if (!result.ok) throw new Error(result.error ?? "Worker state has changed");
    return {
      id: Number(id.replace(/\D/g, "")) || 0,
      state: result.state ?? "running",
      accepted: true,
      request_id: result.requestId ?? "",
    };
  }
  if (seg[0] === "tasks" && seg.length === 3 && seg[2] === "control" && m === "POST") {
    const action = b.action === "resume" ? "resume" : "pause";
    const result = controlMockTask(seg[1], action);
    if (!result.ok) throw new Error(result.error ?? "task state has changed");
    const task = mockTasks.find((item) => item.id === seg[1]);
    return { id: seg[1], paused: Boolean(task?.paused), queued: Boolean(task?.queued), status: task?.status ?? "" };
  }
  if (seg[0] === "tasks" && seg[2] === "chat" && seg[3] === "status") return { running: false };
  if (seg[0] === "tasks" && seg[2] === "chat" && seg[3] === "stop") return { status: "stopped" };
  if (path === "/active") {
    const id = String(b.id ?? mockState.activeTask);
    if (mockTasks.some((item) => item.id === id)) {
      mockState.activeTask = id;
      for (const item of mockTasks) item.active = item.id === id;
    }
    return { active: mockState.activeTask };
  }

  return NO_ROUTE;
}
