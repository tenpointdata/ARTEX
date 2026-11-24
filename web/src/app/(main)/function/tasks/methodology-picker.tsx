"use client";

import * as React from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { MethodologySummary } from "@/lib/types";

/**
 * Task create form: which testing methodologies the task is checked against.
 * Nothing ticked = automatic (the server derives them from the goal and
 * description); ticking any replaces the automatic choice with exactly those.
 */
export function MethodologyPicker({ value, onChange }: { value: string[]; onChange: (ids: string[]) => void }) {
  const [options, setOptions] = React.useState<MethodologySummary[]>([]);
  const [failed, setFailed] = React.useState(false);

  React.useEffect(() => {
    let live = true;
    api
      .methodologies()
      .then((ms) => live && setOptions(ms))
      .catch(() => live && setFailed(true));
    return () => {
      live = false;
    };
  }, []);

  const toggle = (id: string, on: boolean) =>
    onChange(on ? [...value.filter((v) => v !== id), id] : value.filter((v) => v !== id));

  return (
    <div className="grid gap-2">
      <span className="text-sm">{t("Testing methodologies")}</span>
      {failed ? (
        <p className="text-destructive text-xs">{t("Failed to load methodologies")}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8" />
              <TableHead>{t("Methodology")}</TableHead>
              <TableHead className="text-right">{t("Items")}</TableHead>
              <TableHead>{t("Chosen automatically for")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {options.map((m) => (
              <TableRow key={m.id}>
                <TableCell>
                  <Checkbox
                    id={`methodology-${m.id}`}
                    checked={value.includes(m.id)}
                    onCheckedChange={(v) => toggle(m.id, !!v)}
                  />
                </TableCell>
                <TableCell>
                  <label htmlFor={`methodology-${m.id}`} className="cursor-pointer">
                    {m.name} <span className="text-muted-foreground">{m.version}</span>
                  </label>
                </TableCell>
                <TableCell className="text-right tabular-nums">{m.item_count}</TableCell>
                <TableCell
                  className="max-w-48 truncate text-muted-foreground text-xs"
                  title={m.applies_when.join(", ")}
                >
                  {m.applies_when.join(", ")}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <p className="text-muted-foreground text-xs">
        {value.length === 0
          ? t("None ticked: automatic. A checklist applies when the goal or description contains one of its words.")
          : t("Only the ticked checklists apply to this task, whatever its goal says.")}
      </p>
    </div>
  );
}
