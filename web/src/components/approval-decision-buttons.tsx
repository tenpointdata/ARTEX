"use client";

import { CheckIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";

// Allow / Deny for a pending interception approval. Deciding needs the review
// permission (server/rbac_policy.go), so roles without it see nothing.
// compact is the inline size used in transcripts.
export function ApprovalDecisionButtons({
  disabled,
  onDecide,
  compact = false,
}: {
  disabled: boolean;
  onDecide: (decision: "allowed" | "denied") => void;
  compact?: boolean;
}) {
  if (!useCan("review")) return null;
  const btn = compact ? "h-6 px-2 text-xs" : undefined;
  const icon = compact ? "h-3 w-3" : undefined;
  return (
    <div className={compact ? "flex shrink-0 gap-1.5" : "flex flex-wrap gap-2"}>
      <Button size="sm" className={btn} disabled={disabled} onClick={() => onDecide("allowed")}>
        <CheckIcon className={icon} data-icon="inline-start" />
        {t("Allow")}
      </Button>
      <Button size="sm" variant="destructive" className={btn} disabled={disabled} onClick={() => onDecide("denied")}>
        <XIcon className={icon} data-icon="inline-start" />
        {t("Deny")}
      </Button>
    </div>
  );
}

const decidedStyle = {
  allowed: "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400",
  timeout: "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400",
  denied: "bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-400",
};

// The outcome of an already-decided approval.
export function ApprovalDecidedBadge({ decided }: { decided: keyof typeof decidedStyle }) {
  const label = { allowed: t("Allowed"), timeout: t("Timed out"), denied: t("Denied") }[decided];
  return (
    <span className={`shrink-0 rounded px-2 py-0.5 font-medium text-[11px] ${decidedStyle[decided]}`}>{label}</span>
  );
}
