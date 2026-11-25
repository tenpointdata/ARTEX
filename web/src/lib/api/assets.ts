import type {
  Asset,
  Company,
  CompanyScopeMutation,
  CompanyScopeRule,
  IntentAsset,
  TaskAssetMutation,
  TaskAssetScopeMutation,
} from "@/lib/types";

import { arr, del, get, http, post } from "./core";

export const assetsApi = {
  // ---- assets ----
  // Server-side paginated: pass limit/offset, get back the page + full match total.
  assets: (type = "", limit = 50, offset = 0, companyId: number | null = null) =>
    get<{ count: number; total: number; assets: Asset[] }>(
      `/assets?type=${type}&limit=${limit}&offset=${offset}${companyId ? `&company_id=${companyId}` : ""}`,
    ).then((r) => ({ assets: r?.assets ?? [], total: r?.total ?? r?.count ?? 0 })),
  searchAssets: (dsl: string, type = "", limit = 50, offset = 0, companyId: number | null = null) =>
    get<{ count: number; total: number; assets: Asset[] }>(
      `/assets?dsl=${encodeURIComponent(dsl)}${type ? `&type=${encodeURIComponent(type)}` : ""}&limit=${limit}&offset=${offset}${companyId ? `&company_id=${companyId}` : ""}`,
    ).then((r) => ({ assets: r?.assets ?? [], total: r?.total ?? r?.count ?? 0 })),
  assetCounts: (taskId = "") =>
    get<Record<string, number>>(`/assets/counts${taskId ? `?task_id=${encodeURIComponent(taskId)}` : ""}`),
  deleteAssets: (ids: number[]) =>
    http<{ deleted: number }>("/assets", { method: "DELETE", body: JSON.stringify({ ids }) }),
  // task-scoped view of the same endpoint — server-side paginated like `assets`
  taskAssets: (taskId: string, type = "", limit = 50, offset = 0) =>
    get<{ count: number; total: number; assets: Asset[] }>(
      `/assets?task_id=${encodeURIComponent(taskId)}&type=${encodeURIComponent(type)}&limit=${limit}&offset=${offset}`,
    ).then((r) => ({ assets: r?.assets ?? [], total: r?.total ?? r?.count ?? 0 })),
  // DSL search scoped to a task — the backend forces the task_id filter, so it
  // always stays within that task's assets (same DSL grammar as `searchAssets`).
  searchTaskAssets: (taskId: string, dsl: string, type = "", limit = 50, offset = 0) =>
    get<{ count: number; total: number; assets: Asset[] }>(
      `/assets?task_id=${encodeURIComponent(taskId)}&dsl=${encodeURIComponent(dsl)}&type=${encodeURIComponent(type)}&limit=${limit}&offset=${offset}`,
    ).then((r) => ({ assets: r?.assets ?? [], total: r?.total ?? r?.count ?? 0 })),
  attachTaskAssets: (taskId: string, assetIds: number[], sourceSummary: string) =>
    post<TaskAssetMutation>(`/tasks/${taskId}/assets`, {
      asset_ids: assetIds,
      source_summary: sourceSummary,
    }),
  registerTaskAssetScopes: (taskId: string, scope: CompanyScopeRule[]) =>
    post<TaskAssetScopeMutation>(`/tasks/${taskId}/assets`, { scope }),
  detachTaskAsset: (taskId: string, assetId: number) => del<{ detached: number }>(`/tasks/${taskId}/assets/${assetId}`),
  taskIntentAssets: (taskId: string) =>
    get<{ assets: IntentAsset[] }>(`/tasks/${taskId}/intent-assets`).then((r) => arr(r.assets)),

  // ---- companies (company + asset scope; the single source of ownership) ----
  companies: () => get<Company[]>("/companies").then(arr),
  createCompany: (name: string, scope: CompanyScopeRule[]) =>
    post<{ id: number; created: boolean; scope_added?: number; scope_invalid?: number; scope_errors?: string[] }>(
      "/companies",
      {
        name,
        scope,
      },
    ),
  addCompanyScope: (id: number, scope: CompanyScopeRule[], reason = "") =>
    post<CompanyScopeMutation>(`/companies/${id}/scope`, {
      scope,
      reason,
    }),
  updateCompanyScope: (id: number, scope: CompanyScopeRule[], reason = "") =>
    post<CompanyScopeMutation>(`/companies/${id}/scope`, {
      scope,
      reason,
      reset: true,
    }),
  deleteCompany: (id: number, deleteAssets = false) =>
    http<{ deleted: number; assets_deleted: number }>(`/companies/${id}`, {
      method: "DELETE",
      body: JSON.stringify({ delete_assets: deleteAssets }),
    }),
};
