"use client";

import {
  CircleCheckIcon,
  CircleSlashIcon,
  CircleXIcon,
  ClockIcon,
  Loader2Icon,
  PauseIcon,
  ShieldAlertIcon,
  Trash2Icon,
  ZapOffIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import type { Session, SessionStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

// statusIcon maps a session status to its icon. Worker terminal states are
// distinct & color-coded: done (green check circle) / stopped (amber slash circle) / error (red X circle) /
// steps exhausted (violet). running = blue spinner, pending (open) = gray clock.
function statusIcon(status: SessionStatus) {
  switch (status) {
    case "running": // executing
      return <Loader2Icon className="size-3.5 animate-spin text-blue-500" />;
    case "paused":
      return <PauseIcon className="size-3.5 text-amber-500" />;
    case "pending": // open to claim (open intent)
      return <ClockIcon className="size-3.5 text-muted-foreground" />;
    case "done": // completed
      return <CircleCheckIcon className="size-3.5 text-emerald-500" />;
    case "stopped": // cancelled/stopped (terminated by the planner)
      return <CircleSlashIcon className="size-3.5 text-amber-500" />;
    case "blocked": // error
      return <CircleXIcon className="size-3.5 text-red-500" />;
    case "exhausted": // steps exhausted (hit max_turns)
      return <ZapOffIcon className="size-3.5 text-violet-500" />;
    case "deleted": // soft-deleted by the user
      return <CircleSlashIcon className="size-3.5 text-muted-foreground" />;
  }
}

function sessionIcon(s: Session, deleted?: boolean) {
  if (deleted) return <Trash2Icon className="size-3.5 text-destructive" />;
  if (s.role === "worker") return statusIcon(s.status);
  if (s.live) return <Loader2Icon className="size-3.5 animate-spin text-blue-500" />;
  return null;
}

export function SessionItem({
  s,
  active,
  displayTitle,
  hasPending,
  unread,
  onClick,
  onCancel,
  controlling,
  deleted,
}: {
  s: Session;
  active: boolean;
  displayTitle: string;
  hasPending?: boolean;
  unread?: number;
  onClick: () => void;
  onCancel?: () => void;
  controlling?: boolean;
  deleted?: boolean;
}) {
  const icon = sessionIcon(s, deleted);

  const cancellable =
    s.role === "worker" &&
    !s.inherited &&
    !deleted &&
    // pending = open (unclaimed) intent; running and paused ones are also deletable.
    (s.status === "running" || s.status === "paused" || s.status === "pending");
  return (
    <div
      className={cn(
        "group/session flex w-full items-center rounded-md transition-colors",
        active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50",
      )}
    >
      <button
        type="button"
        onClick={onClick}
        className="flex min-w-0 flex-1 items-center gap-1 px-2 py-1.5 text-left text-sm"
      >
        {icon ?? <span className="size-3.5 shrink-0" />}
        {s.intent_id && (
          <span className="shrink-0 rounded bg-muted px-1 py-0.5 font-mono text-[10px] text-muted-foreground tabular-nums">
            #{s.intent_id}
          </span>
        )}
        {s.inherited && s.source_task_id && (
          <Badge variant="outline" className="shrink-0">
            {t("Source #{id}", { id: s.source_task_id })}
          </Badge>
        )}
        <span
          className={cn("min-w-0 flex-1 truncate font-medium text-sm", deleted && "text-muted-foreground line-through")}
        >
          {displayTitle}
        </span>
        {deleted && (
          <Badge variant="outline" className="shrink-0 border-destructive/40 text-destructive">
            {t("Deleted")}
          </Badge>
        )}
        {hasPending && <ShieldAlertIcon className="size-3.5 shrink-0 text-amber-500" />}
        {!active && unread ? (
          <span className="inline-flex min-w-4 items-center justify-center rounded-full bg-blue-500/15 px-1 font-medium text-[10px] text-blue-600 tabular-nums dark:text-blue-400">
            {unread > 99 ? "99+" : unread}
          </span>
        ) : null}
        {s.live && (
          <span className="inline-flex items-center gap-1 rounded bg-blue-500/15 px-1.5 py-0.5 font-medium text-[10px] text-blue-600 dark:text-blue-400">
            <span className="size-1 animate-pulse rounded-full bg-blue-500" />
            {t("Live")}
          </span>
        )}
      </button>
      {cancellable && onCancel && (
        <div className="flex shrink-0 items-center gap-0.5 pr-1 opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover/session:opacity-100 sm:group-focus-within/session:opacity-100">
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            onClick={onCancel}
            disabled={controlling}
            title={t("Delete this intent (reason required; soft or hard delete)")}
            aria-label={t("Delete this intent (reason required; soft or hard delete)")}
            className="text-destructive hover:text-destructive"
          >
            <Trash2Icon />
          </Button>
        </div>
      )}
    </div>
  );
}
