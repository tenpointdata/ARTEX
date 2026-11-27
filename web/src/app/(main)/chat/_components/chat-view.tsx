import * as React from "react";

import { Bot } from "lucide-react";
import { toast } from "sonner";

import { ApprovalExecutionFocus, useApprovalFocus, useApprovalHistory } from "@/components/approval-execution-focus";
import { SideQuestionButton, SideQuestionWorkspace } from "@/components/side-question-workspace";
import { TodoPopover } from "@/components/todo-popover";
import { Transcript } from "@/components/transcript";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useCan } from "@/hooks/use-session";
import { useSideQuestions } from "@/hooks/use-side-questions";
import { t } from "@/i18n";
import { mergeActivities } from "@/lib/activity-merge";
import { api } from "@/lib/api";
import type { Activity, Agent, ChatAttachment, Conversation, LLMProfile } from "@/lib/types";

import { fmtTokens, HISTORY_PAGE } from "./chat-helpers";
import { Composer, LLMProfileRow } from "./composer";

// LiveBadge is the small pulsing "live" chip reused from the task's main-agent
// console — shown while a turn is streaming.
export function LiveBadge() {
  return (
    <span className="inline-flex items-center gap-1 rounded bg-blue-500/15 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">
      <span className="size-1 animate-pulse rounded-full bg-blue-500" />
      {t("Live")}
    </span>
  );
}

