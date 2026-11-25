"use client";

import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { decodeTraceAfter, formatHeaderLines, isTraceLineMasked } from "@/lib/tamper-hooks";
import type { TamperDryRunResult } from "@/lib/types";

export function DryRunResult({ result: dryRunResult }: { result: TamperDryRunResult }) {
  return (
    <div className="space-y-3 border-t pt-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{t("Dry-run result")}</span>
        <Badge variant={dryRunResult.changed ? "default" : "secondary"}>
          {dryRunResult.changed ? t("Modified") : t("Unchanged")}
        </Badge>
      </div>

      {/* Trace lines */}
      <div className="space-y-1.5">
        <div className="text-xs font-medium text-muted-foreground">
          {t("Trace lines ({count})", { count: dryRunResult.lines?.length ?? 0 })}
        </div>
        {!dryRunResult.lines || dryRunResult.lines.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("No trace lines recorded (matcher did not match or hook made no changes).")}
          </p>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-xs">{t("Field")}</TableHead>
                  <TableHead className="text-xs">{t("Outcome")}</TableHead>
                  <TableHead className="text-xs">{t("After / Masked")}</TableHead>
                  <TableHead className="text-xs">{t("Hashes")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {dryRunResult.lines.map((line, idx) => {
                  const masked = isTraceLineMasked(line);
                  const decodedAfter = decodeTraceAfter(line.After);
                  return (
                    <TableRow key={`${line.Field}-${line.Outcome}-${idx}`}>
                      <TableCell className="font-mono text-xs">
                        <div>{line.Field || "—"}</div>
                        <div className="text-[11px] text-muted-foreground">
                          {line.Phase} · {line.Action}
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        <Badge
                          variant={
                            line.Outcome === "error"
                              ? "destructive"
                              : line.Outcome === "applied"
                                ? "default"
                                : "secondary"
                          }
                          className="font-mono text-[11px]"
                        >
                          {line.Outcome}
                        </Badge>
                        {line.Reason && (
                          <div className="mt-1 font-mono text-[11px] text-muted-foreground">{line.Reason}</div>
                        )}
                      </TableCell>
                      <TableCell className="max-w-xs font-mono text-xs">
                        {masked ? (
                          <Badge variant="outline" className="border-amber-500/50 text-amber-600 dark:text-amber-400">
                            {t("Masked (HMAC)")}
                          </Badge>
                        ) : decodedAfter !== null ? (
                          <span className="break-all">{decodedAfter.slice(0, 160)}</span>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-[11px] text-muted-foreground">
                        {line.BeforeHash && <div>{line.BeforeHash.slice(0, 12)}</div>}
                        {line.AfterHash && <div>→ {line.AfterHash.slice(0, 12)}</div>}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* Output preview */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <div className="text-xs font-medium text-muted-foreground">{t("Resulting request")}</div>
          <pre className="max-h-48 overflow-auto rounded-md border bg-muted/40 p-2 font-mono text-xs">
            {`${dryRunResult.request.method} ${dryRunResult.request.url}\n${formatHeaderLines(dryRunResult.request.headers)}${dryRunResult.request.body ? `\n\n${dryRunResult.request.body}` : ""}`}
          </pre>
        </div>
        <div className="space-y-1">
          <div className="text-xs font-medium text-muted-foreground">{t("Resulting response")}</div>
          <pre className="max-h-48 overflow-auto rounded-md border bg-muted/40 p-2 font-mono text-xs">
            {`HTTP ${dryRunResult.response.status}\n${formatHeaderLines(dryRunResult.response.headers)}${dryRunResult.response.body ? `\n\n${dryRunResult.response.body}` : ""}`}
          </pre>
        </div>
      </div>
    </div>
  );
}
