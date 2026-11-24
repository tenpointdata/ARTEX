"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";
import { statusMeta } from "@/lib/status";
import type { FindingReportState, FindingStatus } from "@/lib/types";

import { FINDING_REPORT_STATES, FINDING_STATUSES, REPORT_STATE_META } from "./findings-table";

// BulkTriageControls sets the status / report stage of the selected findings
// (POST bulk-status / bulk-report-state, which need the review permission).
export function BulkTriageControls({
  count,
  busy,
  onStatus,
  onReportState,
}: {
  count: number;
  busy: boolean;
  onStatus: (status: FindingStatus) => void;
  onReportState: (state: FindingReportState) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs text-muted-foreground tabular-nums">{t("{count} selected", { count: count })}</span>
      <Select
        value=""
        onValueChange={(v) => {
          if (v) onStatus(v as FindingStatus);
        }}
        disabled={busy}
      >
        <SelectTrigger size="sm" className="w-40" aria-label={t("Set the status of the selected findings")}>
          <SelectValue placeholder={busy ? t("Updating…") : t("Set status")} />
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
          if (v) onReportState(v as FindingReportState);
        }}
        disabled={busy}
      >
        <SelectTrigger size="sm" className="w-44" aria-label={t("Set the report stage of the selected findings")}>
          <SelectValue placeholder={busy ? t("Updating…") : t("Set report stage")} />
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
  );
}
