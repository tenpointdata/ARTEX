import {
  classifyCompanyScopeLine,
  companyScopeRuleError,
  isCompanyScopeKind,
  normalizeCompanyScopeValue,
} from "@/lib/company-scope";
import type {
  Asset,
  Company,
  CompanyScopeRule,
  IntentAsset,
  TaskAssetMutation,
  TaskAssetScopeMutation,
  TaskScopeRow,
} from "@/lib/types";

import * as D from "../data";
import { deleteMockTaskAssetSources, mockAssetForTask, mockTaskAssetID, setMockTaskAssetSource } from "./archive";
import { mockAssetCounts, mockAssetMatchesDSL } from "./assets";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import { mockAssets, mockCompanies, mockFindings, mockState, mockTaskScopes, mockTasks } from "./state";
import { mockScopeRows } from "./task-helpers";

export function routeScopeAssets({ m, path, seg, q, b }: RouteCtx): unknown {
  const task = q.get("task") ?? undefined;
  // ── Coverage / coverage graph / asset links (per task) ──
  if (seg[0] === "tasks" && seg[2] === "coverage" && seg.length === 3) return D.coverage;
  if (seg[0] === "tasks" && seg[2] === "coverage-graph") return D.coverageGraph;
  if (seg[0] === "tasks" && seg[2] === "asset-refs") return D.assetRefsFor(Number(q.get("asset_id") ?? 0));

  // ── Task test scope (add/remove/query) ──
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 3 && m === "GET") {
    return { scope: mockTaskScopes.get(seg[1]) ?? [] };
  }
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 3 && m === "POST") {
    const current = mockTaskScopes.get(seg[1]) ?? [];
    const kind = String(b.kind ?? "") as TaskScopeRow["kind"];
    const value = String(b.value ?? "").trim();
    const row: TaskScopeRow = {
      id: Date.now(),
      task_id: mockTaskAssetID(seg[1]) ?? Number(seg[1]),
      kind,
      source: "manual",
    };
    if (kind === "root_domain" || kind === "subdomain") row.domain = value;
    else if (kind === "ip" || kind === "cidr") row.net = value;
    else row.value = value;
    mockTaskScopes.set(seg[1], [...current, row]);
    return row;
  }
  if (seg[0] === "tasks" && seg[2] === "scope" && seg.length === 4 && m === "DELETE") {
    const id = Number(seg[3]);
    mockTaskScopes.set(
      seg[1],
      (mockTaskScopes.get(seg[1]) ?? []).filter((row) => row.id !== id),
    );
    return { ok: true };
  }

  // ── Global llm_usage aggregation (new dashboard view, demo) ──
  if (path === "/tokens/usage")
    return {
      by_profile: [
        {
          profile_name: "default",
          calls: 60,
          tasks: 4,
          input_tokens: 1570000,
          output_tokens: 110000,
          cache_read_tokens: 1120000,
          cache_write_tokens: 140000,
        },
      ],
      daily: [
        {
          profile_name: "default",
          date: "2026-08-18",
          input_tokens: 520000,
          output_tokens: 38000,
          cache_read_tokens: 370000,
        },
        {
          profile_name: "default",
          date: "2026-08-19",
          input_tokens: 640000,
          output_tokens: 45000,
          cache_read_tokens: 460000,
        },
        {
          profile_name: "default",
          date: "2026-08-20",
          input_tokens: 410000,
          output_tokens: 27000,
          cache_read_tokens: 290000,
        },
      ],
    };

  // ── Token usage by model (demo: one sample) ──
  if (path === "/llm/records/by-model")
    return {
      models: [
        {
          model: "claude-opus-4-6",
          calls: 42,
          input_tokens: 1250000,
          output_tokens: 86000,
          cache_read_tokens: 940000,
          cache_write_tokens: 120000,
        },
        {
          model: "claude-haiku-4-5",
          calls: 18,
          input_tokens: 320000,
          output_tokens: 24000,
          cache_read_tokens: 180000,
          cache_write_tokens: 20000,
        },
      ],
    };

  // ── Workspace file manager (demo: static sample tree; writes/creates/deletes hit the {ok:true} write fallback below) ──
  if (path === "/workspace/list") return D.workspaceList(q.get("path") ?? "");
  if (path === "/workspace/read") return D.workspaceRead(q.get("path") ?? "");

  // ── stats ──
  if (path === "/stats") {
    return D.stats(task, { tasks: mockTasks, findings: mockFindings, activeTask: mockState.activeTask });
  }

  // ── assets ──
  if (path === "/assets/counts") return mockAssetCounts(q.get("task_id"));
  if (path === "/assets" && m === "GET") {
    const type = q.get("type") ?? "";
    const taskID = q.get("task_id");
    const numericTaskID = taskID ? mockTaskAssetID(taskID) : undefined;
    const dsl = q.get("dsl") ?? "";
    const list = mockAssets.filter((asset) => {
      if (type && asset.type !== type) return false;
      if (taskID && (numericTaskID === undefined || !asset.task_ids.includes(numericTaskID))) return false;
      return !dsl || mockAssetMatchesDSL(asset, dsl);
    });
    const limit = Number(q.get("limit") ?? 50);
    const offset = Number(q.get("offset") ?? 0);
    const page = list.slice(offset, offset + limit).map((asset) => (taskID ? mockAssetForTask(taskID, asset) : asset));
    return { count: page.length, total: list.length, assets: page };
  }
  if (path === "/assets" && m === "DELETE") {
    const ids = new Set(Array.isArray(b.ids) ? b.ids.map(Number) : []);
    let deleted = 0;
    for (let index = mockAssets.length - 1; index >= 0; index--) {
      if (!ids.has(mockAssets[index].id)) continue;
      mockAssets.splice(index, 1);
      deleted++;
    }
    return { deleted };
  }
  if (seg[0] === "tasks" && seg[2] === "assets" && seg.length === 3 && m === "POST") {
    const task = mockTasks.find((item) => item.id === seg[1]);
    const numericTaskID = mockTaskAssetID(seg[1]);
    if (!task || numericTaskID === undefined) throw new Error("task not found");
    if (Array.isArray(b.scope)) {
      if (b.scope.length === 0) throw new Error("fill in a valid test scope");
      const rules: CompanyScopeRule[] = b.scope.map((candidate, index) => {
        if (typeof candidate === "string") {
          const issue = classifyCompanyScopeLine(candidate, index + 1);
          if (!issue.rule || issue.error)
            throw new Error(`Scope entry ${index + 1} is invalid: ${issue.error ?? "unrecognized"}`);
          return issue.rule;
        }
        const item = candidate as { kind?: unknown; value?: unknown };
        const value = String(item?.value ?? "").trim();
        const rule =
          item?.kind && isCompanyScopeKind(item.kind)
            ? { kind: item.kind, value }
            : classifyCompanyScopeLine(value, index + 1).rule;
        const error = rule ? companyScopeRuleError(rule) : "unrecognized";
        if (!rule || error) throw new Error(`Scope entry ${index + 1} is invalid: ${error}`);
        return rule;
      });
      const mutation: TaskAssetScopeMutation = {
        requested: rules.length,
        assets_linked: 0,
        assets_existing: 0,
        scopes_added: 0,
        scopes_existing: 0,
      };
      const currentScopes = mockTaskScopes.get(seg[1]) ?? [];
      const scopeKeys = new Set(
        currentScopes.map((row) => `${row.kind}|${row.domain ?? row.net ?? row.value ?? row.company_id ?? ""}`),
      );
      for (const rule of rules) {
        const normalized = normalizeCompanyScopeValue(rule);
        const scope: TaskScopeRow = {
          id: Date.now() + currentScopes.length,
          task_id: numericTaskID,
          kind: rule.kind === "domain" ? "root_domain" : rule.kind,
          source: "manual",
          reason: "manually added by the user on the test assets page",
        };
        if (rule.kind === "domain") scope.domain = normalized;
        else if (rule.kind === "ip") scope.net = `${normalized}/${normalized.includes(":") ? 128 : 32}`;
        else if (rule.kind === "cidr") scope.net = normalized;
        else scope.value = normalized;
        const scopeKey = `${scope.kind}|${scope.domain ?? scope.net ?? scope.value ?? ""}`;
        if (scopeKeys.has(scopeKey)) mutation.scopes_existing++;
        else {
          scopeKeys.add(scopeKey);
          currentScopes.push(scope);
          mutation.scopes_added++;
        }

        if (rule.kind !== "domain" && rule.kind !== "ip") continue;
        const type = rule.kind === "domain" ? "root_domain" : "ip";
        let asset = mockAssets.find((item) =>
          type === "root_domain"
            ? item.type === type && item.domain === normalized
            : item.type === type && item.ip === normalized,
        );
        const alreadyLinked = asset?.task_ids.includes(numericTaskID) ?? false;
        if (!asset) {
          const nextID = mockAssets.reduce((max, item) => Math.max(max, item.id), 0) + 1;
          asset = {
            id: nextID,
            type,
            task_ids: [],
            ...(type === "root_domain" ? { domain: normalized, root_domain: normalized } : { ip: normalized }),
            last_seen: new Date().toISOString(),
          };
          mockAssets.push(asset);
        }
        if (alreadyLinked) mutation.assets_existing++;
        else {
          asset.task_ids.push(numericTaskID);
          mutation.assets_linked++;
        }
        setMockTaskAssetSource(seg[1], asset.id, {
          task_source: "manual",
          task_source_summary: "manually added by the user on the test assets page",
          task_source_node_id: undefined,
        });
      }
      mockTaskScopes.set(seg[1], currentScopes);
      return mutation;
    }
    const ids = [...new Set(Array.isArray(b.asset_ids) ? b.asset_ids.map(Number) : [])];
    const sourceSummary = String(b.source_summary ?? "").trim();
    if (ids.length === 0 || ids.length > 100 || !sourceSummary)
      throw new Error("select assets and fill in the source note");
    const requestedAssets = ids.map((id) => mockAssets.find((asset) => asset.id === id));
    if (requestedAssets.some((asset) => !asset)) throw new Error("asset not found");
    const mutation: TaskAssetMutation = { requested: ids.length, attached: 0, existing: 0 };
    for (const asset of requestedAssets) {
      if (!asset) continue;
      if (asset.task_ids.includes(numericTaskID)) mutation.existing++;
      else {
        asset.task_ids.push(numericTaskID);
        mutation.attached++;
      }
      setMockTaskAssetSource(seg[1], asset.id, {
        task_source: "manual",
        task_source_summary: sourceSummary,
        task_source_node_id: undefined,
      });
    }
    return mutation;
  }
  if (seg[0] === "tasks" && seg[2] === "assets" && seg.length === 4 && m === "DELETE") {
    const numericTaskID = mockTaskAssetID(seg[1]);
    const asset = mockAssets.find((item) => item.id === Number(seg[3]));
    if (numericTaskID === undefined || !asset) throw new Error("task or asset not found");
    if (!asset.task_ids.includes(numericTaskID)) throw new Error("asset is not linked to the current task");
    asset.task_ids = asset.task_ids.filter((id) => id !== numericTaskID);
    deleteMockTaskAssetSources(seg[1], asset.id);
    return { detached: asset.id };
  }
  if (seg[0] === "tasks" && seg[2] === "intent-assets" && seg.length === 3 && m === "GET") {
    let mappings: Array<{ intentID: string; assetID: number; summary: string }> = [];
    if (seg[1] === "t-acme-web") {
      mappings = [
        {
          intentID: "i3",
          assetID: 6,
          summary: "The admin feature enumeration intent was selected from the preceding subdomain finding",
        },
        { intentID: "i5", assetID: 3, summary: "The order API testing intent was selected from the API task goal" },
      ];
    } else if (seg[1] === "t-acme-api") {
      mappings = [
        { intentID: "i5", assetID: 3, summary: "The order API testing intent was selected from the API task goal" },
      ];
    }
    const sourceTaskID = mockTaskAssetID(seg[1]) ?? 0;
    const assets: IntentAsset[] = mappings.flatMap((mapping) => {
      const storedAsset = mockAssets.find((item) => item.id === mapping.assetID);
      if (!storedAsset) return [];
      const asset = mockAssetForTask(seg[1], storedAsset);
      return [
        {
          intent_id: mapping.intentID,
          asset_id: asset.id,
          type: asset.type,
          label: asset.domain ?? asset.ip ?? asset.app_name ?? asset.url ?? asset.service_name ?? `#${asset.id}`,
          source: asset.task_source ?? "agent",
          source_summary: asset.task_source_summary ?? mapping.summary,
          source_node_id: asset.task_source_node_id,
          source_task_id: sourceTaskID,
          inherited: false,
        },
      ];
    });
    return { assets };
  }

  // ── companies ──
  if (path === "/companies" && m === "GET") return structuredClone(mockCompanies);
  if (path === "/companies" && m === "POST") {
    const name = String(b.name ?? "").trim();
    if (!name) throw new Error("company name cannot be empty");
    if (mockCompanies.some((company) => company.name.toLowerCase() === name.toLowerCase())) {
      throw new Error("company already exists");
    }
    const id = mockCompanies.reduce((max, company) => Math.max(max, company.id), 0) + 1;
    const scopeResult = mockScopeRows(id, b.scope);
    const company: Company = { id, name, asset_count: 0, scope: scopeResult.rows };
    mockCompanies.push(company);
    return {
      id,
      created: true,
      scope_added: scopeResult.rows.length,
      scope_skipped: scopeResult.skipped,
      scope_invalid: scopeResult.invalid,
    };
  }
  if (seg[0] === "companies" && seg[2] === "scope" && m === "POST") {
    const company = mockCompanies.find((item) => item.id === Number(seg[1]));
    if (!company) throw new Error("company not found");
    const reset = b.reset === true;
    const scopeResult = mockScopeRows(company.id, b.scope, reset ? [] : (company.scope ?? []));
    if (reset && scopeResult.invalid > 0)
      throw new Error("company scope contains invalid rules; the original scope was not overwritten");
    company.scope = reset ? scopeResult.rows : [...(company.scope ?? []), ...scopeResult.rows];
    return { added: scopeResult.rows.length, skipped: scopeResult.skipped, invalid: scopeResult.invalid };
  }
  if (seg[0] === "companies" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    const index = mockCompanies.findIndex((item) => item.id === id);
    if (index < 0) throw new Error("company not found");
    mockCompanies.splice(index, 1);
    let assetsDeleted = 0;
    for (let assetIndex = mockAssets.length - 1; assetIndex >= 0; assetIndex--) {
      const asset: Asset = mockAssets[assetIndex];
      if (asset.company_id !== id) continue;
      if (b.delete_assets === true) {
        mockAssets.splice(assetIndex, 1);
        assetsDeleted++;
      } else {
        delete asset.company_id;
      }
    }
    return { deleted: 1, assets_deleted: assetsDeleted };
  }

  return NO_ROUTE;
}
