// ---- LLM config ----
export interface LLMProfile {
  id: string;
  name: string;
  format: "openai" | "anthropic" | "openai-responses" | "ollama" | "bedrock-gateway";
  base_url?: string;
  proxy?: string;
  model: string;
  api_key_hint?: string;
  rate_per_second: number;
  rate_per_minute: number;
  context_window_k?: number;
  // Thinking switch (thinking.type): ""=not sent (default) | "disabled"=off | "enabled"=on
  thinking_type?: string;
  // Thinking effort: ""=not sent (default) | "low"/"medium"/"high"/"xhigh"/"max"
  reasoning_effort?: string;
  is_default: boolean;
  // Pool order: the larger, the earlier chosen. The active profile is always chain head, regardless of this value.
  priority?: number;
  // true = not a failover target (still usable via explicit agent/task binding).
  pool_exclude?: boolean;
  // true (default) = streaming (SSE) | false = genuinely non-streaming (stream:false, one-shot response).
  streaming?: boolean;
  // Cost ledger: EUR per 1M tokens (0/absent = unset).
  price_input_per_m?: number;
  price_output_per_m?: number;
  price_cache_read_per_m?: number;
  price_cache_write_per_m?: number;
  // Output cap per reply (tokens). 0 = do not send the field; the server default decides.
  // Note the distinction from context_window_k: that is the model's total capacity, used locally only for the compression threshold.
  max_tokens?: number;
  // Which request field name carries the cap; only meaningful for format="openai":
  // ""=max_tokens (default) | "max_completion_tokens" (OpenAI reasoning models only accept it)
  max_tokens_field?: string;
  // Custom session header name: when non-empty every request carries this HTTP header, value = the current session/intent session id.
  // "" = not sent. For gateways that do prompt caching/sticky routing on a session-id header.
  session_header_key?: string;
  // Custom User-Agent: when non-empty, every outbound LLM request overrides its User-Agent header with this value. "" = do not override (keep the default UA).
  user_agent?: string;
  custom_headers_set?: boolean;
  // This profile's retry overrides (connect / empty response / same-provider safety window). Empty / all zeros = follow the global policy.
  retry?: LLMRetryOverride;
}

// ---- LLM retry policy ----
// Two knobs per retry layer. Both use "0 = not configured":
//   attempts    0 = default count | -1 = disable this layer's retries | >0 = retry count
//   interval_ms 0 = default exponential backoff | >0 = use this fixed millisecond interval
export interface LLMRetryRule {
  attempts: number;
  interval_ms: number;
}

// The three layers a single LLM profile can override (all "endpoint-bound" retries).
export interface LLMRetryOverride {
  connect: LLMRetryRule; // connect retries: connection reset/timeout/429/5xx, before the stream starts
  empty: LLMRetryRule; // empty-response retries: completed but with no content (openai format only)
  stream: LLMRetryRule; // same-provider safety-window retries: replay of a dropped stream before output is delivered
}

// Global policy = defaults for the three layers above + two global-only layers:
//   breaker pool breaker (attempts = consecutive transient failures before tripping, interval_ms = fixed cooldown)
//   intent  intent rerun (the whole intent reruns after a worker ends with model_error)
export interface LLMRetryPolicy extends LLMRetryOverride {
  breaker: LLMRetryRule;
  intent: LLMRetryRule;
}

// ---- LLM pooling (failover) ----
// A profile's position and health in the pool chain. state:
//   ok       healthy
//   degraded consecutive failures but below the breaker threshold
//   tripped  breaker open, skipped during the cooldown (cooldown_secs = seconds left)
export interface LLMPoolMember {
  profile_id: string;
  name: string;
  model: string;
  format: string;
  priority: number;
  active: boolean; // whether this is the currently active profile (always chain head)
  excluded: boolean; // pool_exclude: not part of the pool
  state: "ok" | "degraded" | "tripped";
  fails: number;
  trips: number;
  cooldown_secs: number;
  last_error?: string;
  last_at?: string;
}

export interface LLMPoolStatus {
  enabled: boolean;
  bind_fallback: boolean;
  chain: LLMPoolMember[];
}
