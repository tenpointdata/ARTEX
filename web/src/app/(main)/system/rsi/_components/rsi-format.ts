// Display helpers shared by the RSI gate page.

import { t } from "@/i18n";
import type { RSIProposal, RSIRate } from "@/lib/rsi-types";

// rate renders passes/n with the percentage; a rate is never shown without its denominator.
export function rate(r: RSIRate): string {
  if (r.n === 0) return "0/0";
  return `${r.passes}/${r.n} (${Math.round((100 * r.passes) / r.n)}%)`;
}

// moduleOf is what a proposal edits: a prompt role, or the lessons it graduates.
export function moduleOf(p: RSIProposal): string {
  if (p.surface === "prompt_body") return p.target ?? "";
  return t("lessons {ids}", { ids: p.lesson_ids.map((id) => `#${id}`).join(", ") });
}

// promptHistoryHref links a prompt proposal to its agent's prompt history
// (the agent editor's default tab lists every saved version).
export function promptHistoryHref(p: RSIProposal): string | null {
  return p.surface === "prompt_body" && p.target ? `/system/agents/detail?key=${encodeURIComponent(p.target)}` : null;
}

export function when(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}
