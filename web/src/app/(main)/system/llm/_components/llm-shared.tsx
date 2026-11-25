import type * as React from "react";

import { t } from "@/i18n";
import type { LLMPoolMember, LLMProfile } from "@/lib/types";

// Renders a translated string, turning <b>…</b> spans into bold elements, so a sentence with emphasis stays one key.
export function rich(s: string): React.ReactNode[] {
  return s.split(/<b>(.*?)<\/b>/).map((part, i) => (i % 2 === 1 ? <b key={i}>{part}</b> : part));
}

// The thinking switch (thinking.type) and the thinking effort (reasoning_effort) are two [independent] fields,
// each set on its own — some APIs have no thinking field and activate thinking via the effort parameter alone, hence the decoupling.
// An empty string in storage = the field is [not sent]; Radix Select rejects an empty value, so the UI uses "none"
// as the sentinel for not sent, converted to and from "" on load and save (NONE / fromStore / toStore).
export const NONE = "none";
export const fromStore = (v?: string) => (v ? v : NONE);
export const toStore = (v: string) => (v === NONE ? "" : v);
export const THINKING_TYPES: { value: string; label: string }[] = [
  { value: NONE, label: t("Not sent (default)") },
  { value: "disabled", label: t("Disabled") },
  { value: "enabled", label: t("Enabled") },
];
// Which request field name carries the output cap (only meaningful for the openai format). NONE ↔ "" shares the same sentinel conversion.
export const MAX_TOKENS_FIELDS: { value: string; label: string }[] = [
  { value: NONE, label: t("max_tokens (default)") },
  { value: "max_completion_tokens", label: "max_completion_tokens" },
];
// The other formats hard-code their field names, so the option is meaningless for them; the hint texts spell this out.
export const MAX_TOKENS_FIELD_HINTS: Record<string, string> = {
  openai: t(
    "Which key carries the cap. max_tokens is the default and the vast majority of compatible gateways only accept it; OpenAI's official reasoning models (the o series / GPT-5) conversely only accept max_completion_tokens and fail outright with unsupported_parameter when they receive max_tokens.",
  ),
  anthropic: t("Only selectable for the openai format. Anthropic's field name is fixed as max_tokens."),
  "openai-responses": t(
    "Only selectable for the openai format. The Responses API's field name is fixed as max_output_tokens.",
  ),
  ollama: t("Only selectable for the openai format. Ollama's field name is fixed as max_tokens."),
};
// LLM profile formats. The default in the create form is "anthropic"; "ollama" is the
// keyless local-server format (base URL defaults to http://127.0.0.1:11434/v1).
export type ProfileFormat = "anthropic" | "openai" | "openai-responses" | "ollama" | "bedrock-gateway";
export const OLLAMA_DEFAULT_BASE_URL = "http://127.0.0.1:11434/v1";
export const EFFORT_LEVELS: { value: string; label: string }[] = [
  { value: NONE, label: t("Not sent (default)") },
  { value: "low", label: "low" },
  { value: "medium", label: "medium" },
  { value: "high", label: "high" },
  { value: "xhigh", label: "xhigh" },
  { value: "max", label: "max" },
];

export function cooldownText(secs: number) {
  if (secs <= 0) return "";
  if (secs < 60) return t("{n}s", { n: secs });
  return t("{n}min", { n: Math.ceil(secs / 60) });
}

// The "healthy or not" badge a profile shows on its card. A profile without a Key cannot send any request at all — say that before the breaker;
// the exception is Ollama, a keyless local server that IS callable without a key. The other states come from the pool's breaker
// records (with polling off no new records appear, so "healthy" = no known failure).
export type Health = { label: string; cls: string; hint?: string };
export function healthOf(p: LLMProfile, m?: LLMPoolMember): Health {
  if (!p.api_key_hint && p.format !== "ollama") {
    return {
      label: t("No Key configured"),
      cls: "border-muted-foreground/40 text-muted-foreground",
      hint: t("No API Key set; cannot be called"),
    };
  }
  if (m?.state === "tripped") {
    return {
      label:
        m.cooldown_secs > 0 ? t("Tripped · {cooldown}", { cooldown: cooldownText(m.cooldown_secs) }) : t("Tripped"),
      cls: "border-destructive/50 text-destructive",
      hint: m.last_error,
    };
  }
  if (m?.state === "degraded") {
    return {
      label: t("Degraded · failed {count} times", { count: m.fails }),
      cls: "border-amber-500/50 text-amber-600 dark:text-amber-400",
      hint: m.last_error,
    };
  }
  return { label: t("Healthy"), cls: "border-emerald-500/50 text-emerald-600 dark:text-emerald-400" };
}
