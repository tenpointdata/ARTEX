"use client";

import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { authorizationStatus } from "@/lib/governance";
import type { EngagementAuthorization } from "@/lib/governance-types";

import { fmtTime, fmtWindow, StatusBadge, statusLabel, TechniqueBadges } from "./authorization-parts";

// History table: every immutable version, newest first. Only the newest version
// is enforced, so only it gets a time-based status; older ones are superseded.
export function AuthorizationHistory({
  versions,
  serverTime,
  selectedId,
  onSelect,
}: {
  versions: readonly EngagementAuthorization[];
  serverTime: string;
  selectedId: number | null;
  onSelect: (id: number) => void;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("Version")}</TableHead>
          <TableHead>{t("Status")}</TableHead>
          <TableHead>{t("Reference")}</TableHead>
          <TableHead>{t("Authorized by")}</TableHead>
          <TableHead>{t("Window (local time)")}</TableHead>
          <TableHead>{t("Technique classes")}</TableHead>
          <TableHead>{t("Scope entries")}</TableHead>
          <TableHead>{t("Created by")}</TableHead>
          <TableHead>{t("Created at")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {versions.map((v, i) => (
          <TableRow
            key={v.id}
            data-state={v.id === selectedId ? "selected" : undefined}
            className="cursor-pointer"
            tabIndex={0}
            aria-selected={v.id === selectedId}
            onClick={() => onSelect(v.id)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(v.id);
              }
            }}
          >
            <TableCell className="font-medium">v{v.version}</TableCell>
            <TableCell>
              {i === 0 ? (
                <StatusBadge status={authorizationStatus(v, serverTime)} />
              ) : (
                <Badge variant="outline">{v.revoked ? statusLabel("revoked") : t("Superseded")}</Badge>
              )}
            </TableCell>
            <TableCell>{v.reference || "—"}</TableCell>
            <TableCell>{v.authorized_by || "—"}</TableCell>
            <TableCell className="whitespace-nowrap text-xs">{fmtWindow(v)}</TableCell>
            <TableCell>
              <TechniqueBadges techniques={v.techniques ?? []} />
            </TableCell>
            <TableCell>{(v.scope ?? []).length}</TableCell>
            <TableCell>{v.created_by}</TableCell>
            <TableCell className="whitespace-nowrap text-xs">{fmtTime(v.created_at)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
