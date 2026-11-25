// ---- App settings (runtime toggles) ----
export interface Settings {
  traffic_capture: boolean;
  agent_traffic_binding: boolean; // agents auto-bind traffic evidence, off by default; does not affect manual binding
  llm_record: boolean; // LLM recording switch (off by default); when off no LLM calls are recorded
  // Web search. brave_key_set / tavily_key_set / keenable_key_set reflect whether a key is stored
  // (the values are never returned). On PUT, send the corresponding field to set/clear.
  web_search_enabled: boolean;
  web_search_backend: string; // "ddgs" | "brave-free" | "tavily" | "keenable" | "deepseek"
  brave_key_set: boolean;
  tavily_key_set: boolean;
  keenable_key_set: boolean;
  // write-only: only sent on PUT to store/clear the key.
  brave_search_api_key?: string;
  tavily_search_api_key?: string;
  keenable_search_api_key?: string;
  // Dedicated egress proxy (http/https/socks5) for reaching search endpoints; unrelated to the MITM proxy that records traffic. Empty = direct.
  web_search_proxy?: string;
  // Global egress proxy (http/https/socks5, may carry user:pass) that all target traffic goes
  // through. Acts as the MITM upstream when traffic capture is on; injected directly into the agent's bash/WebFetch when capture is off. Empty = direct.
  global_proxy?: string;
  python_interpreter?: string; // python interpreter path for custom script tools (empty = detect at runtime)
  workers?: number; // concurrent worker agent count (default 3); applies to tasks started afterwards
  // Task concurrency cap: max simultaneously "running" tasks. Off = unlimited; on, new tasks beyond the limit queue and start automatically when a slot frees up.
  task_concurrency_enabled?: boolean; // false by default
  task_concurrency_limit?: number; // defaults to 5 when enabled
  // LLM pooling (failover). Off by default; when on, an agent with "no model specified"
  // automatically switches to the next profile when the current one is unavailable (out of balance / dead key / rate limited / service error).
  llm_pool_enabled?: boolean; // false by default
  // Whether agents/tasks bound to a specific profile also fall back to the pool chain on failure. Default false = binding is exclusive.
  llm_pool_bind_fallback?: boolean;
  // Operation constraint injection scope (both on by default): splice the task's allow/deny constraints into the matching agent's system prompt.
  constraints_inject_planner?: boolean;
  constraints_inject_worker?: boolean;
  // Experimental: noa model-driven context compression (off by default). When on, the
  // platform's four agent kinds (planner/worker/main agent/chat) have context compression
  // taken over by noa, replacing built-in compaction; read once per run, applying to runs started afterwards.
  noa_compaction?: boolean;
  // ---- Vulnerability IM push (channels are a standalone resource, see /api/notify/*; only three global settings live here) ----
  notify_enabled?: boolean; // master push switch, on by default; one-click kill switch for maintenance windows
  notify_public_base_url?: string; // external base URL for vulnerability detail links; empty = messages carry no link
  notify_digest_interval_min?: number; // digest mode period (minutes), default 30
  // Render limits of IM messages; 0 = no truncation. The backend answers 400 outside the bounds.
  notify_summary_max_runes?: number; // summary characters per finding, 0-4000, default 120
  notify_assets_max_shown?: number; // asset names listed per finding, 0-50, default 3
  // Target noise profile; the stored setting wins over ARTEX_NOISE_PROFILE. Applies to the next request.
  noise_profile?: NoiseProfileName;
}

export type NoiseProfileName = "off" | "quiet" | "balanced" | "aggressive";

// GET /api/settings/noise: the current profile and its per-host counters (current minute, no reset).
export interface NoiseStatus {
  profile: NoiseProfileName;
  hosts: {
    host: string;
    rpm: number;
    peak_rpm: number;
    peak_inflight: number;
    inflight: number;
    duplicates_avoided: number;
  }[];
}

// ---- Vulnerability IM push ----

// NotificationFilter is a channel's filter conditions; all fields are optional, absent means no filtering.
// The backend validates none of them: a malformed config counts as "matched" (better to over-push than to miss).
export interface NotificationFilter {
  min_severity?: string; // "" | low | medium | high | critical
  task_ids?: number[]; // empty = no limit; non-empty requires overlap with the vulnerability's task
  asset_ids?: number[]; // empty = no limit; non-empty requires overlap with the vulnerability's anchored assets
  vulnclass_include?: string[]; // empty = accept all; non-empty requires the vulnerability type to match any keyword (case-insensitive substring)
  vulnclass_exclude?: string[]; // matching any keyword excludes (exclusion wins over inclusion)
  on_status_change?: boolean; // whether to also receive vulnerability status change events
}

// NotificationChannel is one channel instance. config fields vary by kind, and
// credential fields are replaced on read with mask values starting with "__masked__" — passing them back as-is means "do not change".
export interface NotificationChannel {
  id: number;
  name: string;
  kind: string;
  enabled: boolean;
  mode: "realtime" | "digest";
  config: Record<string, unknown>;
  filter: NotificationFilter;
  rate_per_min: number;
  created_at: string;
  updated_at: string;
  // secret_keys come from the backend per channel kind; the frontend renders password fields
  // and "leave empty to keep" hints from them, hardcoding no channel knowledge.
  secret_keys: string[];
}

// NotificationKind is the channel kind metadata returned by /api/notify/meta.
export interface NotificationKind {
  kind: string;
  default_rate_per_min: number;
  secret_keys: string[];
}

export interface NotificationMeta {
  kinds: NotificationKind[];
  enabled: boolean;
  public_base_url: string;
  digest_interval_min: string;
  defaults: { digest_interval_min: number };
  stats: {
    channels: number;
    channels_on: number;
    pending: number;
    failed: number;
    sent_today: number;
    backlog_age_ms: number;
  };
}

// NotificationDelivery is one delivery record, for delivery history and retrying failures.
export interface NotificationDelivery {
  id: number;
  finding_id: string;
  event_kind: string; // finding_created | finding_status_changed
  channel_id: number;
  channel_name: string;
  channel_kind: string;
  state: "pending" | "sending" | "sent" | "failed" | "skipped";
  attempts: number;
  last_error: string;
  batch_id?: number;
  created_at: string;
  sent_at?: string;
  next_attempt_at: string;
  title: string;
  severity: string;
}
