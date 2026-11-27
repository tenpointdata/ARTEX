"use client";

import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { Agent, MCPServer, SkillCall, SkillItem } from "@/lib/types";

import { fmtTime } from "./tree";

export function SkillDetail({
  selectedSkill,
  agents,
  mcpOptions,
  detailMcps,
  visibility,
  usageCalls,
  usageLoading,
  onToggleMcp,
  onToggleVisibility,
}: {
  selectedSkill: SkillItem;
  agents: Agent[];
  mcpOptions: MCPServer[];
  detailMcps: string[];
  visibility: Record<string, string[]>;
  usageCalls: SkillCall[];
  usageLoading: boolean;
  onToggleMcp: (skillName: string, mcpName: string, on: boolean) => void;
  onToggleVisibility: (skillName: string, agentId: string, agentName: string) => void;
}) {
  // Skill MCP metadata is /api/skills (operate); per-agent visibility is /api/visibility (admin).
  const canOperate = useCan("operate");
  const canAdmin = useCan("admin");
  return (
    <div className="max-w-5xl space-y-5">
      <div>
        <h2 className="font-mono text-base font-semibold">{selectedSkill.name}</h2>
        {selectedSkill.description && <p className="mt-1 text-sm text-muted-foreground">{selectedSkill.description}</p>}
        <div className="mt-2 flex flex-wrap gap-2">
          {selectedSkill.license && (
            <Badge variant="outline" className="text-xs font-normal">
              {t("License: {license}", { license: selectedSkill.license })}
            </Badge>
          )}
          {selectedSkill.compatibility && (
            <Badge variant="secondary" className="text-xs font-normal">
              {selectedSkill.compatibility}
            </Badge>
          )}
        </div>
      </div>

      {/* Two columns: config (MCP/visibility) primary on the left, call stats secondary on the right.
                  Below lg they fall back to one column via flex-row-reverse — stats come first in DOM order,
                  so on narrow screens they land above the config (same top-down order as before the redesign). */}
      <div className="flex flex-col gap-6 lg:flex-row-reverse lg:items-start">
        {/* ── right: call stats ── */}
        <div className="space-y-2 lg:w-80 lg:shrink-0">
          <Label className="text-xs text-muted-foreground">{t("Call stats")}</Label>
          <div className="grid grid-cols-3 gap-2">
            <div className="rounded-md border p-2">
              <p className="text-lg font-semibold tabular-nums">{selectedSkill.calls}</p>
              <p className="text-xs text-muted-foreground">{t("Total calls")}</p>
            </div>
            <div className="rounded-md border p-2">
              <p className="text-lg font-semibold tabular-nums">{selectedSkill.tasks}</p>
              <p className="text-xs text-muted-foreground">{t("Tasks covered")}</p>
            </div>
            <div className="rounded-md border p-2">
              <p className="truncate text-sm font-medium" title={fmtTime(selectedSkill.last_used)}>
                {fmtTime(selectedSkill.last_used)}
              </p>
              <p className="text-xs text-muted-foreground">{t("Last called")}</p>
            </div>
          </div>
          {selectedSkill.usage_agents.length > 0 && (
            <div className="flex flex-wrap items-center gap-1">
              <span className="text-xs text-muted-foreground">{t("Called by:")}</span>
              {selectedSkill.usage_agents.map((k) => (
                <Badge key={k} variant="secondary" className="text-xs font-normal">
                  {k}
                </Badge>
              ))}
            </div>
          )}
          {usageLoading ? (
            <p className="text-xs text-muted-foreground">{t("Loading call details…")}</p>
          ) : usageCalls.length > 0 ? (
            <div className="rounded-md border">
              <div className="border-b px-2 py-1 text-xs text-muted-foreground">
                {t("Last {count} calls", { count: usageCalls.length })}
              </div>
              <div className="max-h-56 overflow-y-auto">
                {usageCalls.map((c, i) => (
                  <div
                    key={`${c.ts}-${i}`}
                    className="flex items-center gap-2 border-b px-2 py-1 text-xs last:border-b-0"
                  >
                    <span className="tabular-nums text-muted-foreground">{fmtTime(c.ts)}</span>
                    <Badge variant="outline" className="font-normal">
                      {c.agent_key || "—"}
                    </Badge>
                    <span className="ml-auto text-muted-foreground">
                      {c.task_id > 0 ? t("Task #{id}", { id: c.task_id }) : c.session_id ? t("Chat session") : "—"}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">{t("No call records yet.")}</p>
          )}
        </div>

        {/* ── left: linked MCP + visibility ── */}
        <div className="space-y-5 lg:min-w-0 lg:flex-1">
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">
              {t("Linked MCP")}
              <span className="ml-1 font-normal"> {t("(tools are disclosed/unlocked only when the skill loads)")}</span>
            </Label>
            {mcpOptions.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t('No MCP servers yet; add one on the "MCP" page.')}</p>
            ) : (
              <div className="flex flex-wrap gap-x-4 gap-y-2">
                {mcpOptions.map((m) => (
                  <label
                    key={m.id}
                    htmlFor={`skill-mcp-${m.id}`}
                    className="flex cursor-pointer items-center gap-2 text-sm"
                  >
                    <Checkbox
                      id={`skill-mcp-${m.id}`}
                      checked={detailMcps.includes(m.name)}
                      disabled={!canOperate}
                      onCheckedChange={(on) => onToggleMcp(selectedSkill.name, m.name, !!on)}
                    />
                    {m.name}
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">{t("Visibility (granted per agent)")}</Label>
            <div className="space-y-2">
              {agents.map((a) => (
                <label
                  key={a.key}
                  htmlFor={`skill-agent-${a.id}`}
                  className="flex cursor-pointer items-center gap-2 text-sm"
                >
                  <Checkbox
                    id={`skill-agent-${a.id}`}
                    checked={(visibility[selectedSkill.name] ?? []).includes(a.id)}
                    disabled={!canAdmin}
                    onCheckedChange={() => onToggleVisibility(selectedSkill.name, a.id, a.name)}
                  />
                  {a.name}
                </label>
              ))}
              {agents.length === 0 && <span className="text-xs text-muted-foreground">{t("(no agents)")}</span>}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
