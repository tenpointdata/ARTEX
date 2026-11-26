import { ChevronDownIcon, Loader2Icon, PlusIcon, RotateCwIcon, WifiOffIcon } from "lucide-react";

import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { MOCK } from "@/lib/mock/enabled";
import type { Session, TokenTotal } from "@/lib/types";
import { cn } from "@/lib/utils";

import { keyForSession, roleMeta } from "./helpers";
import { SessionItem, TokenMetrics } from "./session-item";
import type { SessionStore } from "./store";
import type { SessionMetaEntry } from "./use-intents";

type SessionRole = keyof typeof roleMeta;

export type SessionListProps = {
  listOpen: boolean;
  onToggleList: () => void;
  activeDisplayTitle: string;
  hiddenUnread: number;
  sseLive: boolean;
  taskTokens: TokenTotal | null;
  grouped: Record<SessionRole, Session[]>;
  creatingMain: boolean;
  onNewMain: () => void;
  sessionMeta: Map<string, SessionMetaEntry>;
  activeId: string;
  hasPendingForSession: (s: Session) => boolean;
  store: SessionStore;
  onSelect: (s: Session) => void;
  controllingIntent: string | null;
  onCancel: (s: Session) => void;
  intentsHasMore: boolean;
  loadingOlderIntents: boolean;
  loadOlderIntents: () => void;
  showNoWorkers: boolean;
};

// SessionList is the left rail: connection state, task token total, and sessions
// grouped by role (collapsible on mobile).
export function SessionList({
  listOpen,
  onToggleList,
  activeDisplayTitle,
  hiddenUnread,
  sseLive,
  taskTokens,
  grouped,
  creatingMain,
  onNewMain,
  sessionMeta,
  activeId,
  hasPendingForSession,
  store,
  onSelect,
  controllingIntent,
  onCancel,
  intentsHasMore,
  loadingOlderIntents,
  loadOlderIntents,
  showNoWorkers,
}: SessionListProps) {
  const canOperate = useCan("operate"); // new sessions and worker deletion are operate-level
  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
      <div className="border-b px-3 py-2">
        <div className="flex items-center justify-between gap-2">
          <button
            type="button"
            onClick={onToggleList}
            aria-expanded={listOpen}
            aria-controls="session-list-panel"
            className="flex min-w-0 flex-1 items-center gap-1 text-left text-xs font-medium text-muted-foreground lg:pointer-events-none"
          >
            <ChevronDownIcon
              className={cn("size-3.5 shrink-0 transition-transform lg:hidden", !listOpen && "-rotate-90")}
            />
            <span className="shrink-0">{t("Sessions")}</span>
            {!listOpen && (
              <>
                <span className="min-w-0 truncate text-foreground lg:hidden" title={activeDisplayTitle}>
                  · {activeDisplayTitle}
                </span>
                {hiddenUnread > 0 && (
                  <span className="inline-flex min-w-4 shrink-0 items-center justify-center rounded-full bg-blue-500/15 px-1 text-[10px] font-medium tabular-nums text-blue-600 lg:hidden dark:text-blue-400">
                    {hiddenUnread > 99 ? "99+" : hiddenUnread}
                  </span>
                )}
              </>
            )}
          </button>
          {!MOCK && (
            <span
              className={cn(
                "inline-flex items-center gap-1 text-[10px]",
                sseLive ? "text-emerald-500" : "text-amber-500",
              )}
              title={
                sseLive
                  ? t("Live connection healthy")
                  : t("Live connection lost, reconnecting automatically (history still visible)")
              }
            >
              {sseLive ? (
                <>
                  <span className="size-1 animate-pulse rounded-full bg-emerald-500" />
                  {t("Live")}
                </>
              ) : (
                <>
                  <WifiOffIcon className="size-3" />
                  {t("Reconnecting")}
                </>
              )}
            </span>
          )}
        </div>
        {taskTokens && (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground">
                <span>{t("Task total")}</span>
                <span>·</span>
                <TokenMetrics
                  input={taskTokens.input_tokens}
                  cache={taskTokens.cache_read_tokens}
                  output={taskTokens.output_tokens}
                />
              </div>
            </TooltipTrigger>
            <TooltipContent>
              {t("Input {input} · Output {output} · Cache read {cacheRead} · Cache write {cacheWrite}", {
                input: taskTokens.input_tokens.toLocaleString(intlLocale),
                output: taskTokens.output_tokens.toLocaleString(intlLocale),
                cacheRead: taskTokens.cache_read_tokens.toLocaleString(intlLocale),
                cacheWrite: taskTokens.cache_write_tokens.toLocaleString(intlLocale),
              })}
            </TooltipContent>
          </Tooltip>
        )}
      </div>
      <ScrollArea
        id="session-list-panel"
        type="auto"
        className={cn("min-h-0 flex-1 [&_[data-slot=scroll-area-viewport]>div]:block!", !listOpen && "max-lg:hidden")}
      >
        <div className="flex w-full flex-col gap-3 p-2">
          {(["mainagent", "planner", "system", "worker"] as const).map((role) => {
            const items = grouped[role];
            if (!items.length) return null;
            const Meta = roleMeta[role];
            return (
              <div key={role} className="flex flex-col gap-0.5">
                <div className="flex items-center gap-1.5 px-2 py-1 text-xs font-medium text-muted-foreground">
                  <Meta.icon className="size-3.5" />
                  {Meta.label}
                  {role === "mainagent" && canOperate && (
                    <button
                      type="button"
                      onClick={onNewMain}
                      disabled={creatingMain}
                      title={t("New main agent session (clears context, task state preserved)")}
                      aria-label={t("New main agent session")}
                      className="ml-auto flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-accent/60 hover:text-foreground disabled:opacity-50"
                    >
                      {creatingMain ? (
                        <Loader2Icon className="size-3.5 animate-spin" />
                      ) : (
                        <PlusIcon className="size-3.5" />
                      )}
                      {t("New")}
                    </button>
                  )}
                </div>
                {items.map((s) => {
                  const meta = s.role === "worker" ? sessionMeta.get(s.id) : undefined;
                  return (
                    <SessionItem
                      key={s.id}
                      s={s}
                      active={s.id === activeId}
                      displayTitle={meta?.title ?? s.title}
                      hasPending={hasPendingForSession(s)}
                      unread={store[keyForSession(s)]?.unread}
                      onClick={() => onSelect(s)}
                      controlling={controllingIntent === s.intent_id}
                      deleted={meta?.deleted}
                      onCancel={canOperate ? () => onCancel(s) : undefined}
                    />
                  );
                })}
                {role === "worker" && intentsHasMore && (
                  <button
                    type="button"
                    onClick={loadOlderIntents}
                    disabled={loadingOlderIntents}
                    className="mt-0.5 flex items-center justify-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent/50"
                  >
                    {loadingOlderIntents ? (
                      <Loader2Icon className="size-3.5 animate-spin" />
                    ) : (
                      <RotateCwIcon className="size-3.5" />
                    )}
                    {t("Load earlier workers")}
                  </button>
                )}
              </div>
            );
          })}
          {showNoWorkers && (
            <div className="px-2 py-1 text-xs text-muted-foreground">{t("No running worker sessions.")}</div>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
