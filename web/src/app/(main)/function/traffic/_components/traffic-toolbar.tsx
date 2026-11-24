"use client";

import type { ReactNode } from "react";

import { ChevronLeftIcon, ChevronRightIcon, EraserIcon, SearchIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";

import { METHODS, PAGE_SIZES } from "./traffic-format";
import type { TrafficFilters } from "./use-traffic-filters";

interface TrafficToolbarProps {
  f: TrafficFilters;
  /** Target picker (rendered first in the row). */
  picker: ReactNode;
  size: number;
  onSizeChange: (size: number) => void;
  page: number;
  pageCount: number;
  rangeStart: number;
  rangeEnd: number;
  total: number;
  onPageChange: (update: (page: number) => number) => void;
  recordCount: number;
  deleting: boolean;
  onDeleteTarget: () => void;
  onPurgeAll: () => void;
}

export function TrafficToolbar({
  f,
  picker,
  size,
  onSizeChange,
  page,
  pageCount,
  rangeStart,
  rangeEnd,
  total,
  onPageChange,
  recordCount,
  deleting,
  onDeleteTarget,
  onPurgeAll,
}: TrafficToolbarProps) {
  const hostQ = f.host.debounced;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {picker}
      <div className="relative w-48">
        <Input
          placeholder="host…"
          value={f.host.value}
          onChange={(e) => f.host.setValue(e.target.value)}
          className="h-8"
        />
      </div>
      <Button
        variant="destructive"
        size="sm"
        className="h-8"
        disabled={!hostQ || deleting}
        title={hostQ ? undefined : t("Select a target on the left or enter a host first")}
        onClick={onDeleteTarget}
      >
        <Trash2Icon className="size-3.5" />
        {t("Delete this target")}
      </Button>
      {/* Outline rather than a second destructive button: this one ignores every
          filter, so it must not look one mis-click away from "Delete this target". */}
      <Button
        variant="outline"
        size="sm"
        className="h-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
        disabled={!recordCount || deleting}
        title={recordCount ? t("Delete all traffic and compact storage") : t("No traffic records yet")}
        onClick={onPurgeAll}
      >
        <EraserIcon className="size-3.5" />
        {t("Purge all")}
      </Button>
      <div className="relative max-w-sm flex-1">
        <SearchIcon className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder={t("Search everything (URL / method / type / status…)")}
          value={f.search.value}
          onChange={(e) => f.search.setValue(e.target.value)}
          className="h-8 pl-8"
        />
      </div>
      <Select value={f.method} onValueChange={f.setMethod}>
        <SelectTrigger size="sm" className="w-32">
          <SelectValue placeholder={t("Method")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All methods")}</SelectItem>
          {METHODS.map((m) => (
            <SelectItem key={m} value={m}>
              {m}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={String(size)} onValueChange={(v) => onSizeChange(Number(v))}>
        <SelectTrigger size="sm" className="w-28">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PAGE_SIZES.map((n) => (
            <SelectItem key={n} value={String(n)}>
              {t("{count} / page", { count: n })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
        <span className="tabular-nums">
          {rangeStart}–{rangeEnd} / {total}
        </span>
        <Button
          variant="outline"
          size="icon"
          className="size-8"
          disabled={page <= 0}
          onClick={() => onPageChange((p) => Math.max(0, p - 1))}
        >
          <ChevronLeftIcon />
        </Button>
        <span className="tabular-nums">
          {page + 1} / {pageCount}
        </span>
        <Button
          variant="outline"
          size="icon"
          className="size-8"
          disabled={page + 1 >= pageCount}
          onClick={() => onPageChange((p) => Math.min(pageCount - 1, p + 1))}
        >
          <ChevronRightIcon />
        </Button>
      </div>
    </div>
  );
}
