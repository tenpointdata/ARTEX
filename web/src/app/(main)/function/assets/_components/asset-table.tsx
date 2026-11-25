"use client";

import type * as React from "react";

import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { Asset } from "@/lib/types";

import type { AssetCellContext, AssetTabSpec } from "./asset-columns";
import { RowDeleteButton } from "./row-delete-button";

const PAGE_SIZES = [25, 50, 100, 200];

// AssetSelection is the row selection that feeds bulk delete. Pass it (and
// onDelete) only when the role may delete: without them the table has no
// checkbox or trash column at all.
export interface AssetSelection {
  selected: Set<number>;
  toggle: (id: number) => void;
  toggleAll: (ids: number[]) => void;
}

export interface AssetPaging {
  total: number;
  page: number;
  size: number;
  onSize: React.Dispatch<React.SetStateAction<number>>;
  onPage: React.Dispatch<React.SetStateAction<number>>;
}

function checkState(ids: number[], selected: Set<number>): boolean | "indeterminate" {
  if (ids.length > 0 && ids.every((id) => selected.has(id))) return true;
  return ids.some((id) => selected.has(id)) ? "indeterminate" : false;
}

export function AssetTable({
  spec,
  rows,
  ctx,
  loaded,
  paging,
  selection,
  onDelete,
}: {
  spec: AssetTabSpec;
  rows: Asset[];
  ctx: AssetCellContext;
  loaded: boolean;
  paging: AssetPaging;
  selection?: AssetSelection;
  onDelete?: (ids: number[]) => void;
}) {
  const pageIds = rows.map((r) => r.id);
  const isSelected = (id: number) => (selection ? selection.selected.has(id) : false);
  const colCount = spec.columns.length + (selection ? 1 : 0) + (onDelete ? 1 : 0);

  return (
    <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
      <div className="scrollbar-thin scrollbar-track-transparent min-h-0 flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-card">
            <TableRow>
              {selection && (
                <TableHead className="w-8 pr-0">
                  <Checkbox
                    aria-label={t("Select all")}
                    checked={checkState(pageIds, selection.selected)}
                    onCheckedChange={() => selection.toggleAll(pageIds)}
                  />
                </TableHead>
              )}
              {spec.columns.map((c) => (
                <TableHead key={c.header}>{t(c.header)}</TableHead>
              ))}
              {onDelete && <TableHead className="w-8" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((a) => (
              <TableRow key={a.id} className={isSelected(a.id) ? "bg-muted/40" : undefined}>
                {selection && (
                  <TableCell className="w-8 pr-0">
                    <Checkbox
                      aria-label={t("Select {name}", { name: spec.name(a) })}
                      checked={selection.selected.has(a.id)}
                      onCheckedChange={() => selection.toggle(a.id)}
                    />
                  </TableCell>
                )}
                {spec.columns.map((c) => (
                  <TableCell key={c.header} className={c.className} title={c.title?.(a)}>
                    {c.cell(a, ctx)}
                  </TableCell>
                ))}
                {onDelete && (
                  <TableCell className="w-8 pl-0">
                    <RowDeleteButton
                      label={t("Delete asset {name}", { name: spec.name(a) })}
                      onClick={() => onDelete([a.id])}
                    />
                  </TableCell>
                )}
              </TableRow>
            ))}
            {rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={colCount} className="py-12 text-center text-muted-foreground text-sm">
                  {loaded ? t("No data.") : t("Loading…")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <PagingBar paging={paging} shown={rows.length} />
    </Card>
  );
}

function PagingBar({ paging, shown }: { paging: AssetPaging; shown: number }) {
  const { total, page, size, onSize, onPage } = paging;
  if (total <= 0) return null;
  const pageCount = Math.max(1, Math.ceil(total / size));
  const start = page * size + 1;
  const end = page * size + shown;
  return (
    <div className="flex shrink-0 items-center gap-2 border-t px-3 py-1.5 text-muted-foreground text-xs">
      <Select value={String(size)} onValueChange={(v) => onSize(Number(v))}>
        <SelectTrigger size="sm" className="h-7 w-24">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {PAGE_SIZES.map((n) => (
              <SelectItem key={n} value={String(n)}>
                {t("{count} / page", { count: n })}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <span className="tabular-nums">
        {start}–{end} / {total}
      </span>
      {pageCount > 1 && (
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            className="size-7"
            disabled={page <= 0}
            aria-label={t("Previous page")}
            onClick={() => onPage((p) => Math.max(0, p - 1))}
          >
            <ChevronLeftIcon />
          </Button>
          <span className="tabular-nums">
            {page + 1} / {pageCount}
          </span>
          <Button
            variant="outline"
            size="icon"
            className="size-7"
            disabled={page + 1 >= pageCount}
            aria-label={t("Next page")}
            onClick={() => onPage((p) => Math.min(pageCount - 1, p + 1))}
          >
            <ChevronRightIcon />
          </Button>
        </div>
      )}
    </div>
  );
}
