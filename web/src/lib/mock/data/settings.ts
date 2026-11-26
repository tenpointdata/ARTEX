import type { LLMPoolStatus, Settings } from "@/lib/types";

const T = (iso: string) => iso; // readability helper for timestamps

export const settings: Settings = {
  traffic_capture: true,
  agent_traffic_binding: false,
  llm_record: false,
  web_search_enabled: true,
  web_search_backend: "ddgs",
  brave_key_set: false,
  tavily_key_set: true,
  keenable_key_set: false,
  web_search_proxy: "",
  global_proxy: "",
  python_interpreter: "/usr/bin/python3",
  workers: 3,
  llm_pool_enabled: true,
  llm_pool_bind_fallback: false,
  noa_compaction: false,
};

// ── LLM polling (failover)──────────────────────────────────────────────────────
// Demo: the active configuration is fine; the standby was just circuit-broken due to insufficient balance and is cooling down.
export const llmPool: LLMPoolStatus = {
  enabled: true,
  bind_fallback: false,
  chain: [
    {
      profile_id: "1",
      name: "Claude Opus 4.8",
      model: "claude-opus-4-8",
      format: "anthropic",
      priority: 0,
      active: true,
      excluded: false,
      state: "ok",
      fails: 0,
      trips: 0,
      cooldown_secs: 0,
    },
    {
      profile_id: "2",
      name: "DeepSeek V4",
      model: "deepseek-v4-flash",
      format: "openai",
      priority: 10,
      active: false,
      excluded: false,
      state: "tripped",
      fails: 0,
      trips: 1,
      cooldown_secs: 42,
      last_error: "openai: status 402: insufficient balance",
      last_at: T("2026-11-14T00:10:00Z"),
    },
  ],
};

// ── Intercept ────────────────────────────────────────────────────────────────
