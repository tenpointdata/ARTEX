"use client";

import Link from "next/link";

import { ActivityIcon, BugIcon, NetworkIcon, ShieldAlertIcon, TargetIcon, ZapIcon } from "lucide-react";

import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { t } from "@/i18n";
import type { Finding, InterceptPending, Settings, Task } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtTokens } from "./format";
import { LiveDot } from "./live-bar";
import type { useTokenUsage } from "./use-token-usage";

type Displayed = ReturnType<typeof useTokenUsage>["displayedTokens"];

function Tile({
  href,
  icon: Icon,
  label,
  className,
  children,
}: {
  href: string;
  icon: React.ElementType;
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Card className={cn("gap-1 transition-colors hover:bg-muted/30", className)}>
      <Link href={href} className="flex flex-col gap-1" aria-label={label}>
        <CardHeader className="pb-0">
          <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
            <Icon className="size-3" /> {label}
          </div>
        </CardHeader>
        {children}
      </Link>
    </Card>
  );
}

export function KpiRow({
  tasks,
  findings,
  pending,
  settings,
  totalAssets,
  trafficCount,
  tokens,
}: {
  tasks: Task[];
  findings: Finding[];
  pending: InterceptPending[];
  settings: Settings | null;
  totalAssets: number;
  trafficCount: number;
  tokens: Displayed;
}) {
  const running = tasks.filter((x) => x.status === "running").length;
  const paused = tasks.filter((x) => x.status === "paused").length;
  const sev = { critical: 0, high: 0, medium: 0, low: 0 };
  for (const f of findings) if (f.severity in sev) sev[f.severity as keyof typeof sev]++;
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      <Tile href="/function/tasks" icon={TargetIcon} label={t("Active tasks")}>
        <CardContent className="flex flex-col gap-1">
          <div className="flex items-baseline gap-1.5">
            <span className="text-2xl font-semibold tabular-nums">{running}</span>
            <span className="text-xs text-muted-foreground">/ {tasks.length}</span>
          </div>
          <div className="flex gap-2 text-[10px]">
            {running > 0 && <span className="text-blue-400">{t("Exploring {count}", { count: running })}</span>}
            {paused > 0 && <span className="text-amber-400">{t("Paused {count}", { count: paused })}</span>}
            {tasks.length === 0 && <span className="text-muted-foreground">{t("No tasks")}</span>}
          </div>
        </CardContent>
      </Tile>

      <Tile
        href="/system/intercept/approvals"
        icon={ShieldAlertIcon}
        label={t("Pending approvals")}
        className={pending.length > 0 ? "border-amber-500/40 bg-amber-500/5" : undefined}
      >
        <CardContent className="flex flex-col gap-1">
          <span className={cn("text-2xl font-semibold tabular-nums", pending.length > 0 && "text-amber-400")}>
            {pending.length}
          </span>
          <span className="text-[10px] text-muted-foreground">
            {pending.length > 0
              ? t("Pending approvals ({count})", { count: pending.length })
              : t("No pending interception approvals")}
          </span>
        </CardContent>
      </Tile>

      <Tile href="/function/findings" icon={BugIcon} label={t("Confirmed findings")}>
        <CardContent className="flex flex-col gap-1">
          <span className="text-2xl font-semibold tabular-nums">{findings.length}</span>
          <div className="flex flex-wrap gap-2 text-[10px]">
            <span className="text-rose-500">{t("Critical {count}", { count: sev.critical })}</span>
            <span className="text-red-400">{t("High {count}", { count: sev.high })}</span>
            <span className="text-amber-400">{t("Medium {count}", { count: sev.medium })}</span>
          </div>
        </CardContent>
      </Tile>

      <Tile href="/function/assets" icon={NetworkIcon} label={t("Asset nodes")}>
        <CardContent className="flex flex-col gap-1">
          <span className="text-2xl font-semibold tabular-nums">{totalAssets}</span>
          <span className="text-[10px] text-muted-foreground">{t("Shared across tasks")}</span>
        </CardContent>
      </Tile>

      <Tile href="/function/traffic" icon={ActivityIcon} label={t("Traffic exchanges")}>
        <CardContent className="flex flex-col gap-1">
          <span className="text-2xl font-semibold tabular-nums">{trafficCount}</span>
          <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
            {settings?.traffic_capture ? (
              <>
                <LiveDot />
                {t("Recording")}
              </>
            ) : (
              <span className="text-amber-500">{t("Capture is off · required for provable findings")}</span>
            )}
          </span>
        </CardContent>
      </Tile>

      <Tile href="/system/llm" icon={ZapIcon} label={t("Token usage")}>
        <CardContent className="flex flex-col gap-1">
          <span className="text-2xl font-semibold tabular-nums">{fmtTokens(tokens.input + tokens.output)}</span>
          <span className="text-[10px] text-muted-foreground">
            {t("In {input} (incl. cache {cache}) · Out {output}", {
              input: fmtTokens(tokens.input),
              cache: fmtTokens(tokens.cacheRead),
              output: fmtTokens(tokens.output),
            })}
          </span>
        </CardContent>
      </Tile>
    </div>
  );
}
