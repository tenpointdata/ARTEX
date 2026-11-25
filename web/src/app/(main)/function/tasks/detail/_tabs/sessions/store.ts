import type { Activity } from "@/lib/types";

import { MAX_KEEP, mainSessionKey } from "./helpers";

// One session's lazily-loaded, reverse-paginated cache. `lastTs`/`unread` are kept
// live even for sessions that were never opened, so the list shows liveness + unread
// without holding their full history.
export type SessionState = {
  items: Activity[];
  loaded: boolean;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean; // older history remains above the loaded window
  earliestSeq: number; // earliest loaded id — reverse-pagination anchor
  unread: number;
  lastTs: string; // most-recent activity time (drives live badge; updated even when unloaded)
  error?: string;
};
export type SessionStore = Record<string, SessionState>;

export function emptyState(): SessionState {
  return {
    items: [],
    loaded: false,
    loading: false,
    loadingMore: false,
    hasMore: false,
    earliestSeq: 0,
    unread: 0,
    lastTs: "",
  };
}

// sessionKeyOf routes an activity to its stable session key. worker="planner" covers
// BOTH the Goal Agent's round-0 decomposition and the Planner (single Plan session).
export function sessionKeyOf(a: Activity): string {
  if (a.worker === "system" || a.kind === "llm_switch" || a.kind === "llm_failover") return "system";
  if (a.worker === "mainagent") return `main:${a.main_seg ?? 0}`; // one key per conversation segment
  if (a.worker === "planner") return "plan";
  if (a.intent_id) return `intent:${a.intent_id}`;
  return "unknown";
}

// mergeBySeq unions two activity lists by seq (dedup) in ascending seq order. Every
// data source — latest page, older page, SSE compensation, live tail — goes through
// this, so history responses can never clobber live records received meanwhile.
export function mergeBySeq(current: Activity[], incoming: Activity[]): Activity[] {
  if (!incoming.length) return current;
  const bySeq = new Map<number, Activity>();
  for (const a of current) bySeq.set(a.seq, a);
  for (const a of incoming) bySeq.set(a.seq, a);
  return [...bySeq.values()].sort((p, q) => p.seq - q.seq);
}

// loadedState is an empty, already-loaded cache (a session known to hold nothing yet).
export function loadedState(): SessionState {
  return { ...emptyState(), loaded: true };
}

// bucketMockActivity groups one activity snapshot by session key (MOCK demo only).
export function bucketMockActivity(items: Activity[]): SessionStore {
  const buckets: SessionStore = {};
  for (const a of items) {
    const k = sessionKeyOf(a);
    if (!buckets[k]) buckets[k] = emptyState();
    buckets[k].items.push(a);
  }
  for (const k of Object.keys(buckets)) {
    const st = buckets[k];
    st.items.sort((p, q) => p.seq - q.seq);
    st.loaded = true;
    st.hasMore = false;
    st.earliestSeq = st.items.length ? st.items[0].seq : 0;
    st.lastTs = st.items.length ? st.items[st.items.length - 1].ts : "";
  }
  buckets[mainSessionKey(0)] ??= loadedState();
  buckets.system ??= loadedState();
  return buckets;
}

// applyFrame folds one live SSE activity frame into the store.
export function applyFrame(prev: SessionStore, a: Activity, activeK: string, atBottom: boolean): SessionStore {
  const k = sessionKeyOf(a);
  const cur = prev[k] ?? emptyState();
  // Merge into sessions that are loaded, actively loading, or the current
  // view (so returning is instant + a frame that lands mid-load isn't lost).
  // A cold, inactive session also keeps a tiny accounting tail so the
  // sidebar can add the latest unfinished usage without loading history.
  if (!cur.loaded && !cur.loading && k !== activeK) {
    const accountingItems =
      a.kind === "usage" || a.kind === "result" ? mergeBySeq(cur.items, [a]).slice(-4) : cur.items;
    return {
      ...prev,
      [k]: { ...cur, items: accountingItems, lastTs: a.ts, unread: cur.unread + 1 },
    };
  }
  let items = mergeBySeq(cur.items, [a]);
  // Memory bound: trim oldest when over cap (older re-fetched on scroll-up),
  // but never while the user is reading this session's history (scrolled up).
  let hasMore = cur.hasMore;
  let earliestSeq = cur.earliestSeq;
  const trimmable = k !== activeK || atBottom;
  if (trimmable && items.length > MAX_KEEP) {
    items = items.slice(items.length - MAX_KEEP);
    hasMore = true;
    earliestSeq = items[0].seq;
  }
  const unread = k === activeK ? 0 : cur.unread + 1;
  return { ...prev, [k]: { ...cur, items, lastTs: a.ts, unread, hasMore, earliestSeq } };
}
