"use client";

import * as React from "react";

import Link from "next/link";

import { ActivityIcon, ArrowUpRightIcon, BugIcon, ClockIcon } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { t } from "@/i18n";
import type { Activity, Finding, Task } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtRel, SEVERITY_RANK, workerBg } from "./format";

const TH = "px-3 py-2 text-left text-[9px] font-semibold uppercase tracking-widest text-muted-foreground";

function PanelHeader({
  icon: Icon,
  title,
  count,
  href,
  hrefLabel,
}: {
  icon: React.ElementType;
  title: string;
  count: string;
  href: string;
  hrefLabel: string;
}) {
  return (
    <div className="flex items-center justify-between border-b px-4 py-3">
      <div className="flex items-center gap-1.5 text-xs font-semibold">
        <Icon className="size-3.5 text-muted-foreground" />
        {title}
      </div>
      <div className="flex items-center gap-2">
        <span className="text-[10px] text-muted-foreground">{count}</span>
        <Link href={href} className="flex items-center gap-0.5 text-[10px] text-muted-foreground hover:text-foreground">
          {hrefLabel} <ArrowUpRightIcon className="size-3" />
        </Link>
      </div>
    </div>
  );
}

function Empty({ cols, text }: { cols: number; text: string }) {
  return (
    <tr>
      <td colSpan={cols} className="px-4 py-6 text-center text-xs text-muted-foreground">
        {text}
      </td>
    </tr>
  );
}

