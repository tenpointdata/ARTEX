"use client";

import * as React from "react";

import { toast } from "sonner";

import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import { getLocalStorageValue, removeLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client";
import type { Agent, ChatAttachment, Conversation, LLMProfile } from "@/lib/types";

import { BulkDeleteDialog } from "./_components/bulk-delete-dialog";
import {
  COLLAPSED_AGENTS_KEY,
  CONVERSATION_LIST_PAGE,
  conversationIsPinned,
  groupByAgent,
  LAST_CONVERSATION_KEY,
} from "./_components/chat-helpers";
import { ChatView } from "./_components/chat-view";
import { ConversationSidebar } from "./_components/conversation-sidebar";
import { DraftChat } from "./_components/draft-chat";

export default function ChatPage() {
  const [agents, setAgents] = React.useState<Agent[]>([]);
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [convs, setConvs] = React.useState<Conversation[]>([]);
  const [agentFilter, setAgentFilter] = React.useState<string | null>(null);
  const [selectedId, setSelectedId] = React.useState<number | null>(null);
  const [sourceRequested, setSourceRequested] = React.useState(false);
  const [convsLoaded, setConvsLoaded] = React.useState(false);
  const selectConversation = React.useCallback(
    (id: number | null) => {
      if (id !== selectedId) {
        const url = new URL(window.location.href);
        url.searchParams.delete("approval");
        setSourceRequested(false);
        window.history.replaceState(null, "", url);
      }
      setSelectedId(id);
    },
    [selectedId],
  );

  const [renamingId, setRenamingId] = React.useState<number | null>(null);
  const [renameText, setRenameText] = React.useState("");
  const [selectedConversationIds, setSelectedConversationIds] = React.useState<Set<number>>(() => new Set());
  // selectionMode gates the multi-select UI: off by default (clean list, no
  // checkboxes); the header "Select" button turns it on, "Done" turns it off and
  // clears the selection.
  const [selectionMode, setSelectionMode] = React.useState(false);
  const [bulkDeleteOpen, setBulkDeleteOpen] = React.useState(false);
  const [bulkDeleting, setBulkDeleting] = React.useState(false);
  const [visibleConversationCount, setVisibleConversationCount] = React.useState(CONVERSATION_LIST_PAGE);
  // Collapsed agent groups. Hydrated from localStorage after mount (not a lazy
  // useState init) so the server and client render the same first pass.
  const [collapsedAgents, setCollapsedAgents] = React.useState<Set<string>>(() => new Set());
  // Pending text + uploaded attachments handed off from a draft that created a
  // conversation via the paperclip, keyed by the new conversation id (consumed once
  // by ChatView on mount; ids never repeat, so leftover entries are harmless).
  const [pendingByConv, setPendingByConv] = React.useState<
    Record<number, { input?: string; attachments?: ChatAttachment[] }>
  >({});

  const conversationListSeq = React.useRef(0);
  const reloadConvs = React.useCallback(async () => {
    const seq = ++conversationListSeq.current;
    try {
      const items = await api.conversations();
      if (seq === conversationListSeq.current) {
        setConvs(items);
        setConvsLoaded(true);
      }
    } catch {
      // Preserve the selected transcript and list on a transient poll failure.
    }
  }, []);
  React.useEffect(() => {
    api
      .agents()
      .then(setAgents)
      .catch(() => {
        /* Agent metadata is optional for rendering an existing conversation. */
      });
    api
      .llmProfiles()
      .then(setProfiles)
      .catch(() => {
        /* The conversation remains usable without profile labels. */
      });
  }, []);

  // One list poll supplies runtime state for all sidebar rows, including the
  // unselected ones. Await completion so slow requests do not overlap.
  React.useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      await reloadConvs();
      if (!disposed) timer = setTimeout(() => void poll(), 2000);
    }
    void poll();
    return () => {
      disposed = true;
      conversationListSeq.current++;
      clearTimeout(timer);
    };
  }, [reloadConvs]);

  React.useEffect(() => {
    setSelectedConversationIds((current) => {
      if (current.size === 0) return current;
      const live = new Set(convs.map((conversation) => conversation.id));
      const next = new Set([...current].filter((id) => live.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [convs]);

  React.useEffect(() => {
    const raw = getLocalStorageValue(COLLAPSED_AGENTS_KEY);
    if (!raw) return;
    try {
      const keys = JSON.parse(raw);
      if (Array.isArray(keys))
        setCollapsedAgents(new Set(keys.filter((key): key is string => typeof key === "string")));
    } catch {
      // Corrupted entry → start with everything expanded.
    }
  }, []);

  const toggleAgentCollapsed = React.useCallback((key: string) => {
    setCollapsedAgents((current) => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      setLocalStorageValue(COLLAPSED_AGENTS_KEY, JSON.stringify([...next]));
      return next;
    });
  }, []);

  // Restore the open conversation on mount, so a refresh or a return from another
  // page lands on the same thread instead of the empty draft view: the URL
  // (?c=<id>) first, else the last conversation opened here. Runs after
  // hydration (not a lazy useState init) to avoid a server/client mismatch.
  React.useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    setSourceRequested(params.has("approval"));
    const c = params.get("c") ?? getLocalStorageValue(LAST_CONVERSATION_KEY);
    const id = c ? Number(c) : NaN;
    if (Number.isFinite(id)) setSelectedId(id);
  }, []);
  // Mirror the current selection into the URL (replaceState → no history spam). A
  // selectedId with no matching conversation (e.g. a stale ?c=, or a just-created
  // one before reloadConvs lands) simply renders the draft view — harmless — so we
  // deliberately do NOT auto-clear it here (that raced new-conversation creation).
  // The same value goes to localStorage for the restore above. On mount this
  // effect runs once with the initial null before the restored id lands; the
  // restore has already read the stored value by then and writes it back on the
  // next render.
  React.useEffect(() => {
    const url = new URL(window.location.href);
    if (selectedId != null) {
      url.searchParams.set("c", String(selectedId));
      setLocalStorageValue(LAST_CONVERSATION_KEY, String(selectedId));
    } else {
      url.searchParams.delete("c");
      removeLocalStorageValue(LAST_CONVERSATION_KEY);
    }
    window.history.replaceState(null, "", url);
  }, [selectedId]);

  const selected = React.useMemo(() => convs.find((c) => c.id === selectedId) ?? null, [convs, selectedId]);
  const agentByKey = React.useMemo(() => new Map(agents.map((agent) => [agent.key, agent])), [agents]);
  const filteredConversations = React.useMemo(
    () => (agentFilter === null ? convs : convs.filter((conversation) => conversation.agent_key === agentFilter)),
    [convs, agentFilter],
  );
  const visibleConversations = React.useMemo(
    () => filteredConversations.slice(0, visibleConversationCount),
    [filteredConversations, visibleConversationCount],
  );
  // Pinned rows stay a flat block above the groups (server order = pinned_at DESC);
  // everything else is bucketed per agent, most-recently-active agent first.
  const pinnedConversations = React.useMemo(
    () => visibleConversations.filter(conversationIsPinned),
    [visibleConversations],
  );
  const agentGroups = React.useMemo(
    () =>
      groupByAgent(
        visibleConversations.filter((c) => !conversationIsPinned(c)),
        agentByKey,
      ),
    [visibleConversations, agentByKey],
  );
  // conversation agents: custom agents + conversational built-ins (role=assistant,
  // e.g. Auto / penetration testing). The orchestration built-ins (goals/planner/mainagent/worker)
  // are task-specific and stay hidden from the chat page.
  const chatAgents = React.useMemo(() => agents.filter((a) => !a.builtin || a.role === "assistant"), [agents]);
  const agentFilterOptions = React.useMemo(() => {
    const counts = new Map<string, number>();
    for (const conversation of convs) {
      counts.set(conversation.agent_key, (counts.get(conversation.agent_key) ?? 0) + 1);
    }
    // Include historical sources even if their Agent has since been disabled or deleted.
    const keys = new Set([...chatAgents.map((agent) => agent.key), ...counts.keys()]);
    if (agentFilter !== null) keys.add(agentFilter);
    return [...keys]
      .map((key) => ({ key, name: agentByKey.get(key)?.name || key, count: counts.get(key) ?? 0 }))
      .sort((a, b) => a.name.localeCompare(b.name, intlLocale));
  }, [convs, chatAgents, agentByKey, agentFilter]);
  const conversationCountLabel =
    agentFilter === null
      ? t("{count} total", { count: convs.length })
      : `${filteredConversations.length} / ${convs.length}`;

  function changeAgentFilter(key: string | null) {
    setAgentFilter(key);
    setVisibleConversationCount(CONVERSATION_LIST_PAGE);
    setSelectedConversationIds(new Set());
    setBulkDeleteOpen(false);
    setRenamingId(null);
  }

  const selectedConversationCount = selectedConversationIds.size;
  const allConversationsSelected =
    filteredConversations.length > 0 && selectedConversationCount === filteredConversations.length;
  const someConversationsSelected = selectedConversationCount > 0 && !allConversationsSelected;
  let conversationHeaderChecked: boolean | "indeterminate" = false;
  if (allConversationsSelected) conversationHeaderChecked = true;
  else if (someConversationsSelected) conversationHeaderChecked = "indeterminate";

  const toggleConversationSelected = React.useCallback((id: number, checked: boolean) => {
    setSelectedConversationIds((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  function toggleAllConversations(checked: boolean) {
    setSelectedConversationIds(
      checked ? new Set(filteredConversations.map((conversation) => conversation.id)) : new Set(),
    );
  }

  function exitSelectionMode() {
    setSelectionMode(false);
    setSelectedConversationIds(new Set());
  }

  const del = React.useCallback(
    async (id: number) => {
      try {
        await api.deleteConversation(id);
        setSelectedId((current) => (current === id ? null : current));
        setSelectedConversationIds((current) => {
          if (!current.has(id)) return current;
          const next = new Set(current);
          next.delete(id);
          return next;
        });
        void reloadConvs();
      } catch (e) {
        toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
      }
    },
    [reloadConvs],
  );

  async function deleteSelectedConversations() {
    const ids = [...selectedConversationIds];
    if (ids.length === 0 || bulkDeleting) return;
    setBulkDeleting(true);
    const deleted = new Set<number>();
    const failed: { id: number; error: string }[] = [];
    try {
      for (let offset = 0; offset < ids.length; offset += 100) {
        const result = await api.deleteConversations(ids.slice(offset, offset + 100));
        for (const item of result.items) {
          if (item.ok) deleted.add(item.id);
          else failed.push({ id: item.id, error: item.error ?? t("conversation not found") });
        }
      }
      if (deleted.has(selectedId ?? -1)) selectConversation(null);
      setSelectedConversationIds((current) => {
        const next = new Set(current);
        for (const id of deleted) next.delete(id);
        return next;
      });
      if (deleted.size > 0) toast.success(t("Deleted {count} conversations", { count: deleted.size }));
      if (failed.length > 0) {
        const details = failed
          .slice(0, 3)
          .map((item) => `#${item.id} (${item.error})`)
          .join("; ");
        toast.error(
          t("Failed to delete {count} conversations: {details}", {
            count: failed.length,
            details: failed.length > 3 ? t("{items} etc.", { items: details }) : details,
          }),
        );
      }
      setBulkDeleteOpen(false);
      // Fully successful → return to the clean list; keep selection mode on if
      // some failed so the user can retry the remaining ones.
      if (failed.length === 0) setSelectionMode(false);
      void reloadConvs();
    } catch (error) {
      toast.error(t("Bulk delete failed: {error}", { error: (error as Error).message }));
      void reloadConvs();
    } finally {
      setBulkDeleting(false);
    }
  }

  const togglePinned = React.useCallback(
    async (conversation: Conversation) => {
      const pinned = conversationIsPinned(conversation);
      try {
        await api.pinConversation(conversation.id, !pinned);
        void reloadConvs();
      } catch (e) {
        const error = (e as Error).message;
        toast.error(pinned ? t("Failed to unpin: {error}", { error }) : t("Failed to pin: {error}", { error }));
      }
    },
    [reloadConvs],
  );

  const startRename = React.useCallback((c: Conversation) => {
    setRenamingId(c.id);
    setRenameText(c.title || "");
  }, []);
  const commitRename = React.useCallback(
    async (id: number, value: string) => {
      const title = value.trim();
      setRenamingId(null);
      if (!title) return;
      try {
        await api.renameConversation(id, title);
        void reloadConvs();
      } catch (e) {
        toast.error(t("Rename failed: {error}", { error: (e as Error).message }));
      }
    },
    [reloadConvs],
  );
  const cancelRename = React.useCallback(() => setRenamingId(null), []);

  return (
    <div
      data-content-padding="false"
      className="flex h-[calc(100svh-3rem)] min-w-0 flex-col overflow-hidden p-3 sm:p-4 md:h-[calc(100svh-4rem)] md:p-6"
    >
      <div className="grid min-h-0 min-w-0 flex-1 grid-cols-1 grid-rows-[minmax(10rem,15rem)_minmax(0,1fr)] gap-3 md:grid-cols-[18rem_minmax(0,1fr)] md:grid-rows-[minmax(0,1fr)] md:gap-4">
        <ConversationSidebar
          convs={convs}
          filteredConversations={filteredConversations}
          pinnedConversations={pinnedConversations}
          agentGroups={agentGroups}
          agentByKey={agentByKey}
          agentFilter={agentFilter}
          agentFilterOptions={agentFilterOptions}
          changeAgentFilter={changeAgentFilter}
          conversationCountLabel={conversationCountLabel}
          conversationHeaderChecked={conversationHeaderChecked}
          selectedConversationCount={selectedConversationCount}
          selectedConversationIds={selectedConversationIds}
          selectionMode={selectionMode}
          setSelectionMode={setSelectionMode}
          exitSelectionMode={exitSelectionMode}
          toggleAllConversations={toggleAllConversations}
          toggleConversationSelected={toggleConversationSelected}
          bulkDeleting={bulkDeleting}
          setBulkDeleteOpen={setBulkDeleteOpen}
          selectedId={selectedId}
          selectConversation={selectConversation}
          renamingId={renamingId}
          renameText={renameText}
          setRenameText={setRenameText}
          startRename={startRename}
          commitRename={commitRename}
          cancelRename={cancelRename}
          togglePinned={togglePinned}
          del={del}
          collapsedAgents={collapsedAgents}
          toggleAgentCollapsed={toggleAgentCollapsed}
          hasMore={visibleConversationCount < filteredConversations.length}
          loadMore={() => setVisibleConversationCount((count) => count + CONVERSATION_LIST_PAGE)}
        />

        {/* right: chat view */}
        <div className="bg-card flex min-w-0 flex-col overflow-hidden rounded-lg border">
          {selected ? (
            <ChatView
              key={selected.id}
              conv={selected}
              agents={agents}
              profiles={profiles}
              initial={pendingByConv[selected.id]}
              onTitleMaybeChanged={reloadConvs}
              onConvUpdated={reloadConvs}
            />
          ) : sourceRequested ? (
            <div role="status" className="p-6 text-sm text-muted-foreground">
              {convsLoaded ? t("The conversation has been deleted") : t("Loading the conversation…")}
            </div>
          ) : (
            <DraftChat
              agents={chatAgents}
              profiles={profiles}
              onStarted={(c, pending) => {
                if (agentFilter !== null && agentFilter !== c.agent_key) changeAgentFilter(null);
                // Insert the new conversation immediately so `selected` resolves to
                // it on this render (switching to ChatView right away, before the
                // async reloadConvs lands); reloadConvs then reconciles titles etc.
                if (pending) setPendingByConv((p) => ({ ...p, [c.id]: pending }));
                setConvs((prev) => {
                  if (prev.some((item) => item.id === c.id)) return prev;
                  const firstUnpinned = prev.findIndex((item) => !conversationIsPinned(item));
                  const insertAt = firstUnpinned < 0 ? prev.length : firstUnpinned;
                  return [...prev.slice(0, insertAt), c, ...prev.slice(insertAt)];
                });
                setSelectedId(c.id);
                void reloadConvs();
              }}
            />
          )}
        </div>
      </div>
      <BulkDeleteDialog
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        count={selectedConversationCount}
        deleting={bulkDeleting}
        onConfirm={() => void deleteSelectedConversations()}
      />
    </div>
  );
}
