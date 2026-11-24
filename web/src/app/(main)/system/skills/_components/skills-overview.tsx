"use client";

import * as React from "react";


import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { intlLocale, t } from "@/i18n";
import type { MissingSkill, SkillItem } from "@/lib/types";

export function fmtTime(ts?: string) {
  if (!ts) return t("Never called");
  return new Date(ts).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// ── Overview (empty-state) ──────────────────────────────────────────────────
// Shown when nothing is selected: a library-wide snapshot from data already loaded
// (the skill list carries per-skill usage; missing is fetched alongside). No extra
// requests — this is pure aggregation over props.
export function SkillsOverview({
  skills,
  missing,
  onSelect,
}: {
  skills: SkillItem[];
  missing: MissingSkill[];
  onSelect: (name: string) => void;
}) {
  const agg = React.useMemo(() => {
    const totalCalls = skills.reduce((n, s) => n + s.calls, 0);
    const used = skills.filter((s) => s.calls > 0);
    const ranked = [...used].sort((a, b) => b.calls - a.calls);
    const neverUsed = skills.filter((s) => s.calls === 0);
    const recent = skills
      .filter((s) => s.last_used)
      .sort((a, b) => (a.last_used! < b.last_used! ? 1 : -1))
      .slice(0, 6);
    const missingCalls = missing.reduce((n, m) => n + m.calls, 0);
    return {
      totalCalls,
      usedCount: used.length,
      ranked,
      topCalls: ranked[0]?.calls ?? 0,
      neverUsed,
      recent,
      missingCalls,
    };
  }, [skills, missing]);

  if (skills.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-sm text-muted-foreground">
          {t('No skills yet — click "Create" or "Upload archive" on the left to get started')}
        </p>
      </div>
    );
  }

  const stats: { label: string; value: React.ReactNode; hint?: string }[] = [
    { label: t("Total skills"), value: skills.length, hint: t("{count} have been called", { count: agg.usedCount }) },
    { label: t("Total calls"), value: agg.totalCalls },
    {
      label: t("Unused"),
      value: agg.neverUsed.length,
      hint: agg.neverUsed.length > 0 ? t("never loaded by any agent") : t("all used"),
    },
    {
      label: t("Missed calls"),
      value: agg.missingCalls,
      hint: missing.length > 0 ? t("{count} missing skills", { count: missing.length }) : t("none"),
    },
  ];

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <div>
        <h2 className="text-base font-semibold">{t("Skill library overview")}</h2>
        <p className="text-muted-foreground text-sm">
          {t(
            "Select a skill on the left to view details and call history, or get a quick picture of overall usage here.",
          )}
        </p>
      </div>

      {/* stat cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className="rounded-lg border p-3">
            <p className="text-2xl font-semibold tabular-nums">{s.value}</p>
            <p className="text-xs font-medium">{s.label}</p>
            {s.hint && <p className="text-muted-foreground mt-0.5 text-[11px]">{s.hint}</p>}
          </div>
        ))}
      </div>

      {/* call ranking */}
      <div className="space-y-2">
        <Label className="text-xs text-muted-foreground">{t("Call ranking")}</Label>
        {agg.ranked.length === 0 ? (
          <p className="text-muted-foreground rounded-lg border border-dashed p-4 text-center text-xs">
            {t("No skill call records yet.")}
          </p>
        ) : (
          <div className="space-y-1.5">
            {agg.ranked.slice(0, 8).map((s) => (
              <button
                key={s.name}
                type="button"
                onClick={() => onSelect(s.name)}
                className="group flex w-full items-center gap-3 rounded-md px-2 py-1.5 text-left hover:bg-muted"
              >
                <span className="w-40 shrink-0 truncate font-mono text-xs" title={s.name}>
                  {s.name}
                </span>
                <span className="relative h-2 flex-1 overflow-hidden rounded-full bg-muted">
                  <span
                    className="absolute inset-y-0 left-0 rounded-full bg-primary/70"
                    style={{ width: `${agg.topCalls > 0 ? (s.calls / agg.topCalls) * 100 : 0}%` }}
                  />
                </span>
                <span className="w-16 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                  {t("{count} calls", { count: s.calls })}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>

      <div className="grid gap-6 sm:grid-cols-2">
        {/* recent calls */}
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">{t("Recent calls")}</Label>
          {agg.recent.length === 0 ? (
            <p className="text-muted-foreground text-xs">{t("No records yet.")}</p>
          ) : (
            <div className="space-y-1">
              {agg.recent.map((s) => (
                <button
                  key={s.name}
                  type="button"
                  onClick={() => onSelect(s.name)}
                  className="flex w-full items-center gap-2 rounded px-1.5 py-1 text-left hover:bg-muted"
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs" title={s.name}>
                    {s.name}
                  </span>
                  <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">
                    {fmtTime(s.last_used)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* unused (candidates for cleanup / exposure) */}
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">
            {t("Unused skills")}
            {agg.neverUsed.length > 0 && <span className="ml-1 font-normal"> ({agg.neverUsed.length})</span>}
          </Label>
          {agg.neverUsed.length === 0 ? (
            <p className="text-muted-foreground text-xs">{t("All skills have been called.")}</p>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              {agg.neverUsed.map((s) => (
                <button key={s.name} type="button" onClick={() => onSelect(s.name)} title={s.name}>
                  <Badge
                    variant="outline"
                    className="max-w-[12rem] cursor-pointer truncate font-mono text-xs font-normal hover:bg-muted"
                  >
                    {s.name}
                  </Badge>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
