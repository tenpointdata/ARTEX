"use client";

import type * as React from "react";

import { KeyRoundIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { t } from "@/i18n";
import { httpHref } from "@/lib/http-href";
import type { Asset } from "@/lib/types";
import { cn } from "@/lib/utils";

// The asset tabs are data: each tab is a list of typed columns plus the name a row
// is called by (in its delete label). AssetTable renders any of them, so the tabs
// share selection, paging and deletion instead of repeating the table markup.

export type AssetTab = "root_domain" | "ip" | "subdomain" | "app" | "service" | "endpoint";

export const ASSET_TABS: readonly AssetTab[] = ["root_domain", "ip", "subdomain", "app", "service", "endpoint"];

export interface AssetCellContext {
  companyName: (id?: number) => string;
}

export interface AssetColumn {
  header: string; // untranslated; AssetTable passes it through t()
  className?: string;
  title?: (a: Asset) => string | undefined;
  cell: (a: Asset, ctx: AssetCellContext) => React.ReactNode;
}

export interface AssetTabSpec {
  columns: AssetColumn[];
  name: (a: Asset) => string | number;
}

const METHOD_COLOR: Record<string, string> = {
  GET: "bg-emerald-100 text-emerald-700",
  POST: "bg-blue-100 text-blue-700",
  PUT: "bg-amber-100 text-amber-700",
  PATCH: "bg-orange-100 text-orange-700",
  DELETE: "bg-red-100 text-red-700",
  HEAD: "bg-purple-100 text-purple-700",
  OPTIONS: "bg-slate-100 text-slate-600",
};

function MethodBadge({ method }: { method: string }) {
  const m = method.toUpperCase();
  return (
    <span
      className={cn(
        "inline-block rounded px-1.5 py-0.5 font-mono font-semibold text-[10px] leading-none",
        METHOD_COLOR[m] ?? "bg-muted text-muted-foreground",
      )}
    >
      {m || "—"}
    </span>
  );
}

function statusTone(code: number) {
  if (code >= 500) return "text-red-500";
  if (code >= 400) return "text-amber-500";
  if (code >= 300) return "text-blue-500";
  if (code >= 200) return "text-emerald-500";
  return "text-muted-foreground";
}

// uniq drops empty and repeated labels, so each chip has a stable, unique key.
function uniq(items: string[]): string[] {
  return [...new Set(items.filter(Boolean))];
}

export function Chips({ items, mono }: { items: string[]; mono?: boolean }) {
  const clean = uniq(items);
  if (clean.length === 0) return <span className="text-muted-foreground text-xs">—</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {clean.map((s) => (
        <Badge key={s} variant="outline" className={cn("text-[10px]", mono && "font-mono")}>
          {s}
        </Badge>
      ))}
    </div>
  );
}

function ServiceDomain({ a }: { a: Asset }) {
  const href = a.service_type === "http" ? httpHref(a.url) : undefined;
  if (href) {
    return (
      <a href={href} target="_blank" rel="noreferrer" className="text-blue-600 hover:underline dark:text-blue-400">
        {text(a.domain, a.url)}
      </a>
    );
  }
  return <>{text(a.domain, a.url)}</>;
}

function AuthLabels({ a }: { a: Asset }) {
  const labels = uniq(
    (a.auth ?? []).map((authItem) => {
      const item = authItem as Record<string, string>;
      return item.type || item.username || t("Auth");
    }),
  );
  if (labels.length === 0) return <span className="text-muted-foreground text-xs">—</span>;
  return (
    <>
      {labels.map((label) => (
        <span key={label} className="inline-flex items-center gap-1 text-[11px]">
          <KeyRoundIcon className="size-3 text-muted-foreground" />
          <span className="font-mono">{label}</span>
        </span>
      ))}
    </>
  );
}

const MONO = "font-mono text-xs";
const KEY_MONO = "font-medium font-mono text-xs";
// text is the first non-empty value, or a dash: empty strings and port 0 mean "not known".
function text(...vals: (string | number | undefined)[]): string | number {
  return vals.find((v) => v !== undefined && v !== "" && v !== 0) ?? "—";
}

