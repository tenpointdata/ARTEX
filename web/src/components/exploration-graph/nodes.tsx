"use client";
"use no memo";

import * as React from "react";

import { Handle, type NodeProps, Position } from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { intlLocale, t } from "@/i18n";
import { toneClasses } from "@/lib/status";
import type { TaskNode } from "@/lib/types";
import { cn } from "@/lib/utils";

import { digestMemberIds, type ExploreRFNode, nodeSummary, prioTone, typeMeta, viewKind } from "./model";
export function ExploreNode({ data, selected }: NodeProps<ExploreRFNode>) {
  const n = data.node;
  const kind = viewKind(n);
  const meta = typeMeta[kind] ?? typeMeta.intent;
  const Icon = meta.icon;
  const showState = n.type === "goal" || n.type === "intent";
  const showPriority = (n.type === "goal" || n.type === "intent") && n.priority > 0;
  const isLive = !n.inherited && n.type === "intent" && n.state === "running";

  return (
    <div
      className={cn(
        "w-[240px] cursor-pointer rounded-2xl border-2 transition-colors",
        selected ? "border-blue-500" : isLive ? "border-blue-400/70" : "border-transparent",
      )}
    >
      <div className="group relative rounded-[14px] border border-black/[0.06] bg-white shadow-sm transition-shadow hover:shadow-lg dark:border-white/10 dark:bg-neutral-900">
        <Handle
          type="target"
          position={Position.Left}
          className="!size-2 !border-2 !border-neutral-300 !bg-white opacity-0 transition-opacity group-hover:opacity-100 dark:!border-neutral-600 dark:!bg-neutral-800"
        />
        <div className="flex items-center gap-2 px-3 pt-3 pb-1.5">
          <span className={cn("flex size-6 shrink-0 items-center justify-center rounded-lg shadow-sm", meta.iconBg)}>
            <Icon className="size-3.5 text-white" />
          </span>
          <span className="grow truncate text-[13px] font-semibold text-neutral-700 dark:text-neutral-100">
            {meta.label}
          </span>
          {isLive && (
            <span className="relative flex size-2 shrink-0">
              <span className="absolute inline-flex size-full animate-ping rounded-full bg-blue-400 opacity-70" />
              <span className="relative inline-flex size-2 rounded-full bg-blue-500" />
            </span>
          )}
          {showPriority && (
            <span
              className={cn(
                "shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold tabular-nums",
                toneClasses[prioTone(n.priority)],
              )}
            >
              P{n.priority}
            </span>
          )}
        </div>
        <div className="line-clamp-3 px-3 pb-2 text-[13px] leading-snug text-neutral-500 dark:text-neutral-400">
          {nodeSummary(n) || meta.label}
        </div>
        <div className="flex items-center justify-between gap-2 px-3 pb-2.5">
          {n.inherited && n.source_task_id ? (
            <Badge
              variant="outline"
              className="max-w-32 truncate"
              title={t("Inherited from task #{id}, read-only", { id: n.source_task_id })}
            >
              {t("Source #{id}", { id: n.source_task_id })}
            </Badge>
          ) : showState ? (
            <StatusBadge
              domain={n.type === "goal" ? "goal" : "intent"}
              value={n.state}
              dot
              className="px-1.5 py-0 text-[10px]"
            />
          ) : (
            <span className="text-[10px] text-neutral-400 dark:text-neutral-500">{meta.label}</span>
          )}
          <span className="shrink-0 font-mono text-[10px] text-neutral-400 dark:text-neutral-500">#{n.id}</span>
        </div>
        <Handle
          type="source"
          position={Position.Right}
          className="!size-2 !border-2 !border-neutral-300 !bg-white opacity-0 transition-opacity group-hover:opacity-100 dark:!border-neutral-600 dark:!bg-neutral-800"
        />
      </div>
    </div>
  );
}

export const nodeTypes = { explore: ExploreNode };

export function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3 py-1.5 text-sm">
      <span className="text-muted-foreground w-14 shrink-0">{label}</span>
      <span className="text-foreground min-w-0 flex-1 break-words">{children}</span>
    </div>
  );
}

