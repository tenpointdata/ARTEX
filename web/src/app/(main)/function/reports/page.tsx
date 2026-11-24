"use client";

import * as React from "react";

import Link from "next/link";

import { BugIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { ClientTask } from "@/lib/governance-types";
import { useCompanyStore } from "@/stores/company/company-store";

import { ReportTab } from "../tasks/detail/_tabs/report-tab";
import { ClientTaskTable } from "./_components/client-task-table";
import { OutcomeSummary } from "./_components/outcome-summary";

// The client summary: the active company's engagements, and the selected
// one's outcome and report. Every call here is open to client_read and bound
// to the company (server/client_scope.go); the newest engagement is selected.
export default function ReportsPage() {
  const companyId = useCompanyStore((s) => s.companyId);
  const companyName = useCompanyStore((s) => s.companyName);
  const [tasks, setTasks] = React.useState<ClientTask[] | null>(null);
  const [selected, setSelected] = React.useState<string | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    if (companyId === null) return;
    let disposed = false;
    setTasks(null);
    setError(null);
    governanceApi
      .myTasks(companyId)
      .then((rows) => {
        if (disposed) return;
        setTasks(rows);
        setSelected(rows[0]?.id ?? null);
      })
      .catch((e: unknown) => {
        if (!disposed) setError(errorText(e));
      });
    return () => {
      disposed = true;
    };
  }, [companyId]);

  return (
    <div className="space-y-4 p-6">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h1 className="font-semibold text-xl">{t("Reports")}</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            {companyName
              ? t("Engagements for {company}", { company: companyName })
              : t("Pick a company in the sidebar to see its engagements.")}
          </p>
        </div>
        {companyId !== null && (
          <Button variant="outline" size="sm" asChild>
            <Link prefetch={false} href="/function/findings">
              <BugIcon /> {t("All findings")}
            </Link>
          </Button>
        )}
      </div>
      {error && <p className="rounded-md border p-4 text-destructive text-sm">{error}</p>}
      {companyId !== null && !error && tasks === null && (
        <p className="text-muted-foreground text-sm">{t("Loading…")}</p>
      )}
      {tasks?.length === 0 && (
        <p className="rounded-md border p-4 text-muted-foreground text-sm">
          {t("No engagements for this company yet.")}
        </p>
      )}
      {tasks && tasks.length > 0 && (
        <>
          <div className="rounded-md border">
            <ClientTaskTable tasks={tasks} selected={selected} onSelect={setSelected} />
          </div>
          {selected && (
            <div className="space-y-4">
              <OutcomeSummary taskId={selected} />
              <ReportTab taskId={selected} />
            </div>
          )}
        </>
      )}
    </div>
  );
}
