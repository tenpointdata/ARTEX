"use client";

import { Loader2Icon } from "lucide-react";

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
import type { TamperHook, TamperHookVersion } from "@/lib/types";

export function DeleteHookDialog({
  target,
  deleting,
  onClose,
  onConfirm,
}: {
  target: TamperHook | null;
  deleting: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Delete tampering hook?")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('This permanently removes "{name}" and its stored version history.', {
              name: target?.name ?? "",
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
            disabled={deleting}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {deleting && <Loader2Icon className="size-3.5 animate-spin" />}
            {t("Delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function RollbackHookDialog({
  target,
  hookName,
  rollingBack,
  onClose,
  onConfirm,
}: {
  target: TamperHookVersion | null;
  hookName: string;
  rollingBack: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Rollback hook to v{version}?", { version: target?.version ?? 0 })}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('This restores the matchers and source from v{version} as a new version on "{name}".', {
              version: target?.version ?? 0,
              name: hookName,
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={rollingBack}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
            disabled={rollingBack}
          >
            {rollingBack && <Loader2Icon className="size-3.5 animate-spin" />}
            {t("Rollback")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
