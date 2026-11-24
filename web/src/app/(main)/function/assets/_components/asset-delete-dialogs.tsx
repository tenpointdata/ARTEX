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
import { Checkbox } from "@/components/ui/checkbox";
import { t } from "@/i18n";
import type { Company } from "@/lib/types";

const DESTRUCTIVE = "bg-destructive text-destructive-foreground hover:bg-destructive/90";

export function DeleteAssetsDialog({
  open,
  count,
  busy,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  count: number;
  busy: boolean;
  onOpenChange: (o: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Confirm delete")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("This will permanently delete {count} asset records. This action cannot be undone.", { count })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
            disabled={busy}
            className={DESTRUCTIVE}
          >
            {busy ? t("Deleting…") : t("Confirm delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DeleteCompanyDialog({
  target,
  withAssets,
  busy,
  onWithAssets,
  onClose,
  onConfirm,
}: {
  target: Company | null;
  withAssets: boolean;
  busy: boolean;
  onWithAssets: (v: boolean) => void;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={!!target} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Delete company · {name}", { name: target?.name ?? "" })}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-3">
              <p>
                {t(
                  "This will permanently delete the company and its asset scope configuration. This action cannot be undone.",
                )}
              </p>
              <label
                htmlFor="delete-assets-opt"
                className="flex cursor-pointer items-center gap-2.5 rounded-md border p-3 hover:bg-muted/50"
              >
                <Checkbox id="delete-assets-opt" checked={withAssets} onCheckedChange={(v) => onWithAssets(!!v)} />
                <span className="text-sm leading-snug">
                  {t("Also delete all assets under this company")}
                  <span className="block text-xs text-muted-foreground">
                    {t("Leave unchecked to keep the assets and only remove the ownership")}
                  </span>
                </span>
              </label>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              onConfirm();
            }}
            disabled={busy}
            className={DESTRUCTIVE}
          >
            {busy ? t("Deleting…") : t("Confirm delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
