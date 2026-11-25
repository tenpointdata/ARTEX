import type { Activity } from "./activity";
import type { InterceptApprovalRow } from "./intercept";

// ── Asset sync (ScopeSentry data source) ──────────────────────────────────────────────
export interface SSProject {
  id: string; // MongoDB ObjectID — used as filter.project
  name: string;
  logo?: string;
  AssetCount?: number;
  tag?: string;
}

export interface SSTask {
  id: string;
  name: string; // used as filter.task
  status?: number;
  progress?: number;
  creatTime?: string;
  endTime?: string;
}

// ConvTokenSummary — one conversation's token total (+ profile/date) for merging
// chat usage into the dashboard token stats. GET /api/tokens/conversations.
export interface ConvTokenSummary {
  llm_profile_id: number | null;
  created_at: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
}

// ---- Command recording (Bash execution history) ----
export interface CommandRecord {
  id: number;
  exploration_id: number;
  worker: string;
  tool: string;
  command: string; // raw tool input (JSON)
  output: string;
  is_error: boolean;
  created_at: string;
}

// Call stats for one tool (/commands/stats); errors is the failed subset.
export interface ToolStat {
  tool: string;
  total: number;
  errors: number;
}

// ---- LLM recording ----
export interface LLMRecordItem {
  id: number;
  ts: string;
  model: string;
  profile_name: string;
  session_id: string;
  task_id: string;
  worker: string;
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  cache_read: number;
  cache_write: number;
  status: string;
  error?: string;
}

export interface LLMRecordDetail extends LLMRecordItem {
  request_body: string;
  response_body: string;
  // Raw HTTP actually exchanged with the provider: the request is the full body sent by
  // buildBody() (incl. tool schemas), the response is raw SSE frames. The
  // request_body/response_body above are the normalized view, dropping tool schemas and tool_use blocks. Empty for old records.
  raw_request?: string;
  raw_response?: string;
}

// One distinct task with its LLM-record count (task picker on the records page).
export interface LLMTask {
  task_id: string;
  count: number;
}

// The exact JSON sent to the review model, retained for all model verdicts.
export interface InterceptReviewInput {
  version: number;
  background?: {
    // worker_summary is retained only for immutable v2/v3 snapshots.
    source: "user_message" | "worker_summary";
    text: string;
    truncated?: boolean;
  };
  // Version 1 snapshots are immutable and remain readable in historical audits.
  task?: {
    task_id: number;
    description: string;
    goal: string;
    constraints: { id: number; kind: string; text: string; origin: string; created_at: number }[];
    truncated?: boolean;
  };
  working_directory?: string;
  worker_intent?: string;
  turn_input?: string;
  background_truncated?: boolean;
  // Legacy v1/v2 snapshots only; v3 never sends execution history.
  history?: {
    tool_use_id: string;
    tool: string;
    arguments_preview: string;
    result: string;
    status: "succeeded" | "failed";
    truncated?: boolean;
  }[];
  history_truncated?: boolean;
  correlation?: "exact" | "ambiguous" | "unavailable";
  tool_name: string;
  arguments: Record<string, unknown>;
}

// Immutable review snapshot plus separately recorded execution outcome.
export interface InterceptAudit {
  model_input?: InterceptReviewInput;
  model_input_digest?: string;
  run_id?: string;
  tool_use_id?: string;
  correlation: "exact" | "ambiguous" | "unavailable";
  input_digest: string;
  user_message: string;
  user_truncated?: boolean;
  context:
    | { kind: string; tool?: string; tool_use_id?: string; text: string; is_error?: boolean; truncated?: boolean }[]
    | null;
  context_truncated?: boolean;
  captured_at: string;
  model_fallback?: boolean;
  initial_action: "allow" | "ask" | "deny";
  initial_reason: string;
  effective_action?: "allow" | "deny";
  decision_reason?: string;
  rule_name?: string;
  config_digest?: string;
  profile_id?: number;
  judge_answers?: { id: string; answer: "yes" | "no" | "unknown"; evidence: string }[];
  execution_status: "not_started" | "not_executed" | "awaiting_result" | "succeeded" | "failed" | "unknown";
  output?: string;
  output_truncated?: boolean;
  execution_ended_at?: string;
}
export interface InterceptDetail extends InterceptApprovalRow {
  audit: InterceptAudit | null;
  always_allow_prefix?: string; // pending Bash request: the argv prefix "always allow" would allow
}

