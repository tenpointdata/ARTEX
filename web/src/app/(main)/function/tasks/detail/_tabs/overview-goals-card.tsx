"use client";

import * as React from "react";

import { CheckIcon, ListChecksIcon, PencilIcon, PlusIcon, Trash2Icon, XIcon } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { TaskGoal } from "@/lib/types";

export function GoalsCard({ taskId }: { taskId: string }) {
  const canOperate = useCan("operate");
  // Goal management: goal list + add form + inline edit state.
  const [goals, setGoals] = React.useState<TaskGoal[]>([]);
  const [goalText, setGoalText] = React.useState("");
  const [goalVuln, setGoalVuln] = React.useState("");
  const [goalBusy, setGoalBusy] = React.useState(false);
  const [goalErr, setGoalErr] = React.useState("");
  const [editingGoalId, setEditingGoalId] = React.useState<string | null>(null);
  const [editText, setEditText] = React.useState("");
  const [editVuln, setEditVuln] = React.useState("");
  const loadGoals = React.useCallback(async () => {
    try {
      const resp = await api.taskGoals(taskId);
      setGoals(resp.goals);
    } catch {
      // Ignore: transient error, the next poll retries
    }
  }, [taskId]);

  const addGoal = async () => {
    const text = goalText.trim();
    if (!text) return;
    setGoalBusy(true);
    setGoalErr("");
    try {
      await api.addGoal(taskId, text, goalVuln.trim() || undefined);
      setGoalText("");
      setGoalVuln("");
      await loadGoals();
    } catch (e) {
      setGoalErr(e instanceof Error ? e.message : t("Failed to add"));
    } finally {
      setGoalBusy(false);
    }
  };

  const startEditGoal = (g: TaskGoal) => {
    setEditingGoalId(g.id);
    setEditText(g.text);
    setEditVuln(g.vulnclass ?? "");
  };

  const cancelEditGoal = () => {
    setEditingGoalId(null);
    setEditText("");
    setEditVuln("");
  };

  const saveEditGoal = async (g: TaskGoal) => {
    const text = editText.trim();
    if (!text) return;
    setGoalBusy(true);
    setGoalErr("");
    try {
      await api.updateGoal(taskId, g.id, text, editVuln.trim() || undefined);
      cancelEditGoal();
      await loadGoals();
    } catch (e) {
      setGoalErr(e instanceof Error ? e.message : t("Failed to save"));
    } finally {
      setGoalBusy(false);
    }
  };

  const removeGoal = async (g: TaskGoal) => {
    setGoals((prev) => prev.filter((x) => x.id !== g.id));
    try {
      await api.deleteGoal(taskId, g.id);
    } catch {
      await loadGoals(); // delete failed: re-fetch to restore
    }
  };

  React.useEffect(() => {
    void loadGoals();
    return setVisibleInterval(() => {
      void loadGoals();
    }, 3000);
  }, [loadGoals]);

  return (
    <>
      {/* Goal management: view/add/edit/delete this task's exploration goals. Adds and edits notify the planner and revive the task;
          deletes only notify the planner (no revival). A goal = a final deliverable/verifiable outcome, not an attack step or recon action. */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ListChecksIcon className="size-4 text-primary" /> {t("Goal management")}
            <span className="text-muted-foreground text-xs font-normal">
              {t("(final verifiable goals, {count} in total; adds/edits notify the planner and revive the task)", {
                count: goals.length,
              })}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {/* Add form */}
          {canOperate && (
            <div className="flex flex-wrap items-center gap-2">
              <Input
                className="h-7 min-w-56 flex-1 text-sm"
                placeholder={t("Add a goal, e.g. 'gain unauthorized access with an admin account'")}
                value={goalText}
                onChange={(e) => setGoalText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void addGoal();
                }}
                disabled={goalBusy}
              />
              <Input
                className="h-7 w-32 text-sm"
                placeholder={t("Vulnerability class (optional)")}
                value={goalVuln}
                onChange={(e) => setGoalVuln(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void addGoal();
                }}
                disabled={goalBusy}
              />
              <Button
                size="sm"
                variant="outline"
                disabled={goalBusy || !goalText.trim()}
                onClick={() => void addGoal()}
              >
                <PlusIcon className="size-3.5" /> {t("Add")}
              </Button>
              {goalErr && <span className="text-xs text-red-500">{goalErr}</span>}
            </div>
          )}
          {/* Goal list */}
          {goals.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              {goals.map((g) =>
                editingGoalId === g.id ? (
                  <div key={g.id} className="flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5">
                    <Input
                      className="h-7 min-w-56 flex-1 text-sm"
                      value={editText}
                      onChange={(e) => setEditText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void saveEditGoal(g);
                        if (e.key === "Escape") cancelEditGoal();
                      }}
                      disabled={goalBusy}
                      autoFocus
                    />
                    <Input
                      className="h-7 w-32 text-sm"
                      placeholder={t("Vulnerability class (optional)")}
                      value={editVuln}
                      onChange={(e) => setEditVuln(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void saveEditGoal(g);
                        if (e.key === "Escape") cancelEditGoal();
                      }}
                      disabled={goalBusy}
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={goalBusy || !editText.trim()}
                      onClick={() => void saveEditGoal(g)}
                    >
                      <CheckIcon className="size-3.5 text-emerald-500" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={goalBusy}
                      onClick={cancelEditGoal}
                    >
                      <XIcon className="size-3.5" />
                    </Button>
                  </div>
                ) : (
                  <div key={g.id} className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                    <StatusBadge domain="goal" value={g.state} />
                    <span className="min-w-0 flex-1 break-words">{g.text}</span>
                    {g.vulnclass && (
                      <span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-xs">
                        {g.vulnclass}
                      </span>
                    )}
                    {canOperate && (
                      <>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-6 shrink-0 px-1.5"
                          disabled={goalBusy}
                          onClick={() => startEditGoal(g)}
                        >
                          <PencilIcon className="size-3.5" />
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-6 shrink-0 px-1.5"
                          disabled={goalBusy}
                          onClick={() => void removeGoal(g)}
                        >
                          <Trash2Icon className="size-3.5 text-red-500" />
                        </Button>
                      </>
                    )}
                  </div>
                ),
              )}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t(
                "No goals yet. Once added, the planner dispatches exploration intents from them and judges completion.",
              )}
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}
