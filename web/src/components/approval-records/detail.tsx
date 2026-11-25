"use client";

import * as React from "react";

import { ChevronDownIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Skeleton } from "@/components/ui/skeleton";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { InterceptApprovalRow, InterceptDetail } from "@/lib/types";
import { cn } from "@/lib/utils";

import {
  AlwaysAllowAction,
  ApprovalOrigin,
  actionLabels,
  CodeBlock,
  contextLabels,
  type Decide,
  DecidedByBadge,
  DecisionActions,
  executionLabels,
  fmtTime,
  MatchCell,
  ModelReviewContext,
  RiskBadge,
  StatusBadge,
  source,
} from "./shared";
export function ApprovalDetail({
  row,
  busy,
  decide,
  revision,
  readOnly = false,
  defaultExpanded = false,
  onResolved,
}: {
  row: InterceptApprovalRow;
  busy: boolean;
  decide: Decide;
  revision: number;
  readOnly?: boolean;
  defaultExpanded?: boolean;
  onResolved?: (status: "allowed" | "denied" | "timeout") => void;
}) {
  const [detail, setDetail] = React.useState<InterceptDetail | null>(null);
  const [error, setError] = React.useState("");
  const [retry, setRetry] = React.useState(0);
  const [more, setMore] = React.useState(defaultExpanded);

  // biome-ignore lint/correctness/useExhaustiveDependencies: Status, refresh and retry invalidate details without closing the panel.
  React.useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const next = await api.interceptDetail(row.id);
        if (cancelled) return;
        setDetail(next);
        setError("");
        if (next.status !== "pending") onResolved?.(next.status);
        if (next.status === "pending" || next.audit?.execution_status === "awaiting_result")
          timer = setTimeout(() => void load(), 5000);
      } catch (e) {
        if (!cancelled) setError((e as Error).message || t("Failed to load details"));
      }
    }
    void load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [row.id, row.status, revision, retry, onResolved]);

  // Row updates are authoritative until the lazy detail has caught up.
  const current = detail?.status === row.status ? detail : row;
  const audit = detail?.audit;
  let execution = audit ? executionLabels[audit.execution_status] : t("Not recorded");
  if (row.status === "pending") execution = t("Not started");
  if (audit && audit.correlation !== "exact" && audit.effective_action === "allow")
    execution = t("No linked execution result");
  const command = typeof row.tool_input?.command === "string" ? row.tool_input.command : undefined;
  let initialLabel = source(row) === "model" ? t("Model initial decision") : t("Rule initial decision");
  if (audit?.model_fallback) initialLabel = t("Model error fallback");

  return (
    <div className="flex min-w-0 flex-col gap-4 p-3 sm:p-5">
      <div className="grid min-w-0 gap-5 rounded-xl border bg-muted/20 p-4 lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-3">
          <CodeBlock
            label={t("{agent} · tool request", { agent: current.agent_name || current.conv_agent_key || t("Agent") })}
            text={JSON.stringify(row.tool_input ?? {}, null, 2)}
          />
          {command ? (
            <Collapsible>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" size="sm">
                  <ChevronDownIcon data-icon="inline-start" />
                  {t("View command content")}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="pt-2">
                <CodeBlock label={t("Command content")} text={command} />
              </CollapsibleContent>
            </Collapsible>
          ) : null}
          <p className="text-muted-foreground text-xs">
            {t("Execution result:")} <span className="text-foreground">{detail ? execution : t("Loading…")}</span>
          </p>
        </div>
        <div className="flex min-w-0 flex-col gap-4 lg:border-l lg:pl-5">
          <h3 className="font-medium text-muted-foreground text-xs">
            {current.status === "pending" ? t("Review status") : t("Approval decision")}
          </h3>
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={current.status} />
            <MatchCell row={current} showReason={false} />
          </div>
          <p className="whitespace-pre-wrap break-words text-sm leading-7 [overflow-wrap:anywhere]">
            {current.reason?.replace(/^\[model\]\s*/, "") || t("No approval reason recorded")}
          </p>
          {audit?.decision_reason ? <p className="text-sm">{audit.decision_reason}</p> : null}
          {audit?.effective_action ? (
            <p className="text-sm">{t("Final action: {action}", { action: actionLabels[audit.effective_action] })}</p>
          ) : null}
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-xs">
            <dt className="text-muted-foreground">{t("Source")}</dt>
            <dd className="break-words">
              <ApprovalOrigin row={current} detail />
            </dd>
            <dt className="text-muted-foreground">{t("Requested at")}</dt>
            <dd>{fmtTime(row.created_at)}</dd>
            <dt className="text-muted-foreground">{t("Decided at")}</dt>
            <dd>{fmtTime(current.decided_at)}</dd>
            <dt className="text-muted-foreground">{t("Decided by")}</dt>
            <dd>
              <DecidedByBadge row={current} />
            </dd>
            {row.risk ? (
              <>
                <dt className="text-muted-foreground">{t("Risk")}</dt>
                <dd className="flex flex-col gap-1">
                  <span className="flex items-center gap-2">
                    <RiskBadge risk={row.risk} />
                    <span className="text-muted-foreground">{row.risk.score}/100</span>
                  </span>
                  {row.risk.reasons.length ? (
                    <ul className="list-disc pl-4">
                      {row.risk.reasons.map((reason) => (
                        <li key={reason}>{reason}</li>
                      ))}
                    </ul>
                  ) : null}
                </dd>
              </>
            ) : null}
            {audit?.rule_name ? (
              <>
                <dt className="text-muted-foreground">{t("Rule snapshot")}</dt>
                <dd>{audit.rule_name}</dd>
              </>
            ) : null}
            {audit?.profile_id ? (
              <>
                <dt className="text-muted-foreground">{t("Approval model profile")}</dt>
                <dd>#{audit.profile_id}</dd>
              </>
            ) : null}
          </dl>
          {!readOnly ? <DecisionActions row={current} busy={busy} decide={decide} /> : null}
          {!readOnly && current.status === "pending" && row.tool_name === "Bash" ? (
            <AlwaysAllowAction id={row.id} prefix={detail?.always_allow_prefix} />
          ) : null}
        </div>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>
            <div className="flex flex-wrap items-center gap-2">
              <span>{t("Failed to load details: {error}", { error })}</span>
              <Button variant="outline" size="sm" onClick={() => setRetry((v) => v + 1)}>
                {t("Retry")}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : null}
      {!detail && !error ? <Skeleton className="h-8 w-60" /> : null}
      {detail && !audit ? (
        <Alert>
          <AlertDescription>
            {t(
              "This record has no saved approval detail snapshot, so the context, model initial decision, and execution output at the time cannot be restored.",
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      {audit ? (
        <Collapsible open={more} onOpenChange={setMore}>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm">
              <ChevronDownIcon data-icon="inline-start" className={cn(more && "rotate-180")} />
              {more ? t("Collapse context") : t("View context and execution result")}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="pt-4">
            <div className="flex min-w-0 flex-col gap-5">
              {audit.model_input ? (
                <ModelReviewContext input={audit.model_input} />
              ) : (
                <Alert>
                  <AlertDescription>
                    {audit.model_input_digest
                      ? t(
                          "This legacy record saved only the review input fingerprint, not the input itself, so the context sent to the model at the time cannot be restored. This does not mean the review had no context; newly produced model decisions keep an input snapshot.",
                        )
                      : t(
                          "This record has no saved model review input; it may have been decided directly by a rule, failed before the model call, or come from an old version.",
                        )}
                  </AlertDescription>
                </Alert>
              )}
              {audit.user_message || audit.context?.length ? (
                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="ghost" size="sm">
                      <ChevronDownIcon data-icon="inline-start" />
                      {audit.model_input && audit.model_input.version >= 3
                        ? t("View conversation audit excerpt (not sent to model)")
                        : t("View conversation audit excerpt")}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="pt-3">
                    <div className="flex min-w-0 flex-col gap-3">
                      {audit.user_message ? (
                        <CodeBlock
                          label={t("Conversation current-turn input (audit excerpt)")}
                          text={audit.user_message}
                          truncated={audit.user_truncated}
                        />
                      ) : null}
                      <section className="flex min-w-0 flex-col gap-3">
                        <h3 className="font-medium text-muted-foreground text-xs">
                          {t("Visible conversation context")}
                        </h3>
                        <p className="text-muted-foreground text-xs">
                          {t(
                            'Conversation record excerpt saved at {time}. What the model actually used follows "Model review input".',
                            {
                              time: fmtTime(audit.captured_at),
                            },
                          )}
                        </p>
                        {audit.context_truncated ? (
                          <p className="text-muted-foreground text-xs">
                            {t("Only the most recent context is saved; some content is truncated.")}
                          </p>
                        ) : null}
                        {audit.context?.length ? (
                          audit.context.map((entry, index) => (
                            <CodeBlock
                              key={`${entry.kind}-${entry.tool_use_id || index}`}
                              label={`${contextLabels[entry.kind] ?? entry.kind}${entry.tool ? ` · ${entry.tool}` : ""}${entry.is_error ? ` · ${t("error")}` : ""}`}
                              text={entry.text}
                              truncated={entry.truncated}
                            />
                          ))
                        ) : (
                          <p className="text-muted-foreground text-sm">{t("No linkable context recorded")}</p>
                        )}
                      </section>
                    </div>
                  </CollapsibleContent>
                </Collapsible>
              ) : null}
              <CodeBlock
                label={`${initialLabel}: ${actionLabels[audit.initial_action] ?? audit.initial_action}`}
                text={audit.initial_reason.replace(/^\[model\]\s*/, "")}
              />
              {audit.judge_answers?.length ? (
                <CodeBlock
                  label={t("Judge answers")}
                  text={audit.judge_answers
                    .map((a) => `${a.id}: ${a.answer}${a.evidence ? ` - ${a.evidence}` : ""}`)
                    .join("\n")}
                />
              ) : null}
              <CodeBlock
                label={t("Execution output")}
                text={
                  audit.output ??
                  (audit.execution_status === "not_executed" ? t("Tool not executed.") : t("No execution output yet"))
                }
                truncated={audit.output_truncated}
              />
              {audit.correlation !== "exact" ? (
                <Alert>
                  <AlertDescription>
                    {audit.correlation === "ambiguous"
                      ? t(
                          "Concurrent calls with identical arguments exist, so the tool call cannot be uniquely linked; this record does not show a guessed execution result.",
                        )
                      : t("No uniquely linkable tool call ID recorded.")}
                  </AlertDescription>
                </Alert>
              ) : null}
              <dl className="grid gap-2 text-muted-foreground text-xs [overflow-wrap:anywhere]">
                <div>{t("Tool call ID: {value}", { value: audit.tool_use_id || t("Not recorded") })}</div>
                <div>{t("Arguments digest SHA-256: {value}", { value: audit.input_digest })}</div>
                <div>
                  {t("Review config fingerprint SHA-256: {value}", { value: audit.config_digest || t("Not recorded") })}
                </div>
                {audit.model_input_digest ? (
                  <div>{t("Model review input SHA-256: {value}", { value: audit.model_input_digest })}</div>
                ) : null}
                {audit.execution_ended_at ? (
                  <div>{t("Result recorded at: {time}", { time: fmtTime(audit.execution_ended_at) })}</div>
                ) : null}
              </dl>
            </div>
          </CollapsibleContent>
        </Collapsible>
      ) : null}
    </div>
  );
}

// Hidden cells do not occupy table columns. Keep the detail span in sync so
