"use client";

import * as React from "react";

import { ArchiveIcon } from "lucide-react";

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
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { t } from "@/i18n";
import type { Task } from "@/lib/types";

import { ARCHIVABLE_STATUSES } from "./task-helpers";

export function archiveBlockReason(task: Task): string {
  if (task.queued) return t("Queued tasks must be paused first");
  if (!ARCHIVABLE_STATUSES.has(task.status)) return t("Running or unfinished tasks must be paused first");
  if (task.archive_blocked_by_task_id) {
    return t("The task is directly inherited by unarchived task #{id}; archive the dependent task first", {
      id: task.archive_blocked_by_task_id,
    });
  }
  return "";
}

export function ArchiveConfirmDialog({
  count,
  trigger,
  onConfirm,
}: {
  count: number;
  trigger: React.ReactNode;
  onConfirm: () => Promise<void>;
}) {
  const [open, setOpen] = React.useState(false);
  const [pending, setPending] = React.useState(false);
  return (
    <AlertDialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
      <AlertDialogTrigger asChild>{trigger}</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{count === 1 ? t("Archive task") : t("Archive {count} tasks", { count })}</AlertDialogTitle>
          <AlertDialogDescription className="[overflow-wrap:anywhere]">
            {t(
              'Archiving stops task scheduling and compresses the graph, LLM history, files, and exclusive assets and traffic into the configured archive storage. Once archived, a task can be restored from "Archived". Restoring requires free space on this server.',
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
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
            {t("Confirm archive")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function TaskArchiveAction({ task, onArchive }: { task: Task; onArchive: (task: Task) => Promise<void> }) {
  const reason = archiveBlockReason(task);
  const trigger = (
    <Button
      size="icon"
      variant="ghost"
      disabled={Boolean(reason)}
      aria-label={t("Archive task #{id}", { id: task.id })}
    >
      <ArchiveIcon />
    </Button>
  );
  if (reason) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">{trigger}</span>
        </TooltipTrigger>
        <TooltipContent>{reason}</TooltipContent>
      </Tooltip>
    );
  }
  return <ArchiveConfirmDialog count={1} trigger={trigger} onConfirm={() => onArchive(task)} />;
}
