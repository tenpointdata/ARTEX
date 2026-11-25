"use client";

import * as React from "react";

import { CoinsIcon } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { ModelTokenStat } from "@/lib/types";

import { cacheHitRate, fmtTokens } from "./overview-format";

export function TokensCard({ taskId }: { taskId: string }) {
  // Token usage per model (from the always-on llm_usage metering ledger, exact per call).
  const [modelTokens, setModelTokens] = React.useState<ModelTokenStat[]>([]);
  const loadTokens = React.useCallback(async () => {
    try {
      const resp = await api.tokensByModel(taskId);
      setModelTokens(resp.models);
    } catch {
      // Ignore: without PG the endpoint errors and the card stays empty
    }
  }, [taskId]);

  // Token totals (across all models) for the card header overview.
  const tokenTotals = modelTokens.reduce(
    (acc, m) => {
      acc.input += m.input_tokens;
      acc.output += m.output_tokens;
      acc.cacheRead += m.cache_read_tokens;
      acc.cacheWrite += m.cache_write_tokens;
      acc.calls += m.calls;
      return acc;
    },
    { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, calls: 0 },
  );

  React.useEffect(() => {
    void loadTokens();
    return setVisibleInterval(() => {
      void loadTokens();
    }, 3000);
  }, [loadTokens]);

  return (
    <>
      {/* LLM token usage: grouped by model, data from llm_records (requires LLM recording enabled). */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <CoinsIcon className="size-4 text-amber-500" /> {t("LLM token usage")}
            <span className="text-muted-foreground text-xs font-normal">
              {tokenTotals.calls > 0
                ? t("(per model, {count} calls in total)", { count: tokenTotals.calls })
                : t("(per model)")}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {modelTokens.length > 0 ? (
            <>
              {/* Totals overview */}
              <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
                <span className="tabular-nums">
                  <span className="text-muted-foreground">{t("Input")} </span>
                  <span className="font-semibold">{fmtTokens(tokenTotals.input)}</span>
                </span>
                <span className="tabular-nums">
                  <span className="text-muted-foreground">{t("Output")} </span>
                  <span className="font-semibold">{fmtTokens(tokenTotals.output)}</span>
                </span>
                <span className="tabular-nums">
                  <span className="text-muted-foreground">{t("Cache read")} </span>
                  <span className="font-semibold">{fmtTokens(tokenTotals.cacheRead)}</span>
                </span>
                <span className="tabular-nums">
                  <span className="text-muted-foreground">{t("Cache hit rate")} </span>
                  <span className="font-semibold text-emerald-500">
                    {cacheHitRate(tokenTotals.cacheRead, tokenTotals.input)}
                  </span>
                </span>
              </div>
              {/* Per-model detail table */}
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-muted-foreground border-b text-left text-xs">
                      <th className="py-1.5 pr-3 font-medium">{t("Model")}</th>
                      <th className="py-1.5 pr-3 text-right font-medium">{t("Calls")}</th>
                      <th className="py-1.5 pr-3 text-right font-medium">{t("Input")}</th>
                      <th className="py-1.5 pr-3 text-right font-medium">{t("Output")}</th>
                      <th className="py-1.5 pr-3 text-right font-medium">{t("Cache read")}</th>
                      <th className="py-1.5 text-right font-medium">{t("Hit rate")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {modelTokens.map((m) => (
                      <tr key={m.model} className="border-b last:border-0">
                        <td className="max-w-[16rem] truncate py-1.5 pr-3 font-mono text-xs" title={m.model}>
                          {m.model}
                        </td>
                        <td className="py-1.5 pr-3 text-right tabular-nums">{m.calls}</td>
                        <td className="py-1.5 pr-3 text-right tabular-nums">{fmtTokens(m.input_tokens)}</td>
                        <td className="py-1.5 pr-3 text-right tabular-nums">{fmtTokens(m.output_tokens)}</td>
                        <td className="py-1.5 pr-3 text-right tabular-nums">{fmtTokens(m.cache_read_tokens)}</td>
                        <td className="py-1.5 text-right tabular-nums text-emerald-500">
                          {cacheHitRate(m.cache_read_tokens, m.input_tokens)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("No LLM usage yet (the task has made no calls, or records are still being written).")}
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}
