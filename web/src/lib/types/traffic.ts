// ---- Security ----
export interface AuditEntry {
  ts: string;
  tool: string;
  action: "allow" | "block";
  reason?: string;
  command?: string;
}

export interface Audit {
  entries?: AuditEntry[];
  attributions?: Record<string, number>;
}

// ---- Traffic ----
export interface TrafficExchange {
  id: string;
  ts: string;
  host: string;
  method: string;
  url: string;
  status: number;
  content_type: string;
  resp_len: number;
  rewritten?: boolean;
}

export interface TrafficRewriteStats {
  large_responses: number;
  capacity_refusals: number;
  tombstone_drops: number;
  conversions: number;
  sweeps: number;
}

export interface TrafficHARImportError {
  index: number;
  url?: string;
  reason: string;
}

export interface TrafficHARImportResult {
  imported: number;
  skipped: number;
  failed: number;
  truncated?: boolean;
  errors?: TrafficHARImportError[];
}

export interface TrafficHARExportMeta {
  exported: number;
  matched: number;
  truncated: boolean;
  truncatedReason?: string;
}

export interface TrafficResp {
  enabled: boolean;
  proxy?: string;
  count?: number; // global total (unfiltered)
  total?: number; // rows matching the current filter (for pagination)
  page?: number;
  size?: number;
  exchanges?: TrafficExchange[];
  rewrite?: TrafficRewriteStats;
}

// Full raw request/response of one exchange (lazy-loaded on row select).
export interface TrafficDetail {
  req: string;
  resp: string;
  // WebSocket/SSE exchanges carry their messages separately (trafficStream);
  // stream_kind is "" for an ordinary exchange.
  stream_count?: number;
  stream_kind?: "websocket" | "sse" | "";
}

// One recorded WebSocket message or SSE event. ts is in Unix seconds; seq
// orders. payload is a preview of at most 4 KiB (base64 when encoding is
// "base64"); blob names the full payload when it was stored separately.
export interface TrafficStreamMessage {
  seq: number;
  ts: number;
  direction: "c2s" | "s2c" | "meta" | (string & {});
  kind: number;
  type: "text" | "binary" | "event" | "meta" | (string & {});
  event?: string;
  event_id?: string;
  retry?: number;
  size: number;
  payload: string;
  encoding?: string;
  blob?: string;
  truncated?: boolean;
}

export type TrafficStreamDirection = "" | "c2s" | "s2c";

export interface TrafficStreamPage {
  items: TrafficStreamMessage[];
  // Pagination includes recording-limit rows; the message count does not.
  total: number;
  message_total: number;
}

export interface TrafficReplayHeader {
  name: string;
  value: string;
}

// POST /api/traffic/replay. Host is not sent: the server takes it from url.
// apply_rules defaults to false (bypasses capture rewrite rules).
export interface TrafficReplayRequest {
  method: string;
  url: string;
  headers: TrafficReplayHeader[];
  body: string;
  body_base64: boolean;
  apply_rules?: boolean;
}

export interface TrafficReplayResult {
  status: number;
  headers: TrafficReplayHeader[];
  raw: string;
  duration_ms: number;
  truncated: boolean;
  // false when the host is in passthrough: the response was not recorded.
  recorded: boolean;
}

// One distinct recorded host with its exchange count (target picker).
export interface TrafficHost {
  host: string;
  count: number;
}
