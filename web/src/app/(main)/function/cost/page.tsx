"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2Icon } from "lucide-react";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { CostGroupBy, CostRow } from "@/lib/types";
import { COST_GROUPS, COST_RANGES, CostControls } from "./_components/cost-controls";
import { CostKpis } from "./_components/cost-kpis";
import { totalRow } from "./_components/cost-metrics";
import { CostTable } from "./_components/cost-table";

const DEFAULT_BY: CostGroupBy = "task";
const DEFAULT_DAYS = 30;

function CostInner() {
  const router = useRouter();
  const params = useSearchParams();

  const rawBy = params.get("by");
  const by = COST_GROUPS.find((g) => g.value === rawBy)?.value ?? DEFAULT_BY;
  const rawDays = params.get("days");
  const days = COST_RANGES.find((r) => String(r.days) === rawDays)?.days ?? DEFAULT_DAYS;

  const [rows, setRows] = React.useState<CostRow[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let cancelled = false;
    setLoading(true);
    api
      .llmCost(by, days)
      .then((r) => {
        if (cancelled) return;
        setRows(r.rows ?? []);
        setError(null);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [by, days]);

  const setQuery = (nextBy: CostGroupBy, nextDays: number) =>
    router.replace(`/function/cost?by=${nextBy}&days=${nextDays}`, { scroll: false });

  const total = React.useMemo(() => totalRow(rows), [rows]);

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">{t("Cost & efficiency")}</h1>
        <CostControls by={by} days={days} onBy={(v) => setQuery(v, days)} onDays={(v) => setQuery(by, v)} />
      </div>
      <CostKpis total={total} />
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="relative">
        {loading && <Loader2Icon className="absolute right-2 top-2 size-4 animate-spin text-muted-foreground" />}
        <CostTable rows={rows} by={by} />
      </div>
      {total.unpriced_calls > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("* Some calls come from profiles with no price set; cost covers priced calls only.")}
        </p>
      )}
    </div>
  );
}

// useSearchParams must sit under a Suspense boundary for static export.
export default function CostPage() {
  return (
    <React.Suspense fallback={null}>
      <CostInner />
    </React.Suspense>
  );
}
