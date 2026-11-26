"use client";

import * as React from "react";

import {
  ActivityIcon,
  AlertTriangleIcon,
  BugIcon,
  ClockIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  TargetIcon,
} from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { useCan } from "@/hooks/use-session";
import { intlLocale, t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { Finding, Stats, Task, TaskCoverage, TaskNode } from "@/lib/types";

import { ConstraintsCard } from "./overview-constraints-card";
import { CoverageCard } from "./overview-coverage-card";
import { GoalsCard } from "./overview-goals-card";
import { TaskInterceptRulesCard } from "./overview-intercept-rules-card";
import { ScopeCard } from "./overview-scope-card";
import { TokensCard } from "./overview-tokens-card";

function StatCard({
  label,
  value,
  sub,
  icon: Icon,
}: {
  label: string;
  value: React.ReactNode;
  sub?: string;
  icon: React.ElementType;
}) {
  return (
    <Card className="gap-1.5">
      <CardHeader className="pb-0">
        <CardDescription className="flex items-center gap-1.5">
          <Icon className="size-3.5" /> {label}
        </CardDescription>
        <CardTitle className="text-2xl tabular-nums">{value}</CardTitle>
      </CardHeader>
      {sub && <CardContent className="text-xs text-muted-foreground">{sub}</CardContent>}
    </Card>
  );
}

export function OverviewTab({ taskId }: { taskId: string }) {
  const canOperate = useCan("operate"); // re-runs are operate-level, and so is the per-model token ledger
  const [task, setTask] = React.useState<Task | null>(null);
  const [stats, setStats] = React.useState<Stats | null>(null);
  const [intents, setIntents] = React.useState<TaskNode[]>([]);
  const [findings, setFindings] = React.useState<Finding[]>([]);
  const [coverage, setCoverage] = React.useState<TaskCoverage | null>(null);
  // Intent ids currently re-running ("__all__" means the batch); used to disable buttons + spinners.
  const [rerunning, setRerunning] = React.useState<Set<string>>(new Set());

  const markRerun = (key: string, on: boolean) =>
    setRerunning((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });

  // Re-run one: set back to open (optimistic local state update, 3s poll as backstop); a worker re-claims it and runs from scratch.
  const rerunOne = async (id: string) => {
    markRerun(id, true);
    try {
      await api.rerunIntent(taskId, id);
      setIntents((prev) => prev.map((i) => (i.id === id ? { ...i, state: "open" } : i)));
    } catch {
      // Ignore failures: the next poll still shows blocked and the user can click again
    } finally {
      markRerun(id, false);
    }
  };

  // Re-run all of this task's blocked intents in bulk.
  const rerunAll = async () => {
    markRerun("__all__", true);
    try {
      await api.rerunBlocked(taskId);
      setIntents((prev) => prev.map((i) => (i.state === "blocked" ? { ...i, state: "open" } : i)));
    } catch {
      // ignore
    } finally {
      markRerun("__all__", false);
    }
  };

  React.useEffect(() => {
    let cancelled = false;
    let loading = false;

    const load = async () => {
      if (loading) return;
      loading = true;
      try {
        const [taskResp, statsResp, intentsResp, findingsResp] = await Promise.all([
          api.task(taskId),
          api.stats(taskId),
          api.intents(taskId),
          api.findings(taskId),
        ]);
        if (cancelled) return;
        const activeTask = statsResp.active_task;
        setTask(
          activeTask
            ? {
                ...taskResp,
                in_flight: activeTask.in_flight,
                goals_total: activeTask.goals_total,
                goals_met: activeTask.goals_met,
                engine_mode: statsResp.engine_mode ?? activeTask.engine_mode,
                paused: activeTask.paused,
              }
            : taskResp,
        );
        setStats(statsResp);
        setIntents(intentsResp);
        setFindings(findingsResp);
        // coverage is independent + may 503 when no asset store — fetch separately so
        // its failure never blocks the others.
        api
          .taskCoverage(taskId)
          .then((c) => {
            if (!cancelled) setCoverage(c);
          })
          .catch(() => {
            // A transient coverage failure is retried by the next poll.
          });
      } catch {
        // transient errors are ignored; the next poll will retry
      } finally {
        loading = false;
      }
    };

    void load();
    const stopPolling = setVisibleInterval(() => {
      void load();
    }, 3000);
    return () => {
      cancelled = true;
      stopPolling();
    };
  }, [taskId]);

  const running = intents.filter((i) => i.state === "running");
  const open = intents.filter((i) => i.state === "open");
  const blocked = intents.filter((i) => i.state === "blocked");
  const taskFindings = findings.filter((f) => f.task_id === taskId);
  const goalsPct = task?.goals_total ? Math.round(((task.goals_met ?? 0) / task.goals_total) * 100) : 0;

  return (
    <div className="flex flex-col gap-4">
      {/* Original task description and goal (as filled in at creation), pinned at the top for reference. */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <TargetIcon className="size-4 text-primary" /> {t("Task description & goal")}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <div className="text-xs font-medium text-muted-foreground">{t("Description")}</div>
            <p className="text-sm whitespace-pre-wrap break-words">{task?.description?.trim() || "—"}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <div className="text-xs font-medium text-muted-foreground">{t("Goal")}</div>
            <p className="text-sm whitespace-pre-wrap break-words">{task?.goal?.trim() || "—"}</p>
          </div>
        </CardContent>
      </Card>
      <GoalsCard taskId={taskId} />
      <ConstraintsCard taskId={taskId} />
      <TaskInterceptRulesCard taskId={taskId} />
      <CoverageCard coverage={coverage} />
      {canOperate && <TokensCard taskId={taskId} />}
      <ScopeCard taskId={taskId} />
      {/* Heartbeat */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ActivityIcon className="size-4 text-blue-500" /> {t("Heartbeat")}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <div className="text-xs text-muted-foreground">{t("Engine state")}</div>
            <StatusBadge
              domain="engine"
              value={stats?.engine_mode ?? task?.engine_mode ?? "idle"}
              dot
              className="mt-1"
            />
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("Running workers")}</div>
            <div className="mt-1 text-lg font-semibold tabular-nums">{running.length}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("Last activity")}</div>
            <div className="mt-1 inline-flex items-center gap-1 text-sm">
              <ClockIcon className="size-3.5" />
              {task?.last_activity ? new Date(task.last_activity).toLocaleTimeString(intlLocale) : "—"}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">
              {t("Goals {met}/{total}", { met: task?.goals_met ?? 0, total: task?.goals_total ?? 0 })}
            </div>
            <Progress value={goalsPct} className="mt-2" />
          </div>
          {task?.completed_unix && task.completed_unix > 0 ? (
            <div>
              <div className="text-xs text-muted-foreground">{t("Completed at")}</div>
              <div className="mt-1 inline-flex items-center gap-1 text-sm">
                <ClockIcon className="size-3.5" />
                {new Date(task.completed_unix * 1000).toLocaleString(intlLocale)}
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {/* Work set */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <TargetIcon className="size-4" /> {t("Running intents")}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {running.slice(0, 6).map((i) => (
              <div key={i.id} className="flex items-center gap-2 text-sm">
                <StatusBadge domain="intent" value={i.state} />
                <span className="min-w-0 flex-1 truncate">{i.payload}</span>
              </div>
            ))}
            {running.length === 0 && <p className="text-sm text-muted-foreground">{t("No running intents")}</p>}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <AlertTriangleIcon className="size-4 text-amber-500" /> {t("Needs attention")}
            </CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <div className="text-2xl font-semibold tabular-nums text-red-600">{taskFindings.length}</div>
              <div className="text-xs text-muted-foreground">{t("Confirmed vulnerabilities")}</div>
            </div>
            <div>
              <div className="text-2xl font-semibold tabular-nums text-blue-600">{running.length}</div>
              <div className="text-xs text-muted-foreground">{t("Running")}</div>
            </div>
            <div>
              <div className="text-2xl font-semibold tabular-nums">{open.length}</div>
              <div className="text-xs text-muted-foreground">{t("frontier open")}</div>
            </div>
            <div>
              <div className="text-2xl font-semibold tabular-nums text-red-600">{blocked.length}</div>
              <div className="text-xs text-muted-foreground">{t("Blocked intents")}</div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <BugIcon className="size-4 text-red-500" /> {t("Recent findings")}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {taskFindings.slice(0, 6).map((f) => (
              <div key={f.id} className="flex items-center gap-2 text-sm">
                <StatusBadge domain="severity" value={f.severity} dot />
                <span className="min-w-0 flex-1 truncate">{f.summary}</span>
              </div>
            ))}
            {taskFindings.length === 0 && <p className="text-sm text-muted-foreground">{t("No findings yet")}</p>}
          </CardContent>
        </Card>
      </div>

      {/* Blocked intents — errored/blocked intents (e.g. LLM network issues) that can be re-run in one click: set back to open,
          a worker re-claims and re-runs from scratch (data already written to the graph is kept); a terminal/paused task is revived automatically. */}
      {blocked.length > 0 && (
        <Card className="border-red-500/30">
          <CardHeader className="flex-row items-center justify-between gap-2 space-y-0">
            <CardTitle className="flex items-center gap-2 text-sm">
              <AlertTriangleIcon className="size-4 text-red-500" /> {t("Blocked/errored intents")}
              <span className="text-xs font-normal text-muted-foreground">
                {t("({count} in total, re-runnable)", { count: blocked.length })}
              </span>
            </CardTitle>
            {canOperate && (
              <Button size="sm" variant="outline" disabled={rerunning.has("__all__")} onClick={() => void rerunAll()}>
                <RefreshCwIcon className={`size-3.5 ${rerunning.has("__all__") ? "animate-spin" : ""}`} />
                {t("Re-run all")}
              </Button>
            )}
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {blocked.slice(0, 20).map((i) => (
              <div key={i.id} className="flex items-center gap-2 text-sm">
                <StatusBadge domain="intent" value={i.state} />
                <span className="min-w-0 flex-1 truncate">{i.payload}</span>
                {canOperate && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-7 shrink-0 px-2 text-xs"
                    disabled={rerunning.has(i.id)}
                    onClick={() => void rerunOne(i.id)}
                  >
                    <RefreshCwIcon className={`size-3 ${rerunning.has(i.id) ? "animate-spin" : ""}`} />
                    {t("Re-run")}
                  </Button>
                )}
              </div>
            ))}
            {blocked.length > 20 && (
              <p className="text-xs text-muted-foreground">
                {t('Only the first 20 are shown; click "Re-run all" to handle the remaining {count}.', {
                  count: blocked.length - 20,
                })}
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {/* Stat cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <StatCard label={t("Open intents")} value={open.length} icon={ShieldCheckIcon} sub={t("frontier open")} />
        <StatCard label={t("Confirmed findings")} value={taskFindings.length} icon={BugIcon} sub={t("this task")} />
        <StatCard
          label={t("Total intents")}
          value={intents.length}
          icon={AlertTriangleIcon}
          sub={t("all intents of this task")}
        />
      </div>
    </div>
  );
}
