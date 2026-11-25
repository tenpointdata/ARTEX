"use client";

import * as React from "react";

import { SearchIcon, Trash2Icon, Undo2Icon } from "lucide-react";
import { toast } from "sonner";

import { StatusBadge } from "@/components/status-badge";
import { TablePagination } from "@/components/table-pagination";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import { setVisibleInterval } from "@/lib/polling";
import type { TaskArchive, TaskArchiveState } from "@/lib/types";

const ARCHIVE_PROCESSING_STATES = new Set<TaskArchiveState>([
  "archive_queued",
  "archiving",
  "restore_queued",
  "restoring",
  "delete_queued",
  "deleting",
]);

const ARCHIVE_FAILED_STATES = new Set<TaskArchiveState>(["archive_failed", "restore_failed", "delete_failed"]);

function archivePhaseLabel(phase: string): string {
  if (phase === "upload_and_verify") return t("Uploading and verifying");
  if (phase === "download_and_verify") return t("Downloading and verifying");
  return t(phase);
}

function archiveStateLabel(state: TaskArchiveState): string {
  const labels: Record<TaskArchiveState, string> = {
    archive_queued: t("Archive queued"),
    archiving: t("Archiving"),
    archive_failed: t("Archive failed"),
    ready: t("Restorable"),
    restore_queued: t("Restore queued"),
    restoring: t("Restoring"),
    restore_failed: t("Restore failed"),
    delete_queued: t("Delete queued"),
    deleting: t("Deleting"),
    delete_failed: t("Delete failed"),
  };
  return labels[state];
}

function ArchiveStateBadge({ state }: { state: TaskArchiveState }) {
  let variant: "default" | "secondary" | "destructive" | "outline" = "outline";
  if (state === "ready") variant = "secondary";
  if (ARCHIVE_PROCESSING_STATES.has(state)) variant = "default";
  if (ARCHIVE_FAILED_STATES.has(state)) variant = "destructive";
  return <Badge variant={variant}>{archiveStateLabel(state)}</Badge>;
}

function formatArchiveBytes(bytes: number): string {
  if (bytes <= 0) return "—";
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

function archiveCompressionLabel(archive: TaskArchive): string {
  if (archive.original_size <= 0 || archive.compressed_size <= 0) return "—";
  const saved = Math.max(0, 100 - (archive.compressed_size / archive.original_size) * 100);
  return t("{size} · saves {percent}%", {
    size: formatArchiveBytes(archive.compressed_size),
    percent: saved.toFixed(0),
  });
}

function archiveDataTotal(archive: TaskArchive): number {
  return Object.values(archive.data_counts).reduce((sum, value) => sum + (Number(value) || 0), 0);
}

function archiveDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(intlLocale);
}

