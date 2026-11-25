"use client";

import * as React from "react";

import { BotIcon, CheckIcon, ChevronDownIcon, CopyIcon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Kbd } from "@/components/ui/kbd";
import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import { APPROVAL_VERDICTS, type ApprovalDecision } from "@/lib/triage";
import type { InterceptApprovalRow, InterceptAudit, InterceptReviewInput, InterceptRisk } from "@/lib/types";
export function fmtTime(value?: string) {
  if (!value) return "—";
  return new Date(value).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function source(row: InterceptApprovalRow) {
  if (row.decision_source) return row.decision_source;
  if (row.rule_id) return "rule";
  return row.reason?.startsWith("[model]") ? "model" : "unknown";
}

export function originLabel(row: InterceptApprovalRow) {
  if (row.task_id) return row.task_id;
  if (row.conversation_id) return t("Conversation #{id}", { id: row.conversation_id });
  return "—";
}

export function ApprovalOrigin({ row, detail = false }: { row: InterceptApprovalRow; detail?: boolean }) {
  const [locating, setLocating] = React.useState(false);
  const label = detail && row.task_id ? t("Task {id}", { id: row.task_id }) : originLabel(row);
  const query = new URLSearchParams({ approval: String(row.id) });
  let href: string | undefined;
  if (row.conversation_id) {
    query.set("c", String(row.conversation_id));
    href = `/chat?${query}`;
  } else if (row.task_id) {
    query.set("id", row.task_id);
    href = `/function/tasks/detail?${query}`;
  }
  return href ? (
    <a
      href={href}
      className="text-primary underline-offset-4 hover:underline"
      aria-label={t("Locate the source of approval #{id}: {label}", { id: row.id, label })}
      aria-busy={locating}
      onClick={async (e) => {
        e.stopPropagation();
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        e.preventDefault();
        if (locating) return;
        setLocating(true);
        try {
          await api.interceptExecution(row.id, row.conversation_id ?? undefined);
          window.location.assign(href);
        } catch (error) {
          toast.error((error as Error).message || t("Unable to locate the corresponding execution"));
          setLocating(false);
        }
      }}
    >
      {label}
    </a>
  ) : (
    <span>{label}</span>
  );
}

export function StatusBadge({ status }: { status: string }) {
  const labels: Record<string, string> = {
    pending: t("Pending approval"),
    allowed: t("Allowed"),
    denied: t("Denied"),
    timeout: t("Timed out"),
  };
  let variant: "default" | "destructive" | "secondary" | "outline" = "outline";
  if (status === "allowed") variant = "default";
  if (status === "denied") variant = "destructive";
  if (status === "pending") variant = "secondary";
  return <Badge variant={variant}>{labels[status] ?? status}</Badge>;
}

export const decidedByLabels: Record<string, string> = {
  human: t("Human"),
  rule: t("Rule"),
  model: t("Model"),
  system: t("System"),
};

// decided_by is shown exactly as stored, so a row always sits under the
// "Decided by" filter option matching its badge. A decided row without an actor
// predates the column: it reads "Legacy" and matches no option.
export function DecidedByBadge({ row }: { row: InterceptApprovalRow }) {
  if (row.status === "pending") return <span className="text-muted-foreground">—</span>;
  if (!row.decided_by) {
    return (
      <Badge variant="outline" className="text-muted-foreground">
        {t("Legacy")}
      </Badge>
    );
  }
  return <Badge variant="outline">{decidedByLabels[row.decided_by] ?? row.decided_by}</Badge>;
}

export const riskLabels: Record<InterceptRisk["level"], string> = {
  low: t("Low"),
  medium: t("Medium"),
  high: t("High"),
};

// RiskBadge shows the server's deterministic risk level of a pending request.
export function RiskBadge({ risk }: { risk?: InterceptRisk }) {
  if (!risk) return null;
  let variant: "destructive" | "secondary" | "outline" = "outline";
  if (risk.level === "high") variant = "destructive";
  if (risk.level === "medium") variant = "secondary";
  return (
    <Badge variant={variant} title={t("Risk score {score}/100", { score: risk.score })}>
      {riskLabels[risk.level] ?? risk.level}
    </Badge>
  );
}

export function MatchCell({ row, showReason = true }: { row: InterceptApprovalRow; showReason?: boolean }) {
  const reason = row.reason?.replace(/^\[model\]\s*/, "");
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {source(row) === "model" ? (
        <Badge variant="outline">
          <BotIcon />
          {t("Model decision")}
        </Badge>
      ) : (
        <span className="truncate">{row.rule_name || t("Rule not recorded or deleted")}</span>
      )}
      {showReason ? (
        <p className="truncate text-muted-foreground text-xs" title={reason}>
          {reason || t("No reason recorded")}
        </p>
      ) : null}
    </div>
  );
}

