import type { LLMPoolStatus, LLMProfile, LLMRetryOverride, LLMRetryPolicy, LLMSampling } from "@/lib/types";

import { arr, del, get, post } from "./core";

export const llmApi = {
  // ---- LLM ----
  getLLM: () =>
    get<{
      configured: boolean;
      provider: string;
      model: string;
      base_url: string;
      proxy?: string;
      key_set: boolean;
      rate_per_second?: number;
      rate_per_minute?: number;
      context_window_k?: number;
      thinking_type?: string;
      reasoning_effort?: string;
    }>("/llm"),
  setLLM: (
    provider: string,
    model: string,
    base_url: string,
    api_key: string,
    rate_per_second = 0,
    rate_per_minute = 0,
    proxy = "",
    context_window_k = 0,
  ) => post("/llm", { provider, model, base_url, proxy, api_key, rate_per_second, rate_per_minute, context_window_k }),
  testLLM: (
    provider: string,
    model: string,
    base_url: string,
    api_key: string,
    proxy = "",
    thinking_type = "",
    reasoning_effort = "",
    profile_id?: number,
    streaming = true, // test with the profile's real streaming mode so "streaming works, non-streaming does not" never leaks into conversations
    session_header_key = "", // non-empty = the test request also carries this custom session header (value is a one-off session id)
    user_agent = "", // non-empty = the test overrides the User-Agent header with this value
    custom_headers = "",
  ) =>
    // reply = the model's actual reply (truncated); the backend marks a profile that replies with nothing as failed
    post<{ ok: boolean; error?: string; latency_ms?: number; model?: string; reply?: string }>("/llm/test", {
      provider,
      model,
      base_url,
      proxy,
      api_key,
      thinking_type,
      reasoning_effort,
      profile_id,
      streaming,
      session_header_key,
      user_agent,
      custom_headers,
    }),
  llmProfiles: () => get<{ profiles: LLMProfile[] }>("/llm/profiles").then((r) => arr(r.profiles)),
  saveLLMProfile: (p: {
    id?: number; // omit/0 = create; set = update that profile
    name: string;
    format: string;
    model: string;
    base_url?: string;
    proxy?: string;
    api_key?: string; // blank on update keeps the existing key
    rate_per_second?: number;
    rate_per_minute?: number;
    context_window_k?: number;
    thinking_type?: string; // ""(not sent)|"disabled"|"enabled"
    reasoning_effort?: string; // ""(not sent)|"low"|"medium"|"high"|"xhigh"|"max"
    priority?: number; // pool order position; higher is used first
    pool_exclude?: boolean; // true = not a failover target
    streaming?: boolean; // true (default) = streaming | false = non-streaming
    max_tokens?: number; // output cap per reply; 0 = not sent, the server default decides
    max_tokens_field?: string; // ""=max_tokens (default) | "max_completion_tokens" (openai format only)
    session_header_key?: string; // non-empty = send this HTTP header on every request, value = the current session id; "" = not sent
    user_agent?: string; // non-empty = override the User-Agent header on every request; "" = keep the default UA
    custom_headers?: string; // static Bedrock gateway headers; values are write-only
    clear_custom_headers?: boolean;
    retry?: LLMRetryOverride; // this profile's retry override; all zeros = follow the global retry policy
    sampling?: LLMSampling; // optional seed/temperature pin; absent fields are not pinned
  }) => post<{ id: number }>("/llm/profiles", p),
  deleteLLMProfile: (id: string) => del<{ deleted: number }>(`/llm/profiles/${id}`),
  activateLLMProfile: (id: string) => post<{ ok: boolean }>("/llm/profiles/active", { id: Number(id) }),
  // The pool chain's actual order + each profile's breaker state.
  llmPool: () => get<LLMPoolStatus>("/llm/pool"),
  // Clear the breaker so the next call retries this profile immediately; no id = clear all.
  resetLLMPool: (id?: string) => post<LLMPoolStatus>("/llm/pool/reset", { id: id ? Number(id) : 0 }),
  // Global retry policy (count + interval for each of the five layers). All zeros = built-in defaults everywhere.
  llmRetryPolicy: () => get<LLMRetryPolicy>("/llm/retry-policy"),
  saveLLMRetryPolicy: (p: LLMRetryPolicy) => post<LLMRetryPolicy>("/llm/retry-policy", p),
  fetchLLMModels: (provider: string, base_url: string, api_key: string, proxy = "", profile_id?: number) =>
    post<{ ok: boolean; error?: string; models?: string[] }>("/llm/models", {
      provider,
      base_url,
      api_key,
      proxy,
      profile_id,
    }),
};
