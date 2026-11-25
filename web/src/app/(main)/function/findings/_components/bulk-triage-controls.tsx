"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { statusMeta } from "@/lib/status";
import type { FindingReportState, FindingStatus } from "@/lib/types";

import { FINDING_REPORT_STATES, FINDING_STATUSES, REPORT_STATE_META } from "./finding-meta";

// BulkTriageControls sets the status / report stage of the selected findings
// (POST bulk-status / bulk-report-state need the review permission, so it renders
// nothing without it).
export function BulkTriageControls({
  busy,
  onStatus,
  onReportState,
}: {
  busy: boolean;
  onStatus: (status: FindingStatus) => void;
  onReportState: (state: FindingReportState) => void;
}) {
  const canReview = useCan("review");
  if (!canReview) return null;
  return (
    <>
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
    </>
  );
}
