"use client";

import * as React from "react";

import { RadioTowerIcon } from "lucide-react";
import { toast } from "sonner";

import { LinkTrafficDialog } from "@/components/link-traffic-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import { useStoredSortPreference } from "@/lib/sort-preference";
import type { TrafficExchange, TrafficHost, TrafficResp } from "@/lib/types";
import { cn } from "@/lib/utils";
import { useActiveCompanyID } from "@/stores/company/company-store";

import { TamperHooksPanel } from "./_components/tamper-hooks-panel";
import { TargetPicker } from "./_components/target-picker";
import { TrafficAdvancedFilters } from "./_components/traffic-advanced-filters";
import { TrafficBulkBar } from "./_components/traffic-bulk-bar";
import { TrafficDeleteDialog } from "./_components/traffic-delete-dialog";
import { TrafficDetailSheet } from "./_components/traffic-detail-sheet";
import { type DeleteMode, fmtBytes, SORT_FIELDS, SORT_STORAGE_KEY, type SortField } from "./_components/traffic-format";
import { HarAlerts, HarButtons, useTrafficHar } from "./_components/traffic-har";
import { type TrafficSelection, TrafficTable } from "./_components/traffic-table";
import { TrafficToolbar } from "./_components/traffic-toolbar";
import { useTrafficFilters } from "./_components/use-traffic-filters";

const REFRESH_MS = 5000;

