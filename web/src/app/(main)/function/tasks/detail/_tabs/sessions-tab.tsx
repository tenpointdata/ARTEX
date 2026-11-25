"use client";

import * as React from "react";

import { toast } from "sonner";

import { ApprovalExecutionFocus, useApprovalFocus } from "@/components/approval-execution-focus";
import { SideQuestionWorkspace } from "@/components/side-question-workspace";
import { TodoPopover } from "@/components/todo-popover";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useSideQuestions } from "@/hooks/use-side-questions";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { useChatSendMode } from "@/lib/chat-send-mode";
import type { Activity, Session, TaskLLMResolution, TokenTotal } from "@/lib/types";
import { cn } from "@/lib/utils";

import { MainComposer, WorkerComposer } from "./sessions/composer";
import { DeleteWorkerDialog, NewMainSessionDialog } from "./sessions/dialogs";
import {
  keyForSession,
  MAIN_ID,
  MAIN_SESSION,
  mainSessionId,
  mainSessionKey,
  mainSessionTitle,
  PLANNER_SESSION,
  STREAM_WINDOW_MS,
  SYSTEM_SESSION,
} from "./sessions/helpers";
import { SessionList } from "./sessions/session-list";
import { emptyState } from "./sessions/store";
import { TranscriptBody } from "./sessions/transcript-body";
import { TranscriptHeader } from "./sessions/transcript-header";
import { useFocusSession } from "./sessions/use-focus-session";
import { useIntents } from "./sessions/use-intents";
import { useMainComposer } from "./sessions/use-main-composer";
import { useSessionStream } from "./sessions/use-session-stream";
import { useTaskPolling } from "./sessions/use-task-polling";
import { useTranscriptScroll } from "./sessions/use-transcript-scroll";
import { useWorkerChat } from "./sessions/use-worker-chat";
import { useWorkerControl } from "./sessions/use-worker-control";

