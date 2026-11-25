"use client";

import * as React from "react";

import { CopyIcon, DownloadIcon, Loader2Icon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import { api } from "@/lib/api";

import type { TrafficSelection } from "./traffic-table";

interface TrafficBulkBarProps {
  selection: TrafficSelection;
  onClear: () => void;
  onLink: () => void;
  /** Opens the confirm dialog to delete exactly the selected exchanges. */
  onDeleteSelected: () => void;
  /** Opens the confirm dialog to delete every exchange of the selection's hosts. */
  onDeleteTargets: (hosts: string[]) => void;
}

const destructive = "text-destructive hover:bg-destructive/10 hover:text-destructive";

// Bulk actions over the checked exchanges: link to a finding, copy URLs, export
// exactly the selection as HAR, delete exactly the selection, or delete every
// exchange of the selection's hosts. Both deletes go through a confirm dialog.
export function TrafficBulkBar({ selection, onClear, onLink, onDeleteSelected, onDeleteTargets }: TrafficBulkBarProps) {
  const count = selection.size;
  const [exporting, setExporting] = React.useState(false);
  const copyUrls = () => {
    const urls = [...selection.values()].map((e) => e.url).join("\n");
    navigator.clipboard
      .writeText(urls)
      .then(() => toast.success(t("Copied {count} URLs", { count })))
      .catch((e: Error) => toast.error(e.message));
  };
  const exportHar = () => {
    setExporting(true);
    api
      .trafficExportHARByIds([...selection.keys()])
      .then((meta) => {
        if (meta.truncated) {
          toast.warning(
            t("Exported {exported} of {matched} matching exchanges (truncated: {reason})", {
              exported: meta.exported,
              matched: meta.matched,
              reason: meta.truncatedReason ?? "limit",
            }),
          );
        } else {
          toast.success(t("Exported {count} exchanges to HAR", { count: meta.exported }));
        }
      })
      .catch((e: Error) => toast.error(e.message))
      .finally(() => setExporting(false));
  };
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-sm text-muted-foreground">{t("{count} flows selected", { count })}</span>
      <Button variant="outline" size="sm" disabled={count === 0} onClick={onLink}>
        {t("Link to finding")}
      </Button>
      <Button variant="outline" size="sm" disabled={count === 0} onClick={copyUrls}>
        <CopyIcon className="size-3.5" />
        {t("Copy URLs")}
      </Button>
      <Button variant="outline" size="sm" disabled={count === 0 || exporting} onClick={exportHar}>
        {exporting ? <Loader2Icon className="size-3.5 animate-spin" /> : <DownloadIcon className="size-3.5" />}
        {t("Export selected as HAR")}
      </Button>
      <Button variant="outline" size="sm" className={destructive} disabled={count === 0} onClick={onDeleteSelected}>
        <Trash2Icon className="size-3.5" />
        {t("Delete selected")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        className={destructive}
        disabled={count === 0}
        onClick={() => onDeleteTargets([...new Set([...selection.values()].map((e) => e.host))])}
      >
        <Trash2Icon className="size-3.5" />
        {t("Delete targets of selection")}
      </Button>
      {count > 0 ? (
        <Button variant="ghost" size="sm" onClick={onClear}>
          {t("Clear selection")}
        </Button>
      ) : null}
    </div>
  );
}
