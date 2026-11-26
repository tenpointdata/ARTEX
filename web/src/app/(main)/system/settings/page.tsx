"use client";

import { t } from "@/i18n";

import { ReportBrandingCard } from "./_components/report-branding-card";
import { ChatSendKeyCard } from "./_components/chat-send-key-card";
import {
  ConstraintInjectionCard,
  ExperimentalCard,
  PythonInterpreterCard,
  WorkersCard,
} from "./_components/runtime-cards";
import { AgentTrafficBindingCard, GlobalProxyCard, TrafficCaptureCard } from "./_components/traffic-cards";
import { useSystemSettings } from "./_components/use-system-settings";
import { WebSearchCard } from "./_components/web-search-card";
import { NoiseProfileCard } from "./noise-profile-card";

export default function SystemSettingsPage() {
  const s = useSystemSettings();

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">{t("System settings")}</h1>
        <p className="text-muted-foreground text-sm">{t("Global runtime switches")}</p>
      </div>

      {/* Multi-column instead of grid: the web search card is several times taller than the others and
          its height varies with the selected backend (brave/tavily key inputs are conditional). A grid
          would stretch the row to the tallest card and leave blank space beside the others, while columns
          balance-fill by content height. Card spacing uses mb rather than gap — in a multi-column layout
          column-gap only covers the gaps between columns; row spacing must come from the children. */}
      <div className="columns-1 gap-4 md:gap-6 lg:columns-2">
        <TrafficCaptureCard
          trafficCapture={s.trafficCapture}
          loaded={s.loaded}
          saving={s.saving}
          onToggle={s.toggleTraffic}
        />

        <NoiseProfileCard />

        <ReportBrandingCard />

        <AgentTrafficBindingCard
          agentTrafficBinding={s.agentTrafficBinding}
          loaded={s.loaded}
          saving={s.saving}
          onToggle={s.toggleAgentTrafficBinding}
        />

        <GlobalProxyCard {...s} />

        <ConstraintInjectionCard {...s} />

        <ExperimentalCard {...s} />

        <WebSearchCard {...s} />

        <PythonInterpreterCard {...s} />

        <WorkersCard {...s} />

        <ChatSendKeyCard />
      </div>
    </div>
  );
}
