"use client";

import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { ActiveFindingRetest, FindingRetest } from "@/lib/types";

// useActiveRetests polls the running retests across all findings (one light request for every row and view),
// waiting for each round to finish before the next. Live retests span every client, so a session
// without the read permission does not poll them.
export function useActiveRetests(enabled: boolean) {
  const [activeRetests, setActiveRetests] = React.useState<Record<string, ActiveFindingRetest>>({});
  const version = React.useRef(0);

  React.useEffect(() => {
    if (!enabled) return;
    let disposed = false;
    let failed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function refresh() {
      const at = version.current;
      try {
        const rows = await api.activeFindingRetests();
        if (disposed || at !== version.current) return;
        setActiveRetests(Object.fromEntries(rows.map((item) => [item.finding_id, item])));
        failed = false;
      } catch (error) {
        if (!disposed && !failed)
          toast.error(t("Failed to load retest status: {error}", { error: (error as Error).message }));
        failed = true;
      } finally {
        if (!disposed) timer = setTimeout(() => void refresh(), 3000);
      }
    }
    void refresh();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [enabled]);

  // markStarted shows a just-started retest at once, without waiting for the next poll.
  const markStarted = React.useCallback((findingId: string, retest: FindingRetest) => {
    if (retest.conversation_id == null || !["pending", "running"].includes(retest.status)) return;
    version.current++;
    const active: ActiveFindingRetest = {
      id: retest.id,
      finding_id: findingId,
      conversation_id: retest.conversation_id,
      status: retest.status === "pending" ? "pending" : "running",
    };
    setActiveRetests((current) => ({ ...current, [findingId]: active }));
  }, []);

  const fingerprint = Object.values(activeRetests)
    .map((item) => item.id)
    .join(",");
  return { activeRetests, markStarted, fingerprint };
}
