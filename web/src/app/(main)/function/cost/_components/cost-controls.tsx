"use client";

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { t } from "@/i18n";
import type { CostGroupBy } from "@/lib/types";

export const COST_GROUPS: { value: CostGroupBy; label: () => string }[] = [
  { value: "task", label: () => t("Task") },
  { value: "profile", label: () => t("Provider") },
  { value: "worker", label: () => t("Role") },
  { value: "model", label: () => t("Model") },
];

export const COST_RANGES: { days: number; label: () => string }[] = [
  { days: 7, label: () => t("7d") },
  { days: 30, label: () => t("30d") },
  { days: 90, label: () => t("90d") },
  { days: 0, label: () => t("All") },
];

export function CostControls({
  by,
  days,
  onBy,
  onDays,
}: {
  by: CostGroupBy;
  days: number;
  onBy: (v: CostGroupBy) => void;
  onDays: (v: number) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <ToggleGroup
        type="single"
        variant="outline"
        value={by}
        onValueChange={(v) => v && onBy(v as CostGroupBy)}
        aria-label={t("Group by")}
      >
        {COST_GROUPS.map((g) => (
          <ToggleGroupItem key={g.value} value={g.value}>
            {g.label()}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <ToggleGroup
        type="single"
        variant="outline"
        value={String(days)}
        onValueChange={(v) => v && onDays(Number(v))}
        aria-label={t("Range")}
      >
        {COST_RANGES.map((r) => (
          <ToggleGroupItem key={r.days} value={String(r.days)}>
            {r.label()}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
