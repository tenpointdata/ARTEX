"use client";

import * as React from "react";

import { CheckIcon, ClipboardListIcon, RefreshCwIcon, ShieldAlertIcon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { TablePagination } from "@/components/table-pagination";
import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { InterceptApprovalFilter, InterceptApprovalRow, InterceptPending } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ApprovalDetail } from "./approval-records/detail";
import type { Decide } from "./approval-records/shared";
import { ApprovalTable, batchDecideMax, type Selection } from "./approval-records/table";

export { ApprovalDetail };
export function ApprovalRecords({ taskId }: { taskId?: string }) {
  const [rows, setRows] = React.useState<InterceptApprovalRow[]>([]);
  const [pendingRows, setPendingRows] = React.useState<InterceptPending[]>([]);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(20);
  const [total, setTotal] = React.useState(0);
  const [filter, setFilter] = React.useState<InterceptApprovalFilter>({});
  const [loading, setLoading] = React.useState(true);
  const [refreshing, setRefreshing] = React.useState(false);
  const [error, setError] = React.useState("");
  const [pendingError, setPendingError] = React.useState("");
  const [deciding, setDeciding] = React.useState(false);
  const [revision, setRevision] = React.useState(0);
  const [selected, setSelected] = React.useState<Set<number>>(() => new Set());
  const [batchDecision, setBatchDecision] = React.useState<"allowed" | "denied" | null>(null);
  const [batchErrors, setBatchErrors] = React.useState<{ id: number; error: string }[]>([]);
  const request = React.useRef(0);
  const decisionLock = React.useRef(false);
  const filterID = React.useId();
  const filtered = Boolean(filter.status || filter.decision_source || filter.decided_by);

  // A task can stay mounted while the user switches between task details.
  // Reset the cursor so the new scope always starts at its newest records.
  // biome-ignore lint/correctness/useExhaustiveDependencies: taskId intentionally resets pagination when scope changes.
  React.useEffect(() => {
    setPage(1);
  }, [taskId]);

  const load = React.useCallback(
    async (manual = false) => {
      const id = ++request.current;
      if (manual) setRefreshing(true);
      try {
        // The approval queue is independent of the current history page and its
        // filter. Older requests must remain actionable even when newer decisions
        // fill the page or a filter would hide them.
        const [history, pending] = await Promise.allSettled([
          taskId
            ? api.interceptTaskPage(taskId, page, pageSize, filter)
            : api.interceptHistoryPage(page, pageSize, filter),
          api.interceptPending(),
        ]);
        if (id !== request.current) return;
        if (history.status === "fulfilled") {
          setRows(history.value.items);
          setTotal(history.value.total);
          setError("");
        } else {
          setError((history.reason as Error).message || t("Failed to load"));
        }
        if (pending.status === "fulfilled") {
          setPendingRows(pending.value);
          setPendingError("");
        } else {
          setPendingError((pending.reason as Error).message || t("Failed to load"));
        }
        if (manual) setRevision((v) => v + 1);
      } catch (e) {
        if (id === request.current) setError((e as Error).message || t("Failed to load"));
      } finally {
        if (id === request.current) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [taskId, page, pageSize, filter],
  );
  const latestLoad = React.useRef(load);
  latestLoad.current = load;

  React.useEffect(() => {
    void load();
    const stopPolling = setVisibleInterval(() => void load(), 5000);
    return () => {
      request.current++;
      stopPolling();
    };
  }, [load]);

  const changePageSize = (next: number) => {
    setPageSize(next);
    setPage(1);
  };

  const changeFilter = (next: InterceptApprovalFilter) => {
    // An in-flight response for the old filter must not repopulate the table.
    request.current++;
    setFilter(next);
    setPage(1);
    setRows([]);
    setTotal(0);
    setError("");
    setLoading(true);
  };

  const decide: Decide = async (id, decision) => {
    if (decisionLock.current) return;
    decisionLock.current = true;
    setDeciding(true);
    try {
      await api.interceptDecide(id, decision);
      // Invalidate a list request started before this decision.
      request.current++;
      // Optimistically drop the decided item from the independent pending queue
      // for instant feedback; the re-fetch below reconciles with server truth.
      setRows((prev) =>
        prev.map((row) =>
          row.id === id ? { ...row, status: decision, decided_by: "human", decided_at: new Date().toISOString() } : row,
        ),
      );
      setPendingRows((prev) => prev.filter((row) => row.id !== id));
      setRevision((v) => v + 1);
      toast.success(decision === "allowed" ? t("Execution allowed") : t("Execution denied"));
      // Re-fetch counts and rows: a decided item may no longer match the filter.
      await latestLoad.current(true);
    } catch (e) {
      toast.error((e as Error).message);
      await latestLoad.current(true);
    } finally {
      decisionLock.current = false;
      setDeciding(false);
    }
  };

  const historyById = new Map(rows.map((row) => [row.id, row]));
  const pending: InterceptApprovalRow[] = pendingRows
    .filter((row) => !taskId || row.task_id === taskId)
    .map((row) => ({
      conv_title: "",
      conv_agent_key: "",
      rule_name: row.rule_id ? t("Rule #{id}", { id: row.rule_id }) : "",
      ...historyById.get(row.id),
      ...row,
    }));
  // Only rows still in the pending queue count as selected.
  const selectedRows = pending.filter((row) => selected.has(row.id));
  const selectedHigh = selectedRows.filter((row) => row.risk?.level === "high").length;
  const selection: Selection = {
    selected,
    toggle: (id) =>
      setSelected((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        return next;
      }),
    toggleAll: () =>
      setSelected(selectedRows.length === pending.length ? new Set() : new Set(pending.map((row) => row.id))),
  };

  // Each id is decided by the server exactly like a single decision. The server
  // takes at most 100 ids per call (maxBatchDecide in server/intercept.go) and
  // the pending queue is not paginated, so the selection goes out in sequential
  // chunks of at most batchDecideMax ids. If a chunk cannot be sent (network
  // error, 400), sending stops: its ids and the later ones stay selected with
  // the error, and chunks already decided stay decided. A failed id stays
  // pending and selected, with its error listed.
  const batchDecide = async (decision: "allowed" | "denied") => {
    const ids = selectedRows.map((row) => row.id);
    if (!ids.length || decisionLock.current) return;
    decisionLock.current = true;
    setDeciding(true);
    let decided = 0;
    const failed: { id: number; error: string }[] = [];
    try {
      for (let start = 0; start < ids.length; start += batchDecideMax) {
        try {
          const res = await api.interceptBatchDecide(ids.slice(start, start + batchDecideMax), decision);
          decided += res.decided;
          for (const r of res.results) if (!r.ok) failed.push({ id: r.id, error: r.error || t("Not decided") });
        } catch (e) {
          const error = t("Not sent: {error}", { error: (e as Error).message || t("request failed") });
          for (const id of ids.slice(start)) failed.push({ id, error });
          break;
        }
      }
      request.current++;
      setBatchErrors(failed);
      setSelected(new Set(failed.map((f) => f.id)));
      setRevision((v) => v + 1);
      if (failed.length)
        toast.error(
          decision === "allowed"
            ? t("{decided} allowed, {failed} not decided (see the errors listed above)", {
                decided,
                failed: failed.length,
              })
            : t("{decided} denied, {failed} not decided (see the errors listed above)", {
                decided,
                failed: failed.length,
              }),
        );
      else
        toast.success(
          decision === "allowed" ? t("{count} allowed", { count: decided }) : t("{count} denied", { count: decided }),
        );
    } finally {
      decisionLock.current = false;
      setDeciding(false);
      setBatchDecision(null);
      await latestLoad.current(true);
    }
  };
  const title = taskId ? t("Interception approval") : t("Approval records");
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <ClipboardListIcon className="size-5 text-muted-foreground" />
          <h1 className="font-semibold text-xl">{title}</h1>
          {pending.length ? <Badge variant="secondary">{t("{count} pending", { count: pending.length })}</Badge> : null}
        </div>
        <Button variant="outline" size="sm" onClick={() => void load(true)} disabled={loading || refreshing}>
          <RefreshCwIcon data-icon="inline-start" className={cn(refreshing && "animate-spin")} />
          {t("Refresh")}
        </Button>
      </div>
      <p className="text-muted-foreground text-sm">
        {t(
          "Expand a record to view the tool request and approval decision, along with the context and execution result at the time.",
        )}
      </p>
      <FieldGroup className="flex-row flex-wrap items-end gap-3" aria-label={t("Approval record filters")}>
        <Field className="w-full sm:w-40">
          <FieldLabel htmlFor={`${filterID}-status`}>{t("Approval status")}</FieldLabel>
          <Select
            value={filter.status ?? "all"}
            onValueChange={(value) =>
              changeFilter({
                ...filter,
                status: value === "all" ? undefined : (value as InterceptApprovalFilter["status"]),
              })
            }
          >
            <SelectTrigger id={`${filterID}-status`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t("All statuses")}</SelectItem>
                <SelectItem value="denied">{t("Denied")}</SelectItem>
                <SelectItem value="pending">{t("Pending approval")}</SelectItem>
                <SelectItem value="allowed">{t("Allowed")}</SelectItem>
                <SelectItem value="timeout">{t("Timed out")}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field className="w-full sm:w-40">
          <FieldLabel htmlFor={`${filterID}-source`}>{t("Decision source")}</FieldLabel>
          <Select
            value={filter.decision_source ?? "all"}
            onValueChange={(value) =>
              changeFilter({
                ...filter,
                decision_source: value === "all" ? undefined : (value as InterceptApprovalFilter["decision_source"]),
              })
            }
          >
            <SelectTrigger id={`${filterID}-source`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t("All sources")}</SelectItem>
                <SelectItem value="model">{t("Model decision")}</SelectItem>
                <SelectItem value="rule">{t("Rule decision")}</SelectItem>
                <SelectItem value="unknown">{t("Unknown source")}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <Field className="w-full sm:w-40">
          <FieldLabel htmlFor={`${filterID}-decided-by`}>{t("Decided by")}</FieldLabel>
          <Select
            value={filter.decided_by ?? "all"}
            onValueChange={(value) =>
              changeFilter({
                ...filter,
                decided_by: value === "all" ? undefined : (value as InterceptApprovalFilter["decided_by"]),
              })
            }
          >
            <SelectTrigger id={`${filterID}-decided-by`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="all">{t("All actors")}</SelectItem>
                <SelectItem value="model">{t("AI-decided")}</SelectItem>
                <SelectItem value="human">{t("Human")}</SelectItem>
                <SelectItem value="rule">{t("Rule")}</SelectItem>
                <SelectItem value="system">{t("System")}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        {filtered ? (
          <Button variant="ghost" size="sm" onClick={() => changeFilter({})}>
            {t("Clear filters")}
          </Button>
        ) : null}
      </FieldGroup>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t("Failed to load records: {error}. Click Refresh to retry.", { error })}
          </AlertDescription>
        </Alert>
      ) : null}
      {pendingError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t("Failed to load pending approvals: {error}. Click Refresh to retry.", { error: pendingError })}
          </AlertDescription>
        </Alert>
      ) : null}
      {batchErrors.length ? (
        <Alert variant="destructive">
          <AlertDescription>
            <p>{t("{count} selected request(s) were not decided and stay pending:", { count: batchErrors.length })}</p>
            <ul className="list-disc pl-4">
              {batchErrors.map((f) => (
                <li key={f.id}>
                  #{f.id}: {f.error}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      ) : null}
      {pending.length ? (
        <section className="overflow-hidden rounded-xl border">
          <div className="flex items-center gap-2 border-b bg-muted/40 px-4 py-3 font-medium text-sm">
            <ShieldAlertIcon className="size-4" />
            {t("Pending ({count})", { count: pending.length })}
            <span className="text-muted-foreground text-xs">{t("expand to allow or deny")}</span>
          </div>
          <ApprovalTable
            rows={pending}
            busy={deciding}
            decide={decide}
            revision={revision}
            label={t("Pending approvals")}
            selection={selection}
          />
          <div className="flex flex-wrap items-center justify-between gap-2 border-t px-4 py-3 text-sm">
            <span className="text-muted-foreground">{t("{count} selected", { count: selectedRows.length })}</span>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={deciding || !selectedRows.length} onClick={() => setBatchDecision("allowed")}>
                <CheckIcon data-icon="inline-start" />
                {t("Approve selected")}
              </Button>
              <Button
                size="sm"
                variant="destructive"
                disabled={deciding || !selectedRows.length}
                onClick={() => setBatchDecision("denied")}
              >
                <XIcon data-icon="inline-start" />
                {t("Deny selected")}
              </Button>
            </div>
          </div>
        </section>
      ) : null}
      <AlertDialog
        open={batchDecision != null}
        onOpenChange={(open) => {
          if (!open && !deciding) setBatchDecision(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {batchDecision === "denied"
                ? t("Deny {count} request(s)?", { count: selectedRows.length })
                : t("Approve {count} request(s)?", { count: selectedRows.length })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {selectedHigh
                ? t("{count} of them are high risk.", { count: selectedHigh })
                : t("None of them is high risk.")}{" "}
              {t("Each request is decided on its own; one that cannot be decided stays pending and is listed.")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deciding}>{t("Cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant={batchDecision === "denied" ? "destructive" : "default"}
              disabled={deciding || !selectedRows.length}
              onClick={(event) => {
                event.preventDefault();
                if (batchDecision) void batchDecide(batchDecision);
              }}
            >
              {batchDecision === "denied" ? t("Deny selected") : t("Approve selected")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <section className="overflow-hidden rounded-xl border">
        <div className="border-b px-4 py-3 font-medium text-sm">
          {filtered ? t("Filtered results ({total})", { total }) : t("All records ({total})", { total })}
        </div>
        {loading ? (
          <div className="flex flex-col gap-3 p-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : null}
        {!loading && !rows.length && !error ? (
          <Empty>
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <ClipboardListIcon />
              </EmptyMedia>
              <EmptyTitle>
                {filtered ? t("No approval records match the filters") : t("No approval records")}
              </EmptyTitle>
              <EmptyDescription>
                {filtered
                  ? t(
                      "Adjust the approval status, decision source or deciding actor, or clear the filters to see all records.",
                    )
                  : t("Records will appear here once a rule or model makes an approval decision.")}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}
        {rows.length ? (
          <ApprovalTable
            rows={rows}
            busy={deciding}
            decide={decide}
            revision={revision}
            label={t("Approval record list")}
          />
        ) : null}
        {!loading && !error ? (
          <TablePagination
            page={page}
            pageSize={pageSize}
            total={total}
            onPageChange={setPage}
            onPageSizeChange={changePageSize}
            pageSizeOptions={[10, 20, 50]}
          />
        ) : null}
      </section>
    </div>
  );
}
