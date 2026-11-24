// Keyboard-first triage of a review queue: a verdict is one key or one click, and the
// queue advances to the next item on its own. Pure and framework-free so any list
// (findings today; approvals or imports later) can reuse it; the React bindings live
// next to the consumer.

import type { FindingStatus } from "@/lib/types";

export interface TriageVerdict {
  /** The status the verdict writes. */
  status: FindingStatus;
  /** The single lower-case key that applies the verdict while an item is open. */
  key: string;
}

// The verdicts a reviewer reaches for, in the order they are shown. The keys are
// mnemonic (c = confirm, f = false positive …) and avoid the navigation keys below.
export const TRIAGE_VERDICTS: readonly TriageVerdict[] = [
  { status: "confirmed", key: "c" },
  { status: "false_positive", key: "f" },
  { status: "duplicate", key: "d" },
  { status: "risk_accepted", key: "a" },
  { status: "ignored", key: "i" },
  { status: "in_progress", key: "p" },
];

export type TriageNav = "next" | "prev" | "close";

const NAV_KEYS: Readonly<Record<string, TriageNav>> = {
  j: "next",
  arrowdown: "next",
  k: "prev",
  arrowup: "prev",
  escape: "close",
};

export type TriageCommand = { kind: "verdict"; status: FindingStatus } | { kind: "nav"; nav: TriageNav };

/** Maps a KeyboardEvent.key (with no modifier held) to a triage command, or null. */
export function triageCommandForKey(key: string): TriageCommand | null {
  const k = key.toLowerCase();
  const verdict = TRIAGE_VERDICTS.find((v) => v.key === k);
  if (verdict) return { kind: "verdict", status: verdict.status };
  const nav = NAV_KEYS[k];
  return nav ? { kind: "nav", nav } : null;
}

/**
 * The key of the item to open after the current one is triaged or skipped.
 * `next` walks forward and `prev` backward; at either end there is no target (null).
 * When the current key is not in the list (it was removed, or nothing is open),
 * `next` starts at the first item.
 */
export function triageTarget(keys: readonly string[], current: string | null, nav: "next" | "prev"): string | null {
  const at = current === null ? -1 : keys.indexOf(current);
  if (at === -1) return nav === "next" ? (keys[0] ?? null) : null;
  const to = nav === "next" ? at + 1 : at - 1;
  return to >= 0 && to < keys.length ? keys[to] : null;
}

/** Whether an item with `status` still matches a status filter ("all" keeps everything). */
export function matchesStatusFilter(filter: "all" | FindingStatus, status: FindingStatus): boolean {
  return filter === "all" || filter === status;
}

/** Whether a keydown came from somewhere the user is typing (shortcuts must not fire there). */
export function isTypingTarget(target: { tagName?: string; isContentEditable?: boolean } | null): boolean {
  if (!target) return false;
  if (target.isContentEditable) return true;
  const tag = (target.tagName ?? "").toUpperCase();
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}
