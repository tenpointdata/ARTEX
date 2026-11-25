import { t } from "@/i18n";
import { MOCK } from "@/lib/mock/enabled";
import type {
  Activity,
  ArchiveBatchItem,
  AssetInterceptRule,
  AssetInterceptRuleInput,
  BatchCategoryItem,
  BatchControlItem,
  CoverageAssetRefs,
  CoverageGraphData,
  DeleteTaskOptions,
  DeleteTaskResult,
  MethodologySummary,
  Stats,
  Task,
  TaskArchive,
  TaskArchivePage,
  TaskCategory,
  TaskConstraint,
  TaskCoverage,
  TaskCredential,
  TaskCredentialInput,
  TaskGoal,
  TaskLLMResolutions,
  TaskScopeRow,
  TaskTemplate,
  UsageStats,
  WorkspaceFile,
  WorkspaceListing,
} from "@/lib/types";

import { arr, del, get, getToken, patch, post, put, tq } from "./core";

export const tasksApi = {
  // Backend application version (injected via ldflags at release, default "dev").
  // isolation: result of the startup agent isolation self-test; "degraded" shows a banner on every page.
  health: () =>
    get<{
      ok: boolean;
      service: string;
      version: string;
      latest_version?: string;
      isolation?: { mode: "isolated" | "degraded"; reason?: string };
    }>("/health"),

  // ---- auth ----
  authStatus: () => get<{ initialized: boolean }>("/auth/status"),
  login: (username: string, password: string) => post<{ token: string }>("/auth/login", { username, password }),
  initPassword: (password: string, setupToken: string) =>
    post<{ token: string }>("/auth/init", { password, setup_token: setupToken }),
  changePassword: (oldPassword: string, newPassword: string) =>
    post<{ ok: boolean }>("/auth/change-password", { old_password: oldPassword, new_password: newPassword }),

  // ---- tasks ----
  tasks: (companyId: number | null = null) =>
    get<{ tasks: Task[]; active: string }>(`/tasks${companyId ? `?company=${companyId}` : ""}`).then((r) => ({
      tasks: arr(r.tasks),
      active: r.active ?? "",
    })),
  task: (id: string) => get<Task>(`/tasks/${encodeURIComponent(id)}`),
  createTask: (input: {
    name?: string;
    categoryId?: number;
    description: string;
    goal: string;
    llmProfileIds?: number[];
    llmRotate?: boolean;
    sourceTaskIds?: string[];
    companyIds?: number[];
    timeoutSeconds?: number;
    seedFirstIntent?: boolean;
    planHeartbeatSeconds?: number;
    coverageEnabled?: boolean;
    methodologies?: string[]; // empty = automatic (derived from the goal and description)
    interceptRules?: AssetInterceptRuleInput[];
    credentials?: TaskCredentialInput[];
  }) =>
    post<Task>("/tasks", {
      name: input.name ?? "",
      category_id: input.categoryId ?? null,
      description: input.description,
      goal: input.goal,
      llm_profile_ids: input.llmProfileIds ?? [],
      llm_rotate: input.llmRotate ?? false,
      source_task_ids: input.sourceTaskIds ?? [],
      company_ids: input.companyIds ?? [],
      timeout_seconds: input.timeoutSeconds ?? 0,
      seed_first_intent: input.seedFirstIntent ?? false,
      plan_heartbeat_seconds: input.planHeartbeatSeconds ?? 0, // 0 = the backend normalizes to the default 600 (10min)
      coverage_enabled: input.coverageEnabled ?? true, // on by default; false disables the asset coverage feature
      methodologies: input.methodologies ?? [],
      intercept_rules: input.interceptRules ?? [], // task-level asset interception rules
      credentials: input.credentials ?? [], // test accounts declared at creation; an empty password = the agent generates and records it (#3)
    }),
  methodologies: () => get<MethodologySummary[]>("/methodologies").then((r) => arr(r)),
  taskCategories: () => get<{ categories: TaskCategory[] }>("/task-categories").then((r) => arr(r.categories)),
  updateTask: (id: string, input: { name?: string; pinned?: boolean }) => patch<Task>(`/tasks/${id}`, input),
  renameTask: (id: string, name: string) => patch<Task>(`/tasks/${id}`, { name }),
  pinTask: (id: string, pinned: boolean) => patch<Task>(`/tasks/${id}`, { pinned }),
  createTaskCategory: (name: string) => post<TaskCategory>("/task-categories", { name }),
  taskCredentials: (id: string) => get<TaskCredential[]>(`/tasks/${id}/credentials`).then((r) => arr(r)),
  addTaskCredential: (id: string, input: TaskCredentialInput) =>
    post<TaskCredential>(`/tasks/${id}/credentials`, input),
  deleteTaskCredential: (id: string, cid: number) => del(`/tasks/${id}/credentials/${cid}`),
  renameTaskCategory: (id: number, name: string) => patch<TaskCategory>(`/task-categories/${id}`, { name }),
  deleteTaskCategory: (id: number) => del<{ deleted: number }>(`/task-categories/${id}`),
  updateTaskCategory: (taskId: string, categoryId?: number) =>
    patch<Task>(`/tasks/${taskId}/category`, { category_id: categoryId ?? null }),
  // omitting categoryId / undefined = move out of the category (the backend receives null)
  updateTasksCategory: (taskIds: string[], categoryId?: number) =>
    post<{ items: BatchCategoryItem[]; category: TaskCategory | null }>("/tasks/category/batch", {
      task_ids: taskIds,
      category_id: categoryId ?? null,
    }),
  taskTemplates: () => get<{ templates: TaskTemplate[] }>("/task-templates").then((r) => arr(r.templates)),
  createTaskTemplate: (
    input: Pick<TaskTemplate, "name" | "description" | "goal" | "category_id" | "intercept_rules">,
  ) => post<TaskTemplate>("/task-templates", input),
  updateTaskTemplate: (
    id: number,
    input: Partial<Pick<TaskTemplate, "name" | "description" | "goal" | "category_id" | "intercept_rules">>,
  ) => patch<TaskTemplate>(`/task-templates/${id}`, input),
  deleteTaskTemplate: (id: number) => del<{ deleted: number }>(`/task-templates/${id}`),
  updateTaskLLMProfiles: (id: string, llmProfileIds: number[], activeLLMProfileId?: number, llmRotate?: boolean) =>
    put<{
      id: string;
      llm_profile_ids: number[];
      llm_rotate: boolean;
      active_llm_profile_id?: number;
      llm_failover_state: string;
      reopened_intents: number;
      switch_event?: Activity;
    }>(`/tasks/${id}/llm`, {
      llm_profile_ids: llmProfileIds,
      active_llm_profile_id: activeLLMProfileId ?? null,
      llm_rotate: llmRotate,
    }),
  deleteTask: (id: string, options: DeleteTaskOptions) => del<DeleteTaskResult>(`/tasks/${id}`, options),
  controlTask: (id: string, action: "pause" | "resume") =>
    post<{ id: string; paused: boolean; queued: boolean; status: string }>(`/tasks/${id}/control`, { action }),
  controlTasksBatch: (taskIds: string[], action: "pause" | "resume") =>
    post<{ items: BatchControlItem[] }>("/tasks/control/batch", { task_ids: taskIds, action }),
  taskArchives: (input?: { page?: number; size?: number; q?: string; state?: string }) => {
    const query = new URLSearchParams();
    query.set("page", String(input?.page ?? 1));
    query.set("size", String(input?.size ?? 20));
    if (input?.q) query.set("q", input.q);
    if (input?.state) query.set("state", input.state);
    return get<TaskArchivePage>(`/task-archives?${query.toString()}`).then((response) => ({
      ...response,
      items: arr(response.items),
    }));
  },
  taskArchive: (id: number) => get<TaskArchive>(`/task-archives/${id}`),
  archiveTask: (id: string) => post<TaskArchive>(`/tasks/${id}/archive`),
  archiveTasks: (taskIds: string[]) =>
    post<{ items: ArchiveBatchItem[] }>("/tasks/archive/batch", { task_ids: taskIds }),
  restoreTaskArchive: (id: number) => post<TaskArchive>(`/task-archives/${id}/restore`),
  restoreTaskArchives: (archiveIds: number[]) =>
    post<{ items: ArchiveBatchItem[] }>("/task-archives/restore/batch", { archive_ids: archiveIds }),
  deleteTaskArchive: (id: number) => del<TaskArchive>(`/task-archives/${id}`),
  deleteTaskArchives: (archiveIds: number[]) =>
    post<{ items: ArchiveBatchItem[] }>("/task-archives/delete/batch", { archive_ids: archiveIds }),
  controlIntent: (
    taskId: string,
    intentId: string,
    action: "pause" | "resume" | "cancel",
    reason?: string,
    // cancel only: soft (default, soft delete, intent set to deleted + deletion reason recorded) | hard (real delete, cascades to exclusive descendants).
    mode?: "soft" | "hard",
  ) =>
    post<{
      id: number;
      state: "paused" | "open" | "deleted" | "";
      deleted?: { intents: number; facts: number; findings: number; activities: number };
    }>(`/tasks/${taskId}/intents/${intentId}/control`, { action, reason: reason ?? "", mode: mode ?? "soft" }),
  sendWorkerMessage: (taskId: string, intentId: string, message: string, requestId: string) =>
    post<{
      id: number;
      state: "running";
      accepted: true;
      request_id: string;
    }>(`/tasks/${taskId}/intents/${intentId}/messages`, { message, request_id: requestId }),
  taskLLMResolution: (id: string) => get<TaskLLMResolutions>(`/tasks/${id}/llm/resolution`),
  // Rerun an intent that did not succeed (blocked/exhausted/stopped): set back to open, and a worker re-claims and reruns it from scratch.
  rerunIntent: (taskId: string, intentId: string) =>
    post<{ id: string; reopened: number }>(`/tasks/${taskId}/intents/${intentId}/rerun`),
  // Batch-rerun every blocked intent of this task (one-click retry all when a single network/LLM outage blocked several).
  rerunBlocked: (taskId: string) => post<{ id: string; reopened: number }>(`/tasks/${taskId}/intents/rerun-blocked`),
  setActive: (id: string) => post<{ active: string }>("/active", { id }),
  // ---- stats ----
  stats: (task?: string) => get<Stats>(`/stats${tq(task)}`),
  // Asset test coverage (rough estimate, for reference): share of in-scope assets touched by facts + total/tested per type.
  taskCoverage: (id: string) => get<TaskCoverage>(`/tasks/${id}/coverage`),
  // Asset coverage graph: every in-scope asset + root-domain/company nodes used for connection, with tested/in_scope.
  taskCoverageGraph: (id: string) => get<CoverageGraphData>(`/tasks/${id}/coverage-graph`),
  // Global llm_usage aggregate (the dashboard's new token view): totals per profile + daily buckets.
  usageStats: (days = 365) => get<UsageStats>(`/tokens/usage?days=${days}`),
  // ---- Goal management (overview) ----
  // All goals of this task (text/vulnclass/state).
  taskGoals: (id: string) =>
    get<{ goals: TaskGoal[] | null }>(`/tasks/${id}/goals`).then((response) => ({ goals: arr(response.goals) })),
  // Manually add a goal: write it into the graph, notify the planner, revive the task.
  addGoal: (id: string, text: string, vulnclass?: string) =>
    post<TaskGoal>(`/tasks/${id}/goals`, { text, vulnclass: vulnclass ?? "" }),
  // Manually edit goal text (and vulnclass): notify the planner it changed "from old to new", revive the task.
  updateGoal: (id: string, goalId: string, text: string, vulnclass?: string) =>
    patch<TaskGoal>(`/tasks/${id}/goals/${goalId}`, { text, vulnclass: vulnclass ?? "" }),
  // Manually delete a goal (hard delete): notify the planner; deleting does not revive the task.
  deleteGoal: (id: string, goalId: string) => del<{ ok: boolean }>(`/tasks/${id}/goals/${goalId}`),

  // ---- Operation constraints management (overview) ----
  // All operation constraints of this task (allow/deny).
  taskConstraints: (id: string) =>
    get<{ constraints: TaskConstraint[] | null }>(`/tasks/${id}/constraints`).then((response) => ({
      constraints: arr(response.constraints),
    })),
  // Add a constraint (no planner notification; the next planning round reads it naturally).
  addConstraint: (id: string, text: string, kind: TaskConstraint["kind"]) =>
    post<TaskConstraint>(`/tasks/${id}/constraints`, { text, kind }),
  // Edit a constraint (text + allow/deny).
  updateConstraint: (id: string, constraintId: string, text: string, kind: TaskConstraint["kind"]) =>
    patch<TaskConstraint>(`/tasks/${id}/constraints/${constraintId}`, { text, kind }),
  // Delete a constraint.
  deleteConstraint: (id: string, constraintId: string) =>
    del<{ ok: boolean }>(`/tasks/${id}/constraints/${constraintId}`),

  // ---- Task-level asset interception/allow rules (overview) ----
  taskInterceptRules: (id: string) =>
    get<{ rules: AssetInterceptRule[] | null }>(`/tasks/${id}/intercept-rules`).then((r) => arr(r.rules)),
  createTaskInterceptRule: (id: string, rule: AssetInterceptRuleInput) =>
    post<AssetInterceptRule>(`/tasks/${id}/intercept-rules`, rule),
  updateTaskInterceptRule: (id: string, ruleId: number, rule: AssetInterceptRuleInput) =>
    put<AssetInterceptRule>(`/tasks/${id}/intercept-rules/${ruleId}`, rule),
  deleteTaskInterceptRule: (id: string, ruleId: number) =>
    del<{ deleted: boolean }>(`/tasks/${id}/intercept-rules/${ruleId}`),
  toggleTaskInterceptRule: (id: string, ruleId: number, enabled: boolean) =>
    post<{ ok: boolean; enabled: boolean }>(`/tasks/${id}/intercept-rules/${ruleId}/toggle`, { enabled }),

  // Test scope list of this task (including scopes inherited from source tasks).
  taskScope: (id: string) => get<{ scope: TaskScopeRow[] }>(`/tasks/${id}/scope`),
  // Manually add one test scope row (kind=company/root_domain/subdomain/ip/cidr).
  addTaskScope: (id: string, kind: string, value: string, reason?: string) =>
    post<TaskScopeRow>(`/tasks/${id}/scope`, { kind, value, reason }),
  // Delete one test scope row of this task.
  deleteTaskScope: (id: string, scopeId: number) => del<{ ok: boolean }>(`/tasks/${id}/scope/${scopeId}`),
  // Intents / facts / findings of an asset within this task (for the coverage graph node drawer).
  taskAssetRefs: (id: string, assetId: number) => get<CoverageAssetRefs>(`/tasks/${id}/asset-refs?asset_id=${assetId}`),

  // ---- workspace file manager (workDir) ----
  workspaceList: (path = "") => get<WorkspaceListing>(`/workspace/list?path=${encodeURIComponent(path)}`),
  workspaceRead: (path: string) => get<WorkspaceFile>(`/workspace/read?path=${encodeURIComponent(path)}`),
  workspaceWrite: (path: string, content: string) =>
    post<{ ok: boolean; path: string }>(`/workspace/write`, { path, content }),
  workspaceMkdir: (path: string) => post<{ ok: boolean; path: string }>(`/workspace/mkdir`, { path }),
  workspaceDelete: (path: string) => del<{ ok: boolean }>(`/workspace/delete?path=${encodeURIComponent(path)}`),
  workspaceUpload: async (dir: string, files: File[]) => {
    if (MOCK) return { uploaded: files.length };
    const fd = new FormData();
    for (const f of files) fd.append("file", f);
    const token = getToken();
    const r = await fetch(`/api/workspace/upload?path=${encodeURIComponent(dir)}`, {
      method: "POST",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: fd,
    });
    if (!r.ok) throw new Error(t("Upload failed: {error}", { error: r.status }));
    return r.json() as Promise<{ uploaded: number }>;
  },
  workspaceDownload: async (path: string) => {
    let blob: Blob;
    if (MOCK) {
      blob = new Blob([`(demo) Sample download content of ${path}.`], { type: "text/plain" });
    } else {
      const token = getToken();
      const r = await fetch(`/api/workspace/download?path=${encodeURIComponent(path)}`, {
        headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      });
      if (!r.ok) throw new Error(t("Download failed: {error}", { error: r.status }));
      blob = await r.blob();
    }
    const objUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = objUrl;
    a.download = path.split("/").pop() || "download";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(objUrl);
  },
};
