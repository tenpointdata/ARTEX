"use client";

import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { Finding, FindingAssetNode, FindingGroup, FindingLane, FindingStats } from "@/lib/types";

import type { FindingFilters, FindingView } from "./finding-filters";
import { UNASSIGNED_TASK } from "./finding-meta";

// One-shot snapshot of the asset tree. Unlike the other two views, the asset view doesn't poll:
// it re-queries on view entry, filter changes, or finding mutations made on this page.
export interface AssetTreeState {
  nodes: FindingAssetNode[];
  findingTotal: number;
  truncated: boolean;
  droppedKinds: string[];
  loaded: boolean;
  loading: boolean;
}

const EMPTY_ASSET_TREE: AssetTreeState = {
  nodes: [],
  findingTotal: 0,
  truncated: false,
  droppedKinds: [],
  loaded: false,
  loading: false,
};

// In the grouped view, every expanded task group carries its own pagination state, independent of the others.
export interface GroupFindingsState {
  items: Finding[];
  total: number;
  page: number;
  pageSize: number;
  loaded: boolean;
  loading: boolean;
}

// The flat view's page number lives in its own state (not inside the snapshot), so a filter change resets it and triggers a reload.
export interface FlatFindingsState {
  items: Finding[];
  /** The filter fingerprint (see findingFingerprint) the items were loaded for; "" before the first load. */
  fingerprint: string;
  total: number;
  loaded: boolean;
  loading: boolean;
}

const EMPTY_FLAT_STATE: FlatFindingsState = { items: [], fingerprint: "", total: 0, loaded: false, loading: false };

/** Identifies one filter set under one company, scanner-import and lane filter; a change resets every view. */
export function findingFingerprint(
  filters: FindingFilters,
  company: string | undefined,
  importId: string,
  lane: FindingLane | "",
): string {
  return JSON.stringify([filters, company, importId, lane]);
}

const EMPTY_STATS: FindingStats = {
  total: 0,
  pending: 0,
  submitted: 0,
  accepted: 0,
  critical: 0,
  high: 0,
  medium: 0,
  low: 0,
  vulnclasses: [],
  tasks: [],
};

export function findingGroupKey(group: FindingGroup) {
  return group.task_id === null ? UNASSIGNED_TASK : String(group.task_id);
}

interface FindingDataOptions {
  filters: FindingFilters;
  company: string | undefined;
  /** Scanner import id from the ?import_id deep link: only that import's findings. Empty = no import filter. */
  importId: string;
  /** Lane from the ?lane deep link (runtime, sast, sca). Empty = every lane. */
  lane: FindingLane | "";
  view: FindingView;
  /** No request is made until saved preferences are applied, so the defaults never fetch for nothing. */
  hydrated: boolean;
  /** Changes when a retest finishes; a finished retest may change triage status. */
  retestFingerprint: string;
  /** Called when a filter change resets the lists (e.g. to collapse the open row). */
  onReset: () => void;
}

