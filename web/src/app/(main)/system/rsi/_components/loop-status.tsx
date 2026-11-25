"use client";

import { t } from "@/i18n";
import type { RSILoopStatus } from "@/lib/rsi-types";
import { cn } from "@/lib/utils";

// LoopStatus shows the ARTEX_RSI_LOOP flag and what it gates. The flag is an
// owner decision made in the server environment, so it is shown, not toggled.
export function LoopStatus({ status }: { status: RSILoopStatus | null }) {
  if (!status) return null;
  const on = status.loop === "on";
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-1 rounded-md border px-3 py-2 text-sm">
      <span className="flex items-center gap-2">
        <span className={cn("size-2 rounded-full", on ? "bg-emerald-500" : "bg-slate-400")} />
        <span className="font-mono text-xs">{status.flag}</span>
        <span className="font-medium">{on ? t("on") : t("off")}</span>
      </span>
      <span className="text-muted-foreground">
        {on
          ? t("Proposals, evidence and adoption are open.")
          : t("Adoption is closed until the owner sets {flag}=on. Reject and rollback stay open.", {
              flag: status.flag,
            })}
      </span>
      <span className="text-muted-foreground">
        {t("Lessons mode")}: <span className="font-mono text-xs">{status.lessons_mode}</span>
      </span>
      <span className="text-muted-foreground">
        {t("Edits per round")}: <span className="font-mono text-xs">{status.budget_by_round.join(" → ")}</span>
      </span>
    </div>
  );
}
