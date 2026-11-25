"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowUpRightIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { httpHref } from "@/lib/http-href";
import { codeLocation } from "@/lib/integrations";
import { integrationsApi } from "@/lib/integrations-api";
import type {
  FindingImportSource,
  FindingIntegrations,
  FindingTicket,
  TicketTracker,
} from "@/lib/types";

import { LaneBadge } from "../_components/lane-badge";
import { PushToTrackerSelect, useCanPush } from "../_components/push-to-tracker";

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(intlLocale);
}

// FindingIntegrationsPanel shows a finding's tracker tickets and, for an imported
// finding, where it came from. It renders nothing when there is nothing to show
// and no tracker to push to. It needs read (server/rbac_policy.go), so a client_read
// session neither fetches nor renders it.
export function FindingIntegrationsPanel({ findingId }: { findingId: string }) {
  if (!useCan("read")) return null;
  return <FindingIntegrationsCard findingId={findingId} />;
}

function FindingIntegrationsCard({ findingId }: { findingId: string }) {
  const canPush = useCanPush();
  const [data, setData] = React.useState<FindingIntegrations | null>(null);
  const [trackers, setTrackers] = React.useState<TicketTracker[]>([]);

  const load = React.useCallback(() => {
    integrationsApi
      .findingIntegrations(findingId)
      .then(setData)
      .catch(() => setData(null));
  }, [findingId]);
  React.useEffect(() => {
    load();
    integrationsApi
      .trackers()
      .then(setTrackers)
      .catch(() => {
        // Without the tracker list, tickets show the tracker id instead of its name.
      });
  }, [load]);

  const tickets = data?.tickets ?? [];
  const source = data?.import_source ?? null;
  const enabled = canPush ? trackers.filter((tr) => tr.enabled) : [];
  if (tickets.length === 0 && !source && enabled.length === 0) return null;
  const trackerName = (id: number) =>
    trackers.find((tr) => tr.id === id)?.name ?? `#${id}`;

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle className="text-sm">{t("Tickets")}</CardTitle>
        <PushToTrackerSelect
          findingIds={[findingId]}
          trackers={enabled}
          onPushed={load}
        />
      </CardHeader>
      <CardContent className="grid gap-4">
        {tickets.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("Not pushed to a tracker yet.")}
          </p>
        ) : (
          <TicketsTable tickets={tickets} trackerName={trackerName} />
        )}
        {source && <ImportSource source={source} />}
      </CardContent>
    </Card>
  );
}

function TicketsTable({
  tickets,
  trackerName,
}: {
  tickets: FindingTicket[];
  trackerName: (id: number) => string;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("Tracker")}</TableHead>
          <TableHead>{t("Ticket")}</TableHead>
          <TableHead>{t("Remote state")}</TableHead>
          <TableHead>{t("Last synced")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tickets.map((tk) => {
          const href = httpHref(tk.external_url);
          return (
            <TableRow key={tk.id}>
              <TableCell className="text-sm">
                {trackerName(tk.tracker_id)}
              </TableCell>
              <TableCell className="font-mono text-xs">
                {href ? (
                  <a
                    className="inline-flex items-center gap-1 hover:underline"
                    href={href}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {tk.external_id}
                    <ArrowUpRightIcon className="size-3" />
                  </a>
                ) : (
                  tk.external_id
                )}
              </TableCell>
              <TableCell>
                <Badge variant="outline">
                  {tk.remote_open ? t("Open") : t("Closed")}
                  {!tk.remote_open && tk.remote_resolution
                    ? ` · ${tk.remote_resolution}`
                    : ""}
                </Badge>
                {tk.last_error && (
                  <p className="text-destructive mt-1 text-xs">
                    {tk.last_error}
                  </p>
                )}
              </TableCell>
              <TableCell className="text-xs whitespace-nowrap">
                {fmtTime(tk.synced_at)}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function ImportSource({ source }: { source: FindingImportSource }) {
  return (
    <div className="grid gap-1 rounded-md border p-3 text-sm">
      <p className="font-medium">{t("Imported from a scanner")}</p>
      <p className="text-muted-foreground text-xs">
        {t(
          "A scanner claim: it stays unverified until it is retested, and re-imports never change it.",
        )}
      </p>
      <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
        <dt className="text-muted-foreground">{t("Tool")}</dt>
        <dd>{source.tool}</dd>
        {source.lane && (
          <>
            <dt className="text-muted-foreground">{t("Lane")}</dt>
            <dd>
              <LaneBadge lane={source.lane} />
            </dd>
          </>
        )}
        {codeLocation(source) && (
          <>
            <dt className="text-muted-foreground">{t("Location")}</dt>
            <dd className="font-mono break-all">{codeLocation(source)}</dd>
          </>
        )}
        <dt className="text-muted-foreground">{t("Rule")}</dt>
        <dd className="font-mono break-all">{source.rule_id || "—"}</dd>
        <dt className="text-muted-foreground">{t("Import")}</dt>
        <dd>
          {source.import_id ? (
            <Link
              className="hover:underline"
              href={`/system/integrations/imports#import-${source.import_id}`}
            >
              #{source.import_id}
            </Link>
          ) : (
            t("deleted")
          )}
        </dd>
        <dt className="text-muted-foreground">{t("Record")}</dt>
        <dd className="font-mono break-all">{source.record || "—"}</dd>
        <dt className="text-muted-foreground">{t("Imported")}</dt>
        <dd>{fmtTime(source.created_at)}</dd>
      </dl>
    </div>
  );
}