// useFindingData owns the findings page's three list views (flat, grouped by task, by asset), their
// pagination, polling and the whole-table stats, all narrowed by one filter set and the active company.
export function useFindingData({
  filters,
  company,
  importId,
  lane,
  view,
  hydrated,
  retestFingerprint,
  onReset,
}: FindingDataOptions) {
  const [flat, setFlat] = React.useState<FlatFindingsState>(EMPTY_FLAT_STATE);
  const [flatPage, setFlatPage] = React.useState(1);
  const [flatPageSize, setFlatPageSize] = React.useState(20);
  const [assetTree, setAssetTree] = React.useState<AssetTreeState>(EMPTY_ASSET_TREE);
  const [assetScope, setAssetScope] = React.useState<string | null>(null);
  const [groups, setGroups] = React.useState<FindingGroup[]>([]);
  const [groupTotal, setGroupTotal] = React.useState(0);
  const [expandedGroups, setExpandedGroups] = React.useState<Set<string>>(() => new Set());
  const [groupFindings, setGroupFindings] = React.useState<Record<string, GroupFindingsState>>({});
  const [total, setTotal] = React.useState(0);
  const [stats, setStats] = React.useState<FindingStats>(EMPTY_STATS);
  const [statsLoaded, setStatsLoaded] = React.useState(false);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(10);

  const filterFingerprint = findingFingerprint(filters, company, importId, lane);
  const activeFilterFingerprint = React.useRef(filterFingerprint);
  activeFilterFingerprint.current = filterFingerprint;
  const onResetRef = React.useRef(onReset);
  onResetRef.current = onReset;

  // setFindings rewrites the same finding in every view cache at once, so switching views never shows stale state.
  const setFindings = React.useCallback((update: (current: Finding[]) => Finding[]) => {
    setFlat((current) => ({ ...current, items: update(current.items) }));
    setGroupFindings((current) => {
      const next: Record<string, GroupFindingsState> = {};
      for (const [key, state] of Object.entries(current)) next[key] = { ...state, items: update(state.items) };
      return next;
    });
  }, []);

  // dropOne removes a finding the current filter no longer matches, keeping the totals in step.
  const dropOne = React.useCallback(
    (matches: (finding: Finding) => boolean) => {
      setFindings((cur) => cur.filter((x) => !matches(x)));
      setTotal((n) => Math.max(0, n - 1));
      setFlat((cur) => ({ ...cur, total: Math.max(0, cur.total - 1) }));
    },
    [setFindings],
  );

  const flatRequest = React.useRef(0);
  const assetTreeRequest = React.useRef(0);
  const groupRequests = React.useRef<Record<string, number>>({});
  const groupsRequest = React.useRef(0);
  const flatStateRef = React.useRef(flat);
  const expandedGroupsRef = React.useRef(expandedGroups);
  const groupFindingsRef = React.useRef(groupFindings);
  const visibleGroupKeysRef = React.useRef<Set<string>>(new Set());
  flatStateRef.current = flat;
  expandedGroupsRef.current = expandedGroups;
  groupFindingsRef.current = groupFindings;
  visibleGroupKeysRef.current = new Set(groups.map(findingGroupKey));

  // A company switch drops the picked asset node: the node may belong to the
  // company we just left, and the tree is about to be reloaded under the new one.
  React.useEffect(() => {
    void company;
    setAssetScope(null);
  }, [company]);

  // The asset view's right-hand list = the flat list + a filter for the selected subtree, so both views share one list state.
  const activeAssetScope = view === "asset" ? assetScope : null;

  // loadFlat fetches the flat view's current page; the task filter goes to the backend, sharing the same filter set as the grouped view.
  const loadFlat = React.useCallback(async () => {
    const requestFilter = filterFingerprint;
    if (activeFilterFingerprint.current !== requestFilter) return;
    const request = ++flatRequest.current;
    setFlat((current) => ({ ...current, loading: true }));
    try {
      const result = await api.findingsPage({
        page: flatPage,
        pageSize: flatPageSize,
        ...filters,
        assetScope: activeAssetScope ?? undefined,
        company,
        importId,
        lane,
      });
      if (request !== flatRequest.current || activeFilterFingerprint.current !== requestFilter) return;
      setFlat({ items: result.items, fingerprint: requestFilter, total: result.total, loaded: true, loading: false });
    } catch {
      if (request !== flatRequest.current || activeFilterFingerprint.current !== requestFilter) return;
      // Polling keeps the last successful snapshot visible.
      setFlat((current) => ({ ...current, loading: false }));
    }
  }, [activeAssetScope, company, importId, lane, filterFingerprint, filters, flatPage, flatPageSize]);

  // loadAssetTree fetches the whole asset tree. The tree doesn't change with the selected node (otherwise
  // picking one would collapse it into a single chain), so no picked node here — only the active company.
  const loadAssetTree = React.useCallback(async () => {
    const requestFilter = filterFingerprint;
    if (activeFilterFingerprint.current !== requestFilter) return;
    const request = ++assetTreeRequest.current;
    setAssetTree((current) => ({ ...current, loading: true }));
    try {
      const result = await api.findingAssetTree({ ...filters, company, importId, lane });
      if (request !== assetTreeRequest.current || activeFilterFingerprint.current !== requestFilter) return;
      setAssetTree({
        nodes: result.nodes ?? [],
        findingTotal: result.finding_total ?? 0,
        truncated: Boolean(result.truncated),
        droppedKinds: result.dropped_kinds ?? [],
        loaded: true,
        loading: false,
      });
    } catch (e) {
      if (request !== assetTreeRequest.current || activeFilterFingerprint.current !== requestFilter) return;
      setAssetTree((current) => ({ ...current, loading: false }));
      toast.error(t("Failed to load the asset tree: {error}", { error: (e as Error).message }));
    }
  }, [company, importId, lane, filterFingerprint, filters]);

  const refreshGroups = React.useCallback(async () => {
    const requestFilter = filterFingerprint;
    if (activeFilterFingerprint.current !== requestFilter) return;
    const request = ++groupsRequest.current;
    try {
      const result = await api.findingGroups({ page, pageSize, ...filters, company, importId, lane });
      if (request !== groupsRequest.current || activeFilterFingerprint.current !== requestFilter) return;
      setGroups(result.items);
      setGroupTotal(result.total);
      setTotal(result.finding_total);
    } catch {
      // Polling keeps the last successful snapshot visible.
    }
  }, [company, importId, lane, filterFingerprint, filters, page, pageSize]);

  const loadGroup = React.useCallback(
    async (key: string, groupPage: number, groupPageSize: number) => {
      const request = (groupRequests.current[key] ?? 0) + 1;
      const requestFilter = filterFingerprint;
      if (activeFilterFingerprint.current !== requestFilter) return;
      groupRequests.current[key] = request;
      setGroupFindings((current) => ({
        ...current,
        [key]: {
          items: current[key]?.items ?? [],
          total: current[key]?.total ?? 0,
          page: groupPage,
          pageSize: groupPageSize,
          loaded: current[key]?.loaded ?? false,
          loading: true,
        },
      }));
      try {
        const result = await api.findingsPage({
          page: groupPage,
          pageSize: groupPageSize,
          ...filters,
          task: key,
          company,
          importId,
          lane,
        });
        if (groupRequests.current[key] !== request || activeFilterFingerprint.current !== requestFilter) return;
        setGroupFindings((current) => ({
          ...current,
          [key]: {
            items: result.items,
            total: result.total,
            page: result.page,
            pageSize: result.page_size,
            loaded: true,
            loading: false,
          },
        }));
      } catch {
        if (groupRequests.current[key] !== request || activeFilterFingerprint.current !== requestFilter) return;
        setGroupFindings((current) => ({
          ...current,
          [key]: {
            ...(current[key] ?? { items: [], total: 0, page: groupPage, pageSize: groupPageSize, loaded: false }),
            loading: false,
          },
        }));
      }
    },
    [company, importId, lane, filterFingerprint, filters],
  );

  React.useEffect(() => {
    for (const [key, state] of Object.entries(groupFindings)) {
      if (!state.loaded || state.loading) continue;
      const lastPage = Math.max(1, Math.ceil(state.total / state.pageSize));
      if (state.page > lastPage) void loadGroup(key, lastPage, state.pageSize);
    }
  }, [groupFindings, loadGroup]);

  const toggleGroup = React.useCallback(
    (key: string) => {
      const opening = !expandedGroups.has(key);
      const next = new Set(expandedGroups);
      if (opening) next.add(key);
      else next.delete(key);
      setExpandedGroups(next);
      const state = groupFindings[key];
      if (opening && !state?.loaded && !state?.loading) {
        void loadGroup(key, state?.page ?? 1, state?.pageSize ?? 10);
      }
    },
    [expandedGroups, groupFindings, loadGroup],
  );

  // refreshStats re-reads the whole-table aggregates after a mutation; a failure keeps the last snapshot.
  const refreshStats = React.useCallback(() => {
    api
      .findingStats(company)
      .then(setStats)
      .catch(() => {
        // The mutation remains valid even if the aggregate refresh fails.
      });
  }, [company]);

  // Refresh the current view after an inline mutation: the flat view reloads its page, the grouped view refreshes group headers + the finding's group.
  const refreshAfterMutation = React.useCallback(
    (finding: Finding, removed = false) => {
      if (view === "asset") {
        // The asset view doesn't poll, so recount the tree after a mutation too.
        void loadFlat();
        void loadAssetTree();
        return;
      }
      if (view === "flat") {
        // When the last page is emptied, the out-of-range correction effect rolls the page number back and reloads.
        void loadFlat();
        return;
      }
      void refreshGroups();
      const key = finding.task_id ?? UNASSIGNED_TASK;
      const state = groupFindingsRef.current[key];
      if (state?.loaded) {
        const nextTotal = Math.max(0, state.total - (removed ? 1 : 0));
        const lastPage = Math.max(1, Math.ceil(nextTotal / state.pageSize));
        void loadGroup(key, Math.min(state.page, lastPage), state.pageSize);
      }
    },
    [loadAssetTree, loadFlat, loadGroup, refreshGroups, view],
  );

  // Reset every view's pagination and expansion when a shared finding filter changes.
  React.useEffect(() => {
    void filterFingerprint;
    setPage(1);
    onResetRef.current();
    setExpandedGroups(new Set());
    setGroupFindings({});
    setFlatPage(1);
    setFlat(EMPTY_FLAT_STATE);
    // Filters change the tree too; the previously selected node may no longer be in it, so fall back to "All assets".
    setAssetScope(null);
    setAssetTree(EMPTY_ASSET_TREE);
  }, [filterFingerprint]);

  // Switching asset nodes means a different result set; go back to page one.
  React.useEffect(() => {
    void assetScope;
    setFlatPage(1);
  }, [assetScope]);

  // The asset tree is queried once on view entry / filter change (plus actively re-pulled by
  // refreshAfterMutation after this page mutates a finding); no polling.
  React.useEffect(() => {
    if (!hydrated || view !== "asset") return;
    void retestFingerprint;
    void loadAssetTree();
  }, [retestFingerprint, loadAssetTree, hydrated, view]);

  // Poll only the current view: the flat view refreshes its page, the grouped view refreshes group headers and every expanded group.
  React.useEffect(() => {
    if (!hydrated) return;
    void retestFingerprint;
    const refresh = () => {
      if (view === "flat" || view === "asset") {
        if (!flatStateRef.current.loading) void loadFlat();
        return;
      }
      void refreshGroups();
      for (const key of expandedGroupsRef.current) {
        if (!visibleGroupKeysRef.current.has(key)) continue;
        const state = groupFindingsRef.current[key];
        if (state?.loaded && !state.loading) void loadGroup(key, state.page, state.pageSize);
      }
    };
    refresh();
    if (view === "asset") return;
    return setVisibleInterval(refresh, 5000);
  }, [retestFingerprint, loadFlat, loadGroup, hydrated, refreshGroups, view]);

  React.useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(groupTotal / pageSize));
    if (page > lastPage) setPage(lastPage);
  }, [groupTotal, page, pageSize]);

  React.useEffect(() => {
    if (!flat.loaded) return;
    const lastPage = Math.max(1, Math.ceil(flat.total / flatPageSize));
    if (flatPage > lastPage) setFlatPage(lastPage);
  }, [flat.loaded, flat.total, flatPage, flatPageSize]);

  // Whole-table aggregates (stat tiles + vuln-class options) — independent of the current page, so they stay exact.
  React.useEffect(() => {
    let alive = true;
    const load = () => {
      api
        .findingStats(company)
        .then((s) => {
          if (alive) {
            setStats(s);
            setStatsLoaded(true);
          }
        })
        .catch(() => {
          // Keep the previous aggregate snapshot until the next poll.
        });
    };
    load();
    const stopPolling = setVisibleInterval(load, 5000);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [company]);

  return {
    flat,
    flatPage,
    setFlatPage,
    flatPageSize,
    setFlatPageSize,
    assetTree,
    assetScope,
    activeAssetScope,
    setAssetScope,
    groups,
    groupTotal,
    expandedGroups,
    groupFindings,
    toggleGroup,
    loadGroup,
    total,
    stats,
    statsLoaded,
    page,
    setPage,
    pageSize,
    setPageSize,
    setFindings,
    dropOne,
    loadFlat,
    loadAssetTree,
    refreshStats,
    refreshAfterMutation,
  };
}

export type FindingData = ReturnType<typeof useFindingData>;