export function NodeDetailSheet({
  node,
  nodesById,
  onSelect,
  onOpenChange,
}: {
  node: TaskNode | null;
  nodesById: Map<string, TaskNode>;
  onSelect: (n: TaskNode) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const [copied, setCopied] = React.useState(false);
  const meta = node ? (typeMeta[viewKind(node)] ?? typeMeta.intent) : null;
  const Icon = meta?.icon;
  const raw = node ? JSON.stringify(node, null, 2) : "";
  const memberIds = node ? digestMemberIds(node) : [];
  const members = memberIds.map((id) => nodesById.get(id)).filter((m): m is TaskNode => !!m);

  const copy = () => {
    void navigator.clipboard?.writeText(raw).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <Sheet open={node !== null} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col gap-0 p-0 data-[side=right]:sm:max-w-md">
        {node && meta && Icon && (
          <>
            <SheetHeader className="border-b p-4">
              <div className="flex items-center gap-2.5 pr-8">
                <span
                  className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg shadow-sm", meta.iconBg)}
                >
                  <Icon className="size-4 text-white" />
                </span>
                <div className="min-w-0">
                  <SheetTitle className="leading-tight">{meta.label}</SheetTitle>
                  <span className="text-muted-foreground font-mono text-xs">{node.id}</span>
                </div>
                {node.inherited && node.source_task_id && (
                  <Badge variant="outline" className="ml-auto shrink-0">
                    {t("Source task #{id} · read-only", { id: node.source_task_id })}
                  </Badge>
                )}
              </div>
            </SheetHeader>

            <ScrollArea className="min-h-0 flex-1">
              <div className="flex w-full min-w-0 flex-col gap-4 p-4">
                <section>
                  <h4 className="text-muted-foreground mb-1 text-xs font-medium">{t("Properties")}</h4>
                  <DetailRow label={t("Type")}>{meta.label}</DetailRow>
                  <DetailRow label={t("Status")}>
                    {node.type === "goal" || node.type === "intent" ? (
                      <StatusBadge
                        domain={node.type === "goal" ? "goal" : "intent"}
                        value={node.state}
                        dot
                        className="px-1.5 py-0 text-[10px]"
                      />
                    ) : (
                      <span className="font-mono text-xs">{node.state}</span>
                    )}
                  </DetailRow>
                  <DetailRow label={t("Priority")}>
                    <span className="tabular-nums">{node.priority}</span>
                  </DetailRow>
                  <DetailRow label={t("Source")}>
                    <span className="font-mono text-xs">{node.origin}</span>
                  </DetailRow>
                  {node.inherited && node.source_task_id && (
                    <DetailRow label={t("Source task")}>
                      <span className="font-mono text-xs">
                        {t("#{id} (inherited, read-only)", { id: node.source_task_id })}
                      </span>
                    </DetailRow>
                  )}
                  <DetailRow label={t("Time")}>{new Date(node.ts).toLocaleString(intlLocale)}</DetailRow>
                </section>

                {node.type === "digest" && (
                  <>
                    {nodeSummary(node) && (
                      <section className="border-t pt-3">
                        <h4 className="text-muted-foreground mb-1.5 text-xs font-medium">{t("Digest conclusions")}</h4>
                        <p className="text-foreground text-sm leading-relaxed break-words whitespace-pre-wrap">
                          {nodeSummary(node)}
                        </p>
                      </section>
                    )}
                    <section className="border-t pt-3">
                      <h4 className="text-muted-foreground mb-1.5 text-xs font-medium">
                        {t("Compressed nodes ({count})", { count: memberIds.length })}
                      </h4>
                      <div className="flex flex-col gap-1.5">
                        {members.map((m) => {
                          const mMeta = typeMeta[viewKind(m)] ?? typeMeta.intent;
                          const MIcon = mMeta.icon;
                          return (
                            <button
                              key={m.id}
                              type="button"
                              onClick={() => onSelect(m)}
                              className="hover:bg-accent flex items-start gap-2 rounded-md border p-2 text-left transition-colors"
                            >
                              <span
                                className={cn(
                                  "mt-0.5 flex size-5 shrink-0 items-center justify-center rounded",
                                  mMeta.iconBg,
                                )}
                              >
                                <MIcon className="size-3 text-white" />
                              </span>
                              <span className="min-w-0 flex-1">
                                <span className="text-foreground line-clamp-2 text-xs leading-snug">
                                  {nodeSummary(m) || mMeta.label}
                                </span>
                                <span className="text-muted-foreground block font-mono text-[10px]">
                                  {mMeta.label} · #{m.id}
                                </span>
                              </span>
                            </button>
                          );
                        })}
                        {members.length < memberIds.length && (
                          <span className="text-muted-foreground text-[10px]">
                            {t("{count} more member nodes not loaded with the current graph", {
                              count: memberIds.length - members.length,
                            })}
                          </span>
                        )}
                      </div>
                    </section>
                  </>
                )}

                <section className="border-t pt-3">
                  <div className="mb-1.5 flex items-center justify-between">
                    <h4 className="text-muted-foreground text-xs font-medium">{t("Raw data")}</h4>
                    <button
                      type="button"
                      onClick={copy}
                      className="text-muted-foreground hover:text-foreground text-xs transition-colors"
                    >
                      {copied ? t("Copied") : t("Copy")}
                    </button>
                  </div>
                  <pre className="bg-muted/50 text-foreground max-w-full overflow-hidden rounded-md border p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap">
                    {raw}
                  </pre>
                </section>
              </div>
            </ScrollArea>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}

// ExplorationGraphInner renders the given nodes/edges as a React Flow canvas.
// Positions are auto-laid-out and preserved across prop updates (so a polling
// caller's graph doesn't jump); click a node to open its detail drawer.
