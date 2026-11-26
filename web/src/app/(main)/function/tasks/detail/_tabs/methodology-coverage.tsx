"use client";

import * as React from "react";

import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { MethodologyItemStatusValue, MethodologyScore } from "@/lib/types";

type StatusFilter = "all" | MethodologyItemStatusValue;

const FILTERS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "open", label: "Open" },
  { value: "in_progress", label: "In progress" },
  { value: "covered", label: "Covered" },
];

const STATUS_STYLE: Record<MethodologyItemStatusValue, { label: string; className: string }> = {
  covered: { label: "Covered", className: "text-emerald-600 dark:text-emerald-400" },
  in_progress: { label: "In progress", className: "text-amber-600 dark:text-amber-400" },
  open: { label: "Open", className: "text-muted-foreground" },
};

function MethodologyTable({ score }: { score: MethodologyScore }) {
  const [filter, setFilter] = React.useState<StatusFilter>("all");
  const items = filter === "all" ? score.items : score.items.filter((i) => i.status === filter);
  const pct = Math.round(score.pct * 100);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="text-sm font-medium">
          {score.name} <span className="text-muted-foreground text-xs font-normal">{score.version}</span>
        </span>
        <span className="text-sm tabular-nums">
          {t("{covered}/{total} covered", { covered: score.covered, total: score.total })}
        </span>
        <span className="text-muted-foreground text-xs tabular-nums">
          {t("{count} in progress", { count: score.in_progress })} · {t("{count} open", { count: score.open })}
        </span>
      </div>
      <Progress value={pct} />
      <div className="flex gap-1">
        {FILTERS.map((f) => (
          <button
            key={f.value}
            type="button"
            aria-pressed={filter === f.value}
            onClick={() => setFilter(f.value)}
            className={
              "rounded px-2 py-0.5 text-xs " +
              (filter === f.value
                ? "bg-primary text-primary-foreground"
                : "bg-muted text-muted-foreground hover:bg-muted/70")
            }
          >
            {t(f.label)}
          </button>
        ))}
      </div>
      <div className="max-h-96 overflow-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("Item")}</TableHead>
              <TableHead>{t("Category")}</TableHead>
              <TableHead>{t("Title")}</TableHead>
              <TableHead>{t("Status")}</TableHead>
              <TableHead>{t("Intents")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => {
              const st = STATUS_STYLE[item.status];
              return (
                <TableRow key={item.id}>
                  <TableCell className="font-mono text-xs">{item.id}</TableCell>
                  <TableCell className="text-xs">{item.category}</TableCell>
                  <TableCell className="whitespace-normal text-xs">{item.title}</TableCell>
                  <TableCell className={"text-xs font-medium " + st.className}>{t(st.label)}</TableCell>
                  <TableCell className="text-muted-foreground text-xs tabular-nums">
                    {item.intents?.length ? item.intents.map((n) => "#" + n).join(", ") : "—"}
                  </TableCell>
                </TableRow>
              );
            })}
            {items.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-muted-foreground text-center text-xs">
                  {t("No items")}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/** Checklist completion per testing methodology (OWASP WSTG, API Top 10, ...), stacked. */
export function MethodologyCoverage({ scores }: { scores: MethodologyScore[] }) {
  return (
    <div className="flex flex-col gap-6 border-t pt-3">
      {scores.map((s) => (
        <MethodologyTable key={s.id} score={s} />
      ))}
    </div>
  );
}
