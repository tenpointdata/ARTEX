import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { DeleteTaskOptions, Task } from "@/lib/types";

import {
  ARCHIVABLE_STATUSES,
  type DeleteCounts,
  deleteDetails,
  deleteSummary,
  taskControlAction,
  taskIsPinned,
} from "./task-helpers";
import type { TaskListState } from "./use-task-list";

export function useTaskActions(list: TaskListState, setActiveTab: (tab: string) => void) {
  const { tasks, selectedIds, setSelectedIds, lastRef, load, refreshCategoriesAndTasks } = list;
  const [batchControlling, setBatchControlling] = React.useState<"pause" | "resume" | null>(null);
  const [movingCategory, setMovingCategory] = React.useState(false);

  const deleteTask = React.useCallback(
    async (id: string, options: DeleteTaskOptions) => {
      try {
        const result = await api.deleteTask(id, options);
        if (result.cleanup_warning) {
          toast.warning(
            t("{summary}; some external data cleanup did not finish: {warning}", {
              summary: deleteSummary(result),
              warning: result.cleanup_warning,
            }),
          );
        } else {
          toast.success(deleteSummary(result));
        }
        load();
      } catch (e) {
        toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
        throw e;
      }
    },
    [load],
  );

  // controlTask is the inline pause/resume: bulk goes through controlTasksBatch, a single row uses the single-task endpoint, saving the
  // "check first, then click bulk" detour. Clearing lastRef lets the next poll be accepted even with an identical payload; otherwise the state
  // write-back would be swallowed by dedup and the button would look unresponsive.
  const controlTask = React.useCallback(
    async (id: string, action: "pause" | "resume") => {
      try {
        const result = await api.controlTask(id, action);
        toast.success(
          action === "pause"
            ? t("Task #{id} paused", { id })
            : result.queued
              ? t("Task #{id} resumed, queued", { id })
              : t("Task #{id} resumed", { id }),
        );
      } catch (e) {
        toast.error(
          action === "pause"
            ? t("Pause failed: {error}", { error: (e as Error).message })
            : t("Resume failed: {error}", { error: (e as Error).message }),
        );
      } finally {
        // Refresh regardless of outcome: failures usually mean the state changed, and re-fetching restores the right button shape.
        lastRef.current = "";
        load();
      }
    },
    [
      load, // Refresh regardless of outcome: failures usually mean the state changed, and re-fetching restores the right button shape.
      lastRef,
    ],
  );

  const renameTask = React.useCallback(
    async (task: Task, name: string) => {
      try {
        await api.renameTask(task.id, name);
        toast.success(t("Task #{id} renamed", { id: task.id }));
        lastRef.current = "";
        load();
      } catch (error) {
        toast.error(t("Rename failed: {error}", { error: (error as Error).message }));
        throw error;
      }
    },
    [load, lastRef],
  );

  const toggleTaskPinned = React.useCallback(
    async (task: Task) => {
      const pinned = taskIsPinned(task);
      try {
        await api.pinTask(task.id, !pinned);
        toast.success(pinned ? t("Task #{id} unpinned", { id: task.id }) : t("Task #{id} pinned", { id: task.id }));
        lastRef.current = "";
        load();
      } catch (error) {
        toast.error(
          pinned
            ? t("Unpin failed: {error}", { error: (error as Error).message })
            : t("Pin failed: {error}", { error: (error as Error).message }),
        );
        throw error;
      }
    },
    [load, lastRef],
  );

  const queueTaskArchive = React.useCallback(
    async (task: Task) => {
      try {
        await api.archiveTask(task.id);
        toast.success(t("Task #{id} queued for archiving", { id: task.id }));
        setActiveTab("archived");
        lastRef.current = "";
        load();
      } catch (error) {
        toast.error(t("Archive failed: {error}", { error: (error as Error).message }));
        throw error;
      }
    },
    [load, setActiveTab, lastRef],
  );

  // deleteTasks deletes the selected tasks one by one: the backend has no batch endpoint, and each delete cascades into cleaning assets/traffic/files,
  // so they run serially to avoid overwhelming the backend at once; successes leave the selection, failures stay for retry.
  const deleteTasks = React.useCallback(
    async (ids: string[], options: DeleteTaskOptions, onProgress: (done: number) => void) => {
      const total: DeleteCounts = {
        assets_deleted: 0,
        assets_detached: 0,
        traffic_deleted: 0,
        files_deleted: false,
        findings_deleted: 0,
        llm_records_deleted: 0,
      };
      const deleted: string[] = [];
      const failed: { id: string; message: string }[] = [];
      const warnings: string[] = [];

      for (const id of ids) {
        try {
          const r = await api.deleteTask(id, options);
          total.assets_deleted += r.assets_deleted;
          total.assets_detached += r.assets_detached;
          total.traffic_deleted += r.traffic_deleted;
          total.files_deleted = total.files_deleted || r.files_deleted;
          total.findings_deleted += r.findings_deleted;
          total.llm_records_deleted += r.llm_records_deleted;
          if (r.cleanup_warning) warnings.push(`#${id}: ${r.cleanup_warning}`);
          deleted.push(id);
        } catch (e) {
          failed.push({ id, message: (e as Error).message });
        }
        onProgress(deleted.length + failed.length);
      }

      if (deleted.length > 0) {
        setSelectedIds((prev) => {
          const next = new Set(prev);
          for (const id of deleted) next.delete(id);
          return next;
        });
        const details = deleteDetails(total);
        const summary =
          details.length > 0
            ? t("Deleted {count} tasks ({details})", { count: deleted.length, details: details.join(", ") })
            : t("Deleted {count} tasks", { count: deleted.length });
        if (warnings.length > 0) {
          toast.warning(
            t("{summary}; some external data cleanup did not finish: {warning}", {
              summary,
              warning: warnings.join("; "),
            }),
          );
        } else {
          toast.success(summary);
        }
      }
      if (failed.length > 0) {
        const head = failed
          .slice(0, 3)
          .map((f) => `#${f.id} (${f.message})`)
          .join("; ");
        toast.error(
          failed.length > 3
            ? t("{count} tasks failed to delete: {details} and more", { count: failed.length, details: head })
            : t("{count} tasks failed to delete: {details}", { count: failed.length, details: head }),
        );
      }
      load();
    },
    [load, setSelectedIds],
  );

  const selectedTasks = React.useMemo(() => tasks.filter((task) => selectedIds.has(task.id)), [tasks, selectedIds]);
  const pausableTaskIDs = React.useMemo(
    () => selectedTasks.filter((task) => taskControlAction(task.status) === "pause").map((task) => task.id),
    [selectedTasks],
  );
  const resumableTaskIDs = React.useMemo(
    () => selectedTasks.filter((task) => taskControlAction(task.status) === "resume").map((task) => task.id),
    [selectedTasks],
  );
  const archivableTaskIDs = React.useMemo(
    () => selectedTasks.filter((task) => ARCHIVABLE_STATUSES.has(task.status) && !task.queued).map((task) => task.id),
    [selectedTasks],
  );

  const archiveSelectedTasks = React.useCallback(async () => {
    if (archivableTaskIDs.length === 0) return;
    const result = await api.archiveTasks(archivableTaskIDs);
    const succeeded = result.items.filter((item) => item.ok);
    const failed = result.items.filter((item) => !item.ok);
    if (succeeded.length > 0) toast.success(t("Queued {count} tasks for archiving", { count: succeeded.length }));
    if (failed.length > 0) {
      toast.error(
        t("{count} tasks could not be archived: {details}", {
          count: failed.length,
          details: failed
            .slice(0, 3)
            .map((item) => `#${item.id} (${item.error || t("state changed")})`)
            .join("; "),
        }),
      );
    }
    setSelectedIds(new Set());
    setActiveTab("archived");
    lastRef.current = "";
    load();
  }, [archivableTaskIDs, load, setSelectedIds, setActiveTab, lastRef]);

  const controlSelectedTasks = React.useCallback(
    async (action: "pause" | "resume", ids: string[]) => {
      if (ids.length === 0 || batchControlling) return;
      if (ids.length > 100) {
        toast.error(t("You can control at most 100 tasks at once"));
        return;
      }
      setBatchControlling(action);
      try {
        const result = await api.controlTasksBatch(ids, action);
        const succeeded = result.items.filter((item) => item.ok);
        const failed = result.items.filter((item) => !item.ok);
        if (succeeded.length > 0) {
          toast.success(
            action === "pause"
              ? t("Paused {count} tasks", { count: succeeded.length })
              : succeeded.some((item) => item.queued)
                ? t("Resumed {count} tasks, some tasks were queued", { count: succeeded.length })
                : t("Resumed {count} tasks", { count: succeeded.length }),
          );
        }
        if (failed.length > 0) {
          const details = failed
            .slice(0, 3)
            .map((item) => `#${item.id} (${item.error || t("state changed")})`)
            .join("; ");
          toast.error(
            failed.length > 3
              ? t("{count} tasks failed: {details} and more", { count: failed.length, details })
              : t("{count} tasks failed: {details}", { count: failed.length, details }),
          );
        }
        lastRef.current = "";
        load();
      } catch (error) {
        toast.error(
          action === "pause"
            ? t("Bulk pause failed: {error}", { error: (error as Error).message })
            : t("Bulk resume failed: {error}", { error: (error as Error).message }),
        );
      } finally {
        setBatchControlling(null);
      }
    },
    [batchControlling, load, lastRef],
  );

  // The backend writes the whole batch's category changes in one transaction, so the only failures can be tasks deleted after being checked.
  const moveSelectedTasksCategory = React.useCallback(
    async (categoryID?: number) => {
      const ids = [...selectedIds];
      if (ids.length === 0 || movingCategory) return;
      if (ids.length > 100) {
        toast.error(t("You can change the category of at most 100 tasks at once"));
        return;
      }
      setMovingCategory(true);
      try {
        const result = await api.updateTasksCategory(ids, categoryID);
        const succeeded = result.items.filter((item) => item.ok);
        const failed = result.items.filter((item) => !item.ok);
        const target = result.category?.name ?? t("Uncategorized");
        if (succeeded.length > 0) {
          toast.success(t('Moved {count} tasks to "{target}"', { count: succeeded.length, target }));
          setSelectedIds(new Set());
        }
        if (failed.length > 0) {
          const details = failed
            .slice(0, 3)
            .map((item) => `#${item.id} (${item.error || t("task no longer exists")})`)
            .join("; ");
          toast.error(
            failed.length > 3
              ? t("{count} tasks could not be moved: {details} and more", { count: failed.length, details })
              : t("{count} tasks could not be moved: {details}", { count: failed.length, details }),
          );
        }
        refreshCategoriesAndTasks();
      } catch (error) {
        toast.error(t("Failed to change category: {error}", { error: (error as Error).message }));
      } finally {
        setMovingCategory(false);
      }
    },
    [movingCategory, refreshCategoriesAndTasks, selectedIds, setSelectedIds],
  );

  return {
    batchControlling,
    movingCategory,
    deleteTask,
    controlTask,
    renameTask,
    toggleTaskPinned,
    queueTaskArchive,
    deleteTasks,
    pausableTaskIDs,
    resumableTaskIDs,
    archivableTaskIDs,
    archiveSelectedTasks,
    controlSelectedTasks,
    moveSelectedTasksCategory,
  };
}

export type TaskActions = ReturnType<typeof useTaskActions>;
