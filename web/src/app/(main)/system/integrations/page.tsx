"use client";

import * as React from "react";

import Link from "next/link";

import { FileUpIcon, PencilIcon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { evidenceEnabled, summarizeSync, trackerDestination } from "@/lib/integrations";
import { integrationsApi } from "@/lib/integrations-api";
import type { TicketSyncResult, TicketTracker, TicketTrackerMeta } from "@/lib/types";

import { SyncResultsTable } from "./_components/sync-results";
import { trackerKindLabel } from "./_components/tracker-fields";
import { TrackerSheet } from "./_components/tracker-sheet";

// Settings → Integrations: ticket trackers that findings are pushed to and synced with.
// Pushing happens from the findings table (bulk action) or a finding's detail page.
// Tracker CRUD needs admin and Sync needs operate (server/rbac_policy.go); a role
// without them sees the list read-only.
export default function IntegrationsPage() {
  const isAdmin = useCan("admin");
  const canOperate = useCan("operate");
  const hasActions = isAdmin || canOperate;
  const [meta, setMeta] = React.useState<TicketTrackerMeta | null>(null);
  const [trackers, setTrackers] = React.useState<TicketTracker[]>([]);
  const [loaded, setLoaded] = React.useState(false);
  const [sheetOpen, setSheetOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<TicketTracker | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<TicketTracker | null>(null);
  const [busyId, setBusyId] = React.useState<number | null>(null);
  const [lastSync, setLastSync] = React.useState<{
    tracker: TicketTracker;
    results: TicketSyncResult[];
  } | null>(null);

  const load = React.useCallback(() => {
    integrationsApi
      .meta()
      .then(setMeta)
      .catch((e) =>
        toast.error(
          t("Failed to load tracker types: {error}", {
            error: (e as Error).message,
          }),
        ),
      );
    integrationsApi
      .trackers()
      .then(setTrackers)
      .catch((e) =>
        toast.error(
          t("Failed to load trackers: {error}", {
            error: (e as Error).message,
          }),
        ),
      )
      .finally(() => setLoaded(true));
  }, []);
  React.useEffect(load, [load]);

  function openEdit(tr: TicketTracker | null) {
    setEditing(tr);
    setSheetOpen(true);
  }

  async function toggleEnabled(tr: TicketTracker) {
    try {
      const saved = await integrationsApi.updateTracker(tr.id, {
        enabled: !tr.enabled,
      });
      setTrackers((list) => list.map((x) => (x.id === saved.id ? saved : x)));
    } catch (e) {
      toast.error(t("Update failed: {error}", { error: (e as Error).message }));
    }
  }

  async function sync(tr: TicketTracker) {
    setBusyId(tr.id);
    try {
      const results = await integrationsApi.sync(tr.id);
      const s = summarizeSync(results);
      setLastSync({ tracker: tr, results });
      const msg = t("{name}: {pulled} pulled, {pushed} pushed, {unchanged} unchanged, {error} failed", {
        name: tr.name,
        ...s,
      });
      if (s.error > 0) toast.warning(msg);
      else toast.success(msg);
    } catch (e) {
      toast.error(t("Sync failed: {error}", { error: (e as Error).message }));
    } finally {
      setBusyId(null);
    }
  }

  async function remove() {
    if (!deleteTarget) return;
    try {
      await integrationsApi.deleteTracker(deleteTarget.id);
      toast.success(t("Deleted: {name}", { name: deleteTarget.name }));
      setTrackers((list) => list.filter((x) => x.id !== deleteTarget.id));
      if (lastSync?.tracker.id === deleteTarget.id) setLastSync(null);
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
    } finally {
      setDeleteTarget(null);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="mr-auto">
          <h1 className="text-xl font-semibold">{t("Integrations")}</h1>
          <p className="text-muted-foreground text-sm">
            {t(
              "Ticket trackers that findings are pushed to. Status syncs both ways; when both sides changed, the tracker wins.",
            )}
          </p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <Link href="/system/integrations/imports">
            <FileUpIcon /> {t("Scanner imports")}
          </Link>
        </Button>
        {isAdmin && (
          <Button size="sm" onClick={() => openEdit(null)}>
            <PlusIcon /> {t("Add tracker")}
          </Button>
        )}
      </div>

      <Card className="gap-0 py-0">
        <CardContent className="px-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Name")}</TableHead>
                <TableHead>{t("Type")}</TableHead>
                <TableHead>{t("Destination")}</TableHead>
                <TableHead>{t("Evidence")}</TableHead>
                <TableHead>{t("Enabled")}</TableHead>
                {hasActions && <TableHead className="text-right">{t("Actions")}</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {trackers.length === 0 && (
                <TableRow>
                  <TableCell colSpan={hasActions ? 6 : 5} className="text-muted-foreground py-8 text-center text-sm">
                    {loaded ? t("No trackers yet. Add one to push findings as tickets.") : t("Loading…")}
                  </TableCell>
                </TableRow>
              )}
              {trackers.map((tr) => (
                <TableRow key={tr.id}>
                  <TableCell className="font-medium">
                    {isAdmin ? (
                      <button type="button" className="hover:underline" onClick={() => openEdit(tr)}>
                        {tr.name}
                      </button>
                    ) : (
                      tr.name
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{trackerKindLabel(tr.kind)}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{trackerDestination(tr) || "—"}</TableCell>
                  <TableCell className="text-sm">{evidenceEnabled(tr.config) ? t("Included") : t("Off")}</TableCell>
                  <TableCell>
                    {isAdmin ? (
                      <Switch
                        checked={tr.enabled}
                        onCheckedChange={() => void toggleEnabled(tr)}
                        aria-label={t("Enable {name}", { name: tr.name })}
                      />
                    ) : (
                      <span className="text-sm">{tr.enabled ? t("On") : t("Off")}</span>
                    )}
                  </TableCell>
                  {hasActions && (
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {canOperate && (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={!tr.enabled || busyId === tr.id}
                            onClick={() => void sync(tr)}
                          >
                            <RefreshCwIcon className={busyId === tr.id ? "animate-spin" : ""} /> {t("Sync")}
                          </Button>
                        )}
                        {isAdmin && (
                          <>
                            <Button size="icon-sm" variant="ghost" aria-label={t("Edit")} onClick={() => openEdit(tr)}>
                              <PencilIcon />
                            </Button>
                            <Button
                              size="icon-sm"
                              variant="ghost"
                              aria-label={t("Delete")}
                              onClick={() => setDeleteTarget(tr)}
                            >
                              <Trash2Icon className="text-destructive" />
                            </Button>
                          </>
                        )}
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {lastSync && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t("Last sync: {name}", { name: lastSync.tracker.name })}</CardTitle>
          </CardHeader>
          <CardContent className="px-0">
            <SyncResultsTable results={lastSync.results} />
          </CardContent>
        </Card>
      )}

      {isAdmin && (
        <TrackerSheet
          open={sheetOpen}
          onOpenChange={setSheetOpen}
          meta={meta}
          editing={editing}
          onSaved={(saved) =>
            setTrackers((list) =>
              list.some((x) => x.id === saved.id) ? list.map((x) => (x.id === saved.id ? saved : x)) : [...list, saved],
            )
          }
        />
      )}

      <AlertDialog open={deleteTarget !== null} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("Delete tracker?")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('This removes "{name}" and its links to tickets. The tickets themselves stay in the tracker.', {
                name: deleteTarget?.name ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void remove();
              }}
            >
              {t("Delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
