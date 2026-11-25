"use client";

import * as React from "react";

import { CheckIcon, PencilIcon, PlusIcon, ShieldAlertIcon, Trash2Icon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { TaskConstraint } from "@/lib/types";

export function ConstraintsCard({ taskId }: { taskId: string }) {
  // Constraint management: constraint list + add form + inline edit state.
  const [constraints, setConstraints] = React.useState<TaskConstraint[]>([]);
  const [conText, setConText] = React.useState("");
  const [conKind, setConKind] = React.useState<TaskConstraint["kind"]>("deny");
  const [conBusy, setConBusy] = React.useState(false);
  const [conErr, setConErr] = React.useState("");
  const [editingConId, setEditingConId] = React.useState<string | null>(null);
  const [editConText, setEditConText] = React.useState("");
  const [editConKind, setEditConKind] = React.useState<TaskConstraint["kind"]>("deny");
  const loadConstraints = React.useCallback(async () => {
    try {
      const resp = await api.taskConstraints(taskId);
      setConstraints(resp.constraints);
    } catch {
      // Ignore: transient error, the next poll retries
    }
  }, [taskId]);

  const addConstraint = async () => {
    const text = conText.trim();
    if (!text) return;
    setConBusy(true);
    setConErr("");
    try {
      await api.addConstraint(taskId, text, conKind);
      setConText("");
      await loadConstraints();
    } catch (e) {
      setConErr(e instanceof Error ? e.message : t("Failed to add"));
    } finally {
      setConBusy(false);
    }
  };

  const startEditConstraint = (c: TaskConstraint) => {
    setEditingConId(c.id);
    setEditConText(c.text);
    setEditConKind(c.kind);
  };

  const cancelEditConstraint = () => {
    setEditingConId(null);
    setEditConText("");
    setEditConKind("deny");
  };

  const saveEditConstraint = async (c: TaskConstraint) => {
    const text = editConText.trim();
    if (!text) return;
    setConBusy(true);
    setConErr("");
    try {
      await api.updateConstraint(taskId, c.id, text, editConKind);
      cancelEditConstraint();
      await loadConstraints();
    } catch (e) {
      setConErr(e instanceof Error ? e.message : t("Failed to save"));
    } finally {
      setConBusy(false);
    }
  };

  const removeConstraint = async (c: TaskConstraint) => {
    setConstraints((prev) => prev.filter((x) => x.id !== c.id));
    try {
      await api.deleteConstraint(taskId, c.id);
    } catch {
      await loadConstraints(); // delete failed: re-fetch to restore
    }
  };

  React.useEffect(() => {
    void loadConstraints();
    return setVisibleInterval(() => {
      void loadConstraints();
    }, 3000);
  }, [loadConstraints]);

  return (
    <>
      {/* Operation constraints: allow / deny. Constraints are injected into the planner/worker system prompt at the next
          planning round to bound exploration (injection can be toggled per planner/worker in system settings). Changes do not interrupt
          immediately; the next planning round picks them up naturally. */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldAlertIcon className="size-4 text-amber-500" /> {t("Operation constraints")}
            <span className="text-muted-foreground text-xs font-normal">
              {t(
                "(bounds planner/worker exploration, {count} in total; changes take effect at the next planning round)",
                { count: constraints.length },
              )}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {/* Add form */}
          <div className="flex flex-wrap items-center gap-2">
            <NativeSelect
              size="sm"
              value={conKind}
              onChange={(e) => setConKind(e.target.value as TaskConstraint["kind"])}
            >
              <NativeSelectOption value="deny">{t("Deny")}</NativeSelectOption>
              <NativeSelectOption value="allow">{t("Allow")}</NativeSelectOption>
            </NativeSelect>
            <Input
              className="h-7 min-w-56 flex-1 text-sm"
              placeholder={t("One operation constraint, e.g. 'test only the current port, do not scan other ports'")}
              value={conText}
              onChange={(e) => setConText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void addConstraint();
              }}
              disabled={conBusy}
            />
            <Button
              size="sm"
              variant="outline"
              disabled={conBusy || !conText.trim()}
              onClick={() => void addConstraint()}
            >
              <PlusIcon className="size-3.5" /> {t("Add")}
            </Button>
            {conErr && <span className="text-xs text-red-500">{conErr}</span>}
          </div>
          {/* Constraint list */}
          {constraints.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              {constraints.map((c) =>
                editingConId === c.id ? (
                  <div key={c.id} className="flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5">
                    <NativeSelect
                      size="sm"
                      value={editConKind}
                      onChange={(e) => setEditConKind(e.target.value as TaskConstraint["kind"])}
                    >
                      <NativeSelectOption value="deny">{t("Deny")}</NativeSelectOption>
                      <NativeSelectOption value="allow">{t("Allow")}</NativeSelectOption>
                    </NativeSelect>
                    <Input
                      className="h-7 min-w-56 flex-1 text-sm"
                      value={editConText}
                      onChange={(e) => setEditConText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void saveEditConstraint(c);
                        if (e.key === "Escape") cancelEditConstraint();
                      }}
                      disabled={conBusy}
                      autoFocus
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={conBusy || !editConText.trim()}
                      onClick={() => void saveEditConstraint(c)}
                    >
                      <CheckIcon className="size-3.5 text-emerald-500" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={conBusy}
                      onClick={cancelEditConstraint}
                    >
                      <XIcon className="size-3.5" />
                    </Button>
                  </div>
                ) : (
                  <div key={c.id} className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                    <span
                      className={`shrink-0 rounded px-1.5 py-0.5 text-xs ${
                        c.kind === "allow"
                          ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                          : "bg-red-500/15 text-red-600 dark:text-red-400"
                      }`}
                    >
                      {c.kind === "allow" ? t("Allow") : t("Deny")}
                    </span>
                    <span className="min-w-0 flex-1 break-words">{c.text}</span>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={conBusy}
                      onClick={() => startEditConstraint(c)}
                    >
                      <PencilIcon className="size-3.5" />
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 shrink-0 px-1.5"
                      disabled={conBusy}
                      onClick={() => void removeConstraint(c)}
                    >
                      <Trash2Icon className="size-3.5 text-red-500" />
                    </Button>
                  </div>
                ),
              )}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t(
                "No operation constraints yet. They are extracted automatically from the description/goal at task creation; you can also add, edit, or delete them here to bound which operations are allowed/denied.",
              )}
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}
