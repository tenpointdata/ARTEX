"use client";

import { AlertTriangleIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { t } from "@/i18n";
import type { MissingSkill } from "@/lib/types";

import { fmtTime } from "./tree";

export function MissingSkillsPopover({ missing }: { missing: MissingSkill[] }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm" variant="outline" className="ml-auto">
          <AlertTriangleIcon className="size-3.5 text-amber-500" />
          {t("{count} missed calls", { count: missing.length })}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <p className="mb-2 text-xs text-muted-foreground">
          {t("Skills agents invoke by name that do not exist in the library. Sorted by invocation count.")}
        </p>
        <div className="space-y-1">
          {missing.map((m) => (
            <div key={m.skill} className="flex items-center gap-2 text-sm">
              <code className="min-w-0 flex-1 truncate font-mono text-xs" title={m.skill}>
                {m.skill}
              </code>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {t("{count} calls", { count: m.calls })}
              </span>
              <span className="shrink-0 text-xs text-muted-foreground">{fmtTime(m.last_used)}</span>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