export function CodeBlock({ label, text, truncated = false }: { label: string; text: string; truncated?: boolean }) {
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t("Copied"));
    } catch {
      toast.error(t("Copy failed; select the content manually to copy"));
    }
  }
  return (
    <section className="flex min-w-0 flex-col gap-2" aria-label={label}>
      <div className="flex items-center justify-between gap-2">
        <h3 className="font-medium text-muted-foreground text-xs">{label}</h3>
        {text ? (
          <Button variant="ghost" size="icon-xs" aria-label={t("Copy {label}", { label })} onClick={() => void copy()}>
            <CopyIcon />
          </Button>
        ) : null}
      </div>
      <pre className="max-h-80 min-w-0 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/60 p-3 font-mono text-xs leading-6 [overflow-wrap:anywhere]">
        {text || t("Not recorded")}
      </pre>
      {truncated ? (
        <p className="text-muted-foreground text-xs">{t("Content truncated; the above is the saved excerpt.")}</p>
      ) : null}
    </section>
  );
}

export const contextLabels: Record<string, string> = {
  user: t("User message"),
  assistant: t("Agent message"),
  text: t("Agent message"),
  tool_use: t("Tool request"),
  tool_result: t("Tool output"),
};

export const actionLabels: Record<string, string> = {
  allow: t("Allow"),
  ask: t("Escalate to human approval"),
  deny: t("Deny"),
};
export const executionLabels: Record<InterceptAudit["execution_status"], string> = {
  not_started: t("Not started"),
  not_executed: t("Not executed"),
  awaiting_result: t("Allowed, awaiting execution result"),
  succeeded: t("Execution succeeded"),
  failed: t("Execution failed"),
  unknown: t("Execution result unknown"),
};

