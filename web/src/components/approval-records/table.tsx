"use client";

import * as React from "react";

import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useTriageKeys } from "@/hooks/use-triage-keys";
import { t } from "@/i18n";
import { APPROVAL_VERDICTS, type ApprovalDecision, type TriageNav, triageTarget } from "@/lib/triage";
import type { InterceptApprovalRow } from "@/lib/types";

import { ApprovalDetail } from "./detail";
import {
  ApprovalOrigin,
  type Decide,
  DecidedByBadge,
  fmtTime,
  MatchCell,
  originLabel,
  RiskBadge,
  StatusBadge,
} from "./shared";
export const approvalBreakpoints = [
  "(min-width: 40rem)",
  "(min-width: 48rem)",
  "(min-width: 64rem)",
  "(min-width: 80rem)",
];
export function subscribeColumns(onChange: () => void) {
  const queries = approvalBreakpoints.map((query) => window.matchMedia(query));
  for (const query of queries) query.addEventListener("change", onChange);
  return () => {
    for (const query of queries) query.removeEventListener("change", onChange);
  };
}
export function visibleColumnCount() {
  const matches = approvalBreakpoints.map((query) => window.matchMedia(query).matches);
  return 3 + Number(matches[0]) + Number(matches[1]) + 3 * Number(matches[2]) + 2 * Number(matches[3]);
}
export function serverColumnCount() {
  return 10;
}

export type Selection = { selected: Set<number>; toggle: (id: number) => void; toggleAll: () => void };

/**
 * Keyboard triage of the pending queue (lib/triage): A allows and D denies the open request, then the
 * next one opens; J/K move between requests and Esc closes. `decide` resolves to whether the decision
 * landed, so a failed decision leaves the request open.
 */
export type ApprovalTriage = { decide: (id: number, decision: ApprovalDecision) => Promise<boolean> };

// Server limit on ids per batch-decide call (maxBatchDecide in server/intercept.go).
export const batchDecideMax = 100;

