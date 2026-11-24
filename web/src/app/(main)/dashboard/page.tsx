"use client";

import { t } from "@/i18n";
import { useActiveCompany } from "@/stores/company/company-store";

import { ActivityTable, FindingsTable, TasksTable } from "./_components/engagement-tables";
import { KpiRow } from "./_components/kpi-row";
import { LiveStatus } from "./_components/live-bar";
import { SystemPanels } from "./_components/system-panels";
import { TokenUsageCard } from "./_components/token-usage-card";
import { useDashboardData } from "./_components/use-dashboard-data";
import { useTokenUsage } from "./_components/use-token-usage";

export default function DashboardPage() {
  // The sidebar's company switcher narrows the finding-derived panels to that
  // company (server-side `company` filter). Undefined means all companies.
  const company = useActiveCompany();
  const data = useDashboardData(company);
  const usage = useTokenUsage(data);
  const totalAssets = Object.values(data.assetCounts).reduce((a, b) => a + b, 0);

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">{t("Overview")}</h1>
          <p className="text-xs text-muted-foreground">{t("System-wide status · live refresh")}</p>
        </div>
        <LiveStatus lastUpdated={data.lastUpdated} onRefresh={data.refresh} />
      </div>

      <KpiRow
        tasks={data.tasks}
        findings={data.findings}
        pending={data.pending}
        settings={data.settings}
        totalAssets={totalAssets}
        trafficCount={data.traffic.length}
        tokens={usage.displayedTokens}
      />

      <TasksTable tasks={data.tasks} />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <FindingsTable findings={data.findings} />
        <ActivityTable activity={data.activity} />
      </div>

      <SystemPanels
        assetCounts={data.assetCounts}
        totalAssets={totalAssets}
        traffic={data.traffic}
        stats={data.stats}
        settings={data.settings}
        pending={data.pending}
        activeProfile={usage.activeProfile}
      />

      <TokenUsageCard usage={usage} llmProfiles={data.llmProfiles} />
    </div>
  );
}
