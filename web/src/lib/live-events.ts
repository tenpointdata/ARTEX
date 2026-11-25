// The console's live push: GET /api/events/stream (server/events_stream.go).
//
// Each frame says that a finding, the approval queue or a task's status changed,
// or ("resync") that the stream may have missed changes. Frames carry ids only:
// a page re-reads through its normal endpoints. A client_read session must name
// its bound company; the server then sends only that company's findings and tasks.

export type LiveEventKind = "finding" | "approval" | "task" | "resync";
export type LiveEventOp = "insert" | "update" | "delete";

export interface LiveEvent {
  kind: LiveEventKind;
  op?: LiveEventOp;
  id?: number;
  task?: string;
}

const KINDS: readonly LiveEventKind[] = ["finding", "approval", "task", "resync"];
const OPS: readonly LiveEventOp[] = ["insert", "update", "delete"];

function isKind(v: unknown): v is LiveEventKind {
  return typeof v === "string" && (KINDS as readonly string[]).includes(v);
}

function isOp(v: unknown): v is LiveEventOp {
  return typeof v === "string" && (OPS as readonly string[]).includes(v);
}

// parseLiveEvent decodes one frame's data; an unknown kind or malformed frame is null.
export function parseLiveEvent(data: string): LiveEvent | null {
  let raw: unknown;
  try {
    raw = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof raw !== "object" || raw === null) return null;
  const r = raw as Record<string, unknown>;
  if (!isKind(r.kind)) return null;
  const ev: LiveEvent = { kind: r.kind };
  if (isOp(r.op)) ev.op = r.op;
  if (typeof r.id === "number") ev.id = r.id;
  if (typeof r.task === "string") ev.task = r.task;
  return ev;
}

// liveEventsPath is the stream's same-origin path (open it through sseUrl),
// narrowed to a company when one is chosen.
export function liveEventsPath(company?: string): string {
  const q = company ? `?company=${encodeURIComponent(company)}` : "";
  return `/api/events/stream${q}`;
}

// touches reports whether ev may change data of one of the given kinds; a resync touches everything.
export function touches(ev: LiveEvent, kinds: readonly LiveEventKind[]): boolean {
  return ev.kind === "resync" || kinds.includes(ev.kind);
}
