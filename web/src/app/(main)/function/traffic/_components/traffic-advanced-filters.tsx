"use client";

import { FilterXIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";

import { STATUS_BUCKETS } from "./traffic-format";
import type { TrafficFilters } from "./use-traffic-filters";

// Advanced filters (issue #177): narrow 660k+ exchanges down to the one packet.
// Text inputs are debounced; selects apply immediately.
export function TrafficAdvancedFilters({ f }: { f: TrafficFilters }) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 px-2 py-1.5">
      <span className="pl-1 text-xs font-medium text-muted-foreground">{t("Advanced filters")}</span>
      <div className="relative w-56">
        <Input
          placeholder={t("Response content (body keyword, ≥3 chars)")}
          value={f.body.value}
          onChange={(e) => f.body.setValue(e.target.value)}
          className="h-8"
        />
      </div>
      <div className="relative w-52">
        <Input
          placeholder={t("Path (e.g. /api/user/…)")}
          value={f.path.value}
          onChange={(e) => f.path.setValue(e.target.value)}
          className="h-8"
        />
      </div>
      <Select value={f.status} onValueChange={f.setStatus}>
        <SelectTrigger size="sm" className="w-28">
          <SelectValue placeholder={t("Status")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All statuses")}</SelectItem>
          {STATUS_BUCKETS.map((s) => (
            <SelectItem key={s} value={s}>
              {s}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={f.rewritten} onValueChange={f.setRewritten}>
        <SelectTrigger size="sm" className="w-32">
          <SelectValue placeholder={t("Rewritten")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All traffic")}</SelectItem>
          <SelectItem value="rewritten">{t("Rewritten")}</SelectItem>
          <SelectItem value="original">{t("Original")}</SelectItem>
        </SelectContent>
      </Select>
      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <span>{t("Response size")}</span>
        <Input
          type="number"
          min={0}
          placeholder={t("Min (B)")}
          value={f.respMin.value}
          onChange={(e) => f.respMin.setValue(e.target.value)}
          className="h-8 w-24"
        />
        <span>–</span>
        <Input
          type="number"
          min={0}
          placeholder={t("Max (B)")}
          value={f.respMax.value}
          onChange={(e) => f.respMax.setValue(e.target.value)}
          className="h-8 w-24"
        />
      </div>
      {f.hasAdvancedFilter ? (
        <Button variant="ghost" size="sm" className="h-8" onClick={f.resetAdvanced}>
          <FilterXIcon className="size-3.5" />
          {t("Clear filters")}
        </Button>
      ) : null}
    </div>
  );
}
