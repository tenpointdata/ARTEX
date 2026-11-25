import { intlLocale } from "@/i18n";
import type { TrafficExchange } from "@/lib/types";

export function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function fmtBytes(n: number) {
  if (n <= 0) return "0 B";
  // GB matters for the reclaimed-space figure a full purge reports; a capture-heavy
  // instance can hand back several.
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
  const v = n / 1024 ** i;
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

export function statusTone(status: number) {
  if (status >= 500) return "text-red-500";
  if (status >= 400) return "text-amber-500";
  if (status >= 300) return "text-blue-500";
  if (status >= 200) return "text-emerald-500";
  return "text-muted-foreground";
}

// Older captures may predate Host persistence because net/http keeps Host
// outside Request.Header. Fill it for display while newly recorded traffic is
// fixed at the recorder layer as well.
export function requestWithHost(raw: string, exchange: TrafficExchange): string {
  if (!raw.trim() || /^host\s*:/im.test(raw)) return raw;
  let host = exchange.host;
  try {
    host = new URL(exchange.url).host || host;
  } catch {
    // Relative or legacy URLs fall back to the indexed host.
  }
  const newline = raw.includes("\r\n") ? "\r\n" : "\n";
  const firstLineEnd = raw.indexOf(newline);
  if (firstLineEnd < 0) return `${raw}${newline}Host: ${host}`;
  return `${raw.slice(0, firstLineEnd + newline.length)}Host: ${host}${newline}${raw.slice(firstLineEnd + newline.length)}`;
}

// Fixed method set (server filters exact-match); avoids deriving options from a
// single page, which would only ever list the methods on that page.
export const METHODS = ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"];
export const PAGE_SIZES = [25, 50, 100, 200];

// Server-sortable columns. The list is sent to the backend verbatim as `sort`,
// which whitelists these same names, so keep them in sync with traffic.Page.
export const SORT_FIELDS = ["ts", "status", "resp_len"] as const;
export type SortField = (typeof SORT_FIELDS)[number];
export const SORT_STORAGE_KEY = "traffic-sort";

// Status-class buckets for the filter dropdown; the value is sent as `status`,
// which the backend reads as either an exact code or an "Nxx" class band.
export const STATUS_BUCKETS = ["2xx", "3xx", "4xx", "5xx"];

export type DeleteMode = "filter" | "selected" | "all";