function ArchiveDeleteDialog({
  archives,
  onConfirm,
  trigger,
}: {
  archives: TaskArchive[];
  onConfirm: () => Promise<void>;
  trigger: React.ReactNode;
}) {
  const [open, setOpen] = React.useState(false);
  const [pending, setPending] = React.useState(false);
  const rows = archives.reduce((sum, archive) => sum + archiveDataTotal(archive), 0);
  const bytes = archives.reduce((sum, archive) => sum + archive.compressed_size, 0);
  return (
    <AlertDialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
      <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("Permanently delete {count} task archives?", { count: archives.length })}
          </AlertDialogTitle>
          <AlertDialogDescription className="[overflow-wrap:anywhere]">
            {t(
              "About {size} of archive packages and {rows} related data snapshots will be permanently deleted. This action cannot be undone.",
              { size: formatArchiveBytes(bytes), rows: rows.toLocaleString(intlLocale) },
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={async (event) => {
              event.preventDefault();
              setPending(true);
              try {
                await onConfirm();
                setOpen(false);
              } finally {
                setPending(false);
              }
            }}
          >
            {pending && <Spinner data-icon="inline-start" />}
            {t("Delete permanently")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// Archived tab. Restore, retry and permanent delete need operate; reviewers see the list read-only.
export function TaskArchivesPanel({ onChanged }: { onChanged: () => void }) {
  const canOperate = useCan("operate");
  const [archives, setArchives] = React.useState<TaskArchive[]>([]);
  const [query, setQuery] = React.useState("");
  const [stateFilter, setStateFilter] = React.useState("all");
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(20);
  const [total, setTotal] = React.useState(0);
  const [selected, setSelected] = React.useState<Set<number>>(() => new Set());
  const [loading, setLoading] = React.useState(true);
  const pendingRestoreIDs = React.useRef(new Set<number>());

  const load = React.useCallback(async () => {
    try {
      const result = await api.taskArchives({
        page,
        size: pageSize,
        q: query.trim(),
        state: stateFilter === "all" ? undefined : stateFilter,
      });
      setArchives(result.items);
      setTotal(result.total);
      setSelected((current) => {
        const visible = new Set(result.items.map((archive) => archive.id));
        return new Set([...current].filter((id) => visible.has(id)));
      });
      const pending = [...pendingRestoreIDs.current];
      if (pending.length > 0) {
        const states = await Promise.allSettled(pending.map((id) => api.taskArchive(id)));
        let restored = false;
        states.forEach((state, index) => {
          if (state.status !== "rejected" || !(state.reason instanceof Error)) return;
          if (!state.reason.message.includes("archive not found")) return;
          pendingRestoreIDs.current.delete(pending[index]);
          restored = true;
        });
        if (restored) onChanged();
      }
    } catch (error) {
      toast.error(t("Failed to load the archive list: {error}", { error: errorText(error) }));
    } finally {
      setLoading(false);
    }
  }, [onChanged, page, pageSize, query, stateFilter]);

  React.useEffect(() => {
    void load();
    const stopPolling = setVisibleInterval(() => void load(), 2_000);
    return stopPolling;
  }, [load]);

  const selectable = React.useMemo(
    () => archives.filter((archive) => !ARCHIVE_PROCESSING_STATES.has(archive.state)),
    [archives],
  );
  const selectedArchives = React.useMemo(
    () => archives.filter((archive) => selected.has(archive.id)),
    [archives, selected],
  );
  const restorable = selectedArchives.filter(
    (archive) => archive.state === "ready" || archive.state === "restore_failed",
  );
  const deletable = selectedArchives.filter(
    (archive) => archive.state === "ready" || archive.state === "delete_failed",
  );
  const selectedAll = selectable.length > 0 && selectable.every((archive) => selected.has(archive.id));
  const selectedSome = selectable.some((archive) => selected.has(archive.id));

  const afterAction = React.useCallback(() => {
    setSelected(new Set());
    void load();
    onChanged();
  }, [load, onChanged]);

  async function restoreMany(items: TaskArchive[]) {
    try {
      const result =
        items.length === 1
          ? {
              items: [
                {
                  id: String(items[0].id),
                  ok: true,
                  queued: true,
                  archive_id: (await api.restoreTaskArchive(items[0].id)).id,
                },
              ],
            }
          : await api.restoreTaskArchives(items.map((archive) => archive.id));
      const succeeded = result.items.filter((item) => item.ok).length;
      const failed = result.items.length - succeeded;
      for (const item of result.items) {
        const archiveID = Number(item.archive_id ?? item.id);
        if (item.ok && Number.isSafeInteger(archiveID) && archiveID > 0) pendingRestoreIDs.current.add(archiveID);
      }
      if (succeeded > 0) toast.success(t("Queued {count} tasks for restore", { count: succeeded }));
      if (failed > 0) toast.error(t("{count} tasks could not be restored", { count: failed }));
      afterAction();
    } catch (error) {
      toast.error(t("Restore failed: {error}", { error: errorText(error) }));
    }
  }

  async function deleteMany(items: TaskArchive[]) {
    try {
      const result =
        items.length === 1
          ? {
              items: [
                {
                  id: String(items[0].id),
                  ok: true,
                  queued: true,
                  archive_id: (await api.deleteTaskArchive(items[0].id)).id,
                },
              ],
            }
          : await api.deleteTaskArchives(items.map((archive) => archive.id));
      const succeeded = result.items.filter((item) => item.ok).length;
      const failed = result.items.length - succeeded;
      if (succeeded > 0) toast.success(t("Queued {count} archives for permanent deletion", { count: succeeded }));
      if (failed > 0) toast.error(t("{count} archives could not be deleted", { count: failed }));
      afterAction();
    } catch (error) {
      toast.error(t("Permanent delete failed: {error}", { error: errorText(error) }));
      throw error;
    }
  }

  async function retry(archive: TaskArchive) {
    try {
      if (archive.state === "archive_failed") await api.archiveTask(String(archive.task_id));
      if (archive.state === "restore_failed") await api.restoreTaskArchive(archive.id);
      if (archive.state === "delete_failed") await api.deleteTaskArchive(archive.id);
      toast.success(t("Re-queued for processing"));
      afterAction();
    } catch (error) {
      toast.error(t("Retry failed: {error}", { error: errorText(error) }));
    }
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 px-0 pt-6">
        <div className="flex flex-wrap items-center gap-2 px-4 lg:px-6">
          <div className="relative w-full sm:max-w-xs">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setPage(1);
              }}
              placeholder={t("Search task name / description / ID")}
              className="pl-8"
            />
          </div>
          <Select
            value={stateFilter}
            onValueChange={(value) => {
              setStateFilter(value);
              setPage(1);
            }}
          >
            <SelectTrigger className="w-36">
              <SelectValue placeholder={t("State")} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t("All statuses")}</SelectItem>
                <SelectItem value="archive_queued">{t("Archive queued")}</SelectItem>
                <SelectItem value="archiving">{t("Archiving")}</SelectItem>
                <SelectItem value="ready">{t("Restorable")}</SelectItem>
                <SelectItem value="restore_queued">{t("Restore queued")}</SelectItem>
                <SelectItem value="restoring">{t("Restoring")}</SelectItem>
                <SelectItem value="delete_queued">{t("Delete queued")}</SelectItem>
                <SelectItem value="deleting">{t("Deleting")}</SelectItem>
                <SelectItem value="archive_failed">{t("Archive failed")}</SelectItem>
                <SelectItem value="restore_failed">{t("Restore failed")}</SelectItem>
                <SelectItem value="delete_failed">{t("Delete failed")}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <span className="text-muted-foreground text-xs tabular-nums">{t("{count} archives", { count: total })}</span>
          {selectedArchives.length > 0 && (
            <>
              <span className="text-xs tabular-nums">{t("{count} selected", { count: selectedArchives.length })}</span>
              {restorable.length > 0 && (
                <Button size="sm" variant="outline" onClick={() => void restoreMany(restorable)}>
                  <Undo2Icon data-icon="inline-start" />
                  {t("Restore {count}", { count: restorable.length })}
                </Button>
              )}
              {deletable.length > 0 && (
                <ArchiveDeleteDialog
                  archives={deletable}
                  onConfirm={() => deleteMany(deletable)}
                  trigger={
                    <Button size="sm" variant="destructive">
                      <Trash2Icon data-icon="inline-start" />
                      {t("Permanently delete {count}", { count: deletable.length })}
                    </Button>
                  }
                />
              )}
            </>
          )}
        </div>
        {loading && (
          <div className="flex justify-center py-20">
            <Spinner />
          </div>
        )}
        {!loading && archives.length === 0 && (
          <Empty className="mx-4 border border-dashed lg:mx-6">
            <EmptyHeader>
              <EmptyTitle>{t("No task archives")}</EmptyTitle>
              <EmptyDescription>
                {t("Paused or terminal tasks can be archived from the current task list.")}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        {!loading && archives.length > 0 && (
          <Table className="**:data-[slot='table-cell']:px-4 **:data-[slot='table-head']:px-4">
            <TableHeader className="[&_tr]:border-t">
              <TableRow>
                <TableHead className="w-10">
                  {canOperate && (
                    <Checkbox
                      checked={selectedAll || (selectedSome ? "indeterminate" : false)}
                      onCheckedChange={(checked) =>
                        setSelected(checked === true ? new Set(selectable.map((archive) => archive.id)) : new Set())
                      }
                      aria-label={t("Select the archives on this page")}
                    />
                  )}
                </TableHead>
                <TableHead>{t("Task")}</TableHead>
                <TableHead>{t("Original status")}</TableHead>
                <TableHead>{t("Category")}</TableHead>
                <TableHead>{t("Archived at")}</TableHead>
                <TableHead>{t("Compressed size")}</TableHead>
                <TableHead>{t("Data records")}</TableHead>
                <TableHead className="min-w-44">{t("State")}</TableHead>
                <TableHead className="text-right">{t("Actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {archives.map((archive) => {
                const processing = ARCHIVE_PROCESSING_STATES.has(archive.state);
                const canRestore = archive.state === "ready" || archive.state === "restore_failed";
                const canDelete = archive.state === "ready" || archive.state === "delete_failed";
                return (
                  <TableRow key={archive.id} data-state={selected.has(archive.id) ? "selected" : undefined}>
                    <TableCell>
                      {canOperate && (
                        <Checkbox
                          checked={selected.has(archive.id)}
                          disabled={processing}
                          onCheckedChange={(checked) =>
                            setSelected((current) => {
                              const next = new Set(current);
                              if (checked === true) next.add(archive.id);
                              else next.delete(archive.id);
                              return next;
                            })
                          }
                          aria-label={t("Select task archive #{id}", { id: archive.task_id })}
                        />
                      )}
                    </TableCell>
                    <TableCell className="max-w-sm">
                      <div className="flex min-w-0 flex-col gap-0.5">
                        <span className="truncate font-medium">
                          {archive.task_name || archive.task_description || t("Task #{id}", { id: archive.task_id })}
                        </span>
                        <span className="truncate text-muted-foreground text-xs">
                          #{archive.task_id} · {archive.task_description}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge domain="task" value={archive.original_status} dot />
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {archive.category_name || t("Uncategorized")}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground text-xs">
                      {archiveDate(archive.archived_at || archive.requested_at)}
                    </TableCell>
                    <TableCell
                      className="whitespace-nowrap text-xs"
                      title={t("Before compression: {size}", { size: formatArchiveBytes(archive.original_size) })}
                    >
                      {archiveCompressionLabel(archive)}
                      {(archive.storage === "s3" || archive.storage === "local") && (
                        <div className="text-muted-foreground">
                          {archive.storage === "s3" ? t("Object storage") : t("Local storage")}
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-xs tabular-nums">
                      {archiveDataTotal(archive).toLocaleString(intlLocale)}
                    </TableCell>
                    <TableCell>
                      <div className="flex min-w-0 flex-col gap-1.5">
                        <ArchiveStateBadge state={archive.state} />
                        {processing && <Progress value={archive.progress} />}
                        {archive.error && (
                          <p className="text-destructive text-xs [overflow-wrap:anywhere]">{archive.error}</p>
                        )}
                        {[...new Set(archive.warnings ?? [])].map((warning) => (
                          <p
                            key={warning}
                            className="text-amber-700 text-xs [overflow-wrap:anywhere] dark:text-amber-400"
                          >
                            {warning}
                          </p>
                        ))}
                        <span className="text-muted-foreground text-xs">
                          {archivePhaseLabel(archive.phase)} · {archive.progress}%
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      {canOperate && (
                        <div className="flex justify-end gap-0.5">
                          {ARCHIVE_FAILED_STATES.has(archive.state) && (
                            <Button
                              size="icon"
                              variant="ghost"
                              onClick={() => void retry(archive)}
                              aria-label={t("Retry archive operation")}
                              title={t("Retry")}
                            >
                              <Undo2Icon />
                            </Button>
                          )}
                          {canRestore && (
                            <Button
                              size="icon"
                              variant="ghost"
                              onClick={() => void restoreMany([archive])}
                              aria-label={t("Restore task")}
                              title={t("Restore task")}
                            >
                              <Undo2Icon />
                            </Button>
                          )}
                          {canDelete && (
                            <ArchiveDeleteDialog
                              archives={[archive]}
                              onConfirm={() => deleteMany([archive])}
                              trigger={
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  aria-label={t("Permanently delete archive")}
                                  title={t("Permanently delete")}
                                >
                                  <Trash2Icon />
                                </Button>
                              }
                            />
                          )}
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
        <TablePagination
          page={page}
          pageSize={pageSize}
          total={total}
          onPageChange={setPage}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setPage(1);
          }}
        />
      </CardContent>
    </Card>
  );
}
