// Pure helpers for the governance UI. No "@/" imports, so node --test can load
// this file directly (governance.test.mjs).

import type { AuthScopeEntry, Permission, ScopeKind, SessionInfo, TaskAuthorizationSummary } from "./governance-types";

const SCOPE_KIND_SET: ReadonlySet<string> = new Set<ScopeKind>(["domain", "host", "ip", "cidr"]);

// can reports whether the session holds permission p. The API enforces the
// policy regardless; the UI only uses this to hide actions it would refuse.
export function can(session: Pick<SessionInfo, "permissions"> | null | undefined, p: Permission): boolean {
  return Array.isArray(session?.permissions) && session.permissions.includes(p);
}

// isForbidden tells a 403 (role refused; stay signed in) from other failures.
export function isForbidden(err: unknown): boolean {
  return typeof err === "object" && err !== null && (err as { status?: unknown }).status === 403;
}

// guessScopeKind picks the kind for an entry typed without one.
export function guessScopeKind(value: string): ScopeKind {
  if (value.includes("/")) return "cidr";
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(value) || value.includes(":")) return "ip";
  return "domain";
}

// parseScopeText reads one entry per line (or comma), as "kind:value" with a
// known kind, or a bare value whose kind is guessed. The server validates each
// entry; values it rejects come back as a 400 with the reason.
export function parseScopeText(text: string): AuthScopeEntry[] {
  const scope: AuthScopeEntry[] = [];
  const seen = new Set<string>();
  for (const raw of text.split(/[\n,]/)) {
    const line = raw.trim();
    if (!line) continue;
    const m = /^([a-z]+):(.+)$/i.exec(line);
    const prefix = m?.[1].toLowerCase() ?? "";
    const explicit = m !== null && SCOPE_KIND_SET.has(prefix);
    const kind = explicit ? (prefix as ScopeKind) : guessScopeKind(line);
    const value = explicit && m ? m[2].trim() : line;
    const key = `${kind}\u0000${value}`;
    if (!value || seen.has(key)) continue;
    seen.add(key);
    scope.push({ kind, value });
  }
  return scope;
}

export function formatScopeText(scope: readonly AuthScopeEntry[] | null | undefined): string {
  return (scope ?? []).map((e) => `${e.kind}:${e.value}`).join("\n");
}

// toLocalInput renders an RFC 3339 time for <input type="datetime-local">.
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// fromLocalInput turns a datetime-local value (the browser's zone) into RFC 3339 UTC.
export function fromLocalInput(local: string): string {
  if (!local) return "";
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

export type AuthorizationStatus = "in_force" | "revoked" | "not_yet" | "expired" | "none";

// authorizationStatus explains in_force: why an artifact does or does not authorize now.
export function authorizationStatus(
  current: { revoked: boolean; not_before: string; not_after: string } | null | undefined,
  now: string,
): AuthorizationStatus {
  if (!current) return "none";
  if (current.revoked) return "revoked";
  const t = Date.parse(now);
  if (t < Date.parse(current.not_before)) return "not_yet";
  if (t >= Date.parse(current.not_after)) return "expired";
  return "in_force";
}

// landingPath is where a session starts: the task list, or the findings for a
// client_read account (it cannot list tasks).
export function landingPath(session: Pick<SessionInfo, "permissions"> | null | undefined): string {
  return can(session, "read") ? "/function/tasks" : "/function/findings";
}

// resumeBlockedBy is the artifact that makes the server refuse to resume a
// paused task (server/authorization_expiry.go: revoked or past not_after), or
// null when resume is not refused for that reason. A window that has not
// started yet does not block resume.
export function resumeBlockedBy(
  paused: boolean,
  authorization: TaskAuthorizationSummary | null | undefined,
): TaskAuthorizationSummary | null {
  return paused && authorization?.ended ? authorization : null;
}

// targetGateLocked: a task with an authorization cannot turn its target gate
// off (PATCH answers 409, db.ErrTargetGateRequired).
export function targetGateLocked(authorization: TaskAuthorizationSummary | null | undefined): boolean {
  return authorization != null;
}
