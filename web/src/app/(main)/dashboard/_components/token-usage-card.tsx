"use client";

import { ZapIcon } from "lucide-react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";

import { Card } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart";
import { t } from "@/i18n";
import type { LLMProfile } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtTokens } from "./format";
import { dailyTrendConfig, type useTokenUsage } from "./use-token-usage";

type TokenUsage = ReturnType<typeof useTokenUsage>;

export function TokenUsageCard({ usage, llmProfiles }: { usage: TokenUsage; llmProfiles: LLMProfile[] }) {
  const {
    defaultProfileId,
    tokenVersion,
    setTokenVersion,
    tokenTab,
    setTokenTab,
    tokenDays,
    setTokenDays,
    tokenByProfile,
    displayedTokens,
    dailyTokenData,
  } = usage;
  return (
    <Card className="p-4">
      {/* Header */}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 text-xs font-semibold">
          <ZapIcon className="size-3.5 text-muted-foreground" />
          {t("LLM token usage")}
          {/* Data source toggle: old = activity stats (includes historical tasks), new = the llm_usage metering ledger (more accurate, only covers data since it was enabled) */}
          <div className="ml-1 flex gap-0.5 rounded-md border bg-muted/30 p-0.5">
            {(
              [
                { v: "old", label: t("Old") },
                { v: "new", label: t("New") },
              ] as const
            ).map(({ v, label }) => (
              <button
                type="button"
                key={v}
                onClick={() => setTokenVersion(v)}
                title={
                  v === "new"
                    ? t(
                        "New: from the llm_usage metering ledger, exact per call and including aborted runs; only covers data since it was enabled",
                      )
                    : t(
                        "Old: from activity stats (includes historical tasks); aborted runs are not counted and it cannot break down by model",
                      )
                }
                className={cn(
                  "rounded px-2 py-0.5 text-[9px] font-medium transition-colors",
                  tokenVersion === v
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {label}
              </button>
            ))}
          </div>
          <span className="text-[10px] font-normal text-muted-foreground">
            {tokenVersion === "new" ? "llm_usage" : "activity"}
          </span>
        </div>
        {/* Profile tabs */}
        <div className="flex flex-wrap items-center gap-1">
          <button
            type="button"
            onClick={() => setTokenTab("all")}
            className={cn(
              "rounded-md px-2.5 py-1 text-[10px] font-medium transition-colors",
              tokenTab === "all"
                ? "bg-foreground text-background"
                : "bg-muted/30 text-muted-foreground hover:text-foreground",
            )}
          >
            {t("All")}
          </button>
          {llmProfiles.map((p) => {
            const key = Number(p.id);
            const hasData = tokenByProfile.has(key) || (p.is_default && tokenByProfile.has(defaultProfileId));
            const resolvedKey = tokenByProfile.has(key) ? key : p.is_default ? defaultProfileId : key;
            return (
              <button
                type="button"
                key={p.id}
                onClick={() => setTokenTab(resolvedKey)}
                className={cn(
                  "flex items-center gap-1 rounded-md px-2.5 py-1 text-[10px] font-medium transition-colors",
                  tokenTab === resolvedKey
                    ? "bg-foreground text-background"
                    : "bg-muted/30 text-muted-foreground hover:text-foreground",
                  !hasData && "opacity-40",
                )}
              >
                {p.name}
                {p.is_default && (
                  <span
                    className={cn(
                      "rounded px-1 py-0 text-[8px]",
                      tokenTab === resolvedKey
                        ? "bg-background/20 text-background"
                        : "bg-emerald-500/20 text-emerald-400",
                    )}
                  >
                    {t("Default")}
                  </span>
                )}
              </button>
            );
          })}
        </div>
      </div>

      {/* Body: metrics left + bar chart right */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[220px_1fr]">
        {/* Left: key metrics */}
        <div className="flex flex-col gap-4">
          {/* Total */}
          <div>
            <div className="text-[10px] text-muted-foreground">{t("Total (input+output)")}</div>
            <div className="mt-0.5 text-3xl font-bold tabular-nums tracking-tight">
              {fmtTokens(displayedTokens.input + displayedTokens.output) || "—"}
            </div>
            <div className="mt-0.5 text-[10px] text-muted-foreground">
              {t("{count} tasks", { count: displayedTokens.taskCount })}
            </div>
          </div>

          {/* Per-type bars */}
          <div className="space-y-3">
            {(() => {
              // input already includes cache; split into three non-overlapping segments: uncached input + cache hits + output = total.
              const total = displayedTokens.input + displayedTokens.output;
              return [
                {
                  label: t("Input (uncached)"),
                  value: displayedTokens.input - displayedTokens.cacheRead,
                  barColor: dailyTrendConfig.input.color!,
                  text: "text-blue-400",
                },
                {
                  label: t("Cache hits"),
                  value: displayedTokens.cacheRead,
                  barColor: dailyTrendConfig.cacheRead.color!,
                  text: "text-emerald-400",
                },
                {
                  label: t("Output"),
                  value: displayedTokens.output,
                  barColor: dailyTrendConfig.output.color!,
                  text: "text-violet-400",
                },
              ].map(({ label, value, barColor, text }) => {
                const pct = total > 0 ? (value / total) * 100 : 0;
                return (
                  <div key={label}>
                    <div className="mb-1 flex items-center justify-between text-[10px]">
                      <span className="text-muted-foreground">{label}</span>
                      <span className={cn("font-mono font-semibold tabular-nums", text)}>{fmtTokens(value)}</span>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full transition-all duration-500"
                        style={{ width: `${pct}%`, background: barColor }}
                      />
                    </div>
                  </div>
                );
              });
            })()}
          </div>

          {/* Cache hit rate */}
          {(() => {
            // input already includes cache → hit rate = cache hits / total input.
            const denominator = displayedTokens.input;
            const hitPct = denominator > 0 ? Math.round((displayedTokens.cacheRead / denominator) * 100) : 0;
            return (
              <div className="flex items-center justify-between rounded-lg border bg-muted/20 px-3 py-2 text-[10px]">
                <span className="text-muted-foreground">{t("Cache hit rate")}</span>
                <span className={cn("font-semibold tabular-nums", hitPct > 50 ? "text-emerald-400" : "text-amber-400")}>
                  {hitPct}%
                </span>
              </div>
            );
          })()}
        </div>

        {/* Right: daily bar chart */}
        <div className="flex min-h-0 flex-col">
          <div className="mb-2 flex items-center justify-between">
            <div className="flex gap-3 text-[9px] text-muted-foreground">
              {(["input", "output", "cacheRead"] as const).map((k) => (
                <span key={k} className="flex items-center gap-1">
                  <span className="inline-block size-2 rounded-sm" style={{ background: dailyTrendConfig[k].color }} />
                  {dailyTrendConfig[k].label}
                </span>
              ))}
            </div>
            <div className="flex gap-0.5 rounded-md border bg-muted/30 p-0.5">
              {(
                [
                  { days: 7, label: t("7d") },
                  { days: 30, label: t("30d") },
                  { days: 90, label: t("3mo") },
                  { days: 180, label: t("6mo") },
                  { days: 365, label: t("1y") },
                ] as const
              ).map(({ days, label }) => (
                <button
                  type="button"
                  key={days}
                  onClick={() => setTokenDays(days)}
                  className={cn(
                    "rounded px-2.5 py-0.5 text-[9px] font-medium transition-colors",
                    tokenDays === days
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          {dailyTokenData.length === 0 ? (
            <div
              className="flex flex-1 items-center justify-center rounded-lg border bg-muted/10 text-xs text-muted-foreground"
              style={{ minHeight: 180 }}
            >
              {t("No data")}
            </div>
          ) : (
            <ChartContainer config={dailyTrendConfig} className="h-[200px] w-full">
              <BarChart data={dailyTokenData} margin={{ top: 4, right: 4, bottom: 0, left: 0 }} maxBarSize={40}>
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey="date"
                  tickLine={false}
                  axisLine={false}
                  tick={{ fontSize: 10 }}
                  tickMargin={6}
                  interval="preserveStartEnd"
                />
                <YAxis
                  tickFormatter={(v) => fmtTokens(Number(v))}
                  tickLine={false}
                  axisLine={false}
                  tick={{ fontSize: 10 }}
                  width={44}
                />
                <ChartTooltip
                  cursor={false}
                  content={
                    <ChartTooltipContent
                      indicator="line"
                      formatter={(value, name) => (
                        <div className="flex w-full items-center justify-between gap-4">
                          <span>
                            {(dailyTrendConfig as Record<string, { label: string }>)[String(name)]?.label ??
                              String(name)}
                          </span>
                          <span className="font-mono font-semibold tabular-nums">{fmtTokens(Number(value))}</span>
                        </div>
                      )}
                    />
                  }
                />
                <Bar dataKey="input" stackId="1" fill="var(--color-input)" radius={[0, 0, 0, 0]} />
                <Bar dataKey="output" stackId="1" fill="var(--color-output)" radius={[0, 0, 0, 0]} />
                <Bar dataKey="cacheRead" stackId="1" fill="var(--color-cacheRead)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartContainer>
          )}
        </div>
      </div>
    </Card>
  );
}
