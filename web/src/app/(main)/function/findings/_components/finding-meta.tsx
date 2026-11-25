"use client";

// Finding vocabularies (statuses, report stages, verification outcomes), their badges and the
// row-identity helpers, shared by the findings list, its triage bar and the finding detail page.

import { intlLocale, t } from "@/i18n";
import { statusMeta, type Tone, toneClasses } from "@/lib/status";
import type { Finding, FindingReportState, FindingStatus, Severity, VerificationStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

export const SEVERITIES: Severity[] = ["critical", "high", "medium", "low"];

export const FINDING_STATUSES: FindingStatus[] = [
  "pending",
  "in_progress",
  "confirmed",
  "resolved",
  "fixed",
  "false_positive",
  "ignored",
  "duplicate",
  "risk_accepted",
];

export const VERIFICATION_STATUSES: VerificationStatus[] = [
  "verified",
  "unverified",
  "blocked",
  "not_assessed",
  "false_positive",
];

export const FINDING_REPORT_STATES: FindingReportState[] = ["none", "submitted", "accepted", "rejected", "duplicate"];

export const REPORT_STATE_META: Record<FindingReportState, { label: string; tone: Tone }> = {
  none: { label: t("Not submitted"), tone: "slate" },
  submitted: { label: t("Submitted"), tone: "amber" },
  accepted: { label: t("Accepted"), tone: "green" },
  rejected: { label: t("Rejected"), tone: "red" },
  duplicate: { label: t("Duplicate report"), tone: "neutral" },
};

export function ReportStateBadge({ state }: { state?: FindingReportState }) {
  const value = state ?? "none";
  const meta = REPORT_STATE_META[value] ?? REPORT_STATE_META.none;
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center whitespace-nowrap rounded-md border px-2 py-0.5 font-medium text-xs",
        toneClasses[meta.tone],
      )}
    >
      {meta.label}
    </span>
  );
}

// Verification outcome → label/tone, on the same tone tokens StatusBadge uses.
export const VERIFICATION_META: Record<VerificationStatus, { label: string; tone: Tone }> = {
  verified: { label: t("Verified"), tone: "green" },
  unverified: { label: t("Unverified"), tone: "slate" },
  blocked: { label: t("Blocked"), tone: "amber" },
  not_assessed: { label: t("Not assessed"), tone: "red" },
  false_positive: { label: t("False positive"), tone: "neutral" },
};

// VerificationBadge shows the automated verification outcome ("Verified L3" when a
// proof-of-exploitation level is declared). It renders nothing when the finding carries
// no verification status (in-task nodes, payloads from before the column existed).
export function VerificationBadge({ finding }: { finding: Pick<Finding, "verification_status" | "poe_level"> }) {
  const meta = finding.verification_status ? VERIFICATION_META[finding.verification_status] : undefined;
  if (!meta) return null;
  const label =
    finding.verification_status === "verified" && finding.poe_level
      ? `${meta.label} L${finding.poe_level}`
      : meta.label;
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center whitespace-nowrap rounded-md border px-2 py-0.5 font-medium text-xs",
        toneClasses[meta.tone],
      )}
    >
      {label}
    </span>
  );
}

export const UNASSIGNED_TASK = "__unassigned__";

// Inline edit buffer: the expanded row's name/category/severity.
export interface FindingEdit {
  name: string;
  vulnclass: string;
  severity: Severity;
}

export type FindingReport = { status: "loading" | "done" | "error"; text: string };

// Exploration node ids are only unique inside a task. Prefer the persisted
// finding id and otherwise namespace the node id by task so editing one group
// cannot update a similarly-named node in another expanded group.
export function findingRowKey(finding: Finding): string {
  if (finding.finding_id) return `finding:${finding.finding_id}`;
  return `node:${finding.task_id ?? UNASSIGNED_TASK}:${finding.id}`;
}

export function isSameFinding(left: Finding, right: Finding): boolean {
  return findingRowKey(left) === findingRowKey(right);
}

export function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function findingStatusLabel(status: FindingStatus): string {
  return statusMeta("finding", status).label;
}
