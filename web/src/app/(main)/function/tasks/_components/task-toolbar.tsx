"use client";

import { ArchiveIcon, PauseIcon, PlayIcon, SearchIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { t } from "@/i18n";
import type { TaskStatus } from "@/lib/types";

import { CategoryManagementSheet } from "./category-management-sheet";
import { CreateTaskSheet } from "./create-task-sheet";
import { BulkDeleteTasksDialog, MoveTasksCategoryDialog } from "./delete-dialogs";
import { ArchiveConfirmDialog } from "./task-archive-action";
import { STATUS_OPTIONS, UNCATEGORIZED_VALUE } from "./task-helpers";
import { ConcurrencySettingsDialog } from "./task-row";
import type { TaskActions } from "./use-task-actions";
import type { TaskListState } from "./use-task-list";

export function TaskToolbar({ list, actions }: { list: TaskListState; actions: TaskActions }) {
  const {
    tasks,
    categories,
    query,
    setQuery,
    statusFilter,
    setStatusFilter,
    categoryFilter,
    setCategoryFilter,
    filtered,
    selectedIds,
    setSelectedIds,
    refreshCategoriesAndTasks,
    applyTaskCategoryMove,
  } = list;
  const {
    batchControlling,
    movingCategory,
    deleteTasks,
    pausableTaskIDs,
    resumableTaskIDs,
    archivableTaskIDs,
    archiveSelectedTasks,
    controlSelectedTasks,
    moveSelectedTasksCategory,
  } = actions;

  return (
    <div className="flex flex-wrap items-center gap-2 px-4 lg:px-6">
      <div className="relative w-full sm:max-w-xs">
        <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input
          placeholder={t("Search description / goal / ID")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="pl-8"
        />
        {query && (
          <button
            type="button"
            onClick={() => setQuery("")}
            aria-label={t("Clear search")}
            className="text-muted-foreground hover:text-foreground absolute top-1/2 right-2 -translate-y-1/2"
          >
            <XIcon className="size-4" />
          </button>
        )}
      </div>
      <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as TaskStatus | "all")}>
        <SelectTrigger className="w-36">
          <SelectValue placeholder={t("Status")} />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="all">{t("All statuses")}</SelectItem>
            {STATUS_OPTIONS.map((s) => (
              <SelectItem key={s.value} value={s.value}>
                {s.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Select value={categoryFilter} onValueChange={setCategoryFilter}>
        <SelectTrigger className="w-40">
          <SelectValue placeholder={t("Task category")} />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="all">{t("All categories")}</SelectItem>
            <SelectItem value={UNCATEGORIZED_VALUE}>{t("Uncategorized")}</SelectItem>
            {categories.map((category) => (
              <SelectItem key={category.id} value={String(category.id)}>
                {category.name}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <span className="text-muted-foreground text-xs tabular-nums">
        {t("{shown}/{total} tasks", { shown: filtered.length, total: tasks.length })}
      </span>
      {selectedIds.size > 0 && (
        <>
          <span className="text-xs tabular-nums">{t("{count} selected", { count: selectedIds.size })}</span>
          <Button size="sm" variant="ghost" onClick={() => setSelectedIds(new Set())}>
            {t("Clear selection")}
          </Button>
          {pausableTaskIDs.length > 0 && (
            <Button
              size="sm"
              variant="outline"
              disabled={batchControlling !== null}
              onClick={() => void controlSelectedTasks("pause", pausableTaskIDs)}
            >
              {batchControlling === "pause" ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <PauseIcon data-icon="inline-start" />
              )}
              {t("Pause {count}", { count: pausableTaskIDs.length })}
            </Button>
          )}
          {resumableTaskIDs.length > 0 && (
            <Button
              size="sm"
              variant="outline"
              disabled={batchControlling !== null}
              onClick={() => void controlSelectedTasks("resume", resumableTaskIDs)}
            >
              {batchControlling === "resume" ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <PlayIcon data-icon="inline-start" />
              )}
              {t("Resume {count}", { count: resumableTaskIDs.length })}
            </Button>
          )}
          {archivableTaskIDs.length > 0 && (
            <ArchiveConfirmDialog
              count={archivableTaskIDs.length}
              onConfirm={archiveSelectedTasks}
              trigger={
                <Button size="sm" variant="outline">
                  <ArchiveIcon data-icon="inline-start" />
                  {t("Archive {count}", { count: archivableTaskIDs.length })}
                </Button>
              }
            />
          )}
          <MoveTasksCategoryDialog
            categories={categories}
            count={selectedIds.size}
            moving={movingCategory}
            onMove={moveSelectedTasksCategory}
          />
          <BulkDeleteTasksDialog ids={[...selectedIds]} onDelete={deleteTasks} />
        </>
      )}
      <ConcurrencySettingsDialog />
      <CategoryManagementSheet
        categories={categories}
        tasks={tasks}
        onChanged={refreshCategoriesAndTasks}
        onTaskMoved={applyTaskCategoryMove}
      />
      <CreateTaskSheet
        tasks={tasks}
        categories={categories}
        onCreated={refreshCategoriesAndTasks}
        onCategoriesChanged={refreshCategoriesAndTasks}
      />
    </div>
  );
}
