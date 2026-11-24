"use client";

import { Badge } from "@/components/ui/badge";
import { t } from "@/i18n";
import type { AuthorizationStatus } from "@/lib/governance";
import type { AuthScopeEntry, EngagementAuthorization } from "@/lib/governance-types";

// Presentational pieces of the Authorization tab: status badge, window, and the
// read-only detail of one version. No data fetching here.

export function statusLabel(s: AuthorizationStatus): string {
  switch (s) {
    case "in_force":
      return t("In force");
    case "revoked":
      return t("Revoked");
    case "not_yet":
      return t("Not yet in force");
    case "expired":
      return t("Expired");
    default:
      return t("No authorization");
  }
}

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

function statusVariant(s: AuthorizationStatus): BadgeVariant {
  if (s === "in_force") return "default";
  if (s === "revoked" || s === "expired") return "destructive";
  return "outline";
}

export function StatusBadge({ status }: { status: AuthorizationStatus }) {
  return <Badge variant={statusVariant(status)}>{statusLabel(status)}</Badge>;
}

// fmtTime shows an RFC 3339 time in the browser's local zone.
export function fmtTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

export function fmtWindow(a: Pick<EngagementAuthorization, "not_before" | "not_after">): string {
  return `${fmtTime(a.not_before)} → ${fmtTime(a.not_after)}`;
}

export function TechniqueBadges({ techniques }: { techniques: readonly string[] }) {
  if (techniques.length === 0) {
    return <span className="text-muted-foreground text-xs">{t("all classes allowed")}</span>;
  }
  return (
    <span className="flex flex-wrap gap-1">
      {techniques.map((c) => (
        <Badge key={c} variant="secondary">
          {c}
        </Badge>
      ))}
    </span>
  );
}

export function ScopeList({ scope }: { scope: readonly AuthScopeEntry[] }) {
  if (scope.length === 0) {
    return (
      <span className="text-muted-foreground text-xs">
        {t("empty (covers every target; the target gate still applies)")}
      </span>
    );
  }
  return (
    <ul className="flex flex-col gap-1">
      {scope.map((e) => (
        <li key={`${e.kind}:${e.value}`} className="flex items-center gap-2">
          <Badge variant="outline">{e.kind}</Badge>
          <code className="text-sm">{e.value}</code>
        </li>
      ))}
    </ul>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

// VersionDetails is the read-only view of one immutable version.
export function VersionDetails({ a }: { a: EngagementAuthorization }) {
  return (
    <dl className="grid gap-4 sm:grid-cols-2">
      <Field label={t("Reference")}>{a.reference || "—"}</Field>
      <Field label={t("Authorized by")}>{a.authorized_by || "—"}</Field>
      <Field label={t("Window (local time)")}>{fmtWindow(a)}</Field>
      <Field label={t("Created by")}>
        {a.created_by} · {fmtTime(a.created_at)}
      </Field>
      <Field label={t("Technique classes")}>
        <TechniqueBadges techniques={a.techniques ?? []} />
      </Field>
      <Field label={t("Scope entries")}>
        <ScopeList scope={a.scope ?? []} />
      </Field>
      <div className="sm:col-span-2">
        <Field label={t("Note")}>{a.note ? <span className="whitespace-pre-wrap">{a.note}</span> : "—"}</Field>
      </div>
    </dl>
  );
}
