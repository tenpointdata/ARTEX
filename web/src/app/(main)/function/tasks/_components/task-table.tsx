"use client";

import { TablePagination } from "@/components/table-pagination";
import { Checkbox } from "@/components/ui/checkbox";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";

import { SortableTaskHead, TaskRow } from "./task-row";
import type { TaskActions } from "./use-task-actions";
import type { TaskListState } from "./use-task-list";

export function TaskTable({ list, actions }: { list: TaskListState; actions: TaskActions }) {
  const canOperate = useCan("operate"); // selection only feeds mutating bulk actions
  const {
    tasks,
    filtered,
    paginated,
    headerChecked,
    toggleSelectedPage,
    sortField,
    sortDirection,
    sortTasksBy,
    nowSec,
    selectedIds,
    toggleSelected,
    page,
    setPage,
    pageSize,
    setPageSize,
  } = list;
  const { deleteTask, controlTask, renameTask, toggleTaskPinned, queueTaskArchive } = actions;

  return (
    <>
      {tasks.length === 0 ? (
        <div className="text-muted-foreground mx-4 flex items-center justify-center rounded-lg border border-dashed py-20 text-sm lg:mx-6">
          {t('No tasks yet. Click "New task" at the top right to start.')}
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-muted-foreground mx-4 flex items-center justify-center rounded-lg border border-dashed py-20 text-sm lg:mx-6">
          {t("No matching tasks.")}
        </div>
      ) : (
        <Table className="**:data-[slot='table-cell']:px-4 **:data-[slot='table-head']:px-4">
          <TableHeader className="[&_tr]:border-t">
            <TableRow>
              <TableHead className="w-10">
                {canOperate && (
                  <Checkbox
                    checked={headerChecked}
                    onCheckedChange={(checked) => toggleSelectedPage(checked === true)}
                    aria-label={t("Select all tasks on this page")}
                  />
                )}
              </TableHead>
              <SortableTaskHead
                field="id"
                label="ID"
                activeField={sortField}
                direction={sortDirection}
                className="font-mono"
                onSort={sortTasksBy}
              />
              <TableHead>{t("Name")}</TableHead>
              <TableHead>{t("Description")}</TableHead>
              <TableHead>{t("Goal")}</TableHead>
              <SortableTaskHead
                field="status"
                label={t("Status")}
                activeField={sortField}
                direction={sortDirection}
                onSort={sortTasksBy}
              />
              <TableHead className="text-center">{t("Goal progress")}</TableHead>
              <TableHead className="text-center" title={t("Critical / High / Medium / Low")}>
                {t("Findings")} <span className="text-muted-foreground font-normal">{t("C/H/M/L")}</span>
              </TableHead>
              <TableHead className="text-center">{t("Running workers")}</TableHead>
              <SortableTaskHead
                field="created"
                label={t("Created at")}
                activeField={sortField}
                direction={sortDirection}
                align="right"
                onSort={sortTasksBy}
              />
              <SortableTaskHead
                field="duration"
                label={t("Duration")}
                activeField={sortField}
                direction={sortDirection}
                align="right"
                onSort={sortTasksBy}
              />
              <TableHead className="text-right">{t("Token")}</TableHead>
              <TableHead className="sticky right-0 z-10 bg-card text-right shadow-[-1px_0_0_0_hsl(var(--border))]">
                {t("Actions")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {paginated.map((task) => (
              <TaskRow
                key={task.id}
                task={task}
                // only running tasks consume nowSec; other rows get 0 — with unchanged props, memo blocks the per-second tick's
                // whole-table re-render and only the running rows advance their duration.
                nowSec={task.status === "running" ? nowSec : 0}
                onDelete={deleteTask}
                onControl={controlTask}
                onRename={renameTask}
                onTogglePinned={toggleTaskPinned}
                onArchive={queueTaskArchive}
                selected={selectedIds.has(task.id)}
                onSelectedChange={toggleSelected}
              />
            ))}
          </TableBody>
        </Table>
      )}
      <TablePagination
        page={page}
        pageSize={pageSize}
        total={filtered.length}
        onPageChange={setPage}
        onPageSizeChange={(nextPageSize) => {
          setPageSize(nextPageSize);
          setPage(1);
        }}
      />
    </>
  );
}
