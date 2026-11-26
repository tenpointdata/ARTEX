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

import type { DeleteMode } from "./traffic-format";

interface TrafficDeleteDialogProps {
  mode: DeleteMode | null; // null = closed
  deleting: boolean;
  selectedHosts: string[];
  /** Number of checked exchanges, for the "exchanges" mode. */
  exchangeCount: number;
  hostFilter: string;
  recordCount: number;
  onCancel: () => void;
  onConfirm: () => void;
}

export function TrafficDeleteDialog({
  mode,
  deleting,
  selectedHosts,
  exchangeCount,
  hostFilter,
  recordCount,
  onCancel,
  onConfirm,
}: TrafficDeleteDialogProps) {
  // "Purge" for the unfiltered purge, "Delete" for the host-scoped ones — the dialog's
  // title and its confirm button both follow from which is in play.
  const purging = mode === "all";
  const title = mode
    ? {
        all: t("Purge all traffic records?"),
        selected: t("Delete all traffic for the {count} selected targets?", { count: selectedHosts.length }),
        exchanges: t("Delete the {count} selected exchanges?", { count: exchangeCount }),
        filter: t("Delete all traffic for this target?"),
      }[mode]
    : "";
  return (
    <AlertDialog open={mode !== null} onOpenChange={(o) => !o && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            {mode === "all" && (
              <>
                {t(
                  "This will permanently delete all {count} traffic records (including raw requests/responses), ignoring the current filters. This action cannot be undone. Traffic evidence already bound to findings lives in a separate evidence store and is unaffected.",
                  { count: recordCount },
                )}
                <br />
                <span className="text-muted-foreground">
                  {t(
                    "Purging also compacts storage, returning the index's disk space to the system; traffic recording pauses briefly during this time.",
                  )}
                </span>
              </>
            )}
            {mode === "selected" &&
              t(
                "This will permanently delete all traffic records for {count} targets ({hosts}), including raw requests/responses. This action cannot be undone.",
                {
                  count: selectedHosts.length,
                  hosts: `${selectedHosts.slice(0, 3).join(", ")}${selectedHosts.length > 3 ? "…" : ""}`,
                },
              )}
            {mode === "exchanges" &&
              t(
                "This will permanently delete exactly the {count} selected exchanges (including raw requests/responses). Other traffic for the same hosts is kept. This action cannot be undone.",
                { count: exchangeCount },
              )}
            {mode === "filter" &&
              t(
                "This will permanently delete all traffic records for hosts containing {host} (including raw requests/responses). This action cannot be undone.",
                { host: hostFilter },
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
