"use client";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { t } from "@/i18n";

import type { PendingDelete } from "./types";

export function DeleteConfirmDialog({
  pendingDelete,
  deleting,
  onCancel,
  onConfirm,
}: {
  pendingDelete: PendingDelete;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog
      open={!!pendingDelete}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {pendingDelete?.kind === "skill" && t('Delete skill "{name}"?', { name: pendingDelete.skill })}
            {pendingDelete?.kind === "dir" && t('Delete folder "{path}"?', { path: pendingDelete.path })}
            {pendingDelete?.kind === "file" && t('Delete file "{path}"?', { path: pendingDelete.path })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {pendingDelete?.kind === "skill"
              ? t(
                  "This deletes all of the skill's files, MCP links and visibility config. This action cannot be undone.",
                )
              : pendingDelete?.kind === "dir"
                ? t("This also deletes all files under the folder. This action cannot be undone.")
                : t("This action cannot be undone.")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            disabled={deleting}
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
          >
            {deleting ? t("Deleting…") : t("Delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
