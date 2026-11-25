"use client";

import type * as React from "react";

import { KeyRoundIcon, Trash2Icon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { TableCell, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { httpHref } from "@/lib/http-href";
import type { Asset } from "@/lib/types";
import { cn } from "@/lib/utils";

import { Chips } from "./asset-table";

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
        "inline-block rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold leading-none",
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

/** Column headers per asset type (first/last blank = checkbox / row-delete columns). */
export const ASSET_COLS: Record<string, string[]> = {
  root_domain: ["", "Domain", "ICP filing", "Company", ""],
  ip: ["", "IP", "C-segment", "Bound domains", "Open ports", ""],
  subdomain: ["", "Domain", "Root domain", "Record type", "Record value", ""],
  app: ["", "App name", "Bundle ID", "Category", "ICP filing", ""],
  service: ["", "Service", "Domain", "IP", "Port", "Status", "Title", "Fingerprint", "Auth", ""],
  endpoint: ["", "Method", "Full URL", "Params", ""],
};

function assetLabel(a: Asset, type: string): string | number {
  switch (type) {
    case "ip":
      return a.ip || a.id;
    case "app":
      return a.app_name || a.id;
    case "service":
    case "endpoint":
      return a.url || a.id;
    default:
      return a.domain || a.id;
  }
}

function ServiceCells({ a }: { a: Asset }) {
  const isHttp = a.service_type === "http";
  const svc = a.service_name || (isHttp ? "http" : "") || a.service_type || "";
  let domainCell: React.ReactNode = "—";
  const href = isHttp ? httpHref(a.url) : undefined;
  if (href) {
    domainCell = (
      <a href={href} target="_blank" rel="noreferrer" className="text-blue-600 hover:underline dark:text-blue-400">
        {a.domain || a.url}
      </a>
    );
  } else if (a.domain || a.url) {
    domainCell = a.domain || a.url;
  }
  return (
    <>
      <TableCell className="text-xs">
        <Badge variant={isHttp ? "default" : "secondary"} className="font-mono text-[10px]">
          {svc || "—"}
        </Badge>
      </TableCell>
      <TableCell className="max-w-[14rem] truncate font-mono text-xs" title={a.domain || a.url}>
        {domainCell}
      </TableCell>
      <TableCell className="font-mono text-xs">{a.ip || "—"}</TableCell>
      <TableCell className="font-mono text-xs tabular-nums">{a.port || "—"}</TableCell>
      <TableCell>
        {a.status_code != null ? (
          <span className={cn("font-mono text-xs font-semibold tabular-nums", statusTone(a.status_code))}>
            {a.status_code}
          </span>
        ) : (
          "—"
        )}
      </TableCell>
      <TableCell className="max-w-[12rem] truncate text-xs" title={a.page_title}>
        {a.page_title || "—"}
      </TableCell>
      <TableCell>
        <Chips items={a.technologies ?? []} />
      </TableCell>
      <TableCell>
        {(a.auth ?? []).length === 0 ? (
          <span className="text-xs text-muted-foreground">—</span>
        ) : (
          (a.auth ?? []).map((authItem, i) => {
            const item = authItem as Record<string, string>;
            const label = item.type || item.username || t("Auth");
            return (
              <span key={i} className="inline-flex items-center gap-1 text-[11px]">
                <KeyRoundIcon className="size-3 text-muted-foreground" />
                <span className="font-mono">{label}</span>
              </span>
            );
          })
        )}
      </TableCell>
    </>
  );
}

function TypeCells({ a, type, companyName }: { a: Asset; type: string; companyName: (id?: number) => string }) {
  switch (type) {
    case "root_domain":
      return (
        <>
          <TableCell className="font-mono text-xs font-medium">{a.domain}</TableCell>
          <TableCell className="text-xs">{a.icp || "—"}</TableCell>
          <TableCell className="text-xs">
            {companyName(a.company_id) || <span className="text-muted-foreground">{t("Unassigned")}</span>}
          </TableCell>
        </>
      );
    case "ip":
      return (
        <>
          <TableCell className="font-mono text-xs font-medium">{a.ip}</TableCell>
          <TableCell className="font-mono text-xs">{a.c_segment || "—"}</TableCell>
          <TableCell>
            <Chips items={a.bound_domains ?? []} mono />
          </TableCell>
          <TableCell>
            <Chips
              items={(a.open_ports ?? []).map((p) => (p.service ? `${p.port}/${p.service}` : String(p.port)))}
              mono
            />
          </TableCell>
        </>
      );
    case "subdomain":
      return (
        <>
          <TableCell className="font-mono text-xs font-medium">{a.domain}</TableCell>
          <TableCell className="font-mono text-xs">{a.root_domain || "—"}</TableCell>
          <TableCell className="text-xs">{a.record_type || "—"}</TableCell>
          <TableCell className="max-w-xs truncate font-mono text-xs">
            {(Array.isArray(a.record_value) ? a.record_value.join(", ") : a.record_value) || "—"}
          </TableCell>
        </>
      );
    case "app":
      return (
        <>
          <TableCell className="text-xs font-medium">{a.app_name || "—"}</TableCell>
          <TableCell className="font-mono text-xs">{a.bundle_id || "—"}</TableCell>
          <TableCell className="text-xs">{a.category || "—"}</TableCell>
          <TableCell className="text-xs">{a.app_icp || "—"}</TableCell>
        </>
      );
    case "service":
      return <ServiceCells a={a} />;
    default:
      return (
        <>
          <TableCell className="w-16">
            <MethodBadge method={a.method || ""} />
          </TableCell>
          <TableCell className="max-w-sm truncate font-mono text-xs" title={a.url}>
            {a.url || "—"}
          </TableCell>
          <TableCell>
            <Chips
              items={(a.params ?? []).map((p) => {
                const item = p as Record<string, string>;
                return item.name ? `${item.name}(${item.location || "?"})` : "?";
              })}
              mono
            />
          </TableCell>
        </>
      );
  }
}

/** One selectable, deletable asset row; the middle cells depend on the asset type. */
export function AssetRow({
  a,
  type,
  selected,
  onToggle,
  onDelete,
  companyName,
}: {
  a: Asset;
  type: string;
  selected: boolean;
  onToggle: (id: number) => void;
  onDelete: (ids: number[]) => void;
  companyName: (id?: number) => string;
}) {
  return (
    <TableRow className={selected ? "bg-muted/40" : undefined} data-state={selected ? "selected" : undefined}>
      <TableCell className="w-8 pr-0">
        <Checkbox checked={selected} onCheckedChange={() => onToggle(a.id)} />
      </TableCell>
      <TypeCells a={a} type={type} companyName={companyName} />
      <TableCell className="w-8 pl-0">
        <Button
          variant="ghost"
          size="icon"
          className="size-7 text-muted-foreground hover:text-destructive"
          onClick={() => onDelete([a.id])}
          aria-label={t("Delete asset {name}", { name: assetLabel(a, type) })}
        >
          <Trash2Icon className="size-3.5" />
        </Button>
      </TableCell>
    </TableRow>
  );
}
