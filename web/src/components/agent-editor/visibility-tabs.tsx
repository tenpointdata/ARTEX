"use client";

import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { MCPServer, SkillItem, Tool } from "@/lib/types";
import { cn } from "@/lib/utils";

// Traffic tools are host tools gated by the global traffic capture switch: bindable, but
// only usable when capture is on. Keep this list in sync with traffic.SeedToolMetas.
const TRAFFIC_TOOL_KEYS = new Set(["traffic_search", "traffic_get"]);

export function McpTab({
  mcp,
  visible,
  onToggle,
}: {
  mcp: MCPServer[];
  visible: number[];
  onToggle: (id: number) => void;
}) {
  const uid = React.useId();
  const canAdmin = useCan("admin"); // visibility bindings are admin-only
  return (
    <>
      <p className="text-muted-foreground mb-3 text-xs">{t("Check the MCP servers visible to this Agent.")}</p>
      <div className="grid gap-2">
        {mcp.map((m) => (
          <label
            key={m.id}
            htmlFor={`${uid}-${m.id}`}
            className="flex items-center gap-2 rounded-md border p-2 text-sm"
          >
            <Checkbox
              id={`${uid}-${m.id}`}
              checked={visible.includes(m.id)}
              disabled={!canAdmin}
              onCheckedChange={() => onToggle(m.id)}
            />
            {m.name}
            <span className="text-muted-foreground ml-auto text-xs">{m.transport}</span>
          </label>
        ))}
        {mcp.length === 0 && <span className="text-muted-foreground text-xs">{t("(no MCP servers)")}</span>}
      </div>
    </>
  );
}

export function SkillTab({
  skills,
  visible,
  onToggle,
}: {
  skills: SkillItem[];
  visible: string[];
  onToggle: (name: string) => void;
}) {
  const uid = React.useId();
  const canAdmin = useCan("admin"); // visibility bindings are admin-only
  return (
    <>
      <p className="text-muted-foreground mb-3 text-xs">{t("Check the Skills visible to this Agent.")}</p>
      <div className="grid gap-2">
        {skills.map((s) => (
          <label
            key={s.name}
            htmlFor={`${uid}-${s.name}`}
            className="flex items-center gap-2 rounded-md border p-2 text-sm"
          >
            <Checkbox
              id={`${uid}-${s.name}`}
              checked={visible.includes(s.name)}
              disabled={!canAdmin}
              onCheckedChange={() => onToggle(s.name)}
            />
            <span className="font-mono text-xs">{s.name}</span>
            {s.description && <span className="text-muted-foreground ml-auto truncate text-xs">{s.description}</span>}
          </label>
        ))}
        {skills.length === 0 && <span className="text-muted-foreground text-xs">{t("(no Skills)")}</span>}
      </div>
    </>
  );
}

export function ToolsTab({
  tools,
  agentKey,
  captureOn,
  onToggle,
}: {
  tools: Tool[];
  agentKey: string;
  captureOn: boolean;
  onToggle: (tool: Tool) => void;
}) {
  const uid = React.useId();
  const canAdmin = useCan("admin"); // tool bindings are admin-only
  return (
    <>
      <p className="text-muted-foreground mb-3 text-xs">{t("Check the built-in tools bound to this Agent.")}</p>
      <div className="grid gap-2">
        {tools.map((tool) => {
          const isTraffic = TRAFFIC_TOOL_KEYS.has(tool.key);
          const gated = isTraffic && !captureOn; // traffic tools need traffic capture on
          return (
            <label
              key={tool.key}
              htmlFor={`${uid}-${tool.key}`}
              className={cn("flex items-center gap-2 rounded-md border p-2 text-sm", gated && "opacity-60")}
            >
              <Checkbox
                id={`${uid}-${tool.key}`}
                checked={tool.agents.includes(agentKey)}
                disabled={gated || !canAdmin}
                onCheckedChange={() => onToggle(tool)}
              />
              <span className="font-mono text-xs">{tool.key}</span>
              {isTraffic && (
                <Badge variant="secondary" className="px-1 py-0 text-[9px]">
                  {t("Traffic")}
                </Badge>
              )}
              {!tool.enabled && (
                <Badge variant="outline" className="text-destructive px-1 py-0 text-[9px]">
                  {t("Disabled")}
                </Badge>
              )}
              {gated ? (
                <span className="text-muted-foreground ml-auto text-xs">{t("Requires traffic capture")}</span>
              ) : (
                tool.description && (
                  <span className="text-muted-foreground ml-auto line-clamp-1 max-w-[55%] text-xs">
                    {tool.description}
                  </span>
                )
              )}
            </label>
          );
        })}
        {tools.length === 0 && <span className="text-muted-foreground text-xs">{t("(no tools)")}</span>}
      </div>
    </>
  );
}
