import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api, sseUrl } from "@/lib/api";
import { MOCK } from "@/lib/mock/enabled";
import type { Activity, TaskLLMResolutions } from "@/lib/types";

import { MAIN_ID, mainSessionId, mainSessionKey, PAGE, SYSTEM_SCAN_PAGE } from "./helpers";
import {
  applyFrame,
  bucketMockActivity,
  emptyState,
  loadedState,
  mergeBySeq,
  type SessionState,
  type SessionStore,
} from "./store";

export type MainSeg = { seq: number; created_at: string };

// useSessionStream owns the per-session history caches and the single task-level SSE
// (see the reliability model in store.ts). History + SSE meet gap-free at the snapshot
// cursor and are merged by seq, so refresh / tab-switch / sleep / reconnect never drop
// the newest records.
export function useSessionStream({
  taskId,
  setActiveId,
  setLLMResolutions,
}: {
  taskId: string;
  setActiveId: (id: string) => void;
  setLLMResolutions: (value: TaskLLMResolutions | null) => void;
}) {
  // Per-session lazily-loaded caches, keyed by session_key (main | plan | intent:<id>).
  const [store, setStore] = React.useState<SessionStore>({});
  // Main-agent conversation segments (newest-first); currentSeg is the writable one.
  const [mainSegs, setMainSegs] = React.useState<MainSeg[]>([{ seq: 0, created_at: "" }]);
  const [currentSeg, setCurrentSeg] = React.useState(0);
  // SSE connection state — surfaced so a dropped realtime link is visible, never
  // silently shown as "no messages".
  const [sseLive, setSseLive] = React.useState(false);

  // Refs backing SSE/loading without re-render churn.
  const snapshotRef = React.useRef(0); // task-level snapshot cursor → SSE since=
  const esRef = React.useRef<EventSource | null>(null);
  const activeKeyRef = React.useRef(mainSessionKey(0)); // current session key (for SSE dispatch/unread)
  const atBottomRef = React.useRef(true); // transcript pinned to bottom?
  const llmToastSeqRef = React.useRef<Set<number>>(new Set());
  // Per-key request token: a stale response for a key is ignored (guards fast
  // latest/older interleaving). Writes are ALWAYS keyed, so a late response can only
  // touch its own session cache — never the currently-viewed one (see §7.5).
  const reqTokenRef = React.useRef<Record<string, number>>({});
  // Keys with a latest-page load in flight — dedups the double trigger where the
  // first-load effect and the active-session effect both want "main" on mount (the
  // former also opens the SSE, so it must not be pre-empted).
  const loadingKeysRef = React.useRef<Set<string>>(new Set());

  const patchStore = React.useCallback((key: string, fn: (s: SessionState) => SessionState) => {
    setStore((prev) => ({ ...prev, [key]: fn(prev[key] ?? emptyState()) }));
  }, []);

  // Load a session's LATEST page (before=0) on first open. Keyed + request-token
  // guarded so a switch away can't corrupt the view.
  const loadSession = React.useCallback(
    (key: string) => {
      if (MOCK) return; // MOCK preloads everything up front
      if (loadingKeysRef.current.has(key)) return; // already in flight (e.g. main on mount)
      loadingKeysRef.current.add(key);
      const token = (reqTokenRef.current[key] ?? 0) + 1;
      reqTokenRef.current[key] = token;
      patchStore(key, (s) => ({ ...s, loading: true, error: undefined }));

      if (key === "system") {
        void (async () => {
          let since = 0;
          let systemItems: Activity[] = [];
          for (;;) {
            const page = await api.activity(taskId, { since, limit: SYSTEM_SCAN_PAGE });
            if (reqTokenRef.current[key] !== token) return;
            systemItems = mergeBySeq(
              systemItems,
              page.items.filter(
                (item) => item.worker === "system" || item.kind === "llm_switch" || item.kind === "llm_failover",
              ),
            );
            if (page.items.length < SYSTEM_SCAN_PAGE || page.cursor <= since) break;
            since = page.cursor;
          }
          patchStore(key, (s) => {
            const items = mergeBySeq(systemItems, s.items);
            return {
              ...s,
              items,
              loaded: true,
              loading: false,
              hasMore: false,
              earliestSeq: items.length ? items[0].seq : 0,
              unread: 0,
              lastTs: items.length ? items[items.length - 1].ts : s.lastTs,
              error: undefined,
            };
          });
        })()
          .catch((error) => {
            if (reqTokenRef.current[key] !== token) return;
            patchStore(key, (s) => ({
              ...s,
              loading: false,
              error: (error as Error).message || t("Failed to load"),
            }));
          })
          .finally(() => loadingKeysRef.current.delete(key));
        return;
      }

      api
        .activityHistory(taskId, key, 0, PAGE)
        .then((r) => {
          if (reqTokenRef.current[key] !== token) return; // superseded
          if (r.snapshotCursor > snapshotRef.current) snapshotRef.current = r.snapshotCursor;
          patchStore(key, (s) => {
            const items = mergeBySeq(r.items, s.items); // keep any live frames arrived meanwhile
            return {
              ...s,
              items,
              loaded: true,
              loading: false,
              hasMore: r.hasMore,
              earliestSeq: items.length ? items[0].seq : 0,
              unread: 0,
              error: undefined,
            };
          });
        })
        .catch((e) => {
          if (reqTokenRef.current[key] !== token) return;
          patchStore(key, (s) => ({ ...s, loading: false, error: (e as Error).message || t("Failed to load") }));
        })
        .finally(() => loadingKeysRef.current.delete(key));
    },
    [taskId, patchStore],
  );

  // Load one older page (scroll-up) for a session, preserving scroll position.
  const loadEarlier = React.useCallback(
    (key: string, viewport: () => HTMLElement | null) => {
      const st = store[key];
      if (!st || st.loadingMore || !st.hasMore || !st.earliestSeq) return;
      const vp = viewport();
      const prevH = vp?.scrollHeight ?? 0;
      const prevTop = vp?.scrollTop ?? 0;
      patchStore(key, (s) => ({ ...s, loadingMore: true }));
      api
        .activityHistory(taskId, key, st.earliestSeq, PAGE)
        .then((r) => {
          patchStore(key, (s) => {
            const items = mergeBySeq(r.items, s.items);
            return {
              ...s,
              items,
              loadingMore: false,
              hasMore: r.hasMore,
              earliestSeq: items.length ? items[0].seq : s.earliestSeq,
            };
          });
          requestAnimationFrame(() => {
            const v = viewport();
            if (v) v.scrollTop = prevTop + (v.scrollHeight - prevH);
          });
        })
        .catch(() => {
          patchStore(key, (s) => ({ ...s, loadingMore: false }));
        });
    },
    [taskId, store, patchStore],
  );

  // ── first load + single task SSE ────────────────────────────────────────────────
  // On task open: load Main's latest page, take the task-level snapshot cursor from
  // it, THEN open ONE task SSE at since=snapshot_cursor. History covers id≤cursor and
  // the SSE covers id>cursor with no gap. The SSE tails ALL agents; frames are routed
  // by session_key. EventSource auto-reconnects and (via our `id:` lines → Last-Event-
  // ID) resumes from the DB, so a dropped realtime link self-heals; seq-merge dedups.
  React.useEffect(() => {
    setStore({});
    setSseLive(false);
    setActiveId(MAIN_ID); // a stale worker id from the previous task must not leak in
    setMainSegs([{ seq: 0, created_at: "" }]);
    setCurrentSeg(0);
    snapshotRef.current = 0;
    llmToastSeqRef.current = new Set();
    reqTokenRef.current = {};
    // Reserve the initial main key so the active-session effect (which fires for main on
    // mount) won't double-load it and pre-empt the SSE opened here.
    loadingKeysRef.current = new Set([mainSessionKey(0)]);
    let alive = true;

    // MOCK demo: no SSE backend — pull one activity snapshot and bucket by session.
    if (MOCK) {
      api
        .activity(taskId)
        .then((r) => {
          if (!alive) return;
          setStore(bucketMockActivity(r.items));
        })
        .catch(() =>
          setStore({
            [mainSessionKey(0)]: loadedState(),
            system: loadedState(),
          }),
        );
      return () => {
        alive = false;
      };
    }

    const onFrame = (a: Activity) => {
      if ((a.kind === "llm_switch" || a.kind === "llm_failover") && !llmToastSeqRef.current.has(a.seq)) {
        llmToastSeqRef.current.add(a.seq);
        const transition = a.metadata?.llm_transition;
        if (transition?.mode === "exhausted" || a.is_error) {
          toast.error(a.summary, { id: `task-${taskId}-llm-${a.seq}` });
        } else if (transition?.mode === "automatic") {
          toast.success(a.summary, { id: `task-${taskId}-llm-${a.seq}` });
        } else {
          toast.info(a.summary, { id: `task-${taskId}-llm-${a.seq}` });
        }
        void api
          .taskLLMResolution(taskId)
          .then((value) => {
            if (alive) setLLMResolutions(value);
          })
          .catch(() => {
            // The periodic resolver poll will retry if this event-triggered refresh fails.
          });
      }
      setStore((prev) => applyFrame(prev, a, activeKeyRef.current, atBottomRef.current));
    };

    const token = (reqTokenRef.current.mainboot ?? 0) + 1;
    reqTokenRef.current.mainboot = token;
    let bootKey = mainSessionKey(0);
    // Resolve the main-agent segments first, then load the CURRENT segment's history and
    // open the SSE from its snapshot cursor. The SSE tails all segments and routes each
    // frame by session_key (main:<seg>), so switching segments needs no new stream.
    api
      .mainSessions(taskId)
      .then((ms) => {
        if (!alive || reqTokenRef.current.mainboot !== token) throw new Error("superseded");
        const segs = ms.sessions.length ? ms.sessions : [{ seq: 0, created_at: "" }];
        setMainSegs(segs);
        setCurrentSeg(ms.current);
        bootKey = mainSessionKey(ms.current);
        if (bootKey !== mainSessionKey(0)) loadingKeysRef.current.delete(mainSessionKey(0));
        loadingKeysRef.current.add(bootKey);
        setActiveId(mainSessionId(ms.current));
        patchStore(bootKey, (s) => ({ ...s, loading: true }));
        return api.activityHistory(taskId, bootKey, 0, PAGE);
      })
      .then((r) => {
        if (!alive || reqTokenRef.current.mainboot !== token) return;
        snapshotRef.current = r.snapshotCursor;
        patchStore(bootKey, (s) => {
          const items = mergeBySeq(r.items, s.items);
          return {
            ...s,
            items,
            loaded: true,
            loading: false,
            hasMore: r.hasMore,
            earliestSeq: items.length ? items[0].seq : 0,
            unread: 0,
          };
        });
        // Open the single task SSE from the snapshot cursor.
        const es = new EventSource(
          sseUrl(`/api/exploration/activity/stream?task=${encodeURIComponent(taskId)}&since=${snapshotRef.current}`),
        );
        esRef.current = es;
        es.onopen = () => setSseLive(true);
        es.onerror = () => setSseLive(false); // EventSource auto-reconnects; DB compensates the gap
        es.onmessage = (e) => {
          let a: Activity;
          try {
            a = JSON.parse(e.data) as Activity;
          } catch {
            return; // ignore malformed frame
          }
          onFrame(a);
        };
      })
      .catch((err) => {
        if (!alive || reqTokenRef.current.mainboot !== token) return;
        if ((err as Error).message === "superseded") return;
        patchStore(bootKey, (s) => ({ ...s, loading: false, error: (err as Error).message || t("Failed to load") }));
      })
      .finally(() => loadingKeysRef.current.delete(bootKey));

    return () => {
      alive = false;
      esRef.current?.close();
      esRef.current = null;
    };
  }, [taskId, patchStore, setActiveId, setLLMResolutions]);

  return {
    store,
    setStore,
    patchStore,
    loadSession,
    loadEarlier,
    mainSegs,
    setMainSegs,
    currentSeg,
    setCurrentSeg,
    sseLive,
    activeKeyRef,
    atBottomRef,
  };
}
