import type { ArchiveBatchItem } from "@/lib/types";

import * as D from "../data";
import {
  mockArchiveTask,
  mockDeleteArchive,
  mockRestoreArchive,
  mockTaskArchives,
  publicMockTaskArchive,
} from "./archive";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import { mockTasks } from "./state";
import { bodyIDs } from "./task-helpers";

export function routeAuthArchives({ m, path, seg, q, b }: RouteCtx): unknown {
  const _task = q.get("task") ?? undefined;
  if (path === "/chat/mentions" && m === "GET") {
    const kind = q.get("kind") ?? "";
    const query = (q.get("q") ?? "").trim().toLowerCase();
    const candidates = [
      ...D.findings.map((finding, index) => ({
        kind: "finding",
        id: index + 1,
        label: finding.name || finding.vulnclass,
        description: `${finding.severity} · ${finding.summary}`,
      })),
      ...D.companies.map((company) => ({
        kind: "company",
        id: company.id,
        label: company.name,
        description: "company",
      })),
      ...D.assets.map((asset) => ({
        kind: asset.type,
        id: asset.id,
        label:
          asset.type === "endpoint"
            ? `${asset.method || "GET"} ${asset.url}`
            : asset.app_name || asset.url || asset.domain || asset.ip || asset.bundle_id || `Asset #${asset.id}`,
        description: [asset.type, asset.page_title, asset.service_name, asset.bundle_id, asset.ip]
          .filter(Boolean)
          .join(" · "),
      })),
    ];
    const filtered = candidates
      .filter(
        (item) =>
          (!kind || kind === item.kind || (kind === "asset" && item.kind !== "finding" && item.kind !== "company")) &&
          (!query || String(item.id) === query || `${item.label} ${item.description}`.toLowerCase().includes(query)),
      )
      .sort(
        (a, b) =>
          Number(String(b.id) === query) - Number(String(a.id) === query) ||
          b.id - a.id ||
          a.kind.localeCompare(b.kind),
      );
    const offset = Math.max(0, Number(q.get("cursor")) || 0);
    return {
      items: filtered.slice(offset, offset + 20),
      next_cursor: offset + 20 < filtered.length ? String(offset + 20) : undefined,
    };
  }

  // ── auth: let the demo go straight to the main UI ──
  if (path === "/auth/status") return { initialized: true };
  if (path === "/auth/login" || path === "/auth/init") return { token: "mock-demo" };
  if (path === "/auth/change-password") return { ok: true };

  // ── task cold archives ──
  if (path === "/task-archives" && m === "GET") {
    const page = Math.max(1, Number(q.get("page")) || 1);
    const size = Math.min(100, Math.max(1, Number(q.get("size")) || 20));
    const query = (q.get("q") ?? "").trim().toLowerCase();
    const state = (q.get("state") ?? "").trim();
    const filtered = mockTaskArchives.filter((archive) => {
      if (state && archive.state !== state) return false;
      if (!query) return true;
      return [archive.task_id, archive.task_name, archive.task_description].some((value) =>
        String(value ?? "")
          .toLowerCase()
          .includes(query),
      );
    });
    const offset = (page - 1) * size;
    return {
      items: filtered.slice(offset, offset + size).map(publicMockTaskArchive),
      total: filtered.length,
      page,
      size,
    };
  }
  if (seg[0] === "task-archives" && seg.length === 2 && m === "GET") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("archive not found");
    return publicMockTaskArchive(archive);
  }
  if (seg[0] === "task-archives" && seg[2] === "restore" && seg.length === 3 && m === "POST") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("archive not found");
    mockRestoreArchive(archive);
    return publicMockTaskArchive(archive);
  }
  if (seg[0] === "task-archives" && seg.length === 2 && m === "DELETE") {
    const archive = mockTaskArchives.find((item) => item.id === Number(seg[1]));
    if (!archive) throw new Error("archive not found");
    mockDeleteArchive(archive);
    return publicMockTaskArchive(archive);
  }
  if (path === "/task-archives/restore/batch" && m === "POST") {
    const ids = bodyIDs(b.archive_ids).map(Number);
    const items = ids.map<ArchiveBatchItem>((id) => {
      const archive = mockTaskArchives.find((item) => item.id === id);
      if (!archive) return { id: String(id), archive_id: id, ok: false, queued: false, error: "archive not found" };
      try {
        mockRestoreArchive(archive);
        return { id: String(id), archive_id: id, ok: true, queued: true };
      } catch (error) {
        return { id: String(id), archive_id: id, ok: false, queued: false, error: (error as Error).message };
      }
    });
    return { items };
  }
  if (path === "/task-archives/delete/batch" && m === "POST") {
    const ids = bodyIDs(b.archive_ids).map(Number);
    const items = ids.map<ArchiveBatchItem>((id) => {
      const archive = mockTaskArchives.find((item) => item.id === id);
      if (!archive) return { id: String(id), archive_id: id, ok: false, queued: false, error: "archive not found" };
      try {
        mockDeleteArchive(archive);
        return { id: String(id), archive_id: id, ok: true, queued: true };
      } catch (error) {
        return { id: String(id), archive_id: id, ok: false, queued: false, error: (error as Error).message };
      }
    });
    return { items };
  }
  if (path === "/tasks/archive/batch" && m === "POST") {
    const requested = bodyIDs(b.task_ids);
    const selected = new Set(requested);
    const ordered: string[] = [];
    const visiting = new Set<string>();
    const visited = new Set<string>();
    const visit = (id: string) => {
      if (visited.has(id) || visiting.has(id)) return;
      visiting.add(id);
      for (const candidate of mockTasks) {
        if (!selected.has(candidate.id) || !(candidate.source_task_ids ?? []).map(String).includes(id)) continue;
        visit(candidate.id);
      }
      visiting.delete(id);
      visited.add(id);
      ordered.push(id);
    };
    for (const id of requested) visit(id);
    const byID = new Map<string, ArchiveBatchItem>();
    for (const id of ordered) {
      try {
        const archive = mockArchiveTask(id);
        byID.set(id, { id, archive_id: archive.id, ok: true, queued: true });
      } catch (error) {
        byID.set(id, { id, ok: false, queued: false, error: (error as Error).message });
      }
    }
    return { items: requested.map((id) => byID.get(id) ?? { id, ok: false, queued: false, error: "task not found" }) };
  }
  if (seg[0] === "tasks" && seg[2] === "archive" && seg.length === 3 && m === "POST") {
    return publicMockTaskArchive(mockArchiveTask(seg[1]));
  }

  return NO_ROUTE;
}
