import { intlLocale } from "@/i18n";

// fmtAuditTime renders an audit entry's RFC 3339 timestamp in local time.
export function fmtAuditTime(value: string): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value || "—";
  return d.toLocaleString(intlLocale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// shortHash keeps the head of a hex digest; the full value is in the entry sheet.
export function shortHash(hash: string): string {
  return hash.length > 12 ? `${hash.slice(0, 12)}…` : hash || "—";
}
