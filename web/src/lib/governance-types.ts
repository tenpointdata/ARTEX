// Types for the E5 engagement-governance API: console users and roles, the
// per-task authorization artifact, and the hash-chained audit log. They mirror
// the JSON of server/users_api.go, server/governance_api.go, db/users.go,
// db/authorization.go and db/audit_chain.go; see docs/GOVERNANCE.md.

// Permissions are sets, not a ladder (server/rbac_policy.go: allPermissions).
export const PERMISSIONS = ["client_read", "read", "review", "operate", "admin"] as const;
export type Permission = (typeof PERMISSIONS)[number];

// Roles as returned by GET /api/users (db.UserRoles).
export const ROLES = ["admin", "operator", "reviewer", "client_read"] as const;
export type Role = (typeof ROLES)[number];

// GET /api/auth/session. Only an authenticated session carries the rest.
export interface SessionInfo {
  authenticated: boolean;
  username?: string;
  role?: string;
  permissions?: string[];
}

// db.User.
export interface ConsoleUser {
  id: number;
  username: string;
  role: Role;
  disabled: boolean;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface UserList {
  users: ConsoleUser[];
  roles: Role[];
}

export interface NewUser {
  username: string;
  password: string;
  role: Role;
}

// PATCH /api/users/{id}: every field is optional.
export interface UserPatch {
  role?: Role;
  disabled?: boolean;
  password?: string;
}

export const SCOPE_KINDS = ["domain", "host", "ip", "cidr"] as const;
export type ScopeKind = (typeof SCOPE_KINDS)[number];

// db.AuthScopeEntry.
export interface AuthScopeEntry {
  kind: ScopeKind;
  value: string;
}

// db.AuthorizationInput: the body of POST /api/tasks/{id}/authorization.
export interface AuthorizationInput {
  reference: string;
  authorized_by: string;
  scope: AuthScopeEntry[];
  techniques: string[];
  not_before: string; // RFC 3339
  not_after: string; // RFC 3339
  note: string;
}

// db.EngagementAuthorization: one immutable version.
export interface EngagementAuthorization extends AuthorizationInput {
  id: number;
  task_id: number;
  version: number;
  revoked: boolean;
  created_by: string;
  created_at: string;
}

// server.TaskAuthorizationDTO: the newest version's summary on every task
// (GET /api/tasks and /api/tasks/{id}); absent when the task has none. Its
// presence means the target gate is required; ended means resume is refused
// until a new version is stored.
export interface TaskAuthorizationSummary {
  version: number;
  revoked: boolean;
  ended: boolean;
}

// GET /api/tasks/{id}/authorization. versions is newest first.
export interface AuthorizationView {
  current: EngagementAuthorization | null;
  in_force: boolean;
  versions: EngagementAuthorization[] | null;
  technique_classes: string[];
  server_time: string;
}

// db.AuditEntry. payload is arbitrary JSON written by the triggers.
export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

export interface AuditEntry {
  seq: number;
  at: string;
  actor: string;
  kind: string;
  subject: string;
  payload: JsonValue;
  prev_hash: string;
  hash: string;
}

// Query of GET /api/intercept/history/chain. kind "row." is a prefix filter.
export interface AuditFilter {
  after_seq?: number;
  kind?: string;
  subject?: string;
  limit?: number;
}

// db.AuditVerification: GET /api/intercept/history/chain/verify.
export interface AuditVerification {
  ok: boolean;
  entries: number;
  head_seq: number;
  head_hash: string;
  broken_at?: number;
  problem?: string;
  rows_checked: number;
  rows_mismatched: number[] | null;
  rows_unchained: number;
}
