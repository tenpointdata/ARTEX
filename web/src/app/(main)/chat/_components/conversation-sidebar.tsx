import * as React from "react";

import { Bot, ListChecksIcon, PlusIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { Agent, Conversation } from "@/lib/types";

import type { AgentGroup } from "./chat-helpers";
import { AgentGroupHeader, ConversationItem } from "./conversation-item";

export interface ConversationSidebarProps {
  convs: Conversation[];
  filteredConversations: Conversation[];
  pinnedConversations: Conversation[];
  agentGroups: AgentGroup[];
  agentByKey: Map<string, Agent>;
  agentFilter: string | null;
  agentFilterOptions: { key: string; name: string; count: number }[];
  changeAgentFilter: (key: string | null) => void;
  conversationCountLabel: string;
  conversationHeaderChecked: boolean | "indeterminate";
  selectedConversationCount: number;
  selectedConversationIds: Set<number>;
  selectionMode: boolean;
  setSelectionMode: (on: boolean) => void;
  exitSelectionMode: () => void;
  toggleAllConversations: (checked: boolean) => void;
  toggleConversationSelected: (id: number, checked: boolean) => void;
  bulkDeleting: boolean;
  setBulkDeleteOpen: (open: boolean) => void;
  selectedId: number | null;
  selectConversation: (id: number | null) => void;
  renamingId: number | null;
  renameText: string;
  setRenameText: (v: string) => void;
  startRename: (c: Conversation) => void;
  commitRename: (id: number, title: string) => void;
  cancelRename: () => void;
  togglePinned: (c: Conversation) => void;
  del: (id: number) => void;
  collapsedAgents: Set<string>;
  toggleAgentCollapsed: (key: string) => void;
  hasMore: boolean;
  loadMore: () => void;
}

// ConversationSidebar is the left rail: new button, agent filter, bulk-select
// header, and the pinned + per-agent grouped conversation rows.
export function ConversationSidebar(p: ConversationSidebarProps) {
  const canOperate = useCan("operate"); // starting, renaming, pinning and deleting conversations
  const {
    convs,
    filteredConversations,
    agentFilter,
    selectionMode,
    bulkDeleting,
    selectedConversationCount,
    conversationCountLabel,
  } = p;
  const renderItem = (c: Conversation, showAgent: boolean) => (
    <ConversationItem
      key={c.id}
      conv={c}
      agent={p.agentByKey.get(c.agent_key)}
      showAgent={showAgent}
      active={p.selectedId === c.id}
      renaming={p.renamingId === c.id}
      renameText={p.renamingId === c.id ? p.renameText : ""}
      onSelect={p.selectConversation}
      onStartRename={p.startRename}
      onRenameText={p.setRenameText}
      onCommitRename={p.commitRename}
      onCancelRename={p.cancelRename}
      onTogglePinned={p.togglePinned}
      onDelete={p.del}
      selectionMode={selectionMode}
      selectedForDelete={p.selectedConversationIds.has(c.id)}
      onSelectedForDeleteChange={p.toggleConversationSelected}
    />
  );
  return (
    <div className="bg-card flex flex-col overflow-hidden rounded-lg border">
      <div className="flex flex-col gap-2 border-b p-2">
        {canOperate && (
          <Button size="sm" className="w-full" onClick={() => p.selectConversation(null)}>
            <PlusIcon /> {t("New conversation")}
          </Button>
        )}
        <Select
          value={agentFilter === null ? "all" : `agent:${agentFilter}`}
          onValueChange={(value) => p.changeAgentFilter(value === "all" ? null : value.slice(6))}
          disabled={bulkDeleting}
        >
          <SelectTrigger size="sm" className="w-full min-w-0" aria-label={t("Filter conversations by agent")}>
            <Bot />
            <SelectValue placeholder={t("All agents")} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="all">{t("All agents")}</SelectItem>
              {p.agentFilterOptions.map((agent) => (
                <SelectItem key={agent.key} value={`agent:${agent.key}`}>
                  {agent.name} ({agent.count})
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        {convs.length > 0 &&
          (selectionMode && canOperate ? (
            <div className="flex items-center gap-2 px-1">
              <Checkbox
                checked={p.conversationHeaderChecked}
                onCheckedChange={(checked) => p.toggleAllConversations(checked === true)}
                aria-label={t("Select all conversations matching the current filter")}
                disabled={filteredConversations.length === 0 || bulkDeleting}
              />
              <span className="text-muted-foreground min-w-0 flex-1 text-xs tabular-nums">
                {selectedConversationCount > 0
                  ? t("{count} selected", { count: selectedConversationCount })
                  : conversationCountLabel}
              </span>
              {selectedConversationCount > 0 && (
                <Button
                  size="sm"
                  variant="destructive"
                  disabled={bulkDeleting}
                  onClick={() => p.setBulkDeleteOpen(true)}
                >
                  <Trash2Icon data-icon="inline-start" />
                  {t("Delete")}
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={p.exitSelectionMode}>
                {t("Done")}
              </Button>
            </div>
          ) : (
            <div className="flex items-center gap-2 px-1">
              <span className="text-muted-foreground min-w-0 flex-1 text-xs tabular-nums">
                {conversationCountLabel}
              </span>
              {canOperate && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-muted-foreground"
                  onClick={() => p.setSelectionMode(true)}
                  disabled={filteredConversations.length === 0}
                >
                  <ListChecksIcon data-icon="inline-start" />
                  {t("Select")}
                </Button>
              )}
            </div>
          ))}
      </div>
      <ScrollArea
        key={agentFilter === null ? "all" : `agent:${agentFilter}`}
        type="auto"
        className="min-h-0 min-w-0 flex-1 [&_[data-slot=scroll-area-viewport]>div]:block!"
      >
        <div className="flex min-w-0 flex-col gap-0.5 p-2">
          {filteredConversations.length === 0 && (
            <p className="text-muted-foreground px-2 py-6 text-center text-xs">
              {agentFilter === null ? t("No conversations yet") : t("No conversations for this agent")}
            </p>
          )}
          {p.pinnedConversations.map((c) => renderItem(c, true))}
          {p.agentGroups.map((group) => {
            const collapsed = p.collapsedAgents.has(group.key);
            return (
              <React.Fragment key={group.key}>
                <AgentGroupHeader
                  group={group}
                  collapsed={collapsed}
                  hasActive={group.conversations.some((c) => c.id === p.selectedId)}
                  onToggle={p.toggleAgentCollapsed}
                />
                {!collapsed && group.conversations.map((c) => renderItem(c, false))}
              </React.Fragment>
            );
          })}
          {p.hasMore && (
            <Button type="button" variant="ghost" size="sm" className="mt-1 w-full" onClick={p.loadMore}>
              {t("Load more")}
            </Button>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
