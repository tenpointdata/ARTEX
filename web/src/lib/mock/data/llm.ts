import type { LLMProfile } from "@/lib/types";

export const llmProfiles: LLMProfile[] = [
  {
    id: "1",
    name: "Claude Opus 4.8",
    format: "anthropic",
    model: "claude-opus-4-8",
    api_key_hint: "…a3f2",
    rate_per_second: 0,
    rate_per_minute: 0,
    context_window_k: 1000,
    thinking_type: "enabled",
    reasoning_effort: "high",
    is_default: true,
    priority: 0,
    pool_exclude: false,
    // anthropic's field names are fixed, so max_tokens_field is always empty.
    max_tokens: 0,
    max_tokens_field: "",
  },
  {
    id: "2",
    name: "DeepSeek V4",
    format: "openai",
    base_url: "https://api.deepseek.com",
    model: "deepseek-v4-flash",
    api_key_hint: "…9c11",
    rate_per_second: 0,
    rate_per_minute: 60,
    context_window_k: 128,
    thinking_type: "",
    reasoning_effort: "",
    is_default: false,
    priority: 10,
    pool_exclude: false,
    // openai format + reasoning models: the limit goes through max_completion_tokens.
    max_tokens: 8192,
    max_tokens_field: "max_completion_tokens",
  },
];

export const llmConfig = {
  configured: true,
  provider: "anthropic",
  model: "claude-opus-4-8",
  base_url: "",
  proxy: "",
  key_set: true,
  rate_per_second: 0,
  rate_per_minute: 0,
  context_window_k: 1000,
  thinking_type: "enabled",
  reasoning_effort: "high",
};

// ── Agents ───────────────────────────────────────────────────────────────────
