"use client";

import { Badge } from "@/components/ui/badge";
import { t } from "@/i18n";
import type { Tool } from "@/lib/types";

// Traffic tools are host tools gated by the global traffic capture switch: bindable, but
// only usable when capture is on. Keep in sync with traffic.SeedToolMetas.

// ToolGridCard is one clickable tile in the catalog grid.
export function ToolGridCard({ tool, onClick }: { tool: Tool; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex flex-col gap-2 rounded-lg border p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium font-mono text-sm">{tool.key}</span>
        {tool.system ? (
          <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
            {t("System")}
          </Badge>
        ) : (
          <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
            {t("Custom · {kind}", { kind: tool.kind ?? "" })}
          </Badge>
        )}
        {tool.deferred && (
          <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
            {t("deferred")}
          </Badge>
        )}
        {!tool.enabled && (
          <Badge variant="outline" className="px-1.5 py-0 text-[10px] text-destructive">
            {t("Disabled")}
          </Badge>
        )}
        <Badge
          variant="secondary"
          className="ml-auto px-1.5 py-0 text-[10px] tabular-nums"
          title={t("{count} calls in total", { count: tool.calls ?? 0 })}
        >
          {t("{count} calls", { count: tool.calls ?? 0 })}
        </Badge>
      </div>
      <p className="line-clamp-1 h-4 text-muted-foreground text-xs">{tool.description || t("(no description)")}</p>
      <div className="mt-auto flex flex-wrap gap-1 pt-1">
        {tool.agents.length === 0 && (
          <span className="text-[10px] text-muted-foreground">{t("(no agents bound)")}</span>
        )}
        {tool.agents.map((a) => (
          <Badge key={a} variant="outline" className="px-1.5 py-0 text-[10px]">
            {a}
          </Badge>
        ))}
      </div>
    </button>
  );
}
