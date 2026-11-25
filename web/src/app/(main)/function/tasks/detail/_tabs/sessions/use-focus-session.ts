import * as React from "react";

import { type useApprovalFocus, useApprovalHistory } from "@/components/approval-execution-focus";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Activity, Session } from "@/lib/types";

import { MAIN_SESSION, mainSessionId, mainSessionTitle, PAGE, PLANNER_SESSION } from "./helpers";
import { mergeBySeq, type SessionState, type SessionStore } from "./store";

// useFocusSession resolves an approval "locate" request to the session that holds the
// call, retains it after focus mode closes, and pages history back to the call.
export function useFocusSession({
  taskId,
  approvalFocus,
  store,
  patchStore,
  atBottomRef,
}: {
  taskId: string;
  approvalFocus: ReturnType<typeof useApprovalFocus>;
  store: SessionStore;
  patchStore: (key: string, fn: (s: SessionState) => SessionState) => void;
  atBottomRef: React.MutableRefObject<boolean>;
}) {
  const focusSession = React.useMemo<Session | undefined>(() => {
    const source = approvalFocus.state?.source;
    if (!source) return undefined;
    if (source.session.startsWith("main:")) {
      const seg = Number(source.session.slice(5));
      return { ...MAIN_SESSION, id: mainSessionId(seg), seg, title: mainSessionTitle(seg), live: false };
    }
    if (source.session === "plan") return { ...PLANNER_SESSION, live: false };
    if (source.session.startsWith("intent:")) {
      const id = source.session.slice(7);
      return {
        id,
        role: "worker",
        intent_id: id,
        title: t("Worker #{id}", { id }),
        status: "done",
        live: false,
        last_activity: source.items[0]?.ts ?? "",
      };
    }
    return undefined;
  }, [approvalFocus.state?.source]);
  // Keep a located archived/older session selectable after leaving focus mode.
  const [locatedSession, setLocatedSession] = React.useState<{ taskId: string; session: Session }>();
  React.useEffect(() => {
    if (focusSession) setLocatedSession({ taskId, session: focusSession });
  }, [taskId, focusSession]);
  const retainedSession = locatedSession?.taskId === taskId ? locatedSession.session : undefined;

  const focusKey = approvalFocus.state?.source?.session;
  const loadFocusPage = React.useCallback(
    (before: number) => api.activityHistory(taskId, focusKey ?? "main", before, PAGE),
    [taskId, focusKey],
  );
  const mergeFocusPage = React.useCallback(
    (page: { items: Activity[]; hasMore: boolean }) => {
      if (!focusKey) return;
      patchStore(focusKey, (s) => {
        const items = mergeBySeq(page.items, s.items);
        return { ...s, items, hasMore: page.hasMore, earliestSeq: items[0]?.seq ?? s.earliestSeq };
      });
    },
    [focusKey, patchStore],
  );
  const focusHistory = useApprovalHistory(
    approvalFocus.state?.source,
    !!focusKey && !!store[focusKey]?.loaded,
    focusKey ? (store[focusKey]?.items ?? []) : [],
    loadFocusPage,
    mergeFocusPage,
  );
  React.useEffect(() => {
    if (approvalFocus.state) atBottomRef.current = false;
  }, [approvalFocus.state, atBottomRef]);

  return { focusSession, retainedSession, focusHistory };
}