export function SessionsTab({ taskId, taskPaused = false }: { taskId: string; taskPaused?: boolean }) {
  const approvalFocus = useApprovalFocus({ taskId });
  const [selectedSessionId, setActiveId] = React.useState(MAIN_ID);
  const [creatingMain, setCreatingMain] = React.useState(false);
  const [confirmNewMain, setConfirmNewMain] = React.useState(false);
  // On mobile (<lg) the session list is collapsed by default: screen height is already tight, and a fixed 10~15rem list
  // would squeeze the transcript below down to just the header and input. Collapsed, the transcript gets nearly all the height;
  // tapping the title bar expands it to pick a session, collapsing again after choosing. Desktop is unaffected (always expanded from lg up).
  const [listOpen, setListOpen] = React.useState(false);
  const [sending, setSending] = React.useState(false);

  const polling = useTaskPolling(taskId);
  const { taskTokens, sessionTokens, llmResolutions, pendingIntercepts, mainChatRunning } = polling;
  const intentsState = useIntents(taskId);
  const { patchIntentState, workerSessions, sessionMeta } = intentsState;
  const control = useWorkerControl(taskId, patchIntentState);
  const stream = useSessionStream({
    taskId,
    setActiveId,
    setLLMResolutions: polling.setLLMResolutions,
  });
  const { store, setStore, patchStore, loadSession, loadEarlier, mainSegs, setMainSegs, currentSeg, setCurrentSeg } =
    stream;
  const { activeKeyRef, atBottomRef } = stream;
  const { focusSession, retainedSession, focusHistory } = useFocusSession({
    taskId,
    approvalFocus,
    store,
    patchStore,
    atBottomRef,
  });
  const activeId = focusSession?.id ?? selectedSessionId;

  // Re-render on a timer so "streaming" liveness recomputes as activity goes stale.
  const [, setTick] = React.useState(0);
  React.useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1500);
    return () => clearInterval(id);
  }, []);

  // Liveness from each session's last-seen activity time (kept fresh by the 1.5s tick).
  const recentLive = React.useCallback(
    (key: string) => {
      const ts = store[key]?.lastTs;
      if (!ts) return false;
      const t = Date.parse(ts);
      return t > 0 && Date.now() - t < STREAM_WINDOW_MS;
    },
    [store],
  );
  const currentMainKey = mainSessionKey(currentSeg);
  const plannerLive = recentLive("plan");

  // Which main segment is streaming RIGHT NOW. A main turn is serialized per task, so at
  // most one segment is live. Gate on the real running flag (sending / mainChatRunning)
  // rather than "recent activity", and drop it the moment the turn's terminal record
  // (kind='result', or an error) lands — otherwise the badge lingers for STREAM_WINDOW_MS
  // after the agent already finished. null = nothing running.
  const liveMainSeg = React.useMemo<number | null>(() => {
    if (!(sending || mainChatRunning)) return null;
    // the streaming segment is the one with the freshest activity (incl. the just-sent turn)
    let seg = currentSeg;
    let bestTs = -1;
    for (const m of mainSegs) {
      const raw = store[mainSessionKey(m.seq)]?.lastTs;
      const ts = raw ? Date.parse(raw) : -1;
      if (ts > bestTs) {
        bestTs = ts;
        seg = m.seq;
      }
    }
    const items = store[mainSessionKey(seg)]?.items ?? [];
    const last = items[items.length - 1];
    if (last && (last.kind === "result" || (last.kind === "text" && last.is_error))) return null;
    return seg;
  }, [sending, mainChatRunning, mainSegs, store, currentSeg]);

  // Every main-agent segment is an independent, interactive session (like the top-level
  // chat conversations) — you can talk in any of them, newest-first.
  const mainSessions = React.useMemo<Session[]>(
    () =>
      mainSegs.map((m) => ({
        id: mainSessionId(m.seq),
        role: "mainagent",
        title: mainSessionTitle(m.seq),
        status: "running",
        live: m.seq === liveMainSeg,
        last_activity: m.created_at,
        seg: m.seq,
      })),
    [mainSegs, liveMainSeg],
  );

  const sessions = React.useMemo(() => {
    const items = [...mainSessions, { ...PLANNER_SESSION, live: plannerLive }, ...workerSessions, SYSTEM_SESSION];
    const located = focusSession ?? retainedSession;
    if (located && !items.some((s) => s.id === located.id)) items.push(located);
    return items;
  }, [mainSessions, workerSessions, plannerLive, focusSession, retainedSession]);

  const grouped = {
    mainagent: sessions.filter((s) => s.role === "mainagent"),
    planner: sessions.filter((s) => s.role === "planner"),
    worker: sessions.filter((s) => s.role === "worker"),
    system: sessions.filter((s) => s.role === "system"),
  };

  const active = sessions.find((s) => s.id === activeId) ?? MAIN_SESSION;
  const side = useSideQuestions(
    active.role === "mainagent"
      ? `/api/tasks/${taskId}/chat`
      : active.role === "worker" && !active.inherited && active.intent_id
        ? `/api/tasks/${taskId}/intents/${active.intent_id}`
        : null,
  );
  const isMain = active.role === "mainagent";
  const isPlanner = active.role === "planner";
  const isSystem = active.role === "system";
  const activeKey = keyForSession(active);
  const activeState = store[activeKey];
  // A main turn is serialized per task (chat lock), so "busy" is task-wide: while any
  // main segment is mid-turn the active composer is disabled. Clear it the moment the
  // active session's terminal record (kind='result', or an error) lands, so the input
  // re-enables immediately instead of waiting for the next chat-status poll.
  const activeItems = activeState?.items ?? [];
  const activeLast = activeItems[activeItems.length - 1];
  const activeSettled =
    !!activeLast && (activeLast.kind === "result" || (activeLast.kind === "text" && activeLast.is_error));
  const composer = useMainComposer({
    taskId,
    seg: active.seg ?? 0,
    isMain,
    externalBusy: !activeSettled && (mainChatRunning ?? recentLive(activeKey)),
    side,
    sending,
    setSending,
    setMainChatRunning: polling.setMainChatRunning,
    chatStatusRequestRef: polling.chatStatusRequestRef,
  });
  const workerChat = useWorkerChat({ taskId, active, side, patchIntentState });
  // In the collapsed state (mobile) the title bar replaces the whole list: it shows the current session name + the total unread of other sessions,
  // otherwise once collapsed you neither know which session you are viewing nor see new messages elsewhere.
  const activeDisplayTitle = (active.role === "worker" ? sessionMeta.get(active.id)?.title : "") || active.title;
  const hiddenUnread = React.useMemo(
    () => Object.entries(store).reduce((sum, [key, s]) => (key === activeKey ? sum : sum + s.unread), 0),
    [store, activeKey],
  );

  // Keep the SSE dispatcher's notion of the active session current, and lazily load
  // + clear unread whenever the active session changes.
  // biome-ignore lint/correctness/useExhaustiveDependencies: cache updates must not reactivate the current session.
  React.useEffect(() => {
    activeKeyRef.current = activeKey;
    const st = store[activeKey];
    if (!st || (!st.loaded && !st.loading)) {
      loadSession(activeKey);
    } else if (st.unread) {
      patchStore(activeKey, (s) => ({ ...s, unread: 0 }));
    }
  }, [activeKey]);

  // Main agent is the human↔orchestrator CONSOLE: only the conversation (user msgs +
  // the main agent's own replies/steps). Planner session shows planner steps; a
  // worker session shows only its intent's activity, led by the intent objective.
  const activity = React.useMemo(() => {
    const items = activeState?.items ?? [];
    // The main-agent console renders purely from server data: the human turn is
    // persisted+broadcast by the backend BEFORE it returns, so it arrives over the
    // same SSE stream (worker="mainagent") as every agent step — no client-side
    // optimistic echo, hence no fabricated seq that could collide with real DB ids.
    if (isMain) return items;
    if (isPlanner || isSystem) return items;
    // Worker session: the intent leads the transcript as a right-aligned "user"-style
    // message (the task handed to this worker), followed by its execution steps.
    const intentTitle = sessionMeta.get(active.id)?.title ?? active.title;
    const intentMsg: Activity = {
      seq: -1, // sorts/leads before any real step (real seq ≥ 0)
      worker: items[0]?.worker ?? active.id, // reuse the lane so no worker chips appear
      ts: active.last_activity || "",
      kind: "intent", // LLM-generated objective — rendered as a distinct (non-human) bubble
      summary: intentTitle,
      source_task_id: active.source_task_id,
      inherited: active.inherited,
    };
    return [intentMsg, ...items];
  }, [
    isMain,
    isPlanner,
    isSystem,
    activeState,
    active.id,
    active.title,
    active.last_activity,
    active.source_task_id,
    active.inherited,
    sessionMeta,
  ]);

  // seq of this session's most-recent TodoWrite call — for the Todo popover.
  const latestTodoSeq = React.useMemo(() => {
    for (let i = activity.length - 1; i >= 0; i--) {
      const a = activity[i];
      if (a.kind === "tool_use" && a.tool === "TodoWrite") return a.seq;
    }
    return null;
  }, [activity]);

  // Persisted result totals come from the backend's full activity history. Only
  // the latest unfinished usage frame is added client-side, so partial history
  // pages cannot undercount completed runs or double-count the active one.
  const activeTokens = React.useMemo((): TokenTotal => {
    const saved = sessionTokens[activeKey];
    const total: TokenTotal = {
      input_tokens: saved?.input_tokens ?? 0,
      output_tokens: saved?.output_tokens ?? 0,
      cache_read_tokens: saved?.cache_read_tokens ?? 0,
      cache_write_tokens: saved?.cache_write_tokens ?? 0,
    };
    const items = store[activeKey]?.items ?? [];
    for (let index = items.length - 1; index >= 0; index--) {
      const item = items[index];
      if (item.kind === "result") break;
      if (item.kind !== "usage") continue;
      total.input_tokens += item.input_tokens ?? 0;
      total.output_tokens += item.output_tokens ?? 0;
      total.cache_read_tokens += item.cache_read_tokens ?? 0;
      total.cache_write_tokens += item.cache_write_tokens ?? 0;
      break;
    }
    return total;
  }, [sessionTokens, store, activeKey]);

  // Run duration = span from this session's first step to its last (for a live
  // session, "now" so it ticks up — the 1.5s setTick above re-renders it).
  // While the TASK is paused nothing time-derived may advance (#84): sessions
  // keep state='running' in the store during a pause, so live alone would keep
  // ticking and misrepresent the frozen budget. Freeze at the last step instead.
  const runDuration = React.useMemo(() => {
    let min = Infinity,
      max = 0;
    for (const a of activity) {
      const t = Date.parse(a.ts);
      if (!Number.isFinite(t)) continue;
      if (t < min) min = t;
      if (t > max) max = t;
    }
    if (!Number.isFinite(min) || max === 0) return null;
    const end = active.live && !taskPaused ? Date.now() : max;
    return Math.max(0, end - min);
  }, [activity, active.live, taskPaused]);

  const onNearTop = React.useCallback(
    (viewport: () => HTMLElement | null) => loadEarlier(activeKeyRef.current, viewport),
    [loadEarlier, activeKeyRef],
  );
  const { contentRef } = useTranscriptScroll({
    activeId,
    activity,
    focusing: !!approvalFocus.state,
    focusReady: focusHistory.ready,
    atBottomRef,
    onNearTop,
  });

  // Start a fresh main-agent session: only the segment counter advances — the task's
  // graph/assets/goal are untouched, so the agent continues over the same task with a
  // clean context. The old segment stays as read-only history you can switch back to.
  async function createMainSession() {
    if (creatingMain) return;
    setCreatingMain(true);
    try {
      const r = await api.newMainSession(taskId);
      setMainSegs((prev) => [{ seq: r.seq, created_at: r.created_at }, ...prev.filter((m) => m.seq !== r.seq)]);
      setCurrentSeg(r.current ?? r.seq);
      // Seed an empty, loaded state so the new (empty) session renders immediately.
      setStore((prev) => ({ ...prev, [mainSessionKey(r.seq)]: { ...emptyState(), loaded: true } }));
      setActiveId(mainSessionId(r.seq));
      setListOpen(false);
      composer.setInput("");
    } catch (e) {
      toast.error(t("Failed to create session: {error}", { error: (e as Error).message }));
    } finally {
      setCreatingMain(false);
      setConfirmNewMain(false);
    }
  }

  function selectSession(s: Session) {
    approvalFocus.close();
    setActiveId(s.id);
    setListOpen(false); // on mobile, collapse after picking to give the height back to the transcript
    workerChat.resetWorkerMessage();
  }

  function closeDeleteDialog() {
    control.setCancelIntent(null);
    control.setCancelReason("");
    control.setDeleteMode("soft");
  }

  const mainLoaded = !!store[currentMainKey]?.loaded;
  // The send key is decided by system settings (localStorage); Enter sends by default.
  const sendMode = useChatSendMode();
  // What the transcript pane should show for the active session.
  const showLoader = !activeState || (activeState.loading && !activeState.loaded);
  const resolutionForSession = (session: Session): TaskLLMResolution | undefined => {
    if (!llmResolutions || session.inherited || session.role === "system") return undefined;
    if (session.role === "mainagent") return llmResolutions.mainagent;
    if (session.role === "planner") return llmResolutions.planner;
    return llmResolutions.worker;
  };
  const activeResolution = resolutionForSession(active);
  const activeAssets =
    active.role === "worker" && active.intent_id ? intentsState.intentAssetsByID.get(active.intent_id) : undefined;
  // Returns true if any pending intercept belongs to this session.
  // Worker agent_name format: "work#N · #intentID". Main/planner match by role key.
  const hasPendingForSession = (s: Session): boolean => {
    if (s.inherited) return false;
    if (!pendingIntercepts.length) return false;
    if (s.role === "mainagent") return pendingIntercepts.some((r) => r.agent_name === "mainagent");
    if (s.role === "planner") return pendingIntercepts.some((r) => r.agent_name === "planner");
    // Worker: extract intent ID from "work#N · #<intentID>"
    return pendingIntercepts.some((r) => {
      const m = r.agent_name.match(/·\s*#(\d+)$/);
      return m ? m[1] === s.intent_id : false;
    });
  };

  return (
    <TooltipProvider delayDuration={300}>
      {/* Height reserve: page header (title row + goal + Tabs ≈ 7.5rem) + content padding. Mobile p-4,
        desktop lg:p-6, and desktop also needs scroll headroom, so the two tiers reserve 10rem / 13rem respectively — mobile
        reusing 13rem would waste 3rem of transcript height. */}
      <div
        className={cn(
          "grid h-[calc(100svh-10rem)] min-h-0 grid-cols-1 gap-4",
          listOpen ? "grid-rows-[minmax(0,40svh)_minmax(0,1fr)]" : "grid-rows-[auto_minmax(0,1fr)]",
          "lg:h-[calc(100svh-13rem)] lg:grid-cols-[18rem_1fr] lg:grid-rows-[minmax(0,1fr)]",
        )}
      >
        {/* Left: session list */}
        <SessionList
          listOpen={listOpen}
          onToggleList={() => setListOpen((open) => !open)}
          activeDisplayTitle={activeDisplayTitle}
          hiddenUnread={hiddenUnread}
          sseLive={stream.sseLive}
          taskTokens={taskTokens}
          grouped={grouped}
          creatingMain={creatingMain}
          onNewMain={() => setConfirmNewMain(true)}
          sessionMeta={sessionMeta}
          activeId={activeId}
          hasPendingForSession={hasPendingForSession}
          store={store}
          onSelect={selectSession}
          controllingIntent={control.controllingIntent}
          onCancel={control.setCancelIntent}
          intentsHasMore={intentsState.intentsHasMore}
          loadingOlderIntents={intentsState.loadingOlderIntents}
          loadOlderIntents={intentsState.loadOlderIntents}
          showNoWorkers={mainLoaded && !workerSessions.length}
        />

        {/* Right: transcript */}
        <SideQuestionWorkspace
          side={side}
          label={
            active.role === "worker"
              ? `${t("Worker #{id}", { id: active.intent_id ?? "" })} · ${activeDisplayTitle}`
              : activeDisplayTitle
          }
        >
          <div className="flex min-h-0 flex-1 min-w-0 flex-col overflow-hidden rounded-lg border bg-card">
            <TranscriptHeader
              active={active}
              sessionMeta={sessionMeta}
              side={side}
              activeResolution={activeResolution}
              activeAssets={activeAssets}
              activeState={activeState}
              activeTokens={activeTokens}
              runDuration={runDuration}
              isMain={isMain}
            />
            <ApprovalExecutionFocus
              focus={{
                ...approvalFocus,
                close: () => {
                  setActiveId(activeId);
                  approvalFocus.close();
                },
              }}
              history={focusHistory}
            />
            <TranscriptBody
              contentRef={contentRef}
              activeState={activeState}
              showLoader={showLoader}
              activity={activity}
              active={active}
              taskId={taskId}
              isMain={isMain}
              focusedSeq={focusHistory.ready ? approvalFocus.state?.source?.seq : undefined}
              onRetry={() => loadSession(activeKey)}
            />
            {isMain ? (
              <MainComposer composer={composer} sending={sending} sendMode={sendMode} />
            ) : active.role === "worker" &&
              !active.inherited &&
              (active.status === "running" || active.status === "paused") ? (
              <WorkerComposer
                active={active}
                chat={workerChat}
                sendMode={sendMode}
                controllingIntent={control.controllingIntent}
                onPause={() => void control.controlWorker(active, "pause")}
                onResume={() => void control.controlWorker(active, "resume")}
              />
            ) : (
              <div className="flex items-center border-t px-4 py-2">
                <TodoPopover
                  seq={latestTodoSeq}
                  fetchDetail={(seq) => api.activityDetail(seq, taskId).then((r) => r.detail ?? "")}
                />
              </div>
            )}
          </div>
        </SideQuestionWorkspace>
        <DeleteWorkerDialog
          cancelIntent={control.cancelIntent}
          cancelReason={control.cancelReason}
          deleteMode={control.deleteMode}
          controllingIntent={control.controllingIntent}
          onClose={closeDeleteDialog}
          setCancelReason={control.setCancelReason}
          setDeleteMode={control.setDeleteMode}
          onConfirm={(session) =>
            void control.controlWorker(session, "cancel", control.cancelReason, control.deleteMode)
          }
        />
        <NewMainSessionDialog
          open={confirmNewMain}
          creating={creatingMain}
          onClose={() => setConfirmNewMain(false)}
          onConfirm={() => void createMainSession()}
        />
      </div>
    </TooltipProvider>
  );
}
