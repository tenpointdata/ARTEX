"use client";

import * as React from "react";

import { type LiveEvent, liveEventsPath, parseLiveEvent } from "@/lib/live-events";
import { MOCK } from "@/lib/mock/enabled";
import { sseUrl } from "@/lib/sse";

// useLiveEvents keeps one EventSource on GET /api/events/stream open while the
// component is mounted and the tab is visible, and calls onEvent for each frame.
// It returns whether the stream is open now, so a caller can poll fast only while
// it is not. EventSource reconnects by itself (a refused stream, e.g. 403, stays
// closed and the caller keeps polling). Every open, first or reconnect, is
// reported as a resync: frames sent while no stream was open are gone, and the
// first open closes the gap between the caller's initial load and the subscription.
export function useLiveEvents(onEvent: (ev: LiveEvent) => void, company?: string): boolean {
  const [live, setLive] = React.useState(false);
  const handler = React.useRef(onEvent);
  handler.current = onEvent;

  React.useEffect(() => {
    if (MOCK || typeof EventSource === "undefined") return;
    let es: EventSource | null = null;
    const close = () => {
      es?.close();
      es = null;
      setLive(false);
    };
    const open = () => {
      if (es && es.readyState !== EventSource.CLOSED) return;
      es?.close();
      const src = new EventSource(sseUrl(liveEventsPath(company)));
      es = src;
      src.onopen = () => {
        setLive(true);
        handler.current({ kind: "resync" });
      };
      src.onerror = () => setLive(src.readyState === EventSource.OPEN);
      src.onmessage = (m: MessageEvent<string>) => {
        const ev = parseLiveEvent(m.data);
        if (ev) handler.current(ev);
      };
    };
    const onVisibility = () => {
      if (document.visibilityState === "hidden") {
        close();
        return;
      }
      open();
    };
    if (document.visibilityState !== "hidden") open();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      close();
    };
  }, [company]);

  return live;
}
