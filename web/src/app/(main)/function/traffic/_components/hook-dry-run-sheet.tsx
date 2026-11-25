"use client";

import * as React from "react";

import { AlertTriangleIcon, Loader2Icon, PlayIcon } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { exchangeDetailToDryRunIO, formatHeaderLines, parseHeaderLines } from "@/lib/tamper-hooks";
import type { TamperDryRunRequest, TamperDryRunResult, TrafficExchange } from "@/lib/types";

import { DryRunResult } from "./hook-dry-run-result";
import type { DryRunTarget } from "./hook-editor-state";

interface HookDryRunSheetProps {
  /** The hook (or unsaved draft) to run; null closes the sheet. */
  target: DryRunTarget | null;
  onClose: () => void;
  recentExchanges: TrafficExchange[];
}

// Sandbox dry-run of a stored hook or an unsaved draft against a recorded or synthetic exchange.
export function HookDryRunSheet({ target: dryRunTarget, onClose, recentExchanges }: HookDryRunSheetProps) {
  const [dryRunInputMode, setDryRunInputMode] = React.useState<"synthetic" | "exchange">("synthetic");
  const [dryRunExchangeId, setDryRunExchangeId] = React.useState("");
  const [dryRunLoadingExchange, setDryRunLoadingExchange] = React.useState(false);
  const [dryRunExchangeWarning, setDryRunExchangeWarning] = React.useState<string | null>(null);
  const [dryRunMethod, setDryRunMethod] = React.useState("GET");
  const [dryRunUrl, setDryRunUrl] = React.useState("https://example.test/api/v1/items");
  const [dryRunReqHeaders, setDryRunReqHeaders] = React.useState("Accept: application/json");
  const [dryRunReqBody, setDryRunReqBody] = React.useState("");
  const [dryRunStatus, setDryRunStatus] = React.useState("200");
  const [dryRunRespHeaders, setDryRunRespHeaders] = React.useState("Content-Type: application/json");
  const [dryRunRespBody, setDryRunRespBody] = React.useState('{"ok":true}');
  const [dryRunRunning, setDryRunRunning] = React.useState(false);
  const [dryRunError, setDryRunError] = React.useState<string | null>(null);
  const [dryRunResult, setDryRunResult] = React.useState<TamperDryRunResult | null>(null);

  // Opening on a new target starts from a clean result.
  React.useEffect(() => {
    if (!dryRunTarget) return;
    setDryRunError(null);
    setDryRunResult(null);
    setDryRunExchangeWarning(null);
  }, [dryRunTarget]);

  const loadExchangeIntoDryRun = async (id: string) => {
    const trimmed = id.trim();
    if (!trimmed) return;
    setDryRunLoadingExchange(true);
    setDryRunError(null);
    setDryRunExchangeWarning(null);
    try {
      const detail = await api.trafficExchange(trimmed);
      const matched = recentExchanges.find((ex) => ex.id === trimmed);
      const converted = exchangeDetailToDryRunIO(detail, matched?.url);
      setDryRunMethod(converted.io.request.method);
      setDryRunUrl(converted.io.request.url);
      setDryRunReqHeaders(formatHeaderLines(converted.io.request.headers));
      setDryRunReqBody(converted.io.request.body);
      setDryRunStatus(String(converted.io.response.status));
      setDryRunRespHeaders(formatHeaderLines(converted.io.response.headers));
      setDryRunRespBody(converted.io.response.body);
      const warnings: string[] = [];
      if (converted.reqBinary || converted.respBinary) {
        warnings.push(t("Binary body detected; binary bodies are skipped by hooks in 0.7."));
      }
      if (converted.reqPreviewOnly || converted.respPreviewOnly) {
        warnings.push(t("Body was truncated in the recorded preview; only the inline preview was loaded."));
      }
      if (warnings.length) setDryRunExchangeWarning(warnings.join(" "));
      toast.success(t("Loaded exchange {id}", { id: trimmed }));
    } catch (e) {
      setDryRunError((e as Error).message);
    } finally {
      setDryRunLoadingExchange(false);
    }
  };

  const runDryRun = async () => {
    if (!dryRunTarget) return;
    setDryRunRunning(true);
    setDryRunError(null);
    setDryRunResult(null);
    const statusNum = Number.parseInt(dryRunStatus, 10);
    const body: TamperDryRunRequest = {
      ...(dryRunTarget.hookId
        ? { hook_id: dryRunTarget.hookId }
        : {
            name: dryRunTarget.inline?.name,
            phase: dryRunTarget.inline?.phase,
            matchers: dryRunTarget.inline?.matchers,
            source: dryRunTarget.inline?.source,
          }),
      request: {
        method: dryRunMethod.trim().toUpperCase() || "GET",
        url: dryRunUrl.trim() || "https://example.test/",
        headers: parseHeaderLines(dryRunReqHeaders),
        body: dryRunReqBody,
      },
      response: {
        status: Number.isNaN(statusNum) ? 200 : statusNum,
        headers: parseHeaderLines(dryRunRespHeaders),
        body: dryRunRespBody,
      },
    };
    try {
      const res = await api.dryRunTrafficHook(body);
      setDryRunResult(res);
    } catch (e) {
      setDryRunError((e as Error).message);
    } finally {
      setDryRunRunning(false);
    }
  };

  return (
    <Sheet open={dryRunTarget !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-3xl">
        <SheetHeader>
          <SheetTitle>{t("Dry-run: {target}", { target: dryRunTarget?.label ?? "" })}</SheetTitle>
          <SheetDescription>
            {t(
              "Runs the hook in the sandbox against a recorded exchange or synthetic HTTP message without touching live traffic.",
            )}
          </SheetDescription>
        </SheetHeader>

        <div className="mt-4 space-y-4">
          <Tabs value={dryRunInputMode} onValueChange={(v) => setDryRunInputMode(v as "synthetic" | "exchange")}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="exchange">{t("From exchange ID")}</TabsTrigger>
              <TabsTrigger value="synthetic">{t("Synthetic input")}</TabsTrigger>
            </TabsList>

            <TabsContent value="exchange" className="space-y-3 pt-2">
              <div className="flex items-end gap-2">
                <div className="flex-1 space-y-1">
                  <Label htmlFor="dryrun-ex-id" className="text-xs">
                    {t("Exchange ID")}
                  </Label>
                  <Input
                    id="dryrun-ex-id"
                    className="h-8 font-mono text-xs"
                    value={dryRunExchangeId}
                    placeholder={t("Paste a recorded exchange ID")}
                    onChange={(e) => setDryRunExchangeId(e.target.value)}
                  />
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8"
                  disabled={dryRunLoadingExchange || !dryRunExchangeId.trim()}
                  onClick={() => void loadExchangeIntoDryRun(dryRunExchangeId)}
                >
                  {dryRunLoadingExchange && <Loader2Icon className="size-3.5 animate-spin" />}
                  {t("Load exchange")}
                </Button>
              </div>

              {recentExchanges.length > 0 && (
                <div className="space-y-1">
                  <Label className="text-xs text-muted-foreground">{t("Or pick a recent captured exchange")}</Label>
                  <Select
                    value=""
                    onValueChange={(id) => {
                      setDryRunExchangeId(id);
                      void loadExchangeIntoDryRun(id);
                    }}
                  >
                    <SelectTrigger className="h-8 font-mono text-xs">
                      <SelectValue placeholder={t("Select recent exchange…")} />
                    </SelectTrigger>
                    <SelectContent>
                      {recentExchanges.slice(0, 20).map((ex) => (
                        <SelectItem key={ex.id} value={ex.id} className="font-mono text-xs">
                          {ex.method} {ex.host}
                          {ex.url.replace(/^https?:\/\/[^/]+/, "").slice(0, 48)} ({ex.status})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              {dryRunExchangeWarning && (
                <Alert className="border-amber-500/40 bg-amber-500/10 py-2">
                  <AlertTriangleIcon className="size-4 text-amber-500" />
                  <AlertDescription className="text-xs">{dryRunExchangeWarning}</AlertDescription>
                </Alert>
              )}
            </TabsContent>

            <TabsContent value="synthetic" className="pt-1">
              <p className="text-xs text-muted-foreground">
                {t("Edit the synthetic request and response below, then click Run dry-run.")}
              </p>
            </TabsContent>
          </Tabs>

          {/* Editable Request / Response fields (also populated when an exchange is loaded) */}
          <div className="grid grid-cols-1 gap-3 rounded-md border p-3 sm:grid-cols-2">
            <div className="space-y-2">
              <div className="text-xs font-medium">{t("Request")}</div>
              <div className="flex gap-2">
                <Input
                  className="h-8 w-24 font-mono text-xs"
                  value={dryRunMethod}
                  onChange={(e) => setDryRunMethod(e.target.value)}
                  aria-label={t("Method")}
                />
                <Input
                  className="h-8 flex-1 font-mono text-xs"
                  value={dryRunUrl}
                  onChange={(e) => setDryRunUrl(e.target.value)}
                  aria-label={t("URL")}
                />
              </div>
              <Textarea
                rows={3}
                className="font-mono text-xs"
                value={dryRunReqHeaders}
                placeholder="Header: value"
                onChange={(e) => setDryRunReqHeaders(e.target.value)}
              />
              <Textarea
                rows={3}
                className="font-mono text-xs"
                value={dryRunReqBody}
                placeholder={t("Request body")}
                onChange={(e) => setDryRunReqBody(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <div className="text-xs font-medium">{t("Response")}</div>
              <Input
                className="h-8 w-28 font-mono text-xs"
                type="number"
                value={dryRunStatus}
                onChange={(e) => setDryRunStatus(e.target.value)}
                aria-label={t("Status")}
              />
              <Textarea
                rows={3}
                className="font-mono text-xs"
                value={dryRunRespHeaders}
                placeholder="Header: value"
                onChange={(e) => setDryRunRespHeaders(e.target.value)}
              />
              <Textarea
                rows={3}
                className="font-mono text-xs"
                value={dryRunRespBody}
                placeholder={t("Response body")}
                onChange={(e) => setDryRunRespBody(e.target.value)}
              />
            </div>
          </div>

          <div className="flex justify-end">
            <Button type="button" size="sm" onClick={() => void runDryRun()} disabled={dryRunRunning}>
              {dryRunRunning ? <Loader2Icon className="size-3.5 animate-spin" /> : <PlayIcon className="size-3.5" />}
              {t("Run dry-run")}
            </Button>
          </div>

          {dryRunError && (
            <Alert variant="destructive">
              <AlertTriangleIcon className="size-4" />
              <AlertDescription className="font-mono text-xs whitespace-pre-wrap">{dryRunError}</AlertDescription>
            </Alert>
          )}

          {dryRunResult && <DryRunResult result={dryRunResult} />}
        </div>
      </SheetContent>
    </Sheet>
  );
}
