import * as React from "react";

import { api } from "@/lib/api";
import { setVisibleInterval } from "@/lib/polling";
import type { InterceptApprovalRow, SessionTokenUsage, TaskLLMResolutions, TokenTotal } from "@/lib/types";

// useTaskPolling owns the best-effort, visibility-aware polls of task-level aggregates:
// token totals, the LLM resolution per role, pending intercepts, and the main-agent
// chat-running flag. Every poll resets when the task changes.
export function useTaskPolling(taskId: string) {
  // Whole-task token total (all agents), polled from the backend aggregate.
  const [taskTokens, setTaskTokens] = React.useState<TokenTotal | null>(null);
  const [sessionTokens, setSessionTokens] = React.useState<Record<string, SessionTokenUsage>>({});
  const [llmResolutions, setLLMResolutions] = React.useState<TaskLLMResolutions | null>(null);
  // Pending intercept requests for this task — used to show warning icons on sessions.
  const [pendingIntercepts, setPendingIntercepts] = React.useState<InterceptApprovalRow[]>([]);
  const [mainChatRunning, setMainChatRunning] = React.useState<boolean | null>(null);
  const chatStatusRequestRef = React.useRef(0);

  React.useEffect(() => {
    let alive = true;
    setTaskTokens(null);
    setSessionTokens({});
    const load = () =>
      api
        .tokenStats(taskId)
        .then((r) => {
          if (!alive) return;
          setTaskTokens(r.total);
          setSessionTokens(Object.fromEntries(r.sessions.map((item) => [item.session, item])));
        })
        .catch(() => {
          // Polling is best-effort; the next interval retries automatically.
        });
    void load();
    const stopPolling = setVisibleInterval(load, 5000);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [taskId]);

  React.useEffect(() => {
    let alive = true;
    setMainChatRunning(null);
    const load = () => {
      const request = ++chatStatusRequestRef.current;
      return api
        .chatStatus(taskId)
        .then(({ running }) => {
          if (alive && chatStatusRequestRef.current === request) setMainChatRunning(running);
        })
        .catch(() => {
          // Keep the last authoritative value. Before the first success, recent
          // activity remains a conservative fallback.
        });
    };
    void load();
    const stopPolling = setVisibleInterval(load, 2000);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [taskId]);

  React.useEffect(() => {
    let alive = true;
    setLLMResolutions(null);
    const load = () =>
      api
        .taskLLMResolution(taskId)
        .then((value) => {
          if (alive) setLLMResolutions(value);
        })
        .catch(() => {
          // Polling is best-effort; the next interval retries automatically.
        });
    void load();
    const stopPolling = setVisibleInterval(load, 10_000);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [taskId]);

  React.useEffect(() => {
    let alive = true;
    const load = () =>
      api
        .interceptTask(taskId)
        .then((rows) => {
          if (alive) setPendingIntercepts(rows.filter((r) => r.status === "pending"));
        })
        .catch(() => {
          // Polling is best-effort; the next interval retries automatically.
        });
    void load();
    const stopPolling = setVisibleInterval(load, 5000);
    return () => {
      alive = false;
      stopPolling();
    };
  }, [taskId]);

  return {
    taskTokens,
    sessionTokens,
    llmResolutions,
    setLLMResolutions,
    pendingIntercepts,
    mainChatRunning,
    setMainChatRunning,
    chatStatusRequestRef,
  };
}
