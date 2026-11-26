import type { Asset, Task, TaskArchive } from "@/lib/types";

import {
  type MockTaskAssetSource,
  mockAssets,
  mockFindings,
  mockLLMRecords,
  mockState,
  mockTaskAssetIDs,
  mockTaskAssetSources,
  mockTasks,
} from "./state";
import { sortMockTasks } from "./task-helpers";

export type MockTaskArchiveSnapshot = {
  task: Task;
  numericTaskID: number;
  assetIDs: number[];
  assetSources: Array<[number, MockTaskAssetSource]>;
};
export type MockTaskArchive = TaskArchive & { snapshot: MockTaskArchiveSnapshot };
export const mockTaskArchives: MockTaskArchive[] = [];
export let nextMockTaskArchiveID = 1;
export function mockTaskAssetSourceKey(taskID: string, assetID: number): string {
  return JSON.stringify([taskID, assetID]);
}

export function publicMockTaskArchive(archive: MockTaskArchive): TaskArchive {
  const { snapshot: _snapshot, ...item } = archive;
  return structuredClone(item);
}

export function mockArchiveTaskID(taskID: string): number {
  const numeric = Number(taskID);
  if (Number.isSafeInteger(numeric) && numeric > 0) return numeric;
  return mockTaskAssetID(taskID) ?? 0;
}

export function mockTaskArchiveBlocker(taskID: string): string | undefined {
  return mockTasks.find(
    (candidate) =>
      candidate.id !== taskID &&
      (candidate.source_task_ids ?? []).map(String).includes(taskID) &&
      !mockTaskArchives.some(
        (archive) =>
          archive.snapshot.task.id === candidate.id &&
          (archive.state === "archive_queued" || archive.state === "archiving"),
      ),
  )?.id;
}

export function publicMockTask(task: Task): Task {
  const item = structuredClone(task);
  const blocker = mockTaskArchiveBlocker(task.id);
  if (blocker) item.archive_blocked_by_task_id = blocker;
  else delete item.archive_blocked_by_task_id;
  return item;
}

export function mockArchiveTask(taskID: string): MockTaskArchive {
  const task = mockTasks.find((item) => item.id === taskID);
  if (!task) throw new Error("task not found");
  if (!["paused", "done", "failed", "timeout"].includes(task.status) && !task.paused) {
    throw new Error(task.queued ? "A queued task must be paused first" : "A running task must be paused first");
  }
  const existing = mockTaskArchives.find((item) => item.task_id === mockArchiveTaskID(taskID));
  if (existing) throw new Error("task is already in the archive queue");
  const dependent = mockTasks.find(
    (candidate) =>
      candidate.id !== taskID &&
      (candidate.source_task_ids ?? []).map(String).includes(taskID) &&
      !mockTaskArchives.some(
        (archive) =>
          archive.snapshot.task.id === candidate.id &&
          (archive.state === "archive_queued" || archive.state === "archiving"),
      ),
  );
  if (dependent)
    throw new Error(`The task is directly inherited by unarchived task #${dependent.id} and cannot be archived yet`);

  const numericTaskID = mockArchiveTaskID(taskID);
  const assetIDs = mockAssets.filter((asset) => asset.task_ids.includes(numericTaskID)).map((asset) => asset.id);
  const assetSources: Array<[number, MockTaskAssetSource]> = [];
  for (const assetID of assetIDs) {
    const source = mockTaskAssetSources.get(mockTaskAssetSourceKey(taskID, assetID));
    if (source) assetSources.push([assetID, structuredClone(source)]);
  }
  const findings = mockFindings.filter((finding) => finding.task_id === taskID);
  const llmRecords = mockLLMRecords.filter((record) => record.task_id === taskID);
  const tokens = task.tokens ?? { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0 };
  const now = new Date().toISOString();
  const archive: MockTaskArchive = {
    id: nextMockTaskArchiveID++,
    task_id: numericTaskID,
    state: "archive_queued",
    phase: "Waiting to archive",
    progress: 0,
    format_version: 1,
    original_size: 0,
    compressed_size: 0,
    task_name: task.name ?? "",
    task_description: task.description,
    task_goal: task.goal,
    original_status: task.status,
    category_id: task.category_id,
    category_name: task.category_name,
    source_task_ids: (task.source_task_ids ?? []).map(mockArchiveTaskID).filter((id) => id > 0),
    remaining_timeout_seconds: 0,
    data_counts: {
      assets: assetIDs.length,
      findings: findings.length,
      llm_records: llmRecords.length,
    },
    aggregate_stats: {
      tokens: {
        calls: llmRecords.length,
        input_tokens: tokens.input_tokens,
        output_tokens: tokens.output_tokens,
        cache_read_tokens: tokens.cache_read_tokens,
        cache_write_tokens: tokens.cache_write_tokens,
      },
      findings: findings.reduce<Record<string, number>>((counts, finding) => {
        counts[finding.severity] = (counts[finding.severity] ?? 0) + 1;
        return counts;
      }, {}),
    },
    requested_at: now,
    created_at: now,
    updated_at: now,
    snapshot: { task: structuredClone(task), numericTaskID, assetIDs, assetSources },
  };
  mockTaskArchives.unshift(archive);
  setTimeout(() => {
    if (archive.state !== "archive_queued") return;
    archive.state = "archiving";
    archive.phase = "Compressing task data";
    archive.progress = 55;
    archive.updated_at = new Date().toISOString();
  }, 100);
  setTimeout(() => {
    if (archive.state !== "archiving" && archive.state !== "archive_queued") return;
    archive.state = "ready";
    archive.phase = "Archive complete";
    archive.progress = 100;
    archive.archived_at = new Date().toISOString();
    archive.updated_at = archive.archived_at;
    archive.original_size = Math.max(4096, JSON.stringify(archive.snapshot).length * 4);
    archive.compressed_size = Math.max(1024, Math.round(archive.original_size * 0.32));
    const index = mockTasks.findIndex((item) => item.id === taskID);
    if (index >= 0) mockTasks.splice(index, 1);
    for (const asset of mockAssets) asset.task_ids = asset.task_ids.filter((id) => id !== numericTaskID);
    deleteMockTaskAssetSources(taskID);
    if (mockState.activeTask === taskID) {
      mockState.activeTask = mockTasks[0]?.id ?? "";
      for (const item of mockTasks) item.active = item.id === mockState.activeTask;
    }
  }, 500);
  return archive;
}

