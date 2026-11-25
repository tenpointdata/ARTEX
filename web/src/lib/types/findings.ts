import type { IntentState } from "./exploration";

// ---- Findings ----
export type Severity = "critical" | "high" | "medium" | "low";

// Vulnerability handling status: Pending / In progress / Confirmed / Resolved / Fixed / False positive / Ignored / Duplicate / Risk accepted.
export type FindingStatus =
  | "pending"
  | "in_progress"
  | "confirmed"
  | "resolved"
  | "fixed"
  | "false_positive"
  | "ignored"
  | "duplicate"
  | "risk_accepted";

// Verification status (S1): the automated proof-of-exploitation outcome, distinct from the
// human triage status above. "blocked" is produced by no path yet but is displayed when present.
export type VerificationStatus = "verified" | "unverified" | "blocked" | "not_assessed" | "false_positive";

// Report stage after triage: a finding can be confirmed and submitted at the same time.
export type FindingReportState = "none" | "submitted" | "accepted" | "rejected" | "duplicate";

// FindingAsset is an asset bound to a vulnerability (label pre-rendered by the backend).
export interface FindingAsset {
  id: string;
  type: string;
  label: string;
}

export interface Finding {
  traffic_count?: number;
  evidence_version?: number;
  report_evidence_version?: number;
  report_stale?: boolean;
  poe_level?: number; // proof-of-exploitation level (3 or 4 when verified); 0 = none declared
  verification_status?: VerificationStatus; // automated verification outcome (S1)
  verification_note?: string; // why the status was reached; empty when verified
  counterevidence?: string; // what was tested and did not work; optional, omitted when empty
  confidence_rationale?: string; // why the conclusion holds; optional, omitted when empty
  id: string;
  finding_id?: string; // row id in the standalone findings table, the handle for status updates (may be missing for old in-task nodes)
  vulnclass: string;
  name?: string; // vulnerability name; display falls back to vulnclass when empty
  severity: Severity;
  status: FindingStatus;
  report_state?: FindingReportState; // workflow stage after triage; default none
  summary: string;
  evidence: string;
  report?: string; // detailed report (Markdown); returned only by the detail endpoint, empty in lists
  intent_id?: string;
  param_id?: string;
  task_id?: string;
  task_description?: string;
  source_task_id?: string;
  inherited?: boolean;
  assets?: FindingAsset[];
  ts: string;
}

// FindingsPage is the server-side paginated response of the findings list.
export interface FindingsPage {
  items: Finding[];
  total: number;
  page: number;
  page_size: number;
}

export interface FindingGroup {
  task_id: string | number | null;
  task_name?: string; // optional task name; empty/absent = untitled
  task_description: string;
  task_status: string;
  count: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingGroupsPage {
  items: FindingGroup[];
  total: number;
  finding_total: number;
  page: number;
  page_size: number;
}

export interface FindingDeepenResponse {
  task_id: string;
  intent_id: string;
  state: IntentState;
  queued: boolean;
}

// FindingStats is the whole-table findings aggregate (stat cards + vulnerability type dropdown), computed server-side, unaffected by pagination.
export interface FindingStats {
  total: number;
  pending: number;
  submitted: number;
  accepted: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  vulnclasses: string[];
  tasks: FindingTaskOption[];
}

// FindingTaskOption is one entry of the findings page's "by task" filter dropdown: tasks
// with vulnerabilities (an empty description means the task was deleted; the frontend falls back to showing the id) and their vulnerability counts.
export interface FindingTaskOption {
  id: string | number;
  name?: string; // optional task name; empty/absent = untitled
  description: string;
  count: number;
}

// FindingQuery is the findings list pagination/filter/sort parameters.
export interface FindingQuery {
  page: number;
  pageSize: number;
  severity?: "all" | Severity;
  status?: "all" | FindingStatus;
  reportState?: "all" | FindingReportState;
  verificationStatus?: "all" | VerificationStatus;
  vulnclass?: string;
  task?: string; // task id; "all"/empty = no task filter
  query?: string;
  sort?: "severity" | "time";
  // asset tree node key; selecting a node selects its whole subtree. Empty = no asset filter.
  assetScope?: string;
  // company id from the sidebar switcher; also narrows the asset tree. Empty = all companies.
  company?: string;
  // scanner import id: only the findings that import created. Empty = no import filter.
  importId?: string;
}

// ---- Findings by asset (asset view) ----
export type FindingAssetKind = "company" | "root_domain" | "subdomain" | "ip" | "service" | "app" | "endpoint" | "none";

// FindingAssetNode is one node of the asset tree. keys look like a:<id> (asset), c:<id>
// (company), r:<domain> (root domain with no asset row), __none__ (no linked asset).
export interface FindingAssetNode {
  key: string;
  parent?: string;
  kind: FindingAssetKind;
  label: string;
  asset_id?: number;
  company_id?: number;
  self: number; // findings attached directly to this asset
  total: number; // including descendants, deduplicated by finding
  critical: number;
  high: number;
  medium: number;
  low: number;
  last_found_at: string;
}

export interface FindingAssetTree {
  nodes: FindingAssetNode[];
  finding_total: number;
  truncated: boolean;
  dropped_kinds?: string[];
}

// FINDING_UNASSIGNED_ASSET corresponds to the backend's db.FindingUnassignedAsset.
export const FINDING_UNASSIGNED_ASSET = "__none__";
