import {
  BugIcon,
  CompassIcon,
  FlagIcon,
  FlaskConicalIcon,
  LayersIcon,
  LightbulbIcon,
  type LucideIcon,
  TargetIcon,
} from "lucide-react";

import { intlLocale, t } from "@/i18n";
import type { Tone } from "@/lib/status";
import type { ExploreKind, TaskNode } from "@/lib/types";

export const PAGE_SIZES = [20, 50, 100];
export const POLL_MS = 8000;

export type KindMeta = { label: string; icon: LucideIcon; dot: string; chip: string };

// The broadcast board's own display metadata, deliberately not reusing the exploration graph's copy: the graph is a topology view (node cards, edge colors),
// the broadcast a stream view (timeline rows); the two differ in information density and color needs, so they evolve separately.
export const KIND_META: Record<string, KindMeta> = {
  begin: {
    label: t("Start"),
    icon: FlagIcon,
    dot: "bg-slate-500",
    chip: "bg-slate-500/15 text-slate-600 dark:text-slate-300",
  },
  task: {
    label: t("Root task"),
    icon: FlagIcon,
    dot: "bg-slate-500",
    chip: "bg-slate-500/15 text-slate-600 dark:text-slate-300",
  },
  goal: {
    label: t("Goal"),
    icon: TargetIcon,
    dot: "bg-emerald-500",
    chip: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  },
  intent: {
    label: t("Intent"),
    icon: CompassIcon,
    dot: "bg-blue-500",
    chip: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  },
  fact: {
    label: t("Fact"),
    icon: FlaskConicalIcon,
    dot: "bg-amber-500",
    chip: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  },
  finding: {
    label: t("Vulnerability"),
    icon: BugIcon,
    dot: "bg-rose-500",
    chip: "bg-rose-500/15 text-rose-600 dark:text-rose-400",
  },
  hint: {
    label: t("Hint"),
    icon: LightbulbIcon,
    dot: "bg-violet-500",
    chip: "bg-violet-500/15 text-violet-600 dark:text-violet-400",
  },
  digest: {
    label: t("Digest"),
    icon: LayersIcon,
    dot: "bg-teal-500",
    chip: "bg-teal-500/15 text-teal-600 dark:text-teal-400",
  },
};

// Filterable kinds. The start (fact/state=origin) is not listed separately; it is filtered together with "Fact".
export const FILTER_KINDS: ExploreKind[] = ["goal", "intent", "fact", "finding", "hint", "digest"];

export const REL_LABEL: Record<string, string> = {
  spawns: t("Spawns"),
  derived_from: t("Intent chain"),
  yields: t("Yields"),
  proves: t("Proves"),
  covers: t("Compresses"),
};

// State semantics for goal / intent come from the global status table (StatusBadge); the other kinds' states only
// appear in the graph and broadcast, so they are defined here.
export const STATE_META: Record<string, Record<string, { label: string; tone: Tone }>> = {
  fact: {
    origin: { label: t("Start"), tone: "slate" },
    confirmed: { label: t("Confirmed"), tone: "green" },
    dismissed: { label: t("Dismissed"), tone: "slate" },
  },
  finding: {
    confirmed: { label: t("Confirmed"), tone: "red" },
    dismissed: { label: t("Excluded"), tone: "slate" },
  },
  hint: {
    active: { label: t("Pending adoption"), tone: "violet" },
    consumed: { label: t("Adopted"), tone: "slate" },
  },
  digest: {
    active: { label: t("Active"), tone: "green" },
    superseded: { label: t("Superseded"), tone: "slate" },
  },
};

// The task root is a fact with state=origin, shown as "Start" in the broadcast.
export function viewKind(n: TaskNode): string {
  return n.type === "fact" && n.state === "origin" ? "begin" : n.type;
}

export const SUMMARY_FIELDS: Record<string, string[]> = {
  begin: ["summary", "description"],
  task: ["summary", "description"],
  goal: ["text"],
  intent: ["summary"],
  fact: ["summary"],
  finding: ["name", "summary"],
  hint: ["text", "summary"],
  digest: ["body", "summary"],
};

export function summaryOf(n: TaskNode): string {
  const raw = n.payload ?? "";
  if (!raw.trim()) return "";
  for (const field of SUMMARY_FIELDS[viewKind(n)] ?? []) {
    try {
      const obj: unknown = JSON.parse(raw);
      if (obj && typeof obj === "object") {
        const v = (obj as Record<string, unknown>)[field];
        if (typeof v === "string" && v.trim()) return v;
      }
    } catch {
      return raw; // non-JSON payload: broadcast as-is
    }
  }
  return raw;
}

export function prettyPayload(raw?: string): string {
  if (!raw?.trim()) return t("(no payload)");
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export function relTime(ts: number, now: number): string {
  if (!now || !ts) return "";
  const sec = Math.max(0, (now - ts) / 1000);
  if (sec < 60) return t("just now");
  if (sec < 3600) return t("{count} minutes ago", { count: Math.floor(sec / 60) });
  if (sec < 86400) return t("{count} hours ago", { count: Math.floor(sec / 3600) });
  return t("{count} days ago", { count: Math.floor(sec / 86400) });
}

export const dayFmt = new Intl.DateTimeFormat(intlLocale, { month: "long", day: "numeric", weekday: "short" });
export const clockFmt = new Intl.DateTimeFormat(intlLocale, {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});
