// ---- E2 integrations: ticket trackers (findings out) and scanner imports (data in) ----
// Mirrors server/ticket_api.go, server/ticket_sync.go, server/scan_import_api.go and db/scan_imports.go.

import type { FindingLane } from "./findings";

// ConfigValues is an adapter config as JSON: credential keys come back masked ("__masked__…tail").
export type ConfigValues = Record<string, unknown>;

export interface TicketTrackerKindMeta {
  kind: string;
  secret_keys: string[];
  // Keys deciding where credentials are sent; changing one requires re-entering every secret.
  destination_keys: string[];
}

export interface TicketTrackerMeta {
  kinds: TicketTrackerKindMeta[];
  // Server-side keys any tracker config may carry next to the adapter's own (include_evidence).
  option_keys: string[];
}

export interface TicketTracker {
  id: number;
  name: string;
  kind: string;
  enabled: boolean;
  config: ConfigValues;
  secret_keys: string[];
  created_at: string;
  updated_at: string;
}

export interface TicketTrackerInput {
  name?: string;
  kind?: string;
  enabled?: boolean;
  config?: ConfigValues;
}

export interface FindingTicket {
  id: number;
  finding_id: number;
  tracker_id: number;
  dedup_key: string;
  external_id: string;
  external_url: string;
  synced_status: string;
  remote_open: boolean;
  remote_resolution: string;
  last_error: string;
  synced_at: string;
  created_at: string;
}

export interface TicketPushResult {
  finding_id: number;
  ticket?: FindingTicket;
  created: boolean;
  error?: string;
}

export type TicketSyncAction = "unchanged" | "pulled" | "pushed" | "error";

export interface TicketSyncResult {
  finding_id: number;
  external_id: string;
  action: TicketSyncAction;
  from?: string;
  to?: string;
  error?: string;
}

export type ScanImportFormat = "nmap-xml" | "nuclei-jsonl" | "zap-json" | "burp-xml" | "sarif";

export interface ScanImport {
  id: number;
  format: ScanImportFormat;
  tool_name: string;
  tool_version: string;
  file_name: string;
  file_sha256: string;
  task_id?: number;
  fact_node_id?: number; // the task's exploration-graph fact node that records this import
  anchor_asset_ids?: number[]; // assets the import's findings were anchored to
  lane_counts?: Partial<Record<FindingLane, number>>; // findings this import created, per lane
  imported_by: string;
  records: number;
  assets: number;
  findings_new: number;
  findings_existing: number;
  skipped: number;
  informational: number;
  truncated: boolean;
  created_at: string;
}

export interface ScanImportOutcome {
  import: ScanImport;
  new_findings: number[];
  asset_errors?: string[];
  finding_errors?: string[];
}

export interface FindingImportSource {
  finding_id: number;
  import_id?: number;
  tool: string;
  lane?: FindingLane;
  location?: string;
  line?: number;
  rule_id: string;
  dedup_key: string;
  record: string;
  created_at: string;
}

export interface FindingIntegrations {
  tickets: FindingTicket[];
  import_source: FindingImportSource | null;
}
