import { t } from "@/i18n";

export function fmtRel(ts?: string | number): string {
  if (!ts) return "—";
  const ms = Date.now() - (typeof ts === "number" ? ts * 1000 : Date.parse(ts));
  const s = Math.floor(ms / 1000);
  if (s < 60) return t("{n}s ago", { n: s });
  const m = Math.floor(s / 60);
  if (m < 60) return t("{n}m ago", { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t("{n}h ago", { n: h });
  return t("{n}d ago", { n: Math.floor(h / 24) });
}

export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export const ASSET_TYPE_LABELS: Record<string, string> = {
  root_domain: t("Root domain"),
  ip: "IP",
  subdomain: t("Subdomain"),
  app: t("App"),
  service: t("Service"),
  endpoint: t("Endpoint"),
};

export const ASSET_COLORS: Record<string, string> = {
  root_domain: "bg-blue-500",
  ip: "bg-indigo-400",
  subdomain: "bg-violet-500",
  app: "bg-orange-400",
  service: "bg-cyan-500",
  endpoint: "bg-emerald-500",
};

export function statusColor(code: number): string {
  if (code < 300) return "text-emerald-500";
  if (code < 400) return "text-blue-400";
  if (code < 500) return "text-amber-400";
  return "text-red-400";
}

export function statusBg(code: number): string {
  if (code < 300) return "bg-emerald-500";
  if (code < 400) return "bg-blue-400";
  if (code < 500) return "bg-amber-400";
  return "bg-red-400";
}

export function workerBg(w: string): string {
  if (w === "planner") return "bg-violet-500/10 text-violet-400 border-violet-500/20";
  if (w === "mainagent") return "bg-cyan-500/10 text-cyan-400 border-cyan-500/20";
  return "bg-blue-500/10 text-blue-400 border-blue-500/20";
}

export const SEVERITY_RANK: Record<string, number> = { critical: 4, high: 3, medium: 2, low: 1 };
