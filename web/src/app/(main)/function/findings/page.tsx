"use client";

import * as React from "react";

import { FindingRetestDialog } from "@/components/finding-retest-dialog";
import { TablePagination } from "@/components/table-pagination";
import { Card, CardContent } from "@/components/ui/card";
import { Kbd } from "@/components/ui/kbd";
import { Spinner } from "@/components/ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { getLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client";
import { firstNewKey, triageNextPage } from "@/lib/triage";
import type { Finding, FindingStatus } from "@/lib/types";
import { useActiveCompany } from "@/stores/company/company-store";

import { assetPathOf } from "./_components/asset-tree";
import { DeepenDialog, ExportDialog } from "./_components/finding-dialogs";
import { FindingFilterBar } from "./_components/finding-filter-bar";
import {
  DEFAULT_FINDING_FILTERS,
  type FindingFilters,
  type FindingView,
  filtersFromSearch,
  parseSavedFilters,
  serializeFilters,
} from "./_components/finding-filters";
import { type FindingEdit, type FindingReport, findingRowKey } from "./_components/finding-meta";
import { FindingStatTiles } from "./_components/finding-stat-tiles";
import { FindingsAssetView, FindingsGroupedView } from "./_components/finding-views";
import { type FindingRowProps, FindingsTable } from "./_components/findings-table";
import { useImportFilter } from "./_components/import-filter";
import { useLaneFilter } from "./_components/lane-filter";
import { useActiveRetests } from "./_components/use-active-retests";
import {
  findingFingerprint,
  findingGroupKeyOf,
  type GroupFindingsState,
  useFindingData,
} from "./_components/use-finding-data";
import { useFindingMutations } from "./_components/use-finding-mutations";

const FINDING_LIST_PREFERENCE_KEY = "artex_finding_list_preferences";

// useSearchParams (the import_id deep link) must sit under a Suspense boundary for static export.
export default function FindingsPage() {
  return (
    <React.Suspense fallback={null}>
      <FindingsPageInner />
    </React.Suspense>
  );
}

