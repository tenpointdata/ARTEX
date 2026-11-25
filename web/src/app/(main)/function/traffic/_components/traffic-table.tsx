"use client";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { SortableHead } from "@/components/ui/sortable-head";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { TrafficExchange } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtBytes, fmtTime, type SortField, statusTone } from "./traffic-format";

/** Checked exchanges keyed by id; the value keeps host/url available across pages. */
export type TrafficSelection = ReadonlyMap<string, TrafficExchange>;

export function MethodBadge({ method }: { method: string }) {
  return <Badge className="shrink-0 font-mono">{method}</Badge>;
}

interface TrafficTableProps {
  exchanges: TrafficExchange[];
  loading: boolean;
  checked: TrafficSelection;
  onCheckedChange: (next: TrafficSelection) => void;
  openId: string | undefined;
  onOpen: (exchange: TrafficExchange) => void;
  sort: { field: SortField; direction: "asc" | "desc" };
  onSort: (field: SortField) => void;
}

export function TrafficTable({
  exchanges,
  loading,
  checked,
  onCheckedChange,
  openId,
  onOpen,
  sort,
  onSort,
}: TrafficTableProps) {
  const toggle = (exchange: TrafficExchange, on: boolean) => {
    const next = new Map(checked);
    if (on) next.set(exchange.id, exchange);
    else next.delete(exchange.id);
    onCheckedChange(next);
  };
  const togglePage = (on: boolean) => {
    const next = new Map(checked);
    for (const e of exchanges) {
      if (on) next.set(e.id, e);
      else next.delete(e.id);
    }
    onCheckedChange(next);
  };
  const pageChecked = exchanges.length > 0 && exchanges.every((e) => checked.has(e.id));
  const pagePartial = !pageChecked && exchanges.some((e) => checked.has(e.id));

  return (
    <div className="flex h-[calc(100vh-15rem)] min-h-0 flex-col">
      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
        <div className="min-h-0 flex-1 overflow-auto">
          <Table>
            <TableHeader className="sticky top-0 z-10 bg-card">
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox
                    aria-label={t("Select this page's traffic")}
                    checked={pageChecked ? true : pagePartial ? "indeterminate" : false}
                    onCheckedChange={(on) => togglePage(on === true)}
                  />
                </TableHead>
                <SortableHead
                  field="ts"
                  label={t("Time")}
                  activeField={sort.field}
                  direction={sort.direction}
                  onSort={onSort}
                  className="w-36"
                />
                <TableHead className="w-44">host</TableHead>
                <TableHead className="w-20">{t("Method")}</TableHead>
                <TableHead>URL</TableHead>
                <SortableHead
                  field="status"
                  label={t("Status")}
                  activeField={sort.field}
                  direction={sort.direction}
                  onSort={onSort}
                  className="w-20"
                />
                <TableHead className="w-36">content-type</TableHead>
                <SortableHead
                  field="resp_len"
                  label={t("Response size")}
                  activeField={sort.field}
                  direction={sort.direction}
                  onSort={onSort}
                  align="right"
                  className="w-24 text-right"
                />
              </TableRow>
            </TableHeader>
            <TableBody>
              {exchanges.map((e) => (
                <TableRow
                  key={e.id}
                  data-state={checked.has(e.id) ? "selected" : undefined}
                  className={cn("cursor-pointer", openId === e.id && "bg-accent hover:bg-accent")}
                  onClick={() => onOpen(e)}
                >
                  <TableCell>
                    <Checkbox
                      aria-label={t("Select traffic {id}", { id: e.id })}
                      checked={checked.has(e.id)}
                      onClick={(event) => event.stopPropagation()}
                      onCheckedChange={(on) => toggle(e, on === true)}
                    />
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground tabular-nums">{fmtTime(e.ts)}</TableCell>
                  <TableCell className="font-mono text-xs">{e.host}</TableCell>
                  <TableCell>
                    <MethodBadge method={e.method} />
                  </TableCell>
                  <TableCell className="max-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="min-w-0 truncate font-mono text-xs">{e.url}</span>
                      {e.rewritten ? (
                        <Badge
                          variant="outline"
                          className="shrink-0 border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-medium text-amber-600 dark:text-amber-400"
                        >
                          {t("Rewritten")}
                        </Badge>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <span className={cn("font-mono text-xs font-semibold tabular-nums", statusTone(e.status))}>
                      {e.status}
                    </span>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{e.content_type}</TableCell>
                  <TableCell className="text-right text-xs tabular-nums">{fmtBytes(e.resp_len)}</TableCell>
                </TableRow>
              ))}
              {exchanges.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="py-12 text-center text-sm text-muted-foreground">
                    {loading ? t("Loading…") : t("No matching traffic.")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </Card>
    </div>
  );
}
