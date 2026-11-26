"use client";

import * as React from "react";

import { RefreshCwIcon } from "lucide-react";

import { t } from "@/i18n";
import { cn } from "@/lib/utils";

import { fmtRel } from "./format";

export function LiveDot({ className }: { className?: string }) {
  return <span className={cn("inline-block size-1.5 shrink-0 animate-pulse rounded-full bg-blue-400", className)} />;
}

export function SectionTitle({
  icon: Icon,
  children,
  sub,
}: {
  icon: React.ElementType;
  children: React.ReactNode;
  sub?: React.ReactNode;
}) {
  return (
    <div className="mb-3 flex items-center justify-between">
      <div className="flex items-center gap-1.5 text-xs font-semibold">
        <Icon className="size-3.5 text-muted-foreground" />
        {children}
      </div>
      {sub && <span className="text-[10px] text-muted-foreground">{sub}</span>}
    </div>
  );
}

// Visible liveness: pulsing dot while the tab is visible (polling), "Paused"
// while hidden, relative "updated" stamp that ticks every second, and a manual
// refresh button.
export function LiveStatus({ lastUpdated, onRefresh }: { lastUpdated: number | null; onRefresh: () => void }) {
  const [, force] = React.useReducer((n: number) => n + 1, 0);
  const [hidden, setHidden] = React.useState(false);
  React.useEffect(() => {
    const id = setInterval(force, 1000);
    const onVis = () => setHidden(document.visibilityState === "hidden");
    onVis();
    document.addEventListener("visibilitychange", onVis);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, []);
  return (
    <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
      {hidden ? <span>{t("Paused")}</span> : <LiveDot />}
      <span>{!hidden && t("Live")}</span>
      <span className="tabular-nums">
        {t("Updated {time}", { time: lastUpdated ? fmtRel(lastUpdated / 1000) : "—" })}
      </span>
      <button
        type="button"
        onClick={onRefresh}
        title={t("Refresh")}
        aria-label={t("Refresh")}
        className="rounded p-1 hover:bg-muted hover:text-foreground"
      >
        <RefreshCwIcon className="size-3" />
      </button>
    </div>
  );
}