function FindingsPageInner() {
  // The sidebar's company switcher narrows everything below on the server: the
  // flat and grouped lists, the asset tree and the stat tiles all send `company`.
  // Undefined means all companies.
  const company = useActiveCompany();
  // Scanner imports → Past imports link here with ?import_id=N: only that import's findings.
  const { importId, clear: clearImportFilter } = useImportFilter();
  // ?lane=runtime|sast|sca narrows every view, the export and the asset tree to one lane.
  const { lane, setLane } = useLaneFilter();
  const [view, setView] = React.useState<FindingView>("flat");
  const [filters, setFilters] = React.useState<FindingFilters>(DEFAULT_FINDING_FILTERS);
  const [search, setSearch] = React.useState("");
  const [hydrated, setHydrated] = React.useState(false);
  const [expanded, setExpanded] = React.useState<string | null>(null);
  const [selectedIds, setSelectedIds] = React.useState<Set<string>>(() => new Set());
  const [deepenFinding, setDeepenFinding] = React.useState<Finding | null>(null);
  const [retestFinding, setRetestFinding] = React.useState<Finding | null>(null);
  const [exportOpen, setExportOpen] = React.useState(false);
  // openFirst = the filter fingerprint whose first finding should open once its list has loaded:
  // set by the Pending tile, by ?triage=1, and when a verdict empties the open page.
  const [openFirst, setOpenFirst] = React.useState<string | null>(null);

  const setFilter = React.useCallback((patch: Partial<FindingFilters>) => {
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  // Live retests span every client, so a client_read session (no "read") does not poll them.
  const retests = useActiveRetests(useCan("read"));
  const canTriage = useCan("review");

  // Restore saved filters, then let the URL override them (/function/findings?status=pending&triage=1).
  // Runs once on mount; the company, import and lane at mount are the ones the first load uses.
  // biome-ignore lint/correctness/useExhaustiveDependencies: hydration happens once.
  React.useEffect(() => {
    const saved = parseSavedFilters(getLocalStorageValue(FINDING_LIST_PREFERENCE_KEY));
    const fromUrl = filtersFromSearch(window.location.search);
    const { view: nextView, ...restored } = { ...saved, ...fromUrl };
    if (nextView) setView(nextView);
    const next = { ...DEFAULT_FINDING_FILTERS, ...restored };
    setFilters(next);
    if (new URLSearchParams(window.location.search).get("triage") === "1") {
      setOpenFirst(findingFingerprint(next, company, importId, lane));
    }
    setHydrated(true);
  }, []);

  React.useEffect(() => {
    if (hydrated) setLocalStorageValue(FINDING_LIST_PREFERENCE_KEY, serializeFilters(view, filters));
  }, [hydrated, view, filters]);

  React.useEffect(() => {
    const timer = window.setTimeout(() => setFilter({ query: search.trim() }), 300);
    return () => window.clearTimeout(timer);
  }, [search, setFilter]);

  const resetOpenRow = React.useCallback(() => {
    setExpanded(null);
    setEdit(null);
  }, []);

  const data = useFindingData({
    filters,
    company,
    importId,
    lane,
    view,
    hydrated,
    retestFingerprint: retests.fingerprint,
    onReset: resetOpenRow,
  });
  const { stats, statsLoaded, flat } = data;

  // Drop a vuln-class or task filter whose option disappeared from the whole-table stats.
  React.useEffect(() => {
    if (!statsLoaded) return;
    if (filters.vulnclass !== "all" && !stats.vulnclasses.includes(filters.vulnclass)) setFilter({ vulnclass: "all" });
    if (
      filters.task !== "all" &&
      filters.task !== "__unassigned__" &&
      !(stats.tasks ?? []).some((option) => String(option.id) === filters.task)
    ) {
      setFilter({ task: "all" });
    }
  }, [filters.task, filters.vulnclass, setFilter, stats, statsLoaded]);

  // The inline expanded detailed-report cache is keyed by the globally stable row key. report is long-form
  // Markdown the list query doesn't include, so it is fetched once by finding_id on expand; done with empty text = no report yet.
  const [reports, setReports] = React.useState<Record<string, FindingReport>>({});
  // Inline editable buffer: the expanded row's name/category/severity. Only one row expands at a time.
  const [edit, setEdit] = React.useState<FindingEdit | null>(null);

  // openRow opens one finding (initializing its edit buffer and pulling its report once), or collapses with null.
  const openRow = React.useCallback(
    (f: Finding | null) => {
      if (!f) {
        resetOpenRow();
        return;
      }
      const key = findingRowKey(f);
      setExpanded(key);
      setEdit({ name: f.name ?? "", vulnclass: f.vulnclass, severity: f.severity });
      if (!f.finding_id || reports[key]) return;
      const fid = f.finding_id;
      setReports((r) => ({ ...r, [key]: { status: "loading", text: "" } }));
      api
        .getFinding(fid)
        .then((full) => setReports((r) => ({ ...r, [key]: { status: "done", text: full.report ?? "" } })))
        .catch(() => setReports((r) => ({ ...r, [key]: { status: "error", text: "" } })));
    },
    [reports, resetOpenRow],
  );

  const toggleRow = React.useCallback(
    (f: Finding) => openRow(expanded === findingRowKey(f) ? null : f),
    [expanded, openRow],
  );

  const mutations = useFindingMutations({ data, filters, view, selectedIds, setSelectedIds, edit, setExpanded });

  // Open the first finding of a list once it has loaded for the requested filters (flat and asset views).
  React.useEffect(() => {
    if (openFirst === null || view === "grouped") return;
    if (!flat.loaded || flat.fingerprint !== openFirst || flat.loading) return;
    setOpenFirst(null);
    const first = flat.items[0];
    if (first) openRow(first);
  }, [flat, openFirst, openRow, view]);

  // advanceGroup moves grouped-view triage past the end of a group's page: it loads the page that now
  // holds the group's next finding (the same page when the verdict dropped the row, else the next one)
  // and opens its first finding not already seen. `before` is the group's state before the verdict.
  const { loadGroup } = data;
  const advanceGroup = React.useCallback(
    async (key: string, before: GroupFindingsState, dropped: boolean) => {
      const page = triageNextPage(before.page, before.pageSize, before.total, dropped);
      const items = page === null ? null : await loadGroup(key, page, before.pageSize);
      const target = items && firstNewKey(before.items.map(findingRowKey), items.map(findingRowKey));
      openRow(items?.find((item) => findingRowKey(item) === target) ?? null);
    },
    [loadGroup, openRow],
  );

  // triage applies a verdict and moves the queue on: to the next row, or past the end of the page. The flat
  // and asset views reopen the first row of the reloaded page when the verdict emptied the last row out of
  // the filter; the grouped view advances within the group, across its pages.
  const [triageBusy, setTriageBusy] = React.useState(false);
  const triage = React.useCallback(
    async (f: Finding, next: FindingStatus, then: Finding | null) => {
      setTriageBusy(true);
      const groupKey = findingGroupKeyOf(f);
      const groupBefore = data.groupFindings[groupKey];
      try {
        const result = await mutations.updateStatus(f, next);
        if (!result.ok) return;
        if (then) openRow(then);
        else if (view === "grouped" && groupBefore?.loaded) await advanceGroup(groupKey, groupBefore, result.dropped);
        else if (result.dropped) setOpenFirst(findingFingerprint(filters, company, importId, lane));
        else resetOpenRow();
      } finally {
        setTriageBusy(false);
      }
    },
    [advanceGroup, company, data.groupFindings, filters, importId, lane, mutations, openRow, resetOpenRow, view],
  );

  // A tile applies exactly its slice (other filters reset, search and sort kept); Pending also opens the first finding.
  const applySlice = React.useCallback(
    (slice: Partial<FindingFilters>) => {
      const next: FindingFilters = { ...DEFAULT_FINDING_FILTERS, query: filters.query, sort: filters.sort, ...slice };
      setFilters(next);
      if (slice.status === "pending") setOpenFirst(findingFingerprint(next, company, importId, lane));
    },
    [company, filters.query, filters.sort, importId, lane],
  );

  const toggleSelected = React.useCallback((id: string, checked: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  const toggleSelectedPage = React.useCallback((ids: string[], checked: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      for (const id of ids) {
        if (checked) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  }, []);

  // The "current filter" count in the export dialog: the flat and asset views share the flat list state;
  // the grouped view's figure comes from the group API's finding_total.
  const filteredTotal = view === "grouped" ? data.total : flat.total;
  const assetPath = React.useMemo(
    () => (view === "asset" ? assetPathOf(data.assetTree.nodes, data.assetScope) : []),
    [data.assetScope, data.assetTree.nodes, view],
  );

  const rowProps: FindingRowProps = {
    selectedIds,
    onToggleSelected: toggleSelected,
    onToggleSelectedPage: toggleSelectedPage,
    expandedKey: expanded,
    onToggleRow: toggleRow,
    onOpenRow: openRow,
    reports,
    edit,
    onEditChange: setEdit,
    saving: mutations.saving,
    onSave: mutations.saveEdit,
    onStatusChange: (f, next) => void mutations.updateStatus(f, next),
    onReportStateChange: mutations.updateReportState,
    onRetest: setRetestFinding,
    activeRetests: retests.activeRetests,
    onDeepen: setDeepenFinding,
    onDelete: mutations.deleteFinding,
    canTriage,
    triageBusy,
    onTriage: (f, next, then) => void triage(f, next, then),
  };

  // The flat view and the asset view's right side are the same table + pagination, differing only in filter conditions.
  const flatListCard = (
    <Card className="gap-0 py-0">
      <CardContent className="px-0">
        {flat.loading && !flat.loaded ? (
          <div className="flex min-h-36 items-center justify-center">
            <Spinner />
          </div>
        ) : (
          <>
            <FindingsTable items={flat.items} selectAllLabel={t("Select all on this page")} {...rowProps} />
            <TablePagination
              page={data.flatPage}
              pageSize={data.flatPageSize}
              total={flat.total}
              onPageChange={data.setFlatPage}
              onPageSizeChange={(nextSize) => {
                data.setFlatPageSize(nextSize);
                data.setFlatPage(1);
              }}
              pageSizeOptions={[10, 20, 50, 100]}
            />
          </>
        )}
      </CardContent>
    </Card>
  );

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("Findings")}</h1>
          <p className="text-muted-foreground text-sm">{t("Cross-task vulnerability overview")}</p>
        </div>
        <Tabs value={view} onValueChange={(v) => setView(v as FindingView)}>
          <TabsList>
            <TabsTrigger value="flat">{t("All findings")}</TabsTrigger>
            <TabsTrigger value="grouped">{t("By task")}</TabsTrigger>
            <TabsTrigger value="asset">{t("By asset")}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <div className="flex flex-1 flex-col gap-4 md:gap-6">
        <FindingStatTiles stats={stats} filters={filters} onSlice={applySlice} />
        <FindingFilterBar
          filters={filters}
          importId={importId}
          onClearImport={clearImportFilter}
          lane={lane}
          onLaneChange={setLane}
          set={setFilter}
          search={search}
          setSearch={setSearch}
          stats={stats}
          selectedIds={selectedIds}
          bulkBusy={mutations.bulkBusy}
          onBulkStatus={(next) => void mutations.applyBulkStatus(next)}
          onBulkReportState={(next) => void mutations.applyBulkReportState(next)}
          onExport={() => setExportOpen(true)}
        />
        {canTriage && (
          <p className="-mt-2 text-xs text-muted-foreground">
            {t("Open a finding, then press a verdict key; the next finding opens on its own.")} <Kbd>J</Kbd> /{" "}
            <Kbd>K</Kbd> {t("next / previous")} · <Kbd>Esc</Kbd> {t("close")}
          </p>
        )}

        {view === "flat" && flatListCard}
        {view === "asset" && <FindingsAssetView data={data} assetPath={assetPath} list={flatListCard} />}
        {view === "grouped" && <FindingsGroupedView data={data} rowProps={rowProps} />}
      </div>

      {retestFinding?.finding_id ? (
        <FindingRetestDialog
          key={retestFinding.finding_id}
          findingId={retestFinding.finding_id}
          findingName={retestFinding.name || retestFinding.vulnclass || retestFinding.summary}
          onStarted={(retest) => {
            if (retestFinding.finding_id) retests.markStarted(retestFinding.finding_id, retest);
          }}
          onClose={() => setRetestFinding(null)}
        />
      ) : null}

      <DeepenDialog
        finding={deepenFinding}
        onClose={() => setDeepenFinding(null)}
        onDone={(f) => data.refreshAfterMutation(f)}
      />
      <ExportDialog
        open={exportOpen}
        onOpenChange={setExportOpen}
        filters={filters}
        company={company}
        importId={importId}
        lane={lane}
        assetScope={data.activeAssetScope}
        selectedIds={selectedIds}
        filteredTotal={filteredTotal}
      />
    </div>
  );
}
