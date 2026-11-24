"use client";

import * as React from "react";
import Link from "next/link";

import { SortableHead } from "@/components/ui/sortable-head";
import { Table, TableBody, TableCell, TableFooter, TableHeader, TableRow, TableHead } from "@/components/ui/table";
import { t } from "@/i18n";
import type { SortDirection } from "@/lib/sort-preference";
import type { CostGroupBy, CostRow } from "@/lib/types";
import {
  costMetrics,
  fmtDuration,
  fmtEur,
  fmtInt,
  fmtPct,
  sortCostRows,
  totalRow,
  type CostSortField,
} from "./cost-metrics";

const num = "text-right tabular-nums";

function CostCell({ row }: { row: CostRow }) {
  return (
    <TableCell
      className={num}
      title={row.unpriced_calls > 0 ? t("{n} calls have no price set; cost covers priced calls only", { n: row.unpriced_calls }) : undefined}
    >
      {fmtEur(row.cost_eur)}
      {row.unpriced_calls > 0 && <span className="text-muted-foreground"> *</span>}
    </TableCell>
  );
}

function DataCells({ row }: { row: CostRow }) {
  const m = costMetrics(row);
  return (
    <>
      <TableCell className={num}>{row.calls.toLocaleString()}</TableCell>
      <TableCell className={num}>{fmtInt(row.input_tokens)}</TableCell>
      <TableCell className={num}>{fmtInt(row.output_tokens)}</TableCell>
      <TableCell className={num}>{fmtPct(m.cacheHit)}</TableCell>
      <TableCell className={num}>{m.outIn === null ? "—" : m.outIn.toFixed(3)}</TableCell>
      <CostCell row={row} />
      <TableCell className={num}>{fmtEur(m.eurPerMTok)}</TableCell>
      <TableCell className={num}>{fmtDuration(row.latency_ms)}</TableCell>
      <TableCell className={num}>{fmtDuration(m.wallMs)}</TableCell>
      <TableCell className={num}>{fmtDuration(m.avgLatencyMs)}</TableCell>
      <TableCell className={num}>{row.errors > 0 ? <span className="text-destructive">{row.errors}</span> : 0}</TableCell>
    </>
  );
}

export function CostTable({ rows, by }: { rows: CostRow[]; by: CostGroupBy }) {
  const [sort, setSort] = React.useState<{ field: CostSortField; dir: SortDirection }>({ field: "cost", dir: "desc" });
  const sorted = React.useMemo(() => sortCostRows(rows, sort.field, sort.dir), [rows, sort]);
  const total = React.useMemo(() => totalRow(rows), [rows]);

  const onSort = (field: CostSortField) =>
    setSort((s) => ({ field, dir: s.field === field ? (s.dir === "desc" ? "asc" : "desc") : field === "name" ? "asc" : "desc" }));
  const head = (field: CostSortField, label: string, align: "left" | "right" = "right") => (
    <SortableHead field={field} label={label} activeField={sort.field} direction={sort.dir} align={align} onSort={onSort} />
  );

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {head("name", t("Name"), "left")}
          {head("calls", t("Calls"))}
          {head("in", t("Tokens in"))}
          {head("out", t("Tokens out"))}
          {head("cache", t("Cache hit"))}
          {head("ratio", t("Out/in ratio"))}
          {head("cost", t("Cost (EUR)"))}
          {head("perM", t("EUR / 1M tokens"))}
          {head("llm", t("LLM time"))}
          {head("wall", t("Wall time"))}
          {head("avg", t("Avg latency"))}
          {head("errors", t("Errors"))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {sorted.map((r) => {
          const name = r.label || r.key || t("(unlabelled)");
          return (
            <TableRow key={r.key}>
              <TableCell className="max-w-64 truncate font-medium" title={name}>
                {by === "task" && r.key ? (
                  <Link className="hover:underline" href={`/function/tasks/detail?id=${encodeURIComponent(r.key)}`}>
                    {name}
                  </Link>
                ) : (
                  name
                )}
              </TableCell>
              <DataCells row={r} />
            </TableRow>
          );
        })}
        {sorted.length === 0 && (
          <TableRow>
            <TableHead colSpan={12} className="py-8 text-center font-normal text-muted-foreground">
              {t("No LLM usage in this range")}
            </TableHead>
          </TableRow>
        )}
      </TableBody>
      {sorted.length > 0 && (
        <TableFooter>
          <TableRow>
            <TableCell className="font-semibold">{t("Total")}</TableCell>
            <DataCells row={total} />
          </TableRow>
        </TableFooter>
      )}
    </Table>
  );
}
