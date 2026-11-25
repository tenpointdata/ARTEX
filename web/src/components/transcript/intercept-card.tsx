"use client";

import * as React from "react";

import { CheckIcon, ChevronDown, ChevronRight, ShieldAlertIcon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { ApprovalDetail } from "@/components/approval-records";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Activity, InterceptPending } from "@/lib/types";

export function InterceptCard({ step, getDetail }: { step: Activity; getDetail: (seq: number) => Promise<string> }) {
  // extract pending_id from summary: "tool Bash requests approval (#42)"
  const pendingId = React.useMemo(() => {
    const m = /\(#(\d+)\)/.exec(step.summary);
    return m ? parseInt(m[1], 10) : null;
  }, [step.summary]);

  const toolName = React.useMemo(() => {
    const m = /tool\s+(\S+)\s+requests/.exec(step.summary);
    return m ? m[1] : step.summary;
  }, [step.summary]);

  const [detail, setDetail] = React.useState<Record<string, unknown> | null>(null);
  const [decided, setDecided] = React.useState<"allowed" | "denied" | "timeout" | null>(null);
  const [deciding, setDeciding] = React.useState(false);
  const [expanded, setExpanded] = React.useState(false);
  const [pending, setPending] = React.useState<InterceptPending | null>(null);
  const [statusError, setStatusError] = React.useState("");
  const [retry, setRetry] = React.useState(0);

  // Load persisted detail JSON + check the real current status from the backend
  // so that a page refresh shows the already-decided state instead of re-offering buttons.
  React.useEffect(() => {
    let live = true;
    getDetail(step.seq)
      .then((raw) => {
        if (!live || !raw) return;
        try {
          setDetail(JSON.parse(raw));
        } catch {
          /* ignore */
        }
      })
      .catch(() => {
        /* ignore */
      });
    return () => {
      live = false;
    };
  }, [step.seq, getDetail]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: Retry explicitly reloads the same approval after a failed request.
  React.useEffect(() => {
    if (!pendingId) return;
    let live = true;
    api
      .interceptGetOne(pendingId)
      .then((p) => {
        if (!live) return;
        setPending(p);
        setStatusError("");
        if (p.status !== "pending") setDecided(p.status as "allowed" | "denied" | "timeout");
      })
      .catch((error) => {
        if (live) setStatusError((error as Error).message || t("Failed to load approval details"));
      });
    return () => {
      live = false;
    };
  }, [pendingId, retry]);

  async function decide(decision: "allowed" | "denied") {
    if (step.inherited || !pendingId || deciding) return;
    setDeciding(true);
    try {
      await api.interceptDecide(pendingId, decision);
      setDecided(decision);
      toast.success(decision === "allowed" ? t("Execution allowed") : t("Execution denied"));
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setDeciding(false);
    }
  }

  const inputStr = detail?.input ? JSON.stringify(detail.input).slice(0, 200) : null;

  const row = pending
    ? { ...pending, status: decided || pending.status, conv_title: "", conv_agent_key: "", rule_name: "" }
    : null;

  return (
    <div className="my-2 rounded-lg border border-amber-400/50 bg-amber-50/40 dark:bg-amber-950/15 p-3 text-xs">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-2 min-w-0">
          <ShieldAlertIcon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
          <div className="min-w-0 space-y-0.5">
            <div className="flex items-center gap-1.5 font-medium">
              <span className="text-amber-700 dark:text-amber-400">{t("Approval request")}</span>
              <code className="rounded bg-amber-100 dark:bg-amber-900/50 px-1 font-mono text-amber-800 dark:text-amber-300">
                {toolName}
              </code>
              {pendingId && <span className="text-muted-foreground">#{pendingId}</span>}
            </div>
            {inputStr && <p className="font-mono text-muted-foreground truncate">{inputStr}</p>}
          </div>
        </div>

        {step.inherited ? (
          <Badge variant="outline">{t("History · Read-only")}</Badge>
        ) : decided ? (
          <span
            className={
              "shrink-0 rounded px-2 py-0.5 text-[11px] font-medium " +
              (decided === "allowed"
                ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400"
                : decided === "timeout"
                  ? "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                  : "bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-400")
            }
          >
            {decided === "allowed" ? t("Allowed") : decided === "timeout" ? t("Timed out") : t("Denied")}
          </span>
        ) : (
          <div className="flex shrink-0 gap-1.5">
            <Button
              size="sm"
              className="h-6 px-2 text-xs"
              disabled={deciding || !pendingId}
              onClick={() => decide("allowed")}
            >
              <CheckIcon className="h-3 w-3" />
              {t("Allow")}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              className="h-6 px-2 text-xs"
              disabled={deciding || !pendingId}
              onClick={() => decide("denied")}
            >
              <XIcon className="h-3 w-3" />
              {t("Deny")}
            </Button>
          </div>
        )}
      </div>
      {pendingId ? (
        <Collapsible open={expanded} onOpenChange={setExpanded} className="mt-2 min-w-0">
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              aria-label={expanded ? t("Collapse approval details") : t("Expand approval details")}
            >
              {expanded ? <ChevronDown data-icon="inline-start" /> : <ChevronRight data-icon="inline-start" />}
              {expanded ? t("Collapse approval details") : t("Expand approval details")}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            {expanded && row ? (
              <ApprovalDetail
                row={row}
                busy={deciding}
                decide={(_id, decision) => decide(decision)}
                revision={retry}
                readOnly={Boolean(step.inherited)}
                defaultExpanded
                onResolved={setDecided}
              />
            ) : statusError ? (
              <div className="flex flex-wrap items-center gap-2 p-3" role="alert">
                <span>{statusError}</span>
                <Button variant="outline" size="sm" onClick={() => setRetry((value) => value + 1)}>
                  {t("Retry details")}
                </Button>
              </div>
            ) : (
              <p className="p-3 text-muted-foreground">{t("Loading approval details…")}</p>
            )}
          </CollapsibleContent>
        </Collapsible>
      ) : null}
    </div>
  );
}

// ToolBlock renders ONE tool call: the command and its result bound into a single
// row (collapsed shows the command + a status/result preview; expand shows the
// full input AND output together). Lazy-loads both details on expand.
