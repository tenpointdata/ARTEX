"use client";

import type * as React from "react";

import {
  BugIcon,
  CircleCheckIcon,
  ClockIcon,
  FileUpIcon,
  InfoIcon,
  ShieldAlertIcon,
  TriangleAlertIcon,
} from "lucide-react";

import { t } from "@/i18n";
import type { FindingStats } from "@/lib/types";
import { cn } from "@/lib/utils";

import { DEFAULT_FINDING_FILTERS, type FindingFilters } from "./finding-filters";

interface StatTile {
  label: string;
  value: number;
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean | "true" }>;
  tone?: string;
  /** The filter slice the tile stands for; clicking applies exactly that slice. */
  slice: Partial<FindingFilters>;
}

interface FindingStatTilesProps {
  stats: FindingStats;
  filters: FindingFilters;
  /** Applies a slice (the rest of the filters reset, search text kept). */
  onSlice: (slice: Partial<FindingFilters>) => void;
}

function isActive(filters: FindingFilters, slice: Partial<FindingFilters>): boolean {
  const target: FindingFilters = { ...DEFAULT_FINDING_FILTERS, query: filters.query, sort: filters.sort, ...slice };
  return (Object.keys(target) as (keyof FindingFilters)[]).every((k) => target[k] === filters[k]);
}

// FindingStatTiles are the whole-table counts; each is a one-click filter onto the list it counts
// (Pending also starts triage on the first pending finding, see the page).
export function FindingStatTiles({ stats, filters, onSlice }: FindingStatTilesProps) {
  const tiles: StatTile[] = [
    { label: t("Total findings"), value: stats.total, icon: BugIcon, slice: {} },
    {
      label: t("Pending"),
      value: stats.pending,
      tone: "text-amber-500",
      icon: ClockIcon,
      slice: { status: "pending" },
    },
    {
      label: t("Submitted"),
      value: stats.submitted,
      tone: "text-amber-500",
      icon: FileUpIcon,
      slice: { reportState: "submitted" },
    },
    {
      label: t("Accepted"),
      value: stats.accepted,
      tone: "text-emerald-500",
      icon: CircleCheckIcon,
      slice: { reportState: "accepted" },
    },
    {
      label: t("Critical"),
      value: stats.critical,
      tone: "text-rose-600",
      icon: ShieldAlertIcon,
      slice: { severity: "critical" },
    },
    { label: t("High"), value: stats.high, tone: "text-red-500", icon: TriangleAlertIcon, slice: { severity: "high" } },
    {
      label: t("Medium"),
      value: stats.medium,
      tone: "text-amber-500",
      icon: TriangleAlertIcon,
      slice: { severity: "medium" },
    },
    { label: t("Low"), value: stats.low, tone: "text-slate-500", icon: InfoIcon, slice: { severity: "low" } },
  ];
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 lg:grid-cols-8">
      {tiles.map((tile) => {
        const Icon = tile.icon;
        const active = isActive(filters, tile.slice);
        return (
          <button
            key={tile.label}
            type="button"
            aria-pressed={active}
            title={t("Show these findings")}
            onClick={() => onSlice(tile.slice)}
            className={cn(
              "flex flex-col gap-1 rounded-xl border bg-card px-4 py-4 text-left shadow-xs transition-colors hover:bg-accent/50",
              active && "border-primary ring-1 ring-primary",
            )}
          >
            <span className="text-sm text-muted-foreground">{tile.label}</span>
            <span className={cn("flex items-center gap-2 font-semibold text-2xl tabular-nums", tile.tone)}>
              <Icon className="size-5" aria-hidden="true" />
              {tile.value}
            </span>
          </button>
        );
      })}
    </div>
  );
}