export function ModelReviewContext({ input }: { input: InterceptReviewInput }) {
  return (
    <section className="flex min-w-0 flex-col gap-4" aria-label={t("Model review context")}>
      <div className="flex flex-col gap-2">
        <h3 className="font-medium text-sm">{t("Model review context")}</h3>
        <p className="text-muted-foreground text-xs">
          {t(
            "Below is the snapshot of the input actually sent to the review model for this decision. The background only helps it understand the current action; the verdict follows the review policy.",
          )}
          {input.version < 4
            ? ` ${t("This record used the legacy input format; the content actually sent at the time is preserved.")}`
            : null}
        </p>
      </div>
      <CodeBlock
        label={t("Call under review")}
        text={JSON.stringify({ tool_name: input.tool_name, arguments: input.arguments }, null, 2)}
      />
      {input.version >= 2 ? (
        input.background ? (
          <div className="flex min-w-0 flex-col gap-2">
            <CodeBlock
              label={
                input.background.source === "user_message"
                  ? t("Background · user message")
                  : t("Background · Worker intent summary (legacy)")
              }
              text={input.background.text}
              truncated={input.background.truncated}
            />
            <p className="text-muted-foreground text-xs">
              {input.background.source === "user_message"
                ? t("Taken from the current user message.")
                : t(
                    "This is the legacy Worker intent summary that used to be sent; the new Worker review no longer sends it.",
                  )}
            </p>
          </div>
        ) : (
          <p className="text-muted-foreground text-xs">{t("No background message was attached to this review.")}</p>
        )
      ) : (
        <>
          {input.turn_input ? (
            <CodeBlock
              label={t("Current turn input (legacy)")}
              text={input.turn_input}
              truncated={input.background_truncated}
            />
          ) : null}
          {input.task ? (
            <>
              <CodeBlock
                label={t("Task description (legacy)")}
                text={input.task.description}
                truncated={input.task.truncated}
              />
              <CodeBlock label={t("Task goals (legacy)")} text={input.task.goal} truncated={input.task.truncated} />
              <CodeBlock
                label={t("Task operation constraints (legacy)")}
                text={JSON.stringify(input.task.constraints, null, 2)}
              />
            </>
          ) : null}
          {input.worker_intent ? (
            <CodeBlock
              label={t("Worker intent (legacy)")}
              text={input.worker_intent}
              truncated={input.background_truncated}
            />
          ) : null}
        </>
      )}
      {input.working_directory ? <CodeBlock label={t("Working directory")} text={input.working_directory} /> : null}
      {input.version >= 3 ? (
        <p className="text-muted-foreground text-xs">
          {t("No past calls or execution results were sent for this review.")}
        </p>
      ) : (
        <div className="flex min-w-0 flex-col gap-3">
          <h4 className="font-medium text-muted-foreground text-xs">
            {t("Past calls provided to the model at the time (legacy)")}
          </h4>
          {input.history?.length ? (
            input.history.map((entry) => (
              <div key={entry.tool_use_id} className="flex min-w-0 flex-col gap-2 rounded-lg border p-3">
                <p className="break-words font-medium text-xs">
                  {entry.tool} ·{" "}
                  {entry.status === "succeeded" ? t("Succeeded") : t("Failed (may have partial side effects)")}
                </p>
                <CodeBlock
                  label={t("Past call arguments")}
                  text={entry.arguments_preview}
                  truncated={entry.truncated}
                />
                <CodeBlock label={t("Past execution result")} text={entry.result} truncated={entry.truncated} />
              </div>
            ))
          ) : (
            <p className="text-muted-foreground text-xs">{t("No paired past tool executions were provided.")}</p>
          )}
          {input.history_truncated ? (
            <p className="text-muted-foreground text-xs">
              {t("History is a limited window; some content is truncated.")}
            </p>
          ) : null}
        </div>
      )}

      <Collapsible>
        <CollapsibleTrigger asChild>
          <Button variant="outline" size="sm" className="self-start">
            <ChevronDownIcon data-icon="inline-start" />
            {t("View full model review input JSON")}
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-3">
          <CodeBlock label={t("Model review input")} text={JSON.stringify(input, null, 2)} />
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}

// Callers fire and forget; an implementation may resolve to a result (ApprovalRecords: whether it landed).
export type Decide = (id: number, decision: ApprovalDecision) => Promise<unknown>;

// DecisionActions are a pending request's Allow and Deny buttons; with `shortcuts` they show the
// triage key each one answers to (APPROVAL_VERDICTS).
export function DecisionActions({
  row,
  busy,
  decide,
  shortcuts = false,
}: {
  row: InterceptApprovalRow;
  busy: boolean;
  decide: Decide;
  shortcuts?: boolean;
}) {
  if (row.status !== "pending") return null;
  const keyOf = (decision: ApprovalDecision) => APPROVAL_VERDICTS.find((v) => v.status === decision)?.key ?? "";
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        disabled={busy}
        aria-keyshortcuts={shortcuts ? keyOf("allowed") : undefined}
        onClick={() => void decide(row.id, "allowed")}
      >
        <CheckIcon data-icon="inline-start" />
        {t("Allow")}
        {shortcuts ? <Kbd>{keyOf("allowed").toUpperCase()}</Kbd> : null}
      </Button>
      <Button
        size="sm"
        variant="destructive"
        disabled={busy}
        aria-keyshortcuts={shortcuts ? keyOf("denied") : undefined}
        onClick={() => void decide(row.id, "denied")}
      >
        <XIcon data-icon="inline-start" />
        {t("Deny")}
        {shortcuts ? <Kbd>{keyOf("denied").toUpperCase()}</Kbd> : null}
      </Button>
    </div>
  );
}

// AlwaysAllowAction creates an argv allow rule from a pending Bash request.
// The prefix comes from the server; the request itself stays pending.
export function AlwaysAllowAction({ id, prefix }: { id: number; prefix?: string }) {
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
      toast.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
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
