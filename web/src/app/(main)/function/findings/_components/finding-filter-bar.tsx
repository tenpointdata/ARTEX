"use client";

import { BugIcon, DownloadIcon, SearchIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { t } from "@/i18n";
import { statusMeta } from "@/lib/status";
import type { FindingReportState, FindingStats, FindingStatus, Severity, VerificationStatus } from "@/lib/types";

import type { FindingFilters } from "./finding-filters";
import {
  FINDING_REPORT_STATES,
  FINDING_STATUSES,
  REPORT_STATE_META,
  UNASSIGNED_TASK,
  VERIFICATION_META,
  VERIFICATION_STATUSES,
} from "./finding-meta";

interface FindingFilterBarProps {
  filters: FindingFilters;
  /** Applies a partial filter change at once (no confirm step). */
  set: (patch: Partial<FindingFilters>) => void;
  search: string;
  setSearch: (value: string) => void;
  stats: FindingStats;
  selectedIds: Set<string>;
  bulkBusy: boolean;
  onBulkStatus: (next: FindingStatus) => void;
  onBulkReportState: (next: FindingReportState) => void;
  onExport: () => void;
}

// FindingFilterBar is the findings list's search, filters, bulk actions on the selection and export.
export function FindingFilterBar({
  filters,
  set,
  search,
  setSearch,
  stats,
  selectedIds,
  bulkBusy,
  onBulkStatus,
  onBulkReportState,
  onExport,
}: FindingFilterBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <InputGroup className="w-full sm:w-72">
        <InputGroupInput
          type="search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t("Search findings")}
          aria-label={t("Search findings")}
        />
        <InputGroupAddon>
          <SearchIcon aria-hidden="true" />
        </InputGroupAddon>
      </InputGroup>

      <ToggleGroup
        type="single"
        value={filters.severity}
        onValueChange={(value) => value && set({ severity: value as "all" | Severity })}
        variant="outline"
        size="sm"
        spacing={0}
      >
        {(
          [
            ["all", t("All")],
            ["critical", t("Critical")],
            ["high", t("High")],
            ["medium", t("Medium")],
            ["low", t("Low")],
          ] as const
        ).map(([val, label]) => (
          <ToggleGroupItem key={val} value={val} aria-label={t("Filter by {label} severity", { label })}>
            {label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>

      <Select value={filters.status} onValueChange={(v) => set({ status: v as "all" | FindingStatus })}>
        <SelectTrigger size="sm" className="w-32">
          <SelectValue placeholder={t("Status")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All statuses")}</SelectItem>
          {FINDING_STATUSES.map((st) => (
            <SelectItem key={st} value={st}>
              {statusMeta("finding", st).label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={filters.reportState} onValueChange={(v) => set({ reportState: v as "all" | FindingReportState })}>
        <SelectTrigger size="sm" className="w-40">
          <SelectValue placeholder={t("Report stage")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All report stages")}</SelectItem>
          {FINDING_REPORT_STATES.map((st) => (
            <SelectItem key={st} value={st}>
              {REPORT_STATE_META[st].label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.verificationStatus}
        onValueChange={(v) => set({ verificationStatus: v as "all" | VerificationStatus })}
      >
        <SelectTrigger size="sm" className="w-48">
          <SelectValue placeholder={t("Verification")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All verification states")}</SelectItem>
          {VERIFICATION_STATUSES.map((st) => (
            <SelectItem key={st} value={st}>
              {VERIFICATION_META[st].label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={filters.vulnclass} onValueChange={(v) => set({ vulnclass: v })}>
        <SelectTrigger size="sm" className="w-40">
          <SelectValue placeholder={t("Vulnerability type")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All types")}</SelectItem>
          {stats.vulnclasses.map((vc) => (
            <SelectItem key={vc} value={vc}>
              {vc}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={filters.task} onValueChange={(v) => set({ task: v })}>
        <SelectTrigger size="sm" className="w-48">
          <SelectValue placeholder={t("Task")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("All tasks")}</SelectItem>
          <SelectItem value={UNASSIGNED_TASK}>{t("Unlinked / task deleted")}</SelectItem>
          {(stats.tasks ?? []).map((option) => {
            const id = String(option.id);
            const label = option.name || option.description || t("Task #{id} (deleted)", { id });
            return (
              <SelectItem key={id} value={id}>
                <span className="flex w-full items-center gap-2">
                  <span className="max-w-[14rem] truncate" title={label}>
                    {label}
                  </span>
                  <span className="inline-flex items-center gap-1 text-muted-foreground tabular-nums">
                    <BugIcon className="size-3.5" aria-hidden="true" />
                    {option.count}
                  </span>
                </span>
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>

      <Select value={filters.sort} onValueChange={(v) => set({ sort: v as "severity" | "time" })}>
        <SelectTrigger size="sm" className="w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="severity">{t("By severity")}</SelectItem>
          <SelectItem value="time">{t("By time")}</SelectItem>
        </SelectContent>
      </Select>

      <div className="ml-auto flex items-center gap-3">
        {selectedIds.size > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground tabular-nums">
              {t("{count} selected", { count: selectedIds.size })}
            </span>
            <Select
              value=""
              onValueChange={(v) => {
                if (v) onBulkStatus(v as FindingStatus);
              }}
              disabled={bulkBusy}
            >
              <SelectTrigger size="sm" className="w-40" aria-label={t("Set the status of the selected findings")}>
                <SelectValue placeholder={bulkBusy ? t("Updating…") : t("Set status")} />
              </SelectTrigger>
              <SelectContent>
                {FINDING_STATUSES.map((st) => (
                  <SelectItem key={st} value={st}>
                    {statusMeta("finding", st).label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value=""
              onValueChange={(v) => {
                if (v) onBulkReportState(v as FindingReportState);
              }}
              disabled={bulkBusy}
            >
              <SelectTrigger size="sm" className="w-44" aria-label={t("Set the report stage of the selected findings")}>
                <SelectValue placeholder={bulkBusy ? t("Updating…") : t("Set report stage")} />
              </SelectTrigger>
              <SelectContent>
                {FINDING_REPORT_STATES.map((st) => (
                  <SelectItem key={st} value={st}>
                    {REPORT_STATE_META[st].label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <Button size="sm" variant="outline" onClick={onExport}>
          <DownloadIcon /> {t("Export")}
        </Button>
      </div>
    </div>
  );
}
