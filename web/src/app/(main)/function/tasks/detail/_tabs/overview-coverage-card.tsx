"use client";

import { TargetIcon } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { t } from "@/i18n";
import { taskAssetTypeLabel } from "@/lib/task-assets";
import type { NewAssetType, TaskCoverage } from "@/lib/types";

import { MethodologyCoverage } from "./methodology-coverage";

export function CoverageCard({ coverage }: { coverage: TaskCoverage | null }) {
  return (
    <>
      {coverage &&
        coverage.enabled &&
        (coverage.scope_rows > 0 || (coverage.gaps?.length ?? 0) > 0 || !!coverage.methodologies?.length) && (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <TargetIcon className="size-4 text-emerald-500" /> {t("Asset test coverage")}
                <span className="text-muted-foreground text-xs font-normal">
                  {t("(rough estimate, for reference only)")}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <div className="flex items-baseline gap-3">
                <span className="text-2xl font-semibold tabular-nums">
                  {coverage.pct != null ? Math.round(coverage.pct * 100) + "%" : "—"}
                </span>
                <span className="text-muted-foreground text-sm">
                  {t("Tested {tested} / in scope {total}", { tested: coverage.tested, total: coverage.denominator })}
                </span>
              </div>
              {coverage.pct != null && <Progress value={Math.round(coverage.pct * 100)} />}
              {coverage.by_type.length > 0 && (
                <div className="flex flex-wrap gap-1.5 text-xs">
                  {coverage.by_type.map((b) => (
                    <span key={b.type} className="bg-muted rounded px-1.5 py-0.5">
                      <span className="text-muted-foreground">
                        {(taskAssetTypeLabel as (x: NewAssetType) => string | undefined)(b.type as NewAssetType) ||
                          b.type}
                      </span>{" "}
                      <span className="tabular-nums font-medium">
                        {b.tested}/{b.total}
                      </span>
                    </span>
                  ))}
                </div>
              )}
              {coverage.completeness && !coverage.completeness.complete && (
                <p className="text-muted-foreground text-xs">{coverage.completeness.caveat}</p>
              )}
              {coverage.gaps && (
                <div className="text-xs">
                  <span className="font-medium">{t("Coverage gaps: {count}", { count: coverage.gaps.length })}</span>
                  <ul className="text-muted-foreground mt-1 list-disc pl-4">
                    {coverage.gaps.map((g, i) => (
                      <li key={i}>
                        {t(g.kind)} {g.surface ? g.surface + " " : ""}
                        {g.risk_area}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {!!coverage.methodologies?.length && <MethodologyCoverage scores={coverage.methodologies} />}
            </CardContent>
          </Card>
        )}
    </>
  );
}
