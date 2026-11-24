"use client";

import * as React from "react";

import { toast } from "sonner";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { targetGateLocked } from "@/lib/governance";
import type { Task } from "@/lib/types";

// TaskGateSwitch shows and toggles the task's target gate (approval for asset
// targets). While the task has an engagement authorization the gate is
// required: the switch is locked on and says why, so no click reaches the
// server's 409 (db.ErrTargetGateRequired).
export function TaskGateSwitch({ task, onUpdated }: { task: Task; onUpdated: () => void }) {
  const canOperate = useCan("operate");
  const [saving, setSaving] = React.useState(false);
  const locked = targetGateLocked(task.authorization);
  const on = locked || task.target_gate === true;
  let reason = on
    ? t("Target gate on: a call to a target without a pin waits for one approval click.")
    : t("Target gate off: only the global block rules apply to targets.");
  if (task.authorization) {
    reason = t("Required while the task has an engagement authorization (v{n}); it cannot be turned off.", {
      n: task.authorization.version,
    });
  } else if (!canOperate) {
    reason = t("Only operators and admins can change the target gate.");
  }

  const toggle = async (next: boolean) => {
    setSaving(true);
    try {
      await api.updateTask(task.id, { target_gate: next });
      toast.success(next ? t("Target gate turned on") : t("Target gate turned off"));
    } catch (e) {
      toast.error(t("Update failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
      onUpdated();
    }
  };

  const id = `task-${task.id}-target-gate`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-1.5">
          <Switch
            id={id}
            size="sm"
            checked={on}
            disabled={locked || !canOperate || saving}
            onCheckedChange={(next) => void toggle(next)}
            aria-describedby={`${id}-reason`}
          />
          <Label htmlFor={id} className="hidden font-normal text-xs sm:inline">
            {locked ? t("Gate required") : t("Target gate")}
          </Label>
          <span id={`${id}-reason`} className="sr-only">
            {reason}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{reason}</TooltipContent>
    </Tooltip>
  );
}