export function mockRestoreArchive(archive: MockTaskArchive): void {
  if (archive.state !== "ready" && archive.state !== "restore_failed")
    throw new Error("the archive's current state cannot be restored");
  archive.state = "restore_queued";
  archive.phase = "Waiting to restore";
  archive.progress = 0;
  archive.error = undefined;
  archive.updated_at = new Date().toISOString();
  setTimeout(() => {
    if (archive.state !== "restore_queued") return;
    archive.state = "restoring";
    archive.phase = "Restoring task data";
    archive.progress = 60;
    archive.updated_at = new Date().toISOString();
  }, 100);
  setTimeout(() => {
    if (archive.state !== "restoring" && archive.state !== "restore_queued") return;
    const restored = structuredClone(archive.snapshot.task);
    restored.active = false;
    restored.queued = false;
    if (!mockTasks.some((task) => task.id === restored.id)) mockTasks.push(restored);
    mockTaskAssetIDs.set(restored.id, archive.snapshot.numericTaskID);
    for (const assetID of archive.snapshot.assetIDs) {
      const asset = mockAssets.find((candidate) => candidate.id === assetID);
      if (asset && !asset.task_ids.includes(archive.snapshot.numericTaskID)) {
        asset.task_ids.push(archive.snapshot.numericTaskID);
      }
    }
    for (const [assetID, source] of archive.snapshot.assetSources) setMockTaskAssetSource(restored.id, assetID, source);
    const index = mockTaskArchives.indexOf(archive);
    if (index >= 0) mockTaskArchives.splice(index, 1);
    sortMockTasks();
  }, 500);
}

export function mockDeleteArchive(archive: MockTaskArchive): void {
  if (archive.state !== "ready" && archive.state !== "delete_failed")
    throw new Error("the archive's current state does not allow permanent deletion");
  const dependent = mockTaskArchives.find(
    (candidate) => candidate.id !== archive.id && candidate.source_task_ids.includes(archive.task_id),
  );
  if (dependent)
    throw new Error(`The archive is still depended on by task #${dependent.task_id} and cannot be permanently deleted`);
  archive.state = "delete_queued";
  archive.phase = "Waiting for permanent deletion";
  archive.progress = 0;
  archive.error = undefined;
  archive.updated_at = new Date().toISOString();
  setTimeout(() => {
    if (archive.state !== "delete_queued") return;
    archive.state = "deleting";
    archive.phase = "Deleting archive package";
    archive.progress = 70;
  }, 100);
  setTimeout(() => {
    const index = mockTaskArchives.indexOf(archive);
    if (index >= 0) mockTaskArchives.splice(index, 1);
  }, 450);
}

export function setMockTaskAssetSource(taskID: string, assetID: number, source: MockTaskAssetSource) {
  mockTaskAssetSources.set(mockTaskAssetSourceKey(taskID, assetID), source);
}

export function mockAssetForTask(taskID: string, asset: Asset): Asset {
  const source = mockTaskAssetSources.get(mockTaskAssetSourceKey(taskID, asset.id));
  return source ? { ...asset, ...source } : asset;
}

export function deleteMockTaskAssetSources(taskID: string, assetID?: number) {
  if (assetID !== undefined) {
    mockTaskAssetSources.delete(mockTaskAssetSourceKey(taskID, assetID));
    return;
  }
  for (const key of mockTaskAssetSources.keys()) {
    const [linkedTaskID] = JSON.parse(key) as [string, number];
    if (linkedTaskID === taskID) mockTaskAssetSources.delete(key);
  }
}

export function mockTaskAssetID(taskID: string): number | undefined {
  const numeric = Number(taskID);
  if (Number.isInteger(numeric) && numeric > 0) return numeric;
  return mockTaskAssetIDs.get(taskID);
}
