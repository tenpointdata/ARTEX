"use client";

import * as React from "react";

import { toast } from "sonner";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { pushableFindingIds, summarizePush } from "@/lib/integrations";
import { integrationsApi } from "@/lib/integrations-api";
import type { TicketPushResult, TicketTracker } from "@/lib/types";

// useEnabledTrackers loads the trackers a finding can be pushed to. Failures stay
// quiet: without trackers the push control is simply not shown.
export function useEnabledTrackers(): TicketTracker[] {
  const canRead = useCan("read"); // GET /api/integrations/trackers needs read (not client_read)
  const [trackers, setTrackers] = React.useState<TicketTracker[]>([]);
  React.useEffect(() => {
    if (!canRead) return;
    let alive = true;
    integrationsApi
      .trackers()
      .then((list) => alive && setTrackers(list.filter((tr) => tr.enabled)))
      .catch(() => {
        // No trackers (or no access): the push control stays hidden.
      });
    return () => {
      alive = false;
    };
  }, [canRead]);
  return trackers;
}

// PushToTrackerSelect pushes the given findings to the tracker picked: one pick, no
// confirmation, since a push is idempotent (an existing ticket is updated, never duplicated).
export function PushToTrackerSelect({
  findingIds,
  trackers,
  onPushed,
  className = "w-44",
}: {
  findingIds: Iterable<string>;
  trackers: TicketTracker[];
  onPushed?: (results: TicketPushResult[]) => void;
  className?: string;
}) {
  const canOperate = useCan("operate"); // POST /api/integrations/trackers/{id}/push
  const [busy, setBusy] = React.useState(false);
  if (trackers.length === 0 || !canOperate) return null;

  async function push(trackerId: number) {
    const ids = pushableFindingIds(findingIds);
    const tracker = trackers.find((tr) => tr.id === trackerId);
    if (ids.length === 0 || !tracker) return;
    setBusy(true);
    try {
      const results = await integrationsApi.push(trackerId, ids);
      const s = summarizePush(results);
      const msg = t("{name}: {created} created, {updated} updated, {failed} failed", {
        name: tracker.name,
        created: s.created,
        updated: s.updated,
        failed: s.failed,
      });
      if (s.failed > 0) toast.warning(msg, { description: s.errors.join("\n"), duration: 10000 });
      else toast.success(msg);
      onPushed?.(results);
    } catch (e) {
      toast.error(t("Push failed: {error}", { error: (e as Error).message }));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Select
      value=""
      onValueChange={(v) => {
        if (v) void push(Number(v));
      }}
      disabled={busy}
    >
      <SelectTrigger size="sm" className={className} aria-label={t("Push to tracker")}>
        <SelectValue placeholder={busy ? t("Pushing…") : t("Push to tracker")} />
      </SelectTrigger>
      <SelectContent>
        {trackers.map((tr) => (
          <SelectItem key={tr.id} value={String(tr.id)}>
            {tr.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
