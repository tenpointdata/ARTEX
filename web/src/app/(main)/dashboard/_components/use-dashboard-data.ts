"use client";

import * as React from "react";

import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type {
  Activity,
  ConvTokenSummary,
  Finding,
  InterceptPending,
  LLMProfile,
  Settings,
  Stats,
  Task,
  TrafficExchange,
  UsageStats,
} from "@/lib/types";

// The server has no global push channel for tasks/findings/approvals: the only
// SSE streams are the backend log tail, the per-task activity stream and the
// per-exchange traffic stream. So the dashboard polls, pauses while the tab is
// hidden (setVisibleInterval) and refreshes immediately on focus/visibility.

export interface DashboardData {
  tasks: Task[];
  findings: Finding[];
  stats: Stats | null;
  settings: Settings | null;
  pending: InterceptPending[];
  activity: Activity[];
  convTokens: ConvTokenSummary[];
  usageStats: UsageStats | null;
  assetCounts: Record<string, number>;
  traffic: TrafficExchange[];
  llmProfiles: LLMProfile[];
  lastUpdated: number | null;
  refresh: () => void;
}

// Runs `load` now, every `delay` ms while visible, and on window focus.
function usePolled(load: () => Promise<void>, delay: number, tick: () => void, scope?: string) {
  const loadRef = React.useRef(load);
  loadRef.current = load;
  const tickRef = React.useRef(tick);
  tickRef.current = tick;
  // biome-ignore lint/correctness/useExhaustiveDependencies: scope restarts the poll when the company changes
  React.useEffect(() => {
    let alive = true;
    let loading = false;
    const run = async () => {
      if (loading) return;
      loading = true;
      try {
        await loadRef.current();
        if (alive) tickRef.current();
      } catch {
        // Preserve stale data and retry on the next interval.
      } finally {
        loading = false;
      }
    };
    void run();
    const stop = setVisibleInterval(run, delay);
    const onFocus = () => void run();
    window.addEventListener("focus", onFocus);
    const onManual = () => void run();
    window.addEventListener(MANUAL_REFRESH, onManual);
    return () => {
      alive = false;
      stop();
      window.removeEventListener("focus", onFocus);
      window.removeEventListener(MANUAL_REFRESH, onManual);
    };
  }, [delay, scope]);
}

const MANUAL_REFRESH = "artex:dashboard-refresh";

export function useDashboardData(company: string | undefined): DashboardData {
  const [tasks, setTasks] = React.useState<Task[]>([]);
  const [findings, setFindings] = React.useState<Finding[]>([]);
  const [stats, setStats] = React.useState<Stats | null>(null);
  const [settings, setSettings] = React.useState<Settings | null>(null);
  const [pending, setPending] = React.useState<InterceptPending[]>([]);
  const [activity, setActivity] = React.useState<Activity[]>([]);
  const [convTokens, setConvTokens] = React.useState<ConvTokenSummary[]>([]);
  const [usageStats, setUsageStats] = React.useState<UsageStats | null>(null);
  const [assetCounts, setAssetCounts] = React.useState<Record<string, number>>({});
  const [traffic, setTraffic] = React.useState<TrafficExchange[]>([]);
  const [llmProfiles, setLLMProfiles] = React.useState<LLMProfile[]>([]);
  const [lastUpdated, setLastUpdated] = React.useState<number | null>(null);
  const tick = React.useCallback(() => setLastUpdated(Date.now()), []);

  // The task list is the most expensive source; poll it on its own so a large
  // history cannot hold back the other panels.
  const taskSignature = React.useRef("");
  usePolled(
    async () => {
      const tr = await api.tasks();
      const next = JSON.stringify(tr.tasks);
      if (next !== taskSignature.current) {
        taskSignature.current = next;
        setTasks(tr.tasks);
      }
    },
    5000,
    tick,
  );

  // Fast sources stay batched so one poll causes a single render.
  usePolled(
    async () => {
      const [f, s, st, p, a, conv] = await Promise.all([
        api.findings(undefined, company),
        api.stats(),
        api.settings(),
        api.interceptPending(),
        api.activity(undefined, { limit: 30 }),
        api.conversationTokens(),
      ]);
      setFindings(f);
      setStats(s);
      setSettings(st);
      setPending(p);
      setActivity(a.items);
      setConvTokens(conv);
    },
    5000,
    tick,
    company ?? "",
  );

  // Slow sources: traffic, assets, profiles, usage ledger.
  usePolled(
    async () => {
      const [traf, counts, profileList, usage] = await Promise.all([
        api.traffic(0, 50),
        api.assetCounts(),
        api.llmProfiles(),
        api.usageStats(365),
      ]);
      setTraffic(traf.exchanges ?? []);
      setAssetCounts(counts ?? {});
      setLLMProfiles(profileList);
      setUsageStats(usage);
    },
    15000,
    tick,
  );

  const refresh = React.useCallback(() => window.dispatchEvent(new Event(MANUAL_REFRESH)), []);

  return {
    tasks,
    findings,
    stats,
    settings,
    pending,
    activity,
    convTokens,
    usageStats,
    assetCounts,
    traffic,
    llmProfiles,
    lastUpdated,
    refresh,
  };
}
