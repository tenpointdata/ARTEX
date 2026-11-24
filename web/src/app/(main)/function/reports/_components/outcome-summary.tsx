"use client";

import * as React from "react";

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { TaskOutcome } from "@/lib/governance-types";

// OutcomeSummary is one engagement's result in a single row: finding verdicts,
// confirmed findings by severity, and retest verdicts (GET /api/tasks/{id}/outcome).
export function OutcomeSummary({ taskId }: { taskId: string }) {
  const [outcome, setOutcome] = React.useState<TaskOutcome | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let disposed = false;
    setOutcome(null);
    setError(null);
    governanceApi
      .taskOutcome(taskId)
      .then((o) => {
        if (!disposed) setOutcome(o);
      })
      .catch((e: unknown) => {
        if (!disposed) setError(errorText(e));
      });
    return () => {
      disposed = true;
    };
  }, [taskId]);

  if (error) return <p className="text-destructive text-sm">{error}</p>;
  if (!outcome) return <p className="text-muted-foreground text-sm">{t("Loading…")}</p>;
  const f = outcome.findings;
  const r = outcome.retests;
  const cells: [string, number][] = [
    [t("Findings"), f.total],
    [t("Confirmed"), f.confirmed],
    [t("Open"), f.open],
    [t("False positive"), f.false_positive],
    [t("Critical"), f.critical],
    [t("High"), f.high],
    [t("Medium"), f.medium],
    [t("Low"), f.low],
    [t("Retest: fixed"), r.fixed],
    [t("Retest: reproduced"), r.reproduced],
  ];
  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            {cells.map(([label]) => (
              <TableHead key={label} className="text-right">
                {label}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            {cells.map(([label, n]) => (
              <TableCell key={label} className="text-right tabular-nums">
                {n}
              </TableCell>
            ))}
          </TableRow>
        </TableBody>
      </Table>
    </div>
  );
}
