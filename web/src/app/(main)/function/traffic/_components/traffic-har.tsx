"use client";

import * as React from "react";

import { DownloadIcon, Loader2Icon, UploadIcon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { TrafficHARImportResult } from "@/lib/types";

import type { TrafficFilterQuery } from "./use-traffic-filters";

const HAR_MAX_BYTES = 32 * 1024 * 1024;

// HAR 1.2 import/export state and handlers. `onImported` runs after a successful import.
export function useTrafficHar(filters: TrafficFilterQuery, onImported: () => void) {
  const inputRef = React.useRef<HTMLInputElement | null>(null);
  const [importing, setImporting] = React.useState(false);
  const [exporting, setExporting] = React.useState(false);
  const [summary, setSummary] = React.useState<TrafficHARImportResult | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setError(null);
    setSummary(null);
    if (file.size > HAR_MAX_BYTES) {
      const msg = t("HAR file exceeds the 32 MiB limit");
      setError(msg);
      toast.error(msg);
      return;
    }
    setImporting(true);
    try {
      const raw = await file.text();
      const res = await api.trafficImportHAR(raw);
      setSummary(res);
      const text = t("HAR import complete: {imported} imported, {skipped} skipped, {failed} failed", {
        imported: res.imported,
        skipped: res.skipped,
        failed: res.failed,
      });
      if (res.failed > 0 || res.truncated) toast.warning(text);
      else toast.success(text);
      onImported();
    } catch (e) {
      const msg = t("HAR import failed: {error}", { error: (e as Error).message });
      setError(msg);
      toast.error(msg);
    } finally {
      setImporting(false);
    }
  };

  const handleExport = async () => {
    setExporting(true);
    try {
      const meta = await api.trafficExportHAR(filters);
      if (meta.truncated) {
        toast.warning(
          t("Exported {exported} of {matched} matching exchanges (truncated: {reason})", {
            exported: meta.exported,
            matched: meta.matched,
            reason: meta.truncatedReason ?? "limit",
          }),
        );
      } else {
        toast.success(t("Exported {count} exchanges to HAR", { count: meta.exported }));
      }
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setExporting(false);
    }
  };

  return {
    inputRef,
    importing,
    exporting,
    summary,
    error,
    dismissSummary: () => setSummary(null),
    dismissError: () => setError(null),
    handleFileChange,
    handleExport,
  };
}

export type TrafficHar = ReturnType<typeof useTrafficHar>;

export function HarButtons({ har, hasRecords }: { har: TrafficHar; hasRecords: boolean }) {
  return (
    <>
      <input
        ref={har.inputRef}
        type="file"
        accept=".har,application/json"
        className="hidden"
        onChange={(e) => void har.handleFileChange(e)}
      />
      <Button
        variant="outline"
        size="sm"
        className="h-8"
        disabled={har.importing}
        onClick={() => har.inputRef.current?.click()}
      >
        {har.importing ? <Loader2Icon className="size-3.5 animate-spin" /> : <UploadIcon className="size-3.5" />}
        {har.importing ? t("Importing HAR…") : t("Import HAR")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        className="h-8"
        disabled={har.exporting || !hasRecords}
        onClick={() => void har.handleExport()}
      >
        {har.exporting ? <Loader2Icon className="size-3.5 animate-spin" /> : <DownloadIcon className="size-3.5" />}
        {har.exporting ? t("Exporting HAR…") : t("Export HAR")}
      </Button>
    </>
  );
}

export function HarAlerts({ har }: { har: TrafficHar }) {
  const { error, summary } = har;
  return (
    <>
      {error ? (
        <Alert variant="destructive" className="flex items-center justify-between gap-2 py-2">
          <AlertDescription className="text-xs">{error}</AlertDescription>
          <Button variant="ghost" size="icon-xs" aria-label={t("Dismiss")} onClick={har.dismissError}>
            <XIcon className="size-3.5" />
          </Button>
        </Alert>
      ) : null}

      {summary ? (
        <Alert className="flex items-start justify-between gap-2 py-2">
          <AlertDescription className="space-y-1 text-xs">
            <p>
              {t("HAR import complete: {imported} imported, {skipped} skipped, {failed} failed", {
                imported: summary.imported,
                skipped: summary.skipped,
                failed: summary.failed,
              })}
            </p>
            {summary.truncated ? (
              <p className="text-amber-600 dark:text-amber-400">
                {t("HAR import stopped early at the 32 MiB decoded body cap.")}
              </p>
            ) : null}
            {summary.errors && summary.errors.length > 0 ? (
              <ul className="list-disc pl-4 text-muted-foreground">
                {summary.errors.slice(0, 5).map((item) => (
                  <li key={`${item.index}-${item.reason}`} className="font-mono">
                    #{item.index}: {item.reason}
                  </li>
                ))}
              </ul>
            ) : null}
          </AlertDescription>
          <Button variant="ghost" size="icon-xs" aria-label={t("Dismiss")} onClick={har.dismissSummary}>
            <XIcon className="size-3.5" />
          </Button>
        </Alert>
      ) : null}
    </>
  );
}
