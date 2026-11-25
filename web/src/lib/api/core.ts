import { t } from "@/i18n";
import { MOCK } from "@/lib/mock/enabled";
import { mockHandle } from "@/lib/mock/handler";
import type { FindingQuery, InterceptApprovalFilter } from "@/lib/types";

// Combined file data per attachment upload; matches maxChatUpload in server/chatupload.go.
export const MAX_CHAT_UPLOAD_BYTES = 512 * 1024 * 1024;

// The session is an HttpOnly cookie the browser attaches by itself; scripts hold no token.
// Kept as a function so the Authorization-header call sites below stay inert.
export function getToken(): string | null {
  return null;
}

// ApiError is what http() throws for a non-2xx response: the message is the
// server's {error} (or "METHOD path: status"), and status lets callers tell
// failures apart.
export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export async function http<T>(path: string, init?: RequestInit): Promise<T> {
  if (MOCK) return mockHandle<T>(init?.method ?? "GET", path, init?.body ?? null);
  const token = getToken();
  const r = await fetch(`/api${path}`, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers as Record<string, string> | undefined),
    },
  });
  if (r.status === 401) {
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
    throw new Error(t("Unauthorized"));
  }
  if (!r.ok) {
    const fallback = `${init?.method ?? "GET"} ${path}: ${r.status}`;
    let message = fallback;
    try {
      const payload = (await r.json()) as { error?: unknown };
      if (typeof payload.error === "string" && payload.error.trim()) {
        message = payload.error.trim();
      }
    } catch {
      // Keep the status-based fallback for empty or non-JSON error responses.
    }
    throw new ApiError(message, r.status);
  }
  if (r.status === 204) return undefined as T;
  return r.json();
}

// mockReport returns a canned Markdown report for the demo.
export function mockReport(_task?: string): string {
  return `# ARTEX Penetration Test Report — Acme Corp

## Overview
- Scope: acme.com (incl. www / admin / api / shop / vpn subdomains)
- Confirmed findings: 6 (High 3 · Medium 3 · Low 2)
- Engine mode: exploring

## Key findings
1. **[High] Default admin credentials** admin.acme.com admin/admin123 → full backend takeover.
2. **[High] SQL injection** www.acme.com/search?q= → can read the acme_prod database.
3. **[High] IDOR** api.acme.com/v1/orders?id= → can read other users' orders without authorization (incl. phone numbers/addresses).
4. **[Medium] Reflected XSS**, **exposed .git source**, **login without rate limiting**.

## Recommendations
- Enforce admin password changes + enable MFA and ban default credentials.
- Parameterize search endpoint queries and encode output.
- Add object-level authorization checks to the API (IDOR) and rotate to a strong JWT secret.

> (demo) This report is generated from mock data for UI demonstration only.`;
}

export const get = <T>(p: string) => http<T>(p);
export const post = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "POST", body: body ? JSON.stringify(body) : undefined });
export const put = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "PUT", body: body ? JSON.stringify(body) : undefined });
export const patch = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "PATCH", body: body ? JSON.stringify(body) : undefined });
export const del = <T>(p: string, body?: unknown) =>
  http<T>(p, { method: "DELETE", body: body ? JSON.stringify(body) : undefined });

// Go serializes nil slices as JSON null — coerce to [].
export const arr = <T>(x: T[] | null | undefined): T[] => x ?? [];
export const tq = (task?: string, sep: "?" | "&" = "?") => (task ? `${sep}task=${encodeURIComponent(task)}` : "");

// findingFilterParams serializes the findings page filters into a query string. List /
// groups / asset tree / export all share this one copy; add new filters only here (the backend parses only this one too).
export function findingFilterParams(q: Omit<FindingQuery, "page" | "pageSize">): URLSearchParams {
  const p = new URLSearchParams();
  if (q.severity && q.severity !== "all") p.set("severity", q.severity);
  if (q.status && q.status !== "all") p.set("status", q.status);
  if (q.reportState && q.reportState !== "all") p.set("report_state", q.reportState);
  if (q.verificationStatus && q.verificationStatus !== "all") p.set("verification_status", q.verificationStatus);
  if (q.vulnclass && q.vulnclass !== "all") p.set("vulnclass", q.vulnclass);
  if (q.task && q.task !== "all") p.set("task_id", q.task);
  if (q.query?.trim()) p.set("q", q.query.trim());
  if (q.sort) p.set("sort", q.sort);
  if (q.assetScope) p.set("asset_scope", q.assetScope);
  if (q.company) p.set("company", q.company);
  if (q.importId) p.set("import_id", q.importId);
  return p;
}

export function interceptPageQuery(page: number, size: number, filter: InterceptApprovalFilter) {
  const query = new URLSearchParams({ page: String(page), size: String(size) });
  if (filter.status) query.set("status", filter.status);
  if (filter.decision_source) query.set("decision_source", filter.decision_source);
  if (filter.decided_by) query.set("decided_by", filter.decided_by);
  return query.toString();
}
