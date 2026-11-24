import * as React from "react";

import type { Activity } from "@/lib/types";

// useTranscriptScroll keeps the transcript pinned to the live edge (open → bottom;
// stick to bottom unless the user scrolled up) and pages in older history near the top.
export function useTranscriptScroll({
  activeId,
  activity,
  focusing,
  focusReady,
  atBottomRef,
  onNearTop,
}: {
  activeId: string;
  activity: Activity[];
  // An approval-focus is open: scrolling is owned by the focus, not the live edge.
  focusing: boolean;
  focusReady: boolean;
  atBottomRef: React.MutableRefObject<boolean>;
  onNearTop: (viewport: () => HTMLElement | null) => void;
}) {
  const contentRef = React.useRef<HTMLDivElement | null>(null);
  const viewport = React.useCallback(
    () => (contentRef.current?.closest('[data-slot="scroll-area-viewport"]') as HTMLElement | null) ?? null,
    [],
  );
  // biome-ignore lint/correctness/useExhaustiveDependencies: activeId intentionally rebinds the scroll listener.
  React.useEffect(() => {
    const vp = viewport();
    if (!vp) return;
    const onScroll = () => {
      if (focusing && !focusReady) return;
      atBottomRef.current = vp.scrollTop + vp.clientHeight >= vp.scrollHeight - 60;
      if (vp.scrollTop <= 80) onNearTop(viewport); // near top → older page
    };
    vp.addEventListener("scroll", onScroll, { passive: true });
    return () => vp.removeEventListener("scroll", onScroll);
  }, [viewport, activeId, onNearTop, focusing, focusReady, atBottomRef]);
  // open/switch a session → jump to the latest (bottom)
  // biome-ignore lint/correctness/useExhaustiveDependencies: activeId intentionally scrolls a newly selected session.
  React.useLayoutEffect(() => {
    const vp = viewport();
    if (vp && !focusing) {
      vp.scrollTop = vp.scrollHeight;
      atBottomRef.current = true;
    }
  }, [activeId, viewport, focusing, atBottomRef]);
  // new activity → stick to bottom only if the user is already pinned there
  // biome-ignore lint/correctness/useExhaustiveDependencies: activity growth intentionally drives live-edge scrolling.
  React.useLayoutEffect(() => {
    if (focusing || !atBottomRef.current) return;
    const vp = viewport();
    if (vp) vp.scrollTop = vp.scrollHeight;
  }, [activity, viewport, focusing, atBottomRef]);
  // Lazy detail loads (AnswerBlock / ToolBlock / Markdown) grow the content AFTER the
  // activity array settles, WITHOUT changing its reference — so the layout effects
  // above never re-fire and a freshly opened session would leave its last message
  // scrolled partly off-screen (the final answer expands from a one-line summary to
  // full markdown below the fold). A ResizeObserver re-pins to the bottom on any
  // height change while the user is still at the bottom, so opening the main agent
  // lands on the last message fully shown. contentRef's div is always mounted, so the
  // observer catches the transcript mounting + each detail expanding.
  // biome-ignore lint/correctness/useExhaustiveDependencies: activeId intentionally rebinds the observer to the new session's content.
  React.useEffect(() => {
    const el = contentRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => {
      if (focusing || !atBottomRef.current) return;
      const vp = viewport();
      if (vp) vp.scrollTop = vp.scrollHeight;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [activeId, viewport, focusing, atBottomRef]);

  return { contentRef };
}