// ChatView is the right pane for one conversation — mirrors the task detail's
// main-agent console: header with agent + live badge + token/duration meta, a
// scroll-stick transcript (chat mode), and the composer.
export function ChatView({
  conv,
  agents,
  profiles,
  initial,
  onTitleMaybeChanged,
  onConvUpdated,
}: {
  conv: Conversation;
  agents: Agent[];
  profiles: LLMProfile[];
  // pending text + already-uploaded attachments handed off from a draft that
  // created this conversation via the paperclip (consumed once, on mount).
  initial?: { input?: string; attachments?: ChatAttachment[] };
  onTitleMaybeChanged: () => void;
  onConvUpdated: () => void;
}) {
  const canOperate = useCan("operate"); // messages, send, stop and profile switching all need operate
  const approvalFocus = useApprovalFocus({ conversationId: conv.id });
  const [messages, setMessages] = React.useState<Activity[]>([]);
  const [running, setRunning] = React.useState(false);
  const [input, setInput] = React.useState(initial?.input ?? "");
  const [sending, setSending] = React.useState(false);
  const [stopping, setStopping] = React.useState(false);
  // Upload method 1: already-uploaded attachments (stored under sessions/conv-<id>/uploads/) are sent with the next message.
  const [attachments, setAttachments] = React.useState<ChatAttachment[]>(initial?.attachments ?? []);
  const [uploading, setUploading] = React.useState(false);
  const cursorRef = React.useRef(0); // newest loaded id — incremental-tail anchor
  const earliestRef = React.useRef(0); // earliest loaded id — reverse-pagination anchor
  const hasMoreRef = React.useRef(false); // older history remains above the loaded window
  const loadingMoreRef = React.useRef(false); // guard: one scroll-up load at a time
  const [historyLoaded, setHistoryLoaded] = React.useState(false);
  const [hasMore, setHasMore] = React.useState(false); // drives the "load earlier" hint
  const agent = agents.find((a) => a.key === conv.agent_key);
  const currentProfileId = conv.llm_profile_id ?? null;
  const side = useSideQuestions(`/api/conversations/${conv.id}`);

  async function changeProfile(id: number | null) {
    try {
      await api.updateConversationProfile(conv.id, id);
      onConvUpdated();
    } catch (e) {
      toast.error(t("Failed to switch LLM: {error}", { error: (e as Error).message }));
    }
  }

  // conversation-scoped detail fetcher for the reused Transcript renderer.
  const fetchDetail = React.useCallback((seq: number) => api.conversationMsgDetail(conv.id, seq), [conv.id]);

  // seq of the most-recent TodoWrite tool call (for the Todo popover); null if none.
  const latestTodoSeq = React.useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      const a = messages[i];
      if (a.kind === "tool_use" && a.tool === "TodoWrite") return a.seq;
    }
    return null;
  }, [messages]);

  // reset + load whenever the selected conversation changes. Load only the LATEST
  // page on open — a long thread's final answer sits at the very end, so the newest
  // page shows it immediately (issue #3: opening/refresh used to load the oldest
  // page, so the completed result was missing until another message advanced the
  // cursor). Older history streams in on scroll-up (loadEarlier below).
  React.useEffect(() => {
    cursorRef.current = 0;
    earliestRef.current = 0;
    hasMoreRef.current = false;
    setHasMore(false);
    setMessages([]);
    setHistoryLoaded(false);
    setRunning(false);
    if (!canOperate) {
      setHistoryLoaded(true); // GET messages needs operate: skip rather than show a 403
      return;
    }
    let live = true;
    api
      .conversationHistory(conv.id, 0, HISTORY_PAGE)
      .then((r) => {
        if (!live) return;
        setMessages((current) => mergeActivities(r.items, current));
        cursorRef.current = Math.max(cursorRef.current, r.cursor);
        earliestRef.current = r.items.length ? r.items[0].seq : 0;
        hasMoreRef.current = r.hasMore;
        setHasMore(r.hasMore);
        setRunning((current) => current || r.running);
        setHistoryLoaded(true);
      })
      .catch(() => {
        // A source link can retry history through its locator; ordinary chats
        // retain the existing usable empty state on a transient failure.
        if (live) setHistoryLoaded(true);
      });
    return () => {
      live = false;
    };
  }, [conv.id, canOperate]);

  // A trigger or another tab may start a turn while this conversation is open.
  // A false list snapshot must not stop the tail before its final messages load.
  React.useEffect(() => {
    if (conv.running) setRunning(true);
  }, [conv.running]);

  // poll while a turn is running: pull new steps after the cursor.
  React.useEffect(() => {
    if (!running || !canOperate) return;
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      let keepPolling = true;
      try {
        const r = await api.conversationMessages(conv.id, cursorRef.current);
        if (!live) return;
        if (r.items.length) {
          setMessages((prev) => mergeActivities(prev, r.items));
        }
        cursorRef.current = Math.max(cursorRef.current, r.cursor);
        keepPolling = r.running;
        setRunning(r.running);
        if (!r.running) onTitleMaybeChanged(); // first-turn auto-title landed
      } catch {
        /* transient — keep polling */
      } finally {
        // Slow responses must not overlap another poll with the same cursor.
        if (live && keepPolling) timer = setTimeout(() => void tick(), 1000);
      }
    };
    void tick();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [running, canOperate, conv.id, onTitleMaybeChanged]);

  const loadFocusPage = React.useCallback(
    (before: number) => api.conversationHistory(conv.id, before, HISTORY_PAGE),
    [conv.id],
  );
  const mergeFocusPage = React.useCallback((page: { items: Activity[]; hasMore: boolean }) => {
    setMessages((prev) => mergeActivities(page.items, prev));
    earliestRef.current = page.items[0]?.seq ?? earliestRef.current;
    hasMoreRef.current = page.hasMore;
    setHasMore(page.hasMore);
  }, []);
  const focusHistory = useApprovalHistory(
    approvalFocus.state?.source,
    historyLoaded,
    messages,
    loadFocusPage,
    mergeFocusPage,
  );

  // ---- transcript auto-scroll (open → bottom; stick to bottom unless scrolled up) ----
  const contentRef = React.useRef<HTMLDivElement | null>(null);
  const atBottomRef = React.useRef(true);
  const viewport = React.useCallback(
    () => (contentRef.current?.closest('[data-slot="scroll-area-viewport"]') as HTMLElement | null) ?? null,
    [],
  );
  // Scroll-up loads one older page and prepends it, preserving the visual position
  // so the view doesn't jump (record height/offset before, restore the delta after).
  const loadEarlier = React.useCallback(async () => {
    if (loadingMoreRef.current || !hasMoreRef.current) return;
    const vp = viewport();
    if (!vp) return;
    loadingMoreRef.current = true;
    const prevH = vp.scrollHeight;
    const prevTop = vp.scrollTop;
    try {
      const r = await api.conversationHistory(conv.id, earliestRef.current, HISTORY_PAGE);
      if (r.items.length) {
        setMessages((prev) => mergeActivities(r.items, prev));
        earliestRef.current = r.items[0].seq;
      }
      hasMoreRef.current = r.hasMore;
      setHasMore(r.hasMore);
      requestAnimationFrame(() => {
        const v = viewport();
        if (v) v.scrollTop = prevTop + (v.scrollHeight - prevH);
      });
    } catch {
      /* transient — a later scroll retries */
    } finally {
      loadingMoreRef.current = false;
    }
  }, [conv.id, viewport]);
  React.useEffect(() => {
    const vp = viewport();
    if (!vp) return;
    const onScroll = () => {
      if (approvalFocus.state && !focusHistory.ready) return;
      atBottomRef.current = vp.scrollTop + vp.clientHeight >= vp.scrollHeight - 60;
      if (vp.scrollTop <= 80) void loadEarlier(); // near top → pull an older page
    };
    vp.addEventListener("scroll", onScroll, { passive: true });
    return () => vp.removeEventListener("scroll", onScroll);
  }, [viewport, loadEarlier, approvalFocus.state, focusHistory.ready]);
  // open/switch a conversation → jump to the latest (bottom)
  // biome-ignore lint/correctness/useExhaustiveDependencies: changing conversations intentionally retriggers the scroll reset.
  React.useLayoutEffect(() => {
    const vp = viewport();
    if (vp) {
      vp.scrollTop = vp.scrollHeight;
      atBottomRef.current = true;
    }
  }, [conv.id, viewport]);
  // new activity → stick to bottom only if the user is already pinned there
  // biome-ignore lint/correctness/useExhaustiveDependencies: message and running changes intentionally retrigger bottom anchoring.
  React.useLayoutEffect(() => {
    if (approvalFocus.state || !atBottomRef.current) return;
    const vp = viewport();
    if (vp) vp.scrollTop = vp.scrollHeight;
  }, [messages, running, viewport, approvalFocus.state]);

  // Per-conversation token total, live — same accounting as the main-agent
  // console: completed runs' `result` sum + the in-progress run's latest `usage`.
  const tokenTotal = React.useMemo(() => {
    let i = 0,
      o = 0,
      cr = 0;
    let li = 0,
      lo = 0,
      lcr = 0;
    let turns = 0; // agent loop turns = model call count (one kind='usage' entry each)
    for (const a of messages) {
      if (a.kind === "result") {
        i += a.input_tokens ?? 0;
        o += a.output_tokens ?? 0;
        cr += a.cache_read_tokens ?? 0;
        li = lo = lcr = 0;
      } else if (a.kind === "usage") {
        turns += 1;
        li = a.input_tokens ?? 0;
        lo = a.output_tokens ?? 0;
        lcr = a.cache_read_tokens ?? 0;
      }
    }
    const I = i + li,
      O = o + lo,
      CR = cr + lcr;
    return { i: I, o: O, cr: CR, turns, any: I + O + CR > 0 };
  }, [messages]);

  // pickFiles uploads into this conversation's session dir (sessions/conv-<id>/
  // uploads/) and queues the returned metadata to send with the next message.
  async function pickFiles(files: File[]) {
    if (files.length === 0) return;
    setUploading(true);
    try {
      const r = await api.chatUpload("session", `conv-${conv.id}`, files);
      setAttachments((prev) => [...prev, ...r.attachments]);
    } catch (e) {
      toast.error(t("Upload failed: {error}", { error: (e as Error).message }));
    } finally {
      setUploading(false);
    }
  }

  async function send() {
    const msg = input.trim();
    const atts = attachments;
    if (side.handleCommand(msg, () => setInput(""))) return;
    if ((!msg && atts.length === 0) || sending || running) return;
    setSending(true);
    setInput("");
    setAttachments([]);
    try {
      await api.sendConversationMessage(conv.id, msg, atts.length ? atts : undefined);
      // The live loop pulls the persisted human turn immediately. Sharing that
      // fetch avoids racing a separate post-send request against the poller.
      setRunning(true);
    } catch (e) {
      toast.error(t("Send failed: {error}", { error: (e as Error).message }));
      setInput(msg); // restore so the user doesn't lose their text
      setAttachments(atts); // and their attachments
    } finally {
      setSending(false);
    }
  }

  // stop aborts the in-flight run for this conversation. running flips to false on
  // the next 1s poll once the backend unwinds the agent; the trigger queue is not
  // affected — the agent's next queued fire still starts.
  async function stop() {
    if (stopping) return;
    setStopping(true);
    try {
      await api.stopConversation(conv.id);
    } catch (e) {
      toast.error(t("Stop failed: {error}", { error: (e as Error).message }));
    } finally {
      setStopping(false);
    }
  }

  return (
    <SideQuestionWorkspace side={side} label={agent?.name ?? conv.agent_key} composerLayout="inline">
      {/* header: which agent + live + token meta */}
      <div className="flex min-w-0 flex-wrap items-center gap-2 border-b px-4 py-2.5">
        <Bot className="text-muted-foreground size-4 shrink-0" />
        <span className="min-w-0 max-w-48 truncate text-sm font-medium">{agent?.name ?? conv.agent_key}</span>
        <span className="text-muted-foreground hidden shrink-0 font-mono text-xs sm:inline">{conv.agent_key}</span>
        {agent && !agent.builtin && (
          <Badge variant="outline" className="shrink-0 px-1.5 py-0 text-[10px]">
            {t("Custom")}
          </Badge>
        )}
        {agent?.description && (
          <span className="text-muted-foreground min-w-0 truncate text-xs">{agent.description}</span>
        )}
        {running && <LiveBadge />}
        <SideQuestionButton side={side} />
        <div className="text-muted-foreground ml-auto flex min-w-0 max-w-full items-center justify-end gap-x-3 gap-y-1 text-xs max-sm:w-full max-sm:flex-wrap">
          {tokenTotal.turns > 0 && (
            <span title={t("Agent loop turns (model calls)")} className="tabular-nums">
              {t("{count} turns", { count: tokenTotal.turns })}
            </span>
          )}
          {tokenTotal.any && (
            <span title={t("input / cache(read) / output tokens")} className="min-w-0 truncate tabular-nums">
              {t("input {input} · cache {cache} · output {output}", {
                input: fmtTokens(tokenTotal.i),
                cache: fmtTokens(tokenTotal.cr),
                output: fmtTokens(tokenTotal.o),
              })}
            </span>
          )}
        </div>
      </div>

      <ApprovalExecutionFocus focus={approvalFocus} history={focusHistory} />

      {/* messages */}
      <ScrollArea type="auto" className="min-h-0 min-w-0 flex-1 [&_[data-slot=scroll-area-viewport]>div]:block!">
        <div className="min-w-0 max-w-full px-4 py-3" ref={contentRef}>
          {messages.length === 0 && !running ? (
            <div className="text-muted-foreground py-10 text-center text-sm">
              {canOperate
                ? t('Start a conversation with "{name}"', { name: agent?.name ?? conv.agent_key })
                : t("Conversation messages are only available to operators.")}
            </div>
          ) : (
            <>
              {hasMore && (
                <div className="text-muted-foreground/70 pb-2 text-center text-[11px]">
                  {t("Scroll up to load earlier messages…")}
                </div>
              )}
              <Transcript
                activity={messages}
                live={running}
                chat
                fetchDetail={fetchDetail}
                focusedSeq={focusHistory.ready ? approvalFocus.state?.source?.seq : undefined}
              />
            </>
          )}
        </div>
      </ScrollArea>

      {canOperate && (
        <>
          <Composer
            value={input}
            onChange={setInput}
            onSend={send}
            disabled={running || sending}
            allowBtw
            placeholder={
              running
                ? t("Agent is replying; type /btw to ask a side question…")
                : t("Type a message, @ to reference records, Enter to send")
            }
            running={running}
            onStop={stop}
            stopDisabled={stopping}
            attachments={attachments}
            onPickFiles={pickFiles}
            onRemoveAttachment={(path) => setAttachments((p) => p.filter((x) => x.path !== path))}
            uploading={uploading}
          />
          <LLMProfileRow
            profiles={profiles}
            selected={currentProfileId}
            onChange={changeProfile}
            disabled={running || sending}
            rightSlot={<TodoPopover seq={latestTodoSeq} fetchDetail={fetchDetail} />}
          />
        </>
      )}
    </SideQuestionWorkspace>
  );
}
