"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowUpRightIcon, ChevronRightIcon, FlaskConicalIcon, RotateCcwIcon, Trash2Icon } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { useTriageKeys } from "@/hooks/use-triage-keys";
import { t } from "@/i18n";
import { statusMeta } from "@/lib/status";
import { TRIAGE_VERDICTS, type TriageNav, triageTarget } from "@/lib/triage";
import type { ActiveFindingRetest, Finding, FindingReportState, FindingStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

import { FindingExpandedRow } from "./finding-expanded-row";
import {
  FINDING_REPORT_STATES,
  FINDING_STATUSES,
  type FindingEdit,
  type FindingReport,
  findingRowKey,
  fmtTime,
  REPORT_STATE_META,
  ReportStateBadge,
  VerificationBadge,
} from "./finding-meta";
import { CodeLocation, LaneBadge } from "./lane-badge";
import { TriageBar } from "./triage-bar";

// The vocabularies moved to ./finding-meta; re-exported so existing importers keep working.
export * from "./finding-meta";

const COLUMN_COUNT = 10;

interface FindingsTableProps {
  items: Finding[];
  selectedIds: Set<string>;
  onToggleSelected: (id: string, checked: boolean) => void;
  onToggleSelectedPage: (ids: string[], checked: boolean) => void;
  /** The expanded row's findingRowKey; null = all collapsed. */
  expandedKey: string | null;
  onToggleRow: (finding: Finding) => void;
  reports: Record<string, FindingReport>;
  edit: FindingEdit | null;
  onEditChange: React.Dispatch<React.SetStateAction<FindingEdit | null>>;
  saving: boolean;
  onSave: (finding: Finding) => void;
  onStatusChange: (finding: Finding, next: FindingStatus) => void;
  onReportStateChange: (finding: Finding, next: FindingReportState) => void;
  onRetest: (finding: Finding) => void;
  activeRetests: Record<string, ActiveFindingRetest>;
  onDeepen: (finding: Finding) => void;
  onDelete: (finding: Finding) => void;
  /** The select-all checkbox's accessibility label; the flat and grouped views word it differently. */
  selectAllLabel?: string;
  /** Opens one row (collapsing any other), or collapses all with null. */
  onOpenRow: (finding: Finding | null) => void;
  /** Whether the session may set a verdict (the review permission); navigation keys work regardless. */
  canTriage: boolean;
  triageBusy?: boolean;
  /** Applies a verdict to the open finding; `then` is the finding to open next (null at the end of this list). */
  onTriage: (finding: Finding, next: FindingStatus, then: Finding | null) => void;
}

/** Everything a FindingsTable takes besides its rows; the flat, grouped and asset views share one set. */
export type FindingRowProps = Omit<FindingsTableProps, "items" | "selectAllLabel">;

// FindingsTable is the findings list's table body; the flat view and the grouped-by-task view share the same row rendering
// (selection / inline expand / rename & status change / retest / deepen / delete), differing only in the outer container and pagination.
export function FindingsTable({
  items,
  selectedIds,
  onToggleSelected,
  onToggleSelectedPage,
  expandedKey,
  onToggleRow,
  reports,
  edit,
  onEditChange,
  saving,
  onSave,
  onStatusChange,
  onReportStateChange,
  onRetest,
  activeRetests,
  onDeepen,
  onDelete,
  selectAllLabel = t("Select all on this page"),
  onOpenRow,
  canTriage,
  triageBusy,
  onTriage,
}: FindingsTableProps) {
  const canOperate = useCan("operate"); // retest, deepen, delete
  const keys = items.map(findingRowKey);
  const openIndex = expandedKey === null ? -1 : keys.indexOf(expandedKey);
  const openFinding = openIndex >= 0 ? items[openIndex] : null;
  const findingAt = (key: string | null) => (key === null ? null : (items[keys.indexOf(key)] ?? null));
  const nav = (to: TriageNav) => {
    if (to === "close") onOpenRow(null);
    else {
      const target = findingAt(triageTarget(keys, expandedKey, to));
      if (target) onOpenRow(target);
    }
  };
  const verdict = (next: FindingStatus) => {
    if (!canTriage || triageBusy || !openFinding?.finding_id) return;
    onTriage(openFinding, next, findingAt(triageTarget(keys, expandedKey, "next")));
  };
  useTriageKeys(openFinding !== null, TRIAGE_VERDICTS, verdict, nav);

  // Keep the open row in view and focused when the queue advances by keyboard or verdict.
  const tableRef = React.useRef<HTMLTableElement>(null);
  React.useEffect(() => {
    if (expandedKey === null) return;
    const row = tableRef.current?.querySelector<HTMLElement>(`[data-row-key="${CSS.escape(expandedKey)}"]`);
    if (!row) return;
    row.scrollIntoView({ block: "nearest" });
    if (!row.contains(document.activeElement)) row.focus({ preventScroll: true });
  }, [expandedKey]);

  const selectableIds = items.map((finding) => finding.finding_id).filter((id): id is string => Boolean(id));
  const selectedCount = selectableIds.filter((id) => selectedIds.has(id)).length;
  let headerChecked: boolean | "indeterminate" = false;
  if (selectableIds.length > 0 && selectedCount === selectableIds.length) {
    headerChecked = true;
  } else if (selectedCount > 0) {
    headerChecked = "indeterminate";
  }

  return (
    /* Fixed column widths keep expanded content from stretching the table; narrow screens scroll horizontally inside the table only. */
    <Table ref={tableRef} className="min-w-[68rem] table-fixed">
      <TableHeader>
        <TableRow>
          <TableHead className="w-8">
            <Checkbox
              checked={headerChecked}
              onCheckedChange={(checked) => onToggleSelectedPage(selectableIds, checked === true)}
              aria-label={selectAllLabel}
            />
          </TableHead>
          <TableHead className="w-8" />
          <TableHead className="w-20">{t("Severity")}</TableHead>
          <TableHead>{t("Vulnerability")}</TableHead>
          <TableHead className="w-44">{t("Asset")}</TableHead>
          <TableHead className="w-28">{t("Status")}</TableHead>
          <TableHead className="w-36">{t("Report")}</TableHead>
          <TableHead className="w-32">{t("Task")}</TableHead>
          <TableHead className="w-24">{t("Time")}</TableHead>
          <TableHead className="w-48">{t("Actions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((f) => {
          const rowKey = findingRowKey(f);
          const open = expandedKey === rowKey;
          const retest = f.finding_id ? activeRetests[f.finding_id] : undefined;
          return (
            <React.Fragment key={rowKey}>
              <TableRow
                className="cursor-pointer"
                data-row-key={rowKey}
                role="button"
                tabIndex={0}
                aria-expanded={open}
                onClick={() => onToggleRow(f)}
                onKeyDown={(event) => {
                  if (event.target !== event.currentTarget || (event.key !== "Enter" && event.key !== " ")) return;
                  event.preventDefault();
                  onToggleRow(f);
                }}
              >
                <TableCell onClick={(e) => e.stopPropagation()}>
                  {f.finding_id && (
                    <Checkbox
                      checked={selectedIds.has(f.finding_id)}
                      onCheckedChange={(c) => onToggleSelected(f.finding_id as string, c === true)}
                      aria-label={t("Select this vulnerability")}
                    />
                  )}
                </TableCell>
                <TableCell>
                  <ChevronRightIcon
                    className={cn("size-4 text-muted-foreground transition-transform", open && "rotate-90")}
                  />
                </TableCell>
                <TableCell>
                  <StatusBadge domain="severity" value={f.severity} dot />
                </TableCell>
                <TableCell className="max-w-md">
                  <div className="flex min-w-0 flex-col gap-0.5">
                    {f.finding_id ? (
                      <Link
                        href={`/function/findings/detail?id=${f.finding_id}`}
                        onClick={(e) => e.stopPropagation()}
                        className="truncate font-medium hover:text-primary hover:underline"
                        title={t("View finding details")}
                      >
                        {f.name || f.vulnclass || t("Uncategorized")}
                      </Link>
                    ) : (
                      <span className="truncate font-medium">{f.name || f.vulnclass || t("Uncategorized")}</span>
                    )}
                    <span className="truncate text-xs text-muted-foreground">{f.summary}</span>
                    <CodeLocation source={f.source} />
                    <div className="flex flex-wrap items-center gap-1">
                      <LaneBadge lane={f.source?.lane} />
                      <Badge variant="outline">{t("Traffic evidence: {count}", { count: f.traffic_count ?? 0 })}</Badge>
                      <VerificationBadge finding={f} />
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  {f.assets && f.assets.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {f.assets.slice(0, 3).map((a) => (
                        <code
                          key={a.id}
                          className="max-w-full truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs"
                          title={`${a.type} · ${a.label}`}
                        >
                          {a.label}
                        </code>
                      ))}
                      {f.assets.length > 3 && (
                        <span className="text-xs text-muted-foreground">+{f.assets.length - 3}</span>
                      )}
                    </div>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  {f.finding_id && canTriage ? (
                    <Select value={f.status} onValueChange={(v) => onStatusChange(f, v as FindingStatus)}>
                      <SelectTrigger size="sm" className="h-7 w-full border-none px-1 shadow-none focus-visible:ring-0">
                        <StatusBadge domain="finding" value={f.status} dot />
                      </SelectTrigger>
                      <SelectContent position="popper" align="end">
                        {FINDING_STATUSES.map((st) => (
                          <SelectItem key={st} value={st}>
                            {statusMeta("finding", st).label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <StatusBadge domain="finding" value={f.status} dot />
                  )}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  {f.finding_id && canTriage ? (
                    <Select
                      value={f.report_state ?? "none"}
                      onValueChange={(v) => onReportStateChange(f, v as FindingReportState)}
                    >
                      <SelectTrigger size="sm" className="h-7 w-full border-none px-1 shadow-none focus-visible:ring-0">
                        <ReportStateBadge state={f.report_state} />
                      </SelectTrigger>
                      <SelectContent position="popper" align="end">
                        {FINDING_REPORT_STATES.map((st) => (
                          <SelectItem key={st} value={st}>
                            {REPORT_STATE_META[st].label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <ReportStateBadge state={f.report_state} />
                  )}
                </TableCell>
                <TableCell>
                  {f.task_id ? (
                    <Link
                      href={`/function/tasks/detail?id=${f.task_id}`}
                      onClick={(e) => e.stopPropagation()}
                      className="inline-flex max-w-full items-center gap-1 text-primary hover:underline"
                      title={f.task_description}
                    >
                      <span className="truncate">{f.task_description}</span>
                      <ArrowUpRightIcon className="size-3 shrink-0" />
                    </Link>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground tabular-nums">{fmtTime(f.ts)}</TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <div className="flex items-center gap-1">
                    {retest ? (
                      <Button asChild size="sm" variant="ghost">
                        <Link href={`/chat?c=${retest.conversation_id}`} title={t("View the retest in progress")}>
                          <Spinner data-icon="inline-start" />
                          {t("Retesting")}
                        </Link>
                      </Button>
                    ) : null}
                    {canOperate && !retest && f.finding_id && !f.inherited ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onRetest(f)}
                        title={t("Retest this vulnerability in a separate conversation")}
                      >
                        <RotateCcwIcon data-icon="inline-start" />
                        {t("Retest")}
                      </Button>
                    ) : null}
                    {canOperate && f.finding_id && f.task_id && (
                      <Button size="sm" variant="ghost" onClick={() => onDeepen(f)}>
                        <FlaskConicalIcon data-icon="inline-start" />
                        {t("Deepen")}
                      </Button>
                    )}
                    {canOperate && f.finding_id && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            size="icon"
                            variant="ghost"
                            className="size-7 text-muted-foreground hover:text-destructive"
                            aria-label={t("Delete vulnerability")}
                          >
                            <Trash2Icon className="size-4" />
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>{t("Delete this vulnerability?")}</AlertDialogTitle>
                            <AlertDialogDescription className="[overflow-wrap:anywhere]">
                              {t(
                                '"{name}" will be permanently deleted and removed from the findings list, the task findings tab, and the exploration graph. This action cannot be undone.',
                                { name: f.name || f.vulnclass || f.summary || `#${f.finding_id}` },
                              )}
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>{t("Cancel")}</AlertDialogCancel>
                            <AlertDialogAction onClick={() => onDelete(f)}>{t("Delete")}</AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </div>
                </TableCell>
              </TableRow>
              {open && (
                <FindingExpandedRow
                  finding={f}
                  columnCount={COLUMN_COUNT}
                  report={reports[rowKey]}
                  edit={canTriage ? edit : null}
                  onEditChange={onEditChange}
                  saving={saving}
                  onSave={onSave}
                  triage={
                    canTriage && f.finding_id ? (
                      <TriageBar
                        finding={f}
                        busy={triageBusy}
                        onVerdict={verdict}
                        onNav={nav}
                        hasPrev={openIndex > 0}
                        hasNext={openIndex < items.length - 1}
                      />
                    ) : undefined
                  }
                />
              )}
            </React.Fragment>
          );
        })}
        {items.length === 0 && (
          <TableRow>
            <TableCell colSpan={COLUMN_COUNT} className="py-12 text-center text-sm text-muted-foreground">
              {t("No matching findings.")}
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}