export type TrafficEvidenceRole = "baseline" | "proof" | "verification" | "supporting";
export interface TrafficEvidenceRef {
  traffic_id: string;
  role?: TrafficEvidenceRole;
  note?: string;
}
export interface TrafficEvidenceSnapshot {
  id: string;
  source_traffic_id: string;
  captured_at: number;
  url: string;
  method: string;
  status: number;
  content_type: string;
  req_head?: string;
  resp_head?: string;
  req_hash: string;
  resp_hash: string;
  req_len: number;
  resp_len: number;
  rewritten?: boolean;
  rewrite_rule_ids?: number[];
}
export interface FindingTrafficBinding {
  id: string;
  finding_id: string;
  snapshot_id: string;
  role: TrafficEvidenceRole;
  note: string;
  position: number;
  created_at: string;
  snapshot: TrafficEvidenceSnapshot;
}
export interface FindingTraffic {
  finding_id: string;
  version: number;
  report_version: number;
  bindings: FindingTrafficBinding[];
}
export interface EvidenceBodyPreview {
  content: string;
  offset: number;
  total: number;
  next_offset: number;
  truncated: boolean;
  binary: boolean;
}
export interface FindingTrafficDetail {
  binding: FindingTrafficBinding;
  request: EvidenceBodyPreview;
  response: EvidenceBodyPreview;
}

// Original execution selected from an approval, never submitted to the reviewer.
export interface InterceptExecution {
  conversation_id: number | null;
  task_id: string | null;
  session: string;
  seq: number;
  items: Activity[];
}

// ---- capture-proxy constrained Go tampering hooks (P4.2) ----
export type TamperHookPhase = "request" | "response";

export interface TamperHookHeaderMatcher {
  name: string;
  value_regex?: string;
}

export interface TamperHookMatchers {
  methods?: string[];
  host?: string;
  url_regex?: string;
  headers?: TamperHookHeaderMatcher[];
  body_regex?: string;
  status?: string[];
  resp_headers?: TamperHookHeaderMatcher[];
  resp_body_regex?: string;
}

export interface TamperHookLastError {
  class: string;
  at: string;
  count: number;
}

export interface TamperHook {
  id: number;
  name: string;
  enabled: boolean;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  source_sha256: string;
  version: number;
  note: string;
  created_at: string;
  updated_at: string;
  load_error?: string;
  last_error?: TamperHookLastError | null;
}

export interface TamperHooksListResp {
  hooks: TamperHook[];
  enabled_by_env: boolean;
}

export interface TamperHookVersion {
  id: number;
  hook_id: number;
  version: number;
  name: string;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  source_sha256: string;
  note: string;
  created_at: string;
}

export interface TamperHookInput {
  name: string;
  enabled?: boolean;
  phase: TamperHookPhase;
  priority: number;
  matchers: TamperHookMatchers;
  source: string;
  note?: string;
}

export interface TamperDryRunIO {
  request: {
    method: string;
    url: string;
    headers: Record<string, string[]>;
    body: string;
  };
  response: {
    status: number;
    headers: Record<string, string[]>;
    body: string;
  };
}

export interface TamperDryRunRequest extends TamperDryRunIO {
  hook_id?: number;
  name?: string;
  phase?: TamperHookPhase;
  matchers?: TamperHookMatchers;
  source?: string;
}

export interface TamperTraceLine {
  Phase: string;
  SourceKind: string;
  RuleID: number;
  HookID: number;
  HookVersion: number;
  RuleSnapshot: string;
  Action: string;
  Field: string;
  Outcome: string;
  Reason: string;
  BeforeHash: string;
  AfterHash: string;
  After: string | null;
}

export interface TamperDryRunResult extends TamperDryRunIO {
  changed: boolean;
  lines: TamperTraceLine[] | null;
}

export type MethodologyItemStatusValue = "open" | "in_progress" | "covered";

export interface MethodologyItemStatus {
  id: string;
  category: string;
  title: string;
  status: MethodologyItemStatusValue;
  outcomes: string[];
  intents: number[];
}

/** A built-in testing methodology as offered on the task create form (GET /api/methodologies). */
export interface MethodologyRef {
  id: string;
  name: string;
  version: string;
  source: string;
}

export interface MethodologySummary extends MethodologyRef {
  applies_when: string[];
  item_count: number;
}

export interface MethodologyScore extends MethodologyRef {
  total: number;
  covered: number;
  in_progress: number;
  open: number;
  /** 0..1 */
  pct: number;
  items: MethodologyItemStatus[];
}

/** Response of GET /api/tasks/{id}/coverage. */
export interface TaskCoverage {
  /** Whether the asset coverage feature is on; when false the other fields are zero values. */
  enabled: boolean;
  scope_rows: number;
  denominator: number;
  tested: number;
  pct: number | null;
  by_type: { type: string; total: number; tested: number }[];
  /** Coverage ledger (absent when the feature is off): gaps = open items; an unexamined class is never clean. */
  gaps?: { kind: string; surface?: string; risk_area: string }[];
  completeness?: { complete: boolean; state: string; caveat: string };
  /** Checklist completion per testing methodology; absent/empty when none applies. */
  methodologies?: MethodologyScore[];
}
