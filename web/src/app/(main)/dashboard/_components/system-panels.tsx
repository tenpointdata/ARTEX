"use client";

import * as React from "react";

import Link from "next/link";

import { ActivityIcon, ArrowUpRightIcon, NetworkIcon, ShieldCheckIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { t } from "@/i18n";
import type { InterceptPending, LLMProfile, Settings, Stats, TrafficExchange } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ASSET_COLORS, ASSET_TYPE_LABELS, statusBg, statusColor } from "./format";
import { SectionTitle } from "./live-bar";

export function SystemPanels({
  assetCounts,
  totalAssets,
  traffic,
  stats,
  settings,
  pending,
  activeProfile,
}: {
  assetCounts: Record<string, number>;
  totalAssets: number;
  traffic: TrafficExchange[];
  stats: Stats | null;
  settings: Settings | null;
  pending: InterceptPending[];
  activeProfile: LLMProfile | undefined;
}) {
  const pendingCount = pending.length;
  const assetByType = React.useMemo(
    () =>
      Object.entries(assetCounts)
        .filter(([, n]) => n > 0)
        .sort(([, a], [, b]) => b - a)
        .slice(0, 8),
    [assetCounts],
  );
  const assetMax = assetByType[0]?.[1] ?? 1;
  const trafficByCodes = React.useMemo(() => {
    const m: Record<number, number> = {};
    for (const e of traffic) {
      const bucket = Math.floor(e.status / 100) * 100;
      m[bucket] = (m[bucket] ?? 0) + 1;
    }
    return Object.entries(m)
      .sort(([a], [b]) => Number(a) - Number(b))
      .map(([code, n]) => ({ code: Number(code), n }));
  }, [traffic]);
  const trafficMax = Math.max(...trafficByCodes.map((x) => x.n), 1);
  const recentTraffic = React.useMemo(
    () => [...traffic].sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts)).slice(0, 5),
    [traffic],
  );

  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-3">
      {/* Asset distribution */}
      <Card className="p-4">
        <SectionTitle icon={NetworkIcon} sub={t("By type")}>
          {t("Asset distribution")}
        </SectionTitle>

        {assetByType.length === 0 ? (
          <div className="py-6 text-center text-xs text-muted-foreground">{t("No asset data")}</div>
        ) : (
          <div className="flex flex-col gap-2">
            {assetByType.map(([type, count]) => (
              <div key={type} className="flex items-center gap-2 text-xs">
                <span className="w-8 shrink-0 text-right text-[10px] text-muted-foreground">
                  {ASSET_TYPE_LABELS[type] ?? type}
                </span>
                <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                  <div
                    className={cn("h-full rounded-full", ASSET_COLORS[type] ?? "bg-slate-400")}
                    style={{ width: `${(count / assetMax) * 100}%` }}
                  />
                </div>
                <span className="w-4 text-right tabular-nums">{count}</span>
              </div>
            ))}
          </div>
        )}

        <Link
          href="/function/assets"
          className="mt-3 flex items-center justify-between border-t pt-3 text-[10px] text-muted-foreground hover:text-foreground"
        >
          {t("{count} nodes in total", { count: totalAssets })}
          <ArrowUpRightIcon className="size-3" />
        </Link>
      </Card>

      {/* Traffic status codes */}
      <Card className="p-4">
        <SectionTitle icon={ActivityIcon} sub={t("{count} requests", { count: traffic.length })}>
          {t("Traffic status codes")}
        </SectionTitle>

        {/* bar chart */}
        {trafficByCodes.length === 0 ? (
          <div className="py-6 text-center text-xs text-muted-foreground">{t("No traffic data")}</div>
        ) : (
          <>
            <div className="mb-3 flex items-end gap-2" style={{ height: 52 }}>
              {trafficByCodes.map(({ code, n }) => (
                <div key={code} className="flex flex-1 flex-col items-center gap-1">
                  <span className="text-[9px] tabular-nums text-muted-foreground">{n}</span>
                  <div
                    className={cn("w-full min-h-1 rounded-sm", statusBg(code), "opacity-80")}
                    style={{ height: Math.max(4, (n / trafficMax) * 36) }}
                  />
                  <span className={cn("text-[9px] font-mono", statusColor(code))}>{code}</span>
                </div>
              ))}
            </div>

            <div className="border-t pt-2.5">
              <Link
                href="/function/traffic"
                className="mb-1.5 flex items-center justify-between text-[10px] text-muted-foreground hover:text-foreground"
              >
                {t("Recent requests")}
                <ArrowUpRightIcon className="size-3" />
              </Link>
              <div className="flex flex-col gap-1.5">
                {recentTraffic.map((e) => (
                  <div key={e.id} className="flex items-center gap-1.5 text-[10px]">
                    <span
                      className={cn(
                        "shrink-0 rounded border px-1 py-0 font-mono text-[9px]",
                        e.method === "GET"
                          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                          : "border-blue-500/30 bg-blue-500/10 text-blue-400",
                      )}
                    >
                      {e.method}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-muted-foreground">
                      {e.host}
                      {e.url.replace(/^https?:\/\/[^/]+/, "").substring(0, 30)}
                    </span>
                    <span className={cn("shrink-0 font-mono text-[9px]", statusColor(e.status))}>{e.status}</span>
                  </div>
                ))}
              </div>
            </div>
          </>
        )}
      </Card>

      {/* System status & pending approvals */}
      <Card className="p-4">
        <SectionTitle icon={ShieldCheckIcon}>{t("System status")}</SectionTitle>

        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
            <div className="text-[10px] text-muted-foreground">{t("LLM config")}</div>
            <Badge
              variant="outline"
              className={cn(
                "text-[10px]",
                stats?.llm_configured
                  ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                  : "border-red-500/30 bg-red-500/10 text-red-400",
              )}
            >
              {stats?.llm_configured ? t("Configured") : t("Not configured")}
            </Badge>
          </div>

          <div className="rounded-lg border bg-muted/20 px-3 py-2">
            <div className="flex items-center justify-between">
              <div className="text-[10px] text-muted-foreground">{t("Traffic capture")}</div>
              <Badge
                variant="outline"
                className={cn(
                  "text-[10px]",
                  settings?.traffic_capture
                    ? "border-violet-500/30 bg-violet-500/10 text-violet-400"
                    : "border-amber-500/30 bg-amber-500/10 text-amber-500",
                )}
              >
                {settings?.traffic_capture ? t("On") : t("Off")}
              </Badge>
            </div>
            {settings && !settings.traffic_capture && (
              <p className="mt-1.5 text-[10px] leading-relaxed text-amber-500">
                {t(
                  "Traffic capture is off: findings stay recorded, but captured HTTP traffic is required for provable findings.",
                )}{" "}
                <Link href="/system/settings" className="underline">
                  {t("Enable in settings")}
                </Link>
              </p>
            )}
          </div>

          {activeProfile && (
            <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2">
              <div className="text-[10px] text-muted-foreground">{t("Active model")}</div>
              <span className="font-mono text-[10px]">{activeProfile.model}</span>
            </div>
          )}
        </div>

        {/* pending approvals */}
        {pendingCount > 0 && (
          <div className="mt-3">
            <div className="mb-1.5 text-[10px] font-medium text-amber-400">
              {t("Pending approvals ({count})", { count: pendingCount })}
            </div>
            <div className="flex flex-col gap-1.5">
              {pending.slice(0, 3).map((p) => (
                <Link
                  key={p.id}
                  href="/system/intercept/approvals"
                  className="flex items-center justify-between rounded-lg border border-amber-500/20 bg-amber-500/5 px-2.5 py-2 hover:bg-amber-500/10"
                >
                  <div className="min-w-0">
                    <div className="text-[10px] font-medium">{p.tool_name}</div>
                    <div className="text-[9px] text-muted-foreground">{p.agent_name}</div>
                  </div>
                  <ArrowUpRightIcon className="size-3 shrink-0 text-amber-400" />
                </Link>
              ))}
              {pendingCount > 3 && (
                <Link
                  href="/system/intercept/approvals"
                  className="text-center text-[10px] text-muted-foreground hover:text-foreground"
                >
                  {t("{count} more…", { count: pendingCount - 3 })}
                </Link>
              )}
            </div>
          </div>
        )}

        {pendingCount === 0 && (
          <div className="mt-3 rounded-lg border bg-muted/10 px-3 py-3 text-center text-[10px] text-muted-foreground">
            <ShieldCheckIcon className="mx-auto mb-1 size-4 text-emerald-500/50" />
            {t("No pending interception approvals")}
          </div>
        )}
      </Card>
    </div>
  );
}
