"use client";

import * as React from "react";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Activity } from "@/lib/types";

import { AnswerBlock, MessageBlock, ToolBlock, UserRow } from "./transcript/blocks";
import { groupSteps } from "./transcript/helpers";
import { InterceptCard } from "./transcript/intercept-card";

function ExecView({
  activity,
  taskId,
  chat,
  fetchDetail,
  focusedSeq,
}: {
  activity: Activity[];
  taskId?: string;
  chat?: boolean;
  fetchDetail?: (seq: number) => Promise<string>;
  focusedSeq?: number;
}) {
  const showWorker = new Set(activity.map((a) => a.worker)).size > 1;
  // default detail fetcher: the task-scoped activity endpoint. The chat page passes
  // its own (conversation-scoped) fetcher instead.
  const getDetail = React.useCallback(
    (seq: number) => (fetchDetail ? fetchDetail(seq) : api.activityDetail(seq, taskId).then((r) => r.detail ?? "")),
    [fetchDetail, taskId],
  );
  return (
    <div className="flex flex-col">
      {groupSteps(activity, !!chat).map((g) =>
        g.type === "round" ? (
          <div key={"r" + g.key} className="my-2 flex items-center gap-2 text-[10px] font-medium text-muted-foreground">
            <span className="h-px flex-1 bg-border" />
            {g.label}
            <span className="h-px flex-1 bg-border" />
          </div>
        ) : g.type === "user" ? (
          <UserRow key={"u" + g.key} step={g.step} intent={g.intent} getDetail={getDetail} />
        ) : g.type === "answer" ? (
          <AnswerBlock key={"a" + g.key} step={g.step} getDetail={getDetail} />
        ) : g.type === "tool" ? (
          <ToolBlock
            key={"t" + g.key}
            group={g}
            getDetail={getDetail}
            showWorker={showWorker}
            focused={focusedSeq != null && g.use?.seq === focusedSeq}
          />
        ) : g.type === "intercept" ? (
          <InterceptCard key={"ic" + g.key} step={g.step} getDetail={getDetail} />
        ) : (
          <MessageBlock key={"m" + g.key} group={g} getDetail={getDetail} showWorker={showWorker} />
        ),
      )}
    </div>
  );
}

// Transcript renders a session's activity as a compact grouped execution replay:
// human turns, thinking, tool calls (command+result paired), and messages — for
// the main agent (interactive) and worker/planner (read-only) alike.
export function Transcript({
  activity,
  live,
  taskId,
  chat,
  fetchDetail,
  focusedSeq,
}: {
  activity: Activity[];
  live?: boolean;
  taskId?: string;
  chat?: boolean;
  fetchDetail?: (seq: number) => Promise<string>;
  focusedSeq?: number;
}) {
  const transcriptRef = React.useRef<HTMLDivElement>(null);
  const [focusPadding, setFocusPadding] = React.useState(0);
  React.useLayoutEffect(() => {
    if (focusedSeq == null) {
      setFocusPadding(0);
      return;
    }
    const viewport = transcriptRef.current?.closest('[data-slot="scroll-area-viewport"]');
    if (!viewport) return;
    const measure = () => setFocusPadding(viewport.clientHeight / 2);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, [focusedSeq]);
  return (
    <div
      ref={transcriptRef}
      className="flex flex-col gap-1"
      style={focusPadding ? { paddingBlock: focusPadding } : undefined}
    >
      <ExecView activity={activity} taskId={taskId} chat={chat} fetchDetail={fetchDetail} focusedSeq={focusedSeq} />
      {live && (
        <div className="flex items-center gap-2 pl-2 pt-1 text-xs text-muted-foreground">
          <span className="flex gap-1">
            <span className="size-1.5 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.3s]" />
            <span className="size-1.5 animate-bounce rounded-full bg-blue-500 [animation-delay:-0.15s]" />
            <span className="size-1.5 animate-bounce rounded-full bg-blue-500" />
          </span>
          {t("Live streaming…")}
        </div>
      )}
    </div>
  );
}
