import type { ConvTokenSummary, DailyTokenBucket, SessionTokenUsage, TokenTotal, TokenUsage } from "@/lib/types";

import { activity } from "./activity";
import { grandTotal } from "./tasks";

const T = (iso: string) => iso; // readability helper for timestamps

// ── Token stats ──────────────────────────────────────────────────────────────
export const tokenWorkers: TokenUsage[] = [
  {
    worker: "planner",
    input_tokens: 320000,
    output_tokens: 24000,
    cache_read_tokens: 210000,
    cache_write_tokens: 33000,
  },
  {
    worker: "work#1",
    input_tokens: 288000,
    output_tokens: 21000,
    cache_read_tokens: 190000,
    cache_write_tokens: 28000,
  },
  {
    worker: "work#2",
    input_tokens: 402000,
    output_tokens: 31000,
    cache_read_tokens: 280000,
    cache_write_tokens: 41000,
  },
  {
    worker: "work#3",
    input_tokens: 274500,
    output_tokens: 20320,
    cache_read_tokens: 210400,
    cache_write_tokens: 30000,
  },
  { worker: "workG", input_tokens: 88000, output_tokens: 5200, cache_read_tokens: 60000, cache_write_tokens: 8000 },
  { worker: "work#6", input_tokens: 64000, output_tokens: 4100, cache_read_tokens: 42000, cache_write_tokens: 6000 },
  { worker: "work#7", input_tokens: 132000, output_tokens: 8600, cache_read_tokens: 90000, cache_write_tokens: 12000 },
  { worker: "work#8", input_tokens: 74000, output_tokens: 4700, cache_read_tokens: 50000, cache_write_tokens: 7000 },
  { worker: "work#9", input_tokens: 82000, output_tokens: 5100, cache_read_tokens: 56000, cache_write_tokens: 8000 },
  { worker: "work#10", input_tokens: 46000, output_tokens: 3200, cache_read_tokens: 30000, cache_write_tokens: 4500 },
  { worker: "work#11", input_tokens: 96000, output_tokens: 6200, cache_read_tokens: 64000, cache_write_tokens: 9500 },
  { worker: "work#12", input_tokens: 108000, output_tokens: 6800, cache_read_tokens: 72000, cache_write_tokens: 10000 },
  { worker: "work#13", input_tokens: 144000, output_tokens: 9200, cache_read_tokens: 96000, cache_write_tokens: 14000 },
];

// Mirrors TokenStatsBySession: completed usage is keyed by the stable UI session
// (main | plan | intent:<id>) instead of the reusable work#N executor name.
export const tokenSessions: SessionTokenUsage[] = (() => {
  const totals = new Map<string, SessionTokenUsage>();
  for (const item of activity) {
    if (item.kind !== "result") continue;
    let session = "";
    if (item.worker === "mainagent") session = "main";
    else if (item.worker === "planner") session = "plan";
    else if (item.intent_id) session = `intent:${item.intent_id}`;
    if (!session) continue;
    const current = totals.get(session) ?? {
      session,
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
    };
    current.input_tokens += item.input_tokens ?? 0;
    current.output_tokens += item.output_tokens ?? 0;
    current.cache_read_tokens += item.cache_read_tokens ?? 0;
    current.cache_write_tokens += item.cache_write_tokens ?? 0;
    totals.set(session, current);
  }
  return [...totals.values()];
})();

export const tokenTotal: TokenTotal = grandTotal;

export const dailyTokens: DailyTokenBucket[] = Array.from({ length: 30 }, (_, i) => {
  const d = new Date("2026-11-14T00:00:00Z");
  d.setUTCDate(d.getUTCDate() - (29 - i));
  const wave = 40000 + Math.round(30000 * Math.abs(Math.sin(i / 3)));
  return {
    date: d.toISOString().slice(0, 10),
    input_tokens: wave,
    output_tokens: Math.round(wave * 0.08),
    cache_read_tokens: Math.round(wave * 0.7),
    cache_write_tokens: Math.round(wave * 0.12),
  };
});

export const convTokens: ConvTokenSummary[] = [
  {
    llm_profile_id: 1,
    created_at: T("2026-11-13T10:00:00Z"),
    input_tokens: 42000,
    output_tokens: 3800,
    cache_read_tokens: 26000,
    cache_write_tokens: 5200,
  },
  {
    llm_profile_id: 2,
    created_at: T("2026-11-12T16:00:00Z"),
    input_tokens: 18000,
    output_tokens: 1600,
    cache_read_tokens: 9000,
    cache_write_tokens: 2100,
  },
];

// ── Traffic ──────────────────────────────────────────────────────────────────
