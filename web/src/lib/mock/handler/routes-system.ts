import type { Conversation } from "@/lib/types";

import * as D from "../data";
import { stopMockRetest } from "./evidence";
import { NO_ROUTE, type RouteCtx } from "./route-types";
import { mockConversations, mockLLMRecords, mockRetestMessages, mockRetests } from "./state";
import { sortMockConversations } from "./task-helpers";

export function routeSystem({ m, path, seg, q, b }: RouteCtx): unknown {
  if (path === "/tokens/daily") return D.dailyTokens;
  if (path === "/tokens/conversations") return D.convTokens;

  // ── traffic / audit / settings ──
  if (path === "/audit") return D.audit;
  if (path === "/traffic" && m === "DELETE") return { deleted: 0 };
  if (path === "/traffic/hosts" && m === "DELETE") return { deleted: (b.hosts as unknown[])?.length ?? 0 };
  if (path === "/traffic/hosts") return { hosts: D.trafficHosts };
  if (path === "/traffic") return D.traffic;
  if (path === "/traffic/exchange") return D.trafficStreamDetails[q.get("id") ?? ""] ?? D.trafficDetail;
  if (path === "/traffic/stream") {
    const direction = (q.get("direction") ?? "").trim().toLowerCase();
    if (direction && direction !== "c2s" && direction !== "s2c") throw new Error("invalid stream direction");
    const items = (D.trafficStreamMessages[q.get("exchange") ?? ""] ?? []).filter(
      (message) => !direction || message.direction === direction || message.direction === "meta",
    );
    const offset = Math.max(0, Number(q.get("offset")) || 0);
    const requestedLimit = Number(q.get("limit"));
    const limit = requestedLimit > 0 ? Math.min(200, requestedLimit) : 50;
    return {
      items: items.slice(offset, offset + limit),
      total: items.length,
      message_total: items.filter((message) => message.direction !== "meta").length,
    };
  }
  if (path === "/settings" && m === "GET") return D.settings;
  if (path === "/settings" && m === "PUT") return { ...D.settings, ...b };
  if (path === "/settings/web-search/test") return { ok: true, count: 5, backend: D.settings.web_search_backend };
  if (path === "/settings/python/detect") return { python_interpreter: "/usr/bin/python3" };
  if (path === "/chat")
    return {
      reply:
        "(demo) I have injected this suggestion as a high-priority intent; the work agent will execute it as soon as possible.",
      mode: "hint",
    };
  if (path === "/gc") return { removed: 0 };

  // ── Tool execution history ──
  if (path === "/commands" && m === "GET") return { commands: D.commandRecords, total: D.commandRecords.length };
  if (path === "/commands/stats" && m === "GET") {
    const tally = new Map<string, { tool: string; total: number; errors: number }>();
    for (const c of D.commandRecords) {
      const tool = c.tool || "-";
      const s = tally.get(tool) ?? { tool, total: 0, errors: 0 };
      s.total++;
      if (c.is_error) s.errors++;
      tally.set(tool, s);
    }
    return { stats: [...tally.values()].sort((a, b) => b.total - a.total || a.tool.localeCompare(b.tool)) };
  }

  // ── LLM ──
  if (path === "/llm/records" && m === "GET") return { records: mockLLMRecords, total: mockLLMRecords.length };
  if (path === "/llm/records" && m === "DELETE") return { deleted: 0 };
  if (path === "/llm/records/tasks") {
    const counts = new Map<string, number>();
    for (const record of mockLLMRecords) {
      if (record.task_id) counts.set(record.task_id, (counts.get(record.task_id) ?? 0) + 1);
    }
    return { tasks: [...counts].map(([task_id, count]) => ({ task_id, count })) };
  }
  if (seg[0] === "llm" && seg[1] === "records" && seg.length === 3 && m === "GET") {
    return D.llmRecordDetail(Number(seg[2]), mockLLMRecords);
  }
  if (path === "/llm" && m === "GET") return D.llmConfig;
  if (path === "/llm" && m === "POST") return { ok: true };
  if (path === "/llm/test")
    return { ok: true, latency_ms: 128, model: String(b.model ?? "claude-opus-4-8"), reply: "OK" };
  if (path === "/llm/profiles" && m === "GET") return { profiles: D.llmProfiles };
  if (path === "/llm/profiles" && m === "POST") return { id: Number(b.id) || 3 };
  if (path === "/llm/profiles/active") return { ok: true };
  if (path === "/llm/pool" && m === "GET") return D.llmPool;
  if (path === "/llm/pool/reset")
    return { ...D.llmPool, chain: D.llmPool.chain.map((c) => ({ ...c, state: "ok", fails: 0, cooldown_secs: 0 })) };
  if (seg[0] === "llm" && seg[1] === "profiles" && seg.length === 3 && m === "DELETE")
    return { deleted: Number(seg[2]) };

  // ── agents ──
  if (path === "/agents" && m === "GET") return { agents: D.agents };
  if (path === "/agents" && m === "POST")
    return {
      id: "9",
      key: String(b.key ?? "custom"),
      name: String(b.name ?? ""),
      role: "custom",
      builtin: false,
      enabled: true,
    };
  if (seg[0] === "agents" && seg.length === 2 && m === "GET") return D.agentDetail(seg[1]);
  if (seg[0] === "agents" && seg[2] === "triggers" && m === "GET") return { triggers: [] };
  if (seg[0] === "agents" && seg[2] === "prompts") return { versions: D.agentDetail(seg[1]).versions };
  if (seg[0] === "agents" && seg[2] === "variables") return { variables: D.agentDetail(seg[1]).variables };
  if (seg[0] === "agents" && seg[2] === "prompt" && seg[3] === "preview")
    return { rendered: String(b.template ?? "").replace(/\{\{\.(\w+)\}\}/g, "«$1»") };
  if (seg[0] === "agents" && seg[2] === "visibility" && m === "GET") return D.agentDetail(seg[1]).visibility;

  // ── conversations ──
  if (path === "/conversations" && m === "GET") {
    sortMockConversations();
    return {
      conversations: structuredClone(
        mockConversations.map((conversation) => ({
          ...conversation,
          running: mockRetests.some(
            (item) => item.conversation_id === conversation.id && ["pending", "running"].includes(item.status),
          ),
        })),
      ),
    };
  }
  if (path === "/conversations" && m === "POST") {
    const now = new Date().toISOString();
    const title = String(b.title ?? "").trim() || "New conversation";
    const conversation: Conversation = {
      id: mockConversations.reduce((max, item) => Math.max(max, item.id), 0) + 1,
      agent_key: String(b.agent_key ?? "mainagent"),
      title,
      llm_profile_id: typeof b.llm_profile_id === "number" ? b.llm_profile_id : undefined,
      pinned: false,
      created_at: now,
      updated_at: now,
    };
    mockConversations.unshift(conversation);
    return structuredClone(conversation);
  }
  if (seg[0] === "conversations" && seg.length === 2 && m === "PATCH") {
    const conversation = mockConversations.find((item) => item.id === Number(seg[1]));
    if (!conversation) return {};
    if (typeof b.title === "string") conversation.title = b.title.trim();
    if (typeof b.pinned === "boolean") {
      conversation.pinned = b.pinned;
      conversation.pinned_at = b.pinned ? (conversation.pinned_at ?? new Date().toISOString()) : null;
    }
    conversation.updated_at = new Date().toISOString();
    sortMockConversations();
    return structuredClone(conversation);
  }
  if (seg[0] === "conversations" && seg.length === 2 && m === "DELETE") {
    const id = Number(seg[1]);
    stopMockRetest(id);
    const index = mockConversations.findIndex((item) => item.id === id);
    if (index >= 0) mockConversations.splice(index, 1);
    for (const retest of mockRetests) if (retest.conversation_id === id) retest.conversation_id = null;
    return { deleted: id };
  }
  if (path === "/conversations/delete/batch" && m === "POST") {
    const ids = Array.isArray(b.ids)
      ? [...new Set(b.ids.map(Number).filter((id) => Number.isInteger(id) && id > 0))]
      : [];
    const items = ids.map((id) => {
      const index = mockConversations.findIndex((item) => item.id === id);
      if (index < 0) return { id, ok: false, error: "conversation not found" };
      mockConversations.splice(index, 1);
      stopMockRetest(id);
      for (const retest of mockRetests) if (retest.conversation_id === id) retest.conversation_id = null;
      return { id, ok: true };
    });
    return { items };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && seg.length === 3 && m === "GET") {
    const items = mockRetestMessages[Number(seg[1])] ?? D.conversationMessages[Number(seg[1])] ?? [];
    const running = mockRetests.some(
      (item) => item.conversation_id === Number(seg[1]) && ["pending", "running"].includes(item.status),
    );
    return { items, cursor: items.length ? items[items.length - 1].seq : 0, running };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && seg.length === 4) {
    const msgs = mockRetestMessages[Number(seg[1])] ?? D.conversationMessages[Number(seg[1])] ?? [];
    const a = msgs.find((x) => x.seq === Number(seg[3]));
    return { detail: a?.detail ?? a?.summary ?? "" };
  }
  if (seg[0] === "conversations" && seg[2] === "messages" && m === "POST") return { status: "ok" };
  if (seg[0] === "conversations" && seg[2] === "stop") {
    stopMockRetest(Number(seg[1]));
    return { status: "stopped" };
  }

  // ── tools ──
  if (path === "/tools" && m === "GET") return { tools: D.tools };
  if (path === "/tools/custom" && m === "POST") return { key: String(b.key ?? "custom-tool") };
  if (path === "/tools/custom/test") return { output: "(demo) Sample tool execution output.", is_error: false };

  // ── mcp ──
  if (path === "/mcp" && m === "GET") return { servers: D.mcpServers };
  if (path === "/mcp" && m === "POST") return { id: 3 };
  if (seg[0] === "mcp" && seg[2] === "tools") return { tools: D.mcpToolsById[Number(seg[1])] ?? [] };
  if (seg[0] === "mcp" && seg[2] === "refresh") return { tools: D.mcpToolsById[Number(seg[1])] ?? [] };
  if (seg[0] === "mcp" && seg.length === 2 && m === "DELETE") return { deleted: Number(seg[1]) };

  // ── scopesentry (demo: not configured) ──
  if (path === "/sync/scopesentry/status")
    return { exists: false, configured: false, enabled: false, reachable: false, tools: [] };
  if (path === "/sync/scopesentry/projects") return { projects: [], tag: {} };
  if (path === "/sync/scopesentry/tasks") return { tasks: [] };
  if (path === "/sync/scopesentry/sync") return { synced: {}, companies: null, warnings: null, errors: null };

  // ── skills ──
  if (path === "/skills" && m === "GET") return { skills: D.skills };
  if (path === "/skills/missing") return { missing: D.missingSkills };
  if (seg[0] === "skills" && seg[2] === "usage") return { calls: D.skillCalls };
  if (path === "/skills" && m === "POST") return { name: String(b.name ?? "new-skill") };
  if (seg[0] === "skills" && seg[2] === "files" && seg.length === 3) return { files: ["SKILL.md"] };
  if (seg[0] === "skills" && seg[2] === "files" && seg.length >= 4)
    return {
      content: "# SKILL.md\n\n(demo) This is a sample description file for this skill.",
      file: seg.slice(3).join("/"),
    };

  // ── visibility ──
  if (seg[0] === "visibility" && m === "GET") return { agents: [] };

  return NO_ROUTE;
}
