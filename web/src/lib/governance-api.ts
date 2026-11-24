// Client for the E5 governance endpoints. Kept out of lib/api.ts (already over
// the file-size budget); it reuses that module's http() so 401 handling,
// ApiError and mock mode stay identical.

import { http } from "@/lib/api";
import type {
  AuditEntry,
  AuditFilter,
  AuditVerification,
  AuthorizationInput,
  AuthorizationView,
  ConsoleUser,
  EngagementAuthorization,
  NewUser,
  UserList,
  UserPatch,
} from "@/lib/governance-types";

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

function chainQuery(f: AuditFilter): string {
  const q = new URLSearchParams();
  if (f.after_seq) q.set("after_seq", String(f.after_seq));
  if (f.kind) q.set("kind", f.kind);
  if (f.subject) q.set("subject", f.subject);
  if (f.limit) q.set("limit", String(f.limit));
  const s = q.toString();
  return s ? `?${s}` : "";
}

export const governanceApi = {
  // ---- users (admin) ----
  users: () => http<UserList>("/users").then((r) => ({ users: r.users ?? [], roles: r.roles ?? [] })),
  createUser: (u: NewUser) => http<ConsoleUser>("/users", json("POST", u)),
  updateUser: (id: number, patch: UserPatch) => http<ConsoleUser>(`/users/${id}`, json("PATCH", patch)),
  deleteUser: (id: number) => http<{ ok: boolean }>(`/users/${id}`, json("DELETE")),
  userCompanies: (id: number) =>
    http<{ company_ids: number[] | null }>(`/users/${id}/companies`).then((r) => r.company_ids ?? []),
  setUserCompanies: (id: number, companyIds: number[]) =>
    http<{ company_ids: number[] | null }>(`/users/${id}/companies`, json("PUT", { company_ids: companyIds })).then(
      (r) => r.company_ids ?? [],
    ),

  // ---- task authorization artifact ----
  authorization: (taskId: string) => http<AuthorizationView>(`/tasks/${encodeURIComponent(taskId)}/authorization`),
  addAuthorization: (taskId: string, input: AuthorizationInput) =>
    http<EngagementAuthorization>(`/tasks/${encodeURIComponent(taskId)}/authorization`, json("POST", input)),
  revokeAuthorization: (taskId: string, note: string) =>
    http<EngagementAuthorization>(
      `/tasks/${encodeURIComponent(taskId)}/authorization/revoke`,
      json("POST", note ? { note } : {}),
    ),

  // ---- hash-chained audit log ----
  auditChain: (f: AuditFilter = {}) =>
    http<{ entries: AuditEntry[] | null }>(`/intercept/history/chain${chainQuery(f)}`).then((r) => r.entries ?? []),
  verifyAuditChain: () => http<AuditVerification>("/intercept/history/chain/verify"),
};
