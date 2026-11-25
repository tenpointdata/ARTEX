import type { CostRow } from "@/lib/types";

// Derived efficiency numbers for one cost row (or the totals row). Ratios are null
// when the denominator is zero so the table can show "—" instead of NaN.
export interface CostMetrics {
  tokens: number; // input + output (input already includes cache tokens)
  cacheHit: number | null; // cache_read / input
  outIn: number | null; // output / input
  eurPerMTok: number | null;
  wallMs: number | null; // last_ts - first_ts
  avgLatencyMs: number | null;
}

export function costMetrics(r: CostRow): CostMetrics {
  const tokens = r.input_tokens + r.output_tokens;
  const first = r.first_ts ? Date.parse(r.first_ts) : Number.NaN;
  const last = r.last_ts ? Date.parse(r.last_ts) : Number.NaN;
  return {
    tokens,
    cacheHit: r.input_tokens > 0 ? r.cache_read_tokens / r.input_tokens : null,
    outIn: r.input_tokens > 0 ? r.output_tokens / r.input_tokens : null,
    eurPerMTok: r.cost_eur !== null && tokens > 0 ? (r.cost_eur / tokens) * 1e6 : null,
    wallMs: Number.isFinite(first) && Number.isFinite(last) ? Math.max(0, last - first) : null,
    avgLatencyMs: r.calls > 0 ? r.latency_ms / r.calls : null,
  };
}

const isTs = (v: string | null): v is string => Boolean(v);

// Sum rows into one totals row. cost_eur stays null only when no row is priced.
// tasks is not summed: a task spans several profiles, workers and models, so the
// caller passes the distinct count the server computed (CostResponse.tasks).
export function totalRow(rows: CostRow[], tasks: number): CostRow {
  const sum = (f: (r: CostRow) => number) => rows.reduce((a, r) => a + f(r), 0);
  const priced = rows.filter((r) => r.cost_eur !== null);
  const firsts = rows
    .map((r) => r.first_ts)
    .filter(isTs)
    .sort();
  const lasts = rows
    .map((r) => r.last_ts)
    .filter(isTs)
    .sort();
  return {
    key: "",
    label: "",
    calls: sum((r) => r.calls),
    tasks,
    errors: sum((r) => r.errors),
    input_tokens: sum((r) => r.input_tokens),
    output_tokens: sum((r) => r.output_tokens),
    cache_read_tokens: sum((r) => r.cache_read_tokens),
    cache_write_tokens: sum((r) => r.cache_write_tokens),
    latency_ms: sum((r) => r.latency_ms),
    first_ts: firsts[0] ?? null,
    last_ts: lasts[lasts.length - 1] ?? null,
    cost_eur: priced.length ? priced.reduce((a, r) => a + (r.cost_eur ?? 0), 0) : null,
    unpriced_calls: sum((r) => r.unpriced_calls),
  };
}

export type CostSortField =
  | "name"
  | "calls"
  | "in"
  | "out"
  | "cache"
  | "ratio"
  | "cost"
  | "perM"
  | "llm"
  | "wall"
  | "avg"
  | "errors";

export const COST_SORT_FIELDS: readonly CostSortField[] = [
  "name",
  "calls",
  "in",
  "out",
  "cache",
  "ratio",
  "cost",
  "perM",
  "llm",
  "wall",
  "avg",
  "errors",
];

function sortValue(r: CostRow, f: CostSortField): number | string | null {
  const m = costMetrics(r);
  switch (f) {
    case "name":
      return (r.label || r.key).toLowerCase();
    case "calls":
      return r.calls;
    case "in":
      return r.input_tokens;
    case "out":
      return r.output_tokens;
    case "cache":
      return m.cacheHit;
    case "ratio":
      return m.outIn;
    case "cost":
      return r.cost_eur;
    case "perM":
      return m.eurPerMTok;
    case "llm":
      return r.latency_ms;
    case "wall":
      return m.wallMs;
    case "avg":
      return m.avgLatencyMs;
    case "errors":
      return r.errors;
  }
}

// Stable sort; null values always sink to the bottom; ties break on total tokens desc.
export function sortCostRows(rows: CostRow[], field: CostSortField, dir: "asc" | "desc"): CostRow[] {
  const sign = dir === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    const x = sortValue(a, field);
    const y = sortValue(b, field);
    if (x === null && y !== null) return 1;
    if (y === null && x !== null) return -1;
    if (x !== null && y !== null && x !== y) return (x < y ? -1 : 1) * sign;
    return costMetrics(b).tokens - costMetrics(a).tokens;
  });
}

export function fmtInt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(1)}k`;
  return String(Math.round(n));
}

export function fmtPct(v: number | null): string {
  return v === null ? "—" : `${(v * 100).toFixed(1)}%`;
}

export function fmtEur(v: number | null, digits = 2): string {
  return v === null ? "—" : `€${v.toFixed(digits)}`;
}

// Milliseconds as h/m/s ("1h 02m", "3m 05s", "4.2s", "380ms").
export function fmtDuration(ms: number | null): string {
  if (ms === null) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)}s`;
  const total = Math.round(s);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const sec = total % 60;
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`;
  return `${m}m ${String(sec).padStart(2, "0")}s`;
}
