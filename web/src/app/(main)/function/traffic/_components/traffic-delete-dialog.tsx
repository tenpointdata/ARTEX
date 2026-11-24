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

export type TrafficDeleteMode = "filter" | "selected" | "all";

// TrafficDeleteDialog confirms a host-scoped delete or the full purge.
export function TrafficDeleteDialog({
  mode,
  deleting,
  totalCount,
  selectedHosts,
  hostQ,
  onCancel,
  onConfirm,
}: {
  mode: TrafficDeleteMode | null;
  deleting: boolean;
  totalCount: number;
  selectedHosts: string[];
  hostQ: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const purging = mode === "all";
  const deleteTitle = mode
    ? {
        all: t("Purge all traffic records?"),
        selected: t("Delete all traffic for the {count} selected targets?", { count: selectedHosts.length }),
        filter: t("Delete all traffic for this target?"),
      }[mode]
    : "";
  return (
    <AlertDialog
      open={mode !== null}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{deleteTitle}</AlertDialogTitle>
          <AlertDialogDescription>
            {mode === "all" && (
              <>
                {t(
                  "This will permanently delete all {count} traffic records (including raw requests/responses), ignoring the current filters. This action cannot be undone. Traffic evidence already bound to findings lives in a separate evidence store and is unaffected.",
                  { count: totalCount },
                )}
                <br />
                <span className="text-muted-foreground">
                  {t(
                    "Purging also compacts storage, returning the index's disk space to the system; traffic recording pauses briefly during this time.",
                  )}
                </span>
              </>
            )}
            {mode === "selected" && (
              <>
                {t(
                  "This will permanently delete all traffic records for {count} targets ({hosts}), including raw requests/responses. This action cannot be undone.",
                  {
                    count: selectedHosts.length,
                    hosts: `${selectedHosts.slice(0, 3).join(", ")}${selectedHosts.length > 3 ? "…" : ""}`,
                  },
                )}
              </>
            )}
            {mode === "filter" && (
              <>
                {t(
                  "This will permanently delete all traffic records for hosts containing {host} (including raw requests/responses). This action cannot be undone.",
                  { host: hostQ },
                )}
              </>
            )}
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
            {deleting ? (purging ? t("Purging…") : t("Deleting…")) : purging ? t("Confirm purge") : t("Confirm delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
