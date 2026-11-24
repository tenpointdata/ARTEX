"use client";

import * as React from "react";

import { toast } from "sonner";

import { ApprovalDecisionButtons } from "@/components/approval-decision-buttons";
import { Button } from "@/components/ui/button";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import type { InterceptApprovalRow } from "@/lib/types";

export type Decide = (id: number, decision: "allowed" | "denied") => Promise<void>;

export function DecisionActions({ row, busy, decide }: { row: InterceptApprovalRow; busy: boolean; decide: Decide }) {
  if (row.status !== "pending") return null;
  return <ApprovalDecisionButtons disabled={busy} onDecide={(d) => void decide(row.id, d)} />;
}

// AlwaysAllowAction creates an argv allow rule from a pending Bash request.
// The prefix comes from the server; the request itself stays pending.
export function AlwaysAllowAction({ id, prefix }: { id: number; prefix?: string }) {
  const canAdmin = useCan("admin"); // creating an intercept rule is an admin action
  const [busy, setBusy] = React.useState(false);
  async function create() {
    setBusy(true);
    try {
      const res = await api.interceptAlwaysAllow(id);
      toast.success(
        res.created
          ? t("Rule created, the request is still pending approval")
          : t("Rule already exists, the request is still pending approval"),
      );
    } catch (e) {
      toast.error(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  if (!canAdmin) return null;
  return (
    <div className="flex flex-col gap-1">
      <Button size="sm" variant="outline" className="self-start" disabled={busy} onClick={() => void create()}>
        {prefix ? t('Always allow "{prefix} *"', { prefix }) : t("Always allow")}
      </Button>
      <p className="text-muted-foreground text-xs">
        {t(
          "The prefix does not constrain flags, and it does not cover what the program does next (git hooks, npm scripts, docker images).",
        )}
      </p>
    </div>
  );
}
