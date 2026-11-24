"use client";

import * as React from "react";
import { ListChecksIcon } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Task } from "@/lib/types";
import { MethodologyPicker } from "../../methodology-picker";

/**
 * Task overview: change the methodology choice of an existing task. Each tick
 * is saved at once (PATCH /api/tasks/{id}); the planner reads the choice every
 * round, so the next round follows it.
 */
export function MethodologyChoiceCard({ task, onSaved }: { task: Task; onSaved: (task: Task) => void }) {
  const [value, setValue] = React.useState<string[]>(task.methodologies ?? []);
  const [saving, setSaving] = React.useState(false);
  const [error, setError] = React.useState("");
  const stored = (task.methodologies ?? []).join(",");

  // Follow the server's copy (polling, another tab) while no save is in flight.
  React.useEffect(() => {
    if (!saving) setValue(stored ? stored.split(",") : []);
  }, [stored, saving]);

  const save = async (ids: string[]) => {
    const previous = value;
    setValue(ids);
    setSaving(true);
    setError("");
    try {
      onSaved(await api.updateTask(task.id, { methodologies: ids }));
    } catch (e) {
      setValue(previous);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ListChecksIcon className="size-4 text-sky-500" /> {t("Testing methodologies")}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <MethodologyPicker value={value} onChange={save} disabled={saving} />
        {error && <p className="text-destructive text-xs">{error}</p>}
      </CardContent>
    </Card>
  );
}
