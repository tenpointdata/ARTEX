"use client";

import { t } from "@/i18n";
import type { NotificationFilter } from "@/lib/types";

import { SEVERITY_OPTIONS } from "./channel-fields";

export { ConfigField } from "@/components/config-field";

// FilterSummary condenses the filter into one line so the card shows what a channel pushes without expanding it.
export function FilterSummary({ filter }: { filter: NotificationFilter }) {
  const parts: string[] = [];
  if (filter.min_severity) {
    parts.push(SEVERITY_OPTIONS.find((o) => o.value === filter.min_severity)?.label ?? filter.min_severity);
  }
  if (filter.vulnclass_include?.length) parts.push(t("{count} type keywords", { count: filter.vulnclass_include.length }));
  if (filter.vulnclass_exclude?.length) parts.push(t("excludes {count} keywords", { count: filter.vulnclass_exclude.length }));
  if (filter.task_ids?.length) parts.push(t("{count} tasks", { count: filter.task_ids.length }));
  if (filter.asset_ids?.length) parts.push(t("{count} assets", { count: filter.asset_ids.length }));
  if (filter.on_status_change) parts.push(t("includes status changes"));
  if (parts.length === 0) {
    return <p className="text-muted-foreground text-sm">{t("All vulnerabilities")}</p>;
  }
  return <p className="text-muted-foreground text-sm">{parts.join(" · ")}</p>;
}