export const ASSET_TAB_SPECS: Record<AssetTab, AssetTabSpec> = {
  root_domain: {
    name: (a) => a.domain || a.id,
    columns: [
      { header: "Domain", className: KEY_MONO, cell: (a) => a.domain },
      { header: "ICP filing", className: "text-xs", cell: (a) => text(a.icp) },
      {
        header: "Company",
        className: "text-xs",
        cell: (a, ctx) =>
          ctx.companyName(a.company_id) || <span className="text-muted-foreground">{t("Unassigned")}</span>,
      },
    ],
  },
  ip: {
    name: (a) => a.ip || a.id,
    columns: [
      { header: "IP", className: KEY_MONO, cell: (a) => a.ip },
      { header: "C-segment", className: MONO, cell: (a) => text(a.c_segment) },
      { header: "Bound domains", cell: (a) => <Chips items={a.bound_domains ?? []} mono /> },
      {
        header: "Open ports",
        cell: (a) => (
          <Chips
            items={(a.open_ports ?? []).map((p) => (p.service ? `${p.port}/${p.service}` : String(p.port)))}
            mono
          />
        ),
      },
    ],
  },
  subdomain: {
    name: (a) => a.domain || a.id,
    columns: [
      { header: "Domain", className: KEY_MONO, cell: (a) => a.domain },
      { header: "Root domain", className: MONO, cell: (a) => text(a.root_domain) },
      { header: "Record type", className: "text-xs", cell: (a) => text(a.record_type) },
      {
        header: "Record value",
        className: "max-w-xs truncate font-mono text-xs",
        cell: (a) => text(Array.isArray(a.record_value) ? a.record_value.join(", ") : a.record_value),
      },
    ],
  },
  app: {
    name: (a) => a.app_name || a.id,
    columns: [
      { header: "App name", className: "font-medium text-xs", cell: (a) => text(a.app_name) },
      { header: "Bundle ID", className: MONO, cell: (a) => text(a.bundle_id) },
      { header: "Category", className: "text-xs", cell: (a) => text(a.category) },
      { header: "ICP filing", className: "text-xs", cell: (a) => text(a.app_icp) },
    ],
  },
  service: {
    name: (a) => a.url || a.id,
    columns: [
      {
        header: "Service",
        className: "text-xs",
        cell: (a) => {
          const isHttp = a.service_type === "http";
          const svc = a.service_name || (isHttp ? "http" : "") || a.service_type || "";
          return (
            <Badge variant={isHttp ? "default" : "secondary"} className="font-mono text-[10px]">
              {svc || "—"}
            </Badge>
          );
        },
      },
      {
        header: "Domain",
        className: "max-w-[14rem] truncate font-mono text-xs",
        title: (a) => a.domain || a.url,
        cell: (a) => <ServiceDomain a={a} />,
      },
      { header: "IP", className: MONO, cell: (a) => text(a.ip) },
      { header: "Port", className: "font-mono text-xs tabular-nums", cell: (a) => text(a.port) },
      {
        header: "Status",
        cell: (a) =>
          a.status_code != null ? (
            <span className={cn("font-mono font-semibold text-xs tabular-nums", statusTone(a.status_code))}>
              {a.status_code}
            </span>
          ) : (
            "—"
          ),
      },
      {
        header: "Title",
        className: "max-w-[12rem] truncate text-xs",
        title: (a) => a.page_title,
        cell: (a) => text(a.page_title),
      },
      { header: "Fingerprint", cell: (a) => <Chips items={a.technologies ?? []} /> },
      { header: "Auth", cell: (a) => <AuthLabels a={a} /> },
    ],
  },
  endpoint: {
    name: (a) => a.url || a.id,
    columns: [
      { header: "Method", className: "w-16", cell: (a) => <MethodBadge method={a.method || ""} /> },
      {
        header: "Full URL",
        className: "max-w-sm truncate font-mono text-xs",
        title: (a) => a.url,
        cell: (a) => text(a.url),
      },
      {
        header: "Params",
        cell: (a) => (
          <Chips
            items={(a.params ?? []).map((p) => {
              const item = p as Record<string, string>;
              return item.name ? `${item.name}(${item.location || "?"})` : "?";
            })}
            mono
          />
        ),
      },
    ],
  },
};
