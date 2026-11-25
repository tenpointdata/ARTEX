import type { Activity, Asset, FindingRetest, TaskScopeRow } from "@/lib/types";

import * as D from "../data";

export const delay = (ms = 120) => new Promise((r) => setTimeout(r, ms));

// Requests mutate a runtime copy, never the exported fixtures. This keeps module
// initialization deterministic for tests/HMR while preserving state across mock calls.
export const mockInterceptHistory = structuredClone(D.interceptHistory);
export const mockInterceptPending = structuredClone(D.interceptPending);
export const mockInterceptDetails = structuredClone(D.interceptDetails);
export const mockTasks = structuredClone(D.tasks);
export const mockFindings = structuredClone(D.findings);
export const mockLLMRecords = structuredClone(D.llmRecords);
export const mockTaskTemplates = structuredClone(D.taskTemplates);
export const mockTaskCategories = structuredClone(D.taskCategories);
export const mockConversations = structuredClone(D.conversations);
export const mockRetests: FindingRetest[] = [];
export const mockRetestMessages: Record<number, Activity[]> = {};
export const mockIntents = structuredClone(D.intents);
export const mockCompanies = structuredClone(D.companies);
export const mockAssets = structuredClone(D.assets);
export const mockActivity = structuredClone(D.activity);
export const mockTaskAssetIDs = new Map(D.tasks.map((task, index) => [task.id, index + 1]));
export const mockTaskScopes = new Map<string, TaskScopeRow[]>();
export type MockTaskAssetSource = Pick<Asset, "task_source" | "task_source_summary" | "task_source_node_id">;
export const mockTaskAssetSources = new Map<string, MockTaskAssetSource>();
export const mockState = {
  activeTask: D.ACTIVE_TASK,
  bindingSeq: 900,
  nextTaskAssetID: D.tasks.length + 1,
};
