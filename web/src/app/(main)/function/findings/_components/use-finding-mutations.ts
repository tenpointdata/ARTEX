"use client";

import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import { statusMeta } from "@/lib/status";
import { matchesStatusFilter } from "@/lib/triage";
import type { Finding, FindingReportState, FindingStatus } from "@/lib/types";

import type { FindingFilters } from "./finding-filters";
import { type FindingEdit, findingRowKey, isSameFinding, REPORT_STATE_META } from "./finding-meta";
import type { FindingData } from "./use-finding-data";

interface MutationOptions {
  data: FindingData;
  filters: FindingFilters;
  view: string;
  selectedIds: Set<string>;
  setSelectedIds: React.Dispatch<React.SetStateAction<Set<string>>>;
  edit: FindingEdit | null;
  setExpanded: React.Dispatch<React.SetStateAction<string | null>>;
}

// useFindingMutations holds every write the findings list makes: per-row status/report stage/edit/delete
// (optimistic, reverted on error) and the bulk status/report-stage actions on the selection.
export function useFindingMutations({
  data,
  filters,
  view,
  selectedIds,
  setSelectedIds,
  edit,
  setExpanded,
}: MutationOptions) {
  const { setFindings, dropOne, refreshStats, refreshAfterMutation, loadFlat, loadAssetTree } = data;
  const [bulkBusy, setBulkBusy] = React.useState(false);
  const [saving, setSaving] = React.useState(false);

  // updateStatus optimistically flips one finding's triage state, reverting on error. It resolves to
  // whether the write landed and whether the row then left the list (no longer matching the filter).
  const updateStatus = React.useCallback(
    async (f: Finding, next: FindingStatus): Promise<{ ok: boolean; dropped: boolean }> => {
      if (!f.finding_id || next === f.status) return { ok: false, dropped: false };
      const prev = f.status;
      setFindings((cur) => cur.map((x) => (isSameFinding(x, f) ? { ...x, status: next } : x)));
      try {
        await api.setFindingStatus(f.finding_id, next);
        toast.success(t('Marked as "{label}"', { label: statusMeta("finding", next).label }));
        refreshStats();
        const dropped = !matchesStatusFilter(filters.status, next);
        if (dropped) dropOne((x) => isSameFinding(x, f));
        refreshAfterMutation(f);
        return { ok: true, dropped };
      } catch (e) {
        setFindings((cur) => cur.map((x) => (isSameFinding(x, f) ? { ...x, status: prev } : x)));
        toast.error(t("Update failed: {error}", { error: (e as Error).message }));
        return { ok: false, dropped: false };
      }
    },
    [dropOne, filters.status, refreshAfterMutation, refreshStats, setFindings],
  );

  const updateReportState = React.useCallback(
    async (f: Finding, next: FindingReportState) => {
      if (!f.finding_id || next === (f.report_state ?? "none")) return;
      const prev = f.report_state ?? "none";
      setFindings((cur) => cur.map((x) => (isSameFinding(x, f) ? { ...x, report_state: next } : x)));
      try {
        await api.setFindingReportState(f.finding_id, next);
        toast.success(t('Report stage set to "{label}"', { label: REPORT_STATE_META[next].label }));
        refreshStats();
        if (filters.reportState !== "all" && next !== filters.reportState) dropOne((x) => isSameFinding(x, f));
        refreshAfterMutation(f);
      } catch (e) {
        setFindings((cur) => cur.map((x) => (isSameFinding(x, f) ? { ...x, report_state: prev } : x)));
        toast.error(t("Update failed: {error}", { error: (e as Error).message }));
      }
    },
    [dropOne, filters.reportState, refreshAfterMutation, refreshStats, setFindings],
  );

  // runBulk applies one bulk write to the selection, then reloads the current view so rows hidden by the filter disappear.
  const runBulk = React.useCallback(
    async (write: (ids: string[]) => Promise<{ updated: number }>, success: (count: number) => string) => {
      if (selectedIds.size === 0 || bulkBusy) return;
      setBulkBusy(true);
      try {
        const r = await write([...selectedIds]);
        setSelectedIds(new Set());
        toast.success(success(r.updated));
        void loadFlat();
        if (view === "asset") void loadAssetTree();
        refreshStats();
      } catch (e) {
        toast.error(t("Bulk update failed: {error}", { error: (e as Error).message }));
      } finally {
        setBulkBusy(false);
      }
    },
    [bulkBusy, loadAssetTree, loadFlat, refreshStats, selectedIds, setSelectedIds, view],
  );

  const applyBulkStatus = React.useCallback(
    (next: FindingStatus) =>
      runBulk(
        (ids) => api.bulkSetFindingStatus(ids, next),
        (count) => t('Marked {count} findings as "{label}"', { count, label: statusMeta("finding", next).label }),
      ),
    [runBulk],
  );

  const applyBulkReportState = React.useCallback(
    (next: FindingReportState) =>
      runBulk(
        (ids) => api.bulkSetFindingReportState(ids, next),
        (count) =>
          t('Set report stage of {count} findings to "{label}"', { count, label: REPORT_STATE_META[next].label }),
      ),
    [runBulk],
  );

  // saveEdit saves the expanded row's name/category/severity, writes back to the local list and refreshes stats.
  const saveEdit = React.useCallback(
    async (f: Finding) => {
      if (!f.finding_id || !edit) return;
      setSaving(true);
      try {
        const updated = await api.updateFinding(f.finding_id, {
          name: edit.name.trim(),
          vulnclass: edit.vulnclass.trim(),
          severity: edit.severity,
        });
        setFindings((cur) =>
          cur.map((x) =>
            isSameFinding(x, f)
              ? { ...x, name: updated.name, vulnclass: updated.vulnclass, severity: updated.severity }
              : x,
          ),
        );
        toast.success(t("Saved"));
        refreshStats();
        refreshAfterMutation(f);
      } catch (e) {
        toast.error(t("Save failed: {error}", { error: (e as Error).message }));
      } finally {
        setSaving(false);
      }
    },
    [edit, refreshAfterMutation, refreshStats, setFindings],
  );

  // deleteFinding deletes one vulnerability (confirmed in the row): removes it, collapses the row, refreshes stats.
  const deleteFinding = React.useCallback(
    async (f: Finding) => {
      const id = f.finding_id;
      if (!id) return;
      try {
        await api.deleteFinding(id);
        dropOne((x) => isSameFinding(x, f));
        setSelectedIds((current) => {
          const next = new Set(current);
          next.delete(id);
          return next;
        });
        const rowKey = findingRowKey(f);
        setExpanded((cur) => (cur === rowKey ? null : cur));
        toast.success(t("Vulnerability deleted"));
        refreshStats();
        refreshAfterMutation(f, true);
      } catch (e) {
        toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
      }
    },
    [dropOne, refreshAfterMutation, refreshStats, setExpanded, setSelectedIds],
  );

  return {
    bulkBusy,
    saving,
    updateStatus,
    updateReportState,
    applyBulkStatus,
    applyBulkReportState,
    saveEdit,
    deleteFinding,
  };
}