// Running tasks first (those are the live engagement), then the most recently
// active of the rest, so the table is never empty while history exists.
export function TasksTable({ tasks }: { tasks: Task[] }) {
  const rows = React.useMemo(() => {
    const recency = (x: Task) => x.last_activity_unix ?? x.created_unix ?? 0;
    return [...tasks]
      .sort((a, b) => Number(b.status === "running") - Number(a.status === "running") || recency(b) - recency(a))
      .slice(0, 8);
  }, [tasks]);
  const running = tasks.filter((x) => x.status === "running").length;
  return (
    <Card className="overflow-hidden p-0">
      <PanelHeader
        icon={ClockIcon}
        title={t("Running tasks")}
        count={t("{count} tasks", { count: running })}
        href="/function/tasks"
        hrefLabel={t("All")}
      />
      <table className="w-full border-collapse text-xs">
        <thead>
          <tr className="border-b">
            {["Task", "Status", "Engine", "Goal progress", "In flight", "Last activity"].map((h) => (
              <th key={h} className={TH}>
                {t(h)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <Empty cols={6} text={t("No tasks")} />
          ) : (
            rows.map((x) => {
              const pct = x.goals_total ? Math.round(((x.goals_met ?? 0) / x.goals_total) * 100) : null;
              return (
                <tr key={x.id} className="border-b transition-colors last:border-0 hover:bg-muted/30">
                  <td className="max-w-xs px-3 py-2.5">
                    <Link href={`/function/tasks/detail?id=${x.id}`} className="group flex flex-col">
                      <span className="truncate font-medium group-hover:underline">{x.description}</span>
                      <span className="truncate font-mono text-[10px] text-muted-foreground">{x.id}</span>
                    </Link>
                  </td>
                  <td className="px-3 py-2.5">
                    <StatusBadge domain="task" value={x.status} dot className="px-1.5 py-0 text-[10px]" />
                  </td>
                  <td className="px-3 py-2.5">
                    <StatusBadge domain="engine" value={x.engine_mode ?? "idle"} className="px-1.5 py-0 text-[10px]" />
                  </td>
                  <td className="px-3 py-2.5">
                    {pct !== null ? (
                      <div className="flex items-center gap-2">
                        <Progress value={pct} className="h-1 w-14" />
                        <span className="tabular-nums text-muted-foreground">
                          {x.goals_met ?? 0}/{x.goals_total}
                        </span>
                      </div>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2.5 tabular-nums">
                    {(x.in_flight ?? 0) > 0 ? (
                      <span className="font-semibold">{x.in_flight}</span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2.5 tabular-nums text-muted-foreground">
                    {fmtRel(x.last_activity_unix ?? x.created_unix)}
                  </td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </Card>
  );
}

// Severity first (critical on top) among the newest findings; each row links
// straight to the finding detail.
export function FindingsTable({ findings }: { findings: Finding[] }) {
  const rows = React.useMemo(
    () =>
      [...findings]
        .sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts))
        .slice(0, 20)
        .sort((a, b) => (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0))
        .slice(0, 8),
    [findings],
  );
  return (
    <Card className="overflow-hidden p-0">
      <PanelHeader
        icon={BugIcon}
        title={t("Newest findings")}
        count={t("{count} events", { count: findings.length })}
        href="/function/findings?status=pending&triage=1"
        hrefLabel={t("Pending")}
      />
      <table className="w-full border-collapse text-xs">
        <thead>
          <tr className="border-b">
            <th className={TH}>{t("Severity")}</th>
            <th className={TH}>{t("Summary")}</th>
            <th className={TH}>{t("Time")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <Empty cols={3} text={t("No findings")} />
          ) : (
            rows.map((f) => (
              <tr key={f.id} className="border-b transition-colors last:border-0 hover:bg-muted/30">
                <td className="px-3 py-2 align-top">
                  <StatusBadge domain="severity" value={f.severity} className="px-1.5 py-0 text-[10px]" />
                </td>
                <td className="max-w-0 px-3 py-2">
                  <Link href={`/function/findings/detail?id=${encodeURIComponent(f.id)}`} className="group block">
                    <span className="block truncate font-semibold group-hover:underline">{f.name || f.vulnclass}</span>
                    <span className="block truncate text-[10px] text-muted-foreground">{f.summary}</span>
                  </Link>
                </td>
                <td className="whitespace-nowrap px-3 py-2 align-top text-[10px] tabular-nums text-muted-foreground">
                  {fmtRel(f.ts)}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </Card>
  );
}

function kindLabel(a: Activity): string {
  if (a.kind === "tool_use") return a.tool ?? t("tool_use");
  if (a.kind === "tool_result") return t("tool_result");
  return t(a.kind);
}

export function ActivityTable({ activity }: { activity: Activity[] }) {
  const rows = React.useMemo(
    () =>
      activity
        .filter((a) => a.kind !== "usage")
        .sort((a, b) => b.seq - a.seq)
        .slice(0, 8),
    [activity],
  );
  return (
    <Card className="overflow-hidden p-0">
      <PanelHeader
        icon={ActivityIcon}
        title={t("Activity feed")}
        count={t("{count} events", { count: rows.length })}
        href="/function/tasks"
        hrefLabel={t("View tasks")}
      />
      <table className="w-full border-collapse text-xs">
        <thead>
          <tr className="border-b">
            <th className={TH}>{t("Worker")}</th>
            <th className={TH}>{t("Summary")}</th>
            <th className={TH}>{t("Time")}</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <Empty cols={3} text={t("No activity records")} />
          ) : (
            rows.map((a) => (
              <tr key={a.seq} className="border-b transition-colors last:border-0 hover:bg-muted/30">
                <td className="px-3 py-2 align-top">
                  <span className={cn("rounded border px-1.5 py-0.5 font-mono text-[9px]", workerBg(a.worker))}>
                    {a.worker}
                  </span>
                </td>
                <td className="max-w-0 px-3 py-2">
                  <div className="flex items-center gap-1.5">
                    <span className="text-[11px] font-medium">{kindLabel(a)}</span>
                    <Badge variant="outline" className="px-1.5 py-0 text-[9px] text-muted-foreground">
                      {t(a.kind)}
                    </Badge>
                  </div>
                  <div className="truncate text-[10px] text-muted-foreground">{a.summary}</div>
                </td>
                <td className="whitespace-nowrap px-3 py-2 align-top text-[10px] tabular-nums text-muted-foreground">
                  {fmtRel(a.ts)}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </Card>
  );
}
