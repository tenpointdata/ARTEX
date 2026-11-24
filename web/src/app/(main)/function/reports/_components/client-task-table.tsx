"use client";

import { StatusBadge } from "@/components/status-badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { ClientTask } from "@/lib/governance-types";
import { cn } from "@/lib/utils";

// localTime renders an RFC 3339 stamp in the browser's zone; "" stays a dash.
export function localTime(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// ClientTaskTable lists a company's engagements; a click selects one.
export function ClientTaskTable({
  tasks,
  selected,
  onSelect,
}: {
  tasks: ClientTask[];
  selected: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("Engagement")}</TableHead>
          <TableHead>{t("Status")}</TableHead>
          <TableHead>{t("Started")}</TableHead>
          <TableHead>{t("Finished")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tasks.map((task) => (
          <TableRow
            key={task.id}
            aria-selected={task.id === selected}
            tabIndex={0}
            className={cn("cursor-pointer", task.id === selected && "bg-muted")}
            onClick={() => onSelect(task.id)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(task.id);
              }
            }}
          >
            <TableCell className="font-medium">{task.name || t("Task #{id}", { id: task.id })}</TableCell>
            <TableCell>
              <StatusBadge domain="task" value={task.status} dot />
            </TableCell>
            <TableCell className="tabular-nums">{localTime(task.created_at)}</TableCell>
            <TableCell className="tabular-nums">{localTime(task.completed_at)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
