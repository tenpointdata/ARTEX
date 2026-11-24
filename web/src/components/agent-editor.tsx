"use client";

import * as React from "react";

import { toast } from "sonner";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { AgentDetail, MCPServer, PromptVar, PromptVersion, Settings, SkillItem, Tool } from "@/lib/types";

import { PromptTab } from "./agent-editor/prompt-tab";
import { AgentTriggersTab } from "./agent-editor/triggers-tab";
import { McpTab, SkillTab, ToolsTab } from "./agent-editor/visibility-tabs";
import { WrapupTab } from "./agent-editor/wrapup-tab";

// AgentEditor is the tabbed editor for one agent, used inside the agents-page
// drawer (and reused full-page for deep links). Tabs: Config & Prompt / MCP / Skill /
// Tools. Config + prompt save as before; visibility + tool bindings toggle live.
export function AgentEditor({ agentKey, onSaved }: { agentKey: string; onSaved?: () => void }) {
  const [detail, setDetail] = React.useState<AgentDetail | null>(null);
  const [versions, setVersions] = React.useState<PromptVersion[]>([]);
  const [variables, setVariables] = React.useState<PromptVar[]>([]);
  const [mcp, setMcp] = React.useState<MCPServer[]>([]);
  const [skills, setSkills] = React.useState<SkillItem[]>([]);
  const [tools, setTools] = React.useState<Tool[]>([]);
  const [loaded, setLoaded] = React.useState(false);

  const [prompt, setPrompt] = React.useState("");
  const [mcpVisible, setMcpVisible] = React.useState<number[]>([]);
  const [skillVisible, setSkillVisible] = React.useState<string[]>([]);
  const [preview, setPreview] = React.useState("");
  const [maxTurns, setMaxTurns] = React.useState("0");
  const [runSecs, setRunSecs] = React.useState("600");
  // "" = follow (unbound); otherwise a profile id string
  const [llmProfileId, setLlmProfileId] = React.useState("");
  const [llmProfiles, setLlmProfiles] = React.useState<NonNullable<AgentDetail["llm_profiles"]>>([]);
  const [webSearch, setWebSearch] = React.useState(false);
  const [interactiveShell, setInteractiveShell] = React.useState(false);
  const [wrapup, setWrapup] = React.useState("");
  const [wrapupDefault, setWrapupDefault] = React.useState("");
  const [wrapupTurns, setWrapupTurns] = React.useState("0");
  const [wrapupTurnsDefault, setWrapupTurnsDefault] = React.useState(5);
  // task-level timeout wrap-up prompt (worker/planner only)
  const [ttSupported, setTtSupported] = React.useState(false);
  const [ttWrapup, setTtWrapup] = React.useState("");
  const [ttWrapupDefault, setTtWrapupDefault] = React.useState("");
  const [ttTurns, setTtTurns] = React.useState("0");
  const [ttTurnsDefault, setTtTurnsDefault] = React.useState(5);
  const [settings, setSettings] = React.useState<Settings | null>(null);

  React.useEffect(() => {
    api
      .mcpServers()
      .then(setMcp)
      .catch(() => {});
    api
      .skills()
      .then(setSkills)
      .catch(() => {});
    api
      .tools()
      .then(setTools)
      .catch(() => {});
    api
      .settings()
      .then(setSettings)
      .catch(() => {});
  }, []);
  // global gates: traffic tools need traffic capture, web search needs the master switch.
  const captureOn = !!settings?.traffic_capture;
  const webSearchGlobalOn = !!settings?.web_search_enabled;

  const reload = React.useCallback(() => {
    api
      .getAgent(agentKey)
      .then((d) => {
        setDetail(d);
        setPrompt(d.prompt ?? "");
        setVariables(d.variables ?? []);
        setVersions(d.versions ?? []);
        setMcpVisible(d.visibility?.mcp ?? []);
        setSkillVisible(d.visibility?.skill ?? []);
        setMaxTurns(String(d.agent?.max_turns ?? 0));
        setRunSecs(String(d.agent?.run_seconds ?? 600));
        setLlmProfileId(d.agent?.llm_profile_id != null ? String(d.agent.llm_profile_id) : "");
        setLlmProfiles(d.llm_profiles ?? []);
        setWebSearch(!!d.agent?.web_search);
        setInteractiveShell(!!d.agent?.interactive_shell);
        setWrapup(d.wrapup_prompt ?? "");
        setWrapupDefault(d.wrapup_default ?? "");
        setWrapupTurns(String(d.wrapup_max_turns ?? 0));
        setWrapupTurnsDefault(d.wrapup_max_turns_default ?? 5);
        setTtSupported(!!d.task_timeout_wrapup_supported);
        setTtWrapup(d.task_timeout_wrapup_prompt ?? "");
        setTtWrapupDefault(d.task_timeout_wrapup_default ?? "");
        setTtTurns(String(d.task_timeout_wrapup_max_turns ?? 0));
        setTtTurnsDefault(d.task_timeout_wrapup_max_turns_default ?? 5);
      })
      .catch(() => setDetail(null))
      .finally(() => setLoaded(true));
  }, [agentKey]);
  React.useEffect(() => {
    reload();
  }, [reload]);

  async function doPreview() {
    try {
      const r = await api.previewAgentPrompt(agentKey, prompt);
      setPreview(r.error ? t("Render error: {error}", { error: r.error }) : r.rendered);
    } catch (e) {
      setPreview(t("Preview failed: {error}", { error: (e as Error).message }));
    }
  }
  async function savePrompt() {
    try {
      const r = await api.saveAgentPrompt(agentKey, prompt);
      toast.success(t("Saved as version v{version}", { version: r.version }));
      reload();
      onSaved?.();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    }
  }
  async function resetPrompt() {
    try {
      const r = await api.resetAgentPrompt(agentKey);
      toast.success(t("Restored to built-in default (v{version})", { version: r.version }));
      reload();
    } catch (e) {
      toast.error(t("Restore failed: {error}", { error: (e as Error).message }));
    }
  }
  async function saveWrapup() {
    try {
      const turns = Math.max(0, Math.floor(Number(wrapupTurns) || 0));
      await api.saveAgentWrapup(agentKey, wrapup, turns);
      toast.success(
        wrapup.trim() || turns > 0
          ? t("Wrap-up config saved (takes effect on the next run)")
          : t("Cleared; the built-in default will be used"),
      );
      reload();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    }
  }
  async function resetWrapup() {
    try {
      await api.resetAgentWrapup(agentKey);
      toast.success(t("Restored to built-in default"));
      reload();
    } catch (e) {
      toast.error(t("Restore failed: {error}", { error: (e as Error).message }));
    }
  }
  async function saveTaskTimeoutWrapup() {
    try {
      const turns = Math.max(0, Math.floor(Number(ttTurns) || 0));
      await api.saveAgentTaskTimeoutWrapup(agentKey, ttWrapup, turns);
      toast.success(t("Task timeout wrap-up config saved (takes effect on the next run)"));
      reload();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    }
  }
  async function resetTaskTimeoutWrapup() {
    try {
      await api.resetAgentTaskTimeoutWrapup(agentKey);
      toast.success(t("Restored to built-in default"));
      reload();
    } catch (e) {
      toast.error(t("Restore failed: {error}", { error: (e as Error).message }));
    }
  }
  async function saveConfig() {
    try {
      // Only submit the fields this agent actually displays, to avoid overwriting hidden items (such as max_turns for goals) with defaults.
      const patch: Parameters<typeof api.saveAgentConfig>[1] = {
        llm_profile_id: llmProfileId === "" ? null : Number(llmProfileId),
      };
      if (showConfig) {
        patch.max_turns = Math.max(0, Math.floor(Number(maxTurns) || 0));
        patch.run_seconds = Math.max(0, Math.floor(Number(runSecs) || 0));
      }
      if (showWebSearch) patch.web_search = webSearch;
      if (showInteractiveShell) patch.interactive_shell = interactiveShell;
      await api.saveAgentConfig(agentKey, patch);
      toast.success(t("Run config saved (effective immediately)"));
      reload();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    }
  }
  // applyVis optimistically updates, persists, and toasts success/failure. On
  // failure it reverts to the prior selection so the UI never lies about state.
  async function applyVis(nextMcp: number[], nextSkill: string[], okMsg: string) {
    const prevMcp = mcpVisible;
    const prevSkill = skillVisible;
    setMcpVisible(nextMcp);
    setSkillVisible(nextSkill);
    try {
      await api.setAgentVisibility(agentKey, nextMcp, nextSkill);
      toast.success(okMsg);
      onSaved?.(); // refresh the list so the card's MCP/Skill counts stay in sync
    } catch (e) {
      setMcpVisible(prevMcp);
      setSkillVisible(prevSkill);
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    }
  }
  function toggleMcp(id: number) {
    const on = mcpVisible.includes(id);
    const name = mcp.find((m) => m.id === id)?.name ?? String(id);
    applyVis(
      on ? mcpVisible.filter((x) => x !== id) : [...mcpVisible, id],
      skillVisible,
      on ? t('Revoked MCP "{name}" visibility', { name }) : t('Granted MCP "{name}" visibility', { name }),
    );
  }
  function toggleSkill(name: string) {
    const on = skillVisible.includes(name);
    applyVis(
      mcpVisible,
      on ? skillVisible.filter((x) => x !== name) : [...skillVisible, name],
      on ? t('Revoked Skill "{name}" visibility', { name }) : t('Granted Skill "{name}" visibility', { name }),
    );
  }
  async function toggleTool(tool: Tool) {
    const on = tool.agents.includes(agentKey);
    const nextAgents = on ? tool.agents.filter((k) => k !== agentKey) : [...tool.agents, agentKey];
    // optimistic update
    setTools((ts) => ts.map((x) => (x.key === tool.key ? { ...x, agents: nextAgents } : x)));
    try {
      await api.saveTool(tool.key, {
        description: tool.description,
        schema: tool.schema,
        agents: nextAgents,
        enabled: tool.enabled,
      });
      toast.success(on ? t('Unbound tool "{key}"', { key: tool.key }) : t('Bound tool "{key}"', { key: tool.key }));
      onSaved?.(); // refresh the list so the card's tool count stays in sync
    } catch (e) {
      toast.error(t("Failed to save tool bindings: {error}", { error: (e as Error).message }));
      reload();
      api
        .tools()
        .then(setTools)
        .catch(() => {});
    }
  }

  if (loaded && !detail) {
    return (
      <div className="text-muted-foreground p-6 text-center text-sm">
        {t("Agent not found: {key}", { key: agentKey })}
      </div>
    );
  }
  // config is meaningless for the conversational main agent and the fixed-budget
  // goals decomposer; every other agent (workers, custom assistants) honors it.
  const showConfig = agentKey !== "mainagent" && agentKey !== "goals";
  // web search applies to every conversational/executing agent except the one-shot
  // goals decomposer; it's gated by the global master switch.
  const showWebSearch = agentKey !== "goals";
  // interactive shell (persistent PTY session tool family) is likewise open to all agents except goals; no global gate.
  const showInteractiveShell = agentKey !== "goals";
  // Every agent (including goals/mainagent) runs on some LLM, so the "default model" binding is open to all agents.
  const showLLM = true;
  // triggers (P3) only attach to custom agents.
  const isCustom = !!detail && !detail.agent?.builtin;

  return (
    <Tabs defaultValue="prompt" className="flex min-h-0 flex-1 flex-col">
      <TabsList className="mx-4 mt-2 w-fit">
        <TabsTrigger value="prompt">{t("Config & Prompt")}</TabsTrigger>
        <TabsTrigger value="wrapup">{t("Wrap-up Prompt")}</TabsTrigger>
        <TabsTrigger value="mcp">MCP</TabsTrigger>
        <TabsTrigger value="skill">{t("Skill")}</TabsTrigger>
        <TabsTrigger value="tools">{t("Tools")}</TabsTrigger>
        {isCustom && <TabsTrigger value="triggers">{t("Triggers")}</TabsTrigger>}
      </TabsList>

      <TabsContent value="prompt" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <PromptTab
          {...{ showLLM, showConfig, showWebSearch, showInteractiveShell, webSearchGlobalOn }}
          {...{ llmProfileId, setLlmProfileId, llmProfiles, maxTurns, setMaxTurns, runSecs, setRunSecs }}
          {...{ webSearch, setWebSearch, interactiveShell, setInteractiveShell, saveConfig }}
          {...{ variables, versions, prompt, setPrompt, preview, doPreview, savePrompt, resetPrompt }}
        />
      </TabsContent>

      <TabsContent value="wrapup" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <WrapupTab
          {...{ wrapup, setWrapup, wrapupDefault, wrapupTurns, setWrapupTurns, wrapupTurnsDefault }}
          {...{ saveWrapup, resetWrapup, ttSupported, ttWrapup, setTtWrapup, ttWrapupDefault }}
          {...{ ttTurns, setTtTurns, ttTurnsDefault, saveTaskTimeoutWrapup, resetTaskTimeoutWrapup }}
        />
      </TabsContent>

      <TabsContent value="mcp" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <McpTab mcp={mcp} visible={mcpVisible} onToggle={toggleMcp} />
      </TabsContent>

      <TabsContent value="skill" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <SkillTab skills={skills} visible={skillVisible} onToggle={toggleSkill} />
      </TabsContent>

      <TabsContent value="tools" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
        <ToolsTab tools={tools} agentKey={agentKey} captureOn={captureOn} onToggle={toggleTool} />
      </TabsContent>

      {/* Triggers (P3, custom agents only) */}
      {isCustom && (
        <TabsContent value="triggers" className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
          <AgentTriggersTab agentKey={agentKey} agent={detail?.agent} />
        </TabsContent>
      )}
    </Tabs>
  );
}
