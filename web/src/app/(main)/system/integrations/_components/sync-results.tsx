"use client";

import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { TicketSyncAction, TicketSyncResult } from "@/lib/types";

const ACTION_LABEL: Record<TicketSyncAction, string> = {
  unchanged: t("Unchanged"),
  pulled: t("Pulled from tracker"),
  pushed: t("Pushed to tracker"),
  error: t("Error"),
};

function statusChange(r: TicketSyncResult): string {
  if (!r.from && !r.to) return "—";
  return `${r.from ?? "—"} → ${r.to ?? "—"}`;
}

// SyncResultsTable lists what one sync did per ticket; changed and failed rows come first.
export function SyncResultsTable({ results }: { results: TicketSyncResult[] }) {
  if (results.length === 0) {
    return <p className="text-muted-foreground px-6 pb-4 text-sm">{t("This tracker has no linked tickets yet.")}</p>;
  }
  const order: Record<TicketSyncAction, number> = { error: 0, pulled: 1, pushed: 2, unchanged: 3 };
  const rows = [...results].sort((a, b) => order[a.action] - order[b.action]);
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("Finding")}</TableHead>
          <TableHead>{t("Ticket")}</TableHead>
          <TableHead>{t("Result")}</TableHead>
          <TableHead>{t("Status change")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((r) => (
          <TableRow key={`${r.finding_id}-${r.external_id}`}>
            <TableCell>
              <Link className="font-mono text-xs hover:underline" href={`/function/findings/detail?id=${r.finding_id}`}>
                #{r.finding_id}
              </Link>
            </TableCell>
            <TableCell className="font-mono text-xs">{r.external_id}</TableCell>
            <TableCell>
              <Badge variant={r.action === "error" ? "destructive" : "outline"}>
                {ACTION_LABEL[r.action] ?? r.action}
              </Badge>
            </TableCell>
            <TableCell className="text-sm">
              {r.error ? <span className="text-destructive">{r.error}</span> : statusChange(r)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
