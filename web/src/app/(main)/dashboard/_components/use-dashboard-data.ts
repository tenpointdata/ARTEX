"use client";

import * as React from "react";

import { useLiveEvents } from "@/hooks/use-live-events";
import { api } from "@/lib/api";
import { type LiveEvent, type LiveEventKind, touches } from "@/lib/live-events";
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

// Live by push with polling as the fallback. GET /api/events/stream (useLiveEvents)
// says when a task's status, a finding or the approval queue changed; the panels
// fed by those reload within a moment (debounced) and poll only slowly while the
// stream is open. Without the stream (closed, refused, mock mode) they poll every
// 5s as before. Activity, tokens and the slow sources are not pushed and keep
// their own intervals. Every poll pauses while the tab is hidden
// (setVisibleInterval) and runs at once on focus or a manual refresh.

// Poll periods (ms).
const FAST = 5000; // pushed sources while the stream is down
const FALLBACK = 60000; // pushed sources while the stream is open
const AMBIENT = 15000; // activity feed and token totals (not pushed)
const SLOW = 15000; // traffic, assets, profiles, usage ledger
// Coalesces a burst of change frames into one reload.
const PUSH_DEBOUNCE = 250;

// Reload topics: which loaders a change frame wakes.
type Topic = "tasks" | "board";
const TOPIC_KINDS: Record<Topic, readonly LiveEventKind[]> = {
  tasks: ["task"],
  board: ["finding", "approval", "task"],
};

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
  // True while the push stream is open (polling is only the slow fallback).
  live: boolean;
  refresh: () => void;
}

// Runs `load` now, every `delay` ms while visible, on window focus, on a manual
// refresh and when `topic` fires on `kick`. A trigger that lands while a load
// is running reruns it once afterwards, so no change is skipped.
function usePolled(
  load: () => Promise<void>,
  delay: number,
  tick: () => void,
  opts: { scope?: string; kick?: EventTarget; topic?: Topic } = {},
) {
  const { scope, kick, topic } = opts;
  const loadRef = React.useRef(load);
  loadRef.current = load;
  const tickRef = React.useRef(tick);
  tickRef.current = tick;
  // biome-ignore lint/correctness/useExhaustiveDependencies: scope restarts the poll when the company changes
  React.useEffect(() => {
    let alive = true;
    let loading = false;
    let again = false;
    const run = async () => {
      if (loading) {
        again = true;
        return;
      }
      loading = true;
      try {
        do {
          again = false;
          try {
            await loadRef.current();
            if (alive) tickRef.current();
          } catch {
            // Preserve stale data and retry on the next trigger.
          }
        } while (again && alive);
      } finally {
        loading = false;
      }
    };
    void run();
    const stop = setVisibleInterval(run, delay);
    const onTrigger = () => void run();
    window.addEventListener("focus", onTrigger);
    window.addEventListener(MANUAL_REFRESH, onTrigger);
    if (kick && topic) kick.addEventListener(topic, onTrigger);
    return () => {
      alive = false;
      stop();
      window.removeEventListener("focus", onTrigger);
      window.removeEventListener(MANUAL_REFRESH, onTrigger);
      if (kick && topic) kick.removeEventListener(topic, onTrigger);
    };
  }, [delay, scope, kick, topic]);
}

// usePushKick turns change frames into debounced topic events on one EventTarget.
function usePushKick(company: string | undefined): { kick: EventTarget; live: boolean } {
  const [kick] = React.useState(() => new EventTarget());
  const timers = React.useRef<Partial<Record<Topic, ReturnType<typeof setTimeout>>>>({});
  const onEvent = React.useCallback(
    (ev: LiveEvent) => {
      for (const topic of Object.keys(TOPIC_KINDS) as Topic[]) {
        if (!touches(ev, TOPIC_KINDS[topic]) || timers.current[topic] !== undefined) continue;
        timers.current[topic] = setTimeout(() => {
          timers.current[topic] = undefined;
          kick.dispatchEvent(new Event(topic));
        }, PUSH_DEBOUNCE);
      }
    },
    [kick],
  );
  React.useEffect(
    () => () => {
      for (const id of Object.values(timers.current)) clearTimeout(id);
    },
    [],
  );
  const live = useLiveEvents(onEvent, company);
  return { kick, live };
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
  const { kick, live } = usePushKick(company);
  const pushed = live ? FALLBACK : FAST;

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
    pushed,
    tick,
    { kick, topic: "tasks" },
  );

  // Pushed sources stay batched so one reload causes a single render.
  usePolled(
    async () => {
      const [f, s, p] = await Promise.all([api.findings(undefined, company), api.stats(), api.interceptPending()]);
      setFindings(f);
      setStats(s);
      setPending(p);
    },
    pushed,
    tick,
    { scope: company ?? "", kick, topic: "board" },
  );

  // Ambient sources: not pushed, polled on their own interval.
  usePolled(
    async () => {
      const [st, a, conv] = await Promise.all([
        api.settings(),
        api.activity(undefined, { limit: 30 }),
        api.conversationTokens(),
      ]);
      setSettings(st);
      setActivity(a.items);
      setConvTokens(conv);
    },
    AMBIENT,
    tick,
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
    SLOW,
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
    live,
    refresh,
  };
}
