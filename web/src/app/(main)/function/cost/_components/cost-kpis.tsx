import { t } from "@/i18n";
import type { CostRow } from "@/lib/types";

import { costMetrics, fmtDuration, fmtEur, fmtInt, fmtPct } from "./cost-metrics";

export function CostKpis({ total }: { total: CostRow }) {
  const m = costMetrics(total);
  const items: [string, string][] = [
    [t("Total cost"), fmtEur(total.cost_eur) + (total.unpriced_calls > 0 ? " *" : "")],
    [t("Total tokens"), fmtInt(m.tokens)],
    [t("Cache hit rate"), fmtPct(m.cacheHit)],
    [t("LLM time"), fmtDuration(total.latency_ms)],
  ];
  return (
    <dl className="flex flex-wrap gap-x-8 gap-y-2 text-sm">
      {items.map(([k, v]) => (
        <div key={k} className="flex items-baseline gap-2">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="font-semibold text-lg tabular-nums">{v}</dd>
        </div>
      ))}
    </dl>
  );
}
