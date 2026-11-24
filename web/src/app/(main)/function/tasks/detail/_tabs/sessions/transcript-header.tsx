import { ClockIcon, Trash2Icon } from "lucide-react";

import { SideQuestionButton } from "@/components/side-question-workspace";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { SideQuestions } from "@/hooks/use-side-questions";
import { intlLocale, t } from "@/i18n";
import type { IntentAsset, Session, TaskLLMResolution, TokenTotal } from "@/lib/types";

import { fmtDuration, resolutionLabel } from "./helpers";
import { TokenMetrics, WorkerAssetBadge } from "./session-item";
import type { SessionState } from "./store";
import type { SessionMetaEntry } from "./use-intents";

// TranscriptHeader is the active session's title row (config / assets / liveness /
// token + duration metrics) plus the deleted-intent banner.
export function TranscriptHeader({
  active,
  sessionMeta,
  side,
  activeResolution,
  activeAssets,
  activeState,
  activeTokens,
  runDuration,
  isMain,
}: {
  active: Session;
  sessionMeta: Map<string, SessionMetaEntry>;
  side: SideQuestions;
  activeResolution: TaskLLMResolution | undefined;
  activeAssets: IntentAsset[] | undefined;
  activeState: SessionState | undefined;
  activeTokens: TokenTotal;
  runDuration: number | null;
  isMain: boolean;
}) {
  const tokenTotal = {
    i: activeTokens.input_tokens,
    o: activeTokens.output_tokens,
    cr: activeTokens.cache_read_tokens,
    any:
      activeTokens.input_tokens +
        activeTokens.output_tokens +
        activeTokens.cache_read_tokens +
        activeTokens.cache_write_tokens >
      0,
  };
  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 border-b px-3 py-2 sm:px-4 sm:py-2.5">
        {(() => {
          const isWorker = active.role === "worker";
          const meta = isWorker ? sessionMeta.get(active.id) : undefined;
          // Worker: the intent moved into the transcript as a message, so the
          // header shows a stable generic label (intent JSON stays on hover).
          const title = isWorker ? t("Worker execution session") : active.title;
          const titleEl = <span className="min-w-0 truncate text-sm font-medium">{title}</span>;
          return meta?.json ? (
            <Tooltip>
              <TooltipTrigger asChild>{titleEl}</TooltipTrigger>
              <TooltipContent side="bottom" align="start" className="max-h-80 max-w-sm overflow-auto p-0">
                <pre className="p-2 text-[10px] leading-relaxed">{JSON.stringify(meta.json, null, 2)}</pre>
              </TooltipContent>
            </Tooltip>
          ) : (
            titleEl
          );
        })()}
        <SideQuestionButton side={side} />
        {activeResolution && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                variant="outline"
                className="max-w-28 shrink-0 font-normal"
                aria-label={
                  activeResolution.available
                    ? t("Current config: {name}", { name: resolutionLabel(activeResolution) })
                    : t("Model unavailable")
                }
              >
                <span className="truncate">
                  {activeResolution.available ? resolutionLabel(activeResolution) : t("Model unavailable")}
                </span>
              </Badge>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-xs [overflow-wrap:anywhere]">
              {activeResolution.available
                ? [resolutionLabel(activeResolution), activeResolution.model].filter(Boolean).join(" / ")
                : activeResolution.reason || t("No available LLM config")}
            </TooltipContent>
          </Tooltip>
        )}
        {activeAssets && activeAssets.length > 0 && <WorkerAssetBadge assets={activeAssets} />}
        {active.inherited && active.source_task_id && (
          <Badge variant="outline">{t("Source task #{id} · read-only history", { id: active.source_task_id })}</Badge>
        )}
        {active.live && (
          <span className="inline-flex items-center gap-1 rounded bg-blue-500/15 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">
            <span className="size-1 animate-pulse rounded-full bg-blue-500" />
            {t("Live")}
          </span>
        )}
        {activeState?.hasMore && (
          <span className="text-[10px] text-muted-foreground" title={t("Scroll up to load earlier history")}>
            ↑ {t("Earlier history")}
          </span>
        )}
        <div className="ml-auto flex min-w-0 max-w-full items-center justify-end gap-x-3 gap-y-1 text-xs text-muted-foreground max-sm:w-full max-sm:flex-wrap">
          {tokenTotal.any && (
            <Tooltip>
              {/* Mobile uses short labels (In/Cache/Out): long labels wrap this row onto two lines, further squeezing the transcript. */}
              <TooltipTrigger asChild>
                <span className="inline-flex min-w-0 items-center">
                  <TokenMetrics
                    input={tokenTotal.i}
                    cache={tokenTotal.cr}
                    output={tokenTotal.o}
                    labels="short"
                    className="sm:hidden"
                  />
                  <TokenMetrics
                    input={tokenTotal.i}
                    cache={tokenTotal.cr}
                    output={tokenTotal.o}
                    labels="long"
                    className="max-sm:hidden"
                  />
                </span>
              </TooltipTrigger>
              <TooltipContent>
                {t("Input {input} · Output {output} · Cache read {cacheRead} · Cache write {cacheWrite}", {
                  input: activeTokens.input_tokens.toLocaleString(intlLocale),
                  output: activeTokens.output_tokens.toLocaleString(intlLocale),
                  cacheRead: activeTokens.cache_read_tokens.toLocaleString(intlLocale),
                  cacheWrite: activeTokens.cache_write_tokens.toLocaleString(intlLocale),
                })}
              </TooltipContent>
            </Tooltip>
          )}
          {runDuration != null && (
            <span className="inline-flex items-center gap-1" title={t("Run duration (first step → last step)")}>
              <ClockIcon className="size-3" />
              {fmtDuration(runDuration)}
            </span>
          )}
          {isMain && <span>{t("Interactive")}</span>}
        </div>
      </div>
      {(() => {
        const dm = active.role === "worker" ? sessionMeta.get(active.id) : undefined;
        if (!dm?.deleted) return null;
        return (
          <div className="flex items-start gap-2 border-b border-destructive/30 bg-destructive/5 px-4 py-2.5 text-xs">
            <Trash2Icon className="mt-0.5 size-3.5 shrink-0 text-destructive" />
            <div className="min-w-0">
              <span className="font-medium text-destructive">{t("This intent was deleted by the user")}</span>
              <span className="text-muted-foreground">
                {t(
                  "(execution stopped and the planner notified; the intent and its output are kept — view the history below)",
                )}
              </span>
              {dm.deleteReason && (
                <p className="mt-1 break-words text-foreground">
                  <span className="text-muted-foreground">{t("Deletion reason:")} </span>
                  {dm.deleteReason}
                </p>
              )}
            </div>
          </div>
        );
      })()}
    </>
  );
}
