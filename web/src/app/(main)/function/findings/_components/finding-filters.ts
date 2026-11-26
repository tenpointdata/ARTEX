// The findings list's filter set: one typed value shared by the filter bar, the stat tiles, the
// data loaders and the export dialog, plus where it is restored from (saved preferences, then
// the URL, so a link such as /function/findings?status=pending lands on that slice).

import type { FindingQuery, FindingReportState, FindingStatus, Severity, VerificationStatus } from "@/lib/types";

import { FINDING_REPORT_STATES, FINDING_STATUSES, SEVERITIES, VERIFICATION_STATUSES } from "./finding-meta";

// List views: flat = one big cross-task table (default); grouped = collapsible groups by task;
// asset = left asset tree + the findings under that subtree on the right.
export type FindingView = "flat" | "grouped" | "asset";
export const FINDING_VIEWS: FindingView[] = ["flat", "grouped", "asset"];

type FilterFields = "severity" | "status" | "reportState" | "verificationStatus" | "vulnclass" | "task" | "sort";

/** The list filters as the API takes them, every field present; query is the debounced search text. */
export type FindingFilters = Required<Pick<FindingQuery, FilterFields>> & { query: string };

export const DEFAULT_FINDING_FILTERS: FindingFilters = {
  severity: "all",
  status: "all",
  reportState: "all",
  verificationStatus: "all",
  vulnclass: "all",
  task: "all",
  sort: "severity",
  query: "",
};

/** The fields saved in localStorage and accepted from the URL (search text is neither). */
export type SavedFindingFilters = Partial<Omit<FindingFilters, "query">> & { view?: FindingView };

function oneOf<T extends string>(values: readonly T[], value: unknown): value is T {
  return typeof value === "string" && (values as readonly string[]).includes(value);
}

// readSavedFilters validates an untrusted record field by field; anything unknown is dropped.
function readSavedFilters(source: Record<string, unknown>): SavedFindingFilters {
  const out: SavedFindingFilters = {};
  if (oneOf(FINDING_VIEWS, source.view)) out.view = source.view;
  if (source.severity === "all" || oneOf<Severity>(SEVERITIES, source.severity)) out.severity = source.severity;
  if (source.status === "all" || oneOf<FindingStatus>(FINDING_STATUSES, source.status)) out.status = source.status;
  if (source.reportState === "all" || oneOf<FindingReportState>(FINDING_REPORT_STATES, source.reportState)) {
    out.reportState = source.reportState;
  }
  if (
    source.verificationStatus === "all" ||
    oneOf<VerificationStatus>(VERIFICATION_STATUSES, source.verificationStatus)
  ) {
    out.verificationStatus = source.verificationStatus;
  }
  if (typeof source.vulnclass === "string" && source.vulnclass) out.vulnclass = source.vulnclass;
  if (typeof source.task === "string" && source.task) out.task = source.task;
  if (source.sort === "severity" || source.sort === "time") out.sort = source.sort;
  return out;
}

/** Saved preferences (JSON); malformed or legacy values keep the defaults. */
export function parseSavedFilters(raw: string | null): SavedFindingFilters {
  if (!raw) return {};
  try {
    const parsed: unknown = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? readSavedFilters(parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

/** URL query parameters (?status=pending&severity=critical&task=12&view=flat). */
export function filtersFromSearch(search: string): SavedFindingFilters {
  return readSavedFilters(Object.fromEntries(new URLSearchParams(search)));
}

export function serializeFilters(view: FindingView, filters: FindingFilters): string {
  const { query: _query, ...saved } = filters;
  return JSON.stringify({ view, ...saved });
}
