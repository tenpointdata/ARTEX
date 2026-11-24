"use client";

import * as React from "react";

import { Loader2Icon } from "lucide-react";

import { HttpCodeBlock } from "@/components/http-code-block";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { TrafficDetail, TrafficExchange } from "@/lib/types";
import { cn } from "@/lib/utils";

import { ReplayPanel } from "./replay-panel";
import { StreamMessagesPanel } from "./stream-messages";
import { fmtTime, METHODS, requestWithHost, statusTone } from "./traffic-format";
import { MethodBadge } from "./traffic-table";

function LoadingNote({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 p-5 text-xs text-muted-foreground">
      <Loader2Icon className="size-3.5 animate-spin" />
      {label}
    </div>
  );
}

interface TrafficDetailSheetProps {
  selected: TrafficExchange | null;
  onClose: () => void;
  onRecorded: () => void;
}

// Raw request/response of one exchange, plus stream messages and replay.
export function TrafficDetailSheet({ selected, onClose, onRecorded }: TrafficDetailSheetProps) {
  const [detail, setDetail] = React.useState<TrafficDetail | null>(null);
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [detailTab, setDetailTab] = React.useState("request");
  const streamKind = detail?.stream_kind === "websocket" || detail?.stream_kind === "sse" ? detail.stream_kind : null;
  const hasStream = streamKind !== null;

  // Lazy-load the raw request/response for the selected exchange. A new
  // selection opens on Request; a WebSocket/SSE exchange then opens on Stream.
  React.useEffect(() => {
    setDetailTab("request");
    if (!selected) {
      setDetail(null);
      return;
    }
    let alive = true;
    setDetailLoading(true);
    setDetail(null);
    api
      .trafficExchange(selected.id)
      .then((d) => {
        if (!alive) return;
        setDetail(d);
        if (d.stream_kind === "websocket" || d.stream_kind === "sse") setDetailTab("stream");
      })
      .catch(() => {
        if (alive) setDetail({ req: t("(failed to load the messages)"), resp: "" });
      })
      .finally(() => {
        if (alive) setDetailLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [selected]);

  return (
    <Sheet open={selected !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="w-full! max-w-none! gap-0 p-0 sm:w-[48rem]! sm:max-w-[48rem]!">
        {selected && (
          <>
            <SheetHeader className="border-b px-5 py-4">
              <div className="flex items-center gap-2 pr-8">
                <MethodBadge method={selected.method} />
                <Badge variant="secondary" className={cn("font-mono tabular-nums", statusTone(selected.status))}>
                  {selected.status}
                </Badge>
                {selected.rewritten ? (
                  <Badge
                    variant="outline"
                    className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                  >
                    {t("Rewritten")}
                  </Badge>
                ) : null}
                <span className="ml-auto text-xs text-muted-foreground tabular-nums">{fmtTime(selected.ts)}</span>
              </div>
              <SheetTitle className="break-all font-mono">{selected.host}</SheetTitle>
              <SheetDescription className="break-all font-mono">{selected.url}</SheetDescription>
            </SheetHeader>
            <Tabs value={detailTab} onValueChange={setDetailTab} className="min-h-0 flex-1 gap-0">
              <TabsList className={cn("mx-5 mt-4 grid w-auto", hasStream ? "grid-cols-4" : "grid-cols-3")}>
                <TabsTrigger value="request">{t("Request")}</TabsTrigger>
                <TabsTrigger value="response">{t("Response")}</TabsTrigger>
                {hasStream && (
                  <TabsTrigger value="stream">
                    {t("Stream")}
                    {(detail?.stream_count ?? 0) > 0 && (
                      <span className="text-muted-foreground text-xs tabular-nums">{detail?.stream_count}</span>
                    )}
                  </TabsTrigger>
                )}
                {hasStream ? (
                  <span title={t("stream replay is not supported")} className="flex">
                    <TabsTrigger value="replay" disabled>
                      {t("Replay")}
                    </TabsTrigger>
                  </span>
                ) : (
                  <TabsTrigger value="replay">{t("Replay")}</TabsTrigger>
                )}
              </TabsList>
              <TabsContent value="request" className="min-h-0 overflow-auto">
                {detailLoading ? (
                  <LoadingNote label={t("Loading messages…")} />
                ) : (
                  <HttpCodeBlock raw={requestWithHost(detail?.req ?? "", selected)} />
                )}
              </TabsContent>
              <TabsContent value="response" className="min-h-0 overflow-auto">
                {detailLoading ? (
                  <LoadingNote label={t("Loading messages…")} />
                ) : (
                  <HttpCodeBlock raw={detail?.resp ?? ""} />
                )}
              </TabsContent>
              {streamKind && (
                <TabsContent value="stream" className="min-h-0 overflow-auto">
                  <StreamMessagesPanel exchangeId={selected.id} kind={streamKind} />
                </TabsContent>
              )}
              {!hasStream && (
                // Kept mounted so edits survive a look at another tab.
                <TabsContent value="replay" forceMount className="min-h-0 overflow-auto data-[state=inactive]:hidden">
                  {detailLoading || !detail ? (
                    <LoadingNote label={t("Loading…")} />
                  ) : (
                    <ReplayPanel
                      exchange={selected}
                      raw={requestWithHost(detail.req, selected)}
                      methods={METHODS}
                      onRecorded={onRecorded}
                    />
                  )}
                </TabsContent>
              )}
            </Tabs>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