export function ApprovalTable({
  rows,
  busy,
  decide,
  revision,
  label,
  selection,
  triage,
}: {
  rows: InterceptApprovalRow[];
  busy: boolean;
  decide: Decide;
  revision: number;
  label: string;
  selection?: Selection;
  triage?: ApprovalTriage;
}) {
  const [expanded, setExpanded] = React.useState<Set<number>>(() => new Set());
  // focus = the open request the triage keys act on: the one opened last.
  const [focus, setFocus] = React.useState<number | null>(null);
  const columns =
    React.useSyncExternalStore(subscribeColumns, visibleColumnCount, serverColumnCount) + (selection ? 1 : 0);
  const selectedCount = selection ? rows.filter((row) => selection.selected.has(row.id)).length : 0;
  let allChecked: boolean | "indeterminate" = selectedCount > 0 ? "indeterminate" : false;
  if (selectedCount > 0 && selectedCount === rows.length) allChecked = true;
  const prefix = React.useId();
  function toggle(id: number) {
    const opening = !expanded.has(id);
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    if (opening) setFocus(id);
    else if (focus === id) setFocus(null);
  }
  // moveFocus closes the focused request and opens `to` (or only closes, with null).
  function moveFocus(to: number | null) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (focus !== null) next.delete(focus);
      if (to !== null) next.add(to);
      return next;
    });
    setFocus(to);
  }
  const keys = rows.map((row) => String(row.id));
  const focusKey = focus !== null && expanded.has(focus) ? String(focus) : null;
  const targetOf = (nav: "next" | "prev") => {
    const key = triageTarget(keys, focusKey, nav);
    return key === null ? null : Number(key);
  };
  const onNav = (nav: TriageNav) => {
    if (nav === "close") moveFocus(null);
    else {
      const to = targetOf(nav);
      if (to !== null) moveFocus(to);
    }
  };
  const onVerdict = async (decision: ApprovalDecision) => {
    const row = rows.find((r) => String(r.id) === focusKey);
    if (!triage || busy || row?.status !== "pending") return;
    const then = targetOf("next");
    if (await triage.decide(row.id, decision)) moveFocus(then);
  };
  useTriageKeys(Boolean(triage) && rows.length > 0, APPROVAL_VERDICTS, (d) => void onVerdict(d), onNav);
  return (
    <Table className="table-fixed" aria-label={label}>
      <TableHeader>
        <TableRow>
          {selection ? (
            <TableHead className="w-10">
              <Checkbox
                aria-label={t("Select all pending approvals on this page")}
                checked={allChecked}
                disabled={busy || !rows.length}
                onCheckedChange={() => selection.toggleAll()}
              />
            </TableHead>
          ) : null}
          <TableHead className="w-10">
            <span className="sr-only">{t("Expand details")}</span>
          </TableHead>
          <TableHead className="hidden w-14 sm:table-cell">#</TableHead>
          <TableHead className="w-28">{t("Tool")}</TableHead>
          <TableHead className="hidden md:table-cell">{t("Source")}</TableHead>
          <TableHead className="hidden lg:table-cell">{t("Matched rule")}</TableHead>
          <TableHead className="hidden xl:table-cell">{t("Arguments")}</TableHead>
          <TableHead className="w-24">{t("Status")}</TableHead>
          <TableHead className="hidden w-24 lg:table-cell">{t("Decided by")}</TableHead>
          <TableHead className="hidden w-36 lg:table-cell">{t("Requested at")}</TableHead>
          <TableHead className="hidden w-36 xl:table-cell">{t("Decided at")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => {
          const open = expanded.has(row.id);
          const panelID = `${prefix}-${row.id}`;
          return (
            <React.Fragment key={row.id}>
              <TableRow
                data-state={open ? "selected" : undefined}
                className="cursor-pointer"
                onClick={() => toggle(row.id)}
              >
                {selection ? (
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <Checkbox
                      aria-label={t("Select approval #{id}", { id: row.id })}
                      checked={selection.selected.has(row.id)}
                      disabled={busy || row.status !== "pending"}
                      onCheckedChange={() => selection.toggle(row.id)}
                    />
                  </TableCell>
                ) : null}
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={
                      open ? t("Collapse approval #{id}", { id: row.id }) : t("Expand approval #{id}", { id: row.id })
                    }
                    aria-expanded={open}
                    aria-controls={open ? panelID : undefined}
                    onClick={(e) => {
                      e.stopPropagation();
                      toggle(row.id);
                    }}
                  >
                    {open ? <ChevronDownIcon /> : <ChevronRightIcon />}
                  </Button>
                </TableCell>
                <TableCell className="hidden text-muted-foreground sm:table-cell">{row.id}</TableCell>
                <TableCell>
                  <div className="flex min-w-0 flex-col items-start gap-1">
                    <code className="block max-w-full truncate text-xs">{row.tool_name}</code>
                    <RiskBadge risk={row.risk} />
                  </div>
                </TableCell>
                <TableCell className="hidden md:table-cell">
                  <div className="flex flex-col gap-1">
                    <span className="truncate" title={originLabel(row)}>
                      <ApprovalOrigin row={row} />
                    </span>
                    <span className="truncate text-muted-foreground text-xs">
                      {row.agent_name || row.conv_agent_key}
                    </span>
                  </div>
                </TableCell>
                <TableCell className="hidden lg:table-cell">
                  <MatchCell row={row} />
                </TableCell>
                <TableCell className="hidden xl:table-cell">
                  <code className="block truncate text-muted-foreground text-xs">{JSON.stringify(row.tool_input)}</code>
                </TableCell>
                <TableCell>
                  <StatusBadge status={row.status} />
                </TableCell>
                <TableCell className="hidden lg:table-cell">
                  <DecidedByBadge row={row} />
                </TableCell>
                <TableCell className="hidden text-muted-foreground text-xs lg:table-cell">
                  {fmtTime(row.created_at)}
                </TableCell>
                <TableCell className="hidden text-muted-foreground text-xs xl:table-cell">
                  {fmtTime(row.decided_at)}
                </TableCell>
              </TableRow>
              {open ? (
                <TableRow className="hover:bg-transparent has-aria-expanded:bg-transparent">
                  <TableCell colSpan={columns} className="whitespace-normal p-0">
                    <section id={panelID} aria-label={t("Approval details #{id}", { id: row.id })}>
                      <ApprovalDetail
                        row={row}
                        busy={busy}
                        decide={decide}
                        revision={revision}
                        shortcuts={Boolean(triage) && focusKey === String(row.id)}
                      />
                    </section>
                  </TableCell>
                </TableRow>
              ) : null}
            </React.Fragment>
          );
        })}
      </TableBody>
    </Table>
  );
}
