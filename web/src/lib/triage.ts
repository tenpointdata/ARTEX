// Keyboard-first triage of a review queue: a verdict is one key or one click, and the
// queue advances to the next item on its own. Pure and framework-free so any list
// (findings and the interception approval queue today) can reuse it. The verdict set is a
// parameter: each queue has its own keys and the values it writes. The React key binding is
// `useTriageKeys` in `@/hooks/use-triage-keys`.

import type { FindingStatus } from "@/lib/types";

export interface TriageVerdict<S extends string = FindingStatus> {
  /** The status the verdict writes. */
  status: S;
  /** The single lower-case key that applies the verdict while an item is open. */
  key: string;
}

// The verdicts a reviewer reaches for, in the order they are shown. The keys are
// mnemonic (c = confirm, f = false positive …) and avoid the navigation keys below.
export const TRIAGE_VERDICTS: readonly TriageVerdict<FindingStatus>[] = [
  { status: "confirmed", key: "c" },
  { status: "false_positive", key: "f" },
  { status: "duplicate", key: "d" },
  { status: "risk_accepted", key: "a" },
  { status: "ignored", key: "i" },
  { status: "in_progress", key: "p" },
];

/** The decisions an interception approval takes (the body of POST /intercept/pending/{id}/decide). */
export type ApprovalDecision = "allowed" | "denied";

// The approval queue's verdicts: a = allow, d = deny. They only write the same decision the Allow and
// Deny buttons write; the gate itself (who may decide, what a decision does) is the server's.
export const APPROVAL_VERDICTS: readonly TriageVerdict<ApprovalDecision>[] = [
  { status: "allowed", key: "a" },
  { status: "denied", key: "d" },
];

export type TriageNav = "next" | "prev" | "close";

const NAV_KEYS: Readonly<Record<string, TriageNav>> = {
  j: "next",
  arrowdown: "next",
  k: "prev",
  arrowup: "prev",
  escape: "close",
};

export type TriageCommand<S extends string = FindingStatus> =
  | { kind: "verdict"; status: S }
  | { kind: "nav"; nav: TriageNav };

/**
 * Maps a KeyboardEvent.key (with no modifier held) to a triage command, or null. `verdicts` is the
 * queue's verdict set (the findings verdicts when omitted); navigation keys are the same everywhere.
 */
export function triageCommandForKey(key: string): TriageCommand<FindingStatus> | null;
export function triageCommandForKey<S extends string>(
  key: string,
  verdicts: readonly TriageVerdict<S>[],
): TriageCommand<S> | null;
export function triageCommandForKey(
  key: string,
  verdicts: readonly TriageVerdict<string>[] = TRIAGE_VERDICTS,
): TriageCommand<string> | null {
  const k = key.toLowerCase();
  const verdict = verdicts.find((v) => v.key === k);
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

/**
 * The page to load when triage runs off the end of the open page, or null when the list is done.
 * `total` is the list's size before the verdict. When the verdict `dropped` the item out of the
 * filter, later items slide up into the same page, so it is reloaded; otherwise the next page is.
 */
export function triageNextPage(page: number, pageSize: number, total: number, dropped: boolean): number | null {
  if (dropped) return (page - 1) * pageSize < total - 1 ? page : null;
  return page * pageSize < total ? page + 1 : null;
}

/** The first key of a reloaded page that was not on the page before (items already seen are skipped). */
export function firstNewKey(before: readonly string[], after: readonly string[]): string | null {
  const seen = new Set(before);
  return after.find((key) => !seen.has(key)) ?? null;
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
