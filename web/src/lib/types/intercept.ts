// ---- Intercept Rules ----
export type InterceptAction = "allow" | "deny" | "ask";
export type InterceptMatchTarget = "tool_name" | "tool_input";
// argv: pattern is a JSON array of tokens, the prefix of a Bash command (program and subcommands; flags are not constrained).
export type InterceptMatchType = "string" | "regex" | "argv";

export interface InterceptRule {
  id: number;
  name: string;
  enabled: boolean;
  priority: number;
  match_target: InterceptMatchTarget;
  match_type: InterceptMatchType;
  pattern: string;
  action: InterceptAction;
  message: string;
  timeout_enabled: boolean;
  timeout_seconds: number;
  timeout_action: "deny" | "allow";
  created_at: string;
  updated_at: string;
}

// ---- Asset Intercept Rules (asset interception: global blocklist) ----
export type AssetInterceptKind =
  | "exact_domain"
  | "exact_ip"
  | "exact_url"
  | "fuzzy_domain"
  | "fuzzy_ip"
  | "fuzzy_url"
  | "cidr";

// action is only for task-level rules: block = intercept (forbid testing), allow = permit (allowlist).
export type AssetInterceptAction = "block" | "allow";

// Input row for task-level asset interception/allow rules (used when creating a task and editing task details).
export interface AssetInterceptRuleInput {
  action: AssetInterceptAction;
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  enabled: boolean;
}

// Task credentials (#3): per-task test accounts. An empty password on a user-declared
// entry means "agent generates": the agent registers that account and records it back.
export type TaskCredentialInput = {
  domain: string;
  username: string;
  password: string;
  note?: string;
};

export type TaskCredential = TaskCredentialInput & {
  id: number;
  task_id: number;
  source: "user" | "agent";
  note: string;
  created_at: string;
  updated_at: string;
};

export interface AssetInterceptRule {
  id: number;
  enabled: boolean;
  action?: AssetInterceptAction; // global rules omit this field (always intercept); task-level rules distinguish block/allow
  kind: AssetInterceptKind;
  pattern: string;
  note: string;
  builtin: boolean;
  created_at: string;
  updated_at: string;
}

export interface InterceptPending {
  decision_source?: "rule" | "model" | "unknown" | "";
  // Actor that settled the request, exactly as stored: "" while pending and on
  // rows decided before the column existed (shown as "Legacy").
  decided_by?: "human" | "rule" | "model" | "system" | "";
  id: number;
  rule_id?: number;
  conversation_id?: number;
  task_id?: string;
  agent_name: string;
  tool_name: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  tool_input: Record<string, unknown>;
  status: "pending" | "allowed" | "denied" | "timeout";
  reason: string; // the rule's message or the model's verdict reason (model verdicts carry a [model] prefix)
  decided_at?: string;
  created_at: string;
  // Deterministic risk score, computed by the server when a pending row is read.
  risk?: InterceptRisk;
}

export interface InterceptRisk {
  score: number; // 0..100
  level: "low" | "medium" | "high";
  reasons: string[];
}

export interface InterceptBatchDecideResult {
  decided: number;
  failed: number;
  results: { id: number; ok: boolean; error?: string }[];
}

// JudgeConfig: global config for model fallback approval (the model decides only when no interception rule matches).
export interface JudgeConfig {
  enabled: boolean;
  mode?: "closed" | "freeform"; // closed (default): fixed questions, code decides; freeform: the prompt decides
  profile_id: number; // 0 = follow the active/default profile
  prompt: string; // verdict prompt; on GET the backend backfills the full built-in template when unset
  timeout_seconds: number; // model call timeout
  fail_action: "allow" | "ask" | "deny"; // fallback when the model errors/times out/is unparseable
  ask_timeout_seconds: number; // approval wait timeout after the model says ask and it goes to a human
  ask_timeout_action: "allow" | "deny"; // default action after the approval timeout
}

// JudgeUsage: cumulative token usage of model fallback approval (the judge channel) + daily series for the last N days.
export interface JudgeDayUsage {
  date: string; // YYYY-MM-DD (UTC)
  calls: number;
  input_tokens: number;
  output_tokens: number;
}
export interface JudgeUsage {
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  daily: JudgeDayUsage[];
}

export interface InterceptApprovalFilter {
  status?: InterceptPending["status"];
  decision_source?: "rule" | "model" | "unknown";
  decided_by?: "human" | "rule" | "model" | "system";
}

// InterceptApprovalRow enriches InterceptPending with conversation/task and rule context.
export interface InterceptApprovalRow extends InterceptPending {
  conv_title: string; // "" if no linked conversation
  conv_agent_key: string; // "" if no linked conversation
  rule_name: string; // "" if rule was deleted
}
