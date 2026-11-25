"use client";

import { CopyIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";

import type { TrafficSelection } from "./traffic-table";

interface TrafficBulkBarProps {
  selection: TrafficSelection;
  onClear: () => void;
  onLink: () => void;
  /** Opens the confirm dialog to delete every exchange of the selection's hosts. */
  onDeleteTargets: (hosts: string[]) => void;
}

// Bulk actions over the checked exchanges. Uses only existing capabilities: the
// finding-link dialog, clipboard, and the confirmed host-scoped delete.
export function TrafficBulkBar({ selection, onClear, onLink, onDeleteTargets }: TrafficBulkBarProps) {
  const canOperate = useCan("operate"); // linking and deleting mutate; copying URLs is read-only
  const count = selection.size;
  const copyUrls = () => {
    const urls = [...selection.values()].map((e) => e.url).join("\n");
    navigator.clipboard
      .writeText(urls)
      .then(() => toast.success(t("Copied {count} URLs", { count })))
      .catch((e: Error) => toast.error(e.message));
  };
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-sm text-muted-foreground">{t("{count} flows selected", { count })}</span>
      {canOperate && (
        <Button variant="outline" size="sm" disabled={count === 0} onClick={onLink}>
          {t("Link to finding")}
        </Button>
      )}
      <Button variant="outline" size="sm" disabled={count === 0} onClick={copyUrls}>
        <CopyIcon className="size-3.5" />
        {t("Copy URLs")}
      </Button>
      {canOperate && (
        <Button
          variant="outline"
          size="sm"
          className="text-destructive hover:bg-destructive/10 hover:text-destructive"
          disabled={count === 0}
          onClick={() => onDeleteTargets([...new Set([...selection.values()].map((e) => e.host))])}
        >
          <Trash2Icon className="size-3.5" />
          {t("Delete targets of selection")}
        </Button>
      )}
      {count > 0 ? (
        <Button variant="ghost" size="sm" onClick={onClear}>
          {t("Clear selection")}
        </Button>
      ) : null}
    </div>
  );
}
