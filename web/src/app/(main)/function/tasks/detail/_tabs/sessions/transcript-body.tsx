import type * as React from "react";

import { CircleXIcon, Loader2Icon } from "lucide-react";

import { Transcript } from "@/components/transcript";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { t } from "@/i18n";
import type { Activity, Session } from "@/lib/types";

import type { SessionState } from "./store";

// TranscriptBody renders the scrollable transcript with its loading / error / empty states.
export function TranscriptBody({
  contentRef,
  activeState,
  showLoader,
  activity,
  active,
  taskId,
  isMain,
  focusedSeq,
  onRetry,
}: {
  contentRef: React.RefObject<HTMLDivElement | null>;
  activeState: SessionState | undefined;
  showLoader: boolean;
  activity: Activity[];
  active: Session;
  taskId: string;
  isMain: boolean;
  focusedSeq: number | undefined;
  onRetry: () => void;
}) {
  // Force Radix's internal viewport wrapper (display:table, sizes to content)
  // to block so wide/unbreakable steps (long commands, code, URLs) can't blow
  // out the width and defeat the truncation below — the transcript wraps to
  // the panel instead of overflowing horizontally.
  return (
    <ScrollArea type="auto" className="min-h-0 min-w-0 flex-1 [&_[data-slot=scroll-area-viewport]>div]:block!">
      <div className="min-w-0 max-w-full p-4" ref={contentRef}>
        {activeState?.loadingMore && (
          <div className="flex items-center justify-center gap-2 pb-2 text-xs text-muted-foreground">
            <Loader2Icon className="size-3.5 animate-spin" />
            {t("Loading earlier history…")}
          </div>
        )}
        {showLoader ? (
          <div className="flex items-center gap-2 pl-9 text-xs text-muted-foreground">
            <Loader2Icon className="size-3.5 animate-spin" />
            {t("Loading activity stream…")}
          </div>
        ) : activeState?.error ? (
          <div className="flex items-center gap-2 pl-9 text-xs text-red-500">
            <CircleXIcon className="size-3.5" />
            {t("Failed to load: {error}", { error: activeState.error })}
            <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" onClick={onRetry}>
              {t("Retry")}
            </Button>
          </div>
        ) : activity.length ? (
          <Transcript activity={activity} live={active.live} taskId={taskId} chat={isMain} focusedSeq={focusedSeq} />
        ) : (
          <div className="pl-9 text-xs text-muted-foreground">
            {isMain
              ? t("No conversation yet. Message the main agent below to steer exploration or step into the flow.")
              : t("No activity records.")}
          </div>
        )}
      </div>
    </ScrollArea>
  );
}
