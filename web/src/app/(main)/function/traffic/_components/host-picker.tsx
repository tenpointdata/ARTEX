"use client";

import * as React from "react";

import { ArrowDownWideNarrowIcon, ArrowUpNarrowWideIcon, ListChecksIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { t } from "@/i18n";
import type { TrafficHost } from "@/lib/types";

type HostCountSortDirection = "asc" | "desc";

// HostPicker is the multi-select target list that feeds the batch "delete by
// target" action. The caller owns the selection and the delete confirmation.
export function HostPicker({
  hosts,
  selectedHosts,
  setSelectedHosts,
  open,
  onOpenChange,
  onDeleteSelected,
}: {
  hosts: TrafficHost[];
  selectedHosts: string[];
  setSelectedHosts: React.Dispatch<React.SetStateAction<string[]>>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleteSelected: () => void;
}) {
  const [hostCountSortDirection, setHostCountSortDirection] = React.useState<HostCountSortDirection>("desc");
  const allSelected = hosts.length > 0 && hosts.every((h) => selectedHosts.includes(h.host));
  const sortedHosts = React.useMemo(
    () =>
      [...hosts].sort((a, b) => {
        const countOrder = hostCountSortDirection === "asc" ? a.count - b.count : b.count - a.count;
        return countOrder || a.host.localeCompare(b.host);
      }),
    [hosts, hostCountSortDirection],
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
                    onClick={() => setHostCountSortDirection((current) => (current === "desc" ? "asc" : "desc"))}
                    aria-label={
                      hostCountSortDirection === "desc"
                        ? t("Packet count is currently descending; click to switch to ascending")
                        : t("Packet count is currently ascending; click to switch to descending")
                    }
                  >
                    {hostCountSortDirection === "desc" ? <ArrowDownWideNarrowIcon /> : <ArrowUpNarrowWideIcon />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {hostCountSortDirection === "desc"
                    ? t("By packet count, descending")
                    : t("By packet count, ascending")}
                </TooltipContent>
              </Tooltip>
            )}
            {hosts.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={() => setSelectedHosts(allSelected ? [] : hosts.map((h) => h.host))}
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
                    setSelectedHosts((prev) =>
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
            onClick={() => {
              onDeleteSelected();
              onOpenChange(false);
            }}
          >
            {t("Delete selected ({count})", { count: selectedHosts.length })}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