export default function TrafficPage() {
  const [viewTab, setViewTab] = React.useState<"exchanges" | "hooks">("exchanges");
  // The sidebar's company switcher narrows the list to that company's hosts.
  const companyId = useActiveCompanyID();
  const [checked, setChecked] = React.useState<TrafficSelection>(() => new Map());
  const [linking, setLinking] = React.useState(false);
  const [page, setPage] = React.useState(0);
  const [size, setSize] = React.useState(50);
  const filters = useTrafficFilters();
  const { query } = filters;
  const [sort, setSort] = useStoredSortPreference<SortField>(SORT_STORAGE_KEY, SORT_FIELDS, "ts", "desc");

  const [traffic, setTraffic] = React.useState<TrafficResp | null>(null);
  const [selected, setSelected] = React.useState<TrafficExchange | null>(null);

  const [hosts, setHosts] = React.useState<TrafficHost[]>([]); // target picker
  const [selectedHosts, setSelectedHosts] = React.useState<string[]>([]); // checked in picker
  const [pickerOpen, setPickerOpen] = React.useState(false);

  const [deleteMode, setDeleteMode] = React.useState<DeleteMode | null>(null); // null = dialog closed
  const [deleting, setDeleting] = React.useState(false);
  const [reloadTick, setReloadTick] = React.useState(0); // manual refetch trigger

  const refetch = React.useCallback(() => setReloadTick((n) => n + 1), []);
  const reload = React.useCallback(() => {
    setPage(0);
    refetch();
  }, [refetch]);
  const har = useTrafficHar(query, reload);

  // Any filter/size/sort change resets to the first page.
  // biome-ignore lint/correctness/useExhaustiveDependencies: these values intentionally trigger a page reset.
  React.useEffect(() => {
    setPage(0);
  }, [query, size, sort, companyId]);

  // Load the current page. Auto-refresh only on page 0 (newest) so paging back
  // through history isn't yanked out from under the user.
  // biome-ignore lint/correctness/useExhaustiveDependencies: reloadTick is an explicit manual-refetch trigger.
  React.useEffect(() => {
    let alive = true;
    const load = () => {
      api
        .traffic(page, size, query.host, query.method, query.q, {
          body: query.body,
          path: query.path,
          status: query.status,
          rewritten: query.rewritten,
          respMin: query.respMin,
          respMax: query.respMax,
          sort: sort.field,
          order: sort.direction,
          company: companyId,
        })
        .then((r) => {
          if (alive) setTraffic(r);
        })
        .catch(() => {
          // Keep the last successful snapshot during transient refresh failures.
        });
      api
        .trafficHosts()
        .then((r) => {
          if (alive) setHosts(r.hosts ?? []);
        })
        .catch(() => {
          // Keep the last successful host list during transient refresh failures.
        });
    };
    load();
    const stopPolling = setVisibleInterval(() => {
      if (page !== 0) return; // only auto-refresh the newest page
      load();
    }, REFRESH_MS);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [page, size, query, sort, reloadTick, companyId]);

  // `reclaimed` only comes back from the full purge; the host-scoped deletions
  // report the row count alone.
  const requestDelete = (mode: DeleteMode): Promise<{ deleted: number; reclaimed?: number }> => {
    if (mode === "all") return api.trafficDeleteAll();
    if (mode === "selected") return api.trafficDeleteHosts(selectedHosts);
    if (mode === "exchanges") return api.trafficDeleteIds([...checked.keys()]);
    return api.trafficDeleteHost(query.host);
  };

  const confirmDelete = () => {
    if (!deleteMode) return;
    setDeleting(true);
    const mode = deleteMode;
    requestDelete(mode)
      .then((r) => {
        setDeleteMode(null);
        setSelected(null);
        setChecked(new Map());
        if (mode === "selected" || mode === "all") {
          setSelectedHosts([]);
          setPickerOpen(false);
        }
        if (mode === "exchanges") toast.success(t("Deleted {count} exchanges", { count: r.deleted }));
        if (mode === "all") {
          // Reclaimed space is the whole point of compacting an emptied index, so say so.
          const reclaimed = r.reclaimed ?? 0;
          toast.success(
            reclaimed > 0
              ? t("Purged {count} traffic records, freeing {size} of storage", {
                  count: r.deleted,
                  size: fmtBytes(reclaimed),
                })
              : t("Purged {count} traffic records", { count: r.deleted }),
          );
        }
        reload();
      })
      .catch((e) => {
        // Keep the confirmation open so the user can retry a failed deletion.
        const error = (e as Error).message;
        if (mode === "all") toast.error(t("Purge failed: {error}", { error }));
        else toast.error(t("Delete failed: {error}", { error }));
      })
      .finally(() => setDeleting(false));
  };

  // Toggle direction when re-clicking the active column, else sort the new column
  // newest/largest-first.
  const toggleSort = (field: SortField) =>
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === "asc" ? "desc" : "asc" }
        : { field, direction: "desc" },
    );

  const exchanges = React.useMemo(() => traffic?.exchanges ?? [], [traffic]);
  const total = traffic?.total ?? exchanges.length;
  const pageCount = Math.max(1, Math.ceil(total / size));
  const rangeStart = total === 0 ? 0 : page * size + 1;
  const rangeEnd = page * size + exchanges.length;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("Traffic")}</h1>
          <p className="text-muted-foreground text-sm">{t("Global recording proxy · all HTTP traffic")}</p>
        </div>
        <div className="flex items-center gap-4 text-sm">
          <span
            className={cn(
              "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium",
              traffic?.enabled
                ? "border-emerald-500/20 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                : "border-transparent bg-muted text-muted-foreground",
            )}
          >
            <RadioTowerIcon className="size-3.5" />
            {traffic?.enabled ? t("Recording") : t("Disabled")}
          </span>
          {traffic?.proxy && <span className="font-mono text-xs text-muted-foreground">{traffic.proxy}</span>}
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("{count} records", { count: traffic?.count ?? 0 })}
          </span>
          <HarButtons har={har} hasRecords={Boolean(traffic?.count)} />
        </div>
      </div>

      <Tabs
        value={viewTab}
        onValueChange={(v) => setViewTab(v as "exchanges" | "hooks")}
        className="flex min-h-0 flex-1 flex-col gap-4"
      >
        <TabsList className="w-fit">
          <TabsTrigger value="exchanges">{t("Exchanges")}</TabsTrigger>
          <TabsTrigger value="hooks">{t("Hooks")}</TabsTrigger>
        </TabsList>

        <TabsContent value="hooks" className="mt-0 flex min-h-0 flex-1 flex-col gap-4">
          <TamperHooksPanel recentExchanges={exchanges} />
        </TabsContent>

        <TabsContent value="exchanges" className="mt-0 flex min-h-0 flex-1 flex-col gap-4">
          <HarAlerts har={har} />

          <TrafficToolbar
            f={filters}
            picker={
              <TargetPicker
                hosts={hosts}
                selectedHosts={selectedHosts}
                onSelectedHostsChange={setSelectedHosts}
                open={pickerOpen}
                onOpenChange={setPickerOpen}
                onDeleteSelected={() => {
                  setDeleteMode("selected");
                  setPickerOpen(false);
                }}
              />
            }
            size={size}
            onSizeChange={setSize}
            page={page}
            pageCount={pageCount}
            rangeStart={rangeStart}
            rangeEnd={rangeEnd}
            total={total}
            onPageChange={setPage}
            recordCount={traffic?.count ?? 0}
            deleting={deleting}
            onDeleteTarget={() => setDeleteMode("filter")}
            onPurgeAll={() => setDeleteMode("all")}
          />

          <TrafficAdvancedFilters f={filters} />

          <TrafficBulkBar
            selection={checked}
            onClear={() => setChecked(new Map())}
            onLink={() => setLinking(true)}
            onDeleteSelected={() => setDeleteMode("exchanges")}
            onDeleteTargets={(targets) => {
              setSelectedHosts(targets);
              setDeleteMode("selected");
            }}
          />

          <TrafficTable
            exchanges={exchanges}
            loading={traffic === null}
            checked={checked}
            onCheckedChange={setChecked}
            openId={selected?.id}
            onOpen={setSelected}
            sort={sort}
            onSort={toggleSort}
          />

          {linking ? (
            <LinkTrafficDialog
              trafficIds={[...checked.keys()]}
              onClose={() => setLinking(false)}
              onBound={() => setChecked(new Map())}
            />
          ) : null}
          <TrafficDetailSheet selected={selected} onClose={() => setSelected(null)} onRecorded={refetch} />

          <TrafficDeleteDialog
            mode={deleteMode}
            deleting={deleting}
            selectedHosts={selectedHosts}
            exchangeCount={checked.size}
            hostFilter={query.host}
            recordCount={traffic?.count ?? 0}
            onCancel={() => setDeleteMode(null)}
            onConfirm={confirmDelete}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
