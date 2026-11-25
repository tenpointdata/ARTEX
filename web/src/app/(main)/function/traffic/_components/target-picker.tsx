"use client";

import * as React from "react";

import { ArrowDownWideNarrowIcon, ArrowUpNarrowWideIcon, ListChecksIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { t } from "@/i18n";
import type { TrafficHost } from "@/lib/types";

type CountDirection = "asc" | "desc";

interface TargetPickerProps {
  hosts: TrafficHost[];
  selectedHosts: string[];
  onSelectedHostsChange: (update: (prev: string[]) => string[]) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleteSelected: () => void;
}

// Batch-delete target picker: hosts with packet counts, sortable by count.
export function TargetPicker({
  hosts,
  selectedHosts,
  onSelectedHostsChange,
  open,
  onOpenChange,
  onDeleteSelected,
}: TargetPickerProps) {
  const [direction, setDirection] = React.useState<CountDirection>("desc");
  const allSelected = hosts.length > 0 && hosts.every((h) => selectedHosts.includes(h.host));
  const sortedHosts = React.useMemo(
    () =>
      [...hosts].sort((a, b) => {
        const countOrder = direction === "asc" ? a.count - b.count : b.count - a.count;
        return countOrder || a.host.localeCompare(b.host);
      }),
    [hosts, direction],
  );

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          <ListChecksIcon className="size-3.5" />
          {selectedHosts.length > 0
            ? t("Select targets ({count})", { count: selectedHosts.length })
            : t("Select targets…")}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[calc(100vw-2rem)] p-0 data-open:animate-none data-closed:animate-none sm:w-80"
        align="start"
        collisionPadding={16}
      >
        <div className="flex items-center justify-between border-b px-3 py-2">
          <span className="text-xs font-medium text-muted-foreground">{t("Batch delete by target")}</span>
          <div className="flex items-center gap-1">
            {hosts.length > 0 && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => setDirection((current) => (current === "desc" ? "asc" : "desc"))}
                    aria-label={
                      direction === "desc"
                        ? t("Packet count is currently descending; click to switch to ascending")
                        : t("Packet count is currently ascending; click to switch to descending")
                    }
                  >
                    {direction === "desc" ? <ArrowDownWideNarrowIcon /> : <ArrowUpNarrowWideIcon />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {direction === "desc" ? t("By packet count, descending") : t("By packet count, ascending")}
                </TooltipContent>
              </Tooltip>
            )}
            {hosts.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={() => onSelectedHostsChange(() => (allSelected ? [] : hosts.map((h) => h.host)))}
              >
                {allSelected ? t("Clear all") : t("Select all")}
              </Button>
            )}
          </div>
        </div>
        <div className="max-h-64 overflow-y-auto">
          {hosts.length === 0 ? (
            <div className="px-3 py-6 text-center text-xs text-muted-foreground">{t("No traffic records")}</div>
          ) : (
            sortedHosts.map((h, index) => (
              <label
                key={h.host}
                htmlFor={`traffic-host-${index}`}
                className="flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs hover:bg-accent"
              >
                <Checkbox
                  id={`traffic-host-${index}`}
                  checked={selectedHosts.includes(h.host)}
                  onCheckedChange={() =>
                    onSelectedHostsChange((prev) =>
                      prev.includes(h.host) ? prev.filter((x) => x !== h.host) : [...prev, h.host],
                    )
                  }
                />
                <span className="truncate font-mono">{h.host}</span>
                <span className="ml-auto shrink-0 tabular-nums text-muted-foreground">{h.count}</span>
              </label>
            ))
          )}
        </div>
        <div className="border-t p-2">
          <Button
            variant="destructive"
            size="sm"
            className="w-full"
            disabled={selectedHosts.length === 0}
            onClick={onDeleteSelected}
          >
            {t("Delete selected ({count})", { count: selectedHosts.length })}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
